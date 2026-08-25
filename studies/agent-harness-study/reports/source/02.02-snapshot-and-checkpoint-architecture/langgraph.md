# Source Analysis: langgraph

## 02.02 — Snapshot and Checkpoint Architecture

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (pregel/loop, checkpoint bases), SQL (Postgres/SQLite savers), psql/aiosqlite/psycopg |
| Analyzed | 2026-07-06 |

## Summary

LangGraph's checkpoint model is a **versioned, parent-linked, channel-state capture** taken after every Pregel superstep plus on entry/exit/branch events. The base interface (`BaseCheckpointSaver` at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176`) defines six must-implement methods plus four optional extensions (`delete_for_runs`, `copy_thread`, `prune`, plus `get_delta_channel_history` which is in beta). Three checkpoint sources (versions, "input|loop|update|fork") tag each saved tuple, and `durability` (`"sync"|"async"|"exit"` at `libs/langgraph/langgraph/types.py:87`) drives when `_put_checkpoint` actually fires. The Pregel loop writes a fresh `Checkpoint` after every superstep and an extra one for "input", "fork" (time-travel) and at loop exit when `durability="exit"` (`libs/langgraph/langgraph/pregel/_loop.py:706`, `959`, `1310`). Channel values are split between an inline JSON column and a `(channel, version)`-keyed blob table (Postgres) or inline-only SQLite row, and writes for the current superstep are stored separately as `pending_writes` so partial supersteps can be replayed on interrupt without re-running succeeded tasks. A new beta feature, `DeltaChannel` (`libs/langgraph/langgraph/channels/delta.py:25`), reduces blob size by storing a sentinel per intermediate step and reconstructing state via a parent-walk (`get_delta_channel_history`). Time travel, branching (`get_state_history` via `before=`, `update_state` fork at `main.py:2036`, `bulk_update_state` fork at `main.py:1812`), subgraph checkpoint scoping via `checkpoint_ns`, and parent-config plumbing (`parent_config` on every `CheckpointTuple`) are first-class.

## Rating

**8 / 10 — Clear model, explicit interfaces, proven tests; gaps in built-in lifecycle ops.**

The schema and superstep boundary semantics are well-defined and exercised by an entire conformance suite (`libs/checkpoint-conformance/`). There are explicit interfaces, migrations, async-safe primitives, and operational safeguards (delta-channel ancestor walks with safe-recovery warnings, futures synchronization for write→checkpoint ordering, source-tagging, parent chain integrity). What costs the rating: `prune`, `copy_thread`, and `delete_for_runs` are declared on the base but **none of the three built-in savers (InMemory, Sqlite, Postgres) implements them** (conformance specs exist at `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py`, `test_copy_thread.py`, `test_delete_for_runs.py`); the SQLite schema has **no secondary indexes** on `thread_id` (would matter at scale); the docstring at `base/__init__.py:691-712` actively warns that a naive `keep_latest` will silently corrupt `DeltaChannel` state, which is the kind of "footgun" smell that keeps this from a 9. Durability-mode invariants around exit-mode delta-channel writes and fork-on-time-travel are complex (multi-step future plumbing at `_loop.py:201-237`, `_loop.py:1201-1292`) and only documented in source comments. The model is correct and demonstrable; it is not yet simple to operate at scale out of the box.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Checkpoint data shape (TypedDict) | `v, id, ts, channel_values, channel_versions, versions_seen, updated_channels` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-123` |
| CheckpointMetadata schema (`source`, `step`, `parents`, `run_id`, delta counters) | TypedDict declaration | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| `BaseCheckpointSaver` interface (must-implement methods + signatures) | `get`, `get_tuple`, `list`, `put`, `put_writes`, `delete_thread` (sync + async) | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176-580` |
| Optional `delete_for_runs`, `copy_thread`, `prune` (raises NotImplementedError) | base stubs | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-415`, `521-580` |
| `PendingWrite` tuple type `(task_id, channel, value)` | TypedDict | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:31` |
| `CheckpointTuple` (config + checkpoint + metadata + parent_config + pending_writes) | NamedTuple | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:139-146` |
| `WRITES_IDX_MAP` (negative idx for ERROR/SCHEDULED/INTERRUPT/RESUME) | Lookup constant | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| Checkpoint ID generation (UUIDv6, monotonically time-ordered) | `uuid6` helper | `libs/checkpoint/langgraph/checkpoint/base/id.py:79-109` |
| Channel state capture (excludes `UntrackedValue` channels; sanitizes Send packets) | `put_writes` filter | `libs/langgraph/langgraph/pregel/_loop.py:432-447` |
| Superstep completion → `_put_checkpoint({"source": "loop"})` | After every loop tick | `libs/langgraph/langgraph/pregel/_loop.py:706` |
| First-tick input → `_put_checkpoint({"source": "input"})` | `_first` input path | `libs/langgraph/langgraph/pregel/_loop.py:1016` |
| Time-travel fork checkpoint | `_put_checkpoint({"source": "fork"})` in `_first` | `libs/langgraph/langgraph/pregel/_loop.py:959` |
| Exit-mode deferred checkpoint (`durability=="exit"`) | `_suppress_interrupt` final flush | `libs/langgraph/langgraph/pregel/_loop.py:1301-1311` |
| `update_state` fork source tag | bulk_update_state path | `libs/langgraph/langgraph/pregel/main.py:1732`, `:1818`, `:2036` |
| Durability literal type (sync/async/exit) | `Durability = Literal["sync","async","exit"]` | `libs/langgraph/langgraph/types.py:87-93` |
| Per-channel `next_versions` strategy (monotonic string counter `f"{next_v:032}.{next_h:016}"`) | `get_next_version` | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628`; `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:543-552`; `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:626-645` |
| Channel-to-blob staging (split inline + blob table); skip blobs for primitives | Postgres put | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:309-344` |
| Blob-table writes via (channel, version) PK | `UPSERT_CHECKPOINT_BLOBS_SQL` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:131-135` |
| Writes-table PK (`thread_id, ns, checkpoint_id, task_id, idx`) | `UPSERT_CHECKPOINT_WRITES_SQL` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-153` |
| Migration list (10 entries incl. `task_path` column addition) | `MIGRATIONS` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91` |
| Secondary indexes on `thread_id` (Postgres only) | `CREATE INDEX CONCURRENTLY` migrations | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:82-89` |
| SQLite schema (no secondary index) | `setup()` DDL | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:138-164` |
| `parent_checkpoint_id` column linking lineage | DDL | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:147`; `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:47-56` |
| Pending-send migration (`v < 4` migration on read) | `_migrate_pending_sends` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:308-326`; `_migrate_checkpoint` at `libs/langgraph/langgraph/pregel/main.py:1135-1142` |
| `_initial_checkpoint_config` & `_has_persisted_parent` (anchor for exit-mode delta writes) | loop attributes | `libs/langgraph/langgraph/pregel/_loop.py:231-237` |
| `_delta_write_futs` (drain delta writes before next checkpoint put) | loop attribute & drain | `libs/langgraph/langgraph/pregel/_loop.py:200-211`, `:1515-1524`, `:1769-1771` |
| `_error_handler_write_futs` (drain error writes before handler scheduling) | loop attribute | `libs/langgraph/langgraph/pregel/_loop.py:208-211` |
| `_suppress_interrupt` puts exit checkpoint, suppresses `GraphInterrupt` for outer graph | lifecycle | `libs/langgraph/langgraph/pregel/_loop.py:1294-1355` |
| Resume-from-checkpoint detection (`is_resuming`, `is_time_traveling`) | `_first` | `libs/langgraph/langgraph/pregel/_loop.py:836-1018` |
| Time-travel `ReplayState` (subgraph before-bound lookup) | `ReplayState` | `libs/langgraph/langgraph/_internal/_replay.py:14-90` |
| Pending-writes re-apply to ready tasks on resume | `_reapply_writes_to_succeeded_nodes` | `libs/langgraph/langgraph/pregel/_loop.py:724-737` |
| Error-handler resume from `ERROR_SOURCE_NODE` marker | `_resume_error_handlers_if_applicable` | `libs/langgraph/langgraph/pregel/_loop.py:739-804` |
| `get_delta_channel_history` (two-stage paged query) | Postgres fast-path | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:444-550` |
| SQLite delta ancestor walk (streaming cursor, single-merged walk) | helper module | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py:1-172` |
| Default (fallback) walk implementation walking `get_tuple + parent_config` | base method | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690` |
| `_DeltaSnapshot` blob sentinel | serde NamedTuple | `libs/checkpoint/langgraph/checkpoint/serde/types.py:19-31` |
| `DeltaChannel` reducer (snapshot cadence `(updates, supersteps)`) | channel | `libs/langgraph/langgraph/channels/delta.py:25-204` |
| `delta_channels_to_snapshot` predicate | helper | `libs/langgraph/langgraph/pregel/_checkpoint.py:37-58` |
| `channels_from_checkpoint` rebuilds channels; defers `DeltaChannel` to ancestor walk | helper | `libs/langgraph/langgraph/pregel/_checkpoint.py:136-184` |
| `InMemorySaver` (debug-only; context-manager + blob table) | class | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33-704` |
| Conformance test runner (`@checkpointer_test`, capability detection) | runtime | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/validate.py:45-127`; `capabilities.py:1-96` |
| Conformance spec for prune / copy_thread / delete_for_runs | test modules | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:1-217`; `test_copy_thread.py:1-250`; `test_delete_for_runs.py:1-218` |
| `get_state_history` (filter / before / limit) — implements time-travel via `before` cursor | pregel API | `libs/langgraph/langgraph/pregel/main.py:1479-1530` |
| `update_state` (single update → `bulk_update_state([[StateUpdate]])`) | API | `libs/langgraph/langgraph/pregel/main.py:2530-2541` |
| `bulk_update_state` fork + update supersteps | API | `libs/langgraph/langgraph/pregel/main.py:1589-2060` |
| `parents` metadata maps `checkpoint_ns → checkpoint_id` for subgraph replay | schema | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:56-60`; `patch_checkpoint_map` at `libs/langgraph/langgraph/_internal/_config.py:63-80` |
| `CONFIG_KEY_CHECKPOINT_MAP` + `CONFIG_KEY_CHECKPOINT_NS` propagation | constants | `libs/langgraph/langgraph/_internal/_constants.py:54`, `:46` |
| `get_state` → `_prepare_state_snapshot` (migrate + apply pending writes) | API | `libs/langgraph/langgraph/pregel/main.py:1144-1259` |
| `update_state` source `update` metadata | bulk_update_state | `libs/langgraph/langgraph/pregel/main.py:2039-2042`, `:2485-2521` |
| Source tag `fork` for copy-only update | bulk_update_state | `libs/langgraph/langgraph/pregel/main.py:1817-1822` |
| ShallowPostgresSaver (only latest, deprecation notice) | class | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:169-967` |
| Test guard for basesaver fallback re-entrancy | regression test | `libs/checkpoint/tests/test_memory.py:518-560` |
| Lazy stub creation on first-run exit-mode delta anchor | loop method | `libs/langgraph/langgraph/pregel/_loop.py:1230-1258` |

## Answers to Dimension Questions

**1. When is a checkpoint written?**
- On every Pregel superstep completion: `_loop.py:706` calls `_put_checkpoint({"source": "loop"})` from `after_tick`.
- On entry of the first superstep (input): `_loop.py:1016` calls `_put_checkpoint({"source": "input"})` after applying input writes.
- When a time-travel replay begins (`is_time_traveling` True and source not `update|fork`): `_loop.py:959` writes a fork checkpoint before the replay.
- From `update_state` / `bulk_update_state`: writes `source: "update"` at `main.py:1732`, `:2036`, `:2485`; and `source: "fork"` at `main.py:1818`, `:2288`, `:2520`.
- On loop exit with `durability == "exit"`: `_loop.py:1301-1311` writes a final checkpoint (including delta-channel writes anchored to the saved parent or a lazy stub) and suppresses `GraphInterrupt`.
- Tasks write their channel writes via `put_writes` after commit (`_runner.py:583`, `:591`, `:603`, `:613`); the run-mode gate is `self.durability != "exit"` so writes are streamed during `sync`/`async`, batched at exit under `durability=="exit"`.

**2. What is inside it?**
A `Checkpoint` TypedDict at `base/__init__.py:92-123` with `v` (format version), `id` (UUIDv6), `ts` (ISO-8601), `channel_values` (per-channel deserialized value, with `_DeltaSnapshot` blobs in the beta case), `channel_versions` (monotonic per-channel counters), `versions_seen` (per-node last-seen versions for trigger evaluation), and `updated_channels` (sorted list of channels touched in this step). `CheckpointMetadata` at `:38-86` adds `source`, `step` (-1 for input, 0 for first loop, N thereafter), `parents` (ns → checkpoint_id map for subgraphs), `run_id`, and (beta) `counters_since_delta_snapshot`. Writes are stored separately as `(task_id, channel, blob)` with `idx` derived from `WRITES_IDX_MAP` (`base/__init__.py:795`) so errors/interrupts/resumes never collide with regular channel writes.

**3. Can it restore execution or only data?**
Both. The checkpointer is the synchronization point: `PregelLoop.__enter__` calls `get_tuple`, then `channels_from_checkpoint` rebuilds channel state (`_checkpoint.py:136-184`), then `_first` decides resume vs fresh (`_loop.py:836-1018`). A loaded checkpoint carries `pending_writes`; `_reapply_writes_to_succeeded_nodes` (`_loop.py:724-737`) re-injects them so previously-committed tasks skip re-execution, while failures / interrupts / errors are skipped so they re-run or get routed to an error handler. Task ordering, retry policy, and channel versions are all preserved, so re-entering the loop yields deterministic continuation. The `update_state` / `bulk_update_state` API can also fork a new branch without re-executing (`main.py:1818`).

**4. Can checkpoints branch?**
Yes — three mechanisms:
- **Time travel**: `get_state(config={"configurable": {..., "checkpoint_id": <id>}})` loads that exact id; a forked replay then writes a new checkpoint via `source: "fork"` (`_loop.py:959`).
- **Branch via `get_state_history(..., before=<config>)`**: returns prior checkpoints in `CheckpointTuple.parent_config` form (`base/__init__.py:139-146`).
- **`bulk_update_state` with `as_node="__copy__"`** writes a fresh `source: "fork"` checkpoint from a copy of an existing one (`main.py:1799-1822`), so the original lineage is preserved. The checkpoint's `parents` metadata (`:56-60`) records the ns→cid map and is patched via `patch_checkpoint_map` (`_internal/_config.py:63-80`) onto the next config so subgraph replays know where to land.

**5. Are checkpoints pruned safely?**
Only via the explicit `prune(thread_ids, strategy="keep_latest"|"delete")` API (`base/__init__.py:374-415`). The base method docstring loudly warns at `:387-414` that a naive `keep_latest` on a thread using `DeltaChannel` will silently corrupt delta-channel state because intermediate checkpoints and their writes anchor an ancestor walk; it gives three recipes for safe pruning (walk back from kept checkpoint preserving ancestors up to a `_DeltaSnapshot`, force a snapshot before delete, or skip pruning). However, **none of the bundled Postgres/SQLite/InMemory savers implements `prune`** — a `grep` for `def prune` over `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, and `libs/checkpoint/memory` returns only the base. The conformance test suite (`test_prune.py`) exists to validate savers that opt in. This is a real gap between the documented contract and the shipped implementations.

## Architectural Decisions

- **Capture channel state, not call state**: the checkpoint is the *materialized* channel state at the end of a superstep plus the in-flight `pending_writes`; it deliberately does not store node functions or call stacks. This keeps captures small and replays deterministic.
- **Parent-pointing lineage**: every checkpoint row stores `parent_checkpoint_id` (`:47-56` Postgres DDL; `:147` SQLite DDL). All restore, time-travel, branch, and replay logic leans on this single pointer — `_loop.py:853`, `_algo.py:1251`, `_replay.py:65-72`.
- **Split inline/blob storage** (Postgres): small primitive values stay inline in the JSONB checkpoint, large/complex blobs go to `checkpoint_blobs` keyed by `(thread_id, ns, channel, version)` (`__init__.py:309-344`). SQLite stores everything inline (no JSONB equivalent). Reads always rehydrate via `serde.loads_typed`.
- **Background-executor + serialized `_put_checkpoint` ordering** (`_loop.py:1507-1524`, `:1759-1778`): puts are submitted to the executor but a linked future is awaited before the *next* save starts, so checkpoints reach storage in superstep order even when saves are async.
- **Two-tier durability** (`types.py:87`): `sync` blocks before next step, `async` lets the next step run while the save is in flight, `exit` only flushes once on loop exit. The `delta_write_futs` drain (`_loop.py:1515-1524`, `:1769-1771`) and exit-mode `_put_exit_delta_writes` stub (`_:1201-1292`) enforce the invariant: a checkpoint is never visible without its backing writes.
- **Delta-channel compression** (beta, `_checkpoint.py:37-184` and `channels/delta.py`): most non-snapshot checkpoints store only a sentinel per `DeltaChannel`; full state is replayed from `get_delta_channel_history`, which has a 2-stage paged SQL form on Postgres (`__init__.py:444-550`) and a streaming cursor walk on SQLite (`_delta.py:34-172`).
- **Metadata-rich provenance**: `source`, `step`, `parents`, `run_id` capture *why* a checkpoint exists, supporting audits, replay-to-replay, and time-travel from arbitrary history slices.
- **Apply-don't-store for pending writes**: pending writes live in their own table keyed by `(thread_id, ns, checkpoint_id, task_id, idx)`. ERROR / SCHEDULED / INTERRUPT / RESUME use negative `idx` (`WRITES_IDX_MAP` at `base/__init__.py:795`) so control writes can't collide with regular channel writes.

## Notable Patterns

- **Two-stage ancestor walk** (Postgres, `_postgres/__init__.py:444-550`): stage 1 pages the parent chain checking `channel_values[k] IS NOT NULL` and `channel_versions[k]` per requested channel via dynamic column-pair JSONB lookups (one pair per channel); stage 2 fires a `UNION ALL` (one writes branch per channel with non-empty chain, one blobs branch per seeded channel). Empirically documented as 3× faster end-to-end with 61% smaller wire payload than the "ship full JSONB" alternative (note at `base.py:186-198`).
- **Streaming single-merged walk** (SQLite, `_delta.py:68-124`): because SQLite has no JSONB, the cursor streams `(checkpoint_id, parent_id, type, blob)` rows newest-first; the caller deserializes each row only when it matches the current walk target, freeing the blob before advancing. Keeps peak memory at one deserialized checkpoint.
- **Lazy stub for exit-mode first-run** (`_loop.py:1230-1258`): when no parent checkpoint exists and `durability="exit"`, `_put_exit_delta_writes` writes an empty stub at `step=-2` first so that accumulated delta writes have a parent to anchor under. Without this, the first-run exit path would have no anchor and the loader would walk off the end of the chain.
- **Source-tagged provenance** (`CheckpointMetadata.source` ∈ {`input`,`loop`,`update`,`fork`}): every checkpoint is stamped with its origin, and the time-travel path explicitly checks for `source != "update"|"fork"` before writing its own fork (`_loop.py:948-959`).
- **Subgraph-aware replay** (`_loop.py:866-883`, `_replay.py:14-90`): `ReplayState` is shared by reference across all derived configs of one execution. On replay, subgraphs fetch their *pre-replay* checkpoint via `checkpointer.list(..., before={...})`, then resume forward; later invocations of the same logical subgraph fall back to normal latest loading.
- **Channel isolation across checkpoints** (`_loop.py:432-447`): `UntrackedValue` channels are stripped from `put_writes`, so runtime-only state (e.g. scratch tags) never pollutes the durable checkpoint.
- **Exiting-vs-intermediate checkpoint detection via identity check** (`_loop.py:1064-1074`): `metadata is self.checkpoint_metadata` is used as the "this is the exit flush" flag. The author explicitly flags it as a fragile idiom and calls for replacement by an explicit parameter; left as-is to match repo style.

## Tradeoffs

- **Per-superstep cadence**: writing a checkpoint on every step gives free resilience and time-travel granularity but inflates write volume. The `durability` modes trade fidelity for IO cost. Delta-channel compression is a partial mitigation at the cost of needing the entire parent chain alive for replay.
- **Blob split (Postgres only)**: reduces checkpoint-blob size in the JSONB column but adds an extra round-trip per read (and a `UNION ALL` two-stage delta walk). Postgres wins via pipeline mode + JSONB; SQLite doesn't have the equivalent and pays full inline cost on every read.
- **No `prune`/`copy_thread`/`delete_for_runs` shipped**: the API lives in the base, conformance specs exist, but none of `InMemorySaver`, `SqliteSaver`, `AsyncSqliteSaver`, `PostgresSaver`, `AsyncPostgresSaver`, or either `Shallow*` variant implements these. Users who want them must write their own or rely on raw SQL.
- **Serialization trust**: `JsonPlusSerializer` (`serde/jsonplus.py:82-95`) defaults to permissive Python-object deserialization; the module accepts untrusted data only if the operator opts in to `LANGGRAPH_STRICT_MSGPACK=true` or an explicit allowlist. This is a security/feature tradeoff: maximum compat for in-app state, footgun if the saver is reachable by an attacker.
- **`metadata is self.checkpoint_metadata` identity check (`_loop.py:1064-1074`)**: works because in-flight metadata is a fresh dict; the comment notes that converting to an explicit `exiting: bool` parameter would be cleaner. Currently relies on a single, hard-to-discover idiom.
- **SQLite missing secondary indexes** (`sqlite/__init__.py:138-164`): `WHERE thread_id = ?` on every read becomes a full scan as threads grow. The Postgres savern adds `CREATE INDEX CONCURRENTLY` migrations for `checkpoints`, `checkpoint_blobs`, `checkpoint_writes` on `thread_id` (`base.py:82-89`).
- **Two competing superstep-write APIs**: older `Pregel.update_state`/`bulk_update_state` calls go straight to the checkpointer (`main.py:2036`, `:2288`); the new `_loop.py` path uses `BackgroundExecutor` future chain. Code paths overlap; both are production-supported.
- **Beta contracts in stable surface**: `_DeltaSnapshot`, `DeltaChannel`, `get_delta_channel_history`, `counters_since_delta_snapshot` are shipped in a "beta" warning that explicitly promises to keep on-disk history readable but reserves the right to change field names and semantics. Operators must accept beta churn.

## Failure Modes / Edge Cases

- **Naive prune on a DeltaChannel-backed thread**: `base/__init__.py:387-414` documents three safe-recovery recipes; failing to follow one leads to silent channel corruption (no error, `get_delta_channel_history` simply returns no `seed`, channel reconstructs as empty).
- **Concurrent delta-channel walks on the same loop**: the base fallback's task-local re-entrancy guard is exercised by `test_memory.py:518-560`; concurrent `aget_delta_channel_history` calls must each see a full walk, not short-circuit to `writes=[]`. Regression was caught and fixed.
- **Writing DeltaChannel before `apply_writes` bumps the channel version** (`_checkpoint.py:91-108`): in exit mode a snapshot decision may be deferred until after the last superstep; the manual version bump in `create_checkpoint` closes the gap or `saver.put()` would silently drop the snapshot blob.
- **Pre-delta migration** (`serde/types.py:19-31`, `tests/test_memory.py:563-680`): a thread written by a pre-delta saver has plain values at older `channel_versions[k]`; reconstruction must seed from the first such ancestor's blob and ignore any writes stored under that ancestor (the blob subsumes them). Exhaustive unit tests guard this.
- **Time-travel mid-superstep before `after_tick`** (`_loop.py:940-959`): if a replay runs and hits an interrupt before producing its first checkpoint, the head pointer stays at the parent and subsequent resumes load stale state. The fork-checkpoint in `_first` exists to avoid this.
- **Pending-send migration on read** (`postgres/__init__.py:165-188`, `:246-261`): checkpoints with `v < 4` may carry `pending_sends` separate from `channel_values`; the saver runs an extra `SELECT_PENDING_SENDS_SQL` per listed checkpoint and migrates in place, all under the same cursor. If the migration SQL fails the tuple is still returned (caller sees incomplete data) — `try/except` is not wrapped.
- **Checkpoint race if durability is `async` and the next step produces an interrupt** (`_suppress_interrupt`): handled by draining `self._put_checkpoint_fut` in `__exit__` flow, but documented as "no explicit wait for the put future" — callers that require strict durability must use `sync` or `exit`.
- **`loop` vs `from_thread` mapping** (`UPDATE` vs `NULL_TASK_ID`): input writes use `NULL_TASK_ID` (`_loop.py:921`) which is collapsed into accumulated channel reads; if a channel accidentally has both `NULL_TASK_ID` writes and user-named writes that look identical, the NULL path wins.
- **Loss of subgraph checkpointer when not explicit** (`main.py:1625-1637`): if `CONFIG_KEY_CHECKPOINTER` is not in the subgraph config, `bulk_update_state`/`get_state` of a subgraph will return without the checkpointer, leading to `ValueError("No checkpointer set")`. Inherited from parent checkpointer pattern at `types.py:107-109`.

## Future Considerations

- **Add first-class `prune`/`copy_thread`/`delete_for_runs`** to the bundled savers (or document a clear migration to the suite's PGO-backed implementations). Conformance specs already exist (`test_prune.py`, `test_copy_thread.py`, `test_delete_for_runs.py`), so this is filling in known gaps rather than design work.
- **Secondary indexes on the SQLite checkpoint tables** would mirror the Postgres migrations at `base.py:82-89`. SQLite syntax (`CREATE INDEX IF NOT EXISTS checkpoint_thread_id_idx ON checkpoints(thread_id)`) is trivial and would noticeably help read-heavy workloads.
- **Beta → stable for `DeltaChannel`**: the API surface (`DeltaChannel`, `_DeltaSnapshot`, `get_delta_channel_history`, `counters_since_delta_snapshot`) is documented as beta across `serde/types.py`, `channels/delta.py`, `base/__init__.py`. Promoting them to stable requires committing to a no-migration format — current docs say "threads written today are expected to remain readable" but warn that surrounding contracts may shift.
- **Replace identity-check `exiting` flag with an explicit `exiting: bool`** at `_loop.py:1064-1074`; the comment already calls it out as a fragile idiom.
- **Default-on `LANGGRAPH_STRICT_MSGPACK`** for newly-provisioned deployments — serde docstring at `serde/jsonplus.py:82-95` highlights deserialization risk; default-secure would prevent a class of supply-chain attacks against the saver.
- **Pipeline-mode adoption beyond Postgres**: the `_cursor` shim (`postgres/__init__.py:404-442`) is a Postgres-specific optimization. If SQLite got a write-coalescing extension or a Postgres-style pipeline, save latency could drop further.
- **A `copy_thread` builtin**: most operators want a "duplicate thread" workflow (e.g., for a fresh A/B run); today `copy_thread` is `NotImplementedError` everywhere, forcing users to write their own SQL.
- **Documented cross-version migration strategy** for checkpoint format upgrades (`LATEST_VERSION` at `_internal/_config.py` and `v` per Checkpoint is 4 in `langgraph._checkpoint.LATEST_VERSION = 4`, `langgraph.checkpoint.base.LATEST_VERSION = 2` — yet another version field in the base; refactor to a single source of truth).

## Questions / Gaps

- **Why are `prune`, `copy_thread`, `delete_for_runs` declared but unimplemented across Postgres/SQLite/InMemory?** They appear in the conformance test catalog (`test_prune.py`, `test_copy_thread.py`, `test_delete_for_runs.py`) and the base class, yet no shipped saver overrides `aprune`/`acopy_thread`/`adelete_for_runs`. No design doc explaining why. No evidence found in the source for an explicit decision to defer them.
- **What is the relationship between `checkpoint.base.LATEST_VERSION = 2` (`base/__init__.py:811`) and `langgraph._checkpoint.LATEST_VERSION = 4` (`pregel/_checkpoint.py:21`)?** Two parallel version constants; the latter is used to bump `Checkpoint.v` on write, the former is annotated as a "deprecated utility used by past versions". The split is confusing and undocumented.
- **`v=4` migration on Postgres** (`postgres/base.py` MIGRATIONS list) — the only on-read migration is for `pending_sends`. Where does the bump from `v=1/2/3 → v=4` take place, and is the migration idempotent for threads that predate the version constant?
- **No SQLite migration table** (`sqlite/__init__.py:138-164`) — Postgres tracks `checkpoint_migrations` (`base.py:44-46`); SQLite calls `executescript` on every setup with `IF NOT EXISTS`. Adding a future column will require a migration registry, which is currently absent.
- **Is `Checkpoint.pending_sends` really gone?** The base `Checkpoint` TypedDict (`base/__init__.py:92-123`) does not list `pending_sends`, but `_load_checkpoint_tuple` in `postgres/__init__.py:573-578` and `shallow.py:282-291` reads `pending_sends` from old rows. The post-`v=4` on-read migration at `main.py:1135-1142` moves `pending_sends` into `channel_values[TASKS]`. There is no in-source explanation of the lineage boundary — does any active write path still emit `pending_sends`?
- **Is the JSON column used?** Postgres schema declares a `type TEXT` column on both `checkpoints` and `checkpoint_writes` (`base.py:52`, `:73`), but the writes row doesn't populate `type` from `serde.dumps_typed` (it does in `_dump_writes` for writes, but `checkpoints.put` always sets `type` to `None` because the JSONB column doesn't store typed envelopes inline). Confirming this asymmetry requires reading both `_dump_writes` and `UPSERT_CHECKPOINTS_SQL` flows.
- **What's the operational story for very long threads?** Each `get_state(config)` loads the full blob for the latest checkpoint; history reads scan the thread's checkpoints. Without retention policy in the bundled savers, operators must implement their own.
- **Are delta writes ever lost on `interrupt(GraphInterrupt)`?** `_suppress_interrupt` drains `self._put_checkpoint_fut` before exiting (`_loop.py:1310`) but does not explicitly drain `put_writes` futures; `commit()` in `_runner.py:574-613` calls `self.put_writes()` synchronously per task. Confirm via test in `tests/test_delta_channel_exit_mode.py` whether all in-flight writes have settled.

---

Generated by `02.02-snapshot-and-checkpoint-architecture` against `langgraph`.
