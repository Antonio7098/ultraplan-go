# Source Analysis: openai-agents-sdk

## 03.06 — Stuck and Doom-Loop Detection

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (`src/agents/` package, `pyproject.toml`, `uv`-managed); runtime organised around a non-streaming path (`run.py` → `run_internal/run_loop.py::run_single_turn`) and a streaming path (`run_internal/run_loop.py::run_single_turn_streamed` / `start_streaming`) |
| Analyzed | 2026-07-15 |

## Summary

The OpenAI Agents SDK ships **no first-class doom-loop detector and no repeated-tool-call detector**. Stuck / doom-loop behaviour is enforced entirely through three blunt mechanisms — none of which inspect recent tool calls, recent errors, or message history to recognise *patterns*:

1. **`max_turns` counter** (`src/agents/run.py:227,239-241,316,329,396,408,457` and `src/agents/run_config.py:33` define `DEFAULT_MAX_TURNS = 10`). The streaming loop counts each LLM invocation as one turn (`src/agents/run_internal/run_loop.py:875-877`) and short-circuits with `MaxTurnsExceeded` once `current_turn > max_turns` (`src/agents/run_internal/run_loop.py:881-956`; sync equivalent at `src/agents/run.py:1058-1144`). Streaming post-loop verification re-raises if the cap was crossed but no handler ran (`src/agents/result.py:793-801`).
2. **`reset_tool_choice` heuristic** (`src/agents/agent.py:367-369` default `True`; applied at `src/agents/run_internal/tool_execution.py:541-549` → `maybe_reset_tool_choice`). Documented as "infinite loop prevention" in `docs/agents.md:423-425`: when the model was forced to pick a tool (`tool_choice="required"`) and at least one tool has run, the SDK nulls `tool_choice` on the next turn so the model is no longer compelled to call a tool every turn. This is the **only** mechanism in the codebase that is named "loop prevention", and it is a `tool_choice` policy — not a pattern detector.
3. **Per-tool-item dedupe at planning time** (`src/agents/run_internal/tool_planning.py:158-174::_dedupe_tool_call_items`, `src/agents/run_internal/tool_planning.py:140-155::_build_tool_output_index`). Identifies previously-emitted tool calls by `(type, call_id)` to avoid double-executing the same `tool_call` after a model emits a repeat. This is **idempotency at the planning layer**, not loop detection — it only catches *exactly* re-emitted tool calls with the same call id and does nothing for repeated-but-distinct calls that produce the same effects.

`error_handlers` (`src/agents/run_error_handlers.py:50-54`) only allow user code to handle `MaxTurnsExceeded` / `ModelRefusalError` post-hoc. There is no auto-recovery, no escalation, no replanning, and no automatic hint injection. The other internals with "loop"-adjacent names — `AgentToolUseTracker` (`src/agents/run_internal/tool_use_tracker.py:50-117`), `OpenAIServerConversationTracker` (`src/agents/run_internal/oai_conversation.py:43-60`), `fingerprint_input_item` (`src/agents/run_internal/items.py:230-265`), `deduplicate_input_items_preferring_latest` (`src/agents/run_internal/items.py:340-346`), and `OpenAIResponsesCompactionSession` (`src/agents/memory/openai_responses_compaction_session.py:78-340`) — do protocol-level de-duplication or compaction, not behavioural loop detection.

`grep -rn` across `src/agents/` for `stuck`, `doom`, `consecutive`, `recent_turn.*window`, `no_progress`, `repeated_action`, `repeat.{0,30}error`, `same_tool`, and `replan` returns **0 relevant hits**. The only matches for "loop" outside the `while True`/`for tool in ...` language constructs are the `tool_use_behavior` setting (`src/agents/agent.py:345-478`), the `run_demo_loop` REPL helper (`src/agents/repl.py:15,22,26`), and the `_max_turns_handled` flag on the streaming result (`src/agents/result.py:514`, `src/agents/run_internal/run_loop.py:894,910,950`).

**Answering the prompt's headline question** ("Does the system stop repeated behavior before the user pays for 20 useless turns?"): **No**, with the partial exception of `reset_tool_choice`. The default `max_turns=10` (`src/agents/run_config.py:33`) means a typical ReAct-style agent dies at turn 10 with no diagnostic signal; a non-default `max_turns=20` lets the agent complete 20 useless tool calls before stopping. The `reset_tool_choice` trick only works when the caller *also* used `tool_choice="required"`; otherwise the model can repeat successful tool calls indefinitely until `max_turns`.

## Rating

**Score: 2 / 10** — Absent. There is no detector for repeated identical tool calls, alternating A/B patterns, repeated-error streaks, or "no-progress monologues". The only loop-prevention mechanism in the codebase is `reset_tool_choice`, which is a `tool_choice` policy reset, not a pattern detector. `max_turns` is a count cap that fires after N turns, not a stuck detector. `AgentToolUseTracker` (`src/agents/run_internal/tool_use_tracker.py:50-117`) records which tools each agent has ever used — its sole consumer is `maybe_reset_tool_choice` (`src/agents/run_internal/tool_execution.py:541-549`). False positives from a non-existent detector are impossible; false negatives are the norm. "Agent keeps calling the same successful tool" will burn the full `max_turns` budget and surface only as `MaxTurnsExceeded`. The user must supply their own detector by inspecting `result.new_items` themselves in an `error_handlers={"max_turns": ...}` handler; the SDK provides no helper for that.

## Evidence Collected

Every entry includes file path and line numbers. Format: `path/to/file.py:NN`.

### `max_turns` counter (turn cap, not pattern detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default `max_turns` constant | `DEFAULT_MAX_TURNS = 10` | `src/agents/run_config.py:33` |
| Public `Runner.run*` accepts and threads `max_turns` | `max_turns: int \| None = DEFAULT_MAX_TURNS` | `src/agents/run.py:205,289,370` |
| Docstring lists `MaxTurnsExceeded` as one of two exception paths | "If the max_turns is exceeded, a MaxTurnsExceeded exception is raised unless handled." | `src/agents/run.py:227-229,316-318,396-398` |
| Docstring defines a turn as one AI invocation | "A turn is defined as one AI invocation (including any tool calls that might occur)." | `src/agents/run.py:240,329,409` |
| `max_turns=None` disables the cap | "Pass `max_turns=None` to disable the turn limit." | `src/agents/run.py:241,330,410` |
| Streaming loop increments `current_turn` per LLM call | `current_turn += 1; streamed_result.current_turn = current_turn` | `src/agents/run_internal/run_loop.py:875-877` |
| Streaming loop short-circuits on `current_turn > max_turns` | `if max_turns is not None and current_turn > max_turns:` attaches span error and builds a `MaxTurnsExceeded` | `src/agents/run_internal/run_loop.py:881-889` |
| `MaxTurnsExceeded` exception class | `class MaxTurnsExceeded(AgentsException)` | `src/agents/exceptions.py:56-63` |
| Streaming handler resolution & fall-through | `handler_result = await resolve_run_error_handler_result(...)`; if `None` → break loop; otherwise synthesises a final-output item and appends it to history | `src/agents/run_internal/run_loop.py:902-955` |
| Streaming path tracks `_max_turns_handled` to suppress re-raise | `streamed_result._max_turns_handled = True/False` | `src/agents/run_internal/run_loop.py:894,910,950` |
| Streaming post-loop re-raises if cap was crossed without handler | `if self.max_turns is not None and self.current_turn > self.max_turns and not self._max_turns_handled: ... raise` | `src/agents/result.py:793-801` |
| Sync loop raises on overflow and offers handler | `if handler_result is None: raise max_turns_error` else synthesize final output and return `RunResult` | `src/agents/run.py:1058-1144` |
| Tests assert `MaxTurnsExceeded` for streamed and non-streamed paths | `test_non_streamed_max_turns`, `test_streamed_max_turns`, plus structured-output variants | `tests/test_max_turns.py:30-51,82-119,238-282` |
| Test with `max_turns=0` and `error_handlers={"max_turns": ...}` returning fallback | `test_non_streamed_max_turns_handler_returns_output` etc. | `tests/test_max_turns.py:344-489` |

### `reset_tool_choice` heuristic ("infinite loop" guard, but only on forced picks)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Configurable flag with explicit "infinite loop" rationale | "Whether to reset the tool choice to the default value after a tool has been called. Defaults to True. This ensures that the agent doesn't enter an infinite loop of tool usage." | `src/agents/agent.py:367-369` |
| Reset logic only fires when `tool_use_tracker.has_used_tools(agent) is True` | `if agent.reset_tool_choice is True and tool_use_tracker.has_used_tools(agent): return dataclasses.replace(model_settings, tool_choice=None)` | `src/agents/run_internal/tool_execution.py:541-549` |
| Wired in non-streaming path | `model_settings = maybe_reset_tool_choice(public_agent, tool_use_tracker, model_settings)` | `src/agents/run_internal/run_loop.py:1830` |
| Wired in streaming path | same call | `src/agents/run_internal/run_loop.py:1346` |
| Doc note explaining the cause of "infinite loop" | "To prevent infinite loops, the framework automatically resets `tool_choice` to "auto" after a tool call. […] The infinite loop is because tool results are sent to the LLM, which then generates another tool call because of `tool_choice`, ad infinitum." | `docs/agents.md:423-425` |

### `AgentToolUseTracker` (record-keeping, not detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Class docstring: "support `model_settings` resets" — explicitly not a detector | "Track which tools an agent has used to support model_settings resets." | `src/agents/run_internal/tool_use_tracker.py:50-51` |
| Storage is a list (preserves order/count), not a windowed count of recent turns | `self.agent_to_tools: list[tuple[Agent[Any], list[str]]]` | `src/agents/run_internal/tool_use_tracker.py:57` |
| `has_used_tools` is the only query | `return bool(existing and existing[1])` (any past usage, no threshold) | `src/agents/run_internal/tool_use_tracker.py:98-100` |
| Used by `maybe_reset_tool_choice` only (see above) | Single consumer | `src/agents/run_internal/tool_execution.py:541-549` |
| `reset_tool_choice` requires only one occurrence to flip the flag | Same line — `has_used_tools` is binary, not "N consecutive tool calls of the same name" | `src/agents/run_internal/tool_execution.py:547` |
| Serialised via `serialize_tool_use_tracker` for resume | `serialize_tool_use_tracker(tool_use_tracker, starting_agent=...)` writes `_tool_use_tracker_snapshot` into `RunResult` and `RunResultStreaming` | `src/agents/run_internal/tool_use_tracker.py:119-136`, `src/agents/result.py:78,99,343,434,519,932`, `src/agents/run.py:673-678` |

### Per-tool-item dedupe (idempotency, not loop detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `_dedupe_tool_call_items` dedupes by `(type, call_id)` — only catches the same exact emitted call again | `if isinstance(item, ToolCallItem): identity = _tool_call_identity(item.raw_item); if identity in existing_call_keys: continue` | `src/agents/run_internal/tool_planning.py:158-174` |
| Called from `execute_tools_and_side_effects` with `existing_items=pre_step_items` | `new_step_items = _dedupe_tool_call_items(existing_items=pre_step_items, new_items=processed_response.new_items)` | `src/agents/run_internal/turn_resolution.py:659-662` |
| `_build_tool_output_index` makes a `(type, call_id)` set for "have we already produced a tool output" | `index: set[tuple[str, str]] = set()` | `src/agents/run_internal/tool_planning.py:140-155` |
| Streaming path also tracks `emitted_tool_call_ids` to suppress double-event emission | `if output_call_id and output_call_id not in emitted_tool_call_ids: emitted_tool_call_ids.add(output_call_id)` | `src/agents/run_internal/run_loop.py:1280,1566-1571,1667-1678` |
| Docstring frames dedupe as idempotency, not repetition detection | "Return new items while skipping tool call duplicates already seen by identity." | `src/agents/run_internal/tool_planning.py:161` |
| **Distinct but functionally-repeated tool calls are not detected.** Same tool name + same arguments + new `call_id` would survive this layer and run again. | (Absent — no structural detector here.) | No evidence found |

### `error_handlers` (user-supplied post-hoc, no auto)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Only two supported keys, no `tool_stuck` / `doom_loop` | `max_turns`, `model_refusal` | `src/agents/run_error_handlers.py:50-54` |
| Handler input carries only `RunErrorData` snapshot (input, history, raw_responses, last_agent) | `RunErrorHandlerInput(error=..., context=..., run_data=...)` | `src/agents/run_error_handlers.py:28-32` |
| Handler result shape: `final_output` + optional `include_in_history` | `RunErrorHandlerResult(final_output: Any, include_in_history: bool = True)` | `src/agents/run_error_handlers.py:35-40` |
| Resolution is a simple dict-get + dict/`RunErrorHandlerResult`/raw-value coercion | `resolve_run_error_handler_result(error_handlers, error, ...)` | `src/agents/run_internal/error_handlers.py:128-166` |
| Doc explicitly says handlers return controlled output instead of raising | "Use them when you want to return a controlled final output instead of raising `MaxTurnsExceeded` or `ModelRefusalError`." | `docs/running_agents.md:464-468` |
| No automatic intervention — handler must be supplied by the caller | Same doc describes the user-defined `on_max_turns` callback | `docs/running_agents.md:470-495` |

### Recent-event window — none at this layer

| Area | Evidence | File:Line |
|------|----------|-----------|
| `grep "recent_turns\|recent_tool\|recent_message\|window"` in `src/agents/` | Only matches: `ToolOutputTrimmer.recent_turns` (a separate context-trimming extension in `src/agents/extensions/tool_output_trimmer.py:54-55,66`), `recent_turns=2` default; not a detector | `src/agents/extensions/tool_output_trimmer.py:54-55` |
| `AgentToolUseTracker` has no "last N turns" semantics, only "ever used" | Same file | `src/agents/run_internal/tool_use_tracker.py:50-117` |
| `RunState` / `RunResult.new_items` are full lists, not ring buffers | `new_items: list[RunItem]` | `src/agents/result.py:78`, `src/agents/run_state.py:266` |
| Streaming queue is FIFO with no per-event classifier | `_event_queue.put_nowait(...)` | `src/agents/run_internal/run_loop.py:299,416,668,698,843,844,911,955,1239` |

### "Doom-loop"-adjacent terms and their (non-detector) roles

| Area | Evidence | File:Line |
|------|----------|-----------|
| `tool_use_behavior` (LLM-side policy, not a detector) | `"run_llm_again"`, `"stop_on_first_tool"`, `StopAtTools`, callable → `ToolsToFinalOutputResult` | `src/agents/agent.py:345-365`, `src/agents/run_internal/turn_resolution.py:594-626` |
| `fingerprint_input_item` (server-conversation dedupe) | Stable JSON dump used to suppress replays of already-sent items | `src/agents/run_internal/items.py:230-265`, `src/agents/run_internal/oai_conversation.py:43-60,106-150` |
| `OpenAIResponsesCompactionSession` (history compaction, not loop detection) | Calls server-side `responses.compact` when ≥10 candidates accumulate | `src/agents/memory/openai_responses_compaction_session.py:24,54-56,78-340` |
| `ToolOutputTrimmer` (token trimmer, not a detector) | "This module provides a configurable filter that surgically trims bulky tool outputs from older turns" | `src/agents/extensions/tool_output_trimmer.py:1-26` |
| `_FUNCTION_TOOL_POST_INVOKE_WAIT_SECONDS = 0.1` | A small per-tool concurrency watcher, not a loop detector | `src/agents/run_internal/tool_execution.py:162` |

## Answers to Dimension Questions

1. **What stuck patterns are detected?**
   None. There is no detector for repeated identical tool calls, alternating A/B patterns, repeated-error streaks, or "no-progress monologues". The only behavioural counter is `current_turn` (`src/agents/run_internal/run_loop.py:875-877`, `src/agents/run.py:1057`), which fires *after* the configured `max_turns` and is unrelated to whether any individual turn made progress. `AgentToolUseTracker.has_used_tools` (`src/agents/run_internal/tool_use_tracker.py:98-100`) is a binary "has this agent used any tool" flag, not a repetition counter.

2. **How far back does detection look?**
   There is no detection. `AgentToolUseTracker.agent_to_tools` (`src/agents/run_internal/tool_use_tracker.py:57`) accumulates usage for the lifetime of a run but is queried only as a boolean; it does not window or count recent events. `result.new_items` and `run_state._generated_items` contain full history (`src/agents/result.py:78`, `src/agents/run_state.py:266`), so an external observer could inspect them, but the SDK does not.

3. **Does compaction erase loop evidence?**
   Yes, *if* the user opts in to compaction. `OpenAIResponsesCompactionSession` (`src/agents/memory/openai_responses_compaction_session.py:78-340`) replaces accumulated items with a `compaction` summary item once ≥10 candidates exist (`DEFAULT_COMPACTION_THRESHOLD = 10`, line 24; `default_should_trigger_compaction`, lines 54-56). The companion `ToolOutputTrimmer` (`src/agents/extensions/tool_output_trimmer.py:1-309`) similarly truncates tool outputs from older turns (`_find_recent_boundary`, lines 157-173). After either runs, the original tool outputs are no longer in the conversation history, so any external observer that wanted to inspect them gets only the trimmed summary.

4. **What intervention happens?**
   - On `current_turn > max_turns` (`src/agents/run_internal/run_loop.py:881-956` / `src/agents/run.py:1058-1144`): the loop attaches `MaxTurnsExceeded` to the current span and consults `RunErrorHandlers["max_turns"]` if configured (`src/agents/run_error_handlers.py:50-54`). If a handler returns a `RunErrorHandlerResult`, that result becomes the final output and the loop exits cleanly (`src/agents/run_internal/run_loop.py:914-956`, `src/agents/run.py:1083-1144`). If no handler is configured, the loop raises `MaxTurnsExceeded` (streaming sets `_max_turns_handled = True` first; sync raises immediately at `src/agents/run.py:1081`).
   - On detected non-loop bugs (`ModelBehaviorError`, `AgentsException`, etc.): raised out of the loop after attaching span errors (`src/agents/run_internal/run_loop.py:1165-1199`).
   - On `reset_tool_choice=True` and `has_used_tools(agent)` (`src/agents/run_internal/tool_execution.py:541-549`): next model call sees `tool_choice=None`, no longer forcing a tool pick. **This is the only intervention described as "infinite loop prevention" in the docs** (`docs/agents.md:423-425`).

5. **Are false positives possible?**
   The single intervention tied to loops — `reset_tool_choice` — can theoretically cause a "false positive" only in the trivial sense that it nulls `tool_choice` after *any* tool use, even when the user actually wanted continued forced picks. The framework cannot emit false-positive trap-stops because it makes no pattern judgments at all. False *negatives* are the norm: an LLM calling `get_weather` 20 times in a row with the same arguments and the same answer will run all 20 iterations (barring `max_turns`).

## Architectural Decisions

- **Detection is the LLM's job, not the harness's.** The design assumes the model itself recognises loops and adjusts (see `docs/agents.md:423-425`). The harness adds only a count cap and a `tool_choice` policy reset, both of which are blunt.
- **`MaxTurnsExceeded` is the universal "stuck" signal.** All non-loop, non-refusal failure paths bubble up as plain `AgentsException` subtypes; the only "iteration-level" exception is `MaxTurnsExceeded` (`src/agents/exceptions.py:56-63`). The harness cannot distinguish "model is repeating itself" from "task is just long" at the SDK level.
- **`error_handlers` are the escape hatch.** The framework deliberately does not auto-intervene on `MaxTurnsExceeded`; it surfaces the error to the caller and lets them decide. This is documented as a feature (`docs/running_agents.md:464-468`).
- **Tool-call idempotency lives in planning, not detection.** `_dedupe_tool_call_items` (`src/agents/run_internal/tool_planning.py:158-174`) catches *the same emitted call_id* being processed twice in one turn — a safety net against replay bugs, not a loop detector.
- **OpenAI Responses compaction is the only meaningful "history shrinker".** `OpenAIResponsesCompactionSession` (`src/agents/memory/openai_responses_compaction_session.py:78-340`) replaces accumulated items server-side, which means downstream observers lose tool-call evidence. The `ToolOutputTrimmer` extension (`src/agents/extensions/tool_output_trimmer.py:1-309`) does the same client-side for token trimming.

## Notable Patterns

- **`run_loop_task` + `_max_turns_handled` (streaming).** The streaming runner sets `_max_turns_handled = True` whenever the user-supplied handler returns a value, and the post-loop `_check_errors` re-raises the cap only when the flag is still false (`src/agents/result.py:793-801`). This is a clean acknowledgement that a successful handler *is* recovery.
- **`fingerprint_input_item` ring of `sent_item_fingerprints`.** `OpenAIServerConversationTracker` (`src/agents/run_internal/oai_conversation.py:43-60,106-150,182-187,230-235`) keeps a per-run fingerprint set of items already sent to the model. This de-duplicates *across retries/resumes*, not across an iteration loop, and is unrelated to detection.
- **`maybe_reset_tool_choice` is "post-turn" tool_choice policy.** It runs in *both* streaming and non-streaming `get_new_response` paths (`src/agents/run_internal/run_loop.py:1346,1830`) before the model is called, so the tool_choice flip is visible to the model on the very next turn.
- **No "stuck-step" telemetry.** Tracing captures `agent_span`, `turn_span`, `task_span`, and function spans (`src/agents/tracing/span_data.py:99` describes "one agent loop turn"), but there is no `stuck` or `doom_loop` span/event type.
- **Handler-only recovery is consistent across exception types.** `RunErrorHandlers = {"max_turns": ..., "model_refusal": ...}` (`src/agents/run_error_handlers.py:50-54`) is the same shape for both supported kinds, with the same `RunErrorHandlerResult` payload (`src/agents/run_error_handlers.py:35-40`). Adding a hypothetical "tool_repeated" handler would slot in identically.

## Tradeoffs

- **Trusting the model to detect its own loops.** Cheaper to build, simpler API surface (`reset_tool_choice` is one boolean flag, not a detector config). Fails badly when the model itself is the source of the repetition — a stuck tool-call loop will continue until `max_turns` regardless of how smart the model is.
- **`max_turns=10` as the universal backstop.** With the default, an agent burns at most 10 model invocations before stopping. A user opting into `max_turns=None` to allow long runs accepts the unlimited cost surface (`src/agents/run.py:241,330,410`).
- **`reset_tool_choice` is a one-shot policy, not a feedback loop.** It triggers on first tool use, not on "model has called the same tool K times". A model that picks different tools each turn is unaffected.
- **`AgentToolUseTracker` is per-agent, not per-tool, not per-args.** Two identical tool calls with the same name by the same agent are indistinguishable from two distinct calls by the same agent.

## Failure Modes / Edge Cases

- **Repeated successful tool calls with new `call_id`s.** Pass through all layers: `_dedupe_tool_call_items` only catches repeated call_ids (`src/agents/run_internal/tool_planning.py:158-174`); `AgentToolUseTracker.has_used_tools` returns `True` but `maybe_reset_tool_choice` only nulls `tool_choice`, not force-stop the run (`src/agents/run_internal/tool_execution.py:541-549`); no other detector exists. Will burn `max_turns`.
- **Alternating A → B → A → B tool pattern.** Not detected. The SDK has no A/B window concept anywhere.
- **Tool errors followed by tool success (transient error → stuck-success).** The first turn's tool error appears in history; nothing increments a "consecutive errors" counter. `max_turns` is the only backstop.
- **Compaction erases evidence before reaching `max_turns`.** `OpenAIResponsesCompactionSession` (`src/agents/memory/openai_responses_compaction_session.py:78-340`) and `ToolOutputTrimmer` (`src/agents/extensions/tool_output_trimmer.py:66-90`) will replace or trim tool outputs as soon as the configured thresholds (`DEFAULT_COMPACTION_THRESHOLD = 10`, `recent_turns = 2`) are met. Any future detector that wanted to read raw tool output loses it.
- **Forced `tool_choice` not reset if `reset_tool_choice=False` and no tools used yet.** With `tool_choice="required"` and `reset_tool_choice=False` the model genuinely cannot finish without calling a tool every turn; this is documented (`docs/agents.md:423-425`) but there is no further safeguard beyond `max_turns`.
- **Server-managed conversation re-sends items on retry.** `mark_input_as_sent` / `rewind_input` (`src/agents/run_internal/oai_conversation.py:262-275,375-411`, `src/agents/run_internal/run_loop.py:1452-1456,1876-1880`) ensure session continuity on retry but are unrelated to loop detection; they reduce retry-induced waste without inspecting turn-to-turn patterns.
- **`AgentToolUseTracker` snapshot drift across resume.** `_tool_use_tracker_snapshot` is persisted in `RunState` (`src/agents/run_state.py:266,721,1000-1021`) and re-hydrated on resumption, but is only consulted by `maybe_reset_tool_choice` — even a malformed snapshot cannot cause a wrong "loop stopped" verdict, only a wrong `tool_choice`.

## Future Considerations

- **Add a `consecutive_tool_errors` counter + handler.** Symmetric to `AgentToolUseTracker.has_used_tools` but counts `ToolCallOutputItem` whose output indicates failure and resets on success. Would integrate via a third key in `RunErrorHandlers` (`src/agents/run_error_handlers.py:50-54`) and a new `RunErrorHandlerResult` shape.
- **Add a recent-tool-call fingerprint window.** Either extend `AgentToolUseTracker` or stand up a small `RecentToolCallWindow(max_size=N, threshold=M)` with rolling `tool_name + arguments` fingerprints and a `detect()` method. The dedupe primitive already exists at `src/agents/run_internal/items.py:230-265`.
- **Promote `reset_tool_choice` to a configurable policy.** Today it is a boolean (`src/agents/agent.py:367`). A policy callback returning `tool_choice` for the next turn would let the user implement "reset only when N consecutive calls to the same tool/args".
- **Expose a public `run_stuck_detector` hook.** A function `(RunContextWrapper, new_items) -> Maybe[StuckVerdict]`, called once per turn, that can stop or hint. Would slot into the same handler-result plumbing already proven for `max_turns` (`src/agents/run_internal/run_loop.py:902-955`).
- **Add a tracing span/event `stuck_loop_intervention`.** Currently the only spans available are `agent_span`/`turn_span`/`task_span`/`function_span`; a dedicated signal would give users telemetry for counting interventions.
- **Make `max_turns=None` emit a loud warning.** Today `max_turns=None` is silently silent (`src/agents/run.py:241,330,410`); accidental unbounded runs are a real footgun for "user pays for 20 useless turns".

## Questions / Gaps

- **Is "agent keeps calling the same tool successfully" by design?** The doc comment (`docs/agents.md:423-425`) only names `tool_choice` as the loop vector; that implies the SDK's design assumes successful repetition is benign. There is no explicit statement either way.
- **What does the `AgentToolUseTracker` snapshot persistence buy the user?** It is currently a single consumer (`maybe_reset_tool_choice`). If an external user inspects the snapshot via `result.tool_use_tracker_snapshot` (`src/agents/result.py:78`), they get the union of tools ever used, not a recent window.
- **Is `error_handlers` enough?** The doc frames handlers as the primary intervention hook (`docs/running_agents.md:464-468`), but they only fire post-cap. A repeated-call detector must be in-loop to reduce the per-iteration bill; nothing in the current SDK does this.
- **No tests for repeated-tool-call patterns.** I searched `tests/` for `repeat`, `consecutive`, `same_tool`, `doom`, `loop_detect`, `stuck` and found none. The framework has no test coverage for the pattern detection that does not exist.
