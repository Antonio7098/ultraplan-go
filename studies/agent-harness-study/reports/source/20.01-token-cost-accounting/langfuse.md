# Source Analysis: langfuse

## Dimension 20.01: Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (web), Express + BullMQ worker, shared package (Prisma/Postgres + ClickHouse) |
| Analyzed | 2026-08-26 |

## Summary

Langfuse implements token and cost accounting as a first-class ingestion-time enrichment pipeline. Every incoming observation can carry client-provided `usageDetails` and `costDetails` (`packages/shared/src/server/ingestion/types.ts:396-397,491-492,521`); when absent and a model is matched, the worker tokenizes input/output server-side via a tiktoken/Anthropic tokenizer worker-thread pool (`worker/src/features/tokenisation/usage.ts:31-55`, `worker/src/features/tokenisation/async-usage.ts:116-142`) and then multiplies per-usage-type units by tiered model prices using Decimal math (`IngestionService.calculateUsageCosts`, `worker/src/services/IngestionService/index.ts:1617-1689`). Model resolution is regex-based against a project-scoped or global model catalog seeded with 167 default models (`packages/shared/src/server/ingestion/modelMatch.ts:266-306`, `worker/src/constants/default-model-prices.json`), including usage-condition-based pricing tiers (e.g. cached-token prices, batch discounts). Costs are stamped onto each observation at ingest time in ClickHouse (`provided_usage_details`/`usage_details`/`provided_cost_details`/`cost_details` Map columns plus `total_cost`, `packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql:19-23`), and run-level summaries (trace, user, daily, experiment, dataset-run) are aggregated on read from those stamped values. Client-provided costs are strictly authoritative over computed ones, ERROR-level generations are never priced, and retries are safe because cost recomputation is idempotent last-write-wins rather than additive.

The answer to "what did this run cost?" is: yes for LLM generations — trace-level `sum(total_cost)` is one query away and surfaced in UI/API/dashboards — but only for model calls; tool executions carry no independent cost accounting unless a client supplies it.

## Rating

**8 / 10**

Rationale against the rubric:

- Clear model with explicit interfaces: zod-validated ingestion schemas for usage/cost (`packages/shared/src/server/ingestion/types.ts:20-97`), typed ClickHouse columns including materialized cost projections (`packages/shared/clickhouse/migrations/clustered/0040_create_events_core.up.sql:43-48`), persisted pricing-tier provenance per observation (`usage_pricing_tier_id/name`, `packages/shared/clickhouse/migrations/clustered/0031_add_usage_pricing_tier_columns.up.sql:1-2`).
- Extensive tests: a ~1,772-line dedicated suite for token/cost calculation covering provided-price precedence, partial user costs, overwrite semantics, ERROR-level skips, tokenization timeout handling, and mismatch guards (`worker/src/services/IngestionService/tests/calculateTokenCost.unit.test.ts:82-1772`) plus pricing-tier validation tests including catastrophic-backtracking rejection (`web/src/__tests__/server/model-pricing-tiers.servertest.ts:36-474`).
- Operational safeguards: tokenizer worker pool with timeouts instead of event-loop blocking (`worker/src/services/IngestionService/index.ts:1441-1465`), fail-safe condition evaluation (`packages/shared/src/server/pricing-tiers/matcher.ts:70-72`), cache invalidation on price changes (`worker/src/scripts/upsertDefaultModelPrices.ts:199-201`).
- Why not 9–10: no native tool-execution or non-model cost tracking; costs are frozen at ingest time (price-list changes do not restate history); retry *attempts* are observable but there is no explicit "retry-inclusive cost" roll-up; currency/budget guardrails live in cloud-only EE code, not OSS core.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Ingestion API accepts usage + cost | Zod schemas expose `usageDetails: UsageDetails` and `costDetails: CostDetails` on observation events; legacy OpenAI-style `usage` transformed to canonical shape | `packages/shared/src/server/ingestion/types.ts:20-69,396-397,490-492,519-521` |
| Server-side token counting fallback | Manual tokenization only when no provided usage/cost, model matched, level != ERROR | `worker/src/services/IngestionService/index.ts:1415-1424` |
| Tokenizer implementations | tiktoken for OpenAI-family (with chat-message overhead config), `@anthropic-ai/tokenizer` for claude; encodings cached | `worker/src/features/tokenisation/usage.ts:31-55,116-155,157-179` |
| Async tokenization pool | Worker-thread pool, 30s default timeout, worker replacement on crash | `worker/src/features/tokenisation/async-usage.ts:26-46,116-142` |
| Tokenization failure policy | Skip counts + emit metric `langfuse.tokenisation.skipped`; explicitly no byte-based estimate fallback | `worker/src/services/IngestionService/index.ts:1441-1466` |
| Model matching | `findModel` with local-cache → Redis → Postgres regex `match_pattern` lookup; project-scoped rows beat global; most recent `start_date` wins | `packages/shared/src/server/ingestion/modelMatch.ts:44-156,272-305` |
| Tiered pricing matcher | Priority-ordered conditions (regex-sum thresholds over usage keys; attribute membership on modelParameters/metadata); default-tier fallback; fail-safe on error | `packages/shared/src/server/pricing-tiers/matcher.ts:18-73,116-154` |
| Cost computation core | `calculateUsageCosts`: Decimal `price × units` per usage type; total = sum of entries unless priced "total" exists | `worker/src/services/IngestionService/index.ts:1658-1688` |
| Provided-cost precedence | Any non-null user cost point suppresses all computed costs; input+output summed when only those two provided | `worker/src/services/IngestionService/index.ts:1630-1656` |
| Tokenization skipped when costs given | Comment "Provided costs are authoritative"; usage_details left blank to avoid paying for tokenization | `worker/src/services/IngestionService/index.ts:1407-1419` |
| Pricing-tier provenance stored | Matched tier id/name returned into observation record; ClickHouse columns added by migration 0031 | `worker/src/services/IngestionService/index.ts:1306-1359`; `packages/shared/clickhouse/migrations/clustered/0031_add_usage_pricing_tier_columns.up.sql:1-2` |
| Default price catalog | JSON catalog of 167 models upserted into Postgres with tier/price reconciliation and cache clear | `worker/src/scripts/upsertDefaultModelPrices.ts:81-216,325-413`; `worker/src/constants/default-model-prices.json` (167 entries) |
| Storage schema (observations) | Map columns `provided_usage_details`, `usage_details`, `provided_cost_details`, `cost_details` (Decimal64(12)), `total_cost Nullable(Decimal64(12))` | `packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql:19-23` |
| v4 events table cost projections | MATERIALIZED `calculated_input_cost`/`calculated_output_cost`/`calculated_total_cost`, ALIAS `total_cost = cost_details['total']` | `packages/shared/clickhouse/migrations/clustered/0040_create_events_core.up.sql:43-48` |
| Trace/run summary | `sum(total_cost)` across observations joined to traces (users view); traces table itself has no cost column — aggregation is read-time | `packages/shared/src/server/repositories/traces.ts:1080-1145,1285` |
| Daily metrics summary | `sum(coalesce(o.total_cost,0)) as totalCost` merged with legacy metrics; exposed via public metrics API and dashboards | `packages/shared/src/server/repositories/daily-metrics.ts:40,50-76,108-123`; `web/src/pages/api/public/metrics/daily.ts` |
| Dashboard widgets | UserChart sums `totalCost` measure | `web/src/features/dashboard/components/UserChart.tsx:54,158-159` |
| Experiment item subtree cost | Window function sums `e.total_cost` per experiment item subtree | `packages/shared/src/server/repositories/experiments.ts:1074-1095` |
| Dataset run recursive cost | BFS descendant collection + Decimal summation preferring `totalCost`, falling back to input+output | `web/src/features/datasets/lib/costCalculations.ts:66-110` |
| Public API exposure | Trace endpoint returns `totalCost` when metrics included | `web/src/pages/api/public/traces/[traceId].ts:186` |
| Aggregate metric options | Selectable `avg(total_cost)` / `sum(total_cost)` aggregates; median cost in events repo | `packages/shared/src/server/repositories/observations.ts:2020-2030`; `packages/shared/src/server/repositories/events.ts:609-642` |
| Retry idempotency (ingestion) | Processor rethrows for BullMQ retry; Redis recently-processed cache dedupes fast replays; ClickHouse merge picks latest `event_ts` row per id | `worker/src/queues/ingestionQueue.ts:87-109,244-264,330-356`; `worker/src/services/IngestionService/index.ts:1760-1768` |
| Usage-total double-count guard | Warn-only metric/log (rate-limited, tagged by write path) when non-total buckets exceed provided total (instrumentor bug class) | `worker/src/services/IngestionService/index.ts:1557-1615` |
| LLM-call retries observable | AI SDK `maxRetries` option plumbed through eval LLM helper; retry reason/attempt_count emitted as span attributes | `packages/shared/src/server/llm/llmText.ts:115,352`; `worker/src/features/evaluation/evalSpanAttributes.ts:99-106` |
| Internal agent generations | In-app agent emits per-call generation observations with provider-reported usage; model omitted when no usage so no invented estimates | `worker/src/features/in-app-agent/runtime/instrumentation.ts:742-785` |
| Tool call recording (no costing) | Tool definitions/calls normalized onto observations; no per-tool pricing anywhere in cost path | `worker/src/services/IngestionService/index.ts:1024-1032` |
| EE platform metering (cloud-only) | Stripe usage metering queue with duplicate-suppression; free-tier threshold job counts traces/observations/scores | `worker/src/ee/cloudUsageMetering/handleCloudUsageMeteringJob.ts:29-87`; `worker/src/ee/usageThresholds/usageAggregation.ts:176-224` |

## Answers to Dimension Questions

1. **Are tokens counted per run?** Yes. Tokens arrive via three paths: (a) client-provided `usageDetails` accepted on every observation event (`packages/shared/src/server/ingestion/types.ts:396,491,521`) with normalization of string→number and auto-derived `total` (`worker/src/services/IngestionService/index.ts:1526-1532,1540-1555`); (b) server-side tokenization fallback gated on model match + absence of provided usage/cost + non-ERROR level (`worker/src/services/IngestionService/index.ts:1415-1424`); (c) usage details are summed per trace/user via ClickHouse `sumMap(usage_details)` with input/output buckets recovered by key-substring matching (`packages/shared/src/server/repositories/traces.ts:1084,1135-1137`). Per-run (trace-level) totals therefore exist, though computed read-time rather than stored on the trace.
2. **Are costs attributed per model call?** Yes — per observation/generation. Each write path (legacy merge and v4 direct-event path) runs `getGenerationUsage` which resolves the model via regex `match_pattern`, selects a pricing tier, and computes `Decimal(price) × units` per usage type (`worker/src/services/IngestionService/index.ts:1361-1365,1660-1666`). The matched tier is persisted alongside the row for auditability (`worker/src/services/IngestionService/index.ts:1347-1358`; `packages/shared/clickhouse/migrations/clustered/0031_add_usage_pricing_tier_columns.up.sql:1-2`). Prices support tiered dimensions like cached-input tokens — e.g. gpt-4o ships `input_cached_tokens` prices in the default catalog (`worker/src/constants/default-model-prices.json`, gpt-4o entry).
3. **Are tool execution costs tracked?** No, not natively. Tool calls and tool definitions are recorded as structured observation data (`worker/src/services/IngestionService/index.ts:1024-1032`), but the cost-enrichment gate requires a model name or client-provided usage/cost (`worker/src/services/IngestionService/index.ts:315-318`), and the price catalog keys everything off model usage types. A pure tool execution span carries zero cost unless the SDK/client supplies `costDetails`. No evidence found of any per-tool unit-price mechanism (searched `tool*price*`, tool cost keys in pricing schemas).
4. **Are retry costs accounted for?** Partially, by construction rather than by feature. (a) Ingestion-job retries: BullMQ processors rethrow to trigger retry (`worker/src/queues/ingestionQueue.ts:350-355`), a Redis recently-processed cache suppresses hot replays (`worker/src/queues/ingestionQueue.ts:87-109,244-264`), and ClickHouse ReplacingMergeTree semantics keep the latest `event_ts` row per id (`worker/src/services/IngestionService/index.ts:1760-1768`) — so a retried ingestion recomputes and *overwrites* cost instead of adding it; no double-count. Caveat: recomputation uses the pricing in effect at retry time, so a mid-retry price edit can silently change a record's cost. (b) LLM-call retries inside evals: attempts are made visible via `eval.llm.retry.reason` and `eval.llm.retry.attempt_count` attributes (`worker/src/features/evaluation/evalSpanAttributes.ts:99-106`) and each attempt that produced tokens becomes its own generation observation (in-app agent emits one generation per model call, `worker/src/features/in-app-agent/runtime/instrumentation.ts:757-785`), so retry spend is additive across observations — but there is no explicit "retry cost" roll-up or budget hook.
5. **Are per-run cost summaries available?** Yes, extensively, as read-time aggregations over ingest-stamped `total_cost`: per-user rollups (`packages/shared/src/server/repositories/traces.ts:1080-1145`), daily metrics exposed through the public `/api/public/metrics/daily` endpoint (`packages/shared/src/server/repositories/daily-metrics.ts:40-123`; `web/src/pages/api/public/metrics/daily.ts`), dashboard charts (`web/src/features/dashboard/components/UserChart.tsx:54,158-159`), experiment-item subtree costs via window functions (`packages/shared/src/server/repositories/experiments.ts:1095`), recursive dataset-run costs (`web/src/features/datasets/lib/costCalculations.ts:104-110`), median/avg/total selectable aggregates (`packages/shared/src/server/repositories/events.ts:609-642`; `packages/shared/src/server/repositories/observations.ts:2020-2030`), and `totalCost` on the public trace API (`web/src/pages/api/public/traces/[traceId].ts:186`). The traces table deliberately stores no cost column; all summaries derive from observations.

## Architectural Decisions

1. **Stamp cost at ingest, aggregate at read.** Costs are computed once during ingestion enrichment and written into ClickHouse observation rows (`worker/src/services/IngestionService/index.ts:1361-1365`); trace/session/experiment summaries are `sum()` queries over those stamps (`packages/shared/src/server/repositories/traces.ts:1285`). This makes historical records immune to later price edits but means price corrections never restate history.
2. **Client-provided data is authoritative.** If a client supplies any cost point, all computed costs are discarded and tokenization is skipped entirely ("Provided costs are authoritative", `worker/src/services/IngestionService/index.ts:1407-1419,1630-1656`). This avoids billing conflicts between vendor-reported and locally-tokenized numbers.
3. **Pricing tiers keyed by usage-shape, not just model name.** The matcher evaluates priority-ordered conditions — regex sums over usage-detail keys (for cache/batch buckets) and exact attribute membership on model parameters/metadata — falling back to a mandatory default tier (`packages/shared/src/server/pricing-tiers/matcher.ts:116-154`). Validation enforces exactly one default, unique priorities/names, and regex safety including catastrophic-backtracking rejection (`web/src/__tests__/server/model-pricing-tiers.servertest.ts:174-474`).
4. **Tokenization isolated from the request path.** Counting runs in a worker-thread pool with hard timeouts and crash-replacement (`worker/src/features/tokenisation/async-usage.ts:116-142,73-107`); on timeout the counts are skipped and a metric emitted rather than estimating from bytes (`worker/src/services/IngestionService/index.ts:1441-1466`) — an explicit correctness-over-completeness tradeoff.
5. **Three-layer model-match caching with invalidation.** Local TTL cache → Redis (with not-found tombstones and cluster-safe hash-tag keys) → Postgres regex query; price upserts clear caches globally (`packages/shared/src/server/ingestion/modelMatch.ts:31-42,308-325,358-370,430-471`; `worker/src/scripts/upsertDefaultModelPrices.ts:199-201`).
6. **Idempotent writes make retry-cost safety structural.** Rather than tracking retried jobs separately, the design relies on Redis seen-keys plus ClickHouse last-write-wins merges so repeated processing replaces rather than accumulates cost (`worker/src/queues/ingestionQueue.ts:87-109`; `worker/src/services/IngestionService/index.ts:1760-1768`).

## Notable Patterns

- **Usage-type-keyed maps instead of fixed columns.** `usage_details`/`cost_details` are `Map(String, UInt64/Decimal64(12))` (`packages/shared/clickhouse/migrations/clustered/0002_observations.up.sql:19-23`), letting providers report arbitrary buckets (cache reads/writes, reasoning tokens) without migrations; downstream queries bucket by case-insensitive key substrings (`packages/shared/src/server/repositories/traces.ts:1135-1137`).
- **Materialized ClickHouse projections for hot cost paths.** The v4 events table precomputes `calculated_input_cost`/`calculated_output_cost`/`calculated_total_cost` and aliases `total_cost` to `cost_details['total']` (`packages/shared/clickhouse/migrations/clustered/0040_create_events_core.up.sql:43-48`).
- **Defensive numeric hygiene.** String-to-number normalization for ClickHouse UInt64 round-trips (`worker/src/services/IngestionService/index.ts:1540-1555`), Decimal.js for money math (`worker/src/services/IngestionService/index.ts:1664`), and a rate-limited warn-only guard for instrumentor double-count payloads referencing issue #10592 (`worker/src/services/IngestionService/index.ts:1557-1615`).
- **Seed-catalog-as-code.** The 167-model default price list is a validated JSON artifact reconciled transactionally into Postgres (tiers deleted/vacated/priority-shifted to satisfy constraints) at startup (`worker/src/scripts/upsertDefaultModelPrices.ts:81-216,284-303`).
- **Provenance on every number.** Besides `internal_model_id`, each observation stores which pricing tier produced its cost (`worker/src/services/IngestionService/index.ts:1376-1382`), enabling post-hoc audits of pricing decisions.

## Tradeoffs

- **Frozen-at-ingest pricing vs. restatable history:** cheap reads and auditability, but fixing a wrong price leaves old observations mispriced forever (no backfill/repricing job found; searched `backfill.*cost`, `recalc`).
- **Skip-not-estimate tokenization:** avoids fabricating plausible-but-wrong numbers (`worker/src/services/IngestionService/index.ts:1442-1445`), at the cost of blank usage/cost for oversized payloads.
- **Substring-bucket heuristics:** mapping `positionCaseInsensitive(key,'input')>0` to input usage (`packages/shared/src/server/repositories/traces.ts:1135`) is flexible across providers but could misclassify exotic bucket names.
- **Warn-only consistency checks:** the usage-total mismatch detector logs/metrics but never auto-corrects, since some buckets (Bedrock cache writes) are legitimately additive (`worker/src/services/IngestionService/index.ts:1561-1571`) — safe, but corrupt payloads persist uncorrected.
- **Regex-based model matching:** powerful (matches versioned provider-prefixed names via patterns like `(?i)^(openai/)?(gpt-4o)$`, `worker/src/constants/default-model-prices.json`) but depends on catalog freshness; unmatched models yield zero cost silently (tested expectation at `worker/src/services/IngestionService/tests/calculateTokenCost.unit.test.ts:1122`).

## Failure Modes / Edge Cases

Covered by implementation/tests:
- Tokenization timeout or worker crash → counts skipped, metric `langfuse.tokenisation.skipped`, no sync fallback (`worker/src/services/IngestionService/index.ts:1441-1466`; test at `worker/src/services/IngestionService/tests/calculateTokenCost.unit.test.ts:1479`).
- ERROR-level generations are neither tokenized nor priced (`worker/src/services/IngestionService/index.ts:1415-1420`; test at `calculateTokenCost.unit.test.ts:1274`).
- Partial client cost points: input+output summed; other combinations leave total undefined (`worker/src/services/IngestionService/index.ts:1637-1643`; test at `calculateTokenCost.unit.test.ts:224`).
- Incremental updates: later events overwrite computed costs with provided ones, and vice versa, with previous-model reuse when the model arrives in a subsequent call (tests at `calculateTokenCost.unit.test.ts:534,694,775,1032,1199`).
- Zero/missing/fractional/large token counts handled explicitly (tests at `calculateTokenCost.unit.test.ts:346-470`).
- Malformed usage strings ignored rather than NaN-propagated (test at `calculateTokenCost.unit.test.ts:1431`).

Not covered:
- A price-list change between ingestion attempt and BullMQ retry silently reprices the row (recomputation uses current prices; no snapshot of applied prices beyond tier id).
- Tool spans, retrieval steps, sandbox execution, or human-in-the-loop labor have no cost representation at all — an agent harness studying end-to-end run economics must add these externally.
- No budget/ceiling enforcement exists in OSS core; spend alerting/free-tier thresholds are cloud-EE queues (`worker/src/ee/cloudSpendAlerts/handleCloudSpendAlertJob.ts`, `worker/src/ee/usageThresholds/handleCloudFreeTierUsageThresholdJob.ts`).

## Future Considerations

- Add optional non-model unit pricing (per-tool/per-resource-type) to close the tool-cost gap; the usage-type-keyed Map schema would accept new buckets without migration.
- Provide a documented reprice/backfill utility so price corrections can restate history deterministically (the tier-id stamping at `worker/src/services/IngestionService/index.ts:1347-1358` already provides the join key).
- Surface retry-inclusive cost roll-ups (e.g., group generations by attempt metadata) now that `eval.llm.retry.attempt_count` exists (`worker/src/features/evaluation/evalSpanAttributes.ts:106`).
- Consider pinning applied unit prices onto the observation row for byte-exact reproducibility of past calculations.

## Questions / Gaps

- **Tool execution costs:** unanswered by design — searched `tool` × `price|cost|unit` across `worker/src`, `packages/shared/src`, and the pricing catalogs; only recording-side hits (`worker/src/services/IngestionService/index.ts:1024-1032`). Conclusion: absent, not merely undocumented.
- **Historic repricing:** no evidence of a recalculation/backfill pipeline for cost columns after price edits (searched `recalc`, `backfill.*cost`, `reprice` in `web/src`, `worker/src`, `packages/shared/src`).
- **Multi-currency / FX:** all prices and cost columns are single-unit decimals implicitly in USD; no currency field found in `models`/`prices` Prisma schema or cost calculators.
- **Retry-cost attribution granularity:** whether each AI-SDK retry attempt is emitted as a distinct internal generation observation (and thus separately priced) could not be confirmed from static reading alone; the telemetry layer closes prior open attempts before starting the next (`packages/shared/src/server/llm/ai-sdk/telemetry.ts:329`), suggesting per-attempt spans, but runtime verification was out of scope for this filesystem study.

---

Generated by `20.01-token-and-cost-accounting` against `langfuse`.
