# Source Analysis: temporal

## 10.03 — Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server; gRPC + protobuf, Cassandra/SQL/ES persistence) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root `studies/agent-harness-study/sources/temporal/`.

## Summary

In Temporal, causal lineage is not an add-on feature — it *is* the execution model. Every workflow run is a totally ordered, append-only sequence of `HistoryEvent` protos in which each effect event carries explicit back-references to its cause: activity completions carry `ScheduledEventId` + `StartedEventId` (`service/history/historybuilder/event_factory.go:279-295`), command-derived events carry the originating `WorkflowTaskCompletedEventId` (`service/history/historybuilder/event_factory.go:230-253`), and terminal events carry the completing workflow task's event ID plus the result payload (`service/history/historybuilder/event_factory.go:335-367`). Event IDs are allocated sequentially per run (`service/history/historybuilder/event_store.go:58-62`), and a `wireEventIDs` pass resolves scheduled→started ID mappings and request-ID→event-ID mappings before batches commit (`service/history/historybuilder/event_store.go:381-454`). Workers receive causally-addressable task tokens embedding `(namespaceID, workflowID, runID, scheduledEventId, startedEventId)` (`proto/internal/temporal/server/api/token/v1/message.proto:39-56`, `common/tasktoken/token.go:22-58`), so every completion can be validated against the exact history events that caused it (`service/history/api/respondactivitytaskcompleted/api.go:60-110`).

Beyond intra-run linkage, Temporal preserves lineage across runs, workflows, services, and worker versions: parent/root pointers on child-workflow start events (`service/history/historybuilder/event_factory.go:91-98`), cross-run continuation fields (`ContinuedExecutionRunId`, `FirstExecutionRunId`, `OriginalExecutionRunId`, `PrevAutoResetPoints`, `LastCompletionResult`) (`service/history/historybuilder/event_factory.go:58-70`), typed `Link` variants attached to signals/terminations/options-updates (`common/links/validator.go:30-88`, `service/history/api/link_util.go:15-50`), Nexus operation links pointing back to the exact scheduled event in the caller's history (`components/nexusoperations/executors.go:393-407`), worker build-ID / deployment-version stamps recorded into events and indexed as search attributes for "which code version produced this" provenance (`common/worker_versioning/worker_versioning.go:93-139`), and full-history archival after close (`service/history/archival/archiver.go:29-56`). The update protocol persists acceptance/completion pointers to specific history event IDs and batch IDs (`proto/internal/temporal/server/api/persistence/v1/update.proto:20-45`). Integration tests assert exact ID-level linkage across processes (`tests/child_workflow_test.go:288-310`, `tests/activity_test.go:575-592`).

Mapping to the dimension's agent-harness vocabulary: "prompts" ≈ workflow/activity inputs stored in schedule/start events; "tools" ≈ activities and Nexus operations; "tool results" ≈ activity completion/failure events; "approvals" ≈ updates/signals with request-ID links and callbacks; "model versions" ≈ worker Build IDs / Worker Deployment Versions stamped into events; "artifacts" ≈ result payloads and completion callbacks bound to their producing run.

## Rating

**9 / 10 — Mature, durable, observable, extensible, and proven under failure or scale.**

Rationale:
- **Total causal coverage by construction**: no state change exists outside the event log; every event type's factory embeds cause references (`WorkflowTaskCompletedEventId`, `ScheduledEventId`, `StartedEventId`, `InitiatedEventId`, `AcceptedRequestSequencingEventId`, `AcceptedEventId`) (`service/history/historybuilder/event_factory.go:31-1118`). A final answer is traceable to the WFT-completed event, which chains back through every activity's schedule/start/complete triple to the original start input.
- **Causally-addressable dispatch**: task tokens carry the scheduled+started event IDs so completions are provably tied to their invocations, including validation against mutable state and token staleness (`service/history/api/respondactivitytaskcompleted/api.go:60-90`, `common/tasktoken/token.go:22-58`).
- **Lineage survives transformation**: continue-as-new, retry, reset, and replication-fork all preserve explicit bridge fields (`NewExecutionRunId`, `Attempt`+retry policy+last failure, `PrevAutoResetPoints`/auto-reset points, `BaseRunId`/`NewRunId`/`ForkEventVersion`, `VersionHistories` branching) (`service/history/historybuilder/event_factory.go:58-70`, `event_factory.go:202-228`, `service/history/api/resetworkflow/api.go:56-105`, `proto/internal/temporal/server/api/history/v1/message.proto:19-34`).
- **Model/version provenance**: `WorkerVersionStamp` (build ID) recorded on workflow-task started/completed and activity-started events (`service/history/historybuilder/event_factory.go:131-149`, `154-183`, `255-277`); build IDs exposed as searchable attributes with pinned/assigned/versioned/unversioned qualifiers (`common/worker_versioning/worker_versioning.go:34-40`, `93-139`); deployment name + versioning behavior on WFT-completed events (`service/history/historybuilder/event_factory.go:176-178`).
- **Operational safeguards**: vector-clock consistency leases and stale-state detection force re-reads rather than allowing mis-linked writes (`service/history/api/consistency_checker.go:37-79`, `service/history/api/respondactivitytaskcompleted/api.go:73-81`); CHASM refs carry `VersionedTransition` counters for staleness checks (`chasm/transition_history.go:10-33`, `proto/internal/temporal/server/api/persistence/v1/hsm.proto:114-119`); branch tokens are validated on history reads (`service/history/api/getworkflowexecutionhistory/api.go:199-269`).
- **Auditable at rest and post-hoc**: ordered raw-history APIs with pagination/reverse reads across branches (`tests/gethistory_test.go:487-660`), plus archival of both the history branch and the visibility record keyed by run identity (`service/history/archival/archiver.go:29-56`).
- **Tested at the lineage level**: integration tests assert `ParentInitiatedEventId == InitiatedEventId` across parent/child runs and root propagation (`tests/child_workflow_test.go:296-305`), and match failure events to scheduled event IDs (`tests/activity_test.go:582-591`).

Why not 10:
- Link targets are structurally validated only (non-empty namespace/run/op fields), never existence-checked against history at ingestion (`common/links/validator.go:36-88`); a link minted from a buffered event relies on later flush-time resolution via `requestIDToEventID` (`service/history/historybuilder/event_store.go:441-451`), and buffered signal events are silently discarded when the workflow finishes first (`service/history/historybuilder/event_store.go:168-176`) — leaving such a link permanently unresolvable.
- No cryptographic integrity over payloads (no content hashes chaining inputs→outputs; `BinaryChecksum` identifies worker binaries, not data) (`service/history/historybuilder/event_factory.go:171-173`).
- Visibility/search indexing is asynchronous, so provenance *queries* lag the authoritative log (visibility records are written by a background queue, not the history transaction).
- Transitional dual-framework complexity: Nexus completion tokens must be converted between HSM and CHASM address spaces keyed by scheduled event ID (`nexusworkflowref/nexusworkflowref.go:16-63`), and conversion fails loudly but requires caller retry logic.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Sequential causal addressing | `EventStore.AllocateEventID` assigns monotonically increasing per-run event IDs; `add()` stamps them onto events | service/history/historybuilder/event_store.go:58-62, 85-106 |
| Cause reference on command-derived events | Every scheduled/initiated event stores `WorkflowTaskCompletedEventId` linking effect → decision | service/history/historybuilder/event_factory.go:237, 515, 541, 607, 730, 854 |
| Effect→cause chain for activities | Completed/Failed/TimedOut/Canceled activity events carry `ScheduledEventId` + `StartedEventId`; completed carries `Result` | service/history/historybuilder/event_factory.go:279-333, 548-566 |
| Scheduled→started ID resolution | `wireEventIDs` maps scheduled IDs to started IDs (and request IDs to event IDs) when flushing buffers | service/history/historybuilder/event_store.go:381-454 |
| Causal task tokens | `tokenspb.Task` embeds namespace/workflow/run IDs + `scheduled_event_id` + `started_event_id`; activity variant adds activity ID/type/attempt/component ref | proto/internal/temporal/server/api/token/v1/message.proto:39-56 |
| Token-based completion validation | Completion handler resolves `token.GetScheduledEventId()` → `ActivityInfo`, rejects stale/not-found, fabricates missing Started event to keep chain complete | service/history/api/respondactivitytaskcompleted/api.go:60-110 |
| Server-side lineage index | `ActivityInfo` persisted with `scheduled_event_id`, `scheduled_event_batch_id`, `request_id`, `started_identity`, attempt, build-id info; keyed map `pendingActivityInfoIDs[scheduledEventID]` | proto/internal/temporal/server/api/persistence/v1/executions.proto:592-652; service/history/workflow/mutable_state_impl.go:4329-4403 |
| Reload original invocation args | `GetActivityScheduledEvent` reloads the exact scheduled event from the history store using `(eventID, version, batchID, branchToken)` | service/history/workflow/mutable_state_impl.go:1586-1619 |
| Input capture ("prompt" provenance) | Start event stores `WorkflowType`, `Input`, `Header`, memo/search attributes, identity, retry policy | service/history/historybuilder/event_factory.go:50-89 |
| Output→input linking ("final answer") | Terminal events store `WorkflowTaskCompletedEventId` + `Result`/`Failure` and `NewExecutionRunId` for continuation bridges | service/history/historybuilder/event_factory.go:335-381, 476-506 |
| Cross-run continuation lineage | `ContinuedExecutionRunId`, `FirstExecutionRunId`, `OriginalExecutionRunId`, `PrevAutoResetPoints`, `LastCompletionResult`, `ContinuedFailure` on started events | service/history/historybuilder/event_factory.go:58-70 |
| Parent/child + root lineage | Child-start event gets `ParentWorkflowExecution`, `ParentInitiatedEventId`, `ParentInitiatedEventVersion`; child's own start event mirrors them; `RootWorkflowExecution` propagated | service/history/historybuilder/event_factory.go:78, 91-98; tests/child_workflow_test.go:296-305 |
| Update (interactive call) lineage | UpdateAccepted event records `ProtocolInstanceId`, `AcceptedRequestMessageId`, `AcceptedRequestSequencingEventId`, full accepted request; UpdateCompleted records `AcceptedEventId` | service/history/historybuilder/event_factory.go:432-463 |
| Durable update state pointers | `UpdateInfo` oneof stores `{acceptance: event_id}`, `{completion: event_id + event_batch_id}`, `{admission: history_pointer}` | proto/internal/temporal/server/api/persistence/v1/update.proto:20-45 |
| Approval-style external input linkage | SignalWorkflow returns a `Link` referencing the SIGNALED event by RequestId; resolved to concrete EventId at flush | service/history/api/signalworkflow/api.go:96-122; service/history/api/link_util.go:33-50 |
| Typed cross-entity links | Link variants WorkflowEvent/BatchJob/NexusOperation/Activity/Workflow with required-field validation; max count/size limits | common/links/validator.go:17-88 |
| Links persisted on events | `event.Links` set on terminated/options-updated/cancel-requested/signaled events | service/history/historybuilder/event_factory.go:397, 427, 596, 840 |
| Nexus cross-service lineage | Outbound Nexus start request carries a link back to the caller's `NEXUS_OPERATION_SCHEDULED` event (exact EventId + EventType) | components/nexusoperations/executors.go:393-407 |
| Nexus artifact↔run binding | `NexusOperationCompletion` token binds operation → StateMachineRef + request_id embedded in the scheduled event + component ref; HSM↔CHASM conversion keyed by scheduled event ID | proto/internal/temporal/server/api/token/v1/message.proto:80-95; nexusworkflowref/nexusworkflowref.go:16-63 |
| Model version tracking | `WorkerVersionStamp` (build ID) on WFT started/completed and activity started; deployment name/version + behavior on WFT completed; SDK metadata | service/history/historybuilder/event_factory.go:131-149, 154-183, 255-277 |
| Version-searchable provenance | Build IDs encoded as search attributes (`pinned:`, `assigned:`, `versioned:`, `unversioned:` prefixes); `VersionStampToBuildIdSearchAttribute` | common/worker_versioning/worker_versioning.go:34-40, 93-139 |
| Activity build-id binding | `ActivityInfo.build_id_info` oneof binds activity to workflow build ID or independently assigned build ID | proto/internal/temporal/server/api/persistence/v1/executions.proto:636-652 |
| Fork/reset lineage | WFT-failed records `BaseRunId`/`NewRunId`/`ForkEventVersion`; reset derives new run from `baseRunID`; auto_reset_points persisted in execution info | service/history/historybuilder/event_factory.go:202-228; service/history/api/resetworkflow/api.go:56-105; proto/internal/temporal/server/api/persistence/v1/executions.proto:144 |
| Branched-history auditability | `VersionHistory{Item}` maps eventId→failover version per branch; `VersionHistories` tracks branches + current index; branch token validated on reads | proto/internal/temporal/server/api/history/v1/message.proto:19-34; service/history/api/getworkflowexecutionhistory/api.go:199-269 |
| Visibility provenance index | Visibility records persist Parent/Root workflow+run IDs, SearchAttributes, HistoryLength, StateTransitionCount, status | common/persistence/visibility/store/visibility_store.go:55-72, 105-121 |
| Post-close audit trail | Archiver archives history branch (BranchToken, NextEventID, CloseFailoverVersion) and visibility record with URIs | service/history/archival/archiver.go:29-56 |
| Consistency safeguard | Vector-clock lease consistency checker with predicates; stale cache → `ErrStaleState` + re-read instead of mis-linking | service/history/api/consistency_checker.go:37-79; service/history/api/respondactivitytaskcompleted/api.go:73-81 |
| Staleness fingerprints | `VersionedTransition {namespace_failover_version, transition_count}` guards component refs; `ExecutionStateChanged` compares refs | proto/internal/temporal/server/api/persistence/v1/hsm.proto:114-119; chasm/transition_history.go:10-33 |
| Callback (artifact delivery) binding | `CallbackInfo` persists callback definition, trigger, registration time, state, attempts, last failure — bound to the owning run | proto/internal/temporal/server/api/persistence/v1/executions.proto:855-880 |
| Lineage asserted in integration tests | Tests assert failed/timed-out activity events match scheduled IDs; parent initiated-event ID equals child's `ParentInitiatedEventId`; root propagation | tests/activity_test.go:582-591; tests/child_workflow_test.go:296-305 |

## Answers to Dimension Questions

**1. Can every output be traced to its inputs?**
Yes, end-to-end within a run. The start event captures the full input (type, arguments, header, memo, search attributes) (`service/history/historybuilder/event_factory.go:50-89`); each command becomes an effect event referencing the `WorkflowTaskCompletedEventId` that issued it (`service/history/historybuilder/event_factory.go:230-253`); activity outcomes reference their `ScheduledEventId`/`StartedEventId` and carry the result (`service/history/historybuilder/event_factory.go:279-295`); terminal events reference the completing workflow task and embed the result or failure (`service/history/historybuilder/event_factory.go:335-367`). Because event IDs are sequential and immutable (`service/history/historybuilder/event_store.go:58-62`), the chain is reconstructible purely from the log.

**2. Is provenance preserved through transformations?**
Yes, explicitly. Continue-as-new bridges runs via `NewExecutionRunId` + `ContinuedExecutionRunId` and forwards `LastCompletionResult`/`ContinuedFailure` (`service/history/historybuilder/event_factory.go:476-506`, `58-70`); retries retain `Attempt`, retry policy, and last failure on `ActivityInfo` with per-attempt started-state clearing (`service/history/workflow/activity.go:48-57`, `59-77`); resets record fork points (`BaseRunId`/`NewRunId`/`ForkEventVersion`, auto-reset points) (`service/history/historybuilder/event_factory.go:202-228`, executions.proto:144); replication conflicts produce branched histories tracked by `VersionHistories` with per-event failover versions (`proto/internal/temporal/server/api/history/v1/message.proto:19-34`). One caveat: continue-as-new intentionally truncates the visible log; earlier-run lineage lives only in pointer fields and prior runs' retained histories, not in one contiguous log.

**3. Are model versions tracked in lineage?**
Yes — mapped to workers rather than LLMs. Every workflow-task started/completed and activity-started event records a `WorkerVersionStamp` (build ID) and redirect counters (`service/history/historybuilder/event_factory.go:131-149`, `154-183`, `255-277`); WFT-completed additionally records deployment name, deployment version, and effective `VersioningBehavior` (`service/history/historybuilder/event_factory.go:176-179`); activities bind to build IDs via `build_id_info` (`executions.proto:636-652`); build IDs become searchable visibility attributes so any run can be queried by the code version that executed it (`common/worker_versioning/worker_versioning.go:110-139`). Inherited build IDs propagate to children and continued-as-new runs (`service/history/historybuilder/event_factory.go:79-82`, `500`, `875`).

**4. Can causal chains be audited?**
Yes. The log is append-only and totally ordered per run (enforced by the same machinery studied in dimension 02.03); it is readable forward/reverse/raw via history APIs with branch-token validation (`service/history/api/getworkflowexecutionhistory/api.go:199-269`, `tests/gethistory_test.go:487-660`); archived after close together with its visibility record (`service/history/archival/archiver.go:29-56`); and guarded against mis-attribution by vector-clock consistency checks and `VersionedTransition` staleness fingerprints (`service/history/api/consistency_checker.go:37-79`, `chasm/transition_history.go:10-33`). Auditing *across* entities requires composing links and parent pointers client-side; the server provides the primitives (typed links, event refs) but no native multi-hop lineage query API (see Gaps).

## Architectural Decisions

- **Event sourcing as the lineage substrate**: instead of separate audit tables, causality is encoded in the single authoritative event stream; derived indexes (ActivityInfo/ChildExecutionInfo/UpdateInfo) exist for dispatch speed and are keyed by the causal event IDs themselves (`service/history/workflow/mutable_state_impl.go:4329-4403`, `executions.proto:592-652`).
- **Sequential integer event IDs as universal addresses**: O(1) causal referencing inside a run (`event_store.go:58-62`), extended across storage batching by `(event_id, event_batch_id)` pairs and across forks by `(branch_token, event_id, version)` triples (`update.proto:32-41`, `message.proto:19-34`).
- **Capability-style task tokens**: workers get opaque tokens carrying exactly the coordinates needed to attribute their responses (`token/v1/message.proto:39-56`), making spoofed/stale attributions detectable server-side.
- **Typed, size-limited Link vocabulary**: five link variants with mandatory identifying fields and count/size caps (`common/links/validator.go:17-88`) — extensibility with guardrails.
- **Deferred link resolution**: links created while target events are still buffered use RequestIdReferences resolved during flush (`link_util.go:33-50`, `event_store.go:441-451`).
- **Dual-plane persistence of provenance**: history (authoritative, per-run) + visibility (queryable, denormalized incl. parent/root IDs and build-ID attributes) + archival (post-close immutability) (`visibility_store.go:55-72`, `archiver.go:29-56`).

## Notable Patterns

- **Factory-enforced attribution**: no hand-built events; all events flow through `EventFactory`, so causal fields cannot be forgotten per-event-type (`service/history/historybuilder/event_factory.go:26-1118`).
- **Self-healing lineage gaps**: if a started event is missing when an activity completes (force-complete path), the server fabricates the missing Started event to keep the chain well-formed rather than emitting an orphan completion (`service/history/api/respondactivitytaskcompleted/api.go:87-105`).
- **Root anchoring**: every descendant carries `RootWorkflowExecution` alongside immediate-parent pointers, enabling flat traversal of deep trees (`tests/child_workflow_test.go:303-308`).
- **Stale-ref detection over silent overwrite**: component refs and history reads compare versioned-transition counters / branch tokens and fail with typed errors (`ErrStaleState`, `ErrInvalidComponentRef`) rather than proceeding (`chasm/transition_history.go:14-33`, `getworkflowexecutionhistory/api.go:260-268`).
- **Provenance-preserving reset semantics**: Nexus completion tokens embed the `request_id` from the original scheduled event specifically so an operation can be completed correctly *after* a workflow reset recreates its state (`token/v1/message.proto:90-92`).

## Tradeoffs

- **Storage vs. self-containment**: embedding full inputs/results in events makes every fact locally attributable, but drives history size growth; mitigation is continue-as-new (which fragments the single-log narrative) and payload offloading (which weakens self-containment).
- **Write throughput vs. total order**: strict per-run sequencing funnels all lineage writes through one shard transaction per run — perfect ordering, limited horizontal scaling per run.
- **Immediate vs. eventual queryability**: authoritative lineage is instantly durable in history; visibility indexing is async, so "find all runs using build X" lags behind truth (background visibility queue).
- **Flexibility vs. verifiability of links**: links accept user-supplied identifiers validated only for shape, keeping the API open but deferring existence/integrity checks to consumers.
- **Denormalized indexes vs. consistency discipline**: fast dispatch via ActivityInfo/etc. costs continuous reconciliation effort (consistency checker, rebuilders, tombstones).

## Failure Modes / Edge Cases

- **Unresolved request-ID links**: `FlushBufferToCurrentBatch` discards buffered events when the workflow has finished (`service/history/historybuilder/event_store.go:166-176`); any link minted against such a request ID never resolves to an event ID.
- **Stale-cache attribution races**: a completion arriving against a stale cached run returns `ErrStaleState` and must be retried after re-read (`respondactivitytaskcompleted/api.go:73-81`) — correctness preserved, latency added.
- **Branch-token mismatch on audit reads**: reading a forked run's history without the correct branch token fails validation (`getworkflowexecutionhistory/api.go:260-268`) — safe, but surprises auditors expecting a single linear log.
- **Cross-framework ref drift**: a Nexus completion addressed in HSM form may arrive after the operation was recreated under CHASM (or vice versa); conversion handles it but errors on non-convertible shapes (`nexusworkflowref/nexusworkflowref.go:22-63`).
- **Unassigned build-ID window**: `ActivityInfo.assigned_build_id` may briefly be absent even on versioned queues until dispatch heals it (`executions.proto:636-639`) — a transient hole in version provenance for activities.
- **Orphaned activity info on missing events**: reloading a scheduled event that fell out of retention yields `ErrMissingActivityScheduledEvent` (`mutable_state_impl.go:1609-1618`) — lineage index exists but backing evidence is gone.

## Future Considerations

- Existence-check Link targets at ingestion (or expose a server-side link-resolution endpoint) so consumers never hold dangling references.
- Add optional payload integrity digests (hash chaining input→output events) for tamper-evident provenance where regulatory audits demand it.
- Provide a native multi-hop lineage query (e.g., "all descendants and contributing activities of run R") built on existing parent/root pointers and links, instead of requiring client-side graph reconstruction.
- Complete the HSM→CHASM migration to retire dual-address-space token conversion (`nexusworkflowref/`).
- Surface visibility-index freshness (e.g., watermark timestamps) so provenance queries can distinguish settled from pending records.

## Questions / Gaps

- No evidence found of cryptographic integrity mechanisms over payloads or event contents; searches for hash/chaining concepts surfaced only worker-binary checksums (`BinaryChecksum`) and DB-level checksums under `api/checksum`. What was searched: `grep -ri "checksum|hash"` across `service/history`, `common/persistence`, and `api`.
- No evidence found of a server-native lineage-graph query API; tracing beyond direct pointers is delegated to SDK replay. Searched `service/frontend`, `service/history/api` for lineage/graph/traversal endpoints.
- Whether buffered-event link resolution is guaranteed before any consumer can observe the link (vs. only best-effort at flush) could not be fully confirmed from a single file; behavior appears flush-dependent per `event_store.go:161-181` and `441-451`.
- Per-prompt/per-call semantic provenance (e.g., which retrieved document influenced which output) is out of scope for the server: headers/memo/search attributes are user-declared payloads with schema enforcement only (`common/searchattribute/validator.go`), not content analysis.

---

Generated by `10.03-causal-links-and-lineage` against `temporal`.
