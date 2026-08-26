# Source Analysis: openai-agents-sdk

## Dimension 14.01: Human-in-the-Loop Trigger Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (OpenAI Agents SDK, asyncio, Pydantic) |
| Analyzed | 2026-08-26 |

## Summary

The SDK implements HITL as a **tool-approval interruption model**: tools declare whether they need human approval via a configurable `needs_approval` rule (bool or async callable), and the runner pauses the entire run — surfacing `ToolApprovalItem` entries in `RunResult.interruptions` — whenever an unapproved, approval-required tool call is planned. Humans then decide through `RunState.approve()/reject()` (or `RealtimeSession.approve_tool_call()/reject_tool_call()` for realtime sessions), and the serialized run state resumes execution.

Triggers are exclusively **tool-risk based**; there are no uncertainty-, budget-, or validation-failure-driven triggers for human review. Guardrail failures reject content inline rather than pausing for a human, and turn limits raise `MaxTurnsExceeded` errors instead of requesting review. Trigger rules are highly configurable per tool type (function, shell, apply_patch, custom, agent-as-tool, local MCP, hosted MCP), support programmatic auto-decision callbacks (`on_approval`) as an alternative to pausing, combine across multiple concurrent tool calls in one turn, and persist decisions durably inside `RunState` serialization. The evaluation path is fail-closed: if a trigger-checker callable raises unexpectedly, the SDK treats approval as required (`src/agents/run_internal/tool_planning.py:806-812`).

## Rating

**8 / 10** — A clear, well-tested, explicit trigger model with strong operational safeguards (fail-closed evaluation, per-call-ID scoping, sticky-decision identity scoping, durable serialization, streaming/realtime/nested-run coverage). It falls short of 9–10 because trigger *decisions* are not independently auditable (no dedicated audit log or tracing span recording who approved what, when — decisions live only in mutable run context/state and stream events), and there is no built-in trigger for budget exhaustion or model-expressed uncertainty.

## Evidence Collected

Every entry cites file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core trigger field (function tools) | `needs_approval: bool \| Callable[[RunContextWrapper, dict, str], Awaitable[bool]]`; doc states the run will be interrupted and requires `RunState.approve()`/`reject()` | `src/agents/tool.py:486-493` |
| Trigger on shell tool | `ShellTool.needs_approval: bool \| ShellApprovalFunction = False` | `src/agents/tool.py:1368-1374` |
| Trigger on apply_patch tool | `ApplyPatchTool.needs_approval: bool \| ApplyPatchApprovalFunction = False` | `src/agents/tool.py:1423-1429` |
| Trigger on custom tool | `CustomTool.needs_approval` plus `runtime_needs_approval()` accessor | `src/agents/tool.py:1463-1499` |
| Agent-as-tool trigger | `Agent.as_tool(..., needs_approval=...)` parameter; doc: "Bool or callable to decide if this agent tool should pause for approval" | `src/agents/agent.py:600-633`, wiring at `src/agents/agent.py:1028` |
| Local MCP server trigger policy | `require_approval` constructor arg accepts `"always"`/`"never"`, per-tool dict, `{always, never: {tool_names}}` object, bool, or callable `(run_context, agent, tool)` | `src/agents/mcp/server.py:548, 560-564, 575-577` |
| MCP policy normalization & validation | `_normalize_needs_approval` validates shapes, rejects overlapping always/never names with `UserError` | `src/agents/mcp/server.py:710-813` |
| Per-MCP-tool resolution (fail-closed legacy) | `_get_needs_approval_for_tool` returns `True` when a callable policy exists but no agent context is available ("historical fail-closed behavior") | `src/agents/mcp/server.py:815-846` |
| Hosted MCP trigger | Model-emitted `McpApprovalRequest` becomes `MCPApprovalRequestItem`; without an `on_approval_request` hook, approvals "will be surfaced as interruptions" | `src/agents/run_internal/turn_resolution.py:3096-3124` |
| Trigger evaluation engine | `evaluate_needs_approval_setting` resolves bool or sync/async callable, raising `UserError` on invalid types | `src/agents/util/_approvals.py:32-51` |
| Approval planning pipeline | `_collect_runs_by_approval`: checks stored decision → evaluates checker → False=reject item, True=execute, None=pending interruption | `src/agents/run_internal/tool_planning.py:759-840` |
| Fail-closed on checker failure | Exception in `needs_approval_checker` (other than `UserError`) sets `needs_approval = True` | `src/agents/run_internal/tool_planning.py:806-812` |
| Resume-path re-evaluation | `_select_function_tool_runs_for_resume` repeats the same decision flow when resuming from serialized state | `src/agents/run_internal/tool_planning.py:883-940` |
| Interruption step emission | Pending interruptions produce `NextStepInterruption` and stop the loop | `src/agents/run_internal/turn_resolution.py:885-911`; dataclass at `src/agents/run_internal/run_steps.py:127, 150-152` |
| Programmatic auto-decision callback | `on_approval` handlers invoked immediately when approval needed; mapping `{"approve": True/False, "reason": ...}` applied via `approve_tool`/`reject_tool` | `src/agents/tool.py:1375-1378`; `src/agents/run_internal/tool_execution.py:1172-1213` |
| User decision API (standard runs) | `RunState.approve(item, always_approve)` / `RunState.reject(item, always_reject, rejection_message)` incl. routing into nested agent-tool states | `src/agents/run_state.py:1255-1298` |
| Decision storage | `RunContextWrapper.approve_tool` / `reject_tool` write `_ApprovalRecord` (approved/rejected bool-or-call-id-lists, rejection messages, sticky scope) | `src/agents/run_context.py:1043-1063, 57-68` |
| Sticky decision semantics | Per-call IDs vs permanent decisions; exact-call decisions override sticky defaults; approval wins conflicting stickies | `src/agents/run_context.py:628-661` |
| Realtime decision API | `RealtimeSession.approve_tool_call(call_id, always)` / `reject_tool_call(..., rejection_message)` resume or notify the model | `src/agents/realtime/session.py:969-1049` |
| Surfacing to humans | `RunResult.interruptions: list[ToolApprovalItem]` and `to_state()` documented for approve→resume | `src/agents/result.py:515-516, 541-563` |
| What humans see | `ToolApprovalItem` carries raw tool call, resolved `tool_name`, namespace, origin metadata | `src/agents/items.py:555-600` |
| Streaming surface | `RunResultStreaming.interruptions` set when the loop stops; `mcp_approval_requested`/`mcp_approval_response` run-item events | `src/agents/run_internal/run_loop.py:347-350`; `src/agents/stream_events.py:39-40` |
| Durable persistence of decisions | `_serialize_approvals` writes approved/rejected lists and rejection messages into `RunState.to_string()/to_json()` | `src/agents/run_state.py:1300-1319` |
| Custom rejection messaging | `DEFAULT_APPROVAL_REJECTION_MESSAGE = "Tool execution was not approved."`; per-call override and run-wide `RunConfig.tool_error_formatter` | `src/agents/tool.py:194`; `src/agents/run_state.py:1270-1282`; `docs/human_in_the_loop.md:59-87` |
| Invocation-identity safety | Reused call IDs for different invocations raise `ModelBehaviorError` | `src/agents/run_context.py:344-352` |
| Hosted MCP sticky scoping tests | e.g. `test_hosted_mcp_permanent_approval_is_scoped_by_server_label`, `test_namespaced_approval_status_does_not_fall_back_to_bare_tool_decisions` | `tests/test_run_context_approvals.py:30, 531` |
| End-to-end usage example | CLI prompt loop over `result.interruptions` with `state.approve/interrupt` and `always_approve` | `examples/tools/shell_human_in_the_loop.py:105-140` |
| Documented model | HITL guide describing trigger rule evaluation order and surfaces | `docs/human_in_the_loop.md:1-60` |

## Answers to Dimension Questions

### 1. What triggers human review?

Only **approval-required tool calls** trigger it. The runner evaluates each planned tool call against its configured rule during tool planning (`src/agents/run_internal/tool_planning.py:798-838`) and again on resume (`src/agents/run_internal/tool_planning.py:905-939`). If no prior decision exists for the call ID and the rule returns true, the call becomes a pending `ToolApprovalItem` interruption and the run stops before any side effect (`src/agents/run_internal/turn_resolution.py:899-911`). Applicable surfaces: function tools (`src/agents/tool.py:486-493`), shell (`src/agents/tool.py:1368`), apply_patch (`src/agents/tool.py:1423`), custom tools (`src/agents/tool.py:1463`), agent-as-tool (`src/agents/agent.py:600-601`), local MCP servers (`src/agents/mcp/server.py:548, 560-564`), and hosted MCP requests (`src/agents/run_internal/turn_resolution.py:3096-3124`). Nested `Agent.as_tool()` interruptions propagate to the outer run so a single human surface covers handoffs and sub-agents (`src/agents/agent.py:752-851`; `src/agents/tool_context.py:125-191`). **Not implemented as triggers:** model uncertainty, token/budget exhaustion (budgets exist only for schema validation and truncation, e.g. `src/agents/strict_schema.py:126`), guardrail violations (rejected inline, not paused), or max-turns (raises an error).

### 2. Are triggers configurable?

Yes, extensively. Each trigger can be a static bool or an async predicate receiving `(run_context, parsed_arguments, call_id)` resolved by `evaluate_needs_approval_setting` (`src/agents/util/_approvals.py:32-51`). MCP adds richer policy shapes: string literals, per-tool mappings, always/never tool-name lists validated with `UserError` on conflicts, and a `(run_context, agent, tool)` callable (`src/agents/mcp/server.py:560-564, 710-813`). Applications can also invert the pause entirely via `on_approval` callbacks that decide programmatically without interrupting (`src/agents/tool.py:1375-1378`; execution at `src/agents/run_internal/tool_execution.py:1191-1213`). There is no global/default trigger policy knob — configuration is strictly per tool/server/agent-tool, which keeps defaults safe (`needs_approval=False`) but means org-wide risk policies must be composed by the application.

### 3. Can users request human review?

Users cannot inject an ad-hoc "pause here" request mid-run; there is no `request_review()` API. However, users fully control the review surface declaratively (set `needs_approval=True` on any risky tool) and hold complete decision authority once interrupted: `state.approve()/reject()` with optional `always_approve`/`always_reject` sticky semantics and custom `rejection_message` (`src/agents/run_state.py:1255-1298`), mirrored in realtime sessions (`src/agents/realtime/session.py:969-1049`). Partial resolution is supported — unresolved items simply re-interrupt on resume (`docs/human_in_the_loop.md:57`).

### 4. Are trigger decisions auditable?

Partially. Decisions are recorded in structured `_ApprovalRecord`s including per-call rejection messages (`src/agents/run_context.py:57-68`), survive serialization via `_serialize_approvals` (`src/agents/run_state.py:1300-1319`), and MCP approval requests/responses emit observable stream events (`src/agents/stream_events.py:39-40`). But there is **no dedicated audit log**: tracing contains no approval-specific spans (search for "approval" across `src/agents/tracing/` returns nothing), decisions carry no actor/timestamp metadata, and records are mutated in-place within the run context. Who approved what must be reconstructed by the host application.

## Architectural Decisions

1. **Interrupt-and-resume over blocking prompts.** The runner never blocks awaiting input; it returns control with `interruptions` and resumes from a serialized `RunState` (`src/agents/result.py:541-563`). This makes HITL work across process boundaries, queues, and long human latencies.
2. **Trigger evaluation lives in tool planning, centralized and uniform.** All tool types funnel through the same status-check → rule-evaluation → classify flow (`_collect_runs_by_approval`, `src/agents/run_internal/tool_planning.py:759-840`; shared helper `src/agents/util/_approvals.py:32-51`), so one semantic applies everywhere.
3. **Identity-scoped approval state.** Approvals key on canonical invocation identity (type, call_id, approval_scope, fingerprint) with strict mismatch detection raising `ModelBehaviorError` (`src/agents/run_context.py:333-355`), preventing cross-call or cross-server authorization confusion (tested in `tests/test_run_context_approvals.py:30-291`).
4. **Fail-closed defaults.** Checker exceptions force approval-required (`src/agents/run_internal/tool_planning.py:806-812`); callable MCP policy without agent context forces `True` (`src/agents/mcp/server.py:823-831`); malformed persisted approvals cannot alias to bare tool names (`tests/test_run_context_approvals.py:245-291`).
5. **Two-layer automation escape hatch.** `on_approval` callbacks allow code-level decisions while preserving the same record-keeping path (`src/agents/run_internal/tool_execution.py:1191-1213`), separating "who decides" (human vs code) from "how it's recorded".

## Notable Patterns

- **Sticky vs per-call decisions:** `always_approve=True` stores a boolean on the record scoped by approval scope; otherwise the call ID is appended to allow-lists, with exact-call decisions overriding sticky ones (`src/agents/run_context.py:644-661, 998-1037`).
- **Nested-run propagation:** inner `Agent.as_tool()` interruptions bubble to the outer `RunState`, which routes decisions back down (`src/agents/run_state.py:1259-1298`).
- **Rejection feedback to the model:** rejected calls return model-visible text assembled from per-call message, sticky message, or run-wide formatter (`resolve_approval_rejection_message`, `src/agents/run_internal/tool_execution.py:1230-1244`).
- **Hosted MCP dual-key lookup:** decisions indexed both by request ID and by `(server_label, tool_name)` query keys, keeping name-based lookups authoritative without granting extra authority (`src/agents/run_context.py:965-988`).
- **Reference examples as executable docs:** manual prompting (`examples/tools/shell_human_in_the_loop.py:127-138`), streaming (`examples/agent_patterns/human_in_the_loop_stream.py`), custom rejection text (`examples/agent_patterns/human_in_the_loop_custom_rejection.py`).

## Tradeoffs

- **Tool-risk-only triggering** keeps the model simple but misses other natural review moments (high spend, repeated failures, uncertain answers); applications must build those themselves.
- **Per-tool configuration granularity** avoids central policy files but pushes compliance-style policy enforcement onto every tool definition.
- **In-process mutable approval records** simplify the happy path yet sacrifice independent auditability; durability only exists where the app persists `RunState`.
- **Fail-closed exception handling** trades availability for safety: a buggy predicate turns every call into a pause (`src/agents/run_internal/tool_planning.py:806-812`).
- **Positional-compat constraints** shape the public API (fields ordered around `needs_approval` to preserve v0.7.0 constructors, `src/agents/tool.py:477-478, 495`), slightly obscuring which knobs are security-relevant.

## Failure Modes / Edge Cases

- **Call-ID reuse attacks/mistakes** are detected and aborted with `ModelBehaviorError` rather than silently inheriting an approval (`src/agents/run_context.py:344-352, 398-401`).
- **Malformed serialized approvals** (non-bool/list values) are sanitized defensively on restore (`src/agents/run_context.py:704-710`; `tests/test_run_context_approvals.py:673`).
- **Namespaced tools do not fall back** to bare-name decisions, preventing privilege bleed between namespaces (`tests/test_run_context_approvals.py:531, 721`).
- **Hosted shell environments forbid** `needs_approval`/`on_approval` outright, failing fast at construction (`src/agents/tool.py:1403-1410`).
- **Unresolvable approvals stay pending** rather than executing: missing bindings mark calls unbound and re-pause (`src/agents/run_context.py:570-610`).
- **Gap:** no evidence found of rate-limiting or anomaly handling for repeated approve/reject churn; the loop re-prompts indefinitely if the application resubmits unresolved items.

## Future Considerations

- Add a dedicated approval-decision audit trail (actor, timestamp, decision, item snapshot) and/or a tracing span so decisions are observable outside application memory.
- Expose a run-level or global trigger policy (e.g., "always approve-gate tools tagged destructive") to reduce per-tool boilerplate.
- Consider additional built-in trigger classes — spend/token thresholds, consecutive-failure counters, model self-assessed confidence — since none exist today.
- Surface interruption telemetry as first-class events analogous to `mcp_approval_requested` for function/shell/custom approvals (currently only MCP has named stream events, `src/agents/stream_events.py:39-40`).

## Questions / Gaps

- **Uncertainty/budget/policy-violation triggers:** No evidence found. Searched `src/agents/` for `budget`, `uncertain`, `policy violation` tied to review; budgets only govern schema nodes, tracing payload size, and output truncation (e.g., `src/agents/strict_schema.py:126`, `src/agents/sandbox/util/token_truncation.py:24-104`).
- **Explicit user-initiated review request:** No evidence found of a public API to demand review of an already-approved or non-gated call mid-run; searched `request.*review|human_request` patterns across `src/`.
- **Decision audit logging:** No evidence found of structured audit logs beyond `RunState` serialization and stream events; `grep approval src/agents/tracing/` returned zero matches.
- **Multi-condition trigger composition:** Rules do not natively compose (e.g., OR of two predicates); applications compose by wrapping callables. Evidence boundary: `evaluate_needs_approval_setting` accepts exactly one bool/callable (`src/agents/util/_approvals.py:32-51`).

---

Generated by `Dimension 14.01: Human-in-the-Loop Trigger Policy` against `openai-agents-sdk`.
