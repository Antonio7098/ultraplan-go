# Source Analysis: temporal

## Dimension 21.02: Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server; gRPC, uber/fx DI, sqlx, gocql, OpenTelemetry, tally) |
| Analyzed | 2026-08-25 |

## Summary

Temporal's server is architected around a strict, multi-layer provider/adapter model for every durable backend it touches. The core abstraction is `persistence.DataStoreFactory` (`common/persistence/persistence_interface.go:32-51`), which vends nine fine-grained store interfaces (shard, task, fair task, metadata, cluster metadata, execution, queue v1/v2, nexus endpoint). Three factory families implement it — Cassandra (`common/persistence/cassandra/factory.go:31`), SQL over four registered dialect plugins (mysql8, postgres12, postgres12_pgx, sqlite; `common/persistence/sql/sqlplugin/postgresql/plugin.go:22-23`, `common/persistence/sql/sqlplugin/mysql/plugin.go:17`), and a custom extension point (`common/persistence/client/abstract_data_store_factory.go:14-25`). Backend selection happens at process startup from externalized YAML config via `DataStoreFactoryProvider` (`common/persistence/client/fx.go:187-220`), and the chosen backend is then uniformly wrapped in decorator middleware: rate limiting, metrics/health signals, retries, OpenTelemetry tracing, and fault injection (`common/persistence/client/factory.go:109-260`, `common/persistence/client/fx.go:210-217`). A parallel adapter layer exists for visibility stores (SQL vs Elasticsearch vs custom; `common/persistence/visibility/factory.go:255-300`) with genuine runtime read/write routing between two configured backends driven by dynamic config (`common/persistence/visibility/manager_selector.go:43-71`), and for archivers selected per-namespace URI scheme at request time (filestore/gcloud/S3 plus custom factories; `common/archiver/provider/provider.go:123-184`). Metrics sinks are themselves swappable (tally vs OpenTelemetry over statsd or Prometheus; `common/metrics/config.go:462-493`), as are OTLP tracing exporters (`common/telemetry/config.go:222-249`). The design is validated by large shared conformance test suites that run unchanged against every persistence backend (`common/persistence/persistence-tests/setup.go:34-52`). There is no evidence of LLM/model providers, vector databases, or sandbox adapters — those domains do not exist in this codebase.

## Rating

**9 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- **Explicit interfaces at three layers**: datastore factory + store interfaces (`common/persistence/persistence_interface.go:32-51`), SQL dialect plugin interface (`common/persistence/sql/sqlplugin/interfaces.go:32-36`), visibility store interface (`common/persistence/visibility/store/visibility_store.go:19-27`).
- **Multiple proven implementations per abstraction**, including an out-of-tree extension path (`AbstractDataStoreFactory`, `common/persistence/client/abstract_data_store_factory.go:14-25`).
- **Uniform operational safeguards regardless of backend**: retryable/rate-limited/metrics decorators applied identically to all stores (`common/persistence/client/factory.go:109-260`), fault-injection wrapper with targeted per-store/per-method config (`common/persistence/faultinjection/data_store_factory.go:25-33`; example config `common/config/config.go:287-307`), OTel tracing decorator (`common/persistence/telemetry/data_store_factory.go:27-37`).
- **Externalized, validated configuration** (`common/config/persistence.go:35-96`, `156-196`) with per-backend sample YAML files.
- **Adapter implementations tested**: shared suites run against Cassandra, MySQL, PostgreSQL, SQLite, and Elasticsearch (`common/persistence/tests/cassandra_test.go`, `mysql_test.go`, `postgresql_test.go`, `sqlite_test.go`, `elasticsearch_test.go`), parameterized by CLI flags in CI (Makefile:519-528).

Why not 10:
- Primary datastore swap is startup-only; no hot-swap or in-repo live migration tooling (see Tradeoffs).
- Minor abstraction leakage: a Cassandra-specific optimization field sits on the generic interface (see Failure Modes).
- Custom backends require compile-time wiring through fx rather than runtime plugin loading.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Backend abstraction (core) | `DataStoreFactory` interface vending TaskStore/FairTaskStore/ShardStore/MetadataStore/ExecutionStore/Queue/QueueV2/ClusterMetadataStore/NexusEndpointStore; doc comment states "actual datastore is implementation detail hidden behind this interface" | common/persistence/persistence_interface.go:32-51 |
| Store interfaces | ShardStore, TaskStore, MetadataStore, ClusterMetadataStore, ExecutionStore, Queue, NexusEndpointStore, QueueV2 interfaces | common/persistence/persistence_interface.go:54-61, 64-83, 86-97, 102-113, 116-167, 170-185, 188-195, 809-835 |
| Manager-level factory | `Factory` interface ("can vend persistence layer objects backed by a datastore") implemented by `factoryImpl` | common/persistence/client/factory.go:24-48, 50-65 |
| Runtime selection (primary) | `DataStoreFactoryProvider` switch: `Cassandra != nil` → cassandra.NewFactory; `SQL != nil` → sql.NewFactory; `CustomDataStoreConfig != nil` → abstract factory; fatal error otherwise | common/persistence/client/fx.go:197-208 |
| Decorator chain (telemetry/fault injection) | Fault-injection factory wraps base factory when configured; OTel telemetry factory wraps when tracer enabled | common/persistence/client/fx.go:210-217 |
| SQL dialect plugins | Registry map + `RegisterPlugin` (panics on duplicate); lookup by `cfg.PluginName` with sorted supported-list error | common/persistence/sql/store.go:18-26, 74-87 |
| mysql8 plugin | `PluginName = "mysql8"`, `var _ sqlplugin.Plugin = (*plugin)(nil)`, registration in `init()` | common/persistence/sql/sqlplugin/mysql/plugin.go:15-30 |
| postgres12 + pgx plugins | Two plugin names registered from one implementation | common/persistence/sql/sqlplugin/postgresql/plugin.go:21-43 |
| sqlite plugin | `PluginName = "sqlite"` (pure-Go modernc.org/sqlite driver) | common/persistence/sql/sqlplugin/sqlite/plugin.go:23-24; driver.go:7-13 |
| SQL plugin contract | `sqlplugin.Plugin` interface (`CreateDB`, `GetVisibilityQueryConverter`) plus TableCRUD/DB/AdminDB contracts each dialect must satisfy | common/persistence/sql/sqlplugin/interfaces.go:32-36, 39-77, 101-114 |
| SQL factory | `sql.Factory` implements all DataStoreFactory methods against one ref-counted connection | common/persistence/sql/factory.go:16-60, 72-156 |
| Cassandra factory | Parallel `NewFactory` implementing same DataStoreFactory | common/persistence/cassandra/factory.go:31 |
| Custom datastore SPI | `AbstractDataStoreFactory` — "can be used to implement custom datastore support outside of the Temporal core" | common/persistence/client/abstract_data_store_factory.go:14-25 |
| Visibility abstraction | `VisibilityStore` interface with `GetName()`/`GetIndexName()`; `VisibilityStoreFactory` SPI for custom stores | common/persistence/visibility/store/visibility_store.go:19-27; factory.go:20-31 |
| Visibility implementations | Selection by config union: SQL → `sql.NewSQLVisibilityStore`; Elasticsearch → `elasticsearch.NewVisibilityStore`; custom → injected factory | common/persistence/visibility/factory.go:255-300 |
| Dual visibility runtime routing | `writeManagers()` switches off/on/dual by dynamicconfig string; `readManager(ns)` picks primary vs secondary by namespace-filtered bool | common/persistence/visibility/manager_selector.go:43-57, 59-71 |
| Shadow reads | `VisibilityManagerDual` with shadow-read context, enabled by dynamicconfig | common/persistence/visibility/visibility_manager_dual.go:20-46 |
| Visibility validation rules | Allowed/forbidden primary-secondary combos documented and enforced (e.g., cannot mix advanced-SQL and ES) | common/config/persistence.go:44-84 |
| Archiver abstractions | `HistoryArchiver` (Archive/Get/ValidateURI) and `VisibilityArchiver` (Archive/Query/ValidateURI); URI-based dispatch | common/archiver/interface.go:44-59, 76-94 |
| Archiver provider | Scheme-keyed cache; custom factory consulted first (returns `ErrUnknownScheme` to fall back); built-ins: filestore, gstorage, s3store | common/archiver/provider/provider.go:29-32, 123-184, 213-234 |
| Archiver schemes | `URIScheme = "s3"` / `"gs"` / `"file"` constants | common/archiver/s3store/history_archiver.go:32; common/archiver/gcloud/history_archiver.go:29; common/archiver/filestore/history_archiver.go:38 |
| Per-request scheme resolution | History archival resolves provider by `request.HistoryURI.Scheme()` at runtime | service/history/archival/archiver.go:187 |
| Middleware decorators (persistence) | Each manager wrapped rate-limited → metrics(+health signals) → retryable, uniformly for task/shard/metadata/clusterMD/execution/queue/nexus | common/persistence/client/factory.go:109-123, 142-158, 160-176, 178-194, 196-219, 221-235, 245-260 |
| Visibility middleware | Rate limiter + metrics wrappers around any VisibilityManager impl | common/persistence/visibility/factory.go:150-173 |
| Tracing sink decorator | `TelemetryDataStoreFactory` lazily wraps every store type with an OTel tracer | common/persistence/telemetry/data_store_factory.go:10-37, 43-52 |
| Fault injection decorator | `FaultInjectionDatastoreFactory` wraps each store type; per-store targeting via `Targets.DataStores[storeName]` | common/persistence/faultinjection/data_store_factory.go:25-33, 200-206; config schema common/config/config.go:287-314 |
| Externalized config (union) | `DataStore` struct holds exactly one of Cassandra/SQL/CustomDatastoreConfig/Elasticsearch (+FaultInjection); `Validate()` enforces "one and only one datastore" | common/config/config.go:273-285; common/config/persistence.go:156-175 |
| Config keys | `defaultStore`, `visibilityStore`, `secondaryVisibilityStore` select named entries from `datastores` map | common/config/config.go:258-268 |
| Sample configs | Postgres dev config (`pluginName: "postgres12"`) vs SQLite dev config (`pluginName: "sqlite"`, memory mode) vs Cassandra+ES | config/development-postgres12.yaml:6,12; config/development-sqlite.yaml:6,14,18-21; config/development-cass-es.yaml:6 |
| Credential rotation support | `passwordCommand` executes external command for short-lived DB creds; `RefreshingConnector` rebuilds DSN per new physical connection | common/config/persistence.go:301-323; common/persistence/sql/sqlplugin/connector.go:8-39 |
| Metrics sink adapters | `MetricsHandlerFromConfig`: statsd/prometheus × tally/OpenTelemetry matrix, fallback to tally | common/metrics/config.go:89-90, 102, 182-187, 462-493 |
| Tracing sink config | OTLP-gRPC span/metric exporter specs parsed into otel options; unsupported types rejected with explicit error | common/telemetry/config.go:100-152, 222-249 |
| Adapter tests (conformance) | `GetTestClusterOption(storeType, driver)` maps flags to per-backend test clusters (cassandra/mysql/postgres/pgx/sqlite) | common/persistence/persistence-tests/setup.go:34-52 |
| Adapter tests (sqlite matrix) | ~50 suite instantiations reusing shared suites (ExecutionMutableState, HistoryV2, MetadataV2, Queue, ClusterMetadata, Nexus, QueueV2, visibility, sqlplugin table suites) on memory+file sqlite | common/persistence/tests/sqlite_test.go:98-136, 633-702, 706-1109, 1527-1565 |
| Adapter tests (other backends) | Same suites bound to cassandra/mysql/postgres/elasticsearch drivers | common/persistence/tests/cassandra_test.go; mysql_test.go; postgresql_test.go; elasticsearch_test.go |
| Adapter tests (wrappers) | Factory error propagation test using MockDataStoreFactory; fault-injection factory tests; provider caching/custom-precedence tests | common/persistence/client/factory_test.go:34-38; common/persistence/faultinjection/store_fault_generator_test.go:23-28, 94, 147; common/archiver/provider/provider_test.go:117-122 |
| Test harness flags | `-persistenceType` (sql/nosql) and `-persistenceDriver` CLI flags drive functional tests; Makefile targets run matrix incl. fault injection and sqlite leakcheck | tests/testcore/flag.go:14-36; Makefile:40, 519-528, 550 |

## Answers to Dimension Questions

1. **Are backends swappable?**
   Yes, comprehensively — but scoped by subsystem. The primary persistence backend is swappable at startup purely by configuration: `defaultStore` names an entry in the `datastores` map whose payload selects Cassandra, SQL (any registered dialect), or a custom factory (`common/persistence/client/fx.go:198-208`; config keys at `common/config/config.go:258-268`). This directly answers the rubric question "Can you switch from Postgres to SQLite with a config change?": yes — compare `config/development-postgres12.yaml:12` (`pluginName: "postgres12"`) with `config/development-sqlite.yaml:14` (`pluginName: "sqlite"`); both flow through the identical `sql.NewFactory` code path (`common/persistence/sql/factory.go:45`). Caveats: the swap requires a restart, loads the matching schema directory (e.g., `schema/postgresql/v12` vs `schema/sqlite/v3`, `common/persistence/persistence-tests/setup.go:20,24,30`), and does not migrate existing data. Visibility backends are independently swappable (SQL/ES/custom, `common/persistence/visibility/factory.go:255-300`), archivers by namespace URI scheme (`service/history/archival/archiver.go:187`), and metrics/tracing sinks by config framework strings (`common/metrics/config.go:462-493`).

2. **Which backends have multiple implementations?**
   - Core persistence: Cassandra, SQL (mysql8, postgres12, postgres12_pgx, sqlite), plus custom out-of-tree factories (`common/persistence/cassandra/factory.go:31`; `common/persistence/sql/store.go:18-26`; `abstract_data_store_factory.go:14-25`).
   - Visibility: standard SQL, Elasticsearch, custom (`common/persistence/visibility/factory.go:259-299`), combinable as primary+secondary pairs within validated constraints (`common/config/persistence.go:44-84`).
   - Archivers: filestore, Google Cloud Storage, S3, plus custom history/visibility archiver factories with built-in fallback (`common/archiver/provider/provider.go:131-171, 197-234`).
   - Metrics sinks: tally or OpenTelemetry over statsd or Prometheus (`common/metrics/config.go:470-492`).
   - Tracing sinks: pluggable OTLP-gRPC exporter list (`common/telemetry/config.go:122, 222-229`).
   - Queues have no external-broker adapters: `Queue`/`QueueV2` are always backed by the selected datastore (`common/persistence/persistence_interface.go:170-185, 809-835`; both SQL and Cassandra provide implementations).

3. **Can backends be swapped at runtime?**
   Partially, and only where explicitly designed. The primary datastore is fixed for the life of the process — `DataStoreFactoryProvider` runs once during fx initialization and there is no reload mechanism. The clearest runtime swapping exists in dual visibility: write target (off/on/dual broadcast) and read preference (including namespace-scoped reads and shadow-read mode) are re-evaluated per call through dynamicconfig functions (`common/persistence/visibility/manager_selector.go:43-71`; `visibility_manager_dual.go:20-46`). Archivers are resolved per request from the namespace's stored URI scheme with provider-side memoization (`service/history/archival/archiver.go:187`; cache at `provider.go:124-129`), so different namespaces can use different archive sinks concurrently. Rate-limit QPS knobs around adapters are also dynamic-config-driven (`common/persistence/client/fx.go:118-144`).

4. **Are adapter implementations tested?**
   Yes, this is a standout strength. Shared behavioral suites (`ExecutionMutableStateSuite`, `HistoryV2PersistenceSuite`, `MetadataPersistenceSuiteV2`, `ClusterMetadataManagerSuite`, `QueuePersistenceSuite`, `NexusEndpointTestSuite`, `QueueV2` suite, `VisibilityPersistenceSuite`) are instantiated once per backend driver — e.g., the SQLite file binds them in ~50 test functions covering memory and file modes (`common/persistence/tests/sqlite_test.go:98-136, 633-702, 1547-1565`), with sibling bindings for Cassandra/MySQL/PostgreSQL/ES in the same package. Below that, per-table sqlplugin conformance suites run per dialect (`sqlite_test.go:706-1109` invoking `sqltests.New*Suite`). Wrapper/decorator logic has its own unit tests (`client/factory_test.go:34-38`, `faultinjection/store_fault_generator_test.go:23-28`), and the whole stack is exercised end-to-end by functional tests parameterized over `-persistenceType`/`-persistenceDriver` including a fault-injection-enabled pass (`Makefile:519-528`).

## Architectural Decisions

1. **Two-tier factory design**: a low-level `DataStoreFactory` per backend technology, and a higher-level `Factory` that composes stores into managers while injecting cross-cutting concerns (serialization, XDC cache, rate limiters, health signals). The doc comment makes the intent explicit: "The actual datastore is implementation detail hidden behind this interface" (`common/persistence/client/factory.go:23-48`).

2. **Config-union discriminator instead of registry for top-level backends**: the top level uses a `switch` over which config section is non-nil (cassandra/sql/custom) with a fatal default (`common/persistence/client/fx.go:199-208`), while the SQL sub-layer uses a name-based plugin registry populated via `init()` side effects (`common/persistence/sql/store.go:21-26`; import-for-registration shown at `common/persistence/tests/sqlite_test.go:27`). New SQL dialects need no core changes; new top-level technologies require touching the switch.

3. **Decorator pipeline as the universal adapter seam**: every backend gets identical resilience behavior because decorators wrap the interface, not the implementation — rate limiting, metrics with latency health signals, retries keyed on transient-error classification (`IsPersistenceTransientError` treats Unavailable/DataLoss as retryable, `common/persistence/client/factory.go:270-278`), OTel tracing (`common/persistence/client/fx.go:214-217`), and opt-in fault injection (`common/persistence/client/fx.go:210-212`).

4. **Internal requests/responses decoupled from wire types**: store methods operate on `Internal*Request/Response` structs carrying `commonpb.DataBlob`s, so serialization format changes don't ripple across backends (`common/persistence/persistence_interface.go:122-166, 336-503`).

5. **URI-scheme dispatch for open-ended sinks**: archival uses `scheme://` URIs stored per namespace; the provider maps scheme→implementation and allows a custom factory to preempt built-ins by returning non-nil, falling back on `ErrUnknownScheme` (`common/archiver/provider/provider.go:51-63, 148-171`).

6. **Dual-write/read-select pattern for migrations**: secondary visibility store support with writing modes (off/on/dual) and namespace-scoped read switching gives operators a runtime-controlled cutover path between visibility backends without redeployment (`manager_selector.go:43-71`), with invalid combinations rejected at config load (`common/config/persistence.go:44-84`).

## Notable Patterns

- **Abstract Factory + Strategy**: `DataStoreFactory` families (`common/persistence/client/factory.go:24-48`) with interchangeable store strategies beneath.
- **Registry plugin pattern**: `RegisterPlugin` map with duplicate-panic guard and helpful "supported plugins" error listing (`common/persistence/sql/store.go:21-26, 74-87`).
- **Decorator/wrapper chains**: retry → metrics → rate-limit ordering visible in every `New*Manager` (`common/persistence/client/factory.go:109-123`).
- **Lazy singleton decoration**: telemetry and fault-injection factories build each wrapped store once on first request (`common/persistence/telemetry/data_store_factory.go:43-52`; `faultinjection/data_store_factory.go:39-55`).
- **Ref-counted shared connection**: `DbConn.Get()/Close()` reference counting so many stores share one pool per factory (`common/persistence/sql/factory.go:158-217`).
- **Interface-conformance guards**: `var _ sqlplugin.Plugin = (*plugin)(nil)` compile-time assertions (`mysql/plugin.go:24`).
- **Generated delegation**: gowrap-generated decorator sources keep the tracing/fault-injection wrappers complete across large interfaces (`common/persistence/telemetry/*_gen.go`, `gowrap_template/`).
- **Config-template coverage testing**: template coverage tests ensure shipped YAML examples stay valid (`common/config/template_coverage_test.go`).
- **Test-cluster option dispatch mirroring production dispatch**: `GetTestClusterOption` mirrors the production switch so every backend is first-class in CI (`common/persistence/persistence-tests/setup.go:34-52`).

## Tradeoffs

- **Startup-time rigidity for safety**: fixing the primary backend at fx init avoids mid-flight consistency hazards but means engine swaps demand coordinated restart + data migration; there is no equivalent of the visibility dual-write bridge for the main database (contrast `manager_selector.go:43-57` with the single-shot `DataStoreFactoryProvider`).
- **Union-config vs registry**: the cassandra/sql/custom switch is simple and exhaustively checkable by `Validate()`'s "one and only one datastore" rule (`common/config/persistence.go:157-175`), but adding a fourth first-class technology edits core code — the plugin registry approach was consciously not generalized upward.
- **Broad internal interfaces**: `ExecutionStore` carries ~25 methods including DLQ and history-V2 tree APIs (`persistence_interface.go:116-167`), raising the bar for new backends; the project mitigates via shared conformance suites and generated decorators, but cost per adapter remains high.
- **SQLite as embedded dev backend**: pure-Go driver (CGO-free builds) and memory mode make local dev trivial (`development-sqlite.yaml:18-21`), acknowledged as non-production posture (`maxConns: 1`, single-shard config at lines 6-21, 8).
- **Custom extensibility requires embedding**: out-of-core datastores plug in via `AbstractDataStoreFactory`, but wiring happens in the host binary's fx graph — no dynamic module loading, trading deployability for type safety.

## Failure Modes / Edge Cases

- **Backend-specific leakage**: `InternalChasmNode.CassandraBlob` exists on the generic interface solely as a Cassandra encode/decode optimization, with a comment forbidding outside references (`common/persistence/persistence_interface.go:505-516`). Other backends must tolerate the field being set/unset — a latent hazard for third-party store authors.
- **Invalid configurations fail fast**: missing datastore entries, mixed ES/SQL visibility combos, or zero-of-many store configs abort startup with typed errors (`ErrPersistenceConfig` wrapping, `common/config/persistence.go:59-94, 170-174`; fatal log at `fx.go:207`).
- **Duplicate plugin registration panics** rather than silently shadowing (`store.go:22-24`).
- **Unknown schemes/plugins produce actionable errors** listing valid options (`store.go:79-85`; `ErrUnknownScheme`, `provider.go:21`).
- **Fault injection exists precisely to exercise these failure paths**: deterministic seeded errors can be targeted at a single store method (e.g., force `ShardOwnershipLostError` on `UpdateShard`), with a runnable sample config (`common/config/config.go:287-306`; `config/development-cass-es-fi.yaml`), and CI runs the functional suite with injection enabled (`Makefile:526-528`).
- **Credential expiry handled at connection layer**: `RefreshingConnector` refetches DSN on every new physical connection when `passwordCommand` is configured, with mutually-exclusive static-password validation and timeout/wait-delay safeguards (`sqlplugin/connector.go:8-39`; `common/config/persistence.go:284-314`).
- **Retry policy asymmetry**: namespace replication queue retries use a distinct constant-delay policy and treats `ConditionFailedError` as transient, unlike general persistence (`client/factory.go:18-21, 280-287`).

## Future Considerations

- Generalize the dual-store bridge pattern beyond visibility if live primary-store migration ever becomes a goal (the selector/shadow-read machinery is a reusable blueprint).
- Move `CassandraBlob` behind a store-private capability negotiation to fully clean the generic interface (`persistence_interface.go:509-515` already documents the debt).
- Consider a first-class registry for top-level backends to let downstream distributions add technologies without patching `DataStoreFactoryProvider`.
- The pending consolidation noted in-repo ("TODO merge persistence-tests into the tests directory", `sqlite_test.go:631`) would unify the conformance-matrix entry points.

## Questions / Gaps

- **LLM/model providers, vector databases, sandbox adapters**: No evidence found. Searches across the tree (directory listings of `common/persistence`, `common/archiver`, `components`; grep for provider/plugin registries) surface only persistence, visibility, archival, metrics, and tracing backends. Temporal is a workflow orchestration engine; it hosts no model-serving or sandboxing subsystems, so those dimension categories are out of scope for this source.
- **Hot-swap of the primary datastore**: No evidence found of any runtime reload path for `DataStoreFactoryProvider` (single invocation during fx graph construction, `temporal/fx.go:660`; `temporal/server_impl.go:161`). Swap requires restart; no migration tooling ships in this repo beyond schema-install CLIs (`Makefile:609-638`).
- **Third-party custom stores in practice**: the SPI exists (`AbstractDataStoreFactory`; custom visibility factory at `visibility/factory.go:284-299`), but the repo contains no reference implementation beyond mocks, so real-world ergonomics could not be assessed from this source alone.
- **Elasticsearch client swap-out**: the visibility ES store consumes `client.Config` (`common/config/config.go:284`), implying a fixed Elasticsearch-flavored client rather than a generic search-engine SPI; whether OpenSearch deployments rely on ES API compatibility could not be confirmed from in-repo evidence.

---

Generated by `Dimension 21.02: Provider and Backend Adapters` against `temporal`.
