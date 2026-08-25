# Source Analysis: openai-agents-sdk

## 03.01 LLM Turn Loop Structure

### Source Info

| Field | Value |
|-------|-------|
| Name | `openai-agents-sdk` |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (3.10+), asyncio, OpenAI Responses API, Pydantic |
| Analyzed | 2026-07-13 |

## Summary

The OpenAI Agents SDK ships **two parallel LLM turn loops** that must be kept behaviorally identical: a non-streaming `while True:` loop owned by `AgentRunner.run` in `src/agents/run.py:768-1504`, and a streaming equivalent in `start_streaming` at `src/agents/run_internal/run_loop.py:671-1199`. Both loops follow the documented contract: call the LLM, classify the response as a final output / handoff / interruption / tool-call batch, route the next step, persist per-turn artifacts, and repeat until the model produces a final output or `max_turns` is exceeded (default `DEFAULT_MAX_TURNS = 10` at `src/agents/run_config.py:33`).

The unit of work for each iteration is `SingleStepResult` (`src/agents/run_internal/run_steps.py:178-218`) — a dataclass that carries the model response, pre/post-step items, and a discriminated `NextStep*` union (`NextStepFinalOutput`, `NextStepHandoff`, `NextStepRunAgain`, `NextStepInterruption` at `run_steps.py:154-173`). The actual model invocation is wrapped in `run_single_turn` (`src/agents/run_internal/run_loop.py:1708-1795`) for the sync path and `run_single_turn_streamed` (`run_loop.py:1242-1705`) for the streaming path. Both delegate to `get_new_response` (`run_loop.py:1798-1920`) / the inline stream call, which call into the retry-wrapped `Model` interface (`get_response_with_retry` at `src/agents/run_internal/model_retry.py:511-607`, `stream_response_with_retry` at `model_retry.py:610-724`).

Turn state is persisted in three layers: (1) per-loop mutable state (`current_turn`, `generated_items`, `model_responses`, `session_items`) threaded through the closure; (2) durable `RunState` snapshots for HITL pause/resume (`src/agents/run_state.py:185-322`, schema `CURRENT_SCHEMA_VERSION = "1.11"` at `run_state.py:131`); and (3) optional session persistence via the `Session` protocol (`save_result_to_session` invoked at `run.py:1352-1360`, `run_loop.py:349-356`). Each turn emits a `turn_span` trace (`turn_span(...)` at `src/agents/run.py:1166-1170`, `src/agents/run_internal/run_loop.py:1003-1007`, factory at `src/agents/tracing/create.py:139-152`).

The loop is fully **generic across agents**: the same `while True` runs for the starting agent and any agent it hands off to. Handoff re-points `current_agent` (`run.py:1473-1475`, `run_loop.py:1097-1099`), resets `should_run_agent_start_hooks = True` (`run.py:1482`, `run_loop.py:1102`), and continues with the new agent's tools, instructions, and output schema. The same loop also runs from a `RunState` snapshot (resumption) without incrementing the turn counter (`run.py:769-774`, `run_loop.py:672-676` — first-turn guardrails are skipped on resumption per AGENTS.md policy).

## Rating

**9 / 10** — Mature, durable, observable, extensible. The "one turn = one model call (plus tool executions)" contract is documented in the `Runner.run` docstring (`src/agents/run.py:215-229`), implemented as a flat `while True:` with explicit `NextStep*` routing (`run.py:1366-1497`, `run_loop.py:1065-1150`), and grounded in two layers of persistence (`RunState` schema-versioned at `1.11`, plus `Session` adapter). All five mechanisms the dimension asks about (runner loop, turn records, model-call wrapper, message assembly, final-output condition) are first-class and live in well-named symbols. Tests cover max-turns in both streaming and non-streaming paths (`tests/test_max_turns.py:31-77,82-146`), LLM start/end hooks (`tests/test_run_hooks.py:89-253`), tool calls + handoffs as multi-turn sequences (`tests/test_agent_runner.py:589-617,1006-1042`), and full durable `RunState` round-trips (`tests/test_run_state.py:1758-2380`). Deductions: the streaming and non-streaming loops are **two separate implementations** that must be kept in sync (AGENTS.md explicit policy at line "Keep streaming and non-streaming paths behaviorally aligned"), the loop bodies are non-trivially long (`run.py:768-1504` ~737 lines, `run_loop.py:671-1199` ~528 lines), and there are several near-duplicate code paths (e.g. `run_single_turn` vs. `run_single_turn_streamed`, the parallel model+guardrail fan-out only in `run.py:1219-1247`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public Runner loop, async entry | `Runner.run` docstring states "The loop runs like so: 1. invoke LLM … 2. final output ⇒ end, 3. handoff ⇒ re-run, 4. tool calls ⇒ re-run" | `src/agents/run.py:215-229` |
| Public Runner loop, sync entry | `Runner.run_sync` wraps `.run()` on a kept-alive default event loop | `src/agents/run.py:1564-1645` |
| Public Runner loop, streamed entry | `Runner.run_streamed` kicks off `start_streaming` as a background task and returns a `RunResultStreaming` | `src/agents/run.py:364-441`, `:1647-1876` |
| AgentRunner.run outer driver loop | `while True:` over `current_turn`, with resume skip on the first turn | `src/agents/run.py:768` |
| Inner `run_single_turn` (sync) | One model call + tool/handoff/final-output resolution | `src/agents/run_internal/run_loop.py:1708-1795` |
| Inner `run_single_turn_streamed` | Stream variant; deduplicates items already emitted as raw events | `src/agents/run_internal/run_loop.py:1242-1705` |
| Streaming outer loop | `start_streaming` `while True:` with first-turn-only guardrails | `src/agents/run_internal/run_loop.py:671` |
| Turn counter (sync) | `current_turn += 1` and `MaxTurnsExceeded` enforcement | `src/agents/run.py:1057-1144` |
| Turn counter (streamed) | `current_turn += 1` and `MaxTurnsExceeded` enforcement | `src/agents/run_internal/run_loop.py:875-956` |
| Default max turns | `DEFAULT_MAX_TURNS = 10` | `src/agents/run_config.py:33` |
| Turn span (sync) | `turn_span(turn=current_turn, agent_name=current_agent.name).start(mark_as_current=True)` | `src/agents/run.py:1166-1170` |
| Turn span (streamed) | Mirror span in streaming loop | `src/agents/run_internal/run_loop.py:1003-1007` |
| `turn_span` factory | "This represents one agent loop turn" | `src/agents/tracing/create.py:139-152` |
| Per-turn usage snapshot | `turn_usage_start = snapshot_usage(context_wrapper.usage)` | `src/agents/run.py:1165`, `src/agents/run_internal/run_loop.py:1002` |
| Per-turn usage delta | `attach_usage_to_span(span, usage_delta(turn_usage_start, usage))` | `src/agents/run.py:1273-1278`, `src/agents/run_internal/run_loop.py:1040-1045` |
| First-turn guardrails (sync) | Sequential then parallel guardrails, only on `current_turn <= 1` and starting agent only | `src/agents/run.py:1172-1247` |
| First-turn guardrails (streamed) | Same restriction; sequential split, then streaming variant | `src/agents/run_internal/run_loop.py:958-995` |
| Final-output condition type | `NextStepFinalOutput(output)` | `src/agents/run_internal/run_steps.py:159-162` |
| Run-again condition type | `NextStepRunAgain()` | `src/agents/run_internal/run_steps.py:164-166` |
| Handoff condition type | `NextStepHandoff(new_agent)` | `src/agents/run_internal/run_steps.py:154-157` |
| Interruption condition type | `NextStepInterruption(interruptions=list[ToolApprovalItem])` | `src/agents/run_internal/run_steps.py:169-173` |
| Sync final-output branch | Construct `RunResult`, run output guardrails, save session, return | `src/agents/run.py:1366-1414` |
| Streamed final-output branch | Set `is_complete`, push `QueueCompleteSentinel`, break | `src/agents/run_internal/run_loop.py:1113-1125` |
| Sync handoff branch | Switch agent, reset hooks, continue | `src/agents/run.py:1472-1493` |
| Streamed handoff branch | Switch agent, reset hooks, push `AgentUpdatedStreamEvent`, continue | `src/agents/run_internal/run_loop.py:1091-1112` |
| Sync run-again branch | Save session, continue | `src/agents/run.py:1483-1493` |
| Streamed run-again branch | Save session, optionally break on `cancel_mode == "after_turn"` | `src/agents/run_internal/run_loop.py:1151-1164` |
| Interruption branch (sync) | Persist, set `_current_step`, return interruption result | `src/agents/run.py:1415-1471` |
| Interruption branch (streamed) | Persist, finalize, push `QueueCompleteSentinel`, break | `src/agents/run_internal/run_loop.py:1126-1150` |
| Model call wrapper (sync) | `get_new_response(...)` builds `ModelInputData`, calls `model.get_response(...)` via retry wrapper | `src/agents/run_internal/run_loop.py:1798-1920` |
| Model call wrapper (stream) | `retry_stream = stream_response_with_retry(get_stream=lambda: model.stream_response(...))` | `src/agents/run_internal/run_loop.py:1460-1481` |
| Model interface (sync) | `Model.get_response(...)` abstract method | `src/agents/models/interface.py:56-89` |
| Model interface (stream) | `Model.stream_response(...) -> AsyncIterator[TResponseStreamEvent]` | `src/agents/models/interface.py:91-124` |
| Retry wrapper (sync) | `get_response_with_retry` — handles provider retries, conversation-locked compatibility retries, and policy retries | `src/agents/run_internal/model_retry.py:511-607` |
| Retry wrapper (stream) | `stream_response_with_retry` — same retry contract but tracks `failed_retry_attempts_out` | `src/agents/run_internal/model_retry.py:610-724` |
| Message assembly | `system_prompt, prompt_config = await asyncio.gather(agent.get_system_prompt(...), agent.get_prompt(...))` | `src/agents/run_internal/run_loop.py:1338-1341`, `:1751-1754` |
| Input assembly (sync) | `_prepare_turn_input_items(caller_input, generated_items, reasoning_item_id_policy)` | `src/agents/run_internal/run_loop.py:280-288`, called from `:1761` |
| Input assembly (stream) | Same helper called from streaming path | `src/agents/run_internal/run_loop.py:1364-1368` |
| Optional `call_model_input_filter` | `maybe_filter_model_input(...)` runs just before model call | `src/agents/run_internal/turn_preparation.py:51-85`, called from `run_loop.py:1370-1376`, `:1818-1824` |
| Server-managed conversation input | `OpenAIServerConversationTracker.prepare_input(...)` returns only the delta items | `src/agents/run_internal/oai_conversation.py` (used in `run_loop.py:1351-1355`) |
| Response processing | `process_model_response(...)` produces `ProcessedResponse` with `functions`, `computer_actions`, `custom_tool_calls`, `local_shell_calls`, `shell_calls`, `apply_patch_calls`, `handoffs`, `mcp_approval_requests`, `tools_used` | `src/agents/run_internal/turn_resolution.py:1551-2002` |
| Tool execution | `execute_tools_and_side_effects(...)` runs `_execute_tool_plan(...)`, applies `interruptions`, dispatches to handoff or final-output branches | `src/agents/run_internal/turn_resolution.py:629-845` |
| Tool plan construction | `_build_plan_for_fresh_turn(...)` and `_build_plan_for_resume_turn(...)` separate planning from execution | `src/agents/run_internal/tool_planning.py` |
| Multiple tool calls per turn | `ProcessedResponse` collects all tool types; `has_tools_or_approvals_to_run()` returns True if any are pending | `src/agents/run_internal/run_steps.py:115-147` |
| Parallel local tool execution | `tool_execution.max_function_tool_concurrency` capped via `_execute_tool_plan` | `src/agents/run_internal/tool_execution.py` (called from `turn_resolution.py:673-679`) |
| SingleStepResult shape | Dataclass with `original_input`, `model_response`, `pre_step_items`, `new_step_items`, `next_step`, guardrail results, `processed_response` | `src/agents/run_internal/run_steps.py:178-218` |
| Per-turn persistence (sync) | `await save_result_to_session(session, [], items_to_save_turn, run_state, ...)` after most step types | `src/agents/run.py:1352-1360` |
| Per-turn persistence (stream) | `_save_stream_items_with_count` helper | `src/agents/run_internal/run_loop.py:332-360` |
| Per-turn persisted count | `_current_turn_persisted_item_count` reset at start of each turn | `src/agents/run.py:1146-1147`, `src/agents/run_internal/run_loop.py:877-879` |
| Resume pause/resume state | `RunState` with `_current_turn`, `_model_responses`, `_generated_items`, `_session_items`, `_last_processed_response`, `_current_step`, `_tool_use_tracker_snapshot` | `src/agents/run_state.py:185-322` |
| Schema versioning | `CURRENT_SCHEMA_VERSION = "1.11"` with summary table | `src/agents/run_state.py:131-149` |
| Resume path | `resolve_interrupted_turn(...)` rebuilds tool runs from pending approvals | `src/agents/run_internal/turn_resolution.py:848-2057` |
| Resume from `RunState` (sync) | `is_resumed_state` branch consumes `_current_step`, skips guardrails, does not increment turn | `src/agents/run.py:769-1025` |
| Resume from `RunState` (stream) | Same shape | `src/agents/run_internal/run_loop.py:730-840` |
| LLM start hook (sync) | `await asyncio.gather(hooks.on_llm_start(...), public_agent.hooks.on_llm_start(...))` | `src/agents/run_internal/run_loop.py:1835-1847` |
| LLM end hook (sync) | `await asyncio.gather(hooks.on_llm_end(...), public_agent.hooks.on_llm_end(...))` | `src/agents/run_internal/run_loop.py:1911-1918` |
| LLM start hook (stream) | Mirror | `src/agents/run_internal/run_loop.py:1394-1406` |
| LLM end hook (stream) | Mirror after the terminal `ResponseCompletedEvent` | `src/agents/run_internal/run_loop.py:1629-1636` |
| on_agent_start hooks | Re-fired per handoff via `should_run_agent_start_hooks = True` reset | `src/agents/run.py:1482`, `src/agents/run_internal/run_loop.py:1317-1331` |
| Streaming item emission | `RawResponsesStreamEvent` and `RunItemStreamEvent` pushed to `_event_queue` | `src/agents/stream_events.py:10-61`, used at `run_loop.py:1484`, `:1536-1614` |
| Streaming dedupe | `emitted_tool_call_ids`, `emitted_reasoning_item_ids`, `emitted_tool_search_fingerprints` track already-emitted items | `src/agents/run_internal/run_loop.py:1280-1703` |
| Cancel mode | `_cancel_mode: Literal["none", "immediate", "after_turn"]` checked between turns | `src/agents/result.py:496` |
| Tests — max turns (sync) | `test_non_streamed_max_turns`, `test_non_streamed_max_turns_none_disables_limit` | `tests/test_max_turns.py:31-77` |
| Tests — max turns (stream) | `test_streamed_max_turns`, `test_streamed_max_turns_none_disables_limit` | `tests/test_max_turns.py:82-146` |
| Tests — multi-turn tool call + handoff | `test_handoffs` (3 turns, handoff on second turn) | `tests/test_agent_runner.py:1006-1042` |
| Tests — LLM hooks per turn | `test_async_run_hooks_with_llm`, `test_streamed_run_hooks_with_llm`, etc. | `tests/test_run_hooks.py:89-253` |
| Tests — turn input | `test_run_hooks_receives_turn_input_*` | `tests/test_run_hooks.py:270-326` |
| Tests — `RunState` round-trip | `TestRunState` and `TestSerializationRoundTrip` cover `CURRENT_SCHEMA_VERSION` schema | `tests/test_run_state.py:248-1758`, `:1758-2380` |
| Tests — interruption resume | `TestRunStateResumption` exercises full pause/resume through approval | `tests/test_run_state.py:2823-3043` |
| Tests — error handlers (max turns) | `test_non_streamed_max_turns_handler_*` | `tests/test_max_turns.py:345-440` |
| Tests — stream parallel model+guardrail | `test_run_impl_resume_paths.py` covers `asyncio.create_task(run_single_turn)` racing parallel guardrails | `tests/test_run_impl_resume_paths.py` |

## Answers to Dimension Questions

1. **What happens during one turn?**
   - Sync path (`run.py:768-1504`): a turn is one iteration of `while True:` between `current_turn += 1` (`run.py:1057`) and the next loop body. Inside the iteration the runner: (a) runs first-turn sequential input guardrails (only `current_turn == 1` and not resuming, `run.py:1172-1247`); (b) prepares the agent (sandbox runtime hooks at `run.py:806-828`); (c) for resumed state, calls `resolve_interrupted_turn` (no turn increment, `run.py:840-1025`); (d) opens a `turn_span` (`run.py:1166-1170`) and a fresh usage snapshot (`run.py:1165`); (e) calls `run_single_turn` (`run.py:1196-1216` for turn 1, `:1252-1272` for later turns), which itself fires `on_agent_start`, fetches system prompt + prompt config, prepares input via `_prepare_turn_input_items`, runs `get_new_response` (the model call wrapped in retry), processes the response into `ProcessedResponse`, executes tools/approvals/handoffs, and returns a `SingleStepResult` with a `NextStep*` decision; (f) closes the turn span with usage delta (`run.py:1273-1278`); (g) routes on `NextStepFinalOutput / NextStepInterruption / NextStepHandoff / NextStepRunAgain` (`run.py:1366-1497`), persists session items for the just-completed turn (`run.py:1298-1360`), and loops back. The streaming path mirrors this exactly in `start_streaming` (`run_loop.py:671-1199`).

2. **Does every turn call the model?**
   Yes. Each turn body unconditionally calls `run_single_turn` / `run_single_turn_streamed`, both of which call `model.get_response(...)` / `model.stream_response(...)` (`run_loop.py:1882-1896` and `:1460-1481`). Resumption from `RunState` is the only path that skips the model call: `resolve_interrupted_turn` (`turn_resolution.py:848-2057`) replays the cached `ProcessedResponse` and executes only the remaining tools/approvals.

3. **Can a turn include multiple tool calls?**
   Yes. `ProcessedResponse` (`run_steps.py:115-147`) splits the model's output into `functions`, `computer_actions`, `custom_tool_calls`, `local_shell_calls`, `shell_calls`, `apply_patch_calls`, and `mcp_approval_requests`. The plan/execute split (`_build_plan_for_fresh_turn` → `_execute_tool_plan` in `run_internal/tool_planning.py`) runs them in one batch. Parallel execution of local function tools is capped by `tool_execution.max_function_tool_concurrency` (default unlimited per docs).

4. **Is turn state persisted?**
   Three layers, on every turn:
   - **In-loop mutable state** — `current_turn`, `generated_items`, `session_items`, `model_responses` (`run.py:1159-1163`, `run_loop.py:1069-1076`).
   - **`RunState` durable snapshot** — used when resuming an interruption (`run_state.py:185-322`, schema `1.11` with full versioned summary at `run_state.py:133-149`). The loop writes back to `RunState` on interruption (`run.py:1439-1448`) and on resumption finalization.
   - **`Session` protocol** — `save_result_to_session(session, input_items, new_items, run_state, response_id, store)` is called after most step types (`run.py:1352-1360`, `run_loop.py:349-356`). Each turn also tracks a `_current_turn_persisted_item_count` counter to avoid re-saving items after a resume (`run.py:344`, `run_loop.py:877`).
   - **Server-managed conversation** — `OpenAIServerConversationTracker` (`run_internal/oai_conversation.py`) handles `conversation_id` / `previous_response_id` / `auto_previous_response_id` by sending only deltas.

5. **Is the loop generic or agent-specific?**
   Fully generic. The same `while True` body runs for the starting agent and any subsequent agent produced by a `Handoff` (handoffs just update `current_agent` and reset `should_run_agent_start_hooks`, see `run.py:1472-1482`, `run_loop.py:1091-1112`). Tools, output schema, handoffs, hooks, and system prompt are all re-resolved per turn via `get_all_tools`, `get_output_schema`, `get_handoffs`, and `agent.get_system_prompt` (`turn_preparation.py:88-159`, called from `run_single_turn` and `run_single_turn_streamed`). The loop is the same for subagents invoked through `Agent.as_tool` (handled in `agent.py` via nested runner calls) and for replayed resumes. Test coverage confirms multi-agent handoff sequences (`tests/test_agent_runner.py:1006-1042`, three-turn handoff scenario).

## Architectural Decisions

- **Two parallel loop implementations, kept in sync by policy.** Non-streaming lives in `AgentRunner.run` (`src/agents/run.py:768-1504`); streaming lives in `start_streaming` (`src/agents/run_internal/run_loop.py:671-1199`). AGENTS.md mandates behavioral parity. The split lets streaming emit events as they occur (`_event_queue.put_nowait`) while the sync path collects everything before returning.
- **`NextStep*` discriminated union as the loop's primary contract.** All four outcomes are first-class dataclasses (`run_steps.py:154-173`) instead of returned flags. Each branch is a single explicit `if isinstance(...)` block (`run.py:1366-1497`, `run_loop.py:1065-1150`), which makes the routing table readable and exhaustive.
- **Per-turn `turn_span` trace.** Tracing the turn rather than just the model call makes the cost of a run visible in the trace UI even when tool execution dominates.
- **Schema-versioned `RunState` for HITL pause/resume.** `CURRENT_SCHEMA_VERSION = "1.11"` with full versioned summary table (`run_state.py:131-149`) and an intentional fail-fast policy on unsupported versions. The runner never reads or writes `RunState` fields directly — it goes through well-named helpers in `run_internal/run_state_helpers` / `run_internal/session_persistence.py`.
- **Retry policy split.** Provider-managed retries are disabled on explicit replays but enabled on the first attempt (`model_retry.py:530-548`). `conversation_locked` errors get a separate compatibility-retry budget up to 3 attempts (`model_retry.py:43`, `:558-570`, `:673-688`).
- **Server-managed conversation is opt-in and exclusive.** When `conversation_id` / `previous_response_id` / `auto_previous_response_id` is set, `Session` persistence is auto-disabled (run.py:571-576, run_loop.py:606-612). This avoids double-recording history.
- **`call_model_input_filter` is the only user-extensible message-assembly hook** (`run_config.py:55-64`, applied at `turn_preparation.py:51-85`). It runs after session history has been merged but before the retry wrapper.
- **Streaming events are deduped.** The streaming loop tracks emitted tool-call IDs, reasoning-item IDs, and tool-search fingerprints (`run_loop.py:1280-1309`) so the post-stream `get_single_step_result_from_response` can drop duplicates that were already surfaced as `RawResponsesStreamEvent` (`:1667-1700`).

## Notable Patterns

- **`SingleStepResult` as the loop body return type.** A single dataclass carries everything the outer loop needs to make the next decision: `model_response`, `pre_step_items`, `new_step_items`, `next_step`, `processed_response` (cached for resume), guardrail results (`run_steps.py:178-218`).
- **Tool plan vs. tool execution split.** `tool_planning.py` builds a deterministic plan from a `ProcessedResponse`; `tool_execution.py` runs it. This separation lets the resume path (`_build_plan_for_resume_turn`) construct a different plan that skips already-resolved approvals and reruns only pending tools (`turn_resolution.py:848-1301`).
- **Parallel guardrails + model on turn 1.** `run.py:1219-1247` runs `asyncio.gather(run_input_guardrails(parallel_guardrails), model_task)`, cancelling the model task if a parallel guardrail trips (controlled by `should_cancel_parallel_model_task_on_input_guardrail_trip`). Streaming mirrors this in `run_loop.py:986-995`.
- **`OpenAIServerConversationTracker` as the delta calculator.** Tracks `previous_response_id`, `conversation_id`, and "items already sent" so the runner only ships the new delta on subsequent turns (`oai_conversation.py`).
- **`call_model_input_filter` over `Model.get_response` wrapping.** The user hook runs inside `get_new_response` after session merge but before retry — clean separation between model interface and SDK composition.

## Tradeoffs

- **Duplication between streaming and non-streaming loops.** `run.py:768-1504` and `run_loop.py:671-1199` implement the same routing table with minor differences. AGENTS.md acknowledges this as a maintenance burden ("Changes to `run_internal/run_loop.py` … should be mirrored"). Tests cover both paths independently (`tests/test_max_turns.py`, `tests/test_agent_runner.py`, `tests/test_agent_runner_streamed.py`).
- **Loop body length.** `run.py:768-1504` is ~737 lines of inline orchestration; `run_loop.py:671-1199` is ~528 lines. Splitting into more focused helpers (e.g. a `TurnRouter` that takes `(turn_result, run_state, streamed_result)` and returns the next loop decision) would reduce risk.
- **Schema versioning is intentionally fail-fast.** Unsupported versions of `RunState` reject the snapshot rather than attempt to load, which is correct for safety but means SDK upgrades may block older persisted states.
- **`call_model_input_filter` cannot see the retry path's filtered input on replay.** The filter runs once per turn; retries use the already-filtered input. This is intentional (avoid re-running user code on replay) but means filter bugs persist for the lifetime of the retry sequence.
- **No user-facing "turn ID".** Turns are identified by an integer counter only — there is no UUID per turn in `RunState` or in traces. Resume from an interruption depends on matching `_current_turn` + `_current_step` + `_model_responses[-1]` rather than a stable identifier.
- **Tool tracking persists per agent name, not per turn.** `AgentToolUseTracker` (`run_internal/tool_use_tracker.py`) accumulates tool names per agent across the whole run; tool-choice reset logic (`maybe_reset_tool_choice` in `tool_execution.py`) uses this to stop the model from repeating a tool indefinitely, but it does not reset at turn boundaries.

## Failure Modes / Edge Cases

- **`MaxTurnsExceeded` is raisable but optional.** The `error_handlers["max_turns"]` handler can convert it into a synthetic final output (`run.py:1066-1144`, `run_loop.py:881-956`); tests cover both raise and handler paths (`tests/test_max_turns.py:31-146, 345-489`).
- **`ModelBehaviorError` on missing model response.** If the stream ends without a `ResponseCompletedEvent` (`run_loop.py:1638-1639`), the runner raises. Same for `response.incomplete` / `response.failed` / `response.error` events (`:1488-1499`).
- **Tool not found.** Default `tool_not_found_behavior="raise_error"` raises `ModelBehaviorError`; opt-in `"return_error_to_model"` synthesizes a `function_call_output` and continues the loop (`docs/running_agents.md:194-210`).
- **Parallel-tool sibling failure.** When two function tools run in parallel and one fails or is cancelled, the runner cancels siblings with `_should_cancel_parallel_model_task_on_input_guardrail_trip` semantics, drains completed fatal failures first, and surfaces a consistent error (`tests/test_run_step_execution.py:613-2487`).
- **Approval interruption.** A tool that requires approval produces `NextStepInterruption` (`run_steps.py:169-173`), persists `RunState` (`run.py:1439-1448`), and returns a `RunResult` with `interruptions` populated. Resume reuses `resolve_interrupted_turn` to rebuild the plan from pending approvals only (`turn_resolution.py:848-2057`).
- **Conversation-locked compatibility retries.** Up to 3 backoff retries on `conversation_locked` (`model_retry.py:43,558-570,673-688`) before falling through to the user-configured retry policy.
- **Cancel mid-turn.** `_cancel_mode == "immediate"` raises at any await; `"after_turn"` lets the current turn complete, then breaks (`result.py:496`, checked at `run_loop.py:842-845, 1109-1112, 1161-1164`).
- **Sandbox prep tripwire.** Blocking first-turn guardrails run *before* sandbox prep so a tripwire can prevent sandbox session creation (`run.py:780-802`, `run_loop.py:682-712`).

## Future Considerations

- **Refactor the long loop bodies.** The four `NextStep*` branches (`run.py:1366-1497`, `run_loop.py:1065-1150`) are mechanically identical — extract a `decide_next_step(turn_result, agent, run_state, run_config) -> LoopDecision` and let both loops call it.
- **Reduce duplication between `run_single_turn` and `run_single_turn_streamed`.** They differ only in (a) how the model is invoked and (b) how events are emitted. A shared `_prepare_and_call_model(...)` that returns a `ModelResponse` and an optional async event iterator would let both callers share retry wiring.
- **Stable turn identifiers.** Adding a UUID per turn to `RunState` would make resume matching more robust against `_current_turn` collisions in cross-SDK or cross-version scenarios.
- **Streaming vs. non-streaming test parity.** Both paths have max-turns tests, but resume-path tests are still concentrated on the non-streaming path. Coverage for streamed resume from `RunState` could be expanded.

## Questions / Gaps

- **Why are streaming and non-streaming loops separate?** AGENTS.md says they "should be mirrored", but the design rationale (event ordering vs. back-pressure) is not documented in code. Worth confirming whether the split is intentional for performance or a historical accident.
- **Does the loop ever run more than one model call per turn?** No — `run_single_turn` and `run_single_turn_streamed` each make exactly one `model.get_response` / `stream_response` call. There is no internal "sub-turn" concept; agent-as-tool nesting creates a new `Runner.run` call rather than a sub-turn.
- **Where does agent-as-tool invocation land in the loop?** `Agent.as_tool` (`src/agents/agent.py`) creates a fresh `Runner.run` call, which gets its own loop and turn counter. This means tool-nested agent calls are not part of the outer run's `current_turn` (the outer loop sees them as one tool execution).
- **No explicit "loop visibility" hook.** RunHooks (`src/agents/lifecycle.py:13-207`) cover `on_agent_start/end`, `on_llm_start/end`, `on_handoff`, `on_tool_start/end`; there is no `on_turn_start/end` hook, though `turn_span` provides tracing equivalent.
- **Search boundary:** I read `run.py`, `run_loop.py`, `turn_resolution.py`, `turn_preparation.py`, `run_steps.py`, `model_retry.py`, `models/interface.py`, `run_state.py` (top 350 lines + schema summary), `lifecycle.py`, `stream_events.py`, `result.py` (top 600 lines), `run_config.py` (top 100 lines), and skimmed `tests/test_max_turns.py`, `tests/test_agent_runner.py` (top 1100 lines), `tests/test_run_hooks.py` (top 250 lines), `tests/test_run_state.py` (top 250 lines), `docs/running_agents.md`. I did not deeply inspect `oai_conversation.py`, `session_persistence.py`, `tool_execution.py`, `tool_planning.py`, or `sandbox/runtime.py`; the evidence for those areas is from their import sites in the main loops, not their internals.

---

Generated by `reports/source/03.01-llm-turn-loop-structure/openai-agents-sdk.md` against `openai-agents-sdk`.