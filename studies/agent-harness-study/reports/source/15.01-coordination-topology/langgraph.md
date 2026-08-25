# Source Analysis: langgraph

## Dimension 15.01: Coordination Topology

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core + prebuilt), TypeScript (sdk-js); monorepo with `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint*`, `libs/sdk-py`, `libs/sdk-js` |
| Analyzed | 2026-08-25 |

## Summary

LangGraph does not impose a single multi-agent coordination topology; it ships a general-purpose coordination substrate and lets users compose supervisor, network, hierarchical, or map-reduce topologies on top of it. The substrate is a **Pregel / Bulk Synchronous Parallel (BSP) engine**: agents ("actors", implemented as `PregelNode`) communicate exclusively through typed **channels** (`LastValue`, `Topic`, `BinaryOperatorAggregate`, `NamedBarrierValue`, `EphemeralValue`), and execution proceeds in synchronized supersteps of plan → parallel execute → update (`libs/langgraph/langgraph/pregel/main.py:454-477`). The graph API (`StateGraph`) compiles declarative edges/branches into channel subscriptions and writers (`libs/langgraph/langgraph/graph/state.py:1525-1575`). Topology is **static per compiled graph** (node registry fixed at compile time), but **activation is dynamic**: conditional edges, `Send` (dynamic fan-out to named nodes with per-task custom state, `libs/langgraph/langgraph/types.py:704-719`), and `Command(goto=...)` (runtime re-routing, including cross-graph handoff via `Command(graph=Command.PARENT)`, `libs/langgraph/langgraph/types.py:798-848`) are resolved at every superstep. Coordination is **centralized** in one engine loop that plans tasks (`prepare_next_tasks`, the union of PUSH/Send tasks and PULL/edge-triggered tasks — `libs/langgraph/langgraph/pregel/_algo.py:392-513`) and commits writes (`apply_writes`, `libs/langgraph/langgraph/pregel/_algo.py:232-345`); "agent discovery" reduces to compile-time name registration plus runtime name lookup of Send targets, with no service-discovery mechanism. Supervisor-style multi-agent patterns are documented as usage patterns (e.g., "supervisor w/ tools" comment in `libs/prebuilt/langgraph/prebuilt/tool_node.py:981`; subgraph naming for multi-agent composition in `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:466-468`), not as first-class framework constructs.

**Topology drawing (one multi-agent run):**

```
                 ┌────────────────── Pregel engine (central BSP loop) ─────────────────┐
 input ─▶ [input channels]   Plan: prepare_next_tasks() (_algo.py:392)                │
                     │                                                                │
     PULL tasks      ▼            PUSH tasks (Send → __pregel_tasks Topic)            │
  ┌──────────── agent/supervisor node ──── returns [Send("researcher", s), ...] ──┐    │
  │        (PregelNode: triggers=[branch chans], writers=[ChannelWrite])          │    │
  │                                                                               ▼    │
  │   join:"a+b:c" NamedBarrierValue ◀── writes from a, b (fan-in barrier)   Execute   │
  │        │                                                                   n+1    │
  └────────┴───────────── researcher ×N run concurrently (PregelRunner._runner.py:135)│
                     │                                                                │
                  Update: apply_writes() commits all writes atomically per step       │
                  (_algo.py:232) ──▶ next superstep or stop (recursion_limit guard)   │
                 └────────────────────────────────────────────────────────────────────┘
```

## Rating

**9 / 10** — Clear, explicit, and durable coordination model with an unusually strong test and safeguard story:

- *Clear model*: the BSP contract (plan/execute/update, invisible intra-step writes) is spelled out in the engine docstring (`libs/langgraph/langgraph/pregel/main.py:464-477`) and enforced by code (`apply_writes` runs only after all tasks finish, `_algo.py:232-345`).
- *Explicit interfaces*: every communication semantic is a concrete channel class with typed `ValueType`/`UpdateType` contracts (`libs/langgraph/langgraph/channels/base.py:28-36`); dynamic routing has dedicated primitives (`Send`, `Command`).
- *Tests*: 164 tests in `test_pregel.py` alone (fan-out/fan-in, sends, concurrency), mirrored async suites, plus dedicated channel tests (`libs/langgraph/tests/test_channels.py:33-110`), parent-command handoff tests (`libs/langgraph/tests/test_parent_command.py:9-53`).
- *Operational safeguards*: recursion limit raising `GraphRecursionError` (`libs/langgraph/langgraph/pregel/main.py:3005-3011`), conflict detection on concurrent single-writer channel updates (`last_value.py:56-64`, error code `INVALID_CONCURRENT_GRAPH_UPDATE` in `errors.py:36`), retry policies and timeout policies per node (`_read.py:122-133`).

Not a 10 because: there is no runtime peer discovery (addressing is by static node name only — unknown Send targets are silently dropped, `_algo.py:977-979`); write conflicts surface as hard runtime errors rather than configurable resolution; and cross-process coordination is delegated to an out-of-repo server via `RemoteGraph`.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Communication model | Engine docstring: actors read/write channels; execution organized as BSP steps with plan/execute/update phases | `libs/langgraph/langgraph/pregel/main.py:454-477` |
| Actor definition | "An actor is a `PregelNode`. It subscribes to channels, reads data from them, and writes data to them" | `libs/langgraph/langgraph/pregel/main.py:481-483` |
| Channel taxonomy | Built-in `LastValue` (default, single value/step) and `Topic` (PubSub, multiple values between actors) described | `libs/langgraph/langgraph/pregel/main.py:496-502` |
| Channel base contract | Abstract `BaseChannel` with `get/update/consume/finish` protocol; update order within a step declared arbitrary | `libs/langgraph/langgraph/channels/base.py:69-99` |
| Single-writer conflict rule | `LastValue.update` rejects >1 value per step with `INVALID_CONCURRENT_GRAPH_UPDATE` | `libs/langgraph/langgraph/channels/last_value.py:56-64` |
| PubSub channel | `Topic`: flattened fan-in, optional accumulation across steps | `libs/langgraph/langgraph/channels/topic.py:23-31`, `topic.py:77-85` |
| Aggregate/reducer channel | `BinaryOperatorAggregate(int, operator.add)` folds concurrent updates into one value | `libs/langgraph/langgraph/channels/binop.py:65-73` |
| Barrier/join channel | `NamedBarrierValue` waits until ALL named producers have written before releasing readers (fan-in join semantics) | `libs/langgraph/langgraph/channels/named_barrier_value.py:13-14`, `named_barrier_value.py:56-81` |
| Ephemeral routing channels | Branch-to-node channels are `EphemeralValue` (or `LastValueAfterFinish` for deferred nodes), cleared after each step | `libs/langgraph/langgraph/graph/state.py:1525-1530` |
| Node compilation | Each node becomes `PregelNode(triggers=[branch_channel], channels=state keys, writers=[ChannelWrite])` | `libs/langgraph/langgraph/graph/state.py:1531-1547` |
| Static edge → writer | `attach_edge(start, end)` appends a `ChannelWrite` to the source node targeting the branch channel | `libs/langgraph/langgraph/graph/state.py:1551-1559` |
| Join edge → barrier | Multi-source `add_edge([a,b], c)` creates `join:a+b:c` `NamedBarrierValue`, end node subscribes as trigger, sources write their names | `libs/langgraph/langgraph/graph/state.py:1560-1575` |
| Waiting-edge declaration | `add_edge(list_of_starts, end)` documents "wait for ALL of the start nodes to complete" | `libs/langgraph/langgraph/graph/state.py:928-980` |
| Conditional edges | `add_conditional_edges(source, path, path_map)` registers a branch callable evaluated at runtime | `libs/langgraph/langgraph/graph/state.py:982-1030` |
| Dynamic fan-out primitive | `Send(node, arg)`: "dynamically invoke a node ... with a custom state"; map-reduce example | `libs/langgraph/langgraph/types.py:704-719`, `types.py:723-748` |
| Dynamic rerouting primitive | `Command(goto=..., update=...)` with `graph=None \| Command.PARENT` | `libs/langgraph/langgraph/types.py:798-824`, `types.py:848` |
| Command → channel writes | `_control_branch` converts `goto` into `(TASKS, send)` or branch-channel writes; `Command.PARENT` raises `ParentCommand` to bubble up | `libs/langgraph/langgraph/graph/state.py:1749-1775` |
| Reserved TASKS channel | Pregel init installs `channels[TASKS] = Topic(Send, accumulate=False)`; user graphs may not reuse the key | `libs/langgraph/langgraph/pregel/main.py:804-809` |
| Send transport | `ChannelWrite._assemble_writes` maps any `Send` to a `(TASKS, send)` tuple | `libs/langgraph/langgraph/pregel/_write.py:172-189` |
| Task planning (topology resolution) | `prepare_next_tasks` returns "union of all PUSH tasks (Sends) and PULL tasks (nodes triggered by edges)"; uses `trigger_to_nodes` index for O(updated-channels) activation | `libs/langgraph/langgraph/pregel/_algo.py:432-435`, `_algo.py:468-486` |
| Send execution timing | "SEND tasks, executed in superstep n+1"; unknown node names logged and ignored | `libs/langgraph/langgraph/pregel/_algo.py:962-979` |
| Write commit (barrier phase) | `apply_writes` groups writes by channel, applies after all tasks complete, bumps channel versions, notifies finish | `libs/langgraph/langgraph/pregel/_algo.py:232-345` |
| Concurrent task execution | `PregelRunner`: "executing a set of Pregel tasks concurrently, committing their writes" | `libs/langgraph/langgraph/pregel/_runner.py:135-138` |
| Recursion safeguard | Run stops at `step + recursion_limit`; `out_of_steps` status raises `GraphRecursionError` with remediation hint | `libs/langgraph/langgraph/pregel/_loop.py:1701`, `main.py:3001-3011`, `libs/langgraph/langgraph/errors.py:67-71` |
| Cross-process topology | `RemoteGraph` client "can be used directly as a node in another Graph" (coordination across deployments via LangGraph Server API) | `libs/langgraph/langgraph/pregel/remote.py:118-127` |
| Subgraph-as-agent | `create_react_agent(name=...)` name "automatically used when adding ReAct agent graph to another graph as a subgraph node - particularly useful for building multi-agent systems" | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:466-468` |
| Supervisor pattern acknowledgment | ToolNode interrupt handling references "(2 and 3 can happen in a 'supervisor w/ tools' multi-agent architecture)" | `libs/prebuilt/langgraph/prebuilt/tool_node.py:973-982` |
| Handoff test | `test_parent_command_from_nested_subgraph`: child node returns `Command(graph=Command.PARENT, goto="parent_second")` and control jumps to parent's node | `libs/langgraph/tests/test_parent_command.py:9-53` |
| Fan-out/fan-in tests | `test_in_one_fan_out_state_graph_waiting_edge*` family exercises barrier joins incl. multiple & conditional variants | `libs/langgraph/tests/test_pregel.py:1953`, `:2364`, `:2808`, `:2975` |
| Send tests | `test_cond_edge_after_send`, `test_concurrent_emit_sends`, `test_send_sequences` | `libs/langgraph/tests/test_pregel.py:1147`, `:1173`, `:1220` |
| Concurrency safety test | `test_concurrent_execution_thread_safety` | `libs/langgraph/tests/test_pregel.py:5334` |
| Channel unit tests | `test_last_value`, `test_topic`, `test_topic_accumulate`, `test_binop` pin down each communication semantic | `libs/langgraph/tests/test_channels.py:33-110` |

## Answers to Dimension Questions

**1. How do agents coordinate?**
Agents never message each other directly. Every interaction is mediated by channels: a node executes, its `writers` (a `ChannelWrite` runnable appended to the node, `libs/langgraph/langgraph/graph/state.py:1531-1538`) emit `(channel, value)` tuples, and after the whole superstep completes, `apply_writes` folds those values into channels using each channel's reducer semantics (overwrite for `LastValue` — which errors on two concurrent writers; append-flatten for `Topic`; binary-op fold for `BinaryOperatorAggregate`; all-names-seen gate for `NamedBarrierValue`). Downstream nodes fire in the next superstep if any of their `triggers` channels became available (`_algo.py:468-512`). Control-plane coordination (who runs next, with what private state) is expressed through `Send` packets routed over the reserved `__pregel_tasks` topic (`main.py:804-809`, `_write.py:178-179`) and `Command(goto=...)` rewritten into ephemeral branch-channel writes (`state.py:1764-1774`).

**2. Is the topology fixed or dynamic?**
Both, by layer. The *set of nodes and their channel subscriptions/writers is frozen at compile time* — `StateGraph.compile()` builds the `nodes`/`channels` tables and validates them (`validate_graph` in `libs/langgraph/langgraph/pregel/_validate.py:13`). Within that static wiring, *activation and addressing are fully dynamic*: conditional edges choose destinations per step (`state.py:982-1030`), `Send` instantiates unbounded parallel task instances against any registered node with arbitrary per-instance state (`types.py:704-719`), and `Command(goto=...)` reroutes control mid-run (`state.py:1749-1775`). New nodes cannot be added at runtime; unknown `Send` targets are dropped with a warning (`_algo.py:977-979`), so dynamic addressing is bounded by the static registry.

**3. Is there a single point of failure?**
Yes, deliberately. A single `Pregel` loop owns planning, scheduling, and write application (`main.py:450-477`; runner at `_runner.py:135`). If the loop crashes, in-flight tasks are lost unless a checkpointer is attached; durability is therefore externalized to checkpointers (`libs/checkpoint`, e.g. sqlite/postgres implementations) and resumability via interrupts. Within a superstep, one failing task can stop siblings (`_should_stop_others` wired into the futures dict, `_runner.py:190-197`), mitigated by per-node `retry_policy`/timeout (`_read.py:122-133`, `state.py:695-722`) and error-handler nodes (`error_handler_node`, `_read.py:144-148`; routing logic `_runner.py:171-174`). This centralization buys deterministic, checkpointable semantics — a reasonable trade for an orchestration library — but the engine loop and its checkpointer are the failure domain.

**4. Can agents discover each other?**
Only through the static node-name registry. `Send("node_name", arg)` resolves against the `processes` mapping at task-preparation time (`_algo.py:977-981`); `Command(goto="node")` likewise addresses compile-time names. There is no capability-based discovery, no broker, and no runtime membership protocol. Cross-hierarchy visibility exists solely as the explicit `Command(graph=Command.PARENT)` escape hatch that raises `ParentCommand` to the enclosing graph (`state.py:1761-1762`; tested in `test_parent_command.py:18-21`), and cross-deployment reach exists via `RemoteGraph`, where an assistant_id/url substitutes for discovery (`remote.py:132-163`). No evidence found of any richer discovery mechanism; searches for "handoff"/"subagents" APIs inside `libs/prebuilt` returned nothing beyond doc comments (the supervisor/handoff toolkits live in separate packages outside this repo).

## Architectural Decisions

1. **Adopt Pregel/BSP instead of free-form message passing.** Synchronized supersteps with invisible intra-step writes (`main.py:461-473`, enforced by post-step `apply_writes`) eliminate interleaving nondeterminism and make every step checkpointable. Cost: latency floor of one barrier per hop and no streaming of partial results between peers.
2. **Make dataflow semantics pluggable via channel classes.** Conflict policy is a property of the channel, not the engine: strict single-writer (`last_value.py:56-64`), PubSub (`topic.py`), CRDT-ish fold (`binop.py`), join barriers (`named_barrier_value.py`). This lets the same engine express both blackboard-style shared state and point-to-point routing.
3. **Separate static wiring from dynamic activation.** Compile-time graph validation plus runtime `Send`/`Command` gives map-reduce and adaptive routing without sacrificing the ability to statically draw/validate the graph (`destinations` metadata exists purely for rendering, `state.py:704-714`; `draw_graph` in `pregel/_draw.py:42`).
4. **Route dynamic control through a reserved channel rather than special-casing it.** `Send`s are ordinary writes to the `TASKS` topic (`_write.py:178-179`), so they checkpoint, replay, and version exactly like data — dynamic topology survives resume.
5. **Keep multi-agent opinionated patterns out of core.** Supervisor/swarm recipes are comments and docs (`tool_node.py:981`), while reusable machinery is generic (subgraphs as nodes, `chat_agent_executor.py:466-468`). This keeps the kernel small but pushes topology correctness onto library users.
6. **Centralize failure policy at the node boundary.** Retry, timeout, cache, and error-handler configuration attach to `PregelNode` construction (`_read.py:122-148`, `state.py:695-722`) instead of ad-hoc try/except in agent code.

## Notable Patterns

- **Blackboard + scheduler hybrid**: shared state channels act as a structured blackboard; the engine is the scheduler deciding which watchers fire — closer to a typed blackboard than pure actor messaging.
- **Join-barrier fan-in**: multi-parent edges compile to `NamedBarrierValue` channels whose `consume()` resets the seen-set for cyclic graphs (`named_barrier_value.py:77-81`), enabling repeated synchronization rounds.
- **Push/PULL task duality**: one planner emits both edge-triggered ("PULL") and Send-created ("PUSH") tasks, distinguished only by trigger path (`_algo.py:442-512`), keeping conditional fan-out and static edges uniform.
- **Deterministic ordering under concurrency**: writes are sorted by task path before application (`_algo.py:253-256`), and channel `update` explicitly tolerates arbitrary arrival order — determinism is achieved by sorting, not by scheduling luck.
- **Hierarchical namespaces for nested coordination**: subgraph task ids/namespaces embed parent namespace (`f"{parent_ns}{NS_SEP}{packet.node}"`, `_algo.py:987-989`), giving every agent-in-team a unique checkpoint address.

## Tradeoffs

- **Latency vs determinism**: every communication pays a full-superstep barrier; tightly coupled ping-pong dialogs between agents cost 2 hops minimum (`Send`s land in superstep n+1, `_algo.py:962`).
- **Safety vs expressiveness for shared state**: two parallel nodes writing one `LastValue` key is a hard crash (`last_value.py:59-64`), forcing developers to consciously pick reducers (`Annotated[list, operator.add]`) — verbose but prevents silent lost-update races.
- **Static registry vs flexibility**: dropping unknown Send targets (`_algo.py:977-979`) prevents typos from hanging a run but also means misconfigured dynamic topologies fail softly (silently fewer workers).
- **Centralized engine vs fault isolation**: single-loop design simplifies reasoning and checkpointing but concentrates failure; sibling cancellation on error (`_runner.py:190-197`) prioritizes consistency over partial progress.
- **Genericity vs guidance**: because supervisor/network topologies are user-assembled, the repo itself contains no executable reference implementation of them (example notebooks were moved out of this repo to the docs site — both files under `examples/multi_agent/` are redirect stubs).

## Failure Modes / Edge Cases

- **Concurrent-write conflict**: `InvalidUpdateError(INVALID_CONCURRENT_GRAPH_UPDATE)` when ≥2 tasks write a LastValue key in one superstep (`last_value.py:56-64`; code registered at `errors.py:36`).
- **Runaway loops**: bounded by `recursion_limit`; breach raises `GraphRecursionError` (`main.py:3001-3011`, `errors.py:67-71`), stop computed at `_loop.py:1701`.
- **Dangling Sends**: Sends to unknown nodes are warned-and-dropped (`_algo.py:972-979`); malformed packet types likewise skipped (`_algo.py:971-975`).
- **Reserved-key misuse**: user attempts to declare or write the `__pregel_tasks` channel are rejected (`main.py:804-806`, `_write.py:114-117`).
- **Barrier starvation**: a `NamedBarrierValue` stays unavailable until every named producer writes; a producer that exits without writing leaves subscribers permanently unfired — termination is then only guaranteed by the recursion limit (no evidence found of deadlock-specific diagnostics; searched `pregel/` for barrier-timeout handling, none present).
- **Mid-task failures**: handled via retry policies, timeouts (async-only safe cancellation noted at `state.py:715-722`), error-handler node routing (`_runner.py:171-234`), and checkpoint-resume/interrupt machinery (`interrupt()` in `types.py:851-871` requires a checkpointer).

## Future Considerations

- An explicit deadlock/starvation detector for barrier joins would complement the recursion-limit backstop (currently silent stalls rely on `recursion_limit`).
- Configurable conflict policy for LastValue (e.g., last-writer-wins option) would ease migration of loosely-coordinated multi-writer topologies that today must redesign reducers.
- First-class supervisor/handoff helpers remain in satellite packages; folding a minimal reference pattern into `prebuilt` would make the intended topology guidance executable within this repo.

## Questions / Gaps

- How does LangGraph Server mediate multi-process/multi-replica coordination (queueing, ownership of the BSP loop)? Out of scope here: only the client stub `RemoteGraph` (`remote.py:118`) and sdk-py integration fixtures exist in this repo (e.g., scripted supervisor/researcher in `libs/sdk-py/integration/graph/deep_agent.py:4-42`).
- No in-repo executable multi-agent tutorial remains: `examples/multi_agent/*.ipynb` are moved-to-docs stubs, so topology examples could only be assessed at the mechanism level, not end-to-end behavior level.
- Whether unknown-Send-target warnings are surfaced anywhere observable (metrics/UI) could not be determined from this source; only the logger.warning path was found (`_algo.py:977-979`).

---

Generated by `15.01-coordination-topology` against `langgraph`.
