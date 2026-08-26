# Source Analysis: temporal

## Dimension 07.03: Idempotency and Retry Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server: frontend/history/matching/worker services, Cassandra/SQL persistence) |
| Analyzed | 2026-08-26 |

## Summary

Temporal treats retry as a first-class, durable, server-side state machine rather than an in-process loop around a tool call. There are three distinct retry layers:

1. **In-process utility retries** for transient infrastructure failures (`common/backoff/retry.go`), used by internal task schedulers and by generated "retryable client" wrappers on every inter-service gRPC call (`client/history/retryable_client.go`, `client/frontend/retryable_client.go`, `client/matching/retryable_client.go`). These are safe to retry because every mutating RPC is guarded by request-ID dedup and conditional (optimistic-concurrency) persistence writes.
2. **Activity retries** — the closest analogue to "tool retries" in an agent harness. A failed or timed-out activity attempt is classified against the user-supplied `RetryPolicy` (`common/retrypolicy/retry_policy.go`), and if retryable, the history service persists incremented attempt state and schedules a durable `ActivityRetryTimerTask` (`service/history/tasks/activity_retry_timer.go:14`) instead of looping. Duplicate dispatches of the same attempt are rejected via per-attempt stamps and started-event checks (`service/history/api/recordactivitytaskstarted/api.go:150-184`).
3. **Workflow-level retries** implemented as continue-as-new with an inherited attempt counter and backoff before the next run's first workflow task (`service/history/timer_queue_active_task_executor.go:685-726`, `service/history/workflow/retry.go:116-304`).

Idempotency keys pervade the system: start-request IDs persisted in the current-execution record and replayed to dedupe retried starts (`service/history/api/startworkflow/api.go:340-358`), activity scheduled-event IDs + stamps identifying individual attempts, update-IDs/request-IDs for workflow updates (`service/history/workflow/update/update.go:422-499`), and a generic resource-dedup set in mutable state (`service/history/workflow/mutable_state_impl.go:7683-7696`). Retry outcomes are observable: every terminal failure/timeout history event records a `RetryState`, attempts are visible in events and metrics, and workers receive heartbeat details across retries.

Because side effects live in worker code, the server cannot guarantee exactly-once execution of a non-idempotent activity; it guarantees at-least-once dispatch plus the primitives needed for user-side idempotency (attempt numbers, heartbeat details, deterministic workflow replay). The design explicitly acknowledges this boundary.

## Rating

**9 / 10** — Mature, durable, observable, and proven under failure/scale. Explicit error classification (`IsRetryableFailure`), persisted retry state (attempts, timers), multiple idempotency-key mechanisms, extensive tests (`service/history/workflow/mutable_state_impl_restart_activity_test.go`, `common/retrypolicy/retry_policy_test.go`, `tests/activity_parity_test.go`). Not a 10 because: the in-process throttle policy is hard-coded with open TODOs (`common/backoff/retry.go:49`), `RETRY_STATE_PAUSED` is not yet fully supported (`service/history/workflow/mutable_state_impl.go:6925-6927`), and `maxAttempts=0` semantics ("infinite") have an acknowledged TODO (`service/history/workflow/retry.go:29`).

## Evidence Collected

Every entry cites workspace-relative paths from the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic retry wrapper | `ThrottleRetryContext`: context cancellation never retried; `ResourceExhausted` switched to a separate throttle policy (1s→10s max, unexpired); deadline-aware | common/backoff/retry.go:46-102 |
| Throttle policy constants | Initial 1s, max 10s interval, no expiration | common/backoff/retry.go:15-25 |
| Inter-service retry wrapper | Generated retryable clients wrap every RPC method with `backoff.ThrottleRetryContext(ctx, op, c.policy, c.isRetryable)` | client/history/retryable_client_gen.go:25 |
| Retryable client factory | `NewRetryableClient(client, policy, isRetryable)` injected per service in FX wiring | client/history/retryable_client.go:20-26; common/resource/fx.go:292-350 |
| Client retry policies | Exponential policies with max-attempt caps; optional unbounded retry when system-scoped ResourceExhausted occurs (`backoff.NewConditionalRetryPolicy`) | common/util.go:163-209 |
| Server error classification | `IsServiceHandlerRetryableError`: Internal/Unavailable retryable; namespace-handover errors not; MultiOperation errors inspected recursively | common/util.go:356-374 |
| Transient-error classification | `IsServiceClientTransientError`: adds ShardOwnershipLost, StalePartitionCounts, system ResourceExhausted | common/util.go:338-354 |
| gRPC-code classification | `IsRetryableRPCError`: Canceled/Unknown/Unavailable/DeadlineExceeded/ResourceExhausted/Aborted/Internal retryable | common/util.go:766-791 |
| History API retryability | ErrStaleState, conflict errors, handler-retryable errors | service/history/api/retry_util.go:12-18 |
| Failure classification core | `IsRetryableFailure(failure, nonRetryableTypes)`: canceled/terminated → false; ScheduleToStart/ScheduleToClose timeouts → false; StartToClose/Heartbeat timeouts retryable unless `"TemporalTimeout:<Type>"` listed; application failure non-retryable flag or type match → false; unknown → true | common/retrypolicy/retry_policy.go:24-64 |
| Policy validation | nil policy = no retry; MaximumAttempts==1 short-circuits as disabled; BackoffCoefficient >= 1; MaximumInterval >= InitialInterval | common/retrypolicy/retry_policy.go:102-141 |
| Defaults | 1s initial interval, 100x max coefficient, 2.0 backoff, unlimited attempts | common/retrypolicy/retry_policy.go:76-100 |
| Activity retry decision | `MutableStateImpl.RetryActivity`: no policy → RETRY_STATE_RETRY_POLICY_NOT_SET; cancel requested → stop; schedule timeouts → RETRY_STATE_TIMEOUT (not non-retryable); else classify, compute backoff, persist attempt+1, generate retry timer task | service/history/workflow/mutable_state_impl.go:6868-6964 |
| Attempt/backoff computation | `nextBackoffInterval`: attempt counting includes initial attempt; expiration-time cutoff → RETRY_STATE_TIMEOUT; max-attempts cutoff → RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED | service/history/workflow/retry.go:69-112 |
| Worker-specified retry delay | Application failure's `NextRetryDelay` overrides exponential backoff (`backoff.MakeBackoffAlgorithm(delayedRetryDuration)`) | service/history/workflow/retry.go:47-52; common/backoff/retry.go:186-195 |
| Durable retry timer task | `ActivityRetryTimerTask{Attempt, Stamp}` persisted as timer-category task; type TASK_TYPE_ACTIVITY_RETRY_TIMER | service/history/tasks/activity_retry_timer.go:13-59; service/history/workflow/task_generator.go:572-583 |
| Timeout-driven retry guard | Timer executor skips stale attempts: `if timerSequenceID.Attempt < ai.Attempt { return }`; comment notes same-attempt calls are idempotent | service/history/timer_queue_active_task_executor.go:297-307 |
| Timeout→retry resolution | On exhausted retries, timeout converted to ScheduleToClose failure so workflow sees final outcome; RetryState recorded in timed-out event | service/history/timer_queue_active_task_executor.go:312-360 |
| CHASM standalone activity retry | `tryReschedule`/`shouldRetry`: max attempts check, schedule-to-close budget check (`hasEnoughTimeForRetry`), pause-aware transitions, worker-override retry-interval source | chasm/lib/activity/activity.go:1384-1473 |
| Workflow retry (continue-as-new) | Run-timeout task computes `GetRetryBackoffDuration`; if retryable → new run with `CONTINUE_AS_NEW_INITIATOR_RETRY`, new UUID run ID | service/history/timer_queue_active_task_executor.go:680-726 |
| Workflow attempt counter | `SetupNewWorkflowForRetryOrCron` sets `Attempt = previous.Attempt + 1`, propagates RetryPolicy, applies minimal inter-run backoff to prevent spin loops | service/history/workflow/retry.go:245-304 |
| Workflow-level classification | `GetRetryBackoffDuration` reads ExecutionInfo.HasRetryPolicy + stored policy fields | service/history/workflow/mutable_state_impl.go:1767-1788 |
| Start-request idempotency key | Retried start with same RequestId found in `CurrentWorkflowConditionFailedError.RequestIDs` → returns prior result, outcome `StartDeduped`, metric recorded | service/history/api/startworkflow/api.go:321-359 |
| Dedup metadata source | RequestIDs map (create + attached request IDs) persisted inside current-workflow record blob | common/persistence/data_interfaces.go:113-129; common/persistence/serialization/serializer.go:463-467 |
| Conditional persistence writes | CreateWorkflowModeBrandNew fails if current record exists; CurrentWorkflowConditionFailedError/WorkflowConditionFailedError/ConditionFailedError model optimistic concurrency | common/persistence/data_interfaces.go:41-71,113-139 |
| Conflict handling on start | `CurrentWorkflowConditionFailedError` triggers conflict-resolution path (dedup vs reuse vs terminate-existing) instead of blind retry | service/history/api/startworkflow/api.go:206-221 |
| ID reuse/conflict policies | ResolveDuplicateWorkflowID: running → conflict policy (FAIL/USE_EXISTING/TERMINATE_EXISTING); completed → reuse policy (ALLOW_DUPLICATE[_FAILED_ONLY]/REJECT_DUPLICATE); plus minimal-reuse-interval rate limit returning BUSY_WORKFLOW ResourceExhausted | service/history/api/workflow_id_dedup.go:32-205 |
| Activity start dedup | If StartedEventId already set: same request ID → idempotent success returning original StartedTime/Attempt/Clock; different request ID → `TaskAlreadyStarted` (gRPC AlreadyExists) | service/history/api/recordactivitytaskstarted/api.go:150-171 |
| Obsolete-task rejection | Stamp mismatch → `ObsoleteMatchingTask` error; matching drops task; also NotFound drop path for already-completed activities | service/history/api/recordactivitytaskstarted/api.go:131-137,178-184 |
| Generic resource dedup store | `IsResourceDuplicated`/`UpdateDuplicatedResource` keyed by `DeduplicationID` (e.g., EventReappliedID = runID::eventID::version) used for re-applied events during reset/replication | service/history/workflow/mutable_state_impl.go:7683-7696; common/definition/resource_dedup.go:27-66; service/history/ndc/workflow_resetter.go:1136-1139 |
| Update callback dedup | Update attach-callbacks dedupes by requestID against durably recorded event attributes | service/history/workflow/update/update.go:422-499 |
| Cancel/pause request dedup (CHASM) | Cancellation replays after terminal state return success if same request ID; pause replays cannot re-pause after later unpause | chasm/lib/activity/activity.go:931-951,983-992 |
| Side-effect resume support | LastHeartbeatDetails saved into activity progress on failure response; returned to worker on next attempt start | service/history/api/respondactivitytaskfailed/api.go:87-95; service/history/api/recordactivitytaskstarted/api.go:288 |
| Heartbeat progress API | Heartbeats persist progress + last-HB time via UpdateActivityProgress; respond with cancel/pause/reset flags | service/history/api/recordactivitytaskheartbeat/api.go:77-101 |
| Retry visibility (history) | RetryState recorded on ActivityTaskFailed, ActivityTaskTimedOut, WorkflowExecutionFailed, WorkflowExecutionTimedOut events; NewExecutionRunId links retried runs | service/history/historybuilder/event_factory.go:300-379 |
| Retry visibility (metrics) | Per-attempt completion metrics (attempt start time, closed flag, status); timeout metric tagged with ai.Attempt; StartWorkflowRequestDeduped counter | service/history/api/respondactivitytaskfailed/api.go:130-146; service/history/timer_queue_active_task_executor.go:323-354; service/history/api/startworkflow/api.go:341 |
| Shard acquisition retry | Infinite retry except ShardOwnershipLostError/lifecycle end; ownership loss stops shard | service/history/shard/context_impl.go:2118-2148 |
| Tests: classification | Table-driven tests for IsRetryableFailure and backoff states incl. MAXIMUM_ATTEMPTS_REACHED | common/retrypolicy/retry_policy_test.go; service/history/workflow/retry_test.go:149 |
| Tests: activity retry behavior | Restart-activity suite covers paused retries, stamp increments, schedule-to-start/close timeout states, task-generation failures | service/history/workflow/mutable_state_impl_restart_activity_test.go:157-357 |
| Tests: duplicate/dedup paths | TestStartWorkflowExecution_Dedup_* family; RecordActivityTaskStarted duplicate-request clock tests; heartbeat-timer dedup under time-skipping | service/history/history_engine2_test.go:1590-1740; service/history/api/recordactivitytaskstarted/api_test.go:344-366; service/history/timer_queue_active_task_executor_test.go:2645-2734 |

## Answers to Dimension Questions

### 1. Which tool failures are retried?

Three layers with distinct rules:

- **User-defined activity retries**: governed by the workflow-supplied `RetryPolicy`. Application failures are retried unless marked non-retryable by the worker (`ApplicationFailureInfo.NonRetryable`, common/retrypolicy/retry_policy.go:53-56) or matching `NonRetryableErrorTypes` (including `"TemporalTimeout:StartToClose"`-style entries, common/retrypolicy/retry_policy.go:36-44). Cancellation/termination never retry (common/retrypolicy/retry_policy.go:32-34). Schedule-to-start/close timeouts terminate the activity with RETRY_STATE_TIMEOUT rather than being treated as non-retryable application errors (service/history/workflow/mutable_state_impl.go:6893-6899).
- **Internal infrastructure calls**: only transient errors — Internal/Unavailable service errors, shard-ownership loss (for re-routing), stale mutable state, and persistence conflicts are retried (common/util.go:338-374, service/history/api/retry_util.go:12-18). `ResourceExhausted` gets a dedicated slower throttle track (common/backoff/retry.go:81-83).
- **Workflow-level**: whole workflows restart via continue-as-new when a run times out and a workflow RetryPolicy permits (service/history/timer_queue_active_task_executor.go:691-700).

### 2. Are repeated attempts safe?

Yes at the orchestration layer, by construction:
- Each activity attempt has an identity (scheduledEventID + Attempt + Stamp). A late `RecordActivityTaskStarted` for an already-started attempt is answered idempotently for the same request ID and rejected with `TaskAlreadyStarted` otherwise (service/history/api/recordactivitytaskstarted/api.go:150-171).
- Timer executors skip timeout processing for superseded attempts (`timerSequenceID.Attempt < ai.Attempt`, service/history/timer_queue_active_task_executor.go:297-300) and note that same-attempt processing is idempotent (:302-303).
- All state mutations flow through conditional persistence writes keyed on next-event-ID/record-version (common/persistence/data_interfaces.go:54-84), so a retried RPC either lands once or surfaces a typed conflict that callers resolve rather than blindly reapply.
- The residual risk is deliberately pushed to user code: an activity whose worker crashes after performing an external side effect but before reporting will be re-dispatched (at-least-once). Temporal mitigates but does not eliminate this: attempt counts and heartbeat details are provided so users can implement guards (see Q5).

### 3. Is retry state persisted?

Yes, comprehensively:
- Per-activity: `ActivityInfo` stores `Attempt`, full retry-policy fields (`RetryInitialInterval`, `RetryMaximumAttempts`, `RetryBackoffCoefficient`, `RetryNonRetryableErrorTypes`, `RetryExpirationTime`), `Stamp`, and `RetryLastFailure` (mutated in service/history/workflow/mutable_state_impl.go:6911-6928, 6952-6955). Retry scheduling itself is a persisted `ActivityRetryTimerTask` in the timer queue (service/history/tasks/activity_retry_timer.go:14-22, generated at service/history/workflow/task_generator.go:572-583) — surviving process restarts.
- Per-workflow: `ExecutionInfo.Attempt` plus retry policy survive continue-as-new (service/history/workflow/retry.go:269-272).
- Start dedup state: create/attached request IDs persisted in the current-execution blob (common/persistence/serialization/serializer.go:463-467).

### 4. Are non-idempotent tools protected?

The server protects its own operations and *detects* duplicate work, but cannot make arbitrary user side effects atomic. Protections observed:
- Duplicate-start protection via WorkflowIdReusePolicy/ConflictPolicy with explicit FAIL/USE_EXISTING/TERMINATE_EXISTING semantics and a busy-workflow rate limit (service/history/api/workflow_id_dedup.go:98-205).
- Per-attempt stamping prevents two workers running the same activity attempt (stamp mismatch rejection, service/history/api/recordactivitytaskstarted/api.go:178-184) and prevents stale tasks from executing after pause/reset.
- For external effects, the intended pattern is user-managed idempotency supported by server-provided keys: activity ID, attempt number, and heartbeat details round-trip through retries (`response.HeartbeatDetails = ai.LastHeartbeatDetails`, service/history/api/recordactivitytaskstarted/api.go:288). The CHASM standalone-activity APIs even expose final `RetryState` outcomes for programmatic inspection (tests/activity_standalone_test.go:3611).

### 5. Can retries create duplicate side effects?

Yes — for user code, by design: dispatch is at-least-once. If a worker executes a payment call and dies before responding, the activity will be retried and the payment can execute twice unless the user keys off attempt/heartbeat data. The server's own side effects (history writes, task creation, visibility updates) cannot be duplicated thanks to conditional writes + request-ID dedup + event-ID monotonicity. Notably, the codebase documents the exact hazard class in comments: delayed replays of old Pause requests would "silently re-pause" resumed activities without request-ID dedup (chasm/lib/activity/activity.go:986-992), and cancellation replays after terminal transition must dedupe first (:938-941) — evidence the team actively closes duplicate-side-effect windows as features mature.

## Architectural Decisions

1. **Durable timers over in-process retry loops for user-facing retries.** Activity retries become persisted `ActivityRetryTimerTask`s executed by the timer queue (service/history/workflow/task_generator.go:572-583), making retry state crash-proof and cluster-wide, unlike `backoff.ThrottleRetry` which is reserved for infra-internal calls.
2. **Centralized, declarative failure classification.** One function (`retrypolicy.IsRetryableFailure`, common/retrypolicy/retry_policy.go:27-64) decides retryability from the failure proto, shared by both the legacy mutable-state path (mutable_state_impl.go:6901) and workflow-level retries (workflow/retry.go:43), keeping SDK-visible semantics uniform.
3. **Optimistic concurrency + typed conflicts instead of distributed locks.** Persistence returns structured condition-failure errors carrying enough metadata (RequestIDs, RunID, State, Status) for callers to implement semantic conflict resolution (service/history/api/startworkflow/api.go:208-219, common/persistence/data_interfaces.go:113-129).
4. **Retry as new-run (continue-as-new) at workflow scope.** Workflow retries allocate fresh run IDs while threading attempt counters and completion callbacks forward (service/history/workflow/retry.go:116-304), bounding single-history growth even with infinite retries.
5. **Worker-in-the-loop retry tuning.** Workers may override the next retry delay via `NextRetryDelay` on application failures (service/history/workflow/retry.go:47-51; CHASM marks the source `ACTIVITY_RETRY_INTERVAL_SOURCE_WORKER_OVERRIDE`, chasm/lib/activity/activity.go:1415-1418), acknowledging that applications know best when their failed side effect should be retried.

## Notable Patterns

- **Dual-track backoff**: normal exponential policy plus an always-on slower "throttle" policy engaged specifically for `ResourceExhausted` errors, preventing retry storms against overloaded dependencies (common/backoff/retry.go:15-25, 81-83).
- **Conditional retry policies**: `backoff.NewConditionalRetryPolicy(predicate, extended, capped)` lets specific error classes (system-scoped ResourceExhausted) escape attempt caps (common/util.go:192-202).
- **Idempotent-read-modify-write handlers**: every history API handler follows the `GetAndUpdateWorkflowWithNew` pattern — load mutable state under lease, validate staleness (`ErrStaleState` → reload), mutate, commit conditionally (service/history/api/respondactivitytaskfailed/api.go:50-129).
- **Generation/stamp fencing**: monotonically increasing `Stamp` fields fence obsolete tasks after reset/pause/reschedule, mirroring how sequence numbers fence replication (service/history/api/recordactivitytaskstarted/api.go:178-184).
- **RetryState as an observability contract**: the enum (`IN_PROGRESS`, `NON_RETRYABLE_FAILURE`, `MAXIMUM_ATTEMPTS_REACHED`, `TIMEOUT`, `CANCEL_REQUESTED`, `RETRY_POLICY_NOT_SET`) is written into terminal history events so downstream consumers (UI, SDKs, other workflows) can distinguish "gave up" reasons without re-deriving them (service/history/historybuilder/event_factory.go:300-379).

## Tradeoffs

- **Durability vs latency**: routing each retry through persisted timer tasks adds a persistence write + timer-queue round trip per retry, versus a cheap sleep loop. Chosen for crash-safety and auditability; cost is acceptable since retries are usually backoff-spaced anyway.
- **At-least-once vs exactly-once**: the event-sourced design guarantees exactly-once *state* transitions but only at-least-once *effect* execution. Simpler and scalable, but shifts idempotency burden for non-idempotent tools onto users.
- **Centralized classification vs extensibility**: one shared `IsRetryableFailure` keeps semantics consistent but means exotic per-tool retry predicates (e.g., "retry on HTTP 429 only") require encoding into NonRetryableErrorTypes strings or application-failure flags rather than custom logic.
- **Request-ID dedup depth**: dedup metadata lives in the current-record blob; records predating fields like `FirstExecutionRunID` need fallback loading (service/history/api/startworkflow/api.go:329-337) — backward compatibility costs extra reads.
- **Unbounded defaults**: default MaximumAttempts is unlimited (common/retrypolicy/retry_policy.go:80), relying on schedule-to-close/expiration timeouts as circuit breakers; misconfigured activities can retry indefinitely until deadlines.

## Failure Modes / Edge Cases

- **Crash between effect and report**: activity side effect applied, worker dies → attempt retried → duplicate external effect (inherent at-least-once gap; mitigated by heartbeat-details pattern).
- **Race: timeout fires vs failure reported concurrently**: fenced by attempt comparison (timer skips if `Attempt < ai.Attempt`, service/history/timer_queue_active_task_executor.go:297-300) and vector-clock staleness checks that force cache refresh (`ErrStaleState`).
- **Duplicate dispatch under time-skipping**: heartbeat-timer dedup logic had to be fixed for virtual-time skips (test `TestProcessActivityTimeout_Heartbeat_DedupUnderSkip`, service/history/timer_queue_active_task_executor_test.go:2645-2734).
- **Zombie/transition states**: activity start rejected with `ActivityStartDuringTransition` so matching drops and reschedules post-transition (service/history/api/recordactivitytaskstarted/api.go:88-92, 198-202).
- **Incomplete feature surface**: `RETRY_STATE_PAUSED` is defined but unsupported, left behind TODOs in both the retry decision and the caller gate (service/history/workflow/mutable_state_impl.go:6925-6927; service/history/api/respondactivitytaskfailed/api.go:109-111).
- **Known rough edges**: `TODO treat 0 as 0, not infinite` for maximum attempts (service/history/workflow/retry.go:29); throttle policy customization TODO (common/backoff/retry.go:49).
- **Tight-loop protection dependency**: workflow retries/crons rely on `ContinueAsNewMinBackoff` to prevent spin loops (service/history/workflow/retry.go:292-293); removing it would allow rapid-fire run creation.

## Future Considerations

- Complete `RETRY_STATE_PAUSED` support so paused-activity retries are externally distinguishable from in-progress ones (TODOs cited above).
- Make the throttle-retry policy and its error categorization configurable (common/backoff/retry.go:49) instead of hard-coded 1s/10s.
- Extend the generic `DeduplicationID` registry (currently only `EventReappliedID`, common/definition/resource_dedup.go:60-65) toward a uniform dedup abstraction for all re-applied/resent resources.
- Continue migrating ad-hoc mutable-state retry logic onto the CHASM activity component, which already carries richer retry metadata (`ACTIVITY_RETRY_INTERVAL_SOURCE_*`, per-attempt records) — parity is enforced by dedicated suites (tests/activity_parity_test.go:719-827).

## Questions / Gaps

- No evidence found in this source of an automatic, server-side idempotency-key injection into user tool payloads (e.g., auto-generated payment keys). Searched `common/activityoptions`, retry-policy files, and CHASM activity handlers; side-effect idempotency is consistently delegated to users via documented primitives (attempt numbers, heartbeats). This is a stated architectural boundary rather than a defect.
- Nexus operation retries were out of the deep-dive scope here; initial greps show parallel dedup machinery exists (`chasm/lib/nexusoperation/handler.go:55-56` maps `ExecutionAlreadyStartedError` to a typed service error), suggesting equivalent semantics, but I did not fully trace nexus retry classification.
- The SDK-side half (local activities, client-side retry of `RespondActivityTask*` calls) lives in the separate `go.temporal.io/sdk` module, not this repository; claims about user-facing retry ergonomics are therefore inferred from server contracts only.

---

Generated by dimension `07.03-idempotency-and-retry-semantics` against `temporal`.
