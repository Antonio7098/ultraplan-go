# Source Analysis: pydantic-ai

## 10.01 Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-ai-slim agent framework, pydantic-graph execution engine, pydantic-evals framework; OpenTelemetry + Logfire) |
| Analyzed | 2026-08-24 |

## Summary

Pydantic AI implements span hierarchy as an opt-in but first-class subsystem built directly on OpenTelemetry GenAI semantic conventions. The tracing system has three layers: (1) the `Instrumentation` capability (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:68`) opens the root `invoke_agent {name}` run span and `execute_tool {name}` tool/output spans; (2) `InstrumentedModel` / the shared `open_model_request_span` helper (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:332`, `pydantic_ai_slim/pydantic_ai/_instrumentation.py:444`) opens per-request `chat {model}` CLIENT spans carrying full input/output message payloads; (3) `pydantic_graph` provides a fallback `run graph`/`run node` Logfire-span layer for standalone graphs (`pydantic_graph/pydantic_graph/graph_builder.py:345`, `graph_builder.py:903`) plus W3C traceparent extraction used across the codebase.

The hierarchy is coherent by construction: the capability declares itself `'outermost'` (`capabilities/instrumentation.py:116-117`), agent graphs disable the graph-level auto-instrumentation to avoid a duplicate root (`_agent_graph.py:2587`), and OTel context propagation makes every model request, tool call, user logfire span, and delegate-agent run nest under one `invoke_agent` span. The tree shape is pinned by snapshot tests that reconstruct parent-child relationships from exported spans (`tests/test_logfire.py:50-92`, `tests/test_logfire.py:168-190`). Trace IDs propagate across process boundaries through explicit W3C traceparent capture on run results (`pydantic_ai_slim/pydantic_ai/run.py:110-121`, `run.py:604-613`), OTel baggage (`_instrumentation.py:35-37`, `capabilities/instrumentation.py:187-190`), Temporal's `TracingInterceptor` (`durable_exec/temporal/_logfire.py:57-60`), and WebSocket header injection for realtime sessions (`realtime/google.py:609-618`).

The result is a single trace in which you can follow a request from user prompt → `chat` span (with serialized messages, usage, cost, TTFT) → `execute_tool` span (arguments, results, deferral metadata) → nested sub-agent runs — with operational safeguards such as content redaction, sampling-aware metric gating, and stale-cache detection.

## Rating

**Score: 9/10**

Rationale against the rubric ("Mature, durable, observable, extensible, and proven under failure or scale"):

- **Clear model**: a documented three-layer span taxonomy (`invoke_agent` → `chat`/`execute_tool` → arbitrary user/instrumented-library spans), versioned data formats (`InstrumentationSettings.version` 2–6, `models/instrumented.py:79` and docstring at lines 117–135).
- **Explicit interfaces**: public `InstrumentationSettings`, `instrument_model()`, and the `Instrumentation` capability; other capabilities are invited to annotate the current span via `get_current_span()` (`capabilities/instrumentation.py:74-76`).
- **Tests**: ~113 instrumentation-focused tests across `tests/test_logfire.py` (59 tests) and `tests/models/test_instrumented.py` (54 tests), asserting exact span trees, parent links, attributes, streaming TTFT, error statuses, and not-recording behavior.
- **Proven under failure**: dedicated handling of tool retries, deferrals-as-control-flow (`capabilities/instrumentation.py:452-479`), validation-failure error spans (`on_tool_validate_error`, line 325), non-recording spans (sampling), lone-surrogate serialization crashes, cross-task context re-attachment (`capture_current_context`, `_instrumentation.py:547-577`), and cancellation mid-stream.
- Not a 10 because: there is no dedicated span type for output validators/guardrail evaluations (they surface indirectly as retry prompts inside message attributes), and cross-process propagation relies on integration-specific mechanisms (Temporal interceptor, realtime headers) rather than one uniform distributed-tracing story; also no evidence of a first-class "trace viewer" beyond Logfire screenshots/docs.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tracing provider abstraction | `InstrumentationSettings.__init__` resolves `tracer_provider or get_tracer_provider()`, scope name `'pydantic-ai'`; docs state global provider is typically set by `logfire.configure()` | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:145-149` |
| Tracer acquisition on settings | `self.tracer = tracer_provider.get_tracer(scope_name, __version__)` | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:148` |
| Agent run span | `wrap_run` opens `invoke_agent {agent_name}` via `settings.tracer.start_as_current_span(...)` with `gen_ai.operation.name='invoke_agent'`, `gen_ai.agent.call.id` (run id), `gen_ai.conversation.id` | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:152-186` |
| Capability ordering = outermost | `get_ordering()` returns `CapabilityOrdering(position='outermost')` so the run span wraps everything else | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:116-117` |
| Run-span end attributes | `_run_span_end_attributes` sets cumulative usage (`gen_ai.aggregated_usage.*`), `pydantic_ai.all_messages`, `pydantic_ai.new_message_index`, `metadata`, `final_result` | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:236-277` and `196-203` |
| Model request span | `open_model_request_span` creates `chat {model}` CLIENT span with `gen_ai.operation.name='chat'`, provider/model attrs, `gen_ai.tool.definitions`, model settings mapped to `gen_ai.request.*` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:444-545` (span creation at 495) |
| Message payload attributes | `handle_messages` writes `gen_ai.input.messages`, `gen_ai.output.messages`, `gen_ai.system_instructions`, and `logfire.json_schema` onto the chat span | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:253-289` |
| Tool execution span | `wrap_tool_execute` → `_run_tool_span` opens `execute_tool {tool_name}` with `gen_ai.tool.name`, `gen_ai.tool.call.id`, arguments/result attributes | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:499-522`, `418-497`, `383-416` |
| Output-function span | `wrap_output_process` emits `execute_tool {name}` for output functions only when a function executes; plain validation explicitly "not span-worthy" | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:528-591` |
| Validation-failure error span | `on_tool_validate_error` emits an `execute_tool` span marked `pydantic_ai.tool.failure_stage: 'validation'`, records escaped exception, re-raises | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:325-381` |
| Deferral semantics | `CallDeferred`/`ApprovalRequired` set `pydantic_ai.tool.deferral.name`/`metadata` attrs; ERROR status only for versions < 5 (v5+ treats deferrals as control flow, not errors) | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:452-471`; format history at `models/instrumented.py:129-131` |
| Parent-child nesting proven by test | `LogfireSummary` rebuilds the tree from `span['parent']` ids; snapshot shows `invoke_agent my_agent` containing `toolset_enter`, two `chat test`, `execute_tool my_ret` (with user's `toolset_call_tool my_ret` nested inside), `toolset_exit` | `tests/test_logfire.py:50-92`, `168-190` |
| Streaming path nesting | `test_run_stream_sync` asserts `chat test` is "correctly nested under the agent run span (not orphaned)" even though the stream runs via a handler task | `tests/test_logfire.py:3297-3320` |
| Cross-task OTel context stitching | `capture_current_context()` snapshots OTel context so continuation segments opened in the consumer task update the right `chat` span; rationale references #6569 | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:547-577` |
| TTFT propagation between tasks | `time_to_first_chunk_ctx` ContextVar carries streaming time-to-first-chunk from the graph's streaming handler to `wrap_model_request`'s `finish()`; attribute `gen_ai.client.operation.time_to_first_chunk` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:78-87`; set at `_agent_graph.py:1194-1214`; read at `capabilities/instrumentation.py:315-318` |
| No duplicate graph root | `build_agent_graph` uses `auto_instrument=False` so only `invoke_agent` roots the trace (no `run graph` span); standalone graphs do emit it | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2581-2588`; graph auto-instrumentation at `pydantic_graph/pydantic_graph/graph_builder.py:342-353` and node spans at `901-908` |
| Traceparent extraction utility | `get_traceparent(span)` injects W3C traceparent via `TraceContextTextMapPropagator`; graceful None when OTel absent | `pydantic_graph/pydantic_graph/_utils.py:24-50` |
| Traceparent on run objects | `AgentRun._traceparent` falls back to `current_otel_traceparent()` (active span) then raises if required and missing; `AgentRunResult` stores `_traceparent_value` for post-run use | `pydantic_ai_slim/pydantic_ai/run.py:110-121`, `604-613`; fallback helper at `_instrumentation.py:643-654` |
| Feedback attached to same trace | `test_feedback` asserts `get_traceparent(result)` equals the run's traceparent (`00-...01-...0001-01`) and the recorded feedback span lands with `parent: {trace_id: 1, span_id: 1, is_remote: True}` | `tests/test_logfire.py:1289-1401` |
| Baggage correlation keys | `AGENT_NAME_BAGGAGE_KEY='gen_ai.agent.name'`, `RUN_ID_BAGGAGE_KEY='gen_ai.agent.call.id'`, `CONVERSATION_ID_BAGGAGE_KEY='gen_ai.conversation.id'`; set around the run span, read back into every child span via `get_agent_run_baggage_attributes()` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:35-37`, `123-135`; set at `capabilities/instrumentation.py:187-191` |
| Distributed tracing over WebSocket | Realtime Google adapter injects `traceparent` header to propagate trace context to server/gateway | `pydantic_ai_slim/pydantic_ai/realtime/google.py:609-618` |
| Realtime session tree | `SessionInstrumentation` owns session-wide `invoke_agent` span (marked `pydantic_ai.realtime`), per-response `chat` spans, lifecycle spans (`model turn complete`, barge-in, `user speech`, `speak`), all parented to the session span context | `pydantic_ai_slim/pydantic_ai/realtime/_instrumentation.py:93-235`, `237-289`, `410-451` |
| Realtime/classic deduplication | `Instrumentation.wrap_run` skips opening a second run span when `ctx.realtime` to avoid duplicating the session's canonical span; realtime tool spans carry the same marker | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:158-162`, `509-513` |
| Cross-process (Temporal) | `LogfirePlugin` attaches temporalio `TracingInterceptor` so workflow/activity boundaries propagate trace context; auto-configures logfire + `instrument_pydantic_ai()` only if host hasn't | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_logfire.py:40-79` |
| Eval-time span trees | `pydantic_evals.otel` provides in-memory exporter + `SpanTree`/`SpanNode`/`SpanQuery` so evaluators can query the trace subtree (ancestors/descendants/status) of each evaluated case | `pydantic_evals/pydantic_evals/otel/span_tree.py:26-120`; exporter wiring at `_context_in_memory_span_exporter.py:136-169` |
| Online evals reference spans | Online evaluation extracts `SpanReference(trace_id, span_id)` from the active span for later scoring | `pydantic_evals/pydantic_evals/online.py:807-835` |
| Metrics avoid double-counting | Token/cost metrics recorded after span close; run-span usage remapped to `gen_ai.aggregated_usage.*` unless disabled | `_instrumentation.py:493-500`, `542-544`; `models/instrumented.py:300-311` |
| Sampling resilience | `finish()` computes price before `is_recording()` gate so cost metrics survive dropped spans; separate `test_instrumented_model_not_recording` | `_instrumentation.py:520-525`; test at `tests/models/test_instrumented.py:303-307` |
| Content redaction safeguards | `redact_binary_content` walks containers/`ToolReturn`/`DeferredToolRequests` stripping binary data under `include_binary_content=False`; `include_content=False` omits messages/args/results | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:141-217`; flags at `models/instrumented.py:76-77` |
| Staleness detection | `has_stale_message_json` detects in-place history mutation that would make recorded `gen_ai.input.messages` wrong, warning at run end | `_instrumentation.py:256-281`; warning emitted at `capabilities/instrumentation.py:224-234` |
| Multi-agent naming guidance | Docs recommend passing `name=` so parent/delegate run spans are distinguishable in Logfire; delegation traces described under "Tracing Agent Delegation" | `docs/multi-agent-applications.md:71`, `380-412` |
| Viewer/UI story | Logfire trace view renders run tree; SQL monitoring examples; otel-tui visualisation mentioned | `docs/logfire.md:22-27`, `82-108`, `188` |
| Fallback-model annotation on existing span | `FallbackModel` updates chat-span attributes via `get_current_span()`; merged in `finish()` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:506-508` |

## Answers to Dimension Questions

**1. Is there a single coherent trace tree?**
Yes. All spans are created through one tracer obtained from a single `TracerProvider` (`models/instrumented.py:145-148`) using `start_as_current_span`/explicit contexts, so parentage follows OTel context. The `Instrumentation` capability positions its run span `'outermost'` (`capabilities/instrumentation.py:116-117`) and agent graphs suppress their own root span (`_agent_graph.py:2587`), yielding exactly one root per run. Snapshot tests verify the reconstructed tree — including third-party logfire spans from a wrapper toolset landing inside the run tree in causal order (`tests/test_logfire.py:168-190`). Edge cases where tasks differ (streaming handler vs consumer task) are explicitly stitched via captured OTel context (`_instrumentation.py:547-577`) and verified by `test_run_stream_sync` (`tests/test_logfire.py:3310-3320`).

**2. Are all execution steps represented?**
Runs, turns (each model request is a `chat` span), tool calls (`execute_tool`), output functions (`execute_tool` on output), and realtime turns/lifecycle moments are represented (`capabilities/instrumentation.py:152-186`, `283-319`, `499-522`, `528-591`; `realtime/_instrumentation.py:237-289`, `428-451`). Guardrails specifically: pydantic-ai has no separate guardrail span concept. Tool argument-validation failures *do* get a dedicated error span (`on_tool_validate_error`, line 325), and retries are visible as `retry-prompt` parts serialized into `gen_ai.input.messages`/output messages (`tests/models/test_instrumented.py:166-268` shows RetryPromptParts rendered as `tool_call_response`/text parts), but output-validator executions are deliberately "not span-worthy" (`capabilities/instrumentation.py:537-545`) — they're observable only through the resulting message content. Retrieval appears only if implemented as tools or via httpx instrumentation; there are no retrieval-specific span types.

**3. Do handoffs and subagent calls nest correctly?**
Yes for delegation (the dominant multi-agent pattern): a delegate agent run started inside a parent's tool inherits the ambient OTel context, so its `invoke_agent` span nests under the parent's `execute_tool` span automatically. The design leans on this: docs instruct naming agents so their spans are distinguishable (`docs/multi-agent-applications.md:71`) and describe end-to-end multi-agent tracing (`docs/multi-agent-applications.md:380-412`). Programmatic hand-off (successive top-level runs) produces sibling traces rather than one tree — appropriate, since control passes through application code. Usage aggregation ties them together economically via shared `usage=ctx.usage` (`docs/multi-agent-applications.md:20`). Caveat: within Temporal activities, `ctx.usage` does not flow back and run context crosses a serialization boundary (`docs/multi-agent-applications.md:84-85`), so delegation nesting there depends on the Temporal tracing interceptor rather than in-process context.

**4. Can you follow a request from start to finish?**
Yes. One `invoke_agent` span carries conversation id, run id (baggage-propagated to every child), final output, aggregated token usage and cost (`capabilities/instrumentation.py:168-203`, `236-277`); each turn's `chat` span carries full input/output messages, system instructions, finish reasons, response id, per-request usage/cost, and streaming TTFT (`_instrumentation.py:399-420`, `464-539`; `models/instrumented.py:269-289`); each tool span carries call id, arguments, and result (`capabilities/instrumentation.py:383-416`). After the run ends, the traceparent remains retrievable from `AgentRunResult` so out-of-band artifacts (feedback spans, online evals) attach to the same trace (`run.py:604-613`; `tests/test_logfire.py:1299-1301`).

## Architectural Decisions

1. **Instrumentation as a composable capability, not core plumbing.** Tracing lives in `Instrumentation` (`capabilities/instrumentation.py:67-146`), hooking generic wrap points (`wrap_run`, `wrap_model_request`, `wrap_tool_execute`, `wrap_output_process`). Any stack can therefore be traced without touching the agent loop, and users can add attributes to the active span from sibling capabilities (`capabilities/instrumentation.py:73-76`).

2. **Standards-first OTel GenAI semconv with versioned escape hatches.** Span names and attributes track the OTel GenAI spec (`invoke_agent`, `chat`, `execute_tool`, `gen_ai.*`; `InstrumentationNames.for_version`, `_instrumentation.py:657-746`), with a `version` field (2–6) pinning wire-format evolution and deprecation warnings for old formats (`models/instrumented.py:117-163`). A repo rule even mandates that `_otel_*.py` modules implement only spec-defined features (`pydantic_ai_slim/pydantic_ai/AGENTS.md`, rule 17). Non-spec conveniences are quarantined under a custom namespace and flagged as such (`gen_ai.aggregated_usage.*`, `models/instrumented.py:136-141`).

3. **One shared implementation for classic/streaming/realtime paths.** `open_model_request_span` serves both the agent-flow capability and standalone `InstrumentedModel` requests (`_instrumentation.py:451-463`); realtime reuses `response_attributes`, `build_tool_definitions`, `annotate_tool_call_otel_metadata`, and `handle_messages` "so realtime `chat` spans can't drift from the classic path" (`realtime/_instrumentation.py:299-304`), and both run-span variants share `aggregated_usage_attributes` (`models/instrumented.py:301-306`).

4. **Suppress duplicate roots; own the tree at the agent layer.** `auto_instrument=False` for agent graphs (`_agent_graph.py:2587`) and the realtime skip in `wrap_run` (`capabilities/instrumentation.py:158-162`) show a deliberate policy: exactly one canonical run span, with graph-engine spans reserved for standalone graph users.

5. **Traceparent as a durable artifact of a run.** Rather than relying solely on ambient context, the finished `AgentRunResult` retains its W3C traceparent (`run.py:604-613`), enabling post-hoc correlation (logfire `record_feedback`, online evals) — an intentional bridge between ephemeral execution and asynchronous observability workflows (`tests/test_logfire.py:1289-1309`).

6. **Telemetry must never break the run.** Serialization failures fall back to `str()` (`serialize_any`, `_instrumentation.py:220-230`), redaction failures return a placeholder instead of raising (`_instrumentation.py:160-167`), pricing failures degrade to `None` (`response_price_calculation`, line 423-431), and metrics are computed even when spans are unsampled (`_instrumentation.py:520-525`).

## Notable Patterns

- **Baggage-based correlation injection**: run identity (`gen_ai.agent.name`, `gen_ai.agent.call.id`, `gen_ai.conversation.id`) is attached as OTel baggage around the run span and stamped onto every downstream span, giving backend grouping keys without threading parameters (`_instrumentation.py:123-135`; `capabilities/instrumentation.py:187-191`, `397`).
- **Per-run fragment cache for O(new messages) serialization**: `MessageJsonCache` caches each message's serialized OTel JSON keyed by object identity, invalidated by `parts` list identity, keeping growing histories cheap to re-record each request — with an end-of-run staleness audit converting silent drift into a loud `MessageHistoryMutatedWarning` (`_instrumentation.py:107-120`, `256-281`; `capabilities/instrumentation.py:224-234`).
- **Tree-shaped snapshot testing**: `LogfireSummary` reconstructs parent-child structure from exported spans so tests assert the actual hierarchy rather than flat lists (`tests/test_logfire.py:50-92`).
- **Control-flow-vs-error discipline**: v5+ leaves spans UNSET for `CallDeferred`/`ApprovalRequired` while recording structured deferral attributes, reserving ERROR for genuine failures — an explicit semantic decision documented in the format changelog (`models/instrumented.py:129-131`; `capabilities/instrumentation.py:452-479`).
- **Zero-duration/lifecycle spans for visibility**: realtime emits instantaneous `model turn complete`, barge-in, and `user speech` spans (backdated to onset only when both boundaries were measured — "a duration nobody measured is worse than no duration at all") (`realtime/_instrumentation.py:410-451`).
- **Evaluator-facing trace queries**: `SpanQuery` supports declarative ancestor/descendant/status/duration predicates over collected spans, making the trace tree programmatically consumable by eval evaluators (`pydantic_evals/pydantic_evals/otel/span_tree.py:29-88`).

## Tradeoffs

- **Opt-in by default**: without `instrument=True`/the capability or `logfire.instrument_pydantic_ai()`, no run tree exists (`docs/logfire.md:24-26`; `tests/test_logfire.py:163-165` asserts zero spans when un-instrumented). Zero-overhead is prioritized over always-on observability.
- **Content-heavy spans**: default `include_content=True` and `include_model_request_parameters=True` put full prompts, completions, tool args/results, and entire tool schemas on spans (`models/instrumented.py:77-78`, `110-116`), which aids debugging but risks payload size and sensitive-data exposure; mitigations are opt-out flags and redaction rather than opt-in capture.
- **Sequential-request assumption**: `Instrumentation` documents that its per-run mutable fields "assume sequential model requests within a run — if the agent loop ever issues concurrent model requests, accesses to these fields would race" (`capabilities/instrumentation.py:88-92`).
- **Delegation nesting is emergent, not enforced**: sub-agent traces nest correctly because of asyncio/OTel context mechanics, not an explicit handoff API; nothing validates that a delegate run actually landed inside the parent's tree, and sync delegation inside runs is forbidden outright (`docs/multi-agent-applications.md:79-83`).
- **Backend-coupled rendering niceties**: `logfire.msg` and `logfire.json_schema` attributes pervade span construction (`capabilities/instrumentation.py:175`, `399-415`), coupling some telemetry shape to Logfire conventions even though any OTel backend works.

## Failure Modes / Edge Cases

Handled explicitly (with tests):

- Unsampled/non-recording spans: attributes skipped gracefully, metrics still emitted (`_instrumentation.py:520-525`; `tests/models/test_instrumented.py:303-330`).
- Streaming interrupted mid-segment: TTFT reported in `finally` even on cancellation; OTel context detach across different contextvars Contexts avoided via `attach` instead of token `detach` (#6569) (`_agent_graph.py:1206-1214`; `_instrumentation.py:561-575`).
- Malformed base URLs, lone surrogates, unserializable values, pricing errors: each degrades instead of failing the run (`_instrumentation.py:233-243`; tests at `tests/models/test_instrumented.py:329-346`, `455-480`, `2250+`, `1493+`).
- In-place history mutation producing misleading recorded messages: detected and warned at run end (`tests/test_logfire.py:1190-1288`).
- OTel context leakage across tests/workers: isolation pinned by ordered tests (`tests/test_otel_context_isolation.py:1-53`).
- Missing optional dependencies: traceparent returns None without OTel (`pydantic_graph/pydantic_graph/_utils.py:46-50`); eval span-tree unavailable without a processor-capable tracer provider degrades with actionable guidance (`pydantic_evals/pydantic_evals/otel/_context_in_memory_span_exporter.py:146-166`).

Residual gaps:

- A mutated-and-dropped history message can produce a stale span with no warning (documented best-effort boundary) (`_instrumentation.py:265-271`).
- No span representation for output validators/retries as operations; they're inferable only from message attributes.
- Concurrent model requests within one run would race instrumentation bookkeeping (`capabilities/instrumentation.py:88-92`).

## Future Considerations

- **Uniform cross-process propagation**: today Temporal uses its interceptor (`durable_exec/temporal/_logfire.py:57-60`), realtime injects raw headers (`realtime/google.py:609-618`), and in-process runs rely on ambient context. A documented, uniform story for propagating trace context into delegated/sub-agent processes (beyond baggage) would close the main remaining gap.
- **Guardrail/validator spans**: if the framework grows first-class guardrails, a dedicated operation name would keep step inventories complete; currently validators are invisible as spans.
- **Concurrency-safe instrumentation state**: refactoring `_last_*`/cache fields into per-request objects would remove the sequential-only constraint ahead of any parallel-request feature.
- **Retrieval semantics**: RAG-style retrieval has no distinct span convention here (only tool/httpx spans); adopting an emerging OTel retrieval convention would improve tree interpretability.

## Questions / Gaps

- **Is there a non-Logfire trace viewer?** No evidence found in-repo beyond Logfire UI material, otel-tui mentions, and third-party integrations listed in `docs/logfire.md:188`, `253`. Search boundary: `docs/`, `pydantic_ai_slim`, README; no bundled viewer/UI code for traces was found (`docs/ui/` concerns chat UIs, not trace viewing).
- **Do eval harness runs appear in the same trace as the agent run they evaluate?** Partially: `context_subtree()` collects spans produced during an evaluation context into a `SpanTree` (`pydantic_evals/pydantic_evals/otel/_context_in_memory_span_exporter.py:40-55`), and online evals extract the active span reference (`online.py:807-835`), which implies co-tracing when evaluation wraps the run; I did not find a test asserting a single unified trace spanning evaluator + agent spans end-to-end. Search boundary: `pydantic_evals/`, `tests/evals/`.
- **Prefect/DBOS durable paths**: no tracing-specific code was found in `durable_exec/prefect` or `durable_exec/dbos` (only Temporal has the Logfire plugin/interceptor). Whether those engines preserve trace context across workers is unverified. Search boundary: `pydantic_ai_slim/pydantic_ai/durable_exec/`.

---

Generated by dimension 10.01 (Span Hierarchy and Run Tree) against pydantic-ai.
