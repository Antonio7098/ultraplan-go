# Source Analysis: langfuse

## 18.04 Cost, Latency, and Quality Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (`web`), BullMQ worker (`worker`), shared domain package (`packages/shared`), Postgres + ClickHouse + Redis |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths are relative to the selected source root `studies/agent-harness-study/sources/langfuse/`.

## Summary

Langfuse treats cost, latency, and quality measurement as first-class product data rather than as an afterthought of its eval system. Every ingested generation is priced at write time through a model-match → pricing-tier-match → usage×price pipeline (`worker/src/services/IngestionService/index.ts:1265-1383`, `worker/src/services/IngestionService/index.ts:1617-1689`), so eval executions — which are themselves traced into the same ingestion path via internal trace sinks (`worker/src/features/evaluation/evalExecutionDeps.ts:241-253`) — automatically carry token cost, latency, and success metadata. On top of that substrate sit eval-specific cost surfaces: per-evaluator total cost queries (`packages/shared/src/server/repositories/events.ts:3634-3668`), synchronous test-run cost computation from judge token usage (`web/src/features/evals/v2/server/evaluators/testEvaluator.ts:168-194`), and a weekly activation-cost estimator with sampling extrapolation (`web/src/features/evals/v2/server/evaluators/activationCostService.ts:141-153`). Latency is a SQL-defined measure family (latency, time-to-first-token, tokens/sec) in the dashboard query model (`packages/shared/src/features/query/dataModel.ts:551-604`), and quality is tracked through scores aggregated per dataset run alongside cost and latency (`packages/shared/src/server/repositories/dataset-run-items.ts:95-107`).

The one gap relative to this dimension's framing: there is no single artifact that jointly analyzes cost vs quality across model choices. The ingredients exist (per-model cost and latency dashboards, dataset-run comparison aggregates, score-comparison analytics), but comparing two models on cost-vs-quality requires manually cross-referencing separate views.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Cost is measured end-to-end**: pricing happens deterministically at ingestion (`worker/src/services/IngestionService/index.ts:1361-1365`) with explicit precedence for user-provided costs (`worker/src/services/IngestionService/index.ts:1634-1656`), tiered pricing with conditions (`packages/shared/src/server/pricing-tiers/matcher.ts:102-148`), and caching with invalidation safeguards (`packages/shared/src/server/ingestion/modelMatch.ts:26-42`, `packages/shared/src/server/ingestion/modelMatch.ts:396-418`).
- **Eval-specific cost economics are implemented, not implied**: activation-time estimates extrapolate `matchingObservations × sampling × testRunCostUsd` (`web/src/features/evals/v2/server/evaluators/activationCostService.ts:148-151`), with retry/backoff polling (`web/src/features/evals/v2/server/evaluators/activationCostService.ts:21-23`, `:201-210`) and a unit-tested service contract (`web/src/features/evals/v2/server/evaluators/activationCostService.servertest.ts:70`, `:140-156`).
- **Latency is a measured dimension** with dedicated SQL measures including TTFT and throughput (`packages/shared/src/features/query/dataModel.ts:551-604`) and pre-provisioned dashboards (`worker/src/constants/langfuse-dashboards.json:10`, `:180`, `:488`).
- **Success tracking exists but is derived, not named**: job execution status counts (`packages/shared/src/server/repositories/job-executions.ts:41-68`) and boolean/categorical score aggregates stand in for "success rate"; there is no `successRate` metric symbol anywhere in the repo.
- **Why not 9–10**: cost estimation silently degrades to `"Unavailable"` when no pricing tier matches (`web/src/features/evals/v2/components/Evaluators/EvaluatorSavedDialog/EvaluatorSavedCostSummary.tsx:93-95`, `:110-115`); cost attribution for legacy/prompt-experiment executions relies on a trace-name string convention rather than structured IDs (`web/src/features/evals/v2/server/evaluators/evaluatorService.ts:177-186`); and no joint cost-vs-quality comparison view exists.

## Evidence Collected

Every entry cites a file path with line numbers, relative to `studies/agent-harness-study/sources/langfuse/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token cost at ingestion | `getGenerationUsage` matches model via `findModel` (:1295), pricing tier via `matchPricingTier` (:1341), then computes costs; logs computed cost/usage/tier (:1367-1374) | worker/src/services/IngestionService/index.ts:1265-1383 |
| Cost arithmetic | `calculateUsageCosts`: price × units per usageType; user-provided costs are authoritative and suppress calculation | worker/src/services/IngestionService/index.ts:1617-1689 |
| Model/pricing resolution | `findModel` with L1 local cache + Redis cache + Postgres fallback; negative-result caching; project-scoped cache clearing | packages/shared/src/server/ingestion/modelMatch.ts:44-156, :310-325, :396-418 |
| Pricing tiers | Tier matching by usage details and attributes, returns matched prices keyed by usageType | packages/shared/src/server/pricing-tiers/matcher.ts:102-148 |
| Eval run cost query | `getLatestEvaluatorRunCost`: latest cost-bearing evaluator trace within 7 days, requires ≥1 GENERATION child, ordered by timestamp desc | packages/shared/src/server/repositories/events.ts:3634-3668 |
| Per-evaluator totals | `EvaluatorsService.getTotalCosts` maps trace-name convention to evaluator IDs; exposed as tRPC `costByEvaluatorIds` | web/src/features/evals/v2/server/evaluators/evaluatorService.ts:205-231; web/src/features/evals/server/router.ts:1355-1359 |
| Test-run cost | Judge call result usage (`inputTokens/outputTokens/totalTokens`) converted to USD synchronously via `matchPricingTier`; returned as `estimatedCostUsd` | web/src/features/evals/v2/server/evaluators/testEvaluator.ts:129-137, :168-194 |
| Activation cost estimate | Weekly estimate = `matchingObservations * params.sampling * testRunCostUsd`; fallback test run executed when no recent cost exists; bounded retry polling (delays 0–8 s) | web/src/features/evals/v2/server/evaluators/activationCostService.ts:141-153, :116-139, :21-23, :201-210 |
| Cost estimate tests | Mocked `getLatestEvaluatorRunCost` scenarios incl. retry-until-cost and unavailable-cost paths | web/src/features/evals/v2/server/evaluators/activationCostService.servertest.ts:70, :140-156, :278-291 |
| Cost UI | "≈ $X / week" tooltip with calculation breakdown; sampling slider trades coverage vs cost; explicit "Unavailable" + reason copy | web/src/features/evals/v2/components/Rules/RuleSetup/components/RuleEvaluatorCostEstimate.tsx:18-27; web/src/features/evals/v2/components/Evaluators/EvaluatorSavedDialog/EvaluatorSavedCostSummary.tsx:51-60, :93-115 |
| Latency measures | Trace latency = first-to-last observation diff; observation-level TTFT and tokens/sec SQL measures | packages/shared/src/features/query/dataModel.ts:120-128, :551-571, :596-604 |
| Cost measures | `inputCost`, `outputCost`, `totalCost` declared as decimal USD measures over `cost_details`/`total_cost` | packages/shared/src/features/query/dataModel.ts:572-595 |
| Default dashboards | P95 Latency by Use Case/Level/Model; TTFT by prompt/model; Avg Output Tokens/s by Model; Total/Top-N/P95 cost widgets; dedicated "Langfuse Cost Dashboard" and "Langfuse Latency Dashboard" | worker/src/constants/langfuse-dashboards.json:10, :27, :146, :163, :180, :197, :214, :231, :248, :316, :488, :535 |
| Eval execution tracing | Judge LLM calls run through `generateLLMText` with a trace sink into the target project, so judge usage/cost/latency land in normal observability tables | worker/src/features/evaluation/evalExecutionDeps.ts:241-262; web/src/features/evals/v2/server/evaluators/testEvaluator.ts:108-128 |
| Usage telemetry capture | AI SDK telemetry records `gen_ai.usage.input_tokens`/`output_tokens` (+cache buckets) and error types onto generation spans | packages/shared/src/server/llm/ai-sdk/telemetry.ts:360-386 |
| Success/status counts | Prisma groupBy of `jobExecution` status per config; tRPC endpoint folds legacy rule IDs into counts | packages/shared/src/server/repositories/job-executions.ts:24-68; web/src/features/evals/server/router.ts:1300-1353 |
| Status → display state | `deriveEvaluatorDisplayStateFromExecutionCounts` derives ACTIVE/FINISHED/paused from status counts (PENDING detection) | packages/shared/src/features/evals/evalConfigBlocking.ts:162-212 |
| Quality + cost + latency joins | `DatasetRunsMetrics` per run: `avgTotalCost`, `totalCost`, `avgLatency`, `aggScoresAvg`, categorical/boolean score aggregates | packages/shared/src/server/repositories/dataset-run-items.ts:95-107, :165-185 |
| Score comparison analytics | Query builder producing heatmaps (numeric), confusion matrices (categorical/boolean), stacked distributions | web/src/features/score-analytics/server/buildScoreComparisonQuery.ts:35-50 |
| Threshold alerting | Monitors evaluate any view metric over windows with warning/alert thresholds, no-data modes, renotify; window shifted back 30 s to avoid events-table write lag | packages/shared/src/features/monitors/types.ts:17-18, :66-104, :160-183, :186-202, :248-290 |
| Model choice plumbing | Per-evaluator judge model/provider validation; project default eval model upsert/delete blocks/unblocks evaluators | web/src/features/evals/v2/server/evaluators/testEvaluator.ts:85-92; web/src/features/evals/server/defaultEvalModelRouter.ts:30-98 |
| Experiment runs | Prompt experiments execute per-item LLM calls with experiment context and schedule follow-up observation evals | worker/src/features/experiments/experimentServiceClickhouse.ts:157-236 |

## Answers to Dimension Questions

### 1. Is token cost measured in evals?

**Yes, comprehensively.** Two layers:

- **Generic layer**: every generation (including eval/judge calls, which ingest through the same pipeline with internal trace sinks — `worker/src/features/evaluation/evalExecutionDeps.ts:245-252`) is priced at ingestion using model match + pricing-tier match + usage×price (`worker/src/services/IngestionService/index.ts:1295-1299`, `:1341-1358`, `:1361-1365`). Users may supply their own costs, which take precedence and disable computation (`worker/src/services/IngestionService/index.ts:1634-1656`).
- **Eval-specific layer**: per-evaluator cumulative cost via ClickHouse (`packages/shared/src/server/repositories/events.ts:3634-3668` and `getTotalCostByEvaluatorTraceNames` consumed at `web/src/features/evals/v2/server/evaluators/evaluatorService.ts:220-230`); synchronous test-run cost from judge token usage (`web/src/features/evals/v2/server/evaluators/testEvaluator.ts:129-137`); and projected weekly cost at rule activation with a user-adjustable sampling rate (`web/src/features/evals/v2/server/evaluators/activationCostService.ts:148-151`, UI slider at `web/src/features/evals/v2/components/Evaluators/EvaluatorSavedDialog/EvaluatorSavedCostSummary.tsx:51-60`).

### 2. Is latency measured?

**Yes.** Latency is defined as SQL measures in the shared query model — trace-level elapsed time (`packages/shared/src/features/query/dataModel.ts:120-128`) and observation-level TTFT plus throughput (`packages/shared/src/features/query/dataModel.ts:551-571`, `:596-604`). Pre-provisioned dashboards segment p95 latency by use case, level, and model, and track TTFT (`worker/src/constants/langfuse-dashboards.json:10`, `:27`, `:146`, `:163`, `:180`). Eval job executions record wall-clock bounds via `startTime`/`endTime` on `jobExecution` completion (`worker/src/features/evaluation/evalCompletion.ts:83-92`).

### 3. Is success rate tracked?

**Partially — tracked as inputs, not as a named metric.** There is no `successRate` symbol in the repo (searched `successRate|success_rate` across `worker/src`, `packages/shared/src`, `web/src/features`: no production hits). What exists instead:

- Job execution outcome counts grouped by status (`COMPLETED`/`PENDING`/`ERROR`/`CANCELLED`) per evaluator, exposed via tRPC (`packages/shared/src/server/repositories/job-executions.ts:24-39`; `web/src/features/evals/server/router.ts:1300-1353`) and folded into display state such as FINISHED/ACTIVE (`packages/shared/src/features/evals/evalConfigBlocking.ts:180-190`, `:192-212`).
- Quality outcomes as scores: numeric/categorical/boolean score aggregates per dataset run (`packages/shared/src/server/repositories/dataset-run-items.ts:104-106`), score-comparison analytics with confusion matrices (`web/src/features/score-analytics/server/buildScoreComparisonQuery.ts:35-50`), and threshold-based monitors with alert severities (`packages/shared/src/features/monitors/types.ts:248-290`).
- Eval output validity gates: `validateEvalOutputResult` fails an execution whose output does not parse against the schema (`packages/shared/src/server/evals/llmEvaluatorExecution.ts:44-47`), and failures surface via trace `level` and execution-trace listing (`web/src/features/evals/v2/server/evaluators/evaluatorService.ts:187-202`).

A pass-rate percentage must therefore be assembled by the user from these primitives.

### 4. Are model tradeoffs analyzed?

**Ingredients yes, joined analysis no.** Model choice is fully factored into the system: evaluators select a judge provider/model or inherit a project default with block/unblock semantics on deletion (`web/src/features/evals/server/defaultEvalModelRouter.ts:30-98`), pricing tiers can condition on model parameters/metadata (`packages/shared/src/server/pricing-tiers/matcher.ts:102-148`), and dashboards break down cost and latency **by model name** (`worker/src/constants/langfuse-dashboards.json:180`, `:197`, `:316`). Dataset experiments produce runs whose metrics combine avg cost, avg latency, and score aggregates (`packages/shared/src/server/repositories/dataset-run-items.ts:95-107`), which is the closest supported comparison vehicle.

However, **no artifact performs an explicit cost-vs-quality tradeoff analysis across two models** — there is no scatter/ratio view joining score to cost, and the eval activation flow estimates cost only, never quality impact. Answering "can you compare two model choices on cost vs quality?" today means exporting the per-model cost dashboard next to the per-model latency dashboard, or running both models as separate dataset experiments and reading the run comparison table.

## Architectural Decisions

1. **Price at write, aggregate at read.** Costs are computed once during ingestion and stored (`cost_details`, `total_cost` columns written in `worker/src/services/IngestionService/index.ts:1361-1382`); read-side queries only sum stored values (`packages/shared/src/features/query/dataModel.ts:588-595`). This makes every downstream consumer (dashboards, eval cost endpoints) cheap and consistent, at the cost that retroactive price-list changes do not rewrite history.
2. **Reuse the observability substrate for eval accounting.** Eval/judge executions are ordinary traces ("Execute evaluator: \<name\>" / "Test evaluator", environment-tagged) so they inherit pricing, latency, and error capture for free (`worker/src/features/evaluation/evalExecutionDeps.ts:245-252`; `web/src/features/evals/v2/server/evaluators/testEvaluator.ts:121-127`). The tradeoff is attribution: without structured metadata, mapping traces back to evaluators depends on the trace-name convention (`web/src/features/evals/v2/server/evaluators/evaluatorService.ts:177-186`), acknowledged as incomplete for prompt-experiment executions.
3. **Estimate before commit.** Rule activation runs a real test evaluation and polls for its persisted cost rather than guessing from static price lists (`web/src/features/evals/v2/server/evaluators/activationCostService.ts:116-139`), trading activation latency (up to ~15.75 s of retries per evaluator, `:21-23`) for estimate realism grounded in actual token consumption.
4. **Derived success states instead of a canonical success metric.** Execution status counts and score type-specific aggregates are the primitives; UI state machines derive meaning (`packages/shared/src/features/evals/evalConfigBlocking.ts:192-212`). Flexible, but it pushes pass-rate synthesis onto users.

## Notable Patterns

- **Measure declarations as data**: latency, TTFT, tokens/sec, input/output/total cost are declarative SQL measure objects consumed by dashboards and monitors alike (`packages/shared/src/features/query/dataModel.ts:551-604`), so alerting inherits the same metric vocabulary (`packages/shared/src/features/monitors/types.ts:222-242` validates monitor queries against the same measure set).
- **Negative caching for absent models**: unmatched models are cached under a `LANGFUSE_MODEL_MATCH_NOT_FOUND` token with TTL to avoid repeated Postgres probes (`packages/shared/src/server/ingestion/modelMatch.ts:308-325`), plus lock-guarded full-cache clears on boot (`:430-471`).
- **Sampling-as-cost-control**: the sampling slider directly multiplies into the cost estimate and the persisted rule, making cost/coverage an explicit, user-visible knob at configuration time (`web/src/features/evals/v2/components/Evaluators/EvaluatorSavedDialog/EvaluatorSavedCostSummary.tsx:31-37`).
- **Operational lag awareness in evaluation**: monitor evaluation windows shift back 30 s because the events table lags writes (`packages/shared/src/features/monitors/types.ts:17-18`) — a rare explicit acknowledgment that measurement freshness constrains evaluation correctness.
- **Tiered, conditional pricing**: pricing tiers support conditions over model parameters and metadata, not just flat model names (`packages/shared/src/server/pricing-tiers/types.ts:15-34`).

## Tradeoffs

| Decision | Gain | Cost |
|----------|------|------|
| Write-time cost computation | Consistent reads; simple aggregation | Price edits don't backfill; missing tier ⇒ permanently unpriced rows until new data arrives |
| Trace-name-based evaluator cost attribution | No join dependency on metadata propagation | Fragile to renaming; prompt-experiment executions miss `evaluator_id` metadata (`web/src/features/evals/v2/server/evaluators/evaluatorService.ts:177-178`) |
| Live-test-based activation estimates | Realistic USD figures reflecting actual prompt size | Adds seconds of latency and one billable LLM call per evaluator during setup; falls back to "Unavailable" |
| Status counts over pass-rate metric | Works for any executor (LLM-judge, code, future) | Users must compute rates themselves; boolean-score pass rate is implicit in `aggScoreBooleans` (`packages/shared/src/server/repositories/dataset-run-items.ts:106`) |

## Failure Modes / Edge Cases

- **Unpriced models**: if no pricing tier matches, `matchPricingTier` returns null and test-run cost is null (`web/src/features/evals/v2/server/evaluators/testEvaluator.ts:178-179`); activation shows "Unavailable" with an explanatory line (`web/src/features/evals/v2/components/Evaluators/EvaluatorSavedDialog/EvaluatorSavedCostSummary.tsx:110-115`). Estimates never fabricate numbers — good safety property, but silent for already-ingested traffic.
- **Provided-total mismatches in usage details** are counted and rate-limited-warned rather than rejected (`worker/src/services/IngestionService/index.ts:1590-1614`), so malformed usage degrades cost accuracy quietly.
- **Empty-usage tier matching guard**: pricing-tier matching is skipped when usage is empty to avoid stamping a tier chosen against fabricated zero usage (`worker/src/services/IngestionService/index.ts:1306-1315`).
- **Retry exhaustion**: cost polling gives up after the bounded delay ladder and returns null (`web/src/features/evals/v2/server/evaluators/activationCostService.ts:201-210`); automatic fallback test failure is logged as warning, not surfaced as blocking (`:190-198`).
- **Judge recursion prevention**: OpenRouter-style provenance markers keep judge calls from being re-evaluated (`packages/shared/src/server/llm/llmText.ts:363-396`), protecting cost estimates from runaway loops.

## Future Considerations

- A first-class **cost-vs-quality comparison view** (e.g., per-model scatter of score vs avg cost/latency built on `DatasetRunsMetrics`) would convert existing primitives into the analysis this dimension asks for without new instrumentation.
- Promote **pass-rate to a named measure** (e.g., share of boolean-true / expected-category scores per evaluator) in the query data model, mirroring how `totalCost` is declared (`packages/shared/src/features/query/dataModel.ts:588-595`).
- Replace trace-name attribution with structured `evaluator_id` propagation everywhere (the v2 metadata work in `packages/shared/src/server/evals/evalExecutionMetadata.ts` points in this direction) to remove the rename fragility noted above.
- Surface **estimate staleness** (price-list edit timestamps) alongside activation estimates so users know when `testRunCostUsd` reflects outdated pricing.

## Questions / Gaps

- **Is there batch/offline benchmarking that compares models head-to-head?** Dataset experiments support arbitrary prompts/models per run (`worker/src/features/experiments/experimentServiceClickhouse.ts:212-233`) but the repo contains no dedicated model-A/B harness; search boundary: `worker/src/features/experiments`, `web/src/features/datasets`, `packages/shared/src/features/experiments`.
- **Does anything alert on eval spend itself?** Monitors can query any view metric including cost measures, but no pre-provisioned monitor targets evaluator spend specifically (searched `worker/src/constants/langfuse-dashboards.json` and `packages/shared/src/features/monitors`); the cloud-spend alerts in `worker/src/ee/cloudSpendAlerts` target platform usage, not user eval budgets. No evidence found beyond these boundaries.
- **No evidence found** for automated cost-regression gates in CI (contrast with dimension 18.03): searches across `scripts/`, `.github`-referenced tooling in `AGENTS.md`, and turbo/vitest configs show lint/typecheck/test/build verification but no cost-budget assertions.

---

Generated by `dimension 18.04-cost-latency-and-quality-evaluation` against `langfuse`.
