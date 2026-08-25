# Source Analysis: openhands

## Concurrency and Parallel Advancement

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12+ (FastAPI + asyncio + SQLAlchemy AsyncSession + httpx.AsyncClient); TypeScript/Vite frontend in `frontend/` |
| Analyzed | 2026-07-03 |

## Summary

The selected source is the OpenHands monorepo. Under `openhands/app_server/` is the V1 "application server" (`OpenHands V1 uses the Software Agent SDK for the agentic core and runs a new application server`, `openhands/app_server/user_auth/user_auth.py:3-5`). The actual agent step loop does not live in this repository — it is delegated to a separately-deployed agent-server container (`openhands/app_server/sandbox/sandbox_spec_service.py:16` pins `ghcr.io/openhands/agent-server:1.29.0-python`). Per-conversation work is run inside that sandbox process (Docker / remote runtime / Python subprocess), so "parallel advancement of one run" (parallel tool calls, parallel sub-agent invocations) is not observable in this repo.

What IS in scope for this dimension is the app-server's own concurrency model: every I/O path is `async def`, with blocking calls (file I/O, S3, Docker SDK, boto3, psutil) offloaded to the default thread pool via `loop.run_in_executor(None, ...)`. Read fan-out uses `asyncio.gather` consistently across the batch services; write fan-out is mostly serialized with two notable exceptions (event-callback dispatch, webhook event persistence). Cross-worker safety is delegated to the DB or to a Redis lock for the one operation that cannot tolerate re-entry (`conversation export`). The agent-server keeps itself up to date with the app server either by calling back to a webhook URL or by being polled by a single long-lived background task (`poll_agent_servers`).

## Rating

**6 / 10** — Present but inconsistent, weakly documented in places, and fragile around the boundaries.

The async primitives are all present (`asyncio.gather`, `asyncio.Semaphore`, `asyncio.Lock`, `asyncio.Event`, `loop.run_in_executor`, `redis.asyncio.lock.Lock`, a shared `ThreadPoolExecutor`). They are used for the right things in most places (batched reads, semaphore-bounded file loads, Redis-locked cross-worker exports, phased read→release→write polling). The gaps that hold this back from a 7+ are:

- Most `asyncio.gather` calls omit `return_exceptions=True`, so one failing parallel I/O cancels the rest and propagates a single error — fine for correctness, poor for availability.
- Per-user locks (`_user_profile_locks` in `settings/settings_router.py:320`) are explicitly process-local; the code comment hands multi-worker safety to "DB-level row locks or optimistic concurrency tokens" with no implementation.
- Background tasks are spawned with bare `asyncio.create_task(...)` in at least three call sites (`webhook_router.py:513`, `app_conversation_router.py:405`, `app_conversation_router.py:968`, `live_status_app_conversation_service.py:2398`) with no shared error observer or cancellation registry.
- The agent loop / step concurrency is not in this repository and cannot be evaluated here.
- The `_process_pending_messages` docstring claims "Messages are delivered concurrently to the agent server" (`live_status_app_conversation_service.py:1934-1935`), but the implementation is a sequential `for msg in pending_messages:` loop (`live_status_app_conversation_service.py:1976-1995`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Module-level shared `ThreadPoolExecutor` (process-wide) | `EXECUTOR = ThreadPoolExecutor()` | `openhands/app_server/utils/async_utils.py:7` |
| Sync-from-async shorthand | `coro = loop.run_in_executor(None, lambda: fn(*args, **kwargs))` | `openhands/app_server/utils/async_utils.py:10-18` |
| Async-from-sync (run coro in executor) | `EXECUTOR.submit(run)` + `asyncio.run_coroutine_threadsafe` | `openhands/app_server/utils/async_utils.py:21-52`, `openhands/app_server/utils/async_utils.py:115-118` |
| Custom `wait_all` that creates one task per input, awaits with timeout, cancels pending, aggregates errors into `AsyncException` | `async def wait_all(...)` | `openhands/app_server/utils/async_utils.py:62-89` |
| `AsyncException` aggregator class | `class AsyncException(Exception)` | `openhands/app_server/utils/async_utils.py:92-97` |
| Event load bounded by `asyncio.Semaphore` (configurable) | `semaphore = asyncio.Semaphore(_event_load_concurrency())` + `await asyncio.gather(*(load_event(path) for path in paths))` | `openhands/app_server/event/event_service_base.py:56-64` |
| Concurrency knob for event loads | `_event_load_concurrency()` reads `EVENT_SERVICE_LOAD_EVENT_CONCURRENCY` (default 10) | `openhands/app_server/event/event_service_base.py:24-28` |
| Disk reads offloaded to default thread pool | `await loop.run_in_executor(None, self._load_event, path)` (called from 6 sites) | `openhands/app_server/event/event_service_base.py:62`, `:91`, `:107`, `:151`, `:187`, `:197` |
| In-flight dedup of conversation-info lookups | `app_conversation_info_load_tasks: dict[UUID, asyncio.Task[AppConversationInfo \| None]]` | `openhands/app_server/event/event_service_base.py:40-42`, `:72-80` |
| Batch event fetch fans out with `asyncio.gather` | `await asyncio.gather(*[self.get_event(...) for event_id in event_ids])` | `openhands/app_server/event/event_service_base.py:199-204`, `openhands/app_server/event/event_service.py:64-70` |
| Batch event-callback fetch fans out | `results = await asyncio.gather(*[self.get_event_callback(...)])` | `openhands/app_server/event_callback/event_callback_service.py:44-54` |
| SQL event-callback dispatch fans out per callback | `await asyncio.gather(*[self.execute_callback(...) for callback in callbacks])` | `openhands/app_server/event_callback/sql_event_callback_service.py:221-226` |
| Webhook `on_event` saves all incoming events concurrently (no `return_exceptions`) | `await asyncio.gather(*[event_service.save_event(...) for event in events])` | `openhands/app_server/event_callback/webhook_router.py:464-466` |
| Webhook callbacks deliberately run sequentially, on a background task | comment "We don't use asynio.gather here because callbacks must be run in sequence." | `openhands/app_server/event_callback/webhook_router.py:561-573` |
| Background callback chain started via `asyncio.create_task` | `asyncio.create_task(_run_callbacks_in_bg_and_close(...))` | `openhands/app_server/event_callback/webhook_router.py:513-517` |
| Background `start_app_conversation` iterator consumption | `asyncio.create_task(_consume_remaining(async_iter, db_session, httpx_client))` | `openhands/app_server/app_conversation/app_conversation_router.py:405` |
| Background sandbox-delete finalizer | `asyncio.create_task(_finalize_sandbox_delete(...))` | `openhands/app_server/app_conversation/app_conversation_router.py:968-976` |
| Finalizer closes DB session + httpx client concurrently | `await asyncio.gather(db_session.aclose(), httpx_client.aclose())` | `openhands/app_server/app_conversation/app_conversation_router.py:886-889` |
| Batch conversation reads fan out | `return await asyncio.gather(*[self.get_app_conversation(...)])` | `openhands/app_server/app_conversation/app_conversation_service.py:70-79` |
| Batch conversation-info reads fan out | `return await asyncio.gather(*[self.get_app_conversation_info(...)])` | `openhands/app_server/app_conversation/app_conversation_info_service.py:53-62` |
| Batch start-task reads fan out | `return await asyncio.gather(*[self.get_app_conversation_start_task(...)])` | `openhands/app_server/app_conversation/app_conversation_start_task_service.py:43-49` |
| Batch sandbox reads fan out | `results = await asyncio.gather(*[self.get_sandbox(...)])` | `openhands/app_server/sandbox/sandbox_service.py:65-72` |
| Batch sandbox-spec reads fan out | `results = await asyncio.gather(*[self.get_sandbox_spec(...)])` | `openhands/app_server/sandbox/sandbox_spec_service.py:45-55` |
| Live-status view fans out per sandbox | `sandbox_conversation_infos = await asyncio.gather(*tasks)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:553-563` |
| Docker image pulls fan out across specs | `await asyncio.gather(*[self.pull_spec_if_missing(spec) for spec in self.specs])` | `openhands/app_server/sandbox/docker_sandbox_spec_service.py:79` |
| Docker pull: progress logger + actual pull coordinated by `asyncio.Event` | `pull_complete = asyncio.Event()`, two `asyncio.create_task`s, `await pull_task` | `openhands/app_server/sandbox/docker_sandbox_spec_service.py:99-132` |
| Docker blocking pull offloaded to default executor | `await loop.run_in_executor(None, docker_client.images.pull, image_id)` | `openhands/app_server/sandbox/docker_sandbox_spec_service.py:115` |
| Process-wide singleton polling task for the no-webhook path | `polling_task: asyncio.Task \| None = None`, started lazily inside `RemoteSandboxServiceInjector.inject` | `openhands/app_server/sandbox/remote_sandbox_service.py:54`, `:924-932` |
| `poll_agent_servers` runs forever with explicit "fetch -> release -> network -> re-acquire -> write" phases | comments at lines 690-691 and 781-783 | `openhands/app_server/sandbox/remote_sandbox_service.py:682-768`, `:771-870` |
| GitHub suggested-tasks GraphQL queries run concurrently with `return_exceptions=True` | `await asyncio.gather(..., return_exceptions=True)` | `openhands/app_server/integrations/github/service/features.py:38-42` |
| Per-user `asyncio.Lock` for LLM profile mutations | `_user_profile_locks: defaultdict[str, asyncio.Lock] = defaultdict(asyncio.Lock)` | `openhands/app_server/settings/settings_router.py:320` |
| All four profile mutators wrap the read-modify-write in the per-user lock | `async with _user_profile_locks[_profile_lock_key(user_id)]:` (4 sites) | `openhands/app_server/settings/settings_router.py:483`, `:535`, `:555`, `:594` |
| Per-user lock is explicitly process-local (comment) | "In multi-worker SaaS deployments, DB-level row locks or optimistic concurrency tokens are still required for full safety — but this closes the single-process hole." | `openhands/app_server/settings/settings_router.py:313-319` |
| `RedisLockUnavailable` distinguishes "Redis broken" from "lock held" | `class RedisLockUnavailable(Exception)` | `openhands/app_server/utils/redis_lock.py:21-22` |
| Try-acquire Redis lock with `blocking=False` | `acquired = await lock.acquire(blocking=False)` | `openhands/app_server/utils/redis_lock.py:25-33` |
| Periodic lock refresh coroutine (background) | `async def refresh_lock_periodically(lock, interval)` | `openhands/app_server/utils/redis_lock.py:36-53` |
| Export lock acquired + refreshed in background during streaming export | `refresh_task = asyncio.create_task(refresh_lock_periodically(export_lock, refresh_interval))` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2392-2419` |
| Configurable export lock TTL / refresh interval | `export_lock_ttl_seconds`, `export_lock_refresh_interval_seconds` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2469-2476` |
| Module-level `threading.Lock` for lazy Redis client init (sync context) | `_redis_lock = threading.Lock()` | `openhands/app_server/utils/redis.py:17`, `:41-44`, `:59-62` |
| SQLAlchemy async engine has bounded pool | `pool_size: int = 25`, `max_overflow: int = 10`, `pool_recycle: int = 1800` | `openhands/app_server/services/db_session_injector.py:36-38` |
| SQLite path uses `NullPool` to avoid sharing connections across event loops | `poolclass=NullPool` | `openhands/app_server/services/db_session_injector.py:202-207` |
| GCP connector is rebuilt if the event loop changes | `Connector(loop=current_loop)` re-created on loop mismatch | `openhands/app_server/services/db_session_injector.py:98-107` |
| Request-scoped `AsyncSession` reused across dependencies | `setattr(state, DB_SESSION_ATTR, db_session)` and `if db_session: yield db_session` | `openhands/app_server/services/db_session_injector.py:285-323` |
| "Keep open" flag prevents DB session close while a detached background task is using it | `DB_SESSION_KEEP_OPEN_ATTR`, `set_db_session_keep_open(...)` | `openhands/app_server/services/db_session_injector.py:24-25`, `:311`, `:319`, `:326-327` |
| `keep_open` is set before detaching background work in router | `set_db_session_keep_open(request.state, True)` | `openhands/app_server/app_conversation/app_conversation_router.py:963-964` |
| `wait_for_sandbox_running` polls serially in a `while` loop | `while time.time() - start <= timeout:` | `openhands/app_server/sandbox/sandbox_service.py:93-140` |
| Sandbox quota enforcement pauses oldest via serial loop | `for sandbox in sandboxes_to_pause:` | `openhands/app_server/sandbox/sandbox_service.py:233-242`, `openhands/app_server/sandbox/remote_sandbox_service.py:612-621` |
| Sub-conversation deletion is a serial loop | `for sub_id in sub_conversation_ids:` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2218-2236` |
| Polling iteration is serial per conversation | `for app_conversation_info in conversations_to_refresh:` | `openhands/app_server/sandbox/remote_sandbox_service.py:743-754` |
| Pending message delivery is explicitly serial | "Process messages sequentially to preserve order" + `for msg in pending_messages:` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1976-1995` |
| Docstring for `_process_pending_messages` claims concurrency | "Messages are delivered concurrently to the agent server" | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1932-1935` |
| Per-conversation pending-message rate cap (10) | "Rate limit: max 10 pending messages per conversation" | `openhands/app_server/pending_messages/pending_message_router.py:85-91` |
| Atomic file writes use pid+tid temp file + `os.replace` | `temp_path = f'{full_path}.tmp.{os.getpid()}.{threading.get_ident()}'` + `os.replace(temp_path, full_path)` | `openhands/app_server/file_store/local.py:26-43` |
| In-memory rate limiter (no shared state across processes) | `class InMemoryRateLimiter` | `openhands/app_server/middleware.py:78-112` |
| App-level rate limit middleware | `RateLimitMiddleware` with `InMemoryRateLimiter(requests=10, seconds=1)` | `openhands/app_server/app.py:83-85` |
| Sandbox selection strategies (ADD_TO_ANY, GROUP_BY_NEWEST, LEAST_RECENTLY_USED, FEWEST_CONVERSATIONS) | `_select_sandbox_by_strategy` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:721-774`, `:2450-2453` |
| Conversation-per-sandbox cap, configurable per-sandbox | `max_num_conversations_per_sandbox: int = Field(default=20)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2450-2453` |
| Sub-conversation relationship is modeled on `AppConversationInfo` | `parent_conversation_id`, `sub_conversation_ids` | `openhands/app_server/app_conversation/app_conversation_models.py:131-132` |
| Per-request `httpx.AsyncClient` enables concurrent outbound HTTP to the agent server | `httpx_client: httpx.AsyncClient` (declared on services) | `openhands/app_server/sandbox/sandbox_service.py:98`, `openhands/app_server/sandbox/remote_sandbox_service.py:115` |
| Process-based sandbox spawns a fresh agent-server Python process per sandbox | `subprocess.Popen(cmd, env=env, ...)` | `openhands/app_server/sandbox/process_sandbox_service.py:142-144` |
| Tests verify `wait_all` parallelism and timeout | `test_await_all*`, `test_await_all_timeout` | `tests/unit/utils/test_async_utils.py:16-77` |
| Tests verify `run_in_loop` across event loops | `test_run_in_loop_*` | `tests/unit/utils/test_async_utils.py:145-341` |
| Tests verify Redis-locked streaming export and "fails closed" behavior | `test_open_conversation_export_*` | `tests/unit/app_server/test_live_status_app_conversation_service.py:1666-1766` |

## Answers to Dimension Questions

1. **What can run in parallel?**
   - **Read fan-out inside a request.** All `batch_get_*` services (`batch_get_app_conversations`, `batch_get_app_conversation_info`, `batch_get_app_conversation_start_tasks`, `batch_get_sandboxes`, `batch_get_sandbox_specs`, `batch_get_events`, `batch_get_event_callbacks`) use `asyncio.gather` over their async getters. The underlying DB reads use SQLAlchemy `AsyncSession`; the underlying file/disk/S3 reads go through `loop.run_in_executor`.
   - **Event load fan-out under a semaphore.** `_load_events_from_paths` uses `asyncio.Semaphore(_event_load_concurrency())` to bound disk reads, then `asyncio.gather` (`event_service_base.py:56-64`).
   - **Webhook event persistence.** A webhook carrying `N` events saves them all concurrently (`webhook_router.py:464-466`).
   - **Webhook callback dispatch.** `execute_callbacks` runs all callbacks for one event concurrently via `asyncio.gather` (`sql_event_callback_service.py:221-226`). Across events the chain is fired as one `asyncio.create_task` and processed serially (`webhook_router.py:571-573`).
   - **Sandbox image pre-fetch.** `pull_missing_specs` runs pulls for every spec in parallel (`docker_sandbox_spec_service.py:79`); each pull has a paired progress logger joined via `asyncio.Event`.
   - **Live-status page.** Per-sandbox live `ConversationInfo` lookups are gathered (`live_status_app_conversation_service.py:563`).
   - **GitHub suggested-tasks.** Two GraphQL queries run concurrently with `return_exceptions=True` (`features.py:38-42`).
   - **Background polling of remote agent servers** when no public webhook URL is configured (`remote_sandbox_service.py:924-932`, loop at `:699-768`).
   - **Background stream consumption** of the long-lived `start_app_conversation` iterator (`app_conversation_router.py:405`).
   - **Background lock refresh** for long-lived Redis export locks (`redis_lock.py:36-53`, started at `live_status_app_conversation_service.py:2397-2411`).
   - **Across-run parallelism** (between users / between sandboxes) is achieved by routing each conversation to a different sandbox process — Docker container (`docker_sandbox_service.py:484`), remote runtime (`remote_sandbox_service.py`), or Python subprocess (`process_sandbox_service.py:142-144`).

2. **What must be serialized?**
   - **LLM profile mutations per user** (save/delete/activate/rename) under `_user_profile_locks[user_id]` (`settings_router.py:320`, used at `:483`, `:535`, `:555`, `:594`). The code explicitly notes this only protects a single worker (`settings_router.py:316-319`).
   - **Event callbacks per event** — comment is explicit: "We don't use asynio.gather here because callbacks must be run in sequence." (`webhook_router.py:571-573`).
   - **Pending message delivery** is sequential per message to preserve order (`live_status_app_conversation_service.py:1976-1995`), contradicting its own docstring.
   - **Sandbox limit enforcement** is a serial `for sandbox in sandboxes_to_pause:` loop (`sandbox_service.py:233-242`, `remote_sandbox_service.py:612-621`).
   - **Sub-conversation deletion** is a serial `for sub_id in sub_conversation_ids:` loop (`live_status_app_conversation_service.py:2218-2236`).
   - **Remote runtime polling** is serial per conversation (`remote_sandbox_service.py:743-754`).
   - **Conversation export** is held under a Redis lock so only one worker per conversation is exporting at a time (`live_status_app_conversation_service.py:2354-2421`).
   - **Redis client lazy init** uses `threading.Lock` because the init is synchronous (`redis.py:17`, `:41-44`, `:59-62`).

3. **How are shared resources protected?**
   - **DB connection pool**: SQLAlchemy async engine `pool_size=25`, `max_overflow=10`, `pool_recycle=1800` (`db_session_injector.py:36-38`); SQLite path uses `NullPool` to avoid cross-loop sharing (`db_session_injector.py:202-207`).
   - **Sandbox quotas**: `max_num_sandboxes` (defaults: Docker 5, remote 10) and `max_num_conversations_per_sandbox` (default 20) enforced by pausing oldest / least-loaded (`docker_sandbox_service.py:599-602`, `remote_sandbox_service.py:902-905`, `live_status_app_conversation_service.py:2450-2453`).
   - **Cross-worker export**: Redis lock via `try_acquire_redis_lock(key, ttl_seconds)` returning `Lock | None` and raising `RedisLockUnavailable` on Redis failure (`redis_lock.py:25-33`, `live_status_app_conversation_service.py:2354-2380`).
   - **Atomic file writes**: temp-file rename with `pid+threading.get_ident()` in the name (`file_store/local.py:26-43`).
   - **Per-user profile lock**: `defaultdict[str, asyncio.Lock]` (`settings_router.py:320`).
   - **Per-event-load semaphore**: `asyncio.Semaphore(_event_load_concurrency())` caps parallel file/S3/GCS loads (`event_service_base.py:58-64`).
   - **Request-scoped DB session**: one `AsyncSession` reused across dependencies (`db_session_injector.py:285-323`); can be kept alive for detached background tasks via `set_db_session_keep_open` (`db_session_injector.py:326-327`, set at `app_conversation_router.py:963-964`).
   - **App-level rate limit**: in-memory limiter (`middleware.py:78-112`), 10 req/s by default (`app.py:83-85`). Not shared across processes.

4. **Are results ordered deterministically?**
   - `asyncio.gather` preserves input order, so all `batch_get_*` helpers return results in the same order as their input list. This is the dominant deterministic guarantee in the codebase.
   - `wait_all` re-orders by iterating the original `tasks` list (`async_utils.py:80-89`), so the final list is in input order regardless of completion order.
   - **Event search** is gathered and then explicitly sorted by `event.timestamp` before pagination (`event_service_base.py:115-141`).
   - **Live-status view** merges gathered results into a `dict` keyed by conversation id, then re-iterates in input order (`live_status_app_conversation_service.py:567-585`), so the response shape is deterministic.
   - **Pending message delivery** relies on `ORDER BY created_at ASC` at the DB layer (`pending_message_service.py:127`) and the in-process loop iterates that already-ordered list (`live_status_app_conversation_service.py:1976-1995`).
   - **Polling tasks** and `asyncio.create_task` background work make no ordering claim; they advance when their underlying I/O completes.

5. **What happens when one parallel branch fails?**
   - Most `asyncio.gather` calls use the default `return_exceptions=False`, so the first failure cancels the remaining tasks and propagates. Confirmed at: `webhook_router.py:464-466`, `event_service_base.py:64`, `event_service_base.py:203`, `sandbox_service.py:69-72`, `sandbox_spec_service.py:49-55`, `app_conversation_info_service.py:57-62`, `app_conversation_start_task_service.py:47-49`, `event_callback_service.py:48-54`, `app_conversation_router.py:886-889`, `live_status_app_conversation_service.py:563`.
   - `wait_all` (`async_utils.py:62-89`) explicitly collects all exceptions and raises the first; with multiple it raises `AsyncException(errors)` so the caller sees every failure.
   - The only `return_exceptions=True` use in `openhands/app_server/` is GitHub suggested tasks (`features.py:38-42`); the caller then inspects each element with `isinstance(x, Exception)`.
   - **Background tasks** spawned with `asyncio.create_task` (`webhook_router.py:513`, `app_conversation_router.py:405`, `app_conversation_router.py:968`, `live_status_app_conversation_service.py:2397`) have no observer; exceptions are silently dropped. The webhook handler returns `Success()` and the request closes before the callbacks finish.
   - `pause_old_sandboxes` swallows per-sandbox exceptions in its `for` loop (`sandbox_service.py:235-242`).
   - The DB session is auto-rolled-back on error inside `db_session_injector.py:313-316`.

## Architectural Decisions

- **Async/await is the universal concurrency primitive.** Every I/O path is `async def` and uses SQLAlchemy 2.x `AsyncSession` and per-request `httpx.AsyncClient`. The only `concurrent.futures` symbol used as a process-wide resource is the module-level `EXECUTOR` in `async_utils.py:7`.
- **Sync code is offloaded to the default thread pool** via `loop.run_in_executor(None, ...)` rather than wrapped in dedicated pools. This applies to event load/store, Docker image pulls, boto3 calls in `default_llm_model_service.py:162`, psutil in `process_sandbox_service.py`, and the local file store.
- **Per-request dependency injection scope.** `httpx.AsyncClient` and `AsyncSession` are scoped to the FastAPI request. A `_user_profile_locks` dict, the `_redis_lock` global, and the singleton `polling_task` are the only process-global mutable state that is shared across requests.
- **Cross-worker safety is delegated to Redis for exactly one operation** (conversation export). Everything else is either single-process (with `asyncio.Lock`) or DB-serialized (transactions). There is no Redis pub/sub or work queue.
- **Sub-conversations are first-class data** (`AppConversationInfo.parent_conversation_id`, `sub_conversation_ids` at `app_conversation_models.py:131-132`) so the app server can delete them as a group, but it never creates sub-conversations in parallel from this repo.
- **One sandbox per conversation workload.** Concurrency between users is achieved by fanning work out across separate runtime processes; inside one process, fan-out is bounded by `max_num_sandboxes` and `max_num_conversations_per_sandbox`.
- **Background tasks are first-class** but lack a shared "managed task" abstraction. Each `asyncio.create_task` is hand-rolled per call site, with no central registry, error capture, or cancellation hook.

## Notable Patterns

- **Uniform `batch_get_*` fan-out** with `asyncio.gather` on every service (`sandbox_service.py:65-72`, `sandbox_spec_service.py:45-55`, `app_conversation_info_service.py:53-62`, `app_conversation_start_task_service.py:43-49`, `event_service_base.py:199-204`, `event_callback_service.py:44-54`, `app_conversation_service.py:70-79`).
- **Bounded parallel file/disk/S3 reads** via `asyncio.Semaphore` + `loop.run_in_executor` (`event_service_base.py:58-64`).
- **Per-process polling daemon** for service discovery in the no-webhook path (`remote_sandbox_service.py:682-768`, `:924-932`).
- **Phased read→release→network→re-acquire→write** pattern in `poll_agent_servers` to keep DB sessions from being held across network I/O (`remote_sandbox_service.py:722-758`, `:781-870`).
- **Redis lock + periodic refresh** for long-lived exclusive operations (`redis_lock.py:36-53`, `live_status_app_conversation_service.py:2397-2411`).
- **Atomic file writes** with `pid+threading.get_ident()` temp filename and `os.replace` (`file_store/local.py:26-43`).
- **Streaming zip export** with a custom `_StreamingZipBuffer` (declared near `live_status_app_conversation_service.py:147-173`) so the Redis lock can be held for the duration of the stream.
- **In-flight dedup** for `app_conversation_info` via `app_conversation_info_load_tasks: dict[UUID, asyncio.Task[...]]` (`event_service_base.py:40-42`, `:72-80`): a single request coalesces repeated lookups for the same conversation id.
- **Per-request keep-open DB session** via `set_db_session_keep_open(state, True)` to let a detached background task outlive the request (`db_session_injector.py:326-327`, used at `app_conversation_router.py:963-964`).

## Tradeoffs

- **Fail-fast on `asyncio.gather`.** One failing parallel I/O cancels the rest and the request fails; there is no "return whatever succeeded" mode. This is the right default for correctness, but one bad row can degrade the whole `/app-conversations` page.
- **Most gathers omit `return_exceptions=True`.** Simplicity and consistent error messages win over partial-success availability.
- **One module-level `ThreadPoolExecutor`.** `EXECUTOR` (`async_utils.py:7`) is shared across sync-from-async, async-from-sync, Docker pulls, and the local file store. A misbehaving sync function can starve the pool; nothing isolates concerns.
- **Per-user `asyncio.Lock` for profiles is process-local.** The code comment at `settings_router.py:316-319` admits this and leaves multi-worker safety to a future DB-level guarantee that is not implemented in this repo.
- **Serial pending-message delivery** is correct for ordering but defeats the latency benefit of queueing follow-ups that arrive while the conversation is busy.
- **Singleton global polling task** (`polling_task`) avoids thundering-herd against the runtime API but if it dies the process silently loses cross-process state updates — there is no restart-on-failure path.
- **Background `asyncio.create_task` calls** are concise but lack error propagation and a managed lifecycle; a long-running detached task can outlive the request in pathological cases.
- **`_user_profile_locks` grows monotonically.** The comment at `settings_router.py:318-319` calls this acceptable; for SaaS-scale this is worth revisiting.
- **In-memory rate limiter** (`middleware.py:78-112`) does not share state across workers; a multi-worker deployment would need a Redis-backed limiter to be correct.

## Failure Modes / Edge Cases

- **Singleton polling task restart is not concurrency-safe.** `if polling_task is None` (`remote_sandbox_service.py:924-925`) is fine inside a single event loop but not across threads.
- **`keep_open` DB session is a footgun.** The detached task is responsible for both closing the session and not holding it past its useful life (`db_session_injector.py:311`, `:319`, used at `app_conversation_router.py:963-964`).
- **`asyncio.gather` default = first failure cancels the rest.** In `webhook_router.py:464-466`, one bad event fails the entire webhook delivery.
- **`_user_profile_locks` is never evicted** when a user logs out, so memory grows with distinct users seen by the process.
- **Redis lock failure semantics.** `try_acquire_redis_lock` returns `None` for "held" and raises `RedisLockUnavailable` for "Redis broken" (`redis_lock.py:25-33`); the `export_lock_required` flag controls whether the second case is fatal (`live_status_app_conversation_service.py:2365-2380`).
- **Default `ThreadPoolExecutor` is unbounded in worker count**, so a runaway sync call could fill the pool; combined with no observability on the executor, this is a latent DoS vector.
- **Webhook callback background task is unobservable.** `_run_callbacks_in_bg_and_close` (`webhook_router.py:561-574`) runs after the parent request returned; failures are not visible to the caller.
- **`LocalFileStore.write` temp-file name uses `pid + threading.get_ident()`.** Multiple coroutines on the same OS thread will collide on the temp filename (`file_store/local.py:33-43`).
- **Pending message `position`** is `count + 1` (`pending_message_service.py:99-100`) but is not atomic with the insert; concurrent producers can report a wrong position.
- **Docker pull `pull_if_missing = False`** after first run (`docker_sandbox_spec_service.py:75`) — if an image is removed externally, the system will not re-pull until restart.
- **Conversation count for sandbox selection** is a serial loop of per-sandbox `count_app_conversation_info` calls (`live_status_app_conversation_service.py:789-796`), so this is `O(sandboxes * query time)` per call.
- **`_process_pending_messages` docstring vs. implementation drift.** Docstring claims concurrency (`live_status_app_conversation_service.py:1932-1935`); code is a sequential `for` loop (`:1976-1995`).

## Future Considerations

- **Per-concern thread pools** (file I/O vs. Docker SDK vs. zip writing) would isolate misbehaving sync calls from the default executor.
- **Managed background-task registry** to capture failures, propagate cancellation, and surface them through structured logs.
- **`return_exceptions=True` + per-element error reporting** on read-fan-out paths (or a partial-success page shape) would improve availability.
- **Redis-backed rate limiter** so `InMemoryRateLimiter` is correct in multi-worker deployments.
- **Atomic pending-message position** (e.g., SQL `INSERT ... RETURNING` or `INSERT ... ON CONFLICT` with a sequence).
- **Parallel pending-message delivery** with an explicit ordering contract: deliver in arrival order, fail-fast on first error, but allow independent retries on the remaining messages.
- **Profile-lock eviction / TTL** to keep `_user_profile_locks` bounded.
- **Cross-worker profile lock via Redis** (the comment at `settings_router.py:316-319` already anticipates this).
- **Per-sandbox conversation count** as a single aggregate SQL query rather than a per-sandbox serial loop.
- **Restart-on-failure path for the singleton polling task** with a heartbeat metric.

## Questions / Gaps

- **No evidence found** for in-process parallel tool execution (parallel tool calls, parallel sub-agent calls). The `enable_sub_agents` setting is wired in (`live_status_app_conversation_service.py:1622-1625`) but the actual sub-agent machinery lives in the external `openhands-agent-server` package, not in this repo.
- **No evidence found** for an in-process worker pool or job queue (`asyncio.Queue`, `multiprocessing.Pool`, Celery, RQ). All background work is either a single global `asyncio.create_task` or a request-scoped coroutine.
- **No evidence found** for a documented concurrency contract for the agent-server side (e.g., "events arrive in timestamp order" or "two agent loops never overlap writes"). The app server assumes the agent server is well-behaved.
- **No evidence found** for a global shutdown handler that cancels in-flight `asyncio.create_task` background work; the app relies on the FastAPI lifespan.
- **No evidence found** for cross-sandbox transactional guarantees. A sandbox can be paused or deleted while a webhook is in flight, and the code logs and continues.
- **No evidence found** for a temp-file name in `LocalFileStore.write` that uses the asyncio Task id rather than `threading.get_ident()`; in async-only code multiple tasks share one OS thread.

---

Generated by `01.07-concurrency-and-parallel-advancement` against `openhands`.