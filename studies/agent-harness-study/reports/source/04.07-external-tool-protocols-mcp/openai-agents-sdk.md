# Source Analysis: openai-agents-sdk

## Dimension 04.07: External Tool Protocols and MCP Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio, anyio, pydantic; `mcp` Python SDK v1/v2, httpx/httpx2) |
| Analyzed | 2026-08-23 |

> Citation convention: file paths below are relative to the selected source root (`studies/agent-harness-study/sources/openai-agents-sdk/`).

## Summary

The OpenAI Agents SDK (Python) treats MCP as a first-class external tool protocol with two distinct integration models: **local MCP client sessions** (`src/agents/mcp/`) where the SDK connects to servers over stdio, SSE, or Streamable HTTP and re-exposes each MCP tool as an internal `FunctionTool`; and **hosted MCP tools** (`HostedMCPTool`, `src/agents/tool.py:1087-1114`) that delegate listing/calling to OpenAI's Responses API. The local stack is engineered around failure isolation: exponential retries, isolated-session retry for broken Streamable HTTP sessions, credential-redacting logs and exceptions, pagination loop protection, deep-copied tool caches, and a lifecycle manager (`MCPServerManager`, `src/agents/mcp/manager.py:151`) with connect timeouts, parallel workers, and reconnect. Trust is not assumed silently: per-server `require_approval` policies (string/bool/map/grouped-list/callable) gate tool execution as human-in-the-loop approvals, dynamic/static tool filters fail closed, and hosted MCP calls are additionally checked against `allowed_callers`. Schema import converts MCP `inputSchema` to `FunctionTool.params_json_schema`, with optional best-effort strict-mode conversion via `Agent.mcp_config["convert_schemas_to_strict"]`. No OpenAPI-based tool generation or plugin-manifest mechanism exists — MCP is the sole external tool protocol.

## Rating

**Score: 9 / 10**

Rationale (per rubric band "Mature, durable, observable, extensible, and proven under failure or scale"):

- Clear model with explicit interfaces: abstract `MCPServer` contract (`src/agents/mcp/server.py:542-735`) plus three concrete transports and a hosted alternative.
- Operational safeguards are unusually deep: transport-error redaction that scrubs URLs from exception causes and logs (`src/agents/mcp/_logging.py:14-45`, `src/agents/mcp/server.py:181-236`), repeated-cursor pagination guards (`src/agents/mcp/server.py:1433-1474`), required-parameter pre-validation before remote invocation (`src/agents/mcp/server.py:1576-1608`), cache snapshotting so callers cannot mutate cached schemas (`src/agents/mcp/server.py:131-133`, applied at `1489`).
- Proven under failure/scale: ~10,800 lines of dedicated MCP tests across 20+ files in `tests/mcp/` (e.g., `tests/mcp/test_server_errors.py` at 1,608 lines, `tests/mcp/test_mcp_server_manager.py` at 1,282 lines), including retry, cancellation, cleanup-state, v2-compat, and auth-param coverage.
- Not a 10 because: strict schema conversion is best-effort and silently falls back (`src/agents/mcp/util.py:545-561`), resources support is only partially implemented by default (`NotImplementedError` in `src/agents/mcp/server.py:666-707`), external tools run with full process privileges unless the app opts into approvals (no default sandboxing of MCP tool side effects), and no non-MCP protocol (e.g., OpenAPI) exists.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP client abstraction | `MCPServer` ABC defines `connect/cleanup/list_tools/call_tool/list_prompts/get_prompt` contract; optional `list_resources/list_resource_templates/read_resource` raise `NotImplementedError` by default | `src/agents/mcp/server.py:542-707` |
| stdio transport | `MCPServerStdio` wraps `mcp.client.stdio.stdio_client`; params include command, args, env, cwd, encoding | `src/agents/mcp/server.py:1869-1966` |
| SSE transport | `MCPServerSse` wraps `sse_client` with url/headers/timeouts/auth/client-factory; docs mark SSE deprecated | `src/agents/mcp/server.py:2002-2114`; `docs/mcp.md:304-310` |
| Streamable HTTP transport | `MCPServerStreamableHttp` supports legacy v1 client and a custom v2 path; session-id capture/resumption via `Mcp-Session-Id` header documented on `session_id` property | `src/agents/mcp/server.py:2163-2303`, `2525-2554` |
| Hosted MCP tool | `HostedMCPTool` dataclass forwards `tool_config` (OpenAI Responses `Mcp` type imported from `openai.types.responses.tool_param`) to the API; optional `on_approval_request` callback | `src/agents/tool.py:42`, `1087-1114` |
| MCP SDK major-version compat | `MCP_MAJOR_VERSION` detected from installed distribution; `MCP_V2` flag switches field names (`input_schema` vs `inputSchema`, `next_cursor` vs `nextCursor`); dependency pinned `mcp>=1.19.0,<3` | `src/agents/mcp/_compat.py:21-31`, `101-126`; `pyproject.toml:16` |
| HTTP auth config | `params["auth"]` plumbed into SSE/HTTP transports; v2 requires `httpx2.Auth` else `UserError`; headers recommended over URL credentials | `src/agents/mcp/server.py:330-336`, `2089-2114`, `2252-2303`; `docs/mcp.md:45-53` |
| Approval policy normalization | `require_approval` accepts `"always"/"never"`, bool, per-tool map, grouped `{always:{tool_names},never:{tool_names}}` object (mirrors TS SDK), or callable; invalid values raise `UserError`; overlapping always/never names rejected | `src/agents/mcp/server.py:79-115`, `709-813` |
| Fail-closed callable approval | Callable policy without an agent returns `True` ("preserve the historical fail-closed behavior") | `src/agents/mcp/server.py:815-846` |
| Tool filtering (static/dynamic) | Static allow/block lists; dynamic callable receives `ToolFilterContext(run_context, agent, server_name)`; filter errors exclude the tool "for safety"; dynamic filter without context raises `UserError` | `src/agents/mcp/server.py:983-1067`; `tests/mcp/test_tool_filtering.py:163-220` |
| Schema conversion | `MCPUtil.to_function_tool` copies `inputSchema` into `params_json_schema`, injects empty `properties` (MCP spec doesn't require it, OpenAI does), optional `ensure_strict_json_schema` conversion on a separate copy with silent fallback | `src/agents/mcp/util.py:509-585` |
| Agent-level MCP config | `Agent.mcp_config` keys: `convert_schemas_to_strict` (default False), `failure_error_function`, `include_server_in_tool_names` | `src/agents/agent.py:148-167`, `250-267` |
| Tool name collision handling | Duplicate names across servers raise `UserError`; opt-in server-prefixed names `mcp_{server}__{tool}` sanitized, hashed to ≤64 chars, collision-free vs reserved/handoff names | `src/agents/mcp/util.py:63-64`, `305-335`, `416-506`; `src/agents/agent.py:211-262` |
| Tool origin tracking | Converted tools tagged `ToolOrigin(type=ToolOriginType.MCP, mcp_server_name=...)`; hosted items likewise | `src/agents/mcp/util.py:580-583`; `src/agents/run_internal/turn_resolution.py:3150-3161` |
| Output content mapping | Text blocks → text output; image blocks → base64 data URLs; other blocks JSON-serialized as text; opt-in `structuredContent` precedence with error-content carve-out | `src/agents/mcp/util.py:769-800` |
| Failure conversion | `failure_error_function` (server-level overrides agent-level; `_UNSET` sentinel) turns MCP failures into model-visible messages; `None` raises instead | `src/agents/mcp/server.py:565-573`, `848-854`; `src/agents/mcp/util.py:729-751` |
| Cancellation safety | Tool call runs in its own task; cancellation propagates and surfaces `MCPToolCancellationError` | `src/agents/mcp/util.py:706-728`; `src/agents/exceptions.py:497-498` |
| Retry/backoff | Configurable `max_retry_attempts`, exponential backoff with optional cap; `-1` means unlimited | `src/agents/mcp/server.py:163-179`, `1219-1239` |
| Isolated-session retry | Streamable HTTP retries a failed call on a fresh isolated session when shared session is broken (closed resource, connect/timeout, 5xx, MCP timeout/connection-closed) with explicit retry-budget accounting | `src/agents/mcp/server.py:2325-2457` |
| Credential-safe logging/errors | `get_mcp_server_log_name` strips userinfo/query/fragments from URLs; `_safe_transport_cause` retains only credential-safe HTTPX exceptions; request ops wrapped in `_run_request_with_transport_error_redaction` | `src/agents/mcp/_logging.py:14-45`; `src/agents/mcp/server.py:181-302`, `1176-1217` |
| Cache integrity | Cached tool lists returned as deep copies; dirty-flag invalidation via `invalidate_tools_cache()`; partial pagination results never exposed or cached | `src/agents/mcp/server.py:131-133`, `861-866`, `1083-1085`, `1428-1489` |
| Lifecycle manager | `MCPServerManager`: connect timeouts, drop-failed-servers, strict mode, reconnect(failed_only), parallel worker tasks preserving AnyIO task affinity, serialized lifecycle ops | `src/agents/mcp/manager.py:15-27`, `151-211`, `37-148` |
| Hosted approval flow | `McpApprovalRequest` outputs mapped to `MCPApprovalRequestItem`; missing callback logs that approvals surface as interruptions; manual approvals bridged into `mcp_approval_response` raw items with reason on rejection | `src/agents/run_internal/turn_resolution.py:3096-3124`; `src/agents/run_internal/tool_execution.py:1406-1512` |
| Hosted caller restriction | `allowed_callers` enforced for hosted MCP approval/list/call items; unknown caller types rejected; default allows only `direct` | `src/agents/run_internal/tool_caller.py:13-66`; `src/agents/run_internal/turn_resolution.py:3106-3149` |
| Tracing | `mcp_tools_span` wraps list-tools calls; `MCPListToolsSpanData` span type; function spans record `mcp_data.server` | `src/agents/tracing/create.py:466`; `src/agents/tracing/span_data.py:427`; `src/agents/mcp/util.py:814-823` |
| Protocol tests | 20+ test modules covering caching, retries, connect/disconnect, approvals, auth params, pagination integration, resources, manager lifecycle/cleanup state, tracing, util, v2 HTTP, version compat, message handler, prompt server, runner integration, server errors, streamable-http factory/session-id, tool filtering | `tests/mcp/test_caching.py:1`, `tests/mcp/test_client_session_retries.py:1`, `tests/mcp/test_mcp_approval.py:16-198`, `tests/mcp/test_mcp_v2_http.py:1`, `tests/mcp/test_runner_calls_mcp.py:1`, etc. |
| Examples | Runnable samples per transport plus hosted connectors/approvals | `examples/mcp/streamablehttp_example/`, `examples/mcp/sse_example/`, `examples/hosted_mcp/human_in_the_loop.py:1` |
| Docs tied to implementation | `docs/mcp.md` documents trust warning, transport matrix, v1/v2 compat table, approval forms, filters, caching, tracing — matching code cited above | `docs/mcp.md:12-14`, `18-27`, `29-55`, `253-271` |

## Answers to Dimension Questions

1. **Can tools live outside the process?** Yes, twice over. Local MCP servers run as separate subprocesses (`MCPServerStdio`, `src/agents/mcp/server.py:1869-1971`) or remote HTTP endpoints (`MCPServerStreamableHttp`/`MCPServerSse`, `src/agents/mcp/server.py:2002-2554`), with the SDK acting purely as an MCP client. Additionally, `HostedMCPTool` (`src/agents/tool.py:1087-1114`) moves the entire tool round-trip out of the user's process onto OpenAI's Responses infrastructure — no local connection is made at all (`docs/mcp.md:98-137`).

2. **Are external tools trusted by default?** Connection-wise, yes — any configured server's tools are listed and converted with no vetting beyond configuration, and docs explicitly warn "Trust MCP servers before connecting" (`docs/mcp.md:12-14`). Execution-wise, no: approval defaults to *not* required (`require_approval=None → False`, `src/agents/mcp/server.py:720-721`), but apps can enforce per-tool/per-server approval, static allowlists/blocklists, and dynamic filters; filter errors exclude the tool rather than include it (`src/agents/mcp/server.py:1054-1065`). For hosted MCP, calls from unexpected callers are rejected by default since `allowed_callers` defaults to `["direct"]` (`src/agents/run_internal/tool_caller.py:47-48`). So trust is delegated to explicit developer policy, with fail-closed mechanics once policy exists.

3. **How are schemas imported?** Each MCP tool's `inputSchema` is deep-copied into a `FunctionTool.params_json_schema`; an empty `properties` object is injected because the OpenAI spec requires what MCP doesn't (`src/agents/mcp/util.py:538-543`). Optional strict-mode conversion (`convert_schemas_to_strict`) applies `ensure_strict_json_schema` on a separate copy so a mid-conversion failure leaves the original usable schema intact, falling back silently (`src/agents/mcp/util.py:545-561`). Descriptions fall back to title when absent so tools never render blank (`src/agents/_mcp_tool_metadata.py:43-51`). Outputs are mapped back: text/image content blocks, JSON fallback for other block types, opt-in `structuredContent` (`src/agents/mcp/util.py:769-800`).

4. **How are failures isolated?** At four layers: (a) per-call retry with capped exponential backoff (`src/agents/mcp/server.py:1219-1239`); (b) isolated-session retry on Streamable HTTP when the shared session is unusable, charging retry budget deliberately (`src/agents/mcp/server.py:2325-2457`); (c) model-facing failure conversion via `failure_error_function`, with cancellation handled as a distinct `MCPToolCancellationError` and call tasks cancelled deterministically (`src/agents/mcp/util.py:706-762`); (d) process-level lifecycle isolation through `MCPServerManager` which drops failed servers, tracks `failed_servers`/`errors`, supports reconnect, and keeps connect/cleanup on one task for AnyIO cancel-scope safety (`src/agents/mcp/manager.py:151-211`, `113-148`). Transport errors are redacted so a failing server cannot leak credentials into logs or exception chains (`src/agents/mcp/server.py:181-302`).

5. **Can the same tool work across clients?** Yes — this is inherent to adopting MCP: the SDK is a standards-conformant client, so any compliant MCP server (filesystem, git, prompt servers shown in `examples/mcp/`) works unchanged, and any tool exposed here is reachable from other MCP clients too. Within the SDK, the same server instance can back multiple agents, and tool provenance is preserved via `ToolOrigin(type=MCP)` (`src/agents/mcp/util.py:580-583`) plus display metadata extraction used for both local and hosted paths (`src/agents/run_internal/turn_resolution.py:2725-2731`). The dual MCP-SDK-v1/v2 compat layer (`src/agents/mcp/_compat.py:21-98`) keeps the same public API working across dependency generations, aiding portability over time.

## Architectural Decisions

- **Two-tier MCP strategy (local client vs hosted delegation).** Local servers give full control inside the app boundary; `HostedMCPTool` pushes round-trips to OpenAI infra and intentionally shares no local `mcp` dependency requirements (`docs/mcp.md:55`). This trades latency/control for operational simplicity.
- **Adaptation layer instead of hard dependency pinning.** Rather than pinning one `mcp` major, the SDK ships a compat facade detecting the installed major and normalizing field names, cursors, error types, and HTTP stacks (`src/agents/mcp/_compat.py:21-191`), with targeted `UserError`s when a requested feature can't work on the installed major (auth types, `ignore_initialized_notification_failure`, `src/agents/mcp/server.py:330-354`, `2264-2270`).
- **MCP tools compiled to native `FunctionTool`s.** External tools are converted once per listing into the same tool representation as first-party function tools (`src/agents/mcp/util.py:509-585`), so the run loop needs no MCP-specific execution path — approvals, guardrails, and tracing apply uniformly. Provenance survives via `ToolOrigin`.
- **Fail-closed bias in policy machinery.** Callable approval without an agent ⇒ approval required (`src/agents/mcp/server.py:829-831`); filter exceptions drop the tool (`src/agents/mcp/server.py:1054-1065`); invalid `require_approval` shapes raise at construction (`src/agents/mcp/server.py:723-788`).
- **Credential hygiene as a cross-cutting concern.** Server names derived from URLs are sanitized everywhere they appear — logs, span data, error messages, exception causes (`src/agents/mcp/_logging.py:14-45`; `src/agents/mcp/server.py:181-236`, `1128-1217`) — reflecting the repo-level rule that redaction must cover tracebacks and chaining, not just display.
- **Task-affinity-preserving lifecycle management.** `MCPServerManager` serializes lifecycle ops and routes connect/cleanup through per-server worker tasks specifically because AnyIO cancel scopes require same-task teardown (`src/agents/mcp/manager.py:113-119`).

## Notable Patterns

- **Sentinel-default pattern**: `failure_error_function=_UNSET` distinguishes "not configured" from explicit `None` (raise) vs default (convert to message) (`src/agents/mcp/server.py:549`, `848-854`).
- **Snapshot-before-expose caching**: every externally visible tool list is deep-copied (`_snapshot_tools`) so dynamic filters or callers can't corrupt the cache or required-parameter validation (`src/agents/mcp/server.py:131-133`, `1041-1045`, `1486-1489`).
- **Pagination loop defense**: `seen_cursors` set detects repeating cursors and aborts without exposing partial results (`src/agents/mcp/server.py:1432-1474`).
- **Exception-group-aware error mapping**: BaseExceptionGroups are split into HTTP vs remaining parts; unsafe parts replaced with fixed-data stand-ins to preserve control semantics while dropping payloads (`src/agents/mcp/server.py:249-272`, `1190-1217`).
- **TS-parity policy shapes**: grouped `require_approval={"always":{"tool_names":[...]}, ...}` mirrors the TypeScript SDK surface (`docs/mcp.md:260-262`), easing cross-SDK portability of app configs.
- **Lazy exports to keep imports cheap**: `agents.mcp.__init__` defers heavy module imports until attribute access (`src/agents/mcp/__init__.py:32-83`).

## Tradeoffs

- **Best-effort strict schemas**: converting MCP schemas to strict mode can fail; the SDK silently serves the original non-strict schema (`src/agents/mcp/util.py:557-561`). Safe for availability, but a typo'd server schema degrades quality invisibly (logged only at info level).
- **Trust-by-configuration**: nothing verifies server identity or tool semantics; security depends entirely on app-supplied auth, filters, and approvals. The warning is documentation, not enforcement (`docs/mcp.md:12-14`).
- **No default sandboxing for MCP side effects**: unlike the separate code-execution sandbox subsystem (`src/agents/sandbox/`), an approved MCP tool executes with whatever privileges the server process has (e.g., stdio filesystem servers). Approval gates human oversight but not capability containment.
- **Per-run `list_tools()` latency vs staleness**: caching is opt-in per server (`cache_tools_list`, `src/agents/mcp/server.py:884-890`); default refetches every run, favoring freshness over latency.
- **Hosted MCP reduces observability/control**: tool selection happens remotely; the SDK sees only resulting items and approval requests (`src/agents/run_internal/turn_resolution.py:3096-3163`), and `on_approval_request=None` pauses the run as an interruption rather than deciding locally (`src/agents/run_internal/turn_resolution.py:3119-3124`).

## Failure Modes / Edge Cases

Handled explicitly (with tests):
- Broken shared Streamable HTTP session → isolated-session retry with budget accounting; exhausted budget re-raises root cause (`src/agents/mcp/server.py:2359-2457`; `tests/mcp/test_client_session_retries.py`).
- Server disconnect mid-call → `UserError("Connection lost...")` mapped from `ConnectError` (`src/agents/mcp/server.py:1497-1514`).
- Repeated pagination cursor → abort, clear partial data, raise (`src/agents/mcp/server.py:1444-1474`; `docs/mcp.md:479-483`).
- Missing required arguments caught client-side before network round-trip (`src/agents/mcp/server.py:1576-1608`).
- Non-dict / invalid JSON tool input → `ModelBehaviorError` (`src/agents/mcp/util.py:683-697`).
- Cancellation during tool call → child task cancelled, `MCPToolCancellationError` surfaced (`src/agents/mcp/util.py:711-728`).
- Cleanup failures during failed connect don't mask the original error; cancel-scope RuntimeErrors suppressed as known-noise (`src/agents/mcp/server.py:1369-1410`, `1779-1830`).
- Cross-server duplicate tool names → `UserError` suggesting `include_server_in_tool_names` (`src/agents/mcp/util.py:324-335`).
- Filter callback raising → tool excluded, identity-only logging in redacted mode (`tests/mcp/test_tool_filtering.py:163-220`).

Residual risks:
- A malicious/compromised MCP server controls its own tool descriptions and schemas; nothing sanitizes description text before it reaches the model (`src/agents/mcp/util.py:569`), leaving room for prompt-injection via tool metadata.
- `use_structured_content=True` trusts server `structuredContent` exclusively for successful calls (`src/agents/mcp/util.py:776-777`).
- Unlimited retries possible via `max_retry_attempts=-1` (`src/agents/mcp/server.py:1237`) — bounded only by backoff cap if configured.

## Future Considerations

- Add sanitization/size limits or provenance marking for server-provided descriptions/titles to blunt injection via tool metadata (`src/agents/_mcp_tool_metadata.py:28-40` is currently pass-through).
- Surface strict-schema conversion failures more loudly (e.g., per-tool flag or run-time warning) instead of info-level logs (`src/agents/mcp/util.py:557-561`).
- Complete the resources API for all transports (base class currently `NotImplementedError`, `src/agents/mcp/server.py:651-707`) so resource-backed context can be treated uniformly alongside prompts/tools.
- Consider an OAuth/OIDC discovery helper for Streamable HTTP auth; today callers must hand-assemble `httpx.Auth`/headers (`docs/mcp.md:45-53`).
- An OpenAPI→tool bridge would complement MCP for REST-first shops; none exists today (see Gaps).

## Questions / Gaps

- **OpenAPI tool generation:** No evidence found. Searched `openapi|OpenAPI` across `src/agents/` — only incidental hits (`src/agents/strict_schema.py:240` comment; `src/agents/extensions/models/litellm_model.py:150` LiteLLM docstring). REST APIs must be wrapped as MCP servers or function tools.
- **Plugin manifests:** No evidence found. No manifest discovery/loading mechanism exists in `src/`; extensibility is via Python subclasses of `MCPServer` and callbacks (`tool_filter`, `tool_meta_resolver`, `custom_data_extractor`, `src/agents/mcp/util.py:141-210`).
- **Transport set limited to spec transports:** stdio/SSE/Streamable HTTP only; WebSocket or custom IPC would require subclassing `create_streams` (`src/agents/mcp/server.py:1069-1074`). The SSE transport remains despite upstream deprecation, kept for legacy servers (`docs/mcp.md:306-308`).
- **Hosted MCP config typing is external:** `tool_config` uses the `openai` package's `Mcp` TypedDict (`src/agents/tool.py:42`), so schema validation of hosted options lives outside this repository; the SDK adds only `on_approval_request` and `allowed_callers` normalization (`src/agents/tool.py:1102-1110`).
- Unanswered: whether realtime sessions accept MCP servers directly; `src/agents/realtime/_tool_filtering.py` suggests filtering hooks exist but no `mcp_servers` field was found on the realtime agent within the search scope.

---

Generated by `04.07-external-tool-protocols-and-mcp-integration` against `openai-agents-sdk`.
