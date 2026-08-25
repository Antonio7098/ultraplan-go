# Source Analysis: crewai

## 01.02 — Control-Flow Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `sources/crewai` |
| Language / Stack | Python 3.10+. Monorepo under `lib/`: `crewai` (agent/crew/flow runtime), `crewai-core` (paths, printer, settings, telemetry), `crewai-files`, `crewai-tools`, `devtools`. Primary analysis surface: `lib/crewai/src/crewai/`. |
| Analyzed | 2026-07-02 |

## Summary

CrewAI spreads control flow across three runtime owners and one model-driven layer, with the **runtime as the principal authority**. The framework explicitly binds the LLM to a small typed contract (`AgentAction` / `AgentFinish` in the legacy loop; `Literal[...]` event labels on `@router` methods in the Flow loop) and the runtime owns iteration, validation, termination, replanning, and finalization. The model proposes; the runtime disposes.

There are three live executor implementations:

1. `crewai.agents.crew_agent_executor.CrewAgentExecutor` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:98`) — deprecated hand-rolled ReAct loop.
2. `crewai.experimental.agent_executor.AgentExecutor` (`lib/crewai/src/crewai/experimental/agent_executor.py:164`) — `Flow[AgentExecutorState]`, the authoritative control surface.
3. `crewai.lite_agent.LiteAgent._invoke_loop` (`lib/crewai/src/crewai/lite_agent.py:860`) — a single-agent stripped-down ReAct loop.

`Agent.executor_class` (`lib/crewai/src/crewai/agent/core.py:337-344`) selects between #1 and #2. The Flow executor is the default and the only one in active development. The outermost loop is `Crew._execute_tasks` (`lib/crewai/src/crewai/crew.py:1529-1598`), a plain Python `for` loop that the model never influences.

## Rating

**8 / 10** — Clear, typed, well-tested model with explicit interfaces and operational safeguards, with two durable frictions noted below.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Process enum (outermost process types) | `class Process(str, Enum)` with `sequential` / `hierarchical` | `lib/crewai/src/crewai/process.py:4-10` |
| Crew-level outer loop | `_execute_tasks` — plain `for task_index, task in enumerate(tasks)` with optional async futures | `lib/crewai/src/crewai/crew.py:1529-1598` |
| Crew kickoff dispatch (runtime-only) | `if self.process == Process.sequential: result = self._run_sequential_process()` (no model input) | `lib/crewai/src/crewai/crew.py:1037-1044` |
| Async task parallelism | `task.execute_async` returns `Future[TaskOutput]`; `futures` drained at sync boundary | `lib/crewai/src/crewai/task.py:596-610`, `lib/crewai/src/crewai/crew.py:1550-1596` |
| Conditional task | `ConditionalTask.should_execute(context)` predicate decides skip | `lib/crewai/src/crewai/tasks/conditional_task.py:41-55` |
| Hierarchical process | `_create_manager_agent` builds manager at runtime with `allow_delegation=True`; uses `AgentTools(agents=self.agents).tools()` | `lib/crewai/src/crewai/crew.py:1494-1519` |
| Next-step types (legacy executor) | `class AgentAction` / `class AgentFinish` dataclasses — the only two return types the legacy loop accepts | `lib/crewai/src/crewai/agents/parser.py:25-43` |
| Parser enforcement | `parse()` raises `OutputParserError` on malformed ReAct text | `lib/crewai/src/crewai/agents/parser.py:62-128` |
| Legacy executor loop (ReAct) | `while not isinstance(formatted_answer, AgentFinish):` with tool execution and observation | `lib/crewai/src/crewai/agents/crew_agent_executor.py:340-468` |
| Legacy executor native-tools loop | `while True: ... return AgentFinish`; exits only via max-iter, tool-result-is-final, or unsupported-error downgrade | `lib/crewai/src/crewai/agents/crew_agent_executor.py:484-632` |
| Max-iteration gate | `has_reached_max_iterations` + `handle_max_iterations_exceeded` make one extra LLM call to force a final answer | `lib/crewai/src/crewai/utilities/agent_utils.py:280-350` |
| Output parser error recovery | `handle_output_parser_exception` rewrites the failed message and continues | `lib/crewai/src/crewai/utilities/agent_utils.py:660-695` |
| Context length handling | `is_context_length_exceeded` + `handle_context_length` summarizes or `SystemExit`s | `lib/crewai/src/crewai/utilities/agent_utils.py:698-749` |
| Hook veto (before tool) | `if hook_result is False: hook_blocked = True; break` in before-tool hooks | `lib/crewai/src/crewai/agents/crew_agent_executor.py:964-979` |
| Hook result rewrite (after tool) | `if after_hook_result is not None: result = after_hook_result` in after-tool hooks | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1037-1042` |
| Before-LLM hook block | `_setup_before_llm_call_hooks` returns `False` → `_prepare_llm_call` raises `ValueError("LLM call blocked by before_llm_call hook")` | `lib/crewai/src/crewai/utilities/agent_utils.py:401-425` |
| Native tools unsupported → text fallback | `is_native_tool_calling_unsupported_error` → `_downgrade_to_text_tool_calling` → `"continue_reasoning"` (Flow) or `_invoke_loop_react` (legacy) | `lib/crewai/src/crewai/utilities/agent_utils.py:1285-1288`, `lib/crewai/src/crewai/experimental/agent_executor.py:1538-1540`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:577-579` |
| Parallel native tool eligibility | `_should_parallelize_native_tool_calls` returns `False` if any tool has `result_as_answer=True` or `max_usage_count`; else `ThreadPoolExecutor(max_workers=min(8, n))` | `lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858` |
| `result_as_answer` short-circuit (legacy) | `if tool_result.result_as_answer: return AgentFinish(...)` in `handle_agent_action_core` | `lib/crewai/src/crewai/utilities/agent_utils.py:619-624` |
| `result_as_answer` short-circuit (Flow) | `if original_tool.result_as_answer: return AgentFinish(...)` in `_append_tool_result_and_check_finality` and `execute_native_tool` | `lib/crewai/src/crewai/experimental/agent_executor.py:1097-1106, 1773-1783` |
| `max_usage_count` runtime cap | `if current_usage_count >= max_count: max_usage_reached = True`; result becomes a denial string | `lib/crewai/src/crewai/experimental/agent_executor.py:899-909, 2019-2025` |
| Tool cache veto | `cached_result = self.tools_handler.cache.read(...)`; if hit, the function is never called | `lib/crewai/src/crewai/agents/crew_agent_executor.py:929-936` |
| Agent executor select | `executor_class: type[CrewAgentExecutor] \| type[AgentExecutor] = Field(default=AgentExecutor, ...)` — user can pick | `lib/crewai/src/crewai/agent/core.py:337-344` |
| Executor class map | `_EXECUTOR_CLASS_MAP = {"CrewAgentExecutor": CrewAgentExecutor, "AgentExecutor": AgentExecutor}` | `lib/crewai/src/crewai/agent/core.py:135-138` |
| `AgentExecutor` is a Flow | `class AgentExecutor(Flow[AgentExecutorState], BaseAgentExecutor):` — Flow state machine | `lib/crewai/src/crewai/experimental/agent_executor.py:164-167` |
| `AgentExecutorState` | `class AgentExecutorState(BaseModel)` — typed state for the flow | `lib/crewai/src/crewai/experimental/agent_executor.py:126-161` |
| `@start()` entry | `generate_plan` (no-op when `planning_enabled` is false); then `check_todos_available` router | `lib/crewai/src/crewai/experimental/agent_executor.py:314-353, 1018-1031` |
| `@router("step_executed")` decision | `observe_step_result` returns `Literal["step_observed_low", "step_observed_medium", "step_observed_high"]` | `lib/crewai/src/crewai/experimental/agent_executor.py:608-673` |
| Reasoned-effort routing | `handle_step_observed_low` / `handle_step_observed_medium` / `decide_next_action` with `Literal` return annotations | `lib/crewai/src/crewai/experimental/agent_executor.py:675-895` |
| Max-iter Flow router | `check_max_iterations` returns `Literal["force_final_answer", "continue_reasoning", "continue_reasoning_native"]` | `lib/crewai/src/crewai/experimental/agent_executor.py:2112-2123` |
| Force final answer Flow | `ensure_force_final_answer` calls `handle_max_iterations_exceeded` (one extra LLM call); guarded by `state.is_finished` | `lib/crewai/src/crewai/experimental/agent_executor.py:1354-1376` |
| Parser / context-length recovery routers | `recover_from_parser_error`, `recover_from_context_length` → return `"initialized"` to loop back | `lib/crewai/src/crewai/experimental/agent_executor.py:2678-2715` |
| Tool result is final | `tool_result.result_as_answer` short-circuits and routes to `tool_result_is_final` | `lib/crewai/src/crewai/experimental/agent_executor.py:1773-1783, 1811-1822` |
| Replan router | `handle_replan_now` returns `Literal["has_todos", "all_todos_complete"]`; respects `_get_max_replans()` | `lib/crewai/src/crewai/experimental/agent_executor.py:976-1016` |
| `needs_replan` recovery | `handle_replan` returns `Literal["has_todos", "no_todos"]`; `replan_count >= max_replans` terminates | `lib/crewai/src/crewai/experimental/agent_executor.py:2653-2676` |
| Finalize lock | `with self._finalize_lock: if self._finalize_called: return "completed"; self._finalize_called = True` | `lib/crewai/src/crewai/experimental/agent_executor.py:2266-2269` |
| Parallel finalize listeners | `@listen(or_("all_todos_complete", "agent_finished", "tool_result_is_final", "native_finished"))` — multiple branches can fire `finalize` | `lib/crewai/src/crewai/experimental/agent_executor.py:2246-2253` |
| Final fallback when no `AgentFinish` | `RuntimeError("Agent execution ended without reaching a final answer.")` | `lib/crewai/src/crewai/experimental/agent_executor.py:2802-2805, 2908-2911` |
| `StepExecutor` per-todo inner loop | `for _ in range(max_step_iterations):` with isolated messages list | `lib/crewai/src/crewai/agents/step_executor.py:336-367, 546-578` |
| Step timeout | `if step_timeout and start_time: elapsed = time.monotonic() - start_time; if elapsed >= step_timeout: return last_tool_result` | `lib/crewai/src/crewai/agents/step_executor.py:337-340, 547-554` |
| Expected-tool enforcement | `_validate_expected_tool_usage` raises if configured `tool_to_use` was not invoked | `lib/crewai/src/crewai/agents/step_executor.py:504-526` |
| Plan generation | `AgentReasoning.handle_agent_reasoning()` produces a plan from a separate LLM call | `lib/crewai/src/crewai/experimental/agent_executor.py:339-345`, `lib/crewai/src/crewai/utilities/reasoning_handler.py:187` |
| Plan execution synthesis | `_synthesize_final_answer_from_todos` makes a fourth LLM call to assemble the final answer | `lib/crewai/src/crewai/experimental/agent_executor.py:2359-2446` |
| `Flow` runtime executes listeners | `_execute_listeners`: runs routers sequentially then listeners in parallel via `asyncio.gather` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2984-3110` |
| Router annotation source | `_get_router_return_events` reads `Literal[...]` from method return annotation | `lib/crewai/src/crewai/flow/dsl/_router.py:86-88` |
| `or_` and `and_` conditions | `def or_(*triggers)` / `def and_(*triggers)` produce `FlowCondition` | `lib/crewai/src/crewai/flow/dsl/_conditions.py:22-27` |
| Sync method isolation in Flow | `ctx = contextvars.copy_context(); result = await asyncio.to_thread(ctx.run, method, ...)` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2829-2835` |
| Event suppression | `suppress_flow_events: bool = True  # always suppress for executor` — keeps event bus domain-focused | `lib/crewai/src/crewai/experimental/agent_executor.py:182` |
| `_is_executing` lock | `with self._execution_lock: if self._is_executing: raise RuntimeError(...)` — single-execution per executor instance | `lib/crewai/src/crewai/experimental/agent_executor.py:2736-2742, 2842-2848` |
| Magic auto-async | `if is_inside_event_loop(): return self.invoke_async(inputs)` — sync call returns a coroutine | `lib/crewai/src/crewai/experimental/agent_executor.py:2732-2734` |
| LLM stop words | `_llm_stop_words_applied(llm, executor)` injects stop tokens for "Final Answer" / "Observation" | `lib/crewai/src/crewai/utilities/agent_utils.py:248-280` |
| LiteAgent loop | `LiteAgent._invoke_loop` — third control surface, hand-rolled ReAct loop | `lib/crewai/src/crewai/lite_agent.py:860-972` |
| Guardrail retry | `_process_kickoff_guardrail` re-invokes executor up to `guardrail_max_retries` | `lib/crewai/src/crewai/agent/core.py:1823-1892` |
| Delegation as a tool | `track_delegation_if_needed` increments `task.increment_delegations(coworker)` | `lib/crewai/src/crewai/utilities/agent_utils.py:1166-1188` |
| A2A bounded external loop | `for turn_num in range(ctx.max_turns): execute_a2a_delegation(...)` | `lib/crewai/src/crewai/a2a/wrapper.py:1272` |
| ReAct prompt stop-words | `stop_words = [I18N_DEFAULT.slice("observation")]` — runtime-injected stop so the LLM cannot continue past an action | `lib/crewai/src/crewai/agent/core.py:1062-1066` |
| Max-execution-time guard | `Agent._execute_with_timeout` runs the executor in a thread and times out via `concurrent.futures.TimeoutError` | `lib/crewai/src/crewai/agent/core.py:857-890` |
| Replay checkpointing | `Crew._get_execution_start_index` resumes at first task with `task.output is None` | `lib/crewai/src/crewai/crew.py:1521-1527` |
| Test fixture for LLM-free execution | `_build_executor` constructs `AgentExecutor` via `model_construct` with no LLM | `lib/crewai/tests/agents/test_agent_executor.py:23-54` |
| Test: routing decisions | `test_route_by_answer_type_action`, `test_route_by_answer_type_finish`, `test_check_max_iterations_reached` | `lib/crewai/tests/agents/test_agent_executor.py:246-272` |
| Test: plan-step lifecycle events | `test_plan_step_lifecycle_events_are_emitted_from_todo_transitions` | `lib/crewai/tests/agents/test_agent_executor.py:702-741` |
| Test: max-iter finalize reset | `test_finalize_called_reset_in_invoke` | `lib/crewai/tests/agents/test_agent_executor.py:2181-2192` |

## Answers to Dimension Questions

### 1. Who decides what happens next?

- **Crew level (outermost):** `Crew._execute_tasks` (`lib/crewai/src/crewai/crew.py:1529-1598`). Sequential by default; `task.async_execution` enables `concurrent.futures.Future` parallelism, drained in declaration order. `ConditionalTask.should_execute` is the only mid-stream branch (`lib/crewai/src/crewai/tasks/conditional_task.py:41-55`). The model has no input into which task runs next.
- **Agent level (per task):** `AgentExecutor` Flow with `Literal`-typed `@router` methods (`lib/crewai/src/crewai/experimental/agent_executor.py:608-895, 1033-1319, 2112-2123, 2246-2715`). The LLM never picks the next flow method; it returns content and the Flow's `_execute_listeners` (`lib/crewai/src/crewai/flow/runtime/__init__.py:2984-3110`) routes on string labels.
- **Step level (planning only):** `StepExecutor` per-todo `for _ in range(max_step_iterations)` loop (`lib/crewai/src/crewai/agents/step_executor.py:336-367, 546-578`), plus `_validate_expected_tool_usage` enforcement (`lib/crewai/src/crewai/agents/step_executor.py:504-526`).
- **Legacy executor level:** `while not isinstance(formatted_answer, AgentFinish)` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:341`, `lib/crewai/src/crewai/lite_agent.py:873`). Three control surfaces total.

### 2. Can the LLM bypass runtime control?

**No.**

- The runtime constructs the next prompt, owns the iteration counter, and forces a final call when budget is exhausted (`lib/crewai/src/crewai/utilities/agent_utils.py:293-350`).
- The LLM can request tools the runtime doesn't know about — but then the call fails (`tool_result = "Tool not found"` at `lib/crewai/src/crewai/experimental/agent_executor.py:925-927`; `"wrong_tool_name"` error at `lib/crewai/src/crewai/utilities/tool_utils.py:265-268`).
- The LLM can produce malformed text, but `OutputParserError` is caught and the loop retries (`lib/crewai/src/crewai/experimental/agent_executor.py:2678-2699`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:434-442`).
- Stop words injected by the runtime prevent the LLM from continuing past an action (`lib/crewai/src/crewai/agent/core.py:1062-1066`, `lib/crewai/src/crewai/utilities/agent_utils.py:248-280`).

**Caveat:** Planning mode adds a third LLM (`PlannerObserver`) and a fourth LLM (`_synthesize_final_answer_from_todos`) that have no inner-loop guardrails — only `max_replans` (`lib/crewai/src/crewai/experimental/agent_executor.py:497-502`) caps the replan loop.

### 3. Can the runtime reject or rewrite the next action?

**Yes, multiple ways.**

- `before_llm_call` hook returns `False` → `ValueError("LLM call blocked by before_llm_call hook")` (`lib/crewai/src/crewai/utilities/agent_utils.py:401-425`).
- `before_tool_call` hook returns `False` → `ToolResult("Tool execution blocked by hook. Tool: ...")` and the executor keeps looping (`lib/crewai/src/crewai/agents/crew_agent_executor.py:977-979`, `lib/crewai/src/crewai/utilities/agent_utils.py:1519-1520`).
- `after_tool_call` hook can replace the result string (`lib/crewai/src/crewai/agents/crew_agent_executor.py:1037-1042`).
- Tool `result_as_answer=True` short-circuits the loop into `AgentFinish` (`lib/crewai/src/crewai/utilities/agent_utils.py:619-624`, `lib/crewai/src/crewai/experimental/agent_executor.py:1097-1106, 1773-1783`).
- Tool `max_usage_count` returns a denial string when exhausted (`lib/crewai/src/crewai/experimental/agent_executor.py:899-909, 2019-2025`).
- Tool cache returns a stored result without invoking the function (`lib/crewai/src/crewai/agents/crew_agent_executor.py:929-936`).
- `handle_unknown_error` decides what to do with an unhandled exception (`lib/crewai/src/crewai/utilities/agent_utils.py:632-657`).
- Security fingerprint is plumbed into every tool call and a `set_fingerprint` failure becomes a hard error (`lib/crewai/src/crewai/utilities/tool_utils.py:67-77`).

**Caveat:** the `before_tool_call` hook contract is permissive — a single `False` denies the call but the executor keeps iterating; there is no "deny and stop" primitive.

### 4. Are transitions explicit or scattered?

- **In the modern Flow path: explicit and typed.** Every router returns `Literal[...]` (e.g. `lib/crewai/src/crewai/experimental/agent_executor.py:611, 678, 750, 816, 898, 943, 950, 978, 1021, 1036, 1070, 1186, 1322, 1355, 1379, 1452, 1569, 1583, 1653, 2090, 2107, 2112, 2125, 2156, 2217, 2236, 2246, 2653, 2678, 2701`). The Flow DSL validates static router emit sets at definition time.
- **In the legacy executor path: scattered.** A single `try/except/finally` block contains three implicit transitions (max-iter, tool-result-is-final, unknown error) and one explicit `break` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:340-468`).
- **In LiteAgent: scattered.** A near-duplicate of the legacy loop lives at `lib/crewai/src/crewai/lite_agent.py:860-972`.
- **In the Flow runtime:** the router/listener dispatch logic is centralized in `_execute_listeners` (`lib/crewai/src/crewai/flow/runtime/__init__.py:2984-3110`), but the executor's routers themselves are spread across ~30 methods in `agent_executor.py`.

### 5. Is control flow testable without calling an LLM?

**Yes, comprehensively.** `_build_executor` (`lib/crewai/tests/agents/test_agent_executor.py:23-54`) constructs an `AgentExecutor` via `model_construct` (bypassing Pydantic validators), seeds `_state`, `_methods`, `_method_execution_counts`, etc., and patches the LLM with `Mock()`. Routing decisions are then exercised directly:

- `test_route_by_answer_type_action` / `test_route_by_answer_type_finish` (`lib/crewai/tests/agents/test_agent_executor.py:254-272`)
- `test_check_max_iterations_reached` (`lib/crewai/tests/agents/test_agent_executor.py:246-252`)
- `test_recover_from_parser_error` (`lib/crewai/tests/agents/test_agent_executor.py:775-791`)
- `test_recover_from_context_length` (`lib/crewai/tests/agents/test_agent_executor.py:793-799`)
- `test_execute_native_tool_runs_parallel_for_multiple_calls` (`lib/crewai/tests/agents/test_agent_executor.py:1051-1085`)
- `test_execute_native_tool_falls_back_to_sequential_for_result_as_answer` (`lib/crewai/tests/agents/test_agent_executor.py:1087+`)

Plan step lifecycle is testable in isolation: `test_plan_step_lifecycle_events_are_emitted_from_todo_transitions` (`lib/crewai/tests/agents/test_agent_executor.py:702-741`).

**Caveat:** Tests for the actual Flow execution (not the `model_construct`-bypassing unit tests) require `@pytest.mark.vcr()` cassettes with real LLM traces (`lib/crewai/tests/agents/test_agent_executor.py:1297, 1376, 1422, 1476`). VCR-style replay of LLM responses is a workable substitute but couples the test to a recorded trace.

## Architectural Decisions

- **Reuse the `Flow` runtime for agent execution.** `AgentExecutor(Flow[AgentExecutorState], BaseAgentExecutor)` (`lib/crewai/src/crewai/experimental/agent_executor.py:164-167`) reuses the same `@start` / `@router` / `@listen` DSL that powers user-facing `Flow` classes. The Flow runtime is responsible for dispatch (`lib/crewai/src/crewai/flow/runtime/__init__.py:2984-3110`); the executor contributes only the typed state, the routers, and the listeners. This avoids re-implementing a state machine and gives users one mental model.
- **Always suppress flow events from the executor.** `suppress_flow_events: bool = True  # always suppress for executor` (`lib/crewai/src/crewai/experimental/agent_executor.py:182`) prevents the agent loop from emitting `MethodExecutionStartedEvent` / `MethodExecutionFinishedEvent` for every router transition. Domain events (`ToolUsageStartedEvent`, `PlanStepCompletedEvent`, `StepObservationCompletedEvent`, etc.) are emitted explicitly. This is an explicit choice to keep the event bus signal-rich.
- **Plan-and-Execute on top of ReAct.** When `Agent.planning_enabled` is true, the executor prepends a planning LLM call (`generate_plan`, `lib/crewai/src/crewai/experimental/agent_executor.py:314-353`), creates typed `TodoItem`s, runs each through an isolated `StepExecutor`, and re-introduces a separate LLM (`PlannerObserver.observe`, `lib/crewai/src/crewai/agents/planner_observer.py:113-213`) to decide whether to continue / refine / replan. This is a structural commitment to Plan-and-Act (`arxiv 2503.09572`) — the comment in `step_executor.py:1-11` names the paper explicitly.
- **Two executors co-exist during the deprecation window.** `CrewAgentExecutor` (legacy) is the only one with `_invoke_loop_react` / `_invoke_loop_native_tools`; `AgentExecutor` (Flow) replaces it. `Agent.executor_class` (`lib/crewai/src/crewai/agent/core.py:337-344`) lets users pick. This is a deliberate migration ramp.
- **A `LiteAgent` ships as a third, lighter control surface.** It runs a single hand-rolled `while not isinstance(formatted_answer, AgentFinish)` loop (`lib/crewai/src/crewai/lite_agent.py:860-972`) without the Flow machinery, for cases where crews and plans are overkill.
- **Delegation is a tool, not a flow primitive.** `AgentTools(agents=self.agents).tools()` adds delegation tools (`Delegate work to coworker`, `Ask question to coworker`) to the agent's tool list when `allow_delegation` is true (`lib/crewai/src/crewai/agent/core.py:1173-1175`, `lib/crewai/src/crewai/utilities/agent_utils.py:1166-1188`). The LLM calls a tool; the tool itself is what routes to another agent. The control-flow consequence is bounded — delegation is one tool among many, and `max_iter` still applies.
- **A2A is a bounded external loop.** `for turn_num in range(ctx.max_turns):` in `crewai/a2a/wrapper.py:1272` is the cap on multi-turn A2A delegation. The agent's outer loop is unchanged; A2A is a *tool* the agent calls.

## Notable Patterns

- **Hand-rolled ReAct loop + parser as a state machine** (`lib/crewai/src/crewai/agents/crew_agent_executor.py:340-468`, `lib/crewai/src/crewai/agents/parser.py:25-128`). The pattern: each iteration produces an `AgentAction` or `AgentFinish`; if `Action`, the runtime executes the tool and loops; if `Finish`, the runtime returns. Simple, explicit, but only two result types.
- **Flow + `Literal`-typed routers as a state machine** (`lib/crewai/src/crewai/experimental/agent_executor.py`, `lib/crewai/src/crewai/flow/dsl/_router.py:97-160`). The pattern: each router method declares its outgoing event names in a `Literal[...]` return type; the Flow runtime dispatches on the returned string. This generalizes to N-way branching and parallel listeners, at the cost of indirection through string labels.
- **One-shot lock to suppress duplicate finalization** (`lib/crewai/src/crewai/experimental/agent_executor.py:2266-2269`). Pattern: `_finalize_lock` + `_finalize_called` flag, entered before synthesizing the final answer. The comment at `lib/crewai/src/crewai/experimental/agent_executor.py:2261-2265` names the failure mode the lock exists to prevent.
- **`_is_executing` lock per executor instance** (`lib/crewai/src/crewai/experimental/agent_executor.py:2736-2742, 2842-2848`). Pattern: an executor cannot be invoked twice concurrently. `RuntimeError("Executor is already running.")` instead of a deadlock.
- **Stop-word injection at the LLM client** (`lib/crewai/src/crewai/utilities/agent_utils.py:248-280`, `lib/crewai/src/crewai/agent/core.py:1062-1066`). The runtime injects `"Observation"` (and any custom `response_template` tail) as a stop token so the LLM cannot continue past the action boundary. This is a soft form of control — the LLM could still try to write "Observation" itself, but the prompt usually prevents it.
- **Tool-result-as-final-answer short-circuit** (`lib/crewai/src/crewai/utilities/agent_utils.py:619-624`, `lib/crewai/src/crewai/experimental/agent_executor.py:1097-1106, 1773-1783`). Pattern: a tool with `result_as_answer=True` immediately returns its result as the agent's final answer. This is a tool-level terminal that bypasses the loop.
- **Event bus as the only side channel** (`lib/crewai/src/crewai/events/event_bus.py:570`). Pattern: no per-call `set_trace`; the agent emits `ToolUsageStartedEvent`, `PlanStepCompletedEvent`, etc., and the consumer subscribes once. This decouples observability from control flow.
- **Delegate-as-tool pattern** (`lib/crewai/src/crewai/agent/core.py:1173-1175`, `lib/crewai/src/crewai/utilities/agent_utils.py:1166-1188`). Pattern: delegation is a tool, not a flow primitive. The LLM chooses when to delegate by calling the tool; the runtime does not get to interpose on the choice.

## Tradeoffs

- **Expressiveness vs. indirection.** The Flow-based executor can express 30+ typed transitions but reading a transition requires following string labels across files. The legacy executor is one big `while/try/except` block, easy to read but hard to extend.
- **Plan-and-Execute adds 3 LLMs.** `AgentReasoning` (planning) + `PlannerObserver` (post-step) + `_synthesize_final_answer_from_todos` (final assembly) = 3 LLM calls beyond the per-step execution. This is the cost of plan-and-act; the trade is recovered via replanning and recovery from mis-steps.
- **Two executors in deprecation.** `CrewAgentExecutor` is still selected by `executor_class` and is the path the CLI and many examples still exercise. The deprecation warning at `lib/crewai/src/crewai/agents/crew_agent_executor.py:145-151` is the only signal that the user is on a deprecated path.
- **Sync and async paths can drift.** `invoke` runs the Flow on a thread; `invoke_async` runs the Flow natively. Subtle differences (e.g., `_is_executing` lock ordering) can lead to bugs that only appear under specific entry points.
- **Hooks are coarse-grained.** A `before_tool_call` hook that returns `False` blocks the call but does not stop the loop. There is no per-tool allow-list / deny-list primitive — only a single deny signal per call.
- **The Flow runtime's `_finalize_lock` is a workaround for a real race.** The `or_("all_todos_complete", "agent_finished", "tool_result_is_final", "native_finished")` listener (`lib/crewai/src/crewai/experimental/agent_executor.py:2246-2253`) can fire twice in the same listener pass (per the comment at `lib/crewai/src/crewai/experimental/agent_executor.py:1357-1361`). The lock papers over the race but does not fix the underlying double-dispatch.
- **Native tools require per-provider parsing.** `extract_tool_call_info` (`lib/crewai/src/crewai/utilities/agent_utils.py:1191-1231`) and `is_tool_call_list` (`lib/crewai/src/crewai/utilities/agent_utils.py:1234-1264`) each handle OpenAI / Anthropic / Bedrock / Gemini shapes. When a new provider is added, both functions must be updated in lockstep. There is no provider-adapter abstraction.

## Failure Modes / Edge Cases

- **Loop-iteration racing in Flow finalization.** As called out, `finalize` can fire twice; the lock prevents duplicate `AgentFinish` writes but does not prevent double event emission.
- **Parser error never terminates.** If the LLM keeps producing unparseable text past `log_error_after` (default 3, `lib/crewai/src/crewai/agent/core.py:133`), `handle_output_parser_exception` keeps rewriting the prompt and continuing (`lib/crewai/src/crewai/utilities/agent_utils.py:660-695`). The only escape is the global `max_iter`.
- **Context length exceeded with `respect_context_window=False`.** The runtime raises `SystemExit` (`lib/crewai/src/crewai/utilities/agent_utils.py:747-749`). The agent cannot recover; the user's process exits.
- **Native-tools unsupported error mid-loop.** `_downgrade_to_text_tool_calling` (`lib/crewai/src/crewai/experimental/agent_executor.py:249-264`) appends a text-tool-calling message and re-routes. But the legacy `CrewAgentExecutor` *restarts* the loop via `_invoke_loop_react` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:577-579`) which discards the messages built so far. This is a behavior difference between the two executors.
- **Planning with no LLM responses.** If `_trigger_replan` fails (`lib/crewai/src/crewai/experimental/agent_executor.py:2585-2589`), the existing todos are kept. The agent continues executing the (now-stale) plan.
- **`max_usage_count` exhaustion mid-batch.** When the LLM returns multiple tool calls and one of them has `result_as_answer` or `max_usage_count`, the Flow falls back to sequential execution (`lib/crewai/src/crewai/experimental/agent_executor.py:1747-1786`). Sequential execution is the safe fallback; the trade is latency.
- **Stop-word injection can interfere with non-English text.** The runtime injects `"Observation"` as a stop word (English). If the LLM is responding in a different language, the stop word is rarely generated by the LLM so this is fine; but if the LLM does use the word, the response is truncated.
- **Delegation loop unobserved.** When the LLM delegates, the `Delegate work to coworker` tool is called and the receiving agent runs its own loop with its own `max_iter`. There is no per-crew counter for total delegated work.
- **Hook exception swallowed.** A `before_tool_call` hook that raises an exception is silently caught (`lib/crewai/src/crewai/utilities/agent_utils.py:1522-1523`); the call proceeds. A `before_llm_call` hook exception similarly falls through `_setup_before_llm_call_hooks`. This is intentional but easy to miss.
- **`LiteAgent` and `AgentExecutor` differ in their handling of `response_model` with tools.** `LiteAgent._invoke_loop` (`lib/crewai/src/crewai/lite_agent.py:888-897`) passes `response_model` unconditionally; `AgentExecutor.call_llm_and_parse` (`lib/crewai/src/crewai/experimental/agent_executor.py:1390-1394`) sets `effective_response_model = None if self.original_tools else self.response_model`. This is the correct behavior for each, but a port of code between the two can break.
- **Tool result that begins with `VISION_IMAGE:` sentinel** is rewritten into a multimodal content block in `StepExecutor._build_observation_message` (`lib/crewai/src/crewai/agents/step_executor.py:462-502`) but not in `_handle_agent_action_core` (`lib/crewai/src/crewai/utilities/agent_utils.py:589-629`). The legacy loop therefore cannot consume vision sentinels — the flow loop is the only path that supports them.
- **A `tool_choice` parameter is not exposed at the agent level.** The LLM is allowed to call any tool. Provider-level `tool_choice="required"` is not surfaced.

## Future Considerations

- **De-finitive executor path.** The deprecation of `CrewAgentExecutor` is a half-finished migration. The deprecation warning (`lib/crewai/src/crewai/agents/crew_agent_executor.py:145-151`) and `executor_class` selector (`lib/crewai/src/crewai/agent/core.py:337-344`) are the only signals. Documentation, CLI defaults, and templates all still touch the legacy path.
- **Plan-and-Execute invariants.** The four-LLM chain (plan → execute → observe → synthesize) has no formal invariant. The `replan_count` cap is the only one. A `max_planning_calls` / `max_observation_calls` cap would harden the path.
- **Flow `finalize` race.** The `_finalize_lock` is a workaround. Either the Flow runtime should de-duplicate identical-event listeners, or the executor should use a single terminal event.
- **Hook allow/deny lists.** A `before_tool_call` hook returns `False` to deny. A more structured API (allow-list of tools, deny-list of tools, cost caps) would make the runtime authority more discoverable.
- **Cross-executor test harness.** Today the legacy executor has its own tests, the Flow executor has its own tests, and `LiteAgent` has its own. A shared "decision table" that parameterizes the input → next-event mapping would catch drift between the three.
- **Provider-adapter abstraction for native tool calls.** `extract_tool_call_info` and `is_tool_call_list` (`lib/crewai/src/crewai/utilities/agent_utils.py:1191-1264`) are 70+ lines of branching. A `ToolCallAdapter` protocol with one implementation per provider would shrink the diff and surface the abstraction.
- **Removal of `LiteAgent` or formal unification.** `LiteAgent` is a third loop with its own state, its own dispatch, and its own tests. The Flow executor could subsume it; the legacy executor should not.
- **`max_iter` semantics are not uniform.** `AgentExecutor` uses `state.iterations`; `CrewAgentExecutor` uses `self.iterations`; `StepExecutor` uses `max_step_iterations`; `Flow` uses `_max_iterations` (default 100, `lib/crewai/src/crewai/flow/runtime/__init__.py:78`). The semantics differ (per-agent vs per-step vs per-Flow) and the user-facing configuration is also inconsistent.

## Questions / Gaps

- **What authority does the LLM have that the runtime does not surface?** No clear evidence. The runtime owns iteration, dispatch, validation, and termination; the LLM owns *what to say* and *which tool to call*. The "what" is bounded by tool allow-list (no-op — no list exists) and stop words.
- **Is `AgentExecutor.check_max_iterations` reachable for `max_iter == 0`?** No clear evidence found. `has_reached_max_iterations(iterations, max_iterations)` returns `iterations >= max_iterations` (`lib/crewai/src/crewai/utilities/agent_utils.py:280-290`), so a `max_iter=0` would force the very first call. But the constructor at `lib/crewai/src/crewai/experimental/agent_executor.py:187` defaults to 25, and `max_iter=0` is not validated against.
- **Is the `result_as_answer` short-circuit safe when the tool is in a parallel batch with non-`result_as_answer` tools?** The code at `lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858` rejects parallelism when *any* tool has `result_as_answer`; in the sequential fallback, the short-circuit returns immediately (`lib/crewai/src/crewai/experimental/agent_executor.py:1773-1786`). But the test at `test_execute_native_tool_falls_back_to_sequential_for_result_as_answer` only validates the timing (sequential), not whether partial results from earlier tool calls are dropped. No clear evidence that the partial-result state is cleaned up.
- **What happens to the executor's `state.messages` after a successful kickoff that the user inspects post-hoc?** `invoke_async` clears `state.messages` and `state.iterations` at the start of every invocation (`lib/crewai/src/crewai/experimental/agent_executor.py:2854-2866`). So `executor.state.messages` is not a reliable post-execution log; the `LiteAgentOutput` snapshots the messages at construction time (`lib/crewai/src/crewai/agent/core.py:1789-1801`). This is a footgun, not a bug.
- **Is the `or_` condition's `_discard_or_listener` API a private contract or stable?** No clear evidence. `_discard_or_listener(FlowMethodName("continue_iteration"))` (`lib/crewai/src/crewai/experimental/agent_executor.py:2109-2110`) is called inside the executor; the function's visibility is in `lib/crewai/src/crewai/flow/runtime/__init__.py:1261-1264`. The dependency is real but undocumented.
- **Does the runtime pre-validate that a tool is callable on a specific LLM provider?** No clear evidence. The runtime checks `supports_function_calling()` (`lib/crewai/src/crewai/utilities/agent_utils.py:1267-1282`) and falls back to text-tool calling on `is_native_tool_calling_unsupported_error`, but a tool that the LLM claims to support but cannot actually invoke is treated as `"Tool not found"` (`lib/crewai/src/crewai/utilities/agent_utils.py:1482-1490`). The user only sees the LLM call out to a non-existent function, with the LLM reacting to its own error.
- **How does the executor handle a tool that takes > `step_timeout` seconds to execute?** The `StepExecutor` checks `step_timeout` at the *start* of each iteration (`lib/crewai/src/crewai/agents/step_executor.py:337-340, 547-554`) but does not cancel an in-flight tool call. If a single tool runs for 10 minutes, the timeout only fires after it returns. The legacy executor has no per-step timeout at all.

---

Generated by `dimensions/01.02-control-flow-ownership.md` against `crewai`.