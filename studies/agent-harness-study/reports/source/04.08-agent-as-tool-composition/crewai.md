# Source Analysis: crewai

## Agent-as-Tool and Workflow-as-Tool Composition

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (>=3.10), Pydantic v2, LangChain interop, A2A/MCP SDKs |
| Analyzed | 2026-08-23 |

## Summary

CrewAI implements **agent-as-tool** primarily through its *delegation tool* family rather than through a generic `as_tool()` wrapper API. A manager agent (or any agent with `allow_delegation=True`) receives `DelegateWorkTool` and `AskQuestionTool` instances built by `AgentTools.tools()` (`lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:22-36`). Each of these tools holds direct references to peer `BaseAgent` objects (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:18`) and, when invoked, synthesizes an ad-hoc `Task` (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:112-116`) and calls `selected_agent.execute_task(...)` synchronously in-process (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:120`) — a full nested agent run inside the parent's tool call.

**Workflow-as-tool** exists only in one direction. Flows orchestrate crews as steps by calling `Crew.kickoff()` inside `@listen` methods (documented multi-crew pattern, `docs/edge/en/concepts/flows.mdx:846-878`), and the flow runtime aggregates usage across every orchestrated crew (`docs/edge/en/concepts/flows.mdx:231`). There is **no first-class `Flow.as_tool()` / `Crew.as_tool()` wrapper** that exposes a workflow back onto an agent's tool belt; searching the source tree for `as_tool`/`to_tool` wrappers returns only unrelated `has_tool_failures` properties. The closest analogues are remote: the A2A module wraps agents/classes with network-level delegation capabilities (`lib/crewai/src/crewai/a2a/wrapper.py:26-60`, `execute_a2a_delegation` at `lib/crewai/src/crewai/a2a/utils/delegation.py:135`), and MCP tools are wrapped locally as `BaseTool`s (`lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:17-40`).

Nested execution **is traced**: every emitted event is stamped with `parent_event_id`, `previous_event_id`, `triggered_by_event_id`, and an `emission_sequence` resolved from a contextvar scope stack (`lib/crewai/src/crewai/events/event_bus.py:543-568`; stack helpers in `lib/crewai/src/crewai/events/event_context.py:57-59`), with a hard stack depth cap of 100 (`lib/crewai/src/crewai/events/event_context.py:24`). Nested kickoffs deliberately avoid resetting emission counters when already inside a parent context (`lib/crewai/src/crewai/crews/utils.py:271-273`). Child runs are bounded by each agent's own loop limits (`max_iter` default 25, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306`, enforced in the executor at `lib/crewai/src/crewai/agents/crew_agent_executor.py:343-344, 503-504, 1157-1158`), and the flow runtime has its own infinite-loop guard (`max_method_calls=100` raising `RecursionError`, `lib/crewai/src/crewai/flow/runtime/__init__.py:590, 3247-3256`).

The weak points are the **result contract and failure semantics**: delegation returns plain `str` values, exceptions inside the delegated run are caught and converted into human-readable error strings instead of propagating (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:99-108, 121-124`), there is **no per-delegation cost attribution** (usage is summed per-agent/per-crew after the fact), and **no explicit recursion-depth guard on agent-to-agent delegation** — a delegating child can itself be granted delegation tools again, with only `max_iter` acting as an implicit bound.

## Rating

**6/10**

Rationale: The core mechanism is a clear, tested model with typed input schemas and real operational safeguards — Pydantic `args_schema` contracts (`lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:8-13`), role-name sanitization for fragile LLM outputs (`base_agent_tools.py:20-35, 66-75`), VCR-recorded tests plus negative cases (`lib/crewai/tests/tools/agent_tools/test_agent_tools.py:20-31, 103-126`), bounded child loops (`max_iter` enforcement), event-parent-chain tracing with a depth cap, and a flow-level loop guard that raises `RecursionError`. This earns the "clear model with tests, explicit interfaces" credit of the 7–8 band. It falls short of 7+ because: (1) nested results are unstructured strings and child failures are swallowed into error text rather than propagated or surfaced as typed failures; (2) child-run costs are attributed only in aggregate, never per delegation; (3) there is no dedicated recursion guard for chained delegation (unlike the flow runtime's `max_method_calls` guard), so agent→agent→agent cycles rely entirely on per-agent iteration limits; and (4) workflow-as-tool is asymmetric — flows can drive crews, but neither crews nor flows can be exposed back as callable tools, pushing users to ad-hoc `BaseTool` subclasses or the A2A protocol for that shape of composition.

## Evidence Collected

Every entry cites `path:line` relative to `studies/agent-harness-study/sources/crewai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent tool wrapper factory | `AgentTools` builds `[DelegateWorkTool, AskQuestionTool]` with coworker roster interpolated into descriptions | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:16-36` |
| Agents injected as tool fields | `BaseAgentTool.agents: list[BaseAgent]` pydantic field | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:15-18` |
| Nested run entry point | `_execute` matches sanitized role name, creates ad-hoc `Task`, calls `selected_agent.execute_task(task_with_assigned_agent, context)` | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:46-124` (esp. 112-120) |
| Input contract (delegate) | `DelegateWorkToolSchema{task, context, coworker}` required fields | `lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:8-13` |
| Input contract (ask) | `AskQuestionToolSchema{question, context, coworker}` | `lib/crewai/src/crewai/tools/agent_tools/ask_question_tool.py:8-11` |
| Malformed-arg tolerance | `_get_coworker` strips `[...]` array-style coworker values from weak LLM JSON | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:37-44` |
| Manager wiring (hierarchical) | `_create_manager_agent` assigns `tools=AgentTools(agents=self.agents).tools()`, forbids manager having other tools | `lib/crewai/src/crewai/crew.py:1513-1545` (esp. 1533-1540) |
| Delegation opt-in gate | `_prepare_tools` injects delegation tools only when `agent.allow_delegation` is truthy; hierarchical requires manager | `lib/crewai/src/crewai/crew.py:1644-1667` |
| Peer-set scoping | `_add_delegation_tools` passes only agents other than the task agent | `lib/crewai/src/crewai/crew.py:1820-1832` |
| Manager tool refresh per task | `_update_manager_tools` re-injects delegation tools scoped to current task agent | `lib/crewai/src/crewai/crew.py:1854-1864` |
| Training disables delegation | `train()` force-sets `agent.allow_delegation = False` for all agents | `lib/crewai/src/crewai/crew.py:928-935` |
| Config validation | hierarchical process requires `manager_llm` or `manager_agent`; manager must not be listed in `agents` | `lib/crewai/src/crewai/crew.py:723-738` |
| Child loop bound | `max_iter` default 25 ("Maximum iterations for an agent to execute a task") | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306` |
| Bound enforcement | executor checks `has_reached_max_iterations(self.iterations, self.max_iter)` at three dispatch sites | `lib/crewai/src/crewai/agents/crew_agent_executor.py:343-344, 503-504, 1157-1158` |
| Child time bound | `execute_task` documents/raises `TimeoutError` on max execution time | `lib/crewai/src/crewai/agent/core.py:816-839` |
| Per-tool usage caps | `max_usage_count` checked before tool invocation; parallel native execution skipped when flags present | `lib/crewai/src/crewai/agents/crew_agent_executor.py:708-723, 899-909` |
| Event parent chain | emit stamps `previous_event_id`, `triggered_by_event_id`, `emission_sequence`, `parent_event_id` from contextvar scope stack | `lib/crewai/src/crewai/events/event_bus.py:543-568` |
| Trace depth cap | `EventContextConfig.max_stack_depth: int = 100` with `StackDepthExceededError` | `lib/crewai/src/crewai/events/event_context.py:22-32` |
| Parent id lookup | `get_current_parent_id()` reads top of event-id stack | `lib/crewai/src/crewai/events/event_context.py:57-59` |
| Nested kickoff tracing preserved | `prepare_kickoff` resets counters/last-event-id only when `get_current_parent_id() is None` | `lib/crewai/src/crewai/crews/utils.py:269-273` |
| Child lifecycle events | `AgentExecutionCompletedEvent` emitted with output around nested `execute_task` completion | `lib/crewai/src/crewai/agent/core.py:702-710` |
| Cost aggregation (crew) | `calculate_usage_metrics` sums per-agent LLM token summaries incl. manager agent's `_token_process` | `lib/crewai/src/crewai/crew.py:2201-2228` |
| Cost aggregation (fan-out) | `run_for_each_async` sums `usage_metrics` over all crew copies back onto parent crew | `lib/crewai/src/crewai/crews/utils.py:507-511` |
| Structured output object | `CrewOutput` carries `raw`, `pydantic`, `json_dict`, `tasks_output`, `token_usage`, `tool_failures` | `lib/crewai/src/crewai/crews/crew_output.py:14-53` |
| Delegation result is plain str | `DelegateWorkTool._run -> str` / `AskQuestionTool._run -> str`; errors returned as i18n-formatted strings | `lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:22-30`; `base_agent_tools.py:88-124` |
| Error swallowing | `except Exception as e:` inside `_execute` returns formatted error string, never re-raises | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:121-124` |
| Unknown-coworker handling | returns i18n "coworker mentioned not found" listing valid roles | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:99-108`; test `lib/crewai/tests/tools/agent_tools/test_agent_tools.py:103-113` |
| Delegation tests | VCR tests `test_delegate_work`, `test_ask_question`, array-coworker variants, wrong-agent negatives | `lib/crewai/tests/tools/agent_tools/test_agent_tools.py:19-126`; cassettes under `lib/crewai/tests/cassettes/tools/agent_tools/` |
| Flow loop guard | `max_method_calls: int = Field(default=100)`; exceeding raises `RecursionError` naming self-referential `@listen` hazard | `lib/crewai/src/crewai/flow/runtime/__init__.py:590, 3243-3256` |
| Flows orchestrate crews | docs: multi-crew flow scaffolding (`crews/` folder per project) | `docs/edge/en/concepts/flows.mdx:846-878` |
| Flow-level cost rollup | flow `usage_metrics` aggregates every LLM call incl. orchestrated crews | `docs/edge/en/concepts/flows.mdx:231` |
| Reverse direction (tools in flows) | flow actions may supply `Callable[[], BaseTool]` factories | `lib/crewai/src/crewai/flow/runtime/_actions.py:110-117` |
| Remote agent-as-tool (A2A) | metaclass wraps agent classes with A2A delegation; `execute_a2a_delegation` sync wrapper; A2A events emitted | `lib/crewai/src/crewai/a2a/wrapper.py:26-60`; `lib/crewai/src/crewai/a2a/utils/delegation.py:135-200`; `lib/crewai/src/crewai/events/types/a2a_events.py` |
| External tools wrapped as BaseTool | `MCPToolWrapper(BaseTool)` connects on-demand with timeouts/retries constants | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:7-40` |
| Skill instructions as tool | `LoadSkillTool(BaseTool)` progressively discloses skill content, emits `SkillUsedEvent` | `lib/crewai/src/crewai/skills/tool.py:34-69` |
| Adapter parity | LangGraph/OpenAI adapter agents expose their own `get_delegation_tools` implementations | `lib/crewai/src/crewai/agents/agent_adapters/langgraph/langgraph_adapter.py:280-291`; `lib/crewai/src/crewai/agents/agent_adapters/openai_agents/openai_adapter.py:240` |

## Answers to Dimension Questions

**1. Can one agent call another?**
Yes. Two mechanisms: (a) in-process delegation tools — `DelegateWorkTool`/`AskQuestionTool` execute a peer agent's `execute_task` synchronously inside the caller's tool step (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:112-120`); provisioned for the hierarchical manager by default (`lib/crewai/src/crewai/crew.py:1533-1540`) and for any agent opting in via `allow_delegation=True` in sequential mode (`lib/crewai/src/crewai/crew.py:1644-1667`). (b) Remote delegation via the A2A protocol client wrappers (`lib/crewai/src/crewai/a2a/utils/delegation.py:135-200`). Additionally, third-party agent-framework adapters (LangGraph, OpenAI Agents) reimplement `get_delegation_tools` so their native handoff semantics surface as CrewAI-compatible tools.

**2. Are child runs bounded?**
Partially, by inheritance rather than by delegation policy. A delegated child executes the same `Task` pipeline as any top-level task, so it is subject to the assigned agent's `max_iter` (default 25, `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306`) enforced in the executor loop (`lib/crewai/src/crewai/agents/crew_agent_executor.py:343-344`), plus optional `max_rpm` (`base_agent.py:293`) and execution-time `TimeoutError` (`agent/core.py:816-839`). However, the delegation layer itself sets **no** timeout, budget, or depth parameter: `BaseAgentTool._execute` accepts only `(agent_name, task, context)` (`base_agent_tools.py:46-48`) and cannot pass or enforce child-specific limits. There is no equivalent of the flow runtime's call-count cap on the delegation path.

**3. Are child run costs attributed?**
Only in aggregate. `Crew.calculate_usage_metrics` sums token usage across all member agents **and** the manager agent (`lib/crewai/src/crewai/crew.py:2201-2228`), and fan-out execution merges metrics from crew copies onto the parent (`lib/crewai/src/crewai/crews/utils.py:507-511`). `CrewOutput.token_usage` / `usage_metrics` expose the totals (`lib/crewai/src/crewai/crews/crew_output.py:26-29, 47-53`). But nothing attributes tokens to an individual `DelegateWorkTool` invocation or links child spend to the delegating parent call — the delegation result is a bare string with no cost metadata, and the event stream carries no usage payload per delegation. No evidence of per-nesting-level cost accounting was found anywhere in `lib/crewai/src/crewai`.

**4. Can nested tools recurse forever?**
Not trivially, but not by an explicit delegation guard either. Three independent bounds exist: (a) each nested agent run terminates at `max_iter`; (b) the event-tracing scope stack hard-fails past depth 100 (`StackDepthExceededError`, `lib/crewai/src/crewai/events/event_context.py:22-32`); (c) the flow runtime counts listener invocations and raises `RecursionError` beyond `max_method_calls=100` (`lib/crewai/src/crewai/flow/runtime/__init__.py:590, 3247-3256`). The gap: because `_prepare_tools` runs for every executed task (`lib/crewai/src/crewai/crews/utils.py:162-167`), a delegated child whose agent also has `allow_delegation=True` receives delegation tools again, enabling arbitrarily deep agent→agent chains bounded only by cumulative `max_iter` budgets. Nothing detects delegation cycles (e.g., A delegates to B who delegates back to A) as a distinct failure class.

**5. Does the parent receive structured results?**
No for in-process delegation — the parent LLM receives a plain `str` (`delegate_work_tool.py:22-30`), with failures rendered as prose error strings (`base_agent_tools.py:99-108, 121-124`). Yes elsewhere in the composition surface: `CrewOutput` is a fully typed Pydantic model exposing raw/pydantic/JSON payloads, per-task `TaskOutput`s, token usage, and recorded tool failures (`lib/crewai/src/crewai/crews/crew_output.py:14-53`), which is what a Flow step receives when orchestrating a crew; and A2A responses conform to `AgentResponseProtocol`/`LiteAgentOutput` types (`lib/crewai/src/crewai/a2a/wrapper.py:57`, `lib/crewai/src/crewai/lite_agent_output.py`). The asymmetry means programmatic callers get structure while LLM-mediated delegation gets text only.

## Architectural Decisions

1. **Delegation as tools, not as control flow.** Rather than building orchestration into the executor, CrewAI models hierarchy as two ordinary `BaseTool`s holding live agent references (`agent_tools.py:16-36`, `base_agent_tools.py:18`). The LLM chooses targets by role name; the framework supplies the roster in tool descriptions (`agent_tools.py:24-34`). Consequence: delegation semantics are uniform with any other tool (caching, hooks, events, usage limits all apply), at the cost of name-matching fragility that requires defensive sanitization (`base_agent_tools.py:20-35, 66-75`).

2. **Manager exclusivity in hierarchical mode.** The manager agent is constructed without user tools — supplying tools raises an exception (`crew.py:1519-1526`) — and its toolset is refreshed per task to target exactly the current task's agent (`_update_manager_tools`, `crew.py:1854-1864`). This encodes "manager coordinates, workers execute" structurally rather than by prompt discipline.

3. **Opt-in delegation via `allow_delegation` (default False).** Tool injection is gated per agent (`crew.py:1648-1660`, field default at `base_agent.py:297-300`), and training mode strips delegation globally (`crew.py:928-935`). This limits accidental fan-out but also means peer-to-peer graphs are manually configured.

4. **Observability via ambient context, not run objects.** Instead of spawning child-run handles, nested execution inherits the event bus's contextvar scope stack: parent linkage is stamped onto every event automatically (`event_bus.py:543-568`), and nested kickoffs preserve the outer chain by skipping counter resets when a parent id exists (`crews/utils.py:269-273`). Tracing is therefore zero-API-cost to composers, but there is no first-class `ChildRun` object to attach budgets/cancellation to.

5. **Remote composition pushed to protocol adapters.** Cross-runtime composition is delegated to the A2A module (metaclass wrapping, card fetching, delegation helpers, dedicated event types — `a2a/wrapper.py:26-60`) rather than a local sub-agent abstraction, keeping OSS core simple while enabling cross-org agent reuse.

## Notable Patterns

- **Role-name normalization as an LLM-defensive contract**: whitespace collapsing, quote stripping, casefolding both on lookup and on error listings (`base_agent_tools.py:20-35, 80-108`) — a recurring pattern of compensating for weak function-calling models.
- **Ad-hoc Task synthesis**: delegation constructs a throwaway `Task` with an i18n `expected_output` slice (`manager_request`) instead of a bespoke child-run API (`base_agent_tools.py:112-116`) — maximal reuse of the existing execution pipeline, minimal new surface.
- **Per-task manager tool refresh**: `_update_manager_tools` re-scopes the manager's delegate targets to the current task's worker on every task (`crew.py:1854-1864`), narrowing prompt-visible choices.
- **Guarded recursion in flows**: the `@listen` dispatcher tracks per-method invocation counts and fails loudly with an explanatory message naming the classic self-listening footgun (`flow/runtime/__init__.py:3250-3256`) — a model example of an actionable recursion guard message.
- **Progressive disclosure as subroutine-as-tool**: `LoadSkillTool` exposes skill instructions on demand with collision-proof naming against user tools (`skills/tool.py:72-86, 89-116`) — instructions treated as a lazily-loaded capability.

## Tradeoffs

- **String-typed delegation results**: simple for LLM consumption, but loses typing, cost data, and machine-checkable status; contrast with `CrewOutput`'s rich schema available to Flow-based composition.
- **Error swallowing for resilience vs. diagnosability**: converting child exceptions to strings keeps the parent conversation alive (the LLM can retry or pick another coworker), but Python-side callers cannot branch on failure except by parsing text (`base_agent_tools.py:121-124`).
- **Ambient tracing vs. runnable handles**: automatic parent-chain events require zero composer effort, yet the absence of a child-run handle forecloses per-child budgets, cancellation tokens, and per-delegation cost attribution.
- **Reuse-everything delegation vs. blast radius**: because children run the full pipeline (knowledge, skills, memory, tools), delegated work is feature-complete but expensive and side-effecting; no "read-only child" profile exists.
- **Protocol-first remote composition vs. local ergonomics**: A2A gives durable cross-network delegation but demands infrastructure; local crews/flows remain single-process with no serialization boundary.

## Failure Modes / Edge Cases

- **Delegation cycle without dedicated detection**: A↔B mutual delegation burns `max_iter` budgets on both sides before terminating; no cycle alarm distinct from generic iteration exhaustion (analysis of `base_agent_tools.py` + `crew.py:1820-1832`; no cycle-detection symbols found in `lib/crewai/src/crewai`).
- **Duplicate role names**: matching takes `agent[0]` after filtering (`base_agent_tools.py:80-110`), so two agents sharing a role silently resolve to one; sanitization makes near-identical roles ("Researcher " vs "researcher") collide intentionally.
- **Exception masking**: a child crash surfaces as `"Error executing tool..."` prose (`base_agent_tools.py:121-124`); downstream guardrails or evaluators inspecting structured failures will miss it unless they parse `raw`.
- **Context loss on ask-vs-delegate confusion**: both tools share identical `_execute` plumbing; whether the child treats the request as work or a question depends purely on prompt framing, not on mechanism.
- **Flow self-listening**: a `@listen` referencing its own method previously looped forever; now capped at 100 calls with `RecursionError` (`flow/runtime/__init__.py:3247-3256`) — the guard converts a hang into a loud failure mid-flight (partial state persists).
- **Parallel-native execution constraints**: batch runs containing tools flagged `result_as_answer` or `max_usage_count` fall back from parallel native execution (`crew_agent_executor.py:708-723`), so composition-heavy batches may silently serialize.

## Future Considerations

- Introduce a structured `DelegationResult` (status, cost, duration, child event id) either as an optional return mode or via a typed side-channel, closing the gap between delegation and `CrewOutput` consumers.
- Add delegation-scoped knobs (timeout, `max_iter` override, token budget) plumbed through `Task` synthesis in `BaseAgentTool._execute`.
- Add a delegation-cycle detector using the existing fingerprint/family machinery or the event parent chain (child event ancestry already identifies ancestors).
- Provide first-class `Crew.as_tool()` / `Flow.as_tool()` wrappers with generated `args_schema` from crew inputs — currently users hand-roll such `BaseTool` subclasses.
- Attribute costs per delegation by tagging token summaries with the active event parent id already available in contextvars.

## Questions / Gaps

- **No per-delegation telemetry found**: searched `lib/crewai/src/crewai` for usage payloads attached to tool-usage or delegation events; `ToolUsage*` events carry names/args/limits but not token counts. If per-delegation cost exists, it lives outside this tree (e.g., Enterprise UI), not in evidence here.
- **No cancellation propagation to children**: no cancellation-token or cooperative-cancel parameter appears anywhere in the delegation path (`base_agent_tools.py`, `agent/core.py:816-839` documents only `TimeoutError`). Whether platform/Enterprise layers add one could not be verified from this source.
- **Historical `allow_delegation` semantics unclear**: the flag gates tool injection (`crew.py:1648-1660`) but does not restrict a delegated child from being granted delegation tools if it independently opted in — confirming whether this is intended composition or an oversight would require maintainer input; no test covers nested delegation depth.
- **Docs vs. code drift risk**: `docs/edge/en/concepts/processes.mdx:18-47` describes hierarchical delegation at prompt level; implementation details (per-task manager tool refresh, training-mode stripping) are code-only, so documentation-only claims were not relied upon.

---

Generated by `dimensions/04.08-agent-as-tool-and-workflow-as-tool-composition` against `crewai`.
