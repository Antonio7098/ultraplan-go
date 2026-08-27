# Source Analysis: crewai

## Dimension 19.01: Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python / Pydantic, OpenTelemetry, MCP SDK, LiteLLM + native providers |
| Analyzed | 2026-08-27 |

## Summary

CrewAI has mature, multi-transport MCP support as a client (Stdio, Streamable HTTP, SSE) with resolver, tool-wrapper and event coverage, plus strong JSON Schema / Pydantic-native tool-schema pipeline that is shared across OpenAI, Anthropic, Bedrock and Gemini providers via sanitizers. OpenTelemetry/OTLP trace export is present but single-purpose: telemetry to CrewAI's collector via `OTLPSpanExporter` (proto/http) wrapped in `SafeOTLPSpanExporter`. OpenAPI import for tools is absent (OpenAPI files are only documentation references for the hosted enterprise API). Provider abstraction exists via `LLM` router and native completion classes, but tool schemas are not framework-agnostic (Pydantic/CrewAI-specific) and require internal conversion.

## Rating

**6 / 10** — Present but inconsistent.

**Rationale:** MCP tool discovery/execution with timeouts, retries, auth handling and filtering is explicit and tested (`lib/crewai/src/crewai/mcp/*`, `lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py`). JSON Schema generation from Pydantic via `generate_model_description` with provider-specific sanitizers (`sanitize_tool_params_for_openai_strict`, `sanitize_tool_params_for_anthropic_strict`) is implemented and used by all native providers. OTLP export is implemented correctly but narrow (telemetry-only, single hard-coded endpoint, global-provider avoidance noted). OpenAPI-driven tool creation is missing, and `MCPServerAdapter` is Stdio/SSE-only (no native HTTP) creating duplicate adapters. External tools still need a Pydantic/CrewAI adapter; no generic OpenAPI→tool importer exists.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP - dependencies | `mcp~=1.28.1` pinned in main package and `crewai-tools[mcp]` includes `mcp>=1.28.1,<2` + `mcpadapt>=0.1.9` | `lib/crewai/pyproject.toml:43` , `lib/crewai-tools/pyproject.toml:102-105` |
| MCP - config models | Pydantic configs for three transports: `MCPServerStdio`, `MCPServerHTTP`, `MCPServerSSE` with `tool_filter`, `cache_tools_list`, discriminated union `MCPServerConfig` | `lib/crewai/src/crewai/mcp/config.py:12-123` |
| MCP - transports | Transport classes `StdioTransport`, `HTTPTransport`, `SSETransport` via `BaseTransport` | `lib/crewai/src/crewai/mcp/transports/base.py:1` , `lib/crewai/src/crewai/mcp/transports/stdio.py:1` , `lib/crewai/src/crewai/mcp/transports/http.py:1` , `lib/crewai/src/crewai/mcp/transports/sse.py:1` |
| MCP - client | `MCPClient` with `connect`/`disconnect`, `list_tools`/`call_tool_result`/`list_prompts`/`get_prompt`, retry with backoff, `AsyncExitStack`, event bus emits, timeouts 30s | `lib/crewai/src/crewai/mcp/client.py:59-773` |
| MCP - tool resolver | `MCPToolResolver.resolve()` handles HTTPS URLs, AMP slugs, native configs; `_resolve_external` via `MCPToolWrapper`, `_resolve_native` creates per-call `MCPNativeTool` with `client_factory`, caching, filtering, AMP bulk fetch via `PlusAPI.get_mcp_configs` | `lib/crewai/src/crewai/mcp/tool_resolver.py:45-647` |
| MCP - native wrappers | `MCPNativeTool` (per-invocation `MCPClient` + flatten `isError`) and legacy `MCPToolWrapper` (on-demand `streamablehttp_client`) | `lib/crewai/src/crewai/tools/mcp_native_tool.py:17-152` , `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:16-202` |
| MCP - adapter (legacy) | `MCPServerAdapter` in crewai-tools using `mcpadapt.MCPAdapt` + `CrewAIToolAdapter.adapt()` with `create_model_from_schema`, `StdioServerParameters` or dict SSE, tool filtering | `lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:26-235` |
| MCP - events | Seven typed events: `MCPConnection{Started,Completed,Failed}`, `MCPToolExecution{Started,Completed,Failed}`, `MCPConfigFetchFailedEvent` with checkpoint config list | `lib/crewai/src/crewai/events/types/mcp_events.py:1-98` , `lib/crewai/src/crewai/state/checkpoint_config.py:83-89` |
| MCP - agent integration | Agent holds `mcps: list[str|MCPServerConfig]`, `get_mcp_tools()`, `_cleanup_mcp_clients()`, merge into task tools via `_inject_mcp_tools`/`_add_mcp_tools` | `lib/crewai/src/crewai/agent/core.py:81,132,203,209,1232-1252,1450-1455` , `lib/crewai/src/crewai/crew.py:1678-1840` |
| MCP - tests | Functional tests for stdio + SSE context-manager and filter-by-name | `lib/crewai-tools/tests/adapters/mcp_adapter_test.py:77-237` |
| MCP - feature note | Telemetry feature tag `"mcp:connection"` emitted | `lib/crewai/src/crewai/telemetry/telemetry.py:1256` |
| OpenTelemetry / OTLP | Dependencies `opentelemetry-api/sdk/exporter-otlp-proto-http~=1.42.0`; `SafeOTLPSpanExporter` wraps `OTLPSpanExporter`, `TracerProvider`+`BatchSpanProcessor`, endpoint `{CREWAI_TELEMETRY_BASE_URL}/v1/traces`, timeout 30s, resource `SERVICE_NAME` | `lib/crewai/pyproject.toml:21-23` , `lib/crewai/src/crewai/telemetry/telemetry.py:27-148,71-91,129-149` |
| OpenTelemetry - isolation | `set_tracer()` deliberately avoids installing global `TracerProvider` to prevent hijacking host process; creates spans via `self.provider.get_tracer`, `CommonAttributesSpanProcessor` | `lib/crewai/src/crewai/telemetry/telemetry.py:173-191,138-140` |
| OpenTelemetry - baggage propagation | `opentelemetry.baggage` used in `crew_context`, `crews/utils`, `crew.py`, `flow/runtime` | `lib/crewai/src/crewai/utilities/crew/crew_context.py:5` , `lib/crewai/src/crewai/crew.py:21-22` , `lib/crewai/src/crewai/flow/runtime/__init__.py:35-36` |
| JSON Schema - model→schema | `generate_model_description()` produces `{"type":"json_schema","json_schema":{"name","strict":true,"schema":...}}` with `force_additionalProperties=false`, `strip_unsupported_formats`, `ensure_type_in_schemas`, ref resolution, `convert_oneof_to_anyof` | `lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:648-691` |
| JSON Schema - schema→model | `create_model_from_schema()` + `_json_schema_to_pydantic_type()` / `_build_model_from_schema()` with `jsonref`, `FORMAT_TYPE_MAP`, cyclic detection via `ForwardRef` | `lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:809-1262` |
| JSON Schema - sanitizers | Provider-specific pipelines: `sanitize_tool_params_for_openai_strict`, `..._anthropic_strict`, `..._bedrock_strict` with `strip_unsupported_formats`, `lift_top_level_anyof`, strict metadata stripping | `lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:615-645` |
| JSON Schema - tool conversion | `convert_tools_to_openai_schema()` → `{"type":"function","function":{"name","description","parameters","strict":true}}` using `generate_model_description(..., strip_null_types=False)` | `lib/crewai/src/crewai/utilities/agent_utils.py:237-304` |
| JSON Schema - structured tool | `CrewStructuredTool`/`BaseTool` with `args_schema: type[BaseModel]` serialized via `model_json_schema()`, inferred from `@tool` decorator signature, validated at run | `lib/crewai/src/crewai/tools/structured_tool.py:33-48,300-331,357-379` , `lib/crewai/src/crewai/tools/base_tool.py:149-255,701-772` |
| JSON Schema - flow state | `FlowJsonSchemaStateDefinition` with `json_schema: dict`, `FlowDefinition.model_json_schema(by_alias=True)` template | `lib/crewai/src/crewai/flow/flow_definition.py:62-133` , `lib/crewai/src/crewai/flow/flow_definition.py:325` |
| Provider abstraction | `LLM.__new__` factory routing to native providers (`OpenAICompletion`, `AnthropicCompletion`, `GeminiCompletion`, `BedrockCompletion`, `AzureCompletion`, `SnowflakeCompletion`, `OpenAICompatible...`) or LiteLLM fallback; `get_supported_openai_params` gating | `lib/crewai/src/crewai/llm.py:368-716` |
| Provider - native tool schemas | Each provider calls sanitizer before submit: OpenAI `sanitize_tool_params_for_openai_strict`, Anthropic `...anthropic_strict`, Bedrock same as Anthropic | `lib/crewai/src/crewai/llms/providers/openai/completion.py:1875-1876` , `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:581-583` , `lib/crewai/src/crewai/llms/providers/bedrock/completion.py:625,914` |
| Provider - generic LiteLLM | LiteLLM path delegates to `litellm.completion(**params)` with `drop_params=True` | `lib/crewai/src/crewai/llm.py:730-731,1277` |
| OpenAPI - absence | No importer/generator of tools from OpenAPI/Swagger; grep finds only doc-site `openapi:` front-matter and frozen `enterprise-api.*.yaml` specs | `lib/crewai` grep `openapi: "/v1.14.3/enterprise-api.en.yaml POST /kickoff"` pattern in docs e.g. `docs/v1.14.3/en/api-reference/kickoff.mdx:4`, no code in `lib/crewai/src` |
| Schema portability | `extract_tool_info`/`safe_tool_conversion` handle OpenAI vs direct vs Anthropic (`name`/`input` vs `function`/`parameters`) normalization, but no universal adapter registry | `lib/crewai/src/crewai/llms/providers/utils/common.py:44-146` , `lib/crewai/src/crewai/utilities/agent_utils.py:1366-1465` |

## Answers to Dimension Questions

### 1. Which open protocols are supported?

- **MCP (Model Context Protocol)**: Fully supported as *client*. Three transports (Stdio, HTTP/streamable, SSE), discovery (`list_tools`/`list_prompts`/`get_prompt`), execution (`call_tool_result` with `isError` handling), filtering and caching. Server role is not implemented (no exposure of CrewAI tools via MCP server). (`lib/crewai/src/crewai/mcp/client.py:59-773`, `lib/crewai/src/crewai/mcp/config.py:12-123`, `lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:26-235`)
- **OTLP/OpenTelemetry (outbound)**: Supported for telemetry export only. Uses `opentelemetry-api/sdk` + `OTLPSpanExporter` over HTTP protobuf to `https://…/v1/traces` via `SafeOTLPSpanExporter`+`BatchSpanProcessor`. No user-configurable collector; telemetry can be disabled via `OTEL_SDK_DISABLED`/`CREWAI_DISABLE_TELEMETRY`. (`lib/crewai/src/crewai/telemetry/telemetry.py:27-148`, `lib/crewai/pyproject.toml:21-23`)
- **JSON Schema**: First-class. `generate_model_description` ↔ `create_model_from_schema` round-trip, plus strict sanitizers per provider. MCP `inputSchema` is JSON Schema and converted to Pydantic args_schema on ingest. (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:648-691,809-862`)
- **OpenAPI**: **Not supported** for tool ingestion. OpenAPI 3.0.3 files under `docs/` (`enterprise-api.*.yaml`) are only Mintlify documentation sources (`openapi: "/v1.14.3/enterprise-api.en.yaml POST /kickoff"`), not imported to create tools.
- **A2A (Agent-to-Agent)**: Optional (`a2a-sdk~=0.3.10` extra) with telemetry tag `a2a:delegation` (`lib/crewai/pyproject.toml:109`, `lib/crewai/src/crewai/telemetry/telemetry.py:1256`), but not in scope of this dimension.

### 2. Is MCP supported?

**Yes — client side, comprehensive.**

- **Supported**: tools, prompts, resources listing; streamable HTTP / SSE / stdio transports; HTTPS URL refs with optional `#toolName` suffix; AMP registry via CrewAI+ API; tool name sanitization; `ToolFilter` (static/dynamic); TTL cache (300s); timeouts (10–30s) with 3 retries and typed failure events. Two integration surfaces: modern `MCPToolResolver`+`MCPNativeTool` (per-call client) and legacy `MCPServerAdapter` (mcpadapt). (`lib/crewai/src/crewai/mcp/tool_resolver.py:45-647`, `lib/crewai/src/crewai/mcp/client.py:59-773`, `lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:31-89`, `lib/crewai/src/crewai/events/types/mcp_events.py:1-98`)
- **Not supported / gaps**: MCP server mode (exposing CrewAI as MCP server); SSE→HTTP automatic fallback; legacy adapter lacks native HTTP transport (only Stdio + dict-SSE). No streaming `resources` subscription.

### 3. Is OpenTelemetry supported?

**Yes, but narrowly scoped to internal telemetry.**

- Proves OTLP correctness: `Resource(SERVICE_NAME)`, `TracerProvider`, `CommonAttributesSpanProcessor`, `BatchSpanProcessor(SafeOTLPSpanExporter(endpoint=BASE_URL/v1/traces))`, `force_flush/shutdown` with signal/atexit handlers, env-opt-out (`OTEL_SDK_DISABLED`, `CREWAI_DISABLE_TELEMETRY`, `CREWAI_DISABLE_TRACKING`). (`lib/crewai/src/crewai/telemetry/telemetry.py:71-167,192-265`)
- **Limitations**: Single hard-coded collector (CrewAI telemetry backend), no user-supplied `OTEL_EXPORTER_OTLP_ENDPOINT` wiring exposed, `SafeOTLPSpanExporter` swallows export errors (returns `FAILURE` silently). Tracing does not provide generic user span export for application observability — `set_tracer()` intentionally avoids setting the global provider (`lib/crewai/src/crewai/telemetry/telemetry.py:173-191`).

### 4. Are tool schemas portable across providers?

**Partially.**

- **Portable core**: All tools are defined as Pydantic `args_schema` → JSON Schema via `generate_model_description` (`lib/crewai/src/crewai/tools/structured_tool.py:33`, `lib/crewai/src/crewai/tools/base_tool.py:149`), then `convert_tools_to_openai_schema` emits canonical OpenAI-compatible form (`lib/crewai/src/crewai/utilities/agent_utils.py:237-304`). `@tool` decorator auto-derives `args_schema` from function signature (`lib/crewai/src/crewai/tools/base_tool.py:701-772`), so external functions need no custom adapter — just type hints + docstring.
- **Non-portable edges**: Providers apply incompatible sanitization before submission:
  - OpenAI strict: `force_additionalProperties=false` + `anyOf` + `additionalProperties=false` + strip unsupported formats. (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:615-623`)
  - Anthropic/Bedrock strict: additionally strips `minimum`/`maximum`/`pattern`/etc. and lifts top-level `anyOf` (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:626-645`).
  A schema valid on OpenAI can lose constraints on Anthropic, and `ensure_all_properties_required` forces all fields required, which divergences callers must know. 
- **Missing**: No OpenAPI→tool generator, so REST APIs still need manual `BaseTool` wrapping; no cross-framework tool JSON export (e.g., LangChain/Zod) beyond OpenAI shape.

> Can external tools be added without writing custom adapters? **Yes for Python functions / Pydantic tools / MCP servers; No for OpenAPI REST services** — `@tool`-decorated callables and MCP servers are auto-adapted (`lib/crewai/src/crewai/tools/base_tool.py:701-772`, `lib/crewai/src/crewai/mcp/tool_resolver.py:68-88`), but an OpenAPI spec requires hand-written `BaseTool` subclasses.

## Architectural Decisions

- **Per-invocation MCP clients instead of shared sessions** (`lib/crewai/src/crewai/mcp/tool_resolver.py:315-324`, `lib/crewai/src/crewai/tools/mcp_native_tool.py:17-36`): Each tool call creates a fresh `MCPClient`+transport. Decision trades connection latency for thread safety; avoids shared mutable state under producer-consumer event loops (`lib/crewai/src/crewai/mcp/tool_resolver.py:363-371` thread-pool bridging).
- **Pydantic as canonical tool-schema IR** (`lib/crewai/src/crewai/tools/base_tool.py:149-255`, `lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:1-14`): All tool definitions converge to `type[BaseModel]`; JSON Schema is derived output, not source of truth, enabling type-safe validation (`BaseTool._validate_kwargs` at `lib/crewai/src/crewai/tools/base_tool.py:279-300`).
- **Provider-specific sanitizers rather than one strict schema** (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:604-645`, `lib/crewai/src/crewai/llms/providers/openai/completion.py:1875`, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:581`): Preserves provider correctness at cost of portability.
- **No global OTel provider installation** (`lib/crewai/src/crewai/telemetry/telemetry.py:173-191`): Intentional to avoid contaminating host app instrumentation and to keep telemetry spans on CrewAI's provider only.
- **Dual MCP adapters (legacy mcpadapt vs native)** (`lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:26-94` vs `lib/crewai/src/crewai/mcp/client.py:59`): Preserves backward compatibility for `MCPServerAdapter(StdioServerParameters)` users while new `mcp` extra supports HTTP/SSE natively.

## Notable Patterns

- **JSON Schema ↔ Pydantic round-trip with cycle guards**: `create_model_from_schema` uses `jsonref.replace_refs` + `in_progress: dict[id→ForwardRef|BaseModel]` to handle cyclic `$ref` graphs without stack overflow (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:864-902,872-902`).
- **Retry with exponential backoff + error classification**: `_retry_operation` and `_retry_mcp_discovery` distinguish non-retryable auth/not-found vs retryable network/timeout (`lib/crewai/src/crewai/mcp/client.py:688-739`, `lib/crewai/src/crewai/mcp/tool_resolver.py:537-595`).
- **Tool name sanitization at every boundary**: `sanitize_tool_name` applied in `convert_tools_to_openai_schema`, `safe_tool_conversion`, `extract_tool_call_info`, MCP adapters (`lib/crewai/src/crewai/utilities/agent_utils.py:1366-1465`, `lib/crewai/src/crewai/llms/providers/utils/common.py:103,112,138`, `lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:52`).
- **Event-bus observability for protocols**: Every MCP connect/tool execution emits typed events (`lib/crewai/src/crewai/mcp/client.py:160-242,468-543`) consumable via `crewai_event_bus`.
- **Checkpoint-safe tool serialization**: `tool_type: f"{module}.{qualname}"` registry + `_resolve_tool_dict` enables JSON checkpoint round-trip without importing arbitrary types (`lib/crewai/src/crewai/tools/base_tool.py:50-79,201-205`).

## Tradeoffs

- **Connection-per-call simplifies concurrency but hurts latency/cost**: Fresh TCP/TLS+handshake per tool invocation (including remote MCP over HTTPS) increases P95 tool time vs pooled/session reuse. TTL cache for `list_tools` only partially mitigates.
- **Strictness normalization loses fidelity across providers**: `strip_unsupported_formats` and `_CLAUDE_STRICT_UNSUPPORTED` removal silently drops numeric/string constraints; users authoring one schema see different validation semantics on another provider.
- **Single telemetry endpoint vs generic observability**: Hard-coded `CREWAI_TELEMETRY_BASE_URL` with `SafeOTLPSpanExporter` that swallows errors makes CrewAI observable to the vendor but not to the user's collector without forking.
- **Two MCP paths create confusion**: Users must choose between `agent(mcps=[...])` (native) vs `MCPServerAdapter` (crewai-tools). Feature parity diverges (e.g., HTTP support only in native).
- **`ensure_all_properties_required` + `additionalProperties: false` maximizes provider accept rates but forces optionality to be expressed via presence/absence rather than `null`, requiring `strip_null_from_types` and `strip_null_from_types` opts (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:462-512,648-691`).

## Failure Modes / Edge Cases

- **MCP auth failures masked as network errors**: `tool_resolver._retry_mcp_discovery` classifies only `"authentication"/"unauthorized"` strings as non-retryable; novel 401 payloads without those substrings will be retried 3× before surfacing (`lib/crewai/src/crewai/mcp/tool_resolver.py:588-590`).
- **MCP `asyncio.run` nesting deadlocks**: Resolver bridges sync callers inside a running loop via `ThreadPoolExecutor + contextvars.copy_context` (`lib/crewai/src/crewai/mcp/tool_resolver.py:363-371`, `lib/crewai/src/crewai/mcp/client.py:363-371`). Cancellation inside `ClientSession.initialize` raises `CancelledError` vs `BaseExceptionGroup` branches; mis-handled `anyio` groups could leak.
- **Silent schema constraint stripping**: Constraints removed for strict mode are not logged at warning level; tool author may not realize `minimum`/`maxLength` etc. were dropped on Anthropic (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:546-580`).
- **Cache staleness**: `_mcp_schema_cache` TTL 300s is global, no invalidation on tool churn; rapid server deployments serving new tools will be stale for up to 5m (`lib/crewai/src/crewai/mcp/client.py:51-52`, `lib/crewai/src/crewai/mcp/tool_resolver.py:41-42`).
- **OTLP exporter double-failure swallowing**: `SafeOTLPSpanExporter.export` logs at `error` then returns `FAILURE`; `BatchSpanProcessor` will retry per SDK, but `Telemetry._safe_telemetry_operation` also swallows exceptions (`lib/crewai/src/crewai/telemetry/telemetry.py:71-91,267-284`), so user never learns collector is down.
- **Tool name collisions after sanitization**: `convert_tools_to_openai_schema` de-duplicates via `_n` suffix loop (`lib/crewai/src/crewai/utilities/agent_utils.py:283-289`); collisions after sanitizing `"my-tool"` vs `"my_tool"` are silent and ordering-dependent.
- **No OpenAPI validation path**: Supplying an OpenAPI spec does not error clearly — no code path checks for it, so users may assume support that is absent.

## Future Considerations

- Unify `MCPServerAdapter` and `MCPToolResolver` into single entrypoint with explicit deprecation; add HTTP transport to legacy adapter or remove it.
- Add OpenAPI 3.x → `BaseTool` generator (or `openapi-to-json-schema` shim) to close the API-driven tool gap; expose as `crewai_tool.from_openapi(spec, operationId)`.
- Expose OTel exporter config: honor `OTEL_EXPORTER_OTLP_ENDPOINT`/`OTEL_TRACES_EXPORTER` and allow user-provided `SpanExporter`; add switch to emit user spans to their collector alongside telemetry.
- Short-circuit MCP cache on `MCPConnectionCompletedEvent` or expose `refresh=True` param.
- Log at `warning` when strict sanitizers strip constraints, with per-provider diff, so portability regressions are visible.
- Consider connection pooling for remote MCP (keep-alive) to reduce per-call latency without sacrificing isolation.

## Questions / Gaps

- No evidence found of MCP `resources`/`prompts` being surfaced as CrewAI tools or context — `MCPClient.list_prompts`/`get_prompt` exist (`lib/crewai/src/crewai/mcp/client.py:615-687`) but `MCPToolResolver` never exposes them; search for prompt/resource usage in `agent/core.py` returned no consumers.
- No evidence found of MCP notification / progress streaming (`notifications/progress`) handling.
- No evidence found of OpenAPI importer tests or code — grep for `openapi` in `lib/crewai/src` returned zero hits aside from docs.
- No evidence found of custom OTLP endpoint configuration or `OTEL_EXPORTER_OTLP_*` env var handling beyond `OTEL_SDK_DISABLED` (`lib/crewai/src/crewai/telemetry/telemetry.py:161-167`).
- Search boundary: inspected only `studies/agent-harness-study/sources/crewai/lib/crewai`, `lib/crewai-tools`, `lib/crewai-core` (implicit) and `pyproject.toml` files; did not inspect frozen `docs/v*` beyond grep hits.

---

Generated by `Dimension 19.01: Protocol Compatibility` against `crewai`.
