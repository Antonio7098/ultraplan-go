# Source Analysis: agent-framework

## 23.01 Autonomy Boundary

> Citation convention: all file paths below are relative to the studied source root
> `studies/agent-harness-study/sources/agent-framework/`. Only files inside that directory were inspected.

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, `python/packages/*`), C#/.NET (parity implementation, `dotnet/src/*`), Go (out-of-repo pointer only) |
| Analyzed | 2026-08-24 |

## Summary

Microsoft Agent Framework draws its autonomy boundary almost entirely around **explicit human approval of tool execution** plus **deny-by-default gates for service-initiated actions**. The unit of autonomy is the tool call: each `FunctionTool` carries an `approval_mode` (`"always_require"` / `"never_require"`, defaulting to autonomous execution — `python/packages/core/agent_framework/_tools.py:106`, `_tools.py:408`). When a gated tool is called, the function-invocation loop classifies the whole batch *before* executing anything and returns `function_approval_request` control contents to the caller instead of running the tool (`_tools.py:1775-1832`); the run suspends until the caller resumes it with a `function_approval_response` (`_types.py:1273-1362`). Rejections produce a synthetic "rejected by user" result and never execute (`_tools.py:2599-2606`).

Four layers implement the boundary:

1. **Core gating** in the function-calling loop (`_tools.py:1763`, `_tools.py:1796-1832`) with occurrence-aware approval resume, consume-once authority, and strict-boolean consent (`docs/specs/004-python-function-calling-loop.md:359-388`).
2. **Harness coordination** via an opt-out `ToolApprovalMiddleware` wired by default into `create_harness_agent` (`python/packages/core/agent_framework/_harness/_tool_approval.py:343-349`, wiring at `_harness/_agent.py:636-654`), which manages standing "don't ask again" rules, heuristic auto-approval rules, and queued approval prompts.
3. **Fail-safe defaults for untrusted surfaces**: MCP server-initiated sampling is denied unless an explicit approval callback approves it (`_mcp.py:1434-1462`, defaults at `_mcp.py:137-138`); harness-provided risky tool families (file access, skills) default to `always_require` while read-only/memory tools run autonomously (`_harness/_file_access.py:1444-1447`, `_skills.py` tests at `packages/core/tests/core/test_skills.py:3936-3945`).
4. **Workflow-level HITL**: executors pause runs with `request_info` events (`_workflows/_workflow_context.py:403`, event taxonomy at `_workflows/_events.py:113-114`), `AgentExecutor` holds execution until user-input requests resolve (`_workflows/_agent_executor.py:298-300, 396-397`), and checkpoints make human-in-the-loop continuations fully replayable (`_workflows/_workflow.py:1041`).

The design is formally documented: ADR 0006 evaluates five options and chooses typed approval content over callbacks (`docs/decisions/0006-userapproval.md:80-85, 381-383`), and a normative specification governs every change to the approval/resume machinery (`docs/specs/004-python-function-calling-loop.md:17-18, 31`). Behavior is heavily test-covered, including adversarial cases such as forged standing approvals (`packages/core/tests/core/test_harness_tool_approval.py:708`).

The system knows when it is out of its depth: any pending approval request stops autonomous loops (`_harness/_loop.py:442-459`), unresolvable situations fail closed (removed tool ⇒ no execution, `test_harness_tool_approval.py:842`; non-boolean `approved` ⇒ rejection per `docs/specs/004-python-function-calling-loop.md:374`).

## Rating

**Score: 8 / 10** — "Clear model with tests, explicit interfaces, and operational safeguards."

Rationale:
- Explicit, typed boundary protocol (`function_approval_request/response`) chosen through a documented ADR with rejected alternatives (`docs/decisions/0006-userapproval.md:381-383`).
- A binding spec constrains changes to the approval path and maps scenarios to tests (`docs/specs/004-python-function-calling-loop.md:359-412`).
- ~24 named behavior tests for the approval harness alone, including security-adversarial ones (`packages/core/tests/core/test_harness_tool_approval.py:45-1368`).
- Fail-safe operational safeguards: deny-by-default MCP sampling, strict-True consent, consume-once authority, server-label scoping of standing rules, budget accounting across auto-approved loops (`test_harness_tool_approval.py:987-1043`).
- Why not 9–10: the core default for developer-defined tools is still `never_require` (autonomy-first; safety depends on the author opting in), heuristic auto-approval rules are name-string-based with an acknowledged collision-bypass risk documented as a warning rather than eliminated (`_harness/_tool_approval.py:365-377`, `_skills.py:1981-1988`), several gating surfaces (file access, looping) remain experimental, and cross-language parity is uneven (Go lives outside this repo — `go/README.md:1-3`).

## Evidence Collected

Every entry includes file path + line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Autonomy level type | `ApprovalMode = Literal["always_require", "never_require"]` | `python/packages/core/agent_framework/_tools.py:106` |
| Default autonomy | `self.approval_mode = approval_mode or "never_require"` — tools are autonomous unless gated | `python/packages/core/agent_framework/_tools.py:408` |
| Gating semantics | Doc note: `always_require` blocks execution until explicit approval; mixed batches request approval for all calls | `python/packages/core/agent_framework/_tools.py:1229-1233` |
| Batch gating decision | Loop classifies batch before execution; any gated tool ⇒ pause whole batch, no execution | `python/packages/core/agent_framework/_tools.py:1775-1832` |
| Approval protocol types | `from_function_approval_request` (marks `user_input_request=True`), `from_function_approval_response`, `to_function_approval_response` | `python/packages/core/agent_framework/_types.py:1273-1291, 1294-1313, 1346-1362` |
| Rejection handling | Rejected calls yield synthetic result `"Error: Tool call invocation was rejected by user."` and execute zero times | `python/packages/core/agent_framework/_tools.py:2599-2606` |
| Harness middleware | `ToolApprovalMiddleware`: session-backed standing rules, queued requests, auto-approval callbacks; opt-in-but-default-on in harness | `python/packages/core/agent_framework/_harness/_tool_approval.py:343-349, 351-379`; wiring `python/packages/core/agent_framework/_harness/_agent.py:643-645` |
| Auto-approval risk warning | Documented bypass risk: name-matching rules may auto-approve unrelated colliding tools | `python/packages/core/agent_framework/_harness/_tool_approval.py:365-377` |
| Standing-rule scoping | `ToolApprovalRule` binds tool_name + optional exact arguments + hosted `server_label` boundary | `python/packages/core/agent_framework/_harness/_tool_approval.py:86-117, 553-572` |
| Loop escape hatch | `_has_pending_approval_request` stops looping middleware so approvals reach a human (mirrors C# `LoopAgent`) | `python/packages/core/agent_framework/_harness/_loop.py:442-459` |
| History hygiene | Approval control-plane contents filtered from model replay; unresolved occurrences preserved | `python/packages/core/agent_framework/_sessions.py:878-932` |
| MCP per-tool approval config | `MCPSpecificApproval` dict (`always_require_approval` / `never_require_approval` name lists); `_determine_approval_mode` resolution | `python/packages/core/agent_framework/_mcp.py:77-88, 1704-1718` |
| MCP sampling gate (fail-safe) | Deny-by-default when no `sampling_approval_callback`; rate limit + maxTokens cap; warnings logged on denial | `python/packages/core/agent_framework/_mcp.py:137-138, 1434-1462, 1526-1548` |
| Hosted MCP service-side gating | `approval_mode` mapped to OpenAI Responses `require_approval` ("always"/"never"/per-name) | `python/packages/openai/agent_framework_openai/_chat_client.py:1327-1332` |
| Provider-gated tool families | FileAccessProvider registers write+read tools `always_require` unless explicitly disabled; static read-only/all auto-approval rules reject `server_label` calls | `python/packages/core/agent_framework/_harness/_file_access.py:1444-1447`; rules `python/packages/core/agent_framework/_skills.py:1964-2036` |
| Policy-based gating | `PolicyEnforcementFunctionMiddleware` converts policy violations into approvals bound to call id + arguments + security labels, consumed once | `python/packages/core/agent_framework/security.py:1624-1704` |
| Workflow HITL | `ctx.request_info(...)`; `request_info` event labeled "(human-in-the-loop)" and forwarded to agent callers; AgentExecutor holds run until resolved | `python/packages/core/agent_framework/_workflows/_workflow_context.py:403`; `_events.py:113-114, 138-143`; `_agent_executor.py:298-300, 396-397` |
| Resumable HITL state | Checkpoint before suspension makes HITL continuation fully replayable | `python/packages/core/agent_framework/_workflows/_workflow.py:1041` |
| DevUI enforcement | DevUI executor tracks pending server-issued approval requests to validate responses | `python/packages/devui/agent_framework_devui/_executor.py:67-69, 126-136, 246-251` |
| Design rationale (ADR) | Option 5 (typed UserInput/FunctionApproval content) chosen over callback and manual-execution designs; suspend/resume requirement drives the choice | `docs/decisions/0006-userapproval.md:50-57, 80-85, 381-383` |
| Normative contract | Spec: no execution before approved response; replayed history cannot resurrect authority; strict boolean `True` grants; approved executes exactly once | `docs/specs/004-python-function-calling-loop.md:359-388` |
| Behavior tests | e.g. forged standing approval dropped (:708), recorded tool removal prevents execution (:842), mixed-batch hiding (:418), argument-scoped rule not tool-wide (:1284, :1368) | `python/packages/core/tests/core/test_harness_tool_approval.py:45, 418, 708, 842, 1161, 1224, 1284, 1368` |
| Default-mode tests | All SkillsProvider tools `always_require` by default; per-tool disable matrix verified | `python/packages/core/tests/core/test_skills.py:3936-4017`; file-access modes `python/packages/core/tests/core/test_harness_file_access.py:567-605` |
| Sample guidance | Samples warn `never_require` is "for sample brevity. Use `always_require` in production" | `python/samples/03-workflows/agents/handoff_workflow_as_agent.py:44-47`; `python/samples/03-workflows/agents/azure_chat_agents_tool_calls_with_feedback.py:57-59` |
| .NET parity | `Harness/ToolApproval/` (ToolApprovalAgent, ToolApprovalRule, ToolApprovalState, AlwaysApproveToolApprovalResponseContent); declarative `RequireApproval` on function/MCP tool executors | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/` (directory listing); `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative/ObjectModel/InvokeFunctionToolExecutor.cs:76, 443-450` |

## Answers to Dimension Questions

**1. What determines agent autonomy?**
Per-tool `approval_mode` set at definition time (`_tools.py:408`, decorator docs `_tools.py:1212-1213`), evaluated by the function-invocation loop against the registered tool map (`_tools.py:1763, 1787-1790`). Autonomy is then modulated by: (a) harness providers that pre-gate risky built-in tool families (file access writes/reads default gated — `_harness/_file_access.py:1444-1447`; skills default fully gated — `test_skills.py:3936-3945`; memory tools autonomous — `_harness/_file_memory.py:315-472`); (b) runtime standing rules created when a user answers "always approve" (`_harness/_tool_approval.py:218-245, 553-572`); (c) caller-supplied heuristic `auto_approval_rules` (`_harness/_agent.py:499-503`); (d) policy middleware able to convert a hard block into an approval request (`security.py:1688-1696`); (e) service-side gates for hosted tools (`openai/.../_chat_client.py:1327-1332`) and server-initiated sampling (`_mcp.py:1539-1548`). Declaration-only/unregistered tools are also surfaced as user-input rather than executed (`_tools.py:1833-1843`).

**2. Are autonomy levels configurable?**
Yes, at three granularities: whole-tool literals (`@tool(approval_mode="always_require")`, `_tools.py:1252-1259`), per-name dictionaries for MCP servers (`MCPSpecificApproval`, `_mcp.py:77-88`, resolution `_mcp.py:1704-1718`, tested `test_mcp.py:1569-1640, 212-237`), and behavioral callbacks (`auto_approval_rules` receiving the full function-call content, `_harness/_tool_approval.py:355-379`). Providers expose opt-outs (`disable_*_approval` flags, `_skills.py` AGENTS contract in `packages/core/AGENTS.md` Skills section; `file_access_disable_*_tool_approval`, `_harness/_agent.py:452-459`). There is no single global autonomy dial; configuration is compositional per tool/provider.

**3. Are boundaries documented?**
Yes, unusually well for this dimension. ADR 0006 records the problem, five considered designs, drivers, and the accepted outcome (`docs/decisions/0006-userapproval.md:13-29, 31-36, 381-383`), including end-to-end flow appendices (`:389-444`). The function-loop spec normatively defines approval pause/resume, authority binding, replay rejection, and control-content handling, and declares the area extra-validation-gated for contributors (`docs/specs/004-python-function-calling-loop.md:17-18, 31, 359-412`; mirrored in `python/AGENTS.md:62-66`). Security caveats of auto-approval are documented inline at the API surface (`_harness/_tool_approval.py:365-377`; `_skills.py:1981-1988, 2018-2025`), and sample code teaches the production-safe mode (`handoff_workflow_as_agent.py:44-47`).

**4. Does the system respect autonomy boundaries?**
Enforcement points are consistent and fail closed. Gating happens before any execution in the batch classifier (`_tools.py:1775-1832`); an inbound response only honors the pending snapshot recorded server-side, never history replay (`spec :362-371`); executable identity comes from the recorded request, not the response payload (`spec :368-369`); only strict `True` consents (`spec :374`); approved calls execute exactly once and rejections zero times (`spec :377-379`). Adversarial tests confirm forged standing approvals are dropped (`test_harness_tool_approval.py:708`), hosted standing rules respect the `server_label` boundary (`:1224`), and removing the recorded tool prevents execution (`:842`). Autonomous loops hand control back on any pending approval (`_loop.py:543, 647` via `:442-459`). One residual softness: heuristic auto-approval matching is by tool-name strings, with collision risk mitigated by documentation and `server_label` rejection rather than structurally prevented (`_skills.py:2018-2025`).

## Architectural Decisions

1. **Typed approval content over callbacks** (ADR option 5): approvals are message contents so remote/hosted agents can suspend, persist, and resume without holding a call stack — callbacks were explicitly rejected for hosted scenarios (`docs/decisions/0006-userapproval.md:50-57, 381-383`).
2. **Approvals are control-plane, not transcript**: `function_approval_request/response` never reach model replay; resolved wrappers are filtered while unresolved occurrences stay resumable (`_sessions.py:878-932`; `spec :389-412`). Audit retention is delegated to history providers ("may retain... for audit", `spec :409-410`).
3. **Gating in the core loop, coordination in the harness**: minimal, spec-pinned gating in `_tools.py`; convenience features (standing rules, queueing, auto-rules) live behind the opt-out-able `ToolApprovalMiddleware` (`_harness/_agent.py:494-498, 643-645`).
4. **Deny-by-default for untrusted initiators**: MCP sampling requires an explicit approving callback, is rate-limited, and token-capped (`_mcp.py:1434-1462, 1526-1548`); skills/file-access tool families ship gated with explicit opt-outs rather than the reverse (`test_skills.py:3936-3945`; `_harness/_file_access.py:1444-1447`).
5. **Occurrence-aware authority binding**: approvals bind to ordered call occurrences (call_id reuse-safe), arguments, security labels, and hosted `server_label`; they are consumed on first use (`security.py:1624-1704`; `spec :229-279` region; `test_harness_tool_approval.py:1224-1284`).
6. **Workflow HITL as first-class events**: `request_info` events are part of the public event taxonomy and forwarded across `workflow.as_agent()` so UIs can prompt humans mid-graph (`_events.py:104-143`), with checkpoints capturing decisions for replayable continuation (`_workflow.py:1041`; sample `python/samples/03-workflows/orchestrations/handoff_with_tool_approval_checkpoint_resume.py:29`).

## Notable Patterns

- **Batch-level pause semantics**: if any call in a parallel batch needs approval, nothing executes; already-approved siblings are hidden from the user, stored in session, and replayed invisibly when the visible approval resumes (`_tools.py:1796-1832`; test `test_harness_tool_approval.py:418`).
- **Standing rules with scope**: "always approve this tool" vs "always approve this tool with these exact arguments", where `{}` means exactly-no-argument calls and never becomes a wildcard (`_harness/_tool_approval.py:46-58, 1284, 1368` tests).
- **HITL escape hatch in automation**: looping middleware checks for pending approvals before continuing or injecting nudge messages (`_loop.py:442-459, 543, 647`).
- **Budget fairness across pauses**: function-call budgets count executed result groups, so auto-approved loop iterations can't bypass `max_function_calls`; once the budget exhausts, remaining approvals surface to the human instead of silently executing (`test_harness_tool_approval.py:987-1088`; AGENTS contract `packages/core/AGENTS.md` Tool Approval Harness section).
- **Security middleware interplay**: policy engines can emit `function_approval_request` directly from function middleware, which the invocation layer passes through untouched (`_tools.py:1612-1625`; `security.py:1278-1281`).
- **UI-layer validation**: DevUI keeps a registry of issued approval requests and validates responses against it (`devui/.../_executor.py:126-136`), mirroring the core's pending-request registry pattern.

## Tradeoffs

- **Autonomy-first core default vs safety-by-default providers**: `never_require` is the framework-wide default (`_tools.py:408`), placing the burden on tool authors; the harness compensates by gating its own risky surfaces. Ergonomics win in the core; caution wins in shipped providers.
- **String-based auto-approval rules**: flexible and provider-agnostic, but name collisions can widen the boundary; mitigations are documentation warnings and `server_label` scoping, not type-level guarantees (`_harness/_tool_approval.py:365-377`).
- **Complexity concentrated in resume logic**: occurrence-aware correlation spans `_tools.py` (~2500-2650), sessions filtering (`_sessions.py:878-932`), and provider adapters; the repo itself mandates extra validation for changes here (`python/AGENTS.md:62-66`) — a maintenance cost paid for correctness.
- **Audit depth depends on host**: service-managed threads may not durably record approval contents; auditing relies on logging/history-provider retention choices (`docs/decisions/0006-userapproval.md:91-95`; `spec :409-412`).
- **Cross-language drift risk**: .NET mirrors the Python model (Harness/ToolApproval, declarative `RequireApproval`), but Go is external (`go/README.md:1-3`), so boundary semantics may diverge across SDKs.

## Failure Modes / Edge Cases

- **Forged or replayed approvals**: unmatched/duplicate/replayed responses never reach execution; history replay cannot create authority (`spec :367-371`; test `:708`).
- **Tool identity drift after approval**: a same-name upgrade executes; removal of the recorded name fails closed (`spec :372-373`; test `:842`).
- **Ambiguous consent values**: missing/non-boolean `approved` is treated as rejection, not consent (`spec :374`; defensive coercion at `_types.py:1460-1461`).
- **Mixed batches with unknown calls**: user-input pause takes precedence over unknown-call termination, avoiding partial side effects (`_tools.py:1775-1795` comment).
- **Stale queued prompts**: queued approvals are drained against current rules each turn and cancelled flows clear queues so later turns don't see stale prompts (AG-UI behavior per CHANGELOG `python/CHANGELOG.md:293`; drain logic `_harness/_tool_approval.py:579-591`).
- **Runaway auto-approval loops**: iteration budgets stop loops even when everything is auto-approved (`test_harness_tool_approval.py:1043`).
- **Untrusted skill sources**: MCP-delivered skills can never carry runnable scripts, capping what a malicious server can get approved (`packages/core/AGENTS.md` MCPSkillsSource section).

## Future Considerations

- Promote the experimental gated surfaces (file access, looping, background agents — warned at `_harness/_agent.py:541-561`) to stable once their approval UX settles.
- Consider structural (capability/registry-scoped) auto-approval matching instead of name strings to eliminate the documented collision bypass class (`_harness/_tool_approval.py:365-377`).
- Standardize audit persistence of approval control contents across history providers/session stores (currently optional per `spec :409-412`).
- Track Go port parity for the approval protocol once `microsoft/agent-framework-go` materializes in-repo (`go/README.md`).

## Questions / Gaps

- No evidence found of a single top-level "autonomy level" (e.g., full-auto vs supervised mode) knob; searches across `approval_mode`, `autonomy`, `permission` in `python/packages/core` surfaced only per-tool/per-provider configuration and auto-approval rules. If such a dial exists, it lives outside this source.
- Runtime observability of denials is log-based (WARNING lines, e.g., `_mcp.py:1442-1461`); no dedicated metrics/traces for approval latency or rejection rates were found beyond OpenTelemetry feature-usage marking (`mark_feature_used(FeatureIndex.CORE_TOOL_APPROVAL)` at `_harness/_tool_approval.py:383`).
- .NET-side behavior was spot-checked (directory structure + declarative RequireApproval) but not exhaustively line-audited in this study; the rating primarily reflects the Python implementation.
- The `UserInputRequiredException` propagation path for nested agents (OAuth consent, sub-agent approvals — `exceptions.py:184-188`) was identified but its full escalation flow was not traced end-to-end.

---

Generated by dimension `23.01-autonomy-boundary` against `agent-framework`.
