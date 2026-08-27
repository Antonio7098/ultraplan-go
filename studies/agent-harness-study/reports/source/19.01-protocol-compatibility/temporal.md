# Source Analysis: temporal

## 19.01 Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.26, Protobuf/gRPC, gRPC-Gateway, Nexus RPC, OTel |
| Analyzed | 2026-08-26 |

## Summary

Temporal is a durable workflow orchestration platform, not an LLM agent harness. Protocol compatibility is therefore centered on **workflow durability and RPC**, not on agent-tool interop. It has **mature, production-grade OpenTelemetry OTLP** support (gRPC, env + YAML, retries, per-service TracerProviders), **first-class Nexus RPC** (open-source `nexus-rpc/sdk-go` spec) as its cross-service async operation protocol, **Protobuf/gRPC + gRPC-Gateway/HTTP + `protojson`** as its primary serialization plane, and **OpenAPI docs served at runtime** (`/swagger.json`, `/openapi.yaml` via `go.temporal.io/api/temporalproto/openapi`). It has **zero MCP support**, **no JSON Schema-driven tool-schema generator**, and **no provider-independent LLM tool adapter** — workflow "tools" are Activities/Child Workflows/Nexus Operations typed as Protobuf messages, not JSON-Schema function declarations. Adding an external tool without a custom adapter is not the Temporal way: every external integration is intentionally wrapped as an Activity with an explicit adapter.

## Rating

**5/10 — Present but inconsistent for this dimension.**

Rationale: OTLP + Nexus + Protobuf/gRPC + OpenAPI serving are clear, tested, extensible, and operationally safeguarded (retries, TLS, sampler, propagators). However the dimension's core questions — MCP, JSON Schema tool schemas, OpenAPI import for tool creation, and portable provider-independent tool definitions — are largely **out-of-scope or absent by design** for a workflow engine. Nexus is a strong open protocol, but it is not MCP; OpenAPI is served not imported; JSON Schema is unused; provider portability is meaningless here. Score reflects excellence on the protocols Temporal *does* care about, penalized for non-coverage of agent-harness protocols.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| OTLP Dependencies (traces) | `go.opentelemetry.io/otel v1.44.0`, `otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0`, `otel/otlptrace v1.43.0`, `otel/sdk v1.43.0`, `otel/trace v1.44.0` | `go.mod:60-68` |
| OTLP Dependencies (metrics+infra) | `otlpmetric/otlpmetricgrpc v1.43.0`, `otel/exporters/prometheus`, `collector/pdata`, `contrib/grpc/otelgrpc`, `proto/otlp` | `go.mod:58-67` |
| OTEL Config Model (YAML) | `ExportConfig{Connections []connection, Exporters []exporter}`, `exporter.Kind{Signal,Model,Protocol}`, `otlpGrpcSpanExporter`/`otlpGrpcMetricExporter` with `Connection`, `Headers`, `Timeout`, `Retry{Enabled,InitialInterval,MaxInterval,MaxElapsedTime}`, shared-conn wrappers deferring `grpc.Dial` to `Start()` | `common/telemetry/config.go:57-160` |
| OTEL YAML Unmarshal (exporter) | Descriptor `signal+model+protocol` switch: `traces+otlp+grpc` → `otlpGrpcSpanExporter`, `metrics+otlp+grpc` → `otlpGrpcMetricExporter`; error on unsupported | `common/telemetry/config.go:391-418` |
| OTEL YAML Unmarshal (connection) | `kind: grpc` → `grpcconn`; `dialOpts()` sets `ReadBufferSize 32KiB`, `WriteBufferSize 32KiB`, `MinConnectTimeout 10s`, `backoff.DefaultConfig`, `Insecure`/`Block`/`Authority` | `common/telemetry/config.go:369-388`, `common/telemetry/config.go:185-210` |
| OTEL Exporter Construction (spans) | `buildOtlpGrpcSpanExporter` builds `otlptracegrpc.WithEndpoint/WithHeaders/WithTimeout/WithDialOption/WithRetry(Enabled,InitialInterval 5s,MaxInterval 30s,MaxElapsedTime 1m)`; `WithInsecure` workaround for #2940; shared-conn vs direct | `common/telemetry/config.go:292-325` |
| OTEL Exporter Construction (metrics) | `buildOtlpGrpcMetricExporter` analogous with `otlpmetricgrpc`; same retry defaults | `common/telemetry/config.go:256-290` |
| OTEL Shared-Conn Start | `sharedConnSpanExporter.Start()` / `sharedConnMetricExporter.Start()` — `sync.Once` Dial then `WithGRPCConn(cc)` | `common/telemetry/config.go:327-355` |
| OTEL Env Exporters | `SpanExportersFromEnv`: `OTEL_TRACES_EXPORTER=otlp` → `otlptracegrpc.NewUnstarted()`, `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` only `grpc` supported else `unsupportedTraceExporterProtocol` error; `none` ignored | `common/telemetry/env.go:28-60` |
| OTEL Service Name | `ResourceServiceName(rsn, envVars)`: maps `internal-frontend→frontend`, prefix `io.temporal` or `OTEL_SERVICE_NAME` | `common/telemetry/env.go:62-79` |
| OTEL Config Tests | `basicOTLPTraceOnlyConfig` YAML (headers/timeout/retry/connection), `sharedConnOTLPConfig` (named `conn1`), `TestOTLPTraceGRPC`, `TestExportersWithSharedConn` | `common/telemetry/config_test.go:12-132` |
| OTEL Env Tests | `when env specifies valid OTEL exporter type, add exporter`; invalid protocol → `unsupported OTEL exporter protocol`; `none` ignored | `common/telemetry/env_test.go:13-62` |
| OTEL Docs | `otel:` YAML examples for local `otel-collector localhost:4317` and Honeycomb `api.honeycomb.io:443`; mentions env vars `OTEL_TRACES_EXPORTER`, `OTEL_EXPORTER_OTLP_TRACES_*`; only `traces+otlp+grpc` supported note | `docs/development/tracing.md:40-112` |
| gRPC Instrumentation (server) | `NewServerStatsHandler(tp, tmp, logger)` → `otelgrpc.NewServerHandler(WithPropagators, WithTracerProvider)` or `nil` if `noop`; `customServerStatsHandler` `TagRPC/HandleRPC` annotates workflowID/runID + debug payloads | `common/telemetry/grpc.go:37-91` |
| gRPC Instrumentation (client) | `NewClientStatsHandler(tp, tmp)` → `otelgrpc.NewClientHandler` | `common/telemetry/grpc.go:58-74` |
| HTTP Instrumentation | `NewHTTPClientTransport` / `NewHTTPHandler` via `otelhttp.NewTransport/NewHandler`, `WithHTTPClientSpanAttributes`, debug mode `TEMPORAL_OTEL_DEBUG` capturing headers/payloads | `common/telemetry/http.go:19-122` |
| Nexus RPC Dependency | `github.com/nexus-rpc/sdk-go v0.6.0`, `nexus-proto-annotations v0.1.0` | `go.mod:41-42` |
| Nexus Payload Serializer | `PayloadSerializer nexus.Serializer` — `Serialize` maps Temporal `Payload.metadata.encoding` (`json/plain`, `json/protobuf`, `binary/protobuf`, `binary/plain`, `binary/null`, `unknown/nexus-content`) to Nexus `Content.Header["type"]` with MIME negotiation; `Deserialize` reverse | `common/nexus/payload_serializer.go:15-174` |
| Nexus Failure Translation | `TemporalFailureToNexusFailure` / `NexusFailureToTemporalFailure` via `protojson.Marshal/Unmarshal` + base64, `HandlerError` retryable override, `OperationError`→`CanceledFailure`/`ApplicationFailure` | `common/nexus/failure.go:56-288` |
| Nexus HTTP Trace Provider | `HTTPClientTraceProvider` (`NewTrace`, `NewForwardingTrace`) with `system.nexusHTTPTraceConfig` (Enabled, MinAttempt 2, MaxAttempt 2, Hooks) | `common/nexus/trace.go:13-78` |
| Nexus Span Annotation | `AnnotateServerSpan` sets `nexus.service/operation/endpoint/request_id`, `AnnotateClientRequest` sets `nexus.namespace/request_id` | `common/nexus/nexusrpc/telemetry.go:11-51` |
| Nexus Operation Processor (CHASM) | `nexusOperationProcessorAdapter[I]` wrapping `NexusOperationProcessor` with `nexus.NewHandlerErrorf(BadRequest/Internal/NotFound)` | `chasm/nexus_operation_processor.go:81-198` |
| OpenAPI Serving (handler) | `OpenAPIHTTPHandler` serves `openapi.OpenAPIV2JSONSpec` at `/swagger.json` + `openapi.OpenAPIV3YAMLSpec` at `/openapi.yaml` (gzip), rate-limited by `configs.OpenAPIV2APIName/OpenAPIV3APIName` | `service/frontend/openapi_http_handler.go:17-68` |
| OpenAPI Registration | `fx.Invoke(RegisterOpenAPIHTTPHandler)`, `RegisterOpenAPIHTTPHandler(logger, router)` → `h.RegisterRoutes(router)` | `service/frontend/fx.go:135`, `service/frontend/fx.go:1019-1030` |
| OpenAPI Config Names | `OpenAPIV3APIName = "/temporal.api.openapi.v1.OpenAPIService/GetOpenAPIV3Docs"`, `OpenAPIV2APIName = ...GetOpenAPIV2Docs` | `service/frontend/configs/quotas.go:16-17` |
| OpenAPI HTTP Tests | `TestHTTPAPI_Serves_OpenAPIv2_Docs` hits `/swagger.json`, `TestHTTPAPI_Serves_OpenAPIv3_Docs` hits `/openapi.yaml` | `tests/http_api_test.go:428-446` |
| Protobuf/ProtoJSON | `protojson.Marshal/Unmarshal` used for failures (`common/nexus/failure.go:125`), `CommonLink` conversions, `proto.Marshal` for payloads | `common/nexus/failure.go:19`, `common/nexus/payload_serializer.go:164` |
| Temporal API Source | `go.temporal.io/api v1.63.5` (generated stubs; `api/` is localsymlinked via `api/go.mod replace`) | `go.mod:69` |
| MCP Absence (grep) | `grep -rn "MCP\|mcp" --include="*.go"` returns zero hits in `sources/temporal`; `go.mod` has no `mcp-*` dep; `dimensions/04.07-external-tool-protocols-mcp` not applicable | `go.mod:1-90` (no mcp), observed grep |
| JSON Schema Absence | Grep `JSON Schema|jsonSchema|JsonSchema` → 0 hits; schema validation is Protobuf + `Payload` encoding, not JSON Schema | observed grep |
| Temporal FX DI for OTEL | `temporal/fx.go` wires `otel.TracerProvider`, `propagation.TextMapPropagator`, `resource.New` with `semconv.ServiceNameKey`, per-service `TracerProvider` (frontend/history/matching/worker) avoiding global | `temporal/fx.go:13-22` (imports), `docs/development/tracing.md:129-134` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**

- **OTLP / OpenTelemetry — mature.** YAML + env (`OTEL_TRACES_EXPORTER`, `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL`) → `otlptracegrpc.NewUnstarted()` (`common/telemetry/env.go:51`) plus fully-modeled YAML exporters (`common/telemetry/config.go:391-418`) supporting `signal=traces|metrics`, `model=otlp`, `protocol=grpc` only; gRPC dial sharing with retry backoff (`common/telemetry/config.go:185-325`). gRPC (`otelgrpc` — `common/telemetry/grpc.go:51-72`) and HTTP (`otelhttp` — `common/telemetry/http.go:70-122`) instrumentation with W3C `propagation.TraceContext`. Docs confirm `model=otlp, protocol=grpc` only and acknowledge the limitation (`docs/development/tracing.md:36`, `docs/development/tracing.md:88-108`).

- **gRPC / Protobuf + gRPC-Gateway / HTTP + protojson — primary.** `google.golang.org/grpc v1.80.0` (`go.mod:84`), `grpc-gateway/v2 v2.29.0` (`go.mod:33`), `protobuf v1.36.11` (`go.mod:85`). All Temporal APIs are Protobuf (`api/`), payloads are `commonpb.Payload` with `encoding` metadata; `protojson` is used for Nexus↔Temporal failure round-trip (`common/nexus/failure.go:125`) and for debug span attributes (`common/telemetry/grpc.go:104`).

- **Nexus RPC — first-class, open protocol.** `nexus-rpc/sdk-go v0.6.0` (`go.mod:42`) with `nexus-proto-annotations`. Temporal implements the Nexus open spec for cross-cluster async operations (start/cancel/describe/poll) over HTTP. Serializer (`common/nexus/payload_serializer.go:15-174`), failure converter (`common/nexus/failure.go`), HTTP trace provider (`common/nexus/trace.go`), and CHASM adapter (`chasm/nexus_operation_processor.go:81`) are in-tree. This is Temporal's answer to "protocol adapter code" in this dimension.

- **OpenAPI — served, not imported.** Generated specs embedded from `go.temporal.io/api` (`service/frontend/openapi_http_handler.go:10`) served at `/swagger.json` (OAS2) and `/openapi.yaml` (OAS3) with gzip and rate limiting (`service/frontend/openapi_http_handler.go:56-67`). Quota names codified (`service/frontend/configs/quotas.go:16-17`). Tests hit both endpoints (`tests/http_api_test.go:428-446`). Used for human/API discoverability and client code-gen, not for importing external tool definitions.

- **Not supported (natively):** **MCP** (zero code, zero dep — grep empty), **JSON Schema** for tool/parameter validation (no generator, no draft validators), **OpenAPI import for tool creation** (only export), **provider-independent LLM tool schema** (no `Tool`, `FunctionDefinition`, or model adapter layer).

**2. Is MCP supported?**

**No.** No MCP client or server, no `tools/list`, `resources`, or `prompts` handlers, no `mcp-*` Go module. `grep -rn mcp --include="*.go"` in `sources/temporal` returns empty; `go.mod:1-90` confirms absence. The closest analogue is **Nexus RPC** (`nexus-rpc/sdk-go` — `go.mod:41-42`), which provides the same "external service as a capability" primitive (operation start / async completion / cancel) but over a different wire spec (Nexus HTTP, not MCP stdio/SSE). Adding an external capability still requires writing an Activity/Nexus adapter — there is no MCP registry or discovery.

**3. Is OpenTelemetry supported?**

**Yes — mature and operationally safeguarded.** Evidence of a complete OTLP pipeline:

- Exporters: both `otlptracegrpc`/`otlpmetricgrpc` (`go.mod:61-62`) plus env fallback (`common/telemetry/env.go:51`) and YAML construction with `Timeout 10s`, `Retry(Enabled, InitialInterval 5s, MaxInterval 30s, MaxElapsedTime 1m)` (`common/telemetry/config.go:263-305`).
- Shared gRPC connections with `sync.Once` deferred dial (`common/telemetry/config.go:327-355`), per-connection `ReadBufferSize/WriteBufferSize/MinConnectTimeout` tuning (`common/telemetry/config.go:185-210`).
- Instrumentation: `otelgrpc` server+client handlers (`common/telemetry/grpc.go:42-74`), `otelhttp` for Nexus/HTTP (`common/telemetry/http.go:89-122`), plus `customServerStatsHandler` enriching spans with `temporalWorkflowID/RunID` and Nexus attributes (`common/telemetry/grpc.go:160-183`, `common/nexus/nexusrpc/telemetry.go:20-36`).
- Configuration dualism: YAML `otel:` stanza (`docs/development/tracing.md:40-58`) and `OTEL_*` env vars with env-wins-yaml precedence (`docs/development/tracing.md:110-111`).
- Tests: `common/telemetry/config_test.go:106-132`, `common/telemetry/env_test.go:13-62`, `common/telemetry/grpc_test.go:28-53`, `common/telemetry/http_test.go:15-237`.

Caveats: only `protocol=grpc` is valid (`common/telemetry/config.go:403-414` errors on else; `common/telemetry/env.go:43-47` rejects non-grpc); metrics/logs OTLP is sparser than traces; debug attributes are opt-in via `TEMPORAL_OTEL_DEBUG` (`common/telemetry/config.go:25-26`).

**4. Are tool schemas portable across providers?**

**Not applicable as an LLM-tool concern; within its own domain Temporal's invocation schemas *are* portable, but not in the dimension's sense.**

Temporal has no LLM provider concept and therefore no `Tool ↔ {OpenAI, Anthropic, Gemini}` schema translator. Work is modeled as **Activities** (`activitypb`), **Child Workflows**, and **Nexus Operations** whose inputs/outputs are typed Protobuf / `commonpb.Payloads`. Portability across "providers" in this codebase means: any language SDK (Go/Java/Python/TypeScript — e.g., `go.temporal.io/sdk v1.44.0` — `go.mod:71`) can call any workflow/activity because the wire format is canonical `protojson`/`binary/protobuf` and `Payload` encoding negotiation (`common/nexus/payload_serializer.go:58-88` handling `application/json`, `application/x-protobuf`, `application/octet-stream`). The `PayloadSerializer` adapter makes the same Temporal payload callable from Nexus, from gRPC, and from HTTP without rewriting the business logic.

However, there is **no neutral JSON-Schema `ToolDef`** and **no cross-LLM provider adapter** (`Agent → Tool → ProviderModel` mapping). `Payload`'s `encoding` tag is provider-like negotiation but scoped to `json/plain|json/protobuf|binary/protobuf|binary/plain|binary/null|unknown/nexus-content` (`common/nexus/payload_serializer.go:124-157`), not to LLM function-calling schemas. OpenAPI specs are emitted for docs, not consumed to auto-generate Activity stubs. So the answer to "Can external tools be added without writing custom adapters?" is **No** — each external call is an explicit Activity/Endpoint registration (`NexusEndpointClientProvider` — `service/frontend/fx.go:1081`) with custom input/output mapping, which is **intended** for durable execution.

## Architectural Decisions

- **OTLP gRPC-only posture** — Only `traces|metrics + otlp + grpc` is accepted; `http` protocol is rejected with `unsupported exporter kind` (`common/telemetry/config.go:408-414`) and `unsupported OTEL exporter protocol` (`common/telemetry/env.go:46`). Decision favors a single well-tested path (gRPC with `Host`/`Authority`/`Insecure` tuning — `common/telemetry/config.go:185-210`) at the cost of excluding the spec's default `grpc/http` duality; users needing HTTP OTLP must use an `otel-collector` sidecar.

- **Telemetry as struct-unmarshaled YAML with generic `any Spec`** — `connection.Spec any` (`common/telemetry/config.go:66`) + `exporter.Spec any yaml:"-"` (`common/telemetry/config.go:97`) plus custom `UnmarshalYAML` overlays (`common/telemetry/config.go:369-418`) allow heterogeneous `grpcconn` vs future transports without mapstructure. Tradeoff: type switches (`switch spec := expcfg.Spec.(type)` — `common/telemetry/config.go:221-229`) are runtime-checked and two signals reuse distinct wrapper structs (`otlpGrpcSpanExporter` vs `otlpGrpcMetricExporter` — `common/telemetry/config.go:113-118`) even though their YAML is almost identical.

- **Nexus as the external-tool protocol** — Rather than adopting MCP, Temporal standardizes on Nexus RPC (`nexus-rpc/sdk-go` — `go.mod:42`), with explicit `PayloadSerializer` (`common/nexus/payload_serializer.go:15-174`) and `failure.go:99-184` providing the semantic bridge to Temporal's durability model (failures, retry behaviors, links). Nexus's `HandlerErrorType{BadRequest,NotFound,Internal,...}` maps cleanly to gRPC `codes.*` (`common/nexus/failure.go:324-434`), preserving error taxonomy across the boundary.

- **OpenAPI as documentation, not contract-import** — Specs are **generated upstream** in `go.temporal.io/api/temporalproto/openapi` and **served gzipped** (`service/frontend/openapi_http_handler.go:41-51`) behind `RateLimitInterceptor`. No importer turns an external OpenAPI into Activities/Workflows: the direction is server→spec, not spec→server. This keeps the server as the source of truth.

- **Per-service `TracerProvider` DI (no global)** — `temporal/fx.go` provides up to four `trace.TracerProvider` instances (frontend/history/matching/worker) and they are threaded via `fx`; the docs explicitly reject the OTEL global `TracerProvider` (`docs/development/tracing.md:129-134`). `NoopTracerProvider` sentinel (`common/telemetry/config.go:44-47`) makes instrumentation a nil-check (`isEnabled` — `common/telemetry/config.go:49-55`). Good for multi-service single-process tests, but requires every constructor to accept a provider.

## Notable Patterns

- **Deferred gRPC dial for exporters** — `sharedConnSpanExporter`/`sharedConnMetricExporter` store `baseOpts` + `dialer` and `sync.Once` Dial on `Start()` (`common/telemetry/config.go:133-149`, `common/telemetry/config.go:327-355`), avoiding `grpc.NewClient` at construction/startup time and enabling YAML `connections:` reuse across span+metric exporters (`common/telemetry/config_test.go:34-55` covers `connection_name: conn1` sharing).

- **Content-negotiation serializer** — `payloadSerializer.Serialize/Deserialize` treat Nexus `Content.Header["type"]` as MIME (`mime.ParseMediaType` — `common/nexus/payload_serializer.go:52`) and dispatch on `application/x-temporal-payload | application/json (±protobuf) | application/x-protobuf | application/octet-stream | binary/null`. Unknown types round-trip as `unknown/nexus-content` with headers preserved (`common/nexus/payload_serializer.go:92-97`), making the adapter tolerant to new encodings.

- **Telemetry-aware Nexus HTTP start** — `LoggedHTTPClientTraceProvider.NewTrace(attempt, logger)` (`common/nexus/trace.go:74-78`) gates `httptrace.ClientTrace` by `Enabled/MinAttempt/MaxAttempt/Hooks`, mirroring OTel span creation in the task path (`chasm/lib/callback/invocable_outbound.go:80-91` via `commonnexus.HTTPClientTraceProvider`).

- **Failure JSON↔Proto cycle** — `TemporalFailureToNexusFailureInPlace` clones (`proto.Clone` — `common/nexus/failure.go:100`) then `protojson.Marshal` into `nexus.Failure{Metadata:type, Details: data}` and back via `protojson.Unmarshal` with `DiscardUnknown` (`common/nexus/failure.go:200-207`). The `type` metadata key acts as a discriminant; unrecognized types fall back to `ApplicationFailure` with serialized `nexus.Failure` as `json/plain` details (`common/nexus/failure.go:290-316`).

## Tradeoffs

- **Durability vs. plug-and-play** — Every external integration must be an Activity/Nexus endpoint with explicit retry/timeout/heartbeat semantics. This eliminates the "drop-in tool" promise that MCP/JSON Schema aim for, but buys determinism, replay, and exactly-once-like accounting for side effects — the core value prop.

- **OTLP completeness vs. protocol breadth** — Full supplier of traces+metrics over gRPC with `Host/Authority/Insecure/Block/Backoff` (`common/telemetry/config.go:69-88`) and retry, plus exhaustive debug attribute plumbing (`common/telemetry/grpc.go:97-157`, `common/telemetry/http.go:172-258`). But HTTP OTLP, OTLP/log, and configurable sampler beyond env/defaults are not surfaced in YAML; operators must deploy a collector for model/protocol mediation.

- **Nexus investment vs. ecosystem reach** — Deep Nexus stack (serializer, links, endpoint registry — `common/nexus/links.go:13-61`, `common/nexus/endpoint_registry.go`) is cohesive but positions Temporal outside the larger MCP ecosystem. No compatibility shim translates `tools/list` → `nexus.Service`.

- **OpenAPI docs vs. contract-first** — Serving specs is cheap and always in sync with the server build, but without an OpenAPI→Activity importer, teams consuming third-party APIs still hand-write wrappers validated only at call time, not at build time.

## Failure Modes / Edge Cases

- **YAML mis-descriptor** — `kind.signal/model/protocol` triple not exactly `traces|metrics + otlp + grpc` returns `unsupported exporter kind: signal=..., model=..., protocol=...` (`common/telemetry/config.go:408-414`); empty or nil config silently yields zero exporters (`common/telemetry/config_test.go:57-62`), which masks typos (no tracing, no error at the process level unless explicitly validated).

- **Missing named connection** — `connection_name: conn1` referencing absent `connections:` entry returns `OTEL exporter connection %q not found` (`common/telemetry/config.go:284-287`, `common/telemetry/config.go:318-320`).

- **Env-only protocol guard** — `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL != grpc` → `unsupported OTEL exporter protocol` (`common/telemetry/env.go:43-47`); `OTEL_TRACES_EXPORTER` containing `none` is silently ignored (`common/telemetry/env.go:52-53`), while unknown values error at startup.

- **Insecure mode footgun** — `Insecure: true` opts into `grpc.WithTransportCredentials(insecure.NewCredentials())` (`common/telemetry/config.go:200-202`) and additionally `WithInsecure()` for the OTLP exporter workaround (`common/telemetry/config.go:274-276`, `common/telemetry/config.go:309-311`). No mutual-exclusion check with `Authority`/`tls`-ish headers.

- **Payload encoding fallback** — `Deserialize` tolerates missing or unparsable `type` headers by writing `unknown/nexus-content` (`common/nexus/payload_serializer.go:42-88`) and `Serialize` preserves all non-`encoding` metadata in that path (`common/nexus/payload_serializer.go:125-130`). `xTemporalPayload` fallback marshals with `payload.Marshal()` — if that fails, `serializer error: payload marshal error` is returned and the Nexus caller sees a `HandlerErrorTypeInternal` via `nexus.NewHandlerErrorf` wrappers in CHASM (`chasm/nexus_operation_processor.go:85-94`).

- **Debug tracing unbounded buffers** — `payloadCapture.annotateHeaders/payloadCapturingReadCloser` under `TEMPORAL_OTEL_DEBUG` buffers full request/response payloads without size limit (`common/telemetry/http.go:282-298`), which can OOM on large Nexus payloads.

- **Rate-limited OpenAPI serving** — `OpenAPIHTTPHandler.RegisterRoutes` checks `RateLimitInterceptor.Allow(apiName)` (`service/frontend/openapi_http_handler.go:36`) and returns `429 Too Many Requests` on quota exhaustion; gzip reader creation failures are mapped to `500` with `logger.Error` (`service/frontend/openapi_http_handler.go:41-45`), but `io.Copy`/`rdr.Close` checksum errors only log without propagating to the client.

## Future Considerations

- **MCP bridge (Nexus↔MCP shim)** — Build a thin adapter in `common/nexus/mcp/` that translates MCP `tools/list`/`tools/call` to `nexus.Service` registrations (using existing `PayloadSerializer` and `links.go:13` patterns), without changing the durability model. Keep Nexus as canonical; expose MCP as an ingress concern alongside `service/frontend/openapi_http_handler.go:33`.

- **OpenAPI importer for Activities** — Add a `tools/schema gen --openapi <spec>` command reusing `grpc-gateway` codegen and `payloadSerializer` MIME logic to emit Activity definitions with typed inputs. This would invert the current one-way `serve` direction and address "spec→tool" gap.

- **JSON Schema adapter (optional)** — If a future agent-scale integration is needed, generate JSON Schema Draft 2020-12 from `commonpb.Payload`/`api/temporal` Protobuf descriptors (leveraging `google.golang.org/protobuf/reflect/protoreflect` already in `cmd/tools/getproto/files.go:8`) instead of introducing a new validator; reuse `protojson` for fidelity.

- **OTLP protocol expansion** — Extend `exporter.UnmarshalYAML` (`common/telemetry/config.go:402-414`) to accept `traces+otlp+http` by wiring `otlptracehttp.New` behind the same retry/connection model, bringing parity with `common/telemetry/env.go` specs and eliminating the mandatory collector hop for HTTP-only vendors.

- **Telemetry contract hardening** — Surface sampler (`ParentBased/TraceIDRatio`) and `spanProcessor` tuning currently hidden behind defaults (`common/telemetry/config.go:35-42` constants) as explicit YAML fields; add a `temporal otel validate` CLI check that fails instead of silent zero-exporter on typo.

## Questions / Gaps

- **No evidence found: MCP semantics** — Searched `go.mod:1-90`, grep `mcp` over `**/*.go` (0 hits), `common/nexus` (`links.go`, `failure.go`, `nexusrpc/`), `chasm/` (only `NexusOperationProcessor`). Confirmed absent. Question: is Nexus intended to be Temporal's answer to MCP for this dimension, or is MCP adoption on the roadmap as an ingress?

- **No evidence found: JSON Schema generation/validation** — `common/nexus/payload_serializer.go:58-88` uses MIME/media-type branching, not schema validation; `schema/` is DB schemas (`schema/cassandra/README.md:27`, `tools/sql/README.md:29-34`), not JSON Schema. Search `JSON Schema|jsonSchema` returned 0 in-tree hits. Are typed Protobuf messages considered sufficient "tool schemas" for this dimension's portability test?

- **No evidence found: OpenAPI import** — `service/frontend/openapi_http_handler.go:56-67` serves specs; no `importer`, `openapi -> payload`, or `openapi -> activity` generator. Tests only *serve* (`tests/http_api_test.go:428-446`). Is generating activities from third-party OpenAPI specs out-of-scope, or desired?

- **Partial evidence: OTLP metrics/logs parity** — Metrics path exists in `common/telemetry/config.go:235-253` but is thinner than traces (fewer tests, no `env` metric exporter helper like `SpanExportersFromEnv` for metrics). `proto/otlp` includes logs types but no `otlplog` exporter is wired. What is the intended log-signal strategy (COLLECTOR vs direct)?

- **No evidence found: provider-independent tool abstraction** — No `Tool`, `ToolProvider`, or model-adapter interface; tool portability test (`model-independent tool schemas`) does not map to Temporal's SDK-portability model. Should the dimension's rubric be interpreted flexibly for non-agent engines, or strictly penalize mismatch?

---

Generated by `19.01-protocol-compatibility` against `temporal`.
