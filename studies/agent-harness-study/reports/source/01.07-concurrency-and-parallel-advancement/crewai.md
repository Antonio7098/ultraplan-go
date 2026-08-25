# Source Analysis: crewai

## Concurrency and Parallel Advancement (01.07)

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (asyncio + threading, with Pydantic models, FastAPI-style background-event-loop bus) |
| Analyzed | 2026-07-03 |

## Summary

CrewAI runs on a dual execution model: a cooperative `asyncio` event loop for I/O-bound work (LLM calls, A2A streaming, tool wrappers) layered on top of a `concurrent.futures.ThreadPoolExecutor` for blocking CPU/IO work (file and LanceDB writes, parallel tool execution, parallel memory searches, agent-card fetches). Parallel advancement happens at three scopes: (a) inside one crew run when tasks carry `async_execution=True` and are gathered via `asyncio.create_task` (`crew.py:1360-1390`), (b) across runs through `kickoff_for_each[_async]` which schedules N crew copies and gathers their outputs (`crews/utils.py:466-504`), and (c) inside the Flow runtime, where multiple `@start` methods and racing/non-racing listeners are dispatched with `asyncio.gather` and `asyncio.as_completed` + cancellation (`flow/runtime/__init__.py:1399-1429`, `2494-2566`). Resource protection uses a self-implemented reader-writer `RWLock` (`utilities/rw_lock.py:12-81`) on the singleton event-bus handler dict plus a `threading.RLock` for singleton init (`events/event_bus.py:117-152`), per-instance `threading.RLock` for shared mutable state (`memory/unified_memory.py:172-322`), `asyncio.Lock` for OAuth2 token-fetch races (`a2a/auth/client_schemes.py:339-372`), and a process-coordinating `lock_store` (Redis or file `portalocker`) for multi-process safety (`crewai_core/lock_store.py:79-121`). Parallel failures are collected in three idioms: `asyncio.gather(*, return_exceptions=True)` is the predominant shape (event bus, fans-out runs, parallel tool execution, racing listeners, parallel todos), `concurrent.futures.as_completed` writes results into an index-keyed `ordered_results` list (parallel tool execution, agent-card fetch, parallel memory searches), and per-future `try/except` with logger fallback (`memory/recall_flow.py:127-172`). Partial success is preserved at every fan-out (e.g., `experimental/agent_executor.py:1216-1268` lets one failed todo fail that branch without cancelling siblings).

## Rating

**8 / 10 — Clear, tested model with explicit interfaces and operational safeguards, but spotty documentation and one skipped multi-process stress test keep it short of mature-and-proven.**

Rationale:

- (+) Explicit asyncio + threadpool split with consistent patterns (`asyncio.gather(return_exceptions=True)`, `index-aligned ordered_results`, RW-lock on bus, RLock for re-entrant storage writes).
- (+) Async-racing listeners with first-wins cancellation (`flow/runtime/__init__.py:1380-1429`) and async start-method fan-out (`2494-2502`).
- (+) Bounded pool sizes (event bus 10 workers, tool execution `min(8, n)`, memory save `max_workers=1` to serialize, memory search `min(n, 8)`).
- (+) Order-preserving and failure-isolating via `gather(..., return_exceptions=True)` plus a dedicated comment annotating the invariant (`experimental/agent_executor.py:1221-1223`).
- (+) Validation guardrails at crew-build time: `validate_end_with_at_most_one_async_task` (`crew.py:767-786`), `validate_async_tasks_not_async`, `validate_async_task_cannot_include_sequential_async_tasks_in_context` (`crew.py:814-851`).
- (−) `tests/memory/test_concurrent_storage.py:11-13` is **skip-marked** (`pytest.mark.skip(reason="Multiprocessing tests incompatible with xdist...")`); multi-process storage races are therefore unproven.
- (−) Concurrency primitives are scattered: `_state_lock`, `_or_listeners_lock`, `_usage_metrics_lock` (`flow/runtime/__init__.py:995-1008`), separate `_pending_lock` / `_reset_lock` / `_save_pool` for memory (`memory/unified_memory.py:165-172`); no single concurrency guide.
- (−) Singleton event bus ships an explicit singleton with RLock + double-check, but parallel crews share it via `crewai_event_bus` (`test_crew_thread_safety.py:130-168`) — concurrent-kickoff context-bleed protection relies on `crew.copy()` and OpenTelemetry context-var scoping, which is enforced but not separately documented.
- (−) `kickoff_async` historically wraps `kickoff` in `asyncio.to_thread` (`crew.py:1143, 1162`); newer `akickoff` is the native path (`1190-1279`) but both exist — risk of partial migration.

## Evidence Collected

Every entry includes a `path/to/file.py:NN` citation.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Native async kickoff path; async task fan-out in sequential/hierarchical processes | `akickoff` switches on `Process.sequential`/`Process.hierarchical`; `_aexecute_tasks` accumulates `pending_tasks` and awaits them; supports `async_execution` | `lib/crewai/src/crewai/crew.py:1190-1279`, `lib/crewai/src/crewai/crew.py:1308-1390` |
| Sync `kickoff_async` path delegates to thread | `kickoff_async` wraps `self.kickoff` in `asyncio.to_thread` (legacy adapter) | `lib/crewai/src/crewai/crew.py:1130-1162` |
| Async `kickoff_for_each_async` fans out N crews via gather | `crew_copies = [crew.copy() for _ in inputs]`; `asyncio.create_task` per copy; `asyncio.gather(*async_tasks)` aggregates | `lib/crewai/src/crewai/crews/utils.py:440-504` |
| Same fan-out, streaming branch | `asyncio.gather(*[consume_stream(s) for s in streaming_outputs])` re-uses the same gather primitive | `lib/crewai/src/crewai/crews/utils.py:453-469` |
| Sync-task fan-in via per-future await + result list (preserves input order) | Iterates `pending_tasks` and awaits each `async_task`; `_process_task_result` runs sequentially | `lib/crewai/src/crewai/crew.py:1411-1425`, `lib/crewai/src/crewai/crew.py:1568-1598` |
| Per-task `Future` in the sync path | `Task.execute_async` spawns daemon `threading.Thread`; caller awaits `Future` | `lib/crewai/src/crewai/task.py:596-625` |
| Async LLM dispatch (provider-level concurrency unit) | All provider base classes expose `async def acall(...)` plus `_ahandle_streaming_*` | `lib/crewai/src/crewai/llm.py:1335-1388`, `lib/crewai/src/crewai/llms/providers/openai/completion.py:489-636`, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:351-405`, `lib/crewai/src/crewai/llms/providers/gemini/completion.py:359-414`, `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:462-569` |
| Parallel native tool calls inside an agent turn (bounded) | `_should_parallelize_native_tool_calls` rejects batches with `result_as_answer` or `max_usage_count`; `ThreadPoolExecutor(max_workers=min(8, n))` with `as_completed` writes to `ordered_results[idx]` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:695-784`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:1703-1746` (parallel block in experimental agent executor) |
| Same pattern in experimental agent executor | Parallel tool calls rejected for `result_as_answer=True`, executed in pool with `as_completed` and ordered results | `lib/crewai/src/crewai/experimental/agent_executor.py:1703-1746` |
| Sequential fallback for tool batches | Comment: "Execute sequentially so result_as_answer tools can short-circuit" | `lib/crewai/src/crewai/experimental/agent_executor.py:1746-1747` |
| Parallel todo execution in planning | `router("multiple_todos_ready")` => `asyncio.gather(... return_exceptions=True)`; explicit comment documents ordering | `lib/crewai/src/crewai/experimental/agent_executor.py:1185-1269` |
| Flow listener fan-out | `_execute_racing_listeners` uses `asyncio.create_task` for every racing + non-racing listener; first-wins via `asyncio.as_completed` + cancel; non-racers via `gather(..., return_exceptions=True)` | `lib/crewai/src/crewai/flow/runtime/__init__.py:1380-1429` |
| Flow start-method fan-out | When no sequenced start order, kicks off multiple `@start` methods in parallel via `asyncio.gather` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2480-2502` |
| Flow trigger re-dispatch on listener fire | When a trigger has multiple listeners and no racing group, dispatches them in parallel via `asyncio.gather` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3070-3095` |
| Flow event-future await | After kickoff, awaits in-flight event-bus futures before completing | `lib/crewai/src/crewai/flow/runtime/__init__.py:1743-1751`, `2543-2566` |
| Sync Flow methods run on a threadpool | `asyncio.to_thread(ctx.run, method, *args, **kwargs)` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2829-2835` |
| Sync Flow entrypoint (kickoff) spins a thread-isolated event loop | Synchronous `kickoff` runs `asyncio.run(_run_flow())` inside a one-worker pool when an event loop is already running | `lib/crewai/src/crewai/flow/runtime/__init__.py:2197-2207` |
| Flow runtime uses per-instance `threading.Lock`s and locked proxies | `_state_lock`, `_or_listeners_lock`, `_usage_metrics_lock`; `LockedListProxy`/`LockedDictProxy` to bracket state mutation; deadlock-free ordering via id comparison | `lib/crewai/src/crewai/flow/runtime/__init__.py:368-619`, `995-1008`, `1256-1284` |
| Flow input ask() uses bounded pool with non-blocking shutdown | `ThreadPoolExecutor(max_workers=1)`, `executor.shutdown(wait=False, cancel_futures=True)` to avoid deadlock when provider outlives timeout | `lib/crewai/src/crewai/flow/runtime/__init__.py:3379-3404` |
| Memory parallel save (serialized) | `_save_pool = ThreadPoolExecutor(max_workers=1)`; `_pending_saves` list with `_pending_lock`; `_reset_lock` (`RLock`) | `lib/crewai/src/crewai/memory/unified_memory.py:165-322`, `1015-1035` |
| Memory parallel find_similar (concurrent) | `ThreadPoolExecutor(max_workers=min(len(active), 8))`; per-future `try/except` returns `[]` on failure | `lib/crewai/src/crewai/memory/encoding_flow.py:152-219` |
| Memory parallel_analyze | `ThreadPoolExecutor(max_workers=10)`; bounded parallelism for analyze_for_save + analyze_for_consolidation; per-item failure isolation | `lib/crewai/src/crewai/memory/encoding_flow.py:222-345` |
| Memory recall parallel search | `ThreadPoolExecutor(max_workers=min(len(tasks), 4))` keyed by `(embedding, scope)` | `lib/crewai/src/crewai/memory/recall_flow.py:87-176` |
| A2A agent-card parallel fetch (sync via threadpool) | `ThreadPoolExecutor(max_workers=min(len(...), 10))` + `as_completed` writing into `agent_cards[endpoint]` or `failed_agents[endpoint]` | `lib/crewai/src/crewai/a2a/wrapper.py:262-299` |
| A2A agent-card parallel fetch (async) | `await asyncio.gather(*[_afetch_card_from_config(c) for c in a2a_agents])` | `lib/crewai/src/crewai/a2a/wrapper.py:1436-1460` |
| A2A cancel-watcher races task against cancellation | `asyncio.wait([execute_task, cancel_watch], return_when=asyncio.FIRST_COMPLETED)` and cancels the loser | `lib/crewai/src/crewai/a2a/utils/task.py:147-193` |
| Singleton event bus: dedicated background event loop + bounded sync pool | Thread `CrewAIEventsLoop` daemon; `ThreadPoolExecutor(max_workers=10, thread_name_prefix="CrewAISyncHandler")` for sync handlers | `lib/crewai/src/crewai/events/event_bus.py:165-190` |
| Event-bus handler concurrency: dependency-tiered dispatch | Builds `ExecutionPlan` (level-by-level dependencies). Per level: sync handlers go to the pool; async handlers via `asyncio.gather(return_exceptions=True)` | `lib/crewai/src/crewai/events/event_bus.py:444-521` |
| Event-bus reader/writer lock | Custom `RWLock` (`utilities/rw_lock.py`); reader-preferring; writer-prioritized reentry via id-order deadlock avoidance | `lib/crewai/src/crewai/utilities/rw_lock.py:12-81` |
| Event-bus futures tracking + flush | `_pending_futures` set, `_futures_lock`; `flush()` uses `concurrent.futures.wait` for sync drain; `aemit` runs async handlers via `await self._acall_handlers` | `lib/crewai/src/crewai/events/event_bus.py:192-209`, `732-795` |
| Streaming chunk order | `LLMStreamChunkEvent` is special-cased — sync handlers run on the caller thread to preserve ordering (no fire-and-forget) | `lib/crewai/src/crewai/events/event_bus.py:504-518`, `626-633` |
| OAuth2 token race protection | `asyncio.Lock` ensures only one coroutine fetches tokens at a time when several concurrent requests share the auth scheme | `lib/crewai/src/crewai/a2a/auth/client_schemes.py:317-373`, `402-478` |
| OAuth2 refresh path also locked | Same `asyncio.Lock` guards refresh branch to avoid duplicate refresh | `lib/crewai/src/crewai/a2a/auth/client_schemes.py:470-478` |
| Singleton Telemetry with class-level lock | `_lock: ClassVar[threading.Lock] = threading.Lock()` enforces double-checked init | `lib/crewai-core/src/crewai_core/telemetry.py:75-84` |
| RPM (per-minute rate limit) shared controller with internal lock | `threading.Lock` guards counter increment + reset; shared across agents via `set_rpm_controller` to serialize rate-limited calls across tasks/agents | `lib/crewai/src/crewai/utilities/rpm_controller.py:12-89`, `lib/crewai/src/crewai/crew.py:634-635`, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:773-780` |
| Cross-process lock backend | `crewai_core.lock_store.lock(...)` chooses between `portalocker.RedisLock` (distributed) and file-based `portalocker.Lock`; pluggable via `set_lock_backend` | `lib/crewai-core/src/crewai_core/lock_store.py:79-121` |
| File-handler cross-process serialization | `with store_lock(f"file:{realpath}")` wraps read/write/append paths | `lib/crewai/src/crewai/utilities/file_handler.py:88, 150, 163` |
| LanceDB cross-process serialization | Every mutating method wraps `with store_lock(self._lock_name)`; `_do_write` retries LanceDB commit conflicts with exponential backoff | `lib/crewai/src/crewai/memory/storage/lancedb_storage.py:100, 113, 227, 305, 330, 351, 421, 604, 630` |
| Background LanceDB compaction on a daemon thread | `threading.Thread(target=ctx.run, args=(self._compact_safe,), daemon=True, name="lancedb-compact")` | `lib/crewai/src/crewai/memory/storage/lancedb_storage.py:213-221` |
| Checkpoint handler runs on event-bus pool (explicit comment) | "The event bus runs sync handlers in a `ThreadPoolExecutor`, so blocking I/O is safe" | `lib/crewai/src/crewai/state/checkpoint_listener.py:247-270` |
| Tool usage lock for shared counters | `BaseTool._usage_lock` guards current_usage_count mutation | `lib/crewai/src/crewai/tools/base_tool.py:192` |
| Cache handler uses RWLock for shared dict | `_lock: RWLock = PrivateAttr(default_factory=RWLock)` | `lib/crewai/src/crewai/agents/cache/cache_handler.py:7-21` |
| Event record lock | `event_record._lock: RWLock` | `lib/crewai/src/crewai/state/event_record.py:14-108` |
| Tracing batch manager uses Condition + multiple locks | `_init_lock`, `_pending_events_lock`, `_finalize_lock` (Condition at line 61) | `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:61-66` |
| Background LLM summarization (chunks run concurrently) | `_asummarize_chunks` uses `asyncio.gather(*[_summarize_one(...)])`; sync wrapper bridges nested event loop via `ThreadPoolExecutor(max_workers=1)` | `lib/crewai/src/crewai/utilities/agent_utils.py:884-991` |
| A2A streaming/polling handler polls in event loop | `await asyncio.sleep(polling_interval)` etc. for cooperative scheduling | `lib/crewai/src/crewai/a2a/updates/polling/handler.py:120`, `lib/crewai/src/crewai/a2a/updates/streaming/handler.py:139` |
| Sync ↔ async bridge for file store | `_run_sync` runs coroutine in a one-worker pool if a loop is running; `asyncio.run` if not | `lib/crewai/src/crewai/utilities/file_store.py:36-55` |
| Lite agent delegates to thread | `await asyncio.to_thread(...)` for `kickoff_async` | `lib/crewai/src/crewai/lite_agent.py:750-768` |
| Streaming async chunk generator creates a task; cancellation is permitted | `asyncio.create_task(run_coro())`, `except asyncio.CancelledError` swallowed | `lib/crewai/src/crewai/utilities/streaming.py:298-338` |
| Crew end-of-run parallel futures await | `await asyncio.gather(*[asyncio.wrap_future(f) for f in self._event_futures])` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2543-2550`, `2566-2569` |
| Tool-wrapping utility (async) | `async def aexecute_tool_and_check_finality(...)` | `lib/crewai/src/crewai/utilities/tool_utils.py:30-118` |
| Tests asserting concurrent execution (not just thread-safety) | `tests/agents/test_async_agent_executor.py:280-288`, `tests/tools/test_async_tools.py:196` | `lib/crewai/tests/agents/test_async_agent_executor.py:280-288`, `lib/crewai/tests/tools/test_async_tools.py:196` |
| Tests for thread-safety with N crews via ThreadPoolExecutor + asyncio.gather | `test_parallel_crews_thread_safety`, `test_async_crews_thread_safety`, `test_concurrent_kickoff_for_each` | `lib/crewai/tests/test_crew_thread_safety.py:46-225` |
| Multi-process stress test for storage | Skipped via `pytestmark = pytest.mark.skip` | `lib/crewai/tests/memory/test_concurrent_storage.py:11-13` |
| Skip-list of `result_as_answer` / `max_usage_count` tools | Same `force sequential` rationale as parallel tool execution | `lib/crewai/src/crewai/agents/crew_agent_executor.py:683-723` |

## Answers to Dimension Questions

1. **What can run in parallel?**
   - Tasks within a single crew run when `task.async_execution=True` (validated at build time). `crew.py:1360-1390` enqueues each into `pending_tasks` (`asyncio.Task[TaskOutput]`) and flushes them in order. Native async kickoff (`akickoff`) is the preferred path; legacy `kickoff_async` runs the sync path in `asyncio.to_thread` (`crew.py:1143`).
   - Multiple crews via `Crew.kickoff_for_each[_async]` (and `akickoff_for_each`): one crew is `crew.copy()`'d per input, each kicked off, then gathered (`crews/utils.py:440-495`).
   - Multiple `@start` methods on a Flow (when no sequential order is configured) — gathered via `asyncio.gather(*tasks)` (`flow/runtime/__init__.py:2494-2502`).
   - Multiple Flow listeners per trigger — racing listeners race first-wins, others run concurrently (`flow/runtime/__init__.py:1380-1429`, `3070-3095`).
   - Multiple native tool calls in one agent turn — bounded ThreadPoolExecutor capped at 8, unless a tool is `result_as_answer=True` or has `max_usage_count` (`agents/crew_agent_executor.py:695-784`, `experimental/agent_executor.py:1703-1746`).
   - Multiple ready todos in the experimental agent planner — bounded by `gather(..., return_exceptions=True)`, each todo runs `step_executor.execute` in `asyncio.to_thread` (`experimental/agent_executor.py:1185-1268`).
   - A2A agent-card fetch — sync via `ThreadPoolExecutor(max_workers=min(N, 10))` (`a2a/wrapper.py:262-299`), async via `asyncio.gather` (`a2a/wrapper.py:1436-1460`).
   - Memory encoding/search/analyze — bounded pools with `min(len(items), 8)` for searches and a fixed 10 for parallel LLM analyze calls (`memory/encoding_flow.py:199-219`, `259`).
   - Memory recall — `ThreadPoolExecutor(max_workers=min(len(tasks), 4))` for `(embedding × scope)` fan-out (`memory/recall_flow.py:145`).
   - LLM chunk summarization for context compaction — `asyncio.gather` per chunk (`utilities/agent_utils.py:884-917`).
   - Background save in `Memory` — submits to a serialized pool (`max_workers=1`) so writes can be fast but never racing.
   - Event-handler dispatch — `gather(return_exceptions=True)` per dependency tier; sync handlers go to a 10-worker `CrewAISyncHandler` pool (`events/event_bus.py:165-190`, `504-521`).

2. **What must be serialized?**
   - Synchronous tasks within a crew's sequential chain (only async tasks run concurrently).
   - Conditional tasks — pending futures are awaited inline before evaluating the branch (`crew.py:1401-1404`).
   - Memory `_save_pool` (`ThreadPoolExecutor(max_workers=1)`) — _explicit_ "to prevent races" comment (`memory/unified_memory.py:165-169`, `443-446`).
   - LanceDB writes — `store_lock(self._lock_name)` wraps every mutating path, plus `_do_write` retries LanceDB commit conflicts (`memory/storage/lancedb_storage.py:100, 113, 128-153, 305, 421`).
   - File reads/writes — `store_lock` namespaces per realpath (`utilities/file_handler.py:88, 150, 163`).
   - User-data and tracing writes — `crewai_core.lock_store` (Redis or file) for multi-process safety (`crewai_core/user_data.py:61`, `events/listeners/tracing/utils.py:371, 396`).
   - OAuth2 token fetch (one coroutine at a time per auth scheme) — `asyncio.Lock` (`a2a/auth/client_schemes.py:362-373`, `467-478`).
   - `RPMController` per-crew counter — `threading.Lock` (`utilities/rpm_controller.py:60-64`).
   - Tool batches that contain a `result_as_answer=True` or `max_usage_count` tool — forced sequential so the right answer short-circuits or usage counts aren't double-incremented (`experimental/agent_executor.py:1746-1747`, `agents/crew_agent_executor.py:721-784`).
   - Streaming chunk events — sync handlers run on the caller thread, not the bus pool, to preserve order (`events/event_bus.py:508-518`, `626-633`).

3. **How are shared resources protected?**
   - **Event bus handler dict**: `RWLock` reader/writer (`utilities/rw_lock.py:12-81`) used in `with self._rwlock.r_locked()` / `w_locked()` blocks (`events/event_bus.py:153, 229, 378, 476, 484, 600, 688, 821, 850, 873, 907, 946`).
   - **Singleton init**: `threading.RLock` for the event-bus singleton and Telemetry (`events/event_bus.py:117-142`, `crewai_core/telemetry.py:75-84`).
   - **Memory write pool and reset**: `_save_pool` (`max_workers=1`) and `_reset_lock` (`RLock`, reentrant — see comment at `memory/encoding_flow.py:425-428`).
   - **OAuth2 token cache**: `asyncio.Lock` (`a2a/auth/client_schemes.py:339, 433`).
   - **Flow runtime state**: `_state_lock`, `_or_listeners_lock`, `_usage_metrics_lock` (`threading.Lock`); state mutations go through `StateProxy` → `LockedListProxy`/`LockedDictProxy` with deadlock-free ordering by id comparison (`flow/runtime/__init__.py:368-619, 995-1008, 1256-1284`).
   - **Tracing batch manager**: `Condition`, `_init_lock`, `_pending_events_lock`, `_finalize_lock` (`events/listeners/tracing/trace_batch_manager.py:61-66`).
   - **Tool usage counters**: per-tool `BaseTool._usage_lock` (`tools/base_tool.py:192`).
   - **Distributed locks**: `crewai_core.lock_store.lock` chooses Redis (`portalocker.RedisLock`) or file (`portalocker.Lock`); pluggable via `set_lock_backend`; defaults to MD5-hashed channel name (`crewai_core/lock_store.py:69-121`).

4. **Are results ordered deterministically?**
   - `asyncio.gather` preserves input order — explicit comment in the experimental executor notes "asyncio.gather preserves input order, so zip gives us the exact todo ↔ result" mapping (`experimental/agent_executor.py:1221-1223`). The same convention is used in `crew.py:1568-1598` (sync `_execute_tasks`), `crew.py:1308-1390` (`_aexecute_tasks`), `encoding_flow.py:199-219`, `memory/encoding_flow.py:1216-1247`.
   - `ThreadPoolExecutor + as_completed` is always written back into an `ordered_results[idx]` slot — never consumed in completion order (e.g., `agents/crew_agent_executor.py:743-768`, `experimental/agent_executor.py:1724-1746`, `a2a/wrapper.py:273-298`).
   - `_emit_with_dependencies` runs handlers in topological order via cached `ExecutionPlan` (`events/event_bus.py:457-521`).
   - `LLMStreamChunkEvent` is special-cased to run inline so chunk ordering is preserved (`events/event_bus.py:508-518`).
   - Conditional tasks and `last_sync_output` boundaries flush pending async tasks before continuing (`crew.py:1369-1373`, `crew.py:1401-1404`).

5. **What happens when one parallel branch fails?**
   - The dominant idiom is `gather(*, return_exceptions=True)` so an exception becomes a positional result and the others continue. Examples: racing listeners (`flow/runtime/__init__.py:1429`), non-racing listener gather (same line), event-bus async handlers (`events/event_bus.py:450`), parallel todos (`experimental/agent_executor.py:1216-1268`), shutdown drain (`events/event_bus.py:925`).
   - Per-future `try/except` in `as_completed` consumers with logger fallback (per-item empty result): `memory/recall_flow.py:127-172`, `memory/encoding_flow.py:206-219`, `a2a/wrapper.py:287-297`.
   - First-wins cancellation for racing listeners — once any racing listener completes, the others are `task.cancel()`'d (`flow/runtime/__init__.py:1415-1426`).
   - A2A task cancellation race: `asyncio.wait([execute_task, cancel_watch], return_when=FIRST_COMPLETED)` cancels the loser (`a2a/utils/task.py:174-193`).
   - Background save failures emit `MemorySaveFailedEvent` but never fail the calling task/crew/flow — `_on_save_done` swallows everything during shutdown (`memory/unified_memory.py:324-348`).
   - `flush()` collects exceptions from completed futures and prints them but does not raise (`events/event_bus.py:755-767`).

## Architectural Decisions

- **Dual execution model (asyncio + threadpool)**: I/O work (LLM, A2A, MCP, file store via aiocache) runs through awaitables; sync blocking work (LanceDB commit conflicts, agent card fetches, parallel tool calls, parallel memory analysis) runs through `concurrent.futures.ThreadPoolExecutor`. Both paths are inter-bridged through `asyncio.to_thread` and `_run_sync`, with explicit nested-loop guards (`utilities/file_store.py:36-55`, `utilities/agent_utils.py:985-991`).
- **Bounded pools, not unbounded fan-out**: explicit constants — 10 (`events/event_bus.py:178-181`), 8 for tools (`agents/crew_agent_executor.py:742`), `min(n, 8)` for memory search (`memory/encoding_flow.py:199`), 10 for memory analyze (`memory/encoding_flow.py:259`), `min(n, 4)` for memory recall (`memory/recall_flow.py:145`).
- **Dependency-tiered handler dispatch in the event bus**: handlers with `Depends` form an `ExecutionPlan` cached per event type; each tier runs its sync branch on the bounded pool and its async branch via `gather(..., return_exceptions=True)` before advancing (`events/event_bus.py:457-521`).
- **Auto-doc LLM call suppression** for short queries in `RecallFlow.analyze_query_step` so short recall paths skip an LLM round-trip and only parallel-search (`memory/recall_flow.py:189-241`).
- **Order-preserving gather at every fan-out**: comments call out that `gather` preserves input order, and `as_completed` results are written back to indexed slots. Streaming chunk events run inline to keep sequence intact.
- **Reentrant locks for nested storage paths**: `_reset_lock` and the LanceDB write lock are reentrant because writes may recursively acquire while holding them (`memory/encoding_flow.py:425-428`).
- **Race-prevention by serialization, not coordination**: the memory save pool is intentionally `max_workers=1` (`memory/unified_memory.py:165-169`) with comment "to prevent races" — a one-worker pool is the chosen primitive over higher-level coordination.
- **Multi-process safety via pluggable lock backend**: Redis or file `portalocker.Lock` under `crewai_core.lock_store`, with `set_lock_backend` swap-in for custom strategies (`crewai_core/lock_store.py:33-54`).
- **Sync→async adapter pattern preserved** for legacy code: `Crew.kickoff_async` wraps `self.kickoff` in `asyncio.to_thread` so old users still get an awaitable; new users should prefer `Crew.akickoff` (`crew.py:1130-1279`).
- **Context propagation across thread boundaries**: every `pool.submit(..., contextvars.copy_context().run, ...)` pattern preserves `ContextVar` (event-bus runtime state, flow method name, telemetry span baggage) — see e.g., `agents/crew_agent_executor.py:746-764`, `memory/encoding_flow.py:199-208`, `memory/unified_memory.py:305-310`.
- **Validation rules that codify ordering invariants** at Pydantic model time: `validate_end_with_at_most_one_async_task`, `validate_async_tasks_not_async`, `validate_async_task_cannot_include_sequential_async_tasks_in_context` (`crew.py:767-851`).

## Notable Patterns

- **Index-aligned ordered_results** (used wherever `as_completed` could finish out of order): `agents/crew_agent_executor.py:743-768`, `experimental/agent_executor.py:1715-1746`, `a2a/wrapper.py:280-298`.
- **Bounded-pool + contextvars.copy_context()** at every thread dispatch site — preserves `current_flow_method_name`, `baggage`, telemetry runtime state.
- **Racing listener pattern with `asyncio.as_completed` + `cancel()`**: `flow/runtime/__init__.py:1380-1429`.
- **First-complete-then-cancel race**: `a2a/utils/task.py:174-193` (`asyncio.wait(..., return_when=asyncio.FIRST_COMPLETED)`).
- **Background-daemon-thread tasks** for fire-and-forget work (LanceDB compaction): `memory/storage/lancedb_storage.py:213-221`.
- **Gather-and-zip for deterministic results** with `return_exceptions=True`: `experimental/agent_executor.py:1216-1268` plus comment "asyncio.gather preserves input order".
- **Dependency-graph-aware dispatch** with cached `ExecutionPlan`: `events/event_bus.py:457-521`.
- **Conditional-task boundary flush** before evaluating each branch — pending async tasks are awaited before invoking the predicate (`crew.py:1369-1373`, `crew.py:1401-1404`).
- **Streaming-in-thread for sync-only event types**: `LLMStreamChunkEvent` runs synchronously on the caller thread to preserve ordering (`events/event_bus.py:508-518`).
- **Background drain pattern** in `Memory.drain_writes()`: walks pending futures and calls `future.exception()` (blocking without re-raising) so callers can wait for background saves to complete without propagating their failures (`memory/unified_memory.py:350-370`).

## Tradeoffs

- **`asyncio.to_thread` adapters vs native async**: dual API surface (`kickoff`/`kickoff_async`/`akickoff`) means users can pick wrong entry point; legacy `kickoff_async` still blocks on a thread (`crew.py:1130-1162`). The reward is backwards compatibility.
- **Singleton event bus with a dedicated background event loop**: greatly simplifies cross-`await` event dispatch, but creates a hidden daemon thread (`CrewAIEventsLoop`) and a long-lived `asyncio.run` inside it; consumers must `flush()` or wait for shutdown to ensure handlers actually ran (`events/event_bus.py:897-954`).
- **Saved-futures tracking in `_pending_futures`**: bounded-overhead bookkeeping, but `flush()` blocks on `concurrent.futures.wait` — a handler that re-enters `emit()` can self-deadlock if it waits on its own future.
- **`ThreadPoolExecutor(max_workers=1)` for memory save**: simplest correct primitive for preventing races (no need for finer locks), but turns every `remember()` into serialized IO; callers must `drain_writes()` to be sure writes landed.
- **Reentrant `RLock` for storage write sequences**: easy to nest acquires but easy to forget to release on exception paths — relies on `with` discipline.
- **Dependency-tiered execution plan cached per event type**: saves rebuild cost per emit; invalidation happens on `_register_handler`/`off`. Tradeoff is event-during-cache-rebuild races — handled with a second `w_locked()` check (`events/event_bus.py:484-499`).
- **LockedListProxy/DictProxy via `StateProxy`**: helps because Flow methods can mutate state from parallel listeners — but the proxy only fires for attribute access through `state`; raw `_state` accesses bypass the lock (`flow/runtime/__init__.py:600-628`).
- **Asymmetric concurrency budget per domain**: 10 for sync bus, 8 for parallel tool calls, `min(n,8)` for memory search — operators tuning throughput need to know multiple knobs; not consolidated.
- **Multi-process safety on storage relies on the lock backend**: file locking is correct only if processes share temp dir and filesystem semantics; Redis is recommended but optional (`crewai-core/lock_store.py:99-121`).

## Failure Modes / Edge Cases

- **Skipped multi-process stress test**: `tests/memory/test_concurrent_storage.py:11-13` skips "because of xdist importlib" — so the storage write serialization across processes is **unverified by tests** despite the lock backend being in place.
- **Race between in-flight handler futures and shutdown**: mitigated by `_wait_for_all_tasks` in `shutdown()` and by `flush()`, but a threadpool future that has not been polled can still hold an open Redis connection when the loop closes (`events/event_bus.py:897-944`).
- **`flow.kickoff` from inside a running loop**: spins a `ThreadPoolExecutor(max_workers=1)` to call `asyncio.run` in a side thread — fine for one call but **easily breaks** if a Flow is nested inside another Flow without careful input-baggage handling (`flow/runtime/__init__.py:2197-2207`).
- **Concurrent `reset_memories` race**: `_reset_lock` is `RLock`, but if a different process holds `store_lock(self._lock_name)` and `Memory.reset()` is called, behavior depends on the lock backend's timeout (default 120s in `lock_store.py:35`).
- **Tool execution with `result_as_answer=True` in a parallel batch**: forced sequential but the failure mode is silent — the comment doesn't say what happens to subsequent tools when the answer-tool short-circuits; `experimental/agent_executor.py:1746-1753` shows it continues and accumulates `tool_message` entries.
- **Backpressure / unbounded memory growth**: the system deliberately relies on `gather(return_exceptions=True)` and indexing — no per-task timeout unless caller sets one (`a2a/utils/task.py:178-189` enforces none in the base case). A long-running stuck future can delay shutdown.
- **`_execute_task_async` spawns a fresh `threading.Thread` per task** (`task.py:603-610`) — **unbounded thread creation** on crews with many `async_execution=True` tasks; should be pooled in callers (they use `asyncio.create_task` in the async path, so the issue is mostly in `execute_async`).
- **`concurrent.futures.wait(futures, timeout=None)`** in `flush()` would block forever, but it accepts a `timeout` (`events/event_bus.py:732-767`) — default 30s, which is silent and could mask handlers stuck in deadlock.
- **OAuth2 token lock per-instance**: two distinct `OAuth2ClientCredentials` objects do **not** coordinate, so a user creating two clients for the same OAuth server can hammer the token endpoint (and trigger upstream rate limits).

## Future Considerations

- Consolidate the scattered locks (`_state_lock`, `_or_listeners_lock`, `_usage_metrics_lock`, `_pending_lock`, `_reset_lock`, `_save_pool`) into a single Concurrency module so the model is auditable in one place.
- Promote `Crew.akickoff` as the default and deprecate `kickoff_async`'s `asyncio.to_thread` shim to remove the dual path (`crew.py:1130-1162`).
- Re-enable multi-process storage tests in CI — they are the only direct proof of `lock_store` correctness under fork. They were skipped due to xdist import mode; resolving the import-mode constraint would close the largest observable gap.
- Add explicit timeouts / cancellation propagation to `gather(return_exceptions=True)` paths in the Flow runtime so racing listeners that hang don't block `_wait_for_all_tasks` during shutdown (`events/event_bus.py:918-925`).
- Document the order-preservation guarantee of `gather` and the index-aligned `ordered_results` invariant in a developer-facing concurrency guide (the rules are followed consistently but never explained in `docs/edge`).
- Consider replacing per-call `threading.Thread` spawns in `Task.execute_async` with a shared `ThreadPoolExecutor` to avoid unbounded thread fan-out under many async tasks (`task.py:603-610`).
- Extend per-instance `_save_pool` to support a higher `max_workers` while still serializing per-storage-backend — the current `max_workers=1` is safe but a bottleneck for batch writes (`memory/unified_memory.py:165-169`).

## Questions / Gaps

- **Is the per-crew `_save_pool` shared across crews in a multi-crew app?** No evidence found that it is auto-shared; each `Memory(...)` instance creates its own pool with `max_workers=1`. The cardinality of pools is unclear under multi-crew usage.
- **What happens to in-flight handlers if the user process is killed mid-`kickoff`?** `atexit.register(crewai_event_bus.shutdown)` exists, but no test verifies the daemon-thread event loop survives `os.kill`. No clear evidence found that pending handlers run in a worker-then-listener decoupling that survives abrupt termination.
- **Is `LLMStreamChunkEvent`'s "sync inline" guarantee true across all registered sync handlers, including those that throw?** The current contract prints the error (`_call_handlers` exception printing) but no test asserts order preservation under concurrent emitters.
- **What is the inter-agent contract on `current_flow_method_name` ContextVar** when a Flow method calls `Agent.kickoff()` (sync) inside another Flow method — `asyncio.to_thread(ctx.run, ...)` is supposed to propagate, but no explicit test that survives run_in_executor nesting was found in `tests/`.
- **How does `kickoff_async` progress reporting interact with `asyncio.to_thread`?** `signal_end(ctx.state, is_async=True)` is called from inside the thread; the `state.async_queue.put_nowait` is invoked via `loop.call_soon_threadsafe` (`crews/utils.py:453-468`) — fragile pattern; no clear evidence of tests for cross-thread stream ordering.
- **Does parallel native tool execution respect `tool.result_as_answer` ordering in the chat message history?** Code path at `experimental/agent_executor.py:1724-1753` writes ordered_results back into the assistant message sequentially; safety under `max_usage_count` exhaustion mid-batch is not explicitly tested.

---

Generated by `01.07-concurrency-and-parallel-advancement` against `crewai`.
