# Source Analysis: temporal

## 07.01 Tool Scheduling and Dispatch

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` (temporalio/temporal server) |
| Language / Stack | Go; gRPC internal APIs; DB-backed durable task queues (SQL/Cassandra); fx dependency injection |
| Analyzed | 2026-08-24 |

All citations below are relative to the source root `studies/agent-harness-study/sources/temporal/`.

## Summary

Temporal is a workflow orchestration server, so its analog of "tool calls" is **activities** and **workflow tasks**. It does not execute tools itself; it durably schedules, dispatches, and tracks them until an external worker picks them up via long poll.

Dispatch is a three-hop pipeline:

1. **Schedule (history service).** A worker completes a workflow task; the history service validates each command (`handleCommandScheduleActivity`, `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:467`), records an `ActivityTaskScheduledEvent`, and generates a transfer-queue task in the same state-machine transaction (`GenerateActivityTasks`, `service/history/workflow/task_generator.go:552-570`; task IDs are allocated by the shard, per the comment at `service/history/workflow/task_generator.go:561`).
2. **Transfer (history service).** Background queue processors read persisted tasks in batches and submit executables into a layered scheduler stack — interleaved weighted round-robin over namespace+priority channels → execution-aware sequential queues → FIFO worker pool (`service/history/queues/scheduler.go:151-175`) — whose executor RPCs the matching service (`pushActivity` calls `matchingRawClient.AddActivityTask`, `service/history/transfer_queue_task_executor_base.go:95-119`). Shard-level notification makes this near-immediate after persistence (`service/history/shard/context_impl.go:503`, `service/history/history_engine.go:915-939`).
3. **Match (matching service).** `AddActivityTask` resolves a task-queue partition and either **sync-matches** the task directly to an already-waiting poller or **spools it to durable backlog storage** (`taskQueuePartitionManagerImpl.AddTask`, `service/matching/task_queue_partition_manager.go:558-677`: `TrySyncMatch` at :620, `SpoolTask` at :662). Workers long-poll (`PollWorkflowTaskQueue`, `service/matching/matching_engine.go:672`; `pollTask` deadline/jitter logic at `service/matching/matching_engine.go:3009-3070`); a backlog reader drains DB tasks in batches into the matchers (`service/matching/task_reader.go:132-247`).

Two fast paths bypass the slow path: sync match when a poller is already waiting, and **eager activity execution**, where the activity is handed back inside the same `RespondWorkflowTaskCompleted` response to the same worker when `RequestEagerExecution` is set and `EnableActivityEagerExecution` is on (`service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:542-572` and response construction at :601-658; dynamicconfig key at `common/dynamicconfig/constants.go:203`).

The model answers "why did this tool run now" exceptionally well: every hop is event-sourced, rate-limited, priority/fairness-aware, batched, and instrumented with dedicated dispatch-latency metrics plus OpenTelemetry spans on the history-executor side.

## Rating

**Score: 9 / 10** — Mature, durable, observable, extensible, proven under failure and scale.

Rationale against the rubric:
- **Clear model with explicit interfaces**: typed task categories (`service/history/tasks/category.go`), `queues.Executor` interface implemented by `transferQueueActiveTaskExecutor.Execute` with a type switch over all transfer-task kinds (`service/history/transfer_queue_active_task_executor.go:156-174`), and generic scheduler abstractions (`common/tasks/scheduler.go:6-11`).
- **Tests**: sync-match behavior covered by `TestSyncMatchActivities` (`service/matching/matching_engine_test.go:1511`), the fairness matcher by `TestMatchingEngine_Fair_Suite` (`service/matching/matching_engine_test.go:160`) and ordering comparators in `service/matching/fair_level_test.go:9-26`; timer-driven re-dispatch mocked/asserted in `service/history/timer_queue_active_task_executor_test.go:1418-1434`.
- **Operational safeguards**: dispatch rate limits (`service/matching/ratelimit_manager.go:68-110`), schedule-to-start expiry, DLQs (`service/history/queues/executable.go:237-241`), panic recovery (`service/history/queues/executable.go:326-338`), ack levels, partition forwarding/failover.
- Not a 10 because: two parallel matcher implementations coexist during migration (legacy channel-based `TaskMatcher` vs new B-tree priority matcher), the matching-service dispatch hop itself lacks OTEL spans, and end-to-end ordering is intentionally non-deterministic (documented but inherently complex).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dispatcher (schedule side) | Workflow command → scheduled event + pending-activity limit + eager-start decision | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:467-572 |
| Task generation | Transfer `tasks.ActivityTask` created in same transaction as history event ("TaskID ... set by shard") | service/history/workflow/task_generator.go:552-570 |
| Retry scheduling | Activity retry timers generate `ActivityRetryTimerTask` visible at scheduled time | service/history/workflow/task_generator.go:572-583 |
| Executor (transfer queue) | Type-switch executes Activity/Workflow/Close/Cancel/Signal/Child/Reset/Delete/Chasm tasks | service/history/transfer_queue_active_task_executor.go:156-174 |
| Dispatch RPC to matching | `pushActivity` → `AddActivityTask` with ScheduleToStartTimeout + vector clock; NotFound logged not fatal | service/history/transfer_queue_task_executor_base.go:95-124 |
| Timer-driven re-dispatch | Standby/active timer executors call `AddActivityTask` for retries/timeouts | service/history/timer_queue_standby_task_executor.go:789 |
| New-task notification | Shard notifies engine after persisting tasks; engine fans out to category queue processors | service/history/shard/context_impl.go:503 ; service/history/history_engine.go:915-939 |
| History reader batching | Event loop loads slices with `BatchSize()`, submits each executable; future-fire tasks rescheduled | service/history/queues/reader.go:427-487,514-529 |
| Scheduler stack | FIFO pool wrapped by ExecutionAwareScheduler then InterleavedWeightedRoundRobin (namespace+priority channels) | service/history/queues/scheduler.go:105-175 |
| IWRR strategy | Channels keyed per namespace+priority, weights flattened into interleaved round-robin list | common/tasks/interleaved_weighted_round_robin.go:41-70 |
| Sequential per-workflow fallback | ExecutionAwareScheduler routes busy-workflow tasks to strictly sequential per-execution queues | common/tasks/execution_aware_scheduler.go:26-36 ; service/history/queues/scheduler.go:218-223 |
| Executor observability | OTEL span `queue.Execute/<TaskType>` with workflow/run/task IDs; TaskLoadLatency & TaskProcessingLatency metrics | service/history/queues/executable.go:283-323,248-251,345-349 |
| Matching entrypoint | `AddActivityTask` resolves partition, computes expiry from ScheduleToStartTimeout | service/matching/matching_engine.go:632-669 |
| Sync-match vs spool | `TrySyncMatch` first; else `SpoolTask` to durable backlog; result recorded per outcome | service/matching/task_queue_partition_manager.go:600-677 |
| Legacy matcher rendezvous | Synchronous channels pair producers/pollers; Offer blocks on rate-limit token, forwards to parent partition if no local poller | service/matching/matcher.go:23-45,108-170 |
| Backlog-ordering guard | Sync match blocked "to ensure better dispatch ordering" while significant backlog present | service/matching/matcher.go:109-116 |
| Priority matcher | Fast path `MatchTaskImmediately`; otherwise `EnqueueTaskAndWait` blocks until poller match | service/matching/pri_matcher.go:390-443 |
| Deterministic comparator | B-tree ordered by effectivePriority → fairLevel `<pass,id>` → pointer tiebreaker | service/matching/matcher_data.go:128-152 |
| Fairness design doc | Sequential int64 task IDs; fair level tuple ordered lexicographically by persistence; failover may repeat dispatch | service/matching/fairness.md:6-16,26-28 |
| Backlog reader | Pump reads DB ranges up to RangeSize, buffers `GetTasksBatchSize()-1`, drops expired tasks, persists acks periodically | service/matching/task_reader.go:46,132-247,254-260,282-285 |
| Poll side | Long-poll with full-jittered expiration, outstanding-poller registry for cancellation, shutdown-worker rejection | service/matching/matching_engine.go:3020-3069 |
| Spooled-task dispatch | Re-resolves routing/version on each attempt; redirects between backlogs if versioning changed | service/matching/task_queue_partition_manager.go:892-953 |
| Rate limiting config | Dynamic RPS from admin/namespace/partition configs; per-fairness-key limiters; worker-supplied RPS injected at poll | service/matching/ratelimit_manager.go:68-110 ; service/matching/task_queue_partition_manager.go:839-848 |
| Batch/queue knobs | RangeSize=100000 default; GetTasksBatchSize, MaxTaskBatchSize, UpdateAckInterval, LongPollExpirationInterval, NumReadPartitions | service/matching/config.go:296-312 ; common/dynamicconfig/constants.go:1271,1281,1297,1334,1359 |
| Dispatch latency metric | `task_dispatch_latency` timer with source/forwarded/priority/build-id tags; doc table contrasts vs schedule_to_start | common/metrics/metric_defs.go:1315 ; service/matching/matching_engine.go:3072-3134 |
| Forwarding observability | Local/remote match counters distinguish task-forward vs poll-forward paths | service/matching/matcher.go:608-630 |
| No-recent-poller signal | Counter emitted only after partition loaded >2min with no recent pollers | service/matching/task_queue_partition_manager.go:600-608 |
| Eager dispatch bypass | `RequestEagerExecution` + `EnableActivityEagerExecution` returns started activity task in WFT-complete response | service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:542-658 ; common/dynamicconfig/constants.go:203 |
| Sticky queues | Sticky partitions not loaded without pollers; `StickyWorkerUnavailable` error path | service/matching/matching_engine.go:597-603 |
| Tests | TestSyncMatchActivities; TestMatchingEngine_Fair_Suite; fair-level ordering tests; AddActivityTask expectations in timer tests | service/matching/matching_engine_test.go:1511,160 ; service/matching/fair_level_test.go:9-26 ; service/history/timer_queue_active_task_executor_test.go:1418-1434 |

## Answers to Dimension Questions

**1. How does a tool call start?**
A user SDK worker executing a workflow emits commands with a workflow-task completion. The history service processes each command: validation (attributes, payload size, pending-activity cap: `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:490-522`), then `AddActivityTaskScheduledEvent` (:552-556). The task generator appends a transfer-queue `tasks.ActivityTask` in the same mutable-state transaction (`service/history/workflow/task_generator.go:552-570`). After persistence, the shard notifies queue processors (`service/history/shard/context_impl.go:503`), the transfer executor RPCs matching's `AddActivityTask` (`service/history/transfer_queue_task_executor_base.go:103`), which sync-matches or spools the task. Alternatively, eager execution returns the started activity directly in the completion response (`.../workflow_task_completed_handler.go:542-572`).

**2. Is tool execution inline or queued?**
Queued and asynchronous through durable storage at every hop (history event transaction → DB transfer/timer queues → matching DB backlog). Two opportunistic synchronous fast paths exist — sync match to a waiting poller (`service/matching/task_queue_partition_manager.go:619-621`) and eager dispatch — but both are best-effort optimizations that fall back to spooling (`:650-662`). Execution itself happens out-of-process in user workers; the server never runs tools inline.

**3. Are tool calls ordered?**
Partially deterministic by design. Matching-side order is defined by sequential task IDs combined into fair levels `<pass,id>` (`service/matching/fairness.md:6-16`) evaluated in a B-tree ordered by priority → fair level (`service/matching/matcher_data.go:137-152`); sync match is deliberately suppressed under significant backlog to preserve this order (`service/matching/matcher.go:109-116`). Global strict FIFO is intentionally relaxed: fairness passes spread writes out of ID order (`service/matching/fairness.md:13-14`), multiple read partitions exist (`common/dynamicconfig/constants.go:1359`), and history-side processing is weighted round-robin across namespaces/priorities (`common/tasks/interleaved_weighted_round_robin.go:41-70`). Within a single workflow, correctness comes from optimistic concurrency (busy-workflow errors rerouted to sequential per-execution queues: `common/tasks/execution_aware_scheduler.go:26-36`).

**4. Can tools be batched?**
Yes, throughout: history readers load tasks in configurable batches from range slices (`service/history/queues/reader.go:455`); the matching backlog reader fetches DB ranges up to `RangeSize` (100000 default, `service/matching/config.go:296`) with `GetTasksBatchSize` per read (`service/matching/task_reader.go:199-209`) and caps buffered dispatch with `MaxTaskBatchSize` (`common/dynamicconfig/constants.go:1334`). Note these are I/O/dispatch batches, not semantic fan-out of one call to many tools; per-workflow concurrency is bounded instead by the pending-activities limit (`.../workflow_task_completed_handler.go:520-522`).

**5. Is dispatch observable?**
Yes. A dedicated `task_dispatch_latency` timer with a documented semantic contract distinguishing it from schedule-to-start latency (`service/matching/matching_engine.go:3072-3134`, definition at `common/metrics/metric_defs.go:1315`); sync-match emission points (`service/matching/matcher.go:138,153,288`); local/remote match-path counters (`matcher.go:608-630`); no-recent-poller counters (`task_queue_partition_manager.go:600-608`); backlog-age tracking (`service/matching/task_reader.go:70-83`); OTEL spans around history executor runs including optional full-payload debug spans (`service/history/queues/executable.go:283-315`); per-task structured logs (`tasks.InitializeLogger`, `service/history/transfer_queue_task_executor_base.go:123`); and introspection APIs (`DescribeTaskQueue`/poller info, `service/matching/task_queue_partition_manager.go:1178-1200`).

## Architectural Decisions

- **Durable queues over in-memory queues everywhere.** Tasks survive crashes in DB tables keyed by sequentially allocated shard task IDs; matching keeps an in-memory rendezvous only as a fast path and falls back to spooled backlog (`service/matching/task_queue_partition_manager.go:618-662`).
- **Pull-based worker model with long polling.** Workers poll; the server matches polls to tasks synchronously, avoiding push routing complexity and enabling version-directed routing (`PollTask` version-set resolution, `service/matching/task_queue_partition_manager.go:751-832`).
- **Separation of scheduling (history) from matching (dispatch).** History owns causality/state machines; matching owns load distribution, fairness, rate limits, and versioning across partitions (`service/history/transfer_queue_active_task_executor.go:104-124` → `service/matching/matching_engine.go:583-669`).
- **Layered scheduler composition.** Generic reusable scheduler components (FIFO, IWRR, rate-limited, group-by, execution-aware) compose into the history task pipeline (`common/tasks/*.go`; wiring at `service/history/queues/scheduler.go:146-175`).
- **Priority + fairness as first-class dispatch inputs.** Priority keys and per-namespace fairness keys flow through both services (fairness-key extraction hack pending SDK support at `.../workflow_task_completed_handler.go:474-488`; fair levels at `service/matching/fairness.md:8-14`; per-key rate limits at `service/matching/ratelimit_manager.go:73`).
- **Multi-partition task queues with forwarding.** Read/write partitions forward tasks and polls toward the root partition when local match fails (`service/matching/matcher.go:143-166`), with metrics distinguishing all four match combinations (`matcher.go:608-630`).

## Notable Patterns

- **Sync-match fast path with durable fallback**: try in-memory match → else spool → background reader later dispatches (`service/matching/task_queue_partition_manager.go:619,662`).
- **Backpressure via rate limiter tokens attached to tasks**: tokens can be recycled if a matched task turns out invalid (`service/matching/matcher.go:118-128,75-77`).
- **Ack-level/read-level gap tracking** for exactly-once-progress bookkeeping over at-least-once delivery (`service/matching/task_reader.go:171,259,273`).
- **Speculative/in-memory tasks** to avoid DB writes for workflow-timeout racing (`service/history/workflow/task_generator.go:500-517`).
- **Jittered long-poll deadlines** to prevent thundering-herd reconnects (`service/matching/matching_engine.go:3036-3049`).
- **Outstanding-poll registries** allowing the frontend to cancel specific polls and bulk-cancel on worker shutdown (`service/matching/matching_engine.go:3052-3067`).
- **Executor wrapper pattern**: every history task passes through one `executableImpl.Execute` providing panic recovery, latency metrics, tracing, retries, and DLQ escalation uniformly (`service/history/queues/executable.go:255-349,237-241`).

## Tradeoffs

- **Durability vs latency**: the spool-first design costs a DB write per unmatched task; sync match and eager execution exist precisely to amortize this, but eager execution weakens the single-dispatch-path invariant (activity starts recorded before the completion transaction commits, `.../workflow_task_completed_handler.go:601-613`).
- **Fairness vs simplicity/order**: pass-spreading writes break natural ID order and force B-tree re-merging plus a buffer-reset protocol (`service/matching/fairness.md:52-80`); acknowledged possibility of repeated dispatch after failover (`fairness.md:26-28`).
- **At-least-once delivery**: workers must tolerate duplicate task attempts; the server compensates with vector clocks and attempt counters (`service/history/transfer_queue_task_executor_base.go:115`).
- **Dual matcher implementations**: legacy channel matcher and new priority matcher coexist behind runtime flags (`computeEffectiveConfig`, `service/matching/task_queue_partition_manager.go:211`), doubling test surface during migration.
- **Metrics-first, traces-second**: rich counters/timers cover dispatch, but distributed trace context stops at history executors; cross-service dispatch causality must be reconstructed from logs/metrics rather than a trace.

## Failure Modes / Edge Cases

- **Expired tasks silently dropped** at backlog-read time with metric increment (`service/matching/task_reader.go:254-260`), implementing schedule-to-start timeout semantics.
- **DB read failures retried with backoff** and resource-exhaustion-specific delays (`service/matching/task_reader.go:160-167,299-312`); history reader pauses slices on repeated failure (`service/history/queues/reader.go:456-463`).
- **Busy workflow contention**: tasks are nacked and funneled into strictly sequential per-execution queues rather than hammering the workflow lock (`common/tasks/execution_aware_scheduler.go:26-36`; `service/history/queues/scheduler.go:218-223`).
- **Forwarding failures degrade gracefully**: forwarded backlog tasks block locally at root until timeout instead of erroring (`service/matching/matcher.go:158-168`; `pri_matcher.go:425-443`); query polls return `errNoRecentPoller` only when no poller seen recently (`pri_matcher.go:467-477`).
- **Sticky-worker loss** surfaces `StickyWorkerUnavailable` and sticky queues refuse loading without pollers (`service/matching/matching_engine.go:597-603`).
- **Versioning redirect churn**: spooled tasks re-resolve routing each dispatch attempt and may be re-spooled to a different build-ID backlog (`service/matching/task_queue_partition_manager.go:906-951`).
- **Namespace handover** blocks transfer-task execution mid-flight (`service/history/transfer_queue_active_task_executor.go:143-153`).
- **Executor panics** recovered and converted to retryable errors; repeated unexpected errors escalate to DLQ (`service/history/queues/executable.go:326-338`, DLQ config :237-241).
- **Workflow paused** disables eager dispatch and bypasses task generation (`.../workflow_task_completed_handler.go:545-550`).

## Future Considerations

- Complete the legacy→priority matcher migration and remove transitional hacks (fairness-via-activity-ID prefix parsing marked TODO at `.../workflow_task_completed_handler.go:474`; matcher selection at `task_queue_partition_manager.go:211`).
- Move sticky-queue task-addition logic from matching to history (tracked TODO referencing go.temporal.io/server#181, `service/matching/matching_engine.go:605`).
- Include poll-forward latency in `task_dispatch_latency`, currently "excluded for now" (`service/matching/matching_engine.go:3082`).
- Replace eager-start versioning carve-outs once Versioning V3 subsumes old behaviors (TODO at `.../workflow_task_completed_handler.go:528-532`).
- Extend OTEL instrumentation into the matching dispatch path for end-to-end traces.

## Questions / Gaps

- **End-to-end total ordering**: the exact ordering guarantee delivered to workers (across priorities × fairness × partitions) is assembled from several sources (`fairness.md`, `matcher_data.go`, `config.go`) rather than stated in one place; no evidence found of a single spec document defining the composed semantics. Searched `docs/`, `service/matching/*.md`, and comparator code.
- **Matching-side tracing**: no OTEL tracer usage found anywhere under `service/matching/` (searched `trace|otel|opentelemetry` across `service/matching/*.go`); dispatch observability there rests entirely on metrics/logs. Tracers are wired only for history queue executables (`service/history/handler.go:141`, `service/history/queues/executable.go:283`).
- **Worker-side execution**: actual tool/function invocation lives in user SDKs, outside this repository; this study can only attest to server-side dispatch boundaries (poll response construction, e.g. `service/matching/matching_engine.go:720-724`).
- **Persistence-driver ordering nuances**: `fairness.md:30-32` notes Cassandra vs SQL differences in maintaining max-read-level accuracy; driver-specific behavior was not individually verified (only the schema directory was noted, not each driver implementation).

---

Generated by dimension `07.01-tool-scheduling-and-dispatch` against `temporal`.
