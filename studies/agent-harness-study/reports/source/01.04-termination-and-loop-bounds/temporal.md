# Source Analysis: temporal

## Termination and Loop Bounds

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (history/frontend/matching/worker services; CHASM library; SDK-driven long-running scanner workflows) |
| Analyzed | 2026-07-02 |

## Summary

Temporal is a long-running distributed workflow engine, not a step-loop agent — termination is fundamentally an *event-driven* model rather than a turn-bounded one. A workflow execution only stops when one of six explicit terminal events is written to history (`Completed`, `Failed`, `TimedOut`, `Terminated`, `Canceled`, `ContinuedAsNew`), and each of those transitions is enforced in `service/history/workflow/mutable_state_impl.go:4834-5554` (`AddCompletedWorkflowEvent`, `AddFailWorkflowEvent`, `AddTimeoutWorkflowEvent`, `AddWorkflowExecutionCanceledEvent`, `AddWorkflowExecutionTerminatedEvent`, `AddWorkflowExecutionContinuedAsNewEvent`) which all call `UpdateWorkflowStateStatus(..._COMPLETED, ...)` after `checkMutability`.

Loop bounds exist at three distinct layers:

1. **User-facing workflow / activity retry policy** — `RetryPolicy.MaximumAttempts` is a per-namespace default in `common/dynamicconfig/constants.go:2513-2524` (`DefaultActivityRetryPolicy` / `DefaultWorkflowRetryPolicy` both default to `retrypolicy.DefaultDefaultRetrySettings` with `MaximumAttempts: 0` meaning "no cap", see `common/retrypolicy/retry_policy.go:32-37`). The policy is interpreted in `service/history/workflow/retry.go:96-110` (`nextBackoffInterval`) which returns `enumspb.RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED` or `RETRY_STATE_TIMEOUT` when the cap is hit.
2. **Workflow task scheduler** — `service/history/workflow/workflow_task_state_machine.go:47-50` defines `workflowTaskRetryBackoffMinAttempts = 3`, `workflowTaskRetryInitialInterval = 5s`, and computes a backoff in `getStartToCloseTimeout` (`workflow_task_state_machine.go:1428-1452`).
3. **Internal server task executors and replication** — bounded by `resubmitMaxAttempts = 10` (`service/history/queues/executable.go:96`), `resourceExhaustedResubmitMaxAttempts = 1` (`service/history/queues/executable.go:99`), `taskCriticalLogMetricAttempts = 30` (`service/history/queues/executable.go:102`), `MarkPoisonPillMaxAttempts = 3` (`service/history/replication/executable_task_tracker.go:18`), `workerCommandsMaxTaskAttempt = 3` (`service/history/worker_commands_task_dispatcher.go:29`).

Background event loops (queue reader, scheduled queue, ownership acquire loop) are **only** bounded by `shutdownCh` / `ctx.Done()`. There is no per-iteration cap, no stuck-loop detector, no repetition check — the only termination guarantee is the `Stop()` path that closes the shutdown channel and `AwaitWaitGroup`s for up to 1 minute (`service/history/queues/queue_immediate.go:107-122`, `service/history/queues/queue_scheduled.go:141-157`).

The system distinguishes success from exhaustion through the `enumspb.RetryState` enum (`RETRY_STATE_IN_PROGRESS`, `RETRY_STATE_NON_RETRYABLE_FAILURE`, `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED`, `RETRY_STATE_TIMEOUT`, `RETRY_STATE_CANCEL_REQUESTED`, `RETRY_STATE_INTERNAL_SERVER_ERROR`), persisted in `WorkflowExecutionFailedEventAttributes.RetryState` and `WorkflowExecutionTimedOutEventAttributes.RetryState` so callers can tell the two apart.

## Rating

**7 / 10** — Clear, mature termination model for user workflows (six distinct terminal events, persisted `RetryState` semantics, per-namespace configurable caps, dynamic configuration, well-tested at unit and integration levels). Distinct exhaustion reason codes are emitted and persisted. Several gaps prevent a higher score:

- Workflow task retry is **not bounded by a hard maximum attempt count**; the timeout scales `defaultTimeout + ExponentialRetryPolicy(maxInterval=WorkflowTaskRetryMaxInterval)` with `WithExpirationInterval(backoff.NoInterval)` (i.e., no expiration) at `service/history/workflow/workflow_task_state_machine.go:1447-1451`, so a chronically broken workflow task will keep being scheduled forever.
- The `failUpdateWorkflowTaskAttemptCount = 3` and `failQueryWorkflowTaskAttemptCount = 3` fail-fast thresholds at `service/history/api/updateworkflow/api.go:34` and `service/history/api/queryworkflow/api.go:31` are hard-coded constants, not configurable.
- Background event loops (queue readers, ownership, task executors) have **no stuck-loop detection** — they exit only on shutdown.
- `defaultMaximumAttempts = noMaximumAttempts = 0` for `ExponentialRetryPolicy` at `common/backoff/retrypolicy.go:22` means that any caller that builds a retry policy without explicitly setting a max will retry "forever" (only an expiration interval caps it).
- Forced termination on history/mutable-state size limits is effective but unconfigurable beyond the integer dynamic-config keys; there is no in-loop warning before termination that the user could observe.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Six terminal events (workflow close) | `AddCompletedWorkflowEvent`, `AddFailWorkflowEvent`, `AddTimeoutWorkflowEvent`, `AddWorkflowExecutionCanceledEvent`, `AddWorkflowExecutionTerminatedEvent`, `AddWorkflowExecutionContinuedAsNewEvent` | `service/history/workflow/mutable_state_impl.go:4834` ; `service/history/workflow/mutable_state_impl.go:4877` ; `service/history/workflow/mutable_state_impl.go:4928` ; `service/history/workflow/mutable_state_impl.go:5013` ; `service/history/workflow/mutable_state_impl.go:5542` |
| State machine close → `WORKFLOW_EXECUTION_STATE_COMPLETED` | `UpdateWorkflowStateStatus(WORKFLOW_EXECUTION_STATE_COMPLETED, ...)` after each close event | `service/history/workflow/mutable_state_impl.go:4863-4867` ; `service/history/workflow/mutable_state_impl.go:4907-4911` ; `service/history/workflow/mutable_state_impl.go:4956-4960` |
| Workflow helper wrappers for timeout/terminate | `TimeoutWorkflow` and `TerminateWorkflow` fail the in-flight workflow task first with `WORKFLOW_TASK_FAILED_CAUSE_FORCE_CLOSE_COMMAND` | `service/history/workflow/util.go:72-125` |
| Server-side force terminate on hard limits | `enforceHistorySizeCheck`, `enforceHistoryCountCheck`, `enforceMutableStateSizeCheck`, `forceTerminateWorkflow` | `service/history/workflow/context.go:1042-1199` |
| Workflow retry policy evaluation | `getBackoffInterval` / `nextBackoffInterval` returns `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED` when `maxAttempts > 0 && currentAttempt >= maxAttempts` | `service/history/workflow/retry.go:70-113` |
| `RetryState` enum (exhaustion reasons) | `RETRY_STATE_NON_RETRYABLE_FAILURE`, `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED`, `RETRY_STATE_TIMEOUT`, `RETRY_STATE_CANCEL_REQUESTED`, `RETRY_STATE_INTERNAL_SERVER_ERROR`, `RETRY_STATE_IN_PROGRESS` | `service/history/workflow/retry.go:45` ; `service/history/workflow/retry.go:97` ; `service/history/workflow/retry.go:102` ; `service/history/workflow/retry.go:110` ; `service/history/workflow/mutable_state_impl.go:6766-6853` |
| Activity retry state | `RetryActivity` populates `RETRY_STATE_TIMEOUT`/`CANCEL_REQUESTED`/`NON_RETRYABLE_FAILURE`/`INTERNAL_SERVER_ERROR` | `service/history/workflow/mutable_state_impl.go:6766-6853` |
| Default retry settings | `DefaultDefaultRetrySettings{InitialInterval: 1s, MaximumIntervalCoefficient: 100, BackoffCoefficient: 2, MaximumAttempts: 0}` | `common/retrypolicy/retry_policy.go:32-37` |
| Per-namespace default policy keys | `DefaultActivityRetryPolicy` and `DefaultWorkflowRetryPolicy` typed settings | `common/dynamicconfig/constants.go:2513-2524` |
| Suggest ContinueAsNew from history size | `HistorySizeSuggestContinueAsNew = 4*1024*1024` (4 MiB) | `common/dynamicconfig/constants.go:390-395` |
| Suggest ContinueAsNew from history count | `HistoryCountSuggestContinueAsNew = 4*1024` events | `common/dynamicconfig/constants.go:432-437` |
| Suggest logic | `getHistorySizeInfo` returns `SUGGEST_CONTINUE_AS_NEW_REASON_HISTORY_SIZE_TOO_LARGE` / `_TOO_MANY_HISTORY_EVENTS` | `service/history/workflow/workflow_task_state_machine.go:1454-1478` |
| Suggest from update saturation | `SuggestContinueAsNew` returns true when `inFlightCount() + completedCount >= ceil(maxTotal * threshold)` | `service/history/workflow/update/registry.go:496-506` |
| Update saturation config | `WorkflowExecutionMaxTotalUpdates`, `WorkflowExecutionMaxTotalUpdatesSuggestContinueAsNewThreshold`, `WorkflowExecutionMaxInFlightUpdatePayloads` | `common/dynamicconfig/constants.go:2414-2427` |
| Hard terminate on history-size error | `HistorySizeLimitError = 50*1024*1024` | `common/dynamicconfig/constants.go:380-383` |
| Hard terminate on history-count error | `HistoryCountLimitError = 50*1024` | `common/dynamicconfig/constants.go:396-400` |
| Hard terminate on mutable-state size | `MutableStateSizeLimitError = 8*1024*1024` | `common/dynamicconfig/constants.go:417-420` |
| History size / count warning | `HistorySizeLimitWarn = 10*1024*1024`, `HistoryCountLimitWarn = 10*1024` | `common/dynamicconfig/constants.go:385-388` ; `common/dynamicconfig/constants.go:401-404` |
| `MutableStateTombstoneCountLimit = 16` | cap on tracked deleted sub state machines | `common/dynamicconfig/constants.go:427-431` |
| Workflow task retry constants | `workflowTaskRetryBackoffMinAttempts = 3`, `workflowTaskRetryInitialInterval = 5s` | `service/history/workflow/workflow_task_state_machine.go:46-50` |
| Workflow task critical attempts | `WorkflowTaskCriticalAttempts` (default 10) triggers a throttled critical log | `service/history/workflow/workflow_task_state_machine.go:1412-1425` |
| Workflow task retry timeout calc | `defaultTimeout + exponential(WorkflowTaskRetryMaxInterval)` with `WithExpirationInterval(NoInterval)` — no overall expiry | `service/history/workflow/workflow_task_state_machine.go:1447-1451` |
| Workflow task timeout for delete | `maxWorkflowTaskTimeoutToDelete = 120 * time.Second` | `service/history/workflow/workflow_task_state_machine.go:49` |
| Update / query fail-fast on stuck WFT | `failUpdateWorkflowTaskAttemptCount = 3`, `failQueryWorkflowTaskAttemptCount = 3` | `service/history/api/updateworkflow/api.go:32-35` ; `service/history/api/queryworkflow/api.go:30-31` |
| Update / query fail-fast checks | Return `serviceerror.NewWorkflowNotReady` when `WorkflowTaskAttempt >= N` | `service/history/api/updateworkflow/api.go:132-143` ; `service/history/api/queryworkflow/api.go:134-143` |
| Queue executable retry caps | `resubmitMaxAttempts = 10`, `resourceExhaustedResubmitMaxAttempts = 1`, `taskCriticalLogMetricAttempts = 30` | `service/history/queues/executable.go:91-103` |
| `shouldResubmitOnNack` early termination | Returns `false` when attempt > `resubmitMaxAttempts` | `service/history/queues/executable.go:769-794` |
| DLQ after unexpected attempts | When `unexpectedErrorAttempts >= maxUnexpectedErrorAttempts() && dlqEnabled()` → `ErrTerminalTaskFailure` | `service/history/queues/executable.go:587-595` |
| DLQ-or-drop after terminal error | `isUnexpectedNonRetryableError` sends to DLQ when `dlqInternalErrors()` true, else drops | `service/history/queues/executable.go:496-515` ; `service/history/queues/executable.go:571-585` |
| Replication poison-pill cap | `MarkPoisonPillMaxAttempts = 3` → `e.terminalFailureCause` set | `service/history/replication/executable_task_tracker.go:18` ; `service/history/replication/executable_task.go:896` |
| Replication task processor retry cap | `ReplicationTaskProcessorErrorRetryMaxAttempts = 80` (per-shard) | `common/dynamicconfig/constants.go:2667-2671` |
| Replication stream sender retry cap | `ReplicationStreamSenderErrorRetryMaxAttempts = 80` | `common/dynamicconfig/constants.go:2879-2883` |
| Replication executable retry cap | `ReplicationExecutableTaskErrorRetryMaxAttempts = 80` | `common/dynamicconfig/constants.go:2904-2908` |
| Replication event-loop retry cap | `ReplicationStreamEventLoopRetryMaxAttempts = 100` (0 = retry forever) | `common/dynamicconfig/constants.go:2794-2798` |
| Worker commands task max attempts | `workerCommandsMaxTaskAttempt = 3`, `workerCommandsTaskTimeout = 10s` | `service/history/worker_commands_task_dispatcher.go:27-29` |
| Worker commands drop on exhaustion | Emits `max_attempts_exceeded` metric and returns `nil` (drop, not error) | `service/history/worker_commands_task_dispatcher.go:83-92` |
| Task reader reschedule backoff | `taskRescheduleMaxInterval = 3m`, `taskNotReadyRescheduleMaxInterval = 3m`, `taskResourceExhaustedRescheduleMaxInterval = 5m`, `dependencyTaskNotCompletedRescheduleMaxInterval = 3m`, all with `WithExpirationInterval(NoInterval)` (no global cap) | `common/util.go:69-86` ; `common/util.go:224-255` |
| `completeTaskRetryMaxAttempts = 10` | Hard cap on completing task I/O; `completeTaskRetryInitialInterval = 100ms`, `MaxInterval = 1s` | `common/util.go:65-67` ; `common/util.go:218-222` |
| Immediate queue event loop (no per-iter cap) | `for { select { case <-shutdownCh: return ... } }` — exits only on shutdown | `service/history/queues/queue_immediate.go:132-161` |
| Immediate queue max poll interval | `backoff.Jitter(MaxPollInterval, MaxPollIntervalJitterCoefficient)` with `time.Minute` timeout for `AwaitWaitGroup` | `service/history/queues/queue_immediate.go:135-138` ; `service/history/queues/queue_immediate.go:117` |
| Scheduled queue event loop (no per-iter cap) | Same `for { select ... }` pattern with `shutdownCh` / `timerGate` / `lookAheadCh` / `checkpointTimer` / `alertCh` | `service/history/queues/queue_scheduled.go:175-200` |
| Scheduled queue `lookAheadRateLimitDelay = 3s` | Bounded look-ahead window to avoid runaway polling | `service/history/queues/queue_scheduled.go:40-41` |
| Scheduled queue `MaxPollInterval` jitter | Loading triggered every `MaxPollInterval + jitter` if no tasks found | `service/history/queues/queue_scheduled.go:237-278` |
| Shard ownership event loop | `for { select { case <-ctx.Done(): return; case <-acquireTicker.C: scheduleAcquire; case changedEvent: scheduleAcquire } }` | `service/history/shard/ownership.go:79-101` |
| Shard acquire loop | `for { select { case <-ctx.Done(): return; case <-acquireCh: controller.acquireShards(ctx) } }` | `service/history/shard/ownership.go:110-119` |
| Shard controller acquire concurrency | `concurrency = max(AcquireShardConcurrency, 1)` semaphore; `acquireShards` does `sem.Acquire(ctx, concurrency)` to wait | `service/history/shard/controller_impl.go:443-457` |
| Shard engine wait | `engineCtx, engineCancel := context.WithTimeout(ctx, 1*time.Second)` per shard | `service/history/shard/controller_impl.go:436-440` |
| Long-poll expiration (per task queue) | `LongPollExpirationInterval` with `returnEmptyTaskTimeBudget = 1s` reserved buffer | `service/matching/config.go:103` ; `service/matching/physical_task_queue_manager.go:41` |
| Long-poll deadline buffer | `contextutil.WithDeadlineBuffer(ctx, longPollInterval, returnEmptyTaskTimeBudget)` | `service/matching/matching_engine.go:2864-2874` |
| Poller unavailability check | `timeSinceLastPoll() > QueryPollerUnavailableWindow()` returns `errNoRecentPoller` | `service/matching/matcher.go:241-243` |
| Task queue idle unload | `MaxTaskQueueIdleTime = 5*time.Minute` (default) triggers `unloadCauseIdle` | `common/dynamicconfig/constants.go:1257-1262` ; `service/matching/physical_task_queue_manager.go:169-173` |
| `PollerHistoryTTL` | Per-TTQ TTL for poller record; recorded at `service/matching/config.go:78` and consumed at `service/matching/physical_task_queue_manager.go:167` | `service/matching/physical_task_queue_manager.go:167` |
| Forwarded poll min interval | `forwardedPollMinInterval = common.CriticalLongPollTimeout` (20s) | `service/matching/matching_engine.go:82` |
| Task validator offer timeout | `taskReaderOfferTimeout = 60 * time.Second` (pri matcher) | `service/matching/task_validation.go:19` |
| Workflow task min backoff after continue-as-new | `previousMutableState.ContinueAsNewMinBackoff(...)` — comment "enforce minimal interval between runs to prevent tight loop continue as new spin" | `service/history/workflow/retry.go:333` |
| Long-running scanner continue-as-new | `OverdueNextActionTimeWorkflow` / `StuckOpenWorkflow` / `UnknownStateWorkflow` always return `workflow.NewContinueAsNewError(...)`; activity `MaximumAttempts = 10` then `workflowRunTimeout = 366*24*h` | `service/worker/scanner/scheduleinvariants/workflows.go:43-105` |
| Worker migration force-replication termination | Returns `workflow.NewContinueAsNewError(...)` when done; `forceReplicationActivityRetryPolicy.MaximumAttempts` not set in snippet (default policy) | `service/worker/migration/force_replication_workflow.go:199` ; `service/worker/migration/force_replication_workflow.go:277` |
| Worker deletion-reclaim retry cap | `MaximumAttempts: 3` for reclaim-resources activities; test asserts `Times(10)` is `defaultMaximumAttemptsForUnitTest * defaultConcurrentDeleteExecutionsActivities` | `service/worker/deletenamespace/workflow.go:54` ; `service/worker/deletenamespace/reclaimresources/workflow_test.go:106` |
| Worker DLQ workflow retry cap | `MaximumAttempts: 10` for DLQ workflows | `service/worker/dlq/workflow.go:182-190` |
| Worker scanner schedules retry | `MaximumAttempts: 10` initial, `BackoffCoefficient: 1.0`, then continue-as-new | `service/worker/scanner/scheduleinvariants/workflows.go:43-47` |
| Scheduler migrate retry cap | `MaximumAttempts: 1` (no retry) | `service/worker/scheduler/workflow.go:1057` |
| Migration handover retry cap | `MaximumAttempts: 1` (no retry) | `service/worker/migration/handover_workflow.go:161` |
| Test that exercises max-attempts drop | `TestExecute_ExceedsMaxAttempts_DropsTask` exercises `workerCommandsMaxTaskAttempt+1` | `service/history/worker_commands_task_dispatcher_test.go:73-91` |
| Test that retries-to-DLQ | `TestExecute_SendToDLQAfterMaxAttempts`, `TestExecute_DontSendToDLQAfterMaxAttemptsDLQDisabled`, etc. | `service/history/queues/executable_test.go:509-660` |
| Test for poison-pill max | `TestMarkPoisonPill_MaxAttemptsReached` | `service/history/replication/executable_task_test.go:1089` |
| Test for retry backoff | `TestNextBackoffInterval` covers `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED` / `RETRY_STATE_TIMEOUT` / `RETRY_STATE_NON_RETRYABLE_FAILURE` | `service/history/workflow/retry_test.go:154-310` |
| Test for force-terminate on size | `tests/sizelimit_test.go:43-51` sets `HistoryCountLimitError=20`, `HistorySizeLimitError=50MB`, `MutableStateSizeLimitError=8MB` | `tests/sizelimit_test.go:43-51` |
| Test for suggest-CAN | `tests/transient_task_test.go:155` overrides `HistorySizeSuggestContinueAsNew=20*1024` to force a suggestion | `tests/transient_task_test.go:155` |

## Answers to Dimension Questions

1. **What stops the loop?**
   - **User workflows**: the workflow's decision task handler issues a `CompleteWorkflowExecution` / `FailWorkflowExecution` / `CancelWorkflowExecution` / `ContinueAsNewWorkflowExecution` command which `MutableStateImpl` validates via `checkMutability` (`service/history/workflow/mutable_state_impl.go:4840`) and persists via `Add*WorkflowEvent` (`mutable_state_impl.go:4834-5554`). A workflow run-timeout fires a `WorkflowExecutionTimedOut` event via the timer-queue executor (`service/history/timer_queue_active_task_executor.go:831`). External termination writes a `WorkflowExecutionTerminated` event via `TerminateWorkflow` (`service/history/workflow/util.go:95-125`).
   - **System-internal task loops**: the `resubmitMaxAttempts` cap (`service/history/queues/executable.go:96`) plus the `MaxUnexpectedErrorAttempts` dynamic config plus the DLQ enable flag determine when a task is terminally failed. Replication has its own `MarkPoisonPillMaxAttempts` (`service/history/replication/executable_task_tracker.go:18`) and per-stream max attempts (`common/dynamicconfig/constants.go:2667-2908`).
   - **Background event loops**: shutdown channel close only. No per-iteration cap.

2. **Are limits configurable?**
   - Workflow and activity `RetryPolicy.MaximumAttempts` are user-supplied per workflow/activity (`service/history/workflow/retry.go:32-42`). Defaults are namespace-scoped dynamic configs at `common/dynamicconfig/constants.go:2513-2524`.
   - History size / count / mutable state size limits are per-namespace dynamic-config integers (`common/dynamicconfig/constants.go:380-437`).
   - Continue-as-new suggestion is per-namespace and per-updates-occupancy (`common/dynamicconfig/constants.go:2414-2427`).
   - Update/query fail-fast counts are **hard-coded** constants (`failUpdateWorkflowTaskAttemptCount = 3` at `service/history/api/updateworkflow/api.go:34`, `failQueryWorkflowTaskAttemptCount = 3` at `service/history/api/queryworkflow/api.go:31`).
   - Replication retry caps are dynamic-config integers (`common/dynamicconfig/constants.go:2667-2908`).
   - The `resubmitMaxAttempts` / `resourceExhaustedResubmitMaxAttempts` / `taskCriticalLogMetricAttempts` for `executableImpl` are **hard-coded** (`service/history/queues/executable.go:96-102`).
   - `workerCommandsMaxTaskAttempt = 3` is **hard-coded** (`service/history/worker_commands_task_dispatcher.go:29`).

3. **Is exhaustion treated differently from success?**
   - Yes. The `enumspb.RetryState` enum encodes the distinction (`RETRY_STATE_IN_PROGRESS` vs `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED` vs `RETRY_STATE_TIMEOUT` etc.) and is persisted in `WorkflowExecutionFailedEventAttributes.RetryState` and `WorkflowExecutionTimedOutEventAttributes.RetryState`. Tests assert this at `tests/activity_test.go:1551` (`RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED`) and `tests/activity_test.go:109` (`RETRY_STATE_TIMEOUT`).
   - On the server side, the activity timer-queue executor rewrites `TIMEOUT_TYPE_SCHEDULE_TO_START` timeouts as `TIMEOUT_TYPE_SCHEDULE_TO_CLOSE` when retry cannot fit in the remaining schedule-to-close budget (`service/history/timer_queue_active_task_executor.go:310-319`), a deliberate exhaustion semantic.

4. **Are stuck loops detected before the hard limit?**
   - The workflow task scheduler does **not** detect a stuck workflow task before exhausting attempt backoff. It does emit a `WorkflowTaskAttempt` metric and a throttled critical-attempts warning via `WorkflowTaskCriticalAttempts` (`service/history/workflow/workflow_task_state_machine.go:1412-1425`).
   - On the inbound path, `service/history/api/updateworkflow/api.go:132-143` and `service/history/api/queryworkflow/api.go:134-143` short-circuit update/query when `WorkflowTaskAttempt >= 3`, which is a soft stuck-loop detection on the *caller* side.
   - The `forceTerminateWorkflow` path in `service/history/workflow/context.go:1042-1199` is invoked when history / mutable-state size exceeds a hard limit — a server-side backstop rather than a predictive detector.
   - The `ContinueAsNewMinBackoff` comment at `service/history/workflow/retry.go:333` ("enforce minimal interval between runs to prevent tight loop continue as new spin") is the only explicit anti-spin protection I found.
   - No repetition / hash-of-last-event detector exists; the engine relies on the per-attempt timeout, the user-supplied retry policy, and the size/limit backstops.

5. **Does the user get a useful final state?**
   - Yes. The user-visible terminal state is one of `Completed`, `Failed`, `TimedOut`, `Terminated`, `Canceled`, `ContinuedAsNew`, each persisted as a final history event with full details (failure, retry state, new run id, callbacks). The `RetryState` field in `Failed` / `TimedOut` events distinguishes `IN_PROGRESS` (will retry) from `MAXIMUM_ATTEMPTS_REACHED` / `TIMEOUT` / `NON_RETRYABLE_FAILURE` (will not).
   - The `WorkflowExecutionInfo` summary exposed via `DescribeWorkflowExecution` includes `CloseTime`, `Status`, `HistoryLength`, `ExecutionDuration` for callers to inspect post-hoc.
   - On force-terminate, the terminate event carries a `failureReason` of `common.FailureReasonHistorySizeExceedsLimit` / `FailureReasonHistoryCountExceedsLimit` / `FailureReasonMutableStateSizeExceedsLimit` so operators can identify the cause.

## Architectural Decisions

- **Event-sourced termination**: workflow state is derived from history events; closing a workflow is a single-event append (the `Add*WorkflowEvent` methods) followed by a state-status update to `WORKFLOW_EXECUTION_STATE_COMPLETED`. This makes the termination model crash-safe and reproducible from history replay.
- **Stateless "executor" loop over stateful task**: `executableImpl` (`service/history/queues/executable.go:106`) carries a per-task attempt counter, not a queue-level one. The reschedule/retry decision is made on each `HandleErr` invocation (`executable.go:520-598`).
- **Multi-tier termination**: a `serviceerror.workflowNotReady` fail-fast (`updateworkflow/api.go:142`) and an outer `forceTerminateWorkflow` (history-size error) act as nested safety nets around the per-attempt backoff.
- **Dynamic-config defaults over hard-coded caps**: the engine prefers per-namespace, per-shard, or per-TTQ dynamic-config keys over process-wide constants. The few hard-coded caps (`resubmitMaxAttempts`, `failUpdateWorkflowTaskAttemptCount`) look like deliberate "last line of defense" choices.
- **Continue-as-new as a soft termination**: when a workflow approaches size / count / update limits, the engine *suggests* (not enforces) `ContinueAsNew` via `SuggestContinueAsNew` on the workflow task started event (`workflow_task_state_machine.go:550-622`). The user's worker code is expected to call `workflow.NewContinueAsNewError(...)`. `forceTerminateWorkflow` is the hard backstop.
- **Background goroutines only exit on shutdown**: the queue reader/writer event loops, shard ownership loops, task executors are all `for { select { case <-shutdownCh/ctx.Done(): return; ... } }` patterns. There is no in-loop iteration cap or stuck-loop detector.

## Notable Patterns

- **`HandleErr` discriminated-return pattern**: `executable.HandleErr` returns `nil` (drop or success), `ErrTerminalTaskFailure` (write to DLQ), or the original error (re-enqueue). See `service/history/queues/executable.go:520-598`.
- **`MaybeTerminalTaskError` interface**: any error implementing `IsTerminalTaskError() bool` causes `isUnexpectedNonRetryableError` to immediately route the task to the DLQ (`executable.go:496-499`).
- **`enumspb.RetryState` as the contract**: one enum codifies the lifecycle of a workflow or activity retry — the only place where "exhausted" vs "will retry" is decided.
- **Per-category queue executors**: transfer / timer / visibility / outbound / archival / replication each have their own queue factory wired with its own `MaxPollInterval` (`configs/config.go:145-191,348-386`) and `MaxUnexpectedErrorAttempts` settings, with a single `executableImpl` shared across them.
- **Resource-exhausted backpressure**: `isExpectedRetryableError` converts `RESOURCE_EXHAUSTED_CAUSE_APS_LIMIT` / `BUSY_WORKFLOW` into typed retryable errors with separate metrics (`executable.go:445-494`).

## Tradeoffs

- **Crash safety vs. operational overhead**: the event-sourced termination model means every terminate / continue-as-new must produce an event, which is durable but expensive. Long-running forced terminations must wait for the persistence write to land before the close is observable.
- **Dynamic-config explosion**: the proliferation of `Max*` dynamic-config keys (e.g. 4 distinct `Replication*MaxAttempts` settings) means operators have many knobs but the documentation surface is large and the interaction effects are not always obvious.
- **No global stuck-loop detector**: relying on user-supplied retry policies and history-size limits means a misconfigured workflow can run "forever" (within run-timeout / workflow-execution-timeout). The engine does not recognize semantic stuck patterns.
- **Exhaustion attempts as a metric vs. a gate**: `taskCriticalLogMetricAttempts = 30` is observability, not a hard cap. Operators see the warning, but the task is not terminated because of it.
- **Continue-as-new is advisory**: the engine *suggests* but does not require; a user workflow that ignores `SuggestContinueAsNew` will continue accumulating history until a hard cap (50 MB / 50k events / 8 MB mutable state) forces termination.
- **DLQ enablement is opt-in per-task**: `dlqEnabled()` must return `true` for terminally-failed tasks to be preserved; otherwise the task is dropped silently (with a log message). This is a deliberate opt-in but increases the chance of accidental data loss.

## Failure Modes / Edge Cases

- **Workflow task spin**: if a worker repeatedly reports a transient failure (e.g. `panic` on every decision), the workflow task attempt counter increments (`service/history/workflow/mutable_state_impl.go:1034-1040`) and the next timeout scales up exponentially up to `WorkflowTaskRetryMaxInterval` with no expiry. The `WorkflowTaskCriticalAttempts` log at `workflow_task_state_machine.go:1418-1425` is the only signal.
- **Continue-as-new tight loop**: the `FirstWorkflowTaskBackoff` (named `ContinueAsNewMinBackoff` in the call site) is the only anti-spin protection for workflows that CaN in a tight loop (`service/history/workflow/retry.go:333`).
- **Update / query storms during a stuck WFT**: the `fail*WorkflowTaskAttemptCount = 3` cap short-circuits new updates / queries with `WorkflowNotReady`. The cap is not configurable.
- **Resource-exhaustion loop**: a workflow that exhausts APS repeatedly is throttled (`executable.go:460`) but not terminated. `resourceExhaustedResubmitMaxAttempts = 1` only governs the rescheduler path.
- **History-size enforcement timing**: `enforceHistorySizeCheck` is invoked from `UpdateWorkflowExecution` / `RecordWorkflowTaskStarted` flows (`service/history/workflow/context.go:1042-1078`); a workflow that does not receive any further task starts will not be force-terminated until the next call, allowing transient over-limit conditions to persist.
- **DLQ-disabled terminal errors**: if `dlqInternalErrors()` is false and an internal error is detected, the task is dropped (`executable.go:583-585`). The original error is logged but not re-enqueued, so the failure is silent at the workflow level.
- **Replication stream retry cap of 100 with `0 = retry forever`**: misconfiguration of `ReplicationStreamEventLoopRetryMaxAttempts` to 0 disables the cap entirely (`common/dynamicconfig/constants.go:2794-2798`).
- **Long-poll timeouts are not equal to task-validator timeout**: a poller may wait `LongPollExpirationInterval` then return empty while the offer / validation path has `taskReaderOfferTimeout = 60s` (`service/matching/task_validation.go:19`). The two are independent.
- **Watch workflow tight loop in scheduler activities**: `service/worker/scheduler/activities.go:204-218` retries on `errTryAgain` or `IsContextDeadlineExceededErr`. The loop is bounded only by `ctx.Done()`, which is the activity's `StartToCloseTimeout`.

## Future Considerations

- **Global cap on workflow task attempts**: a `WorkflowTaskMaxRetryAttempts` dynamic config could be added; today the only signal is `WorkflowTaskCriticalAttempts` which is observational.
- **Configurable fail-fast for update / query**: lifting `failUpdateWorkflowTaskAttemptCount` and `failQueryWorkflowTaskAttemptCount` to dynamic config would let operators tune for their own worker reliability characteristics.
- **Repetition / hash-of-last-event detector**: an optional stuck-loop detector on workflow task attempts (similar to the agent-framework's `length` finish-reason handling) would catch semantically stuck workflows before they hit history-size limits.
- **Per-namespace task-executable retry caps**: the hard-coded `resubmitMaxAttempts = 10` and `resourceExhaustedResubmitMaxAttempts = 1` could become dynamic-config to allow per-queue tuning.
- **Force-terminate advisory events**: when a workflow approaches the size / count / mutable-state limits, emit a `WorkflowExecutionOptionsChanged` or signal event so workers can react before termination.
- **Bounded `MaxTaskQueueIdleTime` for workflow-task queues vs activity queues**: today all task queues share `MaxTaskQueueIdleTime`; workflow tasks may want a separate (typically shorter) unload to fail-fast on missing workers.

## Questions / Gaps

- **Per-iteration cap on the queue reader / scheduled queue loops**: I found no evidence of a per-iteration budget on the background event loops. The only bound is `Stop()` waiting up to 1 minute via `AwaitWaitGroup` (queue_immediate.go:117). A pathological bug in `processNewRange` / `processPollTimer` could in theory loop forever.
- **Recursive task-graph termination**: I did not find a maximum nesting depth for child workflow chains or for ContinueAsNew chains. The `ReapplyContinueAsNewWorkflowEvents` (`service/history/ndc/workflow_resetter.go:645`) re-applies events across runs but is bounded by persistence reads, not a hard cap.
- **In-memory loops with no shutdown signal**: the `chasm/tree.go:1691-1722` `for len(n.immediatePureTasks) != 0` loop is bounded only by the task map emptying; a misbehaving pure-task generator that re-enqueues itself would loop forever. The comment at `chasm/tree.go:3308` mentions "infinite loop" risk, indicating the maintainers are aware.
- **Worker activity scanner / DLQ cap**: the scanner / DLQ workflows use SDK `MaximumAttempts` only, with `WorkflowRunTimeout` as a backstop. I did not find a `ContinueAsNewError` in `dlq/workflow.go` (only the 10-attempt retry).
- **Matching poller history retention**: `PollerHistoryTTL` is per-TTQ dynamic config; I did not find a hard cap on the number of pollers stored (only the TTL). A burst of unique pollers could grow the in-memory map unbounded within a TTL window.
