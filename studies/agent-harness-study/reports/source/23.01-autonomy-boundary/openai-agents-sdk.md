# Source Analysis: openai-agents-sdk

## Dimension 23.01: Autonomy Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio; OpenAI Responses/Chat Completions/Realtime APIs) |
| Analyzed | 2026-08-24 |

## Summary

The OpenAI Agents SDK implements autonomy boundaries as an **opt-in, per-tool approval system** layered with complementary tripwire guardrails and sandbox containment. The core mechanism is a `needs_approval` setting on every executable tool type — `FunctionTool` (`src/agents/tool.py:486-493`), `ShellTool` (`src/agents/tool.py:1368-1374`), `ApplyPatchTool` (`src/agents/tool.py:1423-1429`), `CustomTool` (`src/agents/tool.py:1463-1464`), agent-as-tool (`src/agents/agent.py:600-601`), and MCP servers via `require_approval` (`src/agents/mcp/server.py:548,560-564`). The setting accepts either a static bool or an async callable that decides per call using the run context, parsed tool arguments, and call ID.

When a gated tool is invoked without a stored decision, the runner pauses instead of executing: it collects `ToolApprovalItem`s into `NextStepInterruption` (`src/agents/run_internal/run_steps.py:171-175`, `src/agents/run_internal/run_steps.py:127`), surfaces them on `RunResult.interruptions` / `RunResultStreaming.interruptions` (`src/agents/result.py:515-516`, `src/agents/result.py:650-651`), and lets the host application decide via `RunState.approve()` / `reject()` (`src/agents/run_state.py:1255-1298`) before resuming with `Runner.run(agent, state)`. Decisions can be one-shot (per call ID) or sticky (`always_approve` / `always_reject` scoped to tool identity, `src/agents/run_context.py:56-68`), and the whole paused state serializes durably for long-running approvals (`src/agents/run_state.py:1300-1324`, documented at `docs/human_in_the_loop.md:187-199`).

The default posture is **autonomous**: every approval knob defaults to `False`/`None`, so nothing is gated unless the developer opts in. The SDK compensates with fail-closed evaluation (unparseable arguments or failing policy callables force approval rather than skip it: `src/agents/run_internal/tool_execution.py:1306-1310`, `src/agents/run_internal/turn_resolution.py:1267-1270`, `src/agents/mcp/server.py:823-831`), input/output guardrails that halt execution on tripwires (`src/agents/guardrail.py:29-32`), per-tool guardrails with allow/reject/halt behaviors (`src/agents/tool_guardrails.py:69-77`), and sandbox workspace scoping for shell/filesystem capabilities (`src/agents/sandbox/capabilities/shell.py:22,47-53`). The model is configurable to a fine grain, extensively tested (100+ dedicated approval tests), uniformly documented (`docs/human_in_the_loop.md`), and applied consistently across Runner, streaming, nested agent-as-tool runs, hosted MCP, and Realtime sessions.

**Rating: 8/10**

## Rating

**Score: 8/10** — "Clear model with tests, explicit interfaces, and operational safeguards," close to mature.

Rationale:
- **Clear model**: a single concept (`needs_approval` → interruption → approve/reject → resume) covers function tools, shell, apply_patch, custom tools, agent-as-tool, local MCP, hosted MCP, and Realtime (`docs/human_in_the_loop.md:43` enumerates all surfaces).
- **Explicit interfaces**: typed settings (`bool | Callable[..., Awaitable[bool]]` at `src/agents/tool.py:710-712`), a public decision API (`RunState.approve/reject`, `src/agents/run_state.py:1255-1298`), and a status query API (`get_approval_status`, `src/agents/run_context.py:1065-1077`).
- **Operational safeguards**: fail-closed argument parsing (`src/agents/run_internal/tool_execution.py:1306-1310`), fail-closed exception handling during approval planning (`src/agents/run_internal/turn_resolution.py:1269-1270` converts unexpected errors into "requires approval"), invalid-setting rejection (`src/agents/util/_approvals.py:46-50`), and durable serialized decisions.
- **Tests**: `tests/test_tool_approval_call_id_reuse.py` (83 test functions, 3231 lines), `tests/test_run_context_approvals.py` (25 tests, 742 lines), plus scenario tests like `tests/test_hitl_session_scenario.py:89,144`.
- **Why not 9–10**: there is no run-level or global autonomy mode (e.g., "require approval for everything"), no autonomy tiers beyond binary per-call gating, sticky decisions are scoped to one resumable run rather than persisted across sessions by default, and the approval-identity resolution machinery carries visible legacy-compatibility complexity (`src/agents/run_context.py:509-568`, schema-version branching at `src/agents/run_internal/turn_resolution.py:1279-1293`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Approval flag on FunctionTool | `needs_approval: bool \| Callable[[RunContextWrapper, dict, str], Awaitable[bool]] = False`; docstring explains interrupt + `RunState.approve()/reject()` resume | `src/agents/tool.py:486-493` |
| Positional-contract protection around the approval field | Comment keeps guardrail fields before `needs_approval` to preserve v0.7.0 constructor compatibility | `src/agents/tool.py:477-478` |
| ShellTool approval gate | `needs_approval: bool \| ShellApprovalFunction = False` with `(run_context, action, call_id)` callable form | `src/agents/tool.py:1368-1374` |
| Hosted shell cannot be self-gated | `__post_init__` raises `UserError` if a hosted-environment ShellTool sets `needs_approval` or `on_approval`, then forces both off | `src/agents/tool.py:1403-1410` |
| ApplyPatchTool approval gate | `needs_approval: bool \| ApplyPatchApprovalFunction = False` taking `(run_context, operation, call_id)` | `src/agents/tool.py:1423-1429` |
| CustomTool approval gate | `needs_approval` + `on_approval` fields; `runtime_needs_approval()` accessor used by runtime | `src/agents/tool.py:1463-1466`, `src/agents/tool.py:1497-1499` |
| Agent-as-tool approval gate | `Agent.as_tool(..., needs_approval=...)` parameter and docstring "Bool or callable to decide if this agent tool should pause for approval" | `src/agents/agent.py:600-601`, `src/agents/agent.py:630` |
| MCP server approval policy | `require_approval` accepts `"always"`/`"never"`, bool, per-tool dict, `{always:{tool_names}, never:{tool_names}}` object, or callable `(run_context, agent, tool)` | `src/agents/mcp/server.py:560-564` |
| MCP policy normalization + validation | `_normalize_needs_approval` rejects unknown keys/values and always∩never overlap with `UserError` | `src/agents/mcp/server.py:709-794` |
| MCP fail-closed default | Callable policy without an available agent returns `True` ("historical fail-closed behavior") | `src/agents/mcp/server.py:820-831` |
| Hosted MCP programmatic decision | `HostedMCPTool.on_approval_request: MCPToolApprovalFunction \| None` | `src/agents/tool.py:1087-1097` |
| Hosted MCP escalation fallback | Without an `on_approval_request` hook, approvals are "surfaced as interruptions for the caller to handle" (debug log) | `src/agents/run_internal/turn_resolution.py:3119-3124` |
| Interruption data type | `ToolApprovalItem` carries raw item, tool name, namespace, lookup key, origin | `src/agents/items.py:555-594` |
| Pause step in run loop | `ProcessedResponse.interruptions` ("Tool approval items awaiting user decision") and `has_interruptions()`; `NextStepInterruption` stops execution | `src/agents/run_internal/run_steps.py:127`, `src/agents/run_internal/run_steps.py:150-152`, `src/agents/run_internal/run_steps.py:171-175` |
| Public result surface | `RunResult.interruptions` / `RunResultStreaming.interruptions` and `to_state()` docstring describing approve→resume | `src/agents/result.py:515-516`, `src/agents/result.py:650-651`, `src/agents/result.py:541-559` |
| Decision application | `RunState.approve(approval_item, always_approve)` / `reject(..., always_reject, rejection_message)`; nested agent-tool approvals are routed to their owning nested state | `src/agents/run_state.py:1255-1298` |
| Approval record shape | `_ApprovalRecord.approved/.rejected` as bool (permanent) or list of call IDs (per-call); sticky scope/message fields | `src/agents/run_context.py:56-68` |
| Sticky-decision storage & query | `approve_tool` / `reject_tool` write records; `get_approval_status` returns True/False/None and requires non-empty call IDs | `src/agents/run_context.py:1043-1063`, `src/agents/run_context.py:1065-1077` |
| Nested-run routing | `ToolContext.approve_tool/reject_tool` route surfaced nested approvals to the nested context owner; conflicting identities raise `UserError` | `src/agents/tool_context.py:194-228`, `src/agents/tool_context.py:178-192` |
| Durable decisions | `_serialize_approvals` persists approved/rejected sets, rejection messages, sticky scopes into `RunState` snapshots | `src/agents/run_state.py:1300-1324` |
| Fail-closed argument parsing | `parse_function_tool_arguments` returns `None` for empty/non-object/non-standard JSON; `function_needs_approval` then returns `True` (approval required) | `src/agents/util/_approvals.py:18-29`, `src/agents/run_internal/tool_execution.py:1306-1311` |
| Fail-closed policy exceptions | `_function_requires_approval` re-raises `UserError` but maps any other exception from a policy callable to `True` | `src/agents/run_internal/turn_resolution.py:1256-1270` |
| Invalid setting rejected eagerly | `evaluate_needs_approval_setting` raises `UserError` for non-bool/non-callable values | `src/agents/util/_approvals.py:32-51` |
| Programmatic auto-decision path | Shell execution checks stored status, evaluates `needs_approval`, then consults `on_approval` before deciding to pause (`resolve_approval_status` returns item when awaiting human) | `src/agents/run_internal/tool_actions.py:480-511` |
| Input/output guardrails | Tripwire halts execution (`InputGuardrailTripwireTriggered`); input guards may run parallel or blocking via `run_in_parallel` | `src/agents/guardrail.py:71-103` (incl. `run_in_parallel` at `:100-103`), `src/agents/guardrail.py:133-143` |
| Guardrail execution semantics | Concurrent tasks cancelled on first tripwire; error attached to trace span; exception raised to halt run | `src/agents/run_internal/guardrails.py:115-168`, `src/agents/run_internal/guardrails.py:171-224` |
| Tool-level guardrails | `allow` / `reject_content(message)` / `raise_exception` behaviors for per-tool input/output checks | `src/agents/tool_guardrails.py:40-77`, `src/agents/tool_guardrails.py:91-117` |
| Dynamic tool enablement boundary | `is_enabled` bool-or-callable hides tools from the LLM based on runtime context | `src/agents/tool.py:472-475` |
| Sandbox containment | Shell capability tools bound to a session with `SandboxWorkspaceScope` workspace limits | `src/agents/sandbox/capabilities/shell.py:22`, `src/agents/sandbox/capabilities/shell.py:47-53` |
| Rejection message configurability | Run-wide `RunConfig.tool_error_formatter` plus per-call `rejection_message` override | `src/agents/run_config.py:448`, `src/agents/run_state.py:1270-1282` |
| Realtime parity | Realtime sessions evaluate the same `needs_approval` (fail-closed on unparseable args, `strict=False`), build `ToolApprovalItem`s, expose `approve_tool_call`/`reject_tool_call` | `src/agents/realtime/session.py:651-668`, `src/agents/realtime/session.py:698-760`, `src/agents/realtime/session.py:969,1015` |
| HITL documentation | Dedicated guide: marking tools, flow steps, sticky decisions, hosted-MCP identity rules, partial resolution, durability, versioning | `docs/human_in_the_loop.md:3-15`, `docs/human_in_the_loop.md:45-57`, `docs/human_in_the_loop.md:89-97`, `docs/human_in_the_loop.md:187-203` |
| Guardrail documentation | Workflow boundaries (input guards only on first agent; output only on final agent) and parallel-vs-blocking tradeoff documented | `docs/guardrails.md:10-14`, `docs/guardrails.md:32-38` |
| Test coverage | 83 tests in call-ID reuse suite; 25 in run-context approvals; internal helper tests; end-to-end HITL session scenarios | `tests/test_tool_approval_call_id_reuse.py:1`, `tests/test_run_context_approvals.py:1`, `tests/test_run_internal_approvals.py:34-113`, `tests/test_hitl_session_scenario.py:89,144` |
| Examples | `examples/agent_patterns/human_in_the_loop.py`, `human_in_the_loop_stream.py`, `human_in_the_loop_custom_rejection.py`, `examples/tools/shell_human_in_the_loop.py` | `examples/agent_patterns/`, `examples/tools/` |

## Answers to Dimension Questions

### 1. What determines agent autonomy?

Autonomy is determined entirely by **per-tool declarations plus stored decisions**, evaluated at execution time:

1. **Declaration layer**: each tool type carries its own gate — `FunctionTool.needs_approval` (`src/agents/tool.py:486`), `ShellTool.needs_approval` (`src/agents/tool.py:1368`), `ApplyPatchTool.needs_approval` (`src/agents/tool.py:1423`), `CustomTool.needs_approval` (`src/agents/tool.py:1463`), `Agent.as_tool(needs_approval=...)` (`src/agents/agent.py:600`), MCP `require_approval` (`src/agents/mcp/server.py:548`), and hosted MCP `require_approval` config with optional `on_approval_request` (`src/agents/tool.py:1087-1097`).
2. **Decision layer**: before executing, the runner queries `context_wrapper.get_approval_status(...)` (`src/agents/run_internal/tool_actions.py:480-484`); only if no stored decision exists does it evaluate the declaration. A `True`/pending result produces a `ToolApprovalItem` interruption instead of execution (`src/agents/run_internal/tool_actions.py:500-511`).
3. **Complementary hard boundaries**: guardrails can halt unconditionally regardless of approval state (`src/agents/guardrail.py:81-84`), tool guardrails can reject content mid-run (`src/agents/tool_guardrails.py:40-44`), and sandbox workspace scoping constrains what shell/file tools can touch at all (`src/agents/sandbox/capabilities/shell.py:22,47-53`).

There is no evidence of a global autonomy level, role-based permission model, or automatic risk classification of tool arguments (e.g., detecting destructive commands). Gating granularity is per-tool/per-call.

### 2. Are autonomy levels configurable?

Yes, at fine grain, but binary per decision point:

- Every gate accepts a static bool or an async predicate receiving `(run_context, parsed_args, call_id)` — e.g., `src/agents/tool.py:486-493` — enabling arbitrary per-call policies (the docs show content-based routing such as approving only refunds, `docs/human_in_the_loop.md:27-33`).
- MCP servers additionally accept string constants, per-tool dicts, an always/never tool-list object with overlap validation, or callables (`src/agents/mcp/server.py:560-564`, normalization at `:709-813`).
- Decision stickiness is configurable per decision: one-shot (default) vs `always_approve=True` / `always_reject=True`, persisted in serialized state (`src/agents/run_context.py:56-68`, `docs/human_in_the_loop.md:53`).
- Escalation behavior is tunable: programmatic auto-decision callbacks (`on_approval` on shell/apply_patch/custom tools, `src/agents/tool.py:1375-1378,1430-1433`; `on_approval_request` for hosted MCP, `src/agents/tool.py:1097`) versus manual pauses, plus customizable rejection text via `RunConfig.tool_error_formatter` and per-call `rejection_message` (`src/agents/run_config.py:448`, `src/agents/run_state.py:1270-1282`).
- Notably asymmetric: hosted shell environments *cannot* opt into local approval flows at all — misconfiguration raises `UserError` and forces `needs_approval=False` (`src/agents/tool.py:1403-1410`).

What is *not* configurable: a run-wide "gate everything" mode, numeric autonomy tiers, or delegation of the approval decision to another agent.

### 3. Are boundaries documented?

Yes, thoroughly and accurately relative to the implementation:

- `docs/human_in_the_loop.md` documents the full lifecycle: declaring gates (`:11-43`), the five-step pause/approve/resume flow including handoff and nested-agent cases (`:45-57`), sticky-decision persistence across serialization (`:53`), hosted-MCP identity scoping rules (`:55`), partial resolution of mixed interruptions (`:57`), programmatic alternatives (`:89-97`), streaming integration (`:99-103`), durable long-running approvals (`:187-199`), and versioning advice (`:201-203`). The documented fail-closed rule for malformed arguments (`:15`) matches `src/agents/run_internal/tool_execution.py:1306-1310`.
- `docs/guardrails.md:10-14` documents the workflow-boundary subtlety that input guardrails run only for the first agent and output guardrails only for the final agent, and `:32-38` documents the parallel-vs-blocking execution tradeoff (parallel tripwires may fire after side effects began) — matching `run_in_parallel` at `src/agents/guardrail.py:100-103`.
- `SECURITY.md` is only a vulnerability-disclosure pointer and does not discuss autonomy posture — no evidence found there.

### 4. Does the system respect autonomy boundaries?

Yes, with multiple enforced mechanisms:

- **Pause-not-execute**: pending approvals become `NextStepInterruption`, which ends the turn without running the tool (`src/agents/run_internal/run_steps.py:171-175`); the tool executes only after `Runner.run(agent, state)` resumes with a recorded decision (`src/agents/result.py:541-559`).
- **Identity-scoped enforcement**: decisions bind to canonical tool-invocation identities (type, call ID, approval scope, fingerprint — `src/agents/_tool_invocation.py:164-236` referenced via `src/agents/run_context.py:288-403`), so an approval for one call ID cannot silently authorize a different invocation; ambiguous identities raise `UserError` (`src/agents/run_state.py:1241-1252`, `src/agents/tool_context.py:178-192`).
- **Fail-closed defaults**: unparseable arguments (`src/agents/run_internal/tool_execution.py:1308-1310`), crashing policy callables (`src/agents/run_internal/turn_resolution.py:1269-1270`), and MCP callable policies lacking an agent (`src/agents/mcp/server.py:830-831`) all degrade to "requires approval."
- **Nested-run containment**: approvals raised inside `Agent.as_tool()` nested runs surface on the outer run and are routed back to the correct owning state on decision (`src/agents/tool_context.py:194-228`), so a delegated sub-agent cannot execute gated work invisibly.
- **Cross-surface consistency**: the same evaluation logic is shared by Runner, streaming, and Realtime through `evaluate_needs_approval_setting` (`src/agents/util/_approvals.py:10-11` states this design intent; imports at `src/agents/realtime/session.py:45` and `src/agents/run_internal/tool_actions.py:42`).

Residual boundary risks: parallel input guardrails may trigger only after the agent already started consuming tokens/executing tools (documented at `docs/guardrails.md:36`), sticky `always_approve` grants persist for the rest of the run with no expiry or revocation hook, and there is no runtime audit trail surface specific to who approved what beyond trace spans and serialized state.

## Architectural Decisions

1. **Opt-in gating with fail-closed evaluation.** The SDK chose developer-declared gates (`needs_approval=False` defaults) over a deny-by-default posture, but hardened the evaluator so ambiguity escalates to human review rather than bypassing it (`src/agents/run_internal/tool_execution.py:1306-1311`, `src/agents/run_internal/turn_resolution.py:1269-1270`). This keeps the common case zero-config while making failure paths conservative.
2. **Interruptions as first-class run items, not callbacks-only.** Pauses materialize as `ToolApprovalItem`s in the result and a resumable `NextStepInterruption` step (`src/agents/items.py:555-594`, `src/agents/run_internal/run_steps.py:171-175`), which makes HITL compatible with durable serialization (`RunState.to_json/from_json`, `src/agents/run_state.py:1732-1754` region) and multi-process workflows.
3. **One approval abstraction across nine surfaces.** Function, shell, apply_patch, custom, computer, agent-as-tool, local MCP, hosted MCP, and Realtime all normalize to the same `needs_approval`/decision-record machinery (`docs/human_in_the_loop.md:43`; shared evaluator at `src/agents/util/_approvals.py:32-51`), minimizing divergent semantics.
4. **Sticky decisions keyed by tool identity, not call ID alone.** `_ApprovalRecord` stores booleans for permanent decisions and call-ID lists for per-call ones, with separate hosted-MCP identity keys (`server_label`, tool name) preventing cross-server confusion (`src/agents/run_context.py:56-68`, `src/agents/run_context.py:446-452`, documented at `docs/human_in_the_loop.md:55`).
5. **Nested approvals tunnel to the outer run.** Rather than requiring the caller to know about nested agents, interruptions propagate outward and decisions route inward via identity matching (`src/agents/tool_context.py:125-228`), with explicit `UserError`s on ambiguous ownership (`src/agents/run_state.py:1237-1252`).
6. **Compatibility discipline around the approval contract.** Field ordering around `needs_approval` is frozen to preserve positional construction (`src/agents/tool.py:477-478,495`), and legacy serialized approvals get reconstruction paths gated by schema version (`src/agents/run_internal/turn_resolution.py:1279-1293`), signaling that approval state is treated as a durable external contract.

## Notable Patterns

- **Bool-or-callable duality**: every autonomy knob uses the same `bool | Callable` union resolved through `evaluate_needs_approval_setting` (`src/agents/util/_approvals.py:32-51`), supporting static policy, dynamic context-dependent policy, and sync/async uniformly.
- **Double-check after evaluation**: shell execution re-queries stored approval status even after evaluating `needs_approval`, guarding against decisions recorded concurrently during evaluation (`src/agents/run_internal/tool_actions.py:485-496` mirrors `src/agents/realtime/session.py:720-731`).
- **Rejection-as-tool-output**: rejections are fed back into the conversation as synthetic tool outputs so the model can adapt, with configurable message text and tracing (`src/agents/run_internal/approvals.py:24-43`, `src/agents/run_internal/turn_resolution.py:1345-1403`).
- **Guardrails as orthogonal tripwires**: where approvals ask "may this run?", guardrails assert "must this stop?" — input guards can block before the expensive model starts (`run_in_parallel=False`, `src/agents/guardrail.py:100-103`), and tool guardrails offer a middle behavior (`reject_content`) that neither approves nor halts but redirects the model (`src/agents/tool_guardrails.py:40-44`).
- **Sandbox as structural autonomy limit**: capability-based sandbox toolsets bound shell execution to a workspace scope up front, removing entire action classes from the autonomy question (`src/agents/sandbox/capabilities/shell.py:22-53`).

## Tradeoffs

- **Zero-config default vs safety-first default**: `needs_approval=False` everywhere maximizes ergonomics but means an undeclared dangerous tool runs autonomously; the SDK pushes responsibility to tool authors (`src/agents/tool.py:486-493`) and mitigates only via fail-closed parsing edges.
- **Latency vs certainty in guardrails**: parallel input guardrails save latency but permit partial side effects before cancellation; blocking mode avoids side effects at latency cost — both modes shipped and documented rather than one chosen (`docs/guardrails.md:32-38`).
- **Programmatic `on_approval` vs human oversight**: auto-decision callbacks let runs continue unattended (`src/agents/tool.py:1375-1378`) at the cost of removing the human from the loop; the docs explicitly position them as an alternative (`docs/human_in_the_loop.md:9,89-97`).
- **Sticky approvals vs blast radius**: `always_approve=True` removes repeated friction but grants an unrevocable-for-the-run standing authorization bounded only by tool identity (`src/agents/run_context.py:56-68`); there is no TTL, quota, or argument-fingerprint binding on sticky grants.
- **Durability vs secret hygiene**: serializing approvals enables cross-process HITL, but the serialized payload embeds app context; users are warned to keep secrets out (`docs/human_in_the_loop.md:199`) rather than the SDK enforcing redaction.

## Failure Modes / Edge Cases

- **Unparseable tool arguments force manual approval** — malformed JSON, non-object JSON, or `NaN`/`Infinity` constants cause the policy callable to be skipped and approval required, identically in Runner and Realtime paths (`src/agents/util/_approvals.py:18-29`, `src/agents/run_internal/tool_execution.py:1306-1311`, `src/agents/realtime/session.py:657-661`).
- **Policy callable crashes are treated as "needs approval"** except for `UserError`, which propagates (`src/agents/run_internal/turn_resolution.py:1261-1270`) — a crash therefore blocks progress rather than granting silent autonomy.
- **Invalid configuration fails fast**: wrong-typed `needs_approval` values raise `UserError` at evaluation time (`src/agents/util/_approvals.py:46-50`); MCP policies with overlapping always/never names or bad shapes raise during normalization (`src/agents/mcp/server.py:772-788`).
- **Ambiguous approval identity aborts the decision**: duplicate call IDs across current/nested runs raise `UserError` instead of guessing (`src/agents/run_state.py:1241-1252`, `src/agents/tool_context.py:178-192`).
- **Hosted MCP approvals without identity fields are not persisted as sticky**, so they will re-prompt later rather than over-authorize (`docs/human_in_the_loop.md:55`).
- **Legacy-state drift is contained**: pre-1.7 schemas accept same-name agent matches on resume because duplicates could not be distinguished historically, while newer schemas require object identity (`src/agents/run_internal/turn_resolution.py:1279-1293`).
- **Parallel guardrail race**: with `run_in_parallel=True`, the agent may have executed tools before the tripwire lands; only blocking mode guarantees no execution (`docs/guardrails.md:36-38`, implementation `src/agents/run_internal/guardrails.py:66-97` cancels sibling tasks but cannot undo prior side effects).
- **Partial resolution is supported**: rerunning after resolving a subset of mixed interruptions continues resolved calls while unresolved ones re-pause (`docs/human_in_the_loop.md:57`; plumbing in `src/agents/run_internal/approvals.py:46-56`).

## Future Considerations

- A run-level or agent-level autonomy mode (e.g., `require_approval="all"|"none"|callable-set`) would give operators a global dial without annotating every tool; today only per-MCP-server list policies approximate this (`src/agents/mcp/server.py:772-794`).
- Sticky-decision governance: expiry, max-uses, or revocation APIs on `always_approve` records would reduce standing-grant risk (`_ApprovalRecord` currently has no such fields, `src/agents/run_context.py:56-68`).
- Argument-risk classification hooks (e.g., destructive-shell detection) could automate `needs_approval` predicates the docs currently leave fully to user code (`docs/human_in_the_loop.md:13-15`).
- An approval audit log distinct from traces would strengthen accountability for "who approved what, when" across serialized resumes.

## Questions / Gaps

- **No global autonomy tier system found.** Searched `src/agents` for permission levels/tiers beyond binary approval (`grep -iE "permission|autonomy_level|trust_level"` over tool/config modules); only per-tool flags, `is_enabled`, `allowed_callers` (`src/agents/tool.py:518-519`, enforced for hosted calls at `src/agents/run_internal/turn_resolution.py:3106-3111`), and sandbox scoping exist. No clear evidence found of richer RBAC.
- **No persistent cross-run approval memory found.** Sticky decisions live inside one `RunState` snapshot (`src/agents/run_state.py:1300-1324`; `docs/human_in_the_loop.md:53` scopes them to "resuming the same paused run"); no built-in store applies past approvals to fresh runs.
- **SECURITY.md contains no autonomy guidance** — only coordinated-disclosure pointers (`SECURITY.md:1-5`); trust-boundary rationale must be inferred from code and `AGENTS.md` contributor rules.
- **Approval UI/observability beyond tracing not found**: guardrail spans record tripwires (`src/agents/tracing/span_data.py` guardrail span usage at `src/agents/run_internal/guardrails.py:37-40`), but no dedicated approval-history event stream was located; observers see `ToolApprovalItem`s only via results/state.

---

Generated by dimension 23.01 (`Autonomy Boundary`) against `openai-agents-sdk`.
