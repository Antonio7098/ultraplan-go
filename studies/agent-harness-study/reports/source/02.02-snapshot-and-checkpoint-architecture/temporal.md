# Source Analysis: temporal

## Snapshot and Checkpoint Architecture

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.24, distributed services (history / frontend / matching / worker / internal persistence plugins for Cassandra, MySQL, PostgreSQL, SQLite) |
| Analyzed | 2026-07-06 |

## Summary

Temporal models durable execution as **event sourcing, not as a periodic snapshot/serialize state-machine**. The "checkpoint" is implicitly the immutable append-only `history_node` table (events) plus a single mutable-state row (`executions`) that is overwritten on every commit. Re-execution of the same event sequence from history reproduces the equivalent mutable state deterministically. This is structurally different from checkpoint-based agent frameworks (LangGraph, Letta, OpenHands), which serialize the in-memory state at user-defined break points.

Concretely, Temporal runs two interleaved mechanisms:

1. **Per-transaction snapshot/mutation persistence** (`service/history/workflow/mutable_state_impl.go:7484-7573` and `common/persistence/data_interfaces.go:345-398`) — every closed transaction produces either a `WorkflowMutation` (delta) or a `WorkflowSnapshot` (full) that is written to `executions`. The choice is mode-driven (create / update / continue-as-new / conflict-resolve).
2. **Asynchronous queue-reader ack-level checkpointing** (`service/history/queues/queue_base.go:83-419`) — periodic timer-driven `checkpoint()` calls persist the per-shard `QueueStates` map so that on shard reload the readers resume close to where they left off.

Resume is implemented by **event replay**, not by reloading a serialized state blob: `service/history/ndc/state_rebuilder.go:103-304` reads the branch's history events from `history_node` and re-applies them through `MutableStateRebuilderImpl.ApplyEvents` (`service/history/workflow/mutable_state_rebuilder.go:70-150`) to reconstruct the mutable state. Branching / forking / reset / conflict resolution all reuse the same replay mechanism with different fork points (`service/history/ndc/branch_manager.go:23-227`, `service/history/ndc/workflow_resetter.go:434-508`).

## Rating

**9 / 10** — Mature, durable, observable, extensible, proven under failure and scale.

Rationale (rubric: 9–10 = "Mature, durable, observable, extensible, and proven under failure or scale"):

- Explicit interfaces: `MutableState` (`service/history/interfaces/mutable_state.go`), `HistoryBranchUtil` (`common/persistence/history_branch_util.go:14-36`), `ExecutionManager` (`common/persistence/persistence_interface.go`).
- Hardened durability: optimistic CAS on `next_event_id` (legacy) **and** `db_record_version` (`common/persistence/cassandra/mutable_state_store.go:107-123`), checksums (`service/history/workflow/checksum.go:18-90`), shard `RangeID` fencing (`common/persistence/cassandra/mutable_state_store.go:22-31`), event-branching/trimming (`common/persistence/history_manager.go:212-308`).
- Observability: dedicated metrics (`MutableStateChecksumMismatch`, `MutableStateChecksumInvalidated`, `QueueReaderCountHistogram`, `AckLevelUpdateCounter`, `TaskBatchCompleteCounter`), structured error sentinels (`ErrStaleState`, `ErrStaleReference` in `common/persistence/transitionhistory/transition_history.go:71-125`), invalidation / verification switches (`service/history/workflow/mutable_state_impl.go:546-560`).
- Extensibility: per-archetype storage via `ArchetypeID` (`service/history/workflow/context.go:155`); CHASM tree layer (`chasm/tree.go:66-204`) co-exists with mutable state; HSM nested machines in `executionInfo.SubStateMachinesByType` (`service/history/workflow/mutable_state_impl.go:651-662`).
- Proven at scale: comes from Uber Cadence lineage (see README:23-27); shard rebalancing, multi-region replication, and backfill are first-class operations backed by branch tokens, version histories, and transition histories.

The only reason not to award a perfect 10: the in-memory representation is monolithic (`MutableStateImpl` is ~10,200 lines, `service/history/workflow/mutable_state_impl.go:1-10247`) and rebuild is linear in event count, which is a known scalability ceiling mitigated only by history trimming (`common/persistence/history_manager.go:212-308`).

## Evidence Collected

Every entry includes a file path and line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Snapshot vs mutation types | `WorkflowMutation` and `WorkflowSnapshot` defined as the only two persistence shapes for closed transactions | `common/persistence/data_interfaces.go:345-398` |
| Internal (blob-encoded) variants | `InternalWorkflowMutation`, `InternalWorkflowSnapshot` carry serialized blobs and DB record version | `common/persistence/persistence_interface.go:435-503` |
| Close-as-mutation code path | `MutableStateImpl.CloseTransactionAsMutation` packages delta fields and `Condition`/`DBRecordVersion` for CAS | `service/history/workflow/mutable_state_impl.go:7484-7528` |
| Close-as-snapshot code path | `MutableStateImpl.CloseTransactionAsSnapshot` packages full pending-state maps; rejects buffered events | `service/history/workflow/mutable_state_impl.go:7530-7573` |
| Buffered-event guardrail | Snapshot path returns `softassert.UnexpectedInternalErr` when buffered events exist | `service/history/workflow/mutable_state_impl.go:7539-7546` |
| Dirty check before commit | `StartTransaction` rejects a non-clean state, logging `MutableStateChecksumInvalidated` | `service/history/workflow/mutable_state_impl.go:7451-7482` |
| Per-field dirty tracking | `isStateDirty()` enumerates 13+ dirty signals including chasm, HSM, signal-requested, time-skipping | `service/history/workflow/mutable_state_impl.go:7420-7445` |
| Transition history stamp | `closeTransactionUpdateTransitionHistory` appends `(NamespaceFailoverVersion, TransitionCount)` and is only updated when `isStateDirty` | `service/history/workflow/mutable_state_impl.go:7840-7865` |
| `VersionedTransition` schema | `{NamespaceFailoverVersion, TransitionCount}` (state-based replication identifier) | `api/persistence/v1/hsm.pb.go:456-465` |
| Staleness check | `transitionhistory.StalenessCheck` rejects out-of-range `(failover_version, transition_count)` with `ErrStaleState` / `ErrStaleReference` | `common/persistence/transitionhistory/transition_history.go:81-125` |
| Unknown-transition clearing | When dirty but transition history not updated, fields are reset and `PreviousTransitionHistory` retained | `service/history/workflow/mutable_state_impl.go:7929-7991` |
| Checksum | CRC32 over a deterministic projection of `executionInfo`, `executionState`, pending-X IDs, CHASM paths | `service/history/workflow/checksum.go:18-90` |
| Checksum gating | Conditional invalidation / verification on load; mismatch is logged and metricked but does not fail the load | `service/history/workflow/mutable_state_impl.go:546-560` |
| State transition counter | `ms.executionInfo.StateTransitionCount += 1` and `LastUpdateTime` set inside `closeTransaction` | `service/history/workflow/mutable_state_impl.go:7754-7755` |
| Last-update per sub-record | `closeTransactionTrackLastUpdateVersionedTransition` stamps per-resource `LastUpdateVersionedTransition` | `service/history/workflow/mutable_state_impl.go:7867-7927` |
| DB record version bump | `ms.dbRecordVersion += 1` inside `closeTransaction` for CAS | `service/history/workflow/mutable_state_impl.go:7764-7768` |
| Context façade | `ContextImpl` wraps `MutableState` and turns domain calls into `UpdateWorkflowExecutionWithNew` / `ConflictResolveWorkflowExecution` / `SubmitClosedWorkflowSnapshot` | `service/history/workflow/context.go:141-1000` |
| Persist orchestrator | `ContextImpl.UpdateWorkflowExecutionWithNew` merges current and (optional) new run, replays events, then calls `Transaction.UpdateWorkflowExecution` | `service/history/workflow/context.go:544-669` |
| Buffered events persisted separately | `buffered_events` SQL / `buffered_events_list` Cassandra columns; flushed on `closeTransactionPrepareEvents` | `schema/postgresql/v12/temporal/schema.sql:83-93`; `schema/cassandra/temporal/schema.cql:47`; `service/history/workflow/mutable_state_impl.go:7687` |
| Schema, executions row | Postgres `executions` keyed by `(shard_id, namespace_id, workflow_id, run_id)`; columns `next_event_id`, `data`, `state`, `db_record_version` | `schema/postgresql/v12/temporal/schema.sql:30-44` |
| Schema, history node | `history_node(shard_id, tree_id, branch_id, node_id, txn_id, prev_txn_id, data, data_encoding)` — events stored as versioned append-only nodes | `schema/postgresql/v12/temporal/schema.sql:312-323` |
| Schema, history tree | `history_tree(shard_id, tree_id, branch_id, data, data_encoding)` — branch metadata (fork ancestry, retention info) | `schema/postgresql/v12/temporal/schema.sql:326-334` |
| Cassandra wide row | Single `executions` row keyed by `(shard_id, type, namespace_id, workflow_id, run_id, visibility_ts, task_id)` containing activity/timer/child/cancel/signal/chasm maps and buffered events | `schema/cassandra/temporal/schema.cql:7-56` |
| Cassandra CAS predicate | `IF next_event_id = ?` (legacy) and `IF db_record_version = ?` (current) on `UPDATE executions` | `common/persistence/cassandra/mutable_state_store.go:90-123` |
| SQL CAS | `txExecuteShardLocked` takes a shard `range_id` lock before any workflow-mutation transaction | `common/persistence/sql/execution.go:40-58` |
| SQL update flow | `updateWorkflowExecutionTx` writes `current_executions` row + applies mutation via `applyWorkflowMutationTx` + (optional) `applyWorkflowSnapshotTxAsNew` | `common/persistence/sql/execution.go:360-444` |
| History append validation | `serializeAppendHistoryNodesRequest` enforces `eventID must be continous`, `event version must be the same inside a batch`, `eventID cannot be less than 1` | `common/persistence/history_manager.go:343-360` |
| History trim | `TrimHistoryBranch` validates the transaction chain via `validateNodeChainAndTrim` and deletes via `DeleteHistoryNodes` | `common/persistence/history_manager.go:212-308` |
| History fork (branching) | `ForkHistoryBranch` produces a new branch token whose `Ancestors` slice references upstream ranges; underlying store accepts the new branch + new node | `common/persistence/history_manager.go:37-127` |
| Branch manager | `BranchMgrImpl.GetOrCreate` finds LCA, copies `VersionHistoryUntilLCA`, calls `ForkHistoryBranch`, then `AddAndSwitchVersionHistory` | `service/history/ndc/branch_manager.go:23-227` |
| History fork (Cassandra) | `HistoryStore.AppendHistoryNodes` writes `history_node` row + (if `IsNewBranch`) `history_tree` row in a logged batch | `common/persistence/cassandra/history_store.go:62-108` |
| Resume via replay | `StateRebuilderImpl.Rebuild` paginates `ReadHistoryBranchByBatch`, builds a fresh mutable state, then `ApplyEvents` rebuilds substate | `service/history/ndc/state_rebuilder.go:103-150`; `service/history/ndc/state_rebuilder.go:224-305` |
| Per-event rebuilder | `MutableStateRebuilderImpl.applyEvents` dispatches one switch statement over all 60+ `enumspb.EventType` cases to mutate state | `service/history/workflow/mutable_state_rebuilder.go:103-775` |
| Reset workflow | `workflowResetterImpl.ResetWorkflow` forks branch, replays history up to `baseRebuildLastEventID` to materialize a new `MutableState` for the new run | `service/history/ndc/workflow_resetter.go:103-261`; `service/history/ndc/workflow_resetter.go:434-508` |
| Conflict resolution path | `ConflictResolveWorkflowExecution` runs three `CloseTransaction*` calls (reset, current, new) and submits them as a single persistence call | `service/history/workflow/context.go:310-426` |
| Workflow context cache | `cacheImpl.getOrCreateWorkflowExecutionInternal` retrieves the `WorkflowContext` and locks it with a `PrioritySemaphore`; on miss it constructs a fresh context | `service/history/workflow/cache/cache.go:255-316` |
| Load from DB | `ContextImpl.LoadMutableState` calls `getWorkflowExecution` → `NewMutableStateFromDB`, which copies `dbRecord.ActivityInfos` / `TimerInfos` / etc. into in-memory `pendingXxxInfoIDs` | `service/history/workflow/context.go:141-231`; `service/history/workflow/mutable_state_impl.go:442-594` |
| Queue ack-level checkpoint | `queueBase.checkpoint` collects per-reader scopes, then writes `shardInfo.QueueStates[categoryID]` | `service/history/queues/queue_base.go:292-354` |
| Checkpoint timer | `p.checkpointTimer = time.NewTimer(backoff.Jitter(CheckpointInterval, ...))` driven by `p.options.CheckpointInterval()` | `service/history/queues/queue_base.go:83-88`; `service/history/queues/queue_base.go:225-242` |
| Range-complete tasks | On watermark advance, `p.rangeCompleteTasks` issues `RangeCompleteHistoryTasks` to drop tasks below the watermark | `service/history/queues/queue_base.go:339-350`; `service/history/queues/queue_base.go:365-387` |
| Checkpoint retry | `resetCheckpointTimer` consults `checkpointRetrier` (exponential backoff) when persistence write fails | `service/history/queues/queue_base.go:411-423` |
| Action-driven move | On alert, `queueBase.handleAlert` immediately runs an action and then forces `p.checkpoint()` | `service/history/queues/queue_base.go:425-437` |
| Shard-level queue state | `ShardContext.GetQueueState` / `SetQueueState` deep-copy `persistencespb.QueueState` (keyed by `int32(categoryID)`) | `service/history/shard/context_impl.go:353-379` |
| Replication queue reader state | `UpdateReplicationQueueReaderState` writes into the same `QueueStates` map | `service/history/shard/context_impl.go:381-398` |
| Shard context update path | `updateShardInfo(tasksCompleted, mutateFn)` persists via execution manager under shard lock | `service/history/shard/context_impl.go:369-379` |
| Buffered events limit | `closeTransactionHandleBufferedEventsLimit` enforces a buffer cap before commit | `service/history/workflow/mutable_state_impl.go:7782-7795` |
| Approximate size tracking | `mutableState.approximateSize` accumulates after every mutation; `enforceMutableStateSizeCheck` force-terminates oversized workflows | `service/history/workflow/context.go:1120-1157` |
| History size / count checks | `enforceHistorySizeCheck` and `enforceHistoryCountCheck` force-terminate workflows that exceed configured limits | `service/history/workflow/context.go:1042-1116` |
| State transition counter exposed | `executionInfo.StateTransitionCount` is incremented and emitted via metric `metrics.StateTransitionCount` | `service/history/workflow/context.go:1217-1231` |
| Rebuild from MutableState + history | `RebuildWithCurrentMutableState` rebuilds history and copies `TransitionHistory`, `PreviousTransitionHistory`, `LastTransitionHistoryBreakPoint` | `service/history/ndc/state_rebuilder.go:152-222` |

## Answers to Dimension Questions

### 1. When is a checkpoint written?

A "checkpoint" in Temporal is ambiguous and the codebase distinguishes three levels:

a. **Per-transaction persistence (synchronous, on every domain call):** every domain action that needs durability closes a transaction through `MutableStateImpl.CloseTransactionAsMutation` (`service/history/workflow/mutable_state_impl.go:7484-7528`) or `CloseTransactionAsSnapshot` (`service/history/workflow/mutable_state_impl.go:7530-7573`). The choice is determined by the persistence mode (`UpdateWorkflowModeUpdateCurrent` / `BypassCurrent` / `IgnoreCurrent`, `CreateWorkflowModeBrandNew` / `UpdateCurrent` / `BypassCurrent`, `ConflictResolveWorkflowModeUpdateCurrent` / `BypassCurrent`). Snapshot is used for creates / continues-as-new / resets; mutation is used for in-place updates.

b. **Per-shard queue-reader checkpoint (asynchronous, timer-driven):** `queueBase.checkpoint` (`service/history/queues/queue_base.go:292-354`) is invoked from the `checkpointTimer.C` channel both in immediate (`queue_immediate.go:155-156`) and scheduled (`queue_scheduled.go:195-196`) queues, and is also forced synchronously after each alert (`queue_base.go:432`).

c. **History branch checkpoint (transactional, on event append):** `AppendHistoryNodes` (`common/persistence/history_manager.go:481-497`) writes `history_node` rows as part of the same persistence transaction as the mutable-state row (see `ExecutionStore.UpdateWorkflowExecution` for Cassandra at `common/persistence/cassandra/execution_store.go:110-126` and SQL at `common/persistence/sql/execution.go:334-358`).

### 2. What is inside it?

`WorkflowSnapshot` / `InternalWorkflowSnapshot` (`common/persistence/data_interfaces.go:378-398`, `common/persistence/persistence_interface.go:475-503`):

- `ExecutionInfo` (`*persistencespb.WorkflowExecutionInfo`) — a large proto containing namespace, workflow ID, task queue, search attributes, memo, last-event bookkeeping, parent-child links, `TransitionHistory []VersionedTransition`, `PreviousTransitionHistory`, `LastTransitionHistoryBreakPoint`, `SubStateMachinesByType`, `UpdateInfos`, `WorkflowTaskLastUpdateVersionedTransition`, etc.
- `ExecutionState` (`*persistencespb.WorkflowExecutionState`) — run ID, state, status, request IDs map, start time, last update.
- `NextEventID` — the next event the mutable state will allocate.
- Full pending-state maps: `ActivityInfos`, `TimerInfos`, `ChildExecutionInfos`, `RequestCancelInfos`, `SignalInfos`, `SignalRequestedIDs`, `ChasmNodes`.
- `Tasks` — generated tasks categorized for queue readers.
- `Condition` (legacy `next_event_id`) and `DBRecordVersion` — optimistic-concurrency tokens.
- `Checksum` — CRC32 over a deterministic snapshot (`service/history/workflow/checksum.go:18-90`).

`WorkflowMutation` (delta) replaces the full maps with `UpsertXxx` / `DeleteXxx` maps and adds `NewBufferedEvents` / `ClearBufferedEvents` (`common/persistence/data_interfaces.go:345-375`).

The blob-encoded internal variants add: `ExecutionInfoBlob`, `ExecutionStateBlob`, `LastWriteVersion`, `StartVersion`, plus per-record `DataBlob` for activity / timer / child / cancel / signal / chasm nodes (`common/persistence/persistence_interface.go:435-503`).

### 3. Can it restore execution or only data?

**Both**, but execution restore is event-sourced, not snapshot-deserialized.

- A *cold* load calls `ContextImpl.LoadMutableState` (`service/history/workflow/context.go:141-231`) which reads the executions row (`executions` table) and **replays only buffered events** through the in-memory `MutableStateImpl`; the events already in `history_node` are not replayed because the executions row is the materialized projection.
- A *full* rebuild reads `history_node` and runs the events through `MutableStateRebuilderImpl.ApplyEvents` (`service/history/workflow/mutable_state_rebuilder.go:70-150`), producing a fresh mutable state from scratch. This is used for: replication resync, workflow reset (`service/history/ndc/workflow_resetter.go:434-508`), admin `RebuildMutableState` (`tests/admin_test.go:26-145`), chasm rebuilder (`tests/chasm_test.go:807-1054`).
- Tasks are *regenerated*, not stored: `taskGenerator.GenerateActivityTimerTasks()` and `GenerateUserTimerTasks()` run after replay (`service/history/workflow/mutable_state_rebuilder.go:89-98`) and the workflow-execution start timer task is regenerated during the `WorkflowExecutionStarted` event apply (`service/history/workflow/mutable_state_rebuilder.go:181-186`). `TaskRefresher.Refresh` is invoked at the end of every replay (`service/history/ndc/state_rebuilder.go:141-143`).

### 4. Can checkpoints branch?

Yes, in two distinct senses:

- **History branches** — `ForkHistoryBranch` (`common/persistence/history_manager.go:37-127`) creates a new branch token whose `Ancestors` slice references the upstream range (`branch_manager.go:188-226`). Branches are produced by reset / continue-as-new / conflict resolution. `history_node` rows are append-only per `(tree_id, branch_id, node_id, txn_id)` (`schema/postgresql/v12/temporal/schema.sql:312-323`).
- **Mutable-state branches via `VersionedTransition`** — `TransitionHistory` is an array of `(failover_version, transition_count)` tuples (`service/history/workflow/mutable_state_impl.go:7859-7862`). When transition history is disabled and state changes happen, the prior history is moved to `PreviousTransitionHistory` and a `LastTransitionHistoryBreakPoint` is recorded (`service/history/workflow/mutable_state_impl.go:7942-7947`). This is the state-based branching model that supports conflict resolution without event-by-event replay.

Branch resolution uses `versionhistory.FindLCAVersionHistoryItemAndIndex` (`common/persistence/versionhistory/version_histories.go:133-158`) and `versionhistory.CopyVersionHistoryUntilLCAVersionHistoryItem` (`common/persistence/versionhistory/version_history.go:36-57`).

### 5. Are checkpoints pruned safely?

Yes, by three coordinated mechanisms:

- **History trimming**: `TrimHistoryBranch` (`common/persistence/history_manager.go:212-308`) walks the txn chain via `validateNodeChainAndTrim`, then issues `DeleteHistoryNodes` for the trimmable prefix. The logic also walks `branchAncestors` to know where the branch begins.
- **Task watermark deletion**: `queueBase.rangeCompleteTasks` (`service/history/queues/queue_base.go:365-387`) deletes tasks below the new `exclusiveDeletionHighWatermark` via `RangeCompleteHistoryTasks`. The order is critical: range-complete before state update, otherwise the shard can reload with un-deletable tasks (`service/history/queues/queue_base.go:332-338`).
- **Branch-reference aware deletion**: `DeleteHistoryBranch` (`common/persistence/history_manager.go:130-209`) walks the history tree, computes reference counts for ancestors, and only deletes unreferenced ranges.

## Architectural Decisions

- **Event-sourced design with materialized mutable state.** `docs/architecture/history-service.md:98-119` states explicitly: "the sequence of History Events alone, for some Workflow Execution, is sufficient to recover all other relevant information about the workflow execution's state". The `executions` row is an index/projection, not a journal.
- **Two write shapes (Snapshot vs Mutation)** rather than a single row overwrite, to express intent: snapshot is used when there is no prior row (create, continue-as-new, reset, conflict-resolve branch), mutation is used for incremental updates. `ContextImpl.UpdateWorkflowExecutionAsActive` uses mutation (`service/history/workflow/context.go:428-485`); `SubmitClosedWorkflowSnapshot` uses snapshot (`service/history/workflow/context.go:678-706`).
- **Per-resource optimistic-concurrency tokens.** Each domain object has its own `LastUpdateVersionedTransition`; the executions row additionally has `db_record_version`. The transition history (`[]VersionedTransition`) records the canonical timeline. `closeTransactionHandleUnknownVersionedTransition` (`service/history/workflow/mutable_state_impl.go:7929-7991`) explicitly rolls state into the unknown-transition state when the writer cannot update the transition history, allowing later replication to reconcile.
- **Branches = fork tokens.** A `HistoryBranch` carries `TreeId`, `BranchId`, `Ancestors []HistoryBranchRange`. The history tree (`history_tree` table) is metadata; the actual events are in `history_node`. Reads walk ancestor ranges via pagination tokens (`common/persistence/history_manager.go:608-666`).
- **Queue ack-levels live on the shard, not on tasks.** `shardInfo.QueueStates` (`service/history/shard/context_impl.go:353-379`) is updated through `updateShardInfo`, which writes the shard row. This makes reader resumption after shard reload cheap at the cost of one shard-write per category per `CheckpointInterval`.
- **Speculative workflow tasks** are first-class in mutable state but explicitly excluded from the dirty check (`service/history/workflow/mutable_state_impl.go:7408-7416`), so they can be live without forcing a flush.
- **CHASM (Coordinated Heterogeneous Application State Machines) is layered on top.** CHASM components have their own per-node `Metadata` + `Data` blobs persisted via `UpsertChasmNodes` / `DeleteChasmNodes` (`service/history/workflow/mutable_state_impl.go:7510-7511`) and reconstructed via `chasm.NewTreeFromDB` (`service/history/workflow/mutable_state_impl.go:575-589`).
- **HSM (Hierarchical State Machines)** sub-state is stored in `executionInfo.SubStateMachinesByType map[string]*StateMachineMap` (`service/history/workflow/mutable_state_impl.go:651-662`) and is replayed via `hsm.NewRoot`.
- **Checksum is best-effort.** It is computed at close (`service/history/workflow/mutable_state_impl.go:7762`), verified on load, and the result is logged + metricked rather than fatal (`service/history/workflow/mutable_state_impl.go:546-560`). The comment explicitly says "we ignore checksum verification errors for now until this feature is tested".

## Notable Patterns

- **Append-only events with chain pointers.** `history_node` rows include `prev_txn_id` so that two transactions on the same node can be ordered by last-write-wins (`schema/cassandra/temporal/schema.cql:58-67`, `common/persistence/history_manager.go:381-385`).
- **Ancestor-aware branch reads.** `readRawHistoryBranch` walks `branchAncestors` as a cursor, allowing reads across fork ancestry without ever materializing the entire tree (`common/persistence/history_manager.go:608-726`).
- **Snapshot-and-replay for `ContinueAsNew`.** `MutableStateInChain` (`service/history/workflow/mutable_state_impl.go:621-649`) creates a fresh state and the workflow resetter forks the branch before replaying (`service/history/ndc/workflow_resetter.go:434-508`).
- **State-based replication alongside event-based.** The codebase supports both: state-based via `SyncVersionedTransitionTask` (see serializer at `common/persistence/serialization/task_serializers.go:1380-1444`) using `VersionedTransition` tuples, and event-based via `HistoryReplicationTask` with a branch token and event IDs.
- **Effect buffer for two-phase commit.** `docs/architecture/effect-package.md:1-33` describes an `effect.Buffer` that allows callbacks to run only after persistence has succeeded, decoupling domain response from database write.
- **Speculative workflow tasks reduce write amplification.** `docs/architecture/speculative-workflow-task.md:1-180` explains that pending WTs are kept in memory and only persisted on success, which avoids unnecessary `history_node` writes.
- **Sticky task queue** is stored in `executionInfo.StickyTaskQueue` and reset on the standby side via `ClearStickyTaskQueue` in the rebuilder (`service/history/workflow/mutable_state_rebuilder.go:122`).
- **CHASM-first CHASM enabled flag** decides whether to materialize a real `chasm.Tree` or a `noopChasmTree` (`service/history/workflow/mutable_state_impl.go:680-693`).

## Tradeoffs

- **Mutable state is monolithic and large.** A single row with many maps means reads/writes contend on one key (`executions` row). Cassandra absorbs this with its wide-row model; SQL stores split maps into child tables (`activity_info_maps`, etc.) (`common/persistence/sql/execution.go:240-326`).
- **Rebuild cost is linear in event count.** Reset and full rebuild re-apply every event from the fork point through `MutableStateRebuilderImpl.applyEvents` (`service/history/workflow/mutable_state_rebuilder.go:103-775`). This is why history trimming (`common/persistence/history_manager.go:212-308`) is essential.
- **Checksum is non-authoritative.** A mismatch is logged and metricked, not fatal (`service/history/workflow/mutable_state_impl.go:552-558`). This trades safety for liveness: a corrupted `executions` row will be loaded, but the corrupted node will likely be detected on the next transaction.
- **State-based replication requires `db_record_version` and `TransitionHistory` to be updated in lockstep.** Failure to do so forces the workflow into the "unknown versioned transition" state (`service/history/workflow/mutable_state_impl.go:7929-7991`), clearing tombstone batches and transition history. Recovery requires the standby cluster to eventually send a state-replication task.
- **Buffered events are persisted separately and flushed lazily.** This is efficient but means an event can live in `buffered_events` indefinitely. `closeTransactionHandleBufferedEventsLimit` (`service/history/workflow/mutable_state_impl.go:7782`) is the only safeguard.
- **Shard `RangeID` is a global fence.** Any history service operation that touches `executions` must take the shard lock first (`common/persistence/sql/execution.go:40-58`); a stale shard holder will lose its lease and fail.
- **Queue checkpoint timer fires on a fixed cadence (`CheckpointInterval`).** Between checkpoints, completed tasks may be processed but not yet "acked", so a shard crash will reprocess them. `docs/architecture/history-service.md:188` calls this out as a deliberate trade.

## Failure Modes / Edge Cases

- **Shard ownership loss mid-transaction**: any persistence write that fails the shard `RangeID` CAS will be rejected; `mutableState.Clear()` is invoked via the deferred `retError` path (`service/history/workflow/context.go:270-274`, `service/history/workflow/context.go:554-558`).
- **Dirty mutable state on entry**: `StartTransaction` refuses to start a new transaction if `IsDirty()` is true and increments `MutableStateChecksumInvalidated` (`service/history/workflow/mutable_state_impl.go:7451-7463`). This is a defensive check for misuse of the API.
- **Versioned-transition staleness**: `transitionhistory.StalenessCheck` returns `ErrStaleState` / `ErrStaleReference` if a ref transition is out of the stored history's range (`common/persistence/transitionhistory/transition_history.go:81-125`). Replication tasks can then be re-requested.
- **History fork race**: `verifyEventsOrder` returns `serviceerrors.NewRetryReplication` (`service/history/ndc/branch_manager.go:171-182`) when the incoming event ID is past the expected next event, prompting the remote side to resend.
- **Snapshot with buffered events**: `CloseTransactionAsSnapshot` returns `softassert.UnexpectedInternalErr` if `bufferEvents` is non-empty (`service/history/workflow/mutable_state_impl.go:7539-7546`).
- **Mutable-state size explosion**: `enforceMutableStateSizeCheck` force-terminates workflows that exceed `MutableStateSizeLimitError` (`service/history/workflow/context.go:1120-1129`).
- **History size / count explosion**: `enforceHistorySizeCheck` and `enforceHistoryCountCheck` terminate over-large histories (`service/history/workflow/context.go:1042-1116`).
- **Buffered event limit**: `closeTransactionHandleBufferedEventsLimit` caps the buffer and may flush events into the history node to free space (`service/history/workflow/mutable_state_impl.go:7782`).
- **CHASM tree corruption on archetype mismatch**: `LoadMutableState` returns `NotFound` if `c.archetypeID != mutableStateArchetypeID` after a cross-archetype collision (`service/history/workflow/context.go:181-204`).
- **Empty branch trim**: `TrimHistoryBranch` returns success without deleting if `validateNodeChainAndTrim` finds a node still being replicated (`common/persistence/history_manager.go:285-293`).

## Future Considerations

- **Transition history cleanup.** When `EnableTransitionHistory` is toggled off → on, `closeTransactionUpdateTransitionHistory` recovers from `PreviousTransitionHistory` (`service/history/workflow/mutable_state_impl.go:7853-7857`). When transition history is unsupported mid-transaction, `closeTransactionHandleUnknownVersionedTransition` clears tombstones (`service/history/workflow/mutable_state_impl.go:7929-7991`). The dynamic flag (`EnableTransitionHistory`) is namespace-scoped (`service/history/workflow/mutable_state_impl.go:7465`).
- **`db_record_version` replacing `Condition` / `next_event_id`.** Multiple `// TODO deprecate` comments flag this transition (`common/persistence/data_interfaces.go:349,371,394`).
- **CHASM always-on.** `ChasmEnabled()` returns true only when the chasm flag is on (`service/history/workflow/mutable_state_impl.go:680-693`); once the flag is removed the noop tree will go away.
- **Effect package migration.** `docs/architecture/effect-package.md:32-33` notes that `workflowTaskResponseMutation` is a candidate to migrate to `effect.Buffer` for cleaner two-phase commit semantics.
- **State-based vs event-based replication convergence.** Both modes are supported; the existence of `SyncVersionedTransitionTask` and `HistoryReplicationTask` in the same persistence layer implies ongoing work to unify them (`common/persistence/serialization/task_serializers.go:1380-1444`).
- **History compression.** Comments around `TrimHistoryBranch` and `validateNodeChainAndTrim` suggest continued work on retention: only trimmable when the txn chain is fully onboarded (`common/persistence/history_manager.go:285-293`).
- **Hierarchical state machines (`hsm`).** `service/history/workflow/hsm/` and `service/history/ndc/hsm_state_replicator.go` indicate that the HSM framework is gaining more state-machine kinds and may eventually subsume parts of the mutable state.
- **CHASM signal backlinks.** `EnableCHASMSignalBacklinks` is a separate dynamic config (`service/history/workflow/mutable_state_impl.go:696-698`), hinting at ongoing migration of signal-request tracking from `pendingSignalRequestedIDs` into CHASM nodes.

## Questions / Gaps

- **Time-skipping integration with CHASM.** A `CONSIDER`/TODO comment explicitly says CHASM support is incomplete: *"todo@time-skipping: but chasm close transaction logic is after isStateDirty, and need to reconsider the sequence of time skipping close trx handling in this function when supporting chasm."* (`service/history/workflow/mutable_state_impl.go:7674-7676`).
- **Backward compat with old `next_event_id` CAS.** `templateUpdateWorkflowExecutionQueryDeprecated` is still in the codebase (`common/persistence/cassandra/mutable_state_store.go:88-106`), suggesting some clients still submit the old condition. Deprecation plan not visible in this slice.
- **No explicit `parent_checkpoint_id` field.** History branches are addressed by opaque `branchToken` bytes; the relationship between base and reset runs is encoded in `BranchInfo.Ancestors` rather than as a foreign key. The ResetWorkflow path stores `baseRunID/baseRebuildLastEventID/baseRebuildLastEventVersion` on the reset mutable state via `SetBaseWorkflow` (`service/history/ndc/workflow_resetter.go:494-498`) — but those fields are local-only and not part of `WorkflowExecutionInfo`.
- **Workflow ID-rate-limit pressure during reset.** `ContextImpl.CreateWorkflowExecution` and `UpdateWorkflowExecutionWithNew` both consult `BusinessIDReuseRateLimiter` (`service/history/workflow/context.go:254-267`, `service/history/workflow/context.go:564-577`), but the rate-limiter is in-process, not persisted. Resets that produce a new run on the same business ID will compete with the original.
- **No "time travel" feature beyond reset.** There is no public API to query arbitrary historical states of mutable state without replaying events. The admin `DescribeMutableState` RPC (`tests/xdc/stream_based_replication_test.go:615-808`) returns the *current* materialized state plus optionally the `cache` view; arbitrary past states are not exposed.
- **Buffered events are not subject to history-size enforcement.** They live in a separate table and have their own limit (`closeTransactionHandleBufferedEventsLimit`) but the trim path (`TrimHistoryBranch`) only operates on `history_node`.

---

Generated by `02.02-snapshot-and-checkpoint-architecture` against `temporal`.