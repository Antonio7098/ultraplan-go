# Source Analysis: langgraph

## 01.03 Step, Turn, and Task Atomicity

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (asyncio + concurrent.futures, LangChain callbacks), with parallel JS/TS SDK in `libs/sdk-js` |
| Analyzed | 2026-07-02 |

## Summary

LangGraph decomposes a graph run into **three layered atomic units** that are not the same shape for persistence, retry, tracing, and UI:

1. The **superstep (a.k.a. "step" / "tick")** is the smallest **persistable** unit. One tick drives one `_put_checkpoint` call (`libs/langgraph/langgraph/pregel/_loop.py:706`) and writes a `Checkpoint` plus a `pending_writes` row per task (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:31,92`). The step counter lives on `PregelLoop.step` (`libs/langgraph/langgraph/pregel/_loop.py:294`), is bumped at the end of `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:1199`), and is exposed in `CheckpointMetadata.step` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:49-55`).
2. The **`PregelExecutableTask`** is the smallest **retryable** unit. Each task carries its own `retry_policy`, `timeout`, `cache_key`, and `writes: deque` (`libs/langgraph/langgraph/types.py:626-640`). `run_with_retry`/`arun_with_retry` clear `task.writes` before every attempt (`libs/langgraph/langgraph/pregel/_retry.py:615,738`) and bound total attempts via `RetryPolicy.max_attempts` (`libs/langgraph/langgraph/types.py:428`). Failures may also route to a node-level error handler (`libs/langgraph/langgraph/pregel/_runner.py:171-174`, handler map built at `libs/langgraph/langgraph/graph/state.py:1327-1331`).
3. The **`(channel, value)` write** is the smallest **streamable / observable** unit. Tasks commit writes via `put_writes(task_id, writes)` (`libs/langgraph/langgraph/pregel/_loop.py:408`), and the runner's `commit` (`libs/langgraph/langgraph/pregel/_runner.py:574-613`) wraps each write in an `(ERROR | INTERRUPT | NO_WRITES | normal)` envelope. Writes are streamed per task via `TasksStreamPart` (`libs/langgraph/langgraph/types.py:318-330`) and emitted as `UpdatesStreamPart` (`libs/langgraph/langgraph/types.py:274-283`).

The unit of **tracing** (LangChain callback span) is the task attempt: each `PregelExecutableTask` config attaches `langgraph_step`, `langgraph_node`, `langgraph_triggers`, `langgraph_path`, `langgraph_checkpoint_ns` (`libs/langgraph/langgraph/pregel/_algo.py:654-660`) under a `manager.get_child(f"graph:step:{step}")` span (`libs/langgraph/langgraph/pregel/_algo.py:717`). The unit **shown to the user** depends on `stream_mode`: `values`/`updates` collapse to a step, `tasks`/`debug` expose individual tasks, `checkpoints` expose the checkpoint (`libs/langgraph/langgraph/types.py:120-134`).

Partial completion is explicit and recoverable. A tick can crash with in-flight `checkpoint_pending_writes` (`libs/langgraph/langgraph/pregel/_loop.py:245`) and still recover because `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:724-737`) restores successful channel writes from the prior checkpoint while routing failed-task recovery to either re-execution or the node error handler (`libs/langgraph/langgraph/pregel/_loop.py:739-804`). Durability is a first-class, configurable axis (`Durability = Literal["sync", "async", "exit"]`, `libs/langgraph/langgraph/types.py:87-93`): `sync` waits for the in-flight `put` future between ticks (`libs/langgraph/langgraph/pregel/main.py:3002-3003,3476-3477`), `async` overlaps the next tick, `exit` flushes only on `__exit__` via `_put_exit_delta_writes` and the exit call to `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:1201-1311`). The system **can say exactly what completed and what did not**: every checkpoint is paired with `pending_writes` for in-flight tasks, and the ordering invariant is enforced by `_checkpointer_put_after_previous` (drains `prev.result()` and `_delta_write_futs` before issuing the next `put`) at `libs/langgraph/langgraph/pregel/_loop.py:1507-1524,1759-1778`.

## Rating

**8/10** — Clear, multi-tier atomic model with explicit types (`Checkpoint`, `PendingWrite`, `PregelExecutableTask`, `CheckpointMetadata`), explicit durability axis, ordered put-after-previous drain semantics, delta-channel snapshot bookkeeping, and a layered retry/error-handler scheme. Not 9–10 because: (a) the persisted atomic unit (superstep + checkpoint) does not coincide with the retried unit (task attempt) — partial writes can sit in `checkpoint_pending_writes` while the next tick begins, with safety delegated to in-memory futures ordering rather than a transactional boundary; (b) `tool calls` inside a node are not first-class atomic units — they ride along inside the task's `writes` deque and inherit the task's retry budget via `RetryPolicy` rather than a per-tool-call budget; (c) the streaming surface area has grown to seven modes (`values`, `updates`, `checkpoints`, `tasks`, `debug`, `messages`, `custom`) with two protocol versions (`v1`/`v2`), making the "what is shown to the user" question mode-dependent.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Superstep / "step" counter (persisted unit) | `PregelLoop.step: int` initialized to 0, bumped in `_put_checkpoint` | `libs/langgraph/langgraph/pregel/_loop.py:294,1199` |
| Superstep is what triggers a checkpoint | `tick()` → runner → `after_tick()` → `_put_checkpoint({"source": "loop"})` | `libs/langgraph/langgraph/pregel/_loop.py:706` |
| `recursion_limit` bounds supersteps; raises `GraphRecursionError` | "Recursion limit of {config['recursion_limit']} reached" | `libs/langgraph/langgraph/pregel/main.py:3018-3026,3501-3508` |
| `step_timeout` bounds wall-clock per step | `timeout=self.step_timeout` passed to runner | `libs/langgraph/langgraph/pregel/main.py:2984,3457` |
| Checkpoint type — persistence unit | `Checkpoint` TypedDict with `id`, `ts`, `channel_values`, `channel_versions`, `versions_seen`, `pending_sends`, `updated_channels` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-123` |
| `CheckpointMetadata` records step + source | `step: int`, `source: Literal["input","loop","update","fork"]` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-55` |
| `PendingWrite` — sub-checkpoint persisted unit | `PendingWrite = tuple[str, str, Any]` stored per `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:31`; `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:497-509` |
| `put_writes` is the writer's API for in-flight writes | `def put_writes(self, config, writes, task_id, task_path="")` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:300-318` |
| `put` is the API for the checkpoint itself | `def put(self, config, checkpoint, metadata, new_versions)` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:277-298` |
| `PregelExecutableTask` — retryable unit | dataclass with `name`, `input`, `proc`, `writes`, `config`, `triggers`, `retry_policy`, `cache_key`, `id`, `path`, `timeout` | `libs/langgraph/langgraph/types.py:626-640` |
| Task ID is deterministic (xxh3 hash of step + name + triggers + ns) | `_xxhash_str` for `checkpoint["v"] > 1` | `libs/langgraph/langgraph/pregel/_algo.py:550,616-623,1404-1409` |
| `RetryPolicy` shape | `initial_interval`, `backoff_factor`, `max_interval`, `max_attempts`, `jitter`, `retry_on` | `libs/langgraph/langgraph/types.py:416-435` |
| Default `retry_on` heuristic | `default_retry_on` retries on connection/5xx, not on value/type/runtime etc. | `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Sync retry wrapper — per-attempt write clearing | `task.writes.clear()` then `task.proc.invoke(task.input, config)` | `libs/langgraph/langgraph/pregel/_retry.py:615-617` |
| Async retry wrapper with timeout scope | `arun_with_retry` clears writes, runs `ainvoke`, has `_arun_with_timeout` with run/idle watchdogs | `libs/langgraph/langgraph/pregel/_retry.py:685-838,422-517` |
| Atomic attempt context for timed runs | `_AttemptContext` + `_AttemptEvent` (`start`/`progress`/`finish`) | `libs/langgraph/langgraph/pregel/_retry.py:87-126` |
| Node error handler routing | `node_error_handler_map` from `spec.error_handler_node`; runner routes via `schedule_error_handler` | `libs/langgraph/langgraph/graph/state.py:1327-1331`; `libs/langgraph/langgraph/pregel/_runner.py:171-174,1549-1584,1803-1838` |
| Trace span per task attempt | `langgraph_step`, `langgraph_node`, `langgraph_triggers`, `langgraph_path`, `langgraph_checkpoint_ns` in `task.config["metadata"]` | `libs/langgraph/langgraph/pregel/_algo.py:654-660,702-705` |
| Span name format | `manager.get_child(f"graph:step:{step}")` | `libs/langgraph/langgraph/pregel/_algo.py:717` |
| `ExecutionInfo` on `Runtime` for tracing | `checkpoint_id`, `checkpoint_ns`, `task_id`, `thread_id`, `run_id`, `node_attempt` | `libs/langgraph/langgraph/pregel/_algo.py:694-700`; `libs/langgraph/langgraph/pregel/_retry.py:694-612` |
| Stream modes | `Literal["values", "updates", "checkpoints", "tasks", "debug", "messages", "custom"]` | `libs/langgraph/langgraph/types.py:120-134` |
| Stream parts carry `step`/`ns` | `ValuesStreamPart`, `UpdatesStreamPart`, `CheckpointStreamPart`, `TasksStreamPart`, `DebugStreamPart` all include `step`/`ns`/`data` | `libs/langgraph/langgraph/types.py:262-339` |
| TasksStreamPart — one part per task | `TaskPayload` for start, `TaskResultPayload` for finish | `libs/langgraph/langgraph/types.py:142-178,318-330` |
| DebugStreamPart — wrapper with `step`, `type` discriminator | `_DebugCheckpointPayload` / `_DebugTaskPayload` / `_DebugTaskResultPayload` | `libs/langgraph/langgraph/types.py:221-258` |
| `_emit` writes to stream channels; debug mode remaps checkpoints/tasks | `step: self.step - 1` for checkpoint events, `self.step` for tasks | `libs/langgraph/langgraph/pregel/_loop.py:1357-1391` |
| `runner.tick`/`runner.atick` execute tasks with `get_waiter` and timeout | `tick()` yields per future completion; `atick()` analogous | `libs/langgraph/langgraph/pregel/_runner.py:176-358,360-572` |
| Task commit — writes + ERROR/INTERRUPT/NO_WRITES envelopes | `commit()` appends `(ERROR, exc)`, `(INTERRUPT, ...)`, `(NO_WRITES, None)` | `libs/langgraph/langgraph/pregel/_runner.py:574-613` |
| `put_writes` loop-path called from runner commit | `self.put_writes()(task.id, task.writes)` | `libs/langgraph/langgraph/pregel/_runner.py:583,591,603,613` |
| Sub-step call (functional API) — `Call` is its own atomic | `_call`/`_acall` schedule next task with `__next_tick__=True` | `libs/langgraph/langgraph/pregel/_runner.py:700-786,789-841`; `libs/langgraph/langgraph/pregel/_algo.py:120-153` |
| Cache key per task — dedup on retry/resume | `CacheKey(ns, key, ttl)` used in `match_cached_writes` | `libs/langgraph/langgraph/types.py:615-623`; `libs/langgraph/langgraph/pregel/_loop.py:1526-1539,1780-1793` |
| Pending writes buffer (in-flight, per tick) | `self.checkpoint_pending_writes: list[PendingWrite]` | `libs/langgraph/langgraph/pregel/_loop.py:245` |
| Replay on resume: restore successful task writes | `_reapply_writes_to_succeeded_nodes` skips `ERROR`/`ERROR_SOURCE_NODE`/`INTERRUPT`/`RESUME` | `libs/langgraph/langgraph/pregel/_loop.py:724-737` |
| Error-handler resume from partial failure | `_resume_error_handlers_if_applicable` scans `ERROR_SOURCE_NODE` markers | `libs/langgraph/langgraph/pregel/_loop.py:739-804` |
| In-order put-after-previous drain (durability ordering) | `_checkpointer_put_after_previous` awaits previous future, then puts | `libs/langgraph/langgraph/pregel/_loop.py:1507-1524,1759-1778` |
| Delta-channel writes durable before checkpoint | `_delta_write_futs` drained before each `_put_checkpoint` | `libs/langgraph/langgraph/pregel/_loop.py:200-205,488-491,1515-1517,1769-1771` |
| Error-handler write durability | `_error_handler_write_futs` drained before handler runs | `libs/langgraph/langgraph/pregel/_loop.py:206-211,495-498,1556-1558,1810-1812` |
| Exit-mode delta-write stub | `_put_exit_delta_writes` synthesizes a stub checkpoint if no parent | `libs/langgraph/langgraph/pregel/_loop.py:1201-1292` |
| `Durability` literal — sync/async/exit | `Durability = Literal["sync", "async", "exit"]` | `libs/langgraph/langgraph/types.py:87-93` |
| `sync` durability waits between ticks | `await loop._put_checkpoint_fut` / `await cast(asyncio.Future, loop._put_checkpoint_fut)` | `libs/langgraph/langgraph/pregel/main.py:3002-3003,3476-3477` |
| `interrupt()` halts a node and writes `INTERRUPT`/`RESUME` | `_interrupt(value)` raises `GraphInterrupt` after recording a sentinel | `libs/langgraph/langgraph/types.py:811-934` |
| Reserved write keys — write-type discriminator | `INPUT`, `INTERRUPT`, `RESUME`, `ERROR`, `ERROR_SOURCE_NODE`, `NO_WRITES`, `TASKS`, `RETURN`, `PREVIOUS`, `OVERWRITE` | `libs/langgraph/langgraph/_internal/_constants.py:7-25,95` |
| `WRITES_IDX_MAP` — negative indices for special writes | `ERROR: -1, SCHEDULED: -2, INTERRUPT: -3, RESUME: -4` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| Null task ID for non-task writes | `NULL_TASK_ID = "00000000-0000-0000-0000-000000000000"` | `libs/langgraph/langgraph/_internal/_constants.py:93` |

## Answers to Dimension Questions

### 1. What is the atomic unit of execution?

Three layered units, in increasing size:
- **Write**: `(channel, value)` tuple (e.g. `("messages", [...])`) — the smallest unit the system persists and surfaces. Carried inside a `task.writes: deque` (`libs/langgraph/langgraph/types.py:631`) and committed via `put_writes(task_id, writes)` (`libs/langgraph/langgraph/pregel/_loop.py:408`).
- **Task** (`PregelExecutableTask`, `libs/langgraph/langgraph/types.py:626-640`): one execution of one node, identified by a deterministic hash ID (`libs/langgraph/langgraph/pregel/_algo.py:550,616-623`), with its own retry/timeout/cache policies. This is the unit the runner schedules (`libs/langgraph/langgraph/pregel/_runner.py:135-358`) and the unit that emits `TasksStreamPart`/`DebugStreamPart` (`libs/langgraph/langgraph/types.py:318-339`).
- **Superstep** (Pregel tick): one batch of parallel task executions plus the channel update that ends the batch (`libs/langgraph/langgraph/pregel/_loop.py:592-674,676-714`). The superstep is the persistence unit: `_put_checkpoint({"source": "loop"})` at `libs/langgraph/langgraph/pregel/_loop.py:706` is the *only* place the loop writes a checkpoint for a regular tick.

### 2. Is the atomic unit the same for persistence, tracing, retry, and UI?

No. The units are deliberately tiered:
- **Persistence**: superstep → `Checkpoint` + `pending_writes` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:31,92-123`).
- **Retry**: `PregelExecutableTask` attempt (`libs/langgraph/langgraph/pregel/_retry.py:573-838`). Node-level error handlers add a second tier (`libs/langgraph/langgraph/pregel/_runner.py:171-174`).
- **Tracing**: per-task attempt via LangChain callback manager; each attempt gets a `langgraph_step`, `langgraph_node`, `langgraph_path`, etc. metadata block (`libs/langgraph/langgraph/pregel/_algo.py:654-660`) under `manager.get_child(f"graph:step:{step}")` (`libs/langgraph/langgraph/pregel/_algo.py:717`).
- **UI**: depends on `stream_mode` (`libs/langgraph/langgraph/types.py:120-134`):
  - `values`/`updates` — show the superstep's results.
  - `tasks` — show individual task start/finish.
  - `debug` — show both checkpoint events and task events, each tagged with `step` and a `type` discriminator (`libs/langgraph/langgraph/types.py:221-258`).

### 3. Can partially completed steps exist?

Yes, partial steps are normal and recoverable. During a tick, in-flight writes sit in `self.checkpoint_pending_writes: list[PendingWrite]` (`libs/langgraph/langgraph/pregel/_loop.py:245`). `after_tick` calls `apply_writes` (atomic per superstep, `libs/langgraph/langgraph/pregel/_loop.py:680-686`) and then persists via `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:706`). On a crash mid-step:
- `put_writes` futures may already be in flight or done (they are submitted to a background executor at `libs/langgraph/langgraph/pregel/_loop.py:474-491`).
- `checkpoint_pending_writes` for the next tick's checkpoint is loaded from the saved row on resume (`libs/langgraph/langgraph/pregel/_loop.py:1657-1661`).
- `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:724-737`) restores writes for succeeded tasks while skipping control writes (`ERROR`, `ERROR_SOURCE_NODE`, `INTERRUPT`, `RESUME`).
- `_resume_error_handlers_if_applicable` (`libs/langgraph/langgraph/pregel/_loop.py:739-804`) routes previously-failed tasks to their error handlers instead of re-executing them.

The atomic *channel* update happens once per tick at `apply_writes`, so channel state never goes through a half-applied state. But the *write table* does: a write can be durable before its parent checkpoint if `durability="async"`, hence the `_delta_write_futs` ordering invariant (`libs/langgraph/langgraph/pregel/_loop.py:200-205,1515-1517,1769-1771`).

### 4. What happens if a crash occurs mid-step?

It is recoverable but uses in-memory ordering rather than a transactional boundary:
- **Before** `apply_writes`: the parent checkpoint is intact; the in-flight `task.writes` are buffered in `checkpoint_pending_writes`. `put_writes` may already have written rows for individual tasks (`libs/langgraph/langgraph/pregel/_loop.py:449-498`). On resume, those rows are re-applied by `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:724-737`).
- **During** `apply_writes`: this is in-memory over the `checkpoint` dict, so a crash leaves the prior checkpoint intact and the new channel state lost. The next `put` future (`_put_checkpoint_fut`, `libs/langgraph/langgraph/pregel/_loop.py:1182-1189,1944-1962`) may or may not have completed.
- **After** `apply_writes` but **before** `put`: the new checkpoint is in `self.checkpoint` but not yet durable; on crash, the prior checkpoint is replayed.
- **After** `put` but with in-flight `put_writes`: the checkpoint is durable but its `pending_writes` may be partial. On resume, the saved `pending_writes` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:139-146`) are loaded and re-applied via `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:1657-1661,724-737`).
- **`durability="exit"`** collapses all of the above to "everything flushes on `__exit__`"; if the process dies before `__exit__`, none of the intermediate checkpoints are durable. Exit mode also synthesizes a stub parent checkpoint for delta-channel writes (`libs/langgraph/langgraph/pregel/_loop.py:1230-1258`).

### 5. Are tool calls their own atomic units?

**No, not as first-class atomic units.** Inside a node, tool calls happen synchronously and ride along with the node's `writes`. The system has these layered mechanisms that *approximate* per-tool atomicity:
- **CachePolicy** (`libs/langgraph/langgraph/types.py:518-527`) can cache a node's writes by key, effectively making the *node output* dedupable across invocations — not the individual tool calls inside it.
- **Functional API `Call` (`libs/langgraph/langgraph/pregel/_algo.py:120-153`)**: a node can call a sub-function via `Call`, which is scheduled as its own task in the **next tick** (`libs/langgraph/langgraph/pregel/_runner.py:758-778,906-931`). The `Call` runs with `__next_tick__=True` so its writes commit only after the current tick's updates stream — it is essentially a deferred sub-task, but it inherits the parent graph's tick boundary.
- **Tool stream mode** (`stream_mode="tools"`) emits per-tool events (`libs/langgraph/langgraph/pregel/_tools.py:35-`), but this is an observability surface, not a durability boundary.
- **`interrupt()` inside a node** (`libs/langgraph/langgraph/types.py:811-934`) can halt the node mid-execution. On resume, the node re-executes from the top, finding the previous `RESUME` write via the scratchpad (`libs/langgraph/langgraph/types.py:914-925`). This makes an `interrupt()` boundary effectively an atomic checkpoint for a node's user code, but only because the surrounding checkpoint holds it together.

The retry budget for tool calls is the **task's** `RetryPolicy.max_attempts` (`libs/langgraph/langgraph/types.py:428`), not a per-tool-call counter.

## Architectural Decisions

- **Bulk Synchronous Parallel model with explicit tick**: The superstep is the indivisible boundary (`libs/langgraph/langgraph/pregel/main.py:2974-2978,3446-3450` — "Similarly to Bulk Synchronous Parallel / Pregel model / computation proceeds in steps, while there are channel updates"). This makes channel updates *between* tasks atomic, but tasks within a tick are concurrent.
- **Deterministic task IDs from `(checkpoint_id, namespace, step, name, type, triggers)`**: `(libs/langgraph/langgraph/pregel/_algo.py:550,616-623,1404-1409)`. Same task inputs always hash to the same id, which is what enables replay/time-travel via `get_state(config={"checkpoint_id": ...})`.
- **Two-tier storage**: `put` for the `Checkpoint` snapshot, `put_writes` for per-task writes indexed by `(task_id, idx)` with negative `WRITES_IDX_MAP` indices for control writes (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:795`; `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:497-509`). This split lets the checkpoint row stay small and writes stream into a separate row that's only read on resume/replay.
- **Durability is a first-class, configurable axis** (`libs/langgraph/langgraph/types.py:87-93`) with three modes (`sync`, `async`, `exit`) and a dedicated exit path (`libs/langgraph/langgraph/pregel/_loop.py:1201-1311`) that synthesizes a stub checkpoint when no parent is persisted yet (`libs/langgraph/langgraph/pregel/_loop.py:1230-1258`).
- **Ordered put-after-previous**: `_checkpointer_put_after_previous` (`libs/langgraph/langgraph/pregel/_loop.py:1507-1524,1759-1778`) ensures checkpoints land in monotonic order and that delta-channel writes become durable before their producing checkpoint is published (`libs/langgraph/langgraph/pregel/_loop.py:200-205,488-491`).
- **Reserved write keys for control-flow**: `INPUT`, `INTERRUPT`, `RESUME`, `ERROR`, `ERROR_SOURCE_NODE`, `NO_WRITES`, `TASKS`, `RETURN`, `OVERWRITE` (`libs/langgraph/langgraph/_internal/_constants.py:7-25,95`). These are the system's only way to express non-channel state across the checkpoint boundary; everything else must go through a regular channel.
- **Null task id for non-task writes**: writes from `map_input` or `Command` without a task context use `NULL_TASK_ID = "00000000-0000-0000-0000-000000000000"` (`libs/langgraph/langgraph/_internal/_constants.py:93`). Persisted separately so they survive the per-task write table's `(task_id, idx)` indexing.
- **Layered retry**: per-task attempt (`run_with_retry`, `libs/langgraph/langgraph/pregel/_retry.py:573-682`) plus node-level error handler routing (`schedule_error_handler`, `libs/langgraph/langgraph/pregel/_runner.py:1549-1584,1803-1838`). The two layers compose: an exhausted retry budget still routes to the error handler before the runner panics.
- **CachePolicy is task-scoped, not tool-scoped** (`libs/langgraph/langgraph/types.py:518-527`; matched via `match_cached_writes` in `libs/langgraph/langgraph/pregel/_loop.py:1526-1539,1780-1793`). This means cached replay is at the node output level.
- **Two-step stream interface (v1 and v2)**: `_V2StreamingCallbackHandler` and v2 `StreamPart` types (`libs/langgraph/langgraph/types.py:341-365`) coexist with the v1 callback-based path. v2 is opt-in via `version="v2"` and turns the stream into a discriminated union over `Literal["values","updates","checkpoints","tasks","debug","messages","custom"]`.

## Notable Patterns

- **Deterministic task-id hashing** (`libs/langgraph/langgraph/pregel/_algo.py:616-623`) — gives free replay/time-travel.
- **`CheckpointMetadata.source` discriminator** (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-55`) — splits the checkpoint lineage into `input`, `loop`, `update`, `fork`, used for fork-on-replay detection (`libs/langgraph/langgraph/pregel/_loop.py:948-959`).
- **Step-prefixed synthetic task IDs for exit-mode delta writes** (`libs/langgraph/langgraph/pregel/_loop.py:1260-1275`) — preserves chronological superstep ordering under the saver's `ORDER BY task_id, idx` sort key.
- **`PendingWrite` indirection** for control writes via negative indices in `WRITES_IDX_MAP` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:795`) — lets the saver's primary-key constraint disambiguate regular and control writes without a separate schema.
- **`PregelScratchpad` for task-scoped runtime state** — supports `interrupt()` replay via `interrupt_counter()` and `get_null_resume(True)` (`libs/langgraph/langgraph/types.py:911-925`); the scratchpad is the only persistent per-task scratch slot that survives resume.
- **`delta_channels_to_snapshot` policy** — delta channels periodically snapshot their value into the `Checkpoint.channel_values` (controlled by `counters_since_delta_snapshot`, `libs/langgraph/langgraph/pregel/_loop.py:1079-1142`). The rest of the time, channel state is reconstructed by walking `pending_writes` along the parent chain (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`).
- **`_coerce_pending_error`** (`libs/langgraph/langgraph/pregel/_algo.py:764-768`) — converts deserialized error payloads back into `BaseException` instances on resume, so that `commit` sees a real exception and routes correctly (`libs/langgraph/langgraph/pregel/_runner.py:579-603`).
- **`NodeCancelledError` vs `CancelledError` discrimination** via `asyncio.Task.cancelling()` (`libs/langgraph/langgraph/pregel/_retry.py:315-334,777-794`) — distinguishes user-raised cancellation from framework-initiated sibling cancellation, so a user `raise CancelledError()` is treated as a node failure, not silent tear-down.

## Tradeoffs

- **Superstep is large for fine-grained recovery**. A tick can produce many writes; if the process dies after some `put_writes` but before `_put_checkpoint`, those writes are present in the row but the parent checkpoint may not be, leading to the `_put_exit_delta_writes` synthetic stub (`libs/langgraph/langgraph/pregel/_loop.py:1201-1292`). The stub pattern recovers correctness at the cost of an extra checkpoint row.
- **`put_writes` is fire-and-forget per task** with ordering enforced only at checkpoint boundaries (`libs/langgraph/langgraph/pregel/_loop.py:459-498`). For `durability="async"` and `durability="sync"`, `_delta_write_futs`/`_error_handler_write_futs` drain the futures before the next `_put_checkpoint` or before the handler starts; `durability="exit"` defers the ordering entirely. This is efficient but assumes the saver's primary-key `(task_id, idx)` keeps the row-level ordering for free.
- **Retry is per task, not per write**. A node that returns a partial result and then crashes cannot be retried write-by-write; it must replay the whole task. This makes tool-level retry impossible without breaking the node into multiple tasks.
- **Cache is task-scoped**. Cached writes for a node replace the node's output wholesale (`libs/langgraph/langgraph/pregel/_loop.py:1526-1539`); there is no per-tool-call cache.
- **Stream modes are mode-dependent**, so the "atomic unit shown to the user" is not a stable concept. A consumer of `values` mode sees a step; a consumer of `tasks` mode sees tasks; a consumer of `checkpoints` mode sees the checkpoint. There is no canonical "atomic event" type.
- **Null task ID overlaps with real task IDs only at the literal level**; the in-memory `checkpoint_pending_writes` keeps null writes separate from per-task writes via `WRITES_IDX_MAP` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:795`). This is safe but undocumented at the type level — the null writes are encoded as `(NULL_TASK_ID, channel, value)` and processed via `if task_id == NULL_TASK_ID` branches (`libs/langgraph/langgraph/pregel/_loop.py:415-424`).
- **`v1` and `v2` stream contracts coexist** (`libs/langgraph/langgraph/pregel/_messages.py:259-420`). `v2` is a discriminated union over `Literal["values","updates","checkpoints","tasks","debug","messages","custom"]` with explicit `type` and `ns` fields; `v1` is callback-driven and ad-hoc. Consumers must pick a version.

## Failure Modes / Edge Cases

- **Mid-`apply_writes` crash** loses the in-memory state change because `apply_writes` mutates `self.checkpoint` directly (`libs/langgraph/langgraph/pregel/_loop.py:680-686`). The next process restart loads the prior checkpoint.
- **Crash between `apply_writes` and `_put_checkpoint`**: durable state is the prior checkpoint. The new state will not exist on resume.
- **Crash between `_put_checkpoint.submit` and `put_writes.submit`**: the checkpoint row exists but writes are missing for tasks whose `put_writes` futures did not complete; on resume, `_reapply_writes_to_succeeded_nodes` (`libs/langgraph/langgraph/pregel/_loop.py:724-737`) is a no-op for those tasks (no saved writes), so they re-execute.
- **Crashed task with no error handler and exhausted retries**: surfaces as `GraphRecursionError` or whatever the node raised; the runner's `_should_stop_others` cancels sibling tasks (`libs/langgraph/langgraph/pregel/_runner.py:616-634`).
- **Recursion-limit exhaustion** is reported via `GraphRecursionError` at `libs/langgraph/langgraph/pregel/main.py:3018-3026,3501-3508` and surfaces `status == "out_of_steps"`. The user has to bump `config["recursion_limit"]`.
- **`step_timeout` is wall-clock, not per-task**. A single stuck task will trip the timeout (`libs/langgraph/langgraph/pregel/main.py:2984,3457`); the runner then cancels remaining tasks via `_panic_or_proceed` (`libs/langgraph/langgraph/pregel/_runner.py:650-697`).
- **Delta-channel pruning hazard**: `prune`/`copy_thread` documentation warns that a naive `keep_latest` strategy can silently break `DeltaChannel` reconstruction (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415,331-372`). The same hazard applies to `delete_for_runs` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-348,522-538`).
- **`durability="exit"` + delta channels + no parent**: handled by lazy stub at `_put_exit_delta_writes` (`libs/langgraph/langgraph/pregel/_loop.py:1201-1292`). If the process dies before exit, the synthetic stub and the accumulated writes are both lost.
- **Time-travel to a checkpoint that contains `INTERRUPT` writes**: `_first` drops stale `INTERRUPT` writes from the loaded checkpoint so that re-firing `interrupt()` produces a fresh id (`libs/langgraph/langgraph/pregel/_loop.py:952-958`).
- **Subgraph time-travel with checkpoint_map**: `is_time_traveling` is computed from `CONFIG_KEY_REPLAY_STATE` and the parent's checkpoint_map (`libs/langgraph/langgraph/pregel/_loop.py:866-884`); a wrong map can cause the subgraph to load the wrong ancestor checkpoint.
- **`Scheduled` future cancellation via parent**: `_should_route_to_error_handler` (`libs/langgraph/langgraph/pregel/_runner.py:171-174`) plus `should_stop_others` (`libs/langgraph/langgraph/pregel/_runner.py:616-634`) cancel siblings when a sibling fails, except for handlers and `GraphBubbleUp`.
- **User `raise CancelledError` inside a node**: in async, `_is_user_raised_cancelled` (`libs/langgraph/langgraph/pregel/_retry.py:315-334`) checks `asyncio.Task.cancelling() == 0` and converts to `NodeCancelledError` so the runner panics the run rather than treating the task as silent tear-down.

## Future Considerations

- **Tool-level atomicity** is a recurring gap. Per-tool retry/cache/durability would require either splitting each tool call into its own `PregelExecutableTask` or adding a parallel concept (e.g., a `SubTask` type with its own retry counter and write isolation).
- **Transactional step boundaries** would let `apply_writes` and `_put_checkpoint` run as one atomic action; today they are split, with ordering enforced only by `_checkpointer_put_after_previous` draining futures.
- **Unifying the v1/v2 stream contracts**. The v2 discriminated union (`libs/langgraph/langgraph/types.py:341-365`) is cleaner but not the default; once the v1 path is removed, the atomic unit shown to the user would be unambiguous.
- **`recursion_limit` / `step_timeout` / `durability`** are all global to the run. Per-node or per-task equivalents (especially per-subgraph `durability`) would let multi-tenant graphs tune atomicity independently.
- **`_exit_delta_writes` synthetic stub** could be replaced by a proper "empty parent" sentinel in the saver schema, removing the special-case code at `libs/langgraph/langgraph/pregel/_loop.py:1230-1258`.
- **`run_step` vs `step`**: pydantic-ai has a separate `run_step` counter (`pydantic-ai.md` reference study). LangGraph conflates "step number" with "superstep number"; if it ever needs a finer counter (e.g., for partial ticks or sub-step retries), the model would need to extend `CheckpointMetadata` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86`).
- **`WRITES_IDX_MAP`** at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` only has four negative indices reserved. Adding more control writes (e.g., a `CANCEL` marker) requires bumping that mapping.
- **Delta-channel snapshot bookkeeping** (`counters_since_delta_snapshot`) is a metadata-only counter; the actual snapshotting is best-effort. A future version could expose the snapshot frequency and bound to user policy.

## Questions / Gaps

- What is the canonical atomic event for **observability** — is it the task, the superstep, or the checkpoint? The stream surface treats all three as first-class via different `stream_mode`s, but there is no unified "event" type. (No clear evidence found for a single event type spanning modes.)
- How does the system report a **partially-flushed tick** to a UI consumer? `put_writes` is fire-and-forget; a consumer that wants "exactly what completed and what did not" would have to inspect `pending_writes` directly. No clear evidence found for an explicit "partial-tick" stream event.
- What happens to a **`stream_mode="messages"` consumer** when the producing tick is rolled back by an exception in a sibling task? The `messages` stream is implemented via LangChain callbacks (`libs/langgraph/langgraph/pregel/_messages.py:49-258`) that emit tokens as they are produced; there is no rollback path. No clear evidence found for a recovery contract on rollback.
- Is there a test for the `_checkpointer_put_after_previous` ordering invariant under concurrent crashes? No clear evidence found in the searched paths.
- The `node_error_handler_map` (`libs/langgraph/langgraph/graph/state.py:1327-1331`) is built at compile time; if a handler node itself crashes, the second-level failure falls through to the standard `_panic_or_proceed` path. No clear evidence found for second-level error handler chaining.

---

Generated by `01.03-step-turn-and-task-atomicity` against `langgraph`.