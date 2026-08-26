# Source Analysis: agent-framework

## Human Intervention and Takeover (Dimension 14.03)

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET monorepo (Microsoft Agent Framework); Go is README-only stub |
| Analyzed | 2026-08-26 |

**Citation convention:** all paths below are relative to `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Agent Framework implements human intervention as a first-class, protocol-shaped concern rather than an ad-hoc callback. The core mechanism is a **typed request/response interrupt** that appears at four layers with a consistent shape:

1. **Workflow layer**: executors call `ctx.request_info(...)` (`python/packages/core/agent_framework/_workflows/_workflow_context.py:403-434`), which emits a `request_info` event carrying a UUID `request_id`; humans answer via `Workflow.run(responses={request_id: data}, checkpoint_id=...)` (`python/packages/core/agent_framework/_workflows/_workflow.py:709-746`) or, in .NET, via `RequestInfoEvent` → `request.CreateResponse(data)` → `handle.SendResponseAsync(response)` (`dotnet/samples/03-workflows/HumanInTheLoop/HumanInTheLoopBasic/Program.cs:35-39`, `dotnet/src/Microsoft.Agents.AI.Workflows/StreamingRun.cs:44`). The run halts in `IDLE_WITH_PENDING_REQUESTS` state until answered (`python/packages/core/agent_framework/_workflows/_workflow.py:254,590`; `python/packages/core/agent_framework/_workflows/_events.py:65`).
2. **Agent tool-approval layer**: tools declare `approval_mode="always_require"` (`python/packages/core/agent_framework/_tools.py:316-331,408`); the run returns `function_approval_request` contents in `result.user_input_requests` instead of executing; the human replies per request with `request.to_function_approval_response(approved=True|False)` sent as a normal run input over the preserved session (`python/packages/core/agent_framework/_types.py:1273-1310,1346-1355`; demonstrated in `python/samples/02-agents/skills/script_approval/script_approval.py:98-115`). .NET mirrors this with `ToolApprovalAgent` and `ToolApprovalRequestContentExtensions.CreateAlwaysApproveToolResponse` standing rules (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:52-107`, `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalRequestContentExtensions.cs:27-61`).
3. **Orchestration layer**: Magentic exposes human plan sign-off (`enable_plan_review`) built directly on `ctx.request_info` — approve, or reply with revision comments that trigger replanning (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:955-958,993-1055,1425-1465`). Group chat offers `.with_request_info()` to pause after every agent round for caller guidance (`python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:884-911`).
4. **UI/transport layer**: AG-UI models approvals as interrupts with server-owned authority lifecycle (`register → claim → execute → settle/reject/cancel/expire`, `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-114,236-266,479-581`), including `editedArgs` full-replacement argument editing at approval time; DevUI renders form-based HITL via a custom `response.request_info.requested` event and a `workflow_hil_response` content type for answers (`python/packages/devui/agent_framework_devui/models/_openai_custom.py:144-186`).

**Resume/fork** is provided by durable checkpoints taken per super-step, which persist pending requests so a workflow can be stopped, restarted in a new process, and re-emitted for answering (`python/packages/core/agent_framework/_workflows/_checkpoint.py:30-98`; sample `python/samples/03-workflows/checkpoint/checkpoint_with_human_in_the_loop.py:227-335`). Restoring *any* checkpoint into *any* instance of the same graph definition yields time-travel/fork semantics.

What is comparatively weaker: there is no dedicated API for a human to *edit* conversation/workflow state in place — state correction is indirect (answer requests, inject messages, serialize-edit-deserialize sessions, or restore an earlier checkpoint), and checkpoint files have no integrity/tamper protection.

## Rating

**8 / 10**

Rationale against the rubric:
- **Clear model with explicit interfaces**: one typed request/response primitive reused across workflows, orchestrators, agents-in-workflows, and UI transports; response handlers are declared with `@response_handler` and type-checked at registration (`python/packages/core/agent_framework/_workflows/_request_info_mixin.py:133-298`); .NET uses typed `RequestPort<TRequest,TResponse>` records (`dotnet/src/Microsoft.Agents.AI.Workflows/RequestPort.cs:13-33`).
- **Tests**: dedicated suites cover approval/denied-approval flows (`python/packages/core/tests/workflow/test_request_info_and_response.py:177,263`), checkpoint round-trip of pending requests (`python/packages/core/tests/workflow/test_checkpoint.py:883`), lineage after resume (`python/packages/core/tests/workflow/test_checkpoint.py:285,339`), and ~23 scenarios for approval middleware including forged standing-rule rejection (`python/packages/core/tests/core/test_harness_tool_approval.py:45-1368`).
- **Operational safeguards**: concurrency guard rejects overlapping runs on one workflow instance (`_workflow.py:759-771`); path-traversal guard on checkpoint IDs (`_checkpoint.py:293-311`); atomic checkpoint writes (`_checkpoint.py:328-334`); deserialization allowlist for checkpoint payloads (`_checkpoint.py:257-290`); warn-only detection of fresh input while requests are pending (`_workflow.py:856-872`).
- **Why not 9-10**: human *state editing* is ad-hoc (no audited transcript-edit API); checkpoint files lack tamper evidence (JSON + base64 pickle, no HMAC/signature); some safeguards are warnings rather than enforcement; audit trails are spread across mechanisms rather than unified.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| State edit APIs (workflow) | `ctx.set_state/get_state` shared-state mutation is executor-owned; humans reach it only indirectly through responses/checkpoints | python/packages/core/agent_framework/_workflows/_workflow_context.py:436-442 |
| State edit APIs (conversation) | `AgentSession.state` is a public mutable dict; `to_dict()`/`from_dict()` enable offline suspend/edit/resume; `SessionStore`/`FileSessionStore` persist snapshots | python/packages/core/agent_framework/_sessions.py:1717-1794,1795-1872 |
| State edit APIs (.NET session) | `AIAgent.SerializeSessionAsync` / `DeserializeSessionAsync(JsonElement)` round-trip sessions with documented security caveats | dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSession.cs:32-57 |
| Feedback injection mid-run (in-process) | `MessageInjectionMiddleware.enqueue_messages(session, ...)` queues messages into `session.state`, drained into the next model call even while a run is in progress (also callable from tool code) | python/packages/core/agent_framework/_sessions.py:1383-1433 |
| Feedback injection mid-run (streaming .NET) | `StreamingRun.TrySendMessageAsync<TMessage>` enqueues messages into a live run; `Run.ResumeAsync<T>(..., params messages)` feeds inputs to the start executor between halts | dotnet/src/Microsoft.Agents.AI.Workflows/StreamingRun.cs:57; dotnet/src/Microsoft.Agents.AI.Workflows/Run.cs:104-128 |
| Request/response interrupt (Python) | `ctx.request_info(request_data, response_type, request_id=...)` emits `WorkflowEvent.request_info`; answered by `run(responses={id: data})`; unanswered requests survive across runs | python/packages/core/agent_framework/_workflows/_workflow_context.py:403-434; _workflow.py:709-746 |
| Response handler contract | `@response_handler` decorator validates `(self, original_request, response, ctx)` signature and registers typed handlers; duplicate handler rejection | python/packages/core/agent_framework/_workflows/_request_info_mixin.py:91-97,133-298,306-366 |
| Halt semantics | Run ends with status `IDLE_WITH_PENDING_REQUESTS`; pending events intentionally not blocked when starting follow-up runs | python/packages/core/agent_framework/_workflows/_events.py:65; _workflow.py:832-845 |
| Pending-request safety | Warning logged when fresh message / checkpoint restore begins while requests are pending (response may apply to moved-on state) | python/packages/core/agent_framework/_workflows/_workflow.py:856-872 |
| Tool approval gate | `@tool(approval_mode="always_require")`; batch rule: if any call requires approval, all are gated; run returns `user_input_requests` | python/packages/core/agent_framework/_tools.py:1212-1232,1314; _agents.py:711-712 |
| Approval content types | `Content.from_function_approval_request/response`, `to_function_approval_response(approved=bool)` conversion | python/packages/core/agent_framework/_types.py:373-374,1273-1310,1346-1355 |
| Approval resume integrity | Resume normalizes a private copy; terminal results returned before any assistant message; occurrence-aware correlation for reused `call_id`s; forged standing approvals dropped | python/packages/core/tests/core/test_harness_tool_approval.py:45-108,708-745 |
| Standing approval rules | `ToolApprovalRule`/`ToolApprovalState` persisted in session state; hosted-server boundary prevents cross-server aliasing | python/packages/core/agent_framework/_harness/_tool_approval.py:86-156,248-276,1224 |
| Policy-violation escalation | `PolicyEnforcementFunctionMiddleware.approval_on_violation` converts a security block into a user-approval request; per-call binding record blocks replay misuse; in-memory `audit_log` of violations | python/packages/core/agent_framework/security.py:1650-1712 |
| Orchestrator plan review (human takeover of planning) | `MagenticPlanReviewRequest/Response`; empty review = approve, comments = replan loop; also used on stall detection | python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:993-1055,1186 |
| Group-chat review pause | `GroupChatBuilder.with_request_info(agents=[...])` pauses after each participant turn for caller guidance | python/packages/orchestrations/agent_framework_orchestrations/_group_chat.py:884-911 |
| Agent-as-node approval bubbling | `AIAgentUnservicedRequestsCollector` routes agent `ToolApprovalRequestContent` out of the workflow as external requests when not handled inline | dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/AIAgentUnservicedRequestsCollector.cs:12-55 |
| Workflow→agent surface | `WorkflowAgent._process_request_info_event` converts request_info events into caller-facing contents (specialized user-input requests preserved; generic ones wrapped in a function-call envelope answerable with a function result) | python/packages/core/agent_framework/_workflows/_agent.py:684-731,733-739 |
| Checkpoint/resume (fork) | `WorkflowCheckpoint` captures messages, state, `pending_request_info_events`, iteration count; instance-independent (keyed to workflow name + `graph_signature_hash`), enabling restore into a different process/instance | python/packages/core/agent_framework/_workflows/_checkpoint.py:30-98 |
| Checkpoint storage backends | Protocol with save/load/list/delete/get_latest; `InMemoryCheckpointStorage`; `FileCheckpointStorage` with atomic writes, ID path-traversal guard, decode allowlist via `register_checkpoint_type` | python/packages/core/agent_framework/_workflows/_checkpoint.py:129-246,249-461 |
| Cross-process HITL demo | Sample exits while awaiting approval, restarts, lists checkpoints, restores chosen checkpoint, re-emits pending request, collects answer | python/samples/03-workflows/checkpoint/checkpoint_with_human_in_the_loop.py:209-335 |
| Live-run time travel (.NET) | `CheckpointableRunBase.RestoreCheckpointAsync(checkpointInfo)` rewinds a still-open run; `InProcessExecution.ResumeAsync(workflow, fromCheckpoint, manager)` resumes from any stored checkpoint | dotnet/src/Microsoft.Agents.AI.Workflows/CheckpointableRunBase.cs:46-48; dotnet/src/Microsoft.Agents.AI.Workflows/InProcessExecution.cs:61-73 |
| .NET external-request plumbing | `RequestPort` record; `ExternalRequest.CreateResponse(data)` builds correlated `ExternalResponse(RequestPortInfo, RequestId, Data)`; `RequestHaltEvent`/`RequestInfoEvent` surfaced on run streams | dotnet/src/Microsoft.Agents.AI.Workflows/RequestPort.cs:13-33; ExternalRequest.cs:16-100; ExternalResponse.cs:15-40 |
| UI-mediated takeover (AG-UI) | Server-owned `ApprovalLifecycle`: statuses pending/claimed/executing/settled/rejected/cancelled/expired/indeterminate; `claim_batch` validates complete decision sets atomically; idempotent duplicate-decision replay retains outcomes; capacity + retention windows | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-119,137-232,236-266,469-581,668-693 |
| Argument editing at approval | AG-UI approvals advertise standard `approved` + full-replacement `editedArgs` (alias `accepted`) responses; cancelled resumes complete without executing | python/packages/ag-ui/AGENTS.md (protocol notes); python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:162-170 |
| DevUI forms | `ResponseRequestInfoEvent` carries `request_schema`/`response_schema`; clients submit answers as new requests with `workflow_hil_response` content type | python/packages/devui/agent_framework_devui/models/_openai_custom.py:144-186 |
| Audit trails | Checkpoint lineage chain (`previous_checkpoint_id`) verified by tests; .NET `Run` keeps full `OutgoingEvents` sink; AG-UI lifecycle emits structured transition logs (occurrence/status/owner); history providers retain control-plane approval contents for audit while filtering them from model replay | python/packages/core/tests/workflow/test_checkpoint.py:285-339; dotnet/src/Microsoft.Agents.AI.Workflows/Run.cs:52-79; python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:762-778; python/packages/core/AGENTS.md (approval/history section) |

## Answers to Dimension Questions

### 1. Can humans edit agent state?

Partially, and only through indirect mechanisms — there is no first-class "edit transcript/state" API.
- Conversation state: `AgentSession.state` is public and mutable, and `AgentSession.to_dict()/from_dict()` (`python/packages/core/agent_framework/_sessions.py:1750-1794`) let a host suspend a session, alter the serialized snapshot, and resume it (pattern shown in `python/samples/02-agents/conversations/suspend_resume_session.py:51-56`). This is application-level surgery, untracked by the framework. The same applies to .NET's `SerializeSessionAsync/DeserializeSessionAsync` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSession.cs:41-42`).
- Workflow state: shared state is mutated by executors via `ctx.set_state` (`_workflow_context.py:436-442`); a human changes it by answering a request whose handler writes state, or by restoring an earlier checkpoint. Checkpoint files themselves are JSON-on-disk and technically editable, but nothing tracks such edits (see Gaps).
- Structured argument editing exists at exactly one point: AG-UI approval decisions accept `editedArgs` full-replacement arguments validated against the registered occurrence before execution (`_approval_lifecycle.py:524-531,561-563`).
- The intended correction channel is feedback, not edits: e.g., the reviewer gateway turns free-text guidance into a revision prompt (`python/samples/03-workflows/checkpoint/checkpoint_with_human_in_the_loop.py:139-163`).

### 2. Can humans provide mid-run feedback?

Yes, via three distinct points:
- **Live message injection**: `MessageInjectionMiddleware.enqueue_messages()` can be called "while a run is in progress, including from tool code"; queued messages drain into the next model call and the middleware loops internally when more arrive (`python/packages/core/agent_framework/_sessions.py:1383-1433`). Wired by default into harness agents.
- **Halting request/response**: `request_info` events pause execution until `responses=` arrive; multiple concurrent requests are supported and individually correlatable (`test_request_info_and_response.py:229` covers multiple requests). In .NET, `SendResponseAsync` on a live `StreamingRun` answers mid-stream (`dotnet/src/Microsoft.Agents.AI.Workflows/StreamingRun.cs:44`).
- **Orchestration guidance**: group chat `.with_request_info()` inserts a review beat after each agent round (`_group_chat.py:884-911`); Magentic plan review accepts revision comments that trigger replanning mid-run (`_magentic.py:1021-1038`).

### 3. Can humans take over execution?

Yes — takeover is expressed as answering typed external requests, plus rewind:
- Full decision authority over side-effecting tools via the approval gate (approve/reject per call; rejection produces an explicit `"Error: Tool call invocation was rejected by user."` result — `_approval_lifecycle.py:545-548`; denied flows tested at `test_request_info_and_response.py:263`).
- Takeover of planning: approve or rewrite Magentic plans via review comments (`_magentic.py:1002-1038`).
- Takeover of a paused run across processes: exit while awaiting approval, then restart, restore the checkpoint, and answer (sample flow above).
- Time-travel takeover: restore any prior checkpoint onto a live .NET run (`CheckpointableRunBase.cs:46-48`) or a fresh Python instance (`checkpoint_with_human_in_the_loop.py:331-335`).
- No sandbox-style console/REPL takeover exists: there is no mechanism to attach to and drive a running executor interactively; intervention always crosses the defined request/response or injection boundaries. (Sandbox concepts like shell tools remain subject to the same approval gates.)

### 4. Are human interventions traceable?

Mostly yes, distributed across several ledgers:
- Every request carries a stable UUID `request_id` (`_workflow_context.py:428-434`) and responses are keyed by it; correlation survives checkpointing because pending events are part of the checkpoint payload (`_checkpoint.py:59-61,91`).
- Approval decisions become ordinary message contents (`function_approval_request`/`function_approval_response`) retained in history providers for audit while being filtered from later model replay (documented behavioral contract in `python/packages/core/AGENTS.md`, tool-approval section; enforced by `test_harness_tool_approval.py:222`).
- Standing rules ("always approve") are persisted in session state with scope metadata (`_tool_approval.py:86-156,248-276`).
- AG-UI logs structured lifecycle transitions (registration/claim/settlement/rejection/expiry with occurrence ids and owners, `_approval_lifecycle.py:762-778`).
- Checkpoints form a lineage chain via `previous_checkpoint_id` (`_checkpoint.py:51-52`; ancestry test `test_checkpoint.py:339`), and .NET runs retain all events in `OutgoingEvents` (`Run.cs:52`).
- Gap: interventions are *reconstructable* rather than *reported* — there is no unified "who did what" audit log tying a human identity to a decision. `PolicyEnforcementFunctionMiddleware.audit_log` (`security.py:1697-1698`) is the only purpose-built audit list, and it is in-memory and policy-scoped.

## Architectural Decisions

1. **One interrupt primitive, many surfaces.** `request_info`/`ExternalRequest` is the single halting mechanism; tool approval, plan review, group-chat guidance, agent-in-workflow escalation, and DevUI forms all compile down to it. This keeps halt/resume semantics uniform (e.g., `AIAgentUnservicedRequestsCollector.cs:12-29` converts agent approval content into workflow external requests).
2. **Halt-and-resume over blocking callbacks.** Runs terminate into `IDLE_WITH_PENDING_REQUESTS` instead of holding threads waiting on humans; continuation is a new `run(responses=...)` call (`_workflow.py:832-845`). This makes human latency unbounded-safe and enables cross-process resumes.
3. **Checkpoints as the durability substrate for HITL.** Pending requests, executor state (`on_checkpoint_save/on_checkpoint_restore` hooks — `checkpoint_with_human_in_the_loop.py:165-173`), and messages are captured per super-step, so a human can take arbitrarily long to answer, including across machine restarts.
4. **Typed, validated boundaries.** Response handlers are signature-validated and type-matched (`_request_info_mixin.py:306-366`); .NET `RequestPort<TRequest,TResponse>` encodes the contract in the type system; AG-UI decisions are validated against registered name/arguments before claiming authority (`_approval_lifecycle.py:493-531`).
5. **Approvals are control-plane content in the transcript.** Requests/responses live in the conversation as `Content` types, which lets history providers persist them for audit while keeping them out of model replay — a deliberate separation of audit vs. context.
6. **Server-owned authority in hosted/UI scenarios.** The AG-UI package explicitly concentrates approval ownership in `ApprovalLifecycle` ("sole owner of approval occurrence registration … runner code must not maintain a parallel pending-approval registry", `python/packages/ag-ui/AGENTS.md`), with idempotency keys and indeterminate states to prevent double-execution of side effects.

## Notable Patterns

- **Escalation gateway executor**: a small executor whose sole job is `ctx.request_info` on agent output and routing the human answer onward (`ReviewGateway`, `checkpoint_with_human_in_the_loop.py:116-173`).
- **Mixed-batch handling**: when a model issues approval-required and non-approved calls together, already-approved calls are hidden from output, stored against visible request ids, and reinjected on resume (`test_harness_tool_approval.py:418-648`).
- **Occurrence-aware correlation**: `call_id` reuse after completion is handled by matching ordered occurrences, not global ids (core AGENTS.md approval section; exercised by AG-UI aliases `_approval_lifecycle.py:322-355`).
- **Warn-don't-block liveness**: stale-pending-request situations produce warnings (`_workflow.py:856-872`) rather than deadlocks, accepting eventual-consistency risk in exchange for responsiveness.
- **Frontend form generation**: DevUI ships JSON schemas for both request and expected response so generic clients can render arbitrary HITL prompts (`_openai_custom.py:170-174`).

## Tradeoffs

- **Durability vs. simplicity**: file checkpoints use JSON + base64-encoded pickle for rich Python objects (`_checkpoint.py:249-261`). This preserves fidelity but means payloads are not human-diffable end-to-end and require a decode allowlist (`allowed_checkpoint_types`) to stay safe.
- **Warn-only safeguards**: starting a fresh run while requests are pending advances state anyway; correctness depends on callers honoring the warning (`_workflow.py:847-872`).
- **No blocking wait**: the halt-and-resume model pushes a state-machine style onto applications (loop: run → collect requests → collect answers → re-run, as in `run_interactive_session`, `checkpoint_with_human_in_the_loop.py:227-273`). Simpler for servers, noisier for scripts. .NET's open `StreamingRun` mitigates this within one process.
- **Indirect state correction**: because there is no audited edit API, hosts wanting "human edits the draft" behavior implement it as feedback loops or snapshot surgery — flexible but non-uniform.
- **Two-language parity drift**: identical concepts exist in both stacks (e.g., Python `FileCheckpointStorage` vs. .NET `CheckpointManager.CreateJson/CreateInMemory`, `dotnet/src/Microsoft.Agents.AI.Workflows/CheckpointManager.cs:34-48`), doubling the surface where behaviors can diverge.

## Failure Modes / Edge Cases

- **Stale answers**: a late response to a superseded request may be applied to a workflow that has moved on — explicitly warned about (`_workflow.py:858-872`).
- **Checkpoint tampering**: checkpoint files lack signatures/HMACs; the graph-signature hash only validates topology compatibility on restore (`_checkpoint.py:48-49`), not content authenticity. A modified state blob will be restored as-is.
- **Duplicate/out-of-order responses**: AG-UI treats repeated decisions idempotently if they match the retained terminal decision and replays the retained outcome; conflicting decisions raise (`_approval_lifecycle.py:506-520`).
- **Indeterminate executions**: crashes between `begin_execution` and settlement mark the occurrence INDETERMINATE and refuse automatic retry unless an idempotency key proves safety (`_approval_lifecycle.py:808-847`).
- **Pending-request abandonment**: pending requests expire after retention windows in AG-UI (default 24h, `_approval_lifecycle.py:239-246`); workflow-level pending events instead linger indefinitely until answered or overwritten by a checkpoint restore.
- **Concurrent runs**: a second `run()` on the same Python workflow instance is rejected synchronously via weakref lock (`_workflow.py:759-771`); dropped streams release the lock on GC.
- **Forged authority**: standing-approval metadata presented by the client is dropped rather than honored (`test_harness_tool_approval.py:708`).

## Future Considerations

- Add an authenticated, append-only intervention ledger (who approved what, when, with which argument revisions) aggregating the currently scattered traces.
- Integrity-protect checkpoint payloads (checksum/HMAC) so offline editing is detectable, complementing the existing topology hash.
- Promote "human edits state" to a supported operation (e.g., a validated state-patch request type) instead of serialize/edit/deserialize conventions.
- Consider escalating repeated pending-request warnings to configurable enforcement (reject vs. quarantine stale requests).

## Questions / Gaps

- No evidence found of a framework-level API for editing persisted conversation transcripts with attribution; search covered `_sessions.py`, `_middleware.py`, samples under `python/samples/02-agents/conversations/`, and .NET `Microsoft.Agents.AI.Abstractions` session types. Editing remains host-application territory.
- No first-class named "fork" API found; fork semantics are inferred from restore-any-checkpoint-into-new-instance behavior (`_checkpoint.py:37-41` docs + samples). If true fork (branching both lineages concurrently) is intended, it is undocumented in code.
- No evidence found of sandbox-console takeover (attaching to a running executor); searched for interactive/attach patterns in `devui`, `hosting*`, and workflow runtime packages.
- Human identity/attribution is absent from the Python approval path (requests/responses carry ids, not users); only AG-UI thread-scoped occurrences provide scoping. Whether production deployments add identity above the framework could not be determined from this source alone.

---

Generated by Dimension 14.03 (Human Intervention and Takeover) against `agent-framework`.
