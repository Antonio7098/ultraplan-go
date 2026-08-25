# Source Analysis: pydantic-ai

## 01.01 — Execution Model Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (>=3.10); graph runner built on `anyio` TaskGroup + memory object streams; UI/UI-event protocols over Starlette/FastAPI |
| Analyzed | 2026-07-02 |

## Summary

Pydantic AI's primary execution model is a **typed async graph / superstep model**, not a ReAct loop, queue worker, or streaming turn loop. The agent is internally compiled into a small, statically-typed finite state machine built with `pydantic_graph.GraphBuilder` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139), executed by a graph runner that drives an `anyio` task group over a memory object stream (pydantic_graph/pydantic_graph/graph_builder.py:403-825). The agent graph itself is a 4-node FSM — `UserPromptNode` → `ModelRequestNode` → `CallToolsNode` → (back to `ModelRequestNode` or `SetFinalResult`→`End`) — that cycles via explicit edge returns, not by an implicit while-loop.

The model is **explicit** (the graph is constructed in code, validated at build time, and rendered as a Mermaid state diagram; pydantic_ai_slim/pydantic_ai/_agent_graph.py:2139 calls `g.build(validate_graph_structure=False)`), **observable** (auto-instrumented spans on graph and node boundaries; pydantic_graph/pydantic_graph/graph_builder.py:247, pydantic_graph/pydantic_graph/graph_builder.py:883), **durable-friendly** (the same graph can be persisted, resumed, and run inside Temporal/DBOS/Prefect workflows; pydantic_graph/pydantic_graph/graph.py:261 `iter_from_persistence`, pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:301), and **dual-API** (a builder-based runner that supports steps, joins, forks, decisions, and parallelism coexists with a legacy `BaseNode`-based runner that is in the process of being deprecated; pydantic_graph/pydantic_graph/__init__.py:5-19).

Pydantic AI also layers a **cooperative capability middleware** ("capability") pipeline on top of the graph that wraps `wrap_run`, `before_node_run`/`wrap_node_run`/`after_node_run`/`on_node_run_error`, `wrap_model_request`, and `wrap_run_event_stream` around the graph run (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1550-1691, pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:501). This is not a separate execution model — it is a hook layer on the same graph — but it materially changes what advances execution (e.g. `wrap_run` can short-circuit the entire graph; pydantic_ai_slim/pydantic_ai/agent/__init__.py:1629-1632).

> One-sentence answer to "what advances execution?": **A graph runner that pulls the next `GraphTask` off an `anyio` TaskGroup fed by a memory object stream, where each task runs a typed node (or step/join/decision) and the node's return value is resolved against pre-built `Path` edges to schedule the next batch of tasks (pydantic_graph/pydantic_graph/graph_builder.py:646-825).**

## Rating

**9 / 10 — Mature, durable, observable, extensible, and proven under failure or scale.**

Justification (evidence-backed):

- **Explicit model with build-time validation.** The agent graph is built declaratively in `build_agent_graph` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139) and made executable by `g.build(...)`. Outgoing edges are inferred from each `BaseNode.run` return annotation (pydantic_graph/pydantic_graph/basenode.py:104-136); the legacy graph even has a `validate_edges` step (pydantic_graph/pydantic_graph/graph.py:105).
- **Dual-API coverage.** The builder-based API (v2) supports steps, joins (with reducers), forks (broadcast + map, including async-iterable map), decisions, transforms, and labels (pydantic_graph/pydantic_graph/graph_builder.py:1091-2278; pydantic_graph/pydantic_graph/step.py; pydantic_graph/pydantic_graph/join.py; pydantic_graph/pydantic_graph/decision.py; pydantic_graph/pydantic_graph/paths.py), with a parallel legacy `BaseNode` API that survives for backwards compatibility (pydantic_graph/pydantic_graph/__init__.py:5-19).
- **Tested at the graph level.** The graph runner has dedicated tests covering basic runs, history, persistence, error propagation, mermaid rendering, state, file persistence, and v1/v2 integration (tests/graph/test_graph.py:49-480, tests/graph/test_persistence.py, tests/graph/beta/test_v1_v2_integration.py). The agent layer is heavily VCR-tested through the public API (tests/test_agent.py).
- **Persistence + durability.** The graph can be snapshot per node via `BaseStatePersistence` (pydantic_graph/pydantic_graph/graph.py:18, pydantic_graph/pydantic_graph/graph.py:261 `iter_from_persistence`) and is the substrate Temporal/DBOS/Prefect durable wrappers sit on (pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:301-370, pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:305, pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_agent.py:178).
- **Operational safeguards.** Cooperative `wrap_run` short-circuit detection, `cancel_and_drain` cleanup, original-exception preservation through `__aexit__` chains, `_node_error` capture before context manager transformation (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1550-1691, pydantic_ai_slim/pydantic_ai/run.py:101-103, pydantic_ai_slim/pydantic_ai/_utils.py reference), and anyio `CancelScope` per task (pydantic_graph/pydantic_graph/graph_builder.py:840-859) are evidence of failure handling.
- **Observable.** Auto-spans on `run graph` and `run node` (pydantic_graph/pydantic_graph/graph.py:246-247, pydantic_graph/pydantic_graph/graph_builder.py:883), traceparent propagation through `GraphRunContext` and `_GraphIterator` (pydantic_graph/pydantic_graph/graph.py:250, pydantic_graph/pydantic_graph/graph_builder.py:468), and an OTel-aware `wrap_model_request` capability (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1314-1331).

Deductions: the graph has TWO APIs (legacy + builder) being run side-by-side with an active deprecation path; that adds cognitive load for new contributors and is documented as transitional (pydantic_graph/pydantic_graph/__init__.py:11-19). The graph runner's "main loop" lives in a 180-line `iter_graph` generator (pydantic_graph/pydantic_graph/graph_builder.py:646-825) and is rated `# noqa: C901` — the complexity is real.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent graph entry point (graph construction) | `build_agent_graph` builds a 4-node graph via `GraphBuilder` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139` |
| Agent graph wiring (explicit edges) | `g.add(edge_from(start_node).to(UserPromptNode), g.node(UserPromptNode), g.node(ModelRequestNode), g.node(CallToolsNode), g.node(SetFinalResult))` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2138` |
| Agent nodes (the loop) | `UserPromptNode.run` → `ModelRequestNode` \| `CallToolsNode` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:270-272` |
| Agent nodes (the model request) | `ModelRequestNode.run` → `CallToolsNode` \| `ModelRequestNode` (retry path) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:581-604` |
| Agent nodes (tool dispatch / output) | `CallToolsNode.run` → `ModelRequestNode` \| `End[FinalResult]` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1046-1079` |
| Agent nodes (terminal) | `SetFinalResult.run` → `End[FinalResult]` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1384-1393` |
| Agent state type | `GraphAgentState` dataclass (message_history, usage, run_step, etc.) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-180` |
| Agent deps type | `GraphAgentDeps` (model, tool_manager, capabilities, instrumentation) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:182-222` |
| Graph definition (builder API) | `Graph` dataclass holds nodes, edges, types | `pydantic_graph/pydantic_graph/graph_builder.py:165-200` |
| Graph definition (legacy `BaseNode` API) | `Graph` dataclass with `node_defs` | `pydantic_graph/pydantic_graph/graph.py:24-104` |
| Graph run entry (v2) | `Graph.run` → drives `iter` → `next` to `EndMarker` | `pydantic_graph/pydantic_graph/graph_builder.py:248-287` |
| Graph run entry (v1) | `Graph.run` → `iter` → `__anext__` to `End` | `pydantic_graph/pydantic_graph/graph.py:107-158` |
| Graph iterator (v2) — the main loop | `GraphRun.__init__` creates `_GraphIterator` + anyio task group | `pydantic_graph/pydantic_graph/graph_builder.py:403-487` |
| Graph iterator (v2) — the "superstep" | `iter_graph` generator dispatches tasks, awaits results from memory stream | `pydantic_graph/pydantic_graph/graph_builder.py:646-825` |
| Task type | `GraphTaskRequest` / `GraphTask` (node_id, inputs, fork_stack, task_id) | `pydantic_graph/pydantic_graph/graph_builder.py:363-400` |
| End marker | `EndMarker` (v2) / `End` (v1) signals completion | `pydantic_graph/pydantic_graph/graph_builder.py:112-131`, `pydantic_graph/pydantic_graph/basenode.py:143-167` |
| Error marker | `ErrorMarker` lets the caller recover by sending new tasks instead of raising | `pydantic_graph/pydantic_graph/graph_builder.py:134-143` |
| Per-task execution | `_run_tracked_task` wraps a `CancelScope`; errors go through a memory object stream | `pydantic_graph/pydantic_graph/graph_builder.py:840-863` |
| Node dispatch (v2) | `_run_task` matches node kind: `StartNode`, `Fork`, `Step`, `Join`, `Decision`, `EndNode` | `pydantic_graph/pydantic_graph/graph_builder.py:865-898` |
| Legacy node dispatch (v1) | `GraphRun.next` calls `node.run(ctx)` and snapshots result | `pydantic_graph/pydantic_graph/graph.py:664-753` |
| Step primitive | `Step` wraps a `StepFunction` (or `NodeStep` for `BaseNode` types) | `pydantic_graph/pydantic_graph/step.py:119-253` |
| Join primitive | `Join` + `ReducerFunction` aggregate parallel branch results | `pydantic_graph/pydantic_graph/join.py:150-216` |
| Fork primitive | `Fork` with `is_map` (sequence) and broadcast; async-iterable map | `pydantic_graph/pydantic_graph/node.py:60-95`, `pydantic_graph/pydantic_graph/graph_builder.py:992-1035` |
| Decision primitive | `Decision` routes by `isinstance` / `Literal` matching per branch | `pydantic_graph/pydantic_graph/decision.py:40-100`, `pydantic_graph/pydantic_graph/graph_builder.py:900-923` |
| Path / Edge representation | `Path`, `DestinationMarker`, `TransformMarker`, `LabelMarker` encode edge contents | `pydantic_graph/pydantic_graph/paths.py` |
| `BaseNode` ABC | Abstract `run(ctx) -> BaseNode \| End`; return annotation is read at build time | `pydantic_graph/pydantic_graph/basenode.py:37-136` |
| `GraphRunContext` | Carries shared `state` + `deps` to every node | `pydantic_graph/pydantic_graph/basenode.py:27-33` |
| Agent entry: `iter` (async CM) | `Agent.iter` builds graph + state + deps, calls `graph.iter(inputs=user_prompt_node, ...)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1053-1545` |
| Agent entry: `run` | `Agent.run` opens `iter`, drives via `agent_run.next(...)` until `End` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:302-449` |
| Agent entry: `run_stream` | Drives the same `iter` with a custom `_stream_and_advance` step that streams inside `wrap_node_run` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:653-900` |
| AgentRun (driver) | `AgentRun.next` → `_run_node_with_hooks` → `_wrap_and_advance` → `graph_run.next` | `pydantic_ai_slim/pydantic_ai/run.py:267-328` |
| Capability middleware (wrap_run) | `wrap_run` may short-circuit the graph entirely | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1550-1632` |
| Capability per-node hooks | `before_node_run`, `wrap_node_run`, `after_node_run`, `on_node_run_error` | `pydantic_ai_slim/pydantic_ai/run.py:267-310`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:501` |
| Manual iteration | `AgentRun.next_node` (next pending task) + `AgentRun.next(node)` (advance) | `pydantic_ai_slim/pydantic_ai/run.py:126-137, 330-404` |
| Persistence | `BaseStatePersistence` snapshot per node; `Graph.iter_from_persistence` resumes a run | `pydantic_graph/pydantic_graph/graph.py:261-309` |
| Durable exec (Temporal) | Subclasses `Agent` to wrap the same graph in a Temporal workflow | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:301-370` |
| Durable exec (DBOS) | Same pattern, different engine | `pydantic_ai_slim/pydantic_ai/durable_exec/dbos/_agent.py:305` |
| Durable exec (Prefect) | Same pattern, different engine | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_agent.py:178` |
| Mermaid rendering | `graph.mermaid_code` for legacy; `graph.render` for v2 — diagrams are first-class | `pydantic_graph/pydantic_graph/graph.py:336-399`, `pydantic_graph/pydantic_graph/graph_builder.py:336` |
| Tests: graph execution | `test_graph`, `test_graph_history`, `test_iter`, `test_iter_next`, `test_iter_next_error` | `tests/graph/test_graph.py:49-480` |
| Tests: graph persistence | `test_dump_load_state`, `test_record_lookup_error`, file-persistence tests | `tests/graph/test_persistence.py`, `tests/graph/test_file_persistence.py` |
| Tests: graph edge cases | fork/join/decision/mapping reducer behavior | `tests/graph/beta/test_graph_edge_cases.py` |
| Tests: v1/v2 interop | `test_v1_nodes_in_v2_graph`, `test_v2_step_to_v1_node`, `test_mixed_v1_v2_with_broadcast` | `tests/graph/beta/test_v1_v2_integration.py` |
| Agent docs (stated design goal) | "each Agent ... uses pydantic-graph to manage its execution flow ... finite state machines" | `docs/agent.md:291-295` |
| Graph docs (stated design goal) | "Graphs and finite state machines (FSMs) ... async graph and state machine library" | `docs/graph.md:15-19` |
| Builder-based graph API | `GraphBuilder` + `Step` + `Join` + `Decision` + `Fork` + `StartNode`/`EndNode` | `pydantic_graph/pydantic_graph/__init__.py:97-135` |
| Legacy graph API (deprecated at top level) | `Graph` (v1 runner), `BaseNode` etc. emit `PydanticGraphDeprecationWarning` from top-level | `pydantic_graph/pydantic_graph/__init__.py:11-19, 78-94` |

## Answers to Dimension Questions

1. **What is the primary execution model?**
   A **graph / superstep model**. A graph (`pydantic_graph.Graph`) holds a set of nodes and pre-built edges. The graph runner (`GraphRun` / `_GraphIterator`) maintains a set of active `GraphTask`s, dispatches them into an `anyio` `TaskGroup`, and consumes results from a memory object stream — yielding one batch of "next tasks" per superstep (pydantic_graph/pydantic_graph/graph_builder.py:646-825). When a node returns, the runner resolves its output against the prebuilt `Path` to compute the next batch (pydantic_graph/pydantic_graph/graph_builder.py:900-991). The model is essentially **BSP / superstep** for the agent: each "step" is a parallel batch of node executions whose results jointly determine the next batch.

2. **Is it explicit or emergent?**
   **Explicit.** The agent's graph is constructed in code by `build_agent_graph` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139) and made executable by `g.build(...)`. Edges are encoded declaratively (`g.add(...)`, `g.edge_from(...).to(...)`); outgoing edges from a node are inferred from its `run` return type annotation (pydantic_graph/pydantic_graph/basenode.py:104-136). The graph is rendered as a Mermaid state diagram (pydantic_graph/pydantic_graph/graph.py:336, pydantic_graph/pydantic_graph/graph_builder.py:336). The capability middleware can redirect the graph at runtime, but the base shape is statically known.

3. **Does the model match the product shape?**
   Yes. Pydantic AI's product is "give me an `Agent`, I call `run`/`iter`/`run_stream`, I get an `AgentRunResult`". The graph model lets the framework expose:
   - **Streaming** as a per-node `stream()` context (`ModelRequestNode.stream`, `CallToolsNode.stream`; pydantic_ai_slim/pydantic_ai/_agent_graph.py:606, 1081).
   - **Step-by-step iteration** as the natural `async for node in agent_run` (pydantic_ai_slim/pydantic_ai/run.py:189-233).
   - **Durable resume** as `iter_from_persistence` / Temporal/DBOS/Prefect subclasses that simply re-attach to the same `BaseStatePersistence`.
   - **Tool loop control** as explicit node returns (retry → `ModelRequestNode`; final → `End`; pydantic_ai_slim/pydantic_ai/_agent_graph.py:1046-1298).

4. **Is the model easy to explain to a new contributor?**
   Mostly yes. The "Agent is a tiny FSM" mental model is a one-liner: `UserPrompt → ModelRequest → CallTools → (loop or end)` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2138). The friction is the **two coexisting graph APIs** (legacy `BaseNode` runner + new `GraphBuilder` runner) and the in-progress deprecation (pydantic_graph/pydantic_graph/__init__.py:5-19, 78-94). New contributors writing their own graph can land on either depending on which doc they read. The new builder API is well-organized but has more concepts (steps, joins, forks, decisions, paths, transforms) that need to be learned together (pydantic_graph/pydantic_graph/graph_builder.py, pydantic_graph/pydantic_graph/decision.py, pydantic_graph/pydantic_graph/join.py, pydantic_graph/pydantic_graph/paths.py).

5. **Does the system mix models cleanly or accidentally?**
   **Cleanly, with one caveat.** Inside the agent, the model is *just* the graph — there is no competing event/queue/loop substrate. The capability layer is a hook layer on top of the graph (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1550-1691, pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:501), not a parallel execution model. The caveat is the **two graph APIs** (legacy `BaseNode` vs. builder-based) being run side-by-side (pydantic_graph/pydantic_graph/__init__.py:5-19). Inside a single run, only one is used (the agent uses the builder API; pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121-2139), so the in-run mix is clean. The cross-run mix is currently noisy and is being deliberately unwound in v2.

## Architectural Decisions

- **The agent loop is a graph, not a while-loop.** `UserPromptNode → ModelRequestNode → CallToolsNode → (loop to ModelRequestNode or terminate via SetFinalResult → End)` is the whole agent (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2138). The graph encodes what would otherwise be a hand-written state machine; turns are visible as nodes.

- **Graph runner uses an anyio `TaskGroup` + memory object stream as its scheduler** (pydantic_graph/pydantic_graph/graph_builder.py:457, 643, 838). This is what gives the model its "superstep" feel: a batch of `GraphTask`s is dispatched in parallel, the runner yields each batch's results back to the caller, and the caller can `asend` a replacement batch — see `GraphRun.override_next` (pydantic_graph/pydantic_graph/graph_builder.py:559-571).

- **Two coexisting graph APIs, with the builder one being the future.** Top-level imports of the legacy runner symbols emit a deprecation warning (pydantic_graph/pydantic_graph/__init__.py:78-94). `BaseNode`/`End`/`GraphRunContext`/`Edge` survive into v2 (pydantic_graph/pydantic_graph/__init__.py:19); the runner and persistence helpers do not. The agent has already migrated to the builder API (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121).

- **Node step types are an explicit union** (`StartNode`, `Step`/`NodeStep`, `Fork`, `Decision`, `Join`, `EndNode`) dispatched in a single `_run_task` switch (pydantic_graph/pydantic_graph/graph_builder.py:865-898) with `assert_never` exhaustiveness. Step vs. NodeStep distinguishes "v2 step function" from "v1 `BaseNode` reified as a step" (pydantic_graph/pydantic_graph/step.py:202-253), supporting the v1/v2 interop tested in `tests/graph/beta/test_v1_v2_integration.py`.

- **State is a single shared dataclass** (`GraphAgentState`, pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-180) passed by reference to every node. There is no per-node state isolation — the state object is the contract that ties nodes together. This matches the graph model: state flows with the graph run, not with individual nodes.

- **The capability middleware sits *above* the graph.** The graph doesn't know about capabilities; the run-loop wraps each node with `before_node_run`/`wrap_node_run`/`after_node_run`/`on_node_run_error` (pydantic_ai_slim/pydantic_ai/run.py:267-310), wraps the entire run with `wrap_run` (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1550-1632), and wraps model calls with `wrap_model_request` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:816-820). Capabilities can redirect the graph (e.g. `on_node_run_error` returning a different node; pydantic_ai_slim/pydantic_ai/run.py:298) but the underlying scheduler is still the graph.

- **Streaming is a node-local concern, not a model concern.** `ModelRequestNode.stream` and `CallToolsNode.stream` are `@asynccontextmanager` methods on the node itself (pydantic_ai_slim/pydantic_ai/_agent_graph.py:606-756, 1081-1091). The graph runs in the background; the caller pulls events from the node and then the runner advances. This is what makes `agent_run.next(SetFinalResult(...))` from inside `StreamedRunResult.on_complete` work — the caller, not the graph, decides to terminate (pydantic_ai_slim/pydantic_ai/agent/abstract.py:868).

- **Persistence is per-node, not per-step.** The legacy runner snapshots the node and state via `BaseStatePersistence` after each `next` (pydantic_graph/pydantic_graph/graph.py:738-748); `iter_from_persistence` resumes by replaying the next unrun snapshot (pydantic_graph/pydantic_graph/graph.py:261-309). This is what makes the durable-exec wrappers (Temporal/DBOS/Prefect) work: they wrap the same graph in an engine-specific durable step boundary.

- **Errors flow back as `ErrorMarker` (v2) rather than exceptions** (pydantic_graph/pydantic_graph/graph_builder.py:134-143, 664-672). This lets capability hooks (`on_node_run_error`) recover by sending replacement tasks without the runner swallowing the error. The legacy runner raises from `__anext__` and the graph layer re-raises (pydantic_graph/pydantic_graph/graph.py:758-767); this is a real difference between the two APIs.

## Notable Patterns

- **Builder + decoration pattern for graph construction** (pydantic_graph/pydantic_graph/graph_builder.py:1091-2278). `@g.step` decorates a function into a `Step`; `g.add(g.edge_from(...).to(...), g.node(...), ...)` registers nodes and edges. The fluent `g.edge_from(...).to(...)` returns a `Path` that can be further transformed (`to(...).map(...)`, `.label(...)`, `.decision(...)`) — see pydantic_graph/pydantic_graph/paths.py for the marker hierarchy.
- **Fork as a real parallel construct.** `_handle_fork_edges` iterates edges and creates one task per branch, with a `ForkStack` capturing the `ForkStackItem` lineage so downstream joins can correlate (pydantic_graph/pydantic_graph/graph_builder.py:992-1035, pydantic_graph/pydantic_graph/id_types.py).
- **Join as a reducer + bookkeeping.** `_GraphIterator.active_reducers` is keyed by `(JoinID, NodeRunID)` and re-enters a join only after all sibling tasks in that fork-run have finished (pydantic_graph/pydantic_graph/graph_builder.py:635, 942-959). Reducers can call `ctx.cancel_sibling_tasks()` to short-circuit the fork (pydantic_graph/pydantic_graph/join.py:73-78).
- **Async-iterable map (`Fork.is_map` with `AsyncIterable`)** yields GraphTask batches mid-stream (pydantic_graph/pydantic_graph/graph_builder.py:1017-1028). The graph doesn't materialize the iterable; each yielded item spawns its own task.
- **Path-based edge encoding.** Paths are lists of `DestinationMarker` / `TransformMarker` / `LabelMarker` / `MapMarker` / `BroadcastMarker` items (pydantic_graph/pydantic_graph/paths.py). Map/Broadcast markers are stripped during graph build; Transform and Label markers are interpreted at runtime by `_handle_path` (pydantic_graph/pydantic_graph/graph_builder.py:961-977). This is the v2 way of encoding the same information that v1 encoded via `Edge` annotations on `BaseNode.run` return types (pydantic_graph/pydantic_graph/basenode.py:175-180).
- **`_advance_graph` / `_wrap_and_advance` separation** in `AgentRun` keeps the graph step (no capability hooks) separable from the full hook lifecycle (pydantic_ai_slim/pydantic_ai/run.py:267-329). This is why bare `async for node in agent_run` warns when capabilities have `wrap_node_run` (pydantic_ai_slim/pydantic_ai/run.py:194-200).
- **Cooperative hand-off protocol** between `_do_run` and `wrap_run` using two `asyncio.Event`s (`_run_ready` / `_run_done`) (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1550-1600). Same pattern in `ModelRequestNode.stream` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:640-693). The cooperative hand-off is what makes `wrap_run` and `wrap_model_request` cancellable without orphaning tasks.
- **Capability composition via `CombinedCapability`** (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1373-1383) auto-flattens nested capabilities, which is what enables `wrap_run` short-circuit detection (`has_wrap_run_event_stream` etc.).
- **`@asynccontextmanager` for node streaming.** Both `ModelRequestNode.stream` and `CallToolsNode.stream` are async context managers (pydantic_ai_slim/pydantic_ai/_agent_graph.py:606, 1081); consumers `async with node.stream(ctx) as stream:` and then drain the stream. This pattern appears in `run_stream` and `iter` and is consistent across the codebase.

## Tradeoffs

- **Two graph APIs at once.** Building a `Graph` with `BaseNode` subclasses is well-trodden but flagged deprecated; building with `GraphBuilder` is newer and richer. Contributors who learn one first have to relearn the other. The cost of the transition is the current dual API (pydantic_graph/pydantic_graph/__init__.py:5-19, 78-94).
- **`iter_graph` is a 180-line generator with `# noqa: C901`** (pydantic_graph/pydantic_graph/graph_builder.py:646-825). The single function has to handle: start, fork/map (sync + async-iterable), step dispatch, join reduction, end, error recovery, and active-reducer finalization. The complexity is inherent to BSP-style execution but is concentrated in one function.
- **Capabilities can short-circuit the entire graph.** `wrap_run` is allowed to return a result without ever invoking the handler, and `Agent.iter` recognizes this by checking `agent_run.result is not None` after the `iter` CM exits (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1629-1632, 1638-1639). This is powerful but also means "did the graph run" is not always a well-defined question.
- **State is shared, not isolated.** `GraphAgentState` is a single dataclass shared by reference across every node (pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-180). This makes cross-node coordination easy but means a node that mutates state in an unexpected place is hard to reason about. The `_refresh_loaded_capability_ids` / `_refresh_discovered_tool_names` helpers explicitly mutate in place rather than reassign to preserve `RunContext` identity (pydantic_ai_slim/pydantic_ai/_agent_graph.py:1431-1456).
- **`anyio` + asyncio interop.** The v2 runner uses `anyio.TaskGroup` and `anyio` memory streams (pydantic_graph/pydantic_graph/graph_builder.py:41, 643, 838); pydantic-ai's agent layer uses `asyncio.Event` / `asyncio.create_task` (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1561, pydantic_ai_slim/pydantic_ai/run.py). They are bridged implicitly. This works on asyncio but is non-obvious for anyio trio users.
- **Validation is opt-in.** `build_agent_graph` calls `g.build(validate_graph_structure=False)` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2139) presumably because the agent graph is hand-built and known-valid. The legacy `Graph` constructor calls `_validate_edges` unconditionally (pydantic_graph/pydantic_graph/graph.py:105). The mismatch is intentional (the agent graph is internal) but worth flagging.

## Failure Modes / Edge Cases

- **Wrap-run short-circuit masking the original error.** If `wrap_run` doesn't recover, `_run_error` is set and may be re-raised after the `iter` CM (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1635-1691). Code reading `agent_run.result` after the CM must handle the case where the CM raised.
- **Bare iteration skipping capability hooks.** `async for node in agent_run:` uses the graph run's internal iteration and does *not* fire `wrap_node_run`/`after_node_run` (pydantic_ai_slim/pydantic_ai/run.py:194-200, 208-211). The runtime warns and `_anext__` raises `UndrainedPendingMessagesError` if the run ends with `pending_messages` enqueued via `enqueue` that needed `after_node_run` to drain (pydantic_ai_slim/pydantic_ai/run.py:221-232).
- **Context manager exception transformation.** `GraphRun → anyio TaskGroup` `__aexit__` chains can transform exceptions (e.g., `ExceptionGroup`); `_node_error` is captured before the chain runs so the original is preserved on the user side (pydantic_ai_slim/pydantic_ai/agent/__init__.py:1638-1639, pydantic_ai_slim/pydantic_ai/run.py:101-103, 218-219).
- **Per-task cancellation.** `_run_tracked_task` wraps each task in a `CancelScope` keyed by `task_id` (pydantic_graph/pydantic_graph/graph_builder.py:840-843); `_finish_task` cancels and removes it (pydantic_graph/pydantic_graph/graph_builder.py:827-832). On `EndMarker` the whole task group is cancelled (pydantic_graph/pydantic_graph/graph_builder.py:677-680).
- **Mid-stream errors preserve type via `BrokenResourceError` swallowing** (pydantic_graph/pydantic_graph/graph_builder.py:851-852, 861-863) but a node error is wrapped in `ErrorMarker` rather than raised, letting the caller recover via `override_next` (pydantic_graph/pydantic_graph/graph_builder.py:559-571, 661-667).
- **Reduction cancellation.** `cancel_sibling_tasks` from a reducer (`pydantic_graph/pydantic_graph/join.py:73-78`) cancels all tasks sharing a `ForkID + node_run_id` lineage, used for "first-match-wins" join strategies (`ReduceFirstValue`, pydantic_graph/pydantic_graph/join.py:140-147). Behavior under partial completion is non-trivial — see `tests/graph/beta/test_graph_edge_cases.py:238-303`.
- **In-flight `process_tool_calls` exceptions during enqueue drain.** Bare iteration can strand `pending_messages` if a `when_idle` enqueue happens late; the agent's `AgentRun` raises `UndrainedPendingMessagesError` rather than silently dropping (pydantic_ai_slim/pydantic_ai/run.py:226-231).
- **Reassignment of shared `loaded_capability_ids` / `discovered_tool_names` breaks in-step capability loads** because those sets are shared by reference into every `RunContext` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:206-213, 1444-1456). Documented as an invariant; mutation is the only correct way.
- **Streaming vs. capability hook ordering.** `run_stream` fires `before_node_run` *before* opening the stream but explicitly skips `wrap_node_run`/`after_node_run` for the streamed `ModelRequestNode` (pydantic_ai_slim/pydantic_ai/agent/abstract.py:786-880). The full lifecycle fires for `SetFinalResult` (the next node) but not for the streamed node. Easy to break; `run()` does it differently.
- **Durable exec model serialization.** Temporal's `Agent.run` serializes deps, message history, etc.; if `deps` isn't pydantic-serializable, a `PydanticSerializationError` is converted to a `UserError` (pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:292-296). This is engine-specific and must be repeated for every durable engine.

## Future Considerations

- **v2 graph runner is the path forward; v1 (`BaseNode` runner) is on the deprecation track** (pydantic_graph/pydantic_graph/__init__.py:5-19, 78-94). The `pydantic_graph.beta.*` namespace forwards with a deprecation warning (pydantic_graph/pydantic_graph/__init__.py:35-37). Once v2 lands, the legacy `pydantic_graph.graph.Graph` (runner) and `pydantic_graph.persistence` modules are slated for removal, and `Graph`/`mermaid` helpers are expected to split out of `graph_builder.py` (pydantic_graph/pydantic_graph/graph_builder.py:14-17).
- **`validate_graph_structure=False` for the agent graph** (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2139) could be turned on as a CI safety net once structure validation is faster / less strict.
- **Capability middleware is the right place to put new cross-cutting behavior.** The hook surface (`before_node_run`/`wrap_node_run`/`after_node_run`/`on_node_run_error`, `wrap_run`, `wrap_model_request`, `wrap_run_event_stream`) is well-defined; the natural place to add features like tracing, retries, persistence, or guardrails is a capability, not a graph change.
- **Streaming is a node concern.** New node types should expose a `stream()` async context manager if they have a streaming surface, mirroring `ModelRequestNode.stream` and `CallToolsNode.stream` (pydantic_ai_slim/pydantic_ai/_agent_graph.py:606, 1081).
- **The graph builder API is rich but unfamiliar to most Python developers.** Docs already call this out (docs/graph.md:3-13: "don't use a nail gun unless you need a nail gun"). Consider whether the agent's internal graph should be a public API for users wanting to build custom agent shapes; today `build_agent_graph` is a `_agent_graph` private helper, not part of the public surface.
- **`iter_graph` complexity** (pydantic_graph/pydantic_graph/graph_builder.py:646-825) is the single biggest concentration of control flow in the runtime. Splitting it into smaller helpers keyed by task outcome (normal, error, end, join) would improve testability and reduce the `# noqa: C901`.
- **The graph runner is the natural place to attach execution-mode plugins.** Today the runner is hard-coded to `anyio.TaskGroup`; an explicit "executor" interface (like LangGraph's checkpointer interface) would let durable exec plug in without subclassing `Agent`.

## Questions / Gaps

- The source includes **two graph runners** (legacy `pydantic_graph.graph.Graph` runner and builder-based `pydantic_graph.graph_builder.Graph` runner). Searched both, but the documentation and examples for the new builder API are spread across `docs/graph.md`, `docs/graph/builder/{steps,joins,decisions,parallel,index}.md` — could not enumerate every behavior with full coverage. Verified via grep that the agent uses the builder API (pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121).
- The "step + node interop" path (`NodeStep` reifying a v1 `BaseNode` as a v2 step) is exercised by `tests/graph/beta/test_v1_v2_integration.py` but I could not find documentation stating whether it is a public surface for users or an internal bridge during the deprecation period. Searched `docs/` and `pydantic_ai_slim/pydantic_ai/.agents/`; no clear statement found.
- The `Decision` routing logic has special-cased handling for `Any`/`object` (always matches) and `Literal` (membership test) before falling back to `isinstance` (pydantic_graph/pydantic_graph/graph_builder.py:908-918). Behavior under PEP 613 / `TypeAliasType` was not exhaustively searched.
- `iter_graph` has a comment "In this case, there are no pending tasks. We can therefore finalize all active reducers that don't have intermediate joins which are also active reducers." (pydantic_graph/pydantic_graph/graph_builder.py:749-808). This reducer-finalization logic is covered by tests (`tests/graph/beta/test_graph_edge_cases.py:238-383`) but the algorithm is non-obvious. A spec or invariant doc would help.
- The `PydanticGraphDeprecationWarning` machinery is in place (pydantic_graph/pydantic_graph/__init__.py:78-94) but the v2 migration timeline is not stated in any doc I found. No clear evidence found for a removal date.
- Whether `validate_graph_structure=False` is a perf concern or a correctness concern (i.e., would validation actually catch a real bug for the agent graph?) was not investigated end-to-end.

---

Generated by `01.01-execution-model-taxonomy` against `pydantic-ai`.
