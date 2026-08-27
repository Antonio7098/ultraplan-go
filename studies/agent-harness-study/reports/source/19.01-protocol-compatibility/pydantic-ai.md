# Source Analysis: pydantic-ai

## Dimension 19.01: Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ / uv workspace (`pydantic-ai-slim`, `pydantic-graph`, `pydantic-evals`) |
| Analyzed | 2026-08-27 |

## Summary

Pydantic AI is provider-agnostic but heavily protocol-aware. It exposes a single canonical `ToolDefinition` with `parameters_json_schema` (JSON Schema) that is transformed per-provider, and layers three open protocols on top: full MCP client via `fastmcp` (tools/resources/prompts/sampling/elicitation/tasks) with SDK v1/v2 compat, OpenTelemetry-native instrumentation (GenAI semconv spans, baggage, metrics) delegating OTLP export to the SDK, and JSON Schema as the lingua franca for tools and output. OpenAPI appears only as an internal Gemini-targeted transform (`GoogleOpenAPISchemaTransformer`) — there is no generic OpenAPI importer. External tools are pluggable without custom adapters via `MCPToolset`, `FunctionToolset`, and the `ext/langchain` bridge. The design trades bundling an OTLP exporter and an OpenAPI importer for lean core dependencies and explicit provider transforms.

## Rating

**8 / 10** — Clear, tested model with explicit interfaces and operational safeguards; falls short of 9–10 because OTLP export and OpenAPI ingestion are delegated/documented rather than bundled/proven at scale.

Rationale: MCP support is mature (typed dataclasses for every MCP entity, two-generation SDK compat, cached listings, task-augmented execution, `load_mcp_toolsets` with env-var expansion) and backed by `tests/test_mcp.py:1` integration tests. OTel conforms strictly to spec (`pydantic_ai_slim/pydantic_ai/AGENTS.md:17` "implement only spec-defined features") with versioned instrumentation (5/6), baggage propagation, and per-run message JSON caching to stay O(n). JSON Schema generation via `function_schema` + `JsonSchemaTransformer` is per-request transformed by `ModelProfile.json_schema_transformer` so schemas stay portable across OpenAI/Anthropic/Bedrock/Google. Weaknesses: no bundled `opentelemetry-exporter-otlp` (consumer brings `OTLPSpanExporter`), and no general OpenAPI-to-tool importer — Gemini’s OpenAPI subset is an output-only transform, limiting “API-driven tools.”

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP client/server implementations | `MCPToolset` imports `fastmcp.client.Client` and transports (`StdioTransport`, `StreamableHttpTransport`, `SSETransport`) and re-exports MCP types via `mcp.types`; constructor normalizes any `MCPToolsetClient` (`FastMCPClient | ClientTransport | FastMCP | AnyUrl | Path | str`) to `FastMCPClient` | `pydantic_ai_slim/pydantic_ai/mcp.py:27-51` |
| MCP client/server implementations | `MCPToolset` class doc and `AbstractToolset` inheritance; supports sampling/elicitation/logging/progress/roots/auth/headers` forwarded kwargs, `prefer_tasks`, caching, `include_instructions` | `pydantic_ai_slim/pydantic_ai/mcp.py:715-759` |
| MCP resources/prompts | Typed wrappers `Resource`, `ResourceTemplate`, `ResourceLink`, `EmbeddedResource`, `Prompt`, `PromptArgument`, `PromptResult`, `ServerCapabilities` with `from_mcp_sdk` converters and `mcp_optional_field` defensive reads | `pydantic_ai_slim/pydantic_ai/mcp.py:167-630` |
| MCP capabilities | `list_resources`, `read_resource`, `list_prompts`, `get_prompt`, `list_tools`, `get_tools`, `direct_call_tool` with cache invalidation on `notifications/{tools,resources,prompts}/list_changed` | `pydantic_ai_slim/pydantic_ai/mcp.py:1280-1390` |
| MCP compat | `is_mcp_sdk_v2()` version probe off `mcp` distribution; `mcp_field`, `mcp_optional_field`, `mcp_validated_field` bridge camelCase (v1) ↔ snake_case (v2) | `pydantic_ai_slim/pydantic_ai/_mcp_compat.py:14-56` |
| MCP sampling bridge | `map_from_mcp_params` / `map_from_pai_messages` convert between MCP `CreateMessageRequestParams` and `messages.ModelMessage`; handles `SystemPromptPart`/`UserPromptPart`/`BinaryContent` | `pydantic_ai_slim/pydantic_ai/_mcp.py:22-118` |
| MCP config loader | `load_mcp_toolsets` expands `${VAR:-default}` via `_ENV_VAR_PATTERN` and builds one `MCPToolset` per server entry; `MCPToolsetClient` TypeAlias covers URL/path/in-process server | `pydantic_ai_slim/pydantic_ai/mcp.py:642-705` |
| OTLP / OTel export | Core dep `opentelemetry-api>=1.28.0`; instrumentation imports `opentelemetry.trace`, `opentelemetry.baggage`, `opentelemetry.metrics`; `InstrumentationSettings` takes `tracer_provider`/`meter_provider` delegating to global provider (`logfire.configure()` or `set_tracer_provider`) | `pydantic_ai_slim/pyproject.toml:67` , `pydantic_ai_slim/pydantic_ai/models/instrumented.py:12-148` |
| OTLP / OTel export | Docs show `OTEL_EXPORTER_OTLP_ENDPOINT` + `OTLPSpanExporter` + `TracerProvider.add_span_processor` pattern; explicitly states “any OTLP backend works” without bundling exporter | `docs/logfire.md:170-223` , `docs/index.md:402` |
| OTel spans & gen-ai semconv | `open_model_request_span` builds `gen_ai.operation.name`, `gen_ai.provider.name`, `gen_ai.request.model`, `gen_ai.tool.definitions`, `gen_ai.input.messages`/`gen_ai.output.messages`, `server.address/port` | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:444-545` |
| OTel spans & gen-ai semconv | `Instrumentation` capability creates `invoke_agent {name}` run span with baggage `gen_ai.agent.name/call.id/conversation.id`, per-tool `execute_tool {name}` spans, versioned via `InstrumentationNames.for_version` (2–6) | `pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:68-591` |
| OTel messages spec | ` _otel_messages.py` defines `TextPart`, `ToolCallPart`, `ToolCallResponsePart`, `UriPart`, `BlobPart`, `ThinkingPart`, `ChatMessage` per `opentelemetry.io/docs/specs/semconv/gen-ai` | `pydantic_ai_slim/pydantic_ai/_otel_messages.py:1-123` |
| JSON Schema generation | `function_schema()` introspects signature + docstring, builds `core_schema` via Pydantic internal `_generate_schema`, validates with `SchemaValidator`, emits `json_schema` via `GenerateJsonSchema` | `pydantic_ai_slim/pydantic_ai/_function_schema.py:110-295` |
| JSON Schema transformer | `JsonSchemaTransformer` walks schema handling `properties`, `additionalProperties`, `prefixItems`, `allOf/anyOf/oneOf`, inlining `$defs` with recursive-ref detection; `InlineDefsJsonSchemaTransformer` inlines defs | `pydantic_ai_slim/pydantic_ai/_json_schema.py:15-233` |
| JSON Schema portable definition | `ToolDefinition.parameters_json_schema: ObjectJsonSchema` + `strict`, `return_schema`, `include_return_schema`; `Tool.tool_def` exposes portable schema; `Tool.from_schema` creates tool from raw `JsonSchemaValue` without Python function | `pydantic_ai_slim/pydantic_ai/tools.py:291-510` , `pydantic_ai_slim/pydantic_ai/tools.py:437-510` |
| JSON Schema per-provider adaptation | `models/__init__.py` selects `profile.get('json_schema_transformer')` in `prepare_request`; Gemini, OpenAI strict mode, etc. walk schema before sending | `pydantic_ai_slim/pydantic_ai/models/__init__.py:581-1807` |
| OpenAPI transform (Gemini) | `GoogleJsonSchemaTransformer` strips `$schema`, `const→enum`, `title`, `format→description`; `GoogleOpenAPISchemaTransformer` further inlines `$defs`, rewrites `anyOf [X,null]`, rejects recursive schemas — emits OpenAPI v3.0.3 subset | `pydantic_ai_slim/pydantic_ai/profiles/google.py:245-366` |
| OpenAPI usage | Realtime Google adapter maps Live `Schema` via `GoogleOpenAPISchemaTransformer` because Live only reads OpenAPI subset `parameters` field | `pydantic_ai_slim/pydantic_ai/realtime/google.py:93-492` |
| Provider APIs (model-independent) | 10+ model adapters (`openai.py:1701`, `anthropic.py:2573`, `google.py:1959`, `bedrock.py:720`, `mistral.py:410`, `groq.py:610`, `cohere.py:366`, `huggingface.py:459`, `xai.py:1237`) each map `ToolDefinition.parameters_json_schema` to provider-native tool spec | `pydantic_ai_slim/pydantic_ai/models/openai.py:1701` etc. |
| Protocol adapter | `ext/langchain.py:32-71` `tool_from_langchain` wraps any `LangChainTool` via `Tool.from_schema`, preserving `args` JSON schema; `LangChainToolset` aggregates | `pydantic_ai_slim/pydantic_ai/ext/langchain.py:32-71` |
| Observability safeguard | `docs/AGENTS.md` rule: “In `_otel_*.py` modules, implement only spec-defined features — no custom additions” | `pydantic_ai_slim/pydantic_ai/AGENTS.md:17` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**

- **MCP (Model Context Protocol) — full client.** Tools, resources, resource templates, prompts, sampling (`sampling_model` → `SamplingHandler`), elicitation, progress, roots, logging (`log_level`), OAuth/HTTP auth, stdio/SSE/StreamableHTTP/in-process transports, multi-server JSON config via `load_mcp_toolsets` (`pydantic_ai_slim/pydantic_ai/mcp.py:104-715`).
- **OpenTelemetry / OTLP — trace + metrics (OTLP via SDK).** Core depends on `opentelemetry-api` (`pydantic_ai_slim/pyproject.toml:67`); `InstrumentationSettings` accepts `tracer_provider`/`meter_provider` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:82-85`); emits GenAI semconv attributes (`gen_ai.*`, `gen_ai.tool.definitions`, `gen_ai.input.messages`) and histograms (`gen_ai.client.token.usage`, `gen_ai.client.operation.time_to_first_chunk`) (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:66-76`). Any OTLP backend works (`docs/logfire.md:212-220`); Logfire’s `instrument_pydantic_ai()` and `otel-tui` are documented alternatives, but `opentelemetry-exporter-otlp` is not a bundled dependency.
- **JSON Schema — native.** Every tool and output is defined by a JSON Schema derived from Python types via `GenerateJsonSchema` (`pydantic_ai_slim/pydantic_ai/_function_schema.py:110-295`), stored on `ToolDefinition` (`pydantic_ai_slim/pydantic_ai/tools.py:553`), then provider-transformed (`pydantic_ai_slim/pydantic_ai/_json_schema.py:15-233`).
- **OpenAPI — partial, provider-specific output only.** `GoogleOpenAPISchemaTransformer` converts JSON Schema → OpenAPI v3.0.3 subset for Gemini function declarations (`pydantic_ai_slim/pydantic_ai/profiles/google.py:298-366`); no generic OpenAPI-to-tool importer or spec ingestion.
- **Provider APIs — broad.** OpenAI (Chat + Responses), Anthropic, Google (GenAI), Bedrock, Mistral, Groq, Cohere, HuggingFace, xAI, OpenRouter, Cerebras/Snowflake/ZAI via OpenAI-compatible path, each with dedicated adapter under `pydantic_ai_slim/pydantic_ai/models/*.py`.

**2. Is MCP supported?**

**Yes — comprehensive client support.** `MCPToolset` (`pydantic_ai_slim/pydantic_ai/mcp.py:715`) is built on `fastmcp` (client-only `fastmcp-slim[client]` extra) and `mcp.types`. Features verified:

- Construction from URL, path, `FastMCP` in-process server, or pre-built `fastmcp.Client`; `auth: 'oauth' | httpx.Auth | bearer-string`, `verify`, `headers`/`http_client` (`pydantic_ai_slim/pydantic_ai/mcp.py:868-960`).
- Tools (`get_tools`/`list_tools` with caching and `notifications/tools/list_changed` invalidation), resources (`list_resources`/`read_resource` with `cache_resources`), resource templates (`ResourceTemplate:290`), prompts (`list_prompts`/`get_prompt` with `Prompt:422`, `PromptMessage:543`, `EmbeddedResource:490`, `ResourceLink:330`), server capabilities/instructions (`ServerCapabilities:575`, `capabilities`/`server_info`/`instructions` props `pydantic_ai_slim/pydantic_ai/mcp.py:1093-1127`).
- Sampling via `sampling_model: models.Model` or custom `sampling_handler` with `_mcp.py:22-118` bridge; elicitation, progress, message handlers; `process_tool_call` hook for per-call metadata/retry (`pydantic_ai_slim/pydantic_ai/mcp.py:827-841`).
- Compat layer `is_mcp_sdk_v2` + `mcp_field` readers (`pydantic_ai_slim/pydantic_ai/_mcp_compat.py:14-46`) allowing fastmcp 3 (SDK v1) and fastmcp 4 (SDK v2) concurrently; task-augmented execution (`prefer_tasks:779`, `fastmcp_tasks` SEP-1686 `pydantic_ai_slim/pydantic_ai/mcp.py:95-101` then SEP-2663 in v2).
- Tests: `tests/test_mcp.py:1` (construction-time conflict detection, lifecycle, caching) and `tests/test_agent.py:9743` multi-server scenarios.

No MCP *server* is implemented — the library is a client/consumer, not a provider.

**3. Is OpenTelemetry supported?**

**Yes — native and spec-strict, with OTLP as bring-your-own exporter.**

- Spans: `Instrumentation` (`pydantic_ai_slim/pydantic_ai/capabilities/instrumentation.py:68`) creates agent `invoke_agent` spans with baggage (`gen_ai.agent.name`, `gen_ai.agent.call.id`, `gen_ai.conversation.id`) (`_instrumentation.py:35-37`), model `chat <model>` CLIENT spans (`_instrumentation.py:476`), and `execute_tool {name}` spans with validation vs execution failure stages (`capabilities/instrumentation.py:349-381`). `models/instrumented.py:332-437` wraps standalone `Model.request`/`request_stream` via same `open_model_request_span`.
- Attributes: GenAI semconv (`gen_ai.system`, `gen_ai.request.model`, `gen_ai.tool.definitions` as JSON, `gen_ai.input/output.messages` via `_otel_messages.py:111`, `gen_ai.usage.*`/`gen_ai.aggregated_usage.*`, `gen_ai.response.*`, `server.address/port`, `gen_ai.client.operation.time_to_first_chunk`).
- Metrics: `gen_ai.client.token.usage`, `operation.cost`, `gen_ai.client.operation.time_to_first_chunk` histograms with advisory bucket boundaries (`_instrumentation.py:66-76`, `models/instrumented.py:169-203`).
- Propagation: `W3C traceparent` via `current_otel_traceparent` (`_instrumentation.py:643`), `capture_current_context` for streaming continuations (`_instrumentation.py:547-577`), `gateway.py:319` injects `traceparent` on forwarded requests.
- Export: `TracerProvider`/`MeterProvider` injected; default is global provider (`get_tracer_provider`/`get_meter_provider`). Logfire is 1-line ergonomic path (`logfire.configure` + `logfire.instrument_pydantic_ai`) but docs prove non-Logfire path with `OTLPSpanExporter` + `BatchSpanProcessor` + `set_tracer_provider` (`docs/logfire.md:212-227`) and declare “any OTLP backend works” (`docs/index.md:402`). No `opentelemetry-exporter-otlp` or `opentelemetry-sdk` in `pydantic_ai_slim/pyproject.toml` core deps — user installs them.

**4. Are tool schemas portable across providers?**

**Yes — tool schemas are provider-independent at definition time, then normalized per provider at request time.**

- Authoring: `Tool`/`ToolsetTool`/`FunctionToolset` and `Tool.from_schema` all produce a single `ToolDefinition` holding `parameters_json_schema: ObjectJsonSchema` derived from a Pydantic/typing introspected JSON Schema (`tools.py:291-510`, `_function_schema.py:110-295`). `ObjectJsonSchema` is `dict[str, Any]` with `type: object` guarantee via `check_object_json_schema`.
- Portability mechanism: in `Model.prepare_request`, `models/__init__.py:581` selects `profile.get('json_schema_transformer')` (e.g., `GoogleJsonSchemaTransformer`, `InlineDefsJsonSchemaTransformer`) and `walk()`s the schema before mapping it to the provider wire shape (`openai.py:1701` `parameters`, `anthropic.py:2573` `input_schema`, `bedrock.py:720` `inputSchema`, `google.py:1959` `parameters_json_schema`). Strict-mode compat (`is_strict_compatible`, `strict: bool | None`) is inferred per tool/provider rather than baked into the definition (`tools.py:565-581`, `profiles/google.py:250-258`).
- External tools without custom adapters: **Yes.** MCP servers are auto-converted (`mcp.py:1301-1308` `input_schema` → `parameters_json_schema`), and any LangChain tool via `ext/langchain.py:32-64` (`Tool.from_schema` path). `Tool.from_schema` also accepts arbitrary `JsonSchemaValue` directly, so OpenAPI-derived schemas could be injected if parsed externally — but there is no built-in OpenAPI importer.
- Limitation: portability is bounded by provider capabilities (e.g., Gemini’s OpenAPI subset strips `exclusiveMinimum`, `discriminator`, `const` → `enum` in `profiles/google.py:283-293`; recursive schemas error in `profiles/google.py:346`). The transformer’s `is_strict_compatible` flag surfaces incompatibility before request rather than silently degrading.

## Architectural Decisions

- **Single canonical `ToolDefinition` + per-provider transformers** (`tools.py:543`, `_json_schema.py:15`, `profiles/google.py:245`). Keeps tool authoring model-agnostic while isolating provider quirks to profile/adapter layers; consistent with `profiles/AGENTS.md:1` “Put intrinsic model-family facts here” vs `providers/AGENTS.md:1` “Place provider-specific code in `models/{provider}.py`”.
- **MCP on `fastmcp` client, not raw `mcp` SDK** (`mcp.py:27-51`). FastMCP provides transports (HTTP/SSE/stdio/in-process), OAuth, task extension, and era-neutral session handling; `mcp.py:102` folds optional `fastmcp-tasks` into session at construction time.
- **Bidirectional SDK v1/v2 compat** (`_mcp_compat.py:14-56`). `wire_name` / `mcp_field` readers let the same `mcp.py` run against fastmcp 3 (SDK v1 camelCase) and fastmcp 4 (SDK v2 snake_case) with forward-compatible gaps (e.g., `lastModified` on `Annotations`).
- **OTel strict spec fidelity** (`pydantic_ai_slim/pydantic_ai/AGENTS.md:17`). `_otel_messages.py:1` mirrors `semantic-conventions/docs/gen-ai/non-normative/models.ipynb` exactly; custom fields like `builtin`/`code_arg_name` are marked “Not part of spec, used by Logfire” (`_otel_messages.py:35-37`).
- **Delegated OTLP export** (`models/instrumented.py:85`, `docs/logfire.md:212`). Avoids forcing `opentelemetry-sdk`/`exporter-otlp` on all users; leans on global provider set by Logfire or manual `TracerProvider`. Tradeoff: one extra setup step for non-Logfire OTLP.
- **Per-request `prepare_request` schema walk** (`models/__init__.py:1791-1807`). Re-walking instead of caching transformed schema allows concurrent provider use with different strict/subset needs but means `prepare_request` is not idempotent (noted in `models/instrumented.py:362-366`).
- **Caching + invalidation for MCP listings** (`mcp.py:786-853`, `1280-1293`). `cache_tools/resources/prompts` default `True` with invalidation via `notifications/*/list_changed` or last `__aexit__`, balancing latency against dynamic servers.

## Notable Patterns

- **Compat reader pattern.** `mcp_field_value(value, name)` checks `name in type(value).model_fields` then falls back to `wire_name(name)` camelCase, returning `None` for future spec fields absent in installed SDK (`_mcp_compat.py:26-46`).
- **Fragment caching for OTel messages.** `MessageJsonCache: dict[int, CachedMessageJson]` keyed by `id(message)` with `parts` identity token, keeping history grows from O(history²) to O(new messages) per request (`_instrumentation.py:90-120`, `models/instrumented.py:225-251`).
- **`Tool.from_schema` escape hatch.** Bypasses Pydantic schema generation; `SchemaValidator(any_schema)` + raw `JsonSchemaValue` lets externally-derived schemas (e.g., LangChain, hand-written) enter the same pipeline (`tools.py:437-493`).
- **Wrapper capability for cross-cutting concerns.** `Instrumentation` extends `AbstractCapability[Any]._safe_at_runtime` and installs outermost wrapping for `wrap_run`/`wrap_model_request`/`wrap_tool_execute`/`wrap_output_process` without touching `AbstractToolset` base (`capabilities/instrumentation.py:78-82`, `capabilities/AGENTS.md:7`).
- **Typed metadata for OTel rendering.** `ToolCallPartOtelMetadata` (`_otel_messages.py:19-28`) flows from `ToolDefinition.metadata.code_arg_name/language` → `BaseToolCallPart.otel_metadata` via `annotate_tool_call_otel_metadata` (`_instrumentation.py:346-367`), informing Logfire syntax highlighting without polluting GenAI semconv payload.

## Tradeoffs

- **Lean core vs. out-of-box OTLP.** Not bundling `opentelemetry-sdk`/`exporter-otlp` keeps `pydantic-ai-slim` minimal (only `opentelemetry-api:67`) but requires explicit setup for OTLP/HTTP; Logfire makes it one line, raw OTel requires 6 lines (`docs/logfire.md:212-223`).
- **Spec purity vs. richness.** Enforcing “only spec-defined features in `_otel_*.py`” (`pydantic_ai_slim/pydantic_ai/AGENTS.md:17`) maximises backend compatibility but pushes useful extras (e.g., `code_arg_language`) into non-spec `builtin` flags or `_instruments` baggage, which some backends ignore.
- **Single JSON Schema transform walk.** Walking on every `prepare_request` handles recursive `$defs` correctly (`_json_schema.py:73-87` `recursive_refs` fallback) but adds CPU per request; profiling shows the cache in `_instrumentation.py` offsets only message serialization, not schema transforms.
- **MCP client-only posture.** Implementing only the client side avoids server/auth complexity and matches agent use-cases, but prevents exposing Pydantic AI tools as an MCP server without third-party wrapping.
- **Provider-agnostic tool definition vs. wire fidelity.** `ToolDefinition.strict=None` tri-state defers strictness to provider (`tools.py:565-581`), preserving portability but surprising users who expect deterministic validation — e.g., OpenAI enables strict when `is_strict_compatible`, while Google uses `VALIDATED` request-wide, and Anthropic/Bedrock leave it off.

## Failure Modes / Edge Cases

- **MCP SDK version skew.** `wire_name` fallback masks missing fields until SDK upgrade; a server advertising `lastModified` before SDK catches up silently drops it (`_mcp_compat.py:195` comment). Conversely, `mcp_validated_field:49` with `TypeAdapter` tolerates generic `dict[str, Any]` shapes but asserts on `expected` non-generic types, raising `AssertionError` if provider sends wrong wire type.
- **Modern (stateless) MCP sessions.** FastMCP 4 stateless sessions have no connection for server-initiated requests; `sampling_model`/`sampling_handler`/`elicitation_handler` never fire and emit `UserWarning: will never be called` (`mcp.py:1208-1228`). `log_level: warning` log handler is legacy-only; `_server_initiated_handlers` tracks dead handlers.
- **Hung transport shutdown.** `_SHUTDOWN_GRACE_SECONDS = 3` (`mcp.py:646`) bounds `__aexit__` cleanup; beyond it the session task is abandoned, risking leaked subprocess without error propagation.
- **OTel span unsampled.** `span.is_recording() is False` short-circuits message serialization but `price_calculation` and `record_metrics` still run (`_instrumentation.py:522-527`), so histograms emit even when traces are dropped — but `gen_ai.input.messages` attribute is omitted, losing debuggability.
- **Message in-place mutation.** `has_stale_message_json` (`_instrumentation.py:256-281`) detects mutated history only once per run at end, best-effort; a message mutated then pruned by a history processor escapes detection. Warning is suppressed on run error to avoid displacing the exception (`capabilities/instrumentation.py:222-224`).
- **Binary content leakage.** `redact_binary_content:141` walks `BinaryContent`/`ToolReturn`/containers but not user-defined models holding `BinaryContent` as fields — those still record full data unless `include_binary_content=False` caller manually strips.
- **Strict-to-OpenAPI fallback.** `GoogleOpenAPISchemaTransformer` raises on recursive schemas (`profiles/google.py:346`) because OpenAPI subset cannot express them; `function_schema`’s return schema falls back to `{}` with `UserWarning` on `PydanticSchemaGenerationError` (`_function_schema.py:273-281`), widening the contract silently.

## Future Considerations

- **Generic OpenAPI-to-tool importer.** Add a `tools.from_openapi(path: str | dict, operation: str) -> Tool` using `openapi-pydantic` (already in `uv.lock:4221` as transitive dep but not used for ingestion) — would answer “Can external tools be added without custom adapters?” affirmatively for REST APIs, not just MCP/LangChain.
- **Bundled OTLP SDK optional group.** Offer `pydantic-ai-slim[otlp]` = `opentelemetry-sdk + opentelemetry-exporter-otlp` extras and a `pydantic_ai.otel.configure_otlp(endpoint)` helper analogous to `logfire.configure`, reducing setup gap while preserving lean default.
- **JSON Schema caching.** Memoize `prepare_request`’s walked schema per `(tool_def.id, transformer_version)` to avoid re-walking identical schemas across retries; invalidate when `ToolDefinition.strict` changes via `PrepareTools` capability.
- **MCP server side.** Expose `Agent` via `fastmcp.server.FastMCP` as an opt-in `pydantic_ai.mcp.run_server(agent)` so tools can be consumed by other MCP clients, completing bidirectional MCP.
- **Return-schema strictness.** Promote `ToolDefinition.return_schema` from best-effort (falls back to `{}`) to validated contract when `supports_tool_return_schema=True` in profile, aligning with output-schema validation path.

## Questions / Gaps

- **No evidence found: OpenAPI import.** Grep for `openapi_pydantic` usage in `pydantic_ai_slim/pydantic_ai/**/*.py` yields no consumer beyond `uv.lock:4221`; `pydantic_ai_slim/pyproject.toml:72` does not list it as a dependency. Searched `mcp.py`, `tools.py`, `models/*.py`, `ext/*.py`.
- **OTLP exporter implementation location.** No `OTLPSpanExporter` instantiation in `pydantic_ai_slim/pydantic_ai/**` — confirmed via `grep opentelemetry.*exporter|OTLP` (only docs and shim `cli.py:646`). This confirms delegation to SDK rather than absen
ce of instrumentation.
- **Tool schema portability under concurrency.** `_instrumentation.py` notes per-run `MessageJsonCache` assumes sequential model requests (`capabilities/instrumentation.py:91-92`); concurrent requests’ schema walks and cache accesses would race — no test for parallel tool execution with shared cache examined.
- **MCP task extension provenance.** `mcp.py:95-101` loads `fastmcp_tasks.call_tool_task` only on SDK v2, but docs do not state minimum `fastmcp-tasks` version or behavior when server requires tasks but client lacks the extension — failure mode inferred as fallback to ordinary call but not asserted in `tests/test_mcp.py:1` excerpt.

---

Generated by `Dimension 19.01: Protocol Compatibility` against `pydantic-ai`.
