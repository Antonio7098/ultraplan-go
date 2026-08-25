# Source Analysis: agent-framework

## Security Auditability (Dimension 08.04)

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (core packages) + .NET (Microsoft.Agents.AI); OpenTelemetry for observability |
| Analyzed | 2026-08-24 |

## Summary

Agent Framework approaches security auditability through three distinct layers, none of which forms a complete audit trail by itself:

1. **Human-approval control plane** — `function_approval_request` / `function_approval_response` content types (`sources/agent-framework/python/packages/core/agent_framework/_types.py:1273`, `:1296`) flow through a middleware-based approval system (`ToolApprovalMiddleware` in `sources/agent-framework/python/packages/core/agent_framework/_harness/_tool_approval.py:343`) whose state is serializable and persisted in session state (`sources/agent-framework/python/packages/core/agent_framework/_tools.py:2075`). The approval machinery is strongly tamper-resistant (request/response binding, strict boolean decisions, replay protection) and heavily tested, but approvals are **not explicitly logged** — the design ADR states "We should however log approvals so that there is a trace of this for debugging and auditing purposes" (`sources/agent-framework/docs/decisions/0006-userapproval.md:95`), yet the approval middleware contains no logging calls at all; auditability relies implicitly on history providers retaining approval wrappers ("History providers may retain approval control contents in their backing store for audit", `sources/agent-framework/docs/specs/004-python-function-calling-loop.md:409`).
2. **Policy enforcement with an in-memory audit log** — The experimental FIDES security module (`PolicyEnforcementFunctionMiddleware`) records structured violation records (type, function, context label, turn number, reason) into a plain in-memory list (`sources/agent-framework/python/packages/core/agent_framework/security.py:1698`, `:2163-2184`). Records carry **no timestamp and no actor identity** — the code explicitly notes "there is no separate user identity here" (`sources/agent-framework/python/packages/core/agent_framework/security.py:1631`).
3. **OpenTelemetry tracing of capability usage** — Every tool execution emits an OTel span with tool name, tool call id, error type, and duration (`sources/agent-framework/python/packages/core/agent_framework/observability.py:2284-2302`, wired at `sources/agent-framework/python/packages/core/agent_framework/_tools.py:733-801`), plus `invoke_agent` spans on .NET (`sources/agent-framework/dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgent.cs:192-212`). Arguments/results are only captured when sensitive-data telemetry is explicitly enabled.

Net assessment: the framework can answer "which tool ran under which span/conversation" well, and "what policy was violated" coarsely (in-process only). It cannot durably answer "who approved what, when, from where" without application-side work. The system partially defends a risky action after the fact, but reconstruction requires correlating ephemeral in-memory logs, session-state snapshots, and externally exported traces.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, or fragile.**

Rationale: A real audit surface exists (structured FIDES violation records with tests at `sources/agent-framework/python/packages/core/tests/test_security.py:613`; OTel tool-execution spans; session-persisted approval state), but the trail is fragile: the dedicated audit log is process-memory-only with no timestamps or actor identity (`sources/agent-framework/python/packages/core/agent_framework/security.py:1698`), approval decisions are never explicitly logged despite the ADR's stated intent (`sources/agent-framework/docs/decisions/0006-userapproval.md:95`), there are no policy decision IDs, and no export path (sink, event bus, or durable store) exists for any security event.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Security event log (FIDES violations) | `PolicyEnforcementFunctionMiddleware.audit_log: list[dict]` populated per violation; `enable_audit_log=True` default | sources/agent-framework/python/packages/core/agent_framework/security.py:1678,1698 |
| Violation record schema | `{type, function, context_label, turn, reason}` (untrusted) and `{type, subtype, function, context_label, reason, turn}` (confidentiality) | sources/agent-framework/python/packages/core/agent_framework/security.py:2017-2023,2038-2045 |
| Audit log accessors | `get_audit_log()` returns copy; `clear_audit_log()` empties it (in-memory only) | sources/agent-framework/python/packages/core/agent_framework/security.py:2174-2184 |
| Approval request/response content types | `Content.from_function_approval_request(...)` / `from_function_approval_response(approved=...)`; deserialization coerces non-boolean `approved` to False | sources/agent-framework/python/packages/core/agent_framework/_types.py:1273-1307,1460-1461 |
| Approval state persistence | `ToolApprovalState` (rules, queued requests, collected responses) serialized into `session.state["tool_approval"]` | sources/agent-framework/python/packages/core/agent_framework/_harness/_tool_approval.py:158-215,248-275; _tools.py:98,2075-2095 |
| Standing-approval rules carry optional reason | `_create_always_approve_response` stores `reason` in `additional_properties` | sources/agent-framework/python/packages/core/agent_framework/_harness/_tool_approval.py:239-245 |
| Approval binding / anti-replay | Pending requests stored as immutable snapshots keyed by id; responses rebound to recorded request; unknown/duplicate ids rejected and logged | sources/agent-framework/python/packages/core/agent_framework/_tools.py:2109-2157,2182-2213,2237-2240 |
| Policy-violation approvals bound to disclosed risk | `_PendingPolicyApproval(body_signature, label_key, session_key, disclosed_violations)`; consume-on-use; explicit "no separate user identity" note | sources/agent-framework/python/packages/core/agent_framework/security.py:1624-1639,2061-2076 |
| Human-involvement marker on replay | Approved replays inject warning "APPROVED BY USER: ... User acknowledged the security risk" | sources/agent-framework/python/packages/core/agent_framework/security.py:2069-2076 |
| Tool-execution tracing (capability usage) | Span attributes `gen_ai.operation.name=execute_tool`, `TOOL_NAME`, `TOOL_CALL_ID`, `ERROR_TYPE`, duration histogram | sources/agent-framework/python/packages/core/agent_framework/observability.py:2284-2302,2787; _tools.py:758-801 |
| Sensitive-data gating on args/results | Tool arguments emitted only when semconv ≥ v1.36 flag set (`emit_tool_call_attributes`); results only when `SENSITIVE_DATA_ENABLED` | sources/agent-framework/python/packages/core/agent_framework/_tools.py:749-757,761-762,777-781,791-795; observability.py:875-885,928-937 |
| Agent-level spans (.NET) | `invoke_agent {Name}({Id})` activity with agent id/name/description tags | sources/agent-framework/dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgent.cs:192-212 |
| MCP sampling denial logging | Deny-by-default sampling gate logs denials at WARNING (content not logged) | sources/agent-framework/python/packages/core/agent_framework/_mcp.py:1440-1461 |
| Shell policy decisions (.NET) | `ShellPolicy.Evaluate(ShellRequest)` → `ShellPolicyOutcome{Allowed, Reason}`; evaluated before execution but result not logged | sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellPolicy.cs:193,56-79; LocalShellExecutor.cs:135 |
| Governance integration (Purview) | `PurviewPolicyMiddleware` pre/post checks log block/error events via logger, no local decision records | sources/agent-framework/python/packages/purview/agent_framework_purview/_middleware.py:24,87-145 |
| Rule-id capture in sample middleware | ATR validation middleware logs matched rule id "included for auditability" | sources/agent-framework/python/samples/02-agents/middleware/atr_validation_middleware.py:120-135 |
| Tests: audit recording & approval integrity | `test_audit_log_recording`; mismatched body rejected; missing identifiers rejected; replay with new violation set requires fresh approval | sources/agent-framework/python/packages/core/tests/test_security.py:613-616,951,1007,1275,1338 |
| Tests: session approval binding/replay | Duplicate request ids rejected; untrusted inbound request history not trusted; persisted approvals replay correctly | sources/agent-framework/python/packages/core/tests/core/test_function_invocation_logic.py:59,131,248,1316,1370 |
| Design intent for approval logging | "We should however log approvals so that there is a trace of this for debugging and auditing purposes." | sources/agent-framework/docs/decisions/0006-userapproval.md:95 |
| Documented audit contract for history | History providers may retain approval wrappers "for audit"; base replay filters them from model input | sources/agent-framework/docs/specs/004-python-function-calling-loop.md:408-409 |

## Answers to Dimension Questions

1. **Who did what?** Only partially answerable. "What" is strong: every tool invocation is traced with tool name + call id (`sources/agent-framework/python/packages/core/agent_framework/observability.py:2294-2299`) and FIDES violations record function name, context label, and turn number (`sources/agent-framework/python/packages/core/agent_framework/security.py:2017-2023`). "Who" is absent: no end-user identity appears anywhere in audit records, spans, or approval state. The closest proxies are session ids (`session_key` in approval bindings, `sources/agent-framework/python/packages/core/agent_framework/security.py:1766-1769`) and the agent name/id tags on .NET spans (`sources/agent-framework/dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgent.cs:203-212`). The code acknowledges this gap directly: "there is no separate user identity here" (`security.py:1631`).

2. **What policy allowed it?** Coarsely. FIDES violations embed the reason string and full context label (`security.py:2020-2022`, `:2042-2044`), and approved replays are marked as user-waived with the disclosed violation set (`security.py:2069-2076`). The .NET shell executor evaluates a deny/allow policy with reasons (`ShellPolicy.cs:193,61-71`), and Purview delegates to an external governance service (`_middleware.py:87-105`). However, there is **no policy decision record format**: no decision IDs, no rule versioning, no durable store. Shell policy outcomes and Purview results are not persisted locally at all (only transient log lines or exceptions).

3. **Was a human involved?** Representable but not logged. Human involvement exists as first-class protocol content (`function_approval_request`/`response` with strict boolean `approved`, `_types.py:1296-1307,1460-1461`), auto-approval bypasses are explicit callbacks (`_tool_approval.py:640-650`), and approved FIDES replays inject an "APPROVED BY USER" marker (`security.py:2071-2075`). But the approval middleware performs **no logging whatsoever** (no logger exists in `_tool_approval.py`), so after the fact a human decision is only recoverable if the host retained history/session state containing the approval contents. The ADR's commitment to log approvals (`docs/decisions/0006-userapproval.md:95`) is not implemented in shipped middleware.

4. **Can auditors reconstruct the decision?** Fragmentarily, and only within one process/session. Reconstruction requires joining: (a) OTel traces exported to an external backend (`observability.py:422,548` configure OTLP exporters), (b) session-store snapshots of `ToolApprovalState` (`_tool_approval.py:248-275`) if the host used a persistent `SessionStore`, and (c) the in-memory FIDES `audit_log`, which dies with the process and can be wiped via `clear_audit_log()` (`security.py:2182-2184`). There is no tamper evidence, no timestamps in FIDES records, and no unified export. An auditor cannot reliably reconstruct a risky action's full decision chain from the framework alone.

## Architectural Decisions

- **Approvals as protocol content, not callbacks** — ADR 0006 chose returning `function_approval_request` to the caller over in-process callbacks specifically to support remote/suspended scenarios (`sources/agent-framework/docs/decisions/0006-userapproval.md:36-58,60-95`). This makes approval records portable and storable, but pushes audit responsibility onto hosts.
- **Audit retention delegated to pluggable storage** — Rather than a built-in audit sink, the spec allows history providers to retain approval control contents "for audit" while filtering them from model replay (`docs/specs/004-python-function-calling-loop.md:408-409`); the docs even sketch a dedicated audit-only provider pattern with `load_messages=False` (`sources/agent-framework/docs/decisions/0016-python-context-middleware.md:1665-1678`).
- **Security events default to privacy-preserving verbosity** — Tool arguments/results are excluded from spans/logs unless sensitive-data mode or newer semantic conventions are enabled (`_tools.py:749-762,777-781`; `observability.py:875-885`), trading forensic completeness against data leakage.
- **Tamper resistance invested in enforcement, not storage** — Binding records, occurrence-aware correlation, consume-on-use approvals, and strict-boolean decisions protect *live* authorization integrity (`security.py:1802-1822`; `_tools.py:2109-2213`), while the historical record remains plain, unsigned, and ephemeral.

## Notable Patterns

- **Structured violation dicts as de facto event schema**: each FIDES check builds an `audit` payload alongside user-facing `block_error`/`approval_reason` fields, separating auditor-facing data from model-facing messages (`security.py:2007-2024,2030-2046`).
- **Disclosure-bound approvals**: an approval only waives exactly the violation set disclosed to the reviewer; a replay tripping a different/worse set re-prompts (`_violation_set_key`, `security.py:1771-1780`; tested at `tests/test_security.py:1275,1338`).
- **Deny-by-default gates with WARNING-level breadcrumbs**: MCP sampling denies by default and logs each denial without content (`_mcp.py:1437-1461`), a consistent pattern of fail-closed + minimal logging.
- **Sample-level audit conventions**: samples demonstrate printing the FIDES audit log (`python/samples/02-agents/security/repo_confidentiality_example.py:320-326`) and embedding matched-rule ids for auditability (`samples/02-agents/middleware/atr_validation_middleware.py:129-131`), signaling intended host practice rather than framework guarantees.

## Tradeoffs

- **Privacy vs. forensics**: sensitive-data-off defaults keep credentials/PⅡ out of traces (`observability.py:875`) but mean an auditor often cannot see *what arguments* a dangerous tool was invoked with unless the deployment opted in.
- **Portability vs. accountability**: caller-owned approval UX (ADR option 3) enables remote HITL but leaves the durable who/when record outside the framework boundary.
- **In-process audit simplicity vs. durability**: the list-based `audit_log` is trivial to inspect in demos but provides no multi-process aggregation, rotation, tamper evidence, or export.
- **Strictness vs. operability in auto-approval rules**: heuristic auto-approval improves usability but carries an explicit documented risk of same-name tools being silently approved across features (`_tool_approval.py:365-376`), which would also silently shrink the human-in-the-loop audit surface.

## Failure Modes / Edge Cases

- **Process restart loses all FIDES policy-violation history** — `audit_log` is instance memory; nothing flushes it anywhere (`security.py:1698,2169-2170`).
- **Silent approval trail when hosts skip history retention** — if no history provider persists approval wrappers (or a caller manually manages history), the approve/reject decision leaves no framework-managed record (`docs/specs/004-python-function-calling-loop.md:408-411`).
- **Auto-approval rule collision silently removes the human** — a colliding tool name gets auto-approved without prompting, and the only trace is a normal successful-tool span (`_tool_approval.py:365-376`; skills variant `python/packages/core/agent_framework/_skills.py:1978-1986`).
- **Non-boolean approval decisions coerce to rejection** — deserialization forces `approved` to a bool, treating malformed responses as denied (`_types.py:1460-1461`), which fails closed but could mask a genuine (malformed) grant.
- **Unlabeled context skips policy checks** — if `context_label` metadata is missing, the policy enforcer logs a warning and executes the tool anyway (`security.py:1969-1976`), an allow-open failure path that produces only a DEBUG/WARNING line.
- **Streaming post-checks skipped** — Purview response post-checks do not run for streaming responses (`purview/agent_framework_purview/_middleware.py:139`), creating unaudited response paths.

## Future Considerations

- Add timestamps, actor/session identity, and monotonically increasing decision IDs to FIDES audit records; emit them as OTel log records or events so existing exporter plumbing (`observability.py:422-548`) can ship them.
- Implement the approval-decision logging promised in `docs/decisions/0006-userapproval.md:95` inside `ToolApprovalMiddleware.process` (approve/deny/auto-approve, requester, standing-rule creation).
- Provide a first-class audit sink interface (e.g., an `AuditProvider` alongside `HistoryProvider`) so the pattern sketched in `0016-python-context-middleware.md:1665-1678` becomes supported API instead of a composition trick.
- Persist `ShellPolicyOutcome` decisions and Purview verdicts into the same record format so cross-language (.NET/Python) audits are comparable.

## Questions / Gaps

- No evidence found of any durable, framework-managed security-event store: searches for audit sinks/export hooks beyond OTLP tracing (`grep -r "audit"` across python packages and dotnet src) surfaced only the in-memory list, docs, and samples described above.
- Whether .NET has parity approval-audit behavior was only spot-checked (`dotnet/src/Microsoft.Agents.AI.AgentHooks/Core/AgentHooksFunctionMiddleware.cs` referenced in listings but approval logging not examined in depth due to scope); the Python core was treated as authoritative for this dimension.
- No evidence found that FIDES audit entries are ever correlated with the OTel `TOOL_CALL_ID` spans, leaving two disjoint identifiers (call_id vs. turn_number) for reconstructing one action.

---

Generated by `dimensions/08.04-security-auditability.md` against `agent-framework`.
