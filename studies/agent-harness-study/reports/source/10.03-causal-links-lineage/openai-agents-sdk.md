# Source Analysis: openai-agents-sdk

## Dimension 10.03: Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk (OpenAI Agents SDK, Python) |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, pydantic, openai SDK, httpx; tracing exporters via `httpx2` |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root `studies/agent-harness-study/sources/openai-agents-sdk/`.

## Summary

The SDK's causal-lineage story is built on three coordinated mechanisms. First, a hierarchical tracing system (`src/agents/tracing/`) records every run as a `Trace` containing nested `Span`s with explicit `trace_id`/`span_id`/`parent_id` links, typed span payloads for generations, tool calls, handoffs, guardrails, and audio operations. Second, the run-item model (`src/agents/items.py`) keeps every output attached to the agent that produced it and ties tool outputs to tool calls through provider `call_id`s, reinforced by a canonical invocation-identity layer (`src/agents/_tool_invocation.py`) that fingerprints invocations semantically so approvals and outputs cannot be silently rebound. Third, approvals are first-class lineage objects: a decision is recorded against a specific call ID or tool identity, serialized into resumable `RunState`, and replayed as synthetic tool outputs back to the model. Sandbox operations emit audit events carrying the originating trace/span IDs, giving artifact-level correlation. The chain "final answer → message item → agent → turn → model response ID → generation span (input, output, model name)" is reconstructible from `RunResult.new_items` plus exported traces. Content-level provenance is gated behind `trace_include_sensitive_data`, and durable trace storage depends entirely on the configured processor backend.

## Rating

**8 / 10** — Clear lineage model with explicit interfaces, extensive tests (trace reattachment on resume, call-ID scoping of approvals, span ordering), and operational safeguards (fingerprinted invocation identity, redaction-aware exports, trace-state persistence). Not higher because: content payloads are conditionally stripped by sensitive-data policy, `ResponseSpanData.export()` drops the stored input (`src/agents/tracing/span_data.py:236-241`), there is no default durable local trace store, approval records carry no actor/timestamp, and the SQLite session schema does not persist per-run/per-turn attribution.

## Evidence Collected

Every entry cites files relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trace identity | `Trace.trace_id` documented as globally unique, used to link spans to parent trace and dashboard lookup | src/agents/tracing/traces.py:121-134 |
| Trace export shape | Export payload `{object: "trace", id, workflow_name, group_id, metadata}` | src/agents/tracing/traces.py:568-575 |
| Span hierarchy | `SpanImpl` stores `_trace_id`, `_span_id`, `_parent_id`; export includes all three plus timestamps and error | src/agents/tracing/spans.py:289-324, 396-423 |
| Span auto-nesting | Spans attach to current trace/span via contextvars; `get_current_trace` / `get_current_span` accessors | src/agents/tracing/create.py:79-87 |
| Run-level spans | `task_span` per Runner invocation; opt-in via `include_task_and_turn_spans` | src/agents/run_internal/run_loop.py:905-910; src/agents/tracing/config.py (include_task_and_turn_spans) |
| Turn-level spans | `turn_span(turn=…, agent_name=…)` started/finished per loop iteration with usage metadata | src/agents/run_internal/run_loop.py:1737-1784; src/agents/tracing/span_data.py:98-132 |
| Agent spans | `agent_span(name, handoffs, tools, output_type)` populated from live agent config each turn | src/agents/run_internal/run_loop.py:1528, 2104-2110 |
| Model output↔input link | `GenerationSpanData` carries input sequence, output sequence, model, model_config, usage | src/agents/tracing/span_data.py:169-209 |
| Generation span wiring | Chat Completions adapter opens `generation_span(model=str(self.model), model_config=model_config_for_trace(...))` and records output when data tracing enabled | src/agents/models/openai_chatcompletions.py:237-260, 303-313, 345-348 |
| Response ID lineage | Responses adapter wraps calls in `response_span`; stores full response + input when `tracing.include_data()`, usage always | src/agents/models/openai_responses.py:573-606, 659-689 |
| ResponseSpanData export | Export emits only `response_id` + usage; raw `input` retained as in-memory attribute for custom processors | src/agents/tracing/span_data.py:227-241 |
| Sensitive-data gate | `ModelTracing.ENABLED_WITHOUT_DATA` returned unless `trace_include_sensitive_data` set | src/agents/tracing/model_tracing.py:6-14 |
| ModelResponse IDs | `ModelResponse` carries `response_id` and transport `request_id` per model call | src/agents/items.py:705-730 |
| Raw responses retained | `RunResultBase.raw_responses: list[ModelResponse]`; `last_response_id` accessor | src/agents/result.py:319-320, 472-478 |
| Server-managed continuity | `conversation_id`, `previous_response_id`, `auto_previous_response_id` tracked on results/state across turns | src/agents/result.py:503-508, 677-682 |
| Item→agent attribution | Every `RunItemBase` has an `agent` field ("the agent whose run caused this item") | src/agents/items.py:97-106 |
| Handoff lineage | `HandoffOutputItem` retains `source_agent` and `target_agent` weakrefs | src/agents/items.py:310-339 |
| Tool call ↔ output link | `ToolCallItem.call_id` and `ToolCallOutputItem.call_id` read from raw items; outputs created with `"call_id": tool_call.call_id` | src/agents/items.py:412-417, 454-460, 918-923 |
| Call-ID reuse guard | Reusing one provider call ID for a different invocation raises `ModelBehaviorError` | src/agents/run_context.py:327-332 |
| Invocation fingerprints | Canonical identity = (type, call_id, approval_scope, SHA-256 semantic fingerprint over arguments/action fields) | src/agents/_tool_invocation.py:155-219, 282-289 |
| Output→invocation mapping | `_TOOL_OUTPUT_TYPES` maps each output type to its invocation type; `tool_output_identity()` resolves completed call | src/agents/_tool_invocation.py:27-35, 298-316 |
| Tool execution spans | `with_tool_function_span` wraps tool callbacks in `function_span(tool_name)`; skipped when tracing disabled/no active trace | src/agents/run_internal/tool_execution.py:1120-1139 |
| MCP provenance | MCP tool results record `mcp_data = {"server": ...}` on the enclosing function span | src/agents/mcp/util.py:814-823; payload field at src/agents/tracing/span_data.py:135-166 |
| MCP tool listing spans | `MCPListToolsSpanData(server, result)` records which server exposed which tools | src/agents/tracing/create.py:466-491; src/agents/tracing/span_data.py:427-451 |
| Tool-use ledger | `AgentToolUseTracker` records agent→used-tool names; serializable/hydratable for resume | src/agents/run_internal/tool_use_tracker.py:53-125, 128-166 |
| Approval items | `ToolApprovalItem` binds raw pending call + tool_name + namespace + origin + lookup key; exposes `call_id` and `arguments` | src/agents/items.py:556-583, 644-676 |
| Approval scoping | `_ApprovalRecord.approved/rejected` are bool (permanent) or lists of call IDs (per-call) with rejection messages | src/agents/run_context.py:56-68 |
| Decision application | `approve_tool` / `reject_tool` apply decisions under canonical keys incl. namespaced and hosted-MCP identities | src/agents/run_context.py:1043-1063, 167-212 |
| Approval persistence | `RunState.approve/reject` delegate to nested states; decisions serialized into state JSON (`approvals`, `hosted_mcp_approvals`) | src/agents/run_state.py:1255-1298, 1300-1324, 1341-1359, 1830 |
| Invocation ledger serialization | `_serialize_tool_invocations` persists type/approval_scope/fingerprint/executed/completed per call ID | src/agents/run_state.py:1326-1339 |
| Rejection feedback loop | Rejections append synthetic `ToolCallOutputItem`s so the model sees why execution was refused | src/agents/run_internal/approvals.py:24-43 |
| Resume-safe traces | `TraceState` serializes trace_id/workflow/group/metadata (+ hashed API key) into RunState; `ReattachedTrace` resumes without duplicate start events | src/agents/tracing/traces.py:195-277, 305-404 |
| Trace reattach validation | `create_trace_for_run` reattaches only when persisted settings match exactly; key verified by SHA-256 fingerprint | src/agents/tracing/context.py:20-88 |
| Harness routing metadata | `agent_harness_id` metadata key propagated to span exports; sourced from registration config or env | src/agents/tracing/spans.py:16, 407-423; src/agents/models/openai_agent_registration.py:9-10, 74-87 |
| Grouping hierarchy | Run grouping resolves conversation > session > group_id > generated per-run id (also used for prompt-cache keys) | src/agents/run_internal/run_grouping.py:12-48 |
| Sandbox audit linkage | Audit events carry session_id, event_id, seq, op, phase, and span_id/parent_span_id/trace_id of the causing SDK span | src/agents/sandbox/session/events.py:32-53 |
| Sandbox span capture | `_audit_trace_ids` extracts live span/parent/trace ids; random fallback id when no active trace | src/agents/sandbox/session/sandbox_session.py:167-175, 397-409 |
| Snapshot identity | Snapshots persisted under snapshot id derived from sandbox session id | src/agents/sandboxes/docker.py:1547-1548; src/agents/sandbox/snapshot.py:91-108 |
| Session persistence | SQLite messages table keyed by session_id FK with auto-increment ordering; raw item JSON stored verbatim | src/agents/memory/sqlite_session.py:235-277 |
| Usage lineage | Per-request `RequestUsage` entries preserved in `Usage.request_usage_entries` for per-call cost accounting | src/agents/usage.py:150-167, 218-229, 295-312 |
| Replay sanitization | `to_input_item()` strips output-only metadata (`created_by`, `status`) while preserving call IDs — provenance kept through transformations | src/agents/items.py:222-256, 462-489 |
| Trace export backend | `BackendSpanExporter` posts grouped batches to OpenAI traces ingest endpoint; console exporter alternative | src/agents/tracing/processors.py:27-54, 118-150 |
| Tests: trace topology | `test_single_run_is_single_trace` snapshots exact span tree; resumed runs reuse original trace without duplicate starts | tests/test_agent_tracing.py:43-70, 400-490 |
| Tests: response spans | `test_get_response_creates_trace` verifies span data captured from provider responses | tests/test_responses_tracing.py:59-230 |
| Tests: approval scoping | Latest decision wins per call ID; rejections scoped to call IDs | tests/test_run_context_approvals.py:516; tests/test_run_state.py:1310 |
| Tests: call-id helpers | Tool call/output `call_id` extraction covered including dict fallbacks and missing-ID cases | tests/test_items_helpers.py:862-914 |
| Docs: stated design | Tracing doc enumerates trace/span properties (`trace_id`, `parent_id`, `group_id`, typed `span_data`) and default span coverage | docs/tracing.md:11-42 |

## Answers to Dimension Questions

1. **Can every output be traced to its inputs?**
   Largely yes, structurally always. Every final answer decomposes into `new_items` where each item names its producing agent (`src/agents/items.py:97-106`) and sits inside a span tree rooted at a unique trace (`src/agents/tracing/spans.py:396-423`). Message items derive from `ModelResponse.output` whose `response_id` links to the exact provider call (`src/agents/items.py:705-730`). The *content* of the prompt that produced a generation is recorded only under `trace_include_sensitive_data=True` (`src/agents/tracing/model_tracing.py:6-14`); otherwise lineage remains structural (ids, timing, model, usage) but not textual. Facts supplied by tools are traceable: each fact-bearing tool output shares a `call_id` with its originating `ToolCallItem` (`src/agents/items.py:454-460`).

2. **Is provenance preserved through transformations?**
   Yes, deliberately. Output→input replay strips only server-assigned display metadata (`created_by`, `status`) while preserving `call_id` and content (`src/agents/items.py:222-256, 462-489`). Handoff filtering tracks divergence between session history and model input, exposing both `preserve_all` and `normalized` views (`src/agents/result.py:255-290, 431-453`). Compaction replaces history via explicit `CompactionItem`s rather than silent edits (`src/agents/items.py:532-540`). One caveat: nested-history ownership refs rebind by digest match, and non-matching refs are dropped rather than surfaced (`src/agents/result.py:93-105`).

3. **Are model versions tracked in lineage?**
   Partially. Generation spans record the model string and sanitized model configuration (including base URL) (`src/agents/models/openai_chatcompletions.py:238-242`; `src/agents/models/_trace.py:20-31`), and speech/transcription spans do likewise (`src/agents/tracing/span_data.py:316-400`). This identifies the model family/deployment string, not an immutable weight version or snapshot hash; providers that alias models would be indistinguishable.

4. **Can causal chains be audited?**
   Yes for in-memory and exported views: traces export as JSON with full parent linkage (`src/agents/tracing/spans.py:396-423`; `src/agents/tracing/traces.py:568-575`) to any `TracingProcessor`, including OpenAI's ingest API (`src/agents/tracing/processors.py:118-150`). Approvals are auditable as persisted decisions bound to call IDs or tool identities, surviving serialize/resume (`src/agents/run_state.py:1300-1359`), and sandbox side effects are auditable via events correlated to trace/span IDs (`src/agents/sandbox/session/events.py:46-50`). Gaps: no actor/timestamp on approval decisions, no default on-disk trace store, and session storage lacks run/turn columns (`src/agents/memory/sqlite_session.py:245-254`).

## Architectural Decisions

- **Tree-structured tracing with contextvar propagation.** Trace/span parenting is implicit via contextvars, so every operation nests correctly without threading parent handles through signatures (`src/agents/tracing/create.py:79-87`; docs/tracing.md "Creating spans"). This makes causality a runtime invariant instead of caller discipline.
- **Typed span payloads per operation kind.** `GenerationSpanData`, `FunctionSpanData`, `HandoffSpanData`, etc. give each causal edge a schema (`src/agents/tracing/span_data.py:28-265`), letting downstream auditors parse lineage without heuristics.
- **Provider `call_id` as the join key, hardened by semantic fingerprints.** The SDK treats provider call IDs as the canonical link between call, approval, and output, and adds a SHA-256 fingerprint over semantic fields so a reused or forged call ID cannot inherit another invocation's approval (`src/agents/_tool_invocation.py:155-219, 282-289`; enforcement at `src/agents/run_context.py:304-333`).
- **Approvals as durable run-state, not ephemeral flags.** Decisions serialize into `RunState` alongside the invocation ledger, so the approval→execution chain survives process restarts (`src/agents/run_state.py:1255-1298, 1326-1339`).
- **Trace identity is checkpointable.** Persisting `TraceState` and reattaching on resume means a resumed interrupted run continues the same causal chain instead of starting a new trace (`src/agents/tracing/traces.py:305-404`; `src/agents/tracing/context.py:47-88`; tested at `tests/test_agent_tracing.py:400-490`).
- **Redaction-aware lineage.** Lineage metadata (ids, types, usage) is emitted even when payloads are withheld, keeping auditability compatible with privacy modes (`src/agents/tracing/model_tracing.py:6-14`; exporter sanitization path at `src/agents/tracing/processors.py:136-143`).

## Notable Patterns

- **Weakref-based item attribution:** items keep agents reachable for inspection without creating GC cycles, with `release_agents()` to drop references eagerly (`src/agents/items.py:108-149`; `src/agents/result.py:384-407`).
- **Correlation IDs across subsystems:** the same `group_id` concept links traces (`docs/tracing.md:17-18`), prompt-cache grouping (`src/agents/run_internal/run_grouping.py:12-34`), and Realtime client/server trace correlation.
- **Audit-event mirror of tracing:** sandbox operations independently emit start/finish events with `seq` numbering and the SDK span coordinates, so side effects remain auditable even if trace export fails (`src/agents/sandbox/session/events.py:32-53`; `src/agents/sandbox/session/sandbox_session.py:397-442`).
- **Rejection-as-output pattern:** denied approvals become synthetic tool outputs with stable call IDs, preserving conversational causality for the model (`src/agents/run_internal/approvals.py:24-43`).
- **Snapshot-tested lineage topology:** tests assert exact normalized span trees, not just presence (`tests/test_agent_tracing.py:43-70`), pinning causal structure against regressions.

## Tradeoffs

- **Content completeness vs. privacy:** default-on sensitive-data capture maximizes auditability but ships prompts/outputs to the trace backend by default (`tests/test_run_config.py:315-356` shows env-controlled default); disabling it degrades lineage to skeleton-only.
- **Implicit parenting vs. explicit passing:** contextvar nesting removes boilerplate but breaks under abandoned async generators; the code compensates with careful `GeneratorExit` handling (`src/agents/tracing/spans.py:31-56`; `src/agents/tracing/traces.py:18-48`).
- **Provider-anchored identity vs. portability:** relying on provider `call_id`s gives exact wire-level joins but requires defensive fingerprinting and explicit errors when providers misbehave (`src/agents/run_context.py:327-332`).
- **Richness vs. cost of dual bookkeeping:** maintaining both session items and model-input views doubles state but enables faithful replay after handoff filters rewrite history (`src/agents/result.py:255-290`).

## Failure Modes / Edge Cases

- **No active trace ⇒ unlinked artifacts:** `_audit_trace_ids` substitutes a random span id and `None` trace id, so sandbox audit events produced outside tracing cannot be joined back to any run (`src/agents/sandbox/session/sandbox_session.py:172-175`).
- **Response spans lose input on export:** `ResponseSpanData.export()` emits `response_id`/usage only; the captured `input` lives solely on the in-memory object, so third-party processors must know to read it before export (`src/agents/tracing/span_data.py:227-241`).
- **Approval ambiguity errors:** identical invocation identities across nested runs raise rather than guess, requiring callers to use distinct call IDs (`src/agents/run_state.py:1241-1253`).
- **Digest-mismatch drops owned history refs:** rebound nested-history refs failing digest verification are silently filtered, potentially changing what `to_input_list()` replays (`src/agents/result.py:93-105`).
- **Trace-start cache bounded:** only the last 4096 started trace ids are remembered for reattachment; very long-lived processes may lose reattach eligibility (`src/agents/tracing/traces.py:280-302`; test at `tests/tracing/test_trace_context.py:330`).

## Future Considerations

- Record approver identity and decision timestamps on `_ApprovalRecord` to make human-in-the-loop chains fully auditable (`src/agents/run_context.py:56-68`).
- Include the stored `input` (or a hash/redacted form) in `ResponseSpanData.export()` so response-level provenance survives serialization (`src/agents/tracing/span_data.py:236-241`).
- Add optional run_id/turn_id columns (or embedded metadata) to session persistence to reconstruct per-turn lineage from history alone (`src/agents/memory/sqlite_session.py:245-254`).
- Consider emitting sandbox audit events even when tracing is disabled, keyed by a synthetic run id, to close the unlinked-artifact gap (`src/agents/sandbox/session/sandbox_session.py:172-175`).
- Document or version the model identifier semantics (e.g., pin deployment/version strings) if lineage consumers need reproducibility guarantees.

## Questions / Gaps

- No evidence found of a general-purpose artifact registry mapping arbitrary tool-produced files to run IDs; the closest mechanisms are sandbox audit events (`src/agents/sandbox/session/events.py:46-50`), session-scoped snapshots (`src/agents/sandbox/sandboxes/docker.py:1547-1548`), and provider file IDs forwarded in structured tool outputs (`src/agents/items.py:1019-1031`).
- No evidence found of lineage propagation into guardrail results beyond their position in the run result collections (`src/agents/result.py:325-335`); guardrail spans exist (`src/agents/tracing/create.py:325-350`) but guardrail verdicts are not linked to specific item IDs.
- Retrieval-context provenance depends on hosted-tool raw items being preserved; no SDK-side citation/annotation extraction layer was found (searched `items.py`, `run_internal/`, and `extensions/` for annotation/provenance handling).
- Whether the OpenAI traces backend retains `metadata` verbatim could not be verified from this repository alone; only the client-side export contract is visible (`src/agents/tracing/processors.py:136-150`).

---

Generated by dimension 10.03 (Causal Links and Lineage) against `openai-agents-sdk`.
