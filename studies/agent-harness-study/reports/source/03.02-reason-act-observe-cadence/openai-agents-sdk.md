# Source Analysis: openai-agents-sdk

## 03.02 Reason/Act/Observe Cadence

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (3.10+), `openai-agents` package, Pydantic models, `openai.types.responses` |
| Analyzed | 2026-07-13 |

## Summary

The OpenAI Agents SDK implements Reason/Act/Observe as a typed, item-oriented loop, not as a text-parse-based ReAct. Each turn invokes the Responses API (or an alternate `Model` implementing `stream_response`/`get_response`), parses the returned `Response.output` into first-class run items (`ToolCallItem`, `ReasoningItem`, `MessageOutputItem`, `HandoffCallItem`, `MCPApprovalRequestItem`, `ToolSearchCallItem`, etc.) inside `process_model_response` (`src/agents/run_internal/turn_resolution.py:1551-2001`), executes the eligible runs in parallel through `_execute_tool_plan` (`src/agents/run_internal/tool_planning.py:542-682`), and feeds tool outputs back to the next turn via structured `function_call_output` / `shell_call_output` / `apply_patch_call_output` items produced by `ItemHelpers.tool_call_output_item` (`src/agents/items.py:782-824`).

There is **no textual "Thought:" parsing**; the cadence is whatever the Responses API returns, replayable verbatim through `to_input_item` (`src/agents/items.py:144-153`, `src/agents/run_internal/items.py:66-83`). Observations are mostly **raw** — `str(tool_output)` plus structured `ToolOutputText/Image/FileContent` items — with a guaranteed `call_id` link (`src/agents/items.py:413-419`). Reasoning is **structured, visible, and replayable** through `ReasoningItem` (`src/agents/items.py:451-459`); the SDK ships the optional `OpenAIResponsesCompactionSession` which calls `responses.compact` to summarize history (`src/agents/memory/openai_responses_compaction_session.py:160-234`). Failed tool calls (errors, missing tools, rejected approvals) are converted into observations the next turn will see. The streaming and non-streaming paths mirror each other and emit `RunItemStreamEvent`s onto a queue.

## Rating

**8 / 10** — Reason/Act/Observe is implemented as an explicit, typed cycle with strong guarantees: (a) structured tool-call extraction by item type — no free-text parsing; (b) parallel tool execution gated by typed run structs (`ToolRunFunction`, `ToolRunShellCall`, etc.); (c) tool outputs replayed as `_convert_tool_output` typed payloads (`items.py:802-842`), supporting both `str` and structured `ToolOutputText/Image/FileContent`; (d) `call_id`-correlation preserved through `ToolCallOutputItem.to_input_item` and the `_TOOL_CALL_TO_OUTPUT_TYPE` index (`run_internal/items.py:23-31`); (e) parallel-failure isolation with sibling-task draining (`tool_execution.py:1480-1554`); (f) configurable failed-observation policy — `run_config.tool_not_found_behavior = "return_error_to_model"` (`turn_resolution.py:1958-1966`), `REJECTION_MESSAGE` for human-rejected calls (`run_internal/items.py:349-365`), Pydantic-style JSON decode errors caught via `UserError` (`tool_execution.py:1621-1623`); (g) **`OpenAIResponsesCompactionSession`** invoking `client.responses.compact` with `previous_response_id` or `input` mode and a customizable trigger hook (`memory/openai_responses_compaction_session.py:160-234`); (h) `drop_orphan_function_calls` + `_drop_reasoning_items_preceding_dropped_calls` prevent replayed history from breaking the Responses API (`run_internal/items.py:98-179`); (i) `ReasoningItem` replay path with `ReasoningItemIdPolicy = "preserve" | "omit"` (`run_internal/items.py:58, 77-83, 308-321`). 

The framework is mature, observable, and replayable, but: (1) compaction is opt-in — `OpenAIResponsesCompactionSession` must be wrapped around another session; nothing inside the loop calls it, and the `gpt-4.1`-only default is restrictive (`memory/openai_responses_compaction_session.py:92, 118-119`); (2) reasoning text is hidden by default *from the user-visible stream*, since `ReasoningItem` only carries the raw item and there is no helper to render its `summary` text — the model emits reasoning, the SDK preserves it, but consumers see opaque `ResponseReasoningItem` objects; (3) the loop has explicit abort points (`MaxTurnsExceeded`, `_run_output_guardrails_for_stream` errors, `tool_use_behavior = "stop_on_first_tool"`) but no automatic re-prompting on repeated identical failures; (4) server-managed truncation (`model_settings.py:120-122`) is delegated to OpenAI rather than implemented locally. Compared with LangGraph (graph-as-loop), the OpenAI Agents SDK trades pluggable handlers (`hooks`, `input_filter`, `handoff.input_filter`, `pre_model_hook`-equivalent via `call_model_input_filter`) for a more prescriptive, structured API-driven cadence.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Loop entrypoint — streaming | `start_streaming` runs the turn loop while `_run_output_guardrails_for_stream` and `MaxTurnsExceeded` short-circuit | `src/agents/run_internal/run_loop.py:440-671`, `run_loop.py:881-956` |
| Single streamed turn | `run_single_turn_streamed` consumes `stream_response_with_retry`, dispatches `ResponseOutputItemDoneEvent`s, then `get_single_step_result_from_response` | `src/agents/run_internal/run_loop.py:1242-1705` |
| Model output parsing | `process_model_response` classifies output items by type into `ToolCallItem`, `HandoffCallItem`, `MessageOutputItem`, `ReasoningItem`, `MCPApprovalRequestItem`, `CompactionItem`, etc. | `src/agents/run_internal/turn_resolution.py:1551-2001` |
| Tool-call -> Run struct dispatch | typed `ToolRunFunction`, `ToolRunHandoff`, `ToolRunComputerAction`, `ToolRunCustom`, `ToolRunShellCall`, `ToolRunApplyPatchCall`, `ToolRunLocalShellCall`, `ToolRunMCPApprovalRequest`, `ToolRunFunctionNotFound` | `src/agents/run_internal/turn_resolution.py:1636-1656, 1700-1704, 1770-1830, 1842-1949` |
| Compass — compaction item handled | `output_type == "compaction"` -> `CompactionItem` (preserved as input on resume) | `src/agents/run_internal/turn_resolution.py:1717-1729` |
| Tool planning & parallel plan | `ToolExecutionPlan`, `_build_plan_for_fresh_turn`, `_build_plan_for_resume_turn`, `_execute_tool_plan` | `src/agents/run_internal/tool_planning.py:177-263`, `tool_planning.py:542-682` |
| Parallel tool execution | `asyncio.gather(execute_function_tool_calls, execute_computer_actions, execute_custom_tool_calls, execute_shell_calls, execute_apply_patch_calls, execute_local_shell_calls)` | `src/agents/run_internal/tool_planning.py:580-624` |
| Function-tool batch executor | `_FunctionToolBatchExecutor` with `_create_tool_task`, `_drain_pending_tasks`, `_raise_failure_after_draining_siblings` — sibling-failure isolation | `src/agents/run_internal/tool_execution.py:1358-1554` |
| Per-tool run with approvals, hooks, tracing | `_run_single_tool` opens `function_span`, runs `_maybe_execute_tool_approval` then `_execute_single_tool_body`, attaches tracing on error | `src/agents/run_internal/tool_execution.py:1556-1627` |
| Function tool execution entry | `execute_function_tool_calls` returns `(function_results, tool_input_guardrail_results, tool_output_guardrail_results)` | `src/agents/run_internal/tool_execution.py:1973-1992` |
| Observation -> input item (function) | `ItemHelpers.tool_call_output_item` builds `{"call_id": ..., "output": ..., "type": "function_call_output"}` | `src/agents/items.py:782-824` |
| Structured tool output (image/file/text) | `_maybe_get_output_as_structured_function_output` + `_convert_single_tool_output_pydantic_model` for `ToolOutputText/Image/FileContent` | `src/agents/items.py:802-876` |
| All-purpose Run -> Input item converter | `run_item_to_input_item` calls `to_input_item` on the RunItem | `src/agents/run_internal/items.py:66-83` |
| ToolApprovalItem is *not* a model input | `run_item_to_input_item` returns `None` for `tool_approval_item` types | `src/agents/run_internal/items.py:71-72` |
| ToolApprovalItem.to_input_item raises | explicit guard to surface misuse | `src/agents/items.py:631-636` |
| `to_input_item` base implementation | `RunItemBase.to_input_item` -> `raw_item.model_dump(exclude_unset=True)` for pydantic, return dict otherwise | `src/agents/items.py:144-153` |
| Tool call/output index for resume | `_TOOL_CALL_TO_OUTPUT_TYPE` | `src/agents/run_internal/items.py:23-31` |
| Resume-time orphan pruning | `drop_orphan_function_calls` + `_drop_reasoning_items_preceding_dropped_calls` keeps the Responses API contract | `src/agents/run_internal/items.py:98-179` |
| Turn input assembly | `_prepare_turn_input_items` -> `ItemHelpers.input_to_new_input_list` + `run_items_to_input_items` | `src/agents/run_internal/run_loop.py:280-287` |
| Model-input normalization for API | `prepare_model_input_items` + `normalize_input_items_for_api` strips SDK-only session metadata | `src/agents/run_internal/items.py:191-217, 297-306` |
| Next-turn replay accumulator | `streamed_result._model_input_items = turn_result.pre_step_items + turn_result.new_step_items` | `src/agents/run_internal/run_loop.py:1069-1071` |
| Server-managed conversation / stateful turn | `OpenAIServerConversationTracker.prepare_input`, `mark_input_as_sent`, `track_server_items` (delta-only input on turn N+1) | `src/agents/run_internal/oai_conversation.py` |
| Stream-event dedupe for tool calls / reasoning | `emitted_tool_call_ids`, `emitted_reasoning_item_ids`, `emitted_tool_search_fingerprints` sets | `src/agents/run_internal/run_loop.py:1280-1282` |
| Tool not found -> model-visible error | `ToolRunFunctionNotFound` -> `ToolCallOutputItem`; controlled by `run_config.tool_not_found_behavior == "return_error_to_model"` | `src/agents/run_internal/turn_resolution.py:1958-1997`, `run_loop.py:217-281` |
| Approval rejection -> model-visible error | `function_rejection_item` -> `ToolCallOutputItem` carrying `REJECTION_MESSAGE` | `src/agents/run_internal/items.py:349-365` |
| Tool-runtime error -> UserError wrapping | `raise UserError(f"Error running tool {func_tool.name}: {e}")` | `src/agents/run_internal/tool_execution.py:1610-1623` |
| JSON schema validation error -> ToolCallOutputItem | `parse_function_tool_arguments` raises caught at `_run_single_tool` and re-raised as `UserError` | `src/agents/tool.py`, `src/agents/run_internal/tool_execution.py:1610-1623`, `tests/test_tracing_errors_streamed.py:163-203` |
| Mid-loop interrupt (HITL) | `NextStepInterruption` -> `RunResultStreaming.interruptions` populated; `_event_queue.put_nowait(QueueCompleteSentinel)` | `src/agents/run_internal/run_loop.py:299, 1126-1150`, `src/agents/run_internal/turn_resolution.py:711-723` |
| Approval resume (re-observed in same turn) | `resolve_interrupted_turn` reuses `_build_tool_output_index` to drop approved-or-output runs | `src/agents/run_internal/turn_resolution.py:848-999` |
| ReasoningItem replay | `ReasoningItem` raw_item is `ResponseReasoningItem`; `to_input_item` lets the Responses API keep paired reasoning | `src/agents/items.py:451-459, 144-153` |
| `ReasoningItemIdPolicy` (preserve/omit) | optional `id` stripping from reasoning items before send | `src/agents/run_internal/items.py:58, 308-321` |
| Loop hook — `on_llm_start`, `on_llm_end` | both global `RunHooks` and per-agent `AgentHooks.on_llm_start` / `on_llm_end` | `src/agents/run_internal/run_loop.py:1394-1406, 1629-1636` |
| Tool trace span (input + output capture) | `function_span` + `span_fn.span_data.input/output` set from `tool_call.arguments` / result | `src/agents/run_internal/tool_execution.py:1576-1580, 1589-1590, 1625-1627` |
| Compaction session — server-side `responses.compact` | `OpenAIResponsesCompactionSession.run_compaction` invokes `client.responses.compact(...)` | `src/agents/memory/openai_responses_compaction_session.py:160-234` |
| Compaction candidate selection | `select_compaction_candidate_items` excludes user messages and prior compaction items | `src/agents/memory/openai_responses_compaction_session.py:30-51` |
| Compaction default trigger | `default_should_trigger_compaction` — compact when ≥10 candidates | `src/agents/memory/openai_responses_compaction_session.py:54-56` |
| Compaction mode resolution | `previous_response_id` / `input` / `auto` | `src/agents/memory/openai_responses_compaction_session.py:509-521` |
| Compaction orchestration (post-turn) | `save_result_to_session` calls `run_compaction` after tool-output items are added; defers when local tool outputs present | `src/agents/run_internal/session_persistence.py:354-387` |
| Compaction-aware session protocol | `OpenAIResponsesCompactionAwareSession.run_compaction` + `is_openai_responses_compaction_aware_session` | `src/agents/memory/session.py:108-150` |
| Compaction orphan ID cleanup | `_strip_orphaned_assistant_ids` removes `id` from assistant messages whose paired reasoning was stripped by the API | `src/agents/memory/openai_responses_compaction_session.py:375-401` |
| Server-managed truncation (model_settings) | `truncation: Literal["auto", "disabled"]` -> Responses API field | `src/agents/model_settings.py:120-122` |
| Models communicate as typed events | async iteration over `ResponseStreamEvent`; `ResponseCompletedEvent` carries terminal `Response`, `ResponseOutputItemDoneEvent` carries per-item output | `src/agents/run_internal/run_loop.py:1483-1532` |
| Stream retry policy | `stream_response_with_retry` wraps `get_stream()`, rewinds on transient failures via `rewind_model_request` | `src/agents/run_internal/model_retry.py:610-704`, `src/agents/run_internal/run_loop.py:1452-1457` |
| Loop-cap safety | `MaxTurnsExceeded` -> synthesized `MessageOutputItem`, `streamed_result._max_turns_handled = True` | `src/agents/run_internal/run_loop.py:881-956` |
| Loop abort on cancel | `_cancel_mode == "after_turn"` -> `is_complete = True` after current turn | `src/agents/run_internal/run_loop.py:842-845, 1109-1112, 1161-1164` |
| Refusal -> handler resolution (model-visible error) | `extract_refusal` -> `ModelRefusalError` -> `resolve_run_error_handler_result` -> synthesize `MessageOutputItem` | `src/agents/run_internal/turn_resolution.py:763-808` |
| Compaction item preserved across replay | `CompactionItem` raw_item is replayed via `to_input_item` returning the raw dict | `src/agents/items.py:491-499` |
| Resume turn input prep | `normalize_resumed_input` drops orphan function calls before re-feeding | `src/agents/run_internal/items.py:220-227` |
| Tool-call dedupe across replays | `_dedupe_tool_call_items` keyed by `(call_id, name, args)` | `src/agents/run_internal/tool_planning.py:158-174` |
| Per-tool concurrency cap | `RunConfig.tool_execution.max_function_tool_concurrency` | `src/agents/run_internal/tool_execution.py:1390-1392, 1438-1448` |
| Post-invoke tail timeout | `_FUNCTION_TOOL_POST_INVOKE_WAIT_SECONDS` drains cancelled tasks | `src/agents/run_internal/tool_execution.py:1533-1546` |

## Answers to Dimension Questions

1. **How does the model decide actions?**
   - The model is not asked to choose between ReAct textual "Thought/Action/Observation" — it returns a typed `Response.output` array. `process_model_response` iterates `response.output` (`run_internal/turn_resolution.py:1551-2001`), inspects each item's `type` (`"shell_call"`, `"shell_call_output"`, `"apply_patch_call"`, `"compaction"`, `"tool_search_call"`, `"tool_search_output"`, `isinstance(output, ResponseOutputMessage|ResponseFileSearchToolCall|ResponseFunctionWebSearch|ResponseReasoningItem|ResponseComputerToolCall|McpApprovalRequest|McpListTools|McpCall|ImageGenerationCall|ResponseCodeInterpreterToolCall|LocalShellCall|ResponseCustomToolCall|ResponseFunctionToolCall)`), and routes each into either a `ToolCallItem` / `HandoffCallItem` / `ReasoningItem` / `MessageOutputItem` and a `ToolRun*` dispatch struct. 
   - Tool resolution happens by matching `qualified_output_name` against `handoff_map` first, then against a `function_map` derived from `build_function_tool_lookup_map` (`turn_resolution.py:1924-1949`, `tool_execution.py:1399-1421`). 
   - Streaming path: each `ResponseOutputItemDoneEvent` is consumed inside `run_single_turn_streamed` (`run_loop.py:1483-1625`) and the same classification is applied inline.

2. **How are observations returned?**
   - A function-tool call returns a `FunctionToolResult` whose `run_item` is a `ToolCallOutputItem` with `raw_item = ItemHelpers.tool_call_output_item(tool_call, result)` — a `FunctionCallOutput` dict carrying the `call_id` correlation (`tool_execution.py:1940-1970`, `items.py:390-419, 782-824`). That `ToolCallOutputItem` is appended to `new_step_items` (`tool_planning.py:334-355`, `turn_resolution.py:680-697`) and goes back into the next turn as `run_items_to_input_items(...)` -> `run_item_to_input_item(...)` -> `item.to_input_item()` (`run_internal/items.py:66-95`).
   - Computer, custom, shell, apply_patch, local_shell, MCP-approval, hosted MCP, tool_search items all flow through the same `_build_tool_result_items` aggregator (`tool_planning.py:334-355`) using their own `ItemHelpers` mappings and custom raw-item converters (e.g. `to_input_item` for `ToolCallOutputItem` strips `status`/`shell_output`/`provider_data` for `shell_call_output` — `items.py:421-448`).
   - Streaming adds `RunItemStreamEvent`s for each `tool_called`, `tool_search_called`, `tool_search_output_created`, `reasoning_item_created`, and after `get_single_step_result_from_response` returns, the post-step tool runs add their results to the queue via `stream_step_result_to_queue` (`run_loop.py:1665-1704`).

3. **Is reasoning visible or hidden?**
   - Reasoning is **structured, visible, and replayable**. The model emits `ResponseReasoningItem` outputs, which `process_model_response` wraps in `ReasoningItem` (`run_internal/turn_resolution.py:1758-1759`) and `run_single_turn_streamed` emits as a `RunItemStreamEvent(name="reasoning_item_created")` (`run_loop.py:1616-1625`). 
   - On the next turn, `ReasoningItem.to_input_item()` returns the raw `ResponseReasoningItem` (via `model_dump(exclude_unset=True)` — `items.py:144-153`) so the Responses API keeps reasoning items paired with their tool calls. 
   - To prevent that pairing being violated across resumption, `_drop_reasoning_items_preceding_dropped_calls` drops reasoning items whose following tool call was orphaned (`run_internal/items.py:150-179`).
   - The SDK exposes no helper to render the reasoning's `summary` text on a typed object, but the underlying field is preserved on `raw_item`.
   - Reasoning can optionally be stripped of `id` via `RunConfig.reasoning_item_id_policy = "omit"` (`run_internal/items.py:308-321`).
   - System prompts and per-agent prompts (via `agent.get_system_prompt`, `agent.get_prompt`) are user-controlled; there is no built-in "scratchpad" template.

4. **Are failed observations fed back?**
   - Yes. Three distinct paths surface failures to the next turn:
     1. **Tool execution exceptions** — caught in `_run_single_tool` (`tool_execution.py:1610-1623`), attached as `SpanError` for tracing, then `raise UserError(f"Error running tool {func_tool.name}: {e}") from e`. Because `_FunctionToolBatchExecutor` drains cancelled siblings before propagating (`tool_execution.py:1480-1554`), the failure aborts the turn and the converted `ModelBehaviorError` is rethrown — the model sees a tool trace and the next turn must retry.
     2. **Tool not found** — controllable per `run_config.tool_not_found_behavior`. With `"return_error_to_model"`, the call becomes a `ToolCallOutputItem` via `_build_tool_not_found_output_items` carrying a customizable formatter message (`turn_resolution.py:217-281, 1958-1966`). Default behavior raises `ModelBehaviorError`.
     3. **Approval rejection (HITL)** — `function_rejection_item` builds a `ToolCallOutputItem` with `REJECTION_MESSAGE` (`run_internal/items.py:349-365`), supplemented by a configurable `tool_approval_rejection_message` resolver in `run_config.tool_approval_rejection_message`. Rejection during a resumed turn is recorded by `_record_function_rejection` (`turn_resolution.py:886-916`).
     4. **Pydantic schema / JSON decode errors** — `function_schema.parse_function_tool_arguments` raises; the tool's text input is wrapped in `UserError` at `tool_execution.py:1621-1623`. A test asserts `tool_call_output_item` carries "An error occurred while parsing tool arguments" — `tests/test_tracing_errors_streamed.py:163-203`.
     5. **Model refusal** — extracted via `ItemHelpers.extract_refusal` (`items.py:738-748`), fed to `resolve_run_error_handler_result`, optionally synthesized as a `MessageOutputItem` so the next turn has context (`turn_resolution.py:763-808`).

5. **Can observations be compacted?**
   - Yes, via `OpenAIResponsesCompactionSession` (`memory/openai_responses_compaction_session.py:78-373`), an opt-in session wrapper that calls `client.responses.compact(...)` with mode `"previous_response_id"` (default when available), `"input"` (when no response_id or `store=False`), or `"auto"` (resolved per-call). Trigger default is `len(compaction_candidate_items) >= 10` candidates — overridable via `should_trigger_compaction`. The wrapper replaces the underlying session with the compacted output items and atomically restores prior history on failure (`openai_responses_compaction_session.py:284-306`). The orchestration point is `save_result_to_session` in `run_internal/session_persistence.py:354-387`: after `add_items`, the SDK checks `is_openai_responses_compaction_aware_session(session)` and calls `session.run_compaction(...)`, deferring compaction when local tool outputs are present in the new items.
   - Server-side truncation (auto / disabled) is delegated to OpenAI via `model_settings.truncation` (`model_settings.py:120-122`).
   - There is no local summarizer; compaction is wholly the OpenAI API's responsibility, with retry/rollback handlers in `_restore_underlying_session_items` for replacement failure (`openai_responses_compaction_session.py:284-306`).
   - Pre-prompt compaction of just-in-time runaway inputs uses `drop_orphan_function_calls` (resume path — `run_internal/items.py:98-147`) and `deduplicate_input_items_preferring_latest` (replay dedup — `run_internal/items.py:340-346`).

## Architectural Decisions

- **Loop is structured, not parsed.** Unlike text-ReAct, the OpenAI Agents SDK relies on `openai.types.responses.Response.output`, where each action is a typed item. This eliminates prompt brittleness entirely.
- **Run items are an intermediate, replayable representation.** Every model output is converted into a `RunItem` (`Turn resolution.py:1551-2001`) before execution. These items are then either accumulated into `_model_input_items` (`run_loop.py:1069-1071`) or persisted to session (`session_persistence.py`) — same data both pipelines.
- **Streaming mirrors non-streaming.** `run_single_turn_streamed` and `run_single_turn` ultimately call the same `get_single_step_result_from_response` (`turn_resolution.py:2004-2057`), so streaming consumers see the same item types as a non-streamed run.
- **Retry/rewind helpers are explicit and shared.** Both `stream_response_with_retry` and `get_response_with_retry` use a `rewind` callable; for streaming that includes `server_conversation_tracker.rewind_input` (`run_loop.py:1452-1457`).
- **Compaction is server-managed.** All compaction work flows through `client.responses.compact` rather than being implemented locally. This couples compaction to OpenAI but keeps the SDK itself small.
- **Server-side vs client-side conversation are first-class toggles.** `previous_response_id` / `auto_previous_response_id` / `conversation_id` configure `OpenAIServerConversationTracker` which switches input building from "send everything" to "send only deltas" (`run_internal/oai_conversation.py`), with `call_model_input_filter` required to produce a `list` input that the tracker can mark sent (`run_internal/run_loop.py:1388-1392`).
- **HITL is observed as a typed interruption, not a special tool.** `NextStepInterruption` (`run_internal/run_steps.py:107-170`) halts the loop with `streamed_result.interruptions = ...` and a `QueueCompleteSentinel`, then `resolve_interrupted_turn` continues from `run_state._last_processed_response` (`turn_resolution.py:848-999`, `run_loop.py:730-840`).

## Notable Patterns

- **Per-call_id reconstruction.** `_build_tool_output_index` (`run_internal/tool_planning.py:140-155`) and `_TOOL_CALL_TO_OUTPUT_TYPE` (`run_internal/items.py:23-31`) keep `(raw_type, call_id)` as the universal join key across `function_call` / `shell_call` / `apply_patch_call` / `computer_call` / `local_shell_call` / `custom_tool_call` / `tool_search_call` and their `_output` counterparts.
- **Dedup before send.** `deduplicate_input_items_preferring_latest` keeps the latest occurrence when items share `id`/`call_id`/`approval_request_id` (`run_internal/items.py:268-346`).
- **Fingerprint-based rewind.** `fingerprint_input_item` + `serialize_to_save_counts` lets the session know exactly which items were successfully persisted (`run_internal/items.py:230-265`, `session_persistence.py:325-347`).
- **Tool identity namespaces.** `_tool_identity.py` provides `get_function_tool_lookup_key` / `get_tool_call_namespace` / `get_tool_call_qualified_name` so per-tool namespacing (hosted MCP, deferred top-level) survives into streaming tool-emitted `RunItemStreamEvent`s (`run_loop.py:1573-1604`).
- **Tool approval as first-class item.** `ToolApprovalItem.to_input_item` deliberately raises so misuse is loud (`items.py:631-636`); the item is meant to live in session/state, never in model input.
- **Streamed-event dedupe.** `emitted_tool_call_ids`, `emitted_reasoning_item_ids`, `emitted_tool_search_fingerprints` (`run_loop.py:1280-1282`) plus the post-pass filter at lines 1667-1701 guarantee each item is published exactly once even when streaming and post-processing paths both try to.
- **Hook-side observability.** `RunHooks` and per-agent `AgentHooks` cover `on_agent_start`, `on_llm_start`, `on_llm_end`, `on_tool_start`, `on_tool_end`, `on_handoff`, `on_agent_end` (`run_loop.py:1324-1331, 1394-1406, 1629-1636`, `tool_execution.py:1556-1627`). `RunConfig.trace_include_sensitive_data` gates whether `tool_call.arguments` and tool outputs land in the trace (`tool_execution.py:1589-1590, 1625-1627`).
- **Stream-of-typed-events consumer contract.** `RawResponsesStreamEvent`/`RunItemStreamEvent`/`AgentUpdatedStreamEvent`/`QueueCompleteSentinel` are the four event types on the streaming queue (`stream_events.py`); the loop places each onto `streamed_result._event_queue` at well-defined points.

## Tradeoffs

- **Hard dependency on Responses-API item shape.** `process_model_response` branches on `output_type == "shell_call" | "apply_patch_call" | "compaction" | "tool_search_call"` and on `isinstance(output, ResponseX)` from `openai.types.responses` — none of this parses on openai-protocol fallbacks; providers that don't return Responses-API-shape items must adapt via `extensions/models/any_llm_model.py` or similar. The LiteLLM-only "json_tool_call" synthesis at `turn_resolution.py:1935-1949` shows the cost of holding too tight a coupling.
- **Compaction is OpenAI-bound.** `OpenAIResponsesCompactionSession` validates `is_openai_model_name(model)` and rejects non-`gpt-*` / non-`o*` / non-`ft:gpt-*` models — no local summarizer, no Anthropic compaction path (`openai_responses_compaction_session.py:59-75, 118-119`).
- **Default truncation = no client-side cap.** When users do not enable `truncation` or compaction, long histories hit the API surface; the SDK does not transparently summarize.
- **Reasoning text is opaque to consumers.** `ReasoningItem` keeps `ResponseReasoningItem` (`summary` array, `content`...) in `raw_item` but no `text`/`summary` helper exists on the SDK class for downstream renderers; the test suite generally treats reasoning items as opaque (`items.py:451-459`, `run_loop.py:1616-1625`).
- **Tool runtime errors always raise.** `_FunctionToolBatchExecutor` propagates a single `failure` even when siblings still produce partial results, but only *after* drained siblings finish their `post_invoke` phase (`tool_execution.py:1480-1554`) — useful, but users must design for full-turn abort rather than per-tool continuation.
- **Mid-loop `RECORD` for tool failures goes through `tool_use_tracker` (`run_internal/tool_use_tracker.py`) but the tracker does not retry — the only automatic `run again` happens once per turn (`NextStepRunAgain`).
- **`MaxTurnsExceeded` shortcut.** Synthesizes a `MessageOutputItem` *only if the configured `error_handlers["max_turns"]` produced output* and `include_in_history=True` (`run_loop.py:881-956`). Otherwise the run terminates without a model-visible message — inconsistent with "every step taught the model" semantics.

## Failure Modes / Edge Cases

- **Tool emits invalid JSON.** Test `test_tool_call_error` injects a `ResponseFunctionToolCall` with `bad_json` and asserts the next turn sees `tool_call_output_item` containing "An error occurred while parsing tool arguments" (`tests/test_tracing_errors_streamed.py:163-203`). Production path: `UserError` raised at `tool_execution.py:1610-1623` -> `ModelBehaviorError`/`UserError` propagated by `_FunctionToolBatchExecutor` -> the streaming turn finalizes with the error and the run aborts.
- **Tool not found.** `process_model_response` raises `ModelBehaviorError` by default; with `run_config.tool_not_found_behavior == "return_error_to_model"` the call becomes a normal `ToolCallOutputItem` so the model can recover (`turn_resolution.py:1958-1966`).
- **Tool calls disabled mid-run.** `_FunctionToolBatchExecutor._execute` raises `ModelBehaviorError` for any tool that *was* in `agent.tools` but not in `available_function_tools` at call time (`tool_execution.py:1407-1417`).
- **Sibling-tool failures during parallel execution.** Drain siblings, partition tasks into `cancellable` vs `post_invoke`, merge late failures (`tool_execution.py:1480-1554`); the merged failure is raised after siblings finish to keep error state consistent.
- **User rejects an approval (HITL).** `ToolApprovalItem` flows into `resolve_interrupted_turn`; `_record_function_rejection` (`turn_resolution.py:886-916`) appends a `function_rejection_item` and the call gets a `REJECTION_MESSAGE` observation; `include_in_history` re-feeds the rejection to the next turn.
- **Stream incomplete / response.failed.** `run_single_turn_streamed` raises `response_terminal_failure_error` on `"response.incomplete" | "response.failed"` and `response_error_event_failure_error` on `"error" | "response.error"` (`run_loop.py:1488-1499`).
- **Orphaned reasoning items in resumed history.** `drop_orphan_function_calls` + `_drop_reasoning_items_preceding_dropped_calls` strip them so the Responses API contract `"reasoning was provided without its required following item"` is not violated on replay (`run_internal/items.py:98-179`).
- **Compaction replacement failure.** `_restore_underlying_session_items` / `_restore_underlying_session_items_after_failed_clear` reapply the prior session items and surface a warning (`openai_responses_compaction_session.py:263-306`).
- **Compaction output with orphaned assistant IDs (gpt-5.4 path).** `_strip_orphaned_assistant_ids` cleans assistant `id`s when reasoning items were stripped by the compact API (`openai_responses_compaction_session.py:375-401`).
- **Refusal from the model.** Routed via `ModelRefusalError` and the configurable `error_handlers["refusal"]` resolver (`turn_resolution.py:763-808`, `run_internal/error_handlers.py`).
- **Stream input persisted mid-run.** `streamed_result._stream_input_persisted` prevents replay of the starting user input twice across retries (`run_loop.py:1414-1423`).
- **Truncation / context overflow.** Delegated to the API via `model_settings.truncation: Literal["auto", "disabled"]` (`model_settings.py:120-122`) with no in-SDK cap.

## Future Considerations

- **Bring local compaction for non-OpenAI providers.** The current path requires OpenAI responses; a pluggable `compactor: Callable[[list[RunItem]], Awaitable[list[RunItem]]]` would let providers without `responses.compact` still benefit from long-history trimming.
- **Surface `ReasoningItem.summary` text.** Add `ReasoningItem.summary_text` / `ReasoningItem.to_summary()` so downstream renderers do not have to introspect `raw_item.summary[*].text`.
- **Per-tool auto-retry hook.** The infrastructure for sibling failure draining exists (`tool_execution.py:1480-1554`); exposing `on_tool_failure(call_id, exc) -> RetryDecision` would let users opt into the ReAct "did the tool actually do what I asked?" re-plan pattern without rewriting the batch executor.
- **Coalesce/compact at the Loop level.** When `MaxTurnsExceeded` is reached, the SDK synthesizes a `MessageOutputItem` only when error handlers fire (`run_loop.py:881-956`); a default compaction-after-N-turns path could leave room for partial progress to be conveyed to the caller.
- **Reasoning text collapse in `extract_text` / `extract_last_text` for completeness.** Today those helpers ignore reasoning items; some outputs (text-only reasoning models) would benefit from synthesizing the last reasoning summary as the model-visible text.
- **Compaction customization hooks.** `_normalize_compaction_output_items` and `select_compaction_candidate_items` could be exposed as overridable hooks in `OpenAIResponsesCompactionSession` so callers can add their own compaction-replay safety filters (`openai_responses_compaction_session.py:30-51, 404-449`).
- **Mid-loop interrupt while running tools.** Today `_raise_failure_after_draining_siblings` aborts the turn rather than partial-completing; a `partial=True` mode for streaming consumers could feed partial tool results back rather than dropping the whole turn on one tool failure.

## Questions / Gaps

- **How is reasoning text rendered for streaming consumers?** The SDK emits a generic `RunItemStreamEvent(name="reasoning_item_created")` and stores the raw `ResponseReasoningItem`; downstream UX (e.g. chat UIs) must read `raw_item.summary[*].text` directly. No evidence of a built-in helper in `src/agents/items.py`. (`run_loop.py:1616-1625`, `items.py:451-459`.)
- **Where does `call_model_input_filter` run?** It is invoked inside `turn_preparation.maybe_filter_model_input`, which `run_single_turn_streamed` calls at `run_loop.py:1370-1378`. Not deeply inspected — behavior under tool-call mutations and server-managed conversation could be checked. (`run_internal/turn_preparation.py`.)
- **What happens if `ReasoningItem` and its paired item appear out-of-order in replay?** `_drop_reasoning_items_preceding_dropped_calls` only catches dropped-tool-call cases, not reversed sequences; the Responses API has its own reordering rules. No defensive re-sort observable in `run_internal/items.py`.
- **Is there a per-tool retry budget?** `_FUNCTION_TOOL_POST_INVOKE_WAIT_SECONDS` (referenced at `tool_execution.py:1545`) is a tail-wait timeout, not a retry budget. Searches for "retry" in `tool_execution.py` only find session/sandbox retry, not tool-call retry counters.
- **Does the SDK auto-compact when `previous_response_id` is set without `OpenAIConversationsSession`?** `OpenAIServerConversationTracker.prepare_input` only sends deltas; over many turns the conversation length grows server-side and the SDK hands control back to OpenAI. No SDK-side trigger when previous_response_id is in use. (`run_internal/oai_conversation.py`.)
- **What is the public surface for the post-turn `run_compaction` deferral?** `_defer_compaction` / `_get_deferred_compaction_response_id` / `_clear_deferred_compaction` are private (`_` prefix) but invoked from `save_result_to_session` — no test asserts the deferral ordering survives an interruption + resume. (`memory/openai_responses_compaction_session.py:308-332`, `run_internal/session_persistence.py:354-387`.)
- **How does sandbox memory interact with reasoning?** Sandbox memory rollouts sanitize items via `_sanitize_memory_items` and `run_items_to_input_items`; reasoning items feed through, but the compaction-vs-sandbox interaction is not documented in `src/agents/sandbox/memory/rollouts.py`.

---

Generated by `03.02-reason-act-observe-cadence` against `openai-agents-sdk`.
