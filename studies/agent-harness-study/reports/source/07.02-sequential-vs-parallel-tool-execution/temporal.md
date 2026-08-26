# Source Analysis: temporal

## 07.02: Sequential vs Parallel Tool Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server: frontend/history/matching/worker services, CHASM state-machine framework) |
| Analyzed | 2026-08-26 |

> Citation note: all file paths below are relative to the source root `studies/agent-harness-study/sources/temporal`.
> "Tools" in this source map to three concrete execution units: (a) background tasks processed by history-service task schedulers (transfer/timer/outbound/archival), (b) activity and workflow tasks dispatched through the matching service to external workers, and (c) CHASM component tasks (PureTask vs SideEffectTask).

## Summary

Temporal implements a layered concurrency model in which parallelism is deliberately wide across workflows and strictly serialized within one workflow execution's state mutations. The generic scheduler framework (`common/tasks/scheduler.go:4-12`) defines a single `Scheduler[T]` interface with many composable implementations: FIFO worker pools, interleaved weighted round-robin channels per namespace/priority, group-by-key schedulers, rate-limited wrappers, dynamic worker pools, sequential schedulers, and a new "execution queue" scheduler that gives contended workflows dedicated sequential queues. Writes to any single workflow execution are funneled through a size-1 priority semaphore per workflow context (`service/history/workflow/context.go:48,116`), backed by persistence-level optimistic concurrency (RangeID fencing, `ConditionFailedError`, `ShardOwnershipLostError` in `common/persistence/data_interfaces.go:113-148`) and monotonically increasing event IDs (`service/history/workflow/mutable_state_impl.go:2483-2485`). Side effects are explicitly declared: the CHASM framework distinguishes `PureTask` (executed atomically inside a state transition) from `SideEffectTask` (validated, executed out-of-band on the outbound queue, grouped by destination, circuit-broken, and discardable on standby clusters) (`chasm/registrable_task.go:46-130`). Failure handling is per-task (Nack → backoff → reschedule → DLQ), never aggregate, and contention is converted into ordering via busy-workflow routing onto sequential queues (`service/history/queues/executable.go:706-721`). Concurrency is heavily configured via dynamic config keys with documented defaults (`common/dynamicconfig/constants.go:2134-2157`).

## Rating

**9 / 10** — Clear, explicit model ("parallel across executions, sequential within an execution"), implemented through well-tested reusable components (`TestSequentialExecution_SameWorkflow`, `common/tasks/execution_queue_scheduler_test.go:116-150`; priority-semaphore suite, `common/locks/priority_semaphore_test.go:53-215`), observable via dedicated metrics (`common/metrics/metric_defs.go:1218-1225`), operationally tunable via dynamic config, and hardened against real failure modes (RangeID fencing, task abort on shutdown, TTL-expiry race regression test `common/tasks/execution_queue_scheduler_test.go:373`). It falls short of 10 because the per-workflow sequential scheduling feature (ExecutionQueueScheduler) is disabled by default (`history.taskSchedulerEnableExecutionQueueScheduler = false`, `common/dynamicconfig/constants.go:2134-2139`), its default per-queue concurrency is 2 rather than strictly 1 (safe only because the correctness boundary is the size-1 workflow lock, not the scheduler), and cross-partition dispatch ordering in matching is best-effort by the code's own admission ("To ensure better dispatch ordering...", `service/matching/matcher.go:110`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic scheduler interface | `Scheduler[T]` interface: Submit/TrySubmit/Start/Stop | common/tasks/scheduler.go:4-12 |
| Parallel base pool | FIFO scheduler with dynamic `WorkerCount` feeding all history task processing | service/history/queues/scheduler.go:146-154 |
| Fair multi-tenant parallelism | InterleavedWeightedRoundRobinScheduler over `{NamespaceID, Priority}` channels | service/history/queues/scheduler.go:166-175; common/tasks/interleaved_weighted_round_robin.go |
| Per-execution serialization | ExecutionAwareScheduler routes contended tasks to per-workflow `executionQueueScheduler` | common/tasks/execution_aware_scheduler.go:28-42,84-113 |
| Sequential-per-execution mechanics | Queue keyed by `(namespaceID, workflowID, runID)`; workers pop under mutex; idle sweeper | common/tasks/execution_queue_scheduler.go:28-62,156-213,317-324 |
| Contention signal | `ErrResourceExhaustedBusyWorkflow` ("Workflow is busy.", cause BUSY_WORKFLOW) | service/history/consts/const.go:102-107 |
| Busy routing on Nack | On busy error, executable routed to sequential queue via `HandleBusyWorkflow` | service/history/queues/executable.go:706-721 |
| Busy raised at lock time | Lock acquisition timeout converted to busy-workflow error | service/history/workflow/cache/cache.go:327-357; service/history/statemachine_environment.go:96-99 |
| Workflow write lock | Each workflow context holds `locks.NewPrioritySemaphore(1)`; `Lock`/`Unlock` acquire/release weight 1 | service/history/workflow/context.go:48,116,128-137 |
| Semaphore primitive | Priority semaphore with high/low wait lists, TryAcquire, context-aware Acquire + tests | common/locks/priority_semaphore_impl.go:80-170; common/locks/priority_semaphore_test.go:121-147 |
| Persistence conflict detection | `CurrentWorkflowConditionFailedError`, `WorkflowConditionFailedError`, `ConditionFailedError`, `ShardOwnershipLostError` | common/persistence/data_interfaces.go:113-148,1377-1409 |
| RangeID fencing | Conditional updates stamped with shard RangeID; mismatch → ConditionFailedError (Cassandra LWT) | common/persistence/cassandra/shard_store.go:126-151; common/persistence/cassandra/matching_task_store_v2.go:92-123 |
| Deterministic result order | Event IDs assigned from single history builder cursor `GetNextEventID()` | service/history/workflow/mutable_state_impl.go:2483-2485,995 |
| Duplicate/duplicate-completion fencing | `workflowTaskIdentity{schedID, attempt, version}`; stale completions → `ErrWorkflowTaskNotFound` | service/history/workflow/context.go:61-66,173-203; service/history/consts/const.go:94-95 |
| Side-effect declaration (CHASM) | `NewRegistrableSideEffectTask` vs `NewRegistrablePureTask`: separate Validate/Execute/Discard paths | chasm/registrable_task.go:46-130 |
| Task singleton conflicts | `WithSingletonTask(Replace|Ignore)` caps one outstanding task of a type per component | chasm/registrable_task.go:210-222 |
| Destination grouping of side effects | Outbound queue groups side-effect tasks by `(namespace, destination)` via GroupByScheduler | service/history/outbound_queue_factory.go:108-177; chasm/registrable_task.go:200-208 |
| Group concurrency limiter | Per-group DynamicWorkerPoolScheduler with dynamic BufferSize/Concurrency | common/tasks/dynamic_worker_pool_scheduler.go:12-61; service/history/outbound_queue_factory.go:41-77 |
| Circuit breaker per group | `CircuitBreakerExecutable` wrapping each outbound task with per-group gobreaker | service/history/outbound_queue_factory.go:141-152; service/history/circuitbreakerpool/fx.go:30 |
| Activity failure isolation | Failed activities produce independent `ActivityRetryTimerTask`s, not blocking siblings | service/history/tasks/activity_retry_timer.go:13-27 |
| Dispatch rate limiting | Per-task-queue rate limiter gates every sync-match offer; tokens recycled for invalid tasks | service/matching/matcher.go:36-37,108-128 |
| Workflow-start fencing in matching | RecordWorkflowTaskStarted rate-limited then verified by history (attempt/stamp) | service/matching/matching_engine.go:3445-3479 |
| Shard acquisition bounded concurrency | `semaphore.NewWeighted(AcquireShardConcurrency)` bounds parallel shard acquisition | service/history/shard/controller_impl.go:435-449 |
| Bounded per-run replication parallelism | Low-priority replication hashes RunID into P slots → P sequential queues per run max | service/history/replication/fx.go:186-209 |
| Sequential scheduler (replication) | SequentialScheduler keyed by WorkflowKeyHashFn, panic-wrapped executor | service/history/replication/fx.go:144-154; common/tasks/sequential_scheduler.go:16-49 |
| Config: EQS defaults | Enable=false, MaxQueues=500, QueueTTL=5s, QueueConcurrency=2 (≤0 capped to 1) | common/dynamicconfig/constants.go:2134-2157 |
| Config: worker counts | timerProcessorSchedulerWorkerCount, transferProcessorSchedulerWorkerCount, etc. | common/dynamicconfig/constants.go:2164-2256; service/history/configs/config.go:151-170 |
| Config: shard IO concurrency | `shardIOConcurrency` forced to 1 under Cassandra | common/dynamicconfig/constants.go:1991-1995; service/history/shard/context_impl.go:2186-2193 |
| Config: outbound group limiter | `history.outboundQueue.groupLimiter.concurrency` per namespace+destination | common/dynamicconfig/constants.go:2376-2380 |
| Tests: sequential vs parallel | Same-workflow tasks execute in submission order; different workflows interleave concurrently | common/tasks/execution_queue_scheduler_test.go:116-198 |
| Tests: busy-workflow routing | Nack(busy) triggers HandleBusyWorkflow; fallbacks to reschedule when disabled/full | service/history/queues/executable_test.go:1362-1434 |
| Tests: lock behavior | Cache lock tests incl. deadline-exceeded → busy error path | service/history/workflow/cache/cache_test.go:645-649 |
| Tests: parallel activities end-to-end | Integration test runs batches of parallel activities across build IDs with random failures; asserts unordered event timestamps are tolerated | tests/versioning_test.go:2098-2120 |

## Answers to Dimension Questions

1. **Can multiple tools run in one turn?**
   Yes — at two levels. A single workflow-task completion may carry many commands which the server applies sequentially inside one transition (events appended from one builder cursor, `service/history/workflow/mutable_state_impl.go:995,2483-2485`); scheduling several activities in that turn puts them all in flight simultaneously, since each becomes an independent task handed to workers via the matching service with no cross-activity lock (`service/matching/matcher.go:108-128`). Server-side background tasks from *different* workflow executions always run in parallel (FIFO worker pool + weighted round-robin channels, `service/history/queues/scheduler.go:151-175`). Tasks belonging to the *same* execution run in parallel by default too, but are pulled onto a dedicated sequential queue once the execution reports contention (`common/tasks/execution_aware_scheduler.go:28-42`).

2. **Which tools are safe to parallelize?**
   The server makes this distinction first-class. CHASM `PureTask`s are safe to fire inside a state transition because they mutate component state atomically with it; `SideEffectTask`s (Nexus invocation/cancellation, callback invocation, activity dispatch, scheduler callbacks — e.g., `chasm/lib/nexusoperation/library.go:111-125`, `chasm/lib/activity/activity_tasks.go:22`) must go through the outbound queue where they are validated before execution, grouped per destination, rate-limited, and circuit-broken (`chasm/registrable_task.go:46-86`; `service/history/outbound_queue_factory.go:89-177`). User-code tools (activities) are always safe to parallelize from the server's viewpoint — isolation comes from the event-sourced state machine, not from locking.

3. **Are write tools serialized?**
   Yes, per workflow execution, absolutely. All mutations of one execution pass through `ContextImpl.lock`, a `PrioritySemaphore` of total size 1 (`service/history/workflow/context.go:116,128-137`); API callers take it with `PriorityHigh`, background tasks with `PriorityLow` (`service/history/statemachine_environment.go:57`, `service/history/workflow/cache/cache.go:40-57`). If the lock cannot be acquired before deadline the caller gets `ErrResourceExhaustedBusyWorkflow` (`cache/cache.go:351-355`) and the task is re-routed to a sequential per-workflow queue (`executable.go:706-721`). Even if two hosts raced, persistence-level conditional writes stamped with the shard `RangeID` fail with `ConditionFailedError`/`ShardOwnershipLostError` (`common/persistence/data_interfaces.go:138-148`), and duplicate workflow-task completions are fenced by scheduled-event-ID/attempt/version identity (`context.go:61-66,222-226`). Across *different* workflows, writes are fully parallel.

4. **How are failures aggregated?**
   They are not aggregated — each tool fails independently and is handled individually. A failing task is Nack'd with metrics, optionally resubmitted immediately, or rescheduled with error-specific backoff (`service/history/queues/executable.go:706-750`); exhausted/unexpected errors can be parked in a DLQ (`service/history/outbound_queue_factory.go:273-277`). Side-effect groups isolate blast radius with a dedicated circuit breaker per `(namespace, destination)` (`outbound_queue_factory.go:143-147`). Activity failures surface to the workflow as retry-timer tasks (`service/history/tasks/activity_retry_timer.go:14-22`) so sibling activities continue unaffected — exercised end-to-end by `tests/versioning_test.go:2102-2120` (random activity failure rates amid parallel batches). Scheduler shutdown aborts pending tasks rather than executing them (`common/tasks/execution_queue_scheduler.go:182-194`).

5. **Is result order deterministic?**
   Within one workflow execution, yes: strictly increasing event IDs define a total order (`mutable_state_impl.go:2483-2485`), the per-execution lock serializes transitions, and the execution-queue scheduler preserves submission order per workflow key — asserted directly by `TestSequentialExecution_SameWorkflow` (`common/tasks/execution_queue_scheduler_test.go:145-149`). Across executions there is intentionally no global order; even matching only promises heuristic dispatch ordering while backlog exists ("To ensure better dispatch ordering...", `service/matching/matcher.go:109-115`), and the integration test explicitly tolerates history events whose timestamps are unordered due to parallel activities (`tests/versioning_test.go:2120`).

## Architectural Decisions

- **Correctness at the state layer, not the scheduler layer.** Serialization of writes is enforced by the size-1 priority semaphore plus RangeID-fenced conditional persistence, not by hoping schedulers behave. This lets the ExecutionAwareScheduler default to parallel-with-fallback-to-sequential without risking corruption (`service/history/workflow/context.go:116`; `common/persistence/data_interfaces.go:113-148`).
- **Contention as a routing signal.** Instead of blocking or dropping, a busy-workflow error promotes the affected workflow into a dedicated sequential queue with TTL'd lifecycle, capped queue count, and fallback to the base scheduler at capacity (`common/tasks/execution_queue_scheduler.go:121-151`; `service/history/queues/executable.go:706-721`).
- **Declared side effects with a typed pipeline.** PureTask/SideEffectTask registration forces authors to declare effectfulness up front; side effects get validation-before-execute, standby-discard semantics, destination grouping, per-group rate limits and circuit breakers (`chasm/registrable_task.go:46-130`; `service/history/outbound_queue_factory.go:89-186`).
- **Fair-share parallelism across tenants.** Namespace+priority weighted round-robin prevents one hot namespace from monopolizing workers while keeping intra-namespace throughput high (`service/history/queues/scheduler.go:105-175`).
- **Everything tunable at runtime.** Worker counts, queue caps, concurrency, TTLs, rate limits, and feature flags are dynamic-config properties with documented defaults, allowing operators to trade latency for safety live (`common/dynamicconfig/constants.go:2134-2157,2376-2380`).

## Notable Patterns

- **Generic composable scheduler algebra** (`Scheduler[T]` wrappers): RateLimited → InterleavedWeightedRoundRobin → ExecutionAware → FIFO, each adding one orthogonal policy (`service/history/queues/scheduler.go:146-184`).
- **Priority semaphore as reader/writer-ish gate**: API requests (high) preempt background tasks (low) waiting on the same workflow lock (`common/locks/priority_semaphore_impl.go:190-220`; `service/history/workflow/cache/cache.go:334-349`).
- **Bounded fan-out via hashing**: low-priority replication maps each RunID to one of P slots, giving deterministic per-run serialization with a hard cap of P concurrent lanes per run (`service/history/replication/fx.go:186-203`).
- **Lifecycle hygiene**: idle queue sweeping (`execution_queue_scheduler.go:216-251`), token recycling for unmatched dispatches (`matcher.go:75-77,118-128`), abort-on-shutdown semantics (`execution_queue_scheduler.go:182-194`).
- **Race-conscious testing**: regression test proving TTL expiry cannot orphan tasks submitted concurrently (`common/tasks/execution_queue_scheduler_test.go:373-379`).

## Tradeoffs

- **Latency vs strictness**: per-queue `QueueConcurrency` defaults to 2, allowing two concurrent tasks of one workflow; this improves tail latency but means the scheduler itself is not strictly FIFO-by-one — safety relies entirely on the underlying lock+fencing stack (`common/dynamicconfig/constants.go:2152-2157`).
- **Opt-in ordering optimization**: the sequential-queue machinery ships disabled by default; clusters that never enable it absorb repeated busy-workflow lock timeouts and rescheduling churn instead (`constants.go:2134-2139`).
- **Global throughput vs per-queue caps**: capping queues at 500 and falling back to the shared FIFO keeps memory bounded but reintroduces contention exactly for the hottest workflows (`execution_queue_scheduler.go:141-146`).
- **Dispatch ordering heuristics**: blocking sync match during backlog improves ordering but adds latency to forwarded/spooled tasks; acknowledged as approximate (`matcher.go:109-116`).
- **Expressiveness vs determinism**: declaring tasks pure vs side-effectful buys aggressive in-transition batching but pushes complexity into validation/discard logic that authors must implement correctly (`chasm/registrable_task.go:46-130`).

## Failure Modes / Edge Cases

- **Lock-holder crash mid-transition**: release func clears dirty contexts and panics loudly rather than persisting half a transaction (`service/history/workflow/cache/cache.go:369-399`).
- **Stale paginated task completion**: buffered pages are dropped if the started-task identity changed (timeout/retry/close), preventing zombie completions from applying (`context.go:191-233`).
- **Shard ownership races**: RangeID mismatch yields `ShardOwnershipLostError`, invalidating in-flight work instead of double-applying it (`common/persistence/data_interfaces.go:148`; `cassandra/shard_store.go:126-151`).
- **Queue capacity exhaustion**: new workflows fall back to base scheduler and metric `execution_queue_scheduler_submit_rejected` records it (`execution_queue_scheduler.go:142-146`; `metric_defs.go:1223`).
- **Destination outage**: per-group circuit breaker trips, halting that destination's side effects without starving other destinations (`outbound_queue_factory.go:141-152`).
- **Cassandra constraint**: `shardIOConcurrency > 1` is silently coerced to 1 with a warning — a documented incompatibility rather than silent corruption (`service/history/shard/context_impl.go:2186-2193`).

## Future Considerations

- Promote `history.taskSchedulerEnableExecutionQueueScheduler` to default-on once soak data exists; today the sequential path is effectively a shadow feature.
- Replace the fixed `MaxQueues=500` restart-gated knob (`configs/config.go:144-147`) with adaptive resizing; hot-workload clusters either thrash the fallback or waste memory.
- Tighten matching's "better dispatch ordering" heuristic into a measurable guarantee (e.g., backlog-age-aware offers) if cross-partition ordering matters for spooled tasks.
- Extend the PureTask/SideEffectTask declaration pattern beyond CHASM so all task categories carry effect metadata, enabling uniform scheduling policy selection.

## Questions / Gaps

- **Worker-side concurrency limits** (e.g., SDK `MaxConcurrentActivityExecutionSize`) live in the separate Go SDK repo, not here; this server-side study found only the worker-scanner/parent-close internal worker settings (`common/dynamicconfig/constants.go:3421-3448,3530-3548`). No evidence of server-enforced per-worker concurrency caps beyond dispatch RPS was searched for beyond matching rate limiters.
- No dedicated documentation page describes the execution-queue scheduler; understanding derives solely from code comments and tests (`common/tasks/execution_queue_scheduler.go:34-42`). Docs directory was not exhaustively searched for design docs on this feature.
- Whether `QueueConcurrency=2` can reorder *observable* side effects (two outbound tasks of one workflow running concurrently) is guarded only by per-destination grouping/rate limiting, not per-workflow ordering; no test was found asserting side-effect order within one workflow (search boundary: `common/tasks/*_test.go`, `service/history/queues/*_test.go`).

---

Generated by `07.02-sequential-vs-parallel-tool-execution` against `temporal`.
