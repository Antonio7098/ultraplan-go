# Source Analysis: temporal

## Dimension 07.05: Resource Locking and Isolation

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (distributed workflow orchestration server; persistence: Cassandra/ScyllaDB, MySQL/PostgreSQL) |
| Analyzed | 2026-08-23 |

## Summary

Temporal is a workflow orchestration server, not an agent harness, so the dimension's "shared resources during tool execution" map onto its own shared mutable resources: workflow executions (mutable state), shard contexts and their database rows, task queue partitions, namespace registry caches, replication queues, and in-memory pagination buffers (`service/history/workflow/context.go:52-58`).

The protection model is layered, with each layer fencing the one below it:

1. **Database conditional writes** — every persistence mutation is a compare-and-swap guarded by `IF range_id = ?` on the shard lease and version conditions on workflow rows (`common/persistence/cassandra/mutable_state_store.go:25-31`), so even a buggy or stale owner cannot corrupt state.
2. **Shard lease (fencing epoch)** — each history host owns a range of workflow IDs via a `RangeId` lease renewed through conditional `UpdateShard` calls (`service/history/shard/context_impl.go:1162-1206`); losing the lease yields `ShardOwnershipLostError`.
3. **Per-workflow execution lock** — a capacity-1 priority semaphore serializes all mutations of a single workflow run (`service/history/workflow/context.go:48`, `:116`, `:128-137`), acquired through the workflow cache with deadline clamping (`service/history/workflow/cache/cache.go:327-357`).
4. **Task queue single-writer** — each physical task queue partition has an in-memory manager whose persistence writes are serialized by an embedded mutex (`service/matching/db.go:36-55`), explicitly to avoid concurrent Cassandra LWT timeouts (`service/matching/db.go:90-95`).

Deadlock prevention is explicit rather than accidental: context-bounded acquisition everywhere, two documented lock-ordering rules (`service/history/shard/context_impl.go:123-128`; `common/namespace/nsregistry/registry.go:125-129`), try-lock variants, and a production deadlock detector that periodically probes the hottest locks (`common/deadlock/deadlock.go:41-59`, `service/history/shard/context_impl.go:236-264`).

On the harness question "can two tools edit the same file safely?" — the analogous case (two concurrent updates mutating one workflow) is safe by construction: mutations are serialized by the per-workflow semaphore, and any write that slips past due to failover or staleness is rejected at the DB layer by typed condition errors (`common/persistence/cassandra/errors.go:48-104`). There is no process-level sandbox for user code inside this source; namespace is the isolation boundary (`service/frontend/workflow_handler.go:458-461`) and worker-code sandboxing deliberately lives in the SDK, outside this repository.

## Rating

**9 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces**: a dedicated `common/locks` package defines `Locker` (`common/locks/lock.go:3-10`), sharded `IDMutex` (`common/locks/id_mutex.go:11-15`), `PriorityMutex`, and the workhorse `PrioritySemaphore` (`common/locks/priority_semaphore.go:5-11`) adapted from `golang.org/x/sync/semaphore`.
- **Operational safeguards**: ctx-bounded acquisition with deadline clamping that converts contention into a visible `ErrResourceExhaustedBusyWorkflow` (`service/history/workflow/cache/cache.go:351-354`); panic-safe idempotent lock release (`service/history/workflow/cache/cache.go:359-405`).
- **Observable**: acquisition-failure and hold-duration metrics (`service/history/workflow/cache/cache.go:315-316`, `:372`), deadlock-detector counters/gauges (`common/deadlock/deadlock.go:114`, `:148-151`), and dedicated latency metrics for the shard lock and I/O semaphore (`service/history/shard/context_impl.go:250`, `:262`).
- **Proven under failure/scale**: lease fencing plus DB-level CAS means failover mid-write produces typed, prioritized conflict errors instead of corruption (`common/persistence/cassandra/errors.go:116-159`); tests cover semaphore behavior, deadlock detection, and handover rejection (`common/locks/priority_semaphore_test.go:53-100`, `common/deadlock/deadlock_test.go:32-77`, `service/history/shard/handover_events_test.go:161-165`).

Not a 10 because lock ordering between components is enforced only by comments, not mechanically (see Failure Modes and Future Considerations).

## Evidence Collected

Every entry cites paths relative to the selected source directory with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Lock manager interface | `Locker` interface (`Lock`/`Unlock`) implemented by all lock primitives | `common/locks/lock.go:3-10` |
| Lock manager: keyed mutexes | `IDMutex` locks any comparable identifier via hash-sharded mutex map; entries refcounted and deleted on last unlock | `common/locks/id_mutex.go:11-15`, `:64-80`, `:83-98` |
| Lock manager: priority semaphore | `PrioritySemaphore.Acquire(ctx, priority, n)` with `PriorityHigh`/`PriorityLow` wait lists; aborts on ctx done even if tokens available | `common/locks/priority_semaphore.go:5-11`; `common/locks/priority_semaphore_impl.go:46-58`, `:83-121` |
| Workflow execution lock | Each workflow context embeds `locks.NewPrioritySemaphore(1)`; `Lock(ctx, priority)`/`Unlock()` serialize mutable-state access | `service/history/workflow/context.go:48`, `:116`, `:128-137` |
| Lock acquisition path | Cache `PutIfNotExist` converges concurrent creators on one context, then acquires its lock before returning release func | `service/history/workflow/cache/cache.go:304-318` |
| Lock timeout policy | Deadline clamping: non-user callers capped at `nonUserContextLockTimeout`; API callers reserve 500 ms tail; timeout → `ErrResourceExhaustedBusyWorkflow` | `service/history/workflow/cache/cache.go:327-357` |
| Safe release | Release func is idempotent via `atomic.CompareAndSwapInt32`, recovers panics, clears dirty state, records hold-duration metric | `service/history/workflow/cache/cache.go:359-405` |
| Shard concurrency control | Per-shard `ioSemaphore` (persistence-concurrency gating) + `stateLock` + `rwLock`; comment forbids acquiring ioSemaphore while holding rwLock (deadlock) | `service/history/shard/context_impl.go:123-137` |
| Shard ownership fencing | `renewRangeLocked` drains in-flight task requests, increments `RangeId`, conditionally updates shard row with `PreviousRangeID` | `service/history/shard/context_impl.go:1162-1206` |
| Deadlock detection probes | Shard exposes rwLock and ioSemaphore as pingable checks with tailored timeouts (e.g. `ShardIOTimeout()+30s`) | `service/history/shard/context_impl.go:236-264`; `service/history/shard/controller_impl.go:129` |
| Deadlock detector runtime | Periodic ping of registered roots; on timeout records metric, optionally dumps goroutines, fails gRPC health, or aborts process | `common/deadlock/deadlock.go:41-59`, `:111-128`, `:177-209` |
| Namespace isolation model | "Namespace acts as a sandbox and provides isolation for all resources within the namespace"; all resources belong to exactly one namespace | `service/frontend/workflow_handler.go:458-461` |
| Namespace cache locking | `nsMapsLock` (RWMutex) protects name/id maps; `readthroughLock` must be acquired before `nsMapsLock` (documented order) | `common/namespace/nsregistry/registry.go:115-132` |
| Failover isolation | Ops for namespaces in handover rejected under shard write lock with `ErrNamespaceHandover` | `service/history/shard/context_impl.go:1144-1152`; `service/history/shard/handover_tracker.go:31-33`, `:153` |
| Task queue partition isolation | Matching engine guards its partition map with `partitionsLock sync.RWMutex`; separate `replicationLock` serializes replication-queue writes | `service/matching/matching_engine.go:166`, `:197` |
| Task queue single-writer gate | `taskQueueDB` embeds `sync.Mutex`; doc states writes are serialized to avoid concurrent-Cassandra-LWT timeouts and guarantee one writer per queue | `service/matching/db.go:36-55`, `:86-95` |
| Queue lease takeover | `RenewLease` → `takeOverTaskQueueLocked`/`updateTaskQueueLocked` bump `rangeID` under the queue mutex; lost races unload the partition | `service/matching/db.go:150-174`, `:176-199` |
| DB enforcement: queue CAS | Task queue create/update/delete templates use LWT guard `IF range_id = ?` | `common/persistence/cassandra/matching_task_store_queue.go:77-115` |
| DB enforcement: shard CAS | Shard update template guarded by `IF range_id = ?` | `common/persistence/cassandra/shard_store.go:37` |
| DB enforcement: execution CAS | Mutable-state batch asserts `IF range_id = ?` plus per-row event-id/version conditions; current-run pointer uses multi-condition CAS | `common/persistence/cassandra/mutable_state_store.go:25-31`, `:33-52` |
| Conflict classification | Failed conditional batches are scanned per-row and converted into priority-ranked typed errors (`ShardOwnershipLostError` > current-workflow > workflow condition failures) | `common/persistence/cassandra/errors.go:48-104`, `:106-144`, `:146-159` |
| SQL path locking | SQL persistence takes a transactional row lock on `current_executions` then compares run-ID/write-version conditions in Go | `common/persistence/sql/execution_util.go:573-596`; `common/persistence/sql/execution.go:96` |
| Priority discipline | Background task executors (timer/transfer/replication/state machines) take workflow locks at `PriorityLow`; interactive APIs preempt them | `service/history/timer_queue_task_executor_base.go:103`; `service/history/statemachine_environment.go:334`; `service/history/replication/task_executor.go:400`; shard I/O low priority at `service/history/shard/context_impl.go:1646` |
| CHASM internal coordination | Scheduler invoker tasks coordinate concurrent results with local `sync.Mutex` guards (no cross-component isolation) | `chasm/lib/scheduler/invoker_tasks.go:281`, `:323`, `:371` |
| Tests: locks | Concurrent acquire/release correctness and blocking-release scenarios for semaphores and mutexes | `common/locks/priority_semaphore_test.go:53-100`; `common/locks/id_mutex_test.go` |
| Tests: deadlock detector | Suspected-deadlock counter/gauge transitions 0→1→0 against a deliberately blocked ping | `common/deadlock/deadlock_test.go:32-77` |
| Tests: handover isolation | `IsInHandover` true/false assertions around replication handover state | `service/history/shard/handover_events_test.go:161-165` |

## Answers to Dimension Questions

1. **Which resources are shared?**
   - Workflow executions and their mutable state (event histories), mutated by APIs and background task executors alike (`service/history/workflow/context.go:40-59`).
   - Shard contexts: the shard metadata row, RangeId lease, and task-key allocator shared by all workflows in an ID range (`service/history/shard/context_impl.go:136-147`).
   - Task queue partitions: matcher data, backlog levels, and the persisted queue metadata row (`service/matching/db.go:36-55`).
   - The namespace registry caches shared across request handlers (`common/namespace/nsregistry/registry.go:104-137`).
   - In-memory pagination buffers for long-poll workflow-task completion, bounded process-wide (`service/history/workflow/context.go:52-58`).
   - Files, shell, browser, secrets: not applicable — the server executes no user code; those resource classes do not exist in this source.

2. **What protects them?**
   - A capacity-1 `PrioritySemaphore` per workflow context (`service/history/workflow/context.go:116`), reached only through the cache's lock-and-release protocol (`service/history/workflow/cache/cache.go:305-325`).
   - A shard `rwLock` plus `ioSemaphore` bounding conflicting persistence ops (`service/history/shard/context_impl.go:123-137`).
   - Lease fencing: `RangeId` renewal with drain-before-renew semantics (`service/history/shard/context_impl.go:1162-1206`) and queue-level equivalents (`service/matching/db.go:150-174`).
   - Database conditional writes as the final arbiter for shards, queues, and executions (`common/persistence/cassandra/shard_store.go:37`; `matching_task_store_queue.go:77-115`; `mutable_state_store.go:25-31`).
   - Namespace-scoped rejection during global-namespace failover (`service/history/shard/context_impl.go:1148-1149`).

3. **Are locks coarse or fine-grained?**
   Fine-grained where it matters: one lock per workflow *run* (not per namespace or shard), per-partition managers for task queues, hash-sharded `IDMutex` for arbitrary keys (`common/locks/id_mutex.go:18-28`). Coarse-grained locking appears only at shard level (`rwLock`), which is intrinsic to lease/failover semantics rather than sloppiness; the `ioSemaphore` keeps coarse shard locking from throttling unrelated persistence traffic.

4. **Can deadlocks occur?**
   The design actively prevents and detects them rather than claiming impossibility:
   - All blocking acquisitions take a `context.Context` and abort on cancellation (`common/locks/priority_semaphore_impl.go:83-121`); lock waits are clamped to deadlines (`service/history/workflow/cache/cache.go:333-349`).
   - Two documented ordering rules: never acquire `ioSemaphore` while holding `rwLock` (`service/history/shard/context_impl.go:123-128`); acquire `readthroughLock` before `nsMapsLock` (`common/namespace/nsregistry/registry.go:125-129`).
   - A runtime detector pings the hottest locks on an interval and can dump goroutines, fail health checks, or abort the process (`common/deadlock/deadlock.go:111-128`), with probe timeouts derived from real operation budgets (`service/history/shard/context_impl.go:238-263`).
   Residual risk: ordering between the many remaining lock pairs is convention, not enforced, so a future violation would surface via the detector rather than be prevented.

5. **Are resource conflicts visible?**
   Yes, at three layers. Lock contention: `AcquireLockFailedCounter` and hold-duration histograms (`service/history/workflow/cache/cache.go:315-316`, `:372`). Deadlock suspicion: `DDSuspectedDeadlocks` counter and current-gauge (`common/deadlock/deadlock.go:114`, `:148-151`) plus per-lock ping-latency metrics (`service/history/shard/context_impl.go:250`, `:262`). Persistence conflicts: failed CAS batches are decomposed into typed, priority-ranked errors so operators see *which* invariant lost the race (`common/persistence/cassandra/errors.go:88-104`, `:146-159`).

## Architectural Decisions

- **Priority semaphore instead of plain mutex for workflows** (`service/history/workflow/context.go:116`): capacity 1 gives mutual exclusion, but two priority classes let frontend API calls preempt background timer/replication processing waiting on the same workflow — a deliberate availability choice for interactive traffic (call sites listed in the evidence table).
- **Lease/epoch fencing at every ownership boundary**: shard ownership (`service/history/shard/context_impl.go:1180-1184`) and queue ownership (`service/matching/db.go:158-166`) both bump a monotonically increasing `RangeId` validated by the DB, so stale owners self-detect via `ShardOwnershipLostError` instead of corrupting state.
- **In-process single-writer gate ahead of DB CAS for task queues** (`service/matching/db.go:90-95`): a workaround for a concrete storage-engine pathology (concurrent LWTs to one Cassandra partition timing out), showing storage-aware lock placement.
- **Cache-mediated locking** (`service/history/workflow/cache/cache.go:304-325`): lock acquisition is fused with an LRU `PutIfNotExist`, so there is exactly one locked context instance per workflow per host, with eviction-time finalizers re-acquiring the lock at high priority to clear state safely.
- **Namespace as the tenancy/isolation boundary** (`service/frontend/workflow_handler.go:458-461`): logical isolation of all resources, with failover-specific rejection (`ErrNamespaceHandover`) rather than per-request ACL-style checks.
- **Deadlock detection as a first-class service**: pluggable `pingable.Pingable` roots wired via fx DI groups (`common/deadlock/deadlock.go:21-31`), letting each subsystem register its critical locks with appropriate budgets.

## Notable Patterns

- **Fencing token (RangeID)** applied uniformly to shards and task queues; DB templates enforce it (`IF range_id = ?`) so application bugs cannot bypass it.
- **Optimistic concurrency control** on workflow rows: writers pre-commit expected versions/event IDs and lose gracefully with typed errors (`common/persistence/cassandra/errors.go:106-144`); the SQL backend mirrors this with row locks plus Go-side condition checks (`common/persistence/sql/execution_util.go:573-596`).
- **Double-checked locking** for shard context creation in the controller's shard map (`service/history/shard/controller_impl.go:39-41`, RWMutex-guarded map).
- **Idempotent, panic-safe resource release**: the release closure uses CAS-once semantics, recovers panics, force-clears dirty state, and still guarantees unlock exactly once (`service/history/workflow/cache/cache.go:366-405`).
- **Priority inversion avoidance by design**: low-priority background holders do not block high-priority waiters indefinitely because the semaphore's wait lists admit high-priority acquirers first when tokens free up (`common/locks/priority_semaphore_impl.go:46-58`, `:102-121`).
- **Drain-before-fence**: `renewRangeLocked` drains in-flight task requests before bumping the epoch so conditioned requests fail cleanly rather than racing (`service/history/shard/context_impl.go:1163-1169`).

## Tradeoffs

- **Throughput vs. correctness for hot workflows**: all mutations of one workflow serialize behind a single token; a slow update delays everything for that run. This is intentional (event-history linearizability) but caps per-run parallelism.
- **Fail-fast over wait-long**: deadline clamping turns heavy contention into `ErrResourceExhaustedBusyWorkflow` responses (`service/history/workflow/cache/cache.go:351-354`) — predictable latency, but clients must retry.
- **Comment-enforced lock ordering**: the two documented ordering rules (`service/history/shard/context_impl.go:123-128`; `common/namespace/nsregistry/registry.go:125-129`) depend on developer discipline; nothing mechanical rejects a violating new call site.
- **Detector aggressiveness is configurable but risky**: `AbortProcess` mode fatals the process on a suspected deadlock (`common/deadlock/deadlock.go:125-127`), which trades availability for fast failure if the timeout budget is mis-tuned.
- **No sandboxing of executed logic in-server**: isolation stops at namespaces and locks; memory/CPU isolation of workflow code is delegated entirely to external workers (by architecture, but it means this source alone does not satisfy the full "sandbox boundaries" step of the dimension).

## Failure Modes / Edge Cases

- **Lock holder stalls on persistence outage**: the shard `rwLock` may be held for a full renew-cycle duration; the deadlock detector's probe timeout is sized to `ShardIOTimeout()+30s` to distinguish slow I/O from a true deadlock (`service/history/shard/context_impl.go:239-242`).
- **Ownership stolen mid-operation**: after failover, the old host's writes fail the `range_id` precondition and surface as `ShardOwnershipLostError`, ranked highest among conflict errors so callers unload instead of retrying blindly (`common/persistence/cassandra/errors.go:116-122`, `:146-159`).
- **Panic while holding a workflow lock**: recovered by the release func, which clears state and unlocks before re-panicking, preventing a permanently poisoned cache entry (`service/history/workflow/cache/cache.go:374-378`).
- **Dirty state detected post-release**: clearing plus a hard panic (`cache.go:386-400`) — fail-stop rather than serving potentially inconsistent history.
- **Global-namespace failover window**: operations touching handover namespaces are rejected outright rather than allowed to race replication state (`service/history/shard/context_impl.go:1148-1149`).
- **Cancellation race in semaphore acquire**: an acquirer canceled after being granted tokens returns them and reports failure (`common/locks/priority_semaphore_impl.go:123-130` region), avoiding leaked tokens.
- **Unlocking an unknown ID**: `IDMutex.UnlockID` panics on a missing entry (`common/locks/id_mutex.go:89-91`) — programmer-error detection over silent corruption.

## Future Considerations

- Mechanize lock-ordering rules (e.g., a debug-mode order-tracking wrapper or lint check) so the ioSemaphore/rwLock and readthrough/nsMaps invariants survive refactors without relying on comments.
- Expose per-namespace/per-workflow-key lock-wait metrics beyond aggregate counters, since tags already exist on the workflow context logger/metrics (`service/history/workflow/context.go:102-114`).
- Consider adaptive lock-timeout budgets: the fixed 500 ms API tail (`workflowLockTimeoutTailTime`, `service/history/workflow/cache/cache.go:343`) could track observed p99 hold durations instead.

## Questions / Gaps

- **Sandbox boundaries**: no evidence of process-, container-, or memory-based sandboxing anywhere in this source (searched `chasm/`, `service/worker/`, and sandbox-named symbols; only mock/test mutexes matched, e.g. `chasm/lib/scheduler/invoker_tasks.go:281`). This is consistent with Temporal's architecture — worker-side sandboxing lives in the Temporal SDK repositories, outside this source's boundary — but it means this repo cannot answer the sandboxing step of the dimension on its own.
- **Cross-service lock visibility**: no evidence found of dashboards/alerts wiring for the deadlock-detector metrics beyond their metric definitions (`common/deadlock/deadlock.go:114`, `:150`); operational runbooks were not inspected (docs directory contains no lock-operations guide found in search).
- **Fairness of `IDMutex`**: the sharded ID mutex has no priority parameter (`common/locks/id_mutex.go:11-15`); whether any production path needs fair ordering there was not determinable from code alone.

---

Generated by `07.05-resource-locking-and-isolation` against `temporal`.
