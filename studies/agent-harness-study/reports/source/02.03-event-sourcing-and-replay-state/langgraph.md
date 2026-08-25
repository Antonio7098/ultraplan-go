# Source Analysis: langgraph

## 02.03 — Event Sourcing and Replay State

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/checkpoint`, `libs/langgraph`, `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/checkpoint-conformance`) |
| Analyzed | 2026-07-06 |

## Summary

LangGraph does **not** use a classical append-only event log as its source of truth. State is rebuilt from a *hybrid* model:

1. A snapshot row per superstep (`checkpoints` table) carrying a serialized `Channel` state in `checkpoint.channel_values`.
2. A separate, append-only write log (`checkpoint_writes` table) that captures the writes a task produced at each step but did not yet fold into the channel.
3. A `checkpoint_blobs` table that holds serialized channel blobs for non-delta channels (Postgres backend; SQLite stores the same payload inline in `checkpoints.checkpoint`).

There is also a **newer event-style layer** on top of this: `DeltaChannel` (`libs/langgraph/langgraph/channels/delta.py`) records writes as an *immutable, append-only write history* that is replayed through a deterministic reducer to reconstruct the channel value. Periodically a `_DeltaSnapshot` blob is materialized so the replay window stays bounded. The reconstruction protocol (`get_delta_channel_history` → `replay_writes`) walks the parent chain by checkpoint id to gather `(seed, writes[])` per channel and is the closest the codebase comes to "event sourcing + replay."

So LangGraph is fundamentally a **snapshot-per-step system with a co-resident per-step write log**, plus an *optional, opt-in* delta/replay channel whose canonical representation is the write log (with periodic snapshots for replay cost control). It is not a pure event-sourced log where all state is derived from events; the checkpoint snapshot itself is the source of truth for non-delta channels, and the writes table is replay-instrumentation rather than authoritative history.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Justification:

- Strong explicit interfaces: `BaseCheckpointSaver.get` / `get_tuple` / `list` / `put` / `put_writes` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:227-318`), plus `get_delta_channel_history` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`).
- Append-only write path is durable by contract (`UPSERT_CHECKPOINT_WRITES_SQL` with `DO UPDATE SET channel = EXCLUDED.channel, …` at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-153`).
- Snapshot/replay cost is bounded: `_DeltaSnapshot` blobs plus per-channel `counters_since_delta_snapshot` (`libs/checkpoint/langgraph/checkpoint/pregel/_checkpoint.py:37-58`, `libs/langgraph/langgraph/pregel/_loop.py:1079-1142`).
- Versioning: `Checkpoint["v"]` integer in the `Checkpoint` TypedDict (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:95`), explicit migration (`libs/langgraph/langgraph/pregel/main.py:1135-1142`), Postgres/SQLite schema migrations tracked in `MIGRATIONS` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:139-164`).
- Conformance test suite (`libs/checkpoint-conformance/...`) and dedicated replay tests (`libs/langgraph/tests/test_time_travel.py`, `test_delta_channel_migration.py`) give operational coverage.
- Held back from 8+ by: there is **no public append-only "event log" abstraction** (events are conflated with channel writes; the schema mixes snapshot and write data into the same saver); durability ordering is mostly correct but the design notes explicitly call out that a `keep_latest` prune can silently corrupt `DeltaChannel` state if the parent chain is not preserved (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`); snapshot cadence is a developer-tunable constant with hardcoded defaults rather than observable/controllable per workload; and replay correctness depends on the user-supplied reducer being deterministic and batching-invariant (`libs/langgraph/langgraph/channels/delta.py:42-49`) — a contract LangGraph enforces only via documentation and a single error class (`InvalidUpdateError`), not with a runtime check.

## Evidence Collected

Every entry includes a file path with line numbers. Format `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Snapshot + write log schema (Postgres) | `checkpoints`, `checkpoint_blobs`, `checkpoint_writes` tables | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91` |
| Append-only write contract | `INSERT … ON CONFLICT DO NOTHING` / `DO UPDATE SET …` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:131-159` |
| Append-only write contract (sqlite) | `writes` table, `INSERT` only, no `UPDATE`/`DELETE` outside `setup`/`delete_thread` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:139-164`, `:262-292` |
| `Checkpoint` TypedDict schema (versioned) | `v: int`, `id`, `ts`, `channel_values`, `channel_versions`, `versions_seen`, `updated_channels` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-123` |
| `CheckpointMetadata` schema (event-style source/step/parents) | `source` enum `input|loop|update|fork`, `step`, `parents`, `run_id`, `counters_since_delta_snapshot` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| Checkpoint ID monotonicity | UUIDv6 with monotonic counter via `_last_v6_timestamp` | `libs/checkpoint/langgraph/checkpoint/base/id.py:79-109` |
| Sequence numbering (per-channel version) | `ChannelVersions` dict on the checkpoint, bumped in `apply_writes` | `libs/langgraph/langgraph/pregel/_algo.py:227-345` |
| `get_next_version` versioning strategy | Default `increment`; `InMemorySaver.get_next_version` uses `<int>.<rand>` for time-ordered unique IDs | `libs/langgraph/langgraph/pregel/_algo.py:227-229`; `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628` |
| Per-step write append (the "event") | `checkpointer.put_writes` from `_put_pending_writes` and from `put_writes` | `libs/langgraph/langgraph/pregel/_loop.py:408-501`; `libs/langgraph/langgraph/pregel/_loop.py:503-541` |
| Snapshot durability model | `_put_checkpoint` and `_checkpointer_put_after_previous` await delta-write futures before commit | `libs/langgraph/langgraph/pregel/_loop.py:1064-1189`, `libs/langgraph/langgraph/pregel/_loop.py:1759-1778` |
| Rebuilder: channel hydration | `channels_from_checkpoint` walks delta history then `from_checkpoint` / `replay_writes` | `libs/langgraph/langgraph/pregel/_checkpoint.py:136-184` |
| Rebuilder: ancestor walk for delta channels | `BaseCheckpointSaver.get_delta_channel_history` parent-chain loop | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690` |
| Rebuilder: optimized InMemory override | Single-pass chain walk + per-channel terminator | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:142-229` |
| Rebuilder: optimized Postgres (two-stage SQL) | Paged stage-1 JSONB key lookup + per-channel UNION ALL stage-2 | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:444-550`; SQL builders at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-288` |
| Rebuilder: optimized SQLite (streamed stage-1) | Row-by-row ancestor walk with single-deserialize-per-cid | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py:34-172` |
| Delta snapshot blob (event log terminus marker) | `_DeltaSnapshot` NamedTuple; msgpack ext type `EXT_DELTA_SNAPSHOT = 7` | `libs/checkpoint/langgraph/checkpoint/serde/types.py:19-31`; `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-307`, `:634-639` |
| Snapshot cadence decision | `delta_channels_to_snapshot` predicate over `(updates, supersteps)` counters | `libs/langgraph/langgraph/pregel/_checkpoint.py:37-58` |
| Snapshot bounded by supersteps | `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` env var (default 5000) | `libs/langgraph/langgraph/_internal/_config.py:33-35` |
| Counter bookkeeping | `_put_checkpoint` bumps `(updates, supersteps)` per delta channel; zeros on snapshot | `libs/langgraph/langgraph/pregel/_loop.py:1079-1142` |
| Immutability of writes | `put_writes` is keyed by `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)`; collision only updates metadata fields, never deletes; InMemory path skips duplicates by `idx >= 0` collision | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:498-509` |
| No in-place mutation of stored checkpoints | `copy_checkpoint` produces a fresh dict before save | `libs/langgraph/langgraph/pregel/_checkpoint.py:229-238`; `libs/checkpoint/langgraph/checkpoint/base/__init__.py:126-136` |
| Time-travel replay | `invoke(None, before_b.config)` re-runs downstream tasks; replay state used to restore subgraph checkpoints before replay point | `libs/langgraph/langgraph/pregel/_loop.py:1606-1686`; `libs/langgraph/langgraph/_internal/_replay.py:14-90` |
| Subgraph replay | `ReplayState.get_checkpoint` falls back to `list(before=…)` on first subgraph visit | `libs/langgraph/langgraph/_internal/_replay.py:52-89` |
| Schema versioning | `Checkpoint["v"]` typed `int`; `_migrate_checkpoint` upgrades pre-v4 (pending_sends relocation) | `libs/langgraph/langgraph/pregel/main.py:1135-1142` |
| Postgres schema migrations | Versioned `MIGRATIONS` list, runs from `current + 1` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`; runner at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85-110` |
| Postgres v3→v4 migration | `_migrate_pending_sends` lifts `TASKS` channel writes from `checkpoint_writes` into `channel_values[TASKS]` for v<4 | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:308-320`; read path at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:246-261` |
| Replay correctness test | `test_time_travel.py` covers replay, fork, subgraph replay | `libs/langgraph/tests/test_time_travel.py:1-3966` |
| Delta migration round-trip | `test_delta_channel_migration.py` — pre-delta state round-trips under new annotation | `libs/langgraph/tests/test_delta_channel_migration.py:1-618` |
| Conformance: PUT/GET/LIST | Standard protocol tests | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put.py:1-411`; `test_get_tuple.py`; `test_list.py` |
| Conformance: PUT_WRITES (events) | Append contract for `put_writes` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py:1-302` |
| Conformance: DELTA_CHANNEL_HISTORY (replay) | Walk order, seed-from-snapshot, target-write exclusion, multi-channel | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delta_channel_history.py:1-247` |
| Supersteps bound forces snapshot | Test guards unbounded ancestor walk | `libs/langgraph/tests/test_delta_channel_supersteps_bound.py:1-195` |
| Exit-mode accumulator | Captures delta writes in memory, persists under anchor checkpoint with step-prefixed synthetic task ids | `libs/langgraph/langgraph/pregel/_loop.py:1201-1292` |
| Idempotency / lock | Channel write-key map `WRITES_IDX_MAP` keeps `ERROR/SCHEDULED/INTERRUPT/RESUME` writes in fixed negative slots to avoid collisions | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| `read committed` re-entrancy guard | Async saver path protects against concurrent `aget_delta_channel_history` | `libs/checkpoint/tests/test_memory.py:518-560` |
| Migration: BinaryOperatorAggregate → DeltaChannel | Public path for legacy state under new annotation | `libs/langgraph/tests/test_delta_channel_migration.py:1-200` |

## Answers to Dimension Questions

1. **Are events authoritative?**

   Partially. There are two layers:

   - For the **non-delta** channels (`LastValue`, `Topic`, `BinaryOperatorAggregate`, `UntrackedValue`): the checkpoint snapshot itself is authoritative. The `checkpoint_writes` table only holds the writes that produced the *next* superstep; once a new checkpoint row is written, those writes are no longer needed for state reconstruction (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:139-146`).
   - For the **DeltaChannel** opt-in layer (`libs/langgraph/langgraph/channels/delta.py`): the *write log* is canonical between snapshots, and a `_DeltaSnapshot` blob is materialized periodically to bound replay cost. Reads that hit a step without a snapshot reconstruct via `get_delta_channel_history` + `replay_writes`.

   The `Checkpoint["v"]` field and `_migrate_checkpoint` only address schema-level migration (`libs/langgraph/langgraph/pregel/main.py:1135-1142`), not event-log versioning in the DDD sense.

2. **Can state be rebuilt from events?**

   Yes for `DeltaChannel` (write history + nearest-ancestor seed); no for ordinary channels (they need the checkpoint snapshot row).

   The rebuilder is `channels_from_checkpoint` (`libs/langgraph/langgraph/pregel/_checkpoint.py:136-184`). It batches every channel needing replay into a single `saver.get_delta_channel_history` call and feeds the returned `DeltaChannelHistory` into `from_checkpoint(seed)` + `replay_writes(writes)` (`libs/langgraph/langgraph/channels/delta.py:118-157`). For non-delta channels, `spec.from_checkpoint(checkpoint["channel_values"][k])` is sufficient.

3. **Are events immutable?**

   In practice, yes for the public contract:

   - The Postgres write path uses `INSERT … ON CONFLICT (thread_id, checkpoint_ns, checkpoint_id, task_id, idx) DO UPDATE SET channel/type/blob = EXCLUDED.*` — collisions overwrite the channel and payload but never delete the row (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-159`). The same applies to the SQLite path (no UPDATE on `writes`).
   - The InMemory path explicitly skips writes whose `(task_id, idx)` already exists in `outer_writes_` (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:498-509`).
   - `delete_thread` is the only path that removes events; it is gated behind an explicit API call (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`; `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:295-385`).

   Immutability is therefore "no in-place mutation of the same row," not "no overwrite of stale data when retries collide." A retry that re-issues `put_writes` with the same `task_id/idx` overwrites the prior payload — which is the desired idempotency behavior, but it is not the strict log-style immutability some event-sourced systems enforce.

4. **Can old events still replay after schema changes?**

   Yes, with caveats:

   - **Annotation change** (e.g., `BinaryOperatorAggregate` → `DeltaChannel` on the same checkpointer): the pre-migration snapshot blob is treated as a plain-value seed by `DeltaChannel.from_checkpoint` and the walk terminates at the pre-delta ancestor (`libs/checkpoint/langgraph/checkpoint/serde/types.py:118-127`; `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`). The public contract for this is asserted by `test_delta_channel_migration.py` (`libs/langgraph/tests/test_delta_channel_migration.py:1-618`).
   - **Pre-v4 schema** (legacy `pending_sends` field): migrated lazily by `_migrate_checkpoint` (`libs/langgraph/langgraph/pregel/main.py:1135-1142`) and eagerly by Postgres's `_migrate_pending_sends` on read (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:308-320`; `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:246-261`).
   - **Database schema migrations**: tracked and replayed by `setup()` in Postgres (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`; runner at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85-110`).

   What is **not** in scope: arbitrary in-payload schema changes are not formally versioned. The `_DeltaSnapshot` blob is unversioned inside the msgpack envelope, and the `Checkpoint["v"]` field only carries checkpoint-format versioning (current = 4; `LATEST_VERSION = 4` at `libs/langgraph/langgraph/pregel/_checkpoint.py:21`).

5. **How is replay cost managed?**

   Three controls:

   - **Per-channel snapshot frequency**: `DeltaChannel(snapshot_frequency=N)` — every Nth update, `_DeltaSnapshot(ch.get())` is written (`libs/langgraph/langgraph/channels/delta.py:60-64`, `libs/langgraph/langgraph/pregel/_checkpoint.py:92-108`).
   - **System-wide supersteps bound**: `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default 5000, env-overridable at `libs/langgraph/langgraph/_internal/_config.py:33-35`) forces a snapshot even on channels that stop receiving writes. The accompanying test (`libs/langgraph/tests/test_delta_channel_supersteps_bound.py:1-195`) pins the behavior.
   - **Two-stage SQL reconstruction** in Postgres (`_build_delta_stage1_sql` paged JSONB lookup → per-channel UNION ALL writes fetch at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-288`, run at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:444-550`) and streamed stage-1 in SQLite (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py:34-124`) keep the ancestor-walk I/O bounded even when the chain is long.

   Counter semantics live in the `counters_since_delta_snapshot` metadata field (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-86`), maintained by `_put_checkpoint` (`libs/langgraph/langgraph/pregel/_loop.py:1079-1142`). The state snapshot itself is `MISSING`-sentineled in `DeltaChannel.checkpoint()` for non-snapshot steps (`libs/langgraph/langgraph/channels/delta.py:195-204`), so the JSON blob stays small.

   **Important weakness**: `BaseCheckpointSaver.prune` is documented as `DeltaChannel`-unsafe — a `keep_latest` strategy that drops intermediate checkpoints and their writes can sever the chain between a kept checkpoint and the nearest `_DeltaSnapshot` ancestor, silently reconstructing delta channels as empty (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`). `copy_thread` carries the same warning (`:350-372`). The SQLite/Postgres implementations inherit the default `NotImplementedError` for `prune`/`copy_thread`/`delete_for_runs` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:415, 372, 348`), so this is mostly a contract issue for third-party savers, but the documentation alone is fragile.

## Architectural Decisions

- **Snapshot + write log together in one saver.** Snapshot rows and per-task writes share `thread_id`, `checkpoint_ns`, `checkpoint_id` keys but live in separate tables. Snapshot is the source of truth for non-delta state; writes are the per-step journal needed to reconstruct delta channels and to honor replay of in-flight tasks. (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`; `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:139-164`.)
- **Channel versions drive change detection.** `apply_writes` maintains `Checkpoint["channel_versions"]` and bumps via `checkpointer.get_next_version` (`libs/langgraph/langgraph/pregel/_algo.py:232-345`). The default `increment` (`libs/langgraph/langgraph/pregel/_algo.py:227-229`) produces monotonic ints; `InMemorySaver` uses a `<int>.<random>` format for globally unique IDs (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628`).
- **Two-tier reconstruction contract.** `BaseCheckpointSaver.get_delta_channel_history` defines the public replay shape (`DeltaChannelHistory = {writes, seed}`), with `seed` *omitted* (TypedDict absence) when the walk reached root without finding a snapshot — consumers treat that as "start empty" (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:149-189, 582-690`). This is the "events are authoritative for delta channels" interface.
- **Per-step writes flushed in background, joined to checkpoint at exit.** `put_writes` is submitted to a background executor (`libs/langgraph/langgraph/pregel/_loop.py:408-501`), and `_checkpointer_put_after_previous` drains `_delta_write_futs` before committing the checkpoint that references them (`libs/langgraph/langgraph/pregel/_loop.py:1759-1778`). Same pattern for exit-mode delta writes via `_put_exit_delta_writes` (`libs/langgraph/langgraph/pregel/_loop.py:1201-1292`).
- **Subgraph replay is gated by `ReplayState`.** Shared by reference, first-visit loads the latest-before-replay checkpoint, later visits load the live latest (`libs/langgraph/langgraph/_internal/_replay.py:14-90`). Hooked in `PregelLoop.__enter__` (`libs/langgraph/langgraph/pregel/_loop.py:1606-1686`).
- **Serializer and schema decoupled.** `_DeltaSnapshot` is encoded as a dedicated msgpack ext type (`EXT_DELTA_SNAPSHOT = 7`, `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-307`, decoded at `:634-639`). This isolates the delta-channel representation from the rest of the snapshot payload.

## Notable Patterns

- **`DeltaChannel` is an opt-in event-sourced reducer.** Reducer signature `reducer(state, list[writes]) -> new_state` is documented as deterministic + batching-invariant (`libs/langgraph/langgraph/channels/delta.py:42-49`). This is the contract that makes "rebuild by replay" correct — and it is the only place in the codebase where such a contract is enforced (and even there, only by documentation, not by a runtime check).
- **Two-stage SQL reconstruction for delta history.** Postgres and SQLite both implement a "stage 1 = walk parents, stage 2 = per-channel UNION ALL fetch" design that avoids over-fetching the writes table when channels have different chain depths (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-288`; `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py:42-65`).
- **Append-only writes with conflict-tolerant UPSERT.** `INSERT … ON CONFLICT DO UPDATE SET channel = EXCLUDED.channel, type = EXCLUDED.type, blob = EXCLUDED.blob` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-153`). Lets at-least-once retries safely overwrite stale data without losing the row identity.
- **Counter-based snapshot policy.** `counters_since_delta_snapshot` per-channel `(updates, supersteps)` is a deliberate two-dimensional policy: per-update cap + system-wide staleness cap (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-86`; `libs/langgraph/langgraph/pregel/_loop.py:1079-1142`).
- **Capability-gated conformance suite.** `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-48` defines base vs. extended capabilities, with `DELTA_CHANNEL_HISTORY` as an explicit one. Any third-party saver is expected to override `aget_delta_channel_history` to opt in (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:53-87`).
- **Snapshot/writes fan-out under a single connection lock (Postgres).** The cursor is wrapped in `self.lock` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:413-442`), and pipeline mode relaxes per-connection serialization. This makes ordering observable: checkpoint and writes hit the same WAL in causal order.

## Tradeoffs

- **Snapshot + log → smaller writes than pure event log, but no canonical "events" object.** Each checkpoint row carries a full channel snapshot (excluding `DeltaChannel` non-snapshot steps). Replay cost on resume is bounded by one snapshot plus a short write tail, instead of replaying from time zero. The cost is that the system does not present a single, uniform event-log interface — callers wanting to observe "every event" have to subscribe to a streaming callback (`libs/langgraph/langgraph/pregel/main.py:1144-1266`), not a log query.
- **Writes are coupled to the checkpoint lifetime, not a stream.** `checkpoint_writes` is cleared "logically" once the next checkpoint references the writes (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:155-159` uses `DO NOTHING` for writes on a non-existent checkpoint, but the row is purged only by `delete_thread`). This means writes are a journal, not an event store — a reader trying to reconstruct a thread after `prune` may find the chain severed (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`).
- **Replay fidelity depends on reducer discipline.** `DeltaChannel` requires `(state, [writes]) -> new_state` to be deterministic and associative-across-folds (`libs/langgraph/langgraph/channels/delta.py:42-49`). The library provides no schema-level versioning for serialized reducer state; migration is on the reducer author.
- **Serialization is msgpack with a security-strict allowlist path.** The default `JsonPlusSerializer` runs in "permissive" mode (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:107-114`) and logs unregistered types. Setting `LANGGRAPH_STRICT_MSGPACK=true` locks deserialization to `SAFE_MSGPACK_TYPES`. The risk is that an attacker who can write to the checkpoint table can trigger code execution on deserialization unless the strict gate is on (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:82-95`). This is unrelated to event immutability but is the relevant operational hazard for an event-sourced store.
- **No time-travel or "replay from event N" public surface.** The replay entry points are `invoke(checkpoint_id=...)` and `get_state_history` (`libs/langgraph/langgraph/pregel/_loop.py:1606-1686`; `libs/langgraph/langgraph/pregel/main.py:1479-1530`). There is no public "rebuild the entire thread from raw event bytes" because the snapshot rows are required.
- **`durability` mode trades snapshot latency for replay robustness.** `"async"` (default) writes checkpoints between supersteps; `"exit"` defers to exit and uses `_put_exit_delta_writes` + a lazy stub for first-run anchoring (`libs/langgraph/langgraph/pregel/_loop.py:1201-1292`). `"sync"` forces every `_put_checkpoint` future to be awaited before the next step (`libs/langgraph/langgraph/pregel/main.py:3002-3003`). Exit mode is the most failure-prone for delta channels because writes sit in memory until exit — a crash mid-run drops them.

## Failure Modes / Edge Cases

- **`keep_latest` prune corrupts delta threads.** Documented in `BaseCheckpointSaver.prune` docstring (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`). The Postgres and SQLite concrete savers do not implement `prune` by default, so this is a contract hazard for third-party savers.
- **`copy_thread` partial chain.** Must copy the entire parent chain, not just the head (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:350-372`). A naive copy would orphan delta writes from the head back to the nearest `_DeltaSnapshot`.
- **`delete_for_runs` on a live thread.** Same class of hazard (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-348`).
- **`DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` too low.** Forcing snapshots too aggressively reverts to non-delta-channel size. Too high risks long ancestor walks on read. The default 5000 is set in code; there is no per-workload auto-tuning.
- **Reducer non-determinism.** Only catches the *dual-overwrite* case in the live update path (`libs/langgraph/langgraph/channels/delta.py:163-171`, raising `InvalidUpdateError`). A non-deterministic reducer silently produces different state across replays; only the `test_*` corpus exercises the equivalence property.
- **Backpressure race: delta writes vs. exit checkpoint.** `_delta_write_futs` is drained before `aput` (`libs/langgraph/langgraph/pregel/_loop.py:1767-1778`). If the background executor fails, the future's exception is observed only when awaited — in the sync path, `_put_pending_writes` and `_put_checkpoint` are submitted but their futures are dropped (`libs/langgraph/langgraph/pregel/_loop.py:459-498, 503-541`). In the async path, futures are awaited at the await site; errors propagate up to the GraphInterrupt / unhandled-task surface.
- **Pre-v4 checkpoints.** `_migrate_pending_sends` does an extra round-trip to read pending sends for v<4 rows (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:246-261`). Slow on large lists, but bounded.
- **Async re-entrancy.** The base `aget_delta_channel_history` walks `aget_tuple` repeatedly; a regression once existed where a thread-local guard short-circuited concurrent calls. Guarded by `test_async_fallback_concurrent_tasks_do_not_interfere` (`libs/checkpoint/tests/test_memory.py:518-560`).
- **Empty channel writes with `durability != "exit"`.** `put_writes` short-circuits when `writes` is empty (`libs/langgraph/langgraph/pregel/_loop.py:410-411`). Callers relying on writes for durability should be aware of this.

## Future Considerations

- **Schema versioning of payload contents.** `Checkpoint["v"]` is for the container format only. A versioned schema for *channel values* (especially `DeltaChannel`-reduced state) would let old threads survive a reducer contract change. No such versioning is present today.
- **First-class "events" query API.** A unified `events(thread_id, from_version, to_version)` over the writes table would let observers reconstruct a thread without going through `get_state`. Currently the writes table is read only by the saver and the rebuilder.
- **Configurable snapshot strategy.** `snapshot_frequency` is per-channel int; `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` is env var. A runtime-tunable, per-thread strategy (e.g., cost-based) would be more flexible.
- **Per-workload reducer validation.** A static check that the reducer is associative-across-folds (e.g., property test) would catch drift before it becomes a silent replay divergence.
- **`prune` / `copy_thread` reference implementations.** The Postgres and SQLite backends currently rely on the base class's `NotImplementedError`. Implementing them with the documented safeguards would close the documented failure mode for first-party users.
- **Observable replay cost.** A `with_delta_channel_metrics` hook emitting per-channel ancestor-walk length, seed-vs-walk count, snapshot-blob size would let operators tune `snapshot_frequency` based on real workload.

## Questions / Gaps

- **No evidence of a pure event-log abstraction.** LangGraph's "events" are channel writes co-resident with snapshot rows. There is no `event_log`, `aggregate_root`, or `projection` vocabulary in the public API. Treat this dimension accordingly: the answer is "yes for `DeltaChannel`, no for the rest."
- **`get_delta_channel_history` capability advertisement.** Detection is by method override (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:75-87`). A savers that doesn't override but works correctly (e.g., inherits the base walk) won't be flagged as having the capability. No clear evidence that capability-based routing in production honors the right path.
- **Encryption boundary.** `EncryptedSerializer` wraps another serde (`libs/checkpoint/langgraph/checkpoint/serde/encrypted.py`); `with_msgpack_allowlist` is propagated through the wrapper. Effect on replay is not separately documented.
- **`JsonPlusSerializer` default permissive mode is the security-relevant tradeoff for event-store workloads.** The strict gate is opt-in via env var. No evidence of a server-enforced default strict mode for production deployments.

---

Generated by `02.03-event-sourcing-and-replay-state` against `langgraph`.