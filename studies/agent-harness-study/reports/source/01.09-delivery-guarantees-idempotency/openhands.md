# Source Analysis: openhands

## 01.09 Delivery Guarantees and Idempotency

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI app server) + TypeScript/React frontend; SQL backends (Postgres/SQLite) + file/S3/GCS event stores; Redis for ephemeral state |
| Analyzed | 2026-07-03 |

## Summary

OpenHands has no single message broker, so delivery semantics are shaped per subsystem:

- **Webhook ingestion (enterprise integrations)** is the most disciplined area: every provider uses HMAC signature verification and a Redis-backed deduplication cache (NX+TTL) before triggering downstream work, providing effectively-once semantics for one redelivery window.
- **Pending messages queue (chat messages before a sandbox is ready)** is **at-most-once** with deletion-after-send: messages are stored in SQL, then `POST`'d sequentially to the agent server's `/events` endpoint, and deleted whether or not the POST succeeded. There is no retry, no idempotency key sent to the agent server, and no dead-letter.
- **Agent-server → App-server webhook (conversation + event delivery)** (`on_event`/`on_conversation_update`) is fire-and-forget from the agent server's perspective: the app server saves events to file/S3/GCS under `<conversation_id>/<event_id>.json`, keyed by event UUID, so duplicate POSTs overwrite the same file silently. Callbacks fan out in `_run_callbacks_in_bg_and_close` without idempotency.
- **Event storage (filesystem / S3 / GCS)** is **exactly-once by write-path design**: `save_event` (`openhands/app_server/event/event_service_base.py:190-197`) writes to a deterministic path `…/{conversation_id}/{event_id.hex}.json`. Concurrent saves with the same event id are last-writer-wins. There is no sequence number or causal ordering on disk; ordering relies on `event.timestamp` strings.
- **Event callbacks** (`SQLEventCallbackService`) record their result row in `event_callback_result` after each invocation but do **not** deduplicate per event: a redelivered event triggers fresh callback runs.
- **Export streaming** uses a Redis distributed lock (`try_acquire_redis_lock` with periodic TTL refresh) to avoid concurrent exports, and throws `ConversationExportAlreadyRunning` to the second caller.

Overall: this is a heterogeneous mix where each subsystem solved its own redelivery story. There is **no project-wide claim of exactly-once**; the most mature guarantees are the per-provider webhook dedup (60-second TTL) and the deterministic event-file naming. Several long-running flows (pending messages, callbacks, automation service forwards) are at-most-once or best-effort.

## Rating

**5 / 10 — Present but inconsistent and weakly documented.**

- Two subsystems (provider webhooks, conversation export lock) implement correctly with tests; callback results are recorded but re-runs are not deduplicated.
- The pending messages flow is at-most-once with no DLQ and no per-message idempotency key sent over the wire.
- The on_event webhook handler swallows exceptions and returns 200 (`Success()`), so a redelivery will silently repeat side effects (analytics, callbacks, conversation-info mutations).
- The event store (FS/S3/GCS) is naturally idempotent for the same `(conversation_id, event_id)` pair by path, but there is no documented guarantee for cross-event ordering, partial writes, or torn files.
- No central policy doc or module specifying "delivery guarantees" — the model is per-subsystem and inferred from code/tests.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event-store contract (event id = file path = natural idempotency for same event id) | `async def save_event(...)` writes to `(await get_conversation_path(conversation_id)) / f'{id_hex}.json'` | `openhands/app_server/event/event_service_base.py:190-197` |
| FS implementation writes per-event file, no atomic rename | `path.parent.mkdir(...); ... path.write_text(content)` | `openhands/app_server/event/filesystem_event_service.py:33-36` |
| S3 implementation uses `put_object` (not transactional) | `self.s3_client.put_object(Bucket=..., Key=..., Body=...)` | `openhands/app_server/event/aws_event_service.py:54-62` |
| GCS implementation uses `blob.open('w')`, no chunking/tx | `with blob.open('w') as f: f.write(...)` | `openhands/app_server/event/google_cloud_event_service.py:57-62` |
| Event sort/order is timestamp-string based; no sequence number | `items.sort(key=lambda e: e.timestamp, ...)` | `openhands/app_server/event/event_service_base.py:127-131` |
| Agent-server → App-server: event ingest is a plain webhook with no dedup | `@router.post('/events/{conversation_id}')` calls `event_service.save_event(...)`; failures logged, returns `Success()` | `openhands/app_server/event_callback/webhook_router.py:453-522` |
| Callback runs once per event per active row (no idempotency) | `await callback.processor(conversation_id, callback, event)`; failures fall through to `EventCallbackResult(status=ERROR)`; the callback status stays ACTIVE so a redelivery re-runs | `openhands/app_server/event_callback/sql_event_callback_service.py:208-250` |
| Callback fan-out runs in background after response, failures only logged | `_run_callbacks_in_bg_and_close` uses `await event_callback_service.execute_callbacks(...)` serially; not awaited by the request | `openhands/app_server/event_callback/webhook_router.py:513-517, 561-573` |
| Pending messages: PK is auto UUID; dedup is not by content | `id: Mapped[str] = mapped_column(String, primary_key=True)` | `openhands/app_server/pending_messages/pending_message_service.py:32` |
| Pending messages: at-most-once with delete-after-attempt | `for msg in pending_messages: ... POST /events ... response.raise_for_status() ... finally deleted_count = ... delete_messages_for_conversation(...)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1925-2002` |
| Pending messages: Task-ID → real conversation-ID remap is the only "idempotent" element | `pending_message_service.update_conversation_id(old, new)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1952-1955` and `pending_message_service.py:168-185` |
| Pending messages: 10-message rate limit per conversation (no global quota) | `if pending_count >= 10: raise HTTPException(status.HTTP_429...)` | `openhands/app_server/pending_messages/pending_message_router.py:85-91` |
| Conversation export: Redis distributed lock with TTL refresh for at-most-one-export | `export_lock = await try_acquire_redis_lock(...)`; `refresh_lock_periodically(...)`; raises `ConversationExportAlreadyRunning` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2354-2419`; `openhands/app_server/utils/redis_lock.py:25-53` |
| Start-task lifecycle persists status row across retries; safe re-read | `save_app_conversation_start_task` upsert via merge and sets `updated_at = utc_now()` | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:235-248` |
| GitLab webhook dedup (Redis NX EX) | `created = await redis.set(dedup_key, 1, nx=True, ex=60)` with key `object_attributes.id` or fallback SHA-256 of body | `enterprise/server/routes/integration/gitlab.py:96-113` |
| GitLab dedup tested with object id, without id (hash fallback), different payloads | tests `test_gitlab_events_deduplication_*` | `enterprise/tests/unit/test_gitlab_resolver.py:19-180, 244-325` |
| Bitbucket Cloud dedup key = (event_key, pr_id, comment_id, request_uuid) or SHA-256(body) | `dedup_key = f'bb:{x_event_key}:{pr_id}:{comment_id}:{x_request_uuid}'`; `ttl=60s` | `enterprise/server/routes/integration/bitbucket.py:88-105` |
| Bitbucket DC dedup (Redis NX EX 60s) keyed on event_key + pr_id + comment_id + request_id | `dedup_key = f'bbdc:{x_event_key}:{pr_id}:{comment_id}:{x_request_id}'` | `enterprise/server/routes/integration/bitbucket_dc.py:660-677` |
| Bitbucket DC processes in `BackgroundTasks` to avoid provider timeout retries | `background_tasks.add_task(bitbucket_dc_manager.receive_message, message)` | `enterprise/server/routes/integration/bitbucket_dc.py:702` |
| Jira Cloud dedup via Redis SETEX keyed on signature | `key = f'jira:{signature}'; redis_client.setex(key, 300, '1')` | `enterprise/server/routes/integration/jira.py:308-316` |
| Slack `on_event` dedup via Redis NX EX keyed on client_msg_id | `key = f'slack_msg:{client_msg_id}'; created = await redis.set(key, 1, nx=True, ex=60)` | `enterprise/server/routes/integration/slack.py:315-320` |
| Slack repo-form interaction idempotency (Redis NX EX) | `redis.set(key, 'processing', ex=SLACK_USER_MSG_EXPIRATION, nx=True)` | `enterprise/integrations/slack/slack_manager.py:217-228` |
| GitHub webhook has **no** dedup in router or manager (verified) | Router delegates straight to `await github_manager.receive_message(message)` | `enterprise/server/routes/integration/github.py:91-93` |
| Automation service forward is a fire-and-forget `background_tasks.add_task` | `background_tasks.add_task(automation_event_service.forward_event, ...)` | `enterprise/server/routes/integration/github.py:82-88` |
| Forward_event catches and logs all errors; no retry | `except (aiohttp.ClientError, asyncio.TimeoutError) as e: logger.error(...); return None` | `enterprise/server/services/automation_event_service.py:126-143` |
| Bitbucket DC webhook uninstall is idempotent | comment + tests `uninstall is idempotent at the route layer` | `enterprise/storage/bitbucket_dc_webhook_store.py:198`; `enterprise/tests/unit/storage/test_bitbucket_dc_webhook_store.py:217` |
| Webhook enrollment checks existing webhook first to avoid duplicate POST | `check_webhook_exists_on_repository` used to avoid re-creating | `openhands/app_server/integrations/bitbucket_data_center/service/webhooks.py:37-52` |
| Bitbucket DC create/update webhook routes idempotently | comment `idempotently creates or updates the webhook on` | `enterprise/server/routes/integration/bitbucket_dc.py:463` |
| Event-callback composite index added for execute_callbacks perf (no dedup) | migration 117 creates `ix_event_callback_conversation_id_status_event_kind` | `enterprise/migrations/versions/117_add_event_callback_composite_index.py:36-42` |
| Pending messages position is informational only (count + 1); not authoritative | `position = result.scalar() or 0`; returned as `position + 1` | `openhands/app_server/pending_messages/pending_message_service.py:96-120` |
| Conversation-info webhook is a single resource, returns 200 even on partial failure | outer `try/except Exception: _logger.exception('Error in webhook',...); return Success()` | `openhands/app_server/event_callback/webhook_router.py:461-522` |
| Save_event runs after rate-limit check; bucket counter not transactional | `if not self.user_id: ...; await self.db_session.merge(...)` then commit | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:235-248` |
| Slack `receive_message` is also called in background tasks | `background_tasks.add_task(slack_manager.receive_message, message)` | `enterprise/server/routes/integration/slack.py:337` |

## Answers to Dimension Questions

1. **Can the same work run twice?**
   Yes, in several places:
   - `on_event` webhook (`openhands/app_server/event_callback/webhook_router.py:453-522`) writes events to a deterministic file path, so a redelivery of the **same event id** overwrites itself harmlessly; but redeliveries of **different event ids for the same conversation** trigger re-runs of every active `EventCallback` row in `SQLEventCallbackService.execute_callbacks` (`openhands/app_server/event_callback/sql_event_callback_service.py:208-231`). The callback processor executes fresh each time and is not deduplicated against past results in `event_callback_result`.
   - GitHub and Automation forwards are `background_tasks.add_task` calls (`enterprise/server/routes/integration/github.py:82-88`) — a FastAPI background task that fails after the response is logged and forgotten, with no retry.
   - Pending messages delete themselves whether or not the POST succeeded (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1997-2006`), so a network blip drops the message; but the **consumer (agent server)** has no idempotency key, so two writers (e.g. two browser tabs) can each enqueue and each cause a duplicate run at the sandbox.

2. **Is duplicate execution safe?**
   - Event-store side: yes (deterministic path; overwrites same event).
   - Event-callback side: **no**. `SetTitleCallbackProcessor` and similar processors re-execute for each redelivery. `data_collector.process_payload` in `enterprise/integrations/github/data_collector.py:698` may emit duplicate metric records.
   - Pending messages: duplicate enqueues from the client side will produce duplicate runs at the agent server. No client-supplied dedup key.
   - Forward to automation service: idempotent w.r.t. socket connect, but a delivered-but-not-acknowledged 5xx would be lost (no outbox).
   - Provisioning side effects (Bitbucket webhook enrollment, jira workspace creation, etc.) are made idempotent by **idempotency-by-construction** (`check_webhook_exists_on_repository`, signature-keyed SETEX).

3. **Which side effects are idempotent?**
   - **Event file write** (same `event_id.hex` → same path): `event_service_base.py:190-197`.
   - **Webhook enrollment** (Bitbucket / Bitbucket DC / Slack webhooks): `webhook_exists`/`check_webhook_exists_on_repository` (`openhands/app_server/integrations/bitbucket_data_center/service/webhooks.py:37-52`), `uninstall is idempotent at the route layer` (`enterprise/storage/bitbucket_dc_webhook_store.py:198`).
   - **Provider-token storage** uses `store_org_token` (overwrites by `installation_id`): `enterprise/integrations/github/github_manager.py:296-299`.
   - **Org-resolver cache** caches both positive and negative results in Redis with TTL to avoid amplifying external lookups on retries (`enterprise/server/services/automation_event_service.py:347-398`).
   - **Side effects that are NOT idempotent**: callback processor results (`SetTitleCallbackProcessor`), GitHub "eyes" reaction (would add second reaction), pending message delivery, automation service forwarding, analytics events (`track_conversation_created`, `track_conversation_errored`, etc. in `webhook_router.py:441-510` — these run on every webhook delivery).

4. **Does the system claim exactly-once semantics?**
   - **No global claim.** No file in the source declares "exactly-once" delivery. The only delivery-confirmation language is the comment "Atomic claim" on Slack's Redis NX (`enterprise/integrations/slack/slack_manager.py:208-216`).
   - Per-subsystem claims are **effectively-once within a 60-second window** for most provider webhooks (Redis NX EX), and **at-most-once** for pending messages, callbacks, and automation forwarding. Event storage is last-writer-wins on `event_id` (idempotent by construction for the same id; not deduplicated by content).

5. **Are guarantees documented?**
   - **No dedicated doc.** There is no `docs/delivery.md` or README section that codifies the model. The contract has to be reconstructed from:
     - Docstrings: `"Stored in the database and delivered to the agent_server when the conversation transitions to READY status. Messages are deleted after processing, regardless of success or failure."` (`openhands/app_server/pending_messages/pending_message_models.py:12-18`) — only states it is best-effort.
     - Inline comments: `"Process in the background so we return 200 fast. Creating the conversation + sandbox synchronously can take ~tens of seconds, which blows Bitbucket DC's webhook timeout -> it marks the delivery failed and retries (a duplicate-trigger risk). Mirrors the Jira DC route."` (`enterprise/server/routes/integration/bitbucket_dc.py:698-702`) — explains the dedup rationale.
     - Migration 117 docstring explains the **performance** fix, not a delivery guarantee (`enterprise/migrations/versions/117_add_event_callback_composite_index.py`).

## Architectural Decisions

1. **Use natural id-based file paths as the write idempotency mechanism for the event log.** Each event is stored at `<prefix>/<user_id>/v1_conversations/<conversation_id>/<event_id>.json` (`openhands/app_server/event/event_service_base.py:190-197`), so duplicate writes for the same `event_id` are coalesced by FS/S3/GCS. There is no separate dedup key store — the path is the key. Trade-off: this is at-most-once for the file write, but the event id is generated by the producer (agent-server), so two producers can produce two events with the same id and one will silently overwrite the other. The intent is captured in `event_service_base.save_event`, which uses `event.id.hex` as the key (`openhands/app_server/event/event_service_base.py:190-197`).

2. **Treat provider webhooks as an external trigger that requires idempotency at the edge.** Every provider route uses HMAC signature verification *before* dedup is touched (`enterprise/server/routes/integration/gitlab.py:65-79`, `enterprise/server/routes/integration/jira.py:130-190`, `enterprise/server/routes/integration/github.py:33-49`, `enterprise/server/routes/integration/slack.py:288-294`). Dedup is implemented with `redis.set(..., nx=True, ex=60)`, where the key is the most stable identifier the provider supplies: `object_attributes.id` (GitLab), signature (Jira), `x-request_uuid` (Bitbucket Cloud/DC), or `client_msg_id` (Slack). When none is available, the fallback is `hashlib.sha256(body)` to ensure an in-process redelivery of the same payload hits the dedup block.

3. **Move slow work out of the request into FastAPI `BackgroundTasks` to avoid provider redeliveries.** Both Bitbucket DC and Bitbucket Cloud process the conversation creation in `background_tasks.add_task(..., manager.receive_message, message)`. The router therefore returns 200 quickly while the side effects run (or fail silently). Acknowledgement to the provider is decoupled from the side effect being complete.

4. **Use a small SQL-backed queue for client chat messages arriving before the sandbox is ready.** `SQLPendingMessageService` persists with a server-generated `uuid4()` id, then the live-status service does `update_conversation_id(task_id → real_id)` and POSTs each row in order to `/api/conversations/{id}/events`. After the loop, all messages for the conversation are deleted in one DELETE (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1997-2002`). The trade-off documented in the message model is "deleted after processing, regardless of success or failure" — explicit at-most-once.

5. **Run event callbacks in a background task so the agent-server's webhook does not block on them.** `_run_callbacks_in_bg_and_close` uses `asyncio.create_task(...)` and returns 200 immediately (`openhands/app_server/event_callback/webhook_router.py:513-517`). Callback results are persisted to `event_callback_result` for observability, but they do not block the response and they do not gate the callback's `status` from `ACTIVE` to `COMPLETED` automatically on success (the processor itself would have to call `save_event_callback` to do so).

6. **Use Redis distributed locks for work that would corrupt if run concurrently.** Conversation export acquires `app_conversation_export:<id>` with periodic TTL refresh and a 1-hour TTL (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2354-2419` + `openhands/app_server/utils/redis_lock.py:25-53`). Falls open when Redis is unavailable, controlled by `export_lock_required`.

7. **Use Postgres advisory locks during migrations to prevent destructive concurrent schema operations.** Comment in `enterprise/migrations/versions/117_add_event_callback_composite_index.py:8-15` (`pg_advisory_lock`) but this is migration-scope only.

## Notable Patterns

- **Edge dedup via `SET NX EX` as the universal at-most-one-with-redelivery-window primitive.** Used in:
  - `enterprise/server/routes/integration/gitlab.py:107` (NX 60s)
  - `enterprise/server/routes/integration/bitbucket.py:99` (NX 60s, event-specific key)
  - `enterprise/server/routes/integration/bitbucket_dc.py:671` (NX 60s, event-specific key)
  - `enterprise/server/routes/integration/jira.py:316` (SETEX 300s, signature key)
  - `enterprise/server/routes/integration/slack.py:317` (NX 60s, client_msg_id)
  - `enterprise/integrations/slack/slack_manager.py:222-228` (NX 300s, form interaction)
  This is the codebase's signature pattern for "exactly one within N seconds".
- **Idempotency-by-key path for object stores.** `_store_event` in every EventService writes to `f'{event_id.hex}.json'` (`openhands/app_server/event/event_service_base.py:190-197`). Three concrete implementations: `FilesystemEventService._store_event` (`openhands/app_server/event/filesystem_event_service.py:33-36`), `AwsEventService._store_event` (`openhands/app_server/event/aws_event_service.py:54-62`), `GoogleCloudEventService._store_event` (`openhands/app_server/event/google_cloud_event_service.py:57-62`). None use conditional writes (S3 `IfNoneMatch`, GCS generation precondition) — this is last-writer-wins with no fencing.
- **Background-task offloading with `add_task` for slow webhook processing.** Used in Bitbucket Cloud (`enterprise/server/routes/integration/bitbucket.py` — though process_message is *synchronous* await here, see line 115: `await bitbucket_manager.receive_message(message)` — actually awaited inline, which is interesting; Bitbucket DC and Slack use `add_task`). This shows inconsistent use of the same Redis dedup: one provider awaits before responding, another offloads.
- **TTL refresh of long-held Redis locks.** `refresh_lock_periodically` (`openhands/app_server/utils/redis_lock.py:36-53`) is used to keep the export lock alive across multi-minute streaming exports.
- **Default-fall-open when Redis is unavailable.** `_get_cached_value` catches and logs the error, returns None (`enterprise/server/services/automation_event_service.py:486-510`). Same philosophy in `try_acquire_redis_lock` (`openhands/app_server/utils/redis_lock.py:30-33`) and the GitLab dedup path.
- **Conversation-info save uses SQLAlchemy `merge` for last-writer-wins semantics.** `save_app_conversation_info` is implemented at `openhands/app_server/app_conversation/sql_app_conversation_info_service.py` (referenced, not loaded here). Merge preserves `id` collisions.
- **Sequence number / event ordering relies on ISO-8601 string compare.** `items.sort(key=lambda e: e.timestamp, reverse=...)` (`openhands/app_server/event/event_service_base.py:127-131`). No monotonic counter, no vector clock; if two events share a `timestamp` string the order is non-deterministic.

## Tradeoffs

- **Dedup TTL vs. provider-redelivery window.** All five provider integrations chose 60 s except Jira (300 s) and Slack form interaction (300 s). If a provider's redelivery interval is longer than the TTL, the second delivery will run twice. There is no observability that surfaces "we redelivered and got past dedup".
- **Pending messages: lose the message rather than retry.** Comments confirm "deleted after processing, regardless of success or failure" (`openhands/app_server/pending_messages/pending_message_models.py:12-18`). The trade-off is simplicity over reliability; a flaky network between the app server and the agent server drops user input silently.
- **Event callbacks have no idempotency memory.** A `ConversationStateUpdateEvent` arriving twice (e.g., webhook retries) re-runs every ACTIVE processor for that conversation (`openhands/app_server/event_callback/sql_event_callback_service.py:208-231`). The processor is responsible for any "did I already handle this event_id" logic itself.
- **Background tasks lose errors.** `_run_callbacks_in_bg_and_close` returns control and any exception inside `execute_callbacks` propagates into the background task, which is unawaited (`openhands/app_server/event_callback/webhook_router.py:513-573`). Operators see nothing in logs by default unless a separate observability pipeline catches `asyncio` task errors.
- **No fencing tokens on object-store writes.** Multiple replicas of the app server can race on `s3_client.put_object` for the same `event_id`; S3 last-writer-wins is the de facto ordering. Corrupt partial writes (truncated JSON) are possible if a process dies mid-write — `_store_event` does not write to a temp file and rename.
- **Conversation export "proceeds without lock" fallback.** If Redis fails, `export_lock_required=False` causes `open_conversation_export` to issue an *unlocked* export and log a warning (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2366-2375`). Two simultaneous exports then write the same conversation twice.
- **Database-backed dedup is not used for agent-server webhooks.** `event_callback` and the SQL event store never check if an event with this UUID is already saved before saving — they rely on the FS/S3/GCS write being idempotent by path. Two writers with the same `event_id` reach the store; the slowest one's payload wins.

## Failure Modes / Edge Cases

1. **App server crash between `receive_message` and 200.** The provider (GitLab/Slack/Jira/Bitbucket) will retry. The 60-second TTL on the dedup key means a delayed retry can land outside the dedup window and cause a duplicate conversation to start. The Slack manager has explicit "duplicate-trigger risk" commentary for Bitbucket DC (`enterprise/server/routes/integration/bitbucket_dc.py:698-702`).

2. **Network failure between app server and agent server during `_process_pending_messages`.** The inner POST raises; the loop catches and logs (`Failed to deliver pending message {msg.id}: {e}`) (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1994-1995`), then continues to the next message and ultimately DELETEs all of them. The user's message is gone.

3. **`event_service.save_event` is called for the same event UUID twice.** With FS: the second `path.write_text` overwrites the first; if the file is partially written at the moment of overwrite, downstream readers can see a malformed JSON file. `Event.model_validate_json` in `_load_event` (`openhands/app_server/event/filesystem_event_service.py:25-31`) catches the parse error and returns None, so the event disappears from history.

4. **Callback processor side-effect re-execution.** Example: `track_conversation_created` in `openhands/app_server/event_callback/webhook_router.py:441-448` is called on every `POST /conversations` webhook, not deduplicated by conversation id, so a redelivered webhook fires the analytics event twice. (No code-level dedup: search boundary = `track_conversation_created` occurrences, the call site only.)

5. **Re-running `_run_callbacks_in_bg_and_close`.** The function takes `events: list[Event]` — there is no "have I already processed these event ids" check. After a successful previous run, the same webhook delivery (or a re-delivery) will run every ACTIVE callback processor again.

6. **Multiple API tabs queueing pending messages.** A user opening two tabs can submit `POST /conversations/{task}/pending-messages` from each. Both succeed (different `id` UUIDs per message). On conversation readiness, both messages are POSTed to the agent server, which has no idempotency key — the agent runs both. No dedup by content either.

7. **Concurrent conversation export on a Redis outage.** `ConversationExportLockUnavailable` is only raised when `self._conversation_export_lock_required()` is true (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2365-2377`). With the default unset/insecure config, two simultaneous exports write to the same conversation and may emit partial streams. The route also has a hard 413 (`ConversationExportTooLarge`) check that can fail mid-stream after the lock has been released (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2382-2391`).

8. **Bitbucket DC webhook uninstall races with reinstall.** The comment "uninstall is idempotent at the route layer so the caller treats" (`enterprise/storage/bitbucket_dc_webhook_store.py:198`) and the corresponding test (`enterprise/tests/unit/storage/test_bitbucket_dc_webhook_store.py:217`) suggest the route swallows "not found" as success, but a concurrent reinstall can re-create the row while an uninstall is in flight.

9. **Jira Cloud dedup is keyed on `x-hub-signature` only, not request UUID.** `enterprise/server/routes/integration/jira.py:308-316`. If a malicious actor could replay the same payload within 300 s with the same signature, it would be deduped — but the signature is verified above, so the dedup is effectively a no-op for a legitimate replay (signatures match, payload matches, no harm). For uniqueness, this is **good**; but if Jira ever delivers the same logical event twice with different signatures (e.g., after a webhook secret rotation), both would pass dedup and execute twice.

10. **GitHub integration has no dedup layer in the route or manager.** `enterprise/server/routes/integration/github.py:91-93` forwards straight to `github_manager.receive_message`. If GitHub delivers the same `installation/<id>/issue_comment` event multiple times within a few seconds (it does, sometimes), each delivery starts a new conversation. Found via grep — no evidence of a Redis dedup call in this path.

## Future Considerations

- **Add an idempotency key to the agent-server events POST.** The pending-messages flow has a server-generated UUID per message; passing it through as the event UUID to the agent server's `/events` endpoint would let downstream callers detect duplicates.
- **Move pending-messages deletion to a per-message ack-and-delete.** Currently a bulk DELETE runs at the end of the loop (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1997-2002`); deleting after each successful POST would prevent a mid-loop crash from re-sending the rest on retry.
- **Implement conditional writes on the event store.** S3 `IfNoneMatch: *` plus a single PUT would prevent partial-file leaks. GCS supports `if_generation_match`.
- **Deduplicate event-callback runs.** Persist a `(callback_id, event_id)` pair as the unique key for `EventCallbackResult` rows and short-circuit `execute_callback` if it already exists for that pair. Today: `event_callback_result.id` is UUID, no unique constraint on `(event_callback_id, event_id)` (`openhands/app_server/event_callback/sql_event_callback_service.py:76-90`).
- **Add a provider-level delivery_id header to GitHub and the agent-server webhooks** so the dedup layer can be uniform across all integrations. GitHub does send `X-GitHub-Delivery`; it is **not** read in `enterprise/server/routes/integration/github.py`. Same for the agent-server-to-app-server webhook which has no delivery-id header today.
- **Document the delivery model explicitly.** Add a `docs/delivery-guarantees.md` listing per-subsystem guarantees, TTLs, retry policies, and how to extend with a new provider. The patterns exist in code/tests but a new contributor has to grep to discover them.
- **Reduce reliance on Redis being up.** `_get_cached_value` falls open silently (`enterprise/server/services/automation_event_service.py:486-510`), which means a Redis outage removes *all* dedup silently — operators should see an alert.

## Questions / Gaps

- **Where is the agent-server event POST actually processed?** The pending-message flow POSTs to `/api/conversations/{id}/events` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1982-1989`), but the agent-server's behavior on duplicate POSTs is not in this source tree. Search boundary: did not open `openhands/agent_server/`, which is referenced only as `from openhands.agent_server.models import ...` — that code lives outside the selected source.
- **Is `event_callback_result` ever consulted as a "have we already processed this event for this callback" check?** No evidence found of such a query in the source. Search boundary: full repo grep of `event_callback_result` yielded only model definitions and INSERT sites.
- **Does the webhook receive an `X-GitHub-Delivery` header in the GitHub router?** No code reads it (`enterprise/server/routes/integration/github.py:52-98`). A Redis dedup on that header would close the GitHub duplicate-delivery gap.
- **Is there a `Sequence` or `version` column on `event_callback` or `app_conversation_info`?** No evidence found. Search boundary: scanned `sql_event_callback_service.py` and `sql_app_conversation_info_service.py` for `version`, `seq`, `etag`. No result.
- **Where is the consumer-side dedup for the agent-server `/events` endpoint?** Outside this source tree.
- **Why does Jira use SETEX (key-value with TTL) where the other integrations use NX?** Inconsistency between `enterprise/server/routes/integration/jira.py:316` and `gitlab.py:107` etc. The behavioural difference: SETEX overwrites, so a duplicate within TTL can blow away the previous TTL reset; NX guarantees the first wins. Jira's logic post-SETEX returns 200 with `{'success': True}` to all duplicates.
- **Is `x_event_key` considered a strong enough dedup key for Bitbucket?** `enterprise/server/routes/integration/bitbucket.py:88-105` keys on `(event_key, pr_id, comment_id, x_request_uuid)`. If a comment is *edited* it has the same `comment_id` but a new `x_request_uuid`. Comment-edit-dedup may treat it as a new event correctly; comment-delete with the same id would not run twice — possibly a desired or undesired behaviour depending on intent.

---

Generated by `01.09-delivery-guarantees-and-idempotency` against `openhands`.
