# Source Analysis: openai-agents-sdk

## Dimension 09.03: Governance UX and Operator Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (Agents SDK library; asyncio; Pydantic; httpx) |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK is a library, not an application, so "governance UX" exists as a **programming surface for host applications to build operator workflows on**, not as dashboards or review consoles shipped in the repo. The core mechanism is a pause/resume human-in-the-loop (HITL) kernel: tools declare `needs_approval`, the run loop pauses and returns pending approvals as a list of `ToolApprovalItem` interruptions (`src/agents/result.py:515-516`), and the host application resolves them via `RunState.approve(...)` / `RunState.reject(...)` before resuming (`src/agents/run_state.py:1255-1298`). Decisions are recorded per tool/call-ID in `_ApprovalRecord` structures inside the run context (`src/agents/run_context.py:57-68`) and survive serialization via versioned `RunState.to_json()` / `from_string()` round-trips (`src/agents/run_state.py:182-222`).

Measured against this dimension's specific lens — approval dashboards, visible decision history, exception surfacing, evidence packs, and bulk actions — the picture splits sharply:

- **Strong**: what needs review is fully enumerable (`result.interruptions`, `state.get_interruptions()` at `src/agents/run_state.py:985-1017`); rejection feedback to both model and operator is configurable at two layers (run-wide formatter plus per-call message, `src/agents/run_config.py:83-105`); ambiguity in approvals fails loudly with actionable errors rather than guessing (`src/agents/run_state.py:1087-1092`); and approval policy evaluation fails closed when arguments are uninspectable (`src/agents/util/_approvals.py:18-29`).
- **Absent**: no dashboard, no persistent review queue, no evidence-pack generation, no bulk approve/reject API, and no public API to audit past approved/rejected decisions. The only shipped operator-facing UI is a demo realtime web app whose "approval dialog" is a browser `window.confirm` (`examples/realtime/app/static/app.js:566`) and CLI prompt loops in examples.

The result is an unusually well-engineered approval *kernel* (identity-matched decisions, nested-run routing, durability, ~3,800 lines of error-scenario tests) wrapped in an intentionally minimal operator experience that every adopting team must build themselves.

**Rating: 5/10**

Rationale: The dimension asks whether governance is usable by humans — dashboards, queues, exception visibility, evidence packs, bulk operations. The SDK's approval machinery itself would score 8+ (explicit interfaces, fail-closed defaults, extensive tests), but the operator-facing surfaces this dimension measures are mostly absent or left to integrators: the review queue is an implicit in-memory list, there is no UI beyond demo code, evidence packs do not exist, bulk action means "write your own loop," and historical decisions are stored in private `_approvals` dicts with no query API. This lands squarely in "present but inconsistent" territory: the primitives are production-grade, but out of the box a human operator cannot see or act on governance state without developer-written tooling.

## Evidence Collected

Every entry includes a file path with line numbers relative to `studies/agent-harness-study/sources/openai-agents-sdk`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Pending-review surface ("queue") | `RunResult.interruptions: list[ToolApprovalItem]` is the sole pending-review listing; documented as "Pending tool approval requests (interruptions) for this run." | `src/agents/result.py:515-516` |
| Re-fetching pending items from paused state | `RunState.get_interruptions()` returns detached copies of pending approvals from the current interruption step | `src/agents/run_state.py:985-1017` |
| Approval item data available to operators | `ToolApprovalItem` exposes `tool_name`, `qualified_name`, `arguments`, `call_id`, `agent`; raw item union covers function/shell/MCP/local-shell calls | `src/agents/items.py:556-676` |
| Decision recording | `_ApprovalRecord` stores `approved`/`rejected` as bool-or-call-ID-lists plus per-call and sticky rejection messages | `src/agents/run_context.py:56-68` |
| Public approve/reject handlers | `RunState.approve(item, always_approve)` and `RunState.reject(item, always_reject, rejection_message)` route nested agent-tool approvals to the owning sub-state | `src/agents/run_state.py:1255-1298` |
| Decision application internals | `RunContextWrapper._apply_approval_decision` validates canonical call IDs, hosted-MCP identity requirements, and writes sticky vs per-call entries | `src/agents/run_context.py:888-1041` |
| Run-loop approval partitioning | `_collect_runs_by_approval` classifies each tool run into executed / rejected-item / pending-interruption using `get_approval_status` | `src/agents/run_internal/tool_planning.py:759-840` |
| Fail-closed policy evaluation | Unparsable or non-object JSON arguments return `None` so callable `needs_approval` rules cannot run; call requires manual approval | `src/agents/util/_approvals.py:18-29` |
| Exception surfaced back to model/operator | `append_approval_error_output` emits a synthetic `ToolCallOutputItem` "so users see why an approval failed" | `src/agents/run_internal/approvals.py:24-43` |
| Ambiguity errors instead of silent guesses | Duplicate invocation identities across current/nested runs raise `UserError` telling the operator to use unique call IDs | `src/agents/run_state.py:1087-1092, 1241-1252` |
| Configurable rejection text (run-wide) | `ToolErrorFormatterArgs` with `kind="approval_rejected"` and `RunConfig.tool_error_formatter` | `src/agents/run_config.py:83-105, 448` |
| Configurable rejection text (per-call) | `rejection_message` parameter on `reject_tool` / `RunState.reject` takes precedence over the run-wide formatter | `src/agents/run_context.py:1051-1059`; `docs/human_in_the_loop.md:59-66` |
| Durable state for long-running approvals | Versioned serialization: `CURRENT_SCHEMA_VERSION = "1.17"`, `_HOSTED_MCP_APPROVALS_MIN_SCHEMA_VERSION = "1.14"` | `src/agents/run_state.py:179-222` |
| Approvals persisted in serialized state | `_serialize_approvals` writes approved/rejected call-ID lists and rejection messages into the state payload | `src/agents/run_state.py:1300-1320` |
| Realtime operator path | `RealtimeSession.approve_tool_call(call_id, always=...)` / `reject_tool_call(...)` resume execution by call ID | `src/agents/realtime/session.py:969-1049` |
| Demo approval UI (browser) | Realtime example server forwards `tool_approval_decision` WebSocket messages; frontend renders `window.confirm` approve/reject dialogs | `examples/realtime/app/server.py:562-579`; `examples/realtime/app/static/app.js:536-579` |
| Streaming event visibility | `mcp_approval_requested` / `mcp_approval_response` run-item stream events; realtime `tool_approval_required` event | `src/agents/stream_events.py:39-40`; `src/agents/realtime/events.py:122` |
| Reference operator loop (CLI) | Example prints Agent/Tool/Arguments per interruption, prompts y/N, calls `state.approve/reject`, persists state to disk between passes | `examples/agent_patterns/human_in_the_loop.py:114-133` |
| Sticky-decision prompting pattern | Shell HITL example prompts "Approve all future shell calls?" and uses `always_approve=True` | `examples/tools/shell_human_in_the_loop.py:75-88, 127-134` |
| Partial-resolution workflow | Docs specify unresolved approvals remain pending while resolved ones continue across resumes | `docs/human_in_the_loop.md:57` |
| Test coverage of failure UX | ~50 scenario tests including fail-closed arguments, changed-tool rejection on resume, duplicate identity errors | `tests/test_hitl_error_scenarios.py:240-2800` (file is 3,800 lines) |

Searches performed for dashboard/review-queue/evidence-pack/bulk features returned no implementation matches: `grep -rni "dashboard|review queue|evidence pack|bulk|audit_log"` over `src/` only hits unrelated uses (bulk DB inserts in `src/agents/extensions/memory/sqlalchemy_session.py:408`, trace-dashboard metadata fields in `src/agents/tracing/traces.py:132-146`). No approval-specific tracing spans exist either (`grep -rn "approval" src/agents/tracing/` → 0 matches).

## Answers to Dimension Questions

**1. Can operators see what needs review?**
Yes, programmatically; no, not visually. The paused run exposes everything an operator needs per item — agent name, qualified tool name, full argument string, and call ID (`src/agents/items.py:556-676`, consumed at `examples/agent_patterns/human_in_the_loop.py:115-119`). `RunState.get_interruptions()` (`src/agents/run_state.py:985-1017`) allows re-querying after deserialization, which supports building an external review queue. However, the SDK ships no dashboard, no persistent queue store, and no push notification: discovery is pull-only via the returned result object, so an approval can sit unnoticed unless the host application polls or builds its own inbox. MCP approval requests additionally emit stream events (`src/agents/stream_events.py:39-40`) usable to drive UIs.

**2. Can they act on approvals efficiently?**
Moderately. The canonical flow is a `for interruption in result.interruptions:` loop calling `state.approve(...)`/`state.reject(...)` (`docs/human_in_the_loop.md:154-163`). There is no batch/bulk API — approving N items requires N calls — though the loop pattern is first-class and partially resolving a mixed set is explicitly supported (`docs/human_in_the_loop.md:57`). Efficiency gains come from sticky decisions: `always_approve=True`/`always_reject=True` persist per tool identity for the rest of the run (`src/agents/run_state.py:1255-1268`, `src/agents/run_context.py:998-1037`), and hosted-MCP stickiness is scoped by `(server_label, tool_name)` so identical tool names on different servers stay distinct (`docs/human_in_the_loop.md:55`, enforced at `src/agents/run_context.py:903-907`). Durability (`to_json`/`from_string`, schema-versioned at `src/agents/run_state.py:179-222`) means decisions can be made asynchronously hours later in another process — the real efficiency lever for long-running approvals.

**3. Are exceptions surfaced?**
Yes, on multiple channels. (a) When an approval decision cannot be applied, a synthetic tool output is appended so the reason is visible in the item stream (`src/agents/run_internal/approvals.py:24-43`). (b) Operators control what the model sees on rejection via run-wide `tool_error_formatter` (`src/agents/run_config.py:83-105`) or per-call `rejection_message` (`src/agents/run_context.py:1051-1059`), with precedence rules documented (`docs/human_in_the_loop.md:66`). (c) Unsafe conditions raise rather than guess: duplicate invocation identities across current/nested runs raise `UserError` with remediation guidance ("Use unique call IDs", `src/agents/run_state.py:1087-1092`, `1153-1157`, `1241-1252`); missing hosted-MCP request IDs block persistent decisions (`src/agents/run_context.py:897-907`); empty call IDs block per-call decisions (`src/agents/run_context.py:912-928`). (d) Policy evaluation fails closed: malformed/non-object arguments skip the callable rule and force manual review (`src/agents/util/_approvals.py:18-29`, tested at `tests/test_hitl_error_scenarios.py:932-1003`).

**4. Is the governance UI usable under pressure?**
There is essentially no shipped UI, so under-pressure usability depends entirely on the host app. The reference implementations are minimal: a blocking stdin prompt in the CLI example (`examples/agent_patterns/human_in_the_loop.py:121`) and a `window.confirm()` modal in the realtime demo (`examples/realtime/app/static/app.js:566-574`). What the SDK does provide for high-pressure operation are safeguards against operator error: identity-checked matching so a decision applies to exactly one invocation (`src/agents/run_state.py:1020-1055`), explicit errors when an ambiguous match would risk approving the wrong call, nested agent-as-tool routing so operators always act on the single outer `RunState` regardless of where the approval originated (`docs/human_in_the_loop.md:5-7`, `src/agents/run_state.py:1095-1253`), and regression tests proving sibling agents with the same tool name do not inherit each other's approvals (`tests/test_hitl_error_scenarios.py:1372`). There is also no RBAC or attribution: anyone holding the serialized state can approve, and decisions carry no user/timestamp metadata.

## Architectural Decisions

1. **Pause-and-resume over inline prompting.** The run loop never blocks on I/O for a human; it terminates the turn into a `NextStepInterruption` carrying `ToolApprovalItem`s (`src/agents/run_state.py:985-1017`, populated at `src/agents/result.py:158-160`). This makes HITL compatible with serverless/web hosts and lets approvals outlive processes, at the cost of pushing all UX burden onto integrators.

2. **Decisions live in run context, keyed by tool + call-ID scope.** `_ApprovalRecord.approved/rejected` is either a bool (sticky) or a list of call IDs (per-invocation) (`src/agents/run_context.py:60-65`). Canonical identity comes from shared helpers in `src/agents/_tool_identity.py` (referenced via `tool_invocation_identity*` calls at `src/agents/run_context.py:929-946`), giving one source of truth for matching across function, shell, apply_patch, custom, local-shell, and MCP tools (`ToolApprovalRawItem` union, `src/agents/items.py:543-552`).

3. **Nested approvals collapse to one outer surface.** Interruptions raised inside handoff targets or `Agent.as_tool()` executions are re-raised on the outer run's state, and `approve`/`reject` recursively locate the owning sub-state (`src/agents/run_state.py:1259-1263`, `1095-1253`). Operators face exactly one approval surface per top-level run.

4. **Fail-closed and fail-loud policy semantics.** Callable `needs_approval` rules are skipped (forcing manual approval) when arguments cannot be safely parsed (`src/agents/util/_approvals.py:32-51` combined with the fail-closed contract in `docs/human_in_the_loop.md:15`), and invalid setting types raise `UserError` immediately (`src/agents/util/_approvals.py:46-50`, tested at `tests/test_hitl_error_scenarios.py:905`).

5. **Versioned durable checkpoints.** Serialized approvals ride along in `RunState` payloads guarded by `SCHEMA_VERSION_SUMMARIES` with per-feature minimum versions (hosted-MCP approvals ≥ 1.14, `src/agents/run_state.py:179-222`), enabling cross-process, cross-time review workflows.

## Notable Patterns

- **Interruptions as a first-class terminal step type**, parallel to final output: `RunResult` and `RunResultStreaming` both carry `interruptions` lists (`src/agents/result.py:515-516`, `649-651`), and streaming docs instruct draining the stream before inspecting them (`docs/streaming.md:40-67`).
- **Synthetic output items for observability**: approval failures become normal-looking `ToolCallOutputItem`s in the generated-items stream (`src/agents/run_internal/approvals.py:34-43`), so downstream consumers see a uniform item timeline.
- **Identity-scoped stickiness**: `always_approve` binds to tool identity (including namespace-aware lookup keys, `src/agents/items.py:601-614`) rather than raw name strings, preventing same-name collisions between sibling agents (tested at `tests/test_hitl_error_scenarios.py:1372-1436`).
- **Layered rejection messaging**: SDK default → run-wide formatter callback receiving structured `ToolErrorFormatterArgs` (`kind`, `tool_type`, `tool_name`, `call_id`, default message, run context, `src/agents/run_config.py:83-102`) → per-call override (`src/agents/run_context.py:1051-1059`).
- **Example-driven operator patterns**: dedicated examples for streaming approvals (`examples/agent_patterns/human_in_the_loop_stream.py`), custom rejection text (`examples/agent_patterns/human_in_the_loop_custom_rejection.py`), session-persisted HITL (`examples/memory/memory_session_hitl_example.py`, `examples/memory/openai_session_hitl_example.py`), indexed in `docs/human_in_the_loop.md:176-185`.

## Tradeoffs

- **Library posture vs operator readiness**: keeping governance headless keeps the SDK embeddable anywhere, but it means every adopting team must independently solve queueing, notification, multi-user review, and audit — none of these problems have even reference-quality solutions here.
- **Pull-based discovery vs timely review**: approvals are only visible after awaiting the run result; there is no webhook/event emitter for "approval needed" in the non-realtime path, so latency-to-first-human-touch is unbounded.
- **Sticky decisions vs blast radius**: `always_approve=True` reduces friction but grants run-long authority to a tool identity; the mitigation (scope by namespace/server identity, `src/agents/run_context.py:975-988`) is careful but there is no expiry, no revocation API, and no count limit on sticky approvals.
- **Strictness vs operability**: refusing ambiguous approvals (`UserError` on duplicate identities, `src/agents/run_state.py:1087-1092`) protects correctness but can hard-block an operator mid-incident until developers fix call-ID generation upstream.
- **Rich internal record, thin public read API**: `_approvals` retains full decision state (`src/agents/run_context.py:89`) and serializes it (`src/agents/run_state.py:1300-1320`), but it is underscore-private; auditing past decisions requires parsing serialized state blobs rather than calling a query method.

## Failure Modes / Edge Cases

Covered explicitly by tests in `tests/test_hitl_error_scenarios.py` (3,800 lines):

- **Uninspectable arguments** (malformed JSON, non-object, NaN/Infinity constants): callable approval rules are bypassed and the call is forced to manual approval (`src/agents/util/_approvals.py:22-29`; test at `tests/test_hitl_error_scenarios.py:932-1003`).
- **Missing/empty call IDs**: recognized invocations without a non-empty call ID raise `ModelBehaviorError` at decision time (`src/agents/run_context.py:912-928`); shell calls without call IDs raise on resume (`tests/test_hitl_error_scenarios.py:781`).
- **Duplicate invocation identities** between current run and nested agent-tool runs: raises `UserError` demanding unique call IDs (`src/agents/run_state.py:1087-1092`, `1241-1252`).
- **Stale approvals across resume**: a decision recorded for a different tool/handoff than the one now holding the call ID is rejected rather than honored (`tests/test_hitl_error_scenarios.py:1663`, `2008`).
- **Un-copyable approval payloads**: if raw items cannot be deep-copied for detached snapshots, `get_interruptions()` raises a redacted `UserError` explaining the supported-shape requirement (`src/agents/run_state.py:992-1016`).
- **Hosted MCP legacy identity changes**: unknown legacy identities require re-approval instead of silently honoring old stickiness (`tests/test_hitl_error_scenarios.py:763`).
- **Partially resolved batches**: rerunning after deciding some items leaves the rest pending and re-interrupts (`docs/human_in_the_loop.md:57`; tested at `tests/test_hitl_error_scenarios.py:536`).

Not covered anywhere: concurrent operators deciding the same interruption from two deserialized copies of the same state (last-writer-wins semantics are undefined in docs), and revoking a sticky decision once made.

## Future Considerations

- A public read API over `_approvals` (e.g., `list_decisions()`) would let hosts render approved/rejected/pending views without touching private fields (`src/agents/run_context.py:89`).
- An optional approval-event hook or emitted stream event for "approval required" in the standard (non-realtime) runner would close the notification gap; today only realtime surfaces `tool_approval_required` (`src/agents/realtime/events.py:122`).
- Bulk helpers such as `state.approve_all(decisions: Mapping[call_id, bool])` would formalize the loop pattern and enable transactional all-or-nothing resolution.
- Decision metadata (actor, timestamp, rationale) captured alongside `approve`/`reject` would make serialized states usable as lightweight audit artifacts, the nearest practical substitute for evidence packs.
- Sticky-decision TTLs or explicit revocation would reduce the risk of run-long `always_approve` grants.

## Questions / Gaps

- No evidence found for any dashboard, persistent review queue, evidence-pack generator, or bulk-approval endpoint in `src/` or `examples/`; searches for `dashboard`, `review queue`, `evidence pack`, `bulk`, and `audit_log` matched only unrelated code (bulk session inserts, trace-metadata fields). The dimension's dashboard/evidence-pack questions are therefore answered "absent by design."
- No RBAC/authentication layer on approvals was found: possession of the `RunState` (or serialized state string) is the only authorization boundary. Nothing in `docs/human_in_the_loop.md` addresses who may approve.
- Whether approvals generate tracing spans could not be confirmed positively; searches in `src/agents/tracing/` found no approval-specific spans, suggesting approval events appear only indirectly via tool-call spans and generated items.
- Long-term storage of decisions beyond a single run's lifetime is undocumented; `_serialize_approvals` persists within a `RunState` blob (`src/agents/run_state.py:1300-1320`), but there is no cross-run decision ledger.

---

Generated by `09.03-governance-ux-and-operator-workflow` against `openai-agents-sdk`.
