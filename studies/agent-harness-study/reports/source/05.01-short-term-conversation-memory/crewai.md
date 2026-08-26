# Source Analysis: crewai

## Dimension 05.01 — Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based agent framework; multi-package repo under `lib/`) |
| Analyzed | 2026-08-26 |

> Citation convention: all `path:line` references below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI implements short-term conversation memory as **per-execution, in-process, full-replay message lists**. The unit of conversation is a single task execution (or standalone `kickoff`), not a user/thread/session: the active executor holds a typed `list[LLMMessage]` (`lib/crewai/src/crewai/experimental/agent_executor.py:135-143`), replays the *entire* list on every LLM call in the ReAct/tool loop (`lib/crewai/src/crewai/experimental/agent_executor.py:1429-1439`, `1519-1531`), and clears it at the start of each invocation (`lib/crewai/src/crewai/experimental/agent_executor.py:2833`). There is no proactive windowing or token budgeting; overflow is handled **reactively** — when the provider raises a context-length error and `respect_context_window=True`, the whole non-system history is chunked, LLM-summarized per chunk, and replaced by one merged summary message (`lib/crewai/src/crewai/utilities/agent_utils.py:795-832`, `1048-1131`). If the flag is false, execution exits (`agent_utils.py:830-832`). Cross-task continuity is achieved not by carrying messages but by injecting prior task outputs (and optionally chat history) into the next task's prompt text (`lib/crewai/src/crewai/task.py:1057-1136`; `lib/crewai/src/crewai/utilities/formatter.py:16-45`). A post-run sanitized snapshot of the last execution's messages is exposed via `Agent.last_messages()` (`lib/crewai/src/crewai/agent/core.py:210`, `1356-1362`; builder at `lib/crewai/src/crewai/agent/utils.py:247-283`), and vector "unified memory" recall supplements prompts but is long-term/RAG in nature rather than verbatim short-term history (`lib/crewai/src/crewai/memory/unified_memory.py:681-816`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Explicit typed message contract (`LLMMessage`, roles `user|assistant|system|tool`: `lib/crewai/src/crewai/utilities/types.py:16-30`).
- Well-tested compaction path: unit tests for chunking/formatting/file preservation (`lib/crewai/tests/utilities/test_agent_utils.py:335-715`) plus provider-matrix integration tests with cassettes for OpenAI/Anthropic/Gemini/Azure and end-to-end kickoff/task compaction (`lib/crewai/tests/utilities/test_summarize_integration.py:85-254`).
- Operational touches: prompt-cache breakpoints stamped on stable prefixes (`lib/crewai/src/crewai/experimental/agent_executor.py:310-335`; `lib/crewai/src/crewai/llms/cache.py:24-32`), memory save/query events for observability (`lib/crewai/src/crewai/memory/unified_memory.py:472-521`, `722-816`).
- Kept from 9-10 by: reactive-only overflow handling (one failed provider round-trip before summarizing), crude `len(text)//4` token estimation (`agent_utils.py:835-844`), a defaults inconsistency between `Agent.respect_context_window=True` (`lib/crewai/src/crewai/agent/core.py:251-254`) and the executor field default `False` (`experimental/agent_executor.py:206`), lossy whole-history replacement during summarization, no session/thread persistence or fork/edit APIs for short-term history, and duplicated logic across the deprecated legacy executor (`lib/crewai/src/crewai/agents/crew_agent_executor.py`) and the new experimental executor.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message store (current executor) | `AgentExecutorState.messages: list[LLMMessage]` plus `execution_log` explicitly marked "NOT used for LLM calls" | `lib/crewai/src/crewai/experimental/agent_executor.py:135-170` |
| Message accessor | `messages` property proxies to flow state | `lib/crewai/src/crewai/experimental/agent_executor.py:300-308` |
| Fresh-per-execution reset | `state.messages.clear()` + full state reset in `invoke()` | `lib/crewai/src/crewai/experimental/agent_executor.py:2831-2854` |
| Legacy store | `BaseAgentExecutor.messages: list[LLMMessage]` field | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:19-29` |
| Seed messages | `_setup_messages` appends system+user prompt with cache breakpoints | `lib/crewai/src/crewai/experimental/agent_executor.py:310-335` |
| Full-history replay per call | `get_llm_response(messages=list(self.state.messages))` in ReAct and native loops | `lib/crewai/src/crewai/experimental/agent_executor.py:1429-1439`, `1519-1531` |
| Context-window guard flag | `respect_context_window` ("Keep messages under the context window size by summarizing content") | `lib/crewai/src/crewai/agent/core.py:251-254` |
| Overflow handling | `handle_context_length`: summarize or `SystemExit` | `lib/crewai/src/crewai/utilities/agent_utils.py:795-832` |
| Compaction algorithm | `summarize_messages`: preserve system msgs, chunk at boundaries, merge summaries, reattach files; token heuristic `len//4` | `lib/crewai/src/crewai/utilities/agent_utils.py:1048-1131`, `835-844` |
| Summarization prompt | i18n slices `summarizer_system_message`, structured `summarize_instruction` with `<summary>` tags | `lib/crewai/src/crewai/translations/en.json:25-26`; tag extraction `agent_utils.py:995-1009` |
| Role labels for summary input | `[ASSISTANT]:`, `[TOOL_RESULT (name)]:`, `[USER]:` formatting incl. tool_calls/multimodal handling | `lib/crewai/src/crewai/utilities/agent_utils.py:900-952` |
| Tool message retention (native) | assistant `tool_calls` message appended, then `role:"tool"` results with `tool_call_id`/`name` | `lib/crewai/src/crewai/experimental/agent_executor.py:1733-1741`, `1806-1812`, `1846-1852` |
| Tool retention (legacy) | same pattern in deprecated executor (`_append_assistant_tool_calls_message`, tool result append) | `lib/crewai/src/crewai/agents/crew_agent_executor.py:843-866`, `1050-1065` |
| ReAct observation folding | `formatted_answer.text += "\nObservation: {result}"` (text protocol instead of role=tool) | `lib/crewai/src/crewai/utilities/agent_utils.py:672-712` |
| Post-run snapshot | `save_last_messages` sanitizes to allowed roles, preserves `tool_call_id/name/tool_calls`; exposed via `last_messages()` | `lib/crewai/src/crewai/agent/utils.py:247-283`; `lib/crewai/src/crewai/agent/core.py:210`, `1356-1362` |
| Chat-mode history | plain list seeded with system+intro, appended per user/assistant turn | `lib/crewai/src/crewai/utilities/crew_chat.py:106-115`, `245-256` |
| Chat → crew injection | chat messages JSON passed as `crew_chat_messages` input; rendered into task description via `conversation_history_instruction` | `lib/crewai/src/crewai/utilities/crew_chat.py:290-304`; `lib/crewai/src/crewai/task.py:1111-1136` |
| Task-context aggregation | prior task outputs joined with dividers into prompt context | `lib/crewai/src/crewai/utilities/formatter.py:13-45`; `lib/crewai/src/crewai/crew.py:1866-1873` |
| Memory recall into prompt | `_retrieve_memory_context` recalls limit=5 (task) / limit=20 (kickoff) and appends "Relevant memories" block | `lib/crewai/src/crewai/agent/core.py:619-682`, `1540-1560` |
| Role conversion (provider) | Anthropic formatter: separate system, tool→user `tool_result` blocks, assistant `tool_use`, first-message-user coercion ("Hello") | `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:737-921` |
| Message type | `LLMMessage` TypedDict: role literal, content, optional `tool_call_id/name/tool_calls/files` | `lib/crewai/src/crewai/utilities/types.py:16-30` |
| Cache breakpoints | `mark_cache_breakpoint` on system/user prefix; Anthropic stamps `cache_control` by content match | `lib/crewai/src/crewai/llms/cache.py:24-37`; `providers/anthropic/completion.py:758-797`, `898-921` |
| Human-feedback continuation | feedback appended to existing messages; flow reruns without clearing messages | `lib/crewai/src/crewai/core/providers/human_input.py:237-264`; `experimental/agent_executor.py:3213-3271` |
| Tests: state & lifecycle | state init, setup-messages hooks, human-feedback rerun keeps state messages | `lib/crewai/tests/agents/test_agent_executor.py:88-119`, `132-235`, `243-293` |
| Tests: summarization units | preserves files/system messages, chunks at boundaries, oversized split, tool-message labeling | `lib/crewai/tests/utilities/test_agent_utils.py:335-715` |
| Tests: provider matrix | summarize integration against OpenAI/Anthropic/Gemini/Azure + crew/agent compaction | `lib/crewai/tests/utilities/test_summarize_integration.py:85-254` |
| Long-term contrast | unified `Memory` (LanceDB/Qdrant, scopes, read barrier) is separate from short-term lists | `lib/crewai/src/crewai/memory/unified_memory.py:76-159`, `681-816` |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
Within one task execution (or one standalone `kickoff`), the model sees the complete, verbatim message list every iteration: system/user seed prompt(s) built in `_setup_messages` (`lib/crewai/src/crewai/experimental/agent_executor.py:310-335`), then all accumulated assistant answers, tool calls, tool results, and injected reasoning prompts (`post_tool_reasoning` user-role nudges at `experimental/agent_executor.py:1663-1668`, `1684-1689`). Every LLM call passes `list(self.state.messages)` whole (`experimental/agent_executor.py:1429-1439`). Between tasks there is no verbatim carry-over: the next task sees prior outputs only if wired through `Task.context` (aggregated raw outputs: `formatter.py:16-45`, applied at `crew.py:1866-1873`) or chat-history interpolation (`task.py:1111-1136`). Optionally a "Relevant memories" RAG block is appended (`agent/core.py:649-657`).

**2. What gets dropped?**
Everything, deliberately, at execution boundaries: `invoke()` clears `state.messages` and all plan/todos/observations state on each fresh invocation (`experimental/agent_executor.py:2831-2845`). Within an execution, nothing is dropped proactively — trimming happens only after a context-length exception, when the entire non-system history is replaced by merged prose summaries (system messages preserved verbatim; files reattached) (`agent_utils.py:1074-1131`). System messages are excluded from summarization input (`agent_utils.py:888-897`). The `execution_log` audit trail is never sent to the LLM (`experimental/agent_executor.py:167-170`). In the Plan-and-Execute mode, step isolation explicitly excludes "execution traces, tool calls, or LLM message history" from dependency context (`experimental/agent_executor.py:610-641`).

**3. Are tool messages retained?**
Yes. The native function-calling paths persist a proper OpenAI-style transcript: one assistant message carrying `tool_calls` followed by `role="tool"` messages keyed by `tool_call_id` and `name` (`experimental/agent_executor.py:1733-1741` and `1806-1812`; legacy equivalent `agents/crew_agent_executor.py:843-866`, `1050-1065`). The ReAct text protocol instead folds results into the assistant text as `Observation:` lines (`agent_utils.py:699-700`). Post-execution, `save_last_messages` retains tool fields in the snapshot (`agent/utils.py:270-280`), and summarization renders tool results with `[TOOL_RESULT (name)]` labels before condensing them (`agent_utils.py:944-946`). Note that after summarization, the structured pairing of `tool_calls`↔`tool_call_id` is destroyed (everything collapses into one user-role summary message), which is acceptable because the loop restarts from a clean turn.

**4. Is memory per user/thread/session?**
No. Short-term history is scoped to a single agent-task invocation held in process memory; there are no thread/session IDs, no persistence layer, and no cross-process durability for these lists. The interactive `crew chat` REPL keeps its history only in a local Python list for the lifetime of the CLI process (`utilities/crew_chat.py:106-115`) and hands it to a crew kickoff as a JSON input string (`crew_chat.py:301-303`). Durable, scoped storage exists only in the unified long-term `Memory` (vector backend with hierarchical scopes like `/crew/{name}` and `/agent/{role}`, provenance `source`, private records): `memory/unified_memory.py:152-159`, `crew.py:653-688`, `base_agent_executor.py:51-63`. Multi-user isolation would have to be built on those scopes, not on short-term history.

**5. Can history be edited or forked?**
Not as a first-class API. Mutation points are internal: appends throughout the loop, in-place replacement by `summarize_messages` (`agent_utils.py:1123-1131`), and file attachment mutating the last user message (`experimental/agent_executor.py:3152-3175`). The closest things to editing/continuing: the human-feedback loop appends a formatted feedback message and re-runs the flow over the existing messages (`core/providers/human_input.py:237-264`; state preserved because `_prepare_feedback_iteration` resets everything except messages, `experimental/agent_executor.py:3256-3271`; tested in `tests/agents/test_agent_executor.py:189-293`); and the experimental conversational Flow exposes `append_message`/`receive_user_message` over `ConversationState.messages` (`experimental/conversational_mixin.py:806-861`) with a separate `agent_threads` scratch space (`experimental/conversational.py:144-164`). There is no branch/fork, no message rewrite API, no undo. Forking exists only for long-term memory views (`Memory.scope/slice`, `unified_memory.py:898-918`) and crew checkpoint restore, not for conversation transcripts.

## Architectural Decisions

1. **Execution-scoped scratchpad over session store.** History lives inside the executor's pydantic Flow state and is reset per invocation (`experimental/agent_executor.py:135-143`, `2831-2845`). CrewAI treats a "conversation" as one task's tool loop; anything longer-lived must be modeled by the caller via task contexts or unified memory.
2. **Reactive, LLM-driven compaction instead of proactive budgeting.** Compaction triggers off provider context-length errors (`agent_utils.py:781-832`), using the model itself to produce structured summaries with mandated sections (Task Overview / Current State / Discoveries / Next Steps / Preserve) (`translations/en.json:25-26`). This avoids token-count guesswork up front but pays one failed request per overflow event.
3. **Full-replay selection policy.** No sliding window, recency cutoff, or relevance filter within an execution — the entire list goes on every call (`experimental/agent_executor.py:1431`). Simplicity is chosen over cost control; prompt-cache breakpoints (`llms/cache.py:24-32`) mitigate repeat-prefix costs instead.
4. **Prompt-level inter-task context.** Continuity across tasks is textual composition (raw output aggregation with `----------` dividers: `formatter.py:13-26`), not message-list threading — consistent with CrewAI's crew-of-tasks mental model.
5. **Dual-protocol transcripts.** Native tool-calling keeps OpenAI-style `tool`/`tool_calls` records; the ReAct fallback encodes actions/observations in plain text (`agent_utils.py:672-712`). Provider adapters then translate (e.g., Anthropic requires alternating user/assistant and folds tool results into user blocks: `providers/anthropic/completion.py:740-747`, `813-823`).
6. **Observability as a sibling channel.** Events (memory saves/queries, tool usage, agent logs) and the `execution_log` capture behavior without polluting LLM-visible history (`experimental/agent_executor.py:167-170`).

## Notable Patterns

- **State/executor separation:** the new executor is a `Flow[AgentExecutorState]`; `messages`, `iterations`, etc. are exposed via compatibility properties so legacy code keeps working (`experimental/agent_executor.py:290-308`).
- **Sanitization boundary:** `save_last_messages` whitelists roles and strips unknown keys before publishing history to `TaskOutput` consumers (`agent/utils.py:261-283`).
- **Read-your-writes barriers:** unified memory drains background saves before recall (`unified_memory.py:350-363`, `711-713`) — the analogous discipline the short-term path gets for free by being synchronous.
- **Content-matched cache stamping:** Anthropic adapter matches marked message content after role-coalescing rewrites positions (`providers/anthropic/completion.py:758-797`, `898-921`).
- **Graceful degradation ladder:** native tools → text-tool fallback message → ReAct loop (`experimental/agent_executor.py:260-275`, `1573-1575`), preserving history across downgrades.
- **Parallel chunk summarization:** multiple history chunks summarized concurrently, with asyncio/thread bridging when already inside an event loop (`agent_utils.py:1012-1045`, `1107-1119`).

## Tradeoffs

- **Simplicity vs. cost:** full replay maximizes model fidelity within a task but grows latency/cost quadratically over iterations; mitigation is caching, not truncation.
- **Reactive vs. proactive compaction:** waiting for a provider error is robust across providers but wastes a failed call and relies on string-matching error signatures (`is_context_length_exceeded`, `agent_utils.py:781-792`; keyword list in `utilities/exceptions/context_window_exceeding_exception.py:6-9`).
- **Fidelity vs. safety on overflow:** replacing all non-system history with summaries guarantees forward progress but silently loses detail (e.g., exact tool outputs, IDs); only attached files survive verbatim (`agent_utils.py:1069-1072`, `1129-1130`).
- **Token estimate accuracy vs. dependency footprint:** `len//4` avoids tokenizer dependencies but underestimates CJK/code-heavy content (`agent_utils.py:835-844`).
- **Two executors:** identical message-lifecycle logic maintained in both `agents/crew_agent_executor.py` (deprecated, warns at construction: lines 143-155) and `experimental/agent_executor.py` — divergence risk until removal.
- **Human-in-the-loop continuity vs. staleness:** rerunning with accumulated feedback messages preserves context but can push the loop toward the same overflow path mid-feedback.

## Failure Modes / Edge Cases

- **Hard exit on overflow when mis-flagged:** `respect_context_window=False` raises `SystemExit` mid-task (`agent_utils.py:824-832`). The executor field defaults to `False` (`experimental/agent_executor.py:206`) while `Agent.respect_context_window` defaults to `True` (`agent/core.py:251-254`) and is propagated at executor creation (`agent/core.py:1160`, `1503`, `1520`) — constructing an executor directly without propagation changes failure semantics.
- **Summarization quality coupling:** compaction depends on the same LLM producing valid `<summary>` tags; extraction falls back to raw text otherwise (`agent_utils.py:995-1009`), and a summarizer failure surfaces as a generic exception in the recovery route (`experimental/agent_executor.py:2786-2800`).
- **Broken tool-call pairing after compaction:** post-summary, prior `assistant.tool_calls`/`tool` pairs vanish; a provider strictly validating consecutive tool-result chains would reject follow-ups until the next full turn completes (observed structure change at `agent_utils.py:1123-1131`).
- **Unbounded chat growth:** `crew_chat` never trims its list (`utilities/crew_chat.py:196-256`), so long sessions will eventually hit the overflow path with no automatic summarization wired into that loop.
- **Force-final-answer mutation:** max-iterations appends an assistant "force final answer" message to shared history (`agent_utils.py:403-415`), which persists in `last_messages()` snapshots.
- **Multimodal edge:** summarization flattens multimodal content blocks to text placeholders `[multimodal content]` (`agent_utils.py:934-940`); images survive only via the files reattachment mechanism.
- **Concurrency:** parallel native tool calls append ordered results after a thread-pool join, preserving deterministic order (`experimental/agent_executor.py:1748-1794`); executor instances guard concurrent `invoke` with a lock (`2821-2828`).

## Future Considerations

- Add **proactive budget management**: track cumulative tokens against `get_context_window_size()` (overridable per provider, e.g., `providers/openai/completion.py:2654-2680`) and compact before the provider rejects the request.
- Unify the **token estimator** with a real tokenizer or provider-reported usage from the previous call (usage is already tracked via `TokenCalcHandler` callbacks).
- Expose **history policies** (window size, keep-last-N tool results, pinned turns) as agent/task configuration rather than the binary summarize-or-exit switch.
- Provide a public **edit/fork/resume API** for transcripts (checkpointing exists for crews — `from_checkpoint` in `agent/core.py:1597`, `1616-1618` — but not for mid-conversation branching).
- Retire the legacy executor to collapse duplicated message-handling paths (`agents/crew_agent_executor.py` vs `experimental/agent_executor.py`).
- Persist chat sessions (thread IDs + pluggable store) so conversational crews survive process restarts; today only long-term `Memory` is durable.

## Questions / Gaps

- **No evidence found** for any session/thread identifier or persistence of short-term message lists: searches across `lib/crewai/src/crewai/` for session/thread-scoped message stores returned only the in-process structures cited above (`experimental/agent_executor.py:143`, `utilities/crew_chat.py:106`, `lite_agent.py:304`).
- **No evidence found** for proactive pre-call token accounting of `state.messages` (only post-hoc estimation inside summarization, `agent_utils.py:835-844`, `1080-1081`).
- **No evidence found** for a documented public contract guaranteeing `last_messages()` fidelity beyond the sanitizer (`agent/utils.py:247-283`); it appears intended for debugging/inspection (used heavily in tests, e.g., `tests/test_task_guardrails.py:41`).
- Whether the deprecated `CrewAgentExecutor` still receives maintenance-level fixes for its duplicated summarization/context handling could not be determined from source alone; its constructor emits a `DeprecationWarning` directing migration to the experimental executor (`agents/crew_agent_executor.py:143-155`).

---

Generated by dimension `05.01-short-term-conversation-memory` against `crewai`.
