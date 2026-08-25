# Source Analysis: langfuse

## 02.09 - State Pruning, Compaction, and Retention

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript (Next.js / tRPC / BullMQ) + ClickHouse + PostgreSQL + Redis + S3 |
| Analyzed | 2026-07-11 |

## Summary

Langfuse is an LLM observability platform, not a conversational agent harness.
There is **no AI-message summarization or conversation compaction** to analyze;
the platform only stores telemetry it receives. The dimension reduces to:

1. A project-scoped, nullable `retentionDays` setting on `Project`
   (`packages/shared/prisma/schema.prisma:126`) that drives a BullMQ-driven,
   per-project ClickHouse/Postgres/S3 delete pipeline.
2. A scheduled delete cascade for projects, traces, scores, datasets, media
   files, and ingestion-event blobs.
3. An opt-in "batch" deletion worker
   (`LANGFUSE_BATCH_DATA_RETENTION_CLEANER_ENABLED=true`,
   `worker/src/features/batch-data-retention-cleaner/index.ts:113`) that
   bulk-DELETEs expired data across many projects in a single ClickHouse
   statement.
4. A ClickHouse `APPLY DELETED MASK` worker that physically removes the
   lightweight-delete tombstones from old monthly partitions
   (`worker/src/features/deleted-mask-cleaner/index.ts:42`).

The model is mature, explicit, and well-tested (the retention/cleaner
features have ~50 unit + integration tests). The model is also deliberately
opt-in: when `retentionDays` is null or `0`, **nothing is ever pruned and
data grows without bound by default**.

## Rating

**Score: 9 / 10**

Rationale:

- Explicit retention contract on `Project.retentionDays` with validation,
  entitlement gating, and audit logging
  (`web/src/features/projects/server/projectsRouter.ts:130-165`,
  `web/src/ee/features/admin-api/server/projects/projectById/index.ts:82-99`).
- Two complementary mechanisms: per-project BullMQ jobs
  (`worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:17`) and a
  batch-data-retention cleaner with chunked, hash-keyed, sorted-by-expired-data
  SELECT + single-query DELETE
  (`worker/src/features/batch-data-retention-cleaner/index.ts:152-399`).
- Comprehensive table coverage: `traces`, `observations`, `scores`,
  `events_full`, `events_core`, `dataset_run_items_rmt`, `media`, S3 event
  blobs, media uploads (`worker/src/features/batch-data-retention-cleaner/index.ts:18-24`,
  `worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:84-103`).
- Redis-locked PeriodicExclusiveRunner pattern
  (`worker/src/utils/PeriodicExclusiveRunner.ts`) so multiple worker
  instances don't double-delete.
- Defensive job behavior: re-fetches retention value before executing
  (`worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:28-39`)
  so a queued job cannot delete after retention was disabled.
- Pre-flight count + min/max timestamp gating before any DELETE
  (`packages/shared/src/server/repositories/traces.ts:966-1032`,
  `packages/shared/src/server/repositories/observations.ts:1257-1318`).
- DeletionGuard with project skip-list
  (`packages/shared/src/server/deletionGuard.ts:5-47`).
- Metrics + structured logs for every cleaner
  (`recordIncrement("langfuse.batch_data_retention_cleaner.*", ...)`,
  `worker/src/features/batch-data-retention-cleaner/index.ts:220-229`).
- Dedicated DeletedMaskCleaner for ClickHouse LIGHTWEIGHT delete compaction
  (`worker/src/features/deleted-mask-cleaner/index.ts:42-263`) gated on
  `LANGFUSE_CLICKHOUSE_DELETED_MASK_CLEANER_ENABLED`.
- Bulk delete tests on CH use `request_timeout`
  `LANGFUSE_CLICKHOUSE_DELETION_TIMEOUT_MS` default 10 min
  (`packages/shared/src/env.ts:320`) so a stuck delete doesn't hang
  indefinitely.

The model loses points on the following:

- Default behavior is unbounded retention; no system-wide hard cap.
- The "fast" batch-data-retention cleaner and the per-project BullMQ path
  are **both** feature-flagged off by default
  (`LANGFUSE_BATCH_DATA_RETENTION_CLEANER_ENABLED`, default `"false"` in
  `worker/src/env.ts:400-402`); the daily cron-based per-project path is the
  only one that runs out of the box
  (`LANGFUSE_DATA_RETENTION_QUEUE_IS_ENABLED`, default `"true"` in
  `worker/src/env.ts:274-276`).
- There is no `OPTLMIZE FINAL` schedule and no PARTITION-level drop path for
  user-data tables in the migrations `0001_traces.up.sql` /
  `0002_observations.up.sql` / `0003_scores.up.sql`; the only TTL on a
  primary table is on the `observations_batch_staging` staging area
  (`packages/shared/clickhouse/scripts/dev-tables.sh:152`) and the
  short-lived aggregating-mergetree variants
  (`packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:174`).
- Trace deletion uses a soft `pending_deletions` table that is cleaned up
  only when the CH delete succeeds; if CH delete repeatedly fails, the
  `pending_deletions` table grows without bound
  (`packages/shared/src/server/traceDeletionProcessor.ts:55-86`,
  `worker/src/queues/traceDelete.ts:105-119`).
- "Compaction" (the AMT `traces_*_amt`) is purely analytical rollups with
  TTL on the rollups themselves (7d / 30d), not compaction of the base data.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Schema: project retention column | `Project.retentionDays Int?` | `packages/shared/prisma/schema.prisma:126` |
| `pending_deletions` model (trace-by-trace soft delete queue) | `PendingDeletion` model | `packages/shared/prisma/schema.prisma:1788-1803` |
| Retention value validation (0 or >=3) | `projectRetentionSchema` | `web/src/features/auth/lib/projectRetentionSchema.ts:3-10` |
| tRPC `setRetention` mutation with entitlement check | tRPC procedure that writes `retentionDays` | `web/src/features/projects/server/projectsRouter.ts:130-165` |
| Admin REST API: set retention + audit log | `handleUpdateProject` | `web/src/ee/features/admin-api/server/projects/projectById/index.ts:82-112` |
| Admin REST API: soft-delete then enqueue `ProjectDelete` | `handleDeleteProject` | `web/src/ee/features/admin-api/server/projects/projectById/index.ts:115-185` |
| UI: retention configuration entrypoint | `ConfigureRetention` form | `web/src/features/projects/components/ConfigureRetention.tsx:1-111` |
| Scheduled daily cron (3:15am UTC) for retention | BullMQ `repeat: { pattern: "15 3 * * *" }` | `packages/shared/src/server/redis/dataRetentionQueue.ts:43` |
| Schedules one job per project with retentionDays > 0 | `addBulk` for `DataRetentionProcessingJob` | `worker/src/ee/dataRetention/handleDataRetentionSchedule.ts:8-40` |
| Per-project retention handler: re-reads DB, deletes CH + media + S3 | Bulk delete of traces/observations/scores/events + media + blob refs | `worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:17-104` |
| ClickHouse trace deletion with timestamp pre-flight | `deleteTraces` | `packages/shared/src/server/repositories/traces.ts:966-1032` |
| ClickHouse trace deletion by retention cutoff | `deleteTracesOlderThanDays` | `packages/shared/src/server/repositories/traces.ts:1063-1105` |
| ClickHouse trace deletion by project id | `deleteTracesByProjectId` | `packages/shared/src/server/repositories/traces.ts:1107-1147` |
| ClickHouse observation delete by trace id (with pre-flight) | `deleteObservationsByTraceIds` | `packages/shared/src/server/repositories/observations.ts:1257-1318` |
| ClickHouse observation delete by project | `deleteObservationsByProjectId` | `packages/shared/src/server/repositories/observations.ts:1342-1375` |
| ClickHouse observation delete by retention cutoff | `deleteObservationsOlderThanDays` | `packages/shared/src/server/repositories/observations.ts:1402-1434` |
| ClickHouse score delete (by id, by traceIds, by projectId, by cutoff) | 4 entry points | `packages/shared/src/server/repositories/scores.ts:1652-1830` |
| ClickHouse events delete (by traceId, by projectId, by cutoff) | cascades through `events_full` + `events_core` | `packages/shared/src/server/repositories/events.ts:2853-3091` |
| ClickHouse dataset_run_items delete (by projectId, datasetId, datasetRunIds) | dedicated repo | `packages/shared/src/server/repositories/dataset-run-items.ts:1125-1218` |
| Delete timeout for any CH DELETE | `LANGFUSE_CLICKHOUSE_DELETION_TIMEOUT_MS` default 600_000 | `packages/shared/src/env.ts:320` |
| Batch-data-retention cleaner: config table list | `BATCH_DATA_RETENTION_TABLES` | `worker/src/features/batch-data-retention-cleaner/index.ts:18-24` |
| Batch-data-retention cleaner: per-table, Redis-locked periodic runner | `BatchDataRetentionCleaner` | `worker/src/features/batch-data-retention-cleaner/index.ts:113-399` |
| Hash-keyed project params (avoids index-mismatch bugs) | `buildRetentionConditions` | `worker/src/features/batch-data-retention-cleaner/index.ts:62-96` |
| Single bulk DELETE per run with OR conditions | `executeBatchDelete` | `worker/src/features/batch-data-retention-cleaner/index.ts:371-399` |
| Media retention cleanup (S3 + Postgres `media` + blob refs) | `MediaRetentionCleaner` | `worker/src/features/media-retention-cleaner/index.ts:35-209` |
| Reusable S3 + Postgres media deletion helper | `deleteMediaFiles` | `packages/shared/src/server/media-deletion.ts:73-116` |
| Trace deletion request → Postgres `pending_deletions` + BullMQ enqueue | `traceDeletionProcessor` | `packages/shared/src/server/traceDeletionProcessor.ts:26-94` |
| Delay between request and actual delete | `LANGFUSE_TRACE_DELETE_DELAY_MS` (default 5000ms) | `packages/shared/src/env.ts:196-199`, `packages/shared/src/server/traceDeletionProcessor.ts:31` |
| Batch deletion size env | `LANGFUSE_DELETE_BATCH_SIZE` referenced in traceDelete | `worker/src/queues/traceDelete.ts:75` |
| Trace deletion worker merges pending + queued events | traceDeleteProcessor | `worker/src/queues/traceDelete.ts:15-131` |
| PG state cleanup for trace deletions | `processPostgresTraceDelete` | `worker/src/features/traces/processPostgresTraceDelete.ts` |
| Media cleanup for trace deletions | `deleteMediaItemsForTraces` | `worker/src/features/traces/processClickhouseTraceDelete.ts:15-124` |
| Score deletion queue handler | scoreDeleteProcessor | `worker/src/queues/scoreDelete.ts:10-20` |
| Dataset deletion queue handler | datasetDeleteProcessor | `worker/src/queues/datasetDelete.ts:5-9` |
| Project deletion queue handler | projectDeleteProcessor | `worker/src/queues/projectDelete.ts:22-119` |
| Skip-list guard for deletion processors | `shouldSkipDeletionFor` | `packages/shared/src/server/deletionGuard.ts:5-47` |
| `LANGFUSE_DELETE_SKIP_PROJECT_IDS` (comma-separated opt-out) | schema + consumer | `packages/shared/src/env.ts:200-203`, `packages/shared/src/server/deletionGuard.ts:11` |
| Batch project cleaner (soft-deleted projects) | `BatchProjectCleaner` | `worker/src/features/batch-project-cleaner/index.ts:45-254` |
| Batch project media cleaner (S3 safety net) | `BatchProjectMediaCleaner` | `worker/src/features/batch-project-media-cleaner/index.ts:25-126` |
| Batch project blob cleaner (events S3 safety net) | `BatchProjectBlobCleaner` | `worker/src/features/batch-project-blob-cleaner/index.ts` |
| Batch trace deletion cleaner (drains pending backlog) | `BatchTraceDeletionCleaner` | `worker/src/features/batch-trace-deletion-cleaner/index.ts:32-186` |
| DELETE-mask cleaner (ClickHouse mutation compaction) | `DeletedMaskCleaner` + helpers | `worker/src/features/deleted-mask-cleaner/index.ts:42-263`, `helpers.ts:1-198` |
| S3 ingestion event deletion (soft delete in CH + hard in S3) | `removeIngestionEventsFromS3AndDeleteClickhouseRefs` | `packages/shared/src/server/data-deletion/ingestionFileDeletion.ts:56-108` |
| Streamed query over `blob_storage_file_log` for retention pruning | `getBlobStorageByProjectIdBeforeDate` | `packages/shared/src/server/repositories/blobStorageLog.ts:59-82` |
| Hard-coded TTL on the event-propagation staging table (CH) | `TTL s3_first_seen_timestamp + INTERVAL 48 HOUR SETTINGS ttl_only_drop_parts = 1` | `packages/shared/clickhouse/scripts/dev-tables.sh:152-153` |
| AMT pre-aggregations: 7d and 30d TTLs (Langfuse Cloud internal analytics) | `traces_7d_amt` / `traces_30d_amt` migrations | `packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:174`, `:261` |
| Primary tables (`traces`, `observations`, `scores`) have NO TTL clause | migrations | `packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql`, `0002_observations.up.sql`, `0003_scores.up.sql` |
| `event_log` and `blob_storage_file_log` (CHANGE logs) have NO TTL | migrations | `packages/shared/clickhouse/migrations/clustered/0007_add_event_log.up.sql:1-19`, `0011_add_blob_storage_file_log.up.sql:1-21` |
| Retention handlers registered when env flag is on | gated registration in `app.ts` | `worker/src/app.ts:570-591` (queue), `:670-690` (batch), `:715-` (trace) |
| Tests: per-project retention processing | 8+ scenarios (cutoff/non-cutoff/media/blob) | `worker/src/__tests__/dataRetentionProcessing.test.ts:23-553` |
| Tests: batch-data-retention cleaner | chunking, hash collisions, deleted-projects | `worker/src/__tests__/batchDataRetentionCleaner.test.ts` |
| Tests: batch-project cleaners (3 modules) | each has its own suite | `worker/src/__tests__/batchProjectCleaner.test.ts`, `batchProjectMediaCleaner.test.ts`, `batchProjectBlobCleaner.test.ts` |
| Tests: batch trace deletion cleaner | "Selects top project workload", etc. | `worker/src/__tests__/batchTraceDeletionCleaner.test.ts` |
| Tests: media retention cleaner | covered | `worker/src/__tests__/mediaRetentionCleaner.test.ts` |
| Tests: deleted-mask cleaner | covered | `worker/src/__tests__/deletedMaskCleaner.test.ts` |
| Tests: tRPC entitlement enforcement for setRetention | both rejection and acceptance paths | `web/src/__tests__/server/projectsRouter-setRetention.servertest.ts:94-133` |
| Tests: project API (tRPC + admin) supports `retentionDays` round-trip | `expect(response.body.retentionDays).toBe(7)` | `web/src/__tests__/server/projects-api.servertest.ts:328-514` |
| Entitlement constant: `data-retention` | central entitlement registry | `web/src/features/entitlements/constants/entitlements.ts:16` |
| Default-init retention env var | `LANGFUSE_INIT_PROJECT_RETENTION` (>=3) | `web/src/env.mjs:372`, `web/src/initialize.ts:100-111` |
| V4 dual-write staging: hard TTL prevents unbounded staging | `observations_batch_staging` | `packages/shared/clickhouse/scripts/dev-tables.sh:103-153` |

## Answers to Dimension Questions

### 1. What grows forever?

With `retentionDays = null` or `0` (the default for a new project,
`packages/shared/prisma/schema.prisma:126` + `web/src/initialize.ts:95-111`):

- ClickHouse `traces`, `observations`, `scores` tables. None of those
  migrations contain a TTL clause
  (`packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql`,
  `0002_observations.up.sql`, `0003_scores.up.sql`).
- ClickHouse `events_full` / `events_core` tables. Same conclusion: the
  dev-table script does not declare a TTL on those tables
  (`packages/shared/clickhouse/scripts/dev-tables.sh:155-410`).
- PostgreSQL `media`, `trace_media`, `observation_media` rows. They only
  shrink on project deletion / trace deletion / explicit media retention
  sweep.
- `blob_storage_file_log` ClickHouse rows. The ReplacingMergeTree only
  collapses duplicates on merge; it does not delete
  (`packages/shared/clickhouse/migrations/clustered/0011_add_blob_storage_file_log.up.sql:14-21`).
- `pending_deletions` Postgres rows can grow if the CH DELETE keeps failing;
  rows are only flipped `is_deleted=true` after success
  (`worker/src/queues/traceDelete.ts:105-119`).

With a positive `retentionDays` the daily
`DataRetentionProcessingJob` covers traces/observations/scores/events/S3-media
plus (via `LANGFUSE_ENABLE_BLOB_STORAGE_FILE_LOG`) the S3 ingestion event
storage and ClickHouse `blob_storage_file_log` soft-delete markers
(`worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:84-103`,
`packages/shared/src/server/data-deletion/ingestionFileDeletion.ts:56-108`).
The opt-in
`BatchDataRetentionCleaner` covers the same tables with a higher-throughput
implementation
(`worker/src/features/batch-data-retention-cleaner/index.ts:152-399`).

What is **not** covered by retention even when set:

- The aggregating-merge-tree `traces_*_amt` rollups — they have their own
  7d/30d TTL that is unrelated to project retention
  (`packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:174, 261`).
- Cloud-only S3 exports (`LANGFUSE_S3_CORE_DATA_UPLOAD_*`); retention of
  the exported copies is the customer's responsibility.
- The PostgreSQL `LegacyPrismaTrace`, `LegacyPrismaObservation`,
  `LegacyPrismaScore` rows — only cleaned when the owning project is
  deleted (`packages/shared/prisma/schema.prisma:153-155`).

### 2. What gets summarized?

In an agent-harness sense, nothing. There is no `summarize`, `compaction`,
or `prune_conversation` step in the codebase that compresses conversation
state. The matches for "summarize" are unrelated to retention
(`packages/shared/src/utils/chatml/adapters/openai.ts` reasoning summaries
in OpenAI ChatML; clickhouse response header `x-clickhouse-summary` parsed
for telemetry by `worker/src/services/IngestionService/index.ts:1449-1461`;
pivot-table summary rows in
`web/src/features/widgets/utils/pivot-table-utils.ts:394`).

What does exist as a form of summarization is ClickHouse **rollup
materialized views**. Two TTL'd rollup tables (`traces_7d_amt`,
`traces_30d_amt`) collapse all trace columns into aggregation-function
columns and rewrite rows at 7/30-day granularity. Their own TTL naturally
expires them
(`packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:172-174,259-261`).
The base `traces` table data they summarize is **not** deleted by them —
the rollups are a complementary read path, not a compaction.

### 3. What gets deleted?

By **job** (opt-in or default):

| Trigger | Tables/files touched | Source |
|---|---|---|
| Per-project cron (`15 3 * * *`) | traces, observations, scores, events_full, events_core, dataset_run_items_rmt; S3 media; S3 event blobs; `media` + `trace_media` + `observation_media` rows | `worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:17-104`, `worker/src/ee/dataRetention/handleDataRetentionSchedule.ts:8-40` |
| Batch data retention cleaner (opt-in) | Same five ClickHouse tables as above | `worker/src/features/batch-data-retention-cleaner/index.ts:152-399` |
| Media retention cleaner (opt-in) | S3 media bucket + `media` + `trace_media` + `observation_media`; S3 event blobs when `LANGFUSE_ENABLE_BLOB_STORAGE_FILE_LOG` | `worker/src/features/media-retention-cleaner/index.ts:35-209` |
| Batch project cleaner (opt-in) | Same five ClickHouse tables, only for projects with `deletedAt IS NOT NULL` | `worker/src/features/batch-project-cleaner/index.ts:45-254` |
| Batch project media cleaner (opt-in) | S3 media for soft-deleted projects (safety net) | `worker/src/features/batch-project-media-cleaner/index.ts:25-126` |
| Batch project blob cleaner (opt-in) | S3 ingestion events + `blob_storage_file_log` soft-delete for soft-deleted projects | `worker/src/features/batch-project-blob-cleaner/index.ts` |
| Batch trace deletion cleaner (opt-in) | Drains `pending_deletions` for projects behind on deletes | `worker/src/features/batch-trace-deletion-cleaner/index.ts:32-186` |
| Project delete (user-initiated) | All of the above + the `Project` row + cascaded Postgres | `web/src/ee/features/admin-api/server/projects/projectById/index.ts:115-185`, `worker/src/queues/projectDelete.ts:22-119` |
| Trace delete (user-initiated) | traces, observations, scores, events_full, events_core, S3 media, S3 ingestion events, PG media + junctions | `worker/src/queues/traceDelete.ts:15-131`, `worker/src/features/traces/processClickhouseTraceDelete.ts:126-159` |
| Score delete (user-initiated) | scores table | `worker/src/queues/scoreDelete.ts:10-20` |
| Dataset item / run delete (user-initiated) | `dataset_run_items_rmt` | `worker/src/queues/datasetDelete.ts:5-9` |
| ClickHouse ENGINE TTL | `observations_batch_staging` (48h self-hosted / 12h cloud) | `packages/shared/clickhouse/scripts/dev-tables.sh:152-153` |
| ClickHouse analytical rollups | `traces_7d_amt` (7d), `traces_30d_amt` (30d) | `packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:174, 261` |

What is **never deleted** by retention:

- The `Project` record itself (only deleted through cascade when an org is
  hard-deleted, or via `Project.deletedAt`).
- `ApiKey`, `User`, `Organization` rows.
- `Survey`, `audit_logs`, `eval-templates`, `prompts` (`Prompt`,
  `PromptVersion`) — none of these have any retention job.

### 4. Does compaction break replay?

Langfuse does not have a "replay" semantics for traces — observation
`read`s query the live tables directly. So there is no replay-time issue
from row deletion. The relevant concern is **read consistency during the
window between a DELETE statement and ClickHouse's physical mutation**.

After `DELETE FROM traces ...` with the lightweight-delete default, rows
become invisible to subsequent reads almost immediately (selection-time
mask), but they are not physically removed until the next mutation or
`APPLY DELETED MASK` step. The `DeletedMaskCleaner` is the explicit
compactor: it inspects `system.parts` for `patch-YYYYMM` partitions older
than the current month, then submits exactly one `ALTER TABLE ... APPLY
DELETED MASK` command per tick and waits up to 30 s for it to show up in
`system.mutations`
(`worker/src/features/deleted-mask-cleaner/index.ts:71-263`,
`helpers.ts:40-56`). The same compactness-via-merge model (no per-row
replay) applies to `traces_7d_amt` / `traces_30d_amt`.

For S3 + media, the deletion has a S3-first / PG-second ordering
(`packages/shared/src/server/media-deletion.ts:88-112`) with a retry-OK
comment ("All callers target expired or soft-deleted media with retry
semantics, so partial failure self-heals on retry (S3 deletes are
idempotent)") which guarantees that the system eventually converges
without manual intervention.

There is one subtle source of replay-like inconsistency: `pending_deletions`
is only marked `isDeleted = true` after the corresponding CH delete
succeeds (`worker/src/queues/traceDelete.ts:105-119`). A retry of a stale
job will therefore re-queue the same trace ids but
`processClickhouseTraceDelete` issues idempotent `DELETE FROM traces WHERE
id IN (...)` (`packages/shared/src/server/repositories/traces.ts:1014-1018`)
and short-circuits on a `cnt = 0` pre-flight
(`packages/shared/src/server/repositories/traces.ts:1004-1010`), so
duplicate re-runs are safe.

### 5. Can users request deletion?

Yes — three entrypoints:

1. **Project-level admin / REST** (`PUT/DELETE
   /api/public/projects/[projectId]` and `POST /api/admin/projects`):
   returns `202` and enqueues `ProjectDelete`. Set
   `retentionDays` via `PUT` (`0` for indefinite, `>=3` otherwise; gated
   by `data-retention` entitlement).
   (`web/src/ee/features/admin-api/server/projects/projectById/index.ts:82-185`,
   `web/src/pages/api/public/projects/index.ts:1-111`.)
2. **Project-level UI** ("Configure Retention" form, `retention = 0` for
   indefinite): goes through `projects.setRetention` tRPC with entitlement
   check (`web/src/features/projects/server/projectsRouter.ts:130-165`,
   `web/src/features/projects/components/ConfigureRetention.tsx:1-111`).
3. **Trace-level UI / API** (`POST /api/public/traces/[traceId]` -> delete,
   `DELETE /api/public/traces/{traceId}` semantics): writes
   `pending_deletions` then enqueues a delayed BullMQ trace delete
   (`packages/shared/src/server/traceDeletionProcessor.ts:26-94`,
   `worker/src/features/traces/processClickhouseTraceDelete.ts:126-159`).

There is also a "skip deletion" safety: any deletion processor can be
forced to no-op for specific projects via the comma-separated
`LANGFUSE_DELETE_SKIP_PROJECT_IDS` env
(`packages/shared/src/server/deletionGuard.ts:11`,
`packages/shared/src/env.ts:200-203`).

## Architectural Decisions

- **Retention is project-scoped, not org- or system-scoped.** A single
  `Project.retentionDays` field is the sole knob
  (`packages/shared/prisma/schema.prisma:126`). This is enforced by the
  `projectRetentionSchema` validator (0 or >=3)
  (`web/src/features/auth/lib/projectRetentionSchema.ts:3-10`).
- **Daily cron over continuous scheduler for the default path.** A
  BullMQ repeatable job fires at 03:15 UTC, fans out one
  `DataRetentionProcessingJob` per project, and runs the deletion under a
  rate-limited worker (`concurrency 1`, `limiter:
  LANGFUSE_PROJECT_DELETE_CONCURRENCY` per
  `LANGFUSE_CLICKHOUSE_PROJECT_DELETION_CONCURRENCY_DURATION_MS`)
  (`packages/shared/src/server/redis/dataRetentionQueue.ts:43`,
  `worker/src/app.ts:578-590`).
- **Stale-value protection.** The job re-reads `retentionDays` before
  deleting, so disabling retention immediately stops pending deletes from
  firing (`worker/src/ee/dataRetention/handleDataRetentionProcessingJob.ts:28-39`).
- **Two-tier deletion: BullMQ path + batch path.** The BullMQ path is
  safe-by-default; the batch path is opt-in (`false` default) and uses
  sorted-by-expired-data single-query DELETEs to scale
  (`worker/src/features/batch-data-retention-cleaner/index.ts:248-301`,
  `worker/src/env.ts:400-402`).
- **Period-based isolation.** PeriodicExclusiveRunner holds a Redis lock
  per table-name so only one worker touches a given table at a time, with
  TTL = `delete_timeout + 300s` buffer
  (`worker/src/features/batch-data-retention-cleaner/index.ts:120-132`,
  `worker/src/utils/PeriodicExclusiveRunner.ts`).
- **ClickHouse deletes use lightweight deletes via ALTER / DELETE.** All
  five core tables are `MergeTree`-family; deletion enqueues a mutation
  the next merge will physically apply. The DeletedMaskCleaner is the
  dedicated "run those mutations on old partitions" worker
  (`worker/src/features/deleted-mask-cleaner/index.ts:42-263`).
- **Hash-keyed parameter names in batch deletes.** Prevents ClickHouse
  index-mismatch bugs when many projects share the same SQL; collisions
  are explicitly checked at runtime
  (`worker/src/features/batch-data-retention-cleaner/index.ts:66-79`).
- **Delete S3 first, then Postgres media rows.** A media item is deleted
  from S3 before its Postgres `media` and junction rows are removed. S3
  deletes are idempotent, the comment explicitly says partial-failure
  self-heals (`packages/shared/src/server/media-deletion.ts:84-87`).
- **Soft delete in `blob_storage_file_log`, hard delete in S3.** A
  ReplacingMergeTree-based log table flags rows with `is_deleted: 1`; the
  S3 bucket objects are removed directly
  (`packages/shared/src/server/data-deletion/ingestionFileDeletion.ts:95-108`).
- **Skip list hook.** `LANGFUSE_DELETE_SKIP_PROJECT_IDS` is parsed once,
  checked per delete
  (`packages/shared/src/server/deletionGuard.ts:5-47`,
  `packages/shared/src/env.ts:200-203`).
- **Retention is gated behind plan entitlement.** Setting `retention > 0`
  requires the `data-retention` entitlement; both UI and admin API enforce
  this and audit-log the change
  (`web/src/features/projects/server/projectsRouter.ts:140-165`,
  `web/src/ee/features/admin-api/server/projects/projectById/index.ts:66-79`).
- **V4 staging has a hard TTL.** `observations_batch_staging` is the
  dual-write staging table for the new `events_*` pipeline and auto-expires
  after 48h (self-hosted) / 12h (cloud) with `ttl_only_drop_parts = 1` so
  only whole partitions are dropped, never partial rows
  (`packages/shared/clickhouse/scripts/dev-tables.sh:144-153`).
- **Aggregation rollups have their own TTL.** `traces_7d_amt` /
  `traces_30d_amt` are aggregating-merge-trees with `TTL toDate(start_time) +
  INTERVAL 7/30 DAY` and a downstream `traces_*_amt_mv` MV
  (`packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:128-300`).

## Notable Patterns

- **Pre-flight min/max timestamp before every targeted DELETE.** The
  pattern in `deleteTraces` (lines `966-1032`) and
  `deleteObservationsByTraceIds` (lines `1257-1318`) computes the
  affected time window first and adds it to the WHERE clause. ClickHouse
  will not partition-prune on `id IN (long-list)` directly; the bounded
  range makes the delete a partition cut.
- **Streaming S3 deletes through `blob_storage_file_log`.** The retention
  processor streams via `getBlobStorageByProjectIdBeforeDate` (an
  `AsyncGenerator`) and accumulates up to 500 refs before each S3
  delete + CH soft-delete
  (`packages/shared/src/server/data-deletion/ingestionFileDeletion.ts:64-93`).
- **Enqueue + delay pattern for trace deletes.** TraceDelete is delayed by
  `LANGFUSE_TRACE_DELETE_DELAY_MS` (default 5 s) so a burst of user
  actions collides into one batch
  (`packages/shared/src/server/traceDeletionProcessor.ts:31-86`).
- **Cleaner metric prefix per subsystem.** Every PeriodicRunner emits
  `langfuse.<feature>_<name>.*` records (gauge and counter), making it
  observable in Datadog/SigNoz
  (`worker/src/features/batch-data-retention-cleaner/index.ts:220-229`,
  `worker/src/features/media-retention-cleaner/index.ts:71-108`,
  `worker/src/features/deleted-mask-cleaner/index.ts:75-126`).
- **Lazy instantiation of BullMQ queues.** `getInstance()` schedules the
  repeatable job on first call so a missed worker boot does not lose the
  registration
  (`packages/shared/src/server/redis/dataRetentionQueue.ts:36-49`).

## Tradeoffs

- **Per-project BullMQ is correct but slow for big fleets.** With 10k
  projects this is 10k jobs × 5+ tables × CH DELETE — that's why the
  batch-data-retention cleaner exists. Adopting it is therefore a
  "graduated scale" decision the operator makes.
- **Daily cadence is fine for cleanup, bad for SLA.** If a project sets
  `retentionDays = 1`, the actual deletion happens only at 03:15 UTC the
  next day. The hourly `MediaRetentionCleaner` is faster but only covers
  S3 + PG media.
- **`pending_deletions` grows on CH failures.** If the CH delete is
  blocked by another long-running mutation, the table accumulates rows
  until success. The `BatchTraceDeletionCleaner` is the explicit safety
  net, but again opt-in.
- **No compaction of PG `LegacyPrismaTrace` rows.** They cascade-delete
  only when the Project row is hard-deleted (path through `prisma.project.delete`,
  `worker/src/queues/projectDelete.ts:84-116`).
- **Compaction of CH lightweight deletes is opt-in.** Without
  `LANGFUSE_CLICKHOUSE_DELETED_MASK_CLEANER_ENABLED=true` the
  `patch-YYYYMM` parts accumulate physical storage until background merges
  catch up — a slow leak.
- **`retention = 0` is indistinguishable from `null`.** Both are accepted
  as "indefinite". The fan-out filter uses `retentionDays: { gt: 0 }`,
  which is correct but easy to miss
  (`worker/src/ee/dataRetention/handleDataRetentionSchedule.ts:14-18`).
- **Defensive ordering of `media` deletion.** S3 is deleted first to avoid
  orphaned S3 objects; the converse (PG first, S3 second) would leave
  dangling S3 blobs. This is documented at line `69-87` of
  `packages/shared/src/server/media-deletion.ts`.
- **No GDPR/PII anonymization in the deletion path.** Deletion is
  physical removal; there is no `replace user_id with null` PII scrub
  alternative. The closest capability is the EE-only
  `applyIngestionMasking` callback
  (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:1-216`)
  which runs **before** the row is inserted, not on a retain/erase later.

## Failure Modes / Edge Cases

- **Re-fetched retention protects against the "queue fired a delete after
  the user disabled it" race**: see
  `handleDataRetentionProcessingJob.ts:28-39`. The comment is explicit.
- **Self-healing on partial media delete.** `deleteMediaFiles`
  documents that S3 deletes are idempotent and PG re-deletes are
  guarded by `in (...mediaIds)` (no error)
  (`packages/shared/src/server/media-deletion.ts:84-87`).
- **`cnt = 0` pre-flight short-circuits noisy deletes.** When a project
  has no matching rows the DELETE statement is not issued
  (`packages/shared/src/server/repositories/traces.ts:1004-1010`,
  `observations.ts:1286-1292`,
  `events.ts:2880-2890`).
- **`request_timeout` floor at 10 min** for all retention/probe
  operations (`LANGFUSE_CLICKHOUSE_DELETION_TIMEOUT_MS`,
  `packages/shared/src/env.ts:320`) prevents worker threads from hanging
  on a stuck CH merge.
- **CH mutation backlog (`system.mutations`) prevents duplicate
  `APPLY DELETED MASK`.** `DeletedMaskCleaner.execute` polls the
  mutation count on the target table before submitting
  (`worker/src/features/deleted-mask-cleaner/index.ts:87-92`,
  `:255-262`).
- **`System.mutations` visibility race acknowledged.** Workers wait up to
  30 s for the submitted mutation to appear, "tiny mutations may complete
  before system.mutations ever exposes them as unfinished, which is fine
  because the lock was still held through the duplicate-submit window"
  (`worker/src/features/deleted-mask-cleaner/index.ts:242-253`).
- **Skip list checked even for "all-deleted" work.** The
  `shouldSkipDeletionFor` guard runs at the top of trace deletion
  handlers, so a misconfigured project id does not break the system; it
  just silently no-ops with a log
  (`packages/shared/src/server/deletionGuard.ts:9-47`).
- **AMT rollups use background TTL.** Misconfiguring the `ORDER BY` or
  partition of `traces_*_amt` could violate the AMT ordering invariant
  (`project_id, id`) and silently return wrong aggregates; not currently
  flagged beyond the migration comment
  (`packages/shared/clickhouse/migrations/clustered/0023_traces_aggregating_merge_trees.up.sql:75-87`).
- **Hash collision crash protection.** `buildRetentionConditions` throws
  loudly if MD5-8 collisions occur between project ids
  (`worker/src/features/batch-data-retention-cleaner/index.ts:71-79`).
- **Project deletion concurrency limit**: 1 per
  `LANGFUSE_CLICKHOUSE_PROJECT_DELETION_CONCURRENCY_DURATION_MS` window
  (`worker/src/app.ts:582-589`); a single tenant deletion storm cannot
  starve other workers.
- **No automatic `OPTIMIZE FINAL` after retention.** Heavy DELETEs rely
  on background merges and the explicit DeletedMaskCleaner; long delays
  between deletion and disk reclaim are possible.

## Future Considerations

- **Promote `events_full` / `events_core` from dev-table to migration.**
  Right now they live in
  `packages/shared/clickhouse/scripts/dev-tables.sh:155-410`, gated on
  self-hosters running >= ClickHouse 24.5/25.x. Once baseline is
  raised, add an explicit TTL on `events_full` mirroring the staging
  table's partition discipline.
- **Add a system-wide hard retention cap.** Today `retentionDays = 0`
  means "forever", with no tier-based ceiling. A maximum (e.g.
  730 days on Pro, 3650 on Enterprise) would let self-hosters rest easier.
- **Auto-promote the batch cleaner on by default for installations with
  >N projects.** Self-hosters who flip
  `LANGFUSE_BATCH_DATA_RETENTION_CLEANER_ENABLED=true` get a clear win
  but currently must discover the flag.
- **Replay/debug story for soft-deleted rows.** A `db.traces__with_tombstones`
  query helper for short-lived debugging, with the tombstones
  automatically cleared by DeletedMaskCleaner, would make CL incidents
  easier to triage.
- **Anonymization / PII scrub path.** Today deletion is binary; adding
  `user_id` rotation as a first-class "request deletion of all data
  for user X" operation would close a real gap.
- **Inline partition-drops for retention.** ClickHouse supports
  `ALTER TABLE ... DROP PARTITION` which is metadata-only. Combining the
  retention job with `event_ts`-aligned partitions on `traces` would
  reduce DELETE cost by orders of magnitude — currently bounded to
  `request_timeout` of 10 min per DELETE per project.
- **Surface `langfuse.batch_data_retention_cleaner.seconds_past_cutoff`
  in the admin UI** (it's already a gauge, but the user has no
  self-service view).

## Questions / Gaps

- No evidence found in the selected source for a UI surfacing of the
  `seconds_past_cutoff` gauge or the `delete_failures` counter. Operators
  must rely on Datadog/SigNoz.
- The cluster-mode flag for the DeletedMaskCleaner
  (`LANGFUSE_CLICKHOUSE_DELETED_MASK_CLEANER_CLUSTER_MODE_ENABLED`) is
  off by default
  (`worker/src/env.ts:436-438`) but there is no test covering the
  On-Cluster behavior — search boundary: `worker/src/__tests__/deletedMaskCleaner.test.ts`
  was inspected and only single-replica tests are present.
- Whether `pending_deletions` rows are ever hard-pruned (e.g. after N days
  regardless of CH delete status) — no evidence found. Search boundary:
  searched `pending_deletions`, `isDeleted`, and `LANGFUSE_PENDING_DELETION_*`
  env vars across `packages/shared/`, `worker/`, and `web/`.
- Whether the `event_log` ClickHouse table (a separate table from
  `blob_storage_file_log`) is also rolled into the retention sweep —
  no evidence found in `handleDataRetentionProcessingJob.ts` or
  `BatchDataRetentionCleaner`. The migration at
  `0007_add_event_log.up.sql:1-19` is unannotated and sits outside the
  retention table list.
- Whether dataset *items* (Postgres `DatasetItem`) participate in any
  retention sweep — no evidence found. Search boundary: searched all of
  `packages/shared/src/server/repositories/dataset-items.ts` and
  `worker/src/features/datasets/`. Only `dataset_run_items_rmt` (the
  experiment-run join) participates.
- Exact behavior on Cloud when retention is disabled but CH data
  accumulates past Cloud's account-hard-cap (if any) — no evidence found
  in the open-source worker.

---

Generated by `02.09-state-pruning-compaction-and-retention.md` against `langfuse`.
