# Source Analysis: temporal

## 01.10 Replay and Determinism in Execution

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server, the durable execution platform itself; `go.mod` declares a multi-module server with services frontend/history/matching/worker) |
| Analyzed | 2026-07-03 |

## Summary

Temporal's server implements **deterministic, event-sourced replay** as the single primitive used to reconstruct any workflow's mutable state from its history branch. The append-only event log in the persistence store (`common/persistence/history_manager.go:36-127` for `ForkHistoryBranch`, `:518-578` for `ReadHistoryBranch`) is the source of truth; mutable state is rebuilt by streaming those events back through `MutableStateRebuilderImpl.ApplyEvents` (`service/history/workflow/mutable_state_rebuilder.go:70-101`). The same primitive powers five distinct replay paths: workflow reset (`service/history/ndc/workflow_resetter.go:434-508`), replication gap-filling and conflict resolution (`service/history/ndc/conflict_resolver.go:116-183`, `service/history/ndc/history_replicator.go:210-231`), the admin "RebuildMutableState" recovery API (`service/history/workflow_rebuilder.go:64-104`), and cross-cluster replication itself. Each replay is deterministic because the events are recorded and applied in order, and the rebuilder exposes one explicit hook — `SetReplayEventBatchID` (`service/history/workflow/mutable_state_impl.go:1369-1395`) — to keep event load tokens stable during the replay path. The CHASM subsystem is an explicit exception: `chasm/lib/workflow/workflow.go:177-178` states "Unlike HSM callbacks, CHASM replicates entire trees rather than replaying events, so deterministic cross-cluster IDs based on event version are not needed." Server replay never re-executes user workflow code or LLMs; LLM/model calls live on the SDK worker side and the SDK does its own time-travel replay (e.g. `service/worker/scheduler/replay_test.go:20-42`, `service/worker/workerdeployment/replaytester/replay_test.go:24-59`) using recorded histories to detect non-determinism. Because the server is event-sourced and never calls models, every server-side replay produces identical mutable state; the audit trail (history) and the reconstructed state are the same thing.

## Rating

**9 / 10** — Temporal's replay model is mature, explicit, and operationally hardened. Server replay is event-driven, fully deterministic, and uses five independent code paths (reset, replication, conflict resolution, admin rebuild, import) that all converge on the same `MutableStateRebuilder` primitive. Replay tests are extensive (`service/history/workflow/mutable_state_rebuilder_test.go` has 50+ `TestApplyEvents_EventType*` cases per event type at lines 164-2158+, plus dedicated reset and rebuilder suites at `service/history/ndc/workflow_resetter_test.go:283-343`, `service/history/ndc/state_rebuilder_test.go:124, 160, 246`, and `service/history/ndc/resetter_test.go:111-202+`), the `SetReplayEventBatchID` hook shows deliberate engineering for replay consistency, and the SDK worker side has end-to-end replay tests against frozen histories (`service/worker/scheduler/replay_test.go:20-42`, `service/worker/workerdeployment/replaytester/replay_test.go:24-59`). The single deductive item is that this is server-side replay, not LLM-output replay: the server never calls models, so it cannot "replay the model from logs." Model determinism is delegated to the SDK; the server's notion of replay correctness is "events are recorded → mutable state reconstructs identically." That is a deliberate architectural split, not a defect.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Replay primitive (event application) | `MutableStateRebuilderImpl.ApplyEvents` drives all replay paths | `service/history/workflow/mutable_state_rebuilder.go:70-101` |
| Replay primitive (event-by-event switch) | Per-event-type `case` blocks call `ApplyXxxEvent` on the mutable state | `service/history/workflow/mutable_state_rebuilder.go:152-693` |
| State rebuilder (pagination + apply) | `StateRebuilderImpl.Rebuild` paginates the branch via `ReadHistoryBranchByBatch` and applies events | `service/history/ndc/state_rebuilder.go:103-150` |
| State rebuilder (paging iterator source) | `getPaginationFn` reads history batches; uses `LastEventID + 1` as upper bound | `service/history/ndc/state_rebuilder.go:356-400` |
| Reset replay | `workflowResetterImpl.replayResetWorkflow` forks a branch and rebuilds up to the reset point | `service/history/ndc/workflow_resetter.go:434-508` |
| Reset entrypoint | `ResetWorkflowExecution` comment: "terminates current workflow (if running) and replay & create new workflow" | `service/history/history_engine.go:855-862` |
| Reset API handler | `resetworkflow.Invoke` loads base+current, computes `baseRebuildLastEventID`, calls `WorkflowResetter.ResetWorkflow` | `service/history/api/resetworkflow/api.go:27-204` |
| Conflict resolution rebuild | `ConflictResolverImpl.rebuild` streams events from a non-current branch into mutable state | `service/history/ndc/conflict_resolver.go:116-183` |
| Conflict resolution verification | After rebuild, `rebuildVersionHistory.Equal(replayVersionHistory)` is asserted | `service/history/ndc/conflict_resolver.go:166-168` |
| Admin rebuild (corruption recovery) | `historyEngineImpl.RebuildMutableState` → `workflowRebuilderImpl.rebuild` → `RebuildWithCurrentMutableState` | `service/history/history_engine.go:993-1006`, `service/history/workflow_rebuilder.go:64-104`, `service/history/workflow_rebuilder.go:201-232` |
| Rebuild RPC handler | `Handler.RebuildMutableState`: "attempts to rebuild mutable state according to persisted history events" | `service/history/handler.go:772-797` |
| Rebuild preconditions | `rebuildableCheck` only allows workflow archetype and non-empty version history | `service/history/workflow_rebuilder.go:110-147` |
| Cross-cluster replication replay | `HistoryReplicatorImpl.BackfillHistoryEvents` replays events from a remote cluster | `service/history/ndc/history_replicator.go:210-231` |
| Branch forking (creates new replay branches) | `executionManagerImpl.ForkHistoryBranch` constructs a new branch token inheriting ancestors up to fork node | `common/persistence/history_manager.go:36-127` |
| History read primitive | `executionManagerImpl.ReadHistoryBranch` and `ReadHistoryBranchByBatch` paginate history | `common/persistence/history_manager.go:516-578` |
| Events cache (replay optimization) | `events.Cache` keys events by `(NamespaceID, WorkflowID, RunID, EventID, Version)`; falls back to persistence on miss | `service/history/events/cache.go:23-144` |
| Events cache TTL config | `EventsCacheTTL` dynamic config; host-level vs shard-level caches | `service/history/configs/config.go:80-85, 506-509` |
| Replay batch ID hook | `MutableStateImpl.SetReplayEventBatchID` records the batch first-event ID so `GenerateEventLoadToken` references the original batch | `service/history/workflow/mutable_state_impl.go:1369-1395` |
| Replay flag in mutable state | `replayEventBatchID` field is `common.EmptyEventID` on the live path; set on the rebuild path | `service/history/workflow/mutable_state_impl.go:278-281` |
| Replay flag reset | `replayEventBatchID = common.EmptyEventID` reset on transaction close | `service/history/workflow/mutable_state_impl.go:8357` |
| Replay-time sticky queue clear | `b.mutableState.ClearStickyTaskQueue()` at start of `applyEvents` (standby side has no stickiness) | `service/history/workflow/mutable_state_rebuilder.go:122` |
| Replay-time workflow task stamp preservation | Comment: "Preserve the WorkflowTaskStamp during rebuild to ensure workflow task validation works correctly" | `service/history/workflow/mutable_state_rebuilder.go:126-128` |
| Replay-time versioning skip | `skipping versioning checks because this task is not actually dispatched but will fail immediately` | `service/history/ndc/workflow_resetter.go:541-542` |
| Last first event TXN ID | `LastFirstEventTxnId` recorded on rebuilt mutable state for branch truncation safety | `service/history/ndc/state_rebuilder.go:135` |
| Last first event ID | `executionInfo.LastFirstEventId = firstEvent.GetEventId()` per batch | `service/history/workflow/mutable_state_rebuilder.go:124` |
| Read history for worker replay | `GetWorkflowExecutionHistory` reads history events with pagination and branch-token continuation | `service/history/api/getworkflowexecutionhistory/api.go:119-530` |
| History raw delivery to SDK | `SendRawHistoryBetweenInternalServices` / `SendRawWorkflowHistory` allow history to be sent to worker as raw blobs for SDK replay | `service/history/api/getworkflowexecutionhistory/api.go:301-323` |
| Transient task history delivery | `appendTransientTasks` adds unsaved in-flight workflow task events so worker replay sees current state | `service/history/api/getworkflowexecutionhistory/api.go:36-117` |
| Worker SDK replay | `worker.NewWorkflowReplayer()` + `replayer.ReplayWorkflowHistory(logger, history)` | `service/worker/scheduler/replay_test.go:21-42` |
| Worker SDK replay (multi-version) | `workerdeployment` replay tests iterate `wv` versions, replaying histories from testdata snapshots | `service/worker/workerdeployment/replaytester/replay_test.go:24-59` |
| CHASM does not replay events | "Unlike HSM callbacks, CHASM replicates entire trees rather than replaying events, so deterministic cross-cluster IDs based on event version are not needed" | `chasm/lib/workflow/workflow.go:177-178` |
| Version history abstraction | `versionhistory` package owns branch lineage (`AddOrUpdateVersionHistoryItem`, `GetVersionHistory`) | `common/persistence/versionhistory/version_history.go:10-80` |
| VersionedTransition (state-based) | `transitionhistory` package introduces newer state-based replication with `Compare` and `StalenessCheck` | `common/persistence/transitionhistory/transition_history.go:44-110` |
| Worker deterministic tests | `worker_versioning_test.go` notes "~50%-per-call probabilistic bug practically deterministic" — tests rely on determinism | `common/worker_versioning/worker_versioning_test.go:763` |
| Determinism metric (server) | `ServiceErrNonDeterministicCounter` — server tracks "service errors nondeterministic" (used for non-deterministic task outcomes) | `common/metrics/metric_defs.go:674` |
| Replay test (state rebuilder) | `TestRebuild`, `TestPagination`, `TestApplyEvents` in `state_rebuilder_test.go` | `service/history/ndc/state_rebuilder_test.go:124, 160, 246-...` |
| Replay test (reset replay) | `TestReplayResetWorkflow` mocks `StateRebuilder.Rebuild` and validates fork + rebuild wiring | `service/history/ndc/workflow_resetter_test.go:283-343` |
| Replay test (reset success/error) | `TestResetWorkflow_NoError` / `TestResetWorkflow_Error` cover the resetter happy and error paths | `service/history/ndc/resetter_test.go:111-202+` |
| Replay test (per event type) | 50+ `TestApplyEvents_EventTypeXxx` tests for each event type in `mutable_state_rebuilder_test.go` | `service/history/workflow/mutable_state_rebuilder_test.go:164-2158+` |
| Replay test (admin rebuilder) | `TestRebuildableCheck` table-driven test for archetype and version-history preconditions | `service/history/workflow_rebuilder_test.go:175-216` |
| Integration replay test | `TestResetWorkflow` and 10+ reset variants in `tests/reset_workflow_test.go` | `tests/reset_workflow_test.go:45-1014+` |
| Cross-cluster replay test | `TestReplicateHistoryEvents_ForceReplicationScenario` and `TestResetWorkflow_SyncWorkflowState` | `tests/xdc/stream_based_replication_test.go:151, 527` |
| Cross-cluster replay migration | `TestHistoryReplication_MultiRunMigrationBack`, `_LongRunningMigrationBack_*` | `tests/ndc/replication_migration_back_test.go:143, 214, 269` |
| Conflict resolution replay test | `TestGetOrRebuildMutableState_NoRebuild_SameIndex` and `TestGetOrRebuildMutableState_Rebuild` | `service/history/ndc/conflict_resolver_test.go:308-326, 376-409` |
| Update dedup during reapply | `reapplyEvents` uses `IsResourceDuplicated(runIdForDeduplication, ...)` to skip already-applied updates | `service/history/ndc/workflow_resetter.go:854-860, 1033-1036` |
| Update cross-branch dedup | `targetBranchUpdateRegistry.Find(attr.Request.Meta.UpdateId)` to skip updates already present in target branch | `service/history/ndc/workflow_resetter.go:890-891, 908-909` |
| Child event reapply | `reapplyChildEvents` re-applies child events (Started/Completed/Failed/...) excluding the Initiated event | `service/history/ndc/workflow_resetter.go:1044-1138` |
| HSM cherry-pick events | Default branch in `reapplyEvents` calls `def.CherryPick(root, event, ...)` for HSM-registered event types | `service/history/ndc/workflow_resetter.go:1015-1031` |
| Reset excludes identity-terminated events | Comments + `if attr.GetIdentity() == consts.IdentityHistoryService || attr.GetIdentity() == consts.IdentityResetter { continue }` prevents re-applying terminations written by the reset path itself | `service/history/ndc/workflow_resetter.go:985-988` |
| IsTerminatedByResetter | Helper to detect terminations written by the resetter (used by callers) | `service/history/ndc/workflow_resetter.go:1164-1169` |
| findStartRequestID helper | Reapplies callbacks under the original start request ID so receiver-side dedup survives reset | `service/history/ndc/state_rebuilder.go:402-411`, `service/history/ndc/workflow_resetter.go:208-210` |
| Start request ID preservation on reset | "Use the original start request ID from the base run so callbacks on the reset workflow are associated with the original start request" | `service/history/ndc/workflow_resetter.go:204-210` |

## Answers to Dimension Questions

1. **Can execution be replayed?** Yes, and replay is the *primary* mechanism for state reconstruction. The server replays an execution's events in at least five independent code paths: workflow reset (`service/history/ndc/workflow_resetter.go:434-508`), replication gap-filling (`service/history/ndc/history_replicator.go:210-231`), conflict resolution between branches (`service/history/ndc/conflict_resolver.go:116-183`), admin recovery for corrupted mutable state (`service/history/workflow_rebuilder.go:64-104, 201-232`), and the import flow for new clusters (`service/history/ndc/history_importer.go:249-253` via `mutableStateMapper.GetOrRebuildMutableState`). Each path ultimately drives `MutableStateRebuilderImpl.ApplyEvents` (`service/history/workflow/mutable_state_rebuilder.go:70-101`) against a paginated branch token. Workers replay recorded histories to detect code-side non-determinism using `worker.NewWorkflowReplayer().ReplayWorkflowHistory` (`service/worker/scheduler/replay_test.go:21-42`).

2. **What exactly is replayed?** Everything needed to rebuild *server-side mutable state* — activities, timers, child workflows, workflow tasks, signals, updates, search attributes, versioning state, chasm nodes (via HSM). Replay is **not** LLM-call replay: the server has no concept of an LLM, and the recorded events describe the *outcome* of workflow code (e.g., `ActivityTaskScheduled`, `ActivityTaskCompleted`) rather than user code tokens. The history delivered to workers for SDK-side replay is the same event stream (`service/history/api/getworkflowexecutionhistory/api.go:119-530`); the SDK then re-runs user code against it. Transient/speculative workflow task events are appended to the response when configured (`service/history/api/getworkflowexecutionhistory/api.go:36-117`), and raw blob delivery is supported via dynamic config flags (`service/history/api/getworkflowexecutionhistory/api.go:301-323`).

3. **Are LLM calls replayed from logs or re-called?** Re-called — but on the SDK side, not the server. The server never invokes LLMs. The SDK's `WorkflowReplayer.ReplayWorkflowHistory` re-invokes the user workflow function with the recorded history (`service/worker/scheduler/replay_test.go:33-37`, `service/worker/workerdeployment/replaytester/replay_test.go:106-109`). There is no built-in server-side model output cache; if a worker re-executes a workflow that calls an LLM with non-zero temperature, results vary. The `temporaltest` package does fail fast on non-determinism (`temporaltest/server.go:50` — "fail fast when workflow code panics or detects non-determinism"), but this is achieved by comparing the events the re-executed code produces against the recorded events.

4. **Does replay verify correctness or only reconstruct state?** Replay *reconstructs* state and *implicitly verifies* on the server side because the events are pre-recorded: applying them to a fresh mutable state yields the same mutable state every time (modulo time-sensitive fields like `LastRunningClock`, which is set explicitly per event at `service/history/workflow/mutable_state_rebuilder.go:145`). On the conflict-resolution path, an explicit equality check is performed: `rebuildVersionHistory.Equal(replayVersionHistory)` (`service/history/ndc/conflict_resolver.go:166-168`). On the SDK side, replay *verifies* by comparing events the re-executed code produces against the recorded events; mismatches are reported as non-determinism errors. The server's notion of "correctness" is "I rebuilt mutable state from history and got the same version history back." There is no replay-time assertion comparing worker output to recorded model tokens.

5. **What non-determinism remains?**
   - **SDK-side LLM calls.** Re-running a workflow that calls a non-deterministic model produces different events. Mitigated only by user choice (deterministic temperature, prompt caching) or workflow code idempotency. Documented in `temporaltest/server.go:50`.
   - **Server-side `now()` substitutes.** Rebuild uses the rebuilder's `now` argument (`service/history/ndc/state_rebuilder.go:103-113`); on the live path this is `r.shard.GetTimeSource().Now()` (`service/history/ndc/workflow_resetter.go:472-473`); on the admin rebuild it is also `r.shard.GetTimeSource().Now()` (`service/history/workflow_rebuilder.go:210-212`). Fields set from `now()` (e.g., activity `ScheduledTime`/`FirstScheduledTime` overrides during reset at `service/history/ndc/workflow_resetter.go:577-578`) are *intentionally* reset to current time on reset, so reset is not bit-identical with the pre-reset run by design.
   - **LastFirstEventTxnId.** Set to the rebuilt transaction ID (`service/history/ndc/state_rebuilder.go:135`) — this is a branch-truncation safety field, not state semantics.
   - **WorkflowTaskAttempt counter.** Replay rebuilds attempt counters from the events but does not re-attempt; speculative/transient task state is reconstructed from events (`service/history/workflow/mutable_state_rebuilder.go:122-122` clears stickiness).
   - **Update ID deduplication across replay paths.** Replay that crosses a `WorkflowExecutionOptionsUpdated` event with completion callbacks re-applies them, but the rebuilder refuses to re-attach if the request ID is already present (`service/history/ndc/workflow_resetter.go:957-963`).
   - **CHASM trees.** Do not replay events at all; they replicate state directly (`chasm/lib/workflow/workflow.go:177-178`), so CHASM correctness depends on serialized snapshots, not event replay.
   - **In-memory state.** Cached `workflowContext` is cleared at `service/history/ndc/state_rebuilder.go:138` and `service/history/workflow_rebuilder.go:84`; in-memory events cache (`service/history/events/cache.go`) is consulted but authoritative only as a hint.
   - **External payload size accumulation.** Replay accumulates `ExternalPayloadSize`/`ExternalPayloadCount` differently than live writes (`service/history/ndc/state_rebuilder.go:386-396`).

## Architectural Decisions

- **Event sourcing as the foundation.** The system explicitly commits to event sourcing in the architecture doc: "The system functions via event sourcing: an append-only history of events is stored for each workflow execution, and all required workflow state can be recreated at any time by replaying this history." (`docs/architecture/README.md:32`).
- **One rebuilder, many callers.** Every replay path funnels through `MutableStateRebuilderImpl.ApplyEvents` (`service/history/workflow/mutable_state_rebuilder.go:70-101`), and `StateRebuilderImpl.Rebuild` (`service/history/ndc/state_rebuilder.go:103-150`) wraps it with persistence pagination. There is no second, divergent replay implementation.
- **History branch token as the address.** Branches (created by reset, replication, or continue-as-new) are identified by opaque tokens (`common/persistence/history_manager.go:36-127`). `ForkHistoryBranch` is the only way to create a new replayable branch, ensuring lineage is preserved (`service/history/ndc/workflow_resetter.go:605-628`).
- **Replay-as-verification.** Conflict resolution uses replay to detect divergence: after rebuilding, the resulting version history must equal the source (`service/history/ndc/conflict_resolver.go:166-168`). This is the server's formal correctness check.
- **Server replay is independent of model calls.** The server only knows about events; it does not record or replay model outputs. Model determinism is a worker/SDK concern.
- **CHASM deliberately does not replay.** The comment at `chasm/lib/workflow/workflow.go:177-178` is an explicit architectural decision to replicate state trees instead of replaying events for the CHASM subsystem, with the consequence that cross-cluster IDs do not need to be tied to event versions.
- **`SetReplayEventBatchID` hook.** The rebuilder sets a per-batch event ID on the mutable state so that event load tokens generated during replay reference the original batch (`service/history/workflow/mutable_state_rebuilder.go:150`, `service/history/workflow/mutable_state_impl.go:1369-1395`). Without this, replay would generate load tokens pointing to batches that do not yet exist in cache.
- **LastFirstEventTxnId for branch safety.** Rebuilding records the transaction ID of the last first event so that persistence layer truncation knows where to stop (`service/history/ndc/state_rebuilder.go:135`).
- **Sticky queue cleared on replay.** Standby replication never uses stickiness, so the rebuilder clears it at the start of every batch (`service/history/workflow/mutable_state_rebuilder.go:122`).
- **Reset reapplies a curated set of event types.** `reapplyEvents` cherry-picks which events to re-apply: signals, updates, cancel requests, options updates, terminations (non-resetter), and HSM cherry-pickable events (`service/history/ndc/workflow_resetter.go:842-1039`). This is policy, not automatic.
- **Updates get dedup keys.** Updates reapplied during reset/conflict use `(runIdForDeduplication, eventId, version)` tuples stored in the mutable state's deduplication set (`service/history/ndc/workflow_resetter.go:1033-1036`).
- **Admin rebuild is a separate `RebuildWithCurrentMutableState` path.** It copies the original mutable state's `TransitionHistory`, `PreviousTransitionHistory`, and `LastTransitionHistoryBreakPoint` onto the rebuilt state (`service/history/ndc/state_rebuilder.go:215-222`), preserving the transition lineage through the recovery operation.

## Notable Patterns

- **Two-tier replay.** `StateRebuilderImpl` does persistence I/O; `MutableStateRebuilderImpl` does event application. Both are interface-driven and mocked (`service/history/ndc/state_rebuilder.go:30-54, 82`; `service/history/workflow/mutable_state_rebuilder.go:25-46, 53`).
- **Append-only lineage with branches.** `ForkHistoryBranch` creates a child branch whose `Ancestors` list points back to the parent up to the fork node (`common/persistence/history_manager.go:36-127`). This is the lineage graph for replay.
- **`IsResourceDuplicated` as a replay idempotency marker.** Reapplied events get a deduplication marker keyed by `(runId, eventId, version)`; subsequent replays of the same event skip it (`service/history/ndc/workflow_resetter.go:854-860, 1033-1036`).
- **Cherry-pick via HSM registry.** The default branch of `reapplyEvents` calls `def.CherryPick(root, event, ...)` on HSM-registered event types (`service/history/ndc/workflow_resetter.go:1015-1031`), making the reapply set extensible.
- **`replayEventBatchID` flow control.** The flag `replayEventBatchID` is reset on transaction close (`service/history/workflow/mutable_state_impl.go:8357`) and read only when `hBuilder.LatestEventBatchID()` is `EmptyEventID` (`service/history/workflow/mutable_state_impl.go:1382-1385`). This is the single hook that distinguishes replay from live event generation.
- **`rebuildVersionHistory.Equal(replayVersionHistory)` postcondition.** Used as the conflict-resolution correctness invariant (`service/history/ndc/conflict_resolver.go:166-168`).
- **Sanitization of mutable state on replication.** `LastFirstEventTxnId` is zeroed before transmission on replication tasks so passive clusters do not inherit the active cluster's truncation safety ID (`service/history/replication/sync_state_retriever_test.go:190-291`).
- **Decision lines on the reset path.** `service/history/ndc/workflow_resetter.go:927-933` skips `CancelRequested` if already requested; `:986-988` skips terminations written by the history service or resetter itself.
- **Reset re-uses the original start request ID.** `findStartRequestID` reads the original `WorkflowExecutionStarted` request ID so receiver-side dedup of completion callbacks survives the reset (`service/history/ndc/state_rebuilder.go:402-411`).

## Tradeoffs

- **Server replay is fully deterministic; SDK replay is not (by design).** The split is clean: the server records events that fully determine its mutable state, and the SDK records nothing about user code, so SDK replay re-runs code. This is the right tradeoff for the architecture but means model-output determinism is a user concern.
- **CHASM does not replay events.** This buys simplicity (no event log per chasm node) at the cost of replication fidelity (entire trees replicated, no granular event-level replay). Documented at `chasm/lib/workflow/workflow.go:177-178`.
- **Reset reapplies a curated event subset.** Choosing which events to reapply (signals, updates, terminations) is policy in `reapplyEvents` (`service/history/ndc/workflow_resetter.go:842-1039`). New event types need code changes to be reapplicable.
- **`TransactionPolicyPassive` on rebuild.** The rebuild path closes transactions passively (`service/history/workflow_rebuilder.go:238-242`), meaning rebuilt state is not published as an active update; it overwrites the existing snapshot via `SetWorkflowExecution` (`service/history/workflow_rebuilder.go:249-253`).
- **Reset mutates `now`-derived fields.** Reset overrides `ScheduledTime`/`FirstScheduledTime` of pending activities to `now` (`service/history/ndc/workflow_resetter.go:577-578`), so post-reset timing is not bit-identical with pre-reset.
- **Duplication map kept in memory.** `ms.IsResourceDuplicated` is in-memory only (`service/history/ndc/workflow_resetter.go:858`); a passive cluster that rebuilds independently must rebuild its own dedup set by replaying the same events.
- **Branch tokens are opaque.** A token's contents are kept inside `history_branch_util.go`; debugging replay correctness often requires token parsing to be aware of.
- **External payload accounting drift.** Rebuild accumulates `ExternalPayloadSize`/`ExternalPayloadCount` for each batch via `workflow.CalculateExternalPayloadSize` (`service/history/ndc/state_rebuilder.go:386-396`), which may not match the live path exactly.

## Failure Modes / Edge Cases

- **Replay against a non-current branch during conflict resolution.** `ConflictResolverImpl.GetOrRebuildMutableState` (`service/history/ndc/conflict_resolver.go:86-114`) returns `mutableState, false, nil` when the branch is already current; otherwise it rebuilds.
- **Rebuild with empty history.** `applyEvents` returns `ErrMessageHistorySizeZero` on empty input (`service/history/workflow/mutable_state_rebuilder.go:113-115`).
- **`LastFirstEventTxnId` zeroing on replication.** Documented and tested in `service/history/replication/sync_state_retriever_test.go:190-291`.
- **`CurrentBranchChanged` retry.** `GetWorkflowExecutionHistory` retries with an empty branch token when close-only is requested and the branch changed (`service/history/api/getworkflowexecutionhistory/api.go:170-193`).
- **Data-loss detection on read.** `TrimHistoryNode` is called on `DataLoss` errors during `GetWorkflowExecutionHistory` (`service/history/api/getworkflowexecutionhistory/api.go:283-295`) — a defensive backstop, not a replay feature.
- **`SetUpdateCondition` during admin rebuild.** The rebuilder sets `nextEventID` and `dbRecordVersion` on the rebuilt state so the persistence write fails safely if the underlying mutable state has changed (`service/history/workflow_rebuilder.go:230`).
- **Resetter `failInflightActivity` errors on transient activities.** Returns `serviceerror.NewInternal("WorkflowResetter encountered transient activity.")` because transient activities should not exist in a rebuilt state (`service/history/ndc/workflow_resetter.go:585-587`).
- **Speculative WFT during reset.** Reset runs `AddWorkflowTaskStartedEvent` with `skipVersioningCheck=true` and `routingInfo=-1` because the synthetic task is not dispatched (`service/history/ndc/workflow_resetter.go:533-545`).
- **Multi-run migration back.** When a cluster falls behind on replication, the migration-back tests at `tests/ndc/replication_migration_back_test.go:143-269` exercise gap-filling replay.
- **Update duplicates across reapply.** Reapplying updates checks `targetBranchUpdateRegistry.Find(...)` (`service/history/ndc/workflow_resetter.go:890-891, 908-909`) to avoid cross-branch duplicate updates.
- **Cherry-pick failures.** `def.CherryPick` returning `ErrNotCherryPickable` / `ErrStateMachineNotFound` / `ErrInvalidTransition` is treated as "skip" (`service/history/ndc/workflow_resetter.go:1023-1024`), not as an error — a deliberate fallback.

## Future Considerations

- **Replay-verify mode for the server.** The server reconstructs state but does not compare against worker outputs. A replay-verify mode would require worker-side hooks.
- **CHASM event replay.** Currently CHASM replicates entire trees (`chasm/lib/workflow/workflow.go:177-178`). If event-level replay becomes important for CHASM, the same `MutableStateRebuilderImpl` could be extended.
- **Replay observability.** `metrics.ServiceErrNonDeterministicCounter` exists (`common/metrics/metric_defs.go:674`) but is mostly server-side; deeper SDK-side replay observability (e.g., which workflow versions triggered non-determinism during replay) is limited.
- **Update replay across cluster boundaries.** Cross-cluster update reapply uses `IsResourceDuplicated` keyed by `runIdForDeduplication` (`service/history/ndc/workflow_resetter.go:849-860`). A more durable cross-cluster dedup mechanism is not evident.
- **Branch pruning interactions.** `DeleteHistoryBranch` (`common/persistence/history_manager.go:130-210`) ref-counts branches so pruning does not destroy active ancestors — a replay safety mechanism, but its interaction with concurrent resets is not deeply tested in the surfaced tests.

## Questions / Gaps

- **Server-side replay determinism vs replay-with-different-time.** Reset overrides activity `ScheduledTime`/`FirstScheduledTime` to `now` (`service/history/ndc/workflow_resetter.go:577-578`); admin rebuild uses `r.shard.GetTimeSource().Now()` as the start time (`service/history/workflow_rebuilder.go:210-212`). These mean replay is deterministic in *state semantics* but not in *every byte*; the boundary is implicit. No clear evidence found of a documented contract on which fields are or are not deterministic across replay.
- **What happens if events cache is disabled during replay?** The events cache is best-effort (`service/history/events/cache.go:118-144`); without it, replay falls back to `ReadHistoryBranch` per event. No clear evidence found of replay correctness under cache misses.
- **How is replay correctness verified across cluster boundaries when CHASM is involved?** CHASM replicates trees rather than events (`chasm/lib/workflow/workflow.go:177-178`). For workflows that include both HSM-managed sub-machines and CHASM nodes, the determinism contract is split. No clear evidence found of an end-to-end determinism test that exercises both layers simultaneously.
- **Replay-verify for SDK worker.** `temporaltest/server.go:50` enables fail-fast non-determinism in tests, but in production, the SDK's replay-verify is opt-in via `Replayer.ReplayWorkflowHistory`. No clear evidence found of automated production replay-verify runs.
- **Reverse history (`ReadHistoryBranchReverse`).** Used by `GetWorkflowExecutionHistoryReverse` (`service/history/api/getworkflowexecutionhistoryreverse/api.go:28-...`) — read in reverse for UI pagination. Not a replay path, but adjacent to it. No clear evidence found of any determinism impact from this read path.
- **What does the SDK do on non-deterministic replay?** `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:236` says "Worker will do full history replay, and updates should be delivered again" — implying the SDK detects non-determinism by event mismatch and the server then redelivers updates. The exact error contract is in the SDK, not in this repo.

---

Generated by `dimensions/01.10-replay-and-determinism-in-execution.md` against `temporal`.