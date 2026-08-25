# Source Analysis: temporal

## 01.07: Concurrency and Parallel Advancement

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `sources/temporal` |
| Language / Stack | Go (server: `go.temporal.io/server`, Go 1.26.4) |
| Analyzed | 2026-07-03 |

## Summary

The Temporal server is a multi-service Go application (frontend, history, matching, worker) plus the emerging Chasm framework for state machines. Concurrency is layered and consistent:

- **Intra-run parallelism is intentionally narrow.** Most state-mutating code paths serialize on a per-workflow `PrioritySemaphore(1)` (single-tenant lock) held inside the workflow cache (`service/history/workflow/cache/cache.go:305-348` and `service/history/workflow/context.go:43,73-93`). The server explicitly trades intra-workflow concurrency for ordering simplicity.
- **Inter-workflow / inter-run parallelism is the default.** Shard acquisition uses `golang.org/x/sync/semaphore`-bounded goroutines (`service/history/shard/controller_impl.go:443-457`), replication shards use per-RunID bucketing to produce P parallel sequential queues (`service/history/replication/fx.go:186-205`), task readers are a pool (`service/history/queues/reader.go`, `service/history/queues/reader_group.go:20-28`), and the matching service fans out across task-queue partitions.
- **Bounded concurrency primitives** are first-class and named: `goro.Group` (`common/goro/group.go:14-20`), `goro.AdaptivePool` (`common/goro/adaptive_pool.go:14-26`), `goro.KeyedSet` (`common/goro/keyed_set.go:9-14`), `goro.Handle` (`common/goro/goro.go:8-17`), `locks.PrioritySemaphore` (`common/locks/priority_semaphore.go:5-11` / `common/locks/priority_semaphore_impl.go:46-52`), `locks.PriorityMutex` (`common/locks/priority_mutex.go:7-26`), `locks.IDMutex` (`common/locks/id_mutex.go:7-39`), `locks.ConditionVariable` (`common/locks/condition_variable.go:3-15`), and the project-wide future abstraction `common/future.FutureImpl[T]` (`common/future/future_impl.go:22-29`).
- **Task scheduling** is split between two reusable schedulers in `common/tasks`: `FIFOScheduler[T]` (`common/tasks/fifo_scheduler.go:24-37`), `SequentialScheduler[T]` (`common/tasks/sequential_scheduler.go:28-47`) which provides per-key sequential queues executed by N workers, and `DynamicWorkerPoolScheduler` (`common/tasks/dynamic_worker_pool_scheduler.go:25-42`) used when concurrency and buffer size must adapt dynamically.
- **Within the Chasm framework, parallelism is modeled through the task system**, not through user goroutines. The Invoker task handler (`chasm/lib/scheduler/invoker_tasks.go`) is the only chasm-layer code that explicitly fans out goroutines: `cancelWorkflows`, `terminateWorkflows`, and `startWorkflows` each allocate their own `sync.WaitGroup` and result mutex and call `wg.Go(...)` for each target (`chasm/lib/scheduler/invoker_tasks.go:260-407`). Per-task concurrency is bounded by `invokerTaskHandlerContext.takeNextAction` (`chasm/lib/scheduler/invoker_tasks.go:250-258`). The scheduler framework itself (`chasm/scheduler.go`, `chasm/engine.go`) is strictly serial per execution key.
- **Failure collection.** Concurrent code in Temporal relies heavily on `sync.WaitGroup` + a result mutex; `golang.org/x/sync/errgroup` is used only in tests (`service/history/queues/dlq_writer_test.go:26,166,246`) and a tooling script (`tools/flakereport/bisect.go:12,483`). `errors.Join` is used where multi-branch errors need to be aggregated (`common/persistence/visibility/visibility_manager_dual.go:225-246`). Documented guidance in `common/goro/package.go:9-12` explicitly directs developers to use `errgroup.Group` for short-lived batches and `goro.Group` for long-lived background goroutines.
- **Ordering guarantees** are achieved by serializing per execution key. The SequentialScheduler enforces "1 queue = 1 worker" via channel ownership (`common/tasks/sequential_scheduler.go:283-307`). The Shard's IO semaphore and workflow lock enforce per-shard/per-workflow serialization (`service/history/shard/context_impl.go:246-255,117-126,1525-1548`).
- **Lock contention is mitigated** by sharding: `ShardedConcurrentTxMap` (32 shards) (`common/collection/concurrent_tx_map.go:13-46`), `IDMutex` with N shards (`common/locks/id_mutex.go:42-55`), and the DLQ writer's per-queue mutex map (`service/history/queues/dlq_writer.go:26,147-155`).

The model answers "yes" to the dimension's lead question: read-only work runs in parallel freely (multi-reader pools, semaphore-bounded shard acquisition, fan-out in update/shutdown paths), while stateful per-workflow work is funneled through a single-writer lock so that race conditions cannot occur.

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:

- Concurrency is a first-class design concern with named, reusable abstractions (`goro`, `locks`, `future`, `tasks` schedulers) and dedicated test files (`common/goro/*_test.go`, `common/locks/*_test.go`, `common/future/future_test.go`).
- Ordering, partial-success, and lock-step semantics are explicit and consistently enforced (per-workflow semaphore + shard IO semaphore + sequential-scheduler-per-key + chasm task model).
- Operational safeguards include a deadlock detector (`common/deadlock/deadlock.go:82-100`) backed by `AdaptivePool`, context cancellation propagated through every primitive, and `AwaitWaitGroup` with a 1-minute timeout (`common/tasks/fifo_scheduler.go:85`, `common/tasks/sequential_scheduler.go:100`).
- It is not a 9-10 because:
  1. The codebase is inconsistent about whether to use `errgroup`, raw `sync.WaitGroup`, or sharded goroutines — each subsystem invents its own fan-out. There is no shared "fan-out N tasks, collect errors, bound by semaphore" utility.
  2. The chasm tree (`chasm/tree.go`, `chasm/statemachine.go`) is single-threaded by design; parallel advancement within one execution requires hand-written goroutine pools in handlers (e.g., `invoker_tasks.go`).
  3. Lock acquisition paths have nuanced ordering rules (e.g., "DO NOT try to acquire ioSemaphore while holding rwLock" — `service/history/shard/context_impl.go:122`) that are only documented in comments, not enforced by lint or types.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goroutine lifecycle helper (long-running) | `goro.Group` spawns/cancels/waits; uses `sync.Once` for init; ignores errors | `sources/temporal/common/goro/group.go:14-54` |
| Goroutine lifecycle helper (single) | `goro.Handle` is thread-safe, multi-stop-safe; safe even when goroutine calls `Done()` on itself | `sources/temporal/common/goro/goro.go:8-71` |
| Adaptive pool with growth/shrink | Workers added when `targetDelay` elapses; shrink uses jittered `shrinkFactor * rand.Float64()` | `sources/temporal/common/goro/adaptive_pool.go:14-138` |
| Keyed set of goroutines | Cancels obsolete goroutines and starts new ones to match `target` set | `sources/temporal/common/goro/keyed_set.go:9-53` |
| Package guidance: errgroup vs goro | Package doc points transient goroutines to `errgroup.Group`; long-running to `Group` | `sources/temporal/common/goro/package.go:1-12` |
| Priority semaphore (weighted) | `Acquire/TryAcquire/Release` with `PriorityHigh/Low` waiter lists; bounded context | `sources/temporal/common/locks/priority_semaphore_impl.go:46-228` |
| Priority mutex | LockHigh waits out LockLow holders; LockLow waits out LockHigh | `sources/temporal/common/locks/priority_mutex_impl.go:31-110` |
| ID-keyed sharded mutex | Sharded by `HashFunc % numShard`; per-ID refcounted | `sources/temporal/common/locks/id_mutex.go:7-102` |
| Condition variable primitive | Signal/Broadcast/Wait with interrupt channel | `sources/temporal/common/locks/condition_variable.go:3-15` |
| Future abstraction (generic) | `Future[T]` interface, `FutureImpl[T]` with atomic state machine `pending→setting→ready`, `SetIfNotReady` and `Get(ctx)` | `sources/temporal/common/future/future.go:5-10`, `sources/temporal/common/future/future_impl.go:22-108` |
| Future is used as workflow update signaling | Update carries two futures: `accepted` and `outcome` | `sources/temporal/service/history/workflow/update/update.go:60-77,90-100` |
| Sharded concurrent map | 32 shards; allows up to 32 concurrent writers via per-shard `sync.RWMutex` | `sources/temporal/common/collection/concurrent_tx_map.go:8-100` |
| FIFO scheduler with dynamic worker pool | Workers started via `sync.WaitGroup`; shutdown bounded by `AwaitWaitGroup` 1m | `sources/temporal/common/tasks/fifo_scheduler.go:24-227` |
| Sequential scheduler (per-key queues) | Tasks hashed into queues; one queue per worker; ownership enforced by channel | `sources/temporal/common/tasks/sequential_scheduler.go:28-372` |
| Sequential task queue contract | `Add`/`Remove`/`IsEmpty`/`Len`/`ID` | `sources/temporal/common/tasks/sequential_task_queue.go:3-18` |
| Dynamic worker pool with bounded concurrency | Concurrency limiter and buffer size limiter; goroutines stop on idle | `sources/temporal/common/tasks/dynamic_worker_pool_scheduler.go:13-175` |
| Rate limiter (multi-stage) | `MultiRateLimiterImpl` reserves across many stages and cancels on partial failure | `sources/temporal/common/quotas/multi_rate_limiter_impl.go:16-235` |
| Single rate limiter (lock-protected) | `RateLimiterImpl` uses `sync.RWMutex` to swap RPS/burst atomically | `sources/temporal/common/quotas/rate_limiter_impl.go:11-115` |
| History shard acquisition | `golang.org/x/sync/semaphore.NewWeighted(concurrency)` for `AcquireShardConcurrency` | `sources/temporal/service/history/shard/controller_impl.go:443-457` |
| Shard IO semaphore (per shard) | `locks.PrioritySemaphore` acquired before every persistence call | `sources/temporal/service/history/shard/context_impl.go:117-126,246-255,1525-1548,2108` |
| Shard readiness check parallelism | `sem := semaphore.NewWeighted(concurrency)` for parallel readiness probes | `sources/temporal/service/history/shard/controller_impl.go:484-499` |
| Workflow lock | `locks.NewPrioritySemaphore(1)` — only one writer per workflow at a time | `sources/temporal/service/history/workflow/context.go:43,73-93` |
| Workflow cache lock acquisition | Cache enforces `Lock(ctx, lockPriority)` with timeout trimming for non-API callers | `sources/temporal/service/history/workflow/cache/cache.go:305-348` |
| Sequential scheduler used for replication | `NewSequentialScheduler` with `ReplicationProcessorSchedulerWorkerCount` workers | `sources/temporal/service/history/replication/fx.go:146-154` |
| Replication low-priority parallelism | RunID-bucketed slots yield P parallel queues for one workflow's low-priority tasks | `sources/temporal/service/history/replication/fx.go:177-217` |
| Replication task fetcher parallelism | `numWorker := config.ReplicationTaskFetcherParallelism()` | `sources/temporal/service/history/replication/task_fetcher.go:196` |
| Handler parallel token fetch | `GetReplicationMessages` fans out across shards via `sync.WaitGroup` + `sync.Map`; `GetDLQReplicationMessages` collects into a channel | `sources/temporal/service/history/handler.go:1489-1535,1557-1606` |
| DLQ writer per-queue serialization | `sync.Map` of `*sync.Mutex` per `persistence.QueueKey` | `sources/temporal/service/history/queues/dlq_writer.go:20-155` |
| DLQ writer concurrency tests | Uses `errgroup.Group` to assert serialization per queue, parallel across queues | `sources/temporal/service/history/queues/dlq_writer_test.go:160-296` |
| Frontend ShutdownWorker fan-out | Best-effort parallel poll cancel + heartbeat, then required unload | `sources/temporal/service/frontend/workflow_handler.go:3004-3042` |
| Frontend poll cancellation fan-out | One `waitGroup.Go` per partition, with `atomic.Int32` counters | `sources/temporal/service/frontend/workflow_handler.go:3092-3135` |
| Chasm Invoker per-target parallel cancel/terminate/start | Each function builds its own `sync.WaitGroup` + `resultMutex`; per-task concurrency bounded by `takeNextAction` | `sources/temporal/chasm/lib/scheduler/invoker_tasks.go:250-407` |
| Chasm invoker action cap | `takeNextAction` increments `actionsTaken`; only first `maxActions` actions spawn goroutines | `sources/temporal/chasm/lib/scheduler/invoker_tasks.go:250-258` |
| Dual visibility manager | `dualWriteWrapper` fans writes across configured stores, joins errors | `sources/temporal/common/persistence/visibility/visibility_manager_dual.go:225-246` |
| Notifier pub/sub map | `collection.ConcurrentTxMap` keyed by workflowKey for parallel subscribers | `sources/temporal/service/history/events/notifier.go:43-95` |
| Scanner goroutines | `sync.WaitGroup` collects scanner workflow runners; not error-grouped | `sources/temporal/service/worker/scanner/scanner.go:105-219` |
| Deadlock detector adaptive pool | `goro.NewAdaptivePool` per root; panic-capable if a deadlock is confirmed | `sources/temporal/common/deadlock/deadlock.go:82-128` |
| Atomic status transitions in daemons | `atomic.CompareAndSwapInt32` for `Initialized→Started→Stopped` lifecycle | `sources/temporal/common/tasks/sequential_scheduler.go:69-92`, `common/tasks/fifo_scheduler.go:55-87` |
| Engine future for shard lazy load | `engineFuture *future.FutureImpl[historyi.Engine]` returned from `GetEngine` | `sources/temporal/service/history/shard/context_impl.go:96,2106` |
| Initial-shard readiness signal | `initialShardsAcquired *future.FutureImpl[struct{}]` gates startup readiness | `sources/temporal/service/history/shard/controller_impl.go:58,99` |
| Matchingservice backlog initialised-error future | Used to surface delayed init errors to consumers | `sources/temporal/service/matching/backlog_manager.go:71,98`, `fair_backlog_manager.go:47,85`, `pri_backlog_manager.go:64,98` |
| Reader group concurrency control | `ReaderGroup` owns a `sync.Mutex` to start/stop multiple readers in lockstep | `sources/temporal/service/history/queues/reader_group.go:20-155` |
| Reader internal mutex | `ReaderImpl` uses `sync.Mutex` for slice manipulation and a separate `shutdownWG` | `sources/temporal/service/history/queues/reader.go:58-100` |
| Conditional variable order | Code comment explicitly forbids acquiring ioSemaphore while holding rwLock | `sources/temporal/service/history/shard/context_impl.go:117-126` |
| errgroup usage outside tests/tools | None found in production code | `sources/temporal/tools/flakereport/bisect.go:12,483`, `service/history/queues/dlq_writer_test.go:26,166,246` |

## Answers to Dimension Questions

1. **What can run in parallel?**
   - Shards within one history host (`service/history/shard/controller_impl.go:443-457`).
   - Shard readiness probes (`controller_impl.go:484-499`).
   - Tasks for different execution keys (RunIDs) in the SequentialScheduler (`common/tasks/sequential_scheduler.go:107-141`).
   - Replication stream tasks bucketed into P slots per workflow (`service/history/replication/fx.go:186-217`).
   - Workflow Update `accepted` and `outcome` futures (`service/history/workflow/update/update.go:60-77`).
   - Schedule invoker actions: cancels, terminates, starts per workflow run (`chasm/lib/scheduler/invoker_tasks.go:260-407`).
   - Per-shard replication token fetches and DLQ replication message retrieval (`service/history/handler.go:1489-1535,1557-1606`).
   - Multiple visibility stores when dual-write is configured (`common/persistence/visibility/visibility_manager_dual.go:225-246`).
   - Multiple matching-service partitions during worker shutdown (`service/frontend/workflow_handler.go:3092-3135`).
   - Deadlock-detection ping workers via AdaptivePool (`common/deadlock/deadlock.go:82-128`).
   - Background scanners, schedulers, DB metric reporters (scanner.go:173-269, sqlplugin/db_metrics.go:41-87).

2. **What must be serialized?**
   - All state-mutating operations on a single workflow execution (single-writer lock via `locks.NewPrioritySemaphore(1)`) — `service/history/workflow/context.go:43,73-93`.
   - Persistence writes per shard (ioSemaphore) — `service/history/shard/context_impl.go:117-126,1525-1548`.
   - Per-task-queue updates in the SequentialScheduler — `common/tasks/sequential_scheduler.go:283-307`.
   - DLQ writes per `QueueKey` — `service/history/queues/dlq_writer.go:86-100,147-155`.
   - Per-ID lock acquisition in `IDMutex` — `common/locks/id_mutex.go:64-99`.
   - Chasm task execution per execution key (engine is single-writer by design) — `chasm/engine.go:16-59`, `chasm/task_handler_base.go`.
   - Workflow cache `Put`/`Release` (single sharded LRU) — `service/history/workflow/cache/cache.go:295-302`.

3. **How are shared resources protected?**
   - Per-workflow writer lock (`PrioritySemaphore(1)`) wrapping every cache acquisition (`cache.go:305-348`).
   - Per-shard IO semaphore (`PrioritySemaphore` with `PriorityHigh`) before each persistence call (`shard/context_impl.go:246-255,1525-1548`).
   - Per-task-queue sequential scheduler (`SequentialScheduler`) — `common/tasks/sequential_scheduler.go:107-141`.
   - Sharded locks: `ShardedConcurrentTxMap` (32 shards) — `common/collection/concurrent_tx_map.go:8-46`; `IDMutex` (configurable shards) — `common/locks/id_mutex.go:42-55`.
   - Per-queue mutex map (`sync.Map` → `*sync.Mutex`) — `service/history/queues/dlq_writer.go:147-155`.
   - Condition variables for high/low priority mutex (no busy-waiting) — `common/locks/priority_mutex_impl.go:46-88`.
   - ReaderGroup uses a `sync.Mutex` to start/stop all readers atomically — `service/history/queues/reader_group.go:20-155`.
   - `Atomic` status transitions on all long-running daemons — `common/tasks/fifo_scheduler.go:55-87`, `common/tasks/sequential_scheduler.go:69-92`.

4. **Are results ordered deterministically?**
   - Per-execution-key ordering is deterministic because the SequentialScheduler and the workflow lock guarantee a single in-flight task per key at any moment (`common/tasks/sequential_scheduler.go:283-307`).
   - Per-shard ordering is deterministic because every persistence request acquires the shard IO semaphore (`shard/context_impl.go:246-255`).
   - When multiple goroutines collect results, ordering is non-deterministic (the invoker's `cancelWorkflows`, `terminateWorkflows`, `startWorkflows` append to slices in completion order — `invoker_tasks.go:281-407`). This is acceptable because each action targets a different workflow.
   - Shard acquisition order is randomized (`c.config.NumberOfShards` shuffled via `randomStartOffset := rand.Int31n(numShards)`) to spread load — `controller_impl.go:446`.

5. **What happens when one parallel branch fails?**
   - Invoker captures the failure per goroutine, increments an error metric, and still appends the workflow to `CompletedCancels` / `CompletedTerminates` / `CompletedStarts` (best-effort semantics) — `invoker_tasks.go:281-292,318-328,373-402`.
   - `dualWriteWrapper` aggregates errors with `errors.Join` so the caller sees every failure — `visibility_manager_dual.go:225-246`.
   - SequentialScheduler retries via `backoff.ThrottleRetry`; permanent failures are `Nack`ed and DLQ-routed — `common/tasks/sequential_scheduler.go:310-345`.
   - DLQ writer failures return `ErrSendTaskToDLQ` while still emitting metrics; concurrent writes to the same queue are serialized by the per-queue mutex — `service/history/queues/dlq_writer.go:64-143`.
   - `ShutdownWorker` distinguishes best-effort (poll cancel, heartbeat) from required (unload) — `service/frontend/workflow_handler.go:3004-3042`.
   - No single-branch failure aborts the parallel batch; failures are isolated per goroutine.

## Architectural Decisions

- **Per-workflow single-writer lock.** Every state mutation on a workflow goes through `ContextImpl.Lock` (`workflow/context.go:84-89`). This is the single most consequential decision: it makes every workflow effectively single-threaded so the persistence layer doesn't need optimistic-concurrency checks above it.
- **Per-shard IO semaphore.** A second semaphore layer caps concurrent persistence calls per shard (`context_impl.go:117-126`). The comment at `context_impl.go:122` explicitly forbids acquiring it while holding the shard rwLock, preventing deadlocks.
- **SequentialScheduler over FIFO.** The standard `FIFOScheduler` is used where order does not matter (`common/tasks/fifo_scheduler.go`). Where per-key ordering matters, `SequentialScheduler` is used (`replication/fx.go:146,209`). The two are wrapped by `InterleavedWeightedRoundRobinScheduler` for cluster fan-out (`replication/fx.go:165-172`).
- **Adaptive pool over fixed worker count.** Deadlock detection uses an AdaptivePool because ping latency varies widely and the load is bursty (`common/deadlock/deadlock.go:84-90`).
- **Errgroup vs raw WaitGroup.** The package doc deliberately routes long-running goroutines to `goro.Group` and short-lived fan-out to `errgroup.Group` (`common/goro/package.go:9-12`), but in practice the code mostly uses raw `sync.WaitGroup` and `sync.Map` for result collection, reserving `errgroup` for tests.
- **Future for signaling.** Engine readiness (`shard/context_impl.go:96`), shard readiness (`controller_impl.go:58`), update acceptance and outcome (`update/update.go:60-77`), and visibility bulk processing (`processor.go:32-63`) all use the generic `FutureImpl[T]` rather than channels. This gives a typed, single-shot, exception-safe signaling primitive.
- **PrioritySemaphore vs IDMutex.** `PrioritySemaphore` is used for weighted, cancellation-aware locking (workflow context, shard IO); `IDMutex` is used for per-key serialization where no priority is needed (DLQ writer path, id-keyed maps).

## Notable Patterns

- **Atomic state machine for futures.** `pending → setting → ready` prevents data races when multiple `Get`s race with one `Set` (`common/future/future_impl.go:11-19,66-84`).
- **Owned-queue pattern in SequentialScheduler.** Each queue is processed by exactly one worker at a time, enforced by a channel that hands the queue back on shutdown (`common/tasks/sequential_scheduler.go:283-307`). This eliminates the need for per-queue locks.
- **Per-key sharding via hash + slot.** Replication parallelism is modeled by extending the workflow key with a hash-derived slot, producing N "virtual" sequential queues for one logical workflow (`service/history/replication/fx.go:186-205`). The shard itself stays single-writer.
- **Bounded fan-out via WaitGroup + mutex.** The dominant pattern in handlers and chasm tasks: one `sync.WaitGroup` plus one `sync.Mutex` for result collection, with each goroutine appending to a slice or incrementing an atomic counter. See `chasm/lib/scheduler/invoker_tasks.go:268-405` and `service/history/handler.go:1489-1535`.
- **Lifecycle via atomic.CompareAndSwap.** Every daemon uses `atomic.CompareAndSwapInt32` to guarantee single Start/Stop transitions (`common/tasks/sequential_scheduler.go:69-92`).
- **Per-queue mutex map.** `sync.Map` stores a `*sync.Mutex` per logical queue, providing automatic mutex-per-key without a global lock (`service/history/queues/dlq_writer.go:147-155`).
- **Deferred goroutine cancellation via context.** Every spawned goroutine accepts a `context.Context`; `goro.Group` cancels and waits in a single call (`common/goro/group.go:38-50`).

## Tradeoffs

- **Single-writer per workflow** trades intra-workflow throughput for ordering simplicity and crash safety. High-throughput workflows cannot use multiple workers to update state, but the system never has to reconcile conflicting updates.
- **SequentialScheduler ownership** (one queue = one worker) is simple but limits parallelism within a key to 1. The RunID-bucketed slot trick in replication (`replication/fx.go:186-205`) partially works around this by treating related tasks as different keys.
- **Per-shard IO semaphore** protects the database but can become a contention point under heavy load. Configurable via `ShardIOConcurrency` (`shard/context_impl.go:2069-2108`); forced to 1 for Cassandra to avoid connection storms.
- **`sync.WaitGroup` over `errgroup`.** Most fan-out code uses raw `sync.WaitGroup` and returns no error from goroutines, then collects partial results. This means callers cannot easily detect that all branches succeeded; they rely on metrics and per-branch counters.
- **Chasm is single-writer per execution.** Parallel advancement within one execution must be hand-written by the task author (`invoker_tasks.go`). This is intentional to keep the state machine deterministic, but it raises the cost of adding new parallel patterns.
- **DLQ writes serialized per queue.** Trades throughput for storage-layer CAS-conflict avoidance (`service/history/queues/dlq_writer.go:86-100,86-93 comment`).
- **Deadlock detector uses AdaptivePool.** Trades some overhead (worker spawn/shrink) for resilience when many goroutines must be pinged concurrently.

## Failure Modes / Edge Cases

- **ioSemaphore deadlock.** The comment at `shard/context_impl.go:122` warns that acquiring the ioSemaphore while holding the shard rwLock will deadlock. There is no static check; lint/test must catch this.
- **goroutine leak on Submit race.** `SequentialScheduler.Submit` adds the task before dispatching it; if the worker pool drains and the queue is then deleted, the task may stay orphaned. The `TrySubmit` path uses a `trySubmitLock` with a 200ms timeout to mitigate (`common/tasks/sequential_scheduler.go:144-199`).
- **Channel close vs Stop race.** `FIFOScheduler.Stop` and `SequentialScheduler.Stop` both close a `shutdownChan` and rely on `AwaitWaitGroup` for shutdown with a 1-minute cap (`common/tasks/fifo_scheduler.go:85`, `common/tasks/sequential_scheduler.go:100`). A misbehaving task that ignores context will be aborted at the timeout boundary.
- **Queue re-attachment on worker shrink.** `SequentialScheduler.processTaskQueue` re-enqueues the queue on `<-workerShutdownCh` so other workers can pick it up (`common/tasks/sequential_scheduler.go:280-282`). This is correct but only safe because queues are pure data + channel.
- **Panic in task execution.** `SequentialScheduler.executeTask` and `executableImpl` both wrap `task.Execute()` in `log.CapturePanic` (`common/tasks/sequential_scheduler.go:311-324`); the worker does not crash.
- **Future double-set panic.** `FutureImpl.Set` panics if already completed (`common/future/future_impl.go:72-78`); `SetIfNotReady` returns false instead (`87-104`). Callers must use the right variant.
- **Chasm duplicate ExecuteTask race.** `recordExecuteResult` uses `newlyStarted` (not `len(result.CompletedStarts)`) to avoid double-counting when two execute tasks overlap (`chasm/lib/scheduler/invoker_tasks.go:233-236`). This relies on the chasm framework serializing the read+update under one execution.
- **Stale readiness signal.** `controller_impl.go:396-405` swaps a new readiness cancel for the old one to prevent the previous readiness check from blocking the new set of owned shards.

## Future Considerations

- **Consolidate fan-out into a shared utility.** Three subsystems (frontend, history handler, chasm invoker) re-implement the same `WaitGroup + result mutex + append to slice` pattern. A `common/parallel.RunWithConcurrency` helper that returns `errors.Join(...)` would simplify the code.
- **Move ioSemaphore/rwLock ordering to a typed guard.** The lint rule at `shard/context_impl.go:122` is enforced only by comments. A type-level enforcement (e.g., a `WithIOLock` wrapper) would prevent future regressions.
- **Chasm parallel tasks.** The chasm tree currently serializes per execution key. A future task attribute could indicate "safe to parallelize" so the scheduler can fan out independent subtasks automatically.
- **errgroup adoption in production code.** Currently used only in tests; production code consistently uses raw `sync.WaitGroup`. Where error collection matters, `errgroup.WithContext` would simplify failure handling.
- **Adaptive pool for backpressure.** The AdaptivePool is currently used only by the deadlock detector. Other hot paths (e.g., a generic worker pool for "small items of work") could benefit.
- **Observability for parallel branches.** Some fan-out (frontend poll cancel, chasm invoker) uses atomic counters or per-task metrics but not a unified `parallel_branch_duration` metric. Adding one would help correlate parallel slow paths.
- **Move from `sync.Map` of mutexes to a typed `LockMap[T]` generic.** The DLQ writer's pattern (`service/history/queues/dlq_writer.go:147-155`) is reusable; promoting it to a typed library would prevent re-implementation.

## Questions / Gaps

- **Is there a documented invariant about goroutine ownership inside chasm tasks?** The framework serializes per execution key, but the `invoker_tasks.go` code spawns goroutines inside a task and claims no shared state will be mutated. This is implicit and not enforced by types.
- **What happens if two `Submit` calls for the same key race in `SequentialScheduler`?** `PutOrDo` (`common/tasks/sequential_scheduler.go:111-121`) uses the queue as a synchronization point, but the ordering of tasks added concurrently is not documented.
- **Is there a test for `AdaptivePool` correctness under load?** `common/goro/adaptive_pool_test.go` exists but the assertion is on worker counts, not throughput or starvation behavior.
- **What is the worst-case task backlog under a stalled ioSemaphore?** The shard IO semaphore has metrics (`shard/context_impl.go:1535,1538,1540`) but no documented SLO.
- **Does `errgroup.WithContext` cancel sibling goroutines on first error?** No production code uses it. The implicit convention is to wait for all branches even if some errored.
- **Are there any unsynchronized reads of shared state in startup paths?** The future-based readiness signal (`controller_impl.go:58`) implies the consumer blocks until set, but it is unclear whether any code path can race past `initialShardsAcquired.Ready()` and miss the signal.

---

Generated by `dimensions/01.07-concurrency-and-parallel-advancement.md` against `temporal`.