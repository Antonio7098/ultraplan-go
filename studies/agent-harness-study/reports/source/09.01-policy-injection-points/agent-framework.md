# Source Analysis: agent-framework

## Dimension 09.01 — Policy Injection Points

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary implementation), .NET/C# (parallel SDK), declarative YAML agents, Go stub (`go/README.md` only) |
| Analyzed | 2026-08-26 |

## Summary

The framework has no single "policy engine". Governance rules enter the system through **six distinct injection points**, layered by enforcement strength:

1. **Runtime code — tool approval policy**: every tool carries an `approval_mode` ("always_require" / "never_require") declared at tool construction (`python/packages/core/agent_framework/_tools.py:316`, default at `_tools.py:408`); the function-calling loop builds the approval set per batch and pauses execution before invoking any tool in it (`python/packages/core/agent_framework/_tools.py:1763-1790`).
2. **Middleware-enforced information-flow policies (FIDES)**: `PolicyEnforcementFunctionMiddleware` enforces integrity/confidentiality rules pre-execution, configured via constructor allowlists (`python/packages/core/agent_framework/security.py:1643`, config args `security.py:1674-1704`). It is wired into any agent by the `SecureAgentConfig` context provider, which also hard-codes two security tools into the untrusted allowlist (`security.py:2281-2292`).
3. **Tool metadata annotations**: tools self-declare policy constraints in `additional_properties` keys — `source_integrity` (read at `security.py:1090-1111`), `max_allowed_confidentiality` (enforced at `security.py:2142-2159`), and `accepts_untrusted` (checked at `security.py:2002-2006`) — e.g. `@tool(additional_properties={"source_integrity": "trusted"})` (`security.py:733`).
4. **External server metadata**: MCP `ToolAnnotations` hints (`readOnlyHint`, `openWorldHint`) are mapped to FIDES labels by `_map_mcp_security_labels` (`security.py:3018-3096`), applied post-connect by `apply_mcp_security_labels` with per-tool overrides (`security.py:3100-3213`), and server-provided `_meta.ifc` result labels override static labels (`security.py:3224-3283`). Documented as design intent in `docs/decisions/0024-prompt-injection-defense.md:46-49`.
5. **Deployment/config files**: declarative YAML agent manifests declare per-MCP-server `approvalMode` (`declarative-agents/agent-samples/foundry/MicrosoftLearnAgent.yaml:18-21`), converted to hosted-tool approval modes in .NET (`dotnet/src/Microsoft.Agents.AI.Declarative/Extensions/McpServerToolExtensions.cs:29-32`, `McpServerToolApprovalModeExtensions.cs:16-27`).
6. **External policy engine**: Microsoft Purview DLP integration evaluates prompts and responses against cloud-hosted policy (`python/packages/purview/agent_framework_purview/_middleware.py:24-147` agent-level, `:150-249` chat-client level), terminating runs with `MiddlewareTermination` on violation.

Two auxiliary mechanisms round this out: **session-backed standing approval rules** that accumulate at runtime without code changes (`python/packages/core/agent_framework/_harness/_tool_approval.py:86-155,218-245`), and **prompt-based rules** injected as instructions (harness defaults `python/packages/core/agent_framework/_harness/_agent.py:54-79`; security guidance `security.py:2442+`) — the latter being advisory only. A separate regex-based `ShellPolicy` exists for shell commands but is explicitly documented as a UX pre-filter, not a security boundary (`python/packages/tools/agent_framework_tools/shell/_policy.py:5-35`).

## Rating

**6 / 10** — Present and substantially tested, but inconsistent across mechanisms and fragile in places.

Supporting rationale:
- The FIDES subsystem is a clear model with explicit precedence rules, replay-safe approval binding, an ADR (`docs/decisions/0024-prompt-injection-defense.md`), and dedicated tests including adversarial cases (`python/packages/core/tests/test_security.py:527` `TestPolicyEnforcementMiddleware`, replay tests at `:663`, `:732`; cross-middleware tests at `:2202-2252`).
- Fail-safe defaults are common: unlabeled content defaults to UNTRUSTED (`security.py:303-306,3044-3045`), unknown declarative approval modes fall back to AlwaysRequire (`dotnet/src/Microsoft.Agents.AI.Declarative/Extensions/McpServerToolApprovalModeExtensions.cs:27`), shell tools require an explicit `acknowledge_unsafe` flag to disable approval (`python/packages/tools/agent_framework_tools/shell/_tool.py:159-165`).
- Deductions: a fail-open path when the label tracker is missing (`security.py:1969-1976` executes the tool with only a warning); audit logs are in-memory only with no persistence, export, or schema versioning (`security.py:1698,2163-2184`); no externalized policy file format for FIDES or ShellPolicy (all lists are constructor-bound); skill frontmatter `allowed-tools` is parsed but never enforced (`python/packages/core/agent_framework/_skills.py:3477-3478` stores it; no consumer found); auto-approval rules carry a documented name-collision bypass risk rather than a structural fix (`_harness/_tool_approval.py:365-376`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool approval mode declaration | `approval_mode` param on `@tool` decorator; defaults to `"never_require"` when unset | python/packages/core/agent_framework/_tools.py:316,408 |
| Approval enforced in loop | `approval_tool_names` computed from `approval_mode == "always_require"`; batch paused before execution | python/packages/core/agent_framework/_tools.py:1763-1790 |
| FIDES policy middleware | `PolicyEnforcementFunctionMiddleware(allow_untrusted_tools, block_on_violation, enable_audit_log, approval_on_violation)` | python/packages/core/agent_framework/security.py:1643-1704 |
| Integrity rule enforcement | UNTRUSTED context + tool not in allowlist + no `accepts_untrusted` → violation | python/packages/core/agent_framework/security.py:2002-2024 |
| Confidentiality/exfiltration rule | Context confidentiality must be ≤ tool's `max_allowed_confidentiality` | python/packages/core/agent_framework/security.py:2136-2161 |
| Security provider wiring | `SecureAgentConfig.before_run` injects tools/instructions/middleware; force-allows `quarantined_llm`, `inspect_variable` | python/packages/core/agent_framework/security.py:2281-2324 |
| Label precedence (3 tiers) | Tier 1 embedded label > Tier 2 tool `source_integrity` > Tier 3 input-label join | python/packages/core/agent_framework/security.py:692-710,1216-1243 |
| Conflict resolution: most-restrictive-wins | `combine_labels`: any UNTRUSTED → UNTRUSTED; max confidentiality wins | python/packages/core/agent_framework/security.py:212-264 |
| Server metadata → policy | `_map_mcp_annotations_to_labels`: `readOnlyHint=True`→accepts_untrusted; else sink capped at PUBLIC | python/packages/core/agent_framework/security.py:3018-3096 |
| Override precedence | `annotation_overrides` beat derived labels but force `accepts_untrusted=False` | python/packages/core/agent_framework/security.py:3186-3194 |
| Server label beats static label | `_meta.ifc` parsed label wins over fallback static label | python/packages/core/agent_framework/security.py:3256-3283 |
| Runtime label refresh | `SecureMCPToolProxy.refresh_labels()` re-applies annotation mapping for newly discovered tools | python/packages/core/agent_framework/security.py:3478-3487 |
| Standing approval rules at runtime | `create_always_approve_tool_response` records session-backed `ToolApprovalRule`s from approval responses | python/packages/core/agent_framework/_harness/_tool_approval.py:218-245 |
| Rule persistence scope | Rules + queued requests stored under `source_id` in `AgentSession.state`, survive across runs | python/packages/core/agent_framework/_harness/_tool_approval.py:158-215,248-275 |
| Auto-approval risk disclosure | Warning docstring: name-matching auto-rules may approve unrelated same-named tools | python/packages/core/agent_framework/_harness/_tool_approval.py:365-376 |
| Shell command policy order | denylist → allowlist → custom callback → default allow ("first hit wins") | python/packages/tools/agent_framework_tools/shell/_policy.py:70-121 |
| Shell policy ordering vs approval | Policy evaluated *before* approval and execution; approval is the stated real boundary | python/packages/tools/agent_framework_tools/shell/_policy.py:31-35; _tool.py:118-138,251-253 |
| Declarative config policy | YAML `approvalMode: kind: never` + `allowedTools` per MCP server | declarative-agents/agent-samples/foundry/MicrosoftLearnAgent.yaml:18-21 |
| Declarative fail-closed default | Unknown approval mode maps to `HostedMcpServerToolApprovalMode.AlwaysRequire` | dotnet/src/Microsoft.Agents.AI.Declarative/Extensions/McpServerToolApprovalModeExtensions.cs:20-27 |
| External policy engine | `PurviewPolicyMiddleware` blocks prompt/response on cloud policy hit via `MiddlewareTermination`; `ignore_exceptions` opt-out | python/packages/purview/agent_framework_purview/_middleware.py:24-147 |
| Prompt-based rules | `DEFAULT_HARNESS_INSTRUCTIONS` prepended to agent instructions; `SECURITY_TOOL_INSTRUCTIONS` teaches quarantine workflow | python/packages/core/agent_framework/_harness/_agent.py:54-79; security.py:2442-2470 |
| Audit log (FIDES) | In-memory `audit_log` list; `get_audit_log()`/`clear_audit_log()`; surfaced via `SecureAgentConfig.get_audit_log()` | python/packages/core/agent_framework/security.py:1698,2163-2184,2353-2361 |
| .NET parity (standing rules) | `ToolApprovalAgent` persists `ToolApprovalState` in session state bag across runs | dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:44-60 |
| Tests: policy behavior | `TestPolicyEnforcementMiddleware` (block/approve/replay), `test_policy_blocks_in_untrusted_context` | python/packages/core/tests/test_security.py:527-720,2202-2252 |
| Tests: shell policy | Default-allows, denylist, fork-bomb, Windows-destructive cases | python/packages/tools/tests/test_policy.py:24-60 |

## Answers to Dimension Questions

### 1. Where do governance rules live?

Six locations, in descending enforcement strength:

- **Runtime code (strongest)**: `approval_mode` per tool (`_tools.py:316,408`); FIDES middleware configuration (`security.py:1674-1704`); `SecureAgentConfig` constructor flags (`security.py:2234-2246`); harness wiring that inserts `ToolApprovalMiddleware` by default and orders the loop outermost (`_harness/_agent.py:636-662`).
- **Tool metadata annotations**: `additional_properties` keys `source_integrity`, `max_allowed_confidentiality`, `accepts_untrusted` read by both middlewares (`security.py:1090-1111,2002-2006,2142-2159`).
- **Remote/server-supplied metadata**: MCP `ToolAnnotations` and `_meta.ifc` result labels translated into local policy inputs (`security.py:3018-3096,3224-3283`); Foundry Toolbox objects carry `.policies` metadata per ADR (`docs/decisions/0025-foundry-toolbox-support.md:78`).
- **Deployment/config files**: declarative YAML manifests (`declarative-agents/agent-samples/foundry/MicrosoftLearnAgent.yaml:18-21`) consumed by the .NET declarative factory.
- **External policy engine**: Purview DLP via Microsoft Graph (`purview/agent_framework_purview/_middleware.py:46-48` — client built from credential + settings, decisions fetched per prompt/response).
- **Prompt text (weakest, advisory)**: harness instructions (`_harness/_agent.py:54-79`) and security usage instructions (`security.py:2442+`). Nothing in the runtime verifies compliance with these.

There is no unified policy object or registry; each mechanism owns its own definition surface.

### 2. Can policies be updated at runtime?

**Partially, mechanism-dependent:**

- **Yes — standing approval rules**: users grant "always approve" during a run and the rule is serialized into session state, taking effect on subsequent calls with no code change (`_harness/_tool_approval.py:218-245` creation, `:289-291` dedup-add, `:308-321` matching). The .NET `ToolApprovalAgent` mirrors this (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:44`).
- **Yes — dynamic label refresh**: `SecureMCPToolProxy.refresh_labels()` re-derives labels from live MCP annotations for tools discovered mid-session (`security.py:3478-3487`), so a connected server can effectively change tool policy at runtime.
- **No — allowlists and thresholds are construction-bound**: `allow_untrusted_tools`, `ShellPolicy` deny/allow regexes, `annotation_overrides`, `auto_hide_untrusted`, and Purview settings are fixed at instantiation; there is no hot-reload, no file watcher, and no admin API. Updating them requires constructing new objects (i.e., a code/deploy change).
- **No — no external policy file loader exists for FIDES or ShellPolicy.** Searched `python/samples/` and package sources: all policy lists appear as inline literals (e.g., `python/samples/02-agents/tools/local_shell_with_allowlist.py:31-37`). Only the declarative-agent YAML path reads governance settings from files, and only for MCP approval mode.

### 3. What happens when policies conflict?

Precedence is defined *within* each mechanism but **not across mechanisms**:

- **Label combination**: most-restrictive-wins — any UNTRUSTED taints; highest confidentiality tier (USER_IDENTITY > PRIVATE > PUBLIC) wins (`security.py:240-254`). This is deterministic.
- **Result labeling tiers**: embedded per-item label > tool's `source_integrity` declaration > join of input labels (`security.py:692-710,1224-1243`). Server-supplied `_meta.ifc` beats the statically derived label (`security.py:3265,3280`).
- **Operator overrides vs annotations**: explicit `annotation_overrides` replace derived labels but deliberately force `accepts_untrusted=False` so an override cannot silently widen integrity tolerance (`security.py:3186-3189`).
- **ShellPolicy internal order**: denylist → allowlist → custom callback → allow, first-hit-wins (`shell/_policy.py:74-81`; .NET identical at `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellPolicy.cs:198-235`).
- **Policy vs approval ordering**: ShellPolicy evaluates *before* human approval (`shell/_tool.py:118-121,251-253`); FIDES violations can be escalated to approval where user consent waves exactly the disclosed violation set, bound to call body + label + session (`security.py:1624-1639,2057-2082`), and a changed violation set forces re-approval (`security.py:1853-1862`).
- **Across mechanisms there is no arbiter.** FIDES, ShellPolicy, `approval_mode`, standing rules, and Purview compose only implicitly through pipeline position (e.g., harness inserts `ToolApprovalMiddleware` then user middleware, loop outermost — `_harness/_agent.py:643-662`). A Purview block and a FIDES allow can coexist; whichever middleware sees the invocation first wins by ordering, not by declared precedence.
- **Degenerate case**: if `LabelTrackingFunctionMiddleware` did not run first, `PolicyEnforcementFunctionMiddleware` logs a warning and **executes the tool anyway** (`security.py:1969-1976`) — a fail-open seam in an otherwise fail-closed design.

### 4. Are policy changes audited?

**Weakly.**

- FIDES violations append structured records (type, function, context label, turn, reason) to an **in-memory** `audit_log` (`security.py:2017-2023,2163-2174`), readable/clearable via API (`security.py:2174-2184`) and via `SecureAgentConfig.get_audit_log()` (`security.py:2353-2361`). The framework ships no persistence, export, tamper protection, or log-schema versioning.
- Standing approval rules record an optional free-text `reason` in response metadata (`_harness/_tool_approval.py:239-245`) and persist with session state, but rule *changes* (additions) generate no audit event.
- Approval request/response contents are control-plane contents that history providers may retain "for audit" per the core design notes (`python/packages/core/AGENTS.md`, Tool Approval Harness section), but retention is provider-dependent, not guaranteed.
- Usage telemetry marks only that the feature ran (`mark_feature_used(FeatureIndex.CORE_TOOL_APPROVAL)` — `_harness/_tool_approval.py:383`), not what was decided.
- The one durable audit path is **external**: Purview evaluation happens service-side (`purview/agent_framework_purview/_client.py` calls Microsoft Graph), placing audit responsibility outside the repository.

No evidence of policy versioning was found: `ToolApprovalState`/`ToolApprovalRule` serialize with type identifiers but no version field (`_harness/_tool_approval.py:145-155,204-215`; grep for "version" in `_tool_approval.py` and `security.py` returned nothing), and the FIDES APIs carry lifecycle-stage markers (`@experimental(feature_id=ExperimentalFeature.FIDES)` at `security.py:1642,3099,3324`) that gate API maturity, not policy revisions.

## Architectural Decisions

1. **Policies attach to tools, not to a central registry.** Every mechanism (approval_mode, additional_properties, MCP hints) binds rules to individual tool instances at composition time (`_tools.py:316`; `security.py:3196-3205`). Consequence: policy is co-located with capability, but there is no single place to answer "what is this agent allowed to do?".
2. **Information-flow control chosen over prompt defense/sanitization for security policy.** ADR-0024 explicitly rejects prompt-engineering defenses as non-deterministic and adopts label-based middleware with most-restrictive-wins semantics (`docs/decisions/0024-prompt-injection-defense.md:30-44,79-87`).
3. **Human approval is the designated security boundary; regex policy is explicitly demoted.** ShellPolicy documents itself as a UX pre-filter and enumerates trivial bypasses; the boundary is approval-in-the-loop plus sandbox tier (`shell/_policy.py:8-35`). Disabling approval requires an explicit `acknowledge_unsafe=True` acknowledgment (`shell/_tool.py:159-165`).
4. **Fail-safe defaults throughout**: unlabeled = UNTRUSTED + PUBLIC sink (`security.py:3057-3062`), unknown declarative approval mode = AlwaysRequire (`McpServerToolApprovalModeExtensions.cs:27`), sampling callbacks deny-by-default (core AGENTS.md, MCP section).
5. **Session state as the policy-update channel**: runtime authorization changes flow exclusively through serializable session-state contents (`_harness/_tool_approval.py:248-275`), making them restartable but host-scoped.

## Notable Patterns

- **Tiered precedence tables in docstrings as contracts**: the 3-tier label propagation table (`security.py:692-710`) and first-hit-wins ordering (`shell/_policy.py:74-81`) are specified in documentation adjacent to the implementing code.
- **Replay-bound approvals**: `_PendingPolicyApproval` binds consent to function-name+arguments signature, security label, session, and the exact disclosed violation tuple, and consumes on use (`security.py:1624-1639,1840-1870`) — approvals cannot be replayed against changed risks.
- **Metadata as a policy side-channel**: policy data rides in `additional_properties` rather than schema changes (`docs/decisions/0024-prompt-injection-defense.md:120-125`), letting remote servers participate in local policy formation.
- **Cross-SDK parity**: Python `_tool_approval.py` ↔ .NET `Harness/ToolApproval/*`, Python `ShellPolicy` (`shell/_policy.py`) ↔ C# `ShellPolicy` (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellPolicy.cs:142`) implement the same models and evaluation order.

## Tradeoffs

- **Composability over centralization**: reusing the middleware pipeline for policy means zero new abstractions and per-agent configurability (ADR-0024 "Good, because policies are configurable per agent or tool" — `docs/decisions/0024-prompt-injection-defense.md:57`), but yields no global view, no cross-mechanism conflict detection, and ordering-sensitive outcomes.
- **Conservative defaults cost utility**: most-restrictive-wins is acknowledged as potentially overly conservative, and developers bear the burden of manual tool-policy configuration (`docs/decisions/0024-prompt-injection-defense.md:63-64,77`).
- **In-memory audit keeps the core dependency-free** but makes compliance reporting the integrator's problem.
- **Explicit non-security framing of ShellPolicy** avoids false assurance yet leaves operators with only approval gates or external sandboxes (Docker/Hyperlight) for real isolation (`shell/_policy.py:31-35`).

## Failure Modes / Edge Cases

- **Fail-open when miscomposed**: missing context label ⇒ tool executes unchecked (`security.py:1969-1976`).
- **Auto-approval name collisions**: a callback written for one provider's read-only rule can approve any same-named local tool; mitigated by documentation and `server_label` scoping, not structurally prevented (`_harness/_tool_approval.py:365-376`).
- **Unenforced frontmatter**: skill `allowed-tools` is parsed and stored (`_skills.py:3477-3478`) but nothing consults it — an operator reading SKILL.md could reasonably believe it constrains execution.
- **Streaming blind spots in external policy**: Purview post-checks skip streaming responses entirely (`purview/agent_framework_purview/_middleware.py:139,241`).
- **Error-tolerance toggles weaken external policy**: `ignore_exceptions` / `ignore_payment_required` convert policy-evaluation failures into pass-through (`purview/agent_framework_purview/_middleware.py:100-107,140-147`).
- **Regex policy bypassability**: documented class of evasions (expansion, substitution, encoding) applies to any ShellPolicy deployment (`shell/_policy.py:14-21`).

## Future Considerations

- Introduce a serializable, versioned policy document (covering approval sets, untrusted allowlists, shell patterns, label overrides) loadable at startup and refreshable, closing the gap between the declarative-YAML path (which already does this for MCP approval mode) and the code-bound paths.
- Persist/export the FIDES audit log behind a pluggable sink interface with a schema version field; emit rule additions/removals as events.
- Make the missing-label path configurable to fail closed (`security.py:1969-1976`).
- Enforce or deprecate skill-frontmatter `allowed-tools` so the declared surface matches the effective surface.
- Define declared precedence across mechanisms (e.g., external engine > FIDES > approval > advisory prompt) instead of relying on pipeline ordering.

## Questions / Gaps

- No evidence found of any policy versioning scheme anywhere in the source (searched `version` in `security.py`, `_harness/_tool_approval.py`, dotnet `Harness/ToolApproval/`; only serialization type identifiers exist, e.g. `python/packages/core/agent_framework/_serialization.py:618`).
- No evidence found that the Go directory contains an implementation (`go/README.md` only), so policy analysis is limited to Python/.NET/declarative surfaces.
- The Foundry Toolbox `.policies` field appears only in ADR design vocabulary (`docs/decisions/0025-foundry-toolbox-support.md:78`); no consuming implementation was located in this snapshot.
- Whether Purview decision caching can serve stale policy after an administrator tightens rules depends on `CacheProvider` TTL configuration (cache abstraction at `purview/agent_framework_purview/_cache.py`); no default invalidation policy was verified within this study's scope.

---

Generated by `Dimension 09.01: Policy Injection Points` against `agent-framework`.
