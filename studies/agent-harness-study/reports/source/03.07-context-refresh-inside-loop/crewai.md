# Source Analysis: crewai

## 03.07 — Context Refresh Inside the Loop

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python 3.11+ (multi-package workspace: `crewai-core`, `crewai`, `crewai-tools`, `crewai-files`, `crewai-cli`); pydantic v2, litellm, rich, FastAPI-style internal event bus |
| Analyzed | 2026-07-15 |

## Summary

CrewAI's agent loop manages context as a mutable, single `list[LLMMessage]` that is **built once at task kickoff** and then **incrementally appended to** for every ReAct iteration. Each LLM call receives the entire accumulated `messages` list (lib/crewai/src/crewai/experimental/agent_executor.py:1486, lib/crewai/src/crewai/agents/crew_agent_executor.py:362). There is no per-turn rebuild, no proactive token budget check, and no sliding-window or repo-map facility.

When the upstream LLM raises a `LLMContextLengthExceededError` (matched by the regex phrases in lib/crewai/src/crewai/utilities/exceptions/context_window_exceeding_exception.py:4-13), the loop reacts through a single compaction primitive — `summarize_messages()` (lib/crewai/src/crewai/utilities/agent_utils.py:920) — which is enabled or disabled by a boolean `respect_context_window` flag (default `False`, see lib/crewai/src/crewai/experimental/agent_executor.py:196 and lib/crewai/src/crewai/agents/crew_agent_executor.py:126). When the flag is `False`, the loop raises `SystemExit` (lib/crewai/src/crewai/utilities/agent_utils.py:747) and the run aborts. When it is `True`, the entire non-system message history is replaced by a single LLM-generated `<summary>` block plus any preserved `files` (lib/crewai/src/crewai/utilities/agent_utils.py:993-1003).

Memory and knowledge retrieval are **one-shot, pre-loop injections** into the task prompt (lib/crewai/src/crewai/agent/core.py:583, lib/crewai/src/crewai/agent/utils.py:119). They are not refreshed between iterations. Token accounting (lib/crewai/src/crewai/utilities/token_counter_callback.py:37) is post-hoc usage telemetry only; the loop never reads accumulated prompt-token counts to decide whether to compact.

The model is told "the above is a structured summary of prior context" (lib/crewai/src/crewai/translations/en.json:27) but is never told which specific messages were dropped. Tool results are kept verbatim as `role: "tool"` messages during normal operation, but when summarization fires they are folded into the LLM-generated summary alongside everything else.

The prompt-cache marker `mark_cache_breakpoint()` (lib/crewai/src/crewai/llms/cache.py:27) provides a small cost-saving by anchoring stable prefixes, but it is not a context-management mechanism; it does not shrink the message list.

## Rating

**5 / 10** — A present, working compaction path with explicit interfaces, async summarization, and integration tests, but the design is **reactive only** (no proactive budget), **all-or-nothing** (one-shot replace, no incremental trim), **opt-in** (off by default and `SystemExit` if off), and the post-compaction message gives the model no visibility into what was lost. Faithfulness relies entirely on the LLM summarizer and the fixed five-section template.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers. Paths are relative to `studies/agent-harness-study/sources/crewai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Context assembly per turn | `messages` list cleared at start of `invoke()`/`ainvoke()`, then system + user messages appended once via `mark_cache_breakpoint(format_message_for_llm(...))` | `lib/crewai/src/crewai/experimental/agent_executor.py:2748-2789`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:220-204`, `lib/crewai/src/crewai/lite_agent.py:523-526`, `lib/crewai/src/crewai/llms/cache.py:27-32` |
| Messages rebuilt vs appended | Whole `messages` list is passed to `get_llm_response()` each turn; tool results appended as `role: "tool"`; assistant tool_calls appended as `role: "assistant"`; text-tool paths append observation into assistant text then inject `post_tool_reasoning` user message | `lib/crewai/src/crewai/experimental/agent_executor.py:1486`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:362, 432, 783, 806, 1082-1088`, `lib/crewai/src/crewai/utilities/agent_utils.py:616` |
| Compaction trigger | Reactive, on `LLMContextLengthExceededError` only; matched by 8 substring phrases | `lib/crewai/src/crewai/utilities/agent_utils.py:698-709`, `lib/crewai/src/crewai/utilities/exceptions/context_window_exceeding_exception.py:4-13`, `lib/crewai/src/crewai/experimental/agent_executor.py:1443-1445, 1541-1543, 2701-2715`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:447-456, 582-591` |
| Compaction gate | `respect_context_window: bool = Field(default=False)`; if `False`, raises `SystemExit` | `lib/crewai/src/crewai/utilities/agent_utils.py:732-749`, `lib/crewai/src/crewai/experimental/agent_executor.py:196`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:126`, `lib/crewai/src/crewai/lite_agent.py:228` |
| Summarizer implementation | `summarize_messages()`: preserves `system` messages verbatim, splits remaining at message boundaries, parallel-summarizes chunks via `asyncio.gather`, replaces all non-system messages with one `<summary>` block | `lib/crewai/src/crewai/utilities/agent_utils.py:920-1003`, `_split_messages_into_chunks()` at `lib/crewai/src/crewai/utilities/agent_utils.py:819-864`, `_asummarize_chunks()` at `lib/crewai/src/crewai/utilities/agent_utils.py:884-917` |
| Token estimator | Conservative `len(text) // 4` heuristic | `lib/crewai/src/crewai/utilities/agent_utils.py:752-761` |
| Context window model | `DEFAULT_CONTEXT_WINDOW_SIZE = 8192`, `CONTEXT_WINDOW_USAGE_RATIO = 0.85`, per-model lookup in `LLM_CONTEXT_WINDOW_SIZES` | `lib/crewai/src/crewai/llm.py:168, 325-326, 2403-2429`, `lib/crewai/src/crewai/llms/base_llm.py:70, 433-439` |
| Token accounting (post-hoc) | `TokenCalcHandler` + `TokenProcess` track prompt/completion/cached tokens via litellm callbacks; never read by the loop | `lib/crewai/src/crewai/utilities/token_counter_callback.py:37-66`, `lib/crewai/src/crewai/agents/agent_builder/utilities/base_token_process.py:8-35`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:130` |
| Memory retrieval (one-shot, pre-loop) | `unified_memory.recall(query, limit=5)` called once per task, results appended to prompt as "Relevant memories:" | `lib/crewai/src/crewai/agent/core.py:583-646`, `lib/crewai/src/crewai/lite_agent.py:556-600`, `lib/crewai/src/crewai/memory/unified_memory.py:681-724` |
| Knowledge retrieval (one-shot, pre-loop) | `handle_knowledge_retrieval()` runs agent/crew knowledge `query_func` once, appends to prompt; LLM-rewritten search query | `lib/crewai/src/crewai/agent/utils.py:119-198`, `lib/crewai/src/crewai/agent/core.py:1346-1399`, `lib/crewai/src/crewai/knowledge/knowledge.py:135-152`, `lib/crewai/src/crewai/knowledge/utils/knowledge_utils.py:4-12` |
| Cache hint | `mark_cache_breakpoint()` adds `cache_breakpoint: True` flag on system and user messages; provider adapters translate or strip | `lib/crewai/src/crewai/llms/cache.py:24-37`, `lib/crewai/src/crewai/experimental/agent_executor.py:2770-2789`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:192-199` |
| Stale-context removal | Only path is `summarize_messages()` clearing `messages` then re-appending `system_messages` + one summary message | `lib/crewai/src/crewai/utilities/agent_utils.py:995-1003` |
| Context-events trace | `MemoryRetrievalStarted/Completed/FailedEvent`, `KnowledgeRetrievalStarted/CompletedEvent`, `KnowledgeQueryStarted/Completed/FailedEvent`, `KnowledgeSearchQueryFailedEvent`, `LLMContextLengthExceededError` re-raised in `llm.py` | `lib/crewai/src/crewai/agent/core.py:596-645`, `lib/crewai/src/crewai/agent/utils.py:146-197`, `lib/crewai/src/crewai/llm.py:1869-1873` |
| Per-executor state | `AgentExecutorState.messages` is the canonical buffer; `BaseAgentExecutor.messages` is a deprecated compat shim; LiteAgent keeps `_messages` separately | `lib/crewai/src/crewai/experimental/agent_executor.py:126-161, 280-301`, `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:28`, `lib/crewai/src/crewai/lite_agent.py:469-475` |
| Conversational layer | `ConversationalMixin` keeps a separate `_conversation_messages` buffer; `conversation_messages` rebuilds the LLM list from state each call | `lib/crewai/src/crewai/experimental/conversational_mixin.py:540-545, 547-575` |
| Integration tests | VCR-backed OpenAI/Anthropic/Gemini/Azure summarize tests; `test_agent_execute_task_compaction` exercises `respect_context_window` via real `Agent.execute_task`; `test_handle_context_length_exceeds_limit` covers abort path | `lib/crewai/tests/utilities/test_summarize_integration.py:86-279`, `lib/crewai/tests/agents/test_agent.py:1341-1413`, `lib/crewai/tests/utilities/test_agent_utils.py:308-792` |

## Answers to Dimension Questions

### 1. Is context static or rebuilt each turn?

**Static-but-append-only.** Each iteration passes the entire accumulated `messages` list to the LLM; nothing is rebuilt from scratch. System and user prompt messages are constructed once in `invoke()` (`lib/crewai/src/crewai/experimental/agent_executor.py:2748-2789`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:170-204`) and only the trailing turn (assistant text, tool messages, native tool_calls, post-tool reasoning user prompt) is added per iteration. There is no semantic rebuild — no re-ranking, no retrieval refresh, no token-budget recheck between turns.

### 2. What gets dropped first?

Nothing is dropped unless `summarize_messages()` runs, and then **everything except `system` messages is replaced by a single LLM-generated summary**. The summarizer explicitly:

- Strips `system` messages out of the chunking loop (`lib/crewai/src/crewai/utilities/agent_utils.py:834`)
- Replaces the whole non-system list with one summary user message (`lib/crewai/src/crewai/utilities/agent_utils.py:995-1003`)
- Merges any `files` attachments from user messages onto the summary message (`lib/crewai/src/crewai/utilities/agent_utils.py:941-944, 1001-1002`)

So when compaction fires, **all tool results, all native `tool_calls` blocks, and the original user prompt** are folded into one synthesized block. The model loses verbatim access to any long tool output (e.g., a 5000-token file read or a large search result).

If `respect_context_window` is `False` (the default), nothing is dropped — the process is killed with `SystemExit` (`lib/crewai/src/crewai/utilities/agent_utils.py:747`).

### 3. Is summarization faithful?

Faithfulness is delegated to the LLM. The instruction template asks for five sections — *Task Overview, Current State, Important Discoveries, Next Steps, Context to Preserve* — and requires the output to be wrapped in `<summary></summary>` tags (`lib/crewai/src/crewai/translations/en.json:26`). The harness extracts whatever is inside those tags (`_extract_summary_tags`, `lib/crewai/src/crewai/utilities/agent_utils.py:867-881`) and falls back to the full text if no tags are present.

There is no post-hoc verification: the summary is not diffed against the source messages, the model is not asked to confirm completeness, and no metrics are recorded. Faithfulness therefore depends entirely on (a) the summarizer model being at least as capable as the agent's primary LLM, and (b) the chunk size being small enough that each chunk fits in the summarizer's window — which `summarize_messages` enforces by splitting on `llm.get_context_window_size()` (`lib/crewai/src/crewai/utilities/agent_utils.py:952-953`). For very long histories, parallel summarization means a single chunk can lose coherence with the others.

### 4. Are tool results preserved or summarized?

- **Preserved verbatim** during normal operation. Native function-calling paths append `{"role": "tool", "tool_call_id": ..., "name": ..., "content": <raw result>}` after each tool (`lib/crewai/src/crewai/agents/crew_agent_executor.py:1082-1088`, same pattern in `lib/crewai/src/crewai/experimental/agent_executor.py:1763, 1801`). Text-tool paths append an `Observation:` line into the assistant message text via `handle_agent_action_core()` (`lib/crewai/src/crewai/utilities/agent_utils.py:616`).
- **Compressed** when summarization fires. The summarizer's `_format_messages_for_summary()` renders `role: "tool"` messages as `[TOOL_RESULT (<tool_name>)]: <content>` (`lib/crewai/src/crewai/utilities/agent_utils.py:806-810`), so the LLM summarizer decides whether to keep, paraphrase, or drop tool output entirely.

There is no tool-result-specific pruning — large tool outputs survive intact until the whole-buffer compaction runs.

### 5. Can the model tell what context is missing?

**No, not from in-context signals.** The post-compaction user message reads:

```
<summary>{merged_summary}</summary>

Continue the task from where the conversation left off. The above is a structured summary of prior context.
```

(`lib/crewai/src/crewai/translations/en.json:27`, formatted into the buffer at `lib/crewai/src/crewai/utilities/agent_utils.py:998-1000`). This tells the model that summarization happened but does not enumerate dropped messages, dropped tool calls, or any loss. The system prompt is unchanged before and after compaction, so the agent has no marker like "you previously had N tool results, only K were preserved". The only operational signal is the absence of specific tool-call ids in the buffer.

External observability does exist for callers via events: `MemoryRetrievalStartedEvent`/`CompletedEvent`/`FailedEvent`, `KnowledgeRetrievalStartedEvent`/`CompletedEvent`, `KnowledgeQueryStartedEvent`/`CompletedEvent`/`FailedEvent`, and the `LLMContextLengthExceededError` re-raise in `lib/crewai/src/crewai/llm.py:1869`. But these are out-of-band telemetry, not in-prompt signals.

## Architectural Decisions

- **Single mutable message buffer, not a builder.** Both `CrewAgentExecutor` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:28`) and the experimental `AgentExecutor` (`lib/crewai/src/crewai/experimental/agent_executor.py:134-161`) hold one `list[LLMMessage]` for the entire task. There is no message-factory pattern; every site appends directly (`agent_executor.py:1150, 256, 628, 649, 783, 806, 1082, 1628, 1649, 1701, 1763, 1801, 2775-2789, 2881-2895`).
- **Reactive-only compaction.** No proactive token-budget check exists; compaction is gated on the LLM provider raising an error that matches the regex phrases in `lib/crewai/src/crewai/utilities/exceptions/context_window_exceeding_exception.py:4-13`.
- **Opt-in failure mode.** `respect_context_window` defaults to `False` on both executors and `LiteAgent` (`lib/crewai/src/crewai/experimental/agent_executor.py:196`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:126`, `lib/crewai/src/crewai/lite_agent.py:228`). The default behavior on overflow is `SystemExit`, not summarize.
- **One-shot replace compaction.** `summarize_messages()` clears the entire non-system history (`lib/crewai/src/crewai/utilities/agent_utils.py:995`) and re-inserts system messages plus one summary — there is no incremental trim, no first-N-drop, no sliding window.
- **Chunked parallel summarization.** When the non-system history exceeds one context window, it is split at message boundaries (`_split_messages_into_chunks`, `lib/crewai/src/crewai/utilities/agent_utils.py:819-864`) and summarized concurrently via `asyncio.gather` (`_asummarize_chunks`, `lib/crewai/src/crewai/utilities/agent_utils.py:884-917`). Single-chunk path uses the synchronous `llm.call` (`lib/crewai/src/crewai/utilities/agent_utils.py:976`).
- **Pre-loop retrieval only.** Memory and knowledge are pulled once at task preparation (`agent/core.py:583-646`, `agent/utils.py:119-198`, `lite_agent.py:556-600`); neither is refreshed between iterations or after compaction.
- **Cache hint, not cache control.** `mark_cache_breakpoint()` (`lib/crewai/src/crewai/llms/cache.py:27`) flags messages where a stable prefix ends, leaving the actual provider translation to adapter code. It reduces cost but does not alter the semantic message list.
- **Task-scoped context lifetime.** Every `invoke()`/`ainvoke()`/`kickoff()` clears `messages` at the start (`agent_executor.py:2748`, `crew_agent_executor.py:220`, `lite_agent.py:523-526`). Agent context is task-scoped; cross-task persistence lives in the unified memory layer (`lib/crewai/src/crewai/memory/unified_memory.py`), which is a separate retrieval step, not a continuation of the in-context buffer.
- **Plan-and-Execute layered on the same buffer.** `AgentExecutorState` (`lib/crewai/src/crewai/experimental/agent_executor.py:126-161`) adds plan/todos/observations/execution_log fields, but they are reconstructed into prompt text via `I18N_DEFAULT.retrieve("planning", ...)` and still enter the same `messages` list — no separate compaction tier.

## Notable Patterns

- **System-message preservation across compaction.** `summarize_messages()` separates `system_messages` from the rest and re-prepends them verbatim (`lib/crewai/src/crewai/utilities/agent_utils.py:946-996`). This is the only message class that is guaranteed to survive context refresh.
- **File attachment propagation.** User messages with `files` attachments are merged across the whole history and re-attached to the summary user message (`lib/crewai/src/crewai/utilities/agent_utils.py:941-944, 1001-1002`). Verified by `test_summarize_preserves_files_integration` (`lib/crewai/tests/utilities/test_summarize_integration.py:258-279`).
- **Cache-breakpoint tagging.** `mark_cache_breakpoint()` is applied at the end of system and user prompt messages (`agent_executor.py:2770-2789`, `crew_agent_executor.py:192-199`), giving providers a stable prefix to cache across iterations.
- **Async-aware compaction.** `summarize_messages()` falls back to running `asyncio.run` inside a `ThreadPoolExecutor(max_workers=1)` if an event loop is already active (`lib/crewai/src/crewai/utilities/agent_utils.py:986-989`), so it works inside Flow-driven async paths.
- **Flow-routed context error path.** The experimental `AgentExecutor` routes a `context_error` event into a `recover_from_context_length` node (`agent_executor.py:2701-2715`) instead of an exception handler, fitting its `Flow[AgentExecutorState]` orchestration model.
- **Memory `drain_writes` read barrier.** `unified_memory.recall()` calls `self.drain_writes()` first (`lib/crewai/src/crewai/memory/unified_memory.py:713`) so recall sees pending background saves — a small but explicit ordering guarantee for retrieval.
- **Conversational layer kept distinct.** `ConversationalMixin.conversation_messages` rebuilds the LLM list from `state.messages` on each access (`lib/crewai/src/crewai/experimental/conversational_mixin.py:540-545`), and `ConversationMessage` records live in their own state slot, so chat history doesn't pollute the executor's `messages` buffer.

## Tradeoffs

- **Reactive-only trigger means the loop will only discover overflow when the provider actually rejects.** If a provider silently truncates or accepts an oversized prompt, no compaction runs. CrewAI cannot bound the prompt size itself because no proactive token-count check exists.
- **All-or-nothing compaction.** A single summarization step replaces the whole non-system history with one block. Long, structured tool results (file dumps, search outputs, code) become summary text whose fidelity depends on the summarizer model. There is no "drop the oldest K messages" or "keep the last N tool results raw" mode.
- **Opt-in safe path = unsafe default.** With `respect_context_window=False` (the default), a context overflow raises `SystemExit` (`lib/crewai/src/crewai/utilities/agent_utils.py:747`), killing the whole run rather than degrading gracefully.
- **Token accounting is not used for control.** `TokenCalcHandler` (`lib/crewai/src/crewai/utilities/token_counter_callback.py`) tracks `prompt_tokens`, `completion_tokens`, `cached_prompt_tokens` for telemetry, but no caller reads these counts to decide "we're at 80%, time to compact".
- **Estimation heuristic is coarse.** `_estimate_token_count(text)` returns `len(text) // 4` (`lib/crewai/src/crewai/utilities/agent_utils.py:761`); this can misjudge CJK/code-heavy tool output by a wide margin and produce unbalanced chunks.
- **Cache breakpoint is set-and-forget.** Marking a message as a breakpoint does not guarantee the provider honors it; if the system prompt is later regenerated or the message list is mutated (e.g., `_downgrade_to_text_tool_calling` appends a new user message at `agent_executor.py:256-264`), the cached prefix may be invalidated silently.
- **Knowledge search query is rewritten with the agent's own LLM.** `_get_knowledge_search_query` (`lib/crewai/src/crewai/agent/core.py:1346-1399`) uses the agent's LLM to rewrite the task into a retrieval query. On context-overflow this round-trip is the first thing to fail because the agent LLM is itself the saturated resource.

## Failure Modes / Edge Cases

- **SystemExit on overflow when `respect_context_window=False`.** Tests confirm the run aborts without invoking `handle_context_length` (`lib/crewai/tests/agents/test_agent.py:1384-1413`).
- **Summarizer recursion on overflow.** If the summarizer's own call exceeds context (`summarize_messages` builds its own two-message prompt at `agent_utils.py:965-975`), there is no second-level fallback — the exception propagates.
- **Empty or single-message histories.** `_split_messages_into_chunks` returns `[]` when there are no non-system messages (`agent_utils.py:834-836`); `summarize_messages` early-returns (`agent_utils.py:949-950`).
- **Multimodal / None content.** The summarizer coerces `None` content with tool_calls to `"[Called tools: ...]"` (`agent_utils.py:784-797`) and list content blocks to `" ".join(text)` or `"[multimodal content]"` (`agent_utils.py:798-804`); image bytes are dropped in the summary even if the provider cache still has them.
- **Tool message role preservation.** `role: "tool"` survives verbatim into the summarizer's input (`agent_utils.py:806-810`) but is rendered as a `[TOOL_RESULT (...)]` label, so `tool_call_id` linkage is lost in the post-summary buffer.
- **In-flight event loop.** `summarize_messages` branches on `is_inside_event_loop()` to avoid `asyncio.run` collisions (`agent_utils.py:986-991`); this is correct for Flow contexts but means callers must not block on the returned future from outside an event loop.
- **Plan-and-Execute adds prompts that themselves consume tokens.** Planning messages (`agent_executor.py:2397-2410`) are appended before the user prompt, so on tight windows the planning preamble competes with the task prompt for space.
- **Conversational context shadowing.** `ConversationalMixin` keeps its own `_conversation_messages` buffer (`lib/crewai/src/crewai/experimental/conversational_mixin.py:127`); if both the conversational layer and the executor append, the executor's `messages.clear()` at `agent_executor.py:2748` does NOT clear conversational state, so two paths can drift.

## Future Considerations

- Add a **proactive token-budget check** that reads `TokenProcess.prompt_tokens` (or the provider's last usage) and triggers `summarize_messages` before the next LLM call, instead of waiting for the provider to reject.
- Introduce **partial compaction primitives** — "drop the oldest N tool messages", "truncate tool output to K tokens", "keep the last M turns raw" — to avoid losing verbatim tool output that may be needed for correctness (e.g., numeric reads).
- Make `respect_context_window` default to `True`, or add a separate `on_context_overflow` enum (`"abort" | "summarize" | "drop_oldest"`) so the safe path is the default.
- Re-introduce retrieval into the loop — e.g., re-query memory/knowledge after compaction so the post-summary buffer gets fresh grounding instead of relying on the LLM to recall pre-summary details.
- **Surface compaction observability** in the message buffer itself (e.g., prepend a `[Earlier context was summarized; N turns omitted]` marker) so the model can reason about missing context.
- Replace `len(text) // 4` with a tokenizer-aware estimate (tiktoken or provider tokenizer) to balance chunks correctly.
- Wire `TokenCalcHandler.log_success_event` into a hook point that fires *before* the next iteration, giving the executor real-time usage awareness.
- Add a dedicated `ContextRefreshTriggeredEvent` so external observers (Tracing UI, telemetry) can correlate summarization with subsequent tool choices.

## Questions / Gaps

- No evidence of a **repo-map / file-tree index** anywhere in the source tree. `grep -rn "repo_map\|repo-map"` returned no hits in `lib/crewai/src/crewai`. Knowledge retrieval is RAG over a vector store, not a code-base map.
- No evidence of a **sliding-window or per-role-context** mechanism — the only persistence across turns is the `messages` list itself plus the unified memory layer.
- The `TokenManager` at `lib/crewai-core/src/crewai_core/token_manager.py` is **unrelated** to LLM context; it manages OAuth credential storage on disk. The naming is misleading when grepping for "token".
- Whether `summarize_messages` correctly handles **very large single messages** (e.g., a 200k-token tool result) is unclear — `_split_messages_into_chunks` only splits *between* messages, so a single oversized message becomes its own chunk and is summarized in isolation.
- The LiteAgent path (`lib/crewai/src/crewai/lite_agent.py:860`) shares the same reactive compaction but does not use the experimental `AgentExecutor`'s `Flow`-routed `recover_from_context_length`; it `continue`s the loop directly (`lite_agent.py:950-959`). The behavioral equivalence is not tested in the searched tests.
- No evidence that compaction **clears tool-cache state** (`tools_handler.cache` at `lib/crewai/src/crewai/agents/crew_agent_executor.py:929-936`); cached tool results survive summarization but the cache may reference inputs that the model can no longer see, creating a divergence between "what the model remembers calling" and "what the model remembers getting back".
- The `execution_log` field in `AgentExecutorState` (`lib/crewai/src/crewai/experimental/agent_executor.py:161`) is explicitly described as **"NOT used for LLM calls"** — it is debugging audit only. There is no in-band mechanism for the agent to inspect its own compaction history.

---

Generated by `studies/agent-harness-study/reports/source/03.07-context-refresh-inside-loop/dimension-03.07.md` against `crewai`.