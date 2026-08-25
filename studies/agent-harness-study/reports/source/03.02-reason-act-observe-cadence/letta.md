# Source Analysis: letta

## Reason/Act/Observe Cadence

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.11+ (async), Pydantic v2, OpenTelemetry; multi-provider LLM client (Anthropic, OpenAI, Google Vertex, Bedrock, SGLang, etc.) |
| Analyzed | 2026-07-13 |

Repository: `letta` v0.16.8 (per `sources/letta/pyproject.toml:3`). Three live agent loop generations coexist: the legacy `LettaAgent` (`letta/agents/letta_agent.py`), the current production `LettaAgentV2` (`letta/agents/letta_agent_v2.py`), and the newer stripped-down `LettaAgentV3` (`letta/agents/letta_agent_v3.py`). The factory `AgentLoop.load` in `letta/agents/agent_loop.py:18-62` selects among them based on `AgentType` and `enable_sleeptime`.

## Summary

Letta does **not** implement a textbook ReAct ("Thought/Action/Observation" three-tuple) loop with a visible scratchpad. Its architecture is closer to a *tool-calling agent with a heartbeat continuation pattern*: each iteration performs one LLM request (Reason), parses one or more tool calls from the response (Act), executes the tools, packages the results into `tool` role messages with status/timestamp and optional truncation (Observe), then decides whether to step again based on tool rules, tool-flagged heartbeats, or step count. Reasoning content is handled as **structured first-class content** (`ReasoningContent`, `RedactedReasoningContent`, `OmittedReasoningContent`, `SummarizedReasoningContent` in `letta/schemas/letta_message_content.py:261-320`), extracted by the per-provider streaming interface and persisted alongside assistant turns.

The closest ReAct-equivalent code path is `AgentType.react_agent` (string `"react_agent"` in `letta/schemas/enums.py:89`), which uses `letta/prompts/system_prompts/react.py:1-21` ("You are Letta ReAct agent..."). However, react agents do not get a separate codepath in the V3 loop; they share the same `_step`/`_handle_ai_response` machinery and rely on provider-side stripping of "request_heartbeat" and explicit tool-call chaining. The V3 loop's docstring at `letta/agents/letta_agent_v3.py:100-110` explicitly says "No heartbeats (loops happen on tool calls)", and `_decide_continuation` (`letta/agents/letta_agent_v3.py:1966-2036`) sets the rule "Called a tool? Loop continues" unconditionally. So the cadence is: **LLM call → tool(s) executed → results packaged → loop again until no tool call or terminal/limit hit**.

Observations are surfaced back to the model as standard OpenAI-style `role="tool"` messages containing `package_function_response(...)` JSON (`{"status": "OK"|"Failed", "message": ..., "time": ...}`) built in `letta/system.py:150-168`. These messages are prepended to the next iteration's `in_context_messages` via `_checkpoint_messages` (`letta/agents/letta_agent_v3.py:1405-1410`). Failed observations (rule violations, denials, execution errors) are converted into `ToolReturn(status="error")` objects with `func_response` reflecting the failure (e.g. `letta/agents/letta_agent_v3.py:1714-1746` for client denials, `letta/server/rest_api/utils.py:230-264` for user-denied tools, `letta/agents/helpers.py:501-505` for rule violations). These errors are then injected as the next tool message so the model sees them and can retry or pivot.

Reasoning visibility is configurable: `llm_config.enable_reasoner` (`letta/schemas/llm_config.py:79-81`), `max_reasoning_tokens` (`letta/schemas/llm_config.py:86-89`), and `reasoning_effort` (`letta/schemas/llm_config.py:82-85`) drive whether the provider's native thinking is requested; the adapter normalizes both native and omitted reasoning into `ReasoningContent`/`OmittedReasoningContent` (`letta/adapters/simple_llm_request_adapter.py:71-83`). `scrub_inner_thoughts_from_messages` in `letta/helpers/reasoning_helper.py:25-48` can erase past assistant `TextContent` from in-context history when reasoning is fully disabled, which keeps the model from being told about reasoning it never saw.

Compactability of observations: yes. Tool returns are truncated by both per-tool `return_char_limit` (validated by `letta/utils.py:898-939`) and a dynamic 20%-of-context-window cap from `LettaAgentV3._compute_tool_return_truncation_chars` (`letta/agents/letta_agent_v3.py:143-153`). Client-side tool returns are also JSON-aware truncated at the same cap (`letta/agents/letta_agent_v3.py:1714-1746`). When the conversation overflows, `compact()` (`letta/agents/letta_agent_v3.py:2077-2118`) summarizes older messages into a `role="summary"` system message via `letta/services/summarizer/compact.py`, after which `_refresh_messages(force_system_prompt_refresh=True)` rebuilds the system prompt (`letta/agents/letta_agent_v2.py:760-792`).

## Rating

**Score: 8 / 10** (Clear model with tests, explicit interfaces, and operational safeguards)

Rationale:
- (+) Mature, well-typed `Message` schema (`letta/schemas/message.py:252-300`) with explicit `tool_calls` / `tool_returns` fields and step/run/step_id linkage.
- (+) Strong distinction between *reasoning content* (provider-native, encrypted, omitted, summarized) and *text content* with first-class content-type tags.
- (+) Uniform single+parallel tool execution path with explicit `parallel_tool_calls` enforcement (`letta/agents/letta_agent_v3.py:1335-1342`, `letta/agents/letta_agent_v3.py:1111-1146`) and provider-specific gating for Anthropic/OpenAI/Gemini.
- (+) Observations are *reliably* fed back as tool messages with status, time, and error reason — every failure path (rule violation, user denial, run cancellation, client-side timeout, runtime exception) routes through `ToolExecutionResult(status="error")` or `ToolReturn(status="error")`.
- (+) Two-stage compaction (context-window-exceeded retry + post-step proactive check) with retries via `max_summarizer_retries` (`letta/agents/letta_agent_v3.py:1093`, `letta/agents/letta_agent_v3.py:1218-1294`).
- (+) Circuit-breaker / fallback router (`letta/services/llm_router/llm_router_client_base.py:14-97`, used at `letta/agents/letta_agent_v3.py:1048-1211`) handles transient LLM errors.
- (-) The "Reason" step is not exposed as a structured scratchpad — it is either provider-native thinking (encrypted/redacted) or `TextContent` mixed into the assistant content. There is no `Thought:` line analogous to ReAct's `Thought:` token.
- (-) The "Observe" message is a JSON-wrapped tool response, not a natural-language observation. Models must parse `{"status": "OK", "message": ..., "time": ...}` rather than seeing a plain-text narrative; this is pragmatic but not ReAct.
- (-) "Heartbeat" continuation is an internal control flow detail (a hidden user-role system message) rather than a first-class part of the cadence contract — `REQUEST_HEARTBEAT_PARAM` (`letta/constants.py:217-218`) is still wired into tool schemas in V2 and `force_set_request_heartbeat=True` (`letta/server/rest_api/utils.py:402`) rewrites it on the assistant message after the fact.
- (-) Most cadence behavior is documented by code comments and inline docstrings; there is no single architectural document that names the cadence pattern. The `AgentType.react_agent` value exists but its codepath is essentially a special case for stripping heartbeats and dropping memory blocks (`letta/services/agent_manager.py:408-409`, `letta/schemas/memory.py:694-728`) — it does not get a dedicated reasoning parser.

A score of 9-10 would require a clearly named "ReasoningTrace" object distinct from text, durable observation traces with provenance, and explicit tests covering partial failure observation paths.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent factory selecting loop generation | `AgentLoop.load` returns `LettaAgentV3` / `LettaAgentV2` / `SleeptimeMultiAgentV3` / `SleeptimeMultiAgentV4` based on `AgentType` and `enable_sleeptime` | `letta/agents/agent_loop.py:18-62` |
| Abstract loop interface | `BaseAgentV2.step` / `stream` / `build_request` declared | `letta/agents/base_agent_v2.py:35-104` |
| V3 _step method: full Reason→Act→Observe orchestration (run loop, checkpoint, build request, invoke LLM, extract tool_calls, call `_handle_ai_response`, persist, stream back, post-step compaction) | V3 `_step` orchestration | `letta/agents/letta_agent_v3.py:895-1592` |
| Loop driver (blocking) | `step()` calls `_step()` up to `max_steps`; breaks on `should_continue=False`, `cancelled`, `insufficient_credits`, `max_steps` | `letta/agents/letta_agent_v3.py:222-441` |
| Loop driver (streaming) | `stream()` yields SSE chunks per step | `letta/agents/letta_agent_v3.py:443-731` |
| Per-step LLM call inside V3 | Build request, retry on `LLMError`/`ContextWindowExceededError`, fallback routing | `letta/agents/letta_agent_v3.py:1092-1299` |
| Model output parsing: tool call extraction (single + parallel) | `if hasattr(... "tool_calls")` else fall back to `llm_adapter.tool_call`; truncate to first when `parallel_tool_calls=false` | `letta/agents/letta_agent_v3.py:1326-1342` |
| Tool-call envelope parsing (legacy V2) | `_safe_load_tool_call_str` with `}{` heuristic for Anthropic; pops `request_heartbeat` and `inner_thoughts` | `letta/agents/helpers.py:378-393`; `letta/agents/letta_agent_v2.py:1125-1127`, `letta/agents/letta_agent_v3.py:1773-1777` |
| Reasoning content extraction | Adapter sets `self.reasoning_content = [ReasoningContent(...)]` or `[OmittedReasoningContent()]` based on `message.reasoning_content` / `message.omitted_reasoning_content` | `letta/adapters/simple_llm_request_adapter.py:71-83` |
| Reasoning content types | `ReasoningContent`, `RedactedReasoningContent`, `OmittedReasoningContent`, `SummarizedReasoningContent` | `letta/schemas/letta_message_content.py:261-320` |
| Observation injection: package function response with status/time | `package_function_response` returns `{"status": "OK"/"Failed", "message": ..., "time": ...}` JSON | `letta/system.py:150-168` |
| Observation injection: build assistant + tool message pair from LLM response | `create_letta_messages_from_llm_response` builds assistant tool-call message and tool message with `ToolReturn` list | `letta/server/rest_api/utils.py:382-515` |
| Observation injection: parallel tool-call message (V3) | `create_parallel_tool_messages_from_llm_response` collapses N tool calls into one assistant + one tool message | `letta/server/rest_api/utils.py:518-627` |
| Observation injection: per-tool truncated return (V3 unified path) | Uses `validate_function_response(func_return, return_char_limit=…, truncate=…)` | `letta/agents/letta_agent_v3.py:1873-1892`; `letta/utils.py:898-939` |
| Failed observations: tool rule violation | Returns `ToolExecutionResult(status="error", func_return="[ToolConstraintError]…")` | `letta/agents/helpers.py:501-505` |
| Failed observations: user denial | `create_tool_returns_for_denials` wraps denials as `ToolReturn(status="error")` | `letta/server/rest_api/utils.py:230-264` |
| Failed observations: client-side return clamping | Truncates `func_response["message"]` to dynamic cap, logs warning | `letta/agents/letta_agent_v3.py:1714-1746` |
| Failed observations: client-side malformed approval | Logs error, sets `stop_reason=invalid_tool_call`, breaks | `letta/agents/letta_agent_v3.py:1009-1017` |
| Failed observations: cancellation | Auto-denial with reason `TOOL_CALL_DENIAL_ON_CANCEL` | `letta/constants.py:221` |
| Reasoning visibility controls | `enable_reasoner`, `max_reasoning_tokens`, `reasoning_effort` per-model | `letta/schemas/llm_config.py:79-85` |
| Reasoning scrubbing (hidden by config) | `scrub_inner_thoughts_from_messages` removes `TextContent` from assistant messages when reasoning fully disabled | `letta/helpers/reasoning_helper.py:25-48`; called from `letta/agents/letta_agent_v2.py:791` |
| Continuation decision (V3 rules) | "No tool call → end_turn (or heartbeat if required tool uncalled); tool call → always continue; child tool / continue tool → continue; terminal tool → stop; max_steps → stop" | `letta/agents/letta_agent_v3.py:1966-2036` |
| Continuation decision (V2) | Heartbeat-driven: `continue_stepping = request_heartbeat`; rule violations force continue; required-before-exit tools force continue | `letta/agents/letta_agent_v2.py:1240-1308` |
| Heartbeat / inner-thoughts constants | `REQUEST_HEARTBEAT_PARAM`, `REQUEST_HEARTBEAT_DESCRIPTION`, `NON_USER_MSG_PREFIX`, `REQ_HEARTBEAT_MESSAGE`, `FUNC_FAILED_HEARTBEAT_MESSAGE` | `letta/constants.py:217-244, 452-455` |
| Heartbeat system message generation | `create_heartbeat_system_message` builds a `role="user"` hidden system message used to keep the loop alive between turns | `letta/server/rest_api/utils.py:630-655` |
| Dynamic tool-return truncation cap | `_compute_tool_return_truncation_chars`: 20% of context window × 4 chars/token, min 5000 | `letta/agents/letta_agent_v3.py:143-153` |
| Observation compaction (post-step) | If `context_token_estimate > compaction_trigger_threshold`, call `compact()` and rebuild system prompt | `letta/agents/letta_agent_v3.py:1438-1510` |
| Observation compaction (context-window-exceeded retry) | Catch `ContextWindowExceededError`, call `compact()`, retry LLM request, up to `max_summarizer_retries` | `letta/agents/letta_agent_v3.py:1218-1294` |
| Compaction entry point | `compact()` invokes `compact_messages` (summarizer service) | `letta/agents/letta_agent_v3.py:2077-2120` |
| Tool rule solver driving continue/stop | `should_force_tool_call`, `register_tool_call`, `is_terminal_tool`, `has_children_tools`, `is_continue_tool` | `letta/helpers/tool_rule_solver.py:88, 174, 178, 182, 273` |
| Approval flow (HITL) as a non-ReAct observation branch | `create_approval_request_message_from_llm_response`, `_maybe_get_approval_messages`, `_maybe_get_pending_tool_call_message` | `letta/server/rest_api/utils.py:304-371`; `letta/agents/helpers.py:522-542`; `letta/agents/letta_agent_v3.py:973-1035` |
| Step lifecycle checkpoint | `StepProgression` enum drives persistence + telemetry; `_step_checkpoint_start`, `_step_checkpoint_llm_request_start`, `_step_checkpoint_llm_request_finish`, `_step_checkpoint_finish` | `letta/agents/letta_agent_v2.py:941-996` |
| Stop reasons enum | `end_turn`, `max_steps`, `tool_rule`, `no_tool_call`, `invalid_tool_call`, `invalid_llm_response`, `llm_api_error`, `cancelled`, `requires_approval`, `context_window_overflow_in_system_prompt`, `insufficient_credits`, `error` | `letta/schemas/letta_stop_reason.py:9-22` |
| LLM routing fallback (failed observation of provider) | `LLMRoutingClient` with `record_failure`, `record_success`, `get_fallback_handle` | `letta/services/llm_router/llm_router_client_base.py:14-97`; used at `letta/agents/letta_agent_v3.py:1048-1211` |
| Per-provider message conversion (Anthropic react path) | `native_content=is_v1`, `strip_request_heartbeat=is_v1`, `merge_tool_results_into_user_messages`, `dedupe_tool_results_in_user_messages`, `merge_heartbeats_into_tool_responses` | `letta/llm_api/anthropic_client.py:670-714` |
| ReAct agent type, system prompt, no memory blocks | `AgentType.react_agent`, `react.PROMPT`, `services/agent_manager.py:408-409` (no default tools), `schemas/memory.py:694-728` (no memory blocks, `_render_directories_react`) | `letta/schemas/enums.py:89`; `letta/prompts/system_prompts/react.py:1-21`; `letta/services/agent_manager.py:408-409`; `letta/schemas/memory.py:638, 694-728` |

## Answers to Dimension Questions

1. **How does the model decide actions?**
   The model emits one or more `tool_calls` (OpenAI-style function calls) in the assistant message. After streaming completes, the adapter extracts `tool_calls` (V3 `letta/agents/letta_agent_v3.py:1328-1333`) and the orchestrator enforces `parallel_tool_calls` per-provider (`letta/agents/letta_agent_v3.py:1118-1146`, `1335-1342`). In V2 the model can additionally emit `request_heartbeat=true|false` as a kwarg to control chaining (`letta/agents/letta_agent_v2.py:1126`, `letta/constants.py:217-218`); V3 has removed this in favor of always continuing on tool calls (V3 `_decide_continuation` comment, `letta/agents/letta_agent_v3.py:1977-1986`). Tool-rules constraints may force a single tool call (`should_force_tool_call`, `letta/helpers/tool_rule_solver.py:273-294`) and the V3 `_get_valid_tools` (`letta/agents/letta_agent_v3.py:2039-2074`) restricts the tool list by `ToolRulesSolver.get_allowed_tool_names`. For approval-required tools (`is_requires_approval_tool`) or client-side tools, `_handle_ai_response` returns `requires_approval` instead of executing (`letta/agents/letta_agent_v3.py:1682-1709`).

2. **How are observations returned?**
   Every tool execution produces a `ToolExecutionResult(status: "success"|"error", func_return, stdout, stderr)` (`letta/schemas/tool_execution_result.py:8-18`) which is normalized into a string/dict by `validate_function_response` (`letta/utils.py:898-939`) with optional per-tool truncation, then wrapped by `package_function_response` into `{"status": "OK"/"Failed", "message": ..., "time": ...}` JSON (`letta/system.py:150-168`). This JSON is written as `Message(role="tool", content=[TextContent(...)])` plus a `tool_returns=[ToolReturn(...)]` payload by either `create_letta_messages_from_llm_response` (single, `letta/server/rest_api/utils.py:382-515`) or `create_parallel_tool_messages_from_llm_response` (parallel, `letta/server/rest_api/utils.py:518-627`). The message is appended to `in_context_messages` via `_checkpoint_messages` (`letta/agents/letta_agent_v3.py:1405-1410`) so it appears at the head of the next LLM request.

3. **Is reasoning visible or hidden?**
   Both, depending on configuration. Native reasoning (Anthropic thinking, OpenAI Responses reasoning summaries) is requested via `llm_config.enable_reasoner` and `max_reasoning_tokens` (`letta/schemas/llm_config.py:79-89`) and is stored as `ReasoningContent` / `SummarizedReasoningContent` / `OmittedReasoningContent` / `RedactedReasoningContent` (`letta/schemas/letta_message_content.py:261-320`). Anthropic-specific "thinking" can also be injected as `INNER_THOUGHTS_KWARG` in tool args when `put_inner_thoughts_in_kwargs=True` (`letta/agents/letta_agent.py:1048-1057`, `letta/local_llm/constants.py:8-11`). When reasoning is fully disabled, `scrub_inner_thoughts_from_messages` strips assistant `TextContent` from in-context history (`letta/helpers/reasoning_helper.py:25-48`) so the model never sees reasoning it did not generate.

4. **Are failed observations fed back?**
   Yes, in five explicit ways:
   - **Tool rule violation** → `ToolExecutionResult(status="error", func_return="[ToolConstraintError] Cannot call {name}, valid tools include: …")` (`letta/agents/helpers.py:501-505`).
   - **User denial of approval** → `ToolReturn(status="error", func_response="Error: request to call tool denied. User reason: …")` (`letta/server/rest_api/utils.py:230-264`).
   - **Client-side tool return** truncation when over the dynamic cap, with a `... [truncated N chars]` suffix and warning log (`letta/agents/letta_agent_v3.py:1714-1746`).
   - **Malformed approval** → logs an error, sets `stop_reason=invalid_tool_call`, breaks the loop (`letta/agents/letta_agent_v3.py:1009-1017`).
   - **Provider failure / LLM error** → classified into `llm_api_error` / `invalid_llm_response` / `no_tool_call` and persisted; the next iteration retries with a compacted context or falls back via the LLM router (`letta/agents/letta_agent_v3.py:1175-1294`).
   The V3 `_decide_continuation` always continues when a tool was called (`letta/agents/letta_agent_v3.py:1977-1986`), so a failed tool result is automatically re-presented on the next LLM call.

5. **Can observations be compacted?**
   Yes at three levels:
   - **Per-call truncation** by `return_char_limit` from `Tool.return_char_limit` (`letta/utils.py:898-939`, `letta/agents/letta_agent_v3.py:1873-1884`).
   - **Dynamic context cap** of ~20% of the model's context window via `_compute_tool_return_truncation_chars` (`letta/agents/letta_agent_v3.py:143-153`).
   - **Conversation-level compaction** triggered either reactively when `ContextWindowExceededError` is raised (`letta/agents/letta_agent_v3.py:1218-1294`) or proactively when `context_token_estimate > compaction_trigger_threshold` after a step (`letta/agents/letta_agent_v3.py:1438-1510`). `compact()` (`letta/agents/letta_agent_v3.py:2077-2118`) calls `compact_messages` in `letta/services/summarizer/compact.py`, which replaces older messages with a summary role message via `package_summarize_message_no_counts` (`letta/system.py:207-236`). The system prompt is then rebuilt from refreshed memory (`letta/agents/letta_agent_v3.py:1255-1262`, `1478-1485`).

## Architectural Decisions

- **OpenAI-style tool-calling as the lingua franca.** Every provider client (Anthropic, Google Vertex, OpenAI, Bedrock, SGLang) implements `build_request_data` + `convert_response_to_chat_completion` so the agent loop sees a uniform `ToolCall` list. The `Message` schema carries OpenAI-format `tool_calls`/`tool_call_id` directly (`letta/schemas/openai/chat_completion_response.py:16-25`, `letta/schemas/message.py:287-290`).

- **First-class structured reasoning content.** Rather than stuffing thinking into `text`, the schema distinguishes `ReasoningContent` / `RedactedReasoningContent` / `OmittedReasoningContent` / `SummarizedReasoningContent` from `TextContent` (`letta/schemas/letta_message_content.py:261-320`). This is necessary because Anthropic, Gemini, and OpenAI Responses all return different shapes and Letta preserves provenance.

- **One-step-per-iteration semantics.** `_step` performs *one* LLM call + N tool executions and then yields control back to the outer `step()` loop (`letta/agents/letta_agent_v3.py:328-396`). The outer loop enforces `max_steps`, credit checks, cancellation, and stop reason aggregation. This separation makes per-step telemetry clean.

- **Step IDs everywhere.** Every step creates a UUID-keyed `Step` row before the LLM call (`_step_checkpoint_start`, `letta/agents/letta_agent_v2.py:941-966`), and messages, tool calls, and tool returns are tagged with `step_id` and `run_id` (`letta/schemas/message.py:292-293`).

- **Heartbeat as user-role hidden message (V2) vs no-heartbeat chaining (V3).** V2 inserts a `role="user"` heartbeat message carrying `[This is an automated system message hidden from the user] ...` between iterations so the model "sees" a turn boundary (`letta/server/rest_api/utils.py:630-655`, `letta/constants.py:244, 452-455`). V3 skips this and chains purely on tool-call presence (`letta/agents/letta_agent_v3.py:100-110`, `1966-2036`). This is a documented divergence.

- **Two-tier error packaging.** Tool-runtime errors go into `ToolExecutionResult(status)`; user/policy denials go into `ToolReturn(status)` created outside the executor. Both feed `package_function_response` so the model's view of failure is uniform JSON.

- **Per-provider parallel-tool gating.** Anthropic/OpenAI/Gemini all set `disable_parallel_tool_use` / `parallel_tool_calls` flags at request build time, gated by tool-rules presence (`letta/agents/letta_agent_v3.py:1111-1146`). V3 also enforces truncation client-side when a provider ignores the flag (`letta/agents/letta_agent_v3.py:1335-1342`).

## Notable Patterns

- **Auto-handle routing with circuit breaker fallback.** `_step` resolves `agent_state.llm_config.handle` against `AUTO_MODE_HANDLES`; if routing fails or the provider raises a rate-limit/server error, the router records the failure, swaps to the fallback config, and retries the same step (`letta/agents/letta_agent_v3.py:1042-1211`, `letta/services/llm_router/llm_router_client_base.py:42-93`).

- **Parallel + serial tool co-execution.** `_handle_ai_response` partitions tool calls by `target_tool.enable_parallel_execution` and uses `asyncio.gather` for the parallel subset (`letta/agents/letta_agent_v3.py:1843-1862`).

- **Pre-merge of prefilled tool args.** Tool rules can inject arguments the model does not control; `merge_and_validate_prefilled_args` validates against JSON Schema before the call (`letta/agents/helpers.py:465-493`, called at `letta/agents/letta_agent_v3.py:1784-1809`).

- **Streaming chunk duality.** Token-level streaming is attempted when the adapter `supports_token_streaming()`; otherwise the entire `(reasoning + tool call + tool return)` is yielded per step (`letta/agents/letta_agent_v3.py:1413-1436`, `base_agent_v2.py:95-98`).

- **Per-provider message coalescing.** Anthropic merges consecutive tool results into a single user message (`merge_tool_results_into_user_messages`, `letta/llm_api/anthropic_client.py:688`) and dedupes duplicate `tool_use_id` references (`letta/llm_api/anthropic_client.py:706-709`) — providers impose their own constraint that Letta adapts to in `Observe`.

- **Heartbeat / pre-execution-message arg injection.** Tools can be augmented with a `pre_exec_msg` field (`letta/helpers/tool_execution_helper.py:165-213`) and `request_heartbeat` field (`letta/helpers/tool_execution_helper.py:216-243`, `letta/constants.py:217-218`). V2 forces `request_heartbeat=True` on the assistant message after the call so the next loop iteration sees a heartbeat (`letta/server/rest_api/utils.py:411`).

- **Step-boundary checkpointing with state machine.** `StepProgression` enum tracks START → STEP_LOGGED → LOGGED_TRACE → FINISHED (`letta/schemas/step.py`, used in `letta/agents/letta_agent_v3.py:940-1592`) so partial-failure cleanup always writes the right metric and stop reason.

- **Approval/denial as first-class message role.** `role="approval"` is used for HITL request/response messages (`letta/server/rest_api/utils.py:355-365`) with a special handling branch that re-injects pending tool calls from the assistant message (`_maybe_get_pending_tool_call_message`, `letta/agents/helpers.py:530-542`).

## Tradeoffs

- **JSON-wrapped tool observations vs plain text.** `package_function_response` produces a JSON envelope around the actual return (`letta/system.py:150-168`). This is structured (machine-parseable for downstream rendering) but the model must parse JSON to read its own observations, which is mildly error-prone for smaller models.

- **Single observation per tool call (no streaming partials).** A long tool call is awaited fully, then a single `tool` message is appended (`letta/agents/letta_agent_v3.py:1822-1862`). No partial/progressive observation surface.

- **Hidden heartbeat message vs explicit reasoning.** V2 uses a `role="user"` hidden message with `NON_USER_MSG_PREFIX` (`letta/constants.py:244`) which is invisible to the user but counts as a model "turn" — this conflates control flow with conversation flow and can confuse prompt-level log analysis. V3 deliberately removes it.

- **Truncation is silent.** When tool returns exceed `return_char_limit`, the suffix `... [NOTE: function output was truncated since it exceeded the character limit (X > Y)]` is appended (`letta/utils.py:925-937`) but there is no metadata flag persisted on the `ToolReturn` indicating truncation occurred — only a logger warning.

- **ReAct agent type is not a true ReAct codepath.** Despite the name and dedicated system prompt (`letta/prompts/system_prompts/react.py:1-21`), react agents share the same `_step` machinery as memgpt agents. Their only special-cases are "no default tools", "no memory blocks", and provider-side stripping of `request_heartbeat` (`letta/services/agent_manager.py:408-409`, `letta/schemas/memory.py:694-728`, `letta/llm_api/anthropic_client.py:677-679`).

- **Inner thoughts via `inner_thoughts` kwarg vs free-form text.** V1 supports both `put_inner_thoughts_in_kwargs=True` (model emits `inner_thoughts` as a function arg) and reasoning API; V2/V3 only use the latter, with a backward-compat `args.pop(INNER_THOUGHTS_KWARG, None)` (`letta/agents/letta_agent_v2.py:1127`, `letta/agents/letta_agent_v3.py:1777`).

- **Streaming interface has multiple specialized classes.** `AnthropicStreamingInterface`, `SimpleAnthropicStreamingInterface`, `OpenAIStreamingInterface`, `OpenAIChatCompletionsStreamingInterface`, `GeminiStreamingInterface` (`letta/interfaces/`). Each implements `get_tool_call_object(s)` and `get_content()` differently. Reasoning extraction is duplicated code across these.

- **Compactible but not summarizable per-tool-call.** Compaction operates on message arrays, not on individual `ToolReturn` payloads. A single 50 MB tool return can blow the context window before compaction triggers, even though `_compute_tool_return_truncation_chars` mitigates this with a 20% cap.

## Failure Modes / Edge Cases

- **Empty LLM response.** `LLMEmptyResponseError` → `stop_reason=invalid_llm_response` (`letta/agents/letta_agent_v3.py:1180-1182`). Not retried.

- **Malformed tool arguments.** `_safe_load_tool_call_str` falls back to `{}` on JSON decode error and logs `Failed to JSON decode tool call argument string` (`letta/agents/helpers.py:378-393`). Silent: the tool will then be called with empty args.

- **Tool rule violation in tool args.** V3 returns `ToolExecutionResult(status="error", func_return="[ToolConstraintError]…")` and sets `continue_stepping=True` (`letta/agents/letta_agent_v3.py:1826`, `_decide_continuation` 1977-2036). The error is fed back to the model.

- **Pre-merge of prefilled args fails validation.** A `ValueError` from `merge_and_validate_prefilled_args` becomes a per-tool `exec_spec` with `"error"` set and `continue_stepping=False`, `stop_reason=invalid_tool_call` (`letta/agents/letta_agent_v3.py:1794-1809`, `1899-1902`).

- **No tool call from LLM but required-before-exit tool uncalled.** V3 synthesizes a heartbeat reason `ToolRuleViolated: You must call {tools} at least once to exit the loop.` (`letta/agents/letta_agent_v3.py:1626-1647`); `_decide_continuation` returns `True, reason, None`.

- **Provider returns `finish_reason=length`.** Triggers `stop_reason=max_tokens_exceeded` (`letta/agents/letta_agent_v3.py:1999-2002`).

- **Job cancelled mid-step.** `_check_run_cancellation(run_id)` at step start sets `stop_reason=cancelled` (`letta/agents/letta_agent_v3.py:1031-1035`). Auto-denial of pending tool calls uses `TOOL_CALL_DENIAL_ON_CANCEL` (`letta/constants.py:221`).

- **Context window exceeded mid-LLM-call.** Caught, triggers compaction+retry up to `max_summarizer_retries` (`letta/agents/letta_agent_v3.py:1218-1294`). After retries exhausted, `stop_reason=error`.

- **System prompt itself too large after compaction.** `SystemPromptTokenExceededError` → `stop_reason=context_window_overflow_in_system_prompt`, no retry (`letta/agents/letta_agent_v3.py:1285-1290`).

- **Insufficient credits.** Detected between iterations via `_check_credits()` and sets `stop_reason=insufficient_credits` (`letta/agents/letta_agent_v3.py:334-339`).

- **Approval with no pending tool call.** `_prepare_in_context_messages_no_persist_async` raises `ValueError` after idempotency check (`letta/agents/helpers.py:287-293`). Idempotency check looks back at the most recent tool message to avoid retry loops.

- **Empty approvals payload.** All three lists (tool_calls / denials / returns) empty → `stop_reason=invalid_tool_call` and break (`letta/agents/letta_agent_v3.py:1009-1017`).

- **Streaming chunk adapter exception.** `tool_calls`, `content`, `usage`, `message_id`, `logprobs` are all wrapped in `try/except` defaults to `[]` / `None` (`letta/adapters/simple_llm_stream_adapter.py:206-235`), so a partial extraction does not crash the loop.

- **Provider fails after one successful LLM call.** No automatic retry of the *just-failed* call; the step raises and the outer loop catches it (`letta/agents/letta_agent_v3.py:1511-1540`). The next user message gets a clean retry path.

## Future Considerations

- **Reasoning Trace as a first-class object.** Today, reasoning is one of four content types inside the assistant message. A dedicated `ReasoningTrace` with `text`, `signature`, `redacted`, `summaries`, `omitted` as variants could simplify the four content classes and make reasoning more queryable in the API.

- **Streaming partial observations.** Long tool calls (e.g. shell, sandbox exec) currently produce a single block observation. Streaming partial observations (like `tool_exec.delta`) would let the model see progress and react.

- **Truncation metadata on `ToolReturn`.** Persist a `truncated: bool` and `original_len: int` on `ToolReturn` so UIs and analytics can flag observations that were clipped.

- **Unified ReAct codepath.** The `react_agent` enum exists but the runtime treats it identically to memgpt. Either remove the type or implement a true single-message Thought/Action/Observation loop with structured `Thought:` tokens.

- **Provider-agnostic compaction trigger.** `compaction_trigger_threshold` is global; per-conversation / per-tool overrides are not exposed (`letta/agents/letta_agent_v3.py:938`, `letta/services/summarizer/thresholds.py`).

- **Observation audit trail.** Tool returns are persisted in `Message.tool_returns`, but only the latest aggregated `TextContent` is rendered (`letta/server/rest_api/utils.py:619` for parallel, `letta/server/rest_api/utils.py:479` for single). A dedicated `tool_return` event stream with timestamps would help debugging.

- **Reasoning content compaction.** When context overflows, the summarizer is fed the full message list including `ReasoningContent`. For native thinking models (Anthropic, Gemini 2.5), this is wasted tokens since the model cannot reuse its prior thinking; a summarizer aware of reasoning-only blocks could discard or summarize them separately.

- **Approval flow consolidation.** Approval messages have special-case branches across V2 and V3 (`_maybe_get_pending_tool_call_message`, `_maybe_get_approval_messages`, `create_approval_request_message_from_llm_response`). A single approval-state machine would reduce the surface area.

## Questions / Gaps

- **Is there a documented cadence contract?** No single document names the Reason/Act/Observe pattern. Cadence is reconstructed from `_step`, `_handle_ai_response`, `_decide_continuation`, and the rest_api utils. The docstring at `letta/agents/letta_agent_v3.py:100-110` is the closest narrative but is brief.

- **What is the contract for `AgentType.react_agent`?** The enum exists (`letta/schemas/enums.py:89`), the prompt exists (`letta/prompts/system_prompts/react.py:1-21`), and the manager excludes default tools (`letta/services/agent_manager.py:408-409`), but the loop logic does not branch on it. The only remaining react-specific behaviors are provider-side: `native_content=is_v1` and `strip_request_heartbeat=is_v1` (`letta/llm_api/anthropic_client.py:677-679`). The commented-out branch in V3 (`letta/agents/letta_agent_v3.py:1431`) suggests react once had a separate rendering path that has since been removed.

- **Are tools ever given partial / streaming observations?** No evidence found. Tool execution is awaited fully (`letta/agents/letta_agent_v3.py:1822-1862`), then one observation is produced.

- **Is reasoning ever persisted without signature?** When `OmittedReasoningContent` is used (e.g., OpenAI GPT-5 on ChatCompletions), the reasoning is dropped and only a placeholder remains (`letta/schemas/letta_message_content.py:285-294`, `letta/adapters/simple_llm_request_adapter.py:79-83`). The model cannot recover the omitted reasoning on later turns — this is by design but worth flagging.

- **Are tool returns ever summarized in place?** No evidence found in the loop. Compaction summarizes whole messages, not individual `ToolReturn` payloads. A very large `func_response` is truncated at `_compute_tool_return_truncation_chars` (`letta/agents/letta_agent_v3.py:143-153`) but never summarized.

- **Where is reasoning content rendered to the user?** The `ReasoningContent` survives into the message stream, but the V3 default `text_is_assistant_message=True` collapses reasoning+text into a single assistant message in streaming (`letta/agents/letta_agent_v3.py:1427-1433`). Whether the user-facing render separates them is the caller's responsibility (`to_letta_messages_from_list`).

- **Where do `compaction_messages` go in non-summary mode?** When `include_compaction_messages=False` (default), `EventMessage` and `SummaryMessage` are constructed but not yielded (`letta/agents/letta_agent_v3.py:1229-1234, 1274-1282, 1451-1498`). The summary is still persisted.

---

Generated by `reports/repo/03.02-reason-act-observe-cadence/{repo-name}.md` against `letta`.