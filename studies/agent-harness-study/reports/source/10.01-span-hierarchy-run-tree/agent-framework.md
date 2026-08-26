# Source Analysis: agent-framework

## Dimension 10.01 — Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Polyglot monorepo: Python (`python/`, OpenTelemetry API/SDK), .NET (`dotnet/`, `System.Diagnostics.Activity`), Go (stub only) |
| Analyzed | 2026-08-25 |

## Summary

Agent Framework's Python core is **natively instrumented with OpenTelemetry GenAI semantic conventions**. Every layer of the run tree emits a span on a shared global tracer: `invoke_agent {name}` wraps an agent turn (`python/packages/core/agent_framework/observability.py:1920-2278`), each model call emits `chat {model}` (`observability.py:1528-1847`), each tool invocation emits `execute_tool {name}` (`observability.py:2305-2322`, `python/packages/core/agent_framework/_tools.py:733-800`), and MCP operations emit CLIENT-kind spans per the OTel MCP conventions (`observability.py:2329-2358`). Workflows add a parallel hierarchy — `workflow.build` → `workflow.run` → `executor.process {id}` → `message.send` / `edge_group.process` — with fan-in causality expressed via span *links* instead of nesting. Parenting across async/streaming boundaries is handled explicitly rather than left to ambient context, including a per-pull span-activation mechanism so child spans created during stream consumption inherit the correct parent.

Trace context crosses process/message boundaries in three ways: W3C `traceparent` carriers are injected into workflow messages (and survive checkpoint serialization), injected into MCP request `_meta` for client-opened MCP transports, and reconstructed as remote span contexts for workflow fan-in links. The .NET stack mirrors this model with `OpenTelemetryAgent` (GenAI semconv v1.37, `invoke_agent` activity reusing M.E.AI's `OpenTelemetryChatClient`) and a workflow `ActivitySource`. Gaps: tool approvals/guardrails, evals, retrieval, and the orchestration layer (handoff/group-chat decision logic) have no dedicated spans; hosted-MCP tools and A2A/hosting HTTP boundaries do not propagate framework-level trace headers; the Go implementation has no tracing at all.

## Rating

**8 / 10** — Clear, tested span model with explicit interfaces (`OtelAttr` attribute registry, telemetry layers as mixins), operational safeguards (sticky disable, sensitive-data opt-in, error capture, weakref-finalized stream spans), and proven parent-child behavior under streaming/fan-in failure modes (dedicated tests). Falls short of 9–10 because several execution-step categories (guardrail evaluations, evals, orchestration decisions) are absent from the tree, cross-process propagation is partial (MCP yes; hosted connectors/A2A no), and some traceparent parsing is self-admittedly "simplified" (`observability.py:3253`).

## Evidence Collected

Every entry cites file paths with line numbers inside the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tracing system (Python) | OTel API imported at module level; tracer/meter/log helpers wrap global providers | python/packages/core/agent_framework/observability.py:41-42 |
| Provider setup | `configure_otel_providers()` builds TracerProvider + BatchSpanProcessor, LoggerProvider, MeterProvider from env/exporters | python/packages/core/agent_framework/observability.py:1300-1496, 1006-1079 |
| Env-based exporters | Standard `OTEL_EXPORTER_OTLP_*` env vars parsed into gRPC/HTTP OTLP exporters | python/packages/core/agent_framework/observability.py:548-638 |
| Settings & kill switch | `ObservabilitySettings` (ENABLE_INSTRUMENTATION default true, ENABLE_SENSITIVE_DATA default false); sticky `disable_instrumentation()` | python/packages/core/agent_framework/observability.py:753-950, 1238-1261 |
| Semantic-convention registry | `OtelAttr` enum defines all GenAI + workflow span names/attributes | python/packages/core/agent_framework/observability.py:223-361 |
| Agent span | `AgentTelemetryLayer._trace_agent_invocation` creates INTERNAL `invoke_agent {name}` span; kind rationale documented | python/packages/core/agent_framework/observability.py:1938-2181, 2404-2426 |
| Agent layer wiring | `AgentTelemetryLayer` mixed into agent classes | python/packages/core/agent_framework/_agents.py:1788 |
| Model-call span | `ChatTelemetryLayer.get_response` creates `chat {model}` span (streaming + non-streaming) | python/packages/core/agent_framework/observability.py:1593-1847 |
| Client layer composition | `OpenAIChatClient(FunctionInvocationLayer, ChatMiddlewareLayer, ChatTelemetryLayer, RawOpenAIChatClient)` — function loop sits above chat span | python/packages/openai/agent_framework_openai/_chat_client.py:3430-3436 |
| Tool span | `execute_tool {tool_name}` span with TOOL_CALL_ID, arguments/result attributes gated on sensitive data + semconv | python/packages/core/agent_framework/_tools.py:733-800; observability.py:2284-2322, 927-934 |
| MCP spans | `create_mcp_client_span` CLIENT-kind spans for initialize/tools/list/tools/call/prompts | python/packages/core/agent_framework/observability.py:2329-2358; _mcp.py:1387, 1755, 1853, 2093, 2652 |
| Workflow build/run spans | `workflow.build` with build.* events; `workflow.run` with workflow.started/completed/error events + ERROR status | python/packages/core/agent_framework/_workflows/_workflow_builder.py:809-889; _workflow.py:509-524, 600-622 |
| Executor span | `executor.process {executor_id}` created around every handler invocation | python/packages/core/agent_framework/_workflows/_executor.py:271-307 |
| Message-send span | `message.send` PRODUCER span; injects current trace context into message | python/packages/core/agent_framework/_workflows/_workflow_context.py:318-348 |
| Edge-group spans | `edge_group.process` spans with delivery-status attributes | python/packages/core/agent_framework/_workflows/_edge_runner.py:108-155, 185-268, 315-330 |
| Streaming parenting | `_activate_span` attaches span per iterator pull so children parent correctly across async contexts; weakref finalizer closes unconsumed streams | python/packages/core/agent_framework/observability.py:2380-2400, 2103-2117, 1780 |
| Usage roll-up | `INNER_ACCUMULATED_USAGE` contextvar aggregates inner chat token usage onto `invoke_agent` span; response_id/usage dedup between layers | python/packages/core/agent_framework/observability.py:121-136, 3078-3103, 2056-2064 |
| Instruction attribution | System instructions attached to agent span only when chat span's parent IS the agent span (explicit parentage check) | python/packages/core/agent_framework/observability.py:2809-2841 |
| Error capture | `capture_exception` sets error.type, records exception, ERROR status on every layer | python/packages/core/agent_framework/observability.py:2787-2791 |
| Fan-in links | `create_processing_span` builds remote SpanContext links from source traceparents (causality without nesting) | python/packages/core/agent_framework/observability.py:3223-3280 |
| Message trace carriers | `WorkflowMessage.trace_contexts/source_span_ids` (W3C headers), serialized through checkpoints | python/packages/core/agent_framework/_workflows/_runner_context.py:46-93 |
| MCP cross-process propagation | `_inject_otel_into_mcp_meta` injects trace context into `tools/call` `params._meta` via global propagator | python/packages/core/agent_framework/_mcp.py:294-308, 2202-2203 |
| Trace viewer/UI | DevUI `SimpleTraceCollector(SpanExporter)` converts spans (incl. parent_span_id, events, errors) to UI trace events; enabled via `--instrumentation` | python/packages/devui/agent_framework_devui/_tracing.py:20-117; python/packages/devui/README.md:128-132 |
| Console/VS Code exporters | ConsoleSpanExporter set + OTLP exporter pointed at VS Code extension port | python/packages/core/agent_framework/observability.py:986-1000 |
| .NET agent span | `OpenTelemetryAgent` delegates to M.E.AI `OpenTelemetryChatClient`, renames activity to `invoke_agent {Name(Id)}`, forwards `Activity.Current` | dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgent.cs:33-113, 154-214 |
| .NET auto-wiring | Auto-wraps inner chat client below FunctionInvokingChatClient so execute_tool spans share one ActivitySource | dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgent.cs:234-269 |
| .NET workflow spans | `WorkflowTelemetryContext` ActivitySource "Microsoft.Agents.AI.Workflows"; activities workflow.build/session/invoke, executor.process, edge_group.process | dotnet/src/Microsoft.Agents.AI.Workflows/Observability/WorkflowTelemetryContext.cs:13-100; Observability/ActivityNames.cs:5-13 |
| Tests: hierarchy | End-to-end workflow test asserts workflow.run → executor.process → message.send parentage and fan-in links | python/packages/core/tests/workflow/test_workflow_observability.py:261-399 |
| Tests: streaming parenting | Sync-setup spans asserted as children of chat/agent spans; event correlation to chat span ids | python/packages/core/tests/core/test_observability.py:425-504, 872-961 |
| Tests: errors & disablement | ERROR status asserted on failing agent span; zero-span assertions when disabled | python/packages/core/tests/core/test_observability.py:3221-3263, 3685-3715 |
| Tests: MCP propagation | `test_mcp_tool_call_tool_otel_meta` covers traceparent injection/extraction in `_meta` | python/packages/core/tests/core/test_mcp.py:5528-5578 |

## Answers to Dimension Questions

**1. Is there a single coherent trace tree?**
Yes, within a process. All spans come from one global OTel tracer (`get_tracer`, `observability.py:1082-1130`; `workflow_tracer()`, `observability.py:3204-3207`) and rely on OTel context propagation, so `invoke_agent` → `chat`/`execute_tool` form one tree, and workflows produce `workflow.run` → `executor.process` → `message.send` trees (verified by parentage assertions in `test_workflow_observability.py:376-385`). The one deliberate exception is fan-in: aggregator processing spans *link* to multiple source send spans rather than nesting them (`observability.py:3231-3235` docstring: "linked (not nested)… for causality tracking"), which is semantically correct for many-to-one joins but means the "tree" becomes a graph at those nodes.

**2. Are all execution steps represented?**
Model calls, tool executions, embeddings, agent invocations, and all workflow mechanics (build, run, executor processing, edge delivery, message sends) are represented. Not represented: guardrail/approval evaluations (`_harness/_tool_approval.py` contains zero tracing references), evaluation runs (`_evaluation.py` has no spans), retrieval/context-provider injection (no dedicated spans; only visible if a RAG provider happens to be tool-implemented), and orchestration-layer decisions (handoff selection, group-chat turn management in `python/packages/orchestrations/` add no spans of their own).

**3. Do handoffs and subagent calls nest correctly?**
Handoff/group-chat/magentic orchestrations compile to core `Workflow`s whose participants run through `AgentExecutor` (`python/packages/orchestrations/agent_framework_orchestrations/_handoff.py:46-52`), so each agent handoff appears as `executor.process` → `invoke_agent` nested under the same `workflow.run` span (`_executor.py:273-307`; agent run invoked in handler at `_agent_executor.py:413-424`). Nested/subagent runs (e.g., background agents spawning `agent.run` in tasks) still nest because parenting rides on contextvars; the code explicitly manages the known asyncio pitfall where a coroutine may be awaited in a different context than created (`observability.py:2119-2131` comments and lazy token setting). No evidence was found of sub-agent traces breaking out of the parent tree when running in-process.

**4. Can you follow a request from start to finish?**
In-process: yes — DevUI renders collected spans with trace_id/parent_span_id/events/errors in a Traces panel (`_tracing.py:82-117`; README:128-132), console/OTLP exporters cover external backends. Across boundaries: partially. Workflow messages carry W3C traceparent through queues and checkpoints (`_runner_context.py:48-49, 72-93`; test at `test_workflow_observability.py:438-471`), and MCP client-opened transports propagate via `_meta` (`_mcp.py:294-308`). But hosted/provider-managed MCP tools explicitly do not propagate (documented limitation in `python/samples/02-agents/observability/README.md:170`), and no A2A/hosting HTTP header propagation exists in the framework itself (no opentelemetry references found in `python/packages/a2a/` or `hosting-a2a/`; searches were directory-wide greps for `opentelemetry|trace|traceparent`).

## Architectural Decisions

- **Native instrumentation over auto-instrumentation**: spans are emitted by framework classes themselves via OTel API-only dependency, so any globally configured SDK picks them up without extra instrumentation packages (`python/samples/02-agents/observability/README.md:164,176`; `observability.py:41-42`).
- **Telemetry layers as mixins**: `AgentTelemetryLayer`/`ChatTelemetryLayer`/`EmbeddingTelemetryLayer` are composed into concrete clients/agents (e.g. `OpenAIChatClient` MRO puts FunctionInvocationLayer above ChatTelemetryLayer above RawClient, `python/packages/openai/agent_framework_openai/_chat_client.py:3430-3436`), which fixes the tree shape by construction: function-calling iterations appear as sibling `chat` spans under one `invoke_agent`.
- **Explicit async-context parenting**: streaming spans are started non-current and attached per-pull (`_start_streaming_span` + `with_pull_context_manager`, `observability.py:2429-2453, 2103-2117`) to avoid OTel "Failed to detach context" errors across asyncio contexts — a deliberate engineering choice, documented in-code.
- **Links for fan-in, nesting for chains**: causality across converging edges uses `trace.Link` with remote span contexts parsed from carried traceparents (`observability.py:3245-3268`).
- **Single attribute registry**: `OtelAttr` centralizes every span name/attribute (`observability.py:223-361`); .NET mirrors it in `OpenTelemetryConsts.cs` and `ActivityNames.cs:5-13`.
- **Sensitive-data gating separate from enablement**: metadata always available when instrumentation is on; message content/tool args/results require `enable_sensitive_data` plus latest-semconv opt-in (`observability.py:762-774, 927-934`).

## Notable Patterns

- **Usage aggregation up the tree**: inner `chat` spans mark captured fields and accumulate usage in contextvars; the outer `invoke_agent` span suppresses duplicate `gen_ai.response.id`/usage and applies accumulated totals (`observability.py:3078-3103, 2056-2064`).
- **Parentage verification before attribution**: system instructions are written to the agent span only after checking the chat span's parent span/trace IDs match (`observability.py:2832-2841`) — defensive against ambient-context leakage.
- **Timestamp stepping for message events**: chat-message log events are spaced +1µs apart so ordering survives backend timestamp truncation, with a filter for stdlib-logged events (`observability.py:190-220`).
- **Graceful degradation everywhere**: workflow link building swallows malformed traceparents (`observability.py:3251, 3339-3341`); telemetry serialization failures warn-and-skip instead of raising (`observability.py:2511-2518, 2558-2563`).
- **Sticky disable flag**: once `disable_instrumentation()` is called, even direct attribute writes cannot re-enable without `force=True`, and integrations consult `is_user_disabled` before side-effect setup (`observability.py:823-871, 940-950`).
- **Checkpoint-durable trace context**: trace contexts serialize/deserialize with workflow messages so resumed runs keep causal linkage (test at `test_workflow_observability.py:438-471`).

## Tradeoffs

- **Mixin layering vs explicit instrumentation**: clean composition, but the tree shape depends on MRO order that callers could disturb by custom subclassing; nothing validates the resulting shape outside tests.
- **String-parsed traceparents for links**: the fan-in linker splits `"00-{trace}-{span}-{flags}"` manually and admits it is "a simplified approach - in production you'd want more robust parsing" (`observability.py:3253-3256`, repeated at `3323-3327`). Robust enough for self-produced W3C headers, brittle for foreign propagators (e.g. B3 via `tracestate`-only setups).
- **INTERNAL kind uniformly for invoke_agent**: correct for in-process agents but diverges from v1.41 guidance for remote-service agents; also still emits `gen_ai.response.id/model/finish_reasons` on invoke_agent contrary to latest conventions — divergence is openly documented (`python/samples/02-agents/observability/README.md:220`).
- **Per-pull span activation cost**: attaching/detaching OTel context on every stream pull adds overhead per chunk in exchange for correct parenting.
- **Polyglot asymmetry**: .NET reimplements the same model with Activities and gets richer options (per-feature toggles like `DisableWorkflowBuild`, session-span concept absent in Python: `WorkflowTelemetryContext.cs:81-100` vs Python's flat `workflow.run`); Go has none (`go/README.md:1-5`).

## Failure Modes / Edge Cases

- **Unconsumed streams**: streaming spans would otherwise leak open; mitigated by `weakref.finalize(wrapped_stream, _close_span)` (`observability.py:1780, 2116`) — but finalization timing depends on GC, so a retained-but-never-consumed stream keeps its span open indefinitely.
- **Stream mid-flight errors**: cleanup hooks capture the stream error on the span and skip result hooks before closing (`observability.py:2040-2053, 1715-1728`); tested at `test_observability.py:3618+`.
- **Cross-context token resets**: setting contextvars eagerly in a sync `run()` body then resetting in another task raises OTel/contextvar errors; code defers sets to first pull/consumer context with detailed rationale (`observability.py:1983-1992, 2119-2131`).
- **Malformed trace contexts on messages**: link construction degrades silently to zero links (`observability.py:3251`), so a corrupted carrier loses causality invisibly rather than erroring.
- **Disabled tracing mid-pipeline**: `message.trace_context` is None when tracing off (`test_workflow_observability.py:232-257`), meaning later-enabled consumers get unlinked runs rather than errors.
- **Duplicate chat spans (.NET)**: auto-wiring checks for existing `OpenTelemetryChatClient` to avoid double spans (`OpenTelemetryAgent.cs:263-269`).

## Future Considerations

- Add spans for guardrail/approval evaluation cycles and eval runs so HITL pauses are visible as first-class tree nodes (currently only inferable from missing terminal results).
- Add orchestration-level spans (handoff decision, group-chat speaker selection) atop the workflow engine spans.
- Replace manual traceparent splitting in link construction with the OTel `TraceContext.from_headers`-style parser or store raw SpanContext alongside the header dict.
- Extend `_meta`-style propagation to hosted MCP connectors and A2A/hosting HTTP surfaces (or document reliance on platform instrumentation explicitly in-code, not only in samples).
- Align `invoke_agent` attribute emission with the v1.41 split (drop response attributes, add `gen_ai.agent.version`) behind the existing semconv opt-in flag.
- Close the polyglot gap: Go implementation lives in a separate repo with no tracing surface here.

## Questions / Gaps

- **Guardrail/approval tracing**: searched `python/packages/core/agent_framework/_harness/_tool_approval.py` and `_evaluation.py` for `span|trace|OtelAttr` — zero matches. No evidence of approval-flow spans anywhere in core.
- **A2A / hosting boundary propagation**: directory-wide searches for `opentelemetry|trace|traceparent` across `python/packages/a2a`, `python/packages/hosting-a2a`, and `dotnet/src/Microsoft.Agents.AI.Hosting.A2A.AspNetCore` returned no framework-level trace propagation code. Whether hosted endpoints preserve caller trace IDs depends on ambient ASP.NET/ASGI instrumentation — not implemented in-repo.
- **Retrieval/RAG visibility**: `ContextProvider` implementations (e.g. azure-ai-search package) were not exhaustively checked for spans; core provides no retrieval-specific span type in `OtelAttr` (`observability.py:223-361` lists none), so retrieval steps are only visible if implemented as tools.
- **Eval traces**: `_evaluation.py:1786` mentions `evaluate_traces(response_ids=...)` as an input mechanism, but there is no evidence that evaluations emit spans/metrics themselves.
- **Python/.NET workflow span parity**: Python lacks the .NET `workflow.session` long-lived parent span (`ActivityNames.cs:8`); whether that asymmetry is intentional could not be determined from in-repo docs.

---

Generated by `10.01-span-hierarchy-and-run-tree` against `agent-framework`.
