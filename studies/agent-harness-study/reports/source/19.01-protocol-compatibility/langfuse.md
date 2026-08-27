# Source Analysis: langfuse

## 19.01 Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Next.js (web), Express+BullMQ (worker), Prisma + ClickHouse, pnpm+turbo |
| Analyzed | 2026-08-26 |

## Summary

Langfuse implements **server-side MCP**, **inbound OTLP trace ingestion** plus **outbound OTLD trace export**, **Fern-driven OpenAPI export**, and **Zod-native JSON Schema draft-7** for tool definitions in a single observability platform. It does **not** implement an MCP client, MCP resources/prompts, or OpenAPI import for tool federation — external tools require a custom `defineTool` adapter registered in the global `ToolRegistry`. Provider APIs are abstracted via Vercel AI SDK (`@ai-sdk/*`) giving model-independent execution, but tool schemas are coupled to the internal Zod→JSON-Schema pipeline rather than a portable cross-provider standard. The overall posture is **server as protocol surface** (consume OTLP, expose OpenAPI/MCP, emit OTel) rather than **agent harness** that orchestrates heterogeneous tools.

## Rating

**Score: 8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards**

Rationale: MCP server is production-grade (stateless Streamable HTTP 2025-03-26 spec, `@modelcontextprotocol/sdk@1.29.0`, 15 feature modules, formal registry with conflict detection, security hardening for Host/Origin, RBAC/entitlement gating, 13+ server tests). OTLP ingestion is spec-compliant (protobuf JSON+gzip, Protobuf decode via compiled proto, exhaustive ID/collection validation, 16 MB warn, S3→BullMQ async pipeline). OpenAPI is authoritative via Fern (`fern/apis/server|client|organizations`, `pnpm openapi:export` in `package.json:41`) with public `web/public/generated/api/openapi.yml:1` and deprecation-sync tooling + tests. Outbound OpenTelemetry tracing is fully instrumented in both web and worker via `OTLPTraceExporter` to `OTEL_EXPORTER_OTLP_ENDPOINT`. Deduction from 9-10 for missing MCP client/resources/prompts, no OpenAPI→tool importer, and `0.3.0-unstable` MCP version signaling incomplete stability/observability under scale.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP Server Implementation | `createMcpServer` creates fresh `Server` per request, stateless pattern, `ListTools`/`CallTool` handlers delegating to `toolRegistry`, instructions + version `0.3.0-unstable` | `web/src/features/mcp/server/mcpServer.ts:15-25`, `web/src/features/mcp/server/mcpServer.ts:57-82`, `web/src/features/mcp/server/mcpServer.ts:85-123` |
| MCP Transport | Streamable HTTP transport (`StreamableHTTPServerTransport`) stateless mode, JSON response, Accept header validation, connection lifecycle handling | `web/src/features/mcp/server/transport.ts:12-13`, `web/src/features/mcp/server/transport.ts:36-95` |
| MCP Endpoint & Auth | Next.js API route at `/api/public/mcp` with BasicAuth project-scoped check, rate limit, CORS, in-app-agent tool allowlist header, auto-bootstrap import | `web/src/pages/api/public/mcp/index.ts:27-53`, `web/src/pages/api/public/mcp/index.ts:86-147`, `web/src/pages/api/public/mcp/index.ts:193-217` |
| MCP Security / DNS Rebinding | `validateMcpRequestSecurity` enforces Host/Origin allowlist via `LANGFUSE_MCP_ALLOWED_HOSTS`, localhost exceptions in dev, wildcard `*` disables check | `web/src/features/mcp/server/security.ts:41-68`, `web/src/features/mcp/server/security.ts:70-116` |
| MCP Tool Registry | Global singleton `ToolRegistry` with feature modules, conflict detection, `getToolDefinitions`/`getEnabledTool` with `isEnabled` and permission gating (`read` vs `tool-allowlist`) | `web/src/features/mcp/server/registry.ts:72-109`, `web/src/features/mcp/server/registry.ts:117-189` |
| MCP Bootstrap | 15 features registered at import time (`prompts`, `observations`, `datasets`, `scores`, `evals`, etc.), type-level `McpToolName` derivation | `web/src/features/mcp/server/bootstrap.ts:14-50`, `web/src/features/mcp/server/bootstrap.ts:57-70` |
| MCP JSON Schema Generation | `defineTool` converts Zod v4 `baseSchema` → JSON Schema draft-7 via `z.toJSONSchema({target:"draft-7"})`, rejects unions/intersections, validates object type, wraps handler with `inputSchema.parse` | `web/src/features/mcp/core/define-tool.ts:126-151`, `web/src/features/mcp/core/define-tool.ts:170-178` |
| MCP Tool Annotations | `readOnlyHint`/`destructiveHint`/`expensiveHint` annotations propagated to `ToolDefinition`; used for RBAC allowlist decisions | `web/src/features/mcp/core/define-tool.ts:39-47`, `web/src/features/mcp/core/define-tool.ts:160-167`, `web/src/features/mcp/server/registry.ts:175-186` |
| MCP Types / Context | `ServerContext` stateless context (projectId, orgId, apiKeyId, plan, inAppAgent) captured in closures | `web/src/features/mcp/types.ts:30-66` |
| MCP Policy (In-App Agent) | Exhaustive per-tool approval/availability policy (`auto` vs `approval` + RBAC scope) for 80+ tools, prefixed names `langfuse_<tool>` | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`, `packages/shared/src/in-app-agent/server/mcpPolicy.ts:477-510` |
| MCP Tests | Dedicated test suites: define-tool (union/intersection rejection, object-type preservation), auth, read/write tools, security, route in-app-agent context, helpers | `web/src/__tests__/server/unit/mcp-define-tool.servertest.ts:6-107`, `web/src/__tests__/server/mcp-auth.servertest.ts:1`, `web/src/__tests__/server/mcp-tools-read.servertest.ts:1`, `web/src/__tests__/server/mcp-tools-write.servertest.ts:1`, `web/src/__tests__/server/unit/mcp-security.servertest.ts:1` |
| MCP Well-Known Discovery | `.well-known/mcp.json` advertises MCP endpoint URL | `web/src/pages/api/well-known/mcp.json.ts:15-24` |
| MCP SDK Dependency | `@modelcontextprotocol/sdk@1.29.0` with `@cfworker/json-schema@4.1.1` for validation | `web/package.json:59`, `pnpm-lock.yaml:12923`, `web/src/features/mcp/server/mcpServer.ts:15` |
| OTLP Ingestion Endpoint (Fern) | Fern definition declares `POST /api/public/otel/v1/traces` with OTLP/HTTP spec compliance (binary protobuf, JSON protobuf, gzip), `ExportTraceServiceRequest` | `fern/apis/server/definition/opentelemetry.yml:6-34`, `fern/apis/server/definition/opentelemetry.yml:67-167` |
| OTLP Handler (Traces) | Disables bodyParser, handles raw Buffer, gunzip, content-type branching (`application/json` vs `application/x-protobuf`), protobuf decode via `$root.opentelemetry.proto.*.ExportTraceServiceRequest`, validation via `validateOtelSpanIds`, publishes to `OtelIngestionProcessor` | `web/src/pages/api/public/otel/v1/traces/index.ts:20-115`, `web/src/pages/api/public/otel/v1/traces/index.ts:132-312` |
| OTLP Metrics Stub | `POST /api/public/otel/v1/metrics` route exists but no implementation (empty fn) | `web/src/pages/api/public/otel/v1/metrics/index.ts:11-18` |
| OTLP Validation Logic | `validateOtelSpanIds` walks `resourceSpans→scopeSpans→spans`, checks `traceId/spanId/parentSpanId` via `getOtelIdRejectionReason` (string/Uint8Array/Buffer), flags non-array `attributes/events/scopeSpans`, bounded tag set | `packages/shared/src/server/otel/utils.ts:12-42`, `packages/shared/src/server/otel/utils.ts:108-229` |
| OTLP Processor | `OtelIngestionProcessor` converts OTel spans to Langfuse observations/traces, handles level aliases, timestamps, metadata-drop caps, S3 queue publish, conversion failure metrics | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:42-65`, `packages/shared/src/server/otel/OtelIngestionProcessor.ts:72-94`, `packages/shared/src/server/otel/OtelIngestionProcessor.ts:241-274` |
| OTLP Tests | Validation unit tests (`getOtelIdRejectionReason`, `validateOtelSpanIds` with malformed collections), mapping servertest, API servertest | `packages/shared/src/server/otel/utils.test.ts:3-68`, `web/src/__tests__/server/otel-api.servertest.ts:1`, `web/src/__tests__/server/api/otel/otelMapping.servertest.ts:1` |
| OTLP Proto Compilation | Pre-compiled protobuf from `v1.5.0` spec copied from `@opentelemetry/otlp-transformer`, Next.js-compatible TS wrapper | `web/src/pages/api/public/otel/otlp-proto/README.md:1-8`, `web/src/pages/api/public/otel/otlp-proto/generated/root.ts:1` |
| Outbound OTLP Exporter (Web) | `OTLPTraceExporter` with `url: ${OTEL_EXPORTER_OTLP_ENDPOINT}/v1/traces`, instrumentations: IORedis, Http (with SDK name header), Prisma, Winston, AWS, BullMQ; Sampler via `OTEL_TRACE_SAMPLING_RATIO` | `web/src/observability.config.ts:15-116`, `web/src/observability.config.ts:117-165` |
| Outbound OTLP Exporter (Worker) | Parallel `OTLPTraceExporter` with Undici/Express/Prisma/etc., shared `ioredisRequestHook`, resource detectors | `worker/src/instrumentation.ts:2-38`, `worker/src/instrumentation.ts:40-99` |
| OpenTelemetry Env | `OTEL_EXPORTER_OTLP_ENDPOINT` default `http://localhost:4318` in prod example, web env, worker env, shared not needed | `.env.prod.example:44`, `web/src/env.mjs:305`, `worker/src/env.ts:204` |
| OpenAPI Generation (Fern) | Three Fern APIs: `server`, `client`, `organizations`; `fern.config.json` org `langfuse@3.88.0`; generators for Python + TS Node SDKs | `fern/fern.config.json:1-4`, `fern/apis/server/definition/api.yml:12`, `fern/apis/server/generators.yml:14-41` |
| OpenAPI Export Script | `pnpm openapi:export` runs `fern-api export` for all three APIs then `sync-deprecations.ts` to stamp `deprecated` flags | `package.json:41`, `web/scripts/openapi/sync-deprecations.ts:14-24`, `web/scripts/openapi/stamp-deprecations.ts:57-80` |
| OpenAPI Spec Output | Generated specs at `web/public/generated/api/openapi.yml`, `api-client/openapi.yml`, `organizations-api/openapi.yml`, served via `next.config.mjs` public dir | `web/public/generated/api/openapi.yml:1-22`, `web/public/generated/api-client/openapi.yml:1`, `web/public/generated/organizations-api/openapi.yml:1-28`, `web/next.config.mjs:234` |
| OpenAPI Deprecation Tests | Unit test pins deprecation drift between Fern definitions and generated OpenAPI | `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:9-40`, `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:95` |
| JSON Schema Generators | Zod v4 `z.toJSONSchema` for MCP; dependencies `zod-to-json-schema@3.25.2`, `@types/json-schema@7.0.15`, `json-schema-faker@0.6.1` (web), `@cfworker/json-schema@4.1.1` (MCP SDK) | `web/src/features/mcp/core/define-tool.ts:127-130`, `web/package.json:171`, `packages/shared/package.json:201` |
| Provider APIs (Model-Independent) | AI SDK provider factories: `@ai-sdk/openai`, `anthropic`, `azure`, `google`, `google-vertex`, `amazon-bedrock`, `openai-compatible`; unified `buildAiSdkModel`/`llmText` layer | `packages/shared/package.json:145-153`, `packages/shared/src/server/llm/ai-sdk/providers/openai.ts:1-2`, `packages/shared/src/server/llm/ai-sdk/providers/anthropic.ts:1`, `packages/shared/src/server/llm/ai-sdk/providers/azure.ts:1`, `packages/shared/src/server/llm/ai-sdk/providers/google.ts:1`, `packages/shared/src/server/llm/ai-sdk/providers/bedrock.ts:1`, `packages/shared/src/server/llm/ai-sdk/providers/vertex.ts:1-2` |
| Instrumentation Helpers | `instrumentAsync`/`instrumentSync` with baggage propagation, span attributes, `addUserToSpan`, CloudWatch+Datadog metrics | `packages/shared/src/server/instrumentation/index.ts:27-72`, `packages/shared/src/server/instrumentation/index.ts:181-244` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**

| Protocol | Role | Evidence |
|----------|------|----------|
| **MCP 2025-03-26** | Server only, Streamable HTTP, tools-only | `web/src/features/mcp/server/mcpServer.ts:15`, `web/src/features/mcp/server/transport.ts:12`, `web/src/pages/api/public/mcp/index.ts:2` |
| **OTLP/HTTP (OpenTelemetry)** | Inbound traces ingestion + outbound trace export | `fern/apis/server/definition/opentelemetry.yml:6`, `web/src/pages/api/public/otel/v1/traces/index.ts:27`, `web/src/observability.config.ts:15`, `worker/src/instrumentation.ts:37` |
| **OpenAPI 3.0.1** | Export (spec generation), not import | `web/public/generated/api/openapi.yml:1`, `fern/fern.config.json:1`, `package.json:41` |
| **JSON Schema draft-7** | Tool input schema generation via Zod v4 | `web/src/features/mcp/core/define-tool.ts:127` |
| **SCIM 2.0** | Public API for user provisioning (organization scope) | `web/src/pages/api/public/scim/Users/index.ts:1`, `fern/apis/server/definition/scim.yml:1` |
| **HTTP semantics** | Batch ingestion (`POST /api/public/ingestion`), REST CRUD for traces/observations/datasets | `web/src/pages/api/public/ingestion.ts:1`, `fern/apis/server/definition/api.yml:1` |

Partial/missing: MCP client, MCP resources/prompts, OpenAPI import → tool generation, JSON Schema validation for ingestion payloads (uses Zod directly, not JSON Schema), gRPC OTLP.

**2. Is MCP supported?**

**Yes — comprehensive server implementation, no client.**

*Server*: `@modelcontextprotocol/sdk@1.29.0` (`web/package.json:59`), stateless per-request server (`web/src/features/mcp/server/mcpServer.ts:57`), Streamable HTTP (`web/src/features/mcp/server/transport.ts:66`), 15 feature modules covering prompts/observations/datasets/scores/evals/traces etc. (`web/src/features/mcp/server/bootstrap.ts:30`), dynamic discovery via `ListTools` (`web/src/features/mcp/server/mcpServer.ts:72`).

*Primitives*: Tools only. No `resources` or `prompts` handlers — `capabilities: { tools: {} }` (`web/src/features/mcp/server/mcpServer.ts:64`). The commented-out code in `web/src/pages/api/public/mcp/index.ts:22-24` indicates resources were planned but not shipped.

*Tool definition*: `defineTool` (`web/src/features/mcp/core/define-tool.ts:112`) with `z.toJSONSchema(draft-7)` + `annotations` (readOnly/destructive). Strict guard: unions/intersections throw (`web/src/features/mcp/core/define-tool.ts:138`), enforced by tests (`web/src/__tests__/server/unit/mcp-define-tool.servertest.ts:6-107`).

*Security*: Host/Origin validation (`web/src/features/mcp/server/security.ts:70`), CORS (`web/src/features/mcp/server/security.ts:118`), project-scoped BasicAuth only (`web/src/pages/api/public/mcp/index.ts:98-106`), rate limiting (`web/src/pages/api/public/mcp/index.ts:123`).

*In-app agent extension*: MCP tool policy with `auto`/`approval` gating and RBAC scope mapping (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27`), allowlist header `IN_APP_AGENT_MCP_TOOL_OVERRIDE_HEADER` (`web/src/pages/api/public/mcp/index.ts:48`).

*Maturity caveat*: Version `0.3.0-unstable` (`web/src/features/mcp/server/mcpServer.ts:30`) and self-describing contract warning in `web/src/features/mcp/README.md:8`.

**3. Is OpenTelemetry supported?**

**Yes — dual role (inbound OTLP ingestion + outbound trace emission).**

*Inbound (OTLP ingestion)*: Endpoint `POST /api/public/otel/v1/traces` (`web/src/pages/api/public/otel/v1/traces/index.ts:32`) accepts `application/json` and `application/x-protobuf` (gzip supported), decodes via `ExportTraceServiceRequest` protobuf (`web/src/pages/api/public/otel/v1/traces/index.ts:86`), validates with `validateOtelSpanIds` (`web/src/pages/api/public/otel/v1/traces/index.ts:132`), then enqueues to `OtelIngestionProcessor.publishToOtelIngestionQueue` → S3 → `OtelIngestionQueue` (BullMQ) → worker conversion to Langfuse domain (`packages/shared/src/server/otel/OtelIngestionProcessor.ts:241`). Fern docs explicitly cite spec compliance (`fern/apis/server/definition/opentelemetry.yml:18-20`). Metrics endpoint (`web/src/pages/api/public/otel/v1/metrics/index.ts:12`) is a stub — returns empty.

*Outbound (trace export)*: Both runtimes emit traces via `OTLPTraceExporter` to `OTEL_EXPORTER_OTLP_ENDPOINT` (`web/src/observability.config.ts:115`, `worker/src/instrumentation.ts:37`), default `http://localhost:4318` (`web/src/env.mjs:305`). Instrumentation covers HTTP, IORedis, Prisma, AWS, BullMQ, Winston, Undici/Express. Sampling via `OTEL_TRACE_SAMPLING_RATIO` (`web/src/observability.config.ts:164`). Instrumentation helpers add baggage propagation (`packages/shared/src/server/instrumentation/index.ts:27`), ClickHouse surface/route tagging via `contextWithLangfuseProps` (`web/src/features/mcp/server/mcpServer.ts:103`).

*Not supported*: OTLP gRPC, logs export (explicitly rejected with 400 if logs payload sent to traces endpoint — `web/src/pages/api/public/otel/v1/traces/index.ts:122-235`).

**4. Are tool schemas portable across providers?**

**Partially — MCP tool schemas are provider-independent (JSON Schema draft-7) but Langfuse-native; LLM model invocation is provider-independent via AI SDK.**

*MCP side*: `defineTool` decouples `baseSchema` (JSON Schema for wire) from `inputSchema` (Zod with refinements for runtime) (`web/src/features/mcp/core/define-tool.ts:31-34`). JSON Schema is emitted via standard `z.toJSONSchema({target:"draft-7"})` (`web/src/features/mcp/core/define-tool.ts:127`), so any JSON-Schema-compatible client (Claude Code, Cursor) can consume tools without custom adapters — provided the client speaks MCP. However, adding external non-MCP tools requires writing a `defineTool` adapter and registering via `toolRegistry.register` (`web/src/features/mcp/server/registry.ts:82`), i.e., **no zero-code OpenAPI→tool import** exists; Fern is export-only. Union/intersection schemas are banned (`web/src/features/mcp/core/define-tool.ts:138`), limiting expressiveness for polymorphic inputs.

*LLM provider side*: `packages/shared/src/server/llm/ai-sdk/providers/` abstracts OpenAI/Anthropic/Azure/Google/Vertex/Bedrock/OpenAI-Compatible behind `buildAiSdkModel` (`packages/shared/src/server/llm/ai-sdk/providers/openai.ts:1`), so prompt/eval execution is model-independent. This does not make MCP tool schemas portable across LLM providers — tool calling still relies on the caller LLM's tool-calling capability via the MCP client, not on Langfuse translating between provider tool formats.

*Verdict*: **Portable within MCP ecosystem, not across arbitrary provider tool formats without the defineTool adapter layer.** Answer to "Can external tools be added without writing custom adapters?" is **No** — a thin adapter (Zod schema + handler closure) is required per tool; there is no generic OpenAPI importer or MCP client that auto-discovers remote tools.

## Architectural Decisions

| Decision | Location | Rationale / Tradeoff |
|----------|----------|----------------------|
| Stateless per-request MCP server (fresh `Server` + closures, no sessions) | `web/src/features/mcp/server/mcpServer.ts:9-13`, `web/src/features/mcp/server/mcpServer.ts:57`, `web/src/pages/api/public/mcp/index.ts:55-66` | Eliminates session leaks between projects, simplifies auth; loses streaming/subscriptions and stateful prompts/resources; forces `enableJsonResponse:true` over SSE |
| Streamable HTTP over legacy SSE | `web/src/features/mcp/server/transport.ts:5`, `web/src/pages/api/public/mcp/index.ts:13` | Follows MCP 2025-03-26 spec; stateless mode (`sessionIdGenerator: undefined` in `web/src/features/mcp/server/transport.ts:67`) avoids coordination |
| Global `ToolRegistry` singleton with feature modules | `web/src/features/mcp/server/registry.ts:72`, `web/src/features/mcp/server/bootstrap.ts:30` | Dynamic discovery, conflict detection, feature gating; but global mutable singleton complicates testing/isolation |
| `z.toJSONSchema(draft-7)` for MCP, Zod for runtime validation (dual schema) | `web/src/features/mcp/core/define-tool.ts:31-34`, `web/src/features/mcp/core/define-tool.ts:127` | Single source of truth (Zod) with spec-compliant wire format; but union/intersection ban forces schema simplification |
| Fern as single source for OpenAPI → SDKs + spec | `fern/fern.config.json:1`, `fern/apis/server/generators.yml:14`, `package.json:41` | Guarantees spec/SDK drift prevention; but adds Fern toolchain dependency and no import path |
| Raw body handling for OTLP (`bodyParser: false`) | `web/src/pages/api/public/otel/v1/traces/index.ts:20-24` | Supports both protobuf and JSON plus gzip; manual parsing is error-prone and duplicates content-type logic already in middleware |
| S3 staging for OTLP batches (`OtelIngestionQueue`) | `packages/shared/src/server/otel/OtelIngestionProcessor.ts:72`, `web/src/pages/api/public/otel/v1/traces/index.ts:310` | Decouples ingestion from processing, enables replay (`worker/src/scripts/replayIngestionEventsV2/README.md:135`); adds S3 latency and operational surface |
| Dual observability (OTLP + Datadog/CloudWatch) | `web/src/observability.config.ts:104`, `packages/shared/src/server/instrumentation/index.ts:292-351` | Vendor neutrality for traces, dogstatsd for metrics; CloudWatch flattening keyed on suffix (`packages/shared/src/server/instrumentation/index.ts:294`) is subtle and tested minimally |
| AI SDK provider abstraction for LLM calls | `packages/shared/package.json:145-153`, `packages/shared/src/server/llm/ai-sdk/providers/` | Model independence without re-implementing provider clients; ties to Vercel AI SDK version churn |

## Notable Patterns

- **Feature-sliced MCP domains**: 15 self-contained `web/src/features/mcp/features/<domain>/tools/` modules each export an `McpFeatureModule` with `isEnabled` hook — enables entitlement-gated tools without central switch statement (`web/src/features/mcp/server/bootstrap.ts:30`, `web/src/features/mcp/server/registry.ts:120`).
- **Annotation-driven RBAC at transport edge**: `readOnlyHint` + `tool-allowlist` permission model (`web/src/features/mcp/server/registry.ts:175`, `packages/shared/src/in-app-agent/server/mcpPolicy.ts:477`) lets in-app-agent enforce least privilege without per-tool middleware.
- **Dual-schema validation**: `baseSchema` drives JSON Schema generation for discovery, `inputSchema` (with refinements) drives runtime `parse` inside `wrapErrorHandling` (`web/src/features/mcp/core/define-tool.ts:170`) — clean separation prevents MCP clients seeing refinement artifacts.
- **Exhaustive policy table**: `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` as `Record<string, Policy>` with `satisfies` + type-level exhaustiveness check in servertest ensures new tools are classified before shipping (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27`, `packages/shared/src/in-app-agent/server/tools.test.ts:3`).
- **Compiled protobuf vendoring**: `web/src/pages/api/public/otel/otlp-proto/generated/root.ts` vendored from `@opentelemetry/otlp-transformer` avoids runtime codegen and Next.js bundling issues (`web/src/pages/api/public/otel/otlp-proto/README.md:3`).
- **Defensive OTLP rejection with observability**: `validateOtelSpanIds` returns `reasonCounts` used as metric tags (`web/src/pages/api/public/otel/v1/traces/index.ts:140-148`), and logs `scopeNames` + `sdkName` for debuggability (`web/src/pages/api/public/otel/v1/traces/index.ts:149-184`).

## Tradeoffs

- **Server-only MCP**: Easy to be a tool provider for Claude/Cursor, but cannot itself orchestrate external MCP tools — Langfuse cannot act as an agent harness without adding a client transport (would require session management, re-auth).
- **Tools-only capability**: Simpler to implement/test securely; but MCP resources/prompts (useful for prompt browsing without tool calls) are unavailable — prompt access still requires `getPrompt` tool call round-trips (`web/src/features/mcp/features/prompts/tools/getPrompt.ts:1`).
- **No OpenAPI import**: Fern export guarantees Langfuse API fidelity but prevents generic “import your REST API as tools” workflows; every external integration needs bespoke `defineTool` code.
- **Stateless transport**: No `Mcp-Session-Id` state simplifies horizontal scaling; but `GET` SSE stream and progress notifications are effectively unused (stateless `enableJsonResponse:true` in `web/src/features/mcp/server/transport.ts:68`).
- **Zod draft-7 ban on unions**: Keeps schemas MCP-compliant at definition time (`web/src/features/mcp/core/define-tool.ts:138`), but forces callers to split polymorphic operations into multiple tools (e.g., separate `createTextPrompt`/`createChatPrompt` instead of discriminated union).
- **Raw OTLP body handling bypasses `withMiddlewares`**: Gives direct response control needed for OTLP but diverges from standard public API auth/error patterns (`web/src/pages/api/public/otel/v1/traces/index.ts:26` vs `web/src/pages/api/public/mcp/index.ts:18` comment).
- **S3-backed ingestion queue**: Durable, replayable ( `worker/src/scripts/replayIngestionEventsV2/README.md:143`), but adds operational cost (S3 lifecycle, `otel/<projectId>/yyyy/mm/dd` keys) vs direct queue enqueue.

## Failure Modes / Edge Cases

| Failure Mode | Handling | Evidence |
|--------------|----------|----------|
| Union/intersection Zod schema passed to `defineTool` | Throws at startup with explicit message; caught by unit tests | `web/src/features/mcp/core/define-tool.ts:138-142`, `web/src/__tests__/server/unit/mcp-define-tool.servertest.ts:6-23` |
| Duplicate tool name across features | `ToolRegistry.register` throws with feature attribution | `web/src/features/mcp/server/registry.ts:88-95` |
| Invalid Host/Origin (DNS rebinding) | `ForbiddenError` unless `LANGFUSE_MCP_ALLOWED_HOSTS=*` | `web/src/features/mcp/server/security.ts:96-112`, `web/src/__tests__/server/unit/mcp-security.servertest.ts:1` |
| Missing/invalid Accept header for MCP POST | 406 JSON-RPC error | `web/src/features/mcp/server/transport.ts:48-62` |
| OTLP logs payload sent to traces endpoint (shared protobuf field numbers) | `validateOtelSpanIds` flags `traceId:absent` uniformly, batch rejected 400 with hint to point logs elsewhere | `web/src/pages/api/public/otel/v1/traces/index.ts:122-235`, `packages/shared/src/server/otel/utils.ts:88-96` |
| Malformed non-array collections (hand-rolled JSON exporter with object attributes) | Counts as `malformedCollectionCount`, 400 with `not_an_array` hint | `packages/shared/src/server/otel/utils.ts:49-51`, `web/src/pages/api/public/otel/v1/traces/index.ts:193-224` |
| OTLP body >16 MB | Warn log with `bodyBytes` + `spanCount`, still processed (no reject) | `web/src/pages/api/public/otel/v1/traces/index.ts:239-252` |
| Pathological `Buffer.from` input (`{length: 8e8}`) | Explicitly rejected via `getOtelIdRejectionReason` string/Uint8Array/Buffer check to avoid event-loop block | `packages/shared/src/server/otel/utils.ts:22-35` |
| Ingestion suspended (quota) for MCP/OTLP | `ForbiddenError` “Usage threshold exceeded” | `web/src/pages/api/public/mcp/index.ts:117-121`, `web/src/pages/api/public/otel/v1/traces/index.ts:34-38` |
| MCP transport error after headers sent | Guard `!res.headersSent` before fallback 500 JSON-RPC error | `web/src/features/mcp/server/transport.ts:104`, `web/src/pages/api/public/mcp/index.ts:172` |
| Rolling deploy: old pods read singular `toolName` vs new `toolNames` | `createInAppAgentMcpRunOverride` writes both fields; `InAppAgentMcpRunOverrideSchema` via `z.union` handles both | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:386-404` |
| Metrics endpoint unimplemented | Silent `fn: async () => {}` — no error, no ingestion, client gets 200 empty | `web/src/pages/api/public/otel/v1/metrics/index.ts:12-18` |

## Future Considerations

- **Add MCP client capability** (`StreamableHTTPClientTransport`) and a registry for remote MCP tool proxying if Langfuse intends to be an agent harness rather than only a tool provider; will require session handling and credential passthrough currently absent.
- **Implement MCP resources + prompts**: prompt browsing via `resources/list` and `prompts/list` would reduce tool-call indirection for clients that prefer resource subscription.
- **OpenAPI→MCP importer**: A generic importer that converts Fern-defined endpoints or user-supplied OpenAPI specs into `defineTool` registrations would answer “Can external tools be added without custom adapters?” affirmatively; currently infeasible.
- **Stabilize MCP version** beyond `0.3.0-unstable` with semver guarantees, changelog, and deprecation cadence to meet higher durability (9-10) bar.
- **Complete OTLP metrics/logs pipeline**: Either implement `/otel/v1/metrics` ingestion or return `405/501` with guidance instead of silent empty 200 (`web/src/pages/api/public/otel/v1/metrics/index.ts:16`).
- **Expose `LANGFUSE_MCP_ALLOWED_HOSTS` docs for self-hosted**: DNS rebinding protection is correct but failure (host mismatch) surfaces as opaque `403 Invalid Host header` — add troubleshooting link in error response.
- **Metrics tag cardinality audit**: `reasonCounts` bounded today (`packages/shared/src/server/otel/utils.ts:70`), but future attribute keys could explode `scopeNames` sampled set; cap already at `maxSamples=5` (`packages/shared/src/server/otel/utils.ts:110`) — keep enforced.

## Questions / Gaps

- **MCP conformance test suite**: No evidence of `modelcontextprotocol` SDK's compliance harness running in CI; search for `mcp.*test` found only project-owned servertests, not SDK conformance runs. Search boundary: `web/src/__tests__/server/mcp*`, `grep MCP` across repo.
- **MCP pagination / list truncation**: `toolRegistry.getToolDefinitions` returns all enabled tools without pagination (`web/src/features/mcp/server/registry.ts:117`); as tool count grows (>50 tools already), MCP `ListTools` pagination support is not implemented.
- **OTLP authorization granularity**: OTLP endpoint uses `createAuthedProjectAPIRoute` (same as public API) — whether it enforces separate `ingestion` rate-limit bucket vs `public-api` for MCP is not verified beyond `rateLimitResource` param inspection.
- **JSON Schema round-trip fidelity**: No test verifies `z.toJSONSchema` output validates against `@cfworker/json-schema` or that required/nullable semantics survive draft-7 conversion; `mcp-define-tool.servertest.ts` checks only type/pattern presence.
- **Provider API contract drift**: Fern SDKs are generated from Fern definitions, but MCP tool schemas are generated from Zod — no cross-check ensures MCP tool input types stay aligned with Fern `types/*` when shared domain contracts change (`packages/shared/src/server/queues.ts` vs Fern sources).
- **OTLP exporter authentication**: `OTLPTraceExporter` config uses bare URL with no auth header (`web/src/observability.config.ts:115`); whether authenticated OTEL backends (Honeycomb, Grafana Cloud requiring headers) are supported is not documented in code.
- **MCP resources deprecation comment**: `web/src/pages/api/public/mcp/index.ts:23` says “Resources: Added in LF-1928” — whether resources were removed or never shipped is not clear from git history without log inspection.

---

Generated by `19.01-protocol-compatibility` against `langfuse`.
