# Source Analysis: openhands

## 02.09 — State Pruning, Compaction, and Retention

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12+ (FastAPI app server, SQLAlchemy, Alembic, external `openhands-agent-server`/`openhands-sdk`/`openhands-tools` v1.29.0 packages) |
| Analyzed | 2026-07-13 |

> Scope note: the in-scope `openhands` package for this study is the V1 app server at `openhands/openhands/app_server/`. Agent-server, SDK, and tools live in separately versioned external pip packages referenced from `pyproject.toml:60-62` (`openhands-agent-server==1.29.0`, `openhands-sdk==1.29.0`, `openhands-tools==1.29.0`); their internals are not in this source tree and are not assessed here.

## Summary

The V1 OpenHands app server stores every event as an immutable per-event JSON file and provides **no storage-layer pruning or compaction at all**. There is no `delete_event` API, no event-level TTL, no scheduled GC, no per-conversation token-budget pruning, and no summarization step that rewrites persisted history. The only compaction-style machinery is the in-process `LLMSummarizingCondenser` from `openhands.sdk`, which shrinks the LLM context window but does not touch persisted events. Conversation deletion is best-effort: the SQL metadata row and start-task rows are removed, the agent server is asked to delete its own state, and the underlying sandbox may be torn down — but the on-disk event JSON files are **not** explicitly deleted by app-server code, leaving orphaned history until out-of-band cleanup. Sandbox backpressure (`pause_old_sandboxes`) protects compute cost, not storage growth. There is no user-driven privacy/anonymization endpoint and no test asserts that event files are removed when a conversation is deleted.

## Rating

**3 / 10** — Pruning and retention are essentially absent at the storage layer. There is a real `delete_conversation` REST endpoint and a real sandbox pause-oldest heuristic, but both are operational/orthogonal policies rather than history-retention mechanisms. The conversation-export guard (`export_max_events=10000`) is the only explicit cap on event-set size and it gates exports rather than deleting anything. The condenser is real but operates on the LLM context window, not persisted state, so it does not satisfy this dimension. Replay determinism is preserved (events are append-only), which is the one positive signal.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event storage is per-file, no prune hook | Each event stored as `{event_id_hex}.json` under `{persistence_dir}/{user_id}/v1_conversations/{conversation_id_hex}/`. `save_event` only writes; there is no `_delete_event` or `delete_event`. | `openhands/app_server/event/event_service_base.py:83-89`, `openhands/app_server/event/event_service_base.py:190-198`; `openhands/app_server/event/filesystem_event_service.py:33-42` |
| EventService abstract has no delete method | `EventService` defines `get_event`, `search_events`, `count_events`, `save_event`, `batch_get_events`; `iter_events_for_export` reads all. No delete path. | `openhands/app_server/event/event_service.py:18-70` |
| Event router exposes only read endpoints | `/events/search`, `/events/count`, `/events` (batch get). No DELETE/PATCH for events. | `openhands/app_server/event/event_router.py:29-110` |
| Conversation deletion removes DB rows, not event files | `delete_app_conversation` deletes `StoredConversationMetadata` and `StoredAppConversationStartTask`, then calls agent-server DELETE; the conversation's event directory under `persistence_dir` is never referenced. | `openhands/app_server/app_conversation/app_conversation_router.py:892-978`; `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2149-2290`; `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:590-608`; `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:250-269` |
| Sandbox cleanup is per-user, not global | `pause_old_sandboxes` triggered on every `start_sandbox` / `resume_sandbox` to keep per-user running count under `max_num_sandboxes` (default 10 remote / 5 docker). Pauses (not deletes) oldest sandboxes. | `openhands/app_server/sandbox/sandbox_service.py:203-244`; `openhands/app_server/sandbox/remote_sandbox_service.py:598-621`; `openhands/app_server/sandbox/remote_sandbox_service.py:425, 509`; `openhands/app_server/sandbox/docker_sandbox_service.py:395, 520` |
| Process sandbox directory rmtree on delete | `ProcessSandboxService.delete_sandbox` calls `shutil.rmtree(process_info.working_dir, ignore_errors=True)`. This is the only explicit storage-directory cleanup in app_server, and it is local-only. | `openhands/app_server/sandbox/process_sandbox_service.py:386-422` |
| Sandbox post-delete done in background task | `_finalize_sandbox_delete` runs `sandbox_service.delete_sandbox` only when no other conversations reference the sandbox; gated by `count_conversations_by_sandbox_id == 0`. | `openhands/app_server/app_conversation/app_conversation_router.py:868-889`, `937-976` |
| Export size guard (hard cap, not pruning) | `_validate_conversation_export_size` raises `ConversationExportTooLarge` when `event_count > export_max_events` (default 10000). Field is configurable via `LiveStatusAppConversationServiceInjector`. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2304-2313`; `openhands/app_server/app_conversation/live_status_app_conversation_service.py:223-226`; `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2465-2483` |
| Pending message queue drains on completion | After start task drains queue, `delete_messages_for_conversation` removes all pending messages. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1997-2006`; `openhands/app_server/pending_messages/pending_message_service.py:151-166` |
| LLM-context condenser (not storage compaction) | `_create_condenser` returns an `LLMSummarizingCondenser` (SDK default `max_size=240`, `keep_first=2`); `condenser_max_size` user setting overrides only `max_size`. Condenser condenses the LLM context, not persisted events. | `openhands/app_server/app_conversation/app_conversation_service_base.py:578-612`; `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1352-1371` |
| Conversation list limit (per sandbox) | `max_num_conversations_per_sandbox` (default 20) filters sandboxes during selection; not a retention policy. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:746`; `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2450-2453` |
| Secrets / profiles deletion | REST endpoints `DELETE /api/v1/secrets/{secret_id}` and `DELETE /api/v1/settings/profiles/{name}` exist; these are user-data deletion paths, not conversation history. | `openhands/app_server/secrets/secrets_router.py:344-383`; `openhands/app_server/settings/settings_router.py:525-540` |
| No user-account / privacy-deletion endpoint | V1 user router only exposes `GET /users/me` and `GET /users/git-info`. No `DELETE /users/me` or GDPR-style erase. | `openhands/app_server/user/user_router.py:21-58` |
| Replay preserved by append-only | `iter_events_for_export` returns all events in timestamp order; this is replay-safe but the converse — no compaction path — is the retention blind spot. | `openhands/app_server/event/event_service_base.py:145-156` |
| No retention tests in V1 unit suite | Searches in `tests/unit/app_server/` show no `retention` / `prune` / `delete_event` tests for the V1 conversation/event services. The only "cleanup" tests target sandbox lifecycle. | `tests/unit/app_server/test_sandbox_service.py:99-350`; `tests/unit/app_server/test_filesystem_event_service.py:1-460` |

## Answers to Dimension Questions

1. **What grows forever?**
   - Per-conversation event JSON files in `{persistence_dir}/{user_id}/v1_conversations/{conversation_id_hex}/{event_id_hex}.json` (`openhands/app_server/conversation_paths.py:12-35`; `openhands/app_server/event/event_service_base.py:83-89`). There is no TTL on events, no per-conversation cap on event count, and no scheduled GC. Conversations that the user has "deleted" still leave their event files in place.
   - `StoredConversationMetadata` rows in SQL only expire when the user deletes the conversation explicitly (`openhands/app_server/app_conversation/sql_app_conversation_info_service.py:590-608`). There is no auto-purge.

2. **What gets summarized?**
   - The LLM's view of the conversation history gets summarized via `LLMSummarizingCondenser` (`openhands/app_server/app_conversation/app_conversation_service_base.py:578-612`). This is in-process context-window compaction, not storage compaction. The persisted event log is unaffected.

3. **What gets deleted?**
   - On explicit `DELETE /api/v1/app-conversations/{id}` (`openhands/app_server/app_conversation/app_conversation_router.py:892-978`): the SQL metadata row, start-task rows, the agent-server-side conversation (best-effort HTTP DELETE), and — only if no other conversations reference the sandbox — the sandbox itself. Event JSON files are not deleted by app-server code paths.
   - Process-sandbox local working directory is rmtree'd (`openhands/app_server/sandbox/process_sandbox_service.py:407-410`).
   - Pending messages are deleted after delivery to the agent server (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1998-2006`).
   - User-deleted secrets (`openhands/app_server/secrets/secrets_router.py:344-383`) and profiles (`openhands/app_server/settings/settings_router.py:525-540`).

4. **Does compaction break replay?**
   - There is no compaction, so the question is moot for storage. Replay from `iter_events_for_export` (`openhands/app_server/event/event_service_base.py:145-156`) returns the full append-only event log in timestamp order. The condenser's output is recomputed at runtime from full history rather than rewriting stored events; this preserves replay but does nothing for storage size.

5. **Can users request deletion?**
   - Of a conversation: yes, via `DELETE /api/v1/app-conversations/{conversation_id}` (`openhands/app_server/app_conversation/app_conversation_router.py:892-978`). However, this does not explicitly remove the on-disk event files in the app server's own event store (see failure modes below).
   - Of their account / all data: no. The V1 user router has no `DELETE /users/me` or equivalent (`openhands/app_server/user/user_router.py:21-58`). There is no GDPR/right-to-erasure flow in `openhands/app_server/`. (Enterprise integration paths exist for OAuth token revocation in `enterprise/server/auth/`, but that is a different package.)

## Architectural Decisions

- **Per-event files as the canonical event store.** `EventServiceBase` stores one JSON file per event (`openhands/app_server/event/event_service_base.py:83-89`, `openhands/app_server/event/filesystem_event_service.py:33-42`). This makes append trivial but means "delete a conversation" requires directory-level deletion, which is not implemented in the deletion path.
- **Pluggable event-service backends.** `FilesystemEventService`, `AwsEventService`, `GoogleCloudEventService` (`openhands/app_server/event/aws_event_service.py:27-76`, `openhands/app_server/event/google_cloud_event_service.py:37-70`) all model events as objects in a flat namespace. None of them implement `delete_event`, so retention policy must be uniform across backends — currently, none.
- **Pause-not-delete for sandbox quota.** `SandboxService.pause_old_sandboxes` (`openhands/app_server/sandbox/sandbox_service.py:203-244`) pauses the oldest running sandboxes when per-user count exceeds the cap. State is preserved; only compute is freed. This is a deliberate "don't lose work" choice that pushes state growth to long-lived paused sandboxes.
- **In-memory condensation for the LLM, not the DB.** The V1 app server uses `LLMSummarizingCondenser` from `openhands.sdk` (`openhands/app_server/app_conversation/app_conversation_service_base.py:578-612`). The choice reflects "compute context budget" not "storage budget"; persisted events remain the source of truth.
- **Transactional SQL deletion with separate file-store deletion.** `_delete_from_database` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2271-2290`) is the transactional core. The event-store counterpart is delegated to the agent server (HTTP DELETE) but never to the file store that backs `FilesystemEventService` / `AwsEventService` / `GoogleCloudEventService`.
- **Export-size guard as the only event-count cap.** `export_max_events=10000` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:223-226`, `2304-2313`) raises `ConversationExportTooLarge`. This bounds the size of an export operation but does not prune storage.

## Notable Patterns

- **Append-only event log**: events are written once and never rewritten or removed (`openhands/app_server/event/event_service_base.py:190-198`).
- **Cascade via SQL, not via file store**: `_delete_sub_conversations` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2203-2236`) deletes child metadata rows; child event directories are not explicitly cleaned.
- **Sandbox quota as a side-effect**: `pause_old_sandboxes` runs synchronously at the start of every `start_sandbox` / `resume_sandbox` (`openhands/app_server/sandbox/remote_sandbox_service.py:425, 509`; `openhands/app_server/sandbox/docker_sandbox_service.py:395, 520`). The pruning is therefore tied to user activity, not to a clock.
- **TTL as auth/cache, not history**: `JWT expires_in`, OAuth `expires_at`, and sandbox-spec cache TTL are the only TTL fields in the app server (`openhands/app_server/services/jwt_service.py:69-98`; `openhands/app_server/sandbox/dynamic_remote_sandbox_spec_service.py:43-73`). None of them touch event history.
- **Log redaction at write time**: secrets are scrubbed from log lines via `redact_text_secrets`, `redact_api_key_literals`, `redact_url_params` (`openhands/app_server/utils/logger.py:17-20, 290-294`). This is a privacy-mitigation pattern, not a deletion pattern.

## Tradeoffs

- **Append-only simplicity vs. unbounded growth.** Per-event files make write paths atomic (`openhands/app_server/file_store/local.py:33-43`), make `iter_events_for_export` deterministic, and remove the need for a compaction/replay subsystem. The cost is that event counts and on-disk bytes grow linearly with conversation length forever.
- **Pause vs. delete for sandboxes.** Pausing preserves conversation history in the sandbox and lets users resume, but it pins storage while freeing compute. Over time, paused sandboxes accumulate as a second retention liability alongside the event-store.
- **Condenser on the LLM side only.** Putting compaction in the LLM layer keeps the persistence model simple but means cost is paid on every context-window expansion; the storage never shrinks. This is acceptable if storage is cheap and the LLM is the bottleneck, but it does not satisfy the dimension's premise.
- **Delegating agent-server deletion to a remote HTTP call.** `_delete_from_agent_server` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2238-2269`) swallows failures and continues with DB cleanup. This is robust for partial failures but masks agent-server-side retention issues from the app server.
- **Per-user sandbox quota.** Limits per-user blast radius but does nothing across users. The aggregate storage growth across all users is bounded only by hardware.

## Failure Modes / Edge Cases

- **Orphaned event files after conversation delete.** `delete_app_conversation` (`openhands/app_server/app_conversation/app_conversation_router.py:892-978`) does not call any `file_store.delete()` on the conversation's event directory. The DB row is gone, but `{persistence_dir}/{user_id}/v1_conversations/{conversation_id_hex}/*.json` remains. No test asserts the directory is purged.
- **Shared-sandbox skip deletion.** When a sandbox is shared (`sandbox_is_shared=True` in `app_conversation_router.py:928-940`), `skip_agent_server_delete=True` is passed. Subsequent explicit deletes on co-hosted conversations leave the agent-server-side conversation live. No compensating cleanup exists.
- **Pause leak.** `pause_old_sandboxes` only sees running sandboxes (`openhands/app_server/sandbox/remote_sandbox_service.py:598-621`). Paused sandboxes that never get resumed continue to hold disk in the runtime; nothing in app_server reaps them by age.
- **Export refusal doesn't tell the user what to do.** `_validate_conversation_export_size` raises `ConversationExportTooLarge` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2304-2313`), but there is no API to chunk, summarize, or delete part of the event log to make export succeed. The 10k cap becomes a hard ceiling with no workaround.
- **No idempotency on delete.** `_delete_from_database` (`live_status_app_conversation_service.py:2271-2290`) returns a boolean and the router checks `deleted` (`app_conversation_router.py:942-943`); repeated calls return 404 cleanly, but partial failure mid-call (e.g., DB row removed, agent-server call throws) leaves the system in an inconsistent state with no recovery path. No retries or reconciliation job.
- **Sub-conversation orphans.** `_delete_sub_conversations` (`live_status_app_conversation_service.py:2203-2236`) iterates and continues on per-child errors. A single failing child leaves its event files in place while siblings are deleted.
- **`export_max_events=0` disables validation silently.** `_validate_conversation_export_size` (`live_status_app_conversation_service.py:2305-2306`) returns immediately when `export_max_events <= 0`. There is no warning that an operator has effectively removed the cap.

## Future Considerations

- Implement a `delete_event_directory(conversation_id)` on each `EventService` backend (filesystem / S3 / GCS) and call it from `_delete_from_database` after the SQL row is removed, so conversation delete is end-to-end.
- Add a background sweep that periodically prunes event files for `AppConversationInfo` rows older than N days, especially for paused / archived sandboxes.
- Add a `max_event_age_seconds` (or per-conversation event-count cap) on `StoredConversationMetadata` so the app server has a real retention policy, with opt-out for paying users.
- Promote the condenser pattern to storage: write a `CondensedEvent` derived record so replay can read either raw or condensed events, bounded by a stored `head_event_id` snapshot.
- Add `DELETE /api/v1/users/me` (and cascading conversation / secret / settings delete) for GDPR/right-to-erasure parity with the secrets endpoint pattern at `openhands/app_server/secrets/secrets_router.py:344-383`.
- Replace silent `export_max_events <= 0` skip with a logged warning at startup.
- Tests asserting that `delete_app_conversation` removes the on-disk event directory (or, if out of scope is intentional, document the policy that storage is event-store-managed and outside the app server).

## Questions / Gaps

- No evidence was found in `openhands/app_server/` for any **time-based event pruning** (e.g., delete events older than N days). Search boundary: grep of `openhands/openhands/app_server/` for `retention|prune|compact|compress|expir|summari` returned only auth/cache TTL hits.
- No evidence was found for a **background/scheduled GC** of conversation state. Search boundary: `grep` for `cron|scheduled|periodic` in `openhands/app_server/` returned only Redis-lock refresh and Docker-image pull progress, no retention sweepers.
- No evidence was found for a **user-account deletion** endpoint. Search boundary: V1 user router is only 58 lines (`openhands/app_server/user/user_router.py:1-58`); no DELETE handler.
- It is unclear whether the **agent-server** (external `openhands-agent-server==1.29.0` package) prunes its own conversation storage on the DELETE call from `live_status_app_conversation_service.py:2238-2269`; that package is not in this source tree and is out of scope.
- The `OH_MAX_API_SECRETS_COUNT` and friends at `openhands/app_server/constants.py:16-27` are character/count caps on user secrets, not retention policies for conversation history.
- No clear evidence was found for **token-budget pruning at the conversation level** (no `token_budget`, no `max_context_*` field on `AppConversationInfo`); the only token-aware behavior is the LLM-side condenser at `app_conversation_service_base.py:578-612`.

---

Generated by `02.09-state-pruning-compaction-and-retention` against `openhands`.