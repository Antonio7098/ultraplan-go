# Source Analysis: temporal

## Dimension 07.08: Tool Failure Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (distributed durable-execution server); gRPC + protobuf APIs; embedded SDK-style workers for internal workflows |
| Analyzed | 2026-08-25 |

> Citation convention: all `file:line` paths below are relative to the source root
> `studies/agent-harness-study/sources/temporal/` (the repo under study). The "tool call" analog in
> Temporal is an activity invocation (also: workflow task, Nexus operation) dispatched to a worker.

## Summary

Temporal treats tool failure escalation as a first-class, four-audience problem with a single wire
envelope. Every tool failure — worker-reported activity error, server-enforced timeout,
workflow-task (determinism) failure, or Nexus operation termination — is captured in a structured
`failurepb.Failure` envelope built by constructors in `common/failure/failure.go:14-46`, persisted
as a history event together with an explicit `RetryState` (`service/history/workflow/mutable_state_impl.go:4627-4666`),
and then routed by one central decision point:

1. **Retryable?** A pure classifier `retrypolicy.IsRetryableFailure` (`common/retrypolicy/retry_policy.go:27-64`)
   combined with exponential/custom backoff math in `service/history/workflow/retry.go:31-112`
   either reschedules the activity or declares exhaustion with a machine-readable reason
   (`RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED`, `RETRY_STATE_TIMEOUT`, `RETRY_STATE_NON_RETRYABLE_FAILURE`).
2. **Terminal?** Exhaustion records an `ActivityTaskFailed/TimedOut` event carrying the full failure
   and retry state into workflow history (`service/history/api/respondactivitytaskfailed/api.go:105-121`),
   which hands control back to the calling workflow ("model") as a catchable error rather than a dead end.
3. **User-facing?** Service-side faults surface as typed `serviceerror` values transported over gRPC
   status + proto details (`common/serviceerror/convert.go:11-57`), optionally masked behind a
   correlation hash (`common/rpc/interceptor/mask_internal_error.go:67-87`) and length-truncated
   (`common/rpc/interceptor/service_error_interceptor.go:44-51`).
4. **Operator-facing?** Failures are classified expected/unexpected, grouped by type/cause tags, and
   emitted as metrics and logs (`common/rpc/interceptor/request_error_handler.go:56-116`),
   with dedicated counters separating per-attempt failures from terminal failures
   (`common/metrics/metric_defs.go:980-990`) and DLQ-bound terminal task failures
   (`common/metrics/metric_defs.go:894-905`). Human escalation is explicit but manual: DLQ
   delete/merge workflows (`service/worker/dlq/workflow.go:1-60`), admin DLQ APIs
   (`service/history/api/getdlqtasks/api.go:19`, `service/history/api/deletedlqtasks/api.go:13`),
   and operator intervention APIs like PauseActivity / UnpauseActivity
   (`service/frontend/workflow_handler.go:7166-7228`).

Size limits protect the system from oversized failures without losing the diagnosis trail:
oversized failures are wrapped as `ServerFailure{NonRetryable}` whose `Cause` holds the truncated
original (`service/history/workflow/mutable_state_impl.go:7217-7238`), mirrored in the new CHASM
activity implementation (`chasm/lib/activity/activity.go:1368-1382`).

The answer to the dimension's guiding question — *does a failed tool become a recovery path or a
dead end?* — is emphatically "recovery path": retries, pause/reset, custom `NextRetryDelay`, and
catchable history events give both the model and the operator multiple exits.

## Rating

**Score: 9/10**

Rationale against the rubric:

- **Clear model**: one typed envelope (`failurepb.Failure`) with discriminated info types
  (Application/Timeout/Canceled/Terminated/Server/Reset) and an explicit `RetryState` enum that
  records *why* retries stopped (`service/history/workflow/retry.go:43-111`).
- **Tests**: unit tests cover the retryability classifier (`common/retrypolicy/retry_policy_test.go:15`),
  backoff math (`service/history/workflow/retry_test.go:17,62`), truncation incl. depth limits
  (`common/failure/failure_test.go:10-59`), masking (`common/rpc/interceptor/mask_internal_error_test.go`),
  and engine-level failure handling across invalid tokens/stale state/conflicts/success
  (`service/history/history_engine_test.go:3160-3700`).
- **Operational safeguards**: error masking with hash correlation, message truncation, blob-size
  warn/error gates (`common/util.go:604-633`), busy-loop protection for repeated workflow-task
  failures (`service/history/api/respondworkflowtaskfailed/api.go:72-77`), and DLQs for
  unprocessable internal tasks.
- **Why not 10**: no push-based alerting/webhook on tool failure inside this repo (operators must
  scrape metrics/logs); masked errors require manual hash-to-log correlation; some behavior is
  still TODO/gated (e.g., `RETRY_STATE_PAUSED` unsupported — commented out at
  `service/history/workflow/mutable_state_impl.go:6925-6927`); the legacy mutable-state retry path
  and the new CHASM path are parallel implementations kept consistent only by convention
  (`chasm/lib/activity/activity.go:1369`). No evidence found of automated human-notification hooks.

## Evidence Collected

Every entry cites `path:line` relative to `studies/agent-harness-study/sources/temporal/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Failure envelope (error classes) | Constructors for `ServerFailure`, `ResetWorkflowFailure`, `TimeoutFailure`; envelope carries Message, Source, StackTrace, Cause, and typed FailureInfo | common/failure/failure.go:14-46 |
| Envelope truncation | `Truncate`/`TruncateWithDepth` preserve NonRetryable flags while trimming message/stack/cause chain to size+depth budget | common/failure/failure.go:48-90 |
| Truncation tests | `TestTruncate`, `TestTruncateDepth` verify budgeted truncation of cause chains | common/failure/failure_test.go:10-59 |
| Retryability classification | `IsRetryableFailure`: canceled/terminated never retried; schedule-to-start/close timeouts not retried unless explicitly whitelisted via `TemporalTimeout:` prefix; application failures honor NonRetryable flag and NonRetryableErrorTypes | common/retrypolicy/retry_policy.go:27-64 |
| Classifier tests | `TestIsRetryableFailure` table-tests each failure class | common/retrypolicy/retry_policy_test.go:15 |
| Policy validation at ingestion | `Validate` rejects bad backoff coefficient, negative attempts, invalid timeout-type entries → user gets `InvalidArgument` before any execution | common/retrypolicy/retry_policy.go:103-141 |
| Defaults injection | `EnsureDefaults` + `DefaultDefaultRetrySettings` (1s initial, ×2 backoff, max-interval ×100, unlimited attempts) applied per namespace via dynamicconfig | common/retrypolicy/retry_policy.go:66-100; service/history/configs/config.go:268-272 |
| Backoff & exhaustion decision | `getBackoffInterval`/`nextBackoffInterval`: returns NoBackoff + `RETRY_STATE_NON_RETRYABLE_FAILURE` (:44), `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED` (:96), `RETRY_STATE_TIMEOUT` (:101,:109) | service/history/workflow/retry.go:31-112 |
| Worker-requested retry delay | `nextRetryDelayFrom` reads application-supplied `NextRetryDelay` from ApplicationFailureInfo and overrides policy interval | service/history/workflow/retry.go:48-67; service/history/workflow/mutable_state_impl.go:6930-6935 |
| Failure intake (model→server) | `RespondActivityTaskFailed` API: heartbeat details preserved as progress, then `RetryActivity(ai, failure)` decides retry vs terminal event | service/history/api/respondactivitytaskfailed/api.go:87-121 |
| Terminal failure recorded w/ RetryState | `AddActivityTaskFailedEvent(scheduledID, startedID, failure, retryState, identity, version)` writes event consumed by SDK/user UIs | service/history/workflow/mutable_state_impl.go:4627-4666 |
| Timeout escalation (server-enforced) | Timer executor builds `TimeoutFailure` with reason string, runs it through the same `RetryActivity` gate, and on exhaustion appends heartbeat details to the timeout failure | service/history/timer_queue_active_task_executor.go:286-367 |
| Timeout cause preservation | Schedule-to-start/close timeouts return `RETRY_STATE_TIMEOUT` while real cause stays in `failure.Cause`; comment directs SDKs to surface cause to users (issue #3667) | service/history/workflow/mutable_state_impl.go:6883-6899 |
| Pause-aware retry | Paused activities absorb failed attempts (attempt++, store truncated last-failure) and stay paused instead of rescheduling | service/history/workflow/mutable_state_impl.go:6906-6928 |
| Oversized-failure wrap | `truncateRetryableActivityFailure` replaces >limit failures with `ServerFailure(FailureReasonFailureExceedsLimit)` keeping truncated original as Cause; limits are namespace-scoped dynamicconfig | service/history/workflow/mutable_state_impl.go:7217-7238; service/history/configs/config.go:256-257,712-713 |
| CHASM mirror implementation | New activity state machine duplicates wrap-and-truncate logic with cross-reference comment; `tryReschedule` handles pause/reset-during-retry transitions | chasm/lib/activity/activity.go:1368-1426 |
| Frontend failure substitution | `RespondWorkflowTaskFailed` replaces oversized request failures with non-retryable ServerFailure after warn/error metric + throttled log | service/frontend/workflow_handler.go:1268-1310 |
| Blob-size guard rails | `CheckEventBlobSizeLimit` records `event_blob_size`, warns at warnLimit, errors + `blob_size_error` counter at errorLimit | common/util.go:602-633 |
| Workflow-task failure handling | Drops repeated failures when enabled (busy-loop guard :72-77); counts by cause tag :79-86; terminates workflow outright on GRPC_MESSAGE_TOO_LARGE :88-101; logs warn with message/source :125-132; recreates workflow task :136-139 | service/history/api/respondworkflowtaskfailed/api.go:72-139 |
| Typed user-facing errors | Server-specific errors reconstructed from gRPC codes + proto error *details* (ShardOwnershipLost, StickyWorkerUnavailable, TaskAlreadyStarted, ...) on top of shared `serviceerror.FromStatus` | common/serviceerror/convert.go:11-57 |
| User-facing masking | Internal/Unknown errors replaced by "something went wrong, please retry (<hash>)"; original logged with same hash — correlation token between user and operator | common/rpc/interceptor/mask_internal_error.go:21,67-87 |
| Error message truncation | gRPC interceptor truncates any outgoing error status beyond namespace-configured max length, appending "... <truncated>" | common/rpc/interceptor/service_error_interceptor.go:15,29-53 |
| Operator classification & grouping | `RequestErrorHandler.HandleError`: metrics `service_error_with_type` (typed tag), `service_err_resource_exhausted` (cause+scope tags), `service_failures`; expected-error list suppresses noise; logs tagged with grpc_code + workflow identity | common/rpc/interceptor/request_error_handler.go:56-116,119-184 |
| Invariant-violation escalation | `NewInternalErrorWithDPanic` escalates core invariant violations via DPanic (crash in dev, loud log in prod) | common/serviceerror/service_error_with_dpanic.go:8-12 |
| Metrics taxonomy (retries vs terminal) | Distinct counters: `activity_task_fail` (incl. retries) vs `activity_fail` (no more retries), `activity_task_timeout` vs `activity_timeout` (with timeout_type tag), plus pause/unpause/reset counters | common/metrics/metric_defs.go:980-990 |
| Attempt vs E2E latency | `RecordActivityCompletionMetrics`: start-to-close per attempt, schedule-to-close only when closed (includes retries/backoffs); timeout metrics carry `timeout_type` tag | service/history/workflow/metrics.go:187-235 |
| Task-processing failure metrics | `task_errors` (unexpected processing errors) and `task_terminal_failures` ("causing it to be sent to the DLQ") | common/metrics/metric_defs.go:894-905 |
| Persistence-level retries | Decorator clients wrap every persistence manager with pluggable policy + `IsRetryable` predicate (system-facing retry layer beneath everything else) | common/persistence/persistence_retryable_clients.go:11-74 |
| DLQ (human escalation for infra tasks) | Delete+merge DLQ workflow avoids concurrent delete/re-enqueue; admin APIs `GetDLQTasks`/`DeleteDLQTasks`; validation errors for unsupported types/tokens | service/worker/dlq/workflow.go:1-40; service/history/api/getdlqtasks/api.go:19; service/history/api/deletedlqtasks/api.go:13; service/frontend/errors.go:41,78 |
| Operator intervention APIs | PauseActivity/UnpauseActivity frontend handlers delegating to history; resetactivity/resetworkflow/pauseworkflow/updateactivityoptions API packages exist under service/history/api | service/frontend/workflow_handler.go:7166-7228 |
| Nexus operation failure envelope | Operation state carries `Failure *failurepb.Failure`; forced cancellation produces TerminatedFailureInfo | chasm/lib/nexusoperation/operation_statemachine.go:51,270-275 |
| Fallback-on-unavailable worker | `StickyWorkerUnavailable` typed error treated as "expected" (no alarm noise); converted from gRPC detail for SDK fallback | common/serviceerror/sticky_worker_unavailable.go:10-19; common/rpc/interceptor/request_error_handler.go:175; common/serviceerror/convert.go:40-44 |
| Engine tests for failure intake | Suite covers RespondActivityTaskFailed: invalid token, no execution, no run ID, task completed/not started, conflict-on-update, success, with-heartbeat, ById success | service/history/history_engine_test.go:3160-3700 |

## Answers to Dimension Questions

1. **Who sees tool failure?** All four audiences, deliberately separated:
   - *Model (workflow code)*: receives the raw `failurepb.Failure` + `RetryState` via history events
     (`service/history/api/respondactivitytaskfailed/api.go:113`,
     `service/history/workflow/mutable_state_impl.go:4653-4660`).
   - *User (SDK/API clients)*: typed `serviceerror` values mapped through gRPC status + details
     (`common/serviceerror/convert.go:11-57`), masked if namespace policy says so
     (`common/rpc/interceptor/mask_internal_error.go:53-56`).
   - *Operator*: metrics grouped by error type/cause/scope and structured logs
     (`common/rpc/interceptor/request_error_handler.go:99-116`;
     `common/metrics/metric_defs.go:980-990`).
   - *Retry system*: the server itself acts as the retry controller
     (`service/history/workflow/retry.go:31-53`).

2. **Is the error actionable?** Yes. The envelope carries message, application-defined type,
   stack trace, nested cause chain, and non-retryable flag
   (`common/failure/failure.go:14-90`); exhaustion carries an explicit `RetryState` reason;
   even masked errors keep an actionable correlation hash for support flows
   (`mask_internal_error.go:80-86`); truncation preserves the NonRetryable flag specifically so
   downstream decisions stay correct (`failure.go:69-81`).

3. **Can the model recover?** Yes, three ways: (a) automatic retries with exponential backoff,
   expiration time, and worker-suggested `NextRetryDelay`
   (`service/history/workflow/retry.go:48-67`); (b) catching the terminal failure event in
   workflow code since it is ordinary history; (c) operator-assisted recovery via
   Pause/Unpause/Reset that re-arms a failing activity mid-policy
   (`mutable_state_impl.go:6906-6928`, `service/frontend/workflow_handler.go:7166`).

4. **When is failure escalated to a human?** Never automatically paged — human escalation is
   pull-based through operator surfaces: DLQ merge/delete workflows for internal tasks that fail
   terminally (`service/worker/dlq/workflow.go:1-40`, metric
   `task_terminal_failures` at `common/metrics/metric_defs.go:898`), Reset/Pause APIs,
   blob-size violation logs/metrics (`common/util.go:604-633`), and DPanic-level logs for core
   invariant violations (`common/serviceerror/service_error_with_dpanic.go:9-12`).

5. **Are failures grouped by cause?** Yes, extensively: `ServiceErrorWithType` tag per error type,
   ResourceExhausted broken down by cause+scope (`request_error_handler.go:100-109`);
   workflow-task failures counted by `WORKFLOW_TASK_FAILED_CAUSE` tag
   (`respondworkflowtaskfailed/api.go:84`); activity timeouts by `timeout_type`
   (`service/history/workflow/metrics.go:229-231`); retry outcomes by `RetryState` enum;
   non-determinism tracked by its own counter
   (`service/frontend/workflow_handler.go:1318`, defined at `common/metrics/metric_defs.go:693`).

## Architectural Decisions

- **Single failure envelope for all classes.** One `failurepb.Failure` proto with discriminated
  info fields serves activities, child workflows, Nexus operations, and server-generated failures
  (`common/failure/failure.go:14-46`), so every consumer (SDK replay, UI, archival) needs one parser.
- **Retries decided server-side, not client-side.** The retry loop lives in the history service's
  mutable-state update path (`service/history/workflow/mutable_state_impl.go:6868-6959`), making it
  deterministic, crash-safe, and auditable from history — workers cannot miscount attempts.
- **Exhaustion as data, not exception.** `enumspb.RetryState` travels alongside the failure in the
  terminal event (`mutable_state_impl.go:4653-4657`), letting consumers distinguish
  "gave up" from "policy forbids" from "deadline passed".
- **Wrap-with-cause under resource limits.** Instead of dropping oversized failures, they become
  `ServerFailure{Cause: truncated-original}` preserving diagnosability while bounding storage
  (`mutable_state_impl.go:7235-7237`; `chasm/lib/activity/activity.go:1379-1381`).
- **Expected-vs-unexpected error taxonomy for observability noise control.** Explicit code/type
  allowlists decide which failures increment the alarming `service_failures` metric versus the
  informational `service_error_with_type` (`request_error_handler.go:119-184`).
- **Typed errors over string matching.** Server-specific errors ride gRPC status *details* protos
  and are reconstructed into typed Go errors (`common/serviceerror/convert.go:18-54`), enabling
  programmatic fallback behavior (e.g., sticky-worker fallback).

## Notable Patterns

- **Interceptor chain for cross-cutting error shaping**: masking, truncation, telemetry, and
  slow-request logging all compose around handlers
  (`common/rpc/interceptor/mask_internal_error.go`, `service_error_interceptor.go`,
  `telemetry.go`).
- **Hash-based correlation between masked users and full operator logs**
  (`mask_internal_error.go:80-86`) — a privacy/actionability compromise pattern.
- **Dual-implementation consistency by documentation**: the CHASM activity state machine copies
  the legacy truncate logic and cites its ancestor in comments
  (`chasm/lib/activity/activity.go:1369`), showing migration-in-place discipline.
- **Busy-loop guards**: dynamicconfig-gated dropping of repeated workflow-task failures prevents a
  poison task from spinning the shard (`respondworkflowtaskfailed/api.go:72-77`;
  `service/history/configs/config.go:284,728`).
- **Metrics that distinguish attempt-level from terminal outcomes**, giving SREs both a rate
  (retries happening) and a slope (activities dying)
  (`service/history/workflow/metrics.go:218-235`).
- **Timeout-as-failure normalization**: server timers synthesize the same `failurepb.Failure`
  shape as worker-reported failures, so one retry gate handles both
  (`timer_queue_active_task_executor.go:305-321`).

## Tradeoffs

- **Storage safety vs fidelity**: truncating retryable failures stored in mutable state bounds row
  sizes, but operators lose full payloads for intermediate attempts (terminal events keep the full
  failure) (`mutable_state_impl.go:7217-7238`).
- **Privacy vs debuggability**: masking internal errors protects tenants but forces manual hash
  lookup in service logs to diagnose (`mask_internal_error.go:76-86`).
- **Automaticity vs safety**: unlimited default retries (`MaximumAttempts: 0`,
  `common/retrypolicy/retry_policy.go:76-81`) favor eventual completion but require users to set
  sane policies or pay unbounded history growth.
- **Two retry engines**: legacy `MutableStateImpl.RetryActivity` and CHASM `tryReschedule` coexist;
  correctness now depends on keeping them semantically identical
  (`chasm/lib/activity/activity.go:1326-1426`).
- **Pull-based human escalation**: DLQs and reset APIs exist, but nothing in-repo pages a human —
  MTTR depends on external monitoring of `task_terminal_failures`/`service_failures`.

## Failure Modes / Edge Cases

- **Oversized failure on final attempt**: frontend substitutes a non-retryable
  `FailureReasonFailureExceedsLimit` ServerFailure for workflow-task failures over the error limit
  (`service/frontend/workflow_handler.go:1301-1310`); the workflow then sees the substitute, not
  the app error — surprising unless the SDK surfaces `Cause`.
- **GRPC_MESSAGE_TOO_LARGE**: workflow is terminated outright rather than retried
  (`respondworkflowtaskfailed/api.go:88-101`) — deliberate dead-end for undeliverable tasks.
- **Poison workflow task**: repeated identical failures could spin; mitigated only when
  `EnableDropRepeatedWorkflowTaskFailures` is enabled (`respondworkflowtaskfailed/api.go:72-77`).
- **Schedule-timeout semantics**: exhaustion-by-deadline reports `RETRY_STATE_TIMEOUT` while the
  triggering failure moves to `failure.Cause`; SDKs must cooperate to show the real error
  (documented at `mutable_state_impl.go:6883-6892`).
- **Stale shard cache**: failure reports against stale state return `ErrStaleState` and bump
  `StaleMutableStateCounter` rather than corrupting history
  (`respondactivitytaskfailed/api.go:74-81`).
- **Paused activity during failure**: attempt counter increments and last failure is stored, but no
  retry is scheduled until unpause — a silent stall visible mainly via `activity_pause` metrics
  (`mutable_state_impl.go:6911-6928`; `metric_defs.go:987-989`).
- **RETRY_STATE_PAUSED not yet supported**: code path commented out with TODO
  (`mutable_state_impl.go:6925-6927`) — paused-retry reporting is approximated as IN_PROGRESS.

## Future Considerations

- Unify legacy and CHASM retry/truncation implementations into one reusable policy module to end
  dual-maintenance (`chasm/lib/activity/activity.go:1369` cross-reference suggests awareness).
- Ship `RETRY_STATE_PAUSED` so consumers can distinguish paused-stall from active retry
  (`mutable_state_impl.go:6925-6927`).
- Add first-class failure notification hooks (webhooks/callbacks on terminal tool failure);
  today only workflow-completion callbacks propagate through retries
  (`service/history/workflow/retry.go:191-197`).
- Consider surfacing truncated-failure substitutions more loudly (e.g., dedicated metric tag) so
  "app error replaced by size-limit ServerFailure" is detectable.

## Questions / Gaps

- **Automated human paging/alerting**: no evidence found in this repo for push notifications on
  tool failure; searched for webhook/alert hooks around failure paths and found only metrics/logs
  plus workflow-completion callbacks. External alert managers must close this loop.
- **Nexus caller-side mapping**: how `failurepb.Failure` on a Nexus operation is translated into a
  Nexus handler error for the remote caller was not located in inspected files
  (`chasm/lib/nexusoperation/handler.go`, `frontend.go` contain no `failurepb` references); likely
  lives in the callback library or external SDK. Searched `handler.go`, `frontend.go`,
  `operation_statemachine.go`.
- **Web UI / CLI presentation** of failures (user-facing rendering): those components live outside
  this source directory and were excluded per source-isolation rules.
- **Elasticsearch/visibility-side failure escalation** for query failures was not examined in
  depth; only the generic typed-error transport (`convert.go`) was verified.

---

Generated by `07.08-tool-failure-escalation` against `temporal`.
