# Source Analysis: letta

## Dimension 05.06 — Memory Compression and Summarization

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas) |
| Analyzed | 2026-08-25 |

## Summary

Letta (formerly MemGPT) implements memory compression through a dedicated `letta/services/summarizer/` package with two coexisting generations. The current pipeline, `compact_messages` (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:135`), is invoked by the V3 agent loop and supports four configurable modes (`all`, `sliding_window`, `self_compact_all`, `self_compact_sliding_window`) declared in `CompactionSettings` (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_config.py:76-78`). A legacy `Summarizer` class with message-count-based buffer modes remains in use by the v1/v2 agents and voice agents (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:36`, `studies/agent-harness-study/sources/letta/letta/agents/voice_sleeptime_agent.py:60`).

Compaction is triggered three ways: reactively on `ContextWindowExceededError` from the LLM (retry loop bounded by `max_summarizer_retries=3`, `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1218-1294`; `studies/agent-harness-study/sources/letta/letta/settings.py:96`), proactively after each step when the token estimate exceeds 90% of the context window (`SUMMARIZATION_TRIGGER_MULTIPLIER = 0.9`, `studies/agent-harness-study/sources/letta/letta/constants.py:83`; threshold helper at `studies/agent-harness-study/sources/letta/letta/services/summarizer/thresholds.py:27-41`; post-step check at `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1438-1474`), and manually via the `POST /v1/agents/{agent_id}/summarize` endpoint (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2430`).

The summary replaces raw messages inside the context window but never deletes them from storage: compaction only rewrites the in-context ID list (`_checkpoint_messages`, `studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:758-816`), so evicted rows persist in the `messages` table. Summaries are first-class records (`MessageRole.summary`, `studies/agent-harness-study/sources/letta/letta/schemas/enums.py:117`) that carry embedded machine-readable `CompactionStats` (trigger, tokens before/after, message counts before/after) packed into the message JSON (`studies/agent-harness-study/sources/letta/letta/system.py:207-238`; schema at `studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:406-420`). Failure handling is deeply layered: mode fallback chains, provider fallbacks (Anthropic→Bedrock Opus, ZAI→Baseten GLM-5), transcript clamping and middle-truncation, and protected-message rules that never evict pending approval requests. What is missing is any semantic evaluation of summary quality or drift detection; evaluation is purely mechanical (token-count verification after compression).

On the dimension question "does compression preserve decisions, facts, and uncertainty?": prompts explicitly require preserving goals, decisions, errors/fixes, verbatim identifiers, and next steps (`ALL_PROMPT`, `studies/agent-harness-study/sources/letta/letta/prompts/summarizer_prompt.py:4-26`). Uncertainty preservation is weaker — it appears only indirectly as "Lookup hints" sections instructing the summarizer to record topics/key terms for later retrieval rather than as explicit uncertainty tracking.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band met):** Four named compaction modes behind one Pydantic config surface (`CompactionSettings`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_config.py:48-89`); a single orchestration entry point (`compact_messages`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:135`); per-mode default prompt selection (`get_default_prompt_for_mode`, `summarizer_config.py:35-45`).
- **Tests:** Unit tests for trigger thresholds (`studies/agent-harness-study/sources/letta/tests/test_compaction_thresholds.py:23-53`), static-buffer behavior (`studies/agent-harness-study/sources/letta/tests/test_static_buffer_summarize.py:41-133`), telemetry propagation (`studies/agent-harness-study/sources/letta/tests/test_provider_trace_summarization.py:85-339`), and an extensive integration suite including a regression test for summarization loops (`studies/agent-harness-study/sources/letta/tests/integration_test_summarizer.py:999-1105`).
- **Operational safeguards:** Post-compaction token re-verification against the trigger threshold with automatic fallback to `all` mode, plus a hard-stop distinction between system-prompt overflow (`SystemPromptTokenExceededError`) and recoverable overflow (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:359-412`).
- **Why not 9–10:** Two parallel summarization systems (legacy `Summarizer` vs. `compact_messages`) with acknowledged deprecation debt (`studies/agent-harness-study/sources/letta/letta/settings.py:88-91`); no semantic drift detection or summary-quality evaluation; magic-number heuristics scattered across modules with TODOs calling for centralization (`studies/agent-harness-study/sources/letta/letta/services/summarizer/thresholds.py:17-19`); voice-path fire-and-forget summarization failures are only logged (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:124-134`).

## Evidence Collected

Every entry cites workspace-relative paths under `studies/agent-harness-study/sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Compaction orchestrator | `compact_messages()` dispatches per-mode and builds `CompactResult` | studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:135-155, 467-472 |
| Mode enum/config | `CompactionSettings.mode` Literal of four modes; default `sliding_window` | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_config.py:76-82 |
| Reactive trigger | `ContextWindowExceededError` → compact + retry LLM request, ≤3 attempts | studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1217-1294 |
| Proactive trigger | post-step check `context_token_estimate > compaction_trigger_threshold` | studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1438-1474 |
| Threshold math | `context_window * SUMMARIZATION_TRIGGER_MULTIPLIER` (0.9) | studies/agent-harness-study/sources/letta/letta/constants.py:82-83; studies/agent-harness-study/sources/letta/letta/services/summarizer/thresholds.py:27-41 |
| GPT-5 special case | GPT-5 family compacts proactively at 90%; rationale comment re codex harness | studies/agent-harness-study/sources/letta/letta/services/summarizer/thresholds.py:27-40 |
| Sliding-window budget | eviction % grows by 0.10 until remaining tokens ≤ `(1-pct)*context_window`; cutoff must be assistant/approval | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:144-198 |
| Token counting | model-specific token counter; ApproxTokenCounter gets 1.3× safety margin; tools included | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:21-42, 45-95 |
| Summary prompts | `ALL_PROMPT` (500-word cap), `SLIDING_PROMPT` (300 words), self variants; required sections incl. verbatim identifier preservation | studies/agent-harness-study/sources/letta/letta/prompts/summarizer_prompt.py:1-97 |
| Self-summarization request shaping | dummy assistant turn prevents conversation continuation; clip to `clip_chars` | studies/agent-harness-study/sources/letta/letta/services/summarizer/self_summarizer.py:61-83, 136-138 |
| Coverage ranges (voice/static path) | evicted vs. in-context transcripts numbered with offset; labeled "(Older) Evicted"/"(Newer) In-Context" | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:317-341, 436-457 |
| Summary record | `MessageRole.summary` persisted; legacy role=user variant supported | studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:434-460; studies/agent-harness-study/sources/letta/letta/schemas/enums.py:117 |
| Raw-to-summary envelope | packed JSON `{"type":"system_alert","message",...,"compaction_stats"}`; `unpack_message` extracts text | studies/agent-harness-study/sources/letta/letta/system.py:207-238, 269-285 |
| Stats surfaced to clients | `SummaryMessage.compaction_stats`; extraction helpers | studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:406-449; studies/agent-harness-study/sources/letta/letta/schemas/message.py:1092-1133 |
| Provider conversion | summary role rendered as user message for OpenAI/Anthropic/chat-completions | studies/agent-harness-study/sources/letta/letta/schemas/message.py:1392-1400, 1587-1595, 1926-1933 |
| Raw history retention | `_checkpoint_messages` rewrites only `message_ids`/conversation in-context set; no deletion | studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:758-816 |
| Recall of evicted content | `conversation_search` / `archival_memory_search` base tools | studies/agent-harness-study/sources/letta/letta/constants.py:114-115 |
| Manual regeneration | `POST /{agent_id}/summarize` merges request overrides into agent `CompactionSettings`; auto-switches default prompt on mode change | studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2430-2508 |
| Mode fallback chain | self_compact_all → self_compact_sliding_window → all; sliding_window → all | studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:192-255, 309-346 |
| Threshold re-check | recount after compaction; retry with all mode; `SystemPromptTokenExceededError` if system prompt alone overflows; else log critical but continue | studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:350-412 |
| Transcript fallbacks | clamp tool returns (5000 chars), then middle-truncate to byte budget with `[TRUNCATED...]` marker | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:387-433, 564-647; studies/agent-harness-study/sources/letta/letta/constants.py:443 |
| Provider fallback | Anthropic→Bedrock Claude Opus handle; ZAI→Baseten GLM-5 on overload/rate-limit | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:714-738, 741-816 |
| Protected messages | pending approvals never evicted; assistant+approval same-step kept together | studies/agent-harness-study/sources/letta/letta/services/summarizer/self_summarizer.py:182-187, 265-289; studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_all.py:41-61 |
| Telemetry tagging | summarizer LLM calls tagged `call_type=summarization` with compaction_settings metadata | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:517-530 |
| Client events | `EventMessage(event_type="compaction")` yielded pre-compaction | studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:818-851 |
| Legacy system | `Summarizer` class w/ STATIC_MESSAGE_BUFFER & PARTIAL_EVICT modes | studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:36-122, 244-343 |
| Global settings | `SummarizerSettings`: buffer limits, retries, memory warning threshold | studies/agent-harness-study/sources/letta/letta/settings.py:74-111 |
| Per-agent defaults | agent creation seeds `CompactionSettings.model` from provider-default lightweight models (haiku/gpt-5-mini/gemini-flash) | studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:495-513; studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_config.py:11-32 |
| Tests | thresholds unit tests; static-buffer unit tests; integration suite incl. regression for summarization loops | studies/agent-harness-study/sources/letta/tests/test_compaction_thresholds.py:5-53; studies/agent-harness-study/sources/letta/tests/test_static_buffer_summarize.py:41-133; studies/agent-harness-study/sources/letta/tests/integration_test_summarizer.py:199-2200 |

## Answers to Dimension Questions

### 1. When does summarization happen?

Three trigger paths in the active (V3) loop:

- **Reactive (hard error):** when the LLM call raises `ContextWindowExceededError`, the step retries up to `summarizer_settings.max_summarizer_retries` (default 3) after compacting (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1217-1294`; retry bound at `studies/agent-harness-study/sources/letta/letta/settings.py:96`).
- **Proactive (post-step):** after every successful step, if `context_token_estimate > context_window * 0.9`, compaction runs immediately (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1438-1505`). The multiplier is a constant chosen "to avoid 'too many tokens in prompt' fallbacks" (`studies/agent-harness-study/sources/letta/letta/constants.py:82-83`). The `thresholds.py` docstring documents a model-specific policy: GPT-5-family models use the proactive 90% threshold because observed runs hit max-output-token errors near the 272k input window, matching "the codex harness' proactive 90% compaction policy"; other models compute the same formula today but a `force_proactive` flag exists for Temporal paths (`studies/agent-harness-study/sources/letta/letta/services/summarizer/thresholds.py:27-41`).
- **Manual:** `POST /v1/agents/{agent_id}/summarize` invokes `LettaAgentV3.compact` on demand with request-level settings overrides (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2430-2508`).

Legacy agents instead trigger on message counts: static buffer limit/min (defaults 60/15, `studies/agent-harness-study/sources/letta/letta/settings.py:79-80`) or partial-evict percentage of message count (30%, `studies/agent-harness-study/sources/letta/letta/settings.py:86`), evaluated in `Summarizer.summarize` (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:104-122`).

### 2. What evidence does the summary cover?

- **Prompt-mandated coverage:** All four active prompts require structured sections — High level goals, What happened, Important details (with "Preserve identifiers verbatim… exact URL, issue/PR number, ticket ID"), Errors and fixes, Current state, Optional Next Step, and Lookup hints (`studies/agent-harness-study/sources/letta/letta/prompts/summarizer_prompt.py:4-26` for `all`; 28-45 for sliding; 48-68 and 71-97 for self variants). Recursive coverage is explicit: "If there is a previous summary being evicted, please extract a concise version of the critical info from it" (`summarizer_prompt.py:9`).
- **Transcript formatting evidence:** the summarizer sees a compact plaintext transcript wrapped in `<start_transcript>` markers, with tool calls flattened to `name(args)` form (`simple_formatter`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:346-384`). Images become `[N images omitted]` placeholders (`format_transcript`, `summarizer.py:684-687`).
- **Coverage ranges:** the voice/static-buffer path numbers evicted vs. retained lines and passes both segments, labeled "(Older) Evicted Messages" and "(Newer) In-Context Messages," so the summary knows exactly which span is leaving context (`build_summary_request_text`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:317-341, 436-457`). Sliding-window mode reports `messages_summarized`/`messages_kept` counts into telemetry metadata (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:215-222`).
- **Machine-readable stats:** every produced summary embeds `CompactionStats` (trigger, context_tokens_before/after, context_window, messages_count_before/after) inside the packed message JSON (`studies/agent-harness-study/sources/letta/letta/system.py:207-238`; schema `studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:406-420`).

### 3. Can summary drift be detected?

**No semantic drift detection found.** Searches for any comparison of summary content against evicted history returned nothing beyond mechanical checks:

- Post-compaction token recount verifies *size*, not fidelity; failure triggers a mode fallback and eventually a critical log ("Failed to summarize messages after fallback") while deliberately not bricking the agent (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:359-412`).
- Drift mitigation is prompt-side only: recursive-summary clauses (`summarizer_prompt.py:9`) and "Lookup hints" that tell the agent where detailed content can be recovered from history (`summarizer_prompt.py:22, 41, 63, 90`). Because raw messages remain queryable via `conversation_search` (`studies/agent-harness-study/sources/letta/letta/constants.py:115`), drift can be *compensated* at runtime, but nothing measures whether the summary diverges from what was evicted.
- The nearest thing to observable drift signal is `CompactionStats` embedded per summary (`studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:406-420`), which enables external monitoring of compression ratios but not accuracy.

### 4. Is raw history retained?

Yes — replacement applies only to the context window, not to storage.

- Compaction produces `[system, summary_message, tail...]` as the new in-context list (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:462-465`).
- Persistence swaps the pointer set: `agent.message_ids` (or `conversation_messages` in-context flags in conversation mode) is rewritten by `_checkpoint_messages`; there is no delete call anywhere in the summarizer package (grep for `delete` in `letta/services/summarizer/*.py` returns nothing) (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:758-816`).
- Evicted messages stay addressable through recall tooling (`conversation_search`, `archival_memory_search`, `studies/agent-harness-study/sources/letta/letta/constants.py:114-133`), and the summary envelope tells the model that prior messages were "hidden from view due to memory constraints" (`studies/agent-harness-study/sources/letta/letta/system.py:213-232`).
- Caveat: the manual summarize endpoint fetches only currently in-context messages (`get_messages_by_ids_async(agent.message_ids...)`, `studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2445`), so already-evicted history is not re-ingested into new summaries.

### 5. Can summaries be regenerated?

Partially.

- **Manual re-run:** `/summarize` accepts a full `CompactionRequest` override; changed modes automatically receive their mode-appropriate default prompt, and results must reduce message count or the endpoint returns HTTP 400 (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2465-2508`).
- **Automatic re-selection:** the fallback chain effectively regenerates summaries with different strategies within a single compaction attempt (self → sliding → all; sliding → all; plus the threshold-failure retry with `all`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:210-255, 309-346, 360-388`).
- **Model/prompt swap:** `CompactionSettings.model`, `model_settings`, and `prompt` are user-settable per agent (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_config.py:55-74`), seeded at creation with lightweight defaults like `anthropic/claude-haiku-4-5` or `openai/gpt-5-mini` (`studies/agent-harness-study/sources/letta/letta/services/agent_manager.py:495-513`).
- **Limitation:** since only the in-context slice is fetched for endpoint-driven compaction, regenerating a summary that spans previously evicted content requires the older raw messages to still be in context; there is no API that rebuilds a summary from arbitrary stored history ranges.

## Architectural Decisions

1. **Two-tier trigger design (reactive + proactive).** Rather than relying solely on provider errors, Letta estimates tokens after each step and compacts at 90% of the window (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1438-1474`), keeping the reactive path as a safety net with bounded retries (`letta_agent_v3.py:1217-1294`). This trades an extra token count per step for avoiding hard failures.
2. **Summaries as persisted, typed messages — not ephemeral strings.** A dedicated `MessageRole.summary` (`studies/agent-harness-study/sources/letta/letta/schemas/enums.py:117`) is downcast to `user` per provider at request-build time (`studies/agent-harness-study/sources/letta/letta/schemas/message.py:1587-1595`), letting the DB keep provenance while staying compatible with providers that lack a summary concept.
3. **Stats embedded in the artifact itself.** `CompactionStats` travels inside the summary's packed JSON (`studies/agent-harness-study/sources/letta/letta/system.py:207-238`), so observability data survives wherever the message goes — client APIs expose it via `SummaryMessage.compaction_stats` (`studies/agent-harness-study/sources/letta/letta/schemas/letta_message.py:442-449`).
4. **Eviction-pointer model.** Storage is append-only; compaction edits only the in-context ID list (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:792-816`), decoupling "what the model sees" from "what exists."
5. **Self vs. delegated summarization.** `self_compact_*` modes reuse the agent's own LLM/client for cache-compatible summarization (tools passed explicitly "for cache compatibility", `studies/agent-harness-study/sources/letta/letta/services/summarizer/self_summarizer.py:99-118`), while `all`/`sliding_window` route to cheap default summarizer models resolved per provider (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:42-131`).
6. **Graceful-degradation philosophy.** Nearly every layer has a documented fallback: mode chain (`compact.py:210-255`), provider overload rerouting (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:741-816`), payload shrinking (`summarizer.py:564-647`), and finally "log error but don't brick the agent" (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:409-410`).

## Notable Patterns

- **Assistant-boundary cutoffs everywhere.** Every eviction strategy walks back to a valid cutoff message (assistant, or approval carrying tool_calls) so partial tool-call sequences are never split (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:156-178`; `self_summarizer.py:196-216`; `summarizer.py:170-179`).
- **Approval-request protection invariant.** Pending approvals cannot be evicted; if the preceding assistant shares the same `step_id`, both are preserved together (`studies/agent-harness-study/sources/letta/letta/services/summarizer/self_summarizer.py:265-289`; mirrored in `summarizer_all.py:41-61` and the static buffer trim logic at `summarizer.py:295-304`). This same rule is duplicated three times — a consistency risk.
- **Iterative eviction-until-budget loop.** Sliding-window starts at the configured keep-percentage and escalates eviction by 10 points until the projected remaining token count meets goal, aborting into complete summarization if no valid cutoff exists (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:163-198`).
- **Anti-continuation guard for self-summarization.** A synthetic assistant turn ("I understand. Let me summarize.") is injected when the last message isn't assistant-role, preventing the LLM from answering the transcript instead of summarizing it (`studies/agent-harness-study/sources/letta/letta/services/summarizer/self_summarizer.py:67-79`).
- **ACK prefill trick (legacy).** `MESSAGE_SUMMARY_REQUEST_ACK` is placed as a prior assistant turn to force summary-only outputs (`studies/agent-harness-study/sources/letta/letta/constants.py:427`; used at `summarizer.py:539-545`, gated by `prompt_acknowledgement`).
- **Middle-out truncation with honest markers.** Byte-budget truncation keeps head/tail fractions and inserts `[TRUNCATED: dropped N middle chars due to context budget]` (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:387-433`); summaries clipped to `clip_chars` get `"... [summary truncated to fit]"` (`constants.py` in summarizer package, line 3; applied at `summarizer_all.py:82-84`).
- **Token-count conservatism.** Approximate counters multiply by a 1.3 safety margin because bytes/4 underestimates JSON overhead by 25-35%, and tool-definition tokens are counted alongside messages (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:21-24, 45-95`).

## Tradeoffs

- **Dual systems raise maintenance cost.** The legacy `Summarizer` (message-count based, fire-and-forget writes to memory blocks) coexists with the token-aware `compact_messages` pipeline; `SummarizerSettings` itself carries comments saying the old fields "should be deprecated or moved" (`studies/agent-harness-study/sources/letta/letta/settings.py:88-91`). Behavior differs by agent class (v1/v2/voice vs. v3), which complicates reasoning about guarantees.
- **Cache compatibility vs. freshness.** Comments note they intentionally stopped refreshing the system prompt before compaction to leverage prefix caching for self mode (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:1236-1239`), accepting that compaction may operate on slightly stale system/memory state until the forced post-compaction rebuild (`letta_agent_v3.py:1253-1262`).
- **Fire-and-forget summarization (legacy/voice).** Static-buffer mode triggers background summarizer-agent tasks whose failures are logged but do not block or notify the main flow (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:124-134, 338-341`) — low latency, but silent quality loss if the background task dies.
- **Cheap-model delegation vs. fidelity.** Defaulting summarization to Haiku/GPT-5-mini/Gemini Flash saves cost (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_config.py:25-32`) but means the fidelity of preserved decisions/facts depends on a smaller model than the one doing the work; word caps (300/500) further compress (`studies/agent-harness-study/sources/letta/letta/prompts/summarizer_prompt.py:1-2`).
- **Approximate budgets.** Heuristic byte-based budgets (`context_window * 0.6 * 4`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:610`) and 1.3 multipliers trade precision for provider independence; the code acknowledges underestimation risk explicitly (`summarizer_sliding_window.py:21-24`).

## Failure Modes / Edge Cases

Handled:

- **Summarization doesn't shrink enough:** recount against `trigger_threshold`, retry once with `all` mode, then either raise `SystemPromptTokenExceededError` (system prompt alone exceeds window) or log critical and continue (`studies/agent-harness-study/sources/letta/letta/services/summarizer/compact.py:359-412`). Regression test locks this in: `test_v3_summarize_hard_eviction_when_still_over_threshold` simulates a summarization-loop scenario and asserts minimal-context output plus updated token estimate (`studies/agent-harness-study/sources/letta/tests/integration_test_summarizer.py:999-1105`).
- **Summarizer input too large:** two-stage degradation — clamp tool returns to 5000 chars, then middle-truncate the whole transcript to ~60% of the summarizer's window (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:564-647`).
- **Provider overload/rate limits during summarization:** transparent rerouting to Bedrock Opus (Anthropic) or Baseten GLM-5 (ZAI), skipped for BYOK configs, with original-error re-raise if the fallback also fails (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:714-816`).
- **Long Anthropic requests:** summarizer uses provider-side streaming to avoid >10-minute request failures (`summarizer.py:819-877`).
- **No valid cutoff point:** ValueError raised and translated into fallback-to-all semantics (`studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer_sliding_window.py:193-198`; consumed at `compact.py:327-346`).
- **Degenerate inputs:** `<2` messages short-circuits to "No conversation to summarize." (`self_summarizer.py:47-49`); endpoint rejects no-op compactions with HTTP 400 (`agents.py:2497-2502`).

Residual risks:

- **Post-failure continuation with oversized context:** when even fallback compaction stays above threshold, the agent continues anyway with a critical log (`compact.py:409-410`), deferring the failure to the next LLM call.
- **Duplicated protection logic:** the approval-protection rules appear independently in three files; divergence would create inconsistent eviction behavior (see Notable Patterns).
- **Silent loss in legacy path:** background summarizer failures only log (`summarizer.py:124-134`).
- **Sliding-window escalation cap:** if `eviction_percentage` reaches 1.0 without meeting the token goal, the function raises rather than forcing through (`summarizer_sliding_window.py:193`), relying on callers to fall back.

## Future Considerations

- Consolidate the legacy `Summarizer`/static-buffer machinery into `CompactionSettings` modes to eliminate dual maintenance (the codebase already flags this: `studies/agent-harness-study/sources/letta/letta/settings.py:88-91`, `studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:35` "NOTE: legacy, new version is functional").
- Centralize model-name classification and magic thresholds as urged by the existing TODO (`studies/agent-harness-study/sources/letta/letta/services/summarizer/thresholds.py:17-19`).
- Add semantic drift checks (e.g., fact-recall probes against evicted ranges) and summary-quality metrics alongside the existing `CompactionStats`.
- Make the deduplicated approval-protection rule a shared helper to prevent divergence across the three implementations.
- Surface background summarizer failures (voice/static path) to clients or retries instead of log-only handling.

## Questions / Gaps

- **Is there any offline/eval harness measuring summary quality?** No evidence found in `tests/` (searched for `summariz|compact` across test files); the integration suite validates structure, counts, and fallback behavior, not content fidelity.
- **Can users rebuild a summary spanning already-evicted history?** No evidence found: the `/summarize` handler operates only on `agent.message_ids` (`studies/agent-harness-study/sources/letta/letta/server/rest_api/routers/v1/agents.py:2445`); no endpoint reconstructs summaries from stored-but-evicted ranges.
- **How does the deprecated `package_summarize_message` (with hidden/recall counts) differ in practice from `..._no_counts`?** The counted variant exists (`studies/agent-harness-study/sources/letta/letta/system.py:190-204`) but its call sites were commented out ("TODO add counts back", `studies/agent-harness-study/sources/letta/letta/services/summarizer/summarizer.py:205-209`); recall-count reporting is currently absent.
- **What enforces `memory_warning_threshold` / `send_memory_warning_message` in the current pipeline?** Configured at `studies/agent-harness-study/sources/letta/letta/settings.py:98-103` with a `get_token_limit_warning` packager in `letta/system.py`, but the V3 loop's pre-execution proactive check is commented out (`studies/agent-harness-study/sources/letta/letta/agents/letta_agent_v3.py:365-379`); warning-message injection appears to be legacy-only.

---

Generated by `05.06-memory-compression-and-summarization` against `letta`.
