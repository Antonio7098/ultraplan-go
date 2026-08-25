# Source Analysis: openai-agents-sdk

## 08.04 Security Auditability

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (Agents SDK library; httpx, pydantic; MkDocs docs; pytest suite) |
| Analyzed | 2026-08-24 |

## Summary

The OpenAI Agents SDK approaches auditability through four complementary mechanisms rather than one dedicated "security event log":

1. **A structured tracing system** (`src/agents/tracing/`) that records typed spans (agent, generation, function/tool call, guardrail, handoff, MCP) with ISO timestamps, trace/span/parent IDs, and pluggable export processors (`src/agents/tracing/processors.py:44`, `src/agents/tracing/span_data.py:11`).
2. **An approval (human-in-the-loop) state machine** on `RunContextWrapper` where every approve/reject decision is recorded per tool identity or call ID (`src/agents/run_context.py:56-68`), is durable across pause/resume via versioned `RunState` snapshots (`src/agents/run_state.py:182-217`), and surfaces pending decisions as `ToolApprovalItem` interruptions (`src/agents/items.py:556-583`).
3. **Guardrail decision records**: input/output and tool guardrails return structured results carrying `output_info` and `tripwire_triggered`, and each evaluation runs inside a dedicated `guardrail_span` whose data records the triggered flag (`src/agents/guardrail.py:19-68`, `src/agents/run_internal/guardrails.py:31-52`).
4. **A purpose-built sandbox audit-event subsystem**: start/finish events with UUID event IDs, UTC timestamps, session-scoped sequence numbers, span/trace correlation, error codes, and configurable redaction policies delivered to pluggable sinks including durable JSONL sinks (`src/agents/sandbox/session/events.py:32-89`, `src/agents/sandbox/session/manager.py:16-50`, `src/agents/sandbox/session/sinks.py:97-342`).

Sensitive-data hygiene is explicit and layered: model/tool payload logging is off by default via `OPENAI_AGENTS_DONT_LOG_MODEL_DATA` / `OPENAI_AGENTS_DONT_LOG_TOOL_DATA` (both default to "don't log", `src/agents/_debug.py:12-28`), trace payloads honor `RunConfig.trace_include_sensitive_data` (`src/agents/run_config.py:404-410`), and tracing API keys are persisted only as SHA-256 fingerprints in serialized state (`src/agents/tracing/traces.py:187-192`).

The main gaps for a strict audit posture: approval records capture the decision but **not the actor** (no user identity, no decision timestamp inside `_ApprovalRecord`), there are no discrete policy-decision IDs, core-run security events ride on generic tracing/logging instead of an append-only audit log, and the default batch exporter can silently drop queued spans under load (`src/agents/tracing/processors.py:604`, `src/agents/tracing/processors.py:621`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The approval trail is well-modeled and durable: decisions, rejection messages, and sticky scopes survive `RunState.to_string()/to_json()` round trips (`src/agents/run_state.py:1300-1324`; documented at `docs/human_in_the_loop.md:53`), with an explicit schema-version changelog tracking audit-relevant changes ("1.6 Persists explicit approval rejection messages across resume flows", "1.14 Scopes hosted MCP approvals... by server label", "1.16 ... exact call approval decision override a sticky decision" — `src/agents/run_state.py:193-216`).
- Test coverage of the audit-critical paths is unusually deep: ~26k lines across `tests/test_run_context_approvals.py` (742), `tests/test_hitl_error_scenarios.py` (3800), `tests/test_tool_approval_call_id_reuse.py` (3231), `tests/test_error_logging_redaction.py` (2893), `tests/test_run_state.py` (12110), plus sandbox instrumentation tests (`tests/sandbox/test_session_manager.py`, `tests/sandbox/test_session_sinks.py`).
- It falls short of 8-9 because attribution ("who approved?") is delegated entirely to application context, policy evaluations produce no standalone decision records with IDs, and the default trace pipeline is lossy under pressure (queue drops, retry give-ups) — acceptable for observability but weak as a compliance-grade audit log.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Security/diagnostic logging channel | Named logger `openai.agents`; all SDK diagnostic logs funnel through helpers that enforce redaction policy | `src/agents/logger.py:7`, `src/agents/logger.py:31-74` |
| Log redaction defaults | `DONT_LOG_MODEL_DATA` / `DONT_LOG_TOOL_DATA` default True (payloads suppressed) via `OPENAI_AGENTS_DONT_LOG_MODEL_DATA` / `OPENAI_AGENTS_DONT_LOG_TOOL_DATA` | `src/agents/_debug.py:12-28` |
| Trace model | Traces carry `trace_id`, `workflow_name`, `group_id`, `metadata`; exported dict shape fixed | `src/agents/tracing/traces.py:568-575` |
| Span timestamps + errors | Spans record `started_at`/`ended_at` ISO times and a `SpanError` (message + data) | `src/agents/tracing/spans.py:396-405`, `src/agents/tracing/spans.py:19-26` |
| Tool-call spans | Function tool execution wrapped in `function_span`; input args recorded at `span_fn.span_data.input = tool_call.arguments` only when `trace_include_sensitive_data`; output recorded likewise | `src/agents/run_internal/tool_execution.py:1805-1856` |
| Guardrail spans | Each guardrail runs inside `guardrail_span(name)`; `span_data.triggered` set from result | `src/agents/run_internal/guardrails.py:31-52` |
| Tripwire evidence | Tripwires attach `SpanError("Guardrail tripwire triggered", {guardrail name, type})` to parent/current span | `src/agents/run_internal/guardrails.py:85-96`, `151-157`, `207-213` |
| Guardrail result records | `InputGuardrailResult` / `OutputGuardrailResult` expose guardrail identity, `output_info`, `tripwire_triggered`; results accumulate on streaming results | `src/agents/guardrail.py:35-68`, `src/agents/run_internal/guardrails.py:76-81` |
| Tool guardrail behaviors | `ToolGuardrailFunctionOutput` carries `output_info` plus allow / reject_content(message) / raise_exception behavior | `src/agents/tool_guardrails.py:59-117` |
| Approval records | `_ApprovalRecord`: approved/rejected as bool or per-call-ID lists, `rejection_messages`, `sticky_rejection_message`, `sticky_scope` | `src/agents/run_context.py:56-68` |
| Decision API | `approve_tool()` / `reject_tool()` (with `always_*` stickiness and `rejection_message`) write into those records after validating canonical invocation identity | `src/agents/run_context.py:1043-1063`, `src/agents/run_context.py:888-1041` |
| Public HITL surface | `RunState.approve/reject/get_interruptions`; pending approvals exposed as `ToolApprovalItem` list on run results | `src/agents/run_state.py:985`, `src/agents/run_state.py:1255-1298`, `src/agents/result.py:515-516` |
| Approval item identity | `ToolApprovalItem` binds agent, raw tool call, tool name/namespace, origin, canonical lookup key | `src/agents/items.py:556-614` |
| Invocation ledger | `_ToolInvocationRecord` per provider call ID tracks type/approval scope/fingerprint/executed/completed; reused call ID with different fingerprint raises `ModelBehaviorError` | `src/agents/run_context.py:46-53`, `src/agents/run_context.py:304-355` |
| Durable approval serialization | `_serialize_approvals` / `_serialize_tool_invocations` persist decisions, rejection messages, sticky scopes, and invocation lifecycle into RunState snapshots | `src/agents/run_state.py:1300-1339` |
| Schema changelog for audit fields | Versioned schema summaries document persistence of rejections (1.6), scoped hosted-MCP approvals (1.14), exact-call override (1.16) | `src/agents/run_state.py:186-217` |
| Hosted-MCP scoping | Sticky approvals keyed by `(server_label, tool_name)` so identical tool names on different servers do not share authorization | `src/agents/run_context.py:446-452`; test `tests/test_run_context_approvals.py:30` |
| Fail-closed policy evaluation | Callable `needs_approval` not invoked on unparseable arguments → requires manual approval | `src/agents/run_internal/tool_execution.py:1300-1318`; `docs/human_in_the_loop.md:15` |
| Sandbox audit events | `SandboxSessionEventBase`: uuid `event_id`, UTC `ts`, `session_id`, monotonic `seq`, `op`, start/finish `phase`, `span_id`/`parent_span_id`/`trace_id` correlation, finish carries `ok`, `duration_ms`, error code/type/message | `src/agents/sandbox/session/events.py:32-89` |
| Audited operations | Op names include exec, read, write, apply_patch, materialize, snapshot_persist/restore, persist/hydrate workspace | `src/agents/sandbox/errors.py:58-73` |
| Audit sink delivery | `Instrumentation` delivers events to `EventSink`s with per-sink/per-op `EventPayloadPolicy`; sync/async/best-effort modes with raise/log/ignore error policies | `src/agents/sandbox/session/manager.py:16-50`, `101-138` |
| Durable audit sinks | `JsonlOutboxSink`, `WorkspaceJsonlSink`, `HttpProxySink`, `CallbackSink`, `ChainedSink` implementations | `src/agents/sandbox/session/sinks.py:61-342` |
| Sandbox audit privacy defaults | `EventPayloadPolicy`: exec stdout/stderr excluded by default, bounded when enabled; write events carry byte count only, never file bytes | `src/agents/sandbox/session/events.py:18-29` |
| Usage ledger | Per-request token usage preserved in `request_usage_entries` for cost/behavior reconstruction | `src/agents/usage.py:218-229`, `295-312` |
| Lifecycle observation hooks | `on_tool_start/on_tool_end`, `on_llm_start/on_llm_end`, `on_handoff` hooks receive tool + result for external audit integration | `src/agents/lifecycle.py:70-103` |
| Trace metadata for attribution | `RunConfig.trace_id`, `group_id` ("e.g., a chat thread ID"), `trace_metadata` let apps attach actor/conversation identifiers | `src/agents/run_config.py:417-429` |
| Secret hygiene in state | Only SHA-256 hash of tracing API key persisted by default; raw key opt-in via `include_tracing_api_key=True` | `src/agents/tracing/traces.py:171-192`, `docs/human_in_the_loop.md:197` |
| Sensitive-data trace control | `RunConfig.trace_include_sensitive_data` (default from env, default true); threaded through tool/model/error paths | `src/agents/run_config.py:53-56`, `404-410`; `src/agents/util/_error_tracing.py:48-77` |
| Console export redaction | `ConsoleSpanExporter` prints "Span data is redacted." unless data logging enabled | `src/agents/tracing/processors.py:30-41` |
| Backend exporter failure handling | Export retries with backoff then "[non-fatal] max retries reached, giving up on this batch"; client errors logged with response text only when data logging allowed | `src/agents/tracing/processors.py:158-221` |
| Lossy queue | `BatchTraceProcessor` drops traces/spans with a warning when its 8192-item queue fills | `src/agents/tracing/processors.py:597-621` |
| HITL documentation | Pause → serialize → decide → resume flow, sticky decisions surviving serialization, custom rejection messages | `docs/human_in_the_loop.md:45-66`, `105-203` |

## Answers to Dimension Questions

**1. Who did what?**
Partially answerable. *What* was done is well captured: each tool invocation is bound to a provider call ID, canonical fingerprint, agent, and tool name (`src/agents/run_context.py:46-53`; `src/agents/items.py:556-614`), and function spans record arguments/results (`src/agents/run_internal/tool_execution.py:1818-1819`, `1854-1855`). Sandbox operations additionally get session ID, sequence number, and trace correlation (`src/agents/sandbox/session/events.py:37-50`). *Who* is weakly captured: neither `_ApprovalRecord` nor any trace span carries an actor/user identity field. Attribution depends on applications supplying `RunConfig.group_id` / `trace_metadata` (`src/agents/run_config.py:420-429`) or their own context object — the SDK explicitly warns that app context travels in serialized state (`docs/human_in_the_loop.md:199`). No evidence found of built-in principal identity propagation.

**2. What policy allowed it?**
Reconstructable within the process. Approval status is resolved deterministically from stored records via `get_approval_status` with exact-call decisions overriding sticky ones (`src/agents/run_context.py:628-661`), and sticky approvals are scoped to a stable approval scope/fingerprint so a decision cannot silently authorize a different invocation (`src/agents/_tool_invocation.py:155-219`). Hosted-MCP stickiness is scoped by server_label+tool name (`src/agents/run_context.py:446-472`). However, the *rule evaluation itself* (which callable said `needs_approval=True`, which guardrail expression matched) is not written anywhere as a discrete record — no policy-decision IDs exist anywhere in the source (searched for `audit`, `decision id` concepts; only guardrail results and approval records approximate this). Guardrail outcomes are the closest artifact: named spans with `triggered` flags and `SpanError` attachments (`src/agents/run_internal/guardrails.py:85-96`).

**3. Was a human involved?**
Yes, structurally. Tools declaring `needs_approval` pause the run; pending calls surface as `ToolApprovalItem.interruptions` (`src/agents/result.py:515-516`; flow documented at `docs/human_in_the_loop.md:45-57`), execution cannot proceed until `state.approve(...)` / `state.reject(...)` is invoked (`src/agents/run_state.py:1255-1298`), and programmatic callbacks (`on_approval`, hosted MCP `on_approval_request`) are the explicit non-human alternative (`docs/human_in_the_loop.md:89-97`). The interruption itself is preserved as a run item and in serialized state, so an auditor can see that a pause occurred. But the record does not distinguish a human approver from an automated script — that distinction lives in application code.

**4. Can auditors reconstruct the decision?**
Largely yes for a single run's lifecycle: RunState snapshots serialize the full approval ledger, invocation ledger, rejection messages, and pending items (`src/agents/run_state.py:1300-1339`) with a fail-forward versioned schema and changelog (`src/agents/run_state.py:175-232`); traces add the timeline (spans with timestamps and errors); sandbox sessions emit ordered, correlated start/finish audit events suitable for JSONL retention (`src/agents/sandbox/session/sinks.py:97-175`). Reconstruction degrades in three ways: (a) the default trace exporter can drop batches on queue overflow or repeated export failure (`src/agents/tracing/processors.py:604`, `621`, `211-215`); (b) tracing can be disabled globally by env var (`OPENAI_AGENTS_DISABLE_TRACING`, `src/agents/tracing/provider.py:348`) and is unavailable under Zero Data Retention org policies (`docs/tracing.md:13`); (c) without app-supplied identity metadata, the reconstructed trail shows *that* a decision was made but not *by whom*.

## Architectural Decisions

- **Tracing-first observability with pluggable processors.** The global `TraceProvider` + `TracingProcessor` interface (`src/agents/tracing/setup.py:27-66`, `src/agents/tracing/processor_interface.py`) makes audit destination a deployment concern: default exports to the OpenAI traces ingest endpoint (`https://api.openai.com/v1/traces/ingest`, `src/agents/tracing/processors.py:45`) while `set_trace_processors()` allows replacing with SIEM/local sinks (`docs/tracing.md:148-158`).
- **Approvals as first-class run state, not side-channel logs.** Decisions mutate `_ApprovalRecord` structures owned by the run context (`src/agents/run_context.py:888-1063`) and are checkpointed through `RunState` with strict schema versioning rather than appended to a log file — enabling resumable HITL across processes (`docs/human_in_the_loop.md:187-199`).
- **Fail-closed approval rules.** When tool arguments cannot be parsed safely, callable approval policies are bypassed and manual approval is forced (`src/agents/run_internal/tool_execution.py:1306-1311`; `docs/human_in_the_loop.md:15`).
- **Canonical invocation identity.** A semantic fingerprint (normalized arguments, scope) per provider call ID blocks call-ID reuse attacks where a stale approval could be replayed against a different payload (`src/agents/_tool_invocation.py:199-219`; enforcement raising `ModelBehaviorError` at `src/agents/run_context.py:344-352`).
- **Redact-by-default diagnostics, opt-in detail everywhere.** Logging suppresses model/tool payloads by default (`src/agents/_debug.py:20-28`), console export prints placeholders (`src/agents/tracing/processors.py:30-41`), sandbox audit events exclude exec output by default (`src/agents/sandbox/session/events.py:20-29`), and secrets become hashes in persisted state (`src/agents/tracing/traces.py:187-192`). Notably asymmetric: `trace_include_sensitive_data` still defaults to true (`src/agents/run_config.py:55`).
- **Dedicated audit subsystem only where risk concentrates.** The sandbox layer — where arbitrary command execution happens — gets a real event-sourcing-style pipeline (uuid events, seq numbers, durable sinks), while the general run loop relies on tracing.

## Notable Patterns

- **Correlation IDs stitched across subsystems:** sandbox audit events copy `span_id`/`trace_id` from the active SDK span so op-level audit records join to distributed traces (`src/agents/sandbox/session/sandbox_session.py:163-175`, `304-323`).
- **Per-sink payload policies:** one event can fan out to sinks with different redaction levels because `_apply_policy` clones the event per sink (`src/agents/sandbox/session/manager.py:76-99`).
- **Sticky-vs-exact decision semantics:** permanent decisions are scoped by `sticky_scope` and exact call-ID lists override them, tested extensively including cross-server aliasing attempts (`src/agents/run_context.py:649-661`; `tests/test_run_context_approvals.py:157-243`).
- **Diagnostic-context pattern in logging:** failures log a safe static message plus lazily-evaluated `diagnostic_extra` metadata (e.g., tool name only, never arguments) (`src/agents/logger.py:9-11`, `22-28`; used at `src/agents/run_internal/tool_execution.py:1276-1281`).
- **Error redaction at exception-construction time:** helpers like `_prepare_data_redacted_error` mark exceptions so tracebacks/logs never carry sensitive payloads (imported at `src/agents/run_state.py:61-67`).

## Tradeoffs

- **Richness vs. privacy:** full-fidelity traces (arguments, outputs) are the best audit artifact but are on by default (`trace_include_sensitive_data=true`), trading confidentiality of traced data for reconstructability; turning it off keeps structure but loses evidence content (`src/agents/run_config.py:404-410`; `docs/tracing.md:138-146`).
- **Durability vs. overhead:** batching with a background thread keeps tracing cheap, but means audit data lives in an in-memory queue until flushed and can be lost at shutdown timeout or queue overflow (`src/agents/tracing/processors.py:541-546`, `623-645`); apps needing delivery guarantees must call `flush_traces()` themselves (`docs/tracing.md:61-102`).
- **Vendor-coupled default backend:** the default audit destination is OpenAI's cloud dashboard, unavailable for ZDR organizations — such deployments must supply custom processors or lose tracing entirely (`docs/tracing.md:13`, `148-158`).
- **Simplicity of approval records vs. forensic completeness:** booleans + call-ID lists + messages are compact and serializable, but omit timestamp/actor/justification, pushing richer forensics onto app-level storage.
- **Sandbox-only deep auditing:** the strongest audit guarantees (ordered events, durable sinks) exist solely for sandbox sessions; shell/exec via other tool families get only spans/logs.

## Failure Modes / Edge Cases

- **Silent-ish audit loss:** `BatchTraceProcessor` drops items when the 8192-slot queue is full, logging only `"Queue is full, dropping span."` (`src/agents/tracing/processors.py:618-621`); exporters give up after 3 retries with a `[non-fatal]` message (`src/agents/tracing/processors.py:211-215`). An auditor reviewing a loaded production system may see gaps.
- **Missing call IDs block decisions safely:** per-call approvals with empty call IDs raise rather than guess (`src/agents/run_context.py:927-941`); malformed hosted-MCP request IDs are rejected without mutating approval state (`tests/test_run_context_approvals.py:427-495`).
- **Call-ID reuse detection:** replaying an executed call ID with a different fingerprint raises `ModelBehaviorError`, preventing approval replay across changed payloads (`src/agents/run_context.py:327-352`).
- **Resume-time binding reconstruction:** legacy serialized states may lack bindings; guarded reconstruction paths (`_restore_pending_approval_binding`, gated by `_allow_legacy_approval_binding_reconstruction`) rebuild them conservatively, and unmatched restored calls are tracked in `_restored_unbound_approval_call_ids` rather than implicitly authorized (`src/agents/run_context.py:97-115`, `509-568`).
- **Identity collisions between nested runs:** applying an approval whose canonical identity belongs to both outer and nested runs raises `UserError` instead of double-authorizing (`src/agents/run_state.py:1237-1252`; `src/agents/tool_context.py:178-192`).
- **Sink failure policy divergence:** audit delivery can raise, log-and-continue, or silently ignore depending on sink configuration (`on_error` = raise/log/ignore, `src/agents/sandbox/session/sinks.py:20-21`); `ignore` mode can lose events without operator awareness beyond construction-time choice.
- **No approval-decision log emission:** grep found no `logger.*approv*` statements in `src/` — a rejected/approved call leaves no standard-log footprint, so log-based alerting on approvals requires hook or processor implementation.

## Future Considerations

- Add optional actor/timestamp/rationale fields to `_ApprovalRecord` (or a parallel decision-record) so serialized states satisfy who/when/why audits without app-side bookkeeping; the schema-version mechanism (`src/agents/run_state.py:186-217`) already provides a migration path.
- Emit a structured decision event (via `custom_span` or a new span type) whenever `_apply_approval_decision` commits, giving policy-decision records parity with guardrail spans.
- Offer a file/stream-backed `TracingProcessor` alongside the backend exporter for deployments that need durable local audit trails (mirroring what `JsonlOutboxSink` does for sandboxes).
- Document and warn loudly when `trace_include_sensitive_data=true` coexists with compliance-sensitive deployments, given the asymmetry with the log-redaction defaults.
- Surface dropped-span counters (queue-full events, failed batches) on the provider/processor API so monitoring can detect audit gaps programmatically.

## Questions / Gaps

- **No policy-decision identifiers.** Searched `src/` for audit/decision-ID constructs (`grep -i "audit|decision"`): only the sandbox event system uses `event_id`. Guardrail and approval outcomes have no queryable ID linking "the rule that fired" to "the action taken". What did we search: `audit|Audit` across `src/**/*.py` (9 hits, all sandbox-related), logger statements matching approvals (none found).
- **No actor identity anywhere in the SDK core.** No user/principal concept exists in `RunContextWrapper`, `_ApprovalRecord`, spans, or sandbox events. Confirmed by reading `src/agents/run_context.py:56-115` and `src/agents/sandbox/session/events.py:32-89`.
- **Approval decision timing.** Decision records contain no timestamps; reconstruction of *when* a human decided relies on surrounding trace spans or application storage. No evidence found of wall-clock capture at `approve_tool`/`reject_tool`.
- **Cross-run aggregation story.** Session persistence stores conversation history (`docs/human_in_the_loop.md:99-103`), but whether approval history aggregates across many runs into one auditable ledger depends entirely on external infrastructure; the repo provides examples but no built-in store.
- **Realtime/voice audit depth not fully assessed.** Realtime sessions expose `approve_tool_call`/`reject_tool_call` (`src/agents/realtime/session.py:969`, `1015`), and voice has separate tracing controls (`docs/voice/tracing.md`), but their persistence characteristics were outside the boundary examined here.

---

Generated by `08.04-security-auditability` against `openai-agents-sdk`.
