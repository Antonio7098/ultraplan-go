# Source Analysis: openai-agents-sdk

## Dimension 10.01: Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, httpx; OpenAI platform integration) |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents SDK ships a first-party tracing subsystem (`src/agents/tracing/`) that builds a single coherent trace tree per `Runner` invocation. A run is enclosed in a `Trace` context manager (`src/agents/run.py:736-745` via `TraceCtxManager`, `src/agents/tracing/context.py:91-132`), and every execution step emits a typed span under it: a `task_span` per runner invocation, an `agent_span` per agent, a `turn_span` per model turn, `response_span`/`generation_span` per LLM call, `function_span` per tool call, `guardrail_span` per guardrail evaluation, `handoff_span` per handoff, and MCP list-tools spans. Parent-child relationships are resolved from Python `contextvars` at span creation time (`src/agents/tracing/scope.py:11-17`, `src/agents/tracing/provider.py:433-481`), so concurrency and nested execution (handoffs, `Agent.as_tool()` subagent runs) nest into the same tree automatically. Spans carry `trace_id` + `parent_id` in their exported payload, producing a flat but fully reconstructable tree.

Cross-process propagation exists only through serialized `RunState`: interrupted runs persist a `TraceState` (trace id, workflow name, group id, metadata, hashed tracing key) and resumed runs "reattach" to the same trace without emitting a duplicate start event. There is no W3C `traceparent`-style header propagation to arbitrary downstream services. Export goes through pluggable `TracingProcessor`s; the default `BatchTraceProcessor` buffers spans in a bounded queue and posts them to the OpenAI traces ingest endpoint for the hosted Traces dashboard.

## Rating

**Score: 9/10**

Rationale: The model is explicit and complete — every step class the dimension asks about (runs, turns, model calls, tools, guardrails, handoffs) has a dedicated span type with tests proving parent-before-child ordering (`tests/tracing/test_span_ordering.py:44`), noop-parent propagation (`tests/test_tracing.py:410`), and error attachment (`tests/test_tracing_errors.py`). Operational safeguards are unusually deep: processor exceptions are non-fatal (`src/agents/tracing/provider.py:117-175`), exporter failures cannot kill the batch worker (`src/agents/tracing/processors.py:696-710`), shutdown is deadline-aware (`src/agents/tracing/processors.py:623-645`), abandoned async generators are tolerated (`src/agents/tracing/spans.py:31-56`), and sensitive-data redaction is enforced at the span-payload level (`src/agents/models/_trace.py:9-31`, `src/agents/util/_error_tracing.py:70-86`). It falls short of 10 because cross-process/cross-service propagation is bespoke (`RunState` serialization plus `group_id` correlation) rather than standards-based, spans are silently dropped when the export queue fills, and Realtime server traces form a separate tree that only correlates by `group_id` rather than nesting.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tracing system | First-party trace/span subsystem with provider, processors, exporters | `src/agents/tracing/__init__.py:1-130` |
| Trace root object | `TraceImpl` emits `on_trace_start`/`on_trace_end`, exports `{object: "trace", id, workflow_name, group_id, metadata}` | `src/agents/tracing/traces.py:486-575` |
| Run encloses trace | `Runner._run_impl` wraps the whole loop in `TraceCtxManager(...)` | `src/agents/run.py:736-745` |
| Trace creation policy | `create_trace_for_run` skips creation when a trace is already current (nesting) | `src/agents/tracing/context.py:58-88` |
| Task span | One `task_span(name=trace_workflow_name)` per top-level runner invocation | `src/agents/run_internal/run_loop.py:905-910` |
| Turn span | `turn_span(turn=current_turn, agent_name=...)` started/finished around each streamed turn with usage delta attached | `src/agents/run_internal/run_loop.py:1737-1784` |
| Agent span | `agent_span(name, handoffs, tools, output_type)` created once per agent, marked current; handoff targets recorded into `span_data.handoffs` later | `src/agents/run_internal/run_loop.py:1528-1534`, `src/agents/run_internal/run_loop.py:2108-2109` |
| Model call spans | `with response_span(...)` around Responses API calls; generation spans in Chat Completions path | `src/agents/models/openai_responses.py:573-606`, `src/agents/models/openai_chatcompletions.py:238-250` |
| Tool function spans | `with_tool_function_span` wraps tool callbacks in `function_span(tool_name)` unless disabled/no trace | `src/agents/run_internal/tool_execution.py:1120-1139` |
| Guardrail spans | Input/output guardrails each wrapped in `guardrail_span`; tripwire attaches `SpanError` to parent/current span | `src/agents/run_internal/guardrails.py:37-40`, `src/agents/run_internal/guardrails.py:49-52`, `src/agents/run_internal/guardrails.py:85-97` |
| Handoff spans | `with handoff_span(from_agent=...)` wraps handoff execution; `to_agent` set after resolution | `src/agents/run_internal/turn_resolution.py:578-597` |
| MCP spans | `mcp_tools_span(server=...)` wraps MCP tool listing | `src/agents/mcp/util.py:348` |
| Span types | 13 concrete `SpanData` classes: agent, task, turn, function, generation, response, handoff, custom, guardrail, transcription, speech, speech_group, mcp_tools | `src/agents/tracing/span_data.py:28-451` |
| Parent-child linkage | `SpanImpl` stores `_trace_id`, `_span_id`, `_parent_id`; export includes all three | `src/agents/tracing/spans.py:304-340`, `src/agents/tracing/spans.py:396-406` |
| Parent resolution | `DefaultTraceProvider.create_span`: explicit `parent` arg, else current span/trace from contextvars; no-op parents propagate as no-op children | `src/agents/tracing/provider.py:433-481` |
| Context mechanism | `_current_span` / `_current_trace` `contextvars.ContextVar`s with token-based set/reset | `src/agents/tracing/scope.py:11-49` |
| Subagent nesting | `Agent.as_tool()` invokes `Runner.run`/`run_streamed` inside the outer tool span; inner run reuses the ambient trace | `src/agents/agent.py:889-904`, `src/agents/agent.py:961-969`, `src/agents/tracing/context.py:59-61` |
| Trace ID format | `trace_<uuid4 hex>`, `span_<24 hex>`, `group_<24 hex>` generators | `src/agents/tracing/provider.py:362-372` |
| Cross-process resume | `RunState.set_trace` snapshots `TraceState.from_trace(trace)`; `to_json` persists it | `src/agents/run_state.py:2066-2073` |
| Reattachment on resume | `reattach_trace` rebuilds trace context without re-emitting start; guarded by `_trace_id_was_started` + settings match | `src/agents/tracing/traces.py:305-404`, `src/agents/tracing/context.py:63-79` |
| Resume wiring | `is_resumed_state` passes stored `trace_state` into `TraceCtxManager` with `reattach_resumed_trace=True` | `src/agents/run.py:743-747` |
| No HTTP header propagation | Search for `traceparent`/W3C headers found nothing in `src/` (only unrelated matches) | search boundary noted below |
| Default exporter pipeline | `BatchTraceProcessor` (bounded queue 8192, batch 128, 5s delay) → `BackendSpanExporter` posting to `https://api.openai.com/v1/traces/ingest` | `src/agents/tracing/processors.py:541-608`, `src/agents/tracing/processors.py:44-47` |
| Processor isolation | Multi-processor fan-out catches and logs per-processor exceptions | `src/agents/tracing/provider.py:117-175` |
| Worker resilience | Exporter exception caught so batch worker thread survives | `src/agents/tracing/processors.py:696-710` |
| Force flush | Public `flush_traces()` → provider `force_flush()` for immediate delivery | `src/agents/tracing/__init__.py:122-130` |
| Error-on-span helpers | `attach_error_to_span`/`attach_error_to_current_span`/`model_span_errors` record failures with redaction control | `src/agents/util/_error_tracing.py:56-157` |
| Sensitive data gating | `ModelTracing.ENABLED_WITHOUT_DATA` when `trace_include_sensitive_data=False`; URL sanitizer strips auth/query params | `src/agents/tracing/model_tracing.py:6-14`, `src/agents/models/_trace.py:9-31` |
| Compact hierarchy option | `TracingConfig.include_task_and_turn_spans` disables task/turn spans for a flatter tree | `src/agents/tracing/config.py:12-18`, `docs/tracing.md:47-55` |
| Viewer | OpenAI Traces dashboard is the default viewer; third-party processors listed (Braintrust, LangSmith, Langfuse, etc.) | `docs/tracing.md:3`, `docs/tracing.md:197-220` |
| Ordering test | Parent always ordered before child even on tied timestamps; normalized tree nests correctly | `tests/tracing/test_span_ordering.py:44-74` |
| Metadata propagation test | Trace metadata propagates to direct and nested child spans | `tests/test_tracing.py:433-450` |
| Disabled-trace test | Disabled parent trace yields no-op child spans end-to-end | `tests/test_tracing.py:410-423`, `tests/test_agent_tracing.py:630` |
| Slow-guardrail timing test | Parent span and trace finish after slow input guardrail completes | `tests/test_stream_input_guardrail_timing.py:206-240` |
| Maintainer contract | Documented invariants: context ownership, no duplicate trace-start on resume, non-fatal processor errors | `.agents/references/tracing-lifecycle.md:5-27` |

## Answers to Dimension Questions

**1. Is there a single coherent trace tree?**
Yes. Every `Runner.run/run_streamed/run_sync` invocation opens exactly one `Trace` (`src/agents/run.py:736-745`) and every span created during the run resolves its parent from the ambient contextvars chain (`src/agents/tracing/provider.py:433-457`), so all spans share one `trace_id` and link to a parent. Multiple sequential runs can be merged into one tree by wrapping caller code in an explicit `with trace(...)` (`docs/tracing.md:104-121`); conversely, creating a nested trace triggers a warning that it is "probably a mistake" (`src/agents/tracing/create.py:63-67`).

**2. Are all execution steps represented?**
Runs, turns, agents, model calls, tool calls, guardrail evaluations, and MCP listings all have dedicated span types wired into the runtime (`src/agents/run_internal/run_loop.py:905-910,1528-1534,1737-1746`; `src/agents/run_internal/tool_execution.py:1120-1139`; `src/agents/run_internal/guardrails.py:37-52`; `src/agents/mcp/util.py:348`). Two gaps: there is **no retrieval/RAG span type** and **no eval span type** in `src/agents/tracing/span_data.py:28-451` — retrieval would have to use `custom_span`. Usage accounting is folded into turn/task spans and generation/response spans rather than being separate events.

**3. Do handoffs and subagent calls nest correctly?**
Handoffs: yes — the handoff executes inside a `handoff_span` within the same trace (`src/agents/run_internal/turn_resolution.py:578-597`) and the subsequent turns of the receiving agent continue under the same task/trace, with the new agent getting a fresh `agent_span` (`src/agents/run_internal/run_loop.py:1528-1534`). Subagents: yes — `Agent.as_tool()` runs the child agent via `Runner.run` inside the parent's `function_span`; since the child run finds an existing current trace, `create_trace_for_run` returns `None` and the child's spans attach to the ambient tree (`src/agents/tracing/context.py:59-61`, `src/agents/agent.py:961-969`). Realtime sessions are the exception: server-side Realtime traces are a separate tree correlated only by `group_id`, explicitly documented as not merged (`.agents/references/realtime-tracing.md:16`).

**4. Can you follow a request from start to finish?**
Within one process, yes: trace → task span → turn span → agent span → response/generation span → function/guardrail/handoff spans, all sharing `trace_id` with `parent_id` edges, viewable in the OpenAI Traces dashboard or any custom processor (`src/agents/tracing/spans.py:396-406`, `docs/tracing.md:15-55`). Across process boundaries you can follow a *resumed* run because `TraceState` is serialized into `RunState` and reattached (`src/agents/run_state.py:2066-2073`, `src/agents/tracing/context.py:63-79`), but you cannot follow a request into arbitrary downstream HTTP services — the SDK emits no standard distributed-tracing headers (searched `traceparent`, W3C, propagat* across `src/`; no hits in tracing code).

## Architectural Decisions

1. **Contextvars-based implicit parenting over explicit span passing.** The current trace/span live in module-level `ContextVar`s (`src/agents/tracing/scope.py:11-17`); token-based set/reset guarantees correct unwinding even across async tasks. Explicit `parent=` overrides exist for advanced cases (`src/agents/tracing/create.py:95`).
2. **Flat wire format with client-reconstructable hierarchy.** Spans are exported individually carrying `(trace_id, parent_id)` (`src/agents/tracing/spans.py:396-406`); the viewer rebuilds the tree. Ordering ambiguity on tied timestamps is handled and tested (`tests/tracing/test_span_ordering.py:30-41`).
3. **Processor plug-in architecture.** All consumption goes through the `TracingProcessor` interface (`src/agents/tracing/processor_interface.py:54-121`); the OpenAI backend is just the default consumer (`add_trace_processor` vs `set_trace_processors`, `src/agents/tracing/__init__.py:94-105`).
4. **No-op objects preserve semantics when tracing is off.** `NoOpTrace`/`NoOpSpan` keep context management working while exporting nothing, and no-op parents deterministically produce no-op children (`src/agents/tracing/provider.py:442-451`, `tests/test_tracing.py:410-423`).
5. **Resume = reattach, not replay.** Interrupted runs persist trace identity in `RunState` and reattach without a second `on_trace_start`, verified against a previously-started-trace-id LRU cache and full settings match (`src/agents/tracing/traces.py:280-302`, `src/agents/tracing/context.py:20-79`).
6. **Task/turn spans are opt-out.** A config flag flattens the tree when users find task+turn nesting too verbose (`src/agents/tracing/config.py:12-18`).

## Notable Patterns

- **Typed span payloads**: each step kind maps to a `SpanData` subclass with a stable `type` discriminator (`"agent"`, `"generation"`, `"handoff"`, ...), enabling schema-driven rendering (`src/agents/tracing/span_data.py:50-52,155-157,256-258`).
- **Usage attribution on spans**: turn/task spans capture usage deltas via `snapshot_usage`/`usage_delta` around the turn (`src/agents/run_internal/run_loop.py:911,1736,1779-1784`), and generation/response spans carry usage dicts.
- **Error surfacing on the tree**: failures mutate spans rather than relying on exception transport — max-turn violations annotate the agent span (`src/agents/run_internal/run_loop.py:1543-1549`), tripwires annotate the guardrail's parent (`src/agents/run_internal/guardrails.py:85-97`), and model errors are recorded best-effort with redaction fallback (`src/agents/util/_error_tracing.py:89-123`).
- **GeneratorExit tolerance**: abandoned async generators finalize spans/traces from a foreign task without crashing, using token-reset `ValueError` tolerance (`src/agents/tracing/spans.py:31-56`, `src/agents/tracing/traces.py:18-48`).
- **Routing metadata allow-list**: only `agent_harness_id` from trace metadata is copied into span export envelopes, a deliberate privacy/routing split (`src/agents/tracing/spans.py:16`, `src/agents/tracing/spans.py:407-422`).

## Tradeoffs

- **Implicit context vs. explicit plumbing**: contextvars make zero-touch nesting possible but tie the tree to a single event-loop/process lineage; anything outside the context (thread pools without context copy, other services) silently becomes a rootless `NoOpSpan` (`src/agents/tracing/provider.py:436-441` logs debug guidance).
- **OpenAI-flavored format over open standards**: IDs (`trace_...`), the ingest endpoint, and the dashboard are cohesive but not W3C-trace-context compatible; adopting OTel requires a custom processor (`docs/tracing.md:155-158`).
- **Bounded queue drop policy**: backpressure drops spans with only a warning (`src/agents/tracing/processors.py:603-604,618-621`), trading completeness for producer non-blocking.
- **Verbose-by-default hierarchy**: task + agent + turn + generation spans per model turn is deep; the opt-out flag mitigates but splits what users can expect in a given trace (`docs/tracing.md:47-55`).

## Failure Modes / Edge Cases

- **Queue overflow loses spans silently-ish** (warning log only) — long high-volume runs may export partial trees (`src/agents/tracing/processors.py:618-621`).
- **Foreign-context resets**: a span finished from a different asyncio context keeps itself "current" in the original task until that scope ends; deliberately tolerated, logged at debug (`src/agents/tracing/spans.py:34-46`).
- **Nested trace warning**: calling `trace()` while a trace is current creates a sibling tree anyway — user error yields fragmented trees rather than an error (`src/agents/tracing/create.py:63-67`).
- **Early streamed guardrails before agent span exists**: tripwire errors fall back to `attach_error_to_current_span` (`src/agents/run_internal/guardrails.py:92-96`), which can land the error on the task span instead of an agent span — correct but position varies.
- **Realtime split-brain**: enabling both client and server tracing produces two dashboards traces linked only by `group_id`; consumers expecting one tree will be surprised (`.agents/references/realtime-tracing.md:11-16`).
- **Key handling on resume**: raw tracing API keys are omitted from serialized state by default; only a SHA-256 fingerprint is kept, and mismatched keys on resume force a fresh trace instead of reattachment (`src/agents/tracing/traces.py:187-192`, `src/agents/tracing/context.py:37-44`).

## Future Considerations

- Add first-class retrieval/RAG and eval span types to close the coverage gap versus the dimension's ideal step inventory (currently requires `custom_span`).
- Emit optional W3C `traceparent`/`tracestate` headers from outbound model/MCP requests so SDK traces can correlate with downstream service traces.
- Consider an overflow policy beyond drop-plus-warning (e.g., counters of dropped spans exported as synthetic spans) to preserve visibility under overload.
- Unify or more strongly bridge Realtime server traces with client run trees beyond `group_id`.

## Questions / Gaps

- **Distributed tracing headers**: none found in the selected source (searched `traceparent`, `trace-state`, `W3C`, `propagat` across `src/agents/`). If cross-service propagation exists, it lives outside this repository (e.g., in OpenAI backend ingestion), so it could not be verified here.
- **Eval tracing**: the repository contains no eval-span machinery; whether eval runs appear in traces depends on external eval products, not this SDK.
- **Viewer implementation**: the Traces dashboard is a hosted service; only the ingest payload shape (`export()` methods cited above) and third-party processor integrations (`docs/tracing.md:211-220`) are verifiable in-tree.

---

Generated by `Dimension 10.01: Span Hierarchy and Run Tree` against `openai-agents-sdk`.
