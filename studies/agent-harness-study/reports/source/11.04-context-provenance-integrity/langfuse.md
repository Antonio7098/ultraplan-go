# Source Analysis: langfuse

## Dimension 11.04 — Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript (Next.js web app, BullMQ worker, shared package; ClickHouse + Postgres + Redis/S3 storage) |
| Analyzed | 2026-08-26 |

## Summary

Langfuse is an LLM observability platform, so its "context items" are ingested traces, observations (spans/generations), scores, dataset items, and media blobs. Provenance is a first-class product concern and is implemented as layered, explicit metadata rather than convention:

1. **Source attribution per record.** Every ingestion request is attributed to an API key plus SDK name/version extracted from `x-langfuse-sdk-name`/`x-langfuse-sdk-version` headers (`packages/shared/src/server/ingestion/ingestionAttribution.ts:188-203`). This attribution travels through the queue payload (`packages/shared/src/server/ingestion/processEventBatch.ts:394-396`) and is persisted into dedicated ClickHouse columns (`ingestion_api_key`, `ingestion_sdk_name`, `ingestion_sdk_version`; migration `packages/shared/clickhouse/migrations/unclustered/0035_add_ingestion_attribution_columns.up.sql:1-8`, written at `worker/src/services/IngestionService/index.ts:677-679`). The v4 events table adds an instrumentation block: `source`, `service_name`, `service_version`, `scope_name`, `scope_version`, and `telemetry_sdk_*` (`packages/shared/clickhouse/migrations/unclustered/0039_create_events_full.up.sql:82-90`, populated in `packages/shared/src/server/repositories/definitions.ts:535-546`).
2. **Freshness via event-time vs ingest-time separation.** Every ingestion event requires a client-supplied top-level `timestamp` (`packages/shared/src/server/ingestion/types.ts:617-621`) which becomes the `event_ts` version vector of a `ReplacingMergeTree(event_ts, is_deleted)` (`0039_create_events_full.up.sql:95-98,112`), while server-side `created_at` is preserved across merges (`worker/src/services/IngestionService/index.ts:834-835`). Immutable-key lists prevent updates from rewriting original timestamps, environment, or trace linkage (`worker/src/services/IngestionService/index.ts:158-207`).
3. **Trust levels through enums and reserved namespaces.** Scores carry a `source` of `API | EVAL | ANNOTATION`, with `EVAL` reserved for internal evaluators on the public API (`packages/shared/src/domain/scores.ts:4-21`); annotation scores must reference a validated score config (`packages/shared/src/server/ingestion/validateAndInflateScore.ts:28-33`). Internal system traces are fenced into reserved `langfuse-*` environments that public ingestion strips from user input (`packages/shared/src/server/ingestion/types.ts:228-246` vs the internal schema at `types.ts:248-258,840-844`).
4. **Transformation traceability by keeping raw + provided data.** Raw payloads are archived to blob storage before transformation and each transformed row stores `blob_storage_file_path` back to that raw source (`processEventBatch.ts:279-292`, `worker/src/services/IngestionService/index.ts:457`). User-provided vs server-computed values are kept side-by-side (`provided_usage_details` vs `usage_details`, `provided_cost_details` vs `cost_details`, `provided_model_name` vs `model_id`; `0039_create_events_full.up.sql:36-44`). Media extraction replaces inline base64 with self-describing `@@@langfuseMedia:type=…|id=…|source=…@@@` references (`packages/shared/src/server/otel/OtelMediaProcessor.ts:236`).

The main weaknesses: legacy traces/observations tables do not carry ingestion-attribution columns (only scores and the v4 events tables do), masking transformations are observable only through metrics/logs rather than a persisted per-record flag, there is no general trust/authority weight on context records themselves, and client-supplied event timestamps are accepted without visible skew clamping.

## Rating

**7 / 10** — Clear provenance model with explicit schemas (attribution columns, source enums, reserved environment namespaces), tests (`ingestionAttribution.test.ts`, `OtelIngestionProcessor.metadataDropped.test.ts`, ingestion masking tests), and operational safeguards (raw blob retention for replay, fail-open/fail-closed masking modes, immutable merge keys). It falls short of 9-10 because provenance coverage is inconsistent between legacy and v4 storage paths, redaction events are not recorded on the affected records, and no unified trust-level field exists.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source annotations — ingestion attribution type | `IngestionAttribution = { ingestionApiKey, ingestionSdkName, ingestionSdkVersion }` built from verified auth scope + headers | `packages/shared/src/server/ingestion/ingestionAttribution.ts:7-11,188-203` |
| Source annotations — attribution persisted via queue payload | Queue payload carries `ingestionApiKey/SdkName/SdkVersion` | `packages/shared/src/server/ingestion/processEventBatch.ts:386-397` |
| Source annotations — ClickHouse columns for scores | Migration adds `ingestion_api_key`, `ingestion_sdk_name`, `ingestion_sdk_version` | `packages/shared/clickhouse/migrations/unclustered/0035_add_ingestion_attribution_columns.up.sql:1-8` |
| Source annotations — score rows stamped with attribution | Score insert sets `ingestion_api_key/sdk_name/sdk_version` from attribution | `worker/src/services/IngestionService/index.ts:677-679` |
| Source annotations — v4 events instrumentation block | `source`, `service_name/version`, `scope_name/version`, `telemetry_sdk_*` columns under "Source metadata (Instrumentation)" | `packages/shared/clickhouse/migrations/unclustered/0039_create_events_full.up.sql:82-90` |
| Source annotations — OTel processor stamps source | `source: "otel"` plus serviceName/serviceVersion/scope/telemetry sdk fields | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:624-634` |
| Source annotations — hierarchy linkage | TraceBody/Observation bodies require `traceId`, optional `parentObservationId`; observation schema has `trace_id`, `parent_observation_id` | `packages/shared/src/server/ingestion/types.ts:425-454`; `packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:3,6` |
| Source annotations — prompt lineage on generations | `promptName`/`promptVersion` refined to be both-or-neither; stored as `prompt_id/prompt_name/prompt_version` | `packages/shared/src/server/ingestion/types.ts:493-500`; `0002_observations.up.sql:25-27` |
| Source annotations — dataset item origin | DatasetItem has `sourceTraceId`/`sourceObservationId` with hash indexes | `packages/shared/prisma/schema.prisma:600-601,614-615`; `packages/shared/src/domain/dataset-items.ts:22-23` |
| Source annotations — UI/query exposure of provenance | Events table exposes "API Key", "SDK Name", "SDK Version", "Ingestion Source" columns; filter option exists | `packages/shared/src/server/tableMappings/mapEventsTable.ts:22-45`; `packages/shared/src/server/queries/clickhouse-sql/event-filter-options.ts:134` |
| Freshness — mandatory event timestamp | Ingestion event envelope requires ISO `timestamp`; body timestamps nullable with defaults applied downstream | `packages/shared/src/server/ingestion/types.ts:617-621` |
| Freshness — ReplacingMergeTree versioning | Rows versioned by `event_ts` with `is_deleted` flag; `created_at/updated_at/event_ts` present on all tables | `packages/shared/clickhouse/migrations/unclustered/0039_create_events_full.up.sql:95-98,112`; `0001_traces.up.sql:16-23` |
| Freshness — created_at preservation on update | Merge keeps first-seen `created_at` (`clickhouseTraceRecord?.created_at ?? createdAtTimestamp`) | `worker/src/services/IngestionService/index.ts:834-835,978-979,757-758` |
| Freshness — immutability of key identity fields | `immutableEntityKeys` protect `id/project_id/timestamp/trace_id/start_time/created_at/environment` from overwrite | `worker/src/services/IngestionService/index.ts:158-207` |
| Freshness — out-of-order handling | Batch sorted updates-last ascending by timestamp; date-boundary delay avoids duplicates; time-sorted merge list | `packages/shared/src/server/ingestion/processEventBatch.ts:66-92,441-460`; `worker/src/services/IngestionService/index.ts:1215-1228` |
| Freshness — partition locking via first-seen timestamp | Staging writes set `s3_first_seen_timestamp` partition-aware within ~4 min window | `worker/src/services/IngestionService/index.ts:1085-1099` |
| Freshness — retention lifecycle | Project `retentionDays` column; data-retention queue processors; media retention cleaner deletes expired media/blob files | `packages/shared/prisma/schema.prisma:135`; `worker/src/queues/dataRetentionQueue.ts:11-41`; `worker/src/features/media-retention-cleaner/index.ts:21-27,124-133` |
| Trust level — score source enum | `API/EVAL/ANNOTATION`; `EVAL` excluded from public API enum via `satisfies` subset | `packages/shared/src/domain/scores.ts:4-21` |
| Trust level — annotation scores need config validation | Central choke point enforces configId for ANNOTATION and validates against config (incl. archived configs rejected) | `packages/shared/src/server/ingestion/validateAndInflateScore.ts:28-33,205-253` |
| Trust level — reserved internal namespaces | Public env names lowercased, `langfuse-*` prefix stripped idempotently; internal schema keeps it; `isLangfuseInternal` selects schema | `packages/shared/src/server/ingestion/types.ts:225-277,840-844`; queue flag rationale at `packages/shared/src/server/queues.ts:71-77` |
| Trust level — internal SDK names | Reserved `langfuse-internal-ai-sdk` / `langfuse-internal-otel-writer` names | `packages/shared/src/server/ingestion/ingestionAttribution.ts:15-18` |
| Trust level — auth scopes gate writes | `isAuthorized` checks `accessLevel === "project" | "scores"` before enqueueing | `packages/shared/src/server/ingestion/processEventBatch.ts:420-436` |
| Trust level — severity annotation | `ObservationLevel` enum `DEBUG/DEFAULT/WARNING/ERROR` on every observation body | `packages/shared/src/server/ingestion/types.ts:18` |
| Transformation log — raw payload archive + back-pointer | Raw event arrays uploaded to blob storage keyed per entity; row stores `blob_storage_file_path` | `packages/shared/src/server/ingestion/processEventBatch.ts:279-292`; `worker/src/services/IngestionService/index.ts:457`; column at `0039_create_events_full.up.sql:93` |
| Transformation log — provided vs computed columns | `provided_usage_details`/`usage_details`, `provided_cost_details`/`cost_details`, `provided_model_name`/`internal_model_id`+`model_id` pairs | `0039_create_events_full.up.sql:35-44`; `packages/shared/src/server/repositories/definitions.ts:49-51` |
| Transformation log — enrichment documented | `createEventRecord` docstring: single point of transformation (prompt lookup, tokenization/cost enrichment, metadata flattening, timestamp normalization) | `worker/src/services/IngestionService/index.ts:275-289,320-354` |
| Transformation log — media reference format | Inline base64 replaced by `@@@langfuseMedia:type=<ct>|id=<mediaId>|source=<origin>@@@` | `packages/shared/src/server/otel/OtelMediaProcessor.ts:236`; parser at `packages/shared/src/utils/IORepresentation/chatml/types.ts:41-54` |
| Transformation log — field overflow to media | Oversized fields replaced with media references tagged `source=field_size_limit` | `worker/src/features/observation-field-overflow/processObservationFieldOverflow.ts:250-260` |
| Transformation log — dropped-metadata telemetry | `langfuse.ingestion.metadata_dropped` metric increments with reason/source/domain when attributes can't be transformed | `packages/shared/src/server/otel/OtelIngestionProcessor.metadataDropped.test.ts:120-131,288-305` |
| Redaction — EE ingestion masking callback | OTel payloads POSTed to external masking endpoint with retries; fail-closed drops, fail-open proceeds; metrics recorded | `packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:145-216`; call site `worker/src/queues/otelIngestionQueue.ts:518-542` |
| Media integrity — content hashing | Media ID derived from SHA-256 hash; upsert guards truncated-ID collisions so content cannot cross-link | `packages/shared/src/server/media/mediaService.ts:17-22,41-104` |
| Serialization survival — zod read/insert schemas | Record schemas round-trip ClickHouse values (string dates coerced, int64 strings parsed) for traces/observations/scores/events | `packages/shared/src/server/repositories/definitions.ts:64-107,459-577` |
| Serialization survival — queue contracts with rolling-deploy compat | Attribution fields optional in queue schemas explicitly for in-flight job compatibility | `packages/shared/src/server/queues.ts:30-40,52-70` |
| Serialization survival — public API contract documents provenance format | Fern spec documents the media reference string format | `fern/apis/server/definition/commons.yml:689`; generated `web/public/generated/api/openapi.yml:10427` |

## Answers to Dimension Questions

**1. Does each context item know where it came from?**
Yes, strongly. Each record links into a hierarchical graph (`project_id → trace_id → span_id → parent_span_id`; `0039_create_events_full.up.sql:3-6`), carries deployment context (`environment`, `release`, `version`), and is stamped with how it arrived: ingestion API key + SDK name/version (`ingestionAttribution.ts:188-203`, persisted at `IngestionService/index.ts:677-679`) plus, on the v4 events table, the full instrumentation block (`service_name`, `scope_name`, `telemetry_sdk_*`; `OtelIngestionProcessor.ts:624-634`). Dataset items additionally point back to the run that produced them (`schema.prisma:600-601`). Caveat: the legacy `traces`/`observations` ClickHouse tables have no attribution columns — migrations 0035 and 0042 target `scores` and the events tables only — so SDK/API-key provenance for legacy-path observations exists only if the batch was also forwarded to the staging/events path (`definitions.ts:98-104`).

**2. Is freshness tracked?**
Yes, with a deliberate two-clock model. Client event time arrives as the mandatory envelope `timestamp` (`types.ts:617-621`) and drives business-time queries and partitioning (`toYYYYMM(start_time)` at `0039_create_events_full.up.sql:113`); server time is captured as `event_ts` (the ReplacingMergeTree version vector, set to now on every merge at `IngestionService/index.ts:1210`) and `created_at`, which is deliberately preserved across updates (`index.ts:834-835`). Out-of-order delivery is mitigated by sorting merges by event timestamp (`index.ts:1215-1228`), a UTC-midnight ingestion delay (`processEventBatch.ts:76-91`), and partition locking via `s3_first_seen_timestamp` (`index.ts:1085-1099`). Lifecycle freshness is enforced by retention jobs (`dataRetentionQueue.ts:11-41`) and media expiry cleanup (`media-retention-cleaner/index.ts:124-133`). Gap found: within the reviewed code paths there is no upper-bound clamp rejecting implausibly skewed client timestamps.

**3. Is trust level indicated?**
Partially — via enums and namespace fencing rather than a general trust field. Scores distinguish `API`/`ANNOTATION` from internally-reserved `EVAL` (`scores.ts:13-21`), and annotation scores are validated against a real score config including rejection of archived configs (`validateAndInflateScore.ts:31-33,218-222`). Observations carry severity levels (`types.ts:18`). The most sophisticated mechanism is the reserved `langfuse-*` environment namespace: user-supplied environments have the prefix stripped idempotently to prevent impersonating internal evaluator traces (`types.ts:228-246` with an explanatory comment about closing the bypass), while internal writers use a separate schema that keeps it (`types.ts:248-258`, `queues.ts:71-77`). Auth scopes limit which keys may write traces vs scores (`processEventBatch.ts:420-436`). However, there is no per-record authority/trust weighting for arbitrary context content (e.g., retrieved documents inside `input` are just opaque JSON).

**4. Are transformations traceable?**
Mostly yes, by construction: raw payloads are archived verbatim to S3 before any transformation and every derived row keeps a `blob_storage_file_path` back-pointer (`processEventBatch.ts:283-292`, `index.ts:457`), enabling replay after failures (referenced replay script at `otelIngestionQueue.ts:529-532`). Server-computed fields never overwrite user-provided ones because they live in parallel `provided_*` columns (`0039_create_events_full.up.sql:41-44`). Structural transformations are self-describing: media extraction embeds content-type, ID, and extraction origin in the replacement string (`OtelMediaProcessor.ts:236`); oversized-field overflow tags references with `source=field_size_limit` (`processObservationFieldOverflow.ts:257`); un-transformable metadata emits a typed metric instead of failing silently (`metadataDropped.test.ts:120-131`). The notable exception: the EE masking callback rewrites payloads pre-storage but persists no per-record marker that masking occurred — evidence exists only as metrics (`applyIngestionMasking.ts:170-193`) and logs; consumers cannot tell masked from unmasked rows by inspecting them.

## Architectural Decisions

1. **Append-only event sourcing with merge-on-write.** Ingestion accepts create/update events against entity IDs, archives every raw event to blob storage, and materializes state by merging event batches with any existing ClickHouse row under immutable-key protection (`processEventBatch.ts:201-273`, `IngestionService/index.ts:1192-1213`). This makes provenance durable (raw log retained) at the cost of read-before-write complexity.
2. **Attribution captured once per request, threaded through queues.** Attribution is computed from authenticated headers at the API boundary and transported in queue payloads rather than recomputed by workers (`processEventBatch.ts:394-396`), with optional fields for rolling-deploy compatibility (`queues.ts:30-34`).
3. **Reserved-namespace trust fencing.** Instead of ACLs on internal data, Langfuse reserves the `langfuse-*` environment prefix and normalizes public input to strip it, choosing schema-level enforcement over runtime permission checks (`types.ts:228-246`).
4. **Provided-vs-derived duality.** All enrichment (token counts, costs, model resolution, pricing tier) writes to distinct computed columns, leaving client claims intact (`getGenerationUsage` at `IngestionService/index.ts:1265-1383`).

## Notable Patterns

- **Self-describing inline references**: the `@@@langfuseMedia:type=X|id=Y|source=Z@@@` magic string encodes provenance inside the serialized I/O JSON itself, so provenance survives even naive serialization paths (`packages/shared/src/utils/mediaReferences.ts:6-9`, documented publicly in `fern/apis/server/definition/commons.yml:689`).
- **Idempotency-first normalization**: environment normalization is intentionally idempotent because values pass through the schema twice on the OTel path (comment at `types.ts:230-237`).
- **Typed drop telemetry**: transformation failures produce categorized metrics (`reason/source/domain` labels) with exactly-once counting tests spanning pipelines (`metadataDropped.test.ts:288-505`), rather than silent data loss.
- **Content-addressed media integrity**: media IDs derive from SHA-256 hashes and collision attempts fail loudly (`mediaService.ts:17-22,99-103`).

## Tradeoffs

- **Durability vs latency**: blocking S3 upload of raw events before queueing (`processEventBatch.ts:279-332`) buys replayability and auditability at the cost of ingestion latency and an extra failure surface (mitigated by slowdown tracking and secondary queues).
- **Compatibility vs completeness**: attribution and flags are `optional()` in queue schemas for in-flight jobs during deploys (`queues.ts:30-34,52-70`), meaning some records legitimately lack provenance fields.
- **Flattened metadata**: nested metadata is flattened into parallel `names`/`values` string arrays for ClickHouse performance (`IngestionService/index.ts:358-365`), trading away nested typing/fidelity of the original structure (original remains only in the blob archive).
- **Fail-open default for masking**: unless operators set fail-closed, a broken masking callback silently processes unmasked data (logged only, `applyIngestionMasking.ts:210-215`) — availability over confidentiality by default.

## Failure Modes / Edge Cases

- **Legacy/v4 provenance skew**: records written purely on the legacy observations path lack SDK/API-key attribution columns; consumers querying old tables cannot recover what the events table exposes (migration comparison `0035` vs `0042`).
- **Masking invisibility**: a masked record is indistinguishable from an unmasked one at query time; forensic review must correlate metrics/logs and raw S3 objects (`otelIngestionQueue.ts:527-542`).
- **Server-clock version ordering**: because `mergeRecords` stamps `event_ts = now` (`index.ts:1210`), last-write-wins resolution follows worker processing order, which can diverge from true event order for heavily delayed batches (partially mitigated by sortBatch and midnight delay windows).
- **Client-trusted timestamps**: event/body timestamps come from clients (`types.ts:43,427`); no skew clamp was found in reviewed code, so future-dated events would sort oddly and affect min-timestamp lookups (`index.ts:802-810`).
- **Silent score-source default**: omitted score `source` defaults to `"API"` (`validateAndInflateScore.ts:107`) and `ANNOTATION` self-declaration is accepted from any project-scoped key — trust labels are declarative, not attested.
- **Internal wrapper traces**: observations without a traceId get a synthetic wrapper trace created automatically (`index.ts:1058-1077`), which preserves linkage but fabricates an ancestor the producer did not declare.

## Future Considerations

- Add ingestion-attribution columns (or a unified provenance view joining staging/events) for legacy traces/observations so provenance queries do not depend on table generation.
- Persist a masking/redaction indicator per record (e.g., a `masked` boolean or transformation-version column on `events_full`) so downstream consumers can reason about data fidelity directly.
- Consider bounded clock-skew handling for client timestamps (clamp or flag outliers) to strengthen freshness guarantees.
- Generalize the reserved-environment pattern into an explicit trust/authority field if third-party evaluator outputs ever need to coexist with internal EVAL scores.

## Questions / Gaps

- No evidence found for a general per-item trust/authority rating beyond the score `source` enum and severity levels; searches across `packages/shared/src/domain/*` and the events schema surfaced only those enums (`scores.ts:4-11`, `types.ts:18`).
- No evidence found of per-record transformation-history chains (e.g., "this input was summarized from X") beyond raw-blob back-pointers and `provided_*` pairs; the blob archive is the only full history, and no UI/code path reconstructs per-event diffs was found in this pass.
- Whether client timestamp skew is clamped somewhere outside the reviewed ingestion path (e.g., in the OTel header-validation layer) could not be confirmed; searched `packages/shared/src/server/ingestion/*` and `worker/src/services/IngestionService/*` without finding bounds checks.
- Prompt-experiment/experiment lineage fields exist on events (`experiment_item_id`, `experiment_dataset_id`, etc., `definitions.ts:522-533`), but their end-to-end write coverage versus the legacy dataset-run-items table was not fully traced in this analysis.

---

Generated by `dimensions/11.04-context-provenance-and-integrity` against `langfuse`.
