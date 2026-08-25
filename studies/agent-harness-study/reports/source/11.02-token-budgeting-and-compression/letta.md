# Source Analysis: letta

## Dimension 11.02: Token Budgeting and Compression

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, Pydantic schemas, SQLAlchemy ORM, provider SDKs) |
| Analyzed | 2026-08-25 |

*Citation convention: file paths below are relative to the source root `studies/agent-harness-study/sources/letta/` (e.g. `letta/services/summarizer/compact.py:135` refers to `studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py` line 135).*

## Summary

Letta implements token budgeting as a layered "compaction" system rather than a single pre-flight gate. The active agent loop (`letta/agents/letta_agent_v3.py`) tracks a per-agent context token estimate derived from the provider's own usage report of the last LLM call, compares it against a per-model compaction trigger threshold (90% of the context window for GPT-5-family models, 100% otherwise), and triggers summarization either proactively after a step or reactively when the provider raises a mapped `ContextWindowExceededError`. Compaction itself supports four modes — `all`, `sliding_window`, `self_compact_all`, `self_compact_sliding_window` — configured per-agent via `CompactionSettings` (model handle, prompt, clip chars, sliding-window percentage), with cascading fallbacks between modes and post-compaction verification against the trigger threshold.

Token counting is abstracted behind a `TokenCounter` interface with four implementations selected by factory: provider-side counting APIs for Anthropic and Gemini, tiktoken for OpenAI-compatible models, and a fast bytes/4 approximation with a 1.3x safety margin as the default fallback. A separate on-demand `ContextWindowCalculator` produces an observable per-section token breakdown (system prompt, core memory, memory filesystem, tool rules, directories, summary memory, messages, tool definitions) exposed through a REST endpoint. Overflow protection extends below the conversation level: tool returns are truncated per-tool and dynamically capped at ~20% of the context window, summaries are clipped to a configurable character budget, and a byte-budget middle-truncation guards the summarizer request itself.

The system is well-instrumented (compaction stats, telemetry call type, Redis-cached counts) and defensively coded (protected approval messages, system-prompt overflow as a distinct error). Weaknesses: measurement is mostly reactive (post-response usage, not pre-call estimation), OpenAI models rely on approximate counting, faithfulness of summaries is enforced only by prompt instructions plus character clipping, and legacy budgeting paths in `letta/agent.py` are broken dead code.

## Rating

**7 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces**: a dedicated `TokenCounter` ABC with four strategies and a factory (`letta/services/context_window_calculator/token_counter.py:21-316`), a typed `CompactionSettings` config surface persisted per agent (`letta/services/summarizer/summarizer_config.py:48-89`; `letta/schemas/agent.py:96-97`), and a single compaction entry point `compact_messages` (`letta/services/summarizer/compact.py:135`).
- **Operational safeguards**: model-specific trigger thresholds (`letta/services/summarizer/thresholds.py:27-41`), reactive retry loop bounded by `max_summarizer_retries` (`letta/agents/letta_agent_v3.py:1093`, `1218-1251`), proactive post-step check (`letta/agents/letta_agent_v3.py:1438-1474`), post-compaction threshold verification with mode fallback (`letta/services/summarizer/compact.py:359-412`), protected pending-approval messages (`letta/services/summarizer/self_summarizer.py:265-289`), and a distinct `SystemPromptTokenExceededError` (`letta/errors.py:364`; `letta/services/summarizer/compact.py:398-407`).
- **Tests exist** for threshold logic (`tests/test_compaction_thresholds.py:23-53`), section extraction (`tests/test_context_window_calculator.py:11-266+`), and static-buffer eviction (`tests/test_static_buffer_summarize.py:41-133`), plus integration tests for counters (`tests/integration_test_token_counters.py:134-216`).
- **Why not higher**: no true pre-flight budget check before the first LLM call of a turn (estimate comes from prior response usage, `letta/adapters/simple_llm_request_adapter.py:113`); OpenAI-path counting is a heuristic; summary fidelity is not programmatically verified; legacy paths contain broken imports and unused knobs (documented under Failure Modes / Gaps).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counter abstraction | `TokenCounter` ABC defining `count_text_tokens`, `count_message_tokens`, `count_tool_tokens`, `convert_messages` | letta/services/context_window_calculator/token_counter.py:21-38 |
| Provider-side counting (Anthropic) | `AnthropicTokenCounter` calls Anthropic count-tokens API; all methods wrapped in `@async_redis_cache` (1h TTL, sha256 keys) | letta/services/context_window_calculator/token_counter.py:41-84 |
| Provider-side counting (Gemini) | `GeminiTokenCounter` using Google count-tokens API with same caching pattern | letta/services/context_window_calculator/token_counter.py:130-175 |
| tiktoken counting | `TiktokenCounter` delegates to `num_tokens_from_messages`, falls back to `cl100k_base` encoding when model unknown | letta/services/context_window_calculator/token_counter.py:178-265; letta/local_llm/utils.py:192-256 |
| Approximate counting | `ApproxTokenCounter`: JSON-serialize then `ceil(bytes/4)`; documented as the codex-cli approach | letta/services/context_window_calculator/token_counter.py:87-127 |
| Counter selection factory | `create_token_counter`: anthropic → Anthropic API counter, google_* → Gemini counter, everything else → ApproxTokenCounter | letta/services/context_window_calculator/token_counter.py:268-316 |
| Approximation safety margin | `APPROX_TOKEN_SAFETY_MARGIN = 1.3` applied to ApproxTokenCounter results ("underestimates by ~25-35% ... due to structural overhead") | letta/services/summarizer/summarizer_sliding_window.py:21-42, 91-93 |
| Provider count-tokens client | Anthropic client count mirrors thinking budget (16000) and beta flags so accounting matches real requests; subtracts 8-token baseline | letta/llm_api/anthropic_client.py:794-948 |
| Streaming fallback counters | Streaming interface keeps tiktoken-based `fallback_input_tokens`/`fallback_output_tokens` alongside provider usage | letta/interfaces/openai_streaming_interface.py:130-141 |
| Per-agent budget field | `LLMConfig.context_window: int` required; defaults when unset (gpt-5 → 272000, gpt-4.1 → 256000, gpt-4o/-mini → 128000, gpt-4 → 8192) | letta/schemas/llm_config.py:65, 181-190 |
| Global window catalog | `LLM_MAX_CONTEXT_WINDOW` mapping per model id (gpt-5 272k, kimi 262144, glm-4.7 180000, ...) | letta/constants.py:249-284 |
| Window bounds constants | `MIN_CONTEXT_WINDOW = 4096`, `DEFAULT_CONTEXT_WINDOW = 128000` | letta/constants.py:77-79 |
| Per-agent compaction settings | `CompactionSettings`: `model`, `model_settings`, `prompt`, `prompt_acknowledgement`, `clip_chars` (default 50000), `mode` (all/sliding_window/self_compact_all/self_compact_sliding_window), `sliding_window_percentage` | letta/services/summarizer/summarizer_config.py:48-89 |
| Compaction settings stored on agent | `compaction_settings: Optional[CompactionSettings]` on create/update/read agent schemas | letta/schemas/agent.py:96-97, 258-259, 476-477 |
| Global env-configurable settings | `SummarizerSettings` (env prefix `LETTA_SUMMARIZER_`): mode, message_buffer_limit=60, message_buffer_min=15, enable_summarization, max_summarizer_retries=3, memory_warning_threshold=0.75, partial_evict_summarizer_percentage=0.30 | letta/settings.py:74-111 |
| Model-specific trigger threshold | `get_compaction_trigger_threshold`: GPT-5 family at 90% of window (proactive, matching "codex harness"), all others at 100% | letta/services/summarizer/thresholds.py:27-41 |
| Trigger multiplier constant | `SUMMARIZATION_TRIGGER_MULTIPLIER = 0.9` "to avoid 'too many tokens in prompt' fallbacks" | letta/constants.py:81-83 |
| Reactive trigger: error mapping | Provider clients translate overflow errors into `ContextWindowExceededError` | letta/llm_api/openai_client.py:1302, 1324, 1394; letta/llm_api/anthropic_client.py:985, 1049, 1133; letta/llm_api/google_vertex_client.py:916; letta/llm_api/chatgpt_oauth_client.py:1172, 1222 |
| Reactive trigger: retry loop | On `ContextWindowExceededError`, up to `summarizer_settings.max_summarizer_retries` compaction attempts inside the step loop | letta/agents/letta_agent_v3.py:1093, 1218-1251 |
| Proactive trigger: post-step check | After each successful step, if `context_token_estimate > compaction_trigger_threshold`, compact with trigger `"post_step_context_check"` | letta/agents/letta_agent_v3.py:1438-1474 |
| Usage-derived estimate | `self.context_token_estimate = llm_adapter.usage.total_tokens` after each LLM request; adapters copy `usage.total_tokens` from the provider response | letta/agents/letta_agent_v3.py:1306-1307; letta/adapters/letta_llm_request_adapter.py:93; letta/adapters/simple_llm_request_adapter.py:113 |
| Manual trigger APIs | `POST /v1/agents/{agent_id}/summarize` and conversation `/compact` endpoints accepting optional `CompactionSettings` overrides | letta/server/rest_api/routers/v1/agents.py:2417-2504; letta/server/rest_api/routers/v1/conversations.py:926-1132 |
| Compaction dispatcher | `compact_messages` routes by mode with cascading fallbacks (self_compact_all → self_compact_sliding_window → all; sliding_window → all) | letta/services/summarizer/compact.py:189-346 |
| Post-compaction verification | Re-count with tools (`count_tokens_with_tools`); if still ≥ threshold, retry with `all` mode; raise `SystemPromptTokenExceededError` if system prompt alone exceeds window; log critical but don't brick agent otherwise | letta/services/summarizer/compact.py:350-412 |
| Sliding-window eviction search | Goal = `(1 - sliding_window_percentage) * context_window`; cutoff advances in 10% message-count increments until token goal met; cutoff must be assistant/approval message | letta/services/summarizer/summarizer_sliding_window.py:144-198 |
| Summary clipping | Summary truncated to `clip_chars` with suffix `"... [summary truncated to fit]"` | letta/services/summarizer/summarizer_sliding_window.py:227-229; letta/services/summarizer/self_summarizer.py:136-138; letta/services/summarizer/constants.py:3 |
| Self-compaction (Claude Code style) | Agent summarizes its own transcript with its own LLM/config for prefix-cache compatibility; dummy assistant message inserted to prevent continuation | letta/services/summarizer/self_summarizer.py:24-150 |
| Protected messages | Pending approval requests never evicted; assistant message sharing the same `step_id` is protected with them | letta/services/summarizer/self_summarizer.py:265-289; letta/services/summarizer/summarizer_sliding_window.py:133-137 |
| Summarizer payload fallback A/B | On summarizer-context overflow: (A) rebuild transcript with tool returns clamped to `TOOL_RETURN_TRUNCATION_CHARS=5000`; (B) middle-truncate transcript to ~60% of summarizer window in bytes | letta/services/summarizer/summarizer.py:562-647; TOOL_RETURN_TRUNCATION_CHARS at letta/constants.py:443 |
| Middle truncation | `middle_truncate_text`: byte-budget aware, keeps head 30%/tail 30%, inserts `"[TRUNCATED: dropped N middle chars due to context budget]"` marker | letta/services/summarizer/summarizer.py:387-433 |
| Tool return limits (per-tool) | Tools carry `return_char_limit`; execution manager appends `FUNCTION_RETURN_VALUE_TRUNCATED` note when exceeded; `validate_function_response` truncates with explicit warning text | letta/schemas/tool.py:51; letta/orm/tool.py:44; letta/services/tool_executor/tool_execution_manager.py:127-128; letta/utils.py:898-937; letta/constants.py:200-202 |
| Dynamic tool-return cap | `_compute_tool_return_truncation_chars`: 20% of context window × 4 chars/token, min 5000; JSON-aware clamping of client tool returns before persisting | letta/agents/letta_agent_v3.py:143-153, 1714-1746 |
| Message-level truncation helper | `truncate_tool_return(content, limit)` appending `"... [truncated N chars]"`, applied across OpenAI/Anthropic/Google conversions | letta/schemas/message.py:68-73, 1462-1473, 1656, 1737-1831 |
| Context-window observability | `ContextWindowCalculator.calculate_context_window` counts each section concurrently and returns `ContextWindowOverview` (system, core memory, memory filesystem, tool usage rules, directories, external memory summary, summary memory, messages, tool definitions) | letta/services/context_window_calculator/context_window_calculator.py:249-384 |
| Section extraction | XML-tag parsing of compiled system message into sections incl. git-enabled agents' bare file blocks | letta/services/context_window_calculator/context_window_calculator.py:166-211 |
| Exposed via API/service | `AgentManager.get_context_window` builds counter from agent LLM config and returns overview | letta/services/agent_manager.py:3558-3599 |
| Summary detection | Summary message recognized by sentinel phrase "The following is a summary of the previous " at index 1 | letta/services/context_window_calculator/context_window_calculator.py:213-247 |
| Summarizer model routing | Cheap per-provider defaults (claude-haiku-4-5, gpt-5-mini, gemini-2.5-flash); auto-mode agents routed to Haiku with zai/glm-5 fallback | letta/services/summarizer/summarizer_config.py:11-32; letta/services/summarizer/compact.py:65-131 |
| Provider overload fallback | Anthropic → Bedrock Opus 4.5; ZAI → Baseten GLM-5, only for non-BYOK | letta/services/summarizer/summarizer.py:714-738, 741-816 |
| Faithfulness-oriented prompts | Structured summary prompts require verbatim preservation of identifiers and "Lookup hints" for content that couldn't fit; word caps ALL=500 / SLIDING=300 | letta/prompts/summarizer_prompt.py:1-67 |
| Anti-drift ACK | `MESSAGE_SUMMARY_REQUEST_ACK` pre-seeded assistant turn forces summary-only output | letta/constants.py:427; letta/services/summarizer/summarizer.py:539-551 |
| Compaction stats telemetry | Stats dict (trigger, tokens before/after, window size, message counts) packed into summary message and surfaced in `LettaCompactionEvent` | letta/services/summarizer/compact.py:414-432; letta/system.py:207-239; letta/schemas/letta_message.py:411-418 |
| Threshold tests | Unit tests assert 90% for gpt-5.2 (272k → 244800) and 100% for gpt-4.1, plus `force_proactive` override | tests/test_compaction_thresholds.py:23-53 |
| Calculator tests | Extensive unit tests for tag/section extraction across standard, Letta Code, git-enabled, react agents | tests/test_context_window_calculator.py:11-266+ |
| Static buffer tests | Eviction behavior incl. user-boundary alignment and approval-message retention | tests/test_static_buffer_summarize.py:41-133 |
| File view windows | `per_file_view_window_char_limit` truncates file blocks rendered in the system prompt with a warning header | letta/utils.py:1385-1391; letta/orm/files_agents.py:96; FILE_IS_TRUNCATED_WARNING at letta/constants.py:440 |

## Answers to Dimension Questions

**1. Is token usage measured before calling the model?**
Not in the main loop. The v3 loop sets `context_token_estimate` from the *previous* response's provider-reported usage (`letta/agents/letta_agent_v3.py:1306-1307`; adapters at `letta/adapters/simple_llm_request_adapter.py:113`), so budget enforcement within a turn is primarily reactive: either the provider rejects the request (mapped to `ContextWindowExceededError`) or the post-step estimate crosses the threshold (`letta/agents/letta_agent_v3.py:1438`). Pre-call token counting does exist, but only *inside* compaction: the sliding-window cutoff search re-counts candidate retained buffers until they meet the token goal (`letta/services/summarizer/summarizer_sliding_window.py:163-191`), and `ContextWindowCalculator` provides an accurate on-demand breakdown including tool definitions (`letta/services/context_window_calculator/context_window_calculator.py:319-352`). The comment at `letta/agents/letta_agent_v3.py:125-129` confirms the estimate is "derived from per-step usage."

**2. What gets dropped when budget is exceeded?**
Ordering of sacrifice during compaction: (a) older history is evicted first into an LLM-generated summary injected at index 1 (`letta/services/summarizer/compact.py:462-465`); (b) the system message is never evicted (always `[system] + [summary] + tail`, `compact.py:462-465`); (c) pending approval requests — and the same-step assistant message preceding them — are protected from eviction (`letta/services/summarizer/self_summarizer.py:265-289`; `letta/services/summarizer/summarizer_sliding_window.py:133-137`); (d) eviction boundaries must fall on assistant/approval messages to avoid splitting tool-call groups (`is_valid_cutoff`, `summarizer_sliding_window.py:156-161`). Independently, oversized tool returns are truncated at ingestion time (per-tool limit + dynamic 20%-of-window cap, `letta/agents/letta_agent_v3.py:143-153`, `1714-1746`), and images are replaced with placeholders in transcripts (`format_transcript`, `letta/services/summarizer/summarizer.py:684-687`). If even the system prompt alone exceeds the window, a distinct `SystemPromptTokenExceededError` is raised instead of silently looping (`compact.py:398-407`).

**3. Is summarization faithful?**
By design intent, yes; by guarantee, no. The prompts demand structured sections and verbatim identifier preservation, plus explicit "lookup hints" pointing back to retrievable history for content that didn't fit (`letta/prompts/summarizer_prompt.py:4-45`), and an ACK turn suppresses conversational drift (`letta/constants.py:427`). However there is no programmatic validation of summary completeness: outputs are hard-clipped to `clip_chars` (50k chars default) mid-content with a suffix marker (`summarizer_sliding_window.py:227-229`), the transcript fed to the summarizer may be middle-truncated (dropping the middle 40%) under fallback B (`summarizer.py:603-619`), and tool returns in the transcript may be clamped to 5000 chars (`simple_formatter` param, `summarizer.py:346-359`). Evicted messages remain queryable in recall storage, which mitigates loss, but nothing verifies that required prompt sections actually appeared in the output.

**4. Is budget configurable?**
Yes, at three levels. (i) **Per-agent/per-model**: `LLMConfig.context_window` is a required per-agent field (`letta/schemas/llm_config.py:65`) with model-family defaults (`:181-190`) and a large per-model catalog (`letta/constants.py:249-284`); `CompactionSettings` (mode, summarizer model, prompt, `clip_chars`, `sliding_window_percentage`) is stored per agent (`letta/schemas/agent.py:96-97`) and overridable per-request via the summarize/compact APIs (`letta/server/rest_api/routers/v1/agents.py:2417-2504`). (ii) **Deployment-level env settings**: `SummarizerSettings` with prefix `LETTA_SUMMARIZER_` controls retries, buffer sizes, eviction percentage, warning threshold (`letta/settings.py:74-111`). (iii) **Implicit per-model policy**: the trigger threshold differs by model family (GPT-5 proactive 90%, else 100%, `thresholds.py:27-41`). Notably, some advertised knobs are wired to nothing: `desired_memory_token_pressure` and `keep_last_n_messages` have no usages outside their definitions (grep across `letta/` found zero call sites).

## Architectural Decisions

1. **Counters as pluggable strategy objects, chosen by provider.** Accuracy where it matters (Anthropic/Gemini native count APIs, cached), speed elsewhere (bytes/4). The factory explicitly documents the tradeoff and the ApproxTokenCounter cites codex-cli as precedent (`token_counter.py:87-95, 268-316`).
2. **Reactive-first with a proactive safety net.** Rather than estimating before every call, Letta lets the provider be the source of truth and recovers via a bounded retry loop (`letta_agent_v3.py:1218`), adding a post-step proactive check so the *next* turn rarely hits the hard failure (`:1438`).
3. **Compaction modes with graceful degradation chains.** `compact_messages` implements explicit try/fallback ladders between self-compaction, sliding-window, and summarize-all, plus a second verification pass against the trigger threshold with another mode fallback (`compact.py:189-346, 359-381`). Failure of compression degrades to a logged critical state rather than bricking the agent (`:409-410`).
4. **Separate summarizer model from agent model.** Summarization runs on cheap provider-specific defaults (Haiku/GPT-5-mini/Gemini Flash) while preserving cache compatibility when in self-compaction mode (`summarizer_config.py:11-32`; `compact.py:42-131`; `self_summarizer.py:99-118`).
5. **Budget awareness pushed down into content ingestion.** Tool returns are capped at write time (~20% of window) and per-tool char limits, so compaction rarely has to fight individual giant messages (`letta_agent_v3.py:143-153`).
6. **Observability as a first-class feature.** Per-section token breakdown via `ContextWindowOverview`, packed `compaction_stats` embedded in the summary message, and a `LettaCompactionEvent` stream type (`context_window_calculator.py:340-384`; `system.py:207-239`; `schemas/letta_message.py:405-418`).
7. **Model-specific trigger policies.** The 90% GPT-5 threshold encodes an operational lesson ("GPT-5 runs hitting max_output_tokens exceeded ... aligns GPT-5 behavior with the codex harness' proactive 90% compaction policy") and is unit-tested (`thresholds.py:33-41`; `tests/test_compaction_thresholds.py:23-31`).

## Notable Patterns

- **Cached token counting**: every provider-count call is memoized in Redis keyed by content hash with a 1-hour TTL (`token_counter.py:49-53, 60-65, 72-77`), making repeated compaction searches cheap.
- **Sentinel-string protocol for summaries**: both detection ("The following is a summary of the previous ", `context_window_calculator.py:235-242`) and truncation marking (`SUMMARY_TRUNCATION_SUFFIX`, `summarizer/constants.py:3`) rely on stable literal strings — simple, but fragile under prompt edits.
- **Structural eviction constraints**: cutoff indices must be assistant or approval-with-tool-calls messages, mirroring how Claude Code-style harnesses avoid splitting tool-use blocks (`summarizer_sliding_window.py:156-161`).
- **Byte-based budgets for cross-lingual safety**: middle truncation converts byte budgets to character slices using the actual bytes-per-char ratio to avoid splitting multi-byte characters (`summarizer.py:407-433`).
- **Prefix-cache-conscious self-compaction**: self mode deliberately matches parallel-tool-call flags and tool schemas from the normal request path "for cache compatibility" (`self_summarizer.py:100-118`; comment at `letta_agent_v3.py:2120`).
- **Legacy layering kept in-tree**: three generations of agent loops coexist (`letta/agent.py`, `agents/letta_agent.py`, `letta_agent_v2.py`, `letta_agent_v3.py`), with v2 explicitly logging "Running deprecated v2 summarizer" (`agents/letta_agent_v2.py:1365`).

## Tradeoffs

- **Speed vs accuracy**: the default ApproxTokenCounter (OpenAI et al.) can misjudge budgets; this is acknowledged in-code ("underestimates by ~25-35%") and patched with a blanket 1.3× margin (`summarizer_sliding_window.py:21-24`), which trades conservatism for possible premature compaction.
- **Reactivity vs latency**: waiting for provider errors means one failed round-trip before recovery in the worst case; the proactive check reduces but does not eliminate this.
- **Summary brevity vs information retention**: word-limited structured prompts (300–500 words) plus char clipping bound cost but guarantee lossy compression; "lookup hints" shift the burden of recovery onto later retrieval rather than the summary itself (`prompts/summarizer_prompt.py:22, 41, 63`).
- **Fallback depth vs complexity**: the multi-mode fallback ladder maximizes success probability but makes the actual compression path hard to predict; mitigated by recording `summarization_mode_used` in stats (`compact.py:189, 431`).
- **Message-count vs token-based eviction**: sliding-window percentage operates on message counts while goals are expressed in tokens, requiring iterative re-counting per increment (`summarizer_sliding_window.py:163-191`) — correct but potentially many counting calls for long histories (partially absorbed by caching).

## Failure Modes / Edge Cases

- **Broken legacy import path**: `letta/agent.py:35` imports `calculate_summarizer_cutoff`, `get_token_counts_for_messages`, `is_context_overflow_error` from `letta.llm_api.helpers`, but none are defined there (verified by AST scan of `helpers.py`; grep finds no definition anywhere). Importing `letta.agent` would raise ImportError; it is still imported by `letta/server/rest_api/routers/openai/chat_completions/chat_completions.py:8`. The legacy MemGPT-style token-budget loop (`agent.py:1020-1175`) is therefore dead code.
- **No assistant cutoff found**: if no valid cutoff exists, sliding-window raises and falls back to full summarization (`summarizer_sliding_window.py:193-194`); if the cutoff lands at the end of the buffer, summarization is skipped entirely (`:196-198`).
- **Unrecoverable compaction**: when post-fallback estimates still exceed the threshold, the code logs critical and proceeds anyway ("Log error but don't brick the agent", `compact.py:409-410`) — the next provider call will fail, deferring failure to the reactive path.
- **Approval-request edge**: evicting a pending approval would corrupt client state, hence the protections noted above; headless runs relax this by allowing approval-with-tool-calls messages as cutoffs (`summarizer_sliding_window.py:155-161`).
- **Empty/short histories**: self-summarization bails out with "No conversation to summarize." for <2 messages (`self_summarizer.py:47-49, 176-178`); manual compact endpoints return HTTP 400 when summarization fails to reduce message count (`agents.py:2496-2501`).
- **Dead configuration surface**: `desired_memory_token_pressure`, `keep_last_n_messages`, `evict_all_messages` (settings.py:91-111) have no consumers outside settings — configuring them has no effect.
- **v2 loop fragility acknowledged in-code**: "TODO: This can be broken by bad configs, e.g. lower bound too high, initial messages too fat" (`agents/letta_agent_v2.py:1367`).

## Future Considerations

- Restore or delete the legacy `letta/agent.py` budget path: either reimplement `calculate_summarizer_cutoff`/`get_token_counts_for_messages`/`is_context_overflow_error` or remove the module and its importer (`chat_completions.py:8`) to eliminate a latent ImportError.
- Add pre-flight estimation for the first LLM call of a turn using `ContextWindowCalculator` (or cheap ApproxTokenCounter) so compaction can start before a provider rejection, especially valuable for OpenAI models currently on the bytes/4 counter.
- Wire up or remove inert `SummarizerSettings` fields (`desired_memory_token_pressure`, `keep_last_n_messages`) to make the config surface honest.
- Programmatic faithfulness checks: validate that generated summaries contain the mandated sections and did not hit `clip_chars` clipping (surface clipping rate in `compaction_stats`); consider retry-on-truncation instead of silent suffix-marking.
- Replace string-sentinel summary detection (`context_window_calculator.py:241`) with the already-existing `role="summary"` message type (`use_summary_role`, `compact.py:434-442`) everywhere, retiring the legacy phrase match.
- Extend unit coverage from thresholds/section-extraction to the fallback ladder in `compact_messages` (currently only integration-tested, `tests/integration_test_summarizer.py`) and to `middle_truncate_text` boundary math.

## Questions / Gaps

- No evidence found of ranking *within* the message tail beyond structural constraints: priority is expressed as keep-ordering (system > summary > recent > old) rather than scored selection; no relevance-based dropping exists. Search covered `services/summarizer/*`, `agents/*`, and grep for `priority`/`rank` patterns.
- No evidence found of per-section *drop* decisions driven by the `ContextWindowOverview` numbers (it is observability-only in the compaction path); memory-block editing is handled by separate sleeptime/memory tools, outside this dimension.
- Whether the tiktoken `cl200k`-style fallback counters in `openai_streaming_interface.py:134-136` feed the compaction estimate or only telemetry was not traced end-to-end; the compaction estimate provably comes from adapter usage (`adapters/*.py:93/113`), so these appear diagnostic-only (stated as inference based on data flow, not a direct test).
- The exact set of provider error strings mapped to `ContextWindowExceededError` per client was not exhaustively enumerated (spot-checked openai/anthropic/google/chatgpt_oauth clients at the lines cited above).

---

Generated by `Dimension 11.02: Token Budgeting and Compression` against `letta`.
