# Source Analysis: langfuse

## Dimension 10.01: Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js web app (UI + tRPC + public REST), BullMQ worker, shared package (`@langfuse/shared`), Postgres (Prisma) + ClickHouse + Redis + S3 |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths below are relative to the selected source root `studies/agent-harness-study/sources/langfuse/`.

## Summary

Langfuse is itself an LLM tracing platform, so this dimension studies its core product model rather than a side feature. The system defines a two-level run tree — a `trace` root plus a forest of `observation` children linked by `parentObservationId` (OTel `parentSpanId`) — persisted as flat rows in ClickHouse and reassembled into a tree at read/render time. The observation type taxonomy explicitly covers every execution step the dimension asks about: model calls (`GENERATION`, incl. `EMBEDDING`), tools (`TOOL`), guardrails (`GUARDRAIL`), retrieval (`RETRIEVER`), evals (`EVALUATOR`), agent turns/handoffs (`AGENT`), and orchestration wrappers (`CHAIN`, `SPAN`, `EVENT`) (`packages/shared/src/domain/observations.ts:5-16`). Two ingestion paths feed the same model: a typed-event ingestion API and an OTLP endpoint; the OTel adapter preserves foreign trace/span IDs verbatim, so any client that propagates W3C trace context nests correctly across processes. A priority-ordered mapper registry translates third-party span vocabularies (OpenInference, OTel GenAI semconv, Genkit, Vercel AI SDK, Pydantic/GenAI tool attrs, Flue, LiveKit) onto the Langfuse types. The full path from user request to tool result is visible in one trace via UI tree/timeline/graph views backed by a single `getObservationsForTrace` query. Hierarchy integrity is enforced progressively (edge validation of span IDs, Redis-backed trace dedup, shallow-trace backfill for out-of-order children) rather than transactionally, with read-side defenses against duplicate/orphaned/cyclic parent links.

## Rating

**9 / 10** — Mature and durable. Evidence for the top band: explicit, versioned data contracts (`packages/shared/src/server/ingestion/types.ts:279-299`), an extensible mapping registry documented as the extension point for new frameworks (`packages/shared/src/server/otel/ObservationTypeMapper.ts:163`), end-to-end tests that assert nested trace→span→generation→event persistence (`worker/src/services/IngestionService/tests/IngestionService.integration.test.ts:1182-1309`; `web/src/__tests__/server/otel-api.servertest.ts:60-133`), and operational safeguards proven under failure (iterative O(N)/no-recursion tree builder hardened after real OOM crashes, `web/src/features/traces/fns/treeBuilding.ts:88-122`; edge rejection of malformed OTLP batches, `web/src/pages/api/public/otel/v1/traces/index.ts:115-236`). Not a 10 because hierarchy integrity is eventual-consistency by design: no server-side check exists that a `parentObservationId` references an existing sibling at write time (orphans are cleaned only in the UI layer, `web/src/features/traces/fns/treeBuilding.ts:142-148`), cross-process nesting depends entirely on client-side context propagation that Langfuse cannot verify, and the dual legacy/v4 write paths add transient states during migration.

## Evidence Collected

Every entry includes a file path with line numbers relative to `studies/agent-harness-study/sources/langfuse/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trace providers | Public ingestion API accepting typed event batches (`trace-create`, `span-create`, `generation-create`, …) processed async to S3 + Redis queue | `web/src/pages/api/public/ingestion.ts:133-182`; `packages/shared/src/server/ingestion/processEventBatch.ts:116-418` |
| Trace providers | OTLP endpoint accepting OTLP/JSON and x-protobuf `ExportTraceServiceRequest`, staging raw batch to S3 then `OtelIngestionQueue` | `web/src/pages/api/public/otel/v1/traces/index.ts:83-113`, `296-312` |
| Event/span taxonomy | `eventTypes` enum: TRACE_CREATE, EVENT_CREATE, SPAN_CREATE/UPDATE, GENERATION_CREATE/UPDATE, AGENT_CREATE, TOOL_CREATE, CHAIN_CREATE, RETRIEVER_CREATE, EVALUATOR_CREATE, EMBEDDING_CREATE, GUARDRAIL_CREATE, SCORE_CREATE, SDK_LOG | `packages/shared/src/server/ingestion/types.ts:279-299` |
| Observation domain model | `ObservationType`: SPAN, EVENT, GENERATION, AGENT, TOOL, CHAIN, RETRIEVER, EVALUATOR, EMBEDDING, GUARDRAIL; generation-like helper groups LLM-call-like types | `packages/shared/src/domain/observations.ts:5-16`, `138-151` |
| Parent-child contract | Every observation body carries nullable `parentObservationId`; observations also carry `traceId`; scores carry both `traceId` and `observationId` | `packages/shared/src/server/ingestion/types.ts:452`, `442-443`, `534-538` |
| Storage schema | ClickHouse `observations` table: `parent_observation_id Nullable(String)`, `trace_id String`, `type LowCardinality(String)`, ReplacingMergeTree keyed on project/type/date/id | `packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql:1-46` |
| V4 unified events table | Events query builder maps `e.parent_span_id AS "parent_observation_id"` — same hierarchy in the v4 events store | `packages/shared/src/server/queries/clickhouse-sql/event-query-builder.ts:124` |
| OTel → tree mapping | `processSpan`: OTLP `span.spanId` becomes observation id, `span.parentSpanId` becomes `parentObservationId`; root detection = no parent OR `langfuse.internal.as_root` attribute | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:902-905`, `923-925`, `1168-1171`; attributes at `packages/shared/src/server/otel/attributes.ts:35-36` |
| Out-of-order handling | Shallow trace-create emitted when child arrives before its root; redundant shallow traces filtered in-batch; Redis seen-traces set (`SET NX EX 600s`) suppresses duplicate trace creation across batches; fail-safe empty set on Redis error | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:705-803`, `3376-3426` |
| Foreign vocabulary mapping | Priority registry maps `langfuse.observation.type`, OpenInference `openinference.span.kind` (LLM→GENERATION, AGENT, TOOL, GUARDRAIL, EVALUATOR, RETRIEVER), OTel GenAI ops (`invoke_agent`→AGENT, `execute_tool`→TOOL), Genkit, Vercel AI SDK, `gen_ai.tool.*` (Pydantic), Flue delegated-task→AGENT, LiveKit `agent_turn`/`function_tool`, model-presence fallback→GENERATION; default SPAN | `packages/shared/src/server/otel/ObservationTypeMapper.ts:165-473`, default at `507` |
| Cross-process propagation (ingested traces) | Ingestion preserves sender-supplied W3C trace/span IDs verbatim (`parseId` handles hex-string and byte-array forms), so processes propagating `traceparent` nest in one tree | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:902-905`, `3428-3432` |
| Cross-process propagation (Langfuse's own infra) | `instrumentAsync` extracts `traceparent`/`tracestate` carriers and supports `startNewTrace` severing; BullMQ instrumentation continues spans across web→worker queue hops | `packages/shared/src/server/instrumentation/index.ts:10-43`; provider setup in `web/src/observability.config.ts:10-17` |
| Internal self-tracing | Langfuse writes its own eval/experiment traces through the same OTel pipeline; writer regenerates valid span IDs and remaps child `parentSpanId`s parents-first, validating W3C 32-hex trace IDs | `packages/shared/src/server/otel/internalTraceOtelWriter.ts:100-213` |
| Sampled-span safeguard | `PassthroughTracer` keeps parent context active when sampling drops spans so descendant spans attach to the real parent instead of orphaning the subtree | `web/src/observability.config.ts:33-46` |
| In-app agent nesting | Agent runtime emits `agent-create` root under the trace, with `tool-create` and `generation-create` children referencing `rootObservationId` as `parentObservationId` | `worker/src/features/in-app-agent/runtime/instrumentation.ts:677-705`, `708-786` |
| Read path | Single query returns the flat observation list for a trace (with dedup-by-id unless OTel project); payload capped ~5MB (LFE-4882) | `packages/shared/src/server/repositories/observations.ts:138-209` |
| UI assembly | tRPC `byIdWithObservationsAndScores` fetches observations + scores for one trace; iterative tree builder (dedup, orphan cleanup, BFS depth, bottom-up cost aggregation); TRACE wrapper vs multi-root events mode | `web/src/server/api/routers/traces.ts:386-449`; `web/src/features/traces/fns/treeBuilding.ts:111-156`, `405-528` |
| Trace viewers | Tree/log views (`TraceTree.tsx`, `VirtualizedTree.tsx`, flattened log order) and read-only ELK graph view with aggregated/expanded modes built from "the instrumented hierarchy" | `web/src/features/traces/components/TraceTree.tsx`; `web/src/features/trace-graph-view/README.md:14-21` |
| Public API read | GET `/api/public/traces/{id}` optionally returns observations list for the trace | `web/src/pages/api/public/traces/[traceId].ts:60-101` |
| API contract docs | Fern spec documents ingestion events with `parentObservationId` and `ObservationType` union | `fern/apis/server/definition/ingestion.yml:183`, `235` |
| Tests: OTel end-to-end | Server test asserts OTLP JSON batch yields trace row (id = traceId hex) + observation row (id = spanId hex); nested public-trace case with `parent_span_id` linkage | `web/src/__tests__/server/otel-api.servertest.ts:61-133`, `135-189` |
| Tests: native ingestion chain | Integration test builds trace-create → span-create/update → generation-create (`parentObservationId: spanId`) → event-create (`parentObservationId: generationId`) + score attached to the generation, then asserts persisted rows | `worker/src/services/IngestionService/tests/IngestionService.integration.test.ts:1182-1309` |
| Tests: ID validation | `parseId` rejects unconvertible `parentSpanId`, accepts string/byte-array forms | `packages/shared/src/server/otel/utils.test.ts:110-131` |
| Tests: internal writer | Child `parentSpanId` remapped onto regenerated span ids; unknown parent rejected | `packages/shared/src/server/otel/internalTraceOtelWriter.test.ts:98-134` |
| Worker write path | OTel queue processor converts spans to ingestion events, splits observations (direct merge-write) from traces (`processEventBatch`), and resolves legacy-dual vs direct-v4-events write path per header/env/scope/org signals | `worker/src/queues/otelIngestionQueue.ts:553-758` |

## Answers to Dimension Questions

**1. Is there a single coherent trace tree?**
Yes. One trace id roots a forest of observations stored flat in ClickHouse with `parent_observation_id` links (`packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql:6`), or equivalently `parent_span_id` in the v4 events table (`packages/shared/src/server/queries/clickhouse-sql/event-query-builder.ts:124`). Both ingestion paths converge on the same model: the native API validates the identical zod schema family (`packages/shared/src/server/ingestion/types.ts:719-739`) and the worker's OTel processor emits those same ingestion events (`worker/src/queues/otelIngestionQueue.ts:553-578`). The tree is reassembled deterministically at read time (`web/src/features/traces/fns/treeBuilding.ts:405-528`). Coherence is eventually consistent: rows arrive via queues with ordered updates-last sorting (`packages/shared/src/server/ingestion/processEventBatch.ts:441-460`), a deliberate processing-delay window around UTC midnight prevents duplicate/out-of-order merges (`processEventBatch.ts:66-92`), and ReplacingMergeTree merges duplicates.

**2. Are all execution steps represented?**
Yes, explicitly in the taxonomy. Model calls are `GENERATION`/`EMBEDDING`, tool calls `TOOL` (with structured `toolDefinitions`/`toolCalls`/`toolCallNames` columns, `packages/shared/src/domain/observations.ts:97-100`), guardrail checks `GUARDRAIL`, retrieval `RETRIEVER`, evaluator runs `EVALUATOR`, agent turns/handoffs `AGENT`, and free-form steps `SPAN`/`EVENT`/`CHAIN` (`packages/shared/src/domain/observations.ts:5-16`). Scores attach at trace or observation level, so evaluation results hang off the exact node they judged (`packages/shared/src/server/ingestion/types.ts:534-538`). For foreign instrumentations that lack Langfuse-native types, ten prioritized mappers derive the type from OpenInference, OTel GenAI semconv, Genkit, Vercel AI SDK, Pydantic-style `gen_ai.tool.*`, Flue, and LiveKit attributes (`packages/shared/src/server/otel/ObservationTypeMapper.ts:165-473`). Caveat: unmapped spans fall back to plain `SPAN` (`ObservationTypeMapper.ts:507`), so representation is guaranteed but semantic typing is best-effort.

**3. Do handoffs and subagent calls nest correctly?**
Correctly whenever context propagates, which is the industry-standard contract rather than a server guarantee. Observation IDs *are* OTel span IDs (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:1169`), so a subagent/service/process that receives W3C `traceparent` and exports its spans to Langfuse lands inside the parent's tree automatically. Delegated sub-agent runs are first-class: Flue task spans map to `AGENT` (`ObservationTypeMapper.ts:412-434`), LiveKit agent activity to `AGENT`, `invoke_agent`/`create_agent` GenAI operations to `AGENT` (`ObservationTypeMapper.ts:250-268`), and Langfuse's own in-app agent nests `agent-turn` → `invoke-model`/tool spans under a root observation (`worker/src/features/in-app-agent/runtime/instrumentation.ts:708-786`). An `as_root` escape hatch can deliberately sever nesting (`packages/shared/src/server/otel/attributes.ts:35`). No evidence found of server-side handoff-specific constructs (e.g., explicit handoff edges distinct from parent-child spans); handoffs are modeled purely as AGENT-type spans in the same tree.

**4. Can you follow a request from start to finish?**
Yes, through three aligned surfaces: (a) sessions group traces via `sessionId` on the trace body (`packages/shared/src/server/ingestion/types.ts:432`); (b) within a trace, the UI renders tree, timeline, log, and graph views from one data fetch (`web/src/server/api/routers/traces.ts:386-449`; graph modes described in `web/src/features/trace-graph-view/README.md:14-21`); (c) programmatically, GET `/api/public/traces/{id}?fields=observations` returns the full node list including parent links (`web/src/pages/api/public/traces/[traceId].ts:60-101`). Latency, cost, and usage aggregate bottom-up along the tree, reinforcing end-to-end readability (`treeBuilding.ts:248-402`).

## Architectural Decisions

1. **Flat storage, read-time tree assembly.** Observations persist as independent rows keyed by `(project_id, type, start_date, id)` with a nullable parent pointer (`packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql:35-46`); the UI rebuilds the tree iteratively per render (`web/src/features/traces/fns/treeBuilding.ts:1-20`). This decouples write throughput from tree shape but moves integrity enforcement to the read path.
2. **Adopt OTLP as the universal spine.** Rather than inventing propagation, Langfuse ingests standard OTLP alongside its typed-event API and keeps sender-supplied W3C IDs verbatim (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:902-905`, `3428-3432`), making every OTel-instrumented framework instantly hierarchy-compatible.
3. **Extensible type-mapper registry over hardcoding.** New frameworks are added by appending prioritized mappers ("This is the constructor to modify if you want to add new mappings", `packages/shared/src/server/otel/ObservationTypeMapper.ts:163`), keeping semantic typing centralized and testable.
4. **Async, queue-mediated ingestion with S3 replayability.** Web containers only validate and stage raw payloads; conversion happens in workers with the original blob retained for replay (`web/src/pages/api/public/otel/v1/traces/index.ts:309-312`; `worker/src/queues/otelIngestionQueue.ts:502-554`).
5. **Trace synthesis for partial trees.** Children arriving before their root get a "shallow" trace row, later superseded or filtered if the full trace appears (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:986-993`, `705-803`).
6. **Dual write-path migration (v3 legacy tables ↔ v4 events table)** resolved per batch via header > env > scope > org-cutoff precedence (`worker/src/queues/otelIngestionQueue.ts:288-331`).

## Notable Patterns

- **Identity passthrough**: observation id = OTel span id, parent link = OTel parentSpanId — zero-mapping distributed nesting (`OtelIngestionProcessor.ts:1168-1171`).
- **Priority-ordered strategy registry** with cached sort for vocabulary mapping (`ObservationTypeMapper.ts:165-484`).
- **Defense-in-depth against malformed trees**: id-collision dedup keeping earliest startTime, orphan-parent nulling, cycle-safe BFS depth propagation — each annotated with the production incident it prevents (multi-parent DAG → exponential paths → OOM crash) (`web/src/features/traces/fns/treeBuilding.ts:88-122`, `200-223`).
- **Fail-open operational posture**: Redis outage returns an empty seen-set and processing continues (`OtelIngestionProcessor.ts:3419-3425`); rate-limiter errors fail open (`web/src/pages/api/public/ingestion.ts:127-131`).
- **Edge validation with actionable errors**: malformed OTLP exports are rejected wholesale at the HTTP boundary with reason counts, metric tags, and remediation hints (e.g., "you sent OTLP logs to the traces endpoint") (`web/src/pages/api/public/otel/v1/traces/index.ts:115-236`).
- **Sampling without orphaning**: a non-recording tracer forwards the real parent context so dropped spans don't detach descendants (`web/src/observability.config.ts:33-46`).

## Tradeoffs

- **Eventual consistency vs write-time integrity**: no FK-style check on `parentObservationId` at ingestion; orphaned nodes render as extra roots until cleaned client-side (`treeBuilding.ts:142-148`). Chosen for throughput; costs a window where trees look fragmented.
- **Client-trust model**: hierarchy fidelity depends on exporters propagating context correctly; Langfuse neither detects broken propagation nor stitches across incompatible trace IDs.
- **Best-effort semantics**: fallback to generic `SPAN` guarantees presence but loses type-specific analytics for unknown frameworks (`ObservationTypeMapper.ts:507`).
- **Complexity budget**: three write paths (native events, dual-write OTel, direct v4 events) coexist behind version/env/header gates (`otelIngestionQueue.ts:607-686`), increasing the surface where hierarchy-affecting regressions could hide.
- **Read-time cost**: tree rebuild, dedup, and 5MB response caps trade CPU/memory in web containers for storage simplicity (`repositories/observations.ts:204-209`).

## Failure Modes / Edge Cases

- **Out-of-order arrival**: children before roots handled via shallow traces + Redis TTL(600s) seen-set + update-last batch ordering (`OtelIngestionProcessor.ts:930-950`, `processEventBatch.ts:441-460`).
- **Duplicate observation ids with differing parents**: collapses to a dense multi-parent DAG whose root→node path count is exponential — previously crashed clients with `RangeError`/OOM; now deduped and visited-guarded (`treeBuilding.ts:88-122`, `200-223`).
- **Orphaned parents** (parent never ingested): nulled so the child promotes to root (`treeBuilding.ts:142-148`).
- **Malformed OTLP** (id-less spans, object-shaped attributes, logs-to-traces misrouting): rejected at the edge before burning six worker retries (`otel/v1/traces/index.ts:115-236`); unconvertible `parentSpanId` fails conversion with a counted reason (`utils.test.ts:110-118`).
- **Straggler traces beyond the 10-minute seen-TTL**: may produce a second trace-create; harmless due to ReplacingMergeTree upsert semantics but adds churn.
- **Eval recursion**: internal `langfuse-*` environments and the trace-upsert guard prevent LLM-as-judge traces from triggering further evals (`internalTraceOtelWriter.ts:56-64`; eval exclusion noted at `otelIngestionQueue.ts:836-838`).
- **Very deep/wide trees**: recursive algorithms replaced with explicit-stack iterations specifically for 10k+ depth (`treeBuilding.ts:5`, `610-611`).

## Future Considerations

- Surface hierarchy-integrity telemetry server-side (orphan/duplicate counts at ingestion) instead of relying on UI-layer cleanup, enabling alerting on broken client propagation.
- Consider optional write-time referential hints (e.g., deferred parent-resolution jobs) to shrink the fragmented-tree visibility window.
- Continue collapsing the dual-write surface once the v4 events table fully supersedes legacy traces/observations (`otelIngestionQueue.ts:693-710`), reducing path-dependent hierarchy behavior.
- Document a recommended minimal attribute set for exporters so unmapped frameworks land as typed observations rather than generic SPANs.

## Questions / Gaps

- **No server-side proof of cross-service nesting**: tests exercise single-batch nesting (`otel-api.servertest.ts:135-189`) and synthetic internal writers (`internalTraceOtelWriter.test.ts:98-121`), but no test simulates two processes exporting interleaved OTLP batches relying on propagated `traceparent`. Behavior follows from identity passthrough, yet it remains inferred rather than directly demonstrated in-repo.
- **Handoff semantics beyond AGENT typing**: searched for dedicated handoff/subagent edge metadata (e.g., causal "delegated-to" links distinct from parent-child); none found in the ingestion schema or mapper registry — such modeling appears delegated to scores/metadata conventions.
- **Guardrail evaluation outcomes**: `GUARDRAIL` observations exist as a type with generation-like I/O fields, but no dedicated outcome field (blocked/passed) was found in the schema; semantics ride on input/output/metadata.
- **Trace-level completeness SLA**: no evidence of a mechanism asserting a trace reached a terminal state (all spans closed); the midnight delay window mitigates boundary effects but nothing declares a run "finished."

---

Generated by `Dimension 10.01: Span Hierarchy and Run Tree` against `langfuse`.
