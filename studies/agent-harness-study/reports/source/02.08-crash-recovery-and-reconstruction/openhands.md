# Source Analysis: openhands

## 02.08 Crash Recovery and Reconstruction

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12, FastAPI, SQLAlchemy 2 async, Alembic, asyncio, httpx, Pydantic v2, Docker SDK / psutil (sandboxing); React/Vite frontend (not relevant here) |
| Analyzed | 2026-07-12 |

## Summary

OpenHands' recovery story is split between three components that each carry their own durability contract:

1. **Schema-level recovery (startup)**: `OssAppLifespanService.__aenter__` runs `alembic upgrade head` against the SQL DB on every server boot (`openhands/app_server/app_lifespan/oss_app_lifespan_service.py:15-37`). Migrations are additive and idempotent (`alembic/env.py:107-111`), so a partially-migrated DB is brought to head automatically. There is no per-row reaper — incomplete runs and stale start-task rows are *persisted* across restarts but no background job scans or repairs them on boot.

2. **Run lifecycle (in-process / per request)**: Long-running conversation starts are persisted to the `app_conversation_start_task` table via `save_app_conversation_start_task` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-307`), then the rest of the work runs inside `asyncio.create_task(_consume_remaining(...))` (`openhands/app_server/app_conversation/app_conversation_router.py:405`). If the request thread or worker is killed mid-start, the task row sits at `WORKING`/`WAITING_FOR_SANDBOX`/etc. (`AppConversationStartTaskStatus.WORKING = 'WORKING'` at `openhands/app_server/app_conversation/app_conversation_models.py:262-271`). There is **no restart-time sweep**: nothing in `oss_app_lifespan_service.py` or `app.py` queries these rows, marks them failed, or reattaches them. They are only cleaned when the user later deletes the conversation (`live_status_app_conversation_service.py:2271-2290`) via `delete_app_conversation_start_tasks`.

3. **Pending message queue (in-flight tool calls / queued user messages)**: `SQLPendingMessageService` stores user messages sent while a conversation is still starting (`openhands/app_server/pending_messages/pending_message_service.py:30-37, 81-120`). After a successful start, `LiveStatusAppConversationService._process_pending_messages` rewrites queued rows from the temporary `task-{uuid}` conversation id to the real conversation id and POSTs them to the agent server (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1925-2006`). **Failures are logged but not retried** — rows are deleted regardless of success (`live_status_app_conversation_service.py:1994-2006`), so a transient HTTP error silently loses the queued message.

4. **Sandbox persistence / orphaned containers**: `RemoteSandboxService` treats the runtime `/list` endpoint as the source of truth for "running" sandboxes — stale DB rows are excluded (`openhands/app_server/sandbox/remote_sandbox_service.py:375-396`). This is the only auto-reconciliation in the codebase. `DockerSandboxService` enumerates live containers via `docker.containers.list(all=True)` (`openhands/app_server/sandbox/docker_sandbox_service.py:291`) on every `search_sandboxes` call, so a container the DB lost track of simply does not appear; the reverse (DB has a row but no container) is not detected.

5. **Event callback persistence**: Callbacks are persisted to `event_callback` with status `ACTIVE`/`DISABLED`/`COMPLETED`/`ERROR` (`openhands/app_server/event_callback/event_callback_models.py:33-38, 92`); `SQLEventCallbackService.execute_callbacks` only fires `ACTIVE` callbacks (`openhands/app_server/event_callback/sql_event_callback_service.py:208-231`). Failures are caught and stored as `EventCallbackResultStatus.ERROR` (`sql_event_callback_service.py:241-249`), but there is no automatic retry/backoff and no dead-letter — retries happen by keeping the row `ACTIVE` and waiting for the next event of the same kind (see `SetTitleCallbackProcessor` at `openhands/app_server/event_callback/set_title_callback_processor.py:52-79, 132-138`, which polls up to 4× with a 3 s delay on title retrieval and then "keeps the callback active so later message events can retry").

What OpenHands does **not** do: there is no startup-time sweeper for orphaned `WORKING` start tasks or stale pending messages; no automatic requeue/retry of failed agent-server POSTs; no dead-letter queue for failed callbacks; no idempotency token on conversation creation (re-POSTing `/app-conversations` would create a duplicate row because `app_conversation_id` is only written at READY); no periodic job that marks a sandbox whose DB row says `RUNNING` but whose runtime says `MISSING` as failed; the `app_conversation_start_task` table grows unboundedly across restarts when users never delete their stuck tasks.

## Rating

**5 / 10** — DB schema is auto-migrated on boot and the runtime is the source of truth for remote sandboxes, but in-flight run state (`AppConversationStartTask`), pending message queue retries, and event-callback retries are all best-effort with no restart-time reconciliation, no DLQ, and minimal test coverage for crash scenarios. The five core questions each have at best a partial answer.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Lifespan abstraction (abstract) | `AppLifespanService` interface with `__aenter__`/`__aexit__` | `openhands/app_server/app_lifespan/app_lifespan_service.py:10-21` |
| OSS lifespan runs alembic on every boot | `run_alembic_on_startup` flag and `command.upgrade(..., 'head')` | `openhands/app_server/app_lifespan/oss_app_lifespan_service.py:12-37` |
| Lifespan wired into FastAPI app | `app_lifespan_ = get_app_lifespan_service()` then added to lifespans | `openhands/app_server/app.py:48-58` |
| Alembic env (imports model registry) | registers `StoredConversationMetadata`, `StoredAppConversationStartTask`, `StoredEventCallback`, `StoredRemoteSandbox` | `openhands/app_server/app_lifespan/alembic/env.py:21-34` |
| Alembic README — startup automation note | "Migrations are applied automatically on startup" | `openhands/app_server/app_lifespan/alembic/README:3-7` |
| Start-task persistence on each status yield | `await self.app_conversation_start_task_service.save_app_conversation_start_task(task)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-307` |
| Start-task status enum (no terminal-on-crash) | `WORKING / WAITING_FOR_SANDBOX / PREPARING_REPOSITORY / RUNNING_SETUP_SCRIPT / SETTING_UP_GIT_HOOKS / SETTING_UP_SKILLS / STARTING_CONVERSATION / READY / ERROR` | `openhands/app_server/app_conversation/app_conversation_models.py:262-271` |
| Background consumption of start-task iterator | `asyncio.create_task(_consume_remaining(async_iter, db_session, httpx_client))` | `openhands/app_server/app_conversation/app_conversation_router.py:405` |
| Iterator drain helper (closes sessions) | `_consume_remaining` consumes until `StopAsyncIteration` | `openhands/app_server/app_conversation/app_conversation_router.py:1619-1631` |
| Error path sets task to ERROR with redacted detail | `_logger.exception(...)` + `task.status = ERROR` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:534-538` |
| Pending message schema + service interface | `StoredPendingMessage` table and `PendingMessageService` ABC | `openhands/app_server/pending_messages/pending_message_service.py:27-72` |
| Pending message SQL add (no idempotency token) | `add_message` writes a new row with random id; no dedupe on `(conversation_id, content)` | `openhands/app_server/pending_messages/pending_message_service.py:81-120` |
| Pending message rewrite (task-id → real id) | `update_conversation_id(old, new)` | `openhands/app_server/pending_messages/pending_message_service.py:168-185` |
| Pending message router rate-limit (10/conversation) | `pending_count >= 10` → HTTP 429 | `openhands/app_server/pending_messages/pending_message_router.py:85-91` |
| Pending message delivery (best-effort, no retry) | catches `Exception`, logs warning, deletes anyway | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1977-2006` |
| Remote sandbox source-of-truth reconciliation | `_get_user_running_sandboxes` intersects `/list` runtime result with DB rows | `openhands/app_server/sandbox/remote_sandbox_service.py:375-396` |
| Same comment emphasises exclusion of stale rows | "stale or expired DB rows are automatically excluded" | `openhands/app_server/sandbox/remote_sandbox_service.py:381` |
| Remote batch fallback when runtime unavailable | `batch_get_sandboxes` catches exception, falls back to empty runtimes | `openhands/app_server/sandbox/remote_sandbox_service.py:623-662` |
| Docker sandbox search enumerates containers | `docker_client.containers.list(all=True)` filtered by prefix | `openhands/app_server/sandbox/docker_sandbox_service.py:283-326` |
| Docker container status mapping | "restarting"/"exited"/"paused"/"dead" → `STARTING`/`PAUSED`/`PAUSED`/`ERROR` | `openhands/app_server/sandbox/docker_sandbox_service.py:115-127` |
| Docker resume behaviour for `paused` and `exited` | `container.unpause()` or `container.start()` | `openhands/app_server/sandbox/docker_sandbox_service.py:517-534` |
| Container startup grace period | `startup_grace_seconds` (default 15) — past that, sandbox is `ERROR` | `openhands/app_server/sandbox/docker_sandbox_service.py:43, 257-280` |
| Process-sandbox resume uses psutil | `process.resume()` if `STATUS_STOPPED` | `openhands/app_server/sandbox/process_sandbox_service.py:359-371` |
| Process-sandbox delete force-kills on timeout | graceful terminate then `process.kill()` | `openhands/app_server/sandbox/process_sandbox_service.py:387-422` |
| Polling task started when no public webhook URL | `polling_task = asyncio.create_task(poll_agent_servers(...))` (module-level singleton) | `openhands/app_server/sandbox/remote_sandbox_service.py:682-768, 919-932` |
| `poll_agent_servers` fetches runtimes then refreshes conversations | "fetch → release → network → re-acquire → write" pattern | `openhands/app_server/sandbox/remote_sandbox_service.py:699-769` |
| `refresh_conversation` calls `execute_callbacks` after event save | only if `existing is None` (idempotent dedupe by event id) | `openhands/app_server/sandbox/remote_sandbox_service.py:852-867` |
| Event callback status enum | `ACTIVE / DISABLED / COMPLETED / ERROR` | `openhands/app_server/event_callback/event_callback_models.py:33-38` |
| Callback execution only fires `ACTIVE` | `StoredEventCallback.status == EventCallbackStatus.ACTIVE` | `openhands/app_server/event_callback/sql_event_callback_service.py:208-216` |
| Callback exception captured into result row | no DLQ; row stays `ACTIVE` for next event | `openhands/app_server/event_callback/sql_event_callback_service.py:233-249` |
| `SetTitleCallbackProcessor` retry-by-event model | "Keep the callback active so later message events can retry" | `openhands/app_server/event_callback/set_title_callback_processor.py:67-73, 132-138` |
| Title poll loop (4 attempts × 3 s) | `_POLL_DELAY_S = 3`, `_NUM_POLL_ATTEMPTS = 4` | `openhands/app_server/event_callback/set_title_callback_processor.py:31-34, 52-79` |
| Webhook event delivery is fire-and-forget in background | `asyncio.create_task(_run_callbacks_in_bg_and_close(...))` | `openhands/app_server/event_callback/webhook_router.py:513-517, 561-573` |
| App conversation deletion cleans up start tasks | `_delete_from_database` calls `delete_app_conversation_start_tasks(id)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2271-2290` |
| Sandbox delete in background after conversation deletion | `asyncio.create_task(_finalize_sandbox_delete(...))` | `openhands/app_server/app_conversation/app_conversation_router.py:966-976` |
| `_finalize_sandbox_delete` cleans sandbox only if unused | `count_conversations_by_sandbox_id(sandbox_id)` then `delete_sandbox` | `openhands/app_server/app_conversation/app_conversation_router.py:868-889` |
| `pause_old_sandboxes` is the only "scheduler" cleanup | called by `start_sandbox` and `resume_sandbox`; pauses oldest beyond `max_num_sandboxes` | `openhands/app_server/sandbox/sandbox_service.py:203-244` and `remote_sandbox_service.py:419-425, 508-509` |
| `pause_old_sandboxes` (remote) uses runtime list as truth | pauses oldest from `_get_user_running_sandboxes()` | `openhands/app_server/sandbox/remote_sandbox_service.py:598-621` |
| `get_app_conversation` fallback to MISSING | `sandbox_status = SandboxStatus.MISSING` when DB has no sandbox row | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:637-641` |
| Agent-server context helper returns 404 for non-running | `if sandbox.status != SandboxStatus.RUNNING: return 404` | `openhands/app_server/app_conversation/app_conversation_router.py:170-178` |
| `live_status` service reads runtime conversation state | `_get_live_conversation_info` issues GET `/api/conversations?ids=...` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:589-628` |
| Connection pool reuse warning on GCS | "Creating one per request leads to pool exhaustion under load" | `openhands/app_server/event/google_cloud_event_service.py:31-32` |
| `depends_db_session` regression test exists for crashloop | regression test for the `depends_db_session() at module load` RecursionError | `tests/unit/app_server/test_db_session_dependency.py:1-66` |
| No test for `OssAppLifespanService.run_alembic` | `grep` returns nothing under `tests/unit/app_server/` | (no evidence found) |
| No test for `delete_app_conversation_start_tasks` | `grep` returns nothing under `tests/unit/app_server/test_sql_app_conversation_start_task_service.py` | (no evidence found) |
| No test for crash mid-start (no simulated server SIGKILL) | `grep` for "crash" in `tests/unit/app_server/` returns only unrelated `agent crashed`/`runtime crashed` strings | (no evidence found) |

## Answers to Dimension Questions

1. **What happens to in-progress work after restart?**
   - **DB schema is fine** — `OssAppLifespanService.__aenter__` runs `alembic upgrade head` on boot (`openhands/app_server/app_lifespan/oss_app_lifespan_service.py:15-18`), so a partially-applied schema is brought forward. Migrations are versioned, additive, and idempotent.
   - **In-progress conversation starts are not recovered.** The `app_conversation_start_task` row persists at whatever non-terminal status it was last saved with (`AppConversationStartTaskStatus` enum at `openhands/app_server/app_conversation/app_conversation_models.py:262-271`). The `_consume_remaining` task (`openhands/app_server/app_conversation/app_conversation_router.py:1619-1631`) ran in the dead worker's memory; nothing in the boot path reads these rows, marks them `ERROR`, or replays them. The `app_conversation_id` field stays `None` (`openhands/app_server/app_conversation/app_conversation_models.py:293-295`), so the user can poll `GET /start-tasks/search` (`openhands/app_server/app_conversation/app_conversation_router.py:996-1035`) and see a forever-`WORKING` row. Cleanup only happens on `DELETE /conversations/{id}` (`live_status_app_conversation_service.py:2271-2290`).
   - **Pending user messages queued during start are recovered only if the start succeeded and reached `READY`.** `_process_pending_messages` (`live_status_app_conversation_service.py:1925-2006`) is invoked at the tail of a successful start (`live_status_app_conversation_service.py:525-533`). If the start task is killed mid-flight, queued messages stay in the `pending_messages` table forever — there is no `pending_message.on_app_start` or similar. The router caps at 10/conversation (`pending_message_router.py:85-91`) but never expires old rows.
   - **Sandbox state is reconciled.** Remote sandboxes are filtered through `/list` (`remote_sandbox_service.py:375-396`), and Docker sandboxes are enumerated from the daemon (`docker_sandbox_service.py:291`). So a sandbox the DB thinks exists but the runtime does not is silently dropped from results.
   - **Event callbacks are durable** — the `event_callback` table is the source of truth. Failure rows are recorded in `event_callback_result` (`sql_event_callback_service.py:76-90`), but retry is opportunistic: the callback stays `ACTIVE` until the next event of the same `event_kind` arrives (`set_title_callback_processor.py:132-138`).

2. **Are orphaned runs detected?**
   - **Orphaned conversation-start tasks: NO.** No code path scans `app_conversation_start_task` on boot or on a schedule. The only operator surface is the `GET /start-tasks/search` and `GET /start-tasks/count` endpoints (`app_conversation_router.py:996-1074`) which list but do not repair.
   - **Orphaned sandboxes: partial.** `RemoteSandboxService.batch_get_sandboxes` tolerates a 404 from `/list` (`remote_sandbox_service.py:623-662`), so a sandbox whose runtime crashed is reported as `MISSING`. But `pause_old_sandboxes` is the only *proactive* cleanup and it only fires when a new sandbox is started or resumed (`remote_sandbox_service.py:419-425, 508-509`), not on a timer.
   - **Orphaned pending messages: NO.** `pending_messages` rows are written without an `expires_at` column (`pending_message_service.py:30-37`). They are only deleted on successful delivery (`live_status_app_conversation_service.py:1997-2006`) or implicitly via `delete_messages_for_conversation` (no caller invokes it after start failure).

3. **Is recovery automatic?**
   - **Schema: YES** — alembic `upgrade head` on boot.
   - **Sandbox reconciliation: PARTIAL** — runtime is the source of truth on read paths.
   - **In-progress conversation starts: NO.** The user must delete the orphaned task (which now means losing the partial start) and re-create.
   - **Pending messages queued during a crashed start: NO** — they persist but no code replays them when/if the user re-attempts the conversation.
   - **Event callbacks: PARTIAL** — durable across restarts, but retries only fire on the next event; there is no timer-driven retry.

4. **Is recovery idempotent?**
   - **`save_app_conversation_start_task` is merge-based** (`sql_app_conversation_start_task_service.py:235-248`) so re-saving the same task with the same id overwrites — idempotent at the row level.
   - **`delete_app_conversation_start_tasks` returns `rowcount > 0`** (`sql_app_conversation_start_task_service.py:250-271`), so calling it twice is safe.
   - **`start_app_conversation` is NOT idempotent** — it does not check for an existing in-progress task with the same `conversation_id` before issuing the POST `/api/conversations` to the agent server (`live_status_app_conversation_service.py:436-459`). A double-click from the user can create two start tasks and (depending on agent-server semantics) two agent conversations.
   - **`refresh_conversation` (poll path) skips events that already exist** (`remote_sandbox_service.py:859-867`: `if existing is None: save_event`), so re-polling the same runtime state is idempotent.
   - **`SetTitleCallbackProcessor` flips itself to `DISABLED` after success** (`set_title_callback_processor.py:151-152`), so a re-fired event does not re-set the title.
   - **`pause_old_sandboxes` is idempotent at the runtime level** — pausing an already-paused sandbox is a no-op on remote (`remote_sandbox_service.py:613-621`).

5. **Are users/operators notified?**
   - **Users: via analytics.** Failed start tasks are NOT user-visible beyond the `AppConversationStartTask.detail` field (`live_status_app_conversation_service.py:536-538`). The analytics layer does fire `track_conversation_created` (`app_conversation_router.py:390-401`) and `track_conversation_deleted` (`app_conversation_router.py:945-957`), but there is no `track_start_task_failed` hook. Users discover stuck starts only by polling `GET /start-tasks/{id}`.
   - **Operators: via logs and metrics.** `_logger.exception(...)` is called on every failure path (`live_status_app_conversation_service.py:535, 624, 949`, `remote_sandbox_service.py:343-343, 369-372, 498, 569, 595, 760-762`, `docker_sandbox_service.py:269-271`). The polling task itself logs `Found {n} conversations to check` / `Matched ...` (`remote_sandbox_service.py:736-756`). There is **no built-in alerting** for orphaned start tasks or stuck sandboxes — operators must wire Sentry/Datadog off the log lines themselves. The only `TODO` for orphan-style work is `opentelemetry/opentelemetry-collector#...` style notes, not in this repo.

## Architectural Decisions

- **DB as durable store for in-flight state, no in-memory worker registry.** Every long-running operation writes a row (`app_conversation_start_task`, `event_callback`, `pending_messages`) before yielding. This means the row is correct, but the **worker that owns it is invisible to other replicas**. There is no `owner_id`/`last_heartbeat` column on `app_conversation_start_task` (`sql_app_conversation_start_task_service.py:54-74`), so cross-replica takeover is impossible.
- **Runtime (Docker / remote runtime / psutil) is authoritative for sandboxes.** `_get_user_running_sandboxes` (`remote_sandbox_service.py:375-396`) and `containers.list(all=True)` (`docker_sandbox_service.py:291`) both treat the live runtime as truth. This is the strongest auto-reconciliation in the codebase and the right model for ephemeral compute.
- **Background polling replaces webhooks when there is no public URL.** `poll_agent_servers` (`remote_sandbox_service.py:682-768`) is the recovery path for environments without an inbound webhook route — it runs every 15 s by default and replays events into the app server. This effectively rebuilds "lost webhook deliveries" by polling, at the cost of constant polling load.
- **Event callbacks are not retried by timer.** The model is "next event of the same kind triggers another attempt" (`set_title_callback_processor.py:67-73, 132-138`). This works for high-traffic callbacks (auto-title on the first `MessageEvent`) but is invisible for low-traffic ones — a callback that never sees another event never retries.
- **Start task is the user's progress signal.** Status enum is fully enumerated (`WORKING / WAITING_FOR_SANDBOX / PREPARING_REPOSITORY / RUNNING_SETUP_SCRIPT / SETTING_UP_GIT_HOOKS / SETTING_UP_SKILLS / STARTING_CONVERSATION / READY / ERROR`, `app_conversation_models.py:262-271`). The `detail` field carries the redacted exception on `ERROR` (`live_status_app_conversation_service.py:537`), so the UI can render the failure reason. The model assumes the worker that started the task is the worker that finishes it.

## Notable Patterns

- **Sandbox source-of-truth reconciliation** (`remote_sandbox_service.py:375-396`, `docker_sandbox_service.py:291`) — every read path intersects DB and runtime, so stale rows silently disappear.
- **Graceful degradation over retries** (`live_status_app_conversation_service.py:1994-2006`) — pending message delivery logs and deletes on failure rather than retrying. Cheaper, but loses messages on transient HTTP errors.
- **Fetch → release → network → re-acquire → write** (`remote_sandbox_service.py:688-768, 770-872`) — explicit anti-pattern of holding DB sessions across network I/O, applied to both the poller and `refresh_conversation`. The pattern is documented in code comments and provides predictable resource use at the cost of extra round-trips.
- **Background callback fan-out** (`webhook_router.py:513-517`, `sql_event_callback_service.py:208-231`) — webhooks return `200 Success` immediately after persisting events; callbacks fire in `asyncio.create_task` so the agent-server is not held up. If the app server dies between persisting the event and running callbacks, the next webhook for the same conversation will replay the events (because `refresh_conversation` only saves events that are missing).
- **Status synthesis on Redis-lock unavailability** (`live_status_app_conversation_service.py:2354-2399`) — the export path tolerates `RedisLockUnavailable` in non-SAAS mode and proceeds without a lock. Different from `ConversationExportLockUnavailable` (which is raised in SAAS), but the same idea: lock failure must not block the user.
- **`__aexit__` is uniformly empty** (`app_lifespan/app_lifespan_service.py:20-21`, `oss_app_lifespan_service.py:20-21`, `sql_event_callback_service.py:252-254`) — the lifespan services are designed for "best-effort startup hook" semantics, not for coordinating shutdown.
- **`update_app_conversation` model mutation uses `model_copy`** (`app_conversation_router.py` — not the dimension's focus but found) — Pydantic v2's `model_copy(update={...})` is used to swap the `llm_model` field atomically; relevant because the surrounding code uses the same idiom for partial updates (`live_status_app_conversation_service.py:1903-1912`).

## Tradeoffs

- **No startup sweeper for `app_conversation_start_task` rows** means a row can stay `WORKING` forever. This is intentional (the operator can `GET /start-tasks/search` and `DELETE` manually) but does not scale — there is no TTL column, so the table grows monotonically until a conversation is created or deleted.
- **Runtime-as-truth for sandboxes** is robust for read paths but blind to DB-side problems (e.g., an `app_conversation` row pointing at a sandbox id that no runtime knows about) — `MISSING` is reported but never reconciled back into the DB.
- **Best-effort pending-message delivery** trades throughput for liveness — a single agent-server 503 silently drops the user's queued message rather than blocking the start task or retrying.
- **Event-callback retry-by-event** is good for title-setting (always fires on the first message) and bad for anything one-shot — there is no scheduler that says "retry this callback N hours from now".
- **`pause_old_sandboxes` only fires on start/resume** (`remote_sandbox_service.py:419-425, 508-509`). A pod that never starts a sandbox never reaps old ones. There is no `apscheduler`/`asyncio.create_task` running on a timer for cleanup.
- **DB migration is non-atomic per migration file** — `command.upgrade(..., 'head')` is one transaction per migration (`alembic/env.py:91-92, 107-111`). If the boot alembic run is SIGKILLed mid-way, the next boot will pick up where it left off because alembic writes to `alembic_version`. There is no operator-facing log line that says "migration N/M applied" beyond alembic's own stdout.

## Failure Modes / Edge Cases

- **App server SIGKILL between `save_app_conversation_start_task(WORKING)` and the next `yield`** — the row is `WORKING` with no `app_conversation_id`. Restart leaves the row unchanged. The user's UI sees "Starting..." forever until they cancel/delete.
- **App server SIGKILL during `_process_pending_messages`** — already-delivered messages have been POSTed to the agent-server but the local row has not yet been deleted (`live_status_app_conversation_service.py:1980-2006`). On next start the same task-id rewrite runs, the conversation is now real, and the messages are *re-POSTed* (because `delete_messages_for_conversation` is keyed on `conversation_id_str`, not on event payload). **Double-delivery is possible** if the agent-server doesn't dedupe by event id.
- **Agent-server returns 500 during conversation create** — task transitions to `ERROR` with `detail` populated (`live_status_app_conversation_service.py:534-538`), the user sees the error, and the row stays for inspection. No retry button.
- **Sandbox runtime restart while a start task is mid-flight** — `RemoteSandboxService._get_runtime` raises an HTTP error caught at `remote_sandbox_service.py:341-347`; the `_wait_for_sandbox_start` helper in `live_status_app_conversation_service.py:804-872` re-raises as `SandboxError`. The start task goes to `ERROR`.
- **DB connection drop during alembic upgrade** — alembic writes `alembic_version` inside the failed transaction; on next boot, alembic retries from the last committed version. No app-level retry counter; relies on alembic's idempotency.
- **Docker daemon restart while a sandbox is RUNNING** — `docker.containers.list(all=True)` will return the container with status `'running'` again (`docker_sandbox_service.py:115-127`). No app-level reconciliation needed; the container's `StartedAt` is updated, so the startup-grace check at `docker_sandbox_service.py:257-280` does not flip it to `ERROR`.
- **Polling task dies** — `polling_task: asyncio.Task | None = None` is module-level (`remote_sandbox_service.py:54`) and recreated lazily by the injector (`remote_sandbox_service.py:919-932`). If the task raises an unhandled exception it ends; the next request to a remote sandbox recreates it. There is no `done_callback` to log a failure.
- **Webhook delivery 5xx from agent-server** — `webhook_router.py:519-520` swallows the exception and returns 200 to the caller; the events have already been persisted (`webhook_router.py:464-466`) so the next poll cycle will replay them via `refresh_conversation` (if polling is enabled) but not via webhook replay.
- **Title never becomes available** — `SetTitleCallbackProcessor` keeps the callback `ACTIVE` indefinitely (`set_title_callback_processor.py:132-138`). A conversation with no further `MessageEvent`s will accumulate `EventCallbackResult` rows but never close out. No max-attempts cap.
- **Operator uses `DELETE /conversations/{id}` while a start task is mid-flight** — the start task is deleted via `delete_app_conversation_start_tasks` (`live_status_app_conversation_service.py:2286-2288`) but the agent-server create-call may already be in flight; subsequent `STARTING_CONVERSATION` saves go to a row that has been deleted. No FK constraint catches this (`sql_app_conversation_start_task_service.py:54-74` has no `FOREIGN KEY`).

## Future Considerations

- **Add `last_heartbeat_at` + `worker_id` to `app_conversation_start_task` and a sweeper.** Without these, no other replica can take over an orphaned start. The existing schema at `sql_app_conversation_start_task_service.py:54-74` has room for the columns.
- **Switch pending-message delivery to a real outbox table with retries.** The current "POST then delete" pattern (`live_status_app_conversation_service.py:1977-2006`) silently loses messages on 5xx. An outbox with `attempts`, `next_attempt_at`, and `delivered_at` would be a drop-in upgrade.
- **Add a max-attempts counter and DLQ for `EventCallback`.** Today the model is "ACTIVE forever until success" (`set_title_callback_processor.py:132-138`). A bounded retry count + a `failed_callbacks` table would make dead-letter inspection possible.
- **Add a periodic sandbox reaper.** `pause_old_sandboxes` only runs on start/resume (`remote_sandbox_service.py:419-425, 508-509`). An `asyncio.create_task` started in `RemoteSandboxServiceInjector.inject` (after `remote_sandbox_service.py:933-938`) could run cleanup every N minutes regardless of user activity.
- **Make `start_app_conversation` idempotent by `conversation_id`.** Today nothing prevents two concurrent start tasks for the same `conversation_id` from racing the agent-server POST (`live_status_app_conversation_service.py:300-459`).
- **Wire `track_start_task_failed` analytics.** The `app_conversation_router.py:382-410` block fires `track_conversation_created` but never `track_start_task_failed`; users have no aggregate signal.
- **Add a test for `OssAppLifespanService.run_alembic`.** The `tests/unit/app_server/` tree has no test that exercises the boot migration path; the only lifespan-adjacent test is `tests/unit/app_server/test_db_session_dependency.py:1-66` (which is a regression for a different bug).
- **Add a test for `delete_app_conversation_start_tasks`.** The current `tests/unit/app_server/test_sql_app_conversation_start_task_service.py` covers save/get/search but not delete.

## Questions / Gaps

- **Is `start_app_conversation` safe to call twice in quick succession from the same user?** No evidence in `live_status_app_conversation_service.py:300-459` of a uniqueness check on `(conversation_id, user_id)` before the agent-server POST. Could a double-click produce two agents?
- **Does the polling task survive a worker-pool restart?** `polling_task` is module-level (`remote_sandbox_service.py:54`), so on app-server reload the next `RemoteSandboxServiceInjector.inject` call recreates it (`remote_sandbox_service.py:924-932`). But the docstring at `remote_sandbox_service.py:682-691` says "DB sessions are scoped tightly to avoid holding connections across network I/O" — does the recreated task honour this? Yes, the inner code uses fresh `get_db_session(state)` blocks (`remote_sandbox_service.py:725-731`).
- **Is there any path that emits metrics for "start_task stuck > N minutes"?** No `prometheus_client.Counter`/`Histogram` registration found in `app_server/` for this; operators must rely on logs.
- **Are `app_conversation_start_task` rows ever hard-deleted on TTL?** No TTL column; only deleted when the conversation is deleted (`live_status_app_conversation_service.py:2286-2288`). What happens to a `WORKING` row whose `conversation_id` was never set? It survives forever and is invisible to `GET /start-tasks/search?conversation_id__eq=...` because the filter is on `app_conversation_id` not on `request.conversation_id` (`sql_app_conversation_start_task_service.py:105-109`).
- **Are `app_conversation` rows with `sandbox_id` pointing to a deleted runtime ever auto-cleaned?** The frontend shows them as `MISSING` (per `live_status_app_conversation_service.py:637-641`) but the row stays. No GC.
- **What happens if alembic fails mid-upgrade?** `oss_app_lifespan_service.py:15-18` raises inside `__aenter__`, so the FastAPI app fails to start. There is no retry-then-skip logic; the pod crash-loops until the DB is reachable.

---

Generated by `02.08-crash-recovery-and-reconstruction` against `openhands`.