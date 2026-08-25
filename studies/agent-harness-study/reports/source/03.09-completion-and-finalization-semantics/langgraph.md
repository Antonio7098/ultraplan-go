# Source Analysis: langgraph

## 03.09: Completion and Finalization Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (Pregel runtime + LangChain integrations) |
| Analyzed | 2026-07-22 |

## Summary

LangGraph's `Pregel` runtime models completion as a **terminal status on the loop** (`status ∈ {"input", "pending", "done", "draining", "interrupt_before", "interrupt_after", "out_of_steps"}` declared as a `Literal` on `PregelLoop` at `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:249-257`). A run is "done" only when `tick()` returns `False` because `prepare_next_tasks` produced an empty task set (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:646-648`), or because the iteration limit was exceeded (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:600-602`), or because a cooperative drain was requested (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:650-652`). The driver `Pregel.stream` translates these into user-facing exceptions: `out_of_steps` → `GraphRecursionError` (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3017-3026`, `:3498-3507`), `draining` → `GraphDrained` (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3027-3030`, `:3508-3511`).

Finalization is **durability-driven, not correctness-driven**. The framework exposes three durability modes — `sync`, `async`, `exit` (`sources/langgraph/libs/langgraph/langgraph/types.py:87-93`) — and uses `durability="exit"` (the default for many streaming paths) to defer the final `Checkpoint` write to the `_suppress_interrupt` `__exit__` hook (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1294-1311`). `_put_exit_delta_writes` then stages accumulated `DeltaChannel` writes so that the final checkpoint and its backing writes become durable together (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1201-1292`). On a clean finish, the loop captures the final output once via `read_channels(self.channels, self.output_keys)` in `_suppress_interrupt` (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1350, :1355`).

There is **no run-summary / no usage or cost record / no per-run "status: success|error" field**. Finalization relies on the `BaseCheckpointSaver` storing `(Checkpoint, CheckpointMetadata, pending_writes)` (`sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86, :277-318`) — the metadata only encodes `source ∈ {"input","loop","update","fork"}` and `step`. The model treats completion as a **stop condition on the task set**, not a correctness assertion: a node that returns `None` (or whose writes happen to leave channels unchanged) terminates the run even though the graph may have produced a "no-op" answer. Validation is graph-structure-only at compile time (`sources/langgraph/libs/langgraph/langgraph/pregel/_validate.py:13-120`), and `_get_updates` raises `InvalidUpdateError` (`sources/langgraph/libs/langgraph/langgraph/graph/state.py:1478`) when nodes return invalid shapes — these raise rather than downgrade to a "completed-with-warnings" state.

## Rating

**8 / 10 — Clear model with explicit interfaces, but no usage/cost record and "done" means "stopped", not "correct".**

Rationale: The status literal is explicit and exhaustively mapped to user-visible behavior. Three durability modes give operators control over when finalization happens. Drain and recursion limits have first-class exceptions. However, there is no run-summary record, no aggregated token/cost accounting, and "done" is defined purely by the absence of triggerable tasks, so a misbehaving node can produce a syntactically valid but semantically empty final state without any explicit warning. The exit-mode finalization path is exceptionally engineered (visibility-invariant for `DeltaChannel` ancestors) but the absence of a `run_status`/summary means callers cannot tell "success with zero output" from "really finished" except by inspecting `state.next`.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Final output type | `GraphOutput` dataclass wrapping `value` + `interrupts`, returned by `invoke(version="v2")` / `ainvoke` | `sources/langgraph/libs/langgraph/langgraph/types.py:368-411` |
| Final output type | `invoke()` falls back to latest stream `values` payload (dict / typed) | `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3899-3972`, `:4075-4149` |
| Final output type | `stream_mode="values"` carries `__interrupt__` payload | `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:4234-4250` |
| Run status literal | `Literal["input", "pending", "done", "draining", "interrupt_before", "interrupt_after", "out_of_steps"]` on `PregelLoop` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:249-257` |
| Run status — done | `if not self.tasks: self.status = "done"; return False` (no more triggerable tasks) | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:646-648` |
| Run status — out_of_steps | `if self.step > self.stop: self.status = "out_of_steps"; return False` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:600-602` |
| Run status — draining | `if self.control is not None and self.control.drain_requested: self.status = "draining"; return False` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:650-652` |
| Driver status dispatch | `out_of_steps` → `GraphRecursionError`; `draining` → `GraphDrained` | `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3017-3031`, `:3498-3511` |
| Pending tool calls block completion | Runner waits on `concurrent.futures.wait(... FIRST_COMPLETED)` until all futures done; missing writes are normalized to `(NO_WRITES, None)` | `sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:282-339`, `:574-613` |
| Pending tool calls block completion | `match_cached_writes` / `amatch_cached_writes` lets tasks with cached results skip execution | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1526-1539`, `:1780-1793` |
| Pending tool calls block completion | `accept_push` may spawn additional `PregelExecutableTask`s mid-superstep (functional API `call()`) | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:543-580`, `_runner.py:700-786` |
| Pending tool calls block completion | `__next_tick__=True` defers child task start to next tick so updates flush first | `sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:776-782`, `:926-935` |
| Finalization records usage/cost/status | **No evidence found.** No `usage_metadata`, `cost`, `token_count`, or `run_status` aggregation in `CheckpointMetadata` or `Pregel` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` (schema does not contain these fields) |
| Final output validation | `validate_graph` rejects unknown channels / nodes at compile time | `sources/langgraph/libs/langgraph/langgraph/pregel/_validate.py:13-120` |
| Final output validation | `_defaults` enforces `recursion_limit >= 1` | `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:2578-2579` |
| Final output validation | `_get_updates` raises `InvalidUpdateError` (with `ErrorCode.INVALID_GRAPH_NODE_RETURN_VALUE`) on bad node return types | `sources/langgraph/libs/langgraph/langgraph/graph/state.py:1478` |
| Final output validation | Map updates reject writes to unknown keys at runtime | `sources/langgraph/libs/langgraph/langgraph/pregel/_io.py:81-97` |
| Finalization event — exit | `_suppress_interrupt` runs as `__exit__`; calls `_put_exit_delta_writes` + `_put_checkpoint(self.checkpoint_metadata)` + `_put_pending_writes` when durability=="exit" | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1294-1311` |
| Finalization event — exit | `_put_exit_delta_writes` builds synthetic step-prefixed `task_id` to preserve chronological order | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1201-1292` |
| Finalization event — exit | Stub checkpoint lazily created when no parent has been persisted yet | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1228-1258` |
| Finalization event — checkpoint | `_put_checkpoint` flushes checkpoint only when `do_checkpoint = _checkpointer_put_after_previous is not None and (exiting or durability != "exit")` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1115-1196` |
| Finalization event — drain | `Request_drain` flips `RunControl._drain_reason`; `drain_requested` checked on each tick | `sources/langgraph/libs/langgraph/langgraph/runtime.py:79-104`, `_loop.py:650-652` |
| Run summary | `run_manager.on_chain_end(loop.output)` fires once at stream/invoke exit; `on_chain_error` on raised exceptions | `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3032-3035`, `:3513-3517` |
| Run summary | `PregelLoop.output = read_channels(self.channels, self.output_keys)` is the canonical final value | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1350, :1355` |
| Finalization visibility | `_checkpointer_put_after_previous` drains `DeltaChannel` write futures before `put` so checkpoints never become visible before their backing writes | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1515-1524`, `:1766-1778` |
| Finalization visibility | Error-handler writes (`ERROR_SOURCE_NODE`) awaited before handler executes | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1556-1558`, `:1810-1812` |
| Pending-writes coalescing | Writes are accumulated per `task_id`; deduplicated for special channels via `WRITES_IDX_MAP` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:408-450` |
| Interrupt finalization | `interrupt_before` raises `GraphInterrupt` before task runs; `interrupt_after` raises after `after_tick` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:659-664, :707-712` |
| Interrupt finalization | `_suppress_interrupt` swallows root-graph `GraphInterrupt`, emits final `values` + `updates` events, pushes `GraphInterruptEvent` for callback consumers | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1312-1352` |
| Subgraph lifecycle | `SubgraphStatus = Literal["started", "completed", "failed", "interrupted", "drained"]` projected onto v3 `lifecycle` events | `sources/langgraph/libs/langgraph/langgraph/stream/transformers.py:343`, `:582-605` |
| Subgraph lifecycle | `LifecycleTransformer.finalize()` emits `completed` for any namespace still open at run end | `sources/langgraph/libs/langgraph/langgraph/stream/transformers.py:568-579` |
| Stop condition — `recursion_limit` | `DEFAULT_RECURSION_LIMIT = 10007` (env-overridable); `self.stop = self.step + self.config["recursion_limit"] + 1` | `sources/langgraph/libs/langgraph/langgraph/_internal/_config.py:32`, `_loop.py:1677`, `:1936` |
| `NO_WRITES` marker | Tasks that produce no writes get `(NO_WRITES, None)` appended by `commit()`; allows `state.next` to surface them as completed | `sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:609-611`, `_internal/_constants.py:18` |
| Drain test | `test_request_drain_allows_inflight_call_scheduling` verifies the drain completion path | `sources/langgraph/libs/langgraph/tests/test_pregel.py:125-145` |
| Recursion-limit test | `test_invoke_two_processes_in_out` asserts `GraphRecursionError` on limit breach | `sources/langgraph/libs/langgraph/tests/test_pregel.py:556-575` |
| Pending-writes resume test | `test_pending_writes_resume` validates checkpoint durability under `ConnectionError` retry policy | `sources/langgraph/libs/langgraph/tests/test_pregel.py:877-973` |

## Answers to Dimension Questions

### 1. What counts as done?

**A superstep is "done" when `prepare_next_tasks` returns an empty dict.** Concretely: every channel that the next step would subscribe to is either unavailable or has not been updated beyond its `versions_seen` for any node (`sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:441-512`). The loop exits when `tick()` returns `False` because `not self.tasks` (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:646-648`). This is a **structural** notion of done — it does not check whether the result is "correct" or "non-empty". A node that returns an empty dict, or whose writes leave every triggerable channel unchanged, will terminate the run successfully.

### 2. Can the model falsely declare done?

**Yes.** Any node that returns nothing writeable triggers termination. `_runner.commit()` even appends `(NO_WRITES, None)` so empty tasks are persisted as "completed with no writes" (`sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:609-611`). The `StateSnapshot.next` field on a successful run is empty by definition (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:1258`), so `get_state()` cannot distinguish "node A produced no writes intentionally" from "node A was supposed to write X but didn't". There is no `result_validator` / `output_validator` hook on `Pregel` — validation is limited to channel-shape checks (`sources/langgraph/libs/langgraph/langgraph/pregel/_validate.py:13-120`).

### 3. Are final outputs validated?

**Compile-time yes; runtime partially.** Compile-time validation rejects unknown channels/nodes/reserved names (`sources/langgraph/libs/langgraph/langgraph/pregel/_validate.py:13-120`). Runtime validation is delegated to channel reducers and the `_get_updates` mapper that raises `InvalidUpdateError` (`sources/langgraph/libs/langgraph/langgraph/graph/state.py:1478`, with `ErrorCode.INVALID_GRAPH_NODE_RETURN_VALUE` per `sources/langgraph/libs/langgraph/langgraph/errors.py:34-47`). These raise; they do not downgrade. There is no schema validation of the **final** `loop.output` against an output model — the only safeguard is that the user calls `read_channels(self.channels, self.output_keys)` (`sources/langgraph/libs/langgraph/langgraph/pregel/_io.py:38-53`), which silently substitutes `None` for unreadable channels via `EmptyChannelError`.

### 4. Is usage/cost finalized?

**No.** `CheckpointMetadata` only carries `source`, `step`, `parents`, `run_id`, and the beta `counters_since_delta_snapshot` (`sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86`). There is no `usage_metadata`, `cost`, `token_count`, or `run_status` aggregation. `run_manager.on_chain_end(loop.output)` is the only LangChain-callback hook fired at finalization (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3032-3035`, `:3513-3517`), and it is the user's responsibility to attach a tracer that records token counts. The LangChain/LangSmith side handles usage; LangGraph itself does not aggregate.

### 5. Can a run complete with warnings?

**No, not as a first-class concept.** The status literal has no `done_with_warnings` state (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:249-257`). Two patterns can carry "soft failure" signals:

1. **`GraphBubbleUp` / `GraphInterrupt`** are caught by `_panic_or_proceed` and the loop suppresses them as run-level interruptions, but this is still an `interrupt_*` status, not a warning (`sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:650-697`).
2. **Node-level `error_handler` chains** can convert an exception into a recovered state (`sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:171-174, :596-602`). The handler executes and the run continues; the original failure is recorded as an `ERROR` pending write (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1339-1403`), recoverable via `state.tasks[i].error`.

There is no `warnings.warn(...)` pipeline or `RunReport.warning` field. The `logger.warning` calls scattered through the algo are dev-time signals (`sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:973-974, :978-979, :999-1000`).

## Architectural Decisions

- **Status-as-Literal, not exception.** LangGraph treats "done" / "draining" / "out_of_steps" / `interrupt_*` as values, not control flow. This lets `invoke` and `stream` share one driver loop and one `_suppress_interrupt` finalizer (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1294-1356`), and lets `Pregel.invoke` produce typed return values for success paths while letting exceptions flow for failure paths (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3017-3036`).
- **Durability is orthogonal to completion.** The `Durability = Literal["sync", "async", "exit"]` literal (`sources/langgraph/libs/langgraph/langgraph/types.py:87-93`) decouples *when* the final `put` happens from *whether* the run completed. `durability="exit"` flips `_put_checkpoint` to defer until `__exit__` (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1115-1196, :1294-1311`), trading throughput for cost on "long-tail" runs.
- **Finalization hook as a stack callback.** `_suppress_interrupt` is registered via `self.stack.push(self._suppress_interrupt)` (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1674`, `:1933`). This unifies context-manager exit with finalization: the same code path handles clean exit, `GraphInterrupt`, `GraphDrained`, and node-level exceptions.
- **Per-task completion, not per-run completion.** `PregelRunner` tracks each task via `FuturesDict` with `on_done` callbacks (`sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:75-132`). Each task produces a `commit()` call that decides whether to write `(NO_WRITES, None)`, `(ERROR, exc)`, `(INTERRUPT, …)`, or normal writes (`sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:574-613`). The run-level completion is then a derived aggregate of these.
- **Final output = `read_channels(channels, output_keys)`.** The loop stashes the answer in `loop.output` exactly once at exit (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1350, :1355`). `invoke()` post-processes this into `GraphOutput(value, interrupts)` for v2 (`sources/langgraph/libs/langgraph/langgraph/types.py:368-378`) or a dict with `__interrupt__` for v1 (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3961-3972`).
- **Visibility invariant for delta writes.** When `durability="exit"` and `DeltaChannel`s are present, `_put_exit_delta_writes` stages a stub-or-parent anchor checkpoint and step-prefixed task_ids so that checkpoint writes and delta writes become durable in a single ordered batch (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1201-1292`). This is the most engineered finalization code in the file.
- **StateSnapshot.next as residual tasks.** `get_state` derives `next` from `tuple(t.name for t in next_tasks.values() if not t.writes)` (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:1258, :1382`) — i.e., "what would have been done if you resumed from this checkpoint". An empty `next` is the definitive signal that the graph is finished; this is how external code confirms completion without re-running the loop.

## Notable Patterns

- **Tasks write to channels, channels drive tasks.** The Pregel/BSP model is its own finalization primitive: when the dataflow subsides, the loop exits naturally. There is no explicit "done" node; `END` is a sentinel channel name (`sources/langgraph/libs/langgraph/langgraph/constants.py:28`) consumed by the `entrypoint` decorator's `ChannelWriteEntry(END, mapper=...)` (`sources/langgraph/libs/langgraph/langgraph/func/__init__.py:584-589`).
- **`entrypoint.final[Return, Save]` two-channel pattern.** The functional API distinguishes the value returned to the caller (`value`) from the value persisted for the next invocation (`save`) via `PREVIOUS`/`END` channels (`sources/langgraph/libs/langgraph/langgraph/func/__init__.py:476-619`). This is a clean way to keep the public return value distinct from the final-state-saved value.
- **Lifecycle event fan-out.** Two parallel surfaces — `GraphResumeEvent`/`GraphInterruptEvent` callback events (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:369-401`) and the v3 `LifecycleTransformer` (`sources/langgraph/libs/langgraph/langgraph/stream/transformers.py:608-668`) — give both in-process consumers and remote SDK clients a uniform `started/completed/failed/interrupted/drained` view of subgraph finalization.
- **Per-superstep write coalescing.** `_put_pending_writes` groups `checkpoint_pending_writes` by `task_id` and submits one `put_writes` per task (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:503-541`). Combined with `WRITES_IDX_MAP` deduplication (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:412-414`), this bounds the persistence cost of multi-write tasks.
- **`channel_versions` is monotonic and idempotent.** `apply_writes` advances channel versions via `checkpointer_get_next_version` (`sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:232-345`), which makes "no channel changed" equivalent to "no task triggered", which makes the empty `next_tasks` set the canonical completion signal.

## Tradeoffs

- **Done ≠ correct.** A graph whose last node writes `{}` will finish successfully and emit no error. Callers who need a non-empty invariant must validate after the fact (the documentation examples in `Pregel.__init__` show this explicitly: the cycle example terminates by writing `None` to a channel and consuming it (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:670-700`)).
- **`durability="exit"` defers visibility.** When the process dies between `after_tick` and `_suppress_interrupt`, the last superstep's checkpoint may be missing its delta-channel writes. `_put_exit_delta_writes` mitigates this with the stub-parent pattern (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1228-1258`) but the trade is unavoidable: lower write throughput vs. crash safety.
- **Recursion limit is global, not per-node.** `self.stop = self.step + self.config["recursion_limit"] + 1` (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1677`, `:1936`) treats the whole graph as one budget. A long-running agent and a tight loop share the same cap, which `GraphRecursionError` does not disambiguate.
- **Per-run cost not aggregated.** Token usage is exposed via LangChain callbacks (which downstream tracers like LangSmith record), but `run_manager.on_chain_end` does not propagate per-node usage. A caller wanting a per-run usage total must assemble it from callback events.
- **No retry / no warning counter.** A node that always returns empty writes is silently retried zero times by the runner (`_should_stop_others` only fires on exceptions or `GraphBubbleUp` — `sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:616-634`). This is correct for an end-of-stream sentinel but means stuck "I'm done" loops will simply terminate without complaint.

## Failure Modes / Edge Cases

- **Infinite loops terminate as `GraphRecursionError`.** `app.invoke(2, {"recursion_limit": 1}, debug=1)` raises `GraphRecursionError` (`sources/langgraph/libs/langgraph/tests/test_pregel.py:574-575`). Driver code maps `loop.status == "out_of_steps"` to `GraphRecursionError` with `ErrorCode.GRAPH_RECURSION_LIMIT` (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3017-3026`).
- **Cooperative drain exits cleanly.** A node that calls `control.request_drain()` while still having downstream work completes the current superstep, then exits with `GraphDrained(reason)` (`sources/langgraph/libs/langgraph/langgraph/errors.py:54-64`). The checkpoint is saved and the run can be resumed (`sources/langgraph/libs/langgraph/langgraph/runtime.py:79-104`, `_loop.py:650-652`).
- **Exit-time exception swallowed silently by `_suppress_interrupt`.** When `_suppress_interrupt` runs in `__exit__`, it suppresses `GraphInterrupt` by design (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1313-1352`) but other exceptions propagate (and `run_manager.on_chain_error` records them — `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3033-3035`). A `BaseException` raised inside `__aexit__` is wrapped with the exit task to permit consumer cleanup (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1959-1962`).
- **Empty input fails fast.** `_first()` raises `EmptyInputError` when the call has neither a `Command` nor an input payload and the checkpoint isn't resuming (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1017-1018`, error class at `sources/langgraph/libs/langgraph/langgraph/errors.py:136-139`).
- **Bad node return value raises `InvalidUpdateError`.** When a node returns a value that is neither a `dict`, a `Command`, an annotated class, nor an `entrypoint.final`, `_get_updates` raises `InvalidUpdateError` with `ErrorCode.INVALID_GRAPH_NODE_RETURN_VALUE` (`sources/langgraph/libs/langgraph/langgraph/graph/state.py:1478`).
- **Per-task write cap via `_put_checkpoint_fut.result()` for `durability="sync"`.** After each `after_tick`, the driver awaits the checkpoint save future before starting the next tick (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3002-3003`, `:3475-3477`). If the saver hangs, the whole run hangs.
- **Cancelled async tasks → `NodeCancelledError` if user-raised.** The retry layer converts user-raised `asyncio.CancelledError` into `NodeCancelledError` so it flows through the normal error path (`sources/langgraph/libs/langgraph/langgraph/errors.py:168-186`). Framework-initiated cancellation is treated as silent teardown.
- **`NO_WRITES` marker on task-without-writes.** If a task body returns without writing, `commit()` writes `(NO_WRITES, None)` to the checkpointer (`sources/langgraph/libs/langgraph/langgraph/pregel/_runner.py:609-611`). On resume, the task will be re-prepared by `prepare_next_tasks` and re-executed; a node that consistently returns no writes will run every resumed tick.
- **`EmptyInputError` from `_first`.** Triggered by `app.invoke(None, config)` without `RESUMING=True` set and no `Command.resume` (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1017-1018`).
- **`_pending_interrupts()` requires `dict` resume map for multi-interrupt.** `_first()` raises `RuntimeError` when more than one interrupt is pending and a single non-map resume is supplied (`sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:903-908`). The error message links to documentation but the failure is not surfaced as a status — it raises.

## Future Considerations

- **`run_status` summary.** A `RunReport` value object aggregating (status, total_tokens, cost, warnings, error_count) would let `invoke` and `astream` return more than just the final value. Today this is reconstructed manually from `state.history` + LangSmith callbacks.
- **Per-node usage propagation.** Hooking `run_manager.on_chain_end` to also call `on_llm_end`-derived aggregators (or adding a `CONFIG_KEY_USAGE_AGGREGATOR`) would expose per-run cost without requiring LangSmith.
- **Soft-finalize status.** A `done_with_warnings` state in the `Literal` (and a `RunWarning` payload list) would let graphs distinguish "structurally finished" from "all invariants satisfied".
- **Per-node validation hooks.** A `node_validator` callback that runs after `apply_writes` but before `after_tick` would catch "no-op completed" cases that today pass silently. The validation API exists for compile-time (`_validate.py`) but not runtime output.
- **Recursion-limit breakdown.** Reporting how many of the `recursion_limit` supersteps each node consumed (in `GraphRecursionError.args`) would help debugging.

## Questions / Gaps

- **Does the `invoke(version="v2")` `GraphOutput` distinguish success from "exited with interrupt"?** No — `GraphOutput.interrupts` is a tuple, and an empty tuple plus a non-`None` value is the only success signal. A consumer who forgets to inspect `interrupts` cannot tell whether the run paused for human input or completed.
- **Is the `_suppress_interrupt` finalization observable to user code beyond `run_manager.on_chain_end`?** No — the callback manager is the only hook. There is no `Pregel.on_run_finished(fn)` or `Pregel.finalizer` attribute.
- **What does the user see when the run exits via `GraphDrained`?** `astream` raises `GraphDrained(reason)` (`sources/langgraph/libs/langgraph/langgraph/pregel/main.py:3508-3511`); `invoke` re-raises since it consumes `astream`. There is no `drain_reason` field on a returned `GraphOutput` — the consumer must catch the exception.
- **How is "duration" tracked?** The retry layer records `started_at` / `finished_at` per attempt (`sources/langgraph/libs/langgraph/langgraph/pregel/_retry.py:104, :385`), but the loop does not record `run_started_at` / `run_finished_at` anywhere accessible to user code.
- **Where is the run's "exceptions" list?** Exceptions bubble up; there is no aggregation into a `run.errors` field. The only side-channel is the LangChain callback manager.

---

Generated by `03.09-completion-and-finalization-semantics` against `langgraph`.
