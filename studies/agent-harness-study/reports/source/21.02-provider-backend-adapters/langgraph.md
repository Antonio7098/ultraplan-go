# Source Analysis: langgraph

## Dimension 21.02: Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/checkpoint`, `libs/checkpoint-postgres`, `libs/checkpoint-sqlite`, `libs/checkpoint-conformance`, `libs/langgraph`, `libs/prebuilt`, `libs/cli`); TypeScript SDKs in `libs/sdk-js` (out of scope here) |
| Analyzed | 2026-08-25 |

> All citations below are relative to the source root, e.g. `libs/checkpoint/...` = `studies/agent-harness-study/sources/langgraph/libs/checkpoint/...`.

## Summary

LangGraph's swappable-backend story is concentrated in a dedicated persistence layer (`langgraph-checkpoint`) that defines abstract base classes for three storage families — checkpoint savers, long-term-memory stores, and node caches — plus two cross-cutting adapter seams (serialization and embeddings). The core engine (`libs/langgraph`) consumes only the abstractions: a compiled graph takes `checkpointer`, `store`, and `cache` objects at `compile()` time and resolves them per-invocation through config keys, so any conforming implementation can be injected without touching engine code.

Multiple concrete backends ship in-repo per family: checkpointer (in-memory, SQLite, PostgreSQL — including shallow and pipeline/pool variants), store (in-memory, SQLite, PostgreSQL with pgvector semantic search and TTL), cache (in-memory, Redis, SQLite). Interchangeability is not just claimed but mechanically enforced twice: (1) the core test suite parametrizes every graph test across all sync/async backends via pytest fixtures (`libs/langgraph/tests/conftest.py:60-226`), and (2) a dedicated conformance package (`langgraph-checkpoint-conformance`) lets third-party saver authors validate their implementation against the storage contract, auto-detecting which optional capabilities they implement.

Model providers are adapted through LangChain's `BaseChatModel` interface: `create_react_agent` accepts a provider string (`"openai:gpt-4"` resolved via `init_chat_model`), a model instance, or a `(state, runtime) -> BaseChatModel` callable for per-invocation dynamic selection. Adapter configuration is partially externalized: the deployment CLI's `langgraph.json` schema supports declarative `checkpointer.path` (an import string to an async context manager yielding any `BaseCheckpointSaver`), `store.index.embed`, custom `encryption.path`, and TTL settings, injected into deployments as env vars. Queues and sandboxes have no pluggable abstraction in this source; tracing is delegated to LangChain callbacks rather than a local sink interface.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces**: `BaseCheckpointSaver` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:176`), `BaseStore` (`libs/checkpoint/langgraph/store/base/__init__.py:708`), `BaseCache` (`libs/checkpoint/langgraph/cache/base/__init__.py:15`), `SerializerProtocol` (`libs/checkpoint/langgraph/checkpoint/serde/base.py:14-26`).
- **Tests proving interchangeability**: identical core suites run against 7 sync + 5 async checkpointer configurations, 4 store configurations, and 3 cache backends (`libs/langgraph/tests/conftest.py:60-226`); a standalone conformance suite exists for external implementers (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/validate.py:45`).
- **Operational safeguards**: capability detection (`supports_ttl` raising `NotImplementedError` at `libs/checkpoint/langgraph/store/base/__init__.py:920-924`), graceful degradation for older third-party savers (`libs/langgraph/langgraph/_internal/_serde.py:36-57`), migration system for Postgres schema evolution (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:43-91`), and explicit warnings documenting contract obligations for custom implementations (e.g., `prune` DeltaChannel caveats, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:387-414`).
- **Why not 9–10**: OSS-library users select backends by importing and instantiating concrete classes — there is no string-keyed registry/factory mapping backend names to implementations inside the core libraries; sync/async pairs are hand-maintained duplicates rather than generated from one adapter; and whole categories named by the dimension (queues, sandboxes, tracing sinks) have no adapter seam here. The deployment-time config path (`CheckpointerConfig.path`) mitigates the registry gap only at the server layer.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Checkpointer abstraction | `BaseCheckpointSaver[Generic[V]]` defines the full storage contract: `get_tuple`, `list`, `put`, `put_writes`, `delete_thread`, `delete_for_runs`, `copy_thread`, `prune`, plus async mirrors; injectable `serde`; `config_specs` extension hook | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176-580` |
| Saver capability metadata | `config_specs` property lets savers declare additional config fields; `get_next_version` overridable for str/int/float versions | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:218-225,692-711` |
| In-memory reference impl | `InMemorySaver(BaseCheckpointSaver[str])`; `MemorySaver` alias kept for backwards compatibility | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33,625` |
| SQLite implementation | `SqliteSaver.from_conn_string(...)` context-manager factory; separate `AsyncSqliteSaver` built on `aiosqlite` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py` (module), `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/aio.py:11,38` |
| Postgres implementation | `PostgresSaver` / `AsyncPostgresSaver` share `BasePostgresSaver`; versioned `MIGRATIONS` list; optional psycopg `Pipeline` and `ConnectionPool` construction paths | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:40,64`; `aio.py:40,65`; `base.py:43-91` |
| Shallow-history variant | `ShallowPostgresSaver` / `AsyncShallowPostgresSaver` keep only latest checkpoint per thread, same base class | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/shallow.py:169,529` |
| Store abstraction | `BaseStore(ABC)` — only `batch`/`abatch` are abstract; `get/search/put/delete/list_namespaces` are convenience decompositions into ops; `supports_ttl` capability flag default `False` | `libs/checkpoint/langgraph/store/base/__init__.py:708-754,727` |
| Store op protocol | Ops as NamedTuples (`GetOp/SearchOp/PutOp/ListNamespacesOp`) decouple callers from storage mechanics | `libs/checkpoint/langgraph/store/base/__init__.py:157-220` (and class docs throughout) |
| In-memory store | `InMemoryStore(BaseStore)` reference implementation | `libs/checkpoint/langgraph/store/memory/__init__.py:136` |
| Postgres store w/ vectors | `PostgresStore` + ANN index configs (HNSW/IVFFlat); `supports_ttl = True`; semantic search via `index={dims, embed, fields}` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:178-236,667,755` |
| SQLite store w/ TTL | `SqliteStore` sets `supports_ttl = True`; shares query-prep base `BaseSqliteStore` between sync/aio | `libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:787,853,273-278` |
| Async batch adapter | `AsyncBatchedBaseStore` coalesces concurrent async ops into background batches with dedupe — an adapter decorating any sync-capable store | `libs/checkpoint/langgraph/store/base/batch.py:58,283` |
| Cache abstraction | `BaseCache(ABC)` with get/set/clear (+ async), TTL-bearing values, injectable serde | `libs/checkpoint/langgraph/cache/base/__init__.py:15-48` |
| Cache implementations | `InMemoryCache` (checkpoint lib), `RedisCache` (key-prefix namespacing, SETEX TTLs), `SqliteCache` (in checkpoint-sqlite) | `libs/checkpoint/langgraph/cache/memory/__init__.py:11`; `libs/checkpoint/langgraph/cache/redis/__init__.py:10,92-100`; `libs/checkpoint-sqlite/langgraph/cache/sqlite/__init__.py:13` |
| Serializer adapter seam | `SerializerProtocol` (runtime-checkable Protocol: `dumps_typed`/`loads_typed`); `SerializerCompat` wraps legacy untyped serializers; `CipherProtocol` for encryption; `EncryptedSerializer` decorator composes cipher+serde | `libs/checkpoint/langgraph/checkpoint/serde/base.py:14-64`; `libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:8,39` |
| Embeddings normalization | `ensure_embeddings` adapts `Embeddings \| sync-fn \| async-fn \| "provider:model" string` → LangChain `Embeddings`; string form delegates to `langchain.init_embeddings` | `libs/checkpoint/langgraph/store/base/embed.py:34-106` |
| Engine accepts abstractions | `StateGraph.compile(checkpointer=..., store=..., cache=...)`; `Checkpointer = None \| bool \| BaseCheckpointSaver` with validation error naming expected types | `libs/langgraph/langgraph/graph/state.py:1179-1182,1231`; `libs/langgraph/langgraph/types.py:100-119` |
| Runtime resolution of backends | `_defaults` resolves checkpointer per invocation: `False` disables, parent-injected `CONFIG_KEY_CHECKPOINTER` wins over instance, store/cache read from runtime or fall back to compile-time values | `libs/langgraph/langgraph/pregel/main.py:2543-2609` (checkpointer `2579-2588`, store `2594-2597`, cache `2598-2601`) |
| Subgraph propagation | Config constants carry `BaseCheckpointSaver`/`BaseCache` from parent graphs into subgraphs; `Checkpointer=True` on subgraphs opts in, `None` inherits | `libs/langgraph/langgraph/_internal/_constants.py:40-44`; `libs/langgraph/langgraph/types.py:100-106` |
| Node-level access | `get_store()` reads the active store from contextvar-backed runtime; `Runtime.store` dataclass field injected into nodes/middleware | `libs/langgraph/langgraph/config.py:32-123`; `libs/langgraph/langgraph/runtime.py:113-121,203-204` |
| Multi-backend test matrix | `sync_checkpointer` fixture parametrized over memory/memory_migrate_sends/sqlite/sqlite_aes/postgres/postgres_pipe/postgres_pool; `async_checkpointer` over memory/sqlite_aio/postgres_aio(+pipe,pool); `sync_store`/`async_store` over in_memory+postgres variants; `cache` over sqlite/memory/redis — same tests run against all | `libs/langgraph/tests/conftest.py:60-89,92-141,144-188,191-226`; factories in `libs/langgraph/tests/conftest_checkpointer.py:44-239` |
| Conformance suite for third parties | `@checkpointer_test` + `validate()` produce a pass/fail report; required vs extended capabilities table; extended capabilities auto-detected via override inspection | `libs/checkpoint-conformance/README.md:27-81`; `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-91`; `validate.py:45` |
| Conformance applied in-repo | Postgres and SQLite repos run delta-channel conformance tests against their own savers | `libs/checkpoint-postgres/tests/test_conformance_delta.py`; `libs/checkpoint-sqlite/tests/test_conformance_delta.py` |
| Model provider adaptation | `create_react_agent(model=...)` union type: provider string, `LanguageModelLike`, or `(state, runtime) -> BaseChatModel` callables (sync/async); strings resolved via lazy-imported `langchain.chat_models.init_chat_model` with actionable ImportError | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-289,563-564,568-580,599-606` |
| Deployment-time externalization | `langgraph.json` top-level keys `store` and `checkpointer` passed through config; injected as `LANGGRAPH_STORE` / `LANGGRAPH_CHECKPOINTER` env vars in Dockerfile generation; relative `checkpointer.path` rewritten to container paths | `libs/cli/langgraph_cli/config.py:379,385,1135-1152,1011-1053` |
| Declarative adapter schemas | `CheckpointerConfig.path`: import string to an `@asynccontextmanager` yielding any `BaseCheckpointSaver`; nested `serde` (allowed json/msgpack modules) and thread `ttl` strategy configs; `StoreConfig.index{dims,embed,fields}` + `ttl`; `EncryptionConfig.path` for custom at-rest encryption | `libs/cli/schemas/schema.json` ($defs `CheckpointerConfig`, `StoreConfig`, `IndexConfig`, `SerdeConfig`, `TTLConfig`, `ThreadTTLConfig`, `EncryptionConfig`) |
| Env-var operational knobs | `LANGGRAPH_AES_KEY` derives an AES cipher for `EncryptedSerializer`; `LANGGRAPH_STRICT_MSGPACK` toggles strict deserialization globally | `libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:54`; `libs/checkpoint/langgraph/checkpoint/serde/_msgpack.py:12` |
| Graceful degradation guard | `apply_checkpointer_allowlist` feature-detects `with_allowlist` and warns instead of failing when a third-party saver lacks it | `libs/langgraph/langgraph/_internal/_serde.py:36-57` |
| Absent categories | No sandbox abstraction (grep for `sandbox` across `libs/**/*.py`: zero matches); no message-queue backend interface (only internal streaming primitives `AsyncQueue`/`SyncQueue`); no OpenTelemetry/tracing-sink adapter (grep `opentelemetry\|otel\|tracing_sink`: zero real matches) | searched `sources/langgraph/libs/**/*.py` |

## Answers to Dimension Questions

**1. Are backends swappable?**
Yes, within each defined family. The engine depends only on `BaseCheckpointSaver` / `BaseStore` / `BaseCache` (`libs/langgraph/langgraph/pregel/main.py:46-52`; `libs/langgraph/langgraph/graph/state.py:1179-1182`). Any subclass — first-party or user-written — can be passed to `compile()`. The docstring-level guidance confirms intent: `InMemorySaver` explicitly recommends swapping to `PostgresSaver`/`AsyncPostgresSaver` for production (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33-45`). The dimension's litmus question — *can you switch Postgres → SQLite with a config change?* — is answered **yes at the application-code level** (replace one constructor line; the rest of the graph code is unchanged because both satisfy the same contract, verified by the shared test matrix). At the *deployment* level it is also yes for the managed server via `CheckpointerConfig.path` import-string config (`libs/cli/schemas/schema.json` $defs/CheckpointerConfig). What does **not** exist is a name-based registry like `"backend": "sqlite"` inside the core libraries — selection requires referencing a concrete class or import path.

**2. Which backends have multiple implementations?**
- **Checkpointers (4+ variants)**: `InMemorySaver` (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33`), `SqliteSaver`/`AsyncSqliteSaver` (`libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py`, `aio.py:38`), `PostgresSaver`/`AsyncPostgresSaver` (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:40`, `aio.py:40`), `ShallowPostgresSaver`/`AsyncShallowPostgresSaver` retention variant (`shallow.py:169,529`).
- **Stores (3)**: `InMemoryStore` (`libs/checkpoint/langgraph/store/memory/__init__.py:136`), `SqliteStore`/`AsyncSqliteStore` (`libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:787`), `PostgresStore`/`AsyncPostgresStore` with vector index (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:667`).
- **Caches (3)**: `InMemoryCache` (`libs/checkpoint/langgraph/cache/memory/__init__.py:11`), `RedisCache` (`.../cache/redis/__init__.py:10`), `SqliteCache` (`libs/checkpoint-sqlite/langgraph/cache/sqlite/__init__.py:13`).
- **Serializers**: `JsonPlusSerializer`, `EncryptedSerializer` wrapper, `SerializerCompat` bridge for untyped legacy serializers (`libs/checkpoint/langgraph/checkpoint/serde/base.py:29-48`, `encrypted.py:8`).
- **Embeddings**: anything normalized by `ensure_embeddings` (`libs/checkpoint/langgraph/store/base/embed.py:34-106`).
- **Models**: any LangChain-compatible chat model, selected by string or instance (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:568-580`).
Queues and sandboxes: no implementations exist (no evidence found; see Absent Categories row above).

**3. Can backends be swapped at runtime?**
Partially, by design phase rather than by hot-reload:
- Per-graph-instance: `compile(checkpointer=..., store=..., cache=...)` selects backends before invocation (`libs/langgraph/langgraph/graph/state.py:1179-1182`).
- Per-invocation/config: `_defaults` reads `CONFIG_KEY_CHECKPOINTER`, `CONFIG_KEY_RUNTIME.store`, and `CONFIG_KEY_CACHE` from the runnable config, so subgraphs inherit the parent's instances and a caller can propagate different instances down a call (`libs/langgraph/langgraph/langgraph/pregel/main.py:2579-2601`; constants at `libs/langgraph/langgraph/_internal/_constants.py:40-44`).
- Dynamic model selection is fully per-call: `create_react_agent` re-resolves the model on every turn via `(state, runtime)` callable (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:599-612`).
- Deployment-time: `CheckpointerConfig.path` lets the server instantiate whatever saver the import string points to, without code changes to graphs (`libs/cli/schemas/schema.json`).
There is no mechanism to swap a live graph's checkpointer mid-run; replacement happens at construction/injection boundaries.

**4. Are adapter implementations tested?**
Extensively, on two levels.
- *Cross-implementation equivalence*: the core library's fixtures parametrize the entire test suite over memory/sqlite/postgres (plus pipeline & pool modes, AES-encrypted serde variant) synchronously and asynchronously (`libs/langgraph/tests/conftest.py:144-226`; factories `conftest_checkpointer.py:44-239`), so behavioral parity is continuously asserted rather than assumed.
- *Contract conformance*: `langgraph-checkpoint-conformance` packages `@checkpointer_test`/`validate()` as a reusable suite with required capabilities (`put`, `put_writes`, `get_tuple`, `list`, `delete_thread`) and optional ones auto-detected by checking method overrides against the base class (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:74-91`; README capability table lines 64-81). First-party consumers exist (`libs/checkpoint-postgres/tests/test_conformance_delta.py`, `libs/checkpoint-sqlite/tests/test_conformance_delta.py`).
- Store/cache backends likewise run shared suites (`test_store.py`, `test_async_store.py`, `test_ttl.py` in both sqlite and postgres libs; redis cache tested at `libs/checkpoint/tests/test_redis_cache.py`).

## Architectural Decisions

1. **Persistence isolated in a dependency-free base package.** `checkpoint` is the root of the dependency map (postgres/sqlite/prebuilt/langgraph all depend on it — declared in `AGENTS.md` and enforced by packaging), so abstractions never import engine or vendor-driver code; drivers live in leaf packages (`libs/checkpoint-postgres/pyproject.toml`, `libs/checkpoint-sqlite/pyproject.toml`).

2. **Minimal-abstract-method design.** `BaseStore` requires only `batch`/`abatch`; everything else (`get`, `search`, `put`, `delete`, `list_namespaces`) decomposes into op objects executed through the batch primitive (`libs/checkpoint/langgraph/store/base/__init__.py:732-999`). New backends implement one transactional surface instead of six endpoints.

3. **Typed-tuple serialization contract.** `dumps_typed -> (type_str, bytes)` lets storage layers persist a type tag column (see `type TEXT` in blob tables, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:57-65`) while remaining codec-agnostic; legacy untyped serializers are bridged by `SerializerCompat` (`serde/base.py:29-48`) rather than broken.

4. **Cross-cutting concerns as decorators, not base-class burden.** Encryption wraps any serializer (`EncryptedSerializer(cipher, inner)`, `serde/encrypted.py:8`); msgpack allowlists wrap any serializer via `with_allowlist` cloning (`checkpoint/base/__init__.py:713-742`); batching wraps stores (`store/base/batch.py:58`).

5. **Capability flags over fat interfaces.** Optional features are advertised (`supports_ttl`, `libs/checkpoint/langgraph/store/base/__init__.py:727`) and guarded with clear `NotImplementedError`s naming the fix (`:920-924`); the conformance suite mirrors this with detected extended capabilities (`capabilities.py:74-91`).

## Notable Patterns

- **Context-manager factories as constructors**: `from_conn_string(...)` yields configured savers with lifecycle management (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:64`; `aio.py:65`; `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py`), which is exactly the shape the server-side `CheckpointerConfig.path` expects ("async context manager yielding a `BaseCheckpointSaver`", schema.json) — factory convention and deployment schema align.
- **Sync/async twin classes sharing SQL**: Postgres keeps shared logic in `BasePostgresSaver` + `_internal.py`/`_ainternal.py` twins; SQLite shares `BaseSqliteStore` (`libs/checkpoint-sqlite/langgraph/store/sqlite/base.py:273`). Duplication is contained but real.
- **Protocol-based structural typing**: `SerializerProtocol` and `CipherProtocol` are `typing.Protocol`s (`serde/base.py:14,51`), so third-party codecs need not inherit from anything.
- **Adapter-with-fallback for optional ecosystem deps**: embedding provider strings require optional `langchain>=0.3.9`; failure raises an instructive error listing alternatives (`store/base/embed.py:83-102`), and `init_chat_model` similarly lazy-imports with an install hint (`chat_agent_executor.py:568-578`).
- **Test doubles as documentation**: fixture names (`postgres_pipe`, `sqlite_aes`) enumerate supported deployment topologies (pipelining, pooling, encryption) more concretely than prose (`conftest.py:146-160`).

## Tradeoffs

- **Explicitness vs convenience**: choosing a backend means importing a class; there is no `"database": "postgres"` shorthand in the OSS libraries. This keeps the dependency graph honest (you install only what you use — `Pipeline` warning even rejects pool+pipeline misuse at `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:54`) but shifts selection boilerplate onto users unless they deploy through the CLI/server layer.
- **Hand-written sync/async pairs** double maintenance surface and invite drift; mitigated by shared SQL modules and the common test matrix, but a change like `task_path` support touched both worlds (`MIGRATIONS` entry `base.py:90`; `put_writes(..., task_path)` signature `checkpoint/base/__init__.py:300-318`).
- **Contract complexity for implementers**: newer features (DeltaChannel history) impose subtle obligations on custom savers — the `prune` docstring must spell out silent-corruption scenarios and safe strategies in ~30 lines of warnings (`checkpoint/base/__init__.py:374-414`). The conformance suite compensates, but only if adopters run it.
- **Feature variance across equal-looking backends**: semantic search works in Postgres/pgvector and SQLite-vec-style setups but silently no-ops without `index` config ("any `index` arguments ... will have no effect", `store/base/__init__.py:716-721`); TTL is opt-in per implementation. Swapping stores is API-compatible but capability-compatible only after consulting flags/docs.

## Failure Modes / Edge Cases

- **Misuse guards**: `checkpointer=True` on a root graph fails fast ("cannot be used for root graphs", `pregel/main.py:2583-2584`); invalid checkpointer values raise `TypeError` listing valid options (`types.py:109-119`); missing `thread_id` under a checkpointer raises (`pregel/main.py:2589-2593`).
- **Version skew between packages**: `checkpoint-postgres` warns when paired with old `langgraph` minors (`postgres/base.py:27-37`); migrations are ordered and idempotent-ish (`base.py:39-91`).
- **Silent capability gaps**: unsupported TTL raises loudly (`store/base/__init__.py:920-924`), but missing vector index quietly ignores `index=` args; strict-msgpack allowlists degrade to a logged warning for exotic third-party serializers (`checkpoint/base/__init__.py:737-742`; `_internal/_serde.py:48-56`).
- **Prune/delete corruption risk for beta DeltaChannel**: documented explicitly — naive `keep_latest` pruning "will silently reconstruct as empty (no error raised)" (`checkpoint/base/__init__.py:397-401`, `delete_for_runs` note `340-347`).
- **Env-dependent test matrix**: heavy backends drop out under `NO_DOCKER` (`tests/conftest.py:39,62,96,151`), so local runs verify fewer equivalences than CI — parity guarantees are environment-contingent.

## Future Considerations

- A lightweight registry (name/class map or entry-point group) would let config-only backend selection reach library users, closing the gap between `ensure_embeddings("openai:...")` string loading (`embed.py:77-102`) and the class-instantiation requirement for savers/stores/caches.
- Generating or templating sync/async twins (or defaulting async to thread-offloaded sync, as `AsyncBatchedBaseStore` partially does for stores) would cut duplication drift.
- Promoting DeltaChannel out of beta and folding its invariants into mandatory conformance capabilities would reduce the custom-implementer hazard currently handled by docstring warnings (`checkpoint/base/__init__.py:150-170,387-414`).
- Exposing queue/sandbox seams (as other harnesses do) would extend the adapter model beyond storage; today those concerns simply do not exist in this source.

## Questions / Gaps

- **Tracing sinks**: No evidence found of a pluggable trace-export interface in this source. Searches for `opentelemetry`, `otel`, and `tracing_sink` across `libs/**/*.py` returned no relevant hits; observability appears delegated to LangChain callback machinery carried on `RunnableConfig` (e.g., config utilities re-exported from `langchain_core` at `libs/langgraph/langgraph/_internal/_config.py`; the checkpoint metadata even records `run_id` for correlation, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:61-62`) rather than a first-class sink abstraction here.
- **Queues**: Only internal in-process primitives exist (`AsyncQueue`/`SyncQueue`, `libs/langgraph/langgraph/_internal/_queue.py:12,70`); no durable/pluggable queue backend interface was found.
- **Sandboxes**: No evidence found (zero matches for `sandbox` in `libs/**/*.py`); code-execution sandboxing is out of scope for this repository.
- **Server-side consumption of `LANGGRAPH_STORE`/`LANGGRAPH_CHECKPOINTER` env vars** is emitted by the CLI (`libs/cli/langgraph_cli/config.py:1135-1152`), but the server runtime that parses them is not part of this source directory, so end-to-end behavior could not be verified here — the JSON schema definitions are the strongest in-repo evidence of the contract.
- **JS parity**: `libs/sdk-js` contains only a README stub in this snapshot; whether the TS SDK exposes equivalent backend adapters could not be assessed (no evidence found).

---

Generated by dimensions/21.02-provider-and-backend-adapters against `langgraph`.
