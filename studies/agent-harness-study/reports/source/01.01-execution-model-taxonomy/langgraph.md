# Source Analysis: langgraph

## 01.01 Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint*`) |
| Analyzed | 2026-07-02 |

## Summary

LangGraph's execution model is an explicit **Pregel / Bulk Synchronous Parallel (BSP) superstep loop** layered with a **concurrent in-superstep task runner** and a **durable checkpointing state machine**. The top-level entry points (`Pregel.stream`, `Pregel.ainvoke`) enter a `while loop.tick(): ... runner.tick(...) ... loop.after_tick()` driver (`libs/langgraph/langgraph/pregel/main.py:2979-3003`). A "tick" picks the next set of `PULL`/`PUSH` tasks, runs them concurrently to completion on a thread pool (sync) or event loop (async), applies their writes to channels in a single barrier (`apply_writes`, `libs/langgraph/langgraph/pregel/_algo.py:232-345`), then commits a checkpoint. The "Plan / Execution / Update" vocabulary in the `Pregel` class docstring (`libs/langgraph/langgraph/pregel/main.py:455-476`) names these phases explicitly. Two layered models — a BSP "superstep loop" for inter-step advancement and a `concurrent.futures` / `asyncio`-based "task runner" for intra-step execution — are deliberately composed; both layers have explicit interfaces (`PregelLoop.tick`/`after_tick`, `PregelRunner.tick`/`atick`) and dedicated tests. The model is durable-aware: three `Durability` modes (`sync`, `async`, `exit`) control when a checkpoint is written relative to the next tick (`libs/langgraph/langgraph/pregel/main.py:757-836`, `libs/langgraph/langgraph/pregel/_loop.py:1116-1189`), and a `RunControl` lets callers cooperatively request drain between ticks (`libs/langgraph/langgraph/runtime.py:79-105`, `libs/langgraph/langgraph/pregel/_loop.py:650-652`).

## Rating

**9 / 10 — Mature, durable, observable, extensible, proven under failure or scale.**

The execution model is named (Pregel/BSP), centrally documented in the `Pregel` class docstring (`libs/langgraph/langgraph/pregel/main.py:449-548`), split into separately-testable units (`PregelLoop`, `PregelRunner`, `BackgroundExecutor`), exercised by extensive tests (`libs/langgraph/tests/test_pregel.py`, `test_pregel_async.py`, `test_large_cases*.py`, `test_runtime.py`, `test_time_travel*.py`), and instrumented with explicit status, debug, and stream-mode outputs. It is the documented model in the library README (`libs/langgraph/langgraph/README.md:36-43`, `sources/langgraph/README.md:80` — "Inspired by Pregel and Apache Beam"). It would lose points only for the surface area of conditional edges / branches / subgraphs producing layered state that requires careful reading, but the layering is intentional and named.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level runtime entrypoint | `Pregel.stream` / `Pregel.ainvoke` build a `SyncPregelLoop` / `AsyncPregelLoop` and drive `while loop.tick(): ... runner.tick(...) ... loop.after_tick()` | `libs/langgraph/langgraph/pregel/main.py:2914-3030` |
| Superstep driver (the loop) | `PregelLoop.tick()` prepares tasks for the next step; `after_tick()` applies writes, persists checkpoint, raises interrupts | `libs/langgraph/langgraph/pregel/_loop.py:592-674` |
| Superstep completion | `after_tick()` calls `apply_writes`, clears pending writes, calls `_put_checkpoint`, runs `interrupt_after` | `libs/langgraph/langgraph/pregel/_loop.py:676-714` |
| Recursion / step bound | `self.stop = self.step + self.config["recursion_limit"] + 1` in `__enter__`; `tick()` returns `False` when `step > stop` and sets `status = "out_of_steps"` | `libs/langgraph/langgraph/pregel/_loop.py:600-602`, `:1677` |
| Plan phase (task scheduler) | `prepare_next_tasks` consumes `TASKS` topic (PUSH), then walks `trigger_to_nodes` or all nodes to materialize PULL tasks | `libs/langgraph/langgraph/pregel/_algo.py:392-513` |
| Trigger index | `_trigger_to_nodes` precomputes `channel -> [node_names]` for `O(updated_channels)` selection | `libs/langgraph/langgraph/pregel/main.py:4190-4196` |
| Execution phase (task runner) | `PregelRunner.tick`/`atick` schedule a `FuturesDict` and `concurrent.futures.wait(... FIRST_COMPLETED)` / `asyncio.wait`, yielding control between events | `libs/langgraph/langgraph/pregel/_runner.py:176-358` |
| Sync background executor | `BackgroundExecutor` wraps a thread pool, copies contextvars, and re-raises on exit | `libs/langgraph/langgraph/pregel/_executor.py:40-119` |
| Async background executor | `AsyncBackgroundExecutor` uses asyncio tasks with optional semaphore (`max_concurrency`) | `libs/langgraph/langgraph/pregel/_executor.py:122-217` |
| Update phase | `apply_writes` updates channels, bumps `channel_versions`, and returns `updated_channels` set; tasks sorted by `task_path_str` for determinism | `libs/langgraph/langgraph/pregel/_algo.py:232-345` |
| Channel versioning (trigger detection) | `consume()` advances channel version; `apply_writes` calls it on read channels to drive trigger re-evaluation | `libs/langgraph/langgraph/channels/base.py:101-110`, `libs/langgraph/langgraph/pregel/_algo.py:284-292` |
| Channel types | `LastValue`, `Topic`, `BinaryOperatorAggregate`, `AnyValue`, `EphemeralValue`, `NamedBarrierValue`, `UntrackedValue`, `LastValueAfterFinish`, `NamedBarrierValueAfterFinish`, `DeltaChannel` | `libs/langgraph/langgraph/channels/last_value.py:20`, `:81`; `topic.py:23`; `binop.py`; `any_value.py:52`; `ephemeral_value.py:55`; `named_barrier_value.py:56`, `:84`; `untracked_value.py:15`; `delta.py:25` |
| Node abstraction | `PregelNode` declares `channels`, `triggers`, `writers`, `bound`, plus `error_handler_node` | `libs/langgraph/langgraph/pregel/_read.py:97-148` |
| Write side of node | `ChannelWrite` collects writes/Sends into `CONFIG_KEY_SEND` callable for the loop to drain at the end of the superstep | `libs/langgraph/langgraph/pregel/_write.py:46-126` |
| Send / PUSH | `Send` carries a target node + arg; deposited in `TASKS` topic channel, consumed by `prepare_next_tasks` next tick | `libs/langgraph/langgraph/types.py:664`; `libs/langgraph/langgraph/pregel/_algo.py:442-466` |
| PULL vs PUSH | `PULL = "__pregel_pull"`, `PUSH = "__pregel_push"` reserved channels distinguish edge-driven vs `Send`-driven tasks | `libs/langgraph/langgraph/_internal/_constants.py:83-86` |
| Status state machine | `Literal["input", "pending", "done", "draining", "interrupt_before", "interrupt_after", "out_of_steps"]` on `PregelLoop.status` | `libs/langgraph/langgraph/pregel/_loop.py:249-257` |
| Interrupt mechanism | `tick()` raises `GraphInterrupt` before execution if `interrupt_before` matches; `after_tick` raises after if `interrupt_after` matches | `libs/langgraph/langgraph/pregel/_loop.py:659-664`, `:708-712` |
| Command/resume input | `_first` maps a `Command` input into pending writes (incl. `RESUME`), replays interrupts appropriately | `libs/langgraph/langgraph/pregel/_loop.py:836-1019` |
| Cooperative drain | `tick()` returns `False` when `control.drain_requested` and sets `status = "draining"` | `libs/langgraph/langgraph/pregel/_loop.py:650-652` |
| `RunControl` API | `request_drain(reason)` flips a flag exposed on `Runtime`; tests verify mid-step drain terminates | `libs/langgraph/langgraph/runtime.py:79-105`; `libs/langgraph/tests/test_runtime.py:100-148` |
| Checkpoint write barrier | `_checkpointer_put_after_previous` waits on the prior checkpoint future, then submits the new one; `_delta_write_futs` drain enforces per-write durability ordering | `libs/langgraph/langgraph/pregel/_loop.py:1507-1525`, `:1759-1778`, `:199-211` |
| Durability modes | `Durability` is `"sync" \| "async" \| "exit"`, set on `stream`/`ainvoke`; `_put_checkpoint` branches on it | `libs/langgraph/langgraph/pregel/main.py:2979-3030`; `libs/langgraph/langgraph/pregel/_loop.py:1116-1189` |
| Exit-mode accumulator | `_exit_delta_writes` collects DeltaChannel writes per superstep; `_put_exit_delta_writes` flushes them as anchored writes at exit | `libs/langgraph/langgraph/pregel/_loop.py:1201-1293` |
| Error handler routing | `PregelRunner` calls `schedule_error_handler` / `aschedule_error_handler` to enqueue a handler task instead of bubbling the exception | `libs/langgraph/langgraph/pregel/_runner.py:230`, `:306-323` |
| On resume, skip original | `_resume_error_handlers_if_applicable` reads `ERROR_SOURCE_NODE` markers, marks original task writes so runner skips it, and prepares a handler task | `libs/langgraph/langgraph/pregel/_loop.py:739-804` |
| Retry / timeout | `RetryPolicy` lives on `PregelNode` and is applied by `run_with_retry` / `arun_with_retry` inside the runner | `libs/langgraph/langgraph/pregel/_retry.py:573` |
| Caching | `match_cached_writes` / `amatch_cached_writes` short-circuits a node when its `CacheKey` is present in the cache | `libs/langgraph/langgraph/pregel/_loop.py:1526-1547`, `:1780-1801` |
| Public protocol | `PregelProtocol` (Runnable subclass) exposes `invoke`, `stream`, `ainvoke`, `astream`, `get_state`, `update_state` etc. | `libs/langgraph/langgraph/pregel/protocol.py:25-269` |
| Higher-level API — Graph | `StateGraph.compile()` returns a `CompiledStateGraph` that is a `Pregel`, with `add_node`/`add_edge`/`add_conditional_edges` lowered into `PregelNode`s | `libs/langgraph/langgraph/graph/state.py:130`, `:1164-1363` |
| Higher-level API — Functional | `entrypoint` decorator constructs a single-node `Pregel` with `START` and `END` channels | `libs/langgraph/langgraph/func/__init__.py:262`, `:576-609` |
| Higher-level API — Agent | `create_react_agent` builds a `StateGraph` of `agent -> conditional -> tools -> agent` and compiles it | `libs/langgraph/langgraph/prebuilt/chat_agent_executor.py:789-990` |
| Remote graph | `RemoteGraph` implements `PregelProtocol` but runs the same loop over the server protocol | `libs/langgraph/langgraph/pregel/remote.py:118` |
| Algo tests | `tests/test_algo.py` tests `prepare_next_tasks` and `task_path_str` ordering | `libs/langgraph/tests/test_algo.py:6-67` |
| Loop tests | Recursion limit, out-of-steps, superstep ordering, concurrency, drain, interrupts — many of these in `tests/test_pregel.py` and `tests/test_pregel_async.py` | `libs/langgraph/tests/test_pregel.py:5349`, `:9025`, `:1159`; `tests/test_pregel_async.py:2158`, `:6594`, `:9516`; `tests/test_runtime.py:100` |
| Time-travel / replay | `_first` distinguishes `is_resuming` vs `is_time_traveling`; replays drop `INTERRUPT` writes; forks are explicit | `libs/langgraph/langgraph/pregel/_loop.py:836-959`; `libs/langgraph/tests/test_time_travel.py:226` |

## Answers to Dimension Questions

**1. What is the primary execution model?**

A Pregel/BSP superstep loop, with each step internally a concurrent task runner over a thread pool or asyncio event loop. The loop is `while loop.tick(): runner.tick(...); loop.after_tick()` in `libs/langgraph/langgraph/pregel/main.py:2979-3003` and `:3310-3380` (sync / async). The "tick" produces a `dict[str, PregelExecutableTask]`, the runner executes them concurrently until any fails or all finish, then `after_tick` calls `apply_writes` (the barrier), commits a checkpoint, and runs interrupt checks.

**2. Is it explicit or emergent?**

Explicit. The class `PregelLoop.tick` / `after_tick` (`libs/langgraph/langgraph/pregel/_loop.py:592-714`) and `PregelRunner.tick` / `atick` (`libs/langgraph/langgraph/pregel/_runner.py:176-528`) have explicit method names matching the Pregel paper's "Plan / Execute / Update" and BSP "superstep" terminology. The `Pregel` class docstring (`libs/langgraph/langgraph/pregel/main.py:449-548`) names the model. Even the file layout separates plan (`_algo.py:prepare_next_tasks`), execute (`_runner.py`), and update (`_algo.py:apply_writes`).

**3. Does the model match the product shape?**

Yes. LangGraph sells durable, long-running agents with cycles and human-in-the-loop checkpoints. BSP is the right substrate because:
- Each superstep is a globally consistent barrier that can be snapshotted atomically (`libs/langgraph/langgraph/pregel/_loop.py:1064-1196`).
- Cycles are first-class: any node can `Send` a future task and the same loop will pick it up on the next tick (`libs/langgraph/langgraph/_algo.py:442-466`).
- Concurrency is bounded per superstep but not per run, so a "fan-out / join" pattern (`Send(...)`) works (`libs/langgraph/langgraph/types.py:664`).
- Human-in-the-loop is implemented by raising `GraphInterrupt` inside the loop and serializing the per-task interrupt into checkpoint pending writes (`libs/langgraph/langgraph/pregel/_loop.py:659-664`).

**4. Is the model easy to explain to a new contributor?**

Yes — one sentence suffices: "Each superstep picks all triggered nodes (PULL) plus all `Send`-d queued tasks (PUSH), runs them concurrently to completion, applies their writes as a barrier, then saves a checkpoint." The naming convention (`tick`/`after_tick`, `prepare_next_tasks`, `apply_writes`, `put_checkpoint`, `interrupt_before/after`) is consistent throughout the file and docstrings. The `Pregel` docstring (`libs/langgraph/langgraph/pregel/main.py:455-476`) gives this explanation directly.

**5. Does the system mix models cleanly or accidentally?**

Cleanly, and by design. There are exactly two models:
1. **Superstep loop** (`PregelLoop`) — inter-step advancement, owned by the run loop.
2. **Concurrent task runner** (`PregelRunner`) — intra-step execution, owned by the runner.

A third, much smaller model is layered on top: the **state machine for `RunControl` drain requests** (`libs/langgraph/langgraph/runtime.py:79-105`), which is observed by `tick()` and short-circuits the loop (`libs/langgraph/langgraph/pregel/_loop.py:650-652`). And a fourth, **lifecycle callback state machine** (`GraphInterruptEvent` / `GraphResumeEvent`, `libs/langgraph/langgraph/callbacks.py:43-83`) is dispatched between ticks. Each is named, localized, and tested (`libs/langgraph/tests/test_runtime.py:100-148`).

## Architectural Decisions

- **Channels, not messages, between actors.** `BaseChannel.update(values) -> bool` is the only way tasks communicate (`libs/langgraph/langgraph/channels/base.py:90-99`). This makes BSP barriers explicit and lets a channel be marked as a "trigger" (`consume()`, `libs/langgraph/langgraph/channels/base.py:101-110`).
- **Versioned channels drive re-triggering.** `apply_writes` bumps `channel_versions` and the plan phase re-evaluates triggers via `versions_seen` (`libs/langgraph/langgraph/pregel/_algo.py:262-292`). A node fires only when one of its `triggers` changed since its `versions_seen[name][chan]`.
- **Pre-computed trigger index.** `_trigger_to_nodes(nodes)` builds a `dict[channel, list[node]]` (`libs/langgraph/langgraph/pregel/main.py:4190-4196`) so that, given the set of `updated_channels` from the last step, the next step's candidate set is selected in `O(updated_channels)` (`libs/langgraph/langgraph/pregel/_algo.py:475-512`).
- **Two parallel loops: sync and async.** `SyncPregelLoop` (`libs/langgraph/langgraph/pregel/_loop.py:1446-1695`) and `AsyncPregelLoop` (`:1698-`) share the same `PregelLoop` base and only override the executor (`BackgroundExecutor` vs `AsyncBackgroundExecutor`, `libs/langgraph/langgraph/pregel/_executor.py:40-217`). The runner mirror is `tick` / `atick` on the same `PregelRunner` (`libs/langgraph/langgraph/pregel/_runner.py:176-528`).
- **`FuturesDict` as the in-superstep scheduler.** `dict[Future, PregelExecutableTask | None]` with `add_done_callback` and an event gate, so the runner can yield control to the caller between done events (`libs/langgraph/langgraph/pregel/_runner.py:75-133`).
- **Durability is part of the loop, not an afterthought.** Three modes (`sync`/`async`/`exit`) gate whether `_put_checkpoint` blocks the next tick (`libs/langgraph/langgraph/pregel/_loop.py:1116-1189`).
- **Interrupts are pending writes, not exceptions past the boundary.** `interrupt(value)` raises inside a node; the runner catches it via `GraphBubbleUp` and persists a `("__interrupt__", ...)` pending write, which is replayable from any checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:806-834`).
- **Error handler routing is in-loop.** A node's `error_handler_node` becomes a sibling task scheduled by `PregelRunner` on failure (`libs/langgraph/langgraph/pregel/_runner.py:226-243`); `ERROR_SOURCE_NODE` markers persisted at the failure site drive handler scheduling on resume (`libs/langgraph/langgraph/pregel/_loop.py:739-804`).
- **Cooperative drain via `RunControl`.** A node can call `runtime.control.request_drain(reason)` to stop the next superstep without aborting the current one (`libs/langgraph/langgraph/runtime.py:79-105`, `libs/langgraph/langgraph/pregel/_loop.py:650-652`).
- **Layered public API.** `Pregel` (low-level) → `CompiledStateGraph` (`StateGraph.compile`, `libs/langgraph/langgraph/graph/state.py:1164-1363`) → `entrypoint` / `task` (functional, `libs/langgraph/langgraph/func/__init__.py:262`) → `create_react_agent` (agent, `libs/langgraph/langgraph/prebuilt/chat_agent_executor.py:789-990`). All compile to the same `Pregel` so the model is the same everywhere.

## Notable Patterns

- **BSP / Pregel superstep** (`libs/langgraph/langgraph/pregel/main.py:455-476`).
- **Concurrent futures scheduler with `FIRST_COMPLETED` wait** for cooperative yielding (`libs/langgraph/langgraph/pregel/_runner.py:282-289`).
- **Trigger-to-nodes reverse index** for cheap re-planning (`libs/langgraph/langgraph/pregel/main.py:4190-4196`).
- **Deterministic write ordering via `task_path_str`** so checkpoint serialization is stable (`libs/langgraph/langgraph/pregel/_algo.py:253-256`).
- **Pending writes as the single source of truth** for cross-step effects (`libs/langgraph/langgraph/pregel/_loop.py:408-499`).
- **Delta channel + counters for high-frequency channels** (`libs/langgraph/langgraph/channels/delta.py:25`, `_loop.py:1079-1138`).
- **Context-isolated node invocation via `copy_context`** so contextvars survive background-thread submission (`libs/langgraph/langgraph/pregel/_executor.py:64-69`).
- **Status literal** that fully describes where the loop is (`libs/langgraph/langgraph/pregel/_loop.py:249-257`).
- **Send-as-data**: dynamic fan-out via a `Topic[Send]` channel that the plan phase consumes (`libs/langgraph/langgraph/pregel/_algo.py:442-466`).

## Tradeoffs

- **BSP barrier cost on every step.** Every node writes go through `apply_writes` (`libs/langgraph/langgraph/pregel/_algo.py:232-345`) and are not visible until the next step; this caps per-step parallelism but guarantees consistency. The cost is amortized via channel versioning and the trigger index.
- **Single-node fast path.** `PregelRunner.tick` skips the futures machinery for a single task with no waiter (`libs/langgraph/langgraph/pregel/_runner.py:201-254`); async mirrors this (`libs/langgraph/langgraph/pregel/_runner.py:389-445`). Tradeoff: subtle path divergence between single and multi-task flows (visible in `commit`/`SKIP_RERAISE_SET`).
- **`_exit_delta_writes` lazy stub.** A `durability="exit"` run on a thread without a persisted parent has to invent a synthetic parent stub to anchor the final delta writes (`libs/langgraph/langgraph/pregel/_loop.py:1201-1293`). This is a non-obvious quirk the comment explicitly walks through.
- **Status literal vs exceptions.** `tick()` returns `False` to signal "done", but raises `GraphInterrupt` for interrupts (`libs/langgraph/langgraph/pregel/_loop.py:592-674`). Mixed return/raise semantics complicate reasoning about the loop boundary but are intentional (interrupts need durable pending writes, which return cannot do).
- **`sync` durability couples the loop to checkpoint latency.** `if durability_ == "sync": loop._put_checkpoint_fut.result()` (`libs/langgraph/langgraph/pregel/main.py:3002-3003`) blocks the next superstep on the previous checkpoint's persistence — strict but slow under load.

## Failure Modes / Edge Cases

- **Recursion limit** when no node halts the loop: `tick()` sets `out_of_steps` and the driver raises `GraphRecursionError` (`libs/langgraph/langgraph/pregel/_loop.py:600-602`, `libs/langgraph/langgraph/pregel/main.py:3017-3026`).
- **First run with no checkpoint**: `__enter__` falls back to a synthetic `empty_checkpoint()` and remembers `_has_persisted_parent = False` so `_put_exit_delta_writes` knows to mint a stub (`libs/langgraph/langgraph/pregel/_loop.py:1636-1666`, `:1201-1293`).
- **Time-travel replay**: `is_time_traveling` drops cached `RESUME` writes and clears stale `INTERRUPT` writes so a replay refires interrupts instead of replaying stale data (`libs/langgraph/langgraph/pregel/_loop.py:862-959`).
- **Multiple pending interrupts**: enforced at the resume site — `Command(resume=...)` with more than one pending interrupt requires an explicit interrupt-id map (`libs/langgraph/langgraph/pregel/_loop.py:898-908`).
- **Single-task `GraphBubbleUp` short-circuit**: the runner routes to error handler without bubbling, but only when the handler isn't itself failing (`libs/langgraph/langgraph/pregel/_runner.py:171-174`, `:226-243`).
- **Cross-subgraph resume via checkpoint_map**: parent passes `CONFIG_KEY_CHECKPOINT_MAP` so a subgraph replay can find its checkpoint at a bound (`libs/langgraph/langgraph/pregel/_loop.py:339-353`).
- **Concurrent errors with handled exceptions**: `_handled_exception_ids` (`libs/langgraph/langgraph/pregel/_runner.py:169`) ensures that exceptions routed to a graph-level handler don't get re-raised on the fatal path.
- **Delta channel write ordering at exit**: synthetic `step`-prefixed task IDs preserve superstep chronological order under the saver's `ORDER BY task_id, idx` sort (`libs/langgraph/langgraph/pregel/_loop.py:1274-1292`).
- **Skip-reraise weakset**: `SKIP_RERAISE_SET` (`libs/langgraph/langgraph/pregel/_runner.py:70-72`) prevents already-handled exceptions from re-raising after the futures settle.

## Future Considerations

- **Consolidating `_put_checkpoint` exit parameter.** The current `metadata is self.checkpoint_metadata` identity check is fragile; a TODO in `_loop.py:1074` flags this.
- **Single-task / multi-task runner divergence.** The two paths duplicate error-routing and re-raise logic; a unified path guarded by a `FuturesDict` even for one task would reduce drift.
- **Watcher for delta-channel snapshotting bounds.** `test_delta_channel_supersteps_bound.py` shows a predicate fires on overflow — the production wiring of this watcher is in tests, not the loop.
- **Migration safety net for plan-phase index changes.** `_trigger_to_nodes` is recomputed at compile time; runtime changes to `PregelNode.triggers` are not supported and could silently drop triggers.
- **Async overhead for sync-only graphs.** `AsyncBackgroundExecutor` requires an event loop even when no node is async; a fast path that returns the loop's running task directly would help latency-sensitive deployments.

## Questions / Gaps

- **How are channel `versions_seen` updated for nodes that don't read?** `apply_writes` only updates `versions_seen` for channels in `task.triggers` (`libs/langgraph/langgraph/pregel/_algo.py:262-269`). A node that reads via a mapper without subscribing via `triggers` would see stale values — no evidence of validation in the search boundary.
- **Is there a documented upper bound on `recursion_limit` / `step`?** `self.stop = self.step + self.config["recursion_limit"] + 1` (`libs/langgraph/langgraph/pregel/_loop.py:1677`) — no explicit cap beyond the user-set `recursion_limit` (default 25).
- **What governs `trigger_to_nodes` for nodes added after compile?** Search boundary: `_trigger_to_nodes` is computed in `Pregel.attach_node` (called once during compile). Late-bound nodes via dynamic `Send` go through PUSH and bypass this index; this is by design but not externally documented.
- **Behavior of `wait`/`await` cancel mid-superstep.** The `BackgroundExecutor.__exit__` cancels `__cancel_on_exit__=True` futures and waits for the rest (`libs/langgraph/langgraph/pregel/_executor.py:93-119`); no tests in the search boundary exercise a forced thread cancellation, only `RunControl.request_drain` which is cooperative (`libs/langgraph/tests/test_runtime.py:100-148`).
- **`delta_channels_to_snapshot` heuristic**: the `counters_since_delta_snapshot` predicate and threshold are tested (`libs/langgraph/tests/test_delta_channel_supersteps_bound.py:132-167`) but the threshold itself isn't quoted from a single source in this study; review the call site at `libs/langgraph/langgraph/pregel/_loop.py:1120-1124` to confirm.

---

Generated by `01.01-execution-model-taxonomy` against `langgraph`.