# Source Analysis: openai-agents-sdk

## Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio) with `pydantic`, `openai` SDK, Responses API |
| Analyzed | 2026-07-02 |

## Summary

The `openai-agents-sdk` defines a layered atomicity model. The persisted
pa/res boundary is the **turn**: a `SingleStepResult` (`src/agents/run_internal/run_steps.py:178`) bundles one model call, the side-effect tools that consume its tool calls, and one of four outcomes (`NextStepFinalOutput`, `NextStepHandoff`, `NextStepRunAgain`, `NextStepInterruption`). Persisted `RunState` snapshots (`src/agents/run_state.py:184`) carry `_current_turn`, `_current_step`, and `_last_processed_response` so a turn can be re-entered after a crash or tool-approval pause. The retried atomic unit is **the model request itself** (`get_response_with_retry`, `src/agents/run_internal/model_retry.py:511`), which rewind is wired to `rewind_session_items` (`src/agents/run_internal/session_persistence.py:416`) and `server_conversation_tracker.rewind_input` so the session and the server-managed `conversation_id` stay in sync with replay. The traced unit is a **span hierarchy** with explicit span data classes (`TaskSpanData`, `AgentSpanData`, `TurnSpanData`, `FunctionSpanData`, `GenerationSpanData`, `ResponseSpanData`, `HandoffSpanData`, `GuardrailSpanData` in `src/agents/tracing/span_data.py:28-451`); one `turn_span` wraps the model call and is sealed in a `finally` block (`src/agents/run.py:1273-1278`). The user-visible atomic unit is the same `SingleStepResult`, surfaced as a single `RunResult` (`src/agents/result.py:334`) for sync mode, or as a stream of `RunItemStreamEvent`s per item (`src/agents/stream_events.py:24`) plus an `AgentUpdatedStreamEvent` for agent transitions (`src/agents/stream_events.py:52`).

Tool calls are NOT their own atomic unit: they are not individually persisted, retried, or traced at a finer boundary than a turn. Function tools run concurrently under `max_function_tool_concurrency` (`src/agents/run_internal/tool_execution.py:1390-1448`); each tool produces a `ToolCallItem`/`ToolCallOutputItem` pair that is committed to the session with the rest of the turn's `new_step_items` (`src/agents/run_internal/turn_resolution.py:629-845`, `src/agents/run.py:1298-1360`). Interrupted (approval-requesting) turns can persist partial work and resume cleanly via `_current_turn_persisted_item_count` and `_last_processed_response` (`src/agents/run_internal/session_persistence.py:264-352`, `src/agents/run_state.py:263-264`).

## Rating

**8 / 10.** Clear atomicity model, explicit interfaces, documented schemas (12 released RunState schema versions, `src/agents/run_state.py:133-149`), and operational safeguards (rewind on retry, soft cancel, persisted-item counters). Deductions: tool calls can execute concurrently inside a turn and the SDK only partially persists them via a counter, so a crash between tool execution and session write can leave the turn in a known-but-non-atomic state, and the schema-per-turn-resume is narrowly serializable but the replay-safe path depends on the conversation-locked compatibility retries in `src/agents/run_internal/model_retry.py:551-570` plus the `rewind_session_items` tail matching (`src/agents/run_internal/session_persistence.py:653-713`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Step/turn vocabulary | `SingleStepResult` dataclass with `next_step` tagged union | `src/agents/run_internal/run_steps.py:178-218` |
| Step/turn vocabulary | Four-way `next_step` union: `NextStepHandoff`, `NextStepFinalOutput`, `NextStepRunAgain`, `NextStepInterruption` | `src/agents/run_internal/run_steps.py:154-174` |
| Step/turn vocabulary | `RunItem` discriminated union (12 item types) | `src/agents/items.py:639-654` |
| Step/turn vocabulary | `RunState._current_turn` and `_current_step` | `src/agents/run_state.py:200-255` |
| Step/turn vocabulary | `Runner.run` doc: "a turn is defined as one AI invocation (including any tool calls that might occur)" | `src/agents/run.py:240,329` |
| Step atomicity | `run_single_turn` produces exactly one `SingleStepResult` | `src/agents/run_internal/run_loop.py:1708-1795` |
| Step atomicity | `current_turn += 1` immediately before `run_single_turn_streamed` | `src/agents/run_internal/run_loop.py:875-876` |
| Step atomicity | Tool plan executed inside `execute_tools_and_side_effects` before the `SingleStepResult` returns | `src/agents/run_internal/turn_resolution.py:664-845` |
| Turn boundary | `current_turn_span.start(mark_as_current=True)` and `.finish(reset_current=True)` in `try/finally` | `src/agents/run.py:1166-1170`, `src/agents/run.py:1273-1278` |
| Turn boundary | Resume path detects `run_state._current_step` and forwards to `resolve_interrupted_turn` | `src/agents/run_internal/run_loop.py:730-748` |
| Persistence unit | `save_result_to_session` persists `turn_session_items` after the turn closes | `src/agents/run_internal/session_persistence.py:247-389` |
| Persistence unit | `_current_turn_persisted_item_count` tracks how many `new_items` were already committed in this turn | `src/agents/run_state.py:263-264` |
| Persistence unit | Partial persistence guards: `missing_outputs` re-inclusion at rewind | `src/agents/run_internal/session_persistence.py:274-281` |
| Persistence unit | `save_resumed_turn_items` used to backfill partial turns after resume | `src/agents/run_internal/session_persistence.py:392-413` |
| Persistence unit | RunState JSON schema contract (`CURRENT_SCHEMA_VERSION = "1.11"`, 12 released versions) | `src/agents/run_state.py:131-149` |
| Persistence unit | Schema-version-gated resume: `to_json()` always emits `CURRENT_SCHEMA_VERSION`, older SDKs reject unknown | `src/agents/run_state.py:124-130` |
| Persistence unit | `_last_processed_response` required to rebuild the in-turn tool plan on resume | `src/agents/run_state.py:257-258`, `src/agents/run_internal/run_steps.py:208-209` |
| Retry unit | `get_response_with_retry` retries the model call only and rewinds via `rewind_session_items` | `src/agents/run_internal/model_retry.py:511-608` |
| Retry unit | `_is_stateful_request` decides whether retry must go through runner-managed rewind (preserves `conversation_id`/`previous_response_id`) | `src/agents/run_internal/model_retry.py:432-441` |
| Retry unit | Compatibility retry for `conversation_locked` errors with up to `COMPATIBILITY_CONVERSATION_LOCKED_RETRIES` attempts | `src/agents/run_internal/model_retry.py:551-570` |
| Retry unit | `rewind_session_items` pops a serialized tail suffix matched by item fingerprint | `src/agents/run_internal/session_persistence.py:416-468` |
| Retry unit | `server_conversation_tracker.rewind_input` rewinds server-managed deltas | `src/agents/run_internal/run_loop.py:1876-1880` |
| Retry unit | `wait_for_session_cleanup` verifies the rewound items are actually gone | `src/agents/run_internal/session_persistence.py:518-557` |
| Retry unit | Retry policies with capability tagging (`retries_safe_transport_errors`, `retries_all_transient_errors`) | `src/agents/retry.py:125-360` |
| Trace unit | Span data classes: `TaskSpanData` (top-level), `AgentSpanData`, `TurnSpanData`, `FunctionSpanData`, `GenerationSpanData`, `ResponseSpanData`, `HandoffSpanData`, `GuardrailSpanData`, `MCPListToolsSpanData` | `src/agents/tracing/span_data.py:28-451` |
| Trace unit | `TaskSpanData` comment: "Represents one top-level Runner run" | `src/agents/tracing/span_data.py:64-95` |
| Trace unit | `TurnSpanData` comment: "Represents one agent loop turn" | `src/agents/tracing/span_data.py:98-132` |
| Trace unit | `get_current_trace` / `get_current_span` keyed by contextvar (`src/agents/tracing/spans.py:31`) | `src/agents/tracing/create.py:79-86` |
| Trace unit | Span start/finish contracts (`start(mark_as_current)`, `finish(reset_current)`) | `src/agents/tracing/spans.py:100-118` |
| Trace unit | `SpanError` typed dict attached to active span for failed ops | `src/agents/tracing/spans.py:19-25` |
| Trace unit | `with_tool_function_span` opens a `function_span` per local tool call | `src/agents/run_internal/tool_execution.py:1019-1038` |
| Trace unit | `GenerationSpanData` wraps each model call (input/output/model/usage) | `src/agents/tracing/span_data.py:169-209` |
| Trace unit | `TracingProcessor` lifecycle: `on_trace_start/on_trace_end/on_span_start/on_span_end` | `src/agents/tracing/processor_interface.py:9-104` |
| UI unit | `RunResult` exposes `new_items`, `raw_responses`, `final_output`, `interruptions` | `src/agents/result.py:176-368` |
| UI unit | `RunResultStreaming.current_turn` mirrors internal counter | `src/agents/result.py:455-458` |
| UI unit | `RunItemStreamEvent.name` enumerates all observable item types (with `handoff_occured` typo kept for compat) | `src/agents/stream_events.py:24-49` |
| UI unit | `AgentUpdatedStreamEvent` marks handoff boundaries | `src/agents/stream_events.py:52-58` |
| UI unit | `stream_step_items_to_queue` maps each `RunItem` to exactly one `RunItemStreamEvent` (skipping `ToolApprovalItem` and `CompactionItem` which are bookkeeping) | `src/agents/run_internal/streaming.py:28-65` |
| Tool atomicity | `_fill_tool_task_slots` schedules tool tasks under `max_function_tool_concurrency` | `src/agents/run_internal/tool_execution.py:1438-1448` |
| Tool atomicity | `_drain_pending_tasks` waits on tool tasks with `asyncio.FIRST_COMPLETED` | `src/agents/run_internal/tool_execution.py:1462-1478` |
| Tool atomicity | Approval results surface as `ToolApprovalItem` inside the turn, not a separate atomic step | `src/agents/items.py:509-636` |
| Tool atomicity | `ToolApprovalItem.to_input_item()` raises: "These items should be filtered out before preparing input for the API" | `src/agents/items.py:631-636` |
| Cancellation | Soft cancel (`after_turn`) waits for the turn to close, then exits | `src/agents/run_internal/run_loop.py:842-848`, `src/agents/run_internal/run_loop.py:1109-1112` |
| Cancellation | Hard cancel propagates as `asyncio.CancelledError` through the run loop; tool pool cancels pending siblings | `src/agents/run_internal/tool_execution.py:1426-1430` |
| Max turns guard | `max_turns` check before turn-start; synthesized final output via `error_handlers` | `src/agents/run_internal/run_loop.py:881-956` |
| Default budget | `DEFAULT_MAX_TURNS = 10` is the documented per-run safety budget | `src/agents/run_config.py:33` |
| Hook ordering | `on_llm_start` / `on_llm_end` bracket the model call; `on_tool_start` / `on_tool_end` bracket each local tool | `src/agents/lifecycle.py:18-103` |
| Multiplicity of items per turn | `next_step` is the only thing telling the outer loop whether to issue another turn | `src/agents/run_internal/run_steps.py:178-218` |
| Approval as atomic exception | `NextStepInterruption` carries a list of `ToolApprovalItem` and is a terminal state for the run until approval | `src/agents/run_internal/run_steps.py:169-174` |

## Answers to Dimension Questions

1. **What is the atomic unit of execution?**
   The atomic unit of execution is the **turn**: each cycle of `_run_single_turn[_streamed]` (`src/agents/run_internal/run_loop.py:1242`, `src/agents/run_internal/run_loop.py:1708`) produces exactly one `SingleStepResult` (`src/agents/run_internal/run_steps.py:178`) that combines one model call with the synchronous-or-concurrent local tool calls that consume its tool calls (`src/agents/run_internal/turn_resolution.py:664-735`). A turn ends with one of four `next_step` outcomes (`src/agents/run_internal/run_steps.py:154-174`); the public documentation reinforces this with "a turn is defined as one AI invocation (including any tool calls that might occur)" (`src/agents/run.py:240`).

2. **Is the atomic unit the same for persistence, tracing, retry, and UI?**
   Mostly yes — at the turn level — but the units diverge slightly:
   - **Persistence**: turns, via `save_result_to_session` (`src/agents/run_internal/session_persistence.py:247`) and `_current_turn_persisted_item_count` (`src/agents/run_state.py:263`).
   - **Tracing**: a strict span hierarchy of `Trace → TaskSpan → AgentSpan → TurnSpan → (GenerationSpan|FunctionSpan|HandoffSpan|GuardrailSpan)` (`src/agents/tracing/span_data.py:64-451`). `TaskSpanData` is the top-level run (`src/agents/tracing/span_data.py:64`); `TurnSpanData` is one turn (`src/agents/tracing/span_data.py:98`).
   - **Retry**: finer — the **model call** is the retry unit (`get_response_with_retry`, `src/agents/run_internal/model_retry.py:511`). A retry may rewind a session tail or a server-managed input delta (`src/agents/run_internal/model_retry.py:567`, `src/agents/run_internal/session_persistence.py:416`).
   - **UI**: turns, with two presentations. Sync exposes one `RunResult` (`src/agents/result.py:334`); streaming exposes a turn-by-turn stream of `RunItemStreamEvent` + `AgentUpdatedStreamEvent` events (`src/agents/stream_events.py:24-58`, `src/agents/run_internal/streaming.py:28-65`).
   So persistence, tracing, and UI agree on "turn", but retry is "model call within a turn" (intra-turn sub-unit).

3. **Can partially completed steps exist?**
   Yes, in two controlled forms:
   - **Within a turn**: pending sibling function tools may have failed while others succeeded (`src/agents/run_internal/tool_execution.py:1480-1499`). The executor cancels siblings on first failure and attaches callbacks to the leftover tasks.
   - **Across a turn boundary**: an interruption turn persists both the `ToolCallItem` for the not-yet-approved tool and the `ProcessedResponse` it came from, and tracks the persisted count (`src/agents/run_state.py:257-264`, `src/agents/run_internal/session_persistence.py:264-352`). On resume, `run_state._last_processed_response` lets the SDK avoid re-emitting items that already produced `tool_call_output_item`s (`src/agents/run.py:1298-1333`).
   Turn-level tool execution is "fail siblings, surface first failure" rather than "all-or-nothing" within a turn.

4. **What happens if a crash occurs mid-step?**
   - **`asyncio.CancelledError` mid-turn**: the streaming loop's outer `try/except/finally` runs (`src/agents/run_internal/run_loop.py:656-668`, `src/agents/run_internal/run_loop.py:1237-1239`), the trace/task spans are closed (`src/agents/run_internal/run_loop.py:1226-1235`), and `streamed_result.is_complete = True` is forced. The session store is NOT partially rolled back; whatever `_save_stream_items` already wrote stays.
   - **`get_response_with_retry` model failure**: the retry helper rewinds the session tail and, for stateful requests, the `OpenAIServerConversationTracker`'s "sent" state (`src/agents/run_internal/model_retry.py:567-569`, `src/agents/run_internal/run_loop.py:1876-1880`). If the policy says "do not retry", the exception is re-raised (`src/agents/run_internal/model_retry.py:592-593`).
   - **Hard crash mid-process**: `RunState.to_json()` snapshots a resumable `CURRENT_SCHEMA_VERSION = "1.11"` payload (`src/agents/run_state.py:131`). Older payloads load via `SCHEMA_VERSION_SUMMARIES` keyed by version (`src/agents/run_state.py:133-149`). After resume, `is_resumed_state` short-circuits input re-persistence and forwards to `resolve_interrupted_turn` (`src/agents/run_internal/run_loop.py:730-815`).
   - **Soft cancel (`after_turn`)**: lets the current turn close, then exits (`src/agents/run_internal/run_loop.py:842-848`, `src/agents/run_internal/run_loop.py:1109-1112`). Tests verify session is saved (`tests/test_soft_cancel.py:99-110`).

5. **Are tool calls their own atomic units?**
   No. Tool calls run concurrently inside a turn under `max_function_tool_concurrency` (`src/agents/run_internal/tool_execution.py:1390-1448`), each is bracketed by `on_tool_start`/`on_tool_end` hooks (`src/agents/lifecycle.py:70-103`) and wrapped in a `function_span` (`src/agents/run_internal/tool_execution.py:1033`), and the resulting `ToolCallItem` + `ToolCallOutputItem` pairs (`src/agents/items.py:347-419`) are persisted together with all other items from the same turn (`src/agents/run.py:1298-1360`, `src/agents/run_internal/session_persistence.py:302-326`). Their atomicity boundary aligns with the turn's session-save boundary, not with the tool call's own latency window. Interruptions (tool-approval items) are treated as a special "interrupted turn" outcome — `ToolApprovalItem` is filtered out of model input (`src/agents/items.py:631-636`) and from the streamer's per-item events (`src/agents/run_internal/streaming.py:56-57`), and exposed via the `interruptions` array on both result types (`src/agents/result.py:367-368`, `src/agents/result.py:499-500`).

## Architectural Decisions

- **Turn is the contract surface.** `SingleStepResult.next_step` (`src/agents/run_internal/run_steps.py:192`) is a 4-way tagged union (handoff / final output / run again / interruption). The streaming and non-streaming loops both consume that union via the same dispatch, keeping behaviour aligned (`src/agents/run.py:1367-1500`, `src/agents/run_internal/run_loop.py:1065-1175`).
- **Schema-gated durable resume.** `CURRENT_SCHEMA_VERSION` (`src/agents/run_state.py:131`) and `SCHEMA_VERSION_SUMMARIES` (`src/agents/run_state.py:133`) form a one-line changelog per version. Forward compatibility is intentionally fail-fast — older SDKs reject newer versions (`src/agents/run_state.py:128-130`).
- **Two persistence layers, one retry rewind.** The session store (`Session.add_items`, `src/agents/memory/session.py:36`) and the server-managed conversation store (`OpenAIServerConversationTracker`, `src/agents/run_internal/oai_conversation.py:1`) are kept in lockstep by routing their rewinds through the same `RewindCallable` on a model retry (`src/agents/run_internal/run_loop.py:1876-1880`, `src/agents/run_internal/session_persistence.py:416-468`).
- **Items, not text, are persisted.** `RunItem` (`src/agents/items.py:639`) is the canonical, replayable, restart-safe unit. `ItemHelpers.to_input_item`/`run_item_to_input_item` normalizes each item type before write so a rehydrated session can be replayed into the next turn (`src/agents/items.py:144-153`, `src/agents/run_internal/items.py:1`).
- **Interruption is a turn outcome, not a side channel.** `NextStepInterruption.interruptions` (`src/agents/run_internal/run_steps.py:169-174`) is the only signal that the run yielded to a human. `RunResult.interruptions` and `RunResultStreaming.interruptions` simply expose the same list (`src/agents/result.py:367`, `src/agents/result.py:499`).
- **`_last_processed_response` is mandatory for resume.** Without it, `Runner.run(state)` cannot reconstruct the pending tool plan after approval (`src/agents/run_internal/run_loop.py:732-733`).

## Notable Patterns

- **Tagged-union step outcome over bool/exception semantics.** `next_step` (`src/agents/run_internal/run_steps.py:154-174`) keeps four states statically typed; the outer loop uses `isinstance` checks (`src/agents/run_internal/run_loop.py:831-837`, `src/agents/run.py:1367-1394`, `src/agents/run.py:1415-1500`) instead of raising/catching.
- **Span-as-context.** `Span.start(mark_as_current=True)` registers the span in a contextvar (`src/agents/tracing/spans.py:100-108`); tool calls and model calls become children of whichever span is current, hiding the hierarchy plumbing from contributors (`src/agents/run_internal/tool_execution.py:1033`).
- **Persist-counter idempotence.** `_current_turn_persisted_item_count` (`src/agents/run_state.py:263`) plus the `run_state._last_processed_response` pair lets `save_result_to_session` skip items it already wrote and re-include orphan tool outputs that had no matching call persisted (`src/agents/run_internal/session_persistence.py:270-281`). See `tests/test_run_impl_resume_paths.py:43-200` for test coverage.
- **Compatibility-retry tier for `conversation_locked`.** Preserved historical behaviour with a small bounded retry ladder (`src/agents/run_internal/model_retry.py:551-570`) — not a generic retry pattern, but a tail call that only fires if the caller did not opt out via `max_retries=0` (`src/agents/run_internal/model_retry.py:444-453`).
- **Capability-tagged retry policies.** `_mark_retry_capabilities` decorates a policy with `_retries_safe_transport_errors` / `_retries_all_transient_errors` (`src/agents/retry.py:142-158`), letting the runtime decide whether provider-managed retries should be suppressed on replay (`src/agents/run_internal/model_retry.py:456-491`). Hidden retries cannot resend `conversation_id`-bound deltas before the runner's rewind/replay checks.
- **Soft cancel is preferred over abrupt cancellation.** Soft cancel (`after_turn`) is a default-blessed path, exercised in `tests/test_soft_cancel.py` and exposed via `Runner.cancel(mode="after_turn"|"immediate")` (`src/agents/run.py:496-498`).
- **Span closure in `finally`.** Every span opened in the run loop is closed in `finally` so a thrown exception or cancellation still emits a clean span to the trace backend (`src/agents/run.py:1273-1278`, `src/agents/run_internal/run_loop.py:1040-1045`, `src/agents/run_internal/run_loop.py:1226-1235`).

## Tradeoffs

- **Tool calls are aggregated, not individually atomic.** Concurrent function tools can finish out-of-order; only the first sibling failure cascades cancellation (`src/agents/run_internal/tool_execution.py:1480-1499`). A crash between "model responded" and "session has been written" leaves the turn in a known-but-non-atomic state; recovery requires `_current_turn_persisted_item_count` + `_last_processed_response` to reconstruct the boundary (`src/agents/run_state.py:257-264`).
- **Retry granularity is the model call, not the turn.** Required by server-managed conversations, but it means a turn with N tools will retry all N side effects if the *first* tool returned but the *second* model call within the same turn never gets to start. The SDK does not currently expose a "dedupe-on-replay" guarantee for tool side effects that already executed.
- **Schema versioning is fail-fast.** `to_json()` always emits `CURRENT_SCHEMA_VERSION` (`src/agents/run_state.py:128`) and SUPPORTED_SCHEMA_VERSIONS is a `frozenset` (`src/agents/run_state.py:150`). Downgrading an SDK after a schema bump loses the ability to resume the new snapshot. This is a deliberate posture, not a regression risk per se.
- **Streamer writes _before_ closing the turn.** The streaming `_save_stream_items*` helpers can write partial turn state to the session before the turn is fully resolved (`src/agents/run_internal/run_loop.py:1083-1086`). The persisted-count + fingerprint rewind (`src/agents/run_internal/session_persistence.py:653-713`) is best-effort and aborts if the session tail no longer matches.
- **Interruptions omit the streaming tuple.** `ToolApprovalItem` and `CompactionItem` are explicitly filtered out of `RunItemStreamEvent` (`src/agents/run_internal/streaming.py:56-59`). UI consumers must read `result.interruptions` separately rather than rely on the event stream; the bookkeeping is consistent but creates two channels.
- **Input guardrails run only on the first turn** (per the contributor guide in `AGENTS.md`), and only for the starting agent. Resuming a `RunState` does not re-run input guardrails (`src/agents/run.py:1172-1173` and `src/agents/run_internal/run_loop.py:672-678`). Consistency over coverage is the choice here.
- **`handoff_occured` typo is intentional** — it's a stable literal in `RunItemStreamEvent.name` (`src/agents/stream_events.py:33`), kept to avoid breaking public-API consumers.

## Failure Modes / Edge Cases

- **Interrupt before tool execution.** When `NextStepInterruption` is the `next_step`, `SingleStepResult` still carries the full `new_step_items` (including `ToolApprovalItem`s) (`src/agents/run_internal/turn_resolution.py:713-723`). On resume, the SDK drives `resolve_interrupted_turn` (`src/agents/run_internal/run_loop.py:737-748`) and consumes `_last_processed_response` to recreate the in-turn plan.
- **Orphan tool-call outputs during partial save.** Persistence logic explicitly re-includes `tool_call_output_item`s that ended up without a paired `tool_call_item` in the persisted slice (`src/agents/run_internal/session_persistence.py:274-281`).
- **Stateful retry rewind mismatch.** If the session tail no longer matches the rewound fingerprint, `_rewind_session_tail_suffix` aborts and logs `Skipping session rewind because the current tail does not match the retry-owned suffix` (`src/agents/run_internal/session_persistence.py:454-462`).
- **Tool-task cancellation during sibling failure.** `_raise_failure_after_draining_siblings` partitions pending tasks, cancels pre-invoke siblings, and awaits the rest with callbacks (`src/agents/run_internal/tool_execution.py:1480-1499`).
- **Cancelled upstream parallel guardrail.** When parallel input guardrails trip, the model task is explicitly cancelled (`src/agents/run.py:1231-1234`) — and the persisted session items are then rolled back via `persist_session_items_for_guardrail_trip` (`src/agents/run_internal/session_persistence.py:190-212`).
- **`ToolApprovalItem.to_input_item` is forbidden.** It raises `AgentsException` if the runtime tries to send an approval placeholder back to the model (`src/agents/items.py:631-636`).

## Future Considerations

- **Crash-recovery is documented only in tests.** The schema-state-roundtrip tests (`tests/test_run_state.py:294-400`) cover persisted identity, but no test directly simulates "process died between tool execution and session save and was restarted with `Runner.run(state)`". Adding such a test would harden the persistence/retry contract.
- **Multi-agent resume is implicit.** Handoff state lives on `RunState._current_agent` and `_generated_items` (`src/agents/run_state.py:203-219`); a tool-side-effect replay story across a handoff boundary is handled by skipping duplicate `tool_call_item`s on resume (`src/agents/run.py:1298-1333`) rather than by an explicit tool-call idempotency token, so future tool families that have irreversible side effects will need a user-supplied de-dup key.
- **Tool-call atomicity vs tool-side-effect atomicity.** Function tools execute concurrently and are persisted in aggregate. A future commit could expose a `tool_use_tracker`-style idempotency record persisted on `RunState` so that, on retry of the model call, only not-yet-completed tools re-execute. Today this is handled outside the SDK (`tests/test_run_impl_resume_paths.py:148-200`).
- **Retry-policies as a public surface.** `retry_policies` is exported (`src/agents/__init__.py:99-109`) and `RetryPolicy`/`ModelRetrySettings` are documented in `src/agents/retry.py:161-360`, but the conversation-locked compatibility retry (`src/agents/run_internal/model_retry.py:551-570`) is implicit. A future contributor guide entry explicitly tying the two could prevent regressions.
- **Span data is a `__slots__` record; the wire format is `dict[str, Any]`.** `SpanData.export()` (`src/agents/tracing/span_data.py:17-19`) is open. Adding a Pydantic-validated envelope to the trace export would let backends reject malformed spans before they ship.

## Questions / Gaps

- **What is the canonical "step" semantics?** The code distinguishes `SingleStepResult` / `SingleStep` / "step item" in many places, but there is no first-class `Step` type — the inner-loop granularity is `next_step` plus `ToolRun*` structs (`src/agents/run_internal/run_steps.py:62-174`). A future commit consolidating `_build_plan_for_fresh_turn` (`src/agents/run_internal/tool_planning.py:1`) into a typed `Plan` would tighten the vocabulary.
- **Can a turn contain only a tool call with no model call?** Not via the SDK — every `run_single_turn` issues a model call (`src/agents/run_internal/run_loop.py:1763-1779`). A "tool-only-resume" turn is expressed by replaying the previous response id and skipping input send (`src/agents/run_internal/model_retry.py:432-491`).
- **Is the `ToolApprovalItem.__eq__` identity-based hash safe under `dict`?** Yes for current uses (dedup sets in `processed_response.interruptions`), but the equality contract (`__eq__ = is`, `__hash__ = id`, `src/agents/items.py:569-575`) would silently misbehave if a future code path stored approval items in a `set` and relied on field equality. No evidence of such misuse, but worth annotating in the docstring.
- **`SingleStepResult.session_step_items` vs `new_step_items` divergence is not surfaced to non-streaming callers.** Only `result.session_step_items` consumers (e.g., `result.to_input_list(mode="normalized")`, `src/agents/result.py:287-305`) see the difference. The handoff-filter history collapse could become a defect source for downstream consumers expecting a 1:1 model.

---

Generated by `reports/source/01.03-step-turn-task-atomicity/openai-agents-sdk.md` against `openai-agents-sdk`.
