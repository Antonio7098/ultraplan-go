# Source Analysis: langgraph

## 02.04 Mutation Discipline and State Transitions

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo of libraries: `checkpoint`, `checkpoint-sqlite`, `checkpoint-postgres`, `langgraph`, `prebuilt`, `cli`, `sdk-py`) |
| Analyzed | 2026-07-10 |

## Summary

LangGraph models state as a fixed set of named **channels** (the `BaseChannel` family) holding values of various semantics (`LastValue`, `BinaryOperatorAggregate` / reducer, `Topic`, `AnyValue`, `EphemeralValue`, `UntrackedValue`, `NamedBarrierValue`, `LastValueAfterFinish`, `NamedBarrierValueAfterFinish`, `DeltaChannel`). State is mutated exclusively through a single `update(Sequence[Update]) -> bool` entry point on each channel (`libs/langgraph/langgraph/channels/base.py:90`). Writes reach channels only through the Pregel apply-write loop (`libs/langgraph/langgraph/pregel/_algo.py:232-345`), which sorts tasks deterministically, dispatches writes into per-channel `update()` calls, bumps monotonically increasing per-channel versions, and records `versions_seen` per node. Checkpointing is a *checkpoint-after-step* model — atomic per-step snapshots are persisted, optionally bucketed into a bounded-replay `DeltaChannel` representation (`libs/langgraph/langgraph/pregel/_checkpoint.py:37-121`). Concurrent multi-node writes are funneled through the same sequential `apply_writes` per superstep, and illegal transitions (e.g. multiple Overwrites, multi-write to a `LastValue`) are rejected synchronously via `InvalidUpdateError` (`libs/langgraph/errors.py:34-99`). The system is therefore strongly disciplined at the channel interface; the primary asynchronous surface (multi-node writes from a superstep) is reconciled deterministically, but *concurrent reads while nodes execute* rely on per-thread `channel.copy()` snapshots rather than locking.

## Rating

**8 / 10** — Clear, explicitly modeled, well-tested mutation discipline at the channel abstraction. Strict monotonic versioning per channel, deterministic superstep batching, dedicated error code (`INVALID_CONCURRENT_GRAPH_UPDATE`), and an exception-based barrier against illegal transitions. Weaknesses: the in-memory mutation model is implicitly single-writer-per-superstep (no application-level lock guards accidental concurrent `BaseChannel.update` calls from multiple threads), topic/list accumulation relies on the user-supplied reducer being non-mutating and batch-invariant (documented but unenforced), and observability of mutations outside of checkpoint saves is mediated only through node-level `Send`/write deques rather than a structured mutation event log.

## Evidence Collected

Every entry includes a path and line number or symbol.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Channel interface | `BaseChannel` ABC defines the only mutation method `update(values) -> bool` plus lifecycle hooks `consume()`, `finish()`, `get()`, `from_checkpoint()`, `checkpoint()` | `libs/langgraph/langgraph/channels/base.py:19-121` |
| Channel catalog | Exported channels: `LastValue`, `LastValueAfterFinish`, `Topic`, `AnyValue`, `EphemeralValue`, `UntrackedValue`, `BinaryOperatorAggregate`, `DeltaChannel`, `NamedBarrierValue`, `NamedBarrierValueAfterFinish` | `libs/langgraph/langgraph/channels/__init__.py:1-29` |
| Single-reducer semantics | `BinaryOperatorAggregate.update()` applies the configured operator to the current value and each new write; rejects second `Overwrite` per super-step | `libs/langgraph/langgraph/channels/binop.py:109-130` |
| LastValue guard | `LastValue.update()` raises `InvalidUpdateError("INVALID_CONCURRENT_GRAPH_UPDATE")` if `len(values) != 1` per step | `libs/langgraph/langgraph/channels/last_value.py:56-67` |
| Barrier channel guards | `NamedBarrierValue.update` raises `InvalidUpdateError` if a value is not in the declared `names` set; `EphemeralValue`/`UntrackedValue` raise when `guard=True` and multiple values arrive | `libs/langgraph/langgraph/channels/named_barrier_value.py:56-67`, `libs/langgraph/langgraph/channels/ephemeral_value.py:55-68`, `libs/langgraph/langgraph/channels/untracked_value.py:56-65` |
| Topic accumulation | `Topic.update()` resets on each call when `accumulate=False` and extends; legally accepts sequences | `libs/langgraph/langgraph/channels/topic.py:77-85` |
| DeltaChannel | Reducer pattern with explicit snapshot cadence (`snapshot_frequency`, `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`) and replay via `replay_writes`; raises on duplicate `Overwrite` | `libs/langgraph/langgraph/channels/delta.py:65-204` |
| Apply-writes | Single funnel `apply_writes(checkpoint, channels, tasks, get_next_version, trigger_to_nodes)` sorts tasks, updates `versions_seen`, computes next version, calls `channels[chan].update(vals)`, bumps version, handles `consume()`, post-step `finish()` for terminal super-step | `libs/langgraph/langgraph/pregel/_algo.py:232-345` |
| Version monotonic counter | Default `increment(current, channel)` → `current + 1 if current else 1`; checkpointer can override via `BaseCheckpointSaver.get_next_version` (int / `f"{next_v:032}.{next_h:016}"`) | `libs/langgraph/langgraph/pregel/_algo.py:227-229`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:692-711`, `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628` |
| Trigger gating | `_triggers(channels, channel_versions, versions_seen.get(name), ...)` runs a node only when a trigger channel’s version exceeds the node’s `seen` version — “optimistic scheduling with re-validation per step” | `libs/langgraph/langgraph/pregel/_algo.py:1260-1277` |
| Delta snapshot predictor | Pure-predicate `delta_channels_to_snapshot` decides which `DeltaChannel`s need a `_DeltaSnapshot` blob this step (updates≥freq OR supersteps≥bound) | `libs/langgraph/langgraph/pregel/_checkpoint.py:37-58` |
| Checkpoint serializer helpers | `create_checkpoint` and `channels_from_checkpoint` walk `channel_versions` + `versions_seen`; delta channels absent from `channel_values` are replayed from `saver.get_delta_channel_history` | `libs/langgraph/langgraph/pregel/_checkpoint.py:61-184` |
| `update_state` external mutation | `bulk_update_state` writes a synthetic `PregelTaskWrites` with `as_node=INPUT` (or END), routes through `apply_writes`, persists via `checkpointer.put`/`put_writes`, validates that `as_node` exists in `self.nodes` and is non-None when multiple updates are passed | `libs/langgraph/langgraph/pregel/main.py:1589-2050` (sync) and `2062-2525` (async) |
| `apply_writes` ignores unknown channels | Writes to unknown channel names are dropped with `logger.warning` | `libs/langgraph/langgraph/pregel/_algo.py:308-313` |
| Reserved channel names | `NO_WRITES`, `PUSH`, `RESUME`, `INTERRUPT`, `RETURN`, `ERROR`, `ERROR_SOURCE_NODE` are reserved send-targets, not real channels | `libs/langgraph/langgraph/pregel/_algo.py:298-307`, `libs/checkpoint/langgraph/checkpoint/serde/types.py:12-16` |
| Error model | `ErrorCode` enum with `INVALID_CONCURRENT_GRAPH_UPDATE`, `INVALID_GRAPH_NODE_RETURN_VALUE` etc.; `InvalidUpdateError(Exception)`; `create_error_message` attaches URL | `libs/langgraph/langgraph/errors.py:34-99` |
| Storage upsert discipline | SQLite saver: `INSERT OR REPLACE` on checkpoints, `INSERT OR IGNORE` on first-time writes; Postgres saver: `UPSERT_CHECKPOINTS_SQL` and `ON CONFLICT … DO UPDATE SET …` on writes; explicitly *no* optimistic locking columns | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:418-482`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:131-159` |
| Per-write id uniqueness | `WRITES_IDX_MAP` reserves negative indices for special writes (`ERROR=-1`, `SCHEDULED=-2`, `INTERRUPT=-3`, `RESUME=-4`) to keep regular writes keyed by index | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| `InMemorySaver` storage shape | Distinct dicts per `(thread_id, checkpoint_ns, checkpoint_id)`: `storage`, `writes`, `blobs`. Writes keyed by `(task_id, idx)`. Blobs keyed by `(thread, ns, channel, version)` | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:68-94` |
| Migration discipline | `_migrate_checkpoint` rewrites channel names + versions when checkpoint schema version `<4` (e.g. `start:`→`branch:to:`, `pending_sends`→`__pregel_tasks`) | `libs/langgraph/langgraph/graph/state.py:1612-1699`, `libs/langgraph/langgraph/pregel/main.py:1135-1142` |
| `apply_writes` deterministic order | Tasks sorted by `task_path_str(t.path[:3])` before write application | `libs/langgraph/langgraph/pregel/_algo.py:256` |
| Future-safe writes | `PregelTaskWrites(path, name, writes=deque[...], triggers)` — writes are a `deque` (chosen because `deque.extend` is thread-safe; the docstring comment appears at call sites) | `libs/langgraph/langgraph/pregel/_algo.py:110-117`, `libs/langgraph/langgraph/pregel/main.py:723, 905, 1073, 1978, 2445` |
| Atomic counter for sub-ids | `LazyAtomicCounter` uses module-level `LAZY_ATOMIC_COUNTER_LOCK` to safely lazily-init `itertools.count(0).__next__` for thread-safe monotonic counters used by `call_counter`, `interrupt_counter`, `subgraph_counter` | `libs/langgraph/langgraph/pregel/_algo.py:1333-1345`, `1423-1439` |
| FuturesDict lock | `PregelRunner.FuturesDict.__setitem__` increments `counter` under `self.lock`; `on_done` decrements and signals an `event` (`threading.Event`/`asyncio.Event`) | `libs/langgraph/langgraph/pregel/_runner.py:75-133` |
| Stop condition | `_should_stop_others(futs)` checks whether any future raised a non-`GraphBubbleUp` exception → fail/cancel siblings; `GraphInterrupt` is treated as cooperative, not fatal | `libs/langgraph/langgraph/pregel/_runner.py:616-634`, `libs/langgraph/langgraph/pregel/_runner.py:650-685` |
| Concurrent writes unit test | `test_concurrent_emit_sends` exercises multiple sibling nodes each producing `Send` packets; verified that emit order is deterministic under superstep batching | `libs/langgraph/tests/test_pregel.py:1159-1203` |
| Illegal-transition tests | `test_invalid_update_last_value_handlers` (raises `InvalidUpdateError`), `test_overwrite_parallel_error` (`Can receive only one Overwrite value per super-step.`), `test_multiple_writes_same_channel_from_same_node` (legal multiple-writes + reducer) | `libs/langgraph/tests/test_pregel.py:121, 766-783, 9353-9388, 9079-9153` |
| Concurrent-execution thread safety | `test_concurrent_execution_thread_safety` starts 10 threads invoking the same `node(0)` graph; each thread asserts its own result is independent (`results` is a `deque`, thread-safe) | `libs/langgraph/tests/test_pregel.py:5349-5384` |
| Channel-level illegal-transition tests | `tests/test_channels.py:test_last_value`, `test_topic`, `test_topic_accumulate` exercise the channel-level `update()` guards | `libs/langgraph/tests/test_channels.py:31-72` |
| Delta-channel exit mode test | `tests/test_delta_channel_exit_mode.py` covers the exit/async-durability commit ordering for delta snapshots | `libs/langgraph/tests/test_delta_channel_exit_mode.py` |
| Delta-channel superstep bound test | `tests/test_delta_channel_supersteps_bound.py` covers the system-wide max-supersteps bound on ancestor replay | `libs/langgraph/tests/test_delta_channel_supersteps_bound.py` |
| Checkpoint conformity | `test_validate_memory_base` runs the conformance suite against `InMemorySaver`; `BaseCheckpointSaver` exposes a `validate`+`@checkpointer_test` registration pattern | `libs/checkpoint-conformance/tests/test_validate_memory.py:1-21` |
| Delta migration regression | `tests/test_delta_channel_migration.py` ensures old `BinaryOperatorAggregate` checkpoints can be replayed through the new channel system | `libs/langgraph/tests/test_delta_channel_migration.py` |
| Mutation observability via streams | ChannelWrite writes flow into a `deque` via `CONFIG_KEY_SEND` (thread-safe); `CONFIG_KEY_SEND: writes.extend` is the standard wiring (see call sites) | `libs/langgraph/langgraph/pregel/main.py:723, 905, 1073, 1978, 2445`; `libs/langgraph/langgraph/pregel/_algo.py:1334-1340` |
| Streaming mutation observability | `run_stream.py` and `stream_channel.py` introduce arrival-ordered, stamp-ordered events (push stamps, `StreamMux` interleave), letting observers watch mutation-driven stream events in arrival order rather than registration order | `libs/langgraph/tests/test_interleave_arrival_order.py:1-359`, `libs/langgraph/langgraph/stream/run_stream.py:309`, `libs/langgraph/langgraph/stream/stream_channel.py:292-314` |
| `RunControl` (cooperative drain) | Single attribute write on `_drain_reason` (no lock needed for the signal); commits a checkpoint, raises `GraphDrained` | `libs/langgraph/langgraph/runtime.py:79-104`, `libs/langgraph/langgraph/errors.py:54-64` |
| `Overwrite` channel type | `Overwrite(value)` wraps a value that bypasses a reducer for one super-step; multiple Overwrites are illegal | `libs/langgraph/langgraph/types.py:937-978`, `libs/langgraph/langgraph/channels/binop.py:109-130` |
| `Command` primitive | `Command(update, resume, goto, graph)` resolved into typed `(channel, value)` writes via `map_command` / `_update_as_tuples`; routing functions recognize `Send` and `branch:to:<node>` | `libs/langgraph/langgraph/types.py:758-808`, `libs/langgraph/langgraph/pregel/_io.py:56-78` |
| Scratchpad semantics | `PregelScratchpad` carries `step`, `stop`, per-task `call_counter`, `interrupt_counter`, `subgraph_counter`, and `resume` history — scoped *to the task*, preventing cross-task bleed | `libs/langgraph/langgraph/_internal/_scratchpad.py:1-19`, `libs/langgraph/langgraph/pregel/_algo.py:1280-1345` |
| `interrupt` resume discipline | `interrupt()` reads `scratchpad.resume` by index, asserts alignment (`assert len(scratchpad.resume) == idx`), yields value if a resume is queued; otherwise raises `GraphInterrupt` | `libs/langgraph/langgraph/types.py:811-934` |
| `LastValueAfterFinish` / `NamedBarrierValueAfterFinish` | Channels gated by `finish()` step predicate; consume-after-finish, gating makes illegal read access (`EmptyChannelError`) impossible before the run ends | `libs/langgraph/langgraph/channels/last_value.py:81-151`, `libs/langgraph/langgraph/channels/named_barrier_value.py:84-167` |
| EmptyChannelError | Raised by `BaseChannel.get()` when channel was never updated — explicit error type prevents accidental silent None fallback | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:745-749` |
| Node-level error handler mechanism | `NodeError` dataclass passed to `error_handler=...` nodes; routed via `schedule_error_handler` with `_handled_exception_ids` set so the same exception won’t re-trigger crash semantics | `libs/langgraph/langgraph/errors.py:148-165`, `libs/langgraph/langgraph/pregel/_runner.py:165-248` |

## Answers to Dimension Questions

**1. Can any module mutate state?**

Strictly bounded. The only mutations to channel state are `update()`, `consume()`, `finish()` on `BaseChannel` (`libs/langgraph/langgraph/channels/base.py:90-120`) — invoked exclusively from `apply_writes` (`libs/langgraph/langgraph/pregel/_algo.py:232-345`). External callers reach channel state via `Pregel.update_state` / `bulk_update_state` (`libs/langgraph/langgraph/pregel/main.py:1589-2050`), which synthesises `PregelTaskWrites` and routes them through the same `apply_writes`; there is no public API that writes to `channel_values` directly. Management values (`Interrupt`, `Previous`, `IsLastStep`) are read-only and computed from the scratchpad (`libs/langgraph/langgraph/managed/base.py`).

**2. Are illegal transitions prevented?**

Yes, with explicit error types:
- `LastValue.update` rejects multi-write per step (`libs/langgraph/langgraph/channels/last_value.py:59-67`)
- `EphemeralValue(guard=True)` / `UntrackedValue(guard=True)` reject multi-write (`libs/langgraph/langgraph/channels/ephemeral_value.py:62-65`, `libs/langgraph/langgraph/channels/untracked_value.py:59-62`)
- `BinaryOperatorAggregate` and `DeltaChannel` reject a second `Overwrite` per super-step (`libs/langgraph/langgraph/channels/binop.py:117-127`, `libs/langgraph/langgraph/channels/delta.py:162-184`)
- `NamedBarrierValue` rejects values not in its declared `names` set (`libs/langgraph/langgraph/channels/named_barrier_value.py:56-67`)
- `update_state` rejects ambiguous nodes, unknown nodes, multiple updates when `as_node=END` / `INPUT` (`libs/langgraph/langgraph/pregel/main.py:1676-1794, 1933-1947`)
All use the unified `InvalidUpdateError` with `ErrorCode.INVALID_CONCURRENT_GRAPH_UPDATE` (`libs/langgraph/langgraph/errors.py:36, 90-99`).

**3. Are concurrent writes safe?**

Within a single Pregel super-step, yes — task writes are batched per channel and applied in deterministic sorted order (`libs/langgraph/langgraph/pregel/_algo.py:256, 296-330`). Across supersteps, the version-vector (`channel_versions`) plus per-node `versions_seen` (`libs/langgraph/langgraph/pregel/_algo.py:262-269, 1260-1277`) ensures a node is re-triggered iff a channel it reads was updated since the node last ran. Cross-thread safety inside `InMemorySaver.put_writes` keys each write by `(task_id, WRITES_IDX_MAP-channel-fallback-idx)` so duplicate regular writes don’t overwrite prior ones (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:499-509`). Postgres backing store is upserted on `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` so writes are idempotent under concurrent retries (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:131-159`). The model has *no* application-level lock guarding `BaseChannel.update()` from concurrent threads; the model assumes one writer thread per super-step (the Pregel executor). Tests demonstrate multi-thread invocation works because each thread builds its own channel set from its own checkpoint (`libs/langgraph/tests/test_pregel.py:5349-5384`).

**4. Are mutations observable?**

Yes, with explicit instrumentation points:
- `apply_writes` returns `updated_channels: set[str]` and bumps `checkpoint["channel_versions"]` (`libs/langgraph/langgraph/pregel/_algo.py:316-345`)
- Per-node write reordering for replay is recorded in `pending_writes_by_channel` (in-memory) and serialized via `checkpointer.put_writes(..., task_id, task_path)` (`libs/langgraph/langgraph/pregel/_algo.py:294-330`; `libs/langgraph/langgraph/pregel/main.py:2025, 2491-2492`)
- Channels emit stream events via `CONFIG_KEY_SEND` (deque.extend) — “deque.extend is thread-safe”, per call-site comments (`libs/langgraph/langgraph/pregel/main.py:1978`, `_algo.py:723, 905, 1073`)
- Stream events arrive in stamp order, not registration order (`libs/langgraph/tests/test_interleave_arrival_order.py:1-359`)
- There is *no* first-class mutation-event log channel; observability is the stream of values, updates, and tasks routed to subscribers plus the persisted checkpoint. Mutation tracing relies on LangChain callbacks (`trace=True`, `graphs` callbacks (`libs/langgraph/langgraph/pregel/_call.py:182-237`)).

**5. Can state become internally inconsistent?**

It is hard by construction, but possible in the following ways:
- A user-supplied reducer in `BinaryOperatorAggregate` / `DeltaChannel` is not validated for purity or batching-invariance beyond a docstring assertion; non-deterministic reducers produce non-replayable state (`libs/langgraph/langgraph/channels/delta.py:30-49`).
- `update_state` and the loop can fork the parent chain (`source="fork"` / `source="update"`), creating divergent versions; restarts rely on `parent_checkpoint_id` for lineage (`libs/langgraph/langgraph/pregel/main.py:1798-1825, 1730-1740`).
- Migration logic rewrites versions using `max(...)` to preserve ordering, but a buggy migration (`libs/langgraph/langgraph/graph/state.py:1612-1706`) could lose information silently; tests cover `test_checkpoint_migration.py`.
- `delta_channels_to_snapshot` is a pure predicate with no side effects (`libs/langgraph/langgraph/pregel/_checkpoint.py:37-58`); the counter bookkeeping in `_put_checkpoint` is centralized (`libs/langgraph/langgraph/pregel/_loop.py:1064-1196`).
- `UntrackedValue` channels bypass `checkpoint()` entirely (`libs/langgraph/langgraph/channels/untracked_value.py:48-50`) — for them, replay relies on adjacent state, which by design means they cannot reconstruct correctly after interrupt.

## Architectural Decisions

1. **Channels as the sole mutation interface.** Every state change is a call to `BaseChannel.update(values) -> bool`. The contract returns a bool so the Pregel loop can decide whether to bump the channel’s version (`libs/langgraph/langgraph/pregel/_algo.py:319-323`). This keeps the model uniform and gives each channel total local authority over its semantics.
2. **Sequential per-superstep batching.** Even with `BackgroundExecutor` / `AsyncBackgroundExecutor` running node bodies concurrently (`libs/langgraph/langgraph/pregel/_executor.py`), their *writes* are not applied in parallel: `apply_writes` groups them by channel, sorts tasks deterministically, then runs each channel’s `update()` exactly once per super-step. Concurrent threads append to a thread-safe `deque` (`CONFIG_KEY_SEND`) and the commit pipeline is the single mutator (`libs/langgraph/langgraph/pregel/_runner.py:574-613`).
3. **Monotonic version vector + per-node seen map.** Triggering uses a monotone `versions_seen[node][channel]` (`libs/langgraph/langgraph/pregel/_algo.py:262-269`); this is the optimistic version of how the act of *reading* a channel updates local state, with implicit “MVCC via version” semantics. `_triggers` is the equivalent of version-based optimistic concurrency check (`libs/langgraph/langgraph/pregel/_algo.py:1260-1277`).
4. **Reducer-based composition vs. overwrite.** Channels are typed: reducers compose (`BinaryOperatorAggregate`/`Topic`/`DeltaChannel`) versus single-assignment (`LastValue`/`AnyValue`/`EphemeralValue`/`UntrackedValue`). `Overwrite` deliberately bypasses the reducer and the bypass is gated by an error for duplicate uses (`libs/langgraph/langgraph/channels/binop.py:117-127`).
5. **Channel-aware checkpointing.** `create_checkpoint` walks `channel_versions` so channels added later don’t appear in older checkpoints; `_DeltaSnapshot` blobs bound the ancestor-walk depth even when channels stop receiving writes (`libs/langgraph/langgraph/pregel/_checkpoint.py:37-121`). Bounded supersteps enforced via `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000) prevents unbounded replays (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-85`).
6. **Error-as-a-signal model.** `InvalidUpdateError`, `GraphInterrupt`, `GraphBubbleUp`, `NodeError`, and `NodeTimeoutError` form a small, explicit error hierarchy for control flow (`libs/langgraph/langgraph/errors.py:34-241`). Cooperative draining via `RunControl.request_drain()` writes a single attribute and surfaces as `GraphDrained` (`libs/langgraph/langgraph/runtime.py:79-104`).
7. **Schema-versioned checkpointer format.** `LATEST_VERSION=4` (`libs/langgraph/langgraph/pregel/_checkpoint.py:21`); on read, `_migrate_checkpoint` rewrites channel names and bumps versions so older checkpoints continue to load (`libs/langgraph/langgraph/graph/state.py:1612-1706`, `libs/langgraph/langgraph/pregel/main.py:1135-1142`).
8. **Lazy atomic counters with module-level init lock.** `LazyAtomicCounter` uses double-checked locking on first call to obtain a thread-safe monotonic counter (`libs/langgraph/langgraph/pregel/_algo.py:1423-1439`). This is the only place an explicit lock guards state creation.

## Notable Patterns

- **Single mutation point**: `apply_writes` is the single funnel for state mutation across Pregel versions.
- **Dedicated error type**: `InvalidUpdateError` with structured `ErrorCode` makes illegal transitions recoverable and machine-readable.
- **Reserved channel namespace**: `NO_WRITES`, `PUSH`, `RESUME`, `INTERRUPT`, `RETURN`, `ERROR`, `ERROR_SOURCE_NODE`, `TASKS` allow out-of-band control without leaking into user state (`libs/checkpoint/langgraph/checkpoint/serde/types.py:12-16`).
- **Thread-safe write buffer**: `deque[...].extend` is used deliberately at every site where user code emits writes (`libs/langgraph/langgraph/pregel/main.py:723, 905, 1073, 1978, 2445`).
- **Pure snapshot predicates**: `delta_channels_to_snapshot` and `_triggers` are pure functions of state, making them testable in isolation.
- **Barrier channels**: `NamedBarrierValue` and `LastValueAfterFinish` make “wait-for-many / wait-until-finish” first-class rather than ad-hoc.
- **Schema-versioned storage**: `Checkpoint.v` + `_migrate_checkpoint` decouple storage evolution from runtime evolution.
- **Atomic lazy init for counters**: `LazyAtomicCounter` with double-checked locking is a self-contained idiom for monotonic id generation.
- **Stop condition injection**: `FuturesDict.should_stop` lets the runner treat graph-error-handled exceptions as non-fatal (`libs/langgraph/langgraph/pregel/_runner.py:75-82`).
- **Durability mode**: `_put_checkpoint` gates writes by `durability != "exit"`, supporting “commit-as-you-go” vs “commit-at-end” (`libs/langgraph/langgraph/pregel/_loop.py:1116-1118`).
- **Conformance test harness**: `@checkpointer_test`/`validate` decorator (referenced by `libs/checkpoint-conformance/tests/test_validate_memory.py`) enforces that `BaseCheckpointSaver` implementations all meet the same mutation contract.

## Tradeoffs

- **Strict per-superstep batching assumes single-writer-per-step.** It is the engine’s responsibility — not the channel’s — to ensure that no two threads call `update()` on the same instance concurrently. The `__slots__` discipline + `deque.extend` makes this safe *within* the framework, but a user-supplied channel subclass that bypasses these will break ordering.
- **Reducer purity is contractual, not enforced.** `DeltaChannel` documents associativity across folds, but doesn’t test reducer purity directly outside `tests/test_channels.py` and `tests/test_delta_channel_*`.
- **InMemorySaver is non-thread-safe across process.** Storage lives in process-local dicts (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:68-94`); cross-process sharing has to use SQLite/Postgres. The DocLayout is otherwise correct.
- **No “schema” for state transitions.** There is no centralized state machine describing legal transitions (e.g., `idle → running → interrupted → finished`). Channels express their own minisets (`seen/names`, `finished`, `MISSING`). State consistency is enforced by composition rather than a typed state enum.
- **`UntrackedValue` and `EphemeralValue` are deliberately ephemeral** — they imply that information disappears after one step. If a user accidentally reads them after they’re cleared, `EmptyChannelError` is the only signal (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:745-749`); there is no compile-time warning.
- **Strict monotonically increasing versions stored as strings** in Postgres (`f"{next_v:032}.{next_h:016}"` in `InMemorySaver.get_next_version`, `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628`) — comparisons rely on Python’s lexicographic ordering of zero-padded strings vs. numerics; mixed checkpointer + non-checkpointer runs could subtly vary.
- **`update_state` cannot remove single channels** — only en bloc via `as_node=END` (`libs/langgraph/langgraph/pregel/main.py:1677-1743`).

## Failure Modes / Edge Cases

- **Overwrite collision**: Parallel nodes each sending an `Overwrite` for the same channel raises `InvalidUpdateError` (`libs/langgraph/tests/test_pregel.py:9353-9388`).
- **LastValue parallel writes**: Two nodes writing the same single-assignment channel raise `InvalidUpdateError("INVALID_CONCURRENT_GRAPH_UPDATE")` (`libs/langgraph/tests/test_pregel.py:766-783`).
- **Barrier with unknown value**: `NamedBarrierValue` raises `InvalidUpdateError` if the produced value isn’t in the declared `names` set (`libs/langgraph/langgraph/channels/named_barrier_value.py:56-67`).
- **Async finalisation gap (delta channels, exit mode)**: If `apply_writes` doesn’t bump a channel’s version this step, the snapshot blob could be silently dropped; `_put_checkpoint` manually bumps the version in that case so it’s included in `new_versions` (`libs/langgraph/langgraph/pregel/_checkpoint.py:104-107`).
- **Concurrent put_writes collisions**: Checkpointers use `(task_id, idx)` keying so a duplicate regular write is *ignored* (not overwritten) when all writes are special (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:498-509`).
- **Pruning risk (DeltaChannel)**: Naive `keep_latest` pruning can drop ancestor writes that delta replay needs; the base class docstring calls this out and warns custom implementations must be DeltaChannel-aware (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-414`).
- **Untracked values lost on checkpointing**: An `UntrackedValue` is never restored from `channel_values`, so on resume after a cross-process checkpoint the value is `MISSING` (`libs/langgraph/langgraph/channels/untracked_value.py:48-50`).
- **`asyncio.CancelledError` semantics**: User-raised `CancelledError` is converted into a `NodeCancelledError` so it flows through the normal failure path, not the framework-initiated silent cancellation (`libs/langgraph/langgraph/errors.py:168-188`).
- **Cooperative drain vs. fatal errors**: `_should_stop_others` decides whether to cancel sibling tasks; `GraphInterrupt` and routed-to-error-handler exceptions are excluded from stop (`libs/langgraph/langgraph/pregel/_runner.py:616-634`).
- **Migration version drift**: `CheckpointMetadata` carries schema-version `v`; migration is gated on `checkpoint["v"] < 3` / `< 4`; a custom serializer that lazily re-serialises legacy checkpoints could surface in unexpected ways (`libs/langgraph/langgraph/graph/state.py:1621-1626`).

## Future Considerations

- A version-checked `update(channel, expected_version, value)` path could let library code implement optimistic concurrency for user-level writes without depending on the Pregel loop.
- Pinned `DeltaChannel` reducers (compile-time purity checks) would convert the current contract-only invariant into a runtime guarantee.
- A first-class mutation trace / event log (similar to `StreamMux` for stream events, but for atomic channel updates) would make “what wrote what, when” easier to debug, especially for `DataChannel` failures.
- `Overwrite` collision is currently a hard error; some teams may want a more nuanced policy (last-write-wins, etc.).
- Checkpoint concurrency: SQLite uses `INSERT OR IGNORE` for non-reserved writes (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:466-470`); for high-contention workloads, an explicit transactional UPSERT or `BEGIN IMMEDIATE` could prevent the lost-write window.
- Centralising `versions_seen` migrations under one helper instead of replicating the rename logic in three places (`libs/langgraph/langgraph/graph/state.py:1612-1706`) would reduce drift risk.

## Questions / Gaps

- Are there documented guarantees for what happens if a `LastValue` step has zero writes (currently `_put_checkpoint` writes a sentinel and downstream still sees the prior version)? The tests in `tests/test_pregel.py` cover zero-write paths but no formal doc was found.
- Does the framework guarantee per-thread isolation when the *same* `BaseChannel` instance is shared across threads? Source suggests not — `__slots__` for safety, but no documentation says “never share a channel across threads” explicitly. `No clear evidence found` in the source comments.
- The `DeltaChannel` docstring asserts “Reducers must be deterministic and batching-invariant”, but there is no built-in verifier; the failure mode is silent replay divergence. Tests assert functional behavior, not formal properties.
- `WRITES_IDX_MAP` reserves negative indices; how this interacts with user-defined `TaskID` collisions is not deeply tested. `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:498-509` keys by `(task_id, WRITES_IDX_MAP.get(c, idx))`, so collisions are deliberately bounded.
- `CheckpointMetadata` declares a `counters_since_delta_snapshot` field documented as beta — its persistence semantics in the Postgres wire format (e.g. tuple vs list) are an open coupling note (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-85`).

---

Generated by `02.04-mutation-discipline-and-state-transitions` against `langgraph`.
