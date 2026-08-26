# Source Analysis: crewai

## Dimension 15.01: Coordination Topology

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based framework; monorepo with core package under `lib/crewai/src/crewai`) |
| Analyzed | 2026-08-26 |

> All file paths below are relative to the source root `studies/agent-harness-study/sources/crewai`.

## Summary

CrewAI implements **four distinct coordination topologies**, selectable per composition:

1. **Sequential pipeline (default)** — a fixed task list executed in order by a single-threaded orchestrator loop; inter-agent communication is *indirect data passing* (a task's output becomes the next task's context) rather than messaging (`lib/crewai/src/crewai/process.py:9`, `lib/crewai/src/crewai/crew.py:1509-1511`, `lib/crewai/src/crewai/utilities/formatter.py:16-26`). Optional intra-step parallelism via `async_execution=True` tasks fanned out on futures and joined before the next synchronous task (`lib/crewai/src/crewai/crew.py:1597-1625`).
2. **Hierarchical supervisor-worker** — an explicit manager agent (auto-created from i18n prompts or user-supplied) executes every task and coordinates workers through two LLM tools, `Delegate work to coworker` and `Ask question to coworker`; delegation resolves the target by sanitized role-name string match and runs a synthetic Task on that agent synchronously in-process (`lib/crewai/src/crewai/process.py:10`, `lib/crewai/src/crewai/crew.py:1513-1548`, `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:46-124`).
3. **Event-driven graph (Flows)** — methods decorated `@start`/`@listen`/`@router` form a directed graph with `or_`/`and_` join conditions; routers run sequentially for flow control and listeners run in parallel via `asyncio.gather`; cyclic flows are supported with once-only OR-listener semantics (`lib/crewai/src/crewai/flow/runtime/__init__.py:3048-3165`, `lib/crewai/src/crewai/flow/dsl/_conditions.py:22-27`).
4. **Cross-process federation (A2A protocol)** — agents configured with `a2a=[...]` have their `execute_task`/`kickoff` wrapped at class construction; remote peers are discovered by fetching A2A AgentCards concurrently over HTTP and injected into the prompt as `<AVAILABLE_A2A_AGENTS>`; delegation is a multi-turn network conversation with polling/streaming/push updates (`lib/crewai/src/crewai/agent/internal/meta.py:59-69`, `lib/crewai/src/crewai/a2a/wrapper.py:97-237`, `lib/crewai/src/crewai/a2a/utils/delegation.py:135-199`).

The topology is **statically declared but dynamically exercised**: process type and membership are fixed at construction time, yet which worker receives delegated work (hierarchical), whether a peer delegates at all (sequential + `allow_delegation`), and which branch a router selects (Flows) are runtime LLM/code decisions. Coordination inside a single Crew is centralized — either the sequential orchestrator loop or the manager agent is the single coordinator; only Flows distribute control (parallel listeners) and only A2A crosses process boundaries. There is no marketplace or registry-style discovery; discovery is role-string lists embedded in tool descriptions or statically configured remote endpoints.

## Rating

**7 / 10** — Clear, explicit, well-tested coordination model with three documented topologies plus protocol-level federation, validated configuration (`check_manager_llm`), and observable execution (event bus, execution UUIDs). It falls short of 8-9 because delegation addressing relies on fragile case-folded role-string matching with silent first-match-wins collision behavior, delegation failures degrade to string observations instead of structured errors, and the hierarchical manager is an unmitigated single point of failure (no failover, no quorum).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Topology definitions | `Process` enum exposes only `sequential` and `hierarchical` (a `consensual` variant remains a TODO comment); default set on `Crew.process` field | `lib/crewai/src/crewai/process.py:4-11`, `lib/crewai/src/crewai/crew.py:251` |
| Process dispatch | `kickoff` branches on process; unknown processes raise `NotImplementedError` | `lib/crewai/src/crewai/crew.py:1051-1058` |
| Sequential pipeline | `_run_sequential_process` iterates `self.tasks` through `_execute_tasks` in list order | `lib/crewai/src/crewai/crew.py:1509-1511`, `lib/crewai/src/crewai/crew.py:1558-1627` |
| Data-passing channel | Task outputs aggregated with dividers into a context string injected into the executing agent's prompt (`Task.prompt_context`) | `lib/crewai/src/crewai/utilities/formatter.py:13-45`, `lib/crewai/src/crewai/task.py:151,167-168,686` |
| Intra-step parallelism | `task.async_execution` futures collected then drained before next sync task (`_process_async_tasks`) | `lib/crewai/src/crewai/crew.py:1579,1597-1612,1624-1625` |
| Manager creation | Hierarchical kickoff calls `_create_manager_agent`: builds `Agent(role="Crew Manager", ...)` with `AgentTools(agents=...)` tools, forces `allow_delegation=True`, strips manager tools | `lib/crewai/src/crewai/crew.py:1513-1548` |
| Manager prompt identity | i18n slice `hierarchical_manager_agent` defines role/goal/backstory ("You are a seasoned manager... delegate work to the right people") | `lib/crewai/src/crewai/translations/en.json:2-7` |
| Central routing | `_get_agent_to_use` returns `manager_agent` for *every* task in hierarchical mode; otherwise `task.agent` | `lib/crewai/src/crewai/crew.py:1714-1717` |
| Delegation tools | `AgentTools.tools()` builds `DelegateWorkTool` + `AskQuestionTool` with coworker role list baked into descriptions | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:22-36` |
| Delegation mechanics | `BaseAgentTool._execute` sanitizes/casefolds role names, finds first match, wraps request in a synthetic `Task`, calls `agent.execute_task(...)` synchronously | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:37-120` |
| Peer-to-peer opt-in | `_add_delegation_tools` injects tools targeting all agents except self when `allow_delegation=True` and crew has >1 agent | `lib/crewai/src/crewai/crew.py:1648-1660,1820-1830` |
| Delegation default off | `allow_delegation: bool = Field(default=False)` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297-300` |
| Dynamic narrowing | `_update_manager_tools` restricts the manager's coworker list to `[task.agent]` when the task pre-assigns an agent | `lib/crewai/src/crewai/crew.py:1853-1863` |
| Config validation | `check_manager_llm` validator requires `manager_llm`/`manager_agent` for hierarchical and forbids manager appearing in `agents` | `lib/crewai/src/crewai/crew.py:721-743` |
| Flow graph DSL | `or_`/`and_` condition combinators over `@start`/`@listen`/`@router` triggers | `lib/crewai/src/crewai/flow/dsl/_conditions.py:22-27`, `lib/crewai/src/crewai/flow/flow.py:21,33-45` |
| Flow dispatch semantics | `_execute_listeners`: routers chained sequentially in a while-loop, plain listeners fired concurrently via `asyncio.gather` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3054-3165` |
| Cyclic-flow safeguards | OR-listener fired-set guarded by `threading.Lock`, cleared/re-armed per trigger so cyclic flows don't double-fire | `lib/crewai/src/crewai/flow/runtime/__init__.py:1023-1078` |
| A2A wiring | `AgentMeta` metaclass wraps `post_init_setup`; agents with `a2a` field get `wrap_agent_with_a2a_instance` applied | `lib/crewai/src/crewai/agent/internal/meta.py:48-79` |
| A2A method wrapping | Wrappers around `execute_task`, `aexecute_task`, `kickoff`, `kickoff_async`; also `inject_a2a_server_methods` so an agent can serve requests | `lib/crewai/src/crewai/a2a/wrapper.py:115-237` |
| Remote discovery | AgentCards fetched concurrently (`max_workers = min(len(a2a_agents), 10)`); failures tracked per endpoint | `lib/crewai/src/crewai/a2a/wrapper.py:240-279` |
| Discovery injection | `<AVAILABLE_A2A_AGENTS>` / unavailable-agents notice templates inserted into the task prompt; turn budget surfaced as `<CONVERSATION_PROGRESS>` | `lib/crewai/src/crewai/a2a/templates.py:7-29` |
| Network delegation | `execute_a2a_delegation` sends messages over A2A protocol with context/task IDs, conversation history, streaming/polling/push update configs | `lib/crewai/src/crewai/a2a/utils/delegation.py:135-199` |
| Observability channel | Single event bus `emit(source, event)` fans events to listeners; nested kickoffs inherit `crewai_execution_uuid` via contextvars | `lib/crewai/src/crewai/events/event_bus.py:572`, `lib/crewai/src/crewai/execution.py:22-61` |
| Static graph rendering | Flow visualization builder derives nodes/edges from conditions; notes some event edges are not "statically inferable" | `lib/crewai/src/crewai/flow/visualization/builder.py:111-146,238` |
| Tests: hierarchy | `test_hierarchical_process`, `test_manager_llm_requirement_for_hierarchical_process`, manager tool-injection assertions ("Delegate a specific task to one of the following coworkers: Researcher"), case-insensitive role matching test | `lib/crewai/tests/test_crew.py:358-376,378-389,393-471,474-524` |
| Tests: flows | `test_flow_with_and_condition`, `test_flow_with_or_condition`, `test_or_listener_fires_once_across_parallel_starts`, `test_cyclic_flow`, `test_flow_with_router` | `lib/crewai/tests/test_flow.py:73,110,137,185,266` |
| Tests: A2A | Card fetch, polling/streaming completion, push-handler timeout/failure paths | `lib/crewai/tests/a2a/test_a2a_integration.py:48,64,93,176,241` |
| Docs (stated design) | Processes doc matches implementation: sequential order + output-as-context; hierarchical manager allocates/reviews tasks; manager_llm required | `docs/edge/en/concepts/processes.mdx:17-18,51-57` |
| Docs (collaboration) | Delegation tools presented as automatic consequence of `allow_delegation=True` | `docs/edge/en/concepts/collaboration.mdx:9-60` |

## Answers to Dimension Questions

**1. How do agents coordinate?**
Three mechanisms, layered. (a) *Task-output relay*: in both processes, coordination data flows as aggregated raw text of prior task outputs passed as `context` into each task's prompt (`lib/crewai/src/crewai/crew.py:1866-1874`, `lib/crewai/src/crewai/utilities/formatter.py:16-26`, `lib/crewai/src/crewai/task.py:686`). There is no message bus between agents — communication is mediated entirely by the Crew. (b) *Tool-mediated delegation*: workers/manager call `DelegateWorkTool`/`AskQuestionTool`, which synchronously execute a synthetic task on the target agent in the same thread and return its string result as the tool observation (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:110-124`, `lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:22-30`). (c) *Protocol messaging*: A2A-configured agents exchange structured A2A `Message` objects with remote agents over HTTP with multi-turn history and update handlers (`lib/crewai/src/crewai/a2a/utils/delegation.py:135-199`). Flows coordinate *methods* (which may wrap crews) via an event-trigger graph rather than coordinating agents directly (`lib/crewai/src/crewai/flow/runtime/__init__.py:3054-3071`).

**2. Is the topology fixed or dynamic?**
Declared static, exercised dynamically. The process type is a construction-time field (`lib/crewai/src/crewai/crew.py:251`), membership lists (`agents`, `tasks`) are frozen at kickoff, and no API mutates edges mid-run. Dynamism lives in LLM decisions: the manager chooses *whether/whom* to delegate to per step, and its coworker set can be narrowed per-task (`_update_manager_tools`, `lib/crewai/src/crewai/crew.py:1853-1863`). Flows are structurally static graphs (edges derived from decorators at class creation, `lib/crewai/src/crewai/flow/visualization/builder.py:175-238`) but support dynamic branching through router return values and cyclic re-entry with once-only OR-listener re-arming (`lib/crewai/src/crewai/flow/runtime/__init__.py:1043-1078`). A2A peer availability is checked at runtime each run, with unavailability announced to the LLM (`lib/crewai/src/crewai/a2a/templates.py:22-29`).

**3. Is there a single point of failure?**
Yes, by design, in both crew processes. Sequential: the `_execute_tasks` loop is one synchronous thread; an exception in any task propagates out of `kickoff` and aborts the whole run after emitting `CrewKickoffFailedEvent` (`lib/crewai/src/crewai/crew.py:1068-1086`). Hierarchical: every task is executed *by* the manager (`lib/crewai/src/crewai/crew.py:1714-1717`); if the manager LLM fails or produces unusable tool calls, no worker is ever reached and the run fails — there is no fallback coordinator, retry-quorum, or health check for the manager itself. Additionally, in-process delegation exceptions are swallowed and returned as formatted error strings to the delegating LLM (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:121-124`), which prevents hard crashes but means a broken delegate can silently steer the manager. Flows reduce blast radius (independent listener branches, `test_flow_with_exceptions` at `lib/crewai/tests/test_flow.py:480`), and A2A bounds remote failures via timeouts and push-handler failure results (`lib/crewai/tests/a2a/test_a2a_integration.py:241-297`).

**4. Can agents discover each other?**
Yes, via three discovery channels. (a) In-crew: role names are enumerated into delegation tool descriptions at tool-build time (`lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:24-34`), and lookup normalizes whitespace/quotes and casefolds the requested `coworker` against those roles (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:20-35,72-84`); unmatched roles return an error listing available coworkers (`:99-108`). (b) Cross-process: A2A AgentCards are fetched from configured endpoints concurrently and rendered into the prompt as the `<AVAILABLE_A2A_AGENTS>` block (`lib/crewai/src/crewai/a2a/wrapper.py:240-279`, `lib/crewai/src/crewai/a2a/templates.py:7-9`). (c) Structural: Flow visualization can render the method-level communication graph statically (`lib/crewai/src/crewai/flow/visualization/builder.py:111-146`). There is no dynamic registry, broadcast query, or capability-based matching beyond what the LLM reads from these descriptions.

## Architectural Decisions

1. **Topology as an enum, not a strategy object.** Only two values exist and dispatch is an if/elif in `kickoff` (`lib/crewai/src/crewai/process.py:4-11`, `lib/crewai/src/crewai/crew.py:1051-1058`). Adding a topology means touching `Crew` directly — deliberately minimal (the abandoned `consensual` TODO confirms scope discipline).
2. **Coordination expressed as LLM tools.** Rather than a scheduler, hierarchy is implemented by giving the manager two tools whose arguments (`coworker`, `task`, `context`) are free strings (`lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:8-20`). This makes the topology inspectable in prompts and unit-testable without LLM calls (`lib/crewai/tests/test_crew.py:418-438`) but moves correctness onto string matching.
3. **Manager executes tasks; workers never see the task list.** In hierarchical mode `_get_agent_to_use` always yields the manager (`lib/crewai/src/crewai/crew.py:1714-1717`), so task assignment is emergent from delegation rather than declared — matching the docs' claim that "tasks are not pre-assigned" (`docs/edge/en/concepts/processes.mdx:57`).
4. **Flows coordinate code, crews coordinate agents.** Flows treat a whole `Crew().kickoff()` as an opaque step inside a method (`docs/edge/en/concepts/flows.mdx:240`), giving composition-level graphs while keeping intra-crew topology independent.
5. **Federation behind method wrapping.** A2A is attached by metaclass magic wrapping four public methods (`lib/crewai/src/crewai/agent/internal/meta.py:48-79`, `lib/crewai/src/crewai/a2a/wrapper.py:166-235`), keeping remote coordination opt-in and invisible to crews that don't use it.

## Notable Patterns

- **Blackboard-flavored relay**: sequential crews approximate a blackboard where each task writes its raw output and downstream tasks read aggregates (`lib/crewai/src/crewai/crew.py:1866-1874`).
- **Fan-out/fan-in within a stage**: async tasks accumulate as futures and are joined before the next sync task, giving bounded concurrency without changing ordering guarantees (`lib/crewai/src/crewai/crew.py:1607-1625`).
- **Prompt-side capability advertisement**: both coworker lists (local) and AgentCards (remote) are surfaced as text blocks in prompts, unifying discovery representation across topologies (`lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:24-34`, `lib/crewai/src/crewai/a2a/templates.py:7-14`).
- **Racing-condition hygiene in graphs**: multi-event `or_()` listeners use a lock-guarded fired-set plus racing-group detection so exactly one listener fires (`lib/crewai/src/crewai/flow/runtime/__init__.py:1075-1078`, tested at `lib/crewai/tests/test_flow.py:185`).
- **Nested-run correlation**: a contextvar-bound execution UUID lets enterprise hosts stamp outermost kickoff ids inherited by child flows and AgentExecutors (`lib/crewai/src/crewai/execution.py:1-61`) — observability-aware topology design.

## Tradeoffs

- **LLM-driven routing vs determinism**: delegation quality depends on the manager model picking valid role strings; mitigations are cosmetic (sanitization) rather than structural (no enum-typed coworker parameter) (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:37-44`).
- **In-process sync delegation vs isolation**: delegated sub-work runs synchronously on the delegator's thread and shares its cache handler/RPM controller (`lib/crewai/src/crewai/crew.py:1543-1548`); simple, but deep hierarchies stack frames and there is no timeout on a delegate.
- **Static membership vs scale**: fixed `agents` list keeps validation simple (`lib/crewai/src/crewai/crew.py:721-743`) but rules out mid-run agent spawning; scaling out requires Flows/A2A layering instead.
- **String errors vs structured failures**: returning error text keeps the ReAct loop alive for self-correction, at the cost of machine-checkable failure semantics (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:121-124`).

## Failure Modes / Edge Cases

- **Role-name collisions**: duplicate roles resolve to `agent[0]` silently (`lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:80-84`); the varied-case/spaces test covers formatting variance, not duplication (`lib/crewai/tests/test_crew.py:474-524`).
- **Manager-with-tools contradiction**: `_create_manager_agent` warns, clears tools, then raises anyway — the warning path is unreachable dead effect (`lib/crewai/src/crewai/crew.py:1522-1529`).
- **Hierarchical resume ambiguity**: replay marks resuming tasks by `manager_agent` role rather than task agent (`lib/crewai/src/crewai/crew.py:489-496`), coupling checkpoint recovery to the manager instance.
- **Unavailable remote peers**: A2A card fetch failures degrade to prompt notices instructing the LLM not to delegate (`lib/crewai/src/crewai/a2a/templates.py:22-29`); correctness then rests on the model obeying.
- **Sync A2A wrapper blocking**: `execute_a2a_delegation` documents that it "blocks the entire thread by creating and running a new event loop" (`lib/crewai/src/crewai/a2a/utils/delegation.py:163-165`) — a deadlock risk inside already-async contexts.
- **OR-listener races across cyclic flows**: handled via locks and re-arm sets (`lib/crewai/src/crewai/flow/runtime/__init__.py:1023-1078`), indicating this was a real historical bug class (tests `lib/crewai/tests/test_flow.py:209,239`).

## Future Considerations

- Replace free-string coworker addressing with a validated identifier (enum/index) or add duplicate-role detection at validation time (`lib/crewai/src/crewai/crew.py:745+` validators are the natural home).
- Add manager failover or a secondary coordinator option for hierarchical crews; currently the only resilience is upstream retry by the caller.
- Surface delegation outcomes as structured events distinct from ordinary tool observations, leveraging the existing event bus (`lib/crewai/src/crewai/events/event_bus.py:572`).
- Consider a third built-in process for peer-to-peer groups (currently achievable only implicitly via `allow_delegation=True` in sequential mode, `lib/crewai/src/crewai/crew.py:1820-1830`).

## Questions / Gaps

- No evidence found of distributed intra-crew execution across machines: all coordination except A2A client calls is same-process (searched `lib/crewai/src/crewai` for queue/celery/redis references; the `execution.py:3-4` docstring references Enterprise Celery but no such transport exists in this source tree).
- No evidence found of dynamic agent spawning or mid-run membership change APIs; searched `crew.py` for mutation of `agents` outside construction and found only copy/restore paths (`lib/crewai/src/crewai/crew.py:498-522`, `lib/crewai/src/crewai/crew.py:1113`).
- The `pipeline` test directory exists but contains only an `__init__.py` (`lib/crewai/tests/pipeline/__init__.py`); if a Pipeline topology was planned, it is absent from this snapshot.

---

Generated by `15.01-coordination-topology` against `crewai`.
