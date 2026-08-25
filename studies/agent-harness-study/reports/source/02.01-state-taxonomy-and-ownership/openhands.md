# Source Analysis: openhands

## Dimension 02.01: State Taxonomy and Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12, FastAPI + SQLAlchemy (async) + Pydantic v2; file/DB/object-store backends |
| Analyzed | 2026-07-04 |

## Summary

The studied tree is the OpenHands **V1 application server** (`openhands/app_server/`), a FastAPI control-plane that brokers sandboxed conversations executed by an external agent-server / `openhands-sdk` (not vendored in this source). Consequently the app_server owns a well-defined slice of state and *deliberately delegates* the rest across a process boundary.

The dominant architecture is a **control-plane / data-plane split**:

- **Control plane (this source, durable):** conversation metadata, the append-only event log, user settings, secrets, event-callback registrations + results, conversation-start tasks, and a pending-message queue. These live in SQL tables and a pluggable file store.
- **Data plane (external agent-server inside the sandbox, ephemeral / derived):** live agent execution status, tool state, agent memory/context, and workspace filesystem artifacts. The app_server never persists these; it fetches them live over HTTP and composes them into response models (`live_status_app_conversation_service.py:589`, `:630`).

State is consistently split into a persisted "info" record and a runtime-derived "live" projection: `AppConversationInfo` (durable) vs `AppConversation` (info + live sandbox status + live execution status), and `SandboxRecord` (durable id/owner) vs `SandboxInfo` (live runtime status/URLs/session key). Durable stores sit behind abstract base classes with SQL and file implementations wired by dependency-injection injectors (`config.py:191`).

## Rating

**7 / 10** — Clear, layered model with explicit interfaces (abstract stores, typed SQLAlchemy models with indexes, DI injectors) and real operational safeguards (atomic+fsync file writes, immutable/frozen secrets). The repo can largely explain what must survive a crash: durable state has concrete tables/files, and ephemeral state is explicitly reconstructed. It falls short of 8+ because (a) crash-critical execution/tool/memory state is *owned outside this source*, weakening any single-source-of-truth guarantee; (b) metrics are denormalized into `conversation_metadata` columns *and* live in the event stream, with a hardcoded `total_tokens=0` on save (`sql_app_conversation_info_service.py:367`); (c) `Settings.secrets_store` is frozen and serialized as an empty dict, signalling an in-progress migration seam (`settings_models.py:118`, `:382`); and (d) there is no single documented state map — the taxonomy must be reconstructed from per-module code.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Conversation metadata (durable, source-of-truth) | `AppConversationInfo` pydantic model — identity, repo/branch, trigger, tags, metrics | `openhands/app_server/app_conversation/app_conversation_models.py:112-156` |
| Conversation metadata SQL table | `StoredConversationMetadata(Base)` table `conversation_metadata`, `conversation_version='V1'` scoping, indexed `sandbox_id`/`parent_conversation_id` | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:65-117` |
| Conversation metadata owner | `SQLAppConversationInfoService.save_app_conversation_info` uses `merge`+`commit`; comment: stores record "even after its sandbox ceases to exist" | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:120-125`, `:347-388` |
| Composite live conversation (derived, ephemeral) | `AppConversation` adds `sandbox_status`, `execution_status`, `conversation_url`, `session_api_key` (defaults MISSING/None) | `openhands/app_server/app_conversation/app_conversation_models.py:173-191` |
| Execution status is fetched live, not persisted | `_get_live_conversation_info` GETs `/api/conversations` from agent-server; `_build_conversation` merges live status onto stored info | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:589-663` |
| Event log (durable transcript) | `EventServiceBase` — path `{prefix}/{user_id}/v1_conversations/{conversation_id.hex}/{event_id}.json`; `save_event`, `iter_events_for_export` | `openhands/app_server/event/event_service_base.py:66-84`, `:145-157`, `:190-197` |
| Event store file backend | `FilesystemEventService._store_event` writes `event.model_dump_json`; `_load_event` validates JSON | `openhands/app_server/event/filesystem_event_service.py:23-42` |
| Atomic durability safeguard | `LocalFileStore.write` writes temp file, `f.flush()`+`os.fsync`, then `os.replace` (atomic rename) | `openhands/app_server/file_store/local.py:26-43` |
| Event backends are pluggable (cross-boundary) | AWS / Google Cloud / filesystem event services co-exist | `openhands/app_server/event/` (`aws_event_service.py`, `google_cloud_event_service.py`, `filesystem_event_service.py`) |
| Start-task state (durable job record) | `AppConversationStartTask` with phased status enum WORKING→READY/ERROR; `app_conversation_id`/`sandbox_id` populated on READY | `openhands/app_server/app_conversation/app_conversation_models.py:262-305` |
| Start-task SQL table | `StoredAppConversationStartTask(Base)` table `app_conversation_start_task`, request stored as JSON | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:54-70` |
| Sandbox state — persisted identity only | `SandboxRecord` (id + owner) docstring: "Persisted identity fields … no live runtime data" | `openhands/app_server/sandbox/sandbox_models.py:33-46` |
| Sandbox state — live/derived | `SandboxInfo` (status, `session_api_key`, `exposed_urls`) — session key None when STARTING/PAUSED | `openhands/app_server/sandbox/sandbox_models.py:48-72` |
| Sandbox status lifecycle | `SandboxStatus` enum STARTING/RUNNING/PAUSED/ERROR/MISSING | `openhands/app_server/sandbox/sandbox_models.py:9-15` |
| Sandbox source-of-truth is the runtime, not the DB | `DockerSandboxService` lists live containers + labels to derive state; no `Stored*` table | `openhands/app_server/sandbox/docker_sandbox_service.py:291`, `:344-366`, `:454-457` |
| Settings state (durable user config) | `Settings` model — agent_settings, conversation_settings, llm_profiles, product fields; `secrets_store` frozen | `openhands/app_server/settings/settings_models.py:105-148` |
| Settings store abstraction (extension point) | `SettingsStore` ABC with `load`/`store`/`get_instance` | `openhands/app_server/settings/settings_store.py:8-36` |
| Settings file persistence | `FileSettingsStore.store` dumps JSON to file store with `expose_secrets`/`persist_settings` | `openhands/app_server/settings/file_settings_store.py:41-45` |
| Secrets state (durable, sensitive, immutable) | `Secrets` model `frozen=True`, MappingProxyType, SecretStr, redacted serialization unless `expose_secrets` | `openhands/app_server/secrets/secrets_models.py:36-107` |
| Secrets migration seam | `Settings.secrets_store` `Field(frozen=True)` + serializer returns empty `{'provider_tokens': {}}`; comment "Planned to be removed from settings" | `openhands/app_server/settings/settings_models.py:117-118`, `:382-384` |
| Pending message queue (durable but transient) | `PendingMessage` docstring: stored in DB, delivered on READY, "deleted after processing, regardless of success or failure" | `openhands/app_server/pending_messages/pending_message_models.py:12-24` |
| Pending message SQL table + owner | `StoredPendingMessage` table `pending_messages`; `SQLPendingMessageService` add/get/delete/`update_conversation_id` (task→real id) | `openhands/app_server/pending_messages/pending_message_service.py:27-38`, `:75-185` |
| Event-callback registration state (durable) | `EventCallback` (status ACTIVE/DISABLED/COMPLETED/ERROR); `StoredEventCallback` table `event_callback` indexed on (conversation_id, status, event_kind) | `openhands/app_server/event_callback/event_callback_models.py:33-99`, `openhands/app_server/event_callback/sql_event_callback_service.py:48-73` |
| Event-callback result state (eval-like) | `StoredEventCallbackResult` table `event_callback_result` records processor outcome | `openhands/app_server/event_callback/sql_event_callback_service.py:76-80` |
| Metrics / cost / token state (denormalized projection) | Metrics columns on `conversation_metadata`; `update_conversation_statistics` / `process_stats_event` reduce stats events into columns | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:89-99`, `:390-513` |
| Metrics fragility (hardcoded) | `total_tokens=0` written unconditionally on save | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:367` |
| Runtime configuration (ephemeral, env-built) | `AppServerConfig` holds `persistence_dir`, `file_store`, `db_session` injector, all service injectors; `config_from_env` | `openhands/app_server/config.py:191-238`, `:240-285` |
| Persistence dir resolution | `get_default_persistence_dir` from `OH_PERSISTENCE_DIR`/`FILE_STORE_PATH` | `openhands/app_server/config.py:75-84`, `:186-193` |
| Cross-boundary credential flow (settings/secrets → sandbox) | `_seed_sandbox_profiles` mirrors LLM profiles into the sandbox; secrets pushed as `LookupSecret`/`StaticSecret` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:925-1000`, `:1093-1144` |
| User-scoped path isolation for events | `get_conversation_path` prefixes with `user_id` when available | `openhands/app_server/event/event_service_base.py:66-84` |

## Answers to Dimension Questions

**1. What kinds of state exist?**
Within the app_server boundary:
- **Conversation state (metadata):** `AppConversationInfo` / `conversation_metadata` (`app_conversation_models.py:112`, `sql_app_conversation_info_service.py:65`).
- **Execution state (agent status):** `execution_status` on `AppConversation` — derived from the agent-server, never persisted (`app_conversation_models.py:178`, `live_status_app_conversation_service.py:589`).
- **Event / history state:** file-backed `Event` JSON log (`event_service_base.py:190`, `filesystem_event_service.py:33`).
- **Start-task state:** `AppConversationStartTask` / `app_conversation_start_task` (`app_conversation_models.py:281`, `sql_app_conversation_start_task_service.py:54`).
- **Sandbox state:** `SandboxRecord` (persisted id/owner) + `SandboxInfo` (live) (`sandbox_models.py:33-72`).
- **Settings (runtime config) state:** `Settings` via `SettingsStore`/`FileSettingsStore` (`settings_models.py:105`, `file_settings_store.py:41`).
- **Secrets state:** `Secrets` frozen model (`secrets_models.py:36`).
- **Approval/eval knobs:** `conversation_settings` (confirmation_mode, security_analyzer, max_iterations) referenced in `settings_models.py:108-110` — enforced by the SDK, not stored per-run here.
- **Event-callback state + results:** `event_callback` / `event_callback_result` (`sql_event_callback_service.py:48`, `:76`).
- **Pending-message queue state:** `pending_messages` (`pending_message_service.py:27`).
- **Metrics/eval state:** denormalized metrics columns + stats reduction (`sql_app_conversation_info_service.py:89`, `:390`).
- **Process runtime config:** `AppServerConfig` (`config.py:191`).
Tool state and agent memory/context state are **not present in this source** — owned by the external agent-server/SDK (see Gaps).

**2. Which state is source-of-truth?**
- Conversation identity/metadata → `conversation_metadata` SQL table (`sql_app_conversation_info_service.py:65`).
- Conversation transcript → the file-backed event log (`event_service_base.py:190`); trajectory export replays it (`:145`).
- User config → `settings.json` via `FileSettingsStore` (`file_settings_store.py:41`).
- Secrets → the `Secrets`/secrets store (`secrets_models.py:36`); note secrets have been *moved out of* `Settings` (`settings_models.py:382`).
- Sandbox liveness → the **Docker/remote runtime itself**, queried live (`docker_sandbox_service.py:291`); the DB only holds the `sandbox_id` reference.
- Live execution status → the **agent-server** (`live_status_app_conversation_service.py:589`).

**3. Which state is derived?**
- `AppConversation` (info + live sandbox status + live execution status), assembled by `_build_conversation` (`live_status_app_conversation_service.py:630-663`).
- `SandboxInfo` runtime fields (status, session key, exposed URLs) derived from Docker (`docker_sandbox_service.py:344-366`).
- `MetricsSnapshot` rebuilt from denormalized columns in `_to_info` (`sql_app_conversation_info_service.py:520-544`) and, in turn, projected from stats events (`:390-513`).
- The `acp_server` computed field is a projection of the `acpserver` tag (`app_conversation_models.py:142-156`).

**4. Which state is ephemeral?**
- `execution_status` / live `SandboxInfo` (recomputed each request).
- `AppServerConfig` (rebuilt from env per process, `config.py:240`).
- In-flight start tasks are conceptually transient (though persisted for resumability, `app_conversation_models.py:281`).
- `app_conversation_info_load_tasks` in-memory `asyncio.Task` cache (`event_service_base.py:40-42`).

**5. Which state is safe to lose?**
- Live `SandboxInfo` / `execution_status` — always re-derivable from the runtime and agent-server.
- `AppServerConfig` — re-derived from env.
- Pending messages are the ambiguous case: durable in SQL but "deleted after processing regardless of success or failure" (`pending_message_models.py:14-18`), so losing them silently drops queued user input.
- **Not** safe to lose: `conversation_metadata`, the event log, `settings.json`, secrets, event-callback rows — these are the crash-critical durable set.

## Architectural Decisions

- **Control-plane / data-plane separation.** The app_server persists durable metadata + events + config, while runtime execution/tool/memory state stays in the sandbox agent-server, accessed live (`live_status_app_conversation_service.py:589-663`). Conversation records intentionally outlive their sandboxes (`sql_app_conversation_info_service.py:123-125`; `SandboxStatus.MISSING` default, `app_conversation_models.py:174-177`).
- **Info vs live composite pattern.** `AppConversationInfo`→`AppConversation` and `SandboxRecord`→`SandboxInfo` cleanly separate durable identity from ephemeral runtime (`app_conversation_models.py:112`/`:173`; `sandbox_models.py:33`/`:48`).
- **Pluggable stores behind abstract bases + DI injectors.** `SettingsStore` (`settings_store.py:8`), `SandboxService` (`sandbox_service.py:30`), `EventService`, `PendingMessageService` (`pending_message_service.py:41`), each with file/SQL/cloud implementations selected in `config.py:191-238`.
- **Event log as the transcript source-of-truth**, one JSON file per event under a user-scoped path (`event_service_base.py:66-84`), enabling replay/export.
- **Secrets isolation.** `Secrets` is `frozen=True` and redacts on serialization unless an explicit `expose_secrets` context is set (`secrets_models.py:47-51`, `:58-89`); secrets are being migrated out of `Settings` (`settings_models.py:382`).

## Notable Patterns

- **Denormalized metrics projection:** cost/token counters cached as columns on `conversation_metadata` and refreshed by reducing `ConversationStateUpdateEvent` stats events (`sql_app_conversation_info_service.py:390-513`).
- **Immutable/value-object state:** `Secrets` uses `MappingProxyType` + `SecretStr` for an immutable secrets snapshot (`secrets_models.py:36-56`).
- **Atomic durability:** temp-file + fsync + rename in `LocalFileStore.write` (`local.py:31-43`).
- **User-scoped storage prefixes** for both events and settings, providing per-tenant isolation at the path level (`event_service_base.py:66-84`).
- **Task-id → conversation-id rekeying:** pending messages queued under a `task-{uuid}` are migrated to the real conversation id once known (`pending_message_service.py:168-185`).
- **Tag-as-column trick:** `acp_server` derived from the `acpserver` tag to avoid a dedicated DB column (`app_conversation_models.py:142-156`).

## Tradeoffs

- **Split ownership vs. simplicity.** Delegating execution/tool/memory state to the agent-server keeps the app_server DB small and lets conversations survive sandbox loss, but means no single store holds the full agent state — crash recovery of a *running* turn depends on the external sandbox, outside this source.
- **Denormalized metrics vs. consistency.** Caching metrics on `conversation_metadata` makes list views cheap but risks drift from the event stream; the hardcoded `total_tokens=0` on save (`sql_app_conversation_info_service.py:367`) is a live example of that drift.
- **File-per-event durability vs. scale.** Atomic JSON files are simple and crash-safe (`local.py:31-43`) but `search_events` loads and filters all event files in-process with a bounded semaphore (`event_service_base.py:56-64`, `:94-143`) — O(events) per query, fine for moderate transcripts, costly at scale.
- **Frozen secrets migration seam.** Keeping `Settings.secrets_store` frozen and empty-serialized (`settings_models.py:118`, `:382`) avoids breaking older `settings.json` but leaves two conceptual homes for secrets during the transition.

## Failure Modes / Edge Cases

- **Sandbox gone / archived:** `SandboxStatus.MISSING` default means a conversation with a deleted sandbox still loads from metadata with `execution_status=None` (`app_conversation_models.py:174-181`; `_build_conversation` `live_status_app_conversation_service.py:638`).
- **Legacy timezone-less rows:** SQLite stores no tz; `_fix_timezone` assumes UTC and falls back to `utc_now()` for null timestamps (`sql_app_conversation_info_service.py:577-588`).
- **Pending-message loss:** messages are deleted after processing "regardless of success or failure" (`pending_message_models.py:14-18`), so a delivery error drops the message.
- **Event read resilience:** `_load_event` swallows parse errors and returns `None`, skipping corrupt event files rather than failing the whole search (`filesystem_event_service.py:23-31`).
- **Stats update best-effort:** `process_stats_event` catches and logs all exceptions, so a malformed stats event won't corrupt metrics but may silently skip an update (`sql_app_conversation_info_service.py:472-512`).
- **Missing sandbox_id assertion:** `_to_info` asserts V1 rows always have a `sandbox_id` (`sql_app_conversation_info_service.py:526-527`) — a data-integrity violation would raise rather than degrade.

## Future Considerations

- Complete the secrets-out-of-`Settings` migration and remove the frozen empty `secrets_store` shim (`settings_models.py:118`, `:382`).
- Fix or document the hardcoded `total_tokens=0` and consider treating the event stream as the sole metrics source-of-truth with a materialized view rather than hand-maintained columns (`sql_app_conversation_info_service.py:367`).
- Add an explicit, documented state map (durable vs derived vs ephemeral) — currently reconstructable only from code.
- Consider an index/manifest for events to avoid full-directory scans in `search_events`/`count_events` at scale (`event_service_base.py:94-143`).
- Clarify pending-message durability semantics (dead-letter vs silent drop) (`pending_message_service.py:151-166`).

## Questions / Gaps

- **Tool state & agent memory/context state:** No clear evidence found within this source. These are owned by the external `openhands-sdk` / agent-server (imported as `openhands.sdk`, `openhands.agent_server`, absent from the vendored tree — confirmed via directory listing showing only `analytics`, `app_server`, `db`, `server`). Per the source-isolation rule they were not inspected; the app_server only *references* SDK types (`live_status_app_conversation_service.py:117-139`).
- **Approval state at runtime:** Only the *configuration* (`conversation_settings.confirmation_mode`, `security_analyzer`) is persisted here (`settings_models.py:108-110`); per-action approval records were not found in the app_server — presumed to live in the agent-server.
- **Eval state:** The closest analog is `event_callback_result` (`sql_event_callback_service.py:76`) and cost metrics; no dedicated evaluation/benchmark state store was found in this source.
- **Crash recovery of an in-flight turn:** Because execution state is external, this source alone cannot fully answer whether a mid-turn agent step survives a crash — the durable set (metadata + events + settings + secrets + callbacks + start tasks + pending messages) survives, but live agent progress depends on the sandbox.

---

Generated by `02.01-state-taxonomy-and-ownership` against `openhands`.
