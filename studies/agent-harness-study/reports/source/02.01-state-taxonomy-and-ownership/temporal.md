# Source Analysis: temporal

## State Taxonomy and Ownership

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (server, ~multiple services) |
| Analyzed | 2026-07-06 |

## Summary

Temporal is a distributed durable execution platform that organizes state into well-defined categories, each with an explicit owner module and a clear durability boundary. The codebase documents the model in `docs/architecture/history-service.md` and implements it through a layered separation: durable canonical state lives in `common/persistence` (Cassandra / MySQL / Postgres / SQLite), per-execution mutable views live in `service/history/workflow`, per-shard coordination metadata lives in `service/history/shard`, in-memory caching is confined to `common/cache` and `service/history/workflow/cache`, runtime tunable configuration is in `common/dynamicconfig`, and membership/cluster metadata is shared via `common/membership` and `common/cluster`.

The class hierarchy is intentional: `ShardInfo` (`api/persistence/v1/executions.pb.go:39`) is the durable record for shard ownership and queue watermarks; `WorkflowExecutionInfo` (`api/persistence/v1/executions.pb.go:135`) is the durable snapshot of one workflow; `NamespaceDetail` (`api/persistence/v1/namespaces.pb.go:31`) plus `NamespaceInfo` (`:123`) capture tenant-level policy; `TaskQueueInfo` (`api/persistence/v1/tasks.pb.go:221`), `VersionedTaskQueueUserData` (`api/persistence/v1/task_queues.pb.go:716`), `QueueState` (`api/persistence/v1/queues.pb.go:79`), and `ClusterMetadata` (`api/persistence/v1/cluster_metadata.pb.go:28`) capture matching, replication, and cluster-level state respectively. The in-memory counterparts (`MutableStateImpl`, `ContextImpl`, `Namespace` cache, `metadataImpl`, `userDataManagerImpl`, `taskQueueDB`, `cacheImpl`) all derive from these durable records and are reconstructed on demand. Process/session boundaries are crossed deliberately: shard transfer is fenced by `RangeID` (`service/history/shard/context_impl.go:212`), task queues are fenced by their own `rangeID` (matching `service/matching/db.go:45`), and replication task IDs serve as ordering across clusters (`api/persistence/v1/queues.pb.go:27`).

The following taxonomy is derived from concrete code locations:

| Dimension Category | Temporal Equivalent | Primary Owner | Durable? | Crosses process / session? |
|---|---|---|---|---|
| Conversation state | Workflow history events + `WorkflowMutableState` | `service/history/workflow` (MutableStateImpl) -> `common/persistence` | Yes (executions + history_node + history_tree) | Yes — reloaded on shard acquisition, replicated across clusters |
| Execution state | `MutableStateImpl` pending fields (activities / timers / children / signals / requests / chasm tree) (`service/history/workflow/mutable_state_impl.go:128-200`) | `service/history/workflow` | Yes (persisted via `WorkflowMutation`/`WorkflowSnapshot`) | Yes |
| Tool / task state | `TaskQueueInfo` (matching queue metadata) and history-task rows (`api/persistence/v1/tasks.pb.go:221`, `service/history/tasks`) | `service/matching` (root partition) + `service/history` queues | Yes | Yes — root owns + read-only replicas long-poll for updates |
| Memory state | `common/cache.Cache` (LRU) instances: workflow cache, events cache, namespace cache, not-found cache | `common/cache`, `service/history/workflow/cache`, `common/namespace/nsregistry` | No | No (process-local; TTL/backing config) |
| Artifact state | History events stored in `history_node` and `history_tree` tables (`schema/cassandra/temporal/schema.cql:58, 72`), visibility rows, search attributes | `common/persistence`, `common/persistence/visibility` | Yes | Yes (read replicas, archival) |
| Approval state | Not directly applicable; closest analogs are Workflow Update (`api/persistence/v1/update.pb.go:198`) and HSM-based `Callback` state machines (`components/callbacks/statemachine.go:37`) | `service/history/workflow/update`, `components/callbacks` | Yes (UpdateInfo stored in mutable state; Callback via CHASM/HSM tree) | Yes |
| Eval state | `service/worker/scheduler` scheduled-workflow state, `time-skipping` mutable state fields (`service/history/workflow/timeskipping.go`, `WorkflowExecutionInfo.TimeSkippingInfo` at `api/persistence/v1/executions.pb.go:1168`) | `service/worker/scheduler`, `service/history` | Yes | Yes |
| Runtime configuration | `dynamicconfig.Setting[T]` (`common/dynamicconfig/setting.go:16`), service-level `*configs.Config` (e.g. `service/history/configs/config.go:12`), `common/config.Config` (`common/config/config.go:30`), `cluster.Config` (`common/cluster/metadata.go:70`) | `common/dynamicconfig`, per-service configs | Mostly ephemeral (in-memory), but cluster metadata and DC defaults can be file-backed / refreshed from `ClusterMetadataStore` | Process-local default; persisted cluster metadata is shared |

## Rating

7 / 10

The repo has a coherent, strongly-typed state taxonomy with explicit owners and gating on mutability (range IDs, versioned transitions). All major state categories are represented in protobuf schemas (`api/persistence/v1/*`) and owner modules have stable test coverage. The platform demonstrably answers the dimension's central question — *which state must survive a crash?* — through its `ShardInfo`-driven range fencing, conditional `dbRecordVersion` writes (`api/persistence/v1/executions.pb.go:1710` `NextEventID`/`DBRecordVersion` use on mutations, e.g. `common/persistence/data_interfaces.go:348`, `:373`), and explicit `QueueState` checkpointing (`common/persistence/queue_v2.go`).

Score breaks down as:
- **+3** Strong, formal taxonomy in protobuf and persistence interface (`common/persistence/persistence_interface.go:115`, `data_interfaces.go:344`).
- **+2** Explicit in-memory vs durable split documented (`docs/architecture/history-service.md:98-132`) with code references for where MutableState is loaded.
- **+1** Cross-process safety primitives are explicit: RangeID fence (`service/history/shard/context_impl.go:212`), versioned transitions for replication (`api/persistence/v1/chasm.pb.go:86-88`), task queue version negotiation (`api/persistence/v1/task_queues.go:716`, see VersionedTaskQueueUserData).
- **+1** Tradeoffs acknowledged in comments and tests (e.g. ephemeral vs persistent user data split in `service/matching/user_data_manager.go:94-98`).

Deductions:
- **-2** Several "ephemeral" state categories (event cache, poller history, liveness, deferred task IDs in `service/history/shard/task_key_manager.go`) are scattered; ownership is implicit or local. No single "ephemeral state" inventory exists.
- **-1** No docs entry consolidates the taxonomy (only `history-service.md` and `matching-service.md` capture parts of it).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Durable shard record | `ShardInfo` proto: shard_id, range_id, owner, stolen_since_renew, queue_states | `api/persistence/v1/executions.pb.go:39-53` |
| Durable execution record | `WorkflowExecutionInfo` proto (full per-execution metadata) | `api/persistence/v1/executions.pb.go:135` |
| Durable execution state | `WorkflowExecutionState` referenced from `GetWorkflowExecutionResponse` | `common/persistence/data_interfaces.go:301-304` |
| Mutation/Snapshot envelope | `WorkflowMutation` and `WorkflowSnapshot` structs persistable as a single row | `common/persistence/data_interfaces.go:344-398` |
| Pending work fields on MutableState | `pendingActivityInfoIDs`, `pendingTimerInfoIDs`, `pendingChildExecutionInfoIDs`, etc. | `service/history/workflow/mutable_state_impl.go:130-200` |
| In-memory only fields | `currentVersion`, `approximateSize`, `stateInDB`, `dbRecordVersion` | `service/history/workflow/mutable_state_impl.go:164-198` |
| History events storage API | AppendHistoryNodes, ReadHistoryBranch, ForkHistoryBranch | `common/persistence/persistence_interface.go:152-167` |
| RangeID fence for writes | `getRangeIDLocked` + rangeID passed in `UpdateShardRequest` | `service/history/shard/context_impl.go:212`, `common/persistence/data_interfaces.go:181` |
| Shard ownership state machine | `contextState`, `stopReasonOwnershipLost`, `UnloadForOwnershipLost` | `service/history/shard/context_impl.go:127-181`, `:1461-1462` |
| Membership monitor | `Monitor` and `ServiceResolver` interfaces (ring) | `common/membership/interfaces.go:36-91` |
| Cluster metadata | `Metadata` interface, `clusterInfo` map, `metadataImpl` | `common/cluster/metadata.go:37-132` |
| Namespace cache (in-memory only) | `nsMapsLock` + maps + readthroughNotFoundCache + stateChangeCallbacks | `common/namespace/nsregistry/registry.go:105-138` |
| Watch-based vs polling-based namespace sync | Package docs + `WatchNamespaces`, `startWatchMaxAttempts`, `refreshInterval` | `common/namespace/nsregistry/registry.go:1-23`, `:50-63` |
| Namespace proto | `NamespaceDetail`, `NamespaceInfo`, `NamespaceConfig` | `api/persistence/v1/namespaces.pb.go:31`, `:123`, `:207` |
| Task queue durable state | `TaskQueueInfo` (ack_level, subqueues, partition scale) | `api/persistence/v1/tasks.pb.go:221-260` |
| Task queue user data | `TaskQueueUserData`, `VersionedTaskQueueUserData` | `api/persistence/v1/task_queues.go-*.pb.go`; pb at `:716` |
| Matching service in-memory task-queue state | `physicalTaskQueueManagerImpl` (backlogMgr, matchers, liveness, pollerHistory) | `service/matching/physical_task_queue_manager.go:55-97` |
| Matching in-memory DB facade | `taskQueueDB` (rangeID, subqueues, lastChange, lastWrite) | `service/matching/db.go:33-54` |
| Queue ack / watermark in ShardInfo | `QueueStates map[int32]*QueueState` | `api/persistence/v1/executions.pb.go:50` |
| Queue V2 abstract queue | `QueueV2Type`, `Queue` and `QueueV2` interfaces, range/messages | `common/persistence/persistence_interface.go:170-185`; `common/persistence/queue_v2.go:7-13` |
| Visibility store state | `VisibilityManager` interface | `common/persistence/visibility/manager/visibility_manager.go:21-51` |
| CHASM node state | `ChasmNode`, `ChasmNodeMetadata`, collection/data/component/pointer attributes | `api/persistence/v1/chasm.pb.go:29`, `:83`, `:191-203` |
| CHASM in-memory tree | `Node`, `valueState` dirtiness model | `chasm/tree.go:83-148` |
| CHASM registry (static, process-local) | `Registry` (libraries, components, tasks, nexus services) | `chasm/registry.go:22-43` |
| Workflow Update state | `UpdateInfo` with oneof (acceptance/completion/admission) | `api/persistence/v1/update.pb.go:198-209` |
| Callback state machine (approval-like) | `Callback`, `CALLBACK_STATE_*`, `BackoffTask`/`InvocationTask` | `components/callbacks/statemachine.go:37-100` |
| Cache abstraction | `Cache` + `StoppableCache` + `Options` (TTL, Pin, OnPut, OnEvict) | `common/cache/cache.go:12-63` |
| LRU implementation | `lru` struct, background eviction, pinning | `common/cache/lru.go:31-62` |
| Workflow mutable-state cache | `cacheImpl` (GetOrCreateWorkflowExecution, GetOrCreateCurrentExecution) | `service/history/workflow/cache/cache.go:60-99`, `:155`, `:172` |
| Events cache | `CacheImpl` (history events cache, host and shard level) | `service/history/events/cache.go:37-95` |
| Dynamic config runtime setting | `setting[T,P]` struct with default + converter + description | `common/dynamicconfig/setting.go:16-22` |
| File-backed DC client | `FileBasedClientConfig`, `FileBasedClient` | `common/dynamicconfig/file_based_client.go` |
| History service config bundle | `configs.Config` with typed property fns for cache sizes, persistence QPS, etc. | `service/history/configs/config.go:12-100` |
| Process-wide static config | `common/config.Config` (persistence, services, archival, DC, namespace defaults, visibility) | `common/config/config.go:30-56` |
| Cluster config blob | `cluster.Config` (EnableGlobalNamespace, FailoverVersionIncrement, ClusterInformation) | `common/cluster/metadata.go:70-83` |
| Schema (durable tables) | Cassandra tables: executions, history_node, history_tree, tasks/tasks_v2, task_queue_user_data, namespaces, namespaces_by_id, queue, queue_metadata, queue_messages, cluster_metadata_info, cluster_membership, queues, nexus_endpoints | `schema/cassandra/temporal/schema.cql:7-235` |
| Queue watermarks persisted | `QueueState{ReaderStates, ExclusiveReaderHighWatermark}` and `QueueReaderState{Scopes}` | `api/persistence/v1/queues.pb.go:79-175` |
| ShardInfo update task | `updateShardInfo` checkpoints tasksCompleted and writes back via `UpdateShard` | `service/history/shard/context_impl.go:1219-1280` |
| Per-execution health signal | `HealthSignalAggregator` for shard-aware persistence health snapshots | `common/persistence/health_signal_aggregator.go:18-43` |
| Replication DLQ ack | `ShardInfo.ReplicationDlqAckLevel` map | `api/persistence/v1/executions.pb.go:49` |
| Cross-cluster metadata persistence | `ClusterMetadataStore` (`List/Save/Get/DeleteClusterMetadata`, `UpsertClusterMembership`) | `common/persistence/persistence_interface.go:99-113` |
| Membership callback registration | `RegisterMetadataChangeCallback`, `clusterChangeCallback` | `common/cluster/metadata.go:131`, `:375-388` |
| Namespace state-change callbacks | `RegisterStateChangeCallback` (`sync.Map` in registry) | `common/namespace/nsregistry/registry.go:123`; `common/namespace/registry.go:37-38` |

## Answers to Dimension Questions

1. **What kinds of state exist?**
   - Durable execution state (workflow mutable state, history events, history tree) — `service/history/workflow`, `common/persistence`.
   - Durable cross-execution coordination state (shard metadata + queue watermarks) — `service/history/shard`, persisted via `ShardInfo` (`api/persistence/v1/executions.pb.go:39`).
   - Durable task-queue bookkeeping (TaskQueueInfo, VersionedTaskQueueUserData, ephemeral queue metadata, raw tasks) — `service/matching/db.go`, `service/matching/user_data_manager.go`.
   - Durable tenant state (NamespaceDetail, NamespaceInfo, NamespaceConfig) — `api/persistence/v1/namespaces.pb.go` and `MetadataStore` (`common/persistence/persistence_interface.go:86`).
   - Durable cross-cluster state (ClusterMetadata, ClusterMembership, Queue V2 messages) — `common/persistence/persistence_interface.go:99`, `cluster_metadata_info`/`cluster_membership` tables (`schema/cassandra/temporal/schema.cql:182, 193`).
   - Durable CHASM-node state (ChasmNode map in MutableStateImpl) — `api/persistence/v1/chasm.pb.go:29`, `service/history/workflow/mutable_state_impl.go:157`.
   - Visibility state — `VisibilityManager` (`common/persistence/visibility/manager/visibility_manager.go:21`).
   - HSM/Callback state machines — `components/callbacks/statemachine.go:37`.
   - In-memory caches (workflow, events, namespace, not-found, poller history, liveness, history-of-task-IDs) — see Cache references above.
   - Runtime configuration (cluster-level static config, per-service dynamic config via `dynamicconfig.Setting[T]`) — `common/config`, `common/dynamicconfig`, `service/history/configs/config.go`.
   - Membership state — `common/membership` ring (ephemeral, recomputed from gossip).
   - Health signals / metrics aggregations — `common/persistence/health_signal_aggregator.go`.

2. **Which state is source-of-truth?**
   - The single durable source-of-truth is the persistence layer (`common/persistence`):
     - `ShardInfo` row in executions table (`api/persistence/v1/executions.pb.go:39`; `schema/cassandra/temporal/schema.cql:7-56`).
     - `WorkflowExecutionInfo` + history events (`api/persistence/v1/executions.pb.go:135`, `schema/cassandra/temporal/schema.cql:7-56, 58-71`).
     - `TaskQueueInfo`, `task_queue_user_data`, `tasks/tasks_v2` (`schema/cassandra/temporal/schema.cql:83-138`).
     - `namespaces`/`namespaces_by_id` (`schema/cassandra/temporal/schema.cql:139-160`).
     - `cluster_metadata_info`/`cluster_membership` (`schema/cassandra/temporal/schema.cql:182-208`).
   - All in-memory constructs (`MutableStateImpl`, `taskQueueDB`, `cacheImpl`, `metadataImpl`, `userDataManagerImpl`) are explicitly described as derived. E.g., `taskQueueDB` comment ("newTaskQueueDB … persistence view of a physical task queue", `service/matching/db.go:84-92`) and `cacheImpl` (TTL eviction and `releaseFunc` "clears state", `service/history/workflow/cache/cache.go:126-140`).

3. **Which state is derived?**
   - In-memory workflow mutable state is derived from `WorkflowExecutionInfo` plus event replay (`docs/architecture/history-service.md:98-132`; `service/history/workflow/mutable_state_impl.go:159-180`).
   - The `Namespace` cache is derived via watch-based or polling-based sync (`common/namespace/nsregistry/registry.go:1-23`) plus read-through caching from `MetadataStore`.
   - `clusterInfo` / `versionToClusterName` maps are derived from `cluster.Config` + `ClusterMetadataStore` (`common/cluster/metadata.go:115-128`, `:155-169`).
   - `userDataManagerImpl` "ephemeral data" is derived and intentionally lazy/conservative (`service/matching/user_data_manager.go:94-98`).
   - `QueueState` in ShardInfo is a checkpointed watermark derived from running queues (`api/persistence/v1/executions.pb.go:50`; service writes these via `updateShardInfo`, `service/history/shard/context_impl.go:1219-1280`).

4. **Which state is ephemeral?**
   - LRU caches (`common/cache/lru.go:31-62`) — TTL + size + pin semantics; explicit eviction callbacks (`service/history/workflow/cache/cache.go:106-140`).
   - Workflow execution cache (TTL `HistoryCacheTTL`, `service/history/configs/config.go:62`).
   - Events cache (TTL `EventsCacheTTL`, `service/history/configs/config.go:83`).
   - `nsregistry.readthroughNotFoundCache` (TTL = 1s by design, `common/namespace/nsregistry/registry.go:53-69`).
   - Membership view of hosts (`common/membership/interfaces.go:36-65`) — re-derived from gossip; ShardController drain/lost ownership is signaled via `UnloadForOwnershipLost` (`service/history/shard/context_impl.go:1461-1462`).
   - In-process rate limiters, liveness trackers, task trackers, deferred task-ID allocator (`service/history/shard/task_key_manager.go`).
   - User data `EphemeralData` (per-partition, intentionally lossy; aggregated lazily — `service/matching/user_data_manager.go:94-98`).

5. **Which state is safe to lose?**
   - All `common/cache.Cache` entries (workflow context cache, events cache, readthrough not-found cache) — by construction they are reconstructible from persistence (`common/cache/cache.go:12-63`).
   - `businessIDRateLimiters` (in-memory, can be dropped with no durability promise, `service/history/shard/context_impl.go:153`).
   - Per-shard watermarks in memory (`remoteClusterInfos`, `handoverTracker`); the persisted `ShardInfo.QueueStates` is the recovery point (`service/history/shard/context_impl.go:144-151`; `:1219-1280`).
   - `pollerHistory`, `liveness`, local `taskTracker` counters (`service/matching/physical_task_queue_manager.go:88-97`).
   - State change callback registrations — re-register on Start.
   - Per-execution `appliedEvents` map is in-memory only and is intentionally recomputed from history (`service/history/workflow/mutable_state_impl.go:190-191`).
   - It is *not* safe to lose: anything in `ShardInfo`, `WorkflowExecutionInfo`, `TaskQueueInfo`, `NamespaceDetail`, `ClusterMetadata`, CHASM `ChasmNode` blobs, raw `tasks` rows, `history_node`/`history_tree` data, the queue_v2 messages table, and the visibility rows — see `schema/cassandra/temporal/schema.cql` tables and `common/persistence/persistence_interface.go` for the durable API.

## Architectural Decisions

- **Conditional updates anchored to durable fences.** Every durable write goes through a `RangeID` (shards) or a per-queue `rangeID` (matching) or a `DBRecordVersion` (executions). See `common/persistence/data_interfaces.go:181-184` (ShardInfo), `:345-373` (`WorkflowMutation.Condition`/`DBRecordVersion`), `service/matching/db.go:45` (taskQueueDB rangeID). This makes mutations safe across ownership transfers.
- **Event-sourced execution model.** The architecture document explicitly commits to event sourcing: state is recoverable by replay (`docs/architecture/history-service.md:113-132`). MutableState is a *projection* over History, and the History branch is the canonical artifact.
- **In-memory derivation layer above persistence.** `MutableStateImpl` (`service/history/workflow/mutable_state_impl.go:128-200`), `cacheImpl` (`service/history/workflow/cache/cache.go:60-99`), `taskQueueDB` (`service/matching/db.go:33-54`), `userDataManagerImpl` (`service/matching/user_data_manager.go:99-138`), and `metadataImpl` (`common/cluster/metadata.go:105-132`) are all in-memory views backed by an explicit durable record.
- **Two-mode namespace synchronization.** The registry supports either watch-based (push from DB) or poll-based sync; both persist into the same `idToNamespace`/`nameToID` maps and emit the same `StateChangeCallbackFn` (`common/namespace/nsregistry/registry.go:1-23`, `:105-138`).
- **Subqueries, not full generic replication.** Task queues use `subqueueIndex` and per-subqueue ack levels to enable priority/fairness without duplicating metadata (`api/persistence/v1/tasks.pb.go:227-243`, `service/matching/db.go:55-67`).
- **RangeID-locked metadata writes.** `taskQueueDB` and `ContextImpl.updateShardInfo` deliberately serialize writes (`service/matching/db.go:84-92`, `service/history/shard/context_impl.go:1219-1280`) to avoid Cassandra LWT contention.
- **Cluster config split.** `common/config.Config` is loaded from disk at process start; cluster metadata is refreshed from `ClusterMetadataStore` (`common/cluster/metadata.go:115-184`); per-service tunables are `dynamicconfig.Setting[T]` with file/client/collection abstractions (`common/dynamicconfig/setting.go:16-22`, `common/dynamicconfig/collection.go`, `common/dynamicconfig/file_based_client.go`).
- **CHASM as a generic abstraction over the executions row.** `ChasmNode` is embedded inside `WorkflowMutableState` (`api/persistence/v1/chasm.pb.go:29` plus `common/persistence/data_interfaces.go:363-364, :390`), allowing extensible typed state machines while reusing existing shard ownership and durability guarantees.

## Notable Patterns

- **Pin + Release + Finalizer for in-memory mutable state.** `cacheImpl` pins items, attaches a `finalizer.Finalizer` that calls `wfContext.Clear()` on evict, and supports `GetOrCreateWorkflowExecution` / `GetOrCreateCurrentExecution` (`service/history/workflow/cache/cache.go:60-148`). This pattern guarantees no stale mutable state outlives its ownership boundary.
- **stateStateLock and state machine for shard context.** The shard owns its lifecycle state explicitly (`contextState` enum, `contextRequest` variants, `stopReason`) instead of scattered flags (`service/history/shard/context_impl.go:127-181`).
- **One proto per state class.** Every durable class lives in `api/persistence/v1/*.pb.go`. Schema evolution is centralized; generated helpers allow consistent (de)serialization.
- **In-memory "EphemeralData" explicitly separated from durable "UserData".** `userDataManagerImpl` documents that EphemeralData is intentionally lossy and only eventually aggregated (`service/matching/user_data_manager.go:94-98`).
- **ReaderStateCheckpoint persisted explicitly.** `QueueState.ReaderStates` lives inside `ShardInfo` so that queue progress can be reconstructed when the shard is transferred (`api/persistence/v1/executions.pb.go:50`, `service/history/shard/context_impl.go:381-395`).
- **Locks / priority semaphores inside ShardContext.** `ioSemaphore` is intentionally *not* held under `rwLock` to avoid deadlock; explicit comment on ordering (`service/history/shard/context_impl.go:118-123`).

## Tradeoffs

- **Duplication of mutable state in cache and DB.** Pros: low latency to update mutable state; mutable state is the working copy. Cons: every transactional write must be careful to keep the cache, in-memory `MutableStateImpl`, and persistence in lockstep. Mitigation: explicit mutation protocol (`WorkflowMutation` / `WorkflowSnapshot`, `common/persistence/data_interfaces.go:344-398`).
- **RangeID-based fences add write contention.** Comments mention "two reasons for doing this: Cassandra LWT contention and single-writer guarantee" (`service/matching/db.go:88-92`). Simpler concurrency model in exchange for throughput cap on per-queue writes.
- **Watch vs poll for namespaces.** Watching gives timely updates but requires DB support; polling is universally supported. The registry gracefully degrades (`common/namespace/nsregistry/registry.go:1-23`).
- **Cluster metadata refresh interval of 1 min.** Acceptable staleness for failover-version math at the cost of lag (`common/cluster/metadata.go:30`). Hard-coded invariant validations in `NewMetadata` will panic on misconfiguration (`common/cluster/metadata.go:144-165`).
- **CHASM keeps versioned transitions in metadata to gate replication.** This requires the `Transition` field on every CHASM node but enables conflict resolution without implicit race detection (`api/persistence/v1/chasm.pb.go:86-88`).
- **In-memory only state for matching (`taskQueueDB`) is anchored to per-queue `rangeID` written back on every successful change** (`service/matching/db.go:50-53`). Tradeoff: extra writes for safety vs chance of corruption on process loss; recovery requires `GetTaskQueue` to read DB.
- **Dynamic config types are generated, not hand-written.** This produces verbose code but centralizes precedence / file-vs-client decisions (`common/dynamicconfig/setting.go:5-22`, `common/dynamicconfig/setting_gen.go` via `//go:generate` in `setting.go:1`).

## Failure Modes / Edge Cases

- **Shard ownership loss.** If the shard is re-acquired by another host mid-write, the in-progress transaction surfaces `ShardOwnershipLostError` and the context transitions to `Stopped` (`common/persistence/data_interfaces.go:144`, `service/history/shard/context_impl.go:144-148`, `:1461-1462`).
- **RangeID drift.** Writes that lose the conditional check return `WorkflowConditionFailedError` / `CurrentWorkflowConditionFailedError` / `ShardOwnershipLostError` (`common/persistence/data_interfaces.go:113-148`); API surfaces translate these into specific service errors.
- **Persistence unavailable briefly.** The namespace registry caps `startWatchMaxAttempts` to avoid indefinite blocking on startup (`common/namespace/nsregistry/registry.go:58-63`).
- **Transaction size limit exceeded.** `TransactionSizeLimitError` is the explicit boundary (`common/persistence/data_interfaces.go:155-158`).
- **History append timeout.** `AppendHistoryTimeoutError` is its own type, signaling history-node insert timeout specifically (`common/persistence/data_interfaces.go:109-112`).
- **Event cache inconsistency under concurrent access.** The LRU caches return copies to avoid concurrent mutation (`common/cache/lru.go:82-83`); the workflow cache additionally uses finalizers and `releaseFunc` (`service/history/workflow/cache/cache.go:106-140`).
- **Cache eviction under load.** With `TTL` and pin+eviction semantics, an eviction while a request is in flight still triggers `Clear()` through the finalizer (`service/history/workflow/cache/cache.go:106-140`).
- **Cluster metadata refresh on failure.** `refreshInterval` plus watch-based / polling-based fallback (see `common/cluster/metadata.go:30, 397-453`).
- **UserData version conflict.** `SingleTaskQueueUserDataUpdate.Conflicting` flag indicates non-retryable conflict (`common/persistence/data_interfaces.go:546-559`).
- **CHASM tree deserialization state.** `valueState` enum ensures dirty/clean state transitions are explicit (`chasm/tree.go:49-76`).

## Future Considerations

- **Generic CHASM growth.** As more "archetypes" (Nexus, callback, schedule, etc.) are added to the registry (`chasm/registry.go:22-44`), the implicit dependency on `WorkflowExecutionInfo` storage layout may become a bottleneck.
- **Single-row executions model.** History docs note that because Cassandra has weak RDBMS features, the per-execution schema is "persisted in a single row" (`docs/architecture/history-service.md:98-110`). At very large mutable state, this is a known pressure point (`WorkflowMutation` mutation/upsert maps `common/persistence/data_interfaces.go:344-373`).
- **Two versions of task persistence (`tasks` vs `tasks_v2`) and `OtherHasTasks` migrations.** Currently `TaskQueueInfo.OtherHasTasks` flips when transitioning between backends (`api/persistence/v1/tasks.pb.go:244-255`). The state model must keep both potential buckets safe under migration.
- **Visibility dual-writes.** `common/persistence/visibility/visibility_manager_dual.go` implies dual-write/read modes; coordination of these is not visible in this dimension's scope.
- **Ephemeral data semantics.** The `EphemeralData` aggregator model may evolve from "eventually correct" to "strictly aggregated" (`service/matching/user_data_manager.go:94-98`).
- **Schedule (cron / temporal-schedule) state shape is not represented in `api/persistence/v1` at first glance**; it appears to live inside Workflow mutable state as `CronSchedule` plus `WorkflowExecutionOptionsUpdatedEvent` attributes (`api/persistence/v1/executions.pb.go:43` CronSchedule). This is acceptable but should be documented for new contributors.

## Questions / Gaps

- A **single canonical table of state classes with owners and durability** does not exist in the codebase or docs. The closest artifact is `docs/architecture/history-service.md`, but it does not cover matching, worker, or cluster/membership states.
- **Schedule / cron state** is referenced (`CronSchedule` field on `WorkflowExecutionInfo`, `api/persistence/v1/executions.pb.go:43`) but no dedicated state category for it appears in `api/persistence/v1`. Schedules' persistence is described via `service/worker/scheduler/workflow.go`; the dimension's "eval state" mapping here relies on the surrounding workflow's mutable state. No clear evidence found for a dedicated `ScheduleState` proto.
- **Approval state**: No clear evidence found that Temporal itself implements an "approval" state class distinct from HSM `Callback` machines and `Update` registration. The `Update` value oneof (`api/persistence/v1/update.pb.go:198-294`) plus `Callback` (`components/callbacks/statemachine.go:37-64`) cover the lifecycle for asynchronous client requests.
- **Visitor metadata and `appliedEvents` map on `MutableStateImpl`** are explicitly noted as in-memory only (`service/history/workflow/mutable_state_impl.go:190-191` "TODO: persist this to db"). If the shard is lost during a transition, this flag can drift and the comment admits it.
- **Cross-cluster replication DLQ state** is part of `ShardInfo.ReplicationDlqAckLevel` (`api/persistence/v1/executions.pb.go:49`), but the deeper DLQ message routing lives behind `PutReplicationTaskToDLQ` (`common/persistence/persistence_interface.go:142-147`) and is not surfaced through the `Registry` abstraction.
- **Liveness, pollerHistory, health signals** all produce state owned by individual components but it is not centralized.
- **Worker heartbeat / status state for workers** (`worker.WorkerHeartbeat`, `service/matching/workers/`) — the boundary between ephemeral (in-process poller) and durable (heartbeats in `task_queue_user_data`) state was not fully traced here.
- **History branch tokens** (`history_branch_util.go`) are durable state; not explicitly covered by the summary above even though they are central to event sourcing. The dedicated `HistoryBranchUtil` lives at `common/persistence/history_branch_util.go:1-50` and the persistence calls are at `common/persistence/persistence_interface.go:158-167`.
- **Search attributes / visibility schema** are configured at the cluster level via `IndexSearchAttributes` in `ClusterMetadata` (`api/persistence/v1/cluster_metadata.pb.go:34`); how new fields propagate to running shards was not traced in this study.

---

Generated by `02.01-state-taxonomy-and-ownership` against `temporal`.
