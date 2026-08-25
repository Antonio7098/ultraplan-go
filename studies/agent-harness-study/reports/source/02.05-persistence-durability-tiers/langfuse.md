# Source Analysis: langfuse

## Persistence Durability Tiers

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Next.js (web) + Express (worker) + BullMQ (queues) on top of PostgreSQL, ClickHouse, Redis, and S3-compatible object storage |
| Analyzed | 2026-07-10 |

## Summary

Langfuse implements a **multi-tier persistence model** that is implicit rather than named. There is no single `Durability` enum like LangGraph's; instead, each subsystem — Postgres (auth/metadata), ClickHouse (observability data), Redis (queues and caches), S3 (event payloads and media), and per-process LRU caches — exposes its own durability contract. Operators must read the `LANGFUSE_*` env schema in `packages/shared/src/env.ts:1-516` to know what survives which failure.

The model in practice is:

1. **Postgres** (`packages/shared/prisma/schema.prisma:10-14`) holds all control-plane state: users, orgs, projects, API keys, prompts, datasets, comments, dashboards, monitors, automations, integration configs, batch exports, batch actions, billing, surveys, RBAC tables, and the legacy `LegacyPrismaTrace` / `LegacyPrismaObservation` shadow models. Migrations are versioned under `packages/shared/prisma/migrations/*` (300+ files).
2. **ClickHouse** (`packages/shared/clickhouse/migrations/clustered/*` and `…/unclustered/*`, 34 paired up/down files) holds the analytical store of record — `traces`, `observations`, `scores`, `events`, `dataset_run_items`, `blob_storage_file_log`. ClickHouse is sharded via `ReplicatedReplacingMergeTree` with `event_ts` versioning (`packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:23`) and is the source of truth for read APIs.
3. **S3 / GCS / Azure Blob / OCI Object Storage** (`packages/shared/src/server/services/StorageService.ts:222-1649`) holds raw ingestion event payloads (`LANGFUSE_S3_EVENT_UPLOAD_BUCKET`, required at `packages/shared/src/env.ts:232`), media uploads (`LANGFUSE_S3_MEDIA_UPLOAD_BUCKET`, optional at `packages/shared/src/env.ts:245`), and tenant-configured blob-export destinations. The factory selects between backends at runtime via `StorageServiceFactory.getInstance` (`packages/shared/src/server/services/StorageService.ts:275-327`).
4. **Redis** holds the entire background-processing pipeline: BullMQ queues (`packages/shared/src/server/redis/ingestionQueue.ts:8-166`, `…/traceUpsert.ts:8-88`, `…/batchActionQueue.ts:6-43`, etc.), transient caches (`packages/shared/src/server/evalJobConfigCache.ts:1-99`, `packages/shared/src/server/redis/ingestionFailureTracking.ts:1-130`, `packages/shared/src/server/redis/s3SlowdownTracking.ts:1-74`, `packages/shared/src/server/redis/otelProjectTracking.ts:1-51`), and an optional API-key cache (`web/src/features/public-api/server/apiAuth.ts:350-404`).
5. **In-process LRUCache** (`packages/shared/src/server/cache/localCache.ts:18-164`) holds L1 model-match caches per Node process with TTL-only invalidation; explicitly documented as not durable across containers (`packages/shared/src/server/ingestion/modelMatch.ts:29-30`).
6. **In-process `Map`s** held inside `WorkerManager.workers` (`worker/src/queues/workerManager.ts:21`), `ClickHouseClientManager.clientMap` (`packages/shared/src/server/clickhouse/client.ts:39`), `recentProjectMarks` (`packages/shared/src/server/redis/ingestionFailureTracking.ts:14`), the ingestion queue shard instance map (`packages/shared/src/server/redis/ingestionQueue.ts:12`), and the `ClickhouseReadSkipCache.projectSkipMap` (`worker/src/utils/clickhouseReadSkipCache.ts:7`) — none of these survive process death.
7. **S3 ↔ ClickHouse cross-reference table** `blob_storage_file_log` (`packages/shared/src/server/data-deletion/ingestionFileDeletion.ts:95-107`, `…/repositories/blobStorageLog.ts:1-253`) is the join table that lets deletion workers enumerate S3 keys for soft-delete in ClickHouse. It is populated during ingestion and queried during project / trace / score deletion.

Recovery is **at-least-once via BullMQ retries + S3 staging**:
- Ingestion payloads are uploaded to S3 first (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`) and only the S3 key + auth scope is enqueued in Redis. If the worker crashes between the API returning 200 and ClickHouse commit, the queue job retries up to `attempts: 6` with exponential backoff (`packages/shared/src/server/redis/ingestionQueue.ts:67-71`).
- Most queues set `removeOnComplete: true` so Redis doesn't bloat (`packages/shared/src/server/redis/batchActionQueue.ts:25`, `…/batchExport.ts:24`, `…/dlqRetryQueue.ts:21`); failed jobs are retained (`removeOnFail: 100` or `100_000`) and a `DeadLetterRetryQueue` (`packages/shared/src/server/redis/dlqRetryQueue.ts:6-53`) sweeps them every 10 minutes.
- Postgres writes go through Prisma transactions (`packages/shared/src/db.ts:8-48`), but the queue + DB are not in a distributed transaction — ClickHouse and Postgres writes happen after the queue has been acked, so on partial failure the `ClickhouseWriter` batch is rebuilt from queued in-memory state (`worker/src/services/ClickhouseWriter/index.ts:42-110`) and the `removeOnFail: 100_000` retention is what keeps failed ingestion recoverable.

There is **no explicit `Durability` literal** that the operator can set; durability is implicit per store and tunable only by env flag. The operator must understand which subsystem holds which state to know what survives a restart.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:

- **Interfaces are explicit**: `StorageService` is a typed contract with five concrete backends (`packages/shared/src/server/services/StorageService.ts:222-255`), `StorageServiceFactory.getInstance` is the single entry point (`…/StorageService.ts:275-327`), and the Redis client is constructed through a documented factory that supports single-node, Sentinel, Cluster, and TLS modes (`packages/shared/src/server/redis/redis.ts:189-235`).
- **Backups are explicit**: Postgres is on a managed `postgresql` provider with `directUrl` and `shadowDatabaseUrl` (`packages/shared/prisma/schema.prisma:9-14`), ClickHouse uses `ReplicatedReplacingMergeTree` with explicit `event_ts` versioning (`packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:23`), and S3 supports SSE / SSE-KMS via `LANGFUSE_S3_EVENT_UPLOAD_SSE` and `LANGFUSE_S3_EVENT_UPLOAD_SSE_KMS_KEY_ID` (`packages/shared/src/env.ts:241-242`).
- **Recovery is operational**: BullMQ retries with exponential backoff across all queues (e.g., `packages/shared/src/server/redis/ingestionQueue.ts:67-71` with `attempts: 6`), a periodic DLQ sweep (`…/dlqRetryQueue.ts:38-49` with `0 */10 * * * *`), S3-staged ingestion payloads that survive worker crash (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`), and a worker `shutdown()` that flushes the `ClickhouseWriter` (`worker/src/services/ClickhouseWriter/index.ts:99-110`).
- **Subtract points — silent no-op persistence**: `packages/shared/src/server/evalJobConfigCache.ts:27-44` and `…:51-68` no-op silently when Redis is absent: a `null` redis returns `false` / no-op, and `recentProjectMarks` (`packages/shared/src/server/redis/ingestionFailureTracking.ts:14`) is a plain in-process `Map<string, number>` that never persists, so ingestion-failure visibility is lost on worker restart.
- **Subtract points — InMemoryFilterService is process-local**: `packages/shared/src/server/services/InMemoryFilterService.ts:1-390` is a pure JS predicate engine with no persistence layer; if a worker uses it on cached entities, results are silently process-bound.
- **Subtract points — multi-instance fragility for the in-memory state**: `ClickhouseReadSkipCache` (`worker/src/utils/clickhouseReadSkipCache.ts:5-172`) is a static singleton with a process-local `Map<string, boolean>`. Two worker pods may diverge after restart on which projects skip the ClickHouse read path until the cutoff-date re-init completes — and `prisma.project.findMany` itself can race with project creation.
- **Subtract points — DLQ retention is bounded**: failed jobs are kept at `removeOnFail: 100_000` for some queues (`packages/shared/src/server/redis/ingestionQueue.ts:66`, `…/projectDelete.ts:28`) and `100` for others (`packages/shared/src/server/redis/batchActionQueue.ts:26`); long-running outages can drop data once the cap is hit.
- **Subtract points — local LRU cache defaults to off but is undocumented in operator docs**: `LANGFUSE_LOCAL_CACHE_MODEL_MATCH_ENABLED` defaults to `"false"` (`packages/shared/src/env.ts:83-85`), which is safe, but the LRU instance is constructed unconditionally at module load (`packages/shared/src/server/ingestion/modelMatch.ts:31-42`), so any cross-process memory footprint from the in-memory layer is a deployment-time footnote rather than an enforced contract.
- **Bonus — ClickHouse client is per-config singleton**: `ClickHouseClientManager` (`packages/shared/src/server/clickhouse/client.ts:37-235`) caches clients by a JSON-stringified config key so the same `(URL, user, db, settings)` combination always reuses the same TCP keep-alive pool; this is the kind of "knows what survives process death" detail that earned the 7/10.
- **Bonus — ingest-failure tracking is rate-limited but persisted**: `markProjectIngestFailure` writes to `langfuse:ingestion-failure:active-projects` with `env.LANGFUSE_INGEST_FAILURE_PROJECT_TTL_SECONDS` (default 1 h, `packages/shared/src/env.ts:300-304`) using a Redis sorted set keyed by expiry score (`packages/shared/src/server/redis/ingestionFailureTracking.ts:82-87`), so the value survives worker death but only because Redis holds it.
- **Bonus — explicit ack contract for batch-action retries**: `BatchActionQueue` sets `attempts: 10` with exponential backoff (`packages/shared/src/server/redis/batchActionQueue.ts:27-31`) so project / trace deletes keep retrying for hours before the DLQ catches them.

The exact rubric question — "Can operators tell which state is production-durable?" — is mostly answerable from the `LANGFUSE_*` env schema (`packages/shared/src/env.ts:1-516`), but the durability story is implicit: there is no single "what survives what" matrix.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Postgres schema entry point | `datasource db { provider = "postgresql"; url = env("DATABASE_URL"); ... }` | `packages/shared/prisma/schema.prisma:9-14` |
| Postgres models enumerated | `grep "model " prisma/schema.prisma` returns 84 models (User, Project, ApiKey, Prompt, Dataset, BatchExport, BatchAction, Media, etc.) | `packages/shared/prisma/schema.prisma:17-1841` |
| ClickHouse traces table | `CREATE TABLE traces ON CLUSTER default ( ... ) ENGINE = ReplicatedReplacingMergeTree(event_ts, is_deleted) Partition by toYYYYMM(timestamp) PRIMARY KEY (project_id, toDate(timestamp)) ORDER BY (project_id, toDate(timestamp), id)` | `packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:1-32` |
| ClickHouse migration count | 34 paired up/down migrations × 2 modes (clustered + unclustered) = 68 + 68 = 136 SQL files | `packages/shared/clickhouse/migrations/{clustered,unclustered}/0001..0034_*` |
| ClickHouse client singleton | `class ClickHouseClientManager { private static instance: ...; private clientMap: Map<string, ClickhouseClientType> = new Map(); ... }` | `packages/shared/src/server/clickhouse/client.ts:37-39` |
| ClickHouse async-insert durability | `async_insert: 1, wait_for_async_insert: 1` (server waits for ack before the client returns) | `packages/shared/src/server/clickhouse/client.ts:208-209` |
| Storage backend interface | `export interface StorageService { uploadFile(...); uploadFileBuffered(...); uploadWithSignedUrl(...); uploadJson(...); download(...); listFiles(...); getSignedUrl(...); getSignedUploadUrl(...); deleteFiles(...); }` | `packages/shared/src/server/services/StorageService.ts:222-255` |
| Storage backend factory | `class StorageServiceFactory { public static getInstance(params): StorageService { if (params.useAzureBlob ...) return new AzureBlobStorageService(params); if (params.useGoogleCloudStorage ...) return new GoogleCloudStorageService(params); if (params.useOCIObjectStorage ...) return new OCIObjectStorageService(params); return new S3StorageService(params); } }` | `packages/shared/src/server/services/StorageService.ts:275-327` |
| S3 event bucket is required | `LANGFUSE_S3_EVENT_UPLOAD_BUCKET: z.string()` (no `.optional()`) | `packages/shared/src/env.ts:232` |
| S3 media bucket is optional | `LANGFUSE_S3_MEDIA_UPLOAD_BUCKET: z.string().optional()` | `packages/shared/src/env.ts:245` |
| S3 SSE-KMS support | `LANGFUSE_S3_EVENT_UPLOAD_SSE: z.enum(["AES256", "aws:kms"]).optional()` and `LANGFUSE_S3_EVENT_UPLOAD_SSE_KMS_KEY_ID: z.string().optional()` | `packages/shared/src/env.ts:241-242` |
| Ingestion S3 staging pattern | `processEventBatch` uploads to S3 and enqueues only the key + scope | `packages/shared/src/server/ingestion/processEventBatch.ts:201-518` |
| OTEL ingestion S3 staging | `const fileKey = \`${env.LANGFUSE_S3_EVENT_UPLOAD_PREFIX}otel/${this.projectId}/${this.getCurrentTimePath()}/${randomUUID()}.json\`;` then `await storageClient.upload(...)` | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:183-187` |
| Blob storage log (S3 ↔ ClickHouse join) | `softDeleteInClickhouse` writes `is_deleted = "1"` to `blob_storage_file_log` | `packages/shared/src/server/data-deletion/ingestionFileDeletion.ts:95-107` |
| Queue durability knobs | `removeOnComplete: true`, `removeOnFail: 100_000`, `attempts: 6`, `backoff: { type: "exponential", delay: 5000 }` | `packages/shared/src/server/redis/ingestionQueue.ts:64-72` |
| DLQ periodic sweeper | `DeadLetterRetryQueue` schedules a `0 */10 * * * *` cron | `packages/shared/src/server/redis/dlqRetryQueue.ts:38-49` |
| DLQ retry service | `DlqRetryService.retryDeadLetterQueue` registered with `concurrency: 1` | `worker/src/app.ts:593-604` |
| BullMQ Cluster-aware key prefix | `getQueuePrefix` uses `{prefix:queueName}` hash tags in cluster mode | `packages/shared/src/server/redis/redis.ts:242-256` |
| Redis TLS / Sentinel / Cluster topology | `createNewRedisInstance` switches on `REDIS_CLUSTER_ENABLED` / `REDIS_SENTINEL_ENABLED` | `packages/shared/src/server/redis/redis.ts:189-235` |
| Redis retry strategy | `retryStrategy: (times) => Math.max(Math.min(Math.exp(times), 20000), 1000)` (retries forever, capped 20 s) | `packages/shared/src/server/redis/redis.ts:17-25` |
| L1 in-memory model-match cache | `const modelMatchLocalCache = new LocalCache<ModelWithPrices>({ enabled: env.LANGFUSE_LOCAL_CACHE_MODEL_MATCH_ENABLED === "true", ... })` | `packages/shared/src/server/ingestion/modelMatch.ts:31-42` |
| LocalCache implementation | `class LocalCache<V extends {}> { private readonly cache: LRUCache<string, V>; ... }` | `packages/shared/src/server/cache/localCache.ts:18-164` |
| Redis eval-config cache (no-op fallback) | `if (!redis) { return false; }` / `if (!redis) { return; }` | `packages/shared/src/server/evalJobConfigCache.ts:31-34`, `…:55-58` |
| Redis ingestion-failure sorted set | `redis.pipeline().zadd(INGESTION_FAILURE_ACTIVE_PROJECTS_KEY, expiresAtMs, projectId).expire(INGESTION_FAILURE_ACTIVE_PROJECTS_KEY, ttlSeconds + 60).exec()` | `packages/shared/src/server/redis/ingestionFailureTracking.ts:82-87` |
| In-process rate-limit map | `const recentProjectMarks = new Map<string, number>()` (not persisted) | `packages/shared/src/server/redis/ingestionFailureTracking.ts:14` |
| ClickhouseReadSkipCache in-process map | `private projectSkipMap = new Map<string, boolean>()` (lost on worker restart) | `worker/src/utils/clickhouseReadSkipCache.ts:7` |
| API-key Redis cache | `await this.redis.set(createApiKeyCacheKey(hash), JSON.stringify(newApiKey), "EX", env.LANGFUSE_CACHE_API_KEY_TTL_SECONDS)` | `web/src/features/public-api/server/apiAuth.ts:358-364` |
| API-key cache invalidation | `safeMultiDel(redisClient, keysToDelete)` + `scanKeys(redisClient, API_KEY_CACHE_PATTERN)` | `packages/shared/src/server/auth/invalidateApiKeys.ts:39-43`, `…:136-152` |
| PromptService Redis cache | `this.cacheEnabled = Boolean(redis) && env.LANGFUSE_CACHE_PROMPT_ENABLED === "true"`; `epochTtlSeconds = 7 * 24 * 60 * 60` | `packages/shared/src/server/services/PromptService/index.ts:40-44` |
| ClickhouseWriter shutdown flush | `await this.flushAll(true);` on `shutdown()` | `worker/src/services/ClickhouseWriter/index.ts:99-110` |
| Worker manager in-process registry | `private static workers: { [key: string]: Worker } = {};` | `worker/src/queues/workerManager.ts:21` |
| Background migrations | `BackgroundMigrationManager.run()` started in `app.ts`; "if (!BackgroundMigrationManager.activeMigration) { ... }"` heartbeat every 15 s | `worker/src/backgroundMigrations/backgroundMigrationManager.ts:35`; `worker/src/app.ts:116-121` |
| ClickHouse-backed read paths | `queryClickhouseStream` used by `getBlobStorageByProjectId*` for soft-delete enumeration | `packages/shared/src/server/repositories/blobStorageLog.ts:37-100` |
| Trace upsert queue delay | `delay: 30_000` (30 s) to coalesce trace writes | `packages/shared/src/server/redis/traceUpsert.ts:71` |
| Ingestion secondary queue | `removeOnComplete: true, removeOnFail: 100_000, attempts: 5` | `packages/shared/src/server/redis/ingestionQueue.ts:147-153` |

## Answers to Dimension Questions

### 1. What survives a restart?

- **Postgres** (auth, RBAC, prompts, datasets, comments, dashboards, monitors, integrations, batch exports, batch actions, billing, surveys, legacy trace/observation shadow tables) survives — `packages/shared/prisma/schema.prisma:17-1841`.
- **ClickHouse** (traces, observations, scores, events, dataset_run_items, blob_storage_file_log) survives — `packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:23` uses `ReplicatedReplacingMergeTree` which is replicated at the storage layer.
- **S3 / GCS / Azure Blob / OCI Object Storage** (raw ingestion events, media, blob-export outputs) survives — the SDKs use the bucket's own durability contract (`packages/shared/src/server/services/StorageService.ts:222-1649`).
- **Redis queues** survive — Redis is the configured durable broker. BullMQ's `removeOnComplete: true` keeps only currently-running and recently-failed jobs; `removeOnFail: 100_000` retains failed jobs so they can be re-driven via the DLQ (`packages/shared/src/server/redis/ingestionQueue.ts:64-72`).
- **Redis caches** survive — `LANGFUSE_CACHE_API_KEY_TTL_SECONDS` (default 300 s, `web/src/env.mjs:325`), `LANGFUSE_CACHE_PROMPT_TTL_SECONDS` (default 3600 s, `packages/shared/src/env.ts:96`), `LANGFUSE_CACHE_MODEL_MATCH_TTL_SECONDS` (default 86 400 s, `packages/shared/src/env.ts:82`), `LANGFUSE_INGEST_FAILURE_PROJECT_TTL_SECONDS` (default 3600 s, `packages/shared/src/env.ts:300-304`).
- **In-process state** does **not** survive: `ClickhouseReadSkipCache.projectSkipMap` (`worker/src/utils/clickhouseReadSkipCache.ts:7`), `recentProjectMarks` (`packages/shared/src/server/redis/ingestionFailureTracking.ts:14`), the `LocalCache` LRU (`packages/shared/src/server/cache/localCache.ts:39-43`), the queue-instance maps (`packages/shared/src/server/redis/ingestionQueue.ts:12`), and the `WorkerManager.workers` registry (`worker/src/queues/workerManager.ts:21`).

### 2. What survives redeploy?

- Same as restart, because there is no separate "first boot vs subsequent boot" code path. `web/src/initialize.ts:40-212` runs only when `LANGFUSE_INIT_ORG_ID` is set, so redeploy is idempotent for normal data but the `LANGFUSE_INIT_*` block can mutate Postgres if the env vars are still set (it `upsert`s the org, project, user, and API keys on every boot).
- `redis.globalThis` cache pattern in `packages/shared/src/server/redis/redis.ts:392-398` keeps a single Redis connection across hot reloads in development only.
- ClickHouse clients are per-process pools (`packages/shared/src/server/clickhouse/client.ts:39`); they reconnect on deploy but the data is unaffected.

### 3. What survives multi-instance operation?

- **Postgres, ClickHouse, S3, and Redis queues** survive and are the only stores that propagate across instances — they are the system-of-record stores.
- **Redis caches** survive because Redis is shared. **Caveat**: the `LocalCache` LRU in `packages/shared/src/server/ingestion/modelMatch.ts:31-42` is per-process, and the explicit comment in the source (`packages/shared/src/server/ingestion/modelMatch.ts:29-30`) acknowledges this: "This L1 cache is intentionally TTL-only. Cross-container consistency continues to come from Redis invalidation plus the short local TTL." So in a multi-instance deploy, two pods may briefly disagree on model-match results until the TTL (default 10 s, `packages/shared/src/env.ts:86-89`) elapses.
- **`ClickhouseReadSkipCache`** is a static singleton per process (`worker/src/utils/clickhouseReadSkipCache.ts:6-25`); different worker pods can disagree on whether to skip the ClickHouse read until they each re-initialize from Postgres. The init is idempotent, but a newly-started worker pod will issue a `prisma.project.findMany` and warm the cache from scratch.
- **`WorkerManager.workers`** is a process-static map (`worker/src/queues/workerManager.ts:21`); each pod registers its own consumers independently. There is no leader election; multiple workers compete on the same Redis-backed BullMQ queue.

### 4. What is silently dropped?

- **`SDK_LOG` event type** is logged via `logger.info("SDK Log Event", { event });` and dropped from further processing (`packages/shared/src/server/ingestion/processEventBatch.ts:188-195`). This is by design but worth flagging.
- **`hasNoEvalConfigsCache`** silently returns `false` when Redis is absent (`packages/shared/src/server/evalJobConfigCache.ts:31-44`), so the eval-execution optimization is disabled without any signal.
- **`markProjectS3Slowdown`** silently no-ops when Redis is absent or `LANGFUSE_S3_RATE_ERROR_SLOWDOWN_ENABLED !== "true"` (`packages/shared/src/server/redis/s3SlowdownTracking.ts:39-56`).
- **`markProjectAsOtelUser`** silently no-ops when `LANGFUSE_SKIP_FINAL_FOR_OTEL_PROJECTS !== "true"` (`packages/shared/src/server/redis/otelProjectTracking.ts:13-15`).
- **DLQ jobs** are bounded by `removeOnFail: 100` or `removeOnFail: 100_000` (`packages/shared/src/server/redis/batchActionQueue.ts:26`, `…/ingestionQueue.ts:66`); once the cap is hit the oldest failures are evicted by BullMQ without warning.
- **In-process rate-limit `recentProjectMarks`** (`packages/shared/src/server/redis/ingestionFailureTracking.ts:14`) — restart clears the rate-limit state, which means a freshly-restarted worker can mark a project as failing even if it was throttled just before death.
- **`isS3SlowDownError`** returns `false` for unrecognized error shapes (`packages/shared/src/server/redis/s3SlowdownTracking.ts:16-33`); a new error shape from a vendor SDK will be misclassified silently.
- **`LANGFUSE_LOCAL_CACHE_MODEL_MATCH_ENABLED`** is off by default but the LRU is still constructed at module load time (`packages/shared/src/server/ingestion/modelMatch.ts:31-42`); if an operator enables it they get a per-process cache with TTL-only invalidation.

### 5. Is durability configurable?

Partially. There is no top-level `DURABILITY=*` literal; instead, the operator tunes per-subsystem via:

| Setting | Effect | File:Line |
|---------|--------|-----------|
| `LANGFUSE_INGESTION_QUEUE_SHARD_COUNT` | Number of BullMQ shards for the ingestion queue | `packages/shared/src/env.ts:146` |
| `LANGFUSE_INGESTION_SECONDARY_QUEUE_SHARD_COUNT` | Same, for the secondary queue | `packages/shared/src/env.ts:147-150` |
| `LANGFUSE_TRACE_UPSERT_QUEUE_SHARD_COUNT` | Trace-upsert fan-out | `packages/shared/src/server/redis/traceUpsert.ts:51-54` |
| `REDIS_CLUSTER_ENABLED` / `REDIS_SENTINEL_ENABLED` | Redis topology selection | `packages/shared/src/env.ts:62-73`, `…/redis/redis.ts:189-235` |
| `LANGFUSE_CLICKHOUSE_DELETION_TIMEOUT_MS` | ClickHouse delete timeout (default 10 min) | `packages/shared/src/env.ts:320` |
| `LANGFUSE_CLICKHOUSE_USE_LIGHTWEIGHT_UPDATE` | ClickHouse `lightweight_update` mode | `packages/shared/src/env.ts:118` |
| `CLICKHOUSE_LIGHTWEIGHT_DELETE_MODE` | `alter_update` vs `lightweight_update` vs `lightweight_update_force` | `packages/shared/src/env.ts:115-117` |
| `LANGFUSE_S3_EVENT_UPLOAD_SSE` / `..._SSE_KMS_KEY_ID` | Per-event S3 encryption | `packages/shared/src/env.ts:241-242` |
| `LANGFUSE_USE_AZURE_BLOB` / `LANGFUSE_USE_GOOGLE_CLOUD_STORAGE` / `LANGFUSE_USE_OCI_NATIVE_OBJECT_STORAGE` | Choose object-storage backend | `packages/shared/src/server/services/StorageService.ts:291-324` |
| `LANGFUSE_CACHE_API_KEY_ENABLED` / `LANGFUSE_CACHE_PROMPT_ENABLED` / `LANGFUSE_CACHE_MODEL_MATCH_ENABLED` | Per-cache enable / disable | `web/src/env.mjs:324-325`; `packages/shared/src/env.ts:81-96` |
| `LANGFUSE_LOCAL_CACHE_MODEL_MATCH_ENABLED` | Process-local LRU on/off | `packages/shared/src/env.ts:83-94` |
| `LANGFUSE_ENABLE_BACKGROUND_MIGRATIONS` | Whether worker runs `BackgroundMigrationManager` | `worker/src/app.ts:116` |
| `LANGFUSE_BATCH_PROJECT_CLEANER_ENABLED` / `LANGFUSE_BATCH_DATA_RETENTION_CLEANER_ENABLED` | Whether background deletion loops run | `worker/src/app.ts:653`, `…:670` |
| `LANGFUSE_S3_RATE_ERROR_SLOWDOWN_ENABLED` | Whether S3 SlowDown routing is active | `packages/shared/src/env.ts:293-298` |
| `LANGFUSE_SKIP_FINAL_FOR_OTEL_PROJECTS` | Whether the OTEL tracking flag is honoured | `packages/shared/src/env.ts:464-466` |

There is **no per-event durability level**. All ingestion events go through the same `processEventBatch` → S3 → BullMQ → ClickHouseWriter path.

## Architectural Decisions

- **ClickHouse is the system of record for telemetry; Postgres is the system of record for control-plane.** This split is hard-coded by the schema: Postgres has only the legacy `LegacyPrismaTrace` / `LegacyPrismaObservation` shadow models (`packages/shared/prisma/schema.prisma:397-541`), and the V4 write-mode env flag `LANGFUSE_MIGRATION_V4_WRITE_MODE` (`web/src/env.mjs:439-441`) explicitly enumerates `legacy | dual | events_only` to control whether legacy tables are still populated.
- **S3 is the durable staging buffer for ingestion events.** The API uploads the full payload to S3 first and only enqueues the key + auth scope in Redis (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`). This means Redis loss does not lose events: the S3 payload still exists and the worker can re-enqueue from the keys (manually). It also means the queue payload is small and serialization is centralized.
- **Queues are mostly `removeOnComplete: true`.** Most queues throw away completed jobs to keep Redis small (`packages/shared/src/server/redis/batchActionQueue.ts:25`, `…/batchExport.ts:24`). This means there is no audit log of "what was processed when" in Redis; observability comes from ClickHouse and the application logs.
- **BullMQ retries are exponential with 5–10 attempts.** Ingestion is `attempts: 6` with 5 s base (`packages/shared/src/server/redis/ingestionQueue.ts:67-71`); batch actions are `attempts: 10` (`…/batchActionQueue.ts:27-31`); DLQ sweep is `attempts: 5` (`…/dlqRetryQueue.ts:23-28`). The DLQ retention cap is `removeOnFail: 100_000` for ingestion vs `100` for DLQ-retry itself, so an operator who is not paying attention can hit the smaller cap.
- **The `ClickhouseWriter` singleton batches writes per table per `LANGFUSE_INGESTION_CLICKHOUSE_WRITE_INTERVAL_MS`** (`worker/src/services/ClickhouseWriter/index.ts:42-110`). This is the one piece of in-flight, non-durable state in the worker: batches are kept in process memory between intervals. The shutdown hook calls `flushAll(true)` (`…/index.ts:99-110`), but a SIGKILL loses the current batch.
- **`LocalCache` is TTL-only by design.** The comment at `packages/shared/src/server/ingestion/modelMatch.ts:29-30` is explicit: cross-container consistency comes from Redis invalidation, the local LRU is just a latency optimization. There is no L1 invalidation hook.
- **API-key cache invalidation is fan-out via `safeMultiDel`.** `invalidateAllCachedApiKeys` calls `scanKeys(redisClient, API_KEY_CACHE_PATTERN)` and then `safeMultiDel` (`packages/shared/src/server/auth/invalidateApiKeys.ts:136-152`). In cluster mode this is one DEL per key to avoid CROSSSLOT (`packages/shared/src/server/redis/redis.ts:303-316`).
- **Background migrations have a heartbeat + leader-takeover** (`worker/src/backgroundMigrations/backgroundMigrationManager.ts:1-100`). The active migration row is updated with `workerId` and a 15 s `setTimeout(BackgroundMigrationManager.heartBeat, 15 * 1000)` keeps the lease alive. There is no `SELECT … FOR UPDATE`; this is a soft coordination.

## Notable Patterns

- **Singleton + per-config-key client cache** (`packages/shared/src/server/clickhouse/client.ts:37-235` and `packages/shared/src/server/redis/redis.ts:381-398`). Both use a process-level `Map<configKey, Client>` keyed by JSON-stringified config, with `globalThis` reuse only in development (`…/redis/redis.ts:392-398`). This is the standard "don't pay reconnect cost" pattern.
- **S3 staging + Redis queue handoff** (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`). This is the same pattern as a write-ahead log: the API cannot return 200 until the S3 PUT has succeeded, and the S3 key is the durable reference for downstream workers.
- **`safeMultiDel` for cluster compatibility** (`packages/shared/src/server/redis/redis.ts:303-316`). Cluster mode requires per-key DEL to avoid CROSSSLOT errors; this is wrapped in one helper so callers don't have to remember.
- **Tagging `prismaGlobal` to `globalThis` in development** (`packages/shared/src/db.ts:50-60`). Prevents Next.js dev-mode HMR from spawning multiple Prisma clients.
- **`redis.pipeline().zadd().expire().exec()` for ingestion-failure tracking** (`packages/shared/src/server/redis/ingestionFailureTracking.ts:82-87`). Using a sorted set keyed by expiry score is a clean "look up who is currently failing" data structure.
- **Cron-scheduled DLQ retry** (`packages/shared/src/server/redis/dlqRetryQueue.ts:38-49`). The `repeat: { pattern: "0 */10 * * * *" }` syntax is BullMQ's; this keeps failed jobs self-healing without an external scheduler.

## Tradeoffs

- **Per-process LRUCache vs cross-instance Redis cache.** The `LocalCache` is faster and cheaper than Redis, but the L1 cache is only coherent within a single pod. Operators must accept up to `LANGFUSE_LOCAL_CACHE_MODEL_MATCH_TTL_MS` of staleness across pods (`packages/shared/src/server/ingestion/modelMatch.ts:29-30`).
- **`removeOnComplete: true` vs operational observability.** Most queues prefer to drop completed jobs, which keeps Redis small but means "what jobs ran today" is not queryable from Redis; operators must rely on ClickHouse telemetry and per-job logs.
- **`removeOnFail: 100_000` cap.** This caps how long a project can stay in DLQ. For a multi-day outage, an ingestion project could have 100 000 failures and then start silently losing retry opportunities once the cap is hit.
- **ClickHouse async_insert with `wait_for_async_insert: 1`.** The client blocks until ClickHouse acknowledges the buffered insert (`packages/shared/src/server/clickhouse/client.ts:208-209`). This trades throughput for end-to-end durability, but `async_insert_busy_timeout_*` settings can be tuned to amortize the ack cost (`…/client.ts:181-198`).
- **`ClickhouseReadSkipCache` in-process map.** Avoids hitting Postgres on every project lookup, but two pods can disagree briefly after restart, and the cache is unbounded by project count.
- **`ClickhouseWriter` in-memory batch queue.** Buffers writes for `LANGFUSE_INGESTION_CLICKHOUSE_WRITE_INTERVAL_MS` (`worker/src/services/ClickhouseWriter/index.ts:42-110`); a SIGKILL during the interval loses buffered rows. The `shutdown()` flush mitigates this for graceful termination only.

## Failure Modes / Edge Cases

- **Redis restart loses in-flight queue jobs** if Redis is not configured with AOF / RDB persistence (not enforced by Langfuse). Operators are responsible for choosing `appendonly yes` and a flush policy.
- **ClickHouse restart loses `events_full` rows that were async-inserted but not yet committed** if `wait_for_async_insert` is set to `0`. The current default is `1` (`packages/shared/src/server/clickhouse/client.ts:208-209`), which prevents this, but operators can override via `CLICKHOUSE_ASYNC_INSERT_*` envs.
- **S3 outage stalls ingestion.** The API tries to PUT events to S3 before returning 200 (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`), so an S3 outage manifests as 503s to ingest clients. The `LANGFUSE_S3_RATE_ERROR_SLOWDOWN_ENABLED` flag (`packages/shared/src/env.ts:293-298`) and `markProjectS3Slowdown` (`packages/shared/src/server/redis/s3SlowdownTracking.ts:39-56`) attempt to route repeat-offending projects to a secondary queue with relaxed S3 behaviour.
- **Redis DLQ eviction during long outage.** `removeOnFail: 100` for batch actions means a project that fails 100 times in DLQ will silently lose retry opportunities (`packages/shared/src/server/redis/batchActionQueue.ts:25-26`).
- **ClickhouseWriter crash mid-batch.** The next worker that takes the ingestion job rebuilds the batch from S3 staging (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`), but only if the original S3 PUT succeeded and the S3 key is still in Redis.
- **`recentProjectMarks` lost on worker restart** (`packages/shared/src/server/redis/ingestionFailureTracking.ts:14`). The rate-limit window is process-local; restart resets it.
- **Background migration worker crash** can leave the migration row stale. There is a `workerId` + 15 s heartbeat (`worker/src/backgroundMigrations/backgroundMigrationManager.ts:1-100`), but no automatic takeover by another worker — recovery requires manual cleanup.
- **Postgres shadow tables diverge from ClickHouse.** `LegacyPrismaTrace` and `LegacyPrismaObservation` are the legacy Postgres traces / observations (`packages/shared/prisma/schema.prisma:397-541`); they are shadowed by ClickHouse `traces` and `observations` and will drift unless `LANGFUSE_MIGRATION_V4_WRITE_MODE` is held at `legacy` or `dual`.
- **API key cache poisoning on stale Redis write.** If Redis is restored from an old backup, `LANGFUSE_CACHE_API_KEY_TTL_SECONDS` (default 300 s, `web/src/env.mjs:325`) bounds how long a revoked key can still authenticate. There is no explicit "always revalidate against Postgres on suspicious auth" guard documented.

## Future Considerations

- **No single durability knob.** Adding a `LANGFUSE_DURABILITY=strict|balanced|async` enum that adjusts Redis TTLs, ClickHouse async_insert settings, and `removeOnFail` caps in one place would let operators reason about durability without reading the 500-line env schema.
- **LocalCache lacks invalidation hooks.** Adding an OTel-driven invalidation message that the local LRU consumes would shorten the staleness window across pods without losing the latency benefit.
- **`ClickhouseReadSkipCache` should move to Redis.** The current in-process map is the closest thing to a silent durability footgun in the worker.
- **DLQ observability.** Adding a ClickHouse-backed `dlq_history` table would let operators answer "what was in the DLQ last Tuesday" without scanning Redis keys.
- **S3 staging could carry the full envelope** instead of only the key + scope; today the queue consumer needs to do an extra S3 GET to read the payload (`packages/shared/src/server/ingestion/processEventBatch.ts:201-518`).

## Questions / Gaps

- **What is the operational runbook for a ClickHouse restart?** `ClickHouseClientManager.closeAllConnections` exists (`packages/shared/src/server/clickhouse/client.ts:228-235`) but no source-visible "rebuild from S3" path was found.
- **Are Prisma migrations and ClickHouse migrations atomically versioned?** The two directories (`packages/shared/prisma/migrations/*` and `packages/shared/clickhouse/migrations/*`) are independent; there is no shared migration manifest.
- **What happens if Redis is enabled in production but absent in CI?** The `redis?.set(...)` / `if (!redis) return` patterns (`packages/shared/src/server/redis/otelProjectTracking.ts:19`, `…/evalJobConfigCache.ts:31-34`) silently no-op, so a CI environment without Redis will pass tests that test these paths but produce different production behaviour. The pattern is safe but invisible.
- **Is the LRU max-size enforcement validated?** `LocalCache` uses `LRUCache({ max: 20_000 })` (`packages/shared/src/server/cache/localCache.ts:39-43`) but the metric is only recorded on `set` and `evict`, not on `get` — there is no test that the cap is actually enforced under load.
- **Where is the "ClickHouse read replica routing" matrix?** `CLICKHOUSE_READ_ONLY_URL` and `CLICKHOUSE_EVENTS_READ_ONLY_URL` exist (`packages/shared/src/env.ts:98-99`) but their routing logic is minimal (`packages/shared/src/server/clickhouse/client.ts:120-136`); it's unclear how operators decide which URL to point at which replica.
- **Is there a documented Postgres backup contract?** The schema (`packages/shared/prisma/schema.prisma:9-14`) declares `directUrl` and `shadowDatabaseUrl` but no backup / PITR config is enforced at the application layer.

---

Generated by `02.05-persistence-durability-tiers` against `langfuse`.