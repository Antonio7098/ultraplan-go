# Source Analysis: temporal

## Dimension 13.02: Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go server (gRPC services), protobuf APIs, SQL (Cassandra/MySQL/PostgreSQL) + Elasticsearch persistence |
| Analyzed | 2026-08-25 |

## Summary

Temporal implements retries as a layered, cross-cutting concern rather than a single mechanism. At the foundation is a reusable backoff library (`common/backoff`) offering exponential, constant-delay, error-dependent, disabled, and conditional retry policies with jitter, deadline awareness, and a resource-exhaustion-aware "throttle" mode. On top of that sit four distinct retry layers: (1) decorator clients that transparently wrap every persistence manager (`common/persistence/persistence_retryable_clients.go`); (2) retryable inter-service gRPC client decorators for frontend/history/matching/admin/operator plus server-side handler interceptors; (3) an in-memory task rescheduling framework in the history service that applies error-class-specific reschedule policies and routes terminally failed tasks to a DLQ; and (4) durable, user-facing workflow/activity retries where attempt counts, backoff parameters, and next-retry timers are persisted in workflow mutable state, so retries survive process restarts and shard failover.

Circuit breakers exist specifically for outbound task destinations (Nexus endpoints): a gobreaker-based two-step breaker pool keyed by namespace/task-group/destination converts destination outages into `ResourceExhausted(CIRCUIT_BREAKER_OPEN)` responses so tasks back off instead of cascading or hitting the DLQ. There is no notion of fallback models or alternate providers — Temporal is not model-backed infrastructure — but there are structural analogs: dual visibility stores with shadow reads, archiver provider fallback to built-in implementations, namespace-registry watch-to-polling fallback, and operator-driven multi-cluster failover. Degraded behavior is mostly graceful: open breakers return throttling errors, terminal failures go to DLQ/drop paths governed by dynamic config, internal errors can be masked at the frontend, and read-only database states are classified as transient rather than fatal.

## Rating

**9 / 10** — Mature, observable, and proven under failure. The retry model is explicit at every layer (interfaces `backoff.RetryPolicy`/`Retrier`, decorator clients, state-machine transitions), configurable both in code and via dynamic config (including per-namespace user policies), persisted durably where correctness requires it, and covered by unit tests across all layers (`common/backoff/*_test.go`, `common/circuitbreaker/circuitbreaker_test.go`, `service/history/queues/executable_test.go`, `chasm/lib/nexusoperation/*_test.go`). It loses the final point because of small inconsistencies: duplicated jitter implementations (`common/backoff/retrypolicy.go:178-185` vs `retrypolicy.go:295-297`), loss of circuit-breaker history when dynamic settings change (`common/circuitbreaker/circuitbreaker.go:19-22`), acknowledged TODOs such as non-customizable throttle policy (`common/backoff/retry.go:49`), and in-memory task attempt counters that reset on restart.

## Evidence Collected

Every entry cites workspace-relative paths into `studies/agent-harness-study/sources/temporal/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry policy interface | `RetryPolicy.ComputeNextDelay(elapsed, attempts, err)` and `Retrier` interface define the pluggable policy contract | `common/backoff/retrypolicy.go:35-44` |
| Exponential backoff formula | `min(initialInterval * pow(coefficient, attempt), maximumInterval)` with expiration interval, max attempts, overflow guards, and clamping to remaining expiration time | `common/backoff/retrypolicy.go:46-55,141-176` |
| Jitter | ±~20% jitter on exponential delays to avoid global synchronization; generic `FullJitter`/`Jitter` helpers used by queues/schedulers | `common/backoff/retrypolicy.go:178-185`; `common/backoff/jitter.go:6-22` |
| Constant-delay & error-dependent policies | `ConstantDelayRetryPolicy`, `ErrorDependentRetryPolicy` with optional max-attempts and jitter | `common/backoff/retrypolicy.go:59-69,241-293` |
| Disabled policy | `DisabledRetryPolicy` never retries (used when tasks must not retry while holding a goroutine) | `common/backoff/retrypolicy.go:27-28,187-189`; `service/history/queues/executable.go:649-662` |
| Conditional policy | `ConditionalRetryPolicy` picks one of two underlying policies per error predicate, sharing the attempt counter across branches | `common/backoff/retrypolicy.go:193-218` |
| Resource-aware throttle retry | `ThrottleRetryContext` never retries ctx cancellation/deadline, stops before crossing the caller deadline, and on `ResourceExhausted` takes the max of normal and a dedicated 1s→10s throttle policy | `common/backoff/retry.go:15-25,39-102` |
| Generic retry w/ generics + deadline check | `ThrottleRetryContextWithReturn[T]` mirrors the loop for value-returning ops | `common/backoff/retry.go:108-163` |
| User-facing RetryPolicy proto defaults | `EnsureDefaults`: initial 1s, max-interval = 100×initial, coefficient 2.0, unlimited attempts; overridable via dynamic config | `common/retrypolicy/retry_policy.go:66-100`; `common/dynamicconfig/constants.go:2681-2691` |
| Retryable-failure classification | Canceled/terminated never retried; start-to-close/heartbeat timeouts retriable unless listed under `TemporalTimeout:*` non-retryable types; application failures honor `NonRetryable` flag and type list | `common/retrypolicy/retry_policy.go:21-64` |
| Policy validation | BackoffCoefficient ≥ 1, MaxInterval ≥ InitialInterval, non-negative attempts, timeout-type validation | `common/retrypolicy/retry_policy.go:103-141` |
| Named per-call-site policies | Constants + constructors for persistence/frontend/history/matching client & handler policies (mostly 2 attempts, 50ms–1s initial intervals) | `common/util.go:40-90,162-234` |
| Task-reschedule policies | Four distinct exponential policies (generic, task-not-ready, dependency-not-completed, resource-exhausted) with coefficients 1.1–1.5 and 3–5 min caps | `common/util.go:69-90,250-281` |
| Unbounded retry under system saturation | `newClientRetryPolicy` builds a conditional capped/unbounded pair gated by dynamic config `system.retryUnboundedOnSystemResourceExhausted` | `common/util.go:180-209`; `common/dynamicconfig/constants.go:154-162` |
| Transient-error classification | Persistence transient = Unavailable/ResourceExhausted; service-client transient adds Internal, ShardOwnershipLost, StalePartitionCounts; context cancel/deadline excluded | `common/util.go:290-354` |
| Persistence retry decorators | Every manager interface (shard/execution/task/metadata/clusterMetadata/queue/nexusEndpoint) wrapped with `backoff.ThrottleRetryContext` per operation | `common/persistence/persistence_retryable_clients.go:11-53,158+` |
| Persistence factory wiring | `retryPolicy = common.CreatePersistenceClientRetryPolicy()` applied to all managers; DataLoss deliberately retried ("read-after-write unreliability") | `common/persistence/client/factory.go:19-20,121-258,270-287` |
| Inter-service retryable clients | Generated decorator clients for history/operator/admin/matching/frontend wrapping every RPC with `ThrottleRetryContext` | `client/history/retryable_client.go:11-38`; `client/history/retryable_client_gen.go:14+`; `client/operator/retryable_client_gen.go:14+` |
| Client wiring with conditional unbounded policy | History/matching clients constructed with `CreateHistoryClientRetryPolicy(dynamicconfig.RetryUnboundedOnSystemResourceExhausted.Get(dc))` | `common/resource/fx.go:292-350` |
| Server-side handler interceptor | `RetryableInterceptor` (grpc.UnaryServerInterceptor) using `CreateFrontendHandlerRetryPolicy` etc. | `common/rpc/interceptor/retry.go:11-42`; `service/frontend/fx.go:383-388`; `service/history/fx.go:261`; `service/matching/fx.go:81` |
| Task NACK → reschedule with class-specific backoff | `Nack` computes `backoffDuration(err)` choosing among generic/not-ready/resource-exhausted/dependency policies; fast-resubmit path for busy workflows capped at 10 attempts | `service/history/queues/executable.go:706-741,795-851`; `executable.go:98-101` |
| Terminal failure → DLQ or drop | `HandleErr` marks terminal failures after unexpected-error budget or regex-matched DLQ patterns; controlled by `DLQEnabled`, `MaxUnexpectedErrorAttempts`, `DLQInternalErrors`, `HistoryTaskDLQErrorPattern` dynamic configs | `service/history/queues/executable.go:80-91,140-152,543-640`; docs referenced at `executable.go:603` |
| Workflow-level retry (continue-as-new) | `getBackoffInterval` honors application-provided `NextRetryDelay`, caps at `maxInterval`/expiration time, returns typed `RetryState` (`MAXIMUM_ATTEMPTS_REACHED`, `TIMEOUT`, `NON_RETRYABLE_FAILURE`); new run's Attempt incremented from previous run | `service/history/workflow/retry.go:31-112,269-304` |
| Workflow timeout → retry/cron decision | Run-timeout timer task consults `GetRetryBackoffDuration` then `GetCronBackoffDuration` to choose `CONTINUE_AS_NEW_INITIATOR_RETRY` vs `CRON_SCHEDULE` vs terminal timeout | `service/history/timer_queue_active_task_executor.go:685-726` |
| Activity retry computation | `GetNextScheduledTime` derives next schedule from persisted `Attempt`, `RetryInitialInterval`, `RetryBackoffCoefficient`, `LastAttemptCompleteTime`, capped by `RetryMaximumInterval` | `service/history/workflow/activity.go:222-243` |
| Activity retry state update | `nextBackoffInterval` + `updateActivityInfoForRetries(ai, now+backoff, ai.Attempt+1, failure)` then durable `ActivityRetryTimerTask` generation | `service/history/workflow/mutable_state_impl.go:6930-6963,6973-6999` |
| Durable retry timer rows | `ActivityRetryTimerTask` serialized to/from proto (`TASK_TYPE_ACTIVITY_RETRY_TIMER`) with `ScheduleAttempt` field | `service/history/workflow/task_generator.go:572-583`; `common/persistence/serialization/task_serializers.go:121-122,836-869` |
| Workflow-task failure backoff | After 3 attempts, speculative workflow-task start-to-close timeout extended by `policy.ComputeNextDelay(attempt-3)` | `service/history/workflow/workflow_task_state_machine.go:47,1480-1487` |
| Nexus operation retries (CHASM) | `EventAttemptFailed` transition records failure, persists `NextAttemptScheduleTime`, emits durable `InvocationBackoffTask`; `EventRescheduled` increments `Attempt`; policy from dynamic config `nexusoperation.retryPolicy` | `chasm/lib/nexusoperation/operation_statemachine.go:49-103`; `chasm/lib/nexusoperation/config.go:180-205` |
| Circuit breaker abstraction | gobreaker two-step breaker wrapper whose settings are hot-swappable from dynamic config | `common/circuitbreaker/circuitbreaker.go:19-71` |
| Breaker pool for outbound tasks | One breaker per namespace/task-group/destination with Warn-level state-change logging; settings sourced from `config.OutboundQueueCircuitBreakerSettings` | `service/history/circuitbreakerpool/fx.go:20-76` |
| Open-breaker degradation | `CircuitBreakerExecutable.Execute` returns `ResourceExhausted(CIRCUIT_BREAKER_OPEN)` when blocked so tasks "are retried less aggressively and do not go to the DLQ"; `DestinationDownError` feeds breaker success/failure; blocked executions counted via `CircuitBreakerExecutableBlocked` metric | `service/history/queues/executable.go:925-982` (esp. 951-963, 974-981) |
| Dual visibility store + shadow reads | Secondary visibility reads enabled per namespace; shadow-read mode fans reads to secondary and cancels on close — provider-style redundancy for the visibility backend | `common/persistence/visibility/visibility_manager_dual.go:20-48,249-260`; `common/dynamicconfig/constants.go:60-74` |
| Archiver provider fallback | Custom archiver factories falling back to built-ins on `ErrUnknownScheme` (test-verified) | `common/archiver/provider/provider_test.go:151-169,416-434` |
| Watch → polling fallback | Namespace registry falls back to polling when watches unsupported (test `TestWatchFallbackToPolling`) | `common/namespace/nsregistry/registry.go:532`; `common/namespace/nsregistry/registry_watch_test.go:783-785` |
| Error masking at edge | Frontend replaces internal errors with "something went wrong, please retry" per namespace via dynamic config | `common/rpc/interceptor/mask_internal_error.go:21-64` |
| Shard-unavailable hint | Converted to `Unavailable("shard unavailable, please backoff and retry")` so SDKs back off | `common/rpc/interceptor/frontend_service_error.go:42` |
| Read-only DB detection | MySQL error codes for read-only mode/read-only transactions classified as transient (retryable), not fatal | `common/persistence/sql/sqlplugin/mysql/db.go:26-50` |
| Operational pause of activities | `PauseActivity`/`ResetActivity` allow operators to halt retry loops and reset attempt counts/options | `service/history/workflow/activity.go:245-349` |
| Tests | Backoff/jitter/cron suites; breaker suite; executable backoff randomness test; DataLoss retry-count test asserting exactly N attempts; nexus operation backoff-transition tests; activity-retry-backoff timeskipping tests | `common/backoff/retry_test.go`, `common/backoff/retrypolicy_test.go`, `common/backoff/jitter_test.go`; `common/circuitbreaker/circuitbreaker_test.go`; `service/history/queues/executable_test.go:245`; `common/persistence/persistence_retryable_clients_test.go:19-88`; `chasm/lib/nexusoperation/operation_statemachine_test.go:149-217`; `service/history/workflow/timeskipping_test.go:430,1105` |

## Answers to Dimension Questions

**1. Are retries configurable?**
Yes, extensively, at three levels.
- User-facing policies are proto-defined per activity/workflow (`InitialInterval`, `BackoffCoefficient`, `MaximumInterval`, `MaximumAttempts`, `ExpirationTime`, `NonRetryableErrorTypes`) with server-side defaults filled by `EnsureDefaults` (`common/retrypolicy/retry_policy.go:84-100`) and validated (`retry_policy.go:103-141`). Even the *default default* is a namespace-scoped dynamic setting (`history.defaultActivityRetryPolicy` / `history.defaultWorkflowRetryPolicy`, `common/dynamicconfig/constants.go:2681-2691`).
- Internal call-site policies are code constants (`common/util.go:40-90`) wrapped by constructors (`util.go:162-288`), with a global escape hatch `system.retryUnboundedOnSystemResourceExhausted` that removes the attempt cap for system-scoped ResourceExhausted errors (`common/util.go:192-209`; `common/dynamicconfig/constants.go:154-162`).
- Nexus operation retry policy is fully dynamic-config-driven including structure conversion (`nexusoperation.retryPolicy`, `chasm/lib/nexusoperation/config.go:196-205`).

**2. Are fallback providers available?**
No fallback-model/alternate-provider concept exists (not applicable to this domain). Structural analogs found:
- Dual visibility stores with per-namespace secondary reads and shadow-read verification (`common/persistence/visibility/visibility_manager_dual.go:249-260`).
- Built-in archiver fallback when a custom factory doesn't handle a scheme (`common/archiver/provider/provider_test.go:151-169`).
- Polling fallback for namespace registry watch (`common/namespace/nsregistry/registry.go:532`).
Cross-cluster failover is operator-driven (namespace handover), and requests during handover fail closed with `ErrNamespaceHandover` Unavailable (`common/util.go:130-132`) rather than silently routing elsewhere.

**3. Does the system degrade gracefully?**
Largely yes. Concrete mechanisms: open circuit breakers convert hard destination failures into ResourceExhausted throttling signals (`service/history/queues/executable.go:954-963`); terminally corrupted tasks are dropped or routed to a DLQ per category instead of blocking queues (`executable.go:597-621`); internal error details are masked at the frontend (`common/rpc/interceptor/mask_internal_error.go:21`); shard unavailability surfaces as explicit Unavailable with backoff guidance (`common/rpc/interceptor/frontend_service_error.go:42`); read-only databases are treated as transient (`common/persistence/sql/sqlplugin/mysql/db.go:26-50`). Activities can be paused/resumed and their retry options reset live (`service/history/workflow/activity.go:245-349`).

**4. Are circuit breakers used to prevent cascading failure?**
Yes, but scoped narrowly: only for outbound task destinations via the history-service breaker pool (`service/history/circuitbreakerpool/fx.go:24-57`) applied through the `CircuitBreakerExecutable` wrapper (`service/history/queues/executable.go:925-982`). Breakers are two-step (gobreaker), dynamically tunable per namespace/destination, log every state transition (`fx.go:60-76`), and expose counts/metrics. Notably, they are *not* applied to persistence or general inter-service calls — those rely on bounded retries, conditional unbounded retry under system saturation, and rate limiters instead.

**Can the system survive a provider outage without failing all requests?** For its providers (databases, dependent Temporal services, Nexus endpoints), mostly yes: persistence blips are absorbed by retry decorators; destination outages trip breakers that shed load gracefully; DB read-only states degrade to transient errors. A full primary-database outage still fails writes at the frontend after exhausting retries (by design — Temporal is the system of record), and visibility-query availability depends on the configured store without automatic query rerouting beyond the secondary/shadow flags.

## Architectural Decisions

1. **Decorator-based layering instead of centralized retry middleware.** Each boundary gets its own wrapper type: persistence managers (`common/persistence/persistence_retryable_clients.go:11-53`), generated service clients (`client/history/retryable_client_gen.go:14`), and gRPC unary interceptors (`common/rpc/interceptor/retry.go:11-42`). This keeps retry policy ownership local to each trust boundary.
2. **Durable retries via timers, not loops.** User-visible retries (activities, workflow runs, Nexus operations) are materialized as persisted timer tasks (`ActivityRetryTimerTask`, `InvocationBackoffTask`) and attempt counters inside mutable state, making them crash-safe and shard-migration-safe — unlike the in-memory task framework, which intentionally keeps attempt counters volatile (`service/history/queues/executable.go:131`).
3. **Error taxonomy drives policy selection.** `IsPersistenceTransientError` vs `IsServiceTransientError` vs `IsServiceClientTransientError` (`common/util.go:290-354`), and four distinct task reschedule policies selected by error identity/class in `backoffDuration` (`service/history/queues/executable.go:822-851`), encode operational knowledge (e.g., internal errors need slower retry than busy-workflow errors) directly into the control flow.
4. **Fail-safe conversion of overload.** Both throttle retry (`common/backoff/retry.go:81-83`) and open circuit breakers normalize pressure into `ResourceExhausted` — a first-class protocol error SDKs understand — rather than surfacing raw infrastructure errors.
5. **Terminal failure quarantine (DLQ).** Rather than infinite retries against corrupt data, the task framework has an explicit terminal path with regex-based pattern matching, attempt budgets, and admin tooling documented in `docs/admin/dlq.md` (`service/history/queues/executable.go:597-640`).
6. **Dynamic configurability for incident response.** Retry caps, DLQ toggles, breaker settings, and unbounded-retry-on-saturation are all dynamic config, allowing operators to change failure posture without deploys (`common/dynamicconfig/constants.go:154-162,2681-2691`).

## Notable Patterns

- **ConditionalRetryPolicy composition**: a predicate selects between a capped and an uncapped policy while sharing the attempt counter, implementing "unbounded only for this error class" cleanly (`common/backoff/retrypolicy.go:193-218`).
- **Application-controlled backoff override**: workers can specify `NextRetryDelay` on application failures, which overrides the server-computed exponential interval (`service/history/workflow/retry.go:47-67`; mirrored for activities at `mutable_state_impl.go:6930-6935`).
- **Typed retry outcomes**: `enumspb.RetryState` (`IN_PROGRESS`, `MAXIMUM_ATTEMPTS_REACHED`, `TIMEOUT`, `NON_RETRYABLE_FAILURE`, ...) is recorded into history events, making retry termination reasons observable to users, not just logs (`service/history/workflow/retry.go:95-111`).
- **Two-step circuit breaker**: allows the decision point and success/failure report to be separated, matching the execute-then-classify shape of task execution (`common/circuitbreaker/circuitbreaker.go:11-17`, `service/history/queues/executable.go:950-981`).
- **State-machine-native retries**: CHASM Nexus operations model retry as explicit transitions (`SCHEDULED → BACKING_OFF → SCHEDULED`) with persisted `Attempt` and `NextAttemptScheduleTime`, so backoff is part of the entity's durable state (`chasm/lib/nexusoperation/operation_statemachine.go:55-103`).
- **Jitter everywhere**: exponential policy embeds ~±10-20% jitter automatically (`common/backoff/retrypolicy.go:178-185`), and scheduler/queue code applies `backoff.Jitter` to periodic work (`service/history/queues/queue_base.go:226-239`).

## Tradeoffs

- **Durability vs latency**: durable retry timers guarantee no lost retries but add a DB write per retry scheduling decision; the in-memory fast-path (`shouldResubmitOnNack`, `executable.go:795-820`) exists precisely to skip that cost for benign errors, at the price of losing those attempts on restart (bounded at 10).
- **Aggressive defaults for internal calls**: 2-attempt caps on inter-service calls keep failure propagation fast but push retry responsibility upward; the `system.retryUnboundedOnSystemResourceExhausted` flag trades that off explicitly and documents the risk (`common/dynamicconfig/constants.go:154-162`).
- **Circuit-breaker settings hot-swap discards state**: replacing the underlying breaker on settings change loses open/half-open history, accepted consciously in a code comment (`common/circuitbreaker/circuitbreaker.go:19-22`).
- **Global jitter RNG contention**: a mutex-wrapped shared random source avoids Go stdlib lock contention but adds its own serialization point (`common/backoff/retrypolicy.go:299-346`); meanwhile a second, simpler jitter implementation coexists inconsistently (`retrypolicy.go:295-297`).
- **DLQ as safety valve introduces operational surface**: dropping vs DLQ-ing terminal failures is config-dependent (`executable.go:602-610`), meaning misconfiguration can either accumulate poison messages or silently discard them.

## Failure Modes / Edge Cases

- **Context deadline awareness**: retry loops stop early if the computed sleep would overshoot the caller's deadline, returning the last operation error rather than a synthetic one (`common/backoff/retry.go:85-101`).
- **Overflow protection**: exponential computation guards negative/overflow intervals and clamps to `math.MaxInt64` (`common/backoff/retrypolicy.go:153-157`; `common/backoff/retry.go:178-184`).
- **DataLoss treated as transient**: immediate read-after-write unreliability is retried, acknowledging storage-layer anomalies (`common/persistence/client/factory.go:270-274`), verified by exact-attempt tests (`common/persistence/persistence_retryable_clients_test.go:19-88`).
- **Shard ownership changes mid-flight**: `ShardOwnershipLost` and stale partition counts are retryable at the client layer (`common/util.go:347-351`), and the redirector re-routes to the new owner (`client/history/redirector.go:17`).
- **Namespace handover**: requests during replication handover fail fast with a distinct Unavailable error instead of half-applying (`common/util.go:130-132`; excluded from handler retryability at `util.go:356-359`).
- **Invalid task detection**: tasks failing validation are only marked invalid on the first attempt, since later failures may be caused by prior partial writes (`service/history/queues/executable.go:563-567`).
- **Busy-workflow storms**: `ErrResourceExhaustedBusyWorkflow` bypasses the rescheduler for faster retry up to 10 attempts, then falls back to backed-off rescheduling (`executable.go:714-721,795-801`).

## Future Considerations

- Unify the two jitter implementations and make the exponential policy's embedded jitter configurable (`common/backoff/retrypolicy.go:178-185,295-297`).
- Implement the stated TODO: customizable throttle retry policy and error categorization (`common/backoff/retry.go:49`).
- Preserve circuit-breaker counters across dynamic-settings updates instead of recreating the breaker (`common/circuitbreaker/circuitbreaker.go:54-71`).
- Persist or bound-account for in-memory task attempt counters across process restarts to avoid post-restart retry bursts (`service/history/queues/executable.go:131`).
- Extend shadow-read comparison results into automated promotion/rollback decisions for visibility stores, building on `visibility_manager_dual.go`.

## Questions / Gaps

- **Fallback provider routing for the core database**: none exists; multi-cluster failover relies on manual namespace handover. No evidence of automatic primary-store election was found within this source (searched `fallback`, `failover`, `handover` across `common/` and `service/`).
- **Circuit breakers for persistence**: searched `circuitbreaker.` usages repo-wide; only outbound-task/Nexus paths use them (`service/history/circuitbreakerpool/fx.go:31`, `service/history/queues/executable.go:951`). If the intent was breaker protection for SQL/Cassandra, no evidence was found — persistence relies solely on retry decorators and health signals.
- **Retry observability beyond metrics/logs**: per-operation retry counts are emitted as task metrics (`metrics.TaskAttempt`, `executable.go:698,856-858`), but no dedicated persistent audit of retry decisions for internal calls was found; `RetryState` enum in history events covers only user-facing workflow/activity retries.
- **Rate-limit interaction**: `ResourceExhausted` handling implies upstream rate limiting (frontend quota interceptors), but a full treatment of limiter↔retry coordination is dimension 13.x territory outside what this study verified in depth; evidence here limited to `throttleRetryPolicy` and `CIRCUIT_BREAKER_OPEN` cause usage.

---

Generated by `13.02-retry-fallback-and-degraded-mode` against `temporal`.
