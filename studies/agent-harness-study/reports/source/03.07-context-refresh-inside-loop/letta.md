# Source Analysis: letta

## 03.07 Context Refresh Inside the Loop

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3 / async asyncio / Pydantic v2 / FastAPI / SQLAlchemy / Postgres / Redis |
| Analyzed | 2026-07-15 |

## Summary

Letta (formerly MemGPT) is a memory-augmented agent framework whose entire architecture exists to make **conversation context refresh durable and observable**. The loop is a stateful per-turn assembly pipeline built around `LettaAgentV3` (current) and `LettaAgentV2` (legacy) in `letta/agents/letta_agent_v{2,3}.py`. Each public turn (`step` / `stream` / `build_request`) starts by re-reading the current "in-context" message set from persisted IDs, then may compact, then dispatches to the LLM.

Context refresh is implemented as a **typed pipeline with four independent layers**, not a single monolithic "truncate when too long" step:

1. **Per-turn assembly from durable IDs** — `_prepare_in_context_messages_no_persist_async` loads `agent_state.message_ids` (or `conversation_messages` when conversation-mode is active) from the DB every turn, so the loop never relies on a stale in-memory message list (`letta/agents/helpers.py:148-317`; `letta/agents/letta_agent_v3.py:274-281`). A `message_buffer_autoclear` flag toggles the agent into "stateless" mode where only the system message is reloaded (`letta/agents/helpers.py:187-218`; `letta/schemas/agent.py:138-142`).
2. **Per-turn scrub + selective system-prompt rebuild** — `_refresh_messages` always scrubs inner thoughts when reasoning is disabled, and only rebuilds the system prompt when memory/tool-rules/directories changed (preserving Anthropic prompt-cache `cache_control` prefix). `force_system_prompt_refresh=True` is used after compaction (`letta/agents/letta_agent_v2.py:759-792`; called at `letta/agents/letta_agent_v3.py:969, 1262, 1485`).
3. **Two distinct compaction triggers** — proactive (LLM usage after each step > threshold, currently `0.9 * context_window` = 90% of the window, see `letta/services/summarizer/thresholds.py:27-41` and `letta/constants.py:82-83`) and reactive (`ContextWindowExceededError` after a failed LLM call, retried up to 3 times — `letta/settings.py:96`). Both flows funnel through `compact()` → `compact_messages()` (`letta/agents/letta_agent_v3.py:1241-1273, 1438-1505`; `letta/agents/letta_agent_v3.py:2077-2134`; `letta/services/summarizer/compact.py:134-472`).
4. **Pluggable summarization strategy with fallback chain** — `CompactionSettings.mode` chooses between `sliding_window` (default, recommended), `all`, `self_compact_all`, `self_compact_sliding_window` (`letta/services/summarizer/summarizer_config.py:48-89`). Each mode has its own fallback if the LLM call fails (sliding-window → all → exception; self_compact → sliding_window → all) and the **whole call has fallbacks**: clamp tool returns, hard-truncate transcript with a marker, propagate (`letta/services/summarizer/summarizer.py:562-647`). System-prompt overflow is detected explicitly and surfaced as a distinct `SystemPromptTokenExceededError` so the loop exits cleanly instead of looping endlessly (`letta/services/summarizer/compact.py:391-411`; `letta/agents/letta_agent_v3.py:1285-1290, 1506-1509`).
5. **Token budgeting has three layers** — (a) provider-specific `TokenCounter` subclasses for Anthropic / Gemini / Tiktoken / a 4-bytes-per-token approximation with a 1.3× safety margin (`letta/services/context_window_calculator/token_counter.py:21-125, 268-316`); (b) tool-return char truncation computed as `0.2 × context_window × 4` with a 5000-char floor (`letta/agents/letta_agent_v3.py:143-153`) and applied in `Message.to_*_dicts` (`letta/schemas/message.py:68-73, 1464-1535`); (c) `ContextWindowCalculator.extract_system_components` for parsing and reasoning over the system prompt text (`letta/services/context_window_calculator/context_window_calculator.py:19-200+`).
6. **Per-event observability** — every compaction emits an `EventMessage(message_type="event_message", event_type="compaction", event_data={trigger, context_token_estimate, context_window})` to the client BEFORE compacting, plus a `SummaryMessage` containing `CompactionStats{trigger, context_tokens_before, context_tokens_after, messages_count_before, messages_count_after, context_window}` (triggers are `context_window_exceeded` or `post_step_context_check`) (`letta/agents/letta_agent_v3.py:818-846`; `letta/schemas/letta_message.py:406-449`). The compacted `messages` itself is persisted atomically with `compaction_stats` packed into the JSON payload (`letta/system.py:207-236`; `letta/schemas/letta_message.py:423-439`).
7. **Stale-context removal** — compaction **only inserts a `summary` message at index 1 and trims index 1..cutoff** — original messages are NOT deleted (`letta/services/summarizer/compact.py:462-465`; `letta/services/summarizer/summarizer.py:241-242`). Removal happens via `update_message_ids_async` writing the new ID list back to `agent.message_ids` (or `ConversationManager.update_in_context_messages` in conversation mode) and setting `in_context=False` on dropped rows (`letta/services/agent_manager.py:999-1018`; `letta/services/conversation_manager.py:752-797`).
8. **Tool-result preservation vs summarization** — tool results are preserved as full messages until they are explicitly summarized away; they are *clamped* (not summarized) per-provider on output (`tool_return_truncation_chars`), and *search tools* (`conversation_search`, `conversation_search_date`, `archival_memory_search`) are excluded from per-call truncation so the agent can fetch rich recall (`letta/agents/letta_agent_v3.py:1877-1884`).
9. **Retrieval-as-tool** — recall is not auto-injected; the agent must call `conversation_search`, `archival_memory_search`, or `archival_memory_insert` to refresh its context from out-of-band storage (`letta/functions/function_sets/base.py:87-130`).
10. **Cache-aware refresh** — the system prompt includes `cache_control: {type: ephemeral}` markers on the system message and on the final block of the final message (`letta/llm_api/anthropic_client.py:1335-1384`); compaction respects this by triggering `rebuild_system_prompt_async` and `_refresh_messages(force_system_prompt_refresh=True)` only after a cut, not on every step (`letta/agents/letta_agent_v3.py:1255-1262, 1478-1485`). Opus 4.5 also enables the provider-side `clear_thinking_20251015` context-editing policy to preserve thinking blocks across compaction (`letta/llm_api/anthropic_client.py:580-588`).

The result is a framework whose loop *can* keep working after the conversation exceeds the context window — provided the system prompt itself fits, every layer above is well-tested (`tests/integration_test_summarizer.py:1-2290`, `tests/test_compaction_thresholds.py:1-53`, `tests/test_context_window_calculator.py:1-648`), and providers' summarizer LLMs are configured.

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale tied to rubric:

- The "context refresh" model is a **first-class typed pipeline** (`ContextWindowCalculator` / `TokenCounter` / `CompactionSettings` / `Summarizer` / `compact` / `_refresh_messages`), not an ad-hoc shrink.
- Triggers are **explicit and dual**: `context_window_exceeded` (reactive, up to `max_summarizer_retries=3` retries) and `post_step_context_check` (proactive at 90% of the model window with a special 90%-for-GPT-5 carve-out; `letta/services/summarizer/thresholds.py:14-41`). Both are wired into `LettaAgentV3._step` (`letta/agents/letta_agent_v3.py:1218-1294, 1438-1505`).
- Token counting has a real model: per-provider `TokenCounter` subclasses including a 1.3× safety-margin ApproxTokenCounter (`letta/services/summarizer/summarizer_sliding_window.py:24-42`; `letta/services/context_window_calculator/token_counter.py:21-125`).
- Compaction **emits structured observability events** (`EventMessage` with `event_data`, `SummaryMessage` with `CompactionStats`) BEFORE and AFTER the cut, so the client and the post-hoc trace can see exactly what changed (`letta/agents/letta_agent_v3.py:818-846, 848-892`; `letta/schemas/letta_message.py:406-449`).
- There are **explicit failure-mode exits**: `SystemPromptTokenExceededError` halts the loop cleanly; compacted messages are validated against `trigger_threshold` and a final escalation to `summarize_all` is attempted; if all modes fail, the error is logged and the run still terminates (`letta/services/summarizer/compact.py:391-411`; `letta/agents/letta_agent_v3.py:1285-1290, 1506-1509`).
- Failure isolation between LLM call → compaction → system-prompt rebuild is clearly layered (`letta/agents/letta_agent_v3.py:1218-1290, 1253-1262, 1478-1486`).

What keeps this from a 9 or 10:

- **Original messages are preserved, only re-flagged.** The agent's `messages` table grows monotonically; "removal" is a flag flip on `in_context` / removal from `agent_state.message_ids` plus a `summary` message inserted at index 1. There is no hard deletion / archival pipeline — old context can resurface via `conversation_search` indefinitely, but there is no TTL or compaction of the underlying `messages` table (no evidence of `pg_partman`, no message-pruning scheduler in `letta/services/`). This is a known tradeoff; described at `letta/agents/letta_agent_v3.py:758-816` but not addressed.
- **Cross-conversation consistency** relies on the agent reading `agent_state.message_ids` correctly; one bug here (wrong ordering, missed compaction message) will silently corrupt future contexts. There is no explicit invariant or validation step after `update_message_ids_async` (no evidence found).
- **The "context refresh inside the loop" claim depends on `summarizer_settings.partial_evict_summarizer_percentage = 0.30` + `sliding_window_percentage` default behavior** (`letta/settings.py:86`; `letta/services/summarizer/summarizer_config.py:79-82`). Some legacy V2 paths still call the deprecated `summarize_conversation_history` (`letta/agents/letta_agent_v2.py:1352-1410`) that uses message-count-based eviction (`message_buffer_limit=10, message_buffer_min=3`) with `clear=True` — and a code comment flags this as deprecated: `"Running deprecated v2 summarizer. This should be removed in the future."` (`letta/agents/letta_agent_v2.py:1361`).
- **The summarizer itself is a regular LLM call** — `simple_summary` falls back to a hard middle-truncate only after the LLM call failed; there is no structural validation of the produced summary against the source (no evidence of `tests/integration_test_summarizer.py` asserting "summary mentions X, Y, Z").
- **Per-call tool-return truncation** (`0.2 * context_window * 4`, `letta/agents/letta_agent_v3.py:150`) computes with `int(... )` and silently falls back to 5000 chars on malformed `context_window`, which is the only safety net — no logging or metric for "truncation triggered".
- **No evidence found** of:
  - repo maps or AST-based retrieval refresh inside the loop (Letta treats retrieval as a tool, not as ambient context),
  - per-event telemetry on `message_buffer_autoclear` paths beyond the `message_ids` truncation,
  - automatic removal of stale tool-return timestamps or unused tool names from history.

## Evidence Collected

Every entry MUST include a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Context rebuild per turn (default mode reads `agent.message_ids`) | `_prepare_in_context_messages_no_persist_async` reloads from DB by ID each turn | `letta/agents/helpers.py:148-317`; called at `letta/agents/letta_agent_v3.py:274-281` |
| Conversation-mode context rebuild | `ConversationManager.get_message_ids_for_conversation` + per-conversation isolated memory blocks | `letta/agents/helpers.py:177-205`; `letta/services/conversation_manager.py:986-1029` |
| `message_buffer_autoclear` flag → system-message-only | Schema field; helper truncates the ID list to `[system]` | `letta/schemas/agent.py:138-142`; `letta/agents/helpers.py:187-218` |
| Incremental system-prompt refresh (cache-aware) | `_refresh_messages` calls `_rebuild_memory` only when memory/tools/dirs changed; always scrubs inner thoughts | `letta/agents/letta_agent_v2.py:759-792` (`_refresh_messages`), `:813-860` (`_rebuild_memory` rebuild guard at `:853-856`) |
| Reasoning scrub on every refresh | Removes text content from assistant messages that have tool calls when reasoner disabled | `letta/helpers/reasoning_helper.py:25-48` |
| Token counter abstraction | `TokenCounter` base + Anthropic / Gemini / Tiktoken / Approx (1.3× safety margin) | `letta/services/context_window_calculator/token_counter.py:21-125, 268-316` |
| Per-step token estimate | `self.context_token_estimate = llm_adapter.usage.total_tokens` after each LLM call | `letta/agents/letta_agent_v3.py:1305-1307` |
| Compaction trigger threshold (proactive) | `int(context_window * SUMMARIZATION_TRIGGER_MULTIPLIER)`; `0.9` default; GPT-5 inherits same default but a `force_proactive` flag exists | `letta/constants.py:82-83`; `letta/services/summarizer/thresholds.py:27-41`; `tests/test_compaction_thresholds.py:1-53` |
| Reactive trigger on `ContextWindowExceededError` | Retry up to `max_summarizer_retries=3`, each retry may call `compact` | `letta/settings.py:96`; `letta/agents/letta_agent_v3.py:1218-1294` |
| In-loop compaction call (post-step) | After every successful step, if estimate > threshold → `compact()` → flush summary message | `letta/agents/letta_agent_v3.py:1438-1505` |
| Compactor entry-point | `LettaAgentV3.compact` → `compact_messages` returns `CompactResult` | `letta/agents/letta_agent_v3.py:2077-2134`; `letta/services/summarizer/compact.py:32-39, 134-472` |
| Mode selection | `CompactionSettings.mode` ∈ `sliding_window` / `all` / `self_compact_all` / `self_compact_sliding_window` | `letta/services/summarizer/summarizer_config.py:48-89` |
| Mode fallback chain | Each mode has its own fallback: `sliding_window → all`, `self_* → sibling → all`, summarizer LLM failure → clamp tool returns → hard-truncate → propagate | `letta/services/summarizer/compact.py:192-348`; `letta/services/summarizer/summarizer.py:562-647` |
| Sliding-window cutoff search | Eviction percentage grows 0.10 per iteration until token target reached; cutoff must land on assistant/approval message | `letta/services/summarizer/summarizer_sliding_window.py:152-198` |
| Approval protection | Cannot evict pending approval or assistant+approval pair from same `step_id` | `letta/services/summarizer/summarizer_sliding_window.py:133-137, 196-202`; `letta/services/summarizer/summarizer_all.py:41-61`; `letta/services/summarizer/self_summarizer.py:265-289` |
| Stale-context "removal" | `update_message_ids_async` rewrites `agent.message_ids`; conversation-mode also flips `conversation_messages.in_context` | `letta/services/agent_manager.py:999-1018`; `letta/services/conversation_manager.py:752-797` |
| Persisted compaction stats | `CompactionStats` packed into the summary message JSON; `SummaryMessage.compaction_stats` exposed | `letta/system.py:227-236`; `letta/schemas/letta_message.py:406-449` |
| Compaction event yield | `EventMessage(event_type="compaction", event_data={trigger, context_token_estimate, context_window})` yielded before compact | `letta/agents/letta_agent_v3.py:818-846, 1229-1234, 1451-1456` |
| Tool-result truncation policy | Per-call cap = `max(5000, int(context_window*0.2*4))`; search tools excluded | `letta/agents/letta_agent_v3.py:143-153, 1877-1884`; `letta/schemas/message.py:68-73` |
| Tool-result summarization | No automatic summarization — the summarizer is a free-form text generation; tool returns only get clamped (`TOOL_RETURN_TRUNCATION_CHARS = 5000`) | `letta/constants.py:443`; `letta/services/summarizer/summarizer.py:572-575` |
| Provider-side context editing (Anthropic) | Opus 4.5 enables `clear_thinking_20251015` to preserve thinking across compaction; ephemeral cache_control on system + last block | `letta/llm_api/anthropic_client.py:580-588, 1335-1384`; `letta/llm_api/anthropic_client.py:711-714` (cache_control placement) |
| System-prompt overflow detection | `_check_for_system_prompt_overflow` triggers a distinct `SystemPromptTokenExceededError` | `letta/agents/letta_agent_v3.py:741-756`; `letta/services/summarizer/compact.py:391-411` |
| Compactor system-prompt rebuild after cut | `agent_manager.rebuild_system_prompt_async(force=True)` + `_refresh_messages(force_system_prompt_refresh=True)` | `letta/agents/letta_agent_v3.py:1255-1262, 1478-1486`; `letta/services/agent_manager.py:1523-1612` |
| Retrieval refresh | Implemented as tools: `conversation_search`, `archival_memory_search`, `archival_memory_insert` | `letta/functions/function_sets/base.py:87-130` |
| Compactor provider fallback (overload/rate-limit) | Anthropic → Bedrock Opus 4.5; ZAI → Baseten GLM-5 | `letta/services/summarizer/summarizer.py:714-816` |
| V2 legacy summarizer (deprecated) | `summarize_conversation_history` uses message-count static buffer with `clear=True` | `letta/agents/letta_agent_v2.py:1352-1410`; warning at `:1361` |
| Static-buffer summarizer (legacy) | `Summarizer._static_buffer_summarization` — retain last `message_buffer_min` (default 3) of N | `letta/services/summarizer/summarizer.py:244-343` |
| Partial-evict summarizer (legacy, deprecated) | Uses `partial_evict_summarizer_percentage = 0.30`; inserts summary at index 1, requires assistant-message anchor | `letta/services/summarizer/summarizer.py:136-242`; `letta/settings.py:86` |
| Approx-token safety margin | `APPROX_TOKEN_SAFETY_MARGIN = 1.3` applied only when counter is ApproxTokenCounter | `letta/services/summarizer/summarizer_sliding_window.py:23-42` |
| Summarizer model selection | Auto-mode agents use Haiku 4.5 (`zai/glm-5` fallback); providers default to lightweight model | `letta/services/summarizer/compact.py:42-132`; `letta/services/summarizer/summarizer_config.py:11-32` |
| Test: thresholds | `tests/test_compaction_thresholds.py` covers GPT-5 carve-out, default threshold, `force_proactive` | `tests/test_compaction_thresholds.py:1-53` |
| Test: context-window calculator | `_extract_tag_content`, `extract_system_components` for system-prompt sections | `tests/test_context_window_calculator.py:1-200+` |
| Test: end-to-end compaction | `test_compact_returns_valid_summary_message_and_event_message`, `test_v3_compact_uses_compaction_settings_model_and_model_settings`, `test_summarize_hard_eviction_when_still_over_threshold`, sliding-window cutoff tests, self-mode fallback | `tests/integration_test_summarizer.py:780-1100, 1106-1314, 1778-1969, 2225-2290` |

## Answers to Dimension Questions

1. **Is context static or rebuilt each turn?**
   Context is **rebuilt each turn** from durable storage. `_prepare_in_context_messages_no_persist_async` re-reads `agent_state.message_ids` (default mode) or `conversation_messages` (conversation mode) from the DB every turn (`letta/agents/helpers.py:148-205`), and `_refresh_messages` selectively rewrites the system prompt if memory changed (`letta/agents/letta_agent_v2.py:759-792`). In `message_buffer_autoclear` mode the context is rebuilt to `[system]` only (`letta/schemas/agent.py:138-142`; `letta/agents/helpers.py:187-218`).

2. **What gets dropped first?**
   For the V3 path with `mode="sliding_window"` (default), the **earliest non-system messages up to an assistant-message boundary** get summarized and replaced by a single `summary`-role message at index 1 (`letta/services/summarizer/summarizer_sliding_window.py:152-198`; `letta/services/summarizer/compact.py:462-465`). The exact cutoff is decided by token-count target `goal_tokens = (1 - sliding_window_percentage) * context_window`. For `mode="all"`, the entire non-system history between the first user message and the last protected message is summarized in one shot (`letta/services/summarizer/summarizer_all.py:41-61`). Per-tool-result truncation drops the **tail** of large tool returns to `min(5000, 0.2*context_window*4)` chars with a `"... [truncated N chars]"` marker (`letta/schemas/message.py:68-73`; `letta/agents/letta_agent_v3.py:143-153`). For the V2 path the **static buffer** drops oldest messages and optionally fires a background summarizer agent (`letta/services/summarizer/summarizer.py:244-343`).

3. **Is summarization faithful?**
   **Faithfulness is delegated to the summarizer LLM with no structural validation in code.** The summarizer prompt (`SELF_ALL_PROMPT`, `SELF_SLIDING_PROMPT`, `SLIDING_PROMPT`, `ALL_PROMPT` — `letta/services/summarizer/summarizer_config.py:35-45`) instructs the model, but there is no evidence of tests asserting that specific facts from the source transcript appear in the output. The one safeguard is the **token-budget enforcement**: after compaction, `compact_messages` re-counts tokens and escalates `sliding_window → all` if the threshold isn't met, and finally checks that the system prompt isn't the overflow culprit (`letta/services/summarizer/compact.py:350-411`). The summary is clipped to `clip_chars` (default 50000) with a `SUMMARY_TRUNCATION_SUFFIX` if needed (`letta/services/summarizer/summarizer_sliding_window.py:227-229`; `letta/services/summarizer/summarizer.py:227-229`).

4. **Are tool results preserved or summarized?**
   **Preserved verbatim** in the messages list until explicitly summarized away on a cut. Per-call **clamping only** (`0.2 * context_window * 4` chars) is applied at message-conversion time (`letta/agents/letta_agent_v3.py:143-153, 1877-1884`). Search tools (`conversation_search`, `conversation_search_date`, `archival_memory_search`) are excluded from per-call truncation so the agent can re-pull rich recall (`letta/agents/letta_agent_v3.py:1877-1884`). When summarization does happen, **tool messages are not specially handled** — they go through `simple_formatter` and are sent to the summarizer LLM as plain text (`letta/services/summarizer/summarizer.py:346-384`).

5. **Can the model tell what context is missing?**
   **Yes, explicitly.** The summary message has a fixed prefix `"Note: prior messages from the beginning of the conversation have been hidden from view due to conversation memory constraints."` plus, for `sliding_window`, `"Note: N messages from the beginning of the conversation have been hidden from view due to memory constraints."` (`letta/system.py:207-224`). For retrieval, the model must use `conversation_search` / `archival_memory_search` / `archival_memory_insert` to fetch from out-of-band storage (`letta/functions/function_sets/base.py:87-130`). `CompactionStats` expose before/after message and token counts and the trigger (`letta/schemas/letta_message.py:406-420`).

## Architectural Decisions

- **Two-tier triggers**: separate reactive (error → retry with compact) and proactive (post-step threshold check). This avoids both context-window overflow errors in the hot path and silent overspend — `letta/agents/letta_agent_v3.py:1218-1294, 1438-1505`.
- **System prompt is protected from compaction**: distinct `SystemPromptTokenExceededError` exits cleanly rather than forcing further context loss — `letta/services/summarizer/compact.py:391-411`; `letta/agents/letta_agent_v3.py:741-756`.
- **Compaction result is its own message role** (`MessageRole.summary`) distinct from `user`/`assistant`/`tool`, enabling server-side packaging and separate API exposure — `letta/services/summarizer/compact.py:434-460`; `letta/schemas/message.py`.
- **`CompactionSettings.mode` + nested fallback chain**: explicit `sliding_window → all → exception` and `self_compact_all → self_compact_sliding_window → all` — `letta/services/summarizer/compact.py:192-348`.
- **`Clamp-then-summary`** for the summarizer LLM itself: clamp tool returns first, hard-truncate the transcript with a marker second — `letta/services/summarizer/summarizer.py:562-647`.
- **Conversation-scoped context** as a first-class concept: `ConversationManager` separately persists `in_context` flags and positions for `conversation_messages`, isolates memory blocks per conversation (`letta/services/conversation_manager.py:752-1029`). This avoids leaking changes across multi-conversation agents.
- **Prefix-cache-aware rebuild**: `_rebuild_memory` rebuilds the system prompt only when memory/tools/directories content changes — explicit cache-control markers on the system message and final user-message block (`letta/llm_api/anthropic_client.py:1335-1384`). This is one of the few frameworks to expose this lever.
- **Provider-side context editing for Opus 4.5**: `clear_thinking_20251015` is enabled explicitly so thinking blocks survive compaction via the provider — `letta/llm_api/anthropic_client.py:580-588`.
- **Summarizer-provider fallbacks**: Anthropic → Bedrock Opus 4.5, ZAI → Baseten GLM-5 (`letta/services/summarizer/summarizer.py:714-816`); auto-mode → Anthropic Haiku 4.5 / ZAI GLM-5 (`letta/services/summarizer/compact.py:65-77`).
- **Context statistics are exposed to the client** via `EventMessage` + `CompactionStats` — both pre-compaction (trigger info) and post-compaction (delta) are surfaced — `letta/agents/letta_agent_v3.py:818-846, 1230-1282`.
- **Tool calls have per-tool `return_char_limit`** plus type-aware defaults (`truncate=False` for search tools so the agent isn't penalized for using recall) — `letta/agents/letta_agent_v3.py:1877-1884`.

## Notable Patterns

- **Stateful, continuously-rebuilt context** — in-context messages are persisted to the `messages` table with `agent.message_ids` as the truth, so a server restart or rerun picks up exactly where it left off (`letta/services/agent_manager.py:999-1018`).
- **Proactive compaction at 90% of context** — `SUMMARIZATION_TRIGGER_MULTIPLIER = 0.9` is a deliberate fudge factor for "too many tokens" defenses and is model-aware via `is_gpt5_model_family` (`letta/services/summarizer/thresholds.py:14-41`).
- **Pre-tokenized summary of tool-heavy transcripts** — `simple_summary` clamps then middle-truncates with `head_frac=0.3, tail_frac=0.3`, favoring recency + first-utterance preservation (`letta/services/summarizer/summarizer.py:387-433, 619`).
- **Background fire-and-forget summarization** for the legacy V2 voice path (`EphemeralSummaryAgent`) — `letta/services/summarizer/summarizer.py:339-341`.
- **`llm_request_finish_timestamp_ns` orchestration** lets the LLM adapter close over timing without the agent re-reading it (`letta/schemas/letta_response.py`; see usage at `letta/agents/letta_agent_v3.py:1301-1303`).
- **Compaction trigger telemetry**: tag every summarizer LLM call with `compaction_settings` payload so traces can be filtered — `letta/services/summarizer/summarizer.py:519-530`; `letta/services/summarizer/summarizer_sliding_window.py:215-222`.

## Tradeoffs

- **Recency-biased vs. uniform summarization**: `sliding_window` keeps the tail; `all` drops uniformly. The default is `sliding_window`, so an agent that needs older context must rely on `conversation_search` / `archival_memory_search` (which preserves raw messages) or accept that it cannot access what was summarized — `letta/services/summarizer/summarizer_config.py:76-82`.
- **Original messages are never deleted from the DB**, only flagged out of context. Cost: unbounded DB growth; benefit: no information is truly lost and `conversation_search` can still retrieve it. No evidence of a purge policy.
- **Token counting is approximate by default** (`ApproxTokenCounter`, bytes/4 with 1.3× margin). It can over- or under-estimate, but the proactive 90% threshold + reactive `ContextWindowExceededError` backstop means under-estimation is caught — `letta/services/summarizer/summarizer_sliding_window.py:23-42`.
- **Compaction always rebuilds the persisted system prompt** (`rebuild_system_prompt_async(force=True)`), intentionally busting the cache for memory edits and accepting the rebuild cost — `letta/agents/letta_agent_v3.py:1255-1262, 1478-1486`.
- **Retry budget is hard-coded** to `max_summarizer_retries = 3` (`letta/settings.py:96`), no per-agent override observed. If summarization is genuinely stuck (e.g., very large system prompt), the loop terminates with `SystemPromptTokenExceededError`, which is the correct behavior but surfaces as a failure to the client.
- **Per-call tool truncation silently clamps to 5000 chars** if `context_window` is missing or non-numeric (`letta/agents/letta_agent_v3.py:149-153`).
- **`message_buffer_autoclear` mode removes all history** but still keeps memory blocks and archival memories (`letta/schemas/agent.py:138-142`). Users may not realize the agent will not remember anything they haven't explicitly stored.

## Failure Modes / Edge Cases

- **System prompt itself exceeds context window** — surfaced as `SystemPromptTokenExceededError` and converted to `StopReasonType.context_window_overflow_in_system_prompt`; the agent halts cleanly instead of looping — `letta/services/summarizer/compact.py:391-411`; `letta/agents/letta_agent_v3.py:741-756, 1285-1290, 1506-1509`.
- **Summarizer LLM unavailable / rate-limited** — provider fallback to Bedrock Opus 4.5 (Anthropic) or Baseten GLM-5 (ZAI) — `letta/services/summarizer/summarizer.py:714-816`.
- **Summarizer LLM itself hits context overflow** — three-step ladder: full payload → clamp tool returns → middle-truncate transcript → propagate error to caller — `letta/services/summarizer/summarizer.py:562-647`.
- **After-compaction token count still above threshold** — escalates `sliding_window → all`; if still over, classifies as system-prompt overflow or logs critical error — `letta/services/summarizer/compact.py:359-411`.
- **`agent.state.message_ids` reordering** — handled for conversation-mode by `update_in_context_messages` explicitly rewriting positions to match `in_context_message_ids` order — `letta/services/conversation_manager.py:787-796`.
- **Pending approval requests** — protected from being summarized away (`letta/services/summarizer/summarizer_sliding_window.py:133-137, 196-202`; `letta/services/summarizer/summarizer_all.py:41-61`; `letta/services/summarizer/self_summarizer.py:265-289`).
- **"stale context" after summarization** — the `_refresh_messages(force_system_prompt_refresh=True)` call right after compaction refreshes in-memory tool/dir lists, but does not re-evaluate per-tool `return_char_limit` values or run on the next request; the only automated refresh trigger is the proactive post-step check — `letta/agents/letta_agent_v3.py:1438-1505`.
- **Run cancellation during compaction** — if the run is cancelled mid-summary step, `_check_run_cancellation` at the top of `_step` exits early; the in-progress compaction is left incomplete (the `summary` message may be partially persisted) — `letta/agents/letta_agent_v2.py:750-757`; `letta/agents/letta_agent_v3.py:1031-1035`.
- **Idempotent approval retries after compaction**: explicit recovery path in `_prepare_in_context_messages_no_persist_async` that searches recent in-context and falls back to the latest tool message in DB by ID — `letta/agents/helpers.py:248-265`.
- **Compaction EventMessage without `id` or `otid`** — explicit ack logged but not failed: `letta/agents/letta_agent_v3.py:601-610`.
- **V2 legacy path**: the deprecated `summarize_conversation_history` clears entire history if forced (`clear=True`), losing all summary-rollup chain — `letta/agents/letta_agent_v2.py:1352-1410`.

## Future Considerations

- Move V3 paths to drop the V2 `summarize_conversation_history` API entirely — it's still wired into `letta_agent_v2.step` and `letta_agent_v2.stream` for the legacy path — `letta/agents/letta_agent_v2.py:276-283, 411-419`.
- Replace the **monolithic `messages` table growth** with a TTL or sparse-archive-of-evicted-messages pipeline for `conversation_messages`. Currently `in_context=False` rows stay forever; a `passages`-style migration is implied by archival memory but not implemented for the messages table (no evidence found in `letta/services/`).
- Add **per-turn `context_token_estimate` trajectory** to the response (only the **last** value is exposed in `self.usage.context_tokens` — `letta/agents/letta_agent_v3.py:425, 697`). A sliding history would make proactive tuning actionable.
- Promote **summarization faithfulness checks** beyond the existing `CompactionStats` packing — e.g., a downstream test that asserts key entities appear in the summary.
- Add an **observation schema** for "context-dropped" events exposed through `message_stream` so the client UI can render a "this part of your history was summarized" badge (today only `EventMessage` is emitted, only when the client requests `include_compaction_messages=True` — `letta/agents/letta_agent_v3.py:818-846`).
- Generalize the **per-call tool-return truncation** to a per-tool `tool_truncation_strategy` (`head`/`tail`/`middle`/`none`) — currently it's a uniform character truncation only.
- Replace the **legacy static buffer** (`letta/services/summarizer/summarizer.py:244-343`) and **legacy partial evict** (`:136-242`) with V3 compaction in the voice paths.
- Extend the **trace metadata** in `compaction_settings` to include the model/provider name (`compact_messages` only carries mode + counts — `letta/services/summarizer/compact.py:414-424`).

## Questions / Gaps

- **What triggers summarization in a "concurrency" model where two V3 agents share the same `agent_id`?** The `_prepare_in_context_messages_no_persist_async` reads message IDs at the top of each step, but with two concurrent calls, the second writer's compaction may be overwritten by the first's. No clear evidence of `SELECT … FOR UPDATE` or optimistic-lock on `message_ids` in `letta/services/agent_manager.py:999-1018`.
- **What happens if the summarizer LLM returns JSON instead of plain text?** `simple_summary` only handles text content via Anthropic streaming or generic chat-completion. No observed validator; the response would be raw JSON injected as the summary.
- **How is the ratio between `messages_count_before` and `messages_count_after` actually used?** It appears in `CompactionStats` only — no evidence of downstream alerting or auto-tuning of `sliding_window_percentage`.
- **What's the relationship between `agent_state.compaction_settings` and the global `summarizer_settings`?** Both exist (`letta/agents/letta_agent_v3.py:2106-2107`; `letta/services/summarizer/summarizer_config.py:79-82`), but the per-agent field's wiring into the planning layer wasn't fully traced.
- **Are there any rate-limits or quotas on `summarize_conversation_history` re-tries that could silently slow down a long run?** `summarizer_settings.max_summarizer_retries = 3` is enforced, but no backoff observed; the loop can pound the summarizer LLM in quick succession.
- **What's the cost model when `mode="self_compact_sliding_window"` and the same LLM is used?** Self-mode reuses the agent's LLM (`letta/services/summarizer/self_summarizer.py:101-124`), risking cost amplification; no evidence of guard-rails.
- **Tests that exercise the actual `_refresh_messages` cache-busting logic end-to-end across many turns** — `tests/integration_test_summarizer.py` covers compaction modes but not the prefix-cache promise over time.

---

Generated by `reports/dimensions/03.07-context-refresh-inside-the-loop.md` against `letta`.
