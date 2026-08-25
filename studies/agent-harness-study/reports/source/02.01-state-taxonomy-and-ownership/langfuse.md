# Source Analysis: langfuse

## Dimension 02.01: State Taxonomy and Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo (Next.js `web`, BullMQ `worker`, `@langfuse/shared`); Postgres (Prisma), ClickHouse, Redis, S3 |
| Analyzed | 2026-07-03 |

## Summary

Langfuse is an LLM observability platform, not a classic agent loop, so its "state" is dominated by high-volume telemetry (traces/observations/scores) plus a bundle of control-plane records. It uses an explicit, tiered persistence model where each state category is deliberately placed in one of four backends, and ownership boundaries are codified in package docs and contract files:

- **Postgres (Prisma)** is the source-of-truth for durable control-plane state: accounts/projects, API keys, prompts, datasets, eval configuration and eval execution records, and the in-app agent conversation/run/event tables (`packages/shared/prisma/schema.prisma`).
- **ClickHouse** owns durable, append-oriented telemetry — traces, observations, scores, dataset run items — modelled as immutable wide events with read-time dedup via `ReplicatedReplacingMergeTree` (`packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:23`).
- **S3/blob storage** holds the raw ingested event payloads and is treated as the durable source-of-truth for ingestion, with a `event_log` pointer table in ClickHouse (`packages/shared/src/server/ingestion/processEventBatch.ts:275`, `packages/shared/clickhouse/migrations/clustered/0007_add_event_log.up.sql:1`).
- **Redis** owns ephemeral coordination state: BullMQ queues (in-flight jobs), plus derived caches (API-key cache, eval-config cache) that are safe to lose (`packages/shared/src/server/queues.ts:342`, `packages/shared/src/server/evalJobConfigCache.ts:4`).
- **In-memory LRU** (`LocalCache`) is a per-process hot cache, fully ephemeral (`packages/shared/src/server/cache/localCache.ts:18`).

The closest analogue to agent-harness state is the **in-app agent** feature (`web/src/ee/features/in-app-agent/`), which uses event-sourcing: `InAppAgentEvent` is an append-only, `sequenceNumber`-ordered log that is the source-of-truth, and conversation messages are *derived* by replaying events through an accumulator (`web/src/ee/features/in-app-agent/server/persistence.ts:464`). Run/execution state is tracked in `InAppAgentRun` with row-level locking and stale-run reclamation (`persistence.ts:112`).

Ownership is explicitly documented: queue payload/name contracts are owned by `packages/shared/src/server/queues.ts`, the Postgres and ClickHouse schemas are owned by `@langfuse/shared`, and a strict dependency direction (`web`/`worker`/`ee` → `shared`, never reverse) is enforced by convention (`.agents/AGENTS.md`).

## Rating

**8 / 10** — Clear, tiered model with explicit interfaces (Prisma schema, queue contracts, ClickHouse migrations), documented ownership boundaries, and operational safeguards (stale-run recovery, S3-as-source-of-truth, blocking-but-non-failing S3 upload, DLQ). Crash-survival semantics are legible: durable state lives in Postgres/ClickHouse/S3, and in-flight Redis queue jobs plus in-memory/Redis caches are explicitly designed to be reconstructable or safe to lose. It falls short of 9-10 because the agent-specific execution state (in-app agent) is self-described "v1 foreground-only" with a wall-clock stale heuristic rather than proven durable resumption (`persistence.ts:23`, `persistence.ts:127`), and there is no single document enumerating the full state taxonomy — it must be reconstructed from schema + migrations + queue contracts.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Conversation state (durable) | `InAppAgentConversation` Postgres model with `visibilityScope`, soft-delete `deletedAt`, `providerSessionId` | `packages/shared/prisma/schema.prisma:218` |
| Conversation events (source-of-truth log) | `InAppAgentEvent` append log keyed by `(projectId, conversationId, sequenceNumber)`, `event Json` payload | `packages/shared/prisma/schema.prisma:239` |
| Execution/run state (durable) | `InAppAgentRun` with `finishedAt`, `errorCode`, `errorMessage` lifecycle fields | `packages/shared/prisma/schema.prisma:256` |
| Run lifecycle + concurrency guard | `createRun` locks conversation row (`SELECT ... FOR UPDATE`), reclaims stale runs, rejects concurrent active run | `web/src/ee/features/in-app-agent/server/persistence.ts:112`, `:709` |
| Derived conversation state | Messages reconstructed by replaying events through accumulator (not stored) | `web/src/ee/features/in-app-agent/server/persistence.ts:306`, `:464` |
| Event flush policy | `shouldFlushPersistedEvent` persists only at message/tool/run boundaries; streaming deltas are ephemeral until flush | `web/src/ee/features/in-app-agent/server/persistence.ts:341` |
| Telemetry state (durable, append) | `traces` ClickHouse table, `ReplicatedReplacingMergeTree(event_ts, is_deleted)`, soft delete via `is_deleted` | `packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:19`, `:23` |
| Telemetry tables set | observations/scores/dataset_run_items/event_log migrations | `packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql`, `0003_scores.up.sql`, `0022_dataset_run_items.up.sql`, `0007_add_event_log.up.sql:1` |
| Raw event state (source-of-truth) | S3 upload of grouped raw events is "blocking, but non-failing"; abort ingestion if S3 fails | `packages/shared/src/server/ingestion/processEventBatch.ts:275`, `:324` |
| Blob pointer index | `event_log` MergeTree maps `(project_id, entity_type, entity_id)` → `bucket_name/bucket_path` | `packages/shared/clickhouse/migrations/clustered/0007_add_event_log.up.sql:1` |
| In-flight job state (ephemeral) | `QueueName`/`QueueJobs` enums; ingestion, eval, delete, export, webhook queues | `packages/shared/src/server/queues.ts:342`, `:382` |
| Ingestion job payload | Queue job carries `fileKey`, `bucketPrefix`, `eventBodyId` — pointer to S3, not the payload itself | `packages/shared/src/server/ingestion/processEventBatch.ts:375` |
| Eval config state (durable) | `JobConfiguration` (config) + `JobExecution` (per-run status/start/end/error) Postgres models | `packages/shared/prisma/schema.prisma:1046`, `:1083` |
| Cross-store references (no FK) | `JobExecution.jobInputTraceId` etc. reference ClickHouse rows; comment notes "no fk constraint" | `packages/shared/prisma/schema.prisma:1101`, `:1105` |
| Eval config cache (derived, ephemeral) | Redis "no eval configs" negative cache, TTL 600s | `packages/shared/src/server/evalJobConfigCache.ts:16`, `:21` |
| Auth/API-key state | Durable `ApiKey`/`LlmApiKeys` (encrypted `secretKey`) in Postgres; Redis-backed `api-key:` cache | `packages/shared/prisma/schema.prisma:188`, `:294`; `packages/shared/src/server/auth/apiKeyCache.ts:1` |
| Session state | NextAuth `Session`/`Account` (Postgres), plus `TraceSession` correlation record | `packages/shared/prisma/schema.prisma:40`, `:382` |
| In-process cache (ephemeral) | `LocalCache` LRU, `ttlAutopurge:false`, `allowStale:false`, disposer records evictions | `packages/shared/src/server/cache/localCache.ts:18`, `:31` |
| Background migration state | `BackgroundMigration.state Json`, `workerId`, `lockedAt` for durable resumable migrations | `packages/shared/prisma/schema.prisma:279` |
| Ownership boundaries | Queue contracts owned by `queues.ts`; schema owned by shared; dependency direction documented | `.agents/AGENTS.md` (Project Structure / Dependency direction) |
| Ordering / dedup on ingest | events grouped by `entityType-body.id`; update events sorted last to survive out-of-order processing | `packages/shared/src/server/ingestion/processEventBatch.ts:213`, `:434` |

## Answers to Dimension Questions

1. **What kinds of state exist?**
   - *Conversation state*: `InAppAgentConversation` + `InAppAgentEvent` event log (`schema.prisma:218`, `:239`).
   - *Execution/run state*: `InAppAgentRun` (`schema.prisma:256`) and eval `JobExecution` (`schema.prisma:1083`); in-flight jobs in Redis/BullMQ queues (`queues.ts:342`).
   - *Tool state*: tool-call lifecycle is encoded as AG-UI events inside the `InAppAgentEvent` log (`persistence.ts:594`-`660`); tool/schema definitions in `LlmTool`/`LlmSchema` (`schema.prisma:1442`, `:1426`).
   - *Memory / telemetry state*: traces/observations/scores in ClickHouse (`0001_traces.up.sql`), raw events in S3 (`processEventBatch.ts:275`).
   - *Artifact / media state*: `Media`/`TraceMedia`/`ObservationMedia` (Postgres) referencing S3 blobs (`schema.prisma:1353`).
   - *Approval / review state*: `AnnotationQueue`/`AnnotationQueueItem` (`schema.prisma:577`), `Comment` (`schema.prisma:755`).
   - *Eval state*: `JobConfiguration`/`JobExecution` (`schema.prisma:1046`).
   - *Runtime configuration*: validated env (`packages/shared/src/env.ts`), encrypted `LlmApiKeys`/`ApiKey` (`schema.prisma:294`, `:188`), `DefaultLlmModel`/`Model`/`Price` (`schema.prisma:1121`, `:892`).
   - *Cache state*: `LocalCache` in-memory (`localCache.ts:18`) and Redis caches (`evalJobConfigCache.ts`, `apiKeyCache.ts`).

2. **Which state is source-of-truth?**
   - Raw ingested events in **S3** are the ingestion source-of-truth; ClickHouse rows are built from them (`processEventBatch.ts:281`-`289`, `:324`). ClickHouse queue jobs carry only the S3 `fileKey`/`bucketPrefix` (`processEventBatch.ts:386`).
   - `InAppAgentEvent` is the source-of-truth for a conversation; messages are derived from it (`persistence.ts:268`, `:306`).
   - **Postgres** is source-of-truth for control-plane config (projects, API keys, eval config, prompts, datasets).

3. **Which state is derived?**
   - Conversation messages, derived by event replay (`persistence.ts:464`, `getMessagesFromEvents:306`).
   - ClickHouse trace/observation rows, derived from S3 events by the ingestion worker (`processEventBatch.ts:375`).
   - Redis `api-key:` cache and eval-config negative cache, derived from Postgres (`apiKeyCache.ts:1`, `evalJobConfigCache.ts:16`).
   - Aggregating merge trees / analytics tables derived from base ClickHouse tables (`0023_traces_aggregating_merge_trees.up.sql`).

4. **Which state is ephemeral?**
   - `LocalCache` in-memory LRU (`localCache.ts:39`).
   - Redis queue jobs while in-flight and Redis caches (`queues.ts:342`, `evalJobConfigCache.ts:21`).
   - Streaming AG-UI deltas not yet flushed at a persist boundary (`persistence.ts:341`).
   - Sampling/slowdown/failure-tracking counters in Redis (`redis/s3SlowdownTracking.ts`, `redis/ingestionFailureTracking.ts` referenced at `processEventBatch.ts:42`).

5. **Which state is safe to lose?**
   - All in-memory and Redis-derived caches (rebuildable from Postgres/ClickHouse).
   - In-flight ingestion queue jobs when the underlying S3 payload persists — the job is a pointer and can be re-enqueued/re-derived (`processEventBatch.ts:386`).
   - Unflushed streaming deltas mid-run (the run is marked stale and reclaimed; `persistence.ts:127`).
   - Not safe to lose: Postgres control-plane rows, ClickHouse telemetry, and S3 raw events.

## Architectural Decisions

- **Four-tier state placement by access pattern** — durable config in Postgres, high-volume immutable telemetry in ClickHouse columnar tables, raw payloads in S3, coordination/cache in Redis. Justified by the wide-event / observability-2.0 principles in `.agents/ARCHITECTURE_PRINCIPLES.md:13`.
- **Immutable append + read-time dedup for telemetry** — `ReplicatedReplacingMergeTree(event_ts, is_deleted)` chooses last-writer-wins by `event_ts` and soft delete by `is_deleted` rather than in-place updates (`0001_traces.up.sql:23`). Update-type events are sorted last on ingest to make merges deterministic (`processEventBatch.ts:434`).
- **S3 as ingestion source-of-truth** — events are written to S3 first (grouped by entity id to cut write ops) and the queue transports only pointers; if S3 write fails, ingestion aborts rather than proceeding on lossy in-memory data (`processEventBatch.ts:284`, `:324`).
- **Event-sourced conversations** — the agent persists an ordered event log and derives messages, so partial streams can be compacted and replayed, and history does not depend on volatile schemas (redirect tool scaffolding is dropped on persist, `persistence.ts:756`).
- **Explicit contract ownership** — queue name/payload schemas centralized in `queues.ts`; schema ownership in `@langfuse/shared`; one-directional package dependency graph (`.agents/AGENTS.md`).
- **Cross-store references without FKs** — eval `JobExecution` intentionally stores `jobInputTraceId`/`jobInputObservationId` with no FK because those rows live in ClickHouse; cleanup relies on project cascade (`schema.prisma:1101`).

## Notable Patterns

- **Pointer indirection**: `event_log` (ClickHouse) and queue payloads reference S3 objects rather than embedding payloads (`0007_add_event_log.up.sql:1`, `processEventBatch.ts:386`).
- **Composite tenant-scoped keys**: nearly all durable models are keyed/indexed by `projectId` first (`schema.prisma:234`, `:251`; ClickHouse `ORDER BY (project_id, ...)` `0001_traces.up.sql:28`), making project deletion a clean cascade boundary.
- **Row-lock + lazy reclamation** for single-writer run semantics (`persistence.ts:709`, `:129`).
- **Negative caching** to avoid DB/queue work for projects with no eval configs (`evalJobConfigCache.ts:16`).
- **Secondary queues / sharding** to isolate high-throughput tenants' in-flight state (`queues.ts:347`, `:355`; `redis/sharding.ts`).

## Tradeoffs

- **Multi-store fan-out increases operational surface** — the same logical entity spans S3 (raw), ClickHouse (queryable), and Postgres (references). `.agents/ARCHITECTURE_PRINCIPLES.md:31` explicitly frames extra stores/queues as costs that must "earn their long-term operational burden," but consistency across stores is eventual, not transactional.
- **No cross-store FK integrity** — `JobExecution` → ClickHouse references can dangle until a cascade/cleanup runs (`schema.prisma:1101`); referential integrity is traded for scale.
- **Read-time dedup cost** — `ReplacingMergeTree` pushes deduplication into query/merge time; unmerged parts can transiently expose duplicate/stale rows (`0001_traces.up.sql:23`), a cost acknowledged in the architecture principles (`ARCHITECTURE_PRINCIPLES.md:21`).
- **Wall-clock staleness for runs** — the in-app agent decides a run is dead via `ACTIVE_RUN_STALE_AFTER_MS` (150s) rather than a heartbeat, so a slow-but-alive run could be reclaimed and a truly dead one blocks up to the timeout (`persistence.ts:23`, `:129`).
- **Unflushed streaming deltas are lost on crash** — only boundary events are flushed (`persistence.ts:341`), trading write amplification for partial-message loss on abrupt failure.

## Failure Modes / Edge Cases

- **Crash mid-run**: the run row stays `finishedAt = null`; the next `createRun` marks it stale and unblocks the conversation, but the interrupted assistant turn is not resumed — v1 is foreground-only (`persistence.ts:127`-`141`).
- **S3 SlowDown / upload failure**: project is flagged for the secondary queue and ingestion of the batch is aborted (no lossy ClickHouse write) (`processEventBatch.ts:296`, `:324`).
- **Redis unavailable**: ingestion throws ("Redis not initialized") and caches fall back to source stores (`processEventBatch.ts:330`; `evalJobConfigCache.ts:31` returns `false` on Redis miss).
- **Out-of-order / duplicate events**: dedup by `entityType-id`, update events sorted last, and a date-boundary ingestion delay window prevent split-day duplicates (`processEventBatch.ts:213`, `:71`-`91`).
- **Concurrent run attempts**: rejected with `LangfuseConflictError` under the conversation row lock (`persistence.ts:152`).
- **Oversized/PII entity IDs**: sanitized for S3 key safety with hashed logging to avoid leaking IDs (`processEventBatch.ts:235`-`246`).

## Future Considerations

- Introduce heartbeat-based liveness for `InAppAgentRun` to replace the fixed 150s stale window and enable background/resumable runs (`persistence.ts:23`).
- Publish a single canonical "state taxonomy" doc enumerating every store, its durability, and owner — currently reconstructed from `schema.prisma` + ClickHouse migrations + `queues.ts`.
- Consider a reconciliation/GC job for dangling cross-store references (e.g., `JobExecution.jobInputTraceId`) beyond project-level cascade (`schema.prisma:1101`).
- Document explicit recovery/replay procedure from S3 `event_log` into ClickHouse as a first-class DR runbook (`0007_add_event_log.up.sql:1`).

## Questions / Gaps

- **Exactly-once vs at-least-once for ingestion**: the code relies on ReplacingMergeTree dedup and a delay window, but the precise delivery guarantee of the BullMQ ingestion path (retries/DLQ interaction) was not fully traced — `DeadLetterRetryQueue` exists (`queues.ts:374`) but its replay semantics were not read in this pass. No clear evidence found within the files inspected.
- **Whether `InAppAgentEvent` is ever pruned/compacted at rest** beyond in-flight `compactEvents` on write (`persistence.ts:747`) — no retention/GC evidence found for that table in the inspected files.
- **Transactional consistency between Postgres and ClickHouse on project delete** is asserted via cascade comments (`schema.prisma:1101`) but the deletion processor implementation (`packages/shared/src/server/traceDeletionProcessor.ts`) was not read in detail here.

---

Generated by `02.01-state-taxonomy-and-ownership` against `langfuse`.
