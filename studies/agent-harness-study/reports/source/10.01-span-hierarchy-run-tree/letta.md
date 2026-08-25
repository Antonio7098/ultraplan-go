# Source Analysis: letta

## Dimension 10.01: Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3 (FastAPI, SQLAlchemy, OpenTelemetry SDK, ClickHouse, Datadog) |
| Analyzed | 2026-08-24 |

Citation convention: all file paths below are relative to the source root `studies/agent-harness-study/sources/letta/`.

## Summary

Letta uses OpenTelemetry as its primary tracing system. A hand-rolled FastAPI middleware opens a single `SERVER`-kind root span per HTTP request (`letta/otel/tracing.py:38-69`), a generic `@trace_method` decorator wraps hundreds of methods (LLM clients, tool execution, summarizers, vector-store helpers, job polling) as child spans (`letta/otel/tracing.py:228-435`), and step-scoped `agent_step` and `time_to_first_token` spans are created manually inside the agent loop with `step_id` attributes and duration events (`letta/agents/letta_agent.py:291-294`, `letta/server/rest_api/utils.py:88-104`). Spans export via OTLP/gRPC through a BatchSpanProcessor to an OpenTelemetry Collector that persists to ClickHouse; the server then re-serves those spans through a public trace-viewer endpoint, `GET /v1/runs/{run_id}/trace`, which joins run → steps → OTEL `trace_id` stored on each `Step` row (`letta/services/step_manager.py:157`, `letta/server/rest_api/routers/v1/runs.py:267-307`). A second, parallel "provider trace" pipeline writes full LLM request/response payloads to a denormalized ClickHouse `llm_traces` table for cost analytics, keyed by `run_id`/`step_id` but with its OTEL correlation column left null (`letta/services/provider_trace_backends/clickhouse.py:124`). The result is a single coherent trace tree per in-process request — request root → middleware/dependency spans → LLM/tool/summarization method spans — with one structural quirk: `agent_step` and TTFT spans are created but never made the active span context, so per-step children hang off the request root as siblings of `agent_step` rather than under it. Cross-process propagation is one-directional: outgoing provider HTTP calls get W3C `traceparent` injection via `RequestsInstrumentor` (`letta/otel/tracing.py:15,166`), but no incoming extraction exists, so external callers cannot join a Letta trace.

## Rating

**7 / 10** — Clear model with explicit interfaces and operational safeguards.

Rationale:
- Clear, explicit tracing model: dedicated module (`letta/otel/tracing.py`), a reusable decorator contract (`@trace_method`, `log_event`, `log_attributes`, `get_trace_id` at `letta/otel/tracing.py:228,478,484,499`), and named span conventions (`agent_step`, `%._execute_tool`, `time_to_first_token`) that are codified in both the trace reader SQL (`letta/services/clickhouse_otel_traces.py:60-78`) and the public API docs (`fern/openapi.json:16321`).
- Operational safeguards: parameter exclusion/truncation with size budgets to prevent span bloat (`letta/otel/tracing.py:250-277`) and unit tests proving it (`tests/test_otel_tracing.py:101-301`); pytest guard disables tracing during tests (`letta/otel/tracing.py:150-151`, `letta/otel/resource.py:59-60`); collector-side memory limiter/batching and retrying ClickHouse exporter (`otel/otel-collector-config-clickhouse.yaml:32-55`); retry-with-backoff on trace writes (`letta/services/llm_trace_writer.py:23-25,138-171`).
- What keeps it out of 9–10: the step-level hierarchy is flat rather than nested (see Tradeoffs), there is no incoming distributed-context extraction anywhere in the codebase (no `TraceContextTextMapPropagator` usage found), durability depends on an optional ClickHouse deployment (endpoint returns 501 without it, `runs.py:289-294`), and the analytics `llm_traces.trace_id` column is never populated (`provider_trace_backends/clickhouse.py:124`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trace provider setup | OTLP gRPC exporter + `BatchSpanProcessor` + global `TracerProvider`; gated by pytest guard | `letta/otel/tracing.py:14-18,150-159` |
| Enable/disable config | `settings.otel_exporter_otlp_endpoint`, `disable_tracing`, `sqlalchemy_tracing` wired at app startup | `letta/settings.py:355,381`, `letta/server/rest_api/app.py:819-833` |
| Root request span | `_trace_request_middleware` creates `SERVER` span `{METHOD} {path}`, renames to route pattern, records status/errors; `/v1/health` excluded | `letta/otel/tracing.py:38-69,28-35` |
| Request enrichment dependency | `Depends(_update_trace_attributes)` appended to every v1 route; copies user/org/project/agent/template headers and JSON body onto the span | `letta/otel/tracing.py:209-220,96-124` |
| Error handling on spans | Exception handlers record exceptions on current span and return `trace_id` in error body | `letta/otel/tracing.py:127-142`, `letta/server/rest_api/app.py:485-487` |
| Generic method tracer | `@trace_method` async/sync wrappers create `Class.method` child spans with serialized parameters | `letta/otel/tracing.py:228-235,389-435` |
| Parameter safety | `SKIP_PARAMS` opt-out list, `MAX_PARAM_SIZE` 2MB, `MAX_TOTAL_SIZE` 4MB, truncation markers | `letta/otel/tracing.py:250-277,366-371` |
| Step span | Per-step `agent_step` span with `step_id` attribute, created with explicit ns start time | `letta/agents/letta_agent.py:291-294`, `letta/agents/letta_agent_v2.py:944-945` |
| Step span events | `llm_request_ms`, `step_ms` events; span ended in checkpoint finish | `letta/agents/letta_agent.py:420-423`, `letta/agents/letta_agent_v2.py:980-996` |
| TTFT span | `time_to_first_token` span opened at request start, closed with `time_to_first_token_ms` event on first chunk | `letta/server/rest_api/utils.py:88-104`, `letta/agents/letta_agent.py:243-245` |
| Tool execution events | `tool_execution_started` / `tool_execution_completed` (tool_name, duration_ms, success) events on `agent_step_span` | `letta/agents/letta_agent.py:1946-1982`, `letta/agents/letta_agent_v2.py:1312-1338` |
| Tool execution span | `_execute_tool` decorated `@trace_method` → span name matches trace-viewer filter `LIKE '%._execute_tool'` | `letta/agents/letta_agent.py:1921-1929`, `letta/services/clickhouse_otel_traces.py:71-72` |
| Model call events | `llm_request_sent` / `llm_response_received` events carry full request/response payloads on current span | `letta/llm_api/llm_client_base.py:227-239,261-274` |
| Broad decorator coverage | `@trace_method` on all provider clients (OpenAI, Anthropic, Groq, DeepSeek, xAI, Azure, BaseTen, Together, Z.ai), prompt generation, summarizers, vector stores, batch polling | `letta/llm_api/openai_client.py:339`, `letta/llm_api/anthropic_client.py:64`, `letta/services/summarizer/summarizer.py:74,487,741,819`, `letta/helpers/pinecone_utils.py:146-308`, `letta/jobs/llm_batch_job_polling.py:41-169` |
| Run↔trace persistence link | `Step` rows persist `trace_id = get_trace_id()` (current OTEL trace) plus cloud `request_id` | `letta/services/step_manager.py:156-158,218-220` |
| Trace viewer API | `GET /v1/runs/{run_id}/trace`: picks first non-null step `trace_id`, filters ClickHouse spans for UI (agent_step, tool spans, root, TTFT) | `letta/server/rest_api/routers/v1/runs.py:267-307` |
| Trace storage query | Reader queries `otel_traces` by `TraceId` with UI-span filter; documented in OpenAPI spec | `letta/services/clickhouse_otel_traces.py:57-93`, `fern/openapi.json:16321` |
| Collector deployment | Dockerfile installs `otelcol-contrib`; clickhouse collector config defines otlp grpc/http receivers and traces/logs/metrics pipelines to ClickHouse exporter with retry queue | `Dockerfile:47-77`, `otel/otel-collector-config-clickhouse.yaml:1-81` |
| Compose env wiring | `LETTA_OTEL_EXPORTER_OTLP_ENDPOINT`, `CLICKHOUSE_*` env pass-through | `compose.yaml:48-52` |
| Secondary analytics store | Provider traces → ClickHouse `llm_traces` ("bypasses OTEL for large payloads"), toggled by `track_provider_trace` / `store_llm_traces` | `letta/settings.py:387-394`, `letta/llm_api/llm_client_base.py:263-272` |
| Analytics conversion gap | `LLMTrace(... trace_id=None ...)` — OTEL correlation column intentionally/unintentionally null | `letta/services/provider_trace_backends/clickhouse.py:116-125` |
| Write durability | `LLMTraceWriter` fire-and-forget asyncio tasks, 3 retries w/ exponential backoff, drop-after-retries warning | `letta/services/llm_trace_writer.py:117-171` |
| Log↔trace correlation | JSON log formatter injects `dd.trace_id`/`dd.span_id` from current OTEL span context | `letta/log.py:74-86` |
| Outgoing propagation | `RequestsInstrumentor` instruments outbound HTTP (W3C `traceparent` injected by default propagator) with status-code hook | `letta/otel/tracing.py:15,162-166` |
| Incoming propagation | No `TraceContextTextMapPropagator`/header extraction anywhere; middleware always starts a fresh root (searched whole source) | `letta/otel/tracing.py:48-51` |
| In-process fan-out nesting | Background multi-agent work spawned via `safe_create_task` (asyncio task inherits ambient OTel context) from `@trace_method`-wrapped group methods | `letta/utils.py:1165-1211`, `letta/groups/sleeptime_multi_agent_v3.py:147-188` |
| Streaming-safe contextvars | Pure ASGI `RequestIdMiddleware` propagates `x-api-request-log-id` contextvar into streaming responses; request attributes ContextVar shared with metrics | `letta/server/rest_api/middleware/request_id.py:32-66`, `letta/otel/context.py:5-24` |
| DB instrumentation | Optional SQLAlchemyInstrumentor plus custom `db_operation` spans via `setup_letta_db_instrumentation` | `letta/otel/tracing.py:168-207`, `letta/otel/sqlalchemy_instrumentation.py:87-88` |
| Tests | Unit tests cover span attribute exclusion/truncation only; integration tests cover ClickHouse LLM-trace storage; no test asserts parent-child span structure | `tests/test_otel_tracing.py:101-301`, `tests/integration_test_clickhouse_llm_traces.py:1-31` |

## Answers to Dimension Questions

1. **Is there a single coherent trace tree?**
   Yes, within one request handled by one process. Every request gets exactly one root `SERVER` span (`letta/otel/tracing.py:48-51`); middleware, dependency, LLM-client, tool-execution, summarization, and DB spans all attach under it via implicit context parenting, sharing one `trace_id`. The caveat is structural: `agent_step` and `time_to_first_token` spans use `tracer.start_span(...)` without `use_span`/activation (`letta/agents/letta_agent.py:293`, `letta/server/rest_api/utils.py:90`), so they are recorded as siblings of the method spans beneath the root, not as their parent. Coherence is by trace membership and `step_id` attributes/events rather than strict nesting.

2. **Are all execution steps represented?**
   Agent steps: yes — `agent_step` span per loop iteration with `step_id` (`letta/agents/letta_agent_v2.py:944-945`) plus a durable `Step` row storing the same trace id (`letta/services/step_manager.py:157`). Model calls: yes — `llm_request_sent`/`llm_response_received` events with full payloads (`letta/llm_api/llm_client_base.py:261-274`), `@trace_method` client spans, and instrumented outbound HTTP. Tools: yes — events plus a `*._execute_tool` span (`letta/agents/letta_agent.py:1963-1982`). Summarization: yes, traced (`letta/services/summarizer/summarizer.py:74`). Retrieval/embedding: traced (`letta/helpers/pinecone_utils.py:146`). Guardrail evaluations: **no evidence found** — Letta has no first-class guardrail primitive; searching `letta/**` for "guardrail" returned no implementation.

3. **Do handoffs and subagent calls nest correctly?**
   For in-process multi-agent patterns, mostly yes: sleeptime/participant agents are dispatched with `safe_create_task` inside `@trace_method`-decorated group methods (`letta/groups/sleeptime_multi_agent_v3.py:159-187`), and because `asyncio.create_task` snapshots the ambient OTel context (`letta/utils.py:1200`), the participant agent's step spans land in the originating trace. However, each participant gets its own `Run` record (`sleeptime_multi_agent_v3.py:167-175`), so run-level identity and trace-level identity diverge — the run tree in the database is not the same tree as the trace tree in ClickHouse. Out-of-process execution breaks nesting entirely: LLM batch job polling runs decorated functions in a scheduler loop that starts new roots (`letta/jobs/llm_batch_job_polling.py:41-169`), and sandboxed tools execute in subprocesses/remote sandcodes whose internals produce no spans (only start/finish events on the caller's span, `letta/agents/letta_agent.py:1970-1982`).

4. **Can you follow a request from start to finish?**
   Yes, when the observability stack is deployed: the persisted `Step.trace_id` bridges the relational model to the span store, and `GET /v1/runs/{run_id}/trace` returns the filtered span set including root, TTFT, per-step, and tool spans (`letta/server/rest_api/routers/v1/runs.py:296-307`). The endpoint explicitly tolerates legacy rows missing `trace_id` by scanning recent steps for the first populated value (`runs.py:299-304`). Without ClickHouse configured it degrades with HTTP 501 (`runs.py:289-294`); logs remain correlatable through `dd.trace_id` injection (`letta/log.py:74-86`). You cannot follow a call *into* Letta from an external client, since incoming W3C trace context is never extracted (no propagator usage found in the source).

## Architectural Decisions

1. **Hand-rolled middleware instead of OpenTelemetry FastAPI auto-instrumentation.** The team wrote `_trace_request_middleware` plus a route-dependency enrichment pass registered over all v1 routers (`letta/otel/tracing.py:209-225`), gaining control over span naming, header-to-attribute mapping (`tracing.py:97-112`), and health-check suppression, at the cost of owning ASGI edge cases themselves.
2. **Trace-ID bridging between runtime and relational data.** Persisting `get_trace_id()` onto every `Step` row (`letta/services/step_manager.py:157,219`) makes the run→steps→spans join possible from the public API without requiring callers to know OTEL ids — the pivotal design enabling the trace viewer endpoint.
3. **Span-name conventions as a query interface.** The ClickHouse UI filter hard-codes `agent_step`, `%._execute_tool`, root spans, and `time_to_first_token` (`letta/services/clickhouse_otel_traces.py:60-78`), making span names a de facto contract enforced across agent implementations (`letta_agent.py:293`, `letta_agent_v2.py:944`, `letta_agent_v3.py` checkpoints).
4. **Decorator-first instrumentation strategy.** `@trace_method` provides uniform naming (`Class.method`), argument capture with exclusion lists, cancellation diagnostics (`tracing.py:401-421`), and a no-op path before init (`tracing.py:391-392`), applied uniformly across ~100+ call sites (clients, managers, jobs, groups).
5. **Two parallel trace stores with different purposes.** Full-fidelity OTEL spans flow collector → ClickHouse `otel_traces` for the trace viewer, while raw provider payloads flow directly to a denormalized `llm_traces` table for cost analytics, deliberately "bypass[ing] OTEL for large payloads" (`letta/settings.py:389-394`). The cost is a correlation gap: the analytics row's `trace_id` column is written as `None` (`provider_trace_backends/clickhouse.py:124`).
6. **Opt-out tracing in tests.** `is_pytest_environment()` short-circuits `setup_tracing` (`tracing.py:150-151`, `resource.py:59-60`), keeping CI free of exporters while unit tests still exercise decorator logic by flipping the internal flag (`tests/test_otel_tracing.py:49-65`).

## Notable Patterns

- **Events instead of child spans for intra-step detail.** Tool execution and LLM timing are recorded as typed events (`tool_execution_started/completed`, `llm_request_ms`, `step_ms`, `time_to_first_token_ms`) on the step span (`letta/agents/letta_agent_v2.py:972-996`, `letta/agents/letta_agent.py:1946-1982`) — cheap and robust, but not addressable as independent nodes.
- **Attribute hygiene as a first-class concern.** Skip-lists for known-large parameters with ID extraction previews (first 5 element ids plus total count, `tracing.py:294-329`), per-value and total size budgets, and serialization-failure fallback strings (`tracing.py:359-381`).
- **Trace ID surfaced to API consumers.** Error responses embed the current `trace_id` (`tracing.py:142`, `app.py:487`), giving support/users a handle to grep the trace store.
- **Log-trace unification via Datadog-compatible fields.** The JSON formatter injects `dd.trace_id`/`dd.span_id` derived from the active OTEL span so OTEL spans and logs correlate in Datadog (`letta/log.py:61-86`).
- **Cancellation-aware tracing.** `asyncio.CancelledError` is recorded with task name and timestamp on the enclosing span rather than silently dropped (`tracing.py:401-421`).
- **Resource decoration for multi-env filtering.** Service name suffixed by `ENV_NAME`, with `deployment.environment` normalization for Datadog APM (`letta/server/rest_api/app.py:823-824`, `letta/otel/resource.py:41-56`).

## Tradeoffs

- **Flat step hierarchy vs. strict nesting.** Because `agent_step`/TTFT spans are never activated as current context, downstream `@trace_method` and HTTP-instrumentor spans become siblings under the request root (`letta/agents/letta_agent.py:293` vs `tracing.py:394`). Benefit: no risk of orphaned spans if a checkpoint forgets to close the step span; cost: a waterfall view shows N steps' internals overlapping rather than nesting, and per-step attribution relies on timestamps and `step_id` events instead of tree structure.
- **Full payload fidelity vs. cardinality/size.** `llm_request_sent`/`llm_response_received` events carry entire request/response objects (`llm_client_base.py:227-239`), maximizing debuggability but inflating span storage; the separate `llm_traces` table exists precisely because OTEL paths were unsuitable for large payloads (`settings.py:389-394`).
- **In-process coherence vs. cross-process durability.** Ambient-context propagation gives free nesting for background tasks, but `BatchSpanProcessor` buffers are in-memory only — a crash loses the tail of the run's trace, and any future move of step execution out of the request process would sever the run tree silently.
- **Convention-over-schema coupling.** Hard-coded span-name filters make the trace viewer zero-config, yet renaming a span or adding new span kinds requires editing SQL (`clickhouse_otel_traces.py:60-78`) — an easy thing to miss since no test covers the filter.

## Failure Modes / Edge Cases

- **Missing ClickHouse = degraded observability.** Trace retrieval returns 501 unless `CLICKHOUSE_ENDPOINT` is set (`runs.py:289-294`); the OTEL pipeline itself also depends on the sidecar collector being reachable (OTLP export failures are otherwise silent to the request path).
- **Steps without trace ids.** Older rows may lack `trace_id`; mitigated by scanning up to 25 recent steps for the first populated value, which fails outright if *all* steps lack it (empty array returned, `runs.py:299-304`).
- **Multi-step runs spanning multiple requests.** Each HTTP request mints a fresh root trace, so a multi-turn conversation produces many disjoint traces; the run-level view stitches them only via the first found `trace_id`, meaning later requests' spans are invisible in that view (by design of the filter, `runs.py:301-307`).
- **Background-task loss window.** Sleeptime participant steps inherit the request trace, but if the process dies after `create_run` and before span flush, the run exists in Postgres with spans lost from ClickHouse (`sleeptime_multi_agent_v3.py:167-187`).
- **Analytics correlation hole.** `LLMTrace.trace_id=None` means cost-analytics rows cannot be joined back to the OTEL waterfall except indirectly through `step_id` (`provider_trace_backends/clickhouse.py:123-124`); the code even warns "ProviderTrace missing id - trace correlation across backends will fail" for the analogous case (`clickhouse.py:111`).
- **Serialization blowups contained, not impossible.** `repr()` of pathological objects is guarded with recursion/memory fallbacks (`tracing.py:359-364`), and total-size budgeting halts attribute capture with a `parameters.truncated` marker (`tracing.py:283-286`).

## Future Considerations

- Activate the `agent_step` span (e.g., wrap step bodies in `trace.use_span(agent_step_span)`) so tool and LLM spans nest under their step, matching what the trace-viewer filter implies.
- Extract incoming W3C context in `_trace_request_middleware` (via `TraceContextTextMapPropagator.extract`) so upstream callers can join traces end-to-end.
- Populate `LLMTrace.trace_id` from `get_trace_id()` at write time to stitch analytics rows to waterfalls.
- Add tests asserting parent-child relationships and the UI-span filter contract; existing tests only cover attribute sizing (`tests/test_otel_tracing.py`).
- Consider a durable exporter path (file/queue) if agent execution ever moves out of the request process, preserving run-tree continuity across restarts.

## Questions / Gaps

- **No evidence found** for guardrail-evaluation spans: Letta exposes no guardrail abstraction in this snapshot (searched `letta/**` for "guardrail"); closest analogues are tool-rule enforcement (`letta/agents/letta_agent.py:1900-1919`), which is untraced.
- **No evidence found** for a hosted trace-viewer UI in this repository beyond the REST endpoint and OpenAPI description (`fern/openapi.json:16321`); whether app.letta.com renders these spans could not be verified from this source alone.
- Whether the production Datadog path (ddtrace) coexists with or replaces the OTEL pipeline at runtime is implied (`letta/log.py:61-72`, `settings.py:543`) but not fully determinable from configuration files in this repo.
- The comment "We assume trace_id is stable across all steps in a run" (`runs.py:299-300`) is an assumption, not an invariant enforced in code; no test verifies it.

---

Generated by `10.01-span-hierarchy-run-tree` against `letta`.
