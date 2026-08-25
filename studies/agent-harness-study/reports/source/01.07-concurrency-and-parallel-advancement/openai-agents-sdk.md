# Source Analysis: openai-agents-sdk

## 01.07 Concurrency and Parallel Advancement

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ on `asyncio`; OpenAI Agents SDK runtime + sandbox/MCP/voice/realtime subsystems |
| Analyzed | 2026-07-03 |

## Summary

Concurrency in `openai-agents-sdk` is **per-turn, single-loop, cooperative, and asyncio-native**, with a deliberately narrow but well-engineered parallel surface. The model call itself is **serial**: a turn awaits one model response (`run.py:1195-1217`, `run_loop.py:1019-1039`), but on either side of that single model hop the SDK does fan out:

- **Input guardrails on the first turn can opt into running in parallel with the model call** (`guardrail.py:100`, `run.py:775-778, 1194-1247`, `run_loop.py:677-678, 985-995`). The sequential subset is awaited before any side effects, and the parallel subset is started as a task that races the model call via `asyncio.gather`. A tripwire cancels the in-flight model task (`run.py:1231-1234`) unless Temporal replay compatibility demands otherwise (`agent_runner_helpers.py:153-168`).
- **Function-tool calls returned in one model response are scheduled as separate `asyncio.Task`s with a slot-filling concurrency limit** (`tool_execution.py:1358-1478`, `_fill_tool_task_slots` at `tool_execution.py:1438-1448`). The default is unbounded (every emitted tool call starts), gated by an optional `ToolExecutionConfig.max_function_tool_concurrency` (`run_config.py:99-118`). Hook start/end fire concurrently for each tool (`tool_execution.py:1757-1764, 1848-1855`).
- **Mixed tool classes (function / computer / custom / shell / apply_patch / local_shell) all run in one `asyncio.gather` fan-out per turn** (`tool_planning.py:580-624`). Computer, shell, and patch actions are *internally serial* within their own gather slot (`tool_execution.py:2007-2067, 2081-2092, 2107-2142`) — only function tools parallelize among themselves and against the other tool classes.
- **Manifest materialization in the sandbox uses a custom worker pool** (`materialization.py:23-78`) with a default cap of 4 manifest entries and 4 per-entry file copies (`run_config.py:34-35, 121-141`, `manifest_application.py:160-163, 135-178`, `entries/artifacts.py:203-206`). Overlapping destination paths force a flush to preserve deterministic ordering and avoid clobbering (`manifest_application.py:167-176`).
- **Output guardrails run in parallel** as a single gather and tripwire-cancel siblings (`guardrails.py:145-177`, `run_loop.py:371-388, 986-995`).
- **MCP server serializes requests with two `asyncio.Lock`s** (`mcp/server.py:597-598`) and reaps cleanup work behind a fire-and-forget task; **the docker sandbox uses an 8-worker `ThreadPoolExecutor` for blocking Docker socket I/O** (`sandbox/sandboxes/docker.py:81-84, 619, 828, 845, 941, 1023, 1051, 1174`); **the unix local sandbox uses `asyncio.to_thread` for fd close** (`sandbox/sandboxes/unix_local.py:569`) and `asyncio.Lock/Event` pairs for output synchronization (`sandbox/sandboxes/unix_local.py:115-137`).

The overall design is "bounded, observable, failure-isolated" — every parallel branch has a documented failure or cancellation path, and there are extensive tests for partial failure, cancelled siblings, and concurrency caps.

## Rating

**8 / 10** — Clear model with explicit interfaces, configurable caps, deliberate failure isolation, and tests for the dangerous interactions (cancelled siblings, queued-but-unstarted calls, parallel-guardrail tripwire cancelling the model task, deterministically-ordered output despite concurrency).

Not a 9-10 because: (1) the concurrency model is single-loop and tied to Python `asyncio`; (2) ordering inside the function-tool batch is preserved only by `asyncio.as_completed` reordering + an ordered `results_by_tool_run` dict (`tool_execution.py:312-341, 1462-1478`), which works but is non-obvious; (3) several subsystems use bare `asyncio.Lock` rather than typed mutex abstractions; (4) there is no unified semaphore/limit object — the worker pool in `materialization.py` is custom and not reused by the function-tool executor.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Function-tool parallel executor class | `_FunctionToolBatchExecutor` holds per-batch state, slots, and per-task order | `src/agents/run_internal/tool_execution.py:1358-1392` |
| Per-tool task creation | Each tool call becomes its own `asyncio.Task`; `_fill_tool_task_slots` enforces concurrency cap | `src/agents/run_internal/tool_execution.py:1438-1460, 1452-1458` |
| Drain loop for parallel function tools | `asyncio.wait(FIRST_COMPLETED)` + slot refill; failure triggers sibling drain | `src/agents/run_internal/tool_execution.py:1462-1478` |
| Failure arbitration | Priority: BaseException > Exception > CancelledError; ties broken by tool-call order | `src/agents/run_internal/tool_execution.py:243-289` |
| Cancellation drain with progress heuristics | Bounded self-progress wait using `_asyncio_progress` deadlines, capped at 64 immediate steps and 0.25s | `src/agents/run_internal/tool_execution.py:292-534, 160-162` |
| Post-invoke phase partition | Tasks in `in_post_invoke_phase` are not cancelled but awaited briefly (0.1s) to surface late failures | `src/agents/run_internal/tool_execution.py:1511-1546, 1825-1856` |
| `asyncio.shield` for invoke task | Protects tool body from external cancellation; cancellation handling for sibling failure | `src/agents/run_internal/tool_execution.py:1858-1893` |
| `ToolExecutionConfig` knobs | `max_function_tool_concurrency` default `None` (unbounded), `pre_approval_tool_input_guardrails` | `src/agents/run_config.py:95-118` |
| Validation of concurrency config | Raises `ValueError` for `max_function_tool_concurrency < 1` | `src/agents/run_config.py:112-118` |
| Concurrency-cap test (default unbounded) | 3 calls all run; ordering preserved by call_id | `tests/test_run_step_execution.py:238-270` |
| Concurrency-cap test (cap=2) | Active count never exceeds 2; output items stay in call order | `tests/test_run_step_execution.py:273-310` |
| Concurrency-cap + partial-failure test | Queued call does not start after sibling failure | `tests/test_run_step_execution.py:313-350` |
| Mixed tool-class parallel gather | Function / computer / custom / shell / apply_patch / local_shell fan out | `src/agents/run_internal/tool_planning.py:572-624` |
| Mixed tool isolation toggle | `isolate_function_tool_failures` auto-true when mixed; controls sibling cancel on failure | `src/agents/run_internal/tool_planning.py:562-571` |
| Mixed tool serial path | Parallel `False` falls back to sequential awaits | `src/agents/run_internal/tool_planning.py:625-680` |
| Computer / shell / apply_patch / local shell are internally serial | `for call in calls: await Action.execute(...)` | `src/agents/run_internal/tool_execution.py:2007-2067, 2081-2092, 2107-2142` |
| Input-guardrail parallel mode | `run_in_parallel=True` default; runner partitions sequential vs parallel | `src/agents/guardrail.py:100, 217-262` |
| First-turn guardrail partition | Sequential first, then parallel as task | `src/agents/run.py:770-779, 1172-1192` |
| First-turn guardrail partition (streaming) | Same partition applied in `start_streaming` | `src/agents/run_internal/run_loop.py:671-678, 985-995` |
| Input guardrails run as `asyncio.create_task` per guardrail | Tasks consumed by `asyncio.as_completed`, results pushed to queue | `src/agents/run_internal/guardrails.py:65-83` |
| Tripwire cancels siblings | All tasks `.cancel()` + `asyncio.gather(..., return_exceptions=True)` | `src/agents/run_internal/guardrails.py:74-83, 98-103, 129-139, 162-174` |
| Streaming output guardrail task | `_output_guardrails_task = asyncio.create_task(run_output_guardrails(...))` | `src/agents/run_internal/run_loop.py:363-388` |
| Parallel guardrail race with model call (sync) | `asyncio.gather(run_input_guardrails(parallel...), model_task)` | `src/agents/run.py:1195-1247` |
| Tripwire cancels in-flight model task | `model_task.cancel()` + `asyncio.gather(model_task, return_exceptions=True)` | `src/agents/run.py:1231-1234` |
| Compat flag for Temporal replay | `should_cancel_parallel_model_task_on_input_guardrail_trip()` honors `temporal_workflow.patched` | `src/agents/run_internal/agent_runner_helpers.py:153-168` |
| Parallel guardrail test (sync) | `test_parallel_guardrail_runs_concurrently_with_agent` | `tests/test_guardrails.py:339-369` |
| Parallel guardrail test (streaming) | `test_parallel_guardrail_runs_concurrently_with_agent_streaming` | `tests/test_guardrails.py:373-405` |
| Tripwire-cancels-model-task tests | `test_parallel_guardrail_trip_cancels_model_task` and `..._compat_mode_does_not_cancel_model_task` | `tests/test_guardrails.py:521-617` |
| Cancelled-sibling-with-final-output test | Parallel tool call with one cancelled sibling; remaining results are returned | `tests/test_agent_runner.py:621-680` |
| Concurrency stream dispatch task | `dispatch_task = asyncio.create_task(dispatch_stream_events())` | `src/agents/agent.py:829` |
| Tool enabled check fan-out | `asyncio.gather(*(_check_tool_enabled(t) for t in self.tools))` | `src/agents/agent.py:262` |
| Handoff enabled check fan-out | `asyncio.gather(*(check_handoff_enabled(h) for h in handoffs))` | `src/agents/run_internal/turn_preparation.py:106` |
| Mixed handoffs/tools fan-out (realtime) | `await asyncio.gather(*(_check_handoff_enabled(h) for h in handoffs))` | `src/agents/realtime/session.py:1403`, `src/agents/realtime/openai_realtime.py:425` |
| MCP `asyncio.gather` for multi-server connect | All servers tried, `return_exceptions=True` so one bad server doesn't block | `src/agents/mcp/manager.py:351-354` |
| MCP `asyncio.wait_for` on single call | `asyncio.wait_for(func(), timeout=timeout_seconds)` | `src/agents/mcp/manager.py:87` |
| MCP server locks | `_cleanup_lock: asyncio.Lock` and `_request_lock: asyncio.Lock` | `src/agents/mcp/server.py:597-598` |
| Manifest entry worker pool | `gather_in_order` uses N workers pulling from a shared `next_index` | `src/agents/sandbox/materialization.py:23-78` |
| Manifest entry batching | Overlapping destination paths force `_flush_parallel_batch()` | `src/agents/sandbox/session/manifest_application.py:135-178` |
| Concurrency limits (defaults) | `DEFAULT_MAX_MANIFEST_ENTRY_CONCURRENCY=4`, `DEFAULT_MAX_LOCAL_DIR_FILE_CONCURRENCY=4` | `src/agents/run_config.py:34-35` |
| Per-session concurrency limit storage | `_max_manifest_entry_concurrency` and `_max_local_dir_file_concurrency` on session | `src/agents/sandbox/session/base_sandbox_session.py:121-145` |
| Local-dir file copy fan-out | `gather_in_order(..., max_concurrency=session._max_local_dir_file_concurrency)` | `src/agents/sandbox/entries/artifacts.py:203-206` |
| Sandbox runtime helper parallel gather | `system_prompt, prompt_config = await asyncio.gather(...)` | `src/agents/run_internal/run_loop.py:1338, 1751` |
| `run_sync` runs on a long-lived default loop | Each thread keeps its loop; `run_sync` reuses it across calls | `src/agents/run.py:1580-1646` |
| Stream kickoff as background task | `streamed_result.run_loop_task = asyncio.create_task(start_streaming(...))` | `src/agents/run.py:1855-1873` |
| Stream queue producer/consumer | `asyncio.Queue` carries events; producer in streaming helpers | `src/agents/run_internal/streaming.py:30, 70` |
| `run_in_parallel` controls model side-effect | Setting propagates to provider as `parallel_tool_calls` flag | `src/agents/model_settings.py:69, 111`, `src/agents/models/openai_responses.py:739-744, 845`, `src/agents/models/openai_chatcompletions.py:432-437, 490` |
| Sandbox cleanup task | `asyncio.create_task(self._run_sandbox_cleanup())` after loop ends | `src/agents/result.py:604-621` |
| Docker sandbox thread pool | `_DOCKER_EXECUTOR: Final = ThreadPoolExecutor(max_workers=8, thread_name_prefix="agents-docker-sandbox")` | `src/agents/sandbox/sandboxes/docker.py:81-84` |
| Docker sandbox blocking I/O offload | `await loop.run_in_executor(_DOCKER_EXECUTOR, _write)` etc. | `src/agents/sandbox/sandboxes/docker.py:619, 828, 845, 941, 1023, 1051, 1174` |
| Unix local sandbox output synchronization | `output_lock: asyncio.Lock`, `output_notify/output_closed: asyncio.Event` | `src/agents/sandbox/sandboxes/unix_local.py:115-137` |
| Unix local sandbox fd close via thread | `asyncio.create_task(asyncio.to_thread(_close_fd_quietly, primary_fd))` | `src/agents/sandbox/sandboxes/unix_local.py:569` |
| Sync function tool offload | `asyncio.to_thread(the_func, ctx, *args, **kwargs_dict)` for sync tool bodies | `src/agents/tool.py:2004-2006` |
| SQLite session thread offloads | Every `_*_sync` method wrapped in `asyncio.to_thread` | `src/agents/memory/sqlite_session.py:255-342`, `src/agents/extensions/memory/advanced_sqlite_session.py:157-1357` |
| Realtime event handler tasks | `asyncio.create_task(self._handle_tool_call(...))` per event | `src/agents/realtime/session.py:1224-1310` |
| Voice pipeline dispatcher | `asyncio.create_task(self._dispatch_audio())` + `asyncio.gather(*self._tasks)` | `src/agents/voice/result.py:208-273` |
| Capability runtime-value awareness | `asyncio.Event`, `asyncio.Lock`, `asyncio.Semaphore` typed by `_is_capability_runtime_only_value` | `src/agents/run_state.py:2747-2759` |
| Memory storage / sink locks | `asyncio.Lock` for layout / flush / sink writes | `src/agents/sandbox/memory/storage.py:69, 97`, `src/agents/sandbox/memory/manager.py:59, 144, 186`, `src/agents/sandbox/session/sinks.py:182`, `src/agents/sandbox/runtime_session_manager.py:49` |

## Answers to Dimension Questions

1. **What can run in parallel?**
   - Multiple function-tool calls emitted in one model response, bounded by `ToolExecutionConfig.max_function_tool_concurrency` (`tool_execution.py:1358-1478`, `run_config.py:99-118`).
   - All six tool classes (function, computer, custom, shell, apply_patch, local_shell) in one `asyncio.gather` per turn when `_execute_tool_plan(parallel=True)` is used (`tool_planning.py:572-624`).
   - Input guardrails with `run_in_parallel=True` (the default) race the model call on the first turn (`run.py:1195-1247`, `run_loop.py:985-995`).
   - Output guardrails run in parallel after a final output (`guardrails.py:145-177`).
   - MCP servers connect concurrently with `return_exceptions=True` (`mcp/manager.py:351-354`).
   - Manifest materialization (default cap 4) and per-entry file copy (default cap 4) (`materialization.py:23-78`, `run_config.py:34-35`).
   - Hook start/end calls per tool — both `RunHooks` and `Agent.hooks` start in parallel (`tool_execution.py:1757-1764, 1848-1855`).
   - `asyncio.to_thread` offloads blocking I/O for sync tools, sync sessions, and Docker/unix sandbox I/O (`tool.py:2004-2006`, `sandbox/sandboxes/docker.py:81-84`).

2. **What must be serialized?**
   - The model call itself is one-at-a-time per turn (`run.py:1195-1247`, `run_loop.py:1019-1039`).
   - Computer, custom, shell, apply_patch, and local-shell calls are serial *within* their own per-class executor (`tool_execution.py:2007-2067, 2081-2092, 2107-2142`); the gather only fans across classes.
   - Sequential input guardrails (`run_in_parallel=False`) run before the model call starts (`run.py:775-792`).
   - Sandbox manifest entries with overlapping destination paths are serialized (`manifest_application.py:167-176`).
   - MCP server request and cleanup operations are guarded by `asyncio.Lock` (`mcp/server.py:597-598`).
   - Session writes inside a session's `add_items`/`get_items` are wrapped in `asyncio.to_thread` per call (so multiple coroutines can call them safely) but a single SQL operation is on one thread (`sqlite_session.py:255-342`).

3. **How are shared resources protected?**
   - **`asyncio.Lock`** for in-process event-loop locks: MCP request/cleanup (`mcp/server.py:597-598`), unix sandbox output buffer (`sandbox/sandboxes/unix_local.py:115-137`), sandbox memory layout / flush / sink / runtime session manager (`sandbox/memory/storage.py:69, 97`, `sandbox/memory/manager.py:59`, `sandbox/session/sinks.py:182`, `sandbox/runtime_session_manager.py:49`).
   - **Dedicated `ThreadPoolExecutor`** for blocking Docker SDK I/O (`sandbox/sandboxes/docker.py:81-84, 619, 828, 845, 941, 1023, 1051, 1174`).
   - **`asyncio.to_thread`** for sync tools, sync session methods, sync filesystem ops — CPython GIL serializes them at the Python level, but they release the event loop (`tool.py:2004-2006`, `memory/sqlite_session.py:255-342`).
   - **`asyncio.shield`** on the function-tool invoke task to prevent the body from being cancelled by external waiters while still allowing sibling-failure cancellation semantics (`tool_execution.py:1858-1893`).
   - **No `asyncio.Semaphore` is used in the core run path.** Concurrency caps are expressed as max-active counters inside the executor (`tool_execution.py:1438-1448`) or as a custom worker pool (`materialization.py:23-78`).
   - **`asyncio.Event`** used for unix sandbox output-notify / closed signaling (`sandbox/sandboxes/unix_local.py:116-117, 501`).

4. **Are results ordered deterministically?**
   - Function-tool outputs are written into a `dict[int, Any]` keyed by `id(tool_run)` (`tool_execution.py:1385, 1432-1433, 1932-1955`); the final list is built in the original `tool_runs` order (`tool_execution.py:1929-1970`). The drain loop records completed tasks in `task_state.order` and the test `test_function_tool_concurrency_cap_limits_calls_and_preserves_output_order` (`tests/test_run_step_execution.py:273-310`) asserts the items appear in call-id order despite concurrent execution.
   - Manifest materialization uses a *preallocated* result list indexed by task position (`materialization.py:33`), so the final `MaterializationResult.files` order matches the input order.
   - Hook start/end are awaited together per tool but tool order is preserved by the executor loop.
   - Input/output guardrails preserve insertion order via `asyncio.as_completed` iteration order being stored in a list (`guardrails.py:71-77, 105-107, 127-141, 162-175`).

5. **What happens when one parallel branch fails?**
   - **Function-tool failure:** the highest-priority failure wins (`_get_function_tool_failure_priority` at `tool_execution.py:243-249`), siblings are cancelled via `task.cancel()` with a bounded self-progress drain (`_drain_cancelled_function_tool_tasks`, `tool_execution.py:473-514, 1517-1531`). Remaining in-flight post-invoke tasks get a 0.1s window to surface failures via `_wait_pending_function_tool_tasks_for_timeout` (`tool_execution.py:517-533, 1533-1546`). Detached tasks get a loop-level `call_exception_handler` callback attached via `_attach_function_tool_task_result_callbacks` (`tool_execution.py:298-309, 1495-1502, 1548-1554`) so unobserved failures are still reported.
   - **Guardrail tripwire:** all sibling guardrail tasks are cancelled and awaited with `return_exceptions=True`; the originating exception is re-raised (`guardrails.py:74-83, 129-139, 162-174`). In the parallel-guardrail-vs-model race, the model task is also cancelled by default (`run.py:1231-1234`), unless Temporal replay compatibility demands otherwise (`agent_runner_helpers.py:153-168`).
   - **Mixed tool gather failure:** `isolate_function_tool_failures` (`tool_planning.py:562-571`) auto-enables when function tools are mixed with any other class; failures inside the function gather drain siblings without raising out of the other gathers unless the planning layer is told to.
   - **Manifest worker failure:** `gather_in_order` cancels pending workers and re-raises the first error (`materialization.py:48-73`).
   - **MCP connect failure:** `asyncio.gather(..., return_exceptions=True)` so a single bad server does not block others (`mcp/manager.py:354`).
   - **Sandbox output pump failure:** `asyncio.gather(*entry.pump_tasks, return_exceptions=True)` — exceptions are swallowed into the entry's state (`sandbox/sandboxes/unix_local.py:448, 576-578`).

## Architectural Decisions

- **Single-loop asyncio design.** Every parallel surface is `asyncio.Task` / `asyncio.gather` based; there is no `TaskGroup` adoption and no multiprocessing. This keeps the model run on a single thread of control and makes cancellation semantics easy to reason about.
- **Configurable cap, default unbounded for function tools.** `ToolExecutionConfig.max_function_tool_concurrency` defaults to `None`, meaning every function tool call from one model response starts immediately (`run_config.py:99-104`, `tool_execution.py:1439-1444`). The default is documented and explicit. This is the opposite of `manifest_entries=4` / `local_dir_files=4` defaults (`run_config.py:34-35`).
- **Two-tier guardrail scheduling.** `run_in_parallel=True` (default) gives latency wins; `run_in_parallel=False` gives deterministic pre-execution safety. Sequential guardrails are also re-run before sandbox prep to prevent state mutation on a tripwire (`run_loop.py:682-712`).
- **Deterministic output ordering despite concurrency.** Achieved by slot-filling + per-task order index + final list construction in input order (`tool_execution.py:1383-1386, 1438-1478, 1929-1970`).
- **Custom `gather_in_order` for ordered parallel materialization.** The team built their own `gather_in_order` (worker pool, max-concurrency, first-error cancel, order-preserving) instead of using `asyncio.gather` — because they need both bounded concurrency and deterministic ordering (`materialization.py:23-78`).
- **Function-tool invoke body shielded from cancellation, but with explicit `CancelledError` handling.** `asyncio.shield` protects the body, then the cancel handler inspects `teardown_cancelled_tasks` to decide whether to surface a late failure or re-raise (`tool_execution.py:1795-1823, 1858-1893`).
- **Realtime is fire-and-forget tasks.** Tool call / guardrail events in realtime sessions are dispatched as detached `asyncio.create_task` calls with no central `asyncio.gather` joining them (`realtime/session.py:1224-1310`).

## Notable Patterns

- **Slot-filling executor**: `_fill_tool_task_slots` + `_drain_pending_tasks` mimics a coroutine-friendly worker pool over a `set[asyncio.Task]` (`tool_execution.py:1438-1478`).
- **Priority + order failure arbitration**: the failure selector (`_select_function_tool_failure`, `_merge_late_function_tool_failure`) is a clean ordering invariant — `BaseException > Exception > CancelledError`, then tool-call order, then post-invoke precedence (`tool_execution.py:252-289`).
- **Best-effort task introspection** for cancel-aware drains via `_asyncio_progress` (private attrs, designed to fail safe by returning `None` when introspection is impossible) (`run_internal/_asyncio_progress.py:1-191`).
- **Lazy cap = 1 of locks** inside sandbox subsystems (e.g. `_pty_lock` in unix/docker sandboxes) so a single owner can be deterministic when needed.
- **Bounded fan-in via `as_completed`** with explicit cancel-and-drain for guardrail trips (`guardrails.py:65-83, 127-141`).
- **Compat flag for distributed-orchestration replay** (`should_cancel_parallel_model_task_on_input_guardrail_trip`, `agent_runner_helpers.py:153-168`) — a Temporal patch-id that decides whether cancelling the model task on a guardrail trip is replay-safe.

## Tradeoffs

- **Latency vs. determinism.** Parallel function tools with no cap trade determinism of *completion order* (whichever tool finishes first surfaces first in error states) for raw speed. The SDK compensates by sorting `results_by_tool_run` in input order before producing the final result list (`tool_execution.py:1929-1970`), so observable *output* is deterministic, but *side-effect* ordering (e.g. multiple tools writing to the same file) is the caller's responsibility.
- **Single-loop simplicity vs. CPU-bound work.** All concurrency is `asyncio`-bound. CPU-bound tool bodies (image processing, compression) would still block the loop unless the user wraps them in `asyncio.to_thread` themselves — which the SDK does for *sync* tool functions automatically (`tool.py:2004-2006`).
- **Custom worker pool vs. `asyncio.Queue` consumer.** `gather_in_order` reinvents what a bounded `asyncio.Queue` worker would do, gaining order-preservation but losing the `Queue`-native backpressure signaling. This is acceptable for short-lived materialization tasks but would not scale to a long-running queue.
- **`asyncio.Lock` vs. typed mutex.** Locks are bare `asyncio.Lock` instances stored in dataclass fields; there is no `RLock` and no named-lock context manager. Re-entrancy is not handled and a developer error can deadlock a session.
- **Unbounded default for function tools vs. bounded defaults for sandbox materialization.** The choice tracks "model-emitted tools" (predictable small N) vs. "user-supplied manifests" (unbounded N).

## Failure Modes / Edge Cases

- **Cancelled sibling with late final output**: handled by `test_parallel_tool_call_with_cancelled_sibling_reaches_final_output` (`tests/test_agent_runner.py:621-680`); the surviving call's output is preserved even though the other was cancelled.
- **Queued calls unstarted after sibling failure**: `test_function_tool_concurrency_cap_leaves_queued_calls_unstarted_after_failure` (`tests/test_run_step_execution.py:313-350`) — with `max_function_tool_concurrency=1`, a failure prevents a queued call from ever starting.
- **Parallel guardrail tripwire cancels model task**: `test_parallel_guardrail_trip_cancels_model_task` (`tests/test_guardrails.py:521-567`).
- **Slow-cancel siblings do not block streaming turn**: `test_parallel_guardrail_trip_with_slow_cancel_sibling_stops_streaming_turn` (`tests/test_guardrails.py:722-785`).
- **Compatibility mode does NOT cancel the model task** when running under a Temporal workflow without the patch: `test_parallel_guardrail_trip_compat_mode_does_not_cancel_model_task` (`tests/test_guardrails.py:568-617`).
- **Late `BaseException` post-invoke**: explicitly raised as a propagated failure via `_merge_late_function_tool_failure` with `post_invoke` precedence (`tool_execution.py:271-289, 1504-1509`).
- **Stale `run_sync` running on a torn-down loop**: `run_sync` checks for a running loop and refuses to call from one (`run.py:1592-1598`); it also explicitly *keeps* the default loop open between sync calls to preserve `asyncio.Lock` affinity (`run.py:1604-1611`).
- **Sandbox cleanup after the run loop**: `asyncio.create_task(self._run_sandbox_cleanup())` is attached as a done-callback on the run task, so cleanup runs even if the loop raises (`result.py:604-621`).

## Future Considerations

- **No `asyncio.TaskGroup` adoption.** A `TaskGroup` would give stronger failure-coverage guarantees than the manual cancel-and-drain pattern in `_FunctionToolBatchExecutor`. The SDK has its own fail-arbiter, but the price is manual cancel correctness.
- **No general semaphore utility.** Caps are scattered: `ToolExecutionConfig.max_function_tool_concurrency`, `SandboxConcurrencyLimits.manifest_entries`, `SandboxConcurrencyLimits.local_dir_files`. A typed `BoundedExecutor` would unify these.
- **No observability for in-flight tool tasks.** Tracing captures start/end but not "currently 3 of 5 tool tasks running, 2 queued." A small metric hook would help diagnose stalls in long-tail tool bodies.
- **`asyncio.shield` semantics for tool bodies** are subtle; if a user awaits something *outside* the SDK inside their tool, the shield does not protect the outer await. This is documented behavior but worth a guidance doc.
- **Realtime path is largely fire-and-forget tasks** (`realtime/session.py:1224-1310`). There is no `asyncio.gather` to await the side-effects of a turn; a future iteration could centralize them for testability.

## Questions / Gaps

- **No evidence of a public "drain on shutdown" hook** for the function-tool executor. If a host process wants to flush in-flight tools before exit, the public API does not expose a flush-and-wait method; the SDK relies on its own teardown callbacks (`tool_execution.py:1548-1554`).
- **No centralized cancel-on-shutdown orchestration.** Each subsystem has its own lock and its own teardown path. There is no documented "graceful shutdown sequence" that orders sandbox cleanup, MCP disconnect, and session flush.
- **No measurement of in-flight tool concurrency from tracing.** `_FunctionToolBatchExecutor` has a `pending_tasks` set but it is not exposed in any span attribute.
- **No async generator / async context manager patterns for parallel stages.** Every parallel stage is `gather` + manual list assembly; a future refactor toward `async with BoundedExecutor()` could simplify ownership of partial-failure state.
- **No `asyncio.Semaphore` example in the public docs** for users who want to bound *their* concurrent tool internals. The SDK only uses semaphores for its own subsystems; users have to roll their own.
- **`agent_runner_helpers.py:153-168`** explicitly couples cancellation policy to the *presence* of `temporalio`. This is a narrow but real dependency; if Temporal isn't installed the function falls through to `return True`, so users under other workflow engines could get surprising behavior. No evidence of a generic hook for other workflow engines.
- **Docker `_DOCKER_EXECUTOR` is a module-level global** with no shutdown hook (`sandbox/sandboxes/docker.py:81-84`). If the process exits while the executor has running futures, they may not complete. No evidence of an `atexit`/`loop.shutdown_default_executor` integration.

---

Generated by `dimensions/01.07-concurrency-and-parallel-advancement.md` against `openai-agents-sdk`.
