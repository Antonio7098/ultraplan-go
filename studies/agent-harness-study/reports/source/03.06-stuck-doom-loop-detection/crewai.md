# Source Analysis: crewai

## 03.06 — Stuck and Doom-Loop Detection

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (crewai 1.x at `lib/crewai/src/crewai/` plus `lib/crewai-core`; ReAct + Plan-and-Execute + Flow runtimes; native tool calling and a LangGraph adapter) |
| Analyzed | 2026-07-15 |

## Summary

CrewAI ships a small but real set of stuck and doom-loop guards, spread across three layers. The ReAct / native-tool executor enforces a hard per-agent iteration cap (default 25) that calls `handle_max_iterations_exceeded` and forces the model to commit a "Final Answer" — this is the only intervention that actually stops the loop. Inside each tool invocation, `ToolUsage._check_tool_repeated_usage` compares the *current* tool call against `ToolsHandler.last_used_tool` (a 1-step window) and, on a byte-identical name + arguments match, injects an i18n "task_repeated_usage" string as the tool result to nudge the LLM off the same action. Per-tool `max_usage_count`, a parsing retry cap (`_max_parsing_attempts = 3`), an agent-level `max_retry_limit`, and a Plan-and-Execute `max_replans` cap provide narrow guards. A Flow runtime recursion counter (`max_method_calls = 100`) stops flow-level infinite routes. There is no detection of alternating A-B-A-B patterns, no detector of repeated error strings, no proactive compaction on loop detection, and the only compaction (`summarize_messages`) is triggered by a context-length error — and it silently erases prior tool evidence by clearing the message list and replacing it with one summary. Net effect: the system will stop the loop, but only *after* the user has paid for up to the full iteration budget (default 25), and only on consecutive identical tool calls, not on the broader patterns a doom-loop detector should catch.

## Rating

**5 / 10** — Present but narrow and fragile. Hard iteration caps exist and force a final answer; one-step "same tool + same args" detection exists; per-tool usage caps exist; replan and recursion caps exist. However: the detection window is 1 step (alternating A-B-A-B passes through), only byte-identical arguments count (LLMs can perturb arguments trivially), no proactive compaction is triggered by loop detection, the post-hoc compaction silently destroys loop evidence, no automatic de-duplication of identical observations, and tests cover the max-iter path but not the repeated-usage detector itself. Lands squarely in the "4–6: present but inconsistent / fragile" band of the rubric.

## Evidence Collected

Every entry includes `path:line`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Hard iteration cap (Crew agent default) | `max_iter: int = Field(default=25, description="Maximum iterations for an agent to execute a task")` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:286-288` |
| Hard iteration cap (executor base) | `max_iter: int = Field(default=25)` on `BaseAgentExecutor` | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:27` |
| Hard iteration cap (LiteAgent) | `max_iterations: int = Field(default=15, description="Maximum number of iterations for tool usage")` | `lib/crewai/src/crewai/lite_agent.py:222-224` |
| Deep flow method-call cap | `self.max_method_calls = self.max_iter * 10` derived in `_setup_executor` | `lib/crewai/src/crewai/experimental/agent_executor.py:229` |
| Cap check helper | `def has_reached_max_iterations(iterations, max_iterations) -> bool: return iterations >= max_iterations` | `lib/crewai/src/crewai/utilities/agent_utils.py:280-290` |
| Cap-exceeded handler (forces Final Answer) | `handle_max_iterations_exceeded(...)` appends i18n `force_final_answer` prompt to messages and runs one more LLM call to synthesize a final answer | `lib/crewai/src/crewai/utilities/agent_utils.py:293-350` |
| i18n forced-answer prompt | `"Now it's time you MUST give your absolute best final answer. You'll ignore all previous instructions, stop using any tools, and just return your absolute BEST Final answer."` | `lib/crewai/src/crewai/translations/en.json:46-47` |
| i18n repeated-usage message | `"task_repeated_usage": "I tried reusing the same input, I must stop using this action input. I'll try something else instead.\n\n"` | `lib/crewai/src/crewai/translations/en.json:49` |
| i18n tool exception message | `"tool_usage_exception"` injected back to LLM after `_max_parsing_attempts` exceeded | `lib/crewai/src/crewai/translations/en.json:53` |
| i18n parser format reminder | `"format_without_tools"` used by `parse()` to push LLM back on format | `lib/crewai/src/crewai/translations/en.json:20` |
| Max-iter gate in ReAct loop | `if has_reached_max_iterations(self.iterations, self.max_iter): formatted_answer = handle_max_iterations_exceeded(...)` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:343-352` |
| Max-iter gate in native-tools loop | Same gate inside `_invoke_loop_native_tools()` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:503-513` |
| Max-iter gate in async + LiteAgent loops | Same gate in async loop and `_invoke_loop` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1180-1181, 1326-1327`; `lib/crewai/src/crewai/lite_agent.py:875-883` |
| Max-iter gate in experimental AgentExecutor (Flow) | `check_max_iterations` router: `if has_reached_max_iterations(self.state.iterations, self.max_iter): return "force_final_answer"` | `lib/crewai/src/crewai/experimental/agent_executor.py:2113-2123` |
| Force-final-answer Flow listener | `ensure_force_final_answer` calls `handle_max_iterations_exceeded` once and sets `state.is_finished = True` to prevent double-trigger | `lib/crewai/src/crewai/experimental/agent_executor.py:1354-1376` |
| Iteration increment | `self.iterations += 1` in `finally:` of every loop pass | `lib/crewai/src/crewai/agents/crew_agent_executor.py:460` |
| Iteration increment (Flow) | `increment_and_continue` listener | `lib/crewai/src/crewai/experimental/agent_executor.py:2236-2244` |
| **Repeated-tool detector** | `_check_tool_repeated_usage`: compares `calling.tool_name + calling.arguments` against `tools_handler.last_used_tool` (window = 1) | `lib/crewai/src/crewai/tools/tool_usage.py:728-738` |
| Repeated-tool firing point (sync) | `_use()` calls the detector and short-circuits with the i18n message | `lib/crewai/src/crewai/tools/tool_usage.py:476-491` |
| Repeated-tool firing point (async) | `_ause()` calls the detector | `lib/crewai/src/crewai/tools/tool_usage.py:238-252` |
| Telemetry for repeated usage | `tool_repeated_usage(llm, tool_name, attempts)` emits a "Tool Repeated Usage" span | `lib/crewai/src/crewai/telemetry/telemetry.py:574-597` |
| Tools-handler last-call slot | `ToolsHandler.last_used_tool: ToolCalling \| InstructorToolCalling \| None = Field(default=None)` | `lib/crewai/src/crewai/agents/tools_handler.py:24` |
| Tools-handler write site | `on_tool_use` sets `self.last_used_tool = calling` after a successful tool run | `lib/crewai/src/crewai/agents/tools_handler.py:26-39` |
| Per-tool usage cap (class) | `ToolUsage._check_usage_limit(tool, tool_name)` enforces `tool.max_usage_count` | `lib/crewai/src/crewai/tools/tool_usage.py:740-757` |
| Per-tool usage counter increment | `_increment_usage_count()` and `current_usage_count += 1` branches | `lib/crewai/src/crewai/tools/tool_usage.py:393-420` |
| Per-tool cap result message | `f"Tool '{tool_name}' has reached its usage limit of {tool.max_usage_count} times and cannot be used anymore."` | `lib/crewai/src/crewai/tools/tool_usage.py:756` |
| Parsing retry cap | `_max_parsing_attempts: int = 3` (2 for OPENAI_BIGGER_MODELS) | `lib/crewai/src/crewai/tools/tool_usage.py:101, 118` |
| Agent error retry cap | `max_retry_limit: int = Field(default=2)` on `Agent`; `_check_execution_error` re-raises after `_times_executed > max_retry_limit` | `lib/crewai/src/crewai/agent/core.py:247-250, 707-708` |
| Parser-error handler | `handle_output_parser_exception(...)` returns the parser error as a user-role message; `log_error_after = 3` only suppresses logs, not the cap | `lib/crewai/src/crewai/utilities/agent_utils.py:660-695` |
| Plan-and-Execute replan cap | `PlanningConfig.max_replans: int = Field(default=3)` | `lib/crewai/src/crewai/agent/planning_config.py:122-126` |
| Plan-and-Execute replan counter | `replan_count: int = Field(default=0)` on `AgentExecutorState` | `lib/crewai/src/crewai/experimental/agent_executor.py:148-150` |
| Replan guard | `if self.state.replan_count >= max_replans: return "all_todos_complete"` | `lib/crewai/src/crewai/experimental/agent_executor.py:984-992, 2463-2467, 2660-2667` |
| Replan trigger heuristic | `len(failed_todos) >= 2` OR `len(error_todos) >= 2` OR agent emits replan-indicator substrings ("replan", "try a different approach", …) | `lib/crewai/src/crewai/experimental/agent_executor.py:2469-2502` |
| Step-level iteration cap | `StepExecutor.execute(..., max_step_iterations: int = 15)`; inner `for _ in range(max_step_iterations):` | `lib/crewai/src/crewai/agents/step_executor.py:130, 336` |
| Step-level timeout | `step_timeout: int \| None = None` returns last tool result or `f"Step timed out after {elapsed:.0f}s"` on expiry | `lib/crewai/src/crewai/agents/step_executor.py:131, 337-340` |
| Agent-level wall-clock timeout | `Agent.max_execution_time: int \| None = Field(default=None)` raises `TimeoutError` via `ThreadPoolExecutor.future.result(timeout=...)` | `lib/crewai/src/crewai/agent/core.py:203-206, 834-887` |
| Recursion guard in Flow runtime | `_method_call_counts[listener_name] += 1; if count > self.max_method_calls: raise RecursionError("Method '...' has been called ... which indicates an infinite loop.")` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3178-3186` |
| Flow method-call cap default | `max_method_calls: int = Field(default=100)` | `lib/crewai/src/crewai/flow/runtime/__init__.py:857` |
| Tool-result caching (used as a de-dup hint) | `CacheHandler.add/read` keyed by `tool_name + input_str`; on hit, `_use()`/`_ause()` skips execution and returns the cached result — but this does NOT block the loop | `lib/crewai/src/crewai/agents/tools_handler.py:40-51`; `lib/crewai/src/crewai/tools/tool_usage.py:283-294, 523-531` |
| Compaction (post-context-window only) | `summarize_messages(messages, llm, ...)` clears all non-system messages and replaces with a single `<summary>...</summary>` message | `lib/crewai/src/crewai/utilities/agent_utils.py:920-1003` |
| Compaction is triggered ONLY on context-length error | `if respect_context_window: summarize_messages(...)` is reached from `handle_context_length`, called when `is_context_length_exceeded(e)` | `lib/crewai/src/crewai/utilities/agent_utils.py:712-748, 698-709` |
| Compaction is NOT triggered by loop detection | `has_reached_max_iterations` and `_check_tool_repeated_usage` do not invoke `summarize_messages`; no code path connects them | (no evidence found beyond the absence above) |
| ReAct parser state error | `OutputParserError` raised when LLM output lacks `Action:` / `Action Input:` markers; surfaced as user-role error to LLM | `lib/crewai/src/crewai/agents/parser.py:45-128` |
| Finalization lock (prevents duplicate forced answer) | `_finalize_lock` / `_finalize_called` ensures `finalize()` runs once | `lib/crewai/src/crewai/experimental/agent_executor.py:213-217, 2266-2269` |
| ReasoningPatternType.LOOP (post-hoc) | `ReasoningEfficiencyEvaluator._calculate_loop_likelihood` scores call-length repetition post-execution; not a runtime guard | `lib/crewai/src/crewai/experimental/evaluation/metrics/reasoning_metrics.py:35-41, 273-329, 351-383` |
| A2A poll count event (observability only) | `A2APollingStatusEvent.poll_count` is emitted each poll — does not cap | `lib/crewai/src/crewai/events/types/a2a_events.py:275-298` |
| Max-iter routing test | `TestMaxIterationsRouting` exercises `check_max_iterations` routes for under-limit / over-limit / native-tools | `lib/crewai/tests/agents/test_agent_executor.py:2120-2156` |
| Max-iter end-to-end test | `test_agent_max_iterations_stops_loop` (VCR-recorded) | `lib/crewai/tests/agents/test_agent.py:447-481` |
| "Moved on after max_iter" test | `test_agent_moved_on_after_max_iterations` (VCR) | `lib/crewai/tests/agents/test_agent.py:484-508` |
| Async max-iter test | `test_ainvoke_loop_respects_max_iterations` mocks `handle_max_iterations_exceeded` | `lib/crewai/tests/agents/test_async_agent_executor.py:165-208` |
| Tool-usage limit test (mocked) | `test_tool_usage_with_toolusage_class` mocks `_check_tool_repeated_usage` to False to isolate cap behavior | `lib/crewai/tests/tools/test_tool_usage_limit.py:109-151` |
| **No test found** for `_check_tool_repeated_usage` directly | Only a mock-stab above; no positive or negative detection test in the repo | (no evidence found) |

## Answers to Dimension Questions

1. **What stuck patterns are detected?**
   - **Hard iteration cap**: consecutive iterations, period — counted by `self.iterations` per loop body, capped at `max_iter` (`agents/crew_agent_executor.py:343, 503, 1180, 1326`; `experimental/agent_executor.py:2119`; `lite_agent.py:875`).
   - **One-step identical tool repeat**: only when the *current* `ToolCalling` has byte-identical `tool_name` *and* `arguments` to `tools_handler.last_used_tool` (`tools/tool_usage.py:728-738`). The check happens in `_use`/`_ause` *before* the tool runs and short-circuits with an i18n string asking the LLM to do something else (`tools/tool_usage.py:476-491, 238-252`).
   - **Per-tool usage cap**: `tool.max_usage_count` is enforced by `_check_usage_limit`, returning a hard error string back to the LLM (`tools/tool_usage.py:740-757`).
   - **Replan pressure**: ≥2 failed todos, ≥2 error todos, or LLM emits one of seven replan-indicator substrings (`experimental/agent_executor.py:2469-2502`).
   - **Parser retry**: `_max_parsing_attempts = 3` retries before yielding a "tool_usage_exception" message (`tools/tool_usage.py:101, 118, 426-446`).
   - **Flow recursion**: a hard `max_method_calls = 100` per listener raises `RecursionError` (`flow/runtime/__init__.py:857, 3178-3186`).
   - **Not detected**: alternating A-B-A-B tool patterns, identical observations that don't go through a tool, no-progress monologues where the LLM keeps "thinking" without calling a tool, repeated identical error responses, repeated identical user prompts, and any post-compaction deduplication of tool calls.

2. **How far back does detection look?**
   - **Window = 1** for `_check_tool_repeated_usage`: only the immediately previous tool call is retained (`agents/tools_handler.py:24, 39`). Anything two steps back is forgotten.
   - `Tasks.tools_errors` and `Task.used_tools` are unbounded counters (`task.py:141-142`), but they are never read by a guard — only `tools_errors` is incremented on errors and `used_tools % _remember_format_after_usages == 0` is used to *inject* a tools-list reminder into the result, not to stop the loop (`tools/tool_usage.py:716-718`).
   - The hard `max_iter` counter is the only monotonically increasing state that is actually checked, and it is the only state that crosses the 1-step window.

3. **Does compaction erase loop evidence?**
   - Yes, but only on context-window overflow. `summarize_messages` does `messages.clear()` and `messages.extend(system_messages)` plus a single summary message (`utilities/agent_utils.py:995-1003`). Every prior tool call and observation is replaced by one LLM-written `<summary>…</summary>` block. After compaction, `tools_handler.last_used_tool` still holds the *most recent* tool call object, but `_check_tool_repeated_usage` only fires when the agent emits another identical call — and after a fresh re-summarize the LLM typically *won't* emit an identical call (it has been told the summary is the new state), so detection effectively resets without ever triggering the stop. Compaction is **never** triggered by the loop detector itself; it is reached only from `handle_context_length` after `is_context_length_exceeded(e)` (`utilities/agent_utils.py:712-748, 698-709`).

4. **What intervention happens?**
   - On hitting `max_iter`: `handle_max_iterations_exceeded` appends the i18n `force_final_answer` prompt, runs one more LLM call, parses the response, and returns an `AgentFinish` (`utilities/agent_utils.py:293-350`). In the experimental AgentExecutor Flow, this routes to `force_final_answer → ensure_force_final_answer` which performs the same call and sets `state.is_finished = True` to prevent re-entry (`experimental/agent_executor.py:1354-1376`).
   - On identical consecutive tool call: an i18n string is returned *as if* it were the tool's output. The next LLM turn can still call the same tool with different arguments and the check passes; there is no escalation (no compaction, no replan, no cap increment, no event emission beyond `tool_repeated_usage` telemetry).
   - On per-tool cap: same pattern — a fixed error message becomes the tool result (`tools/tool_usage.py:756`).
   - On replan cap (`max_replans`): the Flow routes to `all_todos_complete`, which proceeds to finalize rather than re-plan (`experimental/agent_executor.py:986-992, 2660-2667`).
   - On step timeout: `StepExecutor` returns `last_tool_result or f"Step timed out after {elapsed:.0f}s"` — the planner observes this as a failed step (`agents/step_executor.py:337-340`).
   - On agent-level timeout: a `TimeoutError` is raised and emitted as `AgentExecutionErrorEvent` (`agent/core.py:842-851, 881-887`).
   - On parser failures after `_max_parsing_attempts`: the next turn receives the `tool_usage_exception` message as the tool result, and the loop continues — there is no automatic stop.
   - On Flow recursion: `RecursionError` propagates out of the listener invocation (`flow/runtime/__init__.py:3179-3185`).

5. **Are false positives possible?**
   - For the iteration cap: only if `max_iter` is set too low for the real task. The default 25 is generous and the LiteAgent default 15 less so.
   - For `_check_tool_repeated_usage`: yes, in the "intended loop" direction — any legitimate task that requires N≥2 identical tool calls (e.g., polling a status, paginating, repeated lookups) will trip the check on every call after the first. The returned i18n message *suggests* the LLM change its action, but if the LLM ignores the suggestion and re-emits the same args, the cycle repeats on every iteration. The detection is therefore *also* a fragility: it can mislead a correct agent into unnecessary behavior changes.
   - For per-tool `max_usage_count`: false positives only if the tool author mis-set the limit; there is no runtime override.
   - For the replan keyword heuristic: substring matches on the LLM's own message — false positives are easy (any sentence containing "replan" or "approach isn't working" in normal conversation can trigger it).
   - For Flow recursion: false positives only if the author wrote a @listen that legitimately re-fires many times; the cap is 100 which is very generous.

## Architectural Decisions

- **Hard cap is the dominant safety net.** Across ReAct, native-tool, async, LiteAgent, and experimental Flow executors, every loop has a single `if has_reached_max_iterations(...)` guard wired to `handle_max_iterations_exceeded`. There is no other path that stops the executor from continuing.
- **Repeated-tool detection lives in `ToolUsage`, not the executor.** The executor is agnostic; the detector sits in the tool-dispatch layer and only fires when the same tool is called twice in a row with the same args. This makes it cheap (one comparison) but blind to longer-window patterns.
- **ToolsHandler holds exactly one prior tool call.** The handler's `last_used_tool` field is overwritten on every successful tool use (`agents/tools_handler.py:39`). The window is therefore 1, by design.
- **Detection injects a message, never raises.** Repeated usage, parser errors, and per-tool caps all feed strings back to the LLM as the "tool result". The agent loop is never short-circuited by these — only by the `max_iter` boundary. This keeps the executor in charge of termination while letting the LLM self-correct.
- **Compaction is decoupled from loop detection.** `summarize_messages` exists and is fully implemented, but the only caller is `handle_context_length`. There is no "we are looping, summarize now" trigger; the two systems do not communicate.
- **Plan-and-Execute adds a second-level cap.** Replans are bounded by `max_replans = 3`, and *every* replan guard checks `if self.state.replan_count >= max_replans: return "all_todos_complete"`. This caps *plan-regeneration* loops, which are a different failure mode than inner tool-call loops.
- **Flow runtime treats recursion as a programmer error.** The `RecursionError` raised at `flow/runtime/__init__.py:3179` is loud and ungraceful — the flow aborts. This is consistent with Flow being a developer-facing orchestration primitive, not a runtime guard.
- **`CrewAgentExecutor` is deprecated.** Its constructor raises a `DeprecationWarning` pointing users at `AgentExecutor` (`agents/crew_agent_executor.py:145-151`). The two implementations coexist; both wire the same `has_reached_max_iterations` / `handle_max_iterations_exceeded` utilities.

## Notable Patterns

- **Centralized utilities, duplicated wiring.** `has_reached_max_iterations` and `handle_max_iterations_exceeded` are defined once in `utilities/agent_utils.py:280-350` and re-imported in four executors. Each executor wires them differently: ReAct uses a `while` loop with a `finally` increment, the experimental Flow uses a router+listener pair with explicit state, LiteAgent uses a plain `while`. The pattern is "one utility, many adapters."
- **i18n-driven intervention messages.** All force-stop, repeated-usage, parser-error, and tool-error strings live in `translations/en.json:46-55` and are loaded through `I18N_DEFAULT`. Localized agents get localized loop interventions for free.
- **Telemetry-only signal for some loop events.** `Telemetry.tool_repeated_usage` opens an OTel span when the detector fires (`telemetry/telemetry.py:574-597`). This is observability, not intervention.
- **Finalization locks.** `_finalize_lock` + `_finalize_called` (`experimental/agent_executor.py:213-217, 2266-2269`) guarantee that `finalize()` runs exactly once. This is what keeps the forced-final-answer Flow path from firing twice when multiple terminal branches converge.
- **Tool-level idempotency via cache.** `ToolsHandler.cache.read/add` keys by `(tool_name, input_str)` so identical inputs return cached outputs without re-running the tool (`agents/tools_handler.py:26-51`). This is *de facto* deduplication of side-effect-free tools, but is not framed as a loop-detection mechanism.
- **`_remember_format_after_usages` is a UX nudge, not a guard.** Every N tool calls (default 3), the tool result is suffixed with the tools-list slice so the LLM remembers what tools are available (`tools/tool_usage.py:716-726`). This reduces the chance the agent loses track of available tools mid-loop, but does not detect stuckness.
- **`should_remember_format` writes through `task.used_tools`** — the counter is task-scoped and unbounded, but never read by a guard.

## Tradeoffs

- **Cheap detection vs. robust detection.** `_check_tool_repeated_usage` is O(1) per tool call and lives in the hot path. A more powerful detector (e.g., n-gram of last 5 tool calls, embedding-similarity check, observation-hash repeat) would add latency and dependency surface.
- **Cap-then-force vs. escalate.** `handle_max_iterations_exceeded` does *not* summarize before forcing the final answer. The final LLM call is asked to produce an answer from the full message list, which can itself overflow the context window. The fix would be to summarize first.
- **Per-tool cap vs. global cap.** `max_usage_count` is per-tool, not global. An agent can still loop across *different* tools indefinitely as long as no single tool hits its cap.
- **Telemetry without action.** Recording `tool_repeated_usage` is observation, not policy. There is no way for a downstream monitoring system to act on it other than out-of-band human review.
- **Compaction that loses state.** `summarize_messages` is lossy: the summary may omit specific tool arguments, IDs, or error messages, and the LLM summary is heuristic. After compaction, repeated-usage detection still works (it only needs `last_used_tool`), but the LLM's own awareness of "why am I here" is reduced.
- **Replan heuristics vs. structured detection.** The replan-trigger keywords (`"replan"`, `"try a different approach"`, etc.) are substring matches on the LLM's own free text (`experimental/agent_executor.py:2490-2500`). This is brittle but cheap; a structured detector would require a separate LLM call or schema.
- **Default `max_iter=25` is on the high side.** With each "iteration" being at minimum one LLM call plus one tool call, 25 iterations can mean 25 model round-trips before stopping. There is no lower default for high-cost models.
- **`max_method_calls = max_iter * 10 = 250`** is a much deeper guard but it lives at the Flow routing layer, not the agent executor. It catches flow cycles but not agent-internal repetition.

## Failure Modes / Edge Cases

- **The LLM can dodge the repeated-usage detector by perturbing arguments.** Adding a trailing space, a different key order, a redundant `null` field, or a slight string change makes `arguments == last_tool_usage.arguments` False. The check uses `dict == dict` (`tools/tool_usage.py:737`) which is structural; the LLM can perturb structure trivially.
- **The repeated-usage detector does not fire for native tool calls without going through `_use`/`_ause`.** Native tools take a different path in `_invoke_loop_native_tools` (`agents/crew_agent_executor.py:484+`) and the repeated-usage check is *not* invoked there (it lives inside `ToolUsage._use/_ause`). For native-tool loops, only the `max_iter` cap protects.
- **Compaction triggers a "reset" without firing detection.** After `summarize_messages`, `tools_handler.last_used_tool` still holds the most recent call, but the message list is short. If the LLM immediately re-emits the same tool+args (because the summary mentioned the previous attempt), the detector *will* fire — but the LLM has just been told to summarize, so it is more likely to invent a different approach and not trip the check.
- **`log_error_after = 3` is a logging threshold, not a stop.** It suppresses error spam but never escalates (`utilities/agent_utils.py:689-693`).
- **`OutputParserError` recovery has no bound.** The error is appended as a user-role message and the loop continues. There is no parser-retry counter; if the LLM keeps producing malformed output, only `max_iter` saves the agent.
- **The 1-step window forgets identical patterns separated by anything else.** Calling `search("foo") → read_file(...) → search("foo")` will *not* trigger repeated-usage detection because `last_used_tool` was overwritten by `read_file`.
- **A2A polling events have a `poll_count` field but no max-poll cap in this code path.** Polling loops are unbounded at the agent layer; any cap lives in the A2A server, not crewai.
- **Per-tool `max_usage_count` validation rejects negative numbers but not zero or missing values are handled inconsistently.** `validate_max_execution_time` raises `ValueError` for negatives (`agent/utils.py:304-314`); analogous validation for tool usage caps lives in `BaseTool` (referenced in `test_tool_usage_limit.py:72-82`). Zero vs. None semantics differ across tools.
- **`ReasoningEfficiencyEvaluator._calculate_loop_likelihood`** is post-hoc — it cannot stop a running agent, only label past traces. It would not help during execution.

## Future Considerations

- **Window-of-N repeated-tool detector.** A deque of the last N tool calls and arguments, with a threshold for consecutive repeats, would catch A-B-A-B patterns that the current 1-step window misses.
- **Observation-hash de-dup.** Hashing the tool *result* (or the assistant message after it) and detecting `K` consecutive identical hashes would catch "LLM is producing the same text over and over" without depending on the tool layer.
- **Proactive compaction on loop detection.** When `_check_tool_repeated_usage` fires M times within K turns, call `summarize_messages` *before* the next LLM call. The current compaction path is only reached on a context-length error.
- **Lower default `max_iter`** for high-cost models, or expose per-LLM-cost budgeting.
- **Structured replan detection** instead of substring matching — require the agent to emit a structured `needs_replan: true` field, or train a small classifier.
- **Native-tool path should also invoke `_check_tool_repeated_usage`** (or an equivalent). Currently only the text-tool path benefits from repeated-usage detection.
- **Expose `_method_call_counts` and `replan_count` as observable state** for downstream monitors / dashboards.
- **Add a positive test** for `_check_tool_repeated_usage`: assert that calling the same tool with identical args twice yields the i18n message; currently only mocked (`test_tool_usage_limit.py:133`).
- **Add a test** for the "perturb the args to dodge" edge case, to lock in current behavior or motivate a structural comparison (e.g., canonicalize dict ordering, strip None).

## Questions / Gaps

- **Is there an integration test that exercises an actual doom-loop end-to-end?** Searched the test tree; no test simulates an agent that calls the same tool with identical args 20+ times and verifies that the executor stops short of `max_iter`. `test_agent_moved_on_after_max_iterations` (`tests/agents/test_agent.py:484-508`) exercises the cap but not the repeated-usage path.
- **Does `_check_tool_repeated_usage` fire on native-tool calls?** No evidence found that `_invoke_loop_native_tools` calls `ToolUsage._use`/`_ause`. The repeated-usage check appears native-tool-blind.
- **Is the tool-result cache (`ToolsHandler.cache`) consulted before or after the repeated-usage check?** Code in `_use`/`_ause` shows the cache is consulted *after* the repeated-usage short-circuit, but `agents/tools_handler.py` does not show the ordering explicitly — there is no shared cache-then-detect ordering documented.
- **What is the interaction between `_remember_format_after_usages` and stuck detection?** `tools/tool_usage.py:716-718` injects a tools-list reminder every Nth call. Does this help or hurt stuck detection? No evidence found.
- **Are there user-facing knobs to disable or tune `_check_tool_repeated_usage`?** No evidence found in `tools/tool_usage.py:728-738` or in `ToolsHandler`. The detector is unconditional.
- **How is `last_used_tool` cleared between tasks?** `agents/tools_handler.py:24` shows it as `Field(default=None)`, but there is no explicit reset in `ToolsHandler`. If a single `ToolsHandler` instance is reused across tasks, prior-task calls could leak.
- **Does the experimental Flow's `max_method_calls = max_iter * 10` interact correctly with multi-task crews?** Each `AgentExecutor` instance creates its own `_method_call_counts` (`flow/runtime/__init__.py`); cross-task leakage is unlikely but not explicitly documented.