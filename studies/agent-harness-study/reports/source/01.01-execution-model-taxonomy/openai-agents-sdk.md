# Source Analysis: openai-agents-sdk

## 01.01 — Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ with `asyncio`; core runtime in `src/agents/`, event-driven realtime and audio extensions in `src/agents/realtime/` and `src/agents/voice/`, optional experimental Codex subprocess client under `src/agents/extensions/experimental/codex/`. |
| Analyzed | 2026-07-02 |

## Summary

OpenAI Agents SDK exposes one **primary turn-based step loop** that drives every `Runner.run*` call, plus four sibling execution models used by distinct entry surfaces (realtime, voice, REPL, Codex).

The primary model is a single `while True` outer loop (`AgentRunner.run` in `src/agents/run.py:768`) that calls a one-turn workhorse (`run_single_turn` in `src/agents/run_internal/run_loop.py:1708-1795`) and dispatches on the typed discriminant carried back in `SingleStepResult.next_step` (`src/agents/run_internal/run_steps.py:178-218`). The discriminant is a tagged union with four variants: `NextStepFinalOutput`, `NextStepHandoff`, `NextStepRunAgain`, and `NextStepInterruption` (`src/agents/run_internal/run_steps.py:154-173`).

The streaming variant (`start_streaming` in `src/agents/run_internal/run_loop.py:440-1239`, spawned by `Runner.run_streamed` in `src/agents/run.py:1647-1876`) shares the same state machine and discriminant, with the only architectural difference being an `asyncio.Queue` plus `QueueCompleteSentinel` pair that feeds `RunResultStreaming.stream_events()` (`src/agents/result.py:483-779`).

Sibling execution models in this repo:

- An **event-driven (listener) router** for realtime: `RealtimeSession.on_event` (`src/agents/realtime/session.py:293-465`) dispatches `RealtimeModelEvent`s arriving from a WebSocket model (`OpenAIRealtimeWebSocketModel._emit_event`, `src/agents/realtime/openai_realtime.py:622-1119`).
- A **3-stage linear pipeline** for audio: `VoicePipeline._run_single_turn` / `_run_multi_turn` (`src/agents/voice/pipeline.py:86-156`) composes STT → a `VoiceWorkflowBase.run` → TTS; the workflow itself nests the turn loop (`SingleAgentVoiceWorkflow` calls `Runner.run_streamed` at `src/agents/voice/workflow.py:93-97`).
- A **subprocess line-stream consumer** for Codex: `Thread._run_streamed_internal` (`src/agents/extensions/experimental/codex/thread.py:96-160`) reads line-delimited JSON from a `CodexExec` subprocess.
- An **outer chat REPL** wrapping the turn loop: `run_demo_loop` (`src/agents/repl.py:38-75`) drives `Runner.run` / `Runner.run_streamed` from `input()`.

Pause/resume is a first-class concern: the loop's `NextStepInterruption` state is serialized into `RunState` (`src/agents/run_state.py:184-321`, schema `CURRENT_SCHEMA_VERSION = "1.11"` at `src/agents/run_state.py:131-149`), and the resumed run re-enters the same loop with `_current_step` pre-populated (`src/agents/run_internal/turn_resolution.py:848-1548`).

## Rating

**Score: 9 / 10 — Mature, durable, and explicit turn-discriminated step loop, with two cleanly factored streaming and interruption variants; one minor deduction for the size of the outer-loop body, one for the existence of unrelated sibling models that share no state with it.**

Rationale:

- **Explicit model.** The runner is named (`Runner` / `AgentRunner`), the per-turn function is named (`run_single_turn` / `run_single_turn_streamed`), and the state-transition object is a typed Python `@dataclass` union (`SingleStepResult.next_step: NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption`, `src/agents/run_internal/run_steps.py:192`). A new contributor can read `src/agents/run.py:768-1497` and immediately see the four branches in the dispatch.
- **Tests pin the discriminant.** `tests/test_run_step_execution.py:2749-2780` exhaustively asserts each `next_step` variant (`test_final_output_without_tool_runs_again`, `test_final_output_leads_to_final_output_next_step`, `test_handoff_and_final_output_leads_to_handoff_next_step`), and the larger test surface (`tests/test_agent_runner.py`, `tests/test_agent_runner_streamed.py`, `tests/test_runner_guardrail_resume.py`) exercises every flow.
- **Two parallel streams share the same state machine.** `run_single_turn` (`src/agents/run_internal/run_loop.py:1708-1795`) and `run_single_turn_streamed` (`src/agents/run_internal/run_loop.py:1242-1705`) both return `SingleStepResult` and emit the same four `next_step` variants; the dispatch sites at `src/agents/run.py:1367-1497` and `src/agents/run_internal/run_loop.py:791-1156` are structurally identical.
- **Operational safeguards.** `max_turns` is enforced (`MaxTurnsExceeded` raised in both runners, `src/agents/run.py:1066` and `src/agents/run_internal/run_loop.py:889-956`); the first turn races input guardrails against the model task via `asyncio.gather` and cancels the model when a parallel guardrail trips (`src/agents/run.py:1219-1245`); cancellation has explicit modes (`immediate` and `after_turn`) that the inner loop checks (`src/agents/result.py:648-694`, `src/agents/run_internal/run_loop.py:842-845, 1109-1112, 1161-1164`); the streaming `asyncio.Queue` is terminated by a sentinel rather than an implicit generator-close.
- **Deduction 1 (size):** `AgentRunner.run` `while True` body is approximately 730 lines (`src/agents/run.py:768-1497`) and interleaves session persistence, server-conversation tracking, sandbox enqueueing, guardrails, and the `next_step` dispatch. `AGENTS.md:54-67` acknowledges this and prescribes more decomposition into `run_internal/`. Some decomposition has happened (`session_persistence.py`, `oai_conversation.py`, `approvals.py`, `turn_resolution.py`, `tool_planning.py`, `tool_execution.py`, `agent_runner_helpers.py`), but the outer method body still concentrates too much.
- **Deduction 2 (sibling-model isolation):** The realtime session, voice pipeline, and Codex subprocess client are entirely separate execution models — they neither use the turn discriminant nor `RunState`. There is no shared abstraction over them. `SingleAgentVoiceWorkflow` (`src/agents/voice/workflow.py:62-101`) is the only place a sibling model delegates back to the primary step loop.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers, format `path/to/file.ext:NN`.

### Primary step loop (turn loop)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public entry points | `Runner.run` / `Runner.run_sync` / `Runner.run_streamed` classmethods | `src/agents/run.py:199, 283, 365` |
| Underlying orchestrator class | `class AgentRunner` with `run`, `run_sync`, `run_streamed` methods | `src/agents/run.py:444-1876` |
| Outer loop driver | `try: while True:` advances `current_turn += 1`, dispatches based on `turn_result.next_step` | `src/agents/run.py:768-1497` |
| Single-turn work (non-streaming) | `run_single_turn` calls `get_new_response` then `get_single_step_result_from_response` | `src/agents/run_internal/run_loop.py:1708-1795` |
| Single-turn work (streaming) | `run_single_turn_streamed` consumes `model.stream_response` events and emits to `streamed_result._event_queue` | `src/agents/run_internal/run_loop.py:1242-1705` |
| Streaming loop entry | `start_streaming` mirrors the non-streaming `while True` body | `src/agents/run_internal/run_loop.py:440-1239` |
| `run_streamed` schedules background loop | `streamed_result.run_loop_task = asyncio.create_task(start_streaming(...))` | `src/agents/run.py:1855-1873` |
| Step discriminant types | `NextStepFinalOutput`, `NextStepHandoff`, `NextStepRunAgain`, `NextStepInterruption` dataclasses | `src/agents/run_internal/run_steps.py:154-173` |
| Step container | `SingleStepResult.next_step: NextStepHandoff \| NextStepFinalOutput \| NextStepRunAgain \| NextStepInterruption` | `src/agents/run_internal/run_steps.py:178-218` |
| Outer-loop dispatch sites | `if isinstance(turn_result.next_step, NextStepFinalOutput): ...` / `NextStepHandoff` / `NextStepInterruption` / `NextStepRunAgain` branches | `src/agents/run.py:1367-1497`; `src/agents/run_internal/run_loop.py:791-1156` |
| Loop escape via `continue` | `run.py:948-949`, `run.py:1493`, `run_loop.py:831-838` (after `NextStepRunAgain` resumed) | `src/agents/run.py:948, 1493`; `src/agents/run_internal/run_loop.py:831, 838` |
| Max-turns guard (sync) | `if max_turns is not None and current_turn > max_turns` → `MaxTurnsExceeded` after consulting `error_handlers` | `src/agents/run.py:1058-1144` |
| Max-turns guard (streamed) | Same check, emits `QueueCompleteSentinel` and breaks instead of raising | `src/agents/run_internal/run_loop.py:881-956` |
| Streaming cancel modes | `cancel(mode="immediate")` cancels tasks immediately; `mode="after_turn"` lets current turn finish | `src/agents/result.py:648-694` |
| Soft-cancel after-turn check | `if streamed_result._cancel_mode == "after_turn": break` inside the streaming loop | `src/agents/run_internal/run_loop.py:842-845, 1109-1112, 1161-1164` |
| Sync wrapper | `Runner.run_sync` schedules the async `run` on the thread's persistent default loop using `asyncio.get_event_loop()` | `src/agents/run.py:1564-1645` |

### Inside one turn

| Area | Evidence | File:Line |
|------|----------|-----------|
| Classify model output into tool/handoff/function runs | `process_model_response` returns `ProcessedResponse` with `handoffs`, `functions`, `computer_actions`, `local_shell_calls`, `shell_calls`, `apply_patch_calls`, `mcp_approval_requests`, `function_tools_not_found` | `src/agents/run_internal/turn_resolution.py:1551-1999` |
| Run tools/approvals/guardrails and choose next step | `execute_tools_and_side_effects` returns a `SingleStepResult` with one of the four `next_step` variants | `src/agents/run_internal/turn_resolution.py:629-845` |
| Tool planning (function/shell/apply-patch/MCP) | `_build_plan_for_fresh_turn`, `_execute_tool_plan`, `_collect_tool_interruptions` | `src/agents/run_internal/tool_planning.py` |
| Per-tool-type execution | `execute_function_tool_calls`, `execute_shell_calls`, `execute_apply_patch_calls`, `execute_computer_actions`, `execute_local_shell_calls`, `execute_mcp_approval_requests` | `src/agents/run_internal/tool_execution.py` |
| Handoff orchestration (call new agent, mutate input) | `execute_handoffs` returns `NextStepHandoff`; supports `input_filter`, `nest_handoff_history`, `_resolve_server_managed_handoff_behavior` | `src/agents/run_internal/turn_resolution.py:403-591` |
| Final-output coercion / structured output validation | `execute_final_output_step` returns `NextStepFinalOutput`; `output_schema.validate_json` path; refusal/error handler fallback | `src/agents/run_internal/turn_resolution.py:305-368, 769-836` |
| Interruption when approval is required | `processed_response.interruptions` triggers early return with `NextStepInterruption` | `src/agents/run_internal/turn_resolution.py:699-723` |
| Resume an interrupted turn | `resolve_interrupted_turn` rebuilds function/shell/apply-patch runs from approval state, classifies nested approvals, returns one of the four `next_step` variants | `src/agents/run_internal/turn_resolution.py:848-1548` |
| Turn-entry composition | `get_single_step_result_from_response` = `process_model_response` ∘ `execute_tools_and_side_effects` | `src/agents/run_internal/turn_resolution.py` |
| Per-turn model call | `get_new_response` handles retries via `get_response_with_retry`, hooks, `call_model_input_filter`, server-conversation deltas | `src/agents/run_internal/run_loop.py:1798-1919` |
| Tool-use tracker | `AgentToolUseTracker` records which tools ran; consumed via `maybe_reset_tool_choice` and `serialize_tool_use_tracker` | `src/agents/run_internal/tool_use_tracker.py` |

### Streaming event channel

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event types | `RawResponsesStreamEvent`, `RunItemStreamEvent`, `AgentUpdatedStreamEvent` | `src/agents/stream_events.py` |
| Run-item event names | `message_output_created`, `handoff_requested`, `handoff_occured`, `tool_called`, `tool_output`, `reasoning_item_created`, `tool_search_called`, `tool_search_output_created`, `mcp_approval_requested`, `mcp_approval_response`, `mcp_list_tools` | `src/agents/stream_events.py:23-58` |
| Streaming queue | `RunResultStreaming._event_queue: asyncio.Queue[StreamEvent \| QueueCompleteSentinel]` | `src/agents/result.py:483-485` |
| Consumer coroutine | `RunResultStreaming.stream_events()` while-loop pulls from queue, surfaces stored exceptions, yields events | `src/agents/result.py:696-779` |
| Producer emission | `RawResponsesStreamEvent` / `RunItemStreamEvent` / `AgentUpdatedStreamEvent` pushed via `put_nowait` from inside `run_single_turn_streamed` | `src/agents/run_internal/run_loop.py:589, 812, 1104, 1484, 1537, 1549, 1613, 1624` |
| Mapping items → events | `stream_step_items_to_queue`, `stream_step_result_to_queue` classify `RunItem` subclasses into `RunItemStreamEvent` names | `src/agents/run_internal/streaming.py:28-72` |
| Termination sentinel | `QueueCompleteSentinel` placed on queue when turn finalizes, interrupts, cancels, or errors | `src/agents/run_steps.py:52-56`; `src/agents/run_internal/run_loop.py:417, 667, 698, 844, 911, 955, 970, 1111, 1163, 1198, 1239` |
| Cancellation integration | `_cancel_mode` set by `cancel()`; loop checks `_cancel_mode == "after_turn"` each iteration | `src/agents/result.py:648-694`; `src/agents/run_internal/run_loop.py:842-845, 1109-1112, 1161-1164` |

### Durable state and pause/resume

| Area | Evidence | File:Line |
|------|----------|-----------|
| `RunState` dataclass | `RunState(Generic[TContext, TAgent])` with context, original_input, starting_agent, max_turns, conversation_id, generated/session items, model responses, current step | `src/agents/run_state.py:184-321` |
| Current-step field | `_current_step: NextStepInterruption \| None` stores the discriminant that the resumed run re-enters with | `src/agents/run_state.py:254` |
| Schema versioning | `CURRENT_SCHEMA_VERSION = "1.11"` and `SCHEMA_VERSION_SUMMARIES` map documenting 1.0–1.11 | `src/agents/run_state.py:131-149` |
| Approval API on state | `state.approve(approval_item)` / `state.reject(...)` mutate `RunContextWrapper._approvals` | `src/agents/run_state.py` |
| Resume helpers | `apply_resumed_conversation_settings`, `update_run_state_after_resume`, `update_run_state_for_interruption`, `resolve_interrupted_turn` | `src/agents/run_internal/agent_runner_helpers.py`; `src/agents/run_internal/turn_resolution.py:848-1548` |
| `to_state()` round-trip | `RunResult.to_state()` and `RunResultStreaming.to_state()` populate `RunState` from a result | `src/agents/result.py:393-438, 888-936` |
| Approval identity compatibility | `_allow_legacy_name_agent_match` accepts same-name agent match for `RunState` schema `< 1.7`; schema `>= 1.7` requires object identity | `src/agents/run_internal/turn_resolution.py:1132-1144` |

### Event-driven realtime model

| Area | Evidence | File:Line |
|------|----------|-----------|
| Realtime entry point | `RealtimeRunner.run()` returns a `RealtimeSession` connected to a `RealtimeModel` | `src/agents/realtime/runner.py:18-75` |
| Listener pattern | `OpenAIRealtimeWebSocketModel.add_listener`, `_emit_event` iterate over `_listeners` and call `listener.on_event` | `src/agents/realtime/openai_realtime.py:612-626` |
| Session event routing | `RealtimeSession.on_event` dispatches on `event.type` (`function_call`, `audio`, `audio_interrupted`, `audio_done`, `input_audio_transcription_completed`, `input_audio_timeout_triggered`, `transcript_delta`, `item_updated`, `item_deleted`, `connection_status`, `turn_started`, `turn_ended`, `exception`, ...) | `src/agents/realtime/session.py:293-465` |
| Async tool dispatch | `_enqueue_tool_call_task` (default) vs. synchronous `_handle_tool_call` (when `async_tool_calls=False`) | `src/agents/realtime/session.py:300-303, 793-893` |
| Approval flow | `approve_tool_call` / `reject_tool_call` mutate `RunContextWrapper` approval state and resume execution | `src/agents/realtime/session.py:733-791` |
| Guardrail debouncing | `transcript_delta` accumulates `_item_transcripts` and triggers guardrail runs at threshold multiples | `src/agents/realtime/session.py:343-367` |
| Response-create sequencing | `_ResponseCreateSequencer` with `asyncio.Condition` for serializing `response.create` requests to the server | `src/agents/realtime/openai_realtime.py:197-364` |

### Codex subprocess streaming

| Area | Evidence | File:Line |
|------|----------|-----------|
| Entry point | `Codex.start_thread` / `resume_thread` returns a `Thread` | `src/agents/extensions/experimental/codex/codex.py:69-86` |
| Run methods | `Thread.run_streamed` and `Thread.run` (aggregator that consumes the streamed generator) | `src/agents/extensions/experimental/codex/thread.py:90-188` |
| Stream consumption | `async for event in generator` over `_run_streamed_internal`, parsing line-delimited JSON via `coerce_thread_event` | `src/agents/extensions/experimental/codex/thread.py:96-160, 210-214` |
| Idle timeout | `asyncio.wait_for(stream.__anext__(), timeout=idle_timeout)` converts timeout into a `RuntimeError` | `src/agents/extensions/experimental/codex/thread.py:138-150` |

### Voice pipeline (3-stage linear)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pipeline entry | `VoicePipeline.run` chooses `_run_single_turn` (audio file) or `_run_multi_turn` (streamed audio) | `src/agents/voice/pipeline.py:48-65` |
| Single-turn composition | STT → `workflow.run(text)` → TTS, wrapped in `TraceCtxManager` | `src/agents/voice/pipeline.py:86-111` |
| Multi-turn composition | Streamed STT session (`transcription_session.transcribe_turns`) drives a loop of `workflow.run` | `src/agents/voice/pipeline.py:113-156` |
| Workflow base | `VoiceWorkflowBase.run` is abstract; default `SingleAgentVoiceWorkflow` calls `Runner.run_streamed` | `src/agents/voice/workflow.py:13-101` |
| Stream helper | `VoiceWorkflowHelper.stream_text_from` filters `response.output_text.delta` raw events | `src/agents/voice/workflow.py:44-53` |

### REPL (outer user-facing loop)

| Area | Evidence | File:Line |
|------|----------|-----------|
| REPL demo loop | `run_demo_loop` wraps `Runner.run` / `Runner.run_streamed` in a `while True` reading `input()` | `src/agents/repl.py:38-75` |

### Tests that pin the model

| Area | Evidence | File:Line |
|------|----------|-----------|
| Single-turn final output | `test_simple_first_run`, `test_tool_call_runs` exercise the canonical `NextStepFinalOutput` path | `tests/test_agent_runner.py:533-617` |
| Step-by-step turn results | Many `tests/test_run_step_execution.py` cases assert `isinstance(result.next_step, NextStepRunAgain \| NextStepFinalOutput \| NextStepInterruption)` | `tests/test_run_step_execution.py:2749-2780` |
| Streaming model integration | `tests/test_agent_runner_streamed.py` covers `Runner.run_streamed` with text/tool/handoff cases | `tests/test_agent_runner_streamed.py` |
| Cancellation modes | `tests/test_cancel_streaming.py` asserts both `immediate` and `after_turn` cancellation | `tests/test_cancel_streaming.py` |
| Guardrail + model race | `tests/test_stream_input_guardrail_timing.py` asserts parallel guardrail order with model task | `tests/test_stream_input_guardrail_timing.py` |
| Realtime session | `tests/realtime/test_session.py` exercises `RealtimeSession` event loop | `tests/realtime/test_session.py` |
| Voice pipeline | `tests/voice/test_pipeline.py` covers `_run_single_turn` and `_run_multi_turn` | `tests/voice/test_pipeline.py` |
| Run state resume | `tests/test_run_state.py` and `tests/test_run_impl_resume_paths.py` exercise interruption resume | `tests/test_run_state.py`; `tests/test_run_impl_resume_paths.py` |
| Codex subprocess | `tests/extensions/experimental/codex/` covers the subprocess stream model | `tests/extensions/experimental/codex/` |

## Answers to Dimension Questions

### 1. What is the primary execution model?

A **turn-based step loop with a discriminated next-step state machine**. The outer loop (`AgentRunner.run` `while True` at `src/agents/run.py:768`) calls a single-turn function (`run_single_turn` at `src/agents/run_internal/run_loop.py:1708` or `run_single_turn_streamed` at `src/agents/run_internal/run_loop.py:1242`). Each call:

1. Builds the model input from `original_input` + `generated_items` via `_prepare_turn_input_items` (`src/agents/run_internal/run_loop.py:280-288`).
2. Calls the model (`get_new_response` at `src/agents/run_internal/run_loop.py:1798-1919` or `model.stream_response` at `src/agents/run_internal/run_loop.py:1460-1481`).
3. Classifies the response into a `ProcessedResponse` (`process_model_response` in `src/agents/run_internal/turn_resolution.py`).
4. Runs tools / approvals / guardrails / final-output coercion and returns a `SingleStepResult.next_step` (`execute_tools_and_side_effects` in `src/agents/run_internal/turn_resolution.py`).
5. The outer loop branches on `next_step`: continue, handoff, finalize, or pause for approval.

Secondary models:

- **Event-driven (listener) model** for realtime (`src/agents/realtime/openai_realtime.py:622-1119`, `src/agents/realtime/session.py:293-465`).
- **Subprocess stream model** for Codex (`src/agents/extensions/experimental/codex/thread.py:96-160`).
- **3-stage audio pipeline** for voice (`src/agents/voice/pipeline.py:86-156`).
- **Outer REPL loop** wrapping any of the above (`src/agents/repl.py:38-75`).

### 2. Is it explicit or emergent?

Explicit. The model is named in code (`run_single_turn` is literally named), the step discriminant is encoded as a `@dataclass` union (`SingleStepResult.next_step: NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption`, `src/agents/run_internal/run_steps.py:192`), the loop terminator is a typed `raise AgentsException(f"Unknown next step type: ...")` (`src/agents/run.py:1495-1497`), and the streaming variant uses an explicit `QueueCompleteSentinel` rather than generator-close teardown. The discriminated union gives the static type checker a handle on the runtime state, and the public docs reinforce this (`docs/running_agents.md:34-42`, `docs/streaming.md:38-56`).

### 3. Does the model match the product shape?

Yes. The product is "agents configured with tools, handoffs, guardrails, and sessions" (`README.md:10-19`), and the discriminated step loop expresses each of those as a `next_step` variant:

- `NextStepHandoff` (handoffs)
- `NextStepRunAgain` (tool-call iteration, the ReAct-style core)
- `NextStepFinalOutput` (termination; matches `output_type` validation)
- `NextStepInterruption` (human-in-the-loop, called out as a primary feature in `README.md:17` and `docs/human_in_the_loop.md`)

Streaming is layered on top by swapping the workhorse `run_single_turn` for `run_single_turn_streamed` while keeping the same `next_step` semantics. Streaming does not fork the state machine.

### 4. Is it easy to explain to a new contributor?

Two sentences suffice for the primary model:

> The runner is a `while True` loop that calls `run_single_turn`, which calls the LLM and returns a `SingleStepResult` whose `next_step` is one of `NextStepFinalOutput` (loop ends), `NextStepHandoff` (loop continues with a new agent), `NextStepRunAgain` (loop continues with tool outputs), or `NextStepInterruption` (loop pauses for human approval). The streaming variant is the same loop with a queue plus `run_single_turn_streamed`.

The complication is that this primary explanation covers only `Runner.run*`. Contributors must additionally learn:

- The realtime session is event-sourced, not loop-driven (`src/agents/realtime/session.py:240-466`).
- Voice and Codex each have their own entry point.

These are documented in `docs/realtime/`, `docs/voice/`, and `AGENTS.md`, but the abstractions are not unified — a contributor cannot reuse `AgentRunner` to drive a realtime voice session.

### 5. Does the system mix models cleanly or accidentally?

Mostly **cleanly**. The models are kept in separate packages with disjoint public surfaces:

- Turn loop → `Runner` / `AgentRunner` (`src/agents/run.py`).
- Realtime → `RealtimeRunner` / `RealtimeSession` (`src/agents/realtime/runner.py`, `src/agents/realtime/session.py`).
- Voice → `VoicePipeline` / `VoiceWorkflowBase` (`src/agents/voice/pipeline.py`, `src/agents/voice/workflow.py`).
- Codex → `Codex` / `Thread` (`src/agents/extensions/experimental/codex/codex.py`, `.../thread.py`).

The voice workflow explicitly composes the turn loop (`Runner.run_streamed` inside `SingleAgentVoiceWorkflow.run`, `src/agents/voice/workflow.py:93-97`), so voice does not duplicate the turn state machine — it inherits it. The realtime session is not a wrapper around the turn loop; it implements its own event router. Codex is fully isolated.

The slight **accident** is the size of `AgentRunner.run` itself (`src/agents/run.py:768-1497`): it now contains the orchestration of session persistence, server-conversation tracking, sandbox enqueue, guardrails, and the `next_step` dispatch. `AGENTS.md:54-67` flags this and prescribes splitting helpers into `run_internal/` modules; the split is partial (e.g., `session_persistence.py`, `oai_conversation.py`, `approvals.py`, `turn_resolution.py`, `tool_planning.py`, `tool_execution.py`, `agent_runner_helpers.py` already exist), but the outer `run` method body has not yet been decomposed.

## Architectural Decisions

- **Discriminated next-step union over inheritance/state machine classes.** `SingleStepResult.next_step` is a `@dataclass` union (`src/agents/run_internal/run_steps.py:178-218`). This gives static exhaustiveness checking in the dispatch sites (`src/agents/run.py:1367-1497`) and a single runtime type error when an unknown variant appears (`src/agents/run.py:1495-1497`). The shape was chosen over a state-machine class hierarchy to keep state transitions functional and inspectable.
- **Streaming and non-streaming share the step discriminant.** Both `run_single_turn` and `run_single_turn_streamed` return `SingleStepResult` and emit the same `next_step` values, so the outer loop logic is identical (`src/agents/run.py:768-1497` vs. `src/agents/run_internal/run_loop.py:670-1239`). Streaming adds an `asyncio.Queue` and a sentinel; the state machine is unchanged.
- **Pause/resume as a first-class durable artifact.** `RunState` (`src/agents/run_state.py:184-321`) is a serializable snapshot capturing the current `next_step` plus processed response, generated items, tool-use tracker, prompt cache key, and trace state, versioned by `CURRENT_SCHEMA_VERSION = "1.11"` (`src/agents/run_state.py:131-149`). Schema upgrades append new fields and bump the version; older versions remain readable for resumptions, per the documented policy (`AGENTS.md`, `src/agents/run_state.py:124-149`).
- **Parallel guardrail + model first turn.** The first turn runs `sequential_guardrails` (blocking) before the model call, then races `parallel_guardrails` against the model task via `asyncio.gather`. If the parallel guardrail trips, the model task is cancelled (`should_cancel_parallel_model_task_on_input_guardrail_trip`, `src/agents/run.py:1219-1245`). Tripwires are not just raise-on-completion; they actively pre-empt in-flight model work.
- **Server-managed conversation state.** `OpenAIServerConversationTracker` (`src/agents/run_internal/oai_conversation.py`) maintains deltas of items already sent to the server (`mark_input_as_sent` / `track_server_items`), so subsequent turns only ship new input rather than the full history. This is wired through both the sync (`src/agents/run.py:563-571`) and streaming (`src/agents/run_internal/run_loop.py`) paths.
- **REPL is an outer loop, not an execution mode.** `run_demo_loop` (`src/agents/repl.py:38-75`) is a user-facing utility that calls `Runner.run` / `Runner.run_streamed` inside its own `while True`, but does not change the agent execution model — each iteration is a complete `Runner.run` call.
- **Realtime is intentionally not a turn loop.** `RealtimeSession` is a listener on `RealtimeModel` events (`src/agents/realtime/session.py:293-465`); tool calls dispatch immediately upon arrival (`src/agents/realtime/session.py:298-303`), and a `_ResponseCreateSequencer` uses an `asyncio.Condition` to serialize `response.create` requests (`src/agents/realtime/openai_realtime.py:197-364`). The trade-off is documented: realtime gets lower latency for audio but does not benefit from the turn state machine's durability.
- **Codex is a subprocess stream, not a Python loop.** The Codex model is invoked via `CodexExec` as an external subprocess; `Thread._run_streamed_internal` reads line-delimited JSON output and parses each line into a `ThreadEvent` (`src/agents/extensions/experimental/codex/thread.py:96-160, 210-214`). Idle timeouts surface as `RuntimeError` (`src/agents/extensions/experimental/codex/thread.py:138-150`).

## Notable Patterns

- **Discriminated next-step union** (`src/agents/run_internal/run_steps.py:178-218`).
- **Streaming queue + sentinel termination** (`src/agents/result.py:483-485, 696-779`; `src/agents/run_internal/run_loop.py:417, 1198`).
- **Cancellation modes** — `immediate` cancels background tasks; `after_turn` lets the current turn finish (`src/agents/result.py:648-694`; `src/agents/run_internal/run_loop.py:842-845, 1109-1112, 1161-1164`).
- **First-turn guardrail race with model-task cancellation** (`src/agents/run.py:1219-1245`).
- **Handoff input filter / nested history** with explicit support for both async and sync filters (`src/agents/run_internal/turn_resolution.py:403-591`).
- **Tool approval interrupts via `RunState`** with explicit schema-version-gated identity matching for legacy snapshots (`src/agents/run_internal/turn_resolution.py:1132-1154`).
- **Server-managed conversation deltas** via `OpenAIServerConversationTracker` (`src/agents/run_internal/oai_conversation.py`).
- **Sandbox-aware run lifecycle** with `SandboxRuntime` preparing the agent each turn and enqueuing memory payloads on completion (`src/agents/sandbox/runtime.py`).
- **Listener pattern for realtime events** with `_emit_event` iterating over `list(self._listeners)` (`src/agents/realtime/openai_realtime.py:622-626`).
- **Realtime `_ResponseCreateSequencer` condition variable** for serializing `response.create` events (`src/agents/realtime/openai_realtime.py:197-364`).
- **Trace/agent/turn/task span layering** so each iteration of the outer loop produces a `turn_span` inside an `agent_span` inside a `task_span` (`src/agents/run.py:1166-1170`, `src/agents/run_internal/run_loop.py:1003-1007`).
- **Pydantic `__get_pydantic_core_schema__` on `RunResultBase`** to prevent recursive schema generation (`src/agents/result.py:225-233`).

## Tradeoffs

- **Discriminated union over state machine class.** `next_step` as a discriminated union trades "easy to add fields" for "explicit static checks and one-switch dispatch". Adding a new `NextStep*` requires editing every dispatch site (`src/agents/run.py:1367-1497`, `src/agents/run_internal/run_loop.py:791-1156`, `src/agents/run_internal/turn_resolution.py`) — a known maintenance cost.
- **Turn loop grows large.** `AgentRunner.run` (`src/agents/run.py:768-1497`) interleaves session persistence, server-conversation tracking, sandbox enqueueing, guardrails, and dispatch. `AGENTS.md:54-67` flags this and prescribes splitting into `run_internal/` helpers; some splitting has occurred but the outer body remains monolithic.
- **Streaming has parallel `run_single_turn*` paths.** `run_single_turn` (`src/agents/run_internal/run_loop.py:1708-1795`) and `run_single_turn_streamed` (`src/agents/run_internal/run_loop.py:1242-1705`) share intent but not code; changes to one must be mirrored in the other (`AGENTS.md:58-66`). `AGENTS.md` explicitly requires this discipline.
- **Realtime and Codex are isolated models.** They do not reuse `RunState`, do not use the turn discriminant, and have separate interruption semantics. A contributor working on voice or realtime cannot lean on the durable-state and resume capabilities of the turn loop.
- **Sandbox runtime adds a second control flow.** `SandboxRuntime` is invoked on every iteration (`src/agents/run.py:806-813`) and on completion (`src/agents/run.py:1527-1540`), which means a sandbox-enabled run inherits a control flow orthogonal to the turn loop. This is documented and necessary, but it does add surface.
- **First-turn-only input guardrails.** Per `AGENTS.md:73-77`, input guardrails run only on the first turn and only for the starting agent. Resuming from `RunState` does not increment the turn counter, so guardrails are explicitly opt-out for resumed runs.
- **REPL swallows background loop exceptions via stored-exception check.** `RunResultStreaming._check_errors` (`src/agents/result.py:793-837`) drains guardrail queues and run-loop exceptions; a streaming consumer that exits `stream_events()` early without draining will lose late-firing guardrail exceptions until `run_loop_exception` is inspected (`src/agents/result.py:623-646`).
- **Voice pipeline reuses `Runner.run_streamed` only for `SingleAgentVoiceWorkflow`.** More complex workflows (`VoiceWorkflowBase` subclasses) must orchestrate multiple `Runner.run` calls themselves, with the helper `VoiceWorkflowHelper.stream_text_from` only filtering text deltas (`src/agents/voice/workflow.py:44-53`).
- **Codex subprocess stream is fire-and-await.** There is no resumable `RunState`; if the subprocess dies mid-stream, the call raises and the caller must start a new `Thread` (`src/agents/extensions/experimental/codex/thread.py:138-188`).

## Failure Modes / Edge Cases

- **Max-turns exceeded.** `MaxTurnsExceeded` is raised by both `Runner.run` (`src/agents/run.py:1066`) and `Runner.run_streamed` (`src/agents/run_internal/run_loop.py:889-956`). `RunResultStreaming._check_errors` converts this into a stored exception (`src/agents/result.py:793-801`).
- **Guardrail tripwire mid-stream.** First-turn input guardrails can pre-empt an in-flight model task (`src/agents/run.py:1231-1245`); later tripwires from late guardrail tasks are surfaced via `_check_errors` after stream drain (`src/agents/result.py:804-826`).
- **Handoff with mismatched input filter.** `_resolve_server_managed_handoff_behavior` raises `UserError` if `input_filter` is set under server-managed conversation (`src/agents/run_internal/turn_resolution.py:383-389`); async input filters that don't return `HandoffInputData` raise `UserError` (`src/agents/run_internal/turn_resolution.py:539-547`).
- **Empty model input.** `if not filtered.input and server_conversation_tracker is None: raise RuntimeError("Prepared model input is empty")` in `run_single_turn_streamed` (`src/agents/run_internal/run_loop.py:1391-1392`).
- **Model never produces a final response.** `if not final_response: raise ModelBehaviorError("Model did not produce a final response!")` (`src/agents/run_internal/run_loop.py:1638-1639`).
- **Stream terminal failure.** `response_terminal_failure_error` raises on `response.incomplete` / `response.failed` events (`src/agents/run_internal/run_loop.py:1491-1497`).
- **Cancellation race.** `_cleanup_tasks` cancels `run_loop_task`, `_input_guardrails_task`, `_output_guardrails_task`; `_await_task_safely` swallows `CancelledError` but routes other exceptions via `_check_errors` (`src/agents/result.py:852-866`).
- **Sandbox resume state mismatch.** `SandboxRuntime.cleanup` returns serialized sandbox state attached to the result; restored state mismatches surface during `SandboxRuntime.prepare_agent`. The `RunState` schema version gates this for cross-SDK snapshots (`src/agents/run_internal/turn_resolution.py:1132-1144`).
- **Streaming-only final-output backfill.** If a streaming backend emits empty `terminal_response.output`, the runner preserves items emitted during `ResponseOutputItemDoneEvent` so `process_model_response` still resolves the turn (`src/agents/run_internal/run_loop.py:1502-1506`).
- **Realtime response-create sequencing.** `_ResponseCreateSequencer.wait_for_response_create_slot` may return `None` if a newer request supersedes the older one (`src/agents/realtime/openai_realtime.py:323-324`).
- **Realtime async tool calls vs. sync tool calls.** `_async_tool_calls: bool` (`src/agents/realtime/session.py:196`) routes function calls through either `_enqueue_tool_call_task` (default, async) or `_handle_tool_call` (sync, blocks event loop). Mixed modes across runs in the same session can produce different back-pressure semantics.
- **Codex idle timeout.** `asyncio.TimeoutError` after `idle_timeout_seconds` is converted to `RuntimeError(f"Codex stream idle for {idle_timeout} seconds.")` (`src/agents/extensions/experimental/codex/thread.py:138-150`).
- **Legacy schema agent-name matching.** `_allow_legacy_name_agent_match` allows same-name approval matching for `RunState` schema `< 1.7` (`src/agents/run_internal/turn_resolution.py:1132-1144`); schema `>= 1.7` requires object identity.

## Future Considerations

- **Decompose `AgentRunner.run` body further.** `AGENTS.md:54-67` prescribes extracting more helpers into `run_internal/`; the current body still contains substantial inline session/sandbox/server-conversation logic (`src/agents/run.py:828-839, 1027-1029, 1151-1158, 1283-1360`).
- **Unify streaming and non-streaming single-turn paths.** `AGENTS.md:58-66` already requires parity; reducing duplication could lower the maintenance cost of the two `run_single_turn*` functions.
- **Realtime state machine parity.** Realtime sessions have their own approval state and `ToolApprovalItem` lifecycle but do not benefit from `RunState`'s schema versioning or `to_state()` round-trip. Sharing the durability surface across realtime and turn-loop runs would be a sizeable API change.
- **Cycle / repeat-tool detection.** No evidence of a fingerprint-based cycle detector inside the loop was found (`src/agents/run.py:768-1497`). The `AgentToolUseTracker` (`src/agents/run_internal/tool_use_tracker.py`) tracks tool usage but does not gate the loop; an infinite alternating-tool loop is bounded only by `max_turns`.
- **Voice workflow beyond `SingleAgentVoiceWorkflow`.** The base class leaves multi-`Runner.run` composition to user code (`src/agents/voice/workflow.py:13-33`). A built-in helper for sequential / parallel voice workflows could reduce authoring cost.
- **Codex resume durability.** `Thread` does not currently serialize its `thread_id` into a durable artifact analogous to `RunState`. A `to_state()` for Codex would let long-lived Codex sessions be paused and resumed across processes, matching the turn-loop semantics.

## Questions / Gaps

- **No clear evidence found** of a cycle / repetition detector inside `AgentRunner.run` beyond `max_turns`. Search boundary: `src/agents/run.py:768-1497`, `src/agents/run_internal/run_loop.py`, `src/agents/run_internal/turn_resolution.py`. The `AgentToolUseTracker` records tool usage (`src/agents/run_internal/tool_use_tracker.py`) but is informational only.
- **No unified lifecycle hooks** for the realtime session that mirror the turn loop's `RunHooks` (`src/agents/lifecycle.py`). The realtime session exposes `RealtimeSessionEvent`s but does not provide a `RunHooks`-equivalent lifecycle callback surface. (Search boundary: `src/agents/realtime/session.py`, `src/agents/realtime/agent.py`.)
- **No shared schema version** for Codex `Thread` resume. `RunState` has `CURRENT_SCHEMA_VERSION = "1.11"` (`src/agents/run_state.py:131`), but Codex resume relies on the subprocess's own `thread_id` only (`src/agents/extensions/experimental/codex/thread.py:155-158`).
- **Voice workflow text accumulation.** `SingleAgentVoiceWorkflow._input_history` is updated only after a full turn (`src/agents/voice/workflow.py:99-101`); if the workflow raises mid-turn, the partial transcript is lost. Search boundary: `src/agents/voice/workflow.py`.
- **Sandbox-only runtime paths** may not all be reachable from `AgentRunner.run` without specific `RunConfig` setup. The visibility of `SandboxRuntime.enabled` branches (`src/agents/run.py:746, 780, 1210`) makes the conditional logic explicit, but the "default" path for ordinary agents is documented in `AGENTS.md` rather than enforced statically.

---

Generated by `01.01-execution-model-taxonomy` against `openai-agents-sdk`.
