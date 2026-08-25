# Source Analysis: openai-agents-sdk

## 03.09 Completion and Finalization Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+; asyncio; Pydantic v2; SDK built on top of the OpenAI Python SDK and `openai.types.responses` streaming primitives. |
| Analyzed | 2026-07-19 |

## Summary

The SDK encodes "done" as a small, explicit type ladder rather than a single boolean. The run loop produces one of four `NextStep*` variants from every turn (`src/agents/run_internal/run_steps.py:155-174`): `NextStepFinalOutput`, `NextStepHandoff`, `NextStepRunAgain`, `NextStepInterruption`. Only `NextStepFinalOutput` terminates the loop normally; the other three branch the loop into handoff, another turn, or a paused state that carries `ToolApprovalItem`s back to the caller.

Final outputs are typed against `Agent.output_type` (`src/agents/agent.py:332-339`) and validated through `AgentOutputSchema.validate_json()` (`src/agents/agent_output.py:136-164`). Structured outputs are validated through Pydantic's `TypeAdapter`; plain-text outputs are accepted as-is and stored in `RunResult.final_output: Any` (`src/agents/result.py:190`, `src/agents/result.py:463`).

The streaming runner mirrors the non-streaming runner but uses a `QueueCompleteSentinel` to signal end-of-stream (`src/agents/run_internal/run_steps.py:52-56`) plus an `is_complete: bool` flag on `RunResultStreaming` (`src/agents/result.py:470`). A terminal event from the Responses API (`response.completed`, `response.incomplete`, `response.failed`) is required before the runner will even attempt to resolve a turn (`src/agents/run_internal/run_loop.py:1488-1499`); `response.incomplete` / `response.failed` / `error` are translated into `ModelBehaviorError` via `_response_terminal.py:52-64`.

Incomplete runs are not silent. The runner raises typed exceptions (`MaxTurnsExceeded`, `InputGuardrailTripwireTriggered`, `OutputGuardrailTripwireTriggered`, `ModelRefusalError`, `ModelBehaviorError` — `src/agents/exceptions.py:56-174`) and callers can register `RunErrorHandlers` for `max_turns` and `model_refusal` to convert those exceptions into a graceful `RunResult` with a final output (`src/agents/run_error_handlers.py:50-55`, `src/agents/run_internal/error_handlers.py:128-166`). For sandbox rollouts, a `RolloutTerminalMetadata.terminal_state` enum (`src/agents/sandbox/memory/rollouts.py:59-67`) explicitly classifies a run as `completed`, `interrupted`, `cancelled`, `failed`, `max_turns_exceeded`, or `guardrail_tripped`.

Usage and cost are accumulated on `RunContextWrapper.usage` (`src/agents/usage.py:102-215`) as a `Usage` dataclass that tracks per-request entries in addition to aggregate counts. The `RequestUsage` list (`src/agents/usage.py:60-77`) preserves per-request input/output token detail (including cached and reasoning tokens) so cost computation can be reconstructed even after aggregation.

## Rating

**Score: 8 / 10**

Rationale:

- The four `NextStep*` variants in `src/agents/run_internal/run_steps.py:155-174` give "done" a single, narrow definition (`NextStepFinalOutput`); all other branches are explicit continuations or pauses.
- `NextStepInterruption` (`src/agents/run_internal/run_steps.py:169-174`) keeps a run alive when tool approvals are pending; `RunResult.interruptions: list[ToolApprovalItem]` (`src/agents/result.py:367-368`) signals this state to the caller and `result.final_output is None` discriminates interrupted runs from completed ones.
- Output is validated against `AgentOutputSchema.validate_json()` with a strict JSON schema path (`src/agents/agent_output.py:112-120`, `src/agents/agent_output.py:136-164`); invalid JSON from the model raises `ModelBehaviorError` rather than being silently accepted.
- `RunErrorHandlers` (`src/agents/run_error_handlers.py:50-55`) and `RunErrorHandlerResult` (`src/agents/run_error_handlers.py:35-40`) provide a typed extension hook for `max_turns` and `model_refusal`, and the handler's `final_output` is itself validated via `validate_handler_final_output` (`src/agents/run_internal/error_handlers.py:80-107`).
- Per-request `RequestUsage` entries are preserved through `Usage.add()` (`src/agents/usage.py:198-215`) so aggregated totals never lose per-request resolution.
- Streaming has an explicit `is_complete: bool` flag (`src/agents/result.py:470`) and a `run_loop_exception` accessor (`src/agents/result.py:623-646`) that surfaces silent run-loop failures after the stream has been consumed.
- A "completed" `ResponseCompletedEvent` is a hard prerequisite for turn resolution (`src/agents/run_internal/run_loop.py:1488-1506`); without it the runner raises `ModelBehaviorError("Model did not produce a final response!")` (`src/agents/run_internal/run_loop.py:1639`).
- `RolloutTerminalMetadata.terminal_state` (`src/agents/sandbox/memory/rollouts.py:59-67`) gives observers a single, typed terminal classification including `interrupted`, `cancelled`, and `guardrail_tripped`.

Where it falls short of 9-10:

- The decision to emit `NextStepFinalOutput` is **heuristic for plain-text output**: it fires whenever the model emits a `MessageOutputItem` after all tool calls have been resolved (or when `not output_schema` / `output_schema.is_plain_text()` — see `src/agents/run_internal/turn_resolution.py:823-835`). A model can therefore "falsely declare done" simply by sending a message at the end of a turn.
- There is no first-class `RunOutcome` aggregate (success / partial / failed / interrupted / cancelled); callers must combine `final_output`, `interruptions`, `output_guardrail_results`, and the raised exception themselves.
- `MaxTurnsExceeded` is a count fence, not a semantic gate — the SDK does not re-validate that the latest message looks like an answer when the limit trips; the only recovery path is the user-supplied `error_handlers["max_turns"]` callback (`src/agents/run.py:1074-1144`, `src/agents/run_internal/run_loop.py:881-956`).
- `finish_reason` from the chat-completions / LiteLLM / AnyLLM adapters is only logged (`src/agents/models/openai_chatcompletions.py:190-191`, `src/agents/extensions/models/litellm_model.py:255-256`, `src/agents/extensions/models/any_llm_model.py:506-507`); it is not propagated to `RunResult` or surfaced to callers.
- The Responses API's terminal-event contract (`response.incomplete`, `response.failed`) is the only finish-reason signal that influences control flow; everything else (`length`, `tool_calls`, `content_filter`) is collapsed into the same `response.completed` path.
- No dedicated "run summary" / "run report" type. Run metadata is fragmented across `RunResultBase` (`src/agents/result.py:175-330`), `RunResult.max_turns` (`src/agents/result.py:365-366`), `RunResult._current_turn` (`src/agents/result.py:347`), `RunResult.interruptions` (`src/agents/result.py:367-368`), `context_wrapper.usage`, and the raw response list.
- The run can complete with output guardrail warnings intact (guardrails that pass without tripping their tripwire do not block `NextStepFinalOutput`); only `tripwire_triggered` short-circuits the run (`src/agents/run_internal/guardrails.py:162-177`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Final output base class (non-streaming) | `RunResultBase.final_output: Any` field | `src/agents/result.py:190` |
| Final output (streaming) | `RunResultStreaming.final_output: Any`; initial value `None` until run completes | `src/agents/result.py:463-464`, `src/agents/run.py:1800` |
| Final output type declaration on agents | `output_type: type[Any] \| AgentOutputSchemaBase \| None` | `src/agents/agent.py:332-339` |
| NextStep discriminated union | `NextStepFinalOutput`, `NextStepHandoff`, `NextStepRunAgain`, `NextStepInterruption` | `src/agents/run_internal/run_steps.py:155-174` |
| Final output validation (JSON) | `AgentOutputSchema.validate_json` via `TypeAdapter`; strict-mode wrapping | `src/agents/agent_output.py:136-164` |
| Strict JSON schema enforcement | `ensure_strict_json_schema` invoked on construction | `src/agents/agent_output.py:112-120`, `src/agents/strict_schema.py` |
| Final-output decision logic for plain text | `if not output_schema or output_schema.is_plain_text(): return execute_final_output_call(...)` | `src/agents/run_internal/turn_resolution.py:823-835` |
| Final-output decision for structured | `output_schema.validate_json(potential_final_output_text)` | `src/agents/run_internal/turn_resolution.py:809-822` |
| Pending tool calls block completion | `ProcessedResponse.has_tools_or_approvals_to_run()` early-return for `NextStepInterruption` | `src/agents/run_internal/run_steps.py:132-147`, `src/agents/run_internal/turn_resolution.py:713-723` |
| Final-output dispatch from loop (non-streaming) | `NextStepFinalOutput` branch constructs `RunResult(final_output=turn_result.next_step.output, ...)` | `src/agents/run.py:1367-1414` |
| Final-output dispatch from loop (streaming) | `_finalize_streamed_final_output` sets `final_output`, `is_complete=True`, enqueues `QueueCompleteSentinel` | `src/agents/run_internal/run_loop.py:391-416`, `src/agents/run_internal/run_loop.py:1113-1125` |
| Streaming completion flag | `is_complete: bool = False` on `RunResultStreaming` | `src/agents/result.py:470` |
| Streaming sentinel | `QueueCompleteSentinel` + `QUEUE_COMPLETE_SENTINEL` singleton | `src/agents/run_internal/run_steps.py:52-56` |
| Responses stream terminal event handling | `response.completed` consumed as `ResponseCompletedEvent`; `response.incomplete` / `response.failed` / `error` raised as `ModelBehaviorError` | `src/agents/run_internal/run_loop.py:1488-1499` |
| Terminal failure formatting | `format_response_terminal_failure`, `response_terminal_failure_error` | `src/agents/models/_response_terminal.py:10-58` |
| Drain-stream-events attribute | `_mark_error_to_drain_stream_events` / `_should_drain_stream_events_before_raising` | `src/agents/exceptions.py:19-27` |
| No-terminal-response guard | `raise ModelBehaviorError("Model did not produce a final response!")` | `src/agents/run_internal/run_loop.py:1638-1639` |
| Output guardrails run on final output | `run_output_guardrails(agent.output_guardrails + run_config.output_guardrails, current_agent, output, context_wrapper)` | `src/agents/run.py:962-968`, `src/agents/run.py:1099-1104`, `src/agents/run_internal/run_loop.py:939-945` |
| Output guardrail tripwire blocks run | `if result.output.tripwire_triggered: raise OutputGuardrailTripwireTriggered(result)` | `src/agents/run_internal/guardrails.py:162-177` |
| Output guardrail exception class | `OutputGuardrailTripwireTriggered(AgentsException)` | `src/agents/exceptions.py:134-144` |
| Input guardrail exception class | `InputGuardrailTripwireTriggered(AgentsException)` | `src/agents/exceptions.py:121-131` |
| Input guardrails run on first turn only | `if current_turn == 0 and not resuming_turn` (non-streaming) / `current_turn == 0 and not is_resumed_state` (streaming) | `src/agents/run.py:770-774`, `src/agents/run_internal/run_loop.py:672-676` |
| Max-turns fence (non-streaming) | `if max_turns is not None and current_turn > max_turns:` → `MaxTurnsExceeded` | `src/agents/run.py:1058-1144` |
| Max-turns fence (streaming) | Same check inside `start_streaming`; honors `error_handlers["max_turns"]` | `src/agents/run_internal/run_loop.py:881-956` |
| MaxTurnsExceeded exception | Subclass of `AgentsException` with `run_data: RunErrorDetails \| None` | `src/agents/exceptions.py:46-63` |
| Run error handlers registry | `RunErrorHandlers` TypedDict: `max_turns`, `model_refusal` | `src/agents/run_error_handlers.py:50-55` |
| Run error handler return shape | `RunErrorHandlerResult(final_output: Any, include_in_history: bool = True)` | `src/agents/run_error_handlers.py:35-40` |
| Handler final-output validation | `validate_handler_final_output` → `output_schema.validate_json(payload)` | `src/agents/run_internal/error_handlers.py:80-107` |
| Handler invocation in non-streaming path | `resolve_run_error_handler_result` invoked before raising `MaxTurnsExceeded` | `src/agents/run.py:1074-1085` |
| Handler invocation in streaming path | Same call inside `start_streaming`; sends `message_output_created` event | `src/agents/run_internal/run_loop.py:902-934` |
| Refusal detection | `ItemHelpers.extract_refusal(message_items[-1].raw_item)` then `ModelRefusalError(refusal)` | `src/agents/run_internal/turn_resolution.py:763-789`, `src/agents/exceptions.py:78-86` |
| Usage accumulation (per turn) | `context_wrapper.usage.add(new_response.usage)` | `src/agents/run_internal/run_loop.py:1628`, `src/agents/run_internal/run_loop.py:1909` |
| Usage dataclass | `Usage(requests, input_tokens, output_tokens, total_tokens, input_tokens_details, output_tokens_details, request_usage_entries)` | `src/agents/usage.py:102-136` |
| Per-request usage preservation | `RequestUsage` dataclass; `Usage.add` extends `request_usage_entries` | `src/agents/usage.py:60-77`, `src/agents/usage.py:198-215` |
| Usage serialization for state | `serialize_usage` / `deserialize_usage` round-trip via `RequestUsage` | `src/agents/usage.py:13-57`, `src/agents/usage.py:227-273` |
| Usage attached to spans | `attach_usage_to_span`, `usage_delta`, `turn_usage_to_span_data`, `task_usage_to_span_data` | `src/agents/run_internal/agent_runner_helpers.py:69-150`, `src/agents/usage.py:295-310` |
| Streaming completion status accessor | `run_loop_exception` surfaces `task.exception()` after stream consumption | `src/agents/result.py:623-646` |
| Streaming `is_complete` set to True on completion | `_finalize_streamed_final_output` and `start_streaming` finally-block | `src/agents/run_internal/run_loop.py:412`, `src/agents/run_internal/run_loop.py:843`, `src/agents/run_internal/run_loop.py:948`, `src/agents/run_internal/run_loop.py:1100-1200` |
| Streaming cancel modes | `cancel(mode="immediate" \| "after_turn")` documented on `RunResultStreaming` | `src/agents/result.py:648-694` |
| Cancellation semantics in stream loop | `if streamed_result._cancel_mode == "after_turn": streamed_result.is_complete = True` | `src/agents/run_internal/run_loop.py:842-845`, `src/agents/run_internal/run_loop.py:1109-1112`, `src/agents/run_internal/run_loop.py:1161-1164` |
| RunResultBase public surface | `final_output`, `new_items`, `raw_responses`, `input/output/tool guardrail results`, `interruptions`, `last_agent`, `context_wrapper` | `src/agents/result.py:175-330` |
| Interruption result builder | `build_interruption_result` constructs `RunResult(final_output=None, interruptions=...)` | `src/agents/run_internal/agent_runner_helpers.py:373-424` |
| RunState.to_state() conversion | `RunResult.to_state()` packages result into a resumable `RunState` (including interruptions) | `src/agents/result.py:393-438`, `src/agents/run_state.py:323-330` |
| Sandbox rollout terminal classification | `RolloutTerminalMetadata.terminal_state` enum: `completed`, `interrupted`, `cancelled`, `failed`, `max_turns_exceeded`, `guardrail_tripped` | `src/agents/sandbox/memory/rollouts.py:59-67` |
| Terminal state derivation from exception | `terminal_metadata_for_exception` maps `MaxTurnsExceeded`, `*Guardrail*`, `CancelledError`, others | `src/agents/sandbox/memory/rollouts.py:175-196` |
| `final_output_as` typed accessor | `RunResultBase.final_output_as(cls, raise_if_incorrect_type=False)` | `src/agents/result.py:269-285` |
| Stream-event types | `RunItemStreamEvent` (typed `name`); `AgentUpdatedStreamEvent`; `RawResponsesStreamEvent` | `src/agents/stream_events.py:10-61` |
| Max-turns default | `DEFAULT_MAX_TURNS = 10` | `src/agents/run_config.py:33` |
| Test: max-turns exception (non-streamed) | `test_non_streamed_max_turns` | `tests/test_max_turns.py:30-51` |
| Test: max-turns exception (streamed) | `test_streamed_max_turns` | `tests/test_max_turns.py:81-118` |
| Test: max-turns handler returns structured output | `test_structured_output_max_turns_handler_pydantic_output`, `test_structured_output_max_turns_handler_list_output` | `tests/test_max_turns.py:303-340` |
| Test: refusal handler returns structured output | `test_non_streamed_refusal_handler_returns_structured_output` | `tests/test_max_turns.py:169-192` |
| Test: streamed refusal handler returns output | `test_streamed_refusal_handler_returns_output` | `tests/test_max_turns.py:214-234` |
| Test: streamed runner backfills empty terminal output | `test_streamed_runner_backfills_empty_terminal_output_before_step_resolution` | `tests/test_streamed_terminal_output_backfill.py:111-142` |
| Test: handler final_output skipped from history | `test_non_streamed_max_turns_handler_skip_history` (asserts `result.new_items == []`) | `tests/test_max_turns.py:364-382` |
| Test: interruption resume cycle | `test_hitl_session_scenario` (asserts `first_run.interruptions == 1`, `resumed.interruptions == []`) | `tests/test_hitl_session_scenario.py:296-306` |
| Test: streaming final output with `is_complete` | Multiple asserts across `tests/test_agent_runner_streamed.py` (e.g., `result.final_output is None`, `result.is_complete is True`) | `tests/test_agent_runner_streamed.py:1454-1480` |

## Answers to Dimension Questions

1. **What counts as done?**
   The loop terminates when `execute_tools_and_side_effects` (`src/agents/run_internal/turn_resolution.py:629-845`) returns a `SingleStepResult.next_step` of type `NextStepFinalOutput`. That decision has three triggers, all in `turn_resolution.py`:
   - `check_for_final_output_from_tools` returns `is_final_output=True` and `tool_use_behavior` is not `"run_llm_again"` (`src/agents/run_internal/turn_resolution.py:594-626`, `src/agents/run_internal/turn_resolution.py:748-761`).
   - The model emits a refusal and an `error_handlers["model_refusal"]` callback returns a final output (`src/agents/run_internal/turn_resolution.py:769-808`).
   - The model emits a `MessageOutputItem` after all tool calls/approvals have been resolved and `output_schema` matches (or is plain text) (`src/agents/run_internal/turn_resolution.py:809-835`).
   In every case the result is wrapped via `execute_final_output` (`src/agents/run_internal/turn_resolution.py:305-335`) which fires `on_agent_end` lifecycle hooks before returning.

2. **Can the model falsely declare done?**
   Yes, in the plain-text path. `execute_tools_and_side_effects` returns `NextStepFinalOutput` whenever `MessageOutputItem` is the last remaining item and there are no other tools or approvals to run (`src/agents/run_internal/turn_resolution.py:823-835`). Nothing in the SDK verifies that the text constitutes a meaningful answer; it only checks that the message exists. For structured output, the SDK does validate that the JSON parses against `AgentOutputSchema` via `output_schema.validate_json(potential_final_output_text)` (`src/agents/run_internal/turn_resolution.py:809-822`), but a model can still emit schema-valid but semantically-empty output.

3. **Are final outputs validated?**
   - For structured outputs (`Agent.output_type` set, not plain text): validated through Pydantic `TypeAdapter` (`src/agents/agent_output.py:136-164`); raises `ModelBehaviorError` on invalid JSON or missing wrapper key (`src/agents/agent_output.py:149-162`).
   - For strict JSON schema mode: schemas are rewritten to a strict subset at `AgentOutputSchema` construction (`src/agents/agent_output.py:112-120`, `src/agents/strict_schema.py`).
   - For output-guardrail tripwires: `run_output_guardrails` raises `OutputGuardrailTripwireTriggered` on tripwire, blocking the run (`src/agents/run_internal/guardrails.py:162-177`).
   - For handler-provided final outputs: `validate_handler_final_output` re-runs validation (`src/agents/run_internal/error_handlers.py:80-107`).
   - For plain-text outputs: no schema validation, only `format_final_output_text` stringifies (`src/agents/run_internal/error_handlers.py:57-77`).

4. **Is usage/cost finalized?**
   Yes, on the result object via `RunResultBase.context_wrapper.usage` (`src/agents/result.py:205-206`), which holds a `Usage` dataclass (`src/agents/usage.py:102-215`). Usage is accumulated per turn by `context_wrapper.usage.add(new_response.usage)` (`src/agents/run_internal/run_loop.py:1628`, `src/agents/run_internal/run_loop.py:1909`), and per-request breakdowns are preserved in `request_usage_entries` (`src/agents/usage.py:198-215`). For tracing, deltas are computed via `snapshot_usage` + `usage_delta` (`src/agents/run_internal/agent_runner_helpers.py:69-112`) and attached to `turn_span` / `task_span` (`src/agents/run.py:1274-1278`, `src/agents/run.py:1558-1562`, `src/agents/run_internal/run_loop.py:1041-1045`, `src/agents/run_internal/run_loop.py:1229-1233`). Per-response usage is also retained in `RunResult.raw_responses[i].usage` (`src/agents/items.py:657-680`, `src/agents/result.py:187-188`).

5. **Can a run complete with warnings?**
   Yes, partial / warning cases are observable but not blocked:
   - Output guardrails that do not trip their tripwire produce `output_guardrail_results` on the result (`src/agents/result.py:196-197`) but do not prevent `NextStepFinalOutput`.
   - An interruption run returns `RunResult(final_output=None, interruptions=[...])` (`src/agents/run_internal/agent_runner_helpers.py:398-416`), clearly distinct from a completed run because `final_output` is `None` and `interruptions` is non-empty.
   - For sandbox rollouts, `RolloutTerminalMetadata` records `terminal_state="interrupted"` separately from `"completed"` (`src/agents/sandbox/memory/rollouts.py:158-161`).
   - The streaming runner's `is_complete=True` (`src/agents/result.py:470`) is set even when `_stored_exception` carries a later exception (`src/agents/run_internal/run_loop.py:1175-1199`), so callers must also check `run_loop_exception` (`src/agents/result.py:623-646`).

## Architectural Decisions

- **`NextStep*` enum-based next-step dispatch.** Every turn returns one of four tagged union values (`src/agents/run_internal/run_steps.py:155-174`), and the runner uses `isinstance` to branch (`src/agents/run.py:961-1497`). This makes "done" a single, narrow path (`NextStepFinalOutput`) and forces all other exits to be explicit.
- **`NextStepInterruption` instead of partial output.** Pending tool approvals don't pollute `final_output`; instead they flow through `interruptions` and the `RunResult.final_output is None` check discriminates "paused" from "completed" (`src/agents/run_internal/agent_runner_helpers.py:398-416`).
- **`AgentOutputSchema` validates via Pydantic, not custom parsing.** Output type checking leans on `TypeAdapter` and a strict-schema rewriter (`src/agents/agent_output.py:79-120`). Errors flow through `_error_tracing.attach_error_to_current_span` before raising `ModelBehaviorError` (`src/agents/agent_output.py:142-163`).
- **`RunErrorHandlers` are typed and validated.** Two named handlers (`max_turns`, `model_refusal`) returning `RunErrorHandlerResult`; the SDK validates their final outputs through `validate_handler_final_output` (`src/agents/run_error_handlers.py:50-55`, `src/agents/run_internal/error_handlers.py:80-107`).
- **Streaming mirrors non-streaming but uses a queue sentinel.** `_finalize_streamed_final_output` (`src/agents/run_internal/run_loop.py:391-416`) and `_finalize_streamed_interruption` (`src/agents/run_internal/run_loop.py:419-434`) share the `QueueCompleteSentinel` (`src/agents/run_internal/run_steps.py:52-56`) to signal end-of-stream, and `is_complete` is set on every terminal branch (`src/agents/run_internal/run_loop.py:412`, `src/agents/run_internal/run_loop.py:843`, `src/agents/run_internal/run_loop.py:948`, `src/agents/run_internal/run_loop.py:1100-1200`).
- **`Usage` keeps per-request entries.** `request_usage_entries: list[RequestUsage]` (`src/agents/usage.py:125-136`) lets downstream cost engines reconstruct per-call cost even after totals are summed, which `Usage.add` extends automatically (`src/agents/usage.py:198-215`).
- **`RunState.to_state()` makes a paused run resumable.** `final_output=None` + `interruptions=[...]` is a first-class, round-trippable state (`src/agents/result.py:393-438`, `src/agents/run_state.py:323-330`), and `RunResultStreaming.to_state()` mirrors this for streaming runs (`src/agents/result.py:888-936`).
- **`RolloutTerminalMetadata` adds a typed terminal classification** for sandbox-aware deployments (`src/agents/sandbox/memory/rollouts.py:59-67`), mapping exceptions and outcomes to one of six terminal states.

## Notable Patterns

- **End-hook firing before final-output result construction.** `run_final_output_hooks` (`src/agents/run_internal/turn_resolution.py:284-302`) is invoked from `execute_final_output_step` (`src/agents/run_internal/turn_resolution.py:305-335`) so that `on_agent_end` fires whether the run completes via tools, refusal handler, plain text, or structured output.
- **Backfill of empty terminal-response output.** If the Responses API emits output items via `ResponseOutputItemDoneEvent` but the final `ResponseCompletedEvent.response.output` is empty, the streaming runner preserves the streamed output before turn resolution (`src/agents/run_internal/run_loop.py:1501-1506`); this is covered by `tests/test_streamed_terminal_output_backfill.py`.
- **Late-surfacing run-loop exceptions.** `_await_task_safely` swallows exceptions to a sink and a subsequent `_check_errors()` pass in `stream_events` re-raises through `_stored_exception` (`src/agents/result.py:705-779`, `src/agents/result.py:852-866`), so silent run-loop failures don't get lost.
- **Cancellation modes propagated through `is_complete`.** `cancel(mode="immediate")` immediately flips `is_complete` and drains queues (`src/agents/result.py:677-689`); `cancel(mode="after_turn")` is checked inside the loop to break gracefully after the current turn (`src/agents/run_internal/run_loop.py:842-845`, `src/agents/run_internal/run_loop.py:1109-1112`, `src/agents/run_internal/run_loop.py:1161-1164`).
- **Per-call `finish_reason` is intentionally dropped.** Chat-completions / LiteLLM / AnyLLM adapters log `finish_reason` only for debugging (`src/agents/models/openai_chatcompletions.py:190-191`, `src/agents/extensions/models/litellm_model.py:255-256`, `src/agents/extensions/models/any_llm_model.py:506-507`); the runner treats every `response.completed` as terminal without further distinguishing the reason.

## Tradeoffs

- **Heuristic vs. explicit final-answer protocol.** Plain-text completion relies on "no more tool calls + a message exists" (`src/agents/run_internal/turn_resolution.py:823-835`). This keeps the wire format simple but accepts any message text as the final answer. Structured outputs add a JSON-schema check, but a schema-valid empty/garbage payload still passes.
- **No first-class `RunOutcome` aggregate.** Callers wanting "did this complete successfully?" must inspect `final_output`, `interruptions`, `output_guardrail_results`, the raised exception, and (for streaming) `run_loop_exception`. The `RolloutTerminalMetadata` taxonomy exists but only inside `sandbox/memory` (`src/agents/sandbox/memory/rollouts.py:59-67`).
- **Output guardrail warnings ≠ blocking.** `output_guardrail_results` records both tripped and un-tripped guardrails (`src/agents/result.py:196-197`), but only `tripwire_triggered` blocks (`src/agents/run_internal/guardrails.py:162-177`); there is no "completed with warnings" state on `RunResult`.
- **`MAX_TURNS` is a count, not a semantic fence.** `MAX_TURNS = 10` (`src/agents/run_config.py:33`) defaults are conservative but offer no per-agent configuration. Recovery is via a user-supplied handler (`src/agents/run.py:1074-1144`, `src/agents/run_internal/run_loop.py:881-956`).
- **Streaming `is_complete` can be `True` even when an exception is pending.** `stream_events` raises `_stored_exception` after the iterator exits (`src/agents/result.py:778-779`), but callers that read `is_complete` without consuming events may miss the failure (`src/agents/result.py:623-646` documents this).
- **Two parallel "final-output" constructors.** Non-streaming builds `RunResult(final_output=..., ...)` inline at multiple call sites (`src/agents/run.py:971-985`, `src/agents/run.py:1107-1121`, `src/agents/run.py:1380-1394`), while streaming delegates to `_finalize_streamed_final_output` (`src/agents/run_internal/run_loop.py:391-416`). Behavior is parallel but maintenance burden is duplicated.

## Failure Modes / Edge Cases

- **No `ResponseCompletedEvent` before turn resolution.** Raises `ModelBehaviorError("Model did not produce a final response!")` (`src/agents/run_internal/run_loop.py:1638-1639`).
- **`response.incomplete` / `response.failed`.** Converted to `ModelBehaviorError` with `_agents_drain_queued_stream_events=True` so queued stream events flush before the exception propagates (`src/agents/run_internal/run_loop.py:1491-1499`, `src/agents/exceptions.py:19-27`).
- **`response.error` / `error`.** Same path: `response_error_event_failure_error` (`src/agents/models/_response_terminal.py:34-64`).
- **Model refusal.** `ModelRefusalError` raised if no `error_handlers["model_refusal"]` is registered; otherwise handler output is validated as the final output (`src/agents/run_internal/turn_resolution.py:769-808`, `src/agents/exceptions.py:78-86`).
- **Output schema mismatch (structured output).** `ModelBehaviorError` raised by `output_schema.validate_json` (`src/agents/agent_output.py:142-162`).
- **Output guardrail tripwire.** Raises `OutputGuardrailTripwireTriggered` and aborts the run (`src/agents/run_internal/guardrails.py:162-177`).
- **Input guardrail tripwire (non-streaming).** `InputGuardrailTripwireTriggered` raised; in streaming, the same exception is raised after the queued events drain (`src/agents/run.py:1181-1245`, `src/agents/run_internal/run_loop.py:958-983`).
- **`max_turns` exceeded.** `MaxTurnsExceeded` raised unless `error_handlers["max_turns"]` returns a `RunErrorHandlerResult`; handler output goes through `validate_handler_final_output` (`src/agents/run.py:1058-1144`, `src/agents/run_internal/run_loop.py:881-956`, `src/agents/run_internal/error_handlers.py:80-107`).
- **Pending tool approvals.** `NextStepInterruption` produced; `RunResult.final_output=None`, `interruptions=[...]`, `result.to_state()` returns a resumable `RunState` (`src/agents/run_internal/turn_resolution.py:713-723`, `src/agents/run_internal/agent_runner_helpers.py:373-424`, `src/agents/result.py:393-438`).
- **Cancellation mid-run.** `cancel(mode="immediate")` cancels tasks and drains queues (`src/agents/result.py:677-689`); `cancel(mode="after_turn")` waits for the current turn to finish then breaks the loop (`src/agents/run_internal/run_loop.py:842-845`).
- **Stream loop silent failure.** `run_loop_exception` exposes exceptions that occurred before they could be surfaced through the event queue (`src/agents/result.py:623-646`, `src/agents/run_internal/run_loop.py:1165-1199`).
- **Handler returns invalid structured output.** `UserError("Invalid run error handler final_output for structured output.")` (`src/agents/run_internal/error_handlers.py:100-107`).
- **Handler returns dict with extra keys.** `UserError("Invalid run error handler result.")` (`src/agents/run_internal/error_handlers.py:159-164`).
- **Strict JSON schema on unsupported type.** `UserError("Strict JSON schema is enabled, but the output type is not valid...")` at `AgentOutputSchema` construction (`src/agents/agent_output.py:115-120`).
- **Unknown `tool_use_behavior`.** `UserError(f"Invalid tool_use_behavior: {agent.tool_use_behavior}")` (`src/agents/run_internal/turn_resolution.py:625-626`).
- **Sandbox session enqueue failure.** Logged as warning, run still completes (`src/agents/run.py:1540-1541`).

## Future Considerations

- **`RunOutcome` aggregate.** Add a typed `RunOutcome` (success / completed-with-warnings / interrupted / cancelled / failed) on `RunResult` to replace the current four-field inspection pattern. `RolloutTerminalMetadata.terminal_state` (`src/agents/sandbox/memory/rollouts.py:59-67`) is a candidate vocabulary to lift into the public surface.
- **Finish-reason propagation.** `finish_reason` is currently logged and dropped (`src/agents/models/openai_chatcompletions.py:190-191`, `src/agents/extensions/models/litellm_model.py:255-256`, `src/agents/extensions/models/any_llm_model.py:506-507`). Surfacing it on `ModelResponse` (alongside `usage` in `src/agents/items.py:657-680`) would let callers distinguish `length` / `content_filter` / `tool_calls` from a clean `stop`.
- **Final-answer protocol for plain text.** The heuristic in `src/agents/run_internal/turn_resolution.py:823-835` would benefit from a first-class "this is my final answer" sentinel (e.g., a structured `final_output_message` item type or a required marker) to prevent false-positive completion.
- **Aggregated cost summary object.** `Usage` (`src/agents/usage.py:102-215`) is excellent for inputs but lacks a derived cost estimate. A `Usage.summarize(pricing=...)` helper would close the loop without requiring every caller to re-implement cost math.
- **Consolidate `RunResult` final-output constructors.** The four call sites that build `RunResult(final_output=..., ...)` in `src/agents/run.py:971-985`, `src/agents/run.py:1107-1121`, `src/agents/run.py:1380-1394`, and `src/agents/run_internal/agent_runner_helpers.py:398-416` could share a single `_build_final_result(...)` helper to remove drift risk between branches.
- **Output-guardrail "completed with warnings" state.** Allow non-tripwire guardrail outcomes to flip a `RunResult.warnings` bit without blocking completion, giving callers a typed signal.
- **Streaming `is_complete` semantics.** When `_stored_exception` is set, expose `is_complete_successfully: bool` alongside `is_complete` (`src/agents/result.py:470`, `src/agents/result.py:495`) to prevent callers from reading `is_complete=True` and missing a pending exception.

## Questions / Gaps

- Is there a defined protocol for the model to *explicitly* declare "I am done" (e.g., a special tool call, a structured message)? Searched `src/agents/` for any such protocol and found none — completion is purely derived from message + tool-call inspection.
- Does `AgentOutputSchema.validate_json` reject empty strings for structured outputs? `ItemHelpers.extract_text` (`src/agents/items.py:717-735`) returns `None` on empty, and `turn_resolution.py:809-810` short-circuits when `potential_final_output_text` is falsy, so an empty structured payload silently routes to the plain-text fallback. No clear evidence found that this is intentional vs. a leaky default.
- How are partial / streaming-only failures (e.g., the run-loop silently fails before `_stored_exception` is set) surfaced to non-streaming callers? Searched `src/agents/run.py` and found only the streaming-side `run_loop_exception` (`src/agents/result.py:623-646`); non-streaming callers have to rely on exceptions propagating from `run_single_turn` (`src/agents/run_internal/run_loop.py:1708-1795`) and the turn's exception handler in `AgentRunner.run` (`src/agents/run.py:1505-1517`).
- Is there any documented contract for what `final_output` must look like for `output_type=None`? Searched docs (`docs/running_agents.md`, `docs/results.md`) — only references to `result.final_output` as "the output of the last agent" (`src/agents/result.py:190-191`); no formal contract.
- Is the chat-completions / LiteLLM / AnyLLM `finish_reason` ever surfaced to the user, or is the runtime truly indifferent to it? Evidence (`src/agents/models/openai_chatcompletions.py:190-191`, `src/agents/extensions/models/litellm_model.py:255-256`, `src/agents/extensions/models/any_llm_model.py:506-507`) shows only `logger.debug` calls; no evidence found of propagation beyond logging.
- How are context-override `agent.context` plus handler-provided `final_output` reconciled when handlers run? `validate_handler_final_output` (`src/agents/run_internal/error_handlers.py:80-107`) validates against `current_agent.output_type`, but I found no explicit check that the handler's `RunErrorData.last_agent` matches `current_agent`. No clear evidence found.

---

Generated by `dimensions/03.09-completion-and-finalization-semantics.md` against `openai-agents-sdk`.