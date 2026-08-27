# Source Analysis: openai-agents-sdk

## Quality-Cost Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python / `openai` SDK (Responses + ChatCompletions), `pydantic`, `httpx2`, `websockets` |
| Analyzed | 2026-08-26 |

## Summary

`openai-agents-sdk` provides **manual multi-model tiering** — per-agent string model names, per-run `RunConfig.model` overrides, and provider-level prefix routing via `MultiProvider` — but implements **no quality-cost router**. There is no component that selects cheap vs expensive models by query complexity, token budget, latency, or risk; model choice is static configuration plus LLM-driven semantic handoffs (`triage_agent` -> specialist). Reliability is handled by per-model retry/backoff (`ModelRetrySettings` + `retry_policies`) that retries the *same* model rather than falling back to a cheaper/faster tier. Model selection is observable post-hoc via `generation` span fields (`model`, `model_config`, `usage`) and `handoff` spans, but there is no explicit routing-decision trace or cost-aware policy object.

## Rating

**Rating: 2 / 10 — Absent / Ad-hoc**

Rationale: Multiple model tiers are technically available (11+ GPT-5.x defaults, `extra_args.service_tier`, manual `Agent.model` assignment), yet routing is entirely developer-owned and LLM-semantic, not cost/latency/quality-driven. No router abstraction, no cost-aware criteria, no model-fallback chain, and no routing-policy config exist. Retry exists but is single-model. Traceability is limited to raw `generation` span model names. This matches the 1-3 rubric: absent, implicit, ad-hoc, unsafe for autonomous cost optimization.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Multi-model config — per-agent model | `Agent.model: str \| Model \| None` field with doc `if not set, will use get_default_model() (currently "gpt-5.6-luna")` | `src/agents/agent.py:337-342` |
| Multi-model config — default model resolution | `get_default_model()` reads `OPENAI_DEFAULT_MODEL` env else `gpt-5.6-luna`; `get_default_model_settings()` deep-copies GPT-5 reasoning defaults per model pattern | `src/agents/models/default_models.py:99-120` |
| Multi-model config — GPT-5 variant defaults (cost vs frontier) | 4 `ModelSettings` presets (`_GPT_5_LOW_DEFAULT_MODEL_SETTINGS`, `_GPT_5_NONE_...`, etc.) + 9 pattern-to-effort mappings (`gpt-5.6-luna`→`none`, `gpt-5.6-sol`→`none` but docs describe frontier) | `src/agents/models/default_models.py:16-69` |
| Multi-model config — service tier via extra_args | `ModelSettings.extra_args: dict[str,Any]` allows `service_tier: flex/fast/priority`; merged via `resolve()` and passed through in `extra_args` to provider `create_kwargs` | `src/agents/model_settings.py:183-186` ; `src/agents/models/openai_responses.py:961-1012` ; `docs/models/index.md:481-482` |
| Multi-model config — per-run override | `RunConfig.model: str \| Model \| None` overrides every agent's model; `model_provider: ModelProvider = field(default_factory=MultiProvider)` | `src/agents/run_config.py:353-359` |
| Multi-model config — mixing models workflow | Docs example assigns `spanish_agent model="gpt-5-mini"`, `english_agent model=OpenAIChatCompletionsModel(model="gpt-5-nano")`, `triage_agent model="gpt-5.6-sol"` to show intentional tiering | `docs/models/index.md:342-388` |
| Router/routing criteria — handoff semantic routing | `Handoff` defined as agent-to-agent delegation with `tool_name`, `tool_description`, LLM chooses target; `handoff()` helper weak-refs target agent; no cost metric | `src/agents/handoffs/__init__.py:125-135`, `260-374` |
| Router/routing criteria — routing example is semantic, not cost | `triage_agent` with `instructions="Handoff to appropriate agent based on language"` + `[french_agent, spanish_agent, english_agent]` with no cost logic | `examples/agent_patterns/routing.py:30-34` |
| Router/routing criteria — ModelSettings has no quality/cost fields | `ModelSettings` fields: `temperature`, `top_p`, `reasoning`, `verbosity`, `max_tokens`, `parallel_tool_calls`, `truncation`, `retry`, `extra_args` — no `cost_threshold`, `latency_budget`, `quality_score` | `src/agents/model_settings.py:88-222` |
| RunConfig criteria — likewise no cost router | `RunConfig` fields: `model`, `model_provider`, `model_settings`, `tracing_*`, `handoff_*`, `tool_execution`, `sandbox` — no routing policy | `src/agents/run_config.py:349-530` |
| Provider routing (prefix-based, not quality-based) | `MultiProvider.get_model()` splits on `/` prefix (`litellm/`, `any-llm/`, `openai/`) and delegates to mapped `ModelProvider`; modes `openai_prefix_mode`/`unknown_prefix_mode` control literal vs alias | `src/agents/models/multi_provider.py:62-252` |
| Provider fallback (provider instance caching, not model fallback) | `MultiProvider._fallback_providers: dict[str,ModelProvider]` caches lazily created `LitellmProvider`/`AnyLLMProvider`; no chain like `gpt-5.6-sol -> gpt-5.6-mini` | `src/agents/models/multi_provider.py:153-197` |
| Fallback chain — retry is same-model, not tier fallback | `ModelRetrySettings {max_retries, backoff, policy: RetryPolicy}` + `retry_policies.{provider_suggested, network_error, retry_after, http_status, any, all}`; `get_response_with_retry`/`stream_response_with_retry` retry same `get_response` callable with backoff | `src/agents/retry.py:210-464` ; `src/agents/run_internal/model_retry.py:574-682`, `684-891` |
| Fallback chain — retry evaluation with vetoes | `_evaluate_retry()` blocks retry for aborts, `emitted_retry_unsafe_event`, `replay_unsafe_request`, `stateful_request` with `previous_response_id`/`conversation_id`; requires `approve_unsafe_replay` or `provider_advice.replay_safety=="safe"` | `src/agents/run_internal/model_retry.py:389-494` |
| Fallback chain — conversation_locked compatibility retries | `COMPATIBILITY_CONVERSATION_LOCKED_RETRIES=3` hard-coded retries for `code=="conversation_locked"` with exponential 1s*2^(n-1) | `src/agents/run_internal/model_retry.py:50`, `618-640`, `831-851` |
| Routing decision traces — generation span captures model | `GenerationSpanData {input, output, model, model_config, usage}` exported as `type: generation`; `_populate_stream_generation_span()` and `get_response` populate via `generation_span()` context | `src/agents/tracing/span_data.py:169-209` ; `src/agents/models/openai_chatcompletions.py:238-512` ; `src/agents/models/openai_responses.py:568-651` |
| Routing decision traces — ModelTracing enum controls data inclusion | `get_model_tracing_impl(tracing_disabled, trace_include_sensitive_data) -> ModelTracing.{DISABLED,ENABLED,ENABLED_WITHOUT_DATA}` ; `ModelTracing` enum at `models/interface.py:20-34` | `src/agents/tracing/model_tracing.py:6-14` ; `src/agents/models/interface.py:20-34` |
| Routing decision traces — agent/handoff spans but no cost router span | `AgentSpanData {name, handoffs, tools, output_type}` and `HandoffSpanData {from_agent, to_agent}` plus `TurnSpanData` via `agent_span`/`handoff_span`/`turn_span`; no `router` or `cost_routing` span type exists | `src/agents/tracing/span_data.py:28-62`, `98-265` ; `src/agents/tracing/create.py:30-180` (checked) |
| Routing policy config — ModelSettings.resolve merging | `ModelSettings.resolve(override)` non-None overlay with special merge for `extra_args` (dict merge) and `retry.backoff` via `replace()` | `src/agents/model_settings.py:254-290` |
| Routing policy config — no declarative router config file | Grep for `router`, `quality.*cost`, `tier` across `src/agents` returns only tool-identity routing (`_tool_identity.py:287-496`), `MultiProvider` prefix routing comments, and tracing metadata keys; no YAML/JSON routing policy | `src/agents/_tool_identity.py:46-496` (negative evidence) |
| Docs — cost-sensitive default highlighted but manual | Docs: `gpt-5.6-luna` default `reasoning.effort="none" verbosity="low"` described as "cost-sensitive, high-volume"; frontier `gpt-5.6-sol` requires explicit `model="gpt-5.6-sol"` | `docs/models/index.md:24-27` |
| Docs — third-party adapter routing disclaimer | Any-LLM/LiteLLM described as "best-effort, beta" adapters where feature semantics vary by upstream provider; not a cost router | `docs/models/index.md:680-708` |

## Answers to Dimension Questions

**1. Are multiple model tiers available?**
Yes — technically. Three mechanisms enable tiering: (a) static per-`Agent.model` assignment to distinct string names (`gpt-5.6-luna` for cheap high-volume, `gpt-5.6-sol` for frontier — `docs/models/index.md:24-27` and `src/agents/models/default_models.py:16-69`), (b) per-run override `RunConfig.model` (`src/agents/run_config.py:353-356`), and (c) `ModelSettings.extra_args={"service_tier": "flex"|"fast"|"priority"}` for OpenAI tier control (`src/agents/model_settings.py:183`). `MultiProvider` further allows mixing providers in one run (`openai/`, `litellm/`, `any-llm/` prefixes — `src/agents/models/multi_provider.py:62-74`). However, there is no built-in tier registry (e.g., `cheap/standard/premium`) nor validated `model_tier` enum; tiering is stringly-typed and developer-maintained. In practice the mixing-models example (`docs/models/index.md:358-388`) is the canonical pattern.

**2. What criteria drive model selection?**
No quality-cost criteria. Selection is (1) explicit config: `Agent.model`, `RunConfig.model`, `RunConfig.model_provider.get_model(name)`, (2) LLM semantic choice among `Handoff` tools (`handoff_description` informs the LLM — `src/agents/agent.py:188-192`, `src/agents/handoffs/__init__.py:135-136`; example `routing.py:31`), and (3) `ModelSettings.resolve()` precedence (run overrides agent — `src/agents/model_settings.py:254`). There are no latency budgets, token-budget thresholds, classification scores, risk labels, or cost heuristics anywhere in `ModelSettings` (`src/agents/model_settings.py:88-222`) or `RunConfig` (`src/agents/run_config.py:349-530`). Provider prefix resolution (`src/agents/models/multi_provider.py:199-252`) is the only rule-based router and it routes by namespace, not by cost/quality.

**3. Are fallback chains defined?**
No cross-tier fallback chain. The only fallback is **single-model retry**: `ModelRetrySettings.max_retries + backoff + RetryPolicy` retries the same `Model.get_response()` call after normalization of status/retry-after (`src/agents/retry.py:210-464`, `src/agents/run_internal/model_retry.py:574-891`). Policies (`provider_suggested`, `network_error`, `retry_after`, `http_status`, `any`/`all` combinators — `src/agents/retry.py:304-462`) decide *whether* to retry, never *which cheaper model* to try next. Safety vetoes (`is_abort`, `emitted_retry_unsafe_event`, `stateful_request`, `replay_safety` — `src/agents/run_internal/model_retry.py:389-494`) actually narrow retries. `MultiProvider._fallback_providers` (`src/agents/models/multi_provider.py:153-197`) caches provider instances, not tier fallbacks. `Agent.clone(model=...)` (`src/agents/agent.py:548-581`) allows manual tier swapping but requires application glue; there is no declarative `fallback=[gpt-5.6-sol, gpt-5.6-mini]` array.

**4. Are routing decisions observable?**
Partially — via low-level traces, not routing decisions. `GenerationSpanData` records `model` + `model_config` + `usage` per turn (`src/agents/tracing/span_data.py:169-209`) and is emitted through `generation_span()` in `openai_responses.py:568`/`651` and `openai_chatcompletions.py:238/450`. `HandoffSpanData` (`src/agents/tracing/span_data.py:244-265`) + `AgentSpanData`/`TurnSpanData` allow correlation of which agent (hence which model) ran, and `Usage` aggregations track cost indirectly (`src/agents/usage.py`). However, there is no `RouterDecision` span with `selected_tier`, `rejected_tier`, `reason=cost|latency`, `latency_ms`, or `estimated_cost` — grep for `router`/`routing.*trace` finds only `_SPAN_METADATA_ROUTING_KEYS = ("agent_harness_id",)` (`src/agents/tracing/spans.py:16`) and tool-identity helpers (`src/agents/_tool_identity.py:287`). Since no router exists, nothing traces a cost-driven choice.

## Architectural Decisions

- **Manual tiering over autonomous router** (`src/agents/agent.py:337`, `src/agents/run_config.py:353`): The SDK exposes `Agent.model` and `RunConfig.model` as the sole tiering levers, delegating complexity/cost tradeoff to the developer or to LLM handoff reasoning. Decision avoids policy complexity but forces every cost optimization to be hand-wired.
- **Provider-prefix routing instead of quality routing** (`src/agents/models/multi_provider.py:62-252`): `MultiProvider` decides provider by string prefix (`openai/`, `litellm/`, `any-llm/`). This solves heterogeneous auth/endpoint routing, not cost-quality gradation. `openai_prefix_mode`/`unknown_prefix_mode` explicitly manage namespace vs alias ambiguity, showing design focus on compatibility, not tiering.
- **Retry-not-fallback resilience** (`src/agents/retry.py:210`, `src/agents/run_internal/model_retry.py:502-555`): Resilience is framed as transient-error retry with replay-safety gates (`provider_managed_retries_disabled`, `websocket_pre_event_retries_disabled`, `replay_safety` tristate). The 3 separate vetoes documented inline (`run_internal/model_retry.py:442-480`) reveal deliberate conservatism: never escalate to a cheaper model without explicit `approve_unsafe_replay` or `provider_suggested` safe signal.
- **Tracing at generation granularity** (`src/agents/tracing/span_data.py:169`, `src/agents/tracing/model_tracing.py:6`): Observability is per-LLM-call (`GenerationSpanData`) and per-handoff (`HandoffSpanData`), not per-router-decision. `ModelTracing` is tri-state (`DISABLED/ENABLED/ENABLED_WITHOUT_DATA`) driven by `trace_include_sensitive_data`, reflecting a privacy-vs-debuggability tradeoff, not cost telemetry.
- **Static GPT-5 defaults encode implicit cost hint** (`src/agents/models/default_models.py:16-69`, `docs/models/index.md:24-27`): Cheap path (`gpt-5.6-luna` + `effort=none, verbosity=low`) is the default; frontier path (`gpt-5.6-sol`) must be opted in. This bakes a single cost default into `get_default_model()` rather than offering a configurable tier policy.

## Notable Patterns

- **LLM-as-router (handoff pattern)**: `Agent(handoffs=[french_agent, spanish_agent])` where the triage LLM decides delegation via tool-calling (`examples/agent_patterns/routing.py:30-34`). This is the SDK's idiomatic "routing", but criterion is language/specialty semantics, not cost.
- **Prefix-based multi-provider multiplexing**: `MultiProvider` acts like a service-discovery router (`src/agents/models/multi_provider.py:199-252`) with `MultiProviderMap` (`:18-52`) for explicit overrides. Mirrors HTTP path-prefix routing; useful for cost experiments (e.g., route `any-llm/openrouter/openai/gpt-4o-mini` cheap vs `openai/gpt-5.6-sol` expensive) but requires manual model-name authoring.
- **Overlay `resolve()` merging for `ModelSettings`** (`src/agents/model_settings.py:254-290`): `resolve()` non-None overlays `extra_args` dict-merged and `retry.backoff` field-merged via `dataclasses.replace`, enabling per-agent override of shared `RunConfig`. Pattern is extensible but currently carries no routing-relevant fields.
- **Policy combinators for retries** (`src/agents/retry.py:376-462`): `retry_policies.any()` / `.all()` composing `provider_suggested`/`network_error`/`retry_after`/`http_status` yields declarative retry predicates — the closest thing to a policy engine, yet scoped to transient errors, not tier selection.
- **Generation-span model fixation per turn**: Each turn binds one `Agent` → one `Model` (`src/agents/run_internal/run_loop.py:2611` `get_model_tracing_impl` + `Runner` wiring), simplifying tracing but precluding mid-turn tier switching.

## Tradeoffs

- **Simplicity vs autonomy**: No router keeps the SDK surface small and debuggable (static `model` strings trivial to grep/audit) but pushes every cost optimization (cheap pre-classifier → expensive reasoner) to user code, increasing boilerplate and inconsistency across adopters.
- **Provider routing flexibility vs cost-routing absence**: `MultiProvider`'s two historical defaults (`openai/...` alias stripping, `Unknown prefix → UserError` — `src/agents/models/multi_provider.py:189-225`) and explicit opt-in `model_id` pass-through show careful compatibility thinking, yet the same rigor was not applied to quality-cost routing; teams re-implement tiering ad hoc.
- **Retry conservatism vs availability**: Strict replay-safety (`is_abort`/`response_started`/`stateful_request` vetoes — `src/agents/run_internal/model_retry.py:418-480`) prevents duplicate side-effects (critical for `function_call_output` or `previous_response_id` chains) but means a transient overload on the frontier model does not automatically degrade to a cheaper model, leaving throughput on the table.
- **Generation-level observability vs router observability**: `GenerationSpanData.model` is sufficient for billing attribution post-hoc, but without a router span it is impossible to answer "why was cheap model not chosen?" or track routing latency/cost savings — needed for SLOs.
- **Static defaults vs dynamic selection**: Encoding `gpt-5.6-luna` cheap default globally (`src/agents/models/default_models.py:99`) optimizes for high-volume cost out-of-the-box but cannot adapt per-request complexity; a request that needs frontier reasoning pays the same cheap-model attempt plus retry latency before human intervention.

## Failure Modes / Edge Cases

- **No degraded-service path on frontier overload**: If `gpt-5.6-sol` returns 429/503 without `retry_after`, `provider_suggested()` may veto or `network_error()` may retry same model until `max_retries` exhausted, then throw — no automatic escalation to `gpt-5.6-mini`/`nano` despite them being cheaper and likely available.
- **Silent cost drift via forgotten `RunConfig.model` override**: `RunConfig.model` globally overrides per-agent tiers (`src/agents/run_config.py:353`); a debug override left in production silently routes all agents to frontier model with no guardrail or trace warning (only `generation` model field changes).
- **Prefix routing misconfiguration leaks cost**: `unknown_prefix_mode="error"` (default) fails fast on typo `openrouter/...` (`src/agents/models/multi_provider.py:123`), but `model_id` pass-through can send `openrouter/openai/gpt-4o` literally to OpenAI endpoint, causing 404/billing confusion with no cost-aware validation.
- **Stateful-request retry veto traps latency**: Requests with `previous_response_id`/`conversation_id` (`src/agents/run_internal/model_retry.py:453-469`) fail closed unless `provider_advice.replay_safety=="safe"`; a cheap-model precheck that set stateful context then fails cannot even retry the cheap model, forcing full chain restart.
- **Tracer-blind cost regressions**: Because `AgentSpanData` lacks model tier and `GenerationSpanData.model_config` omits cost tier (`src/agents/tracing/span_data.py:28-61, 169-209`), dashboards cannot alert on "cheap-model hit-rate dropped 30%" without joining external model-pricing tables.
- **Handoff semantic drift breaks cost assumption**: LLM may handoff to expensive specialist unnecessarily (e.g., routing simple Spanish to frontier agent) — no cost circuit breaker exists to re-route to `gpt-5-nano` when confidence is low.
- **`extra_args.service_tier` typo/unsupported silently ignored**: `validate_fields` in `_validate_first_party_model_settings` (`src/agents/model_settings.py:333-374`) validates `retry`/`tool_choice`/`prompt_cache_options` but not `extra_args` contents; misspelling `service_tier` as `serviceTier` passes locally then fails server-side with opaque 400, no SDK retry.

## Future Considerations

- **Introduce declarative router abstraction**: Add `ModelRouter` interface `select_model(RouterContext{input_tokens, estimated_complexity, latency_budget, risk_label}) -> ModelSelection{model, tier, reason}` with built-in policies (e.g., `ComplexityThresholdRouter`, `LatencyBudgetRouter`, `CostCapRouter`) and wire via `RunConfig.router` plus per-`Agent.router_override`.
- **Implement tiered fallback chains**: Allow `ModelTierConfig {primary: "gpt-5.6-sol", fallbacks: ["gpt-5-mini","gpt-5-nano"], criteria: [overload, budget]}`, integrated with existing `ModelRetrySettings` so retry walks the chain with `generation` retry-reason tagging.
- **Add routing decision span**: New `RouterSpanData{input_hash, tiers_considered, selected_tier, criteria, estimated_cost, latency_ms}` exported via `router_span()` in `tracing/create.py` so cost-savings and miss-rate are observable without joining external data.
- **Expose cost/latency budget in `ModelSettings`/`RunConfig`**: Add `cost_budget`, `latency_budget_ms`, `quality_floor` fields alongside `timeout` (`src/agents/model_settings.py:215`) to make budgets first-class, validated, and traceable.
- **Standardize tier names and validation**: Provide `TIER_PRESETS = {"cheap":"gpt-5.6-luna", "balanced":"gpt-5-mini", "frontier":"gpt-5.6-sol"}` with env `OPENAI_DEFAULT_TIER` as alternative to `OPENAI_DEFAULT_MODEL`, and validate tier strings in `Agent.__post_init__` (`src/agents/agent.py:432`).
- **Retry-with-degradation mode**: Extend `retry_policies` with `fallback_to_tier(tier)` helper that returns `RetryDecision` with `fallback_model`, reconciled with replay-safety gates to allow degraded retries for non-stateful, non-side-effecting calls.
- **Docs pattern for cheap→expensive orchestration**: Promote `agent_patterns/routing.py` variant where cheap classifier routes to expensive specialists, explicitly naming cost/quality tradeoff and include `extra_args.service_tier` guidance.

## Questions / Gaps

- No evidence found of any quality-cost router implementation, routing policy file, or threshold config after exhaustive grep of `src/agents/**` for `router`, `routing`, `fallback_chain`, `quality.*cost`, `tier`, and inspection of `AGENTS.md`, `.agents/references/`, and `docs/models/index.md`. Search boundary: entire `sources/openai-agents-sdk/src` and `docs/`.
- No evidence of cost-aware A/B or shadow routing (duplicate request to cheap+expensive for质量 eval) — not present in `retry.py` or `run_internal/`.
- No evidence of latency-driven routing (e.g., `timeout` only bounds single attempt — `src/agents/model_settings.py:215-222` and `run_internal/model_retry.py:288-316` — not a tier selector).
- No evidence of risk-based routing (e.g., PII or tool-side-effect flag upgrading tier) — not in `Tool` or `Handoff` definitions.
- Open question: does hosted multi-agent `OpenAIHostedMultiAgentModel` (`src/agents/extensions/experimental/hosted_multi_agent/model.py:413`) perform internal subagent cost routing on the service side? Not observable from client SDK — requires `openai` API server documentation probe.
- Open question: are `Agent-as-tool` nested runs (`Agent.as_tool` — `src/agents/agent.py:583-1040`) intended as the cost-routing primitive (cheap orchestrator calling expensive subagent as tool)? Docs distinguish handoffs vs agents-as-tools (`docs/multi_agent.md:29`) but do not frame as quality-cost control.

---

Generated by `Dimension 20.03: Quality-Cost Routing` against `openai-agents-sdk`.
