# Source Analysis: pydantic-ai

## Dimension 18.04: Cost, Latency, and Quality Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (uv workspace: `pydantic_ai_slim`, `pydantic_evals`, `pydantic_graph`, `clai`) |
| Analyzed | 2026-08-25 |

## Summary

pydantic-ai ships a dedicated evaluation framework, `pydantic-evals` (`pydantic_evals/`), that captures cost, latency, and quality signals per evaluation case by harvesting OpenTelemetry span attributes produced by the framework's own instrumentation. Token cost flows end-to-end: model instrumentation prices each response with the `genai-prices` library and records `operation.cost` on request spans (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:414-415`), and the eval harness extracts it — along with token counts and request counts — into per-case `metrics` (`pydantic_evals/pydantic_evals/_task_run.py:59-74`). Latency is captured twice: wall-clock duration per case via `time.perf_counter()` (`pydantic_evals/pydantic_evals/_task_run.py:48,53`) and per-span timing from OTel timestamps (`pydantic_evals/pydantic_evals/otel/span_tree.py:106-108`); time-to-first-chunk is additionally emitted as an OTel histogram (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:189-203`). Success rate is computed as averaged boolean assertions and exported as an `assertion_pass_rate` span attribute (`pydantic_evals/pydantic_evals/dataset.py:1058-1059`). Model choice, however, is not a first-class report dimension: reports carry no model field, and cross-model comparison is left to manual baseline diffing of two separately-run reports. Cost/quality tradeoff analysis is possible (baseline diff renders scores, metrics including cost, assertions, and durations side by side) but is not automated.

## Rating

**7 / 10**

Rationale against the rubric:

- **Clear model with tests and explicit interfaces (7-8 band).** The cost/latency/success pipeline is explicit and typed: `TaskRun.metrics` accumulation (`pydantic_evals/pydantic_evals/_task_run.py:14-32`), `EvaluatorContext.duration/metrics/attributes` (`pydantic_evals/pydantic_evals/evaluators/context.py:66-84`), and `ReportCase.task_duration/total_duration/metrics/scores/assertions` (`pydantic_evals/pydantic_evals/reporting/__init__.py:101-109`). Extraction is pinned by tests asserting exact metric dicts (`tests/evals/test_dataset.py:933-941`). Operational safeguards exist: pricing never fails a run (`best_effort_price` degrades to `None`, `pydantic_ai_slim/pydantic_ai/_cost.py:62-95`), missing OTel degrades to `SpanTreeRecordingError` rather than crashing evaluators (`pydantic_evals/pydantic_evals/evaluators/agentic.py:532-536`), and "unknown" cost is kept distinct from zero (`pydantic_ai_slim/pydantic_ai/usage.py:130-135`).
- **Why not 8-9:** model choice is only implicitly factored — no model identity is stored on `ReportCase` (`pydantic_evals/pydantic_evals/reporting/__init__.py:87-120` has no such field; `gen_ai.request.model` is used solely as a span filter at `pydantic_evals/pydantic_evals/_task_run.py:62`), so "which model produced this report" is only as good as user-supplied experiment metadata. There is no built-in multi-model orchestration or cost-vs-quality comparison artifact; evals also do not warn when cost cannot be computed (unlike runtime `cost_limit`, which emits `CostNotFoundWarning`, `pydantic_ai_slim/pydantic_ai/usage.py:528-535`).

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Cost extraction in evals | `extract_span_tree_metrics` walks spans carrying `gen_ai.request.model` and increments a `cost` metric from the `operation.cost` attribute | `pydantic_evals/pydantic_evals/_task_run.py:59-74` |
| Metric accumulator | `TaskRun.increment_metric` accumulates numeric metrics per run | `pydantic_evals/pydantic_evals/_task_run.py:21-29` |
| Cost attribute production | `response_attributes()` sets `attributes['operation.cost'] = float(price_calculation.total_price)` on model-request spans | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:399-420` |
| Pricing engine | `best_effort_price()` computes USD cost via `genai_prices.calc_price`, returning `None` on unknown models/providers so "pricing must never fail a run" | `pydantic_ai_slim/pydantic_ai/_cost.py:62-95` |
| Cost histogram | `InstrumentationSettings.__init__` creates an `operation.cost` OTel histogram with unit `{USD}` | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:184-188` |
| Embeddings cost | Instrumented embeddings set `operation.cost` from price calculations | `pydantic_ai_slim/pydantic_ai/embeddings/instrumented.py:162-163` |
| Cost on usage objects | `UsageBase.cost: Decimal \| None = None` — best-effort USD, `None` distinguishes unknown from zero; summed across requests by `_incr_usage_cost` | `pydantic_ai_slim/pydantic_ai/usage.py:130-135`, `pydantic_ai_slim/pydantic_ai/usage.py:393-395` |
| Eval-side cost test | `test_genai_attribute_collection` asserts `metrics={'cost': 1.23, 'requests': 1, 'input_tokens': 1, 'special_tokens': 2}` extracted from synthetic chat spans | `tests/evals/test_dataset.py:914-941` |
| Instrumented-span cost test | Snapshot asserts `'operation.cost': 0.002225` among recorded model-request span attributes | `tests/models/test_instrumented.py:294` |
| Wall-clock latency | `run_task()` times each case with `time.perf_counter()` and exposes `duration` to evaluators | `pydantic_evals/pydantic_evals/_task_run.py:48-53` |
| Duration in evaluator context | `EvaluatorContext.duration: float` ("The duration of the task run for this case") | `pydantic_evals/pydantic_evals/evaluators/context.py:66-67` |
| Report durations | `ReportCase.task_duration` and `total_duration` (including evaluator execution) | `pydantic_evals/pydantic_evals/reporting/__init__.py:108-109` |
| Duration averaging | `ReportCaseAggregate.average` computes average task/total durations across cases | `pydantic_evals/pydantic_evals/reporting/__init__.py:229-230` |
| Per-span timing | `SpanNode.start_timestamp/end_timestamp` with a `duration` property; `SpanQuery.min_duration/max_duration` filters enable latency-aware evaluators | `pydantic_evals/pydantic_evals/otel/span_tree.py:99-108`, `pydantic_evals/pydantic_evals/otel/span_tree.py:57-58` |
| TTFT metric | `gen_ai.client.operation.time_to_first_chunk` streaming-latency histogram | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:189-203` |
| Duration rendering + diff | `_render_durations` / `_render_durations_diff` format task vs total durations, including against a baseline | `pydantic_evals/pydantic_evals/reporting/__init__.py:1268-1294` |
| Success rate calculation | `average_assertions = n_passing / n_assertions` in `ReportCaseAggregate.average` | `pydantic_evals/pydantic_evals/reporting/__init__.py:241-245` |
| Pass-rate telemetry export | `_set_experiment_span_attributes` sets `assertion_pass_rate` on the experiment span | `pydantic_evals/pydantic_evals/dataset.py:1056-1060` |
| Assertion rendering | Aggregate assertion pass rate rendered as percentage with check mark | `pydantic_evals/pydantic_evals/reporting/__init__.py:1358-1366` |
| Score emission semantics | Bool evaluator results emit both `score.value` (1.0/0.0) and `score.label` (`pass`/`fail`) as OTel log attributes for numeric/categorical queries | `pydantic_evals/pydantic_evals/_otel_emit.py:218-232` |
| Task failure capture | `ReportCaseFailure` records error message/stacktrace for cases where task execution raised | `pydantic_evals/pydantic_evals/reporting/__init__.py:123-148` |
| Multi-run aggregation | `repeat > 1` groups runs by `source_case_name`; two-level averaging (per-group then cross-group) for stochastic variance | `pydantic_evals/pydantic_evals/reporting/__init__.py:343-392`, `pydantic_evals/pydantic_evals/reporting/__init__.py:257-313` |
| Baseline diff API | `EvaluationReport.render(baseline=...)` / `console_table(baseline=...)` diff scores, metrics (incl. cost), labels, assertions, durations vs another report | `pydantic_evals/pydantic_evals/reporting/__init__.py:394-451`, `pydantic_evals/pydantic_evals/reporting/__init__.py:1160-1242` |
| Experiment metadata tagging | `Dataset.evaluate(metadata=...)` accepts arbitrary per-experiment metadata (the intended hook for tagging model choice); rendered with colored add/remove/change diff | `pydantic_evals/pydantic_evals/dataset.py:281-316`, `pydantic_evals/pydantic_evals/reporting/__init__.py:642-682` |
| Budget-as-evaluator | `MaxToolCalls` (counts failed attempts by default since "they still consume budget") and `MaxModelRequests` (prefers `ctx.metrics['requests']`) gate resource usage | `pydantic_evals/pydantic_evals/evaluators/agentic.py:510-542`, `pydantic_evals/pydantic_evals/evaluators/agentic.py:545-579` |
| Quality analyses | Built-in report evaluators produce confusion matrices, precision-recall, ROC/AUC, KS plots — quality statistics with no cost axis | `pydantic_evals/pydantic_evals/evaluators/report_common.py:23-29`, `pydantic_evals/pydantic_evals/reporting/analyses.py:21-130` |
| Runtime cost enforcement (contrast) | `UsageLimits.cost_limit` raises `UsageLimitExceeded`; warns `CostNotFoundWarning` when a limit is set but no cost could be calculated | `pydantic_ai_slim/pydantic_ai/usage.py:427-428`, `pydantic_ai_slim/pydantic_ai/usage.py:516-535` |
| Docs: run cost capping | "Capping run cost" section documents `cost_limit` and its best-effort caveat ("Don't rely on `cost_limit` as a hard billing guarantee") | `docs/agent.md:983-1009` |
| Docs: metrics recording | Users can `increment_eval_metric('tokens_used', ...)` inside tasks; metrics surface in reports and evaluator contexts | `docs/evals/how-to/metrics-attributes.md:16-48` |

## Answers to Dimension Questions

1. **Is token cost measured in evals?** **Yes.** The eval harness automatically extracts a `cost` metric from the `operation.cost` span attribute on every instrumented model/embedding request (`pydantic_evals/pydantic_evals/_task_run.py:69-70`), which the agent framework itself produces by pricing responses with genai-prices (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:414-415`; `pydantic_ai_slim/pydantic_ai/_cost.py:44-59`). Costs are averaged into the report's Averages row like any other metric (`pydantic_evals/pydantic_evals/reporting/__init__.py:239`) and participate in baseline diffs (`pydantic_evals/pydantic_evals/reporting/__init__.py:1180-1187`). Caveat: if OTel is not configured or the model/provider is unpriced, `cost` is silently absent from metrics (extraction requires the span tree, `_task_run.py:55-56`; pricing returns `None`, `_cost.py:88-92`) — evals emit no warning analogous to runtime's `CostNotFoundWarning`.
2. **Is latency measured?** **Yes, at three granularities:** wall-clock per-case duration (`_task_run.py:48,53` → `context.py:66-67` → `reporting/__init__.py:108-109`), per-span durations from OTel timestamps queryable via `min_duration/max_duration` (`otel/span_tree.py:57-58,106-108`), and streaming TTFT histograms (`models/instrumented.py:189-203`). Reports render human-formatted durations with optional baseline diffs (`reporting/__init__.py:1268-1294`) and average them across cases and multi-run groups (`reporting/__init__.py:229-230,301-312`).
3. **Is success rate tracked?** **Yes.** Boolean evaluator outputs become `assertions`; their pass ratio is computed per aggregate (`n_passing/n_assertions`, `reporting/__init__.py:241-245`), rendered as a percentage (`reporting/__init__.py:1358-1366`), and exported to OTel as `assertion_pass_rate` on the experiment span (`dataset.py:1058-1059`). Hard task failures are tracked separately as `ReportCaseFailure` entries with stacktraces (`reporting/__init__.py:123-148`). For stochastic tasks, `repeat=N` plus `case_groups()` gives per-case success distributions across runs (`dataset.py` repeat handling; `reporting/__init__.py:343-378`).
4. **Are model tradeoffs analyzed?** **Only partially, and manually.** No built-in mechanism compares two models. The primitive exists: run the same dataset twice under different models, tag each run via `Dataset.evaluate(metadata={'model': ...})` (`dataset.py:291`), and diff the reports with `render(baseline=...)`, which juxtaposes scores/assertions (quality) against cost metrics and durations (`reporting/__init__.py:1160-1242`). But `ReportCase` carries no model identity (`reporting/__init__.py:87-120`), there is no N-model runner, no combined cost-vs-quality table, and the quality-analysis suite (ROC/PR/confusion matrix, `report_common.py:23-29`) has no cost axis. Answering "can you compare two model choices on cost vs quality?" — yes, but only by assembling the comparison yourself from the baseline-diff primitive.

## Architectural Decisions

- **Telemetry-first metric collection.** Rather than threading usage objects through the task boundary, evals harvest standard GenAI OTel semantic-convention attributes (`gen_ai.operation.name`, `operation.cost`, `gen_ai.usage.*`) from an in-memory span subtree (`pydantic_evals/pydantic_evals/_task_run.py:50-74`, `pydantic_evals/pydantic_evals/otel/_context_subtree.py`). This decouples `pydantic-evals` from pydantic-ai specifics: any code emitting these attributes (including hand-written test spans, as in `tests/evals/test_dataset.py:917-926`) feeds the same metrics.
- **Pricing delegated to a data snapshot, never fatal.** All internal pricing goes through `best_effort_price`, which maps known failures (`LookupError` for unpriced models, `ValueError` for unpriceable usage) to `None` and unexpected ones to a warning (`pydantic_ai_slim/pydantic_ai/_cost.py:70-95`). The design comment at `usage.py:131-135` codifies the invariant that unknown ≠ zero.
- **Degrade-don't-fail telemetry.** When OpenTelemetry is unavailable or the tracer provider is incompatible, the span tree becomes a `SpanTreeRecordingError` carried on `EvaluatorContext._span_tree` and raised only when an evaluator actually touches it (`evaluators/context.py:68-103`); span-based evaluators catch it and return a failing-but-explained result (`evaluators/agentic.py:532-536`).
- **Metrics as open dict, averages as generic fold.** `metrics: dict[str, int | float]` is schema-free, so new cost/token metrics require no report-schema change; aggregation averages whatever keys exist (`reporting/__init__.py:208-239`).
- **Baseline diff as the comparison substrate.** Instead of a bespoke A/B feature, all comparisons (and therefore any model comparison) route through one renderer supporting diffs of scores, labels, metrics, assertions, durations, and metadata (`reporting/__init__.py:1559+`, metadata panel `642-682`).

## Notable Patterns

- **Budget evaluators read from two sources with agreement guarantees.** `MaxModelRequests` prefers `ctx.metrics['requests']` and falls back to counting spans directly, documenting that "both use the same criteria, so the two sources agree whenever both are populated" (`pydantic_evals/pydantic_evals/evaluators/agentic.py:546-579`).
- **Failure-inclusive budgeting.** `MaxToolCalls` counts failed tool attempts by default because "they still consume budget... time and tokens," with `include_failed=False` as opt-out (`pydantic_evals/pydantic_evals/evaluators/agentic.py:22-30,511-542`) — a deliberate stance that resource accounting must include waste.
- **Dual-representation score emission.** Booleans emit as both numeric `score.value` (0.0/1.0) and categorical `score.label` (`pass`/`fail`) so backends can query either way (`pydantic_evals/pydantic_evals/_otel_emit.py:218-232`).
- **Multi-run variance handling.** `repeat` produces indexed case names with `source_case_name` preserved as the aggregation key, and averaging happens per-group then cross-group (`reporting/__init__.py:111-113,343-392`) — acknowledging LLM stochasticity in success-rate math.
- **Cost visibility beyond evals.** The same pricing machinery powers docs-level guidance on bounding spend (`docs/agent.md:983-1009`) and per-agent observability recommendations ("End-to-end latency broken down by agent; Token usage and costs per agent", `docs/multi-agent-applications.md:401-402`).

## Tradeoffs

- **OTel coupling vs portability.** Anchoring metrics to span attributes makes the harness provider-agnostic and free of plumbing through user tasks, but means every metric depends on tracer configuration; without it, cost/tokens/requests silently vanish while scores/assertions still work (`evaluators/context.py:68-73`).
- **Third-party pricing data vs accuracy.** genai-prices keeps the library out of the pricing-update business, but coverage gaps yield `None` costs (`_cost.py:73-76`); eval reports don't distinguish "model was cheap" from "model couldn't be priced" unless the user checks.
- **Open metric dicts vs typed contracts.** Adding metrics needs no schema churn, but nothing type-level prevents typos like `cst` vs `cost` from fragmenting aggregates.
- **Generic baseline diff vs purpose-built model comparison.** One diff renderer serves all comparison needs (low maintenance), yet answering the dimension's headline question ("compare two model choices on cost vs quality") requires manual orchestration and discipline about metadata tagging.

## Failure Modes / Edge Cases

- **Uninstrumented tasks lose all resource metrics silently.** If the task doesn't run under a capturing tracer provider, `extract_span_tree_metrics` never sees spans; `metrics` ends up `{}` and averages show no `cost` row (`pydantic_evals/pydantic_evals/_task_run.py:55-56`).
- **Unpriced models contribute zero rows, not zero dollars.** `LookupError` → `None` cost (`_cost.py:88-92`); a mixed-model comparison where only one model has pricing data would mislead a naive reader of the diffed `cost` metric.
- **Aggregated usage double-count risk is explicitly managed.** Run spans report `gen_ai.aggregated_usage.*` while request spans keep `gen_ai.usage.*` to avoid backends summing parent+child usage twice (`docs/changelog.md:184`; attribute naming at `models/instrumented.py:140-142`), and `details` keys colliding with first-class token attributes are suppressed to prevent consumers double-counting tokens/cost (`usage.py:21-26,244-248`).
- **Multi-request summation footgun documented.** `RequestUsage.__add__` warns it "CANNOT be used to sum multiple requests without breaking some pricing calculations" (`usage.py:292-301`) — per-request pricing must precede aggregation.
- **Cost limits are advisory.** Docs state plainly: "Don't rely on `cost_limit` as a hard billing guarantee" (`docs/agent.md:1009`); unpriced runs warn but proceed (`usage.py:528-535`).
- **Streaming cancellation yields partial usage.** Cancelled streams may never deliver final usage events, so post-cancellation cost figures are unreliable (`docs/agent.md:838`).

## Future Considerations

- Record model identity per case (e.g., propagate `gen_ai.request.model` from the span tree into `ReportCase.attributes` or a dedicated field) so reports self-describe what they measured; today the filter value at `pydantic_evals/pydantic_evals/_task_run.py:62` is discarded after use.
- Add a first-class multi-report/multi-model comparison helper (N baselines producing a cost-vs-quality table) on top of the existing diff renderer.
- Emit a warning in evals when a task appears to make model requests (spans with `gen_ai.request.model`) but no `operation.cost` was extractable — mirroring the runtime `CostNotFoundWarning` behavior (`usage.py:528-535`).
- Surface cache economics in eval summaries: `cache_hit_ratio` already exists on usage objects (`usage.py:203-216`), but evals currently extract raw token details without a derived cost-efficiency view.

## Questions / Gaps

- No evidence found of any benchmark suite within the repo that compares pydantic-ai model choices on cost vs quality (searched `pydantic_evals/`, `examples/`, `tests/evals/`, and `docs/evals/` for terms like "baseline", "benchmark", "compare"; the only baseline hits are report-diff APIs and a ROC random-baseline diagonal in `docs/evals/evaluators/report-evaluators.md:252,278`).
- Whether Logfire-hosted dashboards provide cross-experiment cost/quality roll-ups could not be verified from the source alone; the repo only shows the span/log emission side (`pydantic_evals/pydantic_evals/_otel_emit.py:140-144`, `dataset.py:1044-1066`).
- The `examples/` directory was not exhaustively audited for eval examples demonstrating baseline workflows; conclusions about manual-only model comparison rest on the absence of any comparison orchestrator in `pydantic_evals/`.

---

Generated by `18.04-cost-latency-and-quality-evaluation` against `pydantic-ai`.
