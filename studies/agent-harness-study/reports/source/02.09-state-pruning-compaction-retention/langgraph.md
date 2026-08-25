# Source Analysis: langgraph

## State Pruning, Compaction, and Retention

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/checkpoint`, `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/checkpoint-conformance`, `libs/langgraph`, `libs/prebuilt`); psycopg + aiosqlite + (optional) redis |
| Analyzed | 2026-07-11 |

## Summary

LangGraph does **not** ship a generic in-process message-compaction pipeline. State growth is bounded through three orthogonal mechanisms, each owned by a different layer:

1. **Durability modes** in the pregel loop decide *how often* a checkpoint is written — `"sync"`, `"async"`, `"exit"` (`libs/langgraph/langgraph/types.py:87-93`). `durability="exit"` skips intermediate super-step checkpoints and writes only the final one, materially reducing per-invocation checkpoint fan-out (`libs/langgraph/langgraph/pregel/_loop.py:1116-1118`, `libs/langgraph/langgraph/pregel/_loop.py:1665`, `libs/langgraph/langgraph/pregel/main.py:2740-2750`).
2. **`DeltaChannel`** in the core channel layer (`libs/langgraph/langgraph/channels/delta.py:25-204`) is the canonical compaction primitive: a `DeltaChannel` stores only a sentinel between snapshots and reconstructs state by replaying the reducer over `checkpoint_writes`. Snapshot cadence is bounded by `snapshot_frequency` (default `1000`, per-channel) and a system-wide `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (default `5000`, env `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`, `libs/langgraph/langgraph/_internal/_config.py:33-35`). Replay is implemented in `BaseCheckpointSaver.get_delta_channel_history` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`) with per-backend fast paths (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-477`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py`).
3. **Checkpoint retention** in the persistence layer is exposed via three primitives on `BaseCheckpointSaver`: `delete_thread` (mandatory, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:320-329`), `delete_for_runs` (optional, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-348`), and `prune(thread_ids, strategy="keep_latest"|"delete")` (optional, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`). **None** of the built-in checkpointers (`InMemorySaver`, `PostgresSaver`, `AsyncPostgresSaver`, `SqliteSaver`, `AsyncSqliteSaver`, `ShallowPostgresSaver`) override `prune` — they inherit the `NotImplementedError` base. There is no global TTL or per-thread TTL on checkpoints.

In parallel, the **store** layer (`libs/checkpoint/langgraph/store/base/__init__.py:545-720`, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:728-893`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1046-1146`) implements per-item TTL with an opt-in background sweeper (`sweep_interval_minutes`, default `5`). `InMemoryStore` (`libs/checkpoint/langgraph/store/memory/__init__.py`) does **not** implement TTL — the base class default `supports_ttl=False` (`libs/checkpoint/langgraph/store/base/__init__.py:719`) applies.

User-driven deletion exists in two forms: `RemoveMessage` in the message reducer (`libs/langgraph/langgraph/graph/message.py:209-234`) and `Overwrite` for `BinaryOperatorAggregate` channels (`libs/langgraph/langgraph/channels/binop.py:31-37`, `libs/langgraph/langgraph/types.py:937-969`). Both are application-level — the framework never deletes state without an explicit signal from the node.

Compaction preserves replay: `DeltaChannel`'s `_DeltaSnapshot(value)` blob lives in `channel_values` (`libs/langgraph/langgraph/pregel/_checkpoint.py:108`), and `get_delta_channel_history` walks the parent chain for any ancestor without a snapshot (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`). However, naive `prune(strategy="keep_latest")` will sever the DeltaChannel chain — the docstring at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413` explicitly warns implementers that the kept checkpoint is "rarely a snapshot point" and the delta channels would "silently reconstruct as empty." The conformance suite for `prune` exists (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:1-217`) but it exercises a contract that no built-in saver fulfills.

## Rating

**6 / 10 — Present, multi-layered, well-documented, but uneven: checkpoint retention is opt-in and unimplemented in built-in savers, and there is no automatic history size bound.**

Rationale:
- (+) `Durability` is a first-class, public configuration knob on every public entry point (`libs/langgraph/langgraph/pregel/main.py:2720-2726`, `libs/langgraph/langgraph/types.py:87-93`); `"exit"` mode is explicitly designed to reduce checkpoint fan-out.
- (+) `DeltaChannel` is a real compaction primitive with explicit bounds (`snapshot_frequency`, `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`), a dedicated two-stage SQL walk (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-477`), and dedicated tests (`libs/langgraph/tests/test_delta_channel_supersteps_bound.py`, `libs/langgraph/tests/test_delta_channel_exit_mode.py`).
- (+) Store layer ships TTL with a configurable sweeper, refresh-on-read semantics, and a comprehensive test suite (`libs/checkpoint-sqlite/tests/test_ttl.py:1-429`).
- (+) `delete_thread`, `delete_for_runs`, and `prune` are typed abstractions in the base class with concrete SQL implementations (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501`).
- (−) **No built-in checkpointer implements `prune`** — a grep over `libs/checkpoint*` shows zero overrides of `aprune`/`prune`. Operators must implement their own.
- (−) **No TTL on checkpoints** — there is no `expires_at` column in `checkpoints`, `checkpoint_blobs`, or `checkpoint_writes` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:319-339`). Retention is entirely manual via `delete_thread` / `prune`.
- (−) `InMemorySaver` and `InMemoryStore` (the default for tests and the documented "small projects" case) have **no retention at all** — they grow until the process exits (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:511-527`, `libs/checkpoint/langgraph/store/memory/__init__.py:206-234`).
- (−) The `prune` "keep_latest" contract actively breaks `DeltaChannel` replay; the framework acknowledges this in a docstring warning rather than implementing a safe variant (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413`).
- (−) `ShallowPostgresSaver` exists as a "store most recent only" escape hatch but is deprecated as of `2.0.20` and slated for removal in `3.0.0` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:551-556`); the replacement (`durability="exit"`) is less ergonomic and not equivalent for time-travel.
- (−) No metrics/telemetry on checkpoint counts, store sizes, or compaction frequency. Compaction is observable only via log messages (e.g. `libs/checkpoint-postgres/langgraph/store/postgres/aio.py:346`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1102`).
- (−) Conversation summarization is **not** a framework feature — only a docstring pointer via `pre_model_hook` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424`). The user must wire `RemoveMessage` + an LLM call.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| `Durability` literal + docstring | `Literal["sync", "async", "exit"]` with semantically distinct behavior. | `libs/langgraph/langgraph/types.py:87-93` |
| `durability="exit"` skips intermediate checkpoints | `do_checkpoint = ... and (exiting or self.durability != "exit")` | `libs/langgraph/langgraph/pregel/_loop.py:1116-1118` |
| `exit` mode puts writes only at exit | `[] if self.durability == "exit" and self.checkpointer is not None else None` | `libs/langgraph/langgraph/pregel/_loop.py:1665` |
| Deprecated `checkpoint_during` → `durability` mapping | `durability = "async" if checkpoint_during else "exit"` | `libs/langgraph/langgraph/pregel/main.py:2740-2750` |
| `durability` propagated to subgraph configs | `config[CONF][CONFIG_KEY_DURABILITY] = durability_` | `libs/langgraph/langgraph/pregel/main.py:2878-2879` |
| `DeltaChannel` — sentinel-only between snapshots | `checkpoint() -> MISSING`, snapshot decision in `create_checkpoint`. | `libs/langgraph/langgraph/channels/delta.py:195-204` |
| `DeltaChannel` snapshot cadence | `snapshot_frequency` (default `1000`, per-channel). | `libs/langgraph/langgraph/channels/delta.py:74-84` |
| `DeltaChannel` system-wide superstep bound | `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` default `5000`, env-overridable. | `libs/langgraph/langgraph/_internal/_config.py:33-35` |
| `DeltaChannel` snapshot trigger | `updates >= ch.snapshot_frequency or supersteps >= DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`. | `libs/langgraph/langgraph/pregel/_checkpoint.py:52-57` |
| `_DeltaSnapshot(value)` blob in checkpoint | Snapshot is written directly into `channel_values[k] = _DeltaSnapshot(ch.get())`. | `libs/langgraph/langgraph/pregel/_checkpoint.py:108` |
| `get_delta_channel_history` parent-chain walk (default) | Walks `get_tuple` + `parent_config` for all channels in one pass. | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690` |
| Postgres two-stage fast path | Stage 1 paged JSONB key lookups; stage 2 per-channel UNION ALL. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-477` |
| SQLite delta fast path | Stage 1 full-blob paged scan; Python parses `channel_values`. | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/_delta.py` |
| `_messages_delta_reducer` (delta-channel compatible reducer) | Batching-invariant, supports `RemoveMessage` tombstoning, skips full `add_messages` parity. | `libs/langgraph/langgraph/graph/message.py:247-310` |
| `add_messages` — `RemoveMessage` deletion | By-id removal and `REMOVE_ALL_MESSAGES` shortcut. | `libs/langgraph/langgraph/graph/message.py:209-234` |
| `Overwrite` — reducer bypass | `isinstance(value, Overwrite)` short-circuits to direct write. | `libs/langgraph/langgraph/channels/binop.py:31-37` |
| `Overwrite` dataclass | Documented bypass pattern. | `libs/langgraph/langgraph/types.py:937-969` |
| `pre_model_hook` summary guidance | Docstring shows `RemoveMessage(id=REMOVE_ALL_MESSAGES) + new_messages` as the user-supplied summary pattern. | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424` |
| `BaseCheckpointSaver.delete_thread` | Thread-scoped full deletion (mandatory capability). | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:320-329` |
| `BaseCheckpointSaver.delete_for_runs` | Run-scoped deletion (optional capability). | `libs/checkpoint/langgraph/checkpoint/base/__init.py:331-348` |
| `BaseCheckpointSaver.prune` abstract contract | Strategies: `"keep_latest"`, `"delete"`. DeltaChannel warning documented. | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415` |
| DeltaChannel + prune foot-gun warning | "the surviving 'latest' checkpoint is rarely a snapshot point itself, so its delta channels would silently reconstruct as empty." | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413` |
| Postgres `delete_thread` impl | `DELETE FROM checkpoints/checkpoint_blobs/checkpoint_writes WHERE thread_id = %s`. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402` |
| Async Postgres `adelete_thread` impl | Same three `DELETE`s on the async path. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:340-361` |
| SQLite `delete_thread` impl | `DELETE FROM checkpoints` + `DELETE FROM writes WHERE thread_id = ?`. | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501` |
| Async SQLite `adelete_thread` impl | Same two `DELETE`s, async. | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:602-620` |
| `InMemorySaver.delete_thread` | Drops `storage`, `writes`, `blobs` keys for the thread. | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:511-527` |
| `ShallowPostgresSaver` — keeps only the latest, deletes writes for non-kept checkpoints | `DELETE FROM checkpoint_writes WHERE ... AND checkpoint_id NOT IN (current, parent)`. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:420-431` |
| `ShallowPostgresSaver` deprecation warning | `version 2.0.20`, removal in `3.0.0`. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197` |
| `AsyncShallowPostgresSaver` deprecation warning | Same removal timeline. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:551-556` |
| `PostgresSaver` schema — no TTL columns | `checkpoints`, `checkpoint_blobs`, `checkpoint_writes` have no `expires_at`. | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91` |
| `SqliteSaver` / `AsyncSqliteSaver` schema — no TTL columns | Tables defined inline in setup. | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:317-339` |
| Conformance `prune` suite | Eight tests for `aprune`: keep_latest single/multi/namespaces, preserves writes, delete-all, other-threads-untouched, empty/no-op, missing-thread no-op. | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:34-194` |
| Conformance `delete_thread` suite | Tests checkpoints removed, writes removed, all namespaces, other threads preserved, no-op on missing. | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delete_thread.py:18-110` |
| Conformance `delete_for_runs` suite | Per-run deletion; preserves other runs; per-namespace; no-op. | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delete_for_runs.py:37-191` |
| Conformance `copy_thread` suite | Required to carry full parent chain for `DeltaChannel`. | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_copy_thread.py` |
| `BaseCheckpointSaver.copy_thread` DeltaChannel caveat | "the copy must carry the complete parent chain ... back to a `_DeltaSnapshot` ancestor." | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:361-372` |
| Capability detection — `PRUNE` is optional | `EXTENDED_CAPABILITIES = {..., PRUNE, ...}`. | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:30-48` |
| Store `TTLConfig` shape | `refresh_on_read`, `default_ttl`, `sweep_interval_minutes`. | `libs/checkpoint/langgraph/store/base/__init__.py:545-568` |
| `BaseStore.supports_ttl = False` default | Subclasses must opt in. | `libs/checkpoint/langgraph/store/base/__init__.py:715-720` |
| `InMemoryStore` does **not** support TTL | No `ttl_config`, no `sweep_ttl`. | `libs/checkpoint/langgraph/store/memory/__init__.py:176-204` |
| Postgres store TTL migration | `ALTER TABLE store ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE, ttl_minutes INT;` plus partial index. | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:78-89` |
| Postgres store `sweep_ttl` | `DELETE FROM store WHERE expires_at IS NOT NULL AND expires_at < NOW()`. | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:807-821` |
| Postgres store TTL refresh on read | `UPDATE store s SET expires_at = NOW() + (s.ttl_minutes || ' minutes')::interval` in the `GetOp` query. | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:266-285` |
| Postgres async store TTL sweeper | `asyncio.create_task(_sweep_loop)` with cancellable `_ttl_stop_event`. | `libs/checkpoint-postgres/langgraph/store/postgres/aio.py:311-355` |
| Postgres sync store TTL sweeper | `threading.Thread` daemon with `concurrent.futures.Future` cancellation handle. | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:823-879` |
| SQLite store TTL migration | `ALTER TABLE store ADD COLUMN expires_at TIMESTAMP, ttl_minutes REAL` + partial index. | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:56-70` |
| SQLite store `sweep_ttl` | `DELETE FROM store WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP`. | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1046-1060` |
| SQLite store TTL sweeper | Thread daemon, `sweep_interval_minutes` (default `5`). | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1062-1118` |
| SQLite TTL refresh on read | `SET expires_at = DATETIME(CURRENT_TIMESTAMP, '+' || ttl_minutes || ' minutes')`. | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:262-266` |
| `BaseStore.delete` | `batch([PutOp(namespace, str(key), None, ttl=None)])`. | `libs/checkpoint/langgraph/store/base/__init__.py:929-936` |
| `RedisCache` TTL on `set` | `pipe.setex(redis_key, ttl, serialized_value)`. | `libs/checkpoint/langgraph/cache/redis/__init__.py:99-102` |
| `RedisCache.clear(namespaces=None)` deletes all | `pattern = f"{self.prefix}*"`. | `libs/checkpoint/langgraph/cache/redis/__init__.py:114-139` |
| In-memory cache TTL on `set` | `delta = datetime.timedelta(seconds=ttl)`; "expired" entries treated as misses on `get`. | `libs/checkpoint/langgraph/cache/memory/__init__.py:42-44` |
| `_internal._config.DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` env hook | `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT`. | `libs/langgraph/langgraph/_internal/_config.py:33-35` |
| `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` enforcement test | `test_delta_channel_supersteps_bound.py`. | `libs/langgraph/tests/test_delta_channel_supersteps_bound.py` |
| `exit`-mode delta persistence test | `test_delta_channel_exit_mode.py` — verifies `durability="exit"` persists writes without losing data. | `libs/langgraph/tests/test_delta_channel_exit_mode.py` |
| TTL integration tests for SQLite store | `test_ttl_basic`, `test_ttl_refresh`, `test_ttl_sweeper`, `test_ttl_custom_value`, `test_ttl_override_default`, `test_search_with_ttl`, async variants. | `libs/checkpoint-sqlite/tests/test_ttl.py:25-429` |

## Answers to Dimension Questions

1. **What grows forever?**
   - **Checkpoints for any thread that is not explicitly deleted.** `PostgresSaver` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:263-345`) and `SqliteSaver` (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:418-443`) insert a new row in `checkpoints` on every super-step. There is no automatic retention.
   - **`checkpoint_writes` rows.** `put_writes` upserts per `(thread, checkpoint, task, idx)` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:347-379`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:445-482`); they are only deleted by `delete_thread`, `ShallowPostgresSaver`'s per-write DELETE, or `aprune`.
   - **`checkpoint_blobs` rows** in Postgres for non-primitive channel values (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:311-345`). Keys are `(thread, ns, channel, version)`; orphaned blobs accumulate if checkpoints are deleted without cascading.
   - **Message lists inside `messages` state** when no `pre_model_hook` is configured. The default `add_messages` reducer appends by default and only deletes when the node writes `RemoveMessage` (`libs/langgraph/langgraph/graph/message.py:209-234`).
   - **In-memory store contents** with `InMemoryStore` (TTL is opt-in via subclass override; default store has no TTL).

2. **What gets summarized?**
   - **Nothing automatically.** LangGraph does not ship a summarizer. The framework's only guidance is a docstring example: return `{"messages": [RemoveMessage(id=REMOVE_ALL_MESSAGES), *summary_messages]}` from a `pre_model_hook` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424`).
   - **Channel state is "compacted" via `DeltaChannel`** in the storage layer, not summarized in the LLM sense: the snapshot blob is the full state, only the intermediate writes are compacted away (`libs/langgraph/langgraph/channels/delta.py:38-55`).

3. **What gets deleted?**
   - **Per thread**: `delete_thread` / `adelete_thread` removes all three Postgres tables or both SQLite tables (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:340-361`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:602-620`). In-memory equivalent drops three dicts (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:511-527`).
   - **Per run**: `delete_for_runs` / `adelete_for_runs` — abstract base only (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-348`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:522-538`); no built-in implementation.
   - **Per checkpoint set**: `prune` / `aprune` with `strategy="keep_latest"` or `"delete"` — abstract base only (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:560-580`).
   - **TTL-based**: only in the **store** layer (Postgres/SQLite `BaseStore` subclasses), via `sweep_ttl` and the background sweeper (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:807-879`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1046-1118`).
   - **In-graph**: `RemoveMessage(id=...)` and `REMOVE_ALL_MESSAGES` (`libs/langgraph/langgraph/graph/message.py:209-234`); `Overwrite(value=...)` replaces rather than deletes (`libs/langgraph/langgraph/channels/binop.py:31-37`).

4. **Does compaction break replay?**
   - **Storage replay (time-travel): preserved for `DeltaChannel`** — `_DeltaSnapshot` blobs and the parent-chain walk ensure any checkpoint can be rehydrated, provided intermediate ancestors and their `checkpoint_writes` are intact (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-690`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-477`).
   - **`prune(strategy="keep_latest")` breaks `DeltaChannel` replay** if any delta channel sits between the kept checkpoint and the nearest snapshot ancestor. The docstring at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413` documents the failure mode and prescribes three safe options (preserve ancestors, force snapshot, skip pruning for delta threads). The Postgres / SQLite built-in savers implement *neither* — `aprune` falls through to `NotImplementedError`.
   - **`shallow` saver** (deprecated) does not retain history at all — replay is impossible by construction.

5. **Can users request deletion?**
   - **Yes, explicitly**, in three ways:
     - Application code: `checkpointer.delete_thread(thread_id)` / `await checkpointer.adelete_thread(thread_id)` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:320-329`).
     - Application code: `checkpointer.prune(thread_ids, strategy="keep_latest"|"delete")` — only works if the saver overrides the abstract method (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415`).
     - Application code in graph: emit `RemoveMessage(id)` / `REMOVE_ALL_MESSAGES` (`libs/langgraph/langgraph/graph/message.py:38`, `libs/langgraph/langgraph/graph/message.py:209-234`); emit `Overwrite(value=...)` to clear a channel (`libs/langgraph/langgraph/types.py:937-969`).
     - In the store: `store.delete(namespace, key)` (`libs/checkpoint/langgraph/store/base/__init__.py:929-936`); for TTL'd stores, items also vanish on `sweep_ttl` once `expires_at < NOW()` (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:807-821`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1046-1060`).
   - **No GDPR-style "delete everything for user X"** surface. There is no `delete_user` / `delete_assistant_id` helper that walks both checkpoint and store namespaces.

## Architectural Decisions

- **Three independent axes instead of one pruning engine.** Durability (`libs/langgraph/langgraph/types.py:87-93`), `DeltaChannel` snapshot cadence (`libs/langgraph/langgraph/channels/delta.py:74-84`, `libs/langgraph/langgraph/_internal/_config.py:33-35`), and saver-level `delete_thread` / `prune` are orthogonal. The choice of where to place the bound is left to the user.
- **Optional capabilities are first-class.** The conformance suite distinguishes `BASE_CAPABILITIES` (PUT, PUT_WRITES, GET_TUPLE, LIST, DELETE_THREAD) from `EXTENDED_CAPABILITIES` (DELETE_FOR_RUNS, COPY_THREAD, PRUNE, DELTA_CHANNEL_HISTORY) at `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:30-48`. `PRUNE` is optional because implementing it safely with `DeltaChannel` is non-trivial.
- **`shallow` savers are deprecated, not removed.** `ShallowPostgresSaver` was a "keep only the latest" drop-in (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:170-176`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:529-534`); it is deprecated as of `2.0.20` and slated for `3.0.0` removal (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:551-556`). The migration path is `durability="exit"` — which is a different semantic (writes are kept but only checkpointed once).
- **`DeltaChannel`'s `snapshot_frequency` is per-channel.** This lets users configure heavy channels (e.g. message lists) to snapshot more often than trivial ones (`libs/langgraph/langgraph/channels/delta.py:74-84`).
- **The supersteps bound is system-wide, not per-channel.** `DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` (`libs/langgraph/langgraph/_internal/_config.py:33-35`) caps walk depth even for channels that have stopped receiving updates — this is the safety net that prevents unbounded ancestor walks on long-idle threads (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-86`).
- **The store layer ships a TTL but the checkpoint layer does not.** This asymmetry is by design (store items are document-like; checkpoints are replay-critical) but it means a long-lived thread with a `BaseStore` will have its long-term memory auto-expire while its checkpoints live forever — `PostgresSaver`'s `checkpoints` table has no `expires_at` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`).
- **`delete_thread` is a single transaction.** Postgres issues all three DELETEs inside `with self._cursor(pipeline=True) as cur` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`); SQLite issues them inside `with self.cursor() as cur` (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501`). In SQLite there is no explicit transaction wrapper, but the `with` cursor acquires the lock. Partial failures can orphan `checkpoint_blobs`.

## Notable Patterns

- **Sentinel + parent-chain walk** for `DeltaChannel` (`libs/langgraph/langgraph/channels/delta.py:195-204`). Compact in shape, replay-correct by construction; pays two-stage SQL cost on read for cold channels.
- **Two-stage delta reconstruction** in Postgres: stage 1 returns `(version, has-snapshot)` for K channels in one roundtrip with dynamic JSONB key lookups (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:201-243`); stage 2 fetches writes + seed blobs via per-channel UNION ALL (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:246-288`). Comment in `base.py:175-198` documents empirical perf wins vs an earlier monolithic design.
- **TTL refresh-on-read in the same SQL roundtrip as the GET.** `passed_in` CTE merges the lookup and the conditional UPDATE (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:266-285`). No second roundtrip.
- **Pipeline `start_ttl_sweeper` / `stop_ttl_sweeper` API** that returns a Future/Task instead of starting a daemon invisibly (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:823-879`, `libs/checkpoint-postgres/langgraph/store/postgres/aio.py:311-355`). Caller owns lifecycle.
- **Overwrite + reducer hybrid.** `Overwrite(value)` is a write-protocol escape hatch for `BinaryOperatorAggregate` that lets a node replace state without changing reducer semantics (`libs/langgraph/langgraph/channels/binop.py:31-37`, `libs/langgraph/langgraph/types.py:937-969`). `DeltaChannel` also honors it on replay (`libs/langgraph/langgraph/channels/delta.py:151-157`).
- **`REMOVE_ALL_MESSAGES` sentinel** as a faster reset path for `add_messages` — drops the entire merged list instead of tombstoning every id (`libs/langgraph/langgraph/graph/message.py:38`, `libs/langgraph/langgraph/graph/message.py:212-213`).
- **Durability as runtime policy, not storage policy.** `durability="exit"` skips intermediate writes in the pregel loop (`libs/langgraph/langgraph/pregel/_loop.py:1116-1118`) without changing the storage schema. Switching policies does not require a migration.

## Tradeoffs

- **Pragmatic vs principled.** LangGraph exposes retention primitives but ships no default implementation for `prune`. The result is that "what grows forever?" is answered by "everything you do not actively delete." The Postgres / SQLite migrations create indexes on `thread_id` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:81-89`) but no retention index on `ts` — implying the design assumes thread-level deletion, not time-based.
- **Correctness over convenience for `DeltaChannel`.** The framework refuses to ship a `keep_latest` `prune` that would silently corrupt channels. Users who need it must implement it themselves and respect the parent-chain warning (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413`).
- **`shallow` saver was a known compromise.** Marked deprecated in `2.0.20` and slated for `3.0.0` removal (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197`) — i.e., the maintainers decided a "drop history" saver is a poor substitute for `durability="exit"` and policy-driven deletion.
- **TTL on store, not on checkpoint.** Aligns with the semantic that stores are document caches (safe to expire) and checkpoints are state (must be retained until the user decides). But it means a user who wants "auto-forget after 30 days" has to either set `durability="exit"` and run a sweeper over `checkpoints.checkpoint_id < cutoff` themselves, or extend `PostgresSaver`.
- **No automatic compaction summarization.** The framework deliberately stops at "here's how you delete messages" (`libs/langgraph/langgraph/graph/message.py:209-234`) and "here's how you point users to a `pre_model_hook`" (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:396-424`). The actual summarization prompt is application territory. This is consistent with the rest of the framework (e.g. no built-in token budget), but it means the "compaction" word has two different meanings across the store layer (TTL-driven row deletion) and the application layer (LLM-driven summarization), and only the former is automated.

## Failure Modes / Edge Cases

- **`prune` is `NotImplementedError` on every built-in saver.** Calling `PostgresSaver.prune(...)` raises immediately. Operators must either subclass and implement, or schedule `delete_thread` calls externally.
- **`prune(strategy="keep_latest")` on a `DeltaChannel` thread silently loses data.** The docstring calls this out explicitly (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413`); no built-in safeguard prevents it.
- **`delete_thread` does not cascade across checkpoint namespaces.** It deletes all rows for the thread_id across all namespaces (because the WHERE clause has no `checkpoint_ns` filter), but only if the saver issues the right DELETE — the Postgres impl correctly deletes from all three tables for the thread_id (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`); the SQLite impl does the same for both tables (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501`). But the conformance test `test_delete_thread_removes_all_namespaces` (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delete_thread.py:70-86`) verifies this contract for any conforming saver.
- **`delete_for_runs` has no built-in implementation.** A custom saver that advertises this capability (via override detection in `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:74-87`) but does not implement it correctly will corrupt `DeltaChannel` state, per the warning at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:340-348`.
- **TTL refresh-on-read bypasses the original write's timestamp.** `expires_at = NOW() + (ttl_minutes || ' minutes')::interval` (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:273`) extends from read time, not from `updated_at`. A perpetually-read item never expires; an item never read expires on schedule.
- **Sweeper is opt-in.** `start_ttl_sweeper` must be called explicitly; otherwise `sweep_ttl` only runs if the user invokes it (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:807-821`). The docstring at `libs/checkpoint-postgres/langgraph/store/postgres/base.py:711-714` warns: "you must explicitly call `start_ttl_sweeper()` to begin the background thread."
- **Sweeper exceptions are swallowed with `logger.exception`.** A persistent DB error in the sweep loop will keep the thread alive but do nothing; no escalation, no alarm (`libs/checkpoint-postgres/langgraph/store/postgres/aio.py:347-351`, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:864-867`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1099-1106`).
- **`ShallowPostgresSaver`'s writes DELETE is `WHERE checkpoint_id NOT IN (current, parent)`** — concurrent writes that arrive between SELECT and DELETE on a different checkpoint_id would survive. The saver uses a per-instance lock (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:495-525`), but only within the same Python process; multi-process deployments can race.
- **Checkpoint migrations add columns** (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:317-339`) but do not backfill retention columns on existing data, because none exist. There is no migration concern for retention today, but adding TTL to checkpoints later would require a backfill step.
- **No `delete_user` / bulk delete.** The only multi-thread primitive is `prune(thread_ids, ...)`, which is unimplemented. Bulk-delete for compliance (GDPR, etc.) requires the operator to iterate `thread_ids` and call `delete_thread` per-thread.

## Future Considerations

- **TTL on checkpoints.** Adding `expires_at` to the `checkpoints` table and a `start_checkpoint_sweeper` mirror to `BaseCheckpointSaver` would close the largest retention gap. The schema migration would mirror `libs/checkpoint-postgres/langgraph/store/postgres/base.py:78-89`.
- **A safe `prune` for `DeltaChannel` threads.** Either implement the "preserve ancestors back to snapshot" logic in a default helper, or detect at runtime and refuse. The docstring at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-413` already prescribes the algorithm.
- **Default `prune` implementation in built-in savers.** Today the conformance suite is more aspirational than used — no built-in saver passes `test_prune.py` (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:34-194`). A reference implementation in `PostgresSaver` would unblock operators.
- **`durability="exit"` as the de-facto replacement for `ShallowPostgresSaver`.** The deprecation at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-197` is signaling intent. Worth verifying that users can still time-travel to the single checkpoint written by an `"exit"` invocation (they can, because the `Final` step is still a checkpoint — but only the final step).
- **Bulk / namespace-scoped deletion.** A `delete_threads_where(metadata @> ...)` would help compliance workflows. Today the operator must list thread_ids externally.
- **Telemetry on retention events.** `logger.info("Store swept %d expired items", n)` (`libs/checkpoint-postgres/langgraph/store/postgres/aio.py:346`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:1102`) is the only signal. Hooks into LangSmith or callback handlers would make this observable.
- **Default `compaction_strategy` for `add_messages`.** The current behavior is "append unless RemoveMessage is explicit." A built-in summarization hook (e.g. `add_messages(summary_fn=...)` modeled on `pre_model_hook`) would close the "what gets summarized?" gap.

## Questions / Gaps

- **Is the conformance suite's `prune` test suite (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_prune.py:34-194`) ever exercised by a built-in saver?** No grep result for `aprune`/`prune` overrides in `libs/checkpoint*`. The suite looks aspirational — its tests would fail on `PostgresSaver` / `SqliteSaver` today.
- **What is the operator-facing contract for "remove all checkpoints older than N days"?** No clear answer in the framework — must be done via custom SQL on `checkpoints` directly or via per-thread enumeration + `delete_thread`. No environment variable, no config key.
- **Is `prune` documented in the user-facing README?** `README.md` does not mention `prune`; `libs/checkpoint/README.md` documents `delete_thread` (`libs/checkpoint/README.md:64`). `prune` is base-class only.
- **What about Redis as a checkpoint backend?** The `cache` layer has a `RedisCache` with TTL (`libs/checkpoint/langgraph/cache/redis/__init__.py:84-102`), but there is no `langgraph-checkpoint-redis` library in the monorepo — only `cache` and `store` redis are present. Confirmed: no `libs/checkpoint-redis` directory exists.
- **Does `InMemoryStore` ever gain TTL?** No — `InMemoryStore` does not import or override `ttl_config` (`libs/checkpoint/langgraph/store/memory/__init__.py:176-204`); `BaseStore.supports_ttl = False` (`libs/checkpoint/langgraph/store/base/__init__.py:715-720`) is the class-level default. Operators wanting TTL'd in-memory storage must subclass and reimplement.
- **Does the `durability="exit"` mode affect replay?** Replay from the final exit-checkpoint works for plain channels. For `DeltaChannel` it requires the parent chain to be intact up to a snapshot — the test at `libs/langgraph/tests/test_delta_channel_exit_mode.py:1-320` covers this.

---

Generated by `02.09-state-pruning-compaction-and-retention` against `langgraph`.