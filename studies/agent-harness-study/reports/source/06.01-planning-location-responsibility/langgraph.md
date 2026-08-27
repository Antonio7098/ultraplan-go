# Source Analysis: langgraph

## 06.01 Planning Location and Responsibility

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo `libs/langgraph`, `libs/prebuilt`, `libs/cli`, `libs/checkpoint*`, `libs/sdk-py/js`) |
| Analyzed | 2026-08-27 |

## Summary

LangGraph treats "planning" as two distinct concepts. **Orchestration planning** (which nodes run next) is a first-class, runtime-owned object: the developer-authored `StateGraph` compiles to a `Pregel` execution engine that implements a Bulk Synchronous Parallel loop with explicit `Plan → Execution → Update` phases (`libs/langgraph/langgraph/pregel/main.py:464-474`, `libs/langgraph/langgraph/pregel/_algo.py:392-513`). **Task-decomposition planning** (LLM breaks goal into sub-goals) has no runtime primitive. The prebuilt `create_react_agent` is a fixed ReAct loop (`agent → tools → agent`) with routing via `should_continue`/`post_model_hook_router` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-858`, `918-956`) and no planner node. Historic `plan-and-execute` has been removed/redirected (`docs/redirects.json:190`) and the README explicitly outsources decomposition to the external `Deep Agents` package (`README.md:31`, `README.md:54`). Planning is therefore **workflow-graph + Pregel-scheduler + (optionally) model-owned tool-calling**, not a dedicated planner agent or prompt-owned plan artifact.

## Rating

**Score: 6 / 10**

**Rationale:** Workflow-graph planning is mature, explicit, typed, checkpoint-durable and observable (Pregel + `StateGraph` + `prepare_next_tasks` with 60+ tests in `libs/langgraph/tests/test_pregel*.py`). However intentional, LLM-driven task decomposition is deliberately absent as a runtime object: no `Planner` class/prompt, no `Task` decomposition API, no `Plan` state channel in `libs/langgraph` or `libs/prebuilt`. The framework provides the primitives to *build* a planner (conditional edges, `Send`, `Command`, `task`/`entrypoint`), but the plan itself lives in user code or an external system (`deepagents`). This split — excellent low-level scheduling vs. missing high-level planning primitive — maps to "Present but inconsistent / fragile" for the dimension's question *Is planning a real runtime object or just prose?* Low-level: yes. High-level: just prose in node prompts.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pregel runtime Plan phase | `Pregel` docstring defines execution in three phases: `Plan: Determine which actors to execute... select actors that subscribe to channels updated in previous step` followed by Execution and Update | `libs/langgraph/langgraph/pregel/main.py:464-474` |
| Deterministic scheduler | `prepare_next_tasks()` computes next tasks from `checkpoint["channel_versions"]`, `versions_seen`, `trigger_to_nodes` and `updated_channels`; handles `PUSH` (Send) and `PULL` (subscribed nodes) | `libs/langgraph/langgraph/pregel/_algo.py:392-513` |
| Scheduler trigger predicate | `_triggers()` compares channel versions vs `seen` to decide if a node fires; fallback to `is_available()` on first superstep | `libs/langgraph/langgraph/pregel/_algo.py:1260-1277` |
| StateGraph builder API | `StateGraph` is the declarative planning surface: `add_node(node, retry_policy, cache_policy, timeout, ...)`, `add_edge()`, `add_conditional_edges()`, `add_sequence()`, `set_entry_point()`, `validate()`, `compile()` | `libs/langgraph/langgraph/graph/state.py:131-200`, `667-926`, `928-980`, `982-1030`, `1177-1189` |
| Compilation to Pregel | `StateGraph.compile()` validates graph, builds channel specs from `state_schema`, applies `_node_defaults`, creates `Pregel` with `trigger_to_nodes` map and optional `serde_allowlist` | `libs/langgraph/langgraph/graph/state.py:1231-1254`, `1256-1302`, `1340-1390` |
| Functional API planning | `task` decorator wraps callables as `_TaskFunction` producing futures via `_call_with_options`; `entrypoint` compiles singled-node function to `Pregel` with `START/PREVIOUS/END` channels | `libs/langgraph/langgraph/func/__init__.py:59-94`, `262-435`, `516-620` |
| Prebuilt ReAct loop (no planner) | `create_react_agent(model, tools, prompt, pre_model_hook, post_model_hook)` docstring: "calls tools in a loop until stopping condition" with mermaid `U->>A->>T->>A` cycle; `should_continue` returns `END` or `Send("tools", ...)` / `post_model_hook` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-316`, `830-859`, `860-1002` |
| Prebuilt single-turn short-circuit | `if not tool_calling_enabled: workflow = StateGraph(...) workflow.add_node("agent") ... workflow.set_entry_point(entrypoint)` — no planning step when no tools | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:787-828` |
| Conditional planning primitive | `StateGraph.add_conditional_edges(source, path, path_map)` coerce path to `Runnable`, store as `BranchSpec.from_path(path, path_map, True)` — the mechanism for dynamic LLM routing, not a planner abstraction | `libs/langgraph/langgraph/graph/state.py:982-1030` |
| Loop orchestration | `PregelLoop.tick()` calls `prepare_next_tasks(... for_execution=True ...)` then `should_interrupt`; `after_tick()` calls `apply_writes()` and `_put_checkpoint()` | `libs/langgraph/langgraph/pregel/_loop.py:599-682`, `683-726` |
| Channel writes | `apply_writes()` sorts tasks by `task_path_str(t.path[:3])`, bumps `versions_seen`, consumes triggering channels, groups writes by channel, returns `updated_channels` | `libs/langgraph/langgraph/pregel/_algo.py:232-345` |
| Explicit plan absence — grep | `grep -r "planner\|create_react_agent.*plan" libs/langgraph` yields only `create_checkpoint_plan_for_update_state_api` and test fixtures with ad-hoc `def planner(state)` mock node | `libs/langgraph/langgraph/pregel/_checkpoint.py:117`, `libs/langgraph/tests/test_pregel.py:5171`, `libs/langgraph/tests/test_pregel_async.py:6426` |
| Externalized planning | `redirects.json` rewrites `/tutorials/plan-and-execute/plan-and-execute` → `langchain/middleware/built-in#to-do-list`; README recommends `Deep Agents — agents that can plan, use subagents` | `docs/redirects.json:190`, `README.md:31`, `README.md:54` |
| Monorepo isolation | `AGENTS.md` confirms core engine lives in `libs/langgraph` and prebuilt agents in `libs/prebuilt`; no separate planning package | `AGENTS.md:19-29` |
| Checkpoint-planning naming false positive | `create_checkpoint_plan_for_update_state_api` refers to delta-checkpoint write planning, not LLM task planning | `libs/langgraph/langgraph/pregel/_checkpoint.py:117-145` |

## Answers to Dimension Questions

### 1. Where does planning happen?
**Workflow graph (developer-authored) + Pregel runtime scheduler; prompts only as model input.**

- **Static planning** at graph-definition time: developer declares nodes, channels, edges and conditional branches via `StateGraph` (`libs/langgraph/langgraph/graph/state.py:667-1030`). Compilation validates and produces `trigger_to_nodes` map (`libs/langgraph/langgraph/graph/state.py:947-948`, `1340-1390`).
- **Dynamic planning per superstep** at runtime: `PregelLoop.tick()` → `prepare_next_tasks()` (`libs/langgraph/langgraph/pregel/_loop.py:612-629`) decides which `PregelNode`/`Send` tasks fire based on updated channels (BSP model, `libs/langgraph/langgraph/pregel/main.py:464-474`).
- **Model-side "planning"** is implicit: `create_react_agent`'s `prompt` arg (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:366-371`, `589-590`) is prepended to messages via `_get_prompt_runnable` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170`), then `call_model` invokes LLM and `should_continue` checks `tool_calls` presence (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-835`). No planner agent, no plan artifact.
- **Functional API**: `@task`/`@entrypoint` tasks enqueue via `PUSH` paths using `prepare_push_task_functional`/`prepare_push_task_send` (`libs/langgraph/langgraph/pregel/_algo.py:800-1107`), again scheduler-owned.

### 2. Who owns the plan?
**Shared: developer owns the topology, runtime owns execution, model owns the next tool choice.**

- Developer owns `StateGraph` definition (nodes/edges/schemas) — `StateGraph.validate()` enforces correctness (`libs/langgraph/langgraph/graph/state.py:1129-1175`).
- Runtime (`Pregel`/`PregelLoop`) owns the authoritative step plan (`prepare_next_tasks` → `apply_writes` → `create_checkpoint`) and persists it durably (`libs/langgraph/langgraph/pregel/_loop.py:1081-1219`).
- Model owns only the ReAct tool-call decision inside the `agent` node (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:661-694`). The post-model hook can revise (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:919-956`) but does not author a multi-step plan object.
- No `PlannerAgent` class exists. `THREAT_MODEL.md` enumerates components and lists `StateGraph`, `entrypoint`/`task`, `ToolNode` but no planner (`THREAT_MODEL.md:13-14`, `46-62`).

### 3. Is planning required?
**No. Planning is optional and graph-shape-dependent; zero-plan graphs are valid.**

- A `StateGraph` must have an entrypoint (`START` edge) but can be single-node (`libs/langgraph/langgraph/graph/state.py:1142-1145`). Compilation succeeds without conditional edges.
- `Pregel` itself accepts `input_channels`/`output_channels` with arbitrary channel sets; `prepare_next_tasks` returns `()` on empty `channel_versions` (`libs/langgraph/langgraph/pregel/_algo.py:483-484`).
- `create_react_agent` with empty `tools` creates a single `agent` node with direct entrypoint, no loop (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:787-802`).
- `@entrypoint` compiles even a straight-line function to a single `PregelNode` triggered by `START` (`libs/langgraph/langgraph/func/__init__.py:576-609`).
- `retries/cache/timeout` defaults are per-node policies, not planning policies (`libs/langgraph/langgraph/graph/state.py:272-335`).

### 4. Is planning visible?
**Low-level plan is highly visible; high-level intent is not.**

- Visible: `Pregel.get_graph()`/`draw_graph()` drawable (`libs/langgraph/langgraph/pregel/main.py:845-912`), `StateSnapshot` with `next`, `tasks`, `interrupts` (`libs/langgraph/langgraph/pregel/main.py:1145-1266`), `map_debug_tasks`/`map_debug_checkpoint` streaming (`libs/langgraph/langgraph/pregel/_loop.py:632-650`, `672-674`), `checkpoint_metadata["step"]` counters (`libs/langgraph/langgraph/pregel/_loop.py:1125-1138`), and `debug` flag (`libs/langgraph/langgraph/pregel/main.py:730-731`). `PregelLoop._emit` drives `tasks`/`checkpoints`/`values` streams.
- Partially visible: conditional routing via `BranchSpec` (`libs/langgraph/langgraph/graph/state.py:1027`) is observable only as the chosen task name after scheduling.
- Not visible: ReAct reasoning is opaque LLM text and `tool_calls`; there is no `Plan` channel, no `plan` field in `AgentState` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:57-62`), no planning trace unless user adds a `pre_model_hook` reducer + logging.

### 5. Is planning reusable?
**Workflow topology is reusable; task-level plans are not.**

- `StateGraph` is composable: `CompiledStateGraph` is a `Runnable` that can be added as subgraph node (`get_subgraphs`/`aget_subgraphs` in `libs/langgraph/langgraph/pregel/main.py:1076-1114`), with hierarchical checkpoint namespaces (`NS_SEP`/`NS_END` in `libs/langgraph/langgraph/_internal/_constants.py` via `prepare_single_task` at `libs/langgraph/langgraph/pregel/_algo.py:615-625`). `add_sequence()` (`libs/langgraph/langgraph/graph/state.py:1032-1077`) enables pattern reuse.
- `Pregel.copy(with_config)` and `workflow.compile()` produce reusable executables (`libs/langgraph/langgraph/pregel/main.py:922-931`, `libs/langgraph/langgraph/graph/state.py:1188-1230`).
- No plan artifact to reuse: `create_react_agent` returns a full `CompiledStateGraph` but not a serializable `Plan` object; the only reuse is embedding the whole agent as subgraph. Functional `@task`/`@entrypoint` likewise compile to `Pregel` but do not export a `Plan` schema (`libs/langgraph/langgraph/func/__init__.py:516-620`).
- `pre_model_hook`/`post_model_hook` injection points allow reuse of trimming/validation middleware (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424`, `425-430`) but not planning middleware.

## Architectural Decisions

| Decision | Description | Tradeoff |
|----------|-------------|----------|
| **BSP Pregel engine as planner** — Single `Plan→Execution→Update` loop with channel versioning (`libs/langgraph/langgraph/pregel/main.py:464-474`, `libs/langgraph/langgraph/pregel/_loop.py:599-726`) | Deterministic, reproducible, checkpoint-friendly. Burst-parallel nodes via `prepare_next_tasks` sorting by `task_path_str` (`libs/langgraph/langgraph/pregel/_algo.py:256`) | No look-ahead planning; nodes only react to written channels, not predicted futures. Long-horizon tasks must be encoded as graph cycles. |
| **StateGraph as declarative planning DSL** — Typed channels + reducers + `add_node`/`add_edge`/`add_conditional_edges` compile to `Pregel` (`libs/langgraph/langgraph/graph/state.py:131-1390`) | Type-safe, validates at compile, introspectable via `get_graph()` | Static graph: plan cannot be rewritten by LLM at runtime beyond conditional branch choices or `Send` fan-out. |
| **`Send` + `TASKS` Topic channel for dynamic fan-out** — `TASKS` is a reserved `Topic[Send]` channel (`libs/langgraph/langgraph/pregel/main.py:804-809`), `should_continue` in v2 returns `list[Send]` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`) | Enables data-dependent parallelism without new graph structure | Plan surface is now `Send` packets, not a semantic task list; no dependency graph among Sends. |
| **ReAct as only prebuilt planning loop** — `agent` ↔ `tools` cycle until no `tool_calls` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:478-497` mermaid, `830-990`) | Simple, well-tested, supports `return_direct`/`post_model_hook` | No explicit decomposition, no todo-list, no critique step; long tasks rely on LLM recursion limit (`remaining_steps` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634` underestimates). |
| **Externalize deep planning to `deepagents`** — README and redirect deprecate `plan-and-execute` (`docs/redirects.json:190`, `README.md:31`) | Keeps core low-level and small; planning becomes opinionated extension | Dimension gap: core framework has no planner reuse, testing, or safeguard story for decomposition. |
| **Checkpoint-durable plan** — `create_checkpoint`/`_put_checkpoint` + `counters_since_delta_snapshot` for DeltaChannel (`libs/langgraph/langgraph/pregel/_loop.py:1081-1219`) | Plans survive failure, support time-travel (`get_state`/`update_state`) | Adds complexity; exit-mode delta-write accumulator (`_exit_delta_writes`) is subtle and tested mostly via large-case tests. |

## Notable Patterns

- **Channel-triggered planning:** Nodes declare `channels` + `triggers` (`libs/langgraph/langgraph/pregel/_read.py:read`); `PregelNode` metadata drives version comparison, not code inspection (`libs/langgraph/langgraph/pregel/_algo.py:1260-1277`). Clean separation of data plane (channels) and control plane (versions).
- **Runnable coercion for nodes/branches:** `coerce_to_runnable(action, trace=False)` in `StateGraph.add_node` (`libs/langgraph/langgraph/graph/state.py:883`) and `BranchSpec.from_path` (`libs/langgraph/langgraph/graph/state.py:1019`) make any callable a planning primitive without planner interface.
- **Prompt runnable indirection:** `_get_prompt_runnable(prompt)` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:137-170`) normalizes `str|SystemMessage|Callable|Runnable` into a `RunnableCallable` named `"Prompt"` so prompt becomes composable pipeline stage `prompt | model`.
- **Send-based v2 planning:** `ToolNode` + `Send(ToolCallWithContext)` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:850-858`) distributes tool calls as independent `PUSH` tasks in next superstep rather than parallel calls inside one node, making tool-level plan visible as task list.
- **Scratchpad-scoped resume:** `_scratchpad()` (`libs/langgraph/langgraph/pregel/_algo.py:1280-1345`) injects `CONFIG_KEY_RESUME_MAP`/`get_null_resume` so interrupts/resume participate in planning without mutating graph.

## Tradeoffs

- **Explicit topology vs. emergent planning:** Developer gets full visibility/reproducibility; LLM cannot invent new nodes at runtime. Good for production governance, bad for open-ended research tasks needing dynamic decomposition.
- **Checkpoint durability vs. planning flexibility:** Versioned channels allow time-travel and human-in-the-loop interrupts (`libs/langgraph/langgraph/pregel/_loop.py:848-959`), but imply all planning signals must be serializable through `JsonPlusSerializer`/`StrictMsgPack` (`libs/langgraph/langgraph/_internal/_serde.py`). Rich plan objects need allowlisting.
- **Minimal core vs. batteries-included planning:** Core stays unopinionated and dependency-light (`libs/langgraph/pyproject.toml` declares only `langgraph-checkpoint`, `langchain-core`, `pydantic`), but users re-implement similar ReAct/planner patterns. Evaluated against `deepagents`, this is intentional fragmentation.
- **Sync vs. async planning:** `Pregel`/`PregelLoop`/`AsyncPregelLoop`, `StateGraph` async branches, `acall_model`/`agenerate_structured_response` mirror sync paths (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:696-721`), doubling maintenance surface for planning invariants (see `_V3_INVARIANT_KWARGS` at `libs/langgraph/langgraph/pregel/main.py:384-395`).
- **Performance of planning phase:** `updated_channels` + `trigger_to_nodes` optimization (`libs/langgraph/langgraph/pregel/_algo.py:475-482`) avoids scanning all nodes when possible; fallback scans `processes.keys()` when no channel update info, so pathological fan-in graphs still scale linearly.

## Failure Modes / Edge Cases

| Mode | Symptom | Evidence |
|------|---------|----------|
| **No plan for empty tool set** | Agent loops degenerate to single LLM call; caller expects decomposition but gets direct answer | `tool_calling_enabled = len(tool_classes) > 0` branch in `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:567-828` creates one-node graph, `should_continue` never defined in that arm. |
| **Recursion limit masquerading as plan** | `remaining_steps` exhausted returns synthetic message "Sorry, need more steps..." instead of `GraphRecursionError` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:684-692`, `711-718`), silently capping plan length. `MANAGED` channel `RemainingSteps` not exposed as plan control. | Verified at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:620-634`, `438-441` |
| **Stale conditional planning due to missing branches** | `add_conditional_edges` without `path_map` assumes edges to any node (`libs/langgraph/langgraph/graph/state.py:958-965` warns via `draw_graph` but `validate()` only checks known targets when `BranchSpec.ends` provided, so orphan returns go undetected until runtime) | `libs/langgraph/langgraph/graph/state.py:1148-1159` |
| **Plan invalidation on checkpoint time-travel** | `is_time_traveling` drops `RESUME` writes and forks checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:897-972`) to preserve plan, but DeltaChannel `counters_since_delta_snapshot` (`libs/langgraph/langgraph/pregel/_loop.py:1112-1131`) can drift if overwrite tracking (`_delta_channels_with_overwrite`) missed | `libs/langgraph/langgraph/pregel/_loop.py:685-691`, `1138-1157` |
| **Untracked planning writes** | `TASKS` `Send` packets containing `UntrackedValue` channels are sanitized (`libs/langgraph/langgraph/pregel/_loop.py:439-452`, `sanitize_untracked_values_in_send` at `libs/langgraph/langgraph/pregel/_algo.py:109ff`), dropping planning signals silently. | `libs/langgraph/langgraph/pregel/_algo.py:109-`, `libs/langgraph/langgraph/pregel/_loop.py:440-453` |
| **Async model sync misuse** | `create_react_agent` with async `model(state, runtime)` invoked synchronously raises `RuntimeError` at `call_model` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:664-670`), not at compile — late failure of planning element. | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:664-670`, `747-752` |
| **Graph validation gap for Command goto** | `Command(goto="planner")` destinations inferred via `get_type_hints` literal inspection (`libs/langgraph/langgraph/graph/state.py:838-857`) but user can return arbitrary strings at runtime; `validate()` only catches known `ends` tuples, not runtime-constructed `Command`. | `libs/langgraph/langgraph/graph/state.py:838-865` |
| **No plan conflict resolution** | Parallel `Send` tool calls may write same state key concurrently; reducer (`Annotated[list, reducer]`) resolves by last-wins or user reducer, not planner priority. Multiple `TASKS` Sends fan-out deterministically by `task_path_str` sort, not by plan priority. | `libs/langgraph/langgraph/pregel/_algo.py:254-256`, `315-324` |

## Future Considerations

- **First-class `Plan` channel/type:** Introduce `Plan` TypedDict/Pydantic model with steps, dependencies, and status (e.g., `todo_list` middleware referenced in `docs/redirects.json:190`) as a `DeltaChannel` or `BinaryOperatorAggregate` so plan becomes visible, diffable, checkpoint-replayable payload rather than implicit `messages` history. Would map naturally to existing `DeltaChannel` machinery used for `messages`.
- **Planner node library in `prebuilt`:** Ship `create_planner_agent` or `plan_node_factory` wrapping `create_react_agent` with structured-output planning (e.g., `response_format` tuple at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:373-397`) and execution dispatch via `Send`. Leverage existing `generate_structured_response` path (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:744-785`) to make plan generation explicit and tested.
- **Plan-aware interrupts:** Currently interrupts are generic `INTERRUPT` channel writes (`libs/langgraph/langgraph/pregel/_loop.py:818-845`). Planning interrupts (approve/critique/replan) could be typed `Interrupt[Plan]` with resume map keyed by step id (`_xxhash_str` at `libs/langgraph/langgraph/pregel/_algo.py:1404-1409`), enabling per-step human editing.
- **Observable plan stream mode:** Existing `stream_mode` values (`values`, `updates`, `messages`, `tasks`) (`libs/langgraph/langgraph/pregel/main.py:708-709`) could gain `plan`/`steps` mode via `StreamTransformer` pattern (`libs/langgraph/langgraph/stream/transformers.py`), making decomposition visible without reading full checkpoint.
- **Migrate `deepagents` planning back as optional dependency:** Keep core minimal but vendor `deepagents` planning primitives as `langgraph.prebuilt.planning` to close the reusability gap while preserving separation.

## Questions / Gaps

- No planner prompt, planner agent, workflow-graph plan node, or `Task` decomposition code exists in `libs/langgraph` or `libs/prebuilt` beyond ReAct routing — search across `libs/` for `planner` yields only test mocks (`libs/langgraph/tests/test_pregel.py:5171`, `...test_pregel_async.py:6426`). Any planning claims must rely on external `deepagents` or user code.
- `examples/plan-and-execute/plan-and-execute.ipynb` is a redirect stub (8 lines, line 8) — implementation lost; cannot assess prior durability or failure handling.
- No dedicated planning tests: `libs/langgraph/tests/test_pregel.py` and `libs/prebuilt/tests/test_react_agent.py` verify orchestration (conditional edges, tool distribution, `remaining_steps`) but not decomposition quality or replanning after failure.
- Visibility of dynamic plan via `get_state(subgraphs=True)` and interrupt/resume (`libs/langgraph/langgraph/pregel/_loop.py:1375-1390`) is documented for HITL but not evaluated for long-horizon plan editing.
- Reusability claim limited to subgraph embedding; no evidence of cross-graph plan templates or serialized `Plan` JSON that survives `langgraph/prebuilt` version upgrades (`AgentState` deprecation notices at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-108` suggest churn).

---

Generated by `06.01-planning-location-and-responsibility` against `langgraph`.
