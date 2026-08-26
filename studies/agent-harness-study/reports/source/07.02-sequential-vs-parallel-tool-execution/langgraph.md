# Source Analysis: langgraph

## Dimension 07.02 — Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph` core, `libs/prebuilt` agent/tooling) + JS SDK (`libs/sdk-js`, not analyzed here) |
| Analyzed | 2026-08-26 |

## Summary

LangGraph treats parallel tool execution as a first-class, two-layer concern.

**Layer 1 (inside a node):** `ToolNode` executes every tool call attached to the latest `AIMessage` concurrently. The sync path fans out through a config-derived thread-pool executor (`executor.map`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:821-824`); the async path uses `asyncio.gather` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:855-858`). Results are recombined in input order, so output ordering is deterministic regardless of completion order (`_combine_tool_outputs`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:862-887`).

**Layer 2 (across the graph):** `create_react_agent` (v2, the default) distributes each tool call to its own graph task via the `Send` API (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`), turning N tool calls into N parallel Pregel tasks. The Pregel runtime is a Bulk Synchronous Parallel (BSP) engine: tasks of a superstep run concurrently against an immutable channel snapshot, and writes are merged only at the step boundary (`libs/langgraph/langgraph/pregel/main.py:2959-2972`).

Concurrency is bounded by the `max_concurrency` config key, enforced as an `asyncio.Semaphore` around async task submission (`libs/langgraph/langgraph/pregel/_executor.py:135-140,214-217`) and proven by a test that caps 100 concurrent `Send`s at 10 (`libs/langgraph/tests/test_pregel_async.py:3492-3541`). Conflict detection is channel-based, not resource-based: writing two values per superstep to a reducer-less channel raises `InvalidUpdateError` ("Can receive only one value per step", `libs/langgraph/langgraph/channels/last_value.py:56-64`), and duplicate `Overwrite` bypasses are rejected the same way (`libs/langgraph/langgraph/channels/binop.py:135-138`). There is **no side-effect declaration system and no user-facing resource locking**; safety instead comes from BSP isolation, declarative reducers, deterministic write application ordered by task path (`libs/langgraph/langgraph/pregel/_algo.py:253-256`), and per-tool error-to-`ToolMessage` isolation inside `ToolNode`.

## Rating

**8 / 10.**

Rationale against the rubric:

- **Clear model with explicit interfaces:** parallelism is expressible at both levels with public primitives (`Send`, `Command(goto=...)`, reducers via `Annotated`, `max_concurrency`), documented in docstrings (`libs/langgraph/langgraph/types.py:704-749`; `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:456-465`).
- **Tests:** concurrency timing test (`libs/langgraph/tests/test_pregel.py:5258-5288`), max-concurrency cap tests (`libs/langgraph/tests/test_pregel_async.py:3492-3616`), parallel-overwrite conflict tests including the error path (`libs/langgraph/tests/test_pregel.py:9294-9368`), parallel interrupts across fan-out (`libs/langgraph/tests/test_pregel.py:7577-7636`), and ordered multi-tool-call results in `ToolNode` (`libs/prebuilt/tests/test_tool_node.py:237-250`).
- **Operational safeguards:** failure cancels sibling tasks deterministically (`_should_stop_others`, `libs/langgraph/langgraph/pregel/_runner.py:616-634`), errors are checkpointed (`commit`, `_runner.py:595-603`), interrupts are exempt from "failure" cancellation (`_runner.py:622-632`), and per-tool errors degrade into `status="error"` `ToolMessage`s rather than killing the batch.
- **Why not 9–10:** tools cannot declare side effects or read/write sets, so "which tools are safe to parallelize" is entirely the developer's burden; intra-node `asyncio.gather` is unbounded by `max_concurrency` (only inter-task Send fan-out is throttled); and sync-side concurrency limiting is delegated to langchain-core's `get_executor_for_config` behavior rather than enforced in this repo. Corruption risk is structurally low thanks to BSP isolation, but the framework does not *observe* semantic write conflicts between tools that share external resources.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Intra-node parallel executor (sync) | `ToolNode._func` maps `_run_one` over all parsed tool calls using `get_executor_for_config(config)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:820-824` |
| Intra-node parallel executor (async) | `ToolNode._afunc` builds one coroutine per call and awaits `asyncio.gather(*coros)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:855-858` |
| Deterministic result combination | `_combine_tool_outputs` flattens outputs preserving order; non-Command outputs wrapped into `{messages_key: [...]}` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:862-887` |
| Per-call error isolation in ToolNode | Each call's exception becomes `ToolMessage(status="error")`; `GraphBubbleUp` always re-raised | `libs/prebuilt/langgraph/prebuilt/tool_node.py:982-1012` (sync), `1129-1159` (async) |
| Default error policy | `_default_handle_tool_errors` returns message for invocation errors, re-raises others | `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391` |
| Send API primitive | `Send(node, arg)` pushes a custom-state task to a node; map-reduce docstring example | `libs/langgraph/langgraph/types.py:704-792` |
| Agent routes each tool call as its own task (v2) | `should_continue` returns `[Send("tools", ToolCallWithContext(...)) for call in last_message.tool_calls]` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:843-859` |
| Version semantics documented | v1: all calls executed in parallel inside one ToolNode invocation; v2: calls distributed across Send instances | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:456-465` |
| Send payload context type | `ToolCallWithContext` docstring states Send API distributes tool calls in parallel for HITL workflows | `libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306` |
| State hydration for Send-dispatched calls | `_extract_state` reads channels via `CONFIG_KEY_READ` when state isn't inlined in payload | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1281-1313` |
| Graph-level concurrent runner | `PregelRunner` "executing a set of Pregel tasks concurrently, committing their writes"; single-task fast path skips pool | `libs/langgraph/langgraph/pregel/_runner.py:135-138,200-204` |
| BSP superstep loop | Comment: channels immutable during a step; updates applied only at transitions; `runner.tick` over `loop.tasks` | `libs/langgraph/langgraph/pregel/main.py:2959-2984` |
| Task preparation from Send fan-out | PUSH tasks enumerated from `TASKS` Topic channel; candidate nodes sorted for deterministic order | `libs/langgraph/langgraph/pregel/_algo.py:442-466,475-482` |
| Background executors | Sync thread-pool `BackgroundExecutor`; async `AsyncBackgroundExecutor` on running loop with context copy | `libs/langgraph/langgraph/pregel/_executor.py:40-120,122-211` |
| Concurrency limit (async semaphore) | `config["max_concurrency"]` → `asyncio.Semaphore`; `gated()` wraps coroutines | `libs/langgraph/langgraph/pregel/_executor.py:131-140,214-217` |
| Executor wiring into loop | `self.submit = self.stack.enter_context(BackgroundExecutor(self.config))` | `libs/langgraph/langgraph/pregel/_loop.py:1691` |
| Public config surface | `max_concurrency`: "max number of concurrent steps ... also applies to parallelized steps" | `libs/langgraph/langgraph/_internal/_config.py:199-228` |
| Write-conflict detection (no reducer) | `LastValue.update` raises `InvalidUpdateError` if >1 value per step | `libs/langgraph/langgraph/channels/last_value.py:56-64` |
| Write-conflict detection (Overwrite abuse) | "Can receive only one Overwrite value per super-step." raised in BinOp and Delta channels | `libs/langgraph/langgraph/channels/binop.py:135-138`, `libs/langgraph/langgraph/channels/delta.py:168-171` |
| Reducer-based merging | `Annotated[list[str], operator.add]` state keys merge parallel writes; `add_messages` merges message updates | `libs/langgraph/tests/test_pregel.py:5261-5263`; `libs/langgraph/langgraph/graph/message.py:61` |
| Deterministic write application | `apply_writes` sorts tasks on path "to ensure deterministic order for update application" | `libs/langgraph/langgraph/pregel/_algo.py:253-256` |
| PubSub task channel | `Topic` channel collects per-step `Send`s, emptied each step unless `accumulate=True` | `libs/langgraph/langgraph/channels/topic.py:23-31,77-91` |
| Failure propagation at graph level | Any failed future stops/cancels siblings; `GraphBubbleUp` (interrupts) excluded | `libs/langgraph/langgraph/pregel/_runner.py:616-634` |
| Panic/proceed & interrupt aggregation | `_panic_or_proceed` cancels inflight, re-raises first error, aggregates multiple `GraphInterrupt`s | `libs/langgraph/langgraph/pregel/_runner.py:650-697` |
| Error persistence | `commit` appends `(ERROR, exc)` to task writes and checkpoints them per task id | `libs/langgraph/langgraph/pregel/_runner.py:574-613` |
| Functional-API parallelism | `@task` "Calling the function produces a future. This makes it easy to parallelize tasks."; retry/timeout policies per task | `libs/langgraph/langgraph/func/__init__.py:150-170` |
| Timeout / retry policies | `RetryPolicy` and `TimeoutPolicy`; `Send(..., timeout=...)` coerces to `TimeoutPolicy` | `libs/langgraph/langgraph/types.py:418,452,720-776` |
| Side-effect caveat (re-execution) | `interrupt` resumes by re-executing the node from the start — implicit requirement that node/tool logic be safe to re-run | `libs/langgraph/langgraph/types.py:862-871` |
| Test: parallel nodes actually overlap | Two sleeping START-fanned nodes finish in < sum of sleeps | `libs/langgraph/tests/test_pregel.py:5258-5288` |
| Test: max_concurrency caps fan-out | 100 Sends: unbounded hits 100 concurrent; `{"max_concurrency": 10}` caps at exactly 10 | `libs/langgraph/tests/test_pregel_async.py:3508-3542` |
| Test: max_concurrency via Command goto | Same cap verified for `Command(update=..., goto=[Send...])` fan-out | `libs/langgraph/tests/test_pregel_async.py:3553-3616` |
| Test: single Overwrite OK, double Overwrite fails | `test_overwrite_parallel` vs `test_overwrite_parallel_error` expecting `InvalidUpdateError` | `libs/langgraph/tests/test_pregel.py:9294-9330,9334-9368` |
| Test: ordered multi-tool-call output | Two tool calls produce `ToolMessage`s asserted in exact input order; unknown tool yields inline error without breaking sibling | `libs/prebuilt/tests/test_tool_node.py:237-265` |
| Test: Send-dispatched ToolNode hydration | Simulated `ToolCallWithContext` payloads and `Send('tools', [tool_call])` state hydration | `libs/prebuilt/tests/test_on_tool_call.py:1231-1360` |
| Test: parallel interrupts across fan-out | Parent fans out child graphs with `Send`; interrupts collected and resumed per branch | `libs/langgraph/tests/test_pregel.py:7577-7636` |

## Answers to Dimension Questions

1. **Can multiple tools run in one turn?**
   Yes, at two layers. Within a single `ToolNode` invocation, all tool calls on the last `AIMessage` run concurrently (`executor.map` / `asyncio.gather`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:820-858`). With `create_react_agent` v2, each tool call additionally becomes its own Pregel task via `Send`, so they execute as independent superstep actors (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`). The functional API offers a third route: `@task` futures scheduled inside an entrypoint (`libs/langgraph/langgraph/func/__init__.py:156-158`).

2. **Which tools are safe to parallelize?**
   Undetermined by the framework — there is no side-effect/read-write metadata on tools anywhere in `libs/prebuilt` or `libs/langgraph` (searched for `side.effect`, lock/semaphore annotations; only internal infrastructure locks exist, e.g., `libs/langgraph/langgraph/pregel/_retry.py:162`, `libs/langgraph/langgraph/pregel/_runner.py:97`). Safety is delegated to (a) developers choosing pure tools, and (b) state-schema design: parallel writes only compose cleanly when target channels declare reducers (`Annotated[list, operator.add]`, `libs/langgraph/tests/test_pregel.py:5261-5263`; `add_messages`, `libs/langgraph/langgraph/graph/message.py:61`). A related implicit rule: any node/tool containing `interrupt()` will be re-executed from scratch on resume, so side effects must be idempotent (`libs/langgraph/langgraph/types.py:862-864`).

3. **Are write tools serialized?**
   No serialization mechanism exists for external resources (no file/DB locks). What exists is *state-write* arbitration: within a superstep, tasks never observe each other's writes (channels immutable until step end, `libs/langgraph/langgraph/pregel/main.py:2959-2963`), and at merge time a reducer-less channel rejects concurrent updates outright (`InvalidUpdateError`, `libs/langgraph/langgraph/channels/last_value.py:56-64`), while reducer-backed channels merge deterministically after sorting writers by task path (`libs/langgraph/langgraph/pregel/_algo.py:253-256,315-323`). So conflicting in-graph writes either fail fast or merge by declared policy; conflicting *external* writes are the developer's problem.

4. **How are failures aggregated?**
   Three tiers. (i) Inside `ToolNode`, a handled failure converts to a `ToolMessage(status="error")` and siblings continue (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1002-1012`); `handle_tool_errors=False` propagates instead. (ii) At graph level, the first non-interrupt failure triggers sibling cancellation via `_should_stop_others` and re-raise via `_panic_or_proceed` (`libs/langgraph/langgraph/pregel/_runner.py:616-697`), while the failing task's `(ERROR, exc)` write is checkpointed before teardown (`libs/langgraph/langgraph/pregel/_runner.py:595-603`). (iii) Multiple concurrent `GraphInterrupt`s are not failures — they are aggregated into one combined `GraphInterrupt` (`_runner.py:683-690`), enabling parallel human-in-the-loop fan-out (tested: `libs/langgraph/tests/test_pregel.py:7577-7636`).

5. **Is result order deterministic?**
   Yes for final state, no for streaming emission. Final-state determinism comes from three mechanisms: `executor.map`/`asyncio.gather` preserve input order inside `ToolNode` (asserted in `libs/prebuilt/tests/test_tool_node.py:237-250`); `apply_writes` sorts tasks by path before applying updates (`libs/langgraph/langgraph/pregel/_algo.py:253-256`); and triggered-node candidates are sorted before task preparation (`_algo.py:481-482`). An integration test asserts exact ordering of 100 fanned-out task results (`libs/langgraph/tests/test_pregel_async.py:3530`). However, stream events are committed per-task as they complete (`PregelRunner.commit` invoked from done-callbacks, `_runner.py:574-613`), so token/event interleaving follows completion order even though channel application does not.

## Architectural Decisions

- **Bulk Synchronous Parallel (Pregel/BSP) execution model.** Nodes of a superstep run concurrently against an immutable channel snapshot; writes land only at step boundaries (`libs/langgraph/langgraph/pregel/main.py:2959-2972`). This makes parallelism safe-by-construction for shared graph state and removes the need for fine-grained locks on state.
- **Map-reduce as the parallelism primitive, not a scheduler.** Rather than a tool-level "parallelize these N calls" API, LangGraph generalizes to `Send` fan-out (`libs/langgraph/langgraph/types.py:704-749`); the agent's tool distribution is just one consumer of it (`chat_agent_executor.py:849-859`). Tool parallelism therefore inherits graph-level features: checkpointing, interrupts, retries (`RetryPolicy`, `types.py:418`), timeouts (`Send(timeout=...)`, `types.py:763-776`).
- **Two-tier default strategy for agents.** v1 keeps one `ToolNode` process handling all calls with an internal executor; v2 (default) prefers Send-per-call so each tool can be individually interrupted/resumed and state-hydrated (`chat_agent_executor.py:456-465`; `tool_node.py:286-306,1281-1313`).
- **Fail-fast across siblings, degrade-in-place within a tool batch.** Handled tool errors become error messages; unhandled ones panic the whole superstep (`_runner.py:616-697`). This splits "the model gave bad arguments" (recoverable) from "infrastructure broke" (abort).
- **Declarative conflict resolution via channel types.** `LastValue` = exclusive write, BinOp/list reducers = merge, `Topic` = pubsub accumulation (`channels/topic.py:23-31`). Conflicts are detected at merge time, not prevented statically.

## Notable Patterns

- **Order-preserving fan-in:** both `executor.map` (`tool_node.py:821-824`) and `asyncio.gather` (`tool_node.py:858`) return results indexed by input position, giving latency gains with positional determinism — then `_combine_tool_outputs` flattens nested lists while keeping order (`tool_node.py:862-887`).
- **Semaphore-gated coroutines:** the async executor wraps every submitted coroutine in `gated(semaphore, coro)` when `max_concurrency` is set (`_executor.py:152-154,214-217`) — a minimal, composable throttle rather than a bounded queue rewrite.
- **Single-task fast path:** `tick`/`atick` skip the futures machinery when exactly one task exists and no timeout/waiter applies (`_runner.py:201-204,392-393`) — sequential execution is just the degenerate parallel case.
- **Error-object identity tracking:** `_handled_exception_ids` prevents exceptions already routed to a node error handler from being re-raised or re-counted by stop checks (`_runner.py:166-174,627-632`).
- **Path-sorted commits:** deterministic merge achieved by sorting on `task_path_str(t.path[:3])` rather than wall-clock completion (`_algo.py:253-256`).

## Tradeoffs

- **Latency vs. corruption risk:** BSP gives corruption-free shared state under parallelism, but serializes visibility — a fast task's result cannot feed a sibling in the same superstep; it must wait a full round trip.
- **Generality vs. tool-level ergonomics:** because parallelism lives at the graph layer, a plain `ToolNode` used standalone gets `asyncio.gather` with **no** concurrency cap (`tool_node.py:855-858`); `max_concurrency` only throttles Send/task fan-out through `AsyncBackgroundExecutor`. Unbounded gather on I/O-heavy batches can exhaust connections.
- **Sync-path reliance on external dependency:** sync limiting depends on langchain-core's `get_executor_for_config` honoring `max_concurrency` (imported at `tool_node.py:71-75`, `_executor.py:17`); this repo contains no enforcement code for the sync case (verified by search — only the async semaphore exists in-repo).
- **Fail-fast vs. partial progress:** sibling cancellation on first error (`_runner.py:678-687`) avoids wasted work but discards completed siblings' outputs unless a checkpointer persists their committed writes; callers wanting best-effort batch results must wrap tools themselves via `wrap_tool_call` interceptors (`tool_node.py:1044-1067`).
- **Determinism scope:** deterministic channel state simplifies replay/checkpointing, but completion-order streaming means UI event order varies run-to-run even when final state is identical.

## Failure Modes / Edge Cases

- **Concurrent reducer-less writes abort the run:** `InvalidUpdateError` with guidance to use `Annotated` reducers (`channels/last_value.py:59-64`) — tested at `libs/langgraph/tests/test_pregel.py:780,1714`.
- **Double `Overwrite` in one superstep is ambiguous and rejected** ("Can receive only one Overwrite value per super-step.", `channels/binop.py:135-138`; test `test_pregel.py:9334-9368`).
- **Unhandled tool exception kills all sibling tool tasks:** with `handle_tool_errors=False`, `_execute_tool_sync` re-raises (`tool_node.py:1002-1003`), which escalates to superstep panic and sibling cancellation (`_runner.py:678-687`). Default handler mitigates this for argument errors only (`tool_node.py:383-391`).
- **Cancelled siblings leave ERROR pending-writes:** `commit` records `(ERROR, CancelledError)` for cancelled tasks so the superstep can finish coherently (`_runner.py:579-583`).
- **Interrupts inside parallel branches must aggregate:** handled explicitly — collected and re-raised as a single combined `GraphInterrupt` (`_runner.py:669-690`); regression-tested for parent/child graphs (`test_pregel.py:7577-7636`) and nested graphs (`test_pregel.py:3490`).
- **Re-execution hazard on resume:** nodes resume by re-running from the start (`types.py:862-864`), so a partially-completed side-effectful tool in a parallel branch can execute twice unless made idempotent — a documented behavioral contract, not an enforced guard.
- **Unknown tool names degrade, don't crash:** missing tools yield per-call error `ToolMessage`s alongside successful siblings' results (`tool_node.py:1268-1279`; ordered-output test `test_tool_node.py:248-265`).
- **Timeouts cancel inflight work:** `_panic_or_proceed` cancels remaining futures and raises a timeout error when the step deadline lapses (`_runner.py:691-697`; wired via `step_timeout` at `main.py:2969`).

## Future Considerations

- **Side-effect declarations:** a lightweight annotation (e.g., read/write set or purity marker on `BaseTool`) would let planners/routers serialize conflicting tools automatically instead of relying on schema discipline; nothing equivalent exists today (search boundary: `side.effect|sideeffect` across `libs/langgraph/langgraph` and `libs/prebuilt/langgraph` returned no tool-metadata hits).
- **Bound intra-node async fan-out:** applying the existing `gated()` semaphore pattern (`_executor.py:214-217`) to `ToolNode._afunc`'s `gather` would close the gap between task-level and call-level limits.
- **Unify sync limiting in-repo:** implementing sync semaphore logic beside the async one would remove the implicit dependency on langchain-core executor semantics.
- **Partial-result modes:** an opt-in "collect failures" mode at superstep level (beyond per-tool `handle_tool_errors`) would serve long fan-outs where losing 99 completed sends to 1 failure is costly.
- **Observability of merge conflicts:** surfacing near-miss reducer contention (e.g., many writers to one LastValue-adjacent key) as warnings would help users catch accidental serialization hazards before they become `InvalidUpdateError` aborts.

## Questions / Gaps

- **No evidence found** for tool-level resource locking or side-effect metadata anywhere in the selected source (searches: `Semaphore|threading.Lock|asyncio.Lock`, `side.effect|sideeffect` across `libs/langgraph` and `libs/prebuilt`; only internal infrastructures locks matched).
- **No evidence found** for a documented guarantee about streaming-event ordering relative to completion; the completion-order behavior is inferred from `PregelRunner.commit` being invoked from future done-callbacks (`_runner.py:574-613`), and docs in this checkout (`docs/llms.txt`) were not part of the analyzable source tree.
- The exact behavior of `get_executor_for_config` under `max_concurrency` (worker count derivation) lives in the external `langchain-core` dependency and could not be verified from this repository; only its import sites and usage are citable (`tool_node.py:71-75`, `_executor.py:50`).
- Whether v1 (`version == "v1"` returning `"tools"`, `chat_agent_executor.py:844-845`) remains recommended anywhere is unclear; the docstring describes both versions (`chat_agent_executor.py:456-465`) but no deprecation timeline was found in-code.

---

Generated by `Dimension 07.02: Sequential vs Parallel Tool Execution` against `langgraph`.
