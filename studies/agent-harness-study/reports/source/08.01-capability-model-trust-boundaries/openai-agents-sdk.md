# Source Analysis: openai-agents-sdk

## Dimension 08.01: Capability Model and Trust Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (3.10+), Pydantic, asyncio; OpenAI Responses API plus pluggable model providers |
| Analyzed | 2026-08-24 |

## Summary

The OpenAI Agents SDK implements an explicit, layered capability model in which the model can only *request* actions while the SDK runtime holds execution authority. Capabilities are declared through typed tool objects (`src/agents/tool.py`), agent-to-agent transfers (`Handoff`, `src/agents/handoffs/__init__.py:126`), MCP server integrations with allow/block filters (`src/agents/mcp/util.py:122-137`), and a composable sandbox `Capability` system (`src/agents/sandbox/capabilities/capability.py:16-70`) whose default set is Filesystem + Shell + Compaction (`src/agents/sandbox/capabilities/capabilities.py:8-10`).

Trust boundaries are enforced at four distinct choke points:

1. **Human approval boundary** — tools declare `needs_approval`; the run loop interrupts before any side effect and requires `RunState.approve()/reject()` (`src/agents/run_state.py:1255-1298`). Approval-rule evaluation fails closed when arguments cannot be safely parsed (`src/agents/run_internal/tool_execution.py:1306-1310`, `src/agents/util/_approvals.py:14-29`).
2. **Caller identity boundary** — the SDK verifies that each tool call's `caller` (direct vs programmatic) is on the tool's `allowed_callers` list, defaulting to `["direct"]` only (`src/agents/run_internal/tool_caller.py:47-66`).
3. **Filesystem authority boundary** — every sandbox path passes through `WorkspacePathPolicy.normalize_path`, which rejects anything outside the workspace root or explicit `SandboxPathGrant`s and enforces read-only grants (`src/agents/sandbox/workspace_paths.py:365-459, 461-475`). Host-path grants cannot be injected from serialized manifests (`src/agents/sandbox/manifest.py:597-606`).
4. **Mount credential boundary** — a dedicated security module classifies mount strategies by trusted-class provenance tables, treats credential-bearing fields as "authority," redacts them from errors and durable state, and rejects custom subclasses attempting to self-promote into trusted boundaries (`src/agents/sandbox/_mount_security.py:54-111, 193-257`).

The design distinguishes runtime authority (Runner owns approvals, guardrails, tracing) from tool authority (sandbox session owns command/file isolation), a split stated as core in `docs/sandbox/guide.md:29`. Dangerous capabilities are isolated behind containerized or hosted execution surfaces rather than in-process defaults for shell/filesystem work.

## Rating

**9 / 10.**

Rationale against the rubric: this is a mature, durable capability model — explicit interfaces (`FunctionTool.needs_approval` at `src/agents/tool.py:486-493`, `Capability` protocol at `src/agents/sandbox/capabilities/capability.py:16-70`, `ToolFilter` at `src/agents/mcp/util.py:135`), operational safeguards (fail-closed approvals, read-only grant enforcement, network-isolation validation), observability (approval items surface as interruptions; errors attach span data via `_error_tracing.attach_error_to_current_span`, e.g. `src/agents/run_internal/tool_caller.py:34-44`), extensibility (custom capabilities via `configure_tools` hooks), and proof under adversarial conditions: a 4,516-line mount-security test suite (`tests/sandbox/test_mount_security.py`), a 742-line approval-scoping test suite covering cross-server aliasing and legacy-key attacks (`tests/test_run_context_approvals.py:30-721`), and Docker-based security integration tests asserting a runner-owned sandbox cannot read trusted client credentials (`integration_tests/security/test_local_sandbox_isolation.py:114`). The single point keeping it from 10: ordinary `FunctionTool`s execute arbitrary Python inside the host process with full application privileges — OS-level confinement for that path is delegated to the developer (see Tradeoffs).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool permissions — dynamic enable/disable | `FunctionTool.is_enabled: bool \| Callable[[RunContextWrapper, AgentBase], ...]` gates whether a tool is even offered to the model per run | src/agents/tool.py:472 |
| Tool permissions — human approval gate | `needs_approval` bool-or-callable interrupts the run until `RunState.approve()/reject()` is called | src/agents/tool.py:486-493; src/agents/run_state.py:1255-1298 |
| Tool permissions — caller restriction | `allowed_callers` normalized at construction for ShellTool, ApplyPatchTool, CustomTool, hosted tools | src/agents/tool.py:1385-1393, 1441-1448, 1474-1483, 1102-1110 |
| Approval enforcement point | Runner checks stored decision, evaluates rule, then either executes, injects rejection message, or returns the pending `ToolApprovalItem` as interruption — all before executor invocation | src/agents/run_internal/tool_actions.py:480-527 |
| Fail-closed approval evaluation | If tool args are unparseable/malformed JSON/non-object, callable rules are skipped and manual approval is required (`return True`) | src/agents/run_internal/tool_execution.py:1306-1310; src/agents/util/_approvals.py:18-29 |
| Approval scoping | Decisions keyed by tool name/namespace/lookup key; hosted-MCP sticky decisions require `(server_label, tool_name)` identity; legacy bare-name keys do not grant access | src/agents/run_context.py:89, 168-190, 1043-1077; tests/test_run_context_approvals.py:245-267 |
| Caller identity check | `ensure_tool_caller_allowed` raises `ModelBehaviorError` when call's caller type is unsupported or not in effective allowlist (default `["direct"]`); programmatic calls must reference a live parent program call ID | src/agents/run_internal/tool_caller.py:13-66, 69-129 |
| Sandbox rules — workspace scope | `WorkspacePathPolicy.normalize_path` resolves paths under workspace root or explicit grants; outside paths raise `InvalidManifestPathError("escape_root")` | src/agents/sandbox/workspace_paths.py:365-459, 489-496 |
| Sandbox rules — grants validated | `SandboxPathGrant` validators reject relative paths, UNC/device paths, parent segments, and filesystem roots for both sandbox and host paths | src/agents/sandbox/workspace_paths.py:204-285, 288-308 |
| Sandbox rules — read-only enforcement | Writes to read-only grants raise `WorkspaceArchiveWriteError` with reason `read_only_extra_path_grant` | src/agents/sandbox/workspace_paths.py:461-475 |
| Sandbox rules — apply_patch preflight | Every patch operation is path-normalized and validated before any operation executes, so one bad path aborts the whole batch atomically | src/agents/sandbox/capabilities/tools/apply_patch_tool.py:254-260; src/agents/sandbox/apply_patch.py:163-179 |
| Untrusted manifest hardening | Serialized manifest dicts cannot set `extra_path_grants` (TypeError) nor the credential-exposure policy (TypeError in validator) — escape hatches are trusted-code-only | src/agents/sandbox/manifest.py:597-606, 261-271 |
| Mount provenance trust tables | Closed tables map strategy types to owning modules/classes; custom subclasses cannot self-declare trusted boundaries; unknown SDK extension mounts stay untrusted | src/agents/sandbox/_mount_security.py:146-171, 193-257, 760-769 |
| Secrets access — authority fields | Per-mount-type tables enumerate credential fields (`access_key_id`, `secret_access_key`, `account_key`, tokens, service-account files); in-container credential sets demand explicit runtime-only acknowledgement methods | src/agents/sandbox/_mount_security.py:54-111, 301-339; src/agents/sandbox/manifest.py:297-351 |
| Secrets redaction | `redact_mount_error_data` decorator replaces raw exceptions with redacted `MountConfigError`s so tracebacks/logs never carry mount authority; persisted state sanitizes provider identity when authority was present | src/agents/sandbox/_mount_security.py:487-557; src/agents/sandbox/sandboxes/docker.py:198-210 |
| Network policy | Docker sessions validate `network_mode="none"` and reject combining it with exposed ports; state validators re-check on restore | src/agents/sandbox/sandboxes/docker.py:174-180, 190-226, 1658-1660 |
| Auth checks — sandbox user identity | `User`/`Group`/`Permissions` models; capabilities bind a `run_as` user used for exec/read/write operations | src/agents/sandbox/types.py:9-55; src/agents/sandbox/capabilities/capability.py:39-41; src/agents/sandbox/capabilities/tools/shell_tool.py:215-222 |
| Guardrails (tripwires) | Input/output guardrails return `GuardrailFunctionOutput(tripwire_triggered=...)`; triggered input tripwire halts execution and skips session save | src/agents/guardrail.py:20-32, 72-103; src/agents/run_internal/run_loop.py:2053-2056, 330 |
| Tool-level guardrails | Per-tool input/output guardrail lists run around invocation | src/agents/tool.py:480-484; src/agents/tool_guardrails.py:152-181 |
| MCP tool filtering | `ToolFilterStatic` allowlist/blocklist and callable filters evaluated per (run context, agent, server); helper `create_static_tool_filter` | src/agents/mcp/util.py:93-138, 213-237 |
| Handoff privilege separation | `Handoff.is_enabled` dynamic gating; `input_filter` controls how much conversation history the next agent receives (information-flow control between agents) | src/agents/handoffs/__init__.py:158-193 |
| Prompt-injection policy for remote mounts | Remote cloud mounts are labeled untrusted data in generated instructions with a command allowlist; read-only mounts get explicit do-not-edit guidance | src/agents/sandbox/remote_mount_policy.py:8-73 |
| Tracing sensitive data | `trace_include_sensitive_data` defaults from environment; tool payloads only attached to spans when enabled | src/agents/run_config.py:53-54, 404-405; src/agents/run_internal/tool_actions.py:475-478 |
| Security integration tests | Docker-based test proves runner-owned local sandbox cannot inspect trusted client credential; packaged-distribution tests prove mount credentials are redacted from failures | integration_tests/security/test_local_sandbox_isolation.py:114-219; integration_tests/security/test_packaged_mount_redaction.py:88, 145 |
| Realtime parity | Realtime session reuses the same fail-closed argument parsing and approval item machinery | src/agents/realtime/session.py:45, 651-721 |

## Answers to Dimension Questions

### 1. What can the agent do?

The agent's power is exactly the union of its configured surfaces:

- **Function tools**: arbitrary developer Python invoked with validated JSON args (`src/agents/tool.py:440-466`). These run in-process with full application privileges.
- **Hosted tools executed by OpenAI's API**: web search, file search, code interpretation, image generation, computer use, hosted shell, apply_patch (`src/agents/tool.py:771-786, 799-840, 1117-1151, 1362-1414, 1417-1452`).
- **MCP tools**: local servers (stdio/SSE/streamable HTTP) filtered by allow/block lists, and remote hosted MCP servers (`src/agents/mcp/util.py:93-237`; `src/agents/tool.py:1086-1114`).
- **Handoffs**: transferring control to another agent, optionally filtered history (`src/agents/handoffs/__init__.py:126-198`).
- **Sandbox capabilities**: within a sandbox session the model gets `exec_command`/`write_stdin` (shell), `view_image`, `apply_patch` (`src/agents/sandbox/capabilities/shell.py:46-65`; `filesystem.py:32-52`), plus optional Skills, Memory, Compaction capabilities (`skills.py:522-530`; `memory.py:18`; `compaction.py:162`). Shell commands execute inside a Docker/local-Unix/hosted sandbox session, constrained to the manifest workspace root plus explicit path grants.

### 2. What can the model only request but not directly do?

Everything. The model emits structured requests (function_call, shell_call, apply_patch_call, mcp_approval_request, computer_call); actual execution always happens in SDK-owned code paths. Concretely:

- A shell command becomes a `cd <scoped-workdir> && cmd` string executed via the bound session transport, not by the model (`src/agents/sandbox/capabilities/tools/shell_tool.py:69-85, 200-208`).
- File mutations become parsed diff operations whose paths are re-validated by `WorkspaceEditor._validate_path` → `WorkspacePathPolicy.relative_path`, raising `ApplyPatchPathError(reason="escape_root")` on traversal (`src/agents/sandbox/apply_patch.py:154-179`).
- Any call flagged `needs_approval` halts as a `ToolApprovalItem` interruption; only application code holding the `RunState` can approve/reject (`src/agents/run_state.py:1255-1298`). The model has no channel to self-approve, and per-call decisions are bound to specific call IDs to prevent replay (`tests/test_tool_approval_call_id_reuse.py`).
- Programmatic tool invocations must name an active parent program call; orphaned or completed-parent calls are rejected (`src/agents/run_internal/tool_caller.py:97-129`).

### 3. Where is authority enforced?

Authority lives in the SDK runtime, not in prompts or conventions:

- **Pre-execution approval checks** in the run loop (`src/agents/run_internal/tool_actions.py:480-527` for shell; same pattern for function/custom/apply_patch at lines 501, 724, 934).
- **Caller verification** before dispatch (`src/agents/run_internal/tool_caller.py:47-66`), with error details attached to the current trace span.
- **Path normalization at the session layer**: `BaseSandboxSession.normalize_path` consults `WorkspacePathPolicy`, which is the single source of truth for what is reachable (`src/agents/sandbox/workspace_paths.py:365-415`).
- **Construction-time validation**: dangerous configurations fail fast — hosted ShellTool forbids client-side `executor`/approval knobs (`src/agents/tool.py:1403-1409`); Docker options validate network/port combinations eagerly (`src/agents/sandbox/sandboxes/docker.py:174-180`).
- **Serialization boundaries on restore**: restoring `RunState` re-validates approval payloads and refuses malformed or aliased approval identities rather than trusting persisted data (`src/agents/run_state.py:300-529`; `src/agents/run_context.py:1237-1250`).

### 4. Are dangerous capabilities isolated?

Yes, through layered containment:

- **Shell/filesystem** run in a sandbox session (Docker container, local Unix user separation, or hosted provider). The docs' architecture statement assigns "commands, file changes, and environment isolation" to the session and "approvals, tracing, handoffs" to the outer runtime (`docs/sandbox/guide.md:29`).
- **Credentials** never flow into the sandbox by default: credentialed in-container mounts require explicit, runtime-only acknowledgement methods that serialized input cannot forge (`src/agents/sandbox/manifest.py:297-351`), and an end-to-end security test asserts a runner-owned sandbox cannot read the trusted client's sentinel credential even mid-run (`integration_tests/security/test_local_sandbox_isolation.py:114-114+`).
- **Network egress** is opt-out-permissive but explicit: `network_mode="none"` gives full isolation and is mutually exclusive with port exposure, validated at creation and on state restore.
- **Untrusted content** inside mounts is handled at two levels: enforced read-only grants (`workspace_paths.py:461-475`) plus instruction-level prompt-injection defenses for writable remote mounts (`remote_mount_policy.py:8-46`). The former is hard enforcement; the latter is advisory — an honest, documented limitation of instruction-based mitigation.
- **Model-facing instructions themselves are composed deterministically** from capability fragments and the manifest tree, not free-form model output (`runtime_agent_preparation.py:186-235`).

## Architectural Decisions

1. **Capabilities as first-class composable objects.** `Capability` binds a live session, a sandbox user identity, and an immutable per-run `SandboxWorkspaceScope`, then contributes tools, instructions, sampling params, and context transforms (`src/agents/sandbox/capabilities/capability.py:16-70`). Per-run cloning prevents cross-run capability bleed (`clone()` at line 28-33; `clone_capabilities` in `runtime_agent_preparation.py:42-43`). Dependency declaration via `required_capability_types()` fails fast with `UserError` (`runtime_agent_preparation.py:97-103`).

2. **Approvals as serializable run-state, not callbacks-only.** Interruptions pause the run; decisions persist in `_ApprovalRecord.approved/rejected` as bools or per-call-ID lists (`src/agents/run_context.py:60-68`), enabling human-in-the-loop across process restarts (`docs/human_in_the_loop.md:3-9`).

3. **Fail-closed argument inspection.** Callable approval rules receive parsed arguments only when JSON parses cleanly to an object without NaN/Infinity constants; otherwise approval is mandatory (`src/agents/util/_approvals.py:14-29`; `tool_execution.py:1306-1310`; documented contract at `docs/human_in_the_loop.md:15`).

4. **Provenance-based trust classification for mounts.** Trust follows module/class identity tables rather than isinstance checks or flags, so reload attacks and subclass spoofing fail ("Class provenance keeps module reloads stable while ordinary custom subclasses cannot promote themselves into a trusted boundary", `src/agents/sandbox/_mount_security.py:193-195`).

5. **Trusted/untrusted configuration split.** Anything expanding authority (path grants, credential exposure) must be set on constructed `Manifest` instances by application code; dict-shaped (potentially deserialized) manifests raising `TypeError` for those keys (`src/agents/sandbox/manifest.py:261-271, 597-606`). This directly implements the repo rule that "untrusted sandbox manifests [must not] opt themselves out of host filesystem or base-directory boundaries" (`AGENTS.md`).

6. **Redaction at the error boundary.** Mount failures are rebuilt as sanitized exceptions with traceback locals scrubbed, because display suppression alone leaves secrets in exception chains (`src/agents/sandbox/_mount_security.py:487-662`; rationale codified in `AGENTS.md` security-review section).

## Notable Patterns

- **Choke-point normalization**: all path interpretation funnels through `coerce_posix_path` + policy classes; comments explain why raw strings are preserved until the policy runs (OS-dependent backslash handling would otherwise leak, `apply_patch.py:167-170`).
- **Defense against identity confusion**: hosted-MCP approvals scoped by `(server_label, tool_name)` so an always-approve on one server never authorizes same-named tools elsewhere, with regression tests for legacy-key aliasing (`run_context.py:440-472`; `tests/test_run_context_approvals.py:207-247`).
- **Atomic multi-operation validation**: apply_patch validates every operation before executing any (`apply_patch_tool.py:256-260`).
- **Instruction-as-policy with enforcement backup**: the remote-mount allowlist text is advisory, but paired with real read_only mount modes and command-level restrictions in the manifest.
- **Closed-world tables everywhere**: strategy classifications, serialized field allowlists per type, authority-field inventories — all enumerable, testable structures rather than open-ended heuristics (`_mount_security.py:158-279`).
- **Security tests as executable specs**: e.g., `test_manifest_input_cannot_inject_mount_credential_acknowledgement`, `test_custom_mount_cannot_self_declare_a_trusted_credential_boundary`, `test_rejects_rclone_credential_source_overrides` (`tests/sandbox/test_mount_security.py:614-661, 887-902, 1501`).

## Tradeoffs

- **In-process function tools carry full host privileges.** `FunctionTool.on_invoke_tool` executes arbitrary Python with the application's own credentials and network access (`src/agents/tool.py:455-466`). Mitigations are advisory layers (guardrails, approvals, timeouts) — there is no OS-level confinement for this default path. Developers wanting isolation must move logic into sandbox sessions, hosted shells, or code-interpreter tools. This is the main residual risk in the model.
- **Prompt-level policies are best-effort.** The remote-mount injection defense and capability instruction fragments rely on model compliance (`remote_mount_policy.py:8-18`, `capabilities/shell.py:16-26`); the SDK correctly pairs them with hard enforcement where possible (read-only grants, path policy) but writable remote mounts remain instruction-guarded only.
- **Complexity cost of the trust tables.** Maintaining parallel closed tables (`_AUTHORITY_FIELDS_BY_MOUNT_TYPE`, `_IN_CONTAINER_CREDENTIAL_SET_REQUIREMENTS_BY_MOUNT_TYPE`, `_RCLONE_CONFIG_VALUE_FIELDS_BY_MOUNT_TYPE`, ...) requires "keep aligned with" discipline noted inline (`_mount_security.py:79-81, 112-113`) — drift risk is mitigated by the large test matrix but is real.
- **Docker isolation is opt-in.** Default sandbox clients favor `UnixLocalSandboxClient` for development (`docs/sandbox/guide.md:73`), meaning dev-time shell runs share more of the host than production Docker runs; the docs steer Windows users to Docker explicitly, but macOS/Linux developers may run weaker isolation than they deploy.
- **Positional-compatibility constraints shape security-relevant APIs**: field ordering comments ("Keep guardrail fields before needs_approval to preserve v0.7.0 positional constructor compatibility") show safety fields are placed under ABI constraints (`tool.py:477-495`), slightly slowing future redesign.

## Failure Modes / Edge Cases

- **Malformed approval payloads during resume** are rejected rather than trusted: serialization guards raise `TypeError` on non-finite numbers, cyclic structures, colliding keys, or unsupported models (`src/agents/run_state.py:313-529`), preventing crafted state from forging approvals.
- **Call-ID collisions between nested agent-tool runs** raise `UserError` instead of silently applying an approval to the wrong invocation (`src/agents/run_state.py:1237-1253`).
- **Missing call IDs on approval-gated calls** raise `ModelBehaviorError` ("Approval-gated tool calls require a non-empty call ID", `run_context.py:1076-1077`) rather than executing ungated.
- **PTY transport failure** falls back to one-shot exec with a visible notice prepended to output, so the model knows its interactive session did not start (`shell_tool.py:227-242, 259-260`).
- **Invalid patch paths during approval pre-check** deliberately return "no approval needed" so the model receives a recoverable path error instead of a dead-end interruption (`apply_patch_tool.py:235-240`) — a usability/fail-open tradeoff that is safe because execution itself still validates and raises `ApplyPatchPathError`.
- **Credential exposure acknowledgment gaps** (empty scalar authority, incomplete credential sets, mixed usable/empty sources) each have targeted rejection tests (`test_mount_security.py:452-534, 1583-1694`).
- **Restoring sessions with redacted authority** regenerates deterministic placeholder IDs so persisted containers can't be silently reattached (`sandboxes/docker.py:198-210`).

## Future Considerations

- Extend the fail-closed pattern to more tool families (currently callable `needs_approval` on function tools has explicit fail-closed handling; custom-tool preflight handles parse errors separately).
- Provide an opt-in process-level confinement story for plain function tools (e.g., documented patterns pairing function tools with sandbox sessions), since that is the widest trust gap today.
- Continue collapsing instruction-only policies into enforceable mechanisms (e.g., command interception for remote-mount allowlists) as the sandbox session layer matures.
- Watch table drift in `_mount_security.py` — a generation step deriving the authority tables from the entry models themselves would remove the "keep aligned" burden.

## Questions / Gaps

- **No evidence found** for a global, SDK-enforced network-egress policy beyond the Docker `network_mode` option; hosted providers presumably manage their own, but no shared policy abstraction exists in the inspected code (searched `network_mode`, `egress`, `allow_network` across `src/agents/`).
- Computer-use safety checks exist as an acknowledgement surface (`PendingSafetyCheck`, `src/agents/tool.py:972-986`), but the depth of what the SDK verifies versus delegates to the provider was not traced end-to-end in this pass (the computer-action execution path lives largely in provider adapters not examined here).
- Voice pipelines were out of scope for deep reading; realtime parity was confirmed for approvals (`src/agents/realtime/session.py:651-721`) but voice-specific capability surfaces were not inventoried.

---

Generated by `Dimension 08.01: Capability Model and Trust Boundaries` against `openai-agents-sdk`.
