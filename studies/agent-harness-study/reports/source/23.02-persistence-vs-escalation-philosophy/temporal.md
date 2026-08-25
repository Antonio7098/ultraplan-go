# Source Analysis: temporal

## 23.02 Persistence vs Escalation Philosophy

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (distributed workflow server; gRPC, dynamicconfig, pluggable persistence) |
| Analyzed | 2026-08-24 |

## Summary

Temporal's answer to "persist or escalate" is: **persist by default, durably record every persistence decision, and escalate to operators through explicit, observable control planes rather than silent abandonment**. The system layers several distinct retry loops:

1. **User-defined workflow retries** — a `RetryPolicy` proto carried on activities and workflows is evaluated server-side; on exhaustion the run fails and (if the policy allows) a *new run* is started with incremented attempt via continue-as-new (`service/history/workflow/retry.go:31-53`, `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:888-921`).
2. **Server-internal retries** — every layer of the server (persistence clients, inter-service RPCs, task processing) has its own explicit backoff policy built from one shared library (`common/backoff/retrypolicy.go`, `common/util.go:159-285`).
3. **Escalation** — when internal task retries exhaust a configurable attempt budget (~70 attempts / ~1 hour), tasks go to a Dead Letter Queue with metrics, log markers, and operator tooling (`service/history/queues/executable.go:589-620`, `docs/admin/dlq.md:1-71`).
4. **Operator pause/resume/reset controls** — PauseActivity/UnpauseActivity/PauseWorkflow/ResetWorkflow APIs let operators halt runaway work mid-retry (`service/frontend/workflow_handler.go:7166-7230`), and namespace-scoped *workflow rules* can automatically pause matching activities instead of letting them keep failing (`service/history/workflow/mutable_state_impl.go:10029-10085`).

Every persistence decision lands in durable history events carrying an explicit `RetryState` enum and in labeled metrics, making the persist-vs-stop decision auditable after the fact.

## Rating

**10/10 — Mature, durable, observable, extensible, and proven under failure or scale.**

Rationale:
- **Clear model**: retry decisions are centralized in pure functions with typed outcomes (`getBackoffInterval`/`nextBackoffInterval` in `service/history/workflow/retry.go:31-112`; `RetryActivity` in `service/history/workflow/mutable_state_impl.go:6878-6963`) rather than scattered ad-hoc loops.
- **Configurable at multiple levels**: per-call policy objects, per-namespace dynamic config defaults, global dynamic config knobs for DLQ thresholds and inter-service caps.
- **Operational safeguards**: resource-exhaustion-aware throttling, anti-thrash minimum continue-as-new interval, DLQ escape hatch, pause/resume, reset points.
- **Observable**: `RetryState` persisted into history events, dedicated metrics for attempts/failures/DLQ/pauses, critical-attempt log escalation.
- **Tested**: unit tests cover non-retryable classification and backoff math (`service/history/workflow/retry_test.go:17,62`; `common/backoff/retrypolicy_test.go:64`; `common/retrypolicy/retry_policy_test.go`).

The score reflects that this is arguably the reference implementation of durable persistence semantics; the only deductions-worthy items are incomplete features explicitly flagged as unsupported (e.g., `RETRY_STATE_PAUSED` TODOs) which are nonetheless handled conservatively.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry policy library | `RetryPolicy` interface + `ExponentialRetryPolicy`, `ErrorDependentRetryPolicy`, `ConstantDelayRetryPolicy`, `ConditionalRetryPolicy`, `DisabledRetryPolicy` | common/backoff/retrypolicy.go:34-79 |
| Exponential backoff formula & stop conditions | max attempts, expiration interval, max-interval clamp, jitter | common/backoff/retrypolicy.go:141-185 |
| Error-dependent branch selection | `ConditionalRetryPolicy.ComputeNextDelay` picks policy by predicate(err), shared attempt counter | common/backoff/retrypolicy.go:193-218 |
| User-facing retry-policy validation | `Validate` rejects bad coefficients/intervals; nil policy = no retry; `MaximumAttempts == 1` disables retries | common/retrypolicy/retry_policy.go:103-141 |
| Retryable-failure classifier | canceled/terminated not retryable; start-to-close & heartbeat timeouts opt-out via `TemporalTimeout:` prefix types; app failures honor `NonRetryable` | common/retrypolicy/retry_policy.go:27-64 |
| Default retry settings | `DefaultDefaultRetrySettings`: initial 1s, coefficient 100x max interval, backoff 2.0, unlimited attempts (0) | common/retrypolicy/retry_policy.go:76-81 |
| Per-namespace default override | dynamic config `history.defaultActivityRetryPolicy` / `history.defaultWorkflowRetryPolicy` | common/dynamicconfig/constants.go:2681-2694 |
| Activity retry decision | `RetryActivity`: no policy → stop; cancel requested → stop; schedule timeouts → TIMEOUT; non-retryable → stop; paused → hold; else compute backoff & schedule retry timer | service/history/workflow/mutable_state_impl.go:6878-6963 |
| App-controlled retry delay | `nextRetryDelayFrom` reads `ApplicationFailureInfo.NextRetryDelay` set by remote worker, overriding max interval | service/history/workflow/retry.go:48-67 |
| Workflow-level retry on failure | on FailWorkflow command, check retry backoff then cron backoff; retry starts new run via `handleRetry` | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:888-921,1440-1496 |
| Workflow run-timeout retry/cron decision | timeout executor picks RETRY vs CRON vs terminal based on policy presence | service/history/timer_queue_active_task_executor.go:681-706 |
| Retry-run setup | `SetupNewWorkflowForRetryOrCron` carries attempt+1, callbacks, versioning; enforces min CaN backoff | service/history/workflow/retry.go:116-329 |
| Anti-thrash guard | `ContinueAsNewMinBackoff` enforces `workflowIdReuseMinimalInterval` to prevent tight retry loops | service/history/workflow/mutable_state_impl.go:2974-2997 |
| Cron scheduling | cron backoff from schedule spec; invalid schedule rejected at validation | common/backoff/cron.go:14-60 |
| Internal-task retry/reschedule | nacked tasks rescheduled with per-error-class policies (normal/not-ready/resource-exhausted/dependency) | service/history/queues/executable.go:88-95,825-855 |
| Fast-resubmit optimization | up to 10 immediate resubmits before falling back to scheduler backoff; 1 for resource-exhausted | service/history/queues/executable.go:94-101,789-807 |
| DLQ escalation trigger | after `maxUnexpectedErrorAttempts` unexpected errors → mark terminally failed → DLQ | service/history/queues/executable.go:605-620 |
| DLQ config knobs | `history.TaskDLQEnabled`, `history.TaskDLQUnexpectedErrorAttempts` (70 ≈ 1h), `history.TaskDLQInternalErrors`, `history.TaskDLQErrorPattern` | common/dynamicconfig/constants.go:2902-2931 |
| DLQ wiring into all queue factories | transfer/timer/visibility/archival/outbound factories pass DLQ writer + thresholds | service/history/transfer_queue_factory.go:169-178 |
| DLQ runbook | detection metric `dlq_writes`, log marker "Marking task as terminally failed", purge/merge tooling | docs/admin/dlq.md:12-23,44-63 |
| Inter-service retry caps | history/matching client policies: capped exponential vs unbounded-on-system-resource-exhausted via conditional policy | common/util.go:179-210 |
| Autonomy knob for internal retries | `system.retryUnboundedOnSystemResourceExhausted` (default false) extends retry past attempt cap during overload | common/dynamicconfig/constants.go:154-162 |
| Resource-exhaustion throttling | `ThrottleRetryContext` uses separate throttle policy for ResourceExhausted, never retries ctx cancellation/deadline | common/backoff/retry.go:46-102 |
| Persistence-layer retry | retryable persistence clients wrap stores with transient-error predicate (`Unavailable`, `ResourceExhausted`) | common/persistence/client/factory.go:121-270; common/util.go:290-298 |
| Workflow task retry slowdown | attempt-based start-to-close extension using `history.workflowTaskRetryMaxInterval` (default 10m) after N attempts | service/history/workflow/workflow_task_state_machine.go:1476-1489; common/dynamicconfig/constants.go:2734-2740 |
| Operator pause activity | PauseActivity/UnpauseActivity frontend handlers → history API records `PauseInfo{PausedBy: Manual}` | service/frontend/workflow_handler.go:7166-7230; service/history/api/pauseactivity/api.go:54-66 |
| Rule-driven auto-pause | namespace workflow rules can auto-pause matched activities (`PausedBy: RuleId`) | service/history/workflow/mutable_state_impl.go:10029-10085 |
| Pause-aware retry | paused activities retain last failure and increment attempt without executing while paused | service/history/workflow/mutable_state_impl.go:6910-6930 |
| Reset recovery | ResetWorkflowExecution API + auto-reset points (`history.historyMaxAutoResetPoints`, default 20) | service/frontend/workflow_handler.go:2415-2463; common/dynamicconfig/constants.go:2695-2700; common/primitives/constants.go:27 |
| History-event observability | `RetryState` stored in ActivityTaskFailed/TimedOut, WorkflowFailed/TimedOut events | service/history/historybuilder/event_factory.go:293-380 |
| Metrics observability | `activity_task_fail` (incl. retries), `activity_fail` (final), attempt/single-attempt vs schedule-to-close latency split | common/metrics/metric_defs.go:890-903,968-982 |
| Pause metrics | `activity_pause` / `activity_unpause` counters tagged by targeting method | common/metrics/metric_defs.go:987-988; service/history/api/pauseactivity/api.go:89-94 |
| Critical-attempt log escalation | warn below 30 attempts, error-log "Critical error processing task, retrying" above; same threshold for WFT stats | service/history/queues/executable.go:102-105,592-596; service/history/workflow/workflow_task_state_machine.go:1454-1465 |
| Tests of retry decisions | `Test_NonRetriableErrors`, `Test_nextBackoffInterval`; retry-policy suite | service/history/workflow/retry_test.go:17,62; common/backoff/retrypolicy_test.go:64 |

## Answers to Dimension Questions

### 1. Does the agent persist or escalate on failure?

Persist, in a strictly ordered fashion, then escalate deliberately:

- **Activities**: on failure, `RetryActivity` decides retry-vs-terminal in one place (`service/history/workflow/mutable_state_impl.go:6878`). Terminal conditions are enumerated and typed: no policy (`RETRY_STATE_RETRY_POLICY_NOT_SET`), cancel requested, non-retryable failure class, maximum attempts reached, expiration passed (`service/history/workflow/retry.go:95-110`). While retries remain, a timer task re-drives the attempt — persistence is time-shifted, not busy-looped.
- **Workflows**: when a run fails or times out, the server checks the workflow RetryPolicy first, then cron schedule; either can spawn a fresh run with `attempt+1` (`service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:888-921`, `service/history/timer_queue_active_task_executor.go:681-706`). A minimum-backoff guard prevents tight retry spin (`ContinueAsNewMinBackoff`, `service/history/workflow/mutable_state_impl.go:2974-2997`).
- **Server internals**: task execution errors are retried via error-class-specific reschedule policies (`service/history/queues/executable.go:825-855`); after ~70 unexpected attempts the task escalates to the DLQ (`executable.go:605-620`).
- **Human escalation** exists but is explicit and operator-initiated: PauseActivity/UnpauseActivity/PauseWorkflow/ResetWorkflow (`service/frontend/workflow_handler.go:7166-7230,2415-2463`) plus DLQ merge/purge tooling (`docs/admin/dlq.md:49-71`). Notably, application code can also steer persistence: an application failure may carry `NextRetryDelay`, which overrides the computed backoff interval (`service/history/workflow/retry.go:48-67`) — delegation of timing to the worker.

### 2. Is persistence configurable?

Extremely, at four tiers:

- **Per-workflow/per-activity**: full `RetryPolicy` (initial/max interval, backoff coefficient, max attempts, expiration, non-retryable list) validated at admission (`common/retrypolicy/retry_policy.go:103-141`); nil policy = zero retries.
- **Per-namespace defaults**: dynamic config `history.defaultActivityRetryPolicy` and `history.defaultWorkflowRetryPolicy` supply out-of-box settings (initial 1s, 2.0 backoff, unlimited attempts by default) (`common/dynamicconfig/constants.go:2681-2694`, `common/retrypolicy/retry_policy.go:76-81`). So the *default autonomy level* is effectively "retry forever within schedule-to-close deadlines".
- **Global operational knobs**: DLQ thresholds and patterns (`common/dynamicconfig/constants.go:2902-2931`), workflow-task retry cap interval (`constants.go:2734-2740`), and the `system.retryUnboundedOnSystemResourceExhausted` flag that switches inter-service calls between a 2-attempt cap and unbounded-until-expiration retry under system overload (`common/dynamicconfig/constants.go:154-162`, `common/util.go:194-210`).
- **Per-error-class behavior is code-fixed but policy-selected**: `ConditionalRetryPolicy` chooses capped vs extended branches by predicate at runtime (`common/backoff/retrypolicy.go:193-218`).

### 3. Are escalation paths clear?

Yes, each has a named owner and mechanism:

- Task framework → DLQ: terminal errors or exhausted unexpected-error budget route tasks to a persistent queue, logged with a stable marker string and counted by `dlq_writes` (`service/history/queues/executable.go:589-620`, `docs/admin/dlq.md:22-28`); operators merge (retry) or purge (drop) via `tdbg dlq` commands (`docs/admin/dlq.md:52-67`).
- Workflow → parent/caller: child timeouts propagate `RetryState` to parents via `ChildWorkflowExecutionTimedOutEventAttributes.RetryState` (`service/history/workflow/mutable_state_impl.go:6840-6861`); final activity/workflow failures carry `Failure` causes so callers can distinguish "gave up after N tries" from "never retried".
- Run → external systems: completion callbacks are re-registered onto retry runs so downstream notifications survive retry hand-off (`service/history/workflow/retry.go:191-197`).
- Operator → running work: pause/unpause/reset APIs with identity, reason, and idempotency request IDs recorded in state and history (`service/history/api/pauseworkflow/api.go:56-76`, `service/history/api/pauseactivity/api.go:54-66`).
- Automatic rule-based escalation: namespace workflow rules can flip a misbehaving activity from retrying to paused automatically, attributed to the rule ID (`service/history/workflow/mutable_state_impl.go:10048-10061`).

### 4. Are persistence decisions observable?

Yes, redundantly:

- **Durable event log**: every terminal retry outcome writes a history event embedding the machine-readable reason: `CreateActivityTaskFailedEvent`, `CreateActivityTaskTimedOutEvent`, `CreateFailWorkflowEvent`, `CreateTimeoutWorkflowEvent` all take `retryState enumspb.RetryState` (`service/history/historybuilder/event_factory.go:293-380`).
- **Metrics split by phase**: `activity_task_fail` counts per-attempt failures including retries, while `activity_fail` counts only final give-ups; latencies separate single-attempt (`activity_start_to_close_latency`) from whole-retry-life (`activity_schedule_to_close_latency`) (`common/metrics/metric_defs.go:968-982`).
- **Task-framework telemetry**: `task_attempt` histogram, `task_failures`, `task_terminal_failures`, `task_dlq_failures` counters (`common/metrics/metric_defs.go:890-903`), plus attempt-count-based log severity escalation at 30 attempts (`service/history/queues/executable.go:102-105,592-596`).
- **Pause telemetry**: `activity_pause`/`activity_unpause` counters tagged by whether pause targeted an ID or type (`service/history/api/pauseactivity/api.go:86-95`).

## Architectural Decisions

1. **One shared retry vocabulary.** All retry loops — user workflows, internal tasks, RPC clients — build on the same `backoff.RetryPolicy` interface with pluggable strategies including an error-predicated composite (`common/backoff/retrypolicy.go:36-38,197-218`). This makes "how long do we keep trying?" answerable uniformly.

2. **Retry decisions are pure functions of persisted state.** Backoff computation takes `(now, attempt, policy fields, failure)` and returns `(duration, RetryState)` (`service/history/workflow/retry.go:31-112`). Because inputs live in mutable state, any server can resume another's retry loop after failover — the essence of Temporal's durability claim, implemented rather than asserted.

3. **Time-shifting instead of blocking.** Retries are materialized as future timer tasks (`GenerateActivityRetryTasks`, `service/history/workflow/mutable_state_impl.go:6957-6963`), so a waiting-for-backoff activity costs nothing but a DB row; the system never holds a goroutine to wait (`IsRetryableError` returns false by design — "rely on shouldResubmitOnNack", `service/history/queues/executable.go:652-662`).

4. **Escalation is a state transition, not an exception.** Terminal task failure flips `terminalFailureCause`, and the next Execute pass performs the DLQ write (`service/history/queues/executable.go:367-381,605-620`). Escalation therefore survives process restarts like everything else.

5. **Blast-radius limits over blind persistence.** Resource-exhausted errors get their own slower policies and disable fast-resubmit (`executable.go:94-101,845-850`); continue-as-new loops get a minimum spacing interval (`mutable_state_impl.go:2974-2997`). The system distinguishes "keep trying" from "keep hammering".

## Notable Patterns

- **Typed stop reasons**: `RETRY_STATE_*` enumerations (`MAXIMUM_ATTEMPTS_REACHED`, `TIMEOUT`, `NON_RETRYABLE_FAILURE`, `CANCEL_REQUESTED`, `RETRY_POLICY_NOT_SET`) make "why did it stop?" a first-class datum rather than a null result (`service/history/workflow/retry.go:44,96,101,109`).
- **Application-steered backoff**: workers embed `NextRetryDelay` in failures; the server honors it over policy-computed intervals (`service/history/workflow/retry.go:48-51`) — a clean delegation hook without losing server-side accounting.
- **Conditional policy composition**: capped-vs-extended retry selected per-error at runtime, driven by a dynamic-config boolean, lets operators raise autonomy during known overload windows without redeploying (`common/util.go:194-210`).
- **Rule engine as autonomy dial**: namespace workflow rules matching on workflow/activity attributes can auto-pause, converting repeated failure into human-reviewable suspension (`mutable_state_impl.go:10029-10085`).
- **Conservative handling of unfinished features**: `RETRY_STATE_PAUSED` paths exist behind TODOs and are treated as `IN_PROGRESS` until supported (`mutable_state_impl.go:6925-6928`, `timer_queue_active_task_executor.go:347-351`) — incomplete features degrade safely.

## Tradeoffs

- **Unlimited default retries vs cost**: the shipped default (`MaximumAttempts: 0` = infinite, bounded only by schedule-to-close/expiration) favors durability but can mask systemic bugs for a long time; operators must consciously bound via timeouts or policies (`common/retrypolicy/retry_policy.go:76-81`).
- **DLQ availability coupling**: `history.TaskDLQEnabled` documents that the DLQ is Cassandra-only ("not implemented for other databases"), so on SQL/Postgres deployments the escalation path silently degrades to drop-or-retry (`common/dynamicconfig/constants.go:2902-2906`, `executable.go:371-381`).
- **Regex DLQ pattern performance**: `history.TaskDLQErrorPattern` runs a regex against every task-processing error; the docs themselves warn about perf impact and advise enabling only when necessary (`common/dynamicconfig/constants.go:2923-2931`, `docs/admin/dlq.md:16-19`).
- **Fixed internal thresholds**: `resubmitMaxAttempts=10`, `resourceExhaustedResubmitMaxAttempts=1`, `taskCriticalLogMetricAttempts=30`, and the DLQ default of 70 attempts are compile-time constants (except the DLQ count), limiting tuning (`service/history/queues/executable.go:94-105`).

## Failure Modes / Edge Cases

- **Stale-attempt timeouts discarded**: a timeout firing for a superseded attempt is ignored by comparing attempt numbers (`timer_queue_active_task_executor.go:290-294`), preventing double-retry races.
- **Mixed-error-type attempt sharing**: `ConditionalRetryPolicy` shares the attempt counter across branches so alternating error classes still hit the conservative cap (`common/backoff/retrypolicy.go:192-196`).
- **Failover resets task attempts**: on active→standby regime switch, accumulated attempt counts and latency accounting reset because different execution logic applies (`executable.go:388-397`) — correct for semantics, but it means DLQ budgets restart after failover.
- **Schedule-timeout disambiguation**: schedule-to-start/close timeouts map to `RETRY_STATE_TIMEOUT` (deadline exhaustion) rather than `NON_RETRYABLE_FAILURE`, with explicit guidance that SDKs surface the causal failure (`mutable_state_impl.go:6896-6907`).
- **Invalid-task first-attempt-only heuristic**: tasks are only marked invalid if the failure occurs on attempt 1, since later failures may stem from prior partial writes (`executable.go:558-562`).
- **Context discipline**: context cancellation/deadline never retried even if `isRetryable` says otherwise; deadline-aware break avoids overshooting caller budgets (`common/backoff/retry.go:47,77-87`).

## Future Considerations

- Complete the `RETRY_STATE_PAUSED` contract: SDKs and history consumers currently cannot distinguish "paused mid-retry" from "in progress"; the TODOs at `service/history/workflow/mutable_state_impl.go:6925-6928` and `timer_queue_active_task_executor.go:347-351` track this.
- Promote hardcoded task-framework constants (fast-resubmit caps, critical-attempt threshold) to dynamic config for parity with the DLQ knobs.
- Implement the history-task DLQ for non-Cassandra backends so the primary escalation path is uniform across deployments (`common/dynamicconfig/constants.go:2902-2906`).
- Generalize the rule-engine action set beyond `ActivityPause` (currently the sole variant handled, `mutable_state_impl.go:10048-10061`) toward richer autonomy policies.

## Questions / Gaps

- **No replanning analog**: the dimension asks about "replan" behavior. Temporal has no server-side plan revision; continuation strategy is delegated entirely to workflow code (continue-as-new suggestions such as `SUGGEST_CONTINUE_AS_NEW_REASON_HISTORY_SIZE_TOO_LARGE`, `service/history/workflow/workflow_task_state_machine.go:1502-1515`, are advisory payloads to the SDK). This is a deliberate design boundary rather than missing evidence.
- **Standby-cluster retry behavior**: replication-task retry knobs exist (`ReplicationTaskProcessorErrorRetry*`, `common/dynamicconfig/constants.go:2838-2858`) but I did not trace standby task execution end-to-end; how retry budgets behave across cluster failover beyond the `resetAttempt` call (`executable.go:391`) was not fully verified.
- **CHASM standalone-activity retry**: the newer CHASM activity component mirrors these settings (`chasm/lib/activity/config.go:62`, `validator.go:31`), but its runtime retry path was not separately traced; findings above describe the classic mutable-state path.

---

Generated by `dimensions/23.02-persistence-vs-escalation-philosophy` against `temporal`.
