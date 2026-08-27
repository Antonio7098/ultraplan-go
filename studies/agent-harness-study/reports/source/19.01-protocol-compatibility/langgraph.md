# Source Analysis: langgraph

## 19.01 Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph`, `libs/prebuilt`, `libs/sdk-py`, `libs/cli`) / Pydantic, langchain-core, LangSmith |
| Analyzed | 2026-08-26 |

## Summary

LangGraph is a stateful orchestration framework, not a protocol gateway. Its strongest protocol surface is **JSON Schema** generation for graph input/output/context via Pydantic/TypeAdapter, with explicit `get_input_jsonschema`/`get_output_jsonschema` APIs and extensive tests. Tool schemas are portable across providers through `langchain_core.tools.BaseTool` (`bind_tools`/`with_structured_output`) abstraction, handling OpenAI (`type=="function"`) and Anthropic (`name`) shapes. **MCP** is supported only as a deployment concern: the LangGraph API server can expose a deployment as an MCP server (`/mcp` routes, `disable_mcp` flag) and docs show conditional `connect_mcp` patterns, but the core library contains no MCP client/server implementation — integration relies on external `langchain-mcp-adapters`. **OpenTelemetry/OTLP**: no exporter; tracing is LangSmith-native (`distributed_tracing` header propagation in `RemoteGraph`, `langsmith.tracing_context`), with OTEL only via third-party `opentelemetry-instrumentation-langchain` monkey-patch. **OpenAPI**: server emits/exposes its own OpenAPI (`/openapi.json`, `SecurityConfig` for auth) but provides no OpenAPI→tool importer; tool ingestion is code-first.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, or fragile**

Rationale: JSON Schema and provider-agnostic tool abstraction are mature and tested (would be 7-8 alone). However the dimension-weighted protocols are partial: MCP is server-only with no native client/resources/prompts implementation in-repo, OTLP has no exporter, and OpenAPI has no importer. External tools can be added without custom adapters only via the `BaseTool`/`ToolNode` abstraction, not via declarative protocol import. Operational safeguards exist for schema generation but not for cross-protocol translation.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| JSON Schema — graph-level generation | `Pregel.get_input_jsonschema()` delegates to `schema.model_json_schema()`; `get_context_jsonschema()` branches on `BaseModel` vs `TypedDict`/`dataclass` via `TypeAdapter(...).json_schema()` | `libs/langgraph/langgraph/pregel/main.py:1028-1032`, `libs/langgraph/langgraph/pregel/main.py:994-1005` |
| JSON Schema — compiled graph wrapper | `CompiledStateGraph.get_input_jsonschema` / `get_output_jsonschema` call shared `_get_json_schema` helper that creates a dynamic Pydantic model from channel `UpdateType`/`ValueType` | `libs/langgraph/langgraph/graph/state.py:1424-1442`, `libs/langgraph/langgraph/graph/state.py:1944-1978` |
| JSON Schema — Pydantic internals | `create_model` and `_create_root_model` override `model_json_schema` to inject `title`, handle `RootModel`, and remap reserved field names; uses `pydantic.json_schema.GenerateJsonSchema` | `libs/langgraph/langgraph/_internal/_pydantic.py:25-29`, `libs/langgraph/langgraph/_internal/_pydantic.py:81-103`, `libs/langgraph/langgraph/_internal/_pydantic.py:181-249` |
| JSON Schema — tests | Assertions on `graph.get_input_jsonschema()` required/optional properties and `model_json_schema()` for `input_schema`/`output_schema`/`context_schema` | `libs/langgraph/tests/test_state.py:167-184`, `libs/langgraph/tests/test_state.py:231-247`, `libs/langgraph/tests/test_pregel.py:251-268`, `libs/langgraph/tests/test_pregel.py:464-560` |
| Tool schema portability | `ToolNode` accepts `Sequence[BaseTool|Callable]` and normalizes via `create_tool`; `_should_bind_tools` detects already-bound `RunnableBinding` and validates OpenAI vs Anthropic shapes (`bound_tool.get("type")=="function"` vs `bound_tool.get("name")`) | `libs/prebuilt/langgraph/prebuilt/tool_node.py:743-786`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-218` |
| Tool schema — portable structured output | `response_format` supports JSON Schema / TypedDict / Pydantic / tuple `(prompt, schema)` and delegates to `model.with_structured_output(schema)` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:292-295`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:744-785` |
| Tool schema — provider-agnostic tool defs | `create_react_agent` `tools` param allows `dict` builtin tools (MCP) alongside `BaseTool`; `FakeToolCallingModel.bind_tools` tests show mixed binding | `libs/prebuilt/tests/test_react_agent.py:311-327`, `libs/langgraph/tests/fake_chat.py:19` |
| MCP — server exposure | `HttpConfig.disable_mcp: bool` disables `/mcp` routes, enabling deployment-as-MCP-server (also persisted to `libs/cli/schemas/schema.json:1025`) | `libs/cli/langgraph_cli/schemas.py:471-473`, `libs/cli/schemas/schema.json:1025-1028` |
| MCP — execution-runtime pattern | Docs/pattern for conditionally connecting MCP only during `threads.create_run` via `ServerRuntime.execution_runtime`, with example `await connect_mcp(ert.context.mcp_endpoint)` | `libs/sdk-py/langgraph_sdk/runtime.py:87-89`, `libs/sdk-py/langgraph_sdk/runtime.py:106-122`, `libs/sdk-py/langgraph_sdk/runtime.py:194-220` |
| MCP — dict tool passthrough | Test includes MCP tool as dict `{type:"mcp", server_label, server_url, allowed_tools, require_approval}` passed to `bind_tools` alongside regular tools | `libs/prebuilt/tests/test_react_agent.py:315-326` |
| MCP — absence of native client | `grep -r mcp libs --include="*.py"` returns only runtime docs, CLI `disable_mcp`, and test dict; no `mcp/client`, `resources`, `prompts` implementation in `libs/langgraph` or `libs/prebuilt` | `libs/sdk-py/langgraph_sdk/runtime.py:55`, `libs/cli/langgraph_cli/schemas.py:471` (negative evidence) |
| OTLP / OpenTelemetry — no exporter | No `opentelemetry`, `OTLP`, `TracerProvider`, `BatchSpanProcessor` imports in `libs/langgraph`; only LangSmith `distributed_tracing` bool and `_merge_tracing_headers` using `ls.get_current_run_tree().to_headers()` | `libs/langgraph/langgraph/pregel/remote.py:144-171`, `libs/langgraph/langgraph/pregel/remote.py:1297-1308` |
| OTLP — implicit OTEL via instrumentation | Comment acknowledges OTEL via monkey-patch: `opentelemetry-instrumentation-langchain monkey-patch ...` plus `langsmith.tracing_context` in tests | `libs/langgraph/tests/test_graph_callbacks.py:284`, `libs/langgraph/tests/test_utils.py:135-154` |
| OpenAPI — server spec exposure | `SecurityConfig` (securitySchemes/security/paths) and `AuthConfig.openapi` fed into deployment OpenAPI spec; `HttpConfig.disable_meta` controls `/openapi.json`/`/info`/`/metrics`/`/docs` | `libs/cli/langgraph_cli/schemas.py:235-285`, `libs/cli/langgraph_cli/schemas.py:323-324`, `libs/cli/langgraph_cli/schemas.py:484-485` |
| OpenAPI — absence of importer | No `openapi` → tool importer, `import_openapi`, or `OpenAPIToolkit` in `libs/langgraph` or `libs/prebuilt`; only `openapi: SecurityConfig` annotation | `libs/cli/langgraph_cli/schemas.py:323` (negative evidence) |
| Provider API abstraction | Core depends on `langchain-core>=1.4.7` `BaseChatModel`, `RunnableBinding`, `LanguageModelLike`; dynamic model callable `(state, runtime)->BaseChatModel` enables provider swap without graph change | `libs/langgraph/pyproject.toml:27`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:279-289`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:590-593` |
| RemoteGraph protocol adapter | Distributed tracing flag propagated to `client.runs.stream`/`threads.stream` headers; sanitizes config and streams as `StreamPart` (`values`/`messages-tuple`) | `libs/langgraph/langgraph/pregel/remote.py:144-171`, `libs/langgraph/langgraph/pregel/remote.py:804-818`, `libs/langgraph/langgraph/pregel/remote.py:1046-1078` |

## Answers to Dimension Questions

### 1. Which open protocols are supported?

- **JSON Schema (Draft 2020-12 via Pydantic v2)**: First-class. Graph I/O/context schemas generated from `state_schema`/`input_schema`/`output_schema`/`context_schema` (supports `BaseModel`, `TypedDict`, `dataclass`). Implemented at `libs/langgraph/langgraph/pregel/main.py:994-1061` and `libs/langgraph/langgraph/graph/state.py:1944-1978`, wrapped by `create_model` in `libs/langgraph/langgraph/_internal/_pydantic.py:181-249`. Tested at `libs/langgraph/tests/test_state.py:167` and `libs/langgraph/tests/test_pregel.py:251`.
- **MCP (Model Context Protocol)**: Partial, server-side only. Deployment can be exposed as MCP server (`HttpConfig.disable_mcp` at `libs/cli/langgraph_cli/schemas.py:471`). Client-side tools can be passed as dict `{type:"mcp", server_url, allowed_tools}` (`libs/prebuilt/tests/test_react_agent.py:315`) but actual MCP client connection is external (example pattern at `libs/sdk-py/langgraph_sdk/runtime.py:116`).
- **OpenAPI**: As output only (server's own spec). `SecurityConfig`/`AuthConfig.openapi` (`libs/cli/langgraph_cli/schemas.py:235`) and `disable_meta` control of `/openapi.json` (`libs/cli/langgraph_cli/schemas.py:484`). No OpenAPI→tool importer found.
- **OTLP/OpenTelemetry**: Not natively supported. Tracing is LangSmith-based (`RemoteGraph.distributed_tracing` at `libs/langgraph/langgraph/pregel/remote.py:144`). OTEL only via optional `opentelemetry-instrumentation-langchain` monkey-patch noted in `libs/langgraph/tests/test_graph_callbacks.py:284`.
- **Provider APIs**: Abstracted via `langchain-core` `BaseChatModel`/`bind_tools`/`with_structured_output`, not direct HTTP protocol handling in LangGraph.

### 2. Is MCP supported?

Partial/conditional — **Not as a native client/server implementation in the studied source**, but as a deployment integration pattern. The CLI/server layer exposes `/mcp` routes (`libs/cli/langgraph_cli/schemas.py:471`) and the SDK runtime documentation encourages conditional initialization of MCP tools during execution (`libs/sdk-py/langgraph_sdk/runtime.py:87`, `libs/sdk-py/langgraph_sdk/runtime.py:106-122`). Tests demonstrate MCP tool dicts being bound alongside regular tools (`libs/prebuilt/tests/test_react_agent.py:315-326`), yet the core `ToolNode` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:743`) treats them as opaque `dict` built-ins rather than implementing MCP `tools/resources/prompts` discovery. No evidence of `MCPClient`, `list_tools`, `call_tool`, `get_resource`, or `get_prompt` in `libs/langgraph`. Full MCP requires external `langchain-mcp-adapters`.

### 3. Is OpenTelemetry supported?

No native OTLP exporter or OTEL SDK integration was found. Search for `opentelemetry|OTLP|TracerProvider` in `libs/` returns zero implementation hits. Instead, LangGraph propagates **LangSmith distributed tracing headers** when `RemoteGraph(distributed_tracing=True)` is set, merging `run_tree.to_headers()` + `baggage` at `libs/langgraph/langgraph/pregel/remote.py:1297-1308` and passing them to `client.runs.stream` at `libs/langgraph/langgraph/pregel/remote.py:817`. Tests use `langsmith.tracing_context` (`libs/langgraph/tests/test_utils.py:135`) and explicitly note OTEL works only via monkey-patching (`libs/langgraph/tests/test_graph_callbacks.py:284`). Operational OTEL therefore depends on user-added instrumentation, not a built-in exporter with sampling/batch/retry controls.

### 4. Are tool schemas portable across providers?

**Mostly yes, via delegation to `langchain_core`.** `ToolNode` normalizes functions to `BaseTool` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:780-783`) and emits OpenAI-compatible `ToolMessage` content. `create_react_agent`'s `_should_bind_tools` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-212`) explicitly handles both OpenAI (`function.name`) and Anthropic (`name`) bound-tool shapes, validating counts and names before deciding to call `model.bind_tools(tool_classes + llm_builtin_tools)` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:586`. Structured output is similarly portable via `with_structured_output` accepting JSON Schema/TypedDict/Pydantic (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:760-765`). However portability is bounded by provider support: `init_chat_model("openai:gpt-4")` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:570` shows string model resolution, and unknown tool payloads (e.g., `type:"mcp"` dicts) are passed through without translation, so provider-specific extensions still leak.

## Architectural Decisions

- **LangChain delegation over protocol ownership** — LangGraph avoids reimplementing tool/MCP/OpenAPI translation, delegating to `langchain_core` (`BaseTool`, `BaseChatModel.bind_tools`, `with_structured_output`) (`libs/langgraph/pyproject.toml:27`). Evidence: all tool execution flows through `BaseTool.invoke` at `libs/prebuilt/langgraph/prebuilt/tool_node.py:958`/`1105`; no direct HTTP/OpenAPI code.
- **Pydantic as canonical schema language** — `create_model` with `_SchemaConfig(arbitrary_types_allowed=True, frozen=True)` at `libs/langgraph/langgraph/_internal/_pydantic.py:54` centralizes JSON Schema generation; `TypeAdapter` fallback handles `TypedDict`/`dataclass` at `libs/langgraph/langgraph/graph/state.py:1953`. This makes JSON Schema the interop lingua franca.
- **Server/client split for protocols** — MCP/OpenAPI live in `libs/cli` and `libs/sdk-py` (deployment/SDK), not in `libs/langgraph` core orchestration. `libs/cli/langgraph_cli/schemas.py:235-354` defines OpenAPI auth, `libs/sdk-py/langgraph_sdk/runtime.py:87-119` defines MCP execution-runtime gating. Core graph remains protocol-agnostic.
- **LangSmith-first observability** — Tracing via `langsmith` run trees and header propagation (`libs/langgraph/langgraph/pregel/remote.py:1297`) rather than OTEL SDK; OTEL support is explicitly external/monkey-patched (`libs/langgraph/tests/test_graph_callbacks.py:284`).

## Notable Patterns

- **Per-access-context graph factory** — `ServerRuntime` discriminates `threads.create_run` (full execution with `context`) vs `threads.read`/`assistants.read` (schema/graph introspection) to avoid connecting MCP/DB during schema fetches. Implemented at `libs/sdk-py/langgraph_sdk/runtime.py:54-89`, `libs/sdk-py/langgraph_sdk/runtime.py:98-126`. This prevents expensive protocol connections during `get_input_jsonschema`/`get_graph` calls.
- **Injection-based tool context** — `InjectedState`/`InjectedStore`/`ToolRuntime` annotations let tools receive graph state/store without altering their JSON Schema externally; filtering of validation errors removes injected args from LLM-visible errors (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:959-964`). Keeps tool schemas portable.
- **Dynamic model callable for provider swap** — `create_react_agent(model: str|LanguageModelLike|Callable[[State,Runtime],BaseChatModel])` at `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:279-289` enables provider selection at runtime without graph recompilation, demonstrated with context-based routing (`libs/prebuilt/tests/test_react_agent.py:1559-1620`).

## Tradeoffs

- **Portability vs fidelity**: Delegating schema translation to `langchain_core` yields broad provider coverage but hides nuances (e.g., Anthropic `cache_control`, OpenAI `strict` mode) and requires `bind_tools` shape-sniffing (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:202-212`) that is brittle if providers add new tool types.
- **JSON Schema strength vs protocol gaps**: Mature schema generation (with reserved-name remapping at `libs/langgraph/langgraph/_internal/_pydantic.py:152-178`) contrasts with absent OpenAPI importer and MCP client, forcing users to write adapters for declarative API ingestion.
- **LangSmith coupling**: Deep integration with LangSmith (`distributed_tracing`, `langsmith_tracing` in `libs/sdk-py/langgraph_sdk/schema.py:111`) gives rich checkpoint-aware traces but creates vendor lock-in; OTEL users must add monkey-patch instrumentation without sampling/retry guarantees.
- **Server-only MCP**: Exposing a deployment as MCP server (`disable_mcp`) is cheap, but inability to consume MCP servers natively means multi-MCP orchestration requires external runtime code and careful `execution_runtime` gating, increasing operational complexity.

## Failure Modes / Edge Cases

- **MCP dict passthrough without validation** — `ToolNode` treats `{type:"mcp", ...}` as builtin tool list (`libs/prebuilt/tests/test_react_agent.py:315`) and defers validation to the model; typos in `server_url`/`allowed_tools` surface only at LLM call time, not graph compile time, and `require_approval` is unenforced in-core.
- **Missing OTEL backpressure handling** — No batch exporter, retry, or sampling config exists; high-cardinality traces via monkey-patch can block event loop (timeout policies at `libs/langgraph/langgraph/pregel/main.py:359` only cover node execution, not tracing I/O).
- **Reserved field collision** — `create_model` remaps `model_*` and `_`-prefixed keys to `private_*` with alias (`libs/langgraph/langgraph/_internal/_pydantic.py:157-174`); tools using `model_id`/`model_name` as parameters silently get aliasing that can confuse JSON Schema consumers if they inspect raw Pydantic fields.
- **Single-key `__root__` ambiguity** — `_get_json_schema` collapses single `__root__` channel to a RootModel (`libs/langgraph/langgraph/graph/state.py:1956-1960`); clients expecting object schema get a wrapped value schema, causing interoperability issues with strict OpenAPI validators.
- **Injected-arg error filtering hides real errors** — `_filter_validation_errors` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510`) strips injected args from `ValidationError`; if injection itself is misconfigured (e.g., missing `store`), the filtered error omits the root cause, making debugging harder.
- **Remote tracing header merge fragility** — `_merge_tracing_headers` concatenates `baggage` strings at `libs/langgraph/langgraph/pregel/remote.py:1302`; duplicate or oversized `baggage` can exceed header limits and fail silently (no truncation/retry).

## Future Considerations

- Implement a native MCP client (tools/resources/prompts discovery, `Streamable HTTP`/`SSE` transport) behind `ToolNode` or as a `BaseTool` factory, with schema caching tied to `ServerRuntime.execution_runtime` to avoid reconnect on introspection.
- Add OTLP HTTP exporter (OTEL SDK) with configurable `OTEL_EXPORTER_OTLP_ENDPOINT`, sampling, and `BatchSpanProcessor`, reusing existing `distributed_tracing` flag to emit W3C `traceparent`/`tracestate` alongside LangSmith headers.
- Provide OpenAPI 3.x → `BaseTool` importer (operationId → tool name, JSON Schema from components) mirrored after `langchain-community` toolkit, enabling `uvx` declarative toolchains; expose via `StateGraph.from_openapi(spec)`.
- Stabilize MCP tool dict schema (currently ad-hoc `server_label`/`server_url` at `libs/prebuilt/tests/test_react_agent.py:315`) into a typed `MCPToolSpec` with validation at graph compile time.
- Version JSON Schema output (`$schema`, `$id`) and add `get_openapi_schema()` alongside `get_input_jsonschema` to serve aggregated tool+graph spec under `/openapi.json`.

## Questions / Gaps

- No evidence of MCP `resources` or `prompts` exposure beyond `tools` — searched `resources|prompts` alongside `mcp` in `libs/`; only `disable_mcp` and tool dict found. Confirm whether LangGraph Platform server implements full MCP lifecycle.
- OTLP endpoint configuration absent from `libs/cli/langgraph_cli/schemas.py` and `libs/sdk-py/langgraph_sdk/schema.py`; is OTEL export intended to be handled exclusively by the host platform (e.g., LangSmith's OTEL receiver)?
- OpenAPI importer roadmap: docs reference `openapi: SecurityConfig` for output spec but no `import_openapi` or `toolFromOpenAPI`; verify if community-maintained `openapi` toolkit is the official recommendation.
- Tool portability across non-OpenAI providers (e.g., Gemini function calling with `response_schema`) not exercised in `libs/prebuilt/tests/test_react_agent.py`; need conformance tests for `with_structured_output` dialect mapping.

---

Generated by `19.01-protocol-compatibility` against `langgraph`.
