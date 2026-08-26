# Source Analysis: openai-agents-sdk

## Dimension 08.02: Permission Policy and Approval Gates

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (OpenAI Agents SDK, `src/agents/` package) |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK implements human-in-the-loop (HITL) approval gating as a first-class, run-scoped state machine rather than an ad-hoc flag. Sensitive operations are gated per tool via a `needs_approval` setting that accepts a static bool or a per-call callable (`src/agents/tool.py:486-493` for function tools, `src/agents/tool.py:1368-1374` for shell, `src/agents/tool.py:1423-1429` for apply_patch, `src/agents/tool.py:1463-1466` for custom tools, `src/agents/agent.py:600-601` for agent-as-tool, and `require_approval` on MCP servers normalized at `src/agents/mcp/server.py:710-813`). When a gated call arrives and no stored decision applies, the run pauses: the call is surfaced as a `ToolApprovalItem` in `RunResult.interruptions` (`src/agents/run_internal/run_loop.py:1385`, item class at `src/agents/items.py:556-623`). The application resolves decisions through `RunState.approve()` / `RunState.reject()` (`src/agents/run_state.py:1255-1298`) or programmatically via `on_approval` callbacks executed inside `resolve_approval_status` (`src/agents/run_internal/tool_execution.py:1162-1213`). Decisions are stored in `RunContextWrapper._approvals` as `_ApprovalRecord` entries whose `approved`/`rejected` fields are either booleans (sticky "always" decisions bound to an authorization-scope fingerprint) or lists of exact call IDs (`src/agents/run_context.py:56-68`). The whole approval ledger is serialized with the versioned `RunState` snapshot format so paused runs survive process restarts (`src/agents/run_state.py:1300-1324`, schema history at `src/agents/run_state.py:182-217`). The design is fail-closed at multiple points and is covered by a large, dedicated test corpus including error scenarios and a released-schema compatibility corpus.

## Rating

**9 / 10.**

Rationale against the rubric:

- **Clear model with explicit interfaces**: approval states are an explicit tri-state record (`True` / `False` / `None` returned by `get_approval_status`, `src/agents/run_context.py:1065-1235`), with documented sticky vs. per-call semantics (`src/agents/docs/human_in_the_loop.md:45-57`).
- **Tests**: dedicated suites exist at `tests/test_run_context_approvals.py` (24+ tests covering scoping, aliasing, and malformed-state rejection), `tests/test_hitl_error_scenarios.py` (resume failures, nested-run isolation, fail-closed callable behavior — e.g., `test_callable_function_approval_fails_closed_for_invalid_arguments` at `tests/test_hitl_error_scenarios.py:932`), `tests/test_tool_approval_call_id_reuse.py` (identity-fingerprint drift), `tests/test_run_internal_approvals.py`, plus HITL coverage inside `tests/test_run_state.py`, `tests/realtime/test_session.py`, and `tests/mcp/test_mcp_approval.py`.
- **Operational safeguards**: fail-closed defaults when arguments cannot be parsed (`src/agents/run_internal/tool_execution.py:1307-1311`), invalid `needs_approval` types raise `UserError` (`src/agents/util/_approvals.py:46-50`), restored-but-unbound per-call decisions require reapproval (`src/agents/run_context.py:1308-1316`).
- **Durability**: versioned RunState persistence with a documented changelog where four of seventeen schema bumps are approval-specific (`src/agents/run_state.py:186-217`) and backward-read regression via `tests/test_run_state_compatibility_corpus.py`.
- It stops short of 10 because there is no time-based expiry/TTL for grants, no built-in approver identity/RBAC (the "who" is fully delegated to application code), and the key-resolution logic carries substantial legacy-compatibility branching that is hard to audit end-to-end (e.g., the candidate-key walk in `get_approval_status`, `src/agents/run_context.py:1108-1171`).

## Evidence Collected

Every entry cites paths relative to the selected source directory root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission schema (function tools) | `needs_approval: bool \| Callable[[RunContextWrapper, dict[str, Any], str], Awaitable[bool]] = False`; docstring documents pause/approve/reject semantics and per-call callable signature | src/agents/tool.py:486-493 |
| Permission schema (shell) | `ShellTool.needs_approval` + `on_approval`; hosted-environment validation forbids both (`UserError` at construction) | src/agents/tool.py:1368-1377, 1405-1410 |
| Permission schema (apply_patch / custom) | `ApplyPatchTool.needs_approval` + `on_approval`; `CustomTool.needs_approval`, `runtime_needs_approval()` | src/agents/tool.py:1423-1432, 1463-1466, 1497-1499 |
| Permission schema (agent-as-tool) | `Agent.as_tool(..., needs_approval=...)` gates delegated agent runs; nested interruptions surface on the outer run | src/agents/agent.py:600-601, 630; docs/human_in_the_loop.md:5-7 |
| Approval policy normalization (MCP) | `require_approval` accepts `"always"`/`"never"`, bool, name→bool map, `{always:{tool_names},never:{tool_names}}` list policy (with overlap rejected), or callable | src/agents/mcp/server.py:710-813 |
| MCP per-tool resolution, fail-closed | Callable policy with no agent available returns `True` ("preserve the historical fail-closed behavior") | src/agents/mcp/server.py:815-841 |
| Hosted MCP policy | `HostedMCPTool.tool_config={"require_approval": ...}` + optional `on_approval_request`; missing hook logs debug and surfaces interruptions | src/agents/run_internal/turn_resolution.py:3119-3124 |
| Approval states | `_ApprovalRecord.approved/rejected`: bool (sticky) or list of call IDs (exact); plus `rejection_messages`, `sticky_rejection_message`, `sticky_scope` | src/agents/run_context.py:56-68 |
| Status resolution precedence | Per-call ID decision checked first; conflicting sticky approve/reject resolves to approve; exact overrides sticky (schema 1.16 change) | src/agents/run_context.py:628-661; src/agents/run_state.py:212-215 |
| Sticky scope binding | Sticky decisions bind to `sticky_scope`, a SHA-256 fingerprint over invocation type + tool lookup key (+ server_label/name for hosted MCP) | src/agents/run_context.py:998-1002; src/agents/_tool_invocation.py:236-289 |
| Confirmation handler (programmatic) | `resolve_approval_status` builds the item, invokes `on_approval`, maps `{approve: bool, reason}` to `approve_tool`/`reject_tool` | src/agents/run_internal/tool_execution.py:1162-1213 |
| Confirmation handler (manual) | `RunState.approve/reject` resolve detached snapshots to authoritative pending items and route nested agent-tool approvals to their owning state | src/agents/run_state.py:1255-1298, 1095-1253 |
| Interruption surfacing | `NextStepInterruption.interruptions` filtered to `ToolApprovalItem`s and attached to `RunResult.interruptions` on streaming and non-streaming paths | src/agents/run_internal/run_loop.py:1385, 1925; src/agents/run.py:1206, 1364-1376; src/agents/run_internal/approvals.py:46-56 |
| Rejection output to model | Default message `"Tool execution was not approved."`; run-level `tool_error_formatter` (`kind="approval_rejected"`) and per-call `rejection_message` override | src/agents/tool.py:194; src/agents/run_internal/tool_execution.py:1230-1281; docs/human_in_the_loop.md:59-87 |
| Fail-closed argument inspection | Unparseable/non-object/nonstandard-constant JSON ⇒ callable policy skipped, approval required | src/agents/util/_approvals.py:18-29; src/agents/run_internal/tool_execution.py:1300-1318; docs/human_in_the_loop.md:15 |
| Planning-time gate | Resume/planning filter checks stored status, then evaluates `needs_approval_checker`; checker exceptions degrade to `needs_approval=True` | src/agents/run_internal/tool_planning.py:798-838 |
| Persistence | Approvals serialized into RunState JSON (`approved`, `rejected`, `rejection_messages`, `sticky_rejection_message`, `sticky_scope`); typed hosted-MCP records restored by identity kind (`server_tool`/`request`/`query`) | src/agents/run_state.py:1300-1324; src/agents/run_context.py:1318-1353 |
| Restore validation & re-binding | Malformed serialized values filtered; restored per-call decisions lacking a ledger binding are parked as unbound and require reapproval | src/agents/run_context.py:704-710, 1237-1265, 1308-1316 |
| Revocation semantics | Reject removes the call from the approved list and vice versa; latest decision wins per call ID; approving clears rejection messages | src/agents/run_context.py:1017-1037; tests/test_run_context_approvals.py:516 |
| Realtime parity | `RealtimeSession.approve_tool_call/reject_tool_call` enforce canonical invocation identity before resuming or rejecting | src/agents/realtime/session.py:969-1057 |
| Guardrail policy layer (separate) | Tool input/output guardrails with `allow` / `reject_content` / `raise_exception` behaviors; ordering vs. approvals controlled by `pre_approval_tool_input_guardrails` (guardrails also rerun post-approval) | src/agents/tool_guardrails.py:59-117; src/agents/run_config.py:146-150 |
| Caller restriction | `ensure_tool_caller_allowed` enforces `allowed_callers` on hosted MCP/shell/custom tools independent of approvals | src/agents/run_internal/turn_resolution.py:3106-3111 |
| Namespace isolation | Namespaced tools do not fall back to bare-name approvals; deferred-tool legacy keys are handled explicitly | tests/test_run_context_approvals.py:531-586; src/agents/_tool_identity.py:607-655 |
| Documentation | HITL guide covering marking tools, flow steps, sticky scope by server_label+tool name, partial resolution, durable serialization | docs/human_in_the_loop.md:1-203 |

## Answers to Dimension Questions

### 1. Which actions require approval?

Any tool can opt in; nothing requires it by default (`needs_approval=... default False` at `src/agents/tool.py:486-492`). Gateable surfaces: function tools (`@tool(needs_approval=True)`), shell (`src/agents/tool.py:1368`), apply_patch (`src/agents/tool.py:1423`), custom tools (`src/agents/tool.py:1463`), agent-as-tool (`src/agents/agent.py:600-601`), local MCP servers via `require_approval` (`src/agents/mcp/server.py:548-576`), and hosted MCP via `tool_config={"require_approval": "always"}` (`docs/human_in_the_loop.md:43`). Hosted shell environments deliberately reject approval configuration entirely — they neither support `needs_approval` nor `on_approval` (`src/agents/tool.py:1405-1410`), which avoids implying client-side gating the provider controls. Handoffs themselves are not approval-gated in this codebase (no `needs_approval` equivalent exists on `Handoff`, `src/agents/handoffs/__init__.py:126-371`); gating delegation happens only through `Agent.as_tool`.

### 2. Who can approve?

Three decision sources, all application-controlled:

1. **Humans via app code**: the SDK never prompts; the app inspects `result.interruptions` and calls `state.approve()/state.reject()` (`src/agents/run_state.py:1255-1298`; example loop in `docs/human_in_the_loop.md:141-165`).
2. **Programmatic callbacks**: `on_approval` on shell/apply_patch/custom tools and `on_approval_request` on hosted MCP auto-decide mid-turn (`src/agents/run_internal/tool_execution.py:1191-1204`).
3. **Realtime session callers**: WebSocket-facing `approve_tool_call`/`reject_tool_call` (`src/agents/realtime/session.py:969-1057`).

There is no SDK-level identity, role, or authentication concept for approvers — "who" is entirely delegated to the integrating application.

### 3. Are approvals scoped and expiring?

Yes on scoping, no on time-based expiry. Two granularities exist:

- **Per-call**: decisions recorded as exact call-ID lists and never authorize a different invocation (`src/agents/run_context.py:660-661` comment: "Per-call approvals are scoped to the exact call ID, so other calls require a new decision").
- **Sticky (`always_*`)**: opt-in, but not global — bound to a fingerprinted authorization scope derived from invocation type plus canonical tool lookup key (namespace-aware) or `server_label` + tool name for hosted MCP (`src/agents/_tool_invocation.py:236-289`; server-label isolation test at `tests/test_run_context_approvals.py:30`). An exact-call decision overrides a same-key sticky decision since schema 1.16 (`src/agents/run_context.py:649-652`).

Lifetime: decisions live inside the `RunContextWrapper._approvals` dict and persist across serialize/resume cycles for the same paused run (`src/agents/run_state.py:1300-1324`, `docs/human_in_the_loop.md:53`). There is **no TTL**; grants lapse only when the run completes or the state is discarded, and resumed per-call decisions without a matching invocation-ledger binding are invalidated (forced reapproval) at `src/agents/run_context.py:1308-1316`. Sticky grants do **not** leak into new runs or into nested agent-tool contexts (nested-isolation test at `tests/test_hitl_error_scenarios.py:480`).

### 4. Can policy override model intent?

Yes, decisively. A rejected call never executes; instead a synthetic tool output is fed back to the model — by default `"Tool execution was not approved."` (`src/agents/tool.py:194`), customizable run-wide or per call (`src/agents/run_internal/tool_execution.py:1230-1281`, `src/agents/run_internal/approvals.py:24-43`). The pipeline is fail-closed against model misbehavior in several ways: unparseable arguments force manual approval instead of invoking the policy callable (`src/agents/run_internal/tool_execution.py:1307-1311`); a crashing policy callable degrades to "requires approval" rather than allowing (`src/agents/run_internal/tool_planning.py:806-812`); an invalid `needs_approval` type raises `UserError` instead of silently allowing (`src/agents/util/_approvals.py:46-50`); and call-ID reuse for a different invocation raises `ModelBehaviorError` (`src/agents/run_context.py:344-352`). Conversely, the model cannot self-approve: stored decisions are consulted first, and if none applies the run interrupts regardless of what the model requested (`src/agents/run_internal/tool_planning.py:805-838`).

> **Can approval be granted narrowly rather than globally?** Yes — narrow-by-default. The default grant is scoped to one call ID; the broadest available grant (`always_approve=True`) is still confined to a single fingerprinted tool identity within one run, and namespaced tools cannot be unlocked by decisions recorded under their bare names (`tests/test_run_context_approvals.py:531-556`).

## Architectural Decisions

1. **Approval state lives on the run context, keyed by tool identity, not on the tool objects.** `_approvals: dict[str | HostedMCPApprovalKey, _ApprovalRecord]` (`src/agents/run_context.py:89`) centralizes all decisions; tools stay stateless declarative policies. Derived wrappers share the same dicts by reference (`_share_tool_state_with`, `src/agents/run_context.py:108-115`), while checkpointed copies deep-copy them for independence (`_copy_for_run_state`, `src/agents/run_context.py:117-131`).

2. **Dual-mode decision encoding (bool vs. call-ID list)** in a single record type (`src/agents/run_context.py:56-68`) keeps sticky and per-call grants in one lookup structure, with deterministic conflict rules (approve wins ties, exact beats sticky; `src/agents/run_context.py:649-658`).

3. **Interrupt-and-resume rather than block.** Gated calls pause the whole run and surface `ToolApprovalItem`s; resume replays pending items through `_select_function_tool_runs_for_resume` and related filters (`src/agents/run_internal/tool_planning.py:883-909+`). This makes HITL compatible with durable workflows (state serializable to JSON/string, `src/agents/run_state.py:1704+, 2042+, 2100+`).

4. **Versioned persistence boundary for approvals.** Approval-related changes are explicit schema bumps with summaries: 1.0 initial HITL, 1.6 rejection messages, 1.14 hosted-MCP server-label scoping, 1.16 exact-over-sticky override (`src/agents/run_state.py:186-217`), guarded by a released-contract corpus test (`tests/test_run_state_compatibility_corpus.py`).

5. **Canonical invocation ledger as the trust anchor.** Every approval decision must match a registered `_ToolInvocationRecord` (type, approval_scope fingerprint, executed/completed flags; `src/agents/run_context.py:45-53`); decisions without a bindable identity are rejected or quarantined as "restored unbound" (`src/agents/run_context.py:938-941, 1308-1316`). This prevents replaying an old decision against a mutated call payload.

6. **Fail-closed bias.** Across argument-parse failure, checker exceptions, missing agent context for MCP callable policies, and incomplete hosted-MCP identities, the system chooses "require approval" over "allow" (`src/agents/run_internal/tool_execution.py:1307-1311`; `src/agents/run_internal/tool_planning.py:806-812`; `src/agents/mcp/server.py:827-831`; `src/agents/run_context.py:900-907`).

## Notable Patterns

- **Policy-as-value-or-callable uniformly applied** across five tool kinds via one evaluator helper (`evaluate_needs_approval_setting`, `src/agents/util/_approvals.py:32-51`), shared by Runner, realtime, and sandbox shells.
- **Identity-keyed mirroring**: one decision writes to multiple approval keys (canonical lookup-key key plus legacy aliases) so renamed/deferred tools keep working without widening access (`src/agents/run_context.py:888-996`; key derivation `src/agents/_tool_identity.py:607-655`).
- **Nested-approval routing**: approvals raised inside `Agent.as_tool()` executions bubble to the outer `RunState`, which locates the owning nested state and delegates the decision, refusing ambiguous double-ownership (`src/agents/run_state.py:1095-1253`; mirrored in `ToolContext`, `src/agents/tool_context.py:125-228`).
- **Rejection messages as data**: per-call-ID rejection strings and sticky rejection reasons travel with the record and back to the provider (`reason` field on `McpApprovalResponse`, `src/agents/run_internal/tool_execution.py:1496-1503`).
- **Layered defense separate from approvals**: tool input/output guardrails (`src/agents/tool_guardrails.py:59-117`), `allowed_callers` restrictions enforced at response-processing time (`src/agents/run_internal/turn_resolution.py:3106-3111`), and sandbox mount security operate independently of the approval gate, with an explicit config knob for guardrail-vs-approval ordering (`src/agents/run_config.py:146-150`).

## Tradeoffs

- **Correctness vs. complexity**: the candidate-key walk in `get_approval_status` (`src/agents/run_context.py:1108-1235`) and hosted-MCP reconciliation (`collect_manual_mcp_approvals`, `src/agents/run_internal/tool_execution.py:1406-1512`) handle many legacy/partial-identity cases; this maximizes backward compatibility but makes auditing the full authorization path genuinely hard.
- **Run-scoped stickiness vs. operator convenience**: sticky grants expire with the run and never persist across runs — safer, but operators of long-lived automations must re-approve frequently or build their own grant store.
- **Interrupt-based HITL vs. liveness**: any gated call freezes the entire run until resolved (partial resolution supported, `docs/human_in_the_loop.md:57`), trading throughput for a simple, uniform pause semantic.
- **Fail-closed defaults vs. availability**: parse errors or crashing policies convert would-be auto-approved calls into manual interruptions, prioritizing safety over autonomy.

## Failure Modes / Edge Cases

Handled explicitly (with tests):

- Model reuses a call ID for a different invocation → `ModelBehaviorError` (`src/agents/run_context.py:344-352`; `tests/test_tool_approval_call_id_reuse.py:270-298`).
- Non-object JSON / `NaN`/`Infinity` constants in arguments → policy callable bypassed, approval forced (`src/agents/util/_approvals.py:22-29`; `tests/test_hitl_error_scenarios.py:932`).
- Corrupt serialized approvals → malformed values dropped on restore (`src/agents/run_context.py:704-710, 1237-1245`; `tests/test_run_context_approvals.py:673`).
- Restored per-call decisions with no ledger binding → forced reapproval (`src/agents/run_context.py:1308-1316`; `tests/test_hitl_error_scenarios.py:763`).
- Ambiguous ownership (same identity in current run and nested run, or duplicated identities) → `UserError` refusing to guess (`src/agents/run_state.py:1087-1092, 1241-1245`).
- Hosted MCP decisions with missing request id / incomplete identity → rejected up front; persistent decisions require complete `server_label` + tool name (`src/agents/run_context.py:897-907`; `tests/test_run_context_approvals.py:360-377`).
- Invalid `needs_approval` configuration type → `UserError` at evaluation time (`src/agents/util/_approvals.py:46-51`; `tests/test_hitl_error_scenarios.py:905`).
- Cross-server hosted-MCP name collisions → sticky decisions scoped per server label do not authorize another server (`tests/test_run_context_approvals.py:207-243`).

Residual risks: no TTL means a serialized paused run held indefinitely retains valid sticky grants; approval records for many tools grow unboundedly within one long run (lists of call IDs are pruned only per-decision flip, `src/agents/run_context.py:1017-1030`).

## Future Considerations

- Add optional TTL/expiry metadata to `_ApprovalRecord` for long-paused durable workflows (currently only structural unbinding invalidates decisions).
- Introduce an approver-identity hook (who approved, attestation) so enterprises can satisfy audit requirements without wrapping `approve`/`reject`.
- Consolidate legacy approval-key aliases behind a migration window to shrink the authorization-path surface (`src/agents/_tool_identity.py:607-655`).
- Expose metrics/tracing spans specific to gate outcomes (allow/deny/interrupt counts) beyond existing tool tracing, for observability of denial rates.

## Questions / Gaps

- **No evidence found** for time-based expiration of approvals: searched `src/agents/run_context.py`, `src/agents/run_state.py`, and docs for TTL/expiry/timestamp concepts around approvals — only structural rebinding checks exist (`src/agents/run_context.py:1308-1316`).
- **No evidence found** for built-in RBAC/multi-user approval identity: searched `src/agents/` for approver/auth/role concepts tied to `approve_tool`/`reject_tool`; the API takes no principal argument (`src/agents/run_context.py:1043-1063`).
- **Not verifiable from source alone**: whether hosted-MCP sticky decisions are additionally enforced server-side after the SDK sends `mcp_approval_response`; the SDK side is implemented at `src/agents/run_internal/tool_execution.py:1496-1505` but provider enforcement is external.
- Sandbox capability tools (shell/filesystem under `src/agents/sandbox/capabilities/`) expose their own preflight/approval seams (e.g., `tests/sandbox/capabilities/test_apply_patch_preflight.py`); their interaction with the run-level approval ledger was out of scope for this dimension's boundary and is only partially traced here.

---

Generated by `Dimension 08.02: Permission Policy and Approval Gates` against `openai-agents-sdk`.
