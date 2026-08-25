# Source Analysis: agent-framework

## 04.05 — Tool Permissions and Approval Metadata

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python 3.10+ (core package), Pydantic v2, MCP, optional provider packages (openai, anthropic, foundry, ollama, …) |
| Analyzed | 2026-07-27 |

## Summary

The Python `agent-framework` core implements tool permissions and approval metadata through several complementary mechanisms:

1. A primitive `ApprovalMode = Literal["always_require", "never_require"]` flag on every `FunctionTool` (`agent_framework/_tools.py:96`), surfaced via the `@tool(...)` decorator (`agent_framework/_tools.py:1147`) and the `Agent.as_tool(...)` factory (`agent_framework/_agents.py:485`). This is the only first-class "needs approval" bit on the tool object itself.
2. A provider-specific richer permission schema for MCP tools: `MCPSpecificApproval` (`agent_framework/_mcp.py:58`) maps a denylist/allowlist of remote tool names onto `always_require`/`never_require`.
3. A "tool kind" field (`agent_framework/_tools.py:92` declares `SHELL_TOOL_KIND_VALUE = "shell"`, exposed via `FunctionTool(kind=...)` at `agent_framework/_tools.py:307`) that provider packages use to mark execution-capable / shell tools (e.g. `openai/_chat_client.py:911`, `anthropic/_chat_client.py:883`).
4. The experimental `ToolApprovalMiddleware` harness (`agent_framework/_harness/_tool_approval.py:345`) that intercepts `function_approval_request` Content emitted by the model, persists standing rules in session state, supports heuristic auto-approval, and re-prompts only on changed arguments.
5. An FIDES-experimental Information-Flow-Control (IFC) middleware layer (`agent_framework/security.py`) that augments approval with deny-by-default policy enforcement based on `IntegrityLabel.TRUSTED/UNTRUSTED` context-labels and `ConfidentialityLabel` ceilings per-tool (`accepts_untrusted`, `max_allowed_confidentiality` on `additional_properties`).
6. MCP server-initiated sampling is denied-by-default unless the user installs an explicit `sampling_approval_callback` (`agent_framework/_mcp.py:1045`, `:1099-1108`).

The harness factory `create_harness_agent` (`agent_framework/_harness/_agent.py:239`) wires `ToolApprovalMiddleware` by default (outermost), so approval handling is "on" for any harness-built agent unless explicitly disabled (`disable_tool_auto_approval=True`, `_harness/_agent.py:270`, `:504-505`).

The runtime can absolutely stop a high-risk tool even if the model asks for it: `_try_execute_function_calls` (`_tools.py:1691-1755`) builds an `approval_tools` set from `tool.approval_mode == "always_require"` and short-circuits the invocation loop, returning `Content.from_function_approval_request(...)` items; the function is never entered. Conversely, `FunctionTool.__call__` also enforces a hard `max_invocations`/`max_invocation_exceptions` ceiling (`_tools.py:520-531`).

## Rating

**8 / 10 (Clear model with tests, explicit interfaces, and operational safeguards).**

The primitive model is explicit (`ApprovalMode`), the harness middleware is well-tested (823-line test module + 24 scenarios including hosted-server boundaries, mixed batch hiding, streaming, and auto-rules), and the runtime never invokes a function whose `approval_mode == "always_require"` without an approval response. Scores are not 9-10 because (a) the `additional_properties` schema for `accepts_untrusted` / `source_integrity` / `max_allowed_confidentiality` is undocumented and lives as a string-keyed dict (no Pydantic validation at registration time), (b) there is no central allowlist/denylist beyond "approval mode" + MCP per-tool mapping + the FIDES middleware allow-set, and (c) "selective approval by policy" is only realized through the still-experimental FIDES layer.

## Evidence Collected

Every entry includes `path:line`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission primitive | `ApprovalMode = Literal["always_require", "never_require"]` | `python/packages/core/agent_framework/_tools.py:96` |
| Tool-kind constant | `SHELL_TOOL_KIND_VALUE: Final[str] = "shell"` (provider-agnostic risk classification knob) | `python/packages/core/agent_framework/_tools.py:92` |
| Tool-class permission field | `FunctionTool(..., approval_mode: ApprovalMode \| None = None, kind: str \| None = None, max_invocations, max_invocation_exceptions, additional_properties)` | `python/packages/core/agent_framework/_tools.py:301-314` |
| Permission storage on instance | `self.approval_mode = approval_mode or "never_require"` | `python/packages/core/agent_framework/_tools.py:398` |
| Runtime **enforcement** – short-circuit before execution | `approval_tools = {tool_name for tool_name, tool in tool_map.items() if tool.approval_mode == "always_require"}` + early `function_approval_request` return | `python/packages/core/agent_framework/_tools.py:1691-1755` |
| Hard invocation ceiling | `if self.max_invocations is not None and self.invocation_count >= self.max_invocations: raise ToolException(...)` | `python/packages/core/agent_framework/_tools.py:520-523` |
| Decorator surface for permissions | `@tool(approval_mode=...)` and `@tool(kind=...)` typed overloads | `python/packages/core/agent_framework/_tools.py:1147-1190` |
| MCP-specific approval mapping | `class MCPSpecificApproval(TypedDict, total=False): always_require_approval, never_require_approval` | `python/packages/core/agent_framework/_mcp.py:58-69` |
| MCP per-tool resolution | `_determine_approval_mode(*candidate_names)` | `python/packages/core/agent_framework/_mcp.py:1301-1315` |
| MCP approval wiring for prompts | `approval_mode = self._determine_approval_mode(local_name, normalized_name, prompt.name)` | `python/packages/core/agent_framework/_mcp.py:1384-1389` |
| MCP approval wiring for tools | same pattern on tool load | `python/packages/core/agent_framework/_mcp.py:1496-1513` |
| MCP approval across all transports | `MCPStdioTool`, `MCPStreamableHTTPTool`, `MCPWebsocketTool` constructors all accept the same `approval_mode` shape with identical docstring | `python/packages/core/agent_framework/_mcp.py:2350/2530/2768` (signatures); docstrings at `:2394-2399`, `:2575-2580`, `:2810-2815` |
| MCP server-initiated sampling is deny-by-default | `if callback is None: ... return False`; logged at WARNING | `python/packages/core/agent_framework/_mcp.py:1045-1073` |
| MCP sampling request rate cap + cap on `maxTokens` | `_DEFAULT_SAMPLING_MAX_REQUESTS=25`, `_DEFAULT_SAMPLING_MAX_TOKENS=4096` | `python/packages/core/agent_framework/_mcp.py:110-111` |
| MCP sampling approval gate | `if not await self._sampling_request_approved(params): return types.ErrorData(...)` | `python/packages/core/agent_framework/_mcp.py:1150-1159` |
| MCP framework kwarg denylist (defense-in-depth) | `_MCP_FRAMEWORK_DENYLIST = frozenset({chat_options, tools, tool_choice, session, thread, conversation_id, options, response_format, _meta})` | `python/packages/core/agent_framework/_mcp.py:88-98` |
| Delegated agent-to-agent tool permission | `Agent.as_tool(..., approval_mode: Literal["always_require", "never_require"] = "never_require", ...)` | `python/packages/core/agent_framework/_agents.py:485-579` |
| Approval request/response Content shape | `Content.from_function_approval_request(...)`, `Content.from_function_approval_response(...)`, `Content.to_function_approval_response(...)` | `python/packages/core/agent_framework/_types.py:1213-1299` |
| ContentTypes registered (visible to runtime) | `"function_approval_request"`, `"function_approval_response"` enumerated in `Content` types | `python/packages/core/agent_framework/_types.py:360-361` |
| Approval state on Content | `Content.id`, `Content.function_call`, `Content.approved`, `Content.user_input_request` | `python/packages/core/agent_framework/_types.py:517-520, 570-573` |
| `ToolApprovalMiddleware` opt-in harness middleware | `@experimental class ToolApprovalMiddleware(AgentMiddleware)` | `python/packages/core/agent_framework/_harness/_tool_approval.py:345-632` |
| Standing rule serialization | `ToolApprovalRule(tool_name, arguments=None, server_label=None)` with `to_dict`/`from_dict` | `python/packages/core/agent_framework/_harness/_tool_approval.py:86-156` |
| Rules persisted in session state | `_get_state(session, source_id=...)` reads/writes `session.state[source_id]` | `python/packages/core/agent_framework/_harness/_tool_approval.py:250-277` |
| Heuristic auto-approval hooks | `ToolApprovalMiddleware(auto_approval_rules: Sequence[ToolApprovalRuleCallback])` | `python/packages/core/agent_framework/_harness/_tool_approval.py:354-369` |
| "Don't ask again" tool-wide rule helper | `create_always_approve_tool_response(request, *, reason=None)` writes `additional_properties["tool_approval"]["always_approve"] = "tool"` | `python/packages/core/agent_framework/_harness/_tool_approval.py:220-247` |
| Argument-exact rule helper | `create_always_approve_tool_with_arguments_response(...)` (scope=`"tool_with_arguments"`) | `python/packages/core/agent_framework/_harness/_tool_approval.py:236-247` |
| Hosted-tool server-label boundary | Rules match on `server_label` so same-named tools on different remote servers cannot share approvals | `python/packages/core/agent_framework/_harness/_tool_approval.py:61-65, 310-323` |
| Reasoning for "all-or-nothing" visible approval batch | Comment: "if so, we return approval request for all" | `python/packages/core/agent_framework/_tools.py:1700-1715` |
| Mixed-batch already-approved bypass | `_store_already_approved_approval_requests` stores hidden approvals keyed by visible approval ids; `_pop_already_approved_approval_responses` rehydrates them when user replies | `python/packages/core/agent_framework/_tools.py:1974-2037` |
| Replay/resume via stored already-approved requests | `_pop_already_approved_approval_responses(...)` keyed on `approval_response_ids` | `python/packages/core/agent_framework/_tools.py:1998-2037` |
| Hosting request flow into tool loop | `_replace_approval_contents_with_results(messages, fcc_todo, approved_function_results)` swaps approval items for `function_result` items | `python/packages/core/agent_framework/_tools.py:2059-2153` |
| Rejection result for denied calls | Replaced with `Content.from_function_result(result="Error: Tool call invocation was rejected by user.")` and `msg.role = "tool"` | `python/packages/core/agent_framework/_tools.py:2123-2129` |
| Approval metadata exception: hosted tools pass through with `server_label` on `function_call.additional_properties` | `_is_hosted_tool_approval(content)` | `python/packages/core/agent_framework/_tools.py:1927-1937` |
| Harness factory default: tool approval middleware on | `assembled_middleware.append(ToolApprovalMiddleware(auto_approval_rules=auto_approval_rules))` (always unless `disable_tool_auto_approval`) | `python/packages/core/agent_framework/_harness/_agent.py:499-507` |
| Harness opt-out flag | `disable_tool_auto_approval: bool = False` parameter exposed | `python/packages/core/agent_framework/_harness/_agent.py:270,396-405` |
| Auto-approval rules parameter | `auto_approval_rules: Sequence[ToolApprovalRuleCallback] \| None` | `python/packages/core/agent_framework/_harness/_agent.py:271,401-405` |
| File-access provider hard-codes `delete` as `always_require` (default) | `delete_approval_mode: ApprovalMode = "always_require" if self.require_delete_approval else "never_require"` | `python/packages/core/agent_framework/_harness/_file_access.py:1147-1149` |
| File-access provider exposes `require_delete_approval=False` opt-out | constructor kwarg | `python/packages/core/agent_framework/_harness/_file_access.py:1086-1109` |
| File-access non-destructive tools stay `never_require` | `@tool(name="file_access_save_file", ..., approval_mode="never_require")` etc. | `python/packages/core/agent_framework/_harness/_file_access.py:1121/1135/1161/1172/1189` |
| Background-agent tools all default `never_require` | All six `@tool(...)` registrations in background agents provider | `python/packages/core/agent_framework/_harness/_background_agents.py:316/350/401/420/433/474` |
| Todo tools all `never_require` | five `@tool(...)` registrations | `python/packages/core/agent_framework/_harness/_todo.py:505/528/563/577/585` |
| Mode tools all `never_require` | `mode_set`, `mode_get` | `python/packages/core/agent_framework/_harness/_mode.py:289/299` |
| Memory tools all `never_require` | six `@tool(...)` registrations | `python/packages/core/agent_framework/_harness/_memory.py:1196/1207/1216/1233/1247/1259` |
| Skill scripts require approval when `require_script_approval=True` | `approval_mode="always_require" if require_script_approval else "never_require"` | `python/packages/core/agent_framework/_skills.py:2160` |
| Fidelity `IntegrityLabel` enum | `class IntegrityLabel(str, Enum): TRUSTED, UNTRUSTED` | `python/packages/core/agent_framework/security.py:77-91` |
| Fidelity `ConfidentialityLabel` enum | `PUBLIC, PRIVATE, USER_IDENTITY` | `python/packages/core/agent_framework/security.py:94-110` |
| Integrity/combined helper used at runtime | `combine_labels(*labels)` ("most restrictive wins") | `python/packages/core/agent_framework/security.py:198-250` |
| Tool-level fidelity metadata via `additional_properties` | `source_integrity="trusted"/"untrusted"`, `accepts_untrusted=True`, `max_allowed_confidentiality=...` consumed via `_get_additional_properties(...)` | `python/packages/core/agent_framework/security.py:66-69, 837-846, 1732, 1886-1898` |
| LabelTracking middleware (tier 1 / 2 / 3 priority) | `class LabelTrackingFunctionMiddleware` with tiered priority table | `python/packages/core/agent_framework/security.py:792-848` (priority table 799-809) |
| Auto-hide untrusted content | `auto_hide_untrusted: bool = True` (default) | `python/packages/core/agent_framework/security.py:856,1380-1420` |
| Default integrity is UNTRUSTED (safe-default) | `default_integrity: IntegrityLabel = IntegrityLabel.UNTRUSTED` | `python/packages/core/agent_framework/security.py:852-867` |
| Policy enforcement allowlist | `allow_untrusted_tools: set[str] \| None = None` kwarg | `python/packages/core/agent_framework/security.py:1562-1582` |
| Policy enforcement: block or require-approval mode | `approval_on_violation: bool = False`; `block_on_violation: bool = True` | `python/packages/core/agent_framework/security.py:1565-1583` |
| Policy violation produces function_approval_request | `_request_policy_violation_approval(...)` sets `context.result = Content.from_function_approval_request(id=call_id, function_call=..., additional_properties={policy_violation: True, violation_type, reason, context_label})` then raises `MiddlewareTermination` | `python/packages/core/agent_framework/security.py:1636-1660` |
| Audit log | `self.audit_log: list[dict[str, Any]] = []`; `enable_audit_log=True` default | `python/packages/core/agent_framework/security.py:1583-1584, 1904-1913` |
| Tracked call-ids so replay can be matched | `self._pending_policy_approvals: set[str] = set()`; `call_id = context.metadata["call_id"]` always supplied upstream | `python/packages/core/agent_framework/security.py:1585-1589, 1714, 1814-1820` |
| `accepts_untrusted` per-tool opt-in | `accepts_untrusted = function_props.get("accepts_untrusted", False)` | `python/packages/core/agent_framework/security.py:1732` |
| `max_allowed_confidentiality` per-tool ceiling | `max_allowed_conf = function_props.get("max_allowed_confidentiality", None)` compared to `label.confidentiality` | `python/packages/core/agent_framework/security.py:1886-1898` |
| `SecureAgentConfig` ships two security tools with explicit integrity declarations | `quarantined_llm` declares `source_integrity="untrusted"` and `accepts_untrusted=True` (line 1554-1555); `inspect_variable` runs with `approval_mode="never_require"` (relies on context tainting) | `python/packages/core/agent_framework/security.py:1549-1555, 2495-2501, 2664-2686` |
| Tests: harness middleware hides already-approved side of a mixed batch | `test_mixed_batch_hides_already_approved_request_until_approval_replay` | `python/packages/core/tests/core/test_harness_tool_approval.py:34-88` |
| Tests: standing rule after approve-the-tool-once | `test_tool_approval_middleware_always_approve_tool_rule` | `python/packages/core/tests/core/test_harness_tool_approval.py:556-616` |
| Tests: hosted tool approved on server A does not auto-approve server B | `test_tool_approval_middleware_standing_rules_include_hosted_server_boundary` | `python/packages/core/tests/core/test_harness_tool_approval.py:619-676` |
| Tests: argument-exact rule cannot be widened accidentally | `test_tool_approval_middleware_empty_arguments_rule_is_not_tool_wide` | `python/packages/core/tests/core/test_harness_tool_approval.py:763-822` |
| Tests: file_access delete defaults to always_require, opt-out works | `test_file_access_provider_delete_approval_defaults_to_always_require`, `..._opt_out` | `python/packages/core/tests/core/test_harness_file_access.py:500-545` |
| Tests: policy blocks or requests approval on untrusted context | `test_untrusted_call_blocked`, `test_untrusted_call_requests_policy_approval`, `test_confidentiality_violation_requests_policy_approval`, `test_policy_approved_replay_executes_tool` | `python/packages/core/tests/test_security.py:480-636` |
| Tests: `inspect_variable` ships `approval_mode="never_require"` (tainting-based security) | `test_inspect_variable_uses_generic_approval_mode` | `python/packages/core/tests/test_security.py:949-955` |
| Tests: workflow agent surfaces approval via `request_info` event | `test_agent_executor_tool_call_with_approval`, `..._streaming`, `..._parallel_tool_call_with_approval` | `python/packages/core/tests/workflow/test_agent_executor_tool_calls.py:266-396` |

## Answers to Dimension Questions

**1. Are tools risk-classified?**
Yes, with three concurrent classification systems, none of which is a centralized "risk enum":

- A binary "needs human approval" bit via `FunctionTool.approval_mode` (`_tools.py:96, 398`). Default is `never_require`, opt-in to `always_require` per tool. This is what the function-call loop actually gates on.
- A "kind" string field on `FunctionTool` (`_tools.py:92, 307-372`) — there is exactly one canonical kind today (`SHELL_TOOL_KIND_VALUE = "shell"`, consumed by provider packages like `openai/_chat_client.py:911,945,1193` and `anthropic/_chat_client.py:469,883`) — but no central taxonomy (no `READ_ONLY`, `WRITE`, `NETWORK`, etc.) and no function that uses `kind` to drive policy decisions inside core. Provider packages use it for serialization decisions only.
- An IFC risk model in `LabelTrackingFunctionMiddleware` + `PolicyEnforcementFunctionMiddleware` (`security.py:793,1529`) that classifies *content* (input, result, accumulated context) rather than the tool itself, using `IntegrityLabel.{TRUSTED, UNTRUSTED}` and `ConfidentialityLabel.{PUBLIC, PRIVATE, USER_IDENTITY}`, and lets tools carry `additional_properties={"source_integrity": "...", "accepts_untrusted": bool, "max_allowed_confidentiality": ...}` to opt in or out of policy.

There is no explicit "this tool deletes" or "this tool accesses secrets" classification on the tool object itself; that distinction is implicit in the function body plus the developer's choice of `approval_mode`. The "delete file" tool is a notable exception: the harness `FileAccessProvider` hard-codes `file_access_delete_file` to `always_require` by default, while save/read/list/search remain `never_require` (`_harness/_file_access.py:1147-1149, 1099-1105`).

**2. Are permissions enforced?**
Yes — three layered enforcement points:

- **Loop-level hard block**: `_try_execute_function_calls` computes `approval_tools = {name for name, tool in tool_map.items() if tool.approval_mode == "always_require"}` and short-circuits the entire batch (returning `function_approval_request` items for the model to see) before any function is invoked (`_tools.py:1691-1755`). This is the strongest gate.
- **Per-call hard ceiling**: `FunctionTool.__call__` increments `invocation_count` and refuses further calls once `max_invocations` is reached (`_tools.py:520-531`); parallel/consecutive-exception budgets also exist (`max_invocation_exceptions`).
- **Middleware-level IFC block**: `PolicyEnforcementFunctionMiddleware` reads the cumulative `context_label`, and on a `FunctionInvocationContext` whose integrity is `UNTRUSTED` and whose function is not in `allow_untrusted_tools` and does not advertise `accepts_untrusted`, either blocks (`_block_policy_violation` writes an error result and raises `MiddlewareTermination`, `security.py:1662-1679`) or prompts for approval (`_request_policy_violation_approval` writes a `function_approval_request` with `policy_violation: True` metadata, `security.py:1636-1660`). Either way, execution stops before `call_next`.

Thus the runtime can absolutely stop a high-risk tool even when the model asks for it.

**3. Can users approve selectively?**
Yes, at four granularities:

- **Whole session, single tool, no further prompts** (tool-wide standing rule): `create_always_approve_tool_response(...)` (`_harness/_tool_approval.py:220-247`).
- **Whole session, single tool + exact arguments** (argument-scoped standing rule): `create_always_approve_tool_with_arguments_response(...)` (`_harness/_tool_approval.py:236-247`). Argument keys are JSON-serialized for stable matching (`_harness/_tool_approval.py:46-58`); an empty-args rule is *not* widened to a wildcard (`_harness/_tool_approval.py:763-822`).
- **Hosted-tool server boundary** (`server_label`): same-named tools on different MCP servers do not share approvals (`_harness/_tool_approval.py:61-65, 310-323`, test `..._standing_rules_include_hosted_server_boundary`).
- **Heuristic auto-approve callback per call**: `ToolApprovalMiddleware(auto_approval_rules=[...])` lets a developer-supplied function decide per-call (`_harness/_tool_approval.py:354-369, 607-617`), and `PolicyEnforcementFunctionMiddleware(_is_policy_violation_approved, _mark_policy_violation_approved)` keeps a `_approved_violations: set[str]` of previously-approved `call_id`s so a user can replay a single one through the pipeline (`security.py:1608-1633, 1745-1752, 1799-1804`).

There is also a **MCP sampling gate** (`_mcp.py:1045-1073, 1150-1159`) that lets applications auto-approve or deny every server-initiated `sampling/createMessage`.

**4. Are approvals persisted?**
Yes. Standing rules are stored as serialized state in `AgentSession.state[source_id]` (`_harness/_tool_approval.py:250-277`), keyed by `DEFAULT_TOOL_APPROVAL_SOURCE_ID = "tool_approval"` (`_harness/_tool_approval.py:26`). The state structure is `{rules: [ToolApprovalRule], queued_approval_requests: [...], collected_approval_responses: [...]}` and survives `session.to_dict()` → `from_dict()`. Heuristic, per-call decisions (auto-approve rules, policy-violation approvals) are not separately persisted as rules; they are recomputed from the live policy callbacks at each invocation.

**5. Can policy block a model-requested tool?**
Yes, in two ways:

- The `always_require` bit on the tool *halts* the tool loop and surfaces a `function_approval_request` (`_tools.py:1721-1755`). A rejected `function_approval_response` is replaced with a `function_result` carrying `"Error: Tool call invocation was rejected by user."` and `msg.role = "tool"` (`_tools.py:2123-2129`), and the tool body never runs.
- `PolicyEnforcementFunctionMiddleware(block_on_violation=True)` cancels an in-flight `call_next()` via `MiddlewareTermination` and substitutes a `function_approval_request` (or, with `block_on_violation=True` *and* no `approval_on_violation`, a hard error result) before the function ever executes (`security.py:1662-1679, 1770-1781`). Audit-log entries are appended on every blocked attempt (`security.py:1584, 1904-1913`).

## Architectural Decisions

- **Per-tool permission flag, not a central policy store.** `approval_mode` is metadata on the function instance, not a separate policy object. This makes authorization travel with the tool object (and its serialized form) and avoids a global registry to maintain, but it means there is no place to ask "what does this agent expose that can touch the network?" — callers have to enumerate tools.
- **Default-deny stance on frictionless permissions.** `approval_mode` defaults to `never_require` (`_tools.py:398`), `default_integrity` for IFC is `UNTRUSTED` (`security.py:854`), and MCP sampling is deny-by-default with the explicit opt-in pattern `lambda params: True` (`_mcp.py:1100-1107`). These are deliberately opposite defaults: opt-in danger rather than opt-in safety for tools the developer already wired.
- **All-or-nothing visible approval batch.** When a model returns `n` parallel tool calls and *any one* requires approval, the loop returns `n` `function_approval_request` items together (`_tools.py:1700-1714`). The complementary bypass keeps already-approved items hidden behind the visible approval and only re-injects them when the user answers the visible request (`_tools.py:1974-2037`). This avoids leaking partial results.
- **Hosted-tool approvals pass through untouched.** Requests/responses that carry `server_label` on `function_call.additional_properties` are not processed locally; the comment at `_tools.py:1927-1937` notes the contract that the hosted server must handle them itself.
- **"Don't ask again" is scope-bounded.** Two scopes are explicit (`ALWAYS_APPROVE_TOOL`, `ALWAYS_APPROVE_TOOL_WITH_ARGUMENTS` at `_harness/_tool_approval.py:30-37`); arguments are JSON-serialized for stable comparison (`_harness/_tool_approval.py:46-58`), so re-running the tool with the same canonical arguments is silent.
- **Session-state is the persistence story.** Standing rules are written into `AgentSession.state` rather than to disk; hosting this in `AgentSession` makes approvals travel with the session, and the same `_save_state` / `_get_state` pattern is reused for budgets (`_harness/_tool_approval.py:250-277`).
- **Harness agent makes approval default-on.** `create_harness_agent` wires `ToolApprovalMiddleware(auto_approval_rules=auto_approval_rules)` as the outermost middleware unless `disable_tool_auto_approval=True` (`_harness/_agent.py:499-507`). This mirrors the documented .NET harness decision.
- **Policy-violation approvals carry rich metadata.** `additional_properties` on the approval request include `policy_violation: True`, `violation_type`, `reason`, and `context_label` (`security.py:1650-1660`). This lets a UI render a meaningful prompt ("approve even though we hit untrusted content earlier").
- **Defence-in-depth on tool arguments.** MCP applies both an allowlist (built from `inputSchema.properties`, `_mcp.py:1769-1803`) and a denylist of framework-named parameters (`_mcp.py:88-98`); explicit per-tool "extras" win over the denylist.

## Notable Patterns

- **Mode bit + middleware**: `FunctionTool.approval_mode` is the static flag; `ToolApprovalMiddleware` (and `PolicyEnforcementFunctionMiddleware`) is what runs at execution time. The flag has zero effect without the middleware, which is precisely how `ToolApprovalMiddleware` is selected via the outer harness wiring.
- **Content-shaped approval protocol**: approvals are first-class `Content` items (`function_approval_request`/`function_approval_response`, `_types.py:1212-1299`) that flow as ordinary messages. This keeps the approvals model uniform with how text/tool_calls flow, including handling in workflows (`_workflows/_agent.py:691-705`, `_workflows/_agent_executor.py:708`) where approvals become `request_info` events.
- **Tool-server boundary as a first-class identity**: `server_label` discrimination on hosted tools (`_harness/_tool_approval.py:61-65`) prevents cross-tenant approval leakage through name collisions.
- **Audit log shape**: per-violation dict `{type, function, context_label, turn, reason}` (`security.py:1738-1741, 1789-1794`); exposed via `PolicyEnforcementFunctionMiddleware.get_audit_log()` (`security.py:1915-1919`).
- **Max-tokens cap on approved sampling**: `_capped_sampling_max_tokens` (`_mcp.py:1075-1086`) clamps server-requested `maxTokens` to `sampling_max_tokens` even after the user approves — defense against a confused-deputy cost spike.
- **Function-invocation budget integration**: `ToolApprovalMiddleware` reads/writes `_FUNCTION_INVOCATION_BUDGET_STATE_KEY` on `client_kwargs` so auto-approved loops still consume the configured budget (`_harness/_tool_approval.py:27, 377, 484-488`).
- **Async/coroutine auto-rules**: `ToolApprovalRuleCallback` is typed `Callable[[Content], bool | Awaitable[bool]]` and the dispatcher handles sync/awaitable transparently (`_harness/_tool_approval.py:38, 607-617`).

## Tradeoffs

- **Two permission axes, three enforcement layers, no single source of truth.** `approval_mode` (tool-self), `kind` (tool-self, currently only `"shell"`), and IFC `additional_properties` (security middleware) live as separate fields. A change to one does not coordinate with the others, so consumers must remember which layer governs which behavior.
- **`kind` is an open string, no type safety.** `_tools.py:307` accepts `kind: str \| None = None`; there is no `Literal`/`Enum` constraining possible values. The only enforced constant is `SHELL_TOOL_KIND_VALUE` (`_tools.py:92`). New kinds can be added ad-hoc without runtime validation.
- **`source_integrity` / `accepts_untrusted` / `max_allowed_confidentiality` are untyped dict members.** `_get_additional_properties` casts to `dict`, and the key names live in code/tests (`security.py:837-846, 1732, 1886-1898`). A typo in a tool's `additional_properties` will silently disable policy enforcement rather than raise — caught only at policy-evaluation time when the property is missing.
- **`approval_mode` defaults to `never_require`.** This is convenient for local development but means the safety story depends on every developer who writes a destructive tool remembering to flip the flag. The framework mitigates this for `file_access_delete_file` (`_harness/_file_access.py:1147-1149`) and MCP sampling (deny-by-default, `_mcp.py:1045-1073`), but not in general.
- **All-or-nothing approval batch can be a UX papercut.** When the model requests five tools and one needs approval, the user sees five approval prompts or possibly one large combined prompt (`_tools.py:1700-1714`) — there is no per-tool skip-in-batch handling outside the ToolApprovalMiddleware "already-approved" hide (`_tools.py:1974-2037`).
- **Policy-enforcement tracking of approved violations is by `call_id`, in-memory.** The `_approved_violations: set[str]` (`security.py:1586`) is not persisted across process restarts; replays after restart will re-block. The standing `ToolApprovalRule` mechanism *does* persist because it lives in session state.
- **FIDES is experimental.** `LabelTrackingFunctionMiddleware`, `PolicyEnforcementFunctionMiddleware`, `IntegrityLabel`, `ConfidentialityLabel`, `ContentLabel`, `VariableReferenceContent`, `LabeledMessage`, `SecureAgentConfig`, and `quarantined_llm`/`inspect_variable` are all wrapped in `@experimental(feature_id=ExperimentalFeature.FIDES)` (`security.py:77, 94, 113, 309, 396, 479, 793, 1529, 1929, 2476`) — the policy enforcement *can* block, but its API surface is not promised stable.
- **`ToolApprovalMiddleware` is also experimental.** `@experimental(feature_id=ExperimentalFeature.HARNESS)` (`_harness/_tool_approval.py:86, 159, 345`). Production users should expect it to move.
- **`disable_tool_auto_approval` exists but the cheaper path is to add the middleware manually.** `_harness/_agent.py:499-507` only installs the middleware when `create_harness_agent` is used; users that instantiate `Agent` directly must wire middleware themselves or no approval flow runs.
- **No namespace-level ACL.** A user cannot say "this agent is allowed to call only `read_*` tools". The closest is `allow_untrusted_tools` (an IFC allowlist, `security.py:1562`), which has different semantics (which tools may run when context is UNTRUSTED).

## Failure Modes / Edge Cases

- **No middleware → no approval enforcement.** Constructing an `Agent(tools=[dangerous])` directly with `dangerous.approval_mode="always_require"` does not gate execution; only the function-invocation layer's `_try_execute_function_calls` runs and only if it sees `approval_tools` set does it short-circuit — actually, on re-reading `_tools.py:1691-1755`, *this short-circuit happens inside `use_function_invocation()` regardless of middleware presence.* So an `Agent` whose chat client uses `use_function_invocation` (the default in `BaseChatClient.__init__` via `FunctionInvocationLayer`) will still gate `always_require` tools automatically. What the *middleware* adds on top is the standing-rule memory and the auto-approve hooks. Edge case: a custom invocation flow that does not use `FunctionInvocationLayer` would skip the gate.
- **In-memory only for policy-violation approvals across restart** (`security.py:1586`). A user who approves an untrusted call today will be reprompted tomorrow.
- **Concurrent batch with one approval-pending + one approved, after an unrelated "Deny" answer**: `_replace_approval_contents_with_results` rebinds by `call_id` (`_tools.py:2072-2121`), so a misordered answer doesn't accidentally flip the wrong result — covered by test `test_hidden_mixed_batch_requests_replay_only_for_matching_visible_approval` (`test_harness_tool_approval.py:190-263`).
- **Hosted-tool approval cannot be processed locally** by design (`_tools.py:1927-1937`). If an MCP server denies or mishandles the request, the upstream framework has no fallback — this is documented in the comment but not gated.
- **`auto_hide_untrusted=True` (default) means untrusted tool results are stored as `_variable_reference` placeholders, not surfaced to the model** (`security.py:1380-1420, 1424-1425`). If a downstream model needs that data, it must use `inspect_variable` (which is `approval_mode="never_require"` and intentionally taints the context, `security.py:2495-2501`).
- **Mixed-batch hosted vs local**: the bypass in `_store_already_approved_approval_requests` stores *only the local* approvals hidden; the hosted ones stay visible (`_tools.py:1746-1749`). Tested with `_is_hosted_tool_approval(content)` (`_tools.py:1927-1937`).
- **`Accept-Untrusted` flag typo silently falls through.** `function_props.get("accepts_untrusted", False)` (`security.py:1732`) means a tool the developer intended to expose as untrusted-safe but wrote the property wrong will be blocked. The default is `False`.
- **Standing rules persist across runs**: a user who approved `delete_file` for `delete_file(file="…")` yesterday will have that permission applied today. There is no documented expiry; consumers must invalidate `session.state[source_id]` (or recreate the session) to revoke.
- **Schema-supplied tools without properties** get an empty allowlist for MCP forwarding (drop only configured extras), a deliberate safety net but worth noting for tool authors (`AGENTS.md` calls it out at `python/packages/core/AGENTS.md` MCP section).

## Future Considerations

- **A canonical `Risk` enum or classification registry** would unify `kind`, `approval_mode`, and the IFC `additional_properties` keys behind a typed object — eliminating the silent-typo failure of `source_integrity`/`accepts_untrusted`/`max_allowed_confidentiality` reads.
- **Persisting policy-violation approvals**: moving `_approved_violations` into `AgentSession.state` (mirroring the standing-rule pattern) would survive restarts.
- **Per-tool budgets carried with permission metadata**: today `max_invocations` is an instance setting; pairing it with a session-scoped budget would let "every agent run, max 3 calls of `dangerous_tool`" be expressed cleanly.
- **Default `approval_mode` should at least log a warning** for tools whose name/description suggests destructive operation (`delete_*`, `drop_*`, `commit_*`) — even without changing the default, a developer-time WARN nudges correctness.
- **Stable API for `PolicyEnforcementFunctionMiddleware` and `ToolApprovalMiddleware`**: both are experimental with significant surface — promote behind a `tool_approval_mode="mappings"` or similar config knob to give downstream users a compatibility promise.
- **Audit log destination**: `PolicyEnforcementFunctionMiddleware.audit_log` is in-memory (`security.py:1584`); an OTel/log-sink bridge would make denials observable in production.
- **Multi-tenant namespace**: a per-agent allowlist (`agent: {tools: [...]}`) would let users express "this agent cannot touch shell" without writing a custom middleware.

## Questions / Gaps

- **Is `_approved_violations` intended to be persisted later?** The set is per-middleware-instance; cross-restart behavior is undocumented (no evidence found beyond the per-class field at `security.py:1586`).
- **Is there a documented "revoke standing rule" API?** No evidence found of a helper that *removes* a `ToolApprovalRule` from `ToolApprovalState` other than mutating `state.rules` directly or recreating the session.
- **What happens if `function_approval_response.approved=True` references an unknown `id`?** `_collect_approval_responses` collects all responses (`_tools.py:2049-2056`) and `_replace_approval_contents_with_results` only replaces those that have a matching function_call (`_tools.py:2103-2128`). The behavior is "ignored" but not formally specified for a cross-correlated response.
- **Are MCP `approval_mode` TypedDict keys validated at construction time?** No evidence found — `_mcp.py:1301-1315` reads `self.approval_mode.get("always_require_approval")` and treats any `Collection[str]` as truthy; passing a non-string iterable or dict-of-dicts would behave unexpectedly.
- **How does the framework behave with `approval_mode` set on a tool that is *also* declaration-only (`func=None`)?** The gate fires (`_tools.py:1697-1717`) before checking declaration-only, so declaration-only tools with `always_require` still produce approval requests — which then become `function_approval_request` with `user_input_request=True` (tested at `_workflows/_agent.py:691-705`). Worth a dedicated test in `test_harness_tool_approval.py`.
- **What's the durability story for `ToolApprovalState.rules` across `AgentSession.from_dict`?** The state goes through `state.to_dict()/from_dict()` and rules are rehydrated (`_harness/_tool_approval.py:120-156, 187-217`), but cross-version compatibility is not pinned down in the file (no evidence of a schema version field on the state).

Generated by `_dimensions/04.05-tool-permissions-and-approval-metadata.md` against `agent-framework`.
