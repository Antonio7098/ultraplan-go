# Source Analysis: agent-framework

## Dimension 15.01: Coordination Topology

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary analyzed) + .NET/C# parity; monorepo also contains `go/` and `declarative-agents/` |
| Analyzed | 2026-08-25 |

## Summary

Microsoft Agent Framework implements coordination as a **layered system**. At the bottom sits a graph execution engine (`Workflow`) that runs a directed graph of `Executor` nodes in **Pregel-style synchronized supersteps**, delivering typed messages only along statically declared edges (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:208-227`, `_runner.py:45-46`). On top of that engine, a dedicated `agent_framework_orchestrations` package provides five named multi-agent topologies — Sequential (pipeline), Concurrent (fan-out/fan-in star), Handoff (decentralized mesh), Group Chat (centralized hub-and-spoke), and Magentic (orchestrator-worker with plan/progress ledgers) — each assembled by a builder that wires the appropriate edge pattern (`studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/agent_framework_orchestrations/__init__.py:3-11`).

Communication is exclusively **typed message passing over edges** plus an optional **workflow-scoped shared state** with superstep-staged commit semantics (a blackboard-like facility, `_state.py:6-23`). The framework is therefore not one topology but a topology *kit*: coordination style is chosen at build time, is immutable for the life of a workflow instance (enforced by a graph-signature hash on checkpoint resume, `_workflow.py:332-336`), and dynamic behavior is achieved through runtime routing predicates, selection functions, and LLM-driven speaker/handoff decisions rather than by mutating the graph.

## Rating

**8 / 10.**

Rationale per rubric:

- **Clear model (7-8 band):** Every topology is an explicit builder with documented wiring; the handoff module even states its own contrast with group chat ("Group Chat: centralized orchestration... Handoff: decentralized routing by agents themselves", `_handoff.py:16-21`). Interfaces are typed end-to-end (`WorkflowContext[OutT, W_OutT]` generics, `_workflow_context.py:318-350`).
- **Tests:** 169 orchestration-specific tests exist across seven files (`test_group_chat.py`: 35, `test_magentic.py`: 38, `test_handoff.py`: 27, `test_orchestration_intermediate_vs_terminal.py`: 26, `test_orchestration_request_info.py`: 16, `test_sequential.py`: 15, `test_concurrent.py`: 12 under `python/packages/orchestrations/tests/`).
- **Operational safeguards:** max-rounds and termination conditions (`_base_group_chat_orchestrator.py:140-141,499-518`), convergence cap raising `WorkflowConvergenceException` after 100 supersteps (`_runner.py:176-177`), stall detection + replanning in Magentic (`_magentic.py:1117-1125`), unknown-participant/target validation (`_group_chat.py:251-252`, `_handoff.py:400-404`), single-active-run lock (`_workflow.py:768-771`).
- **Observable:** every message send creates a producer OTel span (`_workflow_context.py:330-333`); topology can be exported as DOT/Mermaid/SVG/PNG/PDF for humans to "draw" it (`_viz.py:29,64,142,155,168,181`).
- **Not 9-10 because:** execution is confined to a single asyncio event loop — there is no cross-process/cross-node distribution or coordinator failover inside this source (durable execution is delegated to an external extension, per `python/AGENTS.md` package notes). A centralized group-chat/Magentic orchestrator is a single point of failure within a run, recoverable only via checkpointing. Group chat without `max_rounds`/`termination_condition` "will continue indefinitely" by design (`_group_chat.py:142-143`).

## Evidence Collected

Every entry cites workspace-relative paths into the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Graph engine | `Workflow` docstring: "graph-based execution engine... Pregel-like model, running in supersteps until the graph becomes idle" | `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_workflows/_workflow.py:208-227` |
| Superstep loop | `RunnerImpl.run_until_convergence()` iterates supersteps, commits shared state at boundaries, checkpoints per superstep, stops when no messages remain | `.../_workflows/_runner.py:106-177` |
| Message delivery semantics | Concurrent delivery across sources; per-edge ordering preserved; cross-source ordering to same target unspecified | `.../_workflows/_runner.py:179-199` |
| Directed conditional edges | `Edge(source_id, target_id, condition)`; `should_route(data)` predicate decides traversal at runtime | `.../_workflows/_edge.py:76-91,162-169` |
| Edge group kinds | `SingleEdgeGroup`, `FanOutEdgeGroup`, `FanInEdgeGroup`, `SwitchCaseEdgeGroup` (with cases/default) | `.../_workflows/_edge.py:470,501,616,808` |
| Dynamic target selection | `FanOutEdgeGroup.selection_func(message, available_targets)` lets a message pick its targets at runtime | `.../_workflows/_edge.py:501-595` |
| Communication API | `ctx.send_message(msg, target_id=None→all edge targets)`, `ctx.yield_output()`, `ctx.add_event()` | `.../_workflows/_workflow_context.py:318-401` |
| Human-in-the-loop channel | `ctx.request_info(request_data, response_type)` emits request_info event; workflow parks at `IDLE_WITH_PENDING_REQUESTS` | `.../_workflows/_workflow_context.py:403-434`; `_workflow.py:249-256` |
| Blackboard-style shared state | `State` with pending-buffer + commit-at-superstep-boundary; last write wins within a superstep | `.../_workflows/_state.py:6-43,90-100` |
| State access from executors | `ctx.get_state(key)` / `ctx.set_state(key, value)` | `.../_workflows/_workflow_context.py:436-442` |
| Sequential wiring | Input normalizer chained via repeated `builder.add_edge(prior, p)` — pipeline topology | `.../orchestrations/agent_framework_orchestrations/_sequential.py:9,260-272` |
| Concurrent wiring | Dispatcher fans out to all participants; participants fan-in to aggregator (`add_fan_out_edges`/`add_fan_in_edges`) | `.../_concurrent.py:55-80,429-432` |
| Concurrent aggregator | Default `_AggregateAgentConversations` collects one assistant message per participant in deterministic order | `.../_concurrent.py:83-136` |
| Handoff = decentralized | Module docstring contrasts centralized group chat vs decentralized handoff routing | `.../_handoff.py:16-21` |
| Handoff mechanism | Synthetic `handoff_to_<target>` tools injected per agent; `_AutoHandoffMiddleware` short-circuits tool exec with synthetic result | `.../_handoff.py:124-129,306-350,132-154` |
| Handoff routing act | Agent executor detects handoff result and sends directly to target: `await ctx.send_message(..., target_id=handoff_target)` + `handoff_sent` event | `.../_handoff.py:397-419` |
| Handoff default mesh | With no explicit config, "all agents can hand off to all others by default (mesh topology)" | `.../_handoff.py:1062-1072`; builder wires fully-connected fan-out graph at `999-1010` |
| Group chat hub-and-spoke | Builder adds bidirectional edges orchestrator↔participant for each participant | `.../_group_chat.py:1005-1040` |
| Speaker selection (function) | `GroupChatSelectionFunction = Callable[[GroupChatState], str \| Awaitable[str]]`; unknown return raises `RuntimeError` | `.../_group_chat.py:94-96,239-254` |
| Speaker selection (LLM manager) | `AgentBasedGroupChatOrchestrator` asks an agent for structured `{terminate, reason, next_speaker, final_message}` | `.../_group_chat.py:262-300` |
| Participant registry | `ParticipantRegistry` validates unique IDs, tracks agent-vs-executor type and descriptions | `.../_base_group_chat_orchestrator.py:86-127` |
| Orchestrator broadcast/request | `_broadcast_messages_to_participants()` and dual-envelope `_send_request_to_participant()` (AgentExecutorRequest vs GroupChatRequestMessage) | `.../_base_group_chat_orchestrator.py:411-495` |
| Round-limit safeguard | `_check_round_limit()` logs warning and forces completion when `_round_index >= _max_rounds` | `.../_base_group_chat_orchestrator.py:499-518` |
| Magentic supervisor loop | Orchestrator plans task ledger, then per round: create progress ledger → check satisfied → check stalling → select next speaker + instruction → request participant | `.../_magentic.py:864-881,1057-1154` |
| Magentic ledger artifacts | `MAGENTIC_MANAGER_NAME="magentic_manager"`; msg kinds user_task/task_ledger/instruction/notice; `_MagenticTaskLedger`, `MagenticProgressLedger` | `.../_magentic.py:60-67,272-308` |
| Magentic stall/replan | `stall_count > max_stall_count` triggers `_reset_and_replan`; invalid next speaker falls back to final answer | `.../_magentic.py:1117-1125,1128-1138` |
| Magentic human sign-off | Optional `require_plan_signoff` issues `request_info(MagenticPlanReviewRequest)` before inner loop | `.../_magentic.py:893-908,1040-1055` |
| Topology immutability on resume | `graph_signature_hash` computed at build; checkpoint restore rejects changed graphs | `.../_workflows/_workflow.py:330-336`; `_runner.py:312-317` |
| Single-run concurrency guard | Second `run()` on same instance raises `WorkflowException("Workflow is already running...")` | `.../_workflows/_workflow.py:759-771` |
| Nested composition | `WorkflowExecutor` wraps a child workflow as an executor; sub-workflow owns its pending requests | `.../_workflows/_workflow_executor.py:41,106-169` |
| Workflow-as-agent | `WorkflowAgent(BaseAgent)` exposes a workflow through the agent interface | `.../_workflows/_agent.py:52` |
| Topology visualization | `to_digraph()` (DOT), `to_mermaid()`, `export()/save_svg/save_png/save_pdf` render the communication graph incl. fan-in digests and sub-workflow clusters | `.../_workflows/_viz.py:29-62,181,213-255,301-344` |
| Cross-process discovery (A2A) | `A2AExecutor` bridges local agents onto the A2A protocol with `AgentCard` discovery metadata; `A2AAgent` client calls remote agents by URL | `studies/agent-harness-study/sources/agent-framework/python/packages/a2a/agent_framework_a2a/_a2a_executor.py:31-92`; `packages/a2a/AGENTS.md` usage section |
| .NET parity | `OrchestrationBuilderBase<T>` mirrors output-designation surface; `GroupChatWorkflowBuilder` adds host↔participant edges identically | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Workflows/OrchestrationBuilderBase.cs:15-16`; `GroupChatWorkflowBuilder.cs:77-78` |
| Tests: termination/rounds | `test_max_rounds_enforcement`, `test_termination_condition_halts_conversation` | `studies/agent-harness-study/sources/agent-framework/python/packages/orchestrations/tests/test_group_chat.py:383,409` |
| Tests: bad routing | `test_unknown_participant_error` asserts failure when selection returns unregistered name | `.../orchestrations/tests/test_group_chat.py:532` |

## Answers to Dimension Questions

**1. How do agents coordinate?**
Through three channels. (a) **Edge-routed typed messages**: executors call `ctx.send_message()` which wraps data in a `WorkflowMessage` and routes along declared edges — broadcast when `target_id=None`, point-to-point when set (`_workflow_context.py:318-348`). Delivery happens once per superstep via edge runners (`_runner.py:201-229`). (b) **Shared state**: a workflow-wide key/value `State` visible to all executors, staged per superstep and committed at boundaries (`_state.py:14-23`). Group-chat and Magentic patterns additionally maintain a **conversation transcript** inside the orchestrator that is broadcast to keep all agents synchronized (`_base_group_chat_orchestrator.py:170,298-304,411-441`; `_handoff.py:352-363`). (c) **Events & requests**: observability events (`group_chat`, `magentic_orchestrator`, `handoff_sent`) and `request_info` for human-in-the-loop pauses (`_base_group_chat_orchestrator.py:66-81`; `_handoff.py:415-417`).

**2. Is the topology fixed or dynamic?**
The **wiring is fixed at build time and immutable during a run**: builders emit a static edge list, and resuming a checkpoint against a changed graph is rejected via `graph_signature_hash` mismatch (`_runner.py:312-317`). What *is* dynamic is **routing and role activation**: conditional edges evaluate predicates per message (`_edge.py:162-169`); switch-case groups pick a branch (`_edge.py:808`); fan-out selection functions choose targets per message (`_edge.py:582`); group chat picks a different speaker each round (`_group_chat.py:222-254`); handoff agents redirect control flow mid-conversation by invoking synthetic tools (`_handoff.py:397-419`). So: static graph, dynamic traffic.

**3. Is there a single point of failure?**
Yes, in two senses. Within the four orchestrated patterns except handoff, a single orchestrator/manager executor makes all routing decisions (hub-and-spoke wiring at `_group_chat.py:1035-1038` and `_magentic.py:1802-1805`; Magentic's manager holds plan, progress ledger, and stall counters, `_magentic.py:905-912`). There is no replica/failover mechanism in-source; recovery relies on checkpoint restore (`_runner.py:278-318`) rather than redundancy. Additionally the whole run lives in one asyncio process guarded by a strict single-active-run lock (`_workflow.py:759-771`), so process death loses in-flight work unless checkpointing was enabled. The handoff pattern deliberately avoids a central router — each agent routes itself (`_handoff.py:20-21`) — though it still depends on every agent being correctly configured; a missing handoff configuration merely logs "your workflow may get stuck" (`_handoff.py:1096-1099`) rather than failing fast.

**4. Can agents discover each other?**
Within a workflow, discovery is **registry-based and static**: participants are registered up front, IDs are validated for uniqueness, and descriptions are carried in `ParticipantRegistry` (`_base_group_chat_orchestrator.py:86-127`). Peers become visible to models through prompt content — e.g., Magentic seeds the manager context with `participant_descriptions` (`_magentic.py:936-940`) and group chat exposes name→description maps to the selection function (`_group_chat.py:79-88`). There is no runtime join/leave of participants. Across process boundaries, discovery follows the **A2A protocol**: an agent publishes an `AgentCard` (name, description, capabilities, URL) via A2A server routes, and clients connect with `A2AAgent(url=...)` (`_a2a_executor.py:43-92`; `packages/a2a/AGENTS.md`). No marketplace-style or blackboard-based brokered discovery exists inside this source (searched `packages/` for discovery registries beyond A2A cards and the static participant registry).

### Can you draw the communication topology of a multi-agent run?

Yes — three ways. Programmatically, `Workflow.to_digraph()` / `to_mermaid()` export the exact graph including fan-in digests and nested sub-workflow clusters (`_viz.py:29-62,181,301-344`). Conceptually, the five topologies look like this (arrows = allowed message paths; O = orchestrator):

```
Sequential      I → A1 → A2 → ... → An                    (_sequential.py:9)
Concurrent        ┌─► P1 ─┐                               (_concurrent.py:429-432)
             D ───┼─► P2 ─┼───► Agg        (fan-out/star)
                  └─► Pn ─┘
Handoff        every agent ⇄ every agent                 (_handoff.py:999-1010)
               (full mesh; active speaker chosen by
                handoff_to_X tool call)
Group Chat            O                                  (_group_chat.py:1035-1038)
                   ▲ │ ▲ │        bidirectional spokes;
              P1 ◄─┘ └►P2 ...     O selects next speaker
Magentic               O(manager)                        (_magentic.py:1802-1805)
              plan/ledger ► next_speaker+instruction
                   P1    P2    Pn   (star, manager-driven)
```

All of them compile down to the same substrate: a static edge graph executed in supersteps by `RunnerImpl`.

## Architectural Decisions

1. **One engine, many topologies.** All five orchestration builders lower into the same `WorkflowBuilder`/edge-group primitives (`_sequential.py:260-272`, `_concurrent.py:429-432`, `_group_chat.py:1029-1038`, `_handoff.py:989-1010`, `_magentic.py:1796-1805`). This keeps topology definitions declarative and serializable (`to_dict` at `_workflow.py:386-420`).
2. **Centralized conversation state in orchestrators for chat patterns.** `BaseGroupChatOrchestrator` owns `_full_conversation` and broadcasts deltas so participants stay synchronized without peer-to-peer messaging (`_base_group_chat_orchestrator.py:169-171,411-441`).
3. **Decentralization via tools, not transport.** The handoff pattern implements agent autonomy by injecting fake tools intercepted by function middleware, converting an LLM decision into a routing instruction (`_handoff.py:132-154,345-350`) — coordination logic rides on the existing function-calling channel.
4. **Ledger-driven supervision (Magentic).** Planning and evaluation are externalized behind `MagenticManagerBase.plan/create_progress_ledger` (`_magentic.py:469-512`), separating the supervision policy from the orchestration mechanics and allowing a custom manager implementation.
5. **Immutable topology identity.** The graph fingerprint hash ties checkpoints to a specific topology, trading flexibility for replay safety (`_workflow.py:333-336`).
6. **Two-language symmetry.** The .NET side reproduces the same builder taxonomy and edge wiring (`dotnet/src/Microsoft.Agents.AI.Workflows/GroupChatWorkflowBuilder.cs:77-78`), indicating topology is treated as a portable contract, not a language idiom.

## Notable Patterns

- **Hub-and-spoke with dual envelopes:** the group-chat base class distinguishes agent participants (receive raw `AgentExecutorRequest`) from custom executors (receive `GroupChatRequestMessage` envelope), letting non-agent nodes join the same topology (`_base_group_chat_orchestrator.py:443-495`).
- **Broadcast-minus-speaker:** after a response, messages are rebroadcast to everyone except the responder to avoid echo loops (`_group_chat.py:224-231`, `_magentic.py:982-989`).
- **Blackboard-lite:** `State.get/set` with pending/committed buffers gives executors indirect, keyed sharing without edges — last-write-wins per superstep is documented as consistent with .NET (`_state.py:30-43`).
- **Structured-output voting:** the agent-managed group chat forces the manager model into a schema with `terminate/next_speaker` fields, making speaker selection machine-checkable (`_group_chat.py:262-281`).
- **Self-documenting graphs:** DOT/Mermaid export exists precisely so topology can be reviewed and drawn, including internal executors optionally (`_viz.py:29-62`).

## Tradeoffs

- **Static graph vs. adaptivity:** fixing edges at build time yields serializable, checkpoint-safe, validatable topologies, but any true topology change (adding a participant mid-run) requires rebuilding and invalidates checkpoint continuity (`_runner.py:313-317`).
- **Centralized chat control:** hub-and-spoke simplifies synchronization and guarantees a single ordering authority, at the cost of the orchestrator being both throughput bottleneck and SPOF; the alternative (handoff mesh) removes the bottleneck but cannot guarantee coverage — an agent with no configured handoffs deadlocks the conversation (`_handoff.py:1096-1099`).
- **Tool-call-as-signal:** reusing function calling for handoff avoids new transports but leaks complexity: parallel tool calls must be disabled to avoid ambiguous handoffs (`_handoff.py:287-289`), and history persistence flags are mandatory to keep service-side transcripts consistent (`_handoff.py:952-969`).
- **Superstep batching:** Pregel semantics give deterministic boundaries and clean commit/checkpoint points, but messages are never delivered mid-superstep, adding latency for tight ping-pong loops like Magentic's inner cycle.
- **In-process scope:** rich coordination semantics, but no distributed runtime here; scale-out is outsourced to external durable extensions (per `python/AGENTS.md`).

## Failure Modes / Edge Cases

- **Non-convergence:** runaway message cycles hit `max_iterations=100` and raise `WorkflowConvergenceException` instead of hanging (`_runner.py:176-177`).
- **Unguarded group chats:** if neither `max_rounds` nor `termination_condition` is set, the conversation loops forever — explicitly warned in the constructor docs (`_group_chat.py:142-143`).
- **Bad speaker selection:** function-based selectors raise `RuntimeError` on unknown names (`_group_chat.py:251-252`); Magentic degrades more gracefully, warning and forcing a final answer when the ledger names an invalid speaker (`_magentic.py:1128-1138`).
- **Illegal handoff target:** validated against the source agent's configured targets; violation raises `ValueError` listing valid targets (`_handoff.py:400-404`).
- **Stalled teams:** Magentic counts non-progress rounds and triggers reset+replan past `max_stall_count`; ledger-parse failures retry up to `progress_ledger_retry_count=3` then trigger replan (`_magentic.py:1117-1125,722-738`).
- **Concurrent misuse:** overlapping `run()` calls are rejected synchronously (`_workflow.py:768-771`); fresh input while prior-run messages remain in flight is blocked with guidance to use checkpoints (`_workflow.py:838-845`).
- **Checkpoint/topology drift:** restoring with a rebuilt-but-different graph fails closed (`_runner.py:313-317`).
- **Test coverage of these modes:** termination, round limits, unknown participants, and checkpoint resume are exercised in `tests/test_group_chat.py:383,409,532,550` and per-pattern suites listed above.

## Future Considerations

- Add failover or replication options for orchestrator executors, or a degraded-mode where group-chat selection falls back to round-robin if the manager fails repeatedly (currently only Magentic has a fallback path).
- Support runtime participant admission/removal (dynamic membership) while preserving checkpoint compatibility — today the graph signature forbids any change.
- Surface a first-class "deadlock detector" for handoff meshes (static analysis could flag agents with empty handoff sets at build time instead of logging at runtime).
- Consider documenting distributed deployment story in-source; durable execution currently lives outside this repository.

## Questions / Gaps

- **No evidence found** for inter-node coordination (multi-host execution) inside this source: searched `python/packages/core/agent_framework/_workflows/` and `python/packages/hosting*` for remote executor transports; messaging APIs are in-process asyncio only (`_runner_context.py:106,345`). Distribution appears delegated to the external durable extension referenced in `python/AGENTS.md`.
- **No evidence found** for marketplace/brokered agent discovery: searched `packages/` for registry/marketplace concepts; the only discovery mechanisms found were static `ParticipantRegistry` (`_base_group_chat_orchestrator.py:86-127`) and A2A `AgentCard`s (`_a2a_executor.py:54-65`).
- The `go/` directory was not analyzed in depth for this dimension; findings reflect the Python implementation, with spot-check confirmation of .NET parity (`dotnet/src/Microsoft.Agents.AI.Workflows/GroupChatWorkflowBuilder.cs:77-78`). Whether Go exposes the same five topologies is unverified.

---

Generated by `dimensions/15.01-coordination-topology` against `agent-framework`.
