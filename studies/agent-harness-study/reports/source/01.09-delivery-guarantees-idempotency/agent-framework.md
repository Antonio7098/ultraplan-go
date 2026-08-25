# Source Analysis: agent-framework

## Delivery Guarantees and Idempotency

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | .NET 8/10 (C#) + Python 3.10+, both backed by Durable Task Framework, Azure Durable Functions, and Azure Storage/Redis |
| Analyzed | 2026-07-03 |

## Summary

Microsoft Agent Framework is a multi-language SDK for building agents on top of
`Microsoft.Extensions.AI` (C#) and a Pydantic-typed core (Python). Delivery
guarantees and idempotency are largely **delegated to the underlying Durable
Task / Durable Entities / Azure Functions platform** rather than implemented
in-framework. The framework layer adds:

- a correlation-ID scheme (`correlationId`) on every `RunRequest` and on every
  persisted `DurableAgentStateRequest` / `DurableAgentStateResponse` entry
  (`dotnet/src/Microsoft.Agents.AI.DurableTask/RunRequest.cs:37`,
  `python/packages/durabletask/agent_framework_durabletask/_models.py:115`,
  `python/packages/durabletask/agent_framework_durabletask/_durable_agent_state.py:60`),
- per-session durable entities that serialize concurrent calls
  (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:13`,
  `docs/features/durable-agents/README.md:28`),
- a client-side poll-for-response loop matched by correlation ID
  (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentRunHandle.cs:47`,
  `python/packages/durabletask/agent_framework_durabletask/_executors.py:303`),
- and a **reliable streaming** layer (Redis Streams + cursor) that is
  *explicitly documented as at-least-once* with no exactly-once guarantee
  (`dotnet/samples/04-hosting/DurableAgents/ConsoleApps/07_ReliableStreaming/README.md:152`,
  `dotnet/samples/04-hosting/DurableAgents/AzureFunctions/08_ReliableStreaming/README.md:207`).

Workflow orchestrations rely on Durable Task's deterministic-replay model;
side-effect-bearing steps (custom status publication, HITL waits) are guarded
by `is_replaying` to be issued at most once across replay
(`python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:755`,
`python/packages/durabletask/agent_framework_durabletask/_workflows/context.py:39`).

In short: the framework has a clear correlation-id and entity-serialization
model for the durable-agent path, a deterministic-replay-aware orchestrator,
and an at-least-once streaming story, but it has **no general-purpose
exactly-once / at-most-once mode** and **no global idempotency-key store**.
Most "exactly-once" semantics on the durable path come from the fact that
Durable Entities process operations one-at-a-time per key and the framework
persists the response against the request's correlation ID — if a caller
signals the entity twice with the same correlation ID, the persisted response
is overwritten rather than duplicated, but the agent itself runs twice
unless the caller deduplicates by correlation ID.

## Rating

**6 / 10 — Present but inconsistent; weakly documented; correct for the
Durable Task / Durable Entities subsystem, but the in-process agent loop,
MCP/AG-UI/tool layers, and general event channels do not advertise guarantees
and rely on caller discipline.**

The Durable Agents subsystem scores 8 in isolation: tests cover correlation
matching, ordering, replay filtering of reasoning content, and explicit
replay-aware side-effect guards
(`python/packages/durabletask/tests/test_durable_entities.py:398-444`,
`python/packages/durabletask/tests/test_durable_entities.py:517-583`). The
reliable-streaming samples are even honest about their at-least-once limit
and document exactly how a client must dedupe. But the framework as a whole
mixes this rigorous durability model with an in-memory agent path that has no
deduplication or transactional boundary at all, and there is no consistent
guarantee story across the package surface.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Correlation-ID generation per request | `RunRequest` defaults `CorrelationId = Guid.NewGuid().ToString("N")` | `dotnet/src/Microsoft.Agents.AI.DurableTask/RunRequest.cs:37` |
| Correlation-ID generation per request (Python) | `RunRequest` requires `correlation_id` and persists it on every state entry | `python/packages/durabletask/agent_framework_durabletask/_models.py:115`; `python/packages/durabletask/agent_framework_durabletask/_durable_agent_state.py:60,381-384` |
| Correlation-ID on responses | `DurableAgentStateResponse.FromResponse(correlationId, response)` ties a response entry to its request | `dotnet/src/Microsoft.Agents.AI.DurableTask/State/DurableAgentStateResponse.cs:26-30`; `python/packages/durabletask/agent_framework_durabletask/_durable_agent_state.py:688-695` |
| Per-session entity serializes concurrent calls | `TaskEntity<DurableAgentState>` and "Durable Entities serialize access to each entity instance" | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:13`; `docs/features/durable-agents/README.md:28` |
| Client signal → entity | `Entities.SignalEntityAsync(sessionId, "Run", request)` | `dotnet/src/Microsoft.Agents.AI.DurableTask/DefaultDurableAgentClient.cs:23-29` |
| Client poll → response matching by correlation ID | `FirstOrDefault(r => r.CorrelationId == this.CorrelationId)` | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentRunHandle.cs:65-67` |
| Exponential-backoff polling | "50ms → 3s, doubled each tick" | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentRunHandle.cs:49-81` |
| Python client poll → response matching by correlation ID | `state.try_get_agent_response(correlation_id)` | `python/packages/durabletask/agent_framework_durabletask/_executors.py:419-431`; `python/packages/durabletask/agent_framework_durabletask/_durable_agent_state.py:450-474` |
| Python poll config defaults | `DEFAULT_MAX_POLL_RETRIES = 30`, `DEFAULT_POLL_INTERVAL_SECONDS = 1.0` | `python/packages/durabletask/agent_framework_durabletask/_constants.py:28-29` |
| HTTP trigger → entity → poll → HTTP | `client.signal_entity` then `_get_response_from_entity` | `python/packages/azurefunctions/agent_framework_azurefunctions/_app.py:739-759, 1047-1117` |
| HTTP 202 fire-and-forget mode | `if not run_request.wait_for_response: return acceptance_response`; `x-ms-wait-for-response: false` | `python/packages/azurefunctions/agent_framework_azurefunctions/_app.py:255-260, 760-771`; `python/packages/durabletask/agent_framework_durabletask/_executors.py:506-517` |
| Orchestration call_entity uses checkpointed result | `context.Entities.CallEntityAsync<AgentResponse>` — Durable Task replays on failure | `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAIAgent.cs:136-139` |
| Orchestration deterministic-ID helper | `NewAgentSessionId` calls `context.NewGuid()` (replay-safe) | `dotnet/src/Microsoft.Agents.AI.DurableTask/TaskOrchestrationContextExtensions.cs:39-46` |
| Python orchestrator skips live side effects on replay | `publish_live_status` gated on `ctx.is_replaying` | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:755-767` |
| Workflow orchestrator protocol: is_replaying, current_utc_datetime, new_uuid | Explicit "replay-safe" properties | `python/packages/durabletask/agent_framework_durabletask/_workflows/context.py:39-93` |
| Durable Task platform provides determinism contract | "Durable Task orchestrations require deterministic replay - the same code must execute identically across replays" | `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableExecutorDispatcher.cs:5-8`; `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableWorkflowRunner.cs:5-8` |
| Agent execution atomic with state commit | "the durable scheduling of the orchestration and the agent state update to be committed atomically in a single transaction" | `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAgentContext.cs:80-87` |
| Reliable streaming — explicitly at-least-once | "This sample provides **at-least-once delivery**" + "Clients should handle duplicate messages idempotently" | `dotnet/samples/04-hosting/DurableAgents/ConsoleApps/07_ReliableStreaming/README.md:152-160`; `dotnet/samples/04-hosting/DurableAgents/AzureAgents/AzureFunctions/08_ReliableStreaming/README.md:207-216` |
| Reliable streaming — Redis XADD writes only, no consumer-group ack | `db.StreamAddAsync(streamKey, entries)`; reads use `XREAD` with cursor | `dotnet/samples/04-hosting/DurableAgents/AzureFunctions/08_ReliableStreaming/RedisStreamResponseHandler.cs:93, 150` |
| Reliable streaming — per-chunk sequence number in payload | `NameValueEntry("sequence", sequenceNumber++)` | `dotnet/samples/04-hosting/DurableAgents/AzureFunctions/08_ReliableStreaming/RedisStreamResponseHandler.cs:88` |
| Reliable streaming — TTL on Redis key (default 10 minutes) | `db.KeyExpireAsync(streamKey, this._streamTtl)` | `dotnet/samples/04-hosting/DurableAgents/AzureFunctions/08_ReliableStreaming/RedisStreamResponseHandler.cs:96, 111` |
| Per-stream idempotency: identical-text dedupe | `DeduplicatingAgentSkillsSource` / built-in `DeduplicatingAgentSkillsSource` | `docs/decisions/0021-agent-skills-design.md:436-458` |
| Tool-call / function-call content serialized for replay | `DurableAgentStateFunctionCallContent.call_id` plus matched `DurableAgentStateFunctionResultContent.call_id` | `python/packages/durabletask/agent_framework_durabletask/_durable_agent_state.py:928-1019` |
| Reasoning content filtered from replay history | `_to_replayable_message` strips `content.type == "reasoning"` | `python/packages/durabletask/agent_framework_durabletask/_entities.py:195-208`; test: `python/packages/durabletask/tests/test_durable_entities.py:398-444` |
| Error preserved as a normal response entry | `is_error=True` flag, persisted as `DurableAgentStateResponse` | `python/packages/durabletask/agent_framework_durabletask/_entities.py:177-193` |
| User-registered duplicates rejected at builder | `raise ValueError("Duplicate executor participant detected")` | `python/packages/orchestrations/agent_framework_orchestrations/_sequential.py:142-149`; same in `_concurrent.py:249` |
| MCP approval replay test | `test_approval_replay_is_blocked` | `python/packages/ag-ui/tests/ag_ui/test_agent_wrapper_comprehensive.py:1031-1141` |
| AG-UI message-adapter dedup with content-hash fallback | `_deduplicate_messages` (by `message_id` or content hash) | `python/packages/ag-ui/tests/ag_ui/test_message_adapters.py:1118-1240` |
| A2A re-delivery dedup (UCP-style queue test) | `mock_event_queue.enqueue_event.assert_awaited_once` | `python/packages/a2a/tests/test_a2a_executor.py:39-156` |
| AG-UI HITL `request_id` correlation on resume | `await ctx.request_info(..., request_id="approval-1")` followed by resume with same id | `python/packages/ag-ui/tests/ag_ui/test_workflow_run.py:126, 242, 1302`; `python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:474-849` |
| HITL reject-and-retry path | A rejected payload "does not consume the request, so the caller can resubmit a corrected response" | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:857-901` |
| Workflow streaming custom-status published only on first execution | `if ctx.is_replaying: return` inside `publish_live_status` | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:755-757` |
| At-most-once dedup of pipeline policy | "the policy is registered at most once even when many FoundryChatClient instances share the same underlying chat client" | `dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:649-665` |
| Streaming is not supported for durable agents (proxy path) | `throw new NotSupportedException("Streaming is not supported for durable agents.")` | `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAIAgentProxy.cs:108` |
| Cancellation not supported in durable agents | `throw new NotSupportedException("Cancellation is not supported for durable agents.")` | `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAIAgent.cs:97` |
| Empty-message request ignored | `if (request.Messages.Count == 0) return new AgentResponse();` (entity short-circuit, not idempotency) | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:43-47` |
| Worker rejects duplicate agent registration | `raise ValueError(f"Agent '{registration_name}' is already registered")` | `python/packages/durabletask/agent_framework_durabletask/_worker.py:113-115` |
| `add_agent` skips if already registered | `logger.warning(... "skipping duplicate.")` | `python/packages/azurefunctions/agent_framework_azurefunctions/_app.py:572-576` |
| Workflow instance IDs are unique per scheduling | `client.schedule_new_orchestration(WORKFLOW_ORCHESTRATOR_NAME, instance_id=...)` | `python/packages/azurefunctions/agent_framework_azurefunctions/_app.py:361` |
| Empty-state transition on TTL expiry | `this.State = null!;` | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:183` |
| Workflow max-iterations guard | `iteration < workflow.max_iterations` raises `WorkflowConvergenceException` | `python/packages/durabletask/agent_framework_durabletask/_workflows/orchestrator.py:775, 908-909` |
| AG-UI golden-text dedup across workflow outputs | "Consecutive identical text outputs are deduplicated." | `python/packages/ag-ui/tests/ag_ui/golden/test_scenario_workflow.py:377, 409-427` |

## Answers to Dimension Questions

### 1. Can the same work run twice?

**Yes, in two distinct senses:**

- **At the framework/transport level**, the same work *can* be signalled twice. The
  HTTP/MCP/durable-agent client all use `signal_entity(...)` which is **at-least-once**
  by nature: a retry of the HTTP trigger or a re-sent `SignalEntity` call (e.g. after
  the worker crashed before acknowledging) re-runs the entity operation
  (`python/packages/durabletask/agent_framework_durabletask/_executors.py:300`,
  `python/packages/azurefunctions/agent_framework_azurefunctions/_app.py:739`). There is
  no idempotency key on the request path that prevents a second `run` invocation —
  the framework only correlates requests to responses *after* the agent has run.
- **At the Durable Task / orchestration level**, *completed* operations are not
  re-executed on replay — the orchestrator replays the code path but the result is
  read from the durable history. The docs state this explicitly:
  `docs/features/durable-agents/README.md:127` ("The orchestration framework replays
  orchestrator code on failure, so completed agent calls are not re-executed.") The
  Python orchestrator also guards publish-side-effect calls against replay
  (`_workflows/orchestrator.py:755-767`).

### 2. Is duplicate execution safe?

**Mixed.** It depends on which layer and which side effects:

- **Conversation-history appends are unsafe across identical re-runs** —
  the entity appends both a `DurableAgentStateRequest` and a
  `DurableAgentStateResponse` for every operation
  (`python/packages/durabletask/agent_framework_durabletask/_entities.py:151-172`),
  so two runs of the same `RunRequest` create two request entries and two
  response entries. The `correlationId` is identical on the second run only if
  the caller supplies it (it is generated by `RunRequest.__init__` /
  `RunRequest.correlation_id` if not supplied, so most callers do *not* supply
  one and therefore get fresh GUIDs per re-send). Tests prove this doubles the
  history: `test_durable_entities.py:354-370` increments `len(history)` by 2
  per call.
- **Tool invocations inside the agent are unsafe to re-run** unless the
  underlying tool is itself idempotent. The framework performs no
  deduplication of tool calls by content or arguments; replay during
  orchestration only skips *agent* side effects, not tool side effects within
  the agent invocation.
- **Reliable streaming is explicitly at-least-once and warns clients to
  handle duplicates** (`07_ReliableStreaming/README.md:160`,
  `08_ReliableStreaming/README.md:216`).
- **Workflow HITL responses** are sanitized (`strip_pickle_markers`) and a
  rejected payload explicitly does *not* consume the request, allowing
  resubmission
  (`_workflows/orchestrator.py:857-901`). This is a safe-retry model for HITL.

### 3. Which side effects are idempotent?

| Side effect | Idempotent? | Why |
|-------------|-------------|-----|
| `signal_entity(entity, "run", request)` to the durable agent | **No** by itself, but the entity serializes per-session; a second run of the same correlation ID overwrites the persisted response and history grows | `AgentEntity.cs:32-58`; `_entities.py:151-172` |
| `set_custom_status(...)` inside an orchestrator | **Yes** (effectively-once across replay) | guarded by `is_replaying` check |
| HITL `raise_event(instanceId, requestId, payload)` with the same `request_id` | **Yes at the orchestrator layer** (consumed once); but a malformed payload can be retried safely | `_workflows/orchestrator.py:857-901` |
| Redis `StreamAdd` of a chunk in reliable streaming | **No**, but each entry has a monotonically increasing `sequence` field that lets consumers detect / de-dup | `RedisStreamResponseHandler.cs:88-93` |
| MCP / AG-UI tool approval replay | **Yes at the application layer** — `test_approval_replay_is_blocked` proves a consumed approval cannot be re-played | `test_agent_wrapper_comprehensive.py:1031-1141` |
| Adding an agent with the same name to `AgentFunctionApp` | **No-op** (skipped) | `_app.py:572-576` |
| Adding an agent with the same name to `DurableAIAgentWorker` | **Raises** — strictest guarantee | `_worker.py:113-115` |
| Resetting conversation history | **Yes** — explicit `reset` operation, idempotent in itself | `python/.../azurefunctions/_entities.py:89-91` |
| TTL deletion signal (self-signal) | **Idempotent** — the entity re-arms if not expired and silently deletes if expired | `dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:166-195` |

### 4. Does the system claim exactly-once semantics?

**No.** Search results show:

- `dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:649` uses
  "at-most-once" *only* in the narrow sense of policy registration
  deduplication (not message delivery).
- The Reliable Streaming samples are explicit: "No exactly-once delivery"
  (`07_ReliableStreaming/README.md:160`,
  `08_ReliableStreaming/README.md:216`).
- The orchestration path achieves *effectively-once* for *orchestrator
  side-effects* via `is_replaying` but this is not framed as "exactly-once"
  semantics anywhere in the codebase.

The framework effectively delivers **at-least-once** at the entity-signal /
HTTP-trigger boundary and **effectively-once** for the orchestrator's own
bookkeeping (custom status, scheduling) thanks to the Durable Task replay
contract.

### 5. Are guarantees documented?

**Yes for the Durable Agents subsystem and the Reliable Streaming samples;
no for the rest of the framework.**

Documented guarantees and behaviors:

- `docs/features/durable-agents/README.md:127` — orchestrator replays but does
  not re-execute completed agent calls.
- `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAgentContext.cs:80-87` —
  scheduling + agent state update atomic in a single transaction.
- `dotnet/samples/04-hosting/DurableAgents/ConsoleApps/07_ReliableStreaming/README.md:152-160`
  and `08_ReliableStreaming/README.md:207-216` — explicit at-least-once
  contract and required client-side dedup.
- `dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAIAgent.cs:97`,
  `DurableAIAgentProxy.cs:108` — explicit `NotSupportedException` for
  cancellation/streaming on the durable path (i.e., the framework makes the
  non-support rather than offering best-effort).
- `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableExecutorDispatcher.cs:5-8`
  and `dotnet/src/Microsoft.Agents.AI.DurableTask/Workflows/DurableWorkflowRunner.cs:5-8`
  — comment blocks state the deterministic-replay requirement.
- `docs/decisions/0028-hosting-linking-multicast-enhancements.md:96-127`
  design-doc discusses idempotency, replay windows, dead-letter handling and
  structured logs for the *future* hosting-linking/multicast work; this is
  forward-looking and not yet implemented.
- `docs/decisions/0021-agent-skills-design.md:436-458` — design rationale for
  `DeduplicatingAgentSkillsSource` (case-insensitive, first-wins, warning
  on duplicates).

No general "Delivery Guarantees" or "Idempotency" doc exists at the top of
either `dotnet/` or `python/`.

## Architectural Decisions

1. **Correlation-id per request.** Every durable-agent call gets a
   `correlationId` generated by the framework
   (`dotnet/src/Microsoft.Agents.AI.DurableTask/RunRequest.cs:37`,
   `python/.../_models.py:115`) unless the caller supplies one. This is the
   single idempotency primitive the framework ships with, and it is used only
   for **response matching** during polling
   (`AgentRunHandle.cs:65-67`, `_executors.py:431`), not for de-duplicating
   upstream signals.

2. **Durable Entity per session = at-most-one concurrent execution.**
   `docs/features/durable-agents/README.md:28` ("Durable Entities serializes
   access to each entity instance, concurrent messages to the same session
   are processed one at a time"). This is the framework's strongest
   delivery guarantee and is implicit in the Durable Task runtime, not
   authored by this repo.

3. **Orchestrator replay = effectively-once for orchestrator side-effects.**
   Live-status publication is guarded by `is_replaying`
   (`_workflows/orchestrator.py:755-767`); the durable-history recording
   itself is replay-safe via Durable Task's `context.NewGuid()`,
   `context.current_utc_datetime` helpers
   (`context.py:39-93`).

4. **Reliable streaming: at-least-once with cursor-based resumption and a
   per-chunk sequence number.** Documented as "at-least-once delivery"
   (`07_ReliableStreaming/README.md:152`,
   `08_ReliableStreaming/README.md:207`); consumers are told to dedupe.

5. **Atomic state + side-effect for tool-issued orchestrations.** A tool
   calling `DurableAgentContext.ScheduleNewOrchestration` is queued inside
   the entity op so the state update and the scheduling commit in one
   transaction (`dotnet/src/Microsoft.Agents.AI.DurableTask/DurableAgentContext.cs:80-87`).
   This is the closest the framework comes to a transactional outbox.

6. **HITL reject-and-retry.** Sanitized payloads that fail pickle-marker
   checks do not consume the pending request, so the caller can resubmit
   (`_workflows/orchestrator.py:857-901`). This is a small but explicit
   safe-retry primitive.

7. **In-process agent path has no guarantees.** `AIAgent.RunCoreAsync` in
   the core C# and Python packages does no deduplication, has no idempotency
   store, and produces side-effects via function tools (e.g., tool calls in
   `python/packages/orchestrations/.../_handoff.py` are explicitly
   "Disable parallel tool calls to prevent the agent from invoking multiple
   handoff tools at once"). The contract is the developer-written tools'
   responsibility.

8. **TTL entity self-deletion is idempotent.** The agent entity schedules a
   self-signal; if it fires while the session is still active it
   reschedules, if it fires after expiry it sets `this.State = null!`
   (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:166-195`).
   This is a small but correct idempotent loop.

## Notable Patterns

- **Correlation-ID pairing of request and response in conversation history**
  (`State/DurableAgentStateEntry.cs:24`,
  `python/.../durable_agent_state.py:381-388`). Both languages persist
  correlation IDs on the same conversation-history entry model.
- **Reasoning-content stripping on replay** so a re-run does not feed
  in-process reasoning into the next chat turn
  (`_entities.py:195-208` + `test_durable_entities.py:398-444`). This is a
  deliberate replay-safety filter, not a dedup primitive.
- **Replay-aware side-effect guarding** via
  `WorkflowOrchestrationContext.is_replaying`
  (`_workflows/context.py:39-47`). Code paths that call
  `ctx.set_custom_status` outside the orchestrator's first execution are
  expected to gate on this property; the orchestrator itself does so
  (`orchestrator.py:755-757`).
- **Cursor-based stream resumption with monotonic sequence numbers**
  (`RedisStreamResponseHandler.cs:74-90`).
- **Defensive duplicate detection at builder time** in the orchestration
  DSL: both sequential and concurrent builders raise on duplicate executor
  IDs (`_sequential.py:137-149`, `_concurrent.py:249`). This is a build-time
  guarantee, not a delivery-time one.
- **Per-method-call message-id dedup in AG-UI** as a transport-level
  de-duplication layer
  (`python/packages/ag-ui/.../_message_adapters.py:1118-1240`,
  `python/packages/ag-ui/.../test_event_converters.py:38-91`).
- **At-most-once dedup of pipeline-policy registration** for the Foundry
  client (`FoundryChatClient.cs:649-665`). This is an interesting example of
  in-process pipeline idempotency but is unrelated to message delivery.

## Tradeoffs

- **Effectively-once for orchestration, at-least-once for messaging.** The
  framework relies on Durable Task's orchestrator-replay contract for the
  inside-orchestration path and on Durable Entities' serial access for the
  per-session path, but it exposes neither as "exactly-once" and there is no
  global idempotency store, so anything that crosses the entity-signal
  boundary (HTTP trigger, MCP tool trigger, durable workflow start) inherits
  the underlying transport's at-least-once delivery.
- **Correlation IDs are framework-generated and only used for response
  matching.** This is convenient for the poll-for-response model but means
  callers cannot trivially dedup at the signal layer unless they capture
  the generated `correlationId` and pass it back on retries themselves.
  The framework does not document this contract on the public API; a
  caller retrying an HTTP call after a network blip will get a *new*
  `correlationId` and *will* re-execute the agent run.
- **Reliable streaming gives the wrong-default impression.** It is called
  "Reliable" but the README is honest that it is at-least-once; consumers
  must dedupe. There is no built-in dedup at the reader side.
- **Conversation history grows unbounded across retries.** Because the
  request and response are both persisted unconditionally, a misbehaving
  client that retries 5 times will keep 5× the history. There is no TTL or
  request-correlation compaction on the request-history side, only on
  session-level TTL (`docs/features/durable-agents/durable-agents-ttl.md`).
- **Empty-message requests are ignored, not retried** (`AgentEntity.cs:43-47`).
  This is a documented short-circuit rather than idempotency, but it
  prevents accidental re-runs in a specific case.
- **Cancellation is explicitly unsupported** for durable agents
  (`DurableAIAgent.cs:97`). This removes a class of "stuck retry" bugs at
  the cost of flexibility.
- **HITL responses cannot be sent twice with the same `request_id`**; the
  sanitized payload either consumes the request or is rejected, so duplicate
  responses are safe
  (`_workflows/orchestrator.py:857-901`).
- **Different packages, different dedup strictness.** `AgentFunctionApp`
  silently skips duplicate `add_agent` (`_app.py:572-576`),
  `DurableAIAgentWorker` raises (`_worker.py:113-115`). This inconsistency
  is a developer-experience hazard.

## Failure Modes / Edge Cases

- **Worker crash between agent invocation and response persistence.** If a
  worker crashes after the agent has produced its response but before
  `State.Data.ConversationHistory.Add(DurableAgentStateResponse.FromResponse(...))`
  completes, the entity replays the operation and the agent re-runs. The
  HTTP client polling on the `correlationId` will block until the new
  response is written.
- **HTTP-client retry generating fresh `correlationId`.** A client retrying
  a 5xx from `AgentFunctionApp._get_response_from_entity` will get a new
  `correlationId` from `_build_request_data` (`_app.py:1164-1183`), so the
  agent runs again and history grows.
- **Duplicate `signal_entity` while the first is in flight.** Durable
  Entities serialize access per session, so the second `run` waits for
  the first to finish. There is no per-`correlationId` lock, so two
  re-sends with different correlation IDs will both run.
- **Replayed orchestrator re-runs a tool call.** If a tool call is wrapped
  in an orchestrator activity, replay does not re-run the activity — the
  result is read from history. But tool calls *inside* an agent invocation
  re-run on entity replay.
- **Redis consumer crash mid-stream.** The reliable-streaming design lets
  the client resume from the last cursor; duplicates can be observed
  (documented `07_ReliableStreaming/README.md:160`).
- **HITL response rejected by sanitizer.** Does not consume the request
  (good), but if the rejection happens during the orchestrator's
  `wait_for_external_event` block, a malicious or buggy client can keep
  submitting payloads forever
  (`_workflows/orchestrator.py:863-892`).
- **Schema-version mismatch resets conversation history** with a
  `logger.warning` but no surface to the caller
  (`_durable_agent_state.py:426-429`).
- **Empty-result exception swallowed.** The Python `AgentEntity.run` writes
  the exception into a `DurableAgentStateErrorContent` and persists it as
  a regular response, so it does not re-raise
  (`_entities.py:177-193`). Tests confirm
  (`test_durable_entities.py:520-583`). This is safe for the response
  matching model but means callers see errors as just another response
  with `is_error=True`.
- **Two workflow instances sharing an `instanceId`** are scoped by name via
  `_is_workflow_orchestration` in `_app.py:506-523`, preventing cross-
  instance HITL injection.
- **Max iterations reached** raises `WorkflowConvergenceException`
  (`_workflows/orchestrator.py:775, 908-909`), which will surface to
  clients as an orchestration failure — at-most-once delivery of the
  failure event.

## Future Considerations

1. **A first-class idempotency key on the durable-agent request path.**
   Allow `RunRequest` to accept a caller-supplied `IdempotencyKey` that the
   entity can use to short-circuit a duplicate run. The existing
   `correlationId` mechanism is the natural seed, but it is currently
   response-scoped, not request-dedup-scoped.
2. **Document the delivery-guarantees contract at the package level.** Both
   `dotnet/README.md` and `python/README.md` mention durable agents and
   reliable streaming but do not state the contract. A short `docs/
   delivery-guarantees.md` would lift the answer from "Read the
   samples" to "Read one paragraph."
3. **Implement the hosting-linking/multicast guarantees sketched in
   `docs/decisions/0028-hosting-linking-multicast-enhancements.md:96-127`**:
   channel-level idempotency keys, replay windows, dead-letter handling,
   structured logs. The design is in place; the implementation is not.
4. **Make `add_agent` registration consistent across hosts.** Today
   `AgentFunctionApp` silently skips, `DurableAIAgentWorker` raises
   (`_app.py:572-576` vs `_worker.py:113-115`). Pick one and document it.
5. **Reader-side dedup in reliable streaming.** The chunks already carry a
   monotonic `sequence` field (`RedisStreamResponseHandler.cs:88`); a
   dedup helper would close the documented at-least-once gap.
6. **TTL-aware history compaction for repeated retries.** If a
   `correlationId` was generated for a request that timed out and was
   retried, the original request+response could be compacted rather than
   kept forever, bounding history growth.
7. **Explicit `max_iterations` documentation for callers.** The default of
   100 iterations on the workflow runner (`DurableWorkflowRunner.cs:72`)
   and the corresponding `workflow.max_iterations` in Python are not
   documented in user-facing material.

## Questions / Gaps

- **What guarantees cover the in-process agent loop (no Durable Entity)?**
  No evidence found for idempotency, retry deduplication, or transactional
  boundaries when an `AIAgent` is hosted outside Azure Functions / Durable
  Task. Searched `python/packages/core` and `dotnet/src/Microsoft.Agents.AI`
  for `idempot`, `replay`, `deduplic`, `delivery` — no relevant matches.
- **What happens if a tool invocation returns an error during agent
  replay?** No evidence found for a tool-side idempotency hook or
  retry-with-backoff layer in the agent-entity path. The
  `AgentEntity.Run` method does not catch per-tool errors
  (`dotnet/src/Microsoft.Agents.AI.DurableTask/AgentEntity.cs:32-58`).
- **What is the canonical client retry policy for the HTTP trigger?** The
  framework documents `x-ms-wait-for-response: false` and
  `x-ms-thread-id` headers but does not specify HTTP-level idempotency
  semantics. Searched `_app.py`, `_constants.py`, `docs/specs/` — no
  evidence found for an HTTP-level idempotency-key recommendation.
- **Does the workflow orchestrator dedupe `task_all` results across
  replay?** No evidence found in `orchestrator.py` or the unit tests for
  deduplication of identical parallel task results within a single
  superstep; the orchestrator trusts Durable Task's replay contract.
- **What is the durability of the workflow custom status payload across
  replay?** The orchestrator's `set_custom_status` writes via
  `ctx.set_custom_status` (`orchestrator.py:767`) which is itself
  replayed by Durable Task, but no explicit transaction boundary is
  documented.
- **Are MCP tool trigger invocations at-least-once or exactly-once when
  an MCP client reconnects?** `dotnet/.../08_ReliableStreaming/` is the
  Redis-backed sample, but the plain `mcp_tool_trigger` flow in
  `_app.py:832-988` does not document retry semantics.
- **Cross-host behavior on `correlationId` collision:** if a caller
  supplies the same `correlationId` to two different sessions, the
  durable agent state schema allows it (correlation is per-session in
  practice because of session scoping) but there is no test for this
  edge case.

---

Generated by `01.09-delivery-guarantees-and-idempotency` against
`agent-framework`.
