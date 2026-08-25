# Source Analysis: agent-framework

## 18.04 Cost, Latency, and Quality Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#) monorepo; evaluation framework implemented in both (`python/packages/core`, `dotnet/src/Microsoft.Agents.AI`) |
| Analyzed | 2026-08-25 |

> All file paths below are relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Agent Framework ships a provider-agnostic evaluation framework (ADR `docs/decisions/0023-foundry-evals-integration.md`) with a local, API-free evaluator and a Microsoft Foundry cloud evaluator. Within that framework, **success rate is a first-class, tested concept** (pass/fail/errored counts, per-evaluator breakdowns, CI gate methods), and **token counts are captured per eval item** — but only as counts, never converted to monetary cost. **Latency is entirely absent from the eval result types**; it exists only as runtime OpenTelemetry histograms (`gen_ai.client.operation.duration`) and ad-hoc timing in one end-to-end sample. **Model choice is factored in** as configuration (the judge model for Foundry evaluators, separate agent/judge models in the self-reflection sample) but there is no built-in mechanism to compare two model choices on cost vs quality; comparison is delegated to the external Foundry portal. The overall picture: quality/success-rate evaluation is mature, while the economic and temporal axes of evaluation are partial (tokens) or delegated to samples/OTel (latency) or absent (dollars).

## Rating

**5 / 10** — Present but inconsistent.

Rationale against the rubric:

- Success rate tracking is close to the 7–8 band on its own: typed result counts (`python/packages/core/agent_framework/_evaluation.py:440-468`), per-evaluator breakdowns (`python/packages/core/agent_framework/_evaluation.py:1570-1621`), explicit CI-gate interfaces (`raise_for_status`, `assert_score_at_least`, `_evaluation.py:470-543`), and dedicated tests.
- Token "cost" is measured but only as raw token counts, never priced (`EvalItemResult.token_usage`, `python/packages/core/agent_framework/_evaluation.py:353`; populated at `python/packages/foundry/agent_framework_foundry/_foundry_evals.py:615-622`). No pricing table or dollar aggregation exists anywhere in the repo (searches for `cost|price` across eval code return zero functional hits).
- Latency is not part of any eval API surface; it appears only in OTel runtime metrics and sample code.
- Model choice is parameterizable but not comparable: no harness produces side-by-side model results inside the framework.
- The dimension's headline question — "Can you compare two model choices on cost vs quality?" — cannot be answered with shipped framework tooling alone.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token usage field on eval items | `EvalItemResult.token_usage: dict[str, int]` documented as prompt/completion/total tokens | python/packages/core/agent_framework/_evaluation.py:340-341,353 |
| Token usage populated from provider | `_fetch_output_items` extracts `usage.prompt_tokens/completion_tokens/total_tokens/cached_tokens` from Foundry output items | python/packages/foundry/agent_framework_foundry/_foundry_evals.py:615-622,651 |
| Token usage tests | `test_with_token_usage` asserts `item.token_usage["total_tokens"] == 150`; usage-extraction test with mocked SDK sample | python/packages/foundry/tests/test_foundry_evals.py:2322-2331,2388-2437 |
| .NET token usage parity | `EvalItemResult.TokenUsage` property; `AgentEvaluationResults.DetailedItems` doc mentions token usage | dotnet/src/Microsoft.Agents.AI/Evaluation/EvalItemResult.cs:50-51 ; dotnet/src/Microsoft.Agents.AI/Evaluation/AgentEvaluationResults.cs:62-69 |
| No monetary cost anywhere | Grep for `cost\|price` across eval packages yields only incidental comments/MCP metadata tests | python/packages/core/tests/core/test_mcp.py:1196 |
| Runtime usage aggregation (not priced) | `UsageDetails` TypedDict with input/output/cache/reasoning token counts; `add_usage_details` merger used by loop/workflow aggregators | python/packages/core/agent_framework/_types.py:406-430 ; python/packages/core/agent_framework/_harness/_loop.py:539 |
| OTel token metric | `LLM_TOKEN_USAGE = "gen_ai.client.token.usage"` histogram with token bucket boundaries | python/packages/core/agent_framework/observability.py:275,1511-1516,156 |
| Latency absent from eval types | No duration/latency field on `EvalResults`/`EvalItemResult`; poll deadline is a control bound, not a recorded metric | python/packages/core/agent_framework/_evaluation.py:372-438 ; python/packages/foundry/agent_framework_foundry/_foundry_evals.py:385-426 |
| Runtime latency metric | `LLM_OPERATION_DURATION = "gen_ai.client.operation.duration"` histogram; durations computed via `perf_counter` around chat/embedding calls and function invocation | python/packages/core/agent_framework/observability.py:274,1502-1508,1896-1915 ; python/packages/core/agent_framework/_tools.py:763,798-801 |
| Sample-level latency capture | Self-reflection runner records `total_groundedness_eval_time` per iteration and `total_end_to_end_time` per prompt into persisted JSONL | python/samples/05-end-to-end/evaluation/self_reflection/self_reflection.py:147,158-161,196-197,211-212 |
| Success rate counts | `result_counts` with passed/failed/errored; properties `passed`, `failed`, `total`, `all_passed` | python/packages/core/agent_framework/_evaluation.py:440-468 |
| Provider-side success extraction | `_extract_result_counts` maps run result_counts (passed/failed/errored/total); `_extract_per_evaluator` builds per-evaluator pass/fail | python/packages/foundry/agent_framework_foundry/_foundry_evals.py:429-449 |
| Local evaluator success accounting | `LocalEvaluator.evaluate` tallies passed/failed per check and per item; item passes only if all checks pass; empty-check items fail by design | python/packages/core/agent_framework/_evaluation.py:1570-1621 |
| CI gates on success/scores | `raise_for_status`, `assert_score_at_least(min_score)`, `assert_dimension_score_at_least(dimension_id, min_score)`, `assert_no_failed_items` raise `EvalNotPassedError` | python/packages/core/agent_framework/_evaluation.py:470-500,502-543,545-614,616-642 |
| Consistency repetitions | `num_repetitions` runs each query N times to measure consistency; validated ≥1 | python/packages/core/agent_framework/_evaluation.py:1641,1680-1684,1754-1755,1791 |
| Sample-level success-rate summary | Batch summary prints pass/fail counts, average scores, improvement %, first-try % | python/samples/05-end-to-end/evaluation/self_reflection/self_reflection.py:364-414 |
| .NET success-rate parity | `AgentEvaluationResults.Passed/Failed/Total/AllPassed` computed from MEAI metric interpretations | dotnet/src/Microsoft.Agents.AI/Evaluation/AgentEvaluationResults.cs:71+ |
| Judge model configurable | `FoundryEvals(model=...)` → `initialization_parameters.deployment_name` per evaluator entry; judge model resolved from client or env | python/packages/foundry/agent_framework_foundry/_foundry_evals.py:220-308,784-808,841-849 |
| Agent vs judge model separation | Self-reflection CLI exposes `--agent-model` and `--judge-model`; each output record stamped with `agent_response_model` | python/samples/05-end-to-end/evaluation/self_reflection/self_reflection.py:80-81,219-220,252-266,319,429-439 |
| Comparison delegated to portal | `report_url` points at Foundry dashboards/comparison views; ADR names portal comparisons as the design goal | python/packages/core/agent_framework/_evaluation.py:383 ; docs/decisions/0023-foundry-evals-integration.md:41 |
| Cost knobs outside evals | Loop default iteration cap justified by "LLM-judged loops are costly"; `max_function_calls` documented as "primary knob for controlling cost" | python/packages/core/agent_framework/_harness/_loop.py:124 ; python/packages/core/agent_framework/_tools.py:1344 |

## Answers to Dimension Questions

### 1. Is token cost measured in evals?
**Partially — token counts yes, monetary cost no.** Every Foundry-backed eval item carries `token_usage` (`prompt_tokens`, `completion_tokens`, `total_tokens`, `cached_tokens`) extracted from the provider's output-items API (`python/packages/foundry/agent_framework_foundry/_foundry_evals.py:615-622`) into the typed field `EvalItemResult.token_usage` (`python/packages/core/agent_framework/_evaluation.py:353`), covered by unit tests (`python/packages/foundry/tests/test_foundry_evals.py:2322-2437`). However, no code multiplies tokens by price: searches for `cost`/`price` in all eval modules return nothing functional. There is no per-run cost roll-up on `EvalResults` either — token data lives only at the per-item level. The local `LocalEvaluator` path never touches tokens at all (its `EvalItemResult`s leave `token_usage=None`, `python/packages/core/agent_framework/_evaluation.py:1602-1610`).

### 2. Is latency measured?
**Not within the evaluation framework.** Neither `EvalResults` (`python/packages/core/agent_framework/_evaluation.py:372-438`) nor its .NET counterpart carries any duration field; the only timing in the Foundry provider is a poll timeout that bounds waiting (`python/packages/foundry/agent_framework_foundry/_foundry_evals.py:385-426`) and discards elapsed time. Latency is instead available through two side channels: (a) runtime OTel histograms `gen_ai.client.operation.duration` and `gen_ai.client.token.usage`, recorded around chat/embedding/function calls with `perf_counter` (`python/packages/core/agent_framework/observability.py:274-275,1502-1516,1896-1915`; `python/packages/core/agent_framework/_tools.py:763,798-801`); and (b) ad-hoc instrumentation in the self-reflection sample, which persists `total_groundedness_eval_time` and `total_end_to_end_time` per prompt (`python/samples/05-end-to-end/evaluation/self_reflection/self_reflection.py:147-161,196-197,211-212`). A storage-format micro-benchmark also reports round-trip latency (`python/scripts/session_serialization_benchmark.py:14-24`), but none of these feed eval results.

### 3. Is success rate tracked?
**Yes — this is the strongest axis.** Pass/fail/error counts are aggregated per run (`python/packages/core/agent_framework/_evaluation.py:440-468`), per evaluator (`_extract_per_evaluator`, `python/packages/foundry/agent_framework_foundry/_foundry_evals.py:442-449`; per-check dicts in `LocalEvaluator`, `python/packages/core/agent_framework/_evaluation.py:1572-1595`), and per sub-agent in workflow evals (`sub_results`, `python/packages/core/agent_framework/_evaluation.py:1986-2022`). The design treats errors distinctly from quality failures (`is_error` vs `is_failed`, `_evaluation.py:356-369`), which prevents infrastructure flakiness from polluting quality metrics. Operational safeguards are explicit: `raise_for_status()` and score-threshold assertions are documented CI gates (`_evaluation.py:470-543`), and `num_repetitions` supports consistency measurement (`_evaluation.py:1680-1684`). The self-reflection sample computes pass percentages and improvement statistics over batches (`self_reflection.py:364-414`). Tests cover both providers' counting behavior (e.g., `python/packages/foundry/tests/test_foundry_evals.py`; `python/packages/core/tests/core/test_local_eval.py:66-95`).

### 4. Are model tradeoffs analyzed?
**Model choice is factored as configuration, not analysis.** `FoundryEvals(model=...)` selects the LLM-judge deployment, threaded into each evaluator definition as `deployment_name` (`python/packages/foundry/agent_framework_foundry/_foundry_evals.py:247,281,792-793`), so judge-model sensitivity is experimentally controllable. The self-reflection sample goes further by separating agent model from judge model on the CLI and stamping `agent_response_model` into every persisted record (`python/samples/05-end-to-end/evaluation/self_reflection/self_reflection.py:80-81,258-266,319,429-439`) — enough to reconstruct a quality-vs-model comparison post hoc from JSONL outputs. But the framework itself provides no comparator: no API runs the same queries against two models and diffs scores, and no report joins quality scores with token or time costs. Cross-run comparison is explicitly delegated to the Foundry portal ("dashboards and comparison views", `docs/decisions/0023-foundry-evals-integration.md:41`; `report_url` at `python/packages/core/agent_framework/_evaluation.py:383`). Answering "can you compare two model choices on cost vs quality?" with only this repo: you can approximate quality deltas via repeated runs with different `model=` values plus `num_repetitions`, but the cost half of the comparison has no data source since dollars are never computed and latency never lands in eval results.

## Architectural Decisions

1. **Evaluator protocol with shared orchestration over a full eval framework** (ADR option 2): a minimal `Evaluator` protocol (`name` + `evaluate(items) -> EvalResults`, `python/packages/core/agent_framework/_evaluation.py:683-724`) keeps metric selection inside providers (`_evaluation.py:711-715`). Consequence for this dimension: whether cost/latency get measured depends entirely on each provider implementation; the orchestration layer adds no such telemetry of its own.
2. **Token usage modeled as an opaque `dict[str, int]`, not a priced type**: `token_usage` is a plain dict on `EvalItemResult` (`_evaluation.py:353`) mirroring the OpenAI evals API payload (`_foundry_evals.py:617-622`), while the richer typed usage model (`UsageDetails` with cache/reasoning counters, `python/packages/core/agent_framework/_types.py:406-427`) is reserved for runtime responses, not eval results. Pricing was deliberately left out of scope.
3. **Errors separated from failures** (`status ∈ {pass, fail, error}` with `error_code`, `_evaluation.py:330-369`) so success-rate math isn't corrupted by infra errors — an operational safeguard most naive harnesses lack.
4. **CI-first result consumption**: assertion-style gates (`raise_for_status`, `assert_score_at_least`, `assert_no_failed_items`) make success rate an actionable build signal rather than a dashboard-only number (`_evaluation.py:470-642`).
5. **Comparison views outsourced to the Foundry portal** rather than built in-repo (ADR `docs/decisions/0023-foundry-evals-integration.md:41`); the framework returns a `report_url` link.

## Notable Patterns

- **Provider-pluggable metrics with smart defaults**: `FoundryEvals` auto-selects relevance/coherence/task_adherence and injects tool-call accuracy when tools are present (`_resolve_default_evaluators` / `_filter_tool_evaluators`, `python/packages/foundry/agent_framework_foundry/_foundry_evals.py:334-382`) — quality dimensions adapt to item shape without caller effort.
- **Reproducibility warnings for judges**: versionless rubric-evaluator references trigger a warning recommending pinned versions for CI/replay stability (`GeneratedEvaluatorRef`, `_foundry_evals.py:73-102,249-257`) — an implicit acknowledgment that eval economics (re-runs) must be stable to be meaningful.
- **Consistency-by-repetition**: `num_repetitions` multiplies query coverage N× (`_evaluation.py:1791-1801`) to expose non-determinism, the closest built-in proxy for statistical confidence in quality numbers.
- **Dual-language parity**: the .NET port replicates token usage (`TokenUsage`, `dotnet/src/Microsoft.Agents.AI/Evaluation/EvalItemResult.cs:51`) and pass/fail aggregation (`AgentEvaluationResults.cs:71-74`), but inherits the same gaps (no latency, no cost).
- **Runtime observability decoupled from evals**: OTel histograms carry the latency/token telemetry stream (`observability.py:274-275`) that evals could consume but currently don't.

## Tradeoffs

- **Simplicity vs economic observability**: keeping the `Evaluator` protocol minimal (ADR decision drivers: "lowest concept count", `docs/decisions/0023-foundry-evals-integration.md:39`) means cost/latency instrumentation was consciously left to providers and callers. Teams get low-friction evals but must assemble their own cost story.
- **Portal delegation vs reproducibility**: leaning on Foundry dashboards for model comparison avoids building reporting UI, but ties comparative analysis to an external service and makes offline/CI cost-vs-quality regression tracking impossible without custom export code.
- **Per-item token detail vs run-level totals**: token counts exist only on `EvalItemResult`; consumers wanting a run budget must sum items themselves, and `LocalEvaluator`/post-hoc paths provide no usage at all.
- **Quality-vs-time feedback loop exists only in sample form**: the self-reflection runner demonstrates trading extra iterations (time + tokens) for higher groundedness scores (`self_reflection.py:151-194`), but this pattern is not productized into the framework.

## Failure Modes / Edge Cases

- **Silent loss of token data**: `_fetch_output_items` swallows `AttributeError/KeyError/TypeError` when parsing output items and logs a warning, returning whatever parsed so far (`_foundry_evals.py:654-655`) — usage data can vanish without failing the eval.
- **Poll timeout discards evidence**: a timed-out run returns bare `EvalResults(status="timeout")` with no counts or partial scores (`_foundry_evals.py:423-424`); `all_passed` then returns `False` because status ≠ completed (`_evaluation.py:462-468`), but the operator learns nothing about cost/time spent before timeout.
- **Empty-token guard**: token usage is only recorded when `usage.total_tokens` is truthy (`_foundry_evals.py:616`) — legitimately zero-token items are indistinguishable from missing data.
- **Judge-model drift**: unpinned `GeneratedEvaluatorRef.latest()` silently resolves to current-at-runtime versions (`_foundry_evals.py:95-102`), degrading cross-model comparisons made at different times.
- **Latency blind spot under repetition**: `num_repetitions` multiplies API spend N× (`_evaluation.py:1791`) with no cost/latency feedback in the result object, so an aggressive repetition setting is invisible until the invoice arrives.

## Future Considerations

- Add run-level aggregates (`total_tokens`, wall-clock duration) to `EvalResults` and thread them from both the Foundry output-items API and the existing OTel histograms (`observability.py:1502-1516`).
- Introduce an optional pricing table keyed by deployment name so `UsageDetails` can be projected to dollar cost at eval time; this would make the framework able to answer its own cost/quality question.
- Productize the self-reflection pattern: a `compare_models(agent_factory, models=[...])` helper running identical query sets per model with shared seeds/repetitions, emitting paired `EvalResults`.
- Persist latency per item (start/end or duration) in `EvalItemResult`, mirroring how `token_usage` was added, including on the .NET side for parity.
- Surface timeout/partial-failure economics: include elapsed polling time and items-completed-so-far in timeout results.

## Questions / Gaps

- **Unanswered — monetary cost**: No evidence found of any dollar-cost computation, pricing config, or billing integration. Searches: `cost`, `price`, `usd` across `python/packages/**` (eval modules) and `dotnet/src/**/Evaluation` — only incidental hits (an MCP `_meta` test fixture at `python/packages/core/tests/core/test_mcp.py:1196`).
- **Unanswered — eval-level latency**: No evidence found of latency fields or recording within the eval framework. Searches: `latency`, `elapsed`, `duration_seconds`, `perf_counter` restricted to `packages/*/agent_framework/*eval*` files — zero functional hits inside eval types; all matches live in observability, tools, shell, or samples.
- **Unanswered — built-in model comparison**: No evidence found of a comparator harness or diffing/report utility for multi-model runs. Search boundary: `python/samples/05-end-to-end/evaluation/**`, `python/packages/{core,foundry}`, `dotnet/src/Microsoft.Agents.AI/Evaluation`. The nearest capability is per-record model stamping in the self-reflection sample (`python/samples/05-end-to-end/evaluation/self_reflection/self_reflection.py:319`) plus portal-side comparison views.
- **Open (upstream)**: the ADR lists open questions on datasets and red-teaming callbacks but does not discuss cost/latency metrics at all (`docs/decisions/0023-foundry-evals-integration.md:552-556`), suggesting the economic dimension is not yet on the design roadmap.

---

Generated by `18.04-cost-latency-and-quality-evaluation` against `agent-framework`.
