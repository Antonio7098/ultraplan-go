# Source Analysis: opa

## 19.01 Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go 1.26, Rego, WASM (wazero) |
| Analyzed | 2026-08-26 |

## Summary

OPA is a policy engine, not an agent harness. It fully implements OpenTelemetry OTLP for tracing and metrics, and JSON Schema (Draft 4/6/7 + 2020-12 generation) for static type-checking and validation, but it has **no native MCP client/server, no OpenAPI importer, and no LLM provider-tool schema** abstractions. Protocol extensibility is via HTTP REST, bundle/WASM, and plugin HTTP handlers, not via agent-tool adapters. The external `opa-mcp-server` (OrygnsCode) is a third-party stdio MCP wrapper that shells out to the OPA CLI/REST API — not in-tree code.

## Rating

**5/10 — Present but inconsistent for this dimension.**

Rationale: OTLP + JSON Schema are mature, tested, and operationally safeguarded (sampling, TLS modes, batch processor tuning, resource attributes). MCP is absent in-tree (only an ecosystem external wrapper); OpenAPI is absent; provider-independent tool schemas do not exist because OPA's "tools" are Rego builtins declared via `types.Function`, not LLM tool calls. The score reflects strong observability + schema support but gaps on the exact protocols the dimension probes for agent harnesses.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| OTLP Trace Dependencies | `go.opentelemetry.io/otel v1.44.0`, `otel/exporters/otlp/otlptrace`, `otlptracegrpc`, `otlptracehttp`, `otel/sdk/trace v1.44.0` | `go.mod:39-44` |
| OTLP Metrics Dependencies | `otel/exporters/otlp/otlpmetricgrpc/http`, `contrib/bridges/prometheus`, `sdk/metric v1.44.0`, `proto/otlp` | `go.mod:37-46` |
| Distributed Tracing Core | `Init()` parses `distributed_tracing` config, creates `otlptracegrpc.NewUnstarted` / `otlptracehttp.NewUnstarted`, builds `resource.New` with `semconv.ServiceNameKey`, `trace.BatchSpanProcessor`, sampler `TraceIDRatioBased` | `internal/distributedtracing/distributedtracing.go:98-192` |
| Tracing Defaults & Validation | Defaults for gRPC `localhost:4317` / HTTP `localhost:4318`, service name `opa`, sample 100%, batch processor `blocking:false`, `batchTimeout 5000ms`, `exportTimeout 30000ms`, `maxExportBatch 512`, `maxQueue 2048`; validation of `type` and `encryption` | `internal/distributedtracing/distributedtracing.go:36-54`, `internal/distributedtracing/distributedtracing.go:225-294` |
| Tracing TLS Adapters | `grpcTLSOption` / `httpTLSOption` select `WithInsecure` vs `WithTLSCredentials`/`WithTLSClientConfig` | `internal/distributedtracing/distributedtracing.go:297-309` |
| Tracing DI Abstraction | `tracing.Options = []any`, `HTTPTracingService` interface with `NewTransport`/`NewHandler`, noop fallback when `tracing==nil` | `v1/tracing/tracing.go:14-55` |
| OTEL HTTP Handler Registration | `features/tracing` blank-import registers `otelhttp.NewTransport`/`NewHandler` via `RegisterHTTPTracing`; `convertOpts` casts to `otelhttp.Option` | `v1/features/tracing/tracing.go:1-34` |
| Server Wiring | `Server.WithDistributedTracingOpts`, `instrumentHandler` wraps with `tracing.NewHandler`, `execQuery`/`v0QueryPath` propagate `DistributedTracingOpts` via `rego.DistributedTracingOpts` | `v1/server/server.go:147`, `v1/server/server.go:424-427`, `v1/server/server.go:954-965`, `v1/server/server.go:1001` |
| Metrics Export via OTLP | `metrics_export` config validated via Rego (`validate.rego`), creates `otlpmetricgrpc`/`otlpmetrichttp` exporter, `metric.NewPeriodicReader`, `otelprometheus.NewMetricProducer` | `internal/metricsexport/metricsexport.go:76-150`, `internal/metricsexport/validate.rego:3` |
| Prometheus Bridge | `Gatherer().WithGatherer` exposed for OTLP metrics path | `internal/metricsexport/metricsexport.go:137-142` |
| OpenTelemetry Logging Bridge | `SetupLogging` sets `otel.SetErrorHandler` + `otel.SetLogger(logr.New(sink))`; `sink` implements `logr.LogSink` (Enabled/Info/Error/WithName) | `internal/distributedtracing/distributedtracing.go:194-197`, `internal/distributedtracing/distributedtracing.go:319-351` |
| JSON Schema Vendor: gojsonschema | Vendored `xeipuuv/gojsonschema` fork in `internal/gojsonschema`; supports Draft 4/6/7 + `Hybrid`; meta-schemas embedded, `parseSchemaURL`, `Draft` enum | `internal/gojsonschema/draft.go:24-60`, `internal/gojsonschema/draft.go:89-122`, `internal/gojsonschema/README.md:5-24` |
| JSON Schema Compilation | `compileSchema(goSchema, allowNet)` creates `gojsonschema.NewSchemaLoader`, sets `AllowNet`, compiles via `sl.Compile(NewGoLoader)` | `v1/ast/compile.go:1630-1645` |
| Schema→Type Mapping | `schemaParser.parseSchema` / `parseSchemaWithPropertyKey` handles `$ref` caching (`definitionCache`, `processing`, `Recursive`), `anyOf`/`allOf`/`mergeSchemas`, typed branches (`object`→`StaticProperty`, `array`→`Array`, scalars→`B`/`S`/`N`) | `v1/ast/compile.go:1688-1878` |
| Schema Application | `SchemaSet`, `loadSchema(raw, allowNet)` → `compileSchema` → `newSchemaParser().parseSchema` | `v1/ast/schema.go:41-54` |
| Type Checking with Schemas | `checkRule` loads `processAnnotation` per `AnnotationSet`, `allowNet` host filter threaded through `Capabilities.AllowNet` | `v1/ast/check.go:219-246`, `v1/ast/check.go:1471-1493` |
| JSON Schema Generation | `internal/genjsonschema.Builder` reflects Go structs → Draft 2020-12, `$defs`, `OrderedMap`, `TypeResolver`, `MakeNullable`, `AllowAdditionalProperties` | `internal/genjsonschema/genjsonschema.go:1-60`, `internal/genjsonschema/genjsonschema.go:124-309`, `internal/genjsonschema/genjsonschema.go:54-64` |
| Capabilities / allow_net | `santhosh-tekuri/jsonschema/v6 v6.0.2` dep; `capabilities.json` `allow_net` restricts remote `$ref` fetch | `go.mod:26`, `cmd/eval.go:299-309` |
| Builtin Tool Schemas | Rego builtin registry: `Builtin{ Name, Decl: types.NewFunction(Args, Result)}`, categories, `Nondeterministic`, `CanSkipBctx`; not portable LLM tool schemas | `v1/ast/builtins.go:11-24`, `v1/ast/builtins.go:42-327` |
| MCP Absence | No `internal/*mcp*`, `server/*mcp*`, or `mcp` Go package; only `memcpy` hits in WASM C | `internal/distributedtracing/distributedtracing.go:30-34` (imports show no mcp), `go.mod:1-131` (no mcp dep) |
| MCP External Only | Ecosystem entry: external stdio MCP server wrapping `opa fmt/check/eval/test/build` (50+ tools), hosted at `github.com/OrygnsCode/opa-mcp-server` | `docs/src/data/ecosystem/entries/opa-mcp.md:1-59` |
| OpenAPI Absence | Grep for `openapi`/`OpenAPI`/`swagger` returns zero code hits; only test fixture description strings `OpenAPIV3Schema` | `v1/ast/testdata/_definitions.json:17094` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**

- **OTLP/OpenTelemetry**: Full — traces (gRPC + HTTP) and metrics (gRPC + HTTP) with TLS `off/tls/mtls`, `allow_insecure_tls`, resource attributes (`service.version/instance_id/namespace`, `deployment.environment`), sampling (`sample_percentage` 0-100), and BatchSpanProcessor tuning (`internal/distributedtracing/distributedtracing.go:69-96`, `internal/metricsexport/metricsexport.go:29-39`). Dependencies pinned in `go.mod:37-48`.
- **JSON Schema**: Full — Draft 4/6/7 + Hybrid for validation/type checking (`internal/gojsonschema/draft.go:24-60`) and Draft 2020-12 generation via reflection (`internal/genjsonschema/genjsonschema.go:1-10`). Remote `$ref` fetching gated by `capabilities.json` `allow_net` (`v1/ast/compile.go:1631-1634`).
- **HTTP/REST**: Native — OPA HTTP API (`/v1/data`, `/v1/policies`, `/v1/query`, `/v1/compile`, `/v1/config`, health) served via `net/http` + `otelhttp` instrumentation (`v1/server/server.go:904-928`, `v1/server/server.go:954-965`). Bundle/OCI download (`download/oci_downloader.go:1-40`-ish) and WASM via `wazero` (`go.mod:33`).
- **Not supported natively**: MCP (external wrapper only — `docs/src/data/ecosystem/entries/opa-mcp.md:44-50`), OpenAPI import/generation, JSON-RPC, gRPC policy API (only OTLP gRPC as telemetry sink). No provider API abstraction (OPA is not an LLM harness).

**2. Is MCP supported?**

**No** in-tree. There is no MCP client or server implementation (`go.mod` has no `mcp-*` dependency; file glob shows no `mcp` package). The only MCP artifact is a third-party ecosystem listing: `docs/src/data/ecosystem/entries/opa-mcp.md:9-13` pointing to `github.com/OrygnsCode/opa-mcp-server` / `npm @orygn/opa-mcp` / `orygn/opa-mcp` Docker / Smithery. The description (`docs/src/data/ecosystem/entries/opa-mcp.md:45-49`) confirms it is a stdio MCP server that shells out to `opa fmt/check/eval/test/build`, the REST API, Regal, and Conftest — not a protocol adapter inside OPA. Adding external tools still requires writing a custom adapter/plugin or an external MCP server; there is no `RegisterMCPTool` or `ToolProvider` interface.

**3. Is OpenTelemetry supported?**

**Yes — mature.** Both traces and metrics export via OTLP:

- Trace exporters: `otlptracegrpc` + `otlptracehttp` initialized in `internal/distributedtracing/distributedtracing.go:128-139`, with TLS options `internal/distributedtracing/distributedtracing.go:297-309`, default endpoints `localhost:4317`/`4318` per OTLP spec (`internal/distributedtracing/distributedtracing.go:36-40`). Config lives under `distributed_tracing` (`type`, `address`, `service_name`, `sample_percentage`, `encryption`, `allow_insecure_tls`, `resource.*`, `batch_span_processor_options.*`).
- Metrics export: `metrics_export` Rego-validated config (`internal/metricsexport/validate.rego:1-12`) → `otlpmetricgrpc/http` + `metric.NewPeriodicReader` + `otelprometheus` bridge (`internal/metricsexport/metricsexport.go:108-142`).
- Propagation: `tracing.Options` DI (`v1/tracing/tracing.go:14-22`) registered by `v1/features/tracing/tracing.go:14-16` using `otelhttp.NewTransport/NewHandler`; server instruments every handler (`v1/server/server.go:959`) and annotates spans with `otelDecisionIDAttr = "opa.decision_id"` (`v1/server/server.go:99`, `v1/server/server.go:3214` via `trace.SpanFromContext`).
- Tests: `internal/distributedtracing/distributedtracing_test.go:10` and `v1/server/server_test.go:6182-6229` exercise `Init` validation.

**4. Are tool schemas portable across providers?**

**Not applicable — OPA has no provider-tool concept.** OPA's "tools" are Rego builtins registered statically in `v1/ast/builtins.go:45-327` with `types.NewFunction` / `BuiltinMap` (`v1/ast/builtins.go:22-24`). Capabilities expose them as versioned JSON (`capabilities.json` generated by `internal/cmd/genopacapabilities/main.go:26`, embedded via `capabilities/capabilities.go:11-16`) for compatibility checking, not for LLM provider interop. There is no adapter layer that maps a neutral tool schema to OpenAI Anthropic / Gemini / etc.; no `Tool`, `ToolCall`, `FunctionDefinition` abstraction; no JSON Schema → provider schema translator (the existing JSON Schema → `types.Type` translator at `v1/ast/compile.go:1704-1878` is for Rego type checking only). Portability across LLM providers would require an external harness.

## Architectural Decisions

- **DI for OTEL via blank import** — `tracing/tracing.go:7` re-exports `v1/tracing`, while `v1/features/tracing/tracing.go:14-16` registers the real `otelhttp` implementation in an `init()`. This decouples core `server`/`runtime`/`rego` from OTEL at compile time but preserves runtime injection via `RegisterHTTPTracing` (`v1/tracing/tracing.go:30-35`). Tradeoff: clean layering, but `convertOpts` (`v1/features/tracing/tracing.go:28-33`) uses unsafe `.(otelhttp.Option)` type assertions.
- **Vendored gojsonschema fork** — `internal/gojsonschema/README.md:5-12` duplicates `xeipuuv/gojsonschema` to export private `schema`/`subSchema` fields so `v1/ast/compile.go:1708-1727` can walk `RefSchema`/`PropertiesChildren`/`ItemsChildren` without patching upstream. This freezes behavior and incurs maintenance debt (lint/style fixes noted).
- **Reflection-based JSON Schema generation** — `internal/genjsonschema/genjsonschema.go:38-52` accumulates `OrderedMap` defs sorted byte-stable; `TypeResolver` hook (`internal/genjsonschema/genjsonschema.go:22-34`) lets plan/manifest generators intercept polymorphic types. `MakeNullable` (`internal/genjsonschema/genjsonschema.go:311-347`) handles nullable unions without mutating caller maps.
- **Config validation via Rego** — `internal/metricsexport/validate.rego` + `configpolicy.New` (`internal/metricsexport/metricsexport.go:44-48`) validates `metrics_export` declaratively, whereas `distributed_tracing` uses imperative `validateAndInjectDefaults` (`internal/distributedtracing/distributedtracing.go:225-294`). Two patterns coexist without unification.

## Notable Patterns

- **OTLP exporter abstraction per transport**: `otlptracegrpc.NewUnstarted(WithEndpoint, grpcTLSOption)` vs `otlptracehttp.NewUnstarted` (`internal/distributedtracing/distributedtracing.go:129-139`) mirrored for metrics (`internal/metricsexport/metricsexport.go:108-122`). Centralized `tlsutil.LoadCertificate/LoadCertPool/BuildTLSConfig` shared between both.
- **Batch processor fully configurable**: Every `trace.BatchSpanProcessorOption` exposed (`Blocking`, `BatchTimeoutMs`, `ExportTimeoutMs`, `MaxExportBatchSize`, `MaxQueueSize`) with OTel-spec defaults (`internal/distributedtracing/distributedtracing.go:47-54`).
- **Schema annotations drive type env**: `AnnotationSet` (`v1/ast/check.go:1450-1468`) feeds `processAnnotation` → `loadSchema` → `checker.CheckTypes`; `getSchemaType` caches per `Schema.String()` (`v1/ast/check.go:219-244`).
- **Recursive $ref cycle breaker**: `cachedDef{processing bool, rec *Recursive}` (`v1/ast/compile.go:1688-1696`) reserves `nil` entry then `SetType` on defer, returning `NewRecursive` for cycles (`v1/ast/compile.go:1717-1744`).

## Tradeoffs

- **OTEL maturity vs scope**: Trace + metrics OTLP are first-class with TLS, sampling, and resource semantics, but logs OTLP is absent; `sink.WithValues` is a no-op (`internal/distributedtracing/distributedtracing.go:349-351`) and verbosity is collapsed to a single level (`sink.Enabled` checks `GetLevel()`), so fine-grained OTEL log filtering is lost.
- **Vendored gojsonschema debt**: Fork gives control over internal structs but requires manual sync with upstream (Draft 2020-12 is only in `internal/genjsonschema`, not in `gojsonschema`; `Draft` enum stops at 7 — `internal/gojsonschema/draft.go:29-33`).
- **Remote schema security vs usability**: `AllowNet` host allowlist (`v1/ast/compile.go:1633`) defaults via `capabilities.json` — secure, but empty `allow_net` breaks any remote `$ref` and the error surfaces as `unable to compile the schema` (`v1/ast/compile.go:1641`). No retry/backoff observable.
- **Genjsonschema strictness**: `additionalProperties:false` by default (`internal/genjsonschema/genjsonschema.go:170-172`) yields tight schemas; `AllowAdditionalProperties` opt-out is per-type (`internal/genjsonschema/genjsonschema.go:59-64`) — easy to mis-scope and silently allow extra fields on nested defs (tested at `internal/genjsonschema/genjsonschema_test.go:429-442`).

## Failure Modes / Edge Cases

- **Unknown `distributed_tracing.type`** returns `fmt.Errorf("unknown distributed_tracing.type '%s', must be \"grpc\", \"http\" or \"\" (unset)")` (`internal/distributedtracing/distributedtracing.go:229`); nil raw config silently no-ops (`internal/distributedtracing/distributedtracing.go:109-111`), which can mask misconfiguration (typo in key → no tracing, no error).
- **Invalid sampling**: `sample_percentage` outside [0,100] → `unsupported distributed_tracing.sample_percentage` (`internal/distributedtracing/distributedtracing.go:290-292`); no validation that sampling + `BatchSpanProcessor` `MaxQueueSize`/`MaxExportBatchSize` are coherent (queue overflow → dropped spans, only warned via `errorHandler.Handle` → `logger.Warn` at `internal/distributedtracing/distributedtracing.go:315-317`).
- **TLS mis-wire**: `WithInsecure()` is used when `encryption=="off"` even if `tls_cert_file` was set; the mismatch is not warned (`internal/distributedtracing/distributedtracing.go:297-309`).
- **Schema recursion handled, divergence not**: `mergeSchemas` errors on `type mismatch: %v and %v` (`v1/ast/compile.go:1671`) and on `ItemsChildren` type mismatch (`v1/ast/compile.go:1679-1681`), but `Definitions` name collisions in `internal/genjsonschema` panic on cross-package misuse (`AddNamedDef: %q is already registered` at `internal/genjsonschema/genjsonschema.go:118`).
- **OpenTelemetry shutdown race**: `CHANGELOG.md:2701` notes prior fix for graceful shutdown (`#6651`); `otel.SetErrorHandler`/`SetLogger` are global singletons (`internal/distributedtracing/distributedtracing.go:195-196`), so concurrent `Init` calls can race logger installs.
- **MCP / OpenAPI gap**: Any attempt to expose OPA decisions as MCP tools/resources must build a translation layer; there is no in-repo schema to generate MCP manifests or OpenAPI specs from Rego, so drift between policy surface and external spec is manual.

## Future Considerations

- **Native MCP server** — Promote `opa-mcp-server` (currently external — `docs/src/data/ecosystem/entries/opa-mcp.md:10`) to a first-class distribution artifact or port a minimal stdio MCP transport into `cmd/` that exposes `data`/`query`/`compile`/`test`/`fmt` as MCP tools with `genjsonschema`-generated input schemas. Use `internal/genjsonschema.Builder` as the MCP schema generator to avoid duplication.
- **OpenAPI generation** — Add an `openapi` exporter that traverses `ast.Compiler` rule heads annotated with `openapi:` and reuses `internal/genjsonschema` to emit OpenAPI 3.x `components/schemas`. This would mirror how `compileSchema` consumes JSON Schema and close the policy→API contract loop.
- **Unified config validation** — Align `distributed_tracing` with `metrics_export`'s Rego-based validation (`internal/metricsexport/validate.rego`) to get consistent defaults injection and host/TLS validation.
- **Provider-agnostic tool schema** — If OPA ever fronts LLM agents (e.g., policy-aware tool filter), define a neutral `ToolDef { name string, description string, jsonSchema OrderedMap }` and a translator to OpenAI/Gemini/Anthropic function schemas, reusing `types.Function` metadata from `v1/ast/builtins.go`.

## Questions / Gaps

- **No evidence found: MCP wire semantics** — Searched `go.mod:1-131`, `v1/ast/builtins.go`, `v1/server/server.go:1-130`, `internal/distributedtracing/*`; no `tools/list`, `resources`, or `prompts` handlers. Confirmed external-only via `docs/src/data/ecosystem/entries/opa-mcp.md:44-59`. What is the desired scope if MCP were added (policy eval only vs full CLI)?
- **No evidence found: OTLP logs & exemplars** — Only traces + metrics present; logs bridge in `internal/distributedtracing/distributedtracing.go:319-351` forwards OTEL logs to OPA logger, not OPA logs to OTLP. Search for `otlpmetric`/`otlptrace` found no `otlplog`.
- **No evidence found: OpenAPI ↔ Rego round-trip** — `v1/ast/testdata/_definitions.json:16736-17094` mentions OpenAPI types only in K8s CRD descriptions; no importer that turns an OpenAPI spec into `ast.Module` or Rego types beyond Kubernetes JSON Schema.
- **Partial evidence on schema portability**: `internal/genjsonschema` produces Draft 2020-12, but `internal/gojsonschema` only validates Draft 4/6/7 (`internal/gojsonschema/draft.go:29-33`). A 2020-12 schema generated by OPA cannot be consumed by OPA's own validator without translation — gap worth closing.

---

Generated by `19.01-protocol-compatibility` against `opa`.
