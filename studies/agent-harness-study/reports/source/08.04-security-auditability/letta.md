# Source Analysis: letta

## 08.04 Security Auditability

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, PostgreSQL/SQLite, OpenTelemetry, ClickHouse) |
| Analyzed | 2026-08-24 |

## Summary

Letta's security auditability story is built almost entirely on two pillars: (1) a **durable, database-persisted human-in-the-loop (HITL) approval trail** expressed as first-class `role="approval"` messages in the `messages` table, and (2) **rich execution observability** (`steps`, `runs`, `provider_traces` tables plus OTel spans/metrics and optional ClickHouse LLM traces). Approval requests and responses are validated against pending state, are idempotent on retry, record the approver's boolean decision and denial reason, and are covered by integration tests. Memory edits additionally get an actor-attributed checkpoint history (`block_history` with `actor_type`/`actor_id`).

However, there is **no dedicated security-event log**: failed authentications are not logged, "audit" appears in only three code locations (all incidental), tool-execution telemetry events are gated behind a non-default verbose flag, and policy changes (toggling `requires_approval`) mutate agent state without a who/when decision record. Identity is also weakly bound: a single shared password middleware gates all requests, the acting user comes from a self-asserted `user_id` header that falls back to a default actor, and two auth functions referenced by the REST layer (`api_key_to_user`, `authenticate_user`) are not defined anywhere in this open-source tree — implying cloud-only implementations. An auditor can reconstruct *what* happened (tool calls, args, results, LLM payloads) very well, but reconstructing *who authorized it* depends on header hygiene rather than cryptographic identity.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, or fragile.**
The HITL approval trail is genuinely well-engineered (persisted, validated, idempotent, tested), which keeps this out of the 1–3 band. But the absence of any dedicated security-event log, unlogged auth failures, self-asserted actor identity, no audit records for approval-policy changes, and history-truncation semantics in block checkpoints prevent the "clear model with tests and operational safeguards" bar of 7–8.

## Evidence Collected

Every entry includes a file path with line numbers. All paths relative to `studies/agent-harness-study/sources/letta`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Approval as first-class message role | `MessageRole.approval = "approval"` enum value | letta/schemas/enums.py:116 |
| Approval fields on Message schema | `approval_request_id`, `approve`, `denial_reason`, `approvals` fields | letta/schemas/message.py:303-308 |
| Approvals persisted to DB | `messages` table columns `approval_request_id`, `approve`, `denial_reason`, `approvals` | letta/orm/message.py:75-83 |
| Approval request creation on gated tool call | Agent creates approval request message + sets `stop_reason=requires_approval` instead of executing | letta/agents/letta_agent_v2.py:1138-1153 |
| Policy definition: requires-approval tool rule | `RequiresApprovalToolRule` (returns available tools unchanged; enforcement lives in loop, not rule) | letta/schemas/tool_rule.py:348-357 |
| Tool-level default policy flag | `default_requires_approval` field on Tool schema and ORM column | letta/schemas/tool.py:59-60; letta/orm/tool.py:55 |
| Policy attach/detach API | `PATCH /v1/agents/{agent_id}/tools/approval/{tool_name}` → `modify_approvals_async` | letta/server/rest_api/routers/v1/agents.py:707-740; letta/services/agent_manager.py:3064-3088 |
| Approval response validation | Rejects approvals with no pending request; validates tool_call_ids match pending request | letta/agents/helpers.py:287-294 |
| Approval idempotency | Retry detection scans last 10 in-context messages + full history post-compaction for matching tool returns | letta/agents/helpers.py:234-266 |
| Block during pending approval | Regular message while approval pending raises `PendingApprovalError`; error text at errors.py | letta/agents/helpers.py:309-310; letta/errors.py:49-53 |
| Denial recording (user) | Denied call persisted as error tool-return message including user's denial reason | letta/agents/letta_agent_v2.py:1087-1120; letta/server/rest_api/utils.py:230-264 |
| Denial recording (automated) | Run cancellation synthesizes denial tool returns for pending approvals | letta/services/run_manager.py:655-679; letta/server/rest_api/utils.py:238-241 |
| HITL integration tests | `test_invoke_approval_request`, `test_send_approval_without_pending_request`, `test_send_approval_message_with_incorrect_request_id`, `pending_approval` retrieval assertions | tests/integration_test_human_in_the_loop.py:185-293 |
| Client-side-tool pause semantics tests | Execution pauses with `requires_approval` when client tools override server tools | tests/integration_test_client_side_tools.py:100-102,242-255 |
| Memory edit attribution | `block_history` rows store `actor_type` (`ActorType.LETTA_AGENT` vs `LETTA_USER`) and `actor_id` per checkpoint | letta/orm/block_history.py:36-37; letta/services/block_manager.py:888-900; letta/schemas/enums.py:241 |
| Per-step execution record | `steps` table: provider, model, stop_reason, trace_id, request_id, error_type/data, status | letta/orm/step.py:20-98 |
| Per-run record | `runs` table: status, stop_reason, callback_url/status_code/error, timing | letta/orm/run.py:22-77 |
| Full LLM payload capture | `provider_traces` table stores raw request/response JSON keyed by step, agent, run, org, user | letta/orm/provider_trace.py:15-49 |
| LLM trace query APIs | `GET /v1/steps/{step_id}/trace`; ClickHouse `LLMTraceReader` for debugging/analytics/auditing | letta/server/rest_api/routers/v1/steps.py:97-112; letta/services/llm_trace_reader.py:3,87 |
| ClickHouse trace schema | `LLMTrace`: org/project/agent/run/step/OTEL trace ids, full request/response JSON, error fields | letta/schemas/llm_trace.py:14-104 |
| Provider-trace write path | Gated by `settings.track_provider_trace` (default True); backend selection postgres/clickhouse/socket | letta/settings.py:387,573; letta/adapters/simple_llm_stream_adapter.py:302 |
| Tool execution metrics | `tool_execution_counter` + latency histogram recorded per execution with success/failure attribute | letta/services/tool_executor/tool_execution_manager.py:113-121,156-160 |
| Tool I/O capture in sandbox | Local sandbox subprocess always captures stdout/stderr into `ToolExecutionResult` | letta/services/tool_sandbox/local_sandbox.py:185-228 |
| Tool-call event logging (gated) | `log_telemetry` logs tool name/args at execute start/finish but only if `verbose_telemetry_logging` enabled (default False) | letta/agents/letta_agent_v2.py:1129-1136,1176-1181; letta/utils.py:1136-1149; letta/settings.py:529 |
| Request log context | LoggingMiddleware extracts actor/org/project/primitive IDs into structured log context; logs unhandled exceptions with request context | letta/server/rest_api/middleware/logging.py:32-122,127-172 |
| Request ID correlation | `x-api-request-log-id` header propagated via contextvar/request.state for step↔API-log correlation | letta/server/rest_api/middleware/request_id.py:32-64 |
| Auth gate | Single shared password via header/bearer; health endpoints exempt; 401 returned without any logging | letta/server/rest_api/middleware/check_password.py:10-31 |
| Bearer→user mapping | Password match ⇒ admin default user; otherwise `server.api_key_to_user(...)` | letta/server/rest_api/auth_token.py:11-18 |
| Actor resolution | Acting user from self-asserted `user_id` header; falls back to default actor unless `no_default_actor` set | letta/server/rest_api/dependencies.py:38-61; letta/services/user_manager.py:113-135 |
| Missing auth implementation | `api_key_to_user`/`authenticate_user` called on server but defined nowhere in OSS tree | letta/server/rest_api/auth/index.py:36; letta/server/rest_api/auth_token.py:17 |
| Secret encryption at rest | MCP OAuth tokens stored in encrypted `_enc` columns alongside plaintext columns | letta/orm/mcp_oauth.py:41-54 |
| Sensitive data exposure in traces | Request body keys dumped into span attributes (`http.request.body.{key}`) without redaction | letta/otel/tracing.py:114-124 |
| No dedicated audit table | Full ORM inventory contains no audit/security-event table; grep for `audit` yields only comments/docstrings | letta/orm/ (directory listing); letta/services/block_manager.py:805 |

## Answers to Dimension Questions

**1. Who did what?**
Partially answerable, weakly attributed. *What* is excellent: every tool call (`tool_calls` JSON column, letta/orm/message.py:46), its arguments, result, stdout/stderr (letta/services/tool_sandbox/local_sandbox.py:185-228), the enclosing step (letta/orm/step.py:20-98), run (letta/orm/run.py:22-77), and even raw LLM request/response payloads (letta/orm/provider_trace.py:25-26) are durably recorded. *Who* is fragile: the acting principal is a self-asserted `user_id` HTTP header (letta/server/rest_api/dependencies.py:39), authenticated only by a single server-wide shared password (letta/server/rest_api/middleware/check_password.py:23-27), and silently falls back to a default actor unless operators enable `no_default_actor` (letta/services/user_manager.py:122-135). The one strong exception is memory checkpoints, which record both actor type (agent vs user) and actor id (letta/services/block_manager.py:888-900).

**2. What policy allowed it?**
Reconstructable but implicit. The policy is the agent's `tool_rules` list containing `RequiresApprovalToolRule` entries (letta/schemas/tool_rule.py:348-357) seeded from `Tool.default_requires_approval` (letta/schemas/tool.py:59-60) and mutated via `modify_approvals_async` (letta/services/agent_manager.py:3064-3088). Because rules live on the agent row, an auditor can infer the policy at a point in time only from the current DB state or from the presence of approval-request messages in history — there is no policy-version or decision-ID record. Notably, the enforcement check itself is a loop-side membership test (letta/agents/letta_agent_v2.py:1138), so the "decision" to require approval leaves no explicit record beyond the resulting approval message.

**3. Was a human involved?**
Yes, detectably, for approved-gated tools. The protocol forces a stop: gated calls produce a persisted approval-request message and `stop_reason="requires_approval"` (letta/agents/letta_agent_v2.py:1140-1153), further messages are blocked with `PendingApprovalError` until resolved (letta/agents/helpers.py:309-310; letta/errors.py:49-53), and the human's response is persisted with `approve` boolean, optional `denial_reason`, and per-tool-call `approvals` list (letta/server/rest_api/utils.py:213-227). Automated denials from run cancellation are also materialized as error tool returns (letta/services/run_manager.py:671-679), so the trail distinguishes neither-silently-dropped nor conflates machine/human outcomes. Caveat: nothing cryptographically binds the approving party to a verified human identity beyond the header-based actor.

**4. Can auditors reconstruct the decision?**
Largely yes for the action chain, with gaps. Given a run ID, an auditor can walk runs → steps → messages → provider traces via REST (letta/server/rest_api/routers/v1/runs.py:46-80; letta/server/rest_api/routers/v1/steps.py:97-158) and see exactly which tool was requested, what the LLM argued, whether approval was requested, the approve/deny outcome and reason, and the final execution result — correlated by OTEL `trace_id` and `request_id` (letta/orm/step.py:73-77; letta/schemas/llm_trace.py:63). Gaps: no record of *who changed* the approval policy and when (letta/services/agent_manager.py:3085-3086 mutates rules in place); undo operations delete "future" block-history checkpoints, rewriting memory audit history (letta/services/block_manager.py:874-883); and there is no tamper-evidence (no hash chains, append-only guarantees, or signatures anywhere in the ORM).

## Architectural Decisions

1. **Approvals are conversation messages, not a separate ledger.** Rather than an `audit_events` table, Letta models approval requests/responses as `role="approval"` rows in the same `messages` table as everything else (letta/orm/message.py:75-83). This buys transactional consistency with context management (summarizers deliberately protect pending approvals from eviction, e.g., letta/services/summarizer/self_summarizer.py:182-196) at the cost of making the audit trail inseparable from mutable conversation state.
2. **Policy-as-data on the agent row.** Approval requirements are stored as typed tool rules serialized into `Agent.tool_rules` (letta/schemas/tool_rule.py:360-373; letta/orm/agent.py), enabling per-agent, per-tool gating via simple membership checks in the loop (letta/agents/letta_agent_v2.py:1138) — simple to evaluate, but with no historical versioning.
3. **Observability split between Postgres (system of record) and ClickHouse/OTel (analytics).** Raw provider payloads go to `provider_traces` (letta/orm/provider_trace.py) and optionally ClickHouse `llm_traces` with denormalized cost columns (letta/schemas/llm_trace.py:14-104; letta/settings.py:505-511), while OTel spans provide request-level correlation (letta/otel/tracing.py:145-219).
4. **Multi-tier sandbox execution with uniform result capture.** Tools run in Modal/E2B/local subprocess sandboxes, all funneling stdout/stderr into `ToolExecutionResult` (letta/services/tool_executor/sandbox_tool_executor.py:69-94; letta/services/tool_sandbox/e2b_sandbox.py:120-167), giving consistent capability-usage records regardless of isolation tier.

## Notable Patterns

- **Stop-reason as coordination primitive:** `StopReasonType.requires_approval` (letta/schemas/letta_stop_reason.py:21,30) drives run lifecycle, streaming alignment, and cancellation logic (letta/services/run_manager.py:348,651), keeping HITL state machine-consistent across surfaces.
- **Idempotent approval retries:** duplicate approval responses are detected by scanning recent tool returns for matching tool_call_ids, then treated as keep-alive rather than double-execution (letta/agents/helpers.py:234-286).
- **Attributed memory checkpoints:** `BlockHistory` snapshots carry `actor_type`+`actor_id` (letta/orm/block_history.py:34-37), a rare place where agent-vs-human attribution is explicit and durable.
- **Structured log context injection:** middleware extracts primitive IDs from paths/headers/query into log context so every log line is joinable to entities (letta/server/rest_api/middleware/logging.py:61-111).
- **Encrypted-at-rest credentials:** OAuth tokens/secrets use paired plaintext/`_enc` columns with encrypted variants preferred (letta/orm/mcp_oauth.py:41-54; letta/schemas/secret.py:13-17).

## Tradeoffs

- **Conversation-as-audit-ledger vs. immutability:** messages are designed to be summarized, compacted, and trimmed (summarizer protections aside, letta/services/summarizer/summarizer_sliding_window.py:133-197). The approval trail survives compaction windows today, but it shares retention/deletion semantics with chat data — an auditor's record can shrink with the context window.
- **Simplicity of shared-password auth vs. attribution:** single-password gating (letta/server/rest_api/middleware/check_password.py) makes local/dev deployment trivial but makes multi-human attribution depend entirely on clients honoring the `user_id` header convention.
- **Verbose-telemetry opt-in vs. default-off noise:** detailed tool-arg logging exists but defaults off (letta/settings.py:529; letta/utils.py:1146), trading forensic richness for log hygiene — and pushing sensitive args into traces when on (see failure modes).
- **ClickHouse analytics depth vs. deployment surface:** the richest query layer (`LLMTraceReader`, letta/services/llm_trace_reader.py:83+) requires operating ClickHouse plus `store_llm_traces`/endpoint config (letta/services/llm_trace_writer.py:78); base Postgres tracing is on by default (`track_provider_trace=True`, letta/settings.py:387) but ClickHouse traces are not.

## Failure Modes / Edge Cases

- **OSS/cloud drift breaks auth path:** `/auth` and bearer flows call `server.api_key_to_user(...)`/`authenticate_user()` which do not exist in this tree (letta/server/rest_api/auth/index.py:36; letta/server/rest_api/auth_token.py:17) — in a pure OSS deployment these raise, leaving the shared-password path as the only working gate; there is no evidence of graceful handling.
- **Unlogged auth failures:** `CheckPasswordMiddleware.dispatch` returns 401 with zero logging (letta/server/rest_api/middleware/check_password.py:29-31); brute-force attempts against the single shared password leave no trace.
- **Silent exception swallowing on trace reads:** `retrieve_trace_for_step` catches all exceptions and returns `None` (letta/server/rest_api/routers/v1/steps.py:109-110), which can mask storage failures from auditors.
- **Sensitive data in traces:** request-body keys/values are written verbatim into span attributes with no redaction (letta/otel/tracing.py:114-124), so enabling tracing exports potentially secret-bearing payloads to the OTLP endpoint; similarly, `log_telemetry` emits full tool args (letta/agents/letta_agent_v2.py:1129-1136).
- **History rewriting on undo:** creating a new block checkpoint deletes later checkpoints (letta/services/block_manager.py:874-883), so a redo-chain branch is erased from `block_history` — prior states recorded in good faith become unrecoverable through the normal API.
- **Approval validation window:** idempotency scan only inspects the last 10 in-context messages before falling back to a single most-recent tool message (letta/agents/helpers.py:238,253-260); adversarial or high-volume interleavings could plausibly slip past the "already processed" heuristic, though the strict pending-request check still gates first-time responses.
- **Default-actor fallback:** unless `no_default_actor` is enabled (letta/services/user_manager.py:123-124), requests without a `user_id` header execute under a shared default identity, collapsing all attribution.

## Future Considerations

- Add an append-only `audit_events` table (or stream) capturing security-relevant state transitions: approval-policy changes, auth failures, run cancellations, sandbox escalations — each with actor, timestamp, and correlation IDs already available in context (letta/server/rest_api/middleware/logging.py:61-111).
- Log authentication failures and rate-limit signals in `CheckPasswordMiddleware` (letta/server/rest_api/middleware/check_password.py:10-31) and support per-user API keys in the OSS tree rather than deferring to undefined cloud methods.
- Persist a decision record when `requires_approval` policy changes: `modify_approvals_async` already receives `actor` (letta/services/agent_manager.py:3064) but discards it after access control; writing a `block_history`-style attributed entry would close the policy-provenance gap cheaply.
- Redact or hash request-body values before attaching them to OTel span attributes (letta/otel/tracing.py:114-124).
- Make `no_default_actor` fail-closed by default for multi-tenant deployments, or emit a warning-level event when falling back to the default actor (letta/services/user_manager.py:126-135).
- Preserve redo-chain branches on checkpoint undo (soft-delete/branch pointers instead of hard delete, letta/services/block_manager.py:874-883).

## Questions / Gaps

- **No dedicated security-event or audit-event subsystem found.** Searches for `audit`, `security_event`, `SecurityEvent`, `policy_decision`, `PolicyDecision` across `letta/` produced only incidental docstring hits (letta/services/llm_trace_reader.py:3,87; letta/services/block_manager.py:805). If such a system exists, it lives outside this repository (likely the Letta cloud control plane).
- **Where are `api_key_to_user` and `authenticate_user` implemented?** Not found anywhere under `letta/` despite being invoked on the `SyncServer` instance; presumed defined in a proprietary subclass/fork. Consequence: OSS-only auditability of key-to-identity mapping could not be assessed.
- **Tamper evidence:** no evidence of hash chaining, WORM storage, or signature over messages/history was found; searched the ORM and `sqlalchemy_base.py`. Absence confirmed within search boundary (ORM directory + managers), though DB-level controls (e.g., Postgres permissions) are out of scope for this source.
- **Retention policy for traces/messages:** no TTL or purge job for `provider_traces`/`messages` was located in this tree; PRIVACY.md addresses vendor telemetry collection, not operator-side audit retention (PRIVACY.md:1-14).

---

Generated by `08.04-security-auditability` against `letta`.
