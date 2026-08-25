# Source Analysis: openai-agents-sdk

## 01.02 — Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+; core runtime under `src/agents/`, with realtime (`src/agents/realtime/`), voice (`src/agents/voice/`), and sandbox extensions under `src/agents/extensions/` and `src/agents/sandbox/` |
| Analyzed | 2026-07-02 |

## Summary

The OpenAI Agents SDK is **runtime-owned** at every step. The model is a delegate that returns structured output; the runner is the supervisor that decides what happens next. Three observations anchor the picture:

1. **The "next step" is an explicit, type-checked discriminated union built before the outer loop reads it.** `SingleStepResult.next_step` is declared as `NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption` (`src/agents/run_internal/run_steps.py:177-218`). Each turn's result is one of these four variants; the outer `while True` in `AgentRunner.run` (`src/agents/run.py:768-1497`) does `isinstance(turn_result.next_step, NextStepFinalOutput | NextStepHandoff | NextStepRunAgain | NextStepInterruption)` and dispatches in typed branches (`src/agents/run.py:1367-1497`). The streaming twin in `start_streaming` (`src/agents/run_internal/run_loop.py:670-1239`) does the same shape of dispatch (`src/agents/run_internal/run_loop.py:1065-1164`). There is no string-keyed state machine, no `if next == "..."` chain, and the model is never asked to enumerate the choices.

2. **The runtime can override, pause, reroute, and terminate the model at multiple layers.** Hard termination: `current_turn > max_turns` is checked in the loop header (`src/agents/run.py:1057-1144`, `src/agents/run_internal/run_loop.py:875-956`), with the same check re-applied to a resumed streaming run on result access (`src/agents/result.py:795-801`). Override of model choice: `maybe_reset_tool_choice` rewrites `model_settings.tool_choice` from `"required"` (or a specific tool name) to `None` once the per-agent `AgentToolUseTracker` records that the agent has used any tool (`src/agents/run_internal/tool_execution.py` referenced by `src/agents/run_internal/run_loop.py:1346,1830`; behavior covered by `tests/test_tool_choice_reset.py:36-69`). Pause: the only way to leave the loop without returning is `NextStepInterruption`, which is the return value when any tool call needs approval (`src/agents/run_internal/turn_resolution.py:713-723`) or when a resumed turn still has pending approvals (`src/agents/run_internal/turn_resolution.py:1427-1439`); the runtime persists `_current_step: NextStepInterruption | None` on `RunState` (`src/agents/run_state.py:254-255`) so an external caller can `state.approve_tool(...)` / `state.reject_tool(...)` and resume by re-entering `Runner.run(..., input=run_state)` (`src/agents/run_state.py:336-353`, `src/agents/run_internal/agent_runner_helpers.py:449-457`). Reroute: when multiple handoffs appear in a single response, all but the first are dropped and an explanatory `ToolCallOutputItem` is added (`src/agents/run_internal/turn_resolution.py:426-438`); when the model emits a refusal, the runtime raises `ModelRefusalError` (`src/agents/run_internal/turn_resolution.py:773-789`) unless a registered handler returns a final output; when a tool is not found, the runtime either raises `ModelBehaviorError` or returns an error to the model per `RunConfig.tool_not_found_behavior` (`src/agents/run_internal/turn_resolution.py:213-281`).

3. **Control flow is testable without an LLM.** The full step-resolution logic — `process_model_response` (`src/agents/run_internal/turn_resolution.py:1551`), `execute_tools_and_side_effects` (`src/agents/run_internal/turn_resolution.py:629-845`), `check_for_final_output_from_tools` (`src/agents/run_internal/turn_resolution.py:594-626`), `execute_handoffs` (`src/agents/run_internal/turn_resolution.py:403-591`), `resolve_interrupted_turn` (`src/agents/run_internal/turn_resolution.py:848-1548`) — operates on a hand-constructed `ProcessedResponse` and a `RunContextWrapper` populated by tests. `tests/test_run_step_execution.py` is 3552 lines of such tests (`tests/test_run_step_execution.py:1-3552`), and `tests/test_run_step_processing.py` (548 lines) covers the response-classification path. `tests/test_tool_use_behavior.py` directly exercises `run_loop.check_for_final_output_from_tools` without an LLM (`tests/test_tool_use_behavior.py:48-89`), and `tests/test_max_turns.py` covers the runtime cap with a `FakeModel` (`tests/test_max_turns.py:31-77`).

The model can pick **what** to do (which tool, which handoff, what text) but the runner owns **whether**, **how long**, and **what to do next**.

## Rating

**Score: 9 / 10 — Mature, durable, explicit control flow with strong runtime authority; minor deductions for state-machine duplication between streaming and non-streaming paths and for size of the outer loop body.**

Rationale:

- The control-flow contract is encoded in **named, typed variants** rather than a free-form string. `NextStepHandoff` / `NextStepFinalOutput` / `NextStepRunAgain` / `NextStepInterruption` are `@dataclass`es with no payload, an `Agent`, a `Any` output, or a `list[ToolApprovalItem]` respectively (`src/agents/run_internal/run_steps.py:154-173`). They are not `Enum`s, but the union type is enforced at the call sites that read them.
- The **runtime has explicit authority** to stop, rewrite, and pause the model. `max_turns` cap with `MaxTurnsExceeded` and an optional `error_handlers["max_turns"]` recovery path (`src/agents/run.py:1058-1144`, `src/agents/run_internal/run_loop.py:881-956`, `src/agents/run_error_handlers.py:53`, `src/agents/run_internal/error_handlers.py:140`); `maybe_reset_tool_choice` to defeat `tool_choice="required"`/`"<tool>"` after a tool call (`src/agents/run_internal/run_loop.py:1346,1830`); `run_input_guardrails` + model `asyncio.gather` with `model_task.cancel()` if a parallel guardrail tripwires (`src/agents/run.py:1219-1245`); `ItemHelpers.extract_refusal` raising `ModelRefusalError` unless a handler returns an output (`src/agents/run_internal/turn_resolution.py:773-789`); the `model_task` cancel path on first-turn guardrail tripwire (`src/agents/run.py:1230-1245`).
- The model cannot bypass the runtime. There is no API path for the model to address the runner directly; it can only emit typed output items (function calls, handoff calls, messages, refusals, shell calls, etc.) which `process_model_response` (`src/agents/run_internal/turn_resolution.py:1551-1999`) parses into the `ProcessedResponse` discriminated struct (`src/agents/run_internal/run_steps.py:115-152`).
- The **durable pause/resume boundary** is a first-class state, `RunState`, with a `current_turn`, `_max_turns`, `_current_step: NextStepInterruption | None`, `_last_processed_response`, and a versioned schema (`CURRENT_SCHEMA_VERSION = "1.11"`, `src/agents/run_state.py:131-149` and `src/agents/run_state.py:184-321`). This is one of the most operationally robust pause surfaces surveyed.
- **Tool-approval decisions are recorded by the runtime, not the model.** `RunContextWrapper._approvals` (`src/agents/run_context.py:29-211`, `src/agents/run_context.py:309-375`) is a tri-state (True / False / None) record per tool + call_id; `approve_tool` and `reject_tool` mutate it; `get_approval_status` reads it during `_collect_runs_by_approval` (`src/agents/run_internal/tool_planning.py:376-447`) and during `_select_function_tool_runs_for_resume` (`src/agents/run_internal/tool_planning.py:490-539`). The model can mark a tool call as wanting approval via `needs_approval` (`src/agents/tool.py:417-435`, `src/agents/agent.py:525-528`, `src/agents/util/_approvals.py:13-32`), but the runtime decides whether to run it.
- **Testability without an LLM is excellent**: `tests/test_run_step_execution.py` is 3552 lines, `tests/test_run_step_processing.py` is 548 lines, `tests/test_tool_use_behavior.py` is 226 lines, `tests/test_max_turns.py` is 489 lines. `run_loop.check_for_final_output_from_tools` is called directly in `tests/test_tool_use_behavior.py:51-55` with hand-built `FunctionToolResult` objects.
- **Deductions:**
  1. The non-streaming and streaming `while True` loops **duplicate the next_step dispatch** rather than share a single transition function. `AgentRunner.run` has its own dispatch (`src/agents/run.py:1367-1497`); `start_streaming` has its own (`src/agents/run_internal/run_loop.py:1065-1164`); `resolve_interrupted_turn` returns the next step independently (`src/agents/run_internal/turn_resolution.py:587,719,837-845,1433,1545`); the streaming `run_single_turn_streamed` then runs another check (`src/agents/run_internal/run_loop.py:1136-1147`). Each copy is ~30 lines of `isinstance` chains; a `SingleStepResult` → "next outer-loop action" function would be cleaner. The duplication is intentional (per `AGENTS.md:55-67` "Keep streaming and non-streaming paths behaviorally aligned"), but the contract is **the same discriminated union**, not the same function.
  2. The `AgentRunner.run` body (`src/agents/run.py:768-1497`) is ~730 lines and interleaves session persistence, server-conversation tracking, sandbox enqueueing, and the next-step dispatch. A `apply_turn_result(turn_result, ctx) -> Action` helper would shrink this. `AGENTS.md:55-67` already names this as a refactor target.
  3. `tool_choice` is rewritten in the model layer via `maybe_reset_tool_choice` (`src/agents/run_internal/run_loop.py:1346,1830`), but the rewrite happens *after* the model has already returned a response. It only protects the *next* call. A model that emits the same tool in a loop is caught at the next iteration, not mid-response.
  4. The cancel path on streaming (`src/agents/run.py:179-280`, `src/agents/result.py:702-925`) uses `asyncio.Task.cancel()` plus a `_cancel_mode == "after_turn"` flag. It is correct, but the cancel authority is split between the result object and the run loop rather than a single `Runner.cancel()` API.

## Evidence Collected

Every entry includes a file path with line numbers. Format `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public Runner entry | `class Runner` is the public async/sync/streaming facade. | `src/agents/run.py:197-441` |
| Experimental inner runner | `class AgentRunner` is the actual implementation. | `src/agents/run.py:444-1876` |
| `Runner` instance | `DEFAULT_AGENT_RUNNER = AgentRunner()` — single runtime singleton. | `src/agents/run.py:1879` |
| Outer loop header (non-streaming) | `try: while True:` — single outer loop driving the run. | `src/agents/run.py:767-768` |
| Outer loop header (streaming) | `try: while True:` — same shape in the streaming driver. | `src/agents/run_internal/run_loop.py:670-671` |
| NextStep union | The four-variant discriminated union used as the loop's transition value. | `src/agents/run_internal/run_steps.py:154-173` |
| NextStepRunAgain | The "loop again" variant — runtime-owned, not model-driven. | `src/agents/run_internal/run_steps.py:164-166` |
| NextStepFinalOutput | The "stop with output" variant. | `src/agents/run_internal/run_steps.py:159-162` |
| NextStepHandoff | The "switch to a different agent" variant. | `src/agents/run_internal/run_steps.py:154-157` |
| NextStepInterruption | The "pause for human input" variant — drives durable resume. | `src/agents/run_internal/run_steps.py:169-174` |
| SingleStepResult container | Holds `next_step` as a typed union; the only contract the outer loop reads. | `src/agents/run_internal/run_steps.py:177-218` |
| ProcessedResponse (response classification) | The model output, classified into typed run arrays (handoffs, functions, computer_actions, custom_tool_calls, shell_calls, apply_patch_calls, local_shell_calls, mcp_approval_requests, function_tools_not_found). | `src/agents/run_internal/run_steps.py:115-152` |
| Next-step type guard (non-streaming) | `isinstance(turn_result.next_step, NextStepFinalOutput | NextStepInterruption | NextStepHandoff | NextStepRunAgain)` dispatch in the loop body. | `src/agents/run.py:1366-1497` |
| `else: raise AgentsException` fallback | Unknown next-step types raise — runtime refuses to continue. | `src/agents/run.py:1494-1497` |
| Next-step type guard (streaming) | Same `isinstance` chain in the streaming loop. | `src/agents/run_internal/run_loop.py:1065-1164` |
| `current_turn` counter increment | Runtime-owned turn counter, not model-derived. | `src/agents/run.py:1057` |
| `max_turns` cap | `if max_turns is not None and current_turn > max_turns:` enforced by the runtime. | `src/agents/run.py:1058` |
| Max-turns cap (streaming twin) | Same check in the streaming loop. | `src/agents/run_internal/run_loop.py:875-881` |
| Max-turns error class | `MaxTurnsExceeded(AgentsException)` — runtime-typed error, not a model string. | `src/agents/exceptions.py:56-64` |
| `MaxTurnsExceeded` path on streaming result | Checked lazily on `to_result()` / `stream_events()` access. | `src/agents/result.py:795-801` |
| Default cap | `DEFAULT_MAX_TURNS = 10` — the canonical runtime cap constant. | `src/agents/run_config.py:33` |
| Max-turns error handler | `error_handlers["max_turns"]` lets the user override the runtime's stop and supply a final output. | `src/agents/run_error_handlers.py:53`, `src/agents/run_internal/error_handlers.py:140` |
| `tool_choice` rewrite (model override) | `maybe_reset_tool_choice(public_agent, tool_use_tracker, model_settings)` — runtime mutates the tool choice after a tool was used. | `src/agents/run_internal/run_loop.py:1346,1830` |
| `tool_use_behavior` enum | `"run_llm_again" | "stop_on_first_tool" | StopAtTools | ToolsToFinalOutputFunction` — the runtime, not the model, decides when a tool result becomes the final answer. | `src/agents/agent.py:345-365` |
| `check_for_final_output_from_tools` | The single function that maps tool results to "stop or continue". | `src/agents/run_internal/turn_resolution.py:594-626` |
| `"stop_on_first_tool"` | First tool result becomes the final output — runtime shortcut. | `src/agents/run_internal/turn_resolution.py:605-606` |
| `StopAtTools` mapping | Configured names stop the loop. | `src/agents/run_internal/turn_resolution.py:607-614` |
| Custom callable override | A user-supplied callable decides final-or-continue. | `src/agents/run_internal/turn_resolution.py:615-623` |
| Invalid value rejection | `raise UserError(f"Invalid tool_use_behavior: {agent.tool_use_behavior}")` — runtime rejects unknown values. | `src/agents/run_internal/turn_resolution.py:624-626` |
| Tool-call dispatch plan | `ToolExecutionPlan` — runtime-built struct that fan-outs to per-runtime executors. | `src/agents/run_internal/tool_planning.py:177-193` |
| Per-runtime executors | `execute_function_tool_calls`, `execute_custom_tool_calls`, `execute_local_shell_calls`, `execute_shell_calls`, `execute_apply_patch_calls`, `execute_computer_actions` — each tool family has a separate runtime-owned executor. | `src/agents/run_internal/tool_execution.py:1973,1995,2020,2045,2070,2095` |
| Parallel tool fan-out | `asyncio.gather(execute_function_tool_calls, execute_computer_actions, …)` — runtime coordinates parallelism, not the model. | `src/agents/run_internal/tool_planning.py:580-619` |
| Approval-driven dispatch | `_collect_runs_by_approval` — runtime filters which tool calls run based on `_approvals` state, not model choice. | `src/agents/run_internal/tool_planning.py:376-447` |
| Approval reset path | `_select_function_tool_runs_for_resume` — when resuming, runtime re-checks approval before executing a tool. | `src/agents/run_internal/tool_planning.py:490-539` |
| Approval store | `_ApprovalRecord` (per-tool entry) with permanent/specific-call_id approval and rejection states. | `src/agents/run_context.py:29-40` |
| Approval state accessors | `get_approval_status`, `is_tool_approved` — tri-state (True / False / None) reads. | `src/agents/run_context.py:178-211,377-445` |
| Approval mutators | `approve_tool` / `reject_tool` — runtime state changes; model never calls them. | `src/agents/run_context.py:355-375` |
| `needs_approval` evaluation helper | `evaluate_needs_approval_setting` — runtime evaluates the tool's `needs_approval` policy. | `src/agents/util/_approvals.py:13-32` |
| `AgentToolUseTracker` | Tracks which tools each agent has used; consumed by `maybe_reset_tool_choice` to rewrite the next model call. | `src/agents/run_internal/tool_use_tracker.py:50-110` |
| Handoff dispatch | `execute_handoffs` — runtime processes `ToolRunHandoff` items. | `src/agents/run_internal/turn_resolution.py:403-591` |
| Multi-handoff guard | When multiple handoffs appear, the runtime keeps one and synthesizes rejection output for the rest. | `src/agents/run_internal/turn_resolution.py:426-438` |
| Handoff input filter | User-supplied `HandoffInputFilter` rewrites the next agent's input — runtime hook, not model hook. | `src/agents/run_internal/turn_resolution.py:505-580` |
| HandoffInputData struct | The runtime contract passed to the user filter. | `src/agents/handoffs/__init__.py:42-87` |
| `Handoff` dataclass | Holds `tool_name`, `on_invoke_handoff`, `input_filter`, `nest_handoff_history`, `is_enabled`, `_agent_ref`. | `src/agents/handoffs/__init__.py:93-166` |
| Handoff span | `handoff_span(from_agent=...)` — runtime-emitted tracing event. | `src/agents/run_internal/turn_resolution.py:441-441` |
| `Handoff.default_tool_name` | The conventional `transfer_to_<agent>` name is enforced at construction. | `src/agents/handoffs/__init__.py:172-176` |
| Handoff `is_enabled` predicate | Runtime can hide a handoff from the model by setting `is_enabled=False` (or a callable). | `src/agents/handoffs/__init__.py:153-161,315-323` |
| Agent switch in non-streaming loop | `current_agent = cast(Agent[TContext], turn_result.next_step.new_agent)` — runtime sets the next speaker. | `src/agents/run.py:1011-1015` |
| Agent switch in streaming loop | Same `isinstance(...NextStepHandoff)` branch in streaming. | `src/agents/run_internal/run_loop.py:1065-1107` |
| Refusal handling | `ItemHelpers.extract_refusal` produces a `ModelRefusalError`; the runtime only lets the loop continue if `error_handlers` returns a valid output. | `src/agents/run_internal/turn_resolution.py:763-808` |
| Tool-not-found behaviour | `RunConfig.tool_not_found_behavior` lets the runtime either raise or send an error back to the model. | `src/agents/run_config.py:332-338` |
| Unknown tool emission | `_build_tool_not_found_output_items` synthesizes a `ToolCallOutputItem` with a default or formatted message. | `src/agents/run_internal/turn_resolution.py:259-281` |
| `call_model_input_filter` hook | User-supplied filter rewrites the model input before each call — runtime, not model. | `src/agents/run_config.py:298-306` |
| `run_input_guardrails` parallelism | `asyncio.gather(run_input_guardrails(parallel), model_task)` — runtime runs first-turn guardrails and the model in parallel. | `src/agents/run.py:1219-1245` |
| Guardrail trip cancels model | `if not model_task.done(): model_task.cancel()` — runtime authority over the in-flight model call. | `src/agents/run.py:1231-1234` |
| Streaming input-guardrail timing | `streamed_result._input_guardrails_task` runs concurrently with the model; tripwire raised on stream access. | `src/agents/run_internal/run_loop.py:985-995`, `src/agents/result.py:795-801` |
| Output guardrails | `run_output_guardrails` runs on the final output; tripwire can override termination. | `src/agents/run.py:1368-1374,962-968` |
| Tool input guardrails | `tool_input_guardrail_results` block a tool call before execution (and emit pending interruption). | `src/agents/run_internal/turn_resolution.py:629-845` |
| Tool output guardrails | `tool_output_guardrail_results` run after tool execution. | `src/agents/run_internal/turn_execution.py` (cross-references) |
| `RunState` serialization | `_serialize_approvals` — runtime persists approval state across resume. | `src/agents/run_state.py:359-369` |
| `RunState` schema version | `CURRENT_SCHEMA_VERSION = "1.11"` — versioned snapshot for safe resume. | `src/agents/run_state.py:131-149` |
| `_current_step` field | Persists `NextStepInterruption | None` so the next run picks up the pause boundary. | `src/agents/run_state.py:254-255` |
| Resume re-entry | `if isinstance(input, RunState): ...` — the runner treats a `RunState` input as a resume, not a fresh start. | `src/agents/run.py:469-505` |
| `_current_step` accessors | `get_interruptions`, `approve_tool`, `reject_tool` on `RunState`. | `src/agents/run_state.py:323-356` |
| Approval rebuild on resume | `_rebuild_approvals` hydrates the tri-state approval map from serialized state. | `src/agents/run_context.py:447-468` |
| Resume branch dispatch | `isinstance(run_state._current_step, NextStepInterruption)` — re-enters the resolver for the pending turn. | `src/agents/run.py:840-1025` |
| `resolve_interrupted_turn` | The dedicated resolution path for a turn that paused for approval; emits a new `NextStepInterruption` if any pending approvals remain. | `src/agents/run_internal/turn_resolution.py:848-1548` |
| `ToolApprovalItem` type | The typed `RunItem` subclass carrying a pending approval through the pipeline. | `src/agents/items.py` (referenced by `src/agents/run_internal/run_steps.py:170-174`) |
| `approvals_from_step` helper | Extracts a flat list of `ToolApprovalItem` from a `NextStepInterruption`. | `src/agents/run_internal/approvals.py:42-56` |
| `build_interruption_result` | Constructs the `RunResult` carrying the interruptions and the processed response for resume. | `src/agents/run_internal/agent_runner_helpers.py:449-487` |
| `update_run_state_for_interruption` | Persists the new `_current_step` and `_last_processed_response`. | `src/agents/run_internal/agent_runner_helpers.py` (referenced at `src/agents/run.py:915-923`) |
| `AgentBindings` | Carries `public_agent` and `execution_agent` — runtime may rewrite the execution identity (e.g., for sandbox). | `src/agents/run_internal/agent_bindings.py:16-38` |
| `bind_public_agent` / `bind_execution_agent` | Default (identity) and sandbox-aware bindings. | `src/agents/run_internal/agent_bindings.py:24-37` |
| `start_streaming` driver | Streaming twin of the outer loop, with the same `NextStep*` dispatch. | `src/agents/run_internal/run_loop.py:440-1239` |
| `run_single_turn` | Single non-streaming turn: model call + tool execution. | `src/agents/run_internal/run_loop.py:1708-1795` |
| `run_single_turn_streamed` | Single streaming turn: model stream + tool execution. | `src/agents/run_internal/run_loop.py:1242-1705` |
| `get_single_step_result_from_response` | The single function that converts a `ModelResponse` into a `SingleStepResult` for both streaming and non-streaming. | `src/agents/run_internal/turn_resolution.py:2004-2057` |
| `process_model_response` | Classifies raw model output into `ProcessedResponse` — single source of truth for response parsing. | `src/agents/run_internal/turn_resolution.py:1551-1999` |
| Stream event contract | `RawResponsesStreamEvent`, `RunItemStreamEvent`, `AgentUpdatedStreamEvent` — the runtime-emitted stream events. | `src/agents/stream_events.py` |
| `RunResultStreaming` cancel | `streamed_result.cancel()` triggers a `QueueCompleteSentinel` and `_stored_exception` propagation. | `src/agents/result.py:702-925` |
| `RunResultStreaming._cancel_mode` | `"after_turn"` flag — runtime can defer cancellation to the next turn boundary. | `src/agents/result.py` (referenced at `src/agents/run_internal/run_loop.py:842,1109,1161`) |
| `cancel` propagation | `_event_queue.put_nowait(QueueCompleteSentinel())` — runtime signals the consumer to stop. | `src/agents/run_internal/run_loop.py:844,911,955,970,1111,1163,1239` |
| `_should_attach_generic_agent_error` | Runtime decides which errors to attach to spans vs. propagate. | `src/agents/run_internal/run_loop.py:261-265` |
| `validate_run_hooks` | Runtime type-checks the user's `RunHooks` before starting. | `src/agents/run_internal/turn_preparation.py:33-115` |
| `_max_turns_handled` flag | Distinguishes "max-turns raised a handler that finished the run" from "max-turns raised and the loop just exited". | `src/agents/result.py:514,795-801,894-950` |
| Server-managed conversation overrides | `OpenAIServerConversationTracker` rewrites input and output on the server side, so the runner still drives the loop but the server holds state. | `src/agents/run_internal/oai_conversation.py:98-...` |
| Conversation lock retries | `get_response_with_retry` + `rewind_session_items` — runtime re-emits the input if the server reports a conversation lock. | `src/agents/run_internal/model_retry.py` (cross-referenced in `src/agents/run.py:558-595,1829-1907`) |
| Handoff history nesting | `nest_handoff_history` — runtime rewrites the next agent's history; not a model-side decision. | `src/agents/handoffs/history.py` |
| `RunErrorHandlers` | Per-error-key override map; let the user rewrite the runtime's stop behavior. | `src/agents/run_error_handlers.py` |
| `validate_handler_final_output` | Runtime validates the user's handler output against the agent's `output_type`. | `src/agents/run_internal/error_handlers.py:75-150` |
| `resolve_run_error_handler_result` | The single dispatch from an exception to a handler. | `src/agents/run_internal/error_handlers.py` |
| `Build_run_error_data` | The struct passed to error handlers — input, items, last_agent, etc. | `src/agents/run_internal/error_handlers.py:25-...` |
| Lifecycle hooks | `RunHooksBase` and `AgentHooksBase` — the runtime calls these on `on_llm_start`, `on_llm_end`, `on_agent_start`, `on_agent_end`, `on_handoff`, `on_tool_start`, `on_tool_end`. | `src/agents/lifecycle.py:13-200` |
| Hook call sites | `on_llm_start` is called before each model call inside the runner. | `src/agents/run_internal/run_loop.py:1394-1406,1835-1847` |
| `on_agent_start` | Called at the start of each turn (or on handoff). | `src/agents/run_internal/run_loop.py:1317-1331,1742-1749` |
| `on_handoff` | Called when a handoff is executed. | `src/agents/run_internal/turn_resolution.py:470-485` |
| `on_agent_end` | Called when a final output is produced (and `tool_use_behavior` says final). | `src/agents/run_internal/turn_resolution.py:284-302` |
| `stream_step_items_to_queue` | Runtime bridges the step result to the streamed event queue. | `src/agents/run_internal/streaming.py:28-66` |
| `stream_step_result_to_queue` | Emits one or more `RunItemStreamEvent`s for a `SingleStepResult`. | `src/agents/run_internal/streaming.py:68-...` |
| `_complete_stream_interruption` | Helper that closes a stream with an interruption. | `src/agents/run_internal/run_loop.py:290-299` |
| `_finalize_streamed_interruption` | Finalizes the streamed result for an interruption, including session save. | `src/agents/run_internal/run_loop.py:419-434` |
| `_finalize_streamed_final_output` | Finalizes the streamed result for a final output, including output guardrails and session save. | `src/agents/run_internal/run_loop.py:391-416` |
| Async/sync boundary | `run_sync` schedules the async `run` on the default loop, explicitly handles cancellation, and re-uses the default loop across calls. | `src/agents/run.py:1564-1645` |
| Sync loop reuse | The default loop is kept open across calls so loop-bound `asyncio.Lock` instances stay valid. | `src/agents/run.py:1603-1611,1640-1645` |
| `AgentHooks.on_llm_start` invocation | The agent's own `on_llm_start` runs in parallel with the global hook. | `src/agents/run_internal/run_loop.py:1394-1406` |

## Answers to Dimension Questions

### 1. Who decides what happens next?
**The runtime.** Each turn's `ModelResponse` is classified by `process_model_response` into a `ProcessedResponse` (`src/agents/run_internal/turn_resolution.py:1551-1999`), then `execute_tools_and_side_effects` decides the next step and packages it as a `SingleStepResult` whose `next_step` is one of four typed variants (`src/agents/run_internal/turn_resolution.py:629-845`, `src/agents/run_internal/run_steps.py:154-218`). The outer loop in `AgentRunner.run` (`src/agents/run.py:1366-1497`) and `start_streaming` (`src/agents/run_internal/run_loop.py:1065-1164`) read this discriminant and dispatch in `isinstance` branches. The model is never asked to choose from the four variants.

### 2. Can the LLM bypass runtime control?
**No.** The LLM can only emit typed output items (function calls, handoff calls, messages, refusals, shell calls, apply_patch calls, local_shell calls, MCP approval requests, computer actions, custom tool calls, reasoning items, file search calls, web search calls, code interpreter calls, image generation calls, etc.). All of these are parsed by `process_model_response` into the `ProcessedResponse` discriminated struct (`src/agents/run_internal/run_steps.py:115-152`, `src/agents/run_internal/turn_resolution.py:1551-1999`). The runtime then applies its guards: `max_turns` cap (`src/agents/run.py:1057-1058`), `tool_use_behavior` mapping (`src/agents/run_internal/turn_resolution.py:594-626`), `tool_choice` reset (`src/agents/run_internal/run_loop.py:1346,1830`), approval gating (`src/agents/run_internal/tool_planning.py:376-447`), input/output guardrails (`src/agents/run.py:1181-1245,962-968,1368-1374`), and tool-not-found policy (`src/agents/run_internal/turn_resolution.py:259-281`). The model can pick *which* function to call, but it cannot run it without the runtime's `execute_function_tool_calls` (`src/agents/run_internal/tool_execution.py:1973-1992`).

### 3. Can the runtime reject or rewrite the next action?
**Yes, at multiple layers.**
- **Reject outright:** `MaxTurnsExceeded` is raised when `current_turn > max_turns` (`src/agents/run.py:1066,1081`, `src/agents/run_internal/run_loop.py:889`); `ModelRefusalError` when the model emits a refusal and no handler returns a value (`src/agents/run_internal/turn_resolution.py:773-789`); `ModelBehaviorError` for missing tool, missing shell tool, missing executor (`src/agents/run_internal/turn_resolution.py:1633-1652,215`); guardrail tripwires raise `InputGuardrailTripwireTriggered` / `OutputGuardrailTripwireTriggered` (`src/agents/run.py:1181,1230`, `src/agents/run_internal/turn_resolution.py:1368-1374`).
- **Rewrite the next action:** `Handoff.input_filter` rewrites the next agent's input (`src/agents/run_internal/turn_resolution.py:505-580`); `nest_handoff_history` collapses the prior transcript into a single assistant message (`src/agents/handoffs/history.py`); `call_model_input_filter` rewrites the input before each model call (`src/agents/run_config.py:298-306`, `src/agents/run_internal/turn_preparation.py:155-...`); `maybe_reset_tool_choice` rewrites `model_settings.tool_choice` to defeat `tool_choice="required"` after a tool has been used (`src/agents/run_internal/run_loop.py:1346,1830`); `_resolve_server_managed_handoff_behavior` disables handoff input filters when `conversation_id` is in use (`src/agents/run_internal/turn_resolution.py:371-400`).
- **Pause:** When any tool call's `needs_approval` returns `True` and the user has not yet approved it, the runtime emits `NextStepInterruption` and returns, persisting `_current_step` on `RunState` (`src/agents/run_internal/turn_resolution.py:713-723`, `src/agents/run_state.py:254-255`). The model never gets a chance to act on the unapproved tool.

### 4. Are transitions explicit or scattered?
**Explicit.** Transitions are encoded as the `NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption` union type on `SingleStepResult.next_step` (`src/agents/run_internal/run_steps.py:178-218`). The dispatch sites are `isinstance(turn_result.next_step, NextStepXxx)` chains in three places — `AgentRunner.run` (`src/agents/run.py:1366-1497`), `start_streaming` (`src/agents/run_internal/run_loop.py:1065-1164`), and the resume branch in both (`src/agents/run.py:903-1025`, `src/agents/run_internal/run_loop.py:791-840`). All three copies end with an `else: raise AgentsException` guard (`src/agents/run.py:1494-1497`). The functions that *produce* these next steps are also named: `execute_final_output_step` (`src/agents/run_internal/turn_resolution.py:305-335`), `execute_handoffs` (`src/agents/run_internal/turn_resolution.py:403-591`), `resolve_interrupted_turn` (`src/agents/run_internal/turn_resolution.py:848-1548`), `_maybe_finalize_from_tool_results` (`src/agents/run_internal/turn_resolution.py:171-210`), `check_for_final_output_from_tools` (`src/agents/run_internal/turn_resolution.py:594-626`), `process_hosted_mcp_approvals` (`src/agents/run_internal/tool_execution.py:1204-...`). Scattered fallthroughs are rare; the failure mode is unknown-step `AgentsException`.

The duplication between streaming and non-streaming dispatch is the main "implicit" surface — the two branches are *the same shape* but not *the same code*. `AGENTS.md:55-67` documents the intent ("Changes to `run_internal/run_loop.py` (`run_single_turn`, `run_single_turn_streamed`, `get_new_response`, `start_streaming`) should be mirrored") and the consequence is that the contract is the discriminated union, not a shared function.

### 5. Is control flow testable without calling an LLM?
**Yes, extensively.**
- `tests/test_run_step_execution.py:1-3552` exercises `run_single_turn`, `get_single_step_result_from_response`, `execute_tools_and_side_effects`, and the streaming twin against hand-built `ProcessedResponse` and `ModelResponse` objects, with a `FakeModel` for the LLM layer when needed.
- `tests/test_run_step_processing.py:1-548` covers `process_model_response` against fixtures for each response shape (handoff, function, computer, shell, custom, MCP, refusal, etc.).
- `tests/test_tool_use_behavior.py:48-89` calls `run_loop.check_for_final_output_from_tools` directly with hand-built `FunctionToolResult` arrays (`tests/test_tool_use_behavior.py:25-44`).
- `tests/test_max_turns.py:31-77` asserts `MaxTurnsExceeded` is raised after the runtime cap.
- `tests/test_tool_choice_reset.py:11-69` asserts `maybe_reset_tool_choice` rewrites `tool_choice` for `"required"` and named-tool cases.
- `tests/test_run_internal_approvals.py` exercises `_collect_runs_by_approval` against hand-built `RunContextWrapper` and approval state.
- `tests/test_handoff_tool.py`, `tests/test_handoff_prompt.py`, `tests/test_handoff_history_duplication.py` cover handoff dispatch and input filtering without an LLM.

A single `FakeModel` (in `tests/fake_model.py`) replaces the chat layer; the runtime's structure is verified independently of the model.

## Architectural Decisions

- **Discriminated-union next-step.** `NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption` is the only contract the outer loop reads. The decision is intentional and named in `AGENTS.md`-adjacent design; the variants are not `Enum` members but `@dataclass`es, which lets them carry payloads (`output: Any`, `new_agent: Agent`, `interruptions: list[ToolApprovalItem]`).
- **Two parallel outer loops (streaming / non-streaming).** `AgentRunner.run` (non-streaming, `src/agents/run.py:450-1497`) and `start_streaming` (`src/agents/run_internal/run_loop.py:440-1239`) are deliberately separate, with mirrored inner functions (`run_single_turn` / `run_single_turn_streamed`, `get_new_response` / streaming retry in `run_single_turn_streamed`). The two loops share the *contract* (`SingleStepResult.next_step`) but not the dispatch code. This is the main source of duplication.
- **Runtime owns tool approval.** `RunContextWrapper._approvals` is a tri-state per-tool-per-call_id record (`src/agents/run_context.py:29-211,309-375`). The model emits an approval-worthy tool call; the runtime's `_collect_runs_by_approval` (`src/agents/run_internal/tool_planning.py:376-447`) checks the store and either runs the tool, rejects it, or adds it to `NextStepInterruption`. There is no path for the model to bypass the runtime.
- **Runtime owns tool-result → final-output mapping.** `tool_use_behavior` (`src/agents/agent.py:345-365`) is a user-configured policy, but the *evaluation* is in `check_for_final_output_from_tools` (`src/agents/run_internal/turn_resolution.py:594-626`), which is a runtime function. The model never decides "this tool result is my final answer"; the runtime does.
- **Runtime owns prompt rewrite before each call.** `call_model_input_filter`, `nest_handoff_history`, `Handoff.input_filter`, `RunConfig.call_model_input_filter` are all user-supplied hooks invoked by the runtime. The model receives the post-rewrite input.
- **Runtime owns max-turns and can be customized.** `max_turns` is enforced by the runtime (`src/agents/run.py:1057-1081`); `error_handlers["max_turns"]` lets the user replace the runtime's stop with a final output (`src/agents/run_error_handlers.py:53`, `src/agents/run_internal/error_handlers.py:140`); the streaming runner also re-checks on result access (`src/agents/result.py:795-801`).
- **Durable pause/resume is a first-class state.** `RunState._current_step: NextStepInterruption | None` (`src/agents/run_state.py:254-255`) and the `_serialize_approvals` (`src/agents/run_state.py:359-369`) make the runner's pause boundary serializable. The versioned `CURRENT_SCHEMA_VERSION = "1.11"` (`src/agents/run_state.py:131-149`) protects against schema drift.
- **Handoff input is filterable but only at the runtime level.** `HandoffInputFilter` is a `Callable[[HandoffInputData], MaybeAwaitable[HandoffInputData]]` (`src/agents/handoffs/__init__.py:42-87,225-336`) invoked by the runtime after `on_invoke_handoff`. Server-managed conversations disable this (`src/agents/run_internal/turn_resolution.py:371-400`).
- **Streaming has after-turn cancel only.** `streamed_result._cancel_mode == "after_turn"` (`src/agents/run_internal/run_loop.py:842,1109,1161`) is the only built-in mid-stream interrupt; there is no API to inject a sentinel mid-turn from outside.
- **Async/sync split uses the default loop.** `run_sync` reuses the default event loop and explicitly does not close it (`src/agents/run.py:1603-1611,1640-1645`) so that loop-bound `asyncio.Lock` instances in user code stay valid across calls.

## Notable Patterns

- **Discriminated-union state machine** as the next-step type (`src/agents/run_internal/run_steps.py:177-218`).
- **Plan dataclass** for per-turn tool execution (`ToolExecutionPlan`, `src/agents/run_internal/tool_planning.py:177-193`).
- **Per-tool-runtime executors** behind a common `asyncio.gather` (`src/agents/run_internal/tool_execution.py:1973,1995,2020,2045,2070,2095`, `src/agents/run_internal/tool_planning.py:572-619`).
- **Approval tri-state** with permanent and per-call_id allow/deny (`src/agents/run_context.py:29-40,178-211,309-375`).
- **Versioned `RunState` snapshot** for durable pause/resume (`src/agents/run_state.py:131-321`).
- **First-turn-only guardrails** (`current_turn == 0 and not resuming_turn` at `src/agents/run.py:770-774`, `src/agents/run_internal/run_loop.py:672-678`) with parallel-spawn/cancel (`src/agents/run.py:1219-1245`).
- **Reflection-based tool-name aliasing** for the deferred-loading approval path (`src/agents/_tool_identity.py`, `src/agents/run_context.py:100-144`).
- **Cancellation by sentinel** in streaming — `QueueCompleteSentinel` placed on the event queue (`src/agents/run_internal/run_loop.py:844,911,955,970,1111,1163,1239`, `src/agents/run_internal/run_steps.py:52-56`).
- **Server-managed conversation via tracker** — the runtime owns the input-delta calculation (`src/agents/run_internal/oai_conversation.py:98-...`).
- **Re-attachable trace** on resume — `TraceState.from_trace` is serialized into `RunState._trace_state` (`src/agents/result.py:114-117`, `src/agents/run_state.py`).

## Tradeoffs

- **Dispatch code is duplicated between streaming and non-streaming.** The two loops are 730 / 580 lines respectively (`src/agents/run.py:767-1497` and `src/agents/run_internal/run_loop.py:670-1239`), and both end with the same `isinstance(turn_result.next_step, NextStep*)` ladder. Future contributors must mirror changes per `AGENTS.md:55-67`. The trade: streaming events fire immediately; the cost: two dispatch sites to keep in sync.
- **`tool_choice` is reset between turns, not mid-response.** `maybe_reset_tool_choice` runs once per turn (`src/agents/run_internal/run_loop.py:1346,1830`), so a model that emits the same tool in a single response is caught only on the *next* iteration. This is acceptable for the common case but does not defend against a model that packs many tool calls into one response with `tool_choice="required"`.
- **Cancellation is "after turn" only.** There is no public API to interrupt a turn mid-stream from outside the runner; the only built-in path is `streamed_result.cancel()` with `_cancel_mode = "after_turn"` (`src/agents/result.py:702-925`). Mid-stream injection of an approval decision requires waiting for the current turn to complete.
- **`AgentRunner.run` `while True` body is large.** ~730 lines between `try: while True:` (`src/agents/run.py:768`) and the loop's exit points at `return _finalize_result(result)` (`src/agents/run.py:946,1010,1144,1414,1471`). The body interleaves sandbox prep, session persistence, server-conversation tracking, and the next-step dispatch. `AGENTS.md:55-67` already names this as a refactor target. The trade: keeping streaming and non-streaming behaviorally aligned in one place; the cost: the file is the longest in the package.
- **The state machine is encoded as dataclasses, not enums.** `NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption` are `@dataclass`es with no shared base (`src/agents/run_internal/run_steps.py:154-173`). The trade: dataclasses can carry payloads; the cost: there is no single `NextStep` enum to exhaustively match on, and the `else: raise AgentsException` fallback is the only way to detect an unknown variant.
- **`ToolRunFunctionNotFound` is a recoverable model error.** When the model emits a function call for a tool that the agent does not have, the runtime either raises (`RunConfig.tool_not_found_behavior = "raise_error"`, the default) or emits a `ToolCallOutputItem` for the model to read on the next turn (`"return_error_to_model"`, `src/agents/run_config.py:332-338`). The trade: the default is to raise, which means a hallucinated tool name is a hard stop. The "return error" path keeps the loop alive but lets the model self-correct.
- **Handoff input filter disabled on server-managed conversations.** When `conversation_id` / `previous_response_id` / `auto_previous_response_id` is in use, `HandoffInputFilter` raises `UserError` at construction time of the handoff plan (`src/agents/run_internal/turn_resolution.py:384-389`). The trade: a clean server-side transcript; the cost: a feature that is widely used in non-server-managed flows is unavailable in server-managed flows.
- **Streaming `is_complete` is set in many places.** `streamed_result.is_complete = True` appears in `_finalize_streamed_interruption`, `_finalize_streamed_final_output`, the max-turns cap path, the `after_turn` cancel, and the finally block (`src/agents/run_internal/run_loop.py:298,412,843,948,1109,1110,1161,1162,1197,1201,1237-1239`). The trade: defensive; the cost: easy to miss a path where a sentinel is not queued.

## Failure Modes / Edge Cases

- **Max turns exceeded** with optional `error_handlers["max_turns"]` to recover. `src/agents/run.py:1057-1144`, `src/agents/run_internal/run_loop.py:881-956`, `src/agents/run_error_handlers.py:53`.
- **Model refusal** triggers `ModelRefusalError` unless a handler supplies a final output. `src/agents/run_internal/turn_resolution.py:763-789`, `src/agents/run_internal/error_handlers.py`.
- **Tool not found** either raises `ModelBehaviorError` or returns an error to the model per `RunConfig.tool_not_found_behavior`. `src/agents/run_internal/turn_resolution.py:213-281`, `src/agents/run_config.py:332-338`.
- **Tool approval pending** → `NextStepInterruption` is returned; `_current_step` is persisted on `RunState`; the caller can `state.approve_tool(...)` and re-enter `Runner.run(..., input=run_state)`. `src/agents/run_internal/turn_resolution.py:713-723,1427-1439`, `src/agents/run_state.py:254-255,323-356`.
- **Tool rejection** → `_record_function_rejection` writes a `function_rejection_item` and a synthetic `ToolCallOutputItem` so the model sees the rejection. `src/agents/run_internal/turn_resolution.py:886-916`, `src/agents/run_internal/items.py`.
- **Multiple handoffs in one response** → only the first is executed; the rest are recorded as `ToolCallOutputItem` with "Multiple handoffs detected, ignoring this one." `src/agents/run_internal/turn_resolution.py:426-438`.
- **Handoff input filter result is not a `HandoffInputData`** → `UserError` is raised and the runner exits. `src/agents/run_internal/turn_resolution.py:539-547`.
- **Server-managed conversation + handoff input filter** → `UserError` raised with a specific message. `src/agents/run_internal/turn_resolution.py:384-389`.
- **Input guardrail tripwire during streaming** → model task is cancelled; sentinel is queued; `InputGuardrailTripwireTriggered` is raised on stream access. `src/agents/run.py:1219-1245`, `src/agents/result.py:795-801,1197-1217`.
- **Output guardrail tripwire during streaming** → `OutputGuardrailTripwireTriggered` is raised. `src/agents/run_internal/run_loop.py:939-945,1368-1374`, `src/agents/exceptions.py:134-145`.
- **Shell tool without executor** → `ModelBehaviorError("Model produced local shell call without a local shell executor.")`. `src/agents/run_internal/turn_resolution.py:1643-1652`.
- **Invalid `tool_use_behavior`** → `UserError(f"Invalid tool_use_behavior: {agent.tool_use_behavior}")`. `src/agents/run_internal/turn_resolution.py:624-626`.
- **`run_sync` called from a running loop** → `RuntimeError("AgentRunner.run_sync() cannot be called when an event loop is already running.")`. `src/agents/run.py:1596-1598`.
- **Resume with mismatched state** (e.g., schema mismatch) → `_allow_legacy_name_agent_match` allows schema 1.6 and earlier to match approvals by agent name; 1.7+ requires object identity. `src/agents/run_internal/turn_resolution.py:1132-1154`.
- **Custom tool call without `input`** → `ModelBehaviorError("Custom tool call is missing input.")`. `src/agents/run_internal/turn_resolution.py:1062-1063`.
- **Computer action without `call_id`** → `ModelBehaviorError("Computer action is missing call_id.")`. `src/agents/run_internal/turn_resolution.py:967-969`.

## Future Considerations

- **Extract a single `apply_turn_result(turn_result, ctx) -> Action` transition function** that both the streaming and non-streaming loops call. Would remove the two ~30-line `isinstance` ladders and make the next-step contract enforced in one place. The discriminated union is already a perfect fit; the dispatch code just needs to be a function.
- **Mid-stream interrupt for human approval.** The "after turn" cancel is correct but slow when the user wants to inject an approval between LLM token chunks. A `streamed_result.inject_approval(approval_item)` API could push the sentinel and let the consumer receive both the streaming events and the approval simultaneously.
- **Promote `NextStep*` to an enum (or `Literal` type alias) with sealed payloads.** Today they are dataclasses; an enum with a `payload` field would let match-statements and `assert_never` enforce exhaustiveness at type-check time.
- **Move the streaming `is_complete` / `QueueCompleteSentinel` writes into a single `_finalize_streamed_*` helper** to reduce the surface for missed sentinels (see Tradeoffs).
- **Move the per-runtime executors to a single `ToolRuntime` protocol** (`FunctionToolRuntime`, `ShellToolRuntime`, etc.) so the per-tool fan-out in `_execute_tool_plan` (`src/agents/run_internal/tool_planning.py:572-619`) becomes a single `for runtime in plan.runtimes: await runtime.execute(...)` loop.
- **Defensive `tool_choice` reset inside `_execute_tool_plan`** would protect against a model that emits many tool calls with `tool_choice="required"` in a single response. Currently the reset only applies on the next turn.
- **Stronger typing for `SingleStepResult` consumers.** The `isinstance` chains are correct but lose exhaustiveness. A `@final` mixin class with a `next_step_variant: Literal["handoff", "final", "again", "interruption"]` would let a `match` statement work.
- **A `Runner.cancel()` API** that complements `streamed_result.cancel()` would unify the public surface.

## Questions / Gaps

- **How does the runner behave when `current_span` is `None` at the end of a turn?** The `try/finally` in `start_streaming` (`src/agents/run_internal/run_loop.py:1202-1239`) finishes the span only `if current_span:`. It is unclear whether a missing span is a real failure mode or just defensive. No evidence found in tests of a "span is None at finalize" case beyond `tests/test_tracing_errors_streamed.py`.
- **What is the contract for `AgentToolUseTracker` when an agent is re-bound (e.g., via handoff) under the same name?** `serialize_tool_use_tracker` uses `_build_agent_identity_keys_by_id` (`src/agents/run_internal/tool_use_tracker.py:119-136`); the lookup falls back to agent name when identity is not resolvable. `src/agents/run_internal/turn_resolution.py:1132-1154` documents that schema 1.7+ requires object identity. The interaction between hydration (`hydrate_tool_use_tracker`, `src/agents/run_internal/tool_use_tracker.py:139-156`) and identity-keyed snapshots during long-lived run state is not deeply traced in the read path.
- **What is the runtime's authority over `tool_choice = "<specific tool name>"`?** `maybe_reset_tool_choice` resets it once any tool is used (`src/agents/run_internal/run_loop.py:1346,1830`; tests `tests/test_tool_choice_reset.py:64-69`). But what about a single response with many tool calls and a named-tool `tool_choice`? The reset only fires on the next turn. **No evidence found** for the intra-response case.
- **How does the sandbox-prep path interact with control flow?** `SandboxRuntime.prepare_agent` (`src/agents/run_internal/agent_bindings.py`, applied in `src/agents/run.py:806-818` and `src/agents/run_internal/run_loop.py:714-728`) returns a binding that may rewrite the execution agent. If the rewrite fails, the runtime falls back; but the contract for "fallback" is `None` and re-binding, not "abort the run". The full sandbox control flow is documented elsewhere in the repo and not in this dimension's read scope.
- **What happens if `output_guardrails` is set to a callable that raises an exception other than `OutputGuardrailTripwireTriggered`?** `tests/test_output_guardrail_cancellation.py:1-...` covers cancellation; the test surface for non-tripwire exceptions is not deeply traced. **No evidence found** for the specific failure mode.
- **What is the contract for `Handoff.is_enabled=False` set *mid-run*?** The callable is invoked at handoff construction (`src/agents/handoffs/__init__.py:315-323`) and at handoff planning time. Whether it is re-evaluated at handoff *execution* time is not explicitly tested. **No clear evidence found.**

---

Generated by `01.02-control-flow-ownership` against `openai-agents-sdk`.
