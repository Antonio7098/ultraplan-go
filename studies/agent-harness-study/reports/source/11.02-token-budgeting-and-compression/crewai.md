# Source Analysis: crewai

## Token Budgeting and Compression

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo: `lib/crewai` core, `lib/crewai-tools`, `lib/cli`, native LLM provider SDKs) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI's token budgeting is **reactive, estimate-based, and summarization-centric**. There is no pre-flight token accounting before an LLM call; instead, context overflow is detected after the fact by string-matching provider error messages (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/exceptions/context_window_exceeding_exception.py:4-13`). When overflow occurs and the agent has opted in via `respect_context_window`, the executor replaces the entire non-system conversation with a structured LLM-generated summary (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:795-832`, `:1048-1131`). Budget sizing comes from a per-model context-window table scaled by a 0.85 safety ratio (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llm.py:168`, `:325-326`, `:2450-2476`), overridable per LLM instance. The only in-conversation "ranking" of content happens outside the compaction path: memory recall uses a weighted semantic/recency/importance composite score, and knowledge retrieval filters by similarity threshold and top-k. Task-to-task context assembly does no budgeting at all — raw outputs are concatenated verbatim. Token *usage* (as opposed to budgeting) is well instrumented post-call through callbacks, per-LLM cumulative counters, and a normalized `UsageMetrics` model.

## Rating

**6 / 10** — Present but inconsistent and fragile in places. The overflow→summarize pipeline is clearly structured, cross-provider, and well unit-tested (chunking, file preservation, system-message preservation, headroom math), which earns it above the middle band. It falls short of the 7–8 band because: (a) nothing measures tokens before calling the model — the char//4 estimator is used only inside recovery, not as a gate; (b) the fallback for `respect_context_window=False` is a hard `SystemExit` rather than graceful degradation (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:830-832`); (c) there is no priority ranking or partial-drop strategy within the conversation — everything non-system is summarized wholesale; and (d) summary faithfulness is prompt-engineered but never verified.

## Evidence Collected

Every entry cites a workspace-relative file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Token counter (estimation) | `_estimate_token_count(text)` = `len(text) // 4`, described as a "conservative cross-provider heuristic"; no tiktoken/tokenizer dependency anywhere in core | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:835-844` |
| Token counter (usage tracking callback) | `TokenCalcHandler.log_success_event` sums prompt/completion/cached tokens from provider usage into `TokenProcess` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/token_counter_callback.py:37-66` |
| Token accumulator interface | `TokenProcess.sum_prompt_tokens / sum_completion_tokens / sum_successful_requests` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/agent_builder/utilities/base_token_process.py:17-28` |
| Per-LLM lifetime counters | `BaseLLM._track_token_usage_internal` accumulates into `self._token_usage`; `get_token_usage_summary()` returns cumulative `UsageMetrics` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llms/base_llm.py:955-986` |
| Usage normalization | `UsageMetrics.from_provider_dict` maps provider aliases (`prompt_tokens` / `prompt_token_count` / `input_tokens`, Anthropic cache reconciliation) into one schema | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/types/usage_metrics.py:111-189` |
| Budget config (per-model table) | `LLM_CONTEXT_WINDOW_SIZES` dict keyed by model-name prefix (gpt-4o: 128000, gemini-1.5-pro: 2097152, etc.) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llm.py:168-229+` |
| Budget safety ratio | `DEFAULT_CONTEXT_WINDOW_SIZE = 8192`, `CONTEXT_WINDOW_USAGE_RATIO = 0.85` ("using 75%..." docstring is stale vs. the actual 85% constant) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llm.py:2450-2476` |
| Budget configurable per LLM instance | `context_window_size: int = 0` field short-circuits the table lookup when non-zero (bounds validated 1024–2097152) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llm.py:390`, `:2456-2476` |
| Provider-specific budget overrides | Anthropic table + 200k default; Gemini 1M default; OpenAI/Azure/Bedrock equivalents — all × 0.85 ratio | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llms/providers/anthropic/completion.py:1926-1946`; `gemini/completion.py:1343-1377`; `openai/completion.py:2654-2690`; `azure/completion.py:1292-1322`; `bedrock/completion.py:2093-2121` |
| Base default budget | `BaseLLm.get_context_window_size()` returns `DEFAULT_CONTEXT_WINDOW_SIZE = 4096` for unknown custom LLMs | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llms/base_llm.py:75`, `:497-503` |
| Per-agent budget policy flag | `Agent.respect_context_window: bool = True` ("Keep messages under the context window size by summarizing content") propagated to executors | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agent/core.py:251-254`, `:1160`, `:1503`, `:1520` |
| Executor wiring of the flag | `CrewAgentExecutor.respect_context_window` field; passed to `handle_context_length` on every caught overflow | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:126`, `:444-456` (sync loop; also `:582-585`, `:1262-1265`, `:1381-1384`) |
| Overflow detection | `LLMContextLengthExceededError._is_context_limit_error` matches phrases like "maximum context length", "input is too long", "exceeds token limit" | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/exceptions/context_window_exceeding_exception.py:4-13`, `:32-44` |
| Detection helper + branch point | `is_context_length_exceeded(e)` wraps the phrase matcher; called from sync/native loops, lite agent, and experimental state machine | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:781-792`; `agents/crew_agent_executor.py:447`; `lite_agent.py:1002-1011`; `experimental/agent_executor.py:1477-1480`, `:1576-1578` |
| Truncation strategy (branch) | `handle_context_length`: summarize if `respect_context_window` else print guidance and `raise SystemExit(...)` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:795-832` |
| Litellm-path contract | `except LLMContextLengthExceededError: raise` — explicitly deferred to the executor so it can choose summarize-vs-abort | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llm.py:1909-1913` |
| Summarization entry point | `summarize_messages(messages, llm, ...)` mutates the list in place: keeps system messages, chunks, summarizes, appends one merged summary message | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1048-1131` |
| Chunking under budget | `_split_messages_into_chunks` packs messages into ≤max_tokens chunks at message boundaries using the estimator | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:955-992` |
| Oversized single message split | `_expand_oversized_message` slices one huge message into `[Part i/n]` sub-messages (reserving 5 tokens for the prefix) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:855-885` |
| Summary extraction | `_extract_summary_tags` pulls `<summary>...</summary>`, falls back to full text if tags missing | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:995-1009` |
| Parallel chunk summarization | >1 chunk → `asyncio.gather` over `_asummarize_chunks` (with event-loop-safe thread handoff); single chunk → serial `llm.call` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1012-1045`, `:1086-1119` |
| Faithfulness-oriented prompts | `summarizer_system_message` + 5-section `summarize_instruction` (Task Overview / Current State / Important Discoveries / Next Steps / Context to Preserve) wrapped in `<summary>` tags; final message instructs "Continue the task from where the conversation left off" | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/translations/en.json:25-27` |
| Lossy formatting during summarization | tool_calls → `"[Called tools: names]"`; multimodal blocks → `"[multimodal content]"` placeholder; system messages skipped | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:900-952` |
| File preservation across compaction | user-message `files` dicts merged and re-attached to the summary message | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1069-1072`, `:1129-1131` |
| Priority ranking (memory recall) | `compute_composite_score = semantic_w·similarity + recency_w·decay + importance_w·importance`, decay `0.5^(age_days/half_life)`; results sorted descending; defaults 0.5/0.3/0.2 weights, 30-day half-life | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:135-183`, `:345-380`; `memory/unified_memory.py:754-762` |
| Context injection limits (memory) | Agent injects top-5 recalled memories into task prompt; LiteAgent injects top-10 into system message | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agent/core.py:651-657`; `lite_agent.py:614-625` |
| Priority ranking (knowledge) | `Knowledge.query(results_limit=5, score_threshold=0.6)` — top-k similarity filtering | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/knowledge/knowledge.py:135-152`; threshold documented in `knowledge/knowledge_config.py:13-15` |
| No budgeting in task-context assembly | `aggregate_raw_outputs_from_task_outputs` joins full raw outputs with `\n\n----------\n\n` dividers — no truncation, ranking, or budget check | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/formatter.py:13-45`; consumed by `Crew._get_context` at `crew.py:1865-1874` |
| Output-side budgets | `max_tokens` / `max_completion_tokens` fields forwarded to providers (e.g., OpenAI chat & Responses params) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/llms/providers/openai/completion.py:230-231`, `:1810-1813`, `:867-869`; Anthropic default 4096 at `anthropic/completion.py:222` |
| Telemetry-only truncation | `truncate_messages` caps traces at 5 messages × 500 chars — affects observability payloads, not model context | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/listeners/tracing/utils.py:426-441` |
| Tests: estimator & chunking | Unit tests assert empty/short/long estimates, chunk packing ≤ budget, oversized tool-output splitting, `[Part i/n]` metadata preservation, files preserved through expansion | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/utilities/test_agent_utils.py:702-728`, `:771-833`, `:868-882` |
| Tests: headroom invariant | Chunked summarizer request must fit the *raw* (un-ratio'd) limit because `get_context_window_size` already applied the 85% ratio | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/utilities/test_agent_utils.py:730-749` |
| Tests: compaction behavior | System messages preserved, files merged/preserved, in-place mutation, summary-tag extraction, serial vs parallel paths | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/utilities/test_agent_utils.py:336-560`, `:904-1030` |
| Tests: integration across providers | `test_summarize_integration.py` exercises `summarize_messages` against OpenAI, Anthropic, Gemini, Azure (live-marker gated) | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/utilities/test_summarize_integration.py:86-266` |
| Tests: abort branch | `test_handle_context_length_exceeds_limit_cli_no` covers the `respect_context_window=False` → no-summarize path | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/agents/test_agent.py:1477-1505` |
| Window-size tests per provider | e.g., Anthropic claude-3-5-sonnet → 170000 (200000×0.85); Azure gpt-4 vs gpt-4o | `studies/agent-harness-study/sources/crewai/lib/crewai/tests/llms/anthropic/test_anthropic.py:462`; `tests/test_llm.py:333-342` |

## Answers to Dimension Questions

**1. Is token usage measured before calling the model?**
Not as a gate. The only pre-call measurement is the crude `len(text)//4` character estimator, and it runs exclusively *inside* the recovery flow to size summarization chunks (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:835-844`, `:1080-1081`). Nothing inspects the outbound message list against `get_context_window_size()` before issuing an LLM call; the normal trigger is catching a provider error after the call fails (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:444-456`). Post-call usage *is* measured thoroughly via `TokenCalcHandler` (`token_counter_callback.py:37-66`) and `BaseLLM._track_token_usage_internal` (`base_llm.py:955-971`), but these are accounting, not budgeting.

**2. What gets dropped when budget is exceeded?**
Nothing is selectively dropped — it is all-or-nothing replacement. With `respect_context_window=True`, every non-system message (user turns, assistant turns, tool results) is replaced by a single merged summary message; system messages are kept verbatim and attached files are re-attached (`agent_utils.py:1069-1131`). Within the summarization formatter, tool calls degrade to `"[Called tools: <names>]"` and multimodal blocks to `"[multimodal content]"` (`agent_utils.py:918-940`) — those are the real information losses. With `respect_context_window=False`, the run terminates via `SystemExit` (`agent_utils.py:824-832`). Outside the agent loop, task-context aggregation drops nothing and caps nothing (`utilities/formatter.py:16-26`).

**3. Is summarization faithful?**
It is engineered for faithfulness but unverifiable. The instruction prompt mandates five sections including "Important Discoveries" and "Context to Preserve" with exact values/URLs/code snippets (`translations/en.json:26`), and `<summary>` tag extraction guards output shape (`agent_utils.py:995-1009`). However: fidelity depends entirely on the same (possibly strained) LLM doing the summarizing; there is no verification pass, no coverage check against source messages, and structural details are already flattened before summarization (`tool_calls` → name list, multimodal → placeholder). Multi-chunk summaries are naive concatenations joined by blank lines (`agent_utils.py:1121`), which can duplicate or contradict across chunk boundaries.

**4. Is budget configurable?**
Yes, at several levels, though there is no unified "token budget" API. Per-model windows come from `LLM_CONTEXT_WINDOW_SIZES` (`llm.py:168+`) with provider-specific tables overriding for native SDK paths (`anthropic/completion.py:1926-1946`, etc.). Per-LLM-instance override is available via the `context_window_size` field (`llm.py:390`, `:2458-2459`). Per-agent policy is `respect_context_window` (`agent/core.py:251-254`). Output length is separately capped by `max_tokens`/`max_completion_tokens` per provider config. Memory recall weights (semantic/recency/importance, half-life) are configurable through `MemoryConfig` (`memory/types.py:135-183`). What is *not* configurable: a proactive input budget (reserve-for-response margin beyond the fixed 0.85 ratio), or per-section priorities within the conversation history.

## Architectural Decisions

1. **Recovery over prevention.** CrewAI deliberately treats overflow as an exception-handling concern: the litellm path re-raises `LLMContextLengthExceededError` specifically so the executor decides the response (`llm.py:1909-1913`), and the experimental executor models overflow as a first-class `"context_error"` state routed to a recovery node (`experimental/agent_executor.py:1477-1480`, `:2786-2800`). This avoids maintaining an always-correct tokenizer but accepts paying one failed API call.
2. **Estimation instead of tokenization.** A single shared `len//4` estimator serves chunk sizing everywhere (`agent_utils.py:835-844`). This removes tokenizer dependencies and per-model tokenizer selection, trading accuracy for portability — mitigated by the 15% headroom from `CONTEXT_WINDOW_USAGE_RATIO = 0.85` and a test asserting the summarizer request fits the raw limit (`tests/utilities/test_agent_utils.py:730-749`).
3. **Structured-compaction design.** Rather than dropping oldest messages (sliding window), the whole dialogue becomes a role-labeled, section-structured summary with system prompts and attachments preserved (`agent_utils.py:1048-1131`; `en.json:25-27`). This preserves task continuity semantics over recency heuristics.
4. **Budget knowledge lives in the LLM layer, policy lives in the Agent layer.** Window sizes are methods on LLM classes (per-model), while the summarize-vs-abort decision belongs to the agent/executor (`agent/core.py:251-254` → executor field `crew_agent_executor.py:126`). Clean separation, but the two defaults disagree (Agent=True, executor field default=False), so correctness depends on constructor wiring (`agent/core.py:1160`).
5. **Ranking delegated to retrieval subsystems, not to the transcript.** Composite-scored memory recall (`memory/types.py:345-380`) and thresholded knowledge search (`knowledge/knowledge.py:136-152`) decide what enters context; once in the transcript, no further prioritization exists.

## Notable Patterns

- **Phrase-list error classification**: a portable, provider-agnostic overflow detector fed by a curated list of vendor error strings (`context_window_exceeding_exception.py:4-13`), reused by every native provider and both executors.
- **Event-loop-safe parallelism**: multi-chunk summarization uses `asyncio.gather`, with a thread-pool + `contextvars.copy_context` escape hatch when already inside a running loop (`agent_utils.py:1113-1119`) — the same pattern used elsewhere in the codebase (`agent_utils.py:123-128`).
- **In-place mutation contract**: `summarize_messages` mutates the caller's message list (asserted by tests, `tests/utilities/test_agent_utils.py:419-440`), keeping the executor loop simple (`continue` after handling).
- **Attachment survival**: multimodal `files` are hoisted out before compaction and reattached to the summary message (`agent_utils.py:1069-1072`, `:1129-1131`) — a small but easy-to-miss detail covered by dedicated tests (`test_agent_utils.py:338-396`).
- **Delta accounting**: `UsageMetrics.delta_since` enables per-call attribution against monotonic lifetime counters, clamped at zero to survive accumulator resets (`types/usage_metrics.py:79-109`).

## Tradeoffs

- **No pre-flight check ⇒ wasted calls and latency**: every overflow costs a failed round-trip plus a potentially slow multi-chunk summarization pass before work resumes (the code even warns "Might take a while...", `agent_utils.py:818`).
- **Char//4 accuracy vs. dependency-free portability**: underestimates for CJK/emoji-heavy text, overestimates for code/whitespace-dense text; only the fixed 15% headroom stands between estimation error and another overflow cycle.
- **Whole-transcript summarization vs. sliding window**: preserves global task coherence but destroys verbatim detail (tool outputs become names only), and cost scales with total history since every message is re-summarized.
- **Hard exit alternative**: `SystemExit` (`agent_utils.py:830-832`) makes `respect_context_window=False` a cliff rather than a degradation strategy (e.g., truncate-and-continue).
- **Centralized table maintenance**: per-model window sizes are hand-curated constants across six files (`llm.py:168+` plus five provider modules); unknown/newer models silently fall back to small defaults (8192×0.85 on the litellm path, 4096 on BaseLLM), which triggers unnecessary summarization or spurious exits.

## Failure Modes / Edge Cases

- **Summarization-of-summary loop**: if the compacted conversation still exceeds the window (or the estimator under-counts), the next call fails again, `handle_context_length` fires again, and the previous summary gets re-summarized each iteration until iteration limits intervene (`crew_agent_executor.py:444-460`); there is no explicit "already summarized" guard or shrinking-target strategy.
- **Summary call failure propagates**: exceptions raised by the summarizer LLM call are not caught within `summarize_messages`; they bubble into the generic error handler and abort the task.
- **Multimodal loss**: image/audio/PDF content collapses to the literal string `"[multimodal content]"` during summarization (`agent_utils.py:934-940`); only top-level `files` attachments survive, not inline content blocks.
- **List-content estimation skew**: `_message_content_text` stringifies list-typed (multimodal) contents whole, counting JSON scaffolding as prose characters (`agent_utils.py:847-852`; acknowledged in tests, `tests/utilities/test_agent_utils.py:763-768`).
- **Default mismatch trap**: `CrewAgentExecutor.respect_context_window` defaults to `False` (`crew_agent_executor.py:126`) while `Agent.respect_context_window` defaults to `True` (`agent/core.py:251-252`); any execution path that builds an executor without copying the agent value silently switches to the abort-on-overflow policy.
- **Stale documentation**: `get_context_window_size`'s docstring says "75%" while the constant and all provider implementations use 85% (`llm.py:2452` vs `llm.py:326`) — a minor but real hazard for anyone reasoning about headroom.
- **Unbudgeted inter-task context**: large upstream task outputs concatenated into the next task's prompt (`formatter.py:16-26`, `crew.py:1865-1874`) can push the *first* call of a task past its window before any per-turn machinery ever engages.

## Future Considerations

- Add a pre-call guard: estimate the assembled message list against `get_context_window_size()` minus an output reserve, invoking `summarize_messages` (or a lighter drop policy) before the first failed attempt.
- Replace or augment `len//4` with an optional tokenizer-backed counter (tiktoken or provider tokenizers) behind the existing `BaseLLM` interface, keeping the heuristic as fallback for custom LLMs.
- Introduce graded degradation for `respect_context_window=False` (e.g., truncate oldest tool observations with markers) instead of `SystemExit`.
- Track compaction events as observability signals (count, chunks, chars-before/after) alongside existing `UsageMetrics` so overflow frequency and summary quality drift are visible operationally.
- Verify summary fidelity cheaply — e.g., require key entities/tool names from the source chunk to appear in the summary, with one bounded retry.
- Unify context-window constants into one data source shared by the litellm table and the five native provider tables to stop default drift for new models.

## Questions / Gaps

- **Is there any proactive budget check elsewhere?** Searched for `get_context_window_size` callers, `estimate`, `count_tokens`, and tiktoken across `lib/crewai/src`: the estimator is referenced only within `agent_utils.py` (summarization internals). No evidence found of any pre-send measurement gating normal LLM calls.
- **Does the experimental executor differ semantically?** Its state-machine routes overflow identically into `handle_context_length` (`experimental/agent_executor.py:2786-2800`); no additional compression strategy was found beyond the shared helper.
- **Are summaries persisted or auditable?** No evidence found of summaries being logged, emitted as events, or stored; they exist transiently inside the mutated message list. Searches over `events/types` surfaced memory-retrieval and usage events but no compaction/summary events.
- **Does anything bound how many times compaction can recur in one task?** Only the generic `max_iterations` machinery appears relevant; no compaction-specific cap was found in either executor loop.
- Search boundary: analysis confined to `studies/agent-harness-study/sources/crewai` (primarily `lib/crewai/src/crewai` and `lib/crewai/tests`); frozen docs snapshots under `docs/v*/` were not treated as implementation evidence.

---

Generated by dimension 11.02 (Token Budgeting and Compression) against `crewai`.
