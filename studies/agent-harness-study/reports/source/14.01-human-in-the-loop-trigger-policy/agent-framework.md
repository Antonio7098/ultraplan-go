# Source Analysis: agent-framework

## Dimension 14.01: Human-in-the-Loop Trigger Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Multi-language monorepo: Python (primary implementation), .NET/C#, Go (README-only stub) |
| Analyzed | 2026-08-26 |

All citations below are relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Agent Framework implements HITL as a **control-plane content type plus a workflow interrupt primitive**, not a single policy engine. There are two orthogonal mechanisms:

1. **Tool approval** — the function-invocation loop pauses a model turn when any called tool declares `approval_mode="always_require"` (`python/packages/core/agent_framework/_tools.py:1763`, `_tools.py:1787-1790`), emitting `function_approval_request` content that the host application must answer with a matching `function_approval_response`. An opt-in session-backed middleware (`ToolApprovalMiddleware`, `python/packages/core/agent_framework/_harness/_tool_approval.py:343`) layers standing "don't ask again" rules, heuristic auto-approval callbacks, and one-at-a-time queuing on top. The .NET stack mirrors this with `ToolApprovalAgent` under `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/`.
2. **Workflow request/response interrupts** — any executor can call `ctx.request_info(...)` to suspend the workflow and surface a typed request to the caller; resume happens via `run(responses={request_id: value})` with validation (`python/packages/core/agent_framework/_workflows/_functional.py:196-253`, `_workflows/_workflow.py:1011-1043`). Tool approvals inside workflows are forwarded through exactly this mechanism by `AgentExecutor` (`python/packages/core/agent_framework/_workflows/_agent_executor.py:444-449`).

Trigger conditions are heterogeneous and layered: explicit per-tool risk flags, curated provider defaults (skills/file-access tools default to approval-required), MCP hosted-tool allowlists, deny-by-default server-initiated sampling gates, security policy violations configured as approve-on-violation, orchestration stall detection (Magentic plan review), declaration-only/client-executed tools, frontend confirmation (AG-UI `require_confirmation`), and auto-approval budget exhaustion fallbacks. Trigger state is recorded in session state, checkpoints, history stores, an in-memory violation audit log, feature telemetry, and a formal spec with scenario-to-test mapping.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model**: triggers are explicit, named constructs (`ApprovalMode`, `MCPSpecificApproval`, `function_approval_request/response`, `RequestInfoEvent`) rather than implicit conventions; the approval resume contract is formally specified in `docs/specs/004-python-function-calling-loop.md:17-18,119-122` with a mandated scenario-to-test mapping.
- **Tests**: 1,428 lines of dedicated approval tests (`python/packages/core/tests/core/test_harness_tool_approval.py`, 23 test cases) covering forged approvals, mixed batches, streaming, standing-rule boundaries, and budget interplay, plus golden event-stream HITL tests for AG-UI (`python/packages/ag-ui/tests/ag_ui/golden/test_scenario_hitl.py:60-194`).
- **Operational safeguards**: forged-response rejection via request-id binding (`_tools.py:2182-2213`), one-shot consumption of policy approvals bound to the exact disclosed violation set (`security.py:1824-1870`), DevUI server-side validation of client approvals (`python/packages/devui/agent_framework_devui/_executor.py:742-802`), auto-approval iteration caps that fall back to human review (.NET `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgentOptions.cs:50-68`).
- Not 9–10 because: plain local tools default to `never_require` (opt-in risk model, see Tradeoffs); docstrings reference HITL types that do not exist in code (`_magentic.py:1527,1557-1566` vs actual `_magentic.py:806,835`); audit coverage is fragmented across mechanisms rather than unified; the Go target has no implementation at all.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core trigger condition | `ApprovalMode = Literal["always_require", "never_require"]`; tool-level flag | `python/packages/core/agent_framework/_tools.py:106` |
| Default trigger posture | Plain tools default to `"never_require"` | `python/packages/core/agent_framework/_tools.py:408` |
| Trigger evaluation | Batch classification builds `approval_tool_names` and sets `requires_approval`, pausing before any execution | `python/packages/core/agent_framework/_tools.py:1763-1790` |
| Approval request emission | `Content.from_function_approval_request(id=call_id, function_call=...)` surfaced to host | `python/packages/core/agent_framework/_tools.py:1805-1808` |
| Mixed-batch handling | Safe siblings stored in session state, only host-decidable requests visible | `python/packages/core/agent_framework/_tools.py:1796-1832` |
| Declaration-only tools → user input | Calls marked `user_input_request=True` so workflows pause | `python/packages/core/agent_framework/_tools.py:1836-1843` |
| Strict grant semantics | `_is_approval_granted` requires boolean `True` exactly | `python/packages/core/agent_framework/_tools.py:1975-1977` |
| Session recording of pending requests | Immutable snapshots keyed by id; duplicate ids rejected | `python/packages/core/agent_framework/_tools.py:2141-2157` |
| Response↔request binding | `_bind_approval_response_to_pending_request` rebinds and consumes; unknown ids dropped | `python/packages/core/agent_framework/_tools.py:2182-2213` |
| Approval middleware | `ToolApprovalMiddleware(auto_approval_rules=...)`, queue + standing rules in session state | `python/packages/core/agent_framework/_harness/_tool_approval.py:343-379` |
| One-at-a-time queuing | First unresolved request surfaced, rest queued (`_process_outbound_messages`) | `python/packages/core/agent_framework/_harness/_tool_approval.py:593-622` |
| Auto-approval name-collision warning | Security docstring on `auto_approval_rules` bypass risk | `python/packages/core/agent_framework/_harness/_tool_approval.py:365-376` |
| Provider default-on triggers | Skills provider registers all tools `always_require` by default | `python/packages/core/agent_framework/_skills.py:1864-1878` |
| Provider opt-outs | `disable_load_skill_approval` etc. register tools `never_require` | `python/packages/core/agent_framework/_skills.py:2045-2047`, `_skills.py:2434-2444` |
| MCP hosted-tool triggers | `approval_mode` literal or `MCPSpecificApproval` dict of name lists | `python/packages/core/agent_framework/_mcp.py:77-88`, `_mcp.py:446`, `_mcp.py:1704-1718` |
| MCP both-listed rule | Tool listed in both lists still requires approval | `python/packages/core/agent_framework/_mcp.py:2815-2817` |
| MCP sampling gate | Deny-by-default when no `sampling_approval_callback`; WARNING logs | `python/packages/core/agent_framework/_mcp.py:1434-1462`, `_mcp.py:130-138` |
| Sampling rate limit/cap | Per-session counter + maxTokens clamp around the gate | `python/packages/core/agent_framework/_mcp.py:1526-1548` |
| Policy-violation trigger | `approval_on_violation=True` → approval request instead of block | `python/packages/core/agent_framework/security.py:1679-1696`, `security.py:2077-2083` |
| Violation disclosure bundling | All violations disclosed in one request so none is waved silently | `python/packages/core/agent_framework/security.py:1889-1918` |
| One-shot policy approval | Approval bound to function+args+label+session+exact violation set, consumed once | `python/packages/core/agent_framework/security.py:1824-1870` |
| Magentic uncertainty trigger | Plan sign-off requested after initial planning when enabled | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:956-958` |
| Magentic stall trigger | After stall reset, review requested with `is_stalled=True` (`max_stall_count=3`) | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1185-1186`, `_magentic.py:475-479` |
| Plan review API | `with_plan_review(enable=True)` builder method | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1523-1568` |
| Review decision types | `MagenticPlanReviewResponse.approve()/revise()`, request dataclass | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:806-858` |
| Workflow interrupt primitive | `ctx.request_info()` raises internal `WorkflowInterrupted`; resumes with value | `python/packages/core/agent_framework/_workflows/_functional.py:196-253` |
| Agent→workflow forwarding | Approval/user-input contents become `ctx.request_info(...)` events | `python/packages/core/agent_framework/_workflows/_agent_executor.py:444-449`, `_agent_executor.py:530-542` |
| Resume validation | Unknown request ids rejected; response types coerced/validated | `python/packages/core/agent_framework/_workflows/_workflow.py:1017-1032` |
| Checkpoint capture of triggers | `pending_request_info_events` persisted in checkpoints | `python/packages/core/agent_framework/_workflows/_checkpoint.py:59-61`, `_checkpoint.py:91` |
| Replayable HITL continuation | Response-entry checkpoint before consuming responses | `python/packages/core/agent_framework/_workflows/_workflow.py:1039-1043` |
| Loop escape hatch | Agent loop stops on pending approval before evaluating continue/max-iterations | `python/packages/core/agent_framework/_harness/_loop.py:443`, `_loop.py:543`, `_loop.py:647` |
| History retention for audit | Base provider filters resolved wrappers but preserves pending occurrences | `python/packages/core/agent_framework/_sessions.py:873-929` |
| Feature telemetry | `CORE_TOOL_APPROVAL = 4` marked on middleware use | `python/packages/core/agent_framework/_telemetry.py:43`, `python/packages/core/agent_framework/_harness/_tool_approval.py:383` |
| DevUI validation | Tracks server-issued requests; rejects responses with unknown ids | `python/packages/devui/agent_framework_devui/_executor.py:126-136`, `_executor.py:742-802` |
| Frontend confirmation trigger | AG-UI `require_confirmation=True` interrupt flow, golden tests | `python/packages/ag-ui/tests/ag_ui/golden/test_scenario_hitl.py:16-36`, `:57-60` |
| AG-UI lifecycle ownership | Register/claim/settle state machine with conflict errors; sole owner of occurrence registry | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:77-267` |
| Sub-agent propagation | `UserInputRequiredException` carries child approval requests to parent | `python/packages/core/agent_framework/exceptions.py:184-209` |
| .NET parity (standing rules) | `ToolApprovalState.cs` / `ToolApprovalRule.cs` harness port | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/` |
| .NET budget fallback | Cap reached → final invocation without auto-approve so human decides | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgentOptions.cs:50-68` |
| .NET plan-review parity | `RequirePlanSignoff(bool)` builder API | `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticWorkflowBuilder.cs:83` |
| Formal contract/spec | Approval resume listed as spec-governed area with scenario mapping | `docs/specs/004-python-function-calling-loop.md:17-18`, `:119-122` |

## Answers to Dimension Questions

### 1. What triggers human review?

Eight distinct trigger classes, all converging on two transport primitives (`function_approval_request` content or workflow `request_info` events):

- **Tool risk (per-tool flag)** — `approval_mode="always_require"` on a `FunctionTool` (`python/packages/core/agent_framework/_tools.py:316`, default `never_require` at :408). The invocation loop classifies the whole batch first and pauses before executing anything if any call targets an approval tool (`_tools.py:1775-1796`).
- **Provider-curated defaults** — skills tools (`load_skill`, `read_skill_resource`, `run_skill_script`) and file-access tools are registered `always_require` unless explicitly disabled (`python/packages/core/agent_framework/_skills.py:1864-1878`; file-access analog documented in `python/packages/core/AGENTS.md`, FileAccessProvider section).
- **Hosted-tool allowlists** — MCP servers accept a dict of `always_require_approval`/`never_require_approval` tool names resolved per candidate name (`python/packages/core/agent_framework/_mcp.py:1704-1718`).
- **Uncertainty/stall** — Magentic's manager-driven plan review fires on initial plan creation when `require_plan_signoff` is set (`_magentic.py:956-958`) and again after a stall-triggered replan with `is_stalled=True` (`_magentic.py:1185-1186`; stall threshold `max_stall_count=3` at `_magentic.py:475-479`).
- **Policy violation** — the security middleware converts integrity/confidentiality violations into approval requests when `approval_on_violation=True` instead of blocking (`security.py:2077-2083`).
- **Untrusted-server initiation** — MCP server-initiated sampling is denied unless a human-supplied callback approves (`_mcp.py:1434-1462`), bounded by rate limit and token cap.
- **Non-executable calls** — declaration-only tools are returned to the caller marked `user_input_request=True` so the workflow pauses (`_tools.py:1836-1843`); sub-agent user-input needs propagate via `UserInputRequiredException` (`exceptions.py:184-209`).
- **Budget exhaustion fallback** — .NET's `MaxAutoApprovalIterations` performs a final inner invocation *without* auto-approving so remaining requests reach a human (`dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgentOptions.cs:63-65`); Python's function-call budget counts paused groups and resolves auto-approved queues within it (`test_harness_tool_approval.py:987-1089`).

### 2. Are triggers configurable?

Yes, at five granularities:

- **Per tool**: `approval_mode` constructor arg / `@tool(approval_mode=...)` decorator (`_tools.py:316-332`, decorator at :1142-1314).
- **Per provider/server**: `MCPTool.approval_mode` accepting either a global literal or a per-name `MCPSpecificApproval` dict (`_mcp.py:446-474`); provider disable flags like `disable_run_skill_script_approval` (`_skills.py:2045-2047`).
- **Per middleware instance**: `auto_approval_rules` — sync or async callbacks receiving the full function-call content (`_tool_approval.py:351-379`); evaluation order is standing rules → heuristic rules → prompt (`ToolApprovalAgentOptions.cs:36-38` documents the same order for .NET).
- **Per security policy**: three-way choice of block / approve-on-violation / warn-and-allow (`security.py:2077-2093`), configured via `SecureAgentConfig` (`samples/02-agents/security/email_security_example.py:313-319`).
- **Per orchestration**: `with_plan_review(enable)` / `enable_plan_review` (`_magentic.py:1523-1568`, :1425) and .NET `RequirePlanSignoff` (`MagenticWorkflowBuilder.cs:83`).

### 3. Can users request human review?

Yes, through several first-class paths:

- **Any workflow executor** can call `ctx.request_info(payload, response_type)` to suspend execution and await external input — the canonical HITL primitive (`_functional.py:196-253`). The guessing-game sample demonstrates the full loop (`samples/03-workflows/human-in-the-loop/guessing_game_with_human_input.py:54`).
- **Orchestration authors** can enable plan review so a human approves/edits plans before execution (`_magentic.py:1523-1552`); reviewers answer with `MagenticPlanReviewResponse.approve()` or `.revise(feedback)` (`_magentic.py:816-831`).
- **End users** answer approval prompts through hosts: DevUI (`devui/_executor.py`), AG-UI frontends (`require_confirmation=True`, golden tests at `ag-ui/tests/ag_ui/golden/test_scenario_hitl.py:57+`), or programmatic `run(responses={request_id: ...})` (`_workflow.py:688-732`).
- Users can also convert a one-off approval into a **standing rule** ("don't ask again") scoped to the tool or tool+exact-arguments via `create_always_approve_tool_response` / `create_always_approve_tool_with_arguments_response` (`_tool_approval.py:218-245`).

### 4. Are trigger decisions auditable?

Partially — strong within each mechanism, fragmented across mechanisms:

- **Session-recorded pending requests**: immutable snapshots with id uniqueness enforcement (`_tools.py:2141-2157`); every inbound response must bind to a recorded request or is discarded (`_tools.py:2182-2213`).
- **Standing rules persisted** serializably in session state (`ToolApprovalState`, `_tool_approval.py:158-215`); a test verifies forged standing approvals are dropped (`test_harness_tool_approval.py:708`).
- **Checkpoints persist open triggers** (`pending_request_info_events`, `_checkpoint.py:91`) and add a response-entry checkpoint so HITL continuations replay deterministically (`_workflow.py:1039-1043`; test at `packages/core/tests/workflow/test_functional_workflow.py:653-670`).
- **History providers retain** approval control-plane contents for audit while filtering them from later model replay (`_sessions.py:921-929`).
- **Security violations** get a structured in-memory audit log with accessor (`security.py:2163-2180`), including context labels and turn numbers (:2038-2045).
- **Telemetry** records that the approval feature was used (`FeatureIndex.CORE_TOOL_APPROVAL`, `_telemetry.py:43`).
- **DevUI validates** client approvals against server-tracked requests and logs rejections (`_executor.py:742-802`).

There is no single cross-cutting "HITL decision ledger"; auditability depends on which storage backends (session store, checkpoint storage, history provider, audit log) the host configures.

## Architectural Decisions

1. **Control-plane content types over side-channel APIs.** Approvals travel as ordinary message contents (`function_approval_request`/`function_approval_response`) rather than a separate RPC layer, so they flow through sessions, histories, and streams uniformly (`_types.py:1287,1340`; spec diagram `docs/specs/004-python-function-calling-loop.md:239-268`).
2. **Batch-pause semantics.** If any call in a model batch needs approval, nothing executes; already-safe siblings are hidden from the user but stored in session state and reinjected on resume (`_tools.py:1775-1832`) — trading latency for the invariant that no partially-approved batch runs.
3. **Occurrence-aware correlation.** Because providers may reuse `call_id`s, approval binding matches ordered call occurrences rather than a global id map (spec `docs/specs/004-python-function-calling-loop.md:278-285`; implementation in `_replace_approval_contents_with_results` referenced at :122).
4. **Strict truthiness for grants.** Only the exact boolean `True` counts as approved (`_tools.py:1975-1977`), eliminating accidental approvals from truthy non-boolean values.
5. **Deny-by-default at trust boundaries.** Untrusted directions (server-initiated sampling) default to denial with logged rationale (`_mcp.py:1434-1462`), while locally registered tools default to permissive — a deliberate asymmetry between "our code" and "their server."
6. **Single-owner lifecycle registries at edges.** AG-UI centralizes approval occurrence registration/claim/settlement in one module with conflict-typed errors (`_approval_lifecycle.py:77-267`), preventing parallel registries drifting apart.
7. **Cross-language parity via ports.** The Python harness (`_harness/_tool_approval.py`) and the .NET `Microsoft.Agents.AI.Harness.ToolApproval` implement the same standing-rule/queue/auto-approval design, including identical security warnings about name-based auto-approval collisions (`_tool_approval.py:365-376` vs `ToolApprovalAgentOptions.cs:41-46`).

## Notable Patterns

- **Escape hatch composition**: the looping middleware checks for pending approvals *before* evaluating its own continue predicates, so outer loops never swallow a pause meant for a human (`_loop.py:443,543,647`).
- **Queue-drain with rule promotion**: queued requests are re-evaluated against newly granted standing rules at the start of every run (`_drain_auto_approvable_queue`, `_tool_approval.py:579-586`), so a fresh "always allow" immediately unblocks previously queued items.
- **Disclosure completeness**: policy approvals bundle *every* detected violation into one request so an approval cannot silently wave an undisclosed second violation (`security.py:1889-1893`); the same philosophy appears in DevUI rendering all violations (`security.py:1914-1918`).
- **Replayable interruption**: checkpoints record both open requests and delivered-response states, making crash recovery mid-HITL deterministic (`_workflow.py:1039-1043`, `test_checkpoint.py:579`).
- **Server_label boundary for hosted tools**: standing rules carry the hosted server label so approving a tool on one MCP server never approves a same-named tool on another (`_tool_approval.py:109-111`, test at `test_harness_tool_approval.py:1224`).

## Tradeoffs

- **Opt-in safety for local tools.** A bare `@tool` runs without approval (`_tools.py:408`); safe defaults exist only inside curated providers (skills, file access) and at network trust boundaries. Applications that forget to flag risky tools get no prompt.
- **One-at-a-time queuing vs throughput.** Surfacing unresolved approvals singly (`_tool_approval.py:612-618`) gives clean UX but serializes multi-tool turns; .NET relaxes this after the auto-approval cap, surfacing batches as-is (`ToolApprovalAgentOptions.cs:32-34`).
- **Auto-approval expressiveness vs blast radius.** Callback rules receive the full call content but may match on name alone; the framework documents (rather than prevents) the collision hazard (`_tool_approval.py:365-376`).
- **Checkpoint-everything vs cost.** Full replayability of HITL continuations requires checkpoint storage and doubles state writes around pauses (`_workflow.py:1039-1043`); hosts without checkpointing lose resumability.

## Failure Modes / Edge Cases

- **Forged or stale approvals**: responses not bound to a session-recorded pending request are dropped (`_tools.py:2195-2198`); DevUI rejects unknown ids with a logged error (`_executor.py:755-762`).
- **Replayed approvals across changed policies**: policy approval matches require the *exact* current violation set to equal the disclosed set; a metadata change forces re-request (`security.py:2057-2061`, :1856-1862).
- **Deleted/upgraded tools between pause and resume**: recorded requests whose tool disappeared no longer execute (`test_harness_tool_approval.py:842-880`), while same-name upgrades are handled (`:799-841`).
- **Callback exceptions fail closed**: a raising sampling approval callback denies the request and logs (`_mcp.py:1451-1458`).
- **Duplicate request ids in one batch** raise rather than silently shadowing (`_tools.py:2152-2153`).
- **Unbounded auto-approval loops**: capped explicitly in .NET (`ToolApprovalAgentOptions.cs:56-61`) and indirectly via the shared function-invocation budget in Python (`test_harness_tool_approval.py:987-1089`).
- **Loop/approval interaction**: without the escape hatch, an outer agent loop could mask a pending approval; the check precedes loop evaluation (`_loop.py:543-546`).

## Future Considerations

- Unify audit trails: expose one append-only HITL decision log spanning tool approvals, policy approvals, plan reviews, and sampling decisions (today split across session state, `get_audit_log()`, history stores, and logs).
- Add argument-aware *trigger* predicates symmetric to auto-approval rules (e.g., "require approval when `path` starts with `/etc`"), currently expressible only via middleware workarounds.
- Promote `is_stalled`-style uncertainty triggers beyond Magentic (e.g., confidence-based escalation hooks for other orchestrators).
- Fix the stale docstring type references in `_magentic.py` (`MagenticHumanIntervention*` names) to match `MagenticPlanReviewRequest/Response`.
- Port the trigger/approval machinery to Go (`go/` contains only a README today), or document the Go story explicitly as out of scope.

## Questions / Gaps

- **No evidence found** for a configurable, persistent audit sink for tool-approval decisions themselves (who approved what, when) independent of chat-history retention; searched `python/packages/core/agent_framework` for `audit` beyond `security.py` and found only the security-module log (`security.py:2163-2184`) and DevUI debug logging.
- **No evidence found** for timeout/expiry semantics on pending approvals: nothing in `_tools.py:2141-2213` or `_workflow.py:1011-1043` expires a stale pending request; they remain resumable indefinitely (by design, per `_sessions.py` retention notes), which long-running hosts may want to bound.
- The Go directory (`go/README.md`) contains no implementation, so trigger-policy conclusions apply to Python and .NET only.
- Whether Foundry/CopilotStudio hosted surfaces enforce their own approval triggers could not be verified from source alone (hosted-service ownership is asserted at `docs/specs/004-python-function-calling-loop.md:62`).

---

Generated by `14.01-human-in-the-loop-trigger-policy` against `agent-framework`.
