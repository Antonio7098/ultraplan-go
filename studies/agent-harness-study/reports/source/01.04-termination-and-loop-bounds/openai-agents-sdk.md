# Source Analysis: openai-agents-sdk

## 01.04 — Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, asyncio; OpenAI Agents SDK (PyPI package `openai-agents`) |
| Analyzed | 2026-07-02 |

## Summary

The OpenAI Agents SDK uses an **explicit, turn-bounded** agent loop with a single
configurable hard cap (`max_turns`). There is no implicit "stuck-loop" detection —
the runtime counts every model invocation, and once `current_turn > max_turns` the
runner raises `MaxTurnsExceeded` (sync) or stores it for surfacing (streaming).
The limit can be disabled by passing `max_turns=None` (added in 0.16.0), and is
serialized through `RunState` so HITL resume flows keep the same budget.
Abnormal termination (refusals, guardrail tripwires, tool timeouts) is modeled as
typed `AgentsException` subclasses; `RunErrorHandlers` allow `max_turns` and
`model_refusal` to be converted into controlled `final_output` instead of raising.
Streaming exposes a separate, observer-driven cancel API with `immediate` and
`after_turn` modes. Tool-choice and short-circuit behavior (`stop_on_first_tool`,
`StopAtTools`, custom callable) provide **opt-in** early termination but no
generic cyclic-pattern detection.

## Rating

**Score: 8 / 10** — Clear termination model with explicit config, schema-versioned
persistence, observable error surfacing on the streaming path, and an opt-in
recovery hook (`error_handlers`). The main gap is the absence of any built-in
heuristic that detects repetitive tool calls or "stuck" behavior before the hard
`max_turns` ceiling; the SDK trusts the model to terminate naturally between
turns and trusts the caller to pick a sane `max_turns`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default turn cap constant | `DEFAULT_MAX_TURNS = 10` | `src/agents/run_config.py:33` |
| Public `max_turns` param (async) | `Runner.run(max_turns: int \| None = DEFAULT_MAX_TURNS, ...)` | `src/agents/run.py:205` |
| Public `max_turns` param (sync) | `Runner.run_sync(max_turns: int \| None = DEFAULT_MAX_TURNS, ...)` | `src/agents/run.py:289` |
| Public `max_turns` param (streamed) | `Runner.run_streamed(max_turns: int \| None = DEFAULT_MAX_TURNS, ...)` | `src/agents/run.py:370` |
| Docstring semantics | "A turn is defined as one AI invocation (including any tool calls that might occur). Pass `None` to disable the turn limit." | `src/agents/run.py:239-241`, `:328-330`, `:408-410` |
| Sync-loop turn increment + ceiling check | `current_turn += 1`; `if max_turns is not None and current_turn > max_turns:` | `src/agents/run.py:1057-1058` |
| Sync-loop `MaxTurnsExceeded` raise | `max_turns_error = MaxTurnsExceeded(f"Max turns ({max_turns}) exceeded")`; `raise max_turns_error` | `src/agents/run.py:1066-1081` |
| Streamed-loop turn increment + ceiling | `current_turn += 1`; `streamed_result.current_turn = current_turn`; check | `src/agents/run_internal/run_loop.py:875-881` |
| Streamed-loop exhaustion (no handler) | `max_turns_error = MaxTurnsExceeded(f"Max turns ({max_turns}) exceeded")`; `break` | `src/agents/run_internal/run_loop.py:889-912` |
| Streamed-loop exhaustion (handler path) | Builds `RunErrorData`, calls `resolve_run_error_handler_result`, synthesizes `MessageOutputItem`, sets `is_complete=True` | `src/agents/run_internal/run_loop.py:893-956` |
| `_max_turns_handled` flag | Prevents re-raising after handler-resolved exhaustion | `src/agents/result.py:514`, `:797-801`; set at `src/agents/run_internal/run_loop.py:894`, `:910`, `:950` |
| Exception type | `class MaxTurnsExceeded(AgentsException)` | `src/agents/exceptions.py:56` |
| Span error attached | `_error_tracing.attach_error_to_span(current_span, SpanError(message="Max turns exceeded", data={"max_turns": max_turns}))` | `src/agents/run.py:1059-1065`; mirrored `src/agents/run_internal/run_loop.py:882-888` |
| Stream consumer error check | `_check_errors` synthesizes `MaxTurnsExceeded` when `current_turn > max_turns and not _max_turns_handled` | `src/agents/result.py:793-801` |
| Stream `cancel()` modes | `cancel(mode="immediate" \| "after_turn")`; `_cancel_mode` field | `src/agents/result.py:648-694`, field at `:496` |
| Cancel checked in stream loop | `if streamed_result._cancel_mode == "after_turn": break` (3 sites) | `src/agents/run_internal/run_loop.py:842-845`, `:1109-1112`, `:1161-1164` |
| Normal stop: `NextStepFinalOutput` | `class NextStepFinalOutput: output: Any` | `src/agents/run_internal/run_steps.py:159-161` |
| Normal stop: `NextStepHandoff` | `class NextStepHandoff: new_agent: Agent[Any]` | `src/agents/run_internal/run_steps.py:154-156` |
| Continue: `NextStepRunAgain` | `class NextStepRunAgain: pass` | `src/agents/run_internal/run_steps.py:164-166` |
| Interruption: `NextStepInterruption` | `class NextStepInterruption: interruptions: list[ToolApprovalItem]` | `src/agents/run_internal/run_steps.py:169-174` |
| Final-output synthesis (text/structured) | `execute_final_output(...)` returns `NextStepFinalOutput`; guards refusal + structured JSON path | `src/agents/run_internal/turn_resolution.py:305-335`, `:748-845` |
| Refusal detection | `refusal = ItemHelpers.extract_refusal(message_items[-1].raw_item)` → `ModelRefusalError` | `src/agents/run_internal/turn_resolution.py:763-789`; class at `src/agents/exceptions.py:78-86` |
| Tool-timeout exception | `class ToolTimeoutError(AgentsException)` | `src/agents/exceptions.py:109-118` |
| Tool-timeout enforcement | `await asyncio.wait_for(tool_task, timeout=timeout_seconds)`; `ToolTimeoutError(...)` on expiry | `src/agents/tool.py:1812-1847` |
| Guardrail exceptions | `InputGuardrailTripwireTriggered`, `OutputGuardrailTripwireTriggered`, `ToolInputGuardrailTripwireTriggered`, `ToolOutputGuardrailTripwireTriggered` | `src/agents/exceptions.py:121-173` |
| `error_handlers` TypedDict | `RunErrorHandlers(TypedDict): max_turns`, `model_refusal` | `src/agents/run_error_handlers.py:50-54` |
| Handler resolver | `resolve_run_error_handler_result` picks handler by error class; supports sync/async/None/dict/raw | `src/agents/run_internal/error_handlers.py:128-166` |
| Handler validation | `validate_handler_final_output` raises `UserError` if structured output mismatches schema | `src/agents/run_internal/error_handlers.py:80-107` |
| `error_handlers` wiring on sync | `error_handlers=error_handlers` passed into `Runner.run` and through `AgentRunner.run` | `src/agents/run.py:208`, `:275`, `:1074` |
| `error_handlers` wiring on streamed | `error_handlers=error_handlers` passed into `start_streaming` and used at the exhaustion site | `src/agents/run.py:1864`; `src/agents/run_internal/run_loop.py:448`, `:903` |
| Resume preserves max_turns | `max_turns = run_state._max_turns` overrides caller kwarg | `src/agents/run.py:508`, `:1713` |
| Streamed resume preserves turn counter | `current_turn=run_state._current_turn if run_state else 0` | `src/agents/run.py:1802` |
| Resuming-turn NOT counted toward ceiling | `resuming_turn = is_resumed_state` — only model calls advance `_current_turn` per AGENTS.md note | `src/agents/run.py:769`; mirrored `src/agents/run_internal/run_loop.py:730-840` |
| RunState persisted `max_turns` (default 10) | `RunState._max_turns: int \| None = 10` | `src/agents/run_state.py:224-225` |
| RunState JSON includes `max_turns` | `"max_turns": self._max_turns` | `src/agents/run_state.py:722` |
| RunState JSON allows `null` max_turns | Schema 1.10: "Allows serialized RunState snapshots to disable max_turns with null." | `src/agents/run_state.py:147` |
| RunState round-trip `max_turns=None` | `test_max_turns_none_round_trips` | `tests/test_run_state.py:314-324` |
| Resume keeps limit after `to_state()` | `test_preserve_max_turns_when_resuming_from_runresult_state` | `tests/test_hitl_error_scenarios.py:562-596` |
| HITL resume enforces limit | `test_streaming_hitl_resume_enforces_max_turns` | `tests/test_agent_runner_streamed.py:1941-1966` |
| `current_turn` re-attach on final result | `result._current_turn = max_turns` (sync); mirrored for streamed | `src/agents/run.py:1122`; `src/agents/run_internal/run_loop.py:951-953` |
| Pretty print exposes limit | `output += f"\n- Max turns: {result.max_turns}"` | `src/agents/util/_pretty_print.py:58` |
| Pretty print exposes traceback on error | `RunErrorDetails` carries `input`, `new_items`, `raw_responses`, `last_agent`, guardrail results | `src/agents/exceptions.py:30-43` |
| `__str__` of `RunErrorDetails` | `pretty_print_run_error_details(self)` | `src/agents/exceptions.py:42-43` |
| Streamed drain before raise (deferred error) | `_should_drain_stream_events_before_raising` for `MaxTurnsExceeded` | `src/agents/result.py:710-720`, helpers at `src/agents/exceptions.py:22-27` |
| `_drain_stream_events` marker on error | `_mark_error_to_drain_stream_events(error)` on `MaxTurnsExceeded` raises | `src/agents/run.py:1066`; mirrored `src/agents/run_internal/run_loop.py:889` |
| Final-output `run_final_output_hooks` | `await run_final_output_hooks(current_agent, hooks, ...)` runs before returning | `src/agents/run.py:1093-1098`; `src/agents/run_internal/turn_resolution.py:284-302` |
| Output guardrails still run after handler path | `output_guardrail_results = await run_output_guardrails(...)` | `src/agents/run.py:1099-1104`, mirrored streamed |
| Streamed cancellation handling | `_event_queue.put_nowait(QueueCompleteSentinel())`; `is_complete=True` | `src/agents/run_internal/run_loop.py:911`, `:955`; sentinel type at `src/agents/run_internal/run_steps.py:52-56` |
| `is_complete` finalization in `finally` | Guarantees sentinel emission even on unexpected exception | `src/agents/run_internal/run_loop.py:1237-1239` |
| `_await_task_safely` swallows cancellation | Surfaces error via `_check_errors()` rather than propagating `CancelledError` to caller | `src/agents/result.py:852-866` |
| `run_data` attached to `AgentsException` | `exc.run_data = RunErrorDetails(...)` in streamed loop `except AgentsException` branch | `src/agents/run_internal/run_loop.py:1175-1187` |
| `Agent.reset_tool_choice` (anti-loop default) | `reset_tool_choice: bool = True` — "ensures that the agent doesn't enter an infinite loop of tool usage." | `src/agents/agent.py:367-369` |
| `Agent.reset_tool_choice` enforcement | `maybe_reset_tool_choice(...)` sets `tool_choice=None` after first tool call | `src/agents/run_internal/tool_execution.py:541-549`; call sites `src/agents/run_internal/run_loop.py:1346`, `:1830` |
| `AgentToolUseTracker` | Tracks per-agent tool usage; used by `reset_tool_choice`, persisted in `RunState` | `src/agents/run_internal/tool_use_tracker.py:50-116`; serialization at `src/agents/run_state.py:266`, `:721`, `:1000-1021` |
| `tool_use_behavior` short-circuits | `"run_llm_again"` (default), `"stop_on_first_tool"`, `StopAtTools({"stop_at_tool_names": [...]})`, custom callable | `src/agents/agent.py:134-136`, `:345-365` |
| `tool_use_behavior` dispatch | `check_for_final_output_from_tools(...)` | `src/agents/run_internal/turn_resolution.py:593-626` |
| Tests for short-circuit | `test_run_llm_again_behavior`, `test_stop_on_first_tool_behavior` | `tests/test_tool_use_behavior.py:61-100` |
| Default runner state | `DEFAULT_AGENT_RUNNER = AgentRunner()` (instance at module bottom) | `src/agents/run.py:1879` |
| `current_turn` reset on resume `RunState` | `current_turn = run_state._current_turn` | `src/agents/run.py:617` |
| Streamed resume starts at saved turn | `current_turn = run_state._current_turn` | `src/agents/run_internal/run_loop.py:563-565` |
| Single-turn structure | `run_single_turn` / `run_single_turn_streamed` per turn body | `src/agents/run_internal/run_loop.py:248` |
| Hard timeout in model retry | `RetryableModelError` + bounded retries (not a turn-bound, but a request bound) | `src/agents/run_internal/model_retry.py` (imported via `src/agents/run_internal/run_loop.py:122-126`) |

## Answers to Dimension Questions

1. **What stops the loop?**
   - **Natural completion** — model emits a message that matches `agent.output_type`
     (text or validated structured output), producing `NextStepFinalOutput`
     (`src/agents/run_internal/turn_steps.py:159`, `src/agents/run_internal/turn_resolution.py:809-835`).
   - **Tool short-circuit** — `tool_use_behavior="stop_on_first_tool"` returns the first
     tool output as `final_output`; `StopAtTools({"stop_at_tool_names": [...]})` returns the
     first matching tool output; a custom callable can return `ToolsToFinalOutputResult(is_final_output=True, ...)`
     (`src/agents/run_internal/turn_resolution.py:603-626`).
   - **Handoff** — `NextStepHandoff` swaps `current_agent` and continues the outer loop
     (`src/agents/run.py:1011-1023`, `src/agents/run_internal/run_loop.py:1091-1112`).
   - **HITL interruption** — tool calls with `needs_approval=True` produce
     `NextStepInterruption`, which is converted into a `RunResult.interruptions` list
     and pauses via `QueueCompleteSentinel` (`src/agents/run_internal/turn_resolution.py:713-723`,
     `src/agents/run_internal/run_steps.py:169-174`).
   - **Hard cap** — `MaxTurnsExceeded` once `current_turn > max_turns`
     (`src/agents/run.py:1058`, `src/agents/run_internal/run_loop.py:881`).
   - **Refusal / guardrail / tool-timeout** — typed `AgentsException` subclasses
     (`src/agents/exceptions.py:56-173`).

2. **Are limits configurable?**
   Yes:
   - `max_turns: int | None` on every entry point (`src/agents/run.py:205`, `:289`, `:370`),
     defaulting to `DEFAULT_MAX_TURNS = 10` (`src/agents/run_config.py:33`).
   - `max_turns=None` disables the cap entirely (added in 0.16.0; documented in
     `docs/release.md:67`, exercised by `tests/test_max_turns.py:55-78`).
   - `error_handlers={"max_turns": ...}` converts the hard cap into a controlled
     `final_output` (`src/agents/run_error_handlers.py:50-54`,
     `src/agents/run_internal/error_handlers.py:128-166`).
   - `Agent.tool_use_behavior` short-circuits at the agent level
     (`src/agents/agent.py:345-365`).
   - Streaming exposes `RunResultStreaming.cancel(mode="immediate" | "after_turn")`
     (`src/agents/result.py:648-694`).
   - Resume restores `max_turns` from `RunState` regardless of the caller kwarg
     (`src/agents/run.py:508`, `:1713`); schema 1.10 added `null` support
     (`src/agents/run_state.py:147`).

3. **Is exhaustion treated differently from success?**
   Yes — exhaustion is signal, not data:
   - Sync path raises `MaxTurnsExceeded` with `max_turns` baked into the message
     and attached span error data (`src/agents/run.py:1066`, `:1059-1065`).
   - Streaming path stores `MaxTurnsExceeded` in `_stored_exception` and emits a
     `QueueCompleteSentinel` so `stream_events()` can drain queued events before
     re-raising (`src/agents/result.py:710-720`, helpers at `src/agents/exceptions.py:22-27`).
   - The runner attaches `RunErrorDetails` (input, new_items, raw_responses, last_agent,
     guardrail results) to the exception's `run_data` field for postmortem rendering
     (`src/agents/run_internal/run_loop.py:1178-1186`, `src/agents/exceptions.py:30-43`).
   - When a `max_turns` handler resolves the error, the runner synthesizes a
     `MessageOutputItem`, runs `on_agent_end` hooks, and re-emits the result with
     `_current_turn = max_turns` so callers can tell the run used the budget
     (`src/agents/run.py:1087-1122`, `src/agents/run_internal/run_loop.py:918-956`).

4. **Are stuck loops detected before the hard limit?**
   No — only **opt-in** opt-outs exist:
   - `reset_tool_choice=True` (default) prevents the model from being forced into
     the same tool call repeatedly (`src/agents/agent.py:367-369`,
     `src/agents/run_internal/tool_execution.py:541-549`).
   - `tool_use_behavior="stop_on_first_tool"` and `StopAtTools` provide explicit
     early termination when a target tool is invoked (`src/agents/run_internal/turn_resolution.py:605-614`).
   - Custom `tool_use_behavior` callables may inspect the tool-result list and
     choose to finish (`src/agents/run_internal/turn_resolution.py:615-626`).
   - There is **no** automatic detection of repetitive function-call patterns, no
     output-similarity check, and no separate "loop iteration" budget distinct
     from `max_turns`. The `AgentToolUseTracker` (`src/agents/run_internal/tool_use_tracker.py`)
     records tool usage for `reset_tool_choice` and for serialization into
     `RunState`, not for stuck-loop detection.

5. **Does the user get a useful final state?**
   Yes for both success and exhaustion:
   - `RunResult`/`RunResultStreaming` always carry `final_output`, `new_items`,
     `raw_responses`, guardrail results, `_current_turn`, and `_last_agent`
     (`src/agents/result.py:333-368`, `:444-540`).
   - `result.max_turns` is exposed on the dataclass so downstream code can inspect
     the configured ceiling after the fact (`src/agents/result.py:365`).
   - `pretty_print_result`/`pretty_print_run_result_streaming` render the
     `Max turns:` line plus the entire trace, error details, and tool outputs
     (`src/agents/util/_pretty_print.py:58`, `src/agents/exceptions.py:42-43`).
   - On exhaustion with a configured handler, the synthesized
     `RunErrorHandlerResult.final_output` is the user's `final_output`, optionally
     with `include_in_history=False` (`src/agents/run_error_handlers.py:36-47`).
   - On exhaustion without a handler, the caller receives `MaxTurnsExceeded` whose
     `str()` prints the captured `RunErrorDetails` (`src/agents/exceptions.py:42-43`,
     `src/agents/run.py:1178-1187`).
   - `current_turn` is preserved across `to_state()`/`Runner.run(RunState)` so the
     budget is enforced across HITL pauses (`src/agents/run.py:508`, `:1713`;
     `tests/test_hitl_error_scenarios.py:562-596`).

## Architectural Decisions

- **A single turn counter is the only loop bound.** Both the sync loop
  (`src/agents/run.py:1057-1058`) and the streamed loop
  (`src/agents/run_internal/run_loop.py:875-881`) increment `current_turn` once per
  model invocation and check `current_turn > max_turns` immediately afterward. No
  nested counters, no per-tool counters, no recursion limits.
- **Termination is observed, not predicted.** The runtime waits for the model to
  produce a final output and uses `tool_use_behavior` to convert any tool result
  into a final output when the agent config demands it
  (`src/agents/run_internal/turn_resolution.py:603-626`).
- **`RunState` is the durable pause/resume boundary.** `max_turns`,
  `current_turn`, the agent identity, the conversation ids, and the tool-use
  tracker all round-trip through `RunState.to_json()` so a HITL pause/resume
  enforces the same budget on the next process. Schema 1.10 explicitly added
  `null` support so `max_turns=None` survives serialization
  (`src/agents/run_state.py:131-149`, `:722`).
- **Errors are data, not control flow at the runner level.** `MaxTurnsExceeded`
  and `ModelRefusalError` are routed through `RunErrorHandlers` when supplied;
  otherwise they propagate as exceptions with attached `run_data`
  (`src/agents/run_error_handlers.py:50-54`,
  `src/agents/run_internal/error_handlers.py:128-166`).
- **Streaming termination is event-queue driven.** The run loop signals completion
  with a `QueueCompleteSentinel` rather than returning, so the consumer's
  `stream_events()` can observe the full event stream before the stored
  exception is raised (`src/agents/run_internal/run_steps.py:52-56`,
  `src/agents/result.py:696-779`).
- **Refusals are no longer retried silently.** In 0.15.0 the runner stopped
  treating a refusal as an empty turn and started surfacing `ModelRefusalError`,
  removing a class of accidental `MaxTurnsExceeded` outcomes
  (`docs/release.md:72`, `src/agents/run_internal/turn_resolution.py:774-789`).

## Notable Patterns

- **Typed `AgentsException` hierarchy.** `AgentsException` is the root and carries
  `run_data: RunErrorDetails | None` so any subclass can be rendered with full
  context (`src/agents/exceptions.py:46-53`). Subclasses: `MaxTurnsExceeded`,
  `ModelBehaviorError`, `ModelRefusalError`, `UserError`,
  `MCPToolCancellationError`, `ToolTimeoutError`, plus four guardrail tripwire
  variants (`src/agents/exceptions.py:56-173`).
- **`NextStep*` enum-by-dataclass.** The loop body returns one of
  `NextStepFinalOutput | NextStepHandoff | NextStepRunAgain | NextStepInterruption`,
  and the dispatcher in `run.py`/`run_loop.py` reacts with `continue`/`return`/`break`
  (`src/agents/run_internal/run_steps.py:154-174`, callers at `src/agents/run.py:1003-1025`,
  `src/agents/run_internal/run_loop.py:1065-1150`).
- **Drain-before-raise sentinel.** When `MaxTurnsExceeded` is stored for the stream
  consumer, the runner uses `_mark_error_to_drain_stream_events` so that
  `stream_events()` keeps draining the event queue before raising
  (`src/agents/exceptions.py:22-27`, `src/agents/result.py:710-720`).
- **Per-agent tool-use tracker.** `AgentToolUseTracker` records tool usage keyed by
  agent identity; it's serialized into `RunState._tool_use_tracker_snapshot` and
  re-hydrated on resume (`src/agents/run_internal/tool_use_tracker.py:50-156`,
  `src/agents/run_state.py:266`, `:1000-1021`).
- **Schema-versioned RunState.** `CURRENT_SCHEMA_VERSION = "1.11"` with a parallel
  `SCHEMA_VERSION_SUMMARIES` table; the loop's only direct dependency on a schema
  bump is serialization compatibility (`src/agents/run_state.py:131-149`).
- **Async/sync mirrored logic.** `run.py` (sync) and `run_internal/run_loop.py`
  (streamed) implement the same `current_turn += 1` / `current_turn > max_turns`
  check; a regression in one is unlikely to be caught by tests in the other
  (`tests/test_max_turns.py` covers both).

## Tradeoffs

- **Single scalar bound is simple but coarse.** `max_turns` does not distinguish
  between "model wrote text" turns and "model made 5 tool calls" turns. A
  tool-heavy agent can hit the ceiling after producing far less user-visible
  progress than a text-heavy one. Callers must size the budget themselves.
- **No stuck-loop detection.** Repetitive identical tool calls (e.g., the model
  reformulating the same query) are not detected before the hard cap. This is
  the dimension's central gap — `AgentToolUseTracker` records tool names but
  does not compare their arguments or outputs.
- **`max_turns=None` disables safety.** When set, the loop has no terminator other
  than successful output, guardrail trips, refusals, tool timeouts, or explicit
  cancellation. Documented and tested but easy to misuse.
- **Handler validation is strict.** `validate_handler_final_output` raises
  `UserError` if a structured-output handler returns an unserializable payload,
  forcing callers to be careful with Pydantic models
  (`src/agents/run_internal/error_handlers.py:80-107`,
  `tests/test_max_turns.py:286-341`).
- **Cancellation is observer-side.** `cancel(mode="after_turn")` is honored only at
  well-defined points (`src/agents/run_internal/run_loop.py:842`, `:1109`, `:1161`),
  so a stuck model call still has to finish or be cancelled through the lower-level
  `_cleanup_tasks()` path (`src/agents/result.py:839-847`).
- **Streaming error timing depends on event-queue draining.** Callers must consume
  `stream_events()` to receive the sentinel before the stored exception is raised
  (`src/agents/result.py:705-779`); documented but easy to miss.

## Failure Modes / Edge Cases

- **`max_turns=0` with a handler returns immediately** — exercised in
  `tests/test_max_turns.py:286-341`; demonstrates the handler path can produce a
  final output without any model call.
- **Handler returns invalid structured output** — `UserError("Invalid run error
  handler final_output for structured output.")` (`src/agents/run_internal/error_handlers.py:101-103`).
- **Handler returns dict with disallowed keys** — `UserError("Invalid run error
  handler result.")` (`src/agents/run_internal/error_handlers.py:160`).
- **Async handler returns `None`** — the runner falls back to raising the
  underlying error (`src/agents/run_internal/error_handlers.py:148-152`,
  `tests/test_run_internal_error_handlers.py:101-110`).
- **Refusal with no handler on structured-output agent** — `ModelRefusalError`
  raised immediately, no retry to `MaxTurnsExceeded` (post-0.15.0,
  `src/agents/run_internal/turn_resolution.py:774-789`,
  `docs/release.md:72-83`).
- **HITL resume with exhausted budget** — `test_streaming_hitl_resume_enforces_max_turns`
  shows the resumed run correctly trips `MaxTurnsExceeded`
  (`tests/test_agent_runner_streamed.py:1941-1966`).
- **Resuming a state whose budget was set to `None`** — preserved across
  serialization; subsequent runs will not enforce a turn limit
  (`tests/test_run_state.py:314-324`).
- **Cancel-during-streaming race** — `_await_task_safely` swallows exceptions
  during cancellation and re-checks via `_check_errors()` to avoid silent losses
  (`src/agents/result.py:852-866`).
- **Run-loop failure before producing any event** — `RunResultStreaming.run_loop_exception`
  exposes the captured exception after the queue drains, so callers don't lose
  early sandbox/initialization failures (`src/agents/result.py:623-646`).
- **Tool timeout returning to model** — `FunctionTool.timeout_behavior="error_as_result"`
  yields a string the model can read; `"raise_exception"` raises `ToolTimeoutError`
  (`src/agents/tool.py:1812-1847`, `src/agents/exceptions.py:109-118`).

## Future Considerations

- **A stuck-loop heuristic** could be layered on top of `AgentToolUseTracker`:
  if the same `(agent, tool_name, argument_hash)` repeats within the run window,
  surface a typed exception or interrupt. The tracker already serializes per-agent
  tool names; an argument hash is the natural extension.
- **Per-tool turn counters** would let a caller say "max 3 tool calls to
  `web_search` total, max 10 turns overall" without conflating the two budgets.
- **Differentiated budgets for handoff vs. tool turns** would help when one agent
  hands off and the new agent consumes most of the budget.
- **Streaming cancellation of in-flight model calls** would close the
  "model is stuck" gap. Today `cancel(mode="after_turn")` waits for the model to
  finish; a "cancel mid-model-call" path would require provider-level support.
- **Observability hooks for exhaustion cause** (e.g., why the model kept
  tool-calling) could be added as a structured log/event alongside the existing
  span error (`src/agents/run.py:1059-1065`).
- **Schema migration cost** — bumping `CURRENT_SCHEMA_VERSION` to require a
  newer schema on resume is a breaking change; the schema 1.10 entry explicitly
  mentions backporting null `max_turns` support
  (`src/agents/run_state.py:147`).

## Questions / Gaps

- Is there any **tool-call dedup** in the agent loop beyond
  `deduplicate_input_items_preferring_latest` for conversation-history assembly
  (`src/agents/run_internal/items.py:340-346`)? If the model emits the same
  function call twice in one turn, does the second call actually run?
  *No clear evidence found* in `run_internal/tool_planning.py` /
  `tool_execution.py` of an automatic suppression of repeated tool calls within
  a turn.
- Is there any **output-similarity check** across consecutive turns (e.g., cosine
  similarity of message text or function-call arguments)? *No evidence found.*
- Does the **run_config** carry any `max_turns` knob distinct from the runner
  kwarg? No — only the per-call kwarg and the `DEFAULT_MAX_TURNS` constant exist
  (`src/agents/run_config.py:33`, `src/agents/run.py:457`).
- What happens if the **handler raises** (e.g., network error)? The handler is
  `await`ed directly with no try/except; the exception propagates and the run
  fails with the handler's traceback rather than `MaxTurnsExceeded`. *No clear
  evidence found* of a fallback.
- What is the **interaction between `max_turns=None` and `error_handlers["max_turns"]`**
  in the wild? With `max_turns=None` the check `current_turn > max_turns` never
  fires, so the handler is unreachable through that path. The handler can still
  fire for `model_refusal`, but the test surface is thin for the combined case.
  *No clear evidence found.*

---

Generated by `dimensions/01.04-termination-and-loop-bounds.md` against `openai-agents-sdk`.