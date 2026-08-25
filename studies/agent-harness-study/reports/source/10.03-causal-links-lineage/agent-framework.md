# Source Analysis: agent-framework

## Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary, `python/packages/core`) and .NET (`dotnet/src/Microsoft.Agents.AI.*`); OpenTelemetry GenAI semantic conventions |
| Analyzed | 2026-08-25 |

## Summary

Microsoft Agent Framework builds lineage into its core data model rather than bolting it on in an observability layer. Every piece of content flowing through an agent run is a typed `Content` item (`python/packages/core/agent_framework/_types.py:474-589`), and the types that matter for causality carry explicit linkage keys: function results are bound to their originating call via a shared `call_id` (`_types.py:824-825`, `_types.py:872`), approval responses embed the entire approval-requested `function_call` content plus the request `id` (`_types.py:1293-1313`, `_types.py:1346-1362`), and responses carry provider-issued identifiers (`response_id`, `conversation_id`, `model`, `created_at`) plus the raw provider payload in `raw_representation` (`_types.py:2271-2332`). Retrieved context gets provenance twice: content-level citation `Annotation`s with URL/file-id/tool-name/snippet fields (`_types.py:387-398`) and message-level `_attribution` markers recording which context provider injected a message and, for cross-session memory, which origin sessions produced it (`_sessions.py:624-698`). The workflow engine propagates W3C trace contexts and publishing span IDs inside each inter-executor message and reconstructs causality as OTel span links at fan-in points (`_workflows/_runner_context.py:38-63`, `_workflow_context.py:340-346`, `observability.py:3223-3280`). Model versions are tracked through `gen_ai.request.model`/`gen_ai.response.model` span attributes with response-side backfill (`observability.py:290-291`, `observability.py:1538-1552`).

The main weaknesses: correlation keys like `call_id` are reused by providers over time, forcing occurrence-aware matching logic that is subtle and spec-mandated (`docs/specs/004-python-function-calling-loop.md:231-256`); workflow checkpoints deliberately do not record the workflow *run instance* that produced them (`_workflows/_checkpoint.py:37-41`); tool arguments/results appear in traces only when sensitive-data capture is explicitly enabled (`observability.py:927-934`); and trace-context link construction uses hand-parsed `traceparent` strings that silently degrade to link-less spans on malformed input (`observability.py:3249-3268`).

## Rating

**7/10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: The causal-link model is explicit and pervasive — call/result ID pairing, embedded-function-call approvals, per-message source attribution, W3C trace-context propagation with fan-in span links, and model-version attributes all have concrete implementations and targeted tests (e.g., forged-approval rejection at `tests/core/test_harness_tool_approval.py:708-742`; request-model backfill at `tests/core/test_observability.py:5423-5456`). It falls short of 9–10 because: (a) there is no framework-generated run identifier — `AgentResponse.response_id` is provider-supplied or None (`_types.py:2677-2690`); (b) checkpoint lineage is definition-scoped, not run-scoped (`_checkpoint.py:37-41`); (c) several provenance mechanisms (tool args/results on spans, attribution) are opt-in or silently lossy under malformed input; and (d) durable audit trails depend on the host choosing a session/history store.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Output→input linking (response IDs) | `ChatResponse` carries `response_id`, `conversation_id`, `model`, `created_at`, `usage_details`, `raw_representation` | python/packages/core/agent_framework/_types.py:2271-2332 |
| Output→input linking (message level) | `Message.message_id`, `author_name`, `raw_representation` fields | python/packages/core/agent_framework/_types.py:1802-1835 |
| Raw-provider provenance preserved through merges | `_combine_raw_representations` accumulates raw payloads into lists when combining contents/responses | python/packages/core/agent_framework/_types.py:1668-1679 |
| Service conversation continuity | Response `conversation_id` persisted into `session.service_session_id`; streaming variant extracts it from raw updates | python/packages/core/agent_framework/_agents.py:1194-1198, _agents.py:1254-1268 |
| Conversation ID on telemetry spans | `gen_ai.conversation.id` set from session on chat/agent spans | python/packages/core/agent_framework/observability.py:1646-1649, observability.py:3217-3219; _agents.py:541-551 |
| Tool result → call binding (types) | `from_function_call(call_id,...)` docstring: "Function results use the same ID"; `from_function_result(call_id,...)` mirrors it | python/packages/core/agent_framework/_types.py:809-852, _types.py:855-911 |
| Tool result → call binding (execution) | `_auto_invoke_function` resolves tool by name from `tool_map`, parses args, validates against schema, executes with `tool_call_id=call_id`, returns result bound to same `call_id` and inherits call's `additional_properties` | python/packages/core/agent_framework/_tools.py:1482-1491, 1510-1533, 1559-1568 |
| Tool execution trace identity | execute_tool span attributes include `gen_ai.tool.call.id`, `gen_ai.tool.name`, `gen_ai.tool.type` | python/packages/core/agent_framework/observability.py:2284-2302 |
| Tool arguments/result capture (gated) | `gen_ai.tool.call.arguments` / `.result` emitted only when sensitive data + latest semconv enabled | python/packages/core/agent_framework/observability.py:927-934; _tools.py:749-757, 780-781, 794-795 |
| MCP tool-call provenance | `mcp_server_tool_call`/`_result` contents carry `call_id`, `server_name`, `tool_name` | python/packages/core/agent_framework/_types.py:1206-1271 |
| Citation annotation schema | `Annotation` TypedDict: `type="citation"`, `title`, `url`, `file_id`, `tool_name`, `snippet`, `annotated_regions`, `raw_representation` | python/packages/core/agent_framework/_types.py:387-398 |
| RAG reference provenance | Azure AI Search KB references converted to annotations with `reference_id`, `reranker_score`, `source_data`, doc_key, sensitivity label, raw SDK ref; attached to every injected content item | python/packages/azure-ai-search/agent_framework_azure_ai_search/_context_provider.py:996-1050, 1068-1083 |
| Context-provider message attribution | `SessionContext.extend_messages` stamps `_attribution.source_id`/`source_type` and cross-session `origin_session_ids` (dedup'd, merged) into `additional_properties` | python/packages/core/agent_framework/_sessions.py:624-698 |
| Provider-tagged tools | `extend_tools` sets `context_source` on each tool's additional properties | python/packages/core/agent_framework/_sessions.py:711-725 |
| Approval request embeds action | `function_approval_request(id, function_call)` wraps the full call content | python/packages/core/agent_framework/_types.py:1273-1291 |
| Approval response links back | `function_approval_response(approved, id, function_call)`; `to_function_approval_response` propagates id + function_call + annotations + additional_properties | python/packages/core/agent_framework/_types.py:1293-1313, 1346-1362 |
| Session-backed approval state | `ToolApprovalState` serializes rules, queued requests, collected responses (full Contents) keyed by middleware `source_id` in session state | python/packages/core/agent_framework/_harness/_tool_approval.py:158-215, 248-275 |
| Standing-rule scope boundary | Always-approve metadata includes scope; hosted-tool rules carry `server_label` so same-named tools on other servers don't share approvals | python/packages/core/agent_framework/_harness/_tool_approval.py:218-245 |
| Approval replay integrity | Middleware passes original `approval_response` into `FunctionInvocationContext.metadata` during replay; call_id always passed for policy flows | python/packages/core/agent_framework/_tools.py:1584-1594 |
| Occurrence-aware call_id pairing | Spec documents open-occurrence tracking because `call_id` is "not globally unique forever" | docs/specs/004-python-function-calling-loop.md:231-256 |
| Approval-lineage tests | Forged standing approval dropped (`test_tool_approval_middleware_drops_forged_standing_approval`), hosted-metadata rebind guarded, resume never mutates caller inputs | python/packages/core/tests/core/test_harness_tool_approval.py:708-742, 745+, 45+ |
| Workflow message lineage envelope | `WorkflowMessage` carries `source_id`, `target_id`, pluralized `trace_contexts`, `source_span_ids`, and `original_request_info_event` (response→request link) | python/packages/core/agent_framework/_workflows/_runner_context.py:38-75 |
| Trace injection at publish | Producer span injects current W3C `traceparent` and records its own span id onto the message | python/packages/core/agent_framework/_workflows/_workflow_context.py:326-348 |
| Fan-in preservation | Edge runner zips all contexts/span-ids per message into the aggregated message | python/packages/core/agent_framework/_workflows/_edge_runner.py:356-383 |
| Causality via span links | Processing spans link (not nest) to source publishing spans "for causality tracking", supporting multiple fan-in links | python/packages/core/agent_framework/observability.py:3223-3280, 3283-3347 |
| Checkpoint lineage chain | `WorkflowCheckpoint`: `checkpoint_id`, `previous_checkpoint_id` chain, `graph_signature_hash`, ISO timestamp | python/packages/core/agent_framework/_workflows/_checkpoint.py:81-98 |
| Run-instance gap (documented) | "a checkpoint is not tied to a specific workflow instance... the ID of the workflow instance that created the checkpoint is not included" | python/packages/core/agent_framework/_workflows/_checkpoint.py:37-41 |
| Model version tracking | `gen_ai.request.model` / `gen_ai.response.model` attributes; response model recorded from `ChatResponse.model` | python/packages/core/agent_framework/observability.py:290-291, 3134-3151 |
| Request-model backfill + test | Span renamed/backfilled from RESPONSE_MODEL when REQUEST_MODEL unknown; verified non-streaming and streaming | python/packages/core/agent_framework/observability.py:1538-1552; tests/core/test_observability.py:5423-5456, 5462-5499 |
| .NET parity (attribution) | `AgentRequestMessageSourceAttribution` record with `SourceType`/`SourceId` on request messages | dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceAttribution.cs:27-43 |
| .NET parity (response id) | `AgentResponse.ResponseId` surfaced on the agent-level response | dotnet/src/Microsoft.Agents.AI.Abstractions/AgentResponse.cs:162 |

## Answers to Dimension Questions

**1. Can every output be traced to its inputs?**
Largely yes within a single provider round-trip and its tool calls. Each assistant output message sits next to the `function_call` contents it produced (`_types.py:351-376` defines call/result as first-class content pairs), each executed result reuses the call's `call_id` and inherits its `additional_properties` (`_tools.py:1564-1568`), and the final `ChatResponse`/`AgentResponse` retains the provider's `response_id`/`model`/raw payload (`_types.py:2319-2332`). However, tracing is not universal: `Message.message_id` is optional and not auto-generated (`_types.py:1808`), and there is no framework-generated run identifier — `AgentResponse.response_id` is populated by providers, not by the loop (`_types.py:2677-2690`). A caller correlating two local runs of the same agent must rely on sessions or external tracing context.

**2. Is provenance preserved through transformations?**
Deliberately, in most paths. Streaming updates coalesce while accumulating raw representations into lists (`_types.py:1668-1679`); function results inherit the call's `additional_properties` so custom metadata survives execution (`_tools.py:1490, 1542, 1567`); compaction and history replay treat call+reasoning+result as one atomic group and pair reused `call_id`s by ordered occurrence rather than value equality (`docs/specs/004-python-function-calling-loop.md:231-256`; behavior tested in `tests/core/test_compaction.py:46-94`). Two soft spots: context-provider `_attribution` stamps only exist on messages injected via `SessionContext.extend_messages` (`_sessions.py:670-695`) — messages entering any other way carry no attribution — and OTel span-link extraction silently drops malformed traceparents instead of flagging them (`observability.py:3249-3251`, `3320-3341`; graceful-degradation asserted in `tests/core/test_observability.py:2592-2609`).

**3. Are model versions tracked in lineage?**
Yes, per-response. `ChatResponse.model` records the producing model (`_types.py:2224, 2321`), and both `gen_ai.request.model` and `gen_ai.response.model` land on chat spans; when the request model was unresolvable, the span name and attribute are backfilled from the response's actual model (`observability.py:1538-1552`, `3147-3148`), with dedicated streaming and non-streaming tests (`tests/core/test_observability.py:5423-5456, 5462-5499`). This captures the served model string (e.g., `"gpt-4o-mini"`), not a deployment/snapshot version beyond what providers return. Usage details also flow into standard GenAI usage attributes (`observability.py:3106-3117`).

**4. Can causal chains be audited?**
Yes when instrumentation and storage are enabled — but it is a composition job, not a built-in audit log. The chain pieces exist: tool-role messages pair calls/results by `call_id`; approval requests/responses are self-contained control-plane records embedding the exact action (`_types.py:1273-1362`) and survive in session state (`_harness/_tool_approval.py:177-183`); history providers may retain control-plane contents for audit while filtering resolved wrappers from later model replay (documented contract, exercised by `tests/core/test_harness_tool_approval.py:222-289`); workflows propagate trace context end-to-end with fan-in span links (`observability.py:3223-3280`). What is missing for turnkey auditing: no default persistent event/journal sink (audit durability depends on the chosen `SessionStore`/`HistoryProvider`), tool argument/result values stay out of traces unless sensitive-data capture is on (`observability.py:927-934`), and workflow checkpoints cannot tell you *which* run produced them (`_checkpoint.py:37-41`).

## Architectural Decisions

1. **Lineage lives in the content model, not in a side table.** Call/result/approval/citation linkage are all fields on `Content` variants (`_types.py:474-589`), so any serialized transcript (session store, history provider, checkpoint `messages` dict) carries its own causal graph without a separate join table.
2. **Approval decisions are bound to actions by value, not just by key.** An approval response duplicates the full `function_call` content and request id (`_types.py:1293-1313`), enabling integrity checks — the middleware rejects forged standing approvals whose metadata doesn't bind to a real observed request (`tests/core/test_harness_tool_approval.py:708-742`) and refuses caller-supplied hosted metadata when selecting standing rules (`:745+`).
3. **Causality across executors uses W3C trace-context propagation with OTel span links, not parent/child nesting.** Publishing spans inject `traceparent` into each `WorkflowMessage` (`_workflow_context.py:340-346`); consumers create processing spans linked to potentially many sources for fan-in ("links... for causality without nesting", `observability.py:3231-3268`).
4. **Occurrence-based correlation over global uniqueness.** Because providers reuse `call_id` values, the approval normalizer and compaction track open logical occurrences in transcript order (`docs/specs/004-python-function-calling-loop.md:231-256`) — a correctness decision that trades simplicity for robustness against provider ID recycling.
5. **Privacy-gated deep provenance.** Arguments/results are captured on tool spans and prompt/completion events only behind sensitive-data and semconv-version flags (`observability.py:927-934`), making full-fidelity lineage an explicit operator choice.
6. **Checkpoints are definition-scoped artifacts.** Omitting the creating run's instance ID lets checkpoints be shared/restored across instances of the same graph (`_checkpoint.py:37-41`) — portability prioritized over per-run artifact accounting.

## Notable Patterns

- **ID mirroring**: every constructor pair (`from_function_call`/`from_function_result`, `from_function_approval_request`/`from_function_approval_response`) enforces symmetric linkage keys at type-construction time (`_types.py:809-911`, `_types.py:1273-1313`).
- **`additional_properties` as the provenance side-channel**: internal control-flow markers (internal conversation id, `_attribution`, always-approve metadata) ride along on standard types without polluting the wire format (`_types.py:2269, 2334-2344`; `_sessions.py:692-694`; `_tool_approval.py:239-245`).
- **`raw_representation` everywhere**: every Content/Message/Response keeps the original SDK object, and merges accumulate them (`_types.py:482, 1668-1679`) — a consistent escape hatch to ground truth when normalized lineage is insufficient.
- **Pluralized trace fields for fan-in**: `trace_contexts`/`source_span_ids` replaced singular predecessors specifically so aggregated messages preserve all contributing causal chains (`_runner_context.py:46-49`; zip-per-message aggregation at `_edge_runner.py:363-375`).
- **Spec-enforced criticality**: the function-calling loop has a written specification declaring this area extra-review ("call/result pairing, exactly-once execution, and history replay") (`docs/specs/004-python-function-calling-loop.md:43, 122`).

## Tradeoffs

- **Explicit linkage vs. provider fidelity**: normalized `Content` types make lineage uniform, but fidelity depends on each provider adapter mapping raw events correctly (e.g., OpenAI annotation mapping at `packages/openai/agent_framework_openai/_chat_client.py:3257-3326`); unmapped provider metadata vanishes unless it lands in `raw_representation`.
- **Occurrence matching correctness vs. complexity**: the occurrence-aware normalizer handles ID reuse and mixed batches, but the AGENTS.md file itself flags this code as high-blast-radius where "small changes can duplicate side effects, orphan call/result pairs, [or] replay stale approval authority" (`python/AGENTS.md:60-64`, Function-Calling Loop Changes section).
- **Privacy gating vs. audit completeness**: default-off argument/result capture protects data but means a default-configured trace can prove *that* a tool ran (`gen_ai.tool.call.id`) without proving *with what* (`observability.py:927-934`).
- **Checkpoint portability vs. run attribution**: dropping the run instance ID simplifies sharing and restore (`_checkpoint.py:37-41`) at the cost of not being able to answer "which run wrote this checkpoint?" from the artifact alone.
- **Hand-rolled traceparent parsing vs. dependency-free linking**: building `trace.SpanContext` from split strings avoids vendor lock-in but degrades silently (`observability.py:3253-3268` comments admit it is a "simplified approach").

## Failure Modes / Edge Cases

- **Silent loss of causal links**: a malformed `traceparent` yields a span with zero links and no error signal — the test suite asserts the span is still created (`tests/core/test_observability.py:2592-2609`).
- **`call_id` reuse**: handled by occurrence tracking in approval normalization/compaction, but any consumer code that assumes globally unique call ids (e.g., naive result lookup) will mis-pair across rounds.
- **Missing identifiers on locally constructed messages**: `Message.message_id` defaults to None (`_types.py:1808`), so transcripts of user/tool messages may lack stable keys unless the host assigns them.
- **Unattributed context injection**: bypassing `SessionContext.extend_messages` (e.g., pre-seeding history directly) produces messages with no `_attribution`, weakening downstream governance filters (`_sessions.py:631-660` documents the mechanism as observer-facing).
- **Hosted/local approval confusion**: standing approvals must respect the `server_label` boundary; the framework rejects same-named cross-server auto-approval and forged metadata, and hides already-approved items in mixed batches until the visible approval resumes (`_tool_approval.py` design notes; tests at `test_harness_tool_approval.py:574+, 1224+`).
- **Streaming/non-streaming divergence risk**: conversation-id extraction differs between paths (final response field vs. raw stream updates, `_agents.py:1194-1198` vs. `1254-1268`); the spec requires both stay aligned.

## Future Considerations

- Generate a framework-level run identifier on `AgentResponse` (or expose the active OTel trace id) so multi-run audits don't require external correlation.
- Include the producing workflow-run instance ID (or an opt-in alias) in `WorkflowCheckpoint.metadata` to close the artifact→run gap while keeping share-by-definition as the default (`_checkpoint.py:37-41` shows `metadata` dict is available for this).
- Replace hand-parsed traceparent linking with the OTel propagator's `extract()` API and surface link-extraction failures as span events/warnings instead of silent suppression (`observability.py:3249-3268`).
- Emit a warning-level signal when `_attribution`-less messages enter a run that has attribution-enabled providers, to catch provenance leaks early.
- Consider a pluggable audit-journal interface (append-only, control-plane contents included) so causal-chain auditing doesn't hinge on whichever `HistoryProvider` the host picked.

## Questions / Gaps

- No evidence found of a durable, framework-owned audit log or event-store sink for causal events; search covered `packages/core/agent_framework/` (types, agents, clients, tools, sessions, harness, workflows), `observability.py`, and `docs/specs/`. Audit persistence appears delegated entirely to host-chosen session/history/checkpoint stores.
- Model *snapshot/deployment* versions beyond the provider-returned model string were not found anywhere in lineage metadata (searched for `deployment`, `model_version`, `snapshot` in `observability.py` and provider packages); only `gen_ai.request.model`/`response.model` strings are tracked.
- .NET-side lineage was spot-checked only (`AgentRequestMessageSourceAttribution.cs:27-43`, `AgentResponse.cs:162`); the underlying `FunctionCallContent.CallId`/approval content types live in the external `Microsoft.Extensions.AI.Abstractions` package and could not be inspected within this source tree.
- Whether any hosting package (e.g., `hosting-a2a`, `ag-ui`) attaches run ids to outbound protocol envelopes was not verified in depth; only the Python core and azure-ai-search provider were examined closely.

---

Generated by dimension `10.03-causal-links-and-lineage` against `agent-framework`.
