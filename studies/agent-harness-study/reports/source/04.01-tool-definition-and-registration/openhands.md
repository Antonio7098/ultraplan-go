# Source Analysis: openhands

## Tool Definition and Registration

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (>=3.12,<3.14), FastAPI, FastMCP (>=3.2,<4), Pydantic; integrates with external `openhands-sdk==1.29.0`, `openhands-tools==1.29.0`, `openhands-agent-server==1.29.0` packages (per `pyproject.toml:60-62`). |
| Analyzed | 2026-07-19 |

## Summary

The selected source is the open-source OpenHands backend (`openhands-ai` per `pyproject.toml:8`). The repository is deliberately split: the actual agent toolkit (default tools, planning tools, browser tools, sub-agents) lives in the externally-pinned `openhands-sdk` and `openhands-tools` packages, which this source imports but does not define. Tool definition and registration within this source therefore has two distinct scopes:

1. **App-server MCP surface (locally defined).** A single `FastMCP('mcp', mask_error_details=True)` instance is created at module import (`openhands/app_server/mcp/mcp_router.py:43`) and mounted into the FastAPI app at `/mcp` (`openhands/app_server/app.py:33,59`). Tools are attached via two patterns:
   - **Decorator-based registration** — five `@mcp_server.tool()` decorated async functions: `create_pr` (`openhands/app_server/mcp/mcp_router.py:147`), `create_mr` (`openhands/app_server/mcp/mcp_router.py:215`), `create_bitbucket_pr` (`openhands/app_server/mcp/mcp_router.py:289`), `create_bitbucket_data_center_pr` (`openhands/app_server/mcp/mcp_router.py:356`), `create_azure_devops_pr` (`openhands/app_server/mcp/mcp_router.py:423`). All five delegate to per-provider `*ServiceImpl` classes under `openhands/app_server/integrations/`.
   - **Mount-based registration** — `init_tavily_proxy()` mounts an external Tavily MCP server under the `tavily` namespace at server startup (`openhands/app_server/mcp/mcp_router.py:49-75`, called from `openhands/app_server/app.py:31`).

2. **Toolset handed to the sandbox (external SDK).** When a conversation starts, the local service resolves which tools the agent receives by calling factory functions from the external SDK packages: `get_default_tools(enable_browser=True, enable_sub_agents=...)` for default agents or `get_planning_tools(plan_path=plan_path)` for plan agents (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1617-1625`). Built-in sub-agents are also opted into by calling the SDK's `register_builtins_agents(enable_browser=True)` before the tools are constructed (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1619`). A built-in `SwitchLLMTool` may be appended after the agent is created, gated on having ≥2 valid LLM profiles (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1648-1664`). The resulting tool list is then injected into `AgentSettings.tools` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1633`).

The MCP configuration that gets shipped to the sandbox is built dynamically per conversation by `_configure_llm_and_mcp()` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1286-1315`). It always adds a `default` MCP server entry pointing back at this same app-server's `/mcp/mcp` endpoint (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1235-1245`) and merges user-defined MCP servers from `user.agent_settings.mcp_config` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1247-1284`, `openhands/app_server/settings/settings_models.py:413`). Per-user MCP config is persisted on `OpenHandsAgentSettings.mcp_config` (typed as `fastmcp.mcp_config.MCPConfig`) and replaced wholesale on update rather than deep-merged (`openhands/app_server/settings/settings_models.py:212-224`, `openhands/app_server/utils/jsonpatch_compat.py:7`).

The local source does **not** contain a Python-level tool registry, class hierarchy, or plugin scanner. There is no tool base class, no `register_tool()` decorator that mutates a global catalog, no entry-point discovery, and no AST-derived schema. Registration is static (decorator) at import time, plus runtime dynamic merging of user-configured MCP servers per conversation.

## Rating

**6/10** — Present, clear at the local MCP layer, and consistent with the external SDK, but the heavy lifting (the default toolset, schemas, sub-agents) lives outside the studied source, so we can only rate what is in-tree.

Rationale:
- **Strengths**: explicit decorator-based MCP tool registration with full Pydantic-typed parameters and per-tool docstrings (`openhands/app_server/mcp/mcp_router.py:147-487`); dynamic per-conversation merging of system MCP server (`default`) with user MCP config (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1219-1315`); wholesale replacement of `mcp_config` in settings update semantics (`openhands/app_server/utils/jsonpatch_compat.py:7`); conditional inclusion of `SwitchLLMTool` based on profile count (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:1648-1664`); failure-isolated Tavily mount that logs and continues (`openhands/app_server/mcp/mcp_router.py:62-75`); tests cover the MCP mount, conversation link helpers, and settings mcp_config plumbing (`tests/unit/server/routes/test_mcp_routes.py:11-288`, `tests/unit/mcp/test_mcp_integration.py:14-95`, `tests/unit/app_server/test_live_status_app_conversation_service.py:624-848,1069-1119`).
- **Limitations**: the actual default tool catalog (terminal, file editor, browser, plan tools, sub-agents) is *not* in this source — only imported via `from openhands.tools.preset.default import (get_default_tools, register_builtins_agents)` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-139`). No tool base class or protocol exists locally; naming, schema generation, validation, and approval gating all live in the SDK. There is no Python-level tool discovery (no entry-points, no package scan) — every local tool is a literal `@mcp_server.tool()` line at module import. Adding a new MCP tool requires editing `mcp_router.py` and (for production use) integrating with provider services; nothing else auto-discovers. Tool identity (lookup keys, namespaces, collision detection) is not present locally. There is no `is_enabled` per-tool predicate equivalent; gating is coarse-grained (PLAN vs DEFAULT agent type, plus the post-hoc `SwitchLLMTool` append). The `_import_all_tools()` walk in `openhands/app_server/event_callback/webhook_router.py:576-586` is *not* a registration mechanism — it's a side-effecting import to make pydantic discriminator classes resolvable, and it walks the `openhands.tools` namespace package rather than a registry.

## Evidence Collected

Every entry cites `path/to/file.py:NN` from inside the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| FastMCP instance creation | `mcp_server = FastMCP('mcp', mask_error_details=True)` | `openhands/app_server/mcp/mcp_router.py:43` |
| FastMCP mounted into FastAPI app at `/mcp` | `mcp_app = mcp_server.http_app(path='/mcp', stateless_http=True)` and `routes=[Mount(path='/mcp', app=mcp_app)]` | `openhands/app_server/app.py:33,59` |
| Tavily proxy mount function | `mcp_server.mount(namespace='tavily', server=proxy_server)` | `openhands/app_server/mcp/mcp_router.py:72` |
| Tavily namespace comment | "Mount under 'tavily' namespace so tools are accessible as tavily_*" | `openhands/app_server/mcp/mcp_router.py:71` |
| Tavily init called at app construction | `init_tavily_proxy()` invoked once at module load | `openhands/app_server/app.py:31` |
| GitHub PR tool (decorator) | `@mcp_server.tool() async def create_pr(...)` | `openhands/app_server/mcp/mcp_router.py:147-212` |
| GitLab MR tool (decorator) | `@mcp_server.tool() async def create_mr(...)` | `openhands/app_server/mcp/mcp_router.py:215-286` |
| Bitbucket PR tool (decorator) | `@mcp_server.tool() async def create_bitbucket_pr(...)` | `openhands/app_server/mcp/mcp_router.py:289-353` |
| Bitbucket Data Center PR tool (decorator) | `@mcp_server.tool() async def create_bitbucket_data_center_pr(...)` | `openhands/app_server/mcp/mcp_router.py:356-420` |
| Azure DevOps PR tool (decorator) | `@mcp_server.tool() async def create_azure_devops_pr(...)` | `openhands/app_server/mcp/mcp_router.py:423-486` |
| Pydantic-typed tool parameters (typical) | `Annotated[str, Field(description='GitHub repository ({{owner}}/{{repo}})')]` | `openhands/app_server/mcp/mcp_router.py:149-150` |
| ToolError pattern on failure | `raise ToolError(str(error))` inside each tool | `openhands/app_server/mcp/mcp_router.py:210,284,351,418,485` |
| Per-provider service delegation (GitHub) | `github_service = GithubServiceImpl(user_id=..., external_auth_id=..., external_auth_token=..., token=..., base_domain=...)` | `openhands/app_server/mcp/mcp_router.py:181-187` |
| Conversation link appended in SAAS mode | `body = await get_conversation_link(github_service, conversation_id, body or '')` | `openhands/app_server/mcp/mcp_router.py:190` |
| PR-number metadata capture post-tool | `await save_pr_metadata(user_id, conversation_id, response)` | `openhands/app_server/mcp/mcp_router.py:206,280,346,413,480` |
| Default tools factory import (external SDK) | `from openhands.tools.preset.default import (get_default_tools, register_builtins_agents)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-135` |
| Planning tools factory import (external SDK) | `from openhands.tools.preset.planning import (format_plan_structure, get_planning_tools)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:136-139` |
| PLAN agent toolset selection | `tools = get_planning_tools(plan_path=plan_path)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1617` |
| DEFAULT agent toolset selection + sub-agent registration | `register_builtins_agents(enable_browser=True); tools = get_default_tools(enable_browser=True, enable_sub_agents=user.agent_settings.enable_sub_agents)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1619-1625` |
| Tools injected into AgentSettings | `update={'tools': tools, ...}` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1630-1640` |
| Conditional `SwitchLLMTool` append | `if len(valid_profile_names) >= 2 and SwitchLLMTool.__name__ not in agent.include_default_tools:` then `include_default_tools=[*agent.include_default_tools, SwitchLLMTool.__name__]` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1653-1664` |
| Sub-agent definitions ingestion | `agent_definitions = list(get_registered_agent_definitions())` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1625` |
| System "default" MCP server added per conversation | `mcp_servers['default'] = {'url': f'{self.web_url}/mcp/mcp', 'headers': {'X-OpenHands-ServerConversation-ID': str(conversation_id)}}` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1236-1239` |
| Session API key added to MCP headers | `mcp_servers['default']['headers']['X-Session-API-Key'] = mcp_api_key` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1243-1245` |
| Custom user MCP config merged | `for name, server in sdk_mcp.mcpServers.items(): mcp_servers[name] = server.model_dump(exclude_none=True)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1269-1270` |
| ACP agent settings skip custom MCP merge | `if isinstance(user.agent_settings, ACPAgentSettings): return` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1256-1257` |
| Final MCP shape | `mcp_config = {'mcpServers': mcp_servers} if mcp_servers else {}` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1312` |
| Per-user `mcp_config` typed as `fastmcp.mcp_config.MCPConfig` | `mcp_config: MCPConfig | None = None` on POSTProviderModel | `openhands/app_server/settings/settings_models.py:413` |
| Wholesale replacement of `mcp_config` on settings update | `WHOLESALE_REPLACEMENT_KEYS = frozenset({'mcp_config'})` | `openhands/app_server/utils/jsonpatch_compat.py:7` |
| Wholesale replace application | `replace_mcp_config = 'mcp_config' in agent_update; ... dumped['mcp_config'] = mcp_config; new_settings = validate_agent_settings(dumped)` | `openhands/app_server/settings/settings_models.py:212-224` |
| `_import_all_tools` (NOT a registry — side-effect import for deserialization) | `for _, name, is_pkg in pkgutil.walk_packages(tools.__path__, tools.__name__ + '.'): if is_pkg: importlib.import_module(name)` | `openhands/app_server/event_callback/webhook_router.py:576-583` |
| Webhook router purpose of import | "We need to import all tools so that they are available for deserialization in webhooks." | `openhands/app_server/event_callback/webhook_router.py:577` |
| Plugin loading delegated to agent-server | `sdk_plugins = [PluginSource(source=p.source, ref=p.ref, repo_path=p.repo_path) for p in plugins]` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1695-1703` |
| Hook loading delegated to agent-server | `hook_config = await self._load_hooks_from_workspace(remote_workspace, project_dir)` calls `load_hooks_from_agent_server` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1678-1680`; helper at `openhands/app_server/app_conversation/hook_loader.py:43-80` |
| `init_tavily_proxy` swallows failures | `except Exception as e: logger.error(...)` | `openhands/app_server/mcp/mcp_router.py:74-75` |
| Test — MCP server deprecation-free instantiation | `test_mcp_server_no_stateless_http_deprecation_warning` | `tests/unit/server/routes/test_mcp_routes.py:11-40` |
| Test — Tavily proxy init paths | `TestInitTavilyProxy` (no API key / empty key / with key / exception) | `tests/unit/server/routes/test_mcp_routes.py:154-288` |
| Test — MCP server advertised URL | `mcp_config['mcpServers']['default']['url'] == 'https://test.example.com/mcp/mcp'` | `tests/unit/app_server/test_live_status_app_conversation_service.py:646-647` |
| Test — `get_default_tools` and `register_builtins_agents` invoked | `@patch('...get_default_tools', return_value=[])` and `mock_register_builtins.assert_called_once_with(enable_browser=True)` | `tests/unit/app_server/test_live_status_app_conversation_service.py:1076,1115-1118` |
| Test — `mcp_config` wholesale replace | `test_settings_update_mcp_config` and `test_replace_mcp_config_in_kind_switch` | `tests/unit/storage/data_models/test_settings.py:170-230`; `tests/unit/app_server/test_settings_agent_kind_switch.py:98-105` |
| External SDK pin (defines what tools exist) | `openhands-sdk = "==1.29.0"`, `openhands-tools = "==1.29.0"`, `openhands-agent-server = "==1.29.0"` | `pyproject.toml:60-62,249-251` |
| FastMCP dependency | `fastmcp>=3.2,<4`, `mcp>=1.25` | `pyproject.toml:37,56` |
| Namespace package shim that lets SDK subpackages join `openhands.*` | `__path__ = __import__('pkgutil').extend_path(__path__, __name__)` | `openhands/__init__.py:4` |

## Answers to Dimension Questions

1. **How does a new tool enter the system?**
   - **Add an MCP tool exposed by the app server:** add an `@mcp_server.tool()` decorated async function in `openhands/app_server/mcp/mcp_router.py` (see `create_pr` at line 147 for the canonical shape: Pydantic-`Annotated[str, Field(description=...)]` parameters, docstring, `get_http_request()` for headers, `raise ToolError(...)` on failure). The tool is registered the moment `mcp_router` is imported — i.e., when `openhands.app_server.app` is constructed (line 19 of that file imports `mcp_server`).
   - **Mount an external MCP server:** extend `init_tavily_proxy()` or add a sibling init function in `mcp_router.py` that calls `mcp_server.mount(namespace=..., server=...)`. The namespace prefix is what makes the tools discoverable under a stable name (`tavily_*` in the Tavily case).
   - **Change the default toolset given to a sandbox conversation:** add a new factory under `openhands.tools.preset.*` (lives in the pinned `openhands-tools` package — `pyproject.toml:62`) and switch the call site in `live_status_app_conversation_service.py:1617-1625` to use it. This source alone cannot make new default tools appear; it must upgrade the SDK pin.
   - **Add a per-user remote MCP server:** the user updates `OpenHandsAgentSettings.mcp_config` via the settings API (`openhands/app_server/settings/settings_models.py:413`). On the next conversation start, `_merge_custom_mcp_config` (`live_status_app_conversation_service.py:1247-1284`) adds the server entry to the per-conversation `mcpServers` dict.

2. **Is tool registration explicit?**
   Yes at the local MCP layer (decorator with no default-on behavior; tools must be written), and yes at the settings layer (users must enumerate `mcp_config.mcpServers` to add MCP servers). It is **implicit** with respect to the default agent toolset — the catalog of default tools is whatever ships in `openhands-tools==1.29.0`. There is no audit log or "what did I register" introspection endpoint at the local level.

3. **Can tools be discovered dynamically?**
   - **At the agent-server boundary:** yes, indirectly. The app server tells the sandbox the MCP server URLs (including `default` pointing at `/mcp/mcp`), and the SDK Agent on the sandbox side calls `MCPServer.list_tools()` per its MCP client implementation (out of scope of this source). The local `_import_all_tools` (`webhook_router.py:576-586`) walks the `openhands.tools` package to trigger side-effect imports, but it is **not** a tool registry — it exists only so Pydantic discriminator classes can be resolved during webhook deserialization.
   - **User-configured MCP servers:** yes, via the `mcp_config` setting persisted on `OpenHandsAgentSettings`. The schema is `fastmcp.mcp_config.MCPConfig` (`openhands/app_server/settings/settings_models.py:16-17,413`), so any MCPConfig-compatible server is accepted.
   - **Built-in tools (terminal, file editor, browser, etc.):** no — the default toolset is fixed by the externally-pinned `openhands-tools` package; this source has no Python-level plugin scanner, no entry-point discovery, no decorator-driven in-tree registration.

4. **Are names stable?**
   Locally: yes, names are the function names of the `@mcp_server.tool()` decorated callables (`create_pr`, `create_mr`, `create_bitbucket_pr`, `create_bitbucket_data_center_pr`, `create_azure_devops_pr`) at `mcp_router.py:147,215,289,356,423`. Namespacing is enforced by `mcp_server.mount(namespace='tavily', ...)` (`mcp_router.py:72`) so Tavily tools surface as `tavily_*`. The system MCP server entry is hard-named `'default'` (`live_status_app_conversation_service.py:1237`). No collision detection logic exists locally — collisions would be FastMCP's problem at runtime.
   For user-configured MCP servers, names are whatever the user keys them by in `mcp_config.mcpServers` and are wholesale-replaced on update (`settings_models.py:212-224`, `jsonpatch_compat.py:7`).

5. **Can multiple agents see different tool sets?**
   Yes, in two ways:
   - **Per-conversation agent type:** PLAN agents receive `get_planning_tools(plan_path=...)`; DEFAULT agents receive `get_default_tools(enable_browser=True, enable_sub_agents=...)` (`live_status_app_conversation_service.py:1611-1625`). The selection is made by a literal `if agent_type == AgentType.PLAN` branch at line 1613.
   - **Per-user MCP config:** each user can declare their own `mcp_config.mcpServers`; the per-conversation MCP dict merges `default` + that user list (`live_status_app_conversation_service.py:1286-1315`).
   - **Conditional `SwitchLLMTool`:** added post-creation iff ≥2 valid LLM profiles exist (`live_status_app_conversation_service.py:1648-1664`).
   - **ACP variant skips custom MCP merge entirely:** `if isinstance(user.agent_settings, ACPAgentSettings): return` at `live_status_app_conversation_service.py:1256-1257`, which means ACP agent conversations receive only the system `default` MCP server.

## Architectural Decisions

- **Default toolset is external.** `pyproject.toml:60-62,249-251` pins `openhands-sdk`, `openhands-tools`, and `openhands-agent-server` to the exact version `1.29.0`. The local source deliberately does not re-implement the default toolset — the app server's job is plumbing (LLM, secrets, MCP routing, conversation lifecycle) and the sandbox runs the SDK code. Tradeoff: a single pinned upgrade changes the entire tool surface, and the local repo cannot independently iterate on tool semantics.
- **App-server acts as an MCP server.** A single `FastMCP` instance is mounted at `/mcp` (`app.py:33,59`) so the sandbox can call back into the app server for privileged operations (PR creation, issue creation). This makes the app server both the conversation orchestrator *and* an MCP provider for tools that need server-side credentials.
- **Two registration styles in one router.** Decorator-based for hand-written tools (`@mcp_server.tool()`) and `mount`-based for proxying an upstream MCP server (Tavily). Both populate the same `FastMCP` instance and become indistinguishable from the consumer side.
- **Settings `mcp_config` is wholesale-replaced, not deep-merged.** `WHOLESALE_REPLACEMENT_KEYS = frozenset({'mcp_config'})` (`utils/jsonpatch_compat.py:7`) plus the explicit `replace_mcp_config` branch (`settings_models.py:212-224`) make the user model a full replacement — easier to reason about than merge semantics, but means the SDK cannot quietly keep a server around across an update.
- **No global Python registry.** There is no `ToolRegistry`, no `register_tool()` decorator mutating a global catalog, no entry-point discovery, no AST-derived schema in this source. The "registry" is implicit: it's whatever FastMCP has registered inside `mcp_server` at the time of the call, plus the per-conversation `mcpServers` dict that gets shipped to the sandbox.
- **Delegation pattern, not implementation.** Every locally-defined MCP tool (`create_pr` etc.) does no Git provider work itself — it constructs a `*ServiceImpl` (e.g., `GithubServiceImpl`, `BitBucketServiceImpl`, `AzureDevOpsServiceImpl`, `GitLabServiceImpl`, `BitbucketDCServiceImpl`) and delegates (`mcp_router.py:181-203` etc.). This keeps the MCP router thin and concentrates provider logic under `openhands/app_server/integrations/<provider>/`.
- **Tavily proxy for credential isolation.** Instead of handing the Tavily API key to every sandbox, the app server hosts the Tavily client and exposes a FastMCP proxy under the `tavily` namespace (`mcp_router.py:49-75`). Sandboxes see no API key.
- **`_import_all_tools` is a side-effecting import, not registration.** `webhook_router.py:576-586` walks the `openhands.tools` package and imports each subpackage so Pydantic discriminator classes can resolve during deserialization. It does not enumerate tools or build any local catalog — that work is done by the SDK's `get_default_tools` factory.

## Notable Patterns

- **Decorator + FastMCP instance as the registration primitive.** The pattern at `mcp_router.py:147,215,289,356,423` is: declare `mcp_server = FastMCP(...)` once at module scope, then attach each tool with `@mcp_server.tool()`. The function name becomes the tool name. (`function_tool`-style with explicit name override is not used here.)
- **Pydantic-annotated parameters with `Field(description=...)`.** Every parameter carries an `Annotated[type, Field(description=...)]` wrapper that fastmcp surfaces as the JSON schema description. Examples at `mcp_router.py:149-162` (create_pr), `217-235` (create_mr), `291-302` (create_bitbucket_pr), `358-369` (create_bitbucket_data_center_pr), `425-436` (create_azure_devops_pr).
- **`ToolError` from FastMCP for failure surfacing.** Each provider tool wraps the call in a try/except and re-raises as `ToolError(str(error))` (`mcp_router.py:208-211,282-285,348-351,415-418,482-485`). This ensures the MCP client sees a structured error rather than a stack trace.
- **HTTP-context-derived auth.** Tools read `get_http_request()` (`mcp_router.py:167,240,307,374,441`) and extract `X-OpenHands-ServerConversation-ID`, `provider_tokens`, and `access_token` from headers. The app server authenticates the *call*, not the agent.
- **Per-tool metadata capture.** After a successful PR/MR creation, the tool calls `save_pr_metadata(user_id, conversation_id, response)` to attach the PR number back to the conversation record (`mcp_router.py:206,280,346,413,480`). This is implemented with `setattr(state, USER_CONTEXT_ATTR, SpecifyUserContext(user_id))` to manually construct a request context (`mcp_router.py:101-103`).
- **Conversation link injection (SAAS mode only).** `get_conversation_link` (`mcp_router.py:78-95`) appends a follow-up link in PR/MR bodies if `AppMode.SAAS`. OSS deployments short-circuit out at line 82-83.
- **Per-conversation MCP composition.** `_configure_llm_and_mcp` (`live_status_app_conversation_service.py:1286-1315`) produces an `{'mcpServers': {...}}` dict each time a conversation starts, layering (a) system `default` with conversation-scoped headers, then (b) user `mcp_config` overrides on top. The shape is exactly what the SDK's `fastmcp.mcp_config.MCPConfig` expects (`live_status_app_conversation_service.py:1628-1634`).
- **Sub-agent opt-in via a single boolean.** `enable_sub_agents=True` causes both `register_builtins_agents(enable_browser=True)` *and* `get_default_tools(enable_sub_agents=True)` to fire, plus `agent_definitions = list(get_registered_agent_definitions())` is captured and forwarded (`live_status_app_conversation_service.py:1619-1625`). One flag drives the entire sub-agent bundle.

## Tradeoffs

- **No tool abstraction in this repo.** Every tool must be a `FastMCP` decorator or a mount. There is no `class BaseTool(...)`, no `@register_tool` decorator, no protocol interface — adding a new tool family requires choosing MCP from the start. This is fine for the current shape (everything either is MCP or is delegated to the SDK), but means the app server cannot offer e.g. an OpenAI-Responses-only hosted tool variant locally.
- **Tightly coupled to FastMCP version.** `pyproject.toml:37` pins `fastmcp>=3.2,<4`. The `mask_error_details=True` flag (`mcp_router.py:43`) and `http_app(path='/mcp', stateless_http=True)` (`app.py:33`) are FastMCP-specific. A test (`tests/unit/server/routes/test_mcp_routes.py:11-40`) explicitly guards against regressions in FastMCP deprecation warnings.
- **App server == MCP server == auth context holder.** Because the app server hosts the FastMCP server, every tool that uses provider credentials must go through `get_http_request()` and `get_provider_tokens(request)`. There is no way to add a tool that uses a different auth model without rewriting the request-extraction pattern.
- **Default toolset lives outside version control of this source.** A pinned upgrade of `openhands-tools` can silently change the tools available to every new conversation. There is no in-repo CHANGELOG of "what tools does a default agent have today."
- **`_import_all_tools` couples the webhook router to package layout.** If a future SDK release moves tools to a different subpackage, `webhook_router.py:578-583` will silently miss them (`for _, name, is_pkg in pkgutil.walk_packages(...)` doesn't enumerate at depth).
- **Tavily proxy init swallows all exceptions.** `init_tavily_proxy` only logs (`mcp_router.py:74-75`) — a misconfigured key path or a Tavily outage surfaces only as a log line, not a startup failure. Operators have to read logs to know the mount is missing.
- **Custom MCP merge failures are caught and continued.** `except Exception as e:` at `live_status_app_conversation_service.py:1276-1284` logs and continues with the system config only — better than crashing a conversation, but means a typo in user MCP config silently reverts to "just the default server."
- **`mcp_config` wholesale replace has no per-server toggle.** `WHOLESALE_REPLACEMENT_KEYS = frozenset({'mcp_config'})` (`utils/jsonpatch_compat.py:7`) means an empty `mcpServers: {}` from the user wipes every custom server they previously configured.

## Failure Modes / Edge Cases

- **Tavily proxy fails to construct.** Caught and logged at `mcp_router.py:74-75`; app server starts without Tavily tools. No startup failure.
- **MCP import-time failure of FastMCP.** If FastMCP cannot be constructed at module load, the entire `openhands.app_server.app` import fails — every FastAPI worker fails to boot. The deprecation-warning test (`tests/unit/server/routes/test_mcp_routes.py:11-40`) exists to catch one specific class of this.
- **Custom MCP merge raises.** Logged at `live_status_app_conversation_service.py:1276-1284` with `exc_info=True`; conversation continues with system config only. Note this is a soft failure — the user sees a default set of tools and may not realize their config failed.
- **ACP agent settings skip custom MCP merge entirely.** `live_status_app_conversation_service.py:1256-1257` — ACP-mode users cannot attach custom MCP servers via this path.
- **Sandbox-side MCP client cannot reach app server.** The `/mcp/mcp` URL is set unconditionally (`live_status_app_conversation_service.py:1236`) and the headers include `X-OpenHands-ServerConversation-ID`. If the sandbox cannot route to `self.web_url` (e.g., misconfigured `web_url` env var), tools fail at call time with no preflight check.
- **Two profiles trigger SwitchLLMTool.** At exactly 2 valid profiles the agent gains `SwitchLLMTool` (`live_status_app_conversation_service.py:1653-1664`); with 1 profile it doesn't. Removing a profile mid-conversation can disable a tool the agent has already discovered.
- **`register_builtins_agents` runs each conversation start.** Because `live_status_app_conversation_service.py:1619` calls `register_builtins_agents(enable_browser=True)` on every default-agent start, the SDK's global sub-agent registry can be mutated by every conversation. There is no test asserting isolation between concurrent conversations.

## Future Considerations

- **In-tree default toolset.** Currently the default tools ship in `openhands-tools`; if the team wants to iterate without bumping the SDK pin, they would need to move at least the tool factory into this repo and shadow the import.
- **Tool catalog introspection.** There is no endpoint that lists "what MCP tools does the app server currently expose?" — useful for diagnostics and for the frontend to render what's available. Would need to call `mcp_server.get_tools()` (FastMCP API) and serialize.
- **Health check for the mounted Tavily proxy.** `init_tavily_proxy` succeeds or silently fails; an explicit `/mcp/tavily/ping` or scheduled health probe would surface config drift sooner.
- **Per-server `is_enabled` gating.** Today the per-conversation gating is coarse (PLAN vs DEFAULT, plus the post-hoc SwitchLLMTool append). A per-tool `enabled` flag driven from settings would let users disable a specific MCP tool without redefining the entire set.
- **MCP server discovery from a registry.** A registry-style abstraction (a list of "available MCP sources" that the conversation builder consults) would unify the three current sources (system `default`, user `mcp_config`, future plugin-loaded servers) and make tool discovery more uniform.
- **Wholesale-replace exception for `mcp_config`.** Add a "merge servers" mode (key-by-name, deep-merge per server) for users who want to tweak one remote server without listing everything else.
- **Conflict detection between locally-defined MCP tools and user-defined MCP servers.** Today nothing prevents a user from configuring a server named `'default'` that would collide with the system entry.

## Questions / Gaps

- **What is the full list of tools that a default conversation receives?** `get_default_tools` and `get_planning_tools` live in `openhands-tools==1.29.0` and are not part of this source. The naming, schema, and approval semantics of those tools cannot be evaluated from in-tree code. (No evidence found within this source.)
- **How are MCP tool call errors reported back to the agent conversation?** Locally `ToolError` is raised (`mcp_router.py:210,284,351,418,485`) but the bridge from MCP error → SDK `ObservationEvent` lives in the SDK/agent-server and is out of scope here.
- **Is there an `is_enabled` predicate or namespace-based deferred loading?** No. Locally every `@mcp_server.tool()` is unconditionally exposed. No evidence found within this source of dynamic per-tool gating at the MCP layer.
- **How does the SDK serialize/deserialize a tool list across conversation snapshots?** `register_builtins_agents(enable_browser=True)` mutates a global registry (`live_status_app_conversation_service.py:1619`); the implications for replay and snapshot stability are SDK-level and not visible here.
- **Is there a built-in `OpenHands` MCP tool that delegates to a sub-agent?** No. Sub-agents are exposed as `AgentDefinition` objects (`live_status_app_conversation_service.py:1625`), not as MCP tools. No evidence found of a self-call MCP tool wrapping the sub-agent runner.
- **Are there tests that exercise a real MCP client → FastMCP round-trip for the locally-defined tools?** `tests/unit/server/routes/test_mcp_routes.py` covers the Tavily proxy mount and the conversation-link helpers, but the `@mcp_server.tool()`-decorated tools (`create_pr` etc.) have no end-to-end test that runs the FastMCP server and calls them. No clear evidence found.
- **What happens if a user puts two servers with the same `name` in `mcp_config`?** No collision-detection logic is visible in the merge path (`live_status_app_conversation_service.py:1269-1270`). The MCP client on the sandbox side would surface the conflict, but the app server does not pre-validate. No evidence found of pre-validation.

---

Generated by `04.01-tool-definition-and-registration` against `openhands`.