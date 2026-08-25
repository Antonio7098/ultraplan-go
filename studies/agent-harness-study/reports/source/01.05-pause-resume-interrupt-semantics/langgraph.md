# Source Analysis: langgraph

## 01.05: Pause, Resume, and Interrupt Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (Pregel-style execution, asyncio + threads, pluggable checkpointers) |
| Analyzed | 2026-07-02 |

## Summary

LangGraph exposes a mature, explicit two-axis interrupt model: **declarative breakpoints** (`interrupt_before` / `interrupt_after` configured at compile time, evaluated by `should_interrupt` against channel versions) and **dynamic in-node interrupts** (`interrupt(value)` raises `GraphInterrupt`, surfaces an `Interrupt` payload to the client, and is resumable through `Command(resume=...)`).

Pause is always persisted: the `SyncPregelLoop` / `AsyncPregelLoop` runs the Pregel superstep, persists a checkpoint via the `BaseCheckpointSaver`, and emits a `GraphInterruptEvent` (or `GraphResumeEvent` on the next entry) which the public callback handlers (`GraphCallbackHandler.on_interrupt`, `on_resume`) deliver (`langgraph/callbacks.py:43-77`). Resume is purely a checkpointer + `Command` round-trip: the resume value becomes a special `(NULL_TASK_ID, __resume__, value)` pending write (`langgraph/pregel/_io.py:75`) which the per-task `PregelScratchpad` (`langgraph/_internal/_scratchpad.py:9`, built in `langgraph/pregel/_algo.py:1280-1345`) makes visible to `interrupt()` on the next entry. The resume point is deterministic because it is keyed by checkpoint_id + task namespace + interrupt counter; concurrent interrupt ids use xxh3 hashes so resume values can be targeted (`langgraph/types.py:559-578`, `_loop.py:898-908`).

A separate cooperative-drain mechanism (`RunControl.request_drain()`, `langgraph/runtime.py:95`) lets callers stop a run at the next superstep boundary (e.g., SIGTERM) while still persisting the checkpoint; the run then resumes from the saved state with a fresh `invoke(None, config)` (`tests/test_runtime.py:170-246`). Subgraph interrupts bubble up as a single combined `GraphInterrupt` and are suppressed at the root (`langgraph/pregel/_runner.py:585-690`, `_loop.py:1313-1352`), so the orchestrator sees one list of interrupts per paused run.

## Rating

**9 / 10 — Mature, durable, observable, extensible, and proven under failure.**

Rationale:

- Explicit primitives (`interrupt`, `Command`, `interrupt_before/after`, `RunControl`) with distinct semantics per kind.
- Pause is fully persisted: checkpoints + special `INTERRUPT` and `RESUME` writes are durable via any `BaseCheckpointSaver` (memory, SQLite, Postgres). Multiple durability modes (`sync`/`async`/`exit`) make pause/resume semantics explicit (`langgraph/pregel/main.py:2670-2685`, `_loop.py:1064-1199`).
- Resume after process restart works by re-invoking with the same `thread_id` and `Command(resume=...)`. Tested across `InMemorySaver`, `SqliteSaver`, `PostgresSaver` via `conftest_checkpointer.py`.
- Lifecycle callbacks (`on_interrupt`, `on_resume`) and stream events (`__interrupt__`, `values` mode, `messages`) make pause/resume observable end-to-end (`langgraph/callbacks.py:42-77`, `langgraph/stream/transformers.py:53-80`).
- Multiple interrupts per node are supported via xxh3-keyed `Command(resume={interrupt_id: value})`; migration tests prove legacy interrupt payloads (pre-v1) round-trip through modern serializers (`tests/test_interrupt_migration.py:11-49`).
- Cooperative drain is the weakest link (sub-superstep cancellation is unsafe and not attempted), but that is the correct tradeoff; documented and tested (`tests/test_runtime.py:100-246`).
- Minor deductions: (a) re-execution of an interrupted node re-runs the node body from scratch — fine for idempotent nodes, but the runtime offers no built-in idempotency guarantee; (b) multiple clients targeting the same thread id race on checkpoint writes (first-write-wins semantics); (c) some `_put_checkpoint` exit-mode logic uses an object-identity trick (`_loop.py:1075`) that is subtle and already flagged for replacement.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dynamic interrupt primitive | `interrupt(value)` raises `GraphInterrupt`; returns resume value on subsequent invocations within the same task via `scratchpad` | `libs/langgraph/langgraph/types.py:811-934` |
| Static interrupt configuration | `interrupt_before` / `interrupt_after` compile-time parameters on `StateGraph.compile` | `libs/langgraph/langgraph/graph/state.py:1170-1254` |
| Pregel-loop static interrupt | `should_interrupt` checks trigger versions against `versions_seen[INTERRUPT]` | `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| Pregel-loop `interrupt_before` tick | Loop raises `GraphInterrupt` before any tasks execute | `libs/langgraph/langgraph/pregel/_loop.py:659-664` |
| Pregel-loop `interrupt_after` tick | Loop raises `GraphInterrupt` after tasks complete | `libs/langgraph/langgraph/pregel/_loop.py:707-712` |
| Interrupt payload type | `Interrupt(value, id)` with `Interrupt.from_ns` deriving id via xxh3 hash of checkpoint namespace | `libs/langgraph/langgraph/types.py:530-588` |
| `GraphInterrupt` exception | Bubbles up to root loop; carries `tuple[Interrupt, ...]` | `libs/langgraph/langgraph/errors.py:102-126` |
| Runner commit on interrupt | `commit()` writes `INTERRUPT` (and any `RESUME`) to the checkpointer and does **not** treat interrupt as a failure | `libs/langgraph/langgraph/pregel/_runner.py:574-613` |
| Bubble-up exception class | `GraphBubbleUp` is the exception super-class for interrupts, drain, ParentCommand | `libs/langgraph/langgraph/errors.py:50-65` |
| `_panic_or_proceed` interrupt handling | Combines multiple `GraphInterrupt`s from sibling tasks into one combined exception | `libs/langgraph/langgraph/pregel/_runner.py:669-690` |
| Resume scratchpad construction | `_scratchpad` reads `NULL_TASK_ID+__resume__` (null_resume) and task-id-specific `__resume__` from pending writes | `libs/langgraph/langgraph/pregel/_algo.py:1280-1345` |
| Resume scratchpad type | `PregelScratchpad` carries `resume: list`, `get_null_resume`, `interrupt_counter` | `libs/langgraph/langgraph/_internal/_scratchpad.py:8-19` |
| Map Command → pending writes | `map_command` yields `(NULL_TASK_ID, RESUME, cmd.resume)` for the resume value | `libs/langgraph/langgraph/pregel/_io.py:56-78` |
| Resume classification | `_first` distinguishes first run from resume via `Command`/`None` input / same `run_id` / `CONFIG_KEY_RESUMING` | `libs/langgraph/langgraph/pregel/_loop.py:836-919` |
| Resume-map path | `CONFIG_KEY_RESUME_MAP` populated when `Command(resume=dict_of_xxh3_ids)` arrives; multi-interrupt validation | `libs/langgraph/langgraph/pregel/_loop.py:891-919` |
| Resume map consumption | `_scratchpad` reads `resume_map[namespace_hash]` to target a specific interrupt | `libs/langgraph/langgraph/pregel/_algo.py:1311-1314` |
| Pending-interrupt accounting | `_pending_interrupts` returns interrupt ids without matching resume writes | `libs/langgraph/langgraph/pregel/_loop.py:806-834` |
| Time-travel vs resume | Replay drops cached `RESUME` writes so `interrupt()` re-fires; preserves them for actual resume | `libs/langgraph/langgraph/pregel/_loop.py:862-888` |
| Checkpoint + writes indices | `WRITES_IDX_MAP` reserves `INTERRUPT=-3, RESUME=-4` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| Checkpoint tuple | `CheckpointTuple` bundles checkpoint + metadata + parent_config + pending_writes | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:139-147` |
| Memory saver pending writes | `InMemorySaver.put_writes` stores `(task_id, channel, dumped_value, path)` keyed by `(task_id, idx)` | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:473-509` |
| Checkpoint put_after_previous | `SyncPregelLoop._put_checkpoint` serializes per superstep; respects durability mode | `libs/langgraph/langgraph/pregel/_loop.py:1064-1199` |
| Resume via `CONFIG_KEY_RESUMING` | Loop sets `CONFIG_KEY_RESUMING=True` on retry path so subgraph tasks know to resume | `libs/langgraph/langgraph/pregel/_retry.py:682,838` |
| Replays skip control signals | `_reapply_writes_to_succeeded_nodes` skips `ERROR/ERROR_SOURCE_NODE/INTERRUPT/RESUME` so failed/interrupted tasks re-execute | `libs/langgraph/langgraph/pregel/_loop.py:724-737` |
| Resume from input == None | Static `interrupt_after` + `invoke(None, config)` resumes | `libs/langgraph/langgraph/pregel/_loop.py:844` |
| Durability modes | `Durability` field on `Pregel.invoke/stream`: `sync`/`async`/`exit` | `libs/langgraph/langgraph/pregel/main.py:2670-2685` |
| Drain control | `RunControl.request_drain` flips a single attribute; checked in `tick` to set status `draining` | `libs/langgraph/langgraph/runtime.py:79-104`, `libs/langgraph/langgraph/pregel/_loop.py:650-652` |
| `GraphDrained` exception | Raised when draining; the checkpoint is persisted and resume is a normal `None` invoke | `libs/langgraph/langgraph/errors.py:54-64` |
| Drain-from-subgraph propagation | Subgraph drain request via `Runtime.control` reaches parent loop; checkpoint still saved | `libs/langgraph/langgraph/runtime.py:233-282`, `tests/test_runtime.py:203-246` |
| Lifecycle callback events | `GraphInterruptEvent` / `GraphResumeEvent` payload classes | `libs/langgraph/langgraph/callbacks.py:42-77` |
| Lifecycle callback dispatch | `SyncPregelLoop._push_graph_lifecycle_event` enqueues; `stream` drains to `GraphCallbackManager.on_interrupt`/`on_resume` | `libs/langgraph/langgraph/pregel/_loop.py:369-401`, `libs/langgraph/langgraph/pregel/main.py:2903-2912` |
| Streaming `__interrupt__` | Stream `values` and `updates` modes carry `__interrupt__` payload so streaming consumers see pause signals | `libs/langgraph/langgraph/stream/transformers.py:53-80`, `libs/langgraph/langgraph/stream/run_stream.py:75-103` |
| Subgraph interrupt propagation | Subgraph `GraphInterrupt` bubbles up; root `_suppress_interrupt` translates to combined `GraphInterruptEvent` | `libs/langgraph/langgraph/pregel/_runner.py:585-613`, `libs/langgraph/langgraph/pregel/_loop.py:1313-1352` |
| Multiple interrupts per node | `interrupt_counter` increments per call; sequential resume matches via index in `interrupt()` | `libs/langgraph/langgraph/types.py:911-934`, `libs/langgraph/langgraph/pregel/_algo.py:1340` |
| Legacy interrupt migration | `Interrupt.from_ns` and modern serializer round-trip legacy `ns`/`resumable`/`when` payloads | `libs/langgraph/langgraph/types.py:559-578`, `libs/langgraph/langgraph/checkpoint/serde/jsonplus.py` (test `tests/test_interrupt_migration.py:11-49`) |
| Resume-from-checkpoint-history | `get_state_history` + `app.stream(Command(resume=""), history[N].config)` resumes an arbitrary checkpoint (creates a fork) | `libs/langgraph/langgraph/pregel/_loop.py:1019-1059` (test `tests/test_checkpoint_migration.py:1644-1726`) |
| Static interrupt test | `interrupt_after="*"` resumes via `invoke(None, thread)`; checkpoint counts depend on durability mode | `libs/langgraph/tests/test_interruption.py:11-92` |
| Drain test | `RunControl.request_drain()` inside a node stops the run, persists checkpoint, resume completes | `libs/langgraph/tests/test_runtime.py:170-246` |
| Subgraph interrupt resume | Parent graph compiled with checkpointer + child `interrupt()` resumes through parent `Command(resume=...)` | `libs/langgraph/tests/test_subgraph_persistence.py:30-83`, `libs/langgraph/tests/test_subgraph_persistence_async.py:44-91` |
| Resume error handler scheduling | `_resume_error_handlers_if_applicable` re-runs error handlers on resume for tasks that failed in a prior run | `libs/langgraph/langgraph/pregel/_loop.py:739-804` |
| RemoteGraph interrupt mirror | `RemoteGraph` decodes `__interrupt__` from server stream and re-raises `GraphInterrupt` so the local view mirrors a remote pause | `libs/langgraph/langgraph/pregel/remote.py:838-862` |
| Functional-API interrupt | `@entrypoint(checkpointer=...)` decorated function can call `interrupt(value)`; resume via `Command(resume=...)` | `libs/langgraph/langgraph/func/__init__.py:332-379` |
| Status semantics | `Literal["input", "pending", "done", "draining", "interrupt_before", "interrupt_after", "out_of_steps"]` carried in `GraphInterruptEvent.status` | `libs/langgraph/langgraph/pregel/_loop.py:249-257`, `libs/langgraph/langgraph/callbacks.py:31-39` |

## Answers to Dimension Questions

### 1. Can execution pause safely?

Yes. There are four pause mechanisms, each persisted to the checkpointer:

1. **Dynamic interrupt (`interrupt(value)`)** — raises `GraphInterrupt` carrying `Interrupt(value, id)` (`langgraph/types.py:811-934`). The runner's `commit()` saves `(INTERRUPT, ...)` to the checkpointer as a special write (`langgraph/pregel/_runner.py:585-591`) so the pause is durable independently of the next checkpoint.
2. **Static `interrupt_before`** — loop checks `should_interrupt` before any task executes and raises `GraphInterrupt` (`langgraph/pregel/_loop.py:659-664`).
3. **Static `interrupt_after`** — loop checks `should_interrupt` after tasks complete and raises `GraphInterrupt` (`langgraph/pregel/_loop.py:707-712`).
4. **Cooperative drain** — `RunControl.request_drain()` flips a flag checked at the top of `tick()`; loop returns early with status `"draining"` and `GraphDrained` exception propagates (`langgraph/runtime.py:79-104`, `langgraph/pregel/_loop.py:650-652`). Checkpoint is persisted on the way out.

Pause safety is verifiable: tests (`tests/test_interruption.py:11-92`, `tests/test_runtime.py:170-200`) assert exact checkpoint counts after pause and after resume.

### 2. Can it resume after a crash?

Yes. Pause is fully persisted: a `Checkpoint` plus any pending writes (including `INTERRUPT` and `RESUME`) are written by the configured `BaseCheckpointSaver`. After the process restarts:

- The same `thread_id` (and same `checkpoint_ns` for subgraphs) re-loads the most recent checkpoint via `get_tuple` (`langgraph/pregel/main.py:1427`).
- The client invokes again with either `invoke(None, config)` (for `interrupt_before/after` / drain) or `invoke(Command(resume=...), config)` (for `interrupt()`). The Command path goes through `map_command` → `(NULL_TASK_ID, RESUME, value)` pending write (`langgraph/pregel/_io.py:56-78`).

This works with `InMemorySaver` (lost on process exit — not for production), `SqliteSaver`, and `PostgresSaver`. The Postgres implementation lives in `libs/checkpoint-postgres/langgraph/checkpoint/` (read at `libs/checkpoint-postgres/langgraph/checkpoint/__init__.py`).

### 3. Is the resume point deterministic?

Yes. Determinism is enforced by:

- **Checkpoint lineage**: `_first` only resumes if a prior checkpoint exists (`channel_versions` non-empty) and the input signals continuation (`Command`, `None` input, same `run_id`, or `CONFIG_KEY_RESUMING` flag) (`langgraph/pregel/_loop.py:836-919`).
- **Per-task resume binding**: pending `RESUME` writes are tied to a specific `task_id`, not the global config. The task-id-specific value is consumed by the matching `interrupt()` call (`langgraph/pregel/_algo.py:1300-1314`).
- **Interrupt-counter indexing**: multiple `interrupt()` calls in the same node are matched to resume values by index, with the value provided in declaration order (`langgraph/types.py:911-934`).
- **Namespace-scoped id**: `Interrupt.from_ns` derives the `id` from the namespace xxh3 hash, so the same interrupt in different subgraphs has a different id and can be resumed independently (`langgraph/types.py:577-578`).
- **Time-travel handling**: when replaying from a specific checkpoint, stale `RESUME` writes are dropped so `interrupt()` re-fires; when actually resuming, they are preserved (`langgraph/pregel/_loop.py:862-888`).

There is one non-determinism caveat: if a node body has external side effects (e.g., HTTP calls), re-executing the node from the top on resume will re-run those side effects. LangGraph does not provide automatic node-idempotency.

### 4. What happens if the world changed while paused?

LangGraph's design lets the **node body itself** inspect the world when it re-executes: the node re-runs from the top, including any pre-`interrupt()` logic, and the resume value is whatever `Command(resume=...)` delivered. The runtime does **not** re-execute earlier siblings of the interrupted node in the same superstep (their writes are reapplied from the checkpoint via `_reapply_writes_to_succeeded_nodes`, skipping `ERROR/ERROR_SOURCE_NODE/INTERRUPT/RESUME` — `langgraph/pregel/_loop.py:724-737`).

For subgraph interrupts, the parent's view is fixed at the time of pause — the subgraph re-executes on resume and may produce different results, which propagate back via the normal channel writes on the next superstep. There is no built-in staleness check; that is left to application code (revalidation patterns in `libs/prebuilt/`).

### 5. Can multiple people or systems resume the same run?

Not safely. The model is single-writer per `thread_id`:

- Two concurrent `invoke(Command(resume=...), config)` calls on the same thread id both load the same checkpoint, both compute the next checkpoint, and the second `put` overwrites the first (depending on checkpointer implementation; Postgres uses atomic `INSERT ... ON CONFLICT DO NOTHING`-style writes, see `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py`).
- The checkpointer does **not** provide optimistic concurrency control or CAS-style resume.
- Different `thread_id`s can run the same logic in parallel — that is the supported multi-actor pattern.

There is no built-in "claim" or "lock" on a thread id. Coordination must be implemented at the application layer.

## Architectural Decisions

- **Persist everything needed to resume**. The checkpointer stores both a checkpoint (full channel values + versions) and pending writes (per-task, including reserved keys for `ERROR`, `INTERRUPT`, `RESUME`, `SCHEDULED`). The reserved keys have negative indices (`WRITES_IDX_MAP`) so they cannot collide with regular writes (`libs/checkpoint/base/__init__.py:795`).
- **Two distinct interrupt flavors**. Dynamic (`interrupt()` from inside a node) and static (`interrupt_before` / `interrupt_after` configured at compile time). Static is purely channel-version-based and does not need a `Command`; dynamic is value-driven and requires `Command(resume=...)`.
- **Idempotency boundary is the node body**. The runtime re-executes the entire node body on resume, with the resume value available through the scratchpad. Application code is responsible for making node bodies safe under re-entry (e.g., using cached `entrypoint.final` to memoize earlier work — `libs/langgraph/langgraph/func/__init__.py:479-525`).
- **Explicitness over magic**. Resume is never implicit; it requires either `None` (replay after static interrupt) or `Command(resume=...)`. This is documented and tested (`libs/langgraph/langgraph/types.py:758-779`, `tests/test_large_cases.py:4171-4250`).
- **Durability is orthogonal to interrupt semantics**. Three modes (`sync`, `async`, `exit`) control when the checkpoint is persisted relative to the next superstep, but they do not change when an interrupt is observable to the client — the interrupt event is still emitted through the stream pipeline.
- **Subgraph interrupts bubble up**. A subgraph's `GraphInterrupt` is collected by the runner and re-raised as a combined `GraphInterrupt` so the parent sees one tuple of interrupts (`libs/langgraph/langgraph/pregel/_runner.py:669-690`, `_loop.py:1313-1352`).
- **Lifecycle callbacks are first-class**. `GraphCallbackHandler` is a dedicated `BaseCallbackHandler` subclass with `on_interrupt` / `on_resume` methods, separate from generic LangChain callbacks (`libs/langgraph/langgraph/callbacks.py:42-111`).
- **No partial node state**. If a node fails (raises a non-interrupt exception) after partially mutating channels, those partial writes are **not** applied — only `RESUME` writes from the same task are preserved alongside `INTERRUPT` (`libs/langgraph/langgraph/pregel/_runner.py:584-591`).

## Notable Patterns

- **Pregel superstep loop**: classic BSP model — prepare tasks, run, commit writes, persist checkpoint, decide interrupt (`libs/langgraph/langgraph/pregel/_loop.py:592-714`).
- **Reserved write keys with negative indices**: avoids name collisions and ensures predictable ordering of control signals (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:795`).
- **Scratchpad-based per-task state**: `PregelScratchpad` carries per-task resume values, counters, and parent-scratchpad fallback for subgraphs (`libs/langgraph/langgraph/_internal/_scratchpad.py:9-19`, `libs/langgraph/langgraph/pregel/_algo.py:1280-1345`).
- **Configurable checkpoint indirection**: `CONFIG_KEY_RESUMING`, `CONFIG_KEY_RESUME_MAP`, `CONFIG_KEY_SEND` keys propagate resume information from the parent loop to subgraph tasks without explicit API calls (`libs/langgraph/langgraph/_internal/_constants.py:33-73`).
- **Channel-version-triggered static interrupts**: `should_interrupt` checks if any channel version has moved past `versions_seen[INTERRUPT]`, so `interrupt_before/after` only triggers when there is meaningful state to inspect (`libs/langgraph/langgraph/pregel/_algo.py:155-185`).
- **xxh3-derived interrupt id**: deterministic, namespace-keyed, migratable from legacy `ns`/`resumable` payloads (`libs/langgraph/langgraph/types.py:559-578`).
- **Time-travel as a first-class concept**: `update_state` and explicit `checkpoint_id` config create forks (`source="fork"` / `source="update"`) that resume into a new branch rather than overwriting the original (`libs/langgraph/langgraph/pregel/_loop.py:940-959`).
- **Stream transformers preserve interrupt visibility**: every streaming consumer (sync/async, with/without subgraphs) sees `__interrupt__` in `values` / `updates` mode (`libs/langgraph/langgraph/stream/transformers.py:53-80`, `libs/langgraph/langgraph/stream/run_stream.py:75-103`).

## Tradeoffs

- **Node re-execution vs state diffing**: LangGraph chose re-execution from the top of the node, not replaying the node from after the `interrupt()` call. This keeps the runtime simple but shifts idempotency responsibility to the application.
- **Synchronous checkpoint writes**: even in async mode, the saver returns a future that the next superstep waits on (`_checkpointer_put_after_previous`, `_loop.py:1182-1189`). This gives strong ordering but means an unresponsive saver slows the loop.
- **Subgraph interrupts are surfaced to root only**: parent orchestrators see one merged `GraphInterrupt` per run. This loses per-subgraph attribution unless the `Interrupt.id` / `Interrupt.value` carries the namespace hash.
- **`exit` durability trades latency for atomicity**: in `exit` mode, checkpoints are only persisted when the run ends or interrupts. A crash mid-run leaves the previous checkpoint intact (good) but loses in-flight work (bad if you were relying on incremental persistence).
- **Drain is cooperative, not preemptive**: a node that ignores the drain signal (e.g., busy `time.sleep`) blocks the drain. There is no forced cancellation.
- **`Command` is the only resume primitive**: a plain dict or scalar resume is not supported; clients must wrap their data in `Command(resume=...)`. This is explicit but verbose.

## Failure Modes / Edge Cases

- **Multiple interrupts per node** require either a single value (consumed in order) or a dict keyed by interrupt id; otherwise the runtime raises `RuntimeError("When there are multiple pending interrupts, you must specify the interrupt id when resuming...")` (`libs/langgraph/langgraph/pregel/_loop.py:904-908`).
- **`Command(resume=...)` without a checkpointer** raises `RuntimeError("Cannot use Command(resume=...) without checkpointer")` (`libs/langgraph/langgraph/pregel/_loop.py:893-896`).
- **Mid-task failures**: if a node raises a non-interrupt exception, the node's writes are **not** applied; only the `ERROR` and (if a handler exists) `ERROR_SOURCE_NODE` writes are saved (`libs/langgraph/langgraph/pregel/_runner.py:595-603`).
- **`asyncio.CancelledError` from user code** is now wrapped in `NodeCancelledError` to prevent silent tear-down masquerading as success (`libs/langgraph/langgraph/errors.py:168-186`, `_retry.py:635-640`).
- **`GraphBubbleUp` short-circuits the retry layer**: `interrupt()` and `ParentCommand` skip retry handling and bubble directly to the runner (`libs/langgraph/langgraph/pregel/_retry.py:632-634, 773-776`).
- **Time-travel race**: if the client resumes with an explicit `checkpoint_id` that happens to match the current head, the loop distinguishes time-travel from resume via input shape (`Command` resume vs explicit checkpoint config) (`libs/langgraph/langgraph/pregel/_loop.py:866-884`).
- **Stale fork checkpoint**: replaying from a checkpoint forks the lineage; if a subsequent interrupt happens before `after_tick()` runs, the loop saves an explicit `source="fork"` checkpoint so subsequent resumes do not load the wrong state (`libs/langgraph/langgraph/pregel/_loop.py:940-959`).
- **Pre-v1 interrupt payloads**: legacy checkpoints serialized with `ns` / `resumable` / `when` fields round-trip through the modern `Interrupt` type via `Interrupt.from_ns` and serde migration (`libs/langgraph/langgraph/types.py:559-578`, `tests/test_interrupt_migration.py:11-49`).
- **Checkpoint pruning with DeltaChannels**: naive `prune` that drops ancestors can corrupt delta-channel reconstruction; the docstring warns against this (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`).
- **`_put_checkpoint` exiting flag uses identity check**: `metadata is self.checkpoint_metadata` is fragile and explicitly marked for replacement (`libs/langgraph/langgraph/pregel/_loop.py:1064-1075`).

## Future Considerations

- **Idempotent resume hooks**: a future API could let node authors mark a portion of the node as "before-interrupt" vs "after-resume" to avoid re-running expensive setup on resume. Currently the entire node re-executes.
- **Optimistic concurrency**: a CAS-style `expected_parent_checkpoint_id` parameter on `Command(resume=...)` would let multiple actors coordinate on a thread without application-level locking.
- **Subgraph attribution**: `GraphInterrupt` currently flattens to `tuple[Interrupt, ...]`. Surfacing the originating namespace explicitly (rather than relying on `Interrupt.id` decoding) would help debugging.
- **Drain timeout**: `RunControl` is fire-and-forget; a deadline-aware variant could abort long-running nodes cleanly.
- **Stream `messages` mode and interrupts**: `StreamMessagesHandlerV2` is gated explicitly so v1 streams do not route interrupt-bearing content blocks through the v2 path (`libs/langgraph/langgraph/pregel/main.py:2788-2799`). A unified handler model would reduce the special-case surface.

## Questions / Gaps

- **Cross-process resume**: while `PostgresSaver` and `SqliteSaver` are designed for cross-process access, there is no built-in lock or heartbeat for "another worker is currently running this thread." A status query would help.
- **Resume after partial channel corruption**: no evidence found of automatic detection/repair if a checkpoint file is corrupted. The runtime would surface the error via the checkpointer's `get_tuple`, but recovery is undefined.
- **Interrupt payloads across subgraph boundaries**: how `Interrupt.value` should be structured when the same `interrupt_id` appears in multiple subgraphs is not explicitly documented; tests rely on xxh3 hashing to disambiguate.
- **Resume ordering guarantees with multiple parallel subgraphs**: each subgraph stores its own checkpoint namespace; the order in which multiple parallel subgraph interrupts are surfaced is "first to finish superstep", which is non-deterministic relative to the user's mental model.
- **Drain during a checkpoint save**: the relationship between `_put_checkpoint`'s exit-mode `exiting=True` path and an in-flight drain request is not explicitly tested.