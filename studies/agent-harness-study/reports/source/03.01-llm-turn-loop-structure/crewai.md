# Source Analysis: crewai

## 03.01 - LLM Turn Loop Structure

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic, asyncio, LiteLLM + native provider SDKs) |
| Analyzed | 2026-07-13 |

## Summary

CrewAI implements two reusable LLM turn loops side by side. The legacy
`CrewAgentExecutor._invoke_loop_*` family (deprecated, warns at construction —
`lib/crewai/src/crewai/agents/crew_agent_executor.py:145-151`) is a
straight ReAct-style `while not isinstance(formatted_answer, AgentFinish):`
loop with text-parsed or native-function-calling branches. The default
`crewai.experimental.AgentExecutor` (`lib/crewai/src/crewai/experimental/agent_executor.py:164`)
turns that same primitive into a Flow-based, event-routed state machine
(`@start`, `@router`, `@listen` decorators — `lib/crewai/src/crewai/experimental/agent_executor.py:314,
608, 815, 950, 1378, 1451, 2112, 2125, 2653, 2678`) where a single turn is
"LLM call → parse → route by answer type → tool execution → loop back". A
third nested loop, `StepExecutor.execute` (`lib/crewai/src/crewai/agents/step_executor.py:126`),
runs inside the Plan-and-Execute path of `AgentExecutor` and performs LLM →
tool → observation turn-taking within one outer todo. All three loops share
the same wrapper functions in `crewai/utilities/agent_utils.py`:
`get_llm_response` (line 461), `aget_llm_response` (line 514),
`process_llm_response` (line 567), `has_reached_max_iterations` (line 280),
`handle_max_iterations_exceeded` (line 293), `handle_output_parser_exception`
(line 660), `handle_context_length` (line 712), and `execute_single_native_tool_call`
(line 1399). Every LLM call goes through a `_prepare_llm_call` /
`_validate_and_finalize_llm_response` pre/post pair
(`lib/crewai/src/crewai/utilities/agent_utils.py:401-458`) that wires global
`before_llm_call` and `after_llm_call` hooks
(`lib/crewai/src/crewai/hooks/llm_hooks.py:153-238`); every tool call goes
through `before_tool_call` / `after_tool_call` hooks
(`lib/crewai/src/crewai/hooks/tool_hooks.py`).

Each executor keeps the full conversation in `self.messages` /
`self.state.messages: list[LLMMessage]` and appends after every turn
(`CrewAgentExecutor._append_message` at `crew_agent_executor.py:1511`,
`AgentExecutor._append_message_to_state` at `agent_executor.py:3002`). The
state is a Pydantic `AgentExecutorState` (`agent_executor.py:126-161`) with
`messages`, `iterations`, `current_answer`, `is_finished`, `pending_tool_calls`,
plus todo/plan/observation state. `max_iter` defaults to `25`
(`base_agent_executor.py:27`, `agent_executor.py:187`); the Flow layer
multiplies that by 10 for `max_method_calls`
(`agent_executor.py:229`). The loops cooperate with an event bus
(`crewai_event_bus.emit(...)` calls in `agent_executor.py:1527, 1546, 940-1063,
1113, 1939, 2052, etc.`) — they emit `AgentLogsStartedEvent`,
`AgentLogsExecutionEvent`, `ToolUsageStartedEvent`, `ToolUsageFinishedEvent`,
`ToolUsageErrorEvent`, and (when planning is on) `PlanStepStartedEvent` /
`PlanStepCompletedEvent` / `PlanRefinementEvent` /
`PlanReplanTriggeredEvent` / `GoalAchievedEarlyEvent`.

A turn is "one model call + every tool call(s) executed in response". In
text-parsed ReAct, the executor parses the LLM text into a single
`AgentAction` and runs `execute_tool_and_check_finality`
(`crew_agent_executor.py:415-426, 1253-1264`), then appends the observation
plus a reflection prompt and increments `self.iterations` (line 460, 1298).
In native-function-calling mode with a single tool call, the executor also
processes one tool per turn and re-prompts the LLM with a `post_tool_reasoning`
prompt (lines 535-541, 778-783, 801-806). With multiple native tool calls and
no `result_as_answer`/`max_usage_count` tools, the executor runs them in
parallel via `ThreadPoolExecutor` (`agent_executor.py:742-746, 1714-1745`) and
asks for reflection before the next LLM call. The loop short-circuits when a
tool is marked `result_as_answer` (`crew_agent_executor.py:1097-1106,
agent_executor.py:1773-1784, 1811-1822`) or when the LLM produces a final
answer string/BaseModel without tool calls (lines 543-574, 1365-1396). On
`max_iter` exhaustion, `handle_max_iterations_exceeded` does one extra
asynchronous "force final answer" LLM call (`agent_utils.py:329-332`) and
returns `AgentFinish`. Conversation is held entirely in-memory as
`messages: list[LLMMessage]`; the executor is reset at the start of every
`invoke()` call (`crew_agent_executor.py:218-223, 1118-1124`,
`agent_executor.py:2746-2760, 2852-2866`) unless `_resuming=True`
(`base_agent_executor.py:29`, set by external callers like the Flow
checkpoint/human-input providers). No turn log is persisted to disk by the
loop itself — persistence is handled by the outer Flow checkpoint/persist
subsystem (`flow/runtime/__init__.py:2110-2209`), the memory subsystem
(`base_agent_executor.py:31-65` `_save_to_memory` invoking
`memory.remember_many`), and `self._save_to_memory(formatted_answer)` calls
at `crew_agent_executor.py:246, 1147` and `agent_executor.py:2810, 2918`.
Generic-iness across agents is achieved by composition: the executor is
parameterless about the calling agent type, taking `llm`, `tools`,
`prompt`, `task`, `crew` as injected deps
(`base_agent_executor.py:23-28`, `agent/core.py:1101-1124, 1483-1500`),
sharing helpers with `LiteAgent` (`lite_agent.py:873-972` has its own
copy of the same `while not isinstance(formatted_answer, AgentFinish):`
loop using the same `get_llm_response` /
`handle_max_iterations_exceeded` / `handle_output_parser_exception` /
`handle_context_length` helpers).

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards, but split across three loops with shared-but-not-fully-unified helpers and weak turn persistence.**

Evidence behind the number:

- (+) Three distinct loops, all using the same `BaseLLM.call`/`acall`
  contract (`base_llm.py:283-357`) and the same shared
  `get_llm_response`/`process_llm_response`/`handle_*` helpers — high
  DRY across paths.
- (+) Every loop is exposed via explicit `@start`/`@router`/`@listen`
  routing in `AgentExecutor` (e.g. `agent_executor.py:314,
  608, 815, 1378, 1451`) and well-documented (`crew_agent_executor.py:309-318,
  330-339, 484-493, 597-602, 1150-1158, 1171-1176, 1308-1316`).
- (+) Tool-call lifecycle is fully bracketed by hook/event emission
  (`crew_agent_executor.py:940-1063`, `agent_executor.py:1939-2065`) and
  tool caching (`crew_agent_executor.py:929-1005`,
  `agent_utils.py:1487-1544`).
- (+) Auto-fallback from native tool calls to ReAct text tool calls on
  provider rejection is implemented in both loops
  (`crew_agent_executor.py:577-579, 1399-1401`,
  `agent_executor.py:1539-1540`, `step_executor.py:184-220`).
- (+) Tests directly exercise loop behaviour: `_ainvoke_loop_calls_aget_llm_response`
  (`tests/agents/test_async_agent_executor.py:107-120`),
  `_ainvoke_loop_respects_max_iterations` (lines 166-208),
  `_invoke_loop_native_tools`/`native_tool_loop_falls_back`
  (`tests/agents/test_native_tool_calling.py:1271-1311`),
  `test_invoke_resets_messages_and_iterations` (lines 294-313),
  `test_invoke_preserves_state_when_resuming` (lines 338-358),
  `test_execute_native_tool_runs_parallel_for_multiple_calls`
  (`tests/agents/test_agent_executor.py:1051-1085`),
  `test_execute_native_tool_result_as_answer_short_circuits_remaining_calls`
  (lines 1129-1177).
- (-) Two near-duplicate loops (`CrewAgentExecutor` legacy +
  `AgentExecutor` Flow) plus a third `StepExecutor` — 1300+ lines
  of near-copies (`crew_agent_executor.py:309-468 vs
  agent_executor.py:1378-1547`).
- (-) Conversation persistence is not part of the loop contract:
  every `invoke()` clears `messages` and `iterations`
  (`agent_executor.py:2748-2760`), and turn-level persistence relies
  on caller-side Flow checkpointing rather than the loop.
- (-) Turn records (other than in-memory `messages`) are emitted as
  events, not stored. There is no first-class `TurnRecord` type.
- (-) Native-mode "multiple tool calls in one turn" is supported but
  has quirks: parallel execution is skipped if any tool in the batch
  has `result_as_answer` or `max_usage_count`
  (`crew_agent_executor.py:698-723`, `agent_executor.py:1827-1858`).
- (-) `CrewAgentExecutor` itself is deprecated
  (`crew_agent_executor.py:145-151`), which raises a `DeprecationWarning`
  on every construction in tests — minor but indicates a
  half-migrated state.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Runner loop (ReAct sync) | `while not isinstance(formatted_answer, AgentFinish):` ReAct loop with RPM limit, max-iter guard, get_llm_response, parse, execute tool, append message, increment iterations | `lib/crewai/src/crewai/agents/crew_agent_executor.py:341-468` |
| Runner loop (ReAct async mirror) | Identical ReAct loop body for async path | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1178-1306` |
| Runner loop (native tools sync) | `while True:` loop with native function-calling branch, single-tool or parallel-batch execution, post-tool reasoning prompt | `lib/crewai/src/crewai/agents/crew_agent_executor.py:484-595` |
| Runner loop (native tools async mirror) | Async mirror of native-tools loop | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1308-1417` |
| Runner loop (Flow-based executor entry) | `invoke` resets state, calls `kickoff()`, recovers from RuntimeError if no `AgentFinish` | `lib/crewai/src/crewai/experimental/agent_executor.py:2717-2835` |
| Turn loop body (StepExecutor multi-turn within one todo) | `for _ in range(max_step_iterations):` with text-parsed or native branching, observation message appended each iteration, timeout support | `lib/crewai/src/crewai/agents/step_executor.py:317-367, 546-578` |
| Turn loop body (Flow router) | `@router(or_(initialize_reasoning, continue_iteration))` -> `check_max_iterations` -> `continue_reasoning` / `continue_reasoning_native` loop | `lib/crewai/src/crewai/experimental/agent_executor.py:2106-2123` |
| Turn records (in-memory message log) | `messages: list[LLMMessage]` field on both executors and Plan state | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:28`, `lib/crewai/src/crewai/experimental/agent_executor.py:134, 294-302` |
| Turn records (Pydantic model of state) | `AgentExecutorState` with messages / iterations / current_answer / pending_tool_calls / todos / replan_count / observations / execution_log | `lib/crewai/src/crewai/experimental/agent_executor.py:126-161` |
| Turn records (LiteAgent variants) | `_messages: list[LLMMessage]`, `_iterations: int` private fields | `lib/crewai/src/crewai/lite_agent.py:520-525, 873-964` |
| Model-call wrapper | `get_llm_response` runs before hook, llm.call, after hook; supports native tools | `lib/crewai/src/crewai/utilities/agent_utils.py:461-511` |
| Model-call wrapper (async) | `aget_llm_response` mirrors sync wrapper | `lib/crewai/src/crewai/utilities/agent_utils.py:514-564` |
| Model-call wrapper (shared pre/post) | `_prepare_llm_call` + `_validate_and_finalize_llm_response` shared by sync/async wrappers | `lib/crewai/src/crewai/utilities/agent_utils.py:401-458` |
| LLM call interface (base abstraction) | `BaseLLM.call` (abstract) and `BaseLLM.acall` (raises NotImplementedError) defined | `lib/crewai/src/crewai/llms/base_llm.py:283-357` |
| LLM call interface (OpenAI concrete) | `OpenAICompletion.call` emits start event, formats messages, dispatches to responses/completions API, emits failed event on error | `lib/crewai/src/crewai/llms/providers/openai/completion.py:389-487` |
| Message assembly | `_setup_messages` builds system + user prompt, marks cache breakpoints, allows provider hook | `lib/crewai/src/crewai/agents/crew_agent_executor.py:170-206` |
| Message assembly (AgentExecutor) | Inline in `invoke` / `invoke_async`, marks cache breakpoints, calls provider setup_messages first | `lib/crewai/src/crewai/experimental/agent_executor.py:2769-2796, 2875-2901` |
| Message assembly (helper) | `format_message_for_llm(prompt, role)` — shared `{"role": "...", "content": prompt.rstrip()}` builder | `lib/crewai/src/crewai/utilities/agent_utils.py:353-367` |
| Final output condition | Per turn: `isinstance(formatted_answer, AgentFinish)` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:341, 467-468, 1300-1306` |
| Final output condition (Flow) | `state.is_finished = True` set on `tool_result_is_final` and during `finalize` | `lib/crewai/src/crewai/experimental/agent_executor.py:1640-1642, 1822, 2322-2326` |
| Final output condition (force-final-answer) | One extra LLM call on max-iter; synthesizes `AgentFinish` from last `formatted_answer.text + force_final_answer` | `lib/crewai/src/crewai/utilities/agent_utils.py:320-350` |
| Max-iter guard | Shared `has_reached_max_iterations` + `handle_max_iterations_exceeded` helpers | `lib/crewai/src/crewai/utilities/agent_utils.py:280-350` |
| Parser | `parse(text)` -> `AgentAction`/`AgentFinish`, raises `OutputParserError` on bad format | `lib/crewai/src/crewai/agents/parser.py:62-128` |
| Parser wrapper | `process_llm_response` uses stop-word conditional to strip Observation suffix | `lib/crewai/src/crewai/utilities/agent_utils.py:567-586` |
| Stop-word application | `_llm_stop_words_applied` ctx manager overrides `llm.stop` via `contextvars`, never mutating instance | `lib/crewai/src/crewai/utilities/agent_utils.py:260-277`, `lib/crewai/src/crewai/llms/base_llm.py:93-115` |
| Tool execution in loop | `_execute_single_native_tool_call` handles parse-args, cache read, hooks, event emission, fallback for hooks/limits | `lib/crewai/src/crewai/agents/crew_agent_executor.py:868-1071` |
| Tool execution helper | `execute_single_native_tool_call` shared between executors and StepExecutor; returns `NativeToolCallResult` dataclass | `lib/crewai/src/crewai/utilities/agent_utils.py:1374-1663` |
| Native tool call detection | `is_tool_call_list(response)` matches OpenAI/Anthropic/Gemini tool-call envelope | `lib/crewai/src/crewai/utilities/agent_utils.py:1239-1264` |
| Native tool support check | `check_native_tool_support` calls `llm.supports_function_calling()` and verifies tools are present | `lib/crewai/src/crewai/utilities/agent_utils.py:1267-1282` |
| Parallel native tool execution | `ThreadPoolExecutor(max_workers=min(8, len(tool_calls)))` with `contextvars.copy_context().run` to preserve contextvars across threads | `lib/crewai/src/crewai/agents/crew_agent_executor.py:742-767`, `lib/crewai/src/crewai/experimental/agent_executor.py:1714-1745` |
| Context-length recovery | `is_context_length_exceeded` + `handle_context_length` (summarizes or raises) | `lib/crewai/src/crewai/utilities/agent_utils.py:698-757` |
| Output parser error recovery | `handle_output_parser_exception` injects error as user message, returns `AgentAction` to keep loop alive | `lib/crewai/src/crewai/utilities/agent_utils.py:660-695` |
| Native -> ReAct downgrade | `_append_text_tool_calling_fallback_message` appends user message asking model to use text tool calling | `lib/crewai/src/crewai/agents/crew_agent_executor.py:470-482, 1398-1401`, `lib/crewai/src/crewai/experimental/agent_executor.py:1539-1540, 249-264` |
| LLM hooks | `before_llm_call` and `after_llm_call` hooks with global registration, mutable messages list, returnable response | `lib/crewai/src/crewai/hooks/llm_hooks.py:153-238` |
| Tool hooks | `before_tool_call` and `after_tool_call` hooks with hook context (tool, input, agent, task, result) | `lib/crewai/src/crewai/hooks/tool_hooks.py` |
| Event emission (start/exec) | `_show_start_logs` / `_show_logs` emit `AgentLogsStartedEvent` / `AgentLogsExecutionEvent` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1522-1560` |
| Event emission (tool lifecycle) | `crewai_event_bus.emit(ToolUsageStartedEvent/ToolUsageErrorEvent/ToolUsageFinishedEvent)` for every native tool call | `lib/crewai/src/crewai/agents/crew_agent_executor.py:940-1063` |
| Event emission (plan) | `PlanStepStartedEvent`, `PlanStepCompletedEvent`, `PlanRefinementEvent`, `PlanReplanTriggeredEvent`, `GoalAchievedEarlyEvent` | `lib/crewai/src/crewai/experimental/agent_executor.py:374-450, 922-967, 998-1010` |
| Plan-and-Execute orchestration | `AgentExecutor` Flow wires `generate_plan` → `check_todos_available` → `get_ready_todos_method` → `execute_todo_sequential` → `observe_step_result` → router by effort (low/medium/high) | `lib/crewai/src/crewai/experimental/agent_executor.py:314-948, 1018-1135, 1378-1480` |
| Replan | `_should_replan` + `_trigger_replan` regenerate plan if more than 2 todos failed or replan indicators in last message | `lib/crewai/src/crewai/experimental/agent_executor.py:2452-2502, 2504-2665` |
| Synthesis final answer | One extra LLM call (`self.llm.call([system, user])`) to synthesize todo results into final output | `lib/crewai/src/crewai/experimental/agent_executor.py:2359-2446` |
| Resuming cross-call state | `_resuming: bool = PrivateAttr(default=False)` on `BaseAgentExecutor`; preserves messages + iterations when set | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:29`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:217-223, 1118-1124` |
| Memory persistence | `_save_to_memory(formatted_answer)` calls agent's `memory.remember_many` with extracted snippets | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:31-65`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:246, 1147`, `lib/crewai/src/crewai/experimental/agent_executor.py:2810, 2918` |
| Generic / agent-specific | Same `get_llm_response` helpers used by `CrewAgentExecutor`, `AgentExecutor` (experimental), `LiteAgent._invoke_loop` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:360-370`, `lib/crewai/src/crewai/experimental/agent_executor.py:1394-1404`, `lib/crewai/src/crewai/lite_agent.py:873-964` |
| Base shared executor | `BaseAgentExecutor(BaseModel)` with crew/agent/task/iterations/max_iter/messages | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:19-65` |
| Executor choice wiring | `Agent.executor_class` typed `CrewAgentExecutor | AgentExecutor`; default is `AgentExecutor` | `lib/crewai/src/crewai/agent/core.py:136, 337-343, 1101-1124, 1483-1500` |
| Tests for loop phases | Async loop: `_ainvoke_loop_calls_aget_llm_response`, `_ainvoke_loop_handles_tool_execution`, `_ainvoke_loop_respects_max_iterations` | `lib/crewai/tests/agents/test_async_agent_executor.py:107-208` |
| Tests for native fallback | `native_tool_loop_falls_back_when_provider_rejects_tools` | `lib/crewai/tests/agents/test_native_tool_calling.py:1271-1311` |
| Tests for parallel / short-circuit | `test_execute_native_tool_runs_parallel_for_multiple_calls`, `test_execute_native_tool_result_as_answer_short_circuits_remaining_calls` | `lib/crewai/tests/agents/test_agent_executor.py:1051-1177` |
| Tests for reset/resume | `test_invoke_resets_messages_and_iterations`, `test_invoke_preserves_state_when_resuming` | `lib/crewai/tests/agents/test_async_agent_executor.py:294-358` |
| Deprecated loop guard | `CrewAgentExecutor.__init__` issues `DeprecationWarning` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:145-151` |

## Answers to Dimension Questions

### 1. What happens during one turn?

A single turn is "one LLM call → optional tool execution(s) → observation
appended → iteration counter incremented". Specifically:

- **Synchronous ReAct** (`crew_agent_executor.py:341-468`): `while not
  isinstance(formatted_answer, AgentFinish)` — enforces RPM
  (`enforce_rpm_limit`, line 354), checks `has_reached_max_iterations`
  (line 343), calls `get_llm_response` (line 360), runs
  `process_llm_response` (line 398), branches on
  `AgentAction` vs `AgentFinish`, runs tool via
  `execute_tool_and_check_finality` (line 415), appends the assistant
  message (line 432), and increments `self.iterations` (line 460).
- **Native-tools** (`crew_agent_executor.py:484-595`): `while True` —
  checks max-iter, enforces RPM, calls
  `get_llm_response(..., tools=openai_tools, available_functions=None)`
  (line 517); if response is a tool-call list, dispatches to
  `_handle_native_tool_calls` (line 536) which appends an
  `assistant (tool_calls=None)` message via
  `_append_assistant_tool_calls_message` (line 735, 843) and either
  parallel-executes (lines 742-767) or runs the first call sequentially
  (line 786); afterwards either returns an `AgentFinish` (line 1102) or
  appends a `post_tool_reasoning` user prompt (line 783) and continues.
- **Flow executor** (`agent_executor.py:1378-1547`): one turn is a
  router chain `check_max_iterations → continue_reasoning | continue_reasoning_native
  → call_llm_and_parse | call_llm_native_tools → route_by_answer_type
  | execute_tool_action | execute_native_tool → continue_iteration →
  check_max_iterations`.
- **StepExecutor** (`step_executor.py:317-367, 546-578`): one
  inner-turn within a single Plan todo — loop body builds on either
  observed tool result (text) or appended tool messages + observation
  (native) for up to `max_step_iterations=15`.

### 2. Does every turn call the model?

Yes. Every loop body unconditionally invokes the model once per turn,
either via `get_llm_response` (`crew_agent_executor.py:360, 517`;
`agent_executor.py:1394, 1484`), `aget_llm_response`
(`crew_agent_executor.py:1197, 1340`), `self.llm.call`
(`step_executor.py:341, 555`), or `LiteAgent._invoke_loop`'s
`get_llm_response` (`lite_agent.py:888`). The only exception is
`handle_max_iterations_exceeded` (`agent_utils.py:293-350`) which makes
one final extra model call to force a final answer. There is no "tool
result alone is enough to produce a turn" path — the LLM is re-prompted
with the observation / reflection prompt and allowed to either emit
more tool calls or a final answer.

### 3. Can a turn include multiple tool calls?

Yes in native-tools mode, no in ReAct mode. Specifically:

- Native-tools with multiple tool calls in one LLM response either runs
  them in parallel via `ThreadPoolExecutor(max_workers=min(8, n))` with
  `contextvars.copy_context().run` for cross-thread contextvars
  (`crew_agent_executor.py:742-767`, `agent_executor.py:1714-1745`),
  or sequentially if any tool has `result_as_answer=True` or
  `max_usage_count` set
  (`crew_agent_executor.py:698-723`,
  `agent_executor.py:1827-1858`). The `result_as_answer` tool
  short-circuits remaining calls
  (`crew_agent_executor.py:1097-1106`,
  `agent_executor.py:1773-1784, 1811-1822`).
- ReAct mode parses one `AgentAction` per LLM response
  (`crew_agent_executor.py:402-432`); only the legacy parser supports
  one tool per turn.

### 4. Is turn state persisted?

Mostly no, with caveats:

- The full conversation is held in `self.messages` /
  `self.state.messages` for the duration of one `invoke()` call
  (`crew_agent_executor.py:220-247, 1121-1148`,
  `agent_executor.py:2746-2812, 2852-2921`). It is wiped at the start
  of each `invoke()` call (lines 218-223, 2746-2760, 2852-2866) unless
  `_resuming=True` is set externally
  (`base_agent_executor.py:29`,
  `tests/agents/test_async_agent_executor.py:338-358`).
- The Flow executor multiplies `max_iter` by 10 to derive a
  `max_method_calls` upper bound on the entire Flow execution
  (`agent_executor.py:229`).
- For real persistence: the final answer is saved to memory via
  `_save_to_memory` (`crew_agent_executor.py:246, 1147`,
  `agent_executor.py:2810, 2918`,
  `base_agent_executor.py:31-65`); Conversation/run persistence
  outside the in-memory log is delegated to the outer Flow's
  checkpoint / `@persist` runtime (`flow/runtime/__init__.py:2110-2209`).
- Live turn telemetry is emitted as events on the event bus
  (`crew_agent_executor.py:940-1063, 1527-1560`,
  `agent_executor.py:922-967, 1939-2065`).

There is no first-class `TurnRecord` class, no per-turn row written by
the loop, and no built-in mid-turn persistence that survives across
process restarts outside the Flow subsystem.

### 5. Is the loop generic or agent-specific?

Generic via composition with three concrete implementations sharing the
same helpers:

- `BaseAgentExecutor` (`base_agent_executor.py:19-65`) defines the
  shared state shape (`crew`, `agent`, `task`, `iterations`, `max_iter`,
  `messages`) but holds no logic itself — only `_save_to_memory`.
- `CrewAgentExecutor` (`crew_agent_executor.py:98-1671`) is the legacy
  subclass.
- `AgentExecutor` (`agent_executor.py:164-3206`) is the Flow-based
  modern subclass and the default
  (`agent/core.py:337-343`).
- `LiteAgent._invoke_loop` (`lite_agent.py:860-972`) is its own
  near-duplicate, reusing the same helpers
  (`get_llm_response`, `process_llm_response`,
  `has_reached_max_iterations`, `handle_output_parser_exception`,
  `handle_context_length`).
- `get_llm_response` / `aget_llm_response` accept any `LLM | BaseLLM`
  and `executor_context: CrewAgentExecutor | AgentExecutor | LiteAgent |
  None` (`agent_utils.py:401-458, 461-564`).
- The shared `_setup_before_llm_call_hooks` /
  `_setup_after_llm_call_hooks` work with any of the three
  (`agent_utils.py:1666-1795`).

So the loops are generic — agent type does not appear in the loop body
— but the existence of three implementations is duplication rather than
a single shared core.

## Architectural Decisions

- **Two parallel executor paths with a clear migration target.** The
  legacy `CrewAgentExecutor` is officially deprecated in favour of the
  Flow-based `AgentExecutor`
  (`crew_agent_executor.py:145-151`,
  `agent/core.py:156-160`). The `_validate_executor_class` validator
  raises when `CrewAgentExecutor` is passed via the wire
  (`agent/core.py:148-165`).
- **Single source of truth for LLM wrapping.** `get_llm_response` /
  `aget_llm_response` are the only public entry points and they route
  through shared `_prepare_llm_call` / `_validate_and_finalize_llm_response`
  pairs that fire global hooks
  (`agent_utils.py:401-458, 461-564, 1666-1795`).
- **ReAct vs native function calling as orthogonal modes.** Each
  executor top-level branches on `llm.supports_function_calling() and
  original_tools` and delegates to either
  `_invoke_loop_react`/`_ainvoke_loop_react` (text-parsed
  Thought/Action/Action Input) or
  `_invoke_loop_native_tools`/`_ainvoke_loop_native_tools`
  (`crew_agent_executor.py:309-328, 1150-1169`).
- **Native mode supports both single-tool sequential (with reflection
  prompts) and multi-tool parallel** via `ThreadPoolExecutor`, with an
  override for `result_as_answer` to short-circuit
  (`crew_agent_executor.py:698-767, 1097-1106`,
  `agent_executor.py:1714-1786, 1827-1858`).
- **Auto-downgrade on provider rejection.** Both executors detect
  `is_native_tool_calling_unsupported_error` and append a
  text-tool-calling fallback message before re-entering the ReAct loop
  (`crew_agent_executor.py:470-482, 577-579`,
  `agent_executor.py:249-264, 1539-1540`); `StepExecutor` does the
  same within a single step and keeps already-built messages
  (`step_executor.py:184-220`).
- **Conversation held in one mutable list, mutated by hooks.** Hooks
  may `context.messages.append(...)` to add context before the next
  LLM call (`hooks/llm_hooks.py:24-58`). If a hook replaces the list,
  a warning is logged and the original list restored
  (`agent_utils.py:1704-1717, 1766-1779`).
- **Force-final-answer loop termination.** When `iterations >=
  max_iter`, `handle_max_iterations_exceeded` makes one final LLM
  call asking the model to produce a final answer
  (`agent_utils.py:293-350`); the Flow executor has an additional
  `ensure_force_final_answer` router that is idempotent under
  duplicate-flow-tick
  (`agent_executor.py:1354-1376`).
- **Plan-and-Execute as a nested loop.** When `agent.planning_enabled`
  is true, `AgentExecutor` builds a `TodoList` (planning handler →
  todos), routes to `get_ready_todos_method` (parallel/sequential),
  and for each todo invokes `StepExecutor`
  (`agent_executor.py:314-948, 1018-1135, 1378-1480`). Per-step
  multi-turn action loops live in `StepExecutor._execute_*`
  (`step_executor.py:317-578`). `PlannerObserver` observes each step
  to decide whether to continue / refine / replan
  (`agent_executor.py:608-895`).
- **Event bus as a hard observability contract.** Every loop emits
  `AgentLogsStartedEvent`, `AgentLogsExecutionEvent`, and on each
  native tool call emits `ToolUsageStartedEvent` /
  `ToolUsageFinishedEvent` / `ToolUsageErrorEvent`
  (`crew_agent_executor.py:940-1063, 1527-1560`,
  `agent_executor.py:1939-2065`).
- **Hooks + stop-word override as cross-cutting concerns.** Global
  `before_llm_call` / `after_llm_call` and
  `before_tool_call` / `after_tool_call` hooks with hook context
  objects (`hooks/llm_hooks.py:153-238`, `hooks/tool_hooks.py`).
  Executor stop-words are merged into the LLM's stop list via a
  `contextvars`-scoped override that never mutates the LLM instance
  (`agent_utils.py:260-277`,
  `llms/base_llm.py:93-115`).
- **Synchronous / async duality maintained.** `CrewAgentExecutor`
  has `_invoke_*` and `_ainvoke_*` siblings that mirror each other
  line-for-line (`crew_agent_executor.py:309-595, 1150-1455`); the
  Flow-based `AgentExecutor` instead uses `invoke_async` and detects
  inside-event-loop via `is_inside_event_loop()` to return a coroutine
  (`agent_executor.py:2717-2734`).
- **Recoverable vs terminal error split.** `litellm`-origin
  exceptions are always re-raised (they are surfaced to users verbatim,
  `agent/core.py:685-717`); context-length / parser errors feed
  recovery paths (`is_context_length_exceeded`,
  `handle_output_parser_exception`).

## Notable Patterns

- **State proxy with thread-safe lock.** `AgentExecutor.state` returns
  `StateProxy(self._state, self._state_lock)`, and `iterations` /
  `messages` are properties that mirror `_state.iterations` /
  `_state.messages` so external callers can use either access path
  (`agent_executor.py:279-313`).
- **Initialize-once protect against double-invoke.** `_is_executing`
  flag + `_execution_lock` raise `RuntimeError` if invoked twice
  concurrently on the same executor
  (`agent_executor.py:213, 736-742, 2842-2847`).
- **Finalize guard via `_finalize_lock`.** The Flow's `finalize` may
  be entered twice when concurrent branches both terminate; a separate
  `_finalize_called: bool` plus lock makes the second call return
  `"completed"` (`agent_executor.py:2254-2275`).
- **Synthesis final answer via one more LLM call.** Plan-and-Execute
  finalises with `_synthesize_final_answer_from_todos` instead of
  dumping raw step outputs (`agent_executor.py:2359-2446`); a heuristic
  skips synthesis if the last step's text is a sentence-form answer
  ≥ 200 chars / ≥ 30 words, with structured-output caveat
  (`agent_executor.py:2328-2357`).
- **Per-step timeout.** `StepExecutor.execute` accepts
  `step_timeout: int | None`; the inner loop checks
  `time.monotonic() - start_time >= step_timeout` and returns
  accumulated tool results / timeout sentinel
  (`step_executor.py:128-180, 336-340, 546-554`).
- **Cache breakpoints on message assembly.** System and user messages
  are wrapped with `mark_cache_breakpoint` so providers can cache the
  stable per-agent / per-task prefix across ReAct-loop iterations
  (`crew_agent_executor.py:189-204`,
  `agent_executor.py:2775-2790, 2881-2896`).
- **vision sentinels converted to multimodal content blocks.**
  `_build_observation_message` parses `VISION_IMAGE:<media_type>:<b64>`
  into an `image_url` data URL so the LLM sees the image
  (`step_executor.py:462-503, 619-647`).
- **Hook context can pause for human input.** The `request_human_input`
  method on `LLMCallHookContext` pauses live event-listener updates
  with `event_listener.formatter.pause_live_updates()`, prompts on
  stdin, and resumes (`hooks/llm_hooks.py:109-150`).
- **Multi-modal file injection on last user message.** `_inject_files_from_inputs`
  finds the most recent `user`-role message and attaches a `files` key
  (`crew_agent_executor.py:249-307`,
  `agent_executor.py:3107-3151`).

## Tradeoffs

- **Three near-duplicate loops.** ReAct logic is copy-pasted between
  sync/async in `CrewAgentExecutor` (lines 330-468 vs 1171-1306) and
  re-exists as the Flow-based version in `AgentExecutor` (1378-1547),
  with `StepExecutor` (317-578) being a fourth near-copy. This adds
  maintenance cost: every bug fix / new tool type must be applied
  four places, and the deprecation of `CrewAgentExecutor` adds a
  half-migrated state.
- **Native + parallel tool execution has a non-obvious policy.**
  Parallel execution is *silently* downgraded to sequential if any
  tool in the batch has `result_as_answer` or `max_usage_count`
  (`agent_executor.py:1827-1858`); without reading the code, callers
  may not predict parallelism behaviour.
- **`max_iter=25` default + `max_method_calls = max_iter * 10`.** The
  Flow turns each ReAct iteration into potentially ~10 method calls,
  so the effective per-task step ceiling is 250 methods
  (`agent_executor.py:229`). The relationship is implicit; no doc
  warns that adjusting `max_iter` may not give the expected per-task
  budget.
- **In-memory messages + Flow-state plumbing.** Messages are not
  serialized by the loop itself; if the executor is constructed
  repeatedly (e.g. via pickling for the Flow), `AgentExecutorState`
  must be re-hydrated explicitly. The Flow layer handles this
  (`flow/runtime/__init__.py:2100-2209`).
- **Hooks see executor-context and may accidentally break state.**
  Hook contract requires in-place `context.messages.append(...)`;
  replacing the list is silent loss of the original messages
  (`agent_utils.py:1704-1717`). Each executor re-installs it as a
  best-effort, not a hard guarantee.
- **`_setup_before_llm_call_hooks` checks `executor.before_llm_call_hooks` directly.**
  If a hook is registered globally after the executor has been
  constructed, it is picked up only if the executor fetches
  `get_before_llm_call_hooks()` again (which `CrewAgentExecutor.__init__`
  does at `crew_agent_executor.py:152-155`); `AgentExecutor`'s
  `_setup_executor` model_validator does the same
  (`agent_executor.py:225-226`).
- **Async dispatch via "magic auto-async".** `AgentExecutor.invoke`
  uses `is_inside_event_loop()` to decide whether to return a
  coroutine, which is convenient but easy to get wrong across
  threads/executors (`agent_executor.py:2717-2734`).

## Failure Modes / Edge Cases

- **Litellm-originated exceptions always re-raised.** Several layers
  test `e.__class__.__module__.startswith("litellm")` and re-raise
  (`crew_agent_executor.py:445-446, 580-581, 1283-1284, 1402-1403`,
  `agent_utils.py:1446-1447, 1544-1545`). Any integration that wraps
  litellm errors in its own exception will lose the bypass.
- **ReAct parser: OutputParserError can dead-loop if continued
  misuse.** `handle_output_parser_exception` returns `AgentAction`
  with empty `tool` and `tool_input`, which the executor then tries to
  execute (`agent_utils.py:660-695`,
  `crew_agent_executor.py:434-442`). The loop terminates only via
  max-iter exhaustion or successful final answer.
- **Empty LLM response.** `get_llm_response` raises `ValueError`
  (`agent_utils.py:448-454`); `StepExecutor` raises
  `ValueError("Empty response from LLM")` and the surrounding flow
  route reaches `finalize` with no `current_answer`
  (`step_executor.py:348-349, 563-564`).
- **Native tool batch ambiguity.** When an LLM returns malformed tool
  calls alongside valid ones, the executor parses them all
  (`extract_tool_call_info`), but extras with `info is None` are
  silently dropped
  (`crew_agent_executor.py:688-694, 809-841`).
- **Finalize without `AgentFinish`.** Despite the
  `RuntimeError("without reaching a final answer")` at multiple
  layers (`crew_agent_executor.py:462-466, 1300-1306`,
  `agent_executor.py:2802-2805, 2908-2911`), the Flow `finalize`
  produces a fallback `AgentFinish("Agent completed execution but
  produced no final output...")` to avoid crashing
  (`agent_executor.py:2295-2311`).
- **StepExecutor expected-tool enforcement.** If a todo specifies
  `tool_to_use` and the LLM doesn't call it, the step is marked
  `success=False` (`step_executor.py:504-526, 224-231`).
- **Resume semantics only when caller sets `_resuming`.** Tests rely
  on `executor._resuming = True` to simulate mid-execution resume
  (`tests/agents/test_async_agent_executor.py:338-358`). Without
  caller coordination, default behaviour is to wipe messages.
- **Replan deadlock detection.** `get_ready_todos_method` returns
  `"needs_replan"` if todos are pending but none are ready
  (`agent_executor.py:1047-1060`); replan count is bounded by
  `max_replans` (default 3, line 502). Three consecutive replans
  without progress triggers `all_todos_complete` fallback.
- **Hook-blocked LLM call.** A `before_llm_call` hook that returns
  `False` short-circuits with `ValueError("LLM call blocked by
  before_llm_call hook")` (`agent_utils.py:421-424`).
- **`is_native_tool_calling_unsupported_error` is heuristic.** The
  pattern list (`agent_utils.py:68-76`) is substring matching against
  `str(error).lower()`; a provider that returns the same phrase for
  unrelated errors will trigger a needless downgrade.

## Future Considerations

- **Unify the three loops.** With `CrewAgentExecutor` deprecated,
  folding the legacy code paths into a shared core (or behind a thin
  compat shim) would reduce the four-way drift.
- **First-class turn record.** A `TurnRecord` dataclass / Pydantic
  model (iteration index, prompt hash, response hash, tool calls,
  observation, latency, finish reason) would unlock replay,
  checkpointing, and explainability without depending on event
  consumers.
- **Persist messages within the executor.** Today the loop wipes
  `self.state.messages` on every `invoke()`; a method
  like `serialize_state()` + `restore_state()` round-trip would let
  users resume mid-task from disk instead of only from Flow
  checkpoint.
- **Tighter parallelism policy.** Document when
  `result_as_answer` / `max_usage_count` cause sequential execution
  and surface the choice via a config flag.
- **Replace `max_method_calls = max_iter * 10` heuristic** with a
  named, documented coupling constant.
- **Reconsider hook "list replacement" protection.** Today a hook
  accidentally binding `context.messages = [...]` is silently
  detected and restored (`agent_utils.py:1704-1717`), but this masks
  the bug from the user; a `RuntimeError` with a clearer pointer
  might be friendlier.

## Questions / Gaps

- **Per-turn persistence boundary:** When `invoke()` is called from a
  Flow with `from_checkpoint`, who exactly rehydrates
  `self.state.messages` back into the executor at the right
  iteration? The `_resuming` flag exists but the loop code never
  sets it itself; the answer is in the Flow runtime, which is outside
  the loop's scope and not documented here.
- **Iteration semantics in Flow executor:** `state.iterations` is
  incremented in `increment_and_continue` (line 2243) on
  `todo_not_satisfied`, but the routing logic in
  `check_max_iterations` (line 2113) is keyed off
  `state.iterations` only. How iterations map to the *inner*
  `StepExecutor.execute` iterations is not explicit, and the inner
  loop has its own `max_step_iterations=15`
  (`step_executor.py:130, 336, 546`).
- **"Multiple tool calls per turn" in ReAct text mode:** No
  mechanism. The agent returns exactly one `Action` per ReAct response.
  Multi-tool coordination must wait for the next model call. No
  evidence of plans to batch in this mode.
- **`crewai.experimental.AgentExecutor` and `Flow` coupling:**
  Several methods assume the Flow runtime is in play (e.g.
  `kickoff()` is inherited); whether the loop is intended to be
  usable outside Flow is unclear.
- **No formal turn-id.** Each iteration only carries the
  `iterations` int. Cross-referencing turn ↔ event consumer needs
  consumers to manually join on `(agent_key, agent_role)` rather
  than on a structured turn id.
- **LangGraph / OpenAI Agents adapters use their own loops.** Both
  `BaseAgentAdapter` subclasses implement
  `execute_task(cls, agent, ...)` and do not reuse any of
  `get_llm_response` / `process_llm_response`
  (`agents/agent_adapters/langgraph/langgraph_adapter.py:163`,
  `agents/agent_adapters/openai_agents/openai_adapter.py:109`). The
  shared helper design does not extend to alternate runtimes.
- **`setup_messages` provider hook is undocumented for non-default
  agents.** `CrewAgentExecutor._setup_messages` checks
  `provider.setup_messages(...)` first and only falls back to its
  own logic if the provider returns False
  (`crew_agent_executor.py:176-178`). The matching protocol is in
  `core/providers/human_input.py:68-93` but its applicability to
  agents outside a `HumanInputProvider` is unclear.

---

Generated by `03.01-llm-turn-loop-structure` against `crewai`.
