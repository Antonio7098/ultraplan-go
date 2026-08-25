# Source Analysis: temporal

## 02.08 Crash Recovery and Reconstruction

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (server, multi-service: frontend/history/matching/worker), persistence pluggable over Cassandra / MySQL / PostgreSQL / SQLite, gRPC, ring-based membership |
| Analyzed | 2026-07-11 |

## Summary

Temporal's crash-recovery model is layered and explicitly fenced: a **shard ownership / range-id** lock gates all write paths on the history side (`service/history/shard/context_impl.go:1777-1848`, `common/persistence/sql/shard.go:115-149`), a **persistent task queue + ack-level checkpoint** model guarantees that no enqueued timer/transfer/visibility/replication task is lost across restarts (`service/history/queues/queue_base.go:104-242`, `service/history/queues/queue_scheduled.go:54-87`), and an **executable wrapper with retry/backoff + DLQ** handles terminal-task failures (`service/history/queues/executable.go:520-621`, `service/history/queues/dlq_writer.go:64-143`). Worker-side service lifecycle is a deterministic CAS daemon pattern (`service/matching/service.go:62-125`, `service/history/service.go:67-176`, `common/daemon.go:6-12`) and post-shutdown recovery is automatic: any shard not owned by another host is reacquired on the next membership update (`service/history/shard/controller_impl.go:383-521`).

Recovery is **automatic, idempotent at the shard/task level, and observable**: tasks that have not been ack-completed are re-read on startup from `tasks` tables via range queries (`service/history/queues/queue_base.go:118-133`, `common/persistence/data_interfaces.go:446-488`), shard ownership is re-validated with `assertOwnership` after every persistence error (`service/history/shard/context_impl.go:1913-2034`), and the host is held out of `SERVING` (`service/history/service.go:79-90`) until `initialShardsAcquired` resolves. There is an explicit **DLQ** for terminal tasks (`docs/admin/dlq.md:1-71`, `service/worker/dlq/workflow.go:203-323`) and a workflow **RebuildMutableState** admin path for corrupted workflow state (`service/history/workflow_rebuilder.go:64-110`, `service/history/history_engine.go:993-1006`). Orphaned in-flight work is bounded by range-id fencing — if a host dies mid-update the persistence call returns `ShardOwnershipLostError` (`common/persistence/sql/shard.go:132-149`) and the new owner replays from a consistent `exclusiveReaderHighWatermark`.

## Rating

**9 / 10** — A mature, durable, observable crash-recovery system. Recovery is automatic on restart via shard ownership / membership, idempotent through range-id fencing, retry-and-DLQ on task failure, and end-to-end tested with explicit ownership-lost / non-ownership-lost retry tests (`service/history/shard/context_test.go:462-512`). Deductions: orphaned run detection on the matching side relies on a TTL-based `liveness` unload (`service/matching/liveness.go:23-49`, `service/matching/physical_task_queue_manager.go:169-173`) which is best-effort, not an explicit sweeper; the in-memory `outstandingTasks` ack map (`service/matching/ack_manager.go:14-32`) means the matching engine relies on persistence reconstruction for in-flight task recovery; DLQ is disabled by default and the merge/purge workflow must be triggered manually (`docs/admin/dlq.md:52-71`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shard ownership / range-id model | `ShardOwnershipLostError` raised on range-ID mismatch | `common/persistence/sql/shard.go:115-149` |
| GetOrCreateShard (recovery entry point) | loads shard info on first call after restart | `common/persistence/sql/shard.go:37-80` |
| Shard Manager wrapper | GetOrCreateShard sets initial RangeID; LoadShardMetadata initializes queue state | `common/persistence/shard_manager.go:36-68` |
| Initial shard metadata load | re-reads persisted shard info + queue ack levels after restart | `service/history/shard/context_impl.go:1777-1848` |
| Range-ID renewal + retry policy | 5-min retry with exponential backoff before unloading shard | `service/history/shard/context_impl.go:1913-2034` |
| Shard state machine | `Initialized → Acquiring → Acquired → Stopping → Stopped` | `service/history/shard/context_impl.go:61-69` |
| Context state requests | `Acquire`, `Acquired`, `Lost`, `Stop`, `FinishStop` transitions | `service/history/shard/context_impl.go:168-182` |
| Shard controller start | membership-driven reacquire; readiness gate via `initialShardsAcquired` | `service/history/shard/controller_impl.go:105-162` |
| Shard reacquire logic | iterates all shards owned by host; concurrent acquisition | `service/history/shard/controller_impl.go:383-482` |
| Ownership verification per request | `verifyOwnership` before creating shard context | `service/history/shard/controller_impl.go:230-276` |
| Handover tracker (cross-cluster move) | tracks namespaces in handover so apis are blocked during transition | `service/history/shard/handover_tracker.go:52-130` |
| Initial shards acquired (readiness) | blocks health=Serving until first acquireShards pass succeeds | `service/history/shard/controller_impl.go:484-521` |
| Shard linger on ownership lost | gracefully closes shard instead of immediate teardown | `service/history/shard/controller_impl.go:308-381` |
| Daemon status pattern | CompareAndSwap lifecycle used everywhere | `common/daemon.go:6-12` |
| History Service startup | gRPC NOT_SERVING → SERVING gated on shards | `service/history/service.go:67-114` |
| History Service shutdown | evict self + drain + linger + handler stop | `service/history/service.go:117-176` |
| History engine creation | per-shard engine creation on shard acquire | `service/history/shard/context_impl.go:1969-2000` |
| Task queue (queue base) | reads `persistenceState` from shard on construction | `service/history/queues/queue_base.go:104-242` |
| Queue persistence state recovery | `readerScopes` + `exclusiveReaderHighWatermark` reloaded from shard | `service/history/queues/queue_base.go:118-133` |
| Scheduled queue pagination | `GetHistoryTasks` reads from persistence on each slice load | `service/history/queues/queue_scheduled.go:54-87` |
| Immediate queue pagination | same pattern, used for transfer/visibility/replication | `service/history/queues/queue_immediate.go:44-69` |
| Look-ahead task loader | re-fetches next fire-time task after each reader pass | `service/history/queues/queue_scheduled.go:227-278` |
| Reader event loop | retries on read error with `CreateReadTaskRetryPolicy` backoff | `service/history/queues/reader.go:410-485`, `common/util.go:211-215` |
| Range-complete of history tasks | durable deletion of completed task range | `service/history/queues/queue_base.go:365-387` |
| Checkpoint timer | periodic ack-level + queue-state persistence | `service/history/queues/queue_base.go:234-249` |
| Queue state update | persists `readerScopes` + `exclusiveReaderHighWatermark` | `service/history/queues/queue_base.go:389-409` |
| Executable wrapper (retry policy) | reschedule policy + Nack + DLQ on max attempts | `service/history/queues/executable.go:85-103` |
| HandleErr -> DLQ on terminal | detects `MaybeTerminalTaskError`, marks `terminalFailureCause` | `service/history/queues/executable.go:517-621` |
| DLQ writer | enqueues task to `QueueTypeHistoryDLQ` with serialization mutex | `service/history/queues/dlq_writer.go:64-143` |
| DLQ writer concurrent safety | per-queue mutex to serialize CAS writes | `service/history/queues/dlq_writer.go:86-100,147-155` |
| DLQ metrics | `metrics.DLQWrites`, `TaskDLQFailures`, `TaskDLQSendLatency` | `service/history/queues/dlq_writer.go:125-141` |
| DLQ purge/merge workflow | operator-triggered re-enqueue or delete | `service/worker/dlq/workflow.go:203-323` |
| DLQ admin doc | operational guide for purge / merge / investigation | `docs/admin/dlq.md:1-71` |
| Task rescheduler | buffers nacked tasks; per-namespace failover | `service/history/queues/rescheduler.go:120-172` |
| Rescheduler drain on stop | bounded by WaitGroup + cleanup loop | `service/history/queues/rescheduler.go:208-241` |
| Workflow rebuilder | admin API to rebuild corrupted workflow mutable state | `service/history/workflow_rebuilder.go:51-110` |
| Conflict resolver | replays history branch + version-history on replication conflict | `service/history/ndc/conflict_resolver.go:59-114` |
| StateRebuilder | rebuilds mutable state from history events | `service/history/ndc/state_rebuilder.go:103-150` |
| Workflow resetter | reset to last successful event + replay | `service/history/ndc/resetter.go:81-139` |
| Workflow execution cache | in-memory with finalizer-registered callbacks to clear on shutdown | `service/history/workflow/cache/cache.go:93-149` |
| Cache finalizer | on shutdown, locks + clears each cached mutable state | `service/history/workflow/cache/cache.go:108-125` |
| Mutable state loaded on demand | `GetWorkflowExecution` goes to persistence | `service/history/shard/context_impl.go:801-815` |
| Matching service start | handler.Start registers gRPC, NOT_SERVING → SERVING | `service/matching/service.go:62-125` |
| Matching handler Start | subscribes to membership and starts engine | `service/matching/handler.go` (handler.Start called from service.go) |
| Matching engine ready signal | `WaitUntilInitialized` on physicalTaskQueueManager | `service/matching/physical_task_queue_manager.go:342-353` |
| Physical task queue liveness | idle unload via timer + onIdle callback | `service/matching/liveness.go:23-49` |
| Liveness used on unload | unload physical task queue after TTL | `service/matching/physical_task_queue_manager.go:169-173` |
| AckManager (matching) | in-memory outstanding tasks; reconstructs from persistence | `service/matching/ack_manager.go:14-32` |
| Cassandra task TTL | persisted tasks expire via TTL column | `common/persistence/cassandra/matching_task_store.go:41-55` |
| Server start sequence | matching(1) → history(2) → internal-frontend(3) → frontend(3) → worker(4) | `temporal/server_impl.go:47-53` |
| Server Start | initSystemNamespaces + per-service fx.Start | `temporal/server_impl.go:88-146` |
| Init system namespaces | ensures system namespaces exist before any service | `temporal/server_impl.go:148-198` |
| Service Start guard | `if atomic.CompareAndSwapInt32(...DaemonStatusInitialized, DaemonStatusStarted)` | `service/matching/service.go:62-125`, `service/history/service.go:67-114` |
| AcquireShard test (success) | full mock-based happy path | `service/history/shard/controller_test.go:150-181` |
| AcquireShard test (lookup failure) | membership failure propagates | `service/history/shard/controller_test.go:221-235` |
| AcquireShard test (ownership lost → no retry) | verifies non-retryable error path | `service/history/shard/context_test.go:462-472` |
| AcquireShard test (eventual success after N retries) | validates exponential backoff with retry policy | `service/history/shard/context_test.go:486-499` |
| AcquireShard test (no error first try) | success path on first attempt | `service/history/shard/context_test.go:501-512` |
| History engine closed test | exercises close-on-ownership-change | `service/history/shard/controller_test.go:297-394` |
| DLQ writer tests | ErrGetNamespaceName, Ok, ConcurrentWrites, ConcurrentDifferentQueues | `service/history/queues/dlq_writer_test.go:47-281` |
| Executable DLQ tests | SendToDLQAfterMaxAttempts, DontSend when disabled, DLQFailThenRetry | `service/history/queues/executable_test.go:509-820` |
| Periodic queue checkpoint | `CheckpointInterval` dynamic config drives persistence | `service/history/timer_queue_factory.go:198-200` |
| Multi-reader shard scaling | up to `TimerQueueMaxReaderCount` readers per shard | `service/history/queues/queue_base.go:141-175` |
| Reader stuck detection | `ReaderStuckCriticalAttempts` alert in MonitorOptions | `service/history/timer_queue_factory.go:191-193` |
| Reader Stuck action | move / split / drop slices when reader is stuck | `service/history/queues/action_reader_stuck.go` (file present) |
| Slice splitting | range-based split driven by predicate + ack level | `service/history/queues/grouper.go` |
| Per-namespace failover | `queue.FailoverNamespace` reschedules all pending tasks for namespace | `service/history/queues/queue_base.go:255-259` |
| WorkflowRebuilder.rebuild | loads mutable state + replay via state rebuilder | `service/history/workflow_rebuilder.go:64-110` |
| RebuildableCheck | gate so only "rebuildable" workflows are reset | `service/history/workflow_rebuilder.go:110-148` |
| OverrideToDB | persists rebuilt state | `service/history/workflow_rebuilder.go:234-254` |
| TaskExecutor.NewTimerQueueFactory | wires up scheduler + rescheduler + DLQ + executor | `service/history/timer_queue_factory.go:39-208` |
| Same for transfer | transfer queue factory wires DLQ + factory | `service/history/transfer_queue_factory.go:119` |
| DLQ wired into executable | DLQEnabled + DLQWriter + DLQErrorPattern on every factory | `service/history/queues/executable_factory.go:36-110` |
| DLQErrorPattern | regex match against err to short-circuit to DLQ | `service/history/queues/executable.go:600-621` |
| Panic recovery in Execute | defer catches panic and treats it as task failure | `service/history/queues/executable.go:302-359` |
| Resource exhausted fallback | BusyWorkflow → scheduler.HandleBusyWorkflow | `service/history/queues/executable.go:686-695` |
| Terminated-task safety | DLQ enable/disable toggled at runtime | `service/history/queues/executable_test.go:598-741` |
| ShardOwnershipLostError → ShardOwnershipLost service error | redirect caller to owning host | `service/history/handler.go:2233-2244`, `service/history/chasm_engine.go:1276-1283` |
| Per-host restart metric | `metrics.RestartCount.Record(1)` on every service Start | `service/history/service.go:71`, `service/matching/service.go:66` |
| Initial Shards Acquired readiness check | blocks serving until all owned shards are ready | `service/history/service.go:79-90` |
| Linger on shutdown | up to `ShardLingerTimeLimit` for in-flight requests | `service/history/service.go:138-145` |
| ShardFinalizerTimeout | bounded finalizer run during shutdown | `service/history/service.go:142` |
| Configurable shard linger / ownership check QPS | tuneable per operator | `service/history/shard/controller_impl.go:347-381` |

## Answers to Dimension Questions

1. **What happens to in-progress work after restart?** Two classes of in-progress work are handled differently. (a) *In-flight persistence writes*: any write attempted after the host loses range-ID fencing returns `ShardOwnershipLostError` (`common/persistence/sql/shard.go:132-149`); the new shard owner reloads the persisted state and resumes from there. (b) *Background tasks not yet ack-completed*: the persistent task tables are the source of truth. On shard restart, `queueBase` reads `persistenceState` from the shard (`service/history/queues/queue_base.go:118-133`) and re-creates readers from `exclusiveReaderHighWatermark`. The reader paginates `GetHistoryTasks` again (`service/history/queues/queue_scheduled.go:54-87`) and re-executes any un-acked task. Workflow mutable state is loaded lazily from `GetWorkflowExecution` (`service/history/shard/context_impl.go:801-815`) plus the workflow cache's finalizer that clears all entries on shutdown (`service/history/workflow/cache/cache.go:108-125`).

2. **Are orphaned runs detected?** Orphaned runs *in the history queue sense* are detected via shard ownership-lost + the matching engine's `liveness` TTL (`service/matching/liveness.go:23-49`). The `liveness` timer fires `UnloadFromPartitionManager` if no work touches a physical task queue for `MaxTaskQueueIdleTime`; on unload the in-memory state is dropped and the next task-touch re-loads from persistence. There is **no background sweeper** that scans "orphaned" matching tasks beyond TTL expiry; the matching service relies on persistence re-construction. On the history side, the only orphan-recovery mechanism is the `ShardOwnershipLostError` driven `shardLingerThenClose` (`service/history/shard/controller_impl.go:308-381`).

3. **Is recovery automatic?** Yes. On process start, the shard controller joins membership, calls `acquireShards` (`service/history/shard/controller_impl.go:383-482`), and acquires range-IDs for all owned shards. Each `ContextImpl.acquireShard` call re-loads `shardInfo` + `QueueStates` from persistence (`service/history/shard/context_impl.go:1777-1848`). Tasks not yet ack-completed are re-read from persistence; the workflow cache is re-populated on demand; matching engine rebuilds backlog from persistence on first task or poller. The health server stays in `NOT_SERVING` until `InitialShardsAcquired` resolves (`service/history/service.go:79-90`), guaranteeing the server only serves traffic once recovery is verified.

4. **Is recovery idempotent?** Yes at every level. (a) Shard updates use range-id fencing so a stale owner cannot corrupt state (`common/persistence/sql/shard.go:132-149`). (b) Task execution is idempotent via the `ErrInvalidTask` / `isInvalidTaskError` path that drops the task on first attempt (`service/history/queues/executable.go:537-542`). (c) Executables have explicit Ack/Nack states that gate re-execution (`service/history/queues/executable.go:656-678`). (d) The workflow cache detects dirty mutable states and panics rather than silently double-applying changes (`service/history/workflow/cache/cache.go:377-391`). (e) DLQ writes serialize concurrent writers per-queue with a sync.Map of mutexes (`service/history/queues/dlq_writer.go:86-100,147-155`).

5. **Are users/operators notified?** Notification is metric- and log-based, not push. The DLQ writer emits a `metrics.DLQWrites` counter and a `Warn("Task enqueued to DLQ", ...)` log with `dlq-message-id`, `xdc-source-cluster`, `xdc-target-cluster`, `queue-task-type`, namespace (`service/history/queues/dlq_writer.go:125-141`). The matching engine uses `metrics.RestartCount` on every service Start (`service/matching/service.go:66`). Replication DLQ operations (`MergeDLQMessages`, `PurgeDLQMessages`) are exposed as admin RPCs and the DLQ workflow provides a progress query (`service/worker/dlq/workflow.go:204-211`). Operators use `tdbg dlq list/read/merge/purge` to inspect DLQ contents (`docs/admin/dlq.md:30-71`).

## Architectural Decisions

- **Range-ID fencing**: every shard has a monotonically increasing `RangeID`. Any `UpdateShard` whose `PreviousRangeID` does not match returns `ShardOwnershipLostError`. This is the single source of truth for "am I the owner?" — no Raft/Paxos needed (`common/persistence/sql/shard.go:115-149`).
- **Persistent task tables over in-memory queues**: timer/transfer/visibility/replication tasks live in DB tables (`tasks`, `task_queues`) and are paged in by readers (`service/history/queues/queue_scheduled.go:54-87`). The result is that "queue state" = persisted `exclusiveReaderHighWatermark` and the queue can be rebuilt from scratch.
- **CAS-based daemon lifecycle**: `Initialized → Started → Stopped` enforced via `atomic.CompareAndSwapInt32` (`common/daemon.go:6-12`). Each daemon is idempotent under repeated `Start`/`Stop` calls, which matters for crash-recovery because reentrant shutdown races are possible.
- **Linger on graceful shutdown**: instead of immediate `FinishStop`, the controller "lingers" on shards that just lost ownership for `ShardLingerTimeLimit` to handle in-flight requests before the new owner actually starts (`service/history/shard/controller_impl.go:308-381`).
- **Workflow rebuild as admin path**: `RebuildMutableState` RPC (`service/history/history_engine.go:993-1006`) is exposed for operators to recover a workflow whose mutable state has been corrupted. It re-replays history events via `StateRebuilder.Rebuild` (`service/history/ndc/state_rebuilder.go:103-150`).
- **DLQ as last-resort escape hatch**: terminal tasks can be diverted to a per-cluster DLQ (`service/worker/dlq/workflow.go:203-323`). Operators must explicitly merge (re-enqueue) or purge to clear them.
- **Shard handover for namespaces**: when a namespace is moved between clusters, the `HandoverTracker` records pending task IDs and gates APIs/tasks for that namespace until handover completes (`service/history/shard/handover_tracker.go:52-130`). The replication stream absorbs the migration.

## Notable Patterns

- **Persist + enqueue-task in single transaction**: every workflow mutation creates `WorkflowMutation` + a `tasks` slice in the same `UpdateWorkflowExecution` call (visible in `transaction_manager_new_workflow.go:160-220` and `transaction_manager_existing_workflow.go`). This is the canonical "transactional outbox" pattern, which is why crash recovery is possible at all.
- **Load-and-then-acquire shard metadata**: `loadShardMetadata` is called *outside* the rwlock on first acquisition (`service/history/shard/context_impl.go:1777-1848`) so a slow persistence read does not block other shard ops.
- **Exponential retry with separate resource-exhausted policy**: `CreateTaskReschedulePolicy`, `CreateTaskResourceExhaustedReschedulePolicy`, `CreateDependencyTaskNotCompletedReschedulePolicy` are distinct (`common/util.go:225-255`), so retry behavior is error-class specific.
- **Reader group scaling**: up to `TimerQueueMaxReaderCount` readers per shard, each owning a non-overlapping slice of the task keyspace (`service/history/queues/queue_base.go:141-175`, `service/history/queues/reader_group.go:41-153`).
- **Stuck-reader mitigation**: `Monitor` + `Mitigator` + `Action` triad in `service/history/queues/` detects and acts on stuck readers (`service/history/queues/action_reader_stuck.go`, `service/history/queues/mitigator.go`).
- **Finalizer registry**: cache items are registered with the shard's `Finalizer` so on shutdown every cached mutable state is explicitly cleared and unlocked (`service/history/workflow/cache/cache.go:108-125`, `common/finalizer/finalizer.go:43-119`).
- **Per-host shutdown drain**: `MembershipAlignChange` + `ShutdownDrainDuration` + `ShardLingerTimeLimit` + `ShardFinalizerTimeout` summed (`service/history/service.go:138-145`) gives operators a deterministic timeline.
- **Soft-assert on dirty mutable state**: `softassert.Fail` rather than silent corruption (`service/history/workflow/cache/cache.go:377-391`).
- **Stream vs scheduled queue separation**: `immediateQueue` polls every `MaxPollInterval`, `scheduledQueue` uses a `timerGate` driven by look-ahead to fire at task-due time (`service/history/queues/queue_immediate.go:132-169`, `service/history/queues/queue_scheduled.go:175-216`).

## Tradeoffs

- **Crash recovery is asynchronous and shard-scoped**: a host cannot serve traffic until `InitialShardsAcquired` resolves (`service/history/service.go:79-90`), which under many-shard / slow-DB conditions can delay startup. Operators tune `StartupMembershipJoinDelay` to gate this.
- **Task ack-level lag**: tasks are checkpointed at `CheckpointInterval` (default ~5 min) so on restart the host may re-execute up to that window of tasks. The DLQ and `isInvalidTaskError` path keep this safe.
- **DLQ-disabled-by-default**: `TaskDLQEnabled` defaults to `false`, so terminal failures silently drop tasks (`service/history/queues/executable.go:583-585`). Operators must opt-in.
- **Matching-side in-memory state**: `ackManager.outstandingTasks` lives in process memory (`service/matching/ack_manager.go:14-32`); on matching-host restart, in-flight task IDs are reconstructed from persistence read levels, not directly.
- **RangeID increment on shard ownership lost**: after ownership lost, the shard's rangeID is incremented; any in-flight writes from the old owner are rejected. This is safe but means operators may see "operation conflict" errors that have to be retried.
- **Linger is bounded by config**: `ShardLingerTimeLimit` (max 1 min) caps shutdown grace (`service/history/shard/controller_impl.go:30`), which can drop in-flight matching requests if too long.
- **Read repair on history divergence**: when source and target cluster diverge, the conflict resolver picks the highest version (`service/history/ndc/conflict_resolver.go:76-83`). If a workflow's mutable state is corrupted, manual `RebuildMutableState` is required.
- **No automatic orphan-run sweeper** for matching: relies on liveness-TTL unload.

## Failure Modes / Edge Cases

- **Process SIGKILL during workflow update**: persistence transaction either commits atomically or is rolled back; the next owner reloads and replays (`common/persistence/sql/shard.go:82-113`).
- **DB unreachable during shard acquire**: `acquireShardRetryable` retries every 1s with 5-min expiration (`service/history/shard/context_impl.go:2004-2021`). Beyond 5 min the shard is moved to `Stop` and the controller tries to unload.
- **Shard ownership lost mid-update**: `UpdateShard` returns `ShardOwnershipLostError`, in-flight history requests are rejected, callers redirected to the new owner via `ShardOwnershipLost` (`service/history/handler.go:2239-2244`).
- **Task processing panic**: `executable.Execute()` defer recovers and counts as a task failure, not a worker crash (`service/history/queues/executable.go:302-359`).
- **Workflow mutable state corruption**: operator runs `RebuildMutableState`; rebuilder replays history (`service/history/workflow_rebuilder.go:64-110`).
- **History divergence between clusters**: `ConflictResolver` chooses highest version, rebuilds mutable state from history branch (`service/history/ndc/conflict_resolver.go:116-183`).
- **Matching physical task queue idle forever**: liveness TTL unloads after `MaxTaskQueueIdleTime` (`service/matching/physical_task_queue_manager.go:169-173`).
- **DLQ write failure**: `metrics.TaskDLQFailures` increments and the failure is logged; the original task is still considered failed (`service/history/queues/executable.go:390-395`).
- **Replication backpressure**: replication tasks queued for active cluster use the same DLQ mechanism (`service/history/queues/executable.go:126-134`).
- **Cassandra-specific shard concurrency**: `ShardIOConcurrency` is forced to 1 for Cassandra due to LWT semantics (`service/history/shard/context_impl.go:2069-2076`).
- **Schema version migration**: persistence layer has `SchemaVersion` checks; if the running binary is older than DB schema, startup fails (`common/persistence/operation_mode_validator.go`).
- **Workflow cache eviction during a request**: `forceClearContext` releases and reloads mutable state on the next call (`service/history/workflow/cache/cache.go:371-392`).
- **Membership flap during recovery**: shard controller re-validates ownership per membership update (`service/history/shard/controller_impl.go:411-422`); an old owner is unloaded via linger.
- **Reader stuck on a single slice**: `ActionReaderStuck` splits and re-assigns the slice (`service/history/queues/action_reader_stuck.go`).
- **Workflow task speculative timeout**: `SpeculativeWorkflowTaskTimeoutQueue` is a separate, in-memory queue that survives only within a host (`service/history/queues/speculative_workflow_task_timeout_queue.go:65-78`); restart will drop pending speculative tasks.

## Future Considerations

- **DLQ-by-default**: enabling `TaskDLQEnabled` for non-replication tasks by default would eliminate silent drops; currently disabled (`service/history/queues/executable.go:175-178`).
- **Generic orphan-run sweeper**: a background scan for "running" matching tasks with no poller activity for >X hours, similar to a Temporal executions scavenger. The current `liveness` model is per-queue, not per-task.
- **Resumable workflow mutable state checkpoints**: today, mutable state is rebuilt on every workflow operation. A persistent in-progress mutation log (similar to Letta's `pending_writes`) would reduce rebuild cost for heavy workflows.
- **Auto-rebuild on persistence anomalies**: currently `RebuildMutableState` is an explicit admin action; auto-detection of state divergence could trigger it.
- **Cross-shard idempotency tokens for activity completion**: today activity completion is idempotent via `startedID`, but cross-host retries of an in-flight completion can race the original; explicit idempotency keys could harden this.
- **Distributed matching-scheduler lease**: matching task queue partitions are owned in-memory; persistence already tracks them but no leader-lease is held across hosts. A persisted partition lease would let surviving hosts pick up partitions of a dead host more quickly.
- **Observable shard readiness**: `initialShardsAcquired` blocks serving but the wait is invisible to clients. A `/debug/readyz` with shard-by-shard detail would help operators diagnose stuck starts.

## Questions / Gaps

- What is the default `ShardIOConcurrency` for SQL stores (Postgres/MySQL)? The comment in `service/history/shard/context_impl.go:2069-2076` only enforces 1 for Cassandra.
- Does `RebuildMutableState` (the admin RPC) verify that the workflow has actually drifted before overwriting, or does it always overwrite? The `rebuildableCheck` (`service/history/workflow_rebuilder.go:110-148`) suggests it gates on certain conditions, but the exact contract is not documented.
- When a matching physical task queue is unloaded via `liveness`, are there any pending tasks that could be silently lost? The `physicalTaskQueueManager.Stop` flushes a final ack update (`service/matching/physical_task_queue_manager.go:316-333`) but only if the backlog manager is alive.
- How does the `taskReader` / `taskWriter` recover after the matching process restarts mid-`getTasks`? The `setReadLevelAfterGap` method (`service/matching/ack_manager.go:63-76`) handles the "no tasks in range" case but does not cover mid-stream crash.
- Is there a sweeper for `tasks` rows that exceed `expirationTime` but were never picked up? `matching_task_store.go:41-55` uses TTL in Cassandra, but SQL stores may rely on a sweep job whose code path was not located.
- Does `DLQWriter.WriteTaskToDLQ` block the in-memory DLQ queue from being read while writing? The per-queue mutex serializes writes (`service/history/queues/dlq_writer.go:86-100`) but there is no equivalent serialization on the read side; reads during a write may see transient state.
- For Chasm (`chasm_engine.go`), is there an equivalent shard-state recovery path that re-runs pending transitions? `getExecutionLease` reloads mutable state from persistence (`service/history/chasm_engine.go:1153-1259`), but in-progress Chasm transitions are not described as checkpointed mid-flight.

---

Generated by `02.08-crash-recovery-and-reconstruction` against `temporal`.