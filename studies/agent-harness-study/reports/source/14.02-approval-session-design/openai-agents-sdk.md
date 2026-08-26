# Source Analysis: openai-agents-sdk

## Dimension 14.02 — Approval Session Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (OpenAI Agents SDK, `src/agents/`, async-first, Pydantic-based serialization) |
| Analyzed | 2026-08-26 |

## Summary

Approval sessions in the OpenAI Agents SDK are implemented as a first-class human-in-the-loop (HITL) subsystem. A tool declares an approval rule (`needs_approval` on `FunctionTool` at `src/agents/tool.py:486-493`, and analogous fields on `ShellTool`/`ApplyPatchTool`/`CustomTool` at `src/agents/tool.py:1368`, `src/agents/tool.py:1423`, `src/agents/tool.py:1463`). When a model emits a gated tool call, the runner checks for an already-recorded decision in `RunContextWrapper._approvals`; if none exists, it builds a `ToolApprovalItem` (`src/agents/items.py:556-583`), pauses the run via `NextStepInterruption` (`src/agents/run_internal/run_steps.py:171-181`), and surfaces the pending items as `RunResult.interruptions` (`src/agents/result.py:515-516`). The application approves or rejects on a `RunState`, which is the durable pause/resume boundary (`src/agents/run_state.py:748-762`), optionally serializes it to JSON for storage in a database or queue, and resumes the run later.

Decisions have two scopes — per-call (bound to a specific provider call ID) and sticky/permanent (`always_approve=True`, bound to a fingerprinted "approval scope" identity) — recorded by `_apply_approval_decision` in `src/agents/run_context.py:888-1041`. Sticky decisions survive serialization because both the decision records and the canonical tool-invocation ledger are persisted with a versioned schema (`$schemaVersion` 1.17, `src/agents/run_state.py:182-217`). Two notable gaps exist: pending approvals never time out (no TTL or deadline mechanism exists anywhere in the codebase), and approvals are not independently audited (no approval-specific audit log or tracing span; only sandbox operations emit audit events).

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band):** The request flow (`resolve_approval_status`, `src/agents/run_internal/tool_execution.py:1162-1213`), decision API (`RunState.approve`/`reject`, `src/agents/run_state.py:1255-1298`; `RunContextWrapper.approve_tool`/`reject_tool`, `src/agents/run_context.py:1043-1063`), and persistence boundary (`RunState.to_json`/`to_string`, `src/agents/run_state.py:1704-2064`) are all explicit, typed, and documented (`docs/human_in_the_loop.md`).
- **Operational safeguards:** Versioned schema with fail-fast forward compatibility (`src/agents/run_state.py:175-218`), unbound per-call decisions forced to re-approve after restore (`_mark_restored_unbound_approval_call_ids`, `src/agents/run_context.py:1308-1316`), fail-closed callable approval rules on malformed arguments (`src/agents/run_internal/tool_execution.py:1306-1310`, tested in `tests/test_hitl_error_scenarios.py:932`), and extensive redaction hardening in `RunState.from_string` (`src/agents/run_state.py:2099-2171`).
- **Proven under failure:** Thousands of lines of dedicated tests covering resume, nested agent-tool approvals, call-ID reuse, schema downgrades, and hostile serialized payloads (`tests/test_run_state.py`, `tests/test_hitl_error_scenarios.py`, `tests/test_tool_approval_call_id_reuse.py`, `tests/test_run_state_compatibility_corpus.py`).
- **Why not 9–10:** No timeout/expiry semantics for pending approvals and no audit trail of who approved what and when; observability of decisions is limited to what the app chooses to log.

## Evidence Collected

Every entry includes a path relative to the source root with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Approval rule declaration | `needs_approval: bool \| Callable[[RunContextWrapper, dict, str], Awaitable[bool]]` on `FunctionTool` | src/agents/tool.py:486-493 |
| Approval rule on shell/apply-patch/custom tools | `ShellTool.needs_approval` + `on_approval`; same pattern on `ApplyPatchTool` and `CustomTool` | src/agents/tool.py:1368-1375, 1423-1430, 1463-1465 |
| Agent-as-tool gating | `Agent.as_tool(..., needs_approval=...)` parameter with docstring "pause for approval" | src/agents/agent.py:600-630 |
| Request-time status check | `get_approval_status(tool_name, call_id, existing_pending=..., current_invocation=...)` consulted before execution | src/agents/run_context.py:1065-1235 |
| Pending item construction + `on_approval` hook | `resolve_approval_status` builds `ToolApprovalItem`, runs optional programmatic callback returning `{"approve": bool, "reason": str}` | src/agents/run_internal/tool_execution.py:1162-1213 |
| Run pause point | `NextStepInterruption(interruptions=[...])` returned when unresolved approvals remain; streamed run finalizes with `interruptions=approvals_from_step(...)` | src/agents/run_internal/turn_resolution.py:1810-1829; src/agents/run_internal/run_loop.py:1371-1388 |
| Public interruption surface | `RunResult.interruptions: list[ToolApprovalItem]` and streaming equivalent | src/agents/result.py:515-516, 651 |
| Decision entry points | `state.approve(item, always_approve=False)` / `state.reject(item, always_reject=False, rejection_message=...)` delegate to context wrapper | src/agents/run_state.py:1255-1298 |
| In-memory record shape | `_ApprovalRecord{approved: bool\|list[str], rejected: bool\|list[str], rejection_messages, sticky_rejection_message, sticky_scope}` keyed by tool name or hosted-MCP tuple keys | src/agents/run_context.py:56-68, 89 |
| Scope definitions (call ID vs sticky) | Per-call decisions appended to call-ID lists; sticky sets `approved/rejected = bool` plus `sticky_scope` fingerprint | src/agents/run_context.py:998-1037 |
| Sticky scope binding | `_matching_sticky_approval_keys` matches records whose `sticky_scope` equals the invocation's scope fingerprint | src/agents/run_context.py:407-472 |
| Invocation/scope fingerprints | `tool_invocation_identity_and_scope` returns `(type, call_id, approval_scope, fingerprint)` hashing semantic payload; `tool_invocation_approval_scope` hashes type + lookup key / server_label + tool name | src/agents/_tool_invocation.py:155-279 |
| Hosted MCP sticky identity | `("hosted_mcp", server_label, tool_name)` persistent identity requires both non-empty fields; exact-call fallback `("hosted_mcp_call", request_id)` | src/agents/_tool_identity.py:21-43, 78-113 |
| Session persistence container | `RunState` dataclass: "the durable pause/resume boundary for human-in-the-loop flows … including model responses, generated items, approval state" | src/agents/run_state.py:748-762 |
| Approvals serialization | `_serialize_approvals` (string-keyed records incl. `rejection_messages`, `sticky_scope`) and `_serialize_hosted_mcp_approvals` (typed identity records); emitted into `context_entry["approvals"]` / `"hosted_mcp_approvals"` | src/agents/run_state.py:1300-1324, 1341-1384, 1742-1754 |
| Tool-invocation ledger serialization | Call-ID → `{type, approval_scope, fingerprint, executed, completed}` persisted so restored decisions can be bound to invocations | src/agents/run_state.py:1326-1339 |
| Restore path | `RunState.from_json/from_string` → `_build_run_state_from_json` → `context._rebuild_approvals(...)`, `_rebuild_hosted_mcp_approvals(...)`, `_rebuild_tool_invocations(...)` | src/agents/run_state.py:3981-3996; src/agents/run_context.py:1237-1265, 1267-1347 |
| Schema versioning policy | `CURRENT_SCHEMA_VERSION = "1.17"`; `SCHEMA_VERSION_SUMMARIES` documents approval-relevant bumps 1.6 ("Persists explicit approval rejection messages across resume flows"), 1.14 ("Scopes hosted MCP approvals … by server label"), 1.16 ("…exact call approval decision override a sticky decision") | src/agents/run_state.py:182-217 |
| Durability guidance (docs) | "Long-running approvals": use `to_json()`/`to_string()` to store pending work in a database or queue; serializer/deserializer options documented | docs/human_in_the_loop.md:187-203 |
| End-to-end durable example | Persist paused state to disk, reload (possibly different process), approve, resume | docs/human_in_the_loop.md:107-170 |
| Realtime approvals | `RealtimeSession.approve_tool_call(call_id, always=...)` / `reject_tool_call(...)` pop from `_pending_tool_calls` and reuse the same context-wrapper records | src/agents/realtime/session.py:969-1057 |
| Rejection message plumbing | `rejection_message` stored per call ID and as sticky default; surfaced via `resolve_approval_rejection_message` with run-wide formatter fallback | src/agents/run_context.py:692-702; src/agents/run_internal/tool_execution.py:1230-1297 |
| Timeout search result | Timeouts exist for model calls (`ModelCallTimeoutError`), tools (`timeout_seconds`, `ToolTimeoutError`), and MCP lifecycle (`connect_timeout_seconds`), but no approval-related timeout anywhere | src/agents/exceptions.py:478-516; src/agents/tool.py:495-507; src/agents/mcp/manager.py:196-206 |
| Audit search result | "audit" appears only in the sandbox subsystem (`SandboxAuditEvent`, audit sinks); tracing package contains no approval spans; grep for `approv` under `src/agents/tracing/` returns nothing | src/agents/sandbox/session/events.py:33; src/agents/sandbox/session/manager.py:17 |

## Answers to Dimension Questions

**1. How is approval requested?**
Three coordinated mechanisms. (a) *Manual interruptions:* during tool execution the runner first consults any stored decision (`context_wrapper.get_approval_status`, `src/agents/run_internal/tool_execution.py:1183-1190`); if none exists and the tool's `needs_approval` rule fires (`function_needs_approval`, `src/agents/run_internal/tool_execution.py:1300-1318`), a `ToolApprovalItem` becomes the tool-run result (`resolve_approval_interruption`, `src/agents/run_internal/tool_execution.py:1216-1227`), the turn ends in `NextStepInterruption` (`src/agents/run_internal/turn_resolution.py:1823-1829`), and the caller inspects `result.interruptions` (`src/agents/result.py:515-516`). (b) *Programmatic callbacks:* `on_approval` on shell/apply-patch/custom tools (`src/agents/run_internal/tool_actions.py:501-511`, `src/agents/tool.py:1375`) and `HostedMCPTool.on_approval_request` (`src/agents/tool.py:1097-1100`) can decide in code without pausing (`src/agents/run_internal/tool_execution.py:1191-1204`). (c) *Realtime sessions* expose `approve_tool_call`/`reject_tool_call` over WebSocket (`src/agents/realtime/session.py:969-1057`). Nested `Agent.as_tool()` interruptions propagate to the outer run and route to the nested state that owns them (`RunState._find_nested_approval_state`, `src/agents/run_state.py:1095-1253`).

**2. Are approval sessions durable?**
Yes — this is the strongest dimension of the design. Decisions live in `RunContextWrapper._approvals` and are copied into every checkpoint (`_copy_for_run_state`, `src/agents/run_context.py:117-131`). `RunState.to_json()` serializes approvals, hosted-MCP typed records, rejection messages, sticky scopes, and the invocation ledger (`src/agents/run_state.py:1732-1754`); `from_json` rebuilds them (`src/agents/run_context.py:1237-1347`). Persistence is versioned (`$schemaVersion`, `src/agents/run_state.py:1768`) with per-version summaries (`src/agents/run_state.py:186-217`), backward-read support, and a fixture corpus proving historical snapshots still resume (`tests/test_run_state_compatibility_corpus.py:165-250`). Safety rules on restore: per-call decisions whose call ID has no ledger binding require re-approval (`_mark_restored_unbound_approval_call_ids`, `src/agents/run_context.py:1308-1316`), and malformed ledger lifecycle data fails validation (`src/agents/run_context.py:1277-1299`). So yes — an approval session survives not just a browser refresh but a full process restart, provided the application stores the JSON (documented pattern: `docs/human_in_the_loop.md:187-203`).

**3. Can approvals be scoped?**
Yes, along two axes. *Granularity:* per-call decisions append the specific `call_id` to `approved`/`rejected` lists; `always_approve`/`always_reject` flip the record to a boolean sticky decision (`_apply_approval_decision`, `src/agents/run_context.py:998-1037`; precedence logic where exact-call overrides sticky: `src/agents/run_context.py:649-661`). *Identity:* sticky decisions bind to an approval-scope fingerprint derived from invocation type + function-tool lookup key (bare/namespaced/deferred) or hosted MCP `server_label` + tool name (`tool_invocation_approval_scope`, `src/agents/_tool_invocation.py:236-279`), checked by `_matching_sticky_approval_keys` (`src/agents/run_context.py:407-472`). Tests confirm cross-server isolation (`tests/test_run_context_approvals.py:30`), no bare-name aliasing between namespaced tools (`tests/test_run_context_approvals.py:531,563`), and that an incomplete hosted identity cannot create permanent authority (`tests/test_run_context_approvals.py:360-379`). There is no wildcard/"approve all tools" scope — scoping stops at one tool identity or one call, which is conservative by design.

**4. Do approvals time out?**
No evidence found. I searched `src/agents/` for `expir`, `ttl`, `approval_timeout`, and combinations of `timeout` with `approv`/`pending`: every hit concerns model-call retries/timeouts (`src/agents/exceptions.py:478-484`), per-tool invocation timeouts (`src/agents/tool.py:496-507`), MCP connect/cleanup timeouts (`src/agents/mcp/manager.py:196-257`), or session-storage TTLs in optional memory extensions (`src/agents/extensions/memory/redis_session.py:368-392`, `encrypt_session.py:101-130`) — none govern pending approvals. A pending approval remains valid indefinitely inside its serialized state; expiry is left entirely to the embedding application (the docs suggest storing version markers alongside long-lived states but define no deadline: `docs/human_in_the_loop.md:201-203`).

**5. Are approvals audited?**
No evidence found for a dedicated audit trail. The word "audit" in `src/agents/` occurs only in the sandbox session event system (`src/agents/sandbox/session/events.py:33`, `manager.py:17`, `sandbox_session.py:163-222`); `src/agents/tracing/` contains no approval-related spans (grep for `approv` returns nothing). Observable traces of decisions are indirect: rejection text is persisted and replayed to the model (`rejection_messages`, `sticky_rejection_message`, serialized since schema 1.6 — `src/agents/run_state.py:193, 1316-1321`), rejected calls produce synthetic model-visible outputs (`append_approval_error_output`, `src/agents/run_internal/approvals.py:24-43`), and tool executions themselves get tracing spans (`with_tool_function_span`, `src/agents/run_internal/tool_execution.py:1120`). But the SDK records neither approver identity, decision timestamps, nor an immutable decision history; applications must layer their own auditing.

## Architectural Decisions

1. **Approvals are run-scoped state, not a service.** Decisions live in `RunContextWrapper._approvals` (`src/agents/run_context.py:89`) and die with the run unless explicitly serialized into `RunState`. There is no external approval store or broker; durability is achieved by making the whole pause/resume snapshot portable JSON (`src/agents/run_state.py:1704-1846`). This keeps the SDK storage-agnostic but delegates retention, expiry, and multi-process coordination to hosts.
2. **Canonical invocation ledger as the trust anchor for restore.** Since schema 1.15, every recognized tool call registers a `_ToolInvocationRecord` with type/scope/fingerprint (`src/agents/run_context.py:46-53`, `src/agents/_tool_invocation.py:155-219`), and restored per-call decisions without a matching ledger entry are demoted to "requires re-approval" (`src/agents/run_context.py:1308-1316`). This deliberately trades convenience for safety: replaying an old approval against a changed payload fails closed (tested in `tests/test_run_state.py:3176-3304`).
3. **Fail-closed evaluation of dynamic approval rules.** Callable `needs_approval` rules are skipped (approval required) when arguments cannot be parsed safely — malformed JSON, non-object payloads, NaN/Infinity constants (`parse_function_tool_arguments` rejecting nonstandard constants, `src/agents/util/_approvals.py:18-29`; enforcement at `src/agents/run_internal/tool_execution.py:1306-1310`).
4. **Identity-keyed sticky decisions for hosted MCP.** Permanent hosted decisions require a complete `(server_label, tool_name)` identity and raise `UserError` otherwise (`src/agents/run_context.py:902-907`), preventing one server's approval from authorizing another's identically-named tool.
5. **Versioned, forward-incompatible state format.** `to_json()` always stamps `CURRENT_SCHEMA_VERSION` and older readers reject newer versions intentionally (`src/agents/run_state.py:175-218`), with a compatibility corpus test guarding every released version's readability (`tests/test_run_state_compatibility_corpus.py:165-230`).

## Notable Patterns

- **Detached decision snapshots:** `RunState.get_interruptions()` returns defensively-copied approval items built through hardened public-Pydantic copy paths that reject cyclic data, non-finite numbers, unsafe subtype hooks, and key collisions (`src/agents/run_state.py:683-745`, `985-1017`), so callers can hold snapshots without aliasing live state (`tests/test_run_state.py:1626-1726`).
- **Nested approval routing:** interruptions raised inside `Agent.as_tool()` sub-runs attach to the nested `RunState` and are resolved recursively through `_find_nested_approval_state`, with ambiguity errors when identities collide between parent and child (`src/agents/run_state.py:1241-1253`; behavior tests in `tests/test_hitl_error_scenarios.py:434-535`).
- **Decision mirroring across aliases:** `_resolve_approval_keys` computes multiple keys (qualified name, bare name, legacy deferred key) so a decision applies consistently regardless of how the model later addresses the tool (`src/agents/run_context.py:182-192`; `src/agents/_tool_identity.py:607-655`).
- **Reconciliation of resumed vs. fresh requests:** `process_hosted_mcp_approvals`/`collect_manual_mcp_approvals` dedupe repeated `mcp_approval_request` items across resumes and classify pending-vs-current identity conflicts rather than silently reusing stale approvals (`src/agents/run_internal/tool_execution.py:1363-1512`).

## Tradeoffs

- **Durability vs. operational burden:** Because there is no built-in store, hosts must persist `RunState` themselves and handle context (de)serialization edge cases (mapping contexts round-trip free; custom objects need serializers or lose their type with warnings — `src/agents/run_state.py:1460-1583`).
- **Safety vs. friction on restore:** Requiring re-approval for unbound per-call decisions after restore prevents stale authorization but means long-lived workflows may repeatedly interrupt humans after upgrades or payload changes.
- **Sticky approvals improve UX, shrink oversight:** An `always_approve` decision silences all future prompts for that tool identity for the rest of the run with no revocation API short of abandoning the run context; combined with no audit log, misuse would be invisible to the SDK.
- **Richness vs. complexity of scoping:** The multi-key alias resolution (bare/namespaced/deferred/hosted tuples) is precise but intricate — much of `run_context.py` exists solely to decide which record governs a given call (`src/agents/run_context.py:1108-1235`), a maintenance cost paid for cross-server and namespace isolation guarantees.

## Failure Modes / Edge Cases

- **Duplicate invocation identities:** applying a decision when two pending approvals share one tool-invocation identity raises `UserError` demanding unique call IDs (`src/agents/run_state.py:1087-1093`, `1241-1253`).
- **Empty call IDs:** per-call decisions with empty call IDs raise before mutation (`src/agents/run_context.py:912-928`), and empty tool call IDs fail before approval or execution (`tests/test_tool_approval_call_id_reuse.py:531-652`).
- **Changed tool under an approved call ID:** if the model reuses a call ID for different arguments, the invocation fingerprint mismatch raises `ModelBehaviorError` instead of executing under a borrowed approval (`src/agents/run_context.py:344-352`; handoff-swap case tested at `tests/test_hitl_error_scenarios.py:1663`).
- **Partial resolution:** resuming after approving only some interruptions continues resolved calls while unresolved ones re-pause (`docs/human_in_the_loop.md:57`; `tests/test_hitl_error_scenarios.py:536-556`).
- **Malformed serialized state:** non-mapping approvals are ignored, invalid ledger entries abort restoration with validation errors, and parse failures redact the offending state string (`src/agents/run_context.py:1240-1245`, `1277-1299`; `src/agents/run_state.py:2127-2148`).
- **Stale sticky bindings:** current-schema restores refuse to reconstruct missing sticky bindings from interruption items alone (legacy schemas did reconstruct them), requiring re-approval — a deliberate regression-tightening visible in `tests/test_run_state.py:3053-3204`.

## Future Considerations

- **Approval timeouts/TTLs:** a deadline field on `_ApprovalRecord` plus validation at `get_approval_status` would let hosts express "pending approvals expire in N hours" natively; today they must detect staleness out-of-band (dimension question 4 currently answers "no").
- **Decision auditing hooks:** emitting a trace span or lifecycle hook event on `approve_tool`/`reject_tool` (`src/agents/run_context.py:1043-1063`) would give parity with the sandbox audit-event system and capture actor/timestamp without changing the wire format.
- **Revocation of sticky decisions:** a `clear_approval`/`reset_sticky` API would let long-running sessions respond to policy changes without discarding the run context.
- **Cross-process coordination:** since sticky state is per-checkpoint copy (`_copy_for_run_state`, `src/agents/run_context.py:117-131`), two branches resumed from one snapshot make independent decisions; hosts needing shared approval memory must build it externally.

## Questions / Gaps

- **Who approved?** No approver identity, timestamp, or reason-for-approval metadata is captured anywhere in `_ApprovalRecord` (`src/agents/run_context.py:56-68`); auditability depends entirely on host-side logging. No evidence found of an alternative mechanism; searched `audit`, `who`, `actor`, `timestamp` within approval paths.
- **Is there any maximum lifetime for a paused run?** None found — `max_turns` bounds turns per resume (`src/agents/run_state.py:796-797`) but not wall-clock age of pending approvals.
- **Do voice pipelines support approvals?** The realtime module does (`src/agents/realtime/session.py:969-1057`); no separate approval surface was found in `src/agents/voice/` (searched `approv` — no hits), suggesting voice relies on realtime sessions or does not gate tools.

---

Generated by Dimension 14.02 (Approval Session Design) against openai-agents-sdk.
