# Source Analysis: pydantic-ai

## 15.01 Coordination Topology

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (asyncio), packages: `pydantic-ai-slim` (agent framework), `pydantic-graph` (graph library powering the agent loop) |
| Analyzed | 2026-08-25 |

## Summary

Pydantic AI does not ship a runtime coordination engine; it ships primitives from which the application composes a topology. The documented and implemented model is a hierarchy of patterns ("roughly five levels of complexity", `docs/multi-agent-applications.md:3-9`):

1. **Single agent** — the default; one agent loop.
2. **Agent delegation (supervisor-worker)** — a parent agent calls a child agent inside an `async def` tool and takes back control when the child finishes (`docs/multi-agent-applications.md:13-20`). Communication is a plain Python function call: the delegate's structured `output` becomes the tool's return value (`docs/multi-agent-applications.md:43-49`). There is no message bus, queue, or broker.
3. **Programmatic hand-off (pipeline)** — application code runs agents in succession and decides which agent runs next, passing context explicitly via `message_history` (`docs/multi-agent-applications.md:205-215`, `docs/message-history.md:503-533`). Orchestration lives in user code, not in the framework.
4. **Graph-based control flow** — `pydantic_graph` provides a typed static DAG with fork/join parallelism for complex multi-agent workflows (`docs/multi-agent-applications.md:8`, `pydantic_graph/pydantic_graph/node.py:61-80`, `pydantic_graph/pydantic_graph/join.py:151-175`).
5. **Deep Agents** — a composition recipe (planning toolsets, file toolsets, sub-agent delegation, sandboxing) rather than a new topology (`docs/multi-agent-applications.md:364-374`).

The resulting communication topology of a multi-agent run is a **dynamic call tree rooted at one supervisor run**: edges are created when the LLM chooses to invoke a delegation tool, but the *possible* edge set is fixed by the tools registered on each agent. Within a single step, sibling tools fan out concurrently under configurable execution strategies (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:254-297`). Cross-agent accounting and control are handled through shared mutable objects passed down the tree: a shared `RunUsage` object (`usage=ctx.usage`, `pydantic_ai_slim/pydantic_ai/agent/abstract.py:511`) and a shared `CancellationToken` that can cancel a whole tree of runs (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-90`). Coordination is centralized in one OS process/event loop; there is no peer discovery, no blackboard, no marketplace, and no group-chat substrate. Network-level agent interop (A2A) was deliberately moved upstream to the external `fasta2a` package (`docs/changelog.md:178`, `docs/changelog.md:221`, `docs/migration.md:24`).

## Rating

**7 / 10** — Clear, well-documented model with explicit interfaces and real tests covering delegation semantics, including hard edge cases (sub-agent self-cancellation isolation, usage continuation checks, nested sync-agent rejection). What keeps it out of 8–10: aggregation and dependency compatibility between delegating agents are conventions rather than enforced contracts (passing `usage=ctx.usage` is opt-in and silently skipped if forgotten, `docs/multi-agent-applications.md:20`; deps compatibility is "generally" advice at `docs/multi-agent-applications.md:101`); there is no built-in discovery or dynamic topology mechanism (by stated design, per the repo philosophy in `AGENTS.md`: strong primitives over opinionated frameworks); and durable-execution delegation has known caveats where usage does not flow across activity boundaries (`docs/multi-agent-applications.md:84-85`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Topology taxonomy (delegation, hand-off, graphs, deep agents) | Docs enumerate five composition levels | `docs/multi-agent-applications.md:3-11` |
| Supervisor-worker delegation via tools | Parent tool awaits child `run()` and returns its output as tool result | `docs/multi-agent-applications.md:43-49`; `tests/test_nested_sync_agent.py:66-83` |
| Delegation control-flow diagram | Mermaid graph of parent → tool → child → back | `docs/multi-agent-applications.md:89-97` |
| Programmatic hand-off (app-code orchestration) | Successive agent runs sharing `message_history` | `docs/multi-agent-applications.md:205-215`; `docs/message-history.md:503-533` |
| Run entry point carries coordination state | `run(...)` accepts `message_history`, `usage`, `usage_limits`, `cancellation_token`, `deps` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:496-519` |
| Per-run internal topology | Agent run is itself a fixed graph: START → `UserPromptNode` → `ModelRequestNode` ↔ `CallToolsNode` → `SetFinalResult` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2570-2599` |
| Tool-call node (loop pivot) | `CallToolsNode` decides next node or final result | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1816-1851` |
| Intra-step parallelism (fan-out) with strategies | `process_tool_calls`: `early`/`graceful`/`exhaustive` end strategies; function tools run in parallel within segments; `sequential=True` tools act as barriers | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:254-297` |
| Parallel execution mode switch | `parallel_tool_call_execution_mode(...)` context manager | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1883` |
| Sub-agent cancellation isolation | A delegate's self-cancellation surfaces as a failed tool return instead of tearing down the parent | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62` |
| Whole-tree cancellation | Thread-safe `CancellationToken` registers many runs; one `cancel()` cancels all | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-90` |
| Cancellation arbitration internals | `RunCancellation` counts issued cancels, resolves first-party vs external | `pydantic_ai_slim/pydantic_ai/_cancel.py:92-257` |
| Usage aggregation channel | Shared mutable `RunUsage` passed to delegates; `RunUsage.__add__` merges | `pydantic_ai_slim/pydantic_ai/usage.py:383`; `pydantic_ai_slim/pydantic_ai/_run_context.py:68-79` |
| Mid-turn budget enforcement | `_check_continuation_usage` checks provisional totals against `UsageLimits` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-893` |
| Graph fan-out/fan-in primitives | `Fork` splits into parallel branches; `Join` synchronizes/aggregates with reducers | `pydantic_graph/pydantic_graph/node.py:61-80`; `pydantic_graph/pydantic_graph/join.py:151-175` |
| Human-in-the-loop / external executor channel | `DeferredToolRequests` (external calls + approvals) suspend/resume runs | `pydantic_ai_slim/pydantic_ai/_deferred.py:27-96` |
| No runtime discovery | Agents referenced as module-level globals; `name` only labels tracing spans | `docs/multi-agent-applications.md:18,71`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1303-1304` |
| A2A moved upstream | `a2a` extra removed; `Agent.to_a2a()` migrated to `fasta2a` package | `docs/changelog.md:178,221`; `docs/migration.md:24` |
| Delegation guard rails | Sync delegation rejected inside running agents (`UserError`) | `tests/test_nested_sync_agent.py:40-63` |
| Token-cancels-two-runs test | One shared token cancels two concurrent runs | `tests/test_run_cancellation.py:201` |
| Durable delegation caveat | Temporal activities receive copied run context; `usage=ctx.usage` does not cross the boundary | `docs/multi-agent-applications.md:84-85`; `tests/test_temporal.py:11459-11530` |

## Answers to Dimension Questions

1. **How do agents coordinate?**
   Through three mechanisms, all explicit data/control passing rather than messaging middleware:
   - *Tool-call delegation*: a parent's tool awaits `delegate.run(...)` and returns the child's output as the tool result; the parent LLM then continues reasoning over it (`docs/multi-agent-applications.md:13-49`). The delegate shares the parent's `RunContext.usage` object so token/cost accounting aggregates up the tree (`docs/multi-agent-applications.md:20`, `pydantic_ai_slim/pydantic_ai/agent/abstract.py:511`).
   - *Programmatic hand-off*: user code sequences agent calls and transfers conversation state by passing `result.new_messages()` as the next agent's `message_history` (`docs/message-history.md:503-533`).
   - *Graph control flow*: `GraphBuilder` wires nodes/edges statically; `Fork`/`Join` provide typed fan-out/fan-in with reducer-based aggregation (`pydantic_graph/pydantic_graph/node.py:61-80`, `pydantic_graph/pydantic_graph/join.py:151-175`). Control-plane signals (cancel whole tree) travel through a shared `CancellationToken` (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-75`).

2. **Is the topology fixed or dynamic?**
   The wiring is static per application build-out: which agents exist, which delegation tools are registered, and (for graphs) which edges connect nodes are all decided in application code before any run. What is dynamic at runtime is edge *traversal* — the parent LLM chooses whether and when to invoke a delegation tool, so the realized call tree varies per input. There is no API to add/remove agents or rewire mid-run. Even the internal agent loop is a prebuilt static graph (`build_agent_graph`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2570-2599`).

3. **Is there a single point of failure?**
   Yes, structurally: everything executes in one process on the root run's event loop; a parent crash takes down all delegates, and no cross-process replication exists in core. Mitigations are layered rather than distributed: (a) sub-agent self-cancellation is converted into a failed tool return so the parent survives (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:41-62`); (b) first-party vs external cancellation races are arbitrated so foreign cancellations still propagate correctly (`pydantic_ai_slim/pydantic_ai/_cancel.py:12-16,203-242`); (c) durable-execution integrations (Temporal/DBOS/Prefect) externalize crash recovery while keeping the single-workflow coordination model (`docs/durable_execution/dbos.md:200-205`).

4. **Can agents discover each other?**
   No. Discovery is by direct Python reference: agents are "stateless and designed to be global" module-level objects captured in tool closures (`docs/multi-agent-applications.md:18`). An agent `name` exists for span labeling and is optionally inferred from the assignment frame (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1303-1304`), not for lookup — searches found no registry resolving names to agents at runtime. Network/service-level discovery is out of scope in this version: the `a2a` extra was removed and `Agent.to_a2a()` moved to the upstream `fasta2a` package (`docs/changelog.md:178`, `docs/migration.md:24`). Tool-side discovery (e.g., deferred tool loading) exists but concerns tools, not agents.

## Architectural Decisions

- **Coordination as composition of primitives, not a framework.** The repo philosophy explicitly prefers "strong primitives, powerful abstractions, and general solutions... over narrow solutions for specific use cases" (`AGENTS.md`, Philosophy section). Consequence: supervisor-worker, pipeline, and DAG topologies are user-composed from the same `run()` surface (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:496-519`).
- **Function-call semantics for delegation.** A delegate is invoked like any tool and returns a value into the parent transcript (`docs/multi-agent-applications.md:43-49`); this makes the delegate boundary observable as ordinary tool events and lets retry/approval machinery apply uniformly (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:254-297`).
- **Explicit state channels instead of implicit context.** Usage, limits, history, and cancellation are parameters on every `run()` call, so each edge of the tree declares what it shares. This is why Temporal activities get a *copy* of the run context and lose usage propagation unless bridged (`docs/multi-agent-applications.md:84-85`).
- **Cancellation as a two-tier design**: per-run first-party cancellation with precise asyncio bookkeeping (`RunCancellation`, `pydantic_ai_slim/pydantic_ai/_cancel.py:92-257`) plus a thread-safe fan-out token for trees (`CancellationToken`, `pydantic_ai_slim/pydantic_ai/_cancel.py:42-90`).
- **Static internal graph.** Each agent run compiles to a four-node graph built once (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2581-2599`), reusing the same graph engine exposed to users for custom topologies.

## Notable Patterns

- **Barrier-segmented parallel tool execution**: within one model response, tools run in parallel inside segments split by `sequential=True` barriers, under three selectable end strategies (`early`, `graceful`, `exhaustive`) with a documented "retry-wins" invariant (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:265-290`).
- **Failure isolation envelope for sub-agents**: `cancelled_sub_agent_return` converts a nested `RunCancelled` into a failed `ToolReturnPart` the parent model can react to — deliberately shared by the graph path and realtime sessions so the two cannot drift (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:44-55`).
- **Shared-mutable-object accounting**: the same `RunUsage` instance flows down the delegation tree and is incremented in place (`pydantic_ai_slim/pydantic_ai/usage.py:383`), giving tree-wide budgets without a coordinator process; `UsageLimits.check_tokens` is enforced mid-turn against provisional totals (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-886`).
- **History as interchange format**: `ModelMessage` lists are the lingua franca for hand-offs, including cross-provider (OpenAI agent → Anthropic agent) (`docs/message-history.md:511-533`).
- **Suspend/resume coordination with humans and external systems**: deferred tools pause a run and resume it later with `DeferredToolResults`, validated against pending tool-call IDs (`pydantic_ai_slim/pydantic_ai/_deferred.py:27-96`).

## Tradeoffs

- **Simplicity vs enforcement**: because delegation is just a tool calling another agent, nothing forces you to propagate `usage` or compatible `deps`; forgetting `usage=ctx.usage` silently under-counts spend, and deps mismatches surface only at runtime (`docs/multi-agent-applications.md:20,101`).
- **In-process centrality vs operability**: single-event-loop coordination makes tracing trivially correct (one OTel trace per tree, labeled per agent via `name`, `docs/multi-agent-applications.md:384-406`) but caps scale and fault domains; going multi-process means leaving the framework (Temporal) or the repo (fasta2a).
- **Static wiring vs flexibility**: static edges make behavior auditable and cache-prefix stable (a stated testing concern, see `tests/AGENTS.md` cache-prefix guidance), at the cost of any swarm-style runtime membership change.
- **Upstreaming A2A** keeps core slim but means networked agent-to-agent topology now depends on an external package with its own lifecycle (`docs/migration.md:24`).

## Failure Modes / Edge Cases

- **Nested sync delegation is a hard error**: calling `run_sync()`/`run_stream_sync()` inside a tool during a run raises `UserError` (documented at `docs/multi-agent-applications.md:79-82`; pinned by `tests/test_nested_sync_agent.py:40-63`, including the threads-disabled environment variant).
- **Sub-agent self-cancel vs caller cancel ambiguity**: resolved by convention — `RunCancelled` seen inside a tool body is always the nested run's, since the caller's own cancellation arrives as `CancelledError`; a delegate that wants the caller cancelled must catch and re-issue `ctx.cancel()`, and whole-tree cancel requires a shared token (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:46-53`).
- **External-vs-first-party cancellation races**: attribution is count-based, not identity-based, leaving a narrow window if user code uncancels and an external cancel lands before rebinding (`pydantic_ai_slim/pydantic_ai/_cancel.py:141-152,218-223`, issue #7240 referenced).
- **Durable-boundary divergence**: under Temporal, `usage=ctx.usage` does not aggregate across activity boundaries because the context is copied (`docs/multi-agent-applications.md:84-85`); the framework tests the workflow-vs-in-process difference directly (`tests/test_temporal.py:11503-11530`).
- **Parallel output arbitration**: multiple output tools racing under `graceful`/`exhaustive` strategies need the retry-wins suppression and per-index bookkeeping to avoid committing a losing output after retries (`pydantic_ai_slim/pydantic_ai/_tool_execution.py:285-290,356-364`).

## Future Considerations

- Enforce or lint delegation hygiene: an opt-in check that a delegation tool propagates `usage`/`deps` would convert today's documentation convention (`docs/multi-agent-applications.md:20,101`) into a verifiable contract.
- Identity-based issuance tracking in `RunCancellation` would close the residual mis-attribution window acknowledged in code (`pydantic_ai_slim/pydantic_ai/_cancel.py:218-223`).
- If networked multi-agent deployments become a target, re-evaluate whether `fasta2a`-mediated A2A deserves first-class docs here, since discovery currently ends at Python references.

## Questions / Gaps

- **No evidence found for peer-to-peer, group-chat, blackboard, or marketplace coordination**: searched `docs/*.md`, `pydantic_ai_slim/**`, and `pydantic_graph/**` for `peer`, `blackboard`, `marketplace`, `group chat`, and broadcast patterns; the only hits were unrelated `TypeError` handling. Coordination is strictly hierarchical/pipeline/graph.
- **No runtime agent registry**: searched for `registry` in `pydantic_ai_slim/pydantic_ai/agent/`; hits concern capability/spec registries (`agent/spec.py:15`, `agent/__init__.py:4113-4136`), not agent lookup by name or capability.
- **Dynamic topology changes**: no API found for rewiring agents or graphs mid-run; if such support exists, it is outside this source (the upstream `fasta2a` package is not vendored here).
- Realtime sessions share the same isolation helper (`realtime._session._unsettled_call_return`, referenced at `pydantic_ai_slim/pydantic_ai/_tool_execution.py:53-54`) but a full realtime-topology study was out of scope for this dimension.

---

Generated by `15.01-coordination-topology` against `pydantic-ai`.
