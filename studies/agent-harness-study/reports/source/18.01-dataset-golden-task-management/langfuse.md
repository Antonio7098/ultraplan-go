# Source Analysis: langfuse

## Dimension 18.01 — Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (`web/`), BullMQ worker (`worker/`), shared domain package (`packages/shared/`), Postgres (Prisma) + ClickHouse + Redis |
| Analyzed | 2026-08-26 |

## Summary

Langfuse is an LLM engineering platform, and "datasets" are a first-class product feature rather than an internal test fixture: golden-task datasets (`Dataset`), golden examples (`DatasetItem` with `input` / `expectedOutput` / `metadata`), and benchmark runs/experiments (`DatasetRuns` + ClickHouse `dataset_run_items`) are all persistent, API-exposed entities.

The dataset model has four pillars relevant to this dimension:

1. **Golden answers + schema enforcement**: every item carries an optional `expectedOutput`; datasets can additionally declare JSON Schemas for `input` and `expectedOutput` that are compiled once per operation (Ajv) and enforced on every write path, including bulk CSV upload (`packages/shared/src/server/repositories/dataset-items.ts`, `packages/shared/src/server/services/DatasetService/*`).
2. **Temporal item versioning**: item writes are bitemporal-style row versions keyed by `validFrom`/`validTo` with soft-delete markers, enabling "items as of timestamp T" queries, per-item version history, drift counts since a version, and a UI version-history panel. A dual-implementation strategy (`STATEFUL` vs `VERSIONED`) is selected by env flags that currently default to the versioned implementation.
3. **Run-time snapshots**: when a run item is created, ingestion enriches it in ClickHouse with immutable denormalized copies of the run config *and* the dataset item content at the resolved version (`dataset_item_input`, `dataset_item_expected_output`, `dataset_item_metadata`, `dataset_item_version`). This decouples historical experiment results from later dataset edits.
4. **Experiment reproducibility metadata**: experiments record prompt id, provider, model, model params, structured-output schema, and the pinned `dataset_version` in run metadata; a stable deterministic experiment id is derived from `(projectId, datasetId, runName)`; the worker executes against items fetched at the pinned version.

Reproduction of a benchmark result six months later is well supported at the data layer: you can re-materialize the exact item set via point-in-time queries or read the stored snapshots, and re-run with the recorded model/prompt config. What is *not* provided server-side is a golden-answer scoring harness (comparison of outputs vs expectedOutput) — scoring is delegated to Langfuse's separate evals/scores subsystem — and there is no built-in difficulty/category task taxonomy beyond free-form JSON `metadata`.

## Rating

**8 / 10** — Clear, tested model with explicit public interfaces and operational safeguards. Temporal versioning with point-in-time reads, immutable run-time snapshots, schema-enforced golden answers, migration/backfill tooling, and extensive integration tests put this solidly in the 7–8 band. Not 9–10 because: (a) only *items* are versioned — the dataset container (name, description, schemas, metadata) mutates in place with no history; (b) no structured task-metadata taxonomy (difficulty/category) exists, only free-form JSON; (c) the legacy `STATEFUL` code path remains alongside `VERSIONED`, doubling surface area during migration; and (d) end-to-end reproducibility still depends on the caller to pin versions and supply non-deterministic model credentials.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dataset entity: name unique per project, free-form `metadata`, remote-experiment webhook fields | `Dataset` Prisma model with `@@unique([projectId, name])` | `packages/shared/prisma/schema.prisma:561-591` |
| Golden answer field: `expectedOutput` plus `input`/`metadata` on each item | `DatasetItem` model columns | `packages/shared/prisma/schema.prisma:593-620` |
| Item status taxonomy limited to ACTIVE/ARCHIVED | `enum DatasetStatus` | `packages/shared/prisma/schema.prisma:622-625` |
| Run entity: unique run name per dataset+project, `metadata` Json | `DatasetRuns` model | `packages/shared/prisma/schema.prisma:627-644` |
| Version columns: `validFrom`, `validTo`, `isDeleted`, composite PK `[id, projectId, validFrom]` | "dataset version cols" block | `packages/shared/prisma/schema.prisma:606-613` |
| Dataset-level JSON Schema declarations for input and expected output | `inputSchema` / `expectedOutputSchema` columns | `packages/shared/prisma/schema.prisma:578-579`; contract docs `fern/apis/server/definition/datasets.yml:88-93` |
| Dual write/read implementation strategy (STATEFUL vs VERSIONED) | `executeWithDatasetServiceStrategy` dispatching on env flags | `packages/shared/src/server/datasets/executeWithDatasetServiceStrategy.ts:18-41` |
| Env flags default to VERSIONED for both writes and reads | `LANGFUSE_DATASET_SERVICE_WRITE_TO_VERSIONED_IMPLEMENTATION` / `..._READ_FROM_...` both `.default("true")` | `packages/shared/src/env.ts:546-551` |
| Versioned upsert: invalidate current row then insert new version with monotonic `validFrom` (`Math.max(Date.now(), baseTs + 1)`) | `upsertDatasetItem` VERSIONED branch | `packages/shared/src/server/repositories/dataset-items.ts:430-487` |
| Delete as tombstone: set `validTo` + create `isDeleted: true` marker row; media link rows intentionally preserved for history | `deleteDatasetItem` VERSIONED branch incl. comment | `packages/shared/src/server/repositories/dataset-items.ts:541-575` |
| Point-in-time query: temporal predicate `valid_from <= version AND (valid_to IS NULL OR valid_to > version)` | `buildDatasetItemsAtVersionQuery` | `packages/shared/src/server/repositories/dataset-items.ts:1260-1336` |
| Per-item version history listing distinct `valid_from` values | `getDatasetItemVersionHistory` | `packages/shared/src/server/repositories/dataset-items.ts:1965-1990` |
| Drift count since a version (upserts/deletes) used by warning banner | `getDatasetItemChangesSinceVersion` | `packages/shared/src/server/repositories/dataset-items.ts:1998-2030` |
| Resolve which dataset version a past run used (max `dataset_item_version`, falling back to max `created_at` for pre-versioning runs) | `getDatasetVersionForRun` | `packages/shared/src/server/repositories/dataset-items.ts:1893-1956` |
| Public REST list endpoint accepts `version` datetime param ("returns state of dataset at this timestamp"), requires `datasetName` with it | Fern contract for GET /dataset-items | `fern/apis/server/definition/dataset-items.yml:23-47` |
| Public run-item creation resolves the item *at the requested version* before linking | `createDatasetRunItemForApi` calls `getDatasetItemById({..., version})` | `web/src/features/datasets/server/publicDatasetService.ts:848-857` |
| Run item event stamps `datasetVersion = datasetItem.validFrom` | DATASET_RUN_ITEM_CREATE body construction | `web/src/features/datasets/server/publicDatasetService.ts:898-913` |
| ClickHouse run-item table denormalizes immutable run fields and mutable-but-snapshotted item fields | `dataset_run_items` DDL: `dataset_run_name/metadata`, `dataset_item_input`, `dataset_item_expected_output`, ReplacingMergeTree | `packages/shared/clickhouse/migrations/unclustered/0022_dataset_run_items.up.sql:1-36` |
| Per-row dataset item version column added to snapshots | `ADD COLUMN dataset_item_version Nullable(DateTime64(3))` | `packages/shared/clickhouse/migrations/unclustered/0032_add_dataset_version_to_dataset_run_items_rmt.up.sql:1` |
| Ingestion enrichment: snapshot item IO + version into ClickHouse at run time | `processDatasetRunItemEventList` builds record with `dataset_item_input/expected_output/metadata/version` from item fetched at `event.body.datasetVersion` | `worker/src/services/IngestionService/index.ts:510-602` |
| Experiment worker fetches items at pinned version and deduplicates per run | `getItemsToProcess` uses `getDatasetItems({..., version: config.datasetVersion})` | `worker/src/features/experiments/experimentServiceClickhouse.ts:238-296` |
| Experiment creation records reproducibility config in run metadata: prompt_id, provider, model, model_params, structured_output_schema, dataset_version | `createExperiment` mutation metadata assembly | `web/src/features/experiments/server/router.ts:255-280` |
| Deterministic stable experiment id: sha256 over `["langfuse-experiment-v1", projectId, datasetId, runName]` truncated to 16 hex chars | `createStableExperimentId` | `web/src/features/datasets/server/publicDatasetService.ts:757-772` |
| Schema validation engine: compile-once Ajv validators for input and expectedOutput | `DatasetSchemaValidator` class | `packages/shared/src/server/services/DatasetService/DatasetSchemaValidator.ts:23-91` |
| Normalization + validation pipeline for item payloads (string/object tolerant, control-char sanitization, null handling) | `DatasetItemValidator.validateAndNormalize` | `packages/shared/src/server/services/DatasetService/DatasetItemValidator.ts:50-271` |
| Bulk create with all-or-nothing or partial success, index-preserving errors for CSV rows | `createManyDatasetItems` | `packages/shared/src/server/repositories/dataset-items.ts:604-917` |
| Cross-dataset id conflict guard (item ids unique per project across datasets) | conflict checks in `upsertDatasetItem` | `packages/shared/src/server/repositories/dataset-items.ts:321-326, 445-449` |
| Integration test proving version query semantics (v1 data retrievable after v2 update; version requires datasetName) | test "should support dataset versioning via version query parameter" | `web/src/__tests__/server/datasets-api.servertest.ts:1873-1986` |
| Integration test proving runs can be created at a specific version and store `dataset_version` in run metadata | test "should support creating experiment runs at specific dataset version" | `web/src/__tests__/server/datasets-api.servertest.ts:1988-2062` |
| Backfill migration tests for version-chain integrity (LEAD() ordering, final state where only current versions have null `valid_to`) | `BackfillValidToForDatasetItems` suite | `worker/src/__tests__/backfillValidToForDatasetItems.test.ts:100-380` |
| Schema-validation test suite (unit + public API enforcement) | `datasets-schema-validation.servertest.ts` describes | `web/src/__tests__/server/datasets-schema-validation.servertest.ts:96-499+` |
| UI version browsing: grouped version-history panel, stale-version warning banner, diff view | components | `web/src/features/datasets/components/DatasetVersionHistoryPanel.tsx:24-41`, `DatasetVersionWarningBanner.tsx`, `DatasetItemDiffView.tsx` |
| tRPC exposes version-aware reads to UI: `countChangesSinceVersion`, `itemsByDatasetId(version)`, `runById` resolves `datasetVersion` | procedures | `web/src/features/datasets/server/dataset-router.ts:585-623, 1075-1126` |
| v4 Experiments API exports snapshots: `experimentItemVersion`, `expectedOutput`, `experimentItemMetadata` | Fern `ExperimentItem` type | `fern/apis/server/definition/experiments.yml:153-193` |
| Remote experiment trigger (signed webhook per dataset) for out-of-process benchmark execution | `remote_experiment_*` columns + helpers | `packages/shared/prisma/schema.prisma:567-577`; `web/src/features/datasets/server/remoteExperimentHelpers.ts:208-233` |
| Performance-seeding of versioned data (500 items × multiple versions) for reproducible perf testing | `seedDatasetVersions` seeder | `packages/shared/scripts/seeder/seed-dataset-versions.ts:26-100` |
| Deletion safeguard: queued ClickHouse cleanup drops snapshot rows and media links per dataset/run | `processClickhouseDatasetDelete` | `worker/src/features/datasets/processClickhouseDatasetDelete.ts:10-55` |

## Answers to Dimension Questions

### 1. How are datasets managed?

Datasets are managed as a full product surface, not ad-hoc fixtures:

- **Persistence**: three Postgres entities — `Dataset`, `DatasetItem`, `DatasetRuns` (`packages/shared/prisma/schema.prisma:561-644`) — plus a ClickHouse `dataset_run_items_rmt` table for run-item snapshots.
- **Write paths converge on one repository**: all CRUD goes through `createDatasetItem` / `upsertDatasetItem` / `createManyDatasetItems` / `deleteDatasetItem` in `packages/shared/src/server/repositories/dataset-items.ts:226-579, 604-917`, which enforce schema validation, media-reference resolution, and cross-dataset id-uniqueness before persisting.
- **Surfaces**: public REST v2 API (`fern/apis/server/definition/datasets.yml`, `fern/apis/server/definition/dataset-items.yml`), tRPC UI routers (`web/src/features/datasets/server/dataset-router.ts`), MCP tools (`web/src/features/mcp/features/datasets/tools/*`), batch actions that convert observations into items (`worker/src/features/batchAction/processAddObservationsToDataset.ts`), and CSV upload with per-row error mapping (`createManyDatasetItems`, `packages/shared/src/server/repositories/dataset-items.ts:592-604`).
- **Provenance**: items optionally carry `sourceTraceId`/`sourceObservationId` (`packages/shared/prisma/schema.prisma:600-601`) so golden tasks can be traced back to the production trace they were captured from.

### 2. Are datasets versioned?

Yes — but specifically the *items*, not the dataset container:

- Every `DatasetItem` row is a temporal version: composite PK `[id, projectId, validFrom]` with `validTo` and `isDeleted` (`packages/shared/prisma/schema.prisma:606-613`). Updates close the old row's `validTo` and insert a new version; deletes insert a tombstone marker (`packages/shared/src/server/repositories/dataset-items.ts:430-487, 541-575`).
- Reads accept a `version` timestamp: SQL predicate `valid_from <= T AND (valid_to IS NULL OR valid_to > T)` (`packages/shared/src/server/repositories/dataset-items.ts:1306-1311`), exposed through the public API (`fern/apis/server/definition/dataset-items.yml:35-40`) and verified by tests (`web/src/__tests__/server/datasets-api.servertest.ts:1873-1986`).
- Auxiliary introspection APIs exist: per-item version history (`getDatasetItemVersionHistory`, `packages/shared/src/server/repositories/dataset-items.ts:1965-1990`), change counts since a version (`getDatasetItemChangesSinceVersion`, same file `1998-2030`), and run→version resolution (`getDatasetVersionForRun`, same file `1893-1956`).
- Migration hygiene: a background backfill populates `validTo` chains for pre-existing data with its own correctness tests (`worker/src/backgroundMigrations/backfillValidToForDatasetItems.ts`, `worker/src/__tests__/backfillValidToForDatasetItems.test.ts:242-380`).
- **Gap**: `Dataset` itself (name, description, `metadata`, `inputSchema`, `expectedOutputSchema`) is mutated in place with only `updatedAt` (`packages/shared/prisma/schema.prisma:581-582`); there is no schema-change audit or version pinning of the *validation rules* that applied to historical writes.

### 3. Are expected outputs defined?

Yes, at two levels:

- **Per item**: `expectedOutput` is a nullable JSON column (`packages/shared/prisma/schema.prisma:598`), exposed on the public API (`fern/apis/server/definition/commons.yml:662-664`) and snapshotted into run items (`worker/src/services/IngestionService/index.ts:584-586`).
- **Per dataset (optional contract)**: `expectedOutputSchema` (and `inputSchema`) hold JSON Schemas validated on every item write via compile-once Ajv validators (`packages/shared/src/server/services/DatasetService/DatasetSchemaValidator.ts:33-39`; `fern/apis/server/definition/datasets.yml:91-93` documents the retro-active intent). Validation errors are field-scoped with JSON-path detail (`DatasetItemValidator.validateAndNormalize`, `packages/shared/src/server/services/DatasetService/DatasetItemValidator.ts:241-262`), and the suite covers enum violations, `additionalProperties: false`, null-handling, and unsafe-number preservation (`web/src/__tests__/server/datasets-schema-validation.servertest.ts:163-397`).
- **Consumption**: expected outputs flow into experiment items (`itemExpectedOutput` in the experiment trace context, `worker/src/features/experiments/experimentServiceClickhouse.ts:200-202`) and the v4 Experiments export API (`fern/apis/server/definition/experiments.yml:176-178`). However, no server-side comparator computes pass/fail against the golden output — scoring is owned by the separate evals/scores subsystem.

### 4. Are benchmarks reproducible?

Substantially yes, with defined limits:

- **Data reproducibility**: run items persist immutable snapshots of the exact item content and version used (`0022_dataset_run_items.up.sql:18-27`; `worker/src/services/IngestionService/index.ts:556-590`), so results remain interpretable even after the dataset is edited. Point-in-time re-reads are also available directly from Postgres.
- **Config reproducibility**: experiments store prompt id, provider, model, model params, structured-output schema, and pinned `dataset_version` in run metadata (`web/src/features/experiments/server/router.ts:255-280`); the worker fetches items at that version (`worker/src/features/experiments/experimentServiceClickhouse.ts:245-253`). Run identity is deterministic given `(projectId, datasetId, runName)` (`createStableExperimentId`, `web/src/features/datasets/server/publicDatasetService.ts:757-772`), which keeps multi-request runs coherent without persistence.
- **Operational reproducibility**: version-perf scenarios can be regenerated deterministically via the seed CLI (`packages/shared/scripts/seeder/seed-dataset-versions.ts`), and the repo convention mandates seeded scenarios over ad-hoc scripts (root `AGENTS.md` seeding rules).
- **Limits**: LLM inference itself is non-deterministic (temperature etc. are recorded but not forced); SDK-side runs depend on callers passing `datasetVersion` (omitting it silently uses latest); and there is no built-in golden-answer assertion runner that would make "benchmark score" itself a reproducible artifact. The deprecation of v3 dataset-run endpoints in favor of the v4 Experiments API (`fern/apis/server/definition/datasets.yml:36-76`) also means reproduction tooling must track the new endpoint surface.

## Architectural Decisions

1. **Temporal row versioning over append-only log or snapshot tables.** Item versions live in one table distinguished by `validFrom`/`validTo` (`packages/shared/prisma/schema.prisma:606-613`). This makes point-in-time reads a single indexed query (`packages/shared/src/server/repositories/dataset-items.ts:1306-1335`) at the cost of growing row counts and requiring deduplication guards during migration transitions (`packages/shared/src/server/repositories/dataset-items.ts:1491-1508`).

2. **Strategy pattern for a live storage migration.** Every dataset operation branches between `STATEFUL` and `VERSIONED` implementations selected by env flags (`packages/shared/src/server/datasets/executeWithDatasetServiceStrategy.ts:18-41`, defaults at `packages/shared/src/env.ts:546-551`). This allows flipping deployments to versioned semantics without downtime, but leaves two parallel implementations in the hot path.

3. **Snapshot-at-ingestion for run results.** Rather than joining back to mutable Postgres data, run items embed denormalized copies of run and item fields into ClickHouse (`packages/shared/clickhouse/migrations/unclustered/0022_dataset_run_items.up.sql:18-27`; enrichment at `worker/src/services/IngestionService/index.ts:574-589`). Historical dashboards therefore never change retroactively.

4. **Schema validation pushed to the storage boundary.** All writers (REST, tRPC, MCP, CSV) funnel through repository functions that validate merged state against dataset schemas (`packages/shared/src/server/repositories/dataset-items.ts:337-358`), including merge-with-existing semantics so partial updates cannot bypass contracts (`mergeItemData`, same file `184-216`).

5. **Version resolution delegated to timestamps, not sequence numbers.** A "version" is a UTC timestamp (`validFrom`), user-suppliable on run-item creation (`fern/apis/server/definition/dataset-run-items.yml:53-59`), with monotonicity enforced by `Math.max(Date.now(), baseTs + 1)` (`packages/shared/src/server/repositories/dataset-items.ts:451-452`).

## Notable Patterns

- **Compile-once validators with explicit perf claims**: `DatasetSchemaValidator`/`DatasetItemValidator` document a measured ~3800x speedup from reusing compiled Ajv functions across bulk items (`packages/shared/src/server/services/DatasetService/DatasetItemValidator.ts:38-39`), paired with span tags tracking batch outcomes (`langfuse.dataset_items.create.*`, `packages/shared/src/server/repositories/dataset-items.ts:788-794`).
- **Index-preserving bulk errors**: CSV/API batch failures carry `itemIndex` mapping back to source rows (`CreateManyValidationError`, `packages/shared/src/server/repositories/dataset-items.ts:957-967`).
- **Deterministic ids for distributed coherence**: `createStableExperimentId` hashes run identity so concurrent POSTs converge on one experiment id without locks (`web/src/features/datasets/server/publicDatasetService.ts:742-772`), complementing the DB-level `createOrFetchDatasetRun` race guard (`packages/shared/src/server/repositories/dataset-runs.ts:30-97`).
- **UI-first-class version UX**: version history panel groups versions by day, diff views compare item versions, and a warning banner surfaces drift since the viewed version (`web/src/features/datasets/components/DatasetVersionHistoryPanel.tsx`, `DatasetItemDiffView.tsx`, `DatasetVersionWarningBanner.tsx`).
- **Contract-first public API**: Fern YAML definitions are the source of truth for generated clients, with deprecation messaging baked into the contract (`fern/apis/server/definition/datasets.yml:36-76`).

## Tradeoffs

- **Snapshot duplication vs immutability**: copying item IO into every run item inflates ClickHouse storage (ZSTD-compressed strings, `0022_dataset_run_items.up.sql:25-27`) in exchange for permanently stable historical results.
- **Dual implementation vs migration risk**: keeping `STATEFUL` paths reduces upgrade risk for self-hosters but doubles code and test surface; several functions return degenerate constants in STATEFUL mode (e.g., empty version history, `packages/shared/src/server/repositories/dataset-items.ts:1971-1974`), so behavior silently differs by deployment flag.
- **Timestamp versions vs explicit version labels**: timestamps require no allocation protocol and support arbitrary point-in-time reads, but humans cannot reference "v3" of a dataset; the UI compensates by grouping timestamps (`DatasetVersionHistoryPanel.tsx`).
- **Retroactive schema application vs write stability**: setting a dataset schema validates "all new and existing" items per the contract doc (`fern/apis/server/definition/datasets.yml:90`), which strengthens guarantees but means tightening a schema can invalidate previously acceptable golden data.
- **Free-form metadata vs queryability**: `metadata` as untyped JSON on dataset/item/run (`packages/shared/prisma/schema.prisma:566, 599, 632`) maximizes flexibility but forfeits filtering/indexing (Prisma path explicitly skips metadata filters, `packages/shared/src/server/repositories/dataset-items.ts:1130-1131`).

## Failure Modes / Edge Cases

- **Concurrent updates to one item**: guarded by transactional re-read of the current version and monotonic `validFrom` computation (`packages/shared/src/server/repositories/dataset-items.ts:434-452`); duplicate-null-`validTo` rows possible mid-migration are deduplicated in application code (`same file 1491-1508`).
- **Clock skew / same-timestamp writes**: mitigated by `baseTs + 1` floor, not eliminated — two writers on skewed clocks could interleave versions.
- **Cross-dataset id collisions**: item ids are project-unique; conflicts raise `LangfuseConflictError` with actionable copy naming the owning dataset (`packages/shared/src/server/repositories/dataset-items.ts:321-326`).
- **Pre-versioning runs**: `getDatasetVersionForRun` falls back to max run-item `created_at` when `dataset_item_version` is absent, explicitly commented for "experiments that ran before dataset item versioning was introduced" (`packages/shared/src/server/repositories/dataset-items.ts:1929-1938`).
- **Partial bulk failures**: default is atomic rejection of the whole batch; `allowPartialSuccess` flips to per-item outcomes with accurate success/failed counts (`packages/shared/src/server/repositories/dataset-items.ts:796-804, 899-916`).
- **Unresolvable media references** in item IO are treated as validation failures mapped to the offending row/JSON path (`same file 745-786`).
- **Control-character poisoning of Postgres TEXT**: C0/C1 chars stripped during normalization (`DatasetItemValidator.cleanControlChars`, `packages/shared/src/server/services/DatasetService/DatasetItemValidator.ts:70-99`).
- **Run-item events referencing missing runs/items** are dropped silently (`if (!runData || !itemData) return []`, `worker/src/services/IngestionService/index.ts:550`) — a potential quiet-loss path if ordering races occur.

## Future Considerations

- Complete the `STATEFUL` → `VERSIONED` retirement: remove dual paths once self-hosters have migrated (the flags at `packages/shared/src/env.ts:546-551` and branches throughout `packages/shared/src/server/repositories/dataset-items.ts` are scaffolding for this).
- Version the dataset container itself (schemas, name, metadata) or add an audit log, so a historical run can be matched not just to historical item content but to the validation contract in force at the time.
- Add optional structured task-taxonomy fields (e.g., difficulty/category tags) or first-class dataset tags to enable stratified benchmark reporting; today this lives, if at all, inside untyped `metadata`.
- Provide a server-side golden-answer comparison utility (exact/fuzzy/LLM-judged) so benchmark scoring becomes a reproducible platform artifact instead of an external eval-config concern.
- Ship a documented "reproduce this experiment" recipe (API sequence: resolve `datasetVersion` from run metadata → GET items at version → replay with recorded model params) to operationalize six-month-later reproduction for API consumers.

## Questions / Gaps

- **No evidence found** for any built-in difficulty/category/classification metadata schema: searched `difficulty` across `web/src/features/datasets` and `packages/shared/src` (zero hits), and inspected `Dataset`/`DatasetItem`/`DatasetRuns` models — metadata is uniformly free-form JSON (`packages/shared/prisma/schema.prisma:566, 599, 632`).
- **No evidence found** for dataset-level immutability controls (e.g., freeze/pin a dataset version under a label). The only "pinning" is passing raw ISO timestamps; searched fern definitions and the dataset-router for named-version or tag concepts.
- The Python/JS SDK experiment runners are outside this source tree (separate SDK repos), so client-side ergonomics of pinning `datasetVersion` could not be verified here; server acceptance of the parameter is confirmed at `fern/apis/server/definition/dataset-run-items.yml:53-59`.
- Whether `expectedOutputSchema` changes re-validate existing items eagerly (the Fern doc implies intent, `fern/apis/server/definition/datasets.yml:90`) was not traced to an enforcement job in this repo; observed enforcement is at item-write time only (`packages/shared/src/server/repositories/dataset-items.ts:337-358`).

---

Generated by `18.01-dataset-and-golden-task-management` against `langfuse`.
