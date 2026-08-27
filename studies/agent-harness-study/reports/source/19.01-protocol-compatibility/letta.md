# Source Analysis: letta

## 19.01 Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.11-3.13 / FastAPI, Pydantic v2, SQLAlchemy, OpenTelemetry SDK, MCP SDK (mcp>=1.9.4, fastmcp>=2.12.5) |
| Analyzed | 2026-08-26 |

## Summary

Letta is a stateful-agent platform with mature integration for **open tool/observability protocols**: MCP (full client + server CRUD), OpenTelemetry OTLP (gRPC trace export + instrumentation), JSON Schema (canonical tool schema generation, strict-mode validation, normalization), and OpenAPI 3.1 (FastAPI-generated spec). MCP support is the most elaborate capability — three transports (STDIO/SSE/Streamable HTTP), per-server auth templating, encrypted secret storage, OAuth 2.0 flows, and bidirectional schema healing. OTLP is production-grade. JSON Schema tool definitions are provider-portable at the canonical layer but require per-provider adapters that are unevenly hardened. No OpenAPI-to-tool importer or generic protocol-adapter framework exists; external tools require custom adapters or manual MCP server registration.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards for MCP and OTLP; JSON Schema layer is well-tested but provider translation is fragmented; OpenAPI is export-only. Durability and observability gaps (no universal adapter registry, no OpenAPI import, stdio disabled by default for multi-tenant safety) prevent a 9-10.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP dependencies | `mcp[cli]>=1.9.4` and `fastmcp>=2.12.5` pinned in project dependencies | `letta/pyproject.toml:57,76` |
| MCP server types | `MCPServerType` enum defining `SSE`, `STDIO`, `STREAMABLE_HTTP` | `letta/functions/mcp_client/types.py:36-39` |
| MCP transport configs | `SSEServerConfig`, `StdioServerConfig`, `StreamableHTTPServerConfig` with `to_dict()` and templated variable resolution | `letta/functions/mcp_client/types.py:222-296` |
| MCP auth templating | `TEMPLATED_VARIABLE_REGEX = r"\{\{\s*([A-Z_][A-Z0-9_]*)\s*(?:\|\s*([^}]+?)\s*)?\}\}"` plus `resolve_environment_variables`, `resolve_custom_headers` | `letta/functions/mcp_client/types.py:14-15,127-202` |
| MCP base client | `AsyncBaseMCPClient.connect_to_server()`, `list_tools()`, `execute_tool()`, `cleanup()` with `AsyncExitStack` and expected error classification | `letta/services/mcp/base_client.py:41-153` |
| MCP streamable HTTP client | `AsyncStreamableHTTPMCPClient._initialize_connection()` wrapping `streamablehttp_client` with OAuth header injection and timeout via `tool_settings.mcp_connect_to_server_timeout` | `letta/services/mcp/streamable_http_client.py:16-81` |
| MCP REST API | Full CRUD + tool ops: `POST/GET/PATCH /mcp-servers`, `GET /{id}/tools`, `POST /{id}/tools/{tool_id}/run`, `PATCH /{id}/refresh` (`resync_mcp_server_tools`), `GET /connect/{id}` (SSE OAuth stream) | `letta/server/rest_api/routers/v1/mcp_servers.py:36-310` |
| MCP OAuth | `DatabaseTokenStorage` (persisted `OAuthToken`), `MCPOAuthSession`, `create_oauth_provider()`, `ServerSideOAuth`, `oauth_stream_event()` SSE helpers | `letta/services/mcp/oauth_utils.py:27-318` |
| MCP schema health | `MCPToolHealth` with `status: STRICT_COMPLIANT / NON_STRICT_ONLY / INVALID` | `letta/functions/mcp_client/types.py:21-27` |
| MCP persistence | `ToolType.EXTERNAL_MCP = "external_mcp"`; `MCPServer` ORM with encrypted `token_enc`/`custom_headers_enc` | `letta/schemas/enums.py:224` ; `letta/schemas/mcp.py:29-100` |
| JSON Schema generation | `generate_schema()` (inspect + docstring parsing), `type_to_json_schema_type()`, `pydantic_model_to_json_schema()` | `letta/functions/schema_generator.py:78-194,227-406,409-523` |
| MCP schema normalization | `normalize_mcp_schema()` adds `additionalProperties: false`, explicit `type` for `$ref`, recurses `$defs`; `generate_tool_schema_for_mcp()` inlines `$ref`, deduplicates `anyOf`, heals optional fields for strict mode | `letta/functions/schema_generator.py:586-903` |
| Strict-mode validator | `validate_complete_json_schema()` → `SchemaHealth` enum; checks for `additionalProperties`, empty-object/empty-array rejection, recursive `anyOf`/`oneOf` handling | `letta/functions/schema_validator.py:1-202` |
| Schema health tests | 13 tests covering STRICT vs NON_STRICT, healed optional fields, nested objects, type arrays, anyOf | `letta/tests/test_schema_validator.py:5-278` |
| MCP schema validation tests | `test_mcp_schema_validation.py` exercising `add_mcp_tool` and metadata `SCHEMA_STATUS` | `letta/tests/mcp_tests/test_mcp_schema_validation.py:100,581` |
| OTLP exporter deps | `opentelemetry-api==1.30.0`, `sdk==1.30.0`, `instrumentation-requests/sqlalchemy==0.51b0`, `exporter-otlp==1.30.0` | `letta/pyproject.toml:48-52` |
| OTLP tracing setup | `setup_tracing(endpoint, app, service_name)` creates `TracerProvider(resource=get_resource) + BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint))`, instruments requests + SQLAlchemy, adds `trace_method` decorator with param truncation | `letta/otel/tracing.py:145-160,228-435` |
| OTLP endpoint wiring | `if otlp_endpoint and not settings.disable_tracing: setup_tracing(endpoint=otlp_endpoint, app=app)` + `setup_metrics` guard; also respects `LETTA_OTEL_EXPORTER_OTLP_ENDPOINT` | `letta/server/rest_api/app.py:820-847` ; `letta/compose.yaml:48` |
| OTel collector configs | 5 collector configs (clickhouse/file/signoz) plus `start-otel-collector.sh` downloading `otelcol-contrib` | `letta/otel/otel-collector-config-file.yaml:1` ; `letta/otel/start-otel-collector.sh:7,99,135-145` |
| OpenAPI export | `openapi: 3.1.0` generated by FastAPI; `generate_openapi_schema(app)` writes `openapi_letta.json` and adds Letta-specific overrides | `letta/fern/openapi.json:2` ; `letta/server/rest_api/app.py:136-162,871-873` |
| OpenAPI generation script | `generate_openapi_schema.sh` via `uv run python -c '...generate_openapi_schema(app)'` | `letta/server/generate_openapi_schema.sh:2-12` |
| Provider abstraction | `ProviderType` enum (openai, anthropic, google_ai/vertex, bedrock, mistral, etc. ≈20 values) and `llm_api/` client directory with per-provider adapters | `letta/schemas/enums.py:53-78` ; `letta/llm_api/openai_client.py:1` |
| Tool schema portability adapters | `convert_tools_to_anthropic_format()` strips `REQUEST_HEARTBEAT_PARAM`; `convert_tools_to_google_ai_format()` + `get_function_call_response_schema()` handle Gemini vertex constraints | `letta/llm_api/anthropic_client.py:654,1412-1501` ; `letta/llm_api/google_vertex_client.py:348-488` |
| Composio adapter (non-MCP) | `ToolCreate.from_composio()`, `external_composio` tool type, REST `GET /v1/tools/composio/apps` endpoints | `letta/schemas/enums.py:222` ; `letta/tests/integration_test_composio.py:34-53` |
| Tool execution sandbox | Pluggable executors: `AsyncToolSandboxLocal/E2B/ModalV2`, `SandboxToolExecutor`, `MCPToolExecutor`, `ComposioToolExecutor` | `letta/services/tool_executor/tool_execution_sandbox.py:17` ; `letta/services/tool_executor/mcp_tool_executor.py:5` |

## Answers to Dimension Questions

### 1. Which open protocols are supported?

- **MCP (Model Context Protocol) — full support.** Dependencies `mcp>=1.9.4` + `fastmcp>=2.12.5` (`letta/pyproject.toml:57,76`). Three transports enumerated in `letta/functions/mcp_client/types.py:36-39` and implemented via `AsyncBaseMCPClient` (`letta/services/mcp/base_client.py:41`) plus transport-specific clients (`letta/services/mcp/streamable_http_client.py:16`, `sse_client.py`, `stdio_client.py`, `fastmcp_client.py`). REST surface exposes full lifecycle (`letta/server/rest_api/routers/v1/mcp_servers.py:31`). OAuth 2.0 with encrypted `Secret` storage (`letta/services/mcp/oauth_utils.py:27`, `letta/schemas/mcp.py:39-40`).
- **OpenTelemetry / OTLP — supported.** Pinned `opentelemetry-api/sdk==1.30.0` + `opentelemetry-exporter-otlp==1.30.0` (`letta/pyproject.toml:48-52`). `OTLPSpanExporter` wired in `letta/otel/tracing.py:157` gated by `settings.otel_exporter_otlp_endpoint` (`letta/server/rest_api/app.py:820`) and env `LETTA_OTEL_EXPORTER_OTLP_ENDPOINT` (`letta/compose.yaml:48`). Collector configs shipped (`letta/otel/otel-collector-config-*.yaml`).
- **JSON Schema — native.** Canonical tool schemas generated in `letta/functions/schema_generator.py:409` (`generate_schema`) and validated via `letta/functions/schema_validator.py:20` (`SchemaHealth`). MCP schemas normalized and healed in `letta/functions/schema_generator.py:586-903`.
- **OpenAPI — export-only.** FastAPI generates `openapi: 3.1.0` (`letta/fern/openapi.json:2`) via `generate_openapi_schema` (`letta/server/rest_api/app.py:136`). No OpenAPI-to-tool importer found; grep for `openapi.*import\|OpenAPI.*tool` across source returns only export and doc generation paths.
- **Provider APIs — 20 providers** via `ProviderType` enum (`letta/schemas/enums.py:53-78`) and `letta/llm_api/*_client.py`. OpenAI-compatible base handles `openai/openrouter/azure` with shared `openai_client.py`; Anthropic/Bedrock share `anthropic_client.py`; Google AI/Vertex share `google_vertex_client.py`. This gives provider-independent invocation at API layer.

### 2. Is MCP supported?

**Yes — comprehensive client-side support, no standalone MCP server exposure.** Evidence:

- Client: three server config types + `AsyncBaseMCPClient` with `list_tools`/`execute_tool` (`letta/services/mcp/base_client.py:83-129`), OAuth (`letta/services/mcp/oauth_utils.py:191`), tool health tracking (`letta/functions/mcp_client/types.py:21-27`), and resync (`letta/server/rest_api/routers/v1/mcp_servers.py:194-212` / `letta/services/mcp_manager.py:1-60`).
- Persistence: `MCPServer` + `MCPTool` mappings, encrypted tokens (`letta/schemas/mcp.py:29-100`), tool type `external_mcp` (`letta/schemas/enums.py:224`).
- API: 9 operations under `/mcp-servers` including SSE OAuth connect stream (`letta/server/rest_api/routers/v1/mcp_servers.py:215-310`).
- Schema pipeline: MCP inputSchema → `normalize_mcp_schema` → `validate_complete_json_schema` → `generate_tool_schema_for_mcp` with strict-mode healing (`letta/functions/schema_generator.py:694-903`).

Limitations: Letta does **not** expose itself as an MCP server (no `src/mcp_server` serving Letta tools over MCP). `STDIO` transport is disabled by default for multi-tenant safety (`letta/settings.py:45-54` → `mcp_disable_stdio=True`), requiring `LETTA_MCP_DISABLE_STDIO=false` for local use.

### 3. Is OpenTelemetry supported?

**Yes.** `opentelemetry-exporter-otlp==1.30.0` with gRPC exporter (`letta/otel/tracing.py:14-15,157`). `setup_tracing(endpoint, app)` installs `BatchSpanProcessor`, instruments `requests` and `sqlalchemy` (with DB pool monitoring opt-in at `letta/server/rest_api/app.py:836-847`), adds HTTP middleware `_trace_request_middleware` (`letta/otel/tracing.py:38-69`) and per-route dependency `_update_trace_attributes` (`letta/otel/tracing.py:72-124`). `trace_method` decorator wraps >40 service methods (grep `from letta.otel.tracing import trace_method` hits 30+ files). Collection via `otelcol-contrib` with prod/dev/file/signoz/ClickHouse variants (`letta/otel/otel-collector-config-*.yaml`). Controlled by `letta_otel_exporter_otlp_endpoint` setting (`letta/settings.py:355`) and `disable_tracing` kill-switch. Limitation: tracing is suppressed in pytest (`is_pytest_environment()` early-return at `letta/otel/tracing.py:150`), so CI lacks trace coverage.

### 4. Are tool schemas portable across providers?

**Partially — canonical JSON Schema is portable, but provider translation is ad-hoc.**

- Canonical model: All tools (Python source, `args_json_schema`, or MCP inputSchema) converge to JSON Schema Draft-07 via `generate_schema` / `generate_schema_from_args_schema_v2` / `generate_tool_schema_for_mcp` (`letta/functions/schema_generator.py:409,558,694`). `Tool.json_schema` and `args_json_schema` are generic JSON Schema dicts (`letta/schemas/tool.py:47-48`).
- Translation: Per-provider clients adapt at call time. `anthropic_client.py:654,1412` (`convert_tools_to_anthropic_format`) strips heartbeat params and rewrites `input_schema`. `google_vertex_client.py:348,488` rewrites for Gemini's limited OpenAPI subset. `openai_client.py:405` preserves `strict` flag for structured outputs. Each translation is bespoke, with provider-specific error messages (e.g., `google_vertex_client.py:111-153`).
- Portability gaps: No unified `ToolSchema -> ProviderSchema` abstraction or schema registry; the `Strict / Non-strict / Invalid` health is OpenAI-centric (`letta/functions/schema_validator.py:12-17`), not a neutral portability guarantee. Union types, `anyOf`, and `$ref` require healing (`normalize_mcp_schema`) that is MCP-specific and not reused for custom tools. Custom tools with `source_type=json` bypass docstring parsing (`letta/functions/ast_parsers.py:201`) but still require manual per-provider testing.
- Extensibility answer: **External tools can be added without writing custom adapters *only* if they speak MCP.** `POST /v1/mcp-servers` (`letta/server/rest_api/routers/v1/mcp_servers.py:36`) registers remote MCP servers; tools are auto-discovered and normalized. Non-MCP external tools (e.g., arbitrary REST APIs) require `ToolCreate.from_composio` or manual `ToolCreate` with Python source + `pip_requirements` — i.e., a custom adapter.

## Architectural Decisions

- **MCP as the interoperability backbone** (`letta/pyproject.toml:57` + `letta/functions/mcp_client/types.py:36`). Letta bets on MCP's `Tool.inputSchema` (JSON Schema) as the neutral tool contract, aligning with Anthropic/OpenAI ecosystem. Tradeoff: deeper MCP investment vs. limited OpenAPI import.
- **Encrypted Secret storage for MCP creds** (`letta/schemas/mcp.py:39-40` + `letta/schemas/secret.py`). `token_enc`/`custom_headers_enc` use `Secret.from_plaintext_async` with async encryption, separating plaintext from persistence. Decision favors multi-tenant security over debuggability.
- **Strict-mode healing rather than rejection** (`letta/functions/schema_generator.py:847-903`). Instead of marking optional fields as `NON_STRICT_ONLY` and blocking, `generate_tool_schema_for_mcp(strict=True)` promotes optional fields to `required` with `type: [..., "null"]`. Decision maximizes tool compatibility with OpenAI strict mode at cost of semantic drift (optional becomes required-nullable).
- **OTLP gRPC only** (`letta/otel/tracing.py:14`). Choice of `proto.grpc.trace_exporter` over HTTP/protobuf simplifies ClickHouse/Signoz integration (`letta/otel/otel-collector-config-clickhouse.yaml`) but precludes HTTP-only collectors.
- **STDIO disabled by default** (`letta/settings.py:45-54`). `mcp_disable_stdio=True` with explicit warning about local process spawning protects shared deployments; local users must opt-in. This is a deliberate security-vs-functionality fence.
- **No generic protocol adapter registry.** Distinct paths for MCP (`mcp_manager.py`), Composio (`external_composio`), and native Python tools exist without a common `ProtocolAdapter` interface. Each new protocol requires a new executor (`letta/services/tool_executor/mcp_tool_executor.py`, `composio_tool_executor.py`).

## Notable Patterns

- **Three-layer MCP stack**: `types.py` (config + templating) → `services/mcp/*_client.py` (transport) → `mcp_manager.py` (orchestration + persistence) → `routers/v1/mcp_servers.py` (REST). Clean separation with `AsyncExitStack` lifecycle (`letta/services/mcp/base_client.py:51,136-150`).
- **Template variable injection with fallback**: `{{ VAR | default }}` regex (`letta/functions/mcp_client/types.py:14`) resolved via `get_tool_variable` against `environment_variables` dict; headers are sanitized (`_sanitize_dict`) before resolution.
- **Schema validation as health signal, not gate**: `validate_complete_json_schema` returns `(SchemaHealth, reasons)` (`letta/functions/schema_validator.py:20`); `MCPTool.health` is populated post-normalization (`letta/functions/schema_generator.py:879-883`), allowing `NON_STRICT_ONLY` tools to remain usable outside strict mode.
- **Provider-specific schema shims**: Each `*_client.py` owns its `convert_tools_to_*_format` rather than a shared transpiler. Anthropic strips heartbeat; Google flattens unsupported fields — pattern is duplication over abstraction.
- **Collector-agnostic OTel wiring**: Collector configs are swappable (`config-clickhouse.yaml`, `config-signoz.yaml`, `config-file.yaml`) without code change, driven by `OTEL_EXPORTER_OTLP_ENDPOINT` env.

## Tradeoffs

- **MCP breadth vs. depth**: Supporting 3 transports + OAuth + templating gives high compatibility but increases attack surface and configuration complexity. Stdio toggle and `validate_mcp_server_url(..., resolve_hostname=False)` (`letta/schemas/mcp.py:60`, `letta/helpers/url_validation.py`) mitigate SSRF but add operational overhead.
- **Healing vs. fidelity**: Adding `additionalProperties: false` and `required + null` healing (`letta/functions/schema_generator.py:608-610,847-867`) makes 90% of MCP tools OpenAI-compatible, but mutates upstream schemas and can mask intent (e.g., truly optional nested objects become required-nullable).
- **Canonical schema vs. provider quirks**: A single JSON Schema canonical model simplifies authoring (one `generate_schema` path) but forces per-provider fixups at the edge, leading to duplicated logic and divergent error handling (`google_vertex_client.py:276,408` notes Gemini supports only a subset of OpenAPI schema).
- **Security vs. extensibility for external tools**: Without `POST /mcp-servers`, adding a REST API as a tool requires Composio or custom Python code with `pip_requirements` — no declarative OpenAPI importer. This limits low-code expansion.
- **OTLP gRPC vs. operational flexibility**: gRPC exporter yields efficient batch processing but locks out HTTP-only SaaS collectors unless a local `otelcol-contrib` relay is deployed (`letta/otel/start-otel-collector.sh:145`).

## Failure Modes / Edge Cases

- **MCP connection failures are classified but not retried uniformly**: `StreamableHTTPClient` maps 404/JSON validation to `ConnectionError` (`letta/services/mcp/streamable_http_client.py:63-81`); `base_client.py:30-34` lists 11 expected error names (McpError, ToolError, ConnectError, etc.) but `execute_tool` (`letta/services/mcp/base_client.py:104-129`) swallows exceptions into `(str(e), False)` — caller cannot distinguish transient vs. permanent without parsing string.
- **OAuth session expiry orphaned**: `cleanup_expired_oauth_sessions(max_age_hours=24)` (`letta/services/mcp/oauth_utils.py:260-272`) requires explicit invocation; no periodic task is wired by default, so `MCPOAuth` rows can accumulate.
- **Strict-mode inlining recursion limit**: `inline_ref(..., max_depth=10)` (`letta/functions/schema_generator.py:790`) silently returns unresolved `$ref` if depth exceeded — tools with deeply nested `$defs` (>10) will produce invalid schemas without error.
- **AdditionalProperties healing can break `anyOf`**: `normalize_mcp_schema` adds `type: object` to `$ref` properties in `anyOf` (`letta/functions/schema_generator.py:658-669`) — if the referenced schema is not an object, this creates a type mismatch that `validate_complete_json_schema` will later flag as `INVALID` rather than healing.
- **OTel endpoint missing fails closed**: `setup_tracing` asserts `endpoint` (`letta/otel/tracing.py:152`) but is only called if `otlp_endpoint` is truthy (`letta/server/rest_api/app.py:821`); misconfigured endpoint URL (e.g., missing `http://`) causes `OTLPSpanExporter` init to throw at startup, halting server despite `try/except` around SQLAlchemy instrumentation only.
- **Tool execution timeout fragmentation**: MCP timeouts are configurable via `tool_settings` (`mcp_connect_to_server_timeout=30`, `mcp_execute_tool_timeout=60` at `letta/settings.py:41-43`), but local sandbox timeout is `tool_sandbox_timeout=180` (`letta/settings.py:36`) — inconsistency can cause MCP tools to timeout earlier than local tools under identical load.

## Future Considerations

- Add an **OpenAPI → Tool importer** (mirroring `generate_tool_schema_for_mcp`) that fetches `openapi.json`, filters `operations`, and produces `ToolCreate` objects — closes the “no custom adapters” gap without inventing a new protocol.
- Introduce a **`ProtocolAdapter` interface** (`list_tools`, `execute`, `health_check`) so MCP, Composio, LangChain, and future A2A adapters share registration, auth, and pagination logic instead of parallel executors.
- **Unify tool-schema transpilation**: Extract `convert_tools_to_*_format` into a `ToolSchemaTranspiler` registry keyed by `ProviderType`, with snapshot tests against recorded provider payloads to prevent drift.
- **Wire `cleanup_expired_oauth_sessions`** to APScheduler/Temporal batch poller (`letta/settings.py:418-424`) to prevent token table bloat.
- Support **`OTLP HTTP/protobuf`** exporter alongside gRPC via `OTEL_EXPORTER_OTLP_PROTOCOL` env to broaden SaaS collector compatibility.

## Questions / Gaps

- No evidence found for **OTLP metric/log export** — only `OTLPSpanExporter` (`letta/otel/tracing.py:14`); `otel/metrics` setup exists (`letta/server/rest_api/app.py:825`) but exporter type not verified. Search of `metrics.py` and `otel/*.yaml` is needed to confirm metric temporality vs. trace pipeline.
- No evidence found for **JSON Schema Draft version pinning** — `generate_schema` produces Draft-07 style (`letta/functions/schema_generator.py:394`), but MCP's `Tool.inputSchema` may use 2020-12 features (`$defs` vs `definitions`). Compatibility matrix is undocumented.
- No evidence found for **MCP resources/prompts** — only `Tool` is wrapped (`letta/functions/mcp_client/types.py:6`, `29`); MCP spec also defines resources and prompts, but Letta ignores them. Clarify whether this is intentional scoping.
- MCP server **self-exposure** (Letta as MCP server) is absent — no `FastMCP` server factory serving Letta tools. This limits Letta's use as a tool provider in MCP-native clients.

---

Generated by `Dimension 19.01: Protocol Compatibility` against `letta`.
