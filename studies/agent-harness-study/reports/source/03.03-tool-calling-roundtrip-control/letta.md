# Source Analysis: letta

## 03.03 — Tool-Calling Roundtrip Control

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.x, Pydantic v2, asyncio; FastAPI server; OpenAI/Anthropic/Gemini/MiniMax/Bedrock/Together/Fireworks/Groq/DeepSeek/xAI/OpenRouter/OpenAI-Responses provider clients |
| Analyzed | 2026-07-14 |

## Summary

Letta implements an OpenAI-shaped tool-calling pipeline that is layered rather than monolithic. The core data model is `OpenAIToolCall(id, function.name, function.arguments)` (`letta/schemas/openai/chat_completion_response.py:16-22`, `letta/schemas/openai/chat_completion_request.py:23`). Tool calls are accumulated token-by-token during streaming into provider-specific streaming interfaces — `SimpleOpenAIStreamingInterface.get_tool_call_object` (`letta/interfaces/openai_streaming_interface.py:194-205`), `SimpleAnthropicStreamingInterface.get_tool_call_object` (`letta/interfaces/anthropic_streaming_interface.py:668`), and the parallel variant `AnthropicParallelToolCallStreamingInterface.get_tool_call_objects` (`letta/interfaces/anthropic_parallel_tool_call_streaming_interface.py:137-155`). The streaming adapter then prefers parallel extraction (`SimpleLLMStreamAdapter._extract_tool_calls`, `letta/adapters/simple_llm_stream_adapter.py:34-50`) and falls back to a single-call extractor.

Argument extraction is deliberately lenient: `_safe_load_tool_call_str` (`letta/agents/helpers.py:378-393`) splits on `}{` to clip accidental concatenation, double-decodes non-dict JSON, and replaces malformed payloads with `{}` while logging. Type coercion happens at execution time inside the sandbox via `coerce_dict_args_by_annotations` (`letta/functions/ast_parsers.py:117-168`) which parses strings to JSON or `ast.literal_eval` and re-projects typed containers (`list`/`dict`/`tuple`/`set`).

Execution is mediated by `ToolExecutionManager` (`letta/services/tool_executor/tool_execution_manager.py:68-160`), which selects a `ToolExecutor` subclass per `ToolType` (`LETTA_CORE`, `LETTA_MEMORY_CORE`, `LETTA_SLEEPTIME_CORE`, `LETTA_MULTI_AGENT_CORE`, `LETTA_BUILTIN`, `LETTA_FILES_CORE`, `EXTERNAL_MCP`, defaulting to `SandboxToolExecutor`) via `ToolExecutorFactory._executor_map` (`letta/services/tool_executor/tool_execution_manager.py:35-65`). Each executor returns a `ToolExecutionResult(status: "success"|"error", func_return, agent_state, stdout, stderr, sandbox_config_fingerprint)` (`letta/schemas/tool_execution_result.py:8-18`); the manager catches every exception and emits a structured error result (`letta/services/tool_executor/tool_execution_manager.py:131-155`) rather than raising. Result payload formatting goes through `validate_function_response` (`letta/letta/utils.py:898-939`) — dicts pass through, strings are truncated to a per-tool `return_char_limit` (`letta/schemas/tool.py:51-56`), then double-wrapped by `package_function_response` (`letta/letta/system.py:150-168`) into `{status, message, time}` JSON.

The repair surface is limited but explicit: tool-rule violations are synthesized as `ToolExecutionResult(status="error")` via `_build_rule_violation_result` (`letta/agents/helpers.py:501-505`) and surfaced to the model as JSON; unknown tool names from `valid_tools` (`letta/agents/letta_agent_v3.py:1780`) short-circuit to the same error path; and the streaming layer can miss a tool-call ID entirely (`get_tool_call_object` raises `ValueError("No tool call ID available")` at `letta/interfaces/openai_streaming_interface.py:198-201`), which is swallowed by the stream adapter's `try/except` at `letta/adapters/letta_llm_stream_adapter.py:185-187`. Tool errors never crash a run — `aggregate_continue` is `True` whenever a tool call is produced (`letta/agents/letta_agent_v3.py:1945-1946`, `_decide_continuation` at `letta/agents/letta_agent_v3.py:1967-2036`). Pre-execution gates include a Human-in-the-Loop approval channel implemented as `RequiresApprovalToolRule` (`letta/schemas/tool_rule.py:348`) plus `client_tools` (`letta/agents/letta_agent_v3.py:1684-1696`), both of which emit `create_approval_request_message_from_llm_response` (`letta/server/rest_api/utils.py:304-371`) and pause the loop with `StopReasonType.requires_approval` (`letta/agents/letta_agent_v3.py:1709`). Denials are converted to `ToolReturn` objects (`create_tool_returns_for_denials`, `letta/agents/letta_agent_v3.py:1756-1762`).

The implementation is large, multi-versioned (v1 in `letta/agent.py`, plus `letta_agent_v2.py`, `letta_agent_v3.py`, `letta_agent_batch.py`), and has multiple parallel paths (single vs. parallel tool calls, server vs. client-side tools, with vs. without `client_tools`). Tests exist for `schema_validator.py` (`letta/tests/test_schema_validator.py:1-278`) and full sandbox scenarios (`letta/tests/integration_test_tool_execution_sandbox.py:1-758`) but there are no tests for `_safe_load_tool_call_str` or for the streaming adapters' tool-call extractors. The pipeline surfaces the model-visible contract clearly — tool calls are `OpenAIToolCall` objects with deterministic `tool_call_id`, failures are `ToolExecutionResult(status="error")`, results are `{status, message, time}` JSON — but the paths are not idempotent: a missing `tool_call_id` becomes `None`, dropped responses become empty tool messages, and a corrupt approval response silently fails the run (`letta/agents/letta_agent_v3.py:1009-1017`).

## Rating

**6/10** — A clear model with explicit interfaces and operational safeguards, but inconsistent across versions, lenient in unsafe places (silent `{}` on JSON decode failure, swallowed extraction errors), and not unit-tested at the extraction/parsing seams where most malformed-tool-call bugs live.

Rubric anchor: scores 7-8 require "Clear model with tests, explicit interfaces, and operational safeguards"; 4-6 is "Present but inconsistent, weakly documented, or fragile." Letta earns its score from explicit `ToolExecutionResult` schema, central `_execute_tool`/`ToolExecutionManager` indirection with metrics emission, OTel-traced execution, and the result-formatting pipeline (`validate_function_response` → `package_function_response`), but loses ground because the `_safe_load_tool_call_str` path silently demotes malformed JSON to `{}`, the streaming-id extractor raises raw `ValueError` and is swallowed in `try/except`, there are three concurrent agent loop implementations (`letta_agent.py`, `letta_agent_v2.py`, `letta_agent_v3.py`), and there is no unit test for the parse/repair edge cases.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool-call data model | `OpenAIToolCall(id, function, type="function")` shape reused as the canonical in-memory tool-call | `letta/schemas/openai/chat_completion_response.py:16-22` |
| Tool-call request schema | Request-side mirror used in adapter transforms | `letta/schemas/openai/chat_completion_request.py:18-26` |
| Tool-call streaming dataclass | Internal `ToolCall`/`ToolCallDelta` excludes `None` to mimic OpenAI chunk format | `letta/schemas/letta_message.py:199-220` |
| Tool-call message wrapper | `ToolCallMessage` with single + list representations | `letta/schemas/letta_message.py:222-259` |
| Tool execution result schema | `ToolExecutionResult(status, func_return, agent_state, stdout, stderr, sandbox_config_fingerprint)` with `success_flag` property | `letta/schemas/tool_execution_result.py:8-18` |
| Tool definition entity | `Tool` with `name`, `json_schema`, `return_char_limit`, `default_requires_approval`, `enable_parallel_execution` | `letta/schemas/tool.py:35-69` |
| Strict-mode schema eligibility check | OpenAI strict-mode health classifier (`STRICT_COMPLIANT`/`NON_STRICT_ONLY`/`INVALID`) used before strict-mode rewriting | `letta/functions/schema_validator.py:12-202` |
| Strict-mode schema rewriting | `enable_strict_mode` adds nullable fields, sets `additionalProperties=False`, `required=[...]` | `letta/helpers/tool_execution_helper.py:89-162` |
| Strict-mode deep property processor | `_process_property_for_strict_mode` recurses through objects/arrays/anyOf | `letta/helpers/tool_execution_helper.py:43-86` |
| Lenient JSON load (single-line, parallel-hack fallback, re-decode non-dict) | `_safe_load_tool_call_str`: strips parallel-call artifacts (`}{`), demjson-free re-decoding, returns `{}` on `JSONDecodeError` | `letta/agents/helpers.py:378-393` |
| Merge of prefilled (rule) args over LLM args with schema check | `merge_and_validate_prefilled_args` validates keys exist + basic type/enum/const/anyOf match | `letta/agents/helpers.py:465-493` |
| Lightweight JSON-Schema value check | `_schema_accepts_value` resolves const/enum/anyOf/oneOf, then `_json_type_matches` | `letta/agents/helpers.py:396-462` |
| Tool-rule violation synthesis | `_build_rule_violation_result` returns `ToolExecutionResult(status="error")` with hint | `letta/agents/helpers.py:501-505` |
| Heartbeat arg extraction | `_pop_heartbeat` reads and pops `request_heartbeat`, normalizes to bool | `letta/agents/helpers.py:496-498` |
| Approval message detection (legacy) | `_maybe_get_approval_messages` and `_maybe_get_pending_tool_call_message` | `letta/agents/helpers.py:522-542` |
| Single-call streaming tool-call extractor | OpenAI ChatCompletions: `get_tool_call_object` raises `ValueError` if no `tool_call_id` | `letta/interfaces/openai_streaming_interface.py:194-205` |
| Anthropic parallel streaming extractor | `get_tool_call_objects` returns ordered list, `_tool_use_start_order` sort | `letta/interfaces/anthropic_parallel_tool_call_streaming_interface.py:137-148` |
| Anthropic legacy single-call extractor | `get_tool_call_object` returns `Optional[ToolCall]` | `letta/interfaces/anthropic_streaming_interface.py:668` |
| Stream adapter extraction logic | `SimpleLLMStreamAdapter._extract_tool_calls` prefers `get_tool_call_objects`, falls back to single | `letta/adapters/simple_llm_stream_adapter.py:34-50` |
| Stream adapter catches missing IDs | Outer `try/except` catches and sets `tool_call = None` rather than propagating | `letta/adapters/letta_llm_stream_adapter.py:184-187` |
| v3 stream adapter same shape | `LettaLLMStreamAdapter` suppresses single-call extractor failure similarly | `letta/adapters/letta_llm_stream_adapter.py:185-192` |
| Source-code AST argument coercion | `coerce_dict_args_by_annotations` parses string args and re-projects typed containers | `letta/functions/ast_parsers.py:117-168` |
| Function-annotation extraction | `get_function_annotations_from_source` AST extracts `arg: T` annotations only | `letta/functions/ast_parsers.py:90-113` |
| Argument prepare in sandbox executor | `_prepare_function_args` skips TS sources, skips empty source_code, swallows coercion errors | `letta/services/tool_executor/sandbox_tool_executor.py:149-169` |
| Executor dispatch | `ToolExecutorFactory._executor_map` keyed by `ToolType` (`LETTA_CORE → LettaCoreToolExecutor`, `EXTERNAL_MCP → ExternalMCPToolExecutor`, default → `SandboxToolExecutor`) | `letta/services/tool_executor/tool_execution_manager.py:35-43` |
| Tool executor base | `ToolExecutor` abstract execute() returns `ToolExecutionResult` | `letta/services/tool_executor/tool_executor_base.py:16-46` |
| Manager-level error funnel | Top-level `try/except` returns `ToolExecutionResult(status="error", func_return=get_friendly_error_msg, stderr=[traceback])` | `letta/services/tool_executor/tool_execution_manager.py:131-155` |
| Manager-level metrics | Records `tool_execution_time_ms_histogram`, `tool_execution_counter{tool.execution_success}`, step id | `letta/services/tool_executor/tool_execution_manager.py:113-116`, `157-160` |
| Sandbox executor outer try/except | Returns structured error from `_handle_execution_error` | `letta/services/tool_executor/sandbox_tool_executor.py:146-147`, `180-194` |
| v3 unified loop entry | `_handle_ai_response` coordinates no-tool, approval, denial, single/parallel execution | `letta/agents/letta_agent_v3.py:1595-1964` |
| v3 tool-call extraction per call | `args = _safe_load_tool_call_str(tc.function.arguments)`, strips `REQUEST_HEARTBEAT_PARAM`, `INNER_THOUGHTS_KWARG` | `letta/agents/letta_agent_v3.py:1772-1777` |
| v3 rule violation flag | `tool_rule_violated = name not in valid_tool_names and not is_approval_response` | `letta/agents/letta_agent_v3.py:1780` |
| v3 execution spec list | `exec_specs` are dicts: `{id, name, args, violated, error}` | `letta/agents/letta_agent_v3.py:1771-1819` |
| v3 prefilled-args merge (ChildToolRule) | `merge_and_validate_prefilled_args`, on ValueError records inline `error` and continues | `letta/agents/letta_agent_v3.py:1786-1809` |
| v3 single-call executor | `_run_one` short-circuits on `error` and `violated`, else dispatches to `_execute_tool` | `letta/agents/letta_agent_v3.py:1822-1838` |
| v3 parallel/serial fan-out | Splits by `enable_parallel_execution`, `asyncio.gather` for parallel, sequential otherwise | `letta/agents/letta_agent_v3.py:1840-1862` |
| v3 single-tool-call v1 path | `_execute_tool(tool_name, tool_args, agent_state, ...)` → `tool_execution_manager.execute_tool_async` | `letta/agents/letta_agent.py:1922-1969` |
| v2 single-tool-call path | Mirror of v1 in `letta_agent_v2.py` (still references `_execute_tool`) | `letta/agents/letta_agent_v2.py:1288` |
| Approval gate construction | Builds `requested_tool_calls = [...]` & sends `requires_approval` stop_reason | `letta/agents/letta_agent_v3.py:1682-1709` |
| Approval → denial conversion | `create_tool_returns_for_denials(tool_calls, reason, timezone)` produces a `ToolReturn` result | `letta/agents/letta_agent_v3.py:1753-1762` |
| Approval message factory | `create_approval_request_message_from_llm_response` builds `approval`-role `Message` with all tool calls | `letta/server/rest_api/utils.py:304-371` |
| v3 client tool dispatch check | Combines `requires_approval_tool_rules` and `client_tools` in approval gate | `letta/agents/letta_agent_v3.py:1684-1696` |
| Client tool-return truncation | JSON-aware: clamp `parsed.message` else raw-clamp the string | `letta/agents/letta_agent_v3.py:1716-1746` |
| Function-response validation/truncation | `validate_function_response` decides dict vs string, applies `return_char_limit` | `letta/letta/utils.py:898-939` |
| Function-response packaging | `package_function_response` returns `{status: "OK"|"Failed", message, time}` JSON | `letta/letta/system.py:150-168` |
| Tool-return truncation in manager | `FUNCTION_RETURN_VALUE_TRUNCATED(return_str, return_char, return_char_limit)` invoked when `len(return_str) > tool.return_char_limit` | `letta/services/tool_executor/tool_execution_manager.py:126-129`; cf `letta/constants.py:200-203` |
| Per-tool return char limit | `Tool.return_char_limit` (default `FUNCTION_RETURN_CHAR_LIMIT`) | `letta/schemas/tool.py:51-56` |
| Per-tool search exceptions to truncation | `truncate = tool_call_name not in {"conversation_search", "conversation_search_date", "archival_memory_search"}` | `letta/agents/letta_agent_v3.py:1878` |
| Message construction (single tool) | `create_letta_messages_from_llm_response`: assistant-message + `tool`-message + heartbeat-message | `letta/server/rest_api/utils.py:382-515` |
| Message construction (parallel tools) | `create_parallel_tool_messages_from_llm_response` aggregates all tool_calls into one assistant msg + all returns into one tool msg | `letta/server/rest_api/utils.py:518-627` |
| Heartbeat message factory | `create_heartbeat_system_message` produces user-role heartbeat to force loop | `letta/server/rest_api/utils.py:630-655` |
| Continuation decision v3 | `_decide_continuation` rules: tool called ⇒ continue, no-call + required tool ⇒ continue with violation note | `letta/agents/letta_agent_v3.py:1967-2036` |
| Aggregate continuation v3 | `aggregate_continue = any(continue flags) or denials or tool_returns` | `letta/agents/letta_agent_v3.py:1945-1946` |
| Forced continuation on parallel tools | Always continue unless terminal tool rule or max_steps hit | `letta/agents/letta_agent_v3.py:1954-1964` |
| LLM-side parallel-tool config | `disable_parallel_tool_use` for Anthropic family; `parallel_tool_calls` for OpenAI; gated by tool rules | `letta/agents/letta_agent_v3.py:1111-1147` |
| Truncate-to-first when parallel disabled | Enforced at runtime if `parallel_tool_calls=False` configured | `letta/agents/letta_agent_v3.py:1335-1342` |
| OpenAI parallel adapter toggle | `parallel_tool_calls` set True only when `no_tool_rules and active_llm_config.parallel_tool_calls` | `letta/agents/letta_agent_v3.py:1132-1139` |
| Legacy agent loop entry (v1) | `Agent._get_ai_reply` parses function-call, dispatches `execute_tool_and_persist_state`, builds heartbeat or error response | `letta/agent.py:240-275`, `540-660` |
| Legacy tool-call ID coercion | Hashed UUID prefixed `call_` if LLM omits ID | `letta/agents/letta_agent.py:1737` |
| Legacy rule violation synthesis | `_build_rule_violation_result` reused | `letta/agents/helpers.py:501-505` (used by `letta/agents/letta_agent.py:1799`) |
| Legacy JSON parse | `parse_json`/`parse_json_or_wrap_raw` (json → demjson, fallback returns wrapped raw) | `letta/letta/utils.py:854-895` |
| Legacy executor selection (v1) | Big `if/elif` on `target_letta_tool.tool_type` (`LETTA_CORE`, `LETTA_MEMORY_CORE`, `LETTA_SLEEPTIME_CORE`, `EXTERNAL_COMPOSIO`, `EXTERNAL_MCP`, sandbox default) | `letta/agent.py:1608-1687` |
| Legacy sandbox wrapper | `ToolExecutionSandbox.run(agent_state=...)` returns `ToolExecutionResult` | `letta/agent.py:1681-1687` |
| Legacy execution error funnel | `except Exception as e` returns `ToolExecutionResult(status="error", func_return=get_friendly_error_msg, stderr=[traceback.format_exc()])` | `letta/agent.py:1688-1698` |
| Friendly error format | `get_friendly_error_msg` capped to `MAX_ERROR_MESSAGE_CHAR_LIMIT` | `letta/letta/utils.py:1091-1097` |
| Stop-reason taxonomy | `StopReasonType.invalid_tool_call` and `requires_approval` are distinct reasons | `letta/schemas/letta_stop_reason.py:9-22` |
| v3 stop-on-malformed-approval | Sets `stop_reason = StopReasonType.invalid_tool_call` if approval empty | `letta/agents/letta_agent_v3.py:1015-1017` |
| v3 stop-on-prefilled-args-value-error | Treats prefill validation failure as `invalid_tool_call` per tool | `letta/agents/letta_agent_v3.py:1899-1902` |
| Tool-call cache key check | `_get_valid_tools` flows through `tool_rules_solver.get_allowed_tool_names` | `letta/agents/letta_agent_v3.py:2039-2045` |
| Force tool call injection | `force_tool_call = valid_tools[0]["name"] if exactly one + require_tool_call` | `letta/agents/letta_agent_v3.py:1092` |
| Tool-call message role | `Message(role="assistant", tool_calls=[OpenAIToolCall(...)])` | `letta/server/rest_api/utils.py:431-441`, `579-589` |
| Tool-return message role | `Message(role="tool", content=[TextContent(text=packaged)], tool_call_id=..., tool_returns=[ToolReturn(...)])` | `letta/server/rest_api/utils.py:477-498`, `610-622` |
| ToolCallReturn type payload | `ToolReturn(tool_call_id, status, stdout, stderr, func_response)` stored alongside message | `letta/server/rest_api/utils.py:488-495`, `601-608` |
| State persistence after tool | `update_memory_if_changed_async(agent_state.id, tool_execution_result.agent_state.memory)` | `letta/services/tool_executor/sandbox_tool_executor.py:142` |
| Memory integrity check | Asserts `orig_memory_str == new_memory_str` after sandbox exec | `letta/services/tool_executor/sandbox_tool_executor.py:136-138` |
| Tests — schema validator | 278 lines covering optional/required fields, anyOf/oneOf, empty objects, strict mode | `letta/tests/test_schema_validator.py:1-278` |
| Tests — tool execution sandbox | 758 lines covering local/e2b/Modal sandboxes, pip installs, env vars, errors, client injection | `letta/tests/integration_test_tool_execution_sandbox.py:1-758` |
| Tests — tool rule solver | 1133 lines covering all rule types and compliance | `letta/tests/test_tool_rule_solver.py:1-1133` |
| Tests — optimistic JSON parser | `test_optimistic_json_parser.py` covers Anthropic-style lenient JSON | `letta/tests/test_optimistic_json_parser.py:1-280` |
| Tests — required flows | `integration_test_agent_tool_graph` validates ToolCallMessage traversal | `letta/tests/integration_test_agent_tool_graph.py:337-481` |
| Tests — MCP execution | `integration_test_mcp.py` checks ToolCallMessage + ToolReturnMessage presence | `letta/tests/integration_test_mcp.py:180-227` |

## Answers to Dimension Questions

### 1. How are tool calls represented?

Tool calls are represented as `OpenAIToolCall(id: str, type: "function", function: FunctionCall(arguments: str, name: str))` (`letta/schemas/openai/chat_completion_response.py:16-22`). This is the canonical shape used at provider boundaries and when constructing persisted messages (`letta/server/rest_api/utils.py:412-419`, `557-563`). Internally streaming interfaces accumulate partial state into `ToolCallDelta` and finalize into the same `OpenAIToolCall` shape (`letta/interfaces/openai_streaming_interface.py:194-205`, `letta/interfaces/anthropic_parallel_tool_call_streaming_interface.py:137-155`). The legacy single-call path uses `ToolCallMessage(tool_call, tool_calls)` (`letta/schemas/letta_message.py:222-253`) with a deprecation note that `tool_call` is the legacy single field. Multi-tool-call versions use a `tool_calls: list[ToolCall]` field on `Message` (`letta/server/rest_api/utils.py:579-589`). Parallel calls produce one assistant message with all `tool_calls` and one tool message with all `tool_returns` (`letta/server/rest_api/utils.py:565-626`). Each tool call gets a deterministic `tool_call_id` from the provider; the v3 loop assigns a fallback `f"call_{uuid.uuid4().hex[:8]}"` when the provider omits one (`letta/agents/letta_agent_v3.py:1773`), and the single-tool-call v1 path mirrors this (`letta/agents/letta_agent.py:1737`).

### 2. What happens if arguments are invalid?

Three layers catch argument problems, each at different fidelity:

1. **JSON parse failure** — `_safe_load_tool_call_str` (`letta/agents/helpers.py:378-393`) logs the failure and returns `{}`. The legacy v1 path in `letta/agent.py` parses with `parse_json`/`demjson`; on `ValueError` it produces an error message via `_handle_function_error_response` and forces a heartbeat (`letta/agent.py:540-551`). The newest agent (`letta_agent_v3.py`) does **not** surface this as an explicit `invalid_tool_call`; instead it returns `{}` silently and the empty args dict is then passed downstream.
2. **Schema-level argument validation** — `merge_and_validate_prefilled_args` checks that prefilled (tool-rule-injected) args match keys and basic JSON-Schema type (`letta/agents/helpers.py:465-493`). On `ValueError`, the v3 loop records the error in the `exec_spec` and synthesizes a `ToolExecutionResult(status="error")` (`letta/agents/letta_agent_v3.py:1788-1808`) with stop reason `invalid_tool_call` (`letta/agents/letta_agent_v3.py:1899-1902`). JSON-Schema **LLM-args** are not re-validated against the tool schema at runtime; that is left to the provider's strict-mode enforcement (`letta/helpers/tool_execution_helper.py:89-162`).
3. **Type coercion at execution time** — `coerce_dict_args_by_annotations` parses string args to JSON/literal_eval, re-projects typed containers, and raises `ValueError` on failure (`letta/functions/ast_parsers.py:117-168`). Sandbox executor catches `ValueError`/`TypeError` and falls back to original args (`letta/services/tool_executor/sandbox_tool_executor.py:166-169`); if the actual function call still fails it bubbles to `ToolExecutionManager.execute_tool_async` which catches every exception and returns a structured error (`letta/services/tool_executor/tool_execution_manager.py:131-155`).

**No automated "repair" path exists for malformed tool calls.** A JSON-decode failure becomes `{}`, which then either triggers a sandbox execution error from the missing-required arg, or — in Python strict-mode schemas — should be rejected server-side at submission time, but letta does not have that layer for Anthropic/Gemini.

### 3. Are tool results structured?

Yes. `ToolExecutionResult(status: Literal["success","error"], func_return, agent_state, stdout: List[str], stderr: List[str], sandbox_config_fingerprint)` (`letta/schemas/tool_execution_result.py:8-18`) is the canonical structured result. Persisted into the database as `Message.tool_returns=[ToolReturn(tool_call_id, status, stdout, stderr, func_response)]` (`letta/server/rest_api/utils.py:488-495`). For the model-facing message the result is wrapped by `validate_function_response` (handles dict vs. string, applies `return_char_limit`, `letta/letta/utils.py:898-939`) then `package_function_response` produces a `{status, message, time}` JSON envelope (`letta/letta/system.py:150-168`). Truncation is enforced both inside the manager (`letta/services/tool_executor/tool_execution_manager.py:124-128`) and again inside `validate_function_response` (`letta/letta/utils.py:920-937`), with `FUNCTION_RETURN_VALUE_TRUNCATED` marking the tail (`letta/constants.py:200-203`). The same envelope is used for both success and error (`"OK"` vs `"Failed"`), with the original traceback carried in `stderr` field of the structured `ToolReturn`. Model errors (`LLMRateLimitError`, `LLMServerError`, `LLMProviderOverloaded`) are routed via `LLMError` and a fallback handle system (`letta/agents/letta_agent_v3.py:1183-1212`), and `ContextWindowExceededError` triggers summary compaction (`letta/agents/letta_agent_v3.py:1217-1284`).

### 4. Are tool errors visible to the model?

Yes, and at three distinct layers:

- **Server-side tool execution errors** — propagated as `ToolExecutionResult(status="error")` (`letta/schemas/tool_execution_result.py:8-18`). The error message is built by `get_friendly_error_msg` (`letta/letta/utils.py:1091-1097`) prefixed with `ERROR_MESSAGE_PREFIX`; full tracebacks live in `stderr`. Errors are returned to the model in the tool message via `package_function_response(..., was_success=False, ...)` (`letta/letta/system.py:150-168`), so the model sees `{"status":"Failed", "message":"...", "time":"..."}` JSON.
- **Tool-rule violations** — synthesized as `ToolExecutionResult(status="error")` with hint text by `_build_rule_violation_result` (`letta/agents/helpers.py:501-505`); the message includes the rule solver's guesses at the violated rule.
- **Prefilled-args validation errors** — wrapped in `exec_spec["error"]` and emitted as inline tool errors that the model can react to (`letta/agents/letta_agent_v3.py:1795-1808`).
- **Stop reasons vs. continuing** — `_decide_continuation` keeps the loop going after tool errors regardless (`letta/agents/letta_agent_v3.py:1987-2036`); only hard failures (`max_steps`, malformed approvals, no required tool called) end the run.

Errors **never crash the run**. They are funneled into the result and the loop continues; the only `StopReasonType.invalid_tool_call` paths are (a) malformed approval response (`letta/agents/letta_agent_v3.py:1015-1017`) and (b) pre-validation arg error (`letta/agents/letta_agent_v3.py:1899-1902`).

### 5. Can tool calls be approved before execution?

Yes, through two complementary mechanisms:

1. **`RequiresApprovalToolRule` per tool** — declared as `requires_approval_tool_rules: list[RequiresApprovalToolRule]` on `ToolRulesSolver` (`letta/helpers/tool_rule_solver.py:48`, `186-188`). The v3 loop checks `tool_rules_solver.is_requires_approval_tool(tool_call_name)` and routes those tool calls out of the execution path; all other calls (and `client_tools`) become `allowed_tool_calls`. An approval-message is built by `create_approval_request_message_from_llm_response` (`letta/server/rest_api/utils.py:304-371`), the agent loop returns `should_continue=False` and `stop_reason=requires_approval` (`letta/agents/letta_agent_v3.py:1709`). On the next turn (`_maybe_get_approval_messages`, `letta/agents/helpers.py:522-527`), the agent receives the approval response. Denials are converted via `create_tool_returns_for_denials` (`letta/agents/letta_agent_v3.py:1756-1762`, with `tool_call_denials` produced from `approval_response.approvals` filtered through `ApprovalReturn.approve==False` at `letta/agents/letta_agent_v3.py:995-1000`).
2. **Client-side tools** (`letta/agents/letta_agent_v3.py:1684-1696`) — `client_tools` are treated as if they require approval; tool returns go through `tool_returns` (the `ToolReturn` shape), with JSON-aware message-field truncation (`letta/agents/letta_agent_v3.py:1716-1746`). The v1 single-tool path has its own approval gate at `letta/agents/letta_agent.py:1780-1794`.

## Architectural Decisions

- **OpenAI-shaped canonical model** — `OpenAIToolCall(id, function)` is the lingua franca across provider adapters, streaming interfaces, message construction, and approval flow (`letta/schemas/openai/chat_completion_response.py:16-22`, `letta/server/rest_api/utils.py:412-419`). Reduces cross-provider friction but couples letta to OpenAI's wire format.
- **Single structured result type** — `ToolExecutionResult(status, func_return, agent_state, stdout, stderr, sandbox_config_fingerprint)` (`letta/schemas/tool_execution_result.py:8-18`) is the contract every executor must satisfy. Dictators across `LETTA_CORE`, `LETTA_MEMORY_CORE`, `LETTA_SLEEPTIME_CORE`, `LETTA_MULTI_AGENT_CORE`, `LETTA_BUILTIN`, `LETTA_FILES_CORE`, `EXTERNAL_MCP`, and sandbox default (`letta/services/tool_executor/tool_execution_manager.py:35-43`).
- **Catch-all error funnel in `ToolExecutionManager`** — every `except Exception` becomes a structured error result; the run keeps going. Same pattern in v1 (`letta/agent.py:1688-1698`) and `SandboxToolExecutor._handle_execution_error` (`letta/services/tool_executor/sandbox_tool_executor.py:180-194`).
- **Tool rules + scheduler — `ToolRulesSolver`** — pure-function check on each tool call (`is_requires_approval_tool`, `is_continue_tool`, `get_uncalled_required_tools`; `letta/helpers/tool_rule_solver.py:186-208`). Rules are validated in `merge_and_validate_prefilled_args` when pre-filling child args (`letta/agents/helpers.py:465-493`); rule violations synthesize an error result rather than failing the turn.
- **OpenAI strict-mode healing** — `enable_strict_mode` (`letta/helpers/tool_execution_helper.py:89-162`) is paired with a `SchemaHealth` classifier (`letta/functions/schema_validator.py:12-202`). Non-strict but valid schemas are downgraded (no strict applied); invalid schemas log loudly. Optional fields are auto-nulled to keep MCP tools usable.
- **Per-tool execution surface** — sandbox types are selected by metadata + `tool_settings` (Modal preferred, then E2B/Local; `letta/services/tool_executor/sandbox_tool_executor.py:69-130`). The sandbox returns the agent state after run, and letta asserts that memory was unchanged during execution (`letta/services/tool_executor/sandbox_tool_executor.py:135-138`). Memory modifications are propagated via `update_memory_if_changed_async` (`letta/services/tool_executor/sandbox_tool_executor.py:141-142`).
- **Parallel tool calls via OpenAI/Anthropic/Gemini parameters** — gated at request build time (`disable_parallel_tool_use`, `parallel_tool_calls`; `letta/agents/letta_agent_v3.py:1111-1147`) and enforced at runtime by truncating to first tool call if `parallel_tool_calls=False` (`letta/agents/letta_agent_v3.py:1335-1342`).
- **Heartbeat mechanism** — every tool result includes a `request_heartbeat` field. After every tool call the agent emits a heartbeat user-role message so the loop continues deterministically (`letta/server/rest_api/utils.py:500-510`, `letta/agents/letta_agent_v3.py:500-513`).
- **v3 unified single+parallel path** — single-tool flows and parallel-tool flows go through the same `_handle_ai_response` and `create_parallel_tool_messages_from_llm_response` helpers (`letta/agents/letta_agent_v3.py:1595-1964`, `letta/server/rest_api/utils.py:518-627`). v1 retains a separate code path with a `tool_call` and `is_approval` (`letta/agents/letta_agent.py:1720-1867`).

## Notable Patterns

- **Pydantic-typed everywhere** — `OpenAIToolCall`, `ToolCall`, `ToolExecutionResult`, `ToolReturn`, `Message`, `ToolRulesSolver`, `Tool`, `ToolRule` are all Pydantic `BaseModel`s, giving the agent loop a typed contract.
- **Streaming-deferred extraction** — streaming chunks are accumulated into in-memory buffers (`_current_function_arguments_parts`, `_function_name_parts`, `_function_id_parts`; `letta/interfaces/openai_streaming_interface.py:181-205`) and only finalized to a `ToolCall` after the stream completes (`get_tool_call_object`).
- **Provider plug-in via `streaming_interface` registry** — `SimpleLLMStreamAdapter.invoke_llm` picks `SimpleAnthropicStreamingInterface`, `SimpleOpenAIStreamingInterface`, `SimpleOpenAIResponsesStreamingInterface`, or `SimpleGeminiStreamingInterface` based on `model_endpoint_type` and websocket flags (`letta/adapters/simple_llm_stream_adapter.py:80-90`).
- **Sandbox isolation per execution** — deep-copy of agent state with `tools=[]`, `tool_rules=[]` before sandbox runs (`letta/services/tool_executor/sandbox_tool_executor.py:171-178`); `coerce_dict_args_by_annotations` lets the sandbox model Python types in the source code.
- **Trace-driven observability** — `@trace_method` on `execute_tool_async`, `_execute_tool`, `log_provider_trace`; OTel spans add events for `tool_execution_started`/`tool_execution_completed` (`letta/agents/letta_agent.py:1946-1980`); metrics via `MetricRegistry().tool_execution_time_ms_histogram` and `tool_execution_counter` (`letta/services/tool_executor/tool_execution_manager.py:113-160`).
- **Heartbeat-based continuation** — `request_heartbeat` semantic allows the model to signal "I want another turn without producing output," injected by `package_function_response` and replayed back as a user-role message (`letta/server/rest_api/utils.py:502-509`).
- **Multi-version agent loop** — three concurrent implementations (`letta/agent.py`, `letta/agents/letta_agent.py`, `letta/agents/letta_agent_v2.py`, `letta/agents/letta_agent_v3.py`, `letta/agents/letta_agent_batch.py`) — lets letta iterate on loop semantics without breaking API clients.

## Tradeoffs

- **Silent JSON parse failure vs explicit error** — `_safe_load_tool_call_str` logs and returns `{}` (`letta/agents/helpers.py:389-392`). Safer than raising, but masks provider misbehavior and can produce confusing "missing required argument" downstream errors.
- **Single-message parallel tool calls** — collapses all tool returns into one `role="tool"` message (`letta/server/rest_api/utils.py:597-622`). Some providers and clients want per-call messages; the trade-off is mid-context simplicity over wire fidelity.
- **OpenAI-shaped model** — easy to swap providers, but couples letta to OpenAI's choice of `tool_call_id` semantics; Anthropic's `id`-bearing `tool_use` blocks and Gemini's `function_call` parts all funnel through the same canonical schema.
- **Multi-version agent loops** — three implementations (`letta/agent.py`, `letta_agent.py`, `letta_agent_v2.py`, `letta_agent_v3.py`, plus `letta_agent_batch.py`). Provides migration safety but doubles the surface area to maintain and review.
- **Per-tool `return_char_limit`** — prevents blowing out context (`letta/agents/letta_agent_v3.py:1879-1884`). But truncation can produce partial data that the LLM might misinterpret; list of exceptions (`conversation_search` etc., `letta/agents/letta_agent_v3.py:1878`) exists for cases where partial data is more harmful than large output.
- **Catch-all error funnel** — `ToolExecutionManager` swallows `Exception` and `CancelledError` (`letta/services/tool_executor/tool_execution_manager.py:131-142`). Predictable but masks bugs in tool implementations.
- **Strict-mode healing** — `enable_strict_mode` auto-nulls optional fields to satisfy OpenAI strict mode (`letta/helpers/tool_execution_helper.py:144-155`). This keeps MCP tools compatible but may break non-OpenAI providers that interpret `"type": ["string", "null"]` differently.
- **Approval gate per tool** — `RequiresApprovalToolRule` is a `ToolRule` (`letta/schemas/tool_rule.py:348`), so adding a tool to the rules doesn't require new infra. But clients need to keep state of pending approvals per `step_id` (`letta/agents/letta_agent_v3.py:1019-1028`) and the legacy `backfill_tool_call_id` heuristic at `letta/agents/letta_agent_v3.py:980-983` is fragile.
- **Parallel vs. serial split** — `enable_parallel_execution` per tool (`letta/schemas/tool.py:62-64`) drives fan-out (`letta/agents/letta_agent_v3.py:1843-1862`). Lets the user opt in, but requires manual reasoning about stateful tools.

## Failure Modes / Edge Cases

- **JSON decode failure** — `tool_call_args_str` that isn't valid JSON → `{}` silently, can later produce "missing required argument" tool errors. (No unit test exists.)
- **Missing tool_call_id at stream end** — `get_tool_call_object` raises `ValueError` (`letta/interfaces/openai_streaming_interface.py:198-201`); stream adapter swallows and sets `tool_call = None` (`letta/adapters/letta_llm_stream_adapter.py:185-187`). The loop then sees `tool_calls = []` and either ends the turn or — if required tools remain — emits a heartbeat. Behavior is silent.
- **Empty `truncated parallel-call prefix in Anthropic** — Anthropic sometimes emits concatenated JSON; `_safe_load_tool_call_str` strips on `}{` (`letta/agents/helpers.py:381-382`). Correct in practice but unmotivated; no test.
- **Approval response with no approvals** — `_maybe_get_approval_messages` returns the messages, but `approval_response.approvals` may be empty; the v3 loop logs an error and `stop_reason = invalid_tool_call` (`letta/agents/letta_agent_v3.py:1007-1017`). Multi-step hangs possible.
- **`tool_rule_violated` does not stop the loop** — `aggregate_continue` is True while a violated-but-executed tool call was registered (`letta/agents/letta_agent_v3.py:1944-1946`). The model is told via `function_responses` of the violation but the loop continues — by design, but easy to misinterpret as a bug.
- **Parallel `asyncio.gather` does not cancel siblings on failure** — `asyncio.gather(*[_run_one(spec) for _, spec in parallel_items])` (`letta/agents/letta_agent_v3.py:1857`). A throw in one parallel tool leaves the other to complete; only the manager-level exception handler catches it (`letta/services/tool_executor/tool_execution_manager.py:131-155`).
- **`orig_memory_str == new_memory_str` assertion can throw** — `assert` in non-test paths (`letta/services/tool_executor/sandbox_tool_executor.py:138`), turns into an `AssertionError` that the manager catches; non-debug runs with `python -O` skip it.
- **Sandbox cancel race** — `asyncio.CancelledError` caught explicitly in manager (`letta/services/tool_executor/tool_execution_manager.py:131-142`), but the message is logged after function logic completes, can produce stale tracebacks.
- **Strict-mode downgrade hides real schema bugs** — `NON_STRICT_ONLY` (e.g., MCP tool with optional field) → no strict applied (`letta/helpers/tool_execution_helper.py:113-119`); some downstream LLM behaviors (token stream breakage, extra-property validation) will differ.
- **Per-tool `return_char_limit` default of 10_000** (`letta/schemas/tool.py:51-56`) — large tools can produce 1 MB returns (`letta/schemas/tool.py:55`). Truncated to keep the model window safe but loses information.
- **Resolving parallel tool calls with approval response** — `_maybe_get_pending_tool_call_message` heuristic (`letta/agents/helpers.py:530-542`) requires `messages[-3]` to be the assistant tool-call. Reordering or message eviction breaks the heuristic.

## Future Considerations

- **Replace `OpenAIToolCall` with provider-neutral `ToolCallContent`** — `letta/server/rest_api/utils.py:420` has a TODO for this; would simplify adapter mappings.
- **Schema-aware argument validation at runtime** — currently relies on strict-mode provider validation; a letta-side validator (e.g., reusing `validate_complete_json_schema`) would centralize the contract.
- **Unify agent loop versions** — three concurrent implementations complicate maintenance and repair paths.
- **Repair path for malformed JSON tool args** — e.g., re-prompt the LLM with a structured reminder. Today the silent `{}` path is the only fallback.
- **Test coverage for extraction path** — no tests for `_safe_load_tool_call_str`, streaming extractor edge cases (`tool_call_id` missing), `merge_and_validate_prefilled_args` failure modes.
- **Cancel-propagation across `asyncio.gather`** — wrap `gather` with cancellation so a failing parallel tool doesn't leave siblings running.
- **OpenAI `strict: false` fallback schema generator** — `enable_strict_mode` currently skips strict mode entirely if `SchemaHealth.NON_STRICT_ONLY`; dual-track generators (strict + loose) might be valuable when only one path works.

## Questions / Gaps

- **Is the streaming-extractor exception really safe?** — `_extract_tool_calls`'s `try/except` (`letta/adapters/simple_llm_stream_adapter.py:42-43`, `46-50`) catches everything but never logs; failures are invisible. No evidence that downstream callers distinguish "no tool call" from "extractor error."
- **What happens when Anthropic returns a `tool_use` block whose `id` is empty?** — `get_tool_call_object` raises `ValueError`; the v3 chat-completion transformer may produce a `ToolCall` with empty `id`. Search for callers revealed none that normalize empty IDs.
- **Is the `truncate` opt-out list (`conversation_search`, ...) actually safer?** — justification lives only in code comments; no docstring stating why partial search results would be misleading.
- **Where is the boundary between `requires_approval` and "client tool"?** — both go to the approval message path (`letta/agents/letta_agent_v3.py:1687-1696`) but their return paths differ (server tool returns vs. client-side `ToolReturn` list). Need to dig into specific SDK flows.
- **Why three concurrent loop implementations?** — `letta/agent.py` (v1, used by voice/legacy), `letta_agent.py` (v1.5), `letta_agent_v2.py`, `letta_agent_v3.py`, `letta_agent_batch.py`. Not described in any single doc; behavior drift likely.
- **Is there a test for `_safe_load_tool_call_str`?** — No; tests for `validate_complete_json_schema` exist (`letta/tests/test_schema_validator.py:1-278`) but not for the runtime extractor.
- **Where is `ToolCallDenial.reason` cleared/normalized?** — created from `denies.get(t.id).reason` (`letta/agents/letta_agent_v3.py:999-1000`); if upstream `approval_response` has inconsistent shapes, silently produces None reason.
- **What happens when parallel `enable_parallel_execution=True` is set on a sandbox tool?** — fan-out goes through `asyncio.gather`; two sandboxes spawning simultaneously on the same `agent_state` could race the post-exec memory save (`letta/services/tool_executor/sandbox_tool_executor.py:142`). No serial safety check.

---

Generated by `03.03-tool-calling-roundtrip-control` against `letta`.
