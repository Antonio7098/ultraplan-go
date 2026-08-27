# Source Analysis: crewai

## Quality-Cost Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python / Pydantic + LiteLLM + native SDKs (OpenAI, Anthropic, Azure, Gemini, Bedrock, OpenRouter, Ollama, etc.) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI provides extensive multi-model breadth (8+ native providers plus a LiteLLM fallback covering 100s of models) and per-component model assignment (per-`Agent.llm`, `Crew.manager_llm`, `Crew.planning_llm`, `Crew.chat_llm`, `Flow` conversational `router.llm`/`intent_llm`/`llm`), but it implements **no quality-cost router**. Model choice is static, user-authored assignment — there is no tier definition, no automatic cost/latency/risk/quality-based selection, no fallback chain across model tiers, and no routing-decision trace. The only `router` in the codebase is the `Flow` control-flow `@router` decorator (which routes *execution to next Flow method* by string return value), and an intent-classification router for conversational Flows (which falls back through `router.llm → intent_llm → llm`). Both are unrelated to cost-aware model routing. LLM calls are observable via `LLMCallStarted/Completed/Failed` events and `UsageMetrics`, but routing policy itself is not a first-class concept.

## Rating

**2 / 10 — Absent**

Rationale: Multiple model tiers are *available* (native providers + LiteLLM) so the lower bound is not 1, but all routing criteria, fallback chains, decision tracing, and configurable policy are absent. Selection is manual imperative assignment; cheap-vs-expensive delegation is left entirely to the user. No evidence of cost, latency, quality, or risk driven switching; no `Router`/`fallbacks` config; no tests for tiered routing.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Multi-model config — native provider allowlist | `SUPPORTED_NATIVE_PROVIDERS` lists 14 providers: `openai, anthropic, azure, gemini, bedrock, openrouter, deepseek, ollama, hosted_vllm, cerebras, dashscope, snowflake` | `lib/crewai/src/crewai/llm.py:327-345` |
| Multi-model config — context sizes | `LLM_CONTEXT_WINDOW_SIZES` maps ~100+ model keys to token windows (e.g. `gpt-4o:128000`, `gemini-1.5-pro:2097152`) | `lib/crewai/src/crewai/llm.py:168-323` |
| Multi-model config — provider constants | `MODELS` dict by provider (`openai: [gpt-4, gpt-4o, o1-mini...]`, `bedrock: [...]`, `groq: [...]`) and `PROVIDERS` list & `DEFAULT_LLM_MODEL="gpt-4.1-mini"` | `lib/crewai/src/crewai/constants.py:137-349` |
| Multi-model config — LLM factory routing | `LLM.__new__` is a factory: priority (1) `custom_openai` forces `openai`, (2) explicit `provider`, (3) `model` prefix check `openai/anthropic/azure/gemini/bedrock`, (4) inferred provider; native class via `_get_native_provider` or `use_native=False` → LiteLLM fallback path | `lib/crewai/src/crewai/llm.py:393-512` |
| Multi-model config — provider inference | `_infer_provider_from_model` and `_matches_provider_pattern` handle `gpt-`, `claude-`, `gemini-`, `deepseek`, `ollama` etc.; `_validate_model_in_constants` checks constants or pattern | `lib/crewai/src/crewai/llm.py:515-662` |
| Multi-model config — native provider resolution | `_get_native_provider` maps to `OpenAICompletion`, `AnthropicCompletion`, `GeminiCompletion`, `BedrockCompletion`, `SnowflakeCompletion`, `OpenAICompatibleCompletion` | `lib/crewai/src/crewai/llm.py:664-715` |
| Multi-model config — LiteLLM fallback path | If `is_litellm` or `provider not in SUPPORTED_NATIVE_PROVIDERS` or no native class → `_ensure_litellm()`; error message instructs `uv add 'crewai[litellm]'` if unavailable | `lib/crewai/src/crewai/llm.py:493-512` |
| Multi-model config — LLM model fields | `LLM` declares `model`, `temperature`, `top_p`, `timeout`, `reasoning_effort`, `base_url`, `completion_cost`, etc.; `BaseLLM` declares `model`, `provider`, `temperature`, `base_url`, `is_litellm` | `lib/crewai/src/crewai/llm.py:368-392`, `lib/crewai/src/crewai/llms/base_llm.py:169-190` |
| Per-agent model assignment | `Agent.llm: str|BaseLLM|None` and deprecated `function_calling_llm`; validated via `create_llm` in `post_init_setup`; manager/planning use separate fields | `lib/crewai/src/crewai/agent/core.py:223-236`, `lib/crewai/src/crewai/agent/core.py:364-369` |
| Per-crew model assignments | `Crew.manager_llm`, `Crew.planning_llm`, `Crew.chat_llm`, `Crew.function_calling_llm` each independently configurable; `check_manager_llm` only validates hierarchical need, not tier policy | `lib/crewai/src/crewai/crew.py:275-292`, `lib/crewai/src/crewai/crew.py:348-357`, `lib/crewai/src/crewai/crew.py:376-383`, `lib/crewai/src/crewai/crew.py:722-743` |
| LLM util — env-or-fallback model | `_llm_via_environment_or_fallback` reads `MODEL/MODEL_NAME/OPENAI_MODEL_NAME` or `DEFAULT_LLM_MODEL`; no tier or cost logic | `lib/crewai/src/crewai/utilities/llm_utils.py:97-110` |
| LLM util — generic factory | `create_llm` accepts `str|dict|LLM|BaseLLM`, delegates to `LLM(model=...)` — single-model construction only, no routing table | `lib/crewai/src/crewai/utilities/llm_utils.py:13-87` |
| Absent: quality-cost router | No file defines cost/latency/quality/risk routing. `grep -ri "routing.*policy|quality.*cost|cost.*routing|tier"` finds only flow routing and conversational fallback, not model-tier routing | `lib/crewai/src/crewai/llm.py` + `lib/crewai/src/crewai/flow/` (search boundary: `grep -rn fallbacks, litellm.*Router` returned zero hits for model routing) |
| Absent: fallback chain | No `fallbacks: [{model, priority}]` config; `fallback` string search hits only `fallback_intent`, `fallback call_id`, file-processing fallback — no cross-model retry chain. `litellm.Router` never imported | `lib/crewai/src/crewai/llm.py:499,2391-2432`, `lib/crewai/src/crewai/flow/conversational_definition.py:31`, `lib/crewai/src/crewai/llms/base_llm.py:142-147` |
| Flow router (not cost router) | `@router` decorator sets `FlowMethodDefinition(router=True, emit=[...])`; `_find_triggered_methods(..., router_only=True)` routes Flow execution by string return value — control-flow only | `lib/crewai/src/crewai/flow/dsl/_router.py:97-163`, `lib/crewai/src/crewai/flow/runtime/__init__.py:3054-3124` |
| Conversational router (intent, not cost) | `FlowConversationalRouterDefinition` has `llm`, `fallback_intent="converse"`, `intent_field`; `FlowConversationalDefinition.llm` is "the last router fallback: router uses router.llm, then intent_llm, then this" — LLM selection hierarchy for chat, not cost tier | `lib/crewai/src/crewai/flow/conversational_definition.py:15-32`, `lib/crewai/src/crewai/flow/conversational_definition.py:52-59` |
| Routing decision traces — absent | No `RoutingDecision` event. LLM events record `model` + `call_id` + `usage` + `finish_reason/response_id` but not "why this tier was chosen" or alternative candidates | `lib/crewai/src/crewai/events/types/llm_events.py:38-99`, `lib/crewai/src/crewai/llms/base_llm.py:552-643` |
| Token/cost observability (not routing) | `UsageMetrics` normalizes `prompt_tokens/completion_tokens/cached_prompt_tokens/reasoning_tokens`; aggregated per-LLM via `_track_token_usage_internal` and per-crew via `calculate_usage_metrics`; `completion_cost: float|None` field exists but never drives routing | `lib/crewai/src/crewai/types/usage_metrics.py:32-189`, `lib/crewai/src/crewai/llms/base_llm.py:244-254,955-972`, `lib/crewai/src/crewai/crew.py:2201-2225`, `lib/crewai/src/crewai/llm.py:370` |
| Retry — same model, not fallback tier | `Agent.max_retry_limit` (default 2) re-invokes `execute_task` on the *same* agent/LLM; litellm exceptions are re-raised directly. No tier escalation | `lib/crewai/src/crewai/agent/core.py:255-258`, `lib/crewai/src/crewai/agent/core.py:733-769` |

## Answers to Dimension Questions

**1. Are multiple model tiers available?**
Yes, broadly — but as a *catalog*, not as tiers. `LLM` supports 14 native providers via `SUPPORTED_NATIVE_PROVIDERS` (`lib/crewai/src/crewai/llm.py:327`) with 100+ known model ids in `LLM_CONTEXT_WINDOW_SIZES` (`lib/crewai/src/crewai/llm.py:168`) and `constants.MODELS` (`lib/crewai/src/crewai/constants.py:152`). Anything not native falls through to LiteLLM (`lib/crewai/src/crewai/llm.py:493`). Crew/Agent granularity allows different models per role (`Agent.llm` at `lib/crewai/src/crewai/agent/core.py:223`, `Crew.manager_llm` at `lib/crewai/src/crewai/crew.py:275`, `Crew.planning_llm` at `lib/crewai/src/crewai/crew.py:348`, `Crew.chat_llm` at `lib/crewai/src/crewai/crew.py:376`). However there is no first-class notion of "cheap/fast vs expensive/capable" tier — the user must manually assign `openai/gpt-4o-mini` to one agent and `openai/gpt-4o` to another. No `tier: {low, high}` abstraction exists.

**2. What criteria drive model selection?**
Purely static, ahead-of-time assignment. Criteria searched: `cost`, `latency`, `quality`, `risk`, `tier`, `routing policy` — no evidence found. The only selection logic is the `LLM.__new__` provider-dispatch (`lib/crewai/src/crewai/llm.py:393`) which chooses *which SDK class* to instantiate based on string prefix or an explicit `provider` kwarg, not based on request complexity, token budget, or SLA. Environment fallback is single-model (`MODEL` env or `DEFAULT_LLM_MODEL="gpt-4.1-mini"` at `lib/crewai/src/crewai/constants.py:348` via `lib/crewai/src/crewai/utilities/llm_utils.py:97`). No scoring, no A/B, no latency budget, no token-limit escalation.

**3. Are fallback chains defined?**
No. There is no `fallbacks = ["gpt-4o-mini", "gpt-4o"]` or `litellm.Router(fallbacks=[...])` configuration anywhere. Search for `fallbacks`, `Router`, `litellm.Router` returned zero model-tier hits (only `fallback_intent` for conversational flows at `lib/crewai/src/crewai/flow/conversational_definition.py:31` and `fallback call_id` at `lib/crewai/src/crewai/llms/base_llm.py:144`). Agent retry (`max_retry_limit=2` at `lib/crewai/src/crewai/agent/core.py:255`) retries the *same* LLM; `litellm` errors bypass the retry loop entirely (`lib/crewai/src/crewai/agent/core.py:743`). Conversational fallback chain `router.llm → intent_llm → llm` (`lib/crewai/src/crewai/flow/conversational_definition.py:20-57`) is for chat intent classification, not cost fallback.

**4. Are routing decisions observable?**
Partially for *calls*, not for *routing*. Every LLM call emits `LLMCallStartedEvent` with `model`, `call_id`, `temperature/max_tokens/stream/stop` (`lib/crewai/src/crewai/events/types/llm_events.py:38` and `lib/crewai/src/crewai/llms/base_llm.py:552`) and `LLMCallCompletedEvent` with `response`, `usage`, `finish_reason`, `response_id` (`lib/crewai/src/crewai/events/types/llm_events.py:90`). `UsageMetrics` is aggregated per instance and per crew/flow (`lib/crewai/src/crewai/types/usage_metrics.py:65`, `lib/crewai/src/crewai/crew.py:2201`). But there is no `RoutingDecisionEvent` that records candidates considered, score, cost estimate, latency, or reason for the chosen tier. The `trace_listener`/`OTel` layer forwards those LLM events but has nothing routing-specific to forward.

## Architectural Decisions

*   **Factory-instantiated `LLM` with provider-aware dispatch** (`lib/crewai/src/crewai/llm.py:393`): `LLM.__new__` decides at construction time whether to return an `OpenAICompletion`/`AnthropicCompletion`/etc. or fall through to the `LLM` (litellm) instance. This isolates provider specifics but locks choice to a single model string per instance — there is no wrapper that holds multiple candidates and picks at call time.
*   **Manual per-role assignment over automatic tiering** (`lib/crewai/src/crewai/agent/core.py:223`, `lib/crewai/src/crewai/crew.py:275,348,376`): Architecture assumes the crew author knows which agent deserves which model. `Crew` even shallow-copies `manager_llm` (`lib/crewai/src/crewai/crew.py:2140`) for fork isolation, but never performs cost arbitration.
*   **LiteLLM as transparent fallback, not as router** (`lib/crewai/src/crewai/llm.py:493`, `lib/crewai/pyproject.toml: litellm>=1.84.0`): LiteLLM is an optional dep (`crewai[litellm]`) used for breadth; CrewAI does not import `litellm.Router` which would provide `routing_strategy="cost"`, `fallbacks`, and `num_retries` out of the box.
*   **Token accounting without cost routing** (`lib/crewai/src/crewai/types/usage_metrics.py:32`, `lib/crewai/src/crewai/llms/base_llm.py:244`): Precise normalization of provider-specific usage keys (including Anthropic cache reconciliation at `lib/crewai/src/crewai/types/usage_metrics.py:112`) exists, but the accounting is *post-hoc reporting* (`CrewOutput.token_usage`, `Flow.usage_metrics`) rather than a *pre-call routing input*.
*   **Two unrelated meanings of "router"** (`lib/crewai/src/crewai/flow/dsl/_router.py:97`, `lib/crewai/src/crewai/flow/conversational_definition.py:15`): The only router abstractions are flow control and chat intent classification, which creates naming collision risk if a future cost-router is introduced.

## Notable Patterns

*   **Pattern — single-model instance, caller-side polymorphism:** `create_llm(value: str|dict|LLM|BaseLLM)` (`lib/crewai/src/crewai/utilities/llm_utils.py:13`) normalizes any form to a single `BaseLLM`; callers hold the reference (agent, crew, flow) and never ask a router to pick among models.
*   **Pattern — provider-pattern fallback for unknown models:** `_matches_provider_pattern` (`lib/crewai/src/crewai/llm.py:515`) allows `gpt-5`, `claude-`, `gemini-`, `deepseek` prefixes to pass validation even when not in constants, future-proofing breadth without a tier system.
*   **Pattern — usage delta for per-call attribution:** `UsageMetrics.delta_since(baseline)` (`lib/crewai/src/crewai/types/usage_metrics.py:79`) snapshots the monotonic per-LLM counter before/after a call — good primitive for a future cost-aware router to consult, currently unused for routing.
*   **Pattern — conversational LLM fallback hierarchy:** `router.llm → intent_llm → llm` (`lib/crewai/src/crewai/flow/conversational_definition.py:20-57` description) is the closest to a fallback chain in the repo, but scoped to chat only and undocumented as a cost optimization.

## Tradeoffs

*   **Breadth vs. tiering:** Supporting every provider/model maximizes choice but delegates tiering entirely to users. Hiring a smaller, cheaper model for trivial tasks requires manual duplication of agent definitions; the alternative (automatic tier router) would add complexity and non-determinism that CrewAI currently avoids.
*   **Determinism vs. adaptability:** Static assignment is deterministic and debuggable (`LLM.model` is an explicit string on every agent), but forfeits the 30-50% cost savings that automatic cheap-first routing can deliver on mixed workloads — acknowledged by the `completion_cost` field (`lib/crewai/src/crewai/llm.py:370`) being stored but never acted upon.
*   **LiteLLM minimal use vs. Router capabilities:** Declaring LiteLLM as optional and using only `completion` avoids pulling in Redis, cooldowns, and routing state, keeping the dependency light, but leaves `routing_strategy`, `model_group_aliases`, and `fallbacks` (features that directly answer this dimension) on the table.
*   **Event richness vs. routing silence:** LLM call events carry sampling params and usage (`lib/crewai/src/crewai/events/types/llm_events.py:51-61`), yet no policy trace exists — an external observer cannot answer "why was gpt-4o chosen over gpt-4o-mini for this request?".

## Failure Modes / Edge Cases

*   **Provider-not-installed → hard failure, no fallback tier:** If the user supplies a valid native model but the provider SDK is missing, `LLM.__new__` raises `ImportError("Error importing native provider")` (`lib/crewai/src/crewai/llm.py:491`); if the model is not native and LiteLLM is not installed, it raises `ImportError("LiteLLM fallback package is not installed")` (`lib/crewai/src/crewai/llm.py:494-510`). Neither path tries a cheaper/faster alternative.
*   **Agent retry retries same expensive model:** `Agent._check_execution_error` / `_handle_execution_error` (`lib/crewai/src/crewai/agent/core.py:733-769`) re-invokes `execute_task` with the same `agent.llm`; transient failures on an expensive model are not escalated to a fallback tier nor degraded to a cheaper one.
*   **Hierarchical process binds to a single `manager_llm`** (`lib/crewai/src/crewai/crew.py:275,1531-1540`): If the manager model hits rate limits or context limits, no alternate manager tier is attempted and `CrewKickoffFailedEvent` is emitted.
*   **Cost is not enforced as a guardrail:** `UsageMetrics` (`lib/crewai/src/crewai/types/usage_metrics.py:32`) accumulates tokens but there is no `max_cost` / `budget_exceeded` gate that could short-circuit or downgrade mid-crew. A loop of tool calls can silently burn budget.
*   **Name collision for future routing:** Adding a cost router named `Router` or `router` would collide with `Flow`'s `RouterMethod` (`lib/crewai/src/crewai/flow/flow_wrappers.py:159`) and `FlowConversationalRouterDefinition.router` (`lib/crewai/src/crewai/flow/conversational_definition.py:61`).

## Future Considerations

*   Introduce a `RoutingPolicy` model (`policy.yaml` or `Crew.routing: {strategy: "cost"|"latency"|"quality", tiers: [{model, max_tokens, cost_per_1k}], fallbacks: [...]}`) that wraps `create_llm`. Start with the two-priority signal from the dimension question: classify tasks by expected complexity (e.g., `Task.complexity: low|high` defaulting to cheap vs expensive) and let a `LiteLLM Router` or custom selector pick.
*   Adopt `litellm.Router(routing_strategy="cost_based_routing" | "latency_based_routing", fallbacks=[...], num_retries=2)` instead of raw `litellm.completion` for the fallback path — this is the smallest change that adds the missing tier fallback semantics with minimal new code.
*   Emit a `RoutingDecisionEvent` (`chosen_model`, `candidates`, `criterion`, `estimated_cost`, `latency_budget_ms`) from the router and include it in `TraceCollectionListener`/OTel spans so decisions are observable without parsing LLM start events.
*   Gate `BaseLLM._track_token_usage_internal` into a budget enforcer: `Crew(max_cost_usd=2.0)` that checks cumulative `UsageMetrics` (or LiteLLM's `completion_cost` when available) and forces downgrade to `fallback_intent="end"` or cheapest tier on overrun.
*   Add tests that simulate a cheap-model failure then fallback to the expensive tier (and vice-versa for budget exhaustion) so the policy is exercised under failure, not just happy path.

## Questions / Gaps

*   No evidence found for any `routing_policy`, `model_tier`, `cost_threshold`, `latency_budget`, or `risk_score` config key in `lib/crewai/src/crewai/` (searched `grep -rn "routing.*policy|tier|fallback.*model|litellm.*Router"`). Confirmed absent, not just undocumented.
*   `LLM.completion_cost: float | None` (`lib/crewai/src/crewai/llm.py:370`) is declared but never populated or consumed for routing — unclear if it is legacy or intended for a future cost router. No assignment found via grep.
*   Whether CrewAI Enterprise (outside this OSS source) provides a centralized router: `lib/crewai/src/crewai/plus_api.py` and references to `CREWAI_ENTERPRISE_DEFAULT_OAUTH2_*` (`lib/crewai/src/crewai/constants.py:5`) exist but the enterprise service code is not in scope — no claim can be made.
*   The conversational `answer_from_history_llm` (`lib/crewai/src/crewai/flow/conversational_definition.py:71`) suggests per-intent model customization exists for chat, but no analogous per-task complexity signal exists for crews.

---

Generated by `Dimension 20.03: Quality-Cost Routing` against `crewai`.
