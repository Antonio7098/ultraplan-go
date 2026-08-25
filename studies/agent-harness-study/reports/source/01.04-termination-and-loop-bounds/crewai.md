# Source Analysis: crewai

## Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic v2, async-native) |
| Analyzed | 2026-07-02 |

## Summary

CrewAI implements a layered termination model that combines hard iteration caps, wall-clock timeouts, request-rate throttling, and an LLM-driven "force final answer" fallback that activates when iteration caps are hit. Three distinct execution paths each enforce their own loop bounds: the legacy `CrewAgentExecutor` (`agents/crew_agent_executor.py`) and its Flow-based replacement `AgentExecutor` (`experimental/agent_executor.py`), the `LiteAgent` (`lite_agent.py`), and the `StepExecutor` (`agents/step_executor.py`). A2A delegation has its own turn-bound loop (`a2a/wrapper.py`), and Flow method routing has a generic recursion-style cap (`flow/runtime/__init__.py:3179`). Exhaustion is distinguished from natural success: at the iteration ceiling the executor injects a `force_final_answer` prompt (`translations/en.json:47`) and forces one more LLM call, so the user almost always gets a final state. Termination is primarily runtime-driven (iteration counter + timer), not policy-driven; configuration is exposed per-agent/per-crew but there is no global policy layer that decides when to stop. Stuck-loop detection before the hard cap exists only for repeated-tool-call patterns (`tools/tool_usage.py:728`) and is heuristic rather than a guarantee.

## Rating

**7 / 10**

Rationale: clear primary termination model (`max_iter` + `max_execution_time` + RPM), explicit `AgentFinish` return on exhaustion via the forced-final-answer path, repeated-usage detection, and a dedicated `RecursionError` for Flow method-call cycles. Bounded by gaps: no centralized "stuck-loop" detector at the LLM-response level (only tool-call duplication), the forced-final-answer path is best-effort and can still fail with `RuntimeError`, max_execution_time only applies to the agent-level call (not per-iteration LLM calls), and A2A / Flow have separate, weaker safeguards.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-agent iteration cap (default 25) | `BaseAgent.max_iter: int = Field(default=25, ...)` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:286-288` |
| Executor inherits `max_iter` (default 25) | `max_iter: int = Field(default=25)` on `BaseAgentExecutor` | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:27` |
| LitAgent iteration cap (default 15) | `max_iterations: int = Field(default=15, ...)` | `lib/crewai/src/crewai/lite_agent.py:222-224` |
| Agent-level wall-clock timeout | `max_execution_time: int \| None = Field(default=None, ...)` | `lib/crewai/src/crewai/agent/core.py:203-206` |
| Validator rejects non-positive timeout | `if not isinstance(...) or max_execution_time <= 0: raise ValueError(...)` | `lib/crewai/src/crewai/agent/utils.py:304-317` |
| Sync execution timeout via `ThreadPoolExecutor` | `_execute_with_timeout(...) -> TimeoutError` | `lib/crewai/src/crewai/agent/core.py:857-890` |
| Async execution timeout via `asyncio.wait_for` | `_aexecute_with_timeout(...) -> TimeoutError` | `lib/crewai/src/crewai/agent/core.py:986-1012` |
| Timeout error message includes hint to raise cap | `f"Task '{task.description}' execution timed out after {timeout} seconds. Consider increasing max_execution_time or optimizing the task."` | `lib/crewai/src/crewai/agent/core.py:885-887`, `lib/crewai/src/crewai/agent/core.py:1009-1011` |
| Iteration cap check helper | `has_reached_max_iterations(iterations, max_iterations) -> bool` (returns `iterations >= max_iterations`) | `lib/crewai/src/crewai/utilities/agent_utils.py:280-290` |
| Forced final-answer LLM call on exhaustion | `handle_max_iterations_exceeded(...) -> AgentFinish` (issues one more LLM call after injecting `force_final_answer`) | `lib/crewai/src/crewai/utilities/agent_utils.py:293-350` |
| Forced-final-answer prompt string | `translations/en.json:47` | `lib/crewai/src/crewai/translations/en.json:47` |
| Loop pre-check, ReAct text path | `if has_reached_max_iterations(self.iterations, self.max_iter): formatted_answer = handle_max_iterations_exceeded(...)` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:343-352` |
| Loop pre-check, native-tool path | `if has_reached_max_iterations(self.iterations, self.max_iter): formatted_answer = handle_max_iterations_exceeded(...)` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:503-513` |
| Loop pre-check, async ReAct | `_ainvoke_loop_react` calls `has_reached_max_iterations` then `handle_max_iterations_exceeded` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1180-1189` |
| Loop pre-check, async native tools | `_ainvoke_loop_native_tools` calls `has_reached_max_iterations` then `handle_max_iterations_exceeded` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1326-1336` |
| Iteration counter increments in `finally` | `finally: self.iterations += 1` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:594-595`, `1416-1417` |
| Safety RuntimeError if loop exits without `AgentFinish` | `raise RuntimeError("Agent execution ended without reaching a final answer. ...")` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1300-1306` |
| LiteAgent loop pre-check | `_invoke_loop` checks `has_reached_max_iterations(self._iterations, self.max_iterations)` | `lib/crewai/src/crewai/lite_agent.py:875-883` |
| Experimental Flow executor check | `@router(or_(initialize_reasoning, continue_iteration)) def check_max_iterations(...)` returns `"force_final_answer"` | `lib/crewai/src/crewai/experimental/agent_executor.py:2112-2123` |
| Flow forced-final-answer route | `@router("force_final_answer") def ensure_force_final_answer(...) -> "agent_finished"` calls `handle_max_iterations_exceeded` | `lib/crewai/src/crewai/experimental/agent_executor.py:1354-1376` |
| Flow method-call recursion cap (100) | `if count > self.max_method_calls: raise RecursionError(...)` | `lib/crewai/src/crewai/flow/runtime/__init__.py:857`, `3178-3185` |
| Flow recursion-error message | `f"Method '{listener_name}' has been called {self.max_method_calls} times ... indicates an infinite loop ..."` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3179-3185` |
| A2A turn loop bound | `for turn_num in range(ctx.max_turns): ... return _handle_max_turns_exceeded(...)` | `lib/crewai/src/crewai/a2a/wrapper.py:1272`, `1404-1409` |
| A2A exceeded handler | `_handle_max_turns_exceeded(...)` raises `Exception(f"A2A conversation exceeded maximum turns ({max_turns})")` | `lib/crewai/src/crewai/a2a/wrapper.py:761-819` |
| A2A turn-warning injection at the cap | Prompt injection: `turn_count >= max_turns` => "This is the FINAL turn..." | `lib/crewai/src/crewai/a2a/wrapper.py:713-728` |
| A2A polling timeout / max-polls cap | `raise A2APollingTimeoutError(f"Max polls ({max_polls}) exceeded after {elapsed:.1f}s")` | `lib/crewai/src/crewai/a2a/updates/polling/handler.py:110-118` |
| Crew-level RPM controller | `RPMController(max_rpm=self.max_rpm, ...)` wired to every agent | `lib/crewai/src/crewai/crew.py:634`, `746-752` |
| RPMController enforces pause (blocks thread) | `_wait_for_next_minute` does `time.sleep(60)` then resets counter | `lib/crewai/src/crewai/utilities/rpm_controller.py:73-75` |
| RPM limiter applied each iteration | `enforce_rpm_limit(self.request_within_rpm_limit)` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:354`, `515`, `603`, `1191`, `1338`, `1425` |
| LiteAgent RPM limiter | `enforce_rpm_limit(self.request_within_rpm_limit)` inside `_invoke_loop` | `lib/crewai/src/crewai/lite_agent.py:885` |
| Tool-call parsing-attempt cap | `if self._run_attempts > self._max_parsing_attempts:` (default 3) | `lib/crewai/src/crewai/tools/tool_usage.py:100-101`, `426`, `666`, `873` |
| Per-tool usage-limit | `_check_usage_limit(tool, tool_name)` honors `tool.max_usage_count` | `lib/crewai/src/crewai/tools/tool_usage.py:740-757` |
| Repeated-tool-call detection | `_check_tool_repeated_usage(calling)` compares tool+args to `last_used_tool` | `lib/crewai/src/crewai/tools/tool_usage.py:728-738` |
| Repeated-usage error string | `"I tried reusing the same input, I must stop using this action input. I'll try something else instead.\n\n"` | `lib/crewai/src/crewai/translations/en.json:49` |
| Repeated-usage telemetry counter | `self._telemetry.tool_repeated_usage(...)` | `lib/crewai/src/crewai/tools/tool_usage.py:238-249`, `476-482` |
| Per-plan replan cap | `max_replans: int = Field(default=3, ...)` in `PlanningConfig` | `lib/crewai/src/crewai/agent/planning_config.py:122-126` |
| Replan cap enforced | `if self.state.replan_count >= max_replans: return "all_todos_complete"` | `lib/crewai/src/crewai/experimental/agent_executor.py:984-992` |
| Per-step iteration cap | `max_step_iterations: int = Field(default=15, ...)` in `PlanningConfig` | `lib/crewai/src/crewai/agent/planning_config.py:127-134` |
| Per-step wall-clock timeout | `step_timeout: int \| None = Field(default=None, ...)` | `lib/crewai/src/crewai/agent/planning_config.py:135-142` |
| Step-level time check inside loop | `for _ in range(max_step_iterations): if step_timeout and start_time: elapsed >= step_timeout: return ...` | `lib/crewai/src/crewai/agents/step_executor.py:336-340`, `546-553` |
| Per-task guardrail retry cap | `max_attempts = self.guardrail_max_retries + 1` then `raise Exception("Task failed ... after ... retries")` | `lib/crewai/src/crewai/task.py:1262-1301` |
| Tool-error retry counter | `self.task.increment_tools_errors()` increments `task.tools_errors` | `lib/crewai/src/crewai/task.py:142`, `1063-1065` |
| RPM controller shutdown on finish | `_finish_execution` calls `_rpm_controller.stop_rpm_counter()` | `lib/crewai/src/crewai/crew.py:2084-2086` |
| Agent-level RPM controller shutdown | `self._rpm_controller.stop_rpm_counter()` on completion | `lib/crewai/src/crewai/agent/core.py:659` |
| CrewOutput exposes termination data | `CrewOutput` carries `tasks_output`, `token_usage`, `raw` (no `finish_reason`/`iterations` field) | `lib/crewai/src/crewai/crews/crew_output.py:13-28` |
| LiteAgentOutput surfaces termination metrics | `LiteAgentOutput` exposes `replan_count`, `last_replan_reason`, `todos` (status=completed/failed) | `lib/crewai/src/crewai/lite_agent_output.py:30-59` |
| Replan observability event | `PlanReplanTriggeredEvent(replan_count=self.state.replan_count, ...)` | `lib/crewai/src/crewai/experimental/agent_executor.py:998-1010` |
| Agent-completion lifecycle event | `AgentExecutionCompletedEvent(agent, task, output)` | `lib/crewai/src/crewai/agent/core.py:673-678`, `lib/crewai/src/crewai/events/types/agent_events.py:35-41` |
| Timeout lifecycle event | `AgentExecutionErrorEvent(agent, task, error=str(e))` on `TimeoutError` | `lib/crewai/src/crewai/agent/core.py:842-851`, `970-980` |
| Step executor timeout return value | `return last_tool_result or f"Step timed out after {elapsed:.0f}s"` (graceful, not an exception) | `lib/crewai/src/crewai/agents/step_executor.py:340` |
| Test: max_iter forces single forced final call | `test_agent_custom_max_iterations`: `max_iter=1` => exactly 2 LLM calls | `lib/crewai/tests/agents/test_agent.py:405-444` |
| Test: max_iter terminates loop | `test_agent_max_iterations_stops_loop`: `max_iter=2` => iterations <= max_iter+2 | `lib/crewai/tests/agents/test_agent.py:447-481` |
| Test: max_execution_time raises TimeoutError | `test_task_with_max_execution_time_exceeded` asserts `pytest.raises(TimeoutError)` | `lib/crewai/tests/test_task.py:1564-1595` |
| Test: routing to `force_final_answer` at cap | `test_exceeded_routes_to_force_final_answer` | `lib/crewai/tests/agents/test_agent_executor.py:2127-2135` |
| Test: async respects max_iter | `test_ainvoke_loop_respects_max_iterations` | `lib/crewai/tests/agents/test_async_agent_executor.py:165-199` |
| Test: RPM throttling observation | `test_agent_respect_the_max_rpm_set` | `lib/crewai/tests/agents/test_agent.py:511-544` |
| Test: A2A polling completes | `test_polling_completes_task` (no max-polls test) | `lib/crewai/tests/a2a/test_a2a_integration.py:64` |

## Answers to Dimension Questions

1. **What stops the loop?**
   - The agent-level ReAct / native-tool loop stops when (a) the LLM emits a parseable `AgentFinish`, (b) `iterations >= max_iter`, or (c) `max_execution_time` elapses around the whole task (`agent/core.py:834-851`).
   - On `max_iter` exhaustion, the executor does **not** terminate immediately. It injects the `force_final_answer` instruction (`translations/en.json:47`) and makes **one** additional LLM call that is expected to produce the final answer (`utilities/agent_utils.py:293-350`).
   - The Flow-based `AgentExecutor` routes the cap to `ensure_force_final_answer` (`experimental/agent_executor.py:1354-1376`) and then to `agent_finished`.
   - A2A delegation stops after `ctx.max_turns` iterations (`a2a/wrapper.py:1272`) and Flow method dispatch stops with `RecursionError` after `max_method_calls` hits 100 by default (`flow/runtime/__init__.py:3179-3185`).
   - The StepExecutor (per-plan-step) loop stops on a parseable final answer, on `max_step_iterations` (default 15, `agent/planning_config.py:127-134`), or on `step_timeout` (`agents/step_executor.py:336-340`).
   - The guardrail retry loop stops at `guardrail_max_retries` (default 3, `task.py:1262-1301`) and raises an explicit `Exception`.

2. **Are limits configurable?**
   - Yes. `BaseAgent.max_iter` (default 25) and `BaseAgent.max_rpm` (`agents/agent_builder/base_agent.py:275-288`); `Agent.max_execution_time` (`agent/core.py:203-206`); `LiteAgent.max_iterations` and `LiteAgent.max_execution_time` (`lite_agent.py:222-227`); `Crew.max_rpm` (`crew.py:296-302`); `PlanningConfig.max_step_iterations`, `PlanningConfig.step_timeout`, `PlanningConfig.max_replans`, `PlanningConfig.max_steps` (`agent/planning_config.py:122-142`); `A2AConfig.max_turns` (`a2a/config.py:397`); `Flow.max_method_calls` (`flow/flow_definition.py:214-218`); `Task.guardrail_max_retries`.
   - Validation: `validate_max_execution_time` rejects non-positive integers (`agent/utils.py:304-317`). RPM is integer-typed and `None`-tolerant.
   - Defaults differ across paths (Agent=25, LiteAgent=15, StepExecutor=15) which is a consistency gap.

3. **Is exhaustion treated differently from success?**
   - On success: `AgentFinish` returned from normal LLM call; loop exits the `while` condition `not isinstance(formatted_answer, AgentFinish)` (e.g. `agents/crew_agent_executor.py:341`).
   - On exhaustion: still returns an `AgentFinish` via `handle_max_iterations_exceeded`, but the LLM is asked specifically to stop and produce its best answer, and the iteration count and forced-call are visible in stdout (`utilities/agent_utils.py:314-318`).
   - On `max_execution_time`: a `TimeoutError` is raised and emitted as `AgentExecutionErrorEvent` (`agent/core.py:842-851`); the caller gets an exception, not a wrapped `AgentFinish`.
   - On `max_method_calls` (Flow): a `RecursionError` is raised (`flow/runtime/__init__.py:3180-3185`).
   - On `max_turns` (A2A): an `Exception` is raised (`a2a/wrapper.py:819`).
   - Net: agent-level exhaustion collapses to `AgentFinish` (graceful), wall-clock/A2A/Flow exhaustion surfaces as exceptions (less graceful).

4. **Are stuck loops detected before the hard limit?**
   - Only at the tool-call level. `_check_tool_repeated_usage` detects when the agent issues the same tool with the same arguments as the previous call and substitutes the `"I tried reusing the same input, I must stop using this action input..."` instruction string (`tools/tool_usage.py:728-738`, `translations/en.json:49`). Telemetry counter `tool_repeated_usage` is incremented (`tools/tool_usage.py:244-248`).
   - At the LLM-response level, there is **no** stuck-detection. The system relies on the LLM eventually emitting `Final Answer:` or hitting `max_iter`. The `experimental/evaluation/metrics/reasoning_metrics.py:308` defines a `LOOP` category, but it is an offline evaluator, not a runtime guard.
   - In the Flow runtime, `max_method_calls` catches infinite-router cycles with an explicit message naming the cause (`flow/runtime/__init__.py:3179-3185`).

5. **Does the user get a useful final state?**
   - On agent-iteration exhaustion: yes. `handle_max_iterations_exceeded` produces a best-effort `AgentFinish` via one forced LLM call (`utilities/agent_utils.py:293-350`). The agent's previous best intermediate answer is included verbatim plus the `force_final_answer` prompt. Test `test_agent_custom_max_iterations` verifies this behavior (`tests/agents/test_agent.py:405-444`).
   - On `max_execution_time`: no, the user gets a `TimeoutError` with a hint to raise the cap (`agent/core.py:885-887`).
   - `CrewOutput` does **not** include a structured `finish_reason` field; only `raw`, `tasks_output`, `token_usage` (`crews/crew_output.py:13-28`). `LiteAgentOutput` exposes richer termination data (`replan_count`, `last_replan_reason`, todos with status, `usage_metrics`) (`lite_agent_output.py:30-59`).
   - Lifecycle events (`AgentExecutionStartedEvent`, `AgentExecutionCompletedEvent`, `AgentExecutionErrorEvent`) are emitted for observability (`events/types/agent_events.py:17-66`).
   - `force_final_answer_error` translation exists (`translations/en.json:46`) suggesting a known error mode where forced final-answer parsing still fails.

## Architectural Decisions

- **Iteration cap with forced-final-answer as a backstop.** CrewAI's primary termination is the `max_iter` count, but it is paired with a deterministic `force_final_answer` LLM call so the user almost always gets an `AgentFinish`. This is implemented as a reusable helper `handle_max_iterations_exceeded` shared by every loop variant (`utilities/agent_utils.py:293-350`, `experimental/agent_executor.py:1354-1376`, `crew_agent_executor.py:343-352`).
- **Wall-clock timeout at agent boundary, not at iteration boundary.** `max_execution_time` wraps the *entire* `_execute_without_timeout` call in a `ThreadPoolExecutor`/`asyncio.wait_for` (`agent/core.py:857-912`). A single hung LLM call can consume the whole budget; there is no per-LLM-call timeout that would let a long iteration budget subdivide.
- **RPM throttling as a synchronization primitive.** `RPMController` uses `threading.Lock` + `threading.Timer` + a blocking `time.sleep(60)` (`utilities/rpm_controller.py:73-75`). This is cooperative, not preemptive.
- **Two executor classes with the same termination semantics.** `CrewAgentExecutor` is the legacy `while`/`if` loop (`agents/crew_agent_executor.py:340-468`); `AgentExecutor` (experimental) is the Flow-based router-based version that shares `BaseAgentExecutor` and the same helper functions (`experimental/agent_executor.py:164-209`, `1354-1376`). Both check the same `has_reached_max_iterations` helper.
- **A2A termination is conversation-shaped.** A2A uses turn-count (`max_turns`) plus prompt-injected warnings at `turn_count == max_turns - 1` and `turn_count >= max_turns` (`a2a/wrapper.py:713-728`). It treats the LLM as the decider about when to stop responding.
- **Flow recursion cap is explicit.** `max_method_calls` (default 100) is a hard counter on each method invocation; the violation produces a `RecursionError` with a message that names the likely cause (`flow/runtime/__init__.py:3179-3185`). This is the only place in the codebase that explicitly says "infinite loop".
- **Termination observability is event-driven.** `AgentExecutionCompletedEvent`, `AgentExecutionErrorEvent`, `PlanReplanTriggeredEvent` are the structured outputs the host application can listen on (`events/types/agent_events.py:35-66`, `experimental/agent_executor.py:998-1010`).

## Notable Patterns

- **Reusable helper trio**: `has_reached_max_iterations` + `handle_max_iterations_exceeded` + `force_final_answer` translation form a consistent contract used by all four executor classes (`utilities/agent_utils.py:280-350`).
- **Fan-out of four termination guards**: iteration counter, wall-clock timeout, RPM, and tool-call repetition detector cover most failure modes separately rather than as one unified guard.
- **Prompt-injected final-turn warnings** (A2A `wrapper.py:713-728`) — the system tells the LLM "this is your last turn" rather than relying purely on a hard cutoff. Mirrors ReAct's pattern of nudging the model to commit.
- **Loop continues on context-length errors** (`crew_agent_executor.py:447-456` calls `handle_context_length` then `continue`) — the loop is only stopped by exhaustion, not by single-call context errors.
- **Iteration counter increments in `finally`**, ensuring the cap reflects completed iterations even on error (`crew_agent_executor.py:594-595`, `1416-1417`).

## Tradeoffs

- **Forced final-answer call adds one LLM call of latency but guarantees an `AgentFinish` even at the cap.** If that call itself fails or returns non-finish, the executor raises `RuntimeError("Agent execution ended without reaching a final answer.")` (`crew_agent_executor.py:1300-1304`) — best-effort, not guaranteed.
- **`max_execution_time` is a thread/async barrier, not a per-iteration budget.** A single hung tool consumes the whole budget; there is no way to "save" budget between iterations.
- **Repeated-tool-call detection is single-step (only compares to `last_used_tool`), not sliding-window.** The agent can rotate among three different tools indefinitely and not be detected.
- **`CrewOutput` does not expose `iterations` or `finish_reason`.** The caller must listen to events or parse the printed logs to know whether the result was a forced-final-answer. `LiteAgentOutput` is richer.
- **RPM is blocking.** A `time.sleep(60)` inside a request thread (`utilities/rpm_controller.py:73-75`) means that under RPM throttling the entire crew pauses, not just one agent.
- **Two parallel executor implementations** (`CrewAgentExecutor` + `experimental.AgentExecutor`). Termination semantics are aligned via shared helpers, but the legacy executor emits a `DeprecationWarning` (`crew_agent_executor.py:145-151`) yet is still in active use by default in some paths.
- **No termination-reason field on `CrewOutput`.** Either success or `AgentExecutionErrorEvent` are the only ways to distinguish outcomes after the fact.

## Failure Modes / Edge Cases

- **`max_iter == 0` semantics**: `has_reached_max_iterations(0, 0)` returns `True` immediately, so the very first loop iteration forces the final-answer path. The user gets the `force_final_answer` prompt as the agent's only response. This is observable but rarely documented.
- **Forced-final-answer LLM call returns invalid JSON / `OutputParserError`**: `handle_max_iterations_exceeded` calls `format_answer(answer)` which can fail; the function raises `ValueError("Invalid response from LLM call - None or empty.")` (`utilities/agent_utils.py:334-340`) — propagated up to the executor.
- **`max_execution_time` with a tool that blocks indefinitely**: the thread-pool executor calls `future.cancel()` after the timeout, but Python cannot cancel a running C-extension tool synchronously (`agent/core.py:884`); the future may continue running in the background.
- **Repeated-tool-check false positives**: `_check_tool_repeated_usage` is purely name + args equality (`tools/tool_usage.py:728-738`). A legitimately-identical read-only query is misclassified as stuck.
- **`max_method_calls == 100` default**: arbitrary number. A legitimate flow with many conditional branches can hit this in non-pathological execution.
- **Polling vs streaming A2A backoff**: `polling/handler.py:115-118` raises `A2APollingTimeoutError` on `max_polls` or `polling_timeout`; `streaming/handler.py:230` raises on "Recovery exhausted all resubscribe attempts". Both are exceptions with no built-in retry.
- **Human-input mode (`ask_for_human_input`)**: `_handle_regular_feedback` runs an inner `while context.ask_for_human_input:` (`core/providers/human_input.py:256-264`); only empty feedback exits. No cap on human-feedback iterations — a malicious or runaway UI could keep the loop open.
- **Guardrail `None` result**: `_invoke_guardrail_function` raises `Exception("Task guardrail returned None as result. ...")` (`task.py:1276-1278`).
- **RPM controller with `_lock == None`**: falls through to the unguarded path (`utilities/rpm_controller.py:60-64`), used only when `max_rpm is None`. Edge: shutdown race can cancel the timer after `_reset_request_count` schedules it.

## Future Considerations

- Add a `finish_reason`/`iterations_used`/`forced_final_answer: bool` field to `CrewOutput` and to the legacy `TaskOutput` so callers can detect exhaustion without parsing logs.
- Promote repeated-tool detection from single-step to sliding-window (e.g., last N tool calls) to catch rotation-based loops.
- Add a per-iteration LLM-call timeout in addition to the agent-level `max_execution_time` so a single hung LLM call cannot consume the whole budget.
- Consolidate the legacy `CrewAgentExecutor` and `experimental.AgentExecutor` into a single implementation; the dual code paths with shared helpers is a maintenance liability.
- Replace the blocking `time.sleep(60)` RPM strategy with a token-bucket / async-sleep pattern so that throttling does not freeze the whole crew.
- Promote the Flow `RecursionError` messaging to the agent loop: an equivalent "stuck-loop" exception (rather than a forced-final-answer call) when the agent emits the same `AgentAction` repeatedly.
- Extend the A2A `_handle_max_turns_exceeded` to optionally return a structured `MaxTurnsResult` instead of raising a generic `Exception`.
- Add an upper bound on `ask_for_human_input` rounds in `core/providers/human_input.py:256-264`.

## Questions / Gaps

- No clear evidence found that the A2A turn cap is itself covered by a unit test asserting the `Exception` message. The only polling test (`tests/a2a/test_a2a_integration.py:64`) covers the happy path. A dedicated `test_max_turns_exceeded` was searched for and not found.
- No clear evidence found for runtime detection of loops where the LLM emits the same `Final Answer` candidate repeatedly with different phrasings (semantic-loop detection). Only exact-args tool repetition is checked.
- No clear evidence found for a structured `finish_reason` enum (`stop`, `max_iter`, `timeout`, `error`, `forced_final_answer`) on the executor return type. Callers must infer the reason from logs/events.
- No clear evidence found for tests of `Flow.max_method_calls` triggering a `RecursionError` (only `test_config_max_method_calls_from_yaml` checks the config round-trip).
- No clear evidence found for tests of `PlanningConfig.max_replans` actually exhausting and finalizing (the search for `test_max_replans` returned no matches).
- The `RPMController._wait_for_next_minute` does `time.sleep(60)` then sets `_current_rpm = 0` but `_check_and_increment` only increments after the sleep (`utilities/rpm_controller.py:73-75`, `47-57`). A burst of N>M requests still sleeps a full minute before allowing any new request — this is intentional but undocumented.

---

Generated by `01.04-termination-and-loop-bounds` against `crewai`.