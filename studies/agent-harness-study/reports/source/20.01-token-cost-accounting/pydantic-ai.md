# Source Analysis: pydantic-ai

## Dimension 20.01: Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-ai-slim agent framework; pydantic-graph; genai-prices for pricing) |
| Analyzed | 2026-08-24 |

All citations below are workspace-relative to `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

Token and cost accounting in Pydantic AI is a first-class, end-to-end subsystem rather than an afterthought. Two typed dataclasses — `RequestUsage` (per model request) and `RunUsage` (cumulative per run) — carry token counts, cache/audio breakdowns, request/tool-call counters, and a `Decimal` USD cost field (`pydantic_ai_slim/pydantic_ai/usage.py:82-135`). Token counts are extracted from every provider response through the `genai-prices` library's provider registry (`RequestUsage.extract`, `pydantic_ai_slim/pydantic_ai/usage.py:303-334`), with per-provider adapters delegating to it (`pydantic_ai_slim/pydantic_ai/models/openai.py:5020`, `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2842`, `pydantic_ai_slim/pydantic_ai/models/google.py:2051`, `pydantic_ai_slim/pydantic_ai/models/bedrock.py:1874`, `pydantic_ai_slim/pydantic_ai/models/groq.py:863`, `pydantic_ai_slim/pydantic_ai/models/cohere.py:437`, `pydantic_ai_slim/pydantic_ai/models/mistral.py:987`, `pydantic_ai_slim/pydantic_ai/models/xai.py:1404`).

Cost is calculated with `genai-prices` (a hard dependency of the slim package, `pydantic_ai_slim/pyproject.toml:69`) via a dedicated module whose explicit contract is "pricing must never fail a run": failures degrade to `None` cost or a warning (`best_effort_price`, `pydantic_ai_slim/pydantic_ai/_cost.py:62-95`). Every model response appended to history gets its cost filled and its usage summed into `RunUsage` at one choke point (`_append_response`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1779-1795`). Per-run answers to "what did this run cost?" are available from `AgentRunResult.usage` / mid-stream `StreamedRunResult.usage` (`pydantic_ai_slim/pydantic_ai/run.py:487-489`, `pydantic_ai_slim/pydantic_ai/result.py:204-224`), enforced against optional limits including a USD `cost_limit` (`UsageLimits`, `pydantic_ai_slim/pydantic_ai/usage.py:417-472`), and exported as OpenTelemetry attributes and histograms (`operation.cost`, `gen_ai.client.token.usage`; `pydantic_ai_slim/pydantic_ai/_instrumentation.py:404-420`, `pydantic_ai_slim/pydantic_ai/models/instrumented.py:167-188`). The behavior is heavily tested, including streaming-cost regressions, tiered continuation pricing, and exact Decimal snapshots across providers.

The one clear gap: monetary cost of tool executions (including server-side/native tools) is not tracked — tools are *counted* (`RunUsage.tool_calls`) but not priced. Retry costs are accounted implicitly because each retry is a new billed model request flowing through the same accumulation path.

## Rating

**9 / 10.**

Rationale against the rubric:

- **Clear model with explicit interfaces**: typed `RequestUsage`/`RunUsage` with documented inclusion semantics ("token counts form inclusive parent/child buckets", `usage.py:93-99`); public re-pricing API `ModelResponse.cost()` (`pydantic_ai_slim/pydantic_ai/messages.py:2688-2700`); `None` deliberately distinguishes "unknown price" from zero (`usage.py:130-135`).
- **Operational safeguards**: `UsageLimits` enforces request/token/cost ceilings before each request, after each response, before tool-call batches, and at run completion (`usage.py:492-572`; enforcement points at `_agent_graph.py:1603-1621,1786-1794`, `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1838-1843`); unpriceable runs with a `cost_limit` emit `CostNotFoundWarning` instead of silently running unconstrained (`usage.py:528-535`).
- **Proven under failure**: graceful degradation paths are pinned by tests — unpriceable models stay silent (`tests/test_cost.py:130-136`), invalid usage stays silent (`tests/test_cost.py:139-154`), unexpected pricing errors warn instead of crashing (`tests/test_cost.py:157-167`), and streaming cost was fixed and regression-tested (`tests/test_cost.py:88-127`).
- **Observable**: OTel span attributes plus token/cost metric histograms recorded even when spans are dropped by sampling (`_instrumentation.py:520-522`), with an explicit double-counting guard between first-class attributes and `details.*` keys (`usage.py:21-27,244-252`).

Not a 10 because: tool-execution monetary cost is absent (counters only), pricing depends on the freshness of the vendored `genai-prices` snapshot, `cost_limit` is explicitly best-effort rather than a hard billing guarantee (documented caveat, `docs/agent.md:1009`), and there is no persisted billing artifact beyond the in-memory `RunUsage`.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counter types | `UsageBase` fields: `input_tokens`, `output_tokens`, `cache_write_tokens`, `cache_read_tokens`, audio tokens, free-form `details` dict | `pydantic_ai_slim/pydantic_ai/usage.py:88-128` |
| Inclusion semantics | Docstring: totals include cached and audio tokens; extraction normalizes providers (Anthropic/Bedrock) that report them separately | `pydantic_ai_slim/pydantic_ai/usage.py:93-99` |
| Cache efficiency metric | `cache_hit_ratio = cache_read_tokens / input_tokens` on both request and run usage | `pydantic_ai_slim/pydantic_ai/usage.py:203-216` |
| Token extraction from providers | `RequestUsage.extract` delegates to `genai-prices` provider registry with url → id → fallback chain | `pydantic_ai_slim/pydantic_ai/usage.py:303-334` |
| Provider adapters use extractor | OpenAI (+ custom cache-write mapping) | `pydantic_ai_slim/pydantic_ai/models/openai.py:5020-5028` |
| Provider adapters use extractor | Anthropic merges streaming delta usage + compaction details before extract | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:2752-2843` |
| Provider adapters use extractor | Google merges partial usages across stream chunks | `pydantic_ai_slim/pydantic_ai/models/google.py:2012-2071` |
| Run-level accumulator | `RunUsage.incr` sums requests, `tool_calls`, tokens, details, and cost | `pydantic_ai_slim/pydantic_ai/usage.py:371-390` |
| Cost addition helper | `_incr_usage_cost` adds non-None costs; numeric costs not double-added | `pydantic_ai_slim/pydantic_ai/usage.py:393-395` |
| Cost calculator | `calculate_price_for_usage` wraps `genai_prices.calc_price`, tries `provider_api_url` then `provider_id` | `pydantic_ai_slim/pydantic_ai/_cost.py:29-59` |
| Never-fail pricing contract | `best_effort_price`: `LookupError`/`ValueError` → `None`; other exceptions → `CostCalculationFailedWarning`, returns `None` | `pydantic_ai_slim/pydantic_ai/_cost.py:62-95` |
| Cost fill-on-append | `fill_response_cost` sets `response.usage.cost` only if unset (provider-reported cost could take precedence later) | `pydantic_ai_slim/pydantic_ai/_cost.py:98-118` |
| Pricing data preload | `preload_pricing_data()` called at Model construction to keep snapshot load off the event loop | `pydantic_ai_slim/pydantic_ai/_cost.py:21-26`, `pydantic_ai_slim/pydantic_ai/models/__init__.py:29` |
| Public re-pricing API | `ModelResponse.cost()` returns full `PriceCalculation` (not just total) | `pydantic_ai_slim/pydantic_ai/messages.py:2688-2700` |
| Per-response accumulation point | `_append_response`: fills cost, `ctx.state.usage.incr(response.usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1779-1795` |
| Streaming accumulation | Partial/interrupted streamed responses are priced (`fill_response_cost`) and incremented too | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1316-1329` |
| Continuation pricing | Continuation segments priced individually *before* merging so tiered pricing stays correct | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1021-1028` |
| Fallback rejected-candidate cost | Rejected FallbackModel candidates' costs accumulate into final usage | `pydantic_ai_slim/pydantic_ai/models/fallback.py:247,297-306` |
| Mid-stream live cost | `StreamedRunResult.usage` adds live best-effort price for the in-flight response | `pydantic_ai_slim/pydantic_ai/result.py:204-224` |
| Pre-request token counting | `count_tokens_before_request=True` calls `model.count_tokens` and prices counted input tokens (lower bound) against a copy of usage | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1602-1621`, flag doc `pydantic_ai_slim/pydantic_ai/usage.py:459-472` |
| Limits interface | `UsageLimits`: `cost_limit`, `request_limit=50`, `tool_calls_limit`, input/output/total token limits, per-request input limit | `pydantic_ai_slim/pydantic_ai/usage.py:417-472` |
| Pre-request check | `check_before_request` raises `UsageLimitExceeded` for requests/tokens/cost | `pydantic_ai_slim/pydantic_ai/usage.py:492-514` |
| Post-response checks | `check_tokens` + `check_cost(warn_if_cost_unavailable=False)` after each append | `pydantic_ai_slim/pydantic_ai/usage.py:516-551`, `_agent_graph.py:1787-1790` |
| Final-run enforcement | `_finalize_result` runs `usage_limits.check_cost(result.usage)` with warning enabled | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1838-1843` |
| Unenforceable-limit warning | `CostNotFoundWarning` when `cost_limit` set but cost is None | `pydantic_ai_slim/pydantic_ai/usage.py:528-535` |
| Error type carries usage | Cancelled runs expose `.usage` on the exception | `pydantic_ai_slim/pydantic_ai/exceptions.py:298-309,429-431` |
| Tool call counting | `usage.tool_calls += 1` only after successful execution (and for skipped calls) | `pydantic_ai_slim/pydantic_ai/tool_manager.py:984,1025` |
| Tool-call limit projection | Batch pre-check deep-copies usage and projects all pending function calls | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:444-448` |
| OTel span attributes | `opentelemetry_attributes()` emits `gen_ai.usage.*`; collision guard prevents double counting in consumers like Langfuse | `pydantic_ai_slim/pydantic_ai/usage.py:218-253`, guard rationale `usage.py:21-27` |
| Cost span attribute & metrics | `operation.cost` attribute; `gen_ai.client.token.usage` + `operation.cost` USD histograms; computed even if span unsampled; recorded after span close to avoid double count | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:404-420,520-522,543-544`, `pydantic_ai_slim/pydantic_ai/models/instrumented.py:167-188,305-329` |
| Embeddings cost | Embedding results expose `cost()`; instrumented embeddings record `operation.cost` | `pydantic_ai_slim/pydantic_ai/embeddings/result.py:103`, `pydantic_ai_slim/pydantic_ai/embeddings/instrumented.py:145-151,163` |
| Realtime sessions | Realtime session tracks usage and enforces token/cost/tool limits per turn | `pydantic_ai_slim/pydantic_ai/realtime/_session.py:2237-2295` |
| CLI summary | `/usage` slash command renders cumulative session tokens/requests/tool_calls as JSON or text | `pydantic_ai_slim/pydantic_ai/_cli/__init__.py:484-510` |
| Provider-reported costs preserved | OpenRouter upstream cost/cost_details stored under `provider_details`, not merged into USD total | `pydantic_ai_slim/pydantic_ai/models/openrouter.py:567-573` |
| Dedicated test file | Cost accumulation contract: matches response price (stream+non-stream), silent for unpriceable models, warns on unexpected failure, Decimal arithmetic | `tests/test_cost.py:88-205` |
| Limit tests | `CostNotFoundWarning` timing (only after run completes), default no-op cost_limit, exceeded messages | `tests/test_usage_limits.py:1325-1392` |
| Continuation cost tests | Multi-segment cost summed exactly once; tiered-pricing comment; cost-limit cancels suspended job; resume-seed check | `tests/models/test_streamed_continuation.py:403-482,1137-1165` |
| Exact Decimal pins | VCR-backed snapshots assert precise costs per provider (e.g. gpt-4o = 0.00075) | `tests/test_cost.py:127`, `tests/models/test_openai.py:1308` |
| Pre-request cost enforcement | `test_anthropic_count_tokens_enforces_cost_limit` proves counted-input cost trips `cost_limit` before sending | `tests/models/test_anthropic.py:11545-11562` |
| Realtime cost-limit test | Session raises `UsageLimitExceeded` on `cost_limit` | `tests/realtime/test_session.py:5791-5797` |
| Docs | Usage/cost retrieval documented; `cost_limit` example with honest best-effort caveats | `docs/agent.md:432-436,985-1009` |

## Answers to Dimension Questions

1. **Are tokens counted per run?** Yes. Every provider response's usage is extracted into `RequestUsage` and summed into a per-run `RunUsage` at the single append point (`_agent_graph.py:1786`), covering input/output/cache/audio tokens plus free-form `details` (`usage.py:88-128`). Streaming responses accumulate incrementally (`_agent_graph.py:1328`), continuations merge exactly once (`tests/models/test_streamed_continuation.py:403-426`), and interrupted streams still record their partial usage. Optionally, input tokens can be counted *before* the request via `model.count_tokens` (`_agent_graph.py:1603-1619`).

2. **Are costs attributed per model call?** Yes. Each `ModelResponse` gets a per-call `Decimal` cost filled from `genai-prices`, keyed on the response's own `model_name`, `provider_url`, `provider_name`, and timestamp (`_cost.py:98-118`), preserving per-call attribution even when multiple models appear in one run (`docs/multi-agent-applications.md:25`). `ModelResponse.cost()` allows independent re-pricing (`messages.py:2688-2700`).

3. **Are tool execution costs tracked?** Only as counts, not money. Successful tool executions increment `RunUsage.tool_calls` (`tool_manager.py:984,1025`) and `tool_calls_limit` bounds them, but there is no mechanism to price a local or MCP tool execution, nor to fold native/server-side tool charges (e.g., OpenRouter `cost_details`, stored separately in `provider_details`, `models/openrouter.py:567-573`) into `RunUsage.cost`. A code comment acknowledges a possible future hook for provider-level custom pricing (`_cost.py:89-91`).

4. **Are retry costs accounted for?** Yes, structurally. A retry (tool `ModelRetry` → `RetryPromptPart` new request, `_agent_graph.py:1797-1810`) is a normal subsequent model request, so its usage and cost flow through the same `_append_response` accumulation; nothing is deduplicated or exempted. Continuations (Anthropic `pause_turn`, background polls) are explicitly treated as "separately billed requests" and priced per segment to keep tiered pricing accurate (`_agent_graph.py:1021-1023`). There is no separate retry ledger, but `request_limit` defaults to 50 to cap runaway loops (`usage.py:429`). Edge case handled explicitly: a FallbackModel candidate that fails output validation still has its spent cost added to the run (`models/fallback.py:297-306`; test `tests/models/test_fallback.py:1452-1472`).

5. **Are per-run cost summaries available?** Yes. `result.usage` / `run.usage` return the cumulative `RunUsage` including `cost` (`run.py:487-489`; mid-stream live version `result.py:204-224`); cancelled runs expose usage on the exception (`exceptions.py:429-431`); the CLI renders session totals via `/usage` (`_cli/__init__.py:484-510`); OTel exports both per-span `operation.cost` attributes and aggregated token/cost histograms (`_instrumentation.py:404-420`; `models/instrumented.py:184-188`). Answering "what did this run cost?" is a single attribute read — well under a minute.

## Architectural Decisions

1. **Outsource the pricing table to a dedicated library.** `genai-prices` (same org) owns the provider registry, usage extraction, and price data, refreshed as a data snapshot (`usage.py:11`, `_cost.py:9-10`, dependency pin `pydantic_pyproject.toml` → `pydantic_ai_slim/pyproject.toml:69`). Pydantic AI keeps only orchestration: match order (URL → provider ID → fallback), degradation policy, and accumulation.

2. **Pricing must never fail a run.** `best_effort_price` converts known lookup/validation failures into `None` and unexpected ones into a warning (`_cost.py:70-95`), while `ModelResponse.cost()` — the user-facing API — propagates errors and asserts a model name exists (`messages.py:2693`). Internal plumbing and public APIs intentionally have different failure contracts.

3. **Single accumulation choke point.** All usage enters `ctx.state.usage` via `_append_response` regardless of streaming, retries, continuations, or capabilities (`_agent_graph.py:1779-1795`), which makes double-counting the primary correctness risk — addressed with explicit "commit exactly once" designs for continuations (`_agent_graph.py:875-893`) and tool-search splits (`_tool_search.py:427-438,499-501`).

4. **Unknown ≠ zero.** Cost is `Decimal | None`, and `None` survives serialization round-trips (`usage.py:130-135`), so budget enforcement can distinguish "free" from "unpriceable" and warn accordingly (`usage.py:528-535`).

5. **Limits as runtime guardrails, not accounting reports.** `UsageLimits` checks at five distinct lifecycle points (before request, after response, before tool batch, mid-continuation provisional, run completion), trading strictness for latency: mid-run cost checks suppress the "no price" warning until the run finishes since later responses may still price (`_agent_graph.py:1787-1790`, `tests/test_usage_limits.py:1352-1373`).

## Notable Patterns

- **Provider normalization at the boundary**: raw provider payloads that exclude cache tokens from `input_tokens` (Anthropic, Bedrock) are normalized so cross-provider comparisons hold (`usage.py:96-99`); Google stream chunks are folded into one `RequestUsage` with per-field fallbacks (`models/google.py:2066-2071`).
- **Tiered-pricing-aware merging**: continuation segments are priced *before* their token counts merge, because pricing merged 300k tokens at tier-1 rates would understate cost (`_agent_graph.py:1021-1023`; test comment `tests/models/test_streamed_continuation.py:405`).
- **Telemetry double-count guards**: OTel emission drops `details` keys colliding with first-class attributes because aggregators like Langfuse sum them (`usage.py:244-252`); metrics are recorded after span close so backends aggregating from spans don't double-count (`_instrumentation.py:457-459,543-544`).
- **Budget disclosure to tools**: `ctx.usage_limits` exposes the run's live limits to tools/capabilities so they can adapt to remaining budget without duplicate config (`docs/agent.md:958`, `docs/capabilities/on-demand.md:129`).
- **Durable-execution fidelity**: usage/usage_limits serialize across Temporal/DBOS/Prefect boundaries, and workflow sandbox imports are arranged so lazy `response.cost()` pricing works inside activities (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/__init__.py:138`, test `tests/test_temporal.py:2487`).

## Tradeoffs

- **Freshness vs. simplicity of pricing data**: costs come from the `genai-prices` snapshot bundled at install/run time; new models/providers are unpriceable until the library updates. The failure mode is benign (`None` cost + optional warning) but means `cost_limit` silently degrades for brand-new models.
- **Best-effort enforcement vs. hard guarantees**: docs explicitly warn not to treat `cost_limit` as a hard billing guarantee and to pair it with provider spend controls (`docs/agent.md:1009`). Checks happen post-response for the current call, so a single oversized request is already billed before the limit trips — mitigated but not eliminated by opt-in `count_tokens_before_request` (with its extra API call overhead, `usage.py:459-464`).
- **Count-based tool governance vs. monetary tool cost**: counting tool calls is universal and cheap; pricing them would require per-tool cost models the framework doesn't attempt. Users needing true all-in cost must add tool costs externally.
- **Accumulate-at-append simplicity vs. streaming liveness**: mid-stream `usage.cost` needs a special-case live estimate (`result.py:210-224`) — a small complexity tax paid to keep the common path simple.

## Failure Modes / Edge Cases

Handled explicitly:

- Stream consumed before usage exists: cost was historically read off an empty stream; fixed and regression-pinned for streaming (`tests/test_cost.py:88-127`).
- Inconsistent provider token breakdowns (e.g. negative implied uncached tokens): `ValueError` swallowed, cost stays `None`, no warning (`tests/test_cost.py:139-154`).
- Unexpected pricing-library crash: surfaced as `CostCalculationFailedWarning`, run completes (`tests/test_cost.py:157-167`).
- Unpriced run under a `cost_limit`: `CostNotFoundWarning` deferred until run completion so transient unpriced segments don't spam (`tests/test_usage_limits.py:1336-1373`).
- Suspended/background jobs: cost checked before resuming a suspended history seed, and the server-side job is cancelled when a limit trips mid-continuation so it doesn't keep billing (`_agent_graph.py:896-908`, `tests/models/test_streamed_continuation.py:434-459`).
- Serialization: legacy key aliases and arbitrary extra usage fields survive Pydantic round-trips (`usage.py:142-188`).

Residual risks:

- If `genai-prices` misprices a model, the wrong number flows everywhere silently (single source of truth cuts both ways).
- Pre-request counted-token pricing is a lower bound only (input side), acknowledged inline (`_agent_graph.py:1608-1609`).
- Tool-loop cost blowups bounded only by counts/tokens unless `cost_limit` happens to be enforceable.

## Future Considerations

- Pluggable provider-level cost hooks: the codebase itself sketches this ("allow some kind of hook on the provider level … maybe a parameter on `Provider` classes", `_cost.py:89-91`) — it would cover self-hosted/gateway pricing and BYOK discounts (OpenRouter already reports upstream cost that today lands only in `provider_details`).
- Folding native/server-side tool charges (web search, code execution) into `RunUsage.cost` where providers report them.
- Optional persistence/export of per-run cost summaries (currently in-memory only; the CLI shows session totals but nothing durable).
- Extending `count_tokens_before_request` support beyond Anthropic/Google/Bedrock/OpenAI-Responses (`usage.py:466-472`) for preemptive cost control.

## Questions / Gaps

- **What did a tool execution cost?** Not answerable from the framework; only `tool_calls` counts exist (`tool_manager.py:1025`). No evidence found of any per-tool pricing interface (searched `_tool_execution.py`, `tool_manager.py`, `common_tools/`, `mcp.py` for cost/price terms).
- **How fresh is pricing data, and can users pin/update it?** Determined only up to the `genai-prices>=0.1.0` dependency and its snapshot mechanism (`pydantic_ai_slim/pyproject.toml:69`, `_cost.py:21-26`); no override API found in this repository.
- **Is there a billing-grade audit trail per request?** Per-response cost lives on message objects and OTel spans, but no built-in ledger/export beyond that was found.

---

Generated by `20.01-token-and-cost-accounting` against `pydantic-ai`.
