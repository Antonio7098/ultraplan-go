# Source Analysis: openhands

## Dimension 06.04: Planner/Executor Contract

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12+ / Pydantic, LiteLLM, FastAPI, pydantic discriminated unions |
| Analyzed | 2026-08-27 |

## Summary

OpenHands does not expose a separate "planner" service that emits a serialized plan. Instead the planner is the LLM itself invoked via `Agent.step` (`_sdk_inspect/sdk/agent/agent.py:476`), and the executor is the same `Agent` instance dispatching validated `ActionEvent`s to `ToolDefinition` executors (`_sdk_inspect/sdk/tool/tool.py:348`). The contract is a stream of `ActionEvent` → `ObservationEvent/AgentErrorEvent/UserRejectObservation` events appended to a persistent `EventLog` (`_sdk_inspect/sdk/conversation/event_store.py`) and round-tripped to the LLM via `View.events_to_messages` (`_sdk_inspect/sdk/event/base.py:91`). The boundary is strongly typed (Pydantic `Schema`/`Action`/`Observation` hierarchies with `extra="forbid"` and `DiscriminatedUnionMixin`) and defended by a 5-stage validation pipeline (JSON parse → alias/normalize → malformed fix → `security_risk`/`summary` extraction → `Pydantic model_validate`). Rejection is first-class via `AgentErrorEvent` (validation/execution failures), `UserRejectObservation` (confirmation policy or `PreToolUse` hook `block_action`), and hook-blocked user messages (`blocked_messages`). Feedback is the next `prepare_llm_messages` turn, enriched by `_ActionBatch` truncation, corrective nudges, stuck detection, and condensation. Cross-agent planning is delegated through the deprecated `DelegateTool` (`_sdk_inspect/tools/delegate/definition.py:32`) and file-based `AgentDefinition` subagents (`_sdk_inspect/sdk/subagent/schema.py:151`), which re-enter the same contract per sub-conversation rather than sharing a plan artifact.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards**

Rationale: typed `Action`/`Observation` schemas with `to_mcp_schema`/`to_openai_tool` generation, deterministic dispatch (`response_dispatch.py:53`), multi-layer validation in `_get_action_event` (`agent.py:767`), parallel execution with `DeclaredResources` locking (`parallel_executor.py:98`), and two independent rejection paths (confirmation policy + hooks). Gaps prevent 9: no standalone typed `Plan` artifact separate from tool calls, `DelegateTool` is deprecated (`delegate/definition.py:1`) and offers no structured inter-agent plan handoff, and execution safety under bad plans relies on eventual observation feedback rather than static plan verification or privilege bounding.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner output schema | `LLMResponse` → `Message` classified into `LLMResponseType` (TOOL_CALLS / CONTENT / REASONING_ONLY / EMPTY) before dispatch | `_sdk_inspect/sdk/agent/response_dispatch.py:44-77` |
| Planner output schema | `ActionEvent` schema: `action: Action | None`, `tool_name`, `tool_call`, `tool_call_id`, `llm_response_id`, `security_risk`, `summary`, `thought`, `critic_result` | `_sdk_inspect/sdk/event/llm_convertible/action.py:21-86` |
| Planner output schema | System prompt + tool JSON generated via `ToolDefinition.to_openai_tool` / `to_responses_tool` injecting `security_risk` + `summary` fields | `_sdk_inspect/sdk/tool/tool.py:437-497` |
| Executor interface | `ToolDefinition[ActionT,ObservationT]` ABC with `action_type`, `observation_type`, `executor: ToolExecutor | None`, `action_from_arguments`, `__call__` validating and coercing outputs | `_sdk_inspect/sdk/tool/tool.py:184-377` |
| Executor interface | `ToolExecutor.__call__(action, conversation) -> Observation` protocol; `ServiceTool` base | `_sdk_inspect/sdk/tool/tool.py:132-165` |
| Executor interface | Parallel executor: `ParallelToolExecutor.execute_batch(action_events, tool_runner, tools)` with per-tool locking | `_sdk_inspect/sdk/agent/parallel_executor.py:54-91` |
| Executor interface | `DeclaredResources(keys, declared)` and `declared_resources(action) -> DeclaredResources` for fine-grained locking; fallback `tool:<name>` mutex when `declared=False` | `_sdk_inspect/sdk/tool/tool.py:99-332`, `_sdk_inspect/sdk/agent/parallel_executor.py:152-162` |
| Validation — JSON parse | `parse_tool_call_arguments` with `sanitize_json_control_chars` + `_normalize_arguments` (typo fix for `security_risk`) | `_sdk_inspect/sdk/agent/utils.py:209-218`, `_sdk_inspect/sdk/agent/utils.py:194-207` |
| Validation — normalization | `normalize_tool_call(tool_name, arguments, available_tools)` applies `TOOL_NAME_ALIASES`, terminal fallback for `grep`/`ls`/`pwd`, and `file_editor` command inference (`str_replace`→`view` etc.) | `_sdk_inspect/sdk/agent/utils.py:386-442` |
| Validation — coercion | `fix_malformed_tool_arguments(arguments, action_type)` decodes JSON-stringified `list`/`dict` fields (GLM 4.6 workaround) | `_sdk_inspect/sdk/agent/utils.py:68-174` |
| Validation — Pydantic | `tool.action_from_arguments(arguments)` → `self.action_type.model_validate(arguments)` inside `_get_action_event` try/except | `_sdk_inspect/sdk/tool/tool.py:334-346`, `_sdk_inspect/sdk/agent/agent.py:840` |
| Validation — extra guards | `_extract_security_risk` enforces `security_risk` requiredness when `LLMSecurityAnalyzer` active; `_extract_summary` always injects `summary` with `_tool_has_summary_param` exemption | `_sdk_inspect/sdk/agent/agent.py:648-731` |
| Validation — typed contract | `Schema` base `model_config: extra="forbid", frozen=True`; `DiscriminatedUnionMixin`; `to_mcp_schema()` strips discriminators; `from_mcp_schema` dynamic model | `_sdk_inspect/sdk/tool/schema.py:172-240` |
| Contract validation failure signal | `except (ValueError, JSONDecodeError, ValidationError): _emit_tool_error(... AgentErrorEvent)` — error includes `Parameters provided: [keys]` and is fed back as tool message | `_sdk_inspect/sdk/agent/agent.py:842-876` |
| Function-call conversion errors | `FunctionCallValidationError` / `FunctionCallConversionError` raised for unknown function, bad enum, missing required params, malformed `<function>` tags | `_sdk_inspect/sdk/llm/exceptions/types.py:36-44`, `_sdk_inspect/sdk/llm/mixins/fn_call_converter.py:520-577` |
| Executor rejection — confirmation | `_requires_user_confirmation(state, action_events)` → `state.execution_status = WAITING_FOR_CONFIRMATION`; loop in `LocalConversation.run` pauses and resumes | `_sdk_inspect/sdk/agent/agent.py:605-647`, `_sdk_inspect/sdk/conversation/impl/local_conversation.py:821-848` |
| Executor rejection — hook block | `HookEventProcessor._handle_pre_tool_use` → `state.block_action(event.id, reason)`; `_ActionBatch.prepare` partitions `blocked_reasons` and emits `UserRejectObservation(rejection_source="hook")` | `_sdk_inspect/sdk/hooks/conversation_hooks.py:123-173`, `_sdk_inspect/sdk/agent/agent.py:157-204` |
| Executor rejection — unmatched actions | `ConversationState.get_unmatched_actions(events)` + `reject_pending_actions(reason)` emits `UserRejectObservation` per pending `ActionEvent` | `_sdk_inspect/sdk/conversation/state.py:473-513`, `_sdk_inspect/sdk/conversation/impl/local_conversation.py:896-925` |
| Feedback loop — observation | `ObservationEvent.to_llm_message()` (`role="tool"`) and `AgentErrorEvent.to_llm_message()` fed via `View.from_events` → `LLMConvertibleEvent.events_to_messages` → `prepare_llm_messages` | `_sdk_inspect/sdk/event/llm_convertible/observation.py:51-58,141-149`, `_sdk_inspect/sdk/event/base.py:91-126`, `_sdk_inspect/sdk/agent/utils.py:463-513` |
| Feedback loop — corrective nudge | `_handle_no_content_response` + `_send_corrective_nudge` injects `MessageEvent(role="user", "Your last response did not include a function call…")` | `_sdk_inspect/sdk/agent/response_dispatch.py:239-308` |
| Feedback loop — validation error | `except FunctionCallValidationError: on_event(MessageEvent(source="user", text=str(e)))` — model can self-correct on next turn | `_sdk_inspect/sdk/agent/agent.py:532-542` |
| Feedback loop — execution ValueError | `_execute_action_event` catches `ValueError` → `AgentErrorEvent(f"Error executing tool…")` ; `ParallelToolExecutor._run_safe` does same for any exception | `_sdk_inspect/sdk/agent/agent.py:943-953`, `_sdk_inspect/sdk/agent/parallel_executor.py:98-140` |
| Feedback loop — stuck/condensation | `StuckDetector.is_stuck()` → `execution_status=STUCK`; `LLMContextWindowExceedError` / `LLMMalformedConversationHistoryError` trigger `CondensationRequest` | `_sdk_inspect/sdk/conversation/impl/local_conversation.py:812-820`, `_sdk_inspect/sdk/agent/agent.py:543-580` |
| Contract typing | `Observation` base `content: list[TextContent|ImageContent]`, `is_error`, `ERROR_MESSAGE_HEADER`, `to_llm_content`, `visualize` with discriminated union via `kind` | `_sdk_inspect/sdk/tool/schema.py:268-351` |
| Cross-agent planning | `DelegateAction(command="spawn"|"delegate", ids, agent_types, tasks)` → `DelegateExecutor` spawns `LocalConversation` per sub-agent with parent LLM copy, inherits `confirmation_policy`, joins threads, syncs metrics `usage_to_metrics[f"delegate:{id}"]` | `_sdk_inspect/tools/delegate/definition.py:32-68`, `_sdk_inspect/tools/delegate/impl.py:33-384` |
| Cross-agent planning | `AgentDefinition` Markdown frontmatter: `name, description, tools, skills, mcp_servers, permission_mode, max_iteration_per_run, hooks` → `register_file_agents` / `register_plugin_agents` | `_sdk_inspect/sdk/subagent/schema.py:151-305`, `_sdk_inspect/sdk/conversation/impl/local_conversation.py:550-567` |
| Batch contract | `_ActionBatch._truncate_at_finish` discards tool calls after `FinishTool`; `finalize` handles iterative refinement vs `mark_finished(FINISHED)` unless finish was blocked | `_sdk_inspect/sdk/agent/agent.py:112-238` |

## Answers to Dimension Questions

**Q1. What does the planner output?**
The planner is the LLM invoked through LiteLLM. Its raw output is a `Message` (`_sdk_inspect/sdk/llm/message.py`) containing `content: list[TextContent]` plus optional `tool_calls: list[MessageToolCall]`, `reasoning_content`, `thinking_blocks`, and `responses_reasoning_item` (`_sdk_inspect/sdk/agent/response_dispatch.py:53-77`). `Agent.step` classifies this via `classify_response` (`_sdk_inspect/sdk/agent/response_dispatch.py:53`) and either:
- For `TOOL_CALLS`: fans out each `MessageToolCall` into a validated `ActionEvent` (`_sdk_inspect/sdk/agent/agent.py:767-903`, `_sdk_inspect/sdk/event/llm_convertible/action.py:21`) — one event per tool call sharing `llm_response_id` — with parsed `Action` payload, `security_risk`, `summary`, and optional `critic_result`; or
- For `CONTENT`/`REASONING_ONLY`/`EMPTY`: emits a `MessageEvent(source="agent", llm_message=Message)` and either finishes the run or injects a corrective nudge (`_sdk_inspect/sdk/agent/response_dispatch.py:225-308`).
There is no separate typed `Plan` object with steps, dependencies, or canary checks; the "plan" is the batch of `ActionEvent`s from a single LLM turn (grouped by `llm_response_id` in `events_to_messages` at `_sdk_inspect/sdk/event/base.py:91`). Tool schemas advertised to the planner are generated dynamically from `ToolDefinition.action_type` with injected `security_risk` and `summary` fields (`_sdk_inspect/sdk/tool/tool.py:413-435`).

**Q2. Can the executor trust it?**
No — the executor treats every tool call as untrusted and validates at four points before execution:
1. JSON parsing with control-char sanitization (`_sdk_inspect/sdk/agent/utils.py:209`) and typo normalization (`utils.py:194`);
2. Alias/normalization (`utils.py:386`) and malformed-argument coercion (`utils.py:68`);
3. `security_risk`/`summary` extraction with strictness conditioned on the active `SecurityAnalyzer` (`_sdk_inspect/sdk/agent/agent.py:648-731`);
4. Final `Pydantic model_validate` via `tool.action_from_arguments` (`_sdk_inspect/sdk/tool/tool.py:334`) inside `_get_action_event`'s `except (ValueError, JSONDecodeError, ValidationError)` block that emits an `AgentErrorEvent` instead of executing (`_sdk_inspect/sdk/agent/agent.py:842-876`).
Even after validation, execution is not trusted to be safe: `SecurityAnalyzer.analyze_pending_actions` + `ConfirmationPolicy.should_confirm` can force `WAITING_FOR_CONFIRMATION` (`agent.py:630-642`), and `HookManager.run_pre_tool_use` can asynchronously block any `ActionEvent` (`_sdk_inspect/sdk/hooks/manager.py:49-78`). At runtime, `ToolExecutor.__call__` may still raise `ValueError` which is caught and converted to `AgentErrorEvent` (`agent.py:943-953`, `parallel_executor.py:120-128`), so the LLM turn always receives a textual error rather than a crash.

**Q3. Can the executor modify it?**
The executor cannot rewrite a validated `Action` payload before execution, but it has three sanctioned mutation paths:
- **Reorder/truncate the batch**: `_ActionBatch._truncate_at_finish` discards everything after `FinishTool` (`_sdk_inspect/sdk/agent/agent.py:128`), and `ParallelToolExecutor` may serialize or parallelize within resource-lock constraints (`parallel_executor.py:79-91`).
- **Reject instead of execute**: blocked action IDs are partitioned out in `_ActionBatch.prepare` (`agent.py:168`) and emitted as `UserRejectObservation(action_id, rejection_source="hook"|"user")` rather than calling the `ToolExecutor` (`agent.py:187-204`, `state.py:447-471`).
- **Augment via hooks on observations**: `PostToolUse` hooks run after `ObservationEvent` emission and can inject `HookExecutionEvent`s or `additional_context` that influences the next planner turn, but they do not mutate the already-executed action (`_sdk_inspect/sdk/hooks/conversation_hooks.py:175-238`).
Cross-agent modification is isolated: a `DelegateTool` delegation creates independent `LocalConversation`s per sub-agent (`delegate/impl.py:212`) whose actions are invisible to the parent until consolidated into a single `DelegateObservation` text blob (`delegate/impl.py:368-374`), so the parent executor never edits sub-agent tool calls in place.

**Q4. How is failure fed back?**
All failures surface as events appended to the `EventLog`, then re-serialized for the next LLM call via `prepare_llm_messages` → `View.from_events` → `LLMConvertibleEvent.events_to_messages` (`_sdk_inspect/sdk/agent/utils.py:490`, `_sdk_inspect/sdk/event/base.py:91`):
- **Validation failure**: `_emit_tool_error` creates a synthetic `ActionEvent(action=None)` + `AgentErrorEvent(error, tool_name, tool_call_id)` pair (`_sdk_inspect/sdk/agent/agent.py:733-765`); for the non-function-calling path, `FunctionCallValidationError` is emitted as a user `MessageEvent` (`agent.py:532-541`).
- **Execution failure**: `ValueError` or any exception inside `ToolExecutor` → `AgentErrorEvent` (`agent.py:943`, `parallel_executor.py:123-139`) with `observation.to_llm_content` prepending `ERROR_MESSAGE_HEADER` (`tool/schema.py:321`).
- **Policy/hook rejection**: `UserRejectObservation` with `rejection_source` and `rejection_reason` (`event/llm_convertible/observation.py:71-120`); user-message blocks set `blocked_messages` and cause `step` to set `FINISHED` without LLM call (`agent.py:496-501`, `hooks/conversation_hooks.py:275-283`).
- **Non-actionable LLM output**: `_send_corrective_nudge` injects a user message (`response_dispatch.py:283`) and `StuckDetector` watches for repeated `action_observation`, `action_error`, `monologue`, `alternating_pattern` counters (`conversation/stuck_detector.py`).
- **Context failures**: `LLMContextWindowExceedError` / `LLMMalformedConversationHistoryError` emit `CondensationRequest` for the condenser to summarize history (`agent.py:543-580`).
Because `events_to_messages` reconstructs assistant messages batch-wise by `llm_response_id` (`event/base.py:102-119`), the LLM always sees tool results in causal order with matching `tool_call_id`s, allowing self-correction via retry.

**Q5. Is the contract typed?**
Yes, end-to-end typed with Pydantic, but unevenly enforced:
- `Action` and `Observation` both inherit `Schema(DiscriminatedUnionMixin)` with `frozen=True, extra="forbid"` (`_sdk_inspect/sdk/tool/schema.py:173`), so unknown fields are rejected at `model_validate` time.
- `ToolDefinition` is generic `ToolDefinition[ActionT, ObservationT]` declaring `action_type` and `observation_type` (`tool/tool.py:184`); deserialization/serialization uses `kind_of` / `resolve_kind` discriminators (`tool/tool.py:273-288`).
- Tool JSON for the LLM is derived from the same model via `action_type.to_mcp_schema()` → `to_openai_tool` / `to_responses_tool` (`tool/tool.py:379-497`), guaranteeing planner and executor share a single schema source.
- Wire events are typed: `ActionEvent.action`, `ObservationEvent.observation`, `AgentErrorEvent.error` all carry Pydantic-validated fields and `to_llm_message()` converters (`event/llm_convertible/action.py:37`, `event/llm_convertible/observation.py:31`).
- Gaps: `DelegateAction.tasks: dict[str,str]` and `DelegateObservation.content` are untyped free-text handoffs (`tools/delegate/definition.py:54-67`); subagent outputs are concatenated strings rather than structured plans, and the `summary`/`security_risk` injection is dynamically subclassed (`_create_action_type_with_summary` / `create_action_type_with_risk` at `tool/tool.py:553-634`) with a global lock, which is tested but brittle to cache loss (`#2642` comment). No JSON Schema is published separately from code.

## Architectural Decisions

- **Unified planner-executor in Agent.step vs. external planner service.** Decision to keep LLM call and tool execution inside the same `Agent` loop (`_sdk_inspect/sdk/agent/agent.py:476-603`, `conversation/impl/local_conversation.py:745-833`) simplifies state management (single `ConversationState` + `EventLog`) at the cost of no auditable `Plan` artifact. Consequence: plan quality is only observable through the event stream, not a versioned plan store.
- **Pydantic as the single source of truth for tool contracts.** All tool schemas derive from `Action`/`Observation` models (`tool/schema.py:172`), and `ToolDefinition` is generic over them. Enables `resolve_tool` registry (`tool/registry.py:201`) and `to_mcp_schema` compatibility layer (`tool/schema.py:178-198`) but forces dynamic subclassing for `WithRisk`/`WithSummary` variants.
- **Two independent gating layers: SecurityAnalyzer + HookManager.** `PreToolUse` hooks block by writing `blocked_actions` into `ConversationState` (`hooks/conversation_hooks.py:165`, `state.py:447`), which `_ActionBatch` respects without calling the executor; `ConfirmationPolicy` forces `WAITING_FOR_CONFIRMATION` (`agent.py:605`). Defense-in-depth but introduces two places where an action can be denied with slightly different event shapes (`UserRejectObservation` vs `AgentErrorEvent`).
- **Batch-oriented execution with resource locking.** `_ActionBatch` (`agent.py:112`) + `ParallelToolExecutor` + `DeclaredResources` (`tool/tool.py:99`) allows `tool_concurrency_limit > 1` while serializing on declared keys (`parallel_executor.py:152-162`). Default is `tool_concurrency_limit=1` (`agent/base.py:338`) so parallelism is opt-in; tools that do not declare resources fall back to a `tool:<name>` mutex, preventing races but limiting concurrency.
- **Delegate via nested LocalConversations, not a plan DAG.** `DelegateExecutor._spawn_agents` creates child `LocalConversation`s sharing workspace and copying the parent LLM (`tools/delegate/impl.py:153-224`); `_delegate_tasks` joins threads and folds results into a text observation (`impl.py:254-384`). Marked deprecated in favor of `TaskToolSet` (`delegate/definition.py:1`). No structured task graph or typed inter-agent plan; coordination is prompt text.

## Notable Patterns

- **Normalize-then-validate pipeline.** `parse_tool_call_arguments` → `normalize_tool_call` (aliases + `grep`→`terminal` fallback + `file_editor` inference) → `fix_malformed_tool_arguments` → `_extract_security_risk`/`_extract_summary` → `model_validate` (`agent.py:792-840`, `agent/utils.py:68-442`). Handles model sloppiness (GLM, Kimi, Nemotron tag fixes in `fn_call_converter.py:580-627`) before strict validation.
- **LLM self-correction via error-as-tool-message.** Every validation or execution error becomes a `tool`-role `Message` with either `AgentErrorEvent.error` or prefixed `ERROR_MESSAGE_HEADER` (`tool/schema.py:324`, `event/llm_convertible/observation.py:144-149`), so the next LLM turn can retry with corrected arguments without human intervention.
- **Event-sourced conversation with view materialization.** `EventLog` is file-backed (`event_store.py`), and `View.from_events` + `CondenserBase.condense` produce the LLM message window (`agent/utils.py:490-508`). `events_to_messages` batches `ActionEvent`s by `llm_response_id` to reconstruct multi-tool-call assistant turns (`event/base.py:91`).
- **Hook injection as composable middleware.** `HookEventProcessor.on_event` intercepts `ActionEvent`/`ObservationEvent`/`MessageEvent` and wraps `original_callback` (`hooks/conversation_hooks.py:102-122`); created via `create_hook_callback` and chained in `LocalConversation._ensure_plugins_loaded` (`conversation/impl/local_conversation.py:538-544`). Enables `PreToolUse` deny, `PostToolUse` observation augmentation, and `UserPromptSubmit` context injection without forking the agent.
- **Dynamic schema augmentation with caching.** `create_action_type_with_risk` / `_create_action_type_with_summary` synthesize `*WithRisk` / `*WithSummary` subclasses and memoize under `_action_type_lock` (`tool/tool.py:553-634`), recovering from lost cache entries by scanning `__subclasses__()` (`#2642` comments).

## Tradeoffs

- **Strict Pydantic typing vs. LLM brittleness.** `extra="forbid"` surfaces missing/extra fields immediately as `AgentErrorEvent`s, improving debuggability, but minor model variations (e.g., stringified arrays, missing `security_risk`) would fail without the `fix_malformed_tool_arguments` and conditional `security_risk` logic (`agent/utils.py:108-174`, `agent.py:648-684`). Tradeoff accepted via extensive normalization.
- **Parallel execution with opt-in locking.** `DeclaredResources` enables safe parallelism, but most tools return `declared=False` (default), falling back to coarse `tool:<name>` serialization (`tool/tool.py:324-332`). Maximizes safety over throughput; users must implement `declared_resources` to benefit.
- **Implicit plan vs. auditable plan artifact.** No durable `Plan` object; the event stream *is* the plan. Simplifies persistence (single `EventLog`) but makes plan diffing, rollback, or human approval of a multi-step plan before execution impossible. `LocalConversation.fork` (`conversation/impl/local_conversation.py:314-415`) can snapshot history, but not a future plan.
- **Deprecated delegate vs. no replacement contract in inspected tree.** `DelegateTool` is deprecated since 1.16.0 (`tools/delegate/definition.py:1`) with a promise of `TaskToolSet`; current tree has no `task` tool, so cross-agent delegation has no forward-typed contract to audit. Existing `DelegateExecutor` threads inherit the parent workspace, risking filesystem races unless subagents declare resources.
- **Hook blocking via state mutation vs. return-value.** `HookEventProcessor` writes `blocked_actions` into `ConversationState` then relies on `_ActionBatch` to emit rejections (`hooks/conversation_hooks.py:165`, `agent.py:168`). Decouples hook execution from agent step (hooks run synchronously via callback composition) but requires producer and consumer to agree on the `state.blocked_*` dictionaries; a missed `set_conversation_state` call would silently fail to block (`hooks/conversation_hooks.py:168-173`).

## Failure Modes / Edge Cases

- **Bad plan with hallucinated tool name.** `tools_map.get(tool_name) is None` → `AgentErrorEvent("Tool 'X' not found. Available: […]")` (`agent.py:803-819`). Execution does not crash; LLM receives list of valid tools and can retry. Repeated hallucinations trigger `StuckDetector` (`conversation/stuck_detector.py`) → `STUCK` or `MaxIterationsReached` (`conversation/impl/local_conversation.py:850-871`).
- **Malformed JSON / control chars inside arguments.** `json.loads` failure falls back to `sanitize_json_control_chars` then `fix_malformed_tool_arguments` (`agent/utils.py:209`, `agent/utils.py:147-173`); if still invalid, caught as `ValidationError` → `AgentErrorEvent` with `Parameters provided: [keys]` (`agent.py:842-864`). Truncation of trailing garbage (e.g., XML tags) is best-effort (`agent/utils.py:157-173`).
- **Missing required param / wrong enum / wrong type.** `_extract_and_validate_params` in non-function-calling path raises `FunctionCallValidationError` (`llm/mixins/fn_call_converter.py:520-577`); native function-calling path raises Pydantic `ValidationError` → `AgentErrorEvent`. Either way the error is fed as a `tool`/`user` message, not an exception pop to the caller.
- **Tool executor raises.** `ValueError` or any `Exception` inside `ParallelToolExecutor._run_safe` is caught and wrapped as `AgentErrorEvent` (`parallel_executor.py:120-140`); `_execute_action_event` does the same for `ValueError` (`agent.py:943-953`). No partial observation is emitted.
- **Hook blocks a `FinishAction` mid-batch.** `_truncate_at_finish` already isolated finish at `events[finish_idx]` (`agent.py:128`), but `finalize` checks `has_finish and last_id in blocked_reasons` and does not mark `FINISHED` (`agent.py:222`), so the conversation continues and the LLM can retry finishing.
- **Concurrency race on shared workspace.** Tools that mutate files without declaring `DeclaredResources(keys=("file:/path",), declared=True)` serialize only on `tool:<name>`, not on the file key, so two parallel `file_editor` calls on different files still serialize, while two `terminal` calls on overlapping paths may race. Documented warning in `parallel_executor.py:9-17`.
- **Context window exceeded / malformed history.** `LLMContextWindowExceedError` / `LLMMalformedConversationHistoryError` trigger `CondensationRequest` if condenser handles it, otherwise re-raised with guidance (`agent.py:543-580`, `agent.py:978-1053`). Without a condenser, the run errors out with `ConversationErrorEvent`.
- **Subagent spawn exceeds `max_children` or references missing agent.** Returns `DelegateObservation(is_error=True)` (`tools/delegate/impl.py:122-151`, `273-282`) rather than throwing; parent LLM sees the error text and can adjust.
- **Plan with `security_risk` omitted under `LLMSecurityAnalyzer`.** `_extract_security_risk` raises `ValueError("Failed to provide security_risk field…")` → `AgentErrorEvent` (`agent.py:666-669`). Under weaker analyzers, omission is silently treated as `UNKNOWN` (`agent.py:679-683`).

## Future Considerations

- Introduce an explicit typed `Plan` object (e.g., `Plan: { id, intent, steps: list[PlannedAction], dependencies, verification }`) validated separately from individual `Action`s, so the executor can reject a whole plan before any tool runs and the UI can render plan diffs.
- Promote `TaskToolSet` (successor to deprecated `DelegateTool`) to a typed inter-agent contract: structured task definitions with input/output schemas, progress events, and cancellation, rather than free-text `tasks: dict[str,str]` folded into a single `DelegateObservation` (`tools/delegate/definition.py:54`).
- Add static plan verification: check for privilege escalation (tool allowlist per plan), path containment relative to workspace, and deterministic ordering constraints before `ParallelToolExecutor` scheduling.
- Make `DeclaredResources` mandatory for tools that claim `tool_concurrency_limit > 1` support: fail fast if a tool returns `declared=False` under concurrent config, or default to declaring no resources only after an explicit opt-in.
- Emit a `PlanValidationEvent` distinct from `AgentErrorEvent` so observability can distinguish planner mistakes from executor faults, and so `StuckDetector` can weigh them differently.

## Questions / Gaps

- No evidence found for a dedicated planner component separate from the LLM: `grep -rn planner /_sdk_inspect` returns no matches; planning is implicitly the LLM's function-calling output. Search boundary: `_sdk_inspect/sdk` (agent, conversation, tool, llm, subagent, hooks).
- No evidence found for formal tests of the planner/executor contract in the inspected snapshot: `find _sdk_inspect -name "*test*.py"` yields only `_sdk_inspect/sdk/testing/test_llm.py`; `pyproject.toml` declares test tooling but the source directory contains no `tests/` mirror, so operational safeguards are verified by code inspection, not by cited test cases.
- No evidence found for plan modification by the executor beyond truncation/rejection: `_ActionBatch`, `ParallelToolExecutor`, and `ToolDefinition.__call__` do not rewrite `Action` fields; the only transformation is `normalize_tool_call` aliases before validation. Whether downstream executors (e.g., browser, remote sandbox) further modify plans could not be determined from the local SDK tree.
- No evidence found for a published JSON Schema artifact for the plan contract separate from Pydantic `model_json_schema()` at runtime; `Schema.to_mcp_schema()` is the closest derived schema (`_sdk_inspect/sdk/tool/schema.py:178`).
- Partial evidence for cross-agent planning observability: `LocalConversation.fork` copies events and metrics (`conversation/impl/local_conversation.py:314`), but there is no inspection of forked plans before execution and no cancellation propagation inspected.

---

Generated by `Dimension 06.04: Planner/Executor Contract` against `openhands`.
