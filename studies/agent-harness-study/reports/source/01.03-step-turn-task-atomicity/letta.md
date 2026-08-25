# Source Analysis: letta

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (asyncio) + SQLAlchemy + Pydantic + OpenTelemetry. Long-running agent server (FastAPI) with Postgres/SQLite persistence and optional Redis-backed Lettuce orchestration. |
| Analyzed | 2026-07-02 |

## Summary

Letta's atomicity model is **explicit but layered**: the persisted unit, the retried unit, the traced unit, and the user-visible unit are not all the same shape. The naming is somewhat overloaded, but the boundaries are encoded in named types and enums:

- A **Run** is a per-request conversation/processing session for an agent (`letta/schemas/run.py:18`, `letta/schemas/enums.py:152`) — `RunStatus ∈ {created, running, completed, failed, cancelled}` (`letta/schemas/enums.py:152-161`). It is the lifecycle owner of steps.
- A **Step** is the smallest *persisted* atomic unit of execution: one LLM call + the synchronous tool execution that consumes its tool_calls + optional compaction, all wrapped in a `Step` row in the `steps` table (`letta/orm/step.py:20-98`, `letta/schemas/step.py:16-65`). It carries its own `StepStatus ∈ {pending, success, failed, cancelled}` (`letta/schemas/enums.py:268-274`).
- A **Message** is the message-level unit, with `step_id` and `run_id` foreign keys (`letta/orm/message.py:48-53`, `letta/schemas/message.py:292-293`) that give every persisted message a step and a run.
- A **Trace span hierarchy** (`agent_step` per step, `time_to_first_token` per request, decorator-driven spans per method) is emitted via OpenTelemetry (`letta/otel/tracing.py:228-435`, `letta/agents/letta_agent_v2.py:916-966`) and correlated through `step.trace_id` and `step.request_id` (`letta/schemas/step.py:50-51`, `letta/services/step_manager.py:157-158,219-220`).
- A **Conversation** layer (`letta/schemas/conversation.py:11-35`, `letta/services/conversation_manager.py`) provides an orthogonal per-thread isolation axis, but it is not itself an atomic unit — it is a view on in-context messages.

The system is best characterized as **clear model, mid-tier maturity**: the persisted atomic unit (Step) is fully typed, has a four-state status machine, is observable through OTel, has error/stop-reason capture, has a separate `StepMetrics` row, and has callback hooks (`WebhookService.notify_step_complete` at `letta/services/step_manager.py:413,475,554`). But the *retried* unit is the **inner LLM request inside a step** (`for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1)` at `letta/agents/letta_agent_v3.py:1093`), not the step itself; the step is the *rollback* boundary, not the retry boundary. Tool calls are *not* their own atomic unit — they are aggregated inside a step's message write set. Partial state during a crash is only partially bounded: the `Step` row is written at start with `StepStatus.PENDING` (`letta/agents/letta_agent_v2.py:946-966`) and updated at end, but **message persistence only happens at step success** (`letta/agents/letta_agent_v3.py:1511-1540`, "message persistence does not happen in the case of an exception (rollback to previous state)"), so a crash mid-step leaves a `pending` step with no messages and no automatic resumption.

The phrase "what completed and what did not" is partially answered: the `Step` row tells you `pending` vs `success`/`failed`/`cancelled`, and `error_type`/`error_data` (`letta/schemas/step.py:63-65`) capture the cause on failure, but the OTel span attribute set on the `agent_step` span (`letta/agents/letta_agent_v2.py:944-945`) is the closest thing to a single "step event" record.

## Rating

**7/10** — Clear model with explicit interfaces (`Step`, `Run`, `StepStatus`, `RunStatus`, `StepProgression`), tests cover the status transitions (`tests/test_managers.py:9593-9691`), four-state lifecycle is enforced via separate update methods, and OTel provides an observation axis. Deducted from higher because: (a) the retried unit (LLM request, `letta/agents/letta_agent_v3.py:1093`) is finer than the persisted unit (Step), so retry semantics inside a step are asymmetric vs. the step-level rollback — compaction retry is loop-scoped (`for llm_request_attempt in range(...)`) but step-level failure aborts the agent loop unconditionally (`letta/agents/letta_agent_v3.py:1511-1540`); (b) tool calls are aggregated into one Step — there is no per-tool-call row, no per-tool-call status, and tool execution errors flow through the LLM as `func_response` strings rather than as `FAILED` rows; (c) "Run" is sometimes conflated with the older "Job" concept (`letta/schemas/job.py:13,30,46`) and the Lettuce fallback path (`letta/services/run_manager.py:109-128`) only runs at *read* time, leaving Run status potentially stale during a crash window; (d) the `run_id` referenced by messages (`letta/orm/message.py:51-53`) is set to `None` defensively if the run does not exist (`letta/services/message_manager.py:548-569`), so a crashed run can leave its messages dangling without a run; (e) no test directly simulates a crash mid-step and verifies recovery semantics, so the `rollback to previous state` claim in `letta/agents/letta_agent_v3.py:1513` is by convention rather than by enforced contract.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Step persistence schema (Pydantic + ORM) | `Step` with `id`, `run_id`, `agent_id`, `status`, `error_type`, `error_data`, `stop_reason`, `trace_id`, `request_id` | `letta/schemas/step.py:16-65`; `letta/orm/step.py:20-98` |
| Step id prefix is a first-class typed enum | `PrimitiveType.STEP = "step"` | `letta/schemas/enums.py:27` |
| Step id generation | `generate_step_id(uid)` → `f"step-{uuid4()}"` | `letta/agents/helpers.py:373-376` |
| Step status four-state enum | `StepStatus ∈ {PENDING, SUCCESS, FAILED, CANCELLED}` | `letta/schemas/enums.py:268-274` |
| Run lifecycle enum | `RunStatus ∈ {created, running, completed, failed, cancelled}` | `letta/schemas/enums.py:152-161` |
| Job vs Run split (legacy "Job" still used for batch/long-running) | `Job`, `BatchJob`, `Run`; `JobType ∈ {JOB, RUN, BATCH}` | `letta/schemas/job.py:19-89`; `letta/schemas/enums.py:227-231` |
| Step is created with PENDING at step start | `step_manager.log_step_async(... status=StepStatus.PENDING ...)` | `letta/agents/letta_agent_v2.py:946-962` |
| Step is updated to SUCCESS at step end (when no exception) | `step_manager.update_step_success_async` writes status=SUCCESS, usage, stop_reason | `letta/services/step_manager.py:418-476`; `letta/agents/letta_agent_v2.py:1022-1033` |
| Step is updated to FAILED on caught exception | `update_step_error_async` writes status=FAILED, error_type, error_data, stop_reason, fires webhook | `letta/services/step_manager.py:368-414` |
| Step is updated to CANCELLED | `update_step_cancelled_async` writes status=CANCELLED, fires webhook | `letta/services/step_manager.py:519-555` |
| Step status update only fires webhook outside the DB session | `webhook_service.notify_step_complete(step_id)` after the session closes | `letta/services/step_manager.py:411-414,473-476,552-555` |
| Step is updated to log resolved model (post-auto-mode) | `update_step_resolved_model_async` patches provider/model after auto-routing | `letta/services/step_manager.py:480-514` |
| Step progression states (in-memory FSM) | `StepProgression ∈ {START, STREAM_RECEIVED, RESPONSE_RECEIVED, STEP_LOGGED, LOGGED_TRACE, FINISHED}` | `letta/schemas/step.py:68-74` |
| Step progression driven via checkpoint helpers | `_step_checkpoint_start`, `_step_checkpoint_llm_request_start`, `_step_checkpoint_llm_request_finish`, `_step_checkpoint_finish` | `letta/agents/letta_agent_v2.py:940-1034` |
| Step trace id captured from current OTel span | `trace_id = get_trace_id()` inside `log_step` and `log_step_async` | `letta/services/step_manager.py:157,219` |
| Step request id captured from middleware | `request_id = get_request_id()` | `letta/services/step_manager.py:158,220` |
| Agent-step OTel span opens at step start | `tracer.start_span("agent_step", start_time=step_start_ns); agent_step_span.set_attributes({"step_id": step_id})` | `letta/agents/letta_agent_v2.py:944-945` |
| Request-level OTel span opens at agent entry | `tracer.start_span("time_to_first_token", start_time=request_start_timestamp_ns)` | `letta/agents/letta_agent_v2.py:916-923` |
| OTel decorator adds per-method spans | `@trace_method` decorator wraps function calls in `tracer.start_as_current_span` | `letta/otel/tracing.py:228-435` |
| OTel cancellation handling (asyncio.CancelledError tagged) | `_trace_error_handler` and explicit cancellation in `async_wrapper` | `letta/otel/tracing.py:401-421` |
| Retry is inner-LLM, not step-level | `for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1):` wraps the LLM call only | `letta/agents/letta_agent_v3.py:1093` |
| LLM error class taxonomy | `LLMEmptyResponseError`, `LLMRateLimitError`, `LLMServerError`, `LLMProviderOverloaded`, `LLMError`, `ContextWindowExceededError` | `letta/errors.py` (imported at `letta/agents/letta_agent_v3.py:24-31`) |
| LLM-rate-limit retry helper (used in legacy `create`) | `retry_with_exponential_backoff` with `error_codes=(429,)` and `max_retries=20` | `letta/llm_api/llm_api_tools.py:38-118` |
| Fallback-model routing inside retry loop | On `LLMRateLimitError/LLMServerError/LLMProviderOverloaded` with a fallback handle, swap config and continue | `letta/agents/letta_agent_v3.py:1183-1211` |
| Context-window-exceeded retry triggers compaction, not LLM retry | Inner `for llm_request_attempt in range(... + 1)`: `compact` then `continue` | `letta/agents/letta_agent_v3.py:1218-1299` |
| Step-level failure aborts agent loop unconditionally | `self.should_continue = False` on any exception in `_step`; messages are NOT persisted | `letta/agents/letta_agent_v3.py:1511-1540` |
| Exception path always logs partial step metrics | `_record_step_metrics` fires from the `finally` block with timing so far | `letta/agents/letta_agent_v3.py:1582-1590` |
| Run-cancellation check at start of each step | `if run_id and await self._check_run_cancellation(run_id): return` with `RunStatus.cancelled` check | `letta/agents/letta_agent_v3.py:1031-1035`; `letta/agents/letta_agent_v2.py:750-757` |
| Run lifecycle state machine (no transitions after terminal) | `run.status in {RunStatus.completed}` → only `cancelled` accepted; failed/cancelled are immutable | `letta/services/run_manager.py:342-356` |
| Run callback only fires on first terminal update | `needs_callback = is_terminal_update and not_completed_before and run.callback_url` | `letta/services/run_manager.py:336-339` |
| Run is created with `RunMetrics(num_steps=0)` and timestamp | `metrics = RunMetricsModel(id=run.id, run_start_ns=int(time.time()*1e9), num_steps=0)` | `letta/services/run_manager.py:78-86` |
| Run is updated to terminal status by the router | `update_run_by_id_async` with `RunUpdate(status=run_status, stop_reason=stop_reason, metadata=...)` | `letta/server/rest_api/routers/v1/agents.py:1812-1826` |
| Router creates a Run only if `settings.track_agent_run` is true | `if settings.track_agent_run: runs_manager.create_run(...)` else `run = None` | `letta/server/rest_api/routers/v1/agents.py:1747-1761` |
| Lettuce fallback updates Run status at *read* time | `get_run_with_status` calls `LettuceClient.get_status` when `run.metadata.get("lettuce")` | `letta/services/run_manager.py:104-130` |
| Message persistence is batched and tied to step/run | `create_many_messages_async` writes messages with run_id, project_id, template_id; run_id nulled if Run missing | `letta/services/message_manager.py:477-576` |
| Message schema has step_id and run_id fields | `step_id: Optional[str]`, `run_id: Optional[str]` on `Message` | `letta/schemas/message.py:292-293` |
| Message ORM has indexed step_id and run_id | `Index("idx_messages_step_id", "step_id")`, `Index("ix_messages_run_sequence", "run_id", "sequence_id")` | `letta/orm/message.py:35-36` |
| Messages link to Step via FK with `ON DELETE SET NULL` | `step_id = ForeignKey("steps.id", ondelete="SET NULL")` | `letta/orm/message.py:48-50` |
| Messages link to Run via FK with `ON DELETE SET NULL` | `run_id = ForeignKey("runs.id", ondelete="SET NULL")` | `letta/orm/message.py:51-53` |
| In-context messages tracked on Agent or per Conversation | `agent.message_ids` OR `conversation_messages` table | `letta/agents/letta_agent_v3.py:807-813`; `letta/schemas/conversation.py:18-19` |
| Conversation lock to serialize concurrent runs | `redis_client.release_conversation_lock(conversation_id)` on terminal run | `letta/services/run_manager.py:391-396` |
| Step metrics row separate from Step row | `StepMetrics(id=step_id, step_start_ns, llm_request_start_ns, llm_request_ns, tool_execution_ns, step_ns)` | `letta/schemas/step_metrics.py:13-26` |
| Tool execution timing captured per-step | `step_metrics.tool_execution_ns = max(dt for _, dt in results)` after parallel/serial tool run | `letta/agents/letta_agent_v3.py:1864-1866` |
| Parallel vs serial tool execution | `target_tool.enable_parallel_execution` partitions into `parallel_items` vs `serial_items` | `letta/agents/letta_agent_v3.py:1840-1862` |
| Tool rule enforcement (run_first, exit_loop, etc.) | `ToolRulesSolver.should_force_tool_call()`, `_decide_continuation` | `letta/agents/letta_agent_v3.py:956-963`; `letta/agents/letta_agent_v3.py:1904-1911` |
| Multi-tool aggregation into single Step | `if len(exec_specs) == 1: ... else: parallel_items + serial_items` — no per-tool row | `letta/agents/letta_agent_v3.py:1840-1862` |
| Approval pauses the Step mid-execution | `_handle_ai_response` returns `requires_approval` stop reason, no further tool execution | `letta/agents/letta_agent_v3.py:1697-1709` |
| Approval requests reuse same step_id on resume | `step_id = approval_request.step_id` (else generate new) | `letta/agents/letta_agent_v3.py:1019-1027` |
| Compaction is part of the same Step | `compact_messages` inside `_step`, persisted with same `step_id` | `letta/agents/letta_agent_v3.py:1241-1272, 1500-1505` |
| Step metrics are sent to StepManager (best-effort) | `_record_step_metrics` uses `safe_create_task` to fire-and-forget metrics write | `letta/agents/letta_agent_v2.py:1412-1434` |
| Status lifecycle tests | Tests assert PENDING → FAILED, PENDING → SUCCESS, PENDING → CANCELLED transitions | `tests/test_managers.py:9593-9691` |
| Cancellation test (Redis-backed) | `test_background_streaming_cancellation` flips run to `JobStatus.cancelled` | `tests/integration_test_cancellation.py:171-200` |
| `DEFAULT_MAX_STEPS` is the recursion limit per Run | `DEFAULT_MAX_STEPS = 50` | `letta/constants.py:75` |
| Max-steps exhaustion produces `StopReasonType.max_steps` | `self.stop_reason = LettaStopReason(stop_reason=StopReasonType.max_steps.value)` on last iter | `letta/agents/letta_agent_v3.py:394-395` |
| Streaming adapter yields chunks inside a Step | `stream()` opens `request_span` at top, `_step` yields chunks, `request_span` ends in `finally` | `letta/agents/letta_agent_v3.py:443-740` |
| Webhook fires on step SUCCESS/FAIL/CANCEL | `WebhookService.notify_step_complete(step_id)` after session close | `letta/services/step_manager.py:413, 475, 554` |
| Webhook fires on run terminal | `_dispatch_callback_async` POSTs to `callback_url` | `letta/services/run_manager.py:483-500` |
| Run ↔ Step ↔ Message cascade delete | `relationship("Step", cascade="all, delete-orphan")` on Run, `relationship("Message", cascade="save-update")` on Step | `letta/orm/run.py:76-77`; `letta/orm/step.py:95-98` |
| Run `completed_at` auto-set on terminal update | `update.completed_at = get_utc_time().replace(tzinfo=None)` if missing | `letta/services/run_manager.py:362-364` |

## Answers to Dimension Questions

### 1. What is the atomic unit of execution?

There are three layered atomic units, named explicitly in the codebase:

- **Step** (`letta/schemas/step.py:16-65`, `letta/orm/step.py:20-98`): one LLM request + its synchronously executed tool calls + optional in-step compaction. Bounded by `StepStatus` (`letta/schemas/enums.py:268-274`). The Step is the persisted unit — a row is inserted at the start with `PENDING` (`letta/agents/letta_agent_v2.py:946-962`) and updated to `SUCCESS`/`FAILED`/`CANCELLED` (`letta/services/step_manager.py:368-555`).
- **Run** (`letta/schemas/run.py:17-50`, `letta/orm/run.py:22-77`): one HTTP request (or one background task) that drives the agent loop, owning a sequence of Steps. Bounded by `RunStatus ∈ {created, running, completed, failed, cancelled}` (`letta/schemas/enums.py:152-161`). The Run is the lifecycle owner — Steps are children of a Run (`letta/orm/step.py:39-41`).
- **Message** (`letta/schemas/message.py:252-310`, `letta/orm/message.py:23-97`): the message-level atom. Messages are written in *batches* per Step (`letta/services/message_manager.py:477-576`), each tagged with the parent step_id and run_id.

In the streaming path, a Run is opened before the loop (`letta/server/rest_api/routers/v1/agents.py:1747-1759`), Steps are opened inside `_step` (`letta/agents/letta_agent_v3.py:1037-1040`), and messages are persisted only when the Step completes successfully (`letta/agents/letta_agent_v3.py:1404-1410`).

### 2. Is the atomic unit the same for persistence, tracing, retry, and UI?

**No.** The four axes use different boundaries:

- **Persistence**: the **Step** is the atomic row. A `Step` row + its `StepMetrics` row are written at start; messages are appended at end. The DB enforces FK cascade: `Run` owns `Step` (`cascade="all, delete-orphan"`, `letta/orm/run.py:76`) and `Step` has `Message` backref (`cascade="save-update"`, `letta/orm/step.py:95`).
- **Retry**: the **LLM request** inside a Step is the retried unit (`for llm_request_attempt in range(summarizer_settings.max_summarizer_retries + 1):`, `letta/agents/letta_agent_v3.py:1093`). The retry scope also includes a fallback-model swap (`letta/agents/letta_agent_v3.py:1183-1211`) and a compaction-then-retry path (`letta/agents/letta_agent_v3.py:1218-1299`). The legacy `create` helper adds an HTTP-status retry layer on top (`retry_with_exponential_backoff`, `letta/llm_api/llm_api_tools.py:38-118`).
- **Tracing**: the **Step** is the OTel span boundary (`tracer.start_span("agent_step", ...)`, `letta/agents/letta_agent_v2.py:944`). The Run is implicit (via trace_id correlation only — there is no `run_span`). The decorator `@trace_method` (`letta/otel/tracing.py:228`) adds finer per-method spans.
- **UI**: the **Step** is the user-visible iteration boundary in `stream()` mode (yields between Steps via `_create_letta_response` and the SSE chunks), but the **Run** is the user-visible request boundary (one `Run` → one `LettaResponse` returned at the end, `letta/agents/letta_agent_v3.py:426-441`).

So persistence/tracing/UI agree on Step as the unit, but retry is finer (LLM-call-within-Step). This is similar in spirit to the openai-agents-sdk's "turn is persisted, model call is retried" pattern, but with Step replacing turn.

### 3. Can partially completed steps exist?

**Yes, but the persistence layer is designed to make them observable rather than recoverable.** Three layers of partial state are possible:

- **Step row is `PENDING` but messages are unwritten**: this is the canonical mid-step state. The `Step` row is inserted at `_step_checkpoint_start` (`letta/agents/letta_agent_v2.py:947-962`), but messages are only persisted in `_checkpoint_messages` at the end of a successful Step (`letta/agents/letta_agent_v3.py:1404-1410`). If the process crashes between, the Step row sits at `PENDING` forever. There is no startup sweep that re-classifies stale `PENDING` Steps as failed.
- **Within the Step, tool execution can partially complete**: multiple tools run via `asyncio.gather` for parallel-eligible tools (`letta/agents/letta_agent_v3.py:1857`); the longest tool's `dt` becomes `tool_execution_ns` (`letta/agents/letta_agent_v3.py:1865-1866`). A tool that crashes is captured as a `ToolExecutionResult(status="error", func_return=err_msg)` and persisted as a normal tool message, not as a separate failed row (`letta/agents/letta_agent_v3.py:1822-1824`).
- **The exception `finally` block always updates `step_progression`**: if the Step fails before reaching `_step_checkpoint_finish`, the `finally` block updates the row to `FAILED` with the exception details (`letta/agents/letta_agent_v3.py:1555-1567`). This is a *best-effort* update — if the DB is unavailable, the Step stays `PENDING`.

Partial state is therefore explicitly observable (you can tell a Step was attempted via the row), but it is not automatically resumed. There is no "retry from last completed tool" — a crashed Step must be re-executed as a whole by the next request, generating a new `step_id`.

### 4. What happens if a crash occurs mid-step?

The system follows a **checkpoint-on-success** pattern:

- **At step start**: Step row is created with `PENDING`, `StepMetrics` row is created with `step_start_ns` (`letta/agents/letta_agent_v2.py:946-966`). An OTel span `agent_step` is opened (`letta/agents/letta_agent_v2.py:944-945`).
- **During the step**: messages are NOT persisted. Tool returns are accumulated in `self.response_messages` (`letta/agents/letta_agent_v3.py:1369`); the in-memory `self.in_context_messages` is mutated but not yet checkpointed.
- **At step success**: messages are persisted via `_checkpoint_messages` (`letta/agents/letta_agent_v3.py:1405-1410`), `in_context_messages` updated (`letta/agents/letta_agent_v3.py:809-814`), Step row updated to `SUCCESS` (`letta/agents/letta_agent_v2.py:1022-1033`), StepMetrics finalized.
- **On exception**: `caught_exception` is captured, `self.should_continue = False` is forced, `raise e` re-raises (`letta/agents/letta_agent_v3.py:1511-1540`). The `finally` block then either finalizes via `_step_checkpoint_finish` (if the Step made it to `FINISHED` progression) or calls `update_step_error_async` with the exception details (`letta/agents/letta_agent_v3.py:1555-1567`).

There is **no automatic resume mechanism**. A crashed step leaves a `PENDING` Step row (or `FAILED` if the `finally` block ran) and no messages. The next agent request from the same caller starts fresh with a new Step.

Cross-step durability: a Run in `RunStatus.cancelled` will short-circuit at the next step entry via `_check_run_cancellation` (`letta/agents/letta_agent_v3.py:1031-1035`, `letta/agents/letta_agent_v2.py:750-757`). This is a soft-cancel — the Step is allowed to drain via `self.stop_reason = LettaStopReason(stop_reason=StopReasonType.cancelled.value)`. There is no test in the repo that directly simulates a mid-step crash and verifies recovery (searched `tests/integration_test_cancellation.py` and `tests/test_managers.py`).

### 5. Are tool calls their own atomic units?

**No, not as first-class persisted units.** Tool calls are aggregated into a Step:

- The `Step` row has no per-tool breakdown; tool metadata is captured via the `messages` that the Step produces (`letta/orm/step.py:20-98`).
- Multiple tool calls in a single LLM response are executed via `asyncio.gather` (`letta/agents/letta_agent_v3.py:1857`), serialized by `target_tool.enable_parallel_execution` (`letta/agents/letta_agent_v3.py:1847-1852`).
- Tool execution errors are surfaced as `ToolExecutionResult(status="error", func_return=err_msg)` (`letta/agents/letta_agent_v3.py:1823-1824`) and persisted as a normal `role=tool` message; there is no `tool_call_id`-keyed retry.
- The Step's `tool_execution_ns` is the **max** of the per-tool durations (`letta/agents/letta_agent_v3.py:1866`), not a per-tool breakdown.
- Client-side tools execute outside the server (`letta/agents/letta_agent_v3.py:248-249`), so the server has no retry story for them.

There is one tool-adjacent primitive that *almost* behaves like a sub-step atomicity boundary: **approval gating**. When a tool requires approval, `_handle_ai_response` returns `StopReasonType.requires_approval` (`letta/agents/letta_agent_v3.py:1709`), which makes the Run terminal pending human input. On resume, the approval request's existing `step_id` is reused (`letta/agents/letta_agent_v3.py:1019-1027`), so the persisted Step is continuous across the approval pause.

## Architectural Decisions

- **Step as the persisted atomic unit, not the LLM call.** Documented in `letta/schemas/step.py:21-22`: "The unique identifier of the run that this step belongs to. Only included for async calls." Steps group LLM call + tool execution + compaction into one observability + persistence unit, similar to how openai-agents-sdk's `SingleStepResult` groups a model call with its tools (`src/agents/run_internal/run_steps.py:178`).
- **Step status as a four-state lifecycle.** `StepStatus.PENDING/SUCCESS/FAILED/CANCELLED` (`letta/schemas/enums.py:268-274`) maps to four separate update methods (`log_step_async`, `update_step_success_async`, `update_step_error_async`, `update_step_cancelled_async` — `letta/services/step_manager.py:181-555`). This is heavier than the openai-agents-sdk's binary success/failure but lighter than langgraph's checkpoint+pending-writes split.
- **Run as the lifecycle owner, separate from the legacy Job.** `Run` is the modern concept (`letta/schemas/run.py`), `Job` is the older concept retained for batch and lettuce paths (`letta/schemas/job.py:46-85`). The enum `JobType ∈ {JOB, RUN, BATCH}` (`letta/schemas/enums.py:227-231`) lets the system distinguish these at the DB level.
- **Compaction is folded into the Step.** When the LLM call raises `ContextWindowExceededError`, the agent calls `compact_messages`, persists the summary with the same `step_id`, and retries — all inside the same Step (`letta/agents/letta_agent_v3.py:1218-1299`). This makes compaction visible in the Step's metrics and trace.
- **Webhook firing outside the DB session.** `WebhookService.notify_step_complete(step_id)` is called *after* the SQLAlchemy session closes (`letta/services/step_manager.py:411-414,473-476,552-555`). This decouples webhook reliability from DB transaction boundaries.
- **Auto-mode routing updates the Step row, not a separate trace.** When the agent resolves an auto-mode model handle, `update_step_resolved_model_async` patches the Step row with the actual provider/model/endpoint (`letta/services/step_manager.py:480-514`, `letta/agents/letta_agent_v3.py:1079-1087`). This guarantees billing observability even if the LLM call subsequently fails.
- **Conversation as an orthogonal isolation axis, not an atomic unit.** A `Conversation` owns its own `in_context_message_ids` and `isolated_block_ids` (`letta/schemas/conversation.py:18-23`), so two conversations on the same agent can have different in-context messages. A Run can be `conversation_id`-scoped or default; the Step is the same regardless. Conversation locks are Redis-managed (`letta/services/run_manager.py:391-396`).
- **Message write-batching per Step.** `create_many_messages_async` accepts a list of messages and inserts them in one session (`letta/services/message_manager.py:477-576`). Foreign keys are validated upfront, and `run_id` is nulled defensively if the Run is missing (`letta/services/message_manager.py:548-569`). This is a soft-fail rather than a hard-fail on stale Runs.
- **OTel correlation via `trace_id` and `request_id` columns on the Step row.** Captured at step-creation time (`letta/services/step_manager.py:157-158,219-220`) so a Step row is queryable by OTel trace.

## Notable Patterns

- **`StepProgression` in-memory FSM** (`letta/schemas/step.py:68-74`). Tracks six stages (`START → STREAM_RECEIVED → RESPONSE_RECEIVED → STEP_LOGGED → LOGGED_TRACE → FINISHED`) inside the Step execution, used for telemetry and to decide what kind of recovery to attempt in the `finally` block (`letta/agents/letta_agent_v3.py:1540-1589`). This is more granular than the persisted `StepStatus` and exists only for the duration of the Step.
- **Helper-method checkpoint pattern.** `_step_checkpoint_start`, `_step_checkpoint_llm_request_start`, `_step_checkpoint_llm_request_finish`, `_step_checkpoint_finish` (`letta/agents/letta_agent_v2.py:940-1034`) and `_request_checkpoint_start`, `_request_checkpoint_ttft`, `_request_checkpoint_finish` (`letta/agents/letta_agent_v2.py:916-938`) split the run into named, individually-traceable checkpoints. This is a cleaner separation than the openai-agents-sdk's try/finally span pattern.
- **Two database IDs cross-correlate a Step.** `step.trace_id` (OTel) and `step.request_id` (HTTP middleware) are both captured on the Step row, letting a single Step be reached from the trace UI, the HTTP log, and the Steps API.
- **Auto-resolved model patching.** When auto-mode routing resolves a different model mid-step, the Step is patched in place via `update_step_resolved_model_async` (`letta/services/step_manager.py:480-514`), so the billing trail reflects the actual model even if the Step fails downstream.
- **Step is created with `PENDING` for early observability.** The Step row exists before the LLM call fires, so the dashboard can show "Step started at X" before the response arrives. The OTel span opens at the same moment (`letta/agents/letta_agent_v2.py:944`).
- **Step metrics are a separate row.** `StepMetrics` (`letta/schemas/step_metrics.py:13-26`) decouples timing data from the Step's business fields, allowing `_record_step_metrics` to be fired-and-forgotten via `safe_create_task` (`letta/agents/letta_agent_v2.py:1412-1434`).
- **Webhook fired outside DB session.** Webhook calls happen after the SQLAlchemy session context manager closes (`letta/services/step_manager.py:411-414`), so the DB transaction does not hold open while the webhook is in-flight.
- **Conversational Redis lock.** `release_conversation_lock(conversation_id)` runs after the Run update commits (`letta/services/run_manager.py:391-396`). This serializes concurrent Runs targeting the same Conversation.

## Tradeoffs

- **Step is larger than the natural retry boundary.** The LLM call retries inside a Step, but a tool-execution failure cannot be retried inside a Step — the entire Step is aborted. This means a flaky tool can force an extra full LLM round-trip on the next Step.
- **No automatic resume of a crashed Step.** A `PENDING` Step row with no messages can sit in the DB forever. The system relies on the next user request to start a fresh Step. There is no scheduler that re-classifies stale `PENDING` Steps.
- **Tool calls have no individual observability.** `Step.tool_execution_ns` is a max, not a sum or breakdown (`letta/agents/letta_agent_v3.py:1866`). Operators cannot tell which tool in a multi-tool Step was slow without reading the messages.
- **Messages persist only on Step success.** A Step that crashes mid-flight writes no messages, leaving the Step row orphaned (`letta/agents/letta_agent_v3.py:1513`: "message persistence does not happen in the case of an exception (rollback to previous state)"). This is a deliberate atomicity choice, but it means post-mortem debugging requires the OTel trace + Step row, not the messages.
- **`Run` is sometimes conflated with `Job`.** `RunStatus` (`letta/schemas/enums.py:152-161`) and `JobStatus` (`letta/schemas/enums.py:133-149`) are separate enums with overlapping values; the legacy `Job`-based path in `letta/services/job_manager.py` (referenced in `tests/test_managers.py:46` as `RequestStatusUpdateInfo`) coexists with the newer `Run`-based path. The Lettuce fallback updates Run status at *read* time (`letta/services/run_manager.py:104-130`), which can return stale data.
- **Run can be null on messages if Run is deleted.** `letta/services/message_manager.py:548-569` defensively nulls `run_id` if the parent Run is missing. This preserves the message but loses the Run linkage, making audit-trail analysis harder.
- **Step `run_id` is nullable.** A Step can exist without a Run (`letta/orm/step.py:39-41`, `nullable=True`). In practice this happens when `settings.track_agent_run` is False (`letta/server/rest_api/routers/v1/agents.py:1760-1761`). This means a single Run sometimes contains Steps and sometimes contains orphan Steps.
- **Rollback on exception is by convention, not by contract.** The code says "rollback to previous state" (`letta/agents/letta_agent_v3.py:1513`), but this only holds if no side effects have occurred. Tool execution may have mutated external systems; messages are not persisted, but the OTel trace still records the attempt.
- **`StepStatus.CANCELLED` is set explicitly, not inferred.** There is no daemon that flips `PENDING` Steps to `CANCELLED` when a Run is cancelled mid-Step; the cancellation has to be propagated through the agent loop's `_check_run_cancellation` (`letta/agents/letta_agent_v3.py:1031-1035`), which only fires at Step boundaries.

## Failure Modes / Edge Cases

- **Step row `PENDING` forever on crash.** No startup sweep re-classifies stale Steps. Operators must detect and triage manually via the `GET /v1/steps` endpoint (`letta/services/step_manager.py:45-88`).
- **Run deleted while messages exist.** `letta/services/message_manager.py:548-569` nulls `run_id` to avoid a FK violation. The messages remain but lose their Run linkage.
- **Run cancelled mid-Step.** The Step is allowed to drain via `StopReasonType.cancelled`, but there is no guarantee the Step's `update_step_cancelled_async` is called. If the agent's `finally` block hits the `_step_checkpoint_finish` path, the Step is marked `SUCCESS` with `stop_reason=cancelled`, not `CANCELLED`. (`letta/agents/letta_agent_v3.py:1546-1574`)
- **Compaction fails inside the Step.** `SystemPromptTokenExceededError` causes `raise` from inside the retry loop, which propagates to the Step's exception path (`letta/agents/letta_agent_v3.py:1285-1290`). The Step row is updated to `FAILED`, but no compaction persists.
- **Auto-mode resolution succeeds, LLM fails.** `update_step_resolved_model_async` patches the Step with the resolved model (`letta/agents/letta_agent_v3.py:1079-1087`), then the LLM call fails — the Step is marked `FAILED` but the row reflects the resolved model, not the original. This is by design for billing but can confuse operators.
- **Fallback model retry exhausts.** After `max_summarizer_retries + 1` attempts, the loop raises (`letta/agents/letta_agent_v3.py:1297`). The Step row is updated to `FAILED` with `stop_reason=error` (`letta/agents/letta_agent_v3.py:1569-1573`).
- **`conversation_id` lock leak on crash.** `release_conversation_lock` runs in the run-update `finally`-equivalent path (`letta/services/run_manager.py:391-396`); if the process crashes before the update commits, the lock is held until the Redis TTL expires.
- **Tool rule violation mid-Step.** If `_decide_continuation` returns `tool_rule_violated=True`, the Step's `stop_reason` becomes `tool_rule`, but the Step row is still marked `SUCCESS` (`letta/agents/letta_agent_v3.py:1546-1553`). There is no `FAILED`-status for tool-rule failures.
- **`max_steps` reached.** `self.stop_reason = LettaStopReason(stop_reason=StopReasonType.max_steps.value)` is set, but the Step is `SUCCESS` with that stop_reason — not `FAILED` (`letta/agents/letta_agent_v3.py:394-395`).
- **Approval-request Step on resume.** The approval request's `step_id` is reused (`letta/agents/letta_agent_v3.py:1019-1027`), so all messages from the original Step and the resumed Step share a single Step row. This can be confusing in the Steps UI.
- **`step_id` referenced by messages is set in `_checkpoint_messages`** (`letta/agents/letta_agent_v3.py:773-776`), not at message creation. If the agent crashes between `self.response_messages.append(...)` and the checkpoint call, the in-memory messages have no `step_id`.

## Future Considerations

- **Sweep stale `PENDING` Steps.** A cron or startup sweep that flips Steps where `step_status=PENDING` for more than N seconds to `FAILED` with `stop_reason=error` would close the observability gap left by crashed processes.
- **Per-tool-call observability.** A `tool_call_id`-keyed row or a JSON column on `Step` capturing per-tool latency/status would let operators diagnose flaky tools without parsing messages.
- **Run auto-resume on crash.** If a Run has Steps in `PENDING` state and a new request arrives, the agent could pick up the unfinished Step's in-context messages and continue. This is blocked today because `_checkpoint_messages` was never called.
- **Transactional Step boundary.** Wrapping message creation, in-context update, and Step update in one DB transaction would close the inconsistency window where a Step is `SUCCESS` but the in-context list is stale.
- **Distinguish auto-mode from user-cancelled.** A separate `RunStatus.auto_resolved` or `StepStatus.auto_resolved` would clarify that a model swap happened, rather than inferring from `provider_name != original`.
- **Unify Run and Job status semantics.** `RunStatus` and `JobStatus` overlap; consolidating them into one enum with explicit sub-types would reduce confusion.
- **Reuse Step across approval pauses more explicitly.** Today the resumed Step keeps the same `step_id` but the resumed messages are added to it; a separate `Step.resume_count` column would clarify the resume history.
- **Conversation-level atomicity.** A failed Conversation Run currently orphans Steps; a Conversation-scoped atomicity primitive (parallel to Run but per-thread) would make concurrent conversation semantics cleaner.

## Questions / Gaps

- **What is the canonical atomic event for observability?** The system has `Step`, `StepMetrics`, OTel span `agent_step`, and OTel trace — but no unified "event" type spanning all four. No clear evidence found for a single event record joining all observability axes.
- **How does the system report a partially-flushed Step to a UI consumer?** `StepStatus.PENDING` is the only signal, but there is no `started_at`/`updated_at` distinction that a UI could use to render "Step is taking unusually long". No clear evidence found for an explicit `PENDING` step timeout or staleness indicator.
- **Is there a test for mid-step crash recovery?** Searched `tests/integration_test_cancellation.py` (which only tests Redis-based cancellation) and `tests/test_managers.py` (status transitions only). No clear evidence found for a test that crashes mid-`_step` and verifies recovery.
- **What happens when a Step is deleted but messages reference its `step_id`?** FK is `ON DELETE SET NULL` (`letta/orm/message.py:48-50`), so the messages lose their `step_id` silently. No clear evidence found for a cleanup or warning.
- **Is the Step's `request_id` always populated?** `get_request_id()` (`letta/services/step_manager.py:158,220`) returns whatever the middleware has set; outside an HTTP request context (e.g., a background Lettuce run), it may be `None`. No clear evidence found for how this is handled in the lettuce path.
- **What is the relationship between Lettuce-managed Runs and the in-process Step rows?** `letta/services/run_manager.py:104-130` updates Run status from Lettuce at read time but does not write Step rows. No clear evidence found for how Lettuce's per-task execution maps to Letta's Step atomicity model.
- **Is `StepProgression.FINISHED` reachable on a `requires_approval` Step?** The Step stops at the approval boundary, but the `finally` block can still call `_step_checkpoint_finish`. No clear evidence found for a test that pins down this transition.

---

Generated by `01.03-step-turn-and-task-atomicity` against `letta`.