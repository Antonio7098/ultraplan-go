# Source Analysis: pydantic-ai

## Dimension 04.07: External Tool Protocols and MCP Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, anyio, httpx, FastMCP client SDK, MCP SDK v1/v2) |
| Analyzed | 2026-08-23 |

## Summary

pydantic-ai treats external tools as a first-class axis of its architecture rather than a bolt-on. The core abstraction is `AbstractToolset` (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:76`), a composable collection interface (`get_tools` / `call_tool` / lifecycle / instructions) that both local function tools and remote protocol tools implement. On top of it, `MCPToolset` (`pydantic_ai_slim/pydantic_ai/mcp.py:716`) is a full MCP client built on the FastMCP client, supporting tools, resources, prompts, sampling, elicitation, logging, roots, and task-augmented execution (SEP-1686), over stdio, Streamable HTTP, SSE, in-process FastMCP servers, and pre-built clients (`pydantic_ai_slim/pydantic_ai/mcp.py:699-705`).

Three integration surfaces coexist deliberately:

1. **Local MCP client** — `MCPToolset` runs the protocol in-process/subprocess so credentials, hooks, and tracing stay under user control (`pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:30-33`, `docs/mcp/overview.md:33-35`).
2. **Provider-native MCP** — `MCPServerTool` (`pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:559`) hands the server URL to OpenAI Responses/Anthropic/xAI, which execute the MCP calls themselves (`pydantic_ai_slim/pydantic_ai/models/openai.py:2992-3016`, `pydantic_ai_slim/pydantic_ai/models/anthropic.py:935,968`).
3. **Provider-adaptive capability** — the `MCP` capability (`pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:27`) pairs native execution with a local fallback via `NativeOrLocalTool` (`pydantic_ai_slim/pydantic_ai/capabilities/native_or_local.py:18-22`), so the same URL works across providers without code changes.

The reverse direction is also supported: pydantic-ai agents can run inside MCP servers and call back through MCP sampling via `MCPSamplingModel` (`pydantic_ai_slim/pydantic_ai/models/mcp_sampling.py:22-98`, `docs/mcp/server.md:66-90`).

Trust boundaries are explicit but opt-in: external tool arguments are *not* validated against the server's JSON Schema on the client (`pydantic_ai_slim/pydantic_ai/mcp.py:632-636` uses a permissive `dict[str, Any]` validator); failures from servers are converted to model-recoverable outcomes by policy (`tool_error_behavior`, `pydantic_ai_slim/pydantic_ai/mcp.py:764-776`); approval/filtering/prefixing are applied by wrapping toolsets. Notably, there is no OpenAPI-to-tool generation in this codebase; the external-tool story is entirely MCP-centered.

## Rating

**8 / 10** — A clear, well-tested model with explicit interfaces and operational safeguards: full protocol coverage with capability gating (`ServerCapabilities`, `pydantic_ai_slim/pydantic_ai/mcp.py:575-628`), dual MCP SDK generation compat (`pydantic_ai_slim/pydantic_ai/_mcp_compat.py:14-17`), production-informed failure handling including `ExceptionGroup` unwrapping (`pydantic_ai_slim/pydantic_ai/mcp.py:1420-1447`), bounded session teardown (`pydantic_ai_slim/pydantic_ai/mcp.py:646-651`), and a 2,239-line integration test suite against real in-process servers (`tests/test_mcp.py`). It falls short of 9–10 because two trust-boundary gaps remain deliberate-but-risky defaults: no client-side validation of external tool arguments against their declared schemas, and no SSRF screening of user-supplied MCP URLs (`_ssrf.py` exists but is not referenced from the MCP modules).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Toolset abstraction (portability spine) | `AbstractToolset` defines `get_tools`/`call_tool`/lifecycle/instructions contract; wrapper combinators `.filtered()`, `.prefixed()`, `.renamed()`, `.approval_required()`, `.defer_loading()` | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:76-98,194-257` |
| MCP client entry point | `MCPToolset(AbstractToolset)` docstring: "built on the FastMCP Client … supports the full MCP protocol" | `pydantic_ai_slim/pydantic_ai/mcp.py:715-758` |
| Accepted connection shapes | `MCPToolsetClient = FastMCPClient \| ClientTransport \| FastMCP \| FastMCP1Server \| AnyUrl \| Path \| str` | `pydantic_ai_slim/pydantic_ai/mcp.py:699-705` |
| Dependency isolation | `[mcp]` extra pulls `fastmcp-slim[client]`; separate `mcp-tasks` extra for SEP-2663 tasks; hard import guard with actionable error | `pydantic_ai_slim/pyproject.toml:126-138`; `pydantic_ai_slim/pydantic_ai/mcp.py:26-47` |
| Transport adapters | Stdio/SSE/StreamableHTTP imported from fastmcp; `_build_transport` infers SSE vs streamable from URL, rejects HTTP-only kwargs for non-URL inputs | `pydantic_ai_slim/pydantic_ai/mcp.py:34-41,1647-1693` |
| HTTP auth surface | `auth: httpx.Auth \| Literal['oauth'] \| str`, `verify: ssl.SSLContext \| bool \| str`, `headers`, `http_client` kwargs | `pydantic_ai_slim/pydantic_ai/mcp.py:897-900,952-959` |
| Per-user auth hazard documented | Warning that a shared toolset is a single identity; per-user auth requires per-run toolsets via `@agent.toolset` | `docs/mcp/client.md:463-500` (esp. warning at 471) |
| Native MCP tool definition | `MCPServerTool` dataclass: `url`, `authorization_token`, `allowed_tools`, `headers`, `kind='mcp_server'`, `unique_id = 'mcp_server:<id>'` | `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:558-627` |
| Native wire mapping (OpenAI) | Responses API `tool_param.Mcp` built with authorization token, allowed_tools, headers, connector_id, server_url; results mapped back as `mcp_call`/`mcp_list_tools` items | `pydantic_ai_slim/pydantic_ai/models/openai.py:2992-3016,5570-5615` |
| Native wire mapping (Anthropic) | `_add_native_tools` splits tools into `tools` + `mcp_servers` request params | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:935,968` |
| Native/local parity | `MCP` capability applies `authorization_token`/`headers`/`allowed_tools` to both sides; local fallback builds an `MCPToolset` from the URL | `pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:41-56,186-199,208-213` |
| Schema import (input/output) | `get_tools()` copies server `inputSchema` into `ToolDefinition.parameters_json_schema` and `outputSchema` into `return_schema`; annotations and SEP-1686 `execution.taskSupport` preserved as metadata | `pydantic_ai_slim/pydantic_ai/mcp.py:1295-1325` |
| Schema structural validation | Schemas validated only as `dict[str, Any]` through `TypeAdapter`; field reads go through SDK v1/v2 compat helpers | `pydantic_ai_slim/pydantic_ai/mcp.py:84,1301-1303`; `pydantic_ai_slim/pydantic_ai/_mcp_compat.py:20-56` |
| Args NOT schema-validated client-side | `TOOL_SCHEMA_VALIDATOR = SchemaValidator(dict_schema(str→any))` used as the args validator for all MCP tools | `pydantic_ai_slim/pydantic_ai/mcp.py:632-636,1334-1339` |
| Validation pipeline location | `_validate_tool_args` runs `args_validator.validate_json/python` then optional custom validator funcs | `pydantic_ai_slim/pydantic_ai/tool_manager.py:306-348` |
| Result mapping | Structured content preferred; JSON-looking text parsed; images/audio → `BinaryContent`; embedded resources/resource links mapped | `pydantic_ai_slim/pydantic_ai/mcp.py:1757-1818` |
| Failure policy | `tool_error_behavior: 'retry' \| 'error' \| 'failed'` maps server errors to `ModelRetry` / raw `ToolError` / `ToolFailed` | `pydantic_ai_slim/pydantic_ai/mcp.py:764-776,1740-1754` |
| ExceptionGroup isolation | Group split logic converts pure tool/protocol error groups, re-raises groups containing cancellations; comment cites a production observation | `pydantic_ai_slim/pydantic_ai/mcp.py:1411-1447` |
| Capability gating | `list_prompts`/`list_resources` return empty when server lacks capability; `get_prompt` raises coded `MCPError` | `pydantic_ai_slim/pydantic_ai/mcp.py:1491-1499,1546-1554,1516-1521` |
| Cache coherence | Tool/resource/prompt caches invalidated on `*ListChangedNotification` via wrapped message handler | `pydantic_ai_slim/pydantic_ai/mcp.py:786-811,1627-1644` |
| Lifecycle safety | Init commits state only after full success (`exit_stack.pop_all()`); ref-counted enter/exit; deferred `anyio.Lock` for Temporal sandbox; bounded shutdown grace | `pydantic_ai_slim/pydantic_ai/mcp.py:1165-1252,862-866,646-651` |
| Modern-session degradation warnings | Sampling/elicitation handlers that can never fire on stateless modern sessions produce explicit `UserWarning`s | `pydantic_ai_slim/pydantic_ai/mcp.py:1207-1228`; tests at `tests/test_mcp.py:1315,1396` |
| Task-augmented execution | `prefer_tasks` + per-tool `taskSupport` routing; FastMCP 4 path requires optional `fastmcp-tasks`; legacy-session rejection | `pydantic_ai_slim/pydantic_ai/mcp.py:778-784,1341-1364`; fixture `tests/mcp_task_server.py:1-13` |
| Config-file portability | `load_mcp_toolsets` parses Claude Desktop/Cursor-style `mcpServers` JSON, expands `${VAR}`/`${VAR:-default}`, prefixes each server's tools | `pydantic_ai_slim/pydantic_ai/mcp.py:1877-1974` |
| Config trust warning | "Treat configuration files as trusted input" — config can spawn arbitrary executables and read env vars without allowlist | `docs/mcp/client.md:218-220` |
| Approval wrapper applicable to MCP | `ApprovalRequiredToolset.call_tool` raises `ApprovalRequired` unless approved; docs point MCP users at this wrapper | `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:16-31`; `docs/deferred-tools.md:99` |
| Approval boundary caveat | "Approval is not an authorization boundary against an untrusted client" warning in deferred-tools docs | `docs/deferred-tools.md:101-103` |
| Elicitation security note | Elicitation handler docs link MCP spec security considerations (servers must not request sensitive info) | `docs/mcp/client.md:784-786` |
| SSRF module exists but unused for MCP | `_ssrf.safe_download` blocks private networks/metadata endpoints (`_PRIVATE_NETWORKS` table); no reference to `_ssrf` from `mcp.py` or `capabilities/mcp.py` | `pydantic_ai_slim/pydantic_ai/_ssrf.py:1-5,35-52` (absence verified by search) |
| External-execution toolset | `ExternalToolset` marks tools whose results are produced outside the agent run entirely (`call_tool` raises `NotImplementedError`) | `pydantic_ai_slim/pydantic_ai/toolsets/external.py:15-46` |
| Spec serialization subset | `MCP.from_spec` restricts `local=` to JSON/YAML-serializable values for `AgentSpec` round-tripping; tested | `pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:215-248`; `tests/test_capabilities.py:242-265` |
| Durable-execution identity | Toolset `id` required to identify activities in Temporal/DBOS workflows | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:91-96`; `pydantic_ai_slim/pydantic_ai/mcp.py:906-908` |
| Reverse direction: sampling model | Agent inside an MCP server samples via `MCPSamplingModel(session=ctx.session)`; settings mapped both ways in `_mcp.py` | `pydantic_ai_slim/pydantic_ai/models/mcp_sampling.py:22-98`; `pydantic_ai_slim/pydantic_ai/_mcp.py:22-131` |
| Protocol test breadth | Integration tests against in-process FastMCP server: lifecycle, caching, notifications, resources/prompts, sampling round-trip, env expansion, exception groups, task routing | `tests/test_mcp.py:215-2220` (e.g., 526, 1234, 1567, 1723, 1916); server fixture `tests/mcp_server.py:16` |

## Answers to Dimension Questions

### 1. Can tools live outside the process?

Yes, in three distinct ways. (a) Remote MCP servers over Streamable HTTP or SSE (`pydantic_ai_slim/pydantic_ai/mcp.py:1687-1693`) with automatic SSE/streamable inference (`pydantic_ai_slim/pydantic_ai/mcp.py:1675`). (b) Local subprocesses via stdio transports spawned from script paths or config entries (`pydantic_ai_slim/pydantic_ai/mcp.py:1960-1967`). (c) Fully provider-side execution via `MCPServerTool`, where the model provider performs the MCP calls and pydantic-ai only serializes/deserializes `mcp_call`/`mcp_list_tools` wire items (`pydantic_ai_slim/pydantic_ai/models/openai.py:2992-3016,3517-3538`). Additionally, `ExternalToolset` formalizes tools whose results are produced outside the agent run altogether (`pydantic_ai_slim/pydantic_ai/toolsets/external.py:15-46`).

### 2. Are external tools trusted by default?

Largely yes. The args validator applied to every MCP tool is a permissive `dict[str, str → Any]` schema, not the tool's declared `inputSchema` (`pydantic_ai_slim/pydantic_ai/mcp.py:632-636,1338`) — malformed model-generated arguments reach the server, and only the server's protocol/tool errors come back (converted to `ModelRetry` by default). Server instructions are excluded from the prompt unless `include_instructions=True` (`pydantic_ai_slim/pydantic_ai/mcp.py:813-818,1270-1278`), a small anti-prompt-injection default. Filtering to an allowlist exists only when the user supplies `allowed_tools` (`pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:208-213`), and human approval is an opt-in wrapper (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:26-31`). There is no SSRF screening on user-supplied MCP URLs: the `_ssrf` module blocks private-network targets for downloads (`pydantic_ai_slim/pydantic_ai/_ssrf.py:1-5,32-52`) but nothing in `mcp.py` references it, so a config file or URL can target internal services. The docs are candid about residual risks: config files are declared trusted input because they spawn executables and expand the full environment (`docs/mcp/client.md:218-220`), shared sessions leak credentials across tenants if misused (`docs/mcp/client.md:471-473`), and approval is not an authorization boundary (`docs/deferred-tools.md:101-103`).

### 3. How are schemas imported?

Pass-through of MCP JSON Schema with structural validation only. `get_tools()` validates `inputSchema`/`outputSchema` as generic dicts via `TypeAdapter(dict[str, Any])` (`pydantic_ai_slim/pydantic_ai/mcp.py:84,1301-1303`) and copies them into `ToolDefinition.parameters_json_schema` / `return_schema` (`pydantic_ai_slim/pydantic_ai/mcp.py:1305-1321`). Return schemas are hidden from the model unless enabled (`include_return_schema`, defaulting off unless the `IncludeToolReturnSchemas` capability is present — `pydantic_ai_slim/pydantic_ai/mcp.py:820-826`). Because MCP SDK v2 renamed fields from camelCase to snake_case, all field reads route through compat helpers that try both spellings (`pydantic_ai_slim/pydantic_ai/_mcp_compat.py:20-56`), so schemas survive either SDK generation; tests pin this behavior (`tests/test_mcp.py:136-213`). No OpenAPI/WSDL import path exists anywhere in the package — searches for `openapi` in `pydantic_ai_slim` return only unrelated provider matches.

### 4. How are failures isolated?

Layered. Policy level: `tool_error_behavior` chooses between `ModelRetry` (self-correction), `ToolFailed` (terminal, visible to model), and propagating raw `fastmcp.ToolError` (`pydantic_ai_slim/pydantic_ai/mcp.py:764-776,1407-1419,1740-1754`; exception definitions `pydantic_ai_slim/pydantic_ai/exceptions.py:57-61,100-105`). Protocol level: bare `McpError` (e.g., gateway JSON-RPC rejection) always becomes retryable even under `'failed'` (`pydantic_ai_slim/pydantic_ai/mcp.py:1411-1419,1433-1441`). Concurrency level: errors surfacing inside anyio `ExceptionGroup`s are unwrapped only when the group contains solely tool/protocol errors — cancellations grouped alongside are re-raised untouched (`pydantic_ai_slim/pydantic_ai/mcp.py:1420-1447`), with regression tests (`tests/test_mcp.py:734-853`). Operational level: initialization is atomic (state committed only after the handshake succeeds, `pydantic_ai_slim/pydantic_ai/mcp.py:1167-1250`), shutdown waits a bounded grace period before force-cancel (`pydantic_ai_slim/pydantic_ai/mcp.py:646-651`), stale-listing caches are invalidated by server notifications (`pydantic_ai_slim/pydantic_ai/mcp.py:1633-1642`), and unsupported features degrade to warnings instead of misbehavior (`pydantic_ai_slim/pydantic_ai/mcp.py:1211-1238`).

### 5. Can the same tool work across clients?

Strongly yes, at three levels. Protocol: everything speaks standard MCP through the FastMCP client, so any compliant server works, and pydantic-ai-built agents can themselves be served as MCP servers consumable by any client (`docs/mcp/server.md:11-60`). Configuration: `load_mcp_toolsets` consumes the de-facto-standard `mcpServers` JSON shape used by Claude Desktop, Cursor, and the MCP spec (`pydantic_ai_slim/pydantic_ai/mcp.py:1921-1927`). Provider: the `MCP` capability guarantees identical URL/auth/tool-filter semantics whether execution happens locally or natively at the provider, flipping automatically per model support (`pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:27-39,186-199`; strategy base `pydantic_ai_slim/pydantic_ai/capabilities/native_or_local.py:18-23`); a serialization-safe subset round-trips through `AgentSpec` via `from_spec` (`pydantic_ai_slim/pydantic_ai/capabilities/mcp.py:215-248`). Within a process, the same toolset instance can be shared across agents, and every toolset composes with the same wrappers (prefix/rename/filter/approval/deferral), keeping tool identity stable across wrappers and durable engines (`pydantic_ai_slim/pydantic_ai/toolsets/AGENTS.md` guidance; `abstract.py:91-96`).

## Architectural Decisions

- **Protocol integration delegated to FastMCP, semantics kept local.** `MCPToolset` wraps a normalized `fastmcp.Client` (`pydantic_ai_slim/pydantic_ai/mcp.py:1034-1045`) rather than reimplementing JSON-RPC/session management, while pydantic-ai owns caching, error policy, capability gating, and result mapping. This buys full protocol breadth (sampling, elicitation, OAuth, roots) at the cost of coupling to FastMCP's API surface and version generations.
- **Everything is a toolset.** By making `MCPToolset` just another `AbstractToolset` (`pydantic_ai_slim/pydantic_ai/mcp.py:716`), cross-cutting behavior (approval, filtering, prefixing, deferral, metadata) comes free through wrappers (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:184-281`) and applies uniformly to external and local tools.
- **Dual-surface MCP (local vs native) behind one capability.** `NativeOrLocalTool` removes whichever side the current model doesn't support (`pydantic_ai_slim/pydantic_ai/capabilities/native_or_local.py:18-23`), letting the same agent definition span providers with and without native MCP.
- **Deliberate delegation of argument validation to servers.** Using `TOOL_SCHEMA_VALIDATOR` (`pydantic_ai_slim/pydantic_ai/mcp.py:632-636`) avoids duplicating JSON-Schema validation client-side; the tradeoff is that the client cannot catch bad arguments before they leave the process.
- **Explicit SDK-generation compatibility layer.** Rather than pinning one MCP SDK, `_mcp_compat` reads fields by either naming scheme and detects v2 from the installed distribution (`pydantic_ai_slim/pydantic_ai/_mcp_compat.py:14-33`), with era-specific behavior branches and warnings in `__aenter__` (`pydantic_ai_slim/pydantic_ai/mcp.py:1175-1238`).

## Notable Patterns

- **Notification-driven cache invalidation**: a synthesized message handler runs ahead of any user handler and drops caches on `tools/resources/prompts list_changed` (`pydantic_ai_slim/pydantic_ai/mcp.py:1627-1644`), with cache disabling recommended for pre-built clients where the hook isn't installed (`pydantic_ai_slim/pydantic_ai/mcp.py:793-797`).
- **Atomic async initialization**: the exit stack is built inside a temporary `async with` and only `pop_all()`-ed into `self` once the handshake fully succeeds, preventing torn-down-session state leaks (`pydantic_ai_slim/pydantic_ai/mcp.py:1167-1250`).
- **Ref-counted lifecycle**: nested `__aenter__`/`__aexit__` pairs (implicit via `async with self:` inside `list_tools`/`direct_call_tool`) share one session until the outermost exit (`pydantic_ai_slim/pydantic_ai/mcp.py:1251-1268,1289-1293`).
- **Sentinel-based kwarg conflict detection**: `_UNSET` distinguishes "not passed" from passed-defaults so conflicts with pre-built clients stay correct as defaults evolve (`pydantic_ai_slim/pydantic_ai/mcp.py:708-712,991-1003`).
- **Environment-variable templating for portable configs**: `${VAR}` / `${VAR:-default}` expansion across the whole config tree, failing closed on undefined variables (`pydantic_ai_slim/pydantic_ai/mcp.py:1877-1918`).
- **Wire-faithful metadata preservation**: MCP `_meta` and annotations ride along on tool definitions and prompt content (`by_alias` camelCase pinned for public filters, `pydantic_ai_slim/pydantic_ai/mcp.py:1309-1319`).

## Tradeoffs

- **Breadth vs dependency weight**: full protocol support arrives only with the `[mcp]` extra; the slim install fails fast with an actionable message (`pydantic_ai_slim/pydantic_ai/mcp.py:42-51`), and in-process-server imports are separately guarded because `fastmcp-slim[client]` omits them (`pydantic_ai_slim/pydantic_ai/mcp.py:59-75`).
- **Client-side trust vs safety**: skipping JSON-Schema arg validation keeps the hot path simple and defers truth to the server, but means prompt-induced malformed calls cost a network round trip to detect, and no client-side defense exists against hostile argument shapes.
- **Single identity per toolset instance**: sharing one `MCPToolset` across concurrent runs amortizes connections but makes per-user credential injection impossible on the shared instance — solved socially (docs pattern: build per-run toolsets) rather than technically (`docs/mcp/client.md:467-500`).
- **Modern-session simplicity vs interactivity**: SEP-2575 stateless sessions refuse server-initiated sampling/elicitation; pydantic-ai warns loudly instead of silently dropping configured handlers (`pydantic_ai_slim/pydantic_ai/mcp.py:1207-1228`), preserving predictability at the cost of feature loss on newest stacks.
- **Config-file portability vs security**: accepting the ecosystem-standard `mcpServers` shape maximizes interoperability while requiring an explicit "trusted input only" warning because entries spawn executables and read env vars (`docs/mcp/client.md:218-220`).

## Failure Modes / Edge Cases

Tested and handled: tool errors under all three behaviors (`tests/test_mcp.py:620-650`), structured-content-preserving error payloads (`tests/test_mcp.py:668-732`), `ExceptionGroup` races between tool errors and stream teardown (`tests/test_mcp.py:734-853`), uninitialized clients (`tests/test_mcp.py:1464`), missing server info on modern sessions (`tests/test_mcp.py:1423`), servers lacking prompts/resources capabilities (`tests/test_mcp.py:924,1054`), undefined env vars in configs (`tests/test_mcp.py:1805`), invalid config roots/entries (`tests/test_mcp.py:1814-1830`), task calls on legacy sessions or without `fastmcp-tasks` installed (`tests/test_mcp.py:2110-2121`).

Known soft spots: (1) hung stdio subprocesses bound cleanup at ~3s per phase but still abandon the transport (`pydantic_ai_slim/pydantic_ai/mcp.py:646-651`); (2) `ResourceLink` tool results degrade to URI strings because reading requires a session reference (`pydantic_ai_slim/pydantic_ai/mcp.py:1813-1816`); (3) tool-result annotations/`_meta` intentionally do not propagate to `BinaryContent.vendor_metadata` (asymmetry documented at `pydantic_ai_slim/pydantic_ai/mcp.py:1788-1792`); (4) a shared toolset's first-run credentials silently apply to overlapping runs (`docs/mcp/client.md:471-473`).

## Future Considerations

- Wire `_ssrf`-style private-network checks (or an opt-in allowlist) into `_build_transport` for URL inputs, closing the gap between download protection and MCP connection protection (`pydantic_ai_slim/pydantic_ai/_ssrf.py:1-5` vs `pydantic_ai_slim/pydantic_ai/mcp.py:1647-1693`).
- Offer optional client-side validation of model-supplied arguments against the server's `inputSchema` (e.g., via `TypeAdapter` from the imported schema) to fail fast before network I/O; the plumbing point is `tool_for_tool_def` (`pydantic_ai_slim/pydantic_ai/mcp.py:1327-1339`).
- Consider surfacing MCP server trust posture (capabilities, auth mode) in observability output, building on existing instrumentation hooks (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:392-393` already exports tool schemas to OTel).

## Questions / Gaps

- **No evidence found** for OpenAPI/gRPC/plugin-manifest tool generation: searches for `openapi`/`OpenAPI` across `pydantic_ai_slim` returned only unrelated realtime/profile matches; the external-tool protocol surface is exclusively MCP plus provider-native MCP relays.
- **No evidence found** for sandboxed execution of external tools (process/container isolation): stdio servers run as ordinary child processes with caller-supplied command/args/env (`pydantic_ai_slim/pydantic_ai/mcp.py:1960-1966`); isolation is left to the operator. What exists instead is permissioning (allowlists, approval wrappers) and documentation-level trust warnings.
- No dedicated OAuth implementation lives in this repo — `'oauth'` delegates to FastMCP's flow (`pydantic_ai_slim/pydantic_ai/mcp.py:952-953`, `docs/mcp/client.md:465`); the repo's own tests exercise bearer/httpx.Auth shapes rather than full OAuth handshakes, so end-to-end OAuth behavior is unverified within this source.
- Rate limiting/quota controls for external tool calls were not found; retry budgets exist (`max_retries`, `pydantic_ai_slim/pydantic_ai/mcp.py:772-776`) but no concurrency caps specific to MCP servers.

---

Generated by `Dimension 04.07: External Tool Protocols and MCP Integration` against `pydantic-ai`.
