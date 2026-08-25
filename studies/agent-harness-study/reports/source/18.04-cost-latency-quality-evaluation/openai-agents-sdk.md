# Source Analysis: openai-agents-sdk

## 18.04 Cost, Latency, and Quality Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (Pydantic, asyncio, OpenAI API SDKs; MkDocs docs; pytest test suite) |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents SDK does not ship an evaluation harness that measures cost, latency, success rate, or quality. What it ships instead is a mature **token-usage instrumentation layer** that is explicitly designed so downstream consumers can compute costs themselves: a `Usage` dataclass aggregating requests/input/output tokens with cached-token and reasoning-token detail (`src/agents/usage.py:196-229`), per-request breakdowns documented as being "useful for detailed cost calculation" (`src/agents/usage.py:218-229`, `docs/usage.md:44-53`), and a dedicated regression test simulating a tiered-pricing cost-calculation scenario (`tests/test_usage.py:427-488`). Latency appears only as tracing span timestamps (`src/agents/tracing/spans.py:343-357`) and behavioral timing assertions in tests, not as an eval metric. The only true "eval" artifacts are two tutorial artifact validators (`examples/sandbox/tutorials/dataroom_metric_extract/evals.py:240-302`, `examples/sandbox/tutorials/repo_code_review/evals.py:21-50`) that perform pass/fail assertions on outputs without recording any cost, runtime, or aggregate success statistics. Model-choice economics are addressed qualitatively in documentation: the default model is chosen "for cost-sensitive, high-volume agent workflows" (`docs/models/index.md:26`), and guardrails documentation analyzes when a cheap guardrail model saves money versus a frontier model (`docs/guardrails.md:3,36-38`). No tooling compares two models' cost-vs-quality systematically.

## Rating

**Score: 4 / 10 — Present but inconsistent, weakly operationalized.**

Rationale against the rubric:

- Token-cost *instrumentation* is excellent (tested, documented, explicit interfaces), which keeps this above the 1–3 band: `Usage` with `request_usage_entries` is purpose-built for accurate cost calculation (`src/agents/usage.py:219-229`) and is exercised by a named cost-scenario test (`tests/test_usage.py:427`).
- But the dimension asks whether **evals** measure these things. They do not: no eval records token cost, no eval records wall-clock runtime, no eval computes success rates over multiple runs, and there is no model-comparison reporting of any kind. The gap between rich primitives and zero evaluation composition places this at the bottom of the 4–6 band.
- The question "can you compare two model choices on cost vs quality?" is answerable only by manual assembly: run twice with different `RunConfig.model` values (`docs/models/index.md:39-53`), read `context_wrapper.usage` (`src/agents/run_context.py:83`), and reuse a tutorial eval as a quality gate — all glue code the user must write.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token cost tracking (primitive) | `Usage` dataclass: `requests`, `input_tokens`, `output_tokens`, `total_tokens`, plus cached/cache-write/reasoning details | `src/agents/usage.py:195-229` |
| Per-request cost basis | `request_usage_entries: list[RequestUsage]` docstring states it preserves per-request breakdown "for accurate per-request cost calculation"; example shows 100K/150K/80K split | `src/agents/usage.py:218-229` |
| Usage aggregation semantics | `Usage.add()` aggregates totals and synthesizes/merges request entries | `src/agents/usage.py:257-312` |
| Cost-calculation test | `test_anthropic_cost_calculation_scenario`: simulates 3 calls under a 200K tiered pricing threshold and asserts per-entry preservation | `tests/test_usage.py:427-488` |
| Run-level access point | `RunContextWrapper.usage` field exposed to applications and hooks | `src/agents/run_context.py:83`; hook example at `docs/usage.md:112-121` |
| Loop-level aggregation | Runner adds each response's usage into context (`context_wrapper.usage.add(...)`); turn/task usage deltas feed custom spans | `src/agents/run_internal/run_loop.py:2306,2636`; `src/agents/run_internal/agent_runner_helpers.py:76-118,155` |
| Span serialization for observability | `model_usage_to_span_usage`, `total_usage_to_span_metadata`, `turn_usage_to_span_data`, `task_usage_to_span_data` | `src/agents/usage.py:434-485` |
| Tracing spans carry model + usage | `GenerationSpanData` exports `model`, `model_config`, `usage`; `ResponseSpanData` carries `usage` | `src/agents/tracing/span_data.py:169-209,212-241` |
| Latency timestamps (tracing only) | Spans record `started_at`/`ended_at` ISO timestamps via `util.time_iso()` | `src/agents/tracing/spans.py:343-357`; clock at `src/agents/tracing/provider.py:360` |
| Realtime elapsed-time tracking | Playback tracker computes `elapsed_ms` from `time.monotonic()` for audio interruption decisions | `src/agents/realtime/openai_realtime.py:1007-1017` |
| Behavioral latency assertions in tests | Cancellation propagation asserted under 0.3 s; trace shutdown under 0.5 s | `tests/test_cancel_streaming.py:173-178`; `tests/test_trace_processor.py:216-218` |
| Timeout knob (not a metric) | Per-model-call attempt timeout setting documented as "Maximum duration in seconds for each model-call attempt" | `src/agents/model_settings.py:216` |
| Evals exist (quality-only) | Tutorial artifact validator asserts exact metric rows/values; raises `AssertionError`, prints pass count only | `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:240-302,305-315` |
| Second eval (quality-only) | Repo-review eval checks finding paths/comment content and patch text; no metrics recorded | `examples/sandbox/tutorials/repo_code_review/evals.py:21-50,63-75` |
| LLM-as-judge pattern | Evaluator agent loops until output passes; binary pass/improve loop, no scoring aggregation | `examples/agent_patterns/llm_as_a_judge.py:31-67`; mirrored in `tests/test_example_workflows.py:343-396` |
| Default model chosen for cost | Default `gpt-5.6-luna` with `reasoning.effort="none"` and `verbosity="low"` "for cost-sensitive, high-volume agent workflows" | `docs/models/index.md:26` |
| Model selection interfaces | `OPENAI_DEFAULT_MODEL` env var and `RunConfig(model=...)` override paths | `docs/models/index.md:32-53` |
| Qualitative cost/latency tradeoff analysis | Guardrails: run a "fast/cheap model" guardrail to save "time and money"; parallel mode = best latency but wasted tokens if tripped; blocking = prevents token consumption | `docs/guardrails.md:3,36-38` |
| Orchestration tradeoff guidance | "orchestrating via code makes tasks more deterministic and predictable, in terms of speed, cost and performance" | `docs/multi_agent.md:45` |
| Deliberate-evaluation guidance | Agents-as-tools create nested orchestration: "Evaluate the additional latency, cost, and tool exposure deliberately" | `docs/models/index.md:296` |
| Session-level usage analytics | `AdvancedSQLiteSession.store_run_usage` / `get_session_usage` / `get_turn_usage` persist per-turn token analytics | `docs/sessions/advanced_sqlite_session.md:90-131`; schema at lines 289-302 |
| Provider usage opt-in flags | `ModelSettings.include_usage` and `ModelSettings.preserve_raw_usage` control whether usage chunks/payloads are captured | `src/agents/model_settings.py:156,205` |
| Usage-tracking example app | `examples/basic/usage_tracking.py` prints totals and per-request entries after a run | `examples/basic/usage_tracking.py:25-48` |
| Docs position usage as cost tool | Page intro: track usage "to monitor costs, enforce limits, or record analytics"; nav entry titled "Usage and pricing … cost estimation" | `docs/usage.md:3,46`; `docs/llms.txt:30` |

## Answers to Dimension Questions

### 1. Is token cost measured in evals?

**No — not inside any eval; yes as first-class runtime instrumentation.** No eval script computes or records dollar cost or token totals. However, the SDK makes cost computation straightforward by design: `Usage.request_usage_entries` exists specifically because aggregated input tokens cannot express tiered pricing, with an in-repo test validating exactly that scenario (`tests/test_usage.py:427-488`, comment at line 430: "None exceed 200K, so they should all use the lower pricing tier"). The SDK never multiplies tokens by prices itself; `raw_usage` preservation (`src/agents/model_settings.py:205`, `docs/usage.md:55-76`) lets users recover provider-specific fields for their own calculators. Cost measurement in an eval loop is therefore possible but left entirely to the consumer.

### 2. Is latency measured?

**Not as an eval metric.** Wall-clock duration is recorded structurally on every trace span (`started_at`/`ended_at`, `src/agents/tracing/spans.py:343-357`), and realtime audio playback tracks `elapsed_ms` with a monotonic clock (`src/agents/realtime/openai_realtime.py:1007-1017`). Tests do assert latency bounds behaviorally (cancellation < 0.3 s at `tests/test_cancel_streaming.py:178`; processor shutdown < 0.5 s at `tests/test_trace_processor.py:218`). A maintainer-review reference even mandates measuring "observable elapsed time … rather than inferred only from mocks" when reviewing latency claims (`.agents/skills/maintainer-review/references/evaluation-framework.md:62`). But neither tutorial eval captures runtime, and there is no latency-reporting utility for benchmark runs.

### 3. Is success rate tracked?

**No.** Both shipped evals are single-artifact pass/fail checks: `validate_outputs` raises `AssertionError` on any row mismatch and returns a bare row count on success (`examples/sandbox/tutorials/dataroom_metric_extract/evals.py:268-302,314-315`); the repo-review eval raises `ValueError` on any deviation (`examples/sandbox/tutorials/repo_code_review/evals.py:21-50`). Neither aggregates pass/fail across runs, repeats, or scenarios; no pass-rate, accuracy, or score-aggregation code was found anywhere in `tests/`, `examples/`, or `src/`. Searches for `success_rate`, `pass_rate`, and `accuracy` across the test suite returned only unrelated schema-field matches. The llm-as-judge pattern (`examples/agent_patterns/llm_as_a_judge.py:66-67`) loops to convergence but emits no statistics.

### 4. Are model tradeoffs analyzed?

**Qualitatively in docs; never quantitatively in code or reports.** The strongest analyses are prose: the default model is explicitly a cost decision — `gpt-5.6-luna` with reasoning off and low verbosity "for cost-sensitive, high-volume agent workflows," with frontier capability available via explicit `model="gpt-5.6-sol"` (`docs/models/index.md:26`); guardrail docs compare cheap-guardrail vs expensive-main models and parallel-vs-blocking execution in terms of latency and token waste (`docs/guardrails.md:3,36-38`); agents-as-tools guidance says to "evaluate the additional latency, cost, and tool exposure deliberately" (`docs/models/index.md:296`). Structurally, `GenerationSpanData` pairs each generation's `model` name with its `usage` (`src/agents/tracing/span_data.py:169-209`), which is sufficient raw material for cross-model comparison — but no comparison runner, report format, or benchmark table exists in the repo.

## Architectural Decisions

1. **Instrument, don't evaluate.** The SDK owns usage capture at the lowest level (per-request entries synthesized inside `Usage.add()`, `src/agents/usage.py:295-312`) and exposes it through `RunContextWrapper.usage` (`src/agents/run_context.py:83`) rather than owning any evaluation or billing layer.
2. **Per-request fidelity over convenience totals.** Aggregated tokens lose tiered-pricing information, so `request_usage_entries` preserves the exact per-call sequence (`src/agents/usage.py:218-229`), validated by a scenario-shaped test (`tests/test_usage.py:427-488`).
3. **Lossless normalization boundaries.** Raw provider payloads can be preserved via `preserve_raw_usage` snapshots (`src/agents/usage.py:20-68`, `src/agents/model_settings.py:205`) so cost tools can distinguish omitted fields from provider-reported zeros — an explicit acknowledgment that normalized totals alone are insufficient for economic analysis.
4. **Cost-aware defaults instead of cost knobs.** Rather than exposing budget limits, the SDK bakes frugality into defaults: the default model and its `reasoning.effort="none"`/`verbosity="low"` settings (`docs/models/index.md:26`).
5. **Evals as artifact validators, not harnesses.** Tutorial evals are plain scripts over generated artifacts wired into the examples runner (`examples/run_examples.py:89-92`), intentionally outside the pytest suite (`tests/test_run_examples_script.py:25-28`).

## Notable Patterns

- **Usage deltas around nested scopes:** the run loop snapshots usage before a task/turn and emits the delta into custom span data afterwards (`src/agents/run_internal/run_loop.py:911,1173,1736-1782`; delta math in `src/agents/run_internal/agent_runner_helpers.py:99-118`), giving hierarchical cost attribution per turn/task in traces.
- **Retry-aware accounting:** failed retry attempts contribute request counts without fabricated usage via `_mark_request_completed_without_usage` (`src/agents/usage.py:315-343`) and `apply_retry_attempt_usage` (`src/agents/run_internal/model_retry.py:338`) — cost accounting stays honest under retries.
- **Session-persisted analytics:** optional `store_run_usage()` turns each run into queryable per-turn token rows (`src/agents/extensions/memory/advanced_sqlite_session.py`, documented at `docs/sessions/advanced_sqlite_session.md:90-131`).
- **Cheap-first guardrail pattern:** documentation encodes a reusable economic pattern — screen input with a small model before spending frontier tokens (`docs/guardrails.md:3`).
- **Lifecycle hooks as analytics seams:** `RunHooks.on_agent_end` receives context with cumulative usage for user-side logging (`docs/usage.md:112-121`; example implementation at `examples/basic/lifecycle_example.py:45-106`).

## Tradeoffs

- **Composability vs completeness:** users get exact ingredients for cost/quality studies (per-request tokens, model-per-span, timestamps) but must build every evaluation themselves; nothing in-repo demonstrates a completed cost×quality comparison.
- **Normalized totals vs provider truth:** normalization gives cross-provider consistency, but adapter quirks are real enough that docs warn some LiteLLM backends emit no usage unless `include_usage=True` is set (`docs/usage.md:37-42`; flag at `src/agents/model_settings.py:156`) — silent zero-cost readings are possible if users skip validation.
- **Latency timestamps vs latency metrics:** ISO string timestamps on spans (`src/agents/tracing/spans.py:402-403`) require subtraction by the exporter to become durations; no helper computes them.
- **Pass/fail simplicity vs statistical power:** boolean tutorial evals are deterministic and CI-friendly but cannot express partial credit, variance across runs, or model-dependent difficulty.

## Failure Modes / Edge Cases

- **Missing usage payloads:** providers may omit usage entirely; the SDK counts the request but records zero tokens (`_mark_requests_completed_without_usage`, `src/agents/usage.py:318-324`) — an eval relying on token totals would silently undercount cost rather than fail.
- **Streaming lag:** streamed-run usage totals "can lag until the stream's final chunks have been processed" (`docs/results.md:220`), so reading usage mid-stream skews measurements.
- **Compaction attribution:** auto-compaction usage folds into the enclosing run's totals while manual compaction does not update earlier results (`docs/usage.md:33`; `docs/sessions/index.md:279`) — naive per-turn cost accounting can misattribute compaction spend.
- **Resume isolation surprises:** resumed runs start from checkpointed totals and do not mutate the original result's usage (`docs/usage.md:94-110`), which is correct but easy to misread in longitudinal cost studies.
- **Eval brittleness to nondeterminism:** the tutorial evals assert exact values and exact row sets (`examples/sandbox/tutorials/dataroom_metric_extract/evals.py:268-300`), so any model drift fails the eval with no graded signal of how far off it is.

## Future Considerations

- Add an eval-recording layer that persists `(model, usage, wall_clock_duration, pass/fail)` tuples per run — every required primitive already exists (`Usage`, `GenerationSpanData.model`, span timestamps).
- Provide a duration helper over `started_at`/`ended_at` or numeric span timings to make latency a first-class exported metric.
- Extend tutorial evals with soft thresholds or tolerance bands so quality degradation is measurable rather than binary.
- Document a worked two-model comparison recipe (same workload, two `RunConfig.model` values, usage diff + eval verdict) to make the existing primitives actionable.
- Surface a warning path when a provider returns no usage payload during streaming so cost dashboards don't silently show zeros.

## Questions / Gaps

- **No evidence found of any benchmark runner or model-comparison report** inside the repo. Searched for `benchmark`, `success_rate`, `pass_rate`, `accuracy`, `pricing` across Python sources; the only hits were the Runloop third-party sandbox benchmarks client passthrough (`src/agents/extensions/sandbox/runloop/sandbox.py:575-608`), unrelated schema fields, and sample-output prose — none constitute SDK-owned evaluation.
- **No evidence found of latency capture in either shipped eval**, nor of any repeated-run averaging utility.
- **No evidence found of budget/limit enforcement**: `docs/usage.md:3` mentions "enforce limits" as a consumer use case, but no spending-cap mechanism exists in `src/`.
- Whether the hosted "benchmarks" surface referenced through the Runloop extension implies a future first-party eval story could not be determined from this source alone.

---

Generated by `18.04-cost-latency-and-quality-evaluation` against `openai-agents-sdk`.
