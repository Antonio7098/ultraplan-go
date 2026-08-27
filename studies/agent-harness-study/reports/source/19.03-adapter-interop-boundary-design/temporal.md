# Source Analysis: temporal

## 19.03 Adapter and Interop Boundary Design

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (server) / gRPC + HTTP (Nexus), SQL/Cassandra/Elasticsearch persistence |
| Analyzed | 2026-08-27 |

## Summary

Temporal treats persistence and visibility as **adapter-layer** concerns, not core protocols. Core abstractions are `DataStoreFactory` (`common/persistence/persistence_interface.go:32`), `sqlplugin.Plugin` (`common/persistence/sql/sqlplugin/interfaces.go:31`), `VisibilityStore` (`common/persistence/visibility/store/visibility_store.go`), and `archiver.History/VisibilityArchiver` (`common/archiver/interface.go:44,76`) with pluggable factories (`ArchiverProvider` + `Custom*ArchiverFactory` in `common/archiver/provider/provider.go:29,54,58`). SQL adapters register via a global map `supportedPlugins` (`common/persistence/sql/store.go:18`) and `RegisterPlugin` (`common/persistence/sql/store.go:21`) invoked from `init()` in MySQL (`common/persistence/sql/sqlplugin/mysql/plugin.go:27`), PostgreSQL/PGX (`common/persistence/sql/sqlplugin/postgresql/plugin.go:38,43`), and SQLite (`common/persistence/sql/sqlplugin/sqlite/plugin.go:45`). The higher-level `client.DataStoreFactoryProvider` (`common/persistence/client/fx.go:187`) selects SQL, Cassandra (`cassandra.NewFactory`), or a fully generic `CustomDatastoreConfig` + `AbstractDataStoreFactory` (`common/persistence/client/abstract_data_store_factory.go:16`, `common/config/config.go:282`, `common/persistence/client/fx.go:204`) enabling out-of-tree datastore addition without core edits. Visibility and archival follow the same pattern: `common/persistence/visibility/factory.go:284` branches on `SQL`/`Elasticsearch`/`CustomDataStoreConfig`, and `common/archiver/provider/provider.go:123,186` dispatches on URI scheme (`filestore`/`s3store`/`gcloud`) with custom-factory fallback. Conversely, **Nexus RPC over HTTP/gRPC is a core protocol**, not an adapter—routes are hard-wired in frontend (`service/frontend/nexus_handler.go:418`, `docs/architecture/nexus.md:22`) and use `nexus-rpc/sdk-go` (`go.mod:41`). Adapters are **compile-time/restart-time** swappable (config-driven `PluginName` in `common/config/config.go:441`, `DataStores` map in `common/config/config.go:268`), not hot-swappable. Conformance is exercised via shared test suites parameterized across backends (`common/persistence/tests/*.go`), but no dedicated per-adapter contract test harness validates `sqlplugin.Plugin`/`TableCRUD` isolation. Interop boundaries are code-documented and architecturally described in `docs/architecture/nexus.md:14-446` but lack a single adapter-contract handbook.

## Rating

**Rating: 7 / 10**

Explicit adapter interfaces, registry-based discovery, and generic extension points (`CustomDatastoreConfig`, `AbstractDataStoreFactory`, `CustomHistory/VisibilityArchiverFactory`, `VisibilityStoreFactory`) make boundaries clear and adding a new persistence/visibility/archival backend possible without patching the core interfaces. Shared persistence conformance suites run identical logic across SQL dialects/NoSQL/ES. Downgrades: SQL plugin registry is global mutable state populated by `init()` requiring recompilation to add dialect; `DataStoreFactoryProvider` still enumerates built-in kinds so a non-custom new type would touch core; adapters are not runtime-swappable and lack observability/contract-versioning at the boundary; Nexus remains core-std rather than adapterized.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core protocol abstractions | `DataStoreFactory` interface defines `NewTaskStore`, `NewShardStore`, `NewExecutionStore`, `NewMetadataStore`, etc. – central persistence abstraction | `common/persistence/persistence_interface.go:32-51` |
| Core protocol abstractions | `sqlplugin.Plugin` interface: `CreateDB(DbKind, *config.SQL, ...) (GenericDB,error)` + `GetVisibilityQueryConverter()` | `common/persistence/sql/sqlplugin/interfaces.go:31-36` |
| Core protocol abstractions | `TableCRUD` composes 20+ table interfaces (`HistoryExecution`, `MatchingTask`, `Visibility`, `NexusEndpoints`, etc.) – full SQL surface | `common/persistence/sql/sqlplugin/interfaces.go:39-77` |
| Core protocol abstractions | `VisibilityQueryConverter` interface isolates dialect-specific query compilation | `common/persistence/sql/sqlplugin/visibility_query_converter.go:35` |
| Core protocol abstractions | `HistoryArchiver`/`VisibilityArchiver` interfaces with `Archive`/`Get`/`Query`/`ValidateURI` | `common/archiver/interface.go:44-94` |
| Core protocol abstractions | `ArchiverProvider` + `CustomHistory/VisibilityArchiverFactory` define archival adapter boundary | `common/archiver/provider/provider.go:29-63` |
| Core protocol abstractions | `AbstractDataStoreFactory` allows out-of-tree `persistence.DataStoreFactory` creation from `CustomDatastoreConfig` | `common/persistence/client/abstract_data_store_factory.go:14-25` |
| Core protocol abstractions | `VisibilityStoreFactory` for pluggable visibility stores | `common/persistence/visibility/factory.go:20-31` |
| Adapter implementations | SQL plugin registry `supportedPlugins map[string]sqlplugin.Plugin` + `RegisterPlugin` (panic on dup) | `common/persistence/sql/store.go:18-26` |
| Adapter implementations | Factory helper `getPlugin` + `GetPluginVisibilityQueryConverter` resolve `PluginName` string from config | `common/persistence/sql/store.go:74-94` |
| Adapter implementations | MySQL plugin registers `mysql8` and implements `CreateDB` via `sqlplugin.NewDatabaseHandle` | `common/persistence/sql/sqlplugin/mysql/plugin.go:15-53` |
| Adapter implementations | PostgreSQL plugin registers `postgres12` + `postgres12_pgx` with alternate `driver.PQDriver`/`PGXDriver` | `common/persistence/sql/sqlplugin/postgresql/plugin.go:20-46` |
| Adapter implementations | SQLite plugin registers `sqlite` with DSN/WAL handling and `setupSQLiteDatabase` | `common/persistence/sql/sqlplugin/sqlite/plugin.go:22-50` |
| Adapter implementations | SQL `Factory` implements `DataStoreFactory` (NewTaskStore, NewShardStore, NewExecutionStore, NewQueueV2, etc.) | `common/persistence/sql/factory.go:17,72,117,136,144` |
| Adapter implementations | `VisibilityQueryConverter` per dialect asserted via `var _ sqlplugin.VisibilityQueryConverter = (*queryConverter)(nil)` | `common/persistence/sql/sqlplugin/mysql/query_converter.go:60`, `common/persistence/sql/sqlplugin/postgresql/query_converter.go:40`, `common/persistence/sql/sqlplugin/sqlite/query_converter.go:23` |
| Adapter implementations | `newVisibilityStoreFromDataStoreConfig` branches `SQL` -> `sql.NewSQLVisibilityStore`, `Elasticsearch` -> `elasticsearch.NewVisibilityStore`, `Custom` -> factory | `common/persistence/visibility/factory.go:237-300` |
| Adapter implementations | `archiverProvider.GetHistory/VisibilityArchiver` switch on `filestore.URIScheme`/`gcloud`/`s3store` + custom factory fallback with caching | `common/archiver/provider/provider.go:123-170,186-245` |
| Adapter implementations | Frontend `nexusHandler.StartOperation` + `DispatchNexusTask` – Nexus is core HTTP/gRPC handler, not adapter | `service/frontend/nexus_handler.go:418-470`, `docs/architecture/nexus.md:20-31` |
| Plugin/extension points | `DataStores map[string]DataStore` with `DataStore.SQL/Cassandra/CustomDataStoreConfig/Elasticsearch` union + `Validate()` enforces exactly one | `common/config/config.go:268-285,156-174` |
| Plugin/extension points | `CustomDatastoreConfig{Name, IndexName, Options}` generic bag for external plugins | `common/config/config.go:467-479` |
| Plugin/extension points | `Persistence.DefaultStore`/`VisibilityStore`/`SecondaryVisibilityStore` config keys select adapters at boot | `common/config/config.go:259-263` |
| Plugin/extension points | `SQL.PluginName` selects dialect (`mysql8`, `postgres12`, etc.) | `common/config/config.go:441` |
| Plugin/extension points | `DataStoreFactoryProvider` Fx provider switches on `defaultStoreCfg.Cassandra/SQL/CustomDataStoreConfig`, wraps with fault-injection/telemetry decorators | `common/persistence/client/fx.go:187-219` |
| Plugin/extension points | Archiver URIs: `HistoryArchiverProvider.Filestore/Gstorage/S3store + CustomStores map[string]map[string]any` | `common/config/config.go:514-541`, `common/archiver/provider/provider.go:72-73` |
| Conformance tests | Shared persistence test suites instantiated per backend: `mysql_test.go`, `postgresql_test.go`, `sqlite_test.go`, `cassandra_test.go` delegate to `tests/*.go` | `common/persistence/tests/mysql_test.go:86`, `common/persistence/tests/postgresql_test.go`, `common/persistence/tests/sqlite_test.go:46`, `common/persistence/tests/cassandra_test.go`, `common/persistence/tests/history_store.go`, `common/persistence/tests/queue_v2_test_suite.go` |
| Conformance tests | Visibility SQL store tests including unified query converter tests | `common/persistence/visibility/store/sql/query_converter_test.go`, `common/persistence/visibility/store/sql/visibility_store_test.go`, `common/persistence/visibility/store/elasticsearch/converter_test.go` |
| Conformance tests | Archiver suites per backend (`filestore/history_archiver_test.go:45`, `s3store`, `gcloud`) | `common/archiver/filestore/history_archiver_test.go:45`, `common/archiver/s3store/history_archiver_test.go`, `common/archiver/gcloud/history_archiver_test.go` |
| Conformance tests | `MockVisibilityQueryConverter` generated via mockgen for converter conformance | `common/persistence/sql/sqlplugin/visibility_query_converter_mock.go:20` |
| Protocol documentation | Nexus RPC architecture doc covering HTTP routes, endpoint registry, outbound queue, state machines | `docs/architecture/nexus.md:1-446` |
| Protocol documentation | Nexus over HTTP spec reference via `nexus-rpc/sdk-go` and `nexus-proto-annotations` in `go.mod` | `go.mod:41`, `go.sum:328`, `docs/architecture/nexus.md:16-18` |
| Operational safeguards | `DbConn` reference-counted connection with `ForceClose`/`Close` and retry/rate-limiting wrappers around managers | `common/persistence/sql/factory.go:162-217`, `common/persistence/client/factory.go:109-122` |
| Operational safeguards | `healthSignalAggregator` + `persistenceRateLimitedClients` + `persistenceMetricsClients` decorating every manager | `common/persistence/client/factory.go:54-64,109-259`, `common/persistence/client/fx.go:115-161` |

## Answers to Dimension Questions

**1. Are protocols core or adapter-layer?**
Both. **Persistence/visibility/archival are adapter-layer**: core only defines abstract factories/interfaces (`DataStoreFactory` `common/persistence/persistence_interface.go:32`, `sqlplugin.Plugin` `common/persistence/sql/sqlplugin/interfaces.go:31`, `VisibilityStore` `common/persistence/visibility/store/visibility_store.go`, `History/VisibilityArchiver` `common/archiver/interface.go:44,76`) with multiple interchangeable backends. **Nexus RPC is core**: HTTP/gRPC Nexus routes (`/nexus/endpoints/{endpoint}/services`, `/namespaces/{namespace}/nexus/callback` in `docs/architecture/nexus.md:22-33` and `service/frontend/nexus_handler.go:418`) and gRPC history/matching services are baked into `service/frontend/fx.go:13,140` and `api/historyservice`. Idempotency, retry, and circuit-breaker policies for Nexus are core constants, not pluggable protocols.

**2. Can adapters be added without core changes?**
**Yes for bounded extension, with caveats.** 
- *Yes via generic escape hatches*: A new persistence store can be added by implementing `AbstractDataStoreFactory` (`common/persistence/client/abstract_data_store_factory.go:16`) and pointing `DataStore.CustomDataStoreConfig` (`common/config/config.go:282`) at it—`DataStoreFactoryProvider` (`common/persistence/client/fx.go:204`) handles it with no core edits. Visibility (`VisibilityStoreFactory` `common/persistence/visibility/factory.go:284`) and archival (`CustomHistoryArchiverFactory` `common/archiver/provider/provider.go:54`) expose analogous scheme-factory hooks.
- *Barely yes for new SQL dialect*: Implement `sqlplugin.Plugin` (`common/persistence/sql/sqlplugin/interfaces.go:31`) + `TableCRUD` (~20 methods) and call `sql.RegisterPlugin` in `init()` (`common/persistence/sql/sqlplugin/mysql/plugin.go:27`). No edit to `common/persistence/sql/store.go:21` itself, but the dialect is selected by string `SQL.PluginName` (`common/config/config.go:441`) and must be imported into the binary (no discovery), so rollout requires a new build. Adding a first-class non-SQL, non-Cassandra kind without using `Custom` would require editing the `switch` in `common/persistence/client/fx.go:199-205`.

**3. Are adapters tested for conformance?**
**Partially.** The strongest conformance is the shared persistence test suites (`common/persistence/tests/history_store.go`, `queue_v2_test_suite.go`, `visibility_persistence_suite.go`, etc.) run against each backend (`mysql_test.go`, `postgresql_test.go`, `sqlite_test.go`, `cassandra_test.go:23`), plus visibility query converter tests per dialect and archiver suites per backend. However, there is **no single canonical `Plugin` contract test** that asserts every `TableCRUD` method handles `IsDupEntryError`, transactions, and visibility token pagination identically, and mock files (`sqlplugin/visibility_query_converter_mock.go:20`) are used for unit isolation rather than cross-adapter equivalence. Fault-injection (`common/persistence/faultinjection/`) is injected uniformly but not gated as a per-adapter compliance step.

**4. Are interop boundaries documented?**
**In-code yes, handbook no.** Interface godoc is thorough (`DataStoreFactory` comment `common/persistence/persistence_interface.go:30`, `Plugin`/`TableCRUD` doc, `ArchiverProvider` `common/archiver/provider/provider.go:27`). `docs/architecture/nexus.md:14-446` exhaustively documents Nexus interop (HTTP routes, registry replication, outbound queue, callback lifecycle, circuit breaker, scheduler). No dedicated `docs/architecture/persistence-adapter.md` exists enumerating the `SQL` vs `Cassandra` vs `Custom` boundary, plugin registration, `VisibilityQueryConverter` contract, or versioning/upgrade guidance beyond `config/persistence.go:59-95` validation comments and `AGENTS.md`/`common/persistence/sql/sqlplugin/interfaces.go` inline docs.

## Architectural Decisions

* **Global `init()`-registered SQL plugin map** (`common/persistence/sql/store.go:18-21`) — Pro: zero config indirection for built-ins, dialects self-register; Con: global mutable state, import-side-effect coupling, requires recompilation to add dialect, test ordering sensitivity. Verification: `grep RegisterPlugin` shows only `mysql`, `postgresql`, `sqlite` callers.
* **Adapter composition via `TableCRUD` (20 sub-interfaces)** (`common/persistence/sql/sqlplugin/interfaces.go:39-77`) — Pro: single `DB`/`Tx` covers all persistence; Con: violating ISP—every dialect must stub even unused tables (e.g., `HistoryChasm` on MySQL), large blast radius for interface changes.
* **Generic `CustomDatastoreConfig` + `AbstractDataStoreFactory`** (`common/config/config.go:467`, `common/persistence/client/abstract_data_store_factory.go:14`) — Pro: true out-of-tree extensibility without forking core; Con: `Options map[string]any` is untyped, no schema validation, observability delegated to external factory.
* **Dual visibility stores with primary/secondary roles + Elasticsearch custom index** (`common/config/persistence.go:59-95`, `common/persistence/visibility/factory.go:33-127`) — Pro: enables advanced-SQL ↔ ES dual-read migration; Con: validation complexity (rejects mixed SQL/ES dual writes `common/config/persistence.go:66`).
* **Scheme-dispatched archiver provider with custom override** (`common/archiver/provider/provider.go:148-170`) — Pro: `ErrUnknownScheme` fallback lets custom factories intercept before built-ins; Con: provider caches forever (`sync.RWMutex` `common/archiver/provider/provider.go:69-84`) so scheme config is not dynamically reloadable.
* **Nexus as core HTTP/gRPC, not pluggable transport** (`docs/architecture/nexus.md:20`, `service/frontend/nexus_handler.go:346`) — Pro: single strongly consistent registry, coherent retry/callback semantics; Con: cannot swap in e.g. NATS adapter without core rewrite.

## Notable Patterns

* **Registry + string-keyed factory**: `PluginName` string → `supportedPlugins` map; `URI scheme` → archiver; `QueueType`+`QueueName` → `QueueV2` (`common/persistence/persistence_interface.go:809`). Uniform but leaky (typos fail at runtime with `ErrPluginNotSupported` `common/persistence/sql/store.go:16,79`).
* **Decorator stack per manager**: `TaskManager → RateLimited → Metrics → Retryable` (`common/persistence/client/factory.go:114-122`). Applied consistently across 7 managers in `common/persistence/client/fx.go:69-77`.
* **Fx module wiring for persistence**: `DataStoreFactoryProvider` → `Factory` → individual `*Manager` providers with lifecycle hooks (`common/persistence/client/fx.go:66,222`).
* **Dialect-specific query converters**: Each SQL plugin supplies its own `VisibilityQueryConverter` (`mysql/query_converter.go:60`, `postgresql/query_converter.go:40`, `sqlite/query_converter.go:23`) to handle `LIKE`/`JSON`/`datetime` differences, accessed via `sql.GetPluginVisibilityQueryConverter` (`common/persistence/sql/store.go:89`).
* **RefCounted DB handle for SQLite in-mem**: `DbConn` + `connPool` (`common/persistence/sql/sqlplugin/sqlite/plugin.go:42,64`) prevents last-connection close from destroying DB—adapter-specific lifecycle workaround surfaced as core concern.

## Tradeoffs

* **Extensibility vs type safety**: `CustomDatastoreConfig.Options map[string]any` and `Archiver CustomStores map[string]map[string]any` maximize flexibility but defer validation to runtime/factory implementation; typos or schema drift not caught at config parse except for `DataStore.Validate()` cardinality check (`common/config/persistence.go:170`).
* **Coverage vs surface area**: `TableCRUD` forces full fidelity but makes adding a lightweight adapter (e.g., read-only archival) heavy; `QueueV2` migration (`common/persistence/persistence_interface.go:798-835`) exists precisely because original `Queue` (`common/persistence/persistence_interface.go:169`) assumed single queue per type (Cassandra `queue_metadata` primary key limitation).
* **Build-time vs runtime extensibility**: `init()` registration is simple and dependency-free but prevents hot-loading; `AbstractDataStoreFactory` is injected via `ServerOption.WithCustomDataStoreFactory` (`temporal/server_option.go:147`, `temporal/fx.go:112`) so binary must be rebuilt to wire it.
* **Consistency vs availability of adapter catalog**: `archiverProvider` caches archivers forever—fast and idempotent, but config changes after first `GetHistoryArchiver` are invisible without restart; similarly `supportedPlugins` is never deregistered.
* **Core Nexus guarantees cost**: Outbound queue multi-cursor + `GroupByScheduler` + circuit breaker (`docs/architecture/nexus.md:130-233`) provide per-destination isolation but anchor Nexus to HTTP semantics; swapping transport would require reimplementing callback-token (`common/nexus/callback_token.go`) and endpoint registry (`common/nexus/endpoint_registry.go:??`) assumptions.

## Failure Modes / Edge Cases

* **Unknown plugin string**: `cfg.PluginName` typo returns `ErrPluginNotSupported: unknown plugin "foo", supported: [mysql8 postgres12 ...]` (`common/persistence/sql/store.go:79-84`)—only surfaced at `NewSQLDB`/`GetDB` (`common/persistence/sql/factory.go:185`) on startup, not at config validation for SQL (only `DataStore.Validate()` checks exclusivity).
* **Duplicate `RegisterPlugin`**: panic (`common/persistence/sql/store.go:23`) if two plugins share `PluginName`—global state collision in tests importing both.
* **Reference-count leak**: `DbConn.Get()` increments `refCnt` (`common/persistence/sql/factory.go:191`) but `Close()` must be called symmetrically; mismatched `Close`/`ForceClose` can leak `sqlx.DB` or close in-use pooled conn.
* **Custom factory misconfiguration**: `temporal/fx.go:324` injects `CustomDataStoreFactory` but code path only taken when `defaultStoreCfg.CustomDataStoreConfig != nil` (`common/persistence/client/fx.go:204`); if operator configures `CustomDataStoreConfig` yet forgets `WithCustomDataStoreFactory` option, factory is nil and will panic.Fatal.
* **Visibility dual-write mismatch**: `common/config/persistence.go:67` rejects mixing SQL and ES dual visibility, but allows SQL↔SQL or ES↔ES; secondary visibility not configured returns `nil` manager (`common/persistence/visibility/factory.go:220`)—callers must nil-check.
* **Dialect SQL injection via query converter**: Each `VisibilityQueryConverter` builds raw SQL strings; missing validation in one dialect (e.g., MySQL `query_converter.go`) can emit invalid syntax for edge queries that pass elsewhere.
* **Nexus endpoint rename while workflows inflight**: `docs/architecture/nexus.md:116` warns renaming endpoint by name causes stuck workflow task retry loop—registry is eventual-consistency via long-poll to matching owner; version monotonicity (`endpoints table versioned` `docs/architecture/nexus.md:123`) guards serialization but not orphaned operations.

## Future Considerations

* Introduce a typed `PluginRegistry` with explicit `Unregister`/`List` and compile-time generation instead of global `init()` map to enable hermetic testing and hot-reload.
* Promote `CustomDatastoreConfig.Options` to a validated struct or `proto.Struct` with versioned adapter contract (`Adapter APIVersion`) and conformance vetting in CI (run `common/persistence/tests/*` as contract harness for custom plugins).
* Add per-adapter health/observability dimensions at the boundary (`PluginName` + `DbKind` tags already emitted in visibility manager `common/persistence/visibility/factory.go:229`—extend to all managers and expose `IsDupEntryError`/`IsConnNeedsRefreshError` counters per dialect).
* Unify `Queue` vs `QueueV2` migration path and deprecate `Queue` to shrink `TableCRUD` surface; likewise narrow `TableCRUD` via smaller role interfaces so a new archival-only adapter need not implement `HistoryReplicationDLQTask`.
* Extract Nexus transport behind a ` NexusTransport` interface (currently core HTTP) to allow adapterized transports for isolated failure domains without forking `service/frontend/nexus_handler.go:347`.
* Document persistence adapter boundary in `docs/architecture/` with sequence for adding a new SQL dialect or custom datastore (config snippet, factory wiring via `WithCustomDataStoreFactory`, required `TableCRUD` checklist, test command `go test -tags test_dep ./common/persistence/tests -run ...`).

## Questions / Gaps

* **No evidence of hot-swap**: Search for dynamic `Plugin` reload or `supportedPlugins` mutation after startup found none—confirmed restart-required. Runtime swappability question answered as “no”.
* **Adapter versioning**: No `Plugin` API version field in `common/persistence/sql/sqlplugin/interfaces.go:31`—unclear how breaking `TableCRUD` changes are coordinated across dialect plugins.
* **No central adapter handbook**: `docs/architecture/` covers Nexus and history matching but not `sqlplugin.Plugin` contract; only inline godoc and `common/config/config.go:441` comment exist.
* **Cassandra plugin boundaries**: `common/persistence/cassandra` (and `nosql` hierarchy) not inspected in depth for this run; evidence limited to `common/persistence/client/fx.go:201` reference—full Cassandra `TableCRUD`-equivalent interface location not confirmed.
* **Conformance automation**: Unclear whether CI runs the full `common/persistence/tests/*` matrix against all SQL targets on every PR (only local `make unit-test` found in `AGENTS.md`); no CI workflow file inspected.

---

Generated by `19.03-adapter-and-interop-boundary-design` against `temporal`.
