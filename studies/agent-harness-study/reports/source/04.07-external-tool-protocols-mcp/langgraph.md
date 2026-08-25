# Source Analysis: langgraph

## Dimension 04.07: External Tool Protocols and MCP Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core, prebuilt, cli, sdk-py), TypeScript/JS (sdk-js stub) |
| Analyzed | 2026-08-23 |

## Summary

LangGraph's approach to external tool protocols is **delegation, not implementation**. The core library (`libs/langgraph`) contains no MCP client or server code at all — a repo-wide search for `mcp` in Python source returns hits only in the prebuilt test suite, the SDK runtime docs, and the CLI config schema. External tools reach a LangGraph agent through three distinct channels:

1. **Provider-hosted MCP tools (pass-through dicts).** `create_react_agent` accepts arbitrary dict tool specs alongside callables (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:290`); dict-shaped tools are classified as "llm_builtin_tools", excluded from the local `ToolNode`, and forwarded verbatim to `model.bind_tools()` (`chat_agent_executor.py:554-561`, `586-588`). An OpenAI Responses-style hosted MCP spec (`"type": "mcp"`, `server_url`, `headers`, `allowed_tools`, `require_approval`) is exercised in `libs/prebuilt/tests/test_react_agent.py:313-327`. Execution, transport, auth, and approval for these tools happen entirely inside the model provider; LangGraph only routes the spec and ingests resulting ToolMessages.

2. **Out-of-process MCP clients via ecosystem packages.** The intended way to *call* external MCP servers is user-supplied adapter code (e.g., `langchain-mcp-adapters`, not a dependency here — see `libs/prebuilt/pyproject.toml:26-29`). The SDK formalizes where such connections belong: `ServerRuntime.execution_runtime` lets graph factories connect to "MCP tool servers" only during actual run execution while introspection contexts skip them (`libs/sdk-py/langgraph_sdk/runtime.py:97-127`, worked example at `runtime.py:194-221`).

3. **Serving agents as MCP/A2A endpoints.** The deployment config schema defines `http.disable_mcp` ("If True, /mcp routes are removed, disabling default support to expose the deployment as an MCP server") and `http.disable_a2a` (`libs/cli/langgraph_cli/schemas.py:471-480`, mirrored in JSON Schema at `libs/cli/schemas/schema.json:1025-1027`). Both default to enabled. The route implementations live in the closed-source Agent Server; this repo owns only the configuration contract and the graph-schema plumbing those protocols consume (`runtime.py:84-88`: schemas are surfaced "to populate schemas for MCP, A2A, and other protocol integrations"; REST endpoint `assistants.get_schemas` at `libs/sdk-py/langgraph_sdk/_async/assistants.py:153-178`).

Trust boundaries around external interaction are real but split across layers: a pluggable server-side auth framework (`Auth` class with `@auth.authenticate` / `@auth.on` handlers, `libs/sdk-py/langgraph_sdk/auth/__init__.py:13-107`), an OpenAPI security-declaration surface (`SecurityConfig` in `libs/cli/langgraph_cli/schemas.py:235-284`, wired into `AuthConfig.openapi` at `schemas.py:308-353`), SSRF-conscious webhook URL policies (`WebhookUrlPolicy`, `schemas.py:538-557`), deserialization allowlists for checkpoint data (`SerdeConfig`, `schemas.py:127-190`), and per-tool error isolation that converts tool failures into `ToolMessage`s instead of crashing runs (`libs/prebuilt/langgraph/prebuilt/tool_node.py:982-1009`). However, several defaults are permissive rather than safe-by-default, and there is no sandboxing of externally-sourced tools anywhere in this codebase.

**Rating rationale:** external tool protocol support exists on both the consuming side (hosted-MCP pass-through + documented adapter seam) and serving side (MCP/A2A exposure via config), but the protocol machinery itself is mostly outside this repository, the one in-repo behavior test only checks spec pass-through, and trust defaults lean permissive. This lands squarely in the "present but inconsistent / weakly evidenced in-repo" band.

## Rating

**5 / 10** — Present but inconsistent and fragile as observed inside this repository.

- Clear configuration model for exposing deployments as MCP/A2A servers (`libs/cli/langgraph_cli/schemas.py:471-480`) and a deliberate lifecycle hook for connecting to external MCP servers (`libs/sdk-py/langgraph_sdk/runtime.py:106-123`) earn points.
- Deductions: zero MCP client/server implementation code in-repo; hosted-MCP specs are unvalidated pass-throughs whose unknown variants are silently skipped (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:208-210`); authorization defaults to accept when no handler is registered (`libs/sdk-py/langgraph_sdk/auth/__init__.py:99-103`); custom HTTP routes are unauthenticated by default (`libs/cli/langgraph_cli/schemas.py:522-529`); no sandboxing or per-tool permission layer for external tools.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dict tools accepted as provider-hosted specs | `tools: Sequence[BaseTool \| Callable \| dict[str, Any]] \| ToolNode` parameter type | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:290 |
| Hosted-MCP dicts separated from locally-executed tools | `llm_builtin_tools = [t for t in tools if isinstance(t, dict)]`; only non-dicts go to `ToolNode` | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-561 |
| Dict specs bound straight to the model | `model.bind_tools(tool_classes + llm_builtin_tools)` | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:586-588 |
| OpenAI-style MCP tool spec shape (server_url/sse, headers, allowed_tools, require_approval) | `"type": "mcp"` dict in `test_model_with_tools` | libs/prebuilt/tests/test_react_agent.py:313-327 |
| Unknown bound-tool shapes silently ignored during name matching | `_should_bind_tools` comment "unknown tool type so we'll ignore it" | libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:201-210 |
| Execution-only MCP connection hook (beta) | `execution_runtime` narrows factory calls; example connects `mcp_tools` only for `threads.create_run` | libs/sdk-py/langgraph_sdk/runtime.py:97-127 |
| Graph factories documented for per-user MCP connections | second example passes `user_id` into `connect_mcp(...)` | libs/sdk-py/langgraph_sdk/runtime.py:194-221 |
| Schemas feed MCP/A2A integrations | access_context docs: schemas used "to populate schemas for MCP, A2A, and other protocol integrations" | libs/sdk-py/langgraph_sdk/runtime.py:80-89 |
| Deployment exposed as MCP server by default; opt-out flag | `disable_mcp` field doc ("/mcp routes are removed…") | libs/cli/langgraph_cli/schemas.py:471-475 |
| A2A exposure flag | `disable_a2a` field doc | libs/cli/langgraph_cli/schemas.py:476-480 |
| Same flags in published JSON Schema for langgraph.json | `disable_mcp` property description | libs/cli/schemas/schema.json:1025-1027 |
| Server auth framework (authenticate + authorize handlers) | `Auth` class usage docs, request-processing flow | libs/sdk-py/langgraph_sdk/auth/__init__.py:13-107 |
| Authorization default is accept | flow step: "If no global handler is set, the request is accepted" | libs/sdk-py/langgraph_sdk/auth/__init__.py:96-107 |
| User identity/permission protocol for external callers | `MinimalUser.identity`, `BaseUser.permissions` protocols | libs/sdk-py/langgraph_sdk/auth/types.py:150-161, 181-215 |
| Resource/action authorization vocabulary | `AuthContext.resource` Literal["runs","threads","crons","assistants","store"], action literals | libs/sdk-py/langgraph_sdk/auth/types.py:397-426 |
| Studio auth bypass switchable | `disable_studio_auth` doc; `StudioUser` class | libs/cli/langgraph_cli/schemas.py:316-322; libs/sdk-py/langgraph_sdk/auth/types.py:218-251 |
| OpenAPI security declarations exported for the server | `SecurityConfig.securitySchemes/security/paths`; referenced from `AuthConfig.openapi` | libs/cli/langgraph_cli/schemas.py:235-284, 323-343 |
| Outbound webhook trust boundary (SSRF controls) | `require_https`, `allowed_domains`, `allowed_ports`, loopback disable | libs/cli/langgraph_cli/schemas.py:538-557 |
| Checkpoint deserialization allowlists | `SerdeConfig.allowed_json_modules` / `allowed_msgpack_modules` / `pickle_fallback` | libs/cli/langgraph_cli/schemas.py:127-190 |
| Auth-vs-middleware ordering control | `middleware_order: "auth_first" \| "middleware_first"` | libs/cli/langgraph_cli/schemas.py:511-521 |
| Custom routes unprotected unless opted in | `enable_custom_route_auth` "Default is False" | libs/cli/langgraph_cli/schemas.py:522-529 |
| Local tool-call schema validation via pydantic | `ValidationError` caught → `ToolInvocationError` with filtered injected-arg errors | libs/prebuilt/langgraph/prebuilt/tool_node.py:956-966 |
| Failure isolation converts errors to ToolMessages | exception dispatch on `handle_tool_errors`; returns `ToolMessage(content=...)` | libs/prebuilt/langgraph/prebuilt/tool_node.py:982-1009 |
| Unknown/unregistered tool → error ToolMessage, not crash | `_validate_tool_call` builds `INVALID_TOOL_NAME_ERROR_TEMPLATE` message | libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110, 1268-1279 |
| Configurable error-handling strategies | `handle_tool_errors` bool/str/type/tuple/callable/False docs | libs/prebuilt/langgraph/prebuilt/tool_node.py:674-694 |
| Function→schema conversion delegated to langchain-core | `from langchain_core.tools import BaseTool, create_schema_from_function` | libs/prebuilt/langgraph/prebuilt/tool_validator.py:24 |
| Graph schemas queryable over REST | `assistants.get_schemas(assistant_id)` returning JSON-Schema-style state/input/output | libs/sdk-py/langgraph_sdk/_async/assistants.py:153-199 |
| SSE transport hardening tests (stream failure surfacing) | `test_mid_stream_error_after_ready_surfaces_on_done` over `ProtocolSseTransport` | libs/sdk-py/tests/streaming/test_transport_http.py:213-241 |
| No MCP client dependency in prebuilt | dependencies limited to `langgraph-checkpoint`, `langchain-core` | libs/prebuilt/pyproject.toml:26-29 |
| Docs for MCP moved out of repo | redirects `/concepts/server-mcp` → external docs site; llms.txt states docs relocated | docs/redirects.json:168,220-221,253; docs/llms.txt:1-3 |

## Answers to Dimension Questions

**1. Can tools live outside the process?**
Yes, through two mechanisms. (a) Provider-hosted tools: dict specs such as the OpenAI-style MCP entry tested in `libs/prebuilt/tests/test_react_agent.py:313-327` are excluded from local execution and delegated to the model provider (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:559-560, 586-588`) — the tool never runs in the agent process. (b) Remote tool servers: the SDK explicitly anticipates connecting to "MCP tool servers" at graph-factory time via `execution_runtime` (`libs/sdk-py/langgraph_sdk/runtime.py:106-123`), though the actual client comes from an external package (no MCP dependency in `libs/prebuilt/pyproject.toml:26-29`). Additionally, the deployment itself can *be* an out-of-process tool provider: `/mcp` routes expose graphs as an MCP server unless `disable_mcp` is set (`libs/cli/langgraph_cli/schemas.py:471-475`).

**2. Are external tools trusted by default?**
Effectively yes, at multiple layers. Hosted MCP dict specs are passed through without structural validation, and unrecognized shapes are silently dropped during bind-matching (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:208-210`). There is no per-tool permission or sandboxing layer anywhere in the repo; the only gate for hosted tools is the spec's own `require_approval` field, enforced provider-side (`libs/prebuilt/tests/test_react_agent.py:325`). On the serving side, authorization accepts requests when no handler matches (`libs/sdk-py/langgraph_sdk/auth/__init__.py:99-103`), and custom routes bypass authentication unless `enable_custom_route_auth` is enabled (`libs/cli/langgraph_cli/schemas.py:522-529`). Safe-by-default posture must be configured explicitly.

**3. How are schemas imported?**
Locally executed tools get schemas inferred from Python signatures via langchain-core (`create_schema_from_function`, imported at `libs/prebuilt/langgraph/prebuilt/tool_validator.py:24`; plain functions auto-converted per `ToolNode` docs at `libs/prebuilt/langgraph/prebuilt/tool_node.py:663-668`) and validated against pydantic before invocation (`tool_node.py:956-966`). For outbound directions, graph input/state/output schemas are emitted as JSON Schema and served via `assistants.get_schemas` (`libs/sdk-py/langgraph_sdk/_async/assistants.py:153-199`), which the runtime docs say exist "to populate schemas for MCP, A2A, and other protocol integrations" (`libs/sdk-py/langgraph_sdk/runtime.py:84-88`). Inbound MCP tool schemas (list_tools → tool definitions) would be converted by the external adapters package — no evidence of that conversion in this repo.

**4. How are failures isolated?**
For locally-executed tools, robustly: `ToolNode` catches exceptions according to configurable `handle_tool_errors` strategies (bool/string/exception-type/callable; `libs/prebuilt/langgraph/prebuilt/tool_node.py:674-694`) and converts them into `ToolMessage`s with status="error" so the agent loop can recover (`tool_node.py:984-1009`); invalid arguments raise a structured `ToolInvocationError` with injected-arg errors filtered out (`tool_node.py:339-380, 956-966`); unknown tool names yield an advisory error message listing valid tools (`tool_node.py:1268-1279`); interrupts propagate deliberately (`GraphBubbleUp` re-raised at `tool_node.py:982-983`). For provider-hosted MCP tools, failure isolation is out of scope — whatever the provider returns arrives as ordinary messages, and mid-stream transport failures in the SSE client layer are surfaced on a done-handle rather than silently dropped (`libs/sdk-py/tests/streaming/test_transport_http.py:213-241`).

**5. Can the same tool work across clients?**
Partially, by indirection rather than by tool portability. Tools are bound in application code against langchain-core interfaces, so they are portable across any Python consumer sharing that runtime. Cross-client reach is achieved by putting the agent behind LangGraph Server: sdk-js and sdk-py speak the same REST/SSE protocol (`libs/sdk-js/README.md:1-9` notes the JS SDK consumes the same API; `libs/sdk-py/langgraph_sdk/stream/transport/http.py:33+` implements the shared SSE transport). Exposure as an MCP server (`libs/cli/langgraph_cli/schemas.py:471-475`) makes the whole agent callable from any MCP client, and A2A does the same for agent-to-agent callers (`schemas.py:476-480`). There is no mechanism to register one tool object simultaneously into multiple heterogeneous clients.

## Architectural Decisions

1. **Dict-typed tools as the extension point for provider-hosted protocols** (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-561`). Rather than modeling hosted MCP tools as first-class objects, LangGraph treats any opaque dict as a provider-side capability. This buys zero-cost support for whatever the provider invents, at the price of no local validation and silent skipping of unknown shapes (`chat_agent_executor.py:208-210`).

2. **Protocol serving owned by the platform, not the library.** `/mcp` and `/a2a` are server features toggled from `langgraph.json` (`libs/cli/langgraph_cli/schemas.py:471-480`); implementations live in the closed-source Agent Server. The open repo guarantees the config contract and supplies the schema payloads (`runtime.py:84-88`, `assistants.get_schemas` at `libs/sdk-py/langgraph_sdk/_async/assistants.py:153`).

3. **Resource lifecycle decoupled from graph topology.** `ServerRuntime.execution_runtime` separates "build graph for introspection" from "wire expensive resources for execution," letting MCP connections be scoped strictly to runs and even per-user (`libs/sdk-py/langgraph_sdk/runtime.py:97-145, 194-221`).

4. **User-supplied auth middleware instead of built-in identity.** Authentication/authorization is delegated to developer-provided handlers declared in config (`AuthConfig.path`, `libs/cli/langgraph_cli/schemas.py:308-315`), with OpenAPI security declarations exported for documentation (`SecurityConfig`, `schemas.py:235-284`).

5. **Error-isolation at the tool boundary, not process boundary.** Failures become data (`ToolMessage`) so the LLM can self-correct, with interrupt signals carefully exempted (`libs/prebuilt/langgraph/prebuilt/tool_node.py:982-1009`).

## Notable Patterns

- **Capability triage at construction time**: one list comprehension splits tools into "bind to model" vs "execute locally" (`chat_agent_executor.py:559-560`).
- **Deny-by-default is opt-in**: the documented pattern instructs users to install a global `deny_all` handler first (`libs/sdk-py/langgraph_sdk/auth/__init__.py:61-64`), i.e., secure posture is a recipe, not a default.
- **Mutable authorization values**: auth handlers can rewrite operation payloads (e.g., stamping `metadata["owner"]` or scoping store namespaces) — enforcement by transformation (`libs/sdk-py/langgraph_sdk/auth/types.py:862-871` docstrings note "auth handlers can modify namespace").
- **Allowlist-based deserialization**: checkpoint serde restricts importable modules, with strict modes blocking unregistered types (`libs/cli/langgraph_cli/schemas.py:127-190`) — a trust-boundary pattern applied to state rather than tools.
- **Docs-as-contract**: extensive TypedDict docstrings in `langgraph_cli/schemas.py` are the operative specification, duplicated into generated JSON Schema (`libs/cli/schemas/schema.json:1025-1027`).

## Tradeoffs

- **Pass-through flexibility vs safety**: accepting raw provider tool dicts means LangGraph cannot validate `server_url`, `headers`, or `allowed_tools` before shipping secrets to a provider endpoint; nothing prevents header material from being logged or mis-scoped.
- **Ecosystem delegation vs cohesion**: pushing MCP-client concerns to `langchain-mcp-adapters` keeps core lean but means this repository offers no tested, versioned path for the most common external-tool integration; correctness depends on out-of-repo packages.
- **Closed-source server features**: MCP/A2A serving can be configured but not inspected here — operational behavior under failure is unverifiable from this source tree (docs also relocated: `docs/llms.txt:1-3`).
- **Permissive defaults vs ease of onboarding**: accept-if-no-authz-handler and unauthenticated custom routes lower friction but shift security burden onto every user (`auth/__init__.py:99-103`; `schemas.py:522-529`).
- **Error-to-message isolation vs masking**: converting all handled exceptions into ToolMessages keeps runs alive but can leak internal error details (repr included per `TOOL_CALL_ERROR_TEMPLATE`, `tool_node.py:111`) into model context.

## Failure Modes / Edge Cases

- **Unknown hosted-tool dialects vanish**: a bound dict lacking `"type": "function"` or `"name"` is skipped in name matching (`chat_agent_executor.py:201-210`); a mismatch then surfaces only as a confusing "Missing tools" ValueError (`chat_agent_executor.py:214-215`) or, if counts align, as a model that never sees the tool.
- **Hosted-MCP results bypass local validation entirely**: since dict tools never enter `ToolNode`, malformed provider responses are not schema-checked in-process.
- **Unregistered tool calls degrade gracefully but permissively**: `ToolNode` returns an error ToolMessage listing available tools (`tool_node.py:1268-1279`) — good for recovery, but it means hallucinated tool names cost a full round trip rather than failing fast.
- **SSE stream breakage mid-run**: transport failures after headers are raised on the done handle so callers can distinguish clean end from drop (`libs/sdk-py/tests/streaming/test_transport_http.py:213-241`) — the SDK-side guard for long-lived protocol streams.
- **Authorization gaps by composition**: because specific handlers shadow broader ones (`auth/__init__.py:98-103`), adding a narrow `@auth.on.threads.create` can accidentally widen access relative to an intended global deny if resource-level handlers are absent.
- **No timeout wrapper around local tool execution in ToolNode itself**: a hung external connection inside a tool blocks the node; cancellation/timeouts must come from run-level settings (not visible in `tool_node.py`).

## Future Considerations

- Introduce validated, typed descriptors for provider-hosted tool specs (e.g., reject or warn on unknown dict shapes instead of silently ignoring them at `chat_agent_executor.py:208-210`).
- Add an optional per-tool permission/approval layer in `ToolNode` so externally-sourced tools can require human gating uniformly, independent of provider-side `require_approval`.
- Flip or document-safe defaults: consider making missing authorization handlers deny (`auth/__init__.py:99-103`) or emit loud warnings; same for `enable_custom_route_auth` (`schemas.py:522-529`).
- Expose conformance tests for the `/mcp` and `/a2a` server surfaces analogous to the checkpoint conformance package referenced at `libs/cli/langgraph_cli/schemas.py:213-217`, so protocol behavior is verifiable without the closed-source server.

## Questions / Gaps

- **No MCP client implementation found in-repo.** Searched all `*.py`/`*.ts`/`*.toml` for `mcp`, `jsonrpc`, `langchain_mcp_adapters`, `MultiServerMCPClient`: only the test pass-through (`libs/prebuilt/tests/test_react_agent.py:315`), SDK docs (`libs/sdk-py/langgraph_sdk/runtime.py`), and CLI config keys match. The consuming-side MCP stack is entirely out-of-tree.
- **No plugin manifest system found.** Searches for plugin manifests, tool-server registries, or OpenAPI-based tool generation returned only the server-side `SecurityConfig` OpenAPI declarations (`libs/cli/langgraph_cli/schemas.py:235-284`), which document auth schemes rather than generate tools. "OpenAPI tool generation": No clear evidence found within this source boundary.
- **Transport details for `/mcp` routes unverifiable**: the flag exists (`schemas.py:471-475`) but whether it speaks streamable-HTTP, SSE, or stdio cannot be determined from this repository.
- **Sandboxing**: no OS-level, network-level, or capability-based sandboxing of tools exists in this codebase; isolation is limited to exception handling (`tool_node.py:982-1009`) and config-level network policies for webhooks/CORS (`schemas.py:376-412, 538-557`).

---

Generated by `dimensions/04.07-external-tool-protocols-and-mcp-integration.md` against `langgraph`.
