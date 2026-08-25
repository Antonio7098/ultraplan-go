# Source Analysis: openai-agents-sdk

## 01.08: Streaming Execution Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (3.10+), asyncio, OpenAI Responses API / Chat Completions adapter, structured events over `asyncio.Queue` |
| Analyzed | 2026-07-03 |

## Summary

The OpenAI Agents SDK implements streaming as an **explicit three-tier event protocol** built on top of the OpenAI Responses streaming primitives. The model layer (`src/agents/models/openai_responses.py:527-619` and `src/agents/models/openai_chatcompletions.py:261-308`) yields raw `ResponseStreamEvent` objects (`TResponseStreamEvent`, `src/agents/items.py:79`) token-by-token and item-by-item. The runner (`src/agents/run_internal/run_loop.py:1483-1625`) forwards every raw event into a background `_event_queue` while simultaneously converting boundary events into semantic `RunItemStreamEvent` records when an output item reaches `ResponseOutputItemDoneEvent` (`run_loop.py:1529-1625`). The consumer (`src/agents/result.py:696-779`) drains that queue via `RunResultStreaming.stream_events()` until a `QueueCompleteSentinel` is observed (`src/agents/run_internal/run_steps.py` sentinel type).

Cancellation, retries, and partial-output rollback are first-class: `RunResultStreaming.cancel()` exposes `immediate` and `after_turn` modes (`result.py:648-694`); the streaming retry helper `stream_response_with_retry` in `src/agents/run_internal/model_retry.py:610-724` distinguishes retry-safe events (`response.created`, `response.in_progress`, `model_retry.py:44`) from retry-unsafe ones, and once any unsafe event is observed it flips `emitted_retry_unsafe_event = True` (`model_retry.py:656-657`) to block further retries; session and server-tracker state are rewound via `rewind_session_items` (`src/agents/run_internal/session_persistence.py:416-499`) before the next attempt. Partial-output dedupe (`run_loop.py:1665-1703`) prevents the streaming path from emitting items a second time through the non-streaming `stream_step_items_to_queue` step (`src/agents/run_internal/streaming.py:28-65`).

Interruption is modelled as a typed `ToolApprovalItem` (`src/agents/items.py:508-554`) and a `NextStepInterruption` step that *does not* emit a stream event (`src/agents/run_internal/streaming.py:56-57` explicitly skips approvals); the run finalises via `QueueCompleteSentinel` with `result.interruptions` populated (`result.py:499-500`, `run_loop.py:419-434`, `run_loop.py:1126-1150`). Guardrails use a separate `_input_guardrail_queue` (`result.py:486-488`) and only the *first* turn runs input guardrails (`run_loop.py:672-676`).

> Does streaming give responsiveness without corrupting state? **Yes for happy paths and explicit interruption; partially for arbitrary stream failures.** Cancellation is safe (immediate and after_turn), retries are blocked as soon as a non-safe event is emitted, and approval interruptions cleanly hand off via `RunState`. The weak points are: (1) there is no in-band resume of a half-completed raw stream — failure between `response.created` and `response.completed` aborts the whole step; (2) guardrails run on the final response only, not on streamed deltas; (3) the queue is unbounded (`asyncio.Queue`, `result.py:483-485`) so a fast producer vs. slow consumer cannot backpressure the upstream model.

## Rating

**7/10** — Clear, well-tested model with explicit event hierarchy, dual-mode cancellation, retry-unsafe event tracking, and typed interruption objects. Just below "mature" because (a) no resume of a partially-consumed raw stream — the runner simply aborts when the network fails mid-step; (b) output guardrails still race against the consumer via a free task (`run_loop.py:371-388`) rather than being streamed through the same queue, so they can fire *after* the consumer thinks the run is done; (c) the runner treats `_input_guardrails_task` exceptions as **post-hoc** (re-checked in `result.py:820-826` via `_check_errors`); (d) streaming events are only persisted indirectly through session snapshots, with `_current_turn_persisted_item_count` (`result.py:503-505`, `run_loop.py:317-329`) bookkeeping that can drift if a soft-cancel interrupts mid-persist. Tests cover the major paths: `tests/test_cancel_streaming.py`, `tests/test_soft_cancel.py`, `tests/test_stream_events.py`, `tests/test_streaming_tool_call_arguments.py`, `tests/test_streamed_terminal_output_backfill.py`, `tests/test_stream_input_guardrail_timing.py`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Stream event union type | `StreamEvent: TypeAlias = RawResponsesStreamEvent | RunItemStreamEvent | AgentUpdatedStreamEvent` | `src/agents/stream_events.py:61` |
| Raw passthrough event | `RawResponsesStreamEvent(data: TResponseStreamEvent, type="raw_response_event")` — the LLM event goes through verbatim | `src/agents/stream_events.py:10-19` |
| Semantic run-item event | `RunItemStreamEvent` with a closed set of `name`s: `message_output_created`, `handoff_requested`, `handoff_occured` (misspelled for backward compat), `tool_called`, `tool_search_called`, `tool_search_output_created`, `tool_output`, `reasoning_item_created`, `mcp_approval_requested`, `mcp_approval_response`, `mcp_list_tools` | `src/agents/stream_events.py:23-48` |
| Agent-change event | `AgentUpdatedStreamEvent(new_agent, type="agent_updated_stream_event")` — emitted on handoff / resume | `src/agents/stream_events.py:51-58` |
| Stream type alias from OpenAI SDK | `TResponseStreamEvent = ResponseStreamEvent` | `src/agents/items.py:79` |
| Approval item type (not emitted as stream event) | `ToolApprovalItem(raw_item, tool_name, tool_namespace, tool_lookup_key)` | `src/agents/items.py:508-554` |
| Compaction item (not emitted as stream event) | `CompactionItem` — session bookkeeping, silently skipped | `src/agents/stream_internal/streaming.py:58-59` (semantic skip), `src/agents/run_internal/streaming.py:58-59` |
| Step-to-event converter | `stream_step_items_to_queue` — maps each `RunItem` to a `RunItemStreamEvent`; explicitly skips `ToolApprovalItem` and `CompactionItem` with `event = None` | `src/agents/run_internal/streaming.py:28-65` |
| Wrapper over step result | `stream_step_result_to_queue(step_result, queue)` — calls the per-item helper with `step_result.new_step_items` | `src/agents/run_internal/streaming.py:68-73` |
| Background queue used by the run loop | `asyncio.Queue[StreamEvent | QueueCompleteSentinel]` initialised on `RunResultStreaming` | `src/agents/result.py:483-485` |
| Separate input-guardrail queue | `asyncio.Queue[InputGuardrailResult]` on `RunResultStreaming` | `src/agents/result.py:486-488` |
| Streaming entry point on Runner | `Runner.run_streamed(...)` — returns `RunResultStreaming`, starts `start_streaming` as `asyncio.create_task` | `src/agents/run.py:365-441` |
| Background-task kick-off | `streamed_result.run_loop_task = asyncio.create_task(start_streaming(...))` | `src/agents/run.py:1855-1873` |
| Streaming loop | `start_streaming` — manages `current_turn`, sandbox prep, input guardrails, `run_single_turn_streamed`, persisted items | `src/agents/run_internal/run_loop.py:440-1239` |
| Raw event forwarding into queue | `streamed_result._event_queue.put_nowait(RawResponsesStreamEvent(data=event))` for every event from `retry_stream` | `src/agents/run_internal/run_loop.py:1484` |
| Token-level visibility | Only `response.completed` (`ResponseCompletedEvent`) is treated as terminal inside the loop; all other events are passed through unchanged | `src/agents/run_internal/run_loop.py:1487-1490` |
| Failure events raise immediately | `response.incomplete`, `response.failed`, `error`, `response.error` raise `response_terminal_failure_error` / `response_error_event_failure_error` so partial streams abort the turn | `src/agents/run_internal/run_loop.py:1491-1499` |
| Per-item boundary emit (tool calls) | `ResponseOutputItemDoneEvent` → dedupe by `call_id` via `emitted_tool_call_ids` set → build `ToolCallItem` → emit `RunItemStreamEvent(name="tool_called")` | `src/agents/run_internal/run_loop.py:1529-1614` |
| Per-item boundary emit (reasoning) | `ResponseReasoningItem` → dedupe by `id` → `RunItemStreamEvent(name="reasoning_item_created")` | `src/agents/run_internal/run_loop.py:1616-1625` |
| Per-item boundary emit (tool search) | `tool_search_call` / `tool_search_output` → fingerprint-based dedupe via `emitted_tool_search_fingerprints` | `src/agents/run_internal/run_loop.py:1299-1309`, `1534-1556` |
| Backfill of terminal empty output | `if is_completed_event and not terminal_response.output and streamed_response_output: terminal_response.output = list(streamed_response_output)` — protects against providers that emit items only via `item.done` | `src/agents/run_internal/run_loop.py:1502-1506` |
| Partial-output dedupe after step resolution | After `get_single_step_result_from_response`, items whose IDs were already streamed are filtered out, then `stream_step_result_to_queue` is invoked with the filtered set | `src/agents/run_internal/run_loop.py:1665-1704` |
| Stream interrupt via approval | `NextStepInterruption` step → `_finalize_streamed_interruption` sets `interruptions`, `_last_processed_response`, `is_complete`, puts `QueueCompleteSentinel` | `src/agents/run_internal/run_loop.py:419-434`, `1126-1150` |
| Approval items excluded from queue | `elif isinstance(item, ToolApprovalItem): event = None  # approvals represent interruptions, not streamed items` | `src/agents/run_internal/streaming.py:56-57` |
| Interruption state preservation | `run_state._current_step = turn_result.next_step`, `run_state._model_responses`, `run_state._last_processed_response`, `run_state._current_turn_persisted_item_count` | `src/agents/run_internal/run_loop.py:1130-1140` |
| Cancelled sentinel delivery | `streamed_result._event_queue.put_nowait(QueueCompleteSentinel())` always called on path exit (success, error, finally) | `src/agents/run_internal/run_loop.py:299`, `416`, `667`, `844`, `955`, `970`, `1111`, `1163`, `1177`, `1198`, `1239` |
| Cancellation API surface | `RunResultStreaming.cancel(mode: "immediate" | "after_turn" = "immediate")` | `src/agents/result.py:648-694` |
| Immediate-cancel cleanup | `_cleanup_tasks()` cancels `run_loop_task`, `_input_guardrails_task`, `_output_guardrails_task`; `is_complete = True`; drains `_input_guardrail_queue`; pushes `QueueCompleteSentinel`; drains event queue if no consumer is waiting | `src/agents/result.py:677-688` |
| Soft-cancel (`after_turn`) | Just sets `_cancel_mode = "after_turn"`; the streaming loop checks the flag at three sites | `src/agents/result.py:690-694` |
| Soft-cancel check site (between turns) | `if streamed_result._cancel_mode == "after_turn": ... break` after the resumed-interrupt branch and after each turn | `src/agents/run_internal/run_loop.py:842-845`, `1109-1112`, `1161-1164` |
| Consumer waits on the queue | `await self._event_queue.get()`; handles `asyncio.CancelledError` by calling `self.cancel()` and re-raising | `src/agents/result.py:725-731` |
| Sentinel handling | On `QueueCompleteSentinel`, await the input-guardrail task safely and check for late exceptions via `_check_errors` | `src/agents/result.py:735-745`, `793-837` |
| Final cleanup contract | After the loop: `await self._await_task_safely(self.run_loop_task)` → `_cleanup_tasks()` → `_run_sandbox_cleanup()` → drain queues → re-raise any stored exception | `src/agents/result.py:749-779` |
| Exception surface | Stored exceptions raised *after* the consumer drains the queue so consumers see every event before failing | `src/agents/result.py:778-779` |
| Late-task exception capture | `_check_errors()` inspects `run_loop_task.exception()`, `_input_guardrails_task.exception()`, `_output_guardrails_task.exception()` post-completion | `src/agents/result.py:793-837` |
| Late max-turns check | `_check_errors()` synthesises `MaxTurnsExceeded` from `self.current_turn > self.max_turns` if not already handled | `src/agents/result.py:794-801` |
| Streaming retry helper | `stream_response_with_retry(get_stream, rewind, retry_settings, get_retry_advice, previous_response_id, conversation_id, failed_retry_attempts_out)` | `src/agents/run_internal/model_retry.py:610-724` |
| Retry-safe event types | `_RETRY_SAFE_STREAM_EVENT_TYPES = frozenset({"response.created", "response.in_progress"})` | `src/agents/run_internal/model_retry.py:44` |
| Block-retry detection | `_stream_event_blocks_retry(event)` flips `emitted_retry_unsafe_event = True` for any event whose type is not in the safe set | `src/agents/run_internal/model_retry.py:365-367`, `656-657` |
| Retry-after-policy evaluation | `_evaluate_retry(...)` short-circuits to `retry=False` when `emitted_retry_unsafe_event` or `provider_advice.replay_safety == "unsafe"` or `is_abort` | `src/agents/run_internal/model_retry.py:370-433` |
| Retry-rewind coupling | Before each retry attempt the runner calls `rewind_model_request` which calls `rewind_session_items(session, items_to_rewind, server_conversation_tracker)` | `src/agents/run_internal/run_loop.py:1452-1456` |
| Server-tracker rewind for stateful requests | `server_conversation_tracker.rewind_input(filtered.input)` is invoked inside `rewind_model_request` | `src/agents/run_internal/run_loop.py:1452-1456` |
| Stateful replay-safety gating | Stateful requests (with `previous_response_id` or `conversation_id`) refuse retries unless the policy explicitly approves replay, even on the first attempt | `src/agents/run_internal/model_retry.py:436-491` |
| Conversation-locked compatibility | Up to 3 automatic retries on `conversation_locked` errors with exponential backoff `1.0 * (2 ** (n-1))` | `src/agents/run_internal/model_retry.py:43`, `550-570`, `670-688` |
| Stream backpressure notes | `_event_queue` is a default `asyncio.Queue` (unbounded) | `src/agents/result.py:483-485` |
| Backpressure context | Pulled under `provider_managed_retries_disabled(...)` and `websocket_pre_event_retries_disabled(...)` contexts; yielded outside | `src/agents/run_internal/model_retry.py:641-651` |
| Persisted-stream bookkeeping | `_current_turn_persisted_item_count`, `_stream_input_persisted`, `_original_input_for_persistence` | `src/agents/result.py:503-512` |
| Streamed-input persist guard | Persist input to session only once per run via `_stream_input_persisted` | `src/agents/run_internal/run_loop.py:1408-1423` |
| Session save per turn | `_save_stream_items_with_count` / `_save_stream_items_without_count` callable closures captured at loop start, invoked on each `NextStep*` branch | `src/agents/run_internal/run_loop.py:629-655`, `1083-1158` |
| Persist-after-final-output | `_finalize_streamed_final_output` runs output guardrails, sets `is_complete`, persists items, pushes `QueueCompleteSentinel` | `src/agents/run_internal/run_loop.py:391-416` |
| Persist-after-interrupt | `_finalize_streamed_interruption` persists items and pushes `QueueCompleteSentinel`; transitions the stream to interrupted state | `src/agents/run_internal/run_loop.py:419-434` |
| Input guardrail concurrency | `_input_guardrails_task = asyncio.create_task(run_input_guardrails_with_queue(...))` runs them in parallel; results flow through `_input_guardrail_queue` | `src/agents/run_internal/run_loop.py:985-995`, `src/agents/run_internal/guardrails.py:54-107` |
| Input guardrails limited to first turn / first agent | `all_input_guardrails = (starting_agent.input_guardrails + ...) if current_turn == 0 and not is_resumed_state else []` | `src/agents/run_internal/run_loop.py:672-676` |
| Output guardrails race against consumer | `_output_guardrails_task = asyncio.create_task(run_output_guardrails(...))` created and awaited inside `_finalize_streamed_final_output` | `src/agents/run_internal/run_loop.py:363-388`, `403-409` |
| Output-guardrail result capture | `streamed_result.output_guardrail_results = output_guardrail_results` set inside `_finalize_streamed_final_output` | `src/agents/run_internal/run_loop.py:410` |
| Pending-approval surfacing in result | `streamed_result.interruptions = interruptions` set inside `_complete_stream_interruption` | `src/agents/run_internal/run_loop.py:296-299` |
| Resume via `RunState` | `RunResultStreaming.to_state()` → `Runner.run_streamed(state)` reconstructs `_state`, `_conversation_id`, `_previous_response_id`, `_auto_previous_response_id`, `_current_turn`, `_current_turn_persisted_item_count` | `src/agents/result.py:888-935`, `src/agents/run.py:1667-1831` |
| Drain-before-raise flag | `_mark_error_to_drain_stream_events(error)` / `_should_drain_stream_events_before_raising(error)` allow certain exceptions to surface only after queue is drained | `src/agents/exceptions.py:19-27`, `src/agents/result.py:709-720` |
| Consumer-visible raw event passthrough (Responses) | `async for chunk in stream: ... yield chunk` — every `ResponseStreamEvent` is yielded including `response.output_text.delta` | `src/agents/models/openai_responses.py:562-591` |
| Consumer-visible raw event passthrough (Chat Completions) | `ChatCmplStreamHandler.handle_stream` synthesises `ResponseCreatedEvent`, `ResponseInProgressEvent`, `ResponseTextDeltaEvent`, `ResponseFunctionCallArgumentsDeltaEvent`, `ResponseOutputItemDoneEvent`, etc. — so non-Responses adapters still emit Responses-shaped stream events | `src/agents/models/chatcmpl_stream_handler.py:265-308`, `515`, `602`, `748-828` |
| Async-iterator cleanup on cancel | `_schedule_async_iterator_close` creates a background task that calls `aclose` (or `close`) on the underlying stream | `src/agents/models/openai_responses.py:422-444`, `592-595` |
| Mid-stream `CancelledError` handling | `except asyncio.CancelledError: close_stream_in_background = True; self._schedule_async_iterator_close(stream); raise` | `src/agents/models/openai_responses.py:592-606` |
| Handoff emits `AgentUpdatedStreamEvent` | On `NextStepHandoff`: `streamed_result._event_queue.put_nowait(AgentUpdatedStreamEvent(new_agent=current_agent))` | `src/agents/run_internal/run_loop.py:811-813`, `1103-1105` |
| Per-turn agent-start hooks (streaming) | `await asyncio.gather(hooks.on_agent_start(...), public_agent.hooks.on_start(...))` only when `should_run_agent_start_hooks` is true | `src/agents/run_internal/run_loop.py:1317-1331` |
| Per-turn LLM hooks (streaming) | `hooks.on_llm_start` / `public_agent.hooks.on_llm_start` gathered before `stream_response`; `hooks.on_llm_end` / `public_agent.hooks.on_llm_end` gathered after `final_response` is constructed | `src/agents/run_internal/run_loop.py:1394-1406`, `1629-1636` |
| Sandbox cleanup after streamed run | `ensure_sandbox_cleanup_on_completion` registers a done-callback that creates a background cleanup task | `src/agents/result.py:593-621` |
| OpenAI Responses terminal-event handling | `ResponseCompletedEvent`, `response.failed`, `response.incomplete`, `error`, `response.error` flagged as `yielded_terminal_event = True` to suppress stream-cleanup errors after them | `src/agents/models/openai_responses.py:583-606` |

## Answers to Dimension Questions

1. **What is emitted while execution is still running?**
   - Every raw OpenAI Responses event wrapped in `RawResponsesStreamEvent` is put on the queue *as soon as the provider yields it* (`src/agents/run_internal/run_loop.py:1484`), including `response.created`, `response.in_progress`, `response.output_text.delta`, `response.function_call_arguments.delta`, `response.output_item.added/done`, `response.reasoning_summary_text.delta`, and `response.completed`.
   - Semantic boundary events fire only when the stream reaches the corresponding `ResponseOutputItemDoneEvent` — `message_output_created`, `tool_called`, `tool_output`, `reasoning_item_created`, `handoff_requested`, `handoff_occured`, `tool_search_called`, `tool_search_output_created`, `mcp_approval_requested`, `mcp_approval_response`, `mcp_list_tools` (`src/agents/run_internal/run_loop.py:1529-1625`).
   - `AgentUpdatedStreamEvent` is emitted at handoff (`run_loop.py:811-813`, `1103-1105`) and on the very first turn (`run_loop.py:589`).
   - Approval items are *never* streamed; they surface as `result.interruptions` (`run_loop.py:419-434`).

2. **Are partial outputs trusted?**
   - Token deltas and reasoning deltas are passed through verbatim and *not* validated by the runner — the consumer sees whatever the provider emits.
   - Tool calls and reasoning items are deduplicated by id (`run_loop.py:1280-1282`, `1566-1625`); once emitted, the id is recorded so a retry or duplicate item is not emitted twice.
   - Compaction items, approval items, and handoff-call items are explicitly suppressed from the stream because they are either session bookkeeping or require user action (`src/agents/run_internal/streaming.py:56-62`).
   - Tool-call argument empty-string regression is covered by `tests/test_streaming_tool_call_arguments.py` (issue #1629) — the SDK now waits for the populated `ResponseOutputItemDoneEvent` to construct `ToolCallItem` so consumers do not see a `tool_called` event with empty arguments.

3. **Can a failed stream resume?**
   - Mid-step (between events) failures retry via `stream_response_with_retry` if and only if no retry-unsafe event has been observed (`model_retry.py:656-657`, `370-433`).
   - Once `response.incomplete`, `response.failed`, `response.error`, or any output-item / text-delta event has been yielded, retries are blocked.
   - Stateful requests (with `previous_response_id` / `conversation_id`) require explicit `provider_advice.replay_safety == "safe"` or a policy that approves replay (`model_retry.py:412-419`).
   - Before each retry, the session is rewound via `rewind_session_items` (`session_persistence.py:416-499`) and the server-conversation tracker rewind method is invoked (`run_loop.py:1452-1456`).
   - A whole-step failure (after a retry-unsafe event) does *not* resume — the step aborts and the runner either retries the next turn or fails the run.

4. **Can a user stop the stream safely?**
   - Yes, via `result.cancel(mode="immediate" | "after_turn")` (`result.py:648-694`).
   - `immediate` cancels the run-loop task and the input/output guardrail tasks, drains queues, pushes `QueueCompleteSentinel`, and marks `is_complete = True` (`result.py:677-688`).
   - `after_turn` defers the stop until the current turn completes; the streaming loop checks `_cancel_mode == "after_turn"` at three sites (`run_loop.py:842-845`, `1109-1112`, `1161-1164`).
   - Internal model stream cleanup uses `_schedule_async_iterator_close` so an aborted HTTP request does not leak the underlying `AsyncIterator` (`openai_responses.py:422-444`, `592-595`).
   - The consumer awaits the run-loop task safely (`_await_task_safely` swallows `CancelledError`, `result.py:852-866`) so cancellation does not surface as an unhandled `CancelledError` to the caller.

5. **Does streaming weaken guardrails?**
   - Input guardrails are run only on the **first** turn and only for the **starting** agent (`run_loop.py:672-676`); on resume they are skipped entirely. They run in a parallel task and emit results through `_input_guardrail_queue` (`guardrails.py:54-107`); the consumer re-checks them post-loop (`result.py:804-810`).
   - Output guardrails are launched as a task and awaited inside `_finalize_streamed_final_output` (`run_loop.py:363-388`); they are not streamed event-by-event — they only fire on the *final* output.
   - The drain-before-raise flag (`exceptions.py:19-27`, `result.py:709-720`) lets certain exceptions (`MaxTurnsExceeded`, guardrail tripwires marked via `_mark_error_to_drain_stream_events`) surface only after the queue is drained so consumers do not miss semantic events that arrived immediately before the error.
   - Soft-cancel preserves guardrail results because the cancellation checks happen **between** turns (`run_loop.py:842-845`), so a first-turn guardrail that already produced a result is not lost.

## Architectural Decisions

- **Two-tier event hierarchy** (`RawResponsesStreamEvent` + `RunItemStreamEvent` + `AgentUpdatedStreamEvent`): the SDK keeps raw provider events for low-level consumers (e.g. token streaming) and exposes semantic boundary events for higher-level UI. The two streams are interleaved through a single queue (`src/agents/run_internal/run_loop.py:1483-1625`).
- **Single `asyncio.Queue` per stream** (`_event_queue` plus a separate `_input_guardrail_queue`) — chosen over a callback model for back-pressure-free consumption; the consumer simply awaits `queue.get()` and yields.
- **Dual cancellation modes** (`immediate`, `after_turn`) — `immediate` cancels tasks and clears state for an abrupt stop; `after_turn` is a soft flag the run loop polls between turns to deliver a consistent run state (usage, persisted items, completed tool calls).
- **Retry-unsafe event gating** (`_RETRY_SAFE_STREAM_EVENT_TYPES`) — the runner decides retry eligibility per-event rather than per-stream. Only `response.created` and `response.in_progress` are safe to replay; any other yielded event freezes the stream and forces a fresh request.
- **Server-managed conversation state vs. session persistence** — when `previous_response_id` or `conversation_id` is set, the runner routes retries through `OpenAIServerConversationTracker` and disables session persistence (`run_loop.py:609-614`); this avoids double-recording conversation history that the server already owns.
- **Streaming approval interrupts** as `ToolApprovalItem` exposed via `result.interruptions` (not as stream events) — separation of "is being generated" from "is awaiting human action" so consumers do not have to filter a single stream for action items.
- **Tool-call argument backfill** (`run_loop.py:1502-1506`) — the runner preserves `streamed_response_output` even if the terminal `response.completed` event has empty output, so the step resolver can still construct the right `RunItem`s for downstream tool execution.

## Notable Patterns

- **`QueueCompleteSentinel` fan-out** — placed on the queue at every terminal site (final output, interrupt, max-turns, error, finally), so the consumer never blocks indefinitely even when the run loop raises (`run_loop.py:299`, `416`, `667`, `844`, `955`, `970`, `1111`, `1163`, `1177`, `1198`, `1239`).
- **Dedupe sets keyed on item identity** — `emitted_tool_call_ids`, `emitted_reasoning_item_ids`, `emitted_tool_search_fingerprints` (`run_loop.py:1280-1282`) keep the streamed and the non-streamed code paths in sync after `get_single_step_result_from_response` (`run_loop.py:1665-1703`).
- **Streaming-vs-non-streaming parity** — the AGENTS.md policy (`AGENTS.md:21-24`) requires that changes to `run_loop.py` be mirrored between `run_single_turn` and `run_single_turn_streamed`. The dedupe sets above are the concrete mechanism that keeps the two paths behaviourally aligned.
- **Retry-aware async-iterator close** — `_schedule_async_iterator_close` (`openai_responses.py:434-436`) creates a fire-and-forget task that calls `aclose` on the stream when `asyncio.CancelledError` interrupts mid-iteration.
- **Stream-task failure capture** — `_check_errors` (`result.py:793-837`) reads `task.exception()` from each background task after the consumer loop ends, surfacing failures that would otherwise be silently swallowed.
- **Drain-before-raise attribute** on `AgentsException` (`exceptions.py:19-27`) — exceptions opt into a contract where they are only raised after the consumer has drained remaining events.
- **`is_resumed_state` branch** in `start_streaming` (`run_loop.py:730-840`) — when resuming from a `RunState`, the runner replays the items already streamed (`stream_step_items_to_queue(list(turn_session_items), streamed_result._event_queue)`) so the consumer sees continuity across resumption.

## Tradeoffs

- **Unbounded queue (`asyncio.Queue`)** — chosen for simplicity; under a fast producer / slow consumer, memory can grow and the runner cannot signal the provider to slow down.
- **Output guardrails run as a single async task awaited at finalisation** — keeps guardrail code reusable with the non-streaming path, but they are *not* streamed to the consumer, so consumers observing `output_guardrail_results` only see them after `final_output` is set.
- **Retry-unsafe semantics block all retries after the first non-safe event** — simple, safe, but means an unreliable network 100 ms into a long stream will abort the entire turn.
- **Input guardrails limited to first turn + starting agent** — matches the non-streaming path (`AGENTS.md:19`) but means a multi-agent handoff cannot re-validate the original user input at every hop.
- **Compaction items skipped silently** (`streaming.py:58-59`) — avoids log noise but means consumers have no way to observe compaction activity on the live stream; only `result.new_items` reflects them.
- **Cancellation behavior of "after_turn"** is checked only at three loop sites (`run_loop.py:842-845`, `1109-1112`, `1161-1164`) — within a single turn (e.g., during tool execution) the flag is not polled; if a tool takes a long time the user has no way to abort it short of `immediate` cancel.

## Failure Modes / Edge Cases

- **Mid-stream network drop** — once a retry-unsafe event has been yielded, retries are blocked (`model_retry.py:656-657`); the turn fails and the consumer sees the exception only after the queue is drained.
- **`asyncio.CancelledError` propagation** — the consumer's `CancelledError` triggers `cancel()` and re-raises (`result.py:728-731`), so a consumer that times out on `await stream_events()` will not silently leave a run loop running.
- **Late guardrail exceptions** — if the input guardrail task raises after the sentinel has been delivered, `_check_errors` re-raises it post-loop (`result.py:820-826`); the consumer must keep draining to see them.
- **Queue cleanup on cancellation** — `immediate` cancel drains both queues and pushes a sentinel if no consumer is waiting (`result.py:682-688`); a consumer that arrives after an immediate cancel still sees `is_complete = True` and an empty queue.
- **Sandbox cleanup after streamed run** — `ensure_sandbox_cleanup_on_completion` registers a done-callback that spawns a background cleanup task (`result.py:593-621`); exceptions in cleanup are logged but never re-raised.
- **Terminal-event stream cleanup errors** — if the underlying stream raises on close *after* `response.completed`, the error is suppressed (`openai_responses.py:601-606`); otherwise it is re-raised.
- **Streamed-tool-call arguments regression** (issue #1629) — guarded by `tests/test_streaming_tool_call_arguments.py`; the SDK now reads `arguments` from `ResponseOutputItemDoneEvent` rather than the early `ResponseOutputItemAddedEvent` so consumers do not see empty argument strings.
- **Server-managed conversation retries** — `OpenAIServerConversationTracker` rewind is gated by `mark_input_as_sent` (`run_loop.py:1389-1390`); if a retry fails before marking, a subsequent turn will re-send the same deltas.

## Future Considerations

- **Bounded queue + backpressure** — replacing the default `asyncio.Queue` with `Queue(maxsize=N)` plus a "slow down" signal to the provider would let consumers apply real backpressure.
- **Per-chunk guardrail evaluation** — currently guardrails only see the final output; exposing stream events through the same `guardrails.py` machinery would let operators enforce content policies token-by-token (the test `tests/test_stream_input_guardrail_timing.py` already demonstrates the streaming consumer can be aware of ordering).
- **Stream resume via provider cursor** — the SDK already tracks `previous_response_id` and `conversation_id`; a "resume from last safe event" mode would close the gap when a connection drops after a retry-unsafe event.
- **Per-tool-call streaming events** — the SDK currently emits a single `tool_called` event per completed tool-call item; for long JSON argument payloads, exposing `tool_call_arguments.delta` events (already in the raw stream) would let consumers render incremental tool calls.
- **Cancellation polling inside a turn** — adding a `mode="now"` flag or a periodic cancel check inside tool execution would close the long-tool gap.
- **Unified guardrail queue** — moving output guardrails onto `_input_guardrail_queue` (or a new `_output_guardrail_queue`) would let consumers observe guardrail progress in real time rather than only at finalisation.

## Questions / Gaps

- **No evidence found** for `LiteLLM`-equivalent streaming helpers outside the Responses / Chat Completions adapters; providers using the `Model` interface directly must yield `TResponseStreamEvent`-compatible events.
- **No evidence found** for explicit per-event authorisation hooks (the SDK relies on `ToolApprovalItem` / `MCPAprovalRequestItem` rather than a general "should this event be surfaced" hook).
- The relationship between `_current_turn_persisted_item_count` and `session.get_items()` after a soft-cancel has no dedicated test in the snapshot suite; the closest coverage is `tests/test_soft_cancel.py:99-122` (session save) and `tests/test_soft_cancel.py:439-477` (multi-turn with session).
- The behaviour of `cancel()` called **after** `stream_events()` has fully drained but **before** `_run_loop_task` has finished is not covered in the test suite (the closest test is `test_soft_cancel_mixed_modes` at `tests/test_soft_cancel.py:230-247`, which only switches mode before consumption).
- `TResponseStreamEvent` re-export (`src/agents/items.py:79`) means consumers depend on OpenAI SDK event shapes; a provider that emits non-Responses-shaped streams must run them through `ChatCmplStreamHandler.handle_stream` (`src/agents/models/chatcmpl_stream_handler.py:265`) to participate.

---

Generated by `01.08-streaming-execution-semantics` against `openai-agents-sdk`.