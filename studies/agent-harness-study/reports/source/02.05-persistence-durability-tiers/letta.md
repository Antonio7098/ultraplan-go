# Source Analysis: letta

## Dimension 02.05: Persistence Durability Tiers

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python, SQLAlchemy (async) + Alembic, Postgres/pgvector, Redis, object storage (GCS/S3/local) |
| Analyzed | 2026-07-10 |

## Summary

Letta persists essentially all durable agent state in a **single relational database** accessed through SQLAlchemy's async ORM. The runtime engine is always constructed as a Postgres async engine (`letta/server/db.py:21`, `letta/server/db.py:58`), and the production Docker image ships an internal Postgres + pgvector store with migrations applied on boot (`letta/server/startup.sh:56`, `compose.yaml:3`). A second, weaker tier is SQLite, selected implicitly whenever no Postgres connection info is present (`letta/settings.py:492`); SQLite affects only ORM-level branching (vector columns, functions) and is mainly a dev/test convenience.

Beyond the primary DB there are secondary tiers with very different durability semantics: **Redis** (SSE stream chunks, conversation/memory-repo locks, OTID→run mappings) which silently degrades to an in-memory no-op when unconfigured (`letta/data_sources/redis_client.py:663`); **object storage** (GCS/S3/local filesystem) for git-backed memory repositories (`letta/services/memory_repo/storage/base.py:7`); and **provider-trace backends** (Postgres / ClickHouse / socket sidecar) that are explicitly pluggable and dual-write capable (`letta/services/provider_trace_backends/factory.py:30`).

The dominant risk to operator clarity is the **implicit Postgres-vs-SQLite selection** combined with the **silent Redis no-op fallback**: neither is surfaced as a hard failure, so an operator can run what looks like a working server while locks and streaming persistence are effectively disabled.

## Rating

**7 / 10** — There is a clear, coherent primary durability model: a single relational database with Alembic-managed schema versioning (167 migrations), soft-delete/timestamp mixins, connection-pool safeguards, retry logic, and an explicit pluggable provider-trace tier. Tests and interfaces exist. Points are deducted because durability tiering is *not* uniformly explicit to operators: the Postgres/SQLite choice is inferred from env presence rather than declared (`letta/settings.py:492`), the runtime engine ignores the SQLite branch and always builds Postgres (`letta/server/db.py:58`), and Redis silently falls back to a no-op that drops locks and stream chunks with only an `info`/`warning` log (`letta/data_sources/redis_client.py:669`, `:679`). This is a strong "clear model with tests and safeguards" but short of "mature, observable, proven under failure" for the secondary tiers.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Runtime DB engine (Postgres, always) | Async engine built from `settings.letta_pg_uri` unconditionally | `letta/server/db.py:21`, `letta/server/db.py:58` |
| Pooling / durability of connections | `NullPool` when pooling disabled (default `disable_sqlalchemy_pooling=True`); else pool_size/overflow/recycle | `letta/server/db.py:30-41`, `letta/settings.py:307-314` |
| Session commit/rollback safety | Explicit rollback on cancel/exception; retry on transient connection errors | `letta/server/db.py:88-116` |
| DB choice (implicit tiering) | `database_engine` = POSTGRES if pg info present else SQLITE | `letta/settings.py:492-493`, `letta/settings.py:482-489` |
| Default fallback URI | Falls back to `postgresql+pg8000://letta:letta@localhost:5432/letta` | `letta/settings.py:471-478` |
| Desktop file-based URI | Reads `~/.letta/pg_uri` when `--use-file-pg-uri` | `letta/settings.py:264-270` |
| Schema versioning / migrations | `alembic upgrade head` run at container startup, hard-fails on error | `letta/server/startup.sh:54-61` |
| Migration count | 167 migration scripts | `alembic/versions/` (167 files) |
| Base durability columns | `created_at`, `updated_at`, `is_deleted` on all rows | `letta/orm/base.py:15-17` |
| Soft-delete reads | Queries filter `is_deleted == False` when requested | `letta/orm/sqlalchemy_base.py:394-395`, `:531-532` |
| Archival memory (vectors) | pgvector `Vector` column on Postgres, `CommonVector` (JSON-ish) on SQLite | `letta/orm/passage.py:34-40` |
| File content storage | Full extracted file text stored in DB `file_contents.text` (Text column) | `letta/orm/file.py:21-31` |
| Job/run durable status | `jobs.status`, `completed_at`, `stop_reason` persisted | `letta/orm/job.py:24-46` |
| Redis client + no-op fallback | `get_redis_client` returns `NoopAsyncRedisClient` when unconfigured or on init failure | `letta/data_sources/redis_client.py:663-681` |
| No-op semantics | Locks return `None`, sets return `0`, streams return `[]`/`""` | `letta/data_sources/redis_client.py:562-660` |
| SSE stream persistence (Redis) | Stream chunks written with TTL (default 6h) and `maxlen` trimming | `letta/server/rest_api/redis_stream_manager.py:26-55`, `:146` |
| Object storage abstraction | Abstract `StorageBackend` for GCS/S3/local | `letta/services/memory_repo/storage/base.py:7-82` |
| Local object-store fallback | `LocalStorageBackend` under `~/.letta/memfs` for OSS/self-hosted git memory | `letta/services/memory_repo/memfs_client_base.py:44-56` |
| Object store config | `LETTA_OBJECT_STORE_URI` / memfs service URL | `letta/settings.py:327-347` |
| Provider-trace pluggable tiers | Postgres / ClickHouse / socket backends; dual-write list | `letta/services/provider_trace_backends/factory.py:30-41` |
| Provider-trace metadata-only mode | Option to write only metadata (drops request/response JSON) | `letta/settings.py:586` |
| Docker persistence volume | Postgres data mounted at `./.persist/pgdata` | `compose.yaml:13-15` |
| Internal Postgres/Redis on boot | Starts internal PG + Redis if no external config | `letta/server/startup.sh:24-52` |

## Answers to Dimension Questions

**1. What survives a restart?**
Everything in the relational DB: agents, messages, memory blocks, archival passages/embeddings, files + full file text, sources, tools, sandbox configs, providers, jobs, runs, steps, batch items, identities, and provider traces (`letta/orm/*.py`; base columns `letta/orm/base.py:15-17`). With Docker, Postgres data is on the `./.persist/pgdata` volume (`compose.yaml:13-15`). Git-backed memory repositories survive if stored in object storage or on a persisted local path (`letta/services/memory_repo/storage/base.py:7`, `memfs_client_base.py:52`). **Does NOT survive:** in-flight SSE stream buffers and locks held only in a no-op Redis client (`letta/data_sources/redis_client.py:562-660`).

**2. What survives redeploy?**
Same as restart, provided the DB and object store are external/persisted. Schema changes are reconciled by `alembic upgrade head` at startup, so a redeploy carrying new migrations forward-migrates the existing data (`letta/server/startup.sh:56`). Ephemeral, process-local state (module-level singleton engine `letta/server/db.py:58`, cached Redis client `letta/data_sources/redis_client.py:664`, `lru_cache` trace backends `factory.py:30`) is rebuilt, not persisted.

**3. What survives multi-instance operation?**
Only what lives in shared external stores: the Postgres DB, Redis (for cross-instance conversation/memory-repo locks and SSE stream fan-out via `xadd`/`xrange`, `redis_stream_manager.py:146`, `:493`), and object storage. Coordination is **correct only if Redis is actually configured** — with the no-op client, `acquire_conversation_lock`/`acquire_memory_repo_lock` return `None` (no mutual exclusion) and streams cannot be read across instances (`letta/data_sources/redis_client.py:593-620`). SQLite mode cannot support real multi-instance operation.

**4. What is silently dropped?**
- Redis-backed locks, OTID→run mappings, and SSE stream chunks when Redis is unconfigured or fails to initialize — degraded to no-ops with only a log line (`letta/data_sources/redis_client.py:668-680`).
- SSE stream chunks age out via TTL (~6h default) and `maxlen` trimming even when Redis is present (`redis_stream_manager.py:52`, `:146`).
- Provider-trace request/response JSON when metadata-only mode is enabled (`letta/settings.py:586`).

**5. Is durability configurable?**
Partially and heterogeneously. Configurable: DB target via `pg_uri`/`pg_*` env (`letta/settings.py:301-306`), pooling behavior (`:307-314`), object-store URI (`:327`), memfs service URL (`:343`), and provider-trace backends incl. dual-write (`factory.py:30-41`). Not cleanly configurable/declarative: the Postgres-vs-SQLite decision is inferred, not chosen (`letta/settings.py:492`), and there is no flag to *require* Redis and fail loudly instead of silently no-op'ing (`redis_client.py:668`).

## Architectural Decisions

- **Single relational DB as the durability spine.** All first-class domain state is ORM-mapped and stored in one database with a shared soft-delete + timestamp mixin (`letta/orm/base.py:12-17`), giving a uniform durability contract for agent state.
- **Postgres-first, SQLite-as-fallback.** The runtime engine is always Postgres (`letta/server/db.py:58`); SQLite only alters ORM column types/functions (`letta/orm/passage.py:35-40`, `letta/orm/sqlite_functions.py:12`). This concentrates production durability on Postgres/pgvector.
- **Migrations gate startup.** `alembic upgrade head` must succeed or the container exits (`letta/server/startup.sh:56-61`), tying version-change durability to migration correctness.
- **Redis as an optional coordination/streaming tier** with a deliberate no-op degradation (`letta/data_sources/redis_client.py:562`), prioritizing single-node runnability over fail-fast correctness.
- **Explicitly pluggable trace persistence** with dual-write for migrations (`letta/services/provider_trace_backends/factory.py:30-41`).

## Notable Patterns

- Soft delete via `is_deleted` filtered at query time rather than physical deletion (`letta/orm/sqlalchemy_base.py:394-395`).
- Module-level singleton engine + session factory (`letta/server/db.py:58-67`) with per-request async context manager and retry/rollback safety (`letta/server/db.py:73-116`).
- Dialect-branching persistence logic guarded by `settings.database_engine is DatabaseChoice.SQLITE/POSTGRES` scattered across ORM and services (`letta/orm/message.py:133`, `letta/services/agent_manager.py:3409`, etc.).
- Abstract `StorageBackend` interface enabling GCS/S3/local interchangeability for memory repos (`letta/services/memory_repo/storage/base.py:7-82`).

## Tradeoffs

- **Runnability vs. fail-fast:** silent Redis no-op and default localhost Postgres URI make a laptop start trivially, but hide the fact that locks/streaming are non-durable in that mode (`letta/data_sources/redis_client.py:669`, `letta/settings.py:478`).
- **Uniform DB storage vs. large-object cost:** storing full file text in `file_contents.text` (`letta/orm/file.py:31`) keeps everything transactional and restart-durable, at the cost of DB bloat for large documents versus offloading to object storage.
- **Implicit engine selection vs. clarity:** inferring SQLite/Postgres from env presence (`letta/settings.py:492`) reduces config surface but weakens operator ability to *confirm* which durable engine is active — worsened by the runtime always building a Postgres engine regardless (`letta/server/db.py:58`).

## Failure Modes / Edge Cases

- **Redis unavailable:** cross-instance locks silently disabled → potential concurrent conversation/memory-repo mutation; SSE resumption/fan-out returns empty (`letta/data_sources/redis_client.py:593-620`, `:641-648`).
- **Stream chunk expiry:** clients reconnecting after TTL/maxlen eviction lose earlier chunks (`redis_stream_manager.py:52`, `:146`).
- **Migration failure on deploy:** startup aborts, so a bad migration blocks the whole service rather than running against a stale schema (`letta/server/startup.sh:56-61`) — safe but a hard availability dependency.
- **SQLite mode:** no pgvector; vector search falls back to `CommonVector` path (`letta/orm/passage.py:40`) and multi-instance is unsafe — acceptable for dev, dangerous if used in prod by accident.
- **Connection retry exhaustion:** after 3 retries the session raises `LettaServiceUnavailableError` (`letta/server/db.py:110-116`), surfacing DB outages to callers rather than corrupting state.

## Future Considerations

- Add a startup readiness check / setting to *require* Redis (and the intended DB engine) and fail loudly, instead of silently instantiating `NoopAsyncRedisClient` (`letta/data_sources/redis_client.py:668`).
- Expose an explicit `LETTA_DATABASE_ENGINE` declaration and reconcile the inferred `database_engine` (`letta/settings.py:492`) with the always-Postgres runtime engine (`letta/server/db.py:58`) to remove ambiguity.
- Consider offloading large `file_contents.text` blobs (`letta/orm/file.py:31`) to the existing object-store abstraction for DB size control.
- Surface a durability/health endpoint reporting active tier for each state category (DB engine, Redis real vs no-op, object store configured) so operators can confirm production durability.

## Questions / Gaps

- **Backup/PITR strategy:** no in-repo evidence of DB backup or point-in-time recovery configuration beyond the `./.persist/pgdata` volume (`compose.yaml:14`). No clear evidence found for automated backups.
- **SQLite persistence path:** the runtime `server/db.py` always builds a Postgres engine (`letta/server/db.py:58`); where/whether a SQLite file engine is actually instantiated for a running server was not located in the selected source — SQLite appears to influence only ORM branching. No clear evidence found for a SQLite runtime engine construction.
- **Object-store backend selection:** only `LocalStorageBackend` is wired in the OSS memfs client (`memfs_client_base.py:53`); the GCS/S3 concrete implementations referenced in docs (`storage/base.py:11`) were not found as separate modules in `storage/` (only `base.py`, `local.py`). Cloud backends may live behind the `memfs_service_url` service (`letta/settings.py:343`).

---

Generated by `dimensions/02.05-persistence-durability-tiers.md` against `letta`.
