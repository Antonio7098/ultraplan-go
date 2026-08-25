# Source Analysis: langgraph

## 02.01 — State Taxonomy and Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `langgraph`, `checkpoint`, `checkpoint-postgres`, `checkpoint-sqlite`, `checkpoint-conformance`, `prebuilt`, `cli`, `sdk-py`) |
| Analyzed | 2026-07-03 |

## Summary

LangGraph exposes one of the most explicit state taxonomies of any agent runtime: each kind of state has a dedicated owner module, an explicit on-disk representation, and an explicit lifecycle method (`update`, `checkpoint`, `from_checkpoint`, `consume`, `finish`). State is divided into (1) **graph state** owned by `BaseChannel` subclasses and persisted through the `BaseCheckpointSaver`; (2) **checkpoint metadata** owned by `CheckpointMetadata`/`CheckpointTuple` and serialised alongside checkpoints; (3) **task/control state** owned by `PregelScratchpad` and the `RUN_CONFIG` keys in `_internal/_constants.py`; (4) **run-scoped execution state** owned by `Runtime`/`ExecutionInfo`/`RunControl`; (5) **long-term memory** owned by `BaseStore`; (6) **node result cache** owned by `BaseCache`; and (7) **approval/interrupt state** encoded as `Interrupt` records inside `pending_writes`. The repo documents ownership in source: `channels` owns the per-key state machine (`channels/base.py:19`), `checkpoint.base` owns durable state (`checkpoint/base/__init__.py:92` for `Checkpoint`, `:139` for `CheckpointTuple`, `:176` for `BaseCheckpointSaver`), `store.base` owns cross-thread memory (`store/base/__init__.py:51` for `Item`, `:700` for `BaseStore`), `cache.base` owns per-node result caching (`cache/base/__init__.py:15`), and `managed.base` owns schema-driven scratch values (`managed/base.py:18`). Durability is also explicit: `Durability = Literal["sync", "async", "exit"]` (`types.py:87`) controls when pending writes become durable, and the channel-class `UntrackedValue` (`channels/untracked_value.py:15`) is intentionally never persisted while `EphemeralValue` (`channels/ephemeral_value.py:15`) is cleared after one step.

## Rating

**9/10 — Mature, durable, observable, extensible, proven under failure.**

Rationale:

- The model is exhaustive and documented inline: every state class has a documented owner (`channels/base.py:19-121`, `managed/base.py:18-31`, `checkpoint/base/__init__.py:176-722`, `store/base/__init__.py:700-1252`, `cache/base/__init__.py:15-48`, `runtime.py:124-238`).
- Crash semantics are explicit and tested: `BaseCheckpointSaver.get_delta_channel_history` (`checkpoint/base/__init__.py:582-690`) walks `parent_config` to reconstruct `DeltaChannel` state from `pending_writes` after a crash; `PregelLoop.put_writes` waits for delta-write futures before the next `_checkpointer_put_after_previous` (`pregel/_loop.py:408-498`, `:1507-1524`).
- Operational safeguards include durability modes (`types.py:87-93`), capability detection (`checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-96`), `delete_thread`/`prune`/`copy_thread` (`checkpoint/base/__init__.py:320-415`), `DeltaChannel`'s `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` bound (`pregel/_checkpoint.py:37-58`), a TTL sweeper (`store/postgres/aio.py:311-385`), and msgpack allowlists (`serde/jsonplus.py:97-...`).
- A conformance test suite (`checkpoint-conformance`) validates every checkpointer implementation against the contract; `InMemorySaver` passes the base suite (`checkpoint-conformance/tests/test_validate_memory.py:17-21`).

Where it falls short of 10: (a) `PregelScratchpad` (the per-task counter and resume list) lives in process memory and is not part of the checkpoint, so a crashed process loses unconsumed interrupt counter offsets for any task that hadn't yet checkpointed (mitigated by synchronously persisting writes for `durability="sync"` but not fully eliminated for `"async"`/`"exit"`); (b) the docstrings admit `DeltaChannel` is "in beta" with non-stable APIs (`channels/delta.py:29-37`); (c) `Store` and `Cache` are conceptually "memory" but their `BaseStore`/`BaseCache` abstractions are not versioned together with `Checkpoint` — the write schema for `Item` is JSON-flat only (`store/base/__init__.py:51-115`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Channel state base class (per-key state machine) | `BaseChannel[Value, Update, Checkpoint]` with `checkpoint()` / `from_checkpoint()` / `update()` / `get()` / `consume()` / `finish()` | `sources/langgraph/libs/langgraph/langgraph/channels/base.py:19-121` |
| `LastValue` reducer — last-write-wins key | Reduces concurrent writes to the last value, raises `InvalidUpdateError` on multi-write without reducer | `sources/langgraph/libs/langgraph/langgraph/channels/last_value.py:20-78` |
| `LastValueAfterFinish` — value held until graph finishes | `consume()` clears after `finish()` was called | `sources/langgraph/libs/langgraph/langgraph/channels/last_value.py:81-151` |
| `EphemeralValue` — cleared after one step | "Stores the value received in the step immediately preceding, clears after." | `sources/langgraph/libs/langgraph/langgraph/channels/ephemeral_value.py:15-79` |
| `UntrackedValue` — never checkpointed | `checkpoint()` always returns `MISSING` | `sources/langgraph/libs/langgraph/langgraph/channels/untracked_value.py:15-73` |
| `BinaryOperatorAggregate` — reducer-backed aggregate channel | Calls a binary operator `operator(value, new_value)`; `Overwrite` bypasses reducer | `sources/langgraph/libs/langgraph/langgraph/channels/binop.py:51-141` |
| `Topic` — pub/sub for `Send` | Used as the reserved `TASKS` channel | `sources/langgraph/libs/langgraph/langgraph/channels/topic.py:23-...` |
| `DeltaChannel` — snapshot + replay store | Sentinel in `channel_values`, reconstruction via ancestor walk | `sources/langgraph/libs/langgraph/langgraph/channels/delta.py:25-204` |
| `ManagedValue` / `ManagedValueSpec` — derived state from scratchpad | `ManagedValue.get(scratchpad)` returns a value computed per superstep | `sources/langgraph/libs/langgraph/langgraph/managed/base.py:18-31` |
| Built-in managed values | `IsLastStep`, `RemainingSteps` — read step counter from `PregelScratchpad` | `sources/langgraph/libs/langgraph/langgraph/managed/is_last_step.py:9-24` |
| `Runtime` — run-scoped context, store, stream_writer, etc. | Per-invocation bag injected into nodes | `sources/langgraph/libs/langgraph/langgraph/runtime.py:124-258` |
| `ExecutionInfo` — read-only execution metadata | Carries `checkpoint_id`, `checkpoint_ns`, `task_id`, `node_attempt` | `sources/langgraph/libs/langgraph/langgraph/runtime.py:26-58` |
| `RunControl` — cooperative drain signal | `request_drain()` sets `_drain_reason`; not part of checkpoint | `sources/langgraph/libs/langgraph/langgraph/runtime.py:79-104` |
| `PregelScratchpad` — per-task mutable state | Step counter, interrupt counter, subgraph counter, resume list | `sources/langgraph/libs/langgraph/langgraph/_internal/_scratchpad.py:8-19` |
| `Runtime` access helper | `get_runtime()` reads from `config["configurable"][CONFIG_KEY_RUNTIME]` | `sources/langgraph/libs/langgraph/langgraph/runtime.py:296-310` |
| Run-time config keys | `_internal/_constants.py` defines `CONFIG_KEY_*` keys (checkpointer, store, runtime, scratchpad, send, read, etc.) | `sources/langgraph/libs/langgraph/langgraph/_internal/_constants.py:13-80` |
| Reserved write keys | `INPUT`, `INTERRUPT`, `RESUME`, `ERROR`, `ERROR_SOURCE_NODE`, `NO_WRITES`, `TASKS`, `RETURN`, `PREVIOUS` | `sources/langgraph/libs/langgraph/langgraph/_internal/_constants.py:7-22` |
| `Checkpoint` schema | `v`, `id`, `ts`, `channel_values`, `channel_versions`, `versions_seen`, `updated_channels` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-124` |
| `CheckpointMetadata` | `source`, `step`, `parents`, `run_id`, `counters_since_delta_snapshot` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| `CheckpointTuple` | `config`, `checkpoint`, `metadata`, `parent_config`, `pending_writes` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:139-146` |
| `BaseCheckpointSaver` interface | `get`, `get_tuple`, `list`, `put`, `put_writes`, `delete_thread`, `delete_for_runs`, `copy_thread`, `prune`, `get_delta_channel_history` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:176-722` |
| `PendingWrite` | Tuple of `(task_id, channel, value)` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:31` |
| `DeltaChannelHistory` | Per-channel `writes` + optional `seed` for ancestor walk | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:149-173` |
| Default `get_next_version` | Integer-monotonic versioning | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:692-711` |
| Reserved write-key index map | `WRITES_IDX_MAP = {ERROR: -1, SCHEDULED: -2, INTERRUPT: -3, RESUME: -4}` | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| Excluded metadata keys | `thread_id`, `checkpoint_id`, `checkpoint_ns`, `checkpoint_map`, langgraph-step/node/triggers/path/ns | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/base/__init__.py:797-807` |
| Postgres schema (state of record) | `checkpoints`, `checkpoint_blobs`, `checkpoint_writes` tables | `sources/langgraph/libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91` |
| Postgres version scheme | `"{next_v:032}.{next_h:016}"` — supports replay ordering across concurrent writers | `sources/langgraph/libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:543-552` |
| SQLite schema | `checkpoints`, `writes` tables — inline channel_values (no JSONB) | `sources/langgraph/libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:139-163` |
| `InMemorySaver` storage | `thread_id → checkpoint_ns → checkpoint_id → (serialized checkpoint, metadata, parent)`; separate `writes` and `blobs` dicts | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/memory/__init__.py:68-99` |
| `_DeltaSnapshot` blob shape | Wraps a single value; terminates ancestor walk | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/serde/types.py:19-31` |
| `ChannelProtocol` runtime shape | Mirrors `BaseChannel` for the saver's needs | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/serde/types.py:39-55` |
| `JsonPlusSerializer` | ormsgpack + pickle fallback; `STRICT_MSGPACK_ENABLED` gate | `sources/langgraph/libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:82-100` |
| `BaseStore` (long-term memory) | `get`, `search`, `put`, `delete`, `list_namespaces`, `batch`, `abatch` | `sources/langgraph/libs/checkpoint/langgraph/store/base/__init__.py:700-1252` |
| `Item` | `value`, `key`, `namespace`, `created_at`, `updated_at` | `sources/langgraph/libs/checkpoint/langgraph/store/base/__init__.py:51-115` |
| Store operations | `GetOp`, `SearchOp`, `PutOp`, `ListNamespacesOp` | `sources/langgraph/libs/checkpoint/langgraph/store/base/__init__.py:157-537` |
| `BaseCache` (per-node result cache) | `get`, `set`, `clear` keyed by `(Namespace, str)` | `sources/langgraph/libs/checkpoint/langgraph/cache/base/__init__.py:11-48` |
| `CachePolicy` | `key_func` (default = pickle of frozen args), `ttl` | `sources/langgraph/libs/langgraph/langgraph/types.py:515-527` |
| `default_cache_key` | pickle-based, frozen via `_freeze()` (handles dict/list/ndarray) | `sources/langgraph/libs/langgraph/langgraph/_internal/_cache.py:7-31` |
| `Interrupt` (approval state) | `value`, `id`; `id = xxh3_128_hexdigest("|".join(ns))` derived from checkpoint ns | `sources/langgraph/libs/langgraph/langgraph/types.py:533-588` |
| `interrupt()` function | Reads scratchpad resume list; raises `GraphInterrupt` on first call | `sources/langgraph/libs/langgraph/langgraph/types.py:811-934` |
| `Command` (state update + navigation) | `update`, `resume`, `goto`, `graph=Command.PARENT` | `sources/langgraph/libs/langgraph/langgraph/types.py:758-808` |
| `Durability` | `Literal["sync", "async", "exit"]` controlling checkpoint writes | `sources/langgraph/libs/langgraph/langgraph/types.py:87-93` |
| `StateSnapshot` | `values`, `next`, `config`, `metadata`, `parent_config`, `tasks`, `interrupts` | `sources/langgraph/libs/langgraph/langgraph/types.py:643-661` |
| `StateGraph.compile()` | Wires channels, managed, store, cache, checkpointer into `CompiledStateGraph` | `sources/langgraph/libs/langgraph/langgraph/graph/state.py:1164-1388` |
| Schema → channel resolution | `_get_channel()` / `_is_field_channel()` / `_is_field_binop()` / `_is_field_managed_value()` | `sources/langgraph/libs/langgraph/langgraph/graph/state.py:1801-1927` |
| Pregel runtime fields | `self.channels`, `self.checkpointer`, `self.store`, `self.cache`, `self.context_schema` | `sources/langgraph/libs/langgraph/langgraph/pregel/main.py:799-832` |
| PregelLoop state | `self.checkpoint`, `self.checkpoint_config`, `self.checkpoint_metadata`, `self.checkpoint_pending_writes`, `self.managed` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:240-247` |
| Sync loop write durability | `_checkpointer_put_after_previous` waits on `_delta_write_futs` before `put` | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:1507-1524` |
| Delta-channel write-future tracking | `_delta_write_futs`, `_error_handler_write_futs` lists | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:199-221` |
| `put_writes` API call site | `_loop.put_writes` invokes `checkpointer.put_writes` per task | `sources/langgraph/libs/langgraph/langgraph/pregel/_loop.py:408-541` |
| Snapshot decision | `delta_channels_to_snapshot()` decides when to materialize `_DeltaSnapshot` | `sources/langgraph/libs/langgraph/langgraph/pregel/_checkpoint.py:37-58` |
| `create_checkpoint` | Builds the next `Checkpoint` from live channel state and prior checkpoint | `sources/langgraph/libs/langgraph/langgraph/pregel/_checkpoint.py:61-121` |
| `channels_from_checkpoint` | Hydrates channels from a `Checkpoint`; uses `saver.get_delta_channel_history` for `DeltaChannel` reconstruction | `sources/langgraph/libs/langgraph/langgraph/pregel/_checkpoint.py:136-226` |
| Functional API `previous` | `@entrypoint` writes to `PREVIOUS` channel; `Runtime.previous` exposes it | `sources/langgraph/libs/langgraph/langgraph/func/__init__.py:587-604`, `runtime.py:219-223` |
| `ReplayState` (time-travel) | Tracks parent replay point + visited subgraph namespaces | `sources/langgraph/libs/langgraph/langgraph/_internal/_replay.py:14-90` |
| Conformance capability enum | `PUT`, `PUT_WRITES`, `GET_TUPLE`, `LIST`, `DELETE_THREAD`, `DELETE_FOR_RUNS`, `COPY_THREAD`, `PRUNE`, `DELTA_CHANNEL_HISTORY` | `sources/langgraph/libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-50` |
| Memory-saver delta tests | `test_load_blobs_omits_delta_channel`, `test_get_channel_writes_*` | `sources/langgraph/libs/checkpoint/tests/test_memory.py:324-700` |
| Store namespace reservation | Root label cannot be `"langgraph"`; no `.` allowed | `sources/langgraph/libs/checkpoint/langgraph/store/base/__init__.py:1255-1275` |

## Answers to Dimension Questions

### 1. What kinds of state exist?

Eight explicit categories, each with a dedicated owner:

1. **Graph state (per-key channels)** — owner: `langgraph/channels/`. Each field is a `BaseChannel` instance (`LastValue`, `BinaryOperatorAggregate`, `AnyValue`, `EphemeralValue`, `UntrackedValue`, `Topic`, `NamedBarrierValue`, `DeltaChannel`). Channels expose `update()`, `get()`, `checkpoint()`, `from_checkpoint()`, `consume()`, `finish()` (`channels/base.py:19-121`). Schema annotation drives which channel class is created (`graph/state.py:1801-1927`).
2. **Channel metadata / config (per superstep)** — owner: `langgraph/_internal/_constants.py`. Reserved write keys (`__interrupt__`, `__resume__`, `__error__`, `__pregel_tasks`, `__previous__`, etc.) and config keys (`__pregel_checkpointer`, `__pregel_store`, `__pregel_runtime`, `__pregel_scratchpad`) are interned constants.
3. **Execution state (per invocation)** — owner: `langgraph/runtime.py:124-238` (`Runtime`), with `ExecutionInfo` (`runtime.py:26-58`) and `RunControl` (`runtime.py:79-104`) as nested facets. `Runtime` bundles `context`, `store`, `stream_writer`, `heartbeat`, `previous`, `execution_info`, `server_info`, `control`.
4. **Per-task scratch state (per superstep)** — owner: `PregelScratchpad` (`_internal/_scratchpad.py:8-19`). Holds `step`, `stop`, `call_counter`, `interrupt_counter`, `subgraph_counter`, `get_null_resume`, `resume`.
5. **Durable state (per thread)** — owner: `BaseCheckpointSaver` (`checkpoint/base/__init__.py:176-722`) via `Checkpoint` (`:92-124`) and `CheckpointMetadata` (`:38-86`); intermediate writes stored as `PendingWrite` tuples (`checkpoint/base/__init__.py:31`).
6. **Long-term memory (cross-thread)** — owner: `BaseStore` (`store/base/__init__.py:700-1252`) via `Item` records (`:51-115`) with hierarchical `Namespace = tuple[str, ...]` and TTL semantics (`:545-567`).
7. **Node result cache (cross-thread, per node)** — owner: `BaseCache` (`cache/base/__init__.py:11-48`) keyed by `(Namespace, key)` with TTL.
8. **Approval / interrupt state (per task)** — encoded as `Interrupt` records (`types.py:533-588`) inside `pending_writes`. The interrupt `id` is `xxh3_128_hexdigest` of the checkpoint namespace, so it is stable across replays of the same node position (`types.py:572`).

### 2. Which state is source-of-truth?

- **Channel `channel_values`** are the source-of-truth for graph state during a live run (`channels/base.py:69-71`). The checkpointer's last `put()` is the durable source-of-truth for cross-process / cross-session state.
- **`checkpoint.channel_versions`** is the monotonic truth for "what has changed since checkpoint N" (`checkpoint/base/__init__.py:109-114`, applied in `pregel/_algo.py` to decide triggers).
- **`checkpoint.versions_seen`** is the source-of-truth for which `(node, channel-version)` pairs each task has already consumed (`checkpoint/base/__init__.py:115-120`).
- **`checkpoint.pending_writes`** are the source-of-truth for uncommitted delta writes that haven't been folded into `channel_values` yet; on resume, `channels_from_checkpoint` reconstructs via `saver.get_delta_channel_history` (`pregel/_checkpoint.py:136-226`, `checkpoint/base/__init__.py:582-690`).
- **`BaseStore`** is the source-of-truth for cross-thread memory; the graph never caches it. `BaseCache` is derived state, populated from `task.writes` after each step (`pregel/_loop.py:1586-1602`).

### 3. Which state is derived?

- `PregelScratchpad` counters are derived from the per-superstep call site (`:8-19`).
- Managed values (`IsLastStep`, `RemainingSteps`) are derived from `PregelScratchpad.step` and `stop` (`managed/is_last_step.py:9-24`).
- `Runtime.previous` is derived from the prior checkpoint's `__previous__` write on the same `thread_id` (`func/__init__.py:587-604`).
- `ReplayState._visited_ns` is derived from observed namespaces during the current parent replay (`_internal/_replay.py:14-50`).
- `InMemorySaver`'s `channel_values` view is reconstructed from `checkpoints.channel_versions` joined with `blobs` (`checkpoint/memory/__init__.py:125-140`, `:403-409`).
- `stream_keys` projected into `StateSnapshot.values` is derived from live channels (`pregel/main.py:1256-1265`, `read_channels` in `pregel/_io.py`).

### 4. Which state is ephemeral?

- `EphemeralValue` clears after the consuming step (`channels/ephemeral_value.py:55-68`).
- `UntrackedValue` is never checkpointed (`channels/untracked_value.py:48-49`) and `put_writes` strips it before persisting (`pregel/_loop.py:432-447`).
- `Runtime.context` and `Runtime.stream_writer` live only for the duration of a single `invoke()`/`stream()` call; their defaults are no-ops when the runtime isn't bound (`runtime.py:107-110`, `:198-217`).
- `RunControl` is per-run: "Create a fresh `RunControl` per run; reusing a control after `request_drain()` leaves it drained" (`runtime.py:81-87`).
- `PregelScratchpad` is per-task and not persisted (`_internal/_scratchpad.py:8-19`). However, its `resume` list is consumed by `interrupt()` (`:915-925`) and reflected into the checkpoint's `pending_writes` as `__resume__` so the value is durable; only the *counter* is ephemeral.
- `InMemoryCache` and `InMemoryStore` are process-memory only (`checkpoint/memory/__init__.py:33-99`, `cache/memory/__init__.py:11-73`).
- `DeltaChannel`'s snapshot cadence is bounded by `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000) so non-snapshot steps are reconstructed on demand rather than stored (`pregel/_checkpoint.py:37-58`).

### 5. Which state is safe to lose?

- **Safe to lose**: `Runtime` fields other than `Runtime.previous` (which is durable via `__previous__`); `RunControl`; `PregelScratchpad` counters (rebuilt from `step` in `checkpoint_metadata`); `stream_writer` events (already emitted); `UntrackedValue` writes (explicitly not stored); ephemeral input/output via `EphemeralValue` channels.
- **Must survive crash**: `Checkpoint.channel_values`/`channel_versions`/`versions_seen`/`pending_writes` (`checkpoint/base/__init__.py:92-124`); `Item` records in `BaseStore`; `CachePolicy` results (cache can be lost, but if so the next step recomputes); `__interrupt__` records (encoded in `pending_writes`); `__resume__` value (encoded in `pending_writes` via `WRITES_IDX_MAP`).
- **Safe to lose but expensive**: `BaseCache` — losing it forces re-execution of cached nodes; tolerated because `BaseCache` is opt-in (`cache/base/__init__.py:15`).

**Can the repo explain which state must survive a crash?** Yes. `BaseCheckpointSaver`'s docstring states "thread_id is the primary key used to store and retrieve checkpoints" (`checkpoint/base/__init__.py:190-192`), and `CHECKPOINT_COORDINATE_KEYS` (`_internal/_constants.py:99-104`) plus `RunnableConfig["configurable"]["thread_id"|"checkpoint_ns"|"checkpoint_id"|"checkpoint_map"]` define the durable coordinates. `Durability = Literal["sync", "async", "exit"]` (`types.py:87-93`) explicitly enumerates when writes become durable.

## Architectural Decisions

- **Channel abstraction decouples state shape from persistence.** `BaseChannel` exposes `checkpoint()` / `from_checkpoint()` as the persistence contract (`channels/base.py:49-65`); savers do not know how to serialise a `LastValue` vs. a `BinaryOperatorAggregate`. Adding a new channel class does not require touching any saver.
- **Three-table persistence (Postgres) separates metadata, blobs, and writes** (`checkpoint-postgres/.../base.py:43-91`). `checkpoints` stores the head pointer and metadata; `checkpoint_blobs` stores versioned channel values; `checkpoint_writes` stores per-task intermediate writes. SQLite inlines `channel_values` into the `checkpoints` blob because there is no JSONB and the row count is small (`checkpoint-sqlite/.../__init__.py:139-163`).
- **Versioning is per-channel, monotonically increasing.** Default is integer (`checkpoint/base/__init__.py:692-711`); Postgres uses a composite `"{seq:032}.{rand:016}"` to support concurrent writers (`checkpoint-postgres/.../base.py:543-552`).
- **`DeltaChannel` is an explicit storage-tier optimisation.** Snapshots every N updates or every 5000 supersteps; in between, the channel stores a sentinel and reconstruction walks ancestors via `get_delta_channel_history` (`pregel/_checkpoint.py:37-58`, `checkpoint/base/__init__.py:582-690`).
- **`ManagedValue` keeps schema-driven derived state without leaking it into checkpoints.** `IsLastStep`/`RemainingSteps` (`managed/is_last_step.py:9-24`) read from `PregelScratchpad`, which is rebuilt from `checkpoint.metadata.step` on resume.
- **`Durability = "sync" | "async" | "exit"`** is a knob, not an implementation detail (`types.py:87-93`). Sync waits inside `tick`; async submits to `BackgroundExecutor`; exit defers to `_put_exit_delta_writes` at `__exit__` (`pregel/_loop.py:1006-1301`).
- **`BaseStore` is hierarchically namespaced and TTL-aware.** `Namespace = tuple[str, ...]` cannot start with `"langgraph"` (`store/base/__init__.py:1272-1275`) and `Item` carries `created_at`/`updated_at` for the optional TTL sweep (`store/postgres/aio.py:311-385`).
- **`BaseCache` is opt-in and keyed by node-input hash.** Default `key_func` pickles a frozen representation of `(args, kwargs)` (`_internal/_cache.py:7-31`). Hits are applied in `match_cached_writes()` before downstream tasks schedule (`pregel/_loop.py:1526-1547`).
- **Conformance is enforced by a separate suite.** `checkpoint-conformance` defines `Capability` enum and runs base + extended tests against any registered `BaseCheckpointSaver` (`checkpoint-conformance/.../capabilities.py:15-50`).
- **`RunControl` is the only run-level cooperative drain.** A single `_drain_reason` write; no lock needed (`runtime.py:81-87`).

## Notable Patterns

- **Pre-pregel Pregel model**: state transitions are formulated as a graph of `BaseChannel`s with `update()`/`consume()`/`finish()` semantics, not as a generic KV store. This is how LangGraph keeps reducers explicit (`channels/binop.py:51-141`) and avoids a generic "set" primitive that would lose ordering guarantees.
- **Reserved key sentinel pattern**: every cross-cutting concern (interrupts, errors, push, send, scratchpad) gets a unique `__xxx__` reserved key (`_internal/_constants.py:7-130`). This keeps the channel/serializer surface area small.
- **Dual schema for storage**: in-memory uses `defaultdict` of `defaultdict` keyed by `(thread, ns, cid)` (`checkpoint/memory/__init__.py:69-83`); Postgres uses three normalised tables; SQLite uses two with inlined values — all behind the same `BaseCheckpointSaver` interface.
- **Two-phase durability for delta writes**: `_delta_write_futs` list ensures a checkpoint never becomes durable before the writes that produced it (`pregel/_loop.py:199-205`, `:1507-1524`).
- **Capability detection over inheritance**: `_is_overridden` detects whether a subclass implements `aput_writes`/`adelete_for_runs`/etc., so the conformance suite can decide which tests to run (`checkpoint-conformance/.../capabilities.py:90-96`).
- **`xxh3_128_hexdigest` for stable interrupt IDs**: derived from the checkpoint namespace, so resume maps work across time-travel replays (`types.py:572-578`).
- **Strict msgpack allowlist**: `LANGGRAPH_STRICT_MSGPACK=true` or `allowed_msgpack_modules` enforces a per-graph module allowlist, mitigating deserialization RCE (`serde/jsonplus.py:88-95`, `_serde.py` wiring in `graph/state.py:1220-1241`).

## Tradeoffs

- **Channel-class explosion vs. generic KV**: there are 9+ channel classes (`channels/__init__.py:14-28`); picking the right one is the developer's job, but each is explicit about its semantics.
- **Checkpoint duplication for snapshot vs. write tables**: `pending_writes` are stored both in `pending_writes` (for replay) and eventually in `channel_values` (for snapshots). This is the cost of `DeltaChannel` reconstruction (`channels/delta.py:196-203`).
- **Strict ownership vs. propagation**: store/cache are passed by reference into subgraphs (`pregel/main.py:2609-2616`), but checkpointer inheritance has three modes (`None` inherit, `True` enable, `False` disable) (`types.py:98-104`, applied in `graph/state.py:1187-1189`).
- **`scratchpad` is per-task, not per-thread**: this means crash mid-task loses unconsumed counter offsets, mitigated by persisting `__resume__` values immediately but not the counter itself (`_internal/_scratchpad.py:8-19`).
- **`BaseCache` is best-effort, not transactional**: a node may run twice if the cache key matches but the result wasn't durable. This is acceptable for node-result caching but must be opt-in (`cache/base/__init__.py:15`).
- **`DeltaChannel` is "Beta"**: API and on-disk shape may change (`channels/delta.py:29-37`).

## Failure Modes / Edge Cases

- **Crash before next superstep**: handled by `_checkpointer_put_after_previous` waiting on `_delta_write_futs` (`pregel/_loop.py:1507-1524`).
- **`DeltaChannel` ancestor walk depth**: bounded by `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (`pregel/_checkpoint.py:53-58`); without a snapshot in the chain, `get_delta_channel_history` returns `seed: MISSING` and reconstruction starts empty.
- **Pruning severing delta chain**: `BaseCheckpointSaver.prune` warns that naive `"keep_latest"` can sever delta reconstruction, and documents three safe-recovery strategies (`checkpoint/base/__init__.py:374-414`).
- **Concurrent writes to a non-reducer channel**: `LastValue` raises `InvalidUpdateError` if multiple writes arrive in one step (`channels/last_value.py:59-64`); `BinaryOperatorAggregate` enforces single-reducer semantics via the schema check in `_is_field_binop` (`graph/state.py:1890-1908`).
- **Lost `__resume__` value**: `interrupt()` raises `GraphInterrupt` only if `scratchpad.get_null_resume(True) is None and scratchpad.resume[idx]` was not pre-populated (`types.py:915-934`); if the resume value was not persisted (e.g., `durability="async"` and crash before flush), the graph will re-interrupt on resume.
- **Cross-thread namespace collisions in `BaseStore`**: namespaces cannot start with `langgraph` and cannot contain `.` (`store/base/__init__.py:1255-1275`), but otherwise have no global uniqueness check.
- **Cache TTL eviction race**: TTL is checked on read in `InMemoryCache.get` (`cache/memory/__init__.py:27-31`); concurrent read+set has no atomicity guarantee.
- **Sync between channel_versions and snapshot decision**: if a delta channel's count hits `snapshot_frequency` over several supersteps without an intervening write, `create_checkpoint` manually bumps `channel_versions[k]` to keep the snapshot durable (`pregel/_checkpoint.py:104-107`).
- **Resume map missing for nested subgraph interrupt**: `CONFIG_KEY_RESUME_MAP` (`_internal/_constants.py:71`) is populated by the parent before invoking the child; missing keys cause re-interrupt.

## Future Considerations

- **`DeltaChannel` stabilisation**: docstring explicitly says the API and on-disk representation may change (`channels/delta.py:29-37`). Pluggable snapshot policies are likely.
- **Transactional outbox for cache + checkpoint**: today, `BaseCache` is best-effort; an option to write `cache_key → writes` atomically with `pending_writes` would close the duplication window.
- **Multi-tenant namespace isolation in `BaseStore`**: `Runtime` already has `server_info` for LangSmith server context (`runtime.py:60-77`), but `BaseStore` doesn't enforce isolation; the namespace reservation is the only protection.
- **`ReplayState` hardening**: the `_visited_ns` set is per-process and in-memory (`_internal/_replay.py:30-50`); a multi-process scheduler would need to share it.
- **Conformance as a release gate**: `checkpoint-conformance` exists but the test runs only for `InMemorySaver` in the repo (`checkpoint-conformance/tests/test_validate_memory.py:17-21`); Postgres/SQLite conformance is presumably external. Promoting it to a CI gate would harden the contract.
- **`BaseStore` TTL semantics**: only Postgres/SQLite stores implement TTL sweeps; `InMemoryStore` exposes `supports_ttl = False` (`store/base/__init__.py:719`).
- **Schema evolution for `Checkpoint`**: `LATEST_VERSION = 4` in `pregel/_checkpoint.py:21` (older `empty_checkpoint` had `v=2`, see `checkpoint/base/__init__.py:811`); `_migrate_checkpoint` (`pregel/main.py:1135-1142`) is the migration hook for `pending_sends`. Further bumps will need more migrations.

## Questions / Gaps

- **What happens if two processes checkpoint concurrently on the same `thread_id`?** Postgres uses `INSERT ... ON CONFLICT ... DO UPDATE` (`checkpoint-postgres/.../base.py:137-144`) and a composite `version` (`{seq:032}.{rand:016}`) for tie-break. SQLite and in-memory use `dict.update` semantics. No global lock. Conflict resolution for "best" parent is left to the caller (latest wins).
- **Is `Runtime.context` allowed to be mutable?** Yes, but `frozen=True, slots=True` on the dataclass (`runtime.py:124-198`) means the reference is immutable; mutations to referenced objects are not prevented.
- **What is the relationship between `checkpoint_pending_writes` and `pending_writes` persisted?** `put_writes` persists the value but `WRITES_IDX_MAP` uses negative indices for reserved keys (`ERROR=-1, SCHEDULED=-2, INTERRUPT=-3, RESUME=-4`, `checkpoint/base/__init__.py:795`) so regular writes (positive indices) never collide with reserved ones.
- **How does the runtime handle subgraph checkpoint_ns collapse vs. preserve?** `_internal/_replay.py:14-50` strips the task-id suffix for "first-visit" tracking, so the same logical subgraph namespace is recognised across iterations.
- **Where is `ServerInfo` populated?** Only by LangGraph Server deployments (`runtime.py:62-77`); open-source `langgraph` always sees `None`.
- **Can `BaseCache` key collisions happen across users?** The cache is per-`Pregel` instance; no cross-instance key collision unless instances share a backing store. The `RedisCache` (referenced in tests at `checkpoint/tests/test_redis_cache.py`) shares across instances and trusts the cache key to disambiguate.

---

Generated by `02.01-state-taxonomy-and-ownership` against `langgraph`.