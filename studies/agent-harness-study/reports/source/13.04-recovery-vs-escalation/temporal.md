# Source Analysis: temporal

## Dimension 13.04: Recovery vs Escalation

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (distributed workflow orchestration server; gRPC APIs; Cassandra/SQL/ES storage) |
| Analyzed | 2026-08-25 |

## Summary

Temporal's recovery-vs-escalation model is **automatic retry first, application-code escalation second, human operator last** — and each transition is recorded as an immutable history event.

The retry-vs-stop decision is centralized in two canonical functions: `getBackoffInterval`/`nextBackoffInterval` (`service/history/workflow/retry.go:31-112`) for activities, workflows, and child workflows, and the `ExponentialRetryPolicy` family (`common/backoff/retrypolicy.go:34-79`) for internal infrastructure calls. Both return a typed `enumspb.RetryState` rather than a boolean: `RETRY_STATE_IN_PROGRESS` means keep trying; `RETRY_STATE_NON_RETRYABLE_FAILURE`, `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED`, and `RETRY_STATE_TIMEOUT` mean stop. When retries exhaust, the system does not page anyone: it records the terminal event (`ActivityTaskFailed`, `ActivityTaskTimedOut`, or `WorkflowExecutionFailed` with the `RetryState` stamped into attributes) and wakes the owning workflow so *application code* decides what happens next — that is Temporal's escalation mechanism.

Human intervention exists but is always explicit and operator-initiated: pause/unpause of individual activities or whole workflow executions (`service/history/workflow/activity.go:245-277`, `service/history/api/pauseworkflow/api.go:17`), reset-to-event-id recovery (`service/history/api/resetworkflow/api.go:28`), terminate (`service/history/workflow/util.go:98-125`), and triage via visibility queries such as `ExecutionStatus = 'Failed'` backed by `FailedWorkflowStatuses` (`service/history/consts/const.go:129-136`). There is no built-in paging/on-call/notification integration; alerting on exhausted retries is delegated to external monitoring over metrics and the visibility store.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The retry decision funnels through one server-side function returning an explicit, enumerable state machine (`service/history/workflow/retry.go:31-112`), with failure classification isolated in `retrypolicy.IsRetryableFailure` (`common/retrypolicy/retry_policy.go:27-64`).
- Every outcome is auditable by construction (event-sourced history; `service/history/historybuilder/event_factory.go:351-374` stamps `RetryState` into fail events).
- Thresholds are configurable at three layers: per-call SDK retry policies, namespace-scoped dynamic-config defaults (`common/dynamicconfig/constants.go:2681-2692`), and policy validation at start time (`common/retrypolicy/retry_policy.go:103-141`).
- Strong test coverage of the exact decision matrix (`service/history/timer_queue_active_task_executor_test.go:470-1010`; integration tests in `tests/activity_test.go:506,701`).

Why not 9–10:
- No proactive human-notification path exists anywhere in the repo (searched `escalat*`, `alert*`, `oncall`, `pagerduty` case-insensitively; only incidental security-context hits, e.g., `service/worker/batcher/activities.go:388`). Escalation depends entirely on external monitoring being set up.
- The paused-activity hold is not fully surfaced: `RETRY_STATE_PAUSED` handling is behind TODOs (`service/history/workflow/mutable_state_impl.go:6925-6927`, `service/history/timer_queue_active_task_executor.go:340-341`).
- The default policy is unlimited attempts (`MaximumAttempts: 0`, `common/retrypolicy/retry_policy.go:76-81`), acknowledged as semantically muddy by a standing TODO ("treat 0 as 0, not infinite", `service/history/workflow/retry.go:29`).

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to `studies/agent-harness-study/sources/temporal/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core retry decision | `getBackoffInterval` returns `(delay, RetryState)`; non-retryable failures stop immediately; honors app-supplied `NextRetryDelay` from application failure info | `service/history/workflow/retry.go:31-53` |
| Retry exhaustion states | `RETRY_STATE_MAXIMUM_ATTEMPTS_REACHED` when `currentAttempt >= maxAttempts` (:95-97); `RETRY_STATE_TIMEOUT` when next delay would pass expiration (:108-110); else `RETRY_STATE_IN_PROGRESS` (:111) | `service/history/workflow/retry.go:88-111` |
| Failure classification | Canceled/terminated failures never retried; schedule-to-start/schedule-to-close timeouts never retried; app failures honor `NonRetryableErrorTypes`; server failures honor `nonRetryable` flag | `common/retrypolicy/retry_policy.go:27-64` |
| Default policy + defaults injection | `DefaultDefaultRetrySettings`: initial 1s, coefficient 2.0, max-interval-coefficient 100, `MaximumAttempts: 0` (= unlimited); `EnsureDefaults` fills unset fields | `common/retrypolicy/retry_policy.go:66-100` |
| Configurability (server) | Namespace-typed dynamic config keys `history.defaultActivityRetryPolicy` / `history.defaultWorkflowRetryPolicy`, both defaulting to `DefaultDefaultRetrySettings` | `common/dynamicconfig/constants.go:2681-2692` |
| Policy validation | Rejects backoff coefficient < 1, negative attempts, max < initial interval, invalid timeout-type entries in `nonRetryableErrorTypes` | `common/retrypolicy/retry_policy.go:103-141` |
| Activity retry vs fail gate | Worker-reported failure calls `RetryActivity`; only when state ≠ `IN_PROGRESS` does it append `ActivityTaskFailed` and create a workflow task to wake the workflow | `service/history/api/respondactivitytaskfailed/api.go:105-121` |
| Activity retry decision detail | Special-cases cancel requested (:6879-6881) and schedule-timeouts (:6893-6898, citing issue #3667); paused activities "retry" forever without scheduling work (:6906-6927); success path generates retry timer tasks (:6960-6963) | `service/history/workflow/mutable_state_impl.go:6868-6963` |
| Timeout-driven retry | Timer fire re-evaluates via `RetryActivity`; stale attempts dropped (:297-300); if no time remains, converts to schedule-to-close timeout; silent state-only update while `IN_PROGRESS` (:339-344); terminal `AddActivityTaskTimedOutEvent` otherwise (:355-362) | `service/history/timer_queue_active_task_executor.go:286-367` |
| Workflow-level ladder | On `FailWorkflow` command: check workflow retry policy first (:888), then cron (:892); always record `WorkflowExecutionFailed` (:901); spawn successor run only if retry/cron backoff exists (:912-916) | `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:887-920` |
| Successor-run creation | `SetupNewWorkflowForRetryOrCron` builds new run with attempt+1 (:269-272), carries completion callbacks for retry runs (:192-197), enforces minimal backoff against tight-loop continue-as-new spin (:292-293) | `service/history/workflow/retry.go:116-329` |
| Run-timeout stop point | Run timeout always writes the timeout event (:707-714); picks `CONTINUE_AS_NEW_INITIATOR_RETRY`/`_CRON_SCHEDULE` if a policy allows (:691-699), otherwise aborts permanently ("No more retries, or workflow is expired") (:716-726) | `service/history/timer_queue_active_task_executor.go:645-726` |
| Audit trail | Fail-workflow event factory stamps `EVENT_TYPE_WORKFLOW_EXECUTION_FAILED` attributes with `RetryState` and `NewExecutionRunId`, linking failure → retry run | `service/history/historybuilder/event_factory.go:351-374` |
| Human hold (pause) | `PauseActivity` sets `Paused=true` + `PauseInfo` (idempotent per request ID); running attempts allowed to finish | `service/history/workflow/activity.go:245-277` |
| Human resume | `UnpauseActivity` supports by-ID, by-Type, and unpause-all selectors; variants `UnpauseActivityWithResume` / `UnpauseActivityWithReset` | `service/history/workflow/activity.go:391-484`, `service/history/api/unpauseactivity/api.go:81-135` |
| Operator kill switch | `TerminateWorkflow` force-fails any started workflow task then appends terminal `WorkflowExecutionTerminated` | `service/history/workflow/util.go:95-125` |
| Operator reset recovery | Reset API creates a new run from a historical event ID with post-reset operations; tolerates duplicate resets | `service/history/api/resetworkflow/api.go:28-214` |
| Triage visibility | `FailedWorkflowStatuses` = {FAILED, CANCELED, TERMINATED, TIMED_OUT} used by start-policy and queryable via `ExecutionStatus` search attribute | `service/history/consts/const.go:129-136` |
| Infra-layer retry | `ThrottleRetryContext`: context cancellation never retried; `ResourceExhausted` switches to a separate never-expiring throttle policy (1s→10s cap) instead of failing | `common/backoff/retry.go:15-60` |
| Infra retry client | All history-service gRPC calls wrapped in `NewRetryableClient` → `ThrottleRetryContext` | `client/history/retryable_client.go:19-38` |
| Workflow-task self-healing | Start-to-close timeout grows exponentially once attempt > 3 (`workflowTaskRetryBackoffMinAttempts = 3` at :47), giving stuck workers progressively longer windows | `service/history/workflow/workflow_task_state_machine.go:1465-1489` |
| Tests: decision matrix | `TestProcessActivityTimeout_NoRetryPolicy_Fire` (:470), `_RetryPolicy_Retry` (:638), `_RetryPolicy_RetryTimeout` (:731), `_RetryPolicy_Fire` (:832) cover every branch of retry-vs-stop | `service/history/timer_queue_active_task_executor_test.go:470-1010` |
| Tests: policy math & classification | `Test_NonRetriableErrors` (:17), `Test_nextBackoffInterval` (:62) | `service/history/workflow/retry_test.go:17,62` |
| Tests: integration | `TestActivityRetry` (:506), `TestActivityRetry_Infinite` (:701) | `tests/activity_test.go:506,701` |

## Answers to Dimension Questions

**1. When does the system retry vs escalate?**
Retry continues exactly while all of these hold: the failure is classified retryable (`common/retrypolicy/retry_policy.go:27-64` — canceled/terminated and schedule-timeout failures are never retried; app failures honor `nonRetryableErrorTypes`), attempts remain under `MaximumAttempts` (`service/history/workflow/retry.go:95-97`), and the next backoff fits before the expiration time (`service/history/workflow/retry.go:108-110`). The moment any condition fails, the activity/workflow closes with a terminal history event and a new workflow task wakes the parent workflow (`service/history/api/respondactivitytaskfailed/api.go:105-121`). "Escalation" in Temporal means handing control back to application code (which can signal/update/alert as it sees fit) plus making the failed execution visible (`ExecutionStatus='Failed'`). For internal service-to-service errors, load-shedding is treated differently: `ResourceExhausted` never counts against the caller's policy and instead retries indefinitely under a slow throttle policy (`common/backoff/retry.go:15-60`) — retry harder rather than escalate.

**2. Are escalation thresholds configurable?**
Yes, at three layers: (a) callers supply full `RetryPolicy` proto fields per activity/workflow/child-workflow; (b) operators set namespace-scoped defaults via dynamic config keys `history.defaultActivityRetryPolicy` and `history.defaultWorkflowRetryPolicy` (`common/dynamicconfig/constants.go:2681-2692`), with sample YAML showing the shape (`config/dynamicconfig/development-cass.yaml:10-21`); (c) `retrypolicy.Validate` rejects malformed policies at start time (`common/retrypolicy/retry_policy.go:103-141`). Additionally, workflow-task retry behavior has shard-level knobs (`WorkflowTaskCriticalAttempts`, `WorkflowTaskRetryMaxInterval` consumed at `service/history/workflow/workflow_task_state_machine.go:1455,1485`).

**3. Can the system stop gracefully?**
Yes, through multiple graded mechanisms: terminal events cleanly close executions and wake dependents (`service/history/api/respondactivitytaskfailed/api.go:113-118`); operator terminate fails any in-flight workflow task before appending the terminal event (`service/history/workflow/util.go:107-115`); run timeouts record the timeout event before deciding anything further (`service/history/timer_queue_active_task_executor.go:707-714`); tight retry/cron loops are dampened by a minimal first-task backoff (`service/history/workflow/retry.go:292-293`); and infra retries respect context cancellation unconditionally (`common/backoff/retry.go:46-48`). Graceful degradation also extends to workers: repeated workflow-task failures extend their own timeout window instead of thrashing (`service/history/workflow/workflow_task_state_machine.go:1480-1488`).

**4. Are recovery decisions auditable?**
Yes — this is arguably the strongest dimension of the design. Because mutable state is event-sourced, every retry/stop decision materializes as a durable history event carrying its `RetryState`: `ActivityTaskFailed`/`ActivityTaskTimedOut` with retry state parameters (`service/history/api/respondactivitytaskfailed/api.go:113`, `service/history/timer_queue_active_task_executor.go:355-360`), and `WorkflowExecutionFailed` stamped with both `RetryState` and the successor `NewExecutionRunId`, which makes the failure→retry-run lineage reconstructible (`service/history/historybuilder/event_factory.go:351-374`). Pause/unpause carry request IDs and identity/reason payloads (`service/history/workflow/activity.go:260-275`, `service/history/api/pauseworkflow/api.go:99-102`). What is *not* automatic is surfacing: there is no notification channel; humans must query history/visibility or consume metrics (e.g., `RecordActivityCompletionMetrics` at `service/history/timer_queue_active_task_executor.go:323-337`).

## Architectural Decisions

1. **Typed decision results instead of booleans.** The single most important choice: retry decisions return `enumspb.RetryState`, enabling callers to distinguish "stopped because non-retryable" from "stopped because budget exhausted" from "stopped because deadline passed" and to persist that distinction (`service/history/workflow/retry.go:41-44,96,101,109-111`). This state is written into history events, giving auditability for free.
2. **Escalation target is application code, not operations staff.** Exhausted retries close the activity and create a workflow task; the workflow's own logic decides remediation (`service/history/api/respondactivitytaskfailed/api.go:117`). The server provides no paging; it provides *visibility primitives* instead.
3. **Human controls are explicit, idempotent APIs.** Pause (with request-ID idempotency), unpause (by ID/type/all), reset, and terminate are separate, permission-gated endpoints — not implicit side effects of retry exhaustion (`service/history/workflow/activity.go:245-277`, `service/history/api/resetworkflow/api.go:28`).
4. **Two distinct retry regimes.** User-facing execution retries use caller-configurable expiring policies; internal RPC retries use `ThrottleRetryContext` where resource exhaustion triggers an unbounded slow lane and cancellation is sacred (`common/backoff/retry.go:39-60`). This prevents server overload from masquerading as user failure.
5. **Defaults live server-side and are namespace-scoped.** A namespace can have different out-of-box retry behavior than another without SDK changes (`common/dynamicconfig/constants.go:2681-2692`).

## Notable Patterns

- **Pause-as-infinite-hold:** a paused activity increments attempts and records the last failure but never schedules work until a human unpauses; notably it still reports `RETRY_STATE_IN_PROGRESS` today, with proper `RETRY_STATE_PAUSED` semantics parked behind TODOs (`service/history/workflow/mutable_state_impl.go:6906-6927`, `service/history/timer_queue_active_task_executor.go:340-341`).
- **Deadline conversion:** when a retry would fit the policy but not the remaining schedule-to-close budget, the failure is rewritten as a schedule-to-close timeout so downstream consumers see why retries truly stopped (`service/history/timer_queue_active_task_executor.go:312-321`).
- **Stale-event guarding:** timer tasks compare fired-attempt vs current attempt and no-op if a retry already superseded them (`service/history/timer_queue_active_task_executor.go:297-300`).
- **Application-directed retry timing:** an application failure may carry `NextRetryDelay`, overriding the exponential curve — recovery pacing controlled by the failing worker itself (`service/history/workflow/retry.go:47-51,55-67`).
- **Self-widening worker deadlines:** workflow-task start-to-close grows exponentially after 3 consecutive failures (`service/history/workflow/workflow_task_state_machine.go:47,1480-1488`), trading latency for fewer spurious escalations.
- **Nexus operations mirror the same model:** cross-service Nexus operations compute their own backoff and schedule invocation-backoff tasks, with async callbacks resolving the operation (`chasm/lib/nexusoperation/operation_statemachine.go:49-100`).

## Tradeoffs

- **Silent infinite retries by default.** `MaximumAttempts: 0` means "unlimited" (`common/retrypolicy/retry_policy.go:80`); combined with the 90-day-ish expiration semantics this can mask a permanently broken activity for a very long time unless someone watches metrics/visibility. The ambiguity of `0` is flagged by TODO (`service/history/workflow/retry.go:29`).
- **No push-based escalation.** Humans learn about exhausted retries only if they built dashboards/queries. This keeps the core clean and provider-neutral, but means out-of-the-box failure→human latency is unbounded.
- **Incomplete paused-state semantics.** Reporting `IN_PROGRESS` for held activities (`mutable_state_impl.go:6925-6927`) blurs observability: consumers cannot currently distinguish "actively backing off" from "human-frozen" from the retry state alone; they must inspect `PauseInfo`.
- **Complexity concentration.** The retry decision touches mutable-state mutation, timer generation, stamp/version guards, and pause interplay within one function (~100 lines, `mutable_state_impl.go:6868-6963`); correct behavior depends on subtle interactions (e.g., attempt-vs-timer races) that required dedicated race-guard comments and tests.

## Failure Modes / Edge Cases

- **Timer/attempt race:** an activity retry timer firing after a newer attempt was scheduled is detected and dropped (`service/history/timer_queue_active_task_executor.go:297-300`); similarly stale stamps cause retry-timer drops during re-dispatch.
- **Duplicate pause requests:** same request ID is accepted as no-op; conflicting ones fail with precondition error (`service/history/workflow/activity.go:260-265`).
- **Zero-value traps:** zero `maxInterval` with non-positive computed interval yields `RETRY_STATE_TIMEOUT` (`service/history/workflow/retry.go:100-102`); nil expiration times are normalized away (:84-86).
- **Tight-loop continue-as-new:** a workflow that immediately continue-as-news is damped by minimal first-task backoff (`service/history/workflow/retry.go:292-293`).
- **Reset chains:** pathological reset chains are bounded by `ErrResetRedirectLimitReached` (`service/history/consts/const.go:139-140`, referenced from `service/history/api/resetworkflow/api.go:219`).
- **Cancellation during infra retry:** context cancellation/timeout is never swallowed by the retry loop (`common/backoff/retry.go:46-48`).

## Future Considerations

- Finish `RETRY_STATE_PAUSED` propagation so paused holds are distinguishable in events and metrics (TODO sites: `service/history/workflow/mutable_state_impl.go:6925-6927`, `service/history/timer_queue_active_task_executor.go:340-341`, `respondactivitytaskfailed/api.go:109-110`).
- Make the throttle sub-policy customizable (TODO at `common/backoff/retry.go:49`).
- Clarify `MaximumAttempts = 0` semantics ("treat 0 as 0, not infinite", `service/history/workflow/retry.go:29`) to reduce misconfiguration risk.
- An optional webhook/notification hook on terminal retry states would close the gap between "auditable" and "actionable" without violating the no-builtin-paging design stance.

## Questions / Gaps

- **No evidence found** for proactive human notification/escalation integrations. Searched the whole source case-insensitively for `escalat`, `alert`, `oncall`, `pagerduty`, `page`: the only `escalat` hits are security-context strings (`service/worker/batcher/activities.go:388`, `service/worker/batcher/activities_namespace_test.go:24`) and a merge-heuristic comment (`chasm/tree.go:2629`). Escalation-to-human is exclusively pull-based (visibility queries, operator APIs).
- **No evidence found** in-repo for the canonical event-type enum definitions (`EventTypeWorkflowExecutionFailed` etc.); they live in the external `go.temporal.io/api` module dependency, so precise enum line numbers cannot be cited from this repository. Internal proto references exist at `proto/internal/temporal/server/api/historyservice/v1/service.proto`.
- The frontend authorization layer for pause/reset/terminate was not deeply analyzed here (out of scope boundary: who *may* trigger human interventions), though handler entry points exist at `service/history/handler.go:2412-2459`.

---

Generated by dimension 13.04 (`13.04-recovery-vs-escalation`) against `temporal`.
