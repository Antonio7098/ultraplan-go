# Source Analysis: crewai

## 03.05 Reflection, ReAsk, and Self-Correction Loops

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (3.10+), Pydantic v2, LiteLLM / instructor, FastAPI-style event bus |
| Analyzed | 2026-07-14 |

## Summary

CrewAI ships **three distinct, well-bounded self-correction loops** rather than a single reflection module:

1. **Guardrail reask loop** for *task/agent output validation* — `process_guardrail` (utilities/guardrail.py:123-187) returns a typed `GuardrailResult`, callers (Task at `task.py:1246-1353` and `task.py:1355-1463`, LiteAgent at `lite_agent.py:696-732`, Agent core at `agent/core.py:1823-1892`) wrap the re-execution in a hard `for attempt in range(max_attempts)` / recursive retry bounded by `guardrail_max_retries` (default 3, task.py:273-274 and lite_agent.py:262-263). Failed runs raise a descriptive `Exception`; passed runs can transform `output.raw` / `output.pydantic` via the guardrail's return tuple — the system never silently hides failure.
2. **Structured-output repair loop** for *Pydantic/JSON conversion* — `Converter.to_pydantic` (utilities/converter.py:84-116) and `Converter.to_json` (utilities/converter.py:142-178) recurse with `current_attempt + 1` up to `max_attempts` (default 3, base_output_converter.py:31-34). On exhaustion, `ConverterError` propagates. `handle_partial_json` (utilities/converter.py:280-332) provides a *separate* JSON repair layer that runs *before* recursion is needed.
3. **Plan-and-Act replan loop** for *plan-level recovery* — `AgentExecutor` (experimental/agent_executor.py:148-153, 497-509, 676-895, 2452-2632) maintains a `replan_count`, capped by `PlanningConfig.max_replans` (default 3, agent/planning_config.py:122-126). Per-step `PlannerObserver` (agents/planner_observer.py:113-189) issues a structured `StepObservation` (`utilities/planning_types.py:212-278`) that drives four routing outcomes — `goal_achieved`, `replan_now`, `refine_and_continue`, `continue_plan` (agent_executor.py:813-895). Replan re-invokes `AgentReasoning` with a `_build_replan_context` payload (agent_executor.py:2591-2651).

Auxiliary recovery paths handle **maximum-iteration termination** (`handle_max_iterations_exceeded` in utilities/agent_utils.py:293-329 — appends a "force final answer" prompt, i18n at translations/en.json:46-47), **context-window overflow** (`handle_context_length`, utilities/agent_utils.py:712-749 — summarise or `SystemExit`), **parser errors** (`handle_output_parser_exception`, utilities/agent_utils.py:660-695 — appends the parser error as a user message), and **execution exceptions** (`_check_execution_error` / `_handle_execution_error` in agent/core.py:685-759 — recurses up to `max_retry_limit`=2).

Every retry supplies concrete feedback to the LLM: the guardrail reask prompt (translations/en.json:55) embeds the previous task output + the validation error verbatim (`task.py:1310-1313`); the LiteAgent path appends the error as a `user` message (`lite_agent.py:718-723`); the replan path injects a structured summary of completed/failed steps + the previous reason (agent_executor.py:2591-2632 + translations/en.json:100). The reflection LLM sees real evidence, not a "try again" stub.

Loops are bounded: `max_attempts`, `max_retries`, `guardrail_max_retries`, `max_replans`, `max_iter`, and `max_retry_limit` all default to finite integers. Exhaustion routes to either an `Exception`/`ValueError`/`SystemExit` or a hard "force final answer" LLM call that exits the loop. Replan-only observability is wired into the event bus (`PlanReplanTriggeredEvent`, `StepObservationCompletedEvent` in events/types/observation_events.py:61-131) and into the rich console (`handle_guardrail_started/completed` in events/utils/console_formatter.py:1283-1339).

Hallucination guardrail is an explicit **no-op in open-source** (`tasks/hallucination_guardrail.py:73-77, 95-103`) — a guardrail-pattern surface exists but no LLM-judge correction is implemented.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:
- Multiple independent retry/reask loops with explicit per-path bounds (3 retry budgets: `guardrail_max_retries`, `max_attempts`, `max_replans`; plus 3 hard caps: `max_iter`, `max_retry_limit`, `step_timeout`).
- All four primary loops surface typed errors and rich evidence to the next LLM call; none silently swallow failure (`task.py:1310-1313`, `lite_agent.py:718-723`, `agent_executor.py:2591-2651`, `converter.py:106-117`).
- Dedicated test coverage exists for guardrail retry-budgets (`tests/test_task_guardrails.py:71-122, 458-503`), converter retry (`tests/utilities/test_converter.py:490-514`), and replan routing (`tests/agents/test_agent_executor.py:1579-2116`).
- Event-bus hooks (`LLMGuardrailStartedEvent`, `LLMGuardrailCompletedEvent`, `StepObservationCompletedEvent`, `PlanReplanTriggeredEvent`) make reflection observable.
- Limits: no LLM-as-judge self-critique pipeline that scores the final answer against the goal before returning to the user (the open-source `HallucinationGuardrail` is a no-op, `tasks/hallucination_guardrail.py:73-77`); no global reflection driver at the Agent level — guards are output-side only; LiteAgent guardrail retry recurses through `_execute_core` (`lite_agent.py:726`) which risks message-list growth with no per-attempt token-trimming; LiteAgent guardrail path appends the error string with no per-attempt compaction of earlier errors (`lite_agent.py:718-724`). These prevent a 9-10 score.

## Evidence Collected

Every entry includes file path and line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Guardrail entry-point function | `process_guardrail` emits Started/Completed events and returns a typed `GuardrailResult` | `lib/crewai/src/crewai/utilities/guardrail.py:123-187` |
| Guardrail result model | `GuardrailResult` with mutual-exclusion validator on `result`/`error` | `lib/crewai/src/crewai/utilities/guardrail.py:60-120` |
| Guardrail callable type alias | `GuardrailCallable` accepts callable returning `(bool, Any)` | `lib/crewai/src/crewai/utilities/guardrail_types.py:12-18` |
| LLMGuardrail implementation | Uses a sub-Agent to validate and returns feedback string | `lib/crewai/src/crewai/tasks/llm_guardrail.py:49-119` |
| LLMGuardrail prompt template | Iterative reask prompt embeds prior task output + error | `lib/crewai/src/crewai/translations/en.json:55` |
| Hallucination guardrail (no-op) | Open-source implementation is a no-op that warns and returns success | `lib/crewai/src/crewai/tasks/hallucination_guardrail.py:73-103` |
| Task-level guardrail retry (sync) | `for attempt in range(max_attempts)` re-invokes `agent.execute_task`; final failure raises | `lib/crewai/src/crewai/task.py:1246-1353` |
| Task-level guardrail retry (async) | Same shape, awaits `aexecute_task` | `lib/crewai/src/crewai/task.py:1355-1463` |
| Task retry budget field | `guardrail_max_retries` default 3, `retry_count` defaults to 0 | `lib/crewai/src/crewai/task.py:271-291` |
| Task guardrail context message | Builds `validation_error` prompt with last error + raw task output | `lib/crewai/src/crewai/task.py:1310-1313` |
| Task multiple guardrails retry bookkeeping | `_guardrail_retry_counts: dict[int, int]` | `lib/crewai/src/crewai/task.py:288-291, 1257-1260, 1303-1308, 1367-1370, 1413-1418` |
| LiteAgent guardrail retry path | Recursive call into `_execute_core` after appending error as `user` message | `lib/crewai/src/crewai/lite_agent.py:696-732` |
| LiteAgent retry budget | `_guardrail_retry_count` private attr + `guardrail_max_retries=3` | `lib/crewai/src/crewai/lite_agent.py:262-264, 292-293` |
| Agent core guardrail retry (recursive) | `_process_kickoff_guardrail` recurses with `retry_count + 1` | `lib/crewai/src/crewai/agent/core.py:1823-1892` |
| Agent core retry budget | `guardrail_max_retries: int = 3` | `lib/crewai/src/crewai/agent/core.py:318-320` |
| Agent core execution-error retry | `_check_execution_error` re-raises past `_passthrough_exceptions` and `litellm`; `_handle_execution_error` calls `execute_task` again | `lib/crewai/src/crewai/agent/core.py:685-759` |
| Agent core execution retry budget | `max_retry_limit: int = 2` | `lib/crewai/src/crewai/agent/core.py:247-250, 707-717` |
| Output parser error recovery | `handle_output_parser_exception` appends parser error to messages | `lib/crewai/src/crewai/utilities/agent_utils.py:660-695` |
| Context-length overflow recovery | `handle_context_length` summarises or `SystemExit`s | `lib/crewai/src/crewai/utilities/agent_utils.py:712-749` |
| Max-iteration recovery | `handle_max_iterations_exceeded` appends a "force final answer" prompt and re-invokes LLM | `lib/crewai/src/crewai/utilities/agent_utils.py:293-329` |
| Force-final-answer prompt template | `force_final_answer` / `force_final_answer_error` i18n strings | `lib/crewai/src/crewai/translations/en.json:46-47` |
| Max-iteration routing in AgentExecutor | `check_max_iterations` routes to `"force_final_answer"` router | `lib/crewai/src/crewai/experimental/agent_executor.py:2113-2120` |
| Force-final-answer router | `ensure_force_final_answer` makes the final call and routes to `agent_finished` | `lib/crewai/src/crewai/experimental/agent_executor.py:1354-1380` |
| Structured-output converter retry (sync) | `Converter.to_pydantic` recurses with `current_attempt + 1` | `lib/crewai/src/crewai/utilities/converter.py:84-116` |
| Structured-output converter retry (async) | `ato_pydantic` mirrors sync | `lib/crewai/src/crewai/utilities/converter.py:118-140` |
| Converter retry budget | `max_attempts: int = 3` field on `OutputConverter` | `lib/crewai/src/crewai/agents/agent_builder/utilities/base_output_converter.py:31-34` |
| Partial-JSON repair layer | `handle_partial_json` runs before recursing the converter | `lib/crewai/src/crewai/utilities/converter.py:280-332` |
| Async partial-JSON repair | `async_handle_partial_json` defers LLM fallback to `acall` | `lib/crewai/src/crewai/utilities/converter.py:444-499` |
| Reasoning-loop (Plan-and-Act) | `AgentReasoning._refine_plan_if_needed` re-prompts while not ready | `lib/crewai/src/crewai/utilities/reasoning_handler.py:278-349` |
| Reasoning-loop budget | `max_attempts` from `PlanningConfig` (default None = unbounded) | `lib/crewai/src/crewai/utilities/reasoning_handler.py:293-298, 342-347` |
| PlannerObserver (per-step reflection) | Calls LLM with task + completed steps + remaining plan, returns `StepObservation` | `lib/crewai/src/crewai/agents/planner_observer.py:113-189` |
| Heuristic observation fallback | `PlannerObserver.heuristic_observation` skips LLM call | `lib/crewai/src/crewai/agents/planner_observer.py:87-111` |
| Observation-driven routing (low) | `handle_step_observed_low` — heuristic, hard-fail replan only | `lib/crewai/src/crewai/experimental/agent_executor.py:675-745` |
| Observation-driven routing (medium) | `handle_step_observed_medium` — replan on failure only | `lib/crewai/src/crewai/experimental/agent_executor.py:747-811` |
| Observation-driven routing (high) | `decide_next_action` — goal / replan / refine / continue | `lib/crewai/src/crewai/experimental/agent_executor.py:813-895` |
| Refine-and-continue path | `handle_refine_and_continue` applies structured refinements to pending todos | `lib/crewai/src/crewai/experimental/agent_executor.py:897-942` |
| Replan-now path | `handle_replan_now` invokes `_trigger_replan` and routes back | `lib/crewai/src/crewai/experimental/agent_executor.py:976-1014` |
| Replan dynamic-decision helper | `_should_replan` — failed-todo count, error-todo count, agent phrasing | `lib/crewai/src/crewai/experimental/agent_executor.py:2452-2502` |
| Replan context payload | `_build_replan_context` summarises completed/failed/history | `lib/crewai/src/crewai/experimental/agent_executor.py:2591-2632` |
| Replan enhancement prompt | Injects previous attempt context via `replan_enhancement_prompt` | `lib/crewai/src/crewai/experimental/agent_executor.py:2634-2651` + `lib/crewai/src/crewai/translations/en.json:100` |
| Replan budget | `max_replans: int = 3` on `PlanningConfig` | `lib/crewai/src/crewai/agent/planning_config.py:122-126` |
| Replan budget resolution | `_get_max_replans` reads from agent planning config | `lib/crewai/src/crewai/experimental/agent_executor.py:497-509` |
| Replan counter state | `AgentExecutorState.replan_count`, `last_replan_reason` | `lib/crewai/src/crewai/experimental/agent_executor.py:148-153` |
| StepObservation model | `step_completed_successfully`, `key_information_learned`, `remaining_plan_still_valid`, `needs_full_replan`, `replan_reason`, `goal_already_achieved`, `suggested_refinements` | `lib/crewai/src/crewai/utilities/planning_types.py:212-278` |
| Reasoning effort levels | `"low"` / `"medium"` / `"high"` gate observe and replan | `lib/crewai/src/crewai/agent/planning_config.py:79-97` |
| Step-execution budget | `max_step_iterations: int = 15`, `step_timeout: int | None` | `lib/crewai/src/crewai/agent/planning_config.py:127-142` |
| Replan terminal observation default | On observation failure: conservative replan default | `lib/crewai/src/crewai/agents/planner_observer.py:191-213` |
| Parser-error recovery in flow | `recover_from_parser_error` router | `lib/crewai/src/crewai/experimental/agent_executor.py:2678-2699` |
| Context-length recovery in flow | `recover_from_context_length` router | `lib/crewai/src/crewai/experimental/agent_executor.py:2701-2715` |
| Guardrail observability events | `LLMGuardrailStartedEvent`, `LLMGuardrailCompletedEvent` carry `retry_count`, `success`, `error` | `lib/crewai/src/crewai/events/types/llm_guardrail_events.py:24-70` |
| Plan-observation events | `StepObservationStarted/Completed/FailedEvent`, `PlanReplanTriggeredEvent`, `GoalAchievedEarlyEvent` | `lib/crewai/src/crewai/events/types/observation_events.py:61-131` |
| Guardrail console output | Rich panel `handle_guardrail_started/completed` with `Attempts` field | `lib/crewai/src/crewai/events/utils/console_formatter.py:1283-1339` |
| Replan console output | `handle_plan_replan` shows replan_count + reason | `lib/crewai/src/crewai/events/utils/console_formatter.py:1041-1060` |
| Plan-reflection prompt | Plan-and-Act observation system / user prompts | `lib/crewai/src/crewai/translations/en.json:88-89` |
| Replan enhancement prompt | "Previous execution attempt did not fully succeed. Please create a revised plan…" | `lib/crewai/src/crewai/translations/en.json:100` |
| Guardrail test — failing guardrail retry | Asserts `retry_count == 1` after one failure | `lib/crewai/tests/test_task_guardrails.py:71-95` |
| Guardrail test — guardrail_max_retries budget | Asserts `retry_count == 2` after exhaustion | `lib/crewai/tests/test_task_guardrails.py:98-122` |
| Guardrail test — event emission | Verifies Started/Completed events include `retry_count` | `lib/crewai/tests/test_task_guardrails.py:210-301` |
| Guardrail test — multiple guardrails retry counts | Per-index `_guardrail_retry_counts` | `lib/crewai/tests/test_task_guardrails.py:392-440, 705-775` |
| LiteAgent guardrail retry cap | Verifies `_guardrail_retry_count >= guardrail_max_retries` raises | `lib/crewai/tests/agents/test_lite_agent.py:458-503` |
| Converter retry test | Drives 3-call LLM to recover via JSON repair | `lib/crewai/tests/utilities/test_converter.py:490-514` |
| Agent execution retry test | `_times_executed == 2` after `max_retry_limit=1` | `lib/crewai/tests/agents/test_agent.py:1200-1244` |
| Replan-on-failure routing test (medium) | Mocks `StepObservation(needs_full_replan=True)` → routes to `"replan_now"` | `lib/crewai/tests/agents/test_agent_executor.py:1781-1858` |
| Reasoning-effort routing tests | Low skips LLM observe; High runs full pipeline | `lib/crewai/tests/agents/test_agent_executor.py:1617-1711, 1725-1779` |
| Force-final-answer routing test | `check_max_iterations` returns `"force_final_answer"` | `lib/crewai/tests/agents/test_agent_executor.py:2124-2154` |
| Parser-error recovery test | `recover_from_parser_error` returns `"initialized"` | `lib/crewai/tests/agents/test_agent_executor.py:774-791` |
| Crew-level evaluators (post-hoc, not reflexive) | `CrewEvaluator`, `TaskEvaluator`, `AgentEvaluator` are observability/eval tools, **not** part of the live reask loop | `lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:29-222`, `lib/crewai/src/crewai/experimental/evaluation/agent_evaluator.py:44-365` |

## Answers to Dimension Questions

1. **When does the agent self-correct?**
   - **Output validation failed** → Task/LiteAgent/Agent guardrail path (`task.py:1246-1463`, `lite_agent.py:696-732`, `agent/core.py:1823-1892`) re-invokes the LLM with the previous output and the validation error embedded in a `validation_error` prompt (`translations/en.json:55`).
   - **Pydantic/JSON validation failed** → `Converter.to_pydantic` / `to_json` recurses up to `max_attempts` (`utilities/converter.py:84-178`), with a partial-JSON repair layer (`utilities/converter.py:280-332`) ahead of recursion.
   - **Plan step failed** → `PlannerObserver.observe` produces a `StepObservation` (`agents/planner_observer.py:113-189`); depending on `reasoning_effort`, the executor routes to `replan_now`, `refine_and_continue`, `goal_achieved`, or `continue_plan` (`experimental/agent_executor.py:676-895`).
   - **Iteration cap hit** → `handle_max_iterations_exceeded` appends `force_final_answer` and makes one more LLM call (`utilities/agent_utils.py:293-329`, `translations/en.json:46-47`).
   - **Parser error** → `handle_output_parser_exception` appends the parse error as a `user` message (`utilities/agent_utils.py:660-695`).
   - **Context overflow** → `handle_context_length` summarises or `SystemExit`s (`utilities/agent_utils.py:712-749`).
   - **Generic execution exception** → `_check_execution_error` / `_handle_execution_error` re-invoke the task up to `max_retry_limit` (`agent/core.py:685-759`).

2. **Is correction bounded?**
   - Yes for every path. `for attempt in range(max_attempts)` in `task.py:1264, 1374`; `current_attempt < self.max_attempts` in `utilities/converter.py:106, 112, 130, 136, 160, 176`; `replan_count >= max_replans` guard in `experimental/agent_executor.py:986, 2466, 2662`; `retry_count >= self.guardrail_max_retries` in `lite_agent.py:706` and `agent/core.py:1865`; `_times_executed > self.max_retry_limit` in `agent/core.py:708`. Exhaustion always raises a typed exception or routes to the "force final answer" terminal prompt — never an unbounded recursive retry.

3. **What evidence is shown during correction?**
   - **Guardrail path:** the prior raw task output plus the validation error verbatim, framed by the `validation_error` prompt template (`task.py:1310-1313`, `translations/en.json:55`).
   - **LiteAgent path:** just the error string as a user message (`lite_agent.py:718-723`) — no prior output included.
   - **Agent path (core):** the error string as a user message before re-executing (`agent/core.py:1871-1876`).
   - **Converter path:** the entire prior LLM response is re-fed through the converter each attempt (`utilities/converter.py:96-103`).
   - **Replan path:** `_build_replan_context` produces a structured summary of completed steps + their results, failed/error steps + their errors, and replan history (`experimental/agent_executor.py:2591-2632`); this is injected into `AgentReasoning.handle_agent_reasoning` (`experimental/agent_executor.py:2547, 2559`).
   - **Per-step observation path:** the LLM receives task description + goal + completed steps + their results + remaining steps + the just-completed step's description and result (`agents/planner_observer.py:244-301`, `translations/en.json:88-89`).

4. **Does correction improve outputs or hide errors?**
   - **Improves outputs:** the converter path drives the LLM toward producing valid JSON/Pydantic; the guardrail path drives the LLM toward producing output that satisfies the guardrail; the replan path drives the planner to generate an alternative plan that accounts for previous failures; the per-step observation path allows in-flight refinement of pending steps.
   - **Never hides errors:** every path either (a) successfully transforms the output, (b) returns a structured failure (`GuardrailResult(success=False, error=...)`, `StepObservation(needs_full_replan=True, replan_reason=...)`, `ConverterError`), or (c) raises an exception. The `HallucinationGuardrail` no-op case is the only path that returns `True` without evaluation (`tasks/hallucination_guardrail.py:95-103`), and that is explicit and logged with a red warning.

5. **Are repeated failures escalated?**
   - Yes via bounded exhaustion:
     - Guardrail retries exhausted → `Exception` raised at `task.py:1298-1301`, `lite_agent.py:707-710`, `agent/core.py:1866-1869`. `AgentExecutionErrorEvent` is emitted before the raise (`agent/core.py:709-716`).
     - Converter retries exhausted → `ConverterError` raised at `utilities/converter.py:108-115, 133-139, 162, 178`.
     - Replans exhausted → router returns `"no_todos"` and the executor finalises with current results (`experimental/agent_executor.py:2660-2668`).
     - Max iterations → forced final answer + then `agent_finished` router (`experimental/agent_executor.py:1354-1380`).
     - Reasoning refinement attempts → logger.warning + break with current plan (`utilities/reasoning_handler.py:342-347`).
   - Every escalation point is observable: `LLMGuardrailCompletedEvent` carries `success`/`error`/`retry_count` (`events/types/llm_guardrail_events.py:56-70`); `PlanReplanTriggeredEvent` carries `replan_reason` and `replan_count` (`events/types/observation_events.py:110-121`); `StepObservationFailedEvent` is emitted on observation LLM failure (`agents/planner_observer.py:196-206`).

## Architectural Decisions

- **Three independent retry loops** rather than one global reflection driver. Each loop owns its own budget, evidence format, and exit signal. This avoids cross-coupling but means each surface must be tested independently.
- **Guardrail returns a `tuple[bool, Any]`** — the success branch *transforms* the output, the failure branch *describes the error*. This is explicit at `utilities/guardrail.py:60-120` (typed result + mutual-exclusion validator) and `tasks/llm_guardrail.py:115-117`.
- **Hard retries (`max_attempts`) instead of exponential backoff** for both converter and guardrail paths. Simpler to reason about; cost is predictable.
- **Replan is a new LLM call**, not a rewrite of in-flight state. The previous plan's completed todos are preserved; only `pending` ones are replaced (`utilities/planning_types.py:185-195`, `experimental/agent_executor.py:2577`).
- **Reflection is event-driven**, not callback-driven. `PlannerObserver` emits `StepObservationStartedEvent` / `StepObservationCompletedEvent` / `StepObservationFailedEvent` (`events/types/observation_events.py:61-95`), enabling monitoring without coupling.
- **Reasoning effort is a three-level knob** (`low`/`medium`/`high`, `agent/planning_config.py:79-97`) — not a free parameter. Each level pins a different combination of LLM observation, replan triggering, and refinement behavior (`experimental/agent_executor.py:518-530`).
- **Failures emit before retries** so monitoring systems see every attempt: `AgentExecutionErrorEvent` fires inside `_check_execution_error` *before* re-raise (`agent/core.py:709-716`); `LLMGuardrailCompletedEvent` fires for every attempt regardless of outcome (`utilities/guardrail.py:173-185`).
- **Structured `StepRefinement` objects** carry refinement payloads without a second LLM call (`utilities/planning_types.py:198-204`), applied in-memory by `apply_refinements` (`agents/planner_observer.py:215-242`).
- **Force-final-answer on iteration exhaustion** — rather than raising, the agent makes one last LLM call with a directive to ignore prior instructions and produce the best possible answer (`translations/en.json:47`, `utilities/agent_utils.py:320-329`). This trades off "give up cleanly" vs. "best-effort result".

## Notable Patterns

- **Self-referential recursion with explicit attempt counter**: `Converter.to_pydantic` uses Python recursion (`utilities/converter.py:106-107`) rather than a `while` loop. This makes each attempt observable in stack traces but does not protect against recursion depth.
- **Per-index retry bookkeeping**: `Task._guardrail_retry_counts: dict[int, int]` (`task.py:288-291`) tracks each guardrail in a `guardrails=[...]` list independently. Tests confirm this: `tests/test_task_guardrails.py:440, 518, 554, 773-775`.
- **Coercion validators** to handle LLM output variance: `StepObservation.suggested_refinements` accepts either a dict or a list (`utilities/planning_types.py:259-265`); `_parse_observation_response` handles `BaseModel` / str / dict / unknown (`agents/planner_observer.py:303-351`).
- **Heuristic + LLM fallback** for observation: `PlannerObserver.heuristic_observation` produces a no-LLM `StepObservation` when `observe_steps=False` (`agents/planner_observer.py:87-111`); `_observe_completed_step` chooses between heuristic and LLM paths (`experimental/agent_executor.py:544-573`).
- **Two guardrail classes coexisting**: `LLMGuardrail` (`tasks/llm_guardrail.py:49-119`) for LLM-as-judge and `HallucinationGuardrail` as a no-op placeholder (`tasks/hallucination_guardrail.py:20-103`). The dispatcher (`events/types/llm_guardrail_events.py:42-49`) tags the event type accordingly.
- **Per-replan context window** rather than full conversation replay: `_build_replan_context` emits a summary, not raw messages (`experimental/agent_executor.py:2591-2632`). This protects token budget but loses message-level fidelity.
- **Replan reason telemetry**: both `_should_replan` (`experimental/agent_executor.py:2486-2500`) and the decide pipeline (`experimental/agent_executor.py:851-873`) populate `last_replan_reason` so external observers can tell *why* a replan fired.
- **Default to "ready" on planner error**: `AgentReasoning._call_with_function` fallback returns `ready=True` to avoid getting stuck when planning itself fails (`utilities/reasoning_handler.py:438-444`).
- **Conservative default on observation LLM failure**: `PlannerObserver.observe` returns `step_completed_successfully=True` when its own LLM call fails, with a warning log (`agents/planner_observer.py:191-213`). This biases toward "keep going" rather than "false replan".

## Tradeoffs

- **No LLM-as-judge on the *final* answer** — the open-source `HallucinationGuardrail` is a placeholder (`tasks/hallucination_guardrail.py:73-103`). A real judge would catch successful-but-wrong outputs that pass surface validation; the framework deliberately reserves this for the commercial product.
- **Guardrail re-ask appends error but not prior output** in the LiteAgent path (`lite_agent.py:718-723`) — the LLM sees only the error string. The Task path is richer (`task.py:1310-1313`).
- **Recursive `_execute_core` for LiteAgent guardrail** (`lite_agent.py:726`) — easy to reason about but can stack-overflow on long retries; no iterative rewrite exists.
- **Force-final-answer on iteration cap** trades silent failure for a possibly-contradicting directive prompt. The agent can ignore its prior plan and produce garbage.
- **No global Agent-level reflection** — only the experimental `AgentExecutor` runs the Plan-and-Act observer pipeline. Legacy `CrewAgentExecutor` (`agents/crew_agent_executor.py:309-468`) only handles parser errors, context length, and max iterations.
- **Reasoning effort budget unbounded when `max_attempts is None`** (`utilities/reasoning_handler.py:296`). This matches the docstring's stated behavior — `if None, will continue until the agent indicates readiness` (`utilities/reasoning_handler.py:99-104`) — but is a loop hazard if the planner never emits `READY`.
- **Observation defaults to success on its own failure** (`agents/planner_observer.py:208-213`). Pragmatic for availability but masks a real failure mode.
- **Partial-JSON repair runs before recursion** (`utilities/converter.py:63-79`). This is good for malformed LLM output but means the first retry is sometimes invisible — the user sees a single successful call when in fact a repair layer intervened.
- **Replan "natural-language" trigger** (`experimental/agent_executor.py:2490-2500`) — the agent can phrase something like "let's try a different approach" and the executor will replan. Powerful but fragile to phrasing changes.

## Failure Modes / Edge Cases

- **Reasoning loop unbounded** if `max_attempts=None` and the planner never emits `READY` (`utilities/reasoning_handler.py:296-298`). Mitigated by `step_timeout` (`agent/planning_config.py:135-142`) but only if configured.
- **HallucinationGuardrail silently passes** when no `_validate_output_hook` is registered (`tasks/hallucination_guardrail.py:95-103`). Users may believe hallucination checking is active when it is not.
- **Multiple-guardrail error attribution** — if guardrail index 1 fails but index 0 succeeds, the retry prompt for index 0 is **not** re-emitted because `ensure_guardrails_is_list_of_callables` clears `self.guardrail` (`task.py:454-456`). The retry mechanism is keyed per-index but cross-talk is possible.
- **Checkpoint serialization drops callable guardrails** with a `UserWarning` (`utilities/guardrail.py:12-31`) — restored checkpoints silently lose guardrails.
- **`_passthrough_exceptions` is hard-coded to an empty tuple** (`agent/core.py:133`) — declared as the design hook for "do not retry this" but no exceptions are registered, so every error participates in the retry loop (subject to `max_retry_limit` and the litellm passthrough at `agent/core.py:695-704`).
- **LiteAgent guardrail recursion shares the same `_messages` list** across attempts (`lite_agent.py:718-723`); without an explicit compaction, every failed guardrail error is permanently appended, inflating context size on each retry.
- **Force-final-answer can contradict the original plan**: the i18n string says "ignore all previous instructions, stop using any tools, and just return your absolute BEST Final answer" (`translations/en.json:47`) — this is a hard directive that can produce a non-conforming answer in production.
- **Parser errors do not increment `guardrail_retry_count`** — they participate in the loop's `iterations` counter (`utilities/agent_utils.py:660-695`) but a successful task with hundreds of parser retries could exhaust `max_iter` while still looking fine to a guardrail-based monitor.
- **Replan increments `replan_count` even on heuristic observation** (`experimental/agent_executor.py:2670`) but does **not** re-trigger `PlannerObserver` until the next step completes — so a single bad step burns one replan attempt.
- **AsyncLiteAgent's `_execute_core` recursion** (`lite_agent.py:726`) — `asyncio.run` is not used here, so a long retry chain on an async path could block the event loop on each `_invoke_loop` call.
- **`max_iter` boundary tests show off-by-one tolerance** (`tests/agents/test_agent.py:478-480`): `iterations <= agent.max_iter + 2` — implies the loop can run a couple of iterations past `max_iter` before bailing.
- **No idempotency check** between `GuardrailResult.result` types — `_invoke_guardrail_function` only handles `str` and `TaskOutput` results (`task.py:1280-1288`); any other returned type is silently dropped, leaving the prior task output in place.

## Future Considerations

- Promote `HallucinationGuardrail` from no-op to a real LLM-judge implementation, exposed under a free-tier flag (matching the comment in `tasks/hallucination_guardrail.py:73-77`).
- Add a per-retry compaction step in `LiteAgent` (`lite_agent.py:718-723`) so the message list does not grow linearly with retries.
- Replace `_execute_core` recursion in `LiteAgent` (`lite_agent.py:726`) with an iterative loop and a guard against runaway recursion.
- Move from "reasoning_effort string + `observe_steps` bool" (`agent/planning_config.py:79-97`) to a typed policy object that can be validated up front.
- Surface replan counters and `last_replan_reason` on the returned `LiteAgentOutput` (currently visible only via telemetry at `agent/core.py:1799-1800`).
- Add a global "reflection driver" that scores the *final* agent output against the goal and routes to replan or guardrail-validate if the score is low — this is what an LLM-as-judge pipeline would look like in practice.
- Wire `_passthrough_exceptions` (`agent/core.py:133`) to actual exception types (e.g. `RateLimitError`, `AuthenticationError`) so known-unrecoverable errors do not participate in the retry loop.
- Extend `_should_replan` (`experimental/agent_executor.py:2452-2502`) to recognise *stalled* patterns (same error string N times in a row) and short-circuit to escalation.
- Add per-call budget tracking for the planner LLM (separate token budget from the executor LLM) — currently the same LLM is reused unless `PlanningConfig.llm` is overridden (`agents/planner_observer.py:69-85`, `utilities/reasoning_handler.py:175-185`).

## Questions / Gaps

- The CrewAI docs at `docs/edge/en/concepts/...` were not inspected for this study; the analysis is based solely on source code under `lib/crewai/src/crewai/`. No evidence was found in source for a documented reflection playbook — operators must read code to understand the loop budgets.
- Whether `Agent.reasoning` (deprecated, `agent/core.py:277-286`) still routes to a real reflection loop was not fully traced; the deprecated `max_reasoning_attempts` field is read once in `utilities/reasoning_handler.py:170-172`.
- The `HallucinationGuardrail._validate_output_hook` (`tasks/hallucination_guardrail.py:17`) is module-global and never assigned in this source tree — it is set by the proprietary extension. This is intentional (paid feature) but undocumented in OSS source.
- The event payload of `LLMGuardrailCompletedEvent` includes `result: Any` (`events/types/llm_guardrail_events.py:69`) which can leak the guardrail's transformed output to all subscribers; no scrubbing observed.
- No clear evidence found that `CrewAgentExecutor` (the legacy executor at `agents/crew_agent_executor.py`) participates in the Plan-and-Act replan pipeline — it only handles iteration cap and parser errors. Whether `Agent.executor_type` (`agent/core.py:181-209`) is the only switch is the key question, and the answer in code is "yes, the experimental executor is the only one wired to `PlannerObserver`".

---

Generated by `03.05-reflection-reask-and-self-correction-loops` against `crewai`.