# Source Analysis: pydantic-ai

## Completion and Finalization Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+; Pydantic; async/await; `pydantic-graph` state-machine; provider adapters per LLM |
| Analyzed | 2026-07-22 |

## Summary

Pydantic AI treats agent completion as a typed state-machine event. The agent graph has exactly one success terminal — `End(FinalResult[OutputDataT])` produced by `CallToolsNode._handle_final_result` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1359-1379`) — and a small, well-defined set of typed failure exceptions (`AgentRunError` → `UnexpectedModelBehavior`, `ContentFilterError`, `IncompleteToolCall`, `UsageLimitExceeded`, `ModelAPIError`, `ModelHTTPError`, `UserError`/`UndrainedPendingMessagesError`, plus `FallbackExceptionGroup`; see `pydantic_ai_slim/pydantic_ai/exceptions.py:181-309`). A normalized `FinishReason` literal (`'stop' | 'length' | 'content_filter' | 'tool_call' | 'error'`, aligned to OpenTelemetry values, `pydantic_ai_slim/pydantic_ai/messages.py:114-121`) and a per-response `ModelResponseState` literal (`'complete' | 'incomplete' | 'interrupted'`, `messages.py:123-134`) separate "model finished this response" from "agent run is done". Streaming completion is observable in flight: `StreamedResponse.get()` derives `state` from `_cancelled`/`_finished` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:884-903`) and `AgentStream.stream_output` always yields a final fully-validated value after `final_result_event` is set (`pydantic_ai_slim/pydantic_ai/result.py:76-103`). Pending tool calls are explicitly handled through `EndStrategy` (`'early' | 'graceful' | 'exhaustive'`, `_agent_graph.py:73`; defaults to `'early'`, `agent/__init__.py:290`) which synthesizes "Tool not executed - a final result was already processed." returns so message history stays replayable (`_agent_graph.py:1666-1680`). Usage/cost finalization is layered: `RunUsage` is aggregated in `GraphAgentState` (`_agent_graph.py:126`, populated by `ModelRequestNode._append_response` at `_agent_graph.py:1015-1025`), `UsageLimits` provides three guard points (`usage.py:379-419`), and `ModelResponse.cost()` lazily resolves pricing via `genai-prices` with provider-url → provider-id fallback (`messages.py:2231-2253`). Output validation is a three-stage pipeline (`_output.py:132-363`) with capability hooks and `ToolRetryError`-wrapped `ValidationError`/`ModelRetry`. Incomplete runs are surfaced via three orthogonal channels: `ModelResponse.state == 'interrupted'`, `UndrainedPendingMessagesError` on bare `agent.iter()` reaching `End` with a non-empty queue (`run.py:215-232`), and typed exceptions on length/content-filter/usage exhaustion. The `docs/output.md:888-1015` "Cancelling Streams" section is unusually explicit about what the framework does not do (no tool-call filtering, no usage guarantee after cancel), and the project-level `agent_docs/pydantic-ai-slim.md:5-35` declares that `_agent_graph.py` owns finalization. The model is mature, well-typed, deeply tested, and self-documenting; the principal deduction is the lack of an aggregate "agent run interrupted" status on the public `AgentRunResult` surface, leaving consumers to inspect the last `ModelResponse.state` themselves.

## Rating

**8.5 / 10**

Rationale:

- **Comprehensive normalized taxonomy.** `FinishReason` (`messages.py:114-121`) and `ModelResponseState` (`messages.py:123-134`) provide a complete, OTel-aligned vocabulary for what happened at the model and response level. Every provider adapter maps to these literals; every agent-loop decision reads from them.
- **Explicit success terminal with replay-safe history.** `End(FinalResult)` is the single success exit (`_agent_graph.py:1359-1379`); `_handle_final_result` appends a synthetic `ModelRequest` of tool returns so the history is safe to reuse in a future run. `EndStrategy` (`_agent_graph.py:73`, `agent/__init__.py:213-219`) explicitly handles "output tool + function tool in same response" with three documented strategies and dedicated test coverage (`tests/test_streaming.py:1302-2754`, `tests/test_agent.py:4283-5919`).
- **Layered, capability-aware output validation.** `_output.run_output_validate_hooks` / `run_output_process_hooks` / `run_output_with_hooks` (`_output.py:132-363`) compose validate/process/hooks with explicit `wrap_validation_errors` and `allow_partial` semantics; streaming vs final are clearly separated, and the final yield is unconditional so consumers can rely on "last yielded = fully validated" (`result.py:76-103`).
- **Layered usage/cost finalization.** Three guard points (`check_before_request`, `check_tokens`, `check_before_tool_call` at `usage.py:379-419`); optional `count_tokens_before_request` (`usage.py:283-296`); eager `RunUsage` aggregation (`_agent_graph.py:1015-1025`); lazy `cost()` with provider-url→provider-id fallback (`messages.py:2231-2253`); OTel `operation.cost` histogram (`_instrumentation.py:65, 244-280`).
- **Typed failure surface with body and recovery hooks.** `UnexpectedModelBehavior` carries a JSON-formatted `body` (`exceptions.py:208-228`); `IncompleteToolCall` (`exceptions.py:308-309`) is the dedicated error for length-truncated tool calls; `ContentFilterError` (`exceptions.py:232-233`) carries `finish_reason`/`block_reason`/`refusal`. All raised out of the graph and never silently swallowed.
- **Three orthogonal incomplete-run signals.** `ModelResponse.state == 'interrupted'`, `UndrainedPendingMessagesError` (`run.py:215-232`), and `StreamedRunResult.is_complete` (`result.py:426-434`).
- **Extensive tests + honest docs.** Dedicated `tests/test_usage_limits.py`, ~25 `end_strategy` tests, full lifecycle tests for `state` (`tests/test_streaming.py:4741-4780`), and `docs/output.md:888-1015` explicitly documents the gaps (no interrupted tool-call filtering, partial cost on cancel).

Deductions:

- **−0.5** No aggregate "agent run interrupted" status on `AgentRunResult` / `StreamedRunResult` — consumers must inspect the final `ModelResponse.state` to detect that the run never produced a `FinalResult`. An `AgentRunResult.status: Literal['complete', 'incomplete', 'interrupted', 'errored']` would close the gap.
- **−0.5** No framework-side handling of `state='interrupted'` tool calls — docs are explicit (`docs/output.md:956-957, 1010-1011`) that Pydantic AI does not synthesize tool returns or filter incomplete calls on cancel. The only synthesis is the UI event-stream error path (`pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:231-260`), which emits a *failed* placeholder rather than a retry-eligible return.
- **−0.5** `cost()` is best-effort and lazy. There is no eager cost snapshot on `RunUsage`; pricing errors degrade to `CostCalculationFailedWarning` and cancelled-stream cost is explicitly unsupported (`docs/output.md:1013-1015`).

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Final output schema | `FinalResult[OutputDataT]` dataclass with `output`, `tool_name`, `tool_call_id` | `pydantic_ai_slim/pydantic_ai/result.py:1055-1067` |
| Final output schema (stream) | `FinalResultEvent` discriminated by `event_kind='final_result'` | `pydantic_ai_slim/pydantic_ai/messages.py:2786-2797` |
| `EndStrategy` declaration | `Literal['early', 'graceful', 'exhaustive']` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:73` |
| `EndStrategy` defaults to `'early'` | `Agent.__init__(..., end_strategy: EndStrategy = 'early', ...)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:290` |
| `EndStrategy` documented semantics | docstring matrix early/graceful/exhaustive | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:213-219` |
| Success terminal in graph | `_handle_final_result` returns `End(final_result)` and appends synthetic `ModelRequest` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1359-1379` |
| Streaming short-circuit terminal | `SetFinalResult` node ends graph after streaming | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1384-1388` |
| Normalized `FinishReason` | `Literal['stop','length','content_filter','tool_call','error']` OTel-aligned | `pydantic_ai_slim/pydantic_ai/messages.py:114-121` |
| `FinishReason` attached to response | `ModelResponse.finish_reason: FinishReason \| None` | `pydantic_ai_slim/pydantic_ai/messages.py:2143-2144` |
| Per-response lifecycle state | `ModelResponseState = Literal['complete','incomplete','interrupted']` | `pydantic_ai_slim/pydantic_ai/messages.py:123-134` |
| `state` field default | `state: ModelResponseState = 'complete'` | `pydantic_ai_slim/pydantic_ai/messages.py:2159-2160` |
| `state` derivation in stream | `StreamedResponse.get()` chooses `'interrupted'`/`'complete'`/`'incomplete'` from `_cancelled`/`_finished` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:884-903` |
| `FinishReason == 'length'` while empty/thinking-only → exception | `UnexpectedModelBehavior` raised with actionable message | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1102-1114` |
| `FinishReason == 'content_filter'` on empty → typed error | `ContentFilterError` with `finish_reason`/`block_reason`/`refusal` and JSON body | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1116-1130` |
| Partial content under content filter survives | Only triggers when `is_empty` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1116-1117` |
| Truncated tool call detection | `GraphAgentState.check_incomplete_tool_call` raises `IncompleteToolCall` on `finish_reason=='length'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:146-160` |
| Output retry budget | `consume_output_retry` increments and raises `UnexpectedModelBehavior` after `max_output_retries`; calls `check_incomplete_tool_call` first | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179` |
| Tool retry budget | `ToolManager._check_max_retries` raises `UnexpectedModelBehavior('Tool {name!r} exceeded max retries count of {N}')` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:178-181` |
| Output hooks pipeline | `run_output_validate_hooks` / `run_output_process_hooks` / `run_output_with_hooks` | `pydantic_ai_slim/pydantic_ai/_output.py:132-363` |
| Validation error wrapping | `ValidationError`/`ModelRetry` → `ToolRetryError` when `wrap_validation_errors=True` | `pydantic_ai_slim/pydantic_ai/_output.py:132-180` |
| Streaming final yield | `stream_output` always re-yields after `final_result_event` (allow_partial=False) | `pydantic_ai_slim/pydantic_ai/result.py:76-103` |
| `get_output` final validation | `validate_response_output(response)` with default `allow_partial=False` | `pydantic_ai_slim/pydantic_ai/result.py:232-243` |
| `validate_response_output` schema lookup | Looks up output tool call by name from `final_result_event.tool_name` | `pydantic_ai_slim/pydantic_ai/result.py:245-264` |
| `EndStrategy='early'` skips function tools after final result | Synthetic `ToolReturnPart('Tool not executed - a final result was already processed.')` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1666-1680` |
| `EndStrategy` orchestrator | `process_tool_calls` handles all three strategies + 'output tool validated' / 'output tool execution failed' / 'output tool not used' status parts | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1553-1859` |
| `state='interrupted'` definition | `StreamedResponse.cancel()` flips `_cancelled`; `state` computed in `get()` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:820-844, 884-903` |
| No auto-filter on interrupted | Docs explicitly state no filter / no synthesis of interrupted tool calls | `docs/output.md:956-957, 1010-1011` |
| UI event-stream pending tool close | `_PendingToolCall` map closed on error with `'Tool execution was interrupted by an error.'` (`outcome='failed'`) | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:56-106, 231-260` |
| Pending enqueue drain on bare `iter()` | `UndrainedPendingMessagesError` raised when `End` reached with non-empty `pending_messages` | `pydantic_ai_slim/pydantic_ai/run.py:215-232` |
| Pending message priority model | `PendingMessagePriority = Literal['asap','when_idle']` | `pydantic_ai_slim/pydantic_ai/_enqueue.py:30` |
| Pending message drain capability | `PendingMessageDrainCapability` hooks drain queue | `pydantic_ai_slim/pydantic_ai/capabilities/_pending_messages.py` |
| Deferred tool requests | `_get_deferred_tool_requests` returns `DeferredToolRequests` if `output_schema.allows_deferred_tools`, else `UserError` | `pydantic_ai_slim/pydantic_ai/result.py:1087-1104`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1786-1856` |
| `RunUsage` aggregation point | `state.usage.incr(response.usage)` in `_append_response` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1015-1025` |
| Per-response post-token check | `usage_limits.check_tokens(ctx.state.usage)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1023-1024` |
| Request count increment | `_make_request` increments `state.usage.requests` after handler call | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:716, 793, 837` |
| `UsageLimits` definition | `request_limit=50`, `tool_calls_limit=None`, token limits `None`, `count_tokens_before_request=False` | `pydantic_ai_slim/pydantic_ai/usage.py:264-296` |
| `UsageLimits.check_before_request` | Pre-request request / input / total token guard | `pydantic_ai_slim/pydantic_ai/usage.py:379-395` |
| `UsageLimits.check_tokens` | Post-response input / output / total token guard | `pydantic_ai_slim/pydantic_ai/usage.py:397-411` |
| `UsageLimits.check_before_tool_call` | Pre-tool-call parallel-tool-call count guard | `pydantic_ai_slim/pydantic_ai/usage.py:413-420` |
| `UsageLimits.has_token_limits` | Gates per-chunk token checking in stream | `pydantic_ai_slim/pydantic_ai/usage.py:367-377` |
| Cost calculation | `ModelResponse.cost()` via `genai_prices.calc_price` with provider_url→provider_id fallback | `pydantic_ai_slim/pydantic_ai/messages.py:2231-2253` |
| Cost OTel attribute | `_instrumentation.py` emits `operation.cost`; tolerates `CostCalculationFailedWarning` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:65, 244-280` |
| Run/conversation metadata | `fill_run_metadata` stamps `timestamp`, `run_id`, `conversation_id` | `pydantic_ai_slim/pydantic_ai/_utils.py:399-406` (called at `_agent_graph.py:912, 969, 1021`) |
| OTel finish reason attribute | `gen_ai.response.finish_reasons = [response.finish_reason]` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:283-284` |
| Agent-loop error base | `AgentRunError` | `pydantic_ai_slim/pydantic_ai/exceptions.py:181-192` |
| `UnexpectedModelBehavior` carries `body` | JSON-formatted body included in `__str__` and `__reduce__` | `pydantic_ai_slim/pydantic_ai/exceptions.py:203-229` |
| `ContentFilterError` | Subclass of `UnexpectedModelBehavior` | `pydantic_ai_slim/pydantic_ai/exceptions.py:232-233` |
| `IncompleteToolCall` | Subclass of `UnexpectedModelBehavior` for length-truncated tool calls | `pydantic_ai_slim/pydantic_ai/exceptions.py:308-309` |
| `UsageLimitExceeded` | Subclass of `AgentRunError` | `pydantic_ai_slim/pydantic_ai/exceptions.py:195-196` |
| `ModelAPIError` / `ModelHTTPError` | Provider API error hierarchy | `pydantic_ai_slim/pydantic_ai/exceptions.py:236-266` |
| `FallbackExceptionGroup` | Groups fallback failures | `pydantic_ai_slim/pydantic_ai/exceptions.py:269-270` |
| `ToolRetryError` | Internal control-flow signal carrying `RetryPromptPart` | `pydantic_ai_slim/pydantic_ai/exceptions.py:273-305` |
| `UserError` / `UndrainedPendingMessagesError` | Developer errors and pending-queue drain failure | `pydantic_ai_slim/pydantic_ai/exceptions.py:159-178` |
| `StreamedRunResult.is_complete` | Set to `True` by `_marked_completed` or `cancel()` | `pydantic_ai_slim/pydantic_ai/result.py:426-434, 750-772` |
| `StreamedResponse.cancelled` property | Surface for cancellation state | `pydantic_ai_slim/pydantic_ai/models/__init__.py` (canceller + property in same module) |
| Final result event selection | `_get_final_result_event` decides which stream event marks finalization | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1405-1417` |
| `AgentRun.next` End check | Drains pending or raises `UndrainedPendingMessagesError` | `pydantic_ai_slim/pydantic_ai/run.py:215-232` |
| `AgentRunResult.response` | Searches backward through history for most recent `ModelResponse` | `pydantic_ai_slim/pydantic_ai/run.py:586-593` |
| `GraphAgentState` per-run book | `usage`, `output_retries_used`, `run_step`, `run_id`, `conversation_id`, `pending_messages`, `last_max_tokens`, `last_model_request_parameters` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-179` |
| Provider `content_filter` mapping (Anthropic) | Mapping + VCR test | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:3969-4014`; `tests/models/test_anthropic.py:3969-4014` |
| Provider `content_filter` mapping (OpenAI) | Test at `tests/models/test_openai.py:4851-4908` | `tests/models/test_openai.py:4851-4908` |
| Provider `content_filter` mapping (Google) | `tests/models/test_google.py:6306, 6363` | `tests/models/test_google.py:6306, 6363` |
| Provider `content_filter` mapping (Bedrock) | `tests/models/test_bedrock.py:4225-4226` | `tests/models/test_bedrock.py:4225-4226` |
| Provider `finish_reason` map (Bedrock) | `_FINISH_REASON_MAP` | `pydantic_ai_slim/pydantic_ai/models/bedrock.py:223, 699-708, 1405-1407` |
| Provider finish-reason map (Cohere) | `ChatFinishReason` → `FinishReason` | `pydantic_ai_slim/pydantic_ai/models/cohere.py:86, 263-273` |
| Provider finish-reason map (Gemini) | `SAFETY`/`MAX_TOKENS`/`STOP` mapping | `pydantic_ai_slim/pydantic_ai/models/gemini.py:289-299, 878-880` |
| Provider finish-reason map (Google) | Usage re-extraction from merged details | `pydantic_ai_slim/pydantic_ai/models/google.py:1944` |
| Provider finish-reason map (Groq) | `'error'` injection when no choice | `pydantic_ai_slim/pydantic_ai/models/groq.py:123, 240, 395-407, 667-669` |
| Provider finish-reason map (Mistral) | `tests/models/test_mistral.py:411` | `pydantic_ai_slim/pydantic_ai/models/mistral.py:126, 411-424, 717-719` |
| Provider finish-reason map (HuggingFace) | `pydantic_ai_slim/pydantic_ai/models/huggingface.py:117, 293-306, 558-560`; tests at `tests/models/test_huggingface.py:527, 639` |  |
| Provider finish-reason map (xAI) | Proto-enum + string mapping; tests at `tests/models/test_xai.py:953, 1045, 5030` | `pydantic_ai_slim/pydantic_ai/models/xai.py:161-175, 867-887, 956-965` |
| OTel finish-reason attribute in `InstrumentedModel` | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:235-236` |  |
| Stream-end guards | `UnexpectedModelBehavior('Streamed response ended without content or tool calls')` for OpenAI, Anthropic, Google, xAI, Mistral, Gemini | `pydantic_ai_slim/pydantic_ai/models/openai.py:2234`; `anthropic.py:987`; `google.py:1054`; `xai.py:903`; `gemini.py:332`; `mistral.py` `_get_event_iterator` |
| Test: `state` lifecycle | `test_stream_response_state_incomplete_until_finished`, `test_stream_response_yields_incomplete_then_complete`, `test_stream_response_state_incomplete_after_early_break` | `tests/test_streaming.py:4741-4780` |
| Test: cancellation behavior | `test_run_stream_cancel_all_messages_includes_interrupted_response`, `test_run_stream_cancel_guard_suppresses_transport_error`, `test_run_stream_cancel_after_complete`, `test_completed_streamed_response_cancel_noop`, `test_run_event_stream_handler_interrupted_does_not_drain` | `tests/test_streaming.py:4642, 4678, 4717, 4729, 4004` |
| Test: finish-reason behavior | `test_tool_exceeds_token_limit_error`, `test_tool_exceeds_token_limit_but_complete_args`, `test_empty_response_with_finish_reason_length`, `test_thinking_only_response_with_finish_reason_length`, `test_central_content_filter_handling`, `test_central_content_filter_with_partial_content` | `tests/test_agent.py:4084, 4105, 4123, 4144, 10102, 10123` |
| Test: thinking-only retry | `test_thinking_only_response_retry`, `test_thinking_only_response_retry_with_tool_output` | `tests/test_agent.py:8589, 8725` |
| Test: `EndStrategy` early | `test_early_strategy_stops_after_first_final_result` | `tests/test_streaming.py:1302`; `tests/test_agent.py:4283` |
| Test: `EndStrategy` graceful | `test_graceful_strategy_executes_function_tools_but_skips_output_tools` | `tests/test_streaming.py:1947`; `tests/test_agent.py:4917` |
| Test: `EndStrategy` exhaustive | `test_exhaustive_strategy_executes_all_tools`, `test_exhaustive_strategy_invalid_first_valid_second_output` | `tests/test_streaming.py:2292, 2485`; `tests/test_agent.py:5261, 5461` |
| Test: `EndStrategy` retry exhaustion | `test_exhaustive_raises_unexpected_model_behavior`, `test_exhaustive_skips_output_tool_exceeding_retries_on_validation`, `test_exhaustive_strategy_second_output_max_retries_exceeded` | `tests/test_streaming.py:2729`; `tests/test_agent.py:5712, 5739, 5919` |
| Test: usage limits | `test_request_token_limit`, `test_response_token_limit`, `test_total_token_limit`, `test_retry_limit`, `test_streamed_text_limits`, `test_stream_text_enforces_output_token_limit_mid_stream` | `tests/test_usage_limits.py:43, 52, 61, 68, 83, 154` |
| Test: tool-call limits | `test_tool_call_limit`, `test_output_tool_not_counted`, `test_output_tool_allowed_at_limit`, `test_failed_tool_calls_not_counted`, `test_parallel_tool_calls_limit_enforced` | `tests/test_usage_limits.py:497, 553, 683, 769, 864` |
| Test: usage arithmetic | `test_add_usages`, `test_add_usages_with_none_detail_value`, `test_add_request_usages_does_not_mutate_original`, `test_add_run_usages_does_not_mutate_original`, `test_add_usage_repeated_calls_stable` | `tests/test_usage_limits.py:389, 423, 452, 467, 479` |
| Test: exceptions | `IncompleteToolCall` instantiation / serialization | `tests/test_exceptions.py:18, 41, 54, 97, 115` |
| Test: pending tool calls | `args_valid=True/False/None` semantics for `ToolApproved`/`ToolDenied`/deferred | `tests/test_streaming.py:4516-4621` |
| Test: UI error-path pending tool close | `tests/test_ui.py:558, 586, 628`; analogous at `tests/test_vercel_ai.py:7321-7361` |  |
| Docs: top-level run-end | "A run ends when the model responds with one of the output types … A run can also be cancelled if usage limits are exceeded" | `docs/output.md:1-7` |
| Docs: `end_strategy` matrix | `docs/output.md:363-377` |  |
| Docs: cancelling streams | `state='interrupted'`, history inclusion, "no tool-call filtering / no run-resumption behavior" | `docs/output.md:888-1015` |
| Docs: `Usage Limits` | `request_limit` / `response_tokens_limit` / `tool_calls_limit` examples | `docs/agent.md:584-679` |
| Docs: model errors | `UnexpectedModelBehavior` + `capture_run_messages` | `docs/agent.md:1203-1240` |
| Docs: streaming events + dangling tool calls | `docs/agent.md:124-138` |  |
| Docs: finish reason example using `FallbackModel` | "Pydantic AI already handles some finish reasons automatically in the agent loop: `'length'`/`'content_filter'` raise; empty responses are retried" | `docs/models/overview.md:353-391` |
| Architecture ownership | "`_agent_graph.py` owns loop orchestration: prompt assembly, model requests, tool/output processing, retries, usage checks, and finalization" | `agent_docs/pydantic-ai-slim.md:5-35` |

## Answers to Dimension Questions

1. **What counts as done?**
   The agent run is done when `CallToolsNode._handle_final_result` returns `End(FinalResult[OutputDataT])` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1359-1379`). `FinalResult` carries `output`, `tool_name`, and `tool_call_id` (`pydantic_ai_slim/pydantic_ai/result.py:1055-1067`). The "done" decision is reached through five observable sub-conditions: (a) a text response processed by `output_schema.text_processor` (`_agent_graph.py:1322-1357`); (b) an image response processed by `output_schema.image_processor`; (c) a successful output-tool call (`_agent_graph.py:1562-1662`); (d) a `None` final result on empty response when `output_schema.allows_none` (`_agent_graph.py:1132-1150`); or (e) a `DeferredToolRequests` outcome when `output_schema.allows_deferred_tools` (`_agent_graph.py:1786-1856`, `result.py:1087-1104`). On the streaming path, `SetFinalResult` short-circuits to `End` after the stream produces a `final_result_event` (`_agent_graph.py:1384-1388`). Done means *correct output produced* (validated by the output pipeline, `pydantic_ai_slim/pydantic_ai/_output.py:132-363`), not merely "the model stopped".

2. **Can the model falsely declare done?**
   Yes in principle, but multiple guards make this fail loudly. An empty response with `finish_reason == 'length'` raises `UnexpectedModelBehavior` (`_agent_graph.py:1107-1114`); an empty response with `finish_reason == 'content_filter'` raises `ContentFilterError` (`_agent_graph.py:1116-1130`); a streaming stream that ends without content raises `UnexpectedModelBehavior('Streamed response ended without content or tool calls')` (e.g. `pydantic_ai_slim/pydantic_ai/models/openai.py:2234`, `anthropic.py:987`, `google.py:1054`, `xai.py:903`); a thinking-only response with a length finish reason raises `UnexpectedModelBehavior`; a thinking-only response without a length reason triggers a `ToolRetryError` retry up to `max_output_retries` (`_agent_graph.py:1240-1249`); an output tool that fails validation can either retry (via `consume_output_retry`, `_agent_graph.py:162-179`) or, under `EndStrategy='exhaustive'` with no other valid output tool, raise `UnexpectedModelBehavior` (`_agent_graph.py:1586-1603, 1630-1646`); a tool call truncated by `finish_reason == 'length'` raises `IncompleteToolCall` (`_agent_graph.py:146-160`). A model that emits a text response with no actionable content and no tool calls is also retried (`_agent_graph.py:1240-1249`).

3. **Are final outputs validated?**
   Yes, by a layered pipeline. `_handle_text_response` and `_handle_image_response` route through `_output.run_output_with_hooks` / `run_image_process_hooks` (`_agent_graph.py:1322-1357`). The pipeline runs `before_output_validate` → `wrap_output_validate` → `after_output_validate` then `before_output_process` → `wrap_output_process` → `after_output_process` (`_output.py:132-227, 306-363`), with `on_*_error` hooks. `ValidationError` and `ModelRetry` from any hook are converted to `ToolRetryError` when `wrap_validation_errors=True` (`_output.py:132-180`); `ToolRetryError` is itself caught by `_agent_graph` and routed back into `consume_output_retry` (`_agent_graph.py:1145-1149, 1247-1249`). Output validators are declared on the agent and run inside the process hooks (`_output.py:182-227`). On the streaming path, partial outputs are validated with `allow_partial=True`; the final yield always re-validates with `allow_partial=False`, and consumers can rely on "last yielded = fully validated" (`pydantic_ai_slim/pydantic_ai/result.py:76-103, 232-243`).

4. **Is usage/cost finalized?**
   Usage is finalized eagerly in `RunUsage` per response and per request, and is checked at three distinct points. `ModelRequestNode._append_response` increments `state.usage` and runs `usage_limits.check_tokens` post-response (`_agent_graph.py:1015-1025`); per-request counters increment after the handler runs (`_agent_graph.py:716, 793, 837`); `UsageLimits.check_before_request` runs pre-request (`usage.py:379-395`); `UsageLimits.check_before_tool_call` projects `state.usage.tool_calls + len(calls_to_run)` before parallel tool execution (`usage.py:413-420`, called from `_agent_graph.py:1704-1708`); stream chunks are checked when `limits.has_token_limits()` (`usage.py:367-377`, `_get_usage_checking_stream_response` at `result.py:1070-1084`). Cost is finalized lazily on `ModelResponse.cost()` with provider-url → provider-id fallback (`messages.py:2231-2253`); the OTel instrumentation emits `operation.cost` and tolerates `CostCalculationFailedWarning` (`_instrumentation.py:65, 244-280`). **Important caveat documented in `docs/output.md:1013-1015`: cancelled-stream usage is partial and provider-dependent and is not safe for cost-critical accounting.**

5. **Can a run complete with warnings?**
   In a strict sense no — the agent graph either returns `End(FinalResult)` or raises a typed exception. There is no `result.warnings` field on `AgentRunResult` (`pydantic_ai_slim/pydantic_ai/run.py` has no such field; `pydantic_ai_slim/pydantic_ai/result.py` likewise). However, the framework can produce "informational but non-fatal" outcomes in three senses: (a) `EndStrategy` skips under "final result already processed" conditions, emitting `ToolReturnPart('Tool not executed - a final result was already processed.')` (`_agent_graph.py:1666-1680`) — these are durable events in the message history but are not surfaced as a warning; (b) `ModelResponse.state == 'interrupted'` records cancellation but the run still terminates (typically by the user breaking out of iteration or calling `cancel()`); (c) under `EndStrategy='exhaustive'`, output tools that fail validation or execution are reported as `'Output tool not used - output failed validation.'` / `'Output tool not used - output function execution failed.'` status parts without failing the run when another valid output tool exists (`_agent_graph.py:1571-1577, 1590-1597, 1631-1638`). The library does not synthesize a per-run "completed with warnings" status; consumers must derive it from the message history and `ModelResponse.state`.

## Architectural Decisions

- **Single success terminal + typed exception failures.** `End(FinalResult)` is the only success exit (`_agent_graph.py:1359-1379`); failures are a closed set of `AgentRunError` subclasses with `body`/context (`exceptions.py:181-309`). This makes "what is done" a binary answer at the graph level, with status nuance delegated to the message history.
- **Normalize `FinishReason` to OpenTelemetry literals** (`messages.py:114-121`). Provider-specific finish reasons are mapped by per-provider adapters (e.g. `models/bedrock.py:223, 699-708`, `models/cohere.py:263-273`, `models/gemini.py:289-299, 878-880`, `models/anthropic.py:3969-4014`, `models/groq.py:395-407`). The agent loop only ever reasons about the normalized literals.
- **Separate `FinishReason` (model-level) from `ModelResponseState` (response-lifecycle).** `FinishReason` says "why did the model stop"; `ModelResponseState` says "have we finished receiving this response" (`messages.py:123-134`). This lets the loop distinguish "model said done with length limit" from "we cancelled mid-stream".
- **`EndStrategy` is a first-class field, not a hidden behavior** (`_agent_graph.py:73`, `agent/__init__.py:213-219, 290`). Three strategies, each with its own synthetic-skip language, and a wide test matrix (`tests/test_streaming.py:1302-2754`, `tests/test_agent.py:4283-5919`).
- **Output validation is a capability pipeline** (`_output.py:132-363`). Validation, processing, and user-defined output validators are layered around `do_validate` / `do_process` with explicit `wrap_validation_errors` and `allow_partial` semantics.
- **Streaming final yield is unconditional** (`result.py:95-103`). Even if the final content equals the last partial yield, the stream re-validates with `allow_partial=False` and re-emits, so consumers can rely on "last yielded = fully validated".
- **Eager history repair on finalization** (`_agent_graph.py:1365-1377`). `_handle_final_result` appends a synthetic `ModelRequest` containing the tool returns so the run's history is replay-safe in a future run. This is also how `EndStrategy='early'` produces the "Tool not executed - a final result was already processed." returns.
- **Three usage guards with distinct semantics** (`usage.py:379-419`). `check_before_request` (request count + input/total tokens), `check_tokens` (post-response), `check_before_tool_call` (parallel tool-call count projection).
- **Lazy, best-effort cost** (`messages.py:2231-2253`, `_instrumentation.py:65-67`). Pricing is not eagerly stored on `RunUsage`; `cost()` is invoked on demand, with provider-url lookup taking precedence over provider-id.
- **Documented incompleteness on cancel** (`docs/output.md:956-957, 1010-1011`). Pydantic AI explicitly does *not* filter incomplete tool calls or synthesize tool returns on `state='interrupted'`. This is a deliberate scope choice, not an oversight.

## Notable Patterns

- **Capability hook composition for outputs.** `before_*` / `wrap_*` / `after_*` / `on_*_error` form a uniform API across `run_output_validate_hooks` and `run_output_process_hooks` (`_output.py:132-227`).
- **Discriminated union of stream events with `event_kind`.** `ModelResponseStreamEvent = Annotated[PartStartEvent | PartDeltaEvent | PartEndEvent | FinalResultEvent, pydantic.Discriminator('event_kind')]` (`messages.py:2800-2802`) and the `FinalResultEvent` carries the tool name and call id that identify the eventual `FinalResult` (`messages.py:2786-2797`).
- **OTel-aligned enums.** Both `FinishReason` and `ModelResponseState` explicitly defer to OTel semantics (`messages.py:114-134`).
- **Per-node state + node-local error capture.** `CallToolsNode._stream_error` is captured for the result object (`_agent_graph.py:1256-1258`) without losing the underlying exception.
- **Run metadata propagation.** `fill_run_metadata` stamps `run_id` and `conversation_id` on every `ModelRequest` and `ModelResponse` (`_utils.py:399-406`); these are OTel span attributes and the join key for multi-run conversations.
- **UUID7 identifiers for runs and conversations** (`_agent_graph.py:94-118`).
- **Two-tier retry accounting.** `output_retries_used` on `GraphAgentState` (`_agent_graph.py:121-179`) for output-retry budget, and per-tool `max_retries` in `ToolManager._check_max_retries` (`tool_manager.py:178-181`).

## Tradeoffs

- **No aggregate "run interrupted" status.** Consumers must inspect `ModelResponse.state` on the final response to detect cancellation (`result.py:759-772`, `models/__init__.py:884-903`). The framework provides a complete state on the response, but no `AgentRunResult.status: Literal['complete', 'incomplete', 'interrupted', 'errored']` field. This favors composability (a single response carries enough) over ergonomics.
- **No auto-cleanup of interrupted tool calls.** Pydantic AI records the interrupted response verbatim and delegates cleanup to the caller (`docs/output.md:956-957, 1010-1011`). This is a deliberate scope choice — it keeps the framework from making "delete" decisions about a tool call that the model might have wanted to retry — but it pushes work onto consumers.
- **Eager `RunUsage` vs lazy `cost()`.** Token counts are aggregated as responses arrive; cost is computed only when asked or instrumented. This optimizes the hot path (no pricing lookup per chunk) at the cost of weaker guarantees around post-cancel cost accuracy.
- **`EndStrategy='early'` can leave the user with skipped function tools.** Although a synthetic `ToolReturnPart` is recorded so history stays consistent, the user-visible "you have N pending tool calls" is suppressed. A developer debugging a complex agent run can be surprised.
- **Content-filter and length-finish are hard errors, not retries.** `_agent_graph.py:1116-1130` raises `ContentFilterError` on empty content under `content_filter`; `_agent_graph.py:1107-1114` raises `UnexpectedModelBehavior` on `length` with empty content. This is the correct safety posture (no infinite loops) but means a transient filter hit is fatal to the run.
- **`FallbackModel` is the prescribed escape hatch.** Documentation points users to `FallbackModel` with `fallback_on=(ModelAPIError,)` to retry on filter / length / error (`docs/models/overview.md:353-391`). The library does not retry these in-loop.
- **Output validators are inside the process pipeline, not the validate pipeline** (`_output.py:182-227`). This means validators see the *processed* value, which is usually what you want but is not the same as Pydantic validation on the raw structured output.

## Failure Modes / Edge Cases

- **`finish_reason == 'length'` while emitting a tool call** — raises `IncompleteToolCall` (`_agent_graph.py:146-160`); tested in `tests/test_agent.py:4084`.
- **`finish_reason == 'length'` with valid JSON tool args** — proceeds normally (`tests/test_agent.py:4105`).
- **Empty / thinking-only with `length`** — `UnexpectedModelBehavior` with an actionable message pointing at `max_tokens` (`_agent_graph.py:1107-1114`).
- **Empty response with `content_filter`** — `ContentFilterError` carrying the body and the filter reason; partial content survives (`_agent_graph.py:1116-1117`).
- **Stream that ends without any content or tool calls** — `UnexpectedModelBehavior` from each provider's stream iterator (`models/openai.py:2234`, `anthropic.py:987`, `google.py:1054`, `xai.py:903`, `gemini.py:332`, etc.).
- **Tool call that fails validation under `'exhaustive'`** — output tool is reported as failed and a sibling output tool may take over; if none, the run raises `UnexpectedModelBehavior` after `check_incomplete_tool_call` is consulted (`_agent_graph.py:1586-1603, 1630-1646`).
- **Output retries exhausted** — `consume_output_retry` raises `UnexpectedModelBehavior('Exceeded maximum output retries (N)')` after running `check_incomplete_tool_call` to ensure length-truncated calls surface as `IncompleteToolCall` (`_agent_graph.py:162-179`).
- **Per-tool retries exhausted** — `ToolManager._check_max_retries` raises `UnexpectedModelBehavior('Tool {name!r} exceeded max retries count of {N}')` (`tool_manager.py:178-181`).
- **Usage limits exceeded** — `UsageLimitExceeded` raised by `check_before_request`, `check_tokens`, or `check_before_tool_call` (`usage.py:379-419`).
- **Cancellation mid-stream** — `state='interrupted'` on the response; `StreamedRunResult.cancel` records the response into `_all_messages` (`result.py:759-772`); consumer must inspect `state` (`docs/output.md:984-1015`).
- **Bare `async for node in agent_run`** reaching `End` with non-empty `pending_messages` — `UndrainedPendingMessagesError` raised to surface the missed `after_node_run` drain (`run.py:215-232`).
- **Cancelled-stream usage is partial and provider-dependent** (`docs/output.md:1013-1015`); `OpenAIChatModelSettings.openai_continuous_usage_stats` partially mitigates.
- **Provider mapping gaps** — Gemini uses its own literal alias at `models/gemini.py:878` rather than the global `FinishReason`; this is a known integration quirk.
- **Reusing interrupted history in a follow-up run** — docs explicitly warn: "Passing interrupted history directly into another run can therefore fail or lead to retries if the model was in the middle of emitting a tool call when cancellation happened" (`docs/output.md:1010-1011`).

## Future Considerations

- **Add an aggregate `status` to `AgentRunResult` / `StreamedRunResult`.** Something like `Literal['complete', 'incomplete', 'interrupted', 'errored']` would let consumers distinguish "run completed cleanly" from "we cancelled" without inspecting the last `ModelResponse.state`. The data already exists in the framework.
- **Optional `state='interrupted'` cleanup hook.** A user-supplied hook that synthesizes placeholder tool returns for interrupted calls would close the documented gap (`docs/output.md:956-957, 1010-1011`) without committing the framework to a default policy.
- **Eager cost snapshot on `RunUsage`.** A `RunUsage.cost` property that lazily computes once on first access, then caches, would let cancelled-stream consumers retrieve a best-effort number. (Note: cost on cancelled streams is documented as unsafe — `docs/output.md:1013-1015`.)
- **Standardized post-cancel usage contract.** The current behaviour is "best effort and provider-dependent". A `UsageLimits.count_tokens_before_request`-style guard that pre-counts input tokens could provide a guaranteed lower bound on input usage even when the stream is cut.
- **Formalize `SetFinalResult` as the canonical streaming short-circuit.** Currently a one-line node (`_agent_graph.py:1384-1388`); making it a documented extension point would help library authors building on the graph.
- **Promote the UI event-stream pending-tool synthesis to the agent graph.** `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:231-260` already closes `_PendingToolCall`s on error with `'Tool execution was interrupted by an error.'`. Lifting this into `_agent_graph` (perhaps behind a flag) would unify the cancellation semantics across surfaces.
- **Document the Gemini literal-alias quirk** (`models/gemini.py:878`) in a public-facing note, since it diverges from the OTel-aligned `FinishReason` used elsewhere.

## Questions / Gaps

- **No evidence found** for an explicit `AgentRunResult.status` field. The only run-level state is `StreamedRunResult.is_complete` (`pydantic_ai_slim/pydantic_ai/result.py:426-434`).
- **No evidence found** for a framework-side retry on `finish_reason == 'content_filter'` with non-empty content. The framework treats this as a normal final output (`_agent_graph.py:1116`).
- **No evidence found** for an explicit `cost()` cache on `RunUsage` — pricing is recomputed each call to `ModelResponse.cost()`.
- **No evidence found** for any signal-based or shutdown-hook cancellation path. Cancellation is exclusively driven by the consumer calling `StreamedRunResult.cancel()` / `AgentStream.cancel()` or breaking out of iteration.
- **No evidence found** for automatic retry on `finish_reason == 'length'` in non-thinking, non-tool-call responses — the framework raises immediately (`_agent_graph.py:1107-1114`).
- **No evidence found** of a public "raw finish reason" field per provider — only the normalized `FinishReason` (`messages.py:2143-2144`). Provider-specific details live in `provider_details` (`messages.py:1118-1128`).
- **No evidence found** for run-level warning aggregation. Status-style events emitted by `EndStrategy` are recorded as message parts but not exposed via a `result.warnings` accessor.

---

Generated by `03.09-completion-and-finalization-semantics` against `pydantic-ai`.
