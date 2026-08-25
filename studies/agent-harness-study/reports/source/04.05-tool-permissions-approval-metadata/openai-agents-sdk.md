# Source Analysis: openai-agents-sdk

## 04.05 — Tool Permissions and Approval Metadata

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, Pydantic v2, OpenAI Responses / Chat Completions / Realtime APIs, MCP, asyncio |
| Analyzed | 2026-08-15 |

## Summary

The OpenAI Agents SDK has a deliberate, narrow permission story: **there is no risk-class enum, no central policy engine, and no allowlist/denylist of tool categories.** Every tool that the runtime can execute declares its own gating via a single `needs_approval` field, and a small set of tools (Hosted MCP, local MCP, agent-as-tool) carry a parallel `require_approval` / `on_approval_request` setting.

The implementation splits cleanly into:

1. **Per-tool declaration.** `FunctionTool.needs_approval` (`src/agents/tool.py:426-433`), `ShellTool.needs_approval` (`src/agents/tool.py:1221-1227`), `ApplyPatchTool.needs_approval` (`src/agents/tool.py:1269-1275`), `CustomTool.needs_approval` (`src/agents/tool.py:1300-1303`), `ComputerTool.on_safety_check` (`src/agents/tool.py:721`), `HostedMCPTool.on_approval_request` (`src/agents/tool.py:970`), and the local `MCPServer` `require_approval` setting normalized in `src/agents/mcp/server.py:386-489`. Each accepts either a `bool` or a synchronous/async callable. The default for every tool type is `False` — the SDK ships with an **opt-in, not opt-out, approval posture**.

2. **Runtime enforcement.** The shared helper `evaluate_needs_approval_setting` (`src/agents/util/_approvals.py:13-31`) collapses `bool | Callable[..., Awaitable[bool]]` into a real `bool` and rejects invalid types with `UserError` (`src/agents/util/_approvals.py:27-31`, tested in `tests/test_hitl_error_scenarios.py:868-910`). The decision is checked inside `_collect_runs_by_approval` (`src/agents/run_internal/tool_planning.py:376-447`) for shell/apply_patch/custom tools and inside the per-tool executors (`src/agents/run_internal/tool_actions.py:456-485` for shell, `:649-674` for custom, and `function_needs_approval` at `src/agents/run_internal/tool_execution.py:1183-1201`). If approval is required and unresolved, the run is parked via `NextStepInterruption(interruptions=...)` (`src/agents/run_internal/turn_resolution.py:713-723`).

3. **Sticky/per-call decision store.** Approval decisions live on `RunContextWrapper._approvals: dict[str, _ApprovalRecord]` (`src/agents/run_context.py:29-41`, `:61`). `_ApprovalRecord.approved` and `.rejected` are either a `bool` (sticky "always for this tool" decision) or a `list[str]` of approved/rejected `call_id`s — see `_apply_approval_decision` (`src/agents/run_context.py:309-353`). `always_approve=True` / `always_reject=True` flip a per-call decision into a permanent one (`src/agents/run_context.py:325-338`).

4. **Durable state.** `_serialize_approvals` round-trips the dictionary into `RunState.to_json()` (`src/agents/run_state.py:359-379`, `:685`) and `_rebuild_approvals` restores it on resume (`src/agents/run_context.py:447-468`). Schema `1.6` introduced explicit rejection-message persistence (`src/agents/run_state.py:140`); current schema is `1.11` (`src/agents/run_state.py:131`). Rejection messages are also configurable globally via `RunConfig.tool_error_formatter` (`src/agents/run_config.py:73-92`, `:308-312`) and per-call via `RunState.reject(..., rejection_message=...)` (`src/agents/run_state.py:338-357`).

5. **Realtime parity.** `RealtimeSession._function_needs_approval` and `_maybe_request_tool_approval` (`src/agents/realtime/session.py:471-578`) reuse `evaluate_needs_approval_setting` and `RunContextWrapper` so the same approval store backs both batched and streaming runs; the user-facing API is `approve_tool_call` / `reject_tool_call` on the realtime session (`src/agents/realtime/session.py:733-797`).

> **Can the runtime stop a high-risk tool even if the model asks for it?** Yes. `_collect_runs_by_approval` (`src/agents/run_internal/tool_planning.py:405-411`) skips any tool whose stored decision is `False` and emits a `ToolCallOutputItem` containing the configured rejection text instead of running it; `_execute_function_tool_calls` mirrors the same gate (`src/agents/run_internal/tool_execution.py:2233-2278`). The function body never executes — there is no path that calls `tool.on_invoke_tool(...)` while `approval_status is False`.

## Rating

**6 / 10** — Clear, documented, and runtime-enforced approval model with rich persistence and explicit callable policies per tool. Pulls down from higher tiers because there is no central risk taxonomy (every tool starts "safe" by default and the developer must remember to set `needs_approval`), the only "block a model-requested tool" lever outside MCP is the per-tool `needs_approval` flag (no global allowlist/denylist, no secret/network/egress classification), and pre-approval input guardrails are an opt-in toggle (`src/agents/run_config.py:106-118`, gated on `tool_execution.pre_approval_tool_input_guardrails`).

## Evidence Collected

Every entry includes file paths and line numbers from the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-tool approval field, function tools | `FunctionTool.needs_approval: bool \| Callable[[RunContextWrapper, dict, str], Awaitable[bool]] = False` | `src/agents/tool.py:426-433` |
| Per-tool approval field, shell (local executor) | `ShellTool.needs_approval: bool \| ShellApprovalFunction` + `on_approval` callback | `src/agents/tool.py:1221-1231` |
| Per-tool approval field, apply_patch (file writes) | `ApplyPatchTool.needs_approval: bool \| ApplyPatchApprovalFunction` + `on_approval` | `src/agents/tool.py:1269-1279` |
| Per-tool approval field, custom tools (raw input) | `CustomTool.needs_approval` / `runtime_needs_approval()` | `src/agents/tool.py:1300-1331` |
| Hosted MCP approval callback | `HostedMCPTool.on_approval_request: MCPToolApprovalFunction` | `src/agents/tool.py:970-973`, `:883-886` |
| Local MCP `require_approval` policy normalization | `_normalize_needs_approval` collapses `Literal["always","never"]` / dict / callable to `bool \| dict[str,bool] \| callable` | `src/agents/mcp/server.py:386-489` |
| Local MCP per-tool policy resolver | `_get_needs_approval_for_tool` returns a callable when policy is callable, mapping lookup when dict, bool fallback | `src/agents/mcp/server.py:491-522` |
| Hosted shell sandbox disallows approval | `ShellTool.__post_init__` rejects `needs_approval` / `on_approval` for hosted envs and clears both fields | `src/agents/tool.py:1251-1256` |
| Computer tool safety gate | `ComputerTool.on_safety_check: Callable[[ComputerToolSafetyCheckData], MaybeAwaitable[bool]]` (provider-driven acknowledgements, not approval of the call) | `src/agents/tool.py:721`, `:845-859` |
| Shared approval evaluator (bool → callable → awaitable) | `evaluate_needs_approval_setting`; strict-mode `UserError` for invalid types | `src/agents/util/_approvals.py:13-31` |
| Approval record storage on run context | `_ApprovalRecord` dataclass with `approved: bool\|list[str]`, `rejected: bool\|list[str]`, `rejection_messages`, `sticky_rejection_message` | `src/agents/run_context.py:29-41` |
| Per-call / sticky decision application | `_apply_approval_decision` writes `bool` for sticky, `list[str]` for per-call | `src/agents/run_context.py:309-353` |
| Public approve / reject API | `RunContextWrapper.approve_tool`, `reject_tool(..., always_reject, rejection_message)` | `src/agents/run_context.py:355-375` |
| Status lookup with namespace awareness | `RunContextWrapper.get_approval_status` resolves bare + namespaced + lookup-key candidates | `src/agents/run_context.py:377-445` |
| `ToolApprovalItem` (the pause unit) | Dataclass with `agent`, `raw_item`, `tool_name`, `tool_namespace`, `tool_lookup_key`, `tool_origin`, `_allow_bare_name_alias`; explicit "cannot be sent as input" guard | `src/agents/items.py:509-636` |
| Interrupt surface | `NextStepInterruption(interruptions: list[ToolApprovalItem])` causes the run to pause; collected in `SingleStepResult` | `src/agents/run_internal/turn_resolution.py:699-723` |
| Approval gating at planning time | `_collect_runs_by_approval` skips runs whose status is `False`, otherwise evaluates `needs_approval_checker`, adds pending interruption when `True` is missing | `src/agents/run_internal/tool_planning.py:376-447` |
| Approval gating at resume time | `_select_function_tool_runs_for_resume` short-circuits `needs_approval_checker` when an explicit decision is already stored | `src/agents/run_internal/tool_planning.py:490-539` |
| Approval gating inside shell executor | `execute_shell_calls` invokes `evaluate_needs_approval_setting` and `resolve_approval_status` before the executor | `src/agents/run_internal/tool_actions.py:456-485` |
| Approval gating inside custom-tool executor | `execute_custom_tool_calls` mirrors shell gating | `src/agents/run_internal/tool_actions.py:645-674` |
| Approval gating inside apply_patch executor | `execute_apply_patch_calls` resolves approval per operation | `src/agents/run_internal/tool_actions.py:818-842` |
| Approval gating for function tools (resume path) | `tool_execution.py:_resolve_tool_run` rejects unresolved / rejected function-tool approvals before scheduling | `src/agents/run_internal/tool_execution.py:2233-2278` |
| MCP approval pipeline | `execute_mcp_approval_requests` → `MCPApprovalResponseItem`; `collect_manual_mcp_approvals` bridges manual decisions; `process_hosted_mcp_approvals` filters by `approval_request_id` | `src/agents/run_internal/tool_planning.py:99-137`, `src/agents/run_internal/tool_execution.py:1204-1326` |
| Default rejection text | `DEFAULT_APPROVAL_REJECTION_MESSAGE = "Tool execution was not approved."` | `src/agents/tool.py:181`, re-exported as `REJECTION_MESSAGE` in `src/agents/run_internal/items.py:20` |
| Per-tool / per-call rejection message override | `resolve_approval_rejection_message` looks up stored message, then `RunConfig.tool_error_formatter`, then SDK default | `src/agents/run_internal/tool_execution.py:1127-1180` |
| Rejection messages survive serialization | `RunState._serialize_approvals` round-trips `rejection_messages` and `sticky_rejection_message` | `src/agents/run_state.py:359-379` |
| Schema versioning for approval payloads | `CURRENT_SCHEMA_VERSION = "1.11"`, `SCHEMA_VERSION_SUMMARIES` 1.0..1.11 (1.6 added explicit rejection messages) | `src/agents/run_state.py:131-149` |
| Forward-compatibility contract | "Forward compatibility is intentionally fail-fast (older SDKs reject newer or unsupported versions)" | `src/agents/run_state.py:128-130` |
| Per-tool disable (separate from approval) | `FunctionTool.is_enabled: bool \| Callable[[RunContextWrapper, AgentBase], MaybeAwaitable[bool]]` | `src/agents/tool.py:412-415`, evaluated in `src/agents/agent.py:250-263` |
| MCP static tool filter (allow/block lists) | `ToolFilterStatic` with `allowed_tool_names` / `blocked_tool_names`; applied in `_filter_tools` | `src/agents/mcp/util.py:118-127`, `src/agents/mcp/server.py:644-660` |
| Realtime session approval plumbing | `_function_needs_approval`, `_maybe_request_tool_approval`, `approve_tool_call`, `reject_tool_call`; pre-approval input-guardrail toggle | `src/agents/realtime/session.py:471-578`, `:733-797`, `:580-586` |
| Sandbox reuses SDK approval primitive | `SandboxShellTool.needs_approval`, `SandboxApplyPatchTool.needs_approval` wrap the same callable surface | `src/agents/sandbox/capabilities/tools/shell_tool.py:166-178`, `src/agents/sandbox/capabilities/tools/apply_patch_tool.py:154-188` |
| Tests — invalid type raises `UserError` | `test_function_needs_approval_invalid_type_raises`, `test_resume_invalid_needs_approval_raises` | `tests/test_hitl_error_scenarios.py:868-910` |
| Tests — apply_patch approval item + rejection | `test_apply_patch_tool_needs_approval_returns_approval_item`, `..._rejected_returns_rejection`, `...on_approval_callback_auto_approves`, `..._auto_rejects` | `tests/test_apply_patch_tool.py:251-376` |
| Tests — context-level approval record semantics | latest-wins, namespaced vs. bare, deferred lookup, per-call / always | `tests/test_run_context_approvals.py:1-233` |
| Tests — internal approval helpers | `_build_function_tool_call_for_approval_error` namespace handling, `filter_tool_approvals`, `append_approval_error_output` | `tests/test_run_internal_approvals.py:1-130` |
| Tests — runner-level approvals | `tests/test_agent_runner.py:803-938`, `tests/test_agent_runner_streamed.py:1697-1947`, `tests/test_hitl_session_scenario.py:40-50` |
| Tests — agent-as-tool approvals and nested interruptions | `test_agent_as_tool.py:1240-1330` and `test_hitl_error_scenarios.py:926-1010` |
| Tests — resume skips `needs_approval_checker` when status is resolved | `test_resume_skips_needs_approval_checker_when_status_resolved` | `tests/test_hitl_error_scenarios.py:1251-1296` |
| Tests — denied tool never runs | `shell_tool = ShellTool(..., needs_approval=True)` followed by `context_wrapper.reject_tool(...)`; executor must not be called | `tests/test_hitl_error_scenarios.py:1613-1640` |
| HITL documentation | "Marking tools that need approval", "How the approval flow works", "Custom rejection messages", "Streaming and sessions", "Long-running approvals" | `docs/human_in_the_loop.md:11-205` |

## Answers to Dimension Questions

1. **Are tools risk-classified?** No. There is no `RiskLevel` / `RiskClass` / "dangerous" enum in the SDK (`grep -rn "RiskLevel\|risk_level\|RiskClass\|risk_class"` returns nothing across the source tree). The closest analogue is `ComputerTool.on_safety_check` (`src/agents/tool.py:721`) — a provider-driven acknowledgment rather than a category label. Risk is purely declarative per tool via `needs_approval`.

2. **Are permissions enforced?** Yes, end-to-end. The planning layer (`src/agents/run_internal/tool_planning.py:376-447`), the per-tool executors (`src/agents/run_internal/tool_actions.py:456-485`, `:645-674`, `:818-842`), and the resume path (`src/agents/run_internal/tool_execution.py:2233-2278`) all gate execution on `evaluate_needs_approval_setting` + `RunContextWrapper.get_approval_status(...)`. When status is `False`, the executor body is skipped and a rejection output is appended. `tests/test_hitl_error_scenarios.py:1613-1640` verifies the executor is never invoked after `reject_tool`.

3. **Can users approve selectively?** Yes. Per-call approvals are scoped to the exact `call_id` (`src/agents/run_context.py:206-211`, `:339-346`). `always_approve=True` / `always_reject=True` (passed to `RunState.approve` / `RunState.reject`, `src/agents/run_state.py:332-357`) flips a single decision into a sticky one. Namespaced approvals (`tool_namespace`, `tool_lookup_key`) keep decisions scoped to the namespace, with bare-name aliasing for legacy deferred tools (`src/agents/run_context.py:99-124`, `src/agents/_tool_identity.py`).

4. **Are approvals persisted?** Yes. Approval state lives on `RunContextWrapper._approvals` (`src/agents/run_context.py:29-41`, `:61`) and is serialized by `RunState.to_json()` / `to_string()` (`src/agents/run_state.py:359-379`, `:657-774`). Schema version `1.6` introduced explicit `rejection_messages` persistence (`src/agents/run_state.py:140`); the current schema is `1.11` (`src/agents/run_state.py:131`). `_rebuild_approvals` restores them on resume (`src/agents/run_context.py:447-468`).

5. **Can policy block a model-requested tool?** Indirectly, via three mechanisms:
   - **Local MCP**: `_normalize_needs_approval` returns a per-tool `bool` mapping (`src/agents/mcp/server.py:448-470`); `_get_needs_approval_for_tool` (`src/agents/mcp/server.py:491-522`) returns `True` whenever the policy is callable but the agent context is missing — documented as "fail-closed" at `:498-501`. A callable policy is the most expressive lever and can deny any call.
   - **Tool filtering**: MCP static `ToolFilterStatic.allowed_tool_names` / `blocked_tool_names` (`src/agents/mcp/util.py:118-127`) drops tools before they reach the model.
   - **Per-tool disable**: `FunctionTool.is_enabled` evaluated before the tool is offered (`src/agents/agent.py:250-263`).
   There is no top-level global "block all tools that touch network/secrets" — risk gating is opt-in per tool.

## Architectural Decisions

- **Opt-in approval posture.** `needs_approval` defaults to `False` on every tool type (`src/agents/tool.py:428`, `:1221`, `:1269`, `:1300`). The SDK does not try to guess that a tool is dangerous; the developer who wraps the function takes responsibility for declaring risk. Documented at `docs/human_in_the_loop.md:11-38`.

- **Bool-or-callable contract.** Every approval setting accepts either a `bool` or a callable whose arguments vary by tool type (e.g. `(ctx, parsed_args, call_id)` for `FunctionTool`, `(ctx, action, call_id)` for `ShellTool`, `(ctx, tool_approval_item)` for `on_approval`). `evaluate_needs_approval_setting` (`src/agents/util/_approvals.py:13-31`) is the single shared normalizer so realtime, batch, and sandbox runs all evaluate identically.

- **Decision store keyed by tool identity, not by risk.** `_ApprovalRecord` keys decisions by canonical name (with namespace + lookup-key variants) rather than by risk class. The same `tool_name` can therefore carry independent "approve one call" and "reject all calls" semantics in parallel (`src/agents/run_context.py:188-211`, `:325-338`).

- **Interruption-driven pause.** Approval-required calls are converted into `ToolApprovalItem` entries and emitted as `NextStepInterruption.interruptions` (`src/agents/run_internal/turn_resolution.py:713-723`). The `RunResult` exposes `interruptions`; the caller resolves via `RunState.approve` / `RunState.reject` (`src/agents/run_state.py:323-357`) and re-runs. `ToolApprovalItem.to_input_item()` deliberately raises to prevent the placeholder from being sent back to the model (`src/agents/items.py:631-636`).

- **Programmatic fast path.** For tools that can decide without human input (`ShellTool.on_approval`, `ApplyPatchTool.on_approval`, `CustomTool.on_approval`, `HostedMCPTool.on_approval_request`), `resolve_approval_status` (`src/agents/run_internal/tool_execution.py:1061-1110`) invokes the callback immediately and records the result, so the run never pauses when the app can decide.

- **Schema-gated persistence.** `RunState` explicitly rejects unsupported future schema versions (`src/agents/run_state.py:128-130`) and tracks schema evolution in `SCHEMA_VERSION_SUMMARIES` (`src/agents/run_state.py:133-149`). Approval state is part of that durable boundary, so an interrupted HITL run can be resumed in a different process or even later.

- **MCP is treated as a separate surface.** Local MCP servers have their own `require_approval` policy normalized to `bool | dict[str, bool] | callable` (`src/agents/mcp/server.py:386-489`) and a fail-closed callable branch (`src/agents/mcp/server.py:498-508`). Hosted MCP approvals are bridged to `MCPApprovalResponseItem` keyed by `approval_request_id` (`src/agents/run_internal/tool_execution.py:1204-1264`, `src/agents/run_internal/tool_planning.py:99-137`).

- **Realtime shares the same store.** `RealtimeSession._function_needs_approval` and `_maybe_request_tool_approval` (`src/agents/realtime/session.py:471-578`) call `RunContextWrapper.get_approval_status` so approval decisions made in a synchronous batch run are visible to a subsequent realtime session and vice-versa.

## Notable Patterns

- **Callable per-call policy with strict validation.** The helper raises `UserError` (not a silent default) for non-bool, non-callable inputs (`src/agents/util/_approvals.py:27-31`). Test coverage: `tests/test_hitl_error_scenarios.py:868-910`.
- **Short-circuit when decision is already stored.** `_select_function_tool_runs_for_resume` (`src/agents/run_internal/tool_planning.py:521-533`) deliberately skips invoking `needs_approval_checker` when the status is already resolved; the comment notes this prevents user-side effects and exception swallowing (`src/agents/run_internal/tool_planning.py:525-528`). Tested at `tests/test_hitl_error_scenarios.py:1251-1296`.
- **Per-call vs. sticky layered semantics.** `_ApprovalRecord` (`src/agents/run_context.py:29-41`) supports `bool` (sticky) and `list[str]` (per-call) at the same time; `_apply_approval_decision` (`src/agents/run_context.py:309-353`) keeps them coherent.
- **Sticky rejection message.** `_ApprovalRecord.sticky_rejection_message` (`src/agents/run_context.py:40`) lets a "always reject" decision carry a stable model-visible message even when the call ID has been reissued.
- **Pre-approval input-guardrail toggle.** `RunConfig.tool_execution.pre_approval_tool_input_guardrails` (`src/agents/run_config.py:106-118`) optionally runs tool-input guardrails before the pause event so dangerous inputs never reach the human in cleartext — wired into `_maybe_request_tool_approval` (`src/agents/realtime/session.py:549-562`).
- **Failure-mode isolation.** Approval errors that escape a custom policy are swallowed into `needs_approval=True` (`src/agents/run_internal/turn_resolution.py:929-932`, `src/agents/run_internal/tool_planning.py:417-421`), preserving a fail-closed default for arbitrary user code.
- **Documented "Long-running approvals" story.** `docs/human_in_the_loop.md:181-205` ties `RunState.to_json()` / `from_json()` to the same approval store so paused runs can be stored in queues or databases.

## Tradeoffs

- **No central risk taxonomy.** The SDK forces every author of a function tool to remember `needs_approval=True` for destructive actions; the runtime cannot catch a forgotten flag. Compare to `agent-framework`'s `RiskLevel`-style primitives (referenced for context only — not present in this source).
- **Default-off approval.** Safer for prototype velocity but means a "drop-in" function tool is unguarded until the developer wraps it. The trade-off is intentional and documented (`docs/human_in_the_loop.md:11-20`).
- **Custom policy is user code, not sandboxed.** Callable `needs_approval` policies execute in the runner's event loop with full Python privileges. The SDK does not sandbox or rate-limit these callbacks, so a misbehaving policy can starve the run or raise exceptions — though exceptions are converted into "needs approval" (`src/agents/run_internal/tool_planning.py:417-421`) rather than aborting the run.
- **Approval store is process-local.** `_approvals` lives on the `RunContextWrapper` and is durable only when explicitly serialized into `RunState`. Long-running deployments must opt in by calling `state.to_json()` / `state.to_string()` (`src/agents/run_state.py:657-774`).
- **Hosted shell bypasses approval entirely.** `ShellTool` with a hosted `container_auto` / `container_reference` environment forbids `needs_approval` and forces it to `False` (`src/agents/tool.py:1251-1256`), pushing the approval question onto the OpenAI side (`docs/human_in_the_loop.md:175`).
- **No central secret / network classifier.** A function tool that reads secrets from disk and posts them externally looks identical to a function tool that reads a JSON file locally. There is no enum or annotation to label network or secret access — only per-tool callbacks.
- **Schema-versioning strictness.** Older SDKs reject newer `RunState` payloads (`src/agents/run_state.py:128-130`) — operationally robust, but operators that upgrade mid-pause must rebuild state.

## Failure Modes / Edge Cases

- **Missing `call_id`.** `extract_tool_call_id` returns `None` rather than raising (`src/agents/run_internal/tool_execution.py:597-606`); per-call approvals require a `call_id`, so the SDK falls back to "approve / reject all future calls to this tool" semantics when the ID is absent (`src/agents/run_context.py:320-322`).
- **Callable policy raising.** User exceptions in `needs_approval` callables are caught and converted to `needs_approval=True` (`src/agents/run_internal/tool_planning.py:417-421`, `src/agents/run_internal/turn_resolution.py:929-932`) — fail-closed for arbitrary policy code, at the cost of masking bugs in custom policies.
- **Invalid `needs_approval` type.** Caught at evaluation time with `UserError` (`src/agents/util/_approvals.py:27-31`), covered by `tests/test_hitl_error_scenarios.py:868-910`. Resume path also re-raises (`:883-910`).
- **Approval not resolvable during resume.** `_select_function_tool_runs_for_resume` (`src/agents/run_internal/tool_planning.py:529-537`) re-runs the checker only when status is `None`; if the checker itself can't decide (e.g. an exception, or a `None` boolean from a custom policy), the call is parked again as a pending interruption.
- **Duplicate-name agent identity.** Schema 1.6 and earlier resolved approvals by `agent.name`; with duplicate names, resume might match the wrong agent instance. Schema 1.7 added identity persistence; the code accepts a same-name match only for legacy snapshots (`src/agents/run_internal/turn_resolution.py:1132-1154`).
- **Namespaced vs. bare tool name collisions.** `get_approval_status` walks multiple candidate keys (qualified, bare, namespace) to avoid leaking an "always approve" decision from a bare-name tool onto a namespaced variant (`src/agents/run_context.py:377-445`); covered by `tests/test_run_context_approvals.py:23-100`.
- **Hostless MCP tool approval.** When `_get_needs_approval_for_tool` is called without an agent and the policy is callable, the SDK returns `True` to preserve historical fail-closed behavior (`src/agents/mcp/server.py:498-508`).
- **Rejection-message fallback chain.** `resolve_approval_rejection_message` (`src/agents/run_internal/tool_execution.py:1127-1180`) cascades: stored per-call → stored sticky → `RunConfig.tool_error_formatter` → default text. If the formatter raises or returns a non-string, the SDK logs and falls back to the default (`:1165-1178`).
- **Schema forward-compat failure.** Newer-than-supported `RunState` payloads raise `UserError` (`src/agents/run_state.py:1050-1055`) — operationally protective but not graceful when upgrading mid-resume.

## Future Considerations

- A **first-class risk taxonomy** (e.g. `ToolRisk.READ_ONLY`, `EXECUTE_CODE`, `NETWORK`, `SECRETS`, `MONEY`) wired into default `needs_approval` would catch forgotten flags. Today the SDK relies entirely on the wrapper author.
- **Global deny/allow policies** scoped by risk class (e.g. "any tool marked `EXECUTE_CODE` must be approved") would close the gap between opt-in `needs_approval` and policy-level enforcement.
- **Pre-approval input sandboxing** beyond the existing `pre_approval_tool_input_guardrails` toggle (`src/agents/run_config.py:106-118`) — e.g. argument-size caps, secret-pattern redaction, command-syntax linting — is currently the developer's responsibility.
- **Schema migration tooling.** `RunState.from_json` is fail-fast on unknown schema versions (`src/agents/run_state.py:128-130`); explicit migration helpers or auto-conversion between adjacent versions would smooth long-running approval queues.
- **Observability.** Approval decisions are traced via `RunState.approve`/`reject` calls but there is no aggregated metric hook for "approvals requested / denied / approved" — adding a hook into `RunContextWrapper` (e.g. `on_approval_decision`) would give operators a runtime signal.
- **Hosted MCP `on_approval_request` parity for hosted shell.** Hosted shell already disables approval entirely (`src/agents/tool.py:1251-1256`); a future hosted-shell approval hook would symmetrize the surface.

## Questions / Gaps

- **Risk classification is implicit.** No explicit evidence found for a central risk enum or annotation beyond the per-tool `needs_approval` flag.
- **Approval observability hooks.** No evidence found for SDK-emitted telemetry or metrics around approval requests, denials, or persistence. `RunContextWrapper` exposes the store but no observer API.
- **Cross-run approval reuse.** Approvals are durable inside a `RunState` (serialized), but there is no evidence that two independent runs share an approval store unless the user wires `RunState.from_json` themselves. No persistent "global approval ledger" is shipped.
- **Secret / network policy enforcement.** No evidence found for SDK-level enforcement on what a function tool may do inside its own body — the SDK only gates the call boundary.
- **Policy-block semantics for non-MCP tools.** For local MCP, the policy can map per-tool to "always require" or "never require" (`src/agents/mcp/server.py:386-489`); for plain `FunctionTool` there is no equivalent central registry — the only analogue is `is_enabled` (`src/agents/tool.py:412-415`), which is per-tool rather than policy-driven.
- **Concurrency of approval decisions.** Approvals are stored on a shared dict keyed by tool identity; if the same tool call id is approved twice from different coroutines the latest write wins (`src/agents/run_context.py:339-346`), but there is no documented lock for concurrent decisions across `RunState` resume + live decisions.
- **Tests for the "denied tool never runs" invariant.** Covered for shell / apply_patch / function (`tests/test_hitl_error_scenarios.py:1613-1640`, `:1822-1895`, `tests/test_apply_patch_tool.py:251-376`), but no explicit assertion found for `HostedMCPTool` denial of an `on_approval_request`-returning `False`.

---

Generated by `04.05-tool-permissions-and-approval-metadata` against `openai-agents-sdk`.
