# Source Analysis: temporal

## Dimension 10.01: Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal server: frontend/history/matching/worker services, gRPC + HTTP/Nexus, Cassandra/SQL persistence) |
| Analyzed | 2026-08-24 |

## Summary

The Temporal server implements distributed tracing with the OpenTelemetry Go SDK, exported exclusively via OTLP/gRPC. Tracing is wired through two fx DI modules: a process-global exporter module (`temporal/fx.go:978-1022`) and a per-service tracing module (`temporal/fx.go:1041-1113`), each service getting its own `TracerProvider`, resource (service name `io.temporal.<service>`, see `common/telemetry/env.go:62-79`), and W3C TraceContext propagator.

The span tree has four layers: (1) gRPC spans from otelgrpc stats handlers on both server and client sides of every internode call; (2) generated OpenTelemetry decorators wrapping every persistence store method; (3) per-task consumer spans in the history service task queues; and (4) otelhttp client/server spans for Nexus operations and workflow completion callbacks over HTTP. Within one synchronous request, the tree is coherent: gRPC → handler → persistence spans nest correctly, and cross-process propagation is verified by end-to-end tests asserting identical TraceIDs and parent/child span relationships (`tests/nexus_otel_test.go`).

The critical structural gap is the async boundary: history tasks are durable and are executed from `context.Background()` (`service/history/queues/executable.go:276-280`), so each task processing attempt starts a brand-new root trace. A single user request therefore does NOT appear as one continuous trace from StartWorkflowExecution through activity dispatch to completion; instead, correlation across traces is achieved via span attributes (`temporalWorkflowID`, `temporalRunID`) and TraceQL queries documented in `docs/development/tracing.md:20-23`. The docs explicitly acknowledge this gap and prescribe remediation patterns (`docs/development/tracing.md:194-226`), but no SpanContext persistence into tasks/events was found in production code.

Note on dimension fit: Temporal is a workflow orchestration server, not an LLM agent harness. There are no model calls, guardrail evaluations, or evals to trace. The analog mapping used here: "runs" = workflow executions, "tools" = activities/Nexus operations, "handoffs/subagents" = child workflows/Nexus operations — all represented as gRPC/HTTP/persistence spans rather than semantically named run-tree spans.

## Rating

**7 / 10**

Rationale: The tracing model is clear and explicit — dedicated fx modules with documented override points (`temporal/fx.go:1024-1041`), a written instrumentation guide (`docs/development/tracing.md`), systematic generated coverage of the entire persistence surface (`common/persistence/telemetry/*_gen.go` from `common/persistence/telemetry/gowrap_template:29-48`), operational safeguards (noop-provider detection, graceful trace-dropping shutdown, exporter retries), and real tests that assert parent-child nesting and trace-ID equality across process boundaries (`tests/nexus_otel_test.go:209-258,332-337`). It falls short of 9-10 because: the run tree breaks at the durable-task boundary so "one trace per user request" is impossible today (documented but unresolved); no sampler configuration exists (searched `common/telemetry/*.go` and `temporal/*.go` for "Sampler" — no matches), leaving only all-or-nothing enablement; only OTLP+gRPC export is supported (`common/telemetry/config.go:402-417`); and the update-registry tracer is injected but never used to start spans (`service/history/workflow/update/util.go:16-29`, no `tracer.Start` calls found in that package).

## Evidence Collected

Every entry includes a file path with line numbers relative to the selected source root (`studies/agent-harness-study/sources/temporal/`).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tracing system | OpenTelemetry Go SDK; OTLP/gRPC trace exporters declared in go.mod | `go.mod:58-68` |
| Exporter config (YAML) | `otel:` stanza parses signal/model/protocol triples; only `traces+otlp+grpc` accepted | `common/telemetry/config.go:391-417` |
| OTLP/gRPC span exporter build | endpoint, headers, timeout, dial options, retry defaults | `common/telemetry/config.go:292-325` |
| Env-var exporters | `OTEL_TRACES_EXPORTER=otlp` creates exporter; protocol must be grpc | `common/telemetry/env.go:29-60` |
| Noop default | NoopTracerProvider/NoopTracer when unconfigured; noop type-check gate | `common/telemetry/config.go:44-55` |
| Process-level exporter module | merges config < env < custom(test) exporters; lifecycle start/stop | `temporal/fx.go:978-1022` |
| Per-service TracerProvider | BatchSpanProcessor per exporter; resource with service name/version/instance; shutdown drops traces after 1s | `temporal/fx.go:1041-1113` (provider at 1077-1105) |
| Resource service naming | `io.temporal.<service>` prefix, `internal-frontend` mapped to `frontend` | `common/telemetry/env.go:62-79` |
| Propagator | W3C TraceContext only ("Haven't had use for baggage propagation yet") | `temporal/fx.go:1106-1107` |
| gRPC server spans | otelgrpc ServerStatsHandler wrapped by custom handler; nil when disabled | `common/telemetry/grpc.go:41-56` |
| Span annotation with workflow IDs | extracts WorkflowID/RunID from payloads/task tokens onto spans | `common/telemetry/grpc.go:160-183`; tag keys at `common/telemetry/tags.go:13-21`; extraction via `common/rpc/interceptor/logtags/workflow_tags.go:32-63` |
| gRPC client spans | ClientStatsHandler appended to all internode dial options | `common/resource/fx.go:434-449`; server side wired at `service/fx.go:133-142` and `service/frontend/fx.go:340-341` |
| Persistence spans (generated) | every store method wrapped: `persistence.<Store>/<Method>`, attrs store/method/deadline, error recording | `common/persistence/telemetry/gowrap_template:29-48`; e.g. `common/persistence/telemetry/execution_store_gen.go:45`, `task_store_gen.go:45`, `queue_gen.go:46` |
| Persistence decorator wiring | tracer scoped to component `persistence`; decorator applied when enabled | `common/persistence/client/fx.go:214-217`; factory fan-out at `common/persistence/telemetry/data_store_factory.go:27-137` |
| History task spans | `queue.Execute/<TaskType>` consumer-kind span with workflow/run/task attrs; error recorded; payload in debug mode | `service/history/queues/executable.go:282-323` |
| Task execution context reset | new context built from `context.Background()` → fresh root trace per task attempt | `service/history/queues/executable.go:276-280` |
| Tracer scoping per component | queue factories create tracers per component (transfer/timer/visibility/outbound/memory/archival) | `service/history/transfer_queue_factory.go:83`, `timer_queue_factory.go:76`, `visibility_queue_factory.go:72`, `outbound_queue_factory.go:272`, `memory_scheduled_queue_factory.go:81`, `archival_queue_factory.go:107`; engine tracer at `service/history/history_engine.go:231` |
| CHASM transition events | `chasm.transition` span events with source/destination/error attributes in debug mode | `chasm/statemachine.go:55-68` |
| Nexus HTTP client spans | otelhttp transport wrapper injecting W3C traceparent; attribute injection mechanism | `common/telemetry/http.go:67-97,124-145` |
| Nexus HTTP server spans | frontend Nexus dispatch routes instrumented per API name | `service/frontend/nexus_operation_http_handler.go:114-131` |
| Nexus span attributes | endpoint/service/operation/request-id/namespace annotated on client & server spans | `common/nexus/nexusrpc/telemetry.go:20-51`; keys at `common/telemetry/tags.go:17-21`; wiring at `chasm/lib/nexusoperation/fx.go:182-196` |
| Callback trace propagation | CHASM callback external HTTP client instrumented with same transport instrumenter | `chasm/lib/callback/fx.go:27-62` |
| Update registry tracer (latent) | TracerProvider option stored but no `tracer.Start` in package | `service/history/workflow/update/registry.go:160-166`, `util.go:16-29`; provider passed at `service/history/workflow/context.go:1262` |
| Debug mode | `TEMPORAL_OTEL_DEBUG` adds gRPC payloads/headers, persistence request/response JSON, HTTP headers/payloads | `common/telemetry/config.go:26,419-425`; `common/telemetry/grpc.go:93-158`; `gowrap_template:50-69`; `common/telemetry/http.go:172-283` |
| E2E test: callback trace | callback's `traceparent` header equals history-service client span's SpanContext | `tests/nexus_otel_test.go:121-133` |
| E2E test: Nexus operation nesting | history "HTTP POST" client span is parent of frontend `DispatchByEndpoint` server span, same TraceID | `tests/nexus_otel_test.go:209-258` (assertion helper at 461-473) |
| E2E test: worker receives trace ctx | worker-side header extraction yields valid SpanContext matching client span | `tests/nexus_otel_test.go:303-337` |
| E2E test: externally injected trace | fixed `traceparent` honored: server span TraceID/Parent match injected values | `tests/nexus_otel_test.go:359-389` |
| Test exporter injection | `WithSpanExporter` wires InMemoryExporter into cluster config | `tests/testcore/test_env.go:117-122`; `tests/testcore/test_cluster.go:301`; file exporter via `TEMPORAL_TEST_OTEL_OUTPUT` at `tests/testcore/functional_test_base.go:331-336,442-456` |
| Span assertion helpers | stable local trace/span/parent IDs, span filtering | `common/testing/testtelemetry/spans.go:11-104` |
| Trace viewer (dev) | Grafana Tempo service + Grafana datasource provisioning receiving OTLP grpc on 4317 | `develop/docker-compose/docker-compose.yml:78`; `develop/docker-compose/grafana/provisioning/tempo/tempo.yaml`; datasource yml referencing `tempo:3200` |
| Dev quickstart env | `make OTEL=true` sets exporter env, 100ms batch delay, debug on | `Makefile:85-90`; quickstart/TraceQL workflow doc at `docs/development/tracing.md:15-23` |

## Answers to Dimension Questions

1. **Is there a single coherent trace tree?**
   Only within a synchronous request scope. For an inbound API call, gRPC server span → interceptor chain → handler → persistence spans form one coherent tree (`common/telemetry/grpc.go:41-56` + `common/persistence/telemetry/gowrap_template:29-48`), and synchronous cross-process hops stay in one trace via otelgrpc/otelhttp propagation. However, once work becomes durable (history tasks, timers, callbacks), each task executes from `context.Background()` (`service/history/queues/executable.go:276-280`) and produces disconnected root traces. So: no single end-to-end tree for a workflow lifetime; many trees correlated by attributes.

2. **Are all execution steps represented?**
   Broadly yes at infrastructure granularity: every RPC (gRPC and Nexus HTTP) and every persistence call is spanned; history task processing has consumer-kind spans with task ID/type attributes (`service/history/queues/executable.go:299-307`). Gaps: no semantic spans for workflow commands (child workflow start, activity scheduling appear only as persistence/gRPC spans); the update registry holds a tracer but starts no spans (`service/history/workflow/update/registry.go:162-166` vs. no `tracer.Start` in package); no spans inside the matching service task distribution beyond generic gRPC. Guardrail/model-call/eval tracing does not apply (not an agent harness).

3. **Do handoffs and subagent calls nest correctly?**
   Synchronous handoffs do: Nexus operations produce verified parent→child nesting between history-service client spans and frontend/server spans across processes, including cancellation paths (`tests/nexus_otel_test.go:209-258`), and callbacks propagate their trace to the customer HTTP endpoint verbatim (`tests/nexus_otel_test.go:121-133`). Asynchronous continuation (the actual child-workflow execution, which runs as separate history tasks) does not nest under the originating trace because task execution resets the context.

4. **Can you follow a request from start to finish?**
   Yes within one request/response cycle (verified by tests). Across the asynchronous workflow lifecycle you follow it indirectly: spans carry `temporalWorkflowID`/`temporalRunID`/`temporalBusinessID` attributes (`common/telemetry/tags.go:13-15`, set at `common/telemetry/grpc.go:170-182` and `service/history/queues/executable.go:287-305`), and the documented TraceQL query `{ .temporalWorkflowID =~ "<WF-ID>.*" }` stitches the fragments together in Grafana Tempo (`docs/development/tracing.md:20-23`). That is attribute-based correlation, not trace-tree continuity.

## Architectural Decisions

- **Per-service TracerProviders instead of a global provider.** Because multiple services can be co-resident in one process, the server deliberately avoids the OTEL global TracerProvider and injects providers via fx (`docs/development/tracing.md:128-133`; implementation `temporal/fx.go:1077-1105`). Common code obtains the right provider from the active span (`docs/development/tracing.md:169-185`).
- **Layered configuration precedence**: YAML config < environment variables < code-injected custom exporters (test hook), merged in `temporal/fx.go:987-1009`.
- **Generated instrumentation for breadth**: gowrap-generated decorators cover every persistence interface method uniformly from a single template (`common/persistence/telemetry/gowrap_template:24-74`), eliminating per-method drift.
- **Zero-cost disablement**: when tracing is off, handlers/instrumenters return nil or noop implementations and call sites skip allocation entirely (e.g., `common/telemetry/grpc.go:40-48`, `service/history/queues/executable.go:282-283` comment "Wrapped in if block to avoid unnecessary allocations when OTEL is disabled", `common/persistence/client/fx.go:215-217`).
- **Debug payload capture as opt-in env flag** (`TEMPORAL_OTEL_DEBUG`) rather than always-on, given payload serialization cost and sensitivity (`common/telemetry/config.go:26,419-425`; header-capture rationale comment at `common/telemetry/http.go:176-178`).
- **Attribute-based async correlation**: rather than persisting SpanContext, spans are tagged with stable business identifiers (workflow/run IDs extracted even from opaque task tokens, `common/rpc/interceptor/logtags/workflow_tags.go:53-63`), pushing join-time correlation to the trace backend.

## Notable Patterns

- **Stats-handler wrapping for enrichment**: `customServerStatsHandler` delegates to otelgrpc and post-annotates spans with workflow tags, error payloads, deadlines, and headers keyed off gRPC stats events (`common/telemetry/grpc.go:87-158`).
- **Request-context attribute injection for HTTP client spans**: `WithHTTPClientSpanAttributes` stashes attributes on the request; a transport reads them when otelhttp creates the recording span (`common/telemetry/http.go:126-145`), used to attach Nexus namespace/request-ID (`chasm/lib/nexusoperation/fx.go:190-196`).
- **Span links recommended for batch processing**: the dev guide directs batch workers to use OTEL span links instead of parentage (`docs/development/tracing.md:220-226`); no production use found yet.
- **Test-first observability contracts**: `testtelemetry` provides deterministic local IDs for asserting exact trace/span/parent topologies (`common/testing/testtelemetry/spans.go:63-87`), and suites assert full expected span lists (`tests/nexus_otel_test.go:394-473`).
- **Graceful degradation on shutdown**: exporter/provider shutdown ignores deadline-exceeded errors since dropping traces on shutdown is acceptable (`temporal/fx.go:1096-1102,1130-1144`).

## Tradeoffs

- **Trace continuity vs. write amplification**: persisting SpanContext into every durable task/event would enable end-to-end trees but adds schema and serialization burden across DB rows; Temporal chose attribute correlation, keeping the hot path clean at the cost of fragmented traces.
- **Uniformity vs. semantics**: generated persistence/gRPC spans give total coverage but low semantic meaning; there are no domain spans like "ScheduleActivity" or "StartChildWorkflow", so users must infer intent from method names and attributes.
- **Simplicity vs. flexibility in export**: supporting only OTLP+gRPC simplifies maintenance (`common/telemetry/config.go:402-417`, `env.go:42-48`) but blocks direct integration with vendors/protocols requiring other bindings.
- **Always-on sampling vs. cost control**: no sampler knob exists; enabling tracing samples everything (default AlwaysSample in the SDK), which is simple but potentially expensive at scale; the Makefile dev profile mitigates only flush latency (`Makefile:86-89`).
- **Debug richness vs. safety/performance**: debug mode records full payloads and all header values verbatim into spans (`common/telemetry/http.go:176-182`, `grpc.go:128-136`) — powerful diagnostics that would leak sensitive data if enabled casually; memory buffering is intentionally unbounded (`common/telemetry/http.go:282-283`).

## Failure Modes / Edge Cases

- **Collector outage**: exporter retries enabled by default (5s→30s, 60s max elapsed, `common/telemetry/config.go:38-41,300-306`); OTEL internal errors are surfaced as warnings only after startup completes (`temporal/fx.go:980-985`). Spans are dropped silently thereafter — no backpressure signal into request handling.
- **Shutdown data loss**: any queued spans not flushed within the 1-second stop timeout are deliberately discarded (`temporal/fx.go:1093-1102`).
- **Misconfiguration fails fast at parse time**: unsupported exporter kinds or protocols return errors during YAML decode / env parsing (`common/telemetry/config.go:402-417`; `env.go:42-56`) rather than emitting nothing quietly.
- **Trace fragmentation under retry/CAN**: continue-as-new and task retries each mint fresh root traces; long-running workflows accumulate hundreds of disjoint traces distinguishable only by shared workflow-ID attributes.
- **Long-poll spans conflation**: `GetWorkflowExecutionHistory` handles both long-poll and short calls under one span name with an override tag distinction (`common/rpc/interceptor/telemetry.go:116-127`) — acknowledged tech debt in comments.
- **Clock/ordering skew**: multi-process traces rely on span start times for ordering in tests (`tests/nexus_otel_test.go:414-416`); cross-host clock skew can reorder parent/child presentation in backends without clock-sync.
- **Noop ambiguity**: `isEnabled` detects noop by concrete type assertion (`common/telemetry/config.go:49-55`); a custom non-noop provider that emits nothing would still be treated as "enabled", paying annotation costs with no output.

## Future Considerations

- Persist a serialized SpanContext (or trace/link metadata) on history tasks and workflow events so async continuations can re-parent or link to the originating trace — the approach is already prescribed in `docs/development/tracing.md:194-226`.
- Expose sampler configuration (e.g., parent-based traceidratio per namespace/service) in the `otel` config stanza; currently absent from `common/telemetry/config.go`.
- Add semantic domain spans for workflow-level operations (activity scheduling, child workflow start, Nexus schedule/cancel) using the existing `io.temporal` attribute conventions (`docs/development/tracing.md:139-167`).
- Activate the already-wired update-registry tracer to emit update state-transition spans (`service/history/workflow/update/registry.go:162-166`).
- Adopt span links in batch executors as suggested by the guide, e.g., in `service/history/queues/executable.go:299` where batches of tasks could link to enqueue-time spans.
- Broaden exporter support beyond OTLP/gRPC or document a collector-based path for other protocols.

## Questions / Gaps

- **How do traces behave under failover/standby cluster task processing?** Standby executors exist (`service/history/timer_queue_standby_task_executor_test.go:2600` uses NoopTracer in tests), but no test asserts trace topology for cross-cluster failover processing. No evidence found in this source for XDC-specific span handling.
- **Does the SDK-side (worker/client) trace join the server trace?** Out of scope for this source (SDK lives in another repo); the server honors inbound `traceparent` (`tests/nexus_otel_test.go:359-389`), implying join capability, but server-repo evidence covers only its own spans.
- **Is there any sampling or per-namespace tracing policy?** Searched `common/telemetry/*`, `temporal/*`, and dynamicconfig keys containing "trace"/"otel"; none found besides enable/disable and the Nexus httptrace logging setting (`common/nexus/trace.go:24-57`, which is log-based httptrace, not OTEL sampling). No clear evidence found.
- **Matching-service internals**: no spans beyond gRPC interceptors were found in `service/matching` (searched for `tracer.Start`/`trace.` usage); if deeper visibility exists, it is not present in this snapshot.

---

Generated by `10.01-span-hierarchy-and-run-tree` against `temporal`.
