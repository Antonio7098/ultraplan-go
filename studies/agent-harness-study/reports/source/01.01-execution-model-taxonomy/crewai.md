# Source Analysis: crewai

## Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo: `crewai`, `crewai-core`, `crewai-tools`, `crewai-files`) |
| Analyzed | 2026-07-01 |

## Summary

CrewAI deliberately ships **four layered execution models** rather than committing to a single taxonomy. The framework provides:

1. A **ReAct loop** (text-parsed and native-tool variants) in `lib/crewai/src/crewai/agents/crew_agent_executor.py` — the classical "Thought → Action → Observation" pattern marked deprecated in favor of the experimental executor.
2. A **Plan-and-Execute / step loop** in `lib/crewai/src/crewai/experimental/agent_executor.py` — a `Flow` subclass that generates a plan, executes one step at a time via `StepExecutor`, observes results with a `PlannerObserver`, and can replan/refine.
3. A **DAG / event-driven router** in `lib/crewai/src/crewai/flow/runtime/__init__.py` (`Flow` class) — declarative `@start`/`@listen`/`@router` graph with AND/OR join semantics.
4. A **linear / hierarchical task scheduler** in `lib/crewai/src/crewai/crew.py:_execute_tasks` — top-level Crew loop over tasks with optional `task.async_execution` for fan-out.

The most accurate one-sentence answer to "what advances execution" is: **in the Plan-and-Execute Flow each `@router` method emits a Literal label (e.g. `"continue_plan"`, `"replan_now"`, `"all_todos_complete"`) which the Flow runtime matches against `@listen` conditions to fire the next method.** The runtime advances work via an event-style label dispatch implemented as recursive `_execute_listeners` calls (`flow/runtime/__init__.py:3239`) rather than a traditional while-loop.

The model is **explicit and well-documented in code** (decorator names, type annotations on router return values, dedicated `FlowMethodDefinition` schema in `flow/flow_definition.py:612`), but it is **layered**: a `Flow` is the substrate, `AgentExecutor` wraps a Flow around one agent's task, and `Crew` wraps a sequential/hierarchical loop around multiple agents. New users must choose at three different layers (Crew process, agent executor class, Flow graph shape), so the model is hard to explain to a new contributor even though each layer is individually clear.

## Rating

**7 / 10** — Clear model with explicit interfaces, full type annotations, and strong test coverage, but layered across three abstractions that are not always trivially composable.

- Explicit, code-anchored: ✅ decorators, `FlowMethodDefinition`, `Literal[...]` return types, dedicated runtime (`Flow[AgentExecutorState]`)
- Tests prove behavior: ✅ `lib/crewai/tests/test_flow.py` (2300+ lines), `lib/crewai/tests/agents/test_agent_executor.py`, `lib/crewai/tests/agents/test_lite_agent.py`
- Persistence & resumption: ✅ `flow/persistence/`, `CheckpointConfig` (`flow/runtime/__init__.py:850`, `crew.py:998`)
- Failure-mode handling: ✅ recursion cap (`max_method_calls`), `_execution_lock`, parallel-tool gating (`_should_parallelize_native_tool_calls`), replan cap (`max_replans`)
- Easy to explain to a new contributor: ❌ three independent execution abstractions (Flow, AgentExecutor, Crew process) plus a deprecated ReAct path
- Proves out under scale / observability: ✅ event bus emits `MethodExecutionStartedEvent`/`FinishedEvent`/`PausedEvent`; tracing listener (`trace_listener.py`)

## Evidence Collected

Every entry includes file path with line number. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Primary runtime entry (Flow) | `Flow.kickoff()` wraps async engine in a thread for sync callers | `lib/crewai/src/crewai/flow/runtime/__init__.py:2110` |
| Async entry (Flow) | `Flow.kickoff_async()` runs start methods sequentially or in parallel via `asyncio.gather` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2209`, `:2491-2502` |
| Start-method execution | `_execute_start_method` injects trigger payload, calls `_execute_method`, fans out to `_execute_listeners` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2700-2750` |
| Method execution | `_execute_method` runs sync methods on a thread pool via `asyncio.to_thread`, awaits coroutine returns | `lib/crewai/src/crewai/flow/runtime/__init__.py:2789-2883` |
| Listener dispatch | `_execute_single_listener` recursively calls `_execute_listeners` after each completion | `lib/crewai/src/crewai/flow/runtime/__init__.py:3188-3243` |
| OR-listener racing | `_execute_racing_listeners` runs racing listeners in parallel, cancels on first completion | `lib/crewai/src/crewai/flow/runtime/__init__.py:1380-1429` |
| DAG DSL — start decorator | `@start` marks unconditional or conditional entry points | `lib/crewai/src/crewai/flow/dsl/_start.py:18` |
| DAG DSL — listen decorator | `@listen` binds a trigger condition to a method | `lib/crewai/src/crewai/flow/dsl/_listen.py:18` |
| DAG DSL — router decorator | `@router` enforces `Literal[...]` / `Enum` return events, captured as `FlowMethodDefinition.router=True` | `lib/crewai/src/crewai/flow/dsl/_router.py:97` |
| Static Flow contract | `FlowMethodDefinition` (do, start, listen, router, emit, persist, human_feedback) | `lib/crewai/src/crewai/flow/flow_definition.py:612` |
| Top-level Flow container | `Flow(BaseModel, Generic[T], metaclass=FlowMeta)` — typed state, Pydantic | `lib/crewai/src/crewai/flow/runtime/__init__.py:719` |
| Plan-and-Execute executor | `AgentExecutor(Flow[AgentExecutorState], BaseAgentExecutor)` | `lib/crewai/src/crewai/experimental/agent_executor.py:164` |
| Plan-and-Execute state | `AgentExecutorState` holds messages, iterations, todos, plan, replan_count, observations, execution_log | `lib/crewai/src/crewai/experimental/agent_executor.py:126-162` |
| Plan generation entry | `@start() generate_plan` — only fires if `agent.planning_enabled` | `lib/crewai/src/crewai/experimental/agent_executor.py:314-353` |
| Step observation router | `@router("step_executed") observe_step_result` routes to low/medium/high-effort handler | `lib/crewai/src/crewai/experimental/agent_executor.py:608-673` |
| Plan continue/replan router | `@router("continue_plan")`, `@router("replan_now")`, `@router("goal_achieved")` | `lib/crewai/src/crewai/experimental/agent_executor.py:943-1016` |
| Todo readiness router | `@router("has_todos") get_ready_todos_method` returns `single_todo_ready` / `multiple_todos_ready` / `all_todos_complete` / `needs_replan` | `lib/crewai/src/crewai/experimental/agent_executor.py:1033-1066` |
| Step executor (isolated per-step) | `StepExecutor.execute` runs a multi-turn `for _ in range(max_step_iterations)` loop on a fresh message list | `lib/crewai/src/crewai/agents/step_executor.py:126-183`, `:317-367` |
| Native tool step loop | `StepExecutor._execute_native` iterates LLM → tool calls → observation | `lib/crewai/src/crewai/agents/step_executor.py:528-578` |
| Parallel step execution | `execute_todos_parallel` runs ready todos via `asyncio.gather(*[_run_step(...)])` | `lib/crewai/src/crewai/experimental/agent_executor.py:1185-1219` |
| Replanning trigger | `_trigger_replan` regenerates `TodoList` while preserving completed history | `lib/crewai/src/crewai/experimental/agent_executor.py:2504-2589` |
| Iteration cap enforcement | `@router(or_(initialize_reasoning, continue_iteration)) check_max_iterations` returns `force_final_answer` | `lib/crewai/src/crewai/experimental/agent_executor.py:2112-2123` |
| Termination guard | `@listen(...) finalize` uses `_finalize_lock` + `_finalize_called` to avoid double-finalization | `lib/crewai/src/crewai/experimental/agent_executor.py:2246-2326` |
| ReAct loop (text) | `while not isinstance(formatted_answer, AgentFinish):` inside `CrewAgentExecutor._invoke_loop_react` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:330-468` |
| ReAct loop (native tools) | `while True: ...` inside `CrewAgentExecutor._invoke_loop_native_tools` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:484-` |
| LiteAgent loop | `while not isinstance(formatted_answer, AgentFinish):` in `LiteAgent._invoke_loop` | `lib/crewai/src/crewai/lite_agent.py:860-960` |
| Agent dispatcher | `Agent.execute_task` builds prompt, calls `self.agent_executor.invoke(...)` which routes to the Flow kickoff | `lib/crewai/src/crewai/agent/core.py:786-855` |
| Async agent dispatcher | `Agent.aexecute_task` uses `asyncio.wait_for` for timeout, calls `self.agent_executor.ainvoke` | `lib/crewai/src/crewai/agent/core.py:922-1035` |
| Executor class picker | `executor_class` field with `_validate_executor_class` warns on `CrewAgentExecutor` (deprecated) | `lib/crewai/src/crewai/agent/core.py:135-164` |
| Crew sequential process | `Crew._run_sequential_process` → `_execute_tasks(self.tasks)` | `lib/crewai/src/crewai/crew.py:1485-1487` |
| Crew hierarchical process | `Crew._run_hierarchical_process` creates a manager agent then runs the same task loop | `lib/crewai/src/crewai/crew.py:1489-1519` |
| Crew process enum | `Process.sequential`, `Process.hierarchical`; `consensual` is `TODO` | `lib/crewai/src/crewai/process.py:4-10` |
| Crew async task fan-out | `Task.execute_async` returns a `Future[TaskOutput]` via a thread + `contextvars.copy_context()` | `lib/crewai/src/crewai/task.py:596-` |
| Crew kickoff dispatch | `if self.process == Process.sequential` / `Process.hierarchical` in both sync and async kickoff | `lib/crewai/src/crewai/crew.py:1037-1044`, `:1249-1256` |
| Conditional task skip | `ConditionalTask.should_execute(context)` checked per task in `_execute_tasks` | `lib/crewai/src/crewai/tasks/conditional_task.py:41-55` |
| Event bus | `CrewAIEventsBus` singleton with dedicated daemon event loop and `ThreadPoolExecutor` | `lib/crewai/src/crewai/events/event_bus.py:94-189` |
| Recursion safety | `if count > self.max_method_calls: raise RecursionError(...)` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3178-3185` |
| Locked runtime state | `StateProxy` wraps state with `threading.Lock` (`LockedDictProxy`/`LockedListProxy`) | `lib/crewai/src/crewai/flow/runtime/__init__.py:353-664` |
| Process model claim in README | "event-driven workflows" and "Crews and Flows architecture" | `README.md:173`, `:585` |

## Answers to Dimension Questions

1. **What is the primary execution model?**
   There is no single primary model. CrewAI exposes four layered models:
   - **Flow DAG** (`flow/runtime/__init__.py:719`) — declarative `@start`/`@listen`/`@router` graph; runtime is `kickoff_async` → execute-start-methods → recursive `_execute_listeners` (`flow/runtime/__init__.py:3239`).
   - **Plan-and-Execute state machine** (`experimental/agent_executor.py:164`) — `Flow[AgentExecutorState]` that emits router labels `("continue_plan"`, `"replan_now"`, `"all_todos_complete"`, …) to advance through todos.
   - **ReAct loop** (`agents/crew_agent_executor.py:309`) — classic `while not AgentFinish` over `get_llm_response` → tool → observation.
   - **Crew task scheduler** (`crew.py:1485-1598`) — `for task in tasks:` with optional `task.async_execution` futures drained at sync boundaries.

2. **Is it explicit or emergent?**
   **Explicit.** Every model has named decorators, return-type Literals, dedicated runtimes, and contracts:
   - DSL: `crewai/flow/dsl/_start.py:18`, `_listen.py:18`, `_router.py:97`
   - Static schema: `FlowMethodDefinition` (`flow/flow_definition.py:612`) — `do`, `start`, `listen`, `router`, `emit`, `persist`, `human_feedback`
   - Executor classes are first-class and selectable via `executor_class` on `Agent` (`agent/core.py:337-344`).

3. **Does the model match the product shape?**
   Mostly yes. CrewAI sells "Crews + Flows + Plans" — three user-facing surfaces that each map cleanly to one of the four internal models. The match breaks at the seams:
   - The Plan-and-Execute model is nested *inside* an Agent, not exposed as a top-level Crew-level planner. Users wanting Crew-level replanning must reach into `PlanningConfig` per agent.
   - `lite_agent.LiteAgent` uses a different (ReAct) loop than `Agent` (Plan-and-Execute). This is undocumented at the abstraction boundary — same `kickoff()` surface, different control flow.
   - The hierarchical process auto-creates a manager agent that simply delegates (`crew.py:1489-1519`); it is not a real planner that *executes* plans.

4. **Is the model easy to explain to a new contributor?**
   **No.** The mental model is:
   1. Pick a `Process` (`sequential` / `hierarchical`).
   2. Pick an `executor_class` (`AgentExecutor` recommended, `CrewAgentExecutor` deprecated).
   3. Optionally wrap in a `Flow` with `@start`/`@listen`/`@router`.
   4. Optionally enable `PlanningConfig` per agent to flip into Plan-and-Execute inside the executor.
   This is four orthogonal axes that interact (e.g., a `hierarchical` crew uses `AgentExecutor` if its manager is a `Agent` subclass, but the manager is created via raw `Agent(...)` at `crew.py:1509` and inherits the default `executor_class=AgentExecutor`). The README's "Understanding Flows and Crews" section does not cover the Plan-and-Execute option at all.

5. **Does the system mix models cleanly or accidentally?**
   **Mostly cleanly at the seams, accidentally at the entry points.**
   - Clean: `Flow` exposes a stable contract (`FlowDefinition`, persistence, resumability) that all consumer models (Plan-and-Execute AgentExecutor, conversational mixin, scripted/CLI action handlers) reuse.
   - Accidental: The deprecated `CrewAgentExecutor` is still the default-ish path inside `LiteAgent` (`lite_agent.py:860`) and inside the legacy hierarchical path, so users hitting `agent_executor_type="crew"` warnings at `agent/core.py:157-163` may be confused by `LiteAgent` continuing to use it.

## Architectural Decisions

- **Layered orchestration stack.** Decision: put a Flow runtime at the bottom, wrap it for agent execution, wrap *that* for crew scheduling. Evidence: `AgentExecutor(Flow[AgentExecutorState], BaseAgentExecutor)` at `experimental/agent_executor.py:164`; `Crew._execute_tasks` at `crew.py:1529`.
- **Static + dynamic Flow contracts.** Decision: `FlowDefinition` is a serializable Pydantic schema (`flow_definition.py:679`) so flows can be persisted, visualized (`flow/visualization/`), and instantiated from dict (`Flow.from_definition` at `flow/runtime/__init__.py:770`).
- **Plan-and-Execute as the default executor.** Decision: `executor_class: type = Field(default=AgentExecutor, …)` with an explicit `DeprecationWarning` when users pick `CrewAgentExecutor` (`agent/core.py:157-163`).
- **Reasoning effort as a routing knob.** Decision: a single observation fans out to three routers (`step_observed_low`, `_medium`, `_high`) selected by `reasoning_effort` (`experimental/agent_executor.py:608-673`). This is explicit control over the Plan-and-Act "Section 3.3" tradeoff between latency and adaptivity.
- **Cycle-safe listener dispatch.** Decision: `_completed_methods` + `_clear_or_listeners()` (`flow/runtime/__init__.py:1261-1265`, `:3188-3211`) make cyclic flows possible. The same machinery supports router-loops because router results become `FlowMethodName` triggers (`flow/runtime/__init__.py:2739-2748`).
- **Recursion cap on Flow method calls.** Decision: `self.max_method_calls = self.max_iter * 10` (`experimental/agent_executor.py:229`) and a hard `RecursionError` at `flow/runtime/__init__.py:3179-3185` to prevent infinite listener loops.
- **Two-tier concurrency.** Decision: tasks can opt into `async_execution=True` (`task.py:596-`) to fan out at the Crew layer, *and* within an agent the Plan-and-Execute layer can run multiple ready todos concurrently via `asyncio.gather` (`experimental/agent_executor.py:1216`). The two never interleave — async tasks are drained at sync task boundaries (`crew.py:1579-1583`, `:1595-1596`).
- **Per-step isolation.** Decision: `StepExecutor` owns its own message list and never reads/writes the parent's state (`agents/step_executor.py:63-125`). Only dependency *results* flow across steps via `StepExecutionContext` (`utilities/step_execution_context.py`).

## Notable Patterns

- **Decorator-defined DAG with Literal-typed router emissions.** `@router(begin)` whose return annotation is `Literal["approved", "revise"]` is parsed by `_get_router_return_events` (`flow/dsl/_router.py:86-88`) and stored as `FlowMethodDefinition.emit`. The runtime uses the emitted literal as a `FlowMethodName` to trigger `@listen("approved")` handlers (`flow/runtime/__init__.py:2739-2748`). Strong typing propagates from method signature to scheduler.
- **Locked state proxies.** `StateProxy`, `LockedDictProxy`, `LockedListProxy` (`flow/runtime/__init__.py:377-664`) wrap every read/write to `self.state` to make Flows safe under the async sync mixing that Flow does (sync method bodies run on a thread pool via `asyncio.to_thread`, see `flow/runtime/__init__.py:2832-2841`).
- **Label-based router dispatch inside a Flow.** `AgentExecutor` reuses Flow routing as its *internal* state machine — `@router("step_executed")`, `@router("continue_plan")`, etc. The whole Plan-and-Execute algorithm is expressed as a set of `@listen` / `@router` methods against `AgentExecutorState`. This is unusual: most agent frameworks use a `while` loop. CrewAI reuses its own DAG runtime to implement a step loop.
- **Two-loop pattern: outer plan loop + inner step loop.** `AgentExecutor` advances plan-level todos; each todo is itself executed by `StepExecutor`, which runs its own bounded `for _ in range(max_step_iterations)` (`step_executor.py:336`). The inner loop never touches the outer plan state — isolation is enforced architecturally.
- **Polling-thread async event bus.** `CrewAIEventsBus` runs async handlers on a dedicated `asyncio.new_event_loop()` in a daemon thread (`events/event_bus.py:183-189`). Sync handlers run on a `ThreadPoolExecutor(max_workers=10)`. This keeps Flow routing synchronous-friendly without forcing every consumer to be async.
- **First-wins racing OR.** `_execute_racing_listeners` (`flow/runtime/__init__.py:1380-1429`) cancels later racing listeners when the first completes — a primitive useful for `@listen(or_(...))` where only one branch should proceed.

## Tradeoffs

- **Four models, four mental loads.** Each has its own vocabulary: `step_observed_high`, `replan_now`, `ToolResult.is_final`, `Process.hierarchical`. New contributors can ship one without understanding the others, but debugging cross-cutting issues (e.g., "why did my hierarchical manager not replan?") requires understanding all four.
- **ReAct loop is deprecated but still in LiteAgent.** `LiteAgent._invoke_loop` (`lite_agent.py:860`) still uses the legacy ReAct pattern. This means an agent created with `LiteAgent(...)` cannot use `PlanningConfig`, parallel step execution, or `PlannerObserver`. The path is undocumented in the model and in the README.
- **Plan-and-Execute is nested under one agent.** There is no Crew-level planner; replanning happens per agent (`AgentExecutor`). A crew with three agents cannot share a plan or replan across agents.
- **Hierarchical process is a thin delegation layer.** It only adds a manager agent whose tools are `AgentTools(agents=self.agents)` (`crew.py:1513`). No real supervisor loop, no replanning — the manager is itself an `Agent` running through `AgentExecutor`. The TODO comment at `process.py:11` (`consensual = 'consensual'`) shows further variants were considered and dropped.
- **Recursion protection is coarse.** `max_method_calls` counts every listener invocation including successful ones; an aggressive router that legitimately fans out 30 times per cycle on a 25-step plan can hit the cap (`experimental/agent_executor.py:229`: `self.max_iter * 10`).
- **Plan-and-Execute carries real per-step overhead.** Every todo step pays for `PlannerObserver.observe` (LLM call unless `reasoning_effort="low"`), `_should_replan` check, and message-list marshalling. For trivial two-tool workflows the legacy ReAct loop is faster and simpler.
- **Literal-typed routers are great for type checking but brittle at runtime.** Renaming a literal value (e.g., `"continue_plan"` → `"continue"`) silently breaks routing because there is no central registry. The emit list is captured at Flow-definition time (`flow/dsl/_router.py:144-147`).
- **Parallel-tool safety gating is heuristic.** `_should_parallelize_native_tool_calls` returns False if any tool is `result_as_answer` or has a `max_usage_count` (`experimental/agent_executor.py:1827-1858`). Tools that mutate shared state and don't set these flags can still race.

## Failure Modes / Edge Cases

- **Stuck plan with no ready todos.** `get_ready_todos_method` returns `"needs_replan"` instead of `"all_todos_complete"` when pending todos exist but none have satisfied dependencies (`experimental/agent_executor.py:1049-1060`). Without this guard, the executor would finalize an incomplete plan.
- **Double-finalize races.** `finalize` uses a `threading.Lock` + `_finalize_called` flag because concurrent branches can reach a terminal state simultaneously (`experimental/agent_executor.py:2261-2270`). The same applies to `invoke` (`experimental/agent_executor.py:2736-2742`) where `self._is_executing` rejects concurrent calls with a clear `RuntimeError`.
- **Native-tool provider rejection.** When the LLM provider rejects native tools, `call_llm_native_tools` catches `is_native_tool_calling_unsupported_error` and routes to `"continue_reasoning"` (text tool calling) (`experimental/agent_executor.py:1537-1540`). The state machine cleanly downgrades.
- **Parser errors do not abort.** `recover_from_parser_error` increments `iterations` and routes back to `"initialized"` (`experimental/agent_executor.py:2678-2699`), bounded by `log_error_after` retries.
- **Context-length overrun.** `recover_from_context_length` calls `handle_context_length` (which summarizes) and retries (`experimental/agent_executor.py:2701-2715`).
- **Replan count exhaustion.** Both `handle_replan` and `handle_replan_now` short-circuit to `"all_todos_complete"` after `max_replans` replans (`experimental/agent_executor.py:984-992`, `:2660-2668`).
- **No-tool agents in Plan-and-Execute.** `effective_response_model = None if self.original_tools else self.response_model` (`experimental/agent_executor.py:1390-1392`) — when tools are present, structured-output is suppressed so the LLM can emit tool calls.
- **Async-from-sync trap.** `_execute_without_timeout` calls `invoke_result.close()` and raises a `RuntimeError` if the user calls `agent.execute_task` synchronously from inside a running event loop (`agent/core.py:913-919`). The error message points users at `akickoff` / `await aexecute_task`.
- **Hierarchical + tools on manager raises.** `_create_manager_agent` actively rejects managers that come with non-empty tools (`crew.py:1498-1505`), with `manager.tools = []` before raising — a one-line guard that nonetheless aborts the kickoff.
- **Cycle path completion-skew.** When `@router` returns a Literal that matches an existing method name AND a `@start("…")` exists for that label, both paths fire (`flow/runtime/__init.py:3196-3202`). This is documented but easy to misuse.

## Future Considerations

- **Consolidate around Plan-and-Execute.** `CrewAgentExecutor` and `LiteAgent._invoke_loop` should be retired (or made thin wrappers) so users do not need to pick between two internal patterns at the agent layer. Evidence: `agent/core.py:157-163` already warns on `CrewAgentExecutor`; the same warning is not present on `LiteAgent`.
- **Promote Plan-and-Execute to the Crew layer.** A `CrewPlanningConfig` analogous to `PlanningConfig` (`agent/planning_config.py`) would let a crew generate a plan across multiple agents and replan across agent boundaries. Today replanning is per-agent (`experimental/agent_executor.py:2452-2502`).
- **First-class cycle budget per listener.** `max_method_calls` is global (`flow/runtime/__init__.py:3178-3185`). Per-method budgets would let router-heavy flows express their cycle budget declaratively.
- **Static emit registry for routers.** Today the emit list is captured by reflecting the method's return annotation (`flow/dsl/_router.py:86-88`). A central registry would catch renames at definition time.
- **Hierarchical manager that is also a planner.** The current manager is a delegation agent (`crew.py:1513`); an `Agent` whose executor has `planning=True` and `allow_delegation=True` would give the manager genuine planning power instead of a tool-mediated LLM call.
- **Persistent execution_log.** `AgentExecutorState.execution_log` (`experimental/agent_executor.py:160-162`) is kept in memory only and explicitly excluded from the LLM ("NOT used for LLM calls"). A persistence hook for the audit log would enable postmortem analysis of replanning decisions.

## Questions / Gaps

- **Where is the canonical diagram of how `Crew` → `Agent` → `AgentExecutor` → `Flow` compose?** No code comment, README section, or docstring provides this. Evidence: searched `README.md:1-200`, `crew.py:980-1100`, `agent/core.py:786-870`, `experimental/agent_executor.py:1-180`. Only the high-level README sentence at `README.md:585` ("Crews and Flows architecture") hints at it.
- **Why is `LiteAgent` excluded from Plan-and-Execute?** No comment at `lite_agent.py:1-100` or `:860` explains the design choice. Both `Agent` and `LiteAgent` accept the same role/goal/backstory/tools surface.
- **Is there an SLA on Flow method call latency?** No explicit per-method budget. Reentrancy / cancellation only happens for OR racing (`flow/runtime/__init.py:1424-1426`).
- **How is `human_feedback` modeled as control flow?** `FlowHumanFeedbackDefinition` (`flow/flow_definition.py:647`) routes like a router, but the runtime path for *paused* flows uses persistence (`flow/runtime/__init.py:2518-2554`) and a `HumanFeedbackPending` exception. The relationship between paused-flow state and resumed-flow state is subtle — not summarized anywhere in code comments.
- **Does the Plan-and-Execute path support streaming tokens?** `crewai_event_bus.emit(..., ToolUsageStartedEvent/FinishedEvent)` is emitted per-tool-call but not per-token. Streaming is gated on `self.stream` in `Crew.kickoff` (`crew.py:1002`) and `Flow.kickoff` (`flow/runtime/__init.py:2148`). The Plan-and-Execute executor's per-step LLM call does not visibly plumb through to the streaming state.
- **What evidence exists that the Plan-and-Execute Flow is durable under failure?** `experimental/agent_executor.py` has `RecursionError` protection, replan caps, `_execution_lock`, `_finalize_lock`, and `state.execution_log`, but there is no documented chaos-engineering test or runbook for partial-failure recovery in `tests/experimental/` or `tests/agents/`.

---

Generated by `01.01-execution-model-taxonomy` against `crewai`.