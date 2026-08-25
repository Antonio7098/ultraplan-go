# Source Analysis: crewai

## Dimension 15.02: Message Routing and Termination

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based multi-agent framework; monorepo with `lib/crewai`, `lib/crewai-core`, `lib/crewai-tools`) |
| Analyzed | 2026-08-25 |

> All citations below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI implements message routing and termination across three orchestration layers, each with a distinct routing model:

1. **Crews** (`Process.sequential` / `Process.hierarchical`, `lib/crewai/src/crewai/process.py:4-11`). Routing is *task-driven*, not free-form conversation. In sequential mode, tasks execute in declared order and each task is bound to a fixed agent (`lib/crewai/src/crewai/crews/utils.py:154-160`). In hierarchical mode, an LLM "manager" agent acts as the speaker selector: it chooses coworkers through injected delegation tools (`Delegate work to coworker`, `Ask question to coworker`) whose arguments name the target agent by role (`lib/crewai/src/crewai/crew.py:1518-1548`, `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:22-36`).
2. **Flows** (event-graph orchestration). Methods are wired with `@start` / `@listen` triggers, `or_()`/`and_()` conditions (`lib/crewai/src/crewai/flow/dsl/_conditions.py:22-29`), and `@router` methods that return literal event names which become the next trigger (`lib/crewai/src/crewai/flow/dsl/_router.py:97-162`). The runtime executes routers until quiescence and then fans out listeners in parallel (`lib/crewai/src/crewai/flow/runtime/__init__.py:3048-3179`).
3. **Agent-internal loops** (ReAct or native tool calling) where "routing" reduces to tool selection, including the two inter-agent handoff tools.

Termination is layered rather than centralized: per-agent `max_iter` with a forced final answer (`lib/crewai/src/crewai/utilities/agent_utils.py:363-433`), optional wall-clock timeouts (`lib/crewai/src/crewai/agent/core.py:888-926`), bounded guardrail retries (`lib/crewai/src/crewai/task.py:1321-1439`), A2A `max_turns` caps for remote-agent conversations (`lib/crewai/src/crewai/a2a/wrapper.py:761-819`), and a flow-level `max_method_calls` circuit breaker that raises `RecursionError` on suspected infinite loops (`lib/crewai/src/crewai/flow/runtime/__init__.py:3248-3256`).

The dimension's headline question — *"Can a multi-agent conversation terminate without human intervention?"* — is **yes**: crews always terminate after the fixed task list completes (or raises); flows terminate when no listener/router trigger fires; agent loops are hard-bounded by `max_iter`; and human feedback is implemented as an explicit pause/resume exception rather than an implicit blocking wait.

## Rating

**7 / 10** — Clear routing models (sequential/hierarchical/flow DSL) with explicit interfaces, layered termination guarantees, dedicated deadlock guards (`max_method_calls`, repeated-tool-use detection, `max_usage_count`), and real tests for loop termination and delegation failure. Not rated higher because: hierarchical delegation cycles are only counted, never enforced (`lib/crewai/src/crewai/task.py:1142-1146`); repeated-tool detection compares only against the immediately preceding call (`lib/crewai/src/crewai/tools/tool_usage.py:779-789`); timeout cancellation leaks the underlying thread (`lib/crewai/src/crewai/agent/core.py:904-918`); and speaker selection degrades to first-match on sanitized role strings.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Process types define routing modes | `Process.sequential` / `hierarchical` enum; consensual commented out as TODO | `lib/crewai/src/crewai/process.py:4-11` |
| Process dispatch at kickoff | kickoff branches to `_run_sequential_process` / `_run_hierarchical_process`, else `NotImplementedError` | `lib/crewai/src/crewai/crew.py:1051-1058` |
| Sequential execution = ordered task list | `_run_sequential_process` → `_execute_tasks(self.tasks)` | `lib/crewai/src/crewai/crew.py:1509-1511` |
| Task→agent binding (sequential) | `_get_agent_to_use` returns manager in hierarchical mode, else `task.agent`; missing agent raises `ValueError` | `lib/crewai/src/crewai/crew.py:1714-1717`, `lib/crewai/src/crewai/crews/utils.py:154-160` |
| Async task fan-out + barrier before next sync task | futures collected, drained before next sync task / conditional task | `lib/crewai/src/crewai/crew.py:1597-1626` |
| Conditional task routing | `ConditionalTask` evaluated via `check_conditional_skip` against previous output | `lib/crewai/src/crewai/crew.py:1589-1595`, `lib/crewai/src/crewai/crews/utils.py:180-211` |
| Inter-task context passing | `_get_context` aggregates prior task outputs into prompt context | `lib/crewai/src/crewai/crew.py:1866-1874` |
| Manager agent creation (speaker selector) | role/goal/backstory from i18n; equipped with `AgentTools(...).tools()`; user manager forced to `allow_delegation=True` and stripped of tools | `lib/crewai/src/crewai/crew.py:1518-1548` |
| Manager persona prompts | `hierarchical_manager_agent.role/goal/backstory` ("Crew Manager") | `lib/crewai/src/crewai/translations/en.json:2-6` |
| Delegation tools | `DelegateWorkTool` + `AskQuestionTool` built over coworker roster | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:22-36` |
| Handoff resolution contract | case-insensitive, whitespace-normalized role matching; quotes stripped for weak LLM JSON; first match wins; unknown coworker returns error listing valid roles | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:20-35, 61-110` |
| Handoff execution semantics | creates ephemeral `Task` for target agent and calls `execute_task(task, context)` synchronously; result returned as tool observation string | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:112-124` |
| Peer delegation injection (sequential) | `_add_delegation_tools` injects delegation tools when crew has >1 agent and agent allows delegation | `lib/crewai/src/crewai/crew.py:1820-1830` |
| Manager per-task tool refresh | `_update_manager_tools` re-injects delegation scope per task | `lib/crewai/src/crewai/crew.py:1853-1863` |
| Delegation telemetry hook | `track_delegation_if_needed` maps tool names → `task.increment_delegations(coworker)` | `lib/crewai/src/crewai/utilities/agent_utils.py:1341-1363`, `lib/crewai/src/crewai/task.py:1138-1146` |
| Flow router decorator | `@router(condition, emit=...)` marks router methods; return literals/Enums become emitted events | `lib/crewai/src/crewai/flow/dsl/_router.py:97-162` |
| Flow condition algebra | `or_()` / `and_()` build condition trees consumed by the runtime | `lib/crewai/src/crewai/flow/dsl/_conditions.py:22-29` |
| Router chaining loop | `_execute_listeners` repeatedly resolves triggered routers until none remain, then fans listeners out in parallel (`asyncio.gather`) | `lib/crewai/src/crewai/flow/runtime/__init__.py:3072-3165` |
| Agent loop termination bound | `has_reached_max_iterations(iterations, max_iter)` checked every loop pass (both ReAct and native paths) | `lib/crewai/src/crewai/utilities/agent_utils.py:363-373`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:341-352, 501-513` |
| Forced final answer on cap | `handle_max_iterations_exceeded` appends `force_final_answer` instruction and makes one final LLM call | `lib/crewai/src/crewai/utilities/agent_utils.py:376-433`, `lib/crewai/src/crewai/translations/en.json:47` |
| Default iteration budget | `max_iter: int = Field(default=25)` | `lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:27` |
| Wall-clock termination | optional `max_execution_time` runs task in thread pool; `TimeoutError` raised on expiry | `lib/crewai/src/crewai/agent/core.py:211, 864-926`; validation `lib/crewai/src/crewai/agent/utils.py:304-315` |
| Guardrail retry bound | up to `guardrail_max_retries` (default 3) re-executions of the task, then raises | `lib/crewai/src/crewai/task.py:279-280, 1321-1439` |
| Crew completion | `_create_crew_output` requires ≥1 non-empty raw output; last valid output becomes crew result | `lib/crewai/src/crewai/crew.py:1919-1926` |
| Flow completion | after start methods and listener cascade drain, `FlowFinishedEvent` emitted and final output returned | `lib/crewai/src/crewai/flow/runtime/__init__.py:2446-2485` |
| Flow infinite-loop circuit breaker | `max_method_calls` (default 100, configurable via definition `max_method_calls=...`) → `RecursionError` naming self-matching `@listen` labels | `lib/crewai/src/crewai/flow/runtime/__init__.py:590, 3248-3256`, `lib/crewai/src/crewai/flow/flow_definition.py:219-223` |
| Experimental executor budget | `max_method_calls = max_iter * 10` ties flow-level guard to agent iterations | `lib/crewai/src/crewai/experimental/agent_executor.py:240` |
| Repeated tool call detection | identical consecutive `(tool_name, arguments)` rejected with "I tried reusing the same input..." | `lib/crewai/src/crewai/tools/tool_usage.py:254-261, 510-517, 779-789`, `lib/crewai/src/crewai/translations/en.json:49` |
| Per-tool usage limits | `max_usage_count` field + enforcement in both ToolUsage and native executor path | `lib/crewai/src/crewai/tools/base_tool.py:184, 267-271`, `lib/crewai/src/crewai/tools/structured_tool.py:212, 450-454`, `lib/crewai/src/crewai/tools/tool_usage.py:791-808`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:899-909, 967-969` |
| OR-listener fire-once semantics | `_fired_or_listeners` suppresses multi-trigger `or_()` listeners from double-firing; cleared on cyclic re-execution | `lib/crewai/src/crewai/flow/runtime/__init__.py:724, 1024-1070, 3204-3213, 3274-3281` |
| A2A remote handoff turn cap | `timeout` (default 120s) and `max_turns` config; escalating warnings; `_handle_max_turns_exceeded` returns last agent message or raises | `lib/crewai/src/crewai/a2a/config.py:396-397`, `lib/crewai/src/crewai/a2a/wrapper.py:713-728, 1272, 1407-1410, 761-819` |
| Human-in-the-loop as pause, not deadlock | `HumanFeedbackPending` control-flow exception emits `MethodExecutionPausedEvent`; `resume_async(feedback)` continues | `lib/crewai/src/crewai/flow/async_feedback/types.py:148`, `lib/crewai/src/crewai/flow/runtime/__init__.py:2948-2973, 1338` |
| Termination test (iterations) | `test_agent_max_iterations_stops_loop` asserts `iterations <= max_iter + 2` under adversarial tool | `lib/crewai/tests/agents/test_agent.py:448-482` |
| Termination test (forced answer) | `test_agent_moved_on_after_max_iterations` asserts forced final answer equals expected value | `lib/crewai/tests/agents/test_agent.py:485-509` |
| Circuit-breaker tests | `RecursionError` raised when method called more than configured `max_method_calls` | `lib/crewai/tests/test_flow_from_definition.py:3288-3290` |
| Handoff failure tests | delegating to nonexistent coworker returns enumerated error string | `lib/crewai/tests/tools/agent_tools/test_agent_tools.py:103-126` |
| Usage-limit tests | `test_tool_usage_limit.py` covers limit enforcement, reset, decorator validation | `lib/crewai/tests/tools/test_tool_usage_limit.py:8-151` |
| Hierarchical routing tests | manager delegation to assigned/all agents, varied role casing | `lib/crewai/tests/test_crew.py:358-546` |

## Answers to Dimension Questions

### 1. How are messages routed?

There is no central message bus for agent-to-agent messages; routing is structured around artifacts:

- **Sequential crews**: a deterministic list walk. `_execute_tasks` iterates `self.tasks` in order (`lib/crewai/src/crewai/crew.py:1582-1627`), each executed by its assigned agent, with prior outputs aggregated into context via `_get_context` (`lib/crewai/src/crewai/crew.py:1866-1874`). `async_execution=True` tasks are dispatched as futures and drained at the next synchronization point (`lib/crewai/src/crewai/crew.py:1597-1612`). `ConditionalTask` adds branch/skip behavior evaluated against the previous output (`lib/crewai/src/crewai/crews/utils.py:199-210`).
- **Hierarchical crews**: all tasks are executed nominally by the manager (`_get_agent_to_use` returns `self.manager_agent`, `lib/crewai/src/crewai/crew.py:1714-1717`); actual worker selection is delegated to the manager LLM's choice of coworker via delegation tools.
- **Flows**: event-driven graph routing. Method completions trigger listeners matched by name conditions (`_find_triggered_methods`, `lib/crewai/src/crewai/flow/runtime/__init__.py:3194-3215`); routers chain synchronously until no further router matches (`lib/crewai/src/crewai/flow/runtime/__init__.py:3082-3122`); parallel listeners run under `asyncio.gather` with racing-group support (`lib/crewai/src/crewai/flow/runtime/__init__.py:3140-3165`).

### 2. How is the next speaker selected?

Two mechanisms:

- **LLM-mediated selection (hierarchical)**: The manager agent receives tool descriptions enumerating coworker roles (`lib/crewai/src/crewai/translations/en.json:58-59`) and selects the next worker by emitting a `coworker` argument. Matching is defensive: whitespace normalization, quote stripping, and casefolding (`sanitize_agent_name`, `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:20-35`) — explicitly justified for weaker LLMs producing malformed JSON (`base_agent_tools.py:66-71`). No match returns an error message enumerating valid roles so the LLM can self-correct (`base_agent_tools.py:99-108`).
- **Static assignment (sequential)**: the next speaker is whatever agent each `Task.agent` names; there is no dynamic selection (`lib/crewai/src/crewai/crew.py:1717`).

There is no round-robin, voting, or capability-based selector anywhere in the codebase.

### 3. How are handoffs managed?

- **In-crew handoffs** are synchronous nested executions: the delegate tool constructs a fresh `Task(description=task, agent=selected_agent, expected_output=manager_request)` and calls `selected_agent.execute_task(...)` directly (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:112-120`). There is no queue, no async hop, and no state transfer beyond the `context` string passed in. Handoff counts are recorded per-task (`increment_delegations`, `lib/crewai/src/crewai/task.py:1142-1146`) purely for observability/metrics.
- **Cross-boundary handoffs (A2A)** wrap remote agents as local tools (`lib/crewai/src/crewai/a2a/wrapper.py:61-81` defines `DelegationContext`/`DelegationState`). Multi-turn remote conversations track completed task IDs in `reference_task_ids` to prevent replay within a conversation chain (`lib/crewai/src/crewai/a2a/wrapper.py:1082-1112`), support `input_required` pause states, and are capped by `max_turns`.
- **Manager restrictions encode handoff policy**: a user-supplied manager must not carry tools (`raise Exception("Manager agent should not have tools")`, `lib/crewai/src/crewai/crew.py:1522-1529`), forcing all worker contact through the delegation channel.

### 4. When does a group conversation terminate?

Termination is guaranteed by construction at each layer:

- **Crews**: after every task in the list has executed once (sync, async-drained, or skipped). There is no mechanism for agents to add tasks, so a crew cannot loop. Completion requires at least one non-empty raw output, otherwise `ValueError` (`lib/crewai/src/crewai/crew.py:1919-1925`).
- **Agents**: the ReAct/native loops exit on `AgentFinish` or when `iterations >= max_iter`, in which case one final forced-answer LLM call is made (`lib/crewai/src/crewai/utilities/agent_utils.py:376-433`); if that call returns empty, execution raises rather than hanging (`agent_utils.py:417-423`).
- **Flows**: when the listener cascade reaches quiescence (no triggered routers/listeners/conditional starts), `kickoff_async` emits `FlowFinishedEvent` and returns the last output (`lib/crewai/src/crewai/flow/runtime/__init__.py:2446-2485`).
- **Human intervention points are explicit pauses**, not required terminations: `human_input` on tasks (`lib/crewai/src/crewai/task.py:233`) and `HumanFeedbackPending` pause/resume in flows (`lib/crewai/src/crewai/flow/runtime/__init__.py:2948-2973`).

### 5. Is deadlock possible?

True deadlock (mutual blocking) is largely precluded by the synchronous, single-threaded-per-handoff design: a delegated worker runs inline inside the delegator's tool call (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:120`), so A→B→A recursion deepens the stack rather than deadlocking. However:

- **Unbounded delegation cycles are possible within budget**: A manager can ping-pong between workers indefinitely until each participant's `max_iter` (default 25) exhausts. The `delegations` counter (`lib/crewai/src/crewai/task.py:1146`) observes but never enforces a cap.
- **Infinite listener loops in flows are actively detected**: `_execute_single_listener` increments `_method_call_counts` and raises `RecursionError` past `max_method_calls` (default 100), with a diagnostic pointing at self-matching `@listen` labels (`lib/crewai/src/crewai/flow/runtime/__init__.py:3248-3256`). The error message itself documents the most common footgun.
- **Repeat-call oscillation is partially caught**: only exact consecutive repeats are blocked (`lib/crewai/src/crewai/tools/tool_usage.py:784-788`); alternating A/B/A/B patterns slip through until `max_iter`.
- **Timeout path leaks work**: `future.cancel()` on a running future does not stop the executing thread (`lib/crewai/src/crewai/agent/core.py:913-918`), so a timed-out task keeps consuming resources even though control returns.

## Architectural Decisions

1. **Tasks, not free-form chat, are the unit of group coordination.** The `Process` enum offers exactly sequential and hierarchical modes; "consensual" remains a TODO (`lib/crewai/src/crewai/process.py:9-11`). This makes group termination trivially decidable (fixed task count) at the cost of conversational flexibility.
2. **Speaker selection is pushed into the LLM via tool surface, not algorithmic policy.** Rather than a router module choosing next speakers, CrewAI gives the manager delegation tools and relies on prompt descriptions plus tolerant role matching (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:20-44`).
3. **Handoffs are synchronous function calls disguised as tools.** `BaseAgentTool._execute` inlines the callee's full agent loop and returns its final answer as an observation string (`base_agent_tools.py:112-124`), keeping the entire conversation tree inside one call stack.
4. **Layered defense-in-depth for termination**: per-loop iteration caps, forced final answers, optional time budgets, bounded guardrail retries, remote-conversation turn caps, and a flow circuit breaker — each independently enforced at a different layer.
5. **Explicit pause/resume instead of blocking waits for humans.** `HumanFeedbackPending` converts potential "deadlock-by-waiting" into persisted, resumable state (`lib/crewai/src/crewai/flow/runtime/__init__.py:2948-2973`).
6. **Cyclic flows are legal but metered.** Start methods may re-fire on router outcomes and completed-method bookkeeping is deliberately cleared for cycles (`lib/crewai/src/crewai/flow/runtime/__init__.py:2745-2748, 3172-3179`), with `max_method_calls` as the safety net.

## Notable Patterns

- **Tolerant-name matching with self-correction feedback**: sanitize + casefold the requested coworker, return the full roster on mismatch (`base_agent_tools.py:72-108`) so the LLM retries with a valid name — verified by `test_delegate_work_to_wrong_agent` (`lib/crewai/tests/tools/agent_tools/test_agent_tools.py:103-113`).
- **Forced-final-answer escape hatch**: appending an assistant message instructing the model to stop using tools and answer (`lib/crewai/src/crewai/translations/en.json:47`) converts a runaway loop into a degraded-but-present result; tested end-to-end (`lib/crewai/tests/agents/test_agent.py:485-509`).
- **Escalating warnings near a budget boundary**: A2A conversations inject "next turn will be the last" then "FINAL turn" notices into the prompt before hard-cutting (`lib/crewai/src/crewai/a2a/wrapper.py:713-728`).
- **Fire-once semantics for OR-listeners**: `_fired_or_listeners` prevents an `or_(a, b)` listener from firing twice when both events occur, then re-arms cleanly for cyclic flows (`lib/crewai/src/crewai/flow/runtime/__init__.py:1046-1070, 3204-3213`).
- **Budget coupling across layers**: the experimental executor derives its flow-circuit-breaker budget from the agent budget (`max_method_calls = max_iter * 10`, `lib/crewai/src/crewai/experimental/agent_executor.py:240`).

## Tradeoffs

- **Determinism vs. adaptivity**: fixed task ordering makes termination provable but removes dynamic replanning; hierarchical mode regains adaptivity only through LLM tool choices, which are probabilistic and harder to audit.
- **Synchronous handoffs simplify reasoning but serialize work**: a delegated sub-execution blocks the delegator's loop; there's no parallel committee deliberation within a crew step (parallelism exists only across independent async tasks, `lib/crewai/src/crewai/crew.py:1597-1606`).
- **LLM-selected speakers scale poorly with roster size**: coworker lists are interpolated verbatim into tool descriptions (`lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:24`), and duplicate role names silently resolve to the first match (`base_agent_tools.py:80-84`).
- **Observation-only cycle tracking**: counting delegations without enforcing a ceiling favors flexibility over safety; runaway hierarchies burn tokens until `max_iter`.

## Failure Modes / Edge Cases

- **Alternating tool loops evade repeat detection**: `_check_tool_repeated_usage` compares only with `tools_handler.last_used_tool` (`lib/crewai/src/crewai/tools/tool_usage.py:784-788`), so A,B,A,B sequences continue until `max_iter`.
- **Duplicate role names**: first match wins in delegation resolution; no uniqueness validation is performed at crew assembly.
- **Timeout does not reclaim the worker thread**: `future.cancel()` fails for running tasks; the orphaned thread continues executing LLM/tool calls (`lib/crewai/src/crewai/agent/core.py:904-918`).
- **Manager with tools aborts the crew**: supplying a manager carrying tools raises mid-setup (`lib/crewai/src/crewai/crew.py:1522-1529`), and a missing manager LLM fails fast (tested in `lib/crewai/tests/test_crew.py:378-392`).
- **Guardrail exhaustion raises rather than degrades**: after `guardrail_max_retries` failed validations, the task raises with the last guardrail error (`lib/crewai/src/crewai/task.py:1376-1385`), propagating up through `kickoff` and failing the whole crew.
- **Empty LLM response at the iteration cap raises**: `handle_max_iterations_exceeded` treats `None`/empty as fatal (`lib/crewai/src/crewai/utilities/agent_utils.py:417-423`).
- **Self-referential `@listen` label** causes rapid infinite recursion, caught only by the `max_method_calls` breaker (`lib/crewai/src/crewai/flow/runtime/__init__.py:3250-3255` documents this exact scenario).

## Future Considerations

- Enforce a delegation depth/cycle cap using the already-collected `task.delegations` / `processed_by_agents` data (`lib/crewai/src/crewai/task.py:1142-1146`) — e.g., reject a handoff when the same (delegator, delegate) pair recurs N times.
- Extend repeated-usage detection from "last call" to a sliding window or fingerprint set to catch alternating loops cheaply (`lib/crewai/src/crewai/tools/tool_usage.py:779-789`).
- Replace thread-pool timeouts with cooperative cancellation so timed-out tasks stop consuming provider quota (`lib/crewai/src/crewai/agent/core.py:888-926`).
- Surface `max_method_calls`, `max_iter`, and A2A `max_turns` in a single observability view; today they live in three unrelated subsystems (flows, executors, A2A config).
- Add uniqueness validation for agent roles at crew construction to make delegation targeting unambiguous.

## Questions / Gaps

- **No cross-agent conversation-level watchdog found.** Searched `handoff`, `deadlock`, `cycle`, `visited`, `max_loops` across `lib/crewai/src/crewai/**` — the only cycle protections are the flow `max_method_calls` breaker and OR-listener suppression. No evidence of a hierarchical-mode delegation-cycle detector; if one exists, it lives outside this repository (e.g., enterprise layer) — "No evidence found" within the selected source.
- **No evidence of speaker-selection policies** such as round-robin, capability scoring, or load balancing; selection is entirely static assignment or LLM tool choice. Search boundary: `crew.py`, `process.py`, `crews/utils.py`, `tools/agent_tools/*`, `flow/*`.
- **Consensual process is unimplemented** (TODO comment, `lib/crewai/src/crewai/process.py:11`); any claims about peer-voting routing would be unsupported.
- **Delegation context transfer is lossy by design**: the handoff carries only the `context` string the LLM chose to include; no structured memory/state snapshot accompanies a handoff (evidence: `base_agent_tools.py:112-120` signature). Whether richer state propagation was ever intended is undocumented in-code.

---

Generated by `15.02-message-routing-and-termination` against `crewai`.
