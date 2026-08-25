# Source Analysis: temporal

## 07.06 Side-Effect Ledger and Transaction Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (server) + protobuf/gRPC; Cassandra/SQL persistence; Elasticsearch/OpenSearch visibility; CHASM component state machines; Nexus-RPC outbound calls; S3/GCS filestore archivers |
| Analyzed | 2026-08-24 |

## Summary

Temporal is the reference implementation of a **side-effect ledger as a first-class architectural primitive**. The workflow event history is an append-only, transactionally-chained ledger: every state change — including scheduled side effects (activities, child workflows, timers, signals, Nexus operations) and SDK-recorded markers (`RecordMarker` for `Workflow.sideEffect`/`upsertSearchAttributes`) — is written as a typed `HistoryEvent` before or atomically-with the corresponding state mutation. The question "what did the agent change?" is answered by construction: replaying history reproduces every recorded change, and live summaries are exposed through `GetWorkflowExecutionHistory` and `DescribeWorkflowExecution`.

Key mechanisms observed:

- **Ledger write path**: mutable state accumulates in-memory changes and emits them at transaction close as a `WorkflowMutation` (state diff + new events + tasks + condition) via `CloseTransactionAsMutation` (`service/history/workflow/mutable_state_impl.go:7592-7636`). Events carry monotonic IDs and are chained by `TxnID`/`PrevTransactionID` pairs (`service/history/workflow/transaction_impl.go:279-348`).
- **Explicit transaction boundary**: each shard serializes writes under a write lock, stamps requests with its `RangeID`, and hands them to a conditional persistence write (`service/history/shard/context_impl.go:597-657`). The SQL store appends history first, then commits mutable state inside a DB transaction guarded by a row lock plus optimistic condition on `next_event_id` / `db_record_version` (`common/persistence/sql/execution.go:334-358`, `common/persistence/sql/execution_util.go:629-695`).
- **Uncertainty-aware error model**: `OperationPossiblySucceeded` classifies failures so callers know whether a write definitely did not commit (`common/persistence/error_type.go:5-22`); unknown errors trigger shard re-acquisition with a fresh `RangeID` so subsequent reads can reliably determine the outcome (`service/history/shard/context_impl.go:1538-1547`).
- **Compensation where possible**: orphaned history appended before a failed conditional update is trimmed best-effort (`common/persistence/execution_manager.go:263-274`, `common/persistence/execution_manager.go:1083`); in-memory stats roll back via `historySizeRollback.revertOnError` (`common/persistence/execution_manager.go:77-97`); post-commit/rollback hooks exist for caller notification via the `effect` package (`common/effect/controller.go:5-8`).
- **External-call idempotency**: durable request-ID ledgers (`RequestIds` map with event-ID back-references, `service/history/workflow/mutable_state_impl.go:2591-2653`), attempt-counted side-effect task handlers with pre-execution validation (`chasm/lib/nexusoperation/operation_tasks.go:61-69`), request IDs forwarded to external Nexus handlers (`chasm/lib/nexusoperation/operation_tasks.go:153-161`), and duplicate-start replays for activities (`service/history/api/recordactivitytaskstarted/api.go:150-166`).
- **Audit trail beyond retention**: visibility projections (started/upsert/close records) are emitted asynchronously from the ledger (`service/history/visibility_queue_task_executor.go:180-234`), and closed histories can be archived to secondary storage with read-through fallback (`service/history/archival/archiver.go:107-239`, `service/frontend/workflow_handler.go:995-1000`).

The one thing Temporal deliberately does *not* do is undo arbitrary external changes made by user code; instead it makes every outbound effect retryable-and-idempotent so that compensation becomes a workflow-level concern (retry policies, sagas) rather than a server-side rollback.

## Rating

**9 / 10 — Mature, durable, observable, extensible, and proven under failure or scale; the residual point is that audit projections (visibility, archival) are eventually consistent relative to the execution ledger, and cleanup of orphaned history after failed conditional writes is best-effort rather than atomic.**

Rationale:

- The ledger itself is append-only, totally ordered per execution, chained across transactions (`TxnID`/`PrevTransactionID`), and is the single source of truth from which all other state is derived — this is the strongest possible form of "side-effect ledger".
- Transaction boundaries are explicit at three layers: shard lock + `RangeID` fencing (`service/history/shard/context_impl.go:613-651`), conditional DB updates with row locks (`common/persistence/sql/execution_util.go:629-665`), and in-memory provisional states resolved by commit/rollback effects (`service/history/workflow/update/update.go:274-312`).
- Failure semantics are codified, not ad-hoc: error types partition into "definitely not committed" vs "possibly committed" (`common/persistence/error_type.go:5-22`), and both branches have defined behavior.
- Not rated 10 because: (a) history append happens outside the mutable-state DB transaction (`common/persistence/sql/execution.go:338-350`), leaving a window where orphaned nodes exist until `trimHistoryNode` runs; (b) the `effect` package explicitly disclaims transactional guarantees (`docs/architecture/effect-package.md:24-26`); (c) visibility/archival projections lag the commit and are not atomically consistent with it.

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to `sources/temporal/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Ledger entry generation | `MutableStateImpl.CloseTransactionAsMutation` packages execution info/state, next event ID, upsert/delete sets for activities/timers/children/cancels/signals, CHASM node diffs, buffered events, generated tasks, and the update condition into one `WorkflowMutation`. | `service/history/workflow/mutable_state_impl.go:7592-7636` |
| Snapshot variant of ledger write | `CloseTransactionAsSnapshot` produces full-state snapshots (used for create/conflict-resolution/continue-as-new); refuses to snapshot with un-flushed buffered events. | `service/history/workflow/mutable_state_impl.go:7638-7681` |
| Event batching with txn chaining | `PersistWorkflowEvents` routes first-batch vs continuation batches and stamps each with `PrevTxnID`/`TxnID` for ordered chaining. | `service/history/workflow/transaction_impl.go:249-348` |
| Which changes become events | `bufferEvent` enumerates command-derived events (activity scheduled, timer started, marker recorded, child/signal initiated, nexus scheduled, update accepted/completed) as non-buffered; everything else buffers until the enclosing transaction closes. | `service/history/historybuilder/event_store.go:297-360` |
| SDK side-effect markers persisted | `CreateMarkerRecordedEvent` records marker name, details, header, failure, and the completing workflow-task ID — the durable record for SDK `sideEffect`/`mutableValue` calls. | `service/history/historybuilder/event_factory.go:805-822` |
| Transaction wrapper (history service) | `TransactionImpl.UpdateWorkflowExecution` submits the mutation+events to the shard context and, when `OperationPossiblySucceeded(err)`, still notifies engines of tasks/mutations so downstream consumers converge. | `service/history/workflow/transaction_impl.go:167-221` |
| Shard-level transaction boundary | `ContextImpl.UpdateWorkflowExecution`: acquires IO semaphore, takes shard write lock, checks shard/namespace state, tracks task keys, stamps `request.RangeID`, unlocks, then persists; write errors routed to `handleWriteError`. | `service/history/shard/context_impl.go:597-657` |
| Task-key tracking at the boundary | `taskKeyManager.setAndTrackTaskKeys` registers keys of all tasks created by the transaction so queue readers compute safe high-watermarks. | `service/history/shard/task_key_manager.go:45-79`, `service/history/shard/context_impl.go:633-646` |
| Optimistic concurrency condition | Mutation carries `Condition = ms.nextEventIDInDB` and `DBRecordVersion`; SQL `lockAndCheckExecution` row-locks the execution row and fails with `WorkflowConditionFailedError` if `next_event_id` or record version drifted. | `service/history/workflow/mutable_state_impl.go:7626-7627`, `common/persistence/sql/execution_util.go:629-665` |
| Condition plumbing end-to-end | `SetUpdateCondition`/`GetUpdateCondition` capture the DB state at load time for reuse at close time. | `service/history/workflow/mutable_state_impl.go:7437-7447` |
| History-append ordering vs mutable state | SQL store appends all new history events first (idempotent, outside the tx), then commits mutable state in `txExecuteShardLocked`; same pattern for conflict resolution. | `common/persistence/sql/execution.go:334-358`, `common/persistence/sql/execution.go:446-474` |
| Compensation: trim orphaned history | On condition-failure the manager trims just-appended history nodes so a retried transaction does not leave duplicates. | `common/persistence/execution_manager.go:263-274`, `common/persistence/execution_manager.go:1083` |
| Compensation: in-memory stat rollback | `historySizeRollback.revertOnError` reverts speculative history-size accounting if persistence fails. | `common/persistence/execution_manager.go:77-97`, `common/persistence/execution_manager.go:173-204` |
| Commit/rollback hook abstraction | `effect.Controller` exposes `OnAfterCommit`/`OnAfterRollback`; docs state callbacks fire after the store write succeeds or is discarded, but explicitly provide no transactional guarantees. | `common/effect/controller.go:5-8`, `docs/architecture/effect-package.md:14-26` |
| Effects applied only after durable persist | In `RespondWorkflowTaskCompleted` the buffer is created up front, cancelled on any persistence error (`effects.Cancel(ctx)`), and applied only after `UpdateWorkflowExecution*` succeeds (`effects.Apply(ctx)`). | `service/history/api/respondworkflowtaskcompleted/api.go:245-259`, `service/history/api/respondworkflowtaskcompleted/api.go:673-715` |
| Provisional states resolved by effects | Update admission/abort use `stateProvisionally*` states whose transitions are finalized by `OnAfterCommit`/`OnAfterRollback` closures, so in-memory registry state never diverges from durability. | `service/history/workflow/update/update.go:258-368` |
| Mutable state cleared on failed update | `UpdateWorkflowExecutionWithNew` defers `c.Clear()` on any error, discarding all uncommitted in-memory mutations. | `service/history/workflow/context.go:783-797` |
| Uncertainty classification | `OperationPossiblySucceeded(err)` returns false only for error types that guarantee non-commit (condition failures, ownership lost, size limits, timeouts-before-start); default is "possibly succeeded" → notify consumers anyway. | `common/persistence/error_type.go:5-22`, `service/history/workflow/transaction_impl.go:201-204` |
| Unknown-write-error fencing | `handleWriteErrorLocked`: on unrecognized errors the shard re-acquires itself with a new `RangeID` so later reads either see the write or know it failed; `ShardOwnershipLostError` stops the engine. | `service/history/shard/context_impl.go:1499-1548` |
| Durable request-ID ledger | `AttachRequestID` maps client request IDs to `(EventType, EventId)`; `CreateRequestId` stored for start; `AttachChasmRequestID` persists framework-level IDs with attach-time and forces state persistence for durability. | `service/history/workflow/mutable_state_impl.go:2591-2653` |
| Request-ID ledger bounding | `closeTransactionSweepChasmRequestIDs` evicts entries older than `RequestIDMaxAge` then enforces `MaximumRequestIDsPerExecution`, keeping event-backed IDs forever. | `service/history/workflow/mutable_state_impl.go:2655-2700` |
| Retry rejection via request ID | `ChasmEngine.updateComponent` rejects already-seen request IDs with `ErrRequestIDAlreadyUsed` before running the update function. | `service/history/chasm_engine.go:554-577` |
| Side-effect task validation before execute | Nexus invocation handler validates `op.Status == SCHEDULED && op.Attempt == task.Attempt`; callbacks validate status+attempt similarly — stale/duplicate deliveries are dropped. | `chasm/lib/nexusoperation/operation_tasks.go:61-69`, `chasm/lib/callback/tasks.go:107-109` |
| External call + result recording | Invocation handler performs the outbound `Start` call with `RequestID` and callback token, then records the outcome into component state via `saveInvocationResult` (started/completed/failed/retry/backoff). | `chasm/lib/nexusoperation/operation_tasks.go:147-213`, `chasm/lib/nexusoperation/operation.go:344-378` |
| Attempt-level compensation state machine | `TransitionScheduled` increments `Attempt` and emits invocation+timeout tasks; `transitionAttemptFailed` stores `LastAttemptFailure` and schedules a backoff retry task — retries are recorded, not hidden. | `chasm/lib/nexusoperation/operation_statemachine.go:21-75` |
| Terminate idempotency by request ID | `Operation.Terminate` rejects conflicting terminate requests referencing a different request ID once terminated. | `chasm/lib/nexusoperation/operation.go:408-423` |
| Queryable external IDs | Operation search attributes include endpoint/service/operation/request-ID/status; request IDs also surface as link references (`RequestIdReference`) on completions. | `chasm/lib/nexusoperation/operation.go:425-433`, `service/history/workflow/mutable_state_impl.go:794-812` |
| Activity start dedup / confirmation record | If activity already started by same request ID, the recorded `StartedTime`/`Attempt`/clock is replayed as a positive response instead of double-starting. | `service/history/api/recordactivitytaskstarted/api.go:150-166` |
| Cross-cluster signal reapply dedup | Reapplied signal events are filtered by `NewEventReappliedID(runID, eventID, version)` against the `appliedEvents` dedup set before re-application. | `service/history/api/reapplyevents/api.go:54-81`, `service/history/workflow/mutable_state_impl.go:7683-7696` |
| Audit projection: visibility tasks | Visibility executor writes `RecordWorkflowExecutionStarted` / `UpsertWorkflowExecution` / close records derived from committed mutable state. | `service/history/visibility_queue_task_executor.go:150-280` |
| Audit projection: archival | Archiver fans out per-target archival of history branch data and visibility records to configured URIs, rate-limited, with per-target metrics. | `service/history/archival/archiver.go:107-239` |
| User-facing change summary (history) | `GetWorkflowExecutionHistory` paginates raw ledger events and transparently reads archived history when retained data is gone. | `service/frontend/workflow_handler.go:960-1000` |
| User-facing change summary (describe) | `DescribeWorkflowExecution` surfaces pending activities, children, workflow task, callbacks, pending Nexus operations, and extended info — an in-flight side-effect summary. | `service/frontend/workflow_handler.go:3320-3372` |
| Standby-cluster discard handling | Side-effect tasks pending too long on standby go through explicit `Discard` handlers (default returns `ErrTaskDiscarded`); zombie/stale-reference validation precedes execution. | `service/history/chasm_task_util.go:23-147`, `chasm/task_handler_base.go:5-13` |

## Answers to Dimension Questions

1. **What external changes did the agent make?**
   Everything the platform itself changes externally is enumerated as typed events in the run's history: activity scheduling/start/completion/heartbeat, child-workflow initiation, timer firing, signal delivery, update accept/complete, Nexus operation schedule/start/complete/fail, marker recordings, and search-attribute/memo upserts (event taxonomy enforced in `service/history/historybuilder/event_store.go:300-360`). Outbound HTTP/Nexus calls made by the server (callbacks, Nexus operations) are additionally tracked per-component with attempts and outcomes (`chasm/lib/nexusoperation/operation_statemachine.go:50-75`). What the platform cannot see are arbitrary external changes performed inside user workflow/activity code — those are visible only if the code records them via markers or activity results.

2. **Are side effects auditable?**
   Yes, at three levels. Primary: the immutable event ledger, retrievable via `GetWorkflowExecutionHistory` including archived runs (`service/frontend/workflow_handler.go:960-1000`). Secondary: visibility projections (start/upsert/close records written asynchronously from committed state, `service/history/visibility_queue_task_executor.go:150-280`) enabling list/query APIs. Tertiary: operational audit via transition history embedded in `WorkflowExecutionInfo` (`NotifyNewHistoryMutationEvent` propagates `TransitionHistory`, `service/history/workflow/transaction_impl.go:696-733`) and CHASM component-level state machines with recorded attempts and outcomes.

3. **Can failed side effects be compensated?**
   Server-side, yes for platform-owned effects: orphaned history from failed conditional updates is trimmed (`common/persistence/execution_manager.go:263-274`), uncommitted mutable state is discarded wholesale on error (`service/history/workflow/context.go:793-797`), and post-commit notifications are cancelled on failure (`service/history/api/respondworkflowtaskcompleted/api.go:673-674`). Failed outbound calls are compensated by retry with recorded backoff rather than undo (`chasm/lib/nexusoperation/operation_statemachine.go:50-75`, `service/history/workflow/retry.go:31-53`); true business-level compensation (saga) is delegated to workflow authors, which is a stated design boundary rather than a gap.

4. **Are external IDs stored?**
   Yes, systematically. Client request IDs map to event IDs in the durable `RequestIds` ledger (`service/history/workflow/mutable_state_impl.go:2600-2603`); start request IDs persist as `CreateRequestId`; Nexus operations persist their request ID and expose it as a searchable attribute (`chasm/lib/nexusoperation/operation.go:425-433`); activity infos persist the start request ID (`service/history/api/recordactivitytaskstarted/api.go:152`); cross-cluster reapplied events are keyed by `(runID, eventID, version)` dedup IDs (`service/history/api/reapplyevents/api.go:68`).

5. **Are users shown what changed?**
   Yes: full ledger access via `GetWorkflowExecutionHistory` (`service/frontend/workflow_handler.go:960`), in-flight summaries of pending activities/children/callbacks/Nexus operations via `DescribeWorkflowExecution` (`service/frontend/workflow_handler.go:3362-3371`), per-operation outcome inspection via `DescribeNexusOperationExecution` including input/outcome (`chasm/lib/nexusoperation/operation.go:435-464`), and completion links carrying `RequestIdReference`s back to originating events (`service/history/workflow/mutable_state_impl.go:794-812`).

## Architectural Decisions

1. **Event sourcing as the ledger.** Mutable state is derivable entirely from history; every transaction writes both a state diff (mutation/snapshot) and the corresponding event batch atomically-enough that conflict detection uses the ledger's own watermark (`next_event_id`) as the compare-and-swap condition (`service/history/workflow/mutable_state_impl.go:7626`, `common/persistence/sql/execution_util.go:645-662`).

2. **Shard-range fencing over distributed transactions.** Instead of XA-style cross-store transactions, each write is fenced by the shard's monotonically increasing `RangeID`; ownership loss invalidates outstanding writes (`service/history/shard/context_impl.go:584-594`), and ambiguous failures force range rotation so outcomes become checkable by read (`service/history/shard/context_impl.go:1538-1547`).

3. **Append-first, condition-second write order.** History events (idempotent, chained by TxnID) are appended before the conditional mutable-state commit; failures compensate by trimming appended nodes (`common/persistence/sql/execution.go:338-350`, `common/persistence/execution_manager.go:263-274`). This trades a transient orphaned-data window for higher concurrency.

4. **Provisional states + commit/rollback effects for API-visible transactions.** Workflow Updates adopt two-phase in-memory states (`stateProvisionallyAdmitted/Accepted/Completed/Aborted`) finalized strictly by post-persist effects, giving callers durability-gated responses without blocking the ledger design (`service/history/workflow/update/update.go:258-368`, `service/history/api/respondworkflowtaskcompleted/api.go:713-715`).

5. **Idempotency-by-ledger-reference.** Rather than a global idempotency store, each subsystem records the minimal identifying tuple (request ID → event ID/type, attempt counters, dedup resource keys) inside the same transactional state it protects, with lifecycle-bounded sweeping (`service/history/workflow/mutable_state_impl.go:2655-2700`).

## Notable Patterns

- **Validate-then-execute-then-record for outbound tasks**: side-effect handlers validate component status/attempt (`chasm/lib/nexusoperation/operation_tasks.go:61-69`), perform the external call under timeout (`chasm/lib/nexusoperation/operation_tasks.go:147-190`), then persist the outcome through a component transaction (`chasm/lib/nexusoperation/operation_tasks.go:203-206`). This is the canonical agent-tool pattern implemented as reusable infrastructure (`chasm/task_handler_base.go:5-13`).
- **Destination-routed outbound queues**: side-effect tasks carry an optional `Destination` routing key (e.g., endpoint name) enabling grouped retries and circuit breaking per destination (`chasm/task.go:22-25`, `service/history/outbound_queue_active_task_executor.go:126`).
- **Dual-mode ledger close**: the same accumulated in-memory diff can be emitted as a compact `Mutation` (steady state) or a full `Snapshot` (create/reset/continue-as-new), selected by policy at close time (`service/history/workflow/mutable_state_impl.go:7592-7681`, `service/history/workflow/context.go:569-613`).
- **Post-commit fanout**: successful transactions notify engines about new tasks, CHASM executions, history growth, and fast-forward updates — decoupling durable writes from downstream processing while preserving exactly-the-committed-facts (`service/history/workflow/transaction_impl.go:589-654`).
- **Best-effort async projections**: visibility and archival are derived views fed by tasks from the primary ledger, never blocking the commit path (`service/history/visibility_queue_task_executor.go:188-233`).

## Tradeoffs

- **Durability window vs throughput**: appending history before the guarded mutable-state commit means a crash between the two leaves orphaned history rows until trim; accepted consciously to avoid holding shard locks during history I/O (`common/persistence/sql/execution.go:338-358`).
- **Strong consistency only within the execution store**: visibility/Elasticsearch and archives are eventually consistent with the ledger; a just-completed workflow may briefly be invisible or stale in queries (`service/history/visibility_queue_task_executor.go:236-280` processes close tasks asynchronously).
- **Effects are hooks, not transactions**: `effect.Buffer` callbacks may partially apply after a successful persistence write — acceptable for update-response unblocking but explicitly documented as non-transactional (`docs/architecture/effect-package.md:23-26`).
- **Request-ID ledger boundedness**: CHASM request IDs are swept by age/count, so extremely old duplicate retries could theoretically re-execute after their IDs were evicted (`service/history/workflow/mutable_state_impl.go:2655-2663`); event-backed and create IDs are exempt.
- **No generic external-side-effect undo**: the platform guarantees *at-least-once, deduplicated, recorded* effects, but compensating application-level actions remains the developer's job (retry policies and workflow logic, `service/history/workflow/retry.go:31-97`).

## Failure Modes / Edge Cases

- **Ambiguous persistence failure** (timeout, network): classified as possibly-succeeded; consumers are notified speculatively (`service/history/workflow/transaction_impl.go:201-204`) and the shard rotates its RangeID so subsequent reads resolve the ambiguity deterministically (`service/history/shard/context_impl.go:1538-1547`). Tested extensively via condition-failure suites (`common/persistence/tests/execution_mutable_state.go:174-307`, `common/persistence/cassandra/errors_test.go:245-353`).
- **Concurrent writers on the same execution**: loser receives `WorkflowConditionFailedError`; RespondWorkflowTaskCompleted counts these as concurrency-update failures and reloads state (`service/history/api/respondworkflowtaskcompleted/api.go:673-679`).
- **Zombie workflows receiving side-effect tasks**: explicit zombie check drops tasks targeting dead-but-not-yet-deleted executions (`service/history/chasm_task_util.go:28-33`).
- **Stale task references after shard failover**: task IDs below the shard's task-generation clock are rejected as stale references (`service/history/chasm_task_util.go:68-74`).
- **Transaction size overflow mid-completion**: on `TransactionSizeLimitError` the handler reloads state and force-terminates the workflow, aborting accepted updates with defined reasons — a bounded, observable degradation (`service/history/api/respondworkflowtaskcompleted/api.go:681-708`).
- **Duplicate activity starts**: second `RecordActivityTaskStarted` with the same request ID replays the original confirmation; different request ID on an already-started activity is rejected (`service/history/api/recordactivitytaskstarted/api.go:150-169`).

## Future Considerations

- **CHASM generalization**: newer subsystems (Nexus operations, callbacks, scheduler) migrate onto the CHASM side-effect task framework, suggesting eventual consolidation of ad-hoc transfer/timer task handling behind validated `SideEffectTaskHandler`s (`chasm/task_handler_base.go:5-13`, `chasm/lib/nexusoperation/operation_tasks.go:47-52`).
- **Speculative transitions**: `WithSpeculative` supports unpersisted exploratory transitions, but tasks generated during them are explicitly unsupported yet (`chasm/engine.go:172-177`) — a known seam between the ledger and ephemeral computation.
- **Pagination buffering**: task-completion pagination buffers intermediate pages in memory before merging into the final transaction (`service/history/api/respondworkflowtaskcompleted/api.go:276-291`), an area flagged for migration toward the effect package.

## Questions / Gaps

- **No evidence found** for a unified, cross-store transactional guarantee spanning execution store, visibility store, and archives — searches across `common/persistence` (manager wrappers, retryable/metric/rate-limited clients) show independent write paths composed by tasks, not distributed transactions. Consistency relies on asynchronous convergence.
- **No evidence found** of server-side saga/compensation primitives for user-defined external actions; compensation exists only for platform-owned artifacts (history trimming, state clearing, retry/backoff). Documentation of this boundary lives in SDK-facing retry semantics (`service/history/workflow/retry.go:43-53`) rather than a dedicated server module.
- The exact retention policy interplay between `RequestIDMaxAge` eviction and very-long-horizon duplicate retries (e.g., days-later replay of an identical Nexus request ID) is configurable (`service/history/workflow/mutable_state_impl.go:2669-2674`) but no test was found covering the eviction-then-duplicate-retry sequence specifically; behavior in that corner is inferred from the sweep logic, not demonstrated.

---

Generated by `dimensions/07.06-side-effect-ledger-and-transaction-boundaries.md` against `temporal`.
