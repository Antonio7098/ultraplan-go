# Source Analysis: letta

## 03.06 — Stuck and Doom-Loop Detection

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.12+ (FastAPI server, Pydantic v2, SQLAlchemy 2, asyncio agent loop `LettaAgentV2` / `LettaAgentV3`); `DEFAULT_LETTA_MODEL = "gpt-4"` |
| Analyzed | 2026-07-15 |

## Summary

Letta's detection of non-progress is **almost entirely absent as an explicit concept.** There is no `StuckDetector`, no `doom_loop` tracking, no consecutive-error counter on tool calls, no identical-argument detector, no alternating A/B detector, and no LLM-judged "is the agent making progress?" check inside the agent loop. The only mechanism that bounds a runaway agent is the **hard iteration cap `DEFAULT_MAX_STEPS = 50`** (`letta/constants.py:75`), which terminates the loop with `StopReasonType.max_steps` (`letta/schemas/letta_stop_reason.py:15`) once `i == max_steps - 1` (`letta/agents/letta_agent_v3.py:394-395`, `letta/agents/letta_agent_v2.py:408-409`). Beyond the cap, the only structural defenses are **tool-rule plumbing** (`letta/helpers/tool_rule_solver.py`, `letta/schemas/tool_rule.py`) — `MaxCountPerStepToolRule` (per-step ceiling for one tool, `letta/schemas/tool_rule.py:336-342`), `TerminalToolRule` (exit when a tool is called), `RequiredBeforeExitToolRule` (force a tool to be called before exit), `ContinueToolRule` (force loop to continue), and `InitToolRule`/`ChildToolRule`/`ParentToolRule`/`ConditionalToolRule` (constrain which tools can be called next based on `tool_call_history`). These rules track `tool_call_history: list[str]` (`letta/helpers/tool_rule_solver.py:51`) only to filter the next allowed set, **not** to detect repetition; the same `search_memory` tool called with the same arguments 20 times in a row will be allowed. The legacy V1 agent has a one-step guard that prevents the same tool from being called again if it failed on the previous turn (`letta/agent.py:325-326`) — a 1-strike fingerprint — but V2/V3 (`LettaAgentV2`, `LettaAgentV3`) dropped even that, so failure-driven loops just persist. **Compaction** (`letta/services/summarizer/compact.py`) is a context-window management feature (triggered when `context_token_estimate > context_window * SUMMARIZATION_TRIGGER_MULTIPLIER`, `letta/constants.py:83`), not a loop intervention. There is also an infrastructure-level event-loop watchdog (`letta/monitoring/event_loop_watchdog.py`) that flips the readiness gate to degraded when the asyncio loop is starved, but that is a server-watchdog for CPU starvation, not an agent-stuck detector.

## Rating

**Score: 2 / 10**

Rationale: Letta's only real "stop repeated behavior" defense is the hard `DEFAULT_MAX_STEPS = 50` ceiling. It does not count consecutive errors, does not fingerprint identical successful tool calls, does not detect alternating-tool patterns, does not measure non-progress, and does not provide any warning, hint, or replan intervention before terminating — it simply labels the run `max_steps` and returns. The V1 legacy agent has a one-step "no-retry-after-failure" check (`letta/agent.py:325-326`); V2/V3 inherit no equivalent. The tool-rule system can be configured to behave like a loop boundary (e.g. `TerminalToolRule` ends on a specific tool, `RequiredBeforeExitToolRule` requires a specific tool) but it is **declarative scaffolding, not detection** — it requires the agent author to anticipate the stuck pattern at config-time, and lets the same tool fire with the same args until `max_steps` if no rule constrains it. The Magentic-style "LLM-judge-is-progress-being-made" detector that other agent frameworks ship is not present here. Compaction is independent of loop state and cannot be steered by it. The system will routinely pay for the full 50 turns on a doomed trajectory before stopping.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

### Hard cap (the only "loop stop")

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default iteration ceiling | `DEFAULT_MAX_STEPS = 50` constant | `letta/constants.py:74-75` |
| `StopReasonType.max_steps` enum value | one of 13 stop reasons | `letta/schemas/letta_stop_reason.py:9-22` |
| V3 blocking-loop `for i in range(max_steps)` | top-level step loop | `letta/agents/letta_agent_v3.py:328` |
| V3 streaming-loop `for i in range(max_steps)` | top-level step loop | `letta/agents/letta_agent_v3.py:569` |
| V3 sets `StopReasonType.max_steps` on last iteration when not already set | `if i == max_steps - 1 and self.stop_reason is None: self.stop_reason = LettaStopReason(stop_reason=StopReasonType.max_steps.value)` | `letta/agents/letta_agent_v3.py:394-395` |
| Same guard in streaming path | identical condition in `stream()` | `letta/agents/letta_agent_v3.py:628-629` |
| V2 blocking-loop guarded stop on last iteration | `if self.stop_reason is None: self.stop_reason = LettaStopReason(stop_reason=StopReasonType.max_steps.value)` | `letta/agents/letta_agent_v2.py:407-409` |
| V2 streaming path terminates with `max_steps` when not already stopped | identical fallback | `letta/agents/letta_agent_v2.py:407-410` |
| Batch agent step ceiling check | `if step_count >= self.max_steps:` | `letta/agents/letta_agent_batch.py:597-599` |
| Batch path explicit "premature stop" log | `logger.warning("Hit max steps, stopping agent loop prematurely.")` | `letta/agents/letta_agent_batch.py:599` |

### Tool-rule scaffolding (declarative, not detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `tool_call_history: list[str]` — single field used for per-rule evaluation, **not** a sliding-window detector | `tool_call_history: list[str] = Field(default_factory=list, description="History of tool calls, updated with each tool call.")` | `letta/helpers/tool_rule_solver.py:51` |
| `register_tool_call` appends to history; `clear_tool_history` clears it (called at top of legacy agent loop, `letta/agent.py:766`) | append/clear primitives | `letta/helpers/tool_rule_solver.py:88-94` |
| `MaxCountPerStepToolRule.get_valid_tools` ceiling per step | `count = tool_call_history.count(self.tool_name); if count >= self.max_count_limit: return available_tools - {self.tool_name}` | `letta/schemas/tool_rule.py:334-342` |
| `TerminalToolRule` ends the agent when this tool is called | `is_terminal_tool` / `LettaStopReason(stop_reason=StopReasonType.tool_rule.value)` in `_decide_continuation` | `letta/helpers/tool_rule_solver.py:174-176`, `letta/agents/letta_agent_v3.py:2010-2012` |
| `RequiredBeforeExitToolRule` requires tool before exit; emits `ToolRuleViolated` heartbeat | `uncalled = tool_rules_solver.get_uncalled_required_tools(...); ... reason = f"{NON_USER_MSG_PREFIX}ToolRuleViolated: You must call ..."` | `letta/agents/letta_agent_v3.py:1626-1630`, `1994-1997` |
| `ChildToolRule` restricts the next-step tool set based on `tool_call_history[-1]` | `last_tool = tool_call_history[-1]; return set(self.get_child_names()) if last_tool == self.tool_name else available_tools` | `letta/schemas/tool_rule.py:111-113` |
| `ParentToolRule` reverse direction | same pattern at the parent checkpoint | `letta/schemas/tool_rule.py:155-157` |
| `ConditionalToolRule` maps last function output to a child tool | reads `last_function_response` JSON, never detects repetition | `letta/schemas/tool_rule.py:198-223` |
| `should_force_tool_call` decides `tool_choice="required"` when constrained | only at the moment of `_step` start | `letta/helpers/tool_rule_solver.py:273-298` |
| `_decide_continuation` V3 default behavior — keep stepping on any successful tool call | `continue_stepping = True  # Default continue` | `letta/agents/letta_agent_v3.py:1987` |
| `_decide_continuation` V3 honors `TerminalToolRule`, `ContinueToolRule`, parent-child | applies to single tool_call_name | `letta/agents/letta_agent_v3.py:2003-2036` |
| Parallel tool calls always continue unless terminal hit | `if not has_terminal and not is_max_steps: aggregate_continue = True` | `letta/agents/letta_agent_v3.py:1956-1962` |

### V1 single-shot failure guard (not in V2/V3)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Legacy V1 agent forbids repeating the last tool if it failed on the previous step | `if last_function_failed and self.tool_rules_solver.tool_call_history: allowed_tool_names = [f for f in allowed_tool_names if f != self.tool_rules_solver.tool_call_history[-1]]` | `letta/agent.py:325-326` |
| Function-failed signal is set by exception branch in `_handle_ai_response` | `return messages, False, True  # force a heartbeat to allow agent to handle error` | `letta/agent.py:629`, `letta/agent.py:666`, `letta/agent.py:687` |
| V1 chaining re-arms failed-chain with `FUNC_FAILED_HEARTBEAT_MESSAGE` heartbeat text | `content = get_heartbeat(self.agent_state.timezone, FUNC_FAILED_HEARTBEAT_MESSAGE)` | `letta/agent.py:828-832` (`FUNC_FAILED_HEARTBEAT_MESSAGE` defined `letta/constants.py:455`) |
| **V2/V3 do not enforce this guard** — no equivalent of `last_function_failed` filter in `LettaAgentV2._get_valid_tools` or `LettaAgentV3._get_valid_tools` | `tools = self.agent_state.tools; valid_tool_names = self.tool_rules_solver.get_allowed_tool_names(...) or list(set(t.name for t in tools))` | `letta/agents/letta_agent_v3.py:2039-2045` |

### Cancellation (external interruption, not detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-step check that the run was cancelled by an external actor | `if run_id and await self._check_run_cancellation(run_id):` | `letta/agents/letta_agent_v3.py:1030-1035` |
| `_check_run_cancellation` reads `RunStatus.cancelled` from run manager | `run = await self.run_manager.get_run_by_id(...); return run.status == RunStatus.cancelled` | `letta/agents/letta_agent_v2.py:750-757` |
| Cancellation at loop-exit → `StopReasonType.cancelled` | `if not self.should_continue and self.stop_reason.stop_reason == StopReasonType.cancelled.value: break` | `letta/agents/letta_agent_v3.py:359`, `letta/agents/letta_agent_v3.py:616` |

### Compaction (context-window, not loop-driven)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `DEFAULT_MAX_STEPS = 50` — the only iteration ceiling constant | | `letta/constants.py:74-75` |
| `SUMMARIZATION_TRIGGER_MULTIPLIER = 0.9` — context-window compaction trigger | comment: "Summarization triggers when step usage > context_window * SUMMARIZATION_TRIGGER_MULTIPLIER" | `letta/constants.py:81-83` |
| `get_compaction_trigger_threshold` returns `int(context_window * 0.9)` | GPT-5 family trigger is 90%, all others also 90% (current default) | `letta/services/summarizer/thresholds.py:27-41` |
| V3 mid-step compaction on context overflow | `if self.context_token_estimate is not None and self.context_token_estimate > compaction_trigger_threshold:` then `await self.compact(...)` | `letta/agents/letta_agent_v3.py:1438-1469` |
| V3 retries with compaction on `ContextWindowExceededError` | comment: "Context window exceeded (error {e}), trying to compact messages attempt {llm_request_attempt + 1} of {summarizer_settings.max_summarizer_retries + 1}" | `letta/agents/letta_agent_v3.py:1219-1280` |
| Compaction wraps `context_token_estimate` based summarization, not loop-state — it fires identically whether the agent is making progress or stuck | | `letta/services/summarizer/compact.py:140-216` |

### Voice-agent streaming loop (parallel implementation, no detection)

| Area | Evidence | File:Line |
|------|----------|-----------|
| Voice `step_stream` is `for _ in range(max_steps):` and only breaks on `not should_continue` | unconditional iterate, identical guard | `letta/agents/voice_agent.py:169-194` |

### Infrastructure watchdog (NOT an agent-stuck detector)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `EventLoopWatchdog` only flips readiness to `degraded` on event-loop lag | `self._maybe_degrade_readiness(now, current_lag_ms)` | `letta/monitoring/event_loop_watchdog.py:212-237` |
| `LoadGate` tracks fg/bg task counts and degrades on threshold | `_maybe_degrade_fg`, `_maybe_degrade_bg` | `letta/monitoring/load_gate.py:139-231` |
| Standalone test script for the event-loop watchdog | `test_watchdog_hang.py` (not a unit test for stuck-detection in agent logic) | `test_watchdog_hang.py:1-97` |

## Answers to Dimension Questions

1. **What stuck patterns are detected?**
   Only "you exceeded `DEFAULT_MAX_STEPS = 50` iterations." No detector for identical successful tool calls, alternating A/B sequences, "same tool returning the same response," or "no-progress monologues." Rule-driven exits (`TerminalToolRule`, `RequiredBeforeExitToolRule`) stop on **specific tool names configured at author time**, not on detected repetition. The V1 legacy guard `letta/agent.py:325-326` is the only "agent called this and got an error" check, and only blocks the immediately next call.

2. **How far back does detection look?**
   Not applicable — there is no sliding-window loop detector. The `tool_call_history: list[str]` (`letta/helpers/tool_rule_solver.py:51`) is a per-run unbounded list used to filter the next allowed tool set, not to detect repetition. `tool_call_history` is cleared on step boundary via `clear_tool_history()` (`letta/agent.py:766`), and the only consumer that examines its length is `MaxCountPerStepToolRule` which counts occurrences of one tool name inside the **current step** only (`letta/schemas/tool_rule.py:336-342`).

3. **Does compaction erase loop evidence?**
   Compaction (`letta/services/summarizer/compact.py:140-216`) replaces messages with a summary text. It is not triggered by stuck state — it is triggered by `context_token_estimate > context_window * 0.9` (`letta/services/summarizer/thresholds.py:27-41`). When it fires, the in-memory `tool_call_history` is preserved on the `ToolRulesSolver` instance, but the resolved `Message` history that the LLM will see loses the raw tool-call → tool-return pairs. So even if a future "identify repeated tool call" detector existed, compaction would silently destroy its evidence.

4. **What intervention happens?**
   - **Stop with `StopReasonType.max_steps`**: hard cut-off at iteration 50 (`letta/agents/letta_agent_v3.py:394-395`).
   - **Stop with `StopReasonType.tool_rule`**: only when a configured `TerminalToolRule` tool is called (`letta/agents/letta_agent_v3.py:2010-2012`).
   - **Hint via `NON_USER_MSG_PREFIX`-prefixed heartbeat messages**: when `RequiredBeforeExitToolRule` is unmet, the loop injects `f"{NON_USER_MSG_PREFIX}ToolRuleViolated: You must call {...} at least once to exit the loop."` and continues (`letta/agents/letta_agent_v3.py:1626-1640`, `1996-1997`).
   - **External cancellation**: RunManager sets `RunStatus.cancelled`, next step returns `StopReasonType.cancelled` (`letta/agents/letta_agent_v3.py:1030-1035`).
   - **No escalation, no replan, no LLM-judge reset-and-replan** (which is what is missing relative to agent frameworks like Magentic).

5. **Are false positives possible?**
   Yes — in two ways. First, a legitimate agent trajectory that happens to need 51+ tool round-trips will be truncated as `max_steps`, with the user paying for the cut-off turn's LLM cost. Second, the V1 `last_function_failed` filter (`letta/agent.py:325-326`) blots out every allowed tool when only the last was failing — `if not allowed_tool_names: return None` (`letta/agent.py:327-328`) — which can short-circuit a recoverable trajectory on a one-step failure. V2/V3 do not have either of these issues because they have neither the counter nor the filter.

## Architectural Decisions

- **Iteration cap, not detector.** Letta deliberately chose a fixed ceiling over a sliding-window repetition detector. The relevant constants live in one place (`letta/constants.py:74-75`) and the cap is enforced at exactly one spot per loop variant (`letta/agents/letta_agent_v3.py:328`, `letta/agents/letta_agent_v2.py:233`). This is a "fail-safe at the budget boundary" decision rather than a "detect-and-correct" decision.
- **Declarative rule-based exit.** All exit-or-continue decisions funnel through `ToolRulesSolver` and `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967-2036`). The `TerminalToolRule`/`RequiredBeforeExitToolRule`/`ContinueToolRule` triad is meant to give agent authors explicit verbs ("end here," "force call this," "keep going") instead of letting the runtime detect when those behaviors are needed.
- **Tool-rule filtering on `tool_call_history[-1]`.** Every structural rule (`ChildToolRule`, `ParentToolRule`, `ConditionalToolRule`) keys off the most recent tool name (`letta/schemas/tool_rule.py:111-157`), not the last N calls. This makes the system expressive as a graph but blind to "called this 8 times in a row."
- **No persistent recent-event window.** There is no `RecentActions`, no `EventBuffer`, no `RingBuffer` of step-level decisions anywhere under `letta/agents/`. The state that does exist on a step (`step_metrics` in `letta/agents/letta_agent_v3.py:1402-1404`) is per-step telemetry flushed to the DB, not a recent-events sliding window consulted by the loop.
- **Compaction is context-window driven, not loop driven.** The 90% threshold in `letta/services/summarizer/thresholds.py:27-41` is the only "automatic" engine-issued intervention, and it is keyed off token count, not any measure of progress.

## Notable Patterns

- **Single point of stop-reason origination.** `_decide_continuation` returns `(continue_stepping, heartbeat_reason, stop_reason)` (`letta/agents/letta_agent_v3.py:1967-2036`) and the only `StopReasonType` values reachable inside the loop are `max_steps`, `tool_rule`, `end_turn`, `max_tokens_exceeded`, `requires_approval`, plus the exceptional exits (`invalid_tool_call`, `invalid_llm_response`, `llm_api_error`, `context_window_overflow_in_system_prompt`, `error`, `cancelled`, `insufficient_credits`). There is no `stuck`, `no_progress`, `repeated_call`, or `doom_loop` reason — `letta/schemas/letta_stop_reason.py:9-22` enumerates 13 reasons and none of them signals repetition.
- **Heartbeat messages as soft signals.** `NON_USER_MSG_PREFIX = "[This is an automated system message hidden from the user] "` (`letta/constants.py:244`) prefixes synthetic reminders. Three of them are stuck-style nudges: `ToolRuleViolated` (uncalled required tool), `Continuing: child tool rule`, `Continuing: continue tool rule`, `Continuing: tool rule violation` (`letta/agents/letta_agent_v3.py:1996-2020`, `letta/agents/letta_agent_v2.py:1256-1281`). These are **static** messages, not "you've called this 6 times now" counters.
- **Credit-check piggybacks on the loop.** `_check_credits` runs as a parallel `safe_create_task_with_return` between iterations (`letta/agents/letta_agent_v3.py:389-390`, `625-626`) and breaks with `StopReasonType.insufficient_credits` if it trips (`letta/agents/letta_agent_v3.py:336-337`, `577-579`). This is a *progress-uncoupled* interrupt designed to stop a billing-not-allowed loop, not a loop-detection mechanism per se, but it is the closest thing to "stop repeated behavior early" that exists.

## Tradeoffs

- **Simple to reason about.** The absence of stateful sliding-window detectors means the loop has minimal memory and no "false positive" risk from a stale fingerprint.
- **Cheap to operate.** No extra LLM call per step to judge progress, no per-step message-deduplication hash, no ring buffer.
- **Author burden is moved to schema.** Whoever defines an agent must anticipate stuck patterns at config time and set `TerminalToolRule` / `RequiredBeforeExitToolRule` accordingly. A "I never want the agent stuck" requirement is not expressible — only "force call X first" or "stop when X is called" is.
- **Late termination.** 50 full model calls + tool executions before anything resembling a stop. Compare to frameworks with explicit `MaxStallCount=3` and reset-and-replan at the 3rd stall — Letta has 17× that budget for a single stuck trajectory.
- **Asymmetry with `.NET`-Magentic-style judges.** External orchestrators can layer an LLM-judge on top of `LettaAgentV3.step`, but the platform does not provide one out of the box.

## Failure Modes / Edge Cases

- **Recursion via `send_message_to_agent_async`.** A `multi_agent` agent calling itself repeatedly has no inner guard; the only bound is the outer `max_steps` (set on the **caller**, not the recursive call). Evidence: `LOCAL_ONLY_MULTI_AGENT_TOOLS = ["send_message_to_agent_async"]` (`letta/constants.py:155`).
- **Spinning on a flaky remote tool.** With `MaxCountPerStepToolRule.max_count_limit` unset, the same tool can be re-attempted indefinitely; only `max_steps` saves the user. Evidence: absence of any error-count check in `_decide_continuation` (`letta/agents/letta_agent_v3.py:1967-2036`).
- **Compaction eats loop evidence.** Compaction is run on context overflow (`letta/services/summarizer/compact.py:140-216`) and `summarize_conversation_history` may be called as a safety net after the loop (`letta/agents/letta_agent_v3.py:397-410`, currently commented out but the API is in place). Once the raw messages are evicted, anything that depended on inspecting them is blind.
- **Cancellation only at step boundary.** `_check_run_cancellation` is called at `letta/agents/letta_agent_v3.py:1030-1035`, **inside** the `_step` method. A currently-executing LLM call or tool call cannot be interrupted; the cancellation is observed only at the next iteration boundary.
- **`max_steps` reached *and* cancellation.** Tests acknowledge this edge case explicitly: `assert result.stop_reason.stop_reason in ["cancelled", "max_steps"]` (`tests/managers/test_cancellation.py:1396-1398`).
- **Required-before-exit oscillation.** Without `is_final_step`, `RequiredBeforeExitToolRule` will keep emitting `ToolRuleViolated` heartbeats and continuing until `max_steps` caps the loop (`letta/agents/letta_agent_v3.py:2027-2034`), creating exactly the alternating-message pattern that the dimension is looking for.
- **V1 legacy guards dropped in V2/V3.** The single-shot "no-repeat-after-failure" filter (`letta/agent.py:325-326`) is not reimplemented in `LettaAgentV2._get_valid_tools` (`letta/agents/letta_agent_v2.py:1100-1130`) nor `LettaAgentV3._get_valid_tools` (`letta/agents/letta_agent_v3.py:2039-2045`). A flake on tool `T` allows the model to call `T` again on every subsequent iteration up to `max_steps`.

## Future Considerations

- Add a tiny **consecutive-error counter** (e.g. `consecutive_tool_failure_count`) on `LettaAgentV3` that bumps when `tool_execution_result.status == "error"` and resets on success, with a configurable threshold (default 3) and a `StopReasonType.stuck` enum value surfaced to the run manager.
- Promote `last_function_failed` filtering to V2/V3 so a tool that errored on the previous iteration cannot be invoked next without explicit reason.
- Implement a **sliding window** `recent_tool_calls: deque[(name, args_hash), maxlen=N]` consulted in `_decide_continuation`; emit `ToolRuleViolated`-style heartbeat after K consecutive identical (name, args_hash) hits; trip `StopReasonType.max_steps` if M hits occur across the budget.
- Add a **progress-judge** optional middleware that, at configurable step interval, calls a lightweight LLM with the last 5–10 messages and asks "is the agent making progress?" — then escalates to a `replan` heartbeat or hard stop. This would parallel the pattern in Magentic-style orchestrators without locking it into a specific framework.
- Surface the historical sequence on the run record. Today `StepMetrics` (`letta/agents/letta_agent_v3.py:1864-1866`, `letta/schemas/step_metrics.py`) records timing only — adding a `tool_call_sequence: list[str]` summary per step would make retrospective stuck-analysis possible without runtime detection.
- Distinguish **system-prompt-fits** (`tool_call = None`) from **fail-to-progress** (`tool_call = same X for N iterations`). The current `StopReasonType` enum encodes neither; `end_turn` is used for both successful delivery and silent stagnation.

## Questions / Gaps

- No evidence of any `doom_loop`, `stuck`, `stagnation`, `no_progress`, `consecutive_error`, `recent_action_window`, or `history_fingerprint` symbol under `letta/` beyond what's listed above. Searches performed: `stuck|doom|loop_detect|repetit|same_tool_calls|consecutive|repeat_call|hammer|spinning|recent_calls|tool_repeat` against `letta/**/*.py`.
- No evidence of an explicit "I called this same tool with this same argument N times" check anywhere in the agent code path. The only consumers of `tool_call_history` are the rule evaluators (`letta/helpers/tool_rule_solver.py:96-298`, `letta/schemas/tool_rule.py:32-372`), and none of them implement a sliding-window repetition threshold.
- No evidence of an LLM-judged "IsInLoop" / "IsProgressBeingMade" midpoint. The Magentic-style judge is absent.
- No evidence of an "alternating A/B" detector. The conditional rule can be configured to *force* alternation but never detects non-alternation.
- No LLM-call-time hint injection when the model is mid-spin. All hints are rule-bound (`Terminal`, `RequiredBeforeExit`, `Continue`, `Child`).
- Whether `summarize_conversation_history` is invoked post-loop in practice is currently commented out in V3 (`letta/agents/letta_agent_v3.py:397-410`); the call is preserved but disabled. Without empirical telemetry on runs, the *practical* effect of compaction on loop evidence remains unclear.

---

Generated by `03.06-stuck-and-doom-loop-detection` against `letta`.
