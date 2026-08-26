# Source Analysis: temporal

## Dimension 14.03: Human Intervention and Takeover

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (server), protobuf APIs; event-sourced workflow orchestration engine |
| Analyzed | 2026-08-26 |

## Summary

Temporal treats human intervention as a first-class, event-sourced operation rather than an ad-hoc escape hatch. Humans cannot arbitrarily rewrite workflow state (that would break deterministic replay); instead the server exposes a layered intervention surface: (1) **feedback injection** via asynchronous Signals (`service/frontend/workflow_handler.go:2297`) and synchronous, validator-gated Updates (`service/frontend/workflow_handler.go:5414`) that can be awaited through lifecycle stages; (2) **read-only inspection** via Queries and DescribeWorkflowExecution (`service/frontend/workflow_handler.go:3256`, `service/frontend/workflow_handler.go:3320`); (3) **takeover** via Pause/Unpause of whole workflows and individual activities (`service/frontend/workflow_handler.go:7559`, `service/frontend/workflow_handler.go:7586`, `service/frontend/workflow_handler.go:7166`), Terminate and Cancel (`service/frontend/workflow_handler.go:2468`); and (4) **fork-and-continue** via ResetWorkflowExecution, which branches the append-only history at a chosen WorkflowTaskCompleted point into a new run ID, terminates the current run, and cherry-picks/reapplies human inputs (signals, updates) past the reset point (`service/history/api/resetworkflow/api.go:28-216`, `service/history/ndc/workflow_resetter.go:107-288`). Every intervention is recorded as a durable history event carrying identity, reason, request ID, and links, giving a complete audit trail by construction. The model is explicit, tested at unit and integration levels, idempotent-by-request-ID, and guarded by rate limits, blob-size limits, and dynamic-config feature flags.

## Rating

**Score: 9 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 bar met):** Each intervention mode has a dedicated gRPC API on the public WorkflowService surface with frontend validation (`service/frontend/workflow_handler.go:2297-2560`, `5414-5560`, `7080-7126`, `7559-7618`) and a history-service handler implementing it under a per-workflow consistency lock (`api.GetAndUpdateWorkflowWithNew` in `service/history/api/pauseworkflow/api.go:38-90`, `service/history/api/signalworkflow/api.go:34-111`). Update injection has a formal state machine (`Created→Admitted→Sent→Accepted→Completed`, `service/history/workflow/update/state.go:12-25`).
- **Tests:** Dedicated integration suites exist for pause (`tests/pause_workflow_execution_test.go:55`, covering pause/unpause, pending-task edge cases, query-rejection while paused, request validation), reset (`tests/reset_workflow_test.go:47`, including reapply-exclude matrix at lines 341-411 and continue-as-new reset at line 975), signals (`tests/signal_workflow_test.go:37`), and updates (`tests/update_workflow_sdk_test.go:32`), plus unit suites for the update state machine (`service/history/workflow/update/update_test.go`, `registry_test.go`) and mutable-state pause events (`service/history/workflow/mutable_state_impl_test.go:1940-2156`).
- **Operational safeguards:** per-execution signal limits (`service/history/api/signal_workflow_util.go:53-61`), payload blob-size limits (`service/history/api/signal_workflow_util.go:37-51`), in-flight update count/size limits (`service/history/workflow/update/registry.go:108-120`), business-ID reuse rate limiting on reset (`service/history/api/resetworkflow/api.go:46-55`), and idempotency dedup for signals (`service/history/api/signalworkflow/api.go:45-50`), updates (`service/history/workflow/update/update.go:320-326`), and resets (`service/history/api/resetworkflow/api.go:124-134`).
- **Why not 10:** The pause capability is disabled by default behind `frontend.WorkflowPauseEnabled` (`common/dynamicconfig/constants.go:3704-3708`) and is clearly recent — its test suite documents regression fixes for paused-workflow task handling (`tests/pause_workflow_execution_test.go:262-368`). Reset reapply has documented gaps where completions for CHASM-owned operations are silently dropped (`service/history/ndc/workflow_resetter.go:1176-1181`, referencing issue #11384). Rejected updates leave no durable history record by design (see Tradeoffs), which weakens auditability of *negative* interventions.

## Evidence Collected

Every entry cites file paths relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Signal API (async feedback) | Frontend handler validates signal name/request ID/blob size then forwards to history; response carries a Link referencing the exact `WorkflowExecutionSignaled` event | `service/frontend/workflow_handler.go:2297-2356`; `service/history/api/signalworkflow/api.go:115-123` |
| Signal persistence + dedup | Signal written as event with identity/header/requestId/links; duplicate request IDs short-circuit as Noop | `service/history/api/signalworkflow/api.go:45-50, 87-101`; `service/history/workflow/mutable_state_impl.go:6233-6247` |
| Signal limits | `MaximumSignalsPerExecution` cap and blob-size warn/error enforcement before admitting a signal | `service/history/api/signal_workflow_util.go:31-61` |
| Update API (sync feedback) | `UpdateWorkflowExecution` with WaitPolicy lifecycle stages ADMITTED/ACCEPTED/COMPLETED; long-poll until stage reached | `service/frontend/workflow_handler.go:5414-5448, 5450-5519`; `service/history/api/updateworkflow/api.go:264-276` |
| Update admission gating | Updates rejected when workflow paused, closing, or WFT repeatedly failing; speculative WFT created to carry the update | `service/history/api/updateworkflow/api.go:127-143, 189-199` |
| Update state machine | Created→ProvisionallyAdmitted→Admitted→Sent→ProvisionallyAccepted→Accepted→…→Completed/Aborted | `service/history/workflow/update/state.go:12-25`; `service/history/workflow/update/update.go:353-370, 607-705` |
| Update acceptance event | Worker acceptance writes `WorkflowExecutionUpdateAccepted` event; completion writes outcome; registry rebuilt from mutable state after restarts | `service/history/workflow/update/store.go:33-47`; `service/history/workflow/update/registry.go:168-224`; `service/history/workflow/mutable_state_impl.go:5712-5806` |
| Update rejection path | Worker validator can reject; rejected update returns failure outcome with a Workflow link (no history event); `RejectUnprocessed` fails stale updates | `service/history/workflow/update/update.go:707-709`; `service/history/api/updateworkflow/api.go:283-294`; `service/history/workflow/update/registry.go:57-60` |
| Poll update | `PollWorkflowExecutionUpdate` lets callers await an update result by reference (long-poll) | `service/frontend/workflow_handler.go:5521-5560` |
| Query (read-only inspection) | `QueryWorkflow` supports query-reject conditions; paused workflows return QueryRejected with status PAUSED | `service/frontend/workflow_handler.go:3256-3317`; `tests/pause_workflow_execution_test.go:1026-1041` |
| Reset (fork & continue) | New run ID generated, history branch forked at `WorkflowTaskFinishEventId`, current run terminated, eligible events reapplied | `service/history/api/resetworkflow/api.go:56-77, 136-203`; `service/history/ndc/workflow_resetter.go:103-288` |
| History branch fork | `ForkHistoryBranch` persistence call creates new branch token for the reset run | `service/history/ndc/workflow_resetter.go:677-703` |
| Reset traceability | Base run gets `UpdateResetRunID`; synthetic WFT failure with cause `RESET_WORKFLOW` records baseRunID/resetRunID/version; resetter uses distinct identity `history-service`/resetter | `service/history/ndc/workflow_resetter.go:132, 582-635, 705-718` |
| Reapply policy for human inputs | Signals/updates/cancel/options events selectively reapplied after reset point; exclude-type matrix computed from request | `service/history/ndc/workflow_resetter.go:956-1094`; `service/history/api/resetworkflow/api.go:237-259` |
| Continue-as-new chain reapply | Reset walks the continue-as-new chain and reapplies eligible events across runs | `service/history/ndc/workflow_resetter.go:720-847` |
| Post-reset operations | Reset can apply versioning overrides / options updates atomically as part of forking | `service/history/ndc/workflow_resetter.go:264-266, 1400-1413`; `service/history/api/resetworkflow/api.go:79-84, 206-211` |
| Pause/unpause workflow | Pause writes `WorkflowExecutionPaused` event, sets status PAUSED + PauseInfo(identity/reason/requestId/time); unpause restores RUNNING and reschedules tasks | `service/history/api/pauseworkflow/api.go:38-96`; `service/history/workflow/mutable_state_impl.go:3322-3394, 3396-3454` |
| Pause semantics for in-flight work | Started WFT resolves normally while paused; scheduled-but-unstarted WFT failed with cause `WORKFLOW_PAUSE_REQUESTED_BEFORE_TASK_STARTED`; activities stamped for replication | `service/history/workflow/mutable_state_impl.go:3355-3391`; `tests/pause_workflow_execution_test.go:370-456, 458-546` |
| Pause observability | `TemporalPauseInfo` keyword-list search attribute built from pause info so paused workflows are searchable in visibility | `service/history/workflow/mutable_state_impl.go:7061-7106` |
| Pause feature flag | Namespace-scoped dynamic config `frontend.WorkflowPauseEnabled`, default false | `common/dynamicconfig/constants.go:3704-3708`; `service/frontend/workflow_handler.go:7566-7568` |
| Activity-level takeover | `PauseActivity` / `UnpauseActivity` / `ResetActivity` operate on individual activities inside a running workflow | `service/frontend/workflow_handler.go:7166-7249`; `tests/pause_workflow_execution_test.go:713-982` |
| Execution-options editing mid-run | `UpdateWorkflowExecutionOptions` applies field-masked edits (versioning override, priority) to a running execution; recorded as `WorkflowExecutionOptionsUpdated` event | `service/frontend/workflow_handler.go:7080-7126`; `service/history/workflow/update/store.go:49-64` |
| Terminate (hard takeover) | Terminate records `WorkflowExecutionTerminated` with reason+identity; user-initiated requests get default reason/identity to distinguish from system ones | `service/frontend/workflow_handler.go:2468-2505`; `service/history/api/terminateworkflow/api.go:67-74` |
| Cancel (graceful takeover) | `RequestCancelWorkflowExecution` records cancel-request event consumed by workflow code | `service/history/api/requestcancelworkflow/` (API dir), proto contract `proto/internal/temporal/server/api/historyservice/v1/service.proto:566-576` |
| Integration tests | Suites: pause (incl. validation & edge cases), reset (reapply matrix, buffered-signal reapply, continue-as-new), signal, update SDK | `tests/pause_workflow_execution_test.go:55`; `tests/reset_workflow_test.go:43, 341-411, 719-725, 975-1163`; `tests/signal_workflow_test.go:37-41`; `tests/update_workflow_sdk_test.go:32` |

## Answers to Dimension Questions

### 1. Can humans edit agent state?

**Partially — by design, only through controlled, event-sourced channels.** There is no API that directly rewrites mutable state or history; all "editing" is expressed as new events appended under the workflow lock:

- Data/state feedback: Signals (`service/history/api/signalworkflow/api.go:90-101`) and Updates (`service/history/api/updateworkflow/api.go:83-102`), the latter supporting synchronous round-trips with worker-side validators that may accept or reject the proposed change.
- Metadata/options editing: `UpdateWorkflowExecutionOptions` performs field-mask-based edits of versioning override and priority on a live execution (`service/frontend/workflow_handler.go:7080-7126`).
- Status transitions humans can force: pause/unpause (`service/history/api/pauseworkflow/api.go:59-85`), cancel, terminate (`service/history/api/terminateworkflow/api.go:67-74`).
- Retroactive correction: Reset re-executes from a prior consistent point rather than editing (`service/history/api/resetworkflow/api.go:74-77`).

Direct mutation is structurally prevented because state is derived from replayable history; the resetter rebuilds mutable state by replaying events onto a fresh branch (`service/history/ndc/workflow_resetter.go:505-580`).

### 2. Can humans provide mid-run feedback?

**Yes, through two complementary mechanisms plus queries.**

- **Signals** are fire-and-forget: validated, size-limited, counted against a per-execution maximum, and delivered as buffered events that trigger a workflow task (`service/history/api/signalworkflow/api.go:60-106`). Deduplication by client-supplied request ID prevents double-delivery on retries (`service/history/api/signalworkflow/api.go:45-50, 87-89`).
- **Updates** are synchronous and two-way: the server admits the request (dedup by update ID, `service/history/workflow/update/update.go:320-326`), delivers it on a speculative workflow task (`service/history/api/updateworkflow/api.go:189-199`), and the worker's validator can reject it; callers can wait at ADMITTED (disabled for direct clients, `service/frontend/workflow_handler.go:5493-5495`), ACCEPTED (flag-gated, `service/frontend/workflow_handler.go:5497-5500`), or COMPLETED stages via long-poll (`service/history/api/updateworkflow/api.go:269-276`). Completion callbacks allow webhook-style notification instead of polling (`service/history/workflow/update/store.go:49-64`).
- **Queries** provide read-only mid-run introspection with reject conditions (`service/frontend/workflow_handler.go:3256-3317`).

### 3. Can humans take over execution?

**Yes — suspend, stop, or rewind; but not execute worker code in-process.**

- **Pause/Unpause**: a full execution-level freeze gated by `frontend.WorkflowPauseEnabled`. Pausing stops new workflow-task scheduling, invalidates undelivered tasks, and leaves in-flight tasks to resolve naturally (`service/history/workflow/mutable_state_impl.go:3355-3391`; verified by `tests/pause_workflow_execution_test.go:370-648`). Unpause schedules a fresh workflow task (`service/history/api/unpauseworkflow/api.go:74-77`).
- **Activity pause**: individual activities can be paused/unpaused independently, and activity pauses survive workflow unpause (`tests/pause_workflow_execution_test.go:881-982`).
- **Terminate/Cancel**: hard stop with reason/identity (`service/history/api/terminateworkflow/api.go:67-74`) or cooperative cancellation.
- **No sandbox exec**: the server never executes workflow code itself; workers are external. "Takeover" therefore means regaining control of *orchestration* state, not shelling into a runtime. A human corrects behavior by pausing/resetting and redeploying workers, optionally pinning the reset run to a fixed worker version via post-reset `UpdateWorkflowOptions` (`service/history/ndc/workflow_resetter.go:1400-1413`).

### 4. Are human interventions traceable?

**Yes — this is Temporal's strongest property here.** Every intervention becomes one or more immutable history events:

- Signals → `WorkflowExecutionSignaled` with identity, header, requestId, links (`service/history/api/signalworkflow/api.go:90-98`).
- Updates → `UpdateAdmitted` (reapply path) / `UpdateAccepted` / `UpdateCompleted` events, mirrored in `executionInfo.UpdateInfos` keyed by update ID with admission/acceptance/completion pointers (`service/history/workflow/mutable_state_impl.go:5712-5806`); API responses embed Links pointing at the exact event or, for rejections, at the workflow with reason `"Update rejected"` (`service/history/api/updateworkflow/api.go:278-312`).
- Pause/Unpause → events carrying identity/reason/requestID, plus `TemporalPauseInfo` search attribute for visibility search (`service/history/workflow/mutable_state_impl.go:3322-3350, 7061-7106`).
- Terminate → event with reason and identity; user vs system origin distinguished via defaults (`service/frontend/workflow_handler.go:2488-2494`).
- Reset → base run stores `ResetRunID` pointer (`service/history/ndc/workflow_resetter.go:132`); the new run records a synthetic `WorkflowTaskFailed{Cause: RESET_WORKFLOW}` containing base run ID, reset run ID and version (`service/history/ndc/workflow_resetter.go:623-633`); system identities `history-service` / resetter mark machine-initiated effects (`service/history/consts`, used at `service/history/ndc/workflow_resetter.go:609, 627, 714`).
- Idempotency keys (request ID / update ID) make replays observable as Noops rather than duplicates (`service/history/api/signalworkflow/api.go:45-50`; `service/history/api/resetworkflow/api.go:124-134`).

## Architectural Decisions

1. **Interventions are events, not patches.** All human actions funnel through the same append-only history + mutable-state transaction machinery (`api.GetAndUpdateWorkflowWithNew` closure pattern in `service/history/api/pauseworkflow/api.go:38-90`, `service/history/api/signalworkflow/api.go:34-111`), guaranteeing atomicity, replication, and replay-consistency identical to normal workflow progress.
2. **Determinism over editability.** Arbitrary state rewriting is intentionally impossible; correction is achieved by reset-to-point with selective reapply (`service/history/api/resetworkflow/api.go:74-77`, `GetResetReapplyExcludeTypes` at 237-259). This trades convenience for replay safety.
3. **Two-tier feedback model.** Async signals (buffered, cheap, unlimited ordering guarantees) vs sync updates (validated, dedup'd, wait-stage long-polling) give humans both low-latency steering and RPC-like interaction with a durable protocol state machine (`service/history/workflow/update/state.go:12-25`).
4. **Crash-safe update lifecycle.** Provisional states + OnAfterCommit/OnAfterRollback hooks ensure update state transitions only become visible after durability (`service/history/workflow/update/update.go:353-370, 659-703`), and registries reconstruct admitted/accepted updates from mutable state after process restarts (`service/history/workflow/update/registry.go:168-224`).
5. **Fork via storage-level branch tokens.** Reset forks the history tree at the storage layer (`ForkHistoryBranch`, `service/history/ndc/workflow_resetter.go:687-697`), enabling O(1)-ish branching without copying history.
6. **Opt-in rollout for new takeover features.** Pause ships behind a namespace-scoped dynamic config defaulting to false (`common/dynamicconfig/constants.go:3704-3708`), mirroring how updates were originally rolled out (`EnableUpdateWorkflowExecution`, `service/frontend/workflow_handler.go:5487-5489`).

## Notable Patterns

- **Lease-closure mutation pattern**: every history API mutates state inside a single callback under the workflow lease, returning declarative `{Noop, CreateWorkflowTask}` actions (`service/history/api/pauseworkflow/api.go:82-85`), keeping concurrency control uniform across all intervention types.
- **Speculative workflow tasks** for update delivery avoid persisting a scheduled event until needed (`service/history/api/updateworkflow/api.go:190-199`), reducing history noise from frequent feedback.
- **Cherry-pick framework for reapply**: a tri-state outcome (applied/skipped/fallback) routes each historical event through HSM then CHASM state-machine frameworks during reset (`service/history/ndc/workflow_resetter.go:1144-1254`).
- **Search-attribute projection of operational state**: pause info is projected into a `TemporalPauseInfo` keyword-list attribute so operators can find paused executions with visibility queries (`service/history/workflow/mutable_state_impl.go:7087-7106`), demonstrated in `tests/pause_workflow_execution_test.go:650-711`.
- **Identity conventions distinguish humans from system actors**: e.g., resetter-generated terminations use `consts.IdentityResetter` and are skipped during conflict-driven reapply (`service/history/ndc/workflow_resetter.go:1081-1083, 1379-1384`).
- **EqualHistoryEvents assertions**: integration tests assert exact event sequences (e.g., pause inserting `WorkflowTaskFailed` before `WorkflowExecutionPaused`), pinning audit-trail shape (`tests/pause_workflow_execution_test.go:346-353`).

## Tradeoffs

1. **Safety vs immediacy**: rejecting arbitrary state edits means fixing a bad intermediate value requires signal/update round-trip or full reset; reset additionally requires the target to align with a WorkflowTaskCompleted boundary and fails with pending child workflows unless a flag allows it (`service/history/ndc/workflow_resetter.go:341-343`).
2. **Rejected updates leave no durable trace**: a worker-validator rejection produces no history event; the response links to the workflow with reason `"Update rejected"` instead (`service/history/api/updateworkflow/api.go:283-294`). Audit completeness is sacrificed to keep history clean of non-effects.
3. **Pause granularity vs blast radius**: pausing freezes scheduling but not already-started work; in-flight tasks and their timeouts still resolve (`service/history/workflow/mutable_state_impl.go:3365-3373`), so pause is not a hard fence.
4. **Feature-flag fragmentation**: pause, async-accepted updates, and update API itself are separately gated (`service/frontend/workflow_handler.go:5487-5500, 7566-7568`), so intervention capabilities vary by deployment/namespace configuration.
5. **Reapply gaps during reset**: HSM-missing components cause silent drops on replication paths and warns on reset paths; CHASM-owned op completions can be dropped (tracked issue #11384 referenced at `service/history/ndc/workflow_resetter.go:1176-1181`).
6. **Reset is one-shot and non-idempotent across retries except via CreateRequestId dedup** (`service/history/api/resetworkflow/api.go:124-134`); a deleted run in a continue-as-new chain truncates reapplication (`service/history/ndc/workflow_resetter.go:808-819`).

## Failure Modes / Edge Cases

- **Pausing with a pending-but-unstarted workflow task** previously left a stale task that blocked unpause forever; now explicitly failed in the pause transaction with attempt-count reset (`service/history/workflow/mutable_state_impl.go:3374-3391`; regression test `tests/pause_workflow_execution_test.go:280-368`).
- **Worker crash while paused**: started WFT still resolves via its own start-to-close timeout; no retry scheduled until unpause (`tests/pause_workflow_execution_test.go:458-546`).
- **Signals during pause** are accepted and remain paused (no implicit resume): asserted in `tests/pause_workflow_execution_test.go:208-213`.
- **Updates to paused workflows** fail fast with FailedPrecondition (`service/history/api/updateworkflow/api.go:127-130`); updates fail fast when WFT attempt ≥ 3 (`service/history/api/updateworkflow/api.go:132-143`).
- **Duplicate resets/signals/updates** collapse to Noops via request-ID/update-ID dedup (`service/history/api/resetworkflow/api.go:124-134`; `service/history/workflow/update/update.go:324-326`).
- **Reset across continue-as-new chains** walks every successor run and reapplies eligible events; NotFound on a deleted successor breaks the chain gracefully (`service/history/ndc/workflow_resetter.go:808-819`).
- **Buffered signals at reset points** can be included or excluded via reapply flags, verified both ways (`tests/reset_workflow_test.go:719-725`).
- **Registry resurrection**: if the in-memory registry is cleared after an update was sent, retries recreate it as Admitted and late Acceptance messages are honored rather than erroring (`service/history/workflow/update/update.go:611-617`; `TryResurrect` at `service/history/workflow/update/registry.go:238-281`).

## Future Considerations

- Close the HSM/CHASM cherry-pick gap (issue #11384 cited at `service/history/ndc/workflow_resetter.go:1176-1181`) so reset reapplication never silently drops operation completions.
- Consider durable records for *rejected* updates (or a configurable audit mode) to complete the intervention audit trail.
- Generalize pause beyond the opt-in flag once soak testing completes; currently discoverability depends on deployment config (`common/dynamicconfig/constants.go:3704-3708`).
- Extend post-reset operations beyond `UpdateWorkflowOptions` — the switch already errors on unknown variants (`service/history/api/resetworkflow/api.go:290-292`), indicating planned extensibility.
- Child-workflow-aware reset ("phase 2") remains stubbed-out in code (`service/history/ndc/workflow_resetter.go:883-900`), limiting fork semantics for hierarchical runs.

## Questions / Gaps

- **Authorization**: intervention endpoints rely on caller-supplied `Identity` strings and namespace-level auth; no evidence of per-operation RBAC distinguishing who may pause vs terminate was found in the searched files (`service/frontend/workflow_handler.go:2297-2560, 7559-7618`). Search boundary: frontend handlers, history API packages, dynamicconfig constants; authorization middleware (if any) lives outside this repo's gRPC interceptor review scope for this study.
- **Cross-run propagation of pause**: no evidence found that pausing a parent affects child workflows; tests cover activity-vs-workflow independence (`tests/pause_workflow_execution_test.go:881-982`) but not child propagation. What was searched: pause/unpause history APIs, pause integration suite.
- **Signal ordering guarantees under high concurrency** (interleaving with updates) are defined by event-batching semantics but not exhaustively covered by tests found in this repo slice.
- Whether the public SDK-facing `UpdateAdmitted` event is ever persisted on the primary client path: current code writes it only on the reapply/conflict-resolution path (`service/history/ndc/workflow_resetter.go:988-1016`); the synchronous path admits in-memory only (`service/history/workflow/update/update.go:315-326` comment "Will be useful for durable admitted"), suggesting durable-admission is a future direction rather than shipped behavior.

---

Generated by `Dimension 14.03: Human Intervention and Takeover` against `temporal`.
