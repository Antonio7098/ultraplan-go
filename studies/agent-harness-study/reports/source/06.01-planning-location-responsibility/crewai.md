# Source Analysis: crewai

## 06.01 Planning Location and Responsibility

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python / CrewAI (Pydantic, LiteLLM, Flow framework) |
| Analyzed | 2026-08-27 |

## Summary

CrewAI implements **two distinct planning layers** rather than a single centralized planner. (1) **Crew-level pre-planning** via `Crew.planning: bool` + `CrewPlanner` — a synchronous LLM call that generates task-level step plans and appends them to `task.description` before any agent executes. (2) **Agent-level Plan-and-Execute** via `Agent.planning_config: PlanningConfig` + `AgentReasoning` + `AgentExecutor(TodoList)` — a Flow-based executor that creates `PlanStep`→`TodoItem` decompositions through structured function-calling, then iterates isolated `StepExecutor` executions observed by `PlannerObserver` with effort-gated replanning/refinement. Planning is **explicit, runtime-owned, and embodied as Pydantic objects** (`ReasoningPlan`, `TodoList`, `StepObservation`) and Flow graph nodes, not prompt prose. Both layers are strictly optional (default disabled) and invisible to the alternate orchestration primitive `Flow` (DAG workflow), which carries its own declarative scheduler.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards; not yet fully durable/observable.**

Rationale: Planning is a real runtime object graph (types `lib/crewai/src/crewai/utilities/planning_types.py:14`, `lib/crewai/src/crewai/utilities/reasoning_handler.py:30`, `lib/crewai/src/crewai/experimental/agent_executor.py:135`; config `lib/crewai/src/crewai/agent/planning_config.py:11`) with dedicated handlers (`planning_handler.py`, `reasoning_handler.py`, `planner_observer.py`, `step_executor.py`), effort-graded observation/replan pipeline, dependency-aware `TodoList`, event emissions, and tests (`tests/utilities/test_structured_planning.py`, `tests/agents/test_agent_executor.py:1446`). Downgraded from 9-10 because crew-level planning mutates shared `task.description` strings rather than versioned artifacts, plan reuse/caching is absent, crew and agent planning layers are disjoint (no unified plan registry or external planner), and crew planning lacks TodoList integration.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Crew-level planning flag | `planning: bool \| None = Field(default=False, description="Plan the crew execution...")` and `planning_llm` field on Crew model | `lib/crewai/src/crewai/crew.py:344-357` |
| Crew planning entrypoint | `prepare_kickoff()` → `if crew.planning: crew._handle_crew_planning()` ordering guarantees planning before agent setup | `lib/crewai/src/crewai/crews/utils.py:376-378` |
| Crew planning implementation | `Crew._handle_crew_planning` creates `CrewPlanner(tasks, planning_llm)._handle_crew_planning()` and merges `plan_map` into `task.description += plan_map[n]` | `lib/crewai/src/crewai/crew.py:1451-1477` |
| CrewPlanner agent | `CrewPlanner._create_planning_agent()` returns `Agent(role="Task Execution Planner", goal="create extremely detailed step-by-step plan...", llm=planning_agent_llm)` | `lib/crewai/src/crewai/utilities/planning_handler.py:80-94` |
| CrewPlanner task | `_create_planner_task` creates `Task(..., output_pydantic=PlannerTaskPydanticOutput, agent=planning_agent)` with `list_of_plans_per_task: list[PlanPerTask]` | `lib/crewai/src/crewai/utilities/planning_handler.py:97-115`, `lib/crewai/src/crewai/utilities/planning_handler.py:15-34` |
| Agent planning config | `PlanningConfig` with `reasoning_effort: low/medium/high`, `observe_steps`, `max_attempts`, `max_steps`, `max_replans`, `max_step_iterations`, `step_timeout`, `plan_prompt/system_prompt/refine_prompt`, `llm` | `lib/crewai/src/crewai/agent/planning_config.py:11-151` |
| Agent planning fields | `Agent.planning_config: PlanningConfig \| None`, `planning: bool=False`, deprecated `reasoning: bool` + `max_reasoning_attempts` | `lib/crewai/src/crewai/agent/core.py:277-293` |
| Planning enablement logic | `planning_enabled` property; `post_init_setup()` maps `planning=True` → `PlanningConfig(reasoning_effort="low", max_attempts=1)` if no config | `lib/crewai/src/crewai/agent/core.py:406-410`, `lib/crewai/src/crewai/agent/core.py:396-402` |
| Agent reasoning handler | `AgentReasoning` class: `_execute_planning()` → `_create_initial_plan()` + `_refine_plan_if_needed()` loop gated by `config.max_attempts` | `lib/crewai/src/crewai/utilities/reasoning_handler.py:244-349` |
| Planner prompts | `planning.system_prompt`, `create_plan_prompt`, `refine_plan_prompt`, `observation_system_prompt`, `step_executor_system_prompt` templates retrieved via `I18N_DEFAULT.retrieve("planning", ...)` | `lib/crewai/src/crewai/translations/en.json:84-102`, `lib/crewai/src/crewai/utilities/reasoning_handler.py:482-571` |
| Function-calling schema | `FUNCTION_SCHEMA` for `create_reasoning_plan(plan, steps[], ready)` with `PlanStep{step_number, description, tool_to_use, depends_on}` | `lib/crewai/src/crewai/utilities/reasoning_handler.py:50-104` |
| Planning types | `PlanStep`, `TodoItem{status: pending/running/completed/failed, depends_on, result}`, `TodoList` with `get_ready_todos()`, `can_parallelize`, `replace_pending_todos()` for replanning | `lib/crewai/src/crewai/utilities/planning_types.py:14-196` |
| Observation types | `StepObservation{step_completed_successfully, key_information_learned, remaining_plan_still_valid, suggested_refinements: list[StepRefinement], needs_full_replan, goal_already_achieved}` + `StepRefinement{step_number, new_description}` | `lib/crewai/src/crewai/utilities/planning_types.py:198-278` |
| PlannerObserver | `PlannerObserver.observe()` LLM call with `response_model=StepObservation`, heuristic fallback `heuristic_observation()`, `apply_refinements()` in-place todo mutation | `lib/crewai/src/crewai/agents/planner_observer.py:113-242`, `lib/crewai/src/crewai/agents/planner_observer.py:88-111` |
| StepExecutor | `StepExecutor.execute(todo, context, max_step_iterations, step_timeout)` isolates messages, multi-turn `llm.call → tool → observation` loop for native/text tool calling | `lib/crewai/src/crewai/agents/step_executor.py:64-242`, `lib/crewai/src/crewai/agents/step_executor.py:328-378` |
| AgentExecutor planning flow | `AgentExecutorState{plan, plan_ready, todos: TodoList, observations, replan_count}` + Flow nodes: `@start generate_plan()`, `@router check_todos_available → get_ready_todos_method → execute_todo_sequential/execute_todos_parallel → observe_step_result → handle_* → decide_next_action → replan/refine/continue` | `lib/crewai/src/crewai/experimental/agent_executor.py:135-171`, `lib/crewai/src/crewai/experimental/agent_executor.py:348-408`, `lib/crewai/src/crewai/experimental/agent_executor.py:643-1028` |
| Effort gating | `_get_reasoning_effort()`, `_should_observe_steps()` (low=heuristic, medium/high=LLM), `_get_max_replans/step_iterations/timeout` from config | `lib/crewai/src/crewai/experimental/agent_executor.py:518-565` |
| Crew vs Agent executor selection | `Agent.executor_class = AgentExecutor (default)`; `BaseAgent` registers executor types `crew/experimental/base` | `lib/crewai/src/crewai/agent/core.py:345-351`, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:138-142` |
| Flow workflow graph (separate) | `Flow` as `RuntimeFlow[T]` + `FlowDefinition`, decorators `@start`, `@listen`, `@router`, `or_`, `and_` in `crewai.flow` package — distinct from planning, but `planning_config` exposed on `FlowAgentActionDefinition` templates | `lib/crewai/src/crewai/flow/flow.py:33-41`, `lib/crewai/src/crewai/flow/dsl/_start.py:14-25`, `lib/crewai/src/crewai/flow/skill.py:249-277` |
| Project/declarative planning config | YAML/JSON crew loader serializes `planning`, `planning_llm`, `planning_config` via `PlanningConfig` schema assertion | `lib/crewai/src/crewai/project/crew_definition.py:120-122`, `lib/crewai/src/crewai/project/json_loader.py:125-140`, `lib/crewai/tests/test_flow_from_definition.py:53-59` |
| Visibility / events | `AgentReasoningStartedEvent/CompletedEvent/FailedEvent`, `StepObservationStarted/Completed/Failed`, `PlanStepStarted/Completed`, `PlanReplanTriggered`, `PlanRefinement`, `GoalAchievedEarly` emitted on `crewai_event_bus` | `lib/crewai/src/crewai/utilities/reasoning_handler.py:197-242`, `lib/crewai/src/crewai/agents/planner_observer.py:137-213`, `lib/crewai/src/crewai/experimental/agent_executor.py:409-447`, `lib/crewai/src/crewai/experimental/agent_executor.py:991-1044` |
| Telemetry tagging | `feature_usage_span("planning:creation")`, `"planning:replan"`, `"planning:goal_achieved_early"` | `lib/crewai/src/crewai/events/event_listener.py:709-773` |
| Tests: structured planning | `TestFunctionSchema`, `TestAgentReasoningWithMockedLLM`, `TestTodoCreationFromPlan`, `TestTodoListIntegration` + provider VCR tests for OpenAI/Anthropic/Gemini/Azure | `lib/crewai/tests/utilities/test_structured_planning.py:1-669` |
| Tests: executor planning | `test_agent_kickoff_with_planning_stores_plan_in_state`, `test_planning_disabled_skips_planning`, `AgentExecutorState` with plan/todos fixtures | `lib/crewai/tests/agents/test_agent_executor.py:101-105`, `lib/crewai/tests/agents/test_agent_executor.py:1443-1490` |

## Answers to Dimension Questions

**1. Where does planning happen?**
Planning happens in **two places**, both inside runtime code (not external system):
- **Crew layer** — pre-kickoff in `lib/crewai/src/crewai/crews/utils.py:376` calling `lib/crewai/src/crewai/crew.py:1451` (`CrewPlanner` in `lib/crewai/src/crewai/utilities/planning_handler.py:57`). It runs before sequential/hierarchical task dispatch. Prompt comes from `lib/crewai/src/crewai/utilities/planning_handler.py:107-115` + i18n template, executed as a normal `Task.execute_sync()` with structured Pydantic output.
- **Agent layer** — at task start inside `AgentExecutor` Flow graph (`lib/crewai/src/crewai/experimental/agent_executor.py:348-389` `generate_plan()`), delegating to `AgentReasoning` (`lib/crewai/src/crewai/utilities/reasoning_handler.py:244`) which calls the agent's LLM with `FUNCTION_SCHEMA` (`lib/crewai/src/crewai/utilities/reasoning_handler.py:50-104`). Post-step, planning continues via `PlannerObserver.observe()` (`lib/crewai/src/crewai/agents/planner_observer.py:113`) and `StepExecutor.execute()` (`lib/crewai/src/crewai/agents/step_executor.py:127`) routed through Flow routers `observe_step_result → decide_next_action/handle_replan_now/handle_refine_and_continue` (`lib/crewai/src/crewai/experimental/agent_executor.py:643-1051`).
- **Not in workflow graph**: `crewai.flow` DAG (`lib/crewai/src/crewai/flow/flow.py:33`) is an explicit orchestration alternative, not a planner. It can host agent steps that themselves enable `planning_config`, but the Flow engine does not decompose tasks.

**2. Who owns the plan?**
- **Crew plan**: Owned by **runtime (Crew object)**. `CrewPlanner` is instantiated by `Crew` without caller interaction, result (`PlannerTaskPydanticOutput`) is owned as mutated `Task.description` strings (`lib/crewai/src/crewai/crew.py:1471-1472`). No plan registry; the plan is fused into the task prompt and not retained separately.
- **Agent plan**: **Runtime-owned with model-authored content**. Agent (human configurer) owns *whether* planning runs via `PlanningConfig` (`lib/crewai/src/crewai/agent/planning_config.py:11`); the **model owns *what* the plan says** (`plan`, `steps`, `ready` generated by LLM). Runtime (`AgentExecutorState.todos` in `lib/crewai/src/crewai/experimental/agent_executor.py:154`) owns the authoritative, mutable plan object and its lifecycle (mark_running/completed/failed/replace_pending_todos).
- **LLM role**: Not a separate "planner agent" service; planning agent is either a synthetic `Agent(role="Task Execution Planner")` for crew planning or the executing agent's own LLM (or `planning_config.llm` override) for agent planning (`lib/crewai/src/crewai/utilities/reasoning_handler.py:175-185`, `lib/crewai/src/crewai/agents/planner_observer.py:69-85`).

**3. Is planning required?**
No — **planning is fully optional and defaults to off** at both layers:
- `Crew.planning` defaults to `False` (`lib/crewai/src/crewai/crew.py:345`), and `prepare_kickoff` guards with `if crew.planning:` (`lib/crewai/src/crewai/crews/utils.py:376`).
- `Agent.planning` defaults to `False` and `planning_config` defaults to `None` (`lib/crewai/src/crewai/agent/core.py:277-284`); `planning_enabled` returns `self.planning_config is not None or self.planning` (`lib/crewai/src/crewai/agent/core.py:406-409`), and `AgentExecutor.generate_plan()` early-returns when false (`lib/crewai/src/crewai/experimental/agent_executor.py:357-358`). `AgentReasoning._get_planning_config` falls back to a default `PlanningConfig()` (`lib/crewai/src/crewai/utilities/reasoning_handler.py:160-173`) only when planning is enabled, so absence suppresses planning without side effects. All tests explicitly opt-in (`planning_config=PlanningConfig(...)` in `lib/crewai/tests/utilities/test_structured_planning.py:149`).

**4. Is planning visible?**
- **Runtime visibility: yes, via typed objects and events.** Plan is reified as `ReasoningPlan` (`lib/crewai/src/crewai/utilities/reasoning_handler.py:30`), `TodoList` (`lib/crewai/src/crewai/utilities/planning_types.py:45`), retained in `AgentExecutorState{plan, todos, observations, execution_log}` (`lib/crewai/src/crewai/experimental/agent_executor.py:135-171`). Every phase emits bus events: `AgentReasoningStarted/Completed/Failed` (`lib/crewai/src/crewai/utilities/reasoning_handler.py:197-240`), `StepObservation*` (`lib/crewai/src/crewai/agents/planner_observer.py:137-213`), `PlanStepStarted/Completed`, `PlanReplanTriggered/PlanRefinement/GoalAchievedEarly` (`lib/crewai/src/crewai/experimental/agent_executor.py:409-447`). `execution_log` holds audit trail (`lib/crewai/src/crewai/experimental/agent_executor.py:1143-1153`). Telemetry tags planning features (`lib/crewai/src/crewai/events/event_listener.py:709`).
- **User visibility: conditional.** `agent.verbose=True` drives `PRINTER.print` for `[Execute]`/`[Observe]`/`[Decide]` lines (`lib/crewai/src/crewai/experimental/agent_executor.py:692-782`). Without verbose or an event listener attached, the plan lives only in memory/state and is not persisted to a plan file or checkpoint `TodoList` serialization (checkpoint supports agent/crew restore but does not separately snapshot the plan as artifact).
- **Crew-plan visibility: weak.** Crew planner logs `Planning the crew execution` (`lib/crewai/src/crewai/crew.py:1453`) but plan is only observable as appended task description text, not as structured todo artifact.

**5. Is planning reusable?**
- **Across tasks: limited.** `PlanningConfig` is reusable as a Pydantic model — can be shared across agents via `crew_definition.py:120-122` YAML/JSON (`planning_config` field) and via Python `PlanningConfig(max_attempts=1)` in multiple `Agent()` constructors (`lib/crewai/tests/utilities/test_structured_planning.py:383`). Prompts (`system_prompt`, `plan_prompt`, `refine_prompt`) are templatized and swappable per config (`lib/crewai/src/crewai/agent/planning_config.py:110-121`).
- **Across executions: not cached/durable.** `TodoList.replace_pending_todos()` (`lib/crewai/src/crewai/utilities/planning_types.py:185-195`) supports mid-execution replanning (used by `AgentExecutor.handle_replan_now`), but there is no plan cache, registry, or serialization to reuse a generated plan for a new kickoff. Crew planner always regenerates per `kickoff()` (`lib/crewai/src/crewai/crews/utils.py:377`). `copy()` on Agent/Crew creates fresh executors (`lib/crewai/src/crewai/agent/core.py:1117-1144`, `lib/crewai/src/crewai/crew.py:2114`), so plans do not survive clone.
- **Across systems: no external planner.** No external planning service or workflow-graph planner exists; all planning is LLM-inline.

## Architectural Decisions

| Decision | Evidence | Consequence |
|----------|----------|-------------|
| **Dual-track planning (crew vs. agent) disconnected** | `CrewPlanner` + `AgentReasoning` are independent codepaths with different outputs (`PlannerTaskPydanticOutput` vs `ReasoningPlan/TodoList`) — `lib/crewai/src/crewai/utilities/planning_handler.py:37`, `lib/crewai/src/crewai/utilities/reasoning_handler.py:107` | Enables simple crew-level prompt augmentation without paying per-step observation cost; but means crew plan never becomes structured todos and agent plan ignores crew plan context |
| **Plan-and-Act decomposition via AgentExecutor Flow** | `AgentExecutor extends Flow[AgentExecutorState]` with `generate_plan → check_todos_available → execute → observe → decide` routers — `lib/crewai/src/crewai/experimental/agent_executor.py:173-191,348-771` | Gives planning a real runtime graph with dependency-aware scheduling (`get_ready_todos`, `can_parallelize`) and parallel step execution (`execute_todos_parallel`); ties planning lifecycle to Flow execution model |
| **Structured function-calling for plan generation** | `FUNCTION_SCHEMA` with `steps: {step_number, description, tool_to_use, depends_on}` + `llm.call(..., tools=[FUNCTION_SCHEMA], available_functions={create_reasoning_plan})` — `lib/crewai/src/crewai/utilities/reasoning_handler.py:50-104,368-385` | Makes plan machine-parseable; fallback to text parsing with `"READY:"` sentinel (`lib/crewai/src/crewai/utilities/reasoning_handler.py:583-598`) provides graceful degradation for non-function-calling LLMs |
| **Effort-graded observation pipeline** | `reasoning_effort: low (heuristic) / medium (replan on failure) / high (full refine/early-goal)` gated by `_should_observe_steps()` and `observe_step_result` routers — `lib/crewai/src/crewai/agents/planner_observer.py:88-111`, `lib/crewai/src/crewai/experimental/agent_executor.py:643-930` | Explicit latency/adaptivity tradeoff per `PlanningConfig.reasoning_effort` docs (`lib/crewai/src/crewai/agent/planning_config.py:22-30`); avoids mandatory LLM call per step |
| **Isolated StepExecutor per todo** | `StepExecutor._build_isolated_messages` + `execute()` fresh message list, `StepExecutionContext` only passes dependency results — `lib/crewai/src/crewai/agents/step_executor.py:244-326` | Prevents prompt bloat and cross-step contamination; limits step to `max_step_iterations`/`step_timeout` without polluting `AgentExecutor.messages` |
| **In-place refinement vs. full replan** | `StepObservation.suggested_refinements: list[StepRefinement]` applied via `PlannerObserver.apply_refinements` without second LLM call; `needs_full_replan` triggers `TodoList.replace_pending_todos()` — `lib/crewai/src/crewai/utilities/planning_types.py:198-210`, `lib/crewai/src/crewai/agents/planner_observer.py:215-242`, `lib/crewai/src/crewai/experimental/agent_executor.py:1011-1051` | Cheap plan adaptation for new info vs. expensive regeneration; `max_replans` cap (`lib/crewai/src/crewai/agent/planning_config.py:122-125`) bounds cost |
| **Crew plan as prompt mutation** | `task.description += plan_map[task_number]` — `lib/crewai/src/crewai/crew.py:1471-1472` | Zero new runtime objects; trivial but lossy — plan not queryable, not versioned, and accumulates on re-kickoff if caller reuses Crew without `copy()` |
| **Planning disables LLM observation by default for bare `planning=True`** | `if self.planning and self.planning_config is None: PlanningConfig(reasoning_effort="low", max_attempts=1)` — `lib/crewai/src/crewai/agent/core.py:396-402` | Prevents surprise per-step latency for users who merely flip a boolean; favors safety over adaptivity |

## Notable Patterns

- **Flow-as-planner-executor**: Planning is not a separate service but embedded as Flow graph nodes inside `AgentExecutor` — arguably the most explicit planning-location choice among harness studies.
- **Typed plan primitives everywhere**: `PlanStep` → `TodoItem` → `TodoList` → `StepObservation` chain is fully Pydantic-validated with `field_validator` coercions (e.g., single refinement dict → list, `lib/crewai/src/crewai/utilities/planning_types.py:259-265`).
- **Dependency-aware parallel ready set**: `TodoList.get_ready_todos()` + `can_parallelize` + `AgentExecutor.get_ready_todos_method` + `execute_todos_parallel` (`lib/crewai/src/crewai/utilities/planning_types.py:133-154`, `lib/crewai/src/crewai/experimental/agent_executor.py:1068-1102`) implements minimal DAG scheduler without importing Flow.
- **Effort as policy knob**: `reasoning_effort` is a single enum that cross-cuts observation, replan, and refinement behavior — unusual explicit operational tradeoff.
- **Heuristic degrade without LLM**: `PlannerObserver.heuristic_observation()` (`lib/crewai/src/crewai/agents/planner_observer.py:88`) returns `remaining_plan_still_valid=True` when `reasoning_effort="low"` or `observe_steps=False`, keeping execution moving with zero extra tokens.
- **Telemetry + verbose double-channel**: Every plan transition has both an event-bus event (for programmatic listeners) and an optional `PRINTER` console line gated on `agent.verbose`.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| **Opt-in planning** (`planning=False` default) | No surprise cost; backward compatible; explicit cost acknowledgment | Discoverability low — users must know `PlanningConfig()` enables the most tested path; doc examples may omit it |
| **Two planners, no unification** | Simple crew prompt planning for non-agentic tasks; rich todo planning for agentic deep tasks | Crew plan not convertible to `TodoList`; potential duplication if both enabled; no single `plan_id` to trace across layers |
| **Flow-graph planning vs. sequential Crew** | Planning gains DAG + parallel execution (`multiple_todos_ready`) | Adds indirection; `CrewAgentExecutor` legacy vs `AgentExecutor` split may confuse debugging |
| **`reasoning_effort` coarse knob** | One field controls observation/replan/early-goal spectrum | Users needing fine control (e.g., observe but never replan) must use `observe_steps` override; semantics not obvious without reading `AgentExecutor._should_observe_steps` |
| **String-append crew plan** | Minimal change surface; works with any downstream prompt | Lossy, untyped, not reversible, accumulates on repeated `prepare_kickoff` without copy; cannot feed `depends_on` |
| **Per-step LLM observation (high effort)** | Captures `goal_already_achieved` and subtle invalidations | Adds N extra LLM calls per task (N = number of steps); latency/cost scales with plan length |
| **Isolated step messages** | Clean context, predictable per-step prompt | Step loses conversational memory beyond dependency results; agent cannot reference off-dependency history without re-planning |

## Failure Modes / Edge Cases

- **Crew planning LLM failure raises ValueError**: `_handle_crew_planning` checks `isinstance(result.pydantic, PlannerTaskPydanticOutput)` else `raise ValueError("Failed to get the Planning output")` (`lib/crewai/src/crewai/utilities/planning_handler.py:75-78`). No retry; entire `kickoff()` fails. If `Crew.planning=True` with an overloaded `planning_llm`, the crew never executes.
- **Task re-kickoff accumulates plan text**: `task.description += plan` (`lib/crewai/src/crewai/crew.py:1472`) mutates the task in-place. Reusing the same `Crew` instance for multiple `kickoff()` calls without `copy()` appends a new plan each time, bloating prompts. By contrast AgentExecutor explicitly avoids this via comment `Do NOT mutate task.description — it's a shared object` (`lib/crewai/src/crewai/experimental/agent_executor.py:382-384`).
- **Duplicate task_number guard is lenient**: Duplicate plans for same `task_number` log a warning and keep first (`lib/crewai/src/crewai/crew.py:1460-1467`), silently dropping valid plans if LLM returns duplicates — no error surfaced.
- **Non-function-calling fallback degrades todos**: If `llm.supports_function_calling()` is false, `_create_initial_plan` returns `plan, [], ready` — zero steps (`lib/crewai/src/crewai/utilities/reasoning_handler.py:270-276`), so no `TodoList` is created and execution falls back to unplanned path without warning beyond log.
- **Observation parse failure defaults to success or conservative failure depending on path**: `PlannerObserver._parse_observation_response` falls back to `StepObservation(step_completed_successfully=False, remaining_plan_still_valid=False)` on unparseable LLM output (`lib/crewai/src/crewai/agents/planner_observer.py:348-352`), triggering replan; but `observe()` outer exception handler returns `success=True` (`lib/crewai/src/crewai/agents/planner_observer.py:208-213`) — inconsistent defaults risk masking real failures vs. spuriously replanning.
- **Replan budget exhaustion silently completes**: `handle_replan_now` checks `if replan_count >= max_replans: return "all_todos_complete"` (`lib/crewai/src/crewai/experimental/agent_executor.py:1019-1027`) — remaining pending todos are dropped without marking failed; caller sees partial results as if complete.
- **Dependency deadlock triggers replan**: `get_ready_todos_method` when `ready == [] and not is_complete` forces replan with `last_replan_reason = "No todos are ready..."` (`lib/crewai/src/crewai/experimental/agent_executor.py:1084-1095`). If LLM repeatedly produces unsatisfiable `depends_on` (e.g., cycle), this loops until `max_replans` then terminates incomplete.
- **Per-step timeout not wired to LLM cancellation**: `StepExecutor.execute(..., step_timeout)` checks elapsed time only between loop iterations (`lib/crewai/src/crewai/agents/step_executor.py:348-351, 558-565`); a hung `llm.call` blocks past the timeout until it returns, so `step_timeout` is advisory, not preemptive.
- **Expected tool enforcement can abort otherwise-successful steps**: `_validate_expected_tool_usage` raises `ValueError` if `todo.tool_to_use` not called (`lib/crewai/src/crewai/agents/step_executor.py:515-537`), converting a semantically correct text answer into a failed `StepResult`.

## Future Considerations

- Unify crew and agent planning layers behind a single `TodoList` registry with `plan_id` and artifact persistence, deprecating `task.description += plan` mutation.
- Persist `AgentExecutorState.todos`/`observations` into checkpoint `RuntimeState` (today only agent/crew memory views are rewired, `lib/crewai/src/crewai/crew.py:545-538`) so resume-from-checkpoint retains the plan rather than regenerating.
- Add plan caching keyed by `(task.description hash, tools hash, planning_config hash)` to avoid re-paying planning tokens for repeated identical tasks.
- Expose planner prompts as first-class `PlanningConfig` templates versioned alongside `FlowSkillReferenceExtractor` skill markdown (today prompts live in `en.json` and are opaque to users).
- Replace advisory `step_timeout` polling with LLM call timeout/cancellation and surface `needs_full_replan` vs parse-fallback ambiguity with distinct error codes.
- Consider an external/pluggable planner interface (e.g., `BasePlanner` ABC) so deterministic task decomposers can replace the LLM planner for structured domains.

## Questions / Gaps

- No evidence found for cross-crew plan sharing or a global plan store; search of `lib/crewai/src/crewai/utilities/planning*.py`, `lib/crewai/src/crewai/crew.py`, `lib/crewai/src/crewai/experimental/agent_executor.py` shows plan lives only in `AgentExecutorState` and crew-planner string append.
- No evidence of plan visualization distinct from Flow visualization (`lib/crewai/src/crewai/flow/visualization/*` covers Flow DAG, not TodoList).
- Unclear whether `Crew.planning` uses the same `PlanningConfig` knobs — search shows crew planning only accepts `planning_llm: str|BaseLLM`, not `PlanningConfig`; agent-level knobs (`max_steps`, `max_replans`) do not apply to crew planning.
- No dedicated load/stress test for large `TodoList` (e.g., `max_steps=20` with `depends_on` fan-out) found beyond `lib/crewai/tests/agents/test_agent_executor.py:939` unit tests.
- Relationship between deprecated `max_reasoning_attempts` (`lib/crewai/src/crewai/agent/core.py:290`) and new `max_attempts`/`max_replans`/`max_step_iterations` triad is implicit; no migration guide found in source.

---

Generated by `06.01-planning-location-and-responsibility` against `crewai`.
