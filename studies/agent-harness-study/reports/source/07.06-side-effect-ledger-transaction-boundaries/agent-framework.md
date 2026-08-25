# Source Analysis: agent-framework

## Dimension 07.06 — Side-Effect Ledger and Transaction Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, `python/packages/*`), .NET (`dotnet/src/*`), Go (pointer only) |
| Analyzed | 2026-08-24 |

## Summary

Microsoft's agent-framework does not ship a single unified "side-effect ledger" service that journals every external mutation an agent performs. Instead, side-effect accountability is distributed across four cooperating mechanisms:

1. **A confirmation-record ledger for tool execution (HITL approvals).** Tools can be gated behind `approval_mode="always_require"` (`packages/core/agent_framework/_tools.py:1763`); surfaced approval requests are stored as immutable snapshots in session state (`packages/core/agent_framework/_tools.py:2141-2157`), responses are bound to the recorded request and consumed exactly once (`packages/core/agent_framework/_tools.py:2182-2213`), and the AG-UI transport layer adds a full server-owned occurrence state machine (`PENDING → CLAIMED → EXECUTING → SETTLED/REJECTED/CANCELLED/EXPIRED/INDETERMINATE`) with idempotency keys and retention windows (`packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-114`, `173-192`). This is the closest thing to a durable "what did the agent change" record: every approved external action has a stored request snapshot, a terminal outcome, and a retained replayable result.
2. **Transaction boundaries at the workflow layer.** Workflow shared state uses staged pending buffers committed (or discarded) only at superstep boundaries (`packages/core/agent_framework/_workflows/_state.py:14-18`, `90-104`); the runner commits state then writes a checkpoint after every superstep (`packages/core/agent_framework/_workflows/_runner.py:160-168`) and persists pending `request_info` events so interrupted HITL flows resume with their correlation IDs intact (`packages/core/agent_framework/_workflows/_checkpoint.py:59-91`; `_runner_context.py:509-514`).
3. **A run-persistence gate over transcript durability.** Durable history writes issued during a run are collected by a gate and either flushed after the run's egress verdict permits the content or dropped entirely (`packages/core/agent_framework/_sessions.py:1078-1085`, `1151-1166`, `1169-1179`) — a genuine commit/drop transaction boundary over the transcript side effect.
4. **Observability-based auditing.** Every function/tool invocation is wrapped in an OTel span recording name, arguments, results, error type, and duration (`packages/core/agent_framework/_tools.py:733-801`), plus a duration histogram (`_tools.py:798-800`).

What is **absent** is rollback/compensation for already-committed external effects: there is no saga/compensation API anywhere (searches for `saga|compensat|undo` in Python sources return only unrelated hits, e.g. Docker isolation-flag handling at `packages/tools/agent_framework_tools/shell/_docker.py:296`). The framework instead engineers *exactly-once / no-double-execution* semantics and records honest uncertainty (`INDETERMINATE`) when a side effect may have started without settling. The function-calling loop specification explicitly declares this area high risk ("small changes can produce duplicate side effects...") and mandates scenario-matrix regression coverage (`docs/specs/004-python-function-calling-loop.md:30-45`).

## Rating

**7 / 10.** The confirmation/approval subsystem is a genuinely mature control-plane ledger: explicit interfaces (`Content.from_function_approval_request/response`, `ApprovalLifecycle`), immutable session-backed snapshots keyed by provider `call_id`, idempotency-key retry proof, capacity limits and retention windows, per-occurrence locking, fail-closed batch aborts, and an extensive test suite including failure-injection tests (`packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:720-830`). Workflow checkpointing provides real internal transaction boundaries. It falls short of 8–10 because: the approval lifecycle is process-local/in-memory (not durable across restarts; `ApprovalLifecycle.__init__` keeps dicts + RLocks, `_approval_lifecycle.py:236-265`), the security middleware audit log is a plain in-memory list (`packages/core/agent_framework/security.py:1697-1698`), there is no generic ledger of *external* mutations made by ungated tools, no compensation/rollback mechanism exists, and cooperative cancellation cannot stop a synchronous tool already running in a worker thread — its side effects complete even though the result is discarded (`_tools.py:1863-1876`; spec `docs/specs/004-python-function-calling-loop.md:333-338`).

## Evidence Collected

Every entry cites `path:line` relative to `sources/agent-framework`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Side-effecting tool surface | Function tools executed through the invocation layer; per-call result groups | `python/packages/core/agent_framework/_tools.py:1725-1879` |
| Write-gated file tools | All file-access tools default to `approval_mode="always_require"`; opt-outs via flags | `python/packages/core/agent_framework/_harness/_file_access.py:1444-1445` |
| Write-gated skill tools | Skill tools (`load_skill`, `read_skill_resource`, `run_skill_script`) default to approval required | `python/packages/core/agent_framework/_skills.py:2442-2454` |
| Shell tools (.NET) | `ShellExecutor` family with policy gate returning allow/deny + rationale before execution | `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellPolicy.cs:53-142` |
| Pre-execution confirmation | Batch classification pauses whole batch when any call requires approval | `python/packages/core/agent_framework/_tools.py:1775-1796` |
| Approval-request records | Immutable snapshots of surfaced requests stored in one active session batch; duplicate IDs rejected | `python/packages/core/agent_framework/_tools.py:2141-2157`; spec `docs/specs/004-python-function-calling-loop.md:361-366` |
| Approval-response binding | Response honored only against pending server-held snapshot; executable data sourced from request, not response | `python/packages/core/agent_framework/_tools.py:2182-2213`; spec line 366-371 |
| Already-approved sibling groups | Hidden approved calls persisted keyed to visible approval ids, reinjected on resume | `python/packages/core/agent_framework/_tools.py:2249-2270` |
| Server-owned occurrence ledger | Status enum incl. `EXECUTING`/"external side effect may begin", `INDETERMINATE`; identity = (thread_id, occurrence_id, interrupt_id, call_id) | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-144` |
| Idempotency keys | Optional key on occurrence + authorized intent; matching key allows claim recovery, otherwise marked indeterminate | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:186`, `320-321`, `828-847` |
| Settlement records | `settle()` retains exactly-one replayable `function_result` under original call identity as `ApprovalOutcome` | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:849-875` |
| Retention/expiry safeguards | Pending expiry (default 24h), indeterminate retention 7 days, terminal retention 15 min, capacity cap 10k | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:242-247`, `704-740` |
| Lifecycle audit events | Structured log per transition: registration, execution_start, settlement, indeterminate_recovery, expiration, retention_purge, authority_failure | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:762-778`, `57-60` |
| Exactly-once contract | "An approved tool executes exactly once"; rejected executes zero times with synthetic rejection result | `docs/specs/004-python-function-calling-loop.md:377-379` |
| Batch transaction boundary | On `MiddlewareFailure`: cancel in-flight siblings, no new tool starts; sync tools may still complete side effects (documented gap) | `python/packages/core/agent_framework/_tools.py:1863-1876` |
| Superstep state transactions | Pending buffer with `commit()`/`discard()` at superstep boundaries | `python/packages/core/agent_framework/_workflows/_state.py:14-18`, `90-104` |
| Checkpoint boundaries | Runner commits state then checkpoints each superstep; entry checkpoint owned by `Workflow` | `python/packages/core/agent_framework/_workflows/_runner.py:106-113`, `160-168` |
| External-ID persistence in checkpoints | Pending `request_info` events (with `request_id`) saved/restored from checkpoints | `python/packages/core/agent_framework/_workflows/_checkpoint.py:59-91`; `_runner_context.py:509-514` |
| MCP external task IDs | `task_id` captured before any reconnect retry; never re-issue augmented `tools/call` (double-execution guard) | `python/packages/core/agent_framework/_mcp.py:2246-2278`, comment at 2376 |
| Remote-work cancellation (compensation-adjacent) | Best-effort `tasks/cancel` on abandonment/cancellation paths; abandonment vs terminal-failure distinction | `python/packages/core/agent_framework/_mcp.py:2303-2307`, `2573-2608` |
| Transcript durability gate | `_RunPersistenceGate` collects deferred history writes; flush after egress verdict or drop on denial | `python/packages/core/agent_framework/_sessions.py:1078-1085`, `1151-1166`, `1169-1179` |
| Session-store durability | `FileSessionStore`: temp-file + `os.replace` atomic writes; corrupt snapshots quarantined, originals preserved | `python/packages/core/agent_framework/_sessions.py:1872-1895`, `1996-2022`, `2055-2065` |
| Tool-invocation audit trail | OTel span with gen_ai tool arguments/result attributes (gated by sensitive-data flag), error type, duration histogram | `python/packages/core/agent_framework/_tools.py:733-801` |
| Security violation audit log | `PolicyViolationDetector.audit_log` list with typed violation payloads (function, context label, turn) | `python/packages/core/agent_framework/security.py:1655-1698`, `2007-2046`, `2054-2055` |
| Approval waives violations once | Approval matched against exact violation set, consumed once, cannot authorize different/repeated call | `python/packages/core/agent_framework/security.py:2057-2076` |
| User-facing approval events (DevUI) | Mapped to `response.function_approval.requested/responded` incl. policy-violation reason/context label | `python/packages/devui/agent_framework_devui/_mapper.py:1778-1824` |
| User-facing change summary (AG-UI) | Thread snapshot store replays client-visible state; `confirm_changes` resolves synthetic confirmation back to original `function_call_id` | `python/packages/ag-ui/agent_framework_ag_ui/_run_common.py:923-940`; `_message_adapters.py:86-110` |
| .NET approval state mirror | `ToolApprovalState` holds rules, collected responses, queued/surfaced approval requests | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalState.cs:13-64` |
| .NET batch classification | Approval requests collected (not yielded) so full batch classified first | `dotnet/src/Microsoft.Agents.AI/Harness/ToolApproval/ToolApprovalAgent.cs:243`, `575` |
| .NET checkpoint durability | `FileSystemJsonCheckpointStore` with locked index file and per-checkpoint file writes/deletes | `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:72`, `166-192` |
| .NET checkpoint recovery | `WorkflowSessionCheckpointRecovery` resumes queued work from selected checkpoint | `dotnet/src/Microsoft.Agents.AI.Workflows/WorkflowSessionCheckpointRecovery.cs:24-44` |
| Failure-mode regression tests | "A possibly started side effect is not retried"; idempotency-key retry test; approved-side-effect-executes-once test | `python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:720-753`, `819`; `test_endpoint.py:7263-7351` |

## Answers to Dimension Questions

**1. What external changes did the agent make?**
Partially answerable. For *approved* tool executions, yes: the AG-UI `ApprovalOccurrence` retains name, exact arguments, owner, decision, outcome, and replayable result under the original call identity (`_approval_lifecycle.py:173-192`, `849-875`), and session state holds immutable request snapshots (`_tools.py:2141-2157`). For *ungated* tools, the system relies on OTel spans/logs (`_tools.py:733-801`) and the conversation transcript (call/result pairs); there is no dedicated registry of files written, emails sent, or rows mutated. Built-in write surfaces mitigate this by requiring approval by default (`_file_access.py:1444-1445`; `_skills.py:2454`).

**2. Are side effects auditable?**
Yes, at two levels. (a) Observability: every invocation emits an OTel span with tool arguments/results/error/duration plus a duration histogram (`_tools.py:733-801`), and the approval lifecycle emits structured transition logs (`registration`, `execution_start`, `settlement`, `indeterminate_recovery`, `expiration`, `retention_purge`, `authority_failure`; `_approval_lifecycle.py:762-778`). (b) History: approval control-plane contents may be retained by history providers for audit while being filtered from model replay (`docs/specs/004-python-function-calling-loop.md:389-393`). Security middleware additionally maintains an in-process violation audit log (`security.py:1697-1698`) — but it is not durable.

**3. Can failed side effects be compensated?**
No general mechanism. There is no rollback/undo/compensation API (searched `saga|compensat|rollback|undo` across `python/packages` and `dotnet/src`; only unrelated matches). What exists instead: prevention of double-execution (MCP never re-sends an augmented call whose receipt is unknown, `_mcp.py:2376`; approval occurrences cannot re-authorize, spec line 370-371), remote-task cancellation as best-effort mitigation (`_mcp.py:2573-2608`), and honest uncertainty marking (`mark_indeterminate`, `_approval_lifecycle.py:808-826`). Known unrecoverable case: a synchronous tool body in a worker thread completes its side effects during batch cancellation while its result is discarded (`_tools.py:1866-1876`).

**4. Are external IDs stored?**
Yes, consistently. Approval requests are keyed by the provider `call_id` with duplicate-ID rejection (spec lines 364-365; `_tools.py:2121-2122`); AG-UI identities carry `(thread_id, occurrence_id, interrupt_id, call_id)` plus optional `idempotency_key` (`_approval_lifecycle.py:137-144`, `186`); MCP long-running tasks retain the server-issued `task_id` across polls, reconnects, and cancels (`_mcp.py:2274-2284`, `2519-2546`); workflow `request_info` correlation IDs survive checkpoint round-trips (`_checkpoint.py:91`; `_runner_context.py:509-514`).

**5. Are users shown what changed?**
Yes, for gated actions. Surfaced approval requests expose tool name and parsed arguments to the UI, including security-policy violation details (`devui _mapper.py:1778-1813`). Decisions map to visible responded events with the approved flag (`_mapper.py:1815-1824`). Resumed responses return all newly resolved approved/rejected terminal results before any final assistant message (spec lines 380-383). AG-UI thread snapshots keep a client-replayable record, and `confirm_changes` ties the synthetic confirmation back to the original `function_call_id` (`_run_common.py:923-940`; AGENTS.md protocol notes). There is no aggregate end-of-run "changes made" summary component.

## Architectural Decisions

1. **Confirmation before execution rather than journaling after.** Side-effecting built-ins default to `always_require` approval (`_file_access.py:1444-1445`, `_skills.py:2454`), and a mixed batch pauses entirely before any call executes (`_tools.py:1775-1796`, `.NET` equivalent `ToolApprovalAgent.cs:243`). The design goal is preventing unauthorized side effects rather than accounting for them afterwards.
2. **Immutable request snapshots + occurrence-scoped authority.** Authority derives exclusively from the stored request snapshot, never from replayed history or response payloads (`_tools.py:2195-2209`; spec lines 366-369). Reused `call_id`s are correlated per logical occurrence, preventing stale-authorization replay (spec line 341).
3. **Fail-closed aborts with explicit uncertainty instead of compensation.** `MiddlewareFailure` cancels the batch and discards results but documents that sync tools may still finish (`_tools.py:1866-1876`); `ApprovalStatus.INDETERMINATE` records "may have executed but no retained terminal outcome" (`_approval_lifecycle.py:77-78`, `808-826`). The system prefers truthful ambiguity over fabricated reversibility.
4. **Idempotency keys as the retry-proof primitive.** A claimed occurrence may only be retried safely with a matching pre-declared key; otherwise recovery degrades to indeterminate (`_approval_lifecycle.py:843-847`).
5. **Superstep commit/checkpoint as the workflow transaction unit.** State writes are invisible cross-executor until the superstep commits (`_state.py:14-18`), and checkpoints capture both state and pending external requests (`_checkpoint.py:59-91`).
6. **Spec-enforced change management.** The function-calling loop is declared high-risk with mandatory scenario-matrix regression tests and core-team review ownership (`docs/specs/004-python-function-calling-loop.md:35-51`) — process-level protection for exactly-once guarantees.

## Notable Patterns

- **State machine as ledger:** eight-status `ApprovalStatus` with `is_terminal`/`is_purgeable` properties and retention-aware purging (`_approval_lifecycle.py:93-119`, `704-740`).
- **Hold-back-then-flush persistence:** `_RunPersistenceGate` queues durable writes until an egress verdict permits them, then flushes or drops (`_sessions.py:1151-1179`) — a two-phase-commit flavor for transcript content.
- **Already-approved batching bypass:** non-approval-required siblings of a pending approval are auto-approved, hidden in session state, and reinjected on resume so batches stay consistent (`_tools.py:1796-1832`, `2249-2270`).
- **Atomic durable writes everywhere:** temp-file + `os.replace` for session snapshots, todo state, and memory index (`_sessions.py:2019-2022`; `_harness/_todo.py:433-440`; `_harness/_memory.py:123-133`); corrupt snapshots quarantined rather than deleted (`_sessions.py:2055-2065`).
- **Best-effort remote cancellation with abandonment taxonomy:** MCP distinguishes "remote may still be running" (cancel) from "server already done" (do not cancel) (`_mcp.py:2573-2608` and docstring at 360-374).

## Tradeoffs

- **Safety vs. autonomy friction:** defaulting every write-ish harness tool to human approval maximizes auditability but requires explicit opt-outs or auto-approval rules to run unattended (`_harness/_tool_approval.py` middleware with heuristic `auto_approval_rules`; `_file_access.py:1231-1244`).
- **In-memory ledgers vs. restart survivability:** `ApprovalLifecycle` and the security `audit_log` live in process memory; a crash loses pending-authority state (session-held request snapshots do persist via `FileSessionStore`, but the AG-UI occurrence state machine does not).
- **Exactly-once enforcement vs. liveness:** refusing to re-issue unknown-receipt MCP calls prevents double execution but surfaces as user-visible `ToolExecutionException("connection lost; task state unknown")` (`_mcp.py:2246-2248`, `2537-2543`).
- **Cooperative cancellation simplicity vs. side-effect truth:** async siblings stop cleanly, but thread-pool sync tools cannot be interrupted; the framework discards results yet cannot un-make effects (`_tools.py:1866-1876`).

## Failure Modes / Edge Cases

- **Aborted batch with completed sync tool:** side effect lands, result never reaches transcript/model/history — an unaudited divergence between world state and transcript (`_tools.py:1866-1876`; spec lines 333-338).
- **Indeterminate outcomes:** executions that may have begun but never settle become `INDETERMINATE` and are excluded from purgeable retention for 7 days (`_approval_lifecycle.py:113-119`, `242-244`) — callers must reconcile externally.
- **Capacity exhaustion:** protected occurrences beyond `max_entries` raise `ApprovalCapacityError` and emit `capacity_failure` (`_approval_lifecycle.py:357-359`).
- **Authority failures:** alias conflicts, mismatched keys, or settlement in wrong status raise typed errors and emit `authority_failure` telemetry (`_approval_lifecycle.py:56-60`, `333-349`, `89-90`).
- **Corrupt session snapshots:** malformed files quarantined; schema/version/state-decoder failures preserve the original file (`_sessions.py:1959-1972`) — ledger survives partial corruption.
- **Duplicate pending IDs treated as corruption:** loading state with duplicate request ids raises rather than silently merging (`_tools.py:2121-2122`, `2152-2153`).

## Future Considerations

- Persist the AG-UI `ApprovalLifecycle` occurrences (or export them into session state like core's approval snapshots) so approval authority and indeterminate markers survive process restarts.
- Introduce a durable, queryable external-change journal (even a simple append-only event sink fed from the existing OTel hooks) so "what did the agent change?" is answerable for ungated tools too.
- Add optional compensation hooks (post-execution undo callbacks) for built-in write tools such as `file_access_write`/`replace`, which already know prior content in many cases.
- Promote the in-memory `audit_log` in `security.py` to a pluggable sink interface.

## Questions / Gaps

- No evidence found of any generic side-effect ledger covering ungated third-party tools; searches for `ledger|audit|journal|saga|compensation` outside the approval/security/observability areas returned only Magentic progress-ledger code (an unrelated planning construct) and log comments.
- Whether hosted-service tools (executed remotely) report their outcomes back into any local ledger beyond `settle_forwarded`'s retained approval response could not be fully traced from Python alone; the forwarded-outcome path is implemented (`_approval_lifecycle.py:877-921`) but the provider-side persistence story was out of scope of this source's Python/.NET trees.
- The Go implementation is a stub pointer to a separate repository (`go/README.md:1-3`), so it contributes no evidence.

---

Generated by dimension 07.06 (Side-Effect Ledger and Transaction Boundaries) against `agent-framework`.
