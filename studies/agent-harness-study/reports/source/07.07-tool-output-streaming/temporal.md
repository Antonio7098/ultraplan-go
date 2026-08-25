# Source Analysis: temporal

## 07.07 Tool Output Streaming

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go server (go.temporal.io/server), protobuf APIs, Cassandra/SQL/ES persistence; gRPC services (frontend, history, matching, worker) |
| Analyzed | 2026-08-25 |

## Summary

Temporal treats long-running tools as **activities** and their orchestrator ("the model") as a **workflow**. Tool output streaming is implemented as a **pull-based progress channel** rather than a push event stream: a running activity periodically calls `RecordActivityTaskHeartbeat`, carrying a `Details` payload of partial output (`service/frontend/workflow_handler.go:1445-1511`). The server persists the latest snapshot in workflow mutable state (`service/history/workflow/mutable_state_impl.go:2105-2135`), exposes it to observers via `DescribeWorkflowExecution` (`service/history/api/describeworkflow/api.go:206-213`), redelivers it to the next retry attempt (`service/history/api/recordactivitytaskstarted/api.go:288`), and writes it into durable history on timeout/failure (`service/history/timer_queue_active_task_executor.go:346`). The same heartbeat call is bidirectional: its response returns control flags (`CancelRequested`, `ActivityPaused`, `ActivityReset`) back to the tool worker in-band (`service/history/api/recordactivitytaskheartbeat/api.go:97-101`), making it the interruption channel during streaming.

The orchestrating workflow itself does **not** see partial output mid-flight automatically — it receives the final result at completion. Partial state is observable by external actors through heartbeats-in-mutable-state, strong-consistency queries (`service/history/api/queryworkflow/api.go:33-307`), and update long-polls with lifecycle stages (`service/history/api/pollupdate/api.go:19-82`). Operational safeguards are explicit: blob-size limits that fail an activity whose heartbeat details are too large (`service/frontend/workflow_handler.go:1471-1496`), priority rate-limiting for heartbeat APIs (`service/frontend/configs/quotas.go:100-103`), and dedicated metrics (`service/history/workflow/mutable_state_impl.go:2124-2134`). The design is mature and well-tested, but intermediate progress is lossy (last-writer-wins snapshot, not event-sourced) and there is no push/streaming transport for tool output — the only gRPC streams in the codebase are internal replication (`proto/internal/temporal/server/api/historyservice/v1/service.proto:350`).

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. The heartbeat protocol is a first-class, documented API with durability across retries, timeouts, failover, and cross-cluster replication; cancellation/pause/resume are delivered through the same channel; size limits and quotas protect the pipeline; integration and unit tests verify the behavior end to end. It falls short of 9–10 because partial outputs are single-snapshot (no progress timeline/event history of intermediate results), the orchestrating workflow cannot consume progress without building a query/signal mechanism itself, there is no streaming transport for observers (poll-only), and the Web UI lives outside this source so UI integration is only inferable from the server-side data surface.

## Evidence Collected

Every entry includes a file path relative to the source root with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Streaming/progress API | `RecordActivityTaskHeartbeat` frontend handler accepts task token + `Details` payloads; doc comment states it reports liveness *and* progress | `service/frontend/workflow_handler.go:1440-1511` |
| Progress semantics stated | History engine comment: heartbeat "can be used ... For reporting progress of the activity" independent of liveness config | `service/history/history_engine.go:634-643` |
| Heartbeat implementation (history) | `recordactivitytaskheartbeat.Invoke` updates mutable state under workflow lease and returns `CancelRequested`/`ActivityPaused`/`ActivityReset` | `service/history/api/recordactivitytaskheartbeat/api.go:17-102` |
| Partial output buffer write | `UpdateActivityProgress` sets `ai.LastHeartbeatDetails = request.Details` (overwrite, last-writer-wins) plus `LastHeartbeatUpdateTime` | `service/history/workflow/mutable_state_impl.go:2105-2122` |
| Durable schema field | Persisted `ActivityInfo.last_heartbeat_details = 31` in executions proto | `proto/internal/temporal/server/api/persistence/v1/executions.proto:629` |
| Observer visibility | `GetPendingActivityInfo` copies `LastHeartbeatUpdateTime` → `p.LastHeartbeatTime`, `LastHeartbeatDetails` → `p.HeartbeatDetails` | `service/history/workflow/activity.go:138-141` |
| Describe surfaces pending activities | `DescribeWorkflowExecution` builds `result.PendingActivities` from all pending activity infos | `service/history/api/describeworkflow/api.go:206-213` |
| Retry redelivery of partial output | `RecordActivityTaskStarted` response carries `response.HeartbeatDetails = ai.LastHeartbeatDetails` for the new attempt | `service/history/api/recordactivitytaskstarted/api.go:284-291` |
| Delivery to workers via matching | `PollActivityTaskQueueResponse.heartbeat_details = 13` proto field; engine populates `HeartbeatDetails: historyResponse.HeartbeatDetails` | `proto/internal/temporal/server/api/matchingservice/v1/request_response.proto:126-143`; `service/matching/matching_engine.go:3421-3442` |
| Durability on timeout | Heartbeat timeout writes `TimeoutFailureInfo.LastHeartbeatDetails = ai.LastHeartbeatDetails` into history | `service/history/timer_queue_active_task_executor.go:305-362` (esp. :346) |
| Durability on failure/completion | Failure path records `request.GetLastHeartbeatDetails()` into failure event; WFT-completed handler maps `ai.LastHeartbeatDetails` | `service/history/api/respondactivitytaskfailed/api.go:87-91`; `service/history/api/respondworkflowtaskcompleted/workflow_task_completed_handler.go:650-656` |
| Cross-cluster durability | Replication converter copies `Details: activityInfo.LastHeartbeatDetails` | `service/history/replication/raw_task_converter.go:175-190` (esp. :182) |
| Cancellation flag set | `ApplyActivityTaskCancelRequestedEvent` sets `ai.CancelRequested = true`; comment notes activities "can still call RecordActivityHeartBeat() to see cancellation while reporting progress" | `service/history/workflow/mutable_state_impl.go:4853-4881` |
| Cancellation returned in-band | Heartbeat response echoes `CancelRequested` read from `ai.CancelRequested` | `service/history/api/recordactivitytaskheartbeat/api.go:77-101` |
| Pause/unpause/reset interrupts | Frontend `PauseActivity` (:7166), `UnpauseActivity` (:7199), `ResetActivity` (:7231); pause states `PAUSED` / `PAUSE_REQUESTED` computed in `activity.go:159-176` | `service/frontend/workflow_handler.go:7166-7261`; `service/history/workflow/activity.go:159-176` |
| Strong-consistency live reads | `QueryWorkflow` buffers consistent queries (`queryReg.BufferQuery`) or dispatches directly via matching; reject conditions; buffered-query cap `MaxBufferedQueryCount` | `service/history/api/queryworkflow/api.go:168-307` (esp. :175-217) |
| Update long-poll (partial stages) | `PollWorkflowExecutionUpdate` waits on `upd.WaitLifecycleStage(ctx, waitStage, softTimeout)` returning stage+outcome | `service/history/api/pollupdate/api.go:19-82` |
| Heartbeat timeout scheduling | Timer sequence computes heartbeat deadline from `StartedTime`/`LastHeartbeatUpdateTime` + `HeartbeatTimeout` | `service/history/workflow/timer_sequence.go:399-444` |
| Size-limit safeguard | Oversized heartbeat details fail the activity with `FailureReasonHeartbeatExceedsLimit` and return `CancelRequested: true` to stop the worker; oversized failure-path details stripped | `service/frontend/workflow_handler.go:1471-1496`, `1886-1904`; constant at `common/util.go:104-105` |
| Rate limiting safeguard | Heartbeat APIs classified P1 "Progress APIs" quota entry (default 1 RPS bucket entry listed alongside other protected APIs) | `service/frontend/configs/quotas.go:100-103` |
| Observability metrics | `ActivityPayloadSize` counter for details bytes; `ActivityHeartbeatCount` with `has_details` tag | `service/history/workflow/mutable_state_impl.go:2124-2134` |
| Integration test: progress + details | `TestActivityHeartBeatWorkflow_Success` calls `RecordActivityTaskHeartbeat` with `Details` 10× mid-activity | `tests/activity_test.go:374-459` (call at :449-453) |
| Integration test: timeout carries partials | Assertions `timeoutErr.HasLastHeartbeatDetails()` and decoded value `2` after Heartbeat/ScheduleToClose timeouts | `tests/activity_test.go:346-364` |
| Integration test: pause while running | End-to-end pause → `PENDING_ACTIVITY_STATE_PAUSE_REQUESTED` → `PAUSED`, no retry until unpause, `PauseInfo.Manual` identity/reason recorded | `tests/activity_api_pause_test.go:114-231` |
| Unit test: metrics contract | `TestUpdateActivityProgress_HeartbeatCountMetric` asserts counters for heartbeats with/without details | `service/history/workflow/mutable_state_impl_test.go:8863-8881` |
| Unit test: round-trip fidelity | `LastHeartbeatDetails` equality assertion in mutable-state sync test | `service/history/workflow/mutable_state_impl_test.go:5520-5528` |
| No push-stream transport | Only bidi stream RPCs are `StreamWorkflowReplicationMessages` (internal replication); no public streaming of tool output | `proto/internal/temporal/server/api/historyservice/v1/service.proto:350` |

## Answers to Dimension Questions

1. **Can tools stream progress?**
   Yes, via a client-pull heartbeat protocol. An activity worker sends arbitrary `Details` payloads with each `RecordActivityTaskHeartbeat` call (`service/frontend/workflow_handler.go:1445-1511`, `service/history/api/recordactivitytaskheartbeat/api.go:17-102`). The API is explicitly dual-purpose — liveness *and* progress (`service/history/history_engine.go:634-637`). There is no server-push stream to observers; progress must be polled (see Q6).

2. **Are partial outputs durable?**
   Yes. The latest heartbeat details persist in the workflow's mutable-state row (`last_heartbeat_details`, `proto/internal/temporal/server/api/persistence/v1/executions.proto:629`; written at `service/history/workflow/mutable_state_impl.go:2113`). They survive retry (redelivered to the next attempt, `service/history/api/recordactivitytaskstarted/api.go:288`; passed through matching, `service/matching/matching_engine.go:3436`), are replicated to passive clusters (`service/history/replication/raw_task_converter.go:182`), and are promoted into immutable history events when the attempt ends badly — timeout (`service/history/timer_queue_active_task_executor.go:346`) and failure (`service/history/api/respondactivitytaskfailed/api.go:87-91`). Caveat: only the *latest* snapshot survives; intermediate snapshots are overwritten (`mutable_state_impl.go:2113` is an assignment, not an append).

3. **Does the model act on partial output?**
   Not automatically. In Temporal's architecture the orchestrating workflow does not observe heartbeat details while the activity runs; it receives either completion, or a failure carrying the last heartbeat details (`tests/activity_test.go:346-364` proves the SDK can decode them post-timeout). Acting on partial output requires deliberate mechanisms: (a) workflow-exposed query handlers answered under strong consistency (`service/history/api/queryworkflow/api.go:168-307`), which an agent-style workflow can combine with signals to adapt mid-run; (b) update long-polling with lifecycle wait stages (`service/history/api/pollupdate/api.go:61-67`); or (c) the retry path where the *next attempt* receives prior partials as input context (`recordactivitytaskstarted/api.go:288`). So the plumbing exists, but consumption by the orchestrator is opt-in rather than built into the activity result flow.

4. **Can users interrupt?**
   Yes, through three cooperating channels. Workflow-driven cancel: `ActivityTaskCancelRequested` sets `ai.CancelRequested` (`service/history/workflow/mutable_state_impl.go:4853-4881`), surfaced to the running worker on its next heartbeat (`recordactivitytaskheartbeat/api.go:77,97-98`) — the code comment explicitly recommends calling heartbeat to discover cancellation even when not tracking liveness (`mutable_state_impl.go:4871-4873`). Operator-driven pause/resume/reset: `PauseActivity`/`UnpauseActivity`/`ResetActivity` (`service/frontend/workflow_handler.go:7166-7261`) with observable states `PAUSED` vs `PAUSE_REQUESTED` depending on whether the worker has acknowledged (`service/history/workflow/activity.go:159-176`), verified end-to-end in `tests/activity_api_pause_test.go:114-231`. Note interruption is cooperative: the server marks intent and relies on the worker's next heartbeat/RPC to observe it; a non-heartbeating, non-cooperating worker is stopped only by timeouts (`timer_sequence.go:399-444`).

5. **Are partial outputs clearly marked?**
   Yes, within the limits of the snapshot model. Observers get typed metadata distinguishing progress data from results: `PendingActivityInfo.HeartbeatDetails` alongside `Attempt`, `LastHeartbeatTime`, `State`, `Paused`, `PauseInfo` (`service/history/workflow/activity.go:100-176`, exposed via `service/history/api/describeworkflow/api.go:206-213`), so partial data is unambiguously attributable to a still-open activity attempt with attempt count and timestamps. On terminal outcomes, partials are embedded in `TimeoutFailureInfo.LastHeartbeatDetails` (`timer_queue_active_task_executor.go:346`) — i.e., they arrive inside a failure structure, clearly separated from normal results. There is no marker distinguishing "stale" heartbeat details after a long silence beyond `LastHeartbeatTime`.

## Architectural Decisions

- **Single bidirectional RPC instead of separate progress/interrupt channels.** `RecordActivityTaskHeartbeat` both uploads partial output and downloads control flags (`cancel_requested`, `activity_paused`, `activity_reset` — `service/history/api/recordactivitytaskheartbeat/api.go:97-101`). This gives every tool invocation a natural rendezvous point without requiring a persistent connection.
- **Snapshot-in-mutable-state, not event-log.** Progress overwrites one persisted field (`executions.proto:629`; write at `mutable_state_impl.go:2113`) instead of emitting history events per heartbeat. This keeps history append-only and replayable while bounding storage cost — at the price of losing the progress timeline.
- **Promotion to history only on terminal outcomes.** Partials enter the immutable record solely when an attempt times out or fails (`timer_queue_active_task_executor.go:346`; `respondactivitytaskfailed/api.go:87-91`), preserving forensic value without per-heartbeat event volume.
- **Pull-based observation.** Live inspection goes through `DescribeWorkflowExecution` (server-side projection, `describeworkflow/api.go:206-213`) and strongly-consistent queries dispatched either on workflow tasks or directly through matching depending on safety analysis (`queryworkflow/api.go:175-204`).
- **Frontend-enforced resource guards.** Size validation happens before history writes and converts overflow into a controlled activity failure + cancel signal (`workflow_handler.go:1471-1496`), and heartbeat endpoints get protected priority quotas (`configs/quotas.go:100-103`).
- **Long-poll lifecycle staging for updates.** Callers wait for `Accepted`/`Completed` stages with a soft server timeout returning the reached stage rather than erroring (`pollupdate/api.go:61-67`) — a deliberate availability-over-completeness choice.

## Notable Patterns

- **Task-token addressing with ID-based fallback**: `RecordActivityTaskHeartbeatById` resolves scheduledEventID from activityID when no token is available (`service/history/api/recordactivitytaskheartbeat/api.go:55-61`; frontend variant `workflow_handler.go:1513-1627`).
- **Stale-cache detection on progress writes**: heartbeat rejects with `ErrStaleState` when the activity info is missing *and* behind `NextEventID`, forcing cache refresh after extreme persistence failures (`recordactivitytaskheartbeat/api.go:64-71`).
- **Metrics-tagged progress accounting**: heartbeat counters tagged with `has_details=true/false` and payload-size counters give operators direct visibility into how much streaming traffic actually carries output (`mutable_state_impl.go:2124-2134`).
- **Query fast-fail under broken workflow tasks**: consistent queries fail fast when the workflow task attempt ≥ 3, avoiding wasted history loads (`queryworkflow/api.go:155-164`), and buffered queries are capped (`queryworkflow/api.go:211-215`).
- **Idempotent, reason-attributed operator actions**: pause requests carry identity/reason/request-id and surface as `PauseInfo.Manual` in descriptions (`tests/activity_api_pause_test.go:176-222`).

## Tradeoffs

- **Push vs pull**: polling keeps the server stateless w.r.t. observers and works everywhere gRPC unary works, but consumers (UIs, agents) see progress only as fresh as their poll interval; no subscription exists.
- **Snapshot vs timeline**: last-writer-wins bounds storage (`executions.proto:629`) but discards intermediate progress — impossible to answer "how did progress evolve" from the server alone.
- **Cooperative interruption vs forced preemption**: cancel/pause are cheap and safe but require worker cooperation via heartbeat/context propagation; a wedged worker is only reaped by heartbeat timeout (`timer_sequence.go:399-444`).
- **Strict size limits vs rich partials**: oversized details deliberately kill the activity with `FailureReasonHeartbeatExceedsLimit` and `CancelRequested: true` (`workflow_handler.go:1482-1495`) — protects the persistence layer but turns a payload-policy mistake into a hard tool failure.
- **Orchestrator isolation vs situational awareness**: workflows stay deterministic and don't ingest raw progress (good for replay), but an agent-like workflow wanting to react to partial output must implement its own query/signal feedback loop.

## Failure Modes / Edge Cases

- **Heartbeat-after-timeout race**: a late heartbeat gets `EntityNotExistsError` once the activity timed out (`workflow_handler.go:1440-1444` doc; exercised in `tests/activity_test.go:807-898` where post-timeout poll/complete yields `ErrActivityTaskNotFound`).
- **Client-side batching skew**: SDK heartbeat batching means the last logical heartbeat may not reach the server before a timeout — integration tests explicitly shrink intervals to compensate (`tests/activity_test.go:193-205`).
- **Consistent-query buffer saturation**: exceeding `MaxBufferedQueryCount` fails the query with `ErrConsistentQueryBufferExceeded` (`queryworkflow/api.go:211-215`) — sustained observer pressure degrades into errors, not silent drops.
- **Query starvation during failing tasks**: repeated workflow-task failures make queries return `WorkflowNotReady` after 3 attempts (`queryworkflow/api.go:155-164`).
- **Oversized partials**: enforced at two layers — inline heartbeat (`workflow_handler.go:1471-1496`) and failure-response attachment (`workflow_handler.go:1886-1904`, details stripped and server-failure appended).
- **Split-brain staleness**: stale mutable state detected via event-ID comparison triggers refresh rather than corrupting progress (`recordactivitytaskheartbeat/api.go:64-71`).
- **Pause acknowledgment ambiguity**: between request and ack the state reads `PAUSE_REQUESTED` while the worker still runs (`activity.go:165-168`) — observers must treat this as transitional.

## Future Considerations

- A server-side subscription/streaming endpoint for activity progress would remove poll-latency for observers (today only internal replication uses gRPC streams, `historyservice/v1/service.proto:350`).
- Optional ring-buffer retention of recent heartbeat snapshots would enable progress-timeline debugging without changing the default bounded-storage model.
- Exposing heartbeat freshness thresholds (e.g., derived from `LastHeartbeatTime`) as first-class "stale progress" markers would strengthen Q5's marking story.
- The standalone-activity execution path (`StartActivityExecution`, gated by `IsStandaloneActivityEnabled` at `workflow_handler.go:1464-1466`) reuses the same heartbeat channel; watching its evolution is worthwhile since it generalizes activities into free-standing tools.

## Questions / Gaps

- **UI emitters**: No evidence found inside this source. The Web UI is a separate repository not present here, so UI rendering of `HeartbeatDetails` could not be verified directly. What was found: the complete server-side data surface (`describeworkflow/api.go:206-213`, `activity.go:138-141`) and a CLI-facing message containing `heartbeat_details` (`proto/internal/temporal/server/api/cli/v1/message.proto:42`). Search boundary: `cmd/`, `tools/`, `service/`, `proto/`, `docs/`.
- **Model-feedback loop**: No server-side evidence that partial output is fed back into orchestration decisions mid-flight (by design). Whether any SDK-level helper bridges heartbeats into workflow visibility is out of this source's scope (SDK repos absent).
- **Per-heartbeat throughput tuning**: dynamicconfig knobs for heartbeat-specific throttling beyond the generic quota entries were not located; only the P1 quota classification was confirmed (`configs/quotas.go:100-103`). No evidence found of a dedicated heartbeat-QPS dynamic config in this checkout.

---

Generated by `07.07-tool-output-streaming` against `temporal`.
