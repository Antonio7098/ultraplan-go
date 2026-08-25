# Source Analysis: langfuse

## 10.03 Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (`web/`), BullMQ worker (`worker/`), shared contracts (`packages/shared/`), Postgres (Prisma) + ClickHouse + Redis + S3 |
| Analyzed | 2026-08-25 |

## Summary

Langfuse is an LLM observability platform, so causal lineage is the product's core data model rather than a bolted-on concern. Every ingested record is written with explicit foreign-key-style columns linking it to its causal context: observations carry `trace_id` and `parent_observation_id` for the run tree (`packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:3-6`), generations carry `prompt_id`/`prompt_name`/`prompt_version` resolved server-side against the prompt registry (`worker/src/services/IngestionService/index.ts:320-333`, `405-408`), model attribution keeps both the user-provided name and the matched internal model row (`provided_model_name` vs `internal_model_id`, `0002_observations.up.sql:16-17`), and tool definitions/calls are extracted into dedicated queryable columns (`packages/shared/clickhouse/migrations/unclustered/0033_add_tool_call_columns.up.sql:1-3`). Scores, dataset runs, media artifacts, eval executions, and approvals all carry explicit links back to the traces/observations that produced or targeted them. The next-generation `events_core` table formalizes this further with dedicated Prompt, Model, Tool, Experiment, and Instrumentation-source column groups (`packages/shared/clickhouse/migrations/unclustered/0040_create_events_core.up.sql:30-90`). The main gaps are heuristic (not schema-validated) tool extraction from unstructured I/O, convention-based retrieval provenance (a RETRIEVER span type without structured document-source fields), and the absence of cross-store referential integrity between ClickHouse lineage columns and their Postgres counterparts.

## Rating

**8 / 10.**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band):** Lineage fields are first-class columns in both ingestion zod schemas (`packages/shared/src/server/ingestion/types.ts:493-500` for prompt name/version pairs; `531-546` for score references including `traceId`, `observationId`, `datasetRunId`, `queueId`, `executionTraceId`) and ClickHouse DDL (`0002_observations.up.sql:25-27`; `0024_dataset_run_items.up.sql:1-36`). Provenance through transformations is preserved by design: raw (`provided_*`) vs computed (`usage_details`, `cost_details`) values are stored side-by-side (`0002_observations.up.sql:19-23`; enrichment at `worker/src/services/IngestionService/index.ts:420-428`), and dataset run items denormalize immutable item snapshots explicitly commented "snapshots are relevant" (`0024_dataset_run_items.up.sql:18-27`).
- **Operational safeguards:** idempotent ReplacingMergeTree writes keyed on `event_ts`/`is_deleted` (`0001_traces.up.sql:23`), background backfill migrations that rebuild lineage columns when schemas evolve (`worker/src/backgroundMigrations/backfillEventsFullFromObservations.ts`, `worker/src/backgroundMigrations/backfillEventsFullFromDatasetRunItems.ts`), and admin replay of raw ingested events (`web/src/pages/api/admin/ingestion-replay.ts`).
- **Why not 9–10:** tool-call extraction is shape-detection heuristics over free-form JSON rather than a guaranteed contract (`isToolCallLike` in `packages/shared/src/server/ingestion/extractToolsBackend.ts:189-225`); retrieval provenance has no structured document-source schema; `prompt_id` degrades to empty string on lookup miss (`worker/src/services/IngestionService/index.ts:406`); ClickHouse lineage columns have no FK integrity against Postgres registries.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Output↔input capture | `input`/`output` columns on every observation; ZSTD-compressed | packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:14-15 |
| Run-tree lineage | `trace_id` + `parent_observation_id` on observations; bloom-filter indexes for traversal | packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:3-6,33 |
| Prompt provenance | Ingestion schema requires `promptName`+`promptVersion` set together or not at all | packages/shared/src/server/ingestion/types.ts:493-500 |
| Prompt resolution | Worker resolves name+version to concrete `Prompt` row via `PromptService.getPrompt` before write | worker/src/services/IngestionService/index.ts:320-333 |
| Prompt link columns | `prompt_id`, `prompt_name`, `prompt_version` persisted on observation/event records | worker/src/services/IngestionService/index.ts:405-408 |
| Prompt dependency graph | `PromptDependency` rows link parent prompt → childName/childLabel/childVersion; `resolvePrompt` returns `resolutionGraph` | packages/shared/prisma/schema.prisma:750-768; packages/shared/src/server/services/PromptService/index.ts:132-147 |
| UI traceability of prompts | `PromptBadge` deep-links an observation to `/project/:id/prompts/:name?version=N` | web/src/features/traces/components/PromptBadge.tsx:6-29 |
| Model version tracking | `provided_model_name` (client claim) vs `internal_model_id` (matched pricing/model row) | packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:16-17; worker/src/services/IngestionService/index.ts:411-412 |
| Raw-vs-computed preservation | `provided_usage_details`/`provided_cost_details` kept beside enriched `usage_details`/`cost_details` | packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:19-23 |
| Tool definition/call columns | `tool_definitions` Map, `tool_calls` Array, `tool_call_names` Array added to observations | packages/shared/clickhouse/migrations/unclustered/0033_add_tool_call_columns.up.sql:1-3 |
| Tool extraction contract | Multi-format flattening of OpenAI/Anthropic/AI-SDK/LangChain tool definitions and calls; dedupe by call id | packages/shared/src/server/ingestion/extractToolsBackend.ts:63-120,783-789 |
| Heuristic detection | `isToolCallLike` shape-sniffs unstructured outputs; excludes tool-results | packages/shared/src/server/ingestion/extractToolsBackend.ts:189-225 |
| Retrieval span type | `RETRIEVER` observation type and `RETRIEVER_CREATE` ingestion event type exist | packages/shared/src/domain/observations.ts:12; packages/shared/src/server/ingestion/types.ts:290 |
| Score→action linkage | Scores carry `trace_id`, `observation_id`, `source` (API/EVAL/ANNOTATION), `config_id`, `queue_id`, `author_user_id` | packages/shared/clickhouse/migrations/unclustered/0003_scores.up.sql:5-15 |
| Annotation approval flow | `AnnotationQueueItem` binds objectId/objectType with annotatorUserId, lockedByUserId, status | packages/shared/prisma/schema.prisma:500-524 |
| Annotation config enforcement | ANNOTATION scores require `configId` (except CORRECTION) via shared predicate used by REST + ingestion | packages/shared/src/domain/scores.ts:30-40 |
| Agent tool approvals | `InAppAgentPendingToolApproval` keyed on (project, conversation, `toolCallId`) with `approvalFingerprint` | packages/shared/prisma/schema.prisma:315-331 |
| Approval decision audit | Decision events record `toolCallId`, `approved`, `decidedByUserId` in append-only event stream | packages/shared/src/in-app-agent/approvalEvents.ts:7-28 |
| Control-plane audit trail | `AuditLog` stores userId/apiKeyId, action, resourceType/resourceId, before/after JSON snapshots | packages/shared/prisma/schema.prisma:852-876; web/src/features/audit-logs/auditLog.ts:54-94 |
| Artifact→run association | Media association tables bind mediaId to trace/observation/datasetItem with `field` and origin enum | packages/shared/prisma/schema.prisma:1381-1438 |
| Media reference strings | `@@@langfuseMedia:type=…\|id=…\|source=…@@@` magic strings embed media ids inside I/O payloads | packages/shared/src/utils/IORepresentation/chatML/types.ts:27-41 |
| Dataset item provenance | `DatasetItem.sourceTraceId`/`sourceObservationId` + bitemporal `validFrom`/`validTo` versioning | packages/shared/prisma/schema.prisma:600-609,614-615 |
| Experiment run linkage | `dataset_run_items_rmt` binds run+item+trace+observation, denormalized snapshots, `dataset_item_version` | packages/shared/clickhouse/migrations/unclustered/0024_dataset_run_items.up.sql:1-36; 0032_add_dataset_version_to_dataset_run_items_rmt.up.sql:1 |
| Eval causal chain | `JobExecution`: jobInputTraceId/ObservationId/DatasetItemId(+ValidFrom) → jobOutputScoreId + executionTraceId | packages/shared/prisma/schema.prisma:1074-1109 |
| Instrumentation provenance | `source`, `service_name/version`, `scope_name/version`, `telemetry_sdk_*`, `ingestion_sdk_name/version`, `ingestion_api_key` columns | packages/shared/clickhouse/migrations/unclustered/0035_add_ingestion_attribution_columns.up.sql:1-8; 0040_create_events_core.up.sql:82-90; OTel population at packages/shared/src/server/otel/OtelIngestionProcessor.ts:624-634 |
| Public API exposure | Observations API returns `promptId/promptName/promptVersion` and `modelId` (matched internal model) | web/src/features/public-api/types/observations.ts:73-76,95,207 |

## Answers to Dimension Questions

**1. Can every output be traced to its inputs?**
Yes within the captured trace. Each observation persists its raw `input` and `output` payloads (`0002_observations.up.sql:14-15`), is anchored to its trace and parent span (`trace_id`, `parent_observation_id`, lines 3-6), and generation-type spans additionally persist which managed prompt (id, name, version) produced them (`worker/src/services/IngestionService/index.ts:405-408`). The public API re-exposes these links (`web/src/features/public-api/types/observations.ts:73-76`), and the UI renders a navigation link from any generation back to the exact prompt version (`web/src/features/traces/components/PromptBadge.tsx:17-20`). Caveat: tracing a *fact inside* an output to its originating input relies on the caller wrapping retrieval in RETRIEVER spans; Langfuse stores what is reported rather than deriving dataflow.

**2. Is provenance preserved through transformations?**
Largely yes, deliberately. Client-provided usage/cost are never overwritten: `provided_usage_details`/`provided_cost_details` coexist with server-enriched maps (`0002_observations.up.sql:19-23`; enrichment logic at `worker/src/services/IngestionService/index.ts:420-428`), so post-hoc repricing never destroys original claims. Dataset run items freeze the item content they ran against ("snapshots are relevant", `0024_dataset_run_items.up.sql:24-27`) plus `dataset_item_version` (`0032_add_dataset_version_to_dataset_run_items_rmt.up.sql:1`), while `DatasetItem` itself is bitemporally versioned (`validFrom`/`validTo`, `schema.prisma:607-609`). Audit logs keep before/after JSON for control-plane mutations (`schema.prisma:866-867`). Raw ingestion events can also be retained in blob storage per entity bucket and replayed (`packages/shared/src/server/ingestion/processEventBatch.ts:203-216`; `web/src/pages/api/admin/ingestion-replay.ts`).

**3. Are model versions tracked in lineage?**
Yes, two-sided. Observations store the client-declared `provided_model_name` alongside `internal_model_id` referencing the matched `Model` registry row used for pricing (`0002_observations.up.sql:16-17`; match/enrichment at `worker/src/services/IngestionService/index.ts:411-412`), plus serialized `model_parameters`. The v2 events schema extends this with instrumentation-level provenance — service name/version, scope name/version, telemetry SDK name/version (`0040_create_events_core.up.sql:82-90`), populated on the OTel path (`OtelIngestionProcessor.ts:624-634`). Pricing-tier attribution is also persisted (`usage_pricing_tier_id/name`, `worker/src/services/IngestionService/index.ts:427-428`). What is *not* tracked as lineage is which evaluator/template version produced EVAL scores at score-write time beyond `config_id`; the template version chain lives separately in `EvaluatorVersion` (`schema.prisma:931-955`) and is reachable only through JobExecution.

**4. Can causal chains be audited?**
Yes across four layers: (a) runtime tree — trace → observation tree via parent ids; (b) evaluation chain — scores link to trace/observation/config/queue (`0003_scores.up.sql:5-15`) and `JobExecution` closes the loop from input trace/dataset-item to output score and its own execution trace (`schema.prisma:1091-1103`); (c) experiment chain — dataset runs ↔ items ↔ traces ↔ observations with frozen snapshots (`0024_dataset_run_items.up.sql`); (d) human actions — annotation queue assignments/items record who annotated what (`schema.prisma:500-550`), in-app agent approvals bind decisions to specific `toolCallId`s with the deciding user (`approvalEvents.ts:10-14`), and admin mutations land in `AuditLog` with actor, action, resource, and before/after state (`auditLog.ts:54-94`). Gaps: no FKs from ClickHouse lineage columns to Postgres registries (dangling `prompt_id` possible after prompt deletion; comment "no fk constraint - deletion handled via project cascade" at `schema.prisma:1082,1091-1097`), and `AuditLog` covers only USER/API_KEY record types (`schema.prisma:846-849`), so ordinary trace edits (bookmark/rename) are not audited.

## Architectural Decisions

- **Lineage as denormalized columns, not a join table.** Rather than a generic triple-store, each causal relationship is materialized directly on the record (prompt columns, model columns, tool arrays, score references). This trades write-time coupling for O(1) filterable reads in ClickHouse (`0040_create_events_core.up.sql:30-56` groups these into named sections).
- **Server-side resolution of soft references.** Clients send weak references (`promptName`+`promptVersion`, model names); the worker resolves them to strong ids (`prompt_id`, `internal_model_id`) at ingestion time using cached lookups (`worker/src/services/IngestionService/index.ts:320-333`; cache layer at `packages/shared/src/server/services/PromptService/index.ts:70-81`). Both halves are stored, so the chain survives later renames.
- **Snapshot-on-reference for mutable artifacts.** Dataset items are versioned bitemporally *and* copied into run items at execution time (`0024_dataset_run_items.up.sql:18-27`), guaranteeing experiment results remain interpretable even if the item changes afterwards.
- **Raw/computed duality.** All derived quantities (usage, cost) keep the client-provided originals adjacent to enriched values, making enrichment itself auditable (`0002_observations.up.sql:19-23`).
- **Two-generation storage model.** Legacy `traces`/`observations` tables coexist with the richer `events_core` table; background migrations propagate and backfill lineage fields between them (`worker/src/backgroundMigrations/backfillEventsFullFromObservations.ts`, `backfillEventsFullFromDatasetRunItems.ts`).

## Notable Patterns

- **Schema-enforced pairing:** the ingestion schema refines that `promptName` and `promptVersion` must appear together or not at all, preventing half-linked prompt provenance (`packages/shared/src/server/ingestion/types.ts:495-500`).
- **Parallel-array column design for tools:** tool calls are stored as `tool_calls` (JSON payloads) plus aligned `tool_call_names` so `has(tool_call_names, 'x')` filters avoid JSON parsing — documented at the conversion helper (`packages/shared/src/server/ingestion/extractToolsBackend.ts:859-889`).
- **Origin-tagged artifact association:** media links carry a `MediaAssociationOrigin` enum distinguishing client upload vs ingestion extraction vs field overflow (`schema.prisma:1374-1379`), i.e., the *reason* an artifact exists is itself lineage metadata.
- **Magic-string embedding:** media references travel inside I/O payloads as parseable strings (`@@@langfuseMedia:type=…|id=…|source=…@@@`), keeping artifacts addressable from arbitrary nested content (`packages/shared/src/utils/IORepresentation/chatML/types.ts:27-45`; scanner at `packages/shared/src/utils/mediaReferences.ts:11-33`).
- **Append-only approval history:** agent approval decisions are custom events in the AG-UI stream rather than mutable flags, giving an immutable decision log (`packages/shared/src/in-app-agent/approvalEvents.ts:6-28`).

## Tradeoffs

- **Heuristic extraction vs coverage:** extracting tool calls from arbitrary vendor shapes maximizes compatibility but offers no completeness guarantee; false negatives silently lose lineage, and the code carries format-specific branches (OpenAI choices, Anthropic content parts, LangChain `additional_kwargs`) that must track ecosystem drift (`extractToolsBackend.ts:306-402`). Tool fields are also still withheld from the V1 public API as "not yet released" (`web/src/features/public-api/types/observations.ts:169-175`).
- **Denormalization vs consistency:** storing `prompt_name`/`prompt_version` next to `prompt_id` makes reads self-contained but means renames/deletions in the registry leave stale copies; cross-store (ClickHouse↔Postgres) integrity is enforced only by application-level cascades (`schema.prisma:1082,1091-1097`).
- **Eventual consistency in the run tree:** out-of-order ingestion is mitigated with processing delays around date boundaries (`processEventBatch.ts:66-92`), meaning lineage views can transiently miss parents.
- **Observability platform, not enforcement point:** Langfuse records whatever the SDK reports; it cannot prove an output was *caused* by a claimed input unless instrumentors wrap each step (e.g., retriever spans). The system optimizes for faithful recording over verification.

## Failure Modes / Edge Cases

- **Dangling prompt links:** when a name/version pair does not resolve, the record is still written with `prompt_id: ""` while retaining the unresolved name/version (`worker/src/services/IngestionService/index.ts:406`), producing lineage rows whose strong id cannot be dereferenced.
- **OTel prompt linking restricted to GENERATION type:** non-generation spans drop prompt attributes even if present (`canLinkPrompt` gate, `packages/shared/src/server/otel/OtelIngestionProcessor.ts:520-522`), so agents logging prompts on TOOL/CHAIN spans lose that provenance.
- **Unresolvable tool shapes:** extraction failures are swallowed with a console error and empty arrays (`extractToolsBackend.ts:758-767,828-838`), so malformed payloads degrade silently to "no tool lineage" instead of surfacing a data-quality signal.
- **No referential protection on lineage ids:** ClickHouse score/observation references accept arbitrary strings; deleting referenced objects relies on project-cascade workers, and partial failures could orphan chains (`schema.prisma:1091-1097` comments acknowledge "no fk constraint").
- **Duplicate suppression depends on ordering:** ReplacingMergeTree dedup uses `event_ts` as the finality marker (`0001_traces.up.sql:23`); clock-skewed writers could resurrect stale versions until merges complete.

## Future Considerations

- Promote tool lineage from heuristics to a declared contract: SDKs already emit typed tool events (`TOOL_CREATE`, `packages/shared/src/server/ingestion/types.ts:288,658-661`); validating and exposing `tool_definitions`/`tool_calls` through the public API would make tool-result auditing reliable rather than best-effort.
- Add structured retrieval provenance (document/chunk ids, source URIs) either as first-class RETRIEVER columns or a documented attribute convention mirrored by `events_core`, closing the largest gap toward "trace a fact to its source."
- Replace the `prompt?.id || ""` fallback with an explicit sentinel or nullable semantics plus a data-quality metric, so unlinked prompts are observable.
- Consider extending `AuditLogRecordType` beyond USER/API_KEY (`schema.prisma:846-849`) to cover trace/observation mutations if full causal auditing of analyst edits becomes a requirement.

## Questions / Gaps

- No evidence found of lineage linking *fragments* of outputs to inputs (e.g., citation-to-chunk mapping); searches across `packages/shared/src` and `web/src/features` for structured retrieval-source schemas (`retrieval document`, chunk/source columns in ClickHouse migrations) returned only the RETRIEVER span type convention (`packages/shared/src/domain/observations.ts:12`) and generic input/output storage (`0002_observations.up.sql:14-15`).
- No evidence found that model *provider request ids* or upstream response identifiers (e.g., OpenAI response ids) are persisted as lineage fields; only SDK/service/scope attribution exists (`0040_create_events_core.up.sql:82-90`). Searched ClickHouse migrations and ingestion processors for provider-request-id-like columns.
- The exact retention/guarantee semantics for the raw-event blob store used in replay were not examined in depth here (covered under the delivery-guarantees dimension); this study only confirms the replay entry point exists (`web/src/pages/api/admin/ingestion-replay.ts`).

---

Generated by `reports/repo/10.03-causal-links-and-lineage` dimension spec against `langfuse`.
