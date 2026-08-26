# Source Analysis: agent-framework

## 09.03 Governance UX and Operator Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET monorepo; operator-facing surfaces are a React/TypeScript DevUI frontend with a FastAPI backend, plus protocol-level UI integrations (AG-UI, ChatKit) |
| Analyzed | 2026-08-26 |

## Summary

Agent Framework approaches governance UX in two layers. The first is a **developer/operator reference UI** ("DevUI", `python/packages/devui/`) that renders pending tool approvals as an interactive "Approval Required" banner with per-item Approve/Reject buttons (`python/packages/devui/frontend/src/components/features/agent/agent-view.tsx:2091-2155`), renders workflow human-in-the-loop requests as schema-driven inline forms on the execution timeline (`python/packages/devui/frontend/src/components/features/workflow/hil-timeline-item.tsx:27-153`), and offers a read-only checkpoint inspector for post-hoc review (`python/packages/devui/frontend/src/components/features/workflow/checkpoint-info-modal.tsx:110-223`). The second is **protocol-level governance machinery** that hosts build real operator surfaces on: a server-owned approval lifecycle with typed statuses and atomic batch operations (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-233`, `478-680`), session-backed standing approval rules (`python/packages/core/agent_framework/_harness/_tool_approval.py:86-343`), durable approval-request persistence in Foundry hosting (`python/packages/foundry_hosting/agent_framework_foundry_hosting/_state_store.py:204-253`), and workflow `request_info` pause/resume with structured status events (`python/packages/core/agent_framework/_workflows/_events.py:58-99`).

The core question — *can a human approve or block actions without reading code?* — is answered **yes** inside DevUI: tool name and JSON arguments are displayed verbatim next to Approve/Reject buttons, and HIL requests render form fields generated from a JSON schema with context and a "What's needed" description. However, DevUI is explicitly positioned as "a development-only sample app, not a production hosting surface" (`python/packages/devui/AGENTS.md`, Security Posture section). There is no cross-conversation or cross-agent governance dashboard: pending approvals live in per-chat Zustand state that is wiped on conversation/mode switches (`python/packages/devui/frontend/src/stores/devuiStore.ts:505-538`). No evidence-pack artifact is generated anywhere in the repo (searched `evidence pack|evidence_pack|EvidencePack` across python/, dotnet/src/, docs/ — zero hits); the nearest analogs are inspectable checkpoints and audit-retained approval control-plane contents.

## Rating

**6 / 10** — Present but scoped to developer tooling; the underlying lifecycle is mature and heavily tested, while the human-facing surface is single-conversation, non-durable, and bulk-action-free at the UI layer.

Rationale against the rubric:
- The approval model has explicit interfaces, typed states, operational safeguards (expiry, capacity, indeterminate protection), and 35 dedicated lifecycle tests (`python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:41-818`), which would earn 7–8 on machinery alone.
- What keeps the score at 6 is the *operator workflow*: no aggregate review queue across agents/conversations, approvals invisible in transcript history after resolution (`python/packages/devui/frontend/src/components/features/agent/agent-view.tsx:1097-1102`), no UI bulk approve/reject, no evidence packs, and a dev-only security posture for the only shipped UI.

## Evidence Collected

Every entry cites paths relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Approval prompt UI (dashboard-equivalent) | "Approval Required" amber banner lists each pending approval with function name + JSON args and individual Approve/Reject buttons | `python/packages/devui/frontend/src/components/features/agent/agent-view.tsx:2091-2155` |
| Pending-approval client state | `pendingApprovals: PendingApproval[]` store field, setter, reset to `[]` on mode/config switches (no persistence) | `python/packages/devui/frontend/src/stores/devuiStore.ts:67,157,374,505-538` |
| Approval event transport | Server maps approval request/response contents to `response.function_approval.requested`/`.responded` SSE events | `python/packages/devui/agent_framework_devui/_mapper.py:188-189,1778-1818` |
| Frontend approval handling | Stream events append/remove from `pendingApprovals`; `handleApproval(request_id, approved)` builds `function_approval_response` input and resubmits the conversation | `python/packages/devui/frontend/src/components/features/agent/agent-view.tsx:519-537,1036-1085` |
| Server-side anti-forgery validation | Executor tracks issued approval requests server-side and replaces client-supplied `function_call` data with stored data; consumed on use (no replay) | `python/packages/devui/agent_framework_devui/_executor.py:69,126-131,755` |
| Security tests for approvals | CWE-863 tests: forged request_id rejected, server data used, replay rejected, rejected approvals also validated | `python/packages/devui/tests/devui/test_approval_validation.py:109-199` |
| Standing rules (reduce prompt fatigue) | `create_always_approve_tool_response` records a rule with optional reason in metadata; rules match by name+server_label+canonicalized args | `python/packages/core/agent_framework/_harness/_tool_approval.py:218-245,278-321` |
| Review-queue primitive | `ToolApprovalState.rules / queued_approval_requests / collected_approval_responses` persisted in session state | `python/packages/core/agent_framework/_harness/_tool_approval.py:158-215` |
| Auto-approval safeguard | Constructor docstring warns auto-approval rules can bypass the boundary via name collisions with unrelated tools | `python/packages/core/agent_framework/_harness/_tool_approval.py:365-377` |
| Server-owned approval lifecycle | `ApprovalStatus` enum (PENDING→CLAIMED→EXECUTING→SETTLED/REJECTED/CANCELLED/EXPIRED/INDETERMINATE) with terminal/purgeable semantics | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-119` |
| Bulk operations (protocol level) | `claim_batch`, `resolve_batch`, `cancel_batch`, `expire_batch` atomically validate whole resume batches; duplicate interrupts rejected; lock-serialized per thread+interrupt set | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:34-62,478-491,583-607,609-657,668-681` |
| Batch decision result type | `ApprovalBatchDecision` returns authorized executions + retained outcomes + snapshot reconciliations | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:223-233` |
| Resume error surfacing | Typed resume contract codes `APPROVAL_RESUME_REQUIRED/INVALID/NOT_FOUND/MISSING_INTERRUPT` returned as `RunErrorEvent`s to clients | `python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:1278-1316` |
| Edited arguments support | `ResumeDecision(interrupt_id, accepted, arguments, original_arguments)` allows full-replacement argument edits at approval time | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:162-171` |
| Lifecycle capacity & retention | Configurable `max_entries=10_000`, pending/indeterminate/terminal retention windows enforced at registration | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:239-259` |
| Lifecycle tests | 35 tests: expiry releases capacity, indeterminate stays protected, typed conflict outcomes, telemetry without sensitive payloads | `python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:41-818` |
| Workflow HIL form UI | Orange-highlighted inline card: context key/values, "What's needed" description, `SchemaFormRenderer` inputs, validation-gated Submit button | `python/packages/devui/frontend/src/components/features/workflow/hil-timeline-item.tsx:42-153` |
| Workflow HIL batch submit | All pending HIL responses submitted in one request as `workflow_hil_response` content; submission blocked unless every form validates | `python/packages/devui/frontend/src/components/features/workflow/workflow-view.tsx:951-1000` |
| Multi-round HIL | New `response.request_info.requested` events during resume are collected and re-rendered | `python/packages/devui/frontend/src/components/features/workflow/workflow-view.tsx:1057-1080` |
| HIL wire contract | DevUI extension event `ResponseRequestInfoEvent(request_id, request_data, request_schema)`; docs state workflow enters `IDLE_WITH_PENDING_REQUESTS` | `python/packages/devui/agent_framework_devui/models/_openai_custom.py:144-170` |
| Backend HIL resume | Executor extracts `workflow_hil_response` from input and calls `send_responses` with per-request-id responses | `python/packages/devui/agent_framework_devui/_executor.py:483-532,932-983` |
| Exception/status visibility | `WorkflowRunState` includes `IDLE_WITH_PENDING_REQUESTS`; `WorkflowErrorDetails` carries error_type, message, traceback, executor_id | `python/packages/core/agent_framework/_workflows/_events.py:58-99` |
| Timeline error rendering | Failed runs show error string; failed output items propagate `item.error` into run state | `python/packages/devui/frontend/src/components/features/workflow/execution-timeline.tsx:188-194,345-353,440-442` |
| Evidence-pack analog: checkpoints | Read-only checkpoint timeline modal with per-checkpoint HIL-pending badges, sizes, timestamps, and full-state details pane | `python/packages/devui/frontend/src/components/features/workflow/checkpoint-info-modal.tsx:110-223` |
| Checkpoint backend exposure | Checkpoints surfaced as conversation items (`type="checkpoint"`), loadable and deletable via conversation-items API | `python/packages/devui/agent_framework_devui/_server.py:1140-1197` |
| Checkpoint storage abstraction | `CheckpointStorage` protocol (save/load/list/delete/get_latest) with InMemory and atomic-write File implementations | `python/packages/core/agent_framework/_workflows/_checkpoint.py:129-249,313-339` |
| Durable approval persistence (hosted) | `FunctionApprovalStore` Protocol + `FoundryFunctionApprovalStore` saving approval-request content platform-side with user isolation | `python/packages/foundry_hosting/agent_framework_foundry_hosting/_state_store.py:204-253` |
| Policy exceptions → approval | `PolicyEnforcementFunctionMiddleware(approval_on_violation=True)` converts policy violations into human approval requests instead of hard blocks | `python/packages/core/agent_framework/security.py:1642-1679` |
| Approval binding hardening | `_PendingPolicyApproval` binds approvals to body signature, label, session, and disclosed violation set to block replay under changed risk sets | `python/packages/core/agent_framework/security.py:1624-1639` |
| Audit retention of decisions | Function-calling loop spec: approval request/response are control-plane contents; history providers may retain them "for audit" | `docs/specs/004-python-function-calling-loop.md:391-409,285` |
| Non-UI approval flow (console) | Sample operator loop prompts "Approve shell command? (y/n)" and appends `to_function_approval_response(approved)` — same contract without any UI | `python/samples/02-agents/providers/anthropic/anthropic_with_shell.py:71-96` |

## Answers to Dimension Questions

**1. Can operators see what needs review?**
Partially, within one conversation. Pending tool approvals appear in a dedicated banner with full call details (`python/packages/devui/frontend/src/components/features/agent/agent-view.tsx:2091-2112`), and workflow HIL requests appear as highlighted cards on the execution timeline (`python/packages/devui/frontend/src/components/features/workflow/hil-timeline-item.tsx:48-75`) backed by `IDLE_WITH_PENDING_REQUESTS` status events (`python/packages/core/agent_framework/_workflows/_events.py:63-65`). But visibility ends at the chat scope: `pendingApprovals` resets when switching conversations or toggling modes (`python/packages/devui/frontend/src/stores/devuiStore.ts:505-538`), and there is no cross-agent/cross-conversation queue view anywhere in the frontend (searched all components for aggregate listing — none found). Checkpoints badge how many HIL requests a paused run holds (`python/packages/devui/frontend/src/components/features/workflow/checkpoint-info-modal.tsx:142,182-186`), which is the closest thing to a queue-depth indicator.

**2. Can they act on approvals efficiently?**
Mixed. Single actions are one click (`agent-view.tsx:2113-2148`). Efficiency mechanisms exist below the UI: standing "always approve" rules with recorded reasons eliminate repeat prompts (`python/packages/core/agent_framework/_harness/_tool_approval.py:218-245`), workflow HIL submits *all* pending forms in one request (`python/packages/devui/frontend/src/components/features/workflow/workflow-view.tsx:985-999`), and the AG-UI lifecycle processes complete resumes as atomically validated batches (`claim_batch`/`resolve_batch`, `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:478-607`). However, there is **no bulk Approve-all/Reject-all button** in the shipped UI (grep for `approveAll|Approve All` across the frontend: zero matches), each agent-chat approval must be clicked individually, and all-or-nothing validation means one invalid form blocks the entire HIL batch (`workflow-view.tsx:968-972`).

**3. Are exceptions surfaced?**
Yes, structurally. Workflow failures carry `WorkflowErrorDetails` with type, message, traceback, and originating executor id (`python/packages/core/agent_framework/_workflows/_events.py:70-99`), rendered into the execution timeline (`execution-timeline.tsx:188-194,440-442`). Policy violations become reviewable approval requests rather than silent blocks when `approval_on_violation=True` (`python/packages/core/agent_framework/security.py:1674-1679`; demonstrated in `python/samples/02-agents/security/email_security_example.py:310-315`). Protocol clients get typed resume errors instead of silent drops (`APPROVAL_RESUME_NOT_FOUND`, etc., `python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:1281-1316`). The policy middleware additionally maintains an audit log of violations (`security.py:1655,1678`).

**4. Is the governance UI usable under pressure?**
For a developer debugging one run, yes — the affordances are clear (color-coded banners, schema-driven forms, validation hints). For an operations role, no: the only shipped UI self-describes as development-only (`python/packages/devui/AGENTS.md`, Security Posture: unauthenticated mode restricted to localhost, token required beyond), resolved approvals vanish from the visible transcript because approval-response messages are explicitly filtered from chat display (`agent-view.tsx:1097-1102`), there is no history/trend view of past decisions, and nothing aggregates pending work across agents or sessions. Under pressure an operator would need host-built tooling on the AG-UI/workflow protocols rather than anything provided here.

## Architectural Decisions

1. **Governance UX is delegated to hosts via protocols; the framework ships primitives plus one reference dev UI.** The AG-UI package documents that `_approval_lifecycle.py` "is the sole owner of approval occurrence registration… Runner code … must not maintain a parallel pending-approval registry" (`python/packages/ag-ui/AGENTS.md`, Protocol Notes), making the lifecycle the single integration point for any real operator console.
2. **Server-authoritative approvals over client trust.** DevUI stores every issued approval request server-side and ignores client-supplied `function_call` payloads when validating responses (`python/packages/devui/agent_framework_devui/_executor.py:126-131,755`); AG-UI normalizes decisions through `_canonical_approval_decision` against retained original arguments before claiming authority (`_agent_run.py:1243-1287`). The UI cannot authorize anything the server did not issue.
3. **Approvals as message contents, not side-channel API.** Approval requests/responses are first-class `Content` types flowing through the conversation (`function_approval_request`/`function_approval_response` mapped at `python/packages/devui/agent_framework_devui/_mapper.py:188-189`), so any transport (DevUI, AG-UI interrupts, console samples like `anthropic_with_shell.py:88-96`) implements the same contract.
4. **Lifecycle states include failure semantics.** `INDETERMINATE` exists specifically for "an approval may have executed but has no retained terminal outcome" (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:77-79,103`), with a longer retention window (604_800s vs 900s terminal, lines 243-245) — governance survives crashes rather than silently re-executing.
5. **Standing rules encode operator intent durably.** "Always approve" responses persist as `ToolApprovalRule`s in session state, keyed by tool name + canonicalized arguments + hosted-server label (`python/packages/core/agent_framework/_harness/_tool_approval.py:86-155,289-321`), preventing same-named tools on different servers from inheriting trust.

## Notable Patterns

- **Schema-driven HIL forms**: the server attaches a JSON `request_schema` to each `request_info` event (`python/packages/devui/agent_framework_devui/models/_openai_custom.py:170`); the frontend generates inputs, defaults, and validation from it (`hil-timeline-item.tsx:40,122-127`; `workflow-view.tsx:1074-1080`).
- **Batch serialization decorators**: `_serialized_by_batch` locks every interrupt in a decision batch and emits `authority_failure` telemetry on conflicts (`_approval_lifecycle.py:34-62`) — concurrency-safe bulk governance.
- **Occurrence-aware identity**: approvals are identified by `(thread_id, occurrence_id, interrupt_id, call_id)` tuples (`_approval_lifecycle.py:137-144`) rather than raw `call_id`, defending against id reuse across rounds.
- **Read-only forensic views**: the checkpoint modal labels itself "a read only view of the current checkpoint ids" (`checkpoint-info-modal.tsx:121-123`) — review without mutation risk; deletion is exposed separately through the API (`_server.py:1165-1185`).
- **Test-as-specification for governance**: the function-calling loop spec maps named scenarios to named tests, e.g., queued approvals and hosted-approval replay rows (`docs/specs/004-python-function-calling-loop.md:474,499,542`).

## Tradeoffs

- **Protocol generality vs. shipped usability**: the batch/atomic lifecycle is excellent infrastructure, but operators only get its benefits if a host builds UI for it; DevUI exposes none of the batch endpoints.
- **Security posture vs. operability**: strict consumption-on-use and server-side binding prevent replay attacks (`test_approval_validation.py:160-177`) but mean a stale browser tab holding a pending approval silently fails on submit with a typed error, requiring the operator to rediscover current state.
- **All-or-nothing HIL batching** simplifies resume semantics but couples unrelated decisions: one malformed response blocks submitting valid ones (`workflow-view.tsx:968-972`).
- **Auto-approval convenience vs. boundary integrity**: heuristic `auto_approval_rules` trade prompt fatigue for documented collision risk — a rule designed for one feature can auto-approve an unrelated same-named local tool (`_tool_approval.py:365-377`).

## Failure Modes / Edge Cases

- **Indeterminate executions**: if execution fails after claim, the occurrence becomes `INDETERMINATE` and protected from purge while siblings stay claimed (`test_approval_lifecycle.py:719-772`); operators must understand this state, and DevUI has no rendering for it.
- **Capacity exhaustion**: protected occurrences can exhaust `max_entries=10_000` raising `ApprovalCapacityError` (`_approval_lifecycle.py:81-83,151-172,248-249`) — a fleet-scale operator would need alerting on this; nothing surfaces it in UI.
- **Expired pending approvals**: abandoned pendings expire and release capacity after `pending_retention_seconds` (default 24h, `_approval_lifecycle.py:176-212,242`); the DevUI banner does not show age/expiry, so an operator may approve a long-dead request and receive `APPROVAL_RESUME_NOT_FOUND`.
- **Cross-conversation amnesia**: switching conversations mid-review drops `pendingApprovals` from the store (`devuiStore.ts:505-538`); returning later shows no reminder that an approval is still outstanding server-side.
- **Invisible decision history**: because approval-response messages are hidden from the transcript (`agent-view.tsx:1097-1102`), reconstructing *who approved what* after the fact depends entirely on history-provider retention ("may retain … for audit", `docs/specs/004-python-function-calling-loop.md:409`) — not guaranteed.

## Future Considerations

- Add a cross-conversation governance dashboard to DevUI (or ship a minimal operator service over `ApprovalLifecycle` query methods): pending/approved/rejected listings with age, thread, tool, and args.
- Expose bulk actions in the UI: the backend already supports it (`resolve_batch` accepts arbitrary decision batches, `_approval_lifecycle.py:583-607`); an "Approve all shown" control plus per-item override would close the efficiency gap safely.
- Render approval outcomes (including `INDETERMINATE`/`EXPIRED`) in the transcript instead of hiding them, giving operators a decision trail without relying on host audit stores.
- Generate review artifacts: bundle the approval request payload, matched rule (if any), decision, timestamp, and resulting tool outcome into a checkpoint-anchored record — the checkpoint plumbing (`_checkpoint.py:31-111`) already persists everything needed.
- Show retention countdowns/badges on pending approvals so operators can triage before expiry.

## Questions / Gaps

- **Evidence packs**: no implementation found. Searched `evidence pack|evidence_pack|EvidencePack` across `python/`, `dotnet/src/`, and `docs/` (Python/TS/C#/MD files) — zero matches. Nearest implemented analogs: read-only checkpoint inspector (`checkpoint-info-modal.tsx`) and audit-optional history retention (`docs/specs/004-python-function-calling-loop.md:391-409`).
- **Production operator dashboard**: none in-repo. The .NET `Microsoft.Agents.AI.DevUI` project hosts the same style of development surface (`dotnet/src/Microsoft.Agents.AI.DevUI/README.md`); no Aspire/dashboard code reviewed claims production approval management. If such a capability exists, it lives outside this repository (e.g., Foundry portal), which could not be verified here.
- **Decision audit trail**: whether any shipped history provider *actually* retains approval control-plane contents (vs. "may retain") was not verified end-to-end; the base `HistoryProvider` filters resolved wrappers from model replay per the loop spec, and I did not trace each provider's backing-store behavior.
- **Go stack**: `go/` contains only a README (`go/README.md`); no Go governance surfaces exist to analyze.

---

Generated by `09.03-governance-ux-and-operator-workflow` against `agent-framework`.
