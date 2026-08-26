# Source Analysis: agent-framework

## Dimension 09.02 — Risk Taxonomy and Control Mapping

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#) monorepo; Go stub. Agent framework (Microsoft) with core runtime, provider packages, hosting, and governance integrations |
| Analyzed | 2026-08-26 |

> Citation convention: all paths below are workspace-relative under
> `studies/agent-harness-study/sources/agent-framework/`. Line numbers refer to the files as checked out in this study snapshot.

## Summary

Agent Framework does not maintain a single, unified "risk taxonomy" type or registry. Instead, risk naming and control mapping are spread across four partially overlapping layers:

1. **A binary approval taxonomy at the tool layer.** `ApprovalMode = Literal["always_require", "never_require"]` (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_tools.py:106`) is the framework-wide per-tool risk switch. The function-invocation loop builds `approval_tool_names` from `tool.approval_mode == "always_require"` and pauses the whole batch for human approval before executing anything (`python/packages/core/agent_framework/_tools.py:1763`, classification loop at `_tools.py:1775-1795`, approval-request emission via `Content.from_function_approval_request` at `_tools.py:1805-1832`). MCP adds a third shape: a per-tool-name mapping `MCPSpecificApproval` with `always_require_approval` / `never_require_approval` name collections (`python/packages/core/agent_framework/_mcp.py:77-88`).

2. **An experimental information-flow risk model (FIDES)** in the public `security.py` module: `IntegrityLabel {TRUSTED, UNTRUSTED}` (`python/packages/core/agent_framework/security.py:92-105`) and `ConfidentialityLabel {PUBLIC, PRIVATE, USER_IDENTITY}` (`security.py:109-124`), combined most-restrictively by `combine_labels` (`security.py:212-264`) and checked by `check_confidentiality_allowed` (`security.py:267-319`). Two named policy-violation categories are produced at runtime — `"untrusted_context"` (`security.py:2008-2024`) and `"max_allowed_confidentiality"` (data-exfiltration; `security.py:2150-2157`) — each mapped to one of three controls: block, user approval, or warn-and-continue (`PolicyEnforcementFunctionMiddleware.process`, `security.py:2065-2095`). Every class here carries `@experimental(feature_id=ExperimentalFeature.FIDES)` (e.g., `security.py:91`; enum member at `python/packages/core/agent_framework/_feature_stage.py:57`).

3. **Provider-scoped category→control conventions** in the harness: file-access tools split into read-only vs write categories with separate approval defaults (`python/packages/core/agent_framework/_harness/_file_access.py:1444-1445`), skills tools default every operation to `always_require` with static auto-approval rules as the documented opt-down path (`python/packages/core/agent_framework/_skills.py:2434-2444`, rules at `_skills.py:1965-2035`), and Hyperlight CodeAct *escalates* `execute_code` approval if any registered tool requires it (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:335-346`; .NET mirror `dotnet/src/Microsoft.Agents.AI.Hyperlight/CodeActApprovalMode.cs:11-31`).

4. **Externalized governance risk categories** in the Purview package: `Activity` upload/download enums (`python/packages/purview/agent_framework_purview/_models.py:21-39`), `DlpAction.BLOCK_ACCESS` and `RestrictionAction.BLOCK` (`_models.py:154-161`), translated into a prompt/response block decision (`python/packages/purview/agent_framework_purview/_processor.py:94-97`) enforced by raising `MiddlewareTermination` (`python/packages/purview/agent_framework_purview/_middleware.py:84-97`).

The mapping from risk to control is therefore real but fragmented: an operator can answer "does this tool require approval?" from tool metadata, but cannot ask a single component "what risks apply to this action and which controls cover them?" — that answer is assembled ad hoc across `_tools.py`, `security.py`, provider tool factories, MCP guardrails, and Purview middleware.

## Rating

**6 / 10** — Present but inconsistent and weakly unified.

The mechanisms that exist are implemented carefully: the approval gate sits before execution in the loop (`python/packages/core/agent_framework/_tools.py:1775-1832`), policy approvals are bound to call body + label + session + disclosed-violation set with consume-on-use semantics (`python/packages/core/agent_framework/security.py:1624-1639`, replay tests at `python/packages/core/tests/test_security.py:814-903`), and known bypass/failure modes are documented inline rather than hidden. But there is no shared risk vocabulary across layers: `ApprovalMode` is binary, FIDES labels/violations are experimental and only two violation types exist, Purview uses its own external enum set, and the category→control mappings live as conventions inside each provider. Nothing composes them into a single queryable control map.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Binary approval taxonomy | `ApprovalMode: TypeAlias = Literal["always_require", "never_require"]` on every tool | `python/packages/core/agent_framework/_tools.py:106` |
| Per-tool risk declaration | `approval_mode` constructor arg on `FunctionTool` / `@tool` decorator | `python/packages/core/agent_framework/_tools.py:316`, `1142-1173` |
| Runtime enforcement point | Loop collects `approval_tool_names = {name: tool.approval_mode == "always_require"}` then classifies batch | `python/packages/core/agent_framework/_tools.py:1763`, `1775-1795` |
| Control = HITL pause | Batch paused; `Content.from_function_approval_request(...)` emitted; session state stored | `python/packages/core/agent_framework/_tools.py:1796-1832` |
| Approval binding helpers | `_is_hosted_tool_approval`, `_bind_approval_response_to_pending_request` | `python/packages/core/agent_framework/_tools.py:1962`, `2182` |
| Session-backed standing rules | `ToolApprovalRule(tool_name, arguments, server_label)`; `ToolApprovalState` persisted in session | `python/packages/core/agent_framework/_harness/_tool_approval.py:86-155`, `158-215` |
| Standing-rule scopes | `create_always_approve_tool_response` / `..._with_arguments_response`; scope literals `"tool" \| "tool_with_arguments"` | `python/packages/core/agent_framework/_harness/_tool_approval.py:28-37`, `218-245` |
| Auto-approval heuristic control | `ToolApprovalMiddleware(auto_approval_rules=[...])` with explicit name-collision security warning | `python/packages/core/agent_framework/_harness/_tool_approval.py:343-379` |
| Integrity risk enum (experimental) | `IntegrityLabel.TRUSTED/.UNTRUSTED`, `@experimental(FIDES)` | `python/packages/core/agent_framework/security.py:91-105` |
| Confidentiality risk enum | `ConfidentialityLabel.PUBLIC/.PRIVATE/.USER_IDENTITY` with ordering hierarchy | `python/packages/core/agent_framework/security.py:108-124`, `313-319` |
| Label join (most restrictive wins) | `combine_labels`: UNTRUSTED taints; confidentiality takes max | `python/packages/core/agent_framework/security.py:212-264` |
| Exfiltration check primitive | `check_confidentiality_allowed(context_label, max_allowed)` hierarchy PUBLIC(0)<PRIVATE(1)<USER_IDENTITY(2) | `python/packages/core/agent_framework/security.py:267-319` |
| Per-tool risk metadata keys | Tools declare `source_integrity`, `accepts_untrusted`, `max_allowed_confidentiality` via `additional_properties` | `python/packages/core/agent_framework/security.py:1090-1113`, `2134-2161` |
| Named violation type #1 | `"untrusted_context"` when context tainted and tool not allow-listed/opted-in | `python/packages/core/agent_framework/security.py:2000-2024` |
| Named violation type #2 | `"max_allowed_confidentiality"` data-exfiltration violation | `python/packages/core/agent_framework/security.py:2142-2157` |
| Risk→control dispatch | approve-consume → request-approval → block → warn ladder | `python/packages/core/agent_framework/security.py:2061-2095` |
| Violation audit log | `_log_violation`, `get_audit_log()`, `clear_audit_log()` | `python/packages/core/agent_framework/security.py:2163-2184` |
| Approval binding record | `_PendingPolicyApproval(body_signature, label_key, session_key, disclosed_violations)` — approval bound to exactly disclosed risks | `python/packages/core/agent_framework/security.py:1624-1639`, `1782-1793` |
| Bundled disclosure rule | All detected violations surfaced in ONE approval request so approval cannot wave undisclosed risks | `python/packages/core/agent_framework/security.py:1889-1924` |
| Secure-agent bundle | `SecureAgentConfig` injects label tracker + policy enforcer + quarantine tools as context provider | `python/packages/core/agent_framework/security.py:2187-2351` |
| Quarantined LLM tool risk metadata | `quarantined_llm` declares `accepts_untrusted=True`, `source_integrity="untrusted"`, `confidentiality="private"` | `python/packages/core/agent_framework/security.py:2547-2556` |
| File-access risk categories | read-only vs write tools get distinct `ApprovalMode` (`disable_readonly_tool_approval` / `disable_write_tool_approval`) | `python/packages/core/agent_framework/_harness/_file_access.py:1444-1445` |
| File-access auto-approve rules | `read_only_tools_auto_approval_rule` approves read/ls/grep only; `all_tools_auto_approval_rule` approves all; both reject `server_label` hosted calls | `python/packages/core/agent_framework/_harness/_file_access.py:1350-1357`, `1360-1395`, `1397-1432` |
| Documented bypass: name collisions | Warning that any local tool sharing a reserved name (e.g., caller-named shell tool) gets auto-approved | `python/packages/core/agent_framework/_harness/_file_access.py:1411-1420`; same warning `python/packages/core/agent_framework/_harness/_tool_approval.py:365-376` |
| Skills default-deny | All three skill tools registered `always_require` by default; opt-down via `disable_*_approval` kwargs → `never_require` | `python/packages/core/agent_framework/_skills.py:2434-2444`, `2488`, `2501`, `2518` |
| Skills auto-approval rules | `read_only_tools_auto_approval_rule` still prompts for `run_skill_script`; `all_tools_auto_approval_rule` covers script execution | `python/packages/core/agent_framework/_skills.py:1965-2001`, `2003-2035` |
| MCP per-tool approval map | `MCPSpecificApproval{always_require_approval, never_require_approval}` name lists | `python/packages/core/agent_framework/_mcp.py:77-88` |
| MCP untrusted-server guardrails | Sampling deny-by-default; `_DEFAULT_SAMPLING_MAX_TOKENS=4096`, `_DEFAULT_SAMPLING_MAX_REQUESTS=25`; denial logged WARNING | `python/packages/core/agent_framework/_mcp.py:130-138`, `1435-1461` |
| MCP argument allowlist + denylist | kwargs filtered to declared schema properties; `_MCP_FRAMEWORK_DENYLIST` drops internal objects | `python/packages/core/agent_framework/_mcp.py:104-125` |
| CodeAct approval escalation (Python) | `_resolve_execute_code_approval_mode`: `execute_code` becomes `always_require` if ANY registered tool requires approval | `python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:335-346` |
| CodeAct approval escalation (.NET) | `CodeActApprovalMode { AlwaysRequire, NeverRequire }` where NeverRequire derives from registry containing `ApprovalRequiredAIFunction` | `dotnet/src/Microsoft.Agents.AI.Hyperlight/CodeActApprovalMode.cs:11-31`; computation `dotnet/src/Microsoft.Agents.AI.Hyperlight/HyperlightCodeActProvider.cs:313-315` |
| Purview governance activity taxonomy | `Activity {UPLOAD_TEXT, UPLOAD_FILE, DOWNLOAD_TEXT, DOWNLOAD_FILE}`, `ProtectionScopeActivities` flag enum | `python/packages/purview/agent_framework_purview/_models.py:21-60` |
| Purview control actions | `DlpAction.BLOCK_ACCESS`, `RestrictionAction.BLOCK` mapped from policy scopes | `python/packages/purview/agent_framework_purview/_models.py:154-161` |
| Purview enforcement point | Prompt pre-check and response post-check; block returns canned message + `MiddlewareTermination` | `python/packages/purview/agent_framework_purview/_middleware.py:71-120`; block decision `python/packages/purview/agent_framework_purview/_processor.py:84-99` |
| Purview fail-open config | Exceptions swallowed unless `ignore_exceptions=False` (default raises); `ignore_payment_required` similar | `python/packages/purview/agent_framework_purview/_middleware.py:100-107` |
| Unknown-call fail-closed option | `terminate_on_unknown_calls` raises on calls not in tool map | `python/packages/core/agent_framework/_tools.py:1384`, `1794-1795` |
| Runtime exposure of approvals | DevUI maps `function_approval_request/response` content → UI events `response.function_approval.requested/responded` | `python/packages/devui/agent_framework_devui/_mapper.py:188-189`, `1778-1818` |
| HITL-safe loop escape | `_has_pending_approval_request` stops loops when `type == "function_approval_request"` present | `python/packages/core/agent_framework/_harness/_loop.py:450` (docstring), implementation checks content type |
| Test coverage: label/policy | 3,837-line suite incl. replay-binding attacks (replayed call id, different function, changed arguments rejected) | `python/packages/core/tests/test_security.py:814-903` |
| Test coverage: approval harness | Session-backed rules, queues, mixed batches | `python/packages/core/tests/core/test_harness_tool_approval.py` (1,428 lines) |
| Test coverage: file-access modes | Asserts per-tool `approval_mode == "always_require"/"never_require"` after disable flags | `python/packages/core/tests/core/test_harness_file_access.py:567-605` |
| Test coverage: MCP approvals | `approval_mode={"always_require_approval": [...]}` mapping, normalized-name collision rejection | `python/packages/core/tests/core/test_mcp.py:108-237`, `1569-1640` |
| Stated design goals (docs) | TRANSPARENCY_FAQ names misuse, unintended consequences of tool execution, privacy, accountability risks; prescribes HITL mitigation | `TRANSPARENCY_FAQ.md:41-50` |

## Answers to Dimension Questions

**1. Are risks named and categorized?**
Partially, in several disjoint vocabularies rather than one taxonomy:
- Tool-level: a two-valued `ApprovalMode` literal (`python/packages/core/agent_framework/_tools.py:106`) — this says *whether* a tool needs a human, not *what* risk it poses.
- Content-level (experimental): integrity `{TRUSTED, UNTRUSTED}` and confidentiality `{PUBLIC, PRIVATE, USER_IDENTITY}` enums (`python/packages/core/agent_framework/security.py:92-124`) plus two named violation categories `"untrusted_context"` and `"max_allowed_confidentiality"` (`security.py:2010`, `2152`). This is the closest thing to a genuine risk taxonomy, and it is explicitly marked experimental (`security.py:91`; `ExperimentalFeature.FIDES` at `python/packages/core/agent_framework/_feature_stage.py:57`).
- Governance-level: Purview's `Activity`/`DlpAction`/`RestrictionAction` enums (`python/packages/purview/agent_framework_purview/_models.py:21-39`, `154-161`).
- Docs-level prose risks without code anchors: `TRANSPARENCY_FAQ.md:41-50`.
No single enum or registry unifies these. There is no "risk severity" scale anywhere in either language (grep for `Risk` in `dotnet/src` yields only doc-comment mentions, e.g., `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:330`).

**2. Is every risk mapped to a control?**
Within each subsystem, yes-by-construction; across the framework, no central guarantee.
- Approval-required tools → HITL pause control (`python/packages/core/agent_framework/_tools.py:1787-1832`); standing/auto rules are the documented relaxation controls (`python/packages/core/agent_framework/_harness/_tool_approval.py:86-118`, `351-379`).
- Untrusted-content risk → label tracking, variable-indirection hiding, quarantined LLM (`python/packages/core/agent_framework/security.py:689-767`, `2538-2606`); exfiltration risk → `max_allowed_confidentiality` check (`security.py:2142-2157`).
- Untrusted MCP server → deny-by-default sampling callback, rate/token caps, argument allowlist, `allowed_tools` filter (`python/packages/core/agent_framework/_mcp.py:130-144`, `115-125`).
- Guest-code-reaches-gated-tool risk → CodeAct approval escalation derived from the tool registry (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:340-346`).
- Policy/DLP violations → block/approve/warn ladder and Purview termination (`security.py:2061-2095`; `python/packages/purview/agent_framework_purview/_middleware.py:84-97`).
But the mapping lives implicitly inside each feature; there is no manifest that an operator could query to enumerate "risk X is covered by control Y at enforcement point Z."

**3. Can risks be assessed at runtime?**
Yes, at three granularities, mostly per-tool/per-call rather than per-agent:
- **Per-tool**: static `approval_mode` resolved at registration and re-checked on every batch (`python/packages/core/agent_framework/_tools.py:1763`); dynamic escalation exists only in CodeAct, where the mode is recomputed whenever the guest-visible tool registry changes (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:1305-1336`).
- **Per-action**: policy middleware evaluates the cumulative context label against the specific call's name+arguments, binds approvals to the exact invocation body, label, session, and disclosed violation set (`python/packages/core/agent_framework/security.py:1733-1748`, `1824-1862`), and consumes approvals once (`security.py:1864-1870`). Argument-scoped standing rules ("tool_with_arguments") similarly match exact serialized arguments (`python/packages/core/agent_framework/_harness/_tool_approval.py:46-58`).
- **Per-agent**: no evidence found. No per-agent risk scoring, budget, or posture concept was located (searches: `risk`, `Risk`, `severity`, `posture` across `python/packages/**` and `dotnet/src` returned only doc comments and unrelated sample code).
Runtime exposure of risk metadata to hosts/UIs is real: approval requests carry `additional_properties` such as `policy_violation`, `violation_type`, `violations[]`, `context_label` (`security.py:1904-1922`), and DevUI converts them into `response.function_approval.requested` events (`python/packages/devui/agent_framework_devui/_mapper.py:1778-1812`).

**4. Can controls be bypassed?**
Several documented and structural weaknesses:
- **Auto-approval name collisions (documented).** Auto-approval rules match by local tool name; a colliding caller-configured tool (e.g., shell) is silently auto-approved, and the code warns callers to avoid collisions instead of enforcing uniqueness (`python/packages/core/agent_framework/_harness/_file_access.py:1411-1420`; `python/packages/core/agent_framework/_harness/_tool_approval.py:365-376`).
- **Policy enforcement fail-open when unwired.** If `LabelTrackingFunctionMiddleware` did not run first, `PolicyEnforcementFunctionMiddleware` logs a warning and continues execution without any policy check (`python/packages/core/agent_framework/security.py:1969-1976`).
- **Purview fail-open switches.** Pre/post-check errors are swallowed when `ignore_exceptions` is set, and `PurviewPaymentRequiredError` is ignorable (`python/packages/purview/agent_framework_purview/_middleware.py:100-107`) — governance blocks can be turned off by configuration.
- **Opt-out constructors.** `disable_*_approval=True` flips file/skill tools to `never_require` (`python/packages/core/agent_framework/_harness/_file_access.py:1444-1445`; `python/packages/core/agent_framework/_skills.py:2434-2444`); MCP sampling legacy auto-approve is one lambda away (`lambda params: True`, described at `python/packages/core/agent_framework/_mcp.py:1435-1461`). These are deliberate trust knobs, but they mean the default-deny posture is advisory, not enforced.
- **What is NOT bypassable:** the core approval gate itself runs before execution in the invocation loop and treats approval responses as immutable resume boundaries with occurrence-aware correlation (`python/packages/core/agent_framework/_tools.py:1775-1832`, `2182`; spec-driven design noted at `docs/specs/004-python-function-calling-loop.md:30`), and the FIDES approval replay attacks (reuse call id, swap function, change args, changed label/session/violation-set) are explicitly tested and rejected (`python/packages/core/tests/test_security.py:814-903`).

## Architectural Decisions

1. **Risk expressed as tool metadata, not a central service.** `approval_mode` travels on the `FunctionTool` object (`python/packages/core/agent_framework/_tools.py:106`, `316`) and providers assemble their own defaults, keeping the core loop generic: it merely reads `tool.approval_mode` (`_tools.py:1763`). Consequence: no global view; each provider is a mini-policy-authority.
2. **Information-flow control (FIDES) as an opt-in experimental layer.** Labels propagate through a dedicated `FunctionMiddleware` with a strict 3-tier priority (embedded result label > tool `source_integrity` declaration > input-label join) (`python/packages/core/agent_framework/security.py:692-718`, `1178-1219`), and enforcement is a second middleware reading `context.metadata["context_label"]` (`security.py:1952-1991`). Middleware composition order is load-bearing and manually verified (`security.py:1970-1973`).
3. **Human approval as the universal backstop control.** Everything else (auto-rules, standing rules, policy approvals) compiles down to the same `function_approval_request`/`function_approval_response` content pair consumed by the loop (`python/packages/core/agent_framework/_tools.py:1805`; `security.py:1919-1924`), giving one wire format across plain tools, hosted MCP tools, and policy violations.
4. **Deny-by-default for untrusted boundaries.** Remote MCP sampling denied unless a callback approves (`python/packages/core/agent_framework/_mcp.py:131-137`, `1435-1461`); skill and file-write tools require approval unless explicitly disabled (`python/packages/core/agent_framework/_skills.py:2434-2444`); CodeAct escalates rather than de-escalates (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:340-346`).
5. **Governance delegated to enterprise service.** DLP/exfiltration policy evaluation is outsourced to Microsoft Purview with caching and scoped content processing (`python/packages/purview/agent_framework_purview/_middleware.py:40-48`), keeping the framework free of built-in content-risk classifiers.

## Notable Patterns

- **Category→rule factories**: providers expose static predicate rules paired with their tool sets (`FileAccessProvider.read_only_tools_auto_approval_rule`, `SkillsProvider.read_only_tools_auto_approval_rule`), so the risk-category knowledge (read vs write vs script-execution) is encoded next to the tool definitions (`python/packages/core/agent_framework/_harness/_file_access.py:1360-1432`; `python/packages/core/agent_framework/_skills.py:1965-2035`).
- **Hosted/local boundary marker**: `server_label` in `additional_properties` partitions hosted-tool approvals from local ones; auto-rules refuse same-named hosted calls (`python/packages/core/agent_framework/_harness/_file_access.py:1349-1357`).
- **Binding-record pattern for approvals**: `_PendingPolicyApproval` captures every authorization dimension including canonicalized disclosed risks (`f"{violation_type}\x00{reason}"` tuples) so "approved for one risk set ≠ approved for another" (`python/packages/core/agent_framework/security.py:1771-1793`).
- **Variable indirection as containment**: untrusted results are replaced by `[var_xxx]` placeholders expanded only at tool-execution time, with bracketed/bare-form leniency logged loudly (`python/packages/core/agent_framework/security.py:858-989`).
- **Cross-language parity of control semantics**: the .NET harness mirrors the Python approval/file-access/CodeAct designs (`dotnet/src/Microsoft.Agents.AI/Harness/FileAccess/FileAccessProvider.cs:461-476`; `dotnet/src/Microsoft.Agents.AI.Hyperlight/HyperlightCodeActProvider.cs:288-315`).

## Tradeoffs

- **Fragmentation vs flexibility**: each subsystem picks the risk vocabulary it needs (binary mode, IFC labels, DLP actions). Integration cost falls on application developers who must compose `ToolApprovalMiddleware` + `LabelTrackingFunctionMiddleware` + `PolicyEnforcementFunctionMiddleware` (+ optionally Purview) themselves (`python/packages/core/agent_framework/security.py:2272-2300` shows the intended wiring).
- **Binary `ApprovalMode` loses nuance**: "this tool touches PII in untrusted contexts" cannot be expressed; everything collapses to always/never (`python/packages/core/agent_framework/_tools.py:106`). The richer expression only exists behind the experimental FIDES flag.
- **Documentation-as-enforcement**: collision warnings and fail-open caveats are docstrings, not runtime invariants (`python/packages/core/agent_framework/_harness/_tool_approval.py:365-376`).
- **Experimental maturity of the only true taxonomy**: `IntegrityLabel`/`ConfidentialityLabel`/policy middleware can change or disappear without notice by the project's own stage contract (`python/packages/core/agent_framework/_feature_stage.py:43-51`).
- **Fail-open governance defaults trade availability for safety inversely**: Purview's error swallowing is configurable but payment-required errors being dismissible means a billing lapse silently removes DLP blocking unless operators opt to hard-fail (`python/packages/purview/agent_framework_purview/_middleware.py:100-107`).

## Failure Modes / Edge Cases

- **Silent auto-approval via name shadowing** — a caller-named shell tool registering `file_access_read` would be waved through by the read-only rule; only docstring warnings protect against this (`python/packages/core/agent_framework/_harness/_file_access.py:1411-1420`).
- **Missing label pipeline disables policy** — reordering middleware (or attaching the policy enforcer alone) makes every call pass unchecked with only a log line (`python/packages/core/agent_framework/security.py:1969-1976`).
- **Session-less runs skip session-backed safe-sibling logic** — without an `AgentSession`, all approval requests in a batch become visible prompts (`python/packages/core/agent_framework/_tools.py:1822-1824`), changing operator experience between deployments.
- **Global quarantine client last-writer-wins** — multiple `SecureAgentConfig` instances in one process share a process-global quarantine client slot, so per-agent isolation of the quarantined channel is not guaranteed (`python/packages/core/agent_framework/security.py:2200-2208`, `2396-2429`).
- **MCP server-controlled allowlist widening** — the argument allowlist is derived from the server's own schema, so a server declaring extra properties widens what flows through regardless of caller intent (documented at package level; mechanism `python/packages/core/agent_framework/_mcp.py:104-125`).
- **Unknown-function handling is off by default** — `terminate_on_unknown_calls` defaults to `False` (`python/packages/core/agent_framework/_tools.py:1397`), so hallucinated tool names surface as model-visible errors rather than hard failures unless opted in (`_tools.py:1794-1795`).

## Future Considerations

- Promote the FIDES violation categories (`untrusted_context`, `max_allowed_confidentiality`) out of experimental status and unify them with `ApprovalMode` so a single descriptor can express "untrusted-source tool writing PRIVATE data requires approval."
- Add a queryable control map (e.g., per-agent inventory of tools × applicable controls × enforcement points) so operators can answer the dimension's titular question programmatically.
- Enforce tool-name reservation/uniqueness for auto-approved rule namespaces at registration time instead of relying on docstring warnings.
- Make middleware-order prerequisites explicit (policy enforcer asserting the presence of an upstream label tracker) rather than warning-and-passing (`python/packages/core/agent_framework/security.py:1969-1976`).
- Extend the CodeAct-style registry-derived escalation pattern to other composite surfaces (e.g., skills scripts already do this statically; dynamic tool injection paths could reuse `_resolve_execute_code_approval_mode` semantics).

## Questions / Gaps

- **No per-agent risk assessment**: searches for per-agent risk scores, autonomy budgets, or posture models found nothing in `python/packages/**` or `dotnet/src` (terms tried: `risk`, `Risk`, `severity`, `posture`, `budget` in security contexts). The only budgets found are invocation-count caps (`python/packages/core/agent_framework/_tools.py:95-104`), which bound cost, not risk.
- **No severity ranking**: neither language defines ordered risk levels; the only hierarchies are the confidentiality ordering (`security.py:313-319`) and enum ordinals.
- **Go implementation absent**: the `go/` directory contains only a README (`go/README.md`), so no additional taxonomy surface exists there.
- **.NET has no code-level risk taxonomy**: grep across `dotnet/src` for `Risk`/`risk` produced only XML-doc security considerations (e.g., `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:76`); the FIDES module appears Python-only. Whether parity is planned is undocumented in-repo (no ADR found; `docs/decisions/` indexes reviewed via filenames only).
- **Purview risk categories are opaque**: `ProtectionScope`/`policy_actions` payloads come from the external service (`python/packages/purview/agent_framework_purview/_models.py:875-973`); the framework does not model *why* something is risky, only whether to block.

---

Generated by `dimensions/09.02-risk-taxonomy-control-mapping` against `agent-framework`.
