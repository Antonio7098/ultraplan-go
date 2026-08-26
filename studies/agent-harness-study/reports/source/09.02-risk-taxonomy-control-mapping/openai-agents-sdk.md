# Source Analysis: openai-agents-sdk

## Dimension 09.02: Risk Taxonomy and Control Mapping

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (pydantic-based SDK, asyncio runtime) |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK does not ship a single, first-class "risk taxonomy" type — there is no `Risk` enum, `RiskLevel`, or central risk registry anywhere in `src/agents/` (a repo-wide search for `risk|Risk` returns only incidental comments, e.g. `src/agents/run_state.py:3925` and `src/agents/run_internal/model_retry.py:428`). Instead, risk is *implicit in the control surfaces*: the SDK names risks by where a check attaches (agent input/output, tool call input/output, sandbox filesystem path, mount credential exposure) and maps each to one of several layered controls:

1. **Agent-level guardrails** with binary tripwire semantics (`src/agents/guardrail.py:19-32`, enforced in `src/agents/run_internal/guardrails.py:115-170`).
2. **Tool-level guardrails** with three explicit response behaviors — `allow`, `reject_content`, `raise_exception` (`src/agents/tool_guardrails.py:40-77`).
3. **Per-call approval gating** via `needs_approval` bool-or-callable on every function tool (`src/agents/tool.py:486-493`), normalized MCP `require_approval` policies (`src/agents/mcp/server.py:709-813`), and durable human-in-the-loop approve/reject records (`src/agents/run_context.py:57-68`).
4. **Sandbox boundary controls**: POSIX-style `Permissions` (`src/agents/sandbox/types.py:34-46`), `WorkspacePathPolicy` with validated extra path grants (`src/agents/sandbox/workspace_paths.py:204-308,311-551`), and a remote mount command allowlist (`src/agents/sandbox/manifest.py:34-53`).
5. **A genuine, explicit two-tier risk taxonomy exists only in the mount-credential domain**: `_MountCredentialExposurePolicy` classifies credential exposure as `mount_scoped` vs `broad` authority (`src/agents/sandbox/manifest.py:75-78`), backed by a per-mount-type/strategy/pattern capability table that enumerates exactly which credential fields fall into each tier (`src/agents/sandbox/_mount_security.py:301-409`) and an authority-field table per mount provider (`src/agents/sandbox/_mount_security.py:60-78`).

Risk assessment happens at runtime per tool *call* (the callable form of `needs_approval` receives run context, parsed arguments, and call ID), and approval decisions are exposed to and persisted through the runtime state machine (`RunContextWrapper._approvals`, `src/agents/run_context.py:89`; serialized via `RunState`). Bypass resistance is engineered rather than assumed: fail-closed argument parsing (`src/agents/util/_approvals.py:18-29`), class-provenance checks that prevent custom mounts from self-promoting into trusted boundaries (`src/agents/sandbox/_mount_security.py:1348-1370`), and manifest-input rejection of credential-policy injection (`src/agents/sandbox/manifest.py:261-271`).

An operator can explain what controls apply to an action only by reading across four subsystems; there is no single queryable mapping from action → applicable controls.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards. The control layer is exceptionally well tested (132 tests in `tests/sandbox/test_mount_security.py`, ~2,270 lines in `tests/test_guardrails.py`, dedicated approval suites `tests/test_run_context_approvals.py`, `tests/test_run_internal_approvals.py`, `tests/test_tool_guardrails.py`) and fail-closed behavior is implemented and documented (`docs/human_in_the_loop.md:15`). It does not reach 8–10 because risk naming is fragmented across subsystems with no unified taxonomy or registry, coverage is explicitly uneven (tool guardrails do not cover hosted/built-in tools or handoffs, `docs/guardrails.md:65`), and there is no aggregate risk-reporting surface.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| No global risk enum | Repo search for `risk/Risk` finds only comments; no taxonomy symbol exists | `src/agents/run_state.py:3925`, `src/agents/tracing/spans.py:80` |
| Agent-level risk categories | Input vs output guardrails are distinct named classes with tripwire contract | `src/agents/guardrail.py:72-103`, `src/agents/guardrail.py:134-143` |
| Tripwire verdict type | `GuardrailFunctionOutput.output_info` + `tripwire_triggered: bool` | `src/agents/guardrail.py:19-32` |
| Tool-level behavior taxonomy | `RejectContentBehavior` / `RaiseExceptionBehavior` / `AllowBehavior` typed dicts | `src/agents/tool_guardrails.py:40-56` |
| Tool guardrail verdict factory | `ToolGuardrailFunctionOutput.allow/reject_content/raise_exception` | `src/agents/tool_guardrails.py:79-117` |
| Per-tool approval flag | `FunctionTool.needs_approval: bool \| Callable[(ctx, params, call_id) -> bool]` | `src/agents/tool.py:486-493` |
| Tool enable/disable control | `is_enabled: bool \| Callable` static/dynamic tool availability | `src/agents/tool.py:472-475` |
| Computer safety check data | `ComputerToolSafetyCheckData` carries `PendingSafetyCheck` for acknowledgment | `src/agents/tool.py:972-986` |
| MCP approval request/result types | `MCPToolApprovalRequest`, `MCPToolApprovalFunctionResult(approve, reason)` | `src/agents/tool.py:989-1013` |
| Shell/apply_patch/custom approval callbacks | `ShellOnApprovalFunctionResult`, `ApplyPatchOnApprovalFunctionResult`, `CustomToolOnApprovalFunctionResult` | `src/agents/tool.py:1024-1083` |
| MCP policy normalization | `require_approval` accepts `"always"/"never"`, per-tool-name dict, callable; rejects invalid values with `UserError`; always∩never overlap rejected | `src/agents/mcp/server.py:709-813` |
| MCP fail-closed default | Callable policy without agent resolves to `True` ("historical fail-closed behavior") | `src/agents/mcp/server.py:824-841` |
| Durable approval record | `_ApprovalRecord.approved/rejected` as bool (sticky) or list of call IDs (per-call) + rejection messages | `src/agents/run_context.py:57-68` |
| Runtime approval store | `RunContextWrapper._approvals` dict keyed by tool identity/hosted MCP key | `src/agents/run_context.py:89` |
| Status query API | `get_approval_status(tool_name, call_id, ...)` with non-empty call-ID requirement (`ModelBehaviorError` otherwise) | `src/agents/run_context.py:1065-1077` |
| Fail-closed arg parsing | `parse_function_tool_arguments` returns `None` on malformed/non-object JSON so callable rules cannot inspect untrusted args | `src/agents/util/_approvals.py:18-29` |
| Approval evaluation helper | `evaluate_needs_approval_setting` strict mode raises `UserError` on invalid setting types | `src/agents/util/_approvals.py:32-51` |
| Enforcement point (function tools) | Approval checked before execution; unresolved → pending `ToolApprovalItem`; rejected → model-visible rejection item | `src/agents/run_internal/tool_execution.py:1884-1990` |
| Enforcement point (shell/custom/apply_patch) | Same gate repeated in `tool_actions` for shell (486-513), custom tools (706-736), apply_patch (914-946) | `src/agents/run_internal/tool_actions.py:480-513` |
| Guardrail enforcement | Tripwire raises `InputGuardrailTripwireTriggered` mid-run | `src/agents/run_internal/guardrails.py:115-170` |
| Typed tripwire exceptions | Four exception classes expose which guardrail triggered | `src/agents/exceptions.py:519-572` |
| Two-tier credential-exposure risk taxonomy | `_MountCredentialExposurePolicy(mount_scoped, broad)` private, runtime-only | `src/agents/sandbox/manifest.py:75-78` |
| Untrusted input cannot set policy | `Manifest._reject_mount_credential_exposure_policy_input` raises on any policy key in manifest dict input | `src/agents/sandbox/manifest.py:55-72,261-271` |
| Trusted acknowledgment API | `with_in_container_mount_credential_exposure_acknowledged` / `..._broad_...` on trusted instances only, not serialized | `src/agents/sandbox/manifest.py:297-351` |
| Authority-field classification tables | `_AUTHORITY_FIELDS_BY_MOUNT_TYPE`, `_URL_FIELDS_BY_MOUNT_TYPE`, opaque-strategy authority fields | `src/agents/sandbox/_mount_security.py:60-78,102-107,155-161` |
| Capability-tier table (risk → allowed exposure) | `_IN_CONTAINER_MOUNT_CREDENTIAL_CAPABILITIES`: per (mount_type, strategy_type, pattern_type) lists `mount_scoped_fields` vs `broad_fields` and `enables_broad_credential_discovery` | `src/agents/sandbox/_mount_security.py:301-409` |
| Taxonomy enforcement point | `_mount_boundary_error` denies unsupported field/tier combinations and unacknowledged exposure | `src/agents/sandbox/_mount_security.py:1532-1615` |
| Provenance anti-bypass | Custom mounts/strategies/patterns rejected at credential boundary; closed classification table blocks subclass self-promotion | `src/agents/sandbox/_mount_security.py:193-196,1348-1392` |
| Filesystem permission model | `Permissions(owner/group/other)` + `FileMode.READ/WRITE/EXEC` enum | `src/agents/sandbox/types.py:34-46,138-144` |
| Path grant validation | `SandboxPathGrant` rejects filesystem root, parent segments, UNC/device paths; read-only grants raise `WorkspaceArchiveWriteError` on writes | `src/agents/sandbox/workspace_paths.py:14-27,216-308,461-475` |
| Workspace path policy | `WorkspacePathPolicy.normalize_path(for_write=...)` enforces containment + grant mode | `src/agents/sandbox/workspace_paths.py:311-415` |
| Remote command allowlist | `DEFAULT_REMOTE_MOUNT_COMMAND_ALLOWLIST` restricts remote mount inspection commands | `src/agents/sandbox/manifest.py:34-53` |
| Network egress policy types | Hosted shell container network policy: domain allowlist w/ secrets vs disabled | `src/agents/tool.py:1219-1244` |
| Runtime observability | `GuardrailSpanData(name, triggered)` exported in traces | `src/agents/tracing/span_data.py:292-313` |
| Test coverage: approvals scoping | Sticky hosted-MCP approvals scoped by server_label; bare-name aliasing denied | `tests/test_run_context_approvals.py:30,207,245,267` |
| Test coverage: fail-closed rules | Malformed-args callable rule requires manual approval | `docs/human_in_the_loop.md:15`, `tests/test_hitl_error_scenarios.py` |
| Test coverage: mount security | 132 tests incl. acknowledgement injection rejection, subclass provenance, wildcard/parent-path rejection | `tests/sandbox/test_mount_security.py:614,651,887,1228` |
| Documented coverage gap | Tool guardrails skip hosted/built-in tools, handoffs, and `Agent.as_tool()` | `docs/guardrails.md:58-65` |
| Documented hosted-shell restriction | Hosted shell environments reject `needs_approval`/`on_approval` at construction | `src/agents/tool.py:1405-1409`, `docs/tools.md:222` |

## Answers to Dimension Questions

**1. Are risks named and categorized?**
Partially, but not via a unified taxonomy. There is no `Risk` enum or central classification anywhere in the source. Risks are instead *named by control surface*: agent input/output risk → `InputGuardrail`/`OutputGuardrail` (`src/agents/guardrail.py:72,134`); tool-call content risk → `ToolInputGuardrail`/`ToolOutputGuardrail` with a three-value behavior vocabulary allow/reject/raise (`src/agents/tool_guardrails.py:40-56`); destructive-action risk → `needs_approval` (`src/agents/tool.py:486`); filesystem risk → `SandboxPathGrant.read_only` and `FileMode` bits (`src/agents/sandbox/workspace_paths.py:211-212`, `src/agents/sandbox/types.py:138-144`); credential-leakage risk → the explicit two-tier `mount_scoped` vs `broad` authority classification (`src/agents/sandbox/manifest.py:75-78`, capability table at `src/agents/sandbox/_mount_security.py:301-409`). The mount-credential domain is the only place with a true declared taxonomy; everything else relies on convention.

**2. Is every risk mapped to a control?**
Within each subsystem, yes — every recognized risk category has a concrete enforcement mechanism (see evidence table rows for enforcement points). But the mapping is not exhaustive across surfaces: tool guardrails deliberately do not apply to handoffs, hosted tools (`WebSearchTool`, `HostedMCPTool`, etc.), or built-in execution tools (`ComputerTool`, `ShellTool`, `ApplyPatchTool`, `LocalShellTool`), documented at `docs/guardrails.md:65`; hosted shell environments hard-reject `needs_approval`/`on_approval` (`src/agents/tool.py:1405-1409`). So some high-risk surfaces (hosted shell) have no in-process approval control at all and rely entirely on the hosted provider.

**3. Can risks be assessed at runtime?**
Yes, extensively. (a) `needs_approval` may be a callable evaluated per tool call with `(run_context, parsed_params, call_id)` (`src/agents/tool.py:486-493`, evaluation in `src/agents/run_internal/tool_execution.py:1300-1318`); (b) MCP servers support callable per-tool policies (`src/agents/mcp/server.py:807-808,833-841`); (c) approval decisions live in mutable runtime state queried via `get_approval_status` (`src/agents/run_context.py:1065`); (d) guardrails execute concurrently with runs and their outcomes stream into tracing spans (`src/agents/tracing/span_data.py:292-313`). Decisions also persist across serialization for paused/resumed runs (`docs/human_in_the_loop.md:53`).

**4. Can controls be bypassed?**
Direct bypass paths are actively engineered against, with fail-closed defaults: malformed JSON arguments cause manual approval rather than skipping the rule (`src/agents/util/_approvals.py:18-29`, documented `docs/human_in_the_loop.md:15`); MCP callable policies without an agent resolve to require-approval (`src/agents/mcp/server.py:824-831`); sticky approvals are bound to invocation fingerprints and do not alias across namespaces or servers (`tests/test_run_context_approvals.py:245,531`); untrusted manifest dictionaries cannot inject credential-exposure acknowledgments (`src/agents/sandbox/manifest.py:261-271`) and custom mount subclasses cannot forge trusted provenance (`src/agents/sandbox/_mount_security.py:887,1228` tests). Residual bypass surface: application authors can simply *not attach* any control to a tool (opt-in model, no default-deny), parallel input guardrails permit agent work to start before the verdict (`docs/guardrails.md:36-38`), and `is_enabled`/guardrail configuration are plain attributes an app could mutate between planning and execution — the SDK does not freeze control configuration.

## Architectural Decisions

1. **Controls attach to declarations, not a central policy engine.** Each tool carries its own `needs_approval`, guardrail lists, and enablement (`src/agents/tool.py:472-493`); each agent carries its guardrail lists; each sandbox manifest carries its grants and allowlists (`src/agents/sandbox/manifest.py:253-256`). This trades centralized auditability for locality.
2. **Verdicts are structured, not booleans, at the tool layer.** The tri-state behavior object (`allow`/`reject_content`/`raise_exception`) lets a rejected call feed a corrective message back to the model instead of killing the run (`src/agents/tool_guardrails.py:59-77`), whereas agent-level guardrails remain binary tripwires (`src/agents/guardrail.py:29-32`).
3. **Approvals are identity-bound and durable.** `_ApprovalRecord` supports both per-call-ID and sticky tool-level decisions, keyed through canonical tool-identity helpers (`src/agents/run_context.py:57-68`, `src/agents/_tool_identity.py:31`), enabling serialize/pause/resume HITL workflows (`docs/human_in_the_loop.md:189-203`).
4. **The credential-exposure taxonomy is trust-gated.** The two-tier policy lives in a `PrivateAttr`, is rejected if it appears in deserialized manifest input, and can only be granted via methods on a trusted instance after provenance validation (`src/agents/sandbox/manifest.py:257-271,297-351`) — an explicit decision that risk acceptance must come from trusted code, not data.
5. **Fail-closed over fail-open everywhere ambiguity arises** (unparseable args, missing agent context, unknown strategy classification) (`src/agents/util/_approvals.py:46-50`, `src/agents/sandbox/_mount_security.py:1254-1256`).

## Notable Patterns

- **Capability-table classification**: `_IN_CONTAINER_MOUNT_CREDENTIAL_CAPABILITIES` maps (mount_type × strategy × pattern) to exactly which credential fields may be exposed at which authority tier, making the risk→allowed-exposure mapping data-driven and testable (`src/agents/sandbox/_mount_security.py:301-409`).
- **Closed-world trust via class provenance**: trusted boundaries check exact classes/modules rather than `isinstance`, so subclasses cannot inherit trust (`src/agents/sandbox/_mount_security.py:760-767,193-196`).
- **Error redaction as a control**: mount failures that occurred while authority was configured are replaced with sanitized exceptions so credentials cannot leak through tracebacks (`src/agents/sandbox/_mount_security.py:487-562`).
- **Layered defense ordering at tool execution**: existing decision → needs-approval rule → optional pre-approval guardrails → pending interruption, then post-execution output guardrails (`src/agents/run_internal/tool_execution.py:1884-2050`).
- **Documentation tied to enforcement**: the HITL guide's fail-closed paragraph matches `parse_function_tool_arguments`'s actual None-return behavior (`docs/human_in_the_loop.md:15` vs `src/agents/util/_approvals.py:18-29`).

## Tradeoffs

- **Fragmentation vs flexibility**: no single risk registry means adding a new risky surface requires inventing new naming (`PendingSafetyCheck`, `RequireApprovalSetting`, `SandboxPathGrant`...) rather than extending one taxonomy (`src/agents/tool.py:973`, `src/agents/mcp/server.py:79-83`, `src/agents/sandbox/workspace_paths.py:204`).
- **Latency vs safety in parallel guardrails**: `run_in_parallel=True` default lets the guarded agent start consuming tokens/tools before a tripwire lands; blocking mode costs latency (`docs/guardrails.md:34-38`, knob at `src/agents/guardrail.py:100-103`).
- **Coverage breadth vs depth**: deep, heavily tested controls for local/MCP/sandbox surfaces; thin or absent in-process controls for hosted tools, delegated to the provider.
- **Strictness vs ergonomics**: strict `UserError` on invalid policy values catches typos early (`src/agents/mcp/server.py:728-731`) but makes dynamic policy construction more brittle.

## Failure Modes / Edge Cases

- **Unparseable tool arguments**: callable approval rules are skipped and the call requires manual approval — safe but potentially surprising volume of interruptions (`src/agents/util/_approvals.py:27-29`).
- **Non-empty call ID is mandatory** for gated calls; empty/missing IDs raise `ModelBehaviorError` (`src/agents/run_context.py:1076-1077`).
- **Cross-tool alias attempts are neutralized**: same-named tools on different hosted MCP servers, namespaced vs bare names, and legacy keys all get distinct approval scopes with regression tests (`tests/test_run_context_approvals.py:30,245,531,588`).
- **Incomplete credential sets** for in-container mounts are rejected with specific field-level errors rather than partially activating (`src/agents/sandbox/_mount_security.py:1504-1530`).
- **Acknowledgement path confusion** is blocked: acknowledgments are not prefix-matched and whitespace/platform-path variants are handled explicitly (`tests/sandbox/test_mount_security.py:541,558,576`).
- **Formatter failure fallback**: a crashing `tool_error_formatter` degrades to the standard rejection message instead of leaking or failing open (`src/agents/run_internal/tool_execution.py:1275-1281`).
- **Guardrail exception vs tripwire asymmetry**: exceptions during guardrails produce different session-persistence behavior than returned rejections, carefully documented (`docs/guardrails.md:54-56`).

## Future Considerations

- Introduce a declarative risk-category enum on tools/agents (e.g., `risk_categories=["filesystem_write", "network_egress"]`) that maps to default controls, giving operators a single place to answer "what applies to this action?" while keeping current opt-in mechanisms.
- Close the hosted-shell control gap by surfacing provider-side approval requirements uniformly through `ToolApprovalItem` so all destructive actions share one operator interface.
- Expose an aggregate runtime report of active controls per run (guards × tools × agents), building on the existing `GuardrailSpanData(name, triggered)` tracing surface (`src/agents/tracing/span_data.py:292-313`).
- Extend the two-tier authority pattern from mount credentials to other secret-bearing surfaces (env values, tracing keys) which currently rely on redaction alone (`src/agents/sandbox/manifest.py:166-178`).

## Questions / Gaps

- **No evidence found** for any unified risk-scoring or risk-level construct: searches for `risk`, `dangerous`, `severity`, `threat` across `src/` produced no taxonomy symbols (only incidental comment usage). The dimension's idealized "risk taxonomy" exists only as the mount-credential two-tier scheme.
- **No evidence found** for default-deny posture: all controls are opt-in per declaration; nothing in the source indicates tools are unsafe until classified.
- Whether `ComputerToolSafetyCheckData`/`PendingSafetyCheck` acknowledgment is enforced server-side could not be verified locally — the SDK only forwards the acknowledgment payload (`src/agents/tool.py:972-986`); the enforcement boundary is outside this source tree.
- The interaction between `is_enabled` mutation after planning and the approval pipeline (TOCTOU on tool configuration) is untested in the visible suite; no test exercises mutating tool attributes between turn planning and execution.

---

Generated by `09.02-risk-taxonomy-and-control-mapping` against `openai-agents-sdk`.
