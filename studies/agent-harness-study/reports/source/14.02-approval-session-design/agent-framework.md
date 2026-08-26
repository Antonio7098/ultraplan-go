# Source Analysis: agent-framework

## Dimension 14.02: Approval Session Design

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, multi-package monorepo), C#/.NET (parallel implementation), Go (README only) |
| Analyzed | 2026-08-26 |

> **Citation convention:** all paths below are relative to the selected source root `studies/agent-harness-study/sources/agent-framework/`. The framework ships two full implementations (Python and .NET); this study covers both, with Python as the primary lens.

## Summary

agent-framework implements approval as a typed control-plane protocol (`function_approval_request` / `function_approval_response` message contents) layered over three cooperating surfaces:

1. **Tool gate + function loop** — tools declare an `approval_mode`; the auto-invocation loop pauses a mixed batch, emits immutable approval-request snapshots keyed by request id, persists them in `AgentSession.state`, and binds inbound responses to surfaced requests before execution.
2. **Harness middleware** — `ToolApprovalMiddleware` adds session-backed standing rules ("always approve tool" / "always approve tool with exact arguments"), queued multi-request prompts, and pluggable auto-approval callbacks on top of the same session state.
3. **Workflow HITL** — approvals surface as workflow `request_info` events; pending events are captured in workflow checkpoints and survive process restarts (proven by dedicated rehydrate tests).

Durability is host-selectable: in-memory/file session stores and in-memory/file checkpoint storages exist in core, Foundry hosting adds a server-side durable `FunctionApprovalStore`, and AG-UI ships a purpose-built in-process approval lifecycle engine with statuses, claims, retention windows, expiry, idempotency keys, and structured telemetry. .NET mirrors the harness design (`ToolApprovalAgent` / `ToolApprovalState`) with identical scoping semantics.

The model is explicit, heavily tested, and hardened against forged/replayed responses. Its main gaps: core approvals have no timeout/expiry (they wait indefinitely), audit is log- and history-retention-based rather than a queryable durable trail, and several default stores are process-local, so durability depends on which optional store the host wires.

## Rating

**Score: 8/10** — Clear model with extensive tests, explicit interfaces, and operational safeguards.

Rationale per rubric:
- Clear, documented model: typed content kinds (`python/packages/core/agent_framework/_types.py:373-374`), per-tool gates (`python/packages/core/agent_framework/_tools.py:106,408`), scoped standing rules with hosted-server boundaries (`python/packages/core/agent_framework/_harness/_tool_approval.py:308-321`).
- Tests are unusually deep: forgery rejection, mixed-batch replay, hosted-server-boundary rules, budget interaction with auto-approval (`python/packages/core/tests/core/test_harness_tool_approval.py:649-1368`), lifecycle expiry/idempotency/conflict coverage (`python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:176-1287`), and cross-restart rehydration (`python/packages/core/tests/workflow/test_request_info_event_rehydrate.py:240-310`).
- Operational safeguards: immutable request snapshots, strict boolean approval semantics, deny-by-default MCP sampling gate, capacity-bounded lifecycle store.
- Not 9–10 because: no expiry for core pending approvals (indefinite authority), no durable queryable audit trail of decisions, and durability is opt-in per surface (AG-UI/DevUI defaults are process-local; `python/packages/ag-ui/agent_framework_ag_ui/_approval_state.py:38-45`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Control-plane content types | `function_approval_request` / `function_approval_response` registered as Content types | python/packages/core/agent_framework/_types.py:373-374 |
| Request factory | `Content.from_function_approval_request(id=call_id, function_call, user_input_request=True)` | python/packages/core/agent_framework/_types.py:1272-1291 |
| Response factory (strict) | non-bool `approved` coerced to `False`; `_is_approval_granted` requires `value is True` | python/packages/core/agent_framework/_types.py:1304-1313; python/packages/core/agent_framework/_tools.py:1975-1978 |
| Per-tool gate | `ApprovalMode = Literal["always_require","never_require"]`, default `never_require` | python/packages/core/agent_framework/_tools.py:106,408 |
| Batch pause rule | any `always_require` tool in a batch pauses the whole batch before execution | python/packages/core/agent_framework/_tools.py:1775-1795 |
| Visible-request construction + mixed-batch hiding | safe siblings stored hidden, keyed to visible request ids; pending batch persisted | python/packages/core/agent_framework/_tools.py:1796-1832; python/packages/core/agent_framework/_tools.py:2249-2269 |
| Session persistence (core) | pending snapshots under session key `tool_approval`; duplicate ids rejected | python/packages/core/agent_framework/_tools.py:98-100,2075-2095,2109-2124,2152-2153 |
| Response binding | response bound to surfaced request snapshot, consumed once; unknown ids dropped w/ warning | python/packages/core/agent_framework/_tools.py:2182-2246 |
| Standing-rule scopes | "always approve tool" vs "tool_with_arguments"; metadata `tool_approval.always_approve` | python/packages/core/agent_framework/_harness/_tool_approval.py:28-31,218-245 |
| Rule matching & hosted boundary | match by tool_name + canonicalized args + `server_label`; `None`=tool-wide, `{}`=no-arg only | python/packages/core/agent_framework/_harness/_tool_approval.py:50-58,301-321,553-572 |
| Hosted vs local split | `_is_hosted_tool_approval` detects `server_label`; hosted requests pass through untouched | python/packages/core/agent_framework/_tools.py:1962-1971 |
| Middleware contract | `ToolApprovalMiddleware` raises without `AgentSession`; state load/save round-trip | python/packages/core/agent_framework/_harness/_tool_approval.py:343-349,384-394,248-275 |
| Harness default wiring | middleware enabled by default in `create_harness_agent`, outermost except loop; HITL escape hatch stops loops on pending approvals | python/packages/core/agent_framework/_harness/_agent.py:636-646 |
| Auto-approval callback risk warning | documented name-collision bypass hazard for `auto_approval_rules` | python/packages/core/agent_framework/_harness/_tool_approval.py:365-376 |
| Workflow surfacing | `user_input_requests` become `request_info` events keyed by request id | python/packages/core/agent_framework/_workflows/_agent_executor.py:447-449,540-543 |
| Workflow pause state | run enters `IDLE_WITH_PENDING_REQUESTS`; responses validated against pending requests | python/packages/core/agent_framework/_workflows/_workflow.py:590,1013-1030 |
| Checkpoint capture of approvals | `pending_request_info_events` stored in checkpoints; chain via `previous_checkpoint_id` | python/packages/core/agent_framework/_workflows/_checkpoint.py:59-61,91 |
| Cross-restart durability proof | restore from checkpoint re-emits request, accepts response, completes workflow | python/packages/core/tests/workflow/test_request_info_event_rehydrate.py:240-310 |
| Durable hosting store | `FunctionApprovalStore` protocol; `FoundryFunctionApprovalStore` backed by FoundryStateStore, user isolation, conflict-on-duplicate | python/packages/foundry_hosting/agent_framework_foundry_hosting/_state_store.py:204-261 |
| Streaming-side persistence | approval requests saved into the store as they stream out | python/packages/foundry_hosting/agent_framework_foundry_hosting/_responses.py:1167-1192 |
| AG-UI lifecycle engine | statuses PENDING→CLAIMED→EXECUTING→SETTLED/REJECTED/CANCELLED/EXPIRED/INDETERMINATE; occurrence registry, aliases, claims | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-119,137-222 |
| AG-UI timeouts | pending retention default 86,400s; abandoned pendings expired during purge; `expire_batch` API | python/packages/ag-ui/agent_framework_ag_ui/_approval_state.py:16-19; python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:669-693,704-741 |
| AG-UI duplicate-execution protection | terminal outcomes retained 900s (15 min); indeterminate window 604,800s; idempotency keys allow retry | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:186,195-222,781-789 |
| Client-facing expiry hint | interrupt payload forwards `expiresAt` when present | python/packages/ag-ui/agent_framework_ag_ui/_run_common.py:347-349 |
| AG-UI store durability limit | `InMemoryAGUIApprovalStateStore`: process-local, not durable across restarts/replicas | python/packages/ag-ui/agent_framework_ag_ui/_approval_state.py:38-76 |
| DevUI validation | pending approvals tracked in memory dict; forged responses with unknown ids rejected | python/packages/devui/agent_framework_devui/_executor.py:67-69,126-136,755-759 |
| History-as-audit retention | unresolved approval controls preserved in history; resolved ones filtered from model replay | python/packages/core/agent_framework/_sessions.py:873-940,948 |
| Adjacent audit hooks | shell tool `on_command` audit hook; security middleware in-memory violation `audit_log` | python/packages/tools/agent_framework_tools/shell/_tool.py:137-140; python/packages/core/agent_framework/security.py:1697-1698,2174-2184 |
| MCP sampling approval gate | deny-by-default `sampling_approval_callback` + per-session rate limit (default 25) | python/packages/core/agent_framework/_mcp.py:135-138,456-464,562-564 |
| .NET parity: state shape | rules / collected / queued / surfaced requests persisted in `AgentSessionStateBag` | dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalState.cs:13-64 |
| .NET parity: binding & snapshotting | responses honored only against surfaced snapshots; consumed once; rebound tool call | dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:354-457 |
| .NET parity: rule scoping | null arguments = tool-wide; empty dict = no-arg only (cannot silently widen) | dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalRule.cs:12-28 |

## Answers to Dimension Questions

### 1. How is approval requested?

A tool opts in via `approval_mode="always_require"` (`python/packages/core/agent_framework/_tools.py:106`). During auto-invocation, if any call targets such a tool, the entire batch pauses before execution (`python/packages/core/agent_framework/_tools.py:1776-1790`): each call becomes a `function_approval_request` content whose id is the call's `call_id` (`_tools.py:1805-1808`), marked `user_input_request=True` (`_types.py:1287`). In workflows, `AgentExecutor` converts these into `request_info` events keyed by request id (`python/packages/core/agent_framework/_workflows/_agent_executor.py:447-449,540-543`). In AG-UI they surface as protocol `Interrupt`s with canonical resume entries (`approved`/`editedArgs`). Hosted-tool (server-executed) approvals carry a `server_label` and are passed through rather than executed locally (`python/packages/core/agent_framework/_tools.py:1962-1971`). A separate deny-by-default approval gate exists for MCP server-initiated sampling (`python/packages/core/agent_framework/_mcp.py:1440`).

### 2. Are approval sessions durable?

Yes, by design — but the durability tier depends on the host's storage choice:
- **Core agents**: pending requests, already-approved groups, standing rules, queues, and collected responses all live in `AgentSession.state` under the `tool_approval` key (`_tools.py:98-100,2075-2095`; `_harness/_tool_approval.py:158-215`). The middleware hard-fails without a session (`_tool_approval.py:384-385`). Sessions persist through pluggable stores (in-memory or file-backed per the package docs in `python/packages/core/AGENTS.md`), so approvals survive process restarts when a file/service-backed store is used.
- **Workflows**: strongest guarantee — pending request-info events are serialized into checkpoints (`_workflows/_checkpoint.py:59-61,91`); a test restores a *fresh* workflow instance from a checkpoint and completes the original approval (`tests/workflow/test_request_info_event_rehydrate.py:270-309`).
- **Foundry hosting**: server-side durable store (`foundry_hosting/agent_framework_foundry_hosting/_state_store.py:216-250`) with user isolation and conflict detection on duplicate save (`_state_store.py:240-242`).
- **AG-UI**: explicitly *not* durable across restarts/replicas — `InMemoryAGUIApprovalStateStore` is bounded and process-local (`ag-ui/_approval_state.py:38-45`).

### 3. Can approvals be scoped?

Yes, three axes:
- **One-shot (default)**: a bound response consumes its pending request (`_tools.py:2210-2212`); authority ends after one use.
- **Standing tool-wide**: `create_always_approve_tool_response` records a rule matching every future call to that tool name (`_harness/_tool_approval.py:218-231,555-562`).
- **Standing argument-exact**: `create_always_approve_tool_with_arguments_response` records canonicalized JSON argument values; `arguments=None` means tool-wide while `{}` matches exactly no-argument calls and can never widen (`_tool_approval.py:50-58`). Matching also includes the hosted `server_label` boundary so same-named tools on different hosted servers never share approvals (`_tool_approval.py:308-321`; test at `tests/core/test_harness_tool_approval.py:1224`). .NET mirrors both scopes with the same null-vs-empty-dictionary subtlety (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalRule.cs:12-28`). Additionally, heuristic `auto_approval_rules` callbacks exist but carry an explicit security warning about name-collision bypasses (`_tool_approval.py:365-376`), and provider-scoped static rules reject any call bearing a `server_label`.

### 4. Do approvals time out?

**Split answer.**
- **Core agent/session approvals: no timeout.** No expiry, TTL, or deadline logic exists anywhere in `_harness/_tool_approval.py` or the pending-approval state machinery in `_tools.py` (searches for `timeout|expire|ttl` return nothing relevant). An unanswered pending approval remains resumable indefinitely — arguably correct for durable HITL, but it means approval authority never decays.
- **AG-UI transport: yes, explicitly.** Pending occurrences expire after a configurable retention window (default 24 h = 86,400 s) via `expire_batch` and the purge sweep, transitioning to `EXPIRED`; expired authority cannot be claimed (`_approval_lifecycle.py:669-741`; tests `test_abandoned_pending_occurrence_expires_and_releases_capacity` and `test_expired_authority_cannot_be_claimed` at `tests/ag_ui/test_approval_lifecycle.py:176,1287`). Terminal outcomes are retained only 15 minutes for duplicate-execution protection, and indeterminate executions get a 7-day safety window (`_approval_state.py:16-19`). Interrupt payloads may also forward a client-facing `expiresAt` (`ag-ui/_run_common.py:347-349`).

### 5. Are approvals audited?

Partially — observability exists, but no first-class durable decision ledger:
- Conversation history retains unresolved approval controls until a terminal result closes the occurrence, and `HistoryProvider` explicitly supports "audit/logging storage (stores only, doesn't load)" configurations (`python/packages/core/agent_framework/_sessions.py:873-948`); a file-history test verifies resolved controls are filtered from model replay while pending ones stay resumable (`tests/core/test_harness_tool_approval.py:222`).
- The AG-UI lifecycle emits structured transition logs (`registration`/`cancellation`/`expiration`/`retention_purge`/claims/settlements) carrying occurrence id, status, and owner but no sensitive payloads (`_approval_lifecycle.py:762-778`; telemetry test at `tests/ag_ui/test_approval_lifecycle.py:405`).
- Adjacent hooks: shell tool `on_command` audit callback (`packages/tools/agent_framework_tools/shell/_tool.py:137-140`) and an in-memory policy-violation `audit_log` with `get_audit_log()` (`security.py:1697-1698,2174-2184`).
- No evidence found of a queryable, append-only, cross-session record of *who approved what, when, with which scope*. Searches across `python/packages` for `audit` surfaced only the above log/history surfaces.

## Architectural Decisions

1. **Approvals are message content, not side-channel state.** Encoding them as `Content` types (`_types.py:373-374`) makes approvals serializable, history-recordable, and transport-mappable (workflow events, AG-UI interrupts, OpenAI streaming items), at the cost that every layer must filter them correctly from model input (`_sessions.py:921-940`).
2. **Immutable-snapshot binding.** Surfaced requests are snapshotted into session state; inbound responses bind to the snapshot and consume it once (`_tools.py:2141-2213`; .NET equivalent `ToolApprovalAgent.cs:430-457`). This defeats forged responses (unknown ids ignored, `_tools.py:2236-2241`), tampering (caller mutation cannot alter the recorded call), and double-use.
3. **Session-state as the single source of truth for the harness**, with one shared `tool_approval` key cooperatively used by both `ToolApprovalMiddleware` and the function-invocation layer (`_tools.py:2075-2095`), making the whole approval session a unit of persistence/restore.
4. **Checkpoints, not queues, for workflow approvals.** Pending HITL events live inside chained workflow checkpoints (`_checkpoint.py:51-72,91`), giving crash-safe resumability without a separate approval service.
5. **Centralized lifecycle ownership in AG-UI.** `_approval_lifecycle.py` is documented as the sole owner of registration, aliases, claims, outcomes, and retry deduplication (package instructions in `python/packages/ag-ui/AGENTS.md`), preventing parallel registries in runner code.
6. **Cross-language parity by port.** .NET reproduces the Python semantics nearly 1:1 (state fields at `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalState.cs:18-64`), trading some duplication for consistent behavior across ecosystems.

## Notable Patterns

- **Mixed-batch partial approval**: safe siblings of a batch needing approval are hidden in session state and reinjected only when the visible approval resumes, so one risky call doesn't force re-approving safe ones (`_tools.py:1796-1832`; test `tests/core/test_harness_tool_approval.py:418`).
- **Queued prompts with retroactive rules**: multiple unapproved requests queue in session state and are presented one-at-a-time, letting an "always approve" answer resolve later queue members automatically (`_harness/_tool_approval.py:579-622`; test at `tests/core/test_harness_tool_approval.py:649`).
- **HITL-aware loops**: `AgentLoopMiddleware` checks for pending approval requests before evaluating loop continuation, so wrapping loops can't spin past a human gate (documented and wired in `python/packages/core/agent_framework/_harness/_agent.py:641-644`).
- **Occurrence-aware correlation**: reused `call_id` values are matched by ordered occurrence rather than global identity, supporting providers that recycle ids (documented in `python/packages/core/AGENTS.md`, Tool Approval Harness section; exercised by `tests/ag_ui/test_approval_lifecycle.py:1122`).
- **Indeterminate states as a safety class**: executions interrupted mid-flight become `INDETERMINATE` and are retained longer than normal terminal outcomes, with idempotency keys as the only path to retry after execution began (`_approval_lifecycle.py:93-119,781-789`).

## Tradeoffs

- **Indefinite core approval authority** maximizes resumability (a user can answer next week) but means a leaked pending approval id stays valid forever unless the session itself expires — no TTL compensating control exists in `_tools.py`.
- **Opt-in durability**: hosts get strong guarantees only if they choose file/service-backed stores; the convenient defaults (in-memory sessions, `InMemoryCheckpointStorage` at `_workflows/_checkpoint.py:202`, AG-UI in-memory store) trade durability for zero setup, and the failure is silent (state simply gone after restart).
- **Protocol duplication across layers**: core content protocol, workflow `request_info`, AG-UI interrupts/lifecycle, DevUI tracking dicts, and Foundry stores each re-implement parts of request/response correlation. Centralizing helped within packages, but five surfaces raise consistency cost — mitigated by heavy cross-surface tests.
- **Auto-approval ergonomics vs safety**: name-based `auto_approval_rules` are convenient but documented as a bypass hazard (`_tool_approval.py:365-376`); the framework chose warning + provider-scoped static rules rather than forbidding them.

## Failure Modes / Edge Cases

- **Forged or stale responses**: dropped with a warning when no pending request matches (`_tools.py:2236-2241`; DevUI rejects unknown ids outright, `devui/_executor.py:755-759`).
- **Duplicate pending request ids**: raise `ValueError` rather than silently shadowing (`_tools.py:2121-2122,2152-2153`).
- **Recorded tool disappears between request and resume**: approved call is not executed (test `tests/core/test_harness_tool_approval.py:842`).
- **Unrelated turns must not replay hidden batch requests**: verified that hidden mixed-batch requests only replay for a matching visible approval (`tests/core/test_harness_tool_approval.py:523,574`).
- **Expired authority reuse**: claiming an expired occurrence fails (`tests/ag_ui/test_approval_lifecycle.py:1287`); identical accepted retries return the retained outcome instead of re-executing (`tests/ag_ui/test_approval_lifecycle.py:1160`).
- **Capacity exhaustion**: protected active occurrences can exhaust the AG-UI store, raising `ApprovalCapacityError` instead of evicting live approvals (`_approval_state.py:106-110`; test `tests/ag_ui/test_approval_lifecycle.py:151`).
- **Execution interruption mid-call**: becomes `INDETERMINATE`, sibling claimed calls stay reserved, retry requires idempotency proof (`tests/ag_ui/test_approval_lifecycle.py:719,795,818`).
- **Non-boolean `approved` values** (e.g., truthy junk) fail closed: coerced to `False` at construction and checked with `is True` (`_types.py:1307`; `_tools.py:1975-1978`).

## Future Considerations

- Add an optional TTL/expiry for core pending approvals (mirroring AG-UI's `pending_retention_seconds`) so session-level approval authority can decay; today the only decay is the lifetime of the session store itself.
- Promote approval auditing from logs + history retention to an opt-in durable decision sink (who/what/scope/timestamp/outcome), likely alongside the existing `FunctionApprovalStore` pattern in `python/packages/foundry_hosting/agent_framework_foundry_hosting/_state_store.py:204-213`.
- Provide a durable reference implementation for the AG-UI approval store (the current `InMemoryAGUIApprovalStateStore` documents its own non-durability, `_approval_state.py:38-45`) so production deployments don't each have to write one.
- Unify timeout semantics: currently core waits forever while AG-UI expires in 24 h; a shared configuration knob would prevent divergent operator expectations.

## Questions / Gaps

- No evidence found of per-decision audit persistence (approver identity, timestamp, scope chosen) in core or DevUI; searches for `audit` in `python/packages` returned only history-provider, logging, and shell/security-hook usages (see Answers Q5).
- No evidence found that Go (`go/` directory contains only `README.md`) implements approvals; the study therefore reflects Python and .NET only.
- Whether Foundry-hosted approval requests ever expire server-side could not be determined from this source: `FoundryFunctionApprovalStore` defines save/load only (`_state_store.py:236-250`); retention would be a property of the external Foundry State Store service.
- The `expiresAt` field forwarded in AG-UI interrupts (`ag-ui/_run_common.py:347-349`) has no producer inside this repository — presumably set by upstream integrations; the enforcement path for client-visible expiry versus server-side retention windows is not demonstrated here.

---

Generated by dimension `14.02-approval-session-design` against `agent-framework`.
