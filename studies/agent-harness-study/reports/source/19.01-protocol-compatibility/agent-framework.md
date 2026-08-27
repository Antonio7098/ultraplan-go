# Source Analysis: agent-framework

## Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (.NET + declarative) - Monorepo |
| Analyzed | 2026-08-26 |

## Summary

Agent Framework implements a protocol-centric extensibility layer: MCP is a first-class, transport-agnostic client abstraction (stdio, streamable-http, websocket) with sampling guardrails, progressive disclosure, and MCP Tasks (SEP-2663); OpenTelemetry is native with OTLP exporters for gRPC and HTTP/protobuf driven by standard `OTEL_*` env vars; JSON Schema is the canonical tool/structured-output format generated from Pydantic or supplied dicts; OpenAPI is supported as a hosted/server-side tool via declarative models and Foundry integration; provider APIs are abstracted behind `BaseChatClient`/`SupportsChatGetResponse` with portable `FunctionTool` schemas. No custom adapters are required for external tools in the common cases—`@tool` + JSON Schema, MCP, or OpenAPI covers the spectrum—but MCP is client-only (no embedded server) and OpenAPI execution is server-side only.

## Rating

**8/10** — Clear, tested, and documented protocol model with operational safeguards (sampling rate limits, allowlists, OTel context propagation, denylists, progressive disclosure collision handling). OTLP and JSON Schema are production-grade; MCP coverage is comprehensive for client use but not a server implementation; OpenAPI is declarative/hosted rather than a generic client-side importer. Downgraded from 9 because OpenAPI import is not a standalone client-side fetch-and-execute path and MCP server hosting is absent.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP client/server implementations | `MCPTool` base class with transport subclasses `MCPStdioTool`, `MCPStreamableHTTPTool`, `MCPWebsocketTool` | `python/packages/core/agent_framework/_mcp.py:424-441` |
| MCP transports | `streamable_http_client` lazy import for HTTP transport; stdio/websocket variants referenced in same module | `python/packages/core/agent_framework/_mcp.py:389-402` |
| MCP progressive disclosure | `use_progressive_disclosure`, `always_load`, loader tools `list_mcp_tools`/`load_tool`/`unload_tool` with collision handling | `python/packages/core/agent_framework/_mcp.py:520-535` , `python/packages/core/agent_framework/_mcp.py:884-942` |
| MCP sampling guardrails | `SamplingApprovalCallback`, `sampling_max_tokens`, `sampling_max_requests`, deny-by-default callback | `python/packages/core/agent_framework/_mcp.py:130-144` , `python/packages/core/agent_framework/_mcp.py:502-515` |
| MCP long-running tasks | `MCPTaskOptions` (frozen dataclass, experimental `MCP_LONG_RUNNING_TASKS`) with poll interval bounds and cancel-on-abandonment | `python/packages/core/agent_framework/_mcp.py:347-387` , `python/packages/core/agent_framework/_mcp.py:322-333` |
| MCP arg allowlist / meta injection | `_MCP_FRAMEWORK_DENYLIST`, `_prepare_call_kwargs` filtering, `_meta` validation, `_inject_otel_into_mcp_meta` | `python/packages/core/agent_framework/_mcp.py:115-125` , `python/packages/core/agent_framework/_mcp.py:243-313` |
| MCP skills over resource templates | SEP-2640 `mcp-resource-template` and `completion/complete` flow, `AgentMcpSkillsSource` handling `skill://index.json` | `docs/decisions/0029-mcp-skill-templates-and-direct-references.md:19-68` , `docs/decisions/0029-mcp-skill-templates-and-direct-references.md:60-112` |
| MCP decision matrix | Tool matrix showing Remote MCP Servers as provider-driven hosted tool | `docs/decisions/0002-agent-tools.md:122` , `docs/decisions/0002-agent-tools.md:1366-1493` |
| OTLP exporters | `_create_otlp_exporters` handles both gRPC and HTTP/protobuf with three signal exporters; `_get_exporters_from_env` parses standard OTel env vars | `python/packages/core/agent_framework/observability.py:422-545` , `python/packages/core/agent_framework/observability.py:548-639` |
| OTLP env var spec | `OTEL_EXPORTER_OTLP_ENDPOINT`, `TRACES/METRICS/LOGS_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `HEADERS` handling with HTTP base-url auto-append `/v1/{signal}` | `python/packages/core/agent_framework/observability.py:586-614` , `python/packages/samples/02-agents/observability/README.md:325-338` |
| OTLP dependency bounds | OTLP exporters listed as optional bounds: `opentelemetry-exporter-otlp-proto-grpc`, `opentelemetry-exporter-otlp-proto-http` | `python/scripts/dependencies/_dependency_bounds_runtime.py:33-34` |
| OpenTelemetry instrumentation pattern | Decision doc selects `OpenTelemetryAgent` wrapper pattern following `Microsoft.Extensions.AI` with `AgentOpenTelemetryConsts` | `docs/decisions/0003-agent-opentelemetry-instrumentation.md:75-87` |
| JSON Schema generator | `FunctionTool` lazy schema from Pydantic `model_json_schema()`, caching in `_input_schema_cached`, `parameters()`, `to_json_schema_spec()` | `python/packages/core/agent_framework/_tools.py:804-826` , `python/packages/core/agent_framework/_tools.py:890-903` |
| JSON Schema via decorator | `@tool(schema=...)` accepts `BaseModel` subclass or raw JSON Schema dict; mapping validation via `_validate_arguments_against_schema` | `python/packages/core/agent_framework/_tools.py:1135-1185` , `python/packages/core/agent_framework/_tools.py:1085-1132` |
| JSON Schema tests | Tests for dict-schema decorator, invoke with mapping, missing-required, invalid-type, custom properties preservation | `python/packages/core/tests/core/test_tools.py:92-252` |
| Structured output JSON Schema | `ChatResponseFormat.ForJsonSchema<T>` and `useJsonSchemaResponseFormat` for structured output | `docs/decisions/0016-structured-output.md:27` , `docs/decisions/0014-feature-collections.md:107` |
| Declarative JSON Schema conversion | `PropertySchema.to_json_schema()` and `_normalize_schema_node` handling `kind->type`, `additionalProperties: false` | `python/packages/declarative/agent_framework_declarative/_models.py:286-302` , `python/packages/declarative/agent_framework_declarative/_models.py:239-249` |
| OpenAPI importer (declarative) | `OpenApiTool` class with `kind="openapi"` and `specification` field; dispatched from `Tool.from_dict` | `python/packages/declarative/agent_framework_declarative/_models.py:819-841` , `python/packages/declarative/agent_framework_declarative/_models.py:622-624` |
| OpenAPI hosted execution | `OpenAPI Spec Tool` decision entry with payload/spec object, and Foundry sample using `FoundryAITool.CreateOpenApiTool` / `OpenApiFunctionDefinition` | `docs/decisions/0002-agent-tools.md:124` , `docs/decisions/0002-agent-tools.md:1631-1663` , `dotnet/samples/02-agents/AgentProviders/foundry/Agent_Step17_OpenAPITools/Program.cs:21-94` |
| Provider abstraction | `BaseChatClient` + `SupportsChatGetResponse` + `OpenAIChatClient` / `OpenAIChatCompletionClient` lazy exports | `python/packages/core/agent_framework/__init__.py:46-57` , `python/packages/core/agent_framework/openai/__init__.py:17-31` |
| Portable tool schemas | Standardized JSON-based tool definition with `type: function`, `parameters: JSON Schema object`, normalized via `normalize_tools` | `docs/decisions/0002-agent-tools.md:395-397` , `python/packages/core/agent_framework/_tools.py:962-1025` |
| OTel observability samples | `configure_otel_providers()` with env-var and explicit exporter construction examples | `python/samples/02-agents/observability/configure_otel_providers_with_env_var.py:11-119` , `python/samples/02-agents/observability/configure_otel_providers_with_parameters.py:127-158` |
| .NET MCP hosting | `Microsoft.Agents.AI.Mcp` project with task-aware client extensions, skills source `AgentMcpSkillsSource` | `dotnet/src/Microsoft.Agents.AI.Mcp/McpClientTaskExtensions.cs:14` , `dotnet/src/Microsoft.Agents.AI.Mcp/Skills/AgentMcpSkillsSource.cs:20` |
| User-Agent telemetry | `prepend_agent_framework_to_user_agent` and feature bitmask for operational observability | `python/packages/core/agent_framework/__init__.py:26-32` , `docs/specs/004-feature-usage-telemetry.md:1-30` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**
- **MCP (Model Context Protocol)**: Full client support in Python (`python/packages/core/agent_framework/_mcp.py:424`) and .NET (`dotnet/src/Microsoft.Agents.AI.Mcp/*`). Transports: stdio, streamable-http, websocket. Extensions: sampling (`sampling/createMessage`), long-running tasks (SEP-2663), skills-over-MCP (SEP-2640 via `skill://index.json`). Hosting helpers in `python/packages/hosting-mcp` and `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp`.
- **OTLP/OpenTelemetry**: Native instrumentation (`python/packages/core/agent_framework/observability.py:422`) with gRPC and HTTP/protobuf exporters, standard env vars, resource/metrics views. .NET decision follows `OpenTelemetryAgent` wrapper (`docs/decisions/0003-agent-opentelemetry-instrumentation.md:75`).
- **JSON Schema**: Canonical for tool parameters (`python/packages/core/agent_framework/_tools.py:890`), function declaration (`to_json_schema_spec`), structured output (`docs/decisions/0016-structured-output.md:27`), and declarative schemas (`python/packages/declarative/agent_framework_declarative/_models.py:286`).
- **OpenAPI**: Supported as hosted tool kind `openapi` (`python/packages/declarative/agent_framework_declarative/_models.py:820`) and via Foundry OpenAPI function definitions (`dotnet/samples/02-agents/AgentProviders/foundry/Agent_Step17_OpenAPITools/Program.cs:31`). Execution is server-side; no generic client-side OpenAPI-to-local-function importer.
- **Provider APIs**: Abstracted via `BaseChatClient` (`python/packages/core/agent_framework/__init__.py:47`), with concrete clients for OpenAI (Responses + Chat Completions), Foundry, Anthropic, Gemini, Bedrock, Ollama, etc., loaded lazily.

**2. Is MCP supported?**
Yes, comprehensively as a **client**. `MCPTool` (`python/packages/core/agent_framework/_mcp.py:424`) owns the `ClientSession`, generates `FunctionTool`s from `tools/list`, exposes `functions` with progressive disclosure, allowlists, `tool_name_prefix`, sampling approval, and task-aware polling. No embedded MCP **server** implementation found — the framework does not expose its own tools over MCP; it consumes remote MCP servers. Skills-over-MCP (`docs/decisions/0029-mcp-skill-templates-and-direct-references.md:15`) and Foundry toolbox MCP proxy (`docs/decisions/0025-foundry-toolbox-support.md:248`) extend this.

**3. Is OpenTelemetry supported?**
Yes, maturely. `python/packages/core/agent_framework/observability.py:422-639` implements `_create_otlp_exporters` and `_get_exporters_from_env` honoring `OTEL_EXPORTER_OTLP_ENDPOINT`, per-signal overrides, `OTEL_EXPORTER_OTLP_PROTOCOL` (grpc vs http), and headers, with correct HTTP base-url `/v1/{signal}` auto-append (`python/packages/core/agent_framework/observability.py:606-610`). `configure_otel_providers` wires tracer/meter/logger providers, console exporters, and VS Code extension exporters. `OtelAttr` (`python/packages/core/agent_framework/observability.py:223`) aligns with GenAI semconv, and tool spans are enriched in `python/packages/core/agent_framework/_tools.py:733-772`. Tests in `python/packages/core/tests/core/test_observability.py:1011-1237` cover the exporter logic (though skipped when OTLP deps absent).

**4. Are tool schemas portable across providers?**
Yes, with qualification. `FunctionTool.parameters()` (`python/packages/core/agent_framework/_tools.py:816`) produces a single JSON Schema (Pydantic-derived or dict-supplied) via `to_json_schema_spec()` (`python/packages/core/agent_framework/_tools.py:890-903`) used by all providers. `docs/decisions/0002-agent-tools.md:395` explicitly states the standardized `type: function` + `parameters: JSON Schema object` converges providers. `normalize_tools` (`python/packages/core/agent_framework/_tools.py:962`) and middleware `FunctionInvocationContext` propagation preserve portability. Provider-specific nuances remain: system-instructions vs chat history mapping, `toolChoice`/`allowMultipleToolCalls` semantics, and hosted vs local execution (e.g., OpenAPI/MCP hosted tools are provider-executed, not local `FunctionTool.invoke` paths). No evidence of automatic transpilation for provider-exclusive tool types beyond the hosted `openapi`/`mcp`/`code_interpreter`/`file_search` taxonomy.

## Architectural Decisions

- **Wrapper over aspect-oriented for OTel** (`docs/decisions/0003-agent-opentelemetry-instrumentation.md:58`): Chose `OpenTelemetryAgent` delegating wrapper matching `Microsoft.Extensions.AI.OpenTelemetryChatClient` to keep telemetry optional, testable, and non-invasive.
- **Single JSON Schema as lingua franca** (`python/packages/core/agent_framework/_tools.py:890` + `docs/decisions/0002-agent-tools.md:395`): All tool parameters are JSON Schema; Pydantic is the authoring surface, dict is the escape hatch.
- **MCP as transport-agnostic client abstraction** (`python/packages/core/agent_framework/_mcp.py:424`): One `MCPTool` base with `additional_tool_argument_names`, `_MCP_FRAMEWORK_DENYLIST`, and `_meta` handling to bridge MCP's wire format to `FunctionTool` without leaking framework internals.
- **Progressive disclosure for large MCP surfaces** (`python/packages/core/agent_framework/_mcp.py:884`): Loader tools (`list_mcp_tools`/`load_tool`/`unload_tool`) avoid bloating the model context; `tool_name_prefix` prevents collisions.
- **Deny-by-default sampling** (`python/packages/core/agent_framework/_mcp.py:130`): MCP servers are untrusted; `sampling/createMessage` requires explicit `sampling_approval_callback`, capped by `sampling_max_tokens`/`sampling_max_requests`.
- **OTLP env-var fidelity** (`python/packages/core/agent_framework/observability.py:586`): Replicates SDK auto-append semantics for HTTP so `OTEL_EXPORTER_OTLP_ENDPOINT` as base works correctly.
- **Declarative OpenAPI as hosted tool** (`python/packages/declarative/agent_framework_declarative/_models.py:820`): OpenAPI is a `kind: openapi` tool resource executed server-side (Foundry/Responses), not a local HTTP client generator — keeps execution trust on the platform.

## Notable Patterns

- **Lazy provider loading** (`python/packages/core/agent_framework/__init__.py:355`, `python/packages/core/agent_framework/openai/__init__.py:35`): `__getattr__` defers `agent-framework-openai` import until `OpenAIChatClient` is accessed.
- **Allowlist derived from server schema** (`python/packages/core/agent_framework/_mcp.py:115`): MCP call args filtered to `inputSchema.properties` + explicit extras; `_MCP_FRAMEWORK_DENYLIST` drops framework objects even if declared.
- **ContextVar propagation for OTel** (`python/packages/core/agent_framework/_mcp.py:294`, `python/packages/core/agent_framework/observability.py:134`): Trace context injected into MCP `_meta` via `propagate.inject`; telemetry conversation id via `contextvars`.
- **In-memory archive extraction for MCP skills** (`python/packages/core/AGENTS.md` MCPSkillsSource notes): ZIP/TAR archives from MCP are unpacked in-memory, never on disk, with zip-slip guards.
- **Immutable task options** (`python/packages/core/agent_framework/_mcp.py:348`): `MCPTaskOptions` frozen dataclass replaced wholesale via `MCPTool.task_options = ...`.

## Tradeoffs

- **MCP client-only vs server**: Consuming external MCP servers is ergonomic; exposing framework tools *as* an MCP server requires external hosting (`python/packages/hosting-mcp`), not built-in.
- **Server-side OpenAPI execution**: Simplifies auth/consent via `project_connection_id` (`docs/decisions/0025-foundry-toolbox-support.md:281`) but prevents local, offline OpenAPI tool testing without Foundry.
- **OTLP exporter opt-in**: Core depends only on `opentelemetry-api` (`python/packages/core/pyproject.toml:30`); `opentelemetry-sdk` + `opentelemetry-exporter-otlp-*` are optional, so OTLP tests are `skipif` (`python/packages/core/tests/core/test_observability.py:1009`) — durability depends on extras installation.
- **JSON Schema flexibility vs validation strictness**: `_validate_arguments_against_schema` (`python/packages/core/agent_framework/_tools.py:1085`) does lightweight type/enum/required checks for dict-schemas, but full `oneOf`/`anyOf`/nested-`additionalProperties` semantics are not enforced client-side.
- **Progressive disclosure complexity**: Loader tools reduce context but introduce stateful `ctx.add_tools`/`remove_tools` and collision rules (`python/packages/core/agent_framework/_mcp.py:945-1014`) that operators must understand.

## Failure Modes / Edge Cases

- **MCP task abandoned still running**: Hard `McpError` during `tasks/get` (except 408 timeout) or `max_task_wait` expiry fires best-effort `tasks/cancel` before raising `_MCPTaskAbandoned` (`python/packages/core/agent_framework/_mcp.py:335-344`); callers see `ToolExecutionException` but must assume side effects may have occurred.
- **Submit-vs-track reconnect**: Dropped connection before `task_id` known raises `connection lost; task state unknown` without retrying `tools/call` to avoid double-execution; after `task_id` known, `tasks/get`/`tasks/result` reconnect once (`python/packages/core/agent_framework/_mcp.py:332`).
- **MCP sampling confused deputy**: Without `sampling_approval_callback`, all `sampling/createMessage` is denied (`python/packages/core/agent_framework/_mcp.py:505`); even approved requests are truncated to `sampling_max_tokens` and throttled to `sampling_max_requests` per session.
- **OTLP endpoint misconfiguration**: HTTP base endpoint without auto-append would send to wrong path; framework compensates (`python/packages/core/agent_framework/observability.py:606`) but a user passing `http/protobuf` with full signal-specific env vars must ensure they are full URLs (used verbatim).
- **MCP argument smuggling**: Runtime `function_invocation_kwargs` shared across all tools can reach any MCP server declaring that property (`python/packages/core/AGENTS.md` MCPTool notes); mitigation requires not placing secrets in `function_invocation_kwargs` or isolating via `ContextVar`/`http_client`/`env`.
- **MCPSkills archive limits**: File-count, uncompressed-size, and download-size caps enforced; `..` zip-slip aborts whole skill (`python/packages/core/AGENTS.md` MCPSkillsSource notes) — legitimate large archives may be rejected.
- **Observability disable stickiness**: `disable_instrumentation()` sets `_user_disabled` sticky flag (`python/packages/core/agent_framework/observability.py:829`); later `enable_sensitive_telemetry()` without `force=True` is silently ignored, which can surprise integrations.

## Future Considerations

- Add an embedded **MCP server** hosting layer so framework tools can be exposed to external MCP clients without custom adapters (currently only hosting helpers exist).
- Provide a **client-side OpenAPI importer** that fetches a spec and materializes local `FunctionTool`s for offline use, complementing the current server-side `OpenApiTool`.
- Promote `MCPTaskOptions` from experimental (`python/packages/core/agent_framework/_mcp.py:347`) to stable once `MCP_LONG_RUNNING_TASKS` proves at scale; expose poll bounds as tunable.
- Lighten OTLP optional dependency story — consider a `telemetry` extra that bundles `opentelemetry-sdk` + exporters so `configure_otel_providers` works out-of-the-box.
- Extend `_validate_arguments_against_schema` to cover `oneOf`/`anyOf`/`not` and nested schema merging now handled only by `_normalize_schema_node` for declarative paths.

## Questions / Gaps

- No evidence of **OpenTelemetry log/metrics Views** usage beyond default `create_metric_views` (`python/packages/core/agent_framework/observability.py:718`) — are exemplar or cardinality controls documented?
- No evidence of **JSON Schema generation for streaming tool calls** — how are partial/incremental `function_call` arguments validated?
- **A2A protocol** (`python/packages/a2a`) and **AG-UI** (`python/packages/ag-ui`) are present in the repo but not mapped to the dimension's scope — should they be counted as additional open protocols?
- **Declarative PowerFx** evaluation (`python/packages/declarative/agent_framework_declarative/_models.py:19`) gates env access via `_safe_mode_context` — is the security boundary for OpenAPI/MCP URLs using PowerFx expressions audited?

---

Generated by `Dimension 19.01: Protocol Compatibility` against `agent-framework`.
