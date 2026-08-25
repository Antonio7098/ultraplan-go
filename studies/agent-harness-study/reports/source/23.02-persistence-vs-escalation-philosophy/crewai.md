# Source Analysis: crewai

## Dimension 23.02: Persistence vs Escalation Philosophy

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based agent framework, asyncio-aware) |
| Analyzed | 2026-08-24 |

## Summary

CrewAI's philosophy is **persist-with-bounded-retries, then force termination or escalate**. Persistence is layered at four distinct scopes, each with its own configurable counter: (1) the executor's ReAct loop is capped by `max_iter` (default 25) and responds to exhaustion not by failing but by making one more LLM call that demands a final answer (`lib/crewai/src/crewai/utilities/agent_utils.py:376-433`, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306`); (2) whole-task failures are retried by the Agent up to `max_retry_limit` (default 2), with a deliberate passthrough list so "deliberate stops" like `ToolExecutionFailedError` are never swallowed into the retry loop (`lib/crewai/src/crewai/agent/core.py:139-141`, `lib/crewai/src/crewai/agent/core.py:721-747`); (3) guardrail violations trigger re-execution with the validation error fed back as context, bounded by `guardrail_max_retries` at task level (`lib/crewai/src/crewai/task.py:1321-1439`) and kickoff level (`lib/crewai/src/crewai/agent/core.py:1885-1967`, `lib/crewai/src/crewai/lite_agent.py:742-772`); (4) provider-level HTTP retries default to `max_retries=2` in each LLM completion client (`lib/crewai/src/crewai/llms/providers/openai/completion.py:223`). Escalation paths are explicit and pluggable: human-in-the-loop feedback loops per task (`lib/crewai/src/crewai/task.py:233`, `lib/crewai/src/crewai/core/providers/human_input.py:237-264`), durable flow pause/resume for async HITL (`lib/crewai/src/crewai/flow/runtime/__init__.py:1524-1557`), peer delegation gated by `allow_delegation` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297-300`), a three-way `ToolFailurePolicy` of IGNORE/WARN/RAISE resolved tool→task→agent→crew (`lib/crewai/src/crewai/tools/tool_failure.py:57-68`, `tool_failure.py:177-208`), and post-hoc recovery via `Crew.replay(task_id)` (`lib/crewai/src/crewai/crew.py:2031-2048`). Nearly every persistence decision is observable through the event bus (`AgentExecutionErrorEvent`, `ToolFailureDetectedEvent`, guardrail events carrying `retry_count`, reasoning events carrying `attempt`, `FlowPausedEvent`). The model is coherent and well tested; its main weaknesses are un-reset retry counters that leak across tasks and a scattered set of overlapping knobs with no unified autonomy dial.

## Rating

**8 / 10** — A clear, layered persistence model with explicit escalation paths, event-bus observability at every decision point, and strong test coverage of retry semantics. Falls short of 9–10 because: the agent-level retry counter `_times_executed` is never reset (making it an agent-lifetime budget shared across tasks rather than per-task semantics), the many retry knobs (`max_iter`, `max_retry_limit`, task vs agent `guardrail_max_retries`, provider `max_retries`) have no unified policy layer or documented interaction model, retries use no backoff, and deprecated duplicate fields (`Task.max_retries`) still coexist with their replacements.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Iteration cap config | `max_iter: int = Field(default=25)` on every agent | lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306 |
| Executor loop state | `iterations: int = 0`, `max_iter: int = 25` fields on executor state | lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:26-27 |
| Max-iteration handling | On cap, one more LLM call appends `force_final_answer` error text and returns an `AgentFinish`; prints "Maximum iterations reached. Requesting final answer." | lib/crewai/src/crewai/utilities/agent_utils.py:376-433 |
| Cap check in ReAct loop | `has_reached_max_iterations(...)` checked each pass before forcing final answer | lib/crewai/src/crewai/agents/crew_agent_executor.py:343-352 |
| Task-level retry budget | `max_retry_limit: int = Field(default=2)` — "Maximum number of retries ... when an error occurs" | lib/crewai/src/crewai/agent/core.py:255-258 |
| Retry decision logic | `_check_execution_error`: emit error event, skip litellm errors, `_times_executed += 1`, raise when `> max_retry_limit` | lib/crewai/src/crewai/agent/core.py:721-747 |
| Passthrough exceptions | `_passthrough_exceptions = (ToolExecutionFailedError,)` — "Deliberate stops, not transient errors: never swallowed into the max_retry_limit loop" | lib/crewai/src/crewai/agent/core.py:139-141 |
| Sync retry path | `_handle_execution_error` re-invokes `execute_task` after passing the check | lib/crewai/src/crewai/agent/core.py:749-768 |
| Parse-error self-correction | `handle_output_parser_exception` appends the parser error into messages so the same loop retries within budget | lib/crewai/src/crewai/utilities/agent_utils.py:743-778 |
| Context-window persistence | `handle_context_length` summarizes messages to continue when `respect_context_window=True`, else `SystemExit` | lib/crewai/src/crewai/utilities/agent_utils.py:795-832 |
| Guardrail retry budget (task) | `guardrail_max_retries` default 3 on Task; deprecated alias `max_retries` | lib/crewai/src/crewai/task.py:275-280 |
| Task guardrail retry loop | Up to `guardrail_max_retries + 1` attempts; blocked output becomes retry `context`; raises when exhausted | lib/crewai/src/crewai/task.py:1321-1439 |
| Per-guardrail counters | `_guardrail_retry_counts: dict[int, int]` tracks each guardrail independently | lib/crewai/src/crewai/task.py:304 |
| Kickoff-level guardrail retry | Recursive `_process_kickoff_guardrail` feeds the guardrail error back as a user message and re-executes | lib/crewai/src/crewai/agent/core.py:1885-1967 |
| LiteAgent guardrail retry | `_guardrail_retry_count` incremented, error appended, `_execute_core` re-entered | lib/crewai/src/crewai/lite_agent.py:742-772 |
| Failure accumulation across retries | `merge_tool_failures` keeps blocked-attempt failures visible on the final output | lib/crewai/src/crewai/tools/tool_failure.py:211-234 |
| Tool failure policy enum | `IGNORE` / `WARN` (default) / `RAISE` — how agents react to reported tool failure | lib/crewai/src/crewai/tools/tool_failure.py:57-68 |
| Policy resolution precedence | Most-specific-wins chain: tool → original_tool → task → agent → crew → WARN; invalid policy degrades to warning, never aborts | lib/crewai/src/crewai/tools/tool_failure.py:177-208 |
| RAISE escalation | Policy RAISE emits `ToolFailureDetectedEvent` then throws `ToolExecutionFailedError` | lib/crewai/src/crewai/tools/tool_failure.py:324-384 |
| Human-input flag | `human_input: bool \| None` on Task | lib/crewai/src/crewai/task.py:233 |
| HITL feedback loop | `while context.ask_for_human_input:` — re-prompts and re-invokes the loop until empty feedback ends it | lib/crewai/src/crewai/core/providers/human_input.py:237-264 |
| Pluggable HITL providers | `HumanInputProvider` protocol + `set_provider()` ContextVar override | lib/crewai/src/crewai/core/providers/human_input.py:59-130, 451-483 |
| Durable flow pause | `HumanFeedbackPending` caught → state saved via `persistence.save_pending_feedback` → `FlowPausedEvent` emitted | lib/crewai/src/crewai/flow/runtime/__init__.py:1524-1557 |
| Flow resume | `from_pending(flow_id)` restores persisted state; `resume(feedback)` continues execution | lib/crewai/src/crewai/flow/runtime/__init__.py:1240-1266, 1285-1359 |
| Delegation escalation | `allow_delegation: bool` gates delegate/ask-coworker tools | lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297-300 |
| Delegation tracking | `DELEGATION_TOOL_NAMES` + `track_delegation_if_needed` increments task delegations | lib/crewai/src/crewai/utilities/agent_utils.py:1341-1363 |
| Planning depth | `PlanningConfig.max_attempts` (default 3) and `max_steps` (10) bound the refine loop | lib/crewai/src/crewai/agent/planning_config.py:11, 98-105; consumed at lib/crewai/src/crewai/utilities/reasoning_handler.py:293-296 |
| Pre-execution replanning | Refines plan until `ready`; logs "reached maximum attempts ... Proceeding with current plan" | lib/crewai/src/crewai/utilities/reasoning_handler.py:278-349 |
| Anti-deadlock fallback | Planning failure defaults to `ready=True` "to avoid getting stuck" | lib/crewai/src/crewai/utilities/reasoning_handler.py:438-444 |
| Crew-level plan-ahead | `Crew.planning=True` runs `CrewPlanner._handle_crew_planning()` producing per-task plans before execution | lib/crewai/src/crewai/crew.py:1451-1456; lib/crewai/src/crewai/utilities/planning_handler.py:57-78 |
| Post-hoc recovery | `Crew.replay(task_id, inputs)` restarts execution from a stored task output | lib/crewai/src/crewai/crew.py:2031-2048 |
| Time budget | `max_execution_time` enforced via `_execute_with_timeout`; TimeoutError emits error event and propagates (no retry) | lib/crewai/src/crewai/agent/core.py:211-214, 864-881 |
| Rate-limit pacing | `RPMController.check_or_wait` blocks until within `max_rpm` | lib/crewai/src/crewai/utilities/rpm_controller.py:12-66 |
| Provider HTTP retries | `max_retries: int = 2` on OpenAI/Azure/Anthropic completion clients | lib/crewai/src/crewai/llms/providers/openai/completion.py:223 |
| Observability: error events | `AgentExecutionErrorEvent` emitted once per failed attempt (retry scope reopened per attempt) | lib/crewai/src/crewai/agent/core.py:733-742 |
| Observability: guardrail events | Guardrail started/failed events carry `retry_count` | lib/crewai/src/crewai/events/types/llm_guardrail_events.py:34, 70; console prints "(attempt X/Y), retrying due to:" | lib/crewai/src/crewai/task.py:1398-1402 |
| Observability: reasoning attempts | Reasoning events carry `attempt=n` during refine loop | lib/crewai/src/crewai/utilities/reasoning_handler.py:300-308, 326-340 |
| Observability: telemetry | Telemetry reports `max_retry_limit` alongside agent attributes | lib/crewai/src/crewai/telemetry/telemetry.py:371, 475 |
| Autonomy config surface (declarative) | Hierarchical crew definitions expose `max_retry_limit`, `guardrail_max_retries`, `human_input` as schema fields | lib/crewai/src/crewai/project/crew_base.py:76, 94, 121, 125 |
| Tests: iteration cap | `test_check_max_iterations_not_reached` / `_reached`; force-final-answer routing test | lib/crewai/tests/agents/test_agent_executor.py:430-443, 2316-2326 |
| Tests: guardrail retries | Assertions that `task.retry_count == 1/2`; per-guardrail independent retry tracking; multiple-guardrail middle retry | lib/crewai/tests/test_task_guardrails.py:95, 120, 482, 734 |
| Tests: retry does not swallow aborts | `test_retry_limit_does_not_swallow_the_abort`; guardrail retry preserves earlier failures | lib/crewai/tests/tools/test_tool_failure.py:689, 811, 1600 |
| Tests: retry context | Guardrail feedback included in retry context; retried task asserted once | lib/crewai/tests/test_crew.py:4264-4269 |

## Answers to Dimension Questions

**1. Does the agent persist or escalate on failure?**
Both, by failure class. Transient/ambiguous errors persist: whole-task exceptions retry up to `max_retry_limit` (`lib/crewai/src/crewai/agent/core.py:745-747`), unparseable LLM output is fed back into the conversation for another iteration (`lib/crewai/src/crewai/utilities/agent_utils.py:763`), context overflow triggers summarization-and-continue (`lib/crewai/src/crewai/utilities/agent_utils.py:815-823`), and guardrail violations re-execute the task with the violation as corrective context (`lib/crewai/src/crewai/task.py:1394-1408`). Deliberate escalations stop persistence: `litellm`-module errors are re-raised immediately without retrying (`lib/crewai/src/crewai/agent/core.py:743-744`), `ToolExecutionFailedError` bypasses the retry loop entirely by design (`lib/crewai/src/crewai/agent/core.py:139-141`), exhausted guardrails raise with the last error (`lib/crewai/src/crewai/task.py:1382-1385`), and timeouts propagate uncaught (`lib/crewai/src/crewai/agent/core.py:872-881`). The signature behavior on *budget exhaustion* (max iterations) is neither persist nor fail: the harness forces a best-effort final answer with one extra LLM call (`lib/crewai/src/crewai/utilities/agent_utils.py:384-415`).

**2. Is persistence configurable?**
Extensively, at four scopes. Agent: `max_iter` (25 default, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306`), `max_retry_limit` (2, `lib/crewai/src/crewai/agent/core.py:255-258`), `guardrail_max_retries` (3, `lib/crewai/src/crewai/agent/core.py:326-328`), `max_execution_time`, `max_rpm`, `respect_context_window`. Task: per-task `guardrail_max_retries` and `human_input` (`lib/crewai/src/crewai/task.py:233, 275-282`). Crew/tool: `tool_failure_policy` inheritable down the crew→agent→task→tool chain (`lib/crewai/src/crewai/tools/tool_failure.py:177-208`). Provider: per-client `max_retries` (`lib/crewai/src/crewai/llms/providers/openai/completion.py:223`). Planning depth via `PlanningConfig.max_attempts` (`lib/crewai/src/crewai/agent/planning_config.py`; consumed at `lib/crewai/src/crewai/utilities/reasoning_handler.py:293-296`). All are declarative-config accessible (`lib/crewai/src/crewai/project/crew_base.py:76-125`). Caveat: there is no single "autonomy level" abstraction; operators must compose individual knobs.

**3. Are escalation paths clear?**
Yes, mostly. The clearest is the tool-failure policy: an explicit three-value enum with documented precedence and a hard-abort exception type that is deliberately exempt from retries (`lib/crewai/src/crewai/tools/tool_failure.py:57-68`; `lib/crewai/src/crewai/agent/core.py:139-141`). Human escalation is clear per task (`task.human_input` → interactive feedback loop, `lib/crewai/src/crewai/core/providers/human_input.py:256-264`) and durable at flow level (pause persists state + emits `FlowPausedEvent`, resume via webhook-style `from_pending`/`resume`, `lib/crewai/src/crewai/flow/runtime/__init__.py:1539-1557, 1285-1336`). Peer delegation is opt-in via `allow_delegation` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297-300`). Weaker spots: escalation on repeated LLM failure is implicit (the litellm passthrough relies on module-name sniffing, `lib/crewai/src/crewai/agent/core.py:743`), and "ask a human" is only reachable if configured up front — there is no runtime rule that promotes a task to HITL after N failed retries.

**4. Are persistence decisions observable?**
Largely yes. Every retry decision point emits a typed event on `crewai_event_bus`: per-attempt `AgentExecutionErrorEvent` (`lib/crewai/src/crewai/agent/core.py:735-742`), `ToolFailureDetectedEvent` including the chosen policy (`lib/crewai/src/crewai/tools/tool_failure.py:362-379`), guardrail events with `retry_count` (`lib/crewai/src/crewai/events/types/llm_guardrail_events.py`; usage at `lib/crewai/src/crewai/task.py:1344-1351`), planning events with `attempt` numbers (`lib/crewai/src/crewai/utilities/reasoning_handler.py:300-340`), and `FlowPausedEvent` (`lib/crewai/src/crewai/flow/runtime/__init__.py:1545-1556`). Structured records survive onto outputs: `TaskOutput.tool_failures` carries deduplicated failure records accumulated across guardrail retries (`lib/crewai/src/crewai/tools/tool_failure.py:111-131, 211-234`; wired at `lib/crewai/src/crewai/task.py:1341, 1435`). Verbose mode adds human-readable traces ("Maximum iterations reached...", `lib/crewai/src/crewai/utilities/agent_utils.py:397-401`; "Guardrail ... blocked (attempt X/Y)", `lib/crewai/src/crewai/task.py:1398-1402`). Gaps: retry counters themselves (`_times_executed`, `iterations`) are private state with no events emitted when a budget is set or reset; the forced-final-answer decision is printer-only unless verbose.

## Architectural Decisions

1. **Budgeted persistence instead of open-ended loops.** Every loop has a numeric ceiling — iterations (`base_agent_executor.py:26-27`), task retries (`core.py:255`), guardrail attempts (`task.py:1337`), planning attempts (`reasoning_handler.py:293`) — and each ceiling has a defined terminal action (force answer, raise, proceed-with-plan).
2. **Graceful degradation over hard failure at the iteration cap.** Rather than throwing when `max_iter` is hit, the harness spends one more call demanding a final answer (`agent_utils.py:384-433`) — prioritizing returning something over correctness.
3. **Exception taxonomy separates "transient" from "deliberate".** A dedicated `_passthrough_exceptions` tuple plus module-prefix checks keep intentional aborts out of the retry machinery (`core.py:139-141, 743-744`), while everything else is presumed retryable.
4. **Corrective-feedback retries.** Guardrail and parse failures are converted into conversational context (appended messages / retry `context` string) so the next attempt can actually fix the problem rather than blind-retry (`task.py:1394-1408`; `agent_utils.py:763`; `core.py:1937-1940`).
5. **Policy resolution as a chain of responsibility.** Tool-failure reactions resolve through tool → task → agent → crew → default, letting teams set autonomy once at crew scope and override narrowly (`tool_failure.py:177-208`); malformed policies degrade to WARN instead of breaking execution.
6. **Durability at the escalation boundary.** The moment a run needs a human, Flow persists full state (`save_pending_feedback`, `runtime/__init__.py:1539-1543`) so escalation survives process death — persistence effort is invested exactly where waits become long.

## Notable Patterns

- **Recursive self-call for guardrail retry** (`_process_kickoff_guardrail` calls itself with `retry_count+1`, `core.py:1952-1959`) and **loop-with-counter** variants elsewhere (`task.py:1343`, `lite_agent.py:752-772`) — two idioms for the same policy.
- **Failure-record accumulation across attempts**: `merge_tool_failures` dedupes by identity tuple so blocked-attempt failures stay on the final output (`tool_failure.py:211-234`; applied at `task.py:1370-1372, 1435`) — observability designed against information loss on retry.
- **Per-guardrail independent counters**: `_guardrail_retry_counts: dict[int, int]` allows multiple chained guardrails, each with its own budget (`task.py:304, 1387-1389`), covered by tests (`tests/test_task_guardrails.py:734`).
- **ContextVar-scoped collectors**: tool-failure collection is isolated per concurrent execution via ContextVar so shared agents don't cross-contaminate records (`tool_failure.py:260-280`).
- **Pluggable escalation endpoints**: both human input (`set_provider`, `human_input.py:465-474`) and executors (`executor_class` map, `core.py:143-146`) are swappable behind protocols/maps.
- **Anti-starvation guards**: planning falls back to `ready=True` on repeated failure "to avoid getting stuck" (`reasoning_handler.py:440-444`), and parse errors only start logging after `log_error_after=3` iterations (`agent_utils.py:743-750`).

## Tradeoffs

- **Bounded budgets vs correctness**: forcing a final answer at `max_iter` guarantees termination but can launder an incomplete investigation into a confident-looking output; users get no structured signal distinguishing forced answers from natural completion beyond verbose logs.
- **Passthrough-by-module-name fragility**: escalating immediately on `e.__class__.__module__.startswith("litellm")` couples policy to provider package layout (`core.py:743`); a wrapped or vendored client would silently change retry behavior.
- **Instance-level counters vs task semantics**: `_times_executed` is a private attribute incremented but never reset anywhere in the source (only occurrences: `core.py:208, 745-746`), so an agent shared across tasks in a crew spends one lifetime retry budget across all of them — cheap to implement, surprising to reason about, invisible to observers.
- **Many knobs, one philosophy**: five separate retry counters (iterations, task retries, task guardrails, kickoff guardrails, provider retries) plus RPM/time limits give fine control but no aggregate view or unified documentation of interaction order (provider retry fires inside one iteration, which itself counts toward `max_iter`).
- **WARN default favors liveness**: tool failures recorded-but-continued means runs complete with degraded data unless teams consciously choose `RAISE` (`tool_failure.py:63-68`).

## Failure Modes / Edge Cases

- **Retry-budget bleed-through**: since `_times_executed` never resets, a flaky first task can exhaust the budget so a later, different task fails on its first exception (`core.py:208, 745-746`). No test covers multi-task counter sharing.
- **Forced-final-answer can still fail**: if the extra LLM call returns empty, a plain `ValueError` is raised (`agent_utils.py:417-423`), which then enters the normal task-retry path — the interaction between the two budgets is unspecified.
- **Silent policy degradation**: an invalid `tool_failure_policy` string logs a warning and behaves as WARN (`tool_failure.py:200-207`) — safe, but a typo'd `"raise"` quietly removes abort semantics.
- **HITL deadlock risk in sync flows**: the sync feedback loop terminates only on empty input (`human_input.py:256-264`); a non-interactive environment would block indefinitely unless a custom provider replaces stdin reading.
- **Timeout is non-retryable by design**: `TimeoutError` propagates immediately even though some timeouts are transient (`core.py:872-881`), inconsistent with the retry-everything-else stance.
- **Deprecated dual fields**: `Task.max_retries` still parses and maps to `guardrail_max_retries` (`task.py:275-277, 574`), creating two names for one knob during migration.

## Future Considerations

- Reset or scope `_times_executed` per task execution (and emit an event when a retry budget is consumed/exhausted) to make the retry ledger observable and predictable.
- Introduce backoff/jitter between retries — current retries are immediate, hammering failing endpoints at the same rate as the initial attempt.
- Consolidate the retry knobs under a single `PersistencePolicy` object (mirroring `ToolFailurePolicy`) with named presets (e.g., cautious/balanced/aggressive) to serve as the missing autonomy dial.
- Emit typed events for forced-final-answer and budget-exhaustion decisions so downstream consumers can distinguish them from organic completions without parsing logs.
- Replace module-prefix exception sniffing with an explicit exception-type allowlist for non-retryable provider errors.

## Questions / Gaps

- No evidence found of any code resetting `_times_executed` between tasks or kickoffs; searched all non-test usages (`grep _times_executed` → only declaration and increment sites). If intentional, the semantics are undocumented outside field descriptions.
- No evidence found of a documented interaction ordering between provider `max_retries` and executor `max_iter` (i.e., whether provider retries consume iterations). Searched executor loops and provider clients; nothing reconciles the two budgets.
- No evidence found of runtime-triggered escalation (auto-prompt-for-human after N failures); HITL is strictly pre-configured via `human_input`/hooks (`task.py:233`; hooks API at `lib/crewai/src/crewai/hooks/tool_hooks.py:86-107`).
- No evidence found of persistence/backoff configuration exposed in YAML/JSON crew definitions beyond the flat integer fields listed in `project/crew_base.py:76-125`.

---

Generated by `Dimension 23.02: Persistence vs Escalation Philosophy` against `crewai`.
