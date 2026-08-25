# Source Analysis: agent-framework

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (asyncio, Pydantic, OpenTelemetry, file/redis/durabletask storage, MCP, A2A, AG-UI, OpenAI/Anthropic/Foundry/Gemini/Bedrock/claude/ollama + dotnet parity) |
| Analyzed | 2026-07-02 |

## Summary

Microsoft Agent Framework exposes at least **four nesting levels of "atomic unit," and they are *not* the same shape**:

1. **Function-calling iteration** at the chat client (`FunctionInvocationLayer.get_response`): one LLM roundtrip + zero-or-more parallel tool executions is one iteration (`_tools.py:2556-2662`). Bounded by `max_iterations` (LLM roundtrips; `DEFAULT_MAX_ITERATIONS=40` at `_tools.py:90`) and `max_function_calls` (best-effort cumulative, not exact, `_tools.py:1351-1363`), plus `max_consecutive_errors_per_request` and `terminate_on_unknown_calls` (`_tools.py:1364-1367`).
2. **Per-tool invocation** at the tool layer (`FunctionTool.__call__` + `_invoke_function`): one Python call to the wrapped function is its own atomic unit, gated by `max_invocations` (lifetime), `max_invocation_exceptions` (lifetime), `approval_mode`, and `FunctionMiddleware` (`_tools.py:520-553`, `_tools.py:1340-1421`). Parallel calls in the same iteration execute via `asyncio.gather` (`_tools.py:1821-1823`).
3. **Agent loop iteration** (`AgentLoopMiddleware`): one full `await call_next()` invocation of the agent is one iteration (`_harness/_loop.py:439-516`), bounded by `max_iterations=DEFAULT_MAX_ITERATIONS=10` (`_harness/_loop.py:117`) or `5` in `.with_judge` (`_harness/_loop.py:122`).
4. **Workflow superstep** (`Runner.run_until_convergence`): a Pregel-style barrier where every queued executor runs once, edges drain, and `State.commit()` is called (`_workflows/_runner.py:99-156`); bounded by `max_iterations=DEFAULT_MAX_ITERATIONS=100` (`_workflows/_const.py:4`). The *executor* unit (`_workflows/_executor.py:219-294`) emits `executor_invoked` / `executor_completed` / `executor_failed` events per handler invocation.

Atomicity is **mostly consistent within a layer but not unified across them**:

- Persistence uses **committed superstep state** (`_workflows/_state.py:6-127`); checkpoint snapshots `state` *plus* in-flight `messages` *plus* `pending_request_info_events` *plus* `iteration_count` (`_workflows/_checkpoint.py:30-103`).
- The agent-run API does **not** checkpoint partial mid-iteration state — `_call_chat_client` invokes the chat client once per pipeline pass (`_agents.py:962-1021`); history persistence is opt-in via `require_per_service_call_history_persistence` after every model call (`_agents.py:1221-1240`).
- Tracing (`create_processing_span`) opens an executor span per `_executor.execute` call (`_workflows/_executor.py:245-252`), not per message batch or superstep; `executor_invoked` / `executor_completed` events mark the executor boundary, `superstep_started` / `superstep_completed` mark the superstep boundary (`_workflows/_events.py:118-122`).
- Tool calls **are** their own atomic unit: each call runs through `_auto_invoke_function` with its own `FunctionInvocationContext` and middleware pipeline (`_tools.py:1424-1645`), and failures are converted to `Content.from_function_result(..., exception=...)` rather than raised — so a partial completion is captured as a structured function_result, not a half-state (`_tools.py:1519-1528`, `_tools.py:1556-1569`, `_tools.py:1632-1645`).
- A mid-tool crash leaves the model's `function_call` message persisted but the matching `function_result` may be missing or be an error; the loop recovers via the in-iteration error counter and bounded max_iterations, not via transaction rollback.
- A mid-executor crash surfaces an `executor_failed` event AND re-raises the exception (`_workflows/_executor.py:280-285`); the workflow then yields `WorkflowEvent.failed` and `WorkflowRunState.FAILED` (`_workflows/_workflow.py:606-628`); the next `_run` call can resume from any prior checkpoint via `restore_from_checkpoint` (`_workflows/_runner.py:240-300`).

The unit vocabulary explicitly distinguishes "turn" (one model roundtrip → `AgentResponse` or streamed `AgentResponseUpdate`), "step" (one executor invocation per superstep), "iteration" (superstep count), "task" (not used as a vocabulary item; workflows use `request_info` for human-in-the-loop instead), and "loop iteration" (one `call_next()` in `AgentLoopMiddleware`). Human-in-the-loop requests are persisted on the checkpoint as `pending_request_info_events` (`_workflows/_checkpoint.py:60`) so a partial completion (run paused waiting for human) can be resumed.

**Rating: 7/10** — Clear model with explicit interfaces in each layer, well-tested (`tests/workflow/test_workflow_states.py:31-118`), explicit crash semantics for executors (emits `executor_failed` then bubbles) and partial completion for tools (exception → structured error result), and durable execution support via `_harness/_loop.py`, `PerServiceCallHistoryPersistingMiddleware`, and checkpoint storage. Not higher because (a) the four "atomic units" coexist without a unifying abstraction that says exactly what completed and what did not at *one* model boundary — you must consult four different counters (`attempt_idx` in the chat loop, `iteration` in the agent loop, `iteration_count` in checkpoints, `iteration` in middleware-level events), (b) the chat-client `max_function_calls` is *best-effort* and checked after each parallel batch — a single model turn can overshoot it (`_tools.py:2636-2644`); (c) workflow `executor_failed` re-raises the exception, so any caller that does not catch it loses the partial state; (d) agent-loop crash semantics are implicit — `MiddlewareTermination` is the documented control-flow exit (`_loop.py:547-552`) but a raw exception inside `call_next` is not documented at the boundary and is not idempotent on retry.

## Rating

**7/10** — Strong per-layer atomicity (`Executor.execute` ← `superstep` ← `WorkflowCheckpoint`; `FunctionTool.__call__` ← iteration ← `FunctionInvocationLayer`; `AgentLoopMiddleware` iteration ← one agent `run()`); explicit, testable, event-emitted boundaries. Gaps: no unifying atomic-unit abstraction across the four layers, `max_function_calls` is best-effort, no transactional semantics on agent-loop crash (relies on history providers), no replay/dedup for a re-issued model turn after retry.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Function invocation iteration counter (LLM roundtrips) | `for attempt_idx in range(attempt_start, max_iterations if loop_enabled else 0)` in non-streaming + streaming `_get_response` | `_tools.py:2567`, `_tools.py:2717` |
| Default max_iterations for tool loop | `DEFAULT_MAX_ITERATIONS: Final[int] = 40` | `_tools.py:90` |
| Function-invocation atomic configuration | `FunctionInvocationConfiguration` TypedDict with `enabled`, `max_iterations`, `max_function_calls`, `max_consecutive_errors_per_request`, `terminate_on_unknown_calls`, `additional_tools`, `include_detailed_errors` | `_tools.py:1344-1398` |
| Per-tool lifetime counters | `max_invocations`, `max_invocation_exceptions`, `invocation_count` tracked on the `FunctionTool` instance | `_tools.py:308-345`, `_tools.py:399-406` |
| Tool execution as its own atomic unit | `_auto_invoke_function` constructs a `FunctionInvocationContext`, runs middleware pipeline, wraps result as `Content.from_function_result` | `_tools.py:1424-1645` |
| Per-tool budget enforcement | `if self.max_invocations is not None and self.invocation_count >= self.max_invocations: raise ToolException(...)` | `_tools.py:520-531` |
| Parallel tool calls per iteration | `execution_results = await asyncio.gather(*[invoke_with_termination_handling(...)])` | `_tools.py:1821-1823` |
| Tool exception → structured error result | Catches Exception → `Content.from_function_result(call_id=..., exception=str(exc))` | `_tools.py:1632-1645`, `_tools.py:1556-1569` |
| Best-effort (not exact) `max_function_calls` cap | `max_function_calls is checked after each batch of parallel calls completes, so the current batch always runs to completion even if it overshoots` | `_tools.py:2636-2644`, `_tools.py:2808-2816` |
| Bounded loop on max consecutive errors | `_process_function_requests` returns `action = "stop"` after `errors_in_a_row >= max_errors` | `_tools.py:1790-1799`, `_tools.py:2374-2377` |
| `max_iterations` exhaustion path | After loop, `mutable_options["tool_choice"] = "none"`; final tool-less call forced | `_tools.py:2668-2685`, `_tools.py:2839-2855` |
| Final tool-less safety call (avoid orphan function_call items) | Comment: `produces a plain text answer instead of leaving orphaned function_call items without matching results` | `_tools.py:2664-2673` |
| Function-invocation layer composition (where tool loop lives) | `class FunctionInvocationLayer(Generic[OptionsCoT])` wraps `BaseChatClient.get_response` | `_tools.py:2388-2408` |
| Per-call FunctionInvocationContext scope | `direct_context = FunctionInvocationContext(...)` even without middleware, when tool declares a `ctx` param | `_tools.py:1530-1547` |
| AgentMiddleware (Agent-level loop iteration unit) | `AgentLoopMiddleware` extends `AgentMiddleware`, calls `await call_next()` repeatedly | `_harness/_loop.py:212`, `_harness/_loop.py:454-455` |
| AgentLoopMiddleware default + judge max_iterations | `DEFAULT_MAX_ITERATIONS = 10`, `DEFAULT_JUDGE_MAX_ITERATIONS = 5` | `_harness/_loop.py:117`, `_harness/_loop.py:122` |
| AgentLoopMiddleware iteration counter | `iteration = 0; iteration += 1` inside `_process_non_streaming` | `_harness/_loop.py:446`, `_harness/_loop.py:456` |
| Agent-loop stop precedence | `_evaluate_stop` checks `max_iterations` first (short-circuits before `should_continue`) | `_harness/_loop.py:653-664` |
| Feedback log persisted at iteration boundary | `record: lambda *, iteration, **kwargs: f"note-{iteration}"` → `progress.append(entry)` on iteration boundary | `_harness/_loop.py:637-651` |
| Session snapshot (loop fresh-context mode) | `snapshot = context.session.to_dict() if self.fresh_context and context.session is not None`; restored between iterations | `_harness/_loop.py:417-421`, `_harness/_loop.py:507-510`, `_harness/_loop.py:423-437` |
| Streaming iteration (middleware case) tracks final response per iteration | `holder: dict[str, AgentResponse | None] = {"final": None}` | `_harness/_loop.py:527` |
| Stream + middleware-termination = clean stop, keeps prior final | `except MiddlewareTermination: return` after `await inner.get_final_response()` | `_harness/_loop.py:546-552` |
| Judge predicate = one additional atomic step | `async def _judge(*, last_result, original_messages, **kwargs)` runs `await judge_client.get_response(...)` between iterations | `_harness/_loop.py:166-208` |
| Workflow superstep model (Pregel-like) | `Runner.run_until_convergence`: `while self._iteration < self._max_iterations: ... yield WorkflowEvent.superstep_started(...) ... self._state.commit() ... yield WorkflowEvent.superstep_completed(...)` | `_workflows/_runner.py:99-156` |
| Workflow default max_iterations | `DEFAULT_MAX_ITERATIONS = 100` for workflows | `_workflows/_const.py:4` |
| Executor as the workflow atomic unit | `class Executor(RequestInfoMixin, DictConvertible)`; `execute(message, source_executor_ids, state, runner_context, ...)` opens a processing span, dispatches to handler, emits invoked/completed/failed events | `_workflows/_executor.py:30`, `_workflows/_executor.py:219-294` |
| Per-executor traced and event-bounded | `with create_processing_span(self.id, ...)`, `await context.add_event(invoke_event)`, `await context.add_event(completed_event)` | `_workflows/_executor.py:245-294` |
| Executor mid-handler failure path | `try: await handler(...) except Exception: failure_event = WorkflowEvent.executor_failed(...); raise` | `_workflows/_executor.py:278-285` |
| State superstep caching | `State.set()` writes to `_pending`; `get()` checks pending then committed; `commit()` flushes pending to committed (called by Runner at superstep boundary); `discard()` drops pending | `_workflows/_state.py:6-127` |
| State export excludes pending | `export_state` returns only `dict(self._committed)` | `_workflows/_state.py:106-111` |
| Checkpoint captures superstep boundary state | `WorkflowCheckpoint(messages=dict(self._messages), state=state.export_state(), pending_request_info_events=..., iteration_count=...)` | `_workflows/_runner_context.py:383-392` |
| Checkpoint stored at end of every superstep | `_create_checkpoint_if_enabled` invoked from `run_until_convergence` after each superstep; first checkpoint after start-executor | `_workflows/_runner.py:96-97`, `_workflows/_runner.py:143-144` |
| Checkpoint fields = boundary snapshot | `WorkflowCheckpoint`: `workflow_name`, `graph_signature_hash`, `previous_checkpoint_id`, `messages`, `state` (committed only), `pending_request_info_events`, `iteration_count`, `metadata`, `version` | `_workflows/_checkpoint.py:30-89` |
| Checkpoint restore on resume | `restore_from_checkpoint`: clears state, imports checkpoint state, restores executor states via `on_checkpoint_restore`, applies checkpoint to context, marks resumed | `_workflows/_runner.py:240-300` |
| Executor state serialization hooks | `async def on_checkpoint_save(self) -> dict[str, Any]` and `on_checkpoint_restore(self, state)` overridable on every `Executor` | `_workflows/_executor.py:493-517` |
| Request-info events persisted in checkpoint | `pending_request_info_events` field; `apply_checkpoint` re-enqueues them as events | `_workflows/_runner_context.py:60-62`, `_workflows/_runner_context.py:414-426` |
| Request-info response routing | `send_request_info_response(request_id, response)` builds `WorkflowMessage(type=MessageType.RESPONSE, original_request_info_event=event)` | `_workflows/_runner_context.py:457-486` |
| Response message routing via original_request_info_event | `can_handle` / `_find_handler` check `message.original_request_info_event` to dispatch to a `response_handler` | `_workflows/_executor.py:355-485`, `_workflows/_request_info_mixin.py:1-115` |
| Workflow events (UI / trace vocabulary) | `WorkflowEventType` Literal: `started`, `status`, `failed`, `output`, `intermediate`, `data` (deprecated), `request_info`, `warning`, `error`, `superstep_started`, `superstep_completed`, `executor_invoked`, `executor_completed`, `executor_failed`, `executor_bypassed`, `group_chat`, `handoff_sent`, `magentic_orchestrator` | `_workflows/_events.py:104-130` |
| Iteration event factory | `WorkflowEvent.superstep_started(iteration=self._iteration + 1)` and `WorkflowEvent.superstep_completed(iteration=self._iteration)` | `_workflows/_runner.py:101`, `_workflows/_runner.py:146` |
| Run-state enum | `WorkflowRunState = STARTED / IN_PROGRESS / IN_PROGRESS_PENDING_REQUESTS / IDLE / IDLE_WITH_PENDING_REQUESTS / FAILED / CANCELLED` | `_workflows/_events.py:58-68` |
| Agent-level run identifier (turn boundary) | `response_id`, `created_at`, `agent_id`, `finish_reason`, `usage_details`, `continuation_token` on `AgentResponse` | `_types.py` (consumed at `_agents.py:1039-1049`) |
| Streaming "turn" unit = sequence of `AgentResponseUpdate` finalizing into `AgentResponse` | `ResponseStream[AgentResponseUpdate, AgentResponse]` everywhere; `agent.run(..., stream=True)` returns a stream | `_agents.py:951-977`, `_middleware.py:1254-1345` |
| Agent pipeline middleware layer | `AgentMiddlewareLayer.run` wraps `BaseAgent.run` with `AgentMiddlewarePipeline`; one pipeline pass = one "agent turn" by middleware view | `_middleware.py:1254-1495` |
| `MiddlewareTermination` = explicit early-exit control flow | Exception class with optional `result` attribute; result captured even on termination | `_middleware.py:72-80`, `_tools.py:1612-1629` |
| Per-service-call history persistence opt-in | `Agent(..., require_per_service_call_history_persistence=True)` installs `PerServiceCallHistoryPersistingMiddleware`; persists providers after every model call | `_agents.py:678-705`, `_agents.py:1221-1340`, `_sessions.py:559-705` |
| Per-service-call history middleware concept | `PerServiceCallHistoryPersistingMiddleware._invoke_agent_loop` / `_persist_service_call_response` — per-call persistence boundary | `_sessions.py:559-705` |
| Auto-added InMemoryHistoryProvider when no other provider | `if session is not None and not self.context_providers and not session.service_session_id ...: self.context_providers.append(InMemoryHistoryProvider())` | `_agents.py:1202-1209` |
| `AgentSession` is the durable carrier across turns | `session_id`, `service_session_id`, `state` dict; `to_dict`/`from_dict` round-trip; the harness uses `to_dict()` to snapshot loop state | `_sessions.py:746-811` |
| `InMemoryHistoryProvider` stores one message per call | `class InMemoryHistoryProvider(HistoryProvider)` storing messages in `state["messages"]` | `_sessions.py:814-859` |
| ChatClient `conversation_id` as the per-turn durability handle | `_update_continuation_state` injects `conversation_id` into mutable options; subsequent `prepped_messages.clear()` and use server-side conversation | `_tools.py:1859-1873`, `_tools.py:2654-2661` |
| Tool call or model call after `max_iterations` becomes a forced tool-less call | `mutable_options["tool_choice"] = "none"` to avoid orphan `function_call` items | `_tools.py:2668-2696`, `_tools.py:2839-2855` |
| Approval-flow atomicity: tools run partially even when approval is mixed | Comment: `Mixed tool-call batches use a default .NET-style bypass ... treating as already approved, hidden, stored in session state` | `python/packages/core/AGENTS.md` (Tool Approval Harness) |
| Declarative-only tools suspended via `request_info` pause | `declaration_only_flag` path returns user_input_request to AgentExecutor which then pauses the workflow | `_tools.py:1716-1765` |
| `UserInputRequiredException` is a distinct control-flow exit | Re-raised explicitly; caught at loop entry to convert into user-input propagation | `_tools.py:1554-1555`, `_tools.py:1630-1631` |
| `tool_choice="required"` resets after one iteration to avoid infinite loops | Tool-of-force avoids unbounded tool iterations | `_tools.py:2647-2652`, `_tools.py:2819-2823` |
| Middleware pipeline ordering preserves call_next-as-run-boundary | `AgentMiddleware.process(context, call_next)` is the documented invocation contract; termination via `MiddlewareTermination` | `_middleware.py:498-521` |
| Function-invocation middleware pipeline independently | `FunctionMiddlewarePipeline.execute(context, final_handler)` wraps each tool call; one tool call = one pipeline execution | `_middleware.py:930-970` |
| Tests for executor-failure partial completion | `test_executor_failed_and_workflow_failed_events_streaming`, `test_executor_failed_event_from_second_executor_in_chain`, `test_executor_failed_event_emitted_on_direct_execute` | `tests/workflow/test_workflow_states.py:31-118` |
| Tests for tool-call-and-agent pause interaction | `test_step_hitl_does_not_emit_executor_failed` (request_info inside a step does not look like failure), `test_step_failure_emits_executor_failed` | `tests/workflow/test_functional_workflow.py:320-342`, `tests/workflow/test_functional_workflow.py:1362-1380` |
| Tests for loop boundary stop on max_iterations | `test_loop_stops_at_max_iterations`, `test_should_continue_controls_iterations_and_receives_kwargs`, `test_streaming_middleware_termination_stops_cleanly` | `tests/core/test_harness_loop.py:212-240`, `tests/core/test_harness_loop.py:1111-1138` |
| Test for retention of final response across a stop | `test_streaming_middleware_termination_stops_cleanly` asserts `final.text == "only"` after MiddlewareTermination on second iteration | `tests/core/test_harness_loop.py:1131-1137` |
| Checkpoint encoding for arbitrary types | `FileCheckpointStorage` uses pickle-in-JSON with `allowed_checkpoint_types` allow-list; basic types + framework types auto-allowed | `_workflows/_checkpoint.py:239-285` |
| Atomic write on disk for checkpoints | `_write_atomic() -> tmp + os.replace` | `_workflows/_checkpoint.py:317-323` |
| `executor_bypassed` event for replay skipping | `executor_bypassed` event when a cache hit skips an executor | `_workflows/_events.py:340-342`, `_workflows/_runner.py:341` (parity with replay) |
| Tool-call output classification | `optional OutputDesignation.outputs / .intermediates` allow-list gate emits `type='output'` vs `type='intermediate'` | `_workflows/_runner_context.py:269-275`, `_workflows/_workflow.py:170-204` |

## Answers to Dimension Questions

1. **What is the atomic unit of execution?**

   There are four named units, each scoped to a different layer; the framework refuses to collapse them:

   - At the **chat-client function-calling loop**: one LLM roundtrip plus the parallel batch of tool calls the model requested (`for attempt_idx in range(...)` iteration; `_tools.py:2567`, `_tools.py:2717`).
   - At the **tool layer**: one Python call to the wrapped function (`FunctionTool.__call__` / `tool.invoke`; `_tools.py:520-553`, `_tools.py:577+`).
   - At the **agent loop**: one full `await call_next()` invocation of the pipeline (`AgentLoopMiddleware` iteration; `_harness/_loop.py:454-455`).
   - At the **workflow layer**: one executor handler invocation per superstep (`Executor.execute` call; `_workflows/_executor.py:219-294`), composed into a superstep barrier (`Runner.run_until_convergence`; `_workflows/_runner.py:99-156`).

   The vocabulary is precise at each layer (attempt, iteration, superstep) but **no single name covers them all**, which is a real usability weakness.

2. **Is the atomic unit the same for persistence, tracing, retry, and UI?**

   **No.** Persistence (within workflow): superstep-committed `state` + in-flight `messages` + `pending_request_info_events` (`_workflows/_checkpoint.py:30-89`). Persistence (within agent): per-service-call history (`_sessions.py:559-705`) and `AgentSession.state` (`_sessions.py:746-811`). Tracing (workflow): per-executor `processing_span` (`_executor.py:245-252`); tracing (chat loop): chat-client span per roundtrip (covered by `instrument_*` helpers in `observability.py`). Retry boundaries: `max_iterations` (chat loop), `max_function_calls` (best-effort), `FunctionTool.max_invocations` (lifetime), workflow `max_iterations` (supersteps), `AgentLoopMiddleware.max_iterations` (agent loop), `middleware_max_retries` not used. UI: streaming shows every `AgentResponseUpdate` / `ChatResponseUpdate`, so the user sees individual tool-call deltas; non-streaming shows `AgentResponse` aggregating every message in the agent turn — a different shape from any persistence or retry boundary.

3. **Can partially completed steps exist?**

   **Yes.** Three concrete forms:

   - **Tool raised mid-execution**: `_auto_invoke_function` catches Exception and returns `Content.from_function_result(call_id=..., result="Error: ...", exception=str(exc))` so the model's prior `function_call` Content has a matching `function_result` (`_tools.py:1632-1645`). The agent loop survives.
   - **Executor raised mid-handler**: `_executor.py:280-285` records a `WorkflowEvent.executor_failed` with structured details, then re-raises. The executor's `on_checkpoint_save` was NOT called, so any in-handler state mutations that bypassed `State.set()` are lost; any state written via `ctx.state.set(...)` lives in the pending buffer that is *not* in `state.export_state()` until commit (`_workflows/_state.py:106-111`).
   - **Mixed batch of tool calls**: a parallel batch can have some calls succeed and some fail, then `errors_in_a_row` is incremented and the run may continue, terminate, or fall through to the in-iteration error handler (`_tools.py:1790-1799`).

4. **What happens if a crash occurs mid-step?**

   - **Mid chat-client roundtrip**: the in-flight request dies; the agent-loop outer coroutine dies; `require_per_service_call_history_persistence` has not yet run, so the assistant's partial response is **not** persisted. A retry by the caller re-runs the whole `agent.run()`.
   - **Mid executor handler**: `executor_failed` event is yielded, exception re-raised at the workflow level (`_executor.py:280-285`), `WorkflowEvent.failed` yielded (`_workflow.py:606-628`). The previous superstep's `WorkflowCheckpoint` is intact on disk (created at end of each superstep, `_runner.py:143-144`); `restore_from_checkpoint` resumes from any prior committed superstep (`_runner.py:240-300`), losing everything in the failing superstep only.
   - **Mid tool call**: the tool's exception is caught and converted to a `Content.from_function_result(..., exception=...)` (`_tools.py:1632-1645`); the model is told the tool failed and can react; no crash surface at the framework level.
   - **Mid `AgentLoopMiddleware` iteration**: a synchronous `call_next()` failure bubbles up the middleware pipeline; `AgentMiddleware` does **not** define retry semantics; the run fails unless `MiddlewareTermination` was raised deliberately.

5. **Are tool calls their own atomic units?**

   **Yes.** Each call enters `_auto_invoke_function` (`_tools.py:1424-1645`), runs through `FunctionMiddlewarePipeline.execute` (or direct invocation when no middleware), and exits with a single `Content.from_function_result` (or `function_approval_request`, or `UserInputRequiredException`). The lifecycle is bounded by `max_invocations` (counter on the tool instance) and `max_invocation_exceptions`, configurable at tool construction (`_tools.py:308-345`). Parallel tool calls within one model turn run via `asyncio.gather` (`_tools.py:1821-1823`) — they are siblings, not nested — and `max_function_calls` is checked after the batch completes (`_tools.py:2636-2644`). The atomic unit visible to the model is the `function_call` / `function_result` Content pair; the atomic unit visible to the user is the streamed update; the atomic unit persisted is the matching message in the history provider.

## Architectural Decisions

- **Layered atomicity (no collapse).** The framework keeps four explicit units — chat-loop iteration, tool invocation, agent-loop iteration, executor invocation — because each layer has a different durability, retry, and concurrency contract. This is visible in `FunctionInvocationLayer`, `AgentMiddlewareLayer`, `Executor`, and the superstep runner. The cost is that callers must reason about which unit they care about; the benefit is that each layer can be retried or persisted independently.

- **Pregel-style superstep barrier with state staging.** `State` separates `_committed` from `_pending` writes; the runner calls `commit()` at the superstep boundary so all executors in a superstep see the same committed state at the next iteration (`_workflows/_state.py:30-111`; `_workflows/_runner.py:141`). Checkpoints serialize committed state only (`_workflows/_runner_context.py:383-392`). This is a deliberate consistency choice: it prevents intra-superstep races on state at the cost of discarding partial superstep progress if a crash occurs inside a superstep.

- **Best-effort `max_function_calls`, exact `max_iterations`.** The docstring is explicit: `max_iterations` caps model round-trips; `max_function_calls` is "best-effort ... checked *after* each batch of parallel tool calls completes" (`_tools.py:1355-1363`, `_tools.py:2636-2644`). This means the parallel batch is its own mini-atomic group, but the cumulative ceiling is not exact. The rationale is letting a parallel batch finish for partial-result analysis without premature truncation; the consequence is the counter can be exceeded.

- **Chat-history durability model.** Two layers co-exist: `AgentSession.state` + provider persistence (in `InMemoryHistoryProvider` / `FileHistoryProvider` / `RedisHistoryProvider` / etc., `_sessions.py:814+`) and the workflow's checkpoint (which captures state at superstep boundary, `_workflows/_runner.py:143-144`). `require_per_service_call_history_persistence` opts into per-call history writes; without it, history is written only at `after_run` provider boundaries (`_agents.py:1416-1417`).

- **Loop iterations are nested, not flattened.** `AgentLoopMiddleware` may wrap an agent whose chat client already loops internally for tool calling (`_harness/_loop.py:454-455`); the agent loop calls `call_next()` once per outer iteration, which itself may run an inner iteration loop. `iteration` and `attempt_idx` are independent counters. `should_continue`'s loop-kwargs parameter (`iteration`) is the outer loop's; nothing in the framework correlates `iteration` to `attempt_idx`.

- **Human-in-the-loop is a first-class atomic state.** Request-info events are persisted in the checkpoint as `pending_request_info_events` (`_workflows/_checkpoint.py:60`) and restored on resume (`_workflows/_runner_context.py:414-426`). Responses are correlated by `request_id` (`_workflows/_runner_context.py:457-486`) and routed to `response_handler` methods on the original executor (`_workflows/_request_info_mixin.py:1-115`). The system can say exactly what is still waiting.

- **Streaming vs non-streaming share the same atomicity model.** Both branches go through `_get_response` / `_stream` with the same `attempt_idx` loop; only the inner client call differs. The `_record_progress` callback is invoked on iteration boundary in both branches (`_harness/_loop.py:580-589`, `_harness/_loop.py:495-504`).

## Notable Patterns

- **Compile-time type-driven handler dispatch.** `Executor._discover_handlers` introspects `@handler`-decorated methods to register input types; `can_handle(message)` uses `is_instance_of` to dispatch (`_workflows/_executor.py:329-377`). Adding a new message type is a new `@handler` method; the registry is rebuilt at construction.

- **Live mutable tools list passed to the tool invocation context.** `live_tools: list[ToolTypes] | None` flows from `normalize_tools(mutable_options["tools"])` into `FunctionInvocationContext.tools`, so a tool can `ctx.tools.append(...)` mid-run ("progressive tool exposure", `_tools.py:2544-2551`, `_tools.py:1434-1450`).

- **`to_dict` / `from_dict` as the universal portability boundary.** `AgentSession.to_dict` / `from_dict` (`_sessions.py:779-811`), `WorkflowCheckpoint.to_dict` / `from_dict` (`_workflows/_checkpoint.py:90-116`), `WorkflowMessage.to_dict` / `from_dict` (`_workflows/_runner_context.py:64-94`), and `WorkflowEvent.to_dict` / `from_dict` for `request_info` events (`_workflows/_events.py:412-447`) all share the `SerializationProtocol` convention — `to_dict`/`from_dict` with a `"type"` discriminator — so durable and per-tool storage use the same round-trip pattern.

- **`MiddlewareTermination` as a structured control-flow exit.** A typed exception with an optional `result` attribute, catchable per-pipeline-pass so the loop can decide whether the boundary completed (`_middleware.py:72-80`, `_tools.py:1612-1629`).

- **Origin-tagged events.** `WorkflowEvent.origin = FRAMEWORK | EXECUTOR` carried by `_event_origin_context: ContextVar` to discriminate framework-orchestrated events from user-executor events in the same stream (`_workflows/_events.py:38-55`). This is the answer to "which side of the boundary produced this event?".

- **`executor_bypassed` is a replay-aware event.** Documented as `"executor skipped via cache hit during replay"` (`_workflows/_events.py:340-342`) — i.e., the framework records when an executor did NOT execute in the current superstep because its work was retrieved from a cached result. This is the explicit "what did not run" signal.

## Tradeoffs

- **`max_function_calls` is best-effort, not exact** (`_tools.py:2636-2644`). Callers needing hard limits should use `tool_choice="none"` or `max_iterations` to bound the *rounds*, accepting that the last round's parallel batch may exceed the cap. Tested at `tests/core/test_function_invocation_logic.py`.

- **Agent-loop crash semantics rely on history provider, not transactions.** If `call_next()` raises inside `AgentLoopMiddleware._process_non_streaming` before the partial iteration is appended to `aggregated`, that iteration is lost. With `require_per_service_call_history_persistence`, the previous completed iteration *is* on disk (via `_persist_service_call_response`), so a caller can re-run and the loop boundary is visible in history. Without it, the partial iteration is invisible — the system can say how many *complete* iterations ran (via the user-supplied `should_continue` counter) but cannot say what was inside the failed one.

- **Executor failure is fatal, not retried.** `Executor.execute` raises on failure; the workflow surfaces `executor_failed` and `failed` events and the run terminates (`_workflows/_executor.py:280-285`; `_workflows/_workflow.py:606-628`). No automatic retry of executors exists in the runner; retry is delegated to user code via custom executors or to durable execution frameworks (Durable Task package, etc.) that wrap executor invocations.

- **`_aggregate_response` overwrites metadata from earlier iterations.** `aggregated` includes every iteration's messages plus injected nudges, but `response_id`, `agent_id`, `finish_reason`, and `value` come from the *final* iteration (`_harness/_loop.py:681-702`). Caller inspecting the metadata of an arbitrary iteration has to read the streamed updates or the per-iteration `last_result` they accumulate themselves.

- **Inconsistent cancellation surfaces.** `AgentLoopMiddleware` handles `MiddlewareTermination` cleanly (`_harness/_loop.py:546-552`); the chat loop has no documented cancellation token; the Runner awaits an `asyncio.CancelledError` and propagates by cancelling the iteration task (`_workflows/_runner.py:115-120`). The semantic of "stop right now" varies by layer.

- **`fresh_context` mode truncates `service_session_id`.** When `AgentLoopMiddleware(fresh_context=True)` is used with a session, the service-side conversation id is dropped at every iteration boundary (`_harness/_loop.py:417-421`, `507-510`). This is a correctness detail that callers must know about: the loop will not benefit from any service-managed history continuity.

- **No unified replay semantic.** A checkpoint restores `state` and `messages` but does *not* restore conversation_id history tied to a prior model call; the resumed run may behave differently on the next model round if the underlying service treats its conversation IDs as ephemeral.

## Failure Modes / Edge Cases

- **Cascading tool failure in a parallel batch.** All calls in `asyncio.gather` execute even if one raises (`_tools.py:1821-1823`); `_auto_invoke_function` catches per-call, so one failing call does not cancel siblings. `errors_in_a_row` is incremented only if any result has `.exception` set (`_tools.py:1855`).

- **Loop exhaustion with orphan function_calls.** When `max_iterations` is reached mid-batch, the framework forces `tool_choice="none"` and makes one tool-less call (`_tools.py:2668-2696`). This avoids leaving the model in a state with `function_call` items that have no matching result, but it does mean the next text response is artificially ungrounded in tools.

- **Invalid tool arguments as a soft error, not an exception.** `_validate_arguments_against_schema` failures return `Content.from_function_result(..., exception=str(exc))` (`_tools.py:1519-1528`) without re-raising. The model is told the call was bad and is expected to retry with valid arguments.

- **Unknown function name + `terminate_on_unknown_calls=True`.** Raises `KeyError` immediately, terminating the function loop (`_tools.py:1719-1720`). Without the flag, the same condition returns an error result Content and the model continues.

- **`tool_choice="required"` resets after one iteration** to prevent infinite loops (`_tools.py:2647-2652`); an agent that persistently cannot produce text without tools is force-broken after one iteration.

- **Concurrent `agent.run` on the same Workflow instance is forbidden.** `_ensure_not_running` raises if `_is_running` (`_workflows/_workflow.py:379-383`). Multi-run semantics require separate Workflow instances.

- **State staging losses on exception mid-superstep.** `State.commit()` happens only after successful superstep completion (`_workflows/_runner.py:141`); a crash leaves `_pending` values uncommitted and not part of the checkpoint (since `export_state` returns committed only, `_workflows/_state.py:106-111`). The next checkpoint, if any, will start from a state that does not include the half-superstep writes.

- **`request_info` events without matches.** `send_request_info_response(request_id, response)` raises `ValueError("No pending request found")` if `request_id` is not in `_pending_request_info_events` (`_workflows/_runner_context.py:464-466`).

- **Workflow graph drift across restore.** `restore_from_checkpoint` validates `self._graph_signature_hash == checkpoint.graph_signature_hash` and refuses to restore otherwise (`_workflows/_runner.py:275-279`).

- **Approval-bypass hazard.** The harness auto-approves a tool in a mixed batch when a session carries already-approved approvals for the same batch, and reinjects only when the visible approval flow resumes (`python/packages/core/AGENTS.md`, `_harness/_tool_approval.py`). A model-issued tool call in a half-approved batch can run without per-call approval — this is documented but is the kind of edge case where a partial-completion marker would help.

- **Streaming + non-streaming test artifact: `ext registry stage`.** Conversation-id propagation in streaming mode can fail mid-batch, causing partial updates with mismatched IDs (covered in tests, but the framework's behavior is "best-effort" here too, `_agents.py:1136-1158`).

## Future Considerations

- **A unifying "atomic unit of execution" abstraction.** Today, four layers each define their own iteration/attempt boundary; a future redesign could pick a single vocabulary (e.g., always "step") and decorate each with layer-specific metadata.

- **Exact `max_function_calls`.** The current best-effort cap is the most surprising behavior; tightening it would let callers reason about cost ceilings precisely. The current implementation could pre-count the batch and skip tools that would exceed the cap, but that changes the model's view of available tools.

- **Standardized crash-recovery for `AgentLoopMiddleware`.** The dependency on `PerServiceCallHistoryPersistingMiddleware` for partial completion recall is implicit; lifting it into `AgentLoopMiddleware` directly (or providing a documented contract) would close the agent-loop crash-recovery gap.

- **Executor-level retry via middleware.** Adding an `ExecutorMiddleware` analogous to `FunctionMiddleware` would let the framework opt into automatic retry of failed executors, currently delegated entirely to user code.

- **Streaming cancellation token plumbing.** A consistent `CancellationToken` (or `asyncio.Task` reference) flowing through `AgentContext` and `FunctionInvocationContext` would unify cancellation across layers and let callers safely pause mid-tool-call.

- **Mixed-batch atomicity.** Today a parallel batch is the atomic group; the API could expose a per-batch transaction marker so callers can decide whether to commit or roll back the batch on partial failure.

## Questions / Gaps

- **What is the lifecycle of an `AgentLoopMiddleware` aggregate response when streaming and `MiddlewareTermination` interrupts between iterations?** Tests cover the clean termination case (`tests/core/test_harness_loop.py:1111-1138`) but the contract for `aggregated` is ambiguous when termination occurs inside `await call_next()` versus after; documentation only handles the post-iteration case.

- **Does the framework guarantee `function_call` / `function_result` pairing on persistence?** With `require_per_service_call_history_persistence=True`, history is persisted after the model call but the partial model response is the one written. A mid-batch tool crash leaves a `function_call` history entry without a `function_result` if the partial batch is persisted before `_handle_function_call_results` resolves. This is recoverable on next run via history replay, but the design is implicit. No evidence found of an explicit "function call pair" invariant at the persistence layer.

- **What happens to in-flight parallel tool calls on `_McpError` from a remote server mid-call?** The MCP retry / cancel policies (`_mcp.py` long-running task section in `AGENTS.md`) cover connection loss and slow responses; a synchronous tool with a remote dependency does not appear to have an atomic boundary beyond `FunctionTool.__call__`'s `try/except` in `_tools.py:520-553`.

- **What is the post-condition of a `WorkflowCheckpoint.save` if the workflow process crashes between `_create_checkpoint_if_enabled` returning and the next superstep starting?** The checkpoint is saved before `superstep_started` for the next iteration (`_workflows/_runner.py:143-144`); on next restore, the iteration count is loaded verbatim, but no evidence found of a documented "what iteration is this checkpoint for?" assertion surface beyond `iteration_count`.

- **Is the `iteration` argument to `should_continue` guaranteed monotonic across `should_continue` / `next_message` invocations within the same iteration?** Yes per `_harness/_loop.py:471-503` where `loop_kwargs` is rebuilt with the same `iteration` for `_evaluate_stop` and `_record_progress`, but the field is set in three places (`_harness/_loop.py:471-478`, `484-491`, `496-504`), and a future refactor could drift.

- **Are streaming and non-streaming `superstep_started` semantics mirrored at the agent-loop layer (i.e., a `loop_iteration_started` event)?** No — agent-loop emits no formal event between iterations; the only persistent record is the `should_continue` predicate's view (`iteration`, `last_result`). Tracing at the agent-loop layer relies on whatever the wrapped pipeline (`FunctionInvocationLayer` → `OpenAIChatClient`, etc.) emits. No clear evidence found of an agent-loop-iteration span outside the chat-client spans.

---

Generated by `01.03-step-turn-and-task-atomicity` against `agent-framework`.
