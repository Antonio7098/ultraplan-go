# Source Analysis: temporal

## 02.09 — State Pruning, Compaction, and Retention

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.24 (server for [Temporal](https://github.com/temporalio/temporal); persistence backends: Cassandra, MySQL/PostgreSQL/SQLite; visibility stores: Elasticsearch, SQL) |
| Analyzed | 2026-07-13 |

> Scope note: in-scope source is the Temporal server code itself; persistence backends and the `temporalio` SDK clients live elsewhere and are assessed only as their behavior is invoked from this tree.

## Summary

Temporal encodes retention as a **per-namespace, per-workflow-execution** lifetime that is set when the workflow is closed and enforced by a multi-stage deletion pipeline. Close-time machine work appends a `DeleteHistoryEventTask` to the timer queue at `closeTime + ns.Retention() + jitter` (`service/history/workflow/task_generator.go:357-367`); firing that task walks a five-stage idempotent delete (`service/history/shard/context_impl.go:914-1094`) covering visibility, replication, current-pointer, mutable state, and history branch. Independent background **scavengers** close the loops the timer path can miss: a `TaskQueue` scavenger drops empty/idle queues (`service/worker/scanner/taskqueue/scavenger.go:60-65`), a `History` scavenger reconciles orphan branches (`service/worker/scanner/history/scavenger.go:118-133`) and can also call `adminClient.DeleteWorkflowExecution` when `worker.historyScannerVerifyRetention` is true (`service/worker/scanner/history/scavenger.go:329-378`), and a Cassandra-only `Executions` scavenger validates mutable-state invariants and force-deletes retention-past-workflows (`service/worker/scanner/executions/task.go:206-253`). Visibility itself is not auto-pruned by the server: SQL visibility rows are deleted only via the explicit `DeleteWorkflowExecution` API (`common/persistence/visibility/store/sql/visibility_store.go:182-194`); Elasticsearch relies on the `search.idle.after: 365d` index setting plus externally-managed ILM (`schema/elasticsearch/visibility/index_template_v7.json:9`). Conversation/state "summarization" does not exist for the workflow event history (history is append-only and immutable); the only compaction-style mechanisms operate on the timer-queue slice layer (`service/history/queues/action_slice_count.go:37-100`) and on the HSM operation log (`service/history/hsm/tree.go:254-269`). Privacy deletion is supported through `Admin.DeleteWorkflowExecution` (`service/frontend/admin_handler.go:1915-1949`), `forceDeleteWorkflowExecution` (`service/history/api/forcedeleteworkflowexecution/api.go:20-147`), and the `DeleteNamespace` workflow pipeline (`service/worker/deletenamespace/...`). Bounded growth is otherwise enforced by per-execution caps (`limit.historySize.*`, `limit.historyCount.*`, `limit.mutableStateSize.*` — `common/dynamicconfig/constants.go:380-426`) that push authors toward `ContinueAsNew` rather than deleting.

## Rating

**8 / 10** — Retention has a clear, multi-layer model with explicit per-namespace configuration (`common/namespace/namespace.go:289-296`, `common/namespace/const.go:6-13`), a validated mutator (`service/frontend/namespace_handler.go:1111-1128`), idempotent staged deletion (`service/history/shard/context_impl.go:914-1094`), and three background scavengers backed by dynamic-config tunables (`common/dynamicconfig/constants.go:3242-3273`). Tests exist for each layer (`service/worker/scanner/history/scavenger_test.go:573-672`, `service/history/deletemanager/delete_manager_test.go:175-234`). The mechanisms are observable (metrics, heartbeats, logger fields) and tolerant of retries/scattered shards. Weaknesses: history events are never compacted or summarized (they are immutable); visibility TTL is not server-managed, so retention on `temporal_visibility_v1*` indices depends on Elasticsearch ILM that the codebase merely recommends (`schema/elasticsearch/visibility/index_template_v7.json:9`); the executions scavenger is disabled by default and explicitly unsupported on SQL persistence (`common/dynamicconfig/constants.go:3257-3262`, `service/worker/scanner/scanner.go:172-178`); Cassandra TTLs are capped at 10 years due to the year-2038 limit (`common/persistence/cassandra/execution_store.go:71-77`). These bounds make "run forever" plausible for typical operators but not literally a year-spanning guarantee without operator care.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-namespace retention accessor | `Namespace.Retention()` reads `config.Retention.AsDuration()`. | `common/namespace/namespace.go:289-296` |
| Min-retention floors | `MinRetentionGlobal = 1 * 24 * time.Hour`, `MinRetentionLocal = 1 * time.Hour` and exposed as `NamespaceMinRetentionGlobal` / `NamespaceMinRetentionLocal` dynamic config. | `common/namespace/const.go:6-13`; `common/dynamicconfig/constants.go:208-217` |
| Default workflow retention fallback | `defaultWorkflowRetention = 1 * 24 * time.Hour` used when namespace is unknown at delete-time. | `service/history/workflow/task_generator.go:107`; `service/history/workflow/task_generator.go:283-296` |
| Retention mutator + namespace validator | `WithRetention` clone helper; `validateRetentionDuration` rejects values below global/local minimum. | `common/namespace/mutate.go:60-66`; `service/frontend/namespace_handler.go:1111-1128` |
| Retention write into namespace config on register/update | `registerRequest.GetWorkflowExecutionRetentionPeriod()` flows into `config.Retention`; updates overwrite `config.Retention` after validation. | `service/frontend/namespace_handler.go:196-212`; `service/frontend/namespace_handler.go:478-486` |
| Close-time delete-history timer | `GenerateDeleteHistoryEventTask` adds a `DeleteHistoryEventTask` with `VisibilityTimestamp = closeTime + retention + jitter`. Category=Timer, type=`TASK_TYPE_DELETE_HISTORY_EVENT`. | `service/history/tasks/workflow_cleanup_timer.go:14-25`; `service/history/workflow/task_generator.go:339-368` |
| Jitter to spread timer fan-out | `RetentionTimerJitterDuration` configured at 30m, applied via `backoff.FullJitter`. | `common/dynamicconfig/constants.go:2074-2078`; `service/history/workflow/task_generator.go:357-358` |
| Close-time archival path | If archival enabled, schedules `ArchiveExecutionTask` (bounded delay ≤ retention) and only generates the delete-history timer from the archival queue; otherwise the timer is added inline at close. | `service/history/workflow/task_generator.go:245-269` |
| Archival → enqueues the same delete-history timer after archive | `addDeletionTask` calls `GenerateDeleteHistoryEventTask` and pushes it via `shardContext.AddTasks`. | `service/history/archival_queue_task_executor.go:222-256` |
| Timer-queue executor for delete-history task | `executeDeleteHistoryEventTask` re-loads mutable state; if not found falls back to deleting the orphan branch; if found, calls `DeleteManager.DeleteWorkflowExecutionByRetention` with the persisted `ProcessStage`. | `service/history/timer_queue_task_executor_base.go:81-147` |
| Five-stage idempotent delete driver | `ContextImpl.DeleteWorkflowExecution` enumerates stages (`Visibility`, `Replication`, `Current`, `MutableState`, `History`) and skips already-processed work; stages are persisted via `DeleteWorkflowExecutionStage` bitmask. | `service/history/shard/context_impl.go:903-1095`; `service/history/tasks/delete_workflow_execution_stage.go:10-26` |
| Stage 1: visibility delete task | Inserts `DeleteExecutionVisibilityTask{IsRetentionDelete: retentionDelete}` into the visibility queue; piggybacks delete-execution replication task onto same write. | `service/history/shard/context_impl.go:980-1029` |
| Visibility delete task processing | `processDeleteExecution` in `visibilityQueueTaskExecutor` honors `IsRetentionDelete`, optionally waits for close visibility to drain, then calls `visibilityMgr.DeleteWorkflowExecution`. | `service/history/visibility_queue_task_executor.go:318-351` |
| Stage 2/3: drop current pointer and mutable state | `DeleteCurrentWorkflowExecution` + `DeleteWorkflowExecution` on the `ExecutionManager`. | `service/history/shard/context_impl.go:1031-1074` |
| Stage 4: history branch | `DeleteHistoryBranch` on `ExecutionManager` (skipped if `branchToken` empty). | `service/history/shard/context_impl.go:1082-1093` |
| Cross-cluster skipping of replication stage | `DeleteWorkflowExecutionByRetention` calls `stage.MarkProcessed(tasks.DeleteWorkflowExecutionStageReplication)` — each cluster retains and deletes independently on its own clock. | `service/history/deletemanager/delete_manager.go:138-160` |
| Delete-manager thunk | `deleteWorkflowExecutionInternal` wraps the four-stage flow; emits `WorkflowCleanupDeleteCount` metric; clears `weCtx` to prevent stale reads. | `service/history/deletemanager/delete_manager.go:162-206` |
| Visibility close-workflow cleanup (Elasticsearch only) | `enableCloseWorkflowCleanup` + `relocateAttributesMinBlobSize` lets the visibility processor move memo/search-attributes out of mutable state after close. | `common/dynamicconfig/constants.go:2320-2332`; `service/history/visibility_queue_task_executor.go:304-316` |
| User-driven deletion API | `AdminHandler.DeleteWorkflowExecution` calls `historyClient.ForceDeleteWorkflowExecution`. | `service/frontend/admin_handler.go:1915-1949` |
| Force-delete backend | `forcedeleteworkflowexecution.Invoke` deletes current pointer, mutable state, history branch(es), and visibility record. Marks `IsRetentionDelete: false`. | `service/history/api/forcedeleteworkflowexecution/api.go:105-140` |
| Delete-execution replication across cluster | Uses `DeleteExecutionTask` (transfer-category) so the delete is replicated rather than fired locally; retention path skips replication, explicit delete replicates. | `service/history/tasks/delete_execution_task.go`; `service/history/tasks/workflow_cleanup_timer.go`; `service/history/deletemanager/delete_manager.go:148-160` |
| Background scanner daemon wiring | `worker/scanner` is its own subsystem; runs SDK workers for: executions (Cassandra only), task queue (SQL only), history, buildIds, schedule invariants. | `service/worker/scanner/scanner.go:117-290` |
| History scavenger | `Scavenger.Run` paginates `GetAllHistoryTreeBranches`, filters by `historyDataMinAge`, deletes branches whose mutable state is missing. Optional `cleanUpWorkflowPastRetention` issues `adminClient.DeleteWorkflowExecution` when retention+buffer has passed. | `service/worker/scanner/history/scavenger.go:118-378` |
| History scavenger config | `HistoryScannerEnabled`, `HistoryScannerDataMinAge=60d`, `HistoryScannerVerifyRetention=true`. | `common/dynamicconfig/constants.go:3252-3273` |
| Executions scavenger (Cassandra only) | `Scavenger.run` submits one task per shard; `task.validate` runs `mutableStateValidator` then `historyEventIDValidator`. | `service/worker/scanner/executions/scavenger.go:142-174`; `service/worker/scanner/executions/task.go:86-187` |
| Mutable-state retention validator | `validateRetention` flags `mutableStateRetentionFailureType` when `time.Since(lastUpdate) > ns.Retention() + executionDataDurationBuffer()`. Uses `LastUpdateTime` (not close time) for legacy compatibility. | `service/worker/scanner/executions/mutable_state_validator.go:231-261` |
| Executions scavenger enforces retention via admin | `handleFailures` calls `adminClient.DeleteWorkflowExecution` on retention failures. Treats namespace-not-found as no-op with a TODO referencing #3536. | `service/worker/scanner/executions/task.go:206-253` |
| Build-Id scavenger | Removes build IDs unreferenced for `RemovableBuildIdDurationSinceDefault`. | `service/worker/scanner/build_ids/scavenger.go` (see `service/worker/scanner/scanner.go:192-216`) |
| Schedule invariants scanners | Cron-style workflows (`OverdueNextActionTime`, `StuckOpen`, `UnknownState`) flag anomalies; configurable via `ScheduleInvariantsScannerOptions`. | `service/worker/scanner/scanner.go:218-270`; `service/worker/scanner/scheduleinvariants/...` |
| Task-queue scavenger | Deletes tasks past `ExpiryTime` and entire queues idle for `taskQueueGracePeriod = 48h`. | `common/persistence/cassandra/matching_task_store_queue.go:184-216`; `service/worker/scanner/taskqueue/scavenger.go:59-66` |
| Task-queue tombstone retention | Versioning-row retention `MatchingDeletedRuleRetentionTime` (currently 30s) for deleted assignment rules. | `common/dynamicconfig/constants.go:492-495` |
| Namespace-delete pipeline | `Operator.DeleteNamespace` (with `NamespaceDeleteDelay`) launches the `DeleteNamespaceWorkflow`; `DeleteExecutionsWorkflow` iterates visibility then calls `historyClient.DeleteWorkflowExecution` (or `ForceDeleteWorkflowExecution` for chasm). | `service/frontend/operator_handler.go:546-601`; `service/worker/deletenamespace/workflow.go`; `service/worker/deletenamespace/deleteexecutions/workflow.go:89-222`; `service/worker/deletenamespace/deleteexecutions/activities.go:117-219` |
| Visibility TTL on Elasticsearch | `search.idle.after: 365d` makes indices idle after a year; no built-in ILM (server delegates). | `schema/elasticsearch/visibility/index_template_v7.json:9-13` |
| Cassandra TTL ceiling | `maxCassandraTTL = 315360000` (10y) due to year-2038 issue, applied for task TTLs and analogous limits. | `common/persistence/cassandra/execution_store.go:67-77` |
| Per-execution growth caps | `limit.historySize.error=50MB`, `limit.historySize.suggestContinueAsNew=4MB`; `limit.historyCount.error=50k`, etc. — push authors to `ContinueAsNew` rather than delete. | `common/dynamicconfig/constants.go:380-446` |
| Per-task jitter for delete timer | `RetentionTimerJitterDuration = 30 * time.Minute` per-namespace setting. | `common/dynamicconfig/constants.go:2074-2078` |
| Compression of history events | History events are stored as compressed blobs (`compression/compression.go`-style `serializeWorkflowEvents`); this is payload size compaction, not delete-history. | `common/persistence/execution_manager.go:606-628` |
| HSM operation-log compaction | `tree.OpLog()` returns a compacted op log; `OperationLog.compact()` drops operations on deleted paths so retention-deferred HSM state stays small. | `service/history/hsm/tree.go:252-269`; `service/history/hsm/tree.go:709-714` |
| Queue-slice pruning/merging | `actionSliceCount` shrinks slices and force-merges when total slice count exceeds `CriticalSliceCount`; explicit 4-stage ordering. | `service/history/queues/action_slice_count.go:37-100` |
| Memory/TTL tombstone caps | `MutableStateTombstoneCountLimit` bounds tombstones that deletion leaves in mutable state. | `common/dynamicconfig/constants.go:427-431` (and surrounding limiter block) |

## Answers to Dimension Questions

### 1. What grows forever?

- **Workflow execution history events**: append-only and immutable while the workflow is alive; for running executions, history is bounded only by `limit.historySize.*` / `limit.historyCount.*` and recommended `ContinueAsNew` ([`common/dynamicconfig/constants.go:380-417`]).
- **Visibility records** for active namespaces (no server-side TTL): SQL visibility rows are deleted only on explicit `DeleteWorkflowExecution`; ES relies on external ILM.
- **Task-queue rows** only expire by `ExpiryTime` or via the task-queue scavenger; no retention-driven paging for live queues.
- **`history_task` queues** (timer/transfer/visibility/scheduled) accumulate without a built-in compaction cadence beyond the slice-count `actionSliceCount` triggered by reading (not retention).
- **Audit retention is not server-managed** beyond what each backend supports (Cassandra 10y TTL ceiling).

### 2. What gets summarized?

- **Nothing on the workflow event stream.** Events are immutable; visibility "cleanup" merely strips memo/search attributes when `VisibilityProcessorEnableCloseWorkflowCleanup` is on, which is relocation, not summarization ([`service/history/visibility_queue_task_executor.go:304-316`]).
- **HSM operation log** is compacted to drop operations on deleted paths via `OperationLog.compact()` ([`service/history/hsm/tree.go:709-714`]).
- **No LLM-style conversation summarization.** The dimension concept of "conversation summarization" does not map: agents are reconstructed from history events on demand.

### 3. What gets deleted?

- **Mutable state**, **current workflow pointer**, **history branch(es)** via the five-stage delete driver ([`service/history/shard/context_impl.go:903-1094`]).
- **Visibility records** via `DeleteExecutionVisibilityTask` (visibility queue, retried) and the sync visibility drop in `ForceDelete*` ([`service/history/api/forcedeleteworkflowexecution/api.go:130-140`]).
- **Replica mutations** via `DeleteExecutionReplicationTask` for explicit deletes (skipped for retention deletes so each cluster self-deletes; [`service/history/deletemanager/delete_manager.go:138-160`]).
- **Idle task queues + expired tasks** in the task-queue scavenger ([`service/worker/scanner/taskqueue/scavenger.go:60-65`]).
- **Orphan history branches** by the history scavenger ([`service/worker/scanner/history/scavenger.go:118-289`]).
- **Chasm execution data + unused build IDs** by the build-ID scavenger.
- **All executions of a deleted namespace** by `DeleteNamespaceWorkflow` ([`service/worker/deletenamespace/deleteexecutions/workflow.go:89-222`]).

### 4. Does compaction break replay?

- Workflow history compaction is by **deletion, not summarization**: when all history is dropped, replay is no longer possible — but that is by design after retention has elapsed. The `BuildHistoryGarbageCleanupInfo` annotation lets the history scavenger sanity-check that no live mutable state references the branch ([`common/persistence/data_interfaces.go:1420-1427`], [`common/persistence/execution_manager.go:622-625`]).
- Within-history "compaction" is nonexistent — events are appended immutably and never merged.
- HSM operation-log compaction is replay-safe by construction: it only drops operations on already-deleted paths.

### 5. Can users request deletion?

- **Yes**: `Admin.DeleteWorkflowExecution` (frontend admin RPC, force-delete) at [`service/frontend/admin_handler.go:1915-1949`].
- **Yes for namespaces**: `Operator.DeleteNamespace` at [`service/frontend/operator_handler.go:546-601`], honored by a delay-then-execute child workflow.
- **Indirectly via the SDK**: `client.SignalWithStart`/etc. cannot trigger delete, but operators can invoke via `temporal` CLI which wraps the admin RPC. There is no built-in RPC-level "delete my own history"; users must invoke the admin endpoint or wait for retention.

## Architectural Decisions

- **Retention is a per-namespace time, not a per-execution or global one.** `Namespace.Retention()` ([`common/namespace/namespace.go:289-296`]) is the only knob a workflow sees; global/local minima are enforced at write time ([`common/namespace/const.go:6-13`], [`service/frontend/namespace_handler.go:1111-1128`]).
- **Delete is staged + idempotent.** Five bitmasked stages on `DeleteWorkflowExecutionStage` ([`service/history/tasks/delete_workflow_execution_stage.go:10-26`]) let the timer-queue processor retry partial work safely. Cross-cluster replication is in a separate stage so retention deletes don't cross clusters.
- **Two coordinating deletion paths:** scheduled (`DeleteHistoryEventTask`) and scanning (scavenger). Both call the same admin RPC for tombstone-style deletes.
- **Visibility and history deletion are decoupled.** Visibility records delete via a queue; mutable state and history branches delete via shard-context staging. This prevents runaway retention loops but produces a small window where a workflow is invisible but still has a branch in DB.
- **History events are not summarized.** Compaction is delegated to durable upstream archival (`common/archiver/`); retention's job is only to bound the duration the primary store is authoritative.
- **CHASM-aware.** `ArchetypeID` plumbs through every delete path ([`service/history/deletemanager/delete_manager.go:184-196`], [`service/history/tasks/workflow_cleanup_timer.go:63-65`], [`service/worker/deletenamespace/deleteexecutions/activities.go:189-216`]) so non-workflow archetypes are subject to the same lifecycle.
- **Defensive jitter.** `RetentionTimerJitterDuration` prevents a thundering herd of deletes at the boundary.

## Notable Patterns

- **Bitmask-stage delete driver** as a general-purpose idempotent replay pattern.
- **Polling workflow ** `DeleteExecutionsWorkflow` with `ContinueAsNew` to bound memory while iterating millions of executions ([`service/worker/deletenamespace/deleteexecutions/workflow.go:89-222`]).
- **Pagination iterator + heartbeats** in both scavengers ([`service/worker/scanner/history/scavenger.go:135-172`], [`service/worker/scanner/executions/task.go:93-132`]) for Activity heartbeat survival and resumption.
- **SDK-as-runtime scanner.** The scanner subsystem runs as its own SDK worker with cron-style workflows ([`service/worker/scanner/scanner.go:156-290`]); this lets TimeSkipping apply naturally for backfill and makes progress observable via Temporal's own machinery.
- **Per-shard load distribution** (executions scavenger submits one task per shard; [`service/worker/scanner/executions/scavenger.go:148-171`]).
- **Validator + admin-RPC bridge.** Validators are pure; the actual delete call happens in `handleFailures` so validators remain simple and side-effect-free.
- **At-most-NK tombstones in mutable state** via `MutableStateTombstoneCountLimit` so deleting-then-continuing-as-new doesn't blow up state.

## Tradeoffs

- **No history summarization = no history-size cap during execution**. The only "keep going" mechanism is human-driven `ContinueAsNew`; long-running workflows can hit `limit.historySize.error` and refuse to take more commands. This is intentional: Temporal wants replay fidelity rather than lossy compaction.
- **Retention cannot shrink the visibility store by itself on SQL** (no TTL); ES relies on out-of-band ILM (with `search.idle.after=365d` as the only server-side knob; [`schema/elasticsearch/visibility/index_template_v7.json:9-13`]).
- **Executions scavenger is Cassandra-only.** Operators on MySQL/PostgreSQL/SQLite lose the safety net that catches executions whose `DeleteHistoryEventTask` was lost (e.g., due to a bug or shard loss). The `ExecutionsScannerEnabled` flag is `false` by default for SQL.
- **Cassandra 10y TTL ceiling** indirectly caps retention-style storage ([`common/persistence/cassandra/execution_store.go:67-77`]) — surprising for operators assuming infinite timelines.
- **Cross-cluster retention is *not* coordinated** by design (each cluster self-deletes). This simplifies correctness but means a cluster offline during its retention boundary loses data after a future active handover.
- **Force-delete uses `IsRetentionDelete=false`**, which means admin-driven deletes affect replication where retention deletes don't.
- **`NamespaceNotFound` on retention delete is a silent no-op** in both scavengers — leaving garbage in DB after namespace delete ([`service/worker/scanner/executions/task.go:217-225`], [`service/worker/scanner/history/scavenger.go:340-343`]).

## Failure Modes / Edge Cases

- **Shard loss + retention timer lost**: mutable state may never be deleted; mitigated by `ExecutionsScavenger` (Cassandra only) and `HistoryScavenger` "branch" path.
- **Namespace already deleted when timer fires**: `getRetention` falls back to `defaultWorkflowRetention = 1 * 24 * time.Hour` ([`service/history/workflow/task_generator.go:107`], [`service/history/workflow/task_generator.go:283-296`]). The stage driver skips visibility deletion when namespace is not found but continues with mutable state/branch ([`service/history/shard/context_impl.go:937-945`]).
- **Cross-DC resurrection**: an active cluster can recreate a workflow whose `DeleteHistoryEventTask` already fires; the timer-queue executor explicitly ignores deletes for running workflows and falls back to branch-only delete ([`service/history/timer_queue_task_executor_base.go:124-129`]).
- **Self-reference loop**: history events persist with `Info = BuildHistoryGarbageCleanupInfo(nsID,wfID,runID)`; the scavenger parses it back to know which mutable state to consult ([`common/persistence/data_interfaces.go:1420-1427`]).
- **Failed archival URI**: archival queue retries forever; metric `ArchivalTaskInvalidURI` is the warning ([`service/history/archival_queue_task_executor.go:158-169`]).
- **Reprocessing during compaction**: queue slice merge (`actionSliceCount`) reprocesses tasks in universal-predicate slices; explicit ordering limits how many namespaces are affected per merge ([`service/history/queues/action_slice_count.go:53-100`]).
- **Force-delete on a non-existent run** can produce `WorkflowExecutionAlreadyStarted`-class errors that are swallowed with a warning, not a hard fail ([`service/history/api/forcedeleteworkflowexecution/api.go:61-93`]).
- **CHASM executions bypass retention validation** explicitly; TODO comment notes future work ([`service/worker/scanner/executions/mutable_state_validator.go:65-70`]).

## Future Considerations

- A server-driven SQL visibility TTL/compaction job would close the largest gap.
- Extend the executions scavenger to support SQL persistence (TODOs in [`common/dynamicconfig/constants.go:3257-3262`]).
- Tighten behavior on `NamespaceNotFound` during retention deletion (the open `issue #3536` referenced in the scavenger).
- Add a metric for "near-retention-but-not-yet-deleted" so operators can spot failing timers (only stage-completion metrics exist today).
- Move force-delete visibility deletion to async with retry — currently `ForceDeleteWorkflowExecution` returns success even if visibility deletion is best-effort ([`service/history/api/forcedeleteworkflowexecution/api.go:127-139`]).

## Questions / Gaps

- No evidence found for a configurable upper bound on per-namespace retention at the cluster level (validation only rejects values below `MinRetentionGlobal`); very large retentions are accepted as-is, which can outrun any storage backend.
- No evidence found for an in-server SQL visibility index/partition rotation policy (other than the per-namespace table layout in `common/persistence/sql/...`).
- No visibility-wide TTL/delete-by-time API was found in `common/persistence/visibility/store/...`; only row-level delete exists.
- The relationship between the `DeleteExecutionTask` category (transfer) and the visibility/replication ordering is described but not enforced by schema; understanding requires tracing three executors.
- ES ILM lifecycle is recommended behavior, not enforced: operators must configure it; the server emits no warnings if the index is left unmanaged.
