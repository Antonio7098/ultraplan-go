# Source Analysis: temporal

## 02.04 — Mutation Discipline and State Transitions

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.x (Temporal Server — workflow engine, persistence, replication, chasm, hsm, ndc) |
| Analyzed | 2026-07-10 |

## Summary

Temporal Server enforces mutation discipline through a layered, defense-in-depth architecture that combines (a) declarative state-machine frameworks (`hsm` and `chasm`) that turn writes into typed transitions guarded by source/destination predicates, (b) per-execution validators that reject illegal `WorkflowExecutionState × WorkflowExecutionStatus` combinations before they ever hit persistence, (c) shard-level lease-style optimistic locking via `RangeID` plus per-row `DBRecordVersion`, and (d) a per-workflow execution context (`workflow.ContextImpl`) that holds a `locks.PrioritySemaphore(1)` so only one in-flight transaction can mutate a workflow at a time.

Two distinct state-machine frameworks coexist. `service/history/hsm` (Hierarchical State Machine) models tree-structured per-workflow state via `Node`, `Transition[S,SM,E]` (`service/history/hsm/sm.go:32-66`) and `MachineTransition` (`service/history/hsm/tree.go:579-629`) which atomically bumps `TransitionCount`, sets `LastUpdateVersionedTransition`, marks the node dirty, and **rolls back on error** via `defer`. `chasm` (Coordinated Heterogeneous Application State Machines) models arbitrary typed component sub-trees via `Transition[S,SM,E]` with `MutableContext` (`chasm/statemachine.go:24-79`) that always emits a `chasm.transition` OTel span event (`chasm/statemachine.go:55-67`), plus a full transaction lifecycle (`CloseTransaction` at `chasm/tree.go:1630-1681`) that synchronises structure, reconciles tasks, applies lifecycle-state changes to `WorkflowExecutionState/Status`, and emits `state_transition_count` metrics (`common/metrics/metric_defs.go:703`).

Mutation observability is structured: HSM emits an `OperationLog` (`TransitionOperation`, `DeleteOperation`) appended atomically with each transition (`service/history/hsm/tree.go:619-626`), and CHASM produces OTel span events for every transition attempt (success or failure). Every persisted mutation carries a monotonically increasing `VersionedTransition{TransitionCount, NamespaceFailoverVersion}` that flows from the `MutableStateImpl` backend through `NextTransitionCount()` (`service/history/workflow/mutable_state_impl.go:1185-1198`).

## Rating

**9 / 10** — Mature, durable, observable, extensible, and proven under failure or scale.

Rationale:

- **+** Transitions are first-class objects with explicit source-state predicates and destination states (`hsm/sm.go:32-66`, `chasm/statemachine.go:24-79`); impossible transitions are rejected with `ErrInvalidTransition` before any mutation lands (`hsm/sm.go:60-62`, `chasm/statemachine.go:70-72`).
- **+** HSM `MachineTransition` (`hsm/tree.go:579-629`) atomically increments `TransitionCount`, snapshots `prevLastUpdatedVersionedTransition`, runs the transition function, **and rolls back** on any error via deferred cleanup (`hsm/tree.go:600-607`) — including forced cache invalidation so stale data cannot be observed.
- **+** CHASM enforces access control in `validateAccess`/`validateAccessHelper` (`chasm/tree.go:486-563`) before any mutation: read-only `OperationIntentObserve` skips the check, `OperationIntentProgress` walks every ancestor and refuses writes if any is `IsClosed()` or paused. The `terminated` in-memory flag plus `backend.GetExecutionState().State == WORKFLOW_EXECUTION_STATE_COMPLETED` check (`chasm/tree.go:549-560`) closes the door on post-completion writes.
- **+** `MutableStateImpl.UpdateWorkflowStateStatus` (`workflow/mutable_state_impl.go:7392-7406`) and `setStateStatus` (`workflow/mutable_state_state_status.go:16-125`) encode a full legal-transition matrix for `WorkflowExecutionState × WorkflowExecutionStatus` (CREATED→{RUNNING|COMPLETED|ZOMBIE}, RUNNING→{RUNNING|COMPLETED|ZOMBIE}, COMPLETED→{COMPLETED only with same status}, etc.), with a 5-state × 8-status test matrix in `common/persistence/workflow_state_status_validator_test.go:39-186`.
- **+** Persistence layer uses a two-key optimistic-concurrency contract: shard `RangeID` (in `ShardOwnershipLostError` semantics at `service/history/shard/context_impl.go:1421-1426`) plus row-level `DBRecordVersion` (`common/persistence/persistence_interface.go:414`, `:448`, `:488`, `:500`) carried into `Condition` on every `InternalWorkflowMutation`/`InternalWorkflowSnapshot`. SQL plugin uses `FOR UPDATE`/`LOCK IN SHARE MODE` row locks (`common/persistence/sql/sqlplugin/mysql/execution.go:28-32`) for upgrade-safe reads.
- **+** Per-execution serialisation: `workflow.ContextImpl` uses `locks.PrioritySemaphore(1)` (`service/history/workflow/context.go:43`, `:73`) and exposes `Lock`/`Unlock` (`workflow/context.go:84-93`) so that only one mutator at a time can hold a workflow context. `MutableStateImpl.StartTransaction` (`workflow/mutable_state_impl.go:7451-7463`) actively rejects starting a new transaction if the previous one was left dirty — surfacing programming errors as `Unavailable` errors plus a `MutableStateChecksumInvalidated` metric.
- **+** Observability: CHASM transitions emit `chasm.transition.source/destination/error` OTel span events (`chasm/statemachine.go:55-67`), HSM maintains a per-node `OperationLog` with explicit `TransitionOperation`/`DeleteOperation` types and a deterministic `compact()` algorithm (`hsm/tree.go:709-805`), and `emitStateTransitionCount` records `state_transition_count` histograms on every commit (`workflow/context.go:1217-1231`, `common/metrics/metric_defs.go:703`).
- **+** Impossible-state suppression via the type system: `LifecycleState` (`chasm/component.go:62-86`) classifies OPEN vs CLOSED via `IsClosed()` (`s >= LifecycleStateCompleted`) and `IsPaused()`, so terminal states are not representable as transition *sources* in the HSM transition graphs that components declare (see `chasm/lib/activity/statemachine.go:38-87` where every `TransitionScheduled`/`Started`/`Completed`/`Failed`/`CancelRequested`/`Canceled` declares a strict source-state list).
- **−** `MutableStateImpl` exposes ~40 call sites that mutate `executionState` via `UpdateWorkflowStateStatus` (`workflow/mutable_state_impl.go:7392-7406`); the guard is delegated entirely to `setStateStatus`, which is the right design but means there is no compile-time proof that all callsites route through it (no helper like `withStateStatusGuard`). A code reviewer must trust the convention.
- **−** Replication paths (`MutableStateImpl.ApplyMutation` at `workflow/mutable_state_impl.go:9111-9155` and `ApplySnapshot` at `:9157-9195`) overwrite `ms.executionState = mutation.ExecutionState` **without** re-validating via `setStateStatus`; correctness relies on the source cluster having validated first. Comment in the validator `chasm/tree.go:2512-2515` uses `transitionhistory.Compare` for last-write-wins, which is safe but is a non-typed comparison.
- **−** Some legacy code paths still allow direct field mutation outside the framework (e.g. `chasm/tree.go:129-130` documents the in-memory `terminated` and `deleteAfterClose` flags as deliberately not persisted, which is fine for force-terminate but is "in-memory only" cleverness that future maintainers could accidentally rely on).
- **−** `chasm/statemachine.go:14` returns a `serviceerror.NewFailedPrecondition` from `ErrInvalidTransition`, which is the right wrapper, but HSM's sibling (`hsm/sm.go:10`) returns a bare `errors.New("invalid transition")`. The mismatch is small but means callers must use `errors.Is` carefully — already tested in `hsm/sm_test.go:48` but the asymmetry is intentional friction.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.go:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| HSM transition primitive | `Transition[S comparable, SM StateMachine[S], E any]` with `Sources`/`Destination`/`apply` | `service/history/hsm/sm.go:32-66` |
| HSM transition guard | `Possible()` checks `slices.Contains(t.Sources, sm.State())`; `Apply` errors on mismatch | `service/history/hsm/sm.go:52-66` |
| HSM invalid-transition sentinel | `ErrInvalidTransition` + `from %v: %v` message | `service/history/hsm/sm.go:9-10`, `:61` |
| HSM atomic mutation + rollback | `MachineTransition` bumps `TransitionCount`, snapshots `prevLastUpdatedVersionedTransition`, defers rollback | `service/history/hsm/tree.go:579-629` |
| HSM rollback | `defer func() { if retErr != nil { … cache.dataLoaded = false } }` | `service/history/hsm/tree.go:600-607` |
| HSM dirty tracking | `cache.dirty = true` only after successful serialize | `service/history/hsm/tree.go:617` |
| HSM deletion invariant | `DeleteChild` rejects on deleted node and cascades `cache.deleted = true` | `service/history/hsm/tree.go:406-439` |
| HSM operation log | `OperationLog`, `TransitionOperation`, `DeleteOperation`, deterministic `compact()` | `service/history/hsm/tree.go:87-148`, `:709-805` |
| HSM tasks validate state | `ValidateState` returns `ErrStaleReference` when state mismatches expected | `service/history/hsm/tasks.go:64-81` |
| HSM event-application contract | `EventDefinition.CherryPick` documented to return `ErrInvalidTransition`/`ErrStateMachineNotFound` to skip | `service/history/hsm/events.go:18-26` |
| HSM state-machine interface | `StateMachine[S comparable]` with `State()/SetState()` | `service/history/hsm/sm.go:20-24` |
| HSM transition tests | `TestTransition_Possible`, `TestTransition_ValidTransition`, `TestTransition_InvalidTransition`, `TestTransition_HandlerError` | `service/history/hsm/sm_test.go:27-56` |
| CHASM transition primitive | `Transition[S,SM,E]` with `Sources`/`Destination`/`apply` taking `MutableContext` | `chasm/statemachine.go:24-31` |
| CHASM transition guard | `Possible()` uses `slices.Contains`; `Apply` errors on mismatch | `chasm/statemachine.go:45-72` |
| CHASM transition ordering | apply runs first, then `SetStateMachineState(t.Destination)` (mutable side-effect inside apply) | `chasm/statemachine.go:74-78` |
| CHASM observability | OTel `span.AddEvent("chasm.transition", source/destination/error)` | `chasm/statemachine.go:55-67` |
| CHASM access control | `validateAccess` walks ancestors, blocks writes if any is `IsClosed()`/`IsPaused()`/`terminated` | `chasm/tree.go:486-563` |
| CHASM terminated/closed gate | combined in-memory `terminated` + persistence `GetExecutionState().State == COMPLETED` | `chasm/tree.go:549-560` |
| CHASM intent gating | `OperationIntentObserve` skips check, `OperationIntentProgress` enforces | `chasm/tree.go:486-491` |
| CHASM detached bypass | `isDetached()` skips ancestor validation; comment documents backward-compat for callback/visibility | `chasm/tree.go:661-674` |
| CHASM node deletion invariant | `cache.deleted = true` set on subtree; `MachineTransition` errors on deleted node | `service/history/hsm/tree.go:580-582`, `chasm/tree.go` value-state machine at `:68-76` |
| CHASM transaction lifecycle | `CloseTransaction` syncs sub-components → resolves pointers → handles lifecycle → forces visibility → serialises → updates tasks | `chasm/tree.go:1630-1681` |
| CHASM lifecycle → state/status | `closeTransactionHandleRootLifecycleChange` maps `LifecycleState` to `WorkflowExecutionState/Status` | `chasm/tree.go:1727-1778` |
| CHASM value-state machine | `valueStateUndefined / NeedDeserialize / Synced / NeedSerialize / NeedSyncStructure` ordered | `chasm/tree.go:68-76` |
| CHASM dirty propagation | `isActiveStateDirty` set when `valueState >= valueStateNeedSerialize` | `chasm/tree.go:413-418`, `:172` |
| CHASM mutation maps | `NodesMutation{UpdatedNodes, DeletedNodes}` for replication and apply | `chasm/tree.go:186-192` |
| CHASM apply mutation | `ApplyMutation` documents overlap rejection and walks deleted-first | `chasm/tree.go:2435-2478` |
| CHASM apply snapshot | `ApplySnapshot` synthesises a mutation from current vs incoming snapshots | `chasm/tree.go:2480-2522` |
| CHASM fail-fast invariants | `softassert.UnexpectedInternalErr` on unknown component types, fields, lifecycle states | `chasm/tree.go:450,572,580,608,634,690,753,806,830,899,911,933,983,1043,1057,1123,1132` |
| CHASM component lifecycle | `LifecycleState` enum + `IsClosed()`/`IsPaused()` predicates | `chasm/component.go:62-86` |
| CHASM context split | `Context` (immutable) vs `MutableContext` (write) interfaces | `chasm/context.go:17-108` |
| CHASM real transitions example | Activity state machine: Scheduled → Started → Completed/Failed/CancelRequested → Canceled | `chasm/lib/activity/statemachine.go:36-87`, `:139-318` |
| CHASM transition telemetry | uses `telemetry.DebugMode()` gate so production does not pay cost | `chasm/statemachine.go:55-56` |
| Workflow state validator | `setStateStatus` 5×5 transition matrix with strict source→destination rules | `service/history/workflow/mutable_state_state_status.go:16-125` |
| Workflow state validator helper | `invalidStateTransitionErr` wraps current/target/state in serviceerror | `service/history/workflow/mutable_state_state_status.go:127-138` |
| Workflow validator create-mode | `ValidateCreateWorkflowStateStatus` enforces COMPLETED+{RUNNING\|PAUSED} forbidden, etc. | `common/persistence/workflow_state_status_validator.go:30-54` |
| Workflow validator update-mode | `ValidateUpdateWorkflowStateStatus` enforces RUNNING+{RUNNING\|PAUSED} only, COMPLETED+same status only | `common/persistence/workflow_state_status_validator.go:56-85` |
| Workflow validator tests | 5 sub-suites for create/update × state × status combinations | `common/persistence/workflow_state_status_validator_test.go:39-186` |
| Workflow update-mode × state validator | `ValidateUpdateWorkflowModeState` 3-case matrix for `UpdateCurrent`/`BypassCurrent`/`IgnoreCurrent` | `common/persistence/operation_mode_validator.go:50-138` |
| Workflow conflict-resolve validator | `ValidateConflictResolveWorkflowModeState` 4-case matrix | `common/persistence/operation_mode_validator.go:140-290` |
| MutableState update entry point | `UpdateWorkflowStateStatus` short-circuits on identity, sets `executionStateUpdated` flag | `service/history/workflow/mutable_state_impl.go:7392-7406` |
| MutableState IsDirty | `IsDirty` + `isStateDirty` combine history-builder, sub-state flags, chasm tree, hsm node | `service/history/workflow/mutable_state_impl.go:7408-7445` |
| MutableState StartTransaction guard | Rejects if previous tx dirty; emits `MutableStateChecksumInvalidated` metric | `service/history/workflow/mutable_state_impl.go:7451-7463` |
| MutableState NextTransitionCount | Returns current `VersionedTransition.TransitionCount+1` | `service/history/workflow/mutable_state_impl.go:1185-1198` |
| MutableState ApplyMutation | Replays sub-state-machines; bypasses `setStateStatus` for incoming execution state | `service/history/workflow/mutable_state_impl.go:9111-9155` |
| MutableState ApplySnapshot | Replays sub-state-machines from snapshot | `service/history/workflow/mutable_state_impl.go:9157-9195` |
| Per-execution lock | `ContextImpl.lock: locks.PrioritySemaphore(1)` plus `Lock`/`Unlock` | `service/history/workflow/context.go:43`, `:73`, `:84-93` |
| Per-execution guard | `IsDirty()` exposes `MutableState.IsDirty()` for the lock holder | `service/history/workflow/context.go:95-100` |
| Per-execution lifecycle | `Clear()` tears down query/update registries on rollback | `service/history/workflow/context.go:102-113` |
| Execution lease | `ChasmEngine.lockCurrentExecution` uses `locks.PriorityHigh` over the cache | `service/history/chasm_engine.go:791-811` |
| Shard range ID | `s.GetRangeID()` + `getRangeIDLocked()` plumbing for write requests | `service/history/shard/context_impl.go:212-217`, `:1117` |
| Shard write-error policy | `handleWriteErrorLocked` maps `ShardOwnershipLostError` to engine stop | `service/history/shard/context_impl.go:1388-1437` |
| Persistence Condition field | `Condition int64` on `InternalWorkflowMutation` and `InternalWorkflowSnapshot` | `common/persistence/persistence_interface.go:469`, `:500` |
| Persistence DBRecordVersion | `DBRecordVersion int64` on `InternalWorkflowMutableState`/`InternalWorkflowMutation`/`InternalWorkflowSnapshot` | `common/persistence/persistence_interface.go:414`, `:448`, `:488` |
| SQL row locking | `FOR UPDATE` write lock, `LOCK IN SHARE MODE` read lock queries | `common/persistence/sql/sqlplugin/mysql/execution.go:28-32` |
| SQL update entry | `UpdateExecutions` uses named-exec with `db_record_version` column | `common/persistence/sql/sqlplugin/mysql/execution.go:13-19`, `:195-204` |
| SQL read lock entry | `ReadLockExecutions` returns `db_record_version` + `next_event_id` | `common/persistence/sql/sqlplugin/mysql/execution.go:239-249` |
| ExecutionManager validation | `executionManagerImpl` validates state+status before persistence and validates update-mode × state | `common/persistence/execution_manager.go:130-138`, `:206-214` |
| Visibility on transitions | `emitStateTransitionCount` records `state_transition_count` histogram | `service/history/workflow/context.go:1217-1231` |
| Metric definition | `StateTransitionCount = NewDimensionlessHistogramDef("state_transition_count")` | `common/metrics/metric_defs.go:703` |
| Soft-assert utility | `softassert.That`, `Fail`, `UnexpectedInternalErr`, `UnexpectedDataLoss` | `common/softassert/softassert.go:28-45`, `common/softassert/serviceerror.go:21-56` |

## Answers to Dimension Questions

1. **Can any module mutate state?**
   No. State can only be mutated through:
   - **HSM**: `MachineTransition` (`service/history/hsm/tree.go:579`) or `Collection.Transition` (`:693`), both of which call `MachineTransition`. Direct `SetState` is exposed but the framework's `cache.dirty` flag plus `OperationLog` only updates via `MachineTransition`, so the *intended* path is guarded.
   - **CHASM**: `Component.MutableContext` (`chasm/context.go:86-108`) is the only way to call `AddTask`, `SetRequestLinks`, `SetUserMetadata`; the framework only reaches those methods if the context is a `mutableCtx`. Read-only `Context` returns `immutableCtx` (`chasm/context.go:130-153`, `:155-244`) which has no mutating methods.
   - **Workflow mutable state**: All state/status mutations go through `UpdateWorkflowStateStatus` (`service/history/workflow/mutable_state_impl.go:7392-7406`) which delegates to `setStateStatus` (`service/history/workflow/mutable_state_state_status.go:16`). Persistence-layer entries call `ValidateCreateWorkflowStateStatus`/`ValidateUpdateWorkflowStateStatus` (`common/persistence/workflow_state_status_validator.go:30`, `:56`) before any DB write.
   - **Locking**: Per-execution `workflow.ContextImpl.lock` (`service/history/workflow/context.go:43`) and shard-level `s.rwLock`/`ioSemaphore` (`service/history/shard/context_impl.go:126-138`, `:269-272`) serialise writers.

2. **Are illegal transitions prevented?**
   Yes, at three layers:
   - **Type-level guards**: HSM `Transition.Sources` (`hsm/sm.go:32-39`) and CHASM `Transition.Sources` (`chasm/statemachine.go:25-31`) are explicit; `Apply` errors with `ErrInvalidTransition` when the current state is not in `Sources`.
   - **Access guards**: CHASM `validateAccess` rejects writes to closed or paused ancestors (`chasm/tree.go:486-563`); the in-memory `terminated` flag plus persistence `State == COMPLETED` check prevents post-completion writes.
   - **Workflow-level guard**: `setStateStatus` (`workflow/mutable_state_state_status.go:16-125`) enumerates legal transitions for the full `WorkflowExecutionState × WorkflowExecutionStatus` space. Illegal combinations are returned as `serviceerror.NewInternalf` with the literal message `unable to change workflow state from %v to %v, status %v` (`:11`, `:127-138`).

3. **Are concurrent writes safe?**
   Yes, via a layered design:
   - **In-process serialisation**: `workflow.ContextImpl` holds a `PrioritySemaphore(1)` (`workflow/context.go:43`, `:73`); `Lock`/`Unlock` (`workflow/context.go:84-93`) plus `MutableStateImpl.StartTransaction` (`workflow/mutable_state_impl.go:7451-7463`) refuse to start a new transaction if the previous one was left dirty.
   - **Cross-process optimistic concurrency**: `DBRecordVersion` (`common/persistence/persistence_interface.go:414`, `:448`, `:488`) is plumbed into the `Condition` field (`:469`, `:500`) on every `InternalWorkflowMutation`/`InternalWorkflowSnapshot`; SQL plugin enforces row locks via `FOR UPDATE` (`common/persistence/sql/sqlplugin/mysql/execution.go:31`).
   - **Shard-level lease**: `RangeID` (`service/history/shard/context_impl.go:212-217`, `:1117`) is sent with every write; `handleWriteErrorLocked` (`:1388-1437`) maps `ShardOwnershipLostError` to engine shutdown.

4. **Are mutations observable?**
   Yes, with multiple independent channels:
   - **OTel span events**: CHASM `Transition.Apply` always emits `chasm.transition` with source/destination/error attrs when `telemetry.DebugMode()` is true (`chasm/statemachine.go:55-67`).
   - **Operation log**: HSM `MachineTransition` appends `TransitionOperation{path, TransitionCount}` (`service/history/hsm/tree.go:619-626`); deletions append `DeleteOperation` (`:425-427`); `OperationLog.compact()` (`:709-805`) deterministically prunes against deletion.
   - **Metrics**: `emitStateTransitionCount` (`workflow/context.go:1217-1231`) records `state_transition_count` (`common/metrics/metric_defs.go:703`) per transaction.
   - **Dirty tracking**: `MutableStateImpl.IsDirty` / `isStateDirty` (`workflow/mutable_state_impl.go:7408-7445`) report what changed.

5. **Can state become internally inconsistent?**
   Largely prevented:
   - **HSM**: `MachineTransition` rolls back on error (`hsm/tree.go:600-607`); the deferred function decrements `TransitionCount`, restores `LastUpdateVersionedTransition`, and invalidates the cache so the next read cannot observe half-applied state.
   - **CHASM**: `CloseTransaction` is a single pass that atomically (a) syncs sub-components, (b) handles root-lifecycle changes, (c) force-updates visibility, (d) serialises nodes, (e) updates tasks, (f) applies pending metadata (`chasm/tree.go:1630-1681`). A `softassert` (`chasm/tree.go:1874`) fires if a node's `valueState` is `> valueStateNeedSerialize` mid-serialise, which is a programming-error safeguard.
   - **Workflow**: `MutableStateImpl.StartTransaction` (`workflow/mutable_state_impl.go:7451-7463`) refuses to start a new tx if `IsDirty()` is true.
   - **Edge case**: `MutableStateImpl.ApplyMutation` (`workflow/mutable_state_impl.go:9111-9155`) trusts the incoming `mutation.ExecutionState` without re-running `setStateStatus`. The system relies on the source cluster having validated; this is documented in `chasm/tree.go:2462-2475` and is safe given the global `transitionhistory.Compare` ordering on `LastUpdateVersionedTransition`.

> **Can impossible states be represented?**
> Mostly no. `LifecycleState` (`chasm/component.go:62-86`) classifies open vs closed via `IsClosed()`/`IsPaused()`; component code declares valid transitions as `Sources` lists, so e.g. an Activity cannot transition from `COMPLETED` back to `SCHEDULED` (`chasm/lib/activity/statemachine.go:38-87`, `:139-318`). The CHASM `validateAccess` helper (`chasm/tree.go:549-560`) actively rejects writes to closed components. The only area where impossible states could be represented is the in-memory `terminated` flag (`chasm/tree.go:123`), but its lifecycle is bounded to a single transaction by documentation (`chasm/tree.go:113-122`).

## Architectural Decisions

- **Two parallel state-machine frameworks (HSM and CHASM)**. HSM is older and event-driven (history events drive transitions via `EventDefinition.Apply`/`CherryPick` — `service/history/hsm/events.go:13-26`), CHASM is newer and operation-driven (typed `TransitionOption`s and a richer `MutableContext`). Both share the same "transition with sources/destination" pattern, indicating a deliberate converged abstraction rather than coincidence.
- **Atomic versioned transition**. Every persisted mutation carries a `VersionedTransition{TransitionCount, NamespaceFailoverVersion}` that is monotonic per-cluster; this is the substrate that makes state-based replication deterministic (`workflow/mutable_state_impl.go:1185-1198` + `chasm/tree.go:1647-1650`).
- **Validator-then-state-machine layering**. The persistence layer (`workflow_state_status_validator.go`, `operation_mode_validator.go`) rejects illegal transitions before they reach the DB; the in-memory state machine layers (HSM/CHASM) reject illegal transitions before they reach persistence. This double check means a bug in either layer cannot corrupt storage silently.
- **Optimistic concurrency at the row level + shard lease**. `DBRecordVersion` (row) catches same-shard races; `RangeID` (shard) catches host-level failover races. Together they cover concurrent writers within and across cluster ownership boundaries (`service/history/shard/context_impl.go:1388-1437`).
- **Operation log over per-event logs**. HSM coalesces many state changes into a single ordered log that is compacted against deletions (`hsm/tree.go:709-805`); this lets replication ship one coalesced stream rather than replaying individual events.

## Notable Patterns

- **Source-state guard pattern** (`hsm/sm.go:52-66`, `chasm/statemachine.go:45-72`): every transition declares an allowed `Sources` list; `Possible()` checks membership; `Apply()` refuses to dispatch. The pattern is type-parameterised on the state type so the compiler enforces state types match.
- **Rollback-on-error with cache invalidation** (`hsm/tree.go:600-607`): `MachineTransition` defers a rollback that decrements `TransitionCount`, restores `LastUpdateVersionedTransition`, and forces cache reload — guaranteeing that a failed transition cannot be observed by readers.
- **Operation log compaction** (`hsm/tree.go:709-805`): given an ordered set of `Transition`/`Delete` operations and a deletion set, the algorithm deterministically drops operations under deleted ancestors and the deleted node's other ops. Tested via `TestApplyMutation` and `TestApplyMutation_DeleteUpdateSamePath` in `chasm/tree_test.go`.
- **Two-tier value-state machine** (`chasm/tree.go:68-76`): `valueStateUndefined / NeedDeserialize / Synced / NeedSerialize / NeedSyncStructure` ordered, with `setValueState` enforcing that any `≥ valueStateNeedSerialize` also flips `isActiveStateDirty`. This separates "needs IO" from "needs write" cleanly.
- **Closed/Paused intent check** (`chasm/tree.go:486-563`): the framework walks ancestors only once for both closed and paused checks (via `checkPaused`), avoiding double tree walks.
- **Per-component pending metadata write** (`chasm/tree.go:1439-1490`): `pendingRequestLinks` and `pendingUserMetadata` are staged in-memory and flushed in `closeTransactionApplyPendingComponentMetadata` (`:1498-1526`), which also logs orphan entries for caller misuse.

## Tradeoffs

- **Framework duplication (HSM vs CHASM)**: both implement the same conceptual `Transition` but with different context APIs. Cost is maintenance overhead; benefit is that older workflow constructs (timer/transfer queues, callbacks) keep their existing semantics while new constructs (scheduled activities, nexus operations) get a cleaner API. This is a conscious "no breaking change" choice that future maintainers may want to consolidate.
- **`valueState` enum embedded in CHASM node**: makes it possible to mix "dirty", "needs structure sync", and "needs serialize" semantics in one integer, which is fast but couples ordering semantics to integer comparison (e.g. `s >= valueStateNeedSerialize`). Adding new states requires care to maintain the ordering invariant (`chasm/tree.go:53-66`).
- **In-memory only flags (`terminated`, `deleteAfterClose`)**: documented as transaction-scoped (`chasm/tree.go:113-128`), which works for force-terminate but is a soft contract — relying on developers reading comments.
- **Otel span events gated by `telemetry.DebugMode()`** (`chasm/statemachine.go:55-56`): production avoids the cost; observability engineers must enable debug mode to see transitions, which is reasonable for high-volume paths.
- **`ValidateUpdateWorkflowStateStatus` does not constrain `CORRUPTED`**: `service/history/workflow/mutable_state_state_status.go:118-120` returns "unknown workflow state" for states outside the matrix, which is a fail-closed default but means `CORRUPTED` transitions are not modelled.
- **`MutableStateImpl.ApplyMutation` skips `setStateStatus` re-validation** (`workflow/mutable_state_impl.go:9130`): a deliberate trust-the-source decision; the price is that a corrupt primary could push an invalid state into a standby cluster.

## Failure Modes / Edge Cases

- **TransitionCount drift**: `NextTransitionCount` (`workflow/mutable_state_impl.go:1185-1198`) returns 0 when `transitionHistoryEnabled` is false and 1 when `currentVersionedTransition == nil`. If a cluster toggles the flag mid-flight, transitions may skip counts; this is caught by HSM `compareState` regression.
- **Deleted node transitions**: `MachineTransition` returns `ErrStateMachineInvalidState` if `cache.deleted` is true (`service/history/hsm/tree.go:580-582`); CHASM `valueState` machine rejects write attempts on deleted paths.
- **Stale task references**: `ValidateState` / `ValidateNotTransitioned` (`service/history/hsm/tasks.go:57-81`) return `ErrStaleReference` when `MachineTransitionCount` differs, allowing task executors to skip stale work.
- **Out-of-order mutation application**: tested by `TestApplyMutation_OutOfOrder` (`chasm/tree_test.go:1296-1353`) — children may be applied before parents; the framework handles this by lazy deserialisation.
- **Force-terminate mid-transaction**: `forceTerminateWorkflow` (`workflow/context.go:1159-1199`) discards pending changes via `Clear()`, reloads, and routes to `Terminate` — preventing the original transaction from leaking.
- **Soft-assert on invariant violation**: `softassert.Fail` and `softassert.UnexpectedInternalErr` log an error and (in the latter case) return a `serviceerror.Internal` rather than panicking, so transient corruption is logged but production continues — at the cost of consistency guarantees once an invariant has been broken.
- **RangeID ownership loss**: `handleWriteErrorLocked` (`service/history/shard/context_impl.go:1421-1426`) transitions the context to `stopReasonOwnershipLost` on `ShardOwnershipLostError`, shutting down the engine rather than risking further writes.

## Future Considerations

- **Consolidate HSM and CHASM transition types**: both expose `Transition[S comparable, SM StateMachine[S], E any]` (`hsm/sm.go:32`, `chasm/statemachine.go:24`); if the difference narrows to "does the apply take a `MutableContext`", a single generic abstraction with a context flag is feasible.
- **Re-validate replication-applied state**: `MutableStateImpl.ApplyMutation` (`workflow/mutable_state_impl.go:9130`) overwrites `executionState` without `setStateStatus`. A soft guard ("if state transition would be invalid, panic") would catch replication bugs faster at the cost of one extra comparison per apply.
- **Lock-free CHASM access checks**: the current `validateAccess` walk is recursive (`chasm/tree.go:522-563`); for very deep component trees a memoised "ancestor paused" cache could reduce overhead.
- **Surface operation-log tail as metrics**: the `OperationLog` (`hsm/tree.go:131`) is currently only used for replication and would be a useful operational telemetry source — e.g. `hsm_transitions_per_minute_by_type`.
- **Type-level impossible states**: codegen could derive a sum type from `Sources` lists so that `Transition.Apply` cannot be called from a destination state — purely a Go-generics ergonomics question.
- **`MutableStateChecksumInvalidated` is a strong signal** (`workflow/mutable_state_impl.go:7461`): wiring this into an alert would surface programming errors that the dirty-transaction guard already catches, converting silent failures into operator visibility.

## Questions / Gaps

- **HSM `EventDefinition.Apply` vs `CherryPick`**: `events.go:18-26` documents the contract but a search of the HSM folder did not surface concrete `EventDefinition` implementations in this directory; they likely live in `service/history/workflow/` or `service/history/ndc/`. No clear evidence found for the full surface of implementations within the selected source.
- **`ConstraintViolation` semantics on SQL plugins**: the persistence interface defines `ConditionFailedError` and `WorkflowConditionFailedError` (`service/history/shard/context_impl.go:1409-1411`) but the precise mapping from `DBRecordVersion` mismatch to these errors in the Cassandra plugin was not inspected — only MySQL row-lock evidence at `common/persistence/sql/sqlplugin/mysql/execution.go:28-32`.
- **`CompareState` deprecation path**: `Node.CompareState` (`hsm/tree.go:521-531`) has a TODO to remove once transition history is fully implemented. The current "fallback" approach is non-trivial — search would benefit from confirming the gating config (`ms.transitionHistoryEnabled` at `workflow/mutable_state_impl.go:1187`).
- **`chasm/tree.go:1874` valueState sanity assertion**: this is a runtime sanity check rather than a type-system guarantee; the gap between "valueState > valueStateNeedSerialize" being possible at all and being caught here suggests the value-state machine could be made stricter at compile time via a sealed enum.

---

Generated by `02.04-mutation-discipline-and-state-transitions` against `temporal`.