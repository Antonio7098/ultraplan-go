# Source Analysis: langgraph

## Dimension 15.02: Message Routing and Termination

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core `libs/langgraph`, prebuilt agents in `libs/prebuilt`) |
| Analyzed | 2026-08-25 |

## Summary

LangGraph does not implement "speaker selection" as a first-class concept; instead it generalizes multi-agent message routing into a **dataflow graph routing model** built on the Bulk Synchronous Parallel (Pregel) execution engine. Routing is expressed three ways: (1) static edges, (2) conditional edges evaluated by user-supplied router callables (`BranchSpec`), and (3) dynamic push-style dispatch via the `Send` packet and `Command(goto=...)` primitives. All routing ultimately reduces to writes into typed channels; a node runs in superstep N+1 when one of its subscribed trigger channels was updated in superstep N.

Termination is guaranteed structurally: the Pregel loop ends when no tasks are produced ("done"), or fails fast with `GraphRecursionError` when the configured `recursion_limit` is exhausted ("out_of_steps"). Deadlock in the blocking sense is impossible because each superstep is finite and channel updates are applied only at step boundaries; livelock (infinite cycling) is bounded by the recursion limit, and graceful cooperative shutdown exists via `RunControl` drain requests and per-step timeouts.

## Rating

**9/10**

Rationale: The routing model is explicit, layered (static edges → conditional branches → dynamic Send/Command), and implemented through small, well-typed channel primitives with compile-time validation (`StateGraph.validate`, `validate_graph`) and extensive test coverage including edge cases like Send dedupe on resume and nested-subgraph sends. Termination is provable under the BSP model (finite supersteps + hard recursion bound enforced at `tick()`), surfaced as a dedicated error type with an error code, and complemented by operational safeguards (per-step timeout, drain control, managed `RemainingSteps` for soft stops). It loses the last point only because handoff semantics between agents are delegated entirely to application-level `Command` usage with no framework-enforced handoff contract, and invalid `Send` targets are silently dropped with a log warning rather than failing loudly.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Conditional-edge router spec | `BranchSpec` wraps a router callable plus optional `path_map`; return-type `Literal` hints are auto-coerced into a path map for visualization/validation | libs/langgraph/langgraph/graph/_branch.py:83-120 |
| Router invocation & validation | `_route` invokes the path; `_finish` normalizes results to destinations, rejects `None`/`START` targets ("Branch did not return a valid destination") and rejects `Send` packets addressed to END | libs/langgraph/langgraph/graph/_branch.py:146-210 |
| Public API for conditional routing | `add_conditional_edges(source, path, path_map)` registers a `BranchSpec` on the source node; duplicate branch names rejected | libs/langgraph/langgraph/graph/state.py:982-1030 |
| Command/Send control-plane decode | `_control_branch` maps node returns: `Send` → write to reserved `TASKS` channel; `Command.goto` string targets → write to `branch_to:<node>` ephemeral channel; `Command.PARENT` raises `ParentCommand` | libs/langgraph/langgraph/graph/state.py:1749-1775 |
| Dynamic fan-out primitive | `Send(node, arg, timeout)` packet carries its own private state to a target node, enabling map-reduce dispatch | libs/langgraph/langgraph/types.py:704-792 |
| Handoff/navigation primitive | `Command(update=..., goto=node\|Send\|list, graph=None\|"__parent__")` combines state update and navigation in one value | libs/langgraph/langgraph/types.py:799-848 |
| Reserved TASKS channel setup | Compiled graph installs `Topic(Send, accumulate=False)` under the reserved `TASKS` key; user graphs cannot claim it | libs/langgraph/langgraph/pregel/main.py:804-809 |
| Task preparation (routing → scheduling) | `prepare_next_tasks` builds PUSH tasks from available `TASKS` topic entries and PULL tasks via `trigger_to_nodes` inversion of updated channels | libs/langgraph/langgraph/pregel/_algo.py:392-511 |
| Push task resolution | Unknown node names in pending sends are ignored with a warning (`Ignoring unknown node name ... in pending sends`) rather than raising | libs/langgraph/langgraph/pregel/_algo.py:961-999 |
| Node trigger wiring | Each node gets an EphemeralValue `branch_to:<key>` trigger channel; static edges append `ChannelWriteEntry(branch_to:end)` to the source's writers | libs/langgraph/langgraph/graph/state.py:1508-1547, 1551-1559 |
| Fan-in join semantics | Multi-source edges create a `join:...` NamedBarrierValue channel subscribed by the target — target fires only after all sources wrote | libs/langgraph/langgraph/graph/state.py:1560-1575 |
| Deferred join variant | `NamedBarrierValueAfterFinish` gates availability behind an explicit `finish()` call for `defer=True` nodes | libs/langgraph/langgraph/channels/named_barrier_value.py:84-167 |
| Barrier channel contract | `NamedBarrierValue.is_available()` returns true only when `seen == names`; unknown values raise `InvalidUpdateError` | libs/langgraph/langgraph/channels/named_barrier_value.py:56-81 |
| PubSub channel for sends | `Topic(accumulate=False)` is emptied after each step, so unexecuted sends do not leak across supersteps | libs/langgraph/langgraph/channels/topic.py:23-94 |
| Loop termination state machine | `tick()`: step > stop → `out_of_steps`; no tasks → `done`; `control.drain_requested` → `draining`; interrupt_before → raise `GraphInterrupt` | libs/langgraph/langgraph/pregel/_loop.py:599-681 |
| Hard step bound computation | `self.stop = self.step + self.config["recursion_limit"] + 1` recomputed from checkpoint metadata on resume | libs/langgraph/langgraph/pregel/_loop.py:1700-1701 |
| Recursion limit enforcement surface | After streaming loop exits, `status == "out_of_steps"` raises `GraphRecursionError` with `GRAPH_RECURSION_LIMIT` error code; same check duplicated in async path | libs/langgraph/langgraph/pregel/main.py:3002-3011, 3483-3492 |
| Config validation | `recursion_limit < 1` rejected up front | libs/langgraph/langgraph/pregel/main.py:2563-2564 |
| Default recursion limit | `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))`, merged into every config | libs/langgraph/langgraph/_internal/_config.py:32, 335 |
| Cooperative shutdown | `GraphDrained` raised when `RunControl.request_drain()` was called (e.g., SIGTERM); checkpoint saved, run resumable later | libs/langgraph/langgraph/errors.py:54-64; libs/langgraph/langgraph/pregel/main.py:3012-3015 |
| Per-step timeout safeguard | Runner executes each superstep under `timeout=self.step_timeout` | libs/langgraph/langgraph/pregel/main.py:2967-2972 |
| Soft-stop signal for agents | Managed `IsLastStep` / `RemainingSteps` expose `stop - step` to nodes without extra state plumbing | libs/langgraph/langgraph/managed/is_last_step.py:9-21 |
| Prebuilt agent router | `should_continue` routes agent→tools (via parallel `Send`s of each tool call in v2), post_model_hook, structured-response node, or END when no tool calls remain | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859 |
| Graceful budget exhaustion | `_are_more_steps_needed` checks `remaining_steps < 2`; `call_model` substitutes "Sorry, need more steps..." AIMessage instead of looping into the recursion ceiling | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634, 684-692 |
| Compile-time structural validation | `validate()`: unknown source/target nodes, missing entrypoint, branch targets checked against known nodes; unknown unconditional branch targets assume any-node reachability | libs/langgraph/langgraph/graph/state.py:1129-1175 |
| Runtime channel validation | `validate_graph`: reserved names, unreadable/unsubscribed channels, input channels must be subscribed by some node | libs/langgraph/langgraph/pregel/_validate.py:13-107 |
| Test: recursion limit raises | `test_pregel.py` asserts `GraphRecursionError` on `invoke(2, {"recursion_limit": 1})` for a 2-node cycle-free chain | libs/langgraph/tests/test_pregel.py:586-589 |
| Test: chained Send routing order | `test_send_sequences` verifies Send-of-Command chains execute across correct superstep boundaries with deterministic output ordering | libs/langgraph/tests/test_pregel.py:1220-1266 |
| Test: send dedupe on resume | `test_send_dedupe_on_resume` (sync + async) proves interrupted PUSH tasks are not double-executed after checkpoint resume | libs/langgraph/tests/test_large_cases.py:4645; libs/langgraph/tests/test_pregel_async.py:2530 |
| Test: nested subgraph sends | `test_send_to_nested_graphs` covers Send crossing subgraph namespaces | libs/langgraph/tests/test_large_cases.py:5750 |

## Answers to Dimension Questions

### 1. How are messages routed?

Routing is channel-based, not message-bus-based. Every node publishes state updates through a `ChannelWrite` attached at node construction (`attach_node`, libs/langgraph/langgraph/graph/state.py:1494-1538). Control-plane returns are decoded by `_control_branch` (libs/langgraph/langgraph/graph/state.py:1749-1775): a returned `Send` becomes a write to the reserved `TASKS` Topic channel (installed at libs/langgraph/langgraph/pregel/main.py:804-809), while a `Command(goto="node")` writes `None` into the target's auto-generated `branch_to:<node>` EphemeralValue channel (created per node at libs/langgraph/langgraph/graph/state.py:1525-1530). At the start of each superstep, `prepare_next_tasks` turns updated channels into tasks: PULL tasks fire when a node's subscribed trigger channels were updated (computed via the `trigger_to_nodes` inverted index, libs/langgraph/langgraph/pregel/_algo.py:475-486 and libs/langgraph/langgraph/pregel/main.py:4175-4181); PUSH tasks are created from each `Send` sitting in the `TASKS` topic (libs/langgraph/langgraph/pregel/_algo.py:441-466). Static edges are just unconditional writers appended to the source node (libs/langgraph/langgraph/graph/state.py:1551-1559). Channel writes targeting `TASKS` directly are blocked ("Cannot write to the reserved channel TASKS", libs/langgraph/langgraph/pregel/_write.py:114-116).

### 2. How is the next speaker selected?

There is no central speaker-selection policy. Selection is fully decentralized and declarative:

- **Static edges**: fixed successor(s).
- **Conditional edges**: a user-supplied router callable (`BranchSpec.path`, libs/langgraph/langgraph/graph/_branch.py:83-120) inspects current channel state and returns one/multiple node names or `Send` objects; results are validated against `None`/`START`/END-for-Send (libs/langgraph/langgraph/graph/_branch.py:207-210).
- **Dynamic dispatch**: any node can return `[Send("agent_b", {...}), Send("agent_c", {...})]` to invoke multiple downstream nodes in parallel next superstep, each with custom private state (libs/langgraph/langgraph/types.py:704-720).
- **Prebuilt reference policy**: the react agent's `should_continue` selects "tools" (as parallel per-tool-call `Send`s in v2), an optional `post_model_hook`, `generate_structured_response`, or `END` based purely on whether the last `AIMessage` has tool calls (libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859).

Because all writes land at the superstep boundary, concurrent selections are merged deterministically by channel reducers rather than resolved by a selector.

### 3. How are handoffs managed?

There is no dedicated handoff API in this source (the legacy `/how-tos/agent-handoffs` doc now redirects externally, libs/langgraph/docs/redirects.json:44). Handoffs are composed from public primitives: a node returns `Command(goto="other_agent", update={...})` to transfer control plus state in one atomic value (libs/langgraph/langgraph/types.py:799-824). Cross-graph handoff is supported via `Command(graph=Command.PARENT)`, which propagates upward as a `ParentCommand` bubble-up exception instead of being applied locally (libs/langgraph/langgraph/graph/state.py:1761-1762; libs/langgraph/langgraph/errors.py:129). Subgraphs receiving a `Send` get their own checkpoint namespace derived from the packet's node path (libs/langgraph/langgraph/pregel/_algo.py:986-997), so a handoff into a subgraph remains resumable. The contract is convention-based: nothing in the framework validates that a "handing off" agent declared the target or that the receiver expects the payload — beyond target-existence and type checks noted above.

### 4. When does a group conversation terminate?

Four terminal states exist in `PregelLoop.tick()` (libs/langgraph/langgraph/pregel/_loop.py:599-681):

1. **Natural termination (`done`)**: `prepare_next_tasks` produces zero tasks — no trigger channels updated and no pending Sends (libs/langgraph/langgraph/pregel/_loop.py:652-655).
2. **Budget exhaustion (`out_of_steps`)**: `step > stop` where `stop = step + recursion_limit + 1` (libs/langgraph/langgraph/pregel/_loop.py:606-609, 1700-1701); converted to `GraphRecursionError` after the stream drains (libs/langgraph/langgraph/pregel/main.py:3002-3011).
3. **Cooperative drain (`draining`)**: external `RunControl.request_drain()` (e.g., SIGTERM handling) stops at the next superstep boundary, saves the checkpoint, raises `GraphDrained` (libs/langgraph/langgraph/pregel/_loop.py:657-659; libs/langgraph/langgraph/errors.py:54-64).
4. **Human-in-the-loop interrupts**: `interrupt_before`/`interrupt_after` raise `GraphInterrupt` for checkpointed pause/resume (libs/langgraph/langgraph/pregel/_loop.py:666-671, 719-724).

The prebuilt agent adds a softer fifth option: when `remaining_steps < 2` and tool calls persist, it injects a final "Sorry, need more steps" message and lets routing fall through to END (libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634, 684-692).

So yes — a multi-agent conversation terminates without human intervention either by reaching quiescence or by hitting the enforced recursion bound.

### 5. Is deadlock possible?

Classical deadlock (circular wait) is structurally impossible: execution is Bulk Synchronous Parallel — tasks within a superstep run against immutable channel snapshots, updates apply atomically at the boundary (documented invariant, libs/langgraph/langgraph/pregel/main.py:2959-2963), and no task blocks waiting on another task mid-superstep; each task runs to completion under a per-step timeout (libs/langgraph/langgraph/pregel/main.py:2967-2972). Join barriers (`NamedBarrierValue`) can only defer a target node, never block the engine: if some producer never writes, the barrier simply stays unavailable, the task set eventually empties, and the loop ends cleanly via the `done` path (libs/langgraph/langgraph/channels/named_barrier_value.py:74-75; libs/langgraph/langgraph/pregel/_loop.py:652-655). The real residual risk is **livelock**: two agents routing to each other forever. This is bounded, not prevented — the run fails with `GraphRecursionError` at the configured limit (default 10007, overridable via `LANGGRAPH_DEFAULT_RECURSION_LIMIT`, libs/langgraph/langgraph/_internal/_config.py:32). Two soft spots: misaddressed `Send` packets are silently dropped with a warning (libs/langgraph/langgraph/pregel/_algo.py:977-979), which can cause premature "done" termination that is hard to diagnose; and barrier channels raise `InvalidUpdateError` on out-of-contract values (libs/langgraph/langgraph/channels/named_barrier_value.py:63-66), converting protocol violations into loud failures rather than hangs.

## Architectural Decisions

1. **BSP/Pregel execution over actor-style messaging** — messages are channel writes visible only at superstep boundaries (libs/langgraph/langgraph/pregel/main.py:2959-2963). This buys determinism and checkpointability at the cost of one-step latency between agents.
2. **Reserved control channel for dynamic dispatch** — `Send` packets travel over a dedicated non-accumulating `Topic(Send, accumulate=False)` under the reserved `TASKS` name, cleared each step (libs/langgraph/langgraph/pregel/main.py:804-809; libs/langgraph/langgraph/channels/topic.py:77-85), preventing stale dispatches from re-firing after resume.
3. **Commands unify state update + navigation** — a single return value expresses both, avoiding split-brain updates where routing and state diverge (libs/langgraph/langgraph/types.py:799-824).
4. **Termination as a config-enforced invariant, not a heuristic** — the step bound is computed once per run/resume from checkpoint metadata (libs/langgraph/langgraph/pregel/_loop.py:1700-1701) and validated ≥1 (libs/langgraph/langgraph/pregel/main.py:2563-2564).
5. **Managed values for step-budget introspection** — `RemainingSteps` is injected into agent state without polluting user schemas (libs/langgraph/langgraph/managed/is_last_step.py:18-21), letting prebuilt agents degrade gracefully before the hard error.
6. **Handoffs left to conventions built on `Command`** — no first-class handoff contract; flexibility prioritized over enforcement (evidence absence noted in Q3).

## Notable Patterns

- **Router-as-data (`BranchSpec`)**: routing logic is a serialized spec (callable + optional literal-derived path map), enabling graph drawing and schema inference from type hints (libs/langgraph/langgraph/graph/_branch.py:88-120).
- **Trigger-channel indirection**: every potential edge target owns an ephemeral `branch_to:<name>` channel; routing = writing to that channel, which decouples routers from scheduler internals (libs/langgraph/langgraph/graph/state.py:1525-1532).
- **Inverted-index fast path**: `trigger_to_nodes` avoids scanning all nodes when only a few channels changed (libs/langgraph/langgraph/pregel/_algo.py:468-486).
- **Barrier joins with deferred variant**: plain `NamedBarrierValue` for standard fan-in; `NamedBarrierValueAfterFinish` for `defer=True` nodes so late joiners don't fire early (libs/langgraph/langgraph/graph/state.py:1560-1575; libs/langgraph/langgraph/channels/named_barrier_value.py:162-167).
- **Graceful degradation before hard limits**: layered defense of managed `RemainingSteps` → canned final AI message → `GraphRecursionError` (libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-692; libs/langgraph/langgraph/pregel/main.py:3002-3011).

## Tradeoffs

- **Determinism vs. conversational immediacy**: an agent's output is not visible to concurrently scheduled agents until the next superstep; tight dialogue loops pay a synchronization step each turn.
- **Silent Send-drop vs. fail-fast**: dropping unknown-target Sends (libs/langgraph/langgraph/pregel/_algo.py:977-979) keeps long-running map-reduce jobs resilient but can mask routing bugs as early termination.
- **Convention-based handoffs**: maximum composability, but no schema/type checking of transferred payloads between agents; correctness rests on application tests.
- **Recursion-limit-as-livelock-guard**: simple and robust, but the failure mode is an exception after wasted work (up to 10007 steps by default) rather than early cycle detection at compile time.
- **Ephemeral routing channels**: unexecuted branch writes vanish per step (guard=False, libs/langgraph/langgraph/graph/state.py:1526-1530), keeping checkpoints lean, at the cost that routing intent is only reconstructible from task metadata, not channel history.

## Failure Modes / Edge Cases

- Branch returning `None`/`START` or a `Send` to END raises immediately (`ValueError` / `InvalidUpdateError`, libs/langgraph/langgraph/graph/_branch.py:207-210).
- `recursion_limit < 1` rejected before the run starts (libs/langgraph/langgraph/pregel/main.py:2563-2564).
- Interrupted runs recompute `stop` from saved step metadata so resume does not reset the budget (libs/langgraph/langgraph/pregel/_loop.py:1700-1701); interrupted PUSH tasks are deduped on resume (libs/langgraph/tests/test_large_cases.py:4645).
- Untracked values nested inside `Send.arg` are sanitized before checkpointing (libs/langgraph/langgraph/pregel/_algo.py:1443-1460; libs/langgraph/langgraph/pregel/_loop.py:445-448).
- Unhandled task exceptions bubble per-task with retry policies; the runner stops scheduling new tasks once failures exceed handled set (libs/langgraph/langgraph/pregel/_runner.py:616+).
- Drain requests are honored only between supersteps; a mid-superstep SIGTERM waits for task completion (libs/langgraph/langgraph/pregel/_loop.py:657-659).

## Future Considerations

- Add optional strict mode making unknown `Send` targets a hard error (config-gated), closing the silent-drop gap.
- Provide a first-class handoff descriptor (declared source/target pairs with payload schemas) to give static validation parity with edges while preserving `Command` flexibility.
- Consider cheap compile-time cycle detection that warns (not errors) when a strongly-connected component lacks any END-reaching path, complementing the runtime recursion bound.
- Surface `draining` status and remaining-step counts as stream events for better observability of near-limit runs.

## Questions / Gaps

- No in-repo documentation of handoff patterns survives (docs externalized; only redirect found at libs/langgraph/docs/redirects.json:44) — handoff semantics were assessed purely from implementation, per the implementation-over-README rule.
- No evidence found of a framework-level speaker-selection strategy (e.g., round-robin, LLM-selected next speaker as in AutoGen-style group chat); search covered `libs/langgraph/langgraph/` (graph/, pregel/, channels/, managed/) and `libs/prebuilt/`. LangGraph deliberately delegates this to graph authors.
- No dedicated deadlock-detector module found (searches for `deadlock`, barrier timeout options); safety derives from the BSP model itself rather than detection code.

---

Generated by dimension 15.02 (Message Routing and Termination) against `langgraph`.
