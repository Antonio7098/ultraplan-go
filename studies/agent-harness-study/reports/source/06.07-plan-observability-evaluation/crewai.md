# Source Analysis: crewai

## Dimension 06.07: Plan Observability and Evaluation

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic, Flow/Crew, EventBus, OTel) |
| Analyzed | 2026-08-27 |

## Summary

CrewAI implements two distinct planning subsystems: **Crew-level planning** (`Crew.planning: bool` + `CrewPlanner` that fabricates a static natural-language plan per task) and **Agent-level Plan-and-Execute** (`Agent.planning_config: PlanningConfig` + `AgentReasoning` + `AgentExecutor` + `TodoList`/`TodoItem` + `StepExecutor` + `PlannerObserver`). Only the second is durably observable. Plans are generated via `AgentReasoning.handle_agent_reasoning()` (`src/crewai/utilities/reasoning_handler.py:187`) into `ReasoningPlan`/`PlanStep` (`src/crewai/utilities/reasoning_handler.py:30`, `src/crewai/utilities/planning_types.py:14`) and materialized as `TodoList`/`TodoItem` (`src/crewai/utilities/planning_types.py:27,45`). Execution links each tool call to `plan_step_number`/`plan_step_description` (`src/crewai/agents/step_executor.py:395`, `src/crewai/events/types/tool_usage_events.py:25`), and lifecycle events (`PlanStepStartedEvent`/`PlanStepCompletedEvent`, `StepObservation*Event`, `PlanReplanTriggeredEvent`, `GoalAchievedEarlyEvent`) are emitted on the bus and also captured in `AgentExecutorState.execution_log` (`src/crewai/experimental/agent_executor.py:168`) and `TraceCollectionListener` action traces (`src/crewai/events/listeners/tracing/trace_listener.py:509`). That enables plan-vs-execution debugging, but plan quality evaluation remains ad-hoc: `CrewEvaluator` scores only final task outputs 1-10 (`src/crewai/utilities/evaluators/crew_evaluator_handler.py:24`), the experimental `ReasoningEfficiencyEvaluator` measures LLM-call verbosity/loops not plan structure (`src/crewai/experimental/evaluation/metrics/reasoning_metrics.py:43`), and no eval dataset tests planning regressions - only unit tests of parsing/routing.

## Rating

**6 / 10 — Present but inconsistent, weakly documented, fragile for evaluation**

Observable execution is well-instrumented (typed plan/todo models, correlated events, execution_log, trace batch), and tests cover happy-path planning and observation parsing. Yet evaluation is bolted on: no plan-specific metrics (adherence, coverage, replan rate), no versioned planning eval dataset, and `CrewPlanner`'s static plan is invisible to the Plan-and-Execute trace. Diagnosing "was the plan bad vs the execution bad?" is possible but requires manually correlating two logs rather than a unified plan-vs-trace diff. Meets 7-8 on observability alone, pulled to 6 by the evaluation gap.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan traces — creation | `AgentReasoningStartedEvent` emitted with `task_id`/`attempt`, and `AgentReasoningCompletedEvent` with `plan: str` + `ready: bool` | `src/crewai/utilities/reasoning_handler.py:197-223` |
| Plan traces — creation | `ReasoningPlan` (`plan: str`, `steps: list[PlanStep]`, `ready: bool`) and `AgentReasoningOutput` wrappers; function-calling schema `FUNCTION_SCHEMA` for `create_reasoning_plan` | `src/crewai/utilities/reasoning_handler.py:30-104` |
| Plan traces — crew-level | `Crew.planning: bool` and `Crew.planning_llm`; `_handle_crew_planning()` delegates to `CrewPlanner._handle_crew_planning()` producing `PlannerTaskPydanticOutput` | `src/crewai/crew.py:344,1451-1456` |
| Plan traces — crew planner | `CrewPlanner` creates a planner `Agent` (`Task Execution Planner`) and a planner `Task` with `output_pydantic=PlannerTaskPydanticOutput` containing `list_of_plans_per_task` | `src/crewai/utilities/planning_handler.py:37-115` |
| Plan item IDs | `PlanStep.step_number: int (1-based)`, `description: str`, `tool_to_use: str|None`, `depends_on: list[int]` | `src/crewai/utilities/planning_types.py:14-24` |
| Plan item IDs | `TodoItem.id: str = uuid4()`, `step_number`, `status: TodoStatus`, `depends_on`, `result: str|None`; `TodoList` helpers `mark_running/completed/failed`, `get_ready_todos()`, `replace_pending_todos()` | `src/crewai/utilities/planning_types.py:27-195` |
| Plan item IDs | `TodoList` stored in `AgentExecutorState.todos: TodoList` alongside `plan: str`, `plan_ready: bool` | `src/crewai/experimental/agent_executor.py:155-156` |
| Execution links | `StepExecutor.execute(todo, context)` builds isolated messages and emits `ToolUsageStartedEvent`/`Finished`/`Error` carrying `plan_step_number=todo.step_number` and `plan_step_description=todo.description` | `src/crewai/agents/step_executor.py:127-148,387-453` |
| Execution links | Native tool path also propagates `plan_step_number/description` via `execute_single_native_tool_call(..., plan_step_number=todo.step_number)` | `src/crewai/agents/step_executor.py:609-623` |
| Execution links | `StepExecutionContext` bundles only final dependency `result: str` per `TodoItem.depends_on`, passed to `StepExecutor` — never full trace history | `src/crewai/experimental/agent_executor.py:647-678` |
| Execution links | `AgentExecutor` emits `PlanStepStartedEvent` on `mark_running` and `PlanStepCompletedEvent(success, result, error)` on `mark_completed/failed` | `src/crewai/experimental/agent_executor.py:446-522` |
| Observation trace | `PlannerObserver.observe()` emits `StepObservationStartedEvent` then LLM call with `response_model=StepObservation`, then `StepObservationCompletedEvent(step_completed_successfully, key_information_learned, remaining_plan_still_valid, needs_full_replan, goal_already_achieved, suggested_refinements)` | `src/crewai/agents/planner_observer.py:113-189` |
| Observation trace | `StepObservation` fields: `step_completed_successfully`, `key_information_learned`, `remaining_plan_still_valid`, `suggested_refinements: list[StepRefinement]`, `needs_full_replan`, `goal_already_achieved` | `src/crewai/utilities/planning_types.py:212-278` |
| Observation trace | `AgentExecutor.observe_step_result()` stores `state.observations[step_number]=observation` and appends audit entry `{type: observation, step_completed_successfully, key_information_learned, needs_full_replan, goal_already_achieved, reasoning_effort}` to `execution_log` | `src/crewai/experimental/agent_executor.py:680-727` |
| Execution log | `AgentExecutorState.execution_log: list[dict[str,Any]]` described as "Audit trail for debugging (NOT used for LLM calls)" — entries for `step_execution` and `observation` | `src/crewai/experimental/agent_executor.py:168-171` |
| Execution log | Sequential execution pushes `{type: step_execution, success, result_preview, error, tool_calls, execution_time}` per todo | `src/crewai/experimental/agent_executor.py:1180-1190` |
| Trace persistence | `TraceCollectionListener` registers handlers for `AgentReasoning*Event`, `StepObservation*Event`, `PlanRefinementEvent`, `PlanReplanTriggeredEvent`, `GoalAchievedEarlyEvent` as `_handle_action_event` batch entries | `src/crewai/events/listeners/tracing/trace_listener.py:509-557` |
| Trace persistence | `EventListener` maps reasoning/observation events to console formatter + `feature_usage_span("planning:*")` telemetry | `src/crewai/events/event_listener.py:699-777` |
| Planning config | `PlanningConfig.reasoning_effort: Literal[low,medium,high]`, `observe_steps: bool|None`, `max_attempts`, `max_steps:20`, `max_replans:3`, `max_step_iterations:15`, `step_timeout: int|None`, `llm` | `src/crewai/agent/planning_config.py:79-149` |
| Planning routing | `AgentExecutor._get_reasoning_effort()`, `_should_observe_steps()`, `observe_step_result()` router `step_observed_low/medium/high`, and downstream `handle_step_observed_low/medium` / `decide_next_action` / `handle_replan_now` etc. | `src/crewai/experimental/agent_executor.py:555-1088` |
| Eval harnesses — crew | `CrewEvaluator` installs `task.callback=evaluate`, runs evaluator `Agent` per output, emits `CrewTestResultEvent(quality, execution_duration, model)` | `src/crewai/utilities/evaluators/crew_evaluator_handler.py:53-215` |
| Eval harnesses — experimental | `BaseEvaluator.evaluate(agent, execution_trace, final_output, task) -> EvaluationScore(score 0-10, feedback)` | `src/crewai/experimental/evaluation/base_evaluator.py:52-69` |
| Eval harnesses — experimental | `MetricCategory` enum: `GOAL_ALIGNMENT`, `SEMANTIC_QUALITY`, `REASONING_EFFICIENCY`, `TOOL_SELECTION`, `PARAMETER_EXTRACTION`, `TOOL_INVOCATION` — no `PLAN_QUALITY` | `src/crewai/experimental/evaluation/base_evaluator.py:20-27` |
| Planning metrics — reasoning | `ReasoningEfficiencyEvaluator` evaluates focus/progression/decision_quality/conciseness/loop_avoidance via heuristics + LLM; loop detection via Jaccard n-gram similarity >0.7 | `src/crewai/experimental/evaluation/metrics/reasoning_metrics.py:43-259` |
| Regression tests | `test_agent_kickoff_with_planning_stores_plan_in_state`, `test_planning_creates_minimal_steps_for_multi_step_task`, etc., plus direct `StepObservation` parse tests (instance/JSON-string/dict/markdown-wrapped) | `src/crewai/tests/agents/test_agent_executor.py:1446,1569,2183-2268` |
| Regression tests | `PlanningConfig` defaults, `planning_enabled` property, `_parse_observation_response` robustness, `TodoList` result handling | `src/crewai/tests/agents/test_agent_reasoning.py:13-111`, `src/crewai/tests/agents/test_agent_executor.py:1982-2467` |

## Answers to Dimension Questions

1. **Are plans observable?** Yes, partially two-track. Agent-level plan text + structured `PlanStep[]` produced by `AgentReasoning` (`src/crewai/utilities/reasoning_handler.py:30-41`) are observable via `AgentReasoningStarted/Completed/FailedEvent` (`src/crewai/events/types/reasoning_events.py:24-48`) and console/trace listeners (`src/crewai/events/event_listener.py:699-719`, `src/crewai/events/listeners/tracing/trace_listener.py:509-525`). Runtime plan evolution is observable as `StepObservation*Event` and `PlanReplanTriggered/Refinement/GoalAchievedEarlyEvent` (`src/crewai/events/types/observation_events.py:46-131`). Crew-level static `CrewPlanner` output (`src/crewai/utilities/planning_handler.py:28-34`) has no comparable event stream — not routed through `TodoList` or trace action handlers beyond the underlying LLM calls.

2. **Can each action be linked to a plan item?** Yes for the Plan-and-Execute path. Every `TodoItem` carries `id: uuid4()` + `step_number` (`src/crewai/utilities/planning_types.py:30-31`). `StepExecutor._execute_text_tool_with_events` and `_execute_native_tool_calls` tag `ToolUsage*Event` with `plan_step_number`/`plan_step_description` (`src/crewai/agents/step_executor.py:395-449,621-622`; base event `src/crewai/events/types/tool_usage_events.py:25-26`). `PlanStepStarted/CompletedEvent` are also emitted per todo (`src/crewai/experimental/agent_executor.py:446-522`). ReAct-only steps without planning have no linkage.

3. **Are plans evaluated?** Weakly. `Crew.test(n_iterations, eval_llm)` loops final `TaskOutput.raw` through `CrewEvaluator` which scores 1-10 on "completion, quality, and overall performance" and emits `CrewTestResultEvent` (`src/crewai/utilities/evaluators/crew_evaluator_handler.py:22-27,199-215`; invoked from `src/crewai/crew.py:2253`). Experimental `AgentEvaluator.create_default_evaluator` covers 6 metrics including `REASONING_EFFICIENCY` (`src/crewai/experimental/evaluation/base_evaluator.py:20`) but none score plan structure, adherence, or plan-vs-execution fidelity. No in-repo eval dataset targets planning.

4. **Can poor planning be diagnosed?** Somewhat. A failed run can be reconstructed by correlating `state.plan` + `state.todos` (pending/running/completed/failed partitions, `replan_count`, `last_replan_reason` at `src/crewai/experimental/agent_executor.py:151-171`) + `execution_log` step entries (`src/crewai/experimental/agent_executor.py:1180`) + per-step `observations` (`src/crewai/utilities/planning_types.py:212`) + trace batch events (`src/crewai/events/listeners/tracing/trace_listener.py:509-557`). `PlannerObserver` failure falls back to `StepObservationCompletedEvent` with conservative defaults (`src/crewai/agents/planner_observer.py:191-213`). However diagnosis requires manual cross-reference; there is no plan-diff viewer, no per-plan adherence metric, and `CrewPlanner` output is detached from execution trace.

5. **Does planning improve success rate?** No evidence in-repo. No A/B harness comparing `planning_config=None` vs `PlanningConfig(...)`, no benchmark integrating plan quality with task success, and no tracking of replan/goal-achieved-early rates as success predictors. Tests assert plan *creation* and routing (`src/crewai/tests/agents/test_agent_executor.py:1446-1698`, `src/crewai/tests/agents/test_agent_reasoning.py:185-324`) but not improvement.

## Architectural Decisions

- **Opt-in bounded planning** — `Agent.planning_enabled` gates `PlanningConfig` (`src/crewai/agent/core.py:440-442`); bare `planning=True` is bounded (`PlanningConfig(max_attempts=...)` default at `src/crewai/agent/core.py:432`). Keeps default execution unaffected and makes planning an add-on, at cost of split code paths.
- **Structured plan steps with DAG deps** — `PlanStep.depends_on` + `TodoList.get_ready_todos()` + `execute_todos_parallel` (`src/crewai/utilities/planning_types.py:22,133`, `src/crewai/experimental/agent_executor.py:1257`) enables dependency-aware parallel step execution, unusual among agent frameworks, but demands correct LLM emission of `depends_on`.
- **Isolated StepExecutor** — `StepExecutor` owns its own message list; never reads/writes `AgentExecutor.state` directly, returns `StepResult` (`src/crewai/agents/step_executor.py:64-148`). Separation prevents cross-step leakage and enables clean `plan_step_number` attribution, yet hides full reasoning trajectory from the trace unless reconstructed.
- **PlannerObserver as post-step adjudicator** — Runs after *every* step, not just failures (`src/crewai/agents/planner_observer.py:39-48`), deciding `continue` vs `refine` vs `replan` vs `goal_achieved`. Three `reasoning_effort` tiers route differently (`src/crewai/experimental/agent_executor.py:681-968`). Adds adaptivity but extra LLM latency per step (especially `high`).
- **Event + trace dual telemetry** — Same `observation_events` feed both `EventListener` (console/OTel span at `src/crewai/events/event_listener.py:721-777`) and `TraceCollectionListener` batch (`src/crewai/events/listeners/tracing/trace_listener.py:527-557`). Enables live CLI visibility plus persisted replay; under-documented interaction between `should_enable_tracing()` and first-time ephemeral batch.

## Notable Patterns

- **Plan-to-Todo materialization** — `_create_todos_from_plan()` converts `PlanStep[]` to `TodoItem[]` one-for-one (`src/crewai/experimental/agent_executor.py:427-444`), preserving `step_number` as the stable join key across plan, execution, observation, and trace.
- **Refinement without replan** — `StepObservation.suggested_refinements: list[StepRefinement]` applied directly via `PlannerObserver.apply_refinements()` (`src/crewai/agents/planner_observer.py:215-242`) and emitted as `PlanRefinementEvent` (`src/crewai/experimental/agent_executor.py:994-1005`), avoiding a full regeneration LLM call.
- **Heuristic observe fallback** — When `observe_steps=False` or `reasoning_effort=low`, `PlannerObserver.heuristic_observation(step_success)` returns a no-LLM `StepObservation` (`src/crewai/agents/planner_observer.py:87-111`; gated at `src/crewai/experimental/agent_executor.py:590-645`), keeping the pipeline shape but degrading adaptivity.
- **Execution audit separation** — `execution_log` is explicitly "NOT used for LLM calls" (`src/crewai/experimental/agent_executor.py:169-171`); pure debug audit. Parallels `TraceEvent` lineage (`event_id`, `emission_sequence`, `parent_event_id` at `src/crewai/events/listeners/tracing/trace_listener.py:941-953`).

## Tradeoffs

- **Depth vs latency** — `reasoning_effort=high` (full routing: goal/replan/refine) vs `low` (heuristic, skip LLM observe). `medium` balances by replanning only on failure (`src/crewai/experimental/agent_executor.py:681-883`). Caller chooses cost; default `medium` still incurs one observation LLM call per step when `observe_steps` defaults to True (`src/crewai/agent/planning_config.py:89-97`).
- **Structured function-calling vs text parsing** — `AgentReasoning._call_with_function` falls back to text parsing when function calling unavailable (`src/crewai/utilities/reasoning_handler.py:256-276`); text path drops `steps` (returns `[]` at line 276), yielding a plan string without trackable items.
- **Two planning modes divergence** — `CrewPlanner` (static, whole-crew, no tracking) vs `Agent planning_config` (dynamic, per-agent, traced). Shipped together but unintegrated; users may assume crew plan feeds todo execution — it does not.
- **Eval generality vs plan specificity** — Evaluators score final outputs and tool behavior generically (`src/crewai/experimental/evaluation/base_evaluator.py:52-69`, `src/crewai/utilities/evaluators/crew_evaluator_handler.py:22-27`); no plan adherence/efficiency metric exists, so planning regressions are invisible to `crewai test`.

## Failure Modes / Edge Cases

- **Observation LLM parse failure silently masks** — `_parse_observation_response` handles instance/string/dict/markdown-wrapped cases but falls back to `StepObservation(success=False, remaining_plan_still_valid=False)` with a warning (`src/crewai/agents/planner_observer.py:304-352`); outside `PlannerObserver`, `AgentReasoning._call_with_function` fallback on JSON failure returns `ready=True` and empty steps to avoid stall (`src/crewai/utilities/reasoning_handler.py:440-444`), which can hide a malformed plan.
- **Dependency deadlock triggers replan, not error** — `get_ready_todos()= []` with non-complete plan routes to `needs_replan` with reason "likely a dependency deadlock" (`src/crewai/experimental/agent_executor.py:1119-1132`), then bounded by `max_replans` (`src/crewai/experimental/agent_executor.py:1056-1064`). Without the guard finalization would incorrectly succeed; with it, repeated bad `depends_on` emission exhausts replans and finalizes with incomplete plan.
- **Expected tool not called fast-fails step** — `_validate_expected_tool_usage` raises if `todo.tool_to_use` available but none of `tool_calls_made` matches (`src/crewai/agents/step_executor.py:515-537`), turning a soft hallucination into a hard step failure that `PlannerObserver` must adjudicate.
- **Checkpoint restore coherence risk** — `Crew._restore_runtime` rebuilds executor state but `AgentExecutorState.todos/observations/execution_log` are ephemeral Flow state; restoring mid-plan may re-run `generate_plan()` or duplicate todos if not deduplicated — no dedicated plan checkpoint test found.
- **Tracing gated on auth/ephemeral** — `TraceCollectionListener` batch finalization depends on `should_enable_tracing` / `is_first_time` / `is_tui_mode` (`src/crewai/events/listeners/tracing/trace_listener.py:212-221,302-337`). In unauthenticated non-first-time runs, planning events are emitted but never persisted, breaking post-hoc diagnosis.

## Future Considerations

- Unify Crew static plan and Agent dynamic todos: emit `CrewPlanner` output as `PlanPerTask` trace events and optionally seed `TodoList` to eliminate the current dual-track gap (`src/crewai/utilities/planning_handler.py:15-34` vs `src/crewai/utilities/planning_types.py:27`).
- Add plan-specific eval metrics (adherence ratio, intended-vs-executed tool match, replan rate, premature goal flag) as first-class `MetricCategory` values and integrate into `CrewEvaluator`/`AgentEvaluator` so planning regressions are caught by `crewai test`.
- Provide a plan-vs-execution diff artifact (compare `state.plan`/`todos` pre-run, `execution_log`, and trace events) rather than requiring manual correlation of `execution_log` + `observations` + trace batch.
- Version and ship a small planning eval dataset (multi-step tasks with known correct `depends_on` graphs) exercised in CI to bound LLM plan emission drift.

## Questions / Gaps

- No file establishes a correlation between plan quality and downstream success rate; searched `crew.py`, `experimental/agent_executor.py`, `utilities/evaluators/**`, `tests/**` — no comparative benchmark found.
- `No clear evidence found` for retention policy of `AgentExecutorState.execution_log` / `TraceBatchManager` batches: max size, truncation, or pruning logic not located in `experimental/agent_executor.py` or `events/listeners/tracing/**`.
- `No clear evidence found` for signaling plan staleness when user interrupts or revises inputs mid-execution; replanning is purely observer-driven, not externally triggerable.
- Unclear whether `CrewPlanner` output is persisted anywhere beyond the executing Task's LLM context; no `planning_handler` trace or storage beyond the Task return value (`src/crewai/utilities/planning_handler.py:73-78`).

---

Generated by `Dimension 06.07: Plan Observability and Evaluation` against `crewai`.
