# Source Analysis: openhands

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12+ (FastAPI, Pydantic, SQLAlchemy async, boto3, httpx) — backend under `openhands/`, with the agent loop itself externalised in `openhands-sdk` / `openhands-agent-server` PyPI packages (referenced via `from openhands.sdk ...` imports) |
| Analyzed | 2026-07-02 |

## Summary

OpenHands splits the atomicity question across **two layers with different atomicity models**:

1. **The agent loop itself** (step / turn / tool-call) lives in the external `openhands-sdk` and `openhands-agent-server` PyPI packages. The in-repo `app_server` does NOT define a step, turn, or task unit; it treats the SDK's `Event` as an opaque record whose internal sub-structure is defined elsewhere.
2. **The app-server's own atomic vocabulary** is five concrete units:
   - `Event` (per-event persistence via `EventServiceBase.save_event` at `openhands/app_server/event/event_service_base.py:190-197`),
   - `AppConversationStartTask` (coarse multi-step "start a conversation" workflow with `AppConversationStartTaskStatus` enum at `openhands/app_server/app_conversation/app_conversation_models.py:262-271`),
   - `AppConversationInfo` (the persisted metadata row in `conversation_metadata` at `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:65-117`),
   - `PendingMessage` (a pre-start message buffered in `pending_messages` table at `openhands/app_server/pending_messages/pending_message_service.py:27-38`),
   - `EventCallback` (a side-effect subscription, one per `event_kind`, persisted in `event_callback` table at `openhands/app_server/event_callback/sql_event_callback_service.py:48-73`).

The `Event` is the only fine-grained unit the app-server persists (one JSON file per event_id in `v1_conversations/{conversation_id_hex}/{event_id_hex}.json`, see `openhands/app_server/conversation_paths.py:11-35` and `openhands/app_server/event/filesystem_event_service.py:33-36`). Persistence is append-only with no batching or transactional boundary across multiple events — `save_event` is one `write_text()` call (`openhands/app_server/event/filesystem_event_service.py:33-36`); partial mid-event saves are possible only if a write is interrupted, and there is no detection or recovery code in the app_server for that case.

The `AppConversationStartTask` is the only "task"-like unit that has explicit intermediate states and a terminal state machine (`WORKING → WAITING_FOR_SANDBOX → PREPARING_REPOSITORY → RUNNING_SETUP_SCRIPT → SETTING_UP_GIT_HOOKS → SETTING_UP_SKILLS → STARTING_CONVERSATION → READY (or ERROR)` at `openhands/app_server/app_conversation/app_conversation_models.py:262-271`), and it is yielded as an async stream from `LiveStatusAppConversationService._start_app_conversation` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:309-538`). Each yield is persisted via `app_conversation_start_task_service.save_app_conversation_start_task` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:303-307`), giving a coarse-grained checkpoint trail but no transactional guarantee that all side effects up to that yield succeeded.

The "turn" vocabulary is absent from the app-server: `turn_count` is exposed as an analytics field (`openhands/analytics/analytics_service.py:227, 245, 263, 279`) but the app-server always passes `None` for it (`openhands/app_server/event_callback/webhook_router.py:162, 180`), so there is no persisted turn counter in this codebase. The "step" vocabulary is absent: there is no `Step` class, no step counter, no step record anywhere in `openhands/app_server/`. Tool-call atomicity is also opaque: the app-server just receives `ActionEvent` and `ObservationEvent` as `Event` instances via webhook (see `openhands/app_server/event_callback/webhook_router.py:57` and `454-466`) without distinguishing or pairing them.

Crash semantics: if `save_event` raises inside the webhook handler at `openhands/app_server/event_callback/webhook_router.py:464-466`, the entire batch is wrapped in a single `try/except` (`openhands/app_server/event_callback/webhook_router.py:462-521`); a mid-batch failure logs and returns `Success` anyway (`openhands/app_server/event_callback/webhook_router.py:520-522`), so the user is told the call worked but some events may be missing. There is no rollback for partially-saved events.

Tracing and observability are external: events carry an `id` (UUID) and `timestamp` from the SDK model, but no spans, traces, or hierarchical context in the app_server. The only "execution_status" tracking is `ConversationExecutionStatus` from `openhands.sdk.conversation` (referenced at `openhands/app_server/event_callback/webhook_router.py:56`), read live from the sandbox via `/api/conversations` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:608-617`), and surfaced on `AppConversation.execution_status` (`openhands/app_server/app_conversation/app_conversation_models.py:178-181`).

Retry: there is no per-event retry in the app_server. The only retry mechanisms are (a) `SetTitleCallbackProcessor._poll_for_title` polling up to 4 times at 3s intervals (`openhands/app_server/event_callback/set_title_callback_processor.py:32-34, 52-53`), and (b) Redis lock acquisition with TTL for export (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2354-2421`). Pending messages are NOT retried: `_process_pending_messages` deletes them after a single delivery attempt (no retry on transport error visible in this layer — only `pending_message_service.delete_messages_for_conversation` is called via `update_conversation_id` at `openhands/app_server/pending_messages/pending_message_service.py:62-72`).

## Rating

**6/10** — The app_server has a clear and consistent atomic unit (the `Event` from the SDK) that is persisted, exposed via REST, and replay-able via export (`iter_events_for_export` at `openhands/app_server/event/event_service_base.py:145-156` plus `_stream_conversation_zip` at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2328-2352`), and the multi-step start workflow has an explicit `AppConversationStartTaskStatus` enum with persisted checkpoints. However, the atomicity is **the wrong shape for "what completed and what did not"**: (a) there is no transactional boundary around a step or turn, so a crash mid-batch leaves an unknown number of events persisted, (b) `turn_count` is always `None` in this layer (webhook_router:162, 180) so users cannot see turn progress, (c) callbacks are processed AFTER the HTTP 200 to the sandbox webhook is already returned (via `asyncio.create_task(_run_callbacks_in_bg_and_close, ...)` at `openhands/app_server/event_callback/webhook_router.py:513-517`), meaning callbacks can fail silently without affecting user-visible state, (d) the app_server cannot distinguish between an ActionEvent and its matching ObservationEvent (it just iterates over events in timestamp order, see `iter_events_for_export` at `openhands/app_server/event/event_service_base.py:145-156`), so partial tool calls are visible only by post-hoc inspection. Not lower because the event-level atomicity IS observable and replay-able; not higher because the answer to "what completed and what did not" requires correlating events by timestamp + id across two systems (sandbox-side SDK state and app_server-side files).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event is the persisted atomic unit (filesystem) | `_store_event` writes one JSON file per event_id.hex | `openhands/app_server/event/filesystem_event_service.py:33-36` |
| Event is the persisted atomic unit (AWS S3) | `_store_event` does one `put_object` per event | `openhands/app_server/event/aws_event_service.py:54-62` |
| Event is the persisted atomic unit (Google Cloud) | `_store_event` writes one blob per event | `openhands/app_server/event/google_cloud_event_service.py` (referenced in `openhands/app_server/event/__init__.py`) |
| Path scheme for events | `path.parent.mkdir(parents=True, exist_ok=True); path.write_text(content)`; path is `v1_conversations/{conversation_id_hex}/{event_id_hex}.json` | `openhands/app_server/event/filesystem_event_service.py:33-36`; `openhands/app_server/conversation_paths.py:11-35` |
| No batch / transaction across events | `asyncio.gather(*[event_service.save_event(conversation_id, event) for event in events])` — each save is independent | `openhands/app_server/event_callback/webhook_router.py:464-466` |
| Webhook returns Success even on save failure | Outer `try/except` logs and returns `Success` | `openhands/app_server/event_callback/webhook_router.py:462-522` |
| AppConversationStartTaskStatus state machine | `WORKING → WAITING_FOR_SANDBOX → PREPARING_REPOSITORY → RUNNING_SETUP_SCRIPT → SETTING_UP_GIT_HOOKS → SETTING_UP_SKILLS → STARTING_CONVERSATION → READY (or ERROR)` | `openhands/app_server/app_conversation/app_conversation_models.py:262-271` |
| Start task yielded as async stream (coarse checkpoint trail) | `async for task in self._start_app_conversation(request): await ... save_app_conversation_start_task(task); yield task` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-307` |
| Start task is persisted per yield | `StoredAppConversationStartTask` table, `request` column JSON-encoded | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:54-74` |
| Status progression is documented in service comment | Status progression listed in abstract method docstring | `openhands/app_server/app_conversation/app_conversation_service.py:103-105` |
| No "step" or "turn" vocabulary in app_server | `grep -rn "class.*Step\|step_count\|turn_count"` returns only analytics references | `openhands/analytics/analytics_service.py:227, 245, 263, 279` |
| `turn_count` is always `None` at app_server boundary | Two call sites pass `turn_count=None` | `openhands/app_server/event_callback/webhook_router.py:162, 180` |
| `turn_count` is an analytics-only property | `track_conversation_finished(*, turn_count: int \| None = None)` | `openhands/analytics/analytics_service.py:227` |
| EventCallback is a per-event_kind subscription (one per kind) | `event_kind: ClassVar[EventKind] = 'MessageEvent'` default | `openhands/app_server/event_callback/event_callback_models.py:41, 84-87` |
| Callback execution is fire-and-forget AFTER HTTP 200 | `asyncio.create_task(_run_callbacks_in_bg_and_close(...))` | `openhands/app_server/event_callback/webhook_router.py:513-517` |
| Callbacks run sequentially per event, in background task | `for event in events: await event_callback_service.execute_callbacks(...)` | `openhands/app_server/event_callback/webhook_router.py:571-573` |
| Callback exceptions are caught and stored as ERROR result | `except Exception as exc: stored_result = StoredEventCallbackResult(status=ERROR, ...)` | `openhands/app_server/event_callback/sql_event_callback_service.py:241-249` |
| `PendingMessage` is queued before conversation exists (pre-start atomicity) | `pending_messages` table, conversation_id can be a task-{uuid} | `openhands/app_server/pending_messages/pending_message_service.py:27-38` |
| PendingMessageService has explicit `update_conversation_id` for task→conversation transition | Method signature: `update_conversation_id(self, old_conversation_id: str, new_conversation_id: str) -> int` | `openhands/app_server/pending_messages/pending_message_service.py:66-72` |
| Pending messages are NOT retried after delivery | Service has no retry helper; only `add_message`, `delete_messages_for_conversation` | `openhands/app_server/pending_messages/pending_message_service.py:45-72` |
| Pending message delivery happens after task is READY | `_process_pending_messages` invoked in `_start_app_conversation` after status=READY | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:520-532` |
| SetTitleCallbackProcessor polls up to 4 times for title | `_NUM_POLL_ATTEMPTS = 4`, `_POLL_DELAY_S = 3` | `openhands/app_server/event_callback/set_title_callback_processor.py:32-34` |
| SetTitleCallbackProcessor stays ACTIVE if title not yet available | `return None`; comment "Keep the callback active so later message events can retry." | `openhands/app_server/event_callback/set_title_callback_processor.py:132-138` |
| SetTitleCallbackProcessor disables itself after success | `callback.status = EventCallbackStatus.DISABLED` | `openhands/app_server/event_callback/set_title_callback_processor.py:151-152` |
| Conversation execution status is fetched live from sandbox | `response = await self.httpx_client.get(url, params={'ids': [...]}, ...)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:608-617` |
| `ConversationExecutionStatus` is from SDK, not app_server-defined | `from openhands.sdk.conversation import ConversationExecutionStatus` | `openhands/app_server/app_conversation/app_conversation_models.py:24` |
| Terminal-status detection on webhook | `if event.key != 'execution_status': continue; if exec_status.is_terminal(): await _track_conversation_terminal(...)` | `openhands/app_server/event_callback/webhook_router.py:500-509` |
| Error classification for analytics | `_classify_error_type` returns budget_exceeded / timeout / user_cancelled / model_error / runtime_error / unknown | `openhands/app_server/event_callback/webhook_router.py:70-90` |
| Stats event drives conversation metrics update | `if event.key == 'stats': await process_stats_event(event, conversation_id)` | `openhands/app_server/event_callback/webhook_router.py:469-473` |
| `process_stats_event` updates DB columns individually (no batch transaction) | Per-field `if X is not None: stored.X = X; ... await db_session.commit()` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:447-470` |
| AppConversationInfo persisted as row in `conversation_metadata` | `StoredConversationMetadata` table, versioned V0/V1 | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:65-117` |
| AppConversationInfo save is upsert via merge | `await self.db_session.merge(stored); await self.db_session.commit()` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:386-387` |
| Conversation events are exposed via REST | `/api/v1/conversation/{conversation_id}/events/{search,count,batch}` | `openhands/app_server/event/event_router.py:18-110` |
| Conversation events are exposed via REST pagination | `limit: int = 100, page_id: str \| None = None` | `openhands/app_server/event/event_router.py:50-55` |
| Pagination applies to items, not paths (regression-tested) | `start_offset = 0; items = items[start_offset:]` | `openhands/app_server/event/event_service_base.py:133-143` |
| Pagination test explicitly guards against duplicate-event bug | `assert item.id not in collected_event_ids` | `tests/unit/app_server/test_filesystem_event_service.py:241-244` |
| Event filters by kind, timestamp range | `kind__eq`, `timestamp__gte`, `timestamp__lt` | `openhands/app_server/event/event_service_base.py:94-143` |
| Event kinds are SDK-defined (not app_server-defined) | `EventKind = Literal[tuple(c.__name__ for c in get_known_concrete_subclasses(Event))]` | `openhands/app_server/event_callback/event_callback_models.py:28-30` |
| EventId round-trip uses `.hex` | `id_hex = event.id.replace('-', '')` then `f'{id_hex}.json'` | `openhands/app_server/event/event_service_base.py:191-195` |
| Batch save of multiple events uses gather (no transaction) | `await asyncio.gather(*[event_service.save_event(...)])` | `openhands/app_server/event_callback/webhook_router.py:464-466` |
| Per-event concurrency limit on load (not save) | `_event_load_concurrency() = max(1, int(os.getenv('EVENT_SERVICE_LOAD_EVENT_CONCURRENCY', '10')))` | `openhands/app_server/event/event_service_base.py:24-28` |
| Trajectory export streams events in timestamp order | `items.sort(key=lambda event: event.timestamp)` then yield | `openhands/app_server/event/event_service_base.py:153-156` |
| Trajectory export wrapped in Redis lock (export atomicity, not conversation) | `_EXPORT_LOCK_KEY_PREFIX = 'app_conversation_export'`; `try_acquire_redis_lock` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:144, 2362-2380` |
| Export lock refreshed periodically | `refresh_lock_periodically(export_lock, refresh_interval)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2397-2402` |
| Export validates max event count up front (atomicity bound) | `_validate_conversation_export_size` raises `ConversationExportTooLarge` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2304-2313` |
| Export zip file naming scheme | `f'event_{i:06d}_{event.id}.json'` (sequence by arrival) | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2343` |
| EventCallback is one per (conversation_id, event_kind) pair | Composite index `ix_event_callback_conversation_id_status_event_kind` | `openhands/app_server/event_callback/sql_event_callback_service.py:50-57` |
| EventCallback has explicit lifecycle states | `EventCallbackStatus = ACTIVE / DISABLED / COMPLETED / ERROR` | `openhands/app_server/event_callback/event_callback_models.py:33-37` |
| EventCallback result is persisted per (callback_id, event_id) | `StoredEventCallbackResult` table, `event_callback_id` and `event_id` indexed | `openhands/app_server/event_callback/sql_event_callback_service.py:76-89` |
| Sandbox is started as part of start task, not part of event atomicity | `task.status = WAITING_FOR_SANDBOX` then sandbox polling | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:835-872` |
| Sandbox timeout is configurable | `sandbox_startup_timeout: int = Field(default=120)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2444-2446` |
| Sandbox poll frequency is configurable | `sandbox_startup_poll_frequency: int = Field(default=2)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2447-2449` |
| Setup scripts are part of the start task (not a "step" in agent-loop sense) | `run_setup_scripts` yields each `task.status` change | `openhands/app_server/app_conversation/app_conversation_service_base.py:247-280` |
| Each setup-script step is yielded as task update | `task.status = PREPARING_REPOSITORY; yield task; await self.clone_or_init_git_repo(...)` etc. | `openhands/app_server/app_conversation/app_conversation_service_base.py:254-280` |
| `AppConversation.execution_status` is sourced live, not persisted | `execution_status = conversation_info.execution_status if conversation_info else None` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:639-641` |
| `LiveStatusAppConversationService` combines stored info + live status | `_build_app_conversations` batches sandbox lookups + live status | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:540-587` |
| `openhands.sdk.conversation` is the source of truth for `ConversationExecutionStatus` | Referenced from models file | `openhands/app_server/app_conversation/app_conversation_models.py:24` |
| Frontend sees execution status via `AppConversation.execution_status` | Field on `AppConversation` | `openhands/app_server/app_conversation/app_conversation_models.py:178-181` |
| Frontend handles `sandbox_status === "MISSING"` as archived | From AGENTS.md: `useAgentState` returns `AgentState.STOPPED` and `isArchived: true` | `AGENTS.md` (frontend section) |
| `useAgentState` prioritises live WebSocket execution status over cached API data | From AGENTS.md | `AGENTS.md` |
| File-system event save uses `path.write_text(content)` (no fsync, no atomic rename) | Direct call to `write_text` | `openhands/app_server/event/filesystem_event_service.py:36` |
| File-system event load tolerates corrupt JSON by returning None | `try: ... except Exception: ... return None` | `openhands/app_server/event/filesystem_event_service.py:23-31` |
| Corrupt events are silently skipped (not quarantined) | `_logger.exception('Error reading event', stack_info=True); return None` | `openhands/app_server/event/filesystem_event_service.py:28-31` |
| `count_events` with no filters counts paths (no event load) | `_count_events_no_filter` returns `len(paths)` | `openhands/app_server/event/event_service_base.py:184-188` |
| `count_events` with filters paginates and loads events | `events = page_iterator(self.search_events, ...)` | `openhands/app_server/event/event_service_base.py:172-182` |
| `iter_events_for_export` is the only "replay" iterator | `iter_events_for_export` yields all events once in timestamp order | `openhands/app_server/event/event_service_base.py:145-156` |
| `iter_events_for_export` allows backend override to avoid page reloads | Comment in abstract method | `openhands/app_server/event/event_service.py:48-58` |
| Sub-conversations are linked by parent_conversation_id | `parent_conversation_id: OpenHandsUUID \| None = None; sub_conversation_ids: list[OpenHandsUUID]` | `openhands/app_server/app_conversation/app_conversation_models.py:131-132` |
| Sub-conversation IDs are derived at read time, not persisted | `sub_conversation_ids=sub_conversation_ids or []` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:570` |
| Sub-conversation search queries parent_conversation_id column | `query = query.where(StoredConversationMetadata.parent_conversation_id == str(parent_conversation_id))` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:287-294` |
| `AppConversationStartTask.request` is JSON-serialized | `request: Mapped[AppConversationStartRequest] = mapped_column(create_json_type_decorator(AppConversationStartRequest))` | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:66-68` |
| `AppConversationStartTask` has its own status enum persisted | `status: Mapped[AppConversationStartTaskStatus \| None] = mapped_column(Enum(...))` | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:59-61` |
| `AppConversationStartTask.request` round-trip may be lossy for unknown fields | `request=AppConversationStartRequest.model_validate(row.request)` (re-validation) | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:149` |

## Answers to Dimension Questions

### 1. What is the atomic unit of execution?

There is no single atomic unit of execution in the app_server. The granularity ladder is:

- **Event** (`openhands.sdk.Event`, opaque) — the unit produced by the SDK agent loop, persisted as one JSON file/blob/object per event by the app_server (`openhands/app_server/event/event_service_base.py:190-197`). This is the **smallest** unit visible to and stored by the app_server.
- **AppConversationStartTask update** — one `task.status = X; yield task; await side_effect()` step in `_start_app_conversation` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:309-538`). The start workflow has 7 named intermediate states plus READY/ERROR (`openhands/app_server/app_conversation/app_conversation_models.py:262-271`).
- **PendingMessage** — one queued user message waiting for conversation READY (`openhands/app_server/pending_messages/pending_message_service.py:27-38`).
- **EventCallback invocation** — one `processor.__call__(conversation_id, callback, event)` invocation, persisted as one `EventCallbackResult` row (`openhands/app_server/event_callback/sql_event_callback_service.py:233-250`).
- **Conversation** — the highest unit, comprising all events + metadata + callbacks + pending messages, identified by a UUID and surfaced as `AppConversation` (`openhands/app_server/app_conversation/app_conversation_models.py:173-197`).

The "step" and "turn" vocabulary is **absent** from the app_server. The app_server does not know how many turns or steps an event represents; it treats each `Event` as opaque.

### 2. Is the atomic unit the same for persistence, tracing, retry, and UI?

**Partially.**

- **Persistence** uses the `Event` (via `save_event` at `openhands/app_server/event/event_service_base.py:190-197`), the `AppConversationInfo` row (via `merge` at `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:386-387`), the `AppConversationStartTask` row (via `merge` at `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:246-247`), the `PendingMessage` row (`openhands/app_server/pending_messages/pending_message_service.py:106-114`), the `EventCallback` row (`openhands/app_server/event_callback/sql_event_callback_service.py:202-206`), and the `EventCallbackResult` row (`openhands/app_server/event_callback/sql_event_callback_service.py:240-250`). The persistence units are different for different entities.
- **Tracing** uses no spans/OTel. The only trace-like identification is the event `id` (UUID) and `timestamp`. Conversation-level tracing is the `Laminar.set_trace_user_id` injected into the start body (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:419-430`), but this lives outside the per-event atomicity.
- **Retry** is not per-event; it is per-callback (via `SetTitleCallbackProcessor._poll_for_title` at `openhands/app_server/event_callback/set_title_callback_processor.py:32-34, 52-53`) and per-export (via Redis lock at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2362-2380`). Pending messages are NOT retried at all.
- **UI** sees the `Event` (via `/api/v1/conversation/{id}/events/search` at `openhands/app_server/event/event_router.py:29-67`), the `AppConversation` (with `execution_status` + `sandbox_status` at `openhands/app_server/app_conversation/app_conversation_models.py:174-186`), and the `AppConversationStartTask` (via `/api/v1/app-conversations/start-tasks/...` at `openhands/app_server/app_conversation/app_conversation_router.py:996-1075`). The UI distinguishes events by `kind`, but the app_server does not give the UI any way to reconstruct "step N completed" or "turn N in progress".

The units **diverge**: persistence is per-event, retry is per-callback, tracing is per-event-id, UI is per-conversation-and-event.

### 3. Can partially completed steps exist?

Yes, in multiple ways:

- **Mid-batch event save failure**: webhook calls `asyncio.gather(*[save_event(...)])` (`openhands/app_server/event_callback/webhook_router.py:464-466`). If some saves succeed and others fail (e.g., disk full mid-batch, S3 503), the conversation has a partial event set. The webhook handler then logs and returns `Success` (`openhands/app_server/event_callback/webhook_router.py:519-522`), so the sandbox does not retry — the partial state is permanent.
- **Mid-write filesystem failure**: `_store_event` calls `path.write_text(content)` (`openhands/app_server/event/filesystem_event_service.py:36`). If interrupted, you can have a partial JSON file. `_load_event` catches this and returns `None` (`openhands/app_server/event/filesystem_event_service.py:23-31`), so the corrupt event is silently skipped on read.
- **Mid-start-task failure**: `_start_app_conversation` yields after each `await side_effect()` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:309-538`), but the `AppConversationStartTask` row is only saved by the wrapping `start_app_conversation` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-307`). If the task is killed between a yield and the next yield (after a `await`), the conversation is in an intermediate state with status `WAITING_FOR_SANDBOX` (or whatever was last yielded) but no `app_conversation_id`. There is no automatic resume.
- **Mid-event-callback failure**: callbacks run in `asyncio.create_task(_run_callbacks_in_bg_and_close, ...)` (`openhands/app_server/event_callback/webhook_router.py:513-517`); if the task is killed, callbacks don't complete. SetTitleCallbackProcessor handles "title not yet available" by leaving the callback ACTIVE (`openhands/app_server/event_callback/set_title_callback_processor.py:132-138`), but other callback failures are stored as ERROR and the callback is left ACTIVE for retry on the next matching event (`openhands/app_server/event_callback/sql_event_callback_service.py:241-249`).
- **Mid-conversation-deleted**: a user may delete a conversation while callbacks are still queued (`openhands/app_server/app_conversation/app_conversation_router.py:892-978`); the callback processor will try to look up the deleted conversation and fail silently.

### 4. What happens if a crash occurs mid-step?

There is no recovery story in the app_server:

- **Mid-event-write**: the partial JSON file is silently ignored on next read (`openhands/app_server/event/filesystem_event_service.py:23-31`). No retry, no alert, no admin-visible signal.
- **Mid-start-task**: the `AppConversationStartTask` row remains in its last-yielded state (e.g., `WAITING_FOR_SANDBOX`). There is no automatic resume; the user must re-trigger. The `get_app_conversation_start_task` endpoint can still fetch the orphaned task (`openhands/app_server/app_conversation/app_conversation_router.py:1059-1075`), but no action is implied.
- **Mid-callback-execution**: callback state is left in whatever row was last committed; if the callback itself was updating its own `status` to DISABLED, that update may or may not have persisted (`openhands/app_server/app_conversation/set_title_callback_processor.py:151-152`).
- **Mid-export**: the Redis lock TTL of 3600s (configurable at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2469-2472`) ensures the lock auto-expires, after which another export can acquire it. The streaming `iter_events_for_export` will just continue from wherever the iteration left off; there is no zip partial-file cleanup.
- **Mid-sandbox-startup**: `wait_for_sandbox_running` polls every 2s with a 120s timeout (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:867-872`); if the sandbox dies mid-startup, the task will eventually fail with `SandboxError` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:864`).
- **Webhook handler crash mid-batch**: see above; partial events persisted, webhook returns Success.

The system **cannot say exactly what completed and what did not** beyond "the events that are on disk exist" — there is no log of "attempted but failed to save" events.

### 5. Are tool calls their own atomic units?

**No**, not in the app_server. The app_server sees tool calls as `ActionEvent` (when the agent issues a tool call) and `ObservationEvent` (when the tool returns) — both are subclasses of `Event` (`openhands/app_server/event_callback/webhook_router.py:57` imports `ObservationEvent`). The app_server:

- Does not pair an `ActionEvent` with its corresponding `ObservationEvent` (it just iterates events by timestamp via `iter_events_for_export` at `openhands/app_server/event/event_service_base.py:145-156`).
- Does not check that every `ActionEvent` has a matching `ObservationEvent`.
- Does not know which tool was called (it sees only the opaque event payload).
- Does not retry individual tool calls — retry is a concern of the SDK agent loop, not the app_server.

For the UI, a tool call looks like two consecutive events in the conversation stream. A failed/missing tool result is invisible unless the user reads the event JSON.

## Architectural Decisions

1. **Events are opaque records at the app_server boundary.** The app_server does not interpret `Event` subclasses; it persists, lists, counts, and exports them as JSON. The internal structure (tool name, tool arguments, observations) is only decoded by the SDK and the UI. Decision recorded at the abstract level: `EventService` only sees `openhands.sdk.Event` (`openhands/app_server/event/event_service.py:8, 22, 26, 39, 61`).

2. **One event = one file/blob/object.** All three storage backends (filesystem, AWS S3, GCP) implement `_store_event` as a single write operation (`openhands/app_server/event/filesystem_event_service.py:33-36`, `openhands/app_server/event/aws_event_service.py:54-62`, `openhands/app_server/event/google_cloud_event_service.py`). There is no batching or coalescing, which means partial state IS possible but cheap to detect.

3. **AppConversationStartTask is a state machine, not a saga.** The 7-status enum (`openhands/app_server/app_conversation/app_conversation_models.py:262-271`) is yielded as an async stream (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-307`) and persisted per-yield. Each yield is a "checkpoint" the user can observe, but there is no automatic rollback if a step fails — the task just goes to `ERROR` and stops. This is deliberately not a saga pattern.

4. **Pending messages are pre-start buffering, not at-least-once delivery.** `PendingMessage` is the only place where the app_server buffers user intent across the "before conversation exists" gap (`openhands/app_server/pending_messages/pending_message_service.py:41-72`). The `_process_pending_messages` call in `_start_app_conversation` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:520-532`) deletes messages after delivery with no retry. This is a deliberate "fire once on first contact" model.

5. **EventCallback is a per-kind subscription, not a per-event hook.** A single `EventCallback` is registered per `event_kind` and fires for every matching event of that kind (`openhands/app_server/event_callback/sql_event_callback_service.py:208-214`). One callback per (conversation_id, kind) pair is the composite index at `openhands/app_server/event_callback/sql_event_callback_service.py:51-57`.

6. **Execution status is live-fetched, not persisted.** `AppConversation.execution_status` is read from the agent-server's `/api/conversations` endpoint on every conversation list/get (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:608-617`, 639-641). The app_server does NOT store this; it's a snapshot. This is why `useAgentState` prioritises "live WebSocket execution status over cached API data" (per AGENTS.md frontend section).

7. **Conversation metrics are derived from a single event (`key='stats'`)**. The `ConversationStateUpdateEvent` with `key='stats'` is the only event processed for state, and it carries an accumulated `usage_to_metrics` dict (`openhands/app_server/app_conversation/sql_app_conversation_info_service.py:482-512`). There is no per-step or per-turn metric breakdown stored.

8. **Corrupt events are silently dropped.** `_load_event` returns `None` on any exception (`openhands/app_server/event/filesystem_event_service.py:23-31`). This means a partial write or schema drift causes a silent event loss with only a log line as forensic evidence.

9. **No two-phase commit between events and conversation metadata.** When the start task yields `READY` and writes the `app_conversation_id` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:521-523`), this happens in the same async function as the `await self.app_conversation_info_service.save_app_conversation_info(...)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:495-497`). A crash between these two awaits would leave the task in `READY` but with no `AppConversationInfo` row.

## Notable Patterns

- **Async-generator-as-task-progress**: `AppConversationStartTask` is a pydantic model that is *also* used as the yield value of the start generator (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:300-307`). The "task" is not a thread or process but a serialised stream of status snapshots.

- **Async generator consumer in HTTP handler**: `app_conversation_router.start_app_conversation` consumes the first yield synchronously then spawns `_consume_remaining` as a background task (`openhands/app_server/app_conversation/app_conversation_router.py:380-410`). This pattern decouples HTTP response time from the full start workflow.

- **Opaque EventKind typing**: `EventKind = Literal[tuple(c.__name__ for c in get_known_concrete_subclasses(Event))]` (`openhands/app_server/event_callback/event_callback_models.py:28-30`) — the kind union is auto-derived from the SDK's known event subclasses, so adding a new event kind in the SDK automatically widens the callback filter API.

- **Per-event UUID in filename**: `{event_id_hex}.json` is the canonical filename (`openhands/app_server/event/event_service_base.py:194-195`, `openhands/app_server/conversation_paths.py:11-35`). The event ID is both the primary key and the filename, which means saving the same event twice is idempotent at the file level (same path → same file).

- **Hex-stripped UUIDs in storage**: `id_hex = event.id.replace('-', '')` (`openhands/app_server/event/event_service_base.py:192-194`) keeps filenames compact and shell-safe.

- **Pagination as offset, not cursor**: `next_page_id = str(start_offset + limit)` (`openhands/app_server/event/event_service_base.py:140`). Offset pagination is simple but breaks if events arrive between page reads.

- **Pre-write directory creation**: `path.parent.mkdir(parents=True, exist_ok=True)` (`openhands/app_server/event/filesystem_event_service.py:34`) means the first event to a conversation creates the whole subtree — there is no separate "create conversation directory" operation.

- **Lock-bypass on missing Redis**: `export_lock = None; lock_unavailable = True` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2374-2375`) — in OSS mode without Redis, export proceeds without locking. This is a graceful-degradation pattern.

## Tradeoffs

- **Granular events vs. no transaction**: Persisting one event per file/blob gives fine-grained replayability (the entire trajectory export is just `iter_events_for_export` at `openhands/app_server/event/event_service_base.py:145-156`) but means a partial batch save leaves a partial state with no transactional rollback.

- **Coarse start-task vs. fine events**: The `AppConversationStartTask` is a single coarse "task" for the whole start workflow, while the per-event atomicity is fine. Operators get a clear "is this conversation ready?" signal but no per-step visibility into the agent loop itself.

- **Live execution status vs. cached**: `execution_status` is always live-fetched (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:608-617`), which is correct but means the app_server's "view" of the conversation is only as fresh as the most recent HTTP call. There is no event-driven push to the app_server; the sandbox must call back via webhook.

- **Webhook-based event delivery vs. push streaming**: The app_server learns about events via the sandbox POSTing to `/api/v1/webhooks/events/{conversation_id}` (`openhands/app_server/event_callback/webhook_router.py:453-522`). This is reliable but asynchronous — events can lag the agent loop by network RTT.

- **JSON-per-event storage cost**: 10K events × average JSON size = potentially large S3 bills. The `export_max_events = 10000` cap (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2465-2468`) is the only bound on conversation size.

- **`turn_count` is dead-coded in this layer**: The field exists in analytics (`openhands/analytics/analytics_service.py:227, 245, 263, 279`) but the app_server always passes `None` (`openhands/app_server/event_callback/webhook_router.py:162, 180`). This means turn-based UI affordances cannot be built from app_server data alone.

- **No replay/dedup of webhook batches**: If the sandbox retries a webhook delivery, the app_server will save the same events again (idempotent at file level by UUID, but counts in `count_events` would still increment if events were re-saved with new IDs). The webhook handler does not dedup.

- **Pending messages are fire-and-forget**: No retry on delivery failure (`openhands/app_server/pending_messages/pending_message_service.py:62-72`); a transient network error during `_process_pending_messages` loses the message.

- **Callback success ≠ callback effect observed**: Callbacks run in background (`openhands/app_server/event_callback/webhook_router.py:513-517`) and the HTTP 200 to the sandbox is sent before they complete. The sandbox thinks the webhook succeeded; the callback may still fail.

## Failure Modes / Edge Cases

- **Orphaned start task**: `_start_app_conversation` is killed between yield #4 and yield #5. The task is in `SETTING_UP_GIT_HOOKS` (or similar) with no `app_conversation_id`. No automatic resume; the user sees a "stuck starting" conversation that needs manual cancellation. No cleanup code in the app_server.
- **Missing conversation_metadata row**: If the agent-server creates a conversation directly (e.g., automation run), `valid_conversation` synthesises a stub `AppConversationInfo` on the fly (`openhands/app_server/event_callback/webhook_router.py:289-301`). The stub has `title=None` and is detected as "new" in `on_conversation_update` (`openhands/app_server/event_callback/webhook_router.py:354-355`) to register the SetTitleCallbackProcessor. No double-write protection.
- **S3 / GCS outage during batch save**: `asyncio.gather` will propagate the first exception; some events will be saved, others not. Webhook returns `Success` anyway (`openhands/app_server/event_callback/webhook_router.py:520-522`). No DLQ.
- **Filesystem full during save**: `path.write_text` raises `OSError`. The exception propagates to `webhook_router.on_event` outer try/except (`openhands/app_server/event_callback/webhook_router.py:519-521`). The webhook returns Success.
- **Race between start task and same conversation create**: If a user posts to `/api/v1/conversations/{id}/send-message` while the conversation is still starting, the `app_conversation_router.send_message_to_conversation` checks `sandbox.status != SandboxStatus.RUNNING` and returns 409 (`openhands/app_server/app_conversation/app_conversation_router.py:521-529`). The pending message path is the workaround: post to `/api/v1/conversations/{conversation_id}/pending-messages` (the conversation_id can be the task_id before the real UUID is allocated — see `openhands/app_server/pending_messages/pending_message_router.py:48-51`). The pending messages are linked to the real conversation_id via `update_conversation_id` (`openhands/app_server/pending_messages/pending_message_service.py:66-72, 168-185`) when the start task reaches READY.
- **Pending messages for never-READY conversation**: If the start task fails (status=ERROR), pending messages are NOT delivered and remain in the table. No cleanup code in the app_server.
- **Callback fires on event of unknown kind**: EventKind is a Literal, so pydantic validates at webhook parse time. An unknown event kind would fail JSON deserialisation in the SDK (`Event.model_validate_json` in `_load_event`); the webhook would 500. No graceful handling.
- **Race on export lock**: Two export requests arrive simultaneously. Only one acquires the lock; the other raises `ConversationExportAlreadyRunning` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2377-2380`). The user can retry after the first export's lock TTL (default 1 hour).
- **Sandbox returns 4xx on `/api/conversations`**: `_get_live_conversation_info` logs and returns `[]` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:618-628`). The conversation is rendered with `execution_status=None`, which is the "unknown" state.
- **Same event sent twice (webhook retry)**: The second save is a no-op at the file level (UUID-keyed filename) but the in-memory counter for batch operations may be off. No idempotency-token check.

## Future Considerations

- The app_server could compute `turn_count` from the event stream (count `MessageEvent`s with `source='agent'`) if a turn definition is formalised — currently the field is always None (`openhands/app_server/event_callback/webhook_router.py:162, 180`).
- The `EventCallback` model already has `ACTIVE / DISABLED / COMPLETED / ERROR` states (`openhands/app_server/event_callback/event_callback_models.py:33-37`), but only `ACTIVE` and `DISABLED` are used in the codebase. `COMPLETED` and `ERROR` could be wired into a per-callback summary view.
- `iter_events_for_export` could be the basis for a streaming checkpoint replay API (currently only used by export).
- The webhook handler's `try/except` at `openhands/app_server/event_callback/webhook_router.py:462-521` could return 5xx on partial save failure (currently returns 200), so the sandbox would retry and dedup on UUID.
- `PendingMessage` retry on transient failure is missing; could add an exponential-backoff delivery loop.
- The `AppConversationStartTask` could be promoted to a saga with explicit compensation (rollback clone, rollback skill install) for failed setup steps.
- Filesystem `_store_event` could use atomic rename (`write_text` → `os.replace`) to make mid-write failures leave the prior content intact.
- A `Step` or `Turn` aggregate model could be derived at search time to give the UI a turn/step view without modifying the persistence model.

## Questions / Gaps

- **Where exactly is "step" defined?** Not in the app_server. Possibly in `openhands-sdk` or `openhands-agent-server` (external to this source). The app_server only sees the resulting `Event`s.
- **Where does the agent loop actually run?** The `start_app_conversation` POSTs to the agent-server's `/api/conversations` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:436-441`). The agent-server is an external service running in the sandbox; the app_server receives its events via the webhook.
- **Is there a retry queue for failed event saves?** No. Webhook returns 200 even on partial save failure (`openhands/app_server/event_callback/webhook_router.py:519-522`).
- **Does the sandbox retry webhook delivery on non-200?** The webhook always returns 200 (Success), so the sandbox never retries. But if the app_server itself dies (process crash) and a webhook arrives, it 5xx's and the sandbox would retry — the partial events would be re-saved idempotently by UUID.
- **How is "step" surfaced to the user?** Not at all in the app_server. The UI distinguishes events by `kind` (MessageEvent / ActionEvent / ObservationEvent) but does not group them into steps or turns.
- **What is the canonical event order?** Timestamp from the SDK. The app_server sorts by timestamp on search (`openhands/app_server/event/event_service_base.py:127-131`) but does not verify timestamp monotonicity.
- **Are events ever written to the app_server by the SDK directly?** No. All writes go through the sandbox-to-app-server webhook (`openhands/app_server/event_callback/webhook_router.py:453-522`). The app_server is a passive observer of the agent loop.
- **What does "READY" really mean for the start task?** That the agent-server has acknowledged the POST `/api/conversations` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:436-459`). It does NOT mean the first user message has been processed.

---

Generated by `dimensions/01.03-step-turn-and-task-atomicity.md` against `openhands`.