# Source Analysis: pydantic-ai

## 10.03 Causal Links and Lineage

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, OpenTelemetry, anyio; uv workspace with `pydantic_ai_slim`, `pydantic_graph`, `pydantic_evals`) |
| Analyzed | 2026-08-25 |

## Summary

Pydantic AI makes the **message history itself the causal ledger**. Every `ModelRequest` and `ModelResponse` is a typed dataclass carrying lineage metadata (`run_id`, `conversation_id`, `timestamp`, and on responses `model_name`, `provider_name`, `provider_response_id`, `finish_reason`, per-request `usage`). Tool causality is anchored by a single join key — `tool_call_id` — which links a model-issued `ToolCallPart` to its `ToolReturnPart`/`RetryPromptPart`, to approval decisions (`DeferredToolRequests.approvals` / `DeferredToolResults.approvals` keyed by that ID), and to the final output (`FinalResult.tool_call_id`). Run identity is generated as UUID7 per run (`run_id`) and per conversation (`conversation_id`), soft-stamped onto every message via `fill_run_metadata`, serialized through `ModelMessagesTypeAdapter`, and propagated into OpenTelemetry spans (baggage `gen_ai.agent.call.id`, `gen_ai.conversation.id`; chat spans record full input/output messages; tool spans carry `gen_ai.tool.call.id`). A dedicated history-repair pipeline (`_drop_orphaned_tool_results`, `_repair_dangling_tool_calls`) actively preserves call/result pairing integrity across context eviction and hand-built histories, and duplicate or reused IDs are rejected with explicit errors rather than silently mis-bound.

The main gaps: there is no first-class retrieval-provenance layer (source URLs in search results are the tool author's job), no artifact store that links produced files back to runs beyond their presence inside message parts, and several provenance fields (`metadata`, `provider_details`) are untyped `Any` dicts.

**Answering the dimension's guiding question — "Can you trace a final answer back to the specific tool call that provided each fact?"** — Yes for structured outputs: `FinalResult.output` carries the exact `tool_name`/`tool_call_id` of the output-tool call that produced it (`pydantic_ai_slim/pydantic_ai/result.py:1032-1042`), and every intermediate fact contributed by a function tool is traceable through its `ToolCallPart`→`ToolReturnPart` pair sharing a `tool_call_id`. For free-text answers, facts are traceable only by walking the message history's call/result pairs — the framework preserves the chain but does not annotate which text span came from which tool.

## Rating

**8 / 10** — Clear lineage model with explicit interfaces (`tool_call_id` join key; `run_id`/`conversation_id` stamps; provider/model identifiers on responses), extensive test coverage of the linkage semantics, and operational safeguards (history repair, duplicate-ID rejection, stale-cache detection warnings). Falls short of 9–10 because lineage durability depends on callers persisting message history themselves (no built-in lineage/artifact store), provenance metadata fields are schema-less `Any`, and retrieved-context provenance is not enforced by the framework.

## Evidence Collected

Every entry includes a file path with line numbers. All paths are relative to `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Input/output linking | `ModelResponse` records `model_name`, `usage`, `provider_name/url/details`, `provider_response_id` ("track the specific request to the model"), `finish_reason`, `run_id`, `conversation_id` | `pydantic_ai_slim/pydantic_ai/messages.py:2539-2618` |
| Input/output linking | `ModelRequest` carries `instructions`, `timestamp`, `run_id`, `conversation_id`, app-only `metadata`, lifecycle `state='interrupted'` detection | `pydantic_ai_slim/pydantic_ai/messages.py:1831-1870` |
| Run identity | `GraphAgentState.run_id` / `conversation_id` default to fresh UUID7; `run_id` never inherited from history | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299-318` |
| Run identity resolution | `resolve_conversation_id` priority: explicit → `'new'` fork sentinel → most recent on history → fresh UUID7; `resolve_run_id` raises `UserError` if an explicit id already appears in history (protects `new_messages()` boundary) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:237-295` |
| Lineage stamping | `fill_run_metadata` soft-fills `timestamp`/`run_id`/`conversation_id` preserving producer-supplied values; applied at request/response materialization points | `pydantic_ai_slim/pydantic_ai/_utils.py:560-569`; `_agent_graph.py:1468,1532,1732,1784` |
| Tool result ↔ call link | `BaseToolReturnPart.tool_call_id` defaults to generated ID ("used by some models including OpenAI"); plus `tool_kind`, `metadata`, `outcome` (`success/failed/denied/interrupted`) | `pydantic_ai_slim/pydantic_ai/messages.py:1298-1351` |
| Tool call side | `BaseToolCallPart`: `tool_call_id`, provider-scoped `id` + `provider_name` (OpenAI Responses), `provider_details` | `pydantic_ai_slim/pydantic_ai/messages.py:2160-2207` |
| ID generation fallback | `generate_tool_call_id()` prefixed `pyd_ai_`; `guard_tool_call_id()` regenerates when missing | `pydantic_ai_slim/pydantic_ai/_utils.py:572-600` |
| Streaming lineage | `_parts_manager.handle_tool_call_part` synthesizes `tool_call_id=tool_call_id or _generate_tool_call_id()` so streamed calls always carry a stable ID | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:468-500` |
| Output provenance | `FinalResult(output, tool_name, tool_call_id)` — output tied to the producing output-tool call; both `None` for text outputs | `pydantic_ai_slim/pydantic_ai/result.py:1031-1044` |
| Final-result event | `FinalResultEvent(tool_name, tool_call_id)` marks which stream part will produce the run output | `pydantic_ai_slim/pydantic_ai/messages.py:3904-3913` |
| Binding integrity | Duplicate `tool_call_id`s rejected before execution ("Results are matched back to calls by `tool_call_id`, so duplicate ids make the binding ambiguous") | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:405-419,119-126` |
| Result matching | Deferred-call results matched back strictly by `tool_call_id`; mismatched sets raise `UnexpectedModelBehavior` | `pydantic_ai_slim/pydantic_ai/_tool_execution.py:966-997,413-419` |
| Retry lineage | `RetryPromptPart` keeps `tool_name` + `tool_call_id` so retries stay bound to the originating call; `from_error` factory reuses the call's ID | `pydantic_ai_slim/pydantic_ai/messages.py:1636-1697` |
| History repair | Ordered-walk dangling-call detector; orphaned results dropped (Anthropic/OpenAI reject unmatched results); synthesized returns marked `SYNTHESIZED_TOOL_RETURN_METADATA_KEY='pydantic_ai_synthesized_tool_return'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2702-2768,2771-2813,2816+` |
| Approval-action links | `DeferredToolRequests.approvals: list[ToolCallPart]`; `DeferredToolResults.approvals/calls` dicts keyed by `tool_call_id`; `build_results` raises `ValueError` for unknown IDs; `approve_all` fills defaults | `pydantic_ai_slim/pydantic_ai/_deferred.py:26-96` |
| Approval outcomes | `ToolApproved(override_args)` / `ToolDenied(message)` discriminated by `kind`; denial rendered as `outcome='denied'` return part sent to the model as ordinary result, not error | `pydantic_ai_slim/pydantic_ai/_deferred.py:99-118`; `messages.py:1335-1351`; `_tool_execution.py:674-694` |
| Approval metadata flow | Per-call `metadata` keyed by `tool_call_id` surfaces in `RunContext.tool_call_metadata` | `_deferred.py:41-42,169-170`; `pydantic_ai_slim/pydantic_ai/_run_context.py:119-120` |
| RunContext causality | `RunContext.tool_call_id`, `tool_name`, `retry`, `tool_call_metadata`, `run_id`, `conversation_id` injected into every tool execution | `pydantic_ai_slim/pydantic_ai/_run_context.py:61-130` |
| Model version tracking | Providers stamp `model_name=response.model`, `provider_response_id=response.id`, `provider_name/url`, mapped `finish_reason` onto each response | `pydantic_ai_slim/pydantic_ai/models/openai.py:1271-1281` |
| OTel request/response attrs | `gen_ai.request.model`, `gen_ai.provider.name`, `server.address/port`; response finish sets `gen_ai.response.model`, `gen_ai.response.id` (= `provider_response_id`), `gen_ai.response.finish_reasons`, usage/cost | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:284-327,399-420` |
| OTel agent-run span | `invoke_agent` span with `gen_ai.agent.name`, `gen_ai.agent.call.id`=run_id, `gen_ai.conversation.id`; baggage propagation so child spans inherit run identity; ends with `pydantic_ai.all_messages` + `pydantic_ai.new_message_index` | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:152-277` |
| OTel message capture | Chat spans serialize full `gen_ai.input.messages`/`gen_ai.output.messages` incl. `tool_call_response` parts carrying ids; per-run fragment cache keeps this O(new messages) and warns on in-place mutation (`MessageHistoryMutatedWarning`) | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:225-289`; `_instrumentation.py:90-120`; `capabilities/instrumentation.py:224-234` |
| OTel tool spans | `execute_tool {name}` spans with `gen_ai.tool.call.id` = `call.tool_call_id`, args/result attributes, failure-stage attribute | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:383-395,556-562` |
| Usage lineage | `ModelResponse.usage` per-request; `RunUsage` cumulative on run state; `Usage.opentelemetry_attributes()` feeds span attrs; cost priced from `model_name`+provider via genai-prices | `pydantic_ai_slim/pydantic_ai/messages.py:2547-2554`; `_agent_graph.py:303`; `usage.py:218`; `_instrumentation.py:423-431` |
| Run-result audit API | `AgentRunResult.all_messages()/new_messages()`, `.response`, `.usage`, `.run_id`, `.conversation_id`, `_traceparent` (OTel linkage); new-message boundary computed via `_first_new_message_index` falling back to stamped `run_id` scan | `pydantic_ai_slim/pydantic_ai/run.py:593-733`; `_agent_graph.py:2636-2684` |
| Serialization round-trip | `ModelMessagesTypeAdapter` preserves `run_id`/`conversation_id` across dump/validate; back-compat for histories predating fields | `tests/test_messages.py:861-905` |
| Tests: run/conversation stamps | `test_agent_message_history_includes_run_id`, `..._includes_conversation_id`, `run_id_not_inherited_from_message_history`, `run_id_fresh_on_deferred_resume`, `run_id_reuse_in_message_history_raises`, `preserves_model_response_run_id` | `tests/test_agent.py:3863-3882,3965,4052,4080,4144` |
| Tests: repair of broken chains | `test_reused_tool_call_id_dangling_call_repaired`, `test_reused_tool_call_id_shadowed_open_call_repaired` | `tests/test_transcript_repair.py:749,784` |
| Failure-time lineage | `capture_run_messages` yields partial messages even on exception, with `state='interrupted'` marking truncated requests/responses | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2523-2554`; `messages.py:1864-1870` |
| Durable persistence | Temporal/DBOS/Prefect wrappers thread `run_id`/`conversation_id`/`deferred_tool_results` through durable runs, keeping lineage across engine boundaries | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:455-516` |

## Answers to Dimension Questions

### 1. Can every output be traced to its inputs?

Yes, structurally. The run's message history contains the complete causal sequence: each `UserPromptPart` (input prompt, timestamped) sits in a `ModelRequest` alongside the tool returns that fed the next turn; each `ModelResponse` names the model/provider and carries `provider_response_id` (`messages.py:2539-2618`). Structured outputs carry a direct pointer: `FinalResult.tool_call_id` identifies the exact output-tool invocation that produced the value (`result.py:1041-1042`), set at `pydantic_ai_slim/pydantic_ai/_tool_execution.py:526`. Every fact delivered by a function tool is bound to its call via the shared `tool_call_id` (`messages.py:1308`, `2173`). Limitations: for plain-text outputs there is no fact-level attribution — you can enumerate which tools ran before the answer but not which sentences derived from which result; and `include_content=False` instrumentation drops prompt/completion payloads from telemetry while keeping IDs (`models/instrumented.py:76-77,102-109`).

### 2. Is provenance preserved through transformations?

Largely yes, with explicit guardrails. Serialization/deserialization through `ModelMessagesTypeAdapter` preserves `run_id`/`conversation_id` (`tests/test_messages.py:861-889`) and typed tool-kind promotion survives round-trips while unsubstantiated kinds are stripped rather than kept invalid (`messages.py:2371-2402`). The history-repair pipeline maintains pairing integrity under lossy transformations: orphaned results are removed and dangling calls get synthesized returns flagged with `SYNTHESIZED_TOOL_RETURN_METADATA_KEY` so downstream consumers can distinguish real results from repairs (`_agent_graph.py:2702-2813`). `fill_run_metadata` never overwrites producer-set values (`_utils.py:560-569`), pinned by `test_agent_preserves_model_response_run_id` (`tests/test_agent.py:4144`). Two caveats: user-provided history processors may rebuild messages, which the instrumentation detects post-hoc and reports via `MessageHistoryMutatedWarning` rather than preventing (`capabilities/instrumentation.py:224-234`); and the three-layer fallback for locating the resumed-request boundary in `new_messages()` acknowledges mutation scenarios where even run-id scanning degrades (`_agent_graph.py:2644-2684`).

### 3. Are model versions tracked in lineage?

Yes, at two levels. On the message: `ModelResponse.model_name` is taken from the provider's own echoed model field (`models/openai.py:1274`), alongside `provider_name`, `provider_url`, `provider_details['timestamp']`, and `provider_response_id` — the provider-side request ID usable for cross-referencing provider logs. On telemetry: chat spans emit `gen_ai.request.model` and `gen_ai.response.model` distinctly (`_instrumentation.py:306-327,411-413`), `gen_ai.response.id` mirrors `provider_response_id` (`_instrumentation.py:416-417`), and cost attribution is computed from `model_name`+provider via genai-prices (`_instrumentation.py:423-431`). What is *not* tracked: framework/package version per message (only the tracer scope carries `__version__`, `models/instrumented.py:143-149`), and there is no model-version field on `FinalResult` itself — you recover it from the corresponding `ModelResponse`.

### 4. Can causal chains be audited?

Yes, through four complementary channels: (a) the serializable message history with per-message `run_id`/`conversation_id` stamps and call/result ID pairs; (b) OTel traces — an `invoke_agent` run span whose baggage (`gen_ai.agent.call.id`, `gen_ai.conversation.id`) propagates into child `chat` and `execute_tool` spans keyed by `gen_ai.tool.call.id` (`capabilities/instrumentation.py:168-189,383-395`), closed out with the full `pydantic_ai.all_messages` snapshot (`capabilities/instrumentation.py:248`); (c) programmatic events — `FunctionToolCallEvent`/`FunctionToolResultEvent` exposing `tool_call_id` properties (`messages.py:3946-4006`); (d) approval audits — deferred approvals/denials are keyed by `tool_call_id` end-to-end, with unknown-ID results rejected (`_deferred.py:70-80`) and denials recorded as `outcome='denied'` in history (`messages.py:1335-1351`). Failed runs remain auditable via `capture_run_messages` returning partial history marked `state='interrupted'` (`_agent_graph.py:2523-2554`). Durable-execution integrations preserve the same identifiers across Temporal/DBOS/Prefect boundaries (`durable_exec/temporal/_agent.py:455-516`).

## Architectural Decisions

1. **Message history as the sole system of record.** Rather than a side-table linking outputs to inputs, lineage lives on immutable-ish typed message parts (`messages.py:1831-1870`, `2539-2618`). This makes lineage automatically serializable, replayable, and durable-execution-friendly, at the cost of depending on callers to persist histories.
2. **One join key for everything: `tool_call_id`.** Calls, returns, retries, approvals, denials, external results, output tools, OTel tool spans, and deferred metadata all key off the same ID (`messages.py:1308,1664,2173`; `_deferred.py:37-42`; `capabilities/instrumentation.py:395`). Uniformity makes arbitrary chains reconstructable with dict lookups.
3. **Fail-loud binding over silent best-effort.** Duplicate call IDs abort processing (`_tool_execution.py:405-419`), reused `run_id`s raise `UserError` because they break `new_messages()` boundaries (`_agent_graph.py:280-295`), and unknown approval keys raise `ValueError` (`_deferred.py:73-80`).
4. **Repair, don't reject, externally-broken chains.** Provider-rejected shapes (orphan results, dangling calls from context eviction) are repaired deterministically with synthesis markers instead of failing the run (`_agent_graph.py:2771-2813`).
5. **OTel GenAI semconv as the observability contract**, with versioned attribute formats (v2–v6) and custom namespaces explicitly quarantined (`gen_ai.aggregated_usage.*`) (`models/instrumented.py:79-141`; AGENTS.md rule: spec-only features in `_otel_*.py`).

## Notable Patterns

- **Soft-stamp centralization**: one helper owns the framework-tracked field list so new lineage fields need exactly one edit (`_utils.py:560-569`).
- **Ordered-walk open-call accounting**: dangling-call detection models call/result pairing like a matching problem, handling shadowed IDs and out-of-order results explicitly (`_agent_graph.py:2706-2733`).
- **Identity-keyed serialization cache**: OTel input-message fragments cached per `id(message)` + `parts` identity, making repeated lineage serialization O(new messages) and detecting in-place mutation (`_instrumentation.py:90-120`; `models/instrumented.py:240-251`).
- **Discriminated unions everywhere**: `part_kind`/`kind` discriminators make lineage records self-describing on the wire (`messages.py:1848-1849`, `2566-2567`, deserialization auto-promotion at `2320`).
- **Provider extras normalized, not overloaded**: provider-specific data goes in `provider_name`/`provider_details` slots with required-provider validation rather than being smuggled into content strings (`messages.py:2188-2207`; rule codified in source AGENTS.md).

## Tradeoffs

- **In-memory ledger vs. lineage store**: zero-infrastructure lineage, but nothing persists unless the application saves `all_messages_json()`; there is no queryable lineage database or artifact registry.
- **UUID7 run/conversation IDs vs. caller-supplied identity**: good sortability and collision freedom, but cross-service correlation relies on callers threading explicit IDs (supported, e.g., UI adapters map request/thread IDs: `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_event_stream.py:129`).
- **Content-rich telemetry vs. privacy**: spans include prompts, completions, and binary content by default (`include_content=True`, `include_binary_content=True`), with opt-out redaction (`_instrumentation.py:141-167`) — full auditability trades against data exposure.
- **Untyped extensibility vs. schema rigor**: `metadata: Any` on returns/requests and `provider_details: dict[str, Any]` maximize flexibility but weaken machine-checked provenance.
- **History repair vs. fidelity**: synthesized returns keep runs alive after context eviction but replace lost results with placeholders (marked, though — `SYNTHESIZED_TOOL_RETURN_METADATA_KEY`).

## Failure Modes / Edge Cases

- **Duplicate model-issued `tool_call_id`s**: hard `UnexpectedModelBehavior` before execution and before deferral (`_tool_execution.py:405-419,966-973`).
- **Reused explicit `run_id` in provided history**: `UserError` with remediation guidance (`_agent_graph.py:286-293`).
- **Orphan results / dangling calls** from hand-built histories or context eviction: dropped/synthesized during `_clean_message_history` (`_agent_graph.py:2771+`; tests `tests/test_transcript_repair.py:749,784`).
- **Interrupted runs**: partial requests/responses retained with `state='interrupted'` so consumers detect incomplete causality rather than mistaking them for complete turns (`messages.py:1864-1870`, `2605-2618`).
- **Mid-run in-place history mutation**: not prevented; detected at run end and reported via `MessageHistoryMutatedWarning` noting recorded spans may diverge from what was sent (`capabilities/instrumentation.py:224-234`).
- **Instrumentation must not break runs**: redaction failures degrade to a placeholder string rather than raising (`_instrumentation.py:158-167`).

## Future Considerations

- Add optional typed provenance schemas for `metadata`/`provider_details` (the repo's own guidelines push `TypedDict`/dataclass over `dict[str, Any]`).
- First-class retrieval provenance: a standard envelope for RAG-style tool returns (source URL/id/confidence) so fact-level tracing doesn't depend on each tool author inventing one (cf. `common_tools/duckduckgo.py:24-32`, where `href` exists only because DuckDuckGo returns it).
- Surface `model_name`/provider on `FinalResult` directly, avoiding a history lookup to answer "which model produced this output".
- Optional persisted lineage sink (beyond user-managed `all_messages_json()` and OTel export) for applications that need auditable chains without adopting an observability backend.

## Questions / Gaps

- No evidence found of a built-in mechanism linking generated files/artifacts (e.g., `BinaryContent`, `FileUrl` parts) to the run that produced them beyond their containment in stamped messages; searched `pydantic_ai_slim/pydantic_ai/` for artifact registries, file-store integrations, and output-to-run mapping tables (none present; closest analogues are message-contained media and durable-execution state).
- Retrieval-augmentation provenance is delegated entirely to tool implementations; searched `common_tools/`, `toolsets/`, and docs for a standard citation/source-metadata envelope — none found.
- Framework-version stamping per message/response is absent (only tracer scope version and instrumentation *format* versions exist); impact is low since histories rarely cross package versions without migration code (`tests/test_messages.py:892-905` shows back-compat handling).
- Fact-level attribution within free-text outputs is not modeled anywhere; the search covered `output.py`, `result.py`, `_output*`, and message part types.

---

Generated by `10.03-causal-links-and-lineage` against `pydantic-ai`.
