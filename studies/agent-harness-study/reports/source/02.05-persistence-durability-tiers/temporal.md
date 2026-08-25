# Source Analysis: temporal

## Dimension 02.05: Persistence Durability Tiers

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go server; SQL persistence for MySQL/PostgreSQL/SQLite; Cassandra persistence; Elasticsearch/OpenSearch visibility; file/S3/GCS archival |
| Analyzed | 2026-07-10 |

## Summary

Temporal has an explicit persistence model: `Config.Persistence` wires a named `DefaultStore`, `VisibilityStore`, optional `SecondaryVisibilityStore`, history shard count, and named datastore map (`common/config/config.go:256-268`), while each `DataStore` selects exactly one of Cassandra, SQL, custom, or Elasticsearch (`common/config/config.go:273-284`; `common/config/persistence.go:155-175`). Core workflow, shard, task, namespace, queue, cluster metadata, and Nexus endpoint state is durable when `DefaultStore` points at Cassandra or SQL (`common/persistence/client/fx.go:181-214`; `common/persistence/persistence_interface.go:30-51`). Visibility is independently durable/configurable through SQL, Elasticsearch, or custom visibility stores (`common/persistence/visibility/factory.go:237-301`).

The production Docker template restricts core DB selection to Cassandra, MySQL, or PostgreSQL (`config/docker.yaml:13-16`) and configures visibility either as Elasticsearch or SQL (`config/docker.yaml:93-157`). SQLite is present primarily for development/test/lite-server operation, with an in-memory configuration that does not survive process death (`config/development-sqlite.yaml:18-21`) and a file-backed configuration with WAL pragmas (`config/development-sqlite-file.yaml:18-23`). Startup recovery is explicit for shards and cluster metadata: shard contexts load or create persisted shard rows (`service/history/shard/context_impl.go:1777-1847`), then acquire a range-ID lock via persisted shard update (`service/history/shard/context_impl.go:1913-1999`; `service/history/shard/context_impl.go:1164-1197`), while cluster metadata is read from persistence and merged over static configuration (`temporal/fx.go:604-606`; `temporal/fx.go:691-733`).

## Rating

8/10 — Temporal has clear store interfaces, concrete durable implementations, schema/version checks, startup recovery paths, and broad test coverage across SQL, Cassandra, SQLite, Elasticsearch, and OpenSearch (`common/persistence/persistence_interface.go:30-51`; `common/persistence/sql/factory.go:43-151`; `common/persistence/cassandra/factory.go:29-126`; `.github/workflows/run-tests.yml:58-62`; `.github/workflows/run-tests.yml:117-151`). The score is below 9 because durability tiers are not presented as a single operator-facing map, SQLite schema verification is a no-op (`common/persistence/sql/sqlplugin/sqlite/db.go:119-124`), custom datastore durability is outside core enforcement (`common/persistence/client/abstract_data_store_factory.go:14-25`), and Elasticsearch visibility uses an in-process bulk processor whose pending requests are not themselves a durable queue (`common/persistence/visibility/store/elasticsearch/processor.go:154-183`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Persistence configuration model | `Config.Persistence` exposes `defaultStore`, `visibilityStore`, `secondaryVisibilityStore`, `numHistoryShards`, and `datastores`. | `common/config/config.go:256-268` |
| Datastore types | `DataStore` can contain Cassandra, SQL, custom datastore, or Elasticsearch config. | `common/config/config.go:273-284` |
| Datastore validation | `DataStore.Validate` requires exactly one backend config and validates SQL/Cassandra/Elasticsearch settings. | `common/config/persistence.go:155-195` |
| Core store factory | `DataStoreFactoryProvider` selects Cassandra, SQL, or custom factory from `DefaultStore`; Elasticsearch is not a core default-store backend. | `common/persistence/client/fx.go:181-214` |
| Core persistence interface | `DataStoreFactory` vends task, shard, metadata, execution, queue, cluster metadata, and Nexus endpoint stores. | `common/persistence/persistence_interface.go:30-51` |
| Core manager factory | Managers wrap stores with rate limit, metrics, and retry clients for task/shard/metadata/execution/queues. | `common/persistence/client/factory.go:108-242` |
| Retry behavior | Persistence retries `Unavailable` and `DataLoss`; namespace queues also retry `ConditionFailedError`. | `common/persistence/client/factory.go:270-287` |
| SQL implementation | SQL factory creates SQL-backed task, shard, metadata, execution, queue, queue-v2, and Nexus endpoint stores. | `common/persistence/sql/factory.go:43-151` |
| Cassandra implementation | Cassandra factory creates Cassandra-backed task, shard, metadata, execution, queue, queue-v2, and Nexus endpoint stores. | `common/persistence/cassandra/factory.go:29-126` |
| SQL active-state schema | MySQL schema includes namespaces, shards, executions, current executions, tasks, task queues, history nodes, queues, cluster metadata, queue-v2, and Nexus endpoint tables. | `schema/mysql/v8/temporal/schema.sql:1-147`; `schema/mysql/v8/temporal/schema.sql:311-415` |
| Cassandra active-state schema | Cassandra schema stores executions, history nodes, tasks, namespace mappings, queue metadata/messages, cluster metadata, queue-v2, and Nexus endpoints. | `schema/cassandra/temporal/schema.cql:7-249` |
| Visibility store interface | `VisibilityStore` defines visibility write/read/admin methods for workflow execution visibility and search attributes. | `common/persistence/visibility/store/visibility_store.go:17-49` |
| Visibility store factory | Visibility factory selects SQL, Elasticsearch, or custom visibility store from `visibilityStore`/`secondaryVisibilityStore`. | `common/persistence/visibility/factory.go:237-301` |
| SQL visibility schema | SQL visibility stores workflow execution rows and search attributes in `executions_visibility` and indexed/generated columns. | `schema/mysql/v8/visibility/schema.sql:1-90` |
| Elasticsearch visibility | Elasticsearch visibility store creates a client, optional processor, and index name from config. | `common/persistence/visibility/store/elasticsearch/visibility_store.go:125-180` |
| Elasticsearch write durability boundary | Elasticsearch writes enqueue bulk requests and wait for ACK/NACK/timeout. | `common/persistence/visibility/store/elasticsearch/visibility_store.go:327-361` |
| Elasticsearch in-process bulk processor | Bulk processor keeps an in-memory map from visibility task key to ACK future and adds requests to the bulk processor. | `common/persistence/visibility/store/elasticsearch/processor.go:37-48`; `common/persistence/visibility/store/elasticsearch/processor.go:154-183` |
| Production config | Docker config accepts only Cassandra, MySQL, PostgreSQL, or PostgreSQL PGX for core persistence. | `config/docker.yaml:13-16`; `config/docker.yaml:51-90` |
| Production visibility config | Docker config chooses Elasticsearch if enabled, otherwise SQL visibility for MySQL/PostgreSQL; Cassandra visibility fails. | `config/docker.yaml:93-157` |
| Development SQLite memory | Development SQLite config uses `mode: memory`, `cache: private`, and one connection. | `config/development-sqlite.yaml:18-23`; `config/development-sqlite.yaml:40-45` |
| Development SQLite file | File-backed SQLite config enables schema setup, WAL, and synchronous mode. | `config/development-sqlite-file.yaml:18-23`; `config/development-sqlite-file.yaml:42-49` |
| SQLite process durability note | SQLite plugin notes in-memory DB is deleted when the last connection closes and sets idle timeout to prevent idle connection reaping. | `common/persistence/sql/sqlplugin/sqlite/plugin.go:125-130` |
| SQLite schema setup | SQLite plugin auto-sets up schema for in-memory mode and optional file-mode `setup=true`. | `common/persistence/sql/sqlplugin/sqlite/plugin.go:135-147`; `schema/sqlite/setup.go:33-72` |
| Lite server storage | Lite server overrides storage to SQLite, using `mode=memory` when ephemeral and `mode=rwc` when not ephemeral. | `temporaltest/internal/lite_server.go:74-89`; `temporaltest/internal/lite_server.go:113-120` |
| Lite server recovery/setup | Non-ephemeral lite server requires a database file path and runs schema setup only if the file does not exist. | `temporaltest/internal/lite_server.go:193-199`; `temporaltest/internal/lite_server.go:224-239` |
| Shard recovery | Shard context loads persisted shard metadata via `GetOrCreateShard`, restores queue states, and initializes task minimum scheduled time from persisted queue high-watermarks. | `service/history/shard/context_impl.go:1777-1847` |
| Shard ownership durability | Shard acquisition renews `RangeId` through persisted `UpdateShard`, then updates in-memory shard info and task-key manager. | `service/history/shard/context_impl.go:1164-1197` |
| Cluster metadata recovery | Startup reads current cluster metadata from persistence, initializes missing metadata, updates existing metadata, then loads and merges persisted cluster metadata with static config. | `temporal/fx.go:691-733`; `temporal/fx.go:736-781`; `temporal/fx.go:784-819` |
| Schema compatibility | Startup verifies Cassandra and SQL compatible schema versions. | `temporal/fx.go:912-925`; `common/persistence/schema/version.go:9-30` |
| SQLite schema gap | SQLite `VerifyVersion` returns nil with a TODO to implement schema verification. | `common/persistence/sql/sqlplugin/sqlite/db.go:119-124` |
| Archival config | Archival config supports history/visibility archival, filestore, GCS, S3, and custom archiver config. | `common/config/config.go:493-558` |
| Archiver provider | Archiver provider creates cached history/visibility archivers for file, GCS, S3, or custom schemes. | `common/archiver/provider/provider.go:123-183`; `common/archiver/provider/provider.go:186-247` |
| File archival | Filestore history archiver writes JSON history files under a URI directory; visibility archiver writes one encoded record file per workflow close. | `common/archiver/filestore/history_archiver.go:1-13`; `common/archiver/filestore/history_archiver.go:171-183`; `common/archiver/filestore/visibility_archiver.go:100-120` |
| Object archival | S3 history archiver uploads encoded history blobs to S3 keys and records upload metrics. | `common/archiver/s3store/history_archiver.go:177-218` |
| GCS archival | GCS history archiver uploads multipart history objects to the configured URI bucket/path. | `common/archiver/gcloud/history_archiver.go:165-178` |
| In-memory caches | XDC event blob cache is an in-process cache with TTL and max bytes; namespace registry warns `GetAllNamespaces` returns an in-memory snapshot that may lag persistence. | `common/persistence/xdc_cache.go:88-134`; `common/namespace/registry.go:29-31` |
| In-memory scheduled queue | Memory scheduled queue keeps tasks in an in-process priority queue and channel. | `service/history/queues/memory_scheduled_queue.go:21-64`; `service/history/queues/memory_scheduled_queue.go:101-179` |
| No-op persistence-adjacent behavior | Health signal no-op aggregator drops records and reports zero latency/error ratio. | `common/persistence/noop_health_signal_aggregator.go:7-27` |
| No-op DLQ writer | `NoopDLQWriter` returns nil without writing to a DLQ. | `service/history/replication/noop_dlq_writer.go:5-13` |
| Test storage matrix | CI intentionally covers SQL, NoSQL/Cassandra, Elasticsearch/OpenSearch, SQLite, MySQL, PostgreSQL, and PGX. | `.github/workflows/run-tests.yml:58-62`; `.github/workflows/run-tests.yml:117-151` |

## Answers to Dimension Questions

### 1. What survives a restart?

- Core workflow state, shard state, task queues, history events, namespace metadata, cluster metadata, queue-v2 messages, and Nexus endpoints survive restart when `DefaultStore` is Cassandra or SQL because those categories are exposed by `DataStoreFactory` (`common/persistence/persistence_interface.go:30-51`) and backed by durable SQL/Cassandra tables (`schema/mysql/v8/temporal/schema.sql:1-147`; `schema/mysql/v8/temporal/schema.sql:311-415`; `schema/cassandra/temporal/schema.cql:7-249`).
- SQL visibility survives restart when `visibilityStore` is SQL, because workflow visibility records live in `executions_visibility` (`schema/mysql/v8/visibility/schema.sql:1-90`) and the SQL visibility implementation writes through SQL `InsertIntoVisibility`, `ReplaceIntoVisibility`, and `DeleteFromVisibility` calls (`common/persistence/visibility/store/sql/visibility_store.go:114-193`).
- Elasticsearch visibility survives restart only after Elasticsearch has accepted the indexed/deleted document; pending in-process bulk requests do not survive because the bulk processor uses an in-memory ACK map (`common/persistence/visibility/store/elasticsearch/processor.go:37-48`) and in-memory `Add` path (`common/persistence/visibility/store/elasticsearch/processor.go:154-183`). The write path waits for ACK/NACK/timeout and returns retryable `Unavailable` on NACK (`common/persistence/visibility/store/elasticsearch/visibility_store.go:337-359`).
- File-backed SQLite survives restart if the SQLite database file is preserved, as the lite server uses SQLite `mode=rwc` for non-ephemeral mode (`temporaltest/internal/lite_server.go:86-88`) and requires a `DatabaseFilePath` when ephemeral mode is disabled (`temporaltest/internal/lite_server.go:193-199`).
- In-memory SQLite does not survive process death; development config uses SQLite `mode: memory` (`config/development-sqlite.yaml:18-21`), and the SQLite plugin notes in-memory databases are deleted when the last connection closes (`common/persistence/sql/sqlplugin/sqlite/plugin.go:125-130`).
- In-memory caches and memory scheduled queues do not survive restart; examples include the XDC event blob cache (`common/persistence/xdc_cache.go:88-134`), namespace in-memory snapshot (`common/namespace/registry.go:29-31`), and memory scheduled queue (`service/history/queues/memory_scheduled_queue.go:21-64`).
- Archived history/visibility survives according to the configured archive backend: filestore writes files (`common/archiver/filestore/history_archiver.go:171-183`; `common/archiver/filestore/visibility_archiver.go:100-120`), S3 uploads blobs (`common/archiver/s3store/history_archiver.go:177-218`), and GCS uploads blobs (`common/archiver/gcloud/history_archiver.go:165-178`).

### 2. What survives redeploy?

- External Cassandra/MySQL/PostgreSQL state survives redeploy if the backing service and schema remain compatible; production Docker config restricts core stores to those DBs (`config/docker.yaml:13-16`; `config/docker.yaml:51-90`) and startup checks SQL/Cassandra schema compatibility (`temporal/fx.go:912-925`; `common/persistence/schema/version.go:9-30`).
- Elasticsearch/OpenSearch visibility survives redeploy when the external index survives; Docker config points visibility to Elasticsearch indices (`config/docker.yaml:93-106`) and Elasticsearch config requires a visibility index (`common/persistence/visibility/store/elasticsearch/client/config.go:95-105`).
- File-backed SQLite survives redeploy only if the same file path and filesystem volume survive; the non-ephemeral lite server requires a database file path (`temporaltest/internal/lite_server.go:193-199`) and file-mode development config omits `mode: memory` while enabling WAL/synchronous settings (`config/development-sqlite-file.yaml:18-23`).
- In-memory SQLite, XDC cache, namespace registry cache, health signal aggregator state, and memory scheduled queues do not survive redeploy (`config/development-sqlite.yaml:18-21`; `common/persistence/xdc_cache.go:88-134`; `common/namespace/registry.go:29-31`; `common/persistence/noop_health_signal_aggregator.go:15-27`; `service/history/queues/memory_scheduled_queue.go:21-64`).

### 3. What survives multi-instance operation?

- Cassandra/MySQL/PostgreSQL-backed core state is multi-instance capable at the persistence tier because multiple history hosts acquire shard ownership through persisted `RangeId` updates (`service/history/shard/context_impl.go:1164-1197`) after loading persisted shard metadata (`service/history/shard/context_impl.go:1777-1847`).
- Cluster membership and cluster metadata are persisted in SQL tables (`schema/mysql/v8/temporal/schema.sql:352-376`) and Cassandra tables (`schema/cassandra/temporal/schema.cql:182-208`), while startup merges persisted cluster metadata over static config (`temporal/fx.go:604-606`; `temporal/fx.go:691-733`).
- SQL or Elasticsearch visibility can be shared across instances when configured as external services; the visibility manager is constructed from `visibilityStore` and optional `secondaryVisibilityStore` (`common/persistence/visibility/factory.go:33-127`).
- SQLite is not positioned as a production multi-instance backend in the production Docker template, which excludes SQLite from accepted core DB values (`config/docker.yaml:13-16`). SQLite development config uses one connection and in-memory/private cache (`config/development-sqlite.yaml:18-23`), and file-mode SQLite still depends on a local file path (`config/development-sqlite-file.yaml:18-23`).
- In-memory queues/caches are per-process only; the memory scheduled queue stores tasks in local `taskQueue`, timer, and channel fields (`service/history/queues/memory_scheduled_queue.go:21-64`).

### 4. What is silently dropped?

- The no-op health signal aggregator drops recorded persistence health signals by implementing empty `Start`, `Stop`, and `Record` methods and returning zero metrics (`common/persistence/noop_health_signal_aggregator.go:15-27`). This is observability state, not workflow state.
- `NoopDLQWriter` returns nil from `WriteTaskToDLQ` without storing anything (`service/history/replication/noop_dlq_writer.go:5-13`). The production queue module provides the real `queues.NewDLQWriter` through FX (`service/history/queue_factory_base.go:77-85`), so the no-op writer appears to be a test/substitution path rather than the default production DLQ path.
- S3 archive progress recording is best-effort and ignores failures in `saveHistoryIteratorState` (`common/archiver/s3store/history_archiver.go:240-253`); the history blobs are still uploaded through S3 `Upload` before progress is recorded (`common/archiver/s3store/history_archiver.go:198-212`).
- Pending Elasticsearch bulk requests can be lost on process death because they live in the in-process bulk processor/ACK map (`common/persistence/visibility/store/elasticsearch/processor.go:37-48`; `common/persistence/visibility/store/elasticsearch/processor.go:154-183`). The caller-facing mitigation is ACK waiting plus timeout/NACK errors (`common/persistence/visibility/store/elasticsearch/visibility_store.go:337-359`).
- No evidence found of a production no-op core `DataStoreFactory`; the inspected core factory selects Cassandra, SQL, or custom and fatals if none is configured (`common/persistence/client/fx.go:181-214`), while the only no-op file under `common/persistence` is the health signal aggregator (`common/persistence/noop_health_signal_aggregator.go:7-27`).

### 5. Is durability configurable?

- Yes. Core durability is configured by `persistence.defaultStore` and `persistence.datastores` (`common/config/config.go:256-268`) with backend-specific Cassandra and SQL settings (`common/config/config.go:359-462`).
- Visibility durability is separately configured by `persistence.visibilityStore` and `persistence.secondaryVisibilityStore` (`common/config/config.go:260-263`), and the visibility factory supports SQL, Elasticsearch, or custom visibility stores (`common/persistence/visibility/factory.go:237-301`).
- Archival durability is separately configured for history and visibility with filestore, S3, GCS, or custom stores (`common/config/config.go:493-558`; `common/archiver/provider/provider.go:123-183`; `common/archiver/provider/provider.go:186-247`).
- Custom datastore durability is configurable by injection but not enforceable by the core contract; `CustomDatastoreConfig` provides a name, index name, and options (`common/config/config.go:464-475`), and `AbstractDataStoreFactory` returns a custom `DataStoreFactory` (`common/persistence/client/abstract_data_store_factory.go:14-25`).

## Architectural Decisions

- Temporal separates active persistence from visibility persistence: `DefaultStore` controls core stores (`common/config/config.go:258-259`) while `VisibilityStore` and `SecondaryVisibilityStore` control visibility stores (`common/config/config.go:260-263`); dual visibility is supported only for compatible combinations (`common/config/persistence.go:44-83`) and is wired by `NewVisibilityManagerDual` when a secondary visibility manager exists (`common/persistence/visibility/factory.go:111-123`).
- Temporal hides concrete stores behind interfaces: `DataStoreFactory` returns core store interfaces (`common/persistence/persistence_interface.go:30-51`), SQL and Cassandra factories implement those interfaces (`common/persistence/sql/factory.go:43-151`; `common/persistence/cassandra/factory.go:29-126`), and higher-level managers add retry/rate-limit/metrics wrappers (`common/persistence/client/factory.go:108-242`).
- Temporal treats cluster metadata as persisted truth after boot: `ApplyClusterMetadataConfigProvider` states persisted values take precedence (`temporal/fx.go:604-606`), reads `GetClusterMetadata` (`temporal/fx.go:691-694`), initializes missing metadata (`temporal/fx.go:713-723`), and loads/merges persisted metadata (`temporal/fx.go:728-733`).
- Temporal uses shard range IDs for recovery and multi-instance ownership: shard metadata is loaded from persistence (`service/history/shard/context_impl.go:1777-1798`), queue high-watermarks are restored into in-memory scheduling state (`service/history/shard/context_impl.go:1811-1846`), and shard ownership is acquired by incrementing persisted `RangeId` (`service/history/shard/context_impl.go:1164-1197`).
- Temporal keeps archival outside the active store durability tier: archival APIs define history/visibility archivers (`common/archiver/interface.go:43-59`; `common/archiver/interface.go:75-94`), and providers choose file/S3/GCS/custom implementations by URI scheme (`common/archiver/provider/provider.go:123-183`; `common/archiver/provider/provider.go:186-247`).

## Notable Patterns

- Store category mapping is schema-backed: workflow mutable state and current run pointers are in `executions` and `current_executions` (`schema/mysql/v8/temporal/schema.sql:30-62`), workflow/event history is in `history_node` and `history_tree` (`schema/mysql/v8/temporal/schema.sql:311-334`), task queues/tasks are in `tasks`, `task_queues`, `tasks_v2`, and `task_queues_v2` (`schema/mysql/v8/temporal/schema.sql:95-138`), and queue-v2 messages are in `queues` and `queue_messages` (`schema/mysql/v8/temporal/schema.sql:378-399`).
- Recovery uses persisted queue state but local runtime structures: shard load restores queue states from `ShardInfo.QueueStates` (`service/history/shard/context_impl.go:1811-1846`), while memory scheduled queue runtime data remains local (`service/history/queues/memory_scheduled_queue.go:21-64`).
- Compatibility checks allow actual schema version to be newer than expected, supporting rollback after backward-compatible schema upgrades (`common/persistence/schema/version.go:20-29`).
- Test coverage intentionally spans persistence engines and visibility engines: SQL, NoSQL/Cassandra, Elasticsearch/OpenSearch, SQLite, MySQL, PostgreSQL, and PGX are all listed in CI matrix config (`.github/workflows/run-tests.yml:58-62`; `.github/workflows/run-tests.yml:117-151`).
- Development/test storage is explicit: SQLite in-memory configuration uses `mode: memory` (`config/development-sqlite.yaml:18-21`), SQLite file configuration enables `setup`, WAL, and synchronous mode (`config/development-sqlite-file.yaml:18-23`), and lite server switches between ephemeral memory and file modes (`temporaltest/internal/lite_server.go:74-89`).

## Tradeoffs

- Cassandra/MySQL/PostgreSQL provide production-durable shared stores, but operators must run schema setup/updates and pass startup compatibility checks (`temporal/fx.go:912-925`; `common/persistence/schema/version.go:9-30`).
- Elasticsearch/OpenSearch visibility improves search flexibility but introduces an asynchronous in-process bulk processor (`common/persistence/visibility/store/elasticsearch/processor.go:37-48`; `common/persistence/visibility/store/elasticsearch/processor.go:154-183`) rather than direct per-write durable confirmation like SQL visibility (`common/persistence/visibility/store/sql/visibility_store.go:114-193`).
- SQLite minimizes local/test setup, but in-memory mode loses state on process death (`config/development-sqlite.yaml:18-21`; `common/persistence/sql/sqlplugin/sqlite/plugin.go:125-130`), and SQLite schema compatibility is not enforced (`common/persistence/sql/sqlplugin/sqlite/db.go:119-124`).
- Archival offloads long-lived history/visibility to file/object storage (`common/config/config.go:493-558`; `common/archiver/provider/provider.go:123-247`), but durability depends on backend-specific filesystem/object-store guarantees and configuration rather than the active persistence store.
- Custom datastores extend durability options through `AbstractDataStoreFactory` (`common/persistence/client/abstract_data_store_factory.go:14-25`) but shift durability, multi-instance safety, and recovery semantics outside Temporal core enforcement.

## Failure Modes / Edge Cases

- In-memory SQLite can lose all state on process or last-connection death; the SQLite plugin explicitly notes the database is deleted when the last connection closes (`common/persistence/sql/sqlplugin/sqlite/plugin.go:125-130`).
- SQLite schema mismatch can pass startup because `VerifyVersion` returns nil and contains a TODO to implement compatibility verification (`common/persistence/sql/sqlplugin/sqlite/db.go:119-124`).
- Elasticsearch visibility pending writes can be lost if the process dies while requests are only in the bulk processor/ACK map (`common/persistence/visibility/store/elasticsearch/processor.go:37-48`; `common/persistence/visibility/store/elasticsearch/processor.go:154-183`); the store mitigates failed commits by returning timeout or retryable unavailable errors after ACK wait (`common/persistence/visibility/store/elasticsearch/visibility_store.go:337-359`).
- No-op DLQ writer can silently drop tasks if injected because `WriteTaskToDLQ` returns nil without persistence (`service/history/replication/noop_dlq_writer.go:5-13`); real DLQ writing creates/enqueues into persistent queue-v2 through `queues.DLQWriter` (`service/history/queues/dlq_writer.go:63-142`).
- Cluster metadata persisted values override static config during startup (`temporal/fx.go:604-606`; `temporal/fx.go:691-733`), so config-only redeploy changes may not take effect if persisted metadata differs.
- File-backed SQLite durability depends on the deployment preserving the same local database file, because non-ephemeral lite server requires `DatabaseFilePath` (`temporaltest/internal/lite_server.go:193-199`) and only sets up schema when that file does not yet exist (`temporaltest/internal/lite_server.go:224-239`).

## Future Considerations

- Add an operator-facing durability-tier matrix that maps state categories to `DefaultStore`, `VisibilityStore`, archival, and in-memory caches; current evidence is distributed across config structs (`common/config/config.go:256-284`), factory wiring (`common/persistence/client/fx.go:181-214`; `common/persistence/visibility/factory.go:237-301`), and schema files (`schema/mysql/v8/temporal/schema.sql:1-415`).
- Implement SQLite schema compatibility checks in `VerifyVersion` (`common/persistence/sql/sqlplugin/sqlite/db.go:119-124`) or clearly label SQLite as development/test-only in operator-facing config.
- Add explicit observability or warnings when no-op persistence-adjacent components are injected, especially `NoopDLQWriter` (`service/history/replication/noop_dlq_writer.go:5-13`) and `NoopHealthSignalAggregator` (`common/persistence/noop_health_signal_aggregator.go:7-27`).
- Document custom datastore durability obligations next to `AbstractDataStoreFactory` (`common/persistence/client/abstract_data_store_factory.go:14-25`) and `CustomDatastoreConfig` (`common/config/config.go:464-475`).
- Expose Elasticsearch visibility bulk-processor pending request durability/lag more directly to operators; metrics are recorded for queued requests and failures (`common/persistence/visibility/store/elasticsearch/processor.go:223-275`), but the operator-facing durability tier is not explicitly stated in config.

## Questions / Gaps

- No clear evidence found of a single operator document that says which state is production-durable; the strongest evidence is scattered across `Config.Persistence` (`common/config/config.go:256-284`), production Docker restrictions (`config/docker.yaml:13-16`; `config/docker.yaml:93-157`), and store schemas (`schema/mysql/v8/temporal/schema.sql:1-415`; `schema/cassandra/temporal/schema.cql:7-249`).
- No clear evidence found that custom datastores must declare durability guarantees; the core contract only requires `AbstractDataStoreFactory.NewFactory` to return a `DataStoreFactory` (`common/persistence/client/abstract_data_store_factory.go:14-25`).
- No clear evidence found that SQLite is guarded against production use at runtime; it is excluded from `config/docker.yaml` accepted DB values (`config/docker.yaml:13-16`) but remains a registered SQL plugin (`common/persistence/sql/sqlplugin/sqlite/plugin.go:45-50`).
- No clear evidence found that Elasticsearch visibility pending bulk requests are persisted outside the external Elasticsearch index before ACK; the processor uses in-memory request-to-future tracking (`common/persistence/visibility/store/elasticsearch/processor.go:37-48`) and the visibility store waits for ACK/NACK/timeout (`common/persistence/visibility/store/elasticsearch/visibility_store.go:337-359`).

---

Generated by `02.05-persistence-durability-tiers` against `temporal`.
