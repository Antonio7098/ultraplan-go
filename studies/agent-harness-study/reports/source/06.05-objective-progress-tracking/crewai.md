# Source Analysis: crewai

## Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic models, event bus, Rich/TUI CLI, SQLite persistence, OTel telemetry) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI represents goals as natural-language strings (`Agent.goal`, `Task.description` + required `Task.expected_output`) rather than structured, machine-checkable goal objects. "Progress" is tracked through three cooperating mechanisms: (1) a typed event bus emitting lifecycle events for crews, agents, tasks, plan steps, guardrails, and tools; (2) a Pydantic state machine (`TodoList`) used by the experimental Plan-and-Execute agent executor to mark steps pending → running → completed/failed; and (3) token/usage metrics accumulated per-LLM, per-crew, and per-flow.

Completion criteria are heterogeneous: the sequential crew process treats "all tasks executed with non-empty raw output" as success (no independent verification of `expected_output`); the ReAct loop terminates on a parsed `AgentFinish`; the Plan-and-Execute loop terminates when all todos reach a terminal state or an observer LLM declares `goal_already_achieved`. Output correctness is enforced only when the user attaches guardrails (bounded retries), structured-output models, or runs the opt-in `Crew.test()` evaluation loop, which itself relies on LLM-as-judge scoring. The framework is unusually honest about this gap — `CrewOutput.tool_failures` explicitly documents that a crew can "finish successfully" while steps failed underneath it, and provides declarative `ToolFailure` signalling so failures are at least recorded rather than misread as success. Progress is observable end-to-end via events consumed by a trace listener, OTel telemetry, and a CLI TUI showing per-task/per-step status and live token counts.

## Rating

**7 / 10** — Clear progress model with explicit typed interfaces (event types, todo state machine, usage metrics), bounded retry/replan loops, and test coverage (`tests/task/test_task_guardrails.py`, `tests/utilities/test_planning_types.py`, `tests/test_flow_usage_metrics.py`). It loses points because final success is not independently checked by default: `expected_output` is prompt text never machine-compared against output, evaluation is opt-in and LLM-based (self-grading risk), and several failure paths default optimistically to "continue/success".

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal representation (agent) | `goal: str = Field(description="Objective of the agent")` | lib/crewai/src/crewai/agents/agent_builder/base_agent.py:275 |
| Goal representation (task) | `expected_output` is a **required** string field; validator raises if missing | lib/crewai/src/crewai/task.py:153-155, task.py:384-387 |
| Task identity | Task `key` = md5 of description+expected_output | lib/crewai/src/crewai/task.py:596-601 |
| Expected output carried into result | `TaskOutput(expected_output=..., raw=...)` pairs goal with outcome | lib/crewai/src/crewai/task.py:720-731; lib/crewai/src/crewai/tasks/task_output.py:32-36 |
| Structured output criteria | `output_json` / `output_pydantic` / `response_model` enforce schema'd completion | lib/crewai/src/crewai/task.py:175-204 |
| Guardrails as completion checks | Guardrail validated + retried up to `guardrail_max_retries` (default 3), then hard exception | lib/crewai/src/crewai/task.py:279-281, task.py:1321-1439 (esp. 1376-1385) |
| Guardrail observability | `LLMGuardrailStartedEvent` / `LLMGuardrailCompletedEvent(success, error, retry_count)` emitted per check | lib/crewai/src/crewai/utilities/guardrail.py:162-185 |
| Hallucination guardrail | Optional LLM-judged grounding check on task output | lib/crewai/src/crewai/tasks/hallucination_guardrail.py:20-84 |
| Plan/goal decomposition | `generate_plan()` creates `TodoItem`s from planner output | lib/crewai/src/crewai/experimental/agent_executor.py:348-407 |
| Progress state machine | `TodoStatus = pending\|running\|completed\|failed`; `mark_running/completed/failed`; `is_complete` = all terminal | lib/crewai/src/crewai/utilities/planning_types.py:11, 90-110, 66-71 |
| Progress counters | `pending_count` / `completed_count` properties | lib/crewai/src/crewai/utilities/planning_types.py:73-81 |
| Executor progress state | `AgentExecutorState`: todos, replan_count, observations, execution_log audit trail | lib/crewai/src/crewai/experimental/agent_executor.py:135-170 |
| Step-level success judgment | `PlannerObserver.observe()` asks LLM for `StepObservation(step_completed_successfully, needs_full_replan, goal_already_achieved)` after every step | lib/crewai/src/crewai/agents/planner_observer.py:113-213 |
| Heuristic (non-LLM) observation | `heuristic_observation()` derives success from execution metadata only | lib/crewai/src/crewai/agents/planner_observer.py:87-111 |
| Effort-tiered routing | low/medium/high reasoning_effort routes decide replan/refine/goal-achieved | lib/crewai/src/crewai/experimental/agent_executor.py:644-930 |
| Early goal detection | `GoalAchievedEarlyEvent(steps_completed, steps_remaining)` skips remaining todos | lib/crewai/src/crewai/events/types/observation_events.py:123-131; agent_executor.py:873-883, 985-1009 |
| Replan bound | `handle_replan_now` stops replanning after `_get_max_replans()` | lib/crewai/src/crewai/experimental/agent_executor.py:1011-1027 |
| ReAct completion criterion | Loop ends only on parsed `AgentFinish`; `max_iter=25` bound | lib/crewai/src/crewai/experimental/agent_executor.py:197, 2865-2870 |
| Crew completion criterion | `_create_crew_output` takes last non-empty `raw` — no expected_output comparison | lib/crewai/src/crewai/crew.py:1919-1926 |
| Honest success caveat | `CrewOutput.tool_failures`: "A crew can finish successfully with a non-empty list -- agents narrate a failed step and carry on" | lib/crewai/src/crewai/crews/crew_output.py:35-47 |
| Declarative tool-failure signalling | `ToolFailure`/`ToolFailureRecord` so a tool that ran-but-failed is never read as success; policies ignore/warn/raise | lib/crewai/src/crewai/tools/tool_failure.py:1-11, 35-68, 111-115 |
| Failure accumulation across retries | Tool failures merged onto final output after guardrail retries | lib/crewai/src/crewai/task.py:1339-1372 |
| Task lifecycle events | `TaskStartedEvent` / `TaskCompletedEvent(output)` / `TaskFailedEvent(error)` emitted around execution | lib/crewai/src/crewai/events/types/task_events.py:24-57; task.py:675-677, 793-800 |
| Crew lifecycle events | `CrewKickoffStarted/Completed(total_tokens)/Failed` | lib/crewai/src/crewai/events/types/crew_events.py:36-55; crew.py:1068-1077, 1964-1972 |
| Plan-step lifecycle events | `PlanStepStartedEvent` / `PlanStepCompletedEvent(success, error)` from todo transitions | lib/crewai/src/crewai/events/types/observation_events.py:46-58; agent_executor.py:409-485 |
| Usage metrics model | `UsageMetrics` (total/prompt/cached/completion/reasoning tokens, successful_requests) with provider normalization | lib/crewai/src/crewai/types/usage_metrics.py:32-63, 142-189 |
| Per-LLM accounting | `_track_token_usage_internal` / `get_token_usage_summary` accumulators | lib/crewai/src/crewai/llms/base_llm.py:955-986 |
| Crew aggregation | `calculate_usage_metrics()` sums over agents + manager | lib/crewai/src/crewai/crew.py:2201-2224 |
| Flow aggregation | Flow listens to `LLMCallCompletedEvent` into thread-safe accumulator | lib/crewai/src/crewai/flow/runtime/__init__.py:757-758, 890-914 |
| Timing metrics | `start_time`/`end_time` fields and `execution_duration` property | lib/crewai/src/crewai/task.py:290-295, 603-607 |
| Execution log (blockers) | Per-task log of outputs stored by index (`_store_execution_log`) | lib/crewai/src/crewai/crew.py:1479-1507 |
| Evaluation loop (opt-in) | `Crew.test(n_iterations, eval_llm)` re-runs crew and scores each task 1-10 | lib/crewai/src/crewai/crew.py:2227-2272; lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:69-85, 177-222 |
| Task evaluator scoring | `TaskEvaluation.quality` 0-10 judged from description+expected_output vs actual output | lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:28-49, 86-113 |
| Eval framework categories | `MetricCategory.GOAL_ALIGNMENT` etc.; `EvaluationScore` 0-10 with feedback | lib/crewai/src/crewai/experimental/evaluation/base_evaluator.py:20-49 |
| Test result events | `CrewTestResultEvent(quality, execution_duration, model)` | lib/crewai/src/crewai/events/types/crew_events.py:104-110 |
| Trace listener | `TraceCollectionListener` registers handlers for crew/task/agent/tool/guardrail/memory/reasoning events | lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:140-520 |
| UI status (CLI TUI) | Per-task status dict ("pending"/…), plan-step statuses, live token counters | lib/cli/src/crewai_cli/crew_run_tui.py:507-581 |
| UI event wiring | TUI subscribes to Task/PlanStep/Kickoff events to update statuses | lib/cli/src/crewai_cli/crew_run_tui.py:2046-2316, 2728-2828 |
| Human approval gate (task) | `human_input` flag; executor routes through `_handle_human_feedback` | lib/crewai/src/crewai/task.py:233-236; agent_executor.py:2858-2873 |
| Human approval gate (flow) | HITL pause/resume with `HumanFeedbackRequestedEvent`/`ReceivedEvent`, persisted feedback | lib/crewai/src/crewai/events/types/flow_events.py:244-268; lib/crewai/src/crewai/flow/persistence/sqlite.py:205-280 |
| Checkpoint/resume progress | Flow checkpoints record `checkpoint_completed_methods` and restore them | lib/crewai/src/crewai/flow/runtime/__init__.py:682-703 |
| Telemetry spans | OTel `task_started` / `task_ended` spans | lib/crewai/src/crewai/telemetry/telemetry.py:513-610 |

## Answers to Dimension Questions

### 1. What is the goal?
Free-text, three levels: `Agent.goal` (lib/crewai/src/crewai/agents/agent_builder/base_agent.py:275) injected into system prompts; per-unit `Task.description` + mandatory `Task.expected_output` (lib/crewai/src/crewai/task.py:152-155, validated at task.py:378-388). In Plan-and-Execute mode the goal is decomposed into `PlanStep`s → `TodoItem`s (lib/crewai/src/crewai/experimental/agent_executor.py:348-407). There is no structured, machine-checkable goal object; `expected_output` is interpolated into prompts (lib/crewai/src/crewai/task.py:1092-1101) and echoed onto `TaskOutput.expected_output` (lib/crewai/src/crewai/tasks/task_output.py:32-34) but never programmatically compared to `raw`.

### 2. How is progress measured?
A mix of all four mechanisms, layered:
- **Tool success**: declarative `ToolFailure` returns plus `detect_tool_failure` (lib/crewai/src/crewai/tools/tool_failure.py:8-10, 154+) feed `TaskOutput.tool_failures` under WARN/RAISE policies.
- **Model judgement**: `PlannerObserver.observe()` classifies every executed step's success and whether the remaining plan stays valid (lib/crewai/src/crewai/agents/planner_observer.py:113-213), tiered by `reasoning_effort` (lib/crewai/src/crewai/experimental/agent_executor.py:644-708).
- **Tests**: opt-in `Crew.test()` runs n iterations and scores tasks 1-10 with an evaluator LLM (lib/crewai/src/crewai/crew.py:2227-2272).
- **User approval**: `human_input` task flag and flow-level HITL pause/resume with checkpoints (lib/crewai/src/crewai/task.py:233-236; lib/crewai/src/crewai/flow/persistence/sqlite.py:205-280).
Token usage and wall-clock duration are measured as activity proxies, not goal proximity (lib/crewai/src/crewai/types/usage_metrics.py:32-63; lib/crewai/src/crewai/task.py:603-607).

### 3. Can the model fake progress?
Partially yes. The default crew path accepts any non-empty `raw` string as the final answer (lib/crewai/src/crewai/crew.py:1919-1926); nothing verifies `expected_output` unless a guardrail, structured output model, or evaluation run is configured. The evaluators themselves are LLMs, so a persuasive wrong answer can score well. The framework explicitly acknowledges this: `CrewOutput.tool_failures` warns that "a crew can finish successfully with a non-empty list — agents narrate a failed step and carry on" (lib/crewai/src/crewai/crews/crew_output.py:36-42). Mitigations that raise the cost of faking: native structured outputs (lib/crewai/src/crewai/task.py:175-204), hallucination guardrail (lib/crewai/src/crewai/tasks/hallucination_guardrail.py:20-84), and declarative `ToolFailure` that prevents string-parsed optimism (lib/crewai/src/crewai/tools/tool_failure.py:154-160). Additionally, the observer defaults optimistically: if its own LLM call throws, it logs "Defaulting to conservative replan" but actually returns `step_completed_successfully=True, needs_full_replan=False` (lib/crewai/src/crewai/agents/planner_observer.py:191-213) — a docstring/behavior mismatch worth flagging.

### 4. Are blockers recorded?
Yes, in several places: `TaskFailedEvent(error)` on exception (lib/crewai/src/crewai/task.py:798-801); failed todos retain status `"failed"` plus result/error and are queryable via `get_failed_todos()` (lib/crewai/src/crewai/utilities/planning_types.py:104-110, 169); guardrail validation errors become the retry context fed back to the agent and are raised verbatim after exhausting retries (lib/crewai/src/crewai/task.py:1394-1408, 1376-1385); `CrewKickoffFailedEvent` carries the top-level error (lib/crewai/src/crewai/crew.py:1068-1078); and the executor keeps an `execution_log` audit trail including observations and replan reasons (lib/crewai/src/crewai/experimental/agent_executor.py:167-170, 678-690). Failed dependencies do not permanently block downstream todos — dependency satisfaction accepts terminal states and leaves skip/replan decisions to the observer (lib/crewai/src/crewai/utilities/planning_types.py:112-120).

### 5. Is final success independently checked?
Only optionally. Built-in independent-ish checks: guardrails (callable or LLM-described, retried up to 3 times then fail-hard — lib/crewai/src/crewai/task.py:1376-1385), pydantic/json output schemas, and `HallucinationGuardrail` grounding checks. The dedicated mechanism is `Crew.test()`, which re-executes the crew n times and scores each task with a separate evaluator agent (lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:69-85, 177-222) — but the judge is still an LLM and typically shares provider/model lineage with the workers. In the plain `kickoff()` path there is no post-hoc verification: success = "all tasks returned non-empty raw" (lib/crewai/src/crewai/crew.py:1919-1926). Consumers are told to check `has_tool_failures` before trusting `raw` (lib/crewai/src/crewai/crews/crew_output.py:44-47), which is documentation of the gap rather than closure of it.

## Architectural Decisions

1. **Goals as prompt strings, not objects.** `expected_output` is a required field but lives its life inside prompts (interpolated at lib/crewai/src/crewai/task.py:1092-1101). Verification is delegated to user-supplied guardrails instead of a built-in goal checker. This keeps the core simple and model-agnostic but makes unverified-by-default success the norm.
2. **Typed event bus as the single progress channel.** All lifecycle signals (task started/completed/failed, plan steps, observations, replans, early-goal, checkpoints, human feedback) are Pydantic event classes on a singleton bus (lib/crewai/src/crewai/events/event_bus.py:95+; type catalog under lib/crewai/src/crewai/events/types/). UI, tracing, telemetry, and metrics all subscribe rather than poll — progress representation is decoupled from progress display.
3. **Explicit todo state machine for plan-and-execute.** Status transitions are centralized in `TodoList.mark_*` (lib/crewai/src/crewai/utilities/planning_types.py:90-110) and every transition emits an authoritative `PlanStep*Event` (lib/crewai/src/crewai/experimental/agent_executor.py:451-485), tested in tests/agents/test_agent_executor.py:894, 935.
4. **Effort-tiered adaptivity.** The same observation point routes cheaply (heuristic, no LLM) at `reasoning_effort="low"` or fully (replan/refine/goal-achieved pipeline) at `"high"` (lib/crewai/src/crewai/experimental/agent_executor.py:644-708), acknowledging that progress checking has a latency/token price.
5. **Declarative failure signalling over string sniffing.** Tools return `ToolFailure`; detection is "strictly declarative — nothing here guesses whether a string 'looks like' an error" (lib/crewai/src/crewai/tools/tool_failure.py:8-10). Failures ride alongside output instead of being inferred.
6. **Bounded self-correction loops.** Guardrail retries capped by `guardrail_max_retries` (lib/crewai/src/crewai/task.py:279-281), replans capped by `_get_max_replans()` (agent_executor.py:1019-1027), ReAct iterations capped at `max_iter=25` (agent_executor.py:197) — progress machinery cannot spin forever.

## Notable Patterns

- **Goal-proximity routing**: `decide_next_action` maps one observation to four futures — `goal_achieved` / `replan_now` / `refine_and_continue` / `continue_plan` — making "are we closer?" an explicit branch, not an emergent property (lib/crewai/src/crewai/experimental/agent_executor.py:848-930).
- **Early termination on achieved goals**: `GoalAchievedEarlyEvent` with remaining/completed counts lets the agent stop doing planned work once the objective is met (observation_events.py:123-131).
- **Anti-narration prompts**: planning i18n prompts instruct the observer to mark `step_completed_successfully=false` when "the result is only exploratory (ls, pwd, cat) without producing the required artifact" (lib/crewai/src/crewai/translations/en.json:88), and step executors must verify outcomes before Final Answer (en.json:90, 96). This directly targets the activity-vs-progress distinction.
- **Metrics normalization parity**: `UsageMetrics.from_provider_dict` mirrors `BaseLLM._track_token_usage_internal` so per-LLM totals, flow aggregation, and OTel spans agree per provider (lib/crewai/src/crewai/types/usage_metrics.py:142-155).
- **Honesty properties in result models**: `CrewOutput.tool_failures` / `has_tool_failures` exist specifically so callers can detect "success-shaped" outputs produced over failed steps (lib/crewai/src/crewai/crews/crew_output.py:35-47).
- **Checkpointable progress**: flow progress (`_completed_methods`) is serialized into checkpoints and restored, enabling resume/fork mid-plan (lib/crewai/src/crewai/flow/runtime/__init__.py:682-703; tests at lib/crewai/tests/test_flow_persistence.py:65-98).

## Tradeoffs

- **Simplicity vs verifiability**: string goals keep authoring trivial (README-style DX), but push all real verification onto optional guardrails; the default path trusts the model's Final Answer format contract ("Final Answer:" parsing, lib/crewai/src/crewai/translations/en.json:12).
- **LLM-judged progress costs vs blind continuation**: LLM observation per step doubles calls at high effort; the low-effort heuristic path avoids this but can't detect subtle step failures (planner_observer.py:87-111). Even the low path still gates hard failures into replans (agent_executor.py:710-751).
- **Optimistic failure defaults**: when the observer's LLM call fails, the system continues the plan with `step_completed_successfully=True` despite logging "conservative replan" (planner_observer.py:191-213) — favoring liveness over accuracy; conversely, unparseable responses fall back pessimistically to failure (planner_observer.py:342-352). The two fallbacks disagree in direction.
- **Self-grading evaluations**: `Crew.test()` quality scores come from an LLM evaluator agent whose llm may be the same family as the workers' (crew_evaluator_handler.py:58-67); scores are comparable across runs but not ground truth.
- **Warn-default tool failures**: `ToolFailurePolicy.WARN` records and emits but keeps going (tool_failure.py:57-68), preserving throughput at the cost of possibly compounding broken intermediate results downstream.

## Failure Modes / Edge Cases

- **Success theater**: final crew output taken as last non-empty `raw` regardless of `expected_output` fidelity or failed sub-steps (lib/crewai/src/crewai/crew.py:1919-1926; caveat documented at crews/crew_output.py:36-42).
- **Observer outage silently green-lights steps**: exception in `observe()` yields a success observation (planner_observer.py:208-213), so infrastructure flakiness masquerades as progress.
- **Parse-failure pessimism asymmetry**: unparseable observation JSON marks the step failed and plan invalid (planner_observer.py:342-352), potentially triggering unnecessary replans that consume the `max_replans` budget (agent_executor.py:1019-1027).
- **Evaluator mismatch exceptions**: `CrewEvaluator.evaluate` raises if task matching by description fails or pydantic output shape is unexpected (crew_evaluator_handler.py:183-222) — evaluation itself can crash a test run mid-way (caught by `Crew.test` and surfaced as `CrewTestFailedEvent`, crew.py:2267-2272).
- **Guardrail None-result**: a guardrail returning `(True, None)` raises immediately (task.py:1353-1357) — malformed validators fail loudly rather than passing vacuously.
- **Async task ordering**: async task outputs are collected at synchronization points; `_store_execution_log` keys logs by index with a `was_replayed` flag to keep replayed runs consistent (crew.py:1479-1507, 1582-1622).
- **Duplicate plans during planning**: `_handle_crew_planning` warns and uses the first plan when the planner emits duplicate task numbers (crew.py:1458-1465) — plan drift is logged, not fatal.

## Future Considerations

- Add a first-class, machine-checkable success predicate on `Task` (e.g., optional validator callable or schema diff between `expected_output` and output) evaluated in `_create_crew_output` rather than relying on user-installed guardrails.
- Make the observer's failure default configurable and consistent: today an observer LLM outage yields optimistic continuation (planner_observer.py:208-213) while an unparseable response yields pessimistic failure (planner_observer.py:342-352); unify under one explicit policy enum.
- Surface `has_tool_failures` in `CrewKickoffCompletedEvent` so event-driven consumers get the honesty signal without inspecting the output object.
- Persist `execution_log`/observations beyond executor lifetime (currently reset per invocation, agent_executor.py:2840-2845) to support post-run blocker analysis.
- Allow pinning an evaluation judge distinct from worker LLM providers in `Crew.test()` to reduce correlated grading bias.

## Questions / Gaps

- No evidence found of any built-in comparison of `raw` output against `expected_output` outside of LLM-based evaluators; searches across `lib/crewai/src/crewai` for programmatic expected/output diffing returned only prompt-interpolation and evaluator-query usages.
- The hierarchical process delegates task assignment to a manager agent, but no evidence was found of manager-specific progress tracking beyond shared task events (searched `crew.py` `_run_hierarchical_process`, delegation tools).
- `PlannerObserver` log message says "Defaulting to conservative replan" while returning no-replan (planner_observer.py:191-213); unclear whether this is an intentional naming choice or stale message — behavior confirmed from code, intent not documented elsewhere.
- The `crewai test` CLI path exists via project templates calling `crew().test(...)` (lib/cli/src/crewai_cli/templates/crew/main.py:61), but a dedicated CLI test command implementation was not located inside `lib/cli/src` within this source snapshot; the TUI covers `run` flows only (lib/cli/src/crewai_cli/run_crew.py:30).

---

Generated by `06.05-objective-and-progress-tracking` against `crewai`.
