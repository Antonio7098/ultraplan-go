# Source Analysis: pydantic-ai

## 06.02 — Task Decomposition Representation

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ (`pydantic_ai_slim` + `pydantic_graph` + `pydantic_evals` — uv workspace; `pydantic_graph` built on `anyio` TaskGroup + memory object streams) |
| Analyzed | 2026-08-27 |

## Summary

Pydantic AI core has **no first-party todo/plan/task decomposition schema**. Decomposition in the studied source exists at two separate layers that do not meet:

1. **Agent graph fixed FSM** — `build_agent_graph` in `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2574` builds a 4-node `GraphBuilder` FSM (`UserPromptNode` → `ModelRequestNode` → `CallToolsNode` → `SetFinalResult`/`End`) with `auto_instrument=False` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2585`. This is a static execution loop, not a dynamic plan.

2. **Generic graph decomposition engine** — `pydantic_graph/pydantic_graph/graph_builder.py:1139` provides `GraphBuilder`/`Graph`/`GraphRun`/`_GraphIterator` with typed nodes (`BaseNode` at `pydantic_graph/pydantic_graph/basenode.py:33`, `Step` at `pydantic_graph/pydantic_graph/step.py:120`, `Fork` at `pydantic_graph/pydantic_graph/node.py:60`, `Join` at `pydantic_graph/pydantic_graph/join.py:150`, `Decision` at `pydantic_graph/pydantic_graph/decision.py:41`) and dependency edges via `edges_by_source` / `parent_forks` / `intermediate_join_nodes` at `pydantic_graph/pydantic_graph/graph_builder.py:193`. Tasks are `GraphTaskRequest`/`GraphTask` at `pydantic_graph/pydantic_graph/graph_builder.py:389` executed as supersteps via `iter_graph` at `pydantic_graph/pydantic_graph/graph_builder.py:673`.

There is no `TodoItem`, `Plan`, `Subtask`, or `depends_on` type in `pydantic_ai_slim` — grep for `todo|Todo|Plan|subtask` in `pydantic_ai_slim/pydantic_ai` returns no todo schema. Planning/todo decomposition is explicitly delegated to out-of-core packages: `docs/capabilities/third-party.md:9` routes task planning to `pydantic-ai-harness` `Planning` and community `pydantic-ai-todo` `TodoCapability` (`add_todo`, `read_todos`, `write_todos`, `update_todo_status`, `remove_todo`), and `docs/toolsets.md:898` lists `TodoToolset` with `read_todos`/`write_todos` as third-party.

The answer to "can the system tell which subtask is blocking progress?" is **yes for graph execution tasks** (via `_GraphIterator.active_tasks` at `pydantic_graph/pydantic_graph/graph_builder.py:661` + `active_reducers` at `pydantic_graph/pydantic_graph/graph_builder.py:662` + `ErrorMarker` at `pydantic_graph/pydantic_graph/graph_builder.py:126` / `EndMarker` at `pydantic_graph/pydantic_graph/graph_builder.py:103`), but **no for plan-level todos** (nothing to block — no schema, no status, no dependency field).

## Rating

**3 / 10 — Absent / ad-hoc for plan todos; present as generic graph but not wired as a plan layer.**

Rationale:

- **Strength (graph layer):** Typed, test-covered decomposition engine exists: `GraphBuilder` with `Fork` (broadcast + map including async-iterable) at `pydantic_graph/pydantic_graph/node.py:60`, `Join` with pluggable `ReducerFunction` at `pydantic_graph/pydantic_graph/join.py:91`, `Decision` with exhaustive type matching at `pydantic_graph/pydantic_graph/decision.py:925`, dependency tracking via `parent_forks`/`intermediate_join_nodes` and `_is_fork_run_completed` at `pydantic_graph/pydantic_graph/graph_builder.py:1084`, and flow control via `override_next` at `pydantic_graph/pydantic_graph/graph_builder.py:586` / `next` at `pydantic_graph/pydantic_graph/graph_builder.py:564\|. Mermaid rendering and anyio CancelScope per task provide safeguards.
- **Deduction — no plan/todo schema in core:** Zero plan objects in `pydantic_ai_slim`; grep confirms. Delegation prose in `docs/capabilities/third-party.md:9` and `docs/toolsets.md:898` is the only "schema."
- **Deduction — no todo status tracking:** `GraphTask` at `pydantic_graph/pydantic_graph/graph_builder.py:411` carries `task_id`, `fork_stack`, `node_id` but no `status` enum (`pending|running|blocked`); status is implicit runtime set membership (`active_tasks`). External `TodoCapability` subtask statuses are not evidenced in this source.
- **Deduction — no plan dependency representation:** `Graph` edges support dependency semantics, but `GraphAgentState` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299` holds `message_history`, `usage`, `run_step`, `run_id` — no todo list with `depends_on`.
- **Deduction — no reordering/evaluation of todos:** Reordering for graph tasks requires rebuilding the graph; `_GraphIterator._handle_execution_request` at `pydantic_graph/pydantic_graph/graph_builder.py:850` dispatches in insertion order, no `reorder` API. Evaluation is graph completion (`EndMarker`), not plan coverage.

A 7 would require an explicit, tested `TodoItem`/`Plan` schema with typed subtasks, dependency edges, status lifecycle, and blocking query in slim itself. A 9 would require durable, observable, reorderable decomposition proven under failure — absent.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Todo delegation — docs | Task planning routed to Harness `Planning` ("Model-owned task plans with a cache-safe live reminder") and community `pydantic-ai-todo` `TodoCapability` with `add_todo`, `read_todos`, `write_todos`, `update_todo_status`, `remove_todo`, subtasks + dependencies + Postgres | `docs/capabilities/third-party.md:9-11` |
| Todo toolset delegation | Third-party `TodoToolset` with `read_todos`/`write_todos` listed under Task Management; implies no core toolset | `docs/toolsets.md:895-900` |
| Planning in capabilities overview | Harness `Planning` entry in overview table confirms planning is Harness-owned, not slim | `docs/capabilities/overview.md:72-73` |
| Agent graph absent planner | `build_agent_graph` builds only 4 nodes: `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, `SetFinalResult`; no planner node; `GraphBuilder(name="Agent", state_type=GraphAgentState, ...)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2574-2603` |
| Agent state — no plan field | `@dataclass GraphAgentState` with `message_history`, `usage`, `run_step`, `run_id`, `conversation_id`, `pending_messages`, `event_stream_buffer` — no `todos`, `plan`, `subtasks` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299-343` |
| Agent node base | `class AgentNode(BaseNode[GraphAgentState, GraphAgentDeps[...]])` — base for prompt/model/tools nodes | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:443-447` |
| Graph primitives exports | `GraphBuilder`, `Graph`, `GraphRun`, `GraphTask`, `GraphTaskRequest`, `EndMarker`, `ErrorMarker`, `JoinItem`, `Step`, `Fork`, `Decision`, `Join` re-exported | `pydantic_graph/pydantic_graph/__init__.py:14-35` |
| Base node decomposition contract | `class BaseNode` async `run(ctx: GraphRunContext) -> BaseNode \| End`; return annotation read at build time to infer outgoing edges | `pydantic_graph/pydantic_graph/basenode.py:33-51` |
| GraphContext shared deps/state | `@dataclass GraphRunContext(state: StateT, deps: DepsT)` passed to every node | `pydantic_graph/pydantic_graph/basenode.py:23-30` |
| End/Edge types | `@dataclass End(data: RunEndT)` and `@dataclass Edge(label)` for termination/labels | `pydantic_graph/pydantic_graph/basenode.py:61-74` |
| Task schema | `@dataclass GraphTaskRequest(node_id, inputs, fork_stack)` and `@dataclass GraphTask(GraphTaskRequest, task_id)` with `from_request` factory | `pydantic_graph/pydantic_graph/graph_builder.py:389-428` |
| End/Error markers | `@dataclass EndMarker(_value: OutputT)` and `@dataclass ErrorMarker(error: BaseException)` — next-item contract | `pydantic_graph/pydantic_graph/graph_builder.py:103-138` |
| JoinItem carrier | `@dataclass JoinItem(join_id, inputs, fork_stack)` carrying fan-in data + fork provenance | `pydantic_graph/pydantic_graph/graph_builder.py:139-155` |
| Graph definition with dependencies | `@dataclass Graph(nodes, edges_by_source, parent_forks, intermediate_join_nodes)` with `get_parent_fork`/`is_final_join` | `pydantic_graph/pydantic_graph/graph_builder.py:158-238` |
| Graph run — task lifecycle | `class GraphRun` with `_active_reducers`, `_next: EndMarker\|ErrorMarker\|Sequence[GraphTask]`, `_first_task: GraphTask(__start__)`, `_get_next_task_id`/`_get_next_node_run_id` | `pydantic_graph/pydantic_graph/graph_builder.py:430-634` |
| Graph iterator supersteps | `class _GraphIterator` with `active_tasks: dict[TaskID, GraphTask]`, `active_reducers: dict[tuple[JoinID, NodeRunID], JoinState]`, iter stream sender/receiver via `create_memory_object_stream` | `pydantic_graph/pydantic_graph/graph_builder.py:651-672` |
| Superstep scheduler | `async def iter_graph(first_task) -> AsyncGenerator[EndMarker\|ErrorMarker\|Sequence[GraphTask]]` — handles fork/join/reducer/decision per superstep, yields next batch, supports `cancel_and_drain` | `pydantic_graph/pydantic_graph/graph_builder.py:673-841` |
| Status tracking — active_tasks | `async def _finish_task(task_id)` pops from `cancel_scopes` and `active_tasks`; `_is_fork_run_completed` checks active_tasks to decide join finalization | `pydantic_graph/pydantic_graph/graph_builder.py:843-849,1084-1094` |
| Dependency — fork | `@dataclass Fork(id: ForkID, is_map: bool, downstream_join_id)` with `is_map determining map vs broadcast | `pydantic_graph/pydantic_graph/node.py:60-80` |
| Dependency — join/reducer | `@dataclass Join(id, _reducer, _initial_factory, parent_fork_id)` with `reduce(ctx, current, inputs)` dispatching 2-arg vs 3-arg reducer; `JoinState(current, downstream_fork_stack, cancelled_sibling_tasks)` | `pydantic_graph/pydantic_graph/join.py:30-38,150-199` |
| Dependency — decision branching | `@dataclass Decision(id, branches)` + `DecisionBranch(source, matches, path)` evaluated via `isinstance`/`Literal`/`matches` predicate at `_handle_decision` | `pydantic_graph/pydantic_graph/decision.py:40-125,925-948` |
| Dependency — path/edges | `Path`/`EdgePath`/`DestinationMarker`/`TransformMarker`/`LabelMarker` encoding edge contents; `_handle_path` resolves transform/label/next | `pydantic_graph/pydantic_graph/paths.py:40-120` and `pydantic_graph/pydantic_graph/graph_builder.py:1002-1032` |
| Control — next/override | `GraphRun.next(value)` advances via `anext`/`asend`; `GraphRun.override_next(value)` redirects after `End`/`ErrorMarker` by wiring `_set_next` | `pydantic_graph/pydantic_graph/graph_builder.py:564-605` |
| ID types | `TaskID`, `NodeID`, `ForkID`/`JoinID`, `ForkStackItem(fork_id, node_run_id, thread_index)`, `ForkStack = tuple[ForkStackItem, ...]` | `pydantic_graph/pydantic_graph/id_types.py:27-55` |
| Builder entry | `class GraphBuilder(name, state_type, deps_type, input_type, output_type)` with fluent `g.add`, `g.node`, `g.edge_from`, `g.start_node`, `g.build()` | `pydantic_graph/pydantic_graph/graph_builder.py:1138-1300` |
| Tool mapping — CallToolsNode | `CallToolsNode` parallel tool dispatch via `process_tool_calls` but not modeled as subtask decomposition | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1100-1350` (approx, class `CallToolsNode`) |
| Grep boundary — no todo schema | `grep -R "class Todo\|class Plan\|class SubTask" pydantic_ai_slim/pydantic_ai` returns 0 hits; `grep "todo" -i` hits only `docs/` and tests cassetes | `pydantic_ai_slim/pydantic_ai/` (no match) |
| Tests — graph decomposition | Builder tests cover steps, forks, joins, reducers, decisions, broadcast/map, parent-forks, mermaid rendering, otel spans | `tests/graph/builder/test_graph_builder.py:1-80`, `tests/graph/builder/test_broadcast_and_spread.py:1-60`, `tests/graph/builder/test_joins_and_reducers.py:1-80`, `tests/graph/builder/test_decisions.py:1-80`, `tests/graph/builder/test_parent_forks.py:1-60` |

## Answers to Dimension Questions

**1. How is a task decomposed?**
Two disjoint stories. In-core task decomposition is **graph decomposition**: a developer authors a typed workflow via `GraphBuilder` (`pydantic_graph/pydantic_graph/graph_builder.py:1139`) composing `BaseNode` subclasses (`pydantic_graph/pydantic_graph/basenode.py:33`), `Step` functions (`pydantic_graph/pydantic_graph/step.py:120`), `Fork` nodes for parallel fan-out (`pydantic_graph/pydantic_graph/node.py:60`), `Join` reducers for fan-in (`pydantic_graph/pydantic_graph/join.py:150`), `Decision` branches for conditional routing (`pydantic_graph/pydantic_graph/decision.py:41`), and `Path` edges for transforms (`pydantic_graph/pydantic_graph/paths.py`). At runtime `_GraphIterator.iter_graph` (`pydantic_graph/pydantic_graph/graph_builder.py:673`) decomposes the workflow into supersteps of `GraphTask` batches (`pydantic_graph/pydantic_graph/graph_builder.py:389`). The Pydantic AI agent itself is not decomposed this way — its graph is a fixed 4-node loop at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2594` — so "task" for an agent run means "model decides via tools," not "framework spawns subtasks." For user-facing task planning, decomposition is **model-owned imperative tool use against an external capability**: `docs/capabilities/third-party.md:9` and `docs/toolsets.md:898` describe `TodoCapability`/`TodoToolset` where the model calls `add_todo`/`write_todos` to author subtasks in free-form strings. No first-party `Decompose(task)->list[Subtask]` exists in slim.

**2. Are subtasks typed?**
Partially — typed at the graph layer, untyped at the plan layer. Graph subtasks (`GraphTask` at `pydantic_graph/pydantic_graph/graph_builder.py:411`) are typed by the generic parameters `StateT`, `DepsT`, `InputT`, `OutputT` on `Graph`/`GraphBuilder` (`pydantic_graph/pydantic_graph/graph_builder.py:158`) and by the concrete `BaseNode` subclass or `StepFunction` signature; the compiler infers edge types from `BaseNode.run` return annotations (`pydantic_graph/pydantic_graph/basenode.py:42`). There is no `SubtaskKind` enum or `type` field on a todo — the external `pydantic-ai-todo` schema (outside this source) defines titles/descriptions, and core `GraphTask` carries only `node_id`, `inputs`, `fork_stack`, `task_id` (no `kind`, `priority`, `effort`). For agent execution, every "subtask" is a tool call whose schema derives from `ToolDefinition` (`pydantic_ai_slim/pydantic_ai/tools.py`), not from a plan taxonomy.

**3. Can dependencies be represented?**
Yes for graph dependencies, no for plan dependencies in core. `Graph.edges_by_source: dict[NodeID, list[Path]]` (`pydantic_graph/pydantic_graph/graph_builder.py:193`) plus `parent_forks` (`pydantic_graph/pydantic_graph/graph_builder.py:196`) and `intermediate_join_nodes` (`pydantic_graph/pydantic_graph/graph_builder.py:199`) model fork/join dependencies explicitly; `_is_fork_run_completed` at `pydantic_graph/pydantic_graph/graph_builder.py:1084` and `_resolve_join_fork_run` at `pydantic_graph/pydantic_graph/graph_builder.py:967` implement "wait for all branches" semantics, and `Decision` at `pydantic_graph/pydantic_graph/decision.py:925` models conditional dependencies. However `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:299`) carries no `depends_on` list, and `GraphTask` has no `blocked_by` / `after` edge — dependency is structural (which nodes are wired together at build time), not a per-subtask field the model can author. For plan todos, any `depends_on` would live in the external `pydantic-ai-todo` tool (cited at `docs/capabilities/third-party.md:11` as "Supports subtasks, dependencies, and PostgreSQL persistence"), unverified in this source per isolation.

**4. Are statuses tracked?**
Graph-task status is tracked but implicit, plan-todo status is absent in core. `_GraphIterator.active_tasks: dict[TaskID, GraphTask]` (`pydantic_graph/pydantic_graph/graph_builder.py:661`) and `active_reducers` (`pydantic_graph/pydantic_graph/graph_builder.py:662`) plus `cancel_scopes: dict[TaskID, CancelScope]` (`pydantic_graph/pydantic_graph/graph_builder.py:660`) track which tasks are in-flight; `ErrorMarker` (`pydantic_graph/pydantic_graph/graph_builder.py:126`) vs `EndMarker` (`pydantic_graph/pydantic_graph/graph_builder.py:103`) vs `Sequence[GraphTask]` signals terminal failure vs success vs next batch. There is no persisted `status: pending|in_progress|blocked|done` field — `GraphTask` is ephemeral per superstep and completion is removal from `active_tasks` via `_finish_task` at `pydantic_graph/pydantic_graph/graph_builder.py:843`. `GraphAgentState.run_step: int` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:306` counts loop iterations, not subtask progress. Plan status, if any, is the boolean open/done in external `TodoCapability` tool state, not in slim; no `blocked` or `waiting_on` propagation to `GraphRun.next_task` at `pydantic_graph/pydantic_graph/graph_builder.py:607`.

**5. Can decomposition be evaluated?**
Not structurally inside this source. Graph completeness is evaluated as "did we reach `EndMarker`?" — `Graph.run` at `pydantic_graph/pydantic_graph/graph_builder.py:240` and `GraphRun.next` at `pydantic_graph/pydantic_graph/graph_builder.py:564` assert `isinstance(event, EndMarker)` else the run continues. Observability is via Mermaid rendering at `pydantic_graph/pydantic_graph/graph_builder.py:363` and OTel spans (`logfire_span` at `pydantic_graph/pydantic_graph/graph_builder.py:883` inside `_run_task`). There is no `is_plan_complete` / "which subtask is blocking?" query for todos; the only blocking signal at the graph level is `active_tasks` emptiness triggering reducer finalization at `pydantic_graph/pydantic_graph/graph_builder.py:764-818` and `JoinState.cancelled_sibling_tasks` early-stop via `ReducerContext.cancel_sibling_tasks` at `pydantic_graph/pydantic_graph/join.py:73`. Tests in `tests/graph/builder/test_graph_execution.py` verify execution outcomes, not plan coverage/minimality/quality. For agent todos, evaluation would be model-self-reported or via an external LLM judge, not a runtime verifier in slim.

## Architectural Decisions

| Decision | Why it was made (inferred) | Consequence for this dimension |
|----------|-----------------------------|--------------------------------|
| **Core stays plan-free; planning is Harness/third-party** — `docs/capabilities/third-party.md:9`, `docs/toolsets.md:898`, `docs/capabilities/overview.md:72` | Keep `pydantic_ai_slim` minimal, typed, provider-agnostic; avoid opinionated orchestration per `AGENTS.md` philosophy ("strong primitives over narrow solutions"). | No `Plan`/todo type in core; decomposition for agent tasks is whatever the model invents via tools — no contract, no blocking query. |
| **Fixed 4-node agent FSM** — `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2594` (`UserPromptNode`→`ModelRequestNode`→`CallToolsNode`→`SetFinalResult`) | Simplify the agent loop to a model-driven ReAct loop; reuse generic `pydantic_graph` substrate. | No planner node to emit/evaluate subtasks; execution tasks are graph supersteps, not user-visible plan steps. |
| **Generic typed graph as the universal decomposition primitive** — `pydantic_graph/pydantic_graph/__init__.py:1`, `pydantic_graph/pydantic_graph/graph_builder.py:1139` | Provide one well-typed async superstep engine for all workflows (agent loop, user graphs, harness). | Dependency modeling is powerful (Fork/Join/Decision/Reducer) but build-time declarative; runtime plan authoring still requires external tooling. |
| **Capability as extension point for planning** — `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:1` (`AbstractCapability` with `id`, `defer_loading`, `get_toolset`, `get_instructions`) | Compose planning like any other ability (MCP, Thinking) without coupling core to a plan store. | Planning capability must bring its own persistence/status/dependency semantics; core offers no shared `TodoStore`. |
| **Ehemeral GraphTask + active-set status** — `pydantic_graph/pydantic_graph/graph_builder.py:389,660` | Avoid a heavyweight status enum; leverage anyio task-group + memory streams for lightweight supersteps. | "Which subtask is blocking?" answered by inspecting `active_tasks`/`active_reducers` at runtime, but not queryable post-run or for todos. |
| **Placeholder IDs then deterministic replacement** — `pydantic_graph/pydantic_graph/id_types.py:50`, `pydantic_graph/pydantic_graph/graph_builder.py:50` `generate_placeholder_node_id` / `replace_placeholder_id` | Stable Mermaid diagrams and reproducible builds without manual ID plumbing. | Decomposition graphs are deterministically addressable, aiding observability but not subtask reordering. |

## Notable Patterns

- **Superstep/BSP execution** — `_GraphIterator.iter_graph` (`pydantic_graph/pydantic_graph/graph_builder.py:673`) dispatches a batch of `GraphTask`s into an `anyio` TaskGroup, drains results from a memory object stream, then computes next batch via edge resolution (`_handle_path` at `pydantic_graph/pydantic_graph/graph_builder.py:1002`), mirroring Pregel-style barriers where joins wait for all fork branches.
- **Fork-then-Join with pluggable reducers** — `Fork(is_map)` map vs broadcast at `pydantic_graph/pydantic_graph/node.py:60` plus `Join(reducer, initial_factory)` at `pydantic_graph/pydantic_graph/join.py:150` supporting `reduce_list_append`/\_extend/\_dict\_update/\_sum/`ReduceFirstValue` (early-stop via `ctx.cancel_sibling_tasks()` at `pydantic_graph/pydantic_graph/join.py:73`).
- **Exhaustive Decision routing** — `DecisionBranch` tested in `_handle_decision` (`pydantic_graph/pydantic_graph/graph_builder.py:925`) via `isinstance`/`Literal`/`matches` predicate, with build-time exhaustiveness checking through variance tricks at `pydantic_graph/pydantic_graph/decision.py:88`.
- **NodeStep/JoinNode bridge** — `NodeStep` and `JoinNode` wrappers at `pydantic_graph/pydantic_graph/step.py:160` and `pydantic_graph/pydantic_graph/join.py:220` let legacy `BaseNode` returns participate in builder edges without direct wiring.
- **Error-as-value with recovery hook** — `ErrorMarker` yielded instead of raised at `pydantic_graph/pydantic_graph/graph_builder.py:688` lets callers recover via `override_next` (`pydantic_graph/pydantic_graph/graph_builder.py:586`) / `_wrap_and_advance` capability hooks, analogous to `on_node_run_error`.
- **Delegatory planning documentation pattern** — "Task Management" sections at `docs/capabilities/third-party.md:7` and `docs/toolsets.md:897` list external todo packages as the canonical solution, establishing community-owned plan schemas over core-owned ones.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Graph is generic, not plan-specific | One engine powers agent loop, user workflows, and future planners; heavily typed and tested (`tests/graph/builder/`). | Plan decomposition needs extra packaging; no shared `Plan` type means each harness/todo community does its own subtask status/dependency semantics. |
| Build-time declarative edges (`edges_by_source`) | Static verification, Mermaid rendering (`pydantic_graph/pydantic_graph/graph_builder.py:363`), deterministic parent-fork analysis. | Cannot reorder subtasks at runtime via a plan update tool; reordering = rebuilding the graph. |
| Active-set implicit status vs explicit enum | Minimal allocation, anyio CancelScope per task (`pydantic_graph/pydantic_graph/graph_builder.py:856`), cheap supersteps. | No query like `blocked_tasks: list[GraphTask]`; external observers must read `active_tasks` or stream events; no durable status for todos. |
| External TodoCapability | Keeps slim dependency lean (`pydantic_ai_slim/pyproject.toml` slim), lets community iterate on subtask/persistence models. | Fragmentation: `pydantic-ai-todo` subtasks, dependencies, Postgres persistence cited at `docs/capabilities/third-party.md:11` are unverified in this source; no guarantee of cross-run determinism or blocking detection. |
| Model-owned plan via tools | Any decomposition the model invents is accepted; maximal flexibility. | No runtime validation; malformed / contradictory / cyclic todo dependencies go undetected (no `validate_plan`). |
| Reducer early-stop (`cancel_sibling_tasks`) | Enables "first-wins" joins (`ReduceFirstValue` at `pydantic_graph/pydantic_graph/join.py:140`) for competitive parallel subtasks. | Early cancellation can hide other branch failures; sibling error is discarded if join already finalized. |

## Failure Modes / Edge Cases

- **No plan schema → hallucinated drift** — With no `Plan` object at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299`, a model that mutates its todo list implicitly (continues with different steps than announced) has no guardrail; execution silently diverges. No validator equivalent to output validation at `pydantic_ai_slim/pydantic_ai/_output.py`.
- **Dependency not enforced for todos** — Even if external `TodoCapability` stores `depends_on`, core never topologically sorts or blocks `CallToolsNode`; two todos like "write tests" and "ship (needs tests)" have no machine relation visible to the runner.
- **Blocked subtask invisible at plan layer** — `todos_get_remaining` (external) is boolean "any open"; graph-level `active_reducers` at `pydantic_graph/pydantic_graph/graph_builder.py:765` can locate a join waiting on branches, but no API surfaces "todo X is blocked by Y."
- **No reorder without rebuild** — Graph task order is insertion order in `_handle_execution_request` (`pydantic_graph/pydantic_graph/graph_builder.py:850`); moving a todo earlier requires remove+add (new id, loss of history/refs). No `todos_reorder` tool in core.
- **Fork empty-iterable edge case handled, but empty plan silent** — `_handle_fork_edges` at `pydantic_graph/pydantic_graph/graph_builder.py:1044` eagerly creates join state for empty maps so joins still fire with initial value; an empty todo plan, by contrast, yields immediate loop end with no signal.
- **Non-iterable map input → hard RuntimeError** — `pydantic_graph/pydantic_graph/graph_builder.py:1078` raises `RuntimeError("Cannot map non-iterable value: ...")` when a `Fork(is_map=True)` receives a scalar; plan-level "map subtask over items" would hit the same if decomposition emits wrong shape.
- **Unmatched Decision branch → RuntimeError** — `_handle_decision` at `pydantic_graph/pydantic_graph/graph_builder.py:948` raises if no branch matches; an LLM-authored plan value outside expected union stalls the graph rather than marking a todo as blocked.
- **Non-deterministic tool plan persistence gap** — `GraphAgentState` and message history are the only durable artefacts; mid-run crashes lose any in-memory plan unless a harness persistence capability is composed. `_dangling_tool_calls_by_response` repair at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2710` synthesizes interrupted tool returns but not plan state.
- **Concurrent plan mutation race** — Parallel tool calls (default) can interleave `read_todos`/`write_todos` against a file/Postgres todo store without a core lock; per-source locks needed in external package, not in `pydantic_graph`.

## Future Considerations

- Add an optional first-party `Plan`/`PlanStep{title, depends_on: list[str], status, priority, kind}` dataclass in `pydantic_ai_slim` validated via `Toolset` and surfaced through `AbstractCapability` hooks (`before_node_run` gating `CallToolsNode` on `all(depends_on.done)`), enabling structural blocking queries without adopting full harness.
- Wire a `pydantic_graph`-backed planner that compiles a validated `Plan` into a `GraphBuilder` subgraph (Fork/Join/Decision with reducers) for resumable, type-checked plan execution — reuses `parent_forks` dependency analysis instead of reinventing it.
- Expose observability for plan vs graph tasks: OTEL spans `plan.subtask.blocked` / `plan.subtask.completed` via `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py` so Logfire can surface "which subtask is blocking?"
- Introduce a `todos_reorder`/`todos_update` tool (in-place mutation without re-minting IDs) and a `blocked_todos` derived view, mirroring the `is_fork_run_completed` logic but for plan DAGs, to let `AgentRun.next` surface blockers.
- Persist `status` transitions with timestamps (`pending→in_progress→blocked→done` + `blocked_by`) so file/DB checkpoints survive crashes; mirror `GraphTask.task_id` uniqueness with deterministic `NodeRunID` handling already in `pydantic_graph/pydantic_graph/id_types.py:50`.
- Bridge todos ↔ graph: a `TodoWorkflowAdapter` that materializes `list[TodoItem(depends_on)]` into `WorkflowBuilder.add_edge`-style dependencies so the plan and the executed DAG become one artefact.

## Questions / Gaps

- **No evidence found** for a core `TodoItem`/`Plan`/`Subtask` type — searched `pydantic_ai_slim/pydantic_ai` for `todo|Todo|Plan|subtask` and `class.*Task` besides `GraphTask`; only `GraphTask`/`JoinItem`/`BaseNode` at `pydantic_graph/pydantic_graph/` appear. Confirmation via `docs/capabilities/third-party.md:9` and `docs/toolsets.md:898` that todo planning is external.
- **No evidence found** for a `depends_on`/`blocked_by`/`priority`/`reorder` field on any in-core plan object — `GraphAgentState` at `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299` and `GraphTask` at `pydantic_graph/pydantic_graph/graph_builder.py:389` lack these fields; graph dependencies are structural edges, not per-todo metadata.
- **No evidence found** for a persisted subtask `status` enum beyond graph runtime sets — `_GraphIterator.active_tasks` at `pydantic_graph/pydantic_graph/graph_builder.py:661` is runtime-only; no tests assert todo status lifecycle (sibling harness tests not inspected per isolation).
- **No evidence found** for plan-level mapping to tool calls — `CallToolsNode` maps LLM tool calls to tool execution (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1100` region) but not via a decomposed subtask dispatch table; tool→subtask affinity, if any, is model-authored free text.
- **No evidence found** for decomposition evaluation/validation — no `validate_plan`, no plan-coverage metric, no "is every required subtask covered?" check; only graph `EndMarker` reachability and superstep barrier checks at `pydantic_graph/pydantic_graph/graph_builder.py:764`.
- **Cannot fully answer** whether external `pydantic-ai-todo` decomposition survives durable replay — delegation docs at `docs/capabilities/third-party.md:11` claim Postgres persistence, but implementation, atomicity, and replay semantics were not inspected (external repo, out of scope).
- **No evidence found** for a reordering API that preserves identity — graph reorder requires rebuild; external todo reorder would require `remove`+`add` with id loss.

---

Generated by `dimensions/06.02-task-decomposition-representation.md` against `pydantic-ai`.
