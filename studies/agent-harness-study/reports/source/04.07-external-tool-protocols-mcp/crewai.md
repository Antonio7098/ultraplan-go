# Source Analysis: crewai

## 04.07 External Tool Protocols and MCP Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python 3.10–3.13, Pydantic v2, asyncio/anyio, httpx, Pydantic-based tool wrappers, optional `mcp`/`mcpadapt` SDKs |
| Analyzed | 2026-08-15 |

## Summary

CrewAI ships a first-party, in-process MCP (Model Context Protocol) client that can consume any MCP-compliant server, plus a `MCPServerAdapter` (in `crewai-tools`) that wraps the third-party `mcpadapt` library. Both surface MCP tools as native CrewAI `BaseTool` instances. Three transports are supported (Stdio, HTTP/Streamable HTTP, SSE) — each as a concrete subclass of an abstract `BaseTransport`, dispatching through the official `mcp` SDK. Schemas are converted dynamically from MCP `inputSchema` JSON Schema into Pydantic models via `crewai.utilities.pydantic_schema_utils.create_model_from_schema`. Tool discovery, prompts, and resources are wired into the `crewai_event_bus` for observability, and a `tool_filter` interface supports both static allow/block lists and dynamic, context-aware filtering. There is **no OpenAPI-based tool generator or plugin manifest system**: external tool integration is exclusively MCP-driven. Stdio subprocesses receive a sanitized default environment (not the process's `os.environ`) and are intercepted by an `_env_filter_hook` extension point for org-policy enforcement. Failures are isolated per-invocation by spinning up a fresh `MCPClient`+transport pair on every tool call (`MCPNativeTool._run_async`), avoiding the anyio cancel-scope cross-talk that earlier shared-client designs suffered.

## Rating

**Rating: 8 (Clear model with tests, explicit interfaces, and operational safeguards).**

Rationale:
- Clear, explicit interfaces: `MCPServerStdio`, `MCPServerHTTP`, `MCPServerSSE` Pydantic models (`crewai/src/crewai/mcp/config.py:12-120`), abstract `BaseTransport` with four concrete subclasses (`crewai/src/crewai/mcp/transports/base.py:25`).
- Three first-party transports wrap the official `mcp` SDK with consistent timeout, retry, cache, and event semantics (`crewai/src/crewai/mcp/client.py:43-51`, `transports/{stdio,http,sse}.py`).
- Schema conversion is deliberate and tested: `MCPToolResolver._json_schema_to_pydantic` uses `create_model_from_schema(enrich_descriptions=True)` (`crewai/src/crewai/mcp/tool_resolver.py:630-639`); dedicated end-to-end MCP schema tests live at `crewai/tests/utilities/test_pydantic_schema_utils.py:760`.
- Operational safeguards: per-invocation fresh client to avoid anyio cancel-scope errors (`crewai/src/crewai/tools/mcp_native_tool.py:101-119`), stdio env allowlist (`crewai/src/crewai/mcp/transports/stdio.py:86-96`), env-filter hook for enterprise policy (`crewai/src/crewai/mcp/transports/stdio.py:13-20`), timeout protection with explicit timeouts on connect/discovery/execute (`crewai/src/crewai/mcp/client.py:43-46`).
- Trust boundary fails to be closed by the framework itself: external MCP servers are trusted by default — the security model is "warn and document, then trust the operator", see `docs/edge/en/mcp/security.mdx:10-48`. No signature verification, no allow-list of transports, no rate limit enforcement client-side beyond retries.
- Portability is excellent across transports, but **no OpenAPI-to-tool generator** exists, narrowing the source-of-truth ecosystem to MCP-only.
- Score 8 (not 9–10) because: no client-side verification/cert-pinning config (handlers pass whatever `headers=...` the caller provides), no isolation/sandboxing of remote servers beyond per-connection transport caps, and prompt-injection via tool metadata is documented as a known risk rather than mitigated in-band (`docs/edge/en/mcp/security.mdx:38-51`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP client + transports package | `MCPClient` with `BaseTransport` ABC + `TransportType` enum (`STDIO`, `HTTP`, `STREAMABLE_HTTP`, `SSE`) | `crewai/src/crewai/mcp/client.py:54`, `crewai/src/crewai/mcp/transports/base.py:16-25` |
| Transport: Stdio | `StdioTransport` wraps `mcp.client.stdio.stdio_client`; merges `get_default_environment()` + user `env=` and runs `_env_filter_hook` | `crewai/src/crewai/mcp/transports/stdio.py:69-110`, `crewai/src/crewai/mcp/transports/stdio.py:13-20` |
| Transport: HTTP / Streamable HTTP | `HTTPTransport` wraps `mcp.client.streamable_http.streamablehttp_client` with `terminate_on_close=True`; respects a 30s transport-entry timeout | `crewai/src/crewai/mcp/transports/http.py:61-98` |
| Transport: SSE | `SSETransport` wraps `mcp.client.sse.sse_client`; explicit test that SSE `connect()` does **not** pass `terminate_on_close` | `crewai/src/crewai/mcp/transports/sse.py:50-83`, `crewai/tests/mcp/test_sse_transport.py:1-22` |
| Config models | Pydantic `MCPServerStdio`/`MCPServerHTTP`/`MCPServerSSE` with `tool_filter` and `cache_tools_list`; union type `MCPServerConfig` | `crewai/src/crewai/mcp/config.py:12-123` |
| Lazy import for MCP SDK | `crewai.mcp.__init__` lazy-loads `MCPClient`, `MCPToolResolver`, `BaseTransport`, `TransportType` to avoid ~400ms cold-start when only filters/configs are used | `crewai/src/crewai/mcp/__init__.py:36-51` |
| Reference resolution | Three flavours accepted by `MCPToolResolver.resolve`: native configs, `https://` URLs, and AMP slugs (`notion`, `notion#search`, legacy `crewai-amp:` prefix) | `crewai/src/crewai/mcp/tool_resolver.py:68-89`, `crewai/src/crewai/mcp/tool_resolver.py:107-116` |
| Validation gate | `BaseAgent.validate_mcps` permits only `https://...`, slug regex `^(?:crewai-amp:)?[a-zA-Z0-9][a-zA-Z0-9_-]*(?:#[\w-]+)?$`, or `MCPServerConfig` | `crewai/src/crewai/agents/agent_builder/base_agent.py:557-590`, `crewai/src/crewai/agents/agent_builder/base_agent.py:173-175` |
| Schema conversion | `_json_schema_to_pydantic` calls `create_model_from_schema` (handles `$ref`, `anyOf/oneOf`, formats); same function backs A2A, MCP, and other dynamic tools | `crewai/src/crewai/mcp/tool_resolver.py:630-639`, `crewai/src/crewai/utilities/pydantic_schema_utils.py:799-852` |
| MCP SDK dependency | `crewai` lists `mcp~=1.26.0` as a hard dep; `crewai-tools` exposes it via the `mcp` extra (`mcp>=1.6.0`, `mcpadapt>=0.1.9`) | `crewai/pyproject.toml:43`, `crewai-tools/pyproject.toml:102-105` |
| PlusAPI for AMP | `MCPToolResolver._fetch_amp_mcp_configs` calls `PlusAPI.get_mcp_configs` to bulk-fetch server configs from CrewAI+ (`crewai-oauth` proxy) | `crewai/src/crewai/mcp/tool_resolver.py:182-217`, `crewai-core/src/crewai_core/plus_api.py:474-481` |
| Per-invocation isolation | `MCPNativeTool._run_async` calls `_client_factory()` to spin a fresh `MCPClient`+transport per call so parallel calls never share state | `crewai/src/crewai/tools/mcp_native_tool.py:101-119` |
| Tool filtering | `ToolFilter` = callable on tool (and optionally `ToolFilterContext`); `StaticToolFilter` enforces allow/block precedence; constructors `create_static_tool_filter` / `create_dynamic_tool_filter` | `crewai/src/crewai/mcp/filters.py:32-163` |
| Filter wiring | `MCPToolResolver._resolve_native` applies the configured `tool_filter` to each discovered tool | `crewai/src/crewai/mcp/tool_resolver.py:383-402` |
| Resilience | `MCPClient._retry_operation` uses exponential backoff, classifies auth/not-found errors as non-retryable | `crewai/src/crewai/mcp/client.py:663-714` |
| Schema caching | `MCPClient.list_tools` and `MCPToolResolver._get_mcp_tool_schemas` cache tool list keyed by server URL with 5-min TTL | `crewai/src/crewai/mcp/client.py:50-51,374-404`, `crewai/src/crewai/mcp/tool_resolver.py:41-42,494-519` |
| Observability | Distinct events: `MCPConnectionStarted`, `MCPConnectionCompleted`, `MCPConnectionFailed`, `MCPToolExecutionStarted`, `MCPToolExecutionCompleted`, `MCPToolExecutionFailed`, `MCPConfigFetchFailed` | `crewai/src/crewai/events/types/mcp_events.py:24-98` |
| Checkpoint tracing | Same events are listed in `checkpoint_event_types` for state restore/observability | `crewai/src/crewai/state/checkpoint_config.py:82-88` |
| Console formatting | Dedicated handlers `handle_mcp_*` print transport/URL/timeout panels | `crewai/src/crewai/events/utils/console_formatter.py:1539-1724` |
| Error semantic | `MCPClient.connect` distinguishes `authentication` vs `network` vs `timeout`/`cancelled`/`import_error` failure types | `crewai/src/crewai/mcp/client.py:250-321` |
| CrewBase integration | `@CrewBase` injects `mcp_server_params`, `mcp_connect_timeout`, `get_mcp_tools()`, `close_mcp_server` and auto-cleans via after-kickoff callback | `crewai/src/crewai/project/crew_base.py:159-180`, `crewai/src/crewai/project/crew_base.py:290-334`, `crewai/src/crewai/project/wrappers.py:74-129` |
| AMP config validation | `_resolve_mcp_python_refs` / `_resolve_mcp_config_python_refs` allow inline python refs for `tool_filter` in JSON project files | `crewai/src/crewai/project/json_loader.py:1613-1665` |
| Cross-suite test | `test_internal_crew_with_mcp` exercises `MCPServerAdapter` via `@CrewBase` with patched adapter | `crewai/tests/test_project.py:92-106,417-433` |
| Stdio env safety | `test_ambient_env_does_not_leak_to_server` proves ambient env (e.g. `COMPANY_SECRET`, `AWS_*`) is filtered out; `_env_filter_hook` extension allows further stripping | `crewai/tests/mcp/test_stdio_transport.py:11-52,86-124` |
| Parallel safety tests | `test_parallel_mcp_tool_execution_same_tool`, `test_each_invocation_gets_fresh_client` verify per-call client isolation | `crewai/tests/mcp/test_mcp_config.py:183-301` |
| Adapter (mcpadapt) integration | `MCPServerAdapter` wraps `MCPAdapt` with a custom `CrewAIToolAdapter`; auto-installs `mcp crewai-tools'[mcp]'` if missing | `crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:31-189` |
| Adapter test matrix | Tests cover stdio and SSE end-to-end (real `FastMCP("Echo Server")` spawned), filtered tools, missing tools, `connect_timeout` parameter forwarding | `crewai-tools/tests/adapters/mcp_adapter_test.py:77-237` |
| Public docs | Edge docs `mcp/overview.mdx`, `mcp/stdio.mdx`, `mcp/sse.mdx`, `mcp/streamable-http.mdx`, `mcp/multiple-servers.mdx`, `mcp/dsl-integration.mdx`, `mcp/security.mdx` | `crewai/docs/edge/en/mcp/overview.mdx:1-724`, `crewai/docs/edge/en/mcp/security.mdx:1-167` |
| Schema-conversion unit tests | `TestEndToEndMCPSchema` exercises nested objects, `$ref` arrays, `oneOf`, enum, format, additionalProperties via MCP-style schema fixture | `crewai/tests/utilities/test_pydantic_schema_utils.py:760-860` |
| Telemetry audit string | `mcp:connection` is part of the action dimension surfaced in telemetry | `crewai/src/crewai/telemetry/telemetry.py:1049` |

## Answers to Dimension Questions

1. **Can tools live outside the process?**
   Yes, all three primary transports do exactly that:
   - `StdioTransport` spawns a local subprocess and pipes JSON-RPC over `stdin`/`stdout` (`crewai/src/crewai/mcp/transports/stdio.py:69-122`).
   - `HTTPTransport` opens streamable-HTTP sessions to remote URLs (`crewai/src/crewai/mcp/transports/http.py:61-97`).
   - `SSETransport` opens long-lived Server-Sent Events sessions (`crewai/src/crewai/mcp/transports/sse.py:50-83`).
   Indirectly, `MCPServerAdapter` (in `crewai-tools`) uses the third-party `mcpadapt` library which delegates to the same `mcp` SDK plus `asyncio`/`anyio` task groups (`crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:31-89`).
2. **Are external tools trusted by default?**
   **Yes — external tools are trusted by default.** CrewAI ships no allow-list, signature verification, or capability check beyond the schema each server announces. The `BaseAgent.validate_mcps` validator only checks syntactic shape (URL scheme or slug regex); it does not interrogate the server's identity (`crewai/src/crewai/agents/agent_builder/base_agent.py:557-590`). The docs make this explicit: *"Always ensure that you trust an MCP Server before using it"* and outline prompt-injection via tool metadata as an in-band risk that the framework cannot mitigate (`docs/edge/en/mcp/security.mdx:10-51`). Trust-at-construction is the implicit model; the only thing the framework enforces is at the boundary it controls: the agent asks for specific tool name via the `#tool` suffix and `tool_filter` allow/block list (`crewai/src/crewai/mcp/tool_resolver.py:163-180`, `crewai/src/crewai/mcp/filters.py:38-89`).
3. **How are schemas imported?**
   Two layers of conversion:
   - **Discovery**: `MCPClient.list_tools` calls `session.list_tools()` and copies `tool.name`, `tool.description`, and `tool.inputSchema` into plain `dict[str,Any]` after `sanitize_tool_name` (`crewai/src/crewai/mcp/client.py:406-421`).
   - **Wrapping**: `MCPToolResolver._resolve_native` converts each `inputSchema` into a Pydantic `BaseModel` via `create_model_from_schema(..., enrich_descriptions=True)` (`crewai/src/crewai/mcp/tool_resolver.py:425-452`, `crewai/src/crewai/utilities/pydantic_schema_utils.py:799-852`). The resulting class is attached as `args_schema` on `MCPNativeTool`, which propagates `enforce` validation into tool invocation (`crewai/src/crewai/tools/mcp_native_tool.py:45-57`).
   - End-to-end MCP JSON-Schema → Pydantic coverage is locked down by `TestEndToEndMCPSchema` at `crewai/tests/utilities/test_pydantic_schema_utils.py:760-860`.
4. **How are failures isolated?**
   - **Per-tool-call isolation**: `MCPNativeTool._run_async` creates a fresh `MCPClient`+transport for every invocation via a `_client_factory` closure (`crewai/src/crewai/tools/mcp_native_tool.py:101-119`). This is the explicit answer to anyio cancel-scope crashes seen with shared clients; it is exercised by `test_each_invocation_gets_fresh_client` and `test_parallel_mcp_tool_execution_{same,different}_tool` (`crewai/tests/mcp/test_mcp_config.py:183-301`).
   - **Per-server failure isolation**: `MCPToolResolver.resolve` swallows per-server exceptions and logs warnings, returning `[]` for the affected server while keeping the agent alive (`crewai/src/crewai/mcp/tool_resolver.py:155-180,272-276`). Failed configs surface as `MCPConfigFetchFailedEvent` rather than raising (`crewai/src/crewai/mcp/tool_resolver.py:139-164`).
   - **Transport cleanup**: Each transport's `disconnect()` is best-effort and idempotent; stdio uses SIGTERM-then-SIGKILL with a 5-second grace (`crewai/src/crewai/mcp/transports/stdio.py:124-156`); HTTP swallows known cancel-scope noise while propagating real errors (`crewai/src/crewai/mcp/transports/http.py:109-159`).
   - **Disconnect cleanup at agent level**: `Agent._cleanup_mcp_clients` runs after task execution to tear down resolver-held connections (`crewai/src/crewai/agent/core.py:1199-1203`).
   - **Authentication vs transient errors**: `MCPClient._retry_operation` distinguishes auth/not-found (raise immediately) from network/timeout (retry with exponential backoff) (`crewai/src/crewai/mcp/client.py:663-714`).
5. **Can the same tool work across clients?**
   The `BaseTransport` ABC (`crewai/src/crewai/mcp/transports/base.py:25-114`) is transport-agnostic, and `MCPClient` is generic — meaning a tool wrapper created via `MCPToolResolver` is not bound to a particular transport at code level. Practically, portability depends on the schema being JSON-Schema-compatible and the tool name sanitizing cleanly via `sanitize_tool_name` to OpenAI/Bedrock constraints (`crewai/src/crewai/utilities/string_utils.py:26-54`). The `MCPToolResolver._create_transport` factory intentionally returns a fresh transport on each call so any combination of clients can construct clients against any MCP server config (`crewai/src/crewai/mcp/tool_resolver.py:278-311`). `MCPServerAdapter` is even more portable: it takes a generic `StdioServerParameters | dict[str, Any]` and works with any MCP-compliant server including third-party integrations (`crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:97-189`, `docs/edge/en/mcp/multiple-servers.mdx:9-65`).

## Architectural Decisions

1. **First-party SDK, not a wrapper.** CrewAI built its own `MCPClient` on top of the official `mcp` SDK transport primitives (`crewai/src/crewai/mcp/client.py:168-185`, `crewai/src/crewai/mcp/transports/{stdio,http,sse}.py:69-110`) rather than relying solely on `mcpadapt` (used by the legacy `MCPServerAdapter` at `crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:179-181`). The choice yields tighter control over timeouts, retries, observability, and connection lifecycle at the cost of duplicated maintenance.
2. **Fresh-client-per-call instead of pooled client.** A repository-wide regression taught the project that shared `MCPClient` instances across concurrent invocations produce anyio cancel-scope errors. The fix is structural: per-call client factories in `MCPNativeTool` and `MCPToolResolver` (`crewai/src/crewai/tools/mcp_native_tool.py:26-44`, `crewai/src/crewai/mcp/tool_resolver.py:411-416`). Tests prove this out (`crewai/tests/mcp/test_mcp_config.py:214-301`).
3. **Lazy SDK import.** Cold-start was a problem (the `mcp` SDK pulls in ~400ms of imports); `crewai.mcp.__init__` lazy-imports heavy classes, while keeping config/filter types eagerly available (`crewai/src/crewai/mcp/__init__.py:36-67`).
4. **Tool-name sanitization at the edge.** All MCP tool names flow through `sanitize_tool_name` to satisfy OpenAI/Bedrock 64-char `a-z0-9_` rules *before* ever reaching the LLM context (`crewai/src/crewai/mcp/client.py:415`, `crewai/src/crewai/utilities/string_utils.py:26-54`).
5. **Event-bus instrumentation by default.** Every connect/list/call surfaces as a typed `MCP*Event` so external observers, the console formatter, and checkpoint state restoration share the same wiring (`crewai/src/crewai/events/types/mcp_events.py:24-98`, `crewai/src/crewai/state/checkpoint_config.py:82-88`, `crewai/src/crewai/events/utils/console_formatter.py:1539-1724`).
6. **Schema conversion delegated to a dedicated utility.** `create_model_from_schema` was extracted so the MCP layer, A2A layer, and other dynamic tools share one battle-tested JSON-Schema-to-Pydantic pipeline, with a 100-line end-to-end test specifically for MCP-shaped schemas (`crewai/src/crewai/utilities/pydantic_schema_utils.py:799-852`, `crewai/tests/utilities/test_pydantic_schema_utils.py:760-860`).
7. **Trust the operator, not the server.** No in-band authentication, pinning, or signing — CrewAI accepts whatever headers/env the developer supplies and relies on the operator to vet the server. The docs and tests formalize this stance but do not enforce it (`docs/edge/en/mcp/security.mdx:10-51`, `crewai/src/crewai/mcp/config.py:39-46,72-79,109-114`).
8. **AMP catalog as a separate pathway.** CrewAI+ acts as a managed OAuth proxy: agents can reference MCP integrations by short slug (e.g., `"snowflake"`) and let the platform broker credentials (`crewai/src/crewai/mcp/tool_resolver.py:118-180,182-217`, `crewai-core/src/crewai_core/plus_api.py:474-481`). This sits alongside, not in place of, direct transport configs.
9. **Stdio env hygiene.** Subprocesses get only the SDK's allowlisted `get_default_environment()` plus an explicit user `env=`, with an extension hook (`_env_filter_hook`) for enterprise policy — preventing ambient-leak regressions on a subprocess-by-subprocess basis (`crewai/src/crewai/mcp/transports/stdio.py:13-20,83-96`, `crewai/tests/mcp/test_stdio_transport.py:11-52,86-124`).
10. **Two layers of MCP integration coexist.** The new first-party DSL (`mcps=[...]` on `Agent`) and the older `MCPServerAdapter` context manager (`@CrewBase`'s `get_mcp_tools`) serve different ergonomics; they are deliberately not unified in code path (e.g., `crewai/src/crewai/agent/core.py:1188-1197` vs `crewai/src/crewai/project/crew_base.py:311-334`).

## Notable Patterns

- **Dual-layer resolution**: `MCPToolResolver.resolve` partitions inputs by syntactic shape (`isinstance(str)` + `startswith("https://")`, slug match, otherwise `MCPServerConfig`), then dispatches to three specialized resolvers (`_resolve_external`, `_resolve_amp`, `_resolve_native`) (`crewai/src/crewai/mcp/tool_resolver.py:68-89`).
- **AMP deduplication + caching**: Multiple slug refs share one resolved client; per-ref tool filtering happens after the shared discovery (`crewai/src/crewai/mcp/tool_resolver.py:118-180`).
- **Deterministic name prefixing**: `MCPToolResolver._extract_server_name` produces `domain_path` style prefixes so different servers cannot collide on the agent's tool surface (`crewai/src/crewai/mcp/tool_resolver.py:486-492`).
- **Schema TTL cache**: both the MCP client and the resolver cache tool lists for 5 minutes, keyed by transport+url (`crewai/src/crewai/mcp/client.py:50-51,716-735`, `crewai/src/crewai/mcp/tool_resolver.py:41-42,494-519`).
- **Hybrid sync/async execution**: `MCPNativeTool._run` falls back to a `ThreadPoolExecutor` when an event loop is already running so that CrewAI crews (sync) and Flows (async) both work without rewriting tool code (`crewai/src/crewai/tools/mcp_native_tool.py:73-99`).
- **Discriminated failure typing**: error streams map to `error_type="authentication"|"network"|"timeout"|"tool_error"`, allowing downstream UI/telemetry to surface them without free-form string-matching (`crewai/src/crewai/events/types/mcp_events.py:46-85`, `crewai/src/crewai/mcp/client.py:288-322`).
- **Token redaction by transport** (implicit): the `_clean_tool_arguments` helper strips `None` values and rewrites string-keyed `sources=[...]` into structured form before sending to remote servers (`crewai/src/crewai/mcp/client.py:520-567`).
- **Auto-install affordance**: `MCPServerAdapter.__init__` offers a one-click install of `mcp crewai-tools'[mcp]'` if the package is missing (`crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:159-175`).

## Tradeoffs

- **Trust delegation vs safety**: configurability wins; sandboxing loses. By accepting any URL the user pastes and any headers they choose, CrewAI offloads server provenance to the developer. There is no MCP-level signature, allow-list, or tool-level capability check inside `MCPServerStdio/HTTP/SSE` (`crewai/src/crewai/mcp/config.py:31-121`). Prompt-injection through tool names/descriptions is documented as unavoidable in the framework (`docs/edge/en/mcp/security.mdx:38-51`).
- **Fresh-client-per-call vs latency**: every tool invocation now opens a new transport and runs `session.initialize()`. For high-frequency remote tools this negates HTTP keep-alive benefits. Caching the discovery schema helps, but per-call connection overhead remains (`crewai/src/crewai/mcp/tool_resolver.py:411-452`, `crewai/src/crewai/tools/mcp_native_tool.py:113-119`).
- **Two MCP stacks**: keeping the new DSL resolver and the older `MCPServerAdapter` parallel lets users migrate gradually but doubles the maintenance surface and inconsistent runtime semantics (e.g., the DSL resolver owns client lifecycle; the adapter ends on `__exit__`) (`crewai/src/crewai/agent/core.py:1188-1203` vs `crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:196-234`).
- **Privacy of error strings**: failures are forwarded to the event bus with raw error messages (truncated to 500 chars in console formatting but unbounded elsewhere). For local stdio servers this is fine; for remote services it can leak sensitive stack traces into telemetry (`crewai/src/crewai/events/types/mcp_events.py:76-85`, `crewai/src/crewai/events/utils/console_formatter.py:1719-1722`).
- **Schema sanitization vs strict typing**: `_json_schema_to_pydantic` calls `enrich_descriptions=True` to embed constraints in tool descriptions, beneficial for LLM guidance but redundant with the typed model, and may bloat LLM context (`crewai/src/crewai/mcp/tool_resolver.py:630-639`).
- **`_env_filter_hook` opt-in**: the hook is `None` by default and only set by extensions, so out-of-the-box the stdio path will not enforce org-policy credential redaction. Teams that need it must wire it themselves (`crewai/src/crewai/mcp/transports/stdio.py:13-20,86-96`).
- **PlusAPI coupling**: AMP slugs depend on `get_platform_integration_token()` (CrewAI+ OAuth). If the user lacks a token, the resolver silently degrades and returns no tools rather than raising (`crewai/src/crewai/mcp/tool_resolver.py:182-217,344-350`).
- **Default tool filter**: when `tool_filter` is `None`, every tool the server announces becomes available to the agent — "least surprise" but "maximum risk" for sensitive servers (`crewai/src/crewai/mcp/tool_resolver.py:383-402`).

## Failure Modes / Edge Cases

- **Cancel-scope cross-talk in shared clients**: explicitly tested and solved by `MCPNativeTool`'s per-call factory, but the legacy `MCPToolWrapper` (`crewai/src/crewai/tools/mcp_tool_wrapper.py:69-95`) and `_resolve_external` path still construct a fresh client manually and could regress if reused.
- **MCP SDK not installed**: a single `ImportError` is caught at connect/discovery/call sites, with the diagnostic message `"MCP library not available. Please install with: pip install mcp"` (`crewai/src/crewai/mcp/client.py:236-249`, `crewai/src/crewai/mcp/tool_resolver.py:565-570`, `crewai/src/crewai/mcp/transports/{stdio,http,sse}.py`); downstream callers should know how to surface this.
- **HTTP 401 or "unauthorized" during handshake**: the client specifically promotes that error to `error_type="authentication"` and re-raises wrapped, preventing silent retries (`crewai/src/crewai/mcp/client.py:193-215,288-310,694-699`).
- **Event-loop reuse in Flows**: the resolver detects a running loop via `asyncio.get_running_loop()` and switches to a `ThreadPoolExecutor` so crews can run inside Flows without deadlocking (`crewai/src/crewai/mcp/tool_resolver.py:356-381`, `crewai/src/crewai/tools/mcp_native_tool.py:83-94`).
- **AMP missing slug**: emits `MCPConfigFetchFailedEvent` with `error_type="not_connected"` and continues with the rest of the MCP list (`crewai/src/crewai/mcp/tool_resolver.py:139-148`).
- **Empty tool list post-filter**: logs warning `"No tools discovered from MCP server: ..."` instead of returning an empty `BaseTool` silently (`crewai/src/crewai/mcp/tool_resolver.py:404-409`).
- **`#tool` token after sanitisation mismatch**: tests assert both `notion#search` and `notion#get-page` map to the sanitized server-keyed tool suffix (`crewai/tests/mcp/test_amp_mcp.py:272-318,439-456`).
- **Circular JSON Schemas**: documented as a fix in changelogs; handled by `create_model_from_schema`'s `_strip_keys_recursive` and `resolve_refs` (`crewai/src/crewai/utilities/pydantic_schema_utils.py:594-602`, `crewai/tests/utilities/test_pydantic_schema_utils.py:760-860`).
- **Stdio env leakage from outer process**: explicit guard test (`crewai/tests/mcp/test_stdio_transport.py:11-52`); verified by `test_user_env_overrides_default_environment` and `test_env_filter_hook_runs_after_merge` (`crewai/tests/mcp/test_stdio_transport.py:54-124`).
- **DNS-rebinding warning**: documented for SSE but not enforced at the client; mitigation lives on the server side (`docs/edge/en/mcp/security.mdx:89-105,140-150`, `crewai/src/crewai/mcp/transports/sse.py:50-83`).
- **Cancelled event loop on disconnect**: HTTP and SSE transports explicitly suppress `BaseExceptionGroup`/`RuntimeError("cancel scope…")` to avoid leaking asyncio.run() teardown noise (`crewai/src/crewai/mcp/transports/http.py:126-150`).
- **Schema cache poisoning**: `_mcp_schema_cache` is a module-level `dict` not namespaced by agent/run, so a tool-list change on a remote server can persist for 5 minutes across agents (`crewai/src/crewai/mcp/client.py:50-51`, `crewai/src/crewai/mcp/tool_resolver.py:41-42`).

## Future Considerations

- **No native MCP server side**. CrewAI is an MCP **client**. Serving CrewAI agents *as* MCP servers would close a portability loop, but no `FastMCP`-style server adapter was found in this source. The a2a subsystem is the only "expose yourself" path and it's a different protocol (`crewai/src/crewai/a2a/extensions/a2ui/schema/v0_9/server_capabilities.json:5`; no `crewai_tools/adapters/mcp_server_*` exists).
- **OpenAPI import path absent**. Despite a healthy `pydantic_schema_utils` engine, there is no tool that ingests an OpenAPI document and emits `BaseTool`s. Adding an `OpenAPITool` or `MCPServerAdapter` style "OpenAPI-to-tools" generator would broaden the external-tool surface to REST ecosystems; today the only path is via an external MCP server wrapping the OpenAPI doc.
- **Trust on first use**. There is room for an `Origin`/server-cert-pinning helper, an mTLS config pass-through for `HTTPTransport`, or a CrewAI+-verified registry check before accepting a new `MCPServerConfig`/`https://` URL.
- **Deprecation of `MCPServerAdapter`**. With the new DSL being "recommended" (`docs/edge/en/mcp/overview.mdx:12-16`), the older `crewai-tools` adapter likely becomes a maintenance liability; explicit deprecation messaging would clarify the path.
- **AMP schema cache eviction**. The current cache lacks an explicit invalidation API; a `Cache-Control`-aware cache or per-run scoping would make cached schemas safer for multi-tenant usage.
- **Tool execution observability for `call_tool`**. Events exist, but the payload contains raw `tool_args` which may include credentials; a redaction policy in the event-bus layer would help.
- **Hierarchical trust**. The security model treats all servers equally; today operators cannot say "servers tagged `internal-trusted` may skip the prompt-injection `Warning`" or attach per-server capability grants.

## Questions / Gaps

- **Per-server capability grants**: how would the framework surface a server's declared `capabilities` (MCP spec) and gate tool *invocation* (not just discovery) on it? No evidence found in this source — only the schema-implied capability.
- **Streaming tool results**: `call_tool` currently extracts only the first `TextContent` (`crewai/src/crewai/tools/mcp_native_tool.py:121-131`, `crewai/src/crewai/mcp/client.py:580-588`); multi-modal or chunked outputs are dropped or stringified.
- **Prompts/resources as first-class tools**: `MCPClient.list_prompts`/`get_prompt` are implemented (`crewai/src/crewai/mcp/client.py:590-661`) but no evidence was found that `BaseTool` or `Agent` exposes those primitive flows to the agent runtime. The `CrewBase` adapter likewise surfaces only `tools` (`docs/edge/en/mcp/overview.mdx:720-724`).
- **OAuth 2.1 / MCP Authorization**: there is no client-side MCP-Authorization implementation in this source. Bearer tokens flow only through `headers=` (`crewai/src/crewai/mcp/config.py:72-79`). The infrastructure for OAuth providers exists separately at `crewai/src/crewai/auth/providers/*` but is not wired into MCP.
- **Per-tenant `tool_filter`**: `ToolFilterContext` exposes `agent` and `server_name` but no run-id or tenant-id, so dynamic filters can't easily scope by session (would need to be threaded in `run_context` by the caller — `crewai/src/crewai/mcp/filters.py:17-29`).
- **Schema cache invalidation hooks**: `_mcp_schema_cache` is module-level and unbounded; no API was found to bust/clear it on server-side schema rotation.
- **Origin validation for SSE clients**: only server-side guidance exists (`docs/edge/en/mcp/sse.mdx:140-150`). No `allow_origins=` option was found in `SSETransport` (`crewai/src/crewai/mcp/transports/sse.py:27-43`) — it's a client, not a server, so this is partly out of scope but bears stating.
- **Stdio `cwd` / working dir isolation**: `StdioTransport.__init__` accepts `command`/`args`/`env` but no `cwd` argument (`crewai/src/crewai/mcp/transports/stdio.py:42-62`), so subprocesses inherit the agent's `cwd`.

---

Generated by `04.07-external-tool-protocols-and-mcp-integration` against `crewai`.
