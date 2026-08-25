# Source Analysis: crewai

## Dimension 18.04: Cost, Latency, and Quality Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo: `lib/crewai` core, `lib/cli`, `lib/crewai-tools`, `lib/crewai-core`, `lib/crewai-files`, `lib/devtools`) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI has two distinct evaluation stacks. The stable one is the `crewai test` CLI flow (`lib/cli/src/crewai_cli/evaluate_crew.py:9-41`), which runs a crew N times and produces an LLM-judged quality table (scores 1–10 per task per run) plus an "Execution Time (s)" row (`lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:95-175`). Each test result also emits a `CrewTestResultEvent` carrying `quality`, `execution_duration`, and `model` (`lib/crewai/src/crewai/events/types/crew_events.py:104-110`), which is forwarded to an OpenTelemetry telemetry span (`lib/crewai/src/crewai/events/event_listener.py:228-235`, `lib/crewai/src/crewai/telemetry/telemetry.py:696-725`). The second stack is an explicitly experimental evaluation framework under `lib/crewai/src/crewai/experimental/evaluation/`: dataset-driven `ExperimentRunner` with pass/fail assertions, baseline comparison, regression detection, and a rendered "Success Rate" percentage.

Token usage is tracked thoroughly at runtime — `UsageMetrics` covers total/prompt/completion/cached/reasoning/cache-creation tokens and successful requests (`lib/crewai/src/crewai/types/usage_metrics.py:32-63`) — but it is *not* surfaced in the eval quality table: `print_crew_evaluation_result()` accepts a `token_usage` parameter that its body never reads (`lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:95-96`). Monetary cost (dollars) is never computed anywhere; the only pricing data source in the repo (LiteLLM's `model_prices_and_context_window.json` feed) feeds the CLI model-picker catalog, not any cost analysis (`lib/cli/src/crewai_cli/model_catalog.py:12-15`, `lib/crewai/src/crewai/constants.py:350`). Model choice is recorded as a label on test events and is selectable via the CLI flag, but there is no built-in mechanism that compares two models' cost vs quality side by side; the docs defer systematic model comparison to the hosted CrewAI AMP platform (`docs/edge/en/learn/llm-selection-guide.mdx:604-616`).

## Rating

**5 / 10 — Present but inconsistent, weakly documented, or fragile.**

Rationale against the rubric:

- Latency is genuinely measured and displayed in eval output (execution-time row asserted by tests at `lib/crewai/tests/utilities/evaluators/test_crew_evaluator_handler.py:120`), which clears the "absent" band.
- Success rate exists with baseline/regression tooling (`lib/crewai/src/crewai/experimental/evaluation/experiment/result_display.py:14-27`, `testing.py:13-53`), but the whole framework lives under `experimental/`, has thin test coverage (only four runner tests plus one result-comparison test), and is not referenced from the main testing docs (`docs/edge/en/concepts/testing.mdx` documents only `crewai test`).
- Token counts are robust at runtime yet absent from every eval report surface; the dead `token_usage` parameter (`crew_evaluator_handler.py:96`) suggests a wired-but-abandoned integration.
- No monetary cost computation and no multi-model comparison report exist anywhere in the source.

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to the selected source root (`studies/agent-harness-study/sources/crewai/`).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token metrics type | `UsageMetrics` fields: total/prompt/cached_prompt/completion/reasoning/cache_creation tokens + successful_requests; provider-dict normalizer reconciles Anthropic cache keys "so prompt_tokens and total_tokens don't undercount billed usage" | `lib/crewai/src/crewai/types/usage_metrics.py:32-63`, `usage_metrics.py:111-140` |
| Token counting callback | `TokenCalcHandler.log_success_event` sums successful requests, prompt/completion/cached tokens from litellm responses | `lib/crewai/src/crewai/utilities/token_counter_callback.py:37-66` |
| Legacy token accumulator | `TokenProcess` counters + `get_summary()` returning `UsageMetrics` | `lib/crewai/src/crewai/agents/agent_builder/utilities/base_token_process.py:8-38` |
| LLM-instance lifetime totals | `_track_token_usage_internal` accumulates into `_token_usage`; `get_token_usage_summary()` returns lifetime `UsageMetrics`; docstring describes snapshot+`delta_since` for per-call scoping | `lib/crewai/src/crewai/llms/base_llm.py:955-986` |
| Crew-level token aggregation | `Crew.calculate_usage_metrics()` sums each agent LLM's summary into `self.usage_metrics` after kickoff | `lib/crewai/src/crewai/crew.py:2201-2225`, kickoff wiring `crew.py:1065`, `crew.py:1930` |
| Flow-level token aggregation | Flow wires scoped `LLMCallCompletedEvent` listener; accumulates `event.usage` into `UsageMetrics` correlated by flow-id contextvar | `lib/crewai/src/crewai/flow/runtime/__init__.py:879-911` |
| Tokens NOT in eval report | `print_crew_evaluation_result(token_usage: list[dict] \| None = None)` — parameter accepted but never referenced in body (dead parameter) | `lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:95-175` |
| Eval latency capture | `Task.execution_duration` property = end_time − start_time seconds | `lib/crewai/src/crewai/task.py:603-607` |
| Eval latency reporting | `CrewEvaluator.run_execution_times` per iteration; rendered as "Execution Time (s)" row with per-run values and average | `lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:49`, `:164-171` |
| Eval event carries latency+model | `CrewTestResultEvent(quality, execution_duration, model)` emitted per evaluated task | `lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:206-215`; schema `lib/crewai/src/crewai/events/types/crew_events.py:104-110` |
| Telemetry of test results | `individual_test_result_span(crew, quality, exec_time, model_name)` sets span attrs `quality`, `exec_time`, `model_name`; handler subscribes on `CrewTestResultEvent` | `lib/crewai/src/crewai/telemetry/telemetry.py:696-725`; `lib/crewai/src/crewai/events/event_listener.py:228-235` |
| Per-LLM-call timing in eval traces | `EvaluationTraceCallback` records `start_time`/`end_time` per LLM call and agent trace span, plus `total_tokens` from response usage | `lib/crewai/src/crewai/experimental/evaluation/evaluation_listener.py:284-290`, `:308-329`, `:176-208` |
| Efficiency evaluator uses tokens+time | `ReasoningEfficiencyEvaluator` computes total_tokens, avg tokens/call, avg time-between-calls, loop likelihood from timing consistency; feeds them into LLM-judge prompt | `lib/crewai/src/crewai/experimental/evaluation/metrics/reasoning_metrics.py:67-100`, `:351-383` |
| Success rate calculation | Experiment display renders Total/Passed/Failed/"Success Rate" percent | `lib/crewai/src/crewai/experimental/evaluation/experiment/result_display.py:14-27` |
| Pass/fail scoring rules | `ExperimentRunner._assert_scores`: actual ≥ expected across scalar/dict score shapes; errors become failed result with score 0.0 | `lib/crewai/src/crewai/experimental/evaluation/experiment/runner.py:130-165`, `:104-112` |
| Baseline comparison & regression | `compare_with_baseline` classifies improved/regressed/unchanged/new/missing vs persisted JSON baseline; `assert_experiment_successfully` / `assert_experiment_no_regression` raise on failures/regressions | `lib/crewai/src/crewai/experimental/evaluation/experiment/result.py:48-143`; `lib/crewai/src/crewai/experimental/evaluation/testing.py:13-53` |
| Model selection in test command | `crewai test --n-iterations N --model M` → `evaluate_crew(n_iterations, model)` → `uv run test N M` → `crew().test(n_iterations, eval_llm=M)` | `lib/cli/src/crewai_cli/cli.py:600-616`; `lib/cli/src/crewai_cli/evaluate_crew.py:9-31`; `lib/cli/src/crewai_cli/templates/crew/main.py:51-61`; `lib/crewai/src/crewai/crew.py:2227-2259` |
| Separate eval LLM supported | `Crew.test(n_iterations, eval_llm)` creates judge LLM via `create_llm(eval_llm)` distinct from crew models | `lib/crewai/src/crewai/crew.py:2227-2240`; `CrewEvaluator.__init__(llm=...)` `lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:39-47` |
| Pricing data used only for catalog | LiteLLM `model_prices_and_context_window.json` feed consumed by CLI model picker (three-tier model listing), never for cost math | `lib/cli/src/crewai_cli/model_catalog.py:12-21`, `lib/cli/src/crewai_cli/constants.py:356`, `lib/crewai/src/crewai/constants.py:350` |
| Docs: test output table | Testing docs show scores table incl. "Execution Time (s)" row; no token or cost column documented; default model gpt-4o-mini | `docs/edge/en/concepts/testing.mdx:14`, `:39-54` |
| Docs: qualitative cost guidance | LLM-selection guide discusses speed/cost trade-offs of reasoning vs efficient models, multi-model crews, "Consider Total Cost" step — prose only, no tooling | `docs/edge/en/learn/llm-selection-guide.mdx:90-142`, `:200`, `:489`, `:588-590` |
| Docs defer model comparison to SaaS | "teams serious about optimizing their LLM selection" pointed to CrewAI AMP platform testing | `docs/edge/en/learn/llm-selection-guide.mdx:604-616` |
| Tests: eval table incl. execution time | `test_print_crew_evaluation_result` asserts `"Execution Time (s)", "135", "155", "145"` row | `lib/crewai/tests/utilities/evaluators/test_crew_evaluator_handler.py:81-125` |
| Tests: experiment success path | Runner tests cover success, unknown-metric handling, single-metric expected-score matching | `lib/crewai/tests/experimental/evaluation/test_experiment_runner.py:53`, `:112`, `:142`, `:177` |
| Tests: flow usage aggregation | `test_flow_usage_metrics.py` asserts totals/prompt/completion/successful_requests from emitted LLM events, copy-safety, pause/resume behavior | `lib/crewai/tests/test_flow_usage_metrics.py:228-231`, `:262-275` |
| Kickoff TUI shows tokens+elapsed (non-eval) | Run summary appends `↑in ↓out tokens` and elapsed seconds after kickoff — adjacent capability not connected to eval reports | `lib/cli/src/crewai_cli/run_crew.py:549-572` |

## Answers to Dimension Questions

### 1. Is token cost measured in evals?

**Partially — token counts yes, monetary cost no.** Token counting is deep and multi-layered: per-provider normalization (`lib/crewai/src/crewai/types/usage_metrics.py:142-189`), litellm callback (`lib/crewai/src/crewai/utilities/token_counter_callback.py:37-66`), LLM-instance lifetime counters (`lib/crewai/src/crewai/llms/base_llm.py:955-986`), crew aggregation (`lib/crewai/src/crewai/crew.py:2201-2225`), and flow-scoped event aggregation (`lib/crewai/src/crewai/flow/runtime/__init__.py:879-911`). Inside evaluation specifically, tokens appear only as inputs to the reasoning-efficiency judge (`total_tokens`, `avg_tokens_per_call`, `lib/crewai/src/crewai/experimental/evaluation/metrics/reasoning_metrics.py:67-69`) and in eval trace records (`evaluation_listener.py:325`). The `crewai test` quality table shows no token column, and the `token_usage` parameter of `print_crew_evaluation_result` is accepted but unused (`lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:95-96`) — evidence of an intended-but-unwired integration. A repo-wide search for monetary-cost constructs (`usd`, `price`, `pricing`, `cost_per`, `$NNN`) found no cost computation anywhere; the only pricing feed (LiteLLM JSON) powers the CLI model catalog (`lib/cli/src/crewai_cli/model_catalog.py:12-15`). Despite the field name `token_cost_process` (`token_counter_callback.py:28`), it holds counts, not currency.

### 2. Is latency measured?

**Yes, in both eval paths.** The stable path derives per-task wall time from `Task.execution_duration` (`lib/crewai/src/crewai/task.py:603-607`), aggregates it per run in `run_execution_times` (`crew_evaluator_handler.py:49`), prints an averaged "Execution Time (s)" row (`:164-171`), and ships it off-box via `CrewTestResultEvent.execution_duration` → telemetry span attribute `exec_time` (`crew_events.py:108`, `telemetry.py:704,721`, wiring `event_listener.py:228-235`). The experimental path records `start_time`/`end_time` per LLM call and per agent trace (`evaluation_listener.py:187-195`, `:284-290`, `:320-326`) and computes inter-call intervals and timing-consistency signals for loop detection (`reasoning_metrics.py:70-86`, `:373-381`). Granularity is task/LLM-call level; there is no percentile/histogram latency reporting.

### 3. Is success rate tracked?

**Yes, in the experimental framework only.** `ExperimentResultsDisplay.summary` computes and renders "Success Rate" as passed/total percent (`result_display.py:14-27`); pass/fail comes from threshold rules comparing actual vs expected scores (`runner.py:130-165`). Results persist to JSON and support improved/regressed/unchanged/new/missing classification against a baseline (`result.py:99-143`), with pytest-style guards `assert_experiment_successfully` / `assert_experiment_no_regression` raising on failures or regressions (`testing.py:13-53`). In contrast, the stable `crewai test` path tracks only mean quality scores across iterations (`tasks_scores`, `crew_evaluator_handler.py:48`, averages at `:118-162`) with no pass/fail notion. Note the experimental framework is not exported through docs (`docs/edge/en/concepts/testing.mdx` mentions neither `run_experiment` nor baselines).

### 4. Are model tradeoffs analyzed?

**Only manually; no built-in comparative analysis.** Mechanisms that exist: (a) the model under test is a CLI parameter (`cli.py:600-616` → `evaluate_crew.py:20` → `crew.py:2227-2259`) and defaults to `gpt-4o-mini` (`docs/edge/en/concepts/testing.mdx:14`); (b) each test result records the model name on the event and telemetry span (`crew_evaluator_handler.py:211`, `telemetry.py:722`); (c) the judge LLM can differ from the crew LLM (`eval_llm` at `crew.py:2230`, `create_llm` at `:2238`). What does *not* exist: any code that runs two models over the same dataset and tabulates cost-vs-quality, persists per-model eval history, or normalizes scores by price. `ExperimentResults.compare_with_baseline` compares runs over time, not models (`result.py:80-83` picks the latest baseline regardless of model). Qualitative tradeoff discussion lives in docs (`docs/edge/en/learn/llm-selection-guide.mdx:90-142`, `:489`, `:588-590`), and systematic model evaluation is explicitly deferred to the hosted CrewAI AMP platform (`:604-616`). So: **can you compare two model choices on cost vs quality? Not out of the box** — you could run `crewai test` twice with different `--model` flags and eyeball two score tables plus execution-time rows, but token/cost columns are absent from the report, so the "cost" half of the comparison would have to be derived externally.

## Architectural Decisions

- **Quality judging via LLM-as-judge, decoupled from execution**: `crewai test` copies the crew (`crew.py:2251`), attaches an evaluator agent whose sole job is scoring outputs 1–10 (`crew_evaluator_handler.py:58-85`), and allows a separate judge model (`crew.py:2230`). This isolates eval cost/latency of the judge from the system-under-test but means reported quality inherits judge-model variance.
- **Event-bus-centric instrumentation**: all metrics flow through `crewai_event_bus` — test results (`CrewTestResultEvent`), per-call traces (`LLMCallCompletedEvent`), usage aggregation (`flow/runtime/__init__.py:903`). This makes cost/latency collection pluggable (telemetry spans, hosted tracing, eval listeners all subscribe independently) rather than hardcoded into the executor.
- **Two-tier evaluation strategy**: a stable minimal scorer (quality + execution time) shipped in core utilities, and a richer dataset/baseline framework quarantined under `experimental/evaluation/` (`base_evaluator.py:20-29` defines six metric categories; six default evaluators registered at `agent_evaluator.py:344-365`). Signals an intentional rollout boundary.
- **Provider-normalized usage accounting**: `UsageMetrics.from_provider_dict` unifies OpenAI/Gemini/Anthropic usage aliases and folds Anthropic cache tokens back into billed prompt tokens so totals match billing (`usage_metrics.py:111-189`) — a deliberate decision to treat token counts as a financial proxy even though dollars are never computed.
- **Telemetry-first economics**: rather than local cost dashboards, per-test quality/time/model attributes are exported as OTel spans (`telemetry.py:708-723`), pushing aggregation/comparison to external observability backends.

## Notable Patterns

- **Dead-parameter seam for future cost columns**: `print_crew_evaluation_result(token_usage=None)` (`crew_evaluator_handler.py:95-96`) is a vestigial extension point where token stats were meant to join the eval table; callers today pass nothing (`crew.py:2259`).
- **Snapshot-and-delta usage scoping**: `UsageMetrics.delta_since(baseline)` clamps at zero so reset accumulators can't produce negative usage (`usage_metrics.py:79-109`); kickoff results use this to report per-call usage from lifetime LLM counters (`base_llm.py:973-986`).
- **Trace-driven efficiency heuristics**: cheap statistical signals (n-gram Jaccard similarity > 0.7 for loops `reasoning_metrics.py:244-258`; length-ratio repetition `:351-371`) gate whether an expensive LLM judge call is worth making — a cost-conscious pattern inside the evaluator itself.
- **Baseline-as-file workflow**: first run auto-persists itself as the baseline when none exists (`result.py:70-78`), giving zero-config regression tracking.
- **Score-shape polymorphism**: `_assert_scores` handles scalar/dict expected-vs-actual combinations including "actual must meet average" fallbacks (`runner.py:130-165`).

## Tradeoffs

- **Rich runtime telemetry, thin eval reporting**: token data is comprehensive at runtime but deliberately excluded from the human-facing eval artifact; users get quality + wall-clock only. Wall-clock is a poor cost proxy since it conflates model speed with tool/network time.
- **Experimental framework power vs adoption risk**: baselines, success rates, and six metric categories exist, but the `experimental/` placement, sparse tests (four runner cases), and absence from user docs make reliance fragile.
- **Judge-in-loop cost amplification**: every evaluated task spawns extra judge LLM calls (`crew_evaluator_handler.py:194-199`); with N iterations × T tasks, eval spend scales linearly and is itself untracked in the reported output — the eval's own token burn is invisible.
- **Model label without controlled comparison**: recording `model` on events supports external correlation, but nothing prevents mixed-model crews from making the label ambiguous (it records the judge/eval LLM name via `getattr(self.llm, "model", str(self.llm))`, `crew_evaluator_handler.py:211`, while the crew may run several models).
- **Qualitative docs vs quantitative tools**: extensive prose on choosing cheaper/faster models (`docs/edge/en/learn/llm-selection-guide.mdx`) with no measurement loop behind it in the OSS repo.

## Failure Modes / Edge Cases

- **Eval crash yields fake zero score**: an exception mid-test-case returns `ExperimentResult(score=0.0, passed=False)` (`runner.py:104-112`), indistinguishable in aggregate from a genuine 0-quality failure.
- **Silent degradation when traces are empty**: evaluators return `score=None` with "Insufficient LLM calls..." feedback when `<2` calls exist (`reasoning_metrics.py:61-65`); `AgentEvaluator.evaluate` swallows evaluator exceptions into printed warnings (`agent_evaluator.py:283-292`), so missing cost/efficiency signal surfaces only as N/A.
- **Timing fragility acknowledged in-code**: inter-call interval math marks `has_reliable_timing = False` on any parse failure and omits the metric (`reasoning_metrics.py:70-86`); loop-likelihood silently skips time-consistency when numpy ops fail (`:380-381`).
- **Baseline file corruption tolerated but lossy**: malformed baseline JSON logs a warning and treats current run as new baseline, discarding prior history (`result.py:56-78`).
- **Division-by-zero guarded in display only**: success-rate render checks `total > 0` (`result_display.py:26`), but `crew_average` in the stable path divides `task_averages` unguarded (`crew_evaluator_handler.py:122`) and would raise on a crew with zero scored tasks.
- **Unused `token_usage` param invites silent drift**: callers passing token data today would see it ignored without error (`crew_evaluator_handler.py:95-96`).

## Future Considerations

- Wire the orphaned `token_usage` parameter through: `Crew.test` already has `calculate_usage_metrics()` available (`crew.py:2201-2225`) and could pass per-run deltas into `print_crew_evaluation_result` to add a tokens row beside "Execution Time (s)".
- Add a monetary-cost layer: the LiteLLM pricing feed already cached by the CLI (`model_catalog.py`) contains per-token prices keyed by model id; joining it with `UsageMetrics` would enable true cost-per-run and cost-per-point-of-quality reporting without new data sources.
- Persist model identity alongside baselines so `compare_with_baseline` can refuse (or annotate) cross-model comparisons instead of comparing any two latest runs (`result.py:80-83`).
- Graduate the experiment framework: promote `run_experiment`/`assert_experiment_*` (`testing.py`) into documented API once test coverage extends beyond the four existing runner tests.
- Report the eval harness's own consumption (judge calls) separately from the system-under-test to expose total evaluation cost per iteration.
- Add distributional latency reporting (p50/p95 per task or per LLM call) building on timestamps already captured in traces (`evaluation_listener.py:320-326`).

## Questions / Gaps

- **No monetary cost computation found.** Searched `usd|USD|price|pricing|dollar|cost_per|\$[0-9]` across `lib/**/*.py`; only hits were the LiteLLM catalog feed URL/name and unrelated tool fixtures. Confidence high that OSS-side cost math is absent; hosted AMP platform behavior is outside this source and unverifiable here.
- **Is `print_crew_evaluation_result(token_usage=...)` called with data anywhere?** Searched all callers (`grep print_crew_evaluation_result`): only `crew.py:2259` (no args) and a test (`test_crew_evaluator_handler.py:104`, no args). Concluded the parameter is currently dead.
- **No benchmark suite or eval CI found.** `.github/workflows/` contains lint/unit-test workflows; no dataset-driven model-quality jobs. The only eval-like scripts live in third-party integrations (e.g., Patronus eval tools under `lib/crewai-tools/src/crewai_tools/tools/patronus_eval_tool/`), which delegate to an external service.
- **Success-rate semantics for multi-agent experiments undocumented**: `_extract_scores` averages metric scores across agents (`runner.py:114-128`); how teams should set per-metric thresholds is unspecified beyond the docstring rules in `_assert_scores`.
- **Latency attribution between judge and crew not separated**: `execution_duration` measures task wall time including any nested judge activity; no evidence of a clean split was found.

---

Generated by dimension 18.04 (Cost, Latency, and Quality Evaluation) against `crewai`.
