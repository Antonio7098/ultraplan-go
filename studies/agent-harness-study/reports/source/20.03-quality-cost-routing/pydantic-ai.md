# Source Analysis: pydantic-ai

## 20.03 Quality-Cost Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python / pydantic-ai-slim, pydantic-graph, uv workspace |
| Analyzed | 2026-08-26 |

## Summary

pydantic-ai does not implement automatic quality-cost routing (cheap model for simple requests, expensive for complex). It instead provides composable primitives that allow users to build routing: a sequential `FallbackModel` failover chain (exception- and response-triggered), a per-step `SelectModel` / `ModelSelector` capability that receives usage/messages/deps and returns a model instance or ID, and a best-effort cost accounting layer (`genai-prices` + `UsageLimits.cost_limit`). Fallback is mature and heavily tested (streaming, continuation pinning, OTel span patching, rejected-cost aggregation); selector-based routing is fully user-driven with no built-in heuristics for cost, latency, risk or quality. Routing decisions are observable only via updated OTel span attributes (`gen_ai.request.model`) and `FallbackExceptionGroup`, not a dedicated routing audit log. No declarative routing policy config exists.

## Rating

**5 / 10** — Present but inconsistent.

Fallback chains are clear, tested, and operationally safeguarded (adequate for 7-8 on fallback alone). Quality-cost routing as a dimension is absent as a first-class feature: no tier definitions, no built-in cost/latency/risk criteria, no policy DSL. Per-step selection exists via `SelectModel` but is entirely custom code with no framework-provided cheap-vs-expensive heuristic, no tier registry, and no built-in tracing of "why this model" beyond the final span model name. Weakly documented as a routing pattern; fragile if treated as a managed router.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Multi-model config | `KnownModelName` literal union enumerates hundreds of `provider:model` IDs (e.g. `openai:gpt-5`, `anthropic:claude-sonnet-4-5`). | `pydantic_ai_slim/pydantic_ai/models/_known_model_names.py:13-15` |
| Multi-model config | `known_model_names()` enumerates known IDs via `get_literal_values(KnownModelName)`. | `pydantic_ai_slim/pydantic_ai/models/__init__.py:112-120` |
| Multi-model config | `infer_model(model: Model \| KnownModelName \| str)` resolves string IDs to concrete `Model` instances. | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1529-1530` |
| Multi-model config | `Agent` accepts `model: Model \| KnownModelName \| str \| None` and optional `ModelSelector` callable via capabilities; model layers resolved per-run. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:751-753`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:64-68` |
| Router / routing criteria | `ModelSelector = Callable[[ModelSelectionContext], ModelSelection \| Awaitable[ModelSelection]]`; context provides `deps`, `messages`, `usage`, `run_step`, `model`. | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:64-64` |
| Router / routing criteria | `SelectModel` capability wraps a `ModelSelector`; `get_model()` returns it for per-step evaluation. | `pydantic_ai_slim/pydantic_ai/capabilities/select_model.py:11-22` |
| Router / routing criteria | `ModelSelectionContext` dataclass adds `run_step`, `messages`, `usage` to `ModelResolutionContext`. | `pydantic_ai_slim/pydantic_ai/models/__init__.py:350-364` |
| Router / routing criteria | Graph helpers hold `model_selector`, `model_selected_for_step`, `evaluate_model_selector`; `AbstractCapability.get_model()` doc: last non-None wins. | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:392-397`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:371-385` |
| Router / routing criteria | `Agent._resolve_model_selection` delegates to `capability.resolve_model_id()` chain then `infer_model`. `resolve_model_id` first-wins. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2762-2782` |
| Router / routing criteria | OpenRouter provider exposes routing knobs: `order`, `allow_fallbacks`, `sort` (`price`\|`throughput`), `max_price`, quantizations — provider-level routing, not framework router. | `pydantic_ai_slim/pydantic_ai/models/openrouter.py:195-227` |
| Fallback chain definitions | `FallbackModel(Model)` with `models: list[Model]`, `fallback_on: FallbackOn` (exception types, `ExceptionHandler`, `ResponseHandler`, mixed sequence). | `pydantic_ai_slim/pydantic_ai/models/fallback.py:86-127` |
| Fallback chain definitions | `_parse_fallback_on` auto-detects handler type via `ModelResponse` type hint; raises `UserError` if empty. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:135-165` |
| Fallback chain definitions | `request()` loops `self.models` sequentially; `await _should_fallback(exc)` / `_should_fallback(response)` decides continuation; continuation-pin bypass with rewind. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:229-318` |
| Fallback chain definitions | `request_stream()` mirrors sequential fallback with `AsyncExitStack` per model; rewind on pinned-continuation failure before chain. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:320-406` |
| Fallback chain definitions | `FallbackExceptionGroup('All models from FallbackModel failed', all_errors)` aggregates exceptions + `ResponseRejected`. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:544-554`, `pydantic_ai_slim/pydantic_ai/exceptions.py:612-613` |
| Fallback chain definitions | Continuation pinning: `_stamp_continuation` writes `metadata['__pydantic_ai__']['fallback_model_id']`; `_get_continuation_model` routes suspended turn. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:461-501` |
| Cost accounting | `RequestUsage.cost: Decimal \| None` and `RunUsage` with `_incr_usage_cost`; `fill_response_cost` via `best_effort_price` + `genai-prices`. | `pydantic_ai_slim/pydantic_ai/usage.py:130-135`, `pydantic_ai_slim/pydantic_ai/_cost.py:98-118` |
| Cost accounting | `UsageLimits.cost_limit: Decimal \| None` with `check_cost()` and `check_before_request()`; warns via `CostNotFoundWarning` if pricing unavailable. | `pydantic_ai_slim/pydantic_ai/usage.py:417-435`, `pydantic_ai_slim/pydantic_ai/usage.py:492-535` |
| Cost accounting | Rejected-response cost aggregation in `FallbackModel.request`: `fill_response_cost` + `rejected_cost` rolled into successful response. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:296-306` |
| Routing decision traces | `FallbackModel._set_span_attributes()` patches active OTel span `gen_ai.request.model` to winning inner model + refreshes `model_request_parameters`. | `pydantic_ai_slim/pydantic_ai/models/fallback.py:477-489` |
| Routing decision traces | `open_model_request_span` records `gen_ai.request.model`, `gen_ai.provider.*`, `gen_ai.tool.definitions`, `model_request_parameters`, and per-response `gen_ai.response.model` + `operation.cost`. | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:445-545` |
| Routing decision traces | `response_attributes()` and `response_price_calculation()` compute cost attribute for spans/metrics. | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:399-431` |
| Routing policy config | No declarative routing policy file/DSL found; routing is imperative via `SelectModel` selector or `FallbackModel.fallback_on` constructor args. | `pydantic_ai_slim/pydantic_ai/capabilities/select_model.py:10-26` (no config counterpart) |
| Tests - fallback maturity | 55+ dedicated fallback tests covering streaming error fallback, lifecycle, reentrant locks, concurrent entry, continuation pinning/rewind, cancellation, cost-counting, handler parsing. | `tests/models/test_fallback.py:1-3369` (e.g., `:1146`, `:849`, `:2013`, `:2164`) |

## Answers to Dimension Questions

**1. Are multiple model tiers available?**

Partially. The framework ships a flat registry of hundreds of concrete model IDs (`KnownModelName` in `pydantic_ai_slim/pydantic_ai/models/_known_model_names.py:13`) and resolves them via `infer_model` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1529`). `FallbackModel` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:86`) lets users compose an ordered list `Model | KnownModelName | str` into a failover chain (variadic `default_model, *fallback_models`). `SelectModel` (`pydantic_ai_slim/pydantic_ai/capabilities/select_model.py:11`) plus `ModelSelector` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:64`) allow per-step selection. There is no built-in notion of "tiers" (cheap/fast vs expensive/quality) — no tier enum, no tier config, no profile key like `quality_tier` or `cost_tier`. Tiers must be invented by user code that maps `ModelSelectionContext.usage`/`messages` to a model choice.

**2. What criteria drive model selection?**

- For `FallbackModel`: criteria are failure-driven via `fallback_on: FallbackOn` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:50-57`). Default is `ModelAPIError`; can be extended to arbitrary `ExceptionHandler` / `ResponseHandler` (auto-detected by `ModelResponse` type hint at `pydantic_ai_slim/pydantic_ai/models/fallback.py:67-77`). Typical use is error code/status-code or latency timeout that surfaces as an exception. No built-in cost/latency/quality scorer.
- For `SelectModel`: criteria are entirely user-defined inside the `ModelSelector` function, which receives `ModelSelectionContext` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:350`) containing `deps`, `messages`, `usage` (with `input_tokens`, `output_tokens`, `cost`), `run_step`, and the lower-precedence model. The agent graph evaluates it each logical step (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:392-397`). OpenRouter's provider-level `sort: price | throughput` and `max_price` (`pydantic_ai_slim/pydantic_ai/models/openrouter.py:222-227`) are the only declarative quality-cost hints, and they delegate to the external OpenRouter gateway.

No evidence of internal heuristics that automatically escalate from cheap to expensive based on prompt complexity, trust/risk score, or predicted quality.

**3. Are fallback chains defined?**

Yes, and they are the most mature part of this dimension. `FallbackModel` defines an ordered chain `models: list[Model]` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:92-127`). `request()` and `request_stream()` iterate sequentially (`fallback.py:282`, `fallback.py:374`), preparing each model per-request (`prepare_request`/`prepare_messages` per model to respect profile differences), checking `await _should_fallback(value)` before moving on, aggregating `exceptions` + `rejected_responses`, and raising `FallbackExceptionGroup` if all fail (`fallback.py:544`). Advanced behaviors: stateless continuation pinning for suspended/background jobs (`fallback.py:492-532`), rewind of `ModelResponse(state='suspended')` on pin failure (`fallback.py:522-532`), `_stamp_replace_previous` to avoid duplication after rewind (`fallback.py:504-519`), best-effort `cancel_suspended_response` before rewind (`fallback.py:270-274`), rejected-cost roll-up (`fallback.py:296-306`), and `anyio.Lock` for reentrant lifecycle (`fallback.py:98-102`, `lifecycle tests at tests/models/test_fallback.py:2013-2164`). Agent-level per-step selector is an alternative routing chain where last non-None `get_model()` wins.

**4. Are routing decisions observable?**

Weakly. Successful fallback updates the active OTel `chat` span via `FallbackModel._set_span_attributes` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:477-489`), patching `gen_ai.request.model` to the winning inner model's `model_name` (checked via `span.attributes['gen_ai.request.model'] == self.model_name` before patching). `open_model_request_span` (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:445-545`) emits per-request `gen_ai.request.model`, `gen_ai.response.model`, `gen_ai.tool.definitions`, `model_request_parameters` (when enabled), and cost histograms. Failure of the whole chain is observable as `FallbackExceptionGroup` (carries all inner exceptions). There is no dedicated routing decision event/log that records "tried model A -> failed with X -> fell back to B (reason Y) -> cost delta Z". The per-attempt errors are only visible if the caller inspects `FallbackExceptionGroup.__notes__`/`exceptions`; intermediate attempts' `gen_ai.request.model` values are overwritten rather than appended as a chain. `SelectModel` decisions have no built-in trace beyond the resulting model request span.

## Architectural Decisions

- **Failover over routing.** The framework frames multi-model use as reliability (fault tolerance) not economy. `FallbackModel`'s default is `ModelAPIError` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:108`), and its design docstring says "upon failure" — quality-cost downgrades are not assumed. This keeps the core simple but leaves tiered routing to users.
- **Per-model request preparation.** `FallbackModel.request` re-runs `prepare_request`/`prepare_messages` per inner model (`pydantic_ai_slim/pydantic_ai/models/fallback.py:284-287`). Decision preserves provider/profile correctness (e.g., Anthropic cache breakpoints vs OpenAI) at the cost of extra per-attempt work and loss of a single precomputed request shape.
- **User-provided selector, not policy engine.** Routing policy is a plain Python callable (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:64`) rather than a config file or DSL. This maximizes extensibility (deps-aware, async-capable, can inspect `usage.cost`) but provides no out-of-box cost/latency optimizer, no shared policy validation, and no serializable policy.
- **Stateless continuation via metadata pin.** Suspended-turn affinity is stored in `ModelResponse.metadata['__pydantic_ai__']['fallback_model_id']` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:497-501`) and `replace_previous_response`, not in server state. Enables durable replay without external routing state; couples `FallbackModel` and `models._continuation`.
- **Best-effort pricing.** Cost is populated via `genai-prices` snapshot (`pydantic_ai_slim/pydantic_ai/_cost.py:21-27`, `98-118`) and allowed to be `None`. `UsageLimits.cost_limit` warns rather than crashes when pricing is unavailable (`pydantic_ai_slim/pydantic_ai/usage.py:528-535`). Prevents routing decisions from failing due to pricing metadata gaps.

## Notable Patterns

- **Sequential failover with predicate handlers.** `FallbackOn` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:50-57`) unifies exception types, sync/async exception predicates, and response predicates behind auto-detection (`_is_response_handler` at `fallback.py:67`). Pattern is reusable but conflates error handling and quality gating.
- **Span attribute patching.** `FallbackModel._set_span_attributes` reads `get_current_span().attributes` and conditionally refreshes `model_attributes` + `model_request_parameters_attributes` (`fallback.py:478-489`). Avoids leaking `model_request_parameters` when `include_model_request_parameters=False`.
- **Continuation pin + rewind.** `FallbackModel` participates in the paused-turn protocol (`_continuation.py`) by stamping the chosen model ID into `metadata`, rewinding history (`_rewind_messages` at `fallback.py:522`), and stamping `replace_previous_response` so the fresh response correctly merges (`fallback.py:504-519`). Tested for both `request` and `request_stream` (`tests/models/test_fallback.py:2164-3319`).
- **Last-wins model composition.** Agent graph composes `get_model()` contributions where last non-`None` wins; `resolve_model_id` first-wins. Mirrors middleware wrapping order for predictability, but semantics differ between the two hooks and must be learned.

## Tradeoffs

- **Reliability-first, economy-second.** Investing in `FallbackModel` fidelity (streaming, cancellation, cost roll-up) handles provider outages well, but the absence of a cost-aware router forces every team to reimplement cheap/expensive switching, leading to duplication and ad-hoc heuristics.
- **Imperative selector vs declarative policy.** A callable selector is maximally flexible (can use `deps`, `usage`, `messages`, async I/O) but is opaque to tooling: no validation, no serialization into `AgentSpec`, no UI for operators to tune routing without code deploys.
- **Provider-delegated routing (OpenRouter).** Exposing OpenRouter's `sort`/`max_price`/`order` (`models/openrouter.py:195-227`) offloads smart routing to the gateway, reducing framework complexity but creating a split-brain: self-hosted or multi-provider users without OpenRouter get no equivalent.
- **Span mutation vs event.** Overwriting `gen_ai.request.model` on the existing span keeps metric attributes coherent (one span per logical request) but discards the attempt history that a per-attempt child span or dedicated `routing_decision` event would preserve for debugging.

## Failure Modes / Edge Cases

- **Routing based on stale cost.** `RequestUsage.cost` may be `None` if `genai-prices` lacks data for the model/provider (`pydantic_ai_slim/pydantic_ai/_cost.py:78-95`), or if the provider reports no usage. A selector that switches on `ctx.usage.cost > threshold` would silently never trigger; `UsageLimits.check_cost` mitigates only with a `CostNotFoundWarning` (`pydantic_ai_slim/pydantic_ai/usage.py:528-535`) not an error.
- **Intermediate attempt costs hidden.** `request_stream` does not roll up rejected streaming costs (only `request` aggregates `rejected_cost` at `fallback.py:296-306`). A chain with multiple rejected streams could undercount total spend if the caller only inspects `RunUsage`.
- **Fallback after successful response rejected via handler.** Response handlers that return `True` cause the handler to treat a syntactically valid response as a failure and continue; if all responses are rejected, `ResponseRejected(len(rejected_responses))` is appended (`fallback.py:551-553`) and surfaced inside `FallbackExceptionGroup`. Callers expecting only `ModelAPIError` may mishandle this type.
- **Empty `fallback_on` misconfiguration.** Constructing `FallbackModel(..., fallback_on=[])` raises `UserError` (`pydantic_ai_slim/pydantic_ai/models/fallback.py:160-165`), but `fallback_on=(MyError,)` with a never-raised type silently never falls back — no warning.
- **Pinned continuation with missing model.** If metadata references a `model_id` not in `self.models` (e.g. after chain reordering), `_pinned_continuation_model` returns `None` (`fallback.py:470-475`) and the chain falls through to normal retry, potentially re-issuing the request against the wrong model without error.
- **Concurrency and reentrancy.** `FallbackModel` guards provider `__aenter__/__aexit__` with `anyio.Lock` and refcount (`pydantic_ai_slim/pydantic_ai/models/fallback.py:98-103, 184-205`), but high concurrency through a shared `FallbackModel` instance still serializes entry/exit; misuse could cause head-of-line blocking though not a crash (covered by `tests/models/test_fallback.py:2088-2164`).
- **Idempotency of `prepare_request`.** `InstrumentedModel.request` notes `prepare_request` is not idempotent (appends `prompted_output_instructions` twice) and deliberately hands originals to the wrapped model (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:361-368`). `FallbackModel` correctly re-prepares per model, but a custom wrapper that caches prepared parameters could double-append.

## Future Considerations

- Add a first-class tier registry or `ModelTier` capability (e.g. `fast`, `balanced`, `quality`) mapping to concrete IDs per environment, with a `TierRouter` that encapsulates the common `if complexity > threshold then upgrade tier` pattern without each team reinventing it.
- Provide built-in routing criteria helpers: `cost_budget_remaining`, `latency_slo`, `input_tokens_threshold`, and a `quality_gate` that inspects a lightweight classifier or `ToolSearch` signal, composable as predicates for `SelectModel`.
- Emit structured routing traces: a `gen_ai.routing.decision` event or child spans per attempt (`attempt=1 model=... fallback_reason=... latency_ms=... cost_usd=...`) instead of overwriting `gen_ai.request.model`; include serialized `fallback_on` reason.
- Make fallback policy serializable: allow `AgentSpec` to declare `fallback: [{model: "openai:gpt-4o-mini", on: ["ModelAPIError", "429"]}, {model: "anthropic:claude-sonnet-4-5"}]` and validate it at load time.
- Integrate `UsageLimits.cost_limit` with proactive routing: when `usage.cost` approaches `cost_limit`, automatically downgrade subsequent steps or emit a routing decision to downgrade rather than just failing at `check_cost`.

## Questions / Gaps

- No evidence of a latency-aware router (e.g., hedging, deadline-based fallback) — fallback is exception-triggered, not timer-triggered. Searched `providers/` and `models/` for `timeout`/`deadline`/`hedge` routing; none found as routing criteria.
- No evidence of risk-aware routing (e.g., PII detection downgrades model). No search hit for `risk`/`pii` routing in `pydantic_ai_slim/pydantic_ai`.
- No declarative routing policy file (YAML/JSON) or `AgentSpec` routing section — `AgentSpec` not inspected but expected output template references no such config; searched capability and agent init for policy config keys, none found.
- OpenRouter `sort: price | throughput` is provider-side and undocumented as a framework tiering strategy; unclear whether it is intended to be the framework's cost router or merely a passthrough.

---

Generated by `20.03-quality-cost-routing` against `pydantic-ai`.
