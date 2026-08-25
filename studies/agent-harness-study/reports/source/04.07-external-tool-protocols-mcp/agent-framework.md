# Source Analysis: agent-framework

## Dimension 04.07: External Tool Protocols and MCP Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Multi-language monorepo: Python (primary, deep MCP stack), .NET (`Microsoft.Agents.AI.Mcp` + declarative/hosting packages), Go (README stub only) |
| Analyzed | 2026-08-23 |

All evidence citations below are workspace-relative paths under `studies/agent-harness-study/sources/agent-framework/`.

## Summary

The framework treats external tools as a first-class protocol concern, not an afterthought. The Python core ships a full MCP client stack in one module — base `MCPTool` plus stdio, streamable-HTTP/SSE, and WebSocket transports (`python/packages/core/agent_framework/_mcp.py:424`, `:2728`, `:2927`, `:3250`) — that converts remote `tools/list` schemas into local `FunctionTool`s, filters arguments through a schema-derived allowlist, and gates execution behind approval modes and deny-by-default sampling. Provider-side "hosted" MCP is exposed uniformly via a `SupportsMCPTool` protocol implemented by OpenAI, Anthropic, Gemini, and Foundry clients (`python/packages/core/agent_framework/_clients.py:757-785`). On the server side, the dedicated `hosting-mcp` package exposes agents and workflows as native MCP tools while explicitly refusing to own transport, auth, or session-key policy (`python/packages/hosting-mcp/AGENTS.md`, `python/packages/hosting-mcp/agent_framework_hosting_mcp/_agent_tool.py:22`). The .NET side mirrors this with the `Microsoft.Agents.AI.Mcp` package (MCP Tasks extension support, `dotnet/src/Microsoft.Agents.AI.Mcp/McpClientTaskExtensions.cs:17`), declarative-YAML MCP invocation with hardened HTTP clients (`dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/DefaultMcpToolHandler.cs:191-248`), and consent-gated Foundry toolboxes (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ConsentAwareMcpClientAIFunction.cs:28`). The dominant design stance is that MCP servers are untrusted third parties: every path — sampling, tool exposure, argument forwarding, headers, redirects — carries an explicit trust boundary with a default-deny posture. OpenAPI support exists but is thinner: it is represented declaratively (`python/packages/declarative/agent_framework_declarative/_models.py:819`) or delegated to provider-side execution (`dotnet/src/Microsoft.Agents.AI.Foundry/FoundryAITool.cs:45`), with no generic local OpenAPI-to-tool generator in core.

## Rating

**8 / 10** — Clear model with extensive tests, explicit interfaces, and operational safeguards. The MCP integration is exceptionally mature: three transports, protocol extensions (SEP-2663 long-running tasks at `python/packages/core/agent_framework/_mcp.py:348`; SEP-2640 skills-over-MCP at `python/packages/core/agent_framework/_skills.py:4863`), OTel spans per RPC (`python/packages/core/agent_framework/_mcp.py:1853`), failure-mode-specific tests including double-execution prevention, and defense-in-depth against header leaks and redirect hijacking. It falls short of 9–10 because several pieces are explicitly experimental (progressive disclosure, `SecureMCPToolProxy`), the hosted-vs-local split creates two divergent security postures for the same protocol, OpenAPI is pass-through rather than locally generated, and the Go implementation is absent from this repo (`go/README.md:1-3`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP client base class | `MCPTool` owns session lifecycle, reconnect locks, tool/prompt loading | python/packages/core/agent_framework/_mcp.py:424-586 |
| Transport: stdio | `MCPStdioTool` wraps `mcp.client.stdio.stdio_client` with command/args/env | python/packages/core/agent_framework/_mcp.py:2728-2924 |
| Transport: streamable HTTP/SSE | `MCPStreamableHTTPTool` with custom `http_client`, origin-scoped `header_provider` | python/packages/core/agent_framework/_mcp.py:2927-3095 |
| Transport: WebSocket | `MCPWebsocketTool` documented as third subclass | python/packages/core/agent_framework/_mcp.py:3250-3253 |
| Schema conversion | `load_tools()` maps each server tool's `inputSchema` into `FunctionTool(input_model=...)`; normalizes missing `properties` for non-conforming servers | python/packages/core/agent_framework/_mcp.py:1876-1936 |
| Argument allowlist | `_prepare_call_kwargs` forwards only declared `inputSchema.properties` + configured extras; `_MCP_FRAMEWORK_DENYLIST` blocks non-serializable framework kwargs | python/packages/core/agent_framework/_mcp.py:2166-2207, :115-120 |
| Tool-name allowlist | `allowed_tools` restricts exposure by raw remote name; collisions raise instead of shadowing | python/packages/core/agent_framework/_mcp.py:1906-1915; tests :133, :1661 |
| Approval modes | `MCPSpecificApproval` (always/never per tool); `_determine_approval_mode` resolves per-call; ambiguous names resolve to require-approval | python/packages/core/agent_framework/_mcp.py:77-88, :1704-1718 |
| Sampling guardrails | Default-deny `sampling_callback`: per-session rate limit (25 default), approval gate, `maxTokens` cap (4096 default) | python/packages/core/agent_framework/_mcp.py:137-138, :1477-1548 |
| Header/trust boundary | Origin-scoped `header_provider` prevents token leak on cross-origin redirects; explicit warning that custom `http_client` must enforce the same policy | python/packages/core/agent_framework/_mcp.py:3041-3067 |
| Long-running tasks (SEP-2663) | `MCPTaskOptions`; `call_tool_as_task` polls `tasks/get`, refuses to re-issue submit after connection loss to avoid double execution | python/packages/core/agent_framework/_mcp.py:348-360, :2209-2295 |
| Progressive disclosure | Experimental loader tools `list_mcp_tools`/`load_tool`/`unload_tool`; filtered tools not listed or loadable | python/packages/core/agent_framework/_mcp.py:93-95, :530-538 |
| IFC labeling of MCP results | `SecureMCPToolProxy` auto-labels tools UNTRUSTED by default; write tools treated as sinks; documents that hosted MCP bypasses local middleware | python/packages/core/agent_framework/security.py:3324-3419, :3300-3321 |
| Hosted MCP protocol | `SupportsMCPTool` runtime-checkable protocol with `get_mcp_tool(**kwargs)` | python/packages/core/agent_framework/_clients.py:757-785 |
| Hosted MCP (OpenAI) | `OpenAIChatClient.get_mcp_tool` builds Responses-API `mcp` item executed remotely by OpenAI, with `approval_mode`/`allowed_tools`/headers | python/packages/openai/agent_framework_openai/_chat_client.py:1248-1319 |
| Hosted MCP (Anthropic/Gemini/Foundry) | Equivalent `get_mcp_tool` static methods on provider clients | python/packages/anthropic/agent_framework_anthropic/_chat_client.py:480; python/packages/gemini/agent_framework_gemini/_chat_client.py:469; python/packages/foundry/agent_framework_foundry/_chat_client.py:654 |
| Agent-as-MCP-server adapter | `AgentMCPTool` generates native MCP `Tool` from an agent, converts args/results, optional session persistence; no server/transport/auth owned | python/packages/hosting-mcp/agent_framework_hosting_mcp/_agent_tool.py:22-183 |
| Workflow-as-MCP-tool | `WorkflowMCPTool` derives schema from workflow start-executor input type | python/packages/hosting-mcp/agent_framework_hosting_mcp/_workflow_tool.py:23 |
| Declarative MCP config | YAML `McpTool` model with `connection`, `approvalMode` (always/never/specify), `allowedTools`, `url`; pluggable `MCPToolHandler` | python/packages/declarative/agent_framework_declarative/_models.py:781-816, :747-778; _mcp_handler.py:114-157 |
| Declarative OpenAPI | `OpenApiTool` declarative model (specification + connection) parsed from YAML/JSON | python/packages/declarative/agent_framework_declarative/_models.py:819-840 |
| .NET MCP Tasks extension | `ListAgentToolsWithTasksAsync` wraps `McpClientTool` as `AIFunction` with validated polling/cancellation bounds | dotnet/src/Microsoft.Agents.AI.Mcp/McpClientTaskExtensions.cs:17-139 |
| .NET hardened transport | Client cache keyed by (URL, label, connection, headers hash); cookies disabled, auto-redirect off, `OriginPinningHandler`, StreamableHttp forced over AutoDetect to block server-advertised SSE endpoint hijack | dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/DefaultMcpToolHandler.cs:184-248 |
| .NET consent flow | `ConsentAwareMcpClientAIFunction` intercepts protocol error -32006, cancels loop, surfaces `mcp_approval_request` | dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ConsentAwareMcpClientAIFunction.cs:49-69 |
| Protocol tests (Python) | 196 async tests in core MCP suite: sampling deny-by-default (:2662), rate limiting (:2804), maxTokens clamp (:2748), progressive disclosure suite (:1833-2265), header-provider cross-origin skip (:6258), task lifecycle fallbacks (:6897-6926), meta precedence (:5534-5775) | python/packages/core/tests/core/test_mcp.py |
| Protocol tests (.NET) | Dedicated unit-test project with `InMemoryMcpServerFixture`; ~20 failure-mode tests incl. stuck polls, input limits, local cancellation → remote cancel | dotnet/tests/Microsoft.Agents.AI.Mcp.UnitTests/TaskAwareMcpClientAIFunctionTests.cs:20-712; InMemoryMcpServerFixture.cs:24 |
| Observability tests | Dedicated MCP OTel test module | python/packages/core/tests/core/test_mcp_observability.py:1 |

## Answers to Dimension Questions

**1. Can tools live outside the process?**
Yes, in two distinct models. (a) *Locally proxied*: the application hosts an MCP client connection over stdio, streamable HTTP/SSE, or WebSocket (`python/packages/core/agent_framework/_mcp.py:2728`, `:2927`, `:3250`) and the remote server's tools are converted to local `FunctionTool`s executed by the app's function-calling loop. (b) *Provider-hosted*: the MCP server URL is handed to the model provider, which calls the server remotely (`python/packages/openai/agent_framework_openai/_chat_client.py:1257-1265` states this explicitly). Server-side, agents/workflows can themselves be published as MCP tools via adapters (`python/packages/hosting-mcp/agent_framework_hosting_mcp/_agent_tool.py:22`).

**2. Are external tools trusted by default?**
No. Four independent default-deny mechanisms: (a) MCP sampling requests are denied unless an approval callback is supplied, framed as confused-deputy risk (`python/packages/core/agent_framework/_mcp.py:1488-1496`, test at `python/packages/core/tests/core/test_mcp.py:2662`); (b) tool execution is subject to `approval_mode`, where unspecified per-tool names fall back to requiring approval semantics in the function layer (`python/packages/core/agent_framework/_mcp.py:1704-1718`); (c) the IFC proxy labels unannotated MCP tools `IntegrityLabel.UNTRUSTED` (`python/packages/core/agent_framework/security.py:3388`); (d) only declared schema parameters are forwarded to servers (`python/packages/core/agent_framework/_mcp.py:2172-2199`). However, tool *exposure* defaults to permissive: without `allowed_tools`, every advertised tool becomes callable (`python/packages/core/agent_framework/_mcp.py:2818-2820`) — trust is enforced at call time, not discovery time.

**3. How are schemas imported?**
Automatically during `tools/list` pagination: each tool's JSON `inputSchema` becomes the `input_model` of a generated `FunctionTool` (`python/packages/core/agent_framework/_mcp.py:1926-1936`), with a normalization guard adding empty `properties` for object schemas that omit them (OpenAI rejects those with 400) and tolerating non-conforming `inputSchema=None` (`:1884-1891`). Prompts get the same treatment via `_get_input_model_from_mcp_prompt` (`:1786`). Result parsing is invertible too: `parse_tool_results` converts `CallToolResult` back into framework `Content` (`:2796-2801`). In .NET, `JsonSchema`/`ReturnJsonSchema` are projected straight from `McpClientTool` onto `AIFunction` (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ConsentAwareMcpClientAIFunction.cs:43-45`).

**4. How are failures isolated?**
Layered. Exceptions are wrapped into `ToolExecutionException` with inner exceptions (`python/packages/core/agent_framework/_mcp.py:2249-2260`). Dropped connections trigger exactly one transparent reconnect-and-retry during `tools/list`/`prompts/list` (`:1856-1871`). The long-running-task path distinguishes abandonment (may still be running → best-effort `tasks/cancel`) from terminal failure (never cancel), and refuses to re-issue a submit whose outcome is unknown, preventing double execution of side-effecting tools (`:2243-2246`, `:2277-2295`). Per-session sampling state resets on reconnect (`:1526-1537`). A malformed success payload deliberately does *not* fall back to plain `tools/call` to avoid double execution (`python/packages/core/tests/core/test_mcp.py:6926` context). The .NET mirror cancels remote tasks on handler failure, local cancellation, stuck polls, and input-request limits (`dotnet/tests/Microsoft.Agents.AI.Mcp.UnitTests/TaskAwareMcpClientAIFunctionTests.cs:377-712`).

**5. Can the same tool work across clients?**
Mostly yes, with one split. Local `MCPTool` instances are chat-client-agnostic — they wrap any `SupportsChatGetResponse` (used for sampling) and plug into any agent's `tools=` list. Hosted MCP is normalized behind the `SupportsMCPTool` protocol (`python/packages/core/agent_framework/_clients.py:757-785`) implemented by OpenAI, Anthropic, Gemini, and Foundry clients, so user code is uniform even though the wire representation is provider-specific (Responses API `mcp` items vs. Gemini `McpServer` entries vs. Anthropic containers). The same MCP server definition is also portable into declarative YAML workflows in both languages (`python/packages/declarative/agent_framework_declarative/_models.py:781`; `dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/DefaultMcpToolHandler.cs:79-84`).

## Architectural Decisions

1. **One module owns the entire client-side protocol surface.** All transports, sampling, tasks, progressive disclosure, and meta handling live in a single 3455-line file (`python/packages/core/agent_framework/_mcp.py`), keeping cross-cutting concerns (allowlist filtering, meta precedence, OTel) consistent across transports.
2. **Adapters, not platforms, on the server side.** `hosting-mcp` deliberately provides no server, routes, auth, or session policy — applications compose the adapter with native SDK constructs (`python/packages/hosting-mcp/AGENTS.md`). This avoids reimplementing transport security but pushes operational burden to apps.
3. **Two execution topologies with different trust models.** Hosted MCP executes on the provider's infrastructure and bypasses local security middleware entirely — stated as a documented contrast in `SecureMCPToolProxy` docs (`python/packages/core/agent_framework/security.py:3354-3360`).
4. **Schema-derived allowlists over denylists.** Argument forwarding is bounded by what the server declares (`_prepare_call_kwargs`, `python/packages/core/agent_framework/_mcp.py:2172-2199`); the internal `_MCP_FRAMEWORK_DENYLIST` (`:115-120`) exists only as a safety net for declared-but-non-serializable names.
5. **Protocol extensions tracked upstream.** SEP-2663 tasks (`MCPTaskOptions`, frozen dataclass at `python/packages/core/agent_framework/_mcp.py:348`) and SEP-2640 skills index (`skill://index.json`, `python/packages/core/agent_framework/_skills.py:4863`) show the implementation tracks the evolving MCP spec rather than freezing on core protocol.
6. **Delegating OpenAPI to providers.** OpenAPI tools are declarative metadata or provider-executed (`FoundryAITool.CreateOpenApiTool` delegates to `ProjectsAgentTool.CreateOpenApiTool`, `dotnet/src/Microsoft.Agents.AI.Foundry/FoundryAITool.cs:45-46`), avoiding a second local HTTP-calling engine.

## Notable Patterns

- **ContextVar-based per-request auth injection**: runtime kwargs feed a `header_provider`; headers travel via a `contextvars.ContextVar` (`_mcp_call_headers`, `python/packages/core/agent_framework/_mcp.py:126`) and are attached only to same-origin requests, tested extensively incl. concurrent-call serialization (`python/packages/core/tests/core/test_mcp.py:6532`).
- **Name normalization with collision detection**: remote names are sanitized/prefixed (`_normalize_mcp_name`, `:188`); two remote tools mapping to one local name raise instead of silently shadowing (`:1906-1912`), and `allowed_tools` cannot be spoofed via normalized aliases (tests at `python/packages/core/tests/core/test_mcp.py:133`, `:212`).
- **Defense-in-depth in .NET HTTP handling**: cookies disabled, auto-redirect disabled, certificate revocation checked, origin-pinning `DelegatingHandler`, and forcing StreamableHttp over AutoDetect specifically because the legacy SSE transport trusts a server-advertised message endpoint (`dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/DefaultMcpToolHandler.cs:208-242`).
- **In-memory MCP server fixture**: .NET tests spin up a real in-process MCP server rather than mocking the client (`dotnet/tests/Microsoft.Agents.AI.Mcp.UnitTests/InMemoryMcpServerFixture.cs:24`), exercising the actual protocol.
- **Consent-as-protocol-error**: the Foundry toolbox surfaces human consent by catching protocol error -32006, cancelling the tool loop, and emitting an approval request (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ConsentAwareMcpClientAIFunction.cs:53-68`).
- **Feature-usage telemetry**: `mark_feature_used(FeatureIndex.HOSTING_MCP)` / `CoreMcp` instrument adoption (`python/packages/hosting-mcp/agent_framework_hosting_mcp/_agent_tool.py:91`; `dotnet/src/Microsoft.Agents.AI.Mcp/McpClientTaskExtensions.cs:47`).

## Tradeoffs

- **Hosted vs. local MCP splits the security perimeter.** Hosted tools gain provider-side convenience but escape local middleware, approval interception, and IFC labeling entirely (`python/packages/core/agent_framework/security.py:3354-3360`). The framework documents this rather than preventing it.
- **The allowlist trusts the server's schema.** Because `declared` comes from `inputSchema.properties`, a malicious server widens its own argument surface simply by declaring properties — runtime kwargs from `function_invocation_kwargs` are forwarded whenever the name matches (`python/packages/core/agent_framework/_mcp.py:2176-2185`). This is candidly documented as "the server selects which runtime kwarg names it receives," including guidance to keep credentials out of shared kwargs (`python/packages/core/agent_framework/_mcp.py:3084-3094`).
- **Experimental features inside the critical path.** Progressive disclosure warns and is gated (`python/packages/core/agent_framework/_mcp.py:530-538`); `SecureMCPToolProxy`/FIDES is experimental (`python/packages/core/agent_framework/security.py:3324`). Users needing the strongest isolation use APIs not yet stable.
- **Server-side hosting thinness is a feature and a cost.** App owners must derive trusted session IDs and serialize concurrent session calls themselves (`python/packages/hosting-mcp/AGENTS.md`, boundary section).
- **Single-file concentration.** ~3500 lines in `_mcp.py` concentrates risk of regression in one module; mitigated by the 196-test suite and the mandated spec-driven validation process (`studies/.../python/AGENTS.md` function-loop section).

## Failure Modes / Edge Cases

- **Non-conforming servers**: `inputSchema` missing `properties` or null (`python/packages/core/agent_framework/_mcp.py:1884-1891`); servers ignoring task augmentation fall back to plain `tools/call` (`:2262-2272`); `METHOD_NOT_FOUND`/`INVALID_PARAMS` rejection of augmented calls also falls back (tests `python/packages/core/tests/core/test_mcp.py:6897-6912`).
- **Connection loss mid-operation**: one reconnect attempt per page load (`:1856-1866`); unknown task state after lost submit raises instead of retrying (`:2243-2256`); transient 408 during poll retries within deadline.
- **Header leakage vectors**: cross-origin redirects handled by origin-scoped injection (test `python/packages/core/tests/core/test_mcp.py:6258`); custom `http_client` users explicitly warned they inherit responsibility (`python/packages/core/agent_framework/_mcp.py:3045-3053`).
- **Sampling abuse**: capped at 25 requests/session and 4096 tokens/request by default (`python/packages/core/agent_framework/_mcp.py:137-138`); callback exceptions deny rather than fail open (test `:2699`).
- **Duplicate/shadowed tools**: collision raises `ToolExecutionException` (`:1906-1912`); loader-name collisions in progressive mode omit the remote tool and point callers at prefixing (test `:2207`).
- **Credential isolation in .NET**: cookie jars cannot cross cache keys; redirect-based exfiltration blocked at three layers (`dotnet/src/Microsoft.Agents.AI.Workflows.Declarative.Mcp/DefaultMcpToolHandler.cs:206-229`).
- **Malformed payloads after task completion** raise instead of cancel-or-fallback (tests `python/packages/core/tests/core/test_mcp.py:6882-6926`), preserving side-effect safety.

## Future Considerations

- Stabilize the experimental surfaces (progressive disclosure, FIDES `SecureMCPToolProxy`) or provide released equivalents, since these carry the strongest external-tool isolation guarantees.
- Close the hosted/local gap: offer a documented migration or warning when a hosted MCP server is used in flows that assume local middleware enforcement.
- Consider bounding server-declared argument names (e.g., an opt-in strict mode intersecting declared properties with an explicit caller allowlist) to counter schema-widening by untrusted servers.
- Add a generic local OpenAPI→tool generator if parity with MCP ergonomics is desired; today OpenAPI is declarative metadata or provider-executed only.
- Reconcile the deliberate Python/.NET divergence in skills-over-MCP archive handling (in-memory unpacking vs. disk extraction noted in `python/packages/core/AGENTS.md`, MCPSkillsSource section) to keep cross-language behavior predictable.

## Questions / Gaps

- **Plugin manifests (generic)**: No evidence found for a general plugin/extension manifest format beyond MCP hosting manifests (`python/samples/04-hosting/foundry-hosted-agents/responses/mcp/agent.manifest.yaml`) and declarative agent YAML. Search covered `packages/*` for "plugin", "manifest", "openapi", and MCP symbols; only declarative tool models and Foundry hosting manifests matched.
- **OAuth flows for MCP servers**: No built-in OAuth authorization-code flow was found for `MCPStreamableHTTPTool`; auth is delegated to `header_provider`, custom `http_client`, or static `headers` (samples demonstrate API-key patterns, e.g., `python/samples/02-agents/mcp/mcp_api_key_auth.py:38-59`). If OAuth support exists, it lives outside this source.
- **Sandboxing of external processes**: Stdio MCP servers are spawned as ordinary subprocesses with caller-supplied command/env (`python/packages/core/agent_framework/_mcp.py:2910-2924`); no OS-level sandbox (containers, jobs, seccomp) is applied by the framework itself. Isolation relies on the host environment, consistent with the "adapters own nothing" boundary, but this means rule-of-thumb sandboxing claims should not be made.
- **Scale evidence**: Load/concurrency behavior is addressed indirectly (connection lifecycle locks at `python/packages/core/agent_framework/_mcp.py:554-558`, serialized header-provider calls tested at `python/packages/core/tests/core/test_mcp.py:6532`), but no benchmarks or scale tests were found in-repo.

---

Generated by dimension `04.07-external-tool-protocols-and-mcp-integration` against `agent-framework`.
