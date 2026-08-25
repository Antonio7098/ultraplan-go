# Source Analysis: letta

## Dimension 11.01: Context Selection Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas, async agent loops) |
| Analyzed | 2026-08-25 |

> Citation convention: all file paths below are relative to the repository root `studies/agent-harness-study/sources/letta/` (e.g., `letta/services/summarizer/summarizer.py:104`).

## Summary

Letta implements context selection as a **two-layer, explicitly materialized policy**:

1. **System prompt (layer 1)** — a persisted, recompilable system message (`message_ids[0]`) assembled by templating the base system prompt with a compiled memory string. The memory string is rendered from core memory blocks, tool-usage rules, and an optional `<directories>` section of attached files/sources (`Memory.compile`, `letta/schemas/memory.py:688-732`), plus a `<memory_metadata>` block that advertises out-of-context resources (recall message count, archival count, archive tags) so the model knows what it can retrieve (`PromptGenerator.compile_memory_metadata_block`, `letta/prompts/prompt_generator.py:26-89`). Assembly entry points: `compile_system_message` (`letta/services/helpers/agent_manager_helper.py:251-332`), `PromptGenerator.get_system_message_from_compiled_memory` (`letta/prompts/prompt_generator.py:107-177`), and the system-prompt rebuild service `AgentManager.rebuild_system_prompt_async` (`letta/services/agent_manager.py:1523-1612`).

2. **In-context message list (layer 2)** — an ordered, DB-backed list of message IDs stored on agent state (`agent_state.message_ids`). Every step loads exactly this list via `_prepare_in_context_messages_no_persist_async`; if `message_buffer_autoclear` is set, only the system message is loaded (`letta/agents/helpers.py:81-86`, `letta/agents/helpers.py:187-192`, `letta/agents/helpers.py:213-220`). The window is mutated only through explicit operations: `update_message_ids_async` (`letta/services/agent_manager.py:999`), `set_in_context_messages_async` (`letta/services/agent_manager.py:1622`), and trim helpers (`letta/services/agent_manager.py:1627-1638`).

Eviction from that list is governed by a configurable compaction engine with explicit thresholds and multiple modes (`CompactionSettings`: `all`, `sliding_window`, `self_compact_all`, `self_compact_sliding_window`, `letta/services/summarizer/summarizer_config.py:48-89`). The newest loop (v3) triggers compaction proactively after each step when the observed token estimate exceeds `context_window * SUMMARIZATION_TRIGGER_MULTIPLIER` (0.9) (`letta/constants.py:82-83`, `get_compaction_trigger_threshold`, `letta/services/summarizer/thresholds.py:27-41`, check at `letta/agents/letta_agent_v3.py:1439-1474`) and reactively on `ContextWindowExceededError` mid-request (`letta/agents/letta_agent_v3.py:1218-1284`). Older loops (v1) compact only reactively via `_handle_llm_error` → `_rebuild_context_window` (`letta/agents/letta_agent.py:1550-1618`).

The model itself can influence what it sees through first-class tools — memory-block editors, `conversation_search`, `archival_memory_search`, and file open/close tools — and is told what exists outside the window via the metadata block.

Sensitive-data redaction before inclusion is the weakest area: secrets are protected at rest/in-memory and routed only to sandbox execution environments, but there is no PII/secret scanner on user messages or tool outputs entering the model context.

## Rating

**Score: 8 / 10**

Rationale:
- The selection policy is **explicit and inspectable**: the in-context window is a concrete artifact (an ordered message-ID list), not an implicit side effect; eviction thresholds, modes, and percentages are configuration (`letta/settings.py:74-99`, `letta/services/summarizer/summarizer_config.py:76-82`).
- There are **tests** for threshold behavior (`tests/test_compaction_thresholds.py:5-49`), context-window section parsing (`tests/test_context_window_calculator.py:11-380`), and buffer-trim edge cases including user-message boundary alignment (`tests/test_static_buffer_summarize.py:41-133`).
- **Operational safeguards** exist: layered fallback chains during compaction (`letta/services/summarizer/compact.py:192-346`), post-compaction token verification against the trigger threshold with a dedicated `SystemPromptTokenExceededError` (`letta/services/summarizer/compact.py:359-407`), protection invariants for pending approval requests (`letta/services/summarizer/self_summarizer.py:265-289`), and observability via a per-section token breakdown API (`ContextWindowOverview`, `letta/schemas/memory.py:23-65`) plus structured compaction stats (`letta/services/summarizer/compact.py:414-432`).
- It falls short of 9–10 because: three coexisting agent-loop generations apply different effective policies (v1 reactive-only; v2 summarizer marked deprecated at `letta/agents/letta_agent_v2.py:1361`; v3 proactive+reactive); change detection relies on substring matching with acknowledged correctness gaps (`letta/services/agent_manager.py:1562-1563`); several code paths are commented-out or flagged "doing nothing?" (`letta/agents/letta_agent.py:1602`, `letta/agents/letta_agent_v3.py:365-410`); and there is no sensitive-content filtering before inclusion.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| System-prompt templating | Reserved `{CORE_MEMORY}` variable; missing-variable auto-append; safe_format preserving unknown vars | `letta/services/helpers/agent_manager_helper.py:251-332` |
| System prompt selection by agent type | `derive_system_message` maps AgentType → canned prompt (memgpt_v2_chat, sleeptime_v2, react, letta_v1, workflow, voice) | `letta/services/helpers/agent_manager_helper.py:164-226` |
| Memory compilation into prompt | `Memory.compile()` renders blocks, tool rules, directories/files; line-numbered variant only for Anthropic + specific agent types | `letta/schemas/memory.py:688-732` (line-number policy 696-702) |
| Out-of-context metadata injection | `<memory_metadata>` lists recall message count, archival count, archival tags — signals for tool-based retrieval | `letta/prompts/prompt_generator.py:26-89` (lines 74-85) |
| In-context window = explicit ID list | `_prepare_in_context_messages_no_persist_async` loads `agent_state.message_ids`; autoclear keeps only system message; conversation-scoped loading path | `letta/agents/helpers.py:177-220` |
| Window mutation API | `update_message_ids_async`, `set_in_context_messages_async`, `trim_older_in_context_messages`, `trim_all_in_context_messages_except_system` | `letta/services/agent_manager.py:999, 1622-1638` |
| Proactive compaction trigger | Post-step check: `context_token_estimate > get_compaction_trigger_threshold(...)` → `self.compact(...)` with trigger `"post_step_context_check"` | `letta/agents/letta_agent_v3.py:1439-1474` |
| Reactive compaction trigger | On `ContextWindowExceededError` retry loop with trigger `"context_window_exceeded"`, bounded by `max_summarizer_retries` | `letta/agents/letta_agent_v3.py:1218-1284` |
| Threshold definition | 0.9 × context_window multiplier ("avoid 'too many tokens in prompt' fallbacks"); GPT-5 proactive rationale | `letta/constants.py:82-83`; `letta/services/summarizer/thresholds.py:27-41` |
| Sliding-window eviction algorithm | Eviction % grows in +0.10 steps until post-summarization tokens ≤ goal ((1−pct)×window); cutoff must be assistant/approval message | `letta/services/summarizer/summarizer_sliding_window.py:144-198` |
| Static buffer mode | Trim to `message_buffer_min` keeping user-role boundaries; pairs approval+assistant sharing `step_id`; fire-and-forget background summarizer | `letta/services/summarizer/summarizer.py:244-343` |
| Partial-evict mode | Evicts 30% of messages, inserts recursive summary at index 1 as user role | `letta/services/summarizer/summarizer.py:136-242` |
| Compaction orchestration + fallbacks | Mode dispatch with sliding→all and self→sliding→all fallback chains; post-check vs `trigger_threshold`; `SystemPromptTokenExceededError` | `letta/services/summarizer/compact.py:180-412` |
| Compaction settings surface | `CompactionSettings`: mode enum, `sliding_window_percentage` (default from settings 0.30), `clip_chars=50000`, per-mode default prompts, provider-default summarizer models (haiku/gpt-5-mini/gemini-flash) | `letta/services/summarizer/summarizer_config.py:11-45, 48-89` |
| Protected messages invariant | Pending approval request never evicted; assistant+approval sharing `step_id` protected together | `letta/services/summarizer/self_summarizer.py:265-289`; also `summarizer_sliding_window.py:133-137, 155-161` |
| Summary clipping | Summary truncated to `clip_chars` with `SUMMARY_TRUNCATION_SUFFIX = "... [summary truncated to fit]"` | `letta/services/summarizer/summarizer_sliding_window.py:227-229`; `letta/services/summarizer/constants.py:1` |
| Token counting strategies | Per-provider token counters; approximate bytes/4 heuristic with `APPROX_TOKEN_SAFETY_MARGIN = 1.3`; tools counted separately (`count_tokens_with_tools`) | `letta/services/summarizer/summarizer_sliding_window.py:21-95` |
| Summarizer transcript hygiene | Tool returns clamped to `TOOL_RETURN_TRUNCATION_CHARS=5000` on overflow fallback; images replaced by `[Image omitted]`; send_message tool noise filtered | `letta/services/summarizer/summarizer.py:571-575, 654-711`; `letta/constants.py:443` |
| Model-initiated retrieval (recall) | `conversation_search` tool → hybrid vector+FTS search w/ roles/date/limit; default page size 5; filters tool-role results to prevent recursive nesting; returns relevance metadata (rrf_score, ranks) | `letta/functions/function_sets/base.py:87-161`; `letta/services/tool_executor/core_tool_executor.py:81-276`; `letta/constants.py:458`; search impl `letta/services/message_manager.py:1142-1260` |
| Model-initiated retrieval (archival) | `archival_memory_search` with tags, tag_match_mode any/all, top_k, datetime range → semantic ranking | `letta/services/tool_executor/core_tool_executor.py:278-305`; schema `letta/functions/function_sets/base.py:194-243` |
| Model-edited in-context memory | `core_memory_append/replace`, `memory_insert/replace/rethink/apply_patch` mutate blocks injected into system prompt; read-only blocks enforced | `letta/services/tool_executor/core_tool_executor.py:41-56, 319-360`; `letta/schemas/block.py:36` |
| File selection into context | `FileBlock` open/closed status, char limit per block, `max_files_open` budget advertised in `<file_limits>` | `letta/schemas/block.py:107-114`; rendering `letta/schemas/memory.py:588-636` |
| Prefix-cache-aware rebuild skipping | System prompt rebuilt only when forced or when memory/system text changed (avoids cache busting from dynamic counters) | `letta/agents/letta_agent_v2.py:760-792`; `letta/agents/base_agent.py:130-183` |
| Inner-thought scrubbing | Assistant TextContent removed from history when reasoning fully disabled ("presenting clean message history") | `letta/helpers/reasoning_helper.py:25-48`; applied at `letta/agents/letta_agent.py:1660-1661`, `letta/agents/letta_agent_v3.py:965-971` |
| Tool-return truncation policy | `validate_function_response` truncates to per-tool `return_char_limit`; retrieval tools exempted from truncation | `letta/utils.py:898-938`; `letta/agents/letta_agent_v3.py:1877-1885` |
| Secrets kept out of prompt path | `Secret` encrypted wrapper (falls back to plaintext w/o key, warns); decrypted values passed only as sandbox env vars to `ToolExecutionManager` | `letta/schemas/secret.py:13-69`; `letta/agents/letta_agent.py:1950-1959` |
| Observability API | `ContextWindowOverview` per-section token breakdown (system_prompt, core_memory, memory_filesystem, tool_usage_rules, directories, external_memory_summary, summary_memory, messages, tool defs); REST endpoint | `letta/schemas/memory.py:23-65`; calculator `letta/services/context_window_calculator/context_window_calculator.py:167-384`; endpoint `letta/server/rest_api/routers/v1/agents.py:601` |
| Compaction telemetry/stats | `EventMessage(event_type="compaction")` with trigger + estimates; summary carries `compaction_stats` (trigger, tokens/messages before/after, mode); LLM-call tagging `call_type=summarization` | `letta/agents/letta_agent_v3.py:820-892`; `letta/services/summarizer/compact.py:414-432`; `letta/services/summarizer/summarizer.py:517-530` |
| Autoclear option | `message_buffer_autoclear=False` documented: agent "will not remember previous messages... still retain state via core memory blocks and archival/recall memory" | `letta/schemas/agent.py:139-142` |
| Background memory curation | Sleeptime agents rewrite shared blocks off-thread; frequency configurable per group (`sleeptime_agent_frequency`); prompt mandates selective, dated memory edits | `letta/agents/voice_sleeptime_agent.py:30-181`; `letta/services/group_manager.py:94-101`; `letta/prompts/system_prompts/sleeptime_v2.py:3-28` |
| Tests | Threshold tests (GPT-5 90% vs others 100%, force_proactive); ~30 tag-extraction tests; static-buffer trim tests incl. no-assistant-message and JSON-failure cases | `tests/test_compaction_thresholds.py:5-49`; `tests/test_context_window_calculator.py:11-380`; `tests/test_static_buffer_summarize.py:41-133` |

## Answers to Dimension Questions

**1. What decides what goes into context?**
Three deterministic inputs decide: (a) the persisted system message, recompiled from the current memory state whenever blocks/tool-rules/system prompt change or when compaction forces it (`letta/agents/base_agent.py:93-186`, `letta/agents/letta_agent_v3.py:1253-1262`, `letta/services/agent_manager.py:1523-1612`); (b) the ordered `message_ids` list defining which conversation messages are in-window (`letta/agents/helpers.py:206-220`); and (c) the compaction engine, which decides eviction order (oldest-first up to an assistant/approval-boundary cutoff) and writes a summary message at position 1 (`letta/services/summarizer/compact.py:462-465`, `letta/services/summarizer/summarizer_sliding_window.py:200-232`). New user input is always appended; it is never filtered by relevance. Retrieved content enters only as tool-return messages from explicit `conversation_search`/`archival_memory_search` calls or file-open tools.

**2. Is selection policy explicit or implicit?**
Mostly explicit. The window is a first-class persisted object with mutation APIs (`letta/services/agent_manager.py:999, 1616-1638`), thresholds are named constants and typed config (`letta/constants.py:82-83`, `letta/services/summarizer/summarizer_config.py:48-89`), and each compaction run records its trigger (`"post_step_context_check"` / `"context_window_exceeded"` at `letta/agents/letta_agent_v3.py:1233, 1455`). Implicit/ad-hoc elements remain: substring containment checks for rebuild-skipping (`letta/services/agent_manager.py:1562-1563`, self-flagged risk), approximate token heuristics with a magic 1.3 safety margin (`letta/services/summarizer/summarizer_sliding_window.py:21-24`), and hardcoded model-family special-casing (`letta/services/summarizer/thresholds.py:14-24`).

**3. Can the model influence what it sees?**
Yes — this is Letta's signature design. The model can (a) rewrite its own system-prompt memory via `core_memory_append/replace`, `memory_insert/replace/rethink/apply_patch` (`letta/services/tool_executor/core_tool_executor.py:46-55`); (b) pull recall/archival content into context via `conversation_search` (hybrid search, role/date filters, `letta/services/tool_executor/core_tool_executor.py:81-276`) and `archival_memory_search` (tag/datetime/top-k semantic search, `letta/services/tool_executor/core_tool_executor.py:278-305`); and (c) manage file blocks whose contents render inside `<directories>` (`letta/schemas/memory.py:609-633`). To make these requests well-informed, the system prompt embeds counts of hidden history and archival memories plus available archive tags (`letta/prompts/prompt_generator.py:68-87`). Additionally, a separate sleeptime agent can curate shared blocks between conversations (`letta/agents/voice_sleeptime_agent.py:153-181`, `letta/services/group_manager.py:94-101`).

**4. Are sensitive fields redacted?**
Not systematically. Searches for redaction/PII filtering across ingestion paths (`create_input_messages`, `package_user_message`, tool-result handling) found no content scanners. What exists instead: (a) encryption-at-rest/in-memory for credentials via the `Secret` wrapper with plaintext fallback warnings (`letta/schemas/secret.py:34-69`); (b) secrets injected only into sandbox env vars for tool execution, not into prompts (`letta/agents/letta_agent.py:1950-1959`); (c) structural hygiene rather than security filtering — inner-thought scrubbing (`letta/helpers/reasoning_helper.py:25-48`), image placeholders in summaries (`letta/services/summarizer/summarizer.py:683-687`), tool-message exclusion from `conversation_search` results to stop recursive nesting (`letta/services/tool_executor/core_tool_executor.py:151-165`), and size caps on tool returns (`letta/utils.py:918-937`). If a tool echoes an environment secret into its return value, that value would enter context unredacted.

## Architectural Decisions

1. **Context as durable state, not per-request reconstruction.** The in-context window is stored (`agent_state.message_ids`) and updated transactionally after steps (`letta/agents/letta_agent_v3.py:1402-1410`), enabling resume, inspection, and manual curation via REST (`letta/server/rest_api/routers/v1/agents.py:601`, `letta/server/rest_api/routers/v1/conversations.py:1060-1073`).
2. **Summarize-in-place with recursive summaries.** Evicted ranges are replaced by a single summary message near the front of the sequence (`[system, summary, *tail]`, `letta/services/summarizer/compact.py:462-465`; legacy index-1 insertion at `letta/services/summarizer/summarizer.py:241-242`), keeping full history queryable in storage.
3. **Dual-trigger compaction (proactive 90% + reactive on error).** The v3 loop checks after each step against `context_window * 0.9` (`letta/agents/letta_agent_v3.py:1438-1443`) and additionally catches `ContextWindowExceededError` to compact-and-retry within `max_summarizer_retries` (`letta/agents/letta_agent_v3.py:1218-1222`).
4. **Cache-conscious recomputation.** System prompts are deliberately *not* recompiled on normal turns to preserve provider prefix caching; dynamic counters live in a separate metadata section and rebuilds are skipped unless content actually changed (`letta/agents/letta_agent_v2.py:777-792`, note at `letta/agents/letta_agent_v3.py:965-967`).
5. **Self-summarization mode reusing the agent's own model/cache.** `self_compact_*` modes call the agent's LLM with the same tools/params for cache compatibility and parse the reply as the summary (`letta/services/summarizer/self_summarizer.py:99-118`).
6. **Structured observability of composition.** Section-tag parsing (`<memory_blocks>`, `<directories>`, etc.) converts the prompt back into a tokenized breakdown exposed over the API (`letta/services/context_window_calculator/context_window_calculator.py:167-211, 249-384`).

## Notable Patterns

- **Boundary-respecting eviction:** every eviction strategy snaps its cutoff to assistant (or approval-with-tool-calls) messages so the retained suffix starts at a valid turn boundary, growing eviction by 10% increments until the token goal is met (`letta/services/summarizer/summarizer_sliding_window.py:156-198`).
- **Invariants under eviction:** pending approval requests are un-evictable, and assistant+approval messages sharing a `step_id` are treated as one atomic unit (`letta/services/summarizer/self_summarizer.py:265-289`; mirrored in static mode at `letta/services/summarizer/summarizer.py:296-304`).
- **Degradation ladder with verification:** mode failures cascade (self→sliding→all; sliding→all), then the result is re-counted against `trigger_threshold`, with one more `all`-mode attempt before raising `SystemPromptTokenExceededError` if the system prompt alone is oversized (`letta/services/summarizer/compact.py:274-296, 359-407`).
- **Provider-aware summarizer defaults:** cheap models per provider (Haiku 4.5 / gpt-5-mini / Gemini Flash) with overload fallbacks to Bedrock/Baseten (`letta/services/summarizer/summarizer_config.py:26-32`, `letta/services/summarizer/summarizer.py:720-816`).
- **Context-budget signaling to the model:** `<file_limits>` reports `current_files_open` vs `max_files_open`, and each file block exposes `chars_current` vs `chars_limit` — the model can see how full its own context slots are (`letta/schemas/memory.py:589-633`).
- **Anti-recursion retrieval filter:** `conversation_search` drops all tool-role messages and prior `conversation_search` call sites from results, preventing exponential nesting of past searches (`letta/services/tool_executor/core_tool_executor.py:151-165`).

## Tradeoffs

- **Fidelity vs. window pressure:** summarization discards verbatim history in-window (recoverable only via `conversation_search`), and summaries themselves are clipped to 50k chars (`letta/services/summarizer/summarizer_config.py:72-74`) — information loss is traded for guaranteed progress.
- **Approximate token accounting:** bytes/4 estimation with a blanket 1.3 margin is fast and provider-agnostic but can both under- and over-trigger compaction; exact counting exists but is reserved for specific providers (`letta/services/context_window_calculator/token_counter.py`, used via `create_token_counter` at `letta/services/summarizer/summarizer_sliding_window.py:27-42`).
- **Substring change detection vs. DB cost:** skipping rebuilds when `curr_memory_str in curr_system_message_text` saves work but can miss deletions when old content remains a substring — acknowledged inline (`letta/services/agent_manager.py:1562-1563`).
- **Prefix caching vs. freshness:** deferring system-prompt rebuilds means memory edits made mid-run may lag one step behind what the model sees (`letta/agents/letta_agent_v3.py:1236-1239` comment).
- **Autoclear simplicity vs. amnesia:** `message_buffer_autoclear=True` gives a clean single-turn window but relies entirely on blocks/tools for continuity — flagged "not recommended unless advanced use case" in the schema itself (`letta/schemas/agent.py:139-142`).

## Failure Modes / Edge Cases

- **Bad configs break the floor:** `_rebuild_context_window` notes it "can be broken by bad configs, e.g. lower bound too high, initial messages too fat" (`letta/agents/letta_agent.py:1587`); sliding-window asserts `percentage <= 1.0` and raises if no valid cutoff exists, punting to full summarization (`letta/services/summarizer/summarizer_sliding_window.py:149, 193-198`).
- **Compaction cannot converge:** if post-compaction tokens remain ≥ threshold after fallbacks, the system logs critical and continues rather than bricking the agent (`letta/services/summarizer/compact.py:409-412`) — the next step will likely re-trigger compaction.
- **Summarizer payload overflow during summarization:** three-stage fallback — clamp tool returns to 5000 chars, then middle-truncate transcript to ~60% of context window in bytes, then propagate the error (`letta/services/summarizer/summarizer.py:562-647`).
- **Approval-flow hazards:** evicting a pending approval breaks clients, hence hard protection; conversely, headless flows allow approval-as-cutoff only when tool calls group correctly (`letta/services/summarizer/summarizer_sliding_window.py:133-137, 155-161`).
- **Missing/corrupt system message:** None-messages are filtered with a warning during window calculation, indicating corrupted message data surfaces here first (`letta/services/context_window_calculator/context_window_calculator.py:271-278`).
- **Legacy/v2 dead paths:** v2's `summarize_conversation_history` logs "Running deprecated v2 summarizer" (`letta/agents/letta_agent_v2.py:1361`), and v1's else-branch summarize call is annotated "Seems like this is doing nothing?" (`letta/agents/letta_agent.py:1602-1605`) — stale code paths risk divergent behavior between loops.
- **Secret leakage via tool output:** no output scanning exists; a tool returning env-var values would place them in context and persist them in recall storage searchable by `conversation_search`.

## Future Considerations

- Consolidate on the v3 loop's unified dual-trigger policy and delete v1/v2 summarization paths to remove behavioral divergence (`letta/agents/letta_agent.py:1576-1618`, `letta/agents/letta_agent_v2.py:1352-1410`).
- Replace substring containment checks with hash- or diff-based change detection for system-prompt rebuild decisions (`letta/agents/base_agent.py:141-143`).
- Add an optional sensitive-content filter stage on user input and tool returns before persistence (the natural seam is `validate_function_response` at `letta/utils.py:898` and `create_input_messages` at `letta/server/rest_api/utils.py:159`).
- Record per-message inclusion provenance (e.g., "kept by sliding-window cutoff", "retrieved by archival_memory_search#id") alongside the existing section-level breakdown to answer inclusion questions at item granularity (`letta/services/context_window_calculator/context_window_calculator.py:354-384`).
- Centralize model-name classification already flagged as scattered (`letta/services/summarizer/thresholds.py:17-19`) so threshold policies extend to new model families without regex scatter.

## Questions / Gaps

- **Why-was-this-document-included traceability is partial.** Section-level attribution exists (files appear because they are attached sources rendered into `<directories>`, `letta/schemas/memory.py:597-633`), and retrieval results carry relevance scores (`letta/services/tool_executor/core_tool_executor.py:231-246`), but there is no persistent record explaining why an individual *message* survived a given compaction beyond the cutoff-position rule. No evidence found of per-item audit logs for eviction decisions; searched `services/summarizer/*`, `agents/*`, and step/telemetry managers.
- **Effective policy depends on loop generation.** Which compaction behavior a production deployment gets (reactive-only vs. proactive 90%) depends on whether requests route through `LettaAgent` or `LettaAgentV3`; I did not find a single dispatch table documenting this mapping (routing appears in server/run-manager call sites, e.g. `letta/services/run_manager.py:657-754`, but a definitive matrix was not located within the studied dimension scope).
- No evidence found of user-configurable retention classes (e.g., pinning specific messages) beyond whole-window `autoclear` and developer-triggered `/summarize` (`letta/agents/letta_agent.py:1620-1631`).

---

Generated by `11.01-context-selection-policy` against `letta`.
