# Source Analysis: openai-agents-sdk

## 20.01 Token and Cost Accounting

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (pydantic, openai SDK, asyncio; optional LiteLLM extension) |
| Analyzed | 2026-08-24 |

## Summary

The OpenAI Agents Python SDK has a first-class, centralized token-accounting model. A dedicated public module (`src/agents/usage.py`) defines the `Usage` aggregate — requests, input/output/total tokens, cached/cache-write/reasoning details, and a per-request `request_usage_entries` breakdown — that is mutated in place on every model call and surfaced on `RunContextWrapper.usage`, which every run result exposes (`result.context_wrapper.usage`). Every first-party model adapter (Responses API, Chat Completions non-streaming and streaming, Realtime) plus the LiteLLM extension extracts provider usage into this normalized shape, counts requests even when providers omit usage payloads, and records usage onto tracing spans (generation/response spans, plus opt-in task/turn spans computed via snapshot deltas). Retry cost accounting is explicit: failed retry attempts are folded into `requests` as zero-token per-request entries so attempt counts survive without inventing tokens.

What is absent is monetary cost computation: there are no price tables, no currency fields, and no dollar figures anywhere in the source tree — the only "cost" references are docstrings stating that `request_usage_entries` exists to *enable* downstream cost calculation, and a test simulating an Anthropic tiered-pricing scenario. Tool execution costs are also not tracked as a separate ledger: local function tools have no cost hooks at all, hosted-tool token consumption lands only in response-level aggregates, and the sole exceptions (Codex tool, `Agent.as_tool`) fold their model usage back into the shared run context rather than attributing it per tool.

Answering "what did this run cost?" in under a minute: token totals yes, dollars no — the caller must multiply per-request entries against their own price table.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with tests and explicit interfaces (7–8 band):** a purpose-built public module (`src/agents/usage.py:196-229`), exported from the package root (`src/agents/__init__.py:261`, `src/agents/__init__.py:563`), documented for users (`docs/usage.md:1-130`), and covered by a dedicated test file (`tests/test_usage.py`, 668 lines, ~25 named tests) plus retry/streaming/model-adapter test coverage.
- **Operational safeguards:** None-field normalization for providers that skip detail fields (`src/agents/usage.py:231-255`), JSON-safe raw-usage snapshots that can never fail a model call (`src/agents/usage.py:20-56`), request counting when providers omit usage (`src/agents/usage.py:315-343`), and checkpoint isolation via deep-copied usage so resumed runs do not double-count (`src/agents/run_context.py:117-131`).
- **Proven under failure:** retry-attempt accounting is implemented on both streaming and non-streaming paths and tested (`src/agents/run_internal/model_retry.py:338-350`; `tests/models/test_model_retry.py:514,881,932`; `tests/test_agent_runner_streamed.py:405,449`).
- **Not 9–10 because:** monetary cost calculation and per-tool cost attribution are absent by design; third-party adapter accuracy depends on provider behavior and sometimes manual `ModelSettings(include_usage=True)` (`docs/usage.md:35-42`), and LiteLLM neither warns-free (`src/agents/extensions/models/litellm_model.py:302-303`) nor supports raw-usage preservation (`docs/usage.md:76`).

## Evidence Collected

Every entry cites workspace-relative paths inside the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counters | `Usage` dataclass: `requests`, `input_tokens`, `output_tokens`, `total_tokens`, detail objects | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/usage.py:196-229 |
| Per-request breakdown | `RequestUsage` dataclass + auto-populated `request_usage_entries` preserving [100K, 150K, 80K]-style splits | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/usage.py:150-167, 295-312 |
| Aggregation primitive | `Usage.add()` sums all fields incl. cached/cache-write/reasoning details | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/usage.py:257-312 |
| Public accessor | `RunContextWrapper.usage` ("usage of the agent run so far", stale until last stream chunk) | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_context.py:83-86 |
| Run aggregation point (streaming) | `context_wrapper.usage.add(final_response.usage)` after terminal stream event | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/run_loop.py:2306 |
| Run aggregation point (non-streaming) | same fold after each turn's response | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/run_loop.py:2636 |
| Responses API extraction | `_usage_from_response` converts provider usage; falls back to request-only count when usage omitted | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/models/openai_responses.py:247-251 |
| Chat Completions extraction | maps `prompt_tokens`/`completion_tokens` (+details) or counts `Usage(requests=1)` without payload | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/models/openai_chatcompletions.py:288-301 |
| Streaming Chat Completions | builds Responses-shaped `ResponseUsage` from final streamed chunk; marks completed-without-usage otherwise | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/models/chatcmpl_stream_handler.py:1337-1344, 1352-1373 |
| LiteLLM extension | normalizes usage; logs `"No usage information returned from Litellm"` warning when missing | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/extensions/models/litellm_model.py:278-303 |
| Raw usage preservation | opt-in `ModelSettings.preserve_raw_usage` → `ModelResponse.raw_usage` detached JSON snapshot | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/model_settings.py:205; src/agents/items.py:720-730; src/agents/usage.py:20-45 |
| Request counted without usage | `_mark_request_completed_without_usage` / `_requests_for_response_without_usage` protocol between adapters and runner | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/usage.py:315-343; src/agents/models/openai_responses.py:241-244 |
| Retry cost accounting | `apply_retry_attempt_usage`: adds failed attempts to `requests`, prepends zero-token entries | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/model_retry.py:338-350 |
| Retry accounting applied (non-streaming) | after success: folds `failed_policy_attempts + compatibility_retries_taken` into response usage | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/model_retry.py:613-617 |
| Retry accounting applied (streaming) | comment mandates folding retries even when terminal response omits usage, matching non-streaming path | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/run_loop.py:2274-2287 |
| Conversation-locked compat retries counted | compatibility retry counter increments and feeds the same usage augmentation | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/model_retry.py:628-640 |
| Per-run summary surface | `RunResultBase.context_wrapper` exposed on both `RunResult` and `RunResultStreaming` | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/result.py:337, 481-482, 594-595 |
| Task/turn span usage | snapshot before span, delta attached at finish (`snapshot_usage`/`usage_delta`/`attach_usage_to_span`) | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/agent_runner_helpers.py:76-160; src/agents/run_internal/run_loop.py:1736-1784; src/agents/run.py:791, 957-959, 1615, 1754-1756, 2191-2193 |
| Task/turn spans default on | `include_task_and_turn_spans` defaults True in trace config | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tracing/config.py:12-18 |
| Span schemas carry usage | Generation/Response/Task/Turn span data all have a `usage` field serialized to export | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tracing/span_data.py:64-126, 169-241 |
| Model-call span usage | generation span usage set inside adapters before returning `ModelResponse` | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/models/openai_responses.py:600-602, 685-689; src/agents/models/openai_chatcompletions.py:306-313 |
| Backend sanitization | OpenAI tracing exporter coerces usage to finite numbers / whitelisted keys before upload | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/tracing/processors.py:50-51, 282-312, 448-483 |
| Serialization round-trip | `serialize_usage` / `deserialize_usage` incl. request entries (used by session extensions/state) | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/usage.py:111-147, 405-431 |
| Checkpoint isolation | `_copy_for_run_state` deep-copies usage so one checkpoint's tokens don't land on others | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_context.py:117-131 |
| Compaction usage folded in | `responses.compact` usage added to the live run wrapper | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/memory/openai_responses_compaction_session.py:240-242 |
| Realtime usage events | realtime session adds per-event `RealtimeModelUsageEvent` usage to context | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/realtime/session.py:602-604 |
| Nested agent-as-tool shares usage | nested `ToolContext` receives parent's `usage` object → outer-run totals include nested model calls | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/agent.py:718-731 (esp. line 722); resume rebind at src/agents/agent.py:880-884 |
| Codex tool usage folding | experimental Codex tool adds its internal model usage to `ctx.usage` and returns it on the result | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/extensions/experimental/codex/codex_tool.py:391-401 |
| Session-level persistence | extension stores per-turn usage keyed by anchored turn id; aggregates session totals on demand | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/extensions/memory/advanced_sqlite_session.py:618-654 |
| Dollar-cost calculator | **No evidence found.** Searched `cost|Cost|price|Price` across `src/` — only docstring mentions of enabling cost calc | studies/agent-harness-study/sources/openai-agents-sdk/src/agents/usage.py:219, 228 |
| Cost feasibility test | `test_anthropic_cost_calculation_scenario` demonstrates tiered pricing math over request entries | studies/agent-harness-study/sources/openai-agents-sdk/tests/test_usage.py:427-488 |
| User docs | dedicated Usage page: what's tracked, access patterns, sessions, checkpoints, hooks, adapter caveats | studies/agent-harness-study/sources/openai-agents-sdk/docs/usage.md:1-130 |
| Worked example | `examples/basic/usage_tracking.py` prints totals + per-request entries | studies/agent-harness-study/sources/openai-agents-sdk/examples/basic/usage_tracking.py:25-32 |

## Answers to Dimension Questions

**1. Are tokens counted per run?**
Yes, comprehensively. A fresh `Usage()` lives on `RunContextWrapper` (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_context.py:83`) and every model response in the loop is folded in (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_internal/run_loop.py:2306, 2636`), including tool-driven turns, handoffs, guardrail model calls made through the runner, compaction calls (`.../src/agents/memory/openai_responses_compaction_session.py:240-242`), and nested agent-as-tool runs which deliberately share the parent usage object (`.../src/agents/agent.py:722`). Beyond totals, the SDK preserves a per-request ledger (`request_usage_entries`, `.../src/agents/usage.py:218-229`) and cached/cache-write/reasoning sub-counts. Sessions keep runs independent while history replay inflates later input tokens, which is documented (`.../docs/usage.md:78-92`).

**2. Are costs attributed per model call?**
Tokens, yes; currency, no. Every model call produces a `ModelResponse.usage` normalized from the provider payload (`.../src/agents/items.py:706-711`), and adapters attach per-call usage to generation/response tracing spans (`.../src/agents/models/openai_responses.py:600-602`). There is no pricing table, model-rate lookup, or dollar field anywhere in the source; the only cost references are docstrings positioning `request_usage_entries` as input "for accurate per-request cost calculation" (`.../src/agents/usage.py:219-228`) and a test sketching how Anthropic tiered pricing would be computed from those entries (`.../tests/test_usage.py:427-488`). An opt-in `preserve_raw_usage` setting keeps provider-specific fields so callers can compute exact costs including field-presence semantics (`.../docs/usage.md:55-76`).

**3. Are tool execution costs tracked?**
Not as a distinct ledger. Local function tools have zero cost instrumentation — searching `usage` in `run_internal/tool_execution.py` and `run_internal/tool_caller.py` returns nothing. Hosted tools (web search, code interpreter, computer use) consume tokens billed server-side, but the SDK only sees them inside the response-level usage aggregate with no per-tool attribution. Two partial exceptions exist: the experimental Codex tool explicitly adds its internal model usage into the calling context and exposes it on its result object (`.../src/agents/extensions/experimental/codex/codex_tool.py:391-401`), and `Agent.as_tool` nested runs share the parent usage instance so their model spend is included in outer-run totals (`.../src/agents/agent.py:720-730`) — but both attribute to the run, not to the tool.

**4. Are retry costs accounted for?**
Yes — this is unusually thorough. `apply_retry_attempt_usage(...)` increments `requests` by the number of failed policy/compatibility attempts and prepends zero-token `RequestUsage` entries ahead of the successful one, so attempt counts are visible without fabricating token numbers (`.../src/agents/run_internal/model_retry.py:338-350`). It is applied on the non-streaming success path (`.../model_retry.py:613-617`), mirrored in the streaming path with a comment requiring parity even when terminal responses omit usage (`.../src/agents/run_internal/run_loop.py:2274-2287`), and covers legacy conversation-locked retries (`.../model_retry.py:628-640`). Tests verify request-count inflation and entry preservation, including zero-token successes and streaming-after-retry cases (`.../tests/models/test_model_retry.py:514-555, 881-928, 932-972`; `.../tests/test_agent_runner_streamed.py:405, 449, 532`). Caveat: hidden provider-managed retries (the OpenAI client's own internal retries) are not separately itemized — the SDK toggles them via `provider_managed_retries_disabled` but does not observe them once they happen inside the transport layer.

**5. Are per-run cost summaries available?**
Yes for tokens, immediately after (or during) a run: `result.context_wrapper.usage` (`.../src/agents/result.py:337`; `.../docs/usage.md:17-29`), live values inside lifecycle hooks (`.../examples/basic/lifecycle_example.py:45-106`), and usage-bearing tracing spans — generation/response spans per model call plus task and turn spans whose usage is computed as snapshot deltas (`.../src/agents/run_internal/agent_runner_helpers.py:76-160`; enabled by default per `.../src/agents/tracing/config.py:12-18`). The exporter sanitizes usage payloads for the hosted tracing backend (`.../src/agents/tracing/processors.py:448-483`). For serialization/persistence, `serialize_usage`/`deserialize_usage` round-trip everything including request entries (`.../src/agents/usage.py:111-147, 405-431`), used e.g. by the advanced SQLite session's per-turn usage storage (`.../src/agents/extensions/memory/advanced_sqlite_session.py:618-654`). Dollar summaries remain the caller's job.

## Architectural Decisions

1. **Single mutable aggregate on the run context.** One `Usage` instance owned by `RunContextWrapper` is mutated via `add()` throughout the run rather than collecting events for later reduction (`studies/agent-harness-study/sources/openai-agents-sdk/src/agents/run_context.py:83`; `.../src/agents/run_internal/run_loop.py:2306`). Consequence: any holder of the wrapper always sees current totals (hooks, tools), but streaming consumers see stale numbers until the terminal chunk (`.../run_context.py:84-86`).

2. **Normalized shape + opt-in raw preservation.** All providers are coerced into the Responses-API-style `Usage` with None-normalization guards (`.../src/agents/usage.py:231-255`), while `preserve_raw_usage` captures a detached, JSON-safe provider snapshot before normalization (`.../src/agents/usage.py:20-45`; `.../src/agents/items.py:723-730`). This trades lossless fidelity (off by default) for cross-provider consistency (on by default).

3. **Per-request entry list as the cost-calculation substrate.** Instead of computing dollars, the SDK preserves exactly the granularity needed to compute them later — per-request input/output plus cache-read/write split — explicitly motivated for tiered pricing and context-window monitoring (`.../src/agents/usage.py:218-229`; tested at `.../tests/test_usage.py:427-488`). Cost policy is deliberately left outside the library boundary.

4. **Retries accounted as request counts, not invented tokens.** Failed attempts become zero-token `RequestUsage` rows (`.../src/agents/run_internal/model_retry.py:338-350`), keeping `sum(entries) == top-level tokens` invariant intact while still answering "how many billable attempts happened".

5. **Usage flows through tracing as derived deltas.** Rather than duplicating counters, task/turn spans snapshot the context usage at start and attach `end − start` at finish (`.../src/agents/run_internal/agent_runner_helpers.py:76-160`), guaranteeing span usage reconciles with the run total.

6. **Deep-copy isolation at checkpoint boundaries.** `to_state()` copies usage so multiple resumed branches don't cross-contaminate totals (`.../src/agents/run_context.py:117-131`; documented `.../docs/usage.md:94-110`), with the deliberate exception that nested agent-as-tool resumes re-aggregate into the active outer run (`.../src/agents/agent.py:880-884`).

## Notable Patterns

- **Adapter contract for missing usage:** adapters never synthesize fake zeros; they mark responses with a private attribute declaring how many physical requests completed without usage, and the runner converts that into `Usage(requests=N)` (`.../src/agents/usage.py:315-343`; `.../src/agents/models/chatcmpl_stream_handler.py:1340-1344`). This distinguishes "provider said nothing" from "provider reported zero".
- **Defensive normalization everywhere:** `__post_init__` and `add()` tolerate `None` details injected by `model_construct` bypasses and older/newer OpenAI SDK versions (`.../src/agents/usage.py:231-255, 270-284`); version-compat helpers cover `cache_write_tokens` across OpenAI Python 2.44 vs 2.45+ (`.../usage.py:71-92`).
- **Failure-proof diagnostics:** raw-usage snapshotting swallows all exceptions so a non-JSON provider value can never fail a successful model call (`.../src/agents/usage.py:42-56`).
- **Span usage placement matters:** chat-completions sets span usage *before* validation branches so truncated/empty completions still record their tokens (`.../src/agents/models/openai_chatcompletions.py:303-313`); streaming responses record span usage before yielding the terminal event since consumers may close immediately (`.../src/agents/models/openai_responses.py:686-689`).
- **Docs-as-contract:** `docs/usage.md` documents subtle behaviors (session independence, checkpoint isolation, nested-run exception, adapter caveats) that match implementation specifics cited above (`.../docs/usage.md:78-110`).

## Tradeoffs

- **Consistency vs fidelity:** normalized `Usage` makes cross-provider comparison trivial but discards provider-specific fields unless `preserve_raw_usage=True`; LiteLLM doesn't support raw preservation at all (`.../docs/usage.md:76`), and some backends need `ModelSettings(include_usage=True)` to emit usage chunks (`.../docs/usage.md:39-41`) — i.e., third-party usage accuracy is conditional, not guaranteed.
- **In-place mutation vs auditability:** the single mutable aggregate is efficient and hook-friendly but means historical snapshots must be taken manually (`snapshot_usage` exists internally but isn't part of the public API).
- **Zero-token retry entries keep invariants but not bytes-on-the-wire:** you learn a failed attempt occurred but not whether it consumed any pre-failure tokens (e.g., a stream that died mid-generation contributes nothing token-wise).
- **Nested attribution by sharing, not metering:** agent-as-tool usage lands in the parent total via a shared object (`.../src/agents/agent.py:722`), which keeps global totals correct but prevents per-sub-agent breakdown without wrapping agents yourself.
- **Tracing coupling:** usage visibility in dashboards depends on the tracing pipeline being enabled/sanitizable (`.../src/agents/tracing/processors.py:282-312` drops non-finite/non-conforming usage rather than exporting it).

## Failure Modes / Edge Cases

- **Provider omits usage:** handled — request counted, tokens stay zero, raw usage stays absent (`.../src/agents/models/chatcmpl_stream_handler.py:1340-1344`); LiteLLM additionally warns (`.../src/agents/extensions/models/litellm_model.py:302-303`).
- **Providers leaving detail fields None:** normalized to 0 to prevent TypeErrors (`.../src/agents/usage.py:231-255`).
- **Streaming staleness:** reading `context_wrapper.usage` mid-stream yields stale values until the last chunk (`.../src/agents/run_context.py:84-86`).
- **Checkpoint double-count risk:** avoided by deepcopy on `to_state()` (`.../src/agents/run_context.py:120-124`); conversely, resuming the *same* state twice yields two runs each claiming the pre-checkpoint tokens — totals across sibling resumes are intentionally non-additive to the original result.
- **Conversation-locked retries:** up to 3 hidden compatibility retries are folded into usage counts (`.../src/agents/run_internal/model_retry.py:50, 628-640`); users wanting strict attempt control must set `max_retries=0`.
- **Non-finite/garbage usage values:** dropped at export time by sanitizer checks (`.../src/agents/tracing/processors.py:451-454`), so dashboards may silently under-report if an adapter misbehaves.
- **Compaction timing:** usage from auto-compaction joins the run's totals only when triggered inside the run; manual `run_compaction()` outside a run updates nothing retroactively (`.../docs/usage.md:33`).

## Future Considerations

- A pluggable price-table/cost-calculator (per-model rates, cache-read/write tiers, batch discounts) consuming `request_usage_entries` would close the gap the data structure was explicitly designed for (`.../src/agents/usage.py:219-228`); the Anthropic scenario test sketches the interface (`.../tests/test_usage.py:427-488`).
- Per-tool usage attribution: recording which model turns followed which tool calls (or extending the Codex pattern of returning usage per tool result, `.../codex_tool.py:401`) would enable per-tool cost rollups.
- Observability of provider-managed retries (currently toggled but not itemized) would complete end-to-end attempt accounting.
- Promoting `snapshot_usage`-style deltas to a public API would let applications build their own per-segment cost reports without touching internals (`.../src/agents/run_internal/agent_runner_helpers.py:76-97`).

## Questions / Gaps

- **What did this run cost in currency?** Not answerable from the SDK alone; requires an external price table multiplied over `request_usage_entries` (with cache-tier handling). No evidence of planned support found in the source tree.
- **Do hosted tools (web_search, code interpreter, file_search) report their own call-based billing anywhere?** No — searched adapters and tool-execution modules; their charges are only implicit in response-level token usage (and OpenAI-side per-tool fees the SDK never sees).
- **Does voice pipeline track STT/TTS audio-minute cost?** No evidence found — `grep -rn "usage"` across `src/agents/voice/` returns only an unrelated comment about client reuse (`.../src/agents/voice/models/openai_model_provider.py:23`).
- **Are guardrail/sub-LLM calls made outside the runner loop metered?** Guardrail LLM calls invoked through the standard flow are aggregated like any turn, but custom user-side model calls bypassing the SDK are invisible by construction (no evidence of interception).

---

Generated by dimension 20.01 (Token and Cost Accounting) against `openai-agents-sdk`.
