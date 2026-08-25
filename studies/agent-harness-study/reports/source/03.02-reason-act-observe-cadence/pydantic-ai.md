# Source Analysis: pydantic-ai

## 03.02 Reason/Act/Observe Cadence

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (3.10–3.13), `uv` workspace with `pydantic_ai_slim` (agent framework + model providers), `pydantic_graph` (graph engine), `pydantic_evals` (evals). Core types are dataclasses + Pydantic discriminated unions. |
| Analyzed | 2026-07-13 |

## Summary

Pydantic AI implements Reason/Act/Observe as an **explicit node-graph cycle** (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2129-2139`) rather than a free-text ReAct parser. The cadence is: `UserPromptNode` → `ModelRequestNode` → `CallToolsNode` → (loop or `End`). Reasoning happens inside the model call (`ModelRequestNode._make_request`, `_agent_graph.py:778-839`); Pydantic AI never parses "Thought:" / "Action:" / "Observation:" markers. The framework drives everything off the provider-native `ToolCallPart` / `NativeToolCallPart` discriminated-union output of `ModelResponse` (`messages.py:1871-1900`, `messages.py:1898-1900`, `messages.py:2093-2155`).

Action dispatch lives in `CallToolsNode._handle_tool_calls` (`_agent_graph.py:1260-1298`) and the two helpers `process_tool_calls` (`_agent_graph.py:1537-1859`) + `_call_tools` (`_agent_graph.py:1862-1965`). Calls are split by `ToolKind` (`function` vs `output` vs `unknown`) at `_agent_graph.py:1553-1560`, validated up-front, and executed in `parallel` / `sequential` / `parallel_ordered_events` mode per `tool_manager.get_parallel_execution_mode` (`tool_manager.py:155-168`).

Observations are written back as `ToolReturnPart` or `RetryPromptPart` inside a `ModelRequest` (`messages.py:1333-1356`, `messages.py:1407-1502`). The `ModelRequest` that wraps them is built in `_call_tools` / `_handle_tool_calls` and appended at `_agent_graph.py:1298`, then optionally merged into a previous `ModelRequest` by `_clean_message_history` (`_agent_graph.py:2220-2269`) so the model sees a single `ModelRequest` with all `ToolReturnPart`s placed at the start and any user-supplied `UserPromptPart` after them (`_agent_graph.py:2237-2245`).

The framework has **structured reasoning visibility**: `ThinkingPart` is a first-class `ModelResponsePart` (`messages.py:1629-1678`) — Pydantic AI doesn't strip chain-of-thought, it preserves provider signatures (e.g. Anthropic signature, Google `thought_signature`, OpenAI `encrypted_content`, `messages.py:1644-1655`) so they round-trip on follow-up turns. There is **no automatic ReAct-style scratchpad**; reasoning lives in the model output itself.

Failed observations are fed back as `RetryPromptPart` (`messages.py:1407-1502`) — either because the tool raised `ModelRetry` (`tool_manager.py:760-766`, `exceptions.py:40-77`), Pydantic `ValidationError` (`tool_manager.py:184-191`), an unknown tool name (`tool_manager.py:369`), or a slow tool (`tests/test_tools.py:2750-2784`). The retry budget is split per-tool (`RunContext.retries[name]` plus `tool.max_retries`, `_run_context.py:60-77`) and globally (`max_output_retries`, `GraphAgentState.output_retries_used`, `_agent_graph.py:162-179`). Per-tool retries stop with `UnexpectedModelBehavior` (`tool_manager.py:177-181`); the global budget stops the same way at `_agent_graph.py:171-180`.

There is **no built-in compaction of observations**. Long histories are surfaced via `HistoryProcessor` / `ProcessHistory` capability (`capabilities/process_history.py:31-38`, `_history_processor.py:11-21`) which rewrites `request_context.messages` before the model is called (`_agent_graph.py:895-898`). Compaction-specific provider support exists via `Model.compact_messages` (`models/__init__.py:288-300`, e.g. OpenAI Responses).

The loop has a soft cap on retries via the global output-retry budget (`max_output_retries`, default 1 for tools, `_run_context.py:67-77`, `tool_manager.py:92-93`) and a hard cap via `UsageLimits.request_limit` / `tool_calls_limit` (`tests/test_agent.py:617-684`). On an empty / thinking-only / no-actionable-output response, `_run_stream` consumes the global retry budget and resubmits via `_build_retry_node` (`_agent_graph.py:1101-1250`, `_agent_graph.py:1027-1040`).

## Rating

**8 / 10** — The Reason/Act/Observe cadence is implemented as an explicit, type-safe graph with strong guarantees: (a) structured `ToolCallPart` parsing — no fragile text parsing of "Action:" markers; (b) per-call validation via `ToolManager.validate_tool_call` with `args_valid` surfaced on `FunctionToolCallEvent` (`tool_manager.py:419-465`, `messages.py:2837-2847`); (c) parallel / sequential / parallel_ordered_events execution modes (`tool_manager.py:155-168`); (d) typed observations — `ToolReturnPart` for success, `RetryPromptPart` for failures, both `tool_call_id`-correlated (`messages.py:1114-1156`, `messages.py:1407-1438`); (e) per-tool retry budget separated from the global output-retry budget (`_run_context.py:60-77`, `_agent_graph.py:162-179`, `tool_manager.py:177-181`); (f) `_clean_message_history` orders `ToolReturnPart`/`RetryPromptPart` first inside each `ModelRequest` so providers that demand a strict tool/role separation still see valid input (`_agent_graph.py:2220-2245`); (g) capability hooks at every step (`before_model_request`, `after_model_request`, `wrap_model_request`, `on_model_request_error`, `before_tool_validate`, `wrap_tool_validate`, `on_tool_validate_error`, `after_tool_validate`, `before_tool_execute`, `wrap_tool_execute`, `on_tool_execute_error`, `after_tool_execute`, `capabilities/hooks.py:170-1182`); (h) `ThinkingPart` is a first-class preserved-across-turns discriminated union member (`messages.py:1629-1678`).

Where it loses points: (a) no automatic ReAct-style prompt template or scratchpad buffer — reasoning is whatever the provider returns (not always visible if the model hides it); (b) no built-in compaction of long tool outputs — users must write a `ProcessHistory` capability or rely on `Model.compact_messages` (provider-specific, `models/__init__.py:288-300`); (c) "failed observation" feedback exists but is split across two layers (per-tool `RetryPromptPart` injected into the same `ModelRequest` as other tool returns, `_agent_graph.py:1617-1625` and global `RetryPromptPart` injected as a fresh `ModelRequest` via `_build_retry_node`, `_agent_graph.py:1027-1040`), which is consistent but not always obvious from the docs; (d) there is no first-class "ReAct parser" — Pydantic AI assumes the model is structured-output capable (tool calls on the wire, JSON schema outputs).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Graph cycle (ReAct-as-graph) | `UserPromptNode` → `ModelRequestNode` → `CallToolsNode` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2129-2139` |
| Architecture diagram in skills | `Agent.run() → UserPromptNode → ModelRequestNode → CallToolsNode → (loop or end)` | `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/ARCHITECTURE.md:225-228` |
| Node iteration entry | `AgentRun.next(node)` runs one graph step | `pydantic_ai_slim/pydantic_ai/run.py:330-404` |
| Model request node (Reason step) | `_make_request` issues the chat completion | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:778-839` |
| Wrap-model-request hook entry | `wrap_model_request` short-circuits or retries | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:814-839`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:510-540` |
| `after_model_request` retry path | hook raises `ModelRetry` → `_build_retry_node` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:975-990`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Tool-call extraction (no text parsing) | `_handle_tool_calls` reads `ToolCallPart` from `ModelResponse.parts` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1180-1219` |
| Tool calls by `ToolKind` | split into `function` / `output` / `unknown` / `external` / `unapproved` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1553-1560`, `pydantic_ai_slim/pydantic_ai/tools.py:177-260` |
| Tool call dispatched to `process_tool_calls` | `CallToolsNode._handle_tool_calls` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1260-1298` |
| Parallel / sequential execution modes | `ToolManager.get_parallel_execution_mode` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:155-168` |
| Sequential execution path | awaits one tool at a time | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1903-1913` |
| Parallel ordered-events execution | wait-all then yield | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1928-1933` |
| Parallel unordered execution | `asyncio.wait` `FIRST_COMPLETED` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1934-1945` |
| Per-tool validation hook entry | `ToolManager.validate_tool_call` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:419-465` |
| Output-tool validation path (separate from function tools) | `validate_output_tool_call` runs through output hooks, not tool hooks | `pydantic_ai_slim/pydantic_ai/tool_manager.py:505-574` |
| Tool execution hook entry | `_run_execute_hooks` (`before`/`wrap`/`on_error`/`after`) | `pydantic_ai_slim/pydantic_ai/tool_manager.py:294-354` |
| Observation as `ToolReturnPart` | built in `_call_tool` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2042-2052` |
| `ToolReturnPart` model wire-format helper | `model_response_str_and_user_content` | `pydantic_ai_slim/pydantic_ai/messages.py:1271-1297` |
| Failed observation as `RetryPromptPart` | built from `ValidationError` / `ModelRetry` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:183-191`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2010-2024` |
| `RetryPromptPart` per-step helper | formats Pydantic errors with `Fix the errors and try again.` | `pydantic_ai_slim/pydantic_ai/messages.py:1446-1468` |
| `ModelRetry` exception type | re-raised by tool bodies → observation path | `pydantic_ai_slim/pydantic_ai/exceptions.py:40-77` |
| `_call_tools` writes back ordered observations | tools by index, then user prompt parts | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1958-1965` |
| `_clean_message_history` keeps `ToolReturnPart`/`RetryPromptPart` first | guarantees provider wire order | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2220-2245` |
| Per-tool retry budget | `RunContext.retries[name]` / `tool.max_retries` | `pydantic_ai_slim/pydantic_ai/_run_context.py:60-77`, `pydantic_ai_slim/pydantic_ai/tools.py:527-568` |
| Global output-retry budget | `GraphAgentState.output_retries_used` / `max_output_retries` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-180`, `pydantic_ai_slim/pydantic_ai/_run_context.py:67-77` |
| Per-tool retry exhaustion | `ToolManager._check_max_retries` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:177-181` |
| Unknown tool → observation path | `_resolve_tool` raises `ModelRetry('Unknown tool name...')` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:356-370` |
| Empty-response handling | consumes global retry budget, resubmits via `_build_retry_node` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1166-1173`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Thinking-only / no-actionable handling | falls back to text recovery, then to retry prompt | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1102-1175` |
| Content-filter handling | preserves `provider_details` for observability, raises typed `ContentFilterError` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1117-1130` |
| `ThinkingPart` as first-class preserved reasoning | signature round-trip across turns | `pydantic_ai_slim/pydantic_ai/messages.py:1629-1678` |
| Thinking capability | provider-agnostic `Thinking` capability and unified setting | `pydantic_ai_slim/pydantic_ai/capabilities/thinking.py`, `pydantic_ai_slim/pydantic_ai/models/__init__.py:155-162`, `docs/thinking.md:9-54` |
| `CompactionPart` for provider-managed compaction | Anthropic text + OpenAI encrypted blob | `pydantic_ai_slim/pydantic_ai/messages.py:1681-1729` |
| `Model.compact_messages` provider hook | optional, e.g. OpenAI Responses | `pydantic_ai_slim/pydantic_ai/models/__init__.py:288-300` |
| `HistoryProcessor` / `ProcessHistory` capability | rewrites messages before model call | `pydantic_ai_slim/pydantic_ai/capabilities/process_history.py:31-38`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:895-898` |
| Streamed parts manager (assembly of partial parts) | `ModelResponsePartsManager.handle_tool_call_part` | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:302-401` |
| Parallel tool-call part delta handling | typed promotion by `tool_kind` | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:86-97`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2142-2173` |
| Cancellation safety for parallel tool calls | `cancel_and_drain` on outer error | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1947-1956` |
| Call-back-to-history attachment | `FunctionToolCallEvent` carries `args_valid` for observability | `pydantic_ai_slim/pydantic_ai/messages.py:2837-2847`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1731-1734` |
| Tool definition kind discriminator | `'function'`, `'output'`, `'external'`, `'unapproved'` | `pydantic_ai_slim/pydantic_ai/tools.py:177-260` |
| End strategy controls when to stop after first final result | `EndStrategy` (`early`/`graceful`/`exhaustive`) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:73`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1565-1677` |
| `FunctionToolResultEvent` content carries follow-up user prompt | `tool_return.content` → `UserPromptPart` next to the result | `pydantic_ai_slim/pydantic_ai/messages.py:2878-2912`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1896-1900` |
| `SkipModelRequest` for short-circuit model calls | capability can fabricate a `ModelResponse` | `pydantic_ai_slim/pydantic_ai/exceptions.py:116-131`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:616-633` |
| Unknown-tool test snapshot | full Reason/Act/Observe message trace | `tests/test_agent.py:3897-3996` |
| Retry-until-success test (loop semantics) | 4-turn trace with `RetryPromptPart` then `ToolReturnPart` | `tests/test_tools.py:2648-2746` |
| Tool-timeout retry test | timeout → `RetryPromptPart` re-injected | `tests/test_tools.py:2750-2784` |
| Retry prompt error formatting tests | strip `input` for top-level / keep for nested | `tests/test_messages.py:1523-1597` |
| Unknown-tool retries-exhausted test | per-tool budget gate | `tests/test_agent.py:3999-4072` |
| Validation error → retry test (hook layer) | `after_output_validate` triggers retry path | `tests/test_capabilities.py:18879-18907` |
| Capability retry trace test | validation error from `after_output_process` triggers retry | `tests/test_capabilities.py:18908-18948` |
| Thinking-part round-trip test (replay) | provider signature preserved on next turn | `tests/v2/test_google_history_replay.py:52-86` |

## Answers to Dimension Questions

1. **How does the model decide actions?**
   The chat-model call is delegated to a provider-specific `Model` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:217-271`). When the response contains a non-empty `parts` list with `ToolCallPart` or `NativeToolCallPart` (`pydantic_ai_slim/pydantic_ai/messages.py:2093-2155`), `CallToolsNode._run_stream` routes to `_handle_tool_calls` (`_agent_graph.py:1180-1219`). Pydantic AI never inspects free-text "Action:" markers — it relies on the provider-native structured `tool_calls` channel. Reasoning is whatever the model returned: `TextPart` for chat, `ThinkingPart` for chain-of-thought (preserved across turns when the provider supports it, `messages.py:1644-1655`).

2. **How are observations returned?**
   `_call_tool` builds a `ToolReturnPart` with the typed return value and metadata (`_agent_graph.py:2042-2052`, `messages.py:1333-1356`). For multimodal content the part's `model_response_str_and_user_content` produces a `text+files` pair (`messages.py:1271-1297`); for providers that cannot accept multimodal tool outputs, the files travel as a trailing `UserPromptPart`. Failures become `RetryPromptPart` (`_agent_graph.py:2010-2024`, `messages.py:1407-1502`). The next `ModelRequest` is built in `_handle_tool_calls` (`_agent_graph.py:1275-1298`) by appending tool parts then any user-prompt parts, and `_clean_message_history` orders `ToolReturnPart`/`RetryPromptPart` first within the `ModelRequest` (`_agent_graph.py:2237-2245`). The `ModelRequest` is appended to `state.message_history` on the next `ModelRequestNode.run` (`_agent_graph.py:854`), so the model sees raw tool results on the next turn with no automatic summarization.

3. **Is reasoning visible or hidden?**
   Reasoning is **explicit** but **provider-driven**: `ThinkingPart` (`messages.py:1629-1678`) is a first-class `ModelResponsePart` with `signature`, `id`, and `provider_details`. Pydantic AI preserves signatures (Anthropic, Google `thought_signature`, OpenAI `encrypted_content`, Bedrock) so chain-of-thought can round-trip on follow-up turns (`messages.py:1644-1655`). The framework does not strip reasoning text — but it does not surface a separate "Thought" channel either; the model emits `TextPart` and/or `ThinkingPart` and both are kept verbatim. There is no scratchpad buffer; `ctx.last_attempt` (`_run_context.py:155-156`) and `ctx.retry`/`ctx.retries[name]` (`_run_context.py:60-77`) carry only the retry budget, not any model-generated rationale.

4. **Are failed observations fed back?**
   Yes, in two distinct ways:
   - **Per-tool failure** → `RetryPromptPart` injected into the **same** `ModelRequest` as the other tool returns. The next model call sees the failure inline alongside successes. `RetryPromptPart.content` is a `list[ErrorDetails]` for `ValidationError` and a string for `ModelRetry`, prefixed with `"Fix the errors and try again."` (`messages.py:1446-1468`). The retry budget is per-tool (`ctx.retries[name]`, `_run_context.py:60-77`, `tool_manager.py:177-181`).
   - **Whole-turn failure** (empty / thinking-only / no-actionable / output-validation failure) → global `RetryPromptPart` injected as a **fresh** `ModelRequest` via `_build_retry_node` (`_agent_graph.py:1027-1040`, `_agent_graph.py:1243-1249`). The budget is `max_output_retries` (`_agent_graph.py:162-180`).
   Unknown-tool errors use the per-tool path: `_resolve_tool` raises `ModelRetry('Unknown tool name...')` (`tool_manager.py:356-370`).

5. **Can observations be compacted?**
   Not automatically. There is no built-in summarization. The hooks are:
   - `HistoryProcessor` / `ProcessHistory` capability (`capabilities/process_history.py:31-38`) — replaces `request_context.messages` before the model call (`_agent_graph.py:895-898`).
   - `Model.compact_messages` (`models/__init__.py:288-300`) — provider-implemented, used by Anthropic (text summary, `messages.py:1689-1690`) and OpenAI Responses (encrypted blob, `messages.py:1690`). `CompactionPart` carries the result (`messages.py:1681-1729`).
   - Long tool return strings can be JSON-serialized and split into `tool_content` (text) + `file_content` (multimodal) for providers that can't accept multimodal tool outputs (`messages.py:1271-1297`).
   The user has full control via these hooks, but no opinionated "compact old tool calls" middleware exists in the core.

## Architectural Decisions

- **Graph-as-loop** (`_agent_graph.py:2129-2139`): the cycle is encoded declaratively in `pydantic-graph`, not as an imperative while-loop. Each node is a dataclass with a `run()` method; pydantic-graph drives them.
- **Structured tool calls only**: Pydantic AI never parses free-text ReAct markers. It assumes provider-native tool calls, falling back to text output only when `output_type` allows it (`_agent_graph.py:1234-1245`).
- **Two-layer retry budget**: per-tool (`RunContext.retries[name]`, default 1) and global (`output_retries_used`, default 1). Documented in `_run_context.py:60-77` and `_agent_graph.py:162-180`. The split prevents a single failing tool from eating the global budget (`agent_docs/pydantic-ai-slim.md:24`).
- **`args_valid` exposed on the call event** (`messages.py:2817-2826`): consumers can match on validation outcome, not just success/failure, distinguishing "I never checked", "I checked and it failed", and "I checked and it passed".
- **Tool parts always lead each `ModelRequest`** (`_agent_graph.py:2237-2245`): preserves provider wire-format invariants while still allowing user-prompt additions to follow.
- **Capabilities wrap hooks** (`capabilities/hooks.py:170-1182`): every step in the Reason/Act/Observe cycle has a `before`/`wrap`/`on_error`/`after` hook surface, and capabilities can intercept and re-route via `SkipModelRequest` (`exceptions.py:116-131`), `SkipToolValidation`, `SkipToolExecution`. This is the documented escape hatch for non-ReAct cadence patterns (e.g. short-circuit mock model, redirect output tool to a sub-agent).
- **Reasoning is preserved, not stored** (`messages.py:1629-1678`, `docs/thinking.md:9-184`): `ThinkingPart` round-trips via provider signatures so multi-turn conversations can leverage the model's cache and continuation tokens. Pydantic AI does not maintain its own scratchpad.

## Notable Patterns

- **Three parallel execution modes** (`tool_manager.py:38-42`, `_agent_graph.py:1902-1956`): `'parallel'` (default, first-completed-yields-first), `'parallel_ordered_events'` (parallel execution, ordered yield), `'sequential'` (any tool with `sequential=True` forces this). A `ContextVar` (`tool_manager.py:40-42`) selects mode at runtime; per-tool `sequential=True` overrides (`tool_manager.py:163-164`).
- **Validation ahead of execution** (`_agent_graph.py:1715-1734`): every call is validated up-front before any tool runs; this lets `FunctionToolCallEvent.args_valid` reflect the truth even on async fan-out. Caching via `validated_calls: dict[str, ValidatedToolCall[DepsT]]` (`_agent_graph.py:1714`).
- **Capability short-circuit via exceptions** (`exceptions.py:80-150`): `CallDeferred`, `ApprovalRequired`, `SkipModelRequest`, `SkipToolValidation`, `SkipToolExecution` are control-flow exceptions, not errors. `process_tool_calls` routes them through `deferred_calls` (`_agent_graph.py:1700-1856`).
- **Deferred tool pipeline** (`_agent_graph.py:1747-1856`): if the run ends without resuming a deferred call, `DeferredToolRequests` becomes the agent's output; if a `HandleDeferredToolCalls` capability resolves them inline, the standard tool-execution pipeline handles the result. This keeps the Reason/Act/Observe pipeline identical for human-in-the-loop and synchronous flows.
- **Reasoning compaction through provider** (`messages.py:1681-1729`, `models/__init__.py:288-300`): instead of building a summarizer middleware, Pydantic AI delegates compaction to the provider. The result is opaque (`provider_details` for OpenAI, readable text for Anthropic) but round-trips correctly.

## Tradeoffs

- **No automatic compaction** of long tool results: users must write a `ProcessHistory` capability or rely on `Model.compact_messages`. Compared to LangGraph's `pre_model_hook` / `llm_input_messages` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-413`), Pydantic AI exposes the same hook surface but in a more capability-flavored form.
- **Per-tool retry budget does not auto-grow on `max_retries` retries**: the budget is `tool.max_retries` (defaults to `agent.retries` for tools, 1 by default per `tool_manager.py:92-93`). Misconfigured tools will hit `UnexpectedModelBehavior` fast.
- **`ModelRetry` is a runtime exception, not a state**: a tool that always raises `ModelRetry` cannot itself encode "I've now given up" — that's the caller's responsibility via `ctx.last_attempt` (`_run_context.py:155-156`).
- **Reasoning visibility depends on provider**: providers that return reasoning in `content` and don't surface it as a separate field will see Pydantic AI's `ThinkingPart` collapse into `TextPart`. Anthropic / OpenAI / Google / Bedrock all expose it as a separate channel; xAI Grok 3 Mini may not (`docs/thinking.md:51`).
- **`args_valid=None` semantics** (`messages.py:2817-2826`): "validation was not performed" overlaps with both success and failure, making downstream filters noisier than a strict bool.

## Failure Modes / Edge Cases

- **Truncated tool calls** (`_agent_graph.py:146-161`): if `ModelResponse.finish_reason == 'length'` and the last part is a `ToolCallPart` that can't be parsed, `GraphAgentState.check_incomplete_tool_call` raises `IncompleteToolCall` — clear feedback to the caller about the budget overrun.
- **Empty / thinking-only response** (`_agent_graph.py:1102-1175`): consumes one global retry, then either resubmits, recovers text from prior history, or asks the model for a different output format.
- **Unknown tool names** (`tool_manager.py:356-370`): retried via per-tool budget, then `UnexpectedModelBehavior`. Confirmed in `tests/test_agent.py:3897-3996`.
- **Deferred tool never resumed** (`_agent_graph.py:1849-1856`): if `output_type` does not include `DeferredToolRequests`, raises `UserError`. If included, becomes the agent output.
- **Cancellation mid-fan-out** (`_agent_graph.py:1947-1956`): sibling tasks are cancelled and drained via `cancel_and_drain` before re-raising. Streaming cancellation is handled separately in `stream()` (`_agent_graph.py:725-757`).
- **Per-call validation failure followed by execution failure** (`_agent_graph.py:1715-1734`, `tool_manager.py:730-738`): two separate events are emitted (`FunctionToolCallEvent(args_valid=False)` then a result event with the `RetryPromptPart`), so a UI can surface both.
- **Output tool validation failure with a `final_result` already set** (`_agent_graph.py:1580-1625`): produces a status `ToolReturnPart` ("Output tool not used - output failed validation.") instead of crashing the run, by end-strategy.
- **`ModelRetry` from a capability hook** (`capabilities/hooks.py:1054-1061`): the hook can convert a `ModelRetry` into a fresh error and let the retry loop continue, or swallow it. Documented in `capabilities/hooks.py:1054-1182`.

## Future Considerations

- A first-class observation-compaction capability (similar to OpenAI's `compact_messages` but provider-agnostic) would close the biggest gap versus LangGraph's `pre_model_hook`. The hook already exists; what's missing is a reference implementation in core.
- The `args_valid` event semantics could be tightened (`None` is overloaded) — distinguishing "not validated" from "validation was skipped on purpose" would make UIs cleaner.
- `ModelRetry` could be promoted to a richer signal (e.g. with structured `details: dict[str, Any]`) so downstream consumers don't have to parse the `message` string. The current `ValidationError` → `list[ErrorDetails]` path supports this; the `ModelRetry` string-message path doesn't.
- Native compaction is provider-specific today. A capability-level compaction abstraction would make it portable. Tracked in `messages.py:1683-1729` comments as a design choice, not a TODO.

## Questions / Gaps

- How are "soft" loop guards (e.g. "stop after N tool calls of type X") best expressed? `UsageLimits.tool_calls_limit` (`docs/agent.md:658-679`) handles total tool calls but not type-bucketed limits. No clear evidence found of a per-tool-bucket guard.
- How does Pydantic AI handle a `ToolCallPart` whose `args` JSON is malformed but `ModelResponse.finish_reason != 'length'`? `BaseToolCallPart.args_as_dict(raise_if_invalid=False)` (`messages.py:1826-1850`) defaults to returning `{INVALID_JSON_KEY: <raw args>}`; the path to `IncompleteToolCall` only fires when `finish_reason == 'length'`. No clear evidence found of a malformed-JSON-but-not-truncated guard.
- Is there any internal rate-limiting on observation size (e.g. capping a single tool return to N characters before it auto-truncates)? No clear evidence found; the user must truncate via `ToolReturn.content` shaping (`messages.py:1244-1297`).
- The `process_tool_calls` function is 320+ lines long (`_agent_graph.py:1537-1859`) with an `# noqa: C901` — opportunity for refactor into smaller typed helpers. Tracked as `agent_docs/code-simplification.md` policy, not as a TODO.
- Is there a documented pattern for "scratchpad" reasoning that survives across turns outside of `ThinkingPart`? The `HistoryProcessor` capability lets users inject structured reasoning into the next request, but no canonical pattern is shown.

---

Generated by `03.02-reason-act-observe-cadence` against `pydantic-ai`.
