# Source Analysis: langgraph

## Persistence Durability Tiers

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python 3.11+ (monorepo: libs/checkpoint, libs/checkpoint-sqlite, libs/checkpoint-postgres, libs/checkpoint-conformance, libs/langgraph, libs/cli) |
| Analyzed | 2026-07-10 |

## Summary

LangGraph exposes **four named durability tiers** that the operator explicitly selects per call:

1. `Durability = "sync"` — every superstep's checkpoint write completes (returns) before the next step starts; default for the explicit `durability="sync"` mode (`libs/langgraph/langgraph/pregel/_loop.py:1117`, `libs/langgraph/langgraph/pregel/main.py:3002-3003`).
2. `Durability = "async"` (default) — checkpoint and per-task `put_writes` calls are submitted to a `BackgroundExecutor` thread pool / asyncio tasks; the loop only blocks on `_put_checkpoint_fut` after each `after_tick()` so an in-flight write cannot race the next step (`libs/langgraph/langgraph/pregel/_loop.py:459-498`, `libs/langgraph/langgraph/pregel/_loop.py:1182-1189`, `libs/langgraph/langgraph/pregel/main.py:2999-3003`).
3. `Durability = "exit"` — checkpoint and writes are deferred until `__exit__` / `__aexit__` of the `SyncPregelLoop`/`AsyncPregelLoop`; the `_suppress_interrupt` hook is the one site that calls `_put_checkpoint` and `_put_pending_writes` when `self.durability == "exit"` (`libs/langgraph/langgraph/pregel/_loop.py:1301-1311`).
4. No checkpointer at all (`checkpointer=None` or `checkpointer=False`) — nothing survives even a SIGKILL of the running process; the Pregel loop drops `checkpointer_put_writes` to `None` and uses an in-memory `increment()` as `get_next_version` (`libs/langgraph/langgraph/pregel/_loop.py:1501-1505`).

The **storage backend tier** is orthogonal: each `BaseCheckpointSaver`/`BaseStore`/`BaseCache` implementation provides its own durability, and the operator picks one. The shipped backends are:

- Checkpoint savers: `InMemorySaver` / `MemorySaver` (process memory, lost on exit), `SqliteSaver`/`AsyncSqliteSaver` (local file or `:memory:`), `PostgresSaver`/`AsyncPostgresSaver` (full Postgres with `checkpoints`+`checkpoint_blobs`+`checkpoint_writes` tables), and the deprecated `ShallowPostgresSaver` / `AsyncShallowPostgresSaver` that holds only the latest checkpoint (no history).
- Stores: `InMemoryStore`, `SqliteStore`/`AsyncSqliteStore`, `PostgresStore`/`AsyncPostgresStore`, all implementing the `BaseStore` interface (`libs/checkpoint/langgraph/store/base/__init__.py:700-928`). TTL is opt-in per adapter via `supports_ttl: bool = True` (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:728`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:773`); `InMemoryStore` does **not** set it.
- Caches: `InMemoryCache` (process memory only) and `RedisCache` (`libs/checkpoint/langgraph/cache/memory/__init__.py:11-72`, `libs/checkpoint/langgraph/cache/redis/__init__.py:10-144`). Redis cache fails silently: every `mget`/`setex`/`keys` is wrapped in `try/except Exception: return {} / pass`, so a Redis outage degrades to cache-miss without raising (`libs/checkpoint/langgraph/cache/redis/__init__.py:62-66, 104-108, 117-139`).

The full configuration surface is documented in `libs/cli/langgraph_cli/schemas.py:9-233` (`TTLConfig`, `IndexConfig`, `StoreConfig`, `ThreadTTLConfig`, `SerdeConfig`, `CheckpointerConfig`) and is wired through the Docker image builder at `libs/cli/langgraph_cli/config.py:1135-1153` (`LANGGRAPH_STORE`, `LANGGRAPH_CHECKPOINTER` env vars).

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:

- The four-mode `Durability` literal is typed and validated at every entry point (`libs/langgraph/langgraph/types.py:87`); the warnings on misuse are explicit (`libs/langgraph/langgraph/pregel/main.py:2817-2820`).
- The four-tier store taxonomy (in-memory / SQLite / Postgres / external cache) is documented per adapter and gated behind explicit `setup()` calls plus `BaseCheckpointSaver`/`BaseStore` contracts.
- **Subtract points**: `InMemorySaver` is the documented default when nothing is configured, and its persistence story is "lost when the process exits" (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33-66`). The InMemorySaver class docstring explicitly says "Only use `InMemorySaver` for debugging or testing purposes. For production use cases we recommend installing [langgraph-checkpoint-postgres]" — operators who do not read the docstring will silently lose state on restart.
- **Subtract points**: `EphemeralValue` and `UntrackedValue` channels (`libs/langgraph/langgraph/channels/ephemeral_value.py:15`, `libs/langgraph/langgraph/channels/untracked_value.py:15`) intentionally never checkpoint (`checkpoint() -> MISSING`). Putting important state into an `UntrackedValue` channel is a silent no-op persistence footgun.
- **Subtract points**: There is no first-class TTL/eviction on the checkpointer itself; thread lifetime must be managed by `delete_thread`/`prune` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:320-416`). TTL is only wired for the *store*, not for the checkpointer, even though `ThreadTTLConfig` exists in the CLI schema (`libs/cli/langgraph_cli/schemas.py:109-124`).
- **Subtract points**: ShallowPostgresSaver is deprecated but still exported (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197`); users who adopted it for "single-writer only" deployments need to migrate to `durability="exit"` to get equivalent semantics.
- **Bonus**: Postgres setup runs a real migration system with versioned DDL and indexes (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`); the conformance test library (`libs/checkpoint-conformance`) lets operators verify any custom backend against the same spec the shipped savers are tested against.
- **Bonus**: The DeltaChannel history walker is implemented with `get_delta_channel_history` for all three storage tiers (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:503-583`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:444-550`) — same algorithm shape, JSONB-paged on Postgres, blob-streamed on SQLite, parent-chain walked in-process on InMemory.

The exact quote from the dimension rubric — "Can operators tell which state is production-durable?" — is mostly answerable *if* the operator reads the `Durability` and `BaseCheckpointSaver`/`BaseStore` docs. The information is present, but the *defaults* (Async, InMemory if nothing is configured) are the lowest-durability option, and there are silent drop-paths (`EphemeralValue`, `UntrackedValue`, `drain_mode` exit-only checkpoints) that are easy to miss.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| `Durability` literal definition | `Literal["sync", "async", "exit"]` with per-mode semantics | `libs/langgraph/langgraph/types.py:87-93` |
| Default durability mode (`"async"`) | `durability = config.get(CONF, {}).get(CONFIG_KEY_DURABILITY, "async")` | `libs/langgraph/langgraph/pregel/main.py:2617-2618` |
| `durability="sync"` blocks the loop after each tick | `if durability_ == "sync": loop._put_checkpoint_fut.result()` | `libs/langgraph/langgraph/pregel/main.py:3002-3003` |
| `durability="async"` submits checkpoint to background executor | `self._put_checkpoint_fut = self.submit(self._checkpointer_put_after_previous, ...)` | `libs/langgraph/langgraph/pregel/_loop.py:1182-1189` |
| `durability="async"` also submits per-task writes in background | `fut = self.submit(self.checkpointer_put_writes, config, writes_to_save, task_id, ...)` | `libs/langgraph/langgraph/pregel/_loop.py:474-487` |
| `durability="exit"` only persists on context manager exit | `if self.durability == "exit" and (not self.is_nested or exc_value is not None ...)` | `libs/langgraph/langgraph/pregel/_loop.py:1301-1311` |
| `durability` config key constant | `CONFIG_KEY_DURABILITY = sys.intern("__pregel_durability")` | `libs/langgraph/langgraph/_internal/_constants.py:68-69` |
| `BaseCheckpointSaver` abstract interface | `BaseCheckpointSaver` with `get_tuple`, `list`, `put`, `put_writes`, `delete_thread`, `delete_for_runs`, `copy_thread`, `prune` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176-580` |
| `InMemorySaver` storage is process RAM | `storage: defaultdict`, `writes: defaultdict`, `blobs: dict` — all in-memory | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:69-99` |
| `InMemorySaver` explicitly not for production | docstring: "Only use `InMemorySaver` for debugging or testing purposes" | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:36-44` |
| `InMemorySaver` is the loop's default when no checkpointer is passed | loop falls back to `increment()` for `get_next_version` and `checkpointer_put_writes=None` | `libs/langgraph/langgraph/pregel/_loop.py:1501-1505` |
| `PersistentDict` (legacy file-backed helper) | pickled file via `sync()` and atomic rename, present but not wired into `InMemorySaver` | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:634-704` |
| SQLite saver schema (sync) | `PRAGMA journal_mode=WAL; CREATE TABLE checkpoints / writes` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:139-164` |
| SQLite saver thread safety | `self.lock = threading.Lock()` inside `_cursor()` context manager | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:95, 168-189` |
| SQLite `:memory:` mode | `SqliteSaver.from_conn_string(":memory:")` uses private in-RAM database | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:97-127` |
| Async SQLite saver schema | identical to sync, plus async lock and `aiosqlite.connect(...)` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:305-344` |
| Async SQLite "use Postgres in production" warning | docstring: "While this class supports asynchronous checkpointing, it is not recommended for production workloads" | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:52-56` |
| Postgres schema (checkpoints / checkpoint_blobs / checkpoint_writes) | DDL in `MIGRATIONS` list, with JSONB checkpoints and BYTEA blobs | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91` |
| Postgres migration framework | versioned `MIGRATIONS` applied in `setup()`, recorded in `checkpoint_migrations` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:85-108` |
| Postgres put = `UPSERT_CHECKPOINTS_SQL` | `INSERT ... ON CONFLICT ... DO UPDATE SET checkpoint = EXCLUDED.checkpoint, metadata = EXCLUDED.metadata;` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:137-144` |
| Postgres uses Pipeline mode when available | `Capabilities().has_pipeline()` decides per-connection | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:60, 426-439` |
| ShallowPostgresSaver (deprecated, history-stripped) | "ONLY stores the most recent checkpoint and does NOT retain any history" + `DeprecationWarning` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:169-198` |
| ShallowPostgresSaver put deletes sibling writes | `DELETE FROM checkpoint_writes WHERE thread_id = ? AND checkpoint_ns = ? AND checkpoint_id NOT IN (...)` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:420-449` |
| `BaseStore` (long-term memory) interface | `BaseStore` with `batch`, `abatch`, `get`, `put`, `search`, `list_namespaces`, TTL plumbing | `libs/checkpoint/langgraph/store/base/__init__.py:700-936` |
| `InMemoryStore` storage is process RAM | `_data: dict[tuple[str, ...], dict[str, Item]]`, `_vectors: dict[...]` | `libs/checkpoint/langgraph/store/memory/__init__.py:186-204` |
| `InMemoryStore` TTL is unsupported | no `supports_ttl = True`; `put(ttl=...)` raises if ttl explicitly set | `libs/checkpoint/langgraph/store/base/__init__.py:911-916` |
| `PostgresStore` TTL support and sweeper thread | `supports_ttl: bool = True`, `start_ttl_sweeper`, `sweep_ttl` issues `DELETE ... WHERE expires_at < NOW()` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:728, 807-879` |
| `SqliteStore` TTL support | `supports_ttl = True`, mirror sweeper loop | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:773, 977-1046` |
| `BaseCache` interface | `BaseCache.get/set/clear` with TTLs | `libs/checkpoint/langgraph/cache/base/__init__.py:15-48` |
| `InMemoryCache` storage | `_cache: dict[Namespace, dict[str, tuple[enc, val, expiry]]]`, no persistence | `libs/checkpoint/langgraph/cache/memory/__init__.py:11-72` |
| `RedisCache` silent failure on outage | every `mget`/`set`/`keys` call wrapped in `try/except: return {} / pass` | `libs/checkpoint/langgraph/cache/redis/__init__.py:62-66, 104-108, 117-139` |
| `EphemeralValue` channel — never persisted | `def checkpoint(self) -> Value: return self.value` (and `_first` clears after) | `libs/langgraph/langgraph/channels/ephemeral_value.py:15-78` |
| `UntrackedValue` channel — never persisted | `def checkpoint(self) -> Value \| Any: return MISSING` | `libs/langgraph/langgraph/channels/untracked_value.py:48-49` |
| `UntrackedValue` writes stripped from `put_writes` | `if not isinstance(self.specs.get(c), UntrackedValue)` filter | `libs/langgraph/langgraph/pregel/_loop.py:432-447` |
| `DeltaChannel` writes persisted as ancestor-walk replay | `DeltaChannel.checkpoint() -> MISSING` + saver walks `pending_writes` on read | `libs/langgraph/langgraph/channels/delta.py:194-203` |
| Delta-channel snapshot cadence counters | `counters_since_delta_snapshot` metadata + system-wide `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` default 5000 | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:62-86, 1079-1142` |
| `get_delta_channel_history` exists for every tier | base default + `InMemorySaver` (in-process walk) + Postgres (two-stage JSONB) + SQLite (streamed walk + per-channel UNION ALL) | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`, `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:142-229`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:444-550`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:503-583` |
| `BasePostgresSaver.delete_thread` cascades to all three tables | `DELETE FROM checkpoints / checkpoint_blobs / checkpoint_writes` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402` |
| `SqliteSaver.delete_thread` cascades | `DELETE FROM checkpoints / writes` (no separate blob table in SQLite) | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501` |
| `BaseCheckpointSaver.prune` baseline = `NotImplementedError` | no-op default; subclasses must opt in | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415` |
| Conformance test scaffolding | `@checkpointer_test(name=...)` decorator + `validate()` runner | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/initializer.py:59-99` |
| Conformance test spec coverage | `test_copy_thread`, `test_delete_thread`, `test_delete_for_runs`, `test_get_tuple`, `test_list`, `test_prune`, `test_put`, `test_put_writes`, `test_delta_channel_history` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/*.py` |
| `LangSmith Deployment` no checkpointer needed | documented in `InMemorySaver` docstring | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:42-44` |
| Production CLI config surface | `StoreConfig`, `CheckpointerConfig`, `ThreadTTLConfig`, `SerdeConfig` in `langgraph_cli/schemas.py` | `libs/cli/langgraph_cli/schemas.py:83-232` |
| Docker image wires store+checkpointer via env | `ENV LANGGRAPH_STORE=...` / `ENV LANGGRAPH_CHECKPOINTER=...` | `libs/cli/langgraph_cli/config.py:1135-1152` |
| Custom-checkpointer hook in production | `CheckpointerConfig.path = "./my_checkpointer.py:create_checkpointer"` | `libs/cli/langgraph_cli/schemas.py:199-217` |
| Test setup uses `:memory:` SQLite and a uniquely-named Postgres DB | `SqliteSaver.from_conn_string(":memory:")`, `CREATE DATABASE test_<uuid>` then `DROP DATABASE` after | `libs/langgraph/tests/conftest_checkpointer.py:60-141` |

## Answers to Dimension Questions

**1. What survives a restart?**

| Tier | After process exit | After machine death | After redeploy |
|------|-------------------|----------------------|----------------|
| `InMemorySaver` / `MemorySaver` | No (process RAM) | No | No |
| `InMemoryStore` / `InMemoryCache` | No (process RAM) | No | No |
| `SqliteSaver` with file path (e.g. `"checkpoints.sqlite"`) | Yes (single-file WAL DB) | Yes (if disk is durable) | Yes |
| `SqliteSaver` with `":memory:"` | No (private in-process DB, lost on close) | No | No |
| `AsyncSqliteSaver` with file path | Yes | Yes | Yes |
| `PostgresSaver` / `AsyncPostgresSaver` | Yes (DB-backed JSONB + BYTEA) | Yes (subject to Postgres durability) | Yes (multiple instances OK) |
| `ShallowPostgresSaver` (deprecated) | Yes, but only the latest checkpoint | Yes | Yes, but no history |
| `PostgresStore` / `SqliteStore` (store tier) | Yes | Yes | Yes |
| `RedisCache` | Yes (Redis itself is the durability boundary) | Depends on Redis config | Depends |
| `InMemoryCache` | No | No | No |
| `durability="exit"` + any backend | Yes — but only the *exit* checkpoint survives mid-run crashes (see question 4) | Same | Same |

Operator truth: **only a file-backed or networked backend survives process death; the in-memory tiers do not.**

**2. What survives redeploy?**

For redeploy (rolling pods / new container), only state written to a *shared* backing store survives. `PostgresSaver` and `PostgresStore` are safe by design because the DB is the shared resource. `SqliteSaver` survives redeploy only if the SQLite file is mounted on a persistent volume — there is no built-in multi-writer support, and a thread-locking layer is enforced (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:95, 168-189`). SQLite + multiple replicas without shared storage will diverge.

The server-side wiring in `libs/cli/langgraph_cli/config.py:1135-1152` passes the same `checkpointer.path`/`store` config to all replicas, but it does not guarantee that two replicas pointing at the same SQLite file will cooperate — SQLite's lock layer will serialize but not merge.

**3. What survives multi-instance operation?**

- `PostgresSaver` is designed for multi-instance. It uses upserts keyed on `(thread_id, checkpoint_ns, checkpoint_id)` and writes to `checkpoints`, `checkpoint_blobs`, `checkpoint_writes` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`). Two instances writing to the same thread will produce last-write-wins on the *head* checkpoint but not lose intermediate history (because checkpoints are immutable after insert and writes are append-only on the unique `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` key, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:67-76, 155-159`).
- `InMemorySaver` is single-process by definition.
- `SqliteSaver` is technically cross-instance safe on a shared filesystem but the codebase is explicit: "does not scale to multiple threads" (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:49-53`). Multi-instance SQLite must rely on the SQLite WAL + filesystem lock.
- `RedisCache` is multi-instance safe by virtue of Redis itself; failures are silent (see question 4).
- The Postgres `AsyncPostgresSaver` ships both a single-connection variant (`conn: Connection`) and a pool variant (`ConnectionPool`) for horizontal scale (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:46-55`).

**4. What is silently dropped?**

These are the silent-drop paths in the codebase:

1. **`durability="exit"` mid-run state**: intermediate checkpoints are never written; if the process crashes before `__exit__`/`__aexit__` runs, the latest intermediate superstep is lost. Only the eventual exit checkpoint (or `_put_exit_delta_writes` synthetic stub, `libs/langgraph/langgraph/pregel/_loop.py:1240-1292`) is durable. Confirmed by tests that count checkpoints: `assert n_checkpoints == (3 if durability != "exit" else 1)` (`libs/langgraph/tests/test_interruption.py:40, 82`).
2. **`EphemeralValue` channels**: never persisted (`libs/langgraph/langgraph/channels/ephemeral_value.py:15-78`). Reads after restart return MISSING.
3. **`UntrackedValue` channels**: returns MISSING from `checkpoint()` (`libs/langgraph/langgraph/channels/untracked_value.py:48-49`). Writes to these channels are stripped before `put_writes` (`libs/langgraph/langgraph/pregel/_loop.py:432-447`).
4. **`RedisCache` outages**: `try/except Exception` swallows all errors and returns an empty dict on read (`libs/checkpoint/langgraph/cache/redis/__init__.py:62-66, 74-77, 106-108, 137-139`). A cache outage looks like "no cache hit" to the caller — silent and indistinguishable from a true miss.
5. **`DeltaChannel` without snapshot blobs and without persisted writes**: the ancestor walk returns `seed: <missing>` and the consumer is documented to treat absence as "start empty" (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:594-602`). If a `prune` or `delete_for_runs` operation severs the chain, the delta channel silently reconstructs as empty (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:340-347, 528-538`).
6. **`InMemorySaver` writes inside `_load_blobs` skip the "empty" sentinel silently**: an unwritten channel just doesn't appear in `channel_values` (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:125-140`).
7. **`BaseCheckpointSaver.put` for `SyncPostgresSaver` with `pipeline=True`**: pipeline batching is only effective when the underlying libpq supports it; otherwise it silently falls back to a transaction context manager (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:426-439`). The fallback is not surfaced to the caller.
8. **`ShallowPostgresSaver`** deletes prior writes for the thread on each `put` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:420-430`) — history retention is a *silent* property of this saver, not configurable post-hoc.
9. **`pickle_fallback=False` (default)** in `JsonPlusSerializer` means non-msgpack-encodable objects silently fall back to JSON-only lossless serialization rather than a runtime error; users may be surprised that a custom class round-trips as a dict (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:97-120`).
10. **`LANGGRAPH_STRICT_MSGPACK` not set** → msgpack allowlist is permissive by default (`allowed_msgpack_modules=True`); the warning for unregistered types is deduped in a process-global set capped at 1000 entries (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:53-79`).

**5. Is durability configurable?**

Yes, at three levels:

- **Per-call**: `graph.invoke(..., durability="sync" | "async" | "exit")` on every entry point (`libs/langgraph/langgraph/pregel/main.py:2642, 2662, 2681, 3050, 3070, 3089, 3809, 3845, 3862, 3986, 4004, 4022, 4039`). Default is `"async"` (`libs/langgraph/langgraph/pregel/main.py:2617-2618`).
- **Per-backend**: choose `InMemorySaver` / `SqliteSaver` / `PostgresSaver` / custom at graph compile time (`libs/langgraph/langgraph/pregel/main.py:2594-2608`); for production deploy, choose via `langgraph.json` (`libs/cli/langgraph_cli/schemas.py:193-232`) or the env vars `LANGGRAPH_CHECKPOINTER` / `LANGGRAPH_STORE` (`libs/cli/langgraph_cli/config.py:1135-1152`).
- **Per-store**: TTL via `TTLConfig` (`libs/checkpoint/langgraph/store/base/__init__.py:545-565`); store backend via `StoreConfig` in CLI (`libs/cli/langgraph_cli/schemas.py:83-106`).

There is **no** per-call toggle to switch from "in-memory" to "Postgres" at runtime — the choice is fixed when the graph is compiled.

## Architectural Decisions

- **Storage tier is decoupled from execution tier.** The `Durability` literal controls *when* state hits the saver; the saver class controls *where* it lands. This lets operators tune write latency (`async` vs `sync`) without changing the durability backend.
- **Migrations are first-class.** Both Postgres checkpoints and Postgres store ship a versioned `MIGRATIONS` list applied by `setup()` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:62-89, 1088-1160`). The `_get_version` helper reads the current `checkpoint_migrations.v` and only applies the delta.
- **Two-track write path for checkpoints.** Every checkpoint write goes through `_checkpointer_put_after_previous`, which (a) drains `_delta_write_futs` so delta-channel writes for a superstep land before the checkpoint that references them, and (b) waits for the previous checkpoint future to chain writes in order (`libs/langgraph/langgraph/pregel/_loop.py:1507-1524, 1759-1778`).
- **Inline vs blob storage.** Postgres inlines JSONB-checkpointable channels and pushes only non-JSON-friendly channels into `checkpoint_blobs` as BYTEA (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:311-344`). SQLite inlines everything in the `checkpoint` BLOB column (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:418-443`).
- **Background executor isolates writes from the main loop.** `BackgroundExecutor` / `AsyncBackgroundExecutor` (`libs/langgraph/langgraph/pregel/_executor.py:40-223`) is a thin wrapper over `concurrent.futures.ThreadPoolExecutor` / `asyncio` that records cancellation flags and re-raises on `__exit__`. It is the engine that makes `durability="async"` actually fast.
- **`EphemeralValue` / `UntrackedValue` are first-class channels** with explicit "never persisted" semantics (`libs/langgraph/langgraph/channels/ephemeral_value.py:15-78`, `libs/langgraph/langgraph/channels/untracked_value.py:15-72`). They are documented as scratch state for routing, not state to be checkpointed.
- **`DeltaChannel` (beta)** stores only a sentinel per non-snapshot step and reconstructs state from ancestor writes — saves blob size at the cost of a multi-stage ancestor walk on read (`libs/langgraph/langgraph/channels/delta.py:25-203`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`).
- **`ShallowPostgresSaver` is deprecated** in favor of `durability="exit"`, which preserves the "head-only" semantics on a full-history-capable saver (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197`).
- **Conformance tests are a separate package.** `langgraph-checkpoint-conformance` provides a registry + test spec that any custom saver can opt into via `@checkpointer_test(name=...)` (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/initializer.py:59-99`). This is the official mechanism for telling operators "this saver matches LangGraph's contract."
- **Store is decoupled from checkpointer.** `BaseStore` and `BaseCheckpointSaver` are independent contracts. A graph can have one without the other; persistence and long-term memory are not coupled (`libs/langgraph/langgraph/types.py:98-117`).
- **CLI schema exposes TTL but the base checkpointer doesn't implement thread-level TTL.** `ThreadTTLConfig` exists (`libs/cli/langgraph_cli/schemas.py:109-124`) but the Python `BaseCheckpointSaver` has no `sweep_ttl` equivalent — only the *store* adapters do. The CLI/server is responsible for enforcing thread-level TTL on top of the checkpointer.

## Notable Patterns

- **Channel-level no-op persistence.** `EphemeralValue.checkpoint()` and `UntrackedValue.checkpoint()` return MISSING, and `put_writes` filters them out before submission (`libs/langgraph/langgraph/pregel/_loop.py:432-447`). The graph compiles even if a developer accidentally wires a real reducer into an `UntrackedValue`; the data is dropped silently on save but the data path itself is correct in-process.
- **Delta-channel staged reconstruction.** The two-stage query for `get_delta_channel_history` on Postgres is benchmarked against an alternative JSONB-shipping design and the per-channel UNION ALL wins by ~3x end-to-end (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:186-198`). The same shape is mirrored on SQLite (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py:1-100`).
- **Lazy stub parent for first-run exit-mode delta writes.** When `durability="exit"` runs on a thread with no prior checkpoint, the loop creates a synthetic empty checkpoint and anchors all delta writes to it before persisting the real exit checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:1234-1292`).
- **Pipeline mode for Postgres.** When the libpq connection supports it, `put`/`put_writes` go through `conn.pipeline()` to batch statements; otherwise the saver falls back to a transaction context manager (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:413-442`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:363-403`). The fallback is automatic and silent.
- **Silent Redis degradation.** The cache wrapper treats Redis as a best-effort accelerator: any read or write exception is caught and returns an empty dict / `pass` (`libs/checkpoint/langgraph/cache/redis/__init__.py:62-66, 104-108`).
- **TTL sweeper background thread.** Both `PostgresStore` and `SqliteStore` ship `start_ttl_sweeper()` / `stop_ttl_sweeper()`; the sweeper is opt-in — if you don't call `start_ttl_sweeper()`, expired items stay in the table until manually swept (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:807-908`).
- **Versioned, monotonic checkpoint IDs.** Checkpoint IDs are `uuid6(clock_seq=step)` strings — monotonic, sortable, comparable (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:819, 854`). This is what makes `ORDER BY checkpoint_id DESC` give chronological order without a timestamp index.
- **Channel versions as monotonic `<int>.<random>` strings.** `get_next_version` returns `f"{next_v:032}.{next_h:016}"` so that comparison remains lexicographic + numeric (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:619-628`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:543-552`).
- **Strict msgpack allowlist via env var.** `LANGGRAPH_STRICT_MSGPACK=true` flips the default allowlist from permissive (`True`) to strict (`None`) (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:97-120`).
- **Encrypted serializer wrapper.** `EncryptedSerializer` wraps any inner serde and adds AES (via `pycryptodome`) — operators can layer encryption on top of any backend (`libs/checkpoint/langgraph/checkpoint/serde/encrypted.py`, see `libs/langgraph/tests/conftest_checkpointer.py:66-77`).

## Tradeoffs

- **`async` durability trades crash-time data loss for throughput.** The superstep submits its checkpoint write to the executor and proceeds; the next superstep may commit before the previous write completes. If the process is killed mid-flight, the *last committed superstep* is durable and the rest is lost. The `_delta_write_futs`/`_error_handler_write_futs` chains at least guarantee in-order delivery within a thread (`libs/langgraph/langgraph/pregel/_loop.py:488-498, 1515-1524, 1769-1778`).
- **`exit` durability trades time-travel for write amortization.** Only the final checkpoint is persisted, so history (and `get_state`'s "previous" pointer) is collapsed to a single point (`libs/langgraph/langgraph/pregel/_loop.py:1301-1311`).
- **`sync` durability trades latency for point-in-time durability.** Each step's checkpoint is fully durable before the next step starts. Useful for "exactly-once visible" semantics, costly for long-running graphs.
- **SQLite vs Postgres.** SQLite is single-writer and serializes via `threading.Lock` / `asyncio.Lock` (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:95, 168-189`). Postgres accepts concurrent writers and uses `INSERT ... ON CONFLICT` upserts keyed on `(thread_id, checkpoint_ns, checkpoint_id)` for last-write-wins on head (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:137-159`).
- **`InMemorySaver` vs `MemorySaver`.** The same class; `MemorySaver = InMemorySaver` alias for back-compat (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:631`).
- **DeltaChannel tradeoff.** Reduces blob size on common-case append-mostly channels (e.g., message lists) but adds two queries per read and a `get_delta_channel_history` override on every backend (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`). The snapshot frequency knob is per-channel and bounded by the global `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:78-86`).
- **`ShallowPostgresSaver` keeps `durability="exit"` semantics on the cheap.** It can be tempting to adopt it for single-shot workflows, but the deprecation warning (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197`) pushes users toward `durability="exit"` + full `PostgresSaver`.
- **`Prune` vs `delete_thread`.** `prune` is `keep_latest` by default; a naive `keep_latest` that drops ancestor writes will silently break DeltaChannel reconstruction (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`).
- **Conformance suite is opt-in, not mandatory.** The library exists; CI on the in-tree savers runs it; custom savers in third-party projects must register their own test (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/initializer.py:59-99`).

## Failure Modes / Edge Cases

- **Mid-run SIGKILL on `durability="async"`** — last async write future may not have hit the DB; the next restart will load the last fully-durable checkpoint. This is documented behavior of "async" durability.
- **`SqliteSaver` opened with `":memory:"`** then re-opened with the same connection string — produces a *different* in-memory DB; no persistence between sessions. Same applies to `AsyncSqliteSaver` with `":memory:"`. Tests rely on this (`libs/langgraph/tests/conftest_checkpointer.py:60-78`).
- **Postgres pipeline support** depends on `psycopg.Capabilities().has_pipeline()`. Older versions of `psycopg` will silently fall back to a transaction context manager — there is no operator-visible warning (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:426-439`).
- **Stale checkpoints after upgrade.** The schema is forward-compatible via migrations, but a *downgrade* may leave rows that newer code cannot read. The `_migrate_checkpoint` hook on `PregelLoop` is the in-process migration path (`libs/langgraph/langgraph/pregel/_loop.py:286, 1641-1642`).
- **`EphemeralValue`/`UntrackedValue` accidentally used as state.** No runtime error; the data is dropped silently. Mitigation: review channel types before promoting a graph to production.
- **Redis cache outage** looks like 100% cache miss. If a downstream consumer relies on cached hits (e.g., LLM prompt caching), it will silently degrade latency, not raise.
- **`put_writes` failure on `InMemorySaver`** does not raise (the dict update is best-effort). On a real backend, an exception from `put`/`put_writes` propagates and the loop surfaces it via `on_chain_error` (`libs/langgraph/langgraph/pregel/main.py:3032-3036`).
- **`delete_thread` on Postgres** does a 3-table cascade but does NOT trigger `prune` — it deletes everything for the thread (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`). Operators expecting "keep_latest" behavior need to call `prune` instead.
- **ShallowPostgresSaver is mid-run unsafe with `durability="async"` or `"sync"`**: it deletes sibling writes on every put (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:420-430`). Only `durability="exit"` is safe in the sense that no other superstep's writes get clobbered between exit and the next put.
- **`PostgresStore.start_ttl_sweeper` not called** → expired items stay forever. There is no auto-spawn (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:807-879`).
- **No evidence found** of automatic compaction / vacuum of orphaned `checkpoint_writes` rows after `delete_for_runs` — the `prune` docstring (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`) is the only place this is mentioned.

## Future Considerations

- **TTL on the checkpointer itself.** `ThreadTTLConfig` is declared in `libs/cli/langgraph_cli/schemas.py:109-124` but the `BaseCheckpointSaver` Python API has no `sweep_ttl`. If/when that ships, `delete_for_runs` (already declared in the base class, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-348`) becomes the natural building block.
- **`ShallowPostgresSaver` removal.** Docstring says "will be removed in 3.0.0" (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:193`). Operators depending on it should migrate to `PostgresSaver` + `durability="exit"`.
- **Stable `DeltaChannel`.** Marked `!!! warning "Beta"` in `libs/langgraph/langgraph/channels/delta.py:29-37` and `libs/checkpoint/langgraph/checkpoint/base/__init__.py:64-86`. Once stable, the `_DeltaSnapshot` blob format and `counters_since_delta_snapshot` metadata field will be locked.
- **`langgraph-sdk-js` parity.** The `libs/sdk-js` package is empty in this tree (only a marker directory), so the JS-side persistence story is not part of this study. The JS checkpointer implementations (Postgres, SQLite) live in a separate repository.
- **Encrypted serializer hardening.** `EncryptedSerializer` requires `pycryptodome`; if not installed, import-time failure. Operators with strict-allowlist deployments must include it.
- **Conformance suite as a gate.** Today, conformance is opt-in. If a strict production deployment wants to guarantee no regression, it would wire `validate()` into CI.

## Questions / Gaps

- **What is the exact behavior of `durability="exit"` if the graph process is killed by SIGKILL before `_suppress_interrupt` runs?** The hook is installed via `self.stack.push(self._suppress_interrupt)` (`libs/langgraph/langgraph/pregel/_loop.py:1674, 1925`). It runs on `__exit__`, which only executes on normal scope exit or uncaught exception. A SIGKILL bypasses Python entirely — the in-flight checkpoint future is lost. No evidence found of an atexit handler that flushes pending checkpoint futures.
- **Is there a programmatic "force-checkpoint" hook for `durability="exit"` users who want explicit mid-run durability?** No public API found. The `_put_exit_delta_writes` helper is private (`libs/langgraph/langgraph/pregel/_loop.py:1234-1292`).
- **What is the storage format of `pickle_fallback=True`?** No clear evidence found. `JsonPlusSerializer.__init__` accepts `pickle_fallback: bool = False` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:97-120`) but the actual fallback path was not located in this study's search boundary.
- **Are there official benchmarks comparing `InMemorySaver` vs `PostgresSaver` throughput under `durability="async"`?** No evidence found in this source tree.
- **Does `langgraph-cli` validate that a custom checkpointer path is importable and yields a `BaseCheckpointSaver`?** The CLI schema accepts `CheckpointerConfig.path` as a string (`libs/cli/langgraph_cli/schemas.py:199-217`) but the validation appears to happen server-side, not in the CLI. No clear evidence found in this tree.
- **Cross-source concern (out of scope, flagged only):** the `langgraph-sdk-js` directory is empty in this checkout; the JS persistence story is not analyzed here.

---

Generated by `02.05-persistence-durability-tiers.md` against `langgraph`.