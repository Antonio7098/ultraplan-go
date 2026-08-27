# Source Analysis: openai-agents-sdk

## Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python / OpenAI Agents SDK (Pydantic, httpx2, MCP) |
| Analyzed | 2026-08-26 |

## Summary

OpenAI Agents SDK is OpenAI Responses-centric but provides durable adapters for open protocols. MCP is first-class with local transports (stdio, SSE, Streamable HTTP), Hosted MCP provider tool, `MCPServerManager` lifecycle, strict JSON Schema handling for tool params/outputs, and portable tool abstractions via `Model` interface + Litellm/AnyLLM bridges. Tracing uses a custom `TracingProcessor`/`TracingExporter` pipeline exporting to `https://api.openai.com/v1/traces/ingest` (`src/agents/tracing/processors.py:45`) — no OTLP/OpenTelemetry exporter. No OpenAPI importer was found. Tool schemas are generated from Python via Pydantic/Griffe and normalized to strict JSON Schema, enabling cross-provider portability only after conversion layer; external tools still require the SDK's `FunctionTool`/MCP wrapper rather than zero-adapter OpenAPI/MCP auto-import.

## Rating

**7/10** — Clear model with tests, explicit interfaces, and operational safeguards for MCP + JSON Schema; fragile/absent for OTLP/OpenAPI. MCP has multi-transport implementations, session serialization, retries/backoff, and credential-safe error handling with tests. JSON Schema strict conversion is well-tested and guarded (depth/budget limits). Deductions for missing OTLP exporter (proprietary ingest only), no OpenAPI import, and tool portability gated by manual adapters / conversion.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP dependency | `mcp>=1.19.0,<3` required dependency | `pyproject.toml:16` |
| MCP optional voice/realtime deps | `websockets`, `mcp` usage | `pyproject.toml:15-16` |
| MCP server base | `MCPServer` ABC, `connect/cleanup/list_tools/call_tool/list_prompts/get_prompt` + approval/tool-meta hooks | `src/agents/mcp/server.py:542-666` |
| MCP session base | `_MCPServerWithClientSession` cache, timeout validation, tool filtering, retries, locks | `src/agents/mcp/server.py:857-955` |
| MCP Stdio transport | `MCPServerStdio` with `StdioServerParameters`, `stdio_client` | `src/agents/mcp/server.py:1869-1971` |
| MCP SSE transport | `MCPServerSse` with `sse_client`, httpx2 factory handling | `src/agents/mcp/server.py:2002-2119` |
| MCP Streamable HTTP transport | `MCPServerStreamableHttp` with serialize lock, isolated retry session, v1/v2 branches | `src/agents/mcp/server.py:2163-2408` |
| MCP resources/prompts | `list_resources`, `list_resource_templates`, `read_resource`, `get_prompt`, pagination cursors | `src/agents/mcp/server.py:651-706,1610-1736` |
| MCP manager lifecycle | `MCPServerManager`, parallel/sequential mode, worker per server, timeout validation, reconnect | `src/agents/mcp/manager.py:151-575` |
| MCP tool conversion | `MCPUtil.to_function_tool`, strict schema copy, needs_approval policy, origin tagging | `src/agents/mcp/util.py:508-585` |
| MCP tool aggregation | `get_all_function_tools`, prefixed name overrides `mcp_{server}__{tool}`, collision handling | `src/agents/mcp/util.py:265-339` |
| Hosted MCP provider tool | `HostedMCPTool` wrapping `Mcp` config (`type: "mcp"`), alternative to local servers | `src/agents/tool.py:1086-1115` |
| MCP filtering | `ToolFilterStatic`/`ToolFilterCallable` + `_apply_tool_filter` | `src/agents/mcp/server.py:983-1067` , `src/agents/mcp/util.py:93-138` |
| JSON Schema strict conversion | `ensure_strict_json_schema`, `strict_schema.py` with node budget 100k, depth 100, open-object handling | `src/agents/strict_schema.py:115-152` , `src/agents/strict_schema.py:21-25` |
| JSON Schema copy gate | `_copy_json_schema` + depth validation before deepcopy | `src/agents/strict_schema.py:68-71` |
| Function tool schema | `FunctionTool.params_json_schema`, `strict_json_schema` enforcement in `__post_init__` | `src/agents/tool.py:440-599` |
| Function schema generator | `function_schema` via Pydantic `create_model` + griffe docstring, `ensure_strict_json_schema` | `src/agents/function_schema.py:286-487` |
| Agent output schema | `AgentOutputSchema` Pydantic TypeAdapter + `ensure_strict_json_schema` | `src/agents/agent_output.py:60-126` |
| Schema use in models | OpenAI Responses converter `strict` handling for function tools and handoffs | `src/agents/models/openai_responses.py:2090-2244` |
| ChatCompl converter schema | `chatcmpl_converter.py` strict output schema mapping | `src/agents/models/chatcmpl_converter.py:114-118` |
| Tracing processor interface | `TracingProcessor` ABC (on_trace_start/end, on_span_start/end, shutdown, force_flush) + `TracingExporter.export` | `src/agents/tracing/processor_interface.py:9-142` |
| Tracing exporters | `BackendSpanExporter` endpoint `https://api.openai.com/v1/traces/ingest`, `ConsoleSpanExporter`, `BatchTraceProcessor` queue | `src/agents/tracing/processors.py:44-541` |
| Tracing setup API | `add_trace_processor`, `set_trace_processors`, `set_tracing_export_api_key`, `TraceProvider` | `src/agents/tracing/__init__.py:94-130` , `src/agents/tracing/provider.py:103-182` |
| OTLP/OpenTelemetry search | No `opentelemetry`, `otel`, `OTLP` matches in `src/agents/tracing`; only custom processors | `src/agents/tracing/processors.py:1-15` (grep negative) |
| OpenAPI search | No `openapi|OpenAPI` implementation; only comment referencing LiteLLM can access OpenAPI models | `src/agents/extensions/models/litellm_model.py:150` , grep negative |
| Provider abstraction | `Model` / `ModelProvider` ABCs, `ModelTracing` enum for portability | `src/agents/models/interface.py:20-161` |
| LiteLLM adapter | `LitellmModel` bridges any model via `litellm.acompletion` → Responses format, converter for tools | `src/agents/extensions/models/litellm_model.py:149-241` , `src/agents/extensions/models/litellm_model.py:654-698` |
| AnyLLM adapter | `any_llm` optional dep, `AnyLLMModel/Provider` analogous bridge | `pyproject.toml:41` , `src/agents/extensions/models/any_llm_model.py` |
| Protocol adapter test — MCP | `tests/mcp/test_mcp_server_manager.py` etc. listed in reference doc | `.agents/references/local-mcp-server-lifecycle.md:48-58` |
| Tool origin portability | `ToolOrigin` + `ToolOriginType.MCP/FUNCTION/AGENT_AS_TOOL` for cross-provider correlation | `src/agents/tool.py:285-337` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**
- **MCP (Model Context Protocol): fully supported** — client side for tools/resources/prompts, all 3 transports, plus Hosted MCP provider tool (`src/agents/mcp/server.py:542`, `src/agents/tool.py:1086`). Server implementation is consumer-only; no MCP server mode to expose SDK tools externally.
- **JSON Schema: fully supported** — Pydantic/TypeAdapter → `model_json_schema()` → `ensure_strict_json_schema` (`src/agents/strict_schema.py:115`, `src/agents/function_schema.py:472`). Guardrails: 100k node budget, 100 depth (`src/agents/strict_schema.py:21-25`), DoS protections for untrusted MCP schemas.
- **Provider APIs: OpenAI-first but pluggable** — `Model` interface (`src/agents/models/interface.py:37`) with `OpenAIResponses` native, `LitellmModel`/`AnyLLMModel` adapters (`src/agents/extensions/models/litellm_model.py:149`). Described as accessing OpenAPI models via LiteLLM (`src/agents/extensions/models/litellm_model.py:150`), but this is model routing not OpenAPI import.
- **OTLP/OpenTelemetry: not supported** — tracing exports to proprietary OpenAI ingest (`src/agents/tracing/processors.py:45`) via `TracingProcessor`/`TracingExporter` (`src/agents/tracing/processor_interface.py:9`). Grep for `opentelemetry|OTLP|otel` returns no implementation.
- **OpenAPI: not supported as tool importer** — grep `openapi|OpenAPI` yields only the litellm comment; no generator/importer found.

**2. Is MCP supported?**
Yes, mature client + hosted. Evidence: `MCPServer` hierarchy with `Stdio/SSE/StreamableHttp` (`src/agents/mcp/server.py:1869,2002,2163`), `MCPServerManager` with task-affinity, timeouts, parallel mode, retry/backoff (`src/agents/mcp/manager.py:37,113,498`), util layer for filtering/meta/custom_data + strict conversion (`src/agents/mcp/util.py:240-509`), hosted `Mcp` tool (`src/agents/tool.py:1086`), pagination for prompts/resources (`src/agents/mcp/server.py:1610-1736`), and 8 test suites (`tests/mcp/*.py` per `.agents/references/local-mcp-server-lifecycle.md:52-58`). Supports tools/resources/prompts, resources pagination cursors (`src/agents/mcp/server.py:651,678`), and `ReadResourceResult` (`src/agents/mcp/server.py:693`). Gaps: no MCP server (expose) mode, no protocol version negotiation exposed.

**3. Is OpenTelemetry supported?**
No. Tracing is custom: `TracingProcessor`/`TracingExporter` (`src/agents/tracing/processor_interface.py:9`), `BackendSpanExporter` posting JSON `{"data": [...]}` to `api.openai.com/v1/traces/ingest` with retries, auth, truncation (`src/agents/tracing/processors.py:44-222`), batching via `BatchTraceProcessor` queue (8192 max, 0.7 trigger) (`src/agents/tracing/processors.py:548-574`). Registration via `add_trace_processor`/`set_trace_processors` (`src/agents/tracing/__init__.py:94`). No OTLP exporter, no `opentelemetry-api/sdk` integration, no semantic conventions. Adding OTel requires implementing `TracingExporter.export` or `TracingProcessor` bridge; no prior adapter exists.

**4. Are tool schemas portable across providers?**
Partially, after SDK conversion. Local definition is portable: Python functions → `FuncSchema` via `function_schema` (`src/agents/function_schema.py:286`) and Pydantic strict JSON Schema (`src/agents/strict_schema.py:115`). SDK then translates to provider wire format: OpenAI Responses (`src/agents/models/openai_responses.py:2090`), Chat Completions (`src/agents/models/chatcmpl_converter.py:973`), Realtime (`src/agents/realtime/openai_realtime.py:1841`). Cross-provider portability is achieved via `Model` abstraction (`src/agents/models/interface.py:37`) + adapters like `LitellmModel` which converts via `Converter.tool_to_openai`/`message_to_output_items` (`src/agents/extensions/models/litellm_model.py:578,345`). However schemas are OpenAI strict-flavored (requires `additionalProperties:false`, no `anyOf` at root). External tools cannot be added without an adapter: must wrap as `FunctionTool` (decorator `agents.decorators.tool`) or MCP server (`MCPUtil.to_function_tool`). No zero-code OpenAPI import; no generic JSON Schema → tool auto-registration.

> **Can external tools be added without writing custom adapters?** For MCP, partially yes: drop-in `MCPServerStdio/Sse/StreamableHttp` discovers tools via `list_tools` → `MCPUtil.get_all_function_tools` → `FunctionTool` (`src/agents/mcp/util.py:265`). For non-MCP external tools, no: must author `function_tool` wrapper or Hosted tool config; no OpenAPI/Swagger ingestion.

## Architectural Decisions

- **MCP client-dominated, transport-pluggable**: `MCPServer` ABC + `_MCPServerWithClientSession` with `create_streams()` factory (`src/agents/mcp/server.py:1070,1862`) isolates stdio/sse/streamable_http while sharing session, pagination, retry, and credential-safe error redaction. Enables v1/v2 compat (`mcp_compat.enable_legacy_httpx_compat` at `src/agents/mcp/server.py:922`).
- **Task-affinity for AnyIO**: `MCPServerManager._ServerWorker` per-server task + `anyio` cancel-scope awareness (`src/agents/mcp/manager.py:37,113`) keeps connect/cleanup on same task; docs warn against wrapping in helper that creates new task (`.agents/references/local-mcp-server-lifecycle.md:8`). Shared-session serialization via `_request_lock` for Streamable HTTP (`src/agents/mcp/server.py:952-961`).
- **Strict JSON Schema as lingua franca**: All function/output schemas normalized via `ensure_strict_json_schema` (`src/agents/tool.py:595`, `src/agents/function_schema.py:474`, `src/agents/agent_output.py:120`). Mitigates untrusted MCP schema DoS with node/depth budgets (`src/agents/strict_schema.py:21-25`). Open object handling commented as OpenAPI/MCP legacy (`src/agents/strict_schema.py:240`).
- **Proprietary tracing pipeline over OTel**: `TraceProvider` fan-out to `TracingProcessor`s (`src/agents/tracing/provider.py:103`), `BatchTraceProcessor` background thread + `BackendSpanExporter` to OpenAI ingest (`src/agents/tracing/processors.py:44`). Allows pluggable exporters but locks default observability to OpenAI.
- **Provider portability via abstract `Model`**: Single `get_response`/`stream_response` contract (`src/agents/models/interface.py:68,102`) with `LitellmModel`/`AnyLLMModel` as bridges (`src/agents/extensions/models/litellm_model.py:149`). Keeps tool schema definitions provider-agnostic at SDK layer, conversion deferred to model adapter.

## Notable Patterns

- **Credential-safe error graph**: HTTP transport errors sanitized — `_safe_transport_cause`, `_credential_safe_exception_group`, `_transport_error_urls_are_safe` (`src/agents/mcp/server.py:181-272`) strip URLs with credentials before logging/re-raising; connect vs cleanup error mapping separate (`src/agents/mcp/server.py:1132-1217`).
- **Name qualification for collision avoidance**: `mcp_{server}__{tool}` prefix with SHA1 hash suffix when >64 chars or duplicate base names (`src/agents/mcp/util.py:417-505`), reserved-name handling, deterministic sort.
- **Deep-copy isolation**: `_snapshot_tools` deep-copies cached MCP tools (`src/agents/mcp/server.py:131-133`), `_apply_dynamic_tool_filter` inspects detached copy (`src/agents/mcp/server.py:1042`), preventing filter mutation of cache.
- **Best-effort strict conversion with fallback**: `MCPUtil.to_function_tool` tries strict, logs and keeps original on failure, isolates metadata (`src/agents/mcp/util.py:545-562`); references specify this per `.agents/references/local-mcp-server-lifecycle.md:35`.
- **Failure as model-visible string**: `failure_error_function` per-server/agent with `default_tool_error_function`, `None` means propagate (`src/agents/mcp/server.py:848`), surfacing via `MCPError` handling in `invoke_mcp_tool` (`src/agents/mcp/util.py:733-751`).

## Tradeoffs

- **Ongoing**: MCP richness vs no OTel — deep MCP integration (3 transports, retries, filtering, 8 test groups) shows protocol focus, but observability remains proprietary, limiting interop with Grafana/Jaeger/OTLP collectors without custom `TracingProcessor`.
- **Ongoing**: Strict schema safety vs flexibility — guarantees OpenAI wire compatibility (root `anyOf` rejection `src/agents/strict_schema.py:138`, `additionalProperties:false` enforcement `src/agents/strict_schema.py:235-245`) but rejects permissive OpenAPI `additionalProperties:{}` or object-union roots, requiring `strict_json_schema=False`.
- **Ongoing**: SDK-managed MCP vs raw `mcp.ClientSession` — convenience (caching, filtering, approval policies `src/agents/mcp/server.py:710-846`) hides protocol details but masks low-level MCP features (sampling, notifications) and couples to `httpx`/`httpx2` via `HttpClientFactory` (`src/agents/mcp/util.py:77-91`).
- **Ephemeral**: V1/V2 compat complexity — `_compat` shims + `MCP_V2` branches (`src/agents/mcp/server.py:49-67,1323`) inflate transport code and `ignore_initialized_notification_failure` special-case for v1 tolerant transport (`src/agents/mcp/server.py:426-442`), traded for upgrade path.
- **Ongoing**: Serialization safety vs perf — pervasive `deepcopy`/`model_copy(deep=True)` for tools/schemas/custom_data (`src/agents/mcp/util.py:247,595`) and `MappingProxyType` for args (`src/agents/mcp/util.py:605`) prevents mutation at cost of allocations per tool call/listing.

## Failure Modes / Edge Cases

- **MCP pagination loops**: Repeated cursor detection aborts with `UserError` "repeated cursor while listing tools/prompts" (`src/agents/mcp/server.py:1444,1659`); buffered tools cleared to avoid partial state.
- **Schema DoS**: `$ref` fan-out limited to 100k nodes (`src/agents/strict_schema.py:21`), depth >100 raises `_SCHEMA_DEPTH_ERROR` (`src/agents/strict_schema.py:57`), `$id` nested resource crossing raises `_NESTED_RESOURCE_REF_ERROR` (`src/agents/strict_schema.py:43`), circular `$ref` chain raises (`src/agents/strict_schema.py:449`).
- **Transport credential leak**: URL with `user:s3cr3t_pw@` sanitized; error retains only `get_mcp_server_log_name(url)==url` safe URLs (`src/agents/mcp/server.py:181-216`, test `tests/test_tool_origin.py:170-191` shows `streamable_http: https://user:s3cr3t_pw@...` → `https://mcp.example.test:8443/mcp`).
- **Streamable HTTP session races**: Shared session serialized via `_request_lock` (`src/agents/mcp/server.py:956`); on `ClosedResourceError`/`CancelledError`/5xx, retries on isolated session (`src/agents/mcp/server.py:2325-2358`); `_SharedSessionRequestNeedsIsolation` → `_IsolatedSessionRetryFailed` bubble.
- **Cleanup masking**: `cleanup` errors after failed `connect` mapped to `UserError` with safe HTTP diagnostics; normal teardown logs warnings without masking original exception (`src/agents/mcp/server.py:1739-1837`, `src/agents/mcp/server.py:1369-1410`).
- **Retry budget misuse**: `_retry_backoff_seconds` exponential with optional cap (`src/agents/mcp/server.py:1219-1242`); `max_retry_attempts=-1` means unlimited, `0` is no retry (`src/agents/mcp/server.py:1230-1237`).
- **Tool required params**: Pre-call validation from cached `tool.inputSchema.required` before network call (`src/agents/mcp/server.py:1576-1608`); skips if not cached.
- **Tracing backpressure**: `BatchTraceProcessor` queue 8192/scheduled 5s/0.7 trigger (`src/agents/tracing/processors.py:548`); full queue drops span/trace with warning (`src/agents/tracing/processors.py:603,621`), exporter 5xx retries 3× with jitter (`src/agents/tracing/processors.py:62-216`).

## Future Considerations

- Add an `OTLPExporter implements TracingExporter` (or `TracingProcessor`) that maps `SpanData.export()` payloads to OTLP HTTP/gRPC protobuf, reusing existing `BatchTraceProcessor` backpressure. Currently only `BackendSpanExporter` (`src/agents/tracing/processors.py:44`) and `ConsoleSpanExporter` exist; example traces show only `details:{provider:"litellm"}` no OTel attrs (`tests/test_trace_processor.py:1013`).
- Introduce an OpenAPI→`FunctionTool` importer (or generate via `function_schema` + `Tool` factory) to satisfy “external tools without custom adapters” without manual wrapper. No such importer found (negative grep).
- Optionally expose MCP server mode (SDK tools as MCP server) for bidirectional interop; currently only consumes MCP.
- Consider centralizing `ensure_strict_json_schema` budgets/depth as configurable for large enterprise OpenAPI schemas; currently hard-coded (`src/agents/strict_schema.py:21,24`).
- Document LiteLLM/AnyLLM JSON Schema fidelity: strict-mode mismatch may require `strict_json_schema=False` per-provider; tests `tests/models/test_litellm*` cover but docs sparse.

## Questions / Gaps

- **No evidence found** for OpenAPI importer code: searched `openapi|OpenAPI` in `src/agents` — only comment at `src/agents/extensions/models/litellm_model.py:150` mentioning LiteLLM can access OpenAPI models; no importer/generator. Search boundary: entire `src/agents` and `pyproject.toml` optional deps.
- **No evidence found** for OTLP/OpenTelemetry: grepped `opentelemetry|OTLP|otel` — zero matches; confirmed exporter is OpenAI-specific (`src/agents/tracing/processors.py:45`). Would need custom processor to integrate OTel Collector.
- **MCP prompts/resources usage**: Server methods exist (`src/agents/mcp/server.py:1610,1678`) but runner integration focuses on tools; prompts/resources consumption path not evidenced in `run.py` (`src/agents/run.py`) search — gaps on how prompts feed into Agent context.
- **Tool schema extensibility**: Which JSON Schema keywords survive strict conversion? `oneOf`→`anyOf` handled (`src/agents/strict_schema.py:294`), but `not`, `if/then/else`, `patternProperties` not searched — unclear support boundary for complex OpenAPI schemas.

---
Generated by `19.01-protocol-compatibility` against `openai-agents-sdk`.
