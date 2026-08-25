# Source Analysis: crewai

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic, asyncio, OpenTelemetry, Plan-and-Execute Flow framework) |
| Analyzed | 2026-07-02 |

## Summary

CrewAI exposes **four overlapping atomic-unit vocabularies**, depending on which executor or flow is in use. The smallest meaningful "step" is a single agent-loop iteration (one LLM round-trip plus an optional native or text-parsed tool call), persisted only as event log entries. The durable unit is the `Task` (`task.py:114`) — it carries `id`, `start_time`, `end_time`, `output`, `retry_count`, `guardrail_retry_count`, `used_tools`, and `tools_errors` — and the task lifecycle is bounded by `TaskStartedEvent`/`TaskCompletedEvent`/`TaskFailedEvent`. When planning is enabled, a finer per-plan-step unit emerges (`TodoItem` at `utilities/planning_types.py:27` with `pending`/`running`/`completed`/`failed`), executed in isolation by `StepExecutor` (`agents/step_executor.py:63`) which builds a fresh `messages` list per todo (`agents/step_executor.py:233-247`) and returns a `StepResult` (`utilities/step_execution_context.py:44`) with `success`, `result`, `error`, `tool_calls_made`, `execution_time`. At the flow layer the atomic unit is the flow method invocation tracked via `MethodExecutionStartedEvent`/`MethodExecutionFinishedEvent` and the `_completed_methods: set[FlowMethodName]` set (`flow/runtime/__init__.py:997`).

The system explicitly **does not** guarantee a single atomic unit across persistence, tracing, retry, and UI. Each concern uses a different boundary:

- **Persistence**: `Task` (default `CheckpointConfig.on_events=["task_completed"]`, `state/checkpoint_config.py:171-175`) or full `RuntimeState` snapshot (`state/runtime.py:177`) when `*` is configured. The snapshot serializes the entity tree plus an `EventRecord` (`state/event_record.py:99`).
- **Tracing**: per-event `event_id` / `parent_event_id` / `previous_event_id` / `started_event_id` (`events/base_events.py:82-87`) — events are nodes in a directed graph (`state/event_record.py:64-225`); trace batches ship to a backend (`events/listeners/tracing/trace_batch_manager.py:73`).
- **Retry**: iteration counter inside the agent loop (`agents/crew_agent_executor.py:460`, `experimental/agent_executor.py:2113-2123`) bounded by `max_iter` (default 25, `agents/agent_builder/base_agent_executor.py:27`) plus an explicit guardrail retry budget (`Task.guardrail_max_retries` default 3, `task.py:273-276`).
- **UI**: `step_callback` callable (`crew_agent_executor.py:1485-1496`, `experimental/agent_executor.py` step_callback field), `task_callback`, `before_kickoff_callbacks`/`after_kickoff_callbacks`, plus the rich event stream consumed by the console formatter.

Tool calls are atomic at the event level: every invocation emits `ToolUsageStartedEvent` with `started_at` followed by either `ToolUsageFinishedEvent` with `started_at`/`finished_at` (`events/types/tool_usage_events.py:62-69`) or `ToolUsageErrorEvent`. Cache writes happen AFTER execution (`crew_agent_executor.py:1002-1005`), but the cache wrapper is not transactional — a crash between `available_functions[func_name](**(args_dict or {}))` and `self.tools_handler.cache.add(...)` leaves the result unrecorded and the tool re-executed on retry. Partial-step completion is observable via `iterations` (always advanced in `finally`, `crew_agent_executor.py:459-460`) and via the `output.messages` snapshot captured into `TaskOutput.messages` (`task.py:700`, `task_output.py:46-48`).

CrewAI is **not consistent across concerns**: it ships two parallel executors (`CrewAgentExecutor` for legacy ReAct at `agents/crew_agent_executor.py:98` carrying a `DeprecationWarning` at `crew_agent_executor.py:145-151`, and the new Flow-based `AgentExecutor` for Plan-and-Execute at `experimental/agent_executor.py:164`), each with different atomicity guarantees. The legacy executor's `messages: list[LLMMessage]` is a shared mutable list that can be half-written mid-iteration; the new executor pushes step isolation into `StepExecutionContext` (frozen dataclass carrying only `task_description`, `task_goal`, `dependency_results` at `utilities/step_execution_context.py:14`) and a fresh `messages` per `StepExecutor.execute()`. Mid-step crash recovery works only via the explicit `CheckpointConfig` plumbing (`state/checkpoint_listener.py:113-216`) — without it, a `kickoff()` crash mid-task leaves the previous tasks' outputs persisted in `_task_output_handler` but the current task unfinished, and there is no automatic resume. The plan-and-execute path adds `AgentExecutorState` (`experimental/agent_executor.py:126-161`) with `_finalize_lock` + `_finalize_called` to prevent double-finalize (`agent_executor.py:2264-2269`) and `_execution_lock` to prevent re-entrancy (`agent_executor.py:2736-2741`), which are explicit safety guards missing from the legacy path.

## Rating

**7/10** — Clear model with explicit interfaces (Task, TodoItem, StepExecutionContext, AgentExecutorState), event-scoped tool-call atomicity with start/finish timestamps, three-tier persistence (no checkpoint / task-completed checkpoint / every-event checkpoint), guardrail retry loop with bounded budget, and observable partial-completion markers (`iterations`, `TaskOutput.messages`, `TodoList.status`). Not 8+ because: (a) two parallel executors with different atomicity contracts and the legacy `CrewAgentExecutor` is `DeprecationWarning`'d but still primary on some code paths; (b) the default ReAct loop's `messages` list is shared mutable state with no transactional boundary between LLM call and message append; (c) tool call atomicity is event-level only — cache write is not transactional with execution; (d) `kickoff()` itself is not atomic across the task list — a crash mid-task leaves an inconsistent partial state with no automatic resume unless `CheckpointConfig` was explicitly opted in; (e) `runtime_step` / per-iteration state is not in the snapshot schema (only `messages` of the `BaseAgentExecutor` are persisted via `state/runtime.py` with `_resuming` flag in `crew_agent_executor.py:217`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Task entity (durable atomic unit) | `Task` with `id: uuid.UUID`, `start_time`, `end_time`, `output: TaskOutput`, `retry_count`, `guardrail_max_retries`, `used_tools`, `tools_errors` | `task.py:114-300` |
| Task lifecycle events | `TaskStartedEvent`, `TaskCompletedEvent`, `TaskFailedEvent` with `task_id`/`task_name` set in `__init__` | `events/types/task_events.py:24-57` |
| TaskStartedEvent emission | Emit `TaskStartedEvent(context=context, task=self)` unless `_resuming` | `task.py:660-665` |
| TaskCompletedEvent emission | Emit `TaskCompletedEvent(output=task_output, task=self)` after guardrails | `task.py:749-752` |
| TaskFailedEvent emission | Emit `TaskFailedEvent(error=str(e), task=self)` in except block | `task.py:755-757` |
| Iteration counter atomic advance | `self.iterations += 1` in `finally` block guarantees progress | `crew_agent_executor.py:459-460, 594-595, 1297-1298` |
| Max iteration bound | `BaseAgentExecutor.iterations: int`, `max_iter: int = 25` | `agents/agent_builder/base_agent_executor.py:26-27` |
| Max iteration check | `has_reached_max_iterations(self.iterations, self.max_iter)` per loop | `crew_agent_executor.py:343, 503, 1180, 1326` |
| Iteration-step callback (UI hook) | `_invoke_step_callback(formatted_answer)` after each iteration | `crew_agent_executor.py:431, 549, 561, 571, 1493-1496` |
| `_invoke_step_callback` impl | Calls `self.step_callback(formatted_answer)`, awaits if coroutine | `crew_agent_executor.py:1485-1510` |
| Step callback on async path | `await self._ainvoke_step_callback(formatted_answer)` | `crew_agent_executor.py:1269, 1371, 1383, 1393` |
| Plan-and-Execute todo unit | `TodoItem` with `status: Literal["pending","running","completed","failed"]`, `step_number`, `depends_on`, `result` | `utilities/planning_types.py:27-42` |
| Todo list status transitions | `mark_running`, `mark_completed`, `mark_failed`, `_dependencies_satisfied`, `get_ready_todos` | `utilities/planning_types.py:90-145` |
| Step isolation context (frozen) | `StepExecutionContext` frozen dataclass — task description, goal, dependency results only | `utilities/step_execution_context.py:14-41` |
| Step isolation result | `StepResult` with `success`, `result`, `error`, `tool_calls_made`, `execution_time` | `utilities/step_execution_context.py:44-63` |
| Step isolation comment | "These types mediate between the AgentExecutor (orchestrator) and StepExecutor (per-step worker). StepExecutionContext carries only final results from dependencies — never LLM message histories. StepResult carries only the outcome of a step — never internal execution traces." | `utilities/step_execution_context.py:1-6` |
| StepExecutor fresh messages per todo | `_build_isolated_messages(todo, context)` builds system+user messages only | `agents/step_executor.py:233-247` |
| StepExecutor loop bound | `for _ in range(max_step_iterations)` default 15 | `agents/step_executor.py:336, 546` |
| StepExecutor timeout | `step_timeout` checked at start of each iteration | `agents/step_executor.py:337-340, 547-554` |
| PlanStepStartedEvent emission | `_emit_plan_step_started(todo)` fires `PlanStepStartedEvent` on transition | `experimental/agent_executor.py:374-388, 416-420` |
| PlanStepCompletedEvent emission | `_emit_plan_step_completed(todo, success=..., result=..., error=...)` on terminal state | `experimental/agent_executor.py:390-414, 422-430` |
| AgentExecutorState (P&E state) | `AgentExecutorState` with `iterations`, `current_answer`, `is_finished`, `plan`, `todos`, `replan_count`, `observations`, `execution_log` | `experimental/agent_executor.py:126-161` |
| AgentExecutor finalize guard | `_finalize_lock` + `_finalize_called` flag prevents double-finalize | `experimental/agent_executor.py:2264-2269` |
| AgentExecutor re-entrancy guard | `_execution_lock` + `_is_executing` raises RuntimeError on concurrent invoke | `experimental/agent_executor.py:2736-2741` |
| Plan-and-Act paper reference | "Implements the direct-action execution pattern from Plan-and-Act (arxiv 2503.09572): the Executor receives one step description, makes a single LLM call, executes any tool call returned, and returns the result immediately." | `agents/step_executor.py:1-11` |
| StepExecutor no inner loop (P&A) | "There is no inner loop. Recovery from failure (retry, replan) is the responsibility of PlannerObserver and AgentExecutor" | `agents/step_executor.py:8-10` |
| PlannerObserver per-step analysis | `observe(completed_step, result, all_completed, remaining_todos)` → `StepObservation` | `agents/planner_observer.py:113-189` |
| Step observation event | `StepObservationStartedEvent`, `StepObservationCompletedEvent`, `StepObservationFailedEvent` | `events/types/observation_events.py:61-95` |
| Refinement events | `PlanRefinementEvent`, `PlanReplanTriggeredEvent`, `GoalAchievedEarlyEvent` | `events/types/observation_events.py:98-131` |
| Tool call started event | `ToolUsageStartedEvent` with `tool_name`, `tool_args`, `task_id`, `agent_role`, `plan_step_number` | `events/types/tool_usage_events.py:56-59, 31-44` |
| Tool call finished event | `ToolUsageFinishedEvent` with `started_at`, `finished_at`, `output`, `from_cache` | `events/types/tool_usage_events.py:62-69` |
| Tool call error event | `ToolUsageErrorEvent` with `error` | `events/types/tool_usage_events.py:72-76` |
| Tool call emission (legacy) | `crewai_event_bus.emit(self, ToolUsageStartedEvent(...))` before execution, `ToolUsageFinishedEvent` after | `crew_agent_executor.py:940-949, 1051-1063` |
| Tool call execution wrapper | `execute_tool_and_check_finality(agent_action, ...)` is the atomic-call boundary | `crew_agent_executor.py:415-426, 1253-1267` |
| Tool call parallel (native) | `ThreadPoolExecutor(max_workers=min(8, len(execution_plan)))` for parallel native tool calls | `crew_agent_executor.py:742-767` |
| Tool call sequential native | When `result_as_answer` or `max_usage_count` present in batch, fall back to sequential | `crew_agent_executor.py:698-724, 786-807` |
| Tool cache atomic gap | Cache read at `crew_agent_executor.py:929-936`, execute at `crew_agent_executor.py:989`, cache write at `crew_agent_executor.py:1002-1005` — no transactional guarantee | `crew_agent_executor.py:925-1006` |
| Event base with adjacency | `BaseEvent` with `event_id`, `parent_event_id`, `previous_event_id`, `triggered_by_event_id`, `started_event_id`, `emission_sequence` | `events/base_events.py:82-87` |
| Event emission sequence | `get_next_emission_sequence()` returns monotonic counter, `_emission_counter` ContextVar | `events/base_events.py:13-58` |
| EventRecord (event DAG) | `EventRecord.add(event)` wires `parent`/`child`, `trigger`/`triggered_by`, `previous`/`next`, `started`/`completed_by` edges | `state/event_record.py:99-225` |
| EventRecord RWLock | `RWLock` for thread-safe lookup during emit | `state/event_record.py:108, 119-146` |
| RuntimeState (durable snapshot) | `RuntimeState(RootModel)` with `_event_record`, `_checkpoint_id`, `_parent_id`, `_branch` | `state/runtime.py:177-498` |
| RuntimeState.checkpoint | Writes entire entity tree + event record to JSON/SQLite provider | `state/runtime.py:286-350` |
| RuntimeState.fork | Creates branch with `parent_branch`/`parent_checkpoint_id` lineage | `state/runtime.py:352-389` |
| RuntimeState.from_checkpoint | Restore via `model_validate_json` with `_migrate` for version migrations | `state/runtime.py:391-497` |
| Checkpoint default trigger | `CheckpointConfig.on_events: list = ["task_completed"]` | `state/checkpoint_config.py:171-175` |
| Checkpoint on every event | `["*"]` triggers checkpoint on every event | `state/checkpoint_config.py:174` |
| Auto-checkpoint handler | `_on_any_event` registered lazily on every BaseEvent subclass | `state/checkpoint_listener.py:229-270` |
| Lineage chaining | `_chain_lineage` updates `_checkpoint_id` / `_parent_id` after each write | `state/runtime.py:216-227` |
| Restore `_resuming` flag | `executor._resuming = True` triggers message-reuse path on next `invoke` | `agents/agent_builder/base_agent.py:447-449, crew.py:478-487` |
| Restore event scope | `_restore_event_scope` rebuilds scope stack from event record | `crew.py:547-575, base_agent.py:467-490` |
| Restore emission counter | `set_emission_counter(max_seq)` from event record | `base_agent.py:488-490, crew.py:574-575` |
| Guardrail retry budget | `guardrail_max_retries: int = 3`, `retry_count: int = 0`, `_guardrail_retry_counts: dict[int, int]` | `task.py:273-294` |
| Guardrail retry loop | `for attempt in range(max_attempts)` with `process_guardrail`, re-executes task on failure | `task.py:1264-1322, 1374-1428` |
| Guardrail emit events | `LLMGuardrailStartedEvent`, `LLMGuardrailCompletedEvent` via `process_guardrail` | `utilities/guardrail.py:160-186` |
| Output parser retry | `handle_output_parser_exception` triggers via `log_error_after: int = 3`, no explicit count limit | `crew_agent_executor.py:434-442, 1272-1280` |
| Context length overflow | `is_context_length_exceeded` triggers `handle_context_length` which truncates `messages` in-place | `crew_agent_executor.py:447-457, 1285-1294` |
| LiteAgent iteration counter | `self._iterations` advanced in `finally`, `self._iterations += 1` | `lite_agent.py:520, 873-964` |
| LiteAgent guardrail retry | `_guardrail_retry_count` + `guardrail_max_retries`, recursive `_execute_core` | `lite_agent.py:706-727` |
| LiteAgent execution loop | `_invoke_loop(response_model)` while not `AgentFinish` | `lite_agent.py:860-972` |
| Native tool message format | `_append_assistant_tool_calls_message` emits `role: assistant` with `tool_calls: [{id, type, function:{name, arguments}}]` | `crew_agent_executor.py:843-866` |
| Tool result message format | `_append_tool_result_and_check_finality` emits `role: tool` with `tool_call_id`, `name`, `content` | `crew_agent_executor.py:1073-1107` |
| Step executor tool events | `ToolUsageStartedEvent`/`ToolUsageFinishedEvent` emitted with `plan_step_number`/`plan_step_description` | `agents/step_executor.py:376-387, 428-442` |
| Step tool validation | `_validate_expected_tool_usage` raises `ValueError` if `todo.tool_to_use` not called | `agents/step_executor.py:504-526` |
| Trace batch (backend trace unit) | `TraceBatch` with `events` array, `is_final_batch`, `batch_sequence`, `ephemeral_trace_id` | `events/listeners/tracing/trace_batch_manager.py:65-176` |
| Trace batch finalization | `finalize_batch()` waits for pending handlers, sorts by `emission_sequence`, ships to backend | `events/listeners/tracing/trace_batch_manager.py:320-377` |
| Trace batch failure handling | On send failure, `_mark_batch_as_failed` and `events will be lost` | `events/listeners/tracing/trace_batch_manager.py:301-318` |
| Tracing event registration | TraceCollectionListener registers 50+ handlers on key event classes | `events/listeners/tracing/trace_listener.py:228-510` |
| Crew kickoff lifecycle | `_run_sequential_process` → `_execute_tasks` → per-task `task.execute_sync` | `crew.py:1485-1598` |
| Crew async kickoff | `_arun_sequential_process` → `_aexecute_tasks` → per-task `task.aexecute_sync` | `crew.py:1308-1425` |
| Execution log in-memory | `_store_execution_log` appends to `_task_output_handler` (reset on `kickoff_for_each`) | `crew.py:1455-1483, 1107` |
| Async task execution | `asyncio.create_task(task.aexecute_sync(...))` then `await asyncio.gather(...)` | `crew.py:1360-1388, 1411-1425` |
| ConditionalTask skip | `check_conditional_skip` may skip task entirely | `crew.py:1392-1409, 1600-1614, crews/utils.py:197` |
| Async kickoff thread safety | `kickoff_async` wraps sync `kickoff` in `asyncio.to_thread` (not native async) | `crew.py:1143-1162` |
| Tool hook system | `before_tool_call_hooks`/`after_tool_call_hooks` run before/after every call | `crew_agent_executor.py:955-1048` |
| Tool hook block | `hook_result is False` from before-hook aborts call with `Tool execution blocked by hook` | `crew_agent_executor.py:967-969, 977-979` |
| LLM hook system | `before_llm_call_hooks`/`after_llm_call_hooks` wrap every LLM call | `crew_agent_executor.py:152-155` |
| RPM controller | `enforce_rpm_limit(self.request_within_rpm_limit)` blocks (no retry) | `crew_agent_executor.py:354, 515, 1191, rpm_controller.py:38-65` |
| Migration for checkpoint backward compat | `_migrate` patches missing discriminator fields based on `crewai_version` | `state/runtime.py:89-167` |
| Crash mid-execution ReAct | `invoke` resets `messages = []` and `iterations = 0` on entry unless `_resuming` | `crew_agent_executor.py:217-223` |
| Crash mid-execution P&E | `AgentExecutor.invoke` resets `_finalize_called`, `messages`, `iterations`, `todos`, `replan_count`, etc. | `experimental/agent_executor.py:2745-2760` |
| StepExecutionContext example | "Contains only the information the Executor needs to complete one step: the task description, goal, and final results from dependency steps. No LLM message history, no execution traces, no shared mutable state." | `utilities/step_execution_context.py:21-26` |
| StepExecutor comment on Plan-and-Act | "Executes a SINGLE todo item using direct-action execution... Execution pattern (per Plan-and-Act, arxiv 2503.09572): 1. Build messages from todo + context, 2. Call LLM once, 3. If tool call → execute it → return tool result, 4. If text answer → return it directly" | `agents/step_executor.py:64-73` |
| StepResult audit-only | "Tool call details are for audit logging only — they are NOT passed to subsequent steps or the Planner." | `utilities/step_execution_context.py:48-51` |
| `from_checkpoint` task resume | `_get_execution_start_index` returns first task where `task.output is None` | `crew.py:1521-1527, 1333-1335` |
| Started task detection | Iterates `state.event_record.nodes.values()` for `task_started` events to determine resumed tasks | `crew.py:459-470` |
| Resume skips TaskStarted emit | `if not (executor and executor._resuming and resume_task_scope(...))` | `task.py:660-665` |
| Execution scope event tracking | `set_last_event_id`, `set_emission_counter`, `push_event_scope`/`pop_event_scope` on ContextVars | `events/base_events.py:13-58, events/event_context.py` |
| CLI replay | `kickoff(from_checkpoint=CheckpointConfig(restore_from=...))` dispatches to restored `kickoff` | `crew.py:997-1000, 1130-1134` |

## Answers to Dimension Questions

### 1. What is the atomic unit of execution?

There are **three co-existing atomic units**, depending on the executor:

- **Iteration** (ReAct legacy path, `CrewAgentExecutor`): one full agent-loop iteration = one LLM call + zero or one tool call (native OR text-parsed). Counter is `self.iterations: int` (`agents/agent_builder/base_agent_executor.py:26`), bounded by `self.max_iter: int = 25` (`agents/agent_builder/base_agent_executor.py:27`). Each iteration increments in the `finally` block (`crew_agent_executor.py:459-460, 594-595, 1297-1298`) so the counter is always advanced even on exception. The user-facing step hook is `step_callback`, called at `crew_agent_executor.py:431, 549, 561, 571, 1269`.
- **Plan step** (Plan-and-Execute path, `experimental/AgentExecutor`): one `TodoItem` (`utilities/planning_types.py:27-42`) with status `pending` → `running` → `completed`/`failed`. Each plan step is run in isolation by `StepExecutor.execute(todo, context, ...)` (`agents/step_executor.py:126-231`), which builds a **fresh `messages` list** (`agents/step_executor.py:233-247`) and returns a `StepResult` (`utilities/step_execution_context.py:44-63`). Bounded by `max_step_iterations: int = 15` (`agents/step_executor.py:130, 322, 533`) and an optional `step_timeout` seconds (`agents/step_executor.py:131, 337-340, 547-554`). The comment at `utilities/step_execution_context.py:1-6` and `agents/step_executor.py:8-11` makes isolation explicit: "These types mediate between the AgentExecutor (orchestrator) and StepExecutor (per-step worker). StepExecutionContext carries only final results from dependencies — never LLM message histories. StepResult carries only the outcome of a step — never internal execution traces."
- **Task** (Crew-level): the durable atomic unit, defined in `task.py:114-300` with `id: uuid.UUID`, `start_time`, `end_time`, `output: TaskOutput`, `retry_count`, `guardrail_max_retries`, `used_tools`, `tools_errors`, `delegations`. Lifecycle events are `TaskStartedEvent`/`TaskCompletedEvent`/`TaskFailedEvent` (`events/types/task_events.py:24-57`). `Task.execute_sync` and `aexecute_sync` wrap the entire agent-loop for that task in a `try/except/finally` that emits the lifecycle events (`task.py:572-757, 627-760`).

Tool calls are **not** standalone atomic units at the persistence layer, but they are atomic at the event layer: each call is bracketed by `ToolUsageStartedEvent` (with `started_at`) and either `ToolUsageFinishedEvent` (with `started_at`/`finished_at`) or `ToolUsageErrorEvent` (`events/types/tool_usage_events.py:56-76`).

### 2. Is the atomic unit the same for persistence, tracing, retry, and UI?

**No — the units differ by concern:**

| Concern | Atomic unit | Where defined |
|---|---|---|
| **Persistence (default)** | Task completion (`CheckpointConfig.on_events=["task_completed"]`) | `state/checkpoint_config.py:171-175` |
| **Persistence (every event)** | Full `RuntimeState` entity tree + `EventRecord` | `state/runtime.py:177-350, state/checkpoint_listener.py:113-216` |
| **Tracing (events)** | Single `BaseEvent` with `event_id` + adjacency edges (parent/child, triggered_by, started/completed_by) | `events/base_events.py:82-87, state/event_record.py:99-146` |
| **Tracing (backend batches)** | `TraceBatch` grouping events sorted by `emission_sequence` | `events/listeners/tracing/trace_batch_manager.py:65-377` |
| **Retry (agent loop)** | Iteration — bounded by `max_iter` | `crew_agent_executor.py:343, 503, 1180, 1326` |
| **Retry (guardrail)** | Task — bounded by `guardrail_max_retries` | `task.py:1262-1322, 1372-1428` |
| **Retry (output parser)** | Iteration — bounded by `log_error_after: int = 3` (soft limit) | `crew_agent_executor.py:434-442, 1272-1280` |
| **Retry (context length)** | None — `handle_context_length` truncates messages in-place | `crew_agent_executor.py:447-457` |
| **Retry (tool call)** | Per-tool `max_usage_count` — when exceeded returns synthetic error message | `crew_agent_executor.py:899-909` |
| **UI (verbose console)** | Event — `ConsoleFormatter` listens to all events | `events/utils/console_formatter.py` |
| **UI (callbacks)** | Iteration (step_callback) | `crew_agent_executor.py:1485-1510` |
| **UI (task callbacks)** | Task — `task.callback`, `crew.task_callback`, `crew.before_kickoff_callbacks`, `crew.after_kickoff_callbacks` | `task.py:154-156, crew.py:270-291, task.py:724-738, crew.py:1046-1047` |
| **Plan-step UI** | TodoItem — `PlanStepStartedEvent`/`PlanStepCompletedEvent` | `experimental/agent_executor.py:374-414` |

### 3. Can partially completed steps exist?

**Yes, by design and observable:**

- **Iteration half-write (ReAct)**: `self.messages.append(...)` happens AFTER LLM call but BEFORE next iteration (`crew_agent_executor.py:432, 1270`). A crash between append and tool-result append leaves the `messages` list with an `assistant` message but no corresponding `tool` message.
- **Iteration half-write (P&E)**: `_append_message_to_state(text)` is called in `tool_execution` path (`experimental/agent_executor.py:1638, 1621`); a crash leaves a partial message list, but `AgentExecutorState.messages` is reset on next `invoke` (`experimental/agent_executor.py:2748`).
- **Tool call partial**: `ToolUsageStartedEvent` has fired but `ToolUsageFinishedEvent` has not; on crash, no cleanup of `pending_tool_calls` list (`experimental/agent_executor.py:140`) — though list is cleared on `invoke` reset.
- **Guardrail partial**: `process_guardrail` is synchronous, but if the guardrail itself crashes, the retry loop increments `retry_count` and retries up to `guardrail_max_retries` (`task.py:1307, 1417`).
- **Plan replan partial**: `replace_pending_todos(new_items)` preserves `completed`/`failed`/`running` but replaces `pending` (`utilities/planning_types.py:185-195`). A crash mid-replan leaves mixed terminal/pending state.
- **Todo dependencies partial**: `depends_on` check waits for `completed` OR `failed` (`utilities/planning_types.py:120-131`), so a failed dependency does not block downstream — but downstream may then fail because the failed dependency's `result` is whatever the failing executor returned.
- **Async task mid-flight**: `task.execute_async` returns a `Future` (`task.py:596-610`); `_process_async_tasks` awaits them (`crew.py:1579-1583`). A crash during await loses the in-flight result.

The system explicitly says "what completed":
- `Task.output` (set only after `TaskCompletedEvent` is emitted, `task.py:721-752`)
- `TaskOutput.messages: list[LLMMessage]` (full conversation, `task_output.py:46-48`)
- `TaskOutput.output_format` (`RAW`/`JSON`/`PYDANTIC`, `tasks/output_format.py`)
- `TodoItem.status` (`pending`/`running`/`completed`/`failed`)
- `TodoItem.result: str | None`
- `EventRecord.all_nodes()` — every emitted event is recorded (`state/event_record.py:204-212`)
- `TaskExecutionLog` via `_task_output_handler` (`crew.py:1455-1483`)
- `Task.used_tools`, `tools_errors`, `delegations` counters (`task.py:141-143`, `task.py:1063-1071`)
- `Task.retry_count`, `Task._guardrail_retry_counts: dict[int, int]` (`task.py:276, 291-293`)

### 4. What happens if a crash occurs mid-step?

It depends on whether checkpointing is enabled:

**Without `CheckpointConfig`:**
- The agent-loop's `messages` list may be half-written. On next `invoke`, `messages = []` and `iterations = 0` (`crew_agent_executor.py:220-222`), and the agent re-runs from scratch with no memory of the partial state.
- The `Task` itself is never persisted mid-execution. If the process crashes before `TaskCompletedEvent` fires, the task's `output` is `None` and `start_time` is set but `end_time` is not (`task.py:579`).
- Crew-level execution log is in-memory only; if the process dies, it's lost.

**With `CheckpointConfig(on_events=["task_completed"])` (default):**
- A checkpoint fires AFTER each task completes. A crash mid-task means: previous tasks are persisted, current task is not. On resume, `_get_execution_start_index` returns the first task with `task.output is None` (`crew.py:1521-1527, 1333-1335`), and that task is re-executed from scratch.
- Resume sets `executor._resuming = True` (`crew.py:486`), which causes `_setup_messages` to skip fresh message setup and instead reuse the agent_executor's existing `messages` (`crew_agent_executor.py:217-219`).

**With `CheckpointConfig(on_events=["*"])`:**
- Every event triggers a snapshot via `RuntimeState.checkpoint(location)` (`state/runtime.py:286-350`). This is the most granular: the entire entity tree + event record is serialized on each event.
- On resume, the entire `RuntimeState` is restored from JSON/SQLite (`state/runtime.py:391-497`), `_resuming` flag is set (`base_agent.py:441-449, crew.py:486`), event scope is rebuilt (`base_agent.py:467-490, crew.py:547-575`), and execution continues from the last completed event.

**Specific guards against crash:**
- `_finalize_lock` + `_finalize_called` ensure the agent's finalize step runs exactly once even under concurrent branches (`experimental/agent_executor.py:2264-2269`).
- `_execution_lock` + `_is_executing` raise `RuntimeError` on re-entrant invoke (`experimental/agent_executor.py:2736-2741`).
- `flush(timeout=30.0)` blocks until pending event handlers complete (`events/event_bus.py:732-767`), called automatically at process shutdown via `atexit.register(crewai_event_bus.shutdown)` (`events/event_bus.py:952`).

### 5. Are tool calls their own atomic units?

**Yes at the event layer, NO at the persistence/cache layer.**

- **Event-layer atomicity**: Every call emits `ToolUsageStartedEvent` with `started_at` (`events/types/tool_usage_events.py:56-59`), then either `ToolUsageFinishedEvent` with `started_at`/`finished_at` or `ToolUsageErrorEvent`. `_execute_single_native_tool_call` (`crew_agent_executor.py:868-1071`) is the boundary: hooks run before (`crew_agent_executor.py:955-976`), tool function executes (`crew_agent_executor.py:989`), hooks run after (`crew_agent_executor.py:1037-1048`), `Finished` event emitted (`crew_agent_executor.py:1051-1063`).
- **Cache atomicity gap**: The cache write at `crew_agent_executor.py:1002-1005` is NOT transactional with the tool execution at `crew_agent_executor.py:989`. If the tool succeeds but the process crashes before the cache write, the result is lost AND the tool will re-execute on retry. Conversely, on cache HIT at `crew_agent_executor.py:929-936`, no `ToolUsageStartedEvent` for the underlying call — only `from_cache=True` on `ToolUsageFinishedEvent` (`crew_agent_executor.py:1067`).
- **Parallel atomicity**: For multi-tool native calls, `ThreadPoolExecutor` (`crew_agent_executor.py:746`) runs calls concurrently. The assistant message with all `tool_calls` is appended FIRST (`crew_agent_executor.py:735-740`), then individual tool results are appended in completion order. A crash mid-batch leaves the assistant message present with `tool_calls` but only some `tool` results — this is an explicit "parallel incomplete" state that the agent loop cannot fully recover from.
- **Hook atomicity**: `before_tool_call_hooks` can block the call by returning `False` (`crew_agent_executor.py:967-969`), in which case `result = "Tool execution blocked by hook"` is returned and `ToolUsageFinishedEvent` still fires with the synthetic result (`crew_agent_executor.py:977-979`). Hooks are best-effort — exceptions are logged and ignored (`crew_agent_executor.py:970-975`).
- **Plan-step tool validation**: `StepExecutor._validate_expected_tool_usage` raises `ValueError` if `todo.tool_to_use` was specified but the agent did not call it (`agents/step_executor.py:504-526`).

## Architectural Decisions

- **Two parallel executors**: `CrewAgentExecutor` (legacy ReAct, `agents/crew_agent_executor.py:98`) carries an explicit `DeprecationWarning` at `crew_agent_executor.py:145-151` ("Agents inside Crews now use AgentExecutor (crewai.experimental.AgentExecutor) by default"). The new `AgentExecutor` (`experimental/agent_executor.py:164`) inherits from `Flow[AgentExecutorState]` and uses Plan-and-Execute with explicit `TodoList` state. This is a deliberate migration but creates two competing atomicity contracts simultaneously.
- **Event-driven architecture**: `crewai_event_bus: Final[CrewAIEventsBus]` singleton (`events/event_bus.py:952`) with `on()`/`emit()`/`replay()` methods (`events/event_bus.py:244, 570, 671`). All execution observability is event-sourced — events are nodes in a directed graph (`state/event_record.py`) with `parent`, `trigger`, `previous`, `started` edge types (`state/event_record.py:52-61`).
- **ContextVar-based event correlation**: `_emission_counter`, `_last_emitted`, `_runtime_state_var`, `_registered_entity_ids_var`, `_runtime_scope_depth` (`events/event_bus.py:83-91`, `events/base_events.py:13-30`) let emit order be reconstructed across threads and async contexts. `set_emission_counter(max_seq)` (`base_agent.py:488-490, crew.py:574-575`) lets restores skip counter gaps.
- **Frozen-dataclass step isolation**: `StepExecutionContext` and `StepResult` are both `@dataclass(frozen=True)` (`utilities/step_execution_context.py:13, 44`). The comment at `utilities/step_execution_context.py:1-6` and `utilities/step_execution_context.py:21-26` makes the design intent explicit: no LLM history, no execution traces — only final results.
- **Layered checkpoints**: `CheckpointConfig` defaults to `["task_completed"]` (`state/checkpoint_config.py:171-175`) but accepts `["*"]` for every-event. Provider is pluggable: `JsonProvider` or `SqliteProvider` discriminated via `provider_type` (`state/checkpoint_config.py:176-182`). The `RuntimeState` snapshot includes `_event_record` plus `_checkpoint_id`/`_parent_id`/`_branch` for lineage (`state/runtime.py:181-198`).
- **Replay support**: `crewai_event_bus.replay(source, event)` (`events/event_bus.py:671-730`) re-dispatches a stored event without re-recording it. Listeners can check `is_replaying()` (`events/event_bus.py:72-80`) to skip side effects. `CrewReplayRunner` exists as a separate runner (referenced in `crew.py`).
- **Async without async**: `kickoff_async` wraps the synchronous `kickoff` in `asyncio.to_thread` (`crew.py:1143-1162`). True async uses `akickoff` (`crew.py:1190-1279`) which delegates to `_arun_sequential_process` → `_aexecute_tasks` → `task.aexecute_sync` → `agent.aexecute_task` (`crew.py:1308-1322, task.py:627-760, agents/agent_builder/base_agent.py:667-673`).
- **Guardrail as a first-class retry loop**: `Task.guardrail_max_retries: int = 3` (`task.py:273-275`) with `_guardrail_retry_counts: dict[int, int]` for per-list-element retry tracking (`task.py:291-293`). `LLMGuardrail` is the LLM-as-judge implementation (`tasks/llm_guardrail.py`).
- **Tool hook system**: Before/after hooks run inside the agent loop (`crew_agent_executor.py:955-1048`) and inside `execute_tool_and_check_finality` (`utilities/tool_utils.py:104-141`), giving two enforcement points.

## Notable Patterns

- **Event-sourced execution**: Every meaningful action emits an event (`crewai_event_bus.emit(...)`). Atomicity markers come from event pairs: started+finished, started+error, started+failed. The event bus records everything in `RuntimeState._event_record`, and `BaseStatePersistence.record_run`/`snapshot_node` patterns are not used (those are Flow-level via `_completed_methods`).
- **Frozen-dataclass boundaries**: `StepExecutionContext`, `StepResult` (`utilities/step_execution_context.py`), `PlanStep`, `TodoItem`, `StepObservation` (`utilities/planning_types.py`), `AgentAction`, `AgentFinish` (`agents/parser.py:26-43`). Atomic units are immutable.
- **Status state machine on entities**: `TodoList`/`TodoItem` (`utilities/planning_types.py:90-145`) implements `pending → running → completed/failed` with `mark_*` methods. The `AgentExecutor` only marks a todo complete after PlannerObserver observation succeeds (`experimental/agent_executor.py:1302-1305`).
- **Branch-based checkpoint lineage**: `RuntimeState._branch: str = "main"` and `_checkpoint_id`/`_parent_id` form a chain (`state/runtime.py:181-183`). `fork()` emits `CheckpointForkStartedEvent`/`CheckpointForkCompletedEvent` (`state/runtime.py:352-389`). `CheckpointPrunedEvent` fires when old checkpoints exceed `max_checkpoints` (`state/checkpoint_listener.py:191-216`).
- **Plan-and-Act isolation**: Per Plan-and-Act (arxiv 2503.09572, cited at `agents/step_executor.py:11, 70`), each executor receives ONE step description and returns ONE result. The comment "There is no inner loop. Recovery from failure (retry, replan) is the responsibility of PlannerObserver and AgentExecutor" (`agents/step_executor.py:8-10`) makes the single-purpose design explicit.
- **Asymmetric tool-call execution**: Native calls have parallel-batch support (`crew_agent_executor.py:742-776`), cache-through (`crew_agent_executor.py:929-1005`), and result-as-answer early termination (`crew_agent_executor.py:1097-1106`). Text-parsed calls are sequential via `execute_tool_and_check_finality` (`crew_agent_executor.py:415-426`).
- **Hash-derived keys**: `Task.key` (`task.py:583-588`), `Agent.key` (`base_agent.py:649-655`), `Crew.key` (`crew.py:871-875`) are MD5s of stable inputs — used for caching and dedup, not atomicity, but they're stable identifiers that survive serialization.

## Tradeoffs

- **Two executors → inconsistent atomicity**: `CrewAgentExecutor` (ReAct, legacy) vs `AgentExecutor` (Plan-and-Execute, new). ReAct uses shared `messages` list; P&E uses per-step isolation. A crew using both will have different crash-recovery semantics per task.
- **Event-level vs cache-level atomicity**: Tool calls emit clean start/finish events but the cache write is NOT transactional with execution. A crash between execute and cache-write causes re-execution on retry — possibly harmful for non-idempotent tools.
- **Iteration counter always advances, but messages may not**: `finally: self.iterations += 1` (`crew_agent_executor.py:459-460`) is atomic, but `self.messages.append(...)` is not. The system says "how many iterations ran" reliably but not "what did each iteration's messages look like" after a crash.
- **Default checkpointing is too coarse**: `on_events=["task_completed"]` (`state/checkpoint_config.py:171-175`) means a crash mid-task re-executes the entire task. To get per-iteration recovery, users must opt into `["*"]` which writes a full snapshot per event.
- **`messages` is shared mutable state**: Both `CrewAgentExecutor.messages: list[LLMMessage]` (`agents/agent_builder/base_agent_executor.py:28`) and `AgentExecutorState.messages: list[LLMMessage]` (`experimental/agent_executor.py:134`) are lists that can be appended to during execution. The P&E executor mitigates by resetting on next invoke (`experimental/agent_executor.py:2748`); the legacy executor resets on next invoke unless `_resuming` (`crew_agent_executor.py:217-223`).
- **RPM is a hard block, not a retry**: `enforce_rpm_limit` blocks the calling thread until the next minute (`crew_agent_executor.py:354, rpm_controller.py:53-57`). No retry semantics — the call just stalls.
- **Plan replan is lossy for in-flight steps**: `replace_pending_todos(new_items)` replaces `pending` todos but preserves `running` (`utilities/planning_types.py:194`). A running todo may complete with stale assumptions.
- **Trace batch has hard 30s timeout**: `flush(timeout=30.0)` (`events/event_bus.py:732-767`) — events that take longer are lost (`events/listeners/tracing/trace_batch_manager.py:301-318`).
- **LiteAgent is deprecated but still primary**: `@deprecated("LiteAgent is deprecated and will be removed in v2.0.0.")` (`lite_agent.py:183-186`), but the comment "Use Agent().kickoff(messages) instead" suggests the migration is incomplete.

## Failure Modes / Edge Cases

- **Mid-step crash, no checkpoint**: Agent re-runs from scratch with empty `messages`. The `kickoff` return value is lost; no auto-resume.
- **Tool cache race**: `from_cache=True` is computed BEFORE execution but set on `Finished` event AFTER (`crew_agent_executor.py:933, 1067`). A crash between read and execute would still produce a `Started` event with no `Finished` — event log shows the inconsistency.
- **Parallel native tool crash**: `ThreadPoolExecutor` with `as_completed` (`crew_agent_executor.py:765-776`). A `BaseException` in one tool does NOT cancel siblings in this code path — only the streaming-mode `CallToolsNode` does so via `CancelAndDrain` (Pydantic-AI has this; CrewAI's parallel path does not).
- **LiteAgent guardrail infinite loop**: `return self._execute_core(agent_info=agent_info)` recursive call on guardrail failure (`lite_agent.py:726`). With `guardrail_max_retries=0` and a permanently-failing guardrail, it can stack-overflow on the same call site — but `guardrail_max_retries>=1` (default 3) catches it (`lite_agent.py:706`).
- **Async task timeout**: `asyncio.create_task(task.aexecute_sync(...))` is awaited with no timeout (`crew.py:1360-1388`). A hanging task blocks the entire crew indefinitely.
- **Tool message `content=None` edge case**: When `assistant_message.content is None` (`crew_agent_executor.py:851`), the message has no text but has `tool_calls`. Some LLM providers (e.g., certain Gemini variants) reject this.
- **Re-entrant invoke**: AgentExecutor raises `RuntimeError("Executor is already running")` (`experimental/agent_executor.py:2738-2741`) but `CrewAgentExecutor` has no equivalent guard — calling `executor.invoke()` from inside a callback can cause stack overflow.
- **Output parser retry is unbounded in practice**: `handle_output_parser_exception` returns the existing iteration result if not yet `log_error_after=3` but does NOT count retries separately — it's a soft retry that may loop forever if the LLM consistently mis-formats.
- **Cache hit doesn't emit `Started` event with real `started_at`**: When `from_cache=True`, the `Finished` event carries a `started_at` value but no `Started` event was emitted for that call (`crew_agent_executor.py:933` reads cache before emitting at line 940) — observability gap.
- **Multi-tool native execution race**: `ThreadPoolExecutor` appends tool results in completion order (`crew_agent_executor.py:769-776`), but the LLM may expect results in tool-call order. Mixing this could produce out-of-order messages.
- **Checkpoint during execution**: `_do_checkpoint` is called from a sync handler running in `ThreadPoolExecutor` (`state/checkpoint_listener.py:251-253`) — safe for blocking I/O but not transactional. A crash mid-write leaves a partial checkpoint file.

## Future Considerations

- **Executor consolidation**: `CrewAgentExecutor`'s `DeprecationWarning` (`crew_agent_executor.py:145-151`) signals the legacy executor will be removed. The Plan-and-Execute path with `AgentExecutor` + `StepExecutor` is the future.
- **Checkpoint API stability**: `CheckpointConfig`, `RuntimeState.checkpoint`, `from_checkpoint`, `fork` are all still evolving (`state/checkpoint_config.py`, `state/runtime.py`). Migration logic exists for v1.14.6+ (`state/runtime.py:115-119`) — older checkpoints may need migration.
- **Flow method execution as new atomic unit**: `MethodExecutionStartedEvent`/`MethodExecutionFinishedEvent` (`events/types/flow_events.py:30-85`) and `_completed_methods: set[FlowMethodName]` (`flow/runtime/__init__.py:997`) suggest Flow is becoming a first-class atomic unit, separate from agent iterations.
- **Durable execution engines**: `crewai.experimental.durable_exec` exists (referenced in tests `tests/agents/test_agent_executor.py`) — pattern likely extends to Temporal/Prefect integration similar to Pydantic-AI.
- **TodoItem as a richer record**: Currently `result: str | None` (`utilities/planning_types.py:40-42`). A richer `StepRecord` with `messages`, `tool_calls`, `started_at`, `finished_at` could subsume both event and result tracking.
- **Event-driven UI consolidation**: Currently `ConsoleFormatter` + `step_callback` + `task_callback` + `kickoff_callbacks` are all parallel mechanisms. A unified event subscription API could consolidate.

## Questions / Gaps

- **Per-iteration checkpoint without `["*"]`**: No way to checkpoint every agent iteration (only every task completion, or every event). Per-iteration checkpoint would require custom event handling.
- **What is the canonical "step" for the legacy `CrewAgentExecutor`?** The vocabulary uses "iteration" via `self.iterations` but the user-facing term is "step" via `step_callback`. The legacy path has no explicit `Step` class — the iteration IS the step, but it's not first-class.
- **Is `messages` part of the durable schema?** `AgentExecutorState.messages` is reset on invoke (`experimental/agent_executor.py:2748`), but on `_resuming=True` it's preserved via `agent_executor.messages` (`crew_agent_executor.py:481-484`). The checkpoint dump captures it (`state/runtime.py:196`), but the P&E version's reset-on-invoke means a fresh invoke loses it unless it was just restored.
- **What happens to `pending_tool_calls` on a mid-batch crash?** The list is `state.pending_tool_calls: list[Any]` (`experimental/agent_executor.py:140`). On `invoke` reset it's cleared to `[]` (`experimental/agent_executor.py:2753`). But during execution, between `_handle_native_tool_calls` writing to it and the next iteration consuming it, a crash leaves a half-populated list. No retry resumes from the partial list — the next iteration just re-calls the LLM.
- **How is the LiteAgent → Agent migration handled?** `@deprecated` warning exists but LiteAgent still ships with full functionality. No explicit removal date — the migration path is "use Agent().kickoff(messages)" but the two have different message-handling semantics (LiteAgent uses `self._messages: list[LLMMessage]` directly, Agent uses `AgentExecutorState.messages`).
- **Does `AgentExecutorState.execution_log` survive checkpoint?** Yes, it's serialized via `state/runtime.py:196` (`event_record` not `execution_log`). The `execution_log` is in-memory audit only — not in the snapshot.

---

Generated by `01.03-step-turn-and-task-atomicity.md` against `crewai`.