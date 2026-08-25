# Source Analysis: langgraph

## Session, Thread, and User Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: checkpoint, checkpoint-postgres, checkpoint-sqlite, checkpoint-conformance, langgraph, prebuilt, sdk-py, sdk-js, cli) |
| Analyzed | 2026-07-10 |

## Summary

LangGraph builds its identity and isolation model on a few very small primitives: a configurable dict in `RunnableConfig` carries `thread_id`, `checkpoint_ns`, `checkpoint_id`, and `checkpoint_map` (`libs/langgraph/langgraph/_internal/_constants.py:52-58`); the `BaseCheckpointSaver` interface scopes every read and write by `thread_id` plus `checkpoint_ns` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:227-294`); and the long-term memory `BaseStore` is scoped by an arbitrary tuple `namespace` keyed at the storage layer by a single `prefix` column (`libs/checkpoint/langgraph/store/base/__init__.py:51-115`, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:64-77`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:41-72`).

There is no native concept of a `user_id` or `tenant_id` inside the framework. Identity comes from two sources: a `user_id` field exposed on `Cron` payloads for the LangGraph Server (`libs/sdk-py/langgraph_sdk/schema.py:406`) and `ServerInfo.user` (typed as `BaseUser`) when a graph is hosted on LangGraph Server (`libs/langgraph/langgraph/runtime.py:60-77`). Per-user scoping is therefore not enforced by the checkpoint or store primitives themselves; it is delegated to user-supplied `Auth` handlers in `langgraph_sdk.auth` that can mutate the `value["metadata"]["owner"]`, `value["namespace"]`, or return a `FilterType` filter that the server-side runtime applies as a SQL/JSON predicate (`libs/sdk-py/langgraph_sdk/auth/__init__.py:62-94`, `libs/sdk-py/langgraph_sdk/auth/types.py:58-109,862-974`).

Cross-thread memory is explicit. The `Store` is named "cross-thread memory" by the SDK schema (`libs/sdk-py/langgraph_sdk/schema.py:549-567`), and the only path between threads is sharing the `BaseStore` instance on the `Runtime` (`libs/langgraph/langgraph/runtime.py:124-204`). Checkpoints, in contrast, are scoped strictly per-thread: every SQL query in `PostgresSaver`, `SqliteSaver`, and `ShallowPostgresSaver` predicates on `thread_id` and `checkpoint_ns`, and the composite primary key of `checkpoints`, `checkpoint_writes`, `checkpoint_blobs`, and the SQLite `checkpoints`/`writes` tables include `thread_id` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:47-77`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:142-162`). The only protocol-level caveat is that the **caller is responsible for picking a `thread_id` value** — LangGraph does not generate, hash, or namespace it; two users passing the same string get the same row.

Forking is supported at two levels. The `BaseCheckpointSaver` defines `copy_thread(source, target)` and the conformance suite asserts `acopy_thread` semantics including namespace preservation and ordering (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:350-372`, `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_copy_thread.py:113-189`). The Pregel `update_state` flow also produces a forked checkpoint with metadata `source: "fork"` for `__copy__` updates (`libs/langgraph/langgraph/pregel/main.py:1818,2288`). Neither Postgres nor SQLite implement `acopy_thread` (verified by absence in `libs/checkpoint-postgres/langgraph/checkpoint/postgres/{__init__.py,aio.py,shallow.py}` and `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/{__init__.py,aio.py}`); the capability is classified as "extended" and only enforced by the conformance suite (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:18-50`).

Tenant boundaries at the storage layer are **application-enforced, not framework-enforced**. The `BaseStore` documents that namespace tuples are the recommended per-user scoping mechanism but does not require it (`libs/checkpoint/langgraph/store/base/__init__.py:700-717`); the in-memory store uses a `defaultdict` keyed by the full tuple so cross-namespace reads are impossible only if the caller supplies the right tuple (`libs/checkpoint/langgraph/store/memory/__init__.py:186-217`). Postgres stores use a flat `prefix` string (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:64-72`); the `runtime` injects `ctx.user.identity` as the namespace prefix only if the developer writes an `@auth.on.store` handler (`libs/sdk-py/langgraph_sdk/auth/__init__.py:87-94,193-216`).

## Rating

**Score: 7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards**

Rationale:
- The thread/run/checkpoint identity is unambiguous and the storage layers consistently scope reads/writes to that identity (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:227-262`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:571-594`).
- A complete conformance suite locks down the isolation contract across backends (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delete_thread.py:70-107,179-198`, `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_copy_thread.py:113-189`).
- The `Auth` middleware is a first-class, declarative SDK surface with `FilterType` for both `metadata` and `namespace` rewriting (`libs/sdk-py/langgraph_sdk/auth/__init__.py:62-94,193-216`, `libs/sdk-py/langgraph_sdk/auth/types.py:58-109`).
- Cross-thread leakage is impossible by construction in the **memory store** (only if you pass the correct namespace) and in the **checkpoint store** (only if you pass the correct `thread_id`), but the framework does not generate those IDs and does not refuse to look up an arbitrary user-supplied `thread_id`.
- No native `tenant_id` column or row-level-security helper is shipped in OSS libraries; cross-tenant isolation is a documented user responsibility (`libs/checkpoint/langgraph/store/base/__init__.py:700-717`).
- The `acopy_thread`/`aprune`/`adelete_for_runs` capability set is declared but neither Postgres nor SQLite implementations override the base `NotImplementedError` (verified by absence in `libs/checkpoint-postgres/langgraph/checkpoint/postgres/` and `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/`).

Deductions from a higher score:
- `-1`: `user_id`/`tenant_id` are not first-class schema fields; cross-user isolation depends entirely on the developer adding an `Auth` handler.
- `-1`: Several documented capabilities (`copy_thread`, `prune`, `delete_for_runs`) are declared in the conformance suite but not implemented in either Postgres or SQLite production savers.
- `-1`: The thread_id is just a `str`; there is no helper for deriving a thread_id from a user identity, and no `validate` that warns when a developer reuses an obvious value like `"default"`.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Configurable identity keys | `CONFIG_KEY_THREAD_ID = sys.intern("thread_id")`, `CONFIG_KEY_CHECKPOINT_ID`, `CONFIG_KEY_CHECKPOINT_NS`, `CONFIG_KEY_CHECKPOINT_MAP` | `libs/langgraph/langgraph/_internal/_constants.py:52-58` |
| Coordinate-key grouping | `_CHECKPOINT_COORDINATE_KEYS = (thread_id, checkpoint_ns, checkpoint_id, checkpoint_map)` marks "checkpoint lineage" keys | `libs/langgraph/langgraph/_internal/_constants.py:100-105` |
| Reserved config keys | `RESERVED` set prevents collision with framework keys | `libs/langgraph/langgraph/_internal/_constants.py:110-139` |
| Checkpoint metadata schema | `CheckpointMetadata` records `source`, `step`, `parents`, `run_id` (no `user_id`/`tenant_id`) | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` |
| Checkpoint identity | `Checkpoint` carries `v`, `id`, `ts`, `channel_values`, `channel_versions`, `versions_seen`, `updated_channels` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-123` |
| Base saver interface | `BaseCheckpointSaver` reads/writes keyed by `(thread_id, checkpoint_ns, checkpoint_id)`; `get_tuple`, `list`, `put`, `put_writes`, `delete_thread` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:227-329,417-538` |
| Thread copy contract | `copy_thread(source_thread_id, target_thread_id)` declared in base; warns implementers to copy full parent chain | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:350-372` |
| Async thread copy | `acopy_thread(source_thread_id, target_thread_id)` mirror | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:540-558` |
| In-memory checkpoint storage layout | `storage: defaultdict[thread_id, ns, checkpoint_id]`; `delete_thread` cleans all keys starting with thread_id | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:68-83,511-527` |
| In-memory `get_tuple` queries | Reads always filter by `thread_id` + `checkpoint_ns` (+ optional `checkpoint_id`) | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:236-316` |
| Postgres checkpoint schema | `checkpoints PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id)`; `checkpoint_blobs PRIMARY KEY (thread_id, checkpoint_ns, channel, version)`; `checkpoint_writes PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:47-77` |
| Postgres `get_tuple` scoping | `WHERE thread_id = %s AND checkpoint_ns = %s [AND checkpoint_id = %s]` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:227-262` |
| Postgres `delete_thread` scoping | `DELETE FROM checkpoints WHERE thread_id = %s`, etc. for `checkpoint_blobs` and `checkpoint_writes` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402` |
| Postgres `list` scoping | `_search_where` adds `thread_id = %s` predicate when config supplied | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:567-594` |
| Shallow Postgres schema (deprecated) | `checkpoints PRIMARY KEY (thread_id, checkpoint_ns)` — no `checkpoint_id` because only the latest is kept | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:43-58` |
| SQLite checkpoint schema | `checkpoints PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id)`; `writes PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:142-162` |
| SQLite `get_tuple`/`list` scoping | `WHERE thread_id = ? AND checkpoint_ns = ?` always present | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:226-269,335-373` |
| SQLite `delete_thread` scoping | `DELETE FROM checkpoints WHERE thread_id = ?` and `DELETE FROM writes WHERE thread_id = ?` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501` |
| Store `Item` shape | `value`, `key`, `namespace: tuple[str, ...]` | `libs/checkpoint/langgraph/store/base/__init__.py:51-115` |
| Store namespace validation | Rejects empty tuple, non-string labels, periods, empty strings, and `langgraph` root | `libs/checkpoint/langgraph/store/base/__init__.py:1255-1275` |
| Store docstring | "Stores enable persistence and memory that can be shared across threads, scoped to user IDs, assistant IDs, or other arbitrary namespaces" | `libs/checkpoint/langgraph/store/base/__init__.py:700-717` |
| In-memory store layout | `_data: dict[(namespace_tuple), dict[key, Item]]` | `libs/checkpoint/langgraph/store/memory/__init__.py:186-190` |
| In-memory store search filter | `_filter_items` matches `namespace[:len(prefix)] == prefix` and `filter` dict; cannot cross namespace boundaries | `libs/checkpoint/langgraph/store/memory/__init__.py:238-266` |
| Postgres store schema | `store PRIMARY KEY (prefix, key)` where `prefix` is `namespace` joined with `.`; `prefix LIKE %s` for prefix search | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:64-77,442-464` |
| Postgres store vector schema | `store_vectors PRIMARY KEY (prefix, key, field_name)` + FK to `store(prefix, key)` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:104-114` |
| Postgres store list_namespaces | Uses `prefix LIKE %s` (prefix match) and `%s` (suffix match); query parameter is `_namespace_to_text(prefix)` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:568-620` |
| SQLite store schema | `store PRIMARY KEY (prefix, key)`; prefix index for fast prefix lookups | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:41-72` |
| Auth handler registration | `Auth.authenticate` returns user with `identity`, `permissions` | `libs/sdk-py/langgraph_sdk/auth/__init__.py:225-301` |
| Auth resource/action matrix | Decorators `@auth.on.threads.create/read/update/delete/search`, `@auth.on.threads.create_run`, `@auth.on.assistants.*`, `@auth.on.store.put/get/search/delete/list_namespaces` | `libs/sdk-py/langgraph_sdk/auth/__init__.py:362-602,681-813` |
| Auth handler return contract | `None`/`True` accept, `False` deny, `FilterType` applies as metadata/JSONB filter | `libs/sdk-py/langgraph_sdk/auth/types.py:58-145` |
| Default thread scoping example | `@auth.on.threads.read` returns `{"owner": ctx.user.identity}` → only own threads | `libs/sdk-py/langgraph_sdk/auth/__init__.py:62-94` |
| Store namespace rewriting example | `@auth.on.store` mutates `value["namespace"] = (ctx.user.identity, *value["namespace"])` | `libs/sdk-py/langgraph_sdk/auth/__init__.py:87-94,193-216` |
| Store type payloads are mutable | `StoreGet`, `StoreSearch`, `StorePut`, `StoreDelete`, `StoreListNamespaces` documented as mutable dicts auth can rewrite | `libs/sdk-py/langgraph_sdk/auth/types.py:862-974` |
| SDK Cron schema carries `user_id` | `Cron.user_id: str \| None` | `libs/sdk-py/langgraph_sdk/schema.py:406` |
| SDK Item is "cross-thread memory" | `Item` docstring: "Items are used to store cross-thread memories." | `libs/sdk-py/langgraph_sdk/schema.py:549-567` |
| SDK Assistant/Thread schemas | AssistantSchema lacks `owner`; Thread carries `metadata` (server fills `owner`) and `config`, `context` | `libs/sdk-py/langgraph_sdk/schema.py:160-525` |
| Runtime identity surface | `ExecutionInfo` exposes `thread_id`, `checkpoint_id`, `checkpoint_ns`, `task_id`, `run_id` | `libs/langgraph/langgraph/runtime.py:26-58` |
| `ServerInfo` carries user | `ServerInfo.user: BaseUser \| None` populated by LangGraph Server | `libs/langgraph/langgraph/runtime.py:60-77` |
| `Runtime.store` is the cross-thread channel | `store: BaseStore \| None` field on Runtime, shared across threads | `libs/langgraph/langgraph/runtime.py:124-204` |
| `get_runtime()` access | Pulls `Runtime` from `configurable["__pregel_runtime"]` | `libs/langgraph/langgraph/runtime.py:296-310` |
| Pregel fork metadata | `update_state` writes metadata `source: "fork"` for `__copy__` updates | `libs/langgraph/langgraph/pregel/main.py:1818,2288` |
| Conformance: thread delete isolation | `test_delete_thread_removes_all_namespaces`, `test_delete_thread_preserves_other_threads` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_delete_thread.py:70-107` |
| Conformance: namespace filtering | `test_get_tuple_respects_namespace` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_get_tuple.py:179-198` |
| Conformance: copy thread namespaces | `test_copy_thread_preserves_namespaces` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_copy_thread.py:113-131` |
| Conformance: copy thread ordering | `test_copy_thread_preserves_ordering` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_copy_thread.py:155-174` |
| Capability matrix | `BASE_CAPABILITIES = {put, put_writes, get_tuple, list, delete_thread}`; `EXTENDED = {delete_for_runs, copy_thread, prune, delta_channel_history}` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:18-50` |

## Answers to Dimension Questions

1. **What scopes state?**
   - Threads scope short-term checkpointed state. The `RunnableConfig` `configurable` dict carries `thread_id`, `checkpoint_ns`, `checkpoint_id`, and a `checkpoint_map` (`libs/langgraph/langgraph/_internal/_constants.py:52-58`); every SQL query in Postgres and SQLite checkpointers predicates on `(thread_id, checkpoint_ns)` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:571-594`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:226-269`).
   - Namespaces scope long-term memory. `BaseStore` indexes items by `namespace: tuple[str, ...]` (`libs/checkpoint/langgraph/store/base/__init.py:51-115`), which is flattened to a `prefix` column for SQL backends (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:64-77`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:41-72`).
   - Runs scope execution metadata. `run_id` is plumbed via `ExecutionInfo.run_id` (`libs/langgraph/langgraph/runtime.py:44-48`) and `RunnableConfig` (`libs/sdk-py/langgraph_sdk/auth/types.py:571-572`), and persisted in checkpoint metadata (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:61-62`).
   - Users are not a built-in scope. There is no `user_id` column in any framework table; `ServerInfo.user` is a server-runtime injection (`libs/langgraph/langgraph/runtime.py:60-77`).

2. **Can state leak across users or sessions?**
   - In the **checkpoint store**, state can leak only if the caller passes the wrong `thread_id`. There is no row-level filter on `user_id` because the column does not exist. Two users with the same `thread_id` (e.g., the literal string `"default"`) will read/write the same rows. The base saver trusts the `configurable["thread_id"]` value verbatim (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:227-262`).
   - In the **memory store**, the same rule applies but the namespace tuple is the only key, so a wrong namespace yields `None`. The store does not validate the namespace against any user identity (`libs/checkpoint/langgraph/store/memory/__init__.py:238-266`).
   - Cross-thread leakage via the **store** is the documented pattern, not a bug — `Store` items are "cross-thread memories" (`libs/sdk-py/langgraph_sdk/schema.py:549-567`) and `Runtime.store` is shared across all threads of a graph (`libs/langgraph/langgraph/runtime.py:124-204`).
   - The only built-in safeguard is the namespace validator (no empty tuples, no dots, no `langgraph` root) (`libs/checkpoint/langgraph/store/base/__init__.py:1255-1275`), which prevents structural corruption but not cross-user leakage.

3. **Is cross-thread memory explicit?**
   - Yes. `Store` items have a namespace that is independent of `thread_id`. The same `BaseStore` instance is injected into all threads via `Runtime.store` (`libs/langgraph/langgraph/runtime.py:124-204`), and the SDK schema explicitly says "Items are used to store cross-thread memories" (`libs/sdk-py/langgraph_sdk/schema.py:549-567`).
   - Conversely, checkpoint state is strictly per-thread; there is no API to read another thread's checkpoint other than passing that thread's `thread_id` in `configurable`.

4. **Are tenant boundaries enforced at storage level?**
   - No. The Postgres and SQLite checkpoint schemas use `thread_id` as the only isolation column (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:47-77`); the store schemas use `prefix` (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:64-77`, `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:41-72`). There is no `tenant_id`, no row-level-security policy, and no schema-per-tenant helper.
   - Enforcement is the application's responsibility, typically via an `Auth` handler that mutates `value["metadata"]["owner"]` (for threads/assistants) or `value["namespace"]` (for store) so the server applies an `{"owner": ctx.user.identity}` filter or rewrites the prefix (`libs/sdk-py/langgraph_sdk/auth/__init__.py:62-94,193-216`).
   - The langgraph SDK does provide a `StudioUser` model and `is_studio_user` helper to allow developers to bypass per-user scoping on LangGraph Studio requests (`libs/sdk-py/langgraph_sdk/auth/__init__.py:867-873`, `libs/sdk-py/langgraph_sdk/auth/types.py:218-274`).

5. **Can users fork sessions safely?**
   - Yes, via `copy_thread(source, target)` declared on the base saver and verified by the conformance suite (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:350-372`, `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_copy_thread.py:45-189`). Tests cover namespace preservation, ordering, source-immutability, and missing-source behavior.
   - The Pregel `update_state(..., as_node="__copy__")` path produces a forked checkpoint with metadata `source: "fork"` and reuses the parent's checkpoint map (`libs/langgraph/langgraph/pregel/main.py:1810-1822,2280-2292`).
   - **However**, neither `PostgresSaver`, `AsyncPostgresSaver`, `ShallowPostgresSaver`, nor `SqliteSaver`/`AsyncSqliteSaver` overrides `copy_thread`/`acopy_thread` — they inherit the base `NotImplementedError` (confirmed by absence in the listed files). Only the conformance suite marks this as an extended capability (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:18-50`). Production users must rely on the LangGraph Server REST endpoint `POST /threads/{thread_id}/copy` exposed by the SDK (`libs/sdk-py/langgraph_sdk/_sync/threads.py:401-432`).

## Architectural Decisions

- **`thread_id` is a string, not a structured type.** Documented in `BaseCheckpointSaver` as the primary key for storage and retrieval; the docstring suggests choosing a unique ID per run for one-shot workflows and reusing it across invocations for conversational memory (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:177-200`). No type system prevents two callers from accidentally colliding on the same string.
- **Composite key `(thread_id, checkpoint_ns, checkpoint_id)`** for checkpoints lets subgraphs share a `thread_id` while owning their own lineage. `checkpoint_ns = ""` for the root graph; subgraphs get `"{parent_ns}|{node_name}:{task_id}"` via `NS_SEP`/`NS_END` (`libs/langgraph/langgraph/_internal/_constants.py:87-89`). This is reflected in the Postgres primary key (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:47-56`) and the SQLite one (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:142-151`).
- **Store namespaces are tuples of strings, validated to disallow dots, empty strings, and the `langgraph` prefix.** This enforces structural correctness without making any identity claim (`libs/checkpoint/langgraph/store/base/__init__.py:1255-1275`).
- **`Runtime.store` is shared across threads by design.** This is the only path to "cross-thread memory" — there is no per-thread store. `ExecutionInfo.thread_id` exposes the active thread to the node if it wants to filter, but the framework does not do so automatically (`libs/langgraph/langgraph/runtime.py:26-58,124-204`).
- **`Auth` returns a `FilterType` rather than fetching and filtering client-side.** This is the load-bearing decision for tenant boundaries: the server-side runtime applies the filter as a SQL `metadata @>` predicate (or `value @> %s` for stores), so the data never leaves the database for the wrong user. Examples in the SDK docstring show `{"owner": ctx.user.identity}` (`libs/sdk-py/langgraph_sdk/auth/types.py:58-109`, `libs/sdk-py/langgraph_sdk/auth/__init__.py:62-94`).
- **`StudioUser` escape hatch.** A developer can disable per-user scoping for LangGraph Studio requests via `disable_studio_auth: true` or via `is_studio_user(ctx.user)` checks (`libs/sdk-py/langgraph_sdk/auth/types.py:218-251,867-873`). This is an explicit decision to make local development frictionless at the cost of needing care in production.
- **Capability-based saver contract.** The conformance suite treats `delete_for_runs`, `copy_thread`, `prune`, and `delta_channel_history` as optional capabilities (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-50`), letting the LangGraph Server use a richer in-memory saver while downstream deployments stick to minimal Postgres/SQLite implementations.

## Notable Patterns

- **`RunnableConfig["configurable"]` as a transport for scope IDs.** The single config dict that flows through every layer carries `thread_id`, `checkpoint_id`, `checkpoint_ns`, `checkpoint_map`, plus framework-reserved keys (`__pregel_runtime`, `__pregel_tasks`, …) (`libs/langgraph/langgraph/_internal/_constants.py:32-92`). This avoids per-thread schema changes at the cost of treating the dict as a quasi-typed bag.
- **Validation as defense.** The store's namespace validator (`libs/checkpoint/langgraph/store/base/__init__.py:1255-1275`) and the SQLite filter-key regex `^[a-zA-Z0-9_.-]+$` (`libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:110-130`) prevent storage-level injection without claiming to enforce identity.
- **Auth handler precedence chain.** `_On` documents the precedence chain: exact resource/action match → resource wildcard → global handler → accept by default if no global handler is registered (`libs/sdk-py/langgraph_sdk/auth/__init__.py:96-107`).
- **Resource handlers for store, not global filters.** The store API is not naturally metadata-filter-shaped (its primary key is `namespace` + `key`), so the SDK documents rewriting `value["namespace"]` instead of returning a `FilterType` (`libs/sdk-py/langgraph_sdk/auth/__init__.py:189-216`, `libs/sdk-py/langgraph_sdk/auth/types.py:862-974`).
- **Capability detection by method override.** `DetectedCapabilities.from_instance` walks each `Capability` enum and checks whether the class has overridden the corresponding async method (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:66-96`). This avoids `hasattr` heuristics.

## Tradeoffs

- **Per-thread IDs without per-user IDs.** The framework chose to make `thread_id` the primary identity because conversations are the natural unit of state, while offloading user identity to a server-issued `BaseUser`. The trade-off is that any process that can supply `thread_id` can read or write state — the safeguard is the caller's auth layer, not the storage layer.
- **Namespace as a "soft" tenant boundary.** Namespaces are arbitrary tuples, so `("user-a", "memories")` and `("user-b", "memories")` look symmetric to the storage layer. There is no helper that derives the namespace from `ctx.user.identity` automatically; that helper lives in the example code of the SDK (`libs/sdk-py/langgraph_sdk/auth/__init__.py:193-216`).
- **`FilterType` is rich but server-specific.** The `FilterType` grammar (`$eq`, `$contains`) is honored by the LangGraph Server's metadata search; documented subset containment (`{"$contains": [...]}`) requires a newer server runtime (`libs/sdk-py/langgraph_sdk/auth/types.py:75-77`). Smaller deployments on bare Postgres/SQLite must implement filter semantics themselves.
- **`copy_thread` is documented but unimplemented in OSS savers.** The base class raises `NotImplementedError` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:371-372`), and Postgres/SQLite do not override it (verified). The capability exists in the conformance suite (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:24,60`), so custom savers and the LangGraph Server implement it.
- **`delete_thread` is destructive without `tenant_id` awareness.** `PostgresSaver.delete_thread(thread_id)` issues `DELETE FROM checkpoints WHERE thread_id = %s` for three tables (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`). If two users share a `thread_id` by accident, deleting one deletes the other's data.
- **`ShallowPostgresSaver` keeps only the most recent checkpoint.** Its `checkpoints` PK is `(thread_id, checkpoint_ns)` only — `checkpoint_id` is omitted entirely (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:43-50`). It is deprecated as of 2.0.20 in favor of `PostgresSaver` + `durability='exit'` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:192-196`).
- **`StudioUser` defaults to unauthenticated.** `StudioUser(is_authenticated=False)` is the default; permissions default to `["authenticated"]` only when `is_authenticated=True` (`libs/sdk-py/langgraph_sdk/auth/types.py:255-258`). Combined with the `disable_studio_auth` flag, this gives developers an easy footgun.

## Failure Modes / Edge Cases

- **Two users with the same `thread_id` see each other's state.** No safeguard at the framework layer; relies on the developer using a stable per-user or per-session ID. No `validate_thread_id` helper exists.
- **Delete is whole-thread.** `delete_thread` removes all checkpoints and writes for the thread in a single transaction. There is no "delete for user X" or "delete for run Y unless checkpoint is shared" path (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:381-402`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:484-501`).
- **`prune` corrupts `DeltaChannel` reconstruction if not careful.** The base class documents that `keep_latest` may sever the parent chain and silently reconstruct delta channels as empty (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:386-414`).
- **Postgres `get_delta_channel_history` paged scan can leak across threads if `thread_id` is omitted.** The query always supplies `thread_id`, but the parameter is interpolated as `%s` so an unsafe override could expose data (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:466-522`).
- **In-memory store is process-local.** A second process (or a reload) starts empty; the only durability is what the caller wires up. Cross-process cross-thread sharing requires a real Postgres/SQLite backend (`libs/checkpoint/langgraph/store/memory/__init__.py:91-99`).
- **TTL sweeper is process-local.** `PostgresStore.start_ttl_sweeper` spawns a `threading.Thread` inside the current process (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:872-879`); in multi-replica deployments only one replica sweeps at a time per store instance, and the sweep is best-effort (not a scheduled job).
- **`acopy_thread` raises `NotImplementedError`** on the in-memory and Postgres/SQLite savers; callers must rely on the LangGraph Server endpoint or implement their own (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:540-558`).
- **`disable_studio_auth: true`** can be left enabled in production accidentally; there is no warning in the schema (`libs/sdk-py/langgraph_sdk/auth/types.py:218-251`).

## Future Considerations

- Promote `user_id` (or a more general `subject_id`) to a first-class column on `checkpoints`/`checkpoint_writes` and `store` so row-level security or query-time filters can be enforced even when an `Auth` handler is forgotten.
- Implement `acopy_thread`/`aprune`/`adelete_for_runs` in the OSS Postgres and SQLite savers so users can fork/sanitize threads without requiring the LangGraph Server.
- Provide a built-in `@auth.on.store` template (in `langgraph_sdk.auth`) that scopes namespaces to `ctx.user.identity` so the SDK example stops being the only reference implementation (`libs/sdk-py/langgraph_sdk/auth/__init__.py:189-216`).
- Add a `validate_thread_id` utility and warn at compile time if a developer uses a literal `"default"`/`""`/`None` thread_id.
- Add `prune` documentation/CLI tooling for the `DeltaChannel` caveat so naive `keep_latest` doesn't silently empty delta channels (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:386-414`).
- Document the `StudioUser` escape hatch more visibly in production-mode auth setup; today it's only discoverable via `disable_studio_auth` JSON config (`libs/sdk-py/langgraph_sdk/auth/types.py:218-251`).
- Surface a single `LangGraph isolation overview` doc that shows the four-tier model (run → thread → checkpoint_ns → user/tenant) so security reviewers know where to enforce what.

## Questions / Gaps

- **No `tenant_id`/`workspace_id` primitive.** Searched: `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/checkpoint`, `libs/langgraph`, `libs/sdk-py` for the strings `tenant`, `workspace_id`, `org_id`. No matches found. Multi-tenant deployments must use one database per tenant or partition `thread_id` themselves.
- **`acopy_thread`/`aprune`/`adelete_for_runs` implementation gap.** The base interface declares them and the conformance suite tests them, but neither `PostgresSaver`, `AsyncPostgresSaver`, `ShallowPostgresSaver`, nor `SqliteSaver`/`AsyncSqliteSaver` overrides the base `NotImplementedError` (verified by `grep -n "def a\(copy_thread\|prune\|delete_for_runs\)" libs/checkpoint-postgres libs/checkpoint-sqlite libs/checkpoint/langgraph/checkpoint`). This is a gap that the LangGraph Server evidently fills for hosted users but not for self-hosted deployments.
- **No evidence of cross-store authorization filters.** The `FilterType` contract (`libs/sdk-py/langgraph_sdk/auth/types.py:58-109`) describes metadata filtering for resources like `threads`/`assistants`, but the `store` API has no equivalent — auth handlers must rewrite `namespace` instead (`libs/sdk-py/langgraph_sdk/auth/__init__.py:189-216`, `libs/sdk-py/langgraph_sdk/auth/types.py:862-974`). Whether filter semantics are honored by the store query layer is undocumented in the SDK.
- **No read-after-write consistency guarantees documented.** `PostgresSaver` uses `autocommit=True` by default (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:76-78`), which can break transactional read-your-writes. There is no documented best practice for cross-process checkpoint reads.
- **No evidence that `InMemoryStore` is safe to share across threads in the same process.** It uses `defaultdict(dict)` without locks (`libs/checkpoint/langgraph/store/memory/__init__.py:186-190`); concurrent `put`/`get` may race. `Batch` operations appear atomic per-call, but the framework does not document this.
- **No `delete_namespace` helper on `BaseStore`.** Applications must iterate and call `delete(namespace, key)` for each item; no bulk delete API. Searched `libs/checkpoint/langgraph/store/base/__init__.py` and implementations.

---

Generated by `02.07-session-thread-and-user-boundaries.md` against `langgraph`.