# Source Analysis: openhands

## Dimension 02.05: Persistence Durability Tiers

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python / FastAPI + SQLAlchemy 2.0 (async) + Alembic + Pydantic |
| Analyzed | 2026-07-10 |

## Summary

OpenHands' app server splits persistent state across two distinct store families with explicit abstract interfaces:

1. **A `FileStore` blob abstraction** (`openhands/app_server/file_store/files.py:7`) with four concrete backends — local filesystem, in-memory, S3, and Google Cloud Storage — used for events, user settings, secrets, and the encryption keyring.
2. **A SQLAlchemy relational layer** (`openhands/app_server/utils/sql_utils.py:9`) fronted by a single `DbSessionInjector` (`openhands/app_server/services/db_session_injector.py:27`) that transparently selects **SQLite** (dev/default) or **PostgreSQL** (host/GCP) and is migrated by Alembic on startup.

Event storage is a third, parallel abstraction (`EventServiceBase`, `openhands/app_server/event/event_service_base.py:31`) with filesystem/S3/GCS implementations chosen by `get_storage_provider()` (`openhands/app_server/utils/environment.py:21`). Sandbox/workspace state lives in ephemeral Docker containers + named volumes that are destroyed on delete (`openhands/app_server/sandbox/docker_sandbox_service.py:564-569`).

The tiers are cleanly separated with abstract base classes, discriminated-union injectors, and SQLite-backed unit tests, and the local file writer uses atomic `fsync`+`os.replace` (`openhands/app_server/file_store/local.py:31-42`). However, there is **no operator-facing document mapping each state category to its durability guarantee**, the default configuration (local SQLite + local filesystem events under `~/.openhands`) is single-instance-only, and the `InMemoryFileStore` silently drops everything on process death without any guardrail preventing its use outside tests.

## Rating

**6 / 10** — Present with strong interfaces and tests, but weakly documented and fragile at the defaults.

Rationale: The system earns credit for explicit store interfaces (`FileStore`, `SettingsStore`, `SecretsStore`, `EventService`, `PendingMessageService`), multiple pluggable durable backends (Postgres, S3, GCS), Alembic schema migrations run automatically on startup (`openhands/app_server/app_lifespan/oss_app_lifespan_service.py:16-35`), atomic local file writes with `os.fsync` (`openhands/app_server/file_store/local.py:36-38`), connection safeguards (`pool_pre_ping=True`, `pool_recycle`, `openhands/app_server/services/db_session_injector.py:207-211`), and SQLite-in-memory unit tests for every SQL service. It falls short of 7-8 because: (a) the durability model is implicit — no single reference tells an operator which state is production-durable; (b) the shipped defaults (SQLite file + `FilesystemEventService` writing to a local dir) survive restart but **not** redeploy or multi-instance operation; (c) `InMemoryFileStore` is a silent-drop store with no environment guard; and (d) recovery is limited to schema migration — there is no data-level recovery/repair logic.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Blob store interface | Abstract `FileStore` with `write/read/list/delete`, `extra='forbid'` | `openhands/app_server/file_store/files.py:7-30` |
| In-memory (volatile) store | `InMemoryFileStore` keeps files in a `dict`, no persistence | `openhands/app_server/file_store/memory.py:9-11` |
| Local file store (durable, atomic) | temp-write + `f.flush()` + `os.fsync()` + `os.replace()` | `openhands/app_server/file_store/local.py:31-42` |
| S3 store | boto3-backed, lazy client, bucket from `AWS_S3_BUCKET` | `openhands/app_server/file_store/s3.py:22-55` |
| GCS store | GCS-backed blob store | `openhands/app_server/file_store/google_cloud.py:1-60` |
| Store factory / default | `get_file_store` falls back to `InMemoryFileStore` for unknown type | `openhands/app_server/file_store/__init__.py:8-21` |
| Default file store = local | `_get_default_file_store()` returns `LocalFileStore(root=persistence_dir)` | `openhands/app_server/config.py:186-193` |
| Default persistence dir | `~/.openhands` unless `OH_PERSISTENCE_DIR`/`FILE_STORE_PATH` set | `openhands/app_server/config.py:75-89` |
| DB engine selection | SQLite when no host; Postgres (asyncpg/pg8000) when `host`/GCP set | `openhands/app_server/services/db_session_injector.py:167-241` |
| SQLite dev URL | `sqlite+aiosqlite:///{persistence_dir}/openhands.db`, `NullPool` | `openhands/app_server/services/db_session_injector.py:191-206` |
| Postgres pool safeguards | `pool_pre_ping=True`, `pool_recycle`, `pool_size`, `max_overflow` | `openhands/app_server/services/db_session_injector.py:196-211` |
| SQL base / type decorators | `Base(DeclarativeBase)`, `UtcDateTime`, encrypted `StoredSecretStr` | `openhands/app_server/utils/sql_utils.py:9-92` |
| Startup migration (recovery) | `command.upgrade(cfg, 'head')` on `OssAppLifespanService.__aenter__` | `openhands/app_server/app_lifespan/oss_app_lifespan_service.py:14-35` |
| Alembic offline URL logic | Postgres if `db_session.host` else `sqlite:///.../openhands.db` | `openhands/app_server/app_lifespan/alembic/env.py:74-82` |
| 12 versioned migrations | `001.py`..`012.py` create/alter tables | `openhands/app_server/app_lifespan/alembic/versions/001.py:29-59` |
| Conversation metadata table | `conversation_metadata` (repo, title, cost/token metrics) | `openhands/app_server/app_conversation/sql_app_conversation_info_service.py:64-100` |
| Start-task table | `app_conversation_start_task` | `openhands/app_server/app_conversation/sql_app_conversation_start_task_service.py:54-55` |
| Event-callback tables | `event_callback`, `event_callback_result` | `openhands/app_server/event_callback/sql_event_callback_service.py:48-77` |
| Pending messages table | `StoredPendingMessage` (`pending_messages`) with JSON content | `openhands/app_server/pending_messages/pending_message_service.py:26-37` |
| Event store interface | `EventServiceBase` with `_load_event/_store_event/_search_paths` | `openhands/app_server/event/event_service_base.py:31-52` |
| Filesystem event store | writes `event.model_dump_json` to a file path | `openhands/app_server/event/filesystem_event_service.py:22-41` |
| S3 event store | `put_object`/`get_object` per event | `openhands/app_server/event/aws_event_service.py:52-75` |
| GCS event store | GCS blob per event | `openhands/app_server/event/google_cloud_event_service.py:56-60` |
| Event provider selection | `SHARED_EVENT_STORAGE_PROVIDER`/`FILE_STORE` -> AWS/GCP/FILESYSTEM | `openhands/app_server/utils/environment.py:21-42` |
| Event backend wired in config | default `FilesystemEventServiceInjector()` | `openhands/app_server/config.py:305-328` |
| Settings store (file JSON) | `FileSettingsStore` reads/writes `settings.json` via `FileStore` | `openhands/app_server/settings/file_settings_store.py:14-49` |
| Secrets store (file JSON) | `FileSecretsStore` reads/writes `secrets.json` via `FileStore` | `openhands/app_server/secrets/file_secrets_store.py:12-44` |
| Encryption keyring persistence | `.keys` / `.jwt_secret` file in `persistence_dir`, generated + written if absent | `openhands/app_server/utils/encryption_key.py:29-86` |
| Secret-at-rest encryption | `StoredSecretStr` JWE-encrypts secrets before DB write | `openhands/app_server/utils/sql_utils.py:42-70` |
| Sandbox volume teardown | workspace volume removed on `delete_sandbox` | `openhands/app_server/sandbox/docker_sandbox_service.py:564-569` |
| SQLite in-memory test rig | `sqlite+aiosqlite:///:memory:` + `StaticPool` | `tests/unit/app_server/test_pending_message_service.py:29-30` |
| SQLite fallback test | asserts dev URL `sqlite:///.../openhands.db` | `tests/unit/app_server/test_db_session_injector.py:170-186` |
| Legacy template default | `#file_store = "memory"` shown as example default | `config.template.toml:44` |

## Answers to Dimension Questions

**1. What survives a restart?**
- Anything in the SQL database: conversation metadata, start tasks, event callbacks + results, and pending messages (`openhands/app_server/pending_messages/pending_message_service.py:26`). With the default SQLite file at `{persistence_dir}/openhands.db` this survives restart on the same host (`openhands/app_server/services/db_session_injector.py:237`).
- `FileStore`-backed state written to `LocalFileStore` (default): user `settings.json` (`openhands/app_server/settings/file_settings_store.py:44`), `secrets.json` (`openhands/app_server/secrets/file_secrets_store.py:32`), the encryption keyring `.keys` (`openhands/app_server/utils/encryption_key.py:85`), and filesystem events (`openhands/app_server/event/filesystem_event_service.py:32`).
- **Does NOT survive restart:** anything in `InMemoryFileStore` (`openhands/app_server/file_store/memory.py:9`) and the running sandbox process/container conversation runtime.

**2. What survives redeploy?**
- Only survives redeploy if a network-durable backend is configured: PostgreSQL (host/GCP branch, `openhands/app_server/services/db_session_injector.py:167-190`) and S3/GCS event + file stores. A redeploy that changes the host/loses the local volume loses the default SQLite DB and the `~/.openhands` file tree, since both are anchored to `persistence_dir` (`openhands/app_server/config.py:75-89`).
- Docker sandboxes and their named workspace volumes are per-instance and are explicitly removed on delete (`openhands/app_server/sandbox/docker_sandbox_service.py:564-569`); they are not part of the redeploy-durable tier.

**3. What survives multi-instance operation?**
- PostgreSQL (shared DB) and S3/GCS blob/event stores are the only multi-instance-safe tiers. The encryption keyring is made deterministic across pods via `JWT_SECRET` (`openhands/app_server/utils/encryption_key.py:34-48`), acknowledging multi-pod deployment.
- Default SQLite + local filesystem stores are **not** multi-instance safe (each instance has a private `openhands.db` and private `~/.openhands` tree). No distributed locking or shared-file coordination exists for the local tiers.

**4. What is silently dropped?**
- `InMemoryFileStore` (`openhands/app_server/file_store/memory.py:9`) drops all settings/secrets/events on process death; `get_file_store` returns it as the fallback for any unrecognized `file_store_type` (`openhands/app_server/file_store/__init__.py:20-21`), and the legacy template even shows `file_store = "memory"` as an example (`config.template.toml:44`). No warning is logged when the volatile store is selected.
- The `InMemoryRateLimiter` state is per-process and volatile (`openhands/app_server/middleware.py:78`), so rate-limit counters reset on restart and are not shared across instances.
- Sandbox workspace volumes are silently discarded on `delete_sandbox` (`openhands/app_server/sandbox/docker_sandbox_service.py:564-569`).

**5. Is durability configurable?**
- Yes, via environment/DI. DB tier: `DB_HOST`/`DB_NAME`/`DB_USER`/`DB_PASS` and `GCP_DB_INSTANCE` switch SQLite→Postgres/CloudSQL (`openhands/app_server/services/db_session_injector.py:53-71`). Blob/event tier: `SHARED_EVENT_STORAGE_PROVIDER` / `FILE_STORE` select AWS/GCP/filesystem (`openhands/app_server/utils/environment.py:30-42`). Path: `OH_PERSISTENCE_DIR`/`FILE_STORE_PATH` (`openhands/app_server/config.py:77-81`). Every backend is a discriminated-union injector overridable through `OH_`-prefixed config (`openhands/app_server/config.py:288`).

## Architectural Decisions

- **Two orthogonal persistence families.** Structured, queryable state (conversations, callbacks, pending messages) lives in SQL; opaque documents (settings, secrets, events, keys) live in the `FileStore` blob abstraction. This lets each family scale to its own backend independently (`openhands/app_server/config.py:216-231`, `openhands/app_server/config.py:305-328`).
- **Backend selection by environment, defaulting to local.** `config_from_env()` chooses SQLite + filesystem unless cloud env vars are present (`openhands/app_server/config.py:305-380`), optimizing for zero-config local dev while allowing Postgres/S3/GCS for production.
- **Schema durability via Alembic-on-startup.** Migrations run automatically in `OssAppLifespanService` so the DB schema self-heals to `head` on every boot (`openhands/app_server/app_lifespan/oss_app_lifespan_service.py:16-35`).
- **Secrets encrypted at rest** using a JWE keyring persisted to `.keys`, with a deterministic key ID derived from `JWT_SECRET` for cross-pod consistency (`openhands/app_server/utils/sql_utils.py:42-70`, `openhands/app_server/utils/encryption_key.py:34-48`).
- **Atomic local writes.** `LocalFileStore.write` uses a pid/thread-scoped temp file, `fsync`, and atomic `os.replace` to avoid torn writes under concurrency (`openhands/app_server/file_store/local.py:31-42`).

## Notable Patterns

- **Discriminated-union injectors** provide runtime-swappable stores (`FilesystemEventServiceInjector`, `SQLPendingMessageServiceInjector`, etc.) resolved lazily via FastAPI DI (`openhands/app_server/pending_messages/pending_message_service.py:190-205`).
- **Request-scoped `AsyncSession`** with auto-commit-on-success / rollback-on-error and a `keep_open` escape hatch (`openhands/app_server/services/db_session_injector.py:257-320`).
- **TypeDecorator layer** normalizes cross-dialect quirks: `UtcDateTime` compensates for SQLite's lack of timezone storage (`openhands/app_server/utils/sql_utils.py:73-92`), and JSON columns store Pydantic-serialized lists (`openhands/app_server/utils/sql_utils.py:24-40`).
- **One-file-per-event storage** across all event backends via the shared `EventServiceBase` template method pattern (`openhands/app_server/event/event_service_base.py:44-52`).

## Tradeoffs

- **Zero-config local default vs. production durability.** SQLite + local filesystem is friction-free locally but silently non-durable across redeploys/instances; the switch to Postgres/S3 is entirely env-var driven with no startup assertion that a durable backend is in use.
- **File-per-event simplicity vs. scale.** Storing each event as an individual blob (`filesystem_event_service.py:32`, `aws_event_service.py:57`) is simple and backend-portable but incurs one object op per event and a `glob`/`list_objects` per page (`filesystem_event_service.py:37`), which is costly for long conversations.
- **SQLite `NullPool` correctness vs. throughput.** Using `NullPool` for SQLite (`db_session_injector.py:203`) avoids cross-thread connection issues but forgoes pooling; production Postgres uses a real pool with pre-ping.
- **Encryption keyring auto-generation vs. portability.** If no `JWT_SECRET` is set, a random key is generated and written to `.keys` (`encryption_key.py:74-85`); losing that file renders all DB-encrypted secrets unrecoverable, coupling secret durability to a single file.

## Failure Modes / Edge Cases

- **Silent volatile store.** Unknown `file_store_type` → `InMemoryFileStore` with no warning (`openhands/app_server/file_store/__init__.py:20`); an operator misconfiguring the type loses all settings/secrets/events on restart.
- **Lost encryption key = unreadable secrets.** `StoredSecretStr` decryption requires the keyring; a missing/rotated `.keys` (or changed `JWT_SECRET`) makes stored secrets undecryptable (`openhands/app_server/utils/sql_utils.py:62-70`).
- **Divergent local state under multi-instance.** Two instances sharing a Postgres but each with its own local `FileStore` would present inconsistent settings/secrets/events, since those are not forced onto shared storage.
- **`persistence_dir` on ephemeral disk.** Because SQLite and all local blobs anchor to `persistence_dir` (`config.py:75-89`), running in a container without a mounted volume loses everything on restart despite the code paths being "durable."
- **Event read resilience.** `FilesystemEventService._load_event` swallows parse errors and returns `None` (`filesystem_event_service.py:22-30`), so a corrupted event file is silently skipped rather than surfaced.

## Future Considerations

- Add a startup log/assertion reporting the resolved durability tier for each state category (DB dialect, file store class, event backend) so operators can confirm production-durable configuration.
- Guard `InMemoryFileStore` behind an explicit opt-in (e.g., a `dev`/`test` flag) or emit a WARNING when selected in a non-test context.
- Document a state → durability-tier matrix (restart / redeploy / multi-instance) tied to the concrete classes here; currently the READMEs (`file_store/README.md`, `event/README.md`) describe backends but not durability guarantees.
- Consider event batching/segment files for the file-per-event stores to reduce per-event object operations.

## Questions / Gaps

- **Agent runtime conversation state durability is out of scope of this source.** The actual conversation/event runtime lives in `openhands.agent_server` / `openhands.sdk`, which are external pip dependencies (`pyproject.toml:61` pins `openhands-sdk==1.29.0`) and are not present in this directory. How the sandboxed agent persists and replays in-flight conversation state (and whether it survives sandbox pause/resume) could not be verified here — No clear evidence found within the selected source.
- **Backup/restore or point-in-time recovery** for the SQLite/local tiers: No clear evidence found; recovery is limited to Alembic schema migration on startup (`oss_app_lifespan_service.py:23-35`).
- **Enterprise storage tier** (`enterprise/storage`, `enterprise/migrations`) exists in the tree but was not analyzed in depth as it targets the hosted deployment; its relationship to the OSS tiers is a gap for a follow-up focused study.

---

Generated by `02.05-persistence-durability-tiers` against `openhands`.
