# Source Analysis: temporal

## 02.03 — Event Sourcing and Replay State

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go |
| Analyzed | 2026-07-06 |

## Summary

Temporal implements event sourcing as its foundational state model: every state change for a workflow execution is persisted as an ordered, append-only sequence of `HistoryEvent` protos in the `history_node` table (`schema/cassandra/temporal/versioned/v1.0/schema.cql:53-64`, `common/persistence/cassandra/history_store.go:16-19`). Each event carries an `EventId` (monotonically increasing within a version) and a `Version` (failover-version namespace marker), bound to a `VersionHistoryItem` (`api/history/v1/message.pb.go:73-76`) and ordered into a `VersionHistory` (`api/history/v1/message.pb.go:125-130`). Mutable state is a derived projection that is rebuilt from those events by `StateRebuilderImpl` (`service/history/ndc/state_rebuilder.go:62-150`) and `MutableStateRebuilderImpl` (`service/history/workflow/mutable_state_rebuilder.go:38-101`); the append-only contract is enforced both at the API layer (continuity + same-version invariant at `common/persistence/history_manager.go:339-360`) and at the SQL/Cassandra persistence layer (Cassandra upsert + SQL `INSERT … ON CONFLICT DO UPDATE` keyed by `(shard_id, tree_id, branch_id, node_id, txn_id)` at `common/persistence/sql/sqlplugin/postgresql/events.go:12-16` and `common/persistence/cassandra/history_store.go:16-19`). A newer `VersionedTransition` mechanism (`api/persistence/v1/hsm.pb.go:457-465`, used by `common/persistence/transitionhistory/transition_history.go:51-69`) is layered on top as a compact per-mutation fingerprint to detect stale state without needing to replay events. Replay is explicitly exercised in tests (`service/history/ndc/state_rebuilder_test.go:246-506`, `service/history/workflow/mutable_state_rebuilder_test.go:164-2211`, `service/history/ndc/workflow_resetter_test.go:1037-1746`).

Approach summary: append-only history is authoritative; mutable state is a derived projection; replays are explicit and tested; transitions are fingerprinted by `(namespaceFailoverVersion, transitionCount)`; truncation is reference-counted; versioning is via protobuf field numbers + `DiscardUnknown`.

## Rating

**9 / 10 — Mature, durable, observable, extensible, and proven under failure or scale.**

Rationale:
- The event log is append-only at every layer (API, manager, store, schema) with hard checks for continuity and version monotonicity (`common/persistence/history_manager.go:339-360`, `common/persistence/versionhistory/version_history.go:66-90`).
- State can be rebuilt purely from history via `StateRebuilderImpl.Rebuild` (`service/history/ndc/state_rebuilder.go:103-150`) and `RebuildWithCurrentMutableState` (`service/history/ndc/state_rebuilder.go:152-213`); branch tokens + version history provide continuity.
- Events are effectively immutable in storage (Cassandra `INSERT` upserts only by `(tree_id, branch_id, node_id, txn_id)` and SQL `ON CONFLICT … DO UPDATE` only overwrites the same `(node_id, txn_id)` row, both at `common/persistence/cassandra/history_store.go:16-19` and `common/persistence/sql/sqlplugin/postgresql/events.go:12-16`).
- Schema evolution is handled by protobuf field numbers + `DiscardUnknown` unmarshalling (`common/persistence/serialization/serializer.go:227-229`) and tested at `common/persistence/serialization/serializer_test.go:205-243`; this lets old events replay under new code.
- Replay cost is bounded by `defaultPageSize = 100` batches (`service/history/ndc/constants.go:4`), transaction-size limits (`common/persistence/history_manager.go:367-373`), and history-size / history-count dynamic limits (`common/dynamicconfig/constants.go:386-405`) including `MutableStateSizeLimitError` (`common/dynamicconfig/constants.go:417-420`).
- Extensive test coverage for rebuilder, transition history staleness, and reapply semantics (`service/history/ndc/state_rebuilder_test.go:124-506`, `common/persistence/transitionhistory/transition_history_test.go:11-116`, `service/history/ndc/workflow_resetter_test.go:1037-1746`).
- Operational safeguards: mutable-state cache with TTL + finalizer (`service/history/workflow/cache/cache.go:96-149`), reference-counted branch GC (`common/persistence/history_manager.go:130-208`), and the dual `VersionedTransition` staleness check (`common/persistence/transitionhistory/transition_history.go:71-125`).

No score of 10 because there is no dedicated event-schema *version* field stored per event (Temporal relies on protobuf field tags + `DiscardUnknown` rather than a per-event schema version), and a few failure paths (e.g. `serviceerror.DataLoss` during rebuild, `service/history/ndc/state_rebuilder.go:260-262`) are detected but recovery is a human-in-the-loop retry.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event-log schema (Cassandra) | `history_node` table keyed by `(tree_id, branch_id, node_id, txn_id)`; data is opaque blob with `data_encoding` | `schema/cassandra/temporal/versioned/v1.0/schema.cql:53-64` |
| Event-log schema (SQL) | `addHistoryNodesQuery` UPSERT on `(shard_id, tree_id, branch_id, node_id, txn_id)`; `txn_id *= -1` invariant | `common/persistence/sql/sqlplugin/postgresql/events.go:12-16,55-65` |
| Event-log schema (Cassandra store) | `v2templateUpsertHistoryNode` `INSERT … VALUES (tree_id, branch_id, node_id, prev_txn_id, txn_id, data, data_encoding)` | `common/persistence/cassandra/history_store.go:16-19,62-108` |
| Event-log SQL store impl | `sqlExecutionStore.AppendHistoryNodes` builds `HistoryNodeRow`, enforces upsert, transactional with history_tree for new branches | `common/persistence/sql/history_store.go:24-100` |
| Append-only enforcement | `serializeAppendHistoryNodesRequest` requires continuous `EventId` and uniform `Version` within a batch | `common/persistence/history_manager.go:332-360` |
| Append-only invariant (size) | Per-batch `TransactionSizeLimitError` based on `transactionSizeLimit()` | `common/persistence/history_manager.go:367-373` |
| Append-only invariant (ancestor) | Reject writes above the branch's ancestor range | `common/persistence/history_manager.go:403-407` |
| Event IDs / versioning | `VersionHistoryItem{EventId, Version}` and `VersionHistory{branchToken, items}` | `api/history/v1/message.pb.go:73-130` |
| Version-history monotonicity | `AddOrUpdateVersionHistoryItem` rejects lower version / lower event ID | `common/persistence/versionhistory/version_history.go:66-90` |
| Tests for monotonicity | `TestAddOrUpdateItem_Failed_LowerVersion`, `_Failed_SameVersion_EventIDNotIncreasing`, `_Failed_VersionNoIncreasing` | `common/persistence/versionhistory/version_history_test.go:166-211` |
| Branch management | `ForkHistoryBranch`, `DeleteHistoryBranch` with reference-counted ranges | `common/persistence/history_manager.go:37-208` |
| Branch token utilities | `HistoryBranchRange`, `HistoryBranch`, opaque token parsing | `common/persistence/data_interfaces.go:1420-1438`, `api/persistence/v1/history_tree.pb.go:102-186` |
| Mutable state = derived projection | `MutableStateImpl` keeps `executionInfo`, `executionState`, `hBuilder`, `bufferEventsInDB`; not the source of truth | `service/history/workflow/mutable_state_impl.go:128-200` |
| Loading mutable state | `LoadMutableState` rehydrates from `getWorkflowExecution` and `NewMutableStateFromDB` | `service/history/workflow/context.go:141-220` |
| State rebuilder (NDC) | `StateRebuilderImpl.Rebuild` paginates history via `getPaginationFn`, drives `MutableStateRebuilder.ApplyEvents` | `service/history/ndc/state_rebuilder.go:103-150,329-400` |
| State rebuilder (workflow) | `MutableStateRebuilderImpl.ApplyEvents` switches by `event.GetEventType()`, drives `mutableState.Apply…Event` methods | `service/history/workflow/mutable_state_rebuilder.go:70-230` |
| Event-history pagination | `ReadHistoryBranchByBatch` with `defaultPageSize = 100` | `service/history/ndc/state_rebuilder.go:356-400`, `service/history/ndc/constants.go:4` |
| State rebuilder tests | `TestRebuild`, `TestRebuildWithCurrentMutableState`, `TestApplyEvents`, `TestPagination` | `service/history/ndc/state_rebuilder_test.go:124-506` |
| Mutable state rebuilder tests | 50+ `TestApplyEvents_EventType…` cases | `service/history/workflow/mutable_state_rebuilder_test.go:164-2211` |
| Versioned transition schema | `VersionedTransition{NamespaceFailoverVersion, TransitionCount}` | `api/persistence/v1/hsm.pb.go:457-465` |
| Transition history fields | `WorkflowExecutionInfo.TransitionHistory`, `PreviousTransitionHistory`, `LastTransitionHistoryBreakPoint` | `api/persistence/v1/executions.pb.go:260-274,334-335` |
| Transition history compare | `Compare(a, b)` returns -1/0/1 over `(NamespaceFailoverVersion, TransitionCount)`; nil is smallest | `common/persistence/transitionhistory/transition_history.go:51-69` |
| Transition history staleness | `StalenessCheck` detects `ErrStaleState` / `ErrStaleReference` for split-brain scenarios | `common/persistence/transitionhistory/transition_history.go:71-125` |
| Transition history tests | `TestCompareVersionedTransition`, `TestTransitionHistoryStalenessCheck` | `common/persistence/transitionhistory/transition_history_test.go:11-116` |
| Apply transitions on close | `closeTransactionUpdateTransitionHistory` appends per-transaction fingerprint | `service/history/workflow/mutable_state_impl.go:7840-7865` |
| Transition update helper | `UpdatedTransitionHistory` increments `TransitionCount` and replaces same-version tail | `service/history/workflow/state_transition_history.go:24-45` |
| Transition fingerprinting scope | Tasks and entities are imprinted with `VersionedTransition` (`LastUpdateVersionedTransition` fields) | `service/history/workflow/mutable_state_impl.go:7867-7925`, `api/persistence/v1/executions.pb.go:314-316` |
| Workflow reset = cherry-pick replay | `WorkflowResetter.ResetWorkflow` calls `stateRebuilder.Rebuild`, then `reapplyEvents`/`reapplyEventsFromBranch` | `service/history/ndc/workflow_resetter.go:107-235,767-840` |
| Reapply event switch | `reapplyEvents` dispatches by `event.GetEventType()`, applies HSM cherry-picks | `service/history/ndc/workflow_resetter.go:842-1100` |
| Reapply tests | `TestReapplyEvents` (selective apply), `TestResetterSuite` | `service/history/ndc/workflow_resetter_test.go:1037-1746`, `service/history/ndc/resetter_test.go:55-...` |
| Replication = history delivery | `HistoryReplicatorImpl.BackfillHistoryEvents`, `applyNonStartEventsMissingMutableState` | `service/history/ndc/history_replicator.go:210-329,760-820` |
| Backfill triggers rebuild | On missing mutable state, replication requests retry/resync with event IDs | `service/history/ndc/history_replicator.go:760-820` |
| Cross-cluster replication | `ReplicateHistoryEvents` accepts `versionHistoryItems` + `events` arrays | `service/history/ndc/history_replicator.go:78-97` |
| Importer | `HistoryImporter.ImportWorkflow` takes `events [][]*historypb.HistoryEvent` and rebuilds via `MutableStateRebuilder` | `service/history/ndc/history_importer.go:23-100` |
| Append + upsert path | `serializeWorkflowEvents` → `InternalAppendHistoryNodesRequest`; `serializeWorkflowEventBatches` aggregates per-mutation batches | `common/persistence/execution_manager.go:530-628` |
| Append-on-update | `UpdateWorkflowExecution` calls `serializeWorkflowEventBatches` for both update and new-snapshot paths | `common/persistence/execution_manager.go:168-204` |
| TransactionSizeLimitError | Returned when serialized events exceed `transactionSizeLimit()` | `common/persistence/history_manager.go:369-373` |
| History size/count limits | `HistorySizeLimitError/Warn`, `HistoryCountLimitError/Warn`, `MutableStateSizeLimitError/Warn` dynamic configs | `common/dynamicconfig/constants.go:386-426` |
| Suggest-continue-as-new threshold | `HistorySizeSuggestContinueAsNew = 4*1024*1024` | `common/dynamicconfig/constants.go:390-395` |
| HistoryCountLimit | `HistoryCountLimitError = 50*1024`, `HistoryCountLimitWarn = 10*1024` | `common/dynamicconfig/constants.go:396-405` |
| Data loss detection | `*serviceerror.DataLoss` propagated from `ReadHistoryBranchByBatch` | `service/history/ndc/state_rebuilder.go:257-265` |
| History builder immutability | `HistoryBuilderStateImmutable` / `HistoryBuilderStateSealed` enforced via `assertMutable` / `assertNotSealed` | `service/history/historybuilder/event_store.go:26-30,290-299` |
| Sealed-on-finish | `EventStore.Finish` sets state to `HistoryBuilderStateSealed` in deferred call | `service/history/historybuilder/event_store.go:218-258` |
| NewImmutable replay builder | `NewImmutable` and `NewImmutableForUpdateNextEventID` provide read-only history iterators | `service/history/historybuilder/history_builder.go:101-153` |
| Append-only dedupe at store | SQL store rejects duplicates as `ConditionFailedError` | `common/persistence/sql/history_store.go:60-66` |
| Cassandra overwrite rule | Same `(node_id, txn_id)` upserts, but bigger `txn_id` wins — preserves last-writer-wins for retries | `common/persistence/cassandra/history_store.go:16-19,71-83` |
| Serializer proto+JSON | `Encode` / `Decode` support `ENCODING_TYPE_PROTO3` and `ENCODING_TYPE_JSON`; `DiscardUnknown` on `StrippedHistoryEvent` | `common/persistence/serialization/serializer.go:59-121,193-243` |
| DiscardUnknown test | `ProtoDiscardUnknownFields` confirms old payloads replay under new shapes | `common/persistence/serialization/serializer_test.go:205-243` |
| Event-blob encoding schema | `DataBlob{EncodingType, Data}` stored per node; encoding is recorded explicitly | `common/persistence/history_manager.go:14-17`, `common/persistence/cassandra/history_store.go:78-79` |
| Schema version compatibility | `VerifyCompatibleVersion` ensures DB schema version matches code | `common/persistence/schema/version.go:10-29` |
| Cache TTL on mutable state | `HistoryCacheTTL`, `HistoryHostLevelCacheMaxSizeBytes` bound replay amplification | `service/history/workflow/cache/cache.go:96-149` |
| GC info for branches | `BuildHistoryGarbageCleanupInfo` namespace/workflow/run triple stored alongside appends | `common/persistence/data_interfaces.go:1420-1438` |
| Branch-id UUID per branch | `ForkHistoryBranch` generates `uuid.NewString()` for `BranchId` | `common/persistence/history_manager.go:79-95` |
| Event-blob paging internals | `ReadHistoryBranchByBatch` paginates with `NextPageToken` | `common/persistence/history_manager.go:516-...` |
| Cross-DC XDC cache | `addXDCCacheKV` caches history blobs keyed by `(namespaceID, workflowID, runID, eventID, version)` | `common/persistence/execution_manager.go:574-583` |
| Schema version log marker | `previous_transition_history` retains a checkpoint for back-compat | `api/persistence/v1/executions.pb.go:332-335` |
| Trim history nodes | `TrimHistoryBranch` deletes branch ranges only when not referenced by other branches | `common/persistence/history_manager.go:200-308` |

## Answers to Dimension Questions

### 1. Are events authoritative?

Yes. Every state change to a workflow execution is recorded as a `HistoryEvent` in the `history_node` table (`schema/cassandra/temporal/versioned/v1.0/schema.cql:53-64`, `common/persistence/cassandra/history_store.go:16-19`), and the mutable state in `MutableStateImpl` is a derived cache (`service/history/workflow/mutable_state_impl.go:128-200`) rehydrated from `getWorkflowExecution` (`service/history/workflow/context.go:141-178`). Mutable state writes always piggyback a serialized `WorkflowEvents` batch into `InternalAppendHistoryNodesRequest` (`common/persistence/execution_manager.go:530-628`); the persistence API surface has no update-only path that bypasses history append. The newer `VersionedTransition` layer (`api/persistence/v1/hsm.pb.go:457-465`, `service/history/workflow/state_transition_history.go:24-45`) is a derivative fingerprint, not a competing source of truth.

### 2. Can state be rebuilt from events?

Yes, by design and by test. `StateRebuilderImpl.Rebuild` (`service/history/ndc/state_rebuilder.go:103-150`) pages the history via `ReadHistoryBranchByBatch` (`service/history/ndc/state_rebuilder.go:356-400`) and feeds batches into `MutableStateRebuilderImpl.ApplyEvents` (`service/history/workflow/mutable_state_rebuilder.go:70-101`). A second entry point, `RebuildWithCurrentMutableState` (`service/history/ndc/state_rebuilder.go:152-213`), re-derives mutable state and merges in persisted transition-history data. Tests directly cover both (`service/history/ndc/state_rebuilder_test.go:246-506`) along with the per-event-type application logic (`service/history/workflow/mutable_state_rebuilder_test.go:164-2211`).

### 3. Are events immutable?

Effectively yes.
- Within a batch: same-version and contiguous-`EventId` are enforced (`common/persistence/history_manager.go:339-360`); the API layer rejects malformed batches.
- Within a branch: `AddOrUpdateVersionHistoryItem` enforces strictly non-decreasing `(version, eventID)` (`common/persistence/versionhistory/version_history.go:66-90`); tests verify the rejection paths (`common/persistence/versionhistory/version_history_test.go:166-211`).
- At the store: history is keyed by `(tree_id, branch_id, node_id, txn_id)`. Cassandra upserts only overwrite the same `(node_id, txn_id)` (`common/persistence/cassandra/history_store.go:16-19,71-83`); SQL performs `INSERT … ON CONFLICT … DO UPDATE` keyed on the same tuple (`common/persistence/sql/sqlplugin/postgresql/events.go:12-16`). Higher `txn_id` for the same `node_id` is the "retry wins" rule; otherwise history cannot be back-edited.
- Branch deletion is reference-counted (`common/persistence/history_manager.go:130-208`) so events remain visible to any active branch.

### 4. Can old events still replay after schema changes?

Yes. Events are serialized as protobuf `DataBlob` (`common/persistence/serialization/serializer.go:193-211`) with explicit `EncodingType` recorded per node (`common/persistence/cassandra/history_store.go:78-79`). Two protections make old events replayable under a newer schema:
- Protobuf field-tag-based forward compatibility — the wire format keeps old fields even if the Go struct drops them.
- `DiscardUnknown: true` in `DeserializeStrippedEvents` (`common/persistence/serialization/serializer.go:227-238`) and the explicit test `ProtoDiscardUnknownFields` (`common/persistence/serialization/serializer_test.go:205-243`).
- DB schema compatibility is independently enforced by `VerifyCompatibleVersion` (`common/persistence/schema/version.go:10-29`), but per-event schema evolution uses proto wire compat rather than a per-event schema-version field. There is no event-level `schema_version` integer stored; the design trades that for forward-only protobuf evolution.

### 5. How is replay cost managed?

Multiple coordinated bounds keep replay tractable:
- Page size: `defaultPageSize = 100` events per DB batch during rebuild (`service/history/ndc/constants.go:4`, used in `service/history/ndc/state_rebuilder.go:368`).
- Transaction-size cap: `TransactionSizeLimitError` returned when a serialized batch exceeds `transactionSizeLimit()` (`common/persistence/history_manager.go:367-373`).
- Per-workflow history limits: `HistorySizeLimitError/Warn` and `HistoryCountLimitError/Warn` (`common/dynamicconfig/constants.go:386-405`), with `MutableStateSizeLimitError = 8 MiB` (`common/dynamicconfig/constants.go:417-420`) and `MutableStateTombstoneCountLimit = 16` (`common/dynamicconfig/constants.go:427-430`).
- Continue-as-new nudge: `HistorySizeSuggestContinueAsNew = 4 MiB` (`common/dynamicconfig/constants.go:390-395`).
- Mutable-state cache: TTL-bounded LRU + per-shard finalizer reduces replays from hitting persistence (`service/history/workflow/cache/cache.go:96-149`).
- Branch trimming: `TrimHistoryBranch` deletes un-referenced range segments so old branches don't bloat reads (`common/persistence/history_manager.go:200-308`).
- Branching + duplication of `TransactionID`: a duplicate append is a no-op (`common/persistence/cassandra/history_store.go:71-83`, `common/persistence/sql/history_store.go:60-66`), so retried replication does not blow up history size.

## Architectural Decisions

- **Append-only `history_node` keyed by `(tree_id, branch_id, node_id, txn_id)`** with protobuf-encoded event batches (`schema/cassandra/temporal/versioned/v1.0/schema.cql:53-64`, `common/persistence/cassandra/history_store.go:16-19`, `common/persistence/sql/sqlplugin/postgresql/events.go:12-16`).
- **Versioned branches** via `VersionHistory`/`VersionHistoryItem` (`api/history/v1/message.pb.go:73-130`); branching is a first-class construct used by reset, replication, and continue-as-new (`common/persistence/versionhistory/version_history.go:108-194`).
- **Mutable state as a projection** (`service/history/workflow/mutable_state_impl.go:128-200`), always re-derivable from history.
- **`VersionedTransition` fingerprint per transaction** to detect stale state / stale tasks without full event replay (`api/persistence/v1/hsm.pb.go:457-465`, `common/persistence/transitionhistory/transition_history.go:51-125`, `service/history/workflow/state_transition_history.go:24-45`).
- **Schema evolution via protobuf compatibility + `DiscardUnknown`**, not via per-event version stamps (`common/persistence/serialization/serializer.go:227-238`).
- **Dual staleness signals**: history-continuity checks via version history (`common/persistence/versionhistory/version_history.go:66-90`) and split-brain / stale-state checks via transition history (`common/persistence/transitionhistory/transition_history.go:71-125`).
- **Reference-counted branch GC** to keep multi-branch histories deletable without orphaning active branches (`common/persistence/history_manager.go:130-208`).
- **Mutable-state cache with TTL + finalizer** to bound replay amplification under load (`service/history/workflow/cache/cache.go:96-149`).

## Notable Patterns

- **Per-batch invariants** at the manager layer: `EventId` continuity + uniform `Version` + ancestor check (`common/persistence/history_manager.go:332-407`).
- **Two rebuilder entry points**: `Rebuild` (pure from events) and `RebuildWithCurrentMutableState` (merges persisted transition history), enabling consistent passive-side rebuild (`service/history/ndc/state_rebuilder.go:103-213`).
- **Replay-as-onboarding**: `MutableStateRebuilder.ApplyEvents` switches on `event.GetEventType()` and reissues every mutable-state mutation (`service/history/workflow/mutable_state_rebuilder.go:103-230`).
- **Selective replay (cherry-pick)** for reset, applying only events the new run cares about and excluding others (`service/history/ndc/workflow_resetter.go:842-1100`).
- **State-machine imprinting**: tasks, activity info, timer info, child info, etc. each carry `LastUpdateVersionedTransition`, so stale work is detected cheaply (`service/history/workflow/mutable_state_impl.go:7867-7925`, `api/persistence/v1/executions.pb.go:314-316`).
- **Test-dep generation**: a deep `mockgen`-driven interface layer (`service/history/ndc/state_rebuilder.go:1`, `service/history/workflow/mutable_state_rebuilder.go:3`) enables unit testing of replay without a database.

## Tradeoffs

- **Protobuf field tags vs explicit event schema version.** Storing no per-event `schema_version` simplifies writes but means migration relies on field-tag compatibility + `DiscardUnknown`; a deleted required field would break older events, and Temporal has no on-disk event-schema version to detect that case independently of protobuf evolution (`common/persistence/serialization/serializer.go:227-238`).
- **`TransactionSizeLimitError` + `defaultPageSize = 100`** keep DB transactions small, but a long-running workflow still has linear replay cost unless the mutable-state cache holds it (`common/persistence/history_manager.go:367-373`, `service/history/ndc/constants.go:4`, `service/history/workflow/cache/cache.go:96-149`).
- **Two staleness layers (version history + transition history)**. Version history is dense and complete but only as fast as the log; transition history is compact but only meaningful when enabled (`service/history/workflow/mutable_state_impl.go:357,7447-7465`).
- **`VersionedTransition` adds write amplification** — every close of a transaction updates `TransitionHistory` and stamps many child entities (`service/history/workflow/mutable_state_impl.go:7840-7925`).
- **Branching keeps history bloat in check but complicates queries.** `VersionHistoryItem` and `LCA` resolution (`common/persistence/versionhistory/version_history.go:108-135`) are required to interpret an event ID.

## Failure Modes / Edge Cases

- **Data loss detected during rebuild**: `*serviceerror.DataLoss` returned by `ReadHistoryBranchByBatch` is re-raised by `stateRebuilder` (`service/history/ndc/state_rebuilder.go:260-262`) — there is no automatic recovery; the operator must reconcile.
- **Boundary mismatch**: rebuilds require `baseLastEventID/baseLastEventVersion` to match the last `VersionHistoryItem`; otherwise `serviceerror.NewInvalidArgument` is raised (`service/history/ndc/state_rebuilder.go:292-303`).
- **Stale state**: `StalenessCheck` produces `ErrStaleState` / `ErrStaleReference` for split-brain, post-replication scenarios (`common/persistence/transitionhistory/transition_history.go:71-125`, tested at `common/persistence/transitionhistory/transition_history_test.go:71-116`).
- **Duplicate events**: dedup at the persistence layer returns `ConditionFailedError` for SQL (`common/persistence/sql/history_store.go:60-66`); Cassandra upserts silently overwrite with same `(node_id, txn_id)` (`common/persistence/cassandra/history_store.go:71-83`). Logic differences mean duplicate handling is partly store-specific.
- **Mutation collapse**: `UpdatedTransitionHistory` collapses same-version transitions, losing per-mutation granularity (`service/history/workflow/state_transition_history.go:24-45`) — a tradeoff that makes the log compact but reduces the resolution of staleness detection.
- **Mutable-state cache miss** forces a full rebuild + `taskRefresher.Refresh` — itself non-trivial work (`service/history/ndc/state_rebuilder.go:141-143`).
- **Re-apply excludes update-completed and certain other events** during reset to keep semantics stable; misconfiguration of `resetReapplyExcludeTypes` can drop user-relevant history (`service/history/ndc/workflow_resetter.go:842-1100`).
- **No event-level schema version** means dropping a once-required proto field cannot be detected at the storage layer — old events would fail to deserialize unless the new code keeps the field tag as reserved.

## Future Considerations

- **Per-event schema versioning.** A small `schema_version` integer on `DataBlob` (or a header in the protobuf wrapper) would let future code detect when events predate a migration and either run them through a converter or refuse them gracefully. Currently this is implicit in protobuf tags (`common/persistence/serialization/serializer.go:59-121`).
- **Snapshotting atop events.** A periodic compressed snapshot of `MutableStateImpl` plus the `LastTransitionHistoryBreakPoint` could cap rebuild cost for very long histories; `MutableStateSizeLimitError = 8 MiB` (`common/dynamicconfig/constants.go:417-420`) currently pushes workflows toward `ContinueAsNew` instead (`common/dynamicconfig/constants.go:390-395`).
- **Replay observability.** A trace/logging layer around `StateRebuilderImpl.Rebuild` could expose which history range and how many bytes were replayed per request to tune `defaultPageSize` (`service/history/ndc/constants.go:4`).
- **Standardize the rebuilder API.** There are at least three rebuilder paths (`Rebuild`, `RebuildWithCurrentMutableState`, `MutableStateRebuilder.ApplyEvents`) with overlapping responsibilities; consolidating them would reduce maintenance cost (`service/history/ndc/state_rebuilder.go:103-213`, `service/history/workflow/mutable_state_rebuilder.go:70-101`).
- **Test convergence.** The 50+ `TestApplyEvents_*` cases in `service/history/workflow/mutable_state_rebuilder_test.go:164-2211` could be table-driven to make adding new event types cheaper.
- **Make `TransactionSizeLimitError` configurable** to give operators a way to tune replay batch cost alongside `defaultPageSize` (`common/persistence/history_manager.go:367-373`, `service/history/ndc/constants.go:4`).
- **Hardening for DataLoss auto-recovery.** Today a `*serviceerror.DataLoss` is fatal for the in-flight request (`service/history/ndc/state_rebuilder.go:260-262`); auto-fallback to last known-good snapshot or a peer would reduce operational toil.

## Questions / Gaps

- **No event-level schema version.** Searched `common/persistence/serialization/` and `schema/` (`common/persistence/serialization/codec.go:1-121`); the only versioning concept is `ENCODING_TYPE_*` on `DataBlob` and the DB schema version check at `common/persistence/schema/version.go:10-29`. No `EventBlobVersion` / `EventBlobSchemaVersion` integer stored on the blob itself.
- **No standalone "event log" abstraction.** History persistence is folded into the broader `ExecutionManager` interface (`common/persistence/execution_manager.go:1-279`, `common/persistence/data_interfaces.go:1420-1438`); the term "event log" does not appear as a named interface.
- **Replay test coverage focuses on the rebuilder.** `service/history/ndc/state_rebuilder_test.go:124-506` covers pagination, apply-events, and rebuild stats; `service/history/workflow/mutable_state_rebuilder_test.go:164-2211` covers per-event-type reapplication; cross-cluster replay end-to-end is integration-tested (`tests/` is the integration suite, not inspected here per source-isolation rules).
- **`BackfillHistoryEvents` failure handling.** The `*serviceerror.SyncState` fallback for missing mutable state (`service/history/ndc/history_replicator.go:260-269`) is an in-band retry signal rather than a fully observable event; the exact policy for converting it into a recovery action is in the receiver (out of scope here).
- **`MutableStateTombstoneCountLimit = 16`** (`common/dynamicconfig/constants.go:427-430`) constrains delete-marker churn but does not directly bound history size; the relationship is implicit.

---

Generated by `02.03-event-sourcing-and-replay-state` against `temporal`.