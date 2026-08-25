# Source Analysis: letta

## 01.05: Pause, Resume, and Interrupt Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI, Pydantic, async/await, Postgres + Redis) |
| Analyzed | 2026-07-02 |

## Summary

Letta models pause/resume around a first-class `Run` resource that owns a `Step[]` and a persisted `Message[]` graph. There are three documented ways an agent loop can yield control back to a caller before the natural `end_turn`/`max_steps`/`tool_rule` terminus: (1) **human-in-the-loop approval** of a tool call (synchronous, persisted as `StopReasonType.requires_approval`, resumed by sending an `approval`-typed `MessageCreate`), (2) **client-side tool execution** (functionally identical to approval, but the tool is run by the client and returned as `ToolReturn`), and (3) **explicit run cancellation** — checked cooperatively at every step boundary via `_check_run_cancellation(run_id)` against the `Run.status` row. Pause is therefore fully persisted: the run is in `RunStatus.created` or `RunStatus.running`, the last in-context message is an `ApprovalRequestMessage` (for HITL) or a `ToolCall` (for cancel-during-pending-tool), and the message graph + agent's `message_ids` (or `Conversation.in_context_message_ids`) is committed in the same transaction (`letta/agents/letta_agent_v3.py:758-816`). Resume is just "send the next request with the same `conversation_id` (and an approval/`ToolReturn` if paused on approval)". For long-running background streaming, Letta persists every SSE chunk to a Redis stream and exposes `GET /v1/runs/{run_id}/stream` with cursor-based `starting_after` so a client can detach and reattach to the same run, even across process restarts (`letta/server/rest_api/redis_stream_manager.py:199-526`).

The model is explicit and well-instrumented (separate `RunStatus`, `StepStatus`, `StopReasonType`, `JobStatus` enums in `letta/schemas/enums.py:133-274`), but it has several non-trivial frictions: cancellation is checked only at step boundaries (not inside an LLM call, not mid-tool-execution); there are two parallel lock systems (Redis conversation lock + per-step DB poll) with different guarantees; "pause" and "cancel" are conceptually unified through `RunStatus.cancelled` even though an in-flight approval is structurally different from a "user wants to stop"; and the official resume-after-crash story is "re-run with the same `run_id` and the agent will pick up from the persisted `message_ids`" but the agent loop itself doesn't auto-detect that it is resuming a prior run — the client must explicitly send the approval/`ToolReturn` payload and rely on the idempotency check in `letta/agents/helpers.py:230-310`.

## Rating

**7 / 10 — Clear model with explicit interfaces, persisted state, and a recovery story, but with coarse interrupt points and some scattered control flow.**

Rationale:
- Three explicit pause primitives (`StopReasonType.requires_approval`, `RunStatus.cancelled`, client-side tool handoff) with persistent backing rows (`Run`, `Step`, `Message`).
- Pause state is committed transactionally with the step that produced it (`letta/agents/letta_agent_v3.py:1402-1410`); cancellation flips `RunStatus.cancelled` and a denial-tool-return is appended to make the next resume consistent.
- Background streaming survives process restart via Redis-backed SSE (`letta/server/rest_api/redis_stream_manager.py:199-526`) and an explicit `GET /v1/runs/{run_id}/stream?starting_after=N` cursor API (`letta/server/rest_api/routers/v1/runs.py:357-411`).
- Resume input is small and well-typed: for approval, the client re-sends an `ApprovalCreate` (`letta/schemas/letta_message.py:31-36`); the server validates tool-call IDs match the persisted `ApprovalRequestMessage` (`letta/agents/helpers.py:121-148`) and dedupes via an idempotency check that survives summarization (`letta/agents/helpers.py:230-310`).
- Two public API surfaces for cancel (`POST /v1/agents/{id}/messages/cancel` at `letta/server/rest_api/routers/v1/agents.py:1887-1956` and `POST /v1/conversations/{id}/cancel` at `letta/server/rest_api/routers/v1/conversations.py:836-921`); both surface `NoActiveRunsToCancelError` when there is nothing to cancel (`letta/errors.py:59-69`).
- Cooperative cancellation in the agent loop polls the DB once per step (`letta/agents/letta_agent_v3.py:1029-1035`); the SSE path additionally polls every 0.5 s via `cancellation_aware_stream_wrapper` (`letta/server/rest_api/streaming_response.py:153-229`).
- Resume can be re-driven without a connection by `cancellRun`'s cleanup path, which materializes the denial/`ToolReturn` immediately (`letta/services/run_manager.py:655-777`).

Deductions:
- Cancellation is **not** mid-step: an in-flight LLM call or tool execution always runs to completion (or to its own error). The agent loop sets `should_continue = False` but only after the current `_step()` returns (`letta/agents/letta_agent_v3.py:1511-1540`).
- There is no in-process interruption of LLM HTTP requests; a stuck tool is a stuck run. Tests rely on short prompts to make this observable (`tests/integration_test_cancellation.py:179-188`).
- Three different status enums (`RunStatus`, `JobStatus`, `StopReasonType`) live side-by-side and have to be reconciled; `cancel_run` only treats a run as cancellable when `stop_reason not in {requires_approval}` (`letta/services/run_manager.py:651`), so a paused-on-approval run is the *only* state where "cancel" actually means "deny the pending tool call" — a subtle semantics split.
- Multi-party resume is allowed (any caller with the `run_id` can `GET /v1/runs/{id}/stream`), but there is no ownership/authz beyond organization-scope, and no optimistic concurrency control on resume writes — first writer wins, second writer's messages may collide.
- "World changes while paused" is not modeled: the same OTID retry is idempotent, but a different OTID for the same `conversation_id` triggers a fresh run that races with the original lock (`letta/services/streaming_service.py:283-319`).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Run status enum (lifecycle) | `RunStatus = {created, running, completed, failed, cancelled}` | `sources/letta/letta/schemas/enums.py:152-161` |
| Step status enum | `StepStatus = {PENDING, SUCCESS, FAILED, CANCELLED}` | `sources/letta/letta/schemas/enums.py:268-274` |
| Stop reason enum (terminal states) | `StopReasonType` with `requires_approval` and `cancelled` | `sources/letta/letta/schemas/letta_stop_reason.py:9-22` |
| Stop-reason → run-status mapping | `requires_approval` maps to `RunStatus.completed`; `cancelled` maps to `RunStatus.cancelled` | `sources/letta/letta/schemas/letta_stop_reason.py:24-49` |
| Run model | `Run` schema with `status`, `stop_reason`, `background`, `callback_url`, `conversation_id` | `sources/letta/letta/schemas/run.py:17-50` |
| Run update DTO | `RunUpdate(status, completed_at, stop_reason, metadata_, total_duration_ns)` | `sources/letta/letta/schemas/run.py:53-61` |
| Step model | `Step` with `run_id`, `step_id`, `status`, `error_type`, `error_data` | `sources/letta/letta/schemas/step.py:16-66` |
| Step progression enum | `StepProgression` (START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED) | `sources/letta/letta/schemas/step.py:68-74` |
| Approval message role | `MessageRole.approval` | `sources/letta/letta/schemas/enums.py:110-117` |
| Approval wire types | `ApprovalReturn`, `ToolReturn`, `MessageReturnType.approval/tool` | `sources/letta/letta/schemas/letta_message.py:22-51` |
| Approval request message | `ApprovalRequestMessage` with `tool_calls` and `step_id` | `sources/letta/letta/schemas/letta_message.py:306-326` |
| Approval response message | `ApprovalResponseMessage` with `approvals`, `approve`, `approval_request_id`, `reason` | `sources/letta/letta/schemas/letta_message.py:328-348` |
| Approval helpers | `is_approval_request`, `is_approval_response` on `Message` | `sources/letta/letta/schemas/message.py:2450-2455` |
| Tool-rule type for HITL | `RequiresApprovalToolRule(type="requires_approval")` | `sources/letta/letta/schemas/tool_rule.py:348-357` |
| Agent → "pending approval" pointer | `AgentState.pending_approval: Optional[ApprovalRequestMessage]` | `sources/letta/letta/schemas/agent.py:134-135` |
| Approval request construction | `create_approval_request_message_from_llm_response` (V2) | `sources/letta/letta/server/rest_api/utils.py:304-369` |
| Approval response construction | `create_approval_response_message_from_input` (V2) | `sources/letta/letta/server/rest_api/utils.py:175-227` |
| Denial tool-return helper | `create_tool_returns_for_denials` (also used by cancel) | `sources/letta/letta/server/rest_api/utils.py:230-264` |
| Tool-denial message helper | `create_tool_message_from_returns` | `sources/letta/letta/server/rest_api/utils.py:267-301` |
| Client-tool pause docs | "When the agent calls a client-side tool, execution pauses and returns control to the client" | `sources/letta/letta/schemas/letta_request.py:12-22` |
| ClientToolSchema definition | `ClientToolSchema(name, description, parameters)` | `sources/letta/letta/schemas/letta_request.py:12-22` |
| Background-streaming flag | `LettaStreamingRequest.background: bool` | `sources/letta/letta/schemas/letta_request.py:176-179` |
| Agent loop entry points (abstract) | `BaseAgentV2.step` / `stream` accept `client_tools`, `client_skills`, `run_id` | `sources/letta/letta/agents/base_agent_v2.py:50-104` |
| Agent loop factory | `AgentLoop.load(agent_state, actor)` → `LettaAgentV3` / `LettaAgentV2` / `SleeptimeMultiAgent` | `sources/letta/letta/agents/agent_loop.py:15-62` |
| V3 step entry — pause points | top-of-loop approval-response detection + cancellation check | `sources/letta/letta/agents/letta_agent_v3.py:973-1040` |
| V3 cancel check inside `_step` | `if run_id and await self._check_run_cancellation(run_id):` | `sources/letta/letta/agents/letta_agent_v3.py:1029-1035` |
| V2 cancel check inside `_step` | same logic, parent class | `sources/letta/letta/agents/letta_agent_v2.py:505-509` |
| V2 cancel check primitive | `_check_run_cancellation(run_id)` reads `RunStatus` from DB | `sources/letta/letta/agents/letta_agent_v2.py:749-757` |
| V3 pause→approval path | When LLM requests a tool, agent splits into `requested_tool_calls` vs `allowed_tool_calls` and returns `requires_approval` | `sources/letta/letta/agents/letta_agent_v3.py:1681-1709` |
| V3 client-tool pause path | Client tools treated identically to `requires_approval` tools | `sources/letta/letta/agents/letta_agent_v3.py:1684-1696` |
| V3 resume from approval response | `_maybe_get_approval_messages(messages)` pairs persisted request with incoming response | `sources/letta/letta/agents/letta_agent_v3.py:973-1023` |
| V3 step checkpoint (start) | `_step_checkpoint_start` writes a `Step` row with `status=PENDING` before any LLM call | `sources/letta/letta/agents/letta_agent_v2.py:941-966` |
| V3 step checkpoint (LLM done) | `_step_checkpoint_llm_request_finish` updates `StepProgression` | `sources/letta/letta/agents/letta_agent_v2.py:979-985` |
| V3 step checkpoint (success) | `_step_checkpoint_finish` writes `Step.status=SUCCESS` | `sources/letta/letta/agents/letta_agent_v2.py:988-1029` |
| V3 error checkpoint | `update_step_error_async` writes `Step.error_type`, `error_data` | `sources/letta/letta/agents/letta_agent_v3.py:1555-1567` |
| Message persistence on step success | `_checkpoint_messages(run_id, step_id, new_messages, in_context_messages)` | `sources/letta/letta/agents/letta_agent_v3.py:758-816` |
| Conversation-mode persistence | `ConversationManager.add_messages_to_conversation` / `update_in_context_messages` | `sources/letta/letta/agents/letta_agent_v3.py:788-806` |
| Approval idempotency check | Helper that survives summarization by searching persisted `tool` messages | `sources/letta/letta/agents/helpers.py:230-310` |
| Approval tool-call-id validation | `validate_approval_tool_call_ids` (request must reference all pending tool calls) | `sources/letta/letta/agents/helpers.py:121-148` |
| Persisted-tool-call-id validation | `validate_persisted_tool_call_ids` (used for idempotency) | `sources/letta/letta/agents/helpers.py:102-119` |
| `_maybe_get_approval_messages` | Slices the last two messages as a (request, response) pair | `sources/letta/letta/agents/helpers.py:522-528` |
| Run manager — create | `create_run` writes `Run` + `RunMetrics` in one transaction | `sources/letta/letta/services/run_manager.py:47-90` |
| Run manager — get with status | `get_run_with_status` consults Lettuce if metadata flag set | `sources/letta/letta/services/run_manager.py:103-130` |
| Run manager — list | `list_runs(actor, agent_ids, statuses, stop_reason, conversation_id, ...)` | `sources/letta/letta/services/run_manager.py:132-216` |
| Run manager — update | `update_run_by_id_async` enforces lifecycle invariants | `sources/letta/letta/services/run_manager.py:320-481` |
| Run manager — cancel | `cancel_run` flips `status=cancelled`, `stop_reason=cancelled`, materializes denial for pending approval | `sources/letta/letta/services/run_manager.py:618-781` |
| Idempotent cancel guard | `if run.stop_reason not in {requires_approval}: return` | `sources/letta/letta/services/run_manager.py:651-653` |
| Cancellation reason constant | `TOOL_CALL_DENIAL_ON_CANCEL = "The user cancelled the request, so the tool call was denied."` | `sources/letta/letta/constants.py:221` |
| Cancel-endpoint (agent) | `POST /v1/agents/{id}/messages/cancel` | `sources/letta/letta/server/rest_api/routers/v1/agents.py:1887-1956` |
| Cancel-endpoint (conversation) | `POST /v1/conversations/{id}/cancel` | `sources/letta/letta/server/rest_api/routers/v1/conversations.py:836-921` |
| Job cancel-endpoint | `PATCH /v1/jobs/{id}/cancel` | `sources/letta/letta/server/rest_api/routers/v1/jobs.py:107-128` |
| Run delete-endpoint | `DELETE /v1/runs/{run_id}` | `sources/letta/letta/server/rest_api/routers/v1/runs.py:310-321` |
| Run retrieve-endpoint | `GET /v1/runs/{run_id}` | `sources/letta/letta/server/rest_api/routers/v1/runs.py:143-154` |
| Run stream re-attach endpoint | `POST /v1/runs/{run_id}/stream` (cursor-paged) | `sources/letta/letta/server/rest_api/routers/v1/runs.py:324-411` |
| `RunManager` | `class RunManager` orchestrates run/step/message lifecycle | `sources/letta/letta/services/run_manager.py:38-46` |
| Conversation-level lock | `AsyncRedisClient.acquire_conversation_lock` (non-blocking, raises `ConversationBusyError`) | `sources/letta/letta/data_sources/redis_client.py:194-229` |
| Lock release | `release_conversation_lock` invoked on terminal updates | `sources/letta/letta/services/run_manager.py:391-396` |
| Conversation busy error | `ConversationBusyError(conversation_id, lock_holder_token, run_id)` | `sources/letta/letta/errors.py:81-107` |
| Pending approval error | `PendingApprovalError(pending_request_id)` (HTTP 409) | `sources/letta/letta/errors.py:48-56` |
| No active runs to cancel | `NoActiveRunsToCancelError(agent_id, conversation_id)` | `sources/letta/letta/errors.py:59-69` |
| Run cancel error | `RunCancelError` | `sources/letta/letta/errors.py:488-491` |
| Background streaming service | `StreamingService._create_run` and `create_background_stream_processor` | `sources/letta/letta/services/streaming_service.py:180-470` |
| Background lock acquisition | Redis conversation lock + `try_recover_duplicate_request` for OTID retries | `sources/letta/letta/services/streaming_service.py:283-326` |
| OTID → run_id mapping | `set_otid_run_mapping`, `get_run_id_by_otid` for client retry | `sources/letta/letta/data_sources/redis_client.py:250-289` |
| Request-token derivation | `derive_request_token(otids)` (sha256) for same-OTID idempotency | `sources/letta/letta/services/streaming_service.py:64-80` |
| Same-OTID retry recovery | `try_recover_duplicate_request` returns a Redis-backed stream | `sources/letta/letta/services/streaming_service.py:83-117` |
| Background stream processor | `create_background_stream_processor` writes SSE chunks to Redis stream | `sources/letta/letta/server/rest_api/redis_stream_manager.py:199-526` |
| Redis SSE stream writer | `RedisSSEStreamWriter` (batched XADD with TTL 3 h) | `sources/letta/letta/server/rest_api/redis_stream_manager.py:26-196` |
| Stream re-attachment | `redis_sse_stream_generator(starting_after, poll_interval, batch_size)` | `sources/letta/letta/server/rest_api/redis_stream_manager.py:463-526` |
| Cancellation-aware stream wrapper | `cancellation_aware_stream_wrapper` polls every 0.5 s for cancel | `sources/letta/letta/server/rest_api/streaming_response.py:153-229` |
| Cancellation event registry | `_cancellation_events: Dict[run_id, asyncio.Event]` + `get_cancellation_event_for_run` | `sources/letta/letta/server/rest_api/streaming_response.py:31-40` |
| Cancellation exception class | `RunCancelledException` injected into the generator | `sources/letta/letta/server/rest_api/streaming_response.py:43-48` |
| Keepalive during long pause | `add_keepalive_to_stream(interval=30s, max_silence=1800s)` | `sources/letta/letta/server/rest_api/streaming_response.py:51-149` |
| Task-level cancel handling | Distinguishes explicit `RunStatus.cancelled` from infrastructure `asyncio.CancelledError` | `sources/letta/letta/server/rest_api/redis_stream_manager.py:317-377` |
| Stream finalizer | Synthesizes `[DONE]` and a `LettaStopReason` so client SDKs terminate | `sources/letta/letta/server/rest_api/redis_stream_manager.py:283-461` |
| Run status update on terminal | `update_run_by_id_async(status=run_status, stop_reason=final_stop_reason)` in finally | `sources/letta/letta/server/rest_api/redis_stream_manager.py:428-445` |
| Lettuce client (no-op stub) | `LettuceClient.cancel/get_status/step` — base returns None | `sources/letta/letta/services/lettuce/lettuce_client_base.py:9-101` |
| Summarizer protects pending approval | `if in_context_messages[-1].role == MessageRole.approval: keep` | `sources/letta/letta/services/summarizer/summarizer.py:295-304` |
| Self-summarizer protects pending approval | keeps assistant+approval pair | `sources/letta/letta/services/summarizer/self_summarizer.py:182-280` |
| Conversation manager | `ConversationManager.update_in_context_messages` (per-conversation `message_ids`) | `sources/letta/letta/services/conversation_manager.py` |
| Settings flags | `enable_cancellation_aware_streaming=True`, `track_agent_run=True` | `sources/letta/letta/settings.py:294,386` |
| Test: background cancellation | `test_background_streaming_cancellation` (parametrized over 6 LLMs) | `sources/letta/tests/integration_test_cancellation.py:171-208` |
| Test: cancel via API | `client.agents.messages.cancel(agent_id=...)` | `sources/letta/tests/integration_test_cancellation.py:85-87` |
| Test: approval + cancel race | `test_approve_with_cancellation` exercises cancel-during-approval | `sources/letta/tests/integration_test_human_in_the_loop.py:1340-1454` |
| Test: cursor-fetch for approval response | `test_approve_cursor_fetch` | `sources/letta/tests/integration_test_human_in_the_loop.py:430-477` |
| Test: idempotent retry after summarization | `test_retry_with_summarization` | `sources/letta/tests/integration_test_human_in_the_loop.py:1456-1568` |
| Test: client-side tool pause | `test_client_side_tool_full_flow` | `sources/letta/tests/integration_test_client_side_tools.py:67-333` |
| Test: tool approval modification | `agent_manager.modify_approvals_async` and PATCH endpoint | `sources/letta/letta/server/rest_api/routers/v1/agents.py:707-740` |
| Pending-approval rejection on new message | `raise PendingApprovalError(pending_request_id=...)` | `sources/letta/letta/agents/helpers.py:309-310` |

## Answers to Dimension Questions

### 1. Can execution pause safely?

Yes. Three explicit pause mechanisms, each with a persisted representation:

- **Approval gate (HITL).** When the LLM proposes a tool whose name appears in `requires_approval_tool_rules` (or is a `client_tool`), the agent loop persists an `ApprovalRequestMessage` and returns `StopReasonType.requires_approval` (`letta/agents/letta_agent_v3.py:1681-1709`). The run row is updated to `RunStatus.completed` with `stop_reason=requires_approval` (`letta/schemas/letta_stop_reason.py:24-49`). The agent's `pending_approval` pointer is exposed on `AgentState` (`letta/schemas/agent.py:134-135`).
- **Client-side tool pause.** Functionally identical to approval, but the tool is dispatched to the client (no server execution) and the client returns a `ToolReturn`. Same `_handle_ai_response` branch handles both (`letta/agents/letta_agent_v3.py:1684-1696`).
- **Cooperative cancellation.** `cancel_run` flips `RunStatus.cancelled` and `stop_reason=cancelled` (`letta/services/run_manager.py:662-667`). The agent loop's per-step `_check_run_cancellation` returns `True` at the next step boundary, the loop sets `should_continue=False`, and exits cleanly (`letta/agents/letta_agent_v3.py:1029-1040`).

All three are committed transactionally: the `Step` row is updated at `_step_checkpoint_finish` (`letta/agents/letta_agent_v2.py:988-1029`) before the message is checkpointed, and the `Run` is updated in `update_run_by_id_async` (`letta/services/run_manager.py:320-481`).

### 2. Can it resume after a crash?

Two layers, both crash-safe:

- **In-process pause (approval, client-side tool).** Pause state lives in the `Message` table keyed by `run_id` and `step_id`. On restart, the next request that includes the same `run_id` and an `ApprovalCreate` payload flows through `_maybe_get_approval_messages` (`letta/agents/letta_agent_v3.py:973-1023`) and the loop resumes from the persisted `in_context_messages`. The helper at `letta/agents/helpers.py:230-310` provides idempotency so duplicate approvals don't re-execute tools.
- **Background streaming.** SSE chunks are written to a Redis stream (`sse:run:{run_id}`) with 3-hour TTL (`letta/server/rest_api/redis_stream_manager.py:42-44`). The `GET /v1/runs/{run_id}/stream` endpoint with `starting_after` cursor allows reattachment even after a client or server crash (`letta/server/rest_api/routers/v1/runs.py:357-411`). The `redis_sse_stream_generator` reads from the same key (`letta/server/rest_api/redis_stream_manager.py:463-526`). Task-level cancellation is gracefully handled by inspecting `RunStatus` (`letta/server/rest_api/redis_stream_manager.py:317-377`).

### 3. Is the resume point deterministic?

Mostly. The `run_id` is the unit of resumption; within a run, the resume point is always the next `_step()` after the persisted step. The idempotency check (`letta/agents/helpers.py:230-310`) ensures that re-sending the same approval yields the same outcome (a tool execution), not a duplicate.

Determinism has a few caveats:
- Multiple parallel clients can race on the same `run_id`; there is no row-level write contention control beyond the implicit DB locking during `update_run_by_id_async` (`letta/services/run_manager.py:320-481`).
- The conversation lock (`acquire_conversation_lock`, `letta/data_sources/redis_client.py:194-229`) serializes **new** requests but not resume of an existing `run_id` — so a new request with the same `conversation_id` may interleave with a resumed run.
- When an agent has `enable_sleeptime=True` or `multi_agent_group`, the loop is dispatched to `SleeptimeMultiAgentV3/V4` (`letta/agents/agent_loop.py:35-58`), and pause/resume is delegated to the multi-agent loop; this is opaque to the caller.

### 4. What happens if the world changed while paused?

The model does not auto-detect or reconcile external change. Concretely:

- **Same OTID, same request.** Recovered via `try_recover_duplicate_request` (`letta/services/streaming_service.py:83-117`) — the client gets the same stream again.
- **Same OTID, same request, but `RunStatus.cancelled`.** The cancel-endpoint first inspects `run.stop_reason` and is idempotent (`letta/services/run_manager.py:651-653`).
- **New OTID, same `conversation_id`.** Acquires the Redis lock, creates a new `Run`, and may run concurrently with the prior run. The `ConversationBusyError` is enriched with the holder's `run_id` so the client can decide (`letta/services/streaming_service.py:120-141`).
- **Approval response against a stale `ApprovalRequestMessage`.** Caught by `validate_approval_tool_call_ids` (`letta/agents/helpers.py:121-148`) which compares `approval_request_tool_call_ids` to the response and raises `ValueError` if mismatched.
- **Approval response after summarization evicted the original request.** The idempotency check searches the full `messages` history, not just the in-context window (`letta/agents/helpers.py:248-265`); the summarizer itself protects the trailing approval request and its companion assistant message from eviction (`letta/services/summarizer/summarizer.py:295-304`, `letta/services/summarizer/self_summarizer.py:182-280`).

### 5. Can multiple people or systems resume the same run?

Yes for read, weakly for write:

- **Read.** Any caller with the `run_id` and organization scope can `GET /v1/runs/{run_id}` and `GET /v1/runs/{run_id}/stream` (`letta/server/rest_api/routers/v1/runs.py:143-154, 324-411`). The stream is shared, fan-out is possible.
- **Write.** There is no explicit ownership field. The first writer to `update_run_by_id_async` wins; subsequent writes fail with `ConcurrentUpdateError` (`letta/errors.py:72-78`) or silently overwrite depending on transaction timing. The cancel endpoint loops over a list of `run_ids` and is safe to call concurrently (`letta/services/run_manager.py:619-781`).
- **Idempotency.** The OTID-based dedup uses sha256 of all message OTIDs (`letta/services/streaming_service.py:64-80`); two clients sending the *same* set of OTIDs hit `try_recover_duplicate_request` and read the same stream, but sending *different* OTIDs is treated as a new request.

## Architectural Decisions

- **Run as the unit of resumption.** `Run` (`letta/schemas/run.py:17-50`) carries `status`, `stop_reason`, `callback_url`, `conversation_id`, and `background`; a `Step[]` (`letta/schemas/step.py:16-66`) is the inner unit, and `Message[]` carry `run_id` + `step_id`. This means the database row, not the in-process loop, is the source of truth.
- **Step progression states.** `StepProgression` (`letta/schemas/step.py:68-74`) is a state machine used to decide whether a step's terminal cleanup is a "log the step" or "log the trace" event. This is implicit in `_step`'s `finally` block (`letta/agents/letta_agent_v3.py:1541-1592`) and is the closest thing to a checkpointing protocol.
- **Cooperative cancellation only.** Cancellation is sampled once per step inside the agent loop (`letta/agents/letta_agent_v3.py:1029-1035`) and every 0.5 s in the SSE stream wrapper (`letta/server/rest_api/streaming_response.py:153-229`). There is no mechanism to interrupt an in-flight LLM HTTP request or a running tool.
- **Approval and cancel share a state model.** An approval-pending run is a `RunStatus.completed` run with `stop_reason=requires_approval`. Cancel materializes a denial `ToolReturn` so the in-context graph stays consistent (`letta/services/run_manager.py:655-777`).
- **Two lock systems.** The streaming entry point acquires a Redis conversation lock to serialize *new* requests (`letta/services/streaming_service.py:283-326`). The agent loop separately polls the `Run.status` row to detect cancellation (`letta/agents/letta_agent_v2.py:749-757`). The two are not unified.
- **Idempotency via message OTID.** Every `MessageCreate` carries an `otid` (offline threading id) used to derive the request token (`letta/services/streaming_service.py:64-80`). Retries with the same OTID recover the in-flight run; retries with a new OTID are treated as new work.
- **Background streams survive process restart.** Redis-backed SSE writer with TTL 3 h (`letta/server/rest_api/redis_stream_manager.py:42-44`); client SDK can reattach via `starting_after` cursor.

## Notable Patterns

- **Persistent chat completion as a state machine.** Each `_step` writes a `Step` row, a `Message` row, and updates `Run.status` atomically (`letta/agents/letta_agent_v3.py:1402-1410`).
- **Two separate status enums with a mapping.** `StopReasonType.run_status` (`letta/schemas/letta_stop_reason.py:24-49`) is the canonical mapping from terminal reason to `RunStatus`. Code that updates the run uses this mapping to avoid drift.
- **Asyncio.Event registry for in-process cancellation.** `_cancellation_events: Dict[run_id, asyncio.Event]` (`letta/server/rest_api/streaming_response.py:31-40`) is a process-local registry of cancellation events. It's a complementary signal to the DB poll.
- **`athrow` to inject cancellation into a generator.** The cancellation-aware stream wrapper uses `stream_generator.athrow(RunCancelledException(...))` to let the agent loop's own `finally` block fire and write a `[DONE]` marker (`letta/server/rest_api/streaming_response.py:201-206`).
- **Per-step early-out approval path.** The agent loop treats approval responses as a special first two messages of a step (`_maybe_get_approval_messages` at `letta/agents/helpers.py:522-528`) so the LLM never has to be re-asked about a tool that already has an answer.
- **Summarizer-aware pause.** The summarizer refuses to evict a trailing `ApprovalRequestMessage` or its companion assistant message, so a paused run can survive an aggressive context-window compaction (`letta/services/summarizer/summarizer.py:295-304`).
- **`Callback`-as-final-action.** `Run.callback_url` and the `_dispatch_callback_async` mechanism (`letta/services/run_manager.py:483-510`) fire when the run first becomes terminal; the cancel path also respects this. A long-paused run therefore gets exactly one terminal callback.
- **Idempotency check that searches the full message history.** When the in-context window has been compacted, the approval idempotency check still finds the matching `tool` message via `message_manager.list_messages(roles=["tool"])` (`letta/agents/helpers.py:248-265`).

## Tradeoffs

- **Pause is well-typed but coarse-grained.** All pauses are *step-level* (between LLM call and tool execution, or between two tool executions). Inside a long tool call or a long LLM call, the run cannot be interrupted.
- **Two status systems.** `RunStatus` (DB) and `StepStatus` (per-step) are kept in sync, but only `RunStatus` is consulted for cancellation; the step's own status can lag, which makes "is the run currently paused" queries need to look at `Run.stop_reason`, not `Step.status`.
- **Cancel and deny are conflated.** A run cancelled mid-approval is rewritten as "the user denied all pending tool calls" (`letta/services/run_manager.py:705-777`); this is operationally simple but means a "cancel" and a "deny" leave identical trails.
- **Resumability is implicit.** There is no `POST /v1/runs/{id}/resume` endpoint; the caller must re-send the next user message (or approval) and let the agent loop discover the prior `run_id` and pick up. This is documented in the `client.agents.messages.stream` docstring but is not surfaced as a first-class resource.
- **Lock granularity mismatches.** Conversation locks serialize per `conversation_id` (or per `agent_id` if no conversation), but the agent loop itself has no in-loop lock. Two concurrent `stream` calls with the same `conversation_id` will hit `ConversationBusyError`; two calls with the same `run_id` but different OTIDs will both proceed.
- **Lettuce integration is a stub.** `LettuceClient` is a no-op base class (`letta/services/lettuce/lettuce_client_base.py:9-101`); in production, cancellation is forwarded to a Lettuce remote service when `run.metadata.lettuce` is set (`letta/server/rest_api/routers/v1/agents.py:1940-1947`). The local cancellation flow doesn't depend on this.

## Failure Modes / Edge Cases

- **Stuck LLM call.** An LLM provider that hangs (rather than errors) blocks the agent loop indefinitely. The cooperative cancellation only fires at step boundaries; tests use short models to side-step this (`tests/integration_test_cancellation.py:179-188`).
- **Concurrent cancel + approve.** The test `test_approve_with_cancellation` (`tests/integration_test_human_in_the_loop.py:1340-1454`) shows that whichever side commits first wins; if the approval response is persisted before cancel sees it, the run finishes normally rather than being marked `cancelled`.
- **Cancellation of a completed run.** `cancel_run` is idempotent: `if run.stop_reason and run.stop_reason not in {StopReasonType.requires_approval}: return` (`letta/services/run_manager.py:651-653`). So you cannot "cancel" a run that ended with `end_turn` (it's already terminal) but you *can* "cancel" one that ended with `requires_approval` (denying the pending tool calls).
- **Stream "incomplete" handling.** If the background processor dies without a `[DONE]`, the stream finalizer synthesizes one and writes a `stream_incomplete` error so the SDK can exit cleanly (`letta/server/rest_api/redis_stream_manager.py:283-315`).
- **Redis absence.** If Redis is unavailable, `NoopAsyncRedisClient` is used; background streaming is rejected with HTTP 503 (`letta/server/rest_api/routers/v1/runs.py:374-385`) and `track_agent_run=False` disables the run-tracking + cancel APIs (`letta/server/rest_api/routers/v1/agents.py:1908-1909`).
- **Loss of pending tool call during eviction.** A summarizer that aggressively evicts the trailing `ApprovalRequestMessage` would corrupt the in-context graph; this is prevented by an explicit check (`letta/services/summarizer/summarizer.py:295-304`).
- **Missing tool call id on approval request.** The V3 loop backfills a `step_id` and warns if absent (`letta/agents/letta_agent_v3.py:1019-1027`); older messages without `step_id` are migrated on the fly.
- **Pause that outlasts a Lettuce shutdown.** The Lettuce base class returns `None` from `cancel`, so when Lettuce is configured the local cancel only marks the DB; the remote run may continue until Lettuce notices.

## Future Considerations

- **Mid-step cancellation.** The model would benefit from a token-cancel API (e.g., `asyncio.Task.cancel` on the LLM call) so a user-visible "stop" can interrupt a slow streaming response, not just a step boundary.
- **First-class resume endpoint.** A `POST /v1/runs/{run_id}/resume` that returns the next available step input slot would make the resumability contract explicit.
- **Run ownership / lease.** The lack of an owner field on `Run` means any org member can resume any run. A `lease_token` (similar to Lettuce's `request_token`) would prevent concurrent resume races.
- **Unified status.** The `JobStatus` / `RunStatus` / `StepStatus` / `StopReasonType` set could collapse into two: a per-row "phase" (created/running/terminal) and a "reason" string. The current four enums duplicate `created`/`running`/`completed`/`failed`/`cancelled` semantics.
- **Crash-safe checkpoint stream.** The Redis SSE writer uses `XADD` with a 3 h TTL. A persisted Postgres-only fallback (a `StepEvent` table) would let clients reattach without Redis.
- **Optimistic concurrency on `update_run_by_id_async`.** No version field; concurrent updates can race. Adding a `version` column + check would make the cancel/resume story safe under contention.
- **Generic "interrupted" stop reason.** Today, "cancelled" and "requires_approval" are the only "non-natural" reasons. A `paused` reason (like `AgentStepStatus.paused` in `letta/schemas/enums.py:170-171`, which is currently unused) could unify HITL and explicit pause.

## Questions / Gaps

- **What happens to in-flight tool calls when the agent loop is cancelled mid-step?** The loop is only aware of `should_continue` at step boundaries, so the tool runs to completion. No "tool cancel" primitive is exposed.
- **How does resume-after-process-restart interact with the `current_run_id` on the legacy `LettaAgent` (`letta/agents/letta_agent.py:155-171`)?** The legacy agent uses a per-instance `self.current_run_id`; if the process restarts, that field is lost. The agent_id-based re-derivation is implicit and not documented.
- **Are there observability hooks for the "pause" and "resume" events?** The agent loop emits OTEL spans (`_step_checkpoint_start` creates an `agent_step` span, `letta/agents/letta_agent_v2.py:944`), but there is no `EventMessage` for "approval received" or "run cancelled". Clients have to infer these from message streams.
- **Does the `Run.cancel` API surface a deterministic cancel id?** No — the cancellation is fire-and-forget; the `cancellation_event` is process-local. Cross-process cancellation relies on the DB poll.
- **Can two clients collaborate on resuming a single run?** The shared Redis stream allows read fan-out, but write fan-out (e.g., one client approves tool A while another approves tool B) is undefined. The current `approvals` payload is treated as a single batch.
- **Is there a documented SLO for "time from cancel call to agent loop exit"?** The test uses 1.5 s delay (`tests/integration_test_cancellation.py:179`), but no formal SLO is recorded; the 0.5 s `cancellation_check_interval` in `cancellation_aware_stream_wrapper` (`letta/server/rest_api/streaming_response.py:158`) is the upper bound for SSE-level cancellation, while the per-step poll in the agent loop can be much longer (one LLM call + tool execution + post-step summary).
- **What about multi-agent (group) resume?** `SleeptimeMultiAgentV3/V4` (`letta/agents/agent_loop.py:35-58`) extends `LettaAgentV2`; pause/resume semantics for sub-agent runs are delegated but the surface is opaque to the client (no `run_id` per sub-agent is exposed in the public API).
- **What is the authz story for `cancel_run`?** `cancel_run` checks that the run exists in the actor's organization (`letta/services/run_manager.py:618-639`) but does not check actor permissions on the agent beyond org scope. A "viewer" role can cancel any run they can see.

---

Generated by `dimensions/01.05-pause-resume-and-interrupt-semantics.md` against `letta`.
