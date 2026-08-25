# Source Analysis: langgraph

## Schema Versioning and Migration

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python 3.11+ (asyncio, TypedDict, msgpack via `ormsgpack`, dataclasses, pydantic v1/v2), Postgres / SQLite / in-memory persistence, JS/TS SDK under `libs/sdk-js` |
| Analyzed | 2026-07-10 |

## Summary

LangGraph embeds a deliberate, integer-keyed `Checkpoint` schema version (`v: int`) at the root of every persisted graph checkpoint, declares the current version as a `LATEST_VERSION` constant in two places (base layer `LATEST_VERSION = 2` at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:811`, Pregel layer `LATEST_VERSION = 4` at `libs/langgraph/langgraph/pregel/_checkpoint.py:21`), and ships **runtime, in-place migrations** that are invoked before any thread is resumed. The migrations are partitioned into two layers:

1. **Checkpoint-content migrations** (Pregel scope) — rename channels (`start:*`, `branch:source:cond:node` → `branch:to:node`) in `StateGraph._migrate_checkpoint` (`libs/langgraph/langgraph/graph/state.py:1612-1690`), relocate pending sends into the `TASKS` channel in `Pregel._migrate_checkpoint` (`libs/langgraph/langgraph/pregel/main.py:1135-1142`), and pick a per-task-id hash function based on `checkpoint["v"] > 1` in `libs/langgraph/langgraph/pregel/_algo.py:550,1138`.
2. **Persistence-format migrations** — Postgres declares an ordered `MIGRATIONS` list with a `checkpoint_migrations` table (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:39-91`) and runs them inside `setup()` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85-110`); SQLite has **no equivalent migration table** and relies on `CREATE TABLE IF NOT EXISTS` plus `IF NOT EXISTS` `ALTER`s (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:129-166`). A second on-load migration patches pre-v4 `pending_sends` blobs into `TASKS` channel values inside `PostgresSaver.list/get_tuple` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:164-188, 246-259`) and the async equivalents (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:153-173, 215-224`).

A separate, beta, schema-evolution path exists for `DeltaChannel`: it accepts three on-disk blob shapes (`MISSING`, `_DeltaSnapshot`, plain value) at `libs/langgraph/langgraph/channels/delta.py:118-137` so that pre-`DeltaChannel` threads written with `BinaryOperatorAggregate` keep working unchanged — covered by `libs/langgraph/tests/test_delta_channel_migration.py`.

Wire-format compatibility is layered on the `JsonPlusSerializer` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py`): the old `"json"` lc=2 envelope path is preserved with a "safe types" escape hatch (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:60-70, 223-256`) that lets pre-`v1.0.1` checkpoints hydrate without configuration, and an explicit allow-list (`allowed_json_modules`, `LANGGRAPH_STRICT_MSGPACK`) gates anything outside that set.

Old-state fixtures are **stored as in-source `CheckpointTuple` literals** (`libs/langgraph/tests/test_checkpoint_migration.py:272-1397`) and exercised by `test_saved_checkpoint_state_graph` / `test_saved_checkpoint_state_graph_async` / `test_migrate_checkpoints`. A dedicated migration test fixture — `MemorySaverNeedsPendingSendsMigration` in `libs/langgraph/tests/memory_assert.py:29-48` — downgrades v4 → v3 on the read path so the v<4 migration runs against every parameterized checkpointer in `libs/langgraph/tests/conftest.py:144-188`.

## Rating

**7 / 10** — LangGraph has explicit `LATEST_VERSION` constants, a layered `v`-keyed format, real in-place migrations triggered before the loop touches the checkpoint, and an old-state fixture suite that round-trips stored snapshots through the migration function. Two `LATEST_VERSION` declarations are out of sync (2 in base, 4 in Pregel) and the sqlite checkpointer has no `MIGRATIONS` system at all — Postgres-style schema changes do not flow to SQLite. The `pending_sends` migration is duplicated in two backends (Pregel and Postgres) and absent in SQLite — relying on Pregel to migrate on every load. There is no central registry of "which versions we have ever shipped" and the old-state fixtures are committed as Python literals rather than a documented external corpus.

## Evidence Collected

Every entry includes a file path with line numbers.

### Schema version field

| Area | Evidence | File:Line |
|------|----------|-----------|
| Root `Checkpoint` TypedDict declares `v: int` ("The version of the checkpoint format") | `Checkpoint` definition | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-96` |
| Stale docstring still says `Currently 1` even though `LATEST_VERSION = 2` | Checkpoint docstring | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:95-96` |
| Base layer `LATEST_VERSION = 2` constant | `LATEST_VERSION` definition | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:811` |
| Pregel layer `LATEST_VERSION = 4` constant (separately declared, not imported from base) | `LATEST_VERSION` definition | `libs/langgraph/langgraph/pregel/_checkpoint.py:21` |
| `empty_checkpoint()` writes `v=LATEST_VERSION` (base layer) | `empty_checkpoint` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:814-826` |
| `create_checkpoint` propagates `v=LATEST_VERSION` (base layer) | `create_checkpoint` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:829-859` |
| `empty_checkpoint()` / `create_checkpoint` write `v=LATEST_VERSION` (Pregel layer) | factory functions | `libs/langgraph/langgraph/pregel/_checkpoint.py:26-34, 113-121` |
| `copy_checkpoint` preserves `checkpoint["v"]` verbatim (does NOT upgrade) | `copy_checkpoint` | `libs/langgraph/langgraph/pregel/_checkpoint.py:229-238`; `libs/checkpoint/langgraph/checkpoint/base/__init__.py:126-136` |
| Task-id hashing function is gated on `checkpoint["v"] > 1` (pre-v2 used uuid5; v2+ uses xxh3) | task-id dispatch | `libs/langgraph/langgraph/pregel/_algo.py:550, 1138` |
| uuids used to be `_uuid5_str`; v2+ uses `_xxhash_str` (xxh3_128) | hash functions | `libs/langgraph/langgraph/pregel/_algo.py:1395-1409` |

### Migration entry points

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pregel-level migration: `v < 4` lifts `pending_sends` into `channel_values[TASKS]` and bumps the channel version | `Pregel._migrate_checkpoint` | `libs/langgraph/langgraph/pregel/main.py:1135-1142` |
| StateGraph-level migration: short-circuits when `v >= 3`; rewrites `start:<node>` and `branch:<src>:<cond>:<node>` channels into `branch:to:<node>` | `StateGraph._migrate_checkpoint` | `libs/langgraph/langgraph/graph/state.py:1612-1690` (with the `>= 3` gate at `:1625`) |
| `_migrate_checkpoint` is wired through the `PregelLoop` constructor and called after every `checkpointer.get_tuple` (`_first()` and `_prepare_next_tasks()` paths) | wiring | `libs/langgraph/langgraph/pregel/_loop.py:196, 286, 309, 1466, 1486, 1639-1642, 1718, 1738, 1896-1899`; `libs/langgraph/langgraph/pregel/main.py:1164, 1287, 1646, 2119, 2931, 3385` |
| InMemory pre-load transform used by the test suite to force the v<4 migration to fire on every checkpoint read | `MemorySaverNeedsPendingSendsMigration.get_tuple` | `libs/langgraph/tests/memory_assert.py:29-48` |
| Test fixture exposes the migration-via-read path as a parametrized checkpointer | `_checkpointer_memory_migrate_sends` | `libs/langgraph/tests/conftest_checkpointer.py:52-56`; `libs/langgraph/tests/conftest.py:154, 169-170` |

### Database-schema migrations (Postgres)

| Area | Evidence | File:Line |
|------|----------|-----------|
| `MIGRATIONS` is an ordered list of SQL statements; the position in the list is the version | `MIGRATIONS` docstring + list | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:39-91` |
| `setup()` reads the last `v` from `checkpoint_migrations`, iterates `range(version+1, len(self.MIGRATIONS))`, and inserts `(v,)` after each statement | migration runner | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85-110` |
| Async variant runs the same migration runner | async setup | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:96-112` |
| `ShallowPostgresSaver` reuses the same `MIGRATIONS` list (and prints a deprecation warning since 2.0.20) | reuse | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:37-92, 178, 192-197` |
| PostgresSaver adds an on-load migration: `value["checkpoint"]["v"] < 4` triggers `_migrate_pending_sends` (sync + async, list + get_tuple) | on-load migration | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:164-188, 246-259`; `libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:153-173, 215-224` |
| `_migrate_pending_sends` decodes `(type, blob)` rows into Python, writes a single `TASKS` blob, and bumps the channel version | helper | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:308-326` |
| Cross-package version check: warns `DeprecationWarning` if `langgraph` major.minor < `0.5` | runtime guard | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:27-37` |

### Database-schema migrations (SQLite)

| Area | Evidence | File:Line |
|------|----------|-----------|
| SQLite has **no** `MIGRATIONS` list / migration table — only `CREATE TABLE IF NOT EXISTS` for `checkpoints` and `writes` | sqlite setup | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:129-166` |
| Async SQLite setup repeats the same single-shot `CREATE TABLE IF NOT EXISTS` (no migration history) | async setup | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:319-329` |
| SQLite has **no** on-load migration of pre-v4 `pending_sends` (relies on Pregel's `_migrate_checkpoint`) | absence | searched `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/`: no `v < 4` / `pending_sends` migration branch |
| SQLite cache backend uses a single `CREATE TABLE IF NOT EXISTS cache` with no version field | absence | `libs/checkpoint-sqlite/langgraph/cache/sqlite/__init__.py:35` |

### Serializer (wire-format) versioning

| Area | Evidence | File:Line |
|------|----------|-----------|
| `JsonPlusSerializer.dumps_typed` returns one of `{"null", "bytes", "bytearray", "msgpack", "pickle"}` — the type tag is the format version | typed dumps | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:258-271` |
| `loads_typed` dispatches by that type tag and supports `"json"` for backward compatibility with pre-`v1.0.1` payloads | typed loads | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:273-290` |
| Pre-msgpack (lc=2 JSON) payloads survive without explicit allowlist when type is in `SAFE_MSGPACK_TYPES` ("safe types bypass the `allowed_json_modules` gate so that old 'json' format checkpoints…can be resumed") | safe-type escape hatch | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:60-70, 223-256` |
| `LANGGRAPH_STRICT_MSGPACK=true` switches to "safe types only" and warns on each unregistered revival | env knob | `libs/checkpoint/langgraph/checkpoint/serde/_msgpack.py:12-16`; `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:107-114, 559-609` |
| Built-in safe-type allowlist (datetime, uuid, pathlib, zoneinfo, langchain-core messages, langgraph types) | `SAFE_MSGPACK_TYPES` | `libs/checkpoint/langgraph/checkpoint/serde/_msgpack.py:21-85` |
| `SAFE_MSGPACK_METHODS` only permits `datetime.datetime.fromisoformat` | method allowlist | `libs/checkpoint/langgraph/checkpoint/serde/_msgpack.py:90-94` |
| Pre-`v1.0.1` JSON envelopes that no longer have a default-ctor fallback revive to `None` and emit a `WARNING` so operators see regressions instead of silent dict passthrough | observable fallback | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:208-221` |
| Legacy `method=[None, "construct"]` pydantic envelopes still revive via default constructor (the first entry was always "default") | legacy compat | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:185-191` |
| `EncryptedSerializer.loads_typed` falls through unencrypted `(type, blob)` when `+` is missing — explicit "backwards compat" path | crypto compat | `libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:29-30`; tested at `libs/checkpoint/tests/test_encrypted.py:412` |
| `SerializerCompat` adapts legacy `(dumps/loads) -> bytes` serializers to the typed protocol so old custom savers keep working | protocol compat | `libs/checkpoint/langgraph/checkpoint/serde/base.py:29-48` |
| `with_msgpack_allowlist(...)` returns a new serializer with a merged allowlist; used by frameworks that need to extend safe types | merge API | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:128-158` |
| `_DeltaSnapshot` is a new msgpack ext code (`EXT_DELTA_SNAPSHOT = 7`) — serialized as a tagged ext tuple, deserialized by the ext hook | new ext type | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:302, 634-639`; `libs/checkpoint/langgraph/checkpoint/serde/types.py:19-31` |

### DeltaChannel as a forward-compatible reducer migration

| Area | Evidence | File:Line |
|------|----------|-----------|
| `DeltaChannel.from_checkpoint` accepts three blob shapes: `MISSING` (replay), `_DeltaSnapshot(value)` (restore from snapshot), or a plain value (legacy `BinaryOperatorAggregate` blob). Comment: "plain value (migration from old `BinaryOperatorAggregate` blobs): use directly." | three-shape loader | `libs/langgraph/langgraph/channels/delta.py:118-137` |
| `get_delta_channel_history` returns a `DeltaChannelHistory` with optional `seed` key (omission ≡ "start empty") so callers handle "no pre-delta ancestor found" without exceptions | seed semantics | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:149-184` |
| In-memory saver's optimized `get_delta_channel_history` walks the parent chain once per thread, terminating on either a non-empty `channel_values` entry or a `_DeltaSnapshot` | walk logic | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:142-220` |
| Test asserts that a `BinaryOperatorAggregate` thread keeps working after the annotation is swapped to `DeltaChannel` on the same checkpointer (sync + async + tip-hydration + add_messages + cross-thread isolation) | migration test | `libs/langgraph/tests/test_delta_channel_migration.py:1-44, 137-170, 193-220, 222-289, 291-330, 370-450, 491-560` |
| `DeltaChannel` carries a "Beta" docstring flag acknowledging that "the API and on-disk representation may change in future releases" | beta contract | `libs/langgraph/langgraph/channels/delta.py:29-36` |

### Stored old-state fixtures

| Area | Evidence | File:Line |
|------|----------|-----------|
| Three sets of historical `CheckpointTuple` snapshots are committed in-source: `"3"` (post-v2), `"2-start:*"` (pre-branch:to migration), `"2-quadratic"` (legacy v1 layout) | `SAVED_CHECKPOINTS` | `libs/langgraph/tests/test_checkpoint_migration.py:272-273, 648, 1034` |
| `"2-quadratic"` snapshots literally carry `"v": 1` and old `start:<node>` channel keys — i.e., the in-source fixtures encode prior format versions | v=1 fixture | `libs/langgraph/tests/test_checkpoint_migration.py:1043-1100` |
| `test_saved_checkpoint_state_graph` (sync) and `test_saved_checkpoint_state_graph_async` parametrize on those three keys, write them through the checkpointer, then read history and resume | parametrized round-trip | `libs/langgraph/tests/test_checkpoint_migration.py:1599-1702` |
| `test_migrate_checkpoints` calls `graph._migrate_checkpoint` on each `"2-…"` snapshot and compares against the `"3"` expected output | unit migration test | `libs/langgraph/tests/test_checkpoint_migration.py:1473-1494` |
| Pre-msgpack lc=2 JSON envelopes are revived without explicit allowlist (regression for #7498) | backward-compat serde test | `libs/checkpoint/tests/test_jsonplus.py:336-369` |
| `test_lc2_json_legacy_pydantic_method_list_falls_back_to_default` proves legacy `method=[None, "construct"]` envelopes still round-trip through the default constructor | backward-compat serde test | `libs/checkpoint/tests/test_jsonplus.py:472-495` |
| `test_lc2_json_legacy_construct_payload_logs_warning_when_default_init_rejects` makes a silent regression observable via a `WARNING` | observability test | `libs/checkpoint/tests/test_jsonplus.py:497-545` |
| `EncryptedSerializer` "handles unencrypted data for backwards compat" | crypto compat test | `libs/checkpoint/tests/test_encrypted.py:412` |

### Documentation

| Area | Evidence | File:Line |
|------|----------|-----------|
| README shows a `"v": 4` checkpoint with `channel_values`, `channel_versions`, `versions_seen` — the documented public surface | published sample | `libs/checkpoint/README.md:79`; `libs/checkpoint-postgres/README.md:67, 108`; `libs/checkpoint-sqlite/README.md:40, 81` |
| `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:39-42` docstring explicitly says: "To add a new migration, add a new string to the MIGRATIONS list. The position of the migration in the list is the version number." | migration author guide | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:39-42` |
| `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:37-40` repeats the same MIGRATIONS-index-is-version contract | reuse note | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:37-40` |

## Answers to Dimension Questions

### 1. Does persisted state carry a version?

Yes. Every checkpoint has a top-level integer `v` declared on the `Checkpoint` TypedDict (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-96`) and emitted by `empty_checkpoint()` / `create_checkpoint()` at both base and Pregel layers (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:811-859`; `libs/langgraph/langgraph/pregel/_checkpoint.py:21-121`). The serializer tag (`"msgpack"`, `"json"`, `"pickle"`, `"null"`, `"bytes"`, `"bytearray"` returned by `dumps_typed` at `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:258-271`) is the wire-format version. The Postgres schema has its own version (the `v` row in `checkpoint_migrations`, see `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-46`).

### 2. What happens when old state is loaded?

Three independent migrations run on the read path:

- **Pregel checkpoint migration** (`Pregel._migrate_checkpoint`, `libs/langgraph/langgraph/pregel/main.py:1135-1142`): if `v < 4` and `pending_sends` is present, the pending sends are moved into `channel_values[TASKS]` and the channel version is bumped to `max(channel_versions.values())`.
- **StateGraph channel-naming migration** (`StateGraph._migrate_checkpoint`, `libs/langgraph/langgraph/graph/state.py:1612-1690`): if `v < 3`, keys are rewritten from `start:<node>` and `branch:<src>:<cond>:<node>` to `branch:to:<node>` and the `versions_seen` table is updated to match.
- **PostgresSaver on-load migration** (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:164-188, 246-259`; async variant at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/aio.py:153-173, 215-224`): `if value["checkpoint"]["v"] < 4 and parent_checkpoint_id` triggers `_migrate_pending_sends` to fold the `pending_sends` row into `channel_values` before yielding.
- **Serializer hydration** (`JsonPlusSerializer._revive_lc2` / `_create_msgpack_ext_hook`, `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:181-221, 633-744`): safe types revive without an allowlist, unknown types return raw bytes/None or are blocked depending on `LANGGRAPH_STRICT_MSGPACK`.

The `_migrate_checkpoint` hook is invoked from the Pregel loop after every `checkpointer.get_tuple` (`libs/langgraph/langgraph/pregel/_loop.py:1641-1642, 1898-1899`).

### 3. Are migrations automatic or manual?

Automatic. Users do not run them. `PostgresSaver.setup()` applies DDL migrations (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85-110`); the Pregel loop applies the v<4 + v<3 checkpoint rewrites on the next read (`libs/langgraph/langgraph/pregel/_loop.py:1641-1642, 1898-1899`); the Postgres saver applies the `pending_sends` row-to-channel migration in `list`/`get_tuple` before yielding the tuple (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:164-188, 246-259`). The SQLite saver has no schema-migration runner; it relies on the Pregel-layer migration.

### 4. Are JSON blobs versioned?

Partially. `JsonPlusSerializer.dumps_typed` returns a `(type, bytes)` pair where `type` is one of `{"null", "bytes", "bytearray", "json", "msgpack", "pickle"}` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:258-271`); `loads_typed` switches on that tag (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:273-290`). However, the `v` integer on the `Checkpoint` envelope (the structural schema version) is the only durable cross-codec version marker; the wire tag is per-blob, not per-checkpoint. Pre-`v1.0.1` lc=2 JSON envelopes still revive today through the safe-type escape hatch (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:60-70`; test at `libs/checkpoint/tests/test_jsonplus.py:336-369`).

### 5. Can old checkpoints/events still be read?

Yes. Stored historical fixtures with `v=1` and `v=2` are re-loaded by `test_saved_checkpoint_state_graph` / `test_saved_checkpoint_state_graph_async` and produce an equivalent v4 history (`libs/langgraph/tests/test_checkpoint_migration.py:1599-1702`). Pre-msgpack JSON lc=2 envelopes revive to proper `BaseMessage` instances without any user configuration (`libs/checkpoint/tests/test_jsonplus.py:336-369`). Pre-`DeltaChannel` threads using `BinaryOperatorAggregate` keep working after the annotation is swapped to `DeltaChannel` on the same checkpointer (`libs/langgraph/tests/test_delta_channel_migration.py`). The `pending_sends` migration fixture `MemorySaverNeedsPendingSendsMigration` (`libs/langgraph/tests/memory_assert.py:29-48`) downgrades every v4 checkpoint it sees into a v3-shaped one so the migration runs against every parameterized checkpointer (`libs/langgraph/tests/conftest_checkpointer.py:52-56`; `libs/langgraph/tests/conftest.py:154`).

## Architectural Decisions

- **Two-level version constant.** `LATEST_VERSION` exists in both the base checkpoint library (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:811 = 2`) and the Pregel layer (`libs/langgraph/langgraph/pregel/_checkpoint.py:21 = 4`). The Pregel layer is the source of truth for what new checkpoints are written at, and the base layer's value appears to describe the lower bound needed for `BaseCheckpointSaver`-only operations. This split is undocumented in code and easy to mistake for a typo.
- **In-place, idempotent migration on read.** `_migrate_checkpoint` is called after `checkpointer.get_tuple` but before channels are hydrated (`libs/langgraph/langgraph/pregel/_loop.py:1639-1642, 1896-1899`); the migration rewrites the loaded dict in place and `copy_checkpoint` preserves the original `v` (`libs/langgraph/langgraph/pregel/_checkpoint.py:229-238`). The on-disk format is therefore allowed to lag the in-memory representation indefinitely — `v=1` threads can be loaded by a `LATEST_VERSION=4` runtime.
- **Postgres DDL migrations indexed by list position.** The `MIGRATIONS` array's positional index is the migration version (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:39-42`); `setup()` advances from `last_v + 1` to `len(MIGRATIONS)` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85-110`). Adding a migration in the middle of the list would re-apply prior migrations and is implicitly disallowed by the comment "To add a new migration, add a new string to the MIGRATIONS list".
- **Serializer versioning by `dumps_typed` type tag.** `JsonPlusSerializer` returns one of six tags so a single deserializer can read pre-msgpack `json`, current `msgpack`, opt-in `pickle`, and raw `bytes`/`bytearray` payloads. Tag-driven dispatch (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:273-290`) is the unit of forward/backward compatibility on the wire.
- **Safe-type escape hatch for pre-msgpack payloads.** Pre-`v1.0.1` JSON envelopes for `BaseMessage`, `UUID`, `datetime`, `pathlib.Path`, etc. survive without an explicit allowlist (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:60-70, 223-256`); the allowlist gate is only enforced for non-safe types.
- **`DeltaChannel` is opt-in beta.** `DeltaChannel.from_checkpoint` accepts three blob shapes (`libs/langgraph/langgraph/channels/delta.py:118-137`), which means a thread can be migrated from `BinaryOperatorAggregate` to `DeltaChannel` without rewriting its checkpoints. The class is explicitly marked beta with a "may change in future releases" warning (`libs/langgraph/langgraph/channels/delta.py:29-36`).
- **Migration is duplicated across Postgres + Pregel.** The same v<4 `pending_sends` → `channel_values[TASKS]` rewrite is encoded in `PostgresSaver.list/get_tuple` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:164-188, 246-259`) and in `Pregel._migrate_checkpoint` (`libs/langgraph/langgraph/pregel/main.py:1135-1142`). SQLite checkpoints rely solely on Pregel; Postgres does both, so a thread loaded via the PostgresSaver through `Pregel._prepare_state_snapshot` runs the migration twice in some flows (the second is a no-op because `v >= 4`).

## Notable Patterns

- **In-source `CheckpointTuple` fixtures as a regression corpus.** `SAVED_CHECKPOINTS` (`libs/langgraph/tests/test_checkpoint_migration.py:272-1397`) carries three full state histories — one for the current schema and two for legacy variants — encoded directly in Python. This pattern keeps the "old state" corpus in the same diff as the code that loads it, but it also makes the corpus bulky (~1100 lines) and hard to extend outside the Python test runner.
- **Wire-format safe list as a single frozenset.** `SAFE_MSGPACK_TYPES` (`libs/checkpoint/langgraph/checkpoint/serde/_msgpack.py:21-85`) and `SAFE_MSGPACK_METHODS` (`:90-94`) are literal frozensets — adding a new safe type requires editing the set; there is no plugin mechanism for end users other than `with_msgpack_allowlist` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:128-158`).
- **Observable fallback when a compat path drops payload.** When an old `method=[None, "construct"]` envelope can no longer be revived as a class instance, the reviver logs a `WARNING` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:208-221`) and returns `None`, instead of silently degrading to a raw dict — tested by `test_lc2_json_legacy_construct_payload_logs_warning_when_default_init_rejects` (`libs/checkpoint/tests/test_jsonplus.py:497-545`).
- **`EXT_DELTA_SNAPSHOT` ext code as a tagged union.** The new `_DeltaSnapshot` is encoded with a dedicated msgpack ext type (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:302, 634-639`), so old readers either know how to decode it (current code) or pass the blob through unchanged — this is a forward-compatible discriminator.
- **Migration runner that records per-statement version.** The Postgres `setup()` inserts one row into `checkpoint_migrations` per applied statement (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:108`) — fine-grained enough that a failed statement leaves the table one row short of `len(MIGRATIONS)`, so the next `setup()` will retry exactly that statement.
- **Deduped warnings.** Both the unregistered-type and blocked-type log messages are deduped via a bounded `seen` set with circuit-breaker semantics (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:51-79`) — important because the same unsafe-type payload may be re-deserialized on every checkpoint load.

## Tradeoffs

- **Automatic in-place migration means old state is silently upgraded in memory, never on disk.** Useful for forward progress; risky because re-running a checkpointer against the same DB after a downgrade could re-trigger migrations that were already applied (no idempotency markers per row).
- **Two `LATEST_VERSION` constants** (`base=2`, `Pregel=4`) lets the lower library evolve without forcing Pregel migrations, but the base docstring still says "Currently `1`" (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:95-96`), so users reading the `Checkpoint` definition get a stale hint.
- **No SQLite migration runner.** `CREATE TABLE IF NOT EXISTS` only (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:142-162`). Future DDL changes that need to backfill existing SQLite checkpoints must ship a separate one-shot script; the `MIGRATIONS` machinery in Postgres does not extend to SQLite.
- **Migration duplication across Postgres + Pregel.** Two implementations of the same `pending_sends` rewrite. A future contributor updating one (say, to fix a bug in `_migrate_pending_sends`) could miss the other.
- **Pre-`DeltaChannel` and post-`DeltaChannel` threads share the same blob table.** That is the *feature* of the `DeltaChannel` shape (`libs/langgraph/langgraph/channels/delta.py:118-137`), but it also means a future schema change that splits `_DeltaSnapshot` into a struct with a magic number has to be done without breaking the in-place migration of pre-delta plain-value blobs.
- **`pending_sends` migration is best-effort in `PostgresSaver.list`.** If a checkpoint's `parent_checkpoint_id` is NULL (root checkpoint), the v<4 migration skips it (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:168`), leaving any `pending_sends` field untouched for the caller; in practice this is benign because root checkpoints are unlikely to carry pending sends.
- **`ShallowPostgresSaver` reuses the same `MIGRATIONS` list** (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:178`) but is deprecated since 2.0.20 and slated for removal in 3.0.0 (`:192-197`). Any migration added before then will continue to be applied even after the saver is removed, which is wasted DDL on deployments that never used it.
- **Wire-format `msgpack` ext codes are positional integers (0-7).** Adding a new ext type means reserving a new integer; the existing codes are documented inline (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-302`) but there is no central registry that asserts two ext types will never collide.

## Failure Modes / Edge Cases

- **`v` deserialization mismatch on schema bump.** `copy_checkpoint` preserves `checkpoint["v"]` verbatim (`libs/langgraph/langgraph/pregel/_checkpoint.py:229-238`), so a thread loaded by a `LATEST_VERSION=4` runtime continues to report `v=2` until something explicitly writes a new checkpoint. Downstream code that gates on `v` (e.g., `libs/langgraph/langgraph/pregel/_algo.py:550,1138`) reads the original value, which is correct for replay but can confuse introspection tools that expect `v == LATEST_VERSION`.
- **SQLite is one DDL away from being broken.** A migration that adds a non-null column without a default would fail on existing `checkpoints` / `writes` tables — but the SQLite saver has no `MIGRATIONS` runner, so the author must write a one-shot script. No such script ships with the package today.
- **Pre-v4 threads with a `parent_checkpoint_id` but no `pending_sends` row.** In Postgres, the v<4 migration runs only when both `v < 4` and `parent_checkpoint_id` is set (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:168, 247`); a `v < 4` checkpoint without a parent or without `pending_sends` is returned as-is and then re-migrated by `Pregel._migrate_checkpoint`. The two migrations can disagree on ordering if a future Postgres migration lifts `pending_sends` for a `parent_checkpoint_id IS NULL` row.
- **`LANGGRAPH_STRICT_MSGPACK=true` silently breaks pre-msgpack threads** unless the user explicitly adds the safe types they used to `allowed_msgpack_modules`. The error is `InvalidModuleError` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:223-256`); there is no first-class migration helper for "I have a v0.x thread, how do I read it under strict mode?".
- **`_migrate_pending_sends` decodes via `self.serde.loads_typed` with no allowlist check** (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:317-319`). A row written by a permissive serde and loaded under a strict serde will raise inside the migration — there is no try/except around the decode, so the migration fails the whole `list`/`get_tuple` call.
- **`DeltaChannel` writes are NOT backward-compatible in the strict sense.** Threads written by `DeltaChannel` must be read by code that knows how to call `get_delta_channel_history` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:654-690`). A custom saver that doesn't implement that method will fail to reconstruct delta channels (it returns `MISSING`, then replays nothing). This is a soft compatibility contract, not enforced.
- **Coupling between Postgres on-load migration and Pregel migration.** If `PostgresSaver.get_tuple` migrates `v<4 → v4` in place via `_migrate_pending_sends`, the `Checkpoint` dict returned to the caller still has `v=2` (the SQL helper writes `channel_values[TASKS]` but does not bump `v`). The Pregel loop then re-runs `_migrate_checkpoint`, finds `v < 4` true, and... copies again into a different key. Currently harmless because `_migrate_pending_sends` early-exits on empty `pending_sends`, but a future change to bump `v` in the SQL helper would break the no-op invariant.
- **Stored-checkpoint history fidelity.** The `test_saved_checkpoint_state_graph` parametrization expects that resuming from the second-to-last migrated checkpoint yields the exact same final state as a fresh run (`libs/langgraph/tests/test_checkpoint_migration.py:1643-1661`). If a future migration changes the task-id hash (`_xxhash_str` vs `_uuid5_str` is already gated on `v > 1` at `libs/langgraph/langgraph/pregel/_algo.py:550,1138`), this assertion would need to be re-recorded.

## Future Considerations

- **Promote `LATEST_VERSION` to a single source.** `libs/checkpoint/langgraph/checkpoint/base/__init__.py:811` and `libs/langgraph/langgraph/pregel/_checkpoint.py:21` should both reference one constant (likely the Pregel value, since that is what `empty_checkpoint` actually writes today). The base docstring at `:95-96` should be regenerated when this is done.
- **Port the Postgres `MIGRATIONS` runner to SQLite.** The pattern (`checkpoint_migrations` table, list-position-as-version) is straightforward and would let schema changes ship once for both backends. Today the SQLite saver relies on `CREATE TABLE IF NOT EXISTS` plus `IF NOT EXISTS` `ALTER`s (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:129-166`).
- **Centralize the `pending_sends` migration in Pregel.** The Postgres saver has its own copy (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:164-188, 246-259`) that is a no-op when the same data has already been migrated at the Pregel layer. Removing the Postgres copy and letting Pregel own the rewrite would reduce drift risk.
- **Expose a "schema version registry" / changelog.** The README has a sample `"v": 4` checkpoint but no narrative history of v1 → v2 → v3 → v4. The Postgres docstring (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:39-42`) is the closest thing to a "how to add a migration" guide; there is no equivalent for `v` bumps inside the Checkpoint payload.
- **Wire-format ext-code registry.** `EXT_CONSTRUCTOR_*`, `EXT_PYDANTIC_V1/V2`, `EXT_NUMPY_ARRAY`, `EXT_DELTA_SNAPSHOT` are constants in `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-302`. A central table with name → code, intent, and "added in" version would make future ext codes easier to manage.
- **Round-trip tests for new ext types.** `_DeltaSnapshot` was added without a dedicated back-compat fixture; if a future ext code changes shape, a "decode from v{N-1} snapshot" test (along the lines of `libs/langgraph/tests/test_delta_channel_migration.py`) would catch silent breakage.
- **Idempotent migration markers.** Today the migration rewrites the loaded dict in memory only. A future column `last_migrated_from_version` in `checkpoints` could let a downgrade recover gracefully; today a downgrade would simply not re-trigger the migration.
- **Versioned `DeltaChannel` shape.** The class is beta (`libs/langgraph/langgraph/channels/delta.py:29-36`); when it stabilizes, the `_DeltaSnapshot` ext type and the `counters_since_delta_snapshot` metadata field need a documented version field.
- **Strict-mode migration helper.** A `JsonPlusSerializer.migrate_v0_to_v1(payload)` API would let users with `LANGGRAPH_STRICT_MSGPACK=true` read pre-`v1.0.1` lc=2 envelopes without disabling strict mode globally.

## Questions / Gaps

- **Which checkpoints are `v=1` vs `v=2` vs `v=3` vs `v=4`?** The codebase defines both `LATEST_VERSION = 2` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:811`) and `LATEST_VERSION = 4` (`libs/langgraph/langgraph/pregel/_checkpoint.py:21`) without explaining what happened at v2 and v3 in between. The `StateGraph._migrate_checkpoint` short-circuit at `>= 3` (`libs/langgraph/langgraph/graph/state.py:1625`) and the `Pregel._migrate_checkpoint` gate at `< 4` (`libs/langgraph/langgraph/pregel/main.py:1137`) imply a `start:* → branch:to:*` rename between v2 and v3, and a `pending_sends → channel_values[TASKS]` move at v4 — but no in-repo changelog captures these transitions.
- **Is the sqlite on-disk layout bit-compatible with the Postgres on-disk layout for v3 checkpoints?** Postgres stores checkpoints in `JSONB` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:53`); SQLite stores checkpoints in `BLOB` via `serde.dumps_typed` (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:148`). Cross-backend copy is therefore impossible without re-encoding through the serde. No migration test covers this path.
- **Where is the v3 fixture for `pending_sends`?** `SAVED_CHECKPOINTS` (`libs/langgraph/tests/test_checkpoint_migration.py:272-1397`) only stores v1 and v2 fixtures. The v<4 `pending_sends` migration is exercised only via the synthetic `MemorySaverNeedsPendingSendsMigration` (`libs/langgraph/tests/memory_assert.py:29-48`); there is no real historical v3 snapshot with `pending_sends` in the corpus.
- **How does `pending_sends` survive if `LANGGRAPH_STRICT_MSGPACK=true`?** The `_migrate_pending_sends` decode (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:317-319`) calls `self.serde.loads_typed` with no allowlist. A v3 thread written under permissive mode, loaded under strict mode, would raise inside the migration. The behavior is not tested.
- **Is `update_state` covered by `_migrate_checkpoint`?** `Pregel._migrate_checkpoint` is called in the `_prepare_state_snapshot` and `update_state` paths (`libs/langgraph/langgraph/pregel/main.py:1164, 1287, 1646, 2119`), but the `tup.checkpoint["v"]` access in those paths is implicit — no test parametrizes a v<4 thread going through `update_state` (only `get_state_history` / `stream(Command(resume=...))`).
- **Does the `_DeltaSnapshot` ext type have a "version" subfield?** `EXT_DELTA_SNAPSHOT = 7` is reserved in `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:302`, but the inner tuple format is `(value)` (encoded via `_msgpack_enc(obj.value)` at `:307`). There is no version marker inside the ext payload — adding one would be a breaking change to any reader that decodes the ext directly.
- **What happens to a `ShallowPostgresSaver` thread after the v<4 migration?** The shallow saver is deprecated (`:192-197`) and the SELECT_SQL already returns `pending_sends` as a column (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:106-112`), so the migration is largely a no-op for shallow saves. No test asserts this end-to-end.

---

Generated by `02.06-schema-versioning-and-migration` against `langgraph`.