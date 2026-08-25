# Source Analysis: temporal

## 01.05 — Pause, Resume, and Interrupt Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `sources/temporal` |
| Language / Stack | Go (Temporal server), gRPC, protobuf, Cassandra/MySQL/Postgres/SQLite persistence backends |
| Analyzed | 2026-07-02 |

## Summary

Temporal implements first-class pause and unpause primitives for both workflows and activities, distinct from cancel/terminate. Pause is a durable, persisted state change that is recorded as an event in workflow history, written atomically with mutable state via the standard `GetAndUpdateWorkflowWithNew`/`UpdateWorkflowExecutionAsActive` transaction path (`service/history/api/update_workflow_util.go:14-125`). Pausing a workflow flips the execution status to `PAUSED` (`service/history/workflow/mutable_state_impl.go:3192-3247`), and `UpdateWorkflowWithNew` refuses to schedule any further workflow task while paused (`service/history/api/update_workflow_util.go:80-90`). Pausing an activity sets `ActivityInfo.Paused`/`PauseInfo` and bumps a stamp; transfer tasks generated for already-scheduled activities are dropped on dispatch if the activity is paused or the stamp has changed (`service/history/transfer_queue_active_task_executor.go:259-262`). Resume is symmetric (`unpauseworkflow/api.go`, `unpauseactivity/api.go`), scheduled workflow task is created on unpause, and pending activity retry/tasks are regenerated (`service/history/workflow/mutable_state_impl.go:3280-3302`). Signals received while paused are buffered in history and not delivered to the worker until unpause (`tests/pause_workflow_execution_test.go:1159-1272`). Heartbeat RPCs report the paused flag to the worker so it can stop in-flight activities (`service/history/api/recordactivitytaskheartbeat/api.go:82-105`). Persistence is durable because the pause state lives in mutable state and history events; mutable state is rebuilt from history on process restart via `mutable_state_rebuilder.go` (`service/history/workflow/mutable_state_rebuilder.go:671-678`). Idempotency is supported via `RequestId` (`service/history/api/pauseworkflow/api.go:61-66`). In-flight running activities on workers that do not poll heartbeats may continue running until their next heartbeat, because the server-side pause only blocks new task dispatch and signals the worker via heartbeat.

## Rating

**Score: 9 / 10** — Mature, durable, observable, extensible, and proven under failure.

Rationale:
- Pause/unpause are first-class state-machine transitions, not ad-hoc flags, recorded in history and replicated.
- Resume is deterministic via persisted events and mutable state; no in-memory-only state.
- A workflow "approving a pending action tomorrow" survives crash/restart and resumes from the same state — `mutable_state_rebuilder.go:671-678` re-applies `WORKFLOW_EXECUTION_PAUSED`/`UNPAUSED` events on rebuild.
- Idempotency, validation, and request IDs are explicit (`pauseworkflow/api.go:61-66,99-111`).
- Buffered events during pause prevent loss and guarantee order (`event_store.go:352-356`).
- Stamp-based staleness detection in transfer queue guarantees paused activities never dispatch to a worker (`transfer_queue_active_task_executor.go:259-262`).
- Heartbeat-based in-flight interruption is observed by the worker on the next heartbeat tick (`recordactivitytaskheartbeat/api.go:82-105`).
- The only reason this is not 10: there is a known race window (`tests/pause_workflow_execution_test.go:853-983`) between an in-flight workflow task and pause that requires extra logic; in-flight activities on a stale worker that does not poll heartbeats cannot be force-stopped synchronously by the server.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers. Format: `path/to/file.go:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pause workflow API entrypoint | `Invoke` calls `GetAndUpdateWorkflowWithNew` with state-check + event add | `service/history/api/pauseworkflow/api.go:17-97` |
| Unpause workflow API entrypoint | `Invoke` creates a new workflow task on unpause | `service/history/api/unpauseworkflow/api.go:17-89` |
| Pause activity API entrypoint | Targets by id or type; constructs `PauseInfo` | `service/history/api/pauseactivity/api.go:19-96` |
| Unpause activity API entrypoint | Calls `workflow.UnpauseActivity`; supports reset/heartbeat/jitter | `service/history/api/unpauseactivity/api.go:18-130` |
| State mutation for workflow pause | `AddWorkflowExecutionPausedEvent` appends event and updates status | `service/history/workflow/mutable_state_impl.go:3196-3247` |
| State mutation for workflow unpause | `AddWorkflowExecutionUnpausedEvent` clears status, reschedules activities | `service/history/workflow/mutable_state_impl.go:3249-3308` |
| `UpdateWorkflowWithNew` blocks workflow task scheduling while paused | `if !mutableState.IsWorkflowExecutionStatusPaused() && !HasPendingWorkflowTask()` | `service/history/api/update_workflow_util.go:79-90` |
| Activity `PauseActivity` core logic | Sets `Paused=true`, stamps the activity | `service/history/workflow/activity.go:271-301` |
| Activity `UnpauseActivity` core logic | Clears `Paused`, regenerates retry task | `service/history/workflow/activity.go:405-442` |
| `unpauseActivityInfo` mutator | Clears paused + stamp increment | `service/history/workflow/activity.go:398-403` |
| Stamp-based staleness check in transfer queue | Drops task if `ai.Stamp != task.Stamp \|\| ai.Paused` | `service/history/transfer_queue_active_task_executor.go:259-262` |
| Event buffering for pause/unpause | `bufferEvent` returns true for PAUSED/UNPAUSED to allow ordering with inflight WT | `service/history/historybuilder/event_store.go:302-360` |
| Event factory creates paused events | `EVENT_TYPE_WORKFLOW_EXECUTION_PAUSED/UNPAUSED` | `service/history/historybuilder/event_factory.go:1052-1082` |
| HistoryBuilder emits paused events with `WorkerMayIgnore=true` | So older SDKs can safely ignore | `service/history/historybuilder/history_builder.go:292-314` |
| Rebuild on history replay applies pause/unpause | `ApplyWorkflowExecutionPausedEvent/UnpausedEvent` | `service/history/workflow/mutable_state_rebuilder.go:671-678` |
| Heartbeat reports paused state to worker | `RecordActivityTaskHeartbeat` returns `ActivityPaused` | `service/history/api/recordactivitytaskheartbeat/api.go:82-105` |
| Pending activity state semantics (`PAUSE_REQUESTED` vs `PAUSED`) | State derivation based on `ai.Paused` and started state | `service/history/workflow/activity.go:185-223` |
| `IsActivityTaskValid` stamp check on polling | Compares `ai.Stamp` to requested stamp | `service/history/api/isactivitytaskvalid/api.go:49-63` |
| Frontend pause handler | `PauseWorkflowExecution` calls history service | `service/frontend/workflow_handler.go:7244-7270` |
| Frontend unpause handler | `UnpauseWorkflowExecution` calls history service | `service/frontend/workflow_handler.go:7272-7304` |
| Frontend pause/unpause activity handler | `PauseActivity`/`UnpauseActivity` call history service | `service/frontend/workflow_handler.go:6854-6917` |
| Feature flag gating | `WorkflowPauseEnabled` namespace-level dynamic config | `common/dynamicconfig/constants.go:3452-3456` |
| Feature flag check in frontend | Blocks pause RPC if disabled | `service/frontend/workflow_handler.go:7252-7254` |
| Pause idempotency on `RequestId` | Re-issuing same RequestId returns no-op | `service/history/api/pauseworkflow/api.go:59-67` |
| Validation of reason/requestId/identity length | `validatePauseRequest` | `service/history/api/pauseworkflow/api.go:99-112` |
| Pre-flight workflow running check | `IsWorkflowExecutionRunning()` + status check | `service/history/api/pauseworkflow/api.go:51-69` |
| Persistence struct for workflow pause | `WorkflowExecutionInfo.PauseInfo` field | `api/persistence/v1/executions.pb.go:348` |
| Persistence struct `WorkflowPauseInfo` | PauseTime, Identity, Reason, RequestId | `api/persistence/v1/executions.pb.go:4330-4402` |
| Persistence struct `ActivityInfo.PauseInfo` | PauseTime, Manual{Identity, Reason} or RuleId | `api/persistence/v1/executions.pb.go:4504-4605` |
| PauseInfo type union (Manual vs Rule) | `isActivityInfo_PauseInfo_PausedBy` oneof | `api/persistence/v1/executions.pb.go:4579-4595` |
| Cancel while paused is preserved & replayed on unpause | Test assertion + event ordering | `tests/pause_workflow_execution_test.go:1505-1578` |
| Pause idempotent by RequestId test | `TestPauseIdempotentSameRequestId` | `tests/pause_workflow_execution_test.go:1277-1346` |
| Signal buffering order while paused | `TestSignalBufferingOrderWhilePaused` | `tests/pause_workflow_execution_test.go:1159-1272` |
| Race regression: pause during in-flight workflow task | `TestPauseDuringInFlightWorkflowTask` references issue #10239 | `tests/pause_workflow_execution_test.go:853-983` |
| Reset-while-paused produces fresh run, not paused | `TestResetWhilePaused` | `tests/pause_workflow_execution_test.go:1584-1669` |
| Activity retry deferred while workflow paused | `TestActivityRetryDeferredWhilePaused` | `tests/pause_workflow_execution_test.go:1677-1755` |
| Terminate while paused test | `TestTerminateWhilePaused` | `tests/pause_workflow_execution_test.go:1347-1395` |
| In-flight running activity pause via heartbeat test | `TestActivityPauseApi_WhileRunning` | `tests/activity_api_pause_test.go:31-150` |
| PauseInfo described on `DescribeWorkflow` | Frontend returns `PauseInfo` in extended info | `service/history/api/describeworkflow/api.go:155-162` |
| Pause info search attribute | `TemporalPauseInfo` updated via `updatePauseInfoSearchAttribute` | `service/history/workflow/mutable_state_impl.go:6979-6995` |
| Paused activities metrics | `PausedActivitiesCounter`, `ActivityPauseRequests` | `common/metrics/metric_defs.go:1159-1163` |
| Frontend rate-limit quota for pause APIs | Quota per RPC | `service/frontend/configs/quotas.go:121-126,152-153` |
| Activity retry while paused returns `RETRY_STATE_IN_PROGRESS` | Comment notes future `RETRY_STATE_PAUSED` | `service/history/workflow/mutable_state_impl.go:6803-6819` |
| Worker control commands (cancel-activity) for in-flight cancel via Nexus | `worker_commands_task_dispatcher.go` | `service/history/worker_commands_task_dispatcher.go:39-229` |
| Activity cancel path generates `WorkerCommand_CancelActivity` | `respondworkflowtaskcompleted/workflow_task_completed_handler.go:737-744` | `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:664-748` |
| Activity dispatch via Nexus control task queue | Uses `EnableCancelActivityWorkerCommand` dynamic config | `service/history/worker_commands_task_dispatcher.go:94-103` |
| Reset activity API | Allows re-driving an activity while respecting paused state | `service/history/api/resetactivity/api.go:17-90` |
| Workflow rule engine can pause activities | `ActivityPause` rule action | `service/history/workflow/mutable_state_impl.go:9936-9953` |
| Pause info size tracking in mutable state | Size added/removed on pause/unpause | `service/history/workflow/mutable_state_impl.go:3226-3227,3304-3305` |
| Time skipping pauses while in-flight work exists | `task_generator.go` comment | `service/history/workflow/task_generator.go:1047` |
| Replication of paused status to passive cluster via stamp bump | `ApplyWorkflowExecutionPausedEvent` bumps stamp of pending activities | `service/history/workflow/mutable_state_impl.go:3229-3244` |
| Pause across failure scenarios | Mutable state rebuilt from history events on restart | `service/history/workflow/mutable_state_rebuilder.go:671-678` |

## Answers to Dimension Questions

### 1. Can execution pause safely?

Yes, for both workflows and activities.

- **Workflow pause**: `service/history/api/pauseworkflow/api.go:51-86` validates that the workflow is running and not already paused, appends a `WorkflowExecutionPaused` event to history, sets the workflow status to `PAUSED`, populates `WorkflowExecutionInfo.PauseInfo`, bumps stamps on all pending activities, and invalidates any pending workflow task by bumping its stamp (`service/history/workflow/mutable_state_impl.go:3213-3247`). The pause is durable because it goes through `GetAndUpdateWorkflowWithNew` → `UpdateWorkflowExecutionAsActive` (`service/history/api/update_workflow_util.go:14-125`), the same atomic transaction used for all workflow state mutations.
- **Activity pause**: `service/history/api/pauseactivity/api.go:64-69` calls `workflow.PauseActivity` (`service/history/workflow/activity.go:271-301`), which sets `ai.Paused=true` and `ai.PauseInfo` and bumps the activity stamp if scheduled. Crucially, the transfer queue's activity dispatch path drops the activity task when `ai.Paused` is true (`service/history/transfer_queue_active_task_executor.go:259-262`), so a paused activity will never be delivered to a fresh worker.
- In-flight activities already running on a worker are notified via the next heartbeat (`service/history/api/recordactivitytaskheartbeat/api.go:82-105`); the worker is responsible for stopping itself. Long-running activities that don't heartbeat continue until they do.

### 2. Can it resume after a crash?

Yes. Pause is recorded as a history event and as state on `WorkflowExecutionInfo.PauseInfo` and `ActivityInfo.PauseInfo` (persisted via the standard mutable state row). On shard restart or replica replay, `mutable_state_rebuilder.go` re-applies `WORKFLOW_EXECUTION_PAUSED` and `WORKFLOW_EXECUTION_UNPAUSED` events to mutable state (`service/history/workflow/mutable_state_rebuilder.go:671-678`). After unpause, the workflow is restarted from `Running` and pending activity retry/tasks are regenerated (`service/history/workflow/mutable_state_impl.go:3280-3302`), with the same stamp-based idempotency guard as before.

### 3. Is the resume point deterministic?

Yes for workflows (status flips back to `Running`, a single workflow task is scheduled by `unpauseworkflow/api.go:74-77`, and `UpdateWorkflowWithNew` ensures it is scheduled because `IsWorkflowExecutionStatusPaused()` is now false). For activities, unpause regenerates the activity retry task or schedules the activity task now, based on the activity's `ScheduledTime` (`service/history/workflow/mutable_state_impl.go:3292-3301`). The deterministic event ordering is preserved by buffering pause/unpause events (`service/history/historybuilder/event_store.go:352-356`).

### 4. What happens if the world changed while paused?

- **Signals received while paused** are recorded into history as `WorkflowExecutionSignaled` events but buffered (they do not get a workflow task scheduled) and are delivered to the workflow on unpause. Test `TestSignalBufferingOrderWhilePaused` asserts that signals delivered while paused preserve order and are drained in a single workflow task on unpause (`tests/pause_workflow_execution_test.go:1159-1272`).
- **Timer fires while paused** — `UpdateWorkflowWithNew` blocks scheduling of any new workflow task while paused (`service/history/api/update_workflow_util.go:80-90`), so a fired timer will record `TimerFired` to history but the resulting workflow task will be deferred until unpause.
- **Activity retries while paused** — `mutable_state_impl.go:6798-6819` short-circuits the retry path so the attempt count stays frozen; `TestActivityRetryDeferredWhilePaused` asserts this (`tests/pause_workflow_execution_test.go:1677-1755`). Comment `TODO: uncomment once RETRY_STATE_PAUSED is supported` indicates this state is intentionally normalized to `RETRY_STATE_IN_PROGRESS` for now.
- **Cancel requested while paused** — the cancel event is recorded in history but not delivered until unpause; `TestCancelWhilePaused` asserts the workflow ends CANCELED only after unpause (`tests/pause_workflow_execution_test.go:1505-1578`).
- **Reset while paused** — terminates the current (paused) run and produces a new run that is RUNNING (not paused); pause is per-run, not per-workflow-id (`tests/pause_workflow_execution_test.go:1584-1669`).

### 5. Can multiple people or systems resume the same run?

Yes. Unpause has no `RequestId` idempotency check at the state-machine layer (unlike pause, where same-RequestId is a no-op) but it does guard against double-unpause with a precondition: `mutableState.GetExecutionState().GetStatus() != enumspb.WORKFLOW_EXECUTION_STATUS_PAUSED` returns `FailedPrecondition: workflow is not paused` (`service/history/api/unpauseworkflow/api.go:58-62`). The state-transition lock via `WorkflowConsistencyChecker.GetWorkflowLease` (`service/history/api/consistency_checker.go:114-122`) ensures that two simultaneous unpause RPCs serialize; the second sees `Status=RUNNING` and fails precondition. Pause has explicit RequestId idempotency (`service/history/api/pauseworkflow/api.go:61-66`).

## Architectural Decisions

- **Pause as first-class event** rather than an external flag: `EVENT_TYPE_WORKFLOW_EXECUTION_PAUSED` and `EVENT_TYPE_WORKFLOW_EXECUTION_UNPAUSED` are top-level history event types with their own factory (`service/history/historybuilder/event_factory.go:1052-1082`) and rebuilder paths (`service/history/workflow/mutable_state_rebuilder.go:671-678`). This makes pause observable in workflow history, replay-safe, and replicated (`service/history/workflow/mutable_state_impl.go:3229-3244` bumps stamps to force replication to passive clusters).
- **Event buffering for pause/unpause** so an in-flight workflow task can complete concurrently without racing the pause: `event_store.go:352-356` explicitly buffers PAUSED/UNPAUSED events to preserve ordering with inflight WT completions.
- **Stamp-based staleness detection** for activities: the pause bumps `ai.Stamp` (`service/history/workflow/activity.go:295-296,401`), the transfer queue checks `ai.Stamp != task.Stamp || ai.Paused` and drops the task (`service/history/transfer_queue_active_task_executor.go:259-262`), and `IsActivityTaskValid` (`service/history/api/isactivitytaskvalid/api.go:60-63`) compares stamps for activity polls.
- **Worker-driven in-flight interrupt via heartbeat** rather than server force-stop: heartbeat RPC returns `ActivityPaused` flag (`service/history/api/recordactivitytaskheartbeat/api.go:82-105`). Worker is responsible for honoring it. This is the same model Cadence uses.
- **Separate workflow-level pause from activity-level pause**: pausing a workflow does NOT mark each activity as paused (`service/history/workflow/mutable_state_impl.go:3229-3237` only bumps stamps). Activity-level pause is explicit (`service/history/workflow/activity.go:271-301`). The `TemporalPauseInfo` search attribute carries both kinds (`service/history/workflow/mutable_state_impl.go:6953-6977`).
- **Pause vs Cancel semantics**: cancel records `WorkflowExecutionCancelRequested` and lets the workflow observe cancellation on next WT; pause prevents any WT from being scheduled at all (`service/history/api/update_workflow_util.go:80-90`). The two compose: a cancel recorded during pause is observed only after unpause (`tests/pause_workflow_execution_test.go:1505-1578`).
- **Per-run pause**: reset while paused starts a fresh RUNNING run (`tests/pause_workflow_execution_test.go:1584-1669`). ContinueAsNew and Reset are not paused by inheritance.
- **Reasoning-attached metadata**: `PauseInfo` carries Identity, Reason, and RequestId (workflow) and Manual{Identity, Reason} or RuleId (activity), enabling audit (`api/persistence/v1/executions.pb.go:4330-4402,4504-4605`).
- **Worker command cancel path**: separate from pause — `service/history/worker_commands_task_dispatcher.go:39-211` dispatches `CancelActivity` worker commands over the worker's Nexus control queue when the workflow itself requests an activity cancel (`service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:737-744`). This is best-effort and disabled by default (`EnableCancelActivityWorkerCommand`).

## Notable Patterns

- **Idempotent RequestId on pause** but precondition-only on unpause (`pauseworkflow/api.go:61-67` vs `unpauseworkflow/api.go:58-62`).
- **Validation pass before state mutation** in both pause APIs (`pauseworkflow/api.go:34-36,99-111`; `pauseactivity/api.go`).
- **Mutability check via `checkMutability`** inside event constructors (`mutable_state_impl.go:3202,3255,9000-9014`) ensures paused events cannot be appended to closed workflows.
- **Cooperative cancellation via Stamp** rather than direct cancellation: identical pattern used by retry invalidation, pause, and reset.
- **Search attribute (`TemporalPauseInfo`) for visibility** of paused entities (`mutable_state_impl.go:6979-6995`).
- **Worker commands over Nexus control queue** for cancel-activity — feature-flagged off by default (`worker_commands_task_dispatcher.go:94-103`).
- **Pause/unpause events marked `WorkerMayIgnore=true`** so older SDKs can replay history without crashing (`history_builder.go:299,311`).
- **`approximateSize` accounting** of pause metadata into mutable state size so size limits remain accurate (`mutable_state_impl.go:3226-3227,3304-3305`).
- **Test naming** explicitly maps to issues/bugs, e.g. issue #10239 race (`tests/pause_workflow_execution_test.go:934-936`).

## Tradeoffs

- **Pause does not synchronously interrupt in-flight activities** — relies on the worker's heartbeat response. A worker that ignores heartbeats or whose activity ignores cancellation context will keep running until it completes or times out. This is documented in `recordactivitytaskheartbeat/api.go:38-105`.
- **Pause does not stop running timers** — fired timers append events to history but the workflow task is not scheduled; clock still progresses and timers still fire.
- **Heartbeat-driven interrupt is the only mechanism for runtime interrupt** of activities; there is no separate "force stop" API for a single activity.
- **Pause is a per-run state** — a ContinueAsNew or Reset naturally "resumes" because the new run starts fresh. There is no way to pause a workflow ID across all runs.
- **Race between in-flight workflow task and pause** is acknowledged (`tests/pause_workflow_execution_test.go:853-983`); the test exercises 200 iterations and asserts no desync (Status=RUNNING with pauseInfo set). The mitigation is buffering pause events and invalidating pending WT stamps.
- **`RETRY_STATE_PAUSED` not yet supported**: comment `TODO: uncomment once RETRY_STATE_PAUSED is supported` (`mutable_state_impl.go:6817-6818`) shows retry-while-paused is reported as `RETRY_STATE_IN_PROGRESS`.
- **Pause API quotas are tight** (`configs/quotas.go:121-126,152-153`) — 2 RPS per namespace by default, reflecting that pause/unpause are admin operations.

## Failure Modes / Edge Cases

- **Pause during in-flight workflow task**: handled by buffering the pause event (`event_store.go:352-356`) and bumping the workflow task stamp (`mutable_state_impl.go:3241-3244`). Regression test exists (`tests/pause_workflow_execution_test.go:853-983`).
- **Activity paused while running on worker**: heartbeat informs worker; activity that completes anyway is rejected by `IsActivityTaskValid`/`RecordActivityTaskStarted` stamp check (`isactivitytaskvalid/api.go:60-63`).
- **Workflow paused twice with different RequestIds**: rejected with `FailedPrecondition: workflow is already paused` (`pauseworkflow/api.go:68-69`).
- **Workflow paused then closed** (terminate, cancel-after-unpause, reset, etc.): `IsWorkflowExecutionRunning()` check blocks pause on closed workflows (`pauseworkflow/api.go:51-56`); `checkMutability` blocks paused-event construction on finished workflows (`mutable_state_impl.go:3202,3255`).
- **Unpause with no prior pause**: rejected with `FailedPrecondition: workflow is not paused` (`unpauseworkflow/api.go:58-62`).
- **Stale mutable state during pause**: `pauseworkflow/api.go:52-56` releases the lease with nil error and prevents re-loading, avoiding extra cache churn.
- **Activity not found during pause**: `pauseactivity/api.go:50-52,282-284` returns `ErrActivityNotFound`.
- **Persistence backend variation**: pause state is in the mutable state blob (`executions` table); works across Cassandra/MySQL/Postgres/SQLite as long as the persistence interface is implemented (`schema/`).
- **Replication to passive cluster**: pause bumps activity stamps (`mutable_state_impl.go:3229-3237`), forcing the paused-state to be replicated via existing task replication machinery.
- **Pause & then continue-as-new**: continue-as-new creates a new RUNNING run; pause does not carry over (per-run, not per-workflow-id).

## Future Considerations

- **`RETRY_STATE_PAUSED`**: explicit paused retry state is acknowledged as future work (`mutable_state_impl.go:6817-6818`).
- **PauseActivityExecution / UnpauseActivityExecution** in the public API are explicitly stubbed as `Unimplemented` in the frontend (`workflow_handler.go:7306-7317`). These may be intended future replacements for the `PauseActivity` / `UnpauseActivity` calls (note plural vs singular).
- **Worker cancellation via control queue**: currently gated by `EnableCancelActivityWorkerCommand` dynamic config (`configs/config.go:375,761`); broader rollout pending.
- **Cross-cluster pause observability**: only the active cluster tracks pause state; passive clusters replay it via replication but pause requests must target the active cluster (implicit via `GetActiveNamespace` in `pauseworkflow/api.go:23-26`).

## Questions / Gaps

- **What is the exact propagation latency of pause from server to in-flight activity?** Documented mechanism is heartbeat round-trip; no explicit SLA found in source.
- **Is there a way to pause a child workflow from a parent workflow command?** No `RequestPauseChildWorkflowExecution` command found in this source. Pause is API-driven only.
- **How does pause interact with StartDelay timers or cron schedules?** No explicit handling found; the timer would fire but the workflow task would not be scheduled (`update_workflow_util.go:80-90`).
- **What is the maximum size overhead of `PauseInfo` on mutable state?** Accounted via `approximateSize` but no explicit cap found.
- **Behavior of pause during a multi-operation `UpdateWorkflow` RPC**: `tests/pause_workflow_execution_test.go:986-1087` (`TestUpdateWorkflowWhilePaused`) exercises this but the final outcome is asserted via "completes workflow" path; deeper visibility into update registry behavior during pause would require more analysis.

---

Generated by `01.05-pause-resume-and-interrupt-semantics` against `temporal`.