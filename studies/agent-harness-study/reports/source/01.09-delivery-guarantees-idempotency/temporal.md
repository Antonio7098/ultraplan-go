# Source Analysis: temporal

## Delivery Guarantees and Idempotency

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (server) + protobuf/gRPC, Cassandra/SQL/MySQL/PostgreSQL/SQLite persistence, Elasticsearch visibility, Nexus-RPC, CHASM state machines, HSM |
| Analyzed | 2026-07-03 |

## Summary

Temporal is a **durable execution platform** whose delivery semantics are a
multi-layered composition. The system deliberately avoids a single "exactly-once"
mode and instead composes primitives that together approximate **effectively-once
durability**:

- **Workflow executions** are **event-sourced and append-only**; they are
  persisted via conditional `current_executions` updates keyed by
  `create_request_id` (`schema/postgresql/v12/temporal/schema.sql:46-62`,
  `common/persistence/data_interfaces.go:113-125`,
  `service/history/api/startworkflow/api.go:319-346`). This gives
  at-most-once semantics for workflow creation and idempotency-by-request-id
  for retries.
- **Internal history and outbound queues are range-id-partitioned** with
  monotonically allocated task IDs (`service/history/shard/task_key_generator.go:151-184`)
  and a `syncTaskId = -137` sentinel for synchronous matches
  (`service/matching/physical_task_queue_manager.go:44`). Queue persistence is
  conditional (`IF NOT EXISTS` on Cassandra `INSERT`
  `common/persistence/cassandra/queue_v2_store.go:32`,32),
  and tasks are advanced via ack-level barriers
  (`service/matching/ack_manager.go:97-128`,
  `service/matching/backlog_manager.go:231-272`).
- **Poller-to-history delivery is documented "at most once"** for poll
  responses (`api/historyservice/v1/request_response.pb.go:1171`,
  `api/historyservice/v1/request_response.pb.go:1744`). The History engine
  collapses duplicate `RecordWorkflowTaskStarted` /
  `RecordActivityTaskStarted` calls using `StartedEventId != EmptyEventID`
  plus a `RequestID` equality check
  (`service/history/api/recordworkflowtaskstarted/api.go:98-112`,
  `service/history/api/recordactivitytaskstarted/api.go:150-171`).
- **Workflow signals** dedup by request ID at the mutable-state layer
  (`service/history/api/signalworkflow/api.go:45-89`,
  `service/history/workflow/mutable_state_impl.go:2499-2518`,
  `service/history/workflow/mutable_state_impl.go:2538-2552`).
- **Workflow Updates** dedup by Update ID inside an in-memory registry that is
  rebuilt from `UpdateInfo` events on registry loss
  (`service/history/workflow/update/registry.go:226-235`,
  `service/history/workflow/update/update.go:315-371`,
  `docs/architecture/workflow-update.md:75-83`).
- **Nexus / Chasm callbacks** track an `Attempt` counter and persist
  `RequestId` per callback (`components/callbacks/statemachine.go:75`,
  `components/callbacks/executors.go:159-180`,
  `chasm/lib/activity/activity.go:217-235`). The outbound queue processor
  (`service/history/outbound_queue_active_task_executor.go`,
  `service/history/outbound_queue_standby_task_executor.go`) provides
  destination-grouped retries with circuit breakers.
- **Cross-cluster replication** is task-id ordered; tasks carry an explicit
  `sourceTaskID` and replication state-machines use `VectorClock`s and
  `TransitionCount`s to detect and reject duplicate replication
  (`service/history/ndc/replication_task.go:118-236`,
  `service/history/replication/task_processor.go:520-527`).
- **No "exactly-once" claim is made for activity execution**: the
  README explicitly says activity code must "either be idempotent or
  non-retryable (i.e. at least once or at most once)"
  (`docs/architecture/README.md:35`).

Overall the design is **well-documented, layered, and well-tested** but
explicitly avoids a system-wide exactly-once claim; instead it tries to make
each subsystem idempotent under retry, so that end-to-end behavior is
effectively-once when clients cooperate by supplying stable IDs.

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational
safeguards, but no global exactly-once claim and several subsystems depend on
in-memory dedup tables that are lost on errors (notably Updates in the
`stateAdmitted` window).**

Rationale:

- Pros: request-ID dedup on virtually every public-facing API, conditional
  persistence writes (`IF NOT EXISTS` / range-id assertions), shard-level
  task-id monotonicity, vector-clock/transition-count conflict detection,
  documented at-most-once for poll responses, and explicit rejection of stale
  tasks via "stamp" checks
  (`service/history/api/recordactivitytaskstarted/api.go:178-184`).
- Cons: no global idempotency store (every API maintains its own
  dedup table), admitted-but-not-accepted Updates can be lost
  (`docs/architecture/workflow-update.md:71-83`), WorkerVersionStamp drift
  between matching and history can cause task rewrites that read like duplicate
  delivery (`service/matching/backlog_manager.go:231-260`), and the matching
  service re-enqueues failed `syncMatch` tasks at a higher task ID
  (`service/matching/backlog_manager.go:236-243`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Workflow-create idempotency by RequestID | `service/history/api/startworkflow/api.go:319-346` looks up `currentWorkflowRequestIDs[request.GetRequestId()]` and returns the existing run as `StartDeduped`. | `service/history/api/startworkflow/api.go:328-346` |
| Schema column `create_request_id` for dedup | `schema/postgresql/v12/temporal/schema.sql:52`, `schema/postgresql/v12/temporal/schema.sql:71` | `schema/postgresql/v12/temporal/schema.sql:46-62`, `schema/postgresql/v12/temporal/schema.sql:64-81` |
| `CurrentWorkflowConditionFailedError` carries `RequestIDs` | persistence API contract | `common/persistence/data_interfaces.go:113-125` |
| SQL implementation extracts `RequestIDs` on conflict | `common/persistence/sql/errors.go:31-33` | `common/persistence/sql/errors.go:15-35` |
| Cassandra implementation returns conflict with `RequestIDs` | `common/persistence/cassandra/errors.go:221-228` | `common/persistence/cassandra/errors.go:217-235` |
| Frontend applies default canonicalization before validation to keep idempotent retries | `service/frontend/workflow_handler.go:610`, `service/frontend/workflow_handler.go:685` (`dedupLinksFromCallbacks`) | `service/frontend/workflow_handler.go:608-700` |
| Frontend auto-generates RequestID for Worker Deployment create | `service/frontend/workflow_handler.go:4210-4214` | `service/frontend/workflow_handler.go:4210-4226` |
| Frontend WorkerDeployment dedup tested | `service/worker/workerdeployment/workflow_test.go:105-148` | `service/worker/workerdeployment/workflow_test.go:105-148` |
| Frontend Signal request ID dedup at history layer | `service/history/api/signalworkflow/api.go:45-89` calls `IsSignalRequested` and `AddSignalRequested`; idempotent return via `Noop:true`. | `service/history/api/signalworkflow/api.go:43-89` |
| Signal mutable-state dedup map | `service/history/workflow/mutable_state_impl.go:2499-2518`, `service/history/workflow/mutable_state_impl.go:2538-2552` (and CHASM signal backlink path) | `service/history/workflow/mutable_state_impl.go:2499-2552` |
| Signal-with-Start dedup by request ID | `service/history/api/signalwithstartworkflow/signal_with_start_workflow.go:296-304` | `service/history/api/signalwithstartworkflow/signal_with_start_workflow.go:279-342` |
| Update dedup by Update ID | `service/history/api/updateworkflow/api.go:161-164` (`FindOrCreate`) returns `alreadyExisted`; subsequent `Admit` is a no-op | `service/history/api/updateworkflow/api.go:104-225` |
| Update Registry keyed by ID | `service/history/workflow/update/registry.go:226-235`, in-memory `updates map[string]*Update` | `service/history/workflow/update/registry.go:85-236` |
| Update Admit is a no-op once state advances | `service/history/workflow/update/update.go:315-371` | `service/history/workflow/update/update.go:315-371` |
| Update AttachCallbacks dedup | `service/history/workflow/update/update.go:373-399` (returns success if already attached for same request ID) | `service/history/workflow/update/update.go:373-399` |
| Update Store persists outcomes for replay dedup | `service/history/workflow/update/registry.go:455-481` (`GetUpdateOutcome`) | `service/history/workflow/update/registry.go:455-481` |
| Update dedup narrative | `docs/architecture/workflow-update.md:75-83`, `docs/architecture/workflow-update.md:139-145` (rejected updates are not deduped; in-registry loss is acknowledged) | `docs/architecture/workflow-update.md:60-210` |
| WorkflowTaskStarted idempotent when RequestID matches | `service/history/api/recordworkflowtaskstarted/api.go:98-112` | `service/history/api/recordworkflowtaskstarted/api.go:94-180` |
| ActivityTaskStarted idempotent when RequestID matches | `service/history/api/recordactivitytaskstarted/api.go:150-171` returns success on repeat; falls back to `serviceerrors.NewTaskAlreadyStarted` only if IDs differ. | `service/history/api/recordactivitytaskstarted/api.go:130-304` |
| Stamp check rejects mismatched task | `service/history/api/recordworkflowtaskstarted/api.go:76-79`, `service/history/api/recordactivitytaskstarted/api.go:178-184` returns `NewObsoleteMatchingTask` | `service/history/api/recordactivitytaskstarted/api.go:178-184` |
| Chasm Activity HandleStarted idempotent | `chasm/lib/activity/activity.go:217-235` returns existing response if already started and request ID matches. | `chasm/lib/activity/activity.go:217-235` |
| Chasm attach-links idempotent | `chasm/lib/activity/activity.go:381-391` short-circuits if links already attached for request ID. | `chasm/lib/activity/activity.go:381-399` |
| Chasm activity idempotency test | `chasm/lib/activity/activity_test.go:51`, `chasm/lib/activity/activity_test.go:381` | `chasm/lib/activity/activity_test.go:51`, `chasm/lib/activity/activity_test.go:381` |
| Matching poll request ID is at-most-once for poll responses | documented: "Unique id of each poll request. Used to ensure at most once delivery of tasks." | `api/historyservice/v1/request_response.pb.go:1171`, `api/historyservice/v1/request_response.pb.go:1744` |
| Task IDs are allocated under range ID | `service/history/shard/task_key_generator.go:151-184`, `service/history/shard/context_impl.go:1147-1199` | `service/history/shard/task_key_generator.go:151-184`, `service/history/shard/context_impl.go:1147-1199` |
| RangeID condition update | `service/history/shard/context_impl.go:1155-1199` (renewRangeLocked), `common/persistence/cassandra/shard_store.go:16` (`VALUES(...) IF NOT EXISTS`) | `service/history/shard/context_impl.go:1147-1199`, `common/persistence/cassandra/shard_store.go:16` |
| Matching sync-match sentinel | `service/matching/physical_task_queue_manager.go:44` (`syncMatchTaskId = -137`) | `service/matching/physical_task_queue_manager.go:44-50` |
| Backlog ack-level prevents redelivery | `service/matching/ack_manager.go:97-128`, `service/matching/backlog_manager.go:231-272` (tasks below ackLevel are GC'd) | `service/matching/ack_manager.go:97-128`, `service/matching/backlog_manager.go:227-272` |
| Failed sync-match re-enqueues at higher task ID (at-least-once for that exact task) | `service/matching/backlog_manager.go:231-260` calls `taskWriter.appendTask(task.Data)` on failure. | `service/matching/backlog_manager.go:227-272` |
| Queue V2 enqueue uses `IF NOT EXISTS` (no-op for duplicate message ID) | `common/persistence/cassandra/queue_v2_store.go:32`, `common/persistence/cassandra/queue_v2_store.go:43-72` (returns `ErrEnqueueMessageConflict`) | `common/persistence/cassandra/queue_v2_store.go:30-72` |
| Queue V2 task reader uses monotonic pass/task_id ordering | `common/persistence/sql/sqlplugin/postgresql/execution.go:118-127` (`tasks_v2` table) | `common/persistence/sql/sqlplugin/postgresql/execution.go:115-128` |
| TransactionPolicy + txExecute wraps conditional writes | `common/persistence/sql/common.go:52-81`, `common/persistence/sql/execution.go:39-79` | `common/persistence/sql/common.go:52-81`, `common/persistence/sql/execution.go:39-79` |
| CreateWorkflowExecution is a conditional create by `(namespace, workflow_id, run_id)` and `LastWriteVersion` | `common/persistence/sql/execution.go:81-208`, `common/persistence/cassandra/mutable_state_store.go:383-444` | `common/persistence/sql/execution.go:81-208`, `common/persistence/cassandra/mutable_state_store.go:383-444` |
| Conditional update on current_executions uses `run_id`+`state`+`last_write_version` | `service/history/api/startworkflow/api.go:319-368`, `common/persistence/cassandra/mutable_state_store.go:399-444` | `service/history/api/startworkflow/api.go:319-368`, `common/persistence/cassandra/mutable_state_store.go:399-444` |
| Cross-cluster replication tasks carry sourceTaskID for ordering/dedup | `service/history/ndc/replication_task.go:118-236`, `service/history/ndc/replication_task.go:340-410` | `service/history/ndc/replication_task.go:118-410` |
| Replication max-received-task-id is tracked | `service/history/replication/task_processor.go:527` (`p.maxRxProcessedTimestamp = ...`) | `service/history/replication/task_processor.go:520-560` |
| `WorkflowExecutionAlreadyStarted` error carries `StartRequestId` | `service/frontend/workflow_handler.go:3500-3551`, `service/history/api/workflow_id_dedup.go:233-249` | `service/frontend/workflow_handler.go:3490-3551`, `service/history/api/workflow_id_dedup.go:233-249` |
| Activity ID + RunID uniqueness enforced by error type | `service/frontend/workflow_handler.go:536`, `api/historyservice/v1/service_grpc.pb.go:108` | `service/frontend/workflow_handler.go:536`, `api/historyservice/v1/service_grpc.pb.go:108` |
| Public doc states activities are at-least-once or at-most-once | `docs/architecture/README.md:35` | `docs/architecture/README.md:30-40` |
| Nexus callback IDs derived from request ID | `chasm/lib/workflow/workflow.go:176` (`requestID (unique per API call) + idx (position within the request) ensures unique, idempotent callback IDs.`) | `chasm/lib/workflow/workflow.go:170-180` |
| Callback execution tracks Attempt counter | `components/callbacks/statemachine.go:75` (`CallbackInfo.Attempt++`) | `components/callbacks/statemachine.go:60-90` |
| Callback execution stores RequestId | `components/callbacks/executors.go:166-167` (`requestID: callback.RequestId`) | `components/callbacks/executors.go:155-180` |
| Callback scheduler idempotency ("if a sentinel already exists, succeed idempotently") | `chasm/lib/scheduler/handler.go:148` | `chasm/lib/scheduler/handler.go:140-160` |
| Scheduler migration task idempotent on already-started | `chasm/lib/scheduler/scheduler_migrate_task.go:227-231` (treats `WorkflowExecutionAlreadyStarted` as success) | `chasm/lib/scheduler/scheduler_migrate_task.go:220-235` |
| Scheduler invoker dedups by RequestId | `chasm/lib/scheduler/invoker.go:202` (`if _, isDuplicate := completed[start.RequestId]; isDuplicate`) | `chasm/lib/scheduler/invoker.go:195-210` |
| Nexus operation completion dedups on RequestId | `components/nexusoperations/completion.go:186-203` (`operation.RequestId != requestID → NotFound`) | `components/nexusoperations/completion.go:170-216` |
| Pause workflow idempotent by PauseID | `tests/pause_workflow_execution_test.go:1323` (`idempotent pause must not add a second pause event`), `api/persistence/v1/executions.pb.go:4338` (`A unique identifier for this pause request (for idempotency checks)`) | `tests/pause_workflow_execution_test.go:1281-1323`, `api/persistence/v1/executions.pb.go:4338` |
| Worker Deployment operations idempotent by RequestID | `service/worker/workerdeployment/version_workflow.go:513`, `service/worker/workerdeployment/version_workflow.go:754` ("returning success so it is idempotent") | `service/worker/workerdeployment/version_workflow.go:513`, `service/worker/workerdeployment/version_workflow.go:754` |
| Nexus completion handler ignores retry of already-terminal operation | `components/nexusoperations/executors.go:739` ("operation already in terminal state" returns `ErrStaleReference`) | `components/nexusoperations/executors.go:730-740` |
| `dedupLinksFromCallbacks` deduplicates link-list before write | `service/frontend/workflow_handler.go:685`, `service/frontend/workflow_handler.go:6325-6336` | `service/frontend/workflow_handler.go:685-700`, `service/frontend/workflow_handler.go:6325-6340` |
| `RegisterWorker` buildID update dedups stale writes | `api/deployment/v1/message.pb.go:196`, `api/deployment/v1/message.pb.go:328` ("older than what is already in the TQ will be ignored to avoid stale writes") | `api/deployment/v1/message.pb.go:196`, `api/deployment/v1/message.pb.go:328` |
| `poller_history` removePoller is idempotent | `service/matching/poller_history_test.go:33` | `service/matching/poller_history_test.go:33` |

## Answers to Dimension Questions

### 1. Can the same work run twice?

**Yes for several subsystems, no for the most safety-critical ones.**

- Workflow start: **No.** The `current_executions` row uses
  `create_request_id` and conditional write to guarantee the same workflow
  isn't created twice for the same `(namespace, workflowID)` —
  `service/history/api/startworkflow/api.go:319-368`,
  `common/persistence/data_interfaces.go:113-125`.
- Activity start: **No (at-most-once delivery to history, at-least-once
  to the worker).** History collapses duplicate
  `RecordActivityTaskStarted` calls when the `RequestID` matches the
  previously-started activity's `RequestId`
  (`service/history/api/recordactivitytaskstarted/api.go:150-171`).
  But because the worker may have *already* run the activity (the RPC
  may have succeeded but the ack was lost), the *side effect of running
  the activity* can occur twice. This is why `docs/architecture/README.md:35`
  documents "at least once or at most once" for activities.
- Workflow Task Started: **No.** Same pattern — `RequestID` equality
  check (`service/history/api/recordworkflowtaskstarted/api.go:98-112`).
- Signal: **No** when `RequestId` is supplied — history returns a no-op
  (`service/history/api/signalworkflow/api.go:45-50`).
- Workflow Update (accepted/completed): **No** — registry dedups by
  Update ID and persisted `UpdateInfo` is read on replay
  (`service/history/workflow/update/registry.go:226-235`).
- Workflow Update (admitted but not yet sent to worker): **Possibly yes.**
  If the registry is cleared before the Update is sent, the Update is
  lost; on rebuild from history it is *not* in the registry because no
  history event exists for it yet (the `UpdateAdmitted` event is still a
  TODO — see `docs/architecture/workflow-update.md:70-83`). The
  server returns `WorkflowUpdateAbortedErr` and the *client* must retry
  with the same `UpdateID` for the next attempt to be deduplicated.
- Update rejection: **Yes.** Rejected Updates are not stored anywhere
  and therefore can be delivered to the worker twice
  (`docs/architecture/workflow-update.md:75-83`).
- Task queue delivery to workers: **At-most-once for history responses.**
  The `RequestId` on `RecordWorkflowTaskStarted` /
  `RecordActivityTaskStarted` collapses duplicate calls
  (`api/historyservice/v1/request_response.pb.go:1171`).
- Backlog re-enqueue on failure: **Yes (at-least-once).** If the matching
  engine cannot deliver a task from the backlog, it re-appends the same
  task at a new task ID — `service/matching/backlog_manager.go:231-260`.
- Nexus / Chasm callbacks: **At-least-once with attempt counter and
  circuit breaker.** `Attempt++` on each failure
  (`components/callbacks/statemachine.go:75`), retries continue via the
  outbound queue (`service/history/outbound_queue_factory.go:86-292`)
  with circuit-breaker fallback
  (`service/history/circuitbreakerpool/fx.go:14-44`).
- Cross-cluster replication: **Once per cluster under versioned transitions.**
  `VectorClock` + `TransitionCount` guard against replay
  (`service/history/ndc/replication_task.go:118-236`,
  `service/history/ndc/workflow_state_replicator.go`).

### 2. Is duplicate execution safe?

**Mostly.** Duplicate execution is safe for:

- Workflow creation (deduped on `RequestID`).
- Workflow Task Started (history collapses on `RequestID`).
- Activity Task Started (history collapses on `RequestID` and `Stamp`).
- Signal (`RequestID`).
- Update when admitted or further along.
- Pause (`PauseID` per `api/persistence/v1/executions.pb.go:4338`).
- Worker Deployment create (`RequestID`).

**It is NOT safe for:**

- The actual **activity execution** by the worker — by design the worker
  may run the activity more than once. The platform explicitly requires
  users to write idempotent activity code or use retry policy `at-most-once`
  (`docs/architecture/README.md:35`).
- Rejected Workflow Updates — they can be delivered twice because
  rejection is not persisted (`docs/architecture/workflow-update.md:75-83`).
- Outbound callbacks — they are retried on failure and rely on the
  destination being idempotent (`docs/architecture/nexus.md:339-353`).

### 3. Which side effects are idempotent?

Idempotent side effects:

- Persisting a workflow execution record (conditional write on
  `(namespace, workflowID, run_id)` and `last_write_version` —
  `common/persistence/cassandra/mutable_state_store.go:399-444`).
- Persisting history events (append-only history tree).
- Creating a `current_executions` row (only one succeeds).
- Adding history events with the same `scheduledEventId` to mutable
  state (idempotent on the `StartedEventId != EmptyEventID` check
  plus `RequestID` equality).
- Writing signal requested IDs (`IsSignalRequested` is a set check
  before insert).
- Adding callbacks (`requestID + idx` is a deterministic unique key per
  `chasm/lib/workflow/workflow.go:176`).
- Enqueuing queue messages (Cassandra `INSERT ... IF NOT EXISTS` —
  `common/persistence/cassandra/queue_v2_store.go:32`).
- Shard ownership update (range-id assertion
  `service/history/shard/context_impl.go:1155-1199`).

Non-idempotent side effects:

- Activity execution on the worker (user code).
- Nexus HTTP delivery (best-effort; relies on destination idempotency —
  `components/callbacks/nexus_invocation.go:71-88`,
  `docs/architecture/nexus.md:339-353`).
- Outbound queue re-enqueue (same task can be appended twice under
  different task IDs in failure paths).

### 4. Does the system claim exactly-once semantics?

**No system-wide claim.** The public docs explicitly say "activity code
must either be idempotent or non-retryable (i.e. at least once or at
most once)" (`docs/architecture/README.md:35`) and the API contract for
`RecordWorkflowTaskStarted` says "at most once delivery of tasks"
(`api/historyservice/v1/request_response.pb.go:1171`). The
matching service re-enqueues failed tasks at new task IDs
(`service/matching/backlog_manager.go:236-243`).

Within individual subsystems the system approaches effectively-once:

- Workflow creation is exactly-once by `RequestID`.
- Workflow / activity task started is exactly-once (per `(scheduledEventID, RequestID)`).
- Signal is exactly-once by `RequestID` (when supplied).
- Update is exactly-once-by-id once accepted.
- Nexus operation completion is exactly-once by `RequestID`
  (`components/nexusoperations/completion.go:186-203`).

### 5. Are guarantees documented?

- Yes for poll responses: API-level comments say "at most once delivery"
  (`api/historyservice/v1/request_response.pb.go:1171`).
- Yes for activities: `docs/architecture/README.md:35`.
- Yes for updates: `docs/architecture/workflow-update.md:75-83`,
  `docs/architecture/workflow-update.md:139-145` (rejected updates and
  registry-loss scenarios explicitly documented).
- Yes for Nexus callbacks: `docs/architecture/nexus.md:339-353`
  ("continuously retried using a configurable retry policy").
- For workflow creation, idempotency is implicit in the
  `WorkflowIdReusePolicy` / `WorkflowIdConflictPolicy` semantics and the
  `WorkflowExecutionAlreadyStarted` error path
  (`service/history/api/workflow_id_dedup.go:32-249`).

There is no single "Delivery Guarantees" page that consolidates the
full picture; each subsystem's behavior has to be discovered by reading
code and the per-feature docs.

### If a worker crashes after doing the work but before acknowledging it, what happens?

For **workflow / activity tasks**: the work has already been recorded by
history. The worker that polled the task already received a `Started`
response. When the worker reconnects, it does not see this task again
because the backlog advanced `ackLevel` past its task ID
(`service/matching/ack_manager.go:97-128`,
`service/matching/backlog_manager.go:231-272`). History has the started
event and any subsequent events the worker wrote; a different worker
polling for the *next* task will see them and continue from there.

For **workflow tasks specifically**: the started event plus its `Clock`
vector is persisted, and on the next WFT completion the SDK must
report the result for the same `startedEventID`. The matching
service will not redeliver the same task ID to another poller.

For **activities**: the activity start is recorded on the worker side
(implicit) but not on the history side until the SDK reports back via
`RecordActivityTaskStarted`. If the worker crashes after
`RecordActivityTaskStarted` but before the activity body runs or before
reporting completion, the activity is left in "started" state and will
be timed out by history's activity start-to-close timer. That timer
then schedules a retry attempt, which results in another worker
receiving the task with the same `scheduledEventID` but a fresh attempt
number. The retry *is* delivered; the activity body may run a second
time. This is the documented "at least once" behavior.

For **outbound callbacks / Nexus**: the attempt counter is persisted
(`components/callbacks/statemachine.go:75`); the next attempt is
scheduled by history via the outbound queue
(`service/history/outbound_queue_factory.go:86-292`).

For **replication**: an in-flight replication task may be retried by
the source cluster after an ack is lost; on the receiving cluster the
task processor logs `maxRxProcessedTimestamp` and detects duplicates
(`service/history/replication/task_processor.go:520-560`).

## Architectural Decisions

- **Event-sourced workflow state** (`docs/architecture/README.md:32`).
  Workflow state can be fully rebuilt by replaying history events; this
  means updates that touch only mutable-state in memory can be lost
  without violating durability, but anything written as an event is
  durable exactly once at the storage level (via conditional inserts).
- **Conditional writes with RequestID-style identifiers**.
  `current_executions` rows carry a `create_request_id`; conflicting
  writes surface as `CurrentWorkflowConditionFailedError` which carries
  the full request-id set so the history engine can dedup
  (`common/persistence/data_interfaces.go:113-125`,
  `service/history/api/startworkflow/api.go:319-368`).
- **Range-id protected task IDs**. Each history shard owns a RangeID
  embedded in the high bits of every task ID
  (`service/history/shard/task_key_generator.go:151-184`). Lost ownership
  invalidates in-flight task IDs and forces rescheduling.
- **Per-partition ack levels for matching**. Matching separates
  read-level (highest delivered task) from ack-level (highest
  GC-eligible task), so a slow worker doesn't block newer tasks
  (`service/matching/ack_manager.go:13-22`).
- **Per-task idempotency keys on workflow RPCs**. Every public API
  takes `RequestId`; history rejects duplicates via per-call
  RequestID checks (signal, update, etc.) and per-call stamp checks
  (workflow / activity tasks).
- **Stamp-and-clock identity for matching tasks**. Each enqueued task
  carries a `Stamp` (and vector clock) that the history engine uses to
  reject stale tasks
  (`service/history/api/recordactivitytaskstarted/api.go:178-184`,
  `service/history/api/recordworkflowtaskstarted/api.go:76-79`).
- **In-memory dedup tables with rebuild-from-events**.
  `update.Registry` keeps an in-memory `map[string]*Update` plus a
  persisted `UpdateInfo` store; on registry clear, the registry is
  rebuilt from the store
  (`service/history/workflow/update/registry.go:168-224`).
  Signal-requested IDs follow the same pattern.
- **CHASM signal backlinks**. `IsSignalRequested` now consults both
  a CHASM map and the legacy `pendingSignalRequestedIDs` map, with
  rollout controlled by `ChasmSignalBacklinksEnabled`
  (`service/history/workflow/mutable_state_impl.go:2499-2518`).
- **Outbound queue grouping by destination**. Nexus / Chasm callbacks
  are grouped by destination URL for circuit-breaking
  (`service/history/outbound_queue_factory.go:86-292`,
  `service/history/circuitbreakerpool/fx.go:14-44`).

## Notable Patterns

- **Per-call RequestID keys**. Used uniformly across signal, update,
  workflow-create, pause, worker-deployment, schedule actions. The
  frontend auto-generates RequestIDs when the caller does not supply
  one (`service/frontend/workflow_handler.go:4210-4214`,
  `service/frontend/workflow_handler.go:6595-6607`).
- **Stamp-and-clock mismatch rejection**. Used to reject tasks that
  are from a stale matching decision (worker picked up a task that
  was already rescheduled by history with a new stamp).
- **Idempotent "treat-already-X-as-success" in workers / schedulers**.
  `service/worker/workerdeployment/version_workflow.go:513`,
  `service/worker/workerdeployment/version_workflow.go:754`,
  `chasm/lib/scheduler/handler.go:148`,
  `chasm/lib/scheduler/scheduler_migrate_task.go:227-231`,
  `service/worker/scheduler/activities.go:387`.
- **`StartDeduped` outcome metric**.
  `metrics.StartWorkflowRequestDeduped.With(...)` in
  `service/history/api/startworkflow/api.go:329`.
- **`Update` state machine "no-op on Admit if already past Created"**.
  `service/history/workflow/update/update.go:320-326`.
- **Cassandra `INSERT ... IF NOT EXISTS` semantics on queue insert**.
  `common/persistence/cassandra/queue_v2_store.go:32`.
- **Poller history with idempotent remove**
  (`service/matching/poller_history_test.go:33`).
- **`dedupLinksFromCallbacks`** front-end helper that strips duplicate
  links/callbacks before persisting
  (`service/frontend/workflow_handler.go:685`,
  `service/frontend/workflow_handler.go:6325-6336`).
- **CHASM internal callbacks** use Nexus completion semantics but with
  an internal delivery URL — completion is deduplicated by `RequestId`
  (`components/callbacks/executors.go:159-180`,
  `components/callbacks/chasm_invocation.go:70-127`).
- **Outbound circuit breaker per destination** to avoid retry storms
  when a callback destination is down
  (`service/history/outbound_queue_factory.go:170`,
  `service/history/circuitbreakerpool/fx.go:14-44`).

## Tradeoffs

- **Effectively-once for the durable state machine, at-least-once for
  user-execution side effects.** Temporal chooses to make every
  workflow / activity / signal / update state-machine operation
  idempotent via RequestIDs and stamps, but it cannot make the
  user-supplied activity body idempotent, so retries are
  at-least-once by design. The user is responsible for making activity
  bodies idempotent.
- **In-memory dedup tables lose state on errors**. Updates that are
  admitted but not yet persisted to history are lost if the registry is
  cleared (`docs/architecture/workflow-update.md:70-83`). The system
  mitigates this by returning a retryable error to the client so the
  client retries with the same `UpdateID` and the next attempt is
  deduplicated.
- **Shard range-id churn resets task IDs**. When a shard loses
  ownership and is reassigned, the new range ID forces task ID
  regeneration (`service/history/shard/task_key_generator.go:151-184`).
  This is a correctness requirement (no cross-owner reuse of task IDs)
  but it is also a heavy operation.
- **Re-enqueue on failure instead of in-place retry**. The matching
  service chooses to append the task at a new task ID rather than
  retry in place, ensuring progress for downstream tasks
  (`service/matching/backlog_manager.go:236-243`). This means a task
  whose matching fails may end up being delivered twice — to two
  different pollers — before history rejects the second with a stale
  stamp.
- **Outbound queue retries continue until either success or DLQ**.
  There is no global cap; only the workflow retention period bounds
  retries (`docs/architecture/nexus.md:348-353`). This is necessary
  for at-least-once but means a misconfigured destination can rack up
  retry storms until the circuit breaker kicks in
  (`service/history/outbound_queue_factory.go:170`).
- **Conditional writes are a thin layer over the persistence
  interface**. The conditional logic lives in
  `common/persistence/sql/execution.go:81-208` and
  `common/persistence/cassandra/mutable_state_store.go:399-444`, with
  the engine trusting them implicitly. There is no upper-level
  validator that "if I write X with id Y and the same id Y is already
  there, I get back the original X". The history engine has to look
  up the conflict result and reconcile.

## Failure Modes / Edge Cases

- **Worker crashes after starting a task but before reporting completion**.
  History sees the start but not the completion. The activity's
  start-to-close timer fires and a retry is scheduled. The activity
  body may run twice. Documented by `docs/architecture/README.md:35`.
- **Admitted-but-unsent Update lost on registry clear**. Returns
  `WorkflowUpdateAbortedErr` to caller; client must retry
  (`docs/architecture/workflow-update.md:70-83`).
- **Rejected Update redelivered**. Rejection is not persisted, so a
  client poll may receive it twice or see the rejection
  (`docs/architecture/workflow-update.md:75-77`).
- **Cassandra `INSERT ... IF NOT EXISTS` race**. Concurrent writers
  may both see the same max(message_id) and only one succeeds; the
  other gets `ErrEnqueueMessageConflict`
  (`common/persistence/cassandra/queue_v2_store.go:43-72`).
- **Shard ownership lost while writing**. RangeID mismatch is
  detected by `AssertShardOwnership` and surfaces as
  `ShardOwnershipLostError`; the client retries on a different host
  (`service/history/shard/context_impl.go:1147-1199`).
- **Speculative WFT lost**. A speculative WFT may be created in
  mutable state without being persisted; if matching drops it, history
  must fall back to a fresh WFT (`service/history/api/recordworkflowtaskstarted/api.go:69-93`).
- **Sticky queue mismatch**. If the workflow's current task queue
  drifts from the poller's queue, history rejects the start
  (`service/history/api/recordworkflowtaskstarted/api.go:121-149`).
- **Resume after reset for Nexus completion**.
  `CompletionHandler` falls back to "no run ID" if the operation was
  on a reset run (`components/nexusoperations/completion.go:204-213`).
- **Workflow completion between Describe and Update / signal**. The
  scheduler's `scheduler_tasks.go` specifically handles this by
  returning `WorkflowExecutionAlreadyStarted` (which is a misnomer but
  indicates "already terminal") as success
  (`chasm/lib/scheduler/scheduler_tasks.go:311`).
- **Schedule v1 ↔ v2 migration duplicates**. There is a regression test
  for migration dedup of RunningWorkflows / RecentActions
  (`tests/schedule_migration_test.go:1311-1443`).
- **`lifecycleStop` and `addReminderOrUpdate` idempotency in
  WorkerDeployment**. The client.go dedup paths use deterministic
  request UUIDs (`service/worker/workerdeployment/client.go:2033-2046`).

## Future Considerations

- **Persist admitted Updates** (`docs/architecture/workflow-update.md:70-83`).
  An `UpdateAdmitted` event would close the window where a registry
  clear silently drops in-flight updates.
- **Persist rejected Updates** (`docs/architecture/workflow-update.md:79-83`).
  Currently rejected Updates are not deduped; persisting the rejection
  would enable at-most-once delivery for that case.
- **Replace CHASM fallback for signal dedup**. The mutable state still
  consults both `pendingSignalRequestedIDs` and the CHASM signal map
  until the CHASM rollout completes
  (`service/history/workflow/mutable_state_impl.go:2499-2518`).
- **Generic per-API idempotency store**. Each API today maintains its
  own dedup table (signal, update, callback, schedule, deployment,
  etc.). A generic per-namespace idempotency key store would simplify
  the surface and let users supply their own keys.
- **Tighter stamp coordination**. The `Stamp` is checked at the
  Record-Task-Started boundary but there is no end-to-end invariant
  that "the workflow's last-known stamp is monotonically non-decreasing
  from the worker's perspective".
- **Outbound queue retries could be bounded more visibly**. There is no
  explicit cap surfaced in dynamic config beyond circuit breaker
  settings (`common/dynamicconfig/constants.go:2501` shows updates-only
  semantics in some places).
- **Nexus completion deduplication relies on RequestID being present**.
  Older SDKs may not send one
  (`components/nexusoperations/completion.go:173-176`).

## Questions / Gaps

- **No explicit "Delivery Guarantees" doc page** covering the end-to-end
  story. Each subsystem's behavior is documented separately (workflow
  update docs, nexus docs, README), but no centralized summary of "what
  happens when I retry X" exists in `docs/`. Search boundary:
  `docs/architecture/*.md`.
- **The "exactly-once" guarantee for paused workflows** relies on
  `PauseID` (`api/persistence/v1/executions.pb.go:4338`), but I did not
  find an explicit mention of whether a duplicate `PauseActivity`
  call returns the original pause info or errors out — only the test
  `tests/pause_workflow_execution_test.go:1323` confirms behavior. The
  pause handler dedup logic could not be located in the timeframe.
  Search boundary: `service/history/api/pauseactivity*`,
  `service/history/api/pauseworkflow*` (no exact match found).
- **The CHASM activity idempotency model differs from the classic
  mutable-state activity path**: the CHASM path returns the existing
  response on retry (`chasm/lib/activity/activity.go:217-235`), while
  the classic path returns `NewTaskAlreadyStarted` only if the
  requestID differs (`service/history/api/recordactivitytaskstarted/api.go:150-171`).
  Two different idempotency contracts in two different code paths
  that surface as the same RPC to clients.
- **The behavior of `signal` dedup with empty RequestID** is documented
  in `service/history/api/signalworkflow/api.go:45-50`: empty
  RequestID disables dedup and accepts the signal. There is no
  documentation that warns users about this. Search boundary:
  `docs/architecture/*.md`.
- **Update-with-Start retry semantics on closed workflows** are
  guarded by dynamic config `EnableUpdateWithStartRetryOnClosedWorkflowAbort`
  and `EnableUpdateWithStartRetryableErrorOnClosedWorkflowAbort`
  (`service/history/api/multioperation/api.go:131-149`). This is a
  product-level knob, not a documented contract.
- **Outbound queue retry budgets**: I could not find a documented
  maximum number of callback retries; only the workflow retention
  period bounds retries
  (`docs/architecture/nexus.md:348-353`).
- **Nexus completion on already-terminal operation returns
  `ErrStaleReference`** (`components/nexusoperations/executors.go:739`),
  but I did not find explicit tests for "duplicate completion
  arriving after the operation already completed" beyond the manual
  state-machine comparisons. Search boundary:
  `tests/nexus_*_test.go`, `components/nexusoperations/*_test.go`.

---

Generated by `01.09-delivery-guarantees-and-idempotency` against `temporal`.