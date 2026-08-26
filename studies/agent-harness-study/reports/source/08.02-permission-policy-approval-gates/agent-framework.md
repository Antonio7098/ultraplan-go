# Source Analysis: agent-framework

## Dimension 08.02: Permission Policy and Approval Gates

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Polyglot monorepo: Python (primary implementation studied), .NET/C# harness parity, Go (early, no approval layer found) |
| Analyzed | 2026-08-26 |

All citations below are relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Agent Framework implements permission gating as a layered control plane built around two content types — `function_approval_request` and `function_approval_response` (`python/packages/core/agent_framework/_types.py:373-374`) — that flow through the function-invocation loop alongside ordinary tool calls. The base schema is a per-tool `ApprovalMode` literal of `"always_require" | "never_require"` (`python/packages/core/agent_framework/_tools.py:106`), defaulting to `never_require` (`python/packages/core/agent_framework/_tools.py:408`). Sensitive built-in tool families (file access, shell, skills) flip the default to `always_require`, and the shell tool additionally requires an explicit `acknowledge_unsafe=True` to disable approval at all (`python/packages/tools/agent_framework_tools/shell/_tool.py:153-166`).

On top of that sit several independent policy layers: an opt-in session-backed `ToolApprovalMiddleware` with standing rules, queued prompts, and heuristic auto-approval rules (`python/packages/core/agent_framework/_harness/_tool_approval.py:343`); a FIDES-style information-flow middleware (`LabelTrackingFunctionMiddleware` + `PolicyEnforcementFunctionMiddleware`) that can block or demand approval when untrusted context taints a call (`python/packages/core/agent_framework/security.py:1643-1704`); a deny-by-default MCP sampling gate with rate limit and approval callback (`python/packages/core/agent_framework/_mcp.py:1477-1548`); and a server-owned AG-UI approval lifecycle with explicit state machine, capacity caps, and retention windows (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:236-265`). Approval decisions are fail-closed throughout: only the strict boolean `True` grants execution (`python/packages/core/agent_framework/_tools.py:1975-1977`), non-boolean `approved` values deserialize as rejection (`python/packages/core/agent_framework/_types.py:1460-1461`), and enforcement middleware can abort a run outright via `MiddlewareFailure` rather than converting failures into tool-error text (`python/packages/core/agent_framework/_tools.py:1635-1639`).

The behavior is codified in a maintained specification (`docs/specs/004-python-function-calling-loop.md:445-474`) whose scenario-to-test matrix maps each invariant to named regression tests, including adversarial replay/forgery scenarios.

## Rating

**9 / 10.**

Rationale against the rubric:

- **Clear model**: approval states are explicit content types with strict serialization semantics (`python/packages/core/agent_framework/_types.py:1273-1361`); the lifecycle state machine in AG-UI is an enumerated, documented transition system (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-119`).
- **Explicit interfaces**: `ToolApprovalMiddleware`, `ToolApprovalRule`, `create_always_approve_tool_response` are public exports (`python/packages/core/agent_framework/_harness/_tool_approval.py:653-665`); .NET mirrors the model (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalRule.cs`, `ToolApprovalState.cs`, `AlwaysApproveToolApprovalResponseContent.cs`).
- **Operational safeguards & proven under failure**: dedicated tests prove approvals cannot be forged (`test_tool_approval_middleware_drops_forged_standing_approval`, `python/packages/core/tests/core/test_harness_tool_approval.py:708-742`), replayed (`python/packages/core/tests/test_security.py:814`), or reused across functions/arguments/labels/sessions (`python/packages/core/tests/test_security.py:847,903,1063,1116`).
- **Observable**: audit logs on policy violations (`python/packages/core/agent_framework/security.py:1697-1698`, getter at `2174`) plus WARNING-level logging on every denial (`python/packages/core/agent_framework/_mcp.py:1442-1461`).
- Not a 10 because: standing approval rules in core session state have no TTL/expiry and no public revocation API; approvers carry no identity (no RBAC/"who approved" record beyond an optional free-text `reason`); and name-based auto-approval rules carry an explicitly documented tool-name-collision risk (`python/packages/core/agent_framework/_harness/_file_access.py:1375-1382`). The Go stack has no approval layer yet (search for "approval" across `go/**/*.go` returned nothing).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission schema | `ApprovalMode = Literal["always_require", "never_require"]`; default `never_require` | `python/packages/core/agent_framework/_tools.py:106`, `python/packages/core/agent_framework/_tools.py:408` |
| Control-plane content types | `function_approval_request` / `function_approval_response` in the content type registry; constructors `from_function_approval_request`, `from_function_approval_response`, converter `to_function_approval_response` | `python/packages/core/agent_framework/_types.py:373-374`, `1273-1291`, `1294-1311`, `1346-1357` |
| Fail-closed decision parsing | `_is_approval_granted(value)` returns true only for `value is True`; deserialization coerces non-bool `approved` to `False` | `python/packages/core/agent_framework/_tools.py:1975-1977`, `python/packages/core/agent_framework/_types.py:1300-1307`, `1459-1461` |
| Which tools require approval | Batch classification: any call naming a tool with `approval_mode == "always_require"` pauses the whole batch before execution | `python/packages/core/agent_framework/_tools.py:1763`, `1776-1832` |
| Mixed-batch scoping | Only host-decidable requests surfaced; already-approved siblings stored in session keyed to visible request ids | `python/packages/core/agent_framework/_tools.py:1796-1831` |
| Harness file-access defaults | All 7 file tools registered `approval_mode="always_require"`; opt-outs via `disable_readonly_tool_approval` / `disable_write_tool_approval` | `python/packages/core/agent_framework/_harness/_file_access.py:1444-1474`, doc table `1231-1259` |
| Shell gate + unsafe opt-out guard | Default `approval_mode="always_require"`; `never_require` requires `acknowledge_unsafe=True` or raises | `python/packages/tools/agent_framework_tools/shell/_tool.py:150-172` |
| Skills provider defaults | Skill tools registered with `approval_mode="always_require"`, so each skill operation needs approval; static read-only/all auto-approval rules provided | `python/packages/core/agent_framework/_skills.py:1866`, `2454`; rules referenced from `python/packages/core/agent_framework/_harness/_tool_approval.py:355` |
| MCP per-tool approval modes | `MCPSpecificApproval` dict of always/never tool-name lists; resolution `_determine_approval_mode` | `python/packages/core/agent_framework/_mcp.py:78-88`, `1704-1718` |
| Declarative YAML approval config | `McpServerApprovalMode` hierarchy incl. `McpServerToolSpecifyApprovalMode(alwaysRequireApprovalTools=…)` | `python/packages/declarative/agent_framework_declarative/_models.py:737-814` |
| Standing rules (persistence) | `ToolApprovalRule(tool_name, arguments, server_label)`; `ToolApprovalState(rules, queued_approval_requests, collected_approval_responses)` serializable via `SerializationMixin` into session state | `python/packages/core/agent_framework/_harness/_tool_approval.py:86-155`, `158-215`, store/load at `248-275` |
| Rule matching scope | Tool-wide (`arguments is None`) vs exact-argument match; empty-dict rule matches only zero-argument calls, never wildcard; hosted `server_label` must equal | `python/packages/core/agent_framework/_harness/_tool_approval.py:301-321`, `50-58` |
| Narrow "always approve" helpers | `create_always_approve_tool_response` / `create_always_approve_tool_with_arguments_response` embed scope metadata consumed by middleware | `python/packages/core/agent_framework/_harness/_tool_approval.py:218-245`, extraction at `324-334` |
| Session-pending request registry | Immutable request snapshots keyed by id; duplicate ids raise `ValueError`; new batch replaces abandoned authority | `python/packages/core/agent_framework/_tools.py:2110-2124`, `2141-2157` |
| Response→request binding | `_bind_approval_response_to_pending_request` rebuilds the response from the recorded immutable call, preventing call-id/name/argument substitution; unbound responses dropped with warning | `python/packages/core/agent_framework/_tools.py:2182-2213`, warning at `2237-2239` |
| Rejection handling | Rejected calls produce terminal result `"Error: Tool call invocation was rejected by user."` without execution | `python/packages/core/agent_framework/_tools.py:2599-2606` |
| Queued multi-request flow | Middleware queues unresolved requests beyond the first; auto-approved ones drained via collected responses | `python/packages/core/agent_framework/_harness/_tool_approval.py:593-622`, drain at `579-586` |
| Auto-approval rules + collision warning | Callbacks receive full `function_call`; security warning about name collisions bypassing approval boundary | `python/packages/core/agent_framework/_harness/_tool_approval.py:360-377`, matcher `640-650` |
| Read-only auto-approval rule (narrow grant) | `read_only_tools_auto_approval_rule` approves only read/ls/grep; rejects any call carrying `server_label` | `python/packages/core/agent_framework/_harness/_file_access.py:1356-1394` |
| Policy engine (FIDES) | `PolicyEnforcementFunctionMiddleware(allow_untrusted_tools, block_on_violation, enable_audit_log, approval_on_violation)` blocks untrusted-context calls unless allowlisted | `python/packages/core/agent_framework/security.py:1643-1704` |
| Policy-violation approval binding | `_PendingPolicyApproval` binds approval to function+args signature, security label, session, and exact violation set; consume-on-use | `python/packages/core/agent_framework/security.py:1624-1704`, match/consume at `1824-1870`, disclosure of all violations at `1882-1924` |
| Middleware can override model intent | Function middleware returning `function_approval_request` is passed through so the loop's approval flow activates even mid-batch | `python/packages/core/agent_framework/_tools.py:1612-1626` |
| Fail-closed enforcement escape | `MiddlewareFailure` re-raised instead of being converted into a tool error result | `python/packages/core/agent_framework/_tools.py:1635-1641` |
| MCP sampling gate | Deny-by-default callback gate + per-session rate limit (25) + maxTokens cap (4096); denials logged WARNING | `python/packages/core/agent_framework/_mcp.py:137-144`, `1434-1548` |
| Shell pre-filter policy | `ShellPolicy` allow/deny/custom evaluated before approval AND execution; module docstring explicitly disclaims security-boundary status and lists bypass classes | `python/packages/tools/agent_framework_tools/shell/_policy.py:1-35`, evaluation `102-121`, audit hook `python/packages/tools/agent_framework_tools/shell/_tool.py:235-253` |
| Hosted-tool approval boundary | Approvals with `server_label` are pass-through to the provider API, not executed locally | `python/packages/core/agent_framework/_tools.py:1962-1972` |
| DevUI server-side validation | Pending approval requests tracked by request id; responses validated against stored function call before honoring | `python/packages/devui/agent_framework_devui/_executor.py:126-136`, `742-755` |
| AG-UI server-owned lifecycle | `ApprovalStatus` state machine (pending→claimed→executing→settled/rejected/cancelled/expired/indeterminate); process-local registry with capacity and retention windows (24 h pending, 7 d indeterminate, 15 min terminal) | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-119`, `236-265` |
| AG-UI expiry/purge | `expire_batch`, `_purge_expired_terminal`, `EXPIRED` terminal authority ("Reclamation never recreates approval authority") | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:669-732`; spec statement `docs/specs/004-python-function-calling-loop.md:405-407` |
| Authoritative spec + test matrix | Function-calling loop spec mandates extra validation for approval paths and maps ~30 approval scenarios to named regression tests | `docs/specs/004-python-function-calling-loop.md:31-35`, `445-474` |
| .NET parity | `ToolApprovalRule.cs`, `ToolApprovalState.cs`, `AlwaysApproveToolApprovalResponseContent.cs`, `ToolApprovalAgent.cs` mirror the Python model | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/` |

## Answers to Dimension Questions

### 1. Which actions require approval?

Any local tool registered with `approval_mode="always_require"`; the invocation loop classifies the entire model batch and pauses before executing anything if one call needs approval (`python/packages/core/agent_framework/_tools.py:1776-1832`). The framework's own sensitive surfaces default to requiring it: all file-access tools (`python/packages/core/agent_framework/_harness/_file_access.py:1444-1474`), both shell tools (`python/packages/tools/agent_framework_tools/shell/_tool.py:153`), and skill tools. MCP servers can require approval per tool via `MCPSpecificApproval` name lists (`python/packages/core/agent_framework/_mcp.py:78-88`, `1704-1718`) or wholesale via constructor `approval_mode` (`_mcp.py:446,541`). Additionally, *policy* violations (untrusted-context integrity or confidentiality breaches) can force approval even for otherwise-unrestricted tools when `PolicyEnforcementFunctionMiddleware(approval_on_violation=True)` is installed (`python/packages/core/agent_framework/security.py:1674-1704`; behavior shown in `python/packages/core/tests/test_security.py:618-661`). Conversely, declaration-only tools surface as user-input requests rather than executions (`python/packages/core/agent_framework/_tools.py:1833-1843`), and hosted-tool approvals are delegated to the remote service (`python/packages/core/agent_framework/_tools.py:1962-1972`).

### 2. Who can approve?

The host application / end user driving the run — there is no identity model attached to approvals. The core loop returns `function_approval_request` contents to the caller and accepts `function_approval_response` contents back (`docs/specs/004-python-function-calling-loop.md:128-173`); whoever can inject messages into the session acts as approver. UI transports add integrity checks but not identity: DevUI validates responses against its server-side pending-request registry (`python/packages/devui/agent_framework_devui/_executor.py:126-136,742-755`), and AG-UI makes the *server* the sole owner of approval authority with thread-scoped occurrence identities (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:137-144,236-237`). An optional free-text `reason` is stored on standing-approval metadata (`python/packages/core/agent_framework/_harness/_tool_approval.py:239-245`), but no approver identity, role, or RBAC concept exists anywhere in the searched code (`grep` for `approver|approved_by` over the approval modules returned nothing). This is the clearest gap versus enterprise delegation models.

### 3. Are approvals scoped and expiring?

**Scoping is strong.** A one-shot response authorizes exactly one call occurrence. Standing ("always approve") grants can be narrowed three ways: per tool, per tool+exact-argument-set (canonicalized JSON comparison where an empty dict matches only zero-arg calls, never everything — `python/packages/core/agent_framework/_harness/_tool_approval.py:50-58,301-321`), and per hosted-server boundary via `server_label` so same-named tools on different servers never share approvals (`_tool_approval.py:61-64,315`; regression test `test_tool_approval_middleware_standing_rules_include_hosted_server_boundary` at `python/packages/core/tests/core/test_harness_tool_approval.py:1224`). Policy-violation approvals go further: bound to the function-name+arguments signature, the disclosed security label, the session, and the exact violation set, and consumed on first use (`python/packages/core/agent_framework/security.py:1824-1870`; tests at `python/packages/core/tests/test_security.py:814-1163`).

**Expiring is partial.** The AG-UI lifecycle expires authority deterministically — pending grants expire after a configurable window (default 86,400 s), terminal outcomes are purged after 900 s, and expiration never resurrects authority (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:242-260,669-732`). But core `ToolApprovalMiddleware` standing rules persisted in session state have **no TTL and no revocation API**: they live until the session state is cleared or manually edited (nothing in `_tool_approval.py` implements removal; grep for `revoke|expir` in that file returned nothing). Mitigations exist — a newly surfaced batch replaces abandoned pending authority (`python/packages/core/agent_framework/_tools.py:2141-2179`) and removing the recorded tool makes a resumed approval execute nothing (`test_approval_resume_does_not_execute_when_recorded_tool_disappears`, `test_harness_tool_approval.py:842-880`) — but a granted tool-wide standing rule effectively persists indefinitely within the session's lifetime.

### 4. Can policy override model intent?

Yes, in both directions and at multiple layers. Function middleware sits between the loop and every tool execution and can (a) return a `function_approval_request` as its result, which the loop honors even though the model asked to execute now (`python/packages/core/agent_framework/_tools.py:1612-1626`); (b) terminate the batch via `MiddlewareTermination`; or (c) abort the whole run fail-closed via `MiddlewareFailure`, which is deliberately re-raised rather than laundered into a tool-error string (`_tools.py:1635-1641`). `PolicyEnforcementFunctionMiddleware` blocks calls in tainted contexts unless the tool is explicitly allowlisted (`python/packages/core/agent_framework/security.py:1648-1653,2077+`), and `ShellPolicy` denies commands before approval is ever requested (`python/packages/tools/agent_framework_tools/shell/_policy.py:74-88`; enforced at `shell/_tool.py:251-253`). The framework is candid that regex policies are UX filters, not security boundaries — the documented boundary is approval-in-the-loop plus sandbox tier (`_policy.py:31-34`) — which is an honest and correct threat model.

## Architectural Decisions

1. **Approvals as transcript contents, not side-channel flags.** Requests/responses are first-class `Content` types (`python/packages/core/agent_framework/_types.py:1273-1357`) that persist through history providers and survive streaming/non-streaming duality, enabling resumable HITL across processes. Cost: complex occurrence-aware correlation/replay normalization (`_replace_approval_contents_with_results`, `python/packages/core/agent_framework/_tools.py:2503-2672`).
2. **Session-state persistence with immutable request snapshots.** Surfaced requests are serialized once and rebound on resume so a caller-supplied response cannot substitute call id, tool name, or arguments (`python/packages/core/agent_framework/_tools.py:2141-2213`); duplicate request ids fail loudly (`2122`, `2153`).
3. **Opt-in middleware rather than ambient gatekeeping.** `ToolApprovalMiddleware` requires an `AgentSession` and raises otherwise (`python/packages/core/agent_framework/_harness/_tool_approval.py:381-385`); plain agents without it still enforce per-tool `approval_mode` inside the loop itself (`_tools.py:1763-1832`). This keeps the minimal guarantee in the loop and rich workflow in a composable layer.
4. **Deny-by-default at trust boundaries.** MCP sampling denied unless a callback is configured (`python/packages/core/agent_framework/_mcp.py:1440-1462`); strict-boolean approval semantics everywhere (`_tools.py:1975-1977`); unknown-role/unlabeled content defaults UNTRUSTED in the FIDES layer (`python/packages/core/agent_framework/security.py:595-607`); skills default TRUSTED-integrity only for declared trusted sources (`security.py:750-763`).
5. **Honest threat modeling in documentation.** `ShellPolicy` documents concrete bypass techniques and names the survey finding that no major peer framework uses regex as a primary control (`python/packages/tools/agent_framework_tools/shell/_policy.py:14-29`); auto-approval rules warn about their own name-collision weakness (`_tool_approval.py:365-377`).
6. **Spec-driven change control.** The function-calling loop spec declares this area extra-validation territory because "small changes can … replay stale approval authority" and external contributors must coordinate (`docs/specs/004-python-function-calling-loop.md:31-35`), backed by a scenario-to-test matrix (`445-474`).
7. **Cross-language parity for the model, not the machinery.** .NET mirrors `ToolApprovalRule`/state/response content (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/`) while Go has no approval layer at all yet.

## Notable Patterns

- **Occurrence-aware correlation**: reused `call_id`s after completion create fresh occurrences, and results consume ordered groups per occurrence rather than one global mapping (`python/packages/core/agent_framework/_tools.py:2531-2635`).
- **Mixed-batch sibling handling**: when a batch contains both gated and ungated calls, ungated calls are pre-approved invisibly and reinjected only when the visible approval resumes (`python/packages/core/agent_framework/_tools.py:1796-1831`; test `test_mixed_batch_hides_already_approved_request_until_approval_replay`, `test_harness_tool_approval.py:418`).
- **Disclosure-before-decision**: all applicable policy violations are detected up front so one approval discloses the complete risk set, and any change to that set forces re-approval (`python/packages/core/agent_framework/security.py:1994-2060`; tests `test_multiple_violations_disclosed_in_single_approval` and `test_replay_with_new_violation_set_requires_fresh_approval`, `test_security.py:1214,1275`).
- **Consume-on-use authority**: approved policy approvals are popped after one execution so a repeated identical call re-prompts (`python/packages/core/agent_framework/security.py:1864-1870`).
- **Static, auditable auto-approval presets**: providers expose class-level rules (e.g., `FileAccessProvider.read_only_tools_auto_approval_rule`) that operators wire into middleware explicitly, keeping the default posture conservative (`python/packages/core/agent_framework/_harness/_file_access.py:1360-1394`).
- **Server-owned authority at the transport edge**: AG-UI forbids client-controlled history from acting as authorization evidence and requires deterministic server-side checks (`python/packages/ag-ui/AGENTS.md`, protocol notes; enforced via `_approval_lifecycle.py` ownership model at `236-237`).

## Tradeoffs

- **Resumability vs complexity**: content-in-transcript approvals make HITL durable and auditable but produced one of the largest, most subtle code paths in the framework (`_tools.py` is 3,694 lines; ~560 approval-related lines in the 6,386-line invocation test suite alone). The spec explicitly warns that edits here can "duplicate side effects … [or] replay stale approval authority" (`docs/specs/004-python-function-calling-loop.md:31-33`).
- **Narrow grants vs collision risk**: argument-scoped standing rules are precise, but auto-approval rules match local tools *by name*, and the framework itself documents that a colliding caller-configured tool name (e.g., the shell tool) would be silently auto-approved (`python/packages/core/agent_framework/_harness/_file_access.py:1375-1382`). Scoping by tool identity rather than name would close this, at the cost of cross-instance rule portability.
- **Deny-by-default vs friction**: safe defaults (MCP sampling off, shell approval mandatory, non-bool approvals rejected) maximize safety but push ergonomic burden onto hosts, who must consciously configure callbacks/auto-rules (`_mcp.py:1494-1496`).
- **Regex policy transparency vs false assurance**: shipping no default deny patterns avoids a false sense of safety (`shell/_policy.py:23-29`) but gives operators no starting baseline.
- **Process-local AG-UI authority**: retention windows and capacity caps protect the server, but authority lives in-process (`_approval_lifecycle.py:237`), so horizontal scaling needs a shared backing store the framework does not provide here.

## Failure Modes / Edge Cases

Covered by explicit regression tests (all paths cited):

- Forged standing-approval responses are dropped because they bind to no pending request (`test_harness_tool_approval.py:708-742`).
- Caller-supplied hosted metadata cannot choose the standing-rule boundary (`test_harness_tool_approval.py:745-798`).
- Replay attacks: same call id reused for repeated call, different function, changed arguments, escalated security label, or different session all fail (`test_security.py:814-1163`); mismatched response bodies and missing identifiers rejected (`951-1062`); truthy-but-non-boolean `approved` treated as rejection (`1394+`, also `python/packages/core/agent_framework/_types.py:1459-1461`).
- Registry drift: if the recorded tool disappears before resume, nothing executes (`test_harness_tool_approval.py:842-880`); same-name upgrades may proceed deliberately (`799-841`).
- Budget interactions: auto-approved rounds share the function-call budget and resolve correctly after exhaustion (`test_harness_tool_approval.py:987-1089`).
- Abandoned authority: a newly surfaced batch replaces stale pending requests (`python/packages/core/agent_framework/_tools.py:2141-2179`; spec `docs/specs/004-python-function-calling-loop.md:464`).
- Known residual risks acknowledged in-code: synchronous tool bodies already running in worker threads cannot be cancelled when a batch fails (`python/packages/core/agent_framework/_tools.py:1865-1874`); name-collision auto-approval (above); indefinite standing rules absent TTL.

## Future Considerations

- Add TTL/expiry and a public revocation API to `ToolApprovalState` standing rules (e.g., rule-level `expires_at`, `remove_rule`), mirroring what the AG-UI lifecycle already does (`python/packages/core/agent_framework/_harness/_tool_approval.py:158-155` area vs `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:242-260`).
- Record approver identity/attribution (subject, timestamp, channel) on `function_approval_response.metadata` to support audit and delegated-approval policies; today only an optional free-text `reason` exists (`python/packages/core/agent_framework/_harness/_tool_approval.py:239-245`).
- Scope auto-approval rules by stable tool identity (registration object or capability tag) rather than bare name, eliminating the documented collision class (`python/packages/core/agent_framework/_harness/_file_access.py:1375-1382`).
- Provide a reference distributed backing store for the AG-UI `ApprovalLifecycle` (currently process-local) for horizontally scaled hosts (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:236-237`).
- Bring the Go stack to parity; currently no approval primitives exist under `go/`.

## Questions / Gaps

- **Who approved?** No evidence found of approver identity capture anywhere in the approval path. Searched: `approver|approved_by|user_id` across `_tool_approval.py`, `_approval_lifecycle.py`, `security.py`, `_executor.py`.
- **Standing-rule revocation in core**: no API or test evidence of rule removal short of clearing session state; searched `revoke|remove_rule|delete.*rule|expir` in `_tool_approval.py`. If intended, it is undocumented in the studied tree.
- **Go implementation coverage**: `grep -ril approval go --include=*.go` returned no files, so dimension coverage for Go is "absent" as of this snapshot; the study could not verify whether approval support is planned there.
- **Cross-source filesystem access**: none performed; analysis confined to `studies/agent-harness-study/sources/agent-framework/`. Generated reports outside this source were not inspected.

---

Generated by `08.02-permission-policy-and-approval-gates` against `agent-framework`.
