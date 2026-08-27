# Source Analysis: openai-agents-sdk

## 09.04 Governance Evidence Generation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, `agents` SDK (`src/agents/`) |
| Analyzed | 2026-08-26 |

## Summary

`openai-agents-sdk` does not provide a dedicated compliance/evidence report generator. Governance evidence is fragmented across three durable subsystems: (1) the tracing subsystem (`src/agents/tracing/`) which emits machine-readable `Trace`/`Span` dicts via `export()`/`to_json()` and a pluggable `TracingProcessor`/`TracingExporter` pipeline; (2) the run-result / run-state persistence boundary (`src/agents/result.py`, `src/agents/run_state.py`, `src/agents/run_context.py`) which serializes guardrail results, tool approvals, tool invocations, model responses and usage into `RunState.to_json()` / `RunResult` fields; and (3) the session-persistence and logging helpers (`src/agents/logger.py`, `src/agents/usage.py`). Guardrail tripwire outcomes, function/tool input-output payloads, and approval decisions are recorded, but approval **timestamps**, retention enforcement, and a single compliance report artifact are absent. SIEM integration is possible only by implementing a custom `TracingProcessor`, not via a built-in connector.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, and fragile for governance use.**

*Why not higher:* Tracing is mature, typed, tested (`tests/test_tracing.py:74`, `tests/tracing/test_traces_impl.py`), and exports structured JSON (`src/agents/tracing/traces.py:152`, `src/agents/tracing/spans.py:396`). RunState serializes guardrail decisions and approvals with schema versioning (`src/agents/run_state.py:182`, `src/agents/run_state.py:1704`). However: no first-class evidence report or policy-decision ledger exists; tool approval items carry no wall-clock timestamp; retention is limited to in-memory queue limits and default 5 s shutdown flush, not a compliance retention policy; SIEM integration requires custom code. The system answers "what happened" (spans + items) and "what was checked" (guardrail results) but not "what controls applied when and by whom, for how long" in a governance-grade bundle.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Evidence report generation — no dedicated report | No `compliance_report`, `evidence_report`, `audit_report` symbol found in `src/agents/`; `glob **/*.py` and `grep "evidence.*report|compliance.*report|audit.*log"` return only template/product-name wording (`examples/voice/static/index.html:304`, `docs/models/index.md:1150`). The closest composition is ad-hoc via `RunState.to_json()` + trace `export()`. | `src/agents/run_state.py:1704` |
| Trace evidence artifact | `Trace.export()` returns `{object:"trace",id,workflow_name,group_id,metadata}` and `Trace.to_json()` optionally includes `tracing_api_key`; `TraceState.to_json()` serializes trace_id/workflow_name/group_id/metadata/api_key_hash for resume. | `src/agents/tracing/traces.py:380-387`, `src/agents/tracing/traces.py:171-184`, `src/agents/tracing/traces.py:245-277` |
| Span evidence artifact | `Span.export()` returns `{object:"trace.span",id,trace_id,parent_id,started_at,ended_at,span_data,error,metadata}`; `started_at`/`ended_at` captured via `util.time_iso()` on `SpanImpl.start()`/`finish()` with optional `trace_metadata` routing (`agent_harness_id`). | `src/agents/tracing/spans.py:396-422`, `src/agents/tracing/spans.py:342-361` |
| Machine-readable span types | `SpanData.export()` hierarchy covers `agent`, `generation`, `function`, `response`, `handoff`, `custom`, `guardrail`, `transcription`, `speech`, `mcp_tools`; `FunctionSpanData` captures `input`/`output`/`mcp_data`; `GuardrailSpanData` captures `triggered`. | `src/agents/tracing/span_data.py:11-451` |
| Policy decision records — guardrails | `RunResultBase` stores `input_guardrail_results: list[InputGuardrailResult]`, `output_guardrail_results: list[OutputGuardrailResult]`, `tool_input_guardrail_results`, `tool_output_guardrail_results`; `RunState` persists them via `_serialize_guardrail_results` (guardrail name/type, `tripwireTriggered`, `outputInfo`, `agentOutput`) and `_serialize_tool_guardrail_results` (`behavior`). | `src/agents/result.py:325-333`, `src/agents/run_state.py:1778-1812`, `src/agents/run_state.py:2762-2812` |
| Policy decision records — guardrail definitions | `InputGuardrailResult.guardrail + output: GuardrailFunctionOutput(tripwire_triggered, output_info)`; `GuardrailSpanData(name, triggered)` emitted via `guardrail_span(name,triggered)`. | `src/agents/guardrail.py:35-47`, `src/agents/tracing/span_data.py:292-314`, `src/agents/tracing/create.py:325-350` |
| Approval evidence — storage | Approvals tracked in `RunContextWrapper._approvals: dict[str|HostedMCPApprovalKey, _ApprovalRecord]` (approved/rejected per-call lists or sticky booleans + `rejection_messages` + `sticky_scope`); mirrored to `RunState.context.approvals` on serialization. | `src/agents/run_context.py:56-106`, `src/agents/run_state.py:1742-1754` |
| Approval evidence — pending interruptions | `RunState._current_step: NextStepInterruption | NextStepRunAgain` with `interruptions: list[ToolApprovalItem]`; serialized via `_serialize_tool_approval_interruption` (raw_item dict, agent ref, tool_name/namespace/lookup_key/tool_origin). `RunResult.interruptions` and `RunResultStreaming.interruptions` carry live pending approvals. | `src/agents/run_state.py:826-849`, `src/agents/run_state.py:2565-2593`, `src/agents/result.py:515` |
| Approval timestamps — absent | `ToolApprovalItem` fields are `raw_item, tool_name, tool_namespace, tool_origin, tool_lookup_key, _allow_bare_name_alias` with no `timestamp/approved_at/rejected_at`; `RunState` approval serialization contains no time field; timestamp search in `src/agents` only hits `Span.started_at/ended_at` and tracing `time_iso()`. | `src/agents/items.py:555-684`, `src/agents/run_state.py:2565-2593` |
| Tool execution results — logs | Model call evidence via `GenerationSpanData(input,output,model,model_config,usage)` and `ResponseSpanData(response_id)`; tool call evidence via `FunctionSpanData(name,input,output,mcp_data)`, `ToolCallItem/ToolCallOutputItem` in `RunItem` union and `ModelResponse.output: list[TResponseOutputItem]`. `RunResult.new_items` + `raw_responses` + `context_wrapper.usage` form the per-run execution ledger. | `src/agents/tracing/span_data.py:135-167`, `src/agents/tracing/span_data.py:169-241`, `src/agents/tracing/span_data.py:135-145`, `src/agents/items.py:686-701`, `src/agents/result.py:308-354` |
| Tool execution logs — serialization | `RunState._serialize_item()` preserves `type, raw_item, agent, output, source_agent, target_agent, tool_name/namespace/lookup_key/origin/custom_data`; `_serialize_tool_action_groups` preserves function/computer/custom/shell/apply_patch/mcp/handoff calls with params schema. | `src/agents/run_state.py:1933-1991`, `src/agents/run_state.py:2596-2675` |
| SIEM integration code — interface | `TracingProcessor` ABC (`on_trace_start/on_trace_end/on_span_start/on_span_end/shutdown/force_flush`) and `TracingExporter.export(items)` are the integration seam; `SynchronousMultiTracingProcessor` fans out to all registered processors; `add_trace_processor() / set_trace_processors() / set_trace_provider()` expose registration. | `src/agents/tracing/processor_interface.py:9-142`, `src/agents/tracing/provider.py:93-220`, `src/agents/tracing/__init__.py:93-106` |
| SIEM integration code — default exporter | `BackendSpanExporter` posts `{"data": [trace|span export]}` to `https://api.openai.com/v1/traces/ingest` with api_key/org/project headers, retry + backoff + sanitization/truncation; `BatchTraceProcessor` batches via queue/ background thread. No generic SIEM (Splunk, ELK, OTLP) exporter ships. | `src/agents/tracing/processors.py:44-235`, `src/agents/tracing/processors.py:541-718` |
| Machine-readable guarantee — export surfaces | Every evidence surface is `dict[str,Any]` / JSON: `Trace.export`, `Span.export`, `SpanData.export`, `RunState.to_json`/`to_string` (bounded JSON validator `_validate_run_state_json_value`), `Usage.serialize_usage`, `BackendSpanExporter._sanitize_json_compatible_value`. | `src/agents/tracing/traces.py:152`, `src/agents/tracing/spans.py:181`, `src/agents/run_state.py:2274-2288`, `src/agents/usage.py:405-431` |
| Retention policy — absent | `grep -R retention` in `src/agents` only hits `ModelSettings.prompt_cache_retention` (`in_memory|24h`, `src/agents/model_settings.py:150`); no `evidence_retention`, `log_retention`, `trace_retention`, `audit_retention` key. `BatchTraceProcessor` only enforces `max_queue_size=8192`, `max_batch_size=128`, `schedule_delay=5.0`, drops on `queue.Full` with `logger.warning("Queue is full, dropping trace/span")` and `shutdown(timeout=5.0)` via `atexit`. | `src/agents/tracing/processors.py:548-605`, `src/agents/tracing/processors.py:603-604`, `src/agents/tracing/processors.py:618-622`, `src/agents/tracing/setup.py:10-22` |
| Reproducibility & recovery — RunState | `RunState` is versioned (`CURRENT_SCHEMA_VERSION="1.17"`, `SCHEMA_VERSION_SUMMARIES` 1.0–1.17) with forward-compatibility fail-fast, `to_json`/`from_json`/`from_string` round-trip, validation via `_validate_run_state_schema_version` / `_build_run_state_from_json`. `result.to_state()` rebuilds a resumable `RunState` from `RunResult`/`RunResultStreaming`. | `src/agents/run_state.py:182-218`, `src/agents/run_state.py:1704-2267`, `src/agents/result.py:541-588` |
| Usage/quantitative evidence | `Usage` dataclass aggregates `requests, input_tokens, output_tokens, total_tokens, input_tokens_details, output_tokens_details, request_usage_entries`; serialized for tracing via `model_usage_to_span_usage`, `total_usage_to_span_metadata`, `turn_usage_to_span_data`. | `src/agents/usage.py:195-229`, `src/agents/usage.py:405-486` |
| Observability side-channel | `logger = logging.getLogger("openai.agents")` (`src/agents/logger.py:7`) and redacting helpers `DONT_LOG_MODEL_DATA/DONT_LOG_TOOL_DATA` (`src/agents/_debug.py` referenced in `processors.py:32`); no structured audit logger with retention. | `src/agents/logger.py:1-266`, `src/agents/tracing/processors.py:31-42` |

## Answers to Dimension Questions

### 1. What evidence does the system produce?

The SDK produces **low-level execution evidence**, not governance evidence:

* **Tracing evidence** — `Trace` and `Span` objects emitted through `TracingProcessor`. `Trace.export()` (`src/agents/tracing/traces.py:380`) yields workflow identity (trace_id, workflow_name, group_id, metadata). `SpanImpl.export()` (`src/agents/tracing/spans.py:396`) yields parent/trace linkage, `started_at`/`ended_at` (ISO-8601 via `src/agents/tracing/spans.py:347` + `src/agents/tracing/provider.py:358`), `error`, and typed `span_data`. Typed span data (`src/agents/tracing/span_data.py:28-451`) covers agent identity, model generations (`GenerationSpanData` with input/output/model/usage), function calls (`FunctionSpanData` with input/output), handoffs, and `GuardrailSpanData(triggered)`.

* **Guardrail/policy outcomes** — `RunResultBase.input_guardrail_results / output_guardrail_results / tool_input_guardrail_results / tool_output_guardrail_results` (`src/agents/result.py:325-333`) hold `GuardrailFunctionOutput.tripwire_triggered` plus `output_info`. These are serialized into `RunState.context.input_guardrail_results` etc. via `_serialize_guardrail_results` (`src/agents/run_state.py:2762`). Tool guardrails serialize `behavior` (`allow`/`reject_content`/`raise_exception`) via `_serialize_tool_guardrail_results` (`src/agents/run_state.py:2790`).

* **Tool execution ledger** — `RunResult.new_items: list[RunItem]` (`src/agents/result.py:314`) with the `RunItem` union (`ToolCallItem`, `ToolCallOutputItem`, `MCP*Item`, `HandoffCallItem`, etc., `src/agents/items.py:686`) plus `ModelResponse.output` (`src/agents/items.py:707`). Function call inputs/outputs appear both in `FunctionSpanData` and in `RunState._serialize_item()` (`src/agents/run_state.py:1933`).

* **Approval ledger** — `RunContextWrapper._approvals` (`src/agents/run_context.py:89`) maps tool identity keys to `approved/rejected` (per-call call_id list or sticky bool) plus rejection messages; `RunState._current_step.interruptions` (`src/agents/run_state.py:826`) holds pending `ToolApprovalItem`s with `raw_item` + resolved tool identity. Both are serialized (`src/agents/run_state.py:1742`, `src/agents/run_state.py:2565`).

* **Quantitative evidence** — `Usage` (`src/agents/usage.py:195`) aggregated per runner and per span.

Absent evidence: **approval timestamps**, actor identity for approvals, control-boundary versioning, chain-of-custody hash, tamper-evident seal. The trace-level `started_at`/`ended_at` provide execution timing but not an explicit `approved_at`/`rejected_at` for human-in-the-loop decisions.

### 2. Is evidence machine-readable?

**Yes, partially — via structured dict/JSON, but without a governance schema.**

* Every evidence surface exports `dict[str, Any]` intended for JSON serialization: `Trace.export()` / `Span.export()` / `SpanData.export()` (`src/agents/tracing/traces.py:152`, `src/agents/tracing/spans.py:181`, `src/agents/tracing/span_data.py:17`), `Trace.to_json()` / `TraceState.to_json()` / `SpanImpl.export()` payloads include `object`, `id`, `trace_id`, `parent_id`, `span_data`, `error`, `metadata` fields. `RunState.to_json()` (`src/agents/run_state.py:1704`) produces a bounded JSON tree validated by `_validate_run_state_json_value` (`src/agents/run_state.py:2274`) and rejected if it contains non-JSON primitives. `BatchTraceProcessor` sanitizes NaN/inf, drops non-serializable values, and truncates fields >100 kB before ingest (`src/agents/tracing/processors.py:261-531`).

* The primary consumer is the hard-coded OpenAI ingest endpoint `BackendSpanExporter._OPENAI_TRACING_INGEST_ENDPOINT = "https://api.openai.com/v1/traces/ingest"` (`src/agents/tracing/processors.py:45`); grouping by `tracing_api_key` (`src/agents/tracing/processors.py:125`) allows routing but the wire format is OpenAI-specific, not OTLP/log-SIEM agnostic. Adding support for generic SIEM schemas requires implementing a custom `TracingProcessor`/`TracingExporter` — the SDK guarantees the **interface** (`src/agents/tracing/processor_interface.py:9`) but ships no SIEM adapter, no OTLP exporter, and no evidence normalizer for SIEM field mapping.

* Consequence: traces/spans and `RunState` JSON are trivially ingestible by a SIEM with a JSON-log or HTTP intake, but there is no curated SIEM mapping (e.g., ECS, CEF, Splunk CIM) and no guarantee that `span_data` field names remain stable beyond the tracing ingest contract.

### 3. Can evidence be reproduced?

**Deterministically, within the lifetime of `RunState` serialization, but not as a turnkey compliance report.**

* **Reproducible:** Any interrupted run can be checkpointed as `RunState` and rehydrated later: `RunResult.to_state()` (`src/agents/result.py:541`) captures `current_turn, generated_items, session_items, model_responses, guardrail results, tool invocations, trace_state, sandbox state`; `RunState.to_json()` / `RunState.from_json()` / `RunState.from_string()` (`src/agents/run_state.py:1704`, `src/agents/run_state.py:2100`, `src/agents/run_state.py:2173`) round-trip with schema version gating (`CURRENT_SCHEMA_VERSION="1.17"`, `src/agents/run_state.py:182`). `reattach_trace()` (`src/agents/tracing/traces.py:390`) can re-establish trace context without duplicate `on_trace_start`. `BatchTraceProcessor.force_flush()` and `DefaultTraceProvider.force_flush()` (`src/agents/tracing/processors.py:647`, `src/agents/tracing/provider.py:498`) allow forcing evidence out at the end of a run, and `trace.export()` / `span.export()` can be snapshotted by a custom `TracingProcessor` into any store.

* **Not reproducible as a report:** There is no `generate_compliance_report(run_id)` or `EvidenceReport` type. A consumer must manually join `fetch_normalized_spans()`-style test helpers (`tests/testing_processor.py` pattern), `RunState.to_json()`, and `RunResult` fields. Replaying a run from `RunState` re-executes model calls (nondeterministic LLM output) unless paired with `ScriptedModel` testing doubles (`tests/model_test_helpers.py`). Tamper evidence is absent: `RunState` is not signed or hash-chained, so a third party cannot verify that evidence was not mutated after capture.

### 4. Are evidence retention policies enforced?

**No.**

* Search of `src/agents/` for `retention`, `evidence_retention`, `trace_retention`, `TTL`, `retention_policy` yields only the unrelated `ModelSettings.prompt_cache_retention` (`src/agents/model_settings.py:150`). No configuration key, env var, or policy object governs how long traces, spans, guardrail logs, or approval records must be retained.

* The only lifecycle controls are **capacity and liveness**, not compliance retention: `BatchTraceProcessor(max_queue_size=8192, max_batch_size=128, schedule_delay=5.0)` (`src/agents/tracing/processors.py:541`) enqueues spans in a bounded `queue.Queue` and **drops** on overflow with `logger.warning("Queue is full, dropping trace/span")` (`src/agents/tracing/processors.py:603`). `BatchTraceProcessor.shutdown(timeout)` (`src/agents/tracing/processors.py:623`) and `setup.py: _DEFAULT_SHUTDOWN_TIMEOUT=5.0` (`src/agents/tracing/setup.py:10`) bound flush time via `atexit` rather than guaranteeing delivery. `RunState` snapshots live wherever the caller stores the `dict`/`str` returned by `to_json()`/`to_string()` — the SDK imposes no expiry, no encrypted-at-rest default, and no audit-trail immutability.

* Operational implication: a high-throughput agent that fills the trace queue silently loses evidence, and an operator who relies on OpenAI's ingest endpoint has no SDK-side retention SLA — evidence persistence is entirely contingent on the external ingest succeeding within 5 s of shutdown and on the caller persisting `RunState`.

## Architectural Decisions

* ** tracing as evidence bus — ** The SDK chose to model all observable behavior as spans under a trace (`src/agents/tracing/provider.py:300`, `src/agents/tracing/create.py:31`), with a processor fan-out (`SynchronousMultiTracingProcessor`, `src/agents/tracing/provider.py:93`) and batched export (`BatchTraceProcessor`, `src/agents/tracing/processors.py:541`). This makes evidence pluggable but couples governance evidence to observability sampling (if tracing is disabled via `OPENAI_AGENTS_DISABLE_TRACING` or `TracingConfig.disabled`, `NoOpTrace`/`NoOpSpan` produce no evidence, `src/agents/tracing/provider.py:386`).

* ** Guardrail results as first-class persisted state — ** Rather than treating guardrail checks as log lines, the SDK persists full `InputGuardrailResult` / `OutputGuardrailResult` / `ToolInput/OutputGuardrailResult` in `RunState` (`src/agents/run_state.py:2762-2812`) and replays them on resume. This is stronger than span-only evidence and enables audit of `tripwire_triggered` + `output_info`.

* ** Approval as mutable map, not event log — **`RunContextWrapper._approvals` is a `dict` of `_ApprovalRecord` mutated in place by `approve_tool()` / `reject_tool()` (`src/agents/run_context.py:1043`), with no append-only ledger. This simplifies resume logic but loses the "who approved what when" event history required for SOX/ISO-style governance.

* ** Schema-versioned `RunState` —** `SCHEMA_VERSION_SUMMARIES` + `CURRENT_SCHEMA_VERSION="1.17"` (`src/agents/run_state.py:182-218`) implements explicit durability for HITL pause/resume, with forward-fail and bounded JSON validation (`src/agents/run_state.py:2274`). Governance evidence inherits this durability (approvals + guardrail outcomes survive restart) but is not separately versioned for compliance audits.

* ** Non-finite/overflow sanitization for ingest —** `BackendSpanExporter._sanitize_for_openai_tracing_api` (`src/agents/tracing/processors.py:261`) and `_truncate_span_field_value` family (`src/agents/tracing/processors.py:355`) truncate at 100 kB and strip `usage` for non-`generation` spans. Good for reliability, but means quantitative evidence can be silently truncated.

## Notable Patterns

* ** Processor interface as SIEM seam —** `TracingProcessor` requires only 6 methods (`src/agents/tracing/processor_interface.py:53-129`); any SIEM (Datadog, Splunk HEC, OTLP) can be integrated by implementing `on_span_end` to `span.export()` and shipping JSON. Tested pattern: `tests/tracing/test_traces_impl.py` and `tests/test_tracing.py:494` demonstrate custom `MetadataPropagatingProcessor`.

* ** Span hierarchy mirrors governance questions —** `task_span` (top-level `Runner.run`), `turn_span(turn, agent_name)` (`src/agents/tracing/create.py:139`), `agent_span`, `generation_span`, `function_span`, `guardrail_span(triggered)` (`src/agents/tracing/create.py:325`) let a SIEM reconstruct "what happened, in what order, under which agent" from `parent_id` chains.

* ** Context-scoped evidence propagation —** `SpanImpl.trace_metadata` routing via `_SPAN_METADATA_ROUTING_KEYS = ("agent_harness_id",)` (`src/agents/tracing/spans.py:16`) is the only governance-flavored control propagated to every span; other governance signals (tenant, approval) must be carried manually via `Trace.metadata`.

* ** Redaction-aware logging —** `logger.py` routes through `DONT_LOG_MODEL_DATA / DONT_LOG_TOOL_DATA` guards (`src/agents/logger.py:90`, `src/agents/_debug.py`) and `_prepare_data_redacted_error` in `run_state` error paths, preventing PII leakage into evidence — but also meaning evidence can be intentionally elided.

## Tradeoffs

* ** Pluggability vs out-of-box governance —** The bare `TracingProcessor`/`TracingExporter` interfaces make SIEM integration trivial to add, but shipping only `BackendSpanExporter` (OpenAI SaaS ingest) means an enterprise that needs on-prem SIEM must write and operate its own exporter, with no reference implementation or integration test in the SDK. Alternative: ship a `FileSpanExporter` / `OTLPExporter` would improve governance posture at cost of extra dependencies.

* ** Bounded queue vs evidence loss —** `BatchTraceProcessor` chooses to drop spans rather than block the agent loop (`queue.put_nowait` + `except queue.Full: warning`, `src/agents/tracing/processors.py:601`). This protects latency but violates governance invariants where evidence must not be dropped. A governance-grade processor would need bounded-back-pressure or persistent spool.

* ** Mutable approvals vs audit trail —** Storing approvals as mutable `_ApprovalRecord.approved/rejected` lists enables quick `is_tool_approved(tool_name, call_id)` checks (`src/agents/run_context.py:612`) and simple resume, but loses the append-only ledger, actor, and timestamp that compliance frameworks expect. An event-sourced ledger would add storage and merge complexity.

* ** JSON dict evidence vs typed governance model —** Evidence is untyped `dict[str, Any]` (e.g., `GuardrailFunctionOutput.output_info: Any`, `FunctionSpanData.output: Any|None`). This is flexible for heterogeneous tools but makes SIEM mapping and compliance assertions fragile (field presence not guaranteed).

* ** 5 s shutdown flush vs guaranteed delivery —** `setup.py:10` (`_DEFAULT_SHUTDOWN_TIMEOUT=5.0`) and `BatchTraceProcessor.shutdown(timeout)` with `random exponential backoff` (`src/agents/tracing/processors.py:158-256`) bound worst-case flush time, which is reasonable for ephemeral jobs but risks losing tail evidence in short-lived lambdas/serverless handlers.

## Failure Modes / Edge Cases

* ** Tracing disabled silently eliminates governance evidence —** `DefaultTraceProvider._disabled` gates both `create_trace` and `create_span` to `NoOpTrace`/`NoOpSpan` (`src/agents/tracing/provider.py:386`, `src/agents/tracing/provider.py:423`). Evidence sinks that rely solely on spans receive nothing, and no warning is propagated to `RunResult`.

* ** Queue overflow drops tail evidence —** Burst tool execution (e.g., parallel function calls) can enqueue >8192 spans quickly; `BatchTraceProcessor.on_span_end` drops without retry or spool (`src/agents/tracing/processors.py:619`). Tests never exercise governance retention under backpressure.

* ** Approval without timestamp breaks recency/audit —** Because `ToolApprovalItem` lacks a timestamp, two approvals with the same `call_id` but different wall-clock times are indistinguishable after `RunState.from_json`, and a "stale approval" cannot be detected without external clocking.

* ** Resumption divergence masked —** `RunState` round-trips guardrail results but the SDK does not re-evaluate guardrails on resume unless explicitly re-triggered; if `output_info` contains time-sensitive policy data (e.g., "policy version 12"), the stale snapshot may be presented as current evidence.

* ** Sanitization truncates quantitative evidence —** `_OPENAI_TRACING_MAX_FIELD_BYTES=100_000` (`src/agents/tracing/processors.py:46`) truncates large `input`/`output` span fields; `_sanitize_generation_usage_for_openai_tracing_api` (`src/agents/tracing/processors.py:448`) drops non-finite usage values. A compliance audit that depends on exact token counts or full tool output may see lossy evidence without an explicit `truncated: true` signal in the default ingest.

* ** Redaction mode elides evidence payload —** `BackendSpanExporter.export` with `DONT_LOG_MODEL_DATA/DONT_LOG_TOOL_DATA` prints only `"Trace/Span data is redacted"` (`src/agents/tracing/processors.py:32`) and `logger.py` suppresses exception `exc_info`; evidence pipelines in redacted deployments contain only IDs/timing, not substance.

* ** Sandbox resume state without envelope validation leaks —** `RunState.to_json` (`src/agents/run_state.py:1833`) and `from_json` (`src/agents/run_state.py:2225`) call `sanitize_run_state_sandbox_mount_authority`; malformed `sandbox` envelope raises `ValueError` and can cause resume to fail, orphaning evidence for that run.

## Future Considerations

* ** Emit a `ComplianceEvidence` artifact —** Add a `RunResult.evidence_report()` / `RunState.evidence_report()` that composes `trace_id, workflow_name, group_id, started_at/ended_at, input_guardrail_results, output_guardrail_results, tool_input/output guardrail behaviors, ToolApprovalItem decisions with resolved identity, model usage totals, and span error summary` into a single versioned JSON schema — the direct answer to "Can the system produce a compliance report for a given run?"

* ** Add approval event ledger —** Replace or supplement the mutable `_Approvals` dict with an append-only `ApprovalEvent{call_id, tool_name/namespace/qualified_name, decision: approved|rejected, decision_scope: call|sticky, actor: str|None, at: ISO8601, message}` log serialized alongside the map. Preserve backing map for fast lookups but make ledger the audit source.

* ** Ship a SIEM reference exporter —** Provide `FileTracingExporter(path, rotation)` and/or `OTLPTracingExporter(endpoint)` in `src/agents/tracing/processors.py` alongside `BackendSpanExporter`, with an integration test in `tests/tracing/` exercising ECS-style `agent_harness_id` routing (`src/agents/tracing/spans.py:407`).

* ** Retention controls —** Add `TracingConfig.retention: {mode: memory|file|external, ttl: Duration, on_overflow: block|spool|drop}` and enforce it in `BatchTraceProcessor` (spool to file before dropping). Surface `queue.Full` as a typed `EvidenceDroppedError` that surfaces in `RunResult` instead of only a logger warning.

* ** Tamper evidence —** Chain `RunState.to_json` output with `hash(prev_snapshot) + ed25519 sign(snapshot)` so downstream SIEM/compliance verifiers can detect post-hoc mutation; store hash in `TraceState.metadata`.

* ** Tool execution receipts —** Include `started_at/ended_at/duration_ms, status: success|error, error.code` in `ToolCallOutputItem` serialization (currently only `raw_item` + `output`, `src/agents/items.py:430`), and emit `FunctionSpanData` even when `function_span` is cancelled, so every tool invocation has a receipt independent of tracing being enabled.

## Questions / Gaps

* ** Approval actor —** No `approved_by: user_id|service_principal` field on `ToolApprovalItem` / `_ApprovalRecord`. Is human-in-the-loop identity expected to be carried in `Trace.metadata` or `context`? Search for `actor`/`principal`/`approved_by` returns no evidence in `src/agents/`.

* ** Evidence retention SLA —** What is the intended durability of evidence when `BatchTraceProcessor` drops spans or `shutdown(timeout=5.0)` expires? The SDK appears to treat tracing as best-effort observability, not mandatory governance — should be documented as such.

* ** No spec for compliance report —** The dimension asks "Can the system produce a compliance report?" — there is no `reports/` generator in the source. Confirm with maintainers whether `RunState.to_json() + trace.export()` is intended to be the compliance report, or whether a dedicated report is out of scope for the SDK.

* ** Guardrail `output_info` schema —** `GuardrailFunctionOutput.output_info: Any` (`src/agents/guardrail.py:23`) is free-form; without a policy-decision schema, SIEM consumers cannot reliably assert "what policy version, what rule, what threshold" triggered a tripwire.

* ** MCP approval vs function approval evidence divergence —** `RunContextWrapper` treats `HostedMCPApprovalKey` separately from `FunctionToolLookupKey` (`src/agents/run_context.py:725`). Whether governance consolidates both into a single approval audit view is unspecified.

---

Generated by `Dimension 09.04: Governance Evidence Generation` against `openai-agents-sdk`.
