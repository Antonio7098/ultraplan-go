# Source Analysis: langgraph

## 01.07 — Concurrency and Parallel Advancement

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (plus a JS/TS SDK in `libs/sdk-js`; this study only inspects Python) |
| Analyzed | 2026-07-03 |

## Summary

LangGraph implements concurrency through a strict **Pregel / Bulk Synchronous Parallel (BSP)** model: every step has a Plan, Execution, and Update phase, and within a step all selected actors run in parallel while channel writes remain invisible to peers (`libs/langgraph/langgraph/pregel/main.py:460-476`). The model is uniform across sync and async: a `Submit` callable (`libs/langgraph/langgraph/pregel/_executor.py:27-37`) plugs into either a `BackgroundExecutor` (thread-pool backed, `libs/langgraph/langgraph/pregel/_executor.py:40-119`) or an `AsyncBackgroundExecutor` (asyncio-event-loop backed, with optional `asyncio.Semaphore` gating, `libs/langgraph/langgraph/pregel/_executor.py:122-211`). The `PregelRunner` (`libs/langgraph/langgraph/pregel/_runner.py:135`) is the actual fan-out engine: it submits every executable task for a tick into the background executor and waits on `concurrent.futures.wait(... FIRST_COMPLETED)` (sync, `_runner.py:283-287`) or `asyncio.wait(... FIRST_COMPLETED)` (async, `_runner.py:482-486`), iterating through completion waves and yielding control back to the caller between them.

Concurrency is bounded per run by the config option `max_concurrency` (`libs/langgraph/langgraph/_internal/_config.py:199-228`), which feeds an `asyncio.Semaphore` only on the async path (`libs/langgraph/langgraph/pregel/_executor.py:135-140`). Sync has no built-in semaphore; back-pressure there comes from the single `concurrent.futures.wait` per tick and from `concurrent.futures.wait(self._delta_write_futs)` inside the loop's checkpointer step (`libs/langgraph/langgraph/pregel/_loop.py:1515-1517,1555-1558`). Ordering and determinism are guaranteed in two ways: (a) writes within a step are sorted by `task_path_str(t.path[:3])` before being applied (`libs/langgraph/langgraph/pregel/_algo.py:253-256`), and (b) `chain_future` (`libs/langgraph/langgraph/_internal/_future.py:82-140`) makes the user-observable future resolve only after `commit()` has run, so streaming/commit order is preserved (`_runner.py:784-786`).

Read-only work can clearly run fast (multiple tool calls fan out via `executor.map` or `asyncio.gather` inside `ToolNode`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:821-824` and `libs/prebuilt/langgraph/prebuilt/tool_node.py:855-858`), and stateful work is made safe by the BSP barrier: channel updates from step N are only visible in step N+1 (`libs/langgraph/langgraph/pregel/main.py:2974-2999`).

## Rating

**9/10 — Mature, durable, observable, extensible, proven under failure.**

The model is unambiguous (BSP with explicit Plan/Execution/Update), typed (the `Submit` protocol with `__cancel_on_exit__`/`__reraise_on_exit__`/`__next_tick__` flags, `FuturesDict` with monotonic counter), and exercised by an extensive test corpus (`test_parallel_node_execution`, `test_concurrent_emit_sends`, `test_max_concurrency`, `test_concurrent_execution_thread_safety`, `test_parallel_interrupts`, `test_graph_error_handler_does_not_swallow_interrupt_concurrent`, `test_external_drain_concurrent_*`, `test_pregel_validate_rejects_parallel_sync_branch_timeout`, etc.). The only meaningful gaps are (a) sync has no `max_concurrency` semaphore (only the async executor does), leaving sync back-pressure to the caller and `langchain_core.runnables.config.get_executor_for_config`, and (b) ordering of *streaming* events between concurrent tasks is explicitly documented as non-deterministic (`libs/langgraph/tests/test_pregel.py:1296`, `libs/langgraph/tests/test_pregel_async.py:2297`). Both are deliberate design points, not bugs, which keeps the rating at 9 rather than 10.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| BSP / step-level fan-out model | Pregel docstring describes three phases per step; sync and async drivers loop `while loop.tick(): for _ in runner.tick/atick(...): yield ...` | `libs/langgraph/langgraph/pregel/main.py:460-476`, `libs/langgraph/langgraph/pregel/main.py:2974-2999`, `libs/langgraph/langgraph/pregel/main.py:3446-3478` |
| Sync background executor | `BackgroundExecutor` wraps `concurrent.futures` thread pool via `get_executor_for_config`, supports `__next_tick__` (`time.sleep(0)`), and re-raises on exit | `libs/langgraph/langgraph/pregel/_executor.py:40-119`, `libs/langgraph/langgraph/pregel/_executor.py:220-222` |
| Async background executor | `AsyncBackgroundExecutor` uses `run_coroutine_threadsafe`; `max_concurrency` creates an `asyncio.Semaphore` that gates coroutines via `gated()` | `libs/langgraph/langgraph/pregel/_executor.py:122-211`, `libs/langgraph/langgraph/pregel/_executor.py:214-217`, `libs/langgraph/langgraph/pregel/_executor.py:135-140` |
| `Submit` callable contract | Protocol with `fn, *args, __name__, __cancel_on_exit__, __reraise_on_exit__, __next_tick__, **kwargs` returning a future | `libs/langgraph/langgraph/pregel/_executor.py:27-37`, `libs/langgraph/langgraph/pregel/_executor.py:54-75`, `libs/langgraph/langgraph/pregel/_executor.py:142-169` |
| Async → cross-loop bridge | `run_coroutine_threadsafe` + `chain_future` allow async-graph tasks from sync contexts and vice versa | `libs/langgraph/langgraph/_internal/_future.py:82-140`, `libs/langgraph/langgraph/_internal/_future.py:189-219` |
| Per-step fan-out engine | `PregelRunner.tick` schedules all tasks through `submit()`, waits on `FIRST_COMPLETED`, repeatedly yields control. Single-task fast path runs without the executor. | `libs/langgraph/langgraph/pregel/_runner.py:176-358` (and `_runner.py:201-251` for fast path), `libs/langgraph/langgraph/pregel/_runner.py:280-335` |
| Async per-step fan-out | `PregelRunner.atick` same as sync but uses `asyncio.wait(... FIRST_COMPLETED)` and `asyncio.wait_for(...)` for timeout | `libs/langgraph/langgraph/pregel/_runner.py:360-572` (esp. `_runner.py:479-486`, `_runner.py:545-549`) |
| Tracked futures + completion aggregation | `FuturesDict` subclasses `dict`, holds `lock=threading.Lock`, `event` (Event), `counter`, `done` set; `__setitem__` registers `add_done_callback` that decrements `counter` and sets `event` | `libs/langgraph/langgraph/pregel/_runner.py:75-132` |
| Failure → cancel siblings / panic | `_should_stop_others` returns True if any done future has a non-`GraphBubbleUp`/non-handled exception; `_panic_or_proceed` cancels `inflight`, raises or combines `GraphInterrupt` | `libs/langgraph/langgraph/pregel/_runner.py:616-697` |
| Parallel error handler routing | When a task fails in a fan-out and `node_error_handler_map` is configured, runner schedules the handler task in the same tick (no sibling cancel) | `libs/langgraph/langgraph/pregel/_runner.py:291-323`, `libs/langgraph/langgraph/pregel/_loop.py:1549-1584`, `libs/langgraph/langgraph/pregel/_loop.py:1803-1838` |
| Channel-level synchronization primitives | `Topic` (PubSub) and `NamedBarrierValue`/`NamedBarrierValueAfterFinish` provide join semantics across parallel branches | `libs/langgraph/langgraph/channels/topic.py:23-94`, `libs/langgraph/langgraph/channels/named_barrier_value.py:13-82` |
| Deterministic write application | `apply_writes` sorts tasks by `task_path_str(t.path[:3])` before applying | `libs/langgraph/langgraph/pregel/_algo.py:253-256` |
| Dynamic fan-out (Send) | `Send(node, arg)` enqueued on `__pregel_tasks` Topic channel; consumed in `prepare_next_tasks` per PUSH-tuple; each becomes an independent task running in the same step | `libs/langgraph/langgraph/types.py:664+`, `libs/langgraph/langgraph/pregel/_algo.py:440-466`, `libs/langgraph/langgraph/_internal/_constants.py:20,83-85` |
| Tool fan-out | Sync: `executor.map(_run_one, …)`; Async: `asyncio.gather(*coros)` over one coroutine per tool call | `libs/prebuilt/langgraph/prebuilt/tool_node.py:793-826` (sync), `libs/prebuilt/langgraph/prebuilt/tool_node.py:828-860` (async) |
| Concurrent streaming / waiter task | Loop installs a single `get_waiter()` future that yields when streaming produces output; only one waiter alive at a time | `libs/langgraph/langgraph/pregel/main.py:2951-2973`, `libs/langgraph/langgraph/pregel/main.py:3407-3478` |
| Ordering guarantee on user-observable future | `chain_future(src, dest)` used so `commit()` resolves before the future, preserving stream/commit order | `libs/langgraph/langgraph/pregel/_runner.py:783-786`, `libs/langgraph/langgraph/pregel/_runner.py:936-939`, `libs/langgraph/langgraph/_internal/_future.py:82-140` |
| Yielding to other work between completions | `__next_tick__=True` in sync wraps task in `next_tick` that calls `time.sleep(0)` so streaming-visibility callbacks and other threads can progress | `libs/langgraph/langgraph/pregel/_executor.py:64-71`, `libs/langgraph/langgraph/pregel/_executor.py:220-222`, `libs/langgraph/langgraph/pregel/_runner.py:774-781`, `libs/langgraph/langgraph/pregel/_runner.py:925-934` |
| Cancel-on-exit and re-raise-on-exit | Async executor cancels tasks with `__cancel_on_exit__`; both executors re-raise first non-`CancelledError` from tasks with `__reraise_on_exit__=True` | `libs/langgraph/langgraph/pregel/_executor.py:93-119`, `libs/langgraph/langgraph/pregel/_executor.py:186-211` |
| Checkpoint / delta-write serialization | Sync `concurrent.futures.wait(self._delta_write_futs)` before each checkpoint put; async `asyncio.gather(*futs)` | `libs/langgraph/langgraph/pregel/_loop.py:1515-1524`, `libs/langgraph/langgraph/pregel/_loop.py:1555-1558`, `libs/langgraph/langgraph/pregel/_loop.py:1769-1778`, `libs/langgraph/langgraph/pregel/_loop.py:1810-1812` |
| Step timeout | `runner.tick/atick(timeout=self.step_timeout, ...)` reserves a deadline in `end_time` and times out per `wait(..., timeout=...)` | `libs/langgraph/langgraph/pregel/main.py:2984`, `libs/langgraph/langgraph/pregel/main.py:3457`, `libs/langgraph/langgraph/pregel/_runner.py:280-289`, `libs/langgraph/langgraph/pregel/_runner.py:479-488`, `libs/langgraph/langgraph/pregel/main.py:726-770` |
| Node-level idle/run watchdogs | `arun_with_retry` uses `asyncio.wait({bg, *watchdogs}, return_when=FIRST_COMPLETED)` to enforce per-node timeouts during a parallel tick | `libs/langgraph/langgraph/pregel/_retry.py:460-509` |
| Tests: parallel node execution | Two-node parallel START test asserts wall time < sum of slowness | `libs/langgraph/tests/test_pregel.py:5273-5303`, `libs/langgraph/tests/test_pregel_async.py:6517-6547` |
| Tests: Send fan-out ordering acknowledged non-deterministic | Comments explicitly note non-deterministic ordering of concurrent outputs | `libs/langgraph/tests/test_pregel.py:1296-1300`, `libs/langgraph/tests/test_pregel.py:1361-1367`, `libs/langgraph/tests/test_pregel_async.py:2297-2300` |
| Tests: `max_concurrency` bound | 100 sends limited to 10 in flight via semaphore; assertable via counter on the node | `libs/langgraph/tests/test_pregel_async.py:3481-3609` |
| Tests: failure mid-fan-out + interrupt | `test_graph_error_handler_does_not_swallow_interrupt_concurrent` (sync + async) | `libs/langgraph/tests/test_retry.py:2197-2240`, `libs/langgraph/tests/test_pregel_async.py:9653-9700` |
| Tests: parallel interrupts | `test_parallel_interrupts`, `test_parallel_interrupts_double`, `test_overwrite_parallel_error` | `libs/langgraph/tests/test_pregel.py:7596, 7754, 9314, 9354`, `libs/langgraph/tests/test_interruption.py` |
| Tests: thread safety | `test_concurrent_execution_thread_safety` runs the same graph across 10 threads and asserts results are independent | `libs/langgraph/tests/test_pregel.py:5349-5384` |

## Answers to Dimension Questions

1. **What can run in parallel?**
   - Multiple **PregelNode actors** scheduled within a single step (per BSP) — `prepare_next_tasks` returns a `dict[id, task]` for all triggered PUSH (Send) and PULL tasks, then `runner.tick` submits them all (`libs/langgraph/langgraph/pregel/_algo.py:392-513`, `libs/langgraph/langgraph/pregel/_runner.py:176-358`).
   - The router-supplied **waiter future** (`get_waiter`) is also launched alongside tasks for eager streaming (`libs/langgraph/langgraph/pregel/main.py:2951-2973`, `libs/langgraph/langgraph/pregel/main.py:3407-3478`).
   - **Tool calls** within a single `ToolNode` invocation fan out: `executor.map` (sync, `libs/prebuilt/langgraph/prebuilt/tool_node.py:821-824`) or `asyncio.gather` (async, `libs/prebuilt/langgraph/prebuilt/tool_node.py:855-858`).
   - **Per-task background work** scheduled through `self.submit(...)` (e.g. cache writes in `put_writes`, `libs/langgraph/langgraph/pregel/_loop.py:1586-1602,1840-1859`).
   - **Cross-run parallelism**: each top-level `invoke`/`ainvoke` is its own loop with its own executor/semaphore; multiple invocations are independent processes to the framework.

2. **What must be serialized?**
   - **Channel writes within a step**: invisible to other tasks. Only applied at `after_tick()` (step boundary), per the BSP docstring (`libs/langgraph/langgraph/pregel/main.py:460-476`).
   - **Apply-writes ordering**: deterministic via `task_path_str(t.path[:3])` sort in `apply_writes` (`libs/langgraph/langgraph/pregel/_algo.py:253-256`).
   - **Commit / stream ordering on user-observable futures**: enforced via `chain_future` so that `commit()` (`_runner.py:574-613`) fires before the task's future resolves (`_runner.py:783-786`, `_internal/_future.py:82-140`).
   - **Single waiter future** at a time across the run (explicit comment, `libs/langgraph/langgraph/pregel/main.py:2958-2964`).
   - **Checkpoint put sequencing**: sync waits for `self._delta_write_futs` before `checkpointer.put`; async drains via `asyncio.gather` (`_loop.py:1515-1524,1555-1558,1769-1778,1810-1812`).
   - **Error handler task** for a failed task: scheduled after the failed task commits its `ERROR`/`ERROR_SOURCE_NODE` writes — explicit `await asyncio.gather(*futs)` / `concurrent.futures.wait(futs)` before preparing the handler task (`_loop.py:1549-1584,1803-1838`).

3. **How are shared resources protected?**
   - **Async Semaphore**: `AsyncBackgroundExecutor` creates `asyncio.Semaphore(max_concurrency)` when `config["max_concurrency"]` is set; coroutines are wrapped through `gated()` (`libs/langgraph/langgraph/pregel/_executor.py:135-140`, `:214-217`).
   - **Sync thread pool**: re-used from `langchain_core.runnables.config.get_executor_for_config`; no built-in semaphore; back-pressure is implicit because `runner.tick` does not submit the next wave until all current futures are done (or one fails/cancels) (`_runner.py:282-335`).
   - **Thread-safe futures tracking**: `FuturesDict.lock = threading.Lock` guards `counter` and `done` set mutations across completion callbacks (`_runner.py:97, 116-132`).
   - **Context propagation**: `run_coroutine_threadsafe(..., context=copy_context())` ensures `contextvars` (e.g. tracing, configurable) follow every task across the loop boundary (`_executor.py:155-166`, `_internal/_runnable.py:122-132`).
   - **Cancellation coordination**: tasks submitted with `__cancel_on_exit__=True` are cancelled on loop exit so awaiting user code does not leak (`_executor.py:101-104`, `:194-197`).
   - **Checkpointer ordering**: explicit gating on `_delta_write_futs` and `_error_handler_write_futs` ensures durable ordering of `DeltaChannel` writes and error-source markers before the next checkpoint (`_loop.py:1515-1524,1555-1558,1769-1778,1810-1812`).

4. **Are results ordered deterministically?**
   - **Yes for channel writes** within a step: sorted by task path before application (`_algo.py:253-256`).
   - **Yes for the user-observable future of each task** relative to its own writes: enforced via `chain_future` (`_runner.py:783-786,936-939`, `_internal/_future.py:82-140`).
   - **No for streaming emissions across parallel branches** — explicitly documented and asserted in tests (`tests/test_pregel.py:1296`, `tests/test_pregel_async.py:2297`).
   - **No for raw task completion order**: `concurrent.futures.wait(... FIRST_COMPLETED)` and `asyncio.wait(... FIRST_COMPLETED)` iterate as tasks finish, which is the point of the BSP barrier rather than a determinism violation (`_runner.py:283-287,482-486`).
   - **Yes at step boundary**: `after_tick()` runs `apply_writes` then `self._put_checkpoint(...)` synchronously, so the next step sees a consistent snapshot (`_loop.py:676-714`).

5. **What happens when one parallel branch fails?**
   - **Sync/async runner**: `_should_stop_others` (`_runner.py:616-634`) finds the failing future. Loop breaks (`_runner.py:330-333` / `:539-542`). `_panic_or_proceed` (`_runner.py:650-697`) cancels all `inflight` futures, then raises the first exception. **`GraphInterrupt`** is collected across all branches and combined into a single `GraphInterrupt(tuple(...))` raise (`_runner.py:683-690`).
   - **`GraphBubbleUp`**: explicitly excluded from "fail" — it is the inner-loop interrupt signal, not an error (`_runner.py:81-86, 622-633`).
   - **Graph-level error handler** (`node_error_handler_map`): failing tasks are routed to their configured handler node in the same tick; sibling tasks complete normally. `SKIP_RERAISE_SET` and `_handled_exception_ids` track which exceptions must not be re-raised (`_runner.py:169, 291-323, 616-633, 670-687`).
   - **Per-step timeout**: `step_timeout` causes `wait(... timeout=remaining)` to return empty `done`, exit the wave loop, and raise `TimeoutError("Timed out")`/`asyncio.TimeoutError("Timed out")` after cancelling inflight futures (`_runner.py:280-289, 479-488, 691-697`).
   - **Per-node idle/run timeout**: `_retry.py` watchdogs raise `NodeTimeoutError` and cancel `bg` (`_retry.py:460-509`).

## Architectural Decisions

- **BSP over dataflow** — single shared `dict[str, BaseChannel]` of channels is the synchronization substrate, mutated only at step boundaries (`libs/langgraph/langgraph/pregel/main.py:460-476`, `libs/langgraph/langgraph/pregel/_algo.py:232-345`). This eliminates an entire class of mid-step races that dataflow graphs must guard against. **Tradeoff**: latency cost of one synchronization barrier per step.
- **Unified `Submit` Protocol** with per-task keyword flags (`__cancel_on_exit__`, `__reraise_on_exit__`, `__next_tick__`) lets the `BackgroundExecutor` and `AsyncBackgroundExecutor` share one contract (`libs/langgraph/langgraph/pregel/_executor.py:27-37`). New primitives like error handler scheduling can opt into the right flags without changing the executor (`_runner.py:306-323`).
- **Async path uses `asyncio.Semaphore` for `max_concurrency`; sync path delegates to `langchain_core.runnables.config.get_executor_for_config` and does not implement its own concurrency bound.** This asymmetry is the only clear inconsistency in the model (`libs/langgraph/langgraph/pregel/_executor.py:135-140` vs the absence of one in `_executor.py:40-119`).
- **Yield-back to caller during the tick** via generator-based `tick`/`atick` (`yield  # give control back to the caller`, `_runner.py:199, 335, 341, 388, 544, 551`) keeps streaming latency low without threads.
- **`__next_tick__` flush workaround** — `time.sleep(0)` in the sync executor ensures that other threads (e.g. the main streaming thread) get scheduled before the task truly starts, so deferred writes from this tick don't bleed into the stream output order (`_executor.py:220-222`, inline comment at `_runner.py:774-777`).
- **`chain_future` everywhere**: bridges asyncio ↔ concurrent.futures to keep commit ordering and `add_done_callback` semantics working across executor boundaries (`_internal/_future.py:82-140`).
- **Per-step futures are tracked in `FuturesDict`** with a `threading.Event` (sync) / `asyncio.Event` (async), a counter, and a `should_stop` predicate injected by the runner — separation allows the runner to control when the superstep exits (`_runner.py:75-132, 616-634`).

## Notable Patterns

- **Pregel / BSP with per-step barriers** in `pregel/main.py:460-476` and `pregel/_loop.py:592-714`.
- **Two-tier executors**: outer `BackgroundExecutor` (runner level) and inner per-tool `executor.map` / `asyncio.gather` (tool level). Both reuse thread-pool-asyncio interop primitives from `langchain_core.runnables.config`.
- **Stateless task functions and dynamic `_TaskFunction`** in `libs/langgraph/langgraph/func/__init__.py:59-132` producing `SyncAsyncFuture` (`libs/langgraph/langgraph/pregel/_call.py:253-298`) so callers can write `futures = [mapper(i) for i in input]` then `f.result()` — implicit fan-out into PUSH tasks via `_runner.py:758-786`.
- **PUSH vs PULL tasks** with separate ids (`__pregel_push`/`__pregel_push` vs `__pregel_pull`, `_internal/_constants.py:83-85`) so the runner can distinguish dynamic Send targets from static edge-triggered nodes.
- **Deterministic ordering** of writes by `task_path_str(t.path[:3])` (`_algo.py:253-256`).
- **FuturesDict self-managed completion**: incremental counter and event-set wakeup avoids scanning the dict after every completion (`_runner.py:75-132`).

## Tradeoffs

- **One barrier per step** — channel writes cannot influence sibling tasks within the same step. This is intentional but does penalize workflows that need inter-task pipelining; users work around it by emitting intermediate states via `Send`/`Command(goto=Send(...))`.
- **Sync `max_concurrency` is not enforced** in the Pregel background executor; only the `langchain_core` thread pool limit applies (`_executor.py:50`). For deterministic sync back-pressure, callers must set `max_workers` on their thread pool.
- **Streaming order across branches is non-deterministic** — by design, because tasks complete in arbitrary order; tests `tests/test_pregel.py:1296`, `tests/test_pregel_async.py:2297` use `sorted(...)` to assert set membership rather than order.
- **`next_tick` workaround** (`time.sleep(0)`) is a thread-scheduling hack rather than an explicit notification primitive — it works because GIL/thread switching is quick enough, but it does not scale to thousands of concurrent tiny tasks.
- **`pregel/_runner.py:783-786`** each task's user-observable future is double-wrapped, which adds 1–2 callback hops and may matter in high-fan-out microbenchmarks.
- **`BackgroundExecutor` does not preempt long-running sync tasks** once submitted; only `step_timeout` and `NodeTimeoutPolicy` (`_retry.py`) provide cancellation, and they propagate best-effort.

## Failure Modes / Edge Cases

- **One of N parallel branches fails** → `_should_stop_others` cancels siblings, reraises (`_runner.py:616-697`); `GraphInterrupt` from one branch doesn't cancel siblings but is collected and re-raised (`_runner.py:683-690`).
- **Step timeout reached** → `wait(... timeout=0)` returns empty, breaker exits, then `_panic_or_proceed` cancels `inflight` and raises `TimeoutError("Timed out")` (`_runner.py:280-289, 691-697`).
- **Per-node idle timeout** (`TimeoutPolicy(idle_timeout=...)`) → `_retry.py:486-502` raises `NodeTimeoutError(kind="idle", ...)`.
- **Per-node run timeout** → raises `NodeTimeoutError(kind="run", ...)`.
- **Mid-step cancellation (e.g. client disconnect)** → `loop.submit` closes, cancels `__cancel_on_exit__` futures, lets the rest complete; `_suppress_interrupt` filter hides `GraphBubbleUp` from end users (`_executor.py:101-104, 194-197`, `_loop.py:??`).
- **Channel write conflict** (`Overwrite` reducer with multiple writers) → `InvalidUpdateError` from `LastValue`/`AnyValue` channels; covered by `test_overwrite_parallel_error` (`tests/test_pregel.py:9354`).
- **Failure in graph error handler concurrent with interrupt** — must still raise interrupt, not silently swallow; tested `test_graph_error_handler_does_not_swallow_interrupt_concurrent` (`tests/test_retry.py:2197-2240`, `tests/test_pregel_async.py:9653-9700`).
- **Empty `tasks`** — fast path returns immediately (`_runner.py:201-251`, `_runner.py:389-445`).
- **Single task with no waiter** — runs synchronously in the calling thread, not on the executor (`_runner.py:201-251`), which is a deliberate choice for latency.

## Future Considerations

- Add a sync-side semaphore gating the `BackgroundExecutor` thread pool to align sync and async back-pressure semantics (`libs/langgraph/langgraph/pregel/_executor.py:50,135`).
- Replace `time.sleep(0)` based `__next_tick__` with an explicit scheduling primitive that works better on platforms without preemption (none today, but the trick relies on Python's GIL).
- Provide a depth/breadth bound per `(step, node)` so a runaway `Send` cannot spawn unbounded PUSH tasks even when `max_concurrency` is permissive.
- Expose arrival-ordered interleave via `LangGraph.stream(... interleave=True, ...)` semantics already covered in `libs/langgraph/tests/test_interleave_arrival_order.py`, so users have ordered concurrent output when desired.
- Surface `FuturesDict.counter`/`done` as telemetry so external observers can graph "outstanding tasks" per step.
- Consider whether `asyncio.TaskGroup` could replace the bespoke `AsyncBackgroundExecutor` for a single superstep — the current model has the advantage of `__reraise_on_exit__=False` for sub-tasks scheduled mid-tick (`_runner.py:932`), which TaskGroup semantics do not match directly.

## Questions / Gaps

- What is the exact behavior when a sub-task scheduled from inside a parallel node (`_runner.py:759-781`) completes synchronously before the parent task has started streaming? The `chain_future` ordering comment claims it is preserved, but no direct test was found in the public test suite.
- Is `__next_tick__` strictly necessary in the async path where `run_coroutine_threadsafe` already schedules onto the event loop, or is it dead-weight? The `noop in async (always True)` comment (`_executor.py:149`) suggests the latter but does not document why it remains in the signature.
- The sync runner stops the superstep on first non-handled failure. Is there a documented knob to instead "wait for all" (e.g. partial success for human-in-the-loop review)? No clear evidence found beyond `node_error_handler_map` and `panic=` parameter.
- No explicit "structured concurrency" span is emitted. There is a `__name__` parameter on `submit()` (`_executor.py:32-33,147`) but no evidence it is currently surfaced to traces/langsmith as a logical span name; this is implicit.

---

Generated by `01.07-concurrency-and-parallel-advancement` against `langgraph`.
