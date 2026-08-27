# Source Analysis: langgraph

## Dimension 06.02: Task Decomposition Representation

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (Pregel engine + StateGraph builder + Functional API) |
| Analyzed | 2026-08-27 |

## Summary

LangGraph has no first-class `Plan`/`Todo`/`Subtask` schema with typed fields, status enums, or dependency edges. Task decomposition is represented as a **graph of actor nodes communicating over typed channels**, executed via the Pregel Bulk-Synchronous-Parallel (BSP) loop (`libs/langgraph/langgraph/pregel/main.py:464-478`). Decomposition patterns are: (1) static graph construction (`StateGraph.add_node/add_edge/add_conditional_edges` in `libs/langgraph/langgraph/graph/state.py:667`), (2) dynamic fan-out via `Send` objects (`libs/langgraph/langgraph/types.py:704`) written to the reserved `TASKS` Topic channel (`libs/langgraph/langgraph/pregel/main.py:806`), and (3) functional-API futures via `@task`/`@entrypoint` (`libs/langgraph/langgraph/func/__init__.py:59`, `libs/langgraph/langgraph/pregel/_call.py:258`). Executable subtasks are materialized as `PregelExecutableTask` (`libs/langgraph/langgraph/types.py:666`) scheduled each superstep by `prepare_next_tasks` (`libs/langgraph/langgraph/pregel/_algo.py:392`). Status is tracked externally through `StateSnapshot.tasks` / `PregelTask` (`libs/langgraph/langgraph/types.py:637`, `683`) and stream modes `tasks`/`debug` (`libs/langgraph/langgraph/pregel/debug.py:41`, `106`), but there is no hierarchical plan object that can be atomically evaluated or reordered as a first-class artifact.

## Rating

**7/10** — Clear, tested model with explicit interfaces and operational safeguards for graph-based decomposition; fragile/inconsistent for plan-level decomposition (no typed plan schema, dependency tracking is implicit via channels, and reordering/evaluation require manual state mutation).

Rationale: static and dynamic decomposition are well-defined (node specs, channel triggers, `Send`, `Command`, functional `Call`), dependencies are deterministically resolved and observable via checkpoint tasks, and `StateSnapshot` answers "which subtask is blocking?" via `error`/`interrupts`/`result` and pending writes. Tests (`libs/langgraph/tests/test_pregel.py:5165`, `libs/langgraph/tests/test_pregel_async.py:6420`) show the pattern but reveal that multi-step planning is ad-hoc Dict-in-state, not framework-typed. Missing plan/todo schema, no typed subtask taxonomy, and no plan-update tooling cap the score below 9.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan/task schema (node spec) | `StateNodeSpec` dataclass with `runnable`, `input_schema`, `retry_policy`, `cache_policy`, `error_handler_node`, `ends`, `defer`, `timeout`, `trace_policy` — the static node contract that doubles as task template | `libs/langgraph/langgraph/graph/_node.py:90-103` |
| Runtime node container | `PregelNode` with `channels`, `triggers`, `bound`, `writers`, `retry_policy`, `cache_policy`, `timeout`, `subgraphs`, `flat_writers`, `node` — execution-time node actor | `libs/langgraph/langgraph/pregel/_read.py:97-199` |
| Task object (snapshot) | `PregelTask(id, name, path, error, interrupts, state, result)` — immutable record returned in snapshots; `StateSnapshot.tasks: tuple[PregelTask,...]` | `libs/langgraph/langgraph/types.py:637-646`, `libs/langgraph/langgraph/types.py:683-702` |
| Executable task | `PregelExecutableTask(name, input, proc, writes, config, triggers, retry_policy, cache_key, id, path, writers, subgraphs, timeout)` — scheduled unit with deque `writes` and per-task `config[CONFIG_KEY_SEND/READ]` | `libs/langgraph/langgraph/types.py:666-681` |
| Checkpoint task (persisted) | `CheckpointTask(id, name, error?, result?, interrupts?, state)` inside `CheckpointPayload(tasks: list[CheckpointTask])` — durability layer for tasks | `libs/langgraph/langgraph/types.py:182-220` |
| Dependency graph via channels | `StateGraph.edges`, `waiting_edges`, `branches: dict[str, dict[str, BranchSpec]]`, compiled `Pregel.trigger_to_nodes`; `_triggers()` compares `checkpoint["channel_versions"]` vs `versions_seen` per node | `libs/langgraph/langgraph/graph/state.py:202-209`, `libs/langgraph/langgraph/pregel/main.py:755-756`, `libs/langgraph/langgraph/pregel/_algo.py:1260-1277` |
| Fan-out primitive | `class Send(node, arg, timeout)` with hash/equality; docstring shows `continue_to_jokes` map-reduce pattern; reserved `TASKS = Topic(Send)` channel | `libs/langgraph/langgraph/types.py:704-792`, `libs/langgraph/langgraph/pregel/main.py:806-809` |
| Conditional branch | `BranchSpec(path, ends, input_schema)` + `from_path` Literal-inference + `run(writer,reader)->ChannelWrite` | `libs/langgraph/langgraph/graph/_branch.py:83-122` |
| Command (dynamic re-route + state update) | `Command(graph, update, resume, goto: Send|Sequence[Send|N]|N)` with `_update_as_tuples()` | `libs/langgraph/langgraph/types.py:798-849` |
| Functional decomposition | `_TaskFunction(func, retry_policy, cache_policy, timeout)` and `task()` decorator returning `SyncAsyncFuture`; `call()` delegates to `config[CONF][CONFIG_KEY_CALL]` | `libs/langgraph/langgraph/func/__init__.py:59-94`, `libs/langgraph/langgraph/func/__init__.py:132-251`, `libs/langgraph/langgraph/pregel/_call.py:258-298` |
| Functional push task scheduling | `prepare_push_task_functional` and `prepare_push_task_send` create `PUSH` tasks from `Call` or `Send`; `Call` carries `retry_policy, cache_policy, callbacks, timeout` | `libs/langgraph/langgraph/pregel/_algo.py:800-936`, `libs/langgraph/langgraph/pregel/_algo.py:938-1107` |
| Task scheduling loop | `prepare_next_tasks(checkpoint, pending_writes, processes, channels...)` merges PUSH (Send/Call) and PULL (channel-triggered) tasks; `prepare_single_task` creates id via `xxhash/uuid5` over `checkpoint_id + ns + step + name + PUSH/PULL + triggers` | `libs/langgraph/langgraph/pregel/_algo.py:391-513`, `libs/langgraph/langgraph/pregel/_algo.py:524-761` |
| Status / blocking signal | `tasks_w_writes()` merges `pending_writes` channels `RETURN/ERROR/INTERRUPT` into `PregelTask.error/interrupts/result`; `StateSnapshot.next` lists runnable names without writes = pending | `libs/langgraph/langgraph/pregel/debug.py:209-279`, `libs/langgraph/langgraph/pregel/main.py:1257-1266` |
| Observability streams | `map_debug_tasks` → `TaskPayload(id,name,input,triggers,metadata)` and `map_debug_task_results` → `TaskResultPayload(id,name,error,interrupts,result)`; `map_debug_checkpoint` emits `next` + `tasks` | `libs/langgraph/langgraph/pregel/debug.py:41-71`, `libs/langgraph/langgraph/pregel/debug.py:106-128`, `libs/langgraph/langgraph/pregel/debug.py:144-206` |
| Tick lifecycle & ordering | `PregelLoop.tick()` builds `tasks` dict, `apply_writes` sorts tasks by `task_path_str(path[:3])` for deterministic write order; `after_tick` commits checkpoint and bumps `step` | `libs/langgraph/langgraph/pregel/_loop.py:599-726`, `libs/langgraph/langgraph/pregel/_algo.py:253-256` |
| Reorder / time-travel | `get_state(patch_checkpoint_map(saved.config))`, `update_state` via `create_checkpoint_plan_for_update_state_api`, fork handling `source=="fork"` | `libs/langgraph/langgraph/pregel/main.py:1392-1435`, `libs/langgraph/langgraph/pregel/_checkpoint.py:117`, `libs/langgraph/langgraph/pregel/_loop.py:956-972` |
| Example of ad-hoc plan | `test_multistep_plan` stores `plan: list[str|list[str]]` in `State` and manually dispatches via `Command(goto=next_step, update={"plan": next_plan})` — no framework plan type | `libs/langgraph/tests/test_pregel.py:5165-5220`, `libs/langgraph/tests/test_pregel_async.py:6420-6475` |
| Declarative graph DSL | `add_node`, `add_edge`, `add_conditional_edges`, `add_sequence`, `validate`, `compile(checkpointer, interrupt_before/after)` | `libs/langgraph/langgraph/graph/state.py:667-926`, `libs/langgraph/langgraph/graph/state.py:982-1176` |
| Low-level builder | `NodeBuilder.subscribe_only/subscribe_to/read_from/do/write_to/build()->PregelNode` | `libs/langgraph/langgraph/pregel/main.py:206-376` |
| Write path | `ChannelWrite.do_write`, `ChannelWriteEntry(channel, value, skip_none, mapper)`, `_assemble_writes` handling `Send→(TASKS, Send)` | `libs/langgraph/langgraph/pregel/_write.py:46-192` |

## Answers to Dimension Questions

**1. How is a task decomposed?**

LangGraph decomposes work as a **directed graph of nodes (actors) plus dynamic futures**, not as a hierarchical plan object.

* **Static decomposition (StateGraph):** Builder declares nodes via `StateGraph.add_node(name, action, input_schema, retry_policy, cache_policy, timeout)` (`libs/langgraph/langgraph/graph/state.py:667`) and edges via `add_edge` / `add_conditional_edges` / `waiting_edges` (barrier: `add_edge([a,b], c)` waits for ALL in list, `libs/langgraph/langgraph/graph/state.py:928-980`) plus `BranchSpec` routing (`libs/langgraph/langgraph/graph/_branch.py:83`). Compiled into `Pregel(nodes: dict[str,PregelNode], channels: dict[str,BaseChannel])` (`libs/langgraph/langgraph/pregel/main.py:758-806`).
* **Dynamic decomposition (Pregel-level `Send`):** A node returns `Command(goto=[Send("worker", {"subject": s}) ...])` or writes `Send` to the `TASKS` topic (`libs/langgraph/langgraph/types.py:737`, `libs/langgraph/langgraph/pregel/_write.py:179`). Each `Send` materializes as a `PUSH` task in the next superstep (`libs/langgraph/langgraph/pregel/_algo.py:441-466`, `libs/langgraph/langgraph/pregel/_algo.py:938-1000`). Enables map-reduce fan-out (example docstring at `libs/langgraph/langgraph/types.py:723-748`).
* **Functional decomposition:** Inside an `@entrypoint`, `@task`-decorated functions are called; each call creates a `Call` object holding `(func, (args,kwargs), retry_policy, cache_policy, timeout, callbacks)` (`libs/langgraph/langgraph/pregel/_algo.py:120-152`) futures (`libs/langgraph/langgraph/pregel/_call.py:258`, `libs/langgraph/langgraph/func/__init__.py:86-94`). The parent task can invoke multiple `task` futures then `.result()` / await them, and the engine creates `PUSH` tasks via `prepare_push_task_functional` (`libs/langgraph/langgraph/pregel/_algo.py:800`).
* **Prebuilt agent layer:** No dedicated plan schema either; `create_react_agent` wires agent node + `ToolNode` + `tools_condition` branch (`libs/prebuilt/langgraph/prebuilt/__init__.py:1-23`) — decomposition is agent loop over tool calls, not plan steps.

There is **no framework `Plan` type**; the canonical "multistep plan" test stores a raw list in state and manually loops through it with `Command` (`libs/langgraph/tests/test_pregel.py:5165-5188`).

**2. Are subtasks typed?**

Partially.

* Node/task signatures are typed: `StateNode` union covers `state`, `config`, `writer`, `store`, `runtime` variants (`libs/langgraph/langgraph/graph/_node.py:22-87`), and each node has an `input_schema` (`libs/langgraph/langgraph/graph/_node.py:94`, `libs/langgraph/langgraph/graph/state.py:813-835`). Channel `ValueType`/`UpdateType` propagate through `BaseChannel`.
* But **subtask identity is string-named (`task.name`)**, not an enum/class discriminator for plan-step kinds. `PregelTask.name` is just the node name; `PregelExecutableTask.name` likewise. There is no `SubtaskType` / `status enum` for "research" vs "write" vs "critique". Functional `task` names derive from `func.__name__` (`libs/langgraph/langgraph/func/__init__.py:80`, `libs/langgraph/langgraph/pregel/_call.py:200-212`). Users can supply `metadata` (`libs/langgraph/langgraph/graph/state.py:381`) and it surfaces in `TaskPayload.metadata` (`libs/langgraph/langgraph/types.py:144`, `libs/langgraph/langgraph/pregel/debug.py:60-71`), which is the only typed-ish slot for subtask kind.
* `Send` is generic `Send(node: str, arg: Any)` — `arg` is `Any`, not typed by subgraph input schema at static-check time beyond runtime `TypeAdapter` validation.

Verdict: node/channel contracts are typed; plan-step/subtask taxonomy is not — ad-hoc.

**3. Can dependencies be represented?**

Yes, but **implicitly via channel triggers**, not as an explicit DAG edge list on a plan object.

* Each `PregelNode.triggers: list[str]` declares which channel writes activate it (`libs/langgraph/langgraph/pregel/_read.py:107`). Compile builds `trigger_to_nodes: Mapping[str, Sequence[str]]` (`libs/langgraph/langgraph/pregel/main.py:755-756`, `validate()` at `libs/langgraph/langgraph/pregel/main.py:933-948`). `prepare_next_tasks` uses `updated_channels ∩ trigger_to_nodes` fast-path (`libs/langgraph/langgraph/pregel/_algo.py:475-482`) and fallback `_triggers()` version comparison (`libs/langgraph/langgraph/pregel/_algo.py:1260-1277`).
* Barrier dependencies via `waiting_edges: set[tuple[tuple[str,...], str]]` (`libs/langgraph/langgraph/graph/state.py:209`) — requires ALL listed predecessors; normal `edges` is single-predecessor sequential.
* Dynamic dependencies via `Send` are **ordered by superstep**: PUSH tasks are only dequeued in `step+1` (`libs/langgraph/langgraph/pregel/_algo.py:963` comment "executed in superstep n+1"). Functional `Call` futures are `PUSH` tasks bound to the parent path (`libs/langgraph/langgraph/pregel/_algo.py:826`).
* No first-class "task A depends on task B" list inside a plan; dependency is mediated by shared state channels and sequential BSP barriers. Graph validation prevents unknown sources/targets (`libs/langgraph/langgraph/graph/state.py:1129-1175`) and `Pregel.validate` checks timeout support etc.

Evaluation: dependencies are robust for graph/BSP semantics but not queryable as a plan DAG artifact.

**4. Are statuses tracked?**

Yes, at task granularity, but status lives in **checkpoint + stream events**, not on a mutable plan object.

* Per-task runtime status: `PregelExecutableTask.writes: deque[tuple[str,Any]]` accumulates; pending writes hold `ERROR`, `INTERRUPT`, `RETURN` (`libs/langgraph/langgraph/pregel/_loop.py:526-548`). `tasks_w_writes` folds these into `PregelTask(error, interrupts, result)` (`libs/langgraph/langgraph/pregel/debug.py:218-279`). `StateSnapshot.tasks` returns that tuple plus `next` (runnable names without writes = still pending) and `interrupts` (`libs/langgraph/langgraph/pregel/main.py:1257-1266`).
* Observability: `stream_mode="tasks"` yields `TaskPayload` start and `TaskResultPayload` end events (`libs/langgraph/langgraph/types.py:144-179`, `libs/langgraph/langgraph/pregel/debug.py:41-71`, `libs/langgraph/langgraph/pregel/debug.py:106-128`); `stream_mode="debug"` yields both plus checkpoints (`libs/langgraph/langgraph/types.py:223-261`). Checkpoints persist `PendingWrite(task_id, channel, value)` for replay.
* No dedicated `status: pending|running|blocked|failed|completed` enum on a plan step; status is inferred from `tasks`/`next`/`error`/`interrupts` and checkpoint metadata `step`, `writes`. Functional tasks report `error` via `ERROR` channel but do not expose `retry attempt` count in `StateSnapshot` (stored internally in `_algo`/`_loop` scratchpad).

Verdict: **Can tell which subtask is blocking** — `snapshot.tasks[i].error is not None` → failed; `interrupts` non-empty → blocked on human; `result is None and error is None` with name in `next` → pending/running next step; pending_writes reveal in-flight.

**5. Can decomposition be evaluated?**

Partially — **graph execution is evaluable; plan structure is not**.

* Runtime evaluation: `get_state(config)` returns `StateSnapshot(values, next, tasks, interrupts)` (`libs/langgraph/langgraph/pregel/main.py:1392`); callers can assert `snapshot.next == (...)`, `len(snapshot.tasks)`, and `tasks[0].result`. Resumption via `Command(resume=...)` and `update_state` (`libs/langgraph/langgraph/pregel/main.py:2016` path) allow partial evaluation/time-travel (fork via `source=="fork"`, `ReplayState`, `libs/langgraph/langgraph/pregel/_loop.py:1050-1074`). Checkpointer `get_state_history` and `astream_events(version="v3")` with scoped `StreamMux` (`libs/langgraph/langgraph/pregel/main.py:398-447`) give transformer-observable evaluation.
* But there is **no plan evaluator** that answers "plan is coherent/complete/covers goal" or scores decomposition quality. The multistep plan test merely checks final `messages` length after looping through four steps (`libs/langgraph/tests/test_pregel.py:5205-5222`); no framework metric for fan-out coverage, error aggregation, or subtask deduplication (besides `Topic` dedup flag).
* Caching (`CachePolicy`, `CacheKey`, `libs/langgraph/langgraph/types.py:520-529`, `libs/langgraph/langgraph/pregel/_algo.py:668-687`) allows evaluation of reuse, but not decomposition soundness.

## Architectural Decisions

| Decision | Why | Consequence | File:Line |
|----------|-----|-------------|-----------|
| Pregel/BSP with channels as single coordination primitive | Unifies static edges, conditional routing, and dynamic Send under versioned channels; enables deterministic supersteps | Simple trigger model; dependencies implicit, not inspectable as DAG artifact; all subtasks share state channels | `libs/langgraph/langgraph/pregel/main.py:464-478`, `libs/langgraph/langgraph/pregel/_algo.py:253-256` |
| Reserved `TASKS=Topic(Send)` channel | Isolates dynamic task queue from user state; guarantees PUSH tasks are consumed exactly once per superstep | Enables unbounded fan-out without mutating user schema; backpressure limited to checkpoint channel_versions | `libs/langgraph/langgraph/pregel/main.py:806-809` |
| `Send` + `Command` as dual dispatch mechanisms | `Send` for data-parallel fan-out, `Command(goto,update)` for imperative control + state patch | Flexible but two overlapping APIs; static analysis must union `ends` + `static` writes (`libs/langgraph/langgraph/pregel/_write.py:136-156`) | `libs/langgraph/langgraph/types.py:704-849` |
| Functional `@task` as `Call` future pushing child tasks onto parent path | Lets Python control flow express decomposition without declaring graph; preserves retry/timeout/cache per subtask | Parent task path encodes ancestry (`(PUSH, parent_path, idx, id, Call)`); debugging requires path parsing (`libs/langgraph/langgraph/pregel/_algo.py:551-572`) | `libs/langgraph/langgraph/func/__init__.py:59-107`, `libs/langgraph/langgraph/pregel/_algo.py:800-826` |
| `PregelTask` immutability vs `PregelExecutableTask` mutability | Snapshot tasks are value types for reasoning; executable tasks carry mutable `writes` deque for collection during tick | Clean checkpointing; but requires `tasks_w_writes` reconcile to present readable snapshot | `libs/langgraph/langgraph/types.py:637-681`, `libs/langgraph/langgraph/pregel/debug.py:209` |
| `StateGraph` compiles to `Pregel` with `trigger_to_nodes` index | Moves edge/branch validation to compile time; enables O(|updated_channels|) scheduling instead of scanning all nodes | Fast `prepare_next_tasks` fast path; compile errors surface as `ValueError` on unknown sources/targets | `libs/langgraph/langgraph/graph/state.py:1129-1210`, `libs/langgraph/langgraph/pregel/_algo.py:475-482` |
| No plan object | Keeps core minimal (nodes + channels + tasks); users model plans as state fields | Shifts plan correctness to user code; framework cannot validate plan completeness | `libs/langgraph/tests/test_pregel.py:5165-5188` |

## Notable Patterns

* **BSP actor pattern:** `PregelNode` subscribes to channels, BSP loop `tick()` → `prepare_next_tasks()` → runner executes `PregelExecutableTask` → `apply_writes()` → `after_tick()` checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:599-726`). Maps directly to Pregel paper.
* **Dual PUSH/PULL scheduling:** `TASKS` channel PUSH tasks (Send/Call/error-handler) merged with PULL channel-triggered tasks into single `dict[str, PregelExecutableTask]` (`libs/langgraph/langgraph/pregel/_algo.py:437-513`). Enables both data-parallel and data-driven subtasks in same superstep.
* **Task ID = checkpoint_id ⊕ namespace ⊕ step ⊕ name ⊕ trigger:** deterministic `xxhash_str`/`uuid5_str` (`libs/langgraph/langgraph/pregel/_algo.py:1395-1410`, call sites `libs/langgraph/langgraph/pregel/_algo.py:615-623`, `libs/langgraph/langgraph/pregel/_algo.py:834-843`) guarantees idempotency across replays.
* **Barrier via `waiting_edges`:** `StateGraph.add_edge([a,b], c)` stores `(("a","b"), "c")` and expands in `_all_edges` (`libs/langgraph/langgraph/graph/state.py:338-341`); scheduler requires all listed predecessors via `trigger_to_nodes` intersection.
* **ChannelWrite mapper indirection:** `ChannelWriteEntry(channel, value=PASSTHROUGH, mapper)` allows node output transforms before write (`libs/langgraph/langgraph/pregel/_write.py:26-34`); `PregelNode.flat_writers` dedupes consecutive writes (`libs/langgraph/langgraph/pregel/_read.py:209-224`).
* **Scratchpad per task:** `PregelScratchpad(step, stop, call_counter, interrupt_counter, resume, get_null_resume, subgraph_counter)` injected via `CONFIG_KEY_SCRATCHPAD` (`libs/langgraph/langgraph/pregel/_algo.py:1280-1346`) — enables `interrupt()` inside any node/task (`libs/langgraph/langgraph/types.py:851-974`).
* **StreamMux scoped transformers:** `version="v3"` stream owns `stream_mode`/`subgraphs` invariants and forces transformer factories `scope->StreamTransformer` (`libs/langgraph/langgraph/pregel/main.py:379-447`), allowing per-namespace subtask event routing.

## Tradeoffs

* **Expressiveness vs analysis:** BSP + channels supports arbitrary decomposition (fan-out, loops, conditionals, error handlers) but dependency graph is distributed across `triggers`, `waiting_edges`, `BranchSpec.ends`, and runtime `Send`/`Command` writes — no single DAG artifact to lint or reorder offline.
* **Dynamicism vs static guarantees:** `Send`/`Call` enable unbounded parallelism, but `packet.node not in processes → warning + drop` (`libs/langgraph/langgraph/pregel/_algo.py:977-979`) is best-effort; graph rendering (`destinations` hint, `libs/langgraph/langgraph/graph/state.py:404-414`) is advisory only ("doesn't have any effect on execution").
* **State-central vs plan-central:** All subtask coordination is state-channel mediated. Simplicity (one mechanism) trades off against plan-level concerns: no typed subtask list, no cross-subtask constraint validation, no plan diff/update tool.
* **Checkpoint fidelity vs throughput:** Every tick optionally persists `PendingWrite` via `checkpointer.put_writes` and full checkpoint via `put_after_previous` (`libs/langgraph/langgraph/pregel/_loop.py:415-546`); durability knob `sync`/`async`/`exit` (`libs/langgraph/langgraph/types.py:89`) and `stream_eager` control latency but add I/O overhead. `delta_channels_with_overwrite` and `counters_since_delta_snapshot` optimize delta-channel snapshot frequency (`libs/langgraph/langgraph/pregel/_loop.py:1111-1140`).
* **Observability richness vs ergonomics:** `tasks`/`debug`/`checkpoints` modes expose every subtask input/result/error, but consumers must correlate `task.id` ↔ `task.id` across `TaskPayload`/`TaskResultPayload`/`CheckpointTask` (`libs/langgraph/langgraph/types.py:144-204`) rather than reading a consolidated plan view.

## Failure Modes / Edge Cases

* **Starved trigger:** Node whose `triggers` channel is never written never schedules; `prepare_next_tasks` silently produces no `PULL` task (`libs/langgraph/langgraph/pregel/_algo.py:605-612` returns `None`). No framework warning for orphan subtask. Mitigated by `validate()` checking `START` reachable (`libs/langgraph/langgraph/graph/state.py:1141-1145`) but not liveness.
* **Ambiguous fan-out target:** `Send(node="unknown", arg=...)` or `Command(goto="unknown")` — `prepare_push_task_send` logs warning and returns `None` (`libs/langgraph/langgraph/pregel/_algo.py:977-978`); execution proceeds without error, silently dropping subtask. No hard fail.
* **Multiple writes to same channel:** Without `BinaryOperatorAggregate`/`Topic(accumulate=True)`, later writes overwrite. With `BinaryOperatorAggregate` + `Overwrite`, multiple `Overwrite` in same superstep raises `InvalidUpdateError` (`libs/langgraph/langgraph/types.py:979`). Duplicate `TASKS` writes handled via `Topic` but ordering depends on `task_path_str` sort (`libs/langgraph/langgraph/pregel/_algo.py:253`).
* **Duplicate parallel inputs:** `TASKS=Topic(Send, accumulate=False)` dedup not enabled by default; multiple identical `Send` objects produce distinct tasks (different idx → different task_id). No subtask dedup.
* **Reordering nondeterminism:** Write application is sorted by `path[:3]` (`libs/langgraph/langgraph/pregel/_algo.py:256`), but `triggered_nodes` candidates sorted alphabetically (`libs/langgraph/langgraph/pregel/_algo.py:482`). No priority or custom ordering hook; subtasks cannot be prioritized.
* **Interrupted parent holds child futures:** `Call` PUSH tasks append `True` to path (`*task_path[:3], True` at `libs/langgraph/langgraph/pregel/_algo.py:846`); interrupts inside child are swallowed (responsibility of parent). If parent interrupted, children may be orphaned until resume flushes `pending_writes`.
* **Error-handler shadowing:** `prepare_node_error_handler_task` creates a `PUSH` handler task that shares step but is skipped on replay via `ERROR_SOURCE_NODE` marker (`libs/langgraph/langgraph/pregel/_loop.py:751-816`); if marker lost, original node re-executes instead of handler.
* **Functional `sync` + `timeout` unsupported:** `task()` raises `sync_timeout_unsupported` for sync functions (`libs/langgraph/langgraph/func/__init__.py:237-239`, `libs/langgraph/langgraph/pregel/_call.py:285-287`); timeout only for async tasks/nodes, blocking sync decomposition.
* **`PASSTHROUGH`/`SKIP_WRITE` leakage:** `ChannelWrite.do_write` validates `PASSTHROUGH` not leaked when `allow_passthrough=False` (`libs/langgraph/langgraph/pregel/_write.py:118-122`); misuse raises `InvalidUpdateError`.
* **Subgraph namespace collision:** `checkpoint_ns = parent_ns + NS_SEP + name + NS_END + task_id` (`libs/langgraph/langgraph/pregel/_algo.py:624-625`); reusing node name across levels without namespacing causes `recast_checkpoint_ns` mismatch on `get_state` lookup (`libs/langgraph/langgraph/pregel/main.py:1404-1416`).

## Future Considerations

* Introduce an optional **typed `Plan`/`Subtask` schema** (e.g., `class Subtask(TypedDict): id, kind: Literal[...], input, depends_on: list[str], status`) persisted as a managed `LastValue` or dedicated channel, with a `plan_update` tool (`Command(update={"plan": ...})` already does ad-hoc updates) — would enable static analysis, reordering, and evaluation metrics without breaking BSP.
* Expose **dependency graph as artifact**: emit `trigger_to_nodes` + `waiting_edges` + observed `Send` destinations in `get_state` metadata so callers can reconstruct the effective subtask DAG for visualization/debugging.
* Add **task priority / reordering API**: allow `Send(priority=...)` or `update_state(next=["taskB","taskA"])` to reorder `next` deterministically; currently `next` order follows `prepare_next_tasks` insertion order.
* Unify **plan evaluation harness**: add `graph.get_plan_metrics(subgraphs=True)` that aggregates `tasks_w_writes` across namespaces (counts pending/failed/interrupted, fan-out width, retry rates) to answer "is decomposition sound?" — today requires manual aggregation over `checkpoints` stream.
* Harden **unknown-target fan-out**: promote warning to `InvalidUpdateError` under `auto_validate`-like flag, and surface in `StateSnapshot` as a failed subtask rather than silent drop.
* Generalize **error-handler routing** to multi-handler dispatch (current single `error_handler_node` per node, `libs/langgraph/langgraph/graph/_node.py:98`) and expose handler status in `StateSnapshot.tasks[i].error` with source node attribution already stored via `ERROR_SOURCE_NODE` (`libs/langgraph/langgraph/pregel/_loop.py:500-504`).

## Questions / Gaps

* No evidence found for a native **plan-update tool** (e.g., `update_plan(subtask_id, status, result)`) — `update_state` exists (`libs/langgraph/langgraph/pregel/_checkpoint.py:117`) but operates on raw channel state, not a plan abstraction.
* No evidence found for **typed subtask taxonomy** beyond `PregelTask.name: str` and free-form `metadata: dict` — search over `libs/langgraph/langgraph/types.py`, `libs/langgraph/langgraph/graph/_node.py`, `libs/langgraph/langgraph/func/__init__.py`, and `libs/langgraph/tests/test_pregel.py:5165` found no `SubtaskKind` enum or `TodoSchema`.
* No evidence found for **plan scoring/evaluation API** — decomposition quality (coverage, redundancy, ordering) is not computed by framework; tests only assert final state values.
* `Send.timeout` per-fan-out task (`libs/langgraph/langgraph/types.py:755`) allows per-subtask timeout override, but interaction with graph-level `interrupt_before/after` ("Can the system tell which subtask is currently blocking progress?" — yes via `interrupts`/`error`, but blocking due to timeout vs human interrupt not distinguished in `StateSnapshot` without inspecting `error` type) needs clarification in docs.
* Search boundary: inspected `libs/langgraph/langgraph/{types,graph,pregel,func}` and `libs/prebuilt` for plan/todo keywords; sibling sources not inspected per isolation rule. No exhaustive run of test suite — static analysis only.

---
Generated by `Dimension 06.02: Task Decomposition Representation` against `langgraph`.
