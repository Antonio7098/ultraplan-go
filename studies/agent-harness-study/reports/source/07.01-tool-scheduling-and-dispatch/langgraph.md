# Source Analysis: langgraph

## Dimension 07.01: Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, prebuilt agents/tools `libs/prebuilt`); JS SDK present but not analyzed for this dimension |
| Analyzed | 2026-08-24 |

> **Path convention:** all file citations below are relative to the source root `studies/agent-harness-study/sources/langgraph/`.

## Summary

LangGraph dispatches validated tool calls as tasks inside a Bulk Synchronous Parallel (BSP) "Pregel" engine. There is no dedicated tool dispatcher; tools are just node functions, and scheduling is the union of two mechanisms: (1) **PULL tasks** — nodes triggered by channel updates at each superstep boundary, planned by `prepare_next_tasks` (`libs/langgraph/langgraph/pregel/_algo.py:349-513`); and (2) **PUSH tasks** — dynamic fan-out via the `Send` API (`libs/langgraph/langgraph/types.py:704-792`), which lands packets in an internal `__pregel_tasks` Topic channel (`libs/langgraph/langgraph/_internal/_constants.py:20`) and is materialized into executable tasks in the *next* superstep (`libs/langgraph/langgraph/pregel/_algo.py:961-997`).

The prebuilt agent wires tool dispatch through that Send path: after a model turn with tool calls, `should_continue` returns one `Send("tools", ToolCallWithContext(...))` per tool call (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`), so every tool call becomes its own checkpointed task. Inside a single `ToolNode` execution, remaining parallelism is handled locally: sync calls run on a config-derived thread pool via `executor.map` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:821-824`), async calls via `asyncio.gather` (`tool_node.py:858`). Task execution itself is delegated by `PregelRunner.tick` to a thread pool (`BackgroundExecutor.submit`, `libs/langgraph/langgraph/pregel/_executor.py:54-75`) or to asyncio tasks with an optional `max_concurrency` semaphore (`_executor.py:135-140, 153-154`).

Ordering is deterministic at plan/commit level but concurrent at execution level: task IDs are content-addressed hashes of (checkpoint id, namespace, step, node, path) (`_algo.py:550, 616-623, 990-997`), candidate nodes are sorted before planning (`_algo.py:481-482`), and writes are applied sorted by task path (`apply_writes`, `_algo.py:256`). Dispatch is observable through debug task events (`map_debug_tasks`, emitted in `accept_push` and `tick`), LangSmith callback runs per task, a tool-specific stream protocol (`StreamToolCallHandler` emitting `tool-started` / `tool-output-delta` / `tool-finished` / `tool-error` keyed by `tool_call_id`), and a timed-attempt observer contract used by LangGraph Server.

## Rating

**8 / 10** — Clear, explicit model (BSP supersteps with documented semantics in code comments, `libs/langgraph/langgraph/pregel/main.py:2959-2963`), richly tested (`tests/test_pregel.py` is ~9.7k lines; dedicated concurrency tests such as `test_parallel_node_execution` at `tests/test_pregel.py:5258` and `test_max_concurrency` at `tests/test_pregel_async.py:3492`), and strong operational safeguards: retries with backoff/jitter, run/idle timeout watchdogs, sibling cancellation on failure, error-handler node routing, and write caching. It falls short of 9–10 because the failure/cancellation machinery carries fragile global state (`SKIP_RERAISE_SET` weakset, `libs/langgraph/langgraph/pregel/_runner.py:70-72`), the sync path cannot truly cancel running threads (only "not yet started" tasks, `_executor.py:59`), and several behaviors (next-tick ordering, panic semantics) are encoded implicitly across `_runner.py` rather than behind a named policy interface.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Superstep scheduler | BSP loop comment + `while loop.tick()` driving `runner.tick(...)` with `schedule_task=loop.accept_push` | `libs/langgraph/langgraph/pregel/main.py:2959-2984` |
| Task planner (PULL+PUSH union) | `prepare_next_tasks` docstring: "union of all PUSH tasks (Sends) and PULL tasks" | `libs/langgraph/langgraph/pregel/_algo.py:411-436` |
| PUSH consumption from TASKS channel | reads `channels.get(TASKS)` Topic, prepares one task per send index | `libs/langgraph/langgraph/pregel/_algo.py:441-466` |
| Deterministic node ordering | `sorted(triggered_nodes)` for candidate planning | `libs/langgraph/langgraph/pregel/_algo.py:475-486` |
| Deterministic task IDs | `task_id_func = _xxhash_str if checkpoint["v"] > 1 else _uuid5_str`; IDs from checkpoint id + ns + step + node + PULL/PUSH + triggers/idx | `libs/langgraph/langgraph/pregel/_algo.py:550, 613-623, 985-997` |
| Send API (dynamic fan-out packet) | `Send(node, arg, timeout)` with hash/eq; per-send `TimeoutPolicy` | `libs/langgraph/langgraph/types.py:704-792` |
| Sends execute next superstep | comment "SEND tasks, executed in superstep n+1" | `libs/langgraph/langgraph/pregel/_algo.py:961-967` |
| Agent → tool dispatch via Send | `should_continue` returns `[Send("tools", ToolCallWithContext(...)) for call in last_message.tool_calls]` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859` |
| ToolCallWithContext payload | state snapshot attached to each dispatched tool call for HITL resume | `libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306` |
| Executor (sync worker pool) | `BackgroundExecutor` wraps `get_executor_for_config(config)`; submit copies contextvars; flags `__cancel_on_exit__` / `__reraise_on_exit__` / `__next_tick__` | `libs/langgraph/langgraph/pregel/_executor.py:40-75` |
| Executor (async + concurrency cap) | `AsyncBackgroundExecutor` creates semaphore from `config["max_concurrency"]`; gates coroutines via `gated()` | `libs/langgraph/langgraph/pregel/_executor.py:131-140, 152-154, 214-217` |
| Runner tick (fan-out submission) | fast path for single task; else `for t in tasks: fut = self.submit()(run_with_retry, t, ...)`; waits FIRST_COMPLETED | `libs/langgraph/langgraph/pregel/_runner.py:200-289` |
| Push scheduling mid-tick (functional API) | `_call` schedules child task via `schedule_task` + `submit()`, dedupes retried parents, returns chained future | `libs/langgraph/langgraph/pregel/_runner.py:700-786` |
| Next-tick ordering guarantee | `__next_tick__=True` "starting a new task in the next tick ensures updates from this tick are committed/streamed first" | `libs/langgraph/langgraph/pregel/_runner.py:774-777` |
| In-node tool batching (sync) | `get_config_list(config, len(tool_calls))` then `executor.map(self._run_one, tool_calls, ...)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:800-826` |
| In-node tool batching (async) | builds coroutine list then `outputs = await asyncio.gather(*coros)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:855-860` |
| Command aggregation from batched tools | parent `Command(graph=PARENT, goto=[Send...])` merged across outputs | `libs/prebuilt/langgraph/prebuilt/tool_node.py:894-920` |
| Commit-on-completion | `PregelRunner.commit` persists task writes/interrupts/errors via `put_writes` | `libs/langgraph/langgraph/pregel/_runner.py:574-613` |
| Failure isolation | `_should_stop_others` cancels siblings when a task fails (interrupts exempt); `_panic_or_proceed` re-raises/cancels inflight | `libs/langgraph/langgraph/pregel/_runner.py:616-634, 650-697` |
| Step-level timeout | `step_timeout` param passed to `runner.tick(timeout=...)`; timed-out inflight tasks cancelled, TimeoutError raised | `libs/langgraph/langgraph/pregel/main.py:727, 2967-2969`; `_runner.py:280-289, 691-697` |
| Per-task timeout watchdogs | `TimeoutPolicy(run_timeout, idle_timeout, refresh_on)`; watchdog races bg task; idle progress tracked via guarded send/stream/callback wrappers | `libs/langgraph/langgraph/pregel/_retry.py` (`_resolve_timeout`, `_TimedAttemptScope`, `_arun_with_timeout`); `types.py:452` |
| Retry policy | exponential backoff + jitter, per-policy `retry_on` matching, attempt counter surfaced as `node_attempt` | `libs/langgraph/langgraph/pregel/_retry.py` (`run_with_retry` / `arun_with_retry`, `_should_retry_on`); `types.py:418` |
| Error-handler routing | failed task can spawn handler task (`schedule_error_handler` → `prepare_node_error_handler_task`) instead of panicking | `libs/langgraph/langgraph/pregel/_runner.py:222-233, 297-323`; `_loop.py:1572-1607`; `_algo.py:1110` |
| Write cache short-circuit | cached task writes matched before execution; `output_writes(..., cached=True)` | `libs/langgraph/langgraph/pregel/_loop.py:1549-1570`; cache key computed in `_algo.py:1019-1034` |
| Dispatch trace (debug events) | `self._emit("tasks", map_debug_tasks, ...)` on tick and push acceptance | `libs/langgraph/langgraph/pregel/_loop.py:574-580, 673-674`; `libs/langgraph/langgraph/pregel/debug.py:41` |
| Dispatch trace (tracing callbacks) | child run manager per task: `manager.get_child(f"graph:step:{step}")`; RunnableCallable wraps invoke in `on_chain_start/end/error` unless `trace=False` | `libs/langgraph/langgraph/pregel/_algo.py:1065-1070`; `libs/langgraph/langgraph/_internal/_runnable.py:421-450`; ToolNode opts out: `libs/prebuilt/langgraph/prebuilt/tool_node.py:772` |
| Tool-call stream observability | `StreamToolCallHandler` emits `tool-started` / `tool-output-delta` / `tool-finished` / `tool-error` keyed by `tool_call_id`; `run_inline = True` keeps ordering deterministic | `libs/langgraph/langgraph/pregel/_tools.py:35-53, 121-201` |
| Timed-attempt observer (server contract) | `_AttemptContext`/`_AttemptEvent` start/progress/finish events under `CONFIG_KEY_TIMED_ATTEMPT_OBSERVER` | `libs/langgraph/langgraph/pregel/_retry.py` (`_start_timed_attempt`, `_finish_timed_attempt`, `_dispatch_observer`) |
| Concurrency cap test | `test_max_concurrency`: 100 Sends capped at 10 in flight with `{"max_concurrency": 10}` | `libs/langgraph/tests/test_pregel_async.py:3492-3551` |
| Parallel execution test | `test_parallel_node_execution`: wall time < sum of node durations | `libs/langgraph/tests/test_pregel.py:5258-5288` |
| Deterministic send result order | `test_concurrent_emit_sends`: output list ordered `"2\|1","2\|2","2\|3","2\|4"` despite concurrent sends | `libs/langgraph/tests/test_pregel.py:1173-1217` |

## Answers to Dimension Questions

**1. How does a tool call start?**
A model message containing `tool_calls` is inspected by the agent's conditional edge. In `create_agent` v2, `should_continue` converts each call into `Send("tools", ToolCallWithContext(__type="tool_call_with_context", tool_call=call, state=state))` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:843-859`), snapshotting graph state into the payload so interrupted calls can resume later (`libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`). Each Send is written to the `__pregel_tasks` channel during `apply_writes`; at the next superstep `prepare_next_tasks` consumes it (`_algo.py:441-466`) and `prepare_push_task_send` builds a `PregelExecutableTask` targeting the `"tools"` node with a deterministic ID (`_algo.py:961-1107`). The loop's `accept_push` handles the analogous case where a running task spawns a child mid-tick (`_loop.py:550-587`).

**2. Is tool execution inline or queued?**
Both layers exist. Graph tasks (including the `tools` node) are queued onto a worker pool: `PregelRunner.tick` submits each task through the `Submit` protocol backed by `BackgroundExecutor` (thread pool, `_executor.py:40-75`) or `AsyncBackgroundExecutor` (asyncio tasks on the current loop, optionally gated by a `max_concurrency` semaphore, `_executor.py:131-169`). Within one ToolNode invocation, individual tool calls are executed concurrently — `executor.map` over a per-call config list (sync, `tool_node.py:819-824`) or `asyncio.gather` (async, `tool_node.py:855-860`). A deferred-start option (`__next_tick__`) exists specifically to preserve commit/stream ordering between parent and spawned tasks (`_runner.py:774-777`).

**3. Are tool calls ordered?**
Planning and commits are deterministic; execution is not. Tasks are planned in sorted node order (`_algo.py:481-486`), task IDs are stable hashes (`_algo.py:550, 990-997`), and write application sorts tasks by path (`apply_writes`, `_algo.py:253-256`). Tests assert deterministic final output ordering even when many Sends run concurrently (`tests/test_pregel.py:1207-1217`). However, completion order within a superstep is nondeterministic (`concurrent.futures.wait(FIRST_COMPLETED)` loop, `_runner.py:282-289`), which is why streaming uses arrival-order interleaving semantics (see `tests/test_interleave_arrival_order.py:1-88`). Send-indexed ordering is preserved for planning because each pending send is enumerated by index (`_algo.py:443-466`).

**4. Can tools be batched?**
Yes, twice over. All tool calls in one AIMessage are batched into a single ToolNode execution and fanned out internally (`tool_node.py:799-860`), with results combined and any parent-scoped `Command(goto=[Send...])` merged into a single parent command (`_combine_tool_outputs`, `tool_node.py:862-920`). At the graph layer, the Send API batches arbitrary fan-out (e.g., 100 parallel tasks in `tests/test_pregel_async.py:3514-3541`), and `max_concurrency` bounds the batch width. There is no cross-message tool-call coalescing (no queue that merges calls from separate turns).

**5. Is dispatch observable?**
Yes, at four levels. (a) Debug events: every planned or pushed task emits a `tasks` debug payload (`_loop.py:574-580, 673-674`). (b) Tracing: each task gets a child run manager under `graph:step:{n}` (`_algo.py:1069`), and `RunnableCallable.invoke` wraps execution in `on_chain_start/end/error` (`_internal/_runnable.py:421-450`) — though ToolNode sets `trace=False` to avoid a redundant wrapper span around the whole node (`tool_node.py:772`). (c) Tool-specific streams: `StreamToolCallHandler` emits lifecycle events keyed by `tool_call_id`, deliberately `run_inline = True` "keeps event ordering deterministic" (`pregel/_tools.py:35-53, 158-201`). (d) Server observer: timed attempts publish start/progress/finish events with timeouts and error metadata (`_retry.py`, `_AttemptContext`/`_AttemptEvent`). Retry activity is also logged (`_retry.py`, `logger.info(f"Retrying task {task.name} ...")`).

## Architectural Decisions

1. **Tools are nodes, not a special dispatch target.** The engine has no tool-specific scheduler; `create_agent` lowers tool calls into `Send` packets to a regular node (`chat_agent_executor.py:849-859`). This buys checkpointing, interrupts, retries, caching, and error handlers for free from the generic task machinery, at the cost of one extra superstep latency per model→tool transition (sends always execute in step n+1, `_algo.py:961-962`).

2. **BSP supersteps instead of a continuous work queue.** Channel updates from step N are invisible until step N+1 (`main.py:2959-2963`). This gives deterministic replay/checkpoint semantics — the foundation of durable execution — but forbids same-step pipelining of dependent work.

3. **Executor abstraction via a `Submit` Protocol.** Sync runs use a thread pool from langchain-core's `get_executor_for_config` (`_executor.py:50`); async runs reuse the event loop with optional semaphore gating (`_executor.py:135-140`). The runner only depends on the protocol (`_runner.py:27-37, 143`), keeping sync/async paths structurally parallel (`tick` vs `atick`).

4. **Content-addressed task identity.** Task IDs derive from checkpoint id + namespace + step + node + path (`_algo.py:616-623, 990-997`), so replays and resumed runs map onto the same IDs — required for pending-write reconciliation and interrupt resumption.

5. **Commit-through-callback design.** Writes are persisted as futures complete (`FuturesDict.on_done` → `commit`, `_runner.py:104-132, 574-613`) rather than at barrier end, enabling partial-progress durability and streamed output while preserving per-task atomicity.

## Notable Patterns

- **Push scheduling closure chain:** `_call`/`_acall` inject a `CONFIG_KEY_CALL` callable into every task's config; a task that wants to spawn work calls it with a `Call(func, input)` descriptor, which flows through `schedule_task` (= `loop.accept_push`) and comes back as a future (`_runner.py:211-218, 722-782`). Deduplication guards against double-scheduling on retry (`_runner.py:734-744`: "if the parent task was retried, the next task might already be running").
- **Guarded-config timeout scope:** `_TimedAttemptScope.wrap_config` swaps send/stream/call/runtime functions for guarded versions so any observable side effect doubles as a liveness heartbeat, with `refresh_on="auto"` vs `"heartbeat"` strictness (`_retry.py`).
- **Cache-before-execute:** `match_cached_writes` resolves cache hits into task writes before the runner starts them, and hits are streamed as `cached=True` outputs (`_loop.py:1549-1562`; consumed at `main.py:2965-2966`).
- **Weakref-heavy lifecycle management:** runners, executors, and scratchpads reference tasks/futures via `weakref.WeakMethod`/`WeakSet` (`_runner.py:143, 190-191, 70-72`) to avoid cycles between long-lived loops and per-run objects.

## Tradeoffs

- **Determinism vs latency:** the n+1 superstep rule makes Send-dispatched tools pay a barrier hop; interactive agents accept this for replayability.
- **Thread-pool cancellation gap:** sync futures cannot be force-cancelled; `__cancel_on_exit__` "can cancel only if not started" (`_executor.py:59`), so a hung sync tool blocks `concurrent.futures.wait(pending)` at exit (`_executor.py:105-107`). Async tasks get true cancellation (`_runner.py:552-554`).
- **Global mutable state for reraise suppression:** `SKIP_RERAISE_SET` is a module-level weakset consulted across functions (`_runner.py:70-72, 303, 686, 781`); it avoids duplicate exception surfacing but is hard to reason about and process-global.
- **Two parallel implementations:** `tick`/`atick` (~200 lines each) and `_func`/`_afunc` duplicate logic that must be kept behaviorally aligned (e.g., error-handler routing appears four times: `_runner.py:222-254, 297-323, 414-445, 496-532`).
- **Trace=False on ToolNode:** skips a span for the node itself (`tool_node.py:772`); per-tool spans still occur via langchain-core, but graph-level views lose the aggregate node run.

## Failure Modes / Edge Cases

- **Fail-fast sibling cancellation:** a non-interrupt exception stops and cancels other tasks in the superstep and panics (`_should_stop_others` `_runner.py:616-634`; `_panic_or_proceed` `_runner.py:650-697`), with exceptions already routed to an error-handler node excluded from the panic set (`handled_exception_ids`, `_runner.py:166-169`).
- **User-raised `CancelledError` ambiguity:** a node raising its own CancelledError is converted to `NodeCancelledError` using `asyncio.Task.cancelling() == 0` as the discriminator (LSD-1507 fix, `_retry.py`, `_is_user_raised_cancelled`), so the run doesn't silently report success.
- **Timeout accounting:** step timeout raises `TimeoutError("Timed out")` after cancelling inflight futures (`_runner.py:691-697`); per-node `TimeoutPolicy` distinguishes run vs idle timeouts and clears partial writes on breach (`_retry.py`, `_arun_with_timeout`: `task.writes.clear(); bg.cancel()`).
- **Retry idempotency:** retries clear prior writes (`task.writes.clear()` before each attempt, `_retry.py`) and dedupe already-running child tasks on reschedule (`_runner.py:734-744`).
- **Invalid Send targets ignored:** unknown node names or non-Send packets are dropped with a warning rather than failing the run (`_algo.py:971-979`) — lenient, but a typo'd node name fails silently at dispatch time.
- **Recursion bound as scheduling backstop:** the loop stops with `GraphRecursionError` when `step > stop` (`main.py:3002-3011`; check in `_loop.py:606-609`).

## Future Considerations

- Consolidate the sync/async runner duplication behind one parameterized implementation to reduce drift risk in error-handler and panic logic.
- Replace the global `SKIP_RERAISE_SET` with per-run state (it already threads `handled_futures` explicitly; the weakset could follow).
- Expose a public scheduling-policy hook (batch width, ordering preference, per-Send priority) instead of relying on `max_concurrency` alone.
- Surface dispatch decisions ("why did this task start now") in the debug stream payload — currently triggers/triggers-sorting exist (`_algo.py:613`, `_triggers`) but the `tasks` debug event would need richer cause attribution.

## Questions / Gaps

- No evidence found for remote/queued dispatch (external worker pools or server-side queues) inside this repository; the distributed mode is referenced only indirectly (`_ensure_execution_info` notes LangGraph Platform prepares/deserializes tasks outside `_algo.py`, `_retry.py`). Search boundary: grep for executor/queue/dispatch across `libs/langgraph` and `libs/prebuilt`; server-side code lives outside this monorepo.
- No evidence found for priority or fairness policies among tasks within a superstep; scheduling order beyond sorted planning is left to the executor's FIFO/thread-pool internals (`_executor.py:71`).
- Cross-checking JS behavior (`libs/sdk-js`) was out of scope for this dimension; the JS SDK is an API client, and the runtime counterpart lives in langgraphjs, a separate repo.

---

Generated by `Dimension 07.01: Tool Scheduling and Dispatch` against `langgraph`.
