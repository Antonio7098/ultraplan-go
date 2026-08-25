# Source Analysis: openhands

## Tool Context and Dependency Injection

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python 3.12 + FastAPI + Pydantic v1/v2; tool execution delegated to external `openhands-sdk` (1.29.0) and `openhands-tools` (1.29.0) packages (`pyproject.toml:60-62, 249-251`) |
| Analyzed | 2026-07-27 |

## Summary

OpenHands treats tool context as a **two-layer problem**: (a) the **server-side** `AppConversationService` that *builds* the agent, and (b) the **agent-side** tool execution that happens inside a sandboxed `agent-server` process.

The repository itself does **not** define the agent tools (BashTool, FileEditorTool, BrowserTool, GlobTool, etc.). Those live in the external `openhands-tools` package. What this repo owns is the **injection machinery that hands tools their dependencies**: user identity, secrets, LLM config, MCP config, workspace, sandbox, callback services, JWT/permission tokens, and the per-request `httpx` client. Tools are wired through an `Injector[T]` ABC (`openhands/app_server/services/injector.py:12-34`) whose state is `starlette.datastructures.State` — i.e., per-FastAPI-request. The Injector produces typed `AsyncContextManager[T]` (`context()`) and FastAPI `Depends` (`depends()`) adapters (`openhands/app_server/services/injector.py:23-33`).

Tool **runtime context** for the LLM/agent call is a single `AgentContext(system_message_suffix=..., secrets=...)` Pydantic model passed at agent build time (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1635-1638`). Tools are then attached via `agent_settings.tools` (a list, not a registry), and the *list itself* is fetched from the external package via `get_default_tools(...)` / `get_planning_tools(...)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138, 1611-1625`). Secrets are abstracted as `SecretSource` — either `StaticSecret(value=SecretStr)` for OSS or `LookupSecret(url=..., headers={"X-Access-Token": jwt})` for SaaS, so raw secret values never traverse the wire (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1118-1142, 1789-1797`).

A **second, distinct context model** applies to MCP-server tools in `openhands/app_server/mcp/mcp_router.py:43, 147-487`. There, `get_http_request()` from `fastmcp.server.dependencies` is the only source of truth for user/provider tokens (`openhands/app_server/mcp/mcp_router.py:167-174, 240-247, 309-316, 375-382, 442-449`). The MCP tools do not receive an injected `ToolContext` — they are stateless functions decorated with `@mcp_server.tool()`.

**Testability is excellent for the service layer**: the unit test for `LiveStatusAppConversationService` constructs the service with `Mock(spec=UserContext)` plus 7 other mocks and never boots a real server (`tests/unit/app_server/test_live_status_app_conversation_service.py:159-199`). However, this is *constructor injection of the service*, not *injection into a tool* — tools themselves are not unit-tested in isolation, and the agent-facing tool classes are out of scope (external package).

The biggest gap is that **tool permission enforcement** is not a context-level concern. The repository only configures `ConfirmationPolicyBase` (NeverConfirm/AlwaysConfirm/ConfirmRisky) and `SecurityAnalyzerBase` (None or LLMSecurityAnalyzer) once at agent build time (`openhands/app_server/app_conversation/app_conversation_service_base.py:614-652`). There is no per-tool ACL, no per-user permission grant system, and no audit log of tool calls.

## Rating

**7/10**

Rationale:

- **Strengths**
  - **Clear, generic DI primitive** in `Injector[T]` (`openhands/app_server/services/injector.py:12-34`) — supports both `async with` (`context()`) and FastAPI `Depends` (`depends()`) call sites. State lives on `Request.state`, so dependencies can share resources within a request without globals (`openhands/app_server/services/httpx_client_injector.py:20-37`, `openhands/app_server/services/db_session_injector.py:285-323`).
  - **Tool runtime context is a typed, declarative object** — `AgentContext(system_message_suffix=..., secrets=...)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1635-1638`) and `LocalWorkspace(working_dir=project_dir)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1558, 1813`) are explicit, not implicit.
  - **Secrets are abstracted behind `SecretSource`** (`openhands/sdk/secret` is imported as `LookupSecret` / `StaticSecret` at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:122`). SaaS mode uses `LookupSecret` to a JWT-scoped webhook so the raw value never materialises in this process (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1118-1133, 1789-1797`).
  - **`UserContext` is a small, well-typed ABC** (`openhands/app_server/user/user_context.py:13-83`) with 9 explicit read methods (`get_user_id`, `get_user_email`, `get_user_info`, `get_authenticated_git_url`, `get_provider_tokens`, `get_latest_token`, `get_secrets`, `get_mcp_api_key`, `get_user_git_info`). Two implementations are provided: `AuthUserContext` for normal users and `SpecifyUserContext` for admin contexts (`openhands/app_server/user/auth_user_context.py:27-166`, `openhands/app_server/user/specifiy_user_context.py:13-65`).
  - **Per-request caching of `UserContext`** avoids re-authenticating on every dependency lookup (`openhands/app_server/user/auth_user_context.py:158-164`: `if user_context is None: ... setattr(state, USER_CONTEXT_ATTR, user_context)`).
  - **Constructor-injection style on `LiveStatusAppConversationService`** — every collaborator is a dataclass field with a Pydantic-typed `Injector` (e.g. `user_context: UserContext`, `sandbox_service: SandboxService`, `jwt_service: JwtService`, `httpx_client: httpx.AsyncClient`, `conversation_secret_enricher: ConversationSecretEnricher`) at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:203-226`. This makes unit tests trivial: no global state, no app boot, no DB.
  - **Conditional inclusion of `SwitchLLMTool`** is a clean example of a server-orchestrated tool that depends on runtime state (number of LLM profiles) (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1648-1664`).
  - **Multiple `*ServiceInjector` types are substitutable** (Discriminated Union pattern) so different deployments pick different backends (e.g., `DockerSandboxServiceInjector` vs `RemoteSandboxServiceInjector`) without rewriting the consumer (`openhands/app_server/config.py:215-225, 285-340`).

- **Limitations**
  - **The agent-facing tool classes are not in this repository.** The actual `BaseTool`/`ToolDefinition` classes (whatever their shape) are imported from `openhands.sdk` / `openhands.tools` (external). Therefore, the rating is graded on the *consumer* side, not the tool *base class* side. We can't verify that the tools themselves accept a typed `ToolContext` parameter.
  - **No formal `ToolContext` object** in this repo. Context flows as a constellation of typed parameters: `llm`, `tools`, `mcp_config`, `agent_context` (with embedded `secrets`), `workspace` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1630-1640`). Each tool has to know to consult the right collaborator. There is no `runtime = ToolContext(...)` handoff.
  - **No cancellation token.** Cancellation uses `asyncio.Task.cancel()` directly (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2411`, `openhands/app_server/sandbox/docker_sandbox_spec_service.py:129`, `openhands/app_server/utils/async_utils.py:76`). There is no `CancellationToken` abstraction that tools can check cooperatively.
  - **No logger injection.** The services use module-level `_logger = logging.getLogger(__name__)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:142`), so log context (request id, conversation id, user id) is not automatically attached.
  - **Permissions are not part of the tool context.** Confirmation policy and security analyzer are configured at the agent level (`openhands/app_server/app_conversation/app_conversation_service_base.py:614-652`), not per-tool. There is no per-tool ACL, no per-user permission grant registry, no audit log of tool invocations.
  - **MCP tools have an *inconsistent* context model.** They pull identity from `get_http_request()` (`openhands/app_server/mcp/mcp_router.py:167-174`), which is a different mechanism than the typed `UserContext` used elsewhere. The five PR/MR tools are not constructor-injectable; they read state at call time. Tests would have to mock `fastmcp.server.dependencies.get_http_request` rather than inject a context.
  - **No workspace context beyond a path.** `LocalWorkspace(working_dir=project_dir)` is just a wrapper around a path string (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1558`). There is no structured "workspace capabilities" object (read-only regions, file size limits, network policy).
  - **`web_url` / `access_token_hard_timeout` / `export_lock_*` are passed as plain parameters** to the service constructor (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:215-218, 2458-2476`). These look like config, not runtime context, but they are not bundled into a single `RuntimeConfig` object.
  - **No test fixture for tool-context injection.** The repository's tests verify service-level behavior (e.g. `test_build_request_passes_enable_sub_agents_true` at `tests/unit/app_server/test_live_status_app_conversation_service.py:1076-1118`) but never assert that the `AgentContext` object passed to `create_agent()` carries the right secrets/system_message_suffix combination. Secrets are tested only in `tests/unit/app_server/test_constants.py:1-163` (validation rules, not the end-to-end context build).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool context object | `AgentContext(system_message_suffix=..., secrets=...)` Pydantic model used as the explicit runtime context for the tool list. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1635-1638` |
| Tool context object | `LocalWorkspace(working_dir=project_dir)` carries workspace path into the agent; explicit per-conversation. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1558, 1813` |
| Tool list | Tools come from external `openhands.tools.preset.default` and `openhands.tools.preset.planning` via `get_default_tools()` and `get_planning_tools()`. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138, 1611-1625` |
| Tool list | `register_builtins_agents(enable_browser=True)` registers sub-agents, then `get_registered_agent_definitions()` returns them as `AgentDefinition` objects. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:134, 1619, 1624-1625` |
| Tool list | `SwitchLLMTool` is appended to `agent.include_default_tools` only when at least two valid LLM profiles are saved. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1648-1664` |
| Dependency providers | `Injector[T]` ABC with `inject()` (abstract), `context()` (async-with), `depends()` (FastAPI Depends). | `openhands/app_server/services/injector.py:12-34` |
| Dependency providers | `InjectorState: TypeAlias = State` — the per-FastAPI-request state bag used as the DI container. | `openhands/app_server/services/injector.py:9` |
| Dependency providers | `HttpxClientInjector` — single shared `httpx.AsyncClient` per request, cleaned up at end. | `openhands/app_server/services/httpx_client_injector.py:13-37` |
| Dependency providers | `DbSessionInjector` — shared `AsyncSession` per request, with `keep_open` flag for cross-request pooling. | `openhands/app_server/services/db_session_injector.py:28-323` |
| Dependency providers | `LiveStatusAppConversationServiceInjector` wires 10 collaborators via `async with` blocks at injection time. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2485-2516` |
| Dependency providers | `get_*_service` functions in `config.py` return `AsyncContextManager[T]` for each injector. | `openhands/app_server/config.py:452-519` |
| Dependency providers | `depends_*` functions return FastAPI `Depends(injector.depends)` for direct use as route dependencies. | `openhands/app_server/config.py:527-549` |
| User context | `UserContext` ABC declares 9 explicit read methods. | `openhands/app_server/user/user_context.py:13-83` |
| User context | `AuthUserContext` reads from `UserAuth`, exposes `get_user_id`, `get_user_email`, `get_user_info`, `get_authenticated_git_url`, `get_provider_tokens`, `get_latest_token`, `get_secrets`, `get_mcp_api_key`, `get_user_git_info`. | `openhands/app_server/user/auth_user_context.py:27-166` |
| User context | `AuthUserContextInjector` caches the resolved `UserContext` on request state to avoid re-resolution. | `openhands/app_server/user/auth_user_context.py:152-166` |
| User context | `SpecifyUserContext` is a frozen dataclass for admin (no-user) operations; `as_admin(request)` patches request state. | `openhands/app_server/user/specifiy_user_context.py:13-65` |
| User context | `USER_CONTEXT_ATTR = 'user_context'` — the conventional state attribute name, set by injectors. | `openhands/app_server/user/specifiy_user_context.py:51` |
| Secrets | `LookupSecret(url=..., headers={"X-Access-Token": jwt}, description=...)` used in SaaS so values never materialise in this process. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1118-1133` |
| Secrets | `StaticSecret(value=SecretStr(...), description=...)` fallback for OSS (no `web_url`). | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1134-1142` |
| Secrets | `access_token_hard_timeout` defaults to 14 days; controls JWT expiry for `LookupSecret` headers. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2458-2464, 1118-1126` |
| Secrets | `ConversationSecretEnricher` extension point lets integrations (GitHub, GitLab, Jira, Linear, Slack) inject per-trigger secrets. | `openhands/app_server/app_conversation/conversation_secret_enricher.py:21-40` |
| Secrets | `validate_secret_name` / `validate_secrets_dict` reject blocked env-var names and cap total byte length before they reach the agent. | `openhands/app_server/constants.py:97-160` |
| Constructor injection | `LiveStatusAppConversationService` is a `@dataclass` with 17 typed fields; every collaborator is explicit. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:199-226` |
| Test fixture | `setup_method` constructs the service with `Mock(spec=UserContext)` and 7 other mocks; no app boot, no DB, no HTTP. | `tests/unit/app_server/test_live_status_app_conversation_service.py:159-199` |
| Test fixture | `Mock(spec=UserContext)` pattern proves the test harness treats `UserContext` as a fully substitutable interface. | `tests/unit/app_server/test_live_status_app_conversation_service.py:165` |
| Test fixture | Patching `get_default_tools` to return `[]` lets tests isolate the request-building logic from real tool registration. | `tests/unit/app_server/test_live_status_app_conversation_service.py:1011-1017, 1075-1078, 1126-1129` |
| Test fixture | `@pytest.fixture(autouse=True)` sets `ALLOW_SHORT_CONTEXT_WINDOWS=true` so the SDK LLM validation accepts tiny models in tests. | `tests/unit/app_server/test_live_status_app_conversation_service.py:141-156` |
| Testability (MCP side) | No test mocks `get_http_request` for the MCP tools; their context model is untested. | `openhands/app_server/mcp/mcp_router.py:167-174` (search for tests) |
| Permissions | `_select_confirmation_policy` returns `NeverConfirm`, `AlwaysConfirm`, or `ConfirmRisky` based on `confirmation_mode` + `security_analyzer` strings. | `openhands/app_server/app_conversation/app_conversation_service_base.py:641-652` |
| Permissions | `_create_security_analyzer_from_string` produces `LLMSecurityAnalyzer` for "llm", `None` for "none" or unknown (logs warning). | `openhands/app_server/app_conversation/app_conversation_service_base.py:614-639` |
| Permissions | `_set_security_analyzer_from_settings` posts to `/api/conversations/{id}/security_analyzer` on the agent-server. | `openhands/app_server/app_conversation/app_conversation_service_base.py:654-703` |
| Sandbox context | `SandboxService`, `SandboxSpecService`, `EventService`, `EventCallbackService`, `PendingMessageService` are typed Injector-based services; selected via env var `RUNTIME` (`local` / `process` / `remote`). | `openhands/app_server/config.py:285-340` |
| Sandbox context | `AsyncRemoteWorkspace(host=agent_server_url, api_key=sandbox.session_api_key, working_dir=working_dir)` provides remote command exec / file I/O to the agent. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:380-384` |
| Hooks | `HookConfig` loaded from `agent-server /api/hooks` (not from a local registry), then attached to `ConversationSettings.hook_config`. | `openhands/app_server/app_conversation/hook_loader.py:43-100, 103-148` |
| Skills | `Skill` objects loaded from `agent-server /api/skills` (public + user + org + project), merged into `AgentContext.skills`. | `openhands/app_server/app_conversation/skill_loader.py:264-358` |
| Plugins | `PluginSource` passed via `ConversationSettings.plugins`; supports remote plugin load. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1694-1703, 1880-1898` |
| MCP tool context | `get_http_request()` reads `X-OpenHands-ServerConversation-ID` from headers (not injected). | `openhands/app_server/mcp/mcp_router.py:167-169, 240-242, 308-309, 374-376, 441-443` |
| MCP tool context | `get_provider_tokens`, `get_access_token`, `get_user_id` resolve identity from the inbound HTTP request via `get_user_auth(request)`. | `openhands/app_server/user_auth/__init__.py:13-43` |
| MCP tool context | `mcp_server = FastMCP('mcp', mask_error_details=True)` is constructed globally at import time. | `openhands/app_server/mcp/mcp_router.py:43` |
| MCP tool context | `init_tavily_proxy()` mounts a Tavily MCP proxy under namespace `'tavily'` so sandboxes can use Tavily search without an API key. | `openhands/app_server/mcp/mcp_router.py:49-76` |
| Cancellation | `task.cancel()` is used directly on background refresh tasks. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2411` |
| Cancellation | `logger_task.cancel()` cancels the docker log forwarder. | `openhands/app_server/sandbox/docker_sandbox_spec_service.py:129` |
| Logger | Module-level `_logger = logging.getLogger(__name__)` — no contextual log binding (no request id, no conversation id). | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:142` |
| Logger | `sanitize_config(hook_config.model_dump())` used to mask secrets before logging. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1683` |
| Discriminated union | Multiple `*ServiceInjector` types participate in a `DiscriminatedUnionMixin` so configuration can choose which one to instantiate. | `openhands/app_server/config_api/llm_model_service.py:60`, `openhands/app_server/event/event_service.py:73`, `openhands/app_server/sandbox/sandbox_service.py`, `openhands/app_server/app_conversation/app_conversation_service.py:191-194` |
| Discriminated union | `LiveStatusAppConversationServiceInjector` extends `AppConversationServiceInjector` so it can substitute in `get_app_conversation_service()`. | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:2443`, `openhands/app_server/config.py:484-489` |
| Webhook → context | Webhook callbacks run with `SpecifyUserContext(user_id)` set on a fresh `InjectorState`; `as_admin` pattern. | `openhands/app_server/event_callback/webhook_router.py:561-573` |
| Background tasks | Background export uses `state = InjectorState(); setattr(state, USER_CONTEXT_ATTR, SpecifyUserContext(user_id))` because no `Request` is available. | `openhands/app_server/mcp/mcp_router.py:100-110` |
| Cleanup | `httpx_client` and `db_session` are removed from state in the `finally` block of their injectors; other injectors rely on GC. | `openhands/app_server/services/httpx_client_injector.py:30-37`, `openhands/app_server/services/db_session_injector.py:317-323` |
| Cleanup | `KEEP_OPEN` flags allow callers to prevent auto-close when they take responsibility for the resource. | `openhands/app_server/services/httpx_client_injector.py:32-33, 40-42`, `openhands/app_server/services/db_session_injector.py:25-26, 311-322, 326-328` |

## Answers to Dimension Questions

### 1. What context does a tool receive?

A tool (in the agent-facing tool list) receives its context through the agent that wraps it, not as a per-call argument. Specifically:

- The `Agent` is constructed with `llm=llm, tools=tools, mcp_config=mcp_config, agent_context=AgentContext(system_message_suffix=..., secrets=secrets), include_default_tools=[...]` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1630-1664`).
- `AgentContext` carries (a) `system_message_suffix` for the prompt, (b) `secrets: dict[str, SecretSource]`, (c) `skills: list[Skill]`, and (d) any subclass-specific fields (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1635-1638, 184-187`).
- Tools themselves are external `BaseTool` instances from `openhands-tools`; the local repo never sees a per-tool method invocation, so we cannot describe the call-time context shape.
- MCP tools (the five `create_pr`/`create_mr` variants) receive no context parameter at all — they read identity from the inbound HTTP request at call time (`openhands/app_server/mcp/mcp_router.py:167-174, 240-247, 309-316, 375-382, 442-449`).

### 2. Is context explicit or global?

**Explicit at the service level, request-scoped for user identity, and stateless for MCP tools.**

- `LiveStatusAppConversationService` lists every collaborator as a typed dataclass field (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:203-226`). No global lookup; no `singleton.getInstance()` pattern.
- `UserContext` lives on `request.state` via `USER_CONTEXT_ATTR` (`openhands/app_server/user/specifiy_user_context.py:51`), and the injector caches it once per request (`openhands/app_server/user/auth_user_context.py:158-164`).
- DB sessions, httpx clients, JWT services are all stored on `request.state` so they're shared within a request but do not leak across requests (`openhands/app_server/services/httpx_client_injector.py:23-37`, `openhands/app_server/services/db_session_injector.py:301-323`).
- The `Injector[T]` ABC and the `*ServiceInjector` subclasses make the *factory* explicit (`openhands/app_server/services/injector.py:12-34`). The factory can be swapped via `AppServerConfig` (e.g., `DockerSandboxServiceInjector` vs `ProcessSandboxServiceInjector`) without touching the consumer.
- The MCP tools have no explicit context — they implicitly rely on `get_http_request()` from `fastmcp.server.dependencies` (`openhands/app_server/mcp/mcp_router.py:167-174`).

### 3. Are secrets passed safely?

**Yes, with two distinct strategies depending on deployment mode.**

- **SaaS / hosted mode**: each provider token (e.g. `GITHUB_TOKEN`) is wrapped in `LookupSecret(url=..., headers={"X-Access-Token": jwt}, description=...)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1118-1133`). The webhook URL is a `/api/v1/webhooks/secrets` endpoint that checks the `X-Access-Token` and returns the secret only if the token is valid and unexpired. The raw token value never enters the agent-server process.
- **OSS / no `web_url` mode**: `StaticSecret(value=SecretStr(...), description=...)` is used (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1134-1142`). The `SecretStr` wrapping prevents accidental log/print of the value.
- `access_token_hard_timeout` defaults to 14 days (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2458-2464`), so even stolen JWTs become useless after 2 weeks.
- `validate_secret_name` / `validate_secrets_dict` reject blocked env-var names (e.g. `LLM_*`) and cap the dict size before the secrets reach the agent (`openhands/app_server/constants.py:97-160`).
- `sanitize_config(...)` is called before logging any `HookConfig` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1683`).
- The ACP path explicitly *forbids* `LookupSecret` with JWT headers because the SDK redacts anything matching `SECRET_KEY_PATTERNS` (which includes `TOKEN`), so ACP secrets must be `StaticSecret` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1815-1819`).

### 4. Can tools be unit tested?

**Indirectly. The *service* that hands tools their context is unit-tested; the *tools themselves* are not, because they live in the external `openhands-tools` package.**

- `tests/unit/app_server/test_live_status_app_conversation_service.py:159-199` constructs `LiveStatusAppConversationService` with `Mock(spec=UserContext)` plus 7 other mocks. No app boot, no DB, no HTTP.
- `@patch('...get_default_tools', return_value=[])` lets the test verify the request builder logic without invoking the external tool registry (`tests/unit/app_server/test_live_status_app_conversation_service.py:1011-1017, 1075-1078, 1126-1129`).
- `Mock(spec=UserContext)` ensures the test can only call methods declared on the ABC, providing a structural-typing guarantee (`tests/unit/app_server/test_live_status_app_conversation_service.py:165`).
- The Injector pattern itself is testable: `Injector.depends(request)` returns an async generator suitable for FastAPI `Depends`, and `Injector.context(state, request)` returns an `AsyncContextManager[T]` (`openhands/app_server/services/injector.py:23-33`).
- **Gap**: the test never asserts the *content* of the `AgentContext` that gets passed into `create_agent()`. It only checks the resulting `StartConversationRequest.agent` shape (`tests/unit/app_server/test_live_status_app_conversation_service.py:1047-1049`). So a bug that swaps two secrets, or forgets to copy the system_message_suffix, would not be caught by this test.

### 5. Can context enforce permissions?

**No, not at the tool level. Permissions are configured once at the agent level, not per tool.**

- `_select_confirmation_policy` returns `NeverConfirm`, `AlwaysConfirm`, or `ConfirmRisky` based on `confirmation_mode` + `security_analyzer` strings (`openhands/app_server/app_conversation/app_conversation_service_base.py:641-652`).
- `_create_security_analyzer_from_string` returns `LLMSecurityAnalyzer` for the string `"llm"`, `None` for `"none"` or unknown (`openhands/app_server/app_conversation/app_conversation_service_base.py:614-639`).
- These are then attached to the `Agent` via the SDK — not embedded in a `ToolContext` object that each tool checks.
- There is no per-tool ACL, no per-user permission grant, no audit log of which user invoked which tool with which args.
- The error-category classifier in `webhook_router.py:73-84` (mapping exception messages to `budget_exceeded`, `model_error`, `runtime_error`, `timeout`, `user_cancelled`, `unknown`) is the closest thing to a post-hoc tool-event audit, but it operates at the conversation-event level, not the tool-call level.

## Architectural Decisions

1. **Decouple tool context from tool implementation.** Tools come from `openhands-tools` (external package); the context (LLM, secrets, workspace, MCP) is composed here. This lets the SDK be released independently from the app server, but it means we cannot grade the tool *base class* from this repo.

2. **Use `Injector[T]` as a single, generic DI primitive.** `inject()`, `context()`, `depends()` cover `async with`, `async for`, and FastAPI `Depends` call sites from one ABC (`openhands/app_server/services/injector.py:12-34`). Reusable for httpx, DB, JWT, user, sandbox, conversation, event, event_callback, etc.

3. **Per-request state bag, not a global container.** `InjectorState: TypeAlias = State` (`openhands/app_server/services/injector.py:9`) — i.e., the FastAPI `Request.state` object. Resources are created on first use, cached, and cleaned up in `finally`. The `KEEP_OPEN` flag lets background tasks opt out (`openhands/app_server/services/httpx_client_injector.py:32-37, 40-42`, `openhands/app_server/services/db_session_injector.py:25-26, 311-322, 326-328`).

4. **Secrets as `SecretSource` discriminated union.** `StaticSecret` for OSS, `LookupSecret` for SaaS. The agent never sees a raw token; it only sees a URL it can fetch from. JWT is short-lived (14 days max) and scoped to a single secret.

5. **`UserContext` is an abstract base with two concrete implementations.** `AuthUserContext` for end-users, `SpecifyUserContext` for admin/background operations (`openhands/app_server/user/user_context.py:13-83`, `openhands/app_server/user/auth_user_context.py:27-166`, `openhands/app_server/user/specifiy_user_context.py:13-65`). The `as_admin(request)` helper patches request state for the duration of a single endpoint (`openhands/app_server/user/specifiy_user_context.py:55-65`).

6. **MCP tools use a different context model** because `fastmcp` already provides `get_http_request()`. The five `create_pr`/`create_mr` tools read identity from inbound HTTP headers at call time. This is consistent with the MCP server philosophy (request-scoped), but inconsistent with the typed `UserContext` model used by the rest of the app server.

7. **Constructor injection over service-locator.** The service is a `@dataclass` with all collaborators as fields (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:199-226`). This makes unit tests trivial and makes the dependency graph visible at a glance.

## Notable Patterns

- **`Injector[T]`** — a single, generic async-context-manager DI primitive that supports both `async with` and FastAPI `Depends` call sites. Reusable across 10+ service types (`openhands/app_server/services/injector.py:12-34`).
- **Discriminated union of injectors** — `*ServiceInjector` types implement `DiscriminatedUnionMixin`, so configuration can pick one of several concrete implementations without the consumer caring. Visible in `AppServerConfig` (`openhands/app_server/config.py:215-225`).
- **Per-request state caching** — the first injector call stores the resolved object on `Request.state`; subsequent calls in the same request reuse it (`openhands/app_server/user/auth_user_context.py:158-164`, `openhands/app_server/services/httpx_client_injector.py:23-26`, `openhands/app_server/services/db_session_injector.py:301-310`).
- **`KEEP_OPEN` flags** for resources that need to outlive a single request — e.g., a background task that streams from a `httpx` client across multiple requests (`openhands/app_server/services/httpx_client_injector.py:32-37, 40-42`).
- **`SecretSource` discriminated union** — `StaticSecret` vs `LookupSecret` so the wire format changes between SaaS and OSS without changing the call site (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1118-1142`).
- **Constructor injection for the service, request-state injection for user identity** — two different mechanisms for two different lifecycles. The service is long-lived; the user is per-request. Both are explicit, neither is a singleton.
- **Hooks and skills are loaded over HTTP from the agent-server, not from a local registry** — the app server is a thin proxy, the agent-server is the source of truth (`openhands/app_server/app_conversation/hook_loader.py:43-100`, `openhands/app_server/app_conversation/skill_loader.py:264-358`).

## Tradeoffs

- **Externalising tools** to `openhands-tools` allows SDK release cadence to be independent of the app server, but it means the tool-class design (including any per-tool context object) is opaque to this study.
- **Per-request state as the DI container** is simple and uses a built-in (FastAPI's `Request.state`), but it implicitly assumes the request lifecycle is short and that resources can be created cheaply. For long-lived connections (WebSockets, background workers), the `KEEP_OPEN` flag is a workaround, not a clean abstraction.
- **`LookupSecret` for SaaS** keeps the raw token out of the agent-server process, but it adds a synchronous HTTP roundtrip per secret access. The app must be reachable from the agent-server, which is a deployment constraint.
- **`StaticSecret` for OSS** is simpler and avoids the webhook roundtrip, but the raw token is now visible to the agent-server process. The mitigation is `SecretStr` (Pydantic), which prevents accidental logging but not deliberate access.
- **Constructor injection of services** makes tests trivial but inflates the dataclass field list (17 fields in `LiveStatusAppConversationService`). Adding a new collaborator requires editing the constructor in multiple places.
- **MCP tools reading from `get_http_request()`** is consistent with the FastMCP framework but inconsistent with the typed `UserContext` model used by the rest of the system. The two context models cannot share test fixtures.
- **No `CancellationToken` abstraction** — relies on `asyncio.Task.cancel()`. Tools that perform long blocking I/O have no cooperative cancellation mechanism; they can only be killed at the task boundary.

## Failure Modes / Edge Cases

1. **User context leakage across requests.** The `InjectorState` is `Request.state`, so a bug that stashes a `UserContext` in a module-level variable would leak one user's identity to the next request. The audit shows injectors store on `state` (`setattr(state, USER_CONTEXT_ATTR, ...)`) — correct. But the `as_admin(request)` helper raises `OpenHandsError` if a non-admin context is already set (`openhands/app_server/user/specifiy_user_context.py:55-65`), implying this kind of leak was anticipated.

2. **`LookupSecret` JWT expiry is a hard timeout.** `access_token_hard_timeout` defaults to 14 days (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2458-2464`), but a token issued at conversation start expires at conversation end + 14d. There is no per-secret refresh hook; if a long-lived agent-server process outlives the JWT, secret access will silently fail.

3. **`LookupSecret` does not work for ACP agents** because the SDK redacts `X-Access-Token` headers (matches `SECRET_KEY_PATTERNS`) (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1815-1819`). This is documented in the source but easy to miss; a developer who adds a new `LookupSecret` for an ACP flow will silently break auth.

4. **`tools = []` returned by `get_default_tools(...)` if the external package is not installed** would cause the agent to start with no tools. There is no defensive `assert tools` at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1620-1623`.

5. **`MCPConfig` failure** — if the user has invalid MCP server config (bad URL, bad auth), `_configure_llm_and_mcp` will throw before tools are built (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1587-1590`). The exception bubbles up; the conversation start fails.

6. **Webhook → context bridge** has to construct a fresh `InjectorState` for background callbacks (no `Request` available) and manually `setattr(state, USER_CONTEXT_ATTR, SpecifyUserContext(user_id))` (`openhands/app_server/event_callback/webhook_router.py:561-573`, `openhands/app_server/mcp/mcp_router.py:100-110`). If the user is later deleted, the callback will still attempt to run as that user.

7. **Sandbox startup timeout** is configurable (`sandbox_startup_timeout`, default 120s) but the `httpx` client for the conversation has a separate 15s timeout (`openhands/app_server/services/httpx_client_injector.py:18`). A slow sandbox start will hit the agent-server timeout before the conversation-start timeout.

8. **No test of `AgentContext` content shape.** The test asserts the request has the right `agent.llm.model` (`tests/unit/app_server/test_live_status_app_conversation_service.py:1049`) but not the right `agent.agent_context.secrets` or `agent.agent_context.system_message_suffix`. A regression that drops a secret or a system prompt would not be caught.

9. **MCP tools can't be unit-tested without mocking `fastmcp.server.dependencies.get_http_request`**, which is a private API surface. Any change to FastMCP's internal context model will silently break the five `create_pr`/`create_mr` tools.

10. **`PluginSource` and `HookConfig` are passed by value into `StartConversationRequest`** (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1694-1703, 1714`). If the agent-server cannot reach the plugin URL, the conversation will start but the plugin will be unavailable — no retry, no warning surfaced to the user.

## Future Considerations

- **Introduce an explicit `ToolContext` object** on the consumer side (this repo). The current "context as constellation of fields" model (`llm`, `tools`, `mcp_config`, `agent_context`, `workspace`) makes it hard to add a new dependency (e.g., metrics, feature flags) without editing every builder.
- **Inject the logger** with a `LoggerProvider` that auto-binds `request_id`, `conversation_id`, `user_id`. The current module-level `_logger = logging.getLogger(__name__)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:142`) cannot do contextual logging.
- **Add a `CancellationToken` abstraction** that tools can check cooperatively. The current `asyncio.Task.cancel()` only works at task boundaries.
- **Unify the MCP context model** with the typed `UserContext` model. A `MCPUserContext` injected into each `@mcp_server.tool()` function would make the five PR/MR tools testable and would align with the rest of the app.
- **Add tool-permission policy as a context field.** `ConfirmationPolicyBase` and `SecurityAnalyzerBase` are configured once at the agent level (`openhands/app_server/app_conversation/app_conversation_service_base.py:641-652`); per-tool ACLs are missing.
- **Add an end-to-end test that asserts the `AgentContext` content.** The current tests check `result.agent.llm.model` but not `result.agent.agent_context.secrets` or `result.agent.agent_context.system_message_suffix` (`tests/unit/app_server/test_live_status_app_conversation_service.py:1047-1049`).
- **Document the `LookupSecret` → ACP redaction hazard** in a place where new contributors will see it, not only in the source comment at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1815-1819`.
- **Document the tool-class contract** in this repo or in a `docs/` directory, even if the tool classes live elsewhere. Right now a reader of this repo cannot tell whether `BaseTool` accepts a typed context, an injected context, or no context at all.

## Questions / Gaps

1. **What does the `BaseTool` / `ToolDefinition` class look like in `openhands-tools`?** This repo only consumes the tool list (`get_default_tools(...)` at `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1620`); it does not show the tool base class. The actual contract for "how a tool receives its context" is therefore unknown. No evidence found in this source.

2. **Is there a per-tool `Runtime` or `ToolContext` parameter in the SDK?** Not visible from this repo. Search for `runtime` / `ToolContext` in `openhands/app_server/` returns no tool-related matches.

3. **How are tool invocations audited?** The webhook router has an error-category classifier (`openhands/app_server/event_callback/webhook_router.py:73-84`), but it operates at the conversation-event level. No tool-call audit log was found in this repo. No evidence found.

4. **Does the `ConversationSecretEnricher` extension point run for ACP conversations?** The base class returns `ConversationSecretEnrichment(system_message_suffix=system_message_suffix)` with empty secrets (`openhands/app_server/app_conversation/conversation_secret_enricher.py:21-40`). The integration overrides (Jira, GitHub, etc.) are in the `enterprise/` directory which is excluded from this study's scope.

5. **How does the `SwitchLLMTool` know the list of available profiles at runtime?** The check is `len(valid_profile_names) >= 2` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1653-1654`) computed from `user.llm_profiles.profiles`. The tool itself (in the external package) presumably re-derives the list from the user context it receives. We cannot verify this from this repo.

6. **Is there a `Permission` / `ToolPermission` registry?** No. The only permission-related code is `ConfirmationPolicyBase` and `SecurityAnalyzerBase` (`openhands/app_server/app_conversation/app_conversation_service_base.py:614-652`). No evidence of a per-tool ACL.

7. **Why is the `webhook_router._import_all_tools()` call required?** (`openhands/app_server/event_callback/webhook_router.py:576-586`). It walks the `openhands.tools` package and imports every subpackage "so that they are available for deserialization in webhooks". This implies the tool instances are serialized and deserialized — but the serialization format is not visible from this repo.

8. **What happens to the `AgentContext` across conversation resume?** The `LiveStatusAppConversationService._load_skills_onto_request` path updates the agent's context with new skills (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1746-1773`), but the `secrets` field is built once at conversation start. Resuming a conversation may require re-fetching `LookupSecret` URLs with the same JWT, which has a 14-day expiry (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:2458-2464`). Not verified.

---

Generated by `04.04-tool-context-and-dependency-injection` against `openhands`.
