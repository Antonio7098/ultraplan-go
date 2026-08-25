# Source Analysis: langgraph

## Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph/langgraph` + functional `langgraph.func`, prebuilt `libs/prebuilt/langgraph_prebuilt`); pregel-style superstep engine |
| Analyzed | 2026-07-02 |

## Summary

LangGraph's loop-bound model has four independent termination regimes that compose:

1. **Recursion limit** (`recursion_limit` in `RunnableConfig`) — the primary hard cap on supersteps per run. Default **10007** supersteps (`libs/langgraph/langgraph/_internal/_config.py:32`), env-overridable via `LANGGRAPH_DEFAULT_RECURSION_LIMIT`. Checked at the top of every `PregelLoop.tick()` (`libs/langgraph/langgraph/pregel/_loop.py:600-602`); on exit `stream()`/`astream()` translates the `out_of_steps` status into a `GraphRecursionError` with a remediation message (`libs/langgraph/langgraph/pregel/main.py:3017-3026`, `3501-3507`).
2. **Per-superstep wall-clock timeout** (`Pregel.step_timeout`, `libs/langgraph/langgraph/pregel/main.py:726, 770-816`) — caps each step's execution phase via the runner's `wait(..., timeout=...)` primitive; produces `TimeoutError`/`asyncio.TimeoutError` (`libs/langgraph/langgraph/pregel/_runner.py:280-289, 479-485, 691-697`).
3. **Per-node timeout policies** (`TimeoutPolicy`, `libs/langgraph/langgraph/types.py:450-512`) — `run_timeout` (hard wall-clock) and `idle_timeout` (no observable progress), with `runtime.heartbeat()` / LangChain callback events as progress signals (`libs/langgraph/langgraph/pregel/_retry.py:128-510`). Surfaces as `NodeTimeoutError` (`libs/langgraph/langgraph/errors.py:190-241`), which is retryable by the default `RetryPolicy` (`libs/langgraph/langgraph/errors.py:193-194`).
4. **Cooperative draining** (`RunControl.request_drain()`, `libs/langgraph/langgraph/runtime.py:79-104`) — thread-safe single-attribute write polled at start of every tick (`libs/langgraph/langgraph/pregel/_loop.py:650-652`); exits with `GraphDrained(reason)` (`libs/langgraph/langgraph/errors.py:54-64`, raised at `libs/langgraph/langgraph/pregel/main.py:3027-3030, 3508-3511`). This is the documented mechanism for SIGTERM handling (`libs/langgraph/langgraph/errors.py:57-60`).

On top of these, the framework distinguishes **expected pauses** (interrupt_before / interrupt_after / dynamic `interrupt()`, all surfacing `GraphInterrupt(Interrupt(...))` as `GraphBubbleUp`) from **runtime stops** (draining) and **exhaustion** (out_of_steps). There is **no stuck-loop detection** — the only safeguard against an LLM stuck emitting the same action every step is the `recursion_limit` cap plus `IsLastStep` / `RemainingSteps` managed values (`libs/langgraph/langgraph/managed/is_last_step.py:9-24`) that the prebuilt `create_react_agent` uses to *voluntarily* stop before the cap fires (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634, 432-440`).

Termination is **runtime-driven**, not model-driven: `loop.status` is the source of truth and is checked once after the superstep loop exits. **Termination reasons are partially persisted** — every step's `step` value is written to checkpoint metadata (`libs/langgraph/langgraph/pregel/_loop.py:1108`), and the last checkpoint is flushed on exit (`libs/langgraph/langgraph/pregel/_loop.py:1294-1311`), but the *reason* (out_of_steps vs draining vs done vs interrupt_before/after) is **not** stored as a status field — callers learn the reason from the exception type and message.

## Rating

**8 / 10** — Mature loop-bound model. Explicit typed statuses (`Literal[...]` in `_loop.py:249-257`), dedicated exception hierarchy (`GraphRecursionError`, `GraphDrained`, `GraphInterrupt`, `NodeTimeoutError`), multiple orthogonal caps (superstep count, wall-clock per superstep, wall-clock per node attempt, idle timeout per node attempt, cooperative drain), and persistence of the final checkpoint on every termination path. Tests cover all main paths (`tests/test_pregel.py:574-575`, `tests/test_pregel_async.py:1163-1194`, `tests/test_runtime.py:100-668`).

Missing / weaker: (a) no semantic stuck-loop detection (identical channel states or repeated node calls are not flagged before the hard cap), (b) termination reason is not persisted in checkpoint metadata — `get_state` after exhaustion returns the same shape as a normal checkpoint, only the *exception* differentiates them, (c) `step_timeout` is a per-superstep cap not a per-run cap, so a single slow step can consume the budget while the run never approaches `recursion_limit`, (d) the `recursion_limit = 10007` default is huge by design (this is a documented "default never triggers" choice — see `_internal/_config.py:32`) which means small misconfigured loops fail loudly only when users explicitly lower it.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default recursion limit & env override | `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))`; applied by `ensure_config` and merged by `merge_configs` | `libs/langgraph/langgraph/_internal/_config.py:32` ; `libs/langgraph/langgraph/_internal/_config.py:184-186` ; `libs/langgraph/langgraph/_internal/_config.py:335` |
| `recursion_limit` validation | `if config["recursion_limit"] < 1: raise ValueError("recursion_limit must be at least 1")` | `libs/langgraph/langgraph/pregel/main.py:2578-2579` |
| Superstep counter init | `self.stop = self.step + self.config["recursion_limit"] + 1` (sync `__enter__`, async `__aenter__`) | `libs/langgraph/langgraph/pregel/_loop.py:1677` ; `libs/langgraph/langgraph/pregel/_loop.py:1936` |
| Tick-level step cap check | `if self.step > self.stop: self.status = "out_of_steps"; return False` — first statement of `PregelLoop.tick()` | `libs/langgraph/langgraph/pregel/_loop.py:600-602` |
| Loop status enum (typed) | `status: Literal["input", "pending", "done", "draining", "interrupt_before", "interrupt_after", "out_of_steps"]` | `libs/langgraph/langgraph/pregel/_loop.py:249-257` |
| Normal completion (`done`) | Empty task set terminates tick: `if not self.tasks: self.status = "done"; return False` | `libs/langgraph/langgraph/pregel/_loop.py:646-648` |
| Drain detection inside tick | `if self.control is not None and self.control.drain_requested: self.status = "draining"; return False` | `libs/langgraph/langgraph/pregel/_loop.py:650-652` |
| `interrupt_before` exit | Status set then `GraphInterrupt()` raised (caught and suppressed for non-nested) | `libs/langgraph/langgraph/pregel/_loop.py:660-664` |
| `interrupt_after` exit | Same pattern at end of `after_tick()` | `libs/langgraph/langgraph/pregel/_loop.py:708-712` |
| Sync stream: `out_of_steps` → `GraphRecursionError` | Constructs message with `recursion_limit` value and `error_code=ErrorCode.GRAPH_RECURSION_LIMIT`, raises `GraphRecursionError` | `libs/langgraph/langgraph/pregel/main.py:3017-3026` |
| Async stream: same translation | `error_code=ErrorCode.GRAPH_RECURSION_LIMIT`, raises `GraphRecursionError` | `libs/langgraph/langgraph/pregel/main.py:3498-3507` |
| `GraphRecursionError` exception type | `class GraphRecursionError(RecursionError)` with troubleshooting guide link and inline example showing `recursion_limit: 1000` override | `libs/langgraph/langgraph/errors.py:67-87` |
| `ErrorCode` enum | `GRAPH_RECURSION_LIMIT = "GRAPH_RECURSION_LIMIT"`; `create_error_message` appends docs URL | `libs/langgraph/langgraph/errors.py:34-47` |
| `GraphDrained` exception type | `class GraphDrained(GraphBubbleUp)` with `reason` attribute, docstring explaining SIGTERM use | `libs/langgraph/langgraph/errors.py:54-64` |
| Sync stream: `draining` → `GraphDrained` | `raise GraphDrained(loop.control.drain_reason or "shutdown")` | `libs/langgraph/langgraph/pregel/main.py:3027-3030` |
| Async stream: same translation | Equivalent async path | `libs/langgraph/langgraph/pregel/main.py:3508-3511` |
| `RunControl` definition | `class RunControl` with `request_drain(reason)`, `drain_requested`, `drain_reason`; thread-safe single-attribute write | `libs/langgraph/langgraph/runtime.py:79-104` |
| Runtime.drain_requested accessor | Mirrors `RunControl.drain_requested` on `Runtime` for node injection | `libs/langgraph/langgraph/runtime.py:277-282` |
| `Pregel.step_timeout` field | `step_timeout: float \| None = None` with docstring "Maximum time to wait for a step to complete, in seconds." | `libs/langgraph/langgraph/pregel/main.py:726-727` |
| `step_timeout` constructor | `def __init__(self, *, step_timeout: float \| None = None, ...); self.step_timeout = step_timeout` | `libs/langgraph/langgraph/pregel/main.py:770, 816` |
| Sync runner tick: `timeout` parameter consumed | `end_time = timeout + time.monotonic() if timeout else None`; `concurrent.futures.wait(..., timeout=...)`; on no-done break → `_panic_or_proceed` raises `TimeoutError("Timed out")` | `libs/langgraph/langgraph/pregel/_runner.py:280-289, 343-349, 697` |
| Async runner tick: same | `end_time = timeout + loop.time() if timeout else None`; `asyncio.wait_for(...)`; `_panic_or_proceed(... timeout_exc_cls=asyncio.TimeoutError ...)` | `libs/langgraph/langgraph/pregel/_runner.py:479-485, 546-549, 559, 691-697` |
| `step_timeout` plumbed into runner | Sync: `for _ in runner.tick([...], timeout=self.step_timeout, ...)`; async: same | `libs/langgraph/langgraph/pregel/main.py:2982-2987` ; `libs/langgraph/langgraph/pregel/main.py:3455-3460` |
| `TimeoutPolicy` definition | `run_timeout` (hard cap), `idle_timeout` (no progress), `refresh_on: Literal["auto", "heartbeat"]`, `coerce()` validator | `libs/langgraph/langgraph/types.py:449-512` |
| `NodeTimeoutError` exception | Carries `node`, `timeout`, `kind`, `elapsed`, `idle_timeout`, `run_timeout`; docstring notes it deliberately does NOT inherit `TimeoutError` so default `RetryPolicy` treats it as retryable | `libs/langgraph/langgraph/errors.py:190-241` |
| `_TimedAttemptScope` | Guards writes so cancelled background tasks cannot persist writes past the timeout boundary; stream output is best-effort | `libs/langgraph/langgraph/pregel/_retry.py:128-152` |
| `_run_timeout_watchdog` | `await asyncio.sleep(run_timeout_s)` then triggers cancellation | `libs/langgraph/langgraph/pregel/_retry.py:417-418` |
| `_arun_with_timeout` | Sets up idle + run watchdogs in parallel; raises `NodeTimeoutError` on whichever fires first | `libs/langgraph/langgraph/pregel/_retry.py:422-510` |
| Sync timeout limitation | `sync_timeout_unsupported(name)` raised at compile time when a sync node has `timeout` set | `libs/langgraph/langgraph/pregel/_utils.py:135-137` ; `libs/langgraph/langgraph/pregel/main.py:934-935` |
| `interrupt_before` / `interrupt_after` on compile | `def compile(self, ..., interrupt_before: All \| list[str] \| None = None, interrupt_after: All \| list[str] \| None = None, ...)` | `libs/langgraph/langgraph/graph/state.py:1164-1249` |
| Interrupt nodes wired to `Pregel` | `interrupt_before_nodes=interrupt_before, interrupt_after_nodes=interrupt_after` | `libs/langgraph/langgraph/graph/state.py:1348-1349` |
| `should_interrupt` predicate | Compares channel versions against `versions_seen[INTERRUPT]` to detect activity since the last interrupt; supports `"*"` to interrupt at every node | `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Dynamic `interrupt()` function | Raises `GraphInterrupt(Interrupt.from_ns(value, ns))`; returns resume value on re-entry; requires checkpointer | `libs/langgraph/langgraph/types.py:811-929` |
| `Command.PARENT` subgraph termination | `PARENT: ClassVar[Literal["__parent__"]] = "__parent__"`; subgraph returns `Command(graph=Command.PARENT, goto=...)`; bubbles via `ParentCommand(GraphBubbleUp)` | `libs/langgraph/langgraph/types.py:808` ; `libs/langgraph/langgraph/errors.py:129-134` ; `libs/langgraph/langgraph/graph/state.py:1447-1466` |
| Interrupt suppression at loop exit | `_suppress_interrupt` flushes final checkpoint then returns `True` to swallow `GraphInterrupt` for top-level loop; emits `__interrupt__` stream event and saves `self.output` | `libs/langgraph/langgraph/pregel/_loop.py:1294-1355` |
| Final checkpoint persistence on exit | `if self.durability == "exit" and (not self.is_nested or exc_value or all(NS_END not in part for part in self.checkpoint_ns)): self._put_exit_delta_writes(); self._put_checkpoint(self.checkpoint_metadata); self._put_pending_writes()` | `libs/langgraph/langgraph/pregel/_loop.py:1301-1311` |
| `IsLastStep` managed value | `return scratchpad.step == scratchpad.stop - 1` — true only on the final step (one before `recursion_limit` cap) | `libs/langgraph/langgraph/managed/is_last_step.py:9-15` |
| `RemainingSteps` managed value | `return scratchpad.stop - scratchpad.step` — used by `create_react_agent` to short-circuit before cap | `libs/langgraph/langgraph/managed/is_last_step.py:18-24` |
| Prebuilt agent short-circuit | `_are_more_steps_needed`: `if remaining_steps < 1 and all_tools_return_direct: return True` (force stop) or `elif remaining_steps < 2 and has_tool_calls: return True` (force stop with explanation) | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634` |
| Agent state default | `remaining_steps: RemainingSteps = 25` (Pydantic default); docstring documents `remaining_steps ≈ recursion_limit - total_steps_taken` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:62, 74, 432-440` |
| `step` persisted in checkpoint metadata | `metadata["step"] = self.step` in `_put_checkpoint` | `libs/langgraph/langgraph/pregel/_loop.py:1108` |
| Step increment | `if not exiting: self.step += 1` after successful checkpoint | `libs/langgraph/langgraph/pregel/_loop.py:1197-1199` |
| `recursion_limit` sanitization on remote graph | `if "recursion_limit" in config: sanitized["recursion_limit"] = config["recursion_limit"]` | `libs/langgraph/langgraph/pregel/remote.py:410-411` |
| `max_concurrency` (not a termination control) | Semaphore-bounded parallel execution in `AsyncBackgroundExecutor`; throughput control, not loop bound | `libs/langgraph/langgraph/pregel/_executor.py:131-140` |
| `merge_configs` recursion_limit precedence | Custom rule: only overwrite base if `config["recursion_limit"] != DEFAULT_RECURSION_LIMIT` (so unset callers don't clobber explicit values) | `libs/langgraph/langgraph/_internal/_config.py:184-186` |
| `patch_config` recursion_limit setter | Helper used by library internals to propagate a new `recursion_limit` | `libs/langgraph/langgraph/_internal/_config.py:194-226` |
| Tests for `recursion_limit` exhaustion | `pytest.raises(GraphRecursionError): app.invoke(2, {"recursion_limit": 1})` (sync + async) | `libs/langgraph/tests/test_pregel.py:574-575` ; `libs/langgraph/tests/test_pregel_async.py:1571-1572` |
| Tests for `step_timeout` | `test_step_timeout_on_stream_hang` confirms `pytest.raises(asyncio.TimeoutError)` and that the inner task receives `asyncio.CancelledError` | `libs/langgraph/tests/test_pregel_async.py:1163-1194` |
| Tests for `step_timeout` propagation to subgraphs | Parameterized `with_timeout in ("inner", "outer", "both")` confirms both parent and child step timeouts apply; `test_timeout_with_parent_command` confirms ParentCommand propagates despite timeout | `libs/langgraph/tests/test_pregel.py:8356-8488` ; `libs/langgraph/tests/test_pregel_async.py:8880-8948` |
| Tests for cooperative drain | `test_run_control_request_drain_stops_future_steps`, `test_external_drain_concurrent_sync`, `test_drain_with_exit_durability_persists_resume_checkpoint` | `libs/langgraph/tests/test_runtime.py:100-203, 582-626, 670-740` |

## Answers to Dimension Questions

1. **What stops the loop?**
   Five distinct stop paths inside `PregelLoop.tick()` / `after_tick()` / post-stream check (`libs/langgraph/langgraph/pregel/_loop.py:592-712`, `main.py:3017-3030, 3498-3511`):
   - **Done**: no tasks scheduled (`_loop.py:646-648`). The graph has reached a stable point with no triggered nodes.
   - **`out_of_steps`**: `self.step > self.stop` (`_loop.py:600-602`). The run exceeded `recursion_limit`.
   - **`draining`**: `self.control.drain_requested` (`_loop.py:650-652`). External `RunControl.request_drain()` signaled shutdown.
   - **`interrupt_before` / `interrupt_after`**: `should_interrupt(...)` matched a configured node set, raised `GraphInterrupt()` (`_loop.py:660-664, 708-712`). Expected pause, not failure.
   - **Dynamic `interrupt()`** inside a node: `GraphInterrupt(Interrupt(...))` raised at the `interrupt()` call site (`types.py:927-929`).

2. **Are limits configurable?**
   Yes, all four primary limits are runtime-configurable:
   - `recursion_limit`: per-run via `RunnableConfig` (default 10007, env-overridable). `_internal/_config.py:32, 335`.
   - `step_timeout`: per-graph via `Pregel.__init__(step_timeout=...)` or attribute assignment. `main.py:726, 770-816`.
   - `TimeoutPolicy`: per-node via `PregelNode(timeout=...)` or `add_node(..., timeout=...)`. `types.py:450-512, 723-736`; `pregel/_read.py:128, 165-177`.
   - `interrupt_before` / `interrupt_after`: per-graph via `compile(..., interrupt_before=..., interrupt_after=...)` or `Pregel.stream(..., interrupt_before=..., interrupt_after=...)`. `state.py:1164-1249`.
   - `RunControl` is constructed per run and passed through `Pregel.stream(..., control=...)` (`main.py:2643, 3087-3094`).

3. **Is exhaustion treated differently from success?**
   Yes. `recursion_limit` exhaustion raises `GraphRecursionError` (a subclass of `RecursionError`) with the `ErrorCode.GRAPH_RECURSION_LIMIT` and a remediation message instructing the user to raise the limit (`errors.py:67-87`; `main.py:3017-3026, 3501-3507`). Clean completion returns the channel values. `GraphDrained` is a separate `GraphBubbleUp` exception (`errors.py:54-64`). `GraphInterrupt` (the `interrupt_before/after` and dynamic `interrupt()` exit) is also `GraphBubbleUp` and is suppressed at the top-level loop with the interrupt payload returned via `__interrupt__` (`_loop.py:1294-1355`). The prebuilt react agent's `remaining_steps` guard short-circuits *before* the cap fires, returning a polite "Sorry, need more steps" message instead of an exception (`prebuilt/chat_agent_executor.py:432-440, 620-634`).

4. **Are stuck loops detected before the hard limit?**
   No. There is no "no progress" detector, no hash/repetition check on channel state, no detection of repeated node sequences. The only pre-cap signal is the `RemainingSteps` managed value, which is read by application code (e.g., `create_react_agent`) on a voluntary basis — it is not enforced by the framework. A user-built cycle without that pattern will silently eat through `recursion_limit` steps before being killed.

5. **Does the user get a useful final state?**
   Mostly. On all termination paths:
   - The current checkpoint is flushed (`_suppress_interrupt` calls `_put_checkpoint(self.checkpoint_metadata)` and `_put_pending_writes()` at `_loop.py:1301-1311`). This means `get_state(config)` after the run returns the channel values at the point of termination.
   - For `recursion_limit` exhaustion: `GraphRecursionError` includes the configured `recursion_limit` value and an actionable instruction to raise it (`main.py:3017-3026`).
   - For drain: `GraphDrained(reason)` carries the `drain_reason` string (`errors.py:54-64`; `main.py:3027-3030`).
   - For `interrupt_before/after`: `__interrupt__` key in the stream and `state.values['__interrupt__']` carry the `Interrupt` tuple (`_loop.py:1315-1352`).
   - For dynamic `interrupt()`: the value passed to `interrupt(...)` is the `Interrupt.value`, surfaced via the same path.
   - **Caveat**: the *reason* (out_of_steps vs done vs draining) is not stored on the checkpoint itself. After a recursive failure the user must look at the *exception* to know why — a `get_state` after exhaustion returns the same `StateSnapshot` shape as a normal run. The `step` counter (`_loop.py:1108`) is in checkpoint metadata, so users can compare `step` to `recursion_limit` to detect exhaustion, but this is not surfaced as a first-class status field.

## Architectural Decisions

- **`recursion_limit` is a step counter, not a recursion counter.** Despite the name, it bounds the *number of supersteps per run*, not the recursion depth of subgraph calls (`_loop.py:1677`: `self.stop = self.step + self.config["recursion_limit"] + 1`). Subgraphs share the parent's recursion budget because they live under the same `tick()` driver.
- **Default of 10007 is "effectively unbounded for sane runs".** The env var exists (`_internal/_config.py:32`) precisely so deployments can lower the default; the high baseline avoids surprising users with a mid-run `GraphRecursionError`.
- **`recursion_limit` validation is `>= 1`, not `> 1`.** `main.py:2578-2579`. This makes `recursion_limit=1` a valid "do exactly one step then stop" knob, which the tests rely on (`test_pregel.py:574-575`).
- **Loop status is a typed literal, not a free-form string.** `_loop.py:249-257` declares seven mutually exclusive values, exhaustively handled in the post-stream exit checks (`main.py:3017-3030, 3508-3511`). Adding a new status would require updating those literals.
- **Termination is post-loop, not in-loop.** The runner executes inside `while loop.tick(): ...` (`main.py:2979`, `main.py:3452`); `out_of_steps` and `draining` are surfaced by `tick()` *returning False* but the exception is raised only after the `while` exits. The single-source-of-truth is `loop.status`.
- **Drain is single-attribute, not lock-protected.** `RunControl._drain_reason: str | None` (`runtime.py:90-104`). Docstring explicitly notes "Safe to call from any thread: the drain request is represented by a single attribute write, so no lock is needed for this signal. If more mutable state is added here, add synchronization." This is an intentional trade-off.
- **`NodeTimeoutError` deliberately does NOT subclass `TimeoutError`.** `errors.py:193-194`. The comment explains: "Does **not** inherit from the built-in `TimeoutError` (a subclass of `OSError`) so that the default `RetryPolicy` treats it as retryable."
- **Timeout cancellation cannot persist writes past the boundary.** `_TimedAttemptScope.close()` serializes writes so a cancelled background task cannot race a checkpoint commit (`_retry.py:128-152`).
- **Sync timeouts are compile-time rejected.** `validate_timeout_supported` raises `sync_timeout_unsupported` if a sync node is given a `timeout` (`_utils.py:135-137`, `main.py:934-935`). Reason: `time.sleep()` and other GIL-blocking work cannot be cancelled by `asyncio` (`_retry.py:_run_timeout_watchdog` is async-only).
- **Interrupt is a `GraphBubbleUp`, not a failure.** `GraphInterrupt(GraphBubbleUp)` (`errors.py:102-107`). The runner collects `GraphInterrupt` exceptions across siblings without marking them as failures (`_runner.py:683-690`); the loop's `_suppress_interrupt` swallows the top-level one (`_loop.py:1313-1352`).

## Notable Patterns

- **Layered budget model**: count (`recursion_limit`) + wall-clock per superstep (`step_timeout`) + wall-clock/idle per node attempt (`TimeoutPolicy`) + cooperative external signal (`RunControl`). Each layer catches failure modes the others miss.
- **Typed `Literal[...]` status field** instead of string constants — catches typo bugs at type-check time.
- **`_TimedAttemptScope` writes-then-progress** ordering: cancellation closes the scope *before* writes are persisted, preventing checkpoint writes from leaking past the timeout.
- **`versions_seen[INTERRUPT]` in `should_interrupt`** (`_algo.py:155-185`): prevents re-firing `interrupt_before`/`interrupt_after` for nodes that already produced writes since the last interrupt — an implicit "did anything change?" liveness check that doubles as a stuck-loop mitigation.
- **`ErrorCode` enum + `create_error_message`** (`errors.py:34-47`): produces stable error codes with a docs URL, so external monitoring / log search can match termination classes by `error_code` rather than parsing message strings.
- **`scratchpad.stop` exposed to nodes**: `IsLastStep` and `RemainingSteps` are managed values that read `scratchpad.step` and `scratchpad.stop` (`_internal/_scratchpad.py:8-19`; `managed/is_last_step.py:9-24`). Application code can opt into a soft stop *before* the framework's hard stop, without poking private fields.
- **`_suppress_interrupt` is registered via `ExitStack.push`** (`_loop.py:1674, 1294-1355`): the loop's exit path is a single context-manager hook that both finalizes the checkpoint and converts `GraphInterrupt` into a stream payload. This keeps "exit logic" out of the main `tick`/`after_tick` path.

## Tradeoffs

- **Hard cap on supersteps, not wall-clock per run.** A run with 10006 small steps and one slow step will be killed by `recursion_limit` before `step_timeout` fires; conversely a run with 10006 slow steps will exhaust `step_timeout` on each before `recursion_limit`. The two budgets are independent and don't interact.
- **No stuck-loop detection pre-cap.** An LLM that emits the same tool call every step will not be flagged — the framework will execute all `recursion_limit` steps before raising. `versions_seen[INTERRUPT]` only fires when `interrupt_before/after` is configured.
- **Termination reason not on the checkpoint.** `get_state()` after exhaustion returns the same shape as a clean run. The `step` field in checkpoint metadata is the only breadcrumb.
- **Drain check happens only at superstep boundaries** (`_loop.py:650-652`), not mid-superstep. A node that takes 60 seconds to run will not be cancelled by drain — the framework relies on `TimeoutPolicy` for that.
- **`recursion_limit` is inherited by subgraphs.** A deeply nested subgraph shares the parent's budget, so a misconfigured top-level cap can prematurely kill a long subgraph chain.
- **Default 10007 effectively means "no limit for sane runs"**, which is good for ergonomics but bad for catching runaway loops in production. The env var `LANGGRAPH_DEFAULT_RECURSION_LIMIT` is the only knob for hardening the default at deployment time.
- **`TimeoutPolicy` does not work on sync nodes.** Compile-time rejection (`_utils.py:135-137`) means users with sync infrastructure must either migrate to async or rely on `step_timeout`.

## Failure Modes / Edge Cases

- **`recursion_limit = 0` rejected with `ValueError`** (`main.py:2578-2579`). `recursion_limit = 1` is valid (do one step then stop) and used in tests as the cheapest way to trigger `GraphRecursionError`.
- **Drain requested during the final superstep finishes normally** — tests confirm `test_drain_requested_in_terminal_step_finishes_normally` (`tests/test_runtime.py:151-167`). The check is at the *top* of `tick()`, so a superstep already in progress will complete.
- **Drain requested from a subgraph** propagates to the parent (`test_drain_from_subgraph_can_resume_parent`, `tests/test_runtime.py:203-248`).
- **`step_timeout` and `ParentCommand` interaction**: `test_timeout_with_parent_command` confirms `Command(graph=Command.PARENT, ...)` is *not* swallowed by the step timeout — `ParentCommand` is a `GraphBubbleUp` so the runner's `_should_stop_others` ignores it (`_runner.py:616-634`).
- **Cancelled background tasks are surfaced as `NodeCancelledError`** (`errors.py:168-187`) — the docstring explains the distinction between framework-initiated cancellation (silent) and user-raised `asyncio.CancelledError` (surfaced as `NodeCancelledError` so it flows through the normal error path).
- **`NodeTimeoutError` retry behavior**: the default `RetryPolicy` retries it because it does not inherit `TimeoutError` (`errors.py:193-194`). Users who don't want this must explicitly opt out.
- **Sync work uncancellable under timeout**: `_utils.py:116-117` warns that idle-timeout enforcement on a sync runnable "will fire `NodeTimeoutError` correctly, but the background thread sync work is still uncancellable."
- **`recursion_limit` boundary check is `>` not `>=`**: `if self.step > self.stop` (`_loop.py:600`). With `recursion_limit=1` and starting step 0, `stop = 0 + 1 + 1 = 2`. Step 0 runs, step 1 runs, step 2 sees `step > stop` → `out_of_steps`. Two steps per "limit of 1" may surprise users, though tests rely on this.
- **No status persistence in checkpoint**: the `source` field in checkpoint metadata records `loop`/`input`/`fork`/`update` (`_loop.py:706, 959, 1016`) but does not record why the loop ended. A `state_snapshot` after exhaustion is indistinguishable from one after a clean stop.

## Future Considerations

- **Persist `loop.status` on the final checkpoint.** Adding a `termination_reason` field to checkpoint metadata would let `get_state` distinguish exhaustion from success without inspecting exceptions. The framework already has the typed literal (`_loop.py:249-257`) and the error-code system (`errors.py:34-47`) ready to be lifted into the checkpoint schema.
- **Add a "no progress" detector.** Hash channel state at the end of each superstep; if the last N supersteps produced identical channel hashes, raise early (or warn). This would catch LLM loops that emit the same output every step.
- **Cross-run stuck-loop detection.** A `recursion_limit` budget is per-run; a long-running thread that hits `recursion_limit` per invoke is invisible. Persisting the cumulative `step` count per `thread_id` would let the system cap total work across resumes.
- **Per-run wall-clock budget.** `step_timeout` is per-superstep. A top-level `run_timeout` (analogous to `TimeoutPolicy.run_timeout` but applied to the whole run) would close the gap where a "fast enough per step but many steps" run never triggers either cap.
- **Drain mid-superstep.** Currently drain is checked only at superstep boundaries (`_loop.py:650-652`). Polling inside the runner's `wait(..., timeout=...)` loop would shorten drain latency for slow nodes.
- **Make `IsLastStep` accessible to user-defined loop bounds.** Currently only `create_react_agent` consults `RemainingSteps`. Generalizing this — e.g., an `on_exhaustion` hook that runs with the final state — would let applications decide their own exit semantics without losing the framework's hard cap.

## Questions / Gaps

- No evidence found of a per-run wall-clock cap. `step_timeout` is per-superstep, `TimeoutPolicy.run_timeout` is per-node attempt. There is no documented `run_timeout` or `total_timeout` on `Pregel`. A search for `run_timeout` returns only the per-node policy field.
- No evidence found of a built-in "repetition" detector (hash-of-state, cosine similarity, exact-match repeats) for catching stuck LLM loops before `recursion_limit`.
- No evidence found of `recursion_limit` being a hard cross-resume budget. Each `invoke`/`stream` call gets a fresh budget; a long thread that hits `recursion_limit` per call has no aggregate cap.
- No evidence found of `step_timeout` or `TimeoutPolicy` propagating to async subgraphs as separate budgets — the parameter is set on the root `Pregel` instance (`main.py:816`) and propagated as-is. Test coverage exists for subgraph propagation (`test_pregel.py:8364-8377`), but it's unclear whether each level has independent budgets or a single shared clock.
- No evidence found of a public API to query "how close am I to `recursion_limit`?" from inside a node, beyond the `IsLastStep` / `RemainingSteps` managed values, which only fire on the *last* step (`managed/is_last_step.py:12`).
- The bench scripts set `recursion_limit=20000000000` (`libs/langgraph/bench/wide_state.py:156`, `wide_dict.py:144`, `sequential.py:39`, `react_agent.py:74`), confirming that the 10007 default is treated as the "no, really, no limit" choice. No evidence found of a documented "set recursion_limit to None for unlimited" path.

---

Generated by `dimensions/01.04-termination-and-loop-bounds.md` against `langgraph`.