# Source Analysis: openhands

## 04.03 Tool Catalog, Discovery, and Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI V1 app server + V0 legacy); V1 mounts a fastmcp sub-app and depends on `openhands-sdk`, `openhands-tools`, `openhands-agent-server` (external) |
| Analyzed | 2026-07-27 |

## Summary

Tool catalog and discovery is split across three layers, none of which implements capability/permission-based filtering or audit-grade explainability:

1. **Static catalog** lives outside this source in `openhands.tools.preset.default` / `openhands.tools.preset.planning` (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138`). The app server just selects one of two presets at conversation start (`live_status_app_conversation_service.py:1611-1625`).
2. **MCP server aggregation** happens in the app server (`live_status_app_conversation_service.py:1219-1315`, `openhands/app_server/mcp/mcp_router.py:43-487`). A default Tavily proxy is mounted under the `tavily` namespace (`mcp_router.py:49-75`) and merged with the user's `mcp_config` (`live_status_app_conversation_service.py:1247-1280`).
3. **Skill discovery** is a thin proxy over the agent-server's `/api/skills` endpoint (`openhands/app_server/app_conversation/skill_loader.py:264-358`); the actual fan-out across global / user / project / org / sandbox sources happens inside the sandbox.

There is **no per-task or per-context tool filtering**, no per-tool ACL, and **no structured trace of which tools are exposed at run time**. Tool availability is largely determined by agent type (DEFAULT vs PLAN), a single boolean (`enable_sub_agents`), the persistent `mcp_config` stored on user settings, and the dynamic `SwitchLLMTool` injection gated on profile count (`live_status_app_conversation_service.py:1648-1664`).

## Rating

**4 / 10** — "Present but inconsistent, weakly documented, or fragile."

The model has a *static*, *binary* (DEFAULT vs PLAN) tool catalog plus user-supplied MCP servers, but it lacks dynamic capability routing, fine-grained permissioning, per-task tool selection, or any tool-availability observability. The selection logic is concentrated in two functions (`_configure_llm_and_mcp`, the `tools = ...` branch of `_build_start_conversation_request_for_user`) and there are tests confirming flag propagation, but no documented API surface for "what tools does this conversation see?" and no log/spans emit a tool inventory.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool preset import (catalog source) | `get_default_tools`, `get_planning_tools`, `register_builtins_agents` imported from external `openhands.tools.preset.*` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138` |
| Per-agent tool selection (binary) | `tools = get_planning_tools(...)` if `AgentType.PLAN`, else `tools = get_default_tools(enable_browser=True, enable_sub_agents=user.agent_settings.enable_sub_agents)` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1611-1625` |
| Sub-agent gating | `if user.agent_settings.enable_sub_agents: agent_definitions = list(get_registered_agent_definitions())` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1624-1625` |
| Dynamic built-in tool injection | `SwitchLLMTool.__name__ not in agent.include_default_tools` → append to `include_default_tools` only when ≥2 valid profiles exist | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1648-1664` |
| Built-in agents registered globally | `register_builtins_agents(enable_browser=True)` called unconditionally for DEFAULT agent | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1619` |
| MCP server catalog (default Tavily proxy) | `init_tavily_proxy()` mounts Tavily under namespace `tavily` when `tavily_api_key` is configured | `openhands/app_server/mcp/mcp_router.py:49-75`, `openhands/app_server/app.py:31` |
| MCP tool surface (`@mcp_server.tool`) | Five PR/MR-creation tools (`create_pr`, `create_mr`, `create_bitbucket_pr`, `create_bitbucket_data_center_pr`, `create_azure_devops_pr`) plus `tavily_*` (mounted) | `openhands/app_server/mcp/mcp_router.py:147-487` |
| MCP server merged into agent config | `_configure_llm_and_mcp` builds `{mcpServers: {default: {...}, ...user_servers}}` and assigns to `agent_settings.mcp_config` | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1286-1315`, `1633-1634` |
| Per-conversation MCP API key | `mcp_api_key` injected into the default MCP `X-Session-API-Key` header | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1243-1245` |
| Custom user MCP merge | `_merge_custom_mcp_config` loops over `user.agent_settings.mcp_config.mcpServers`; bypassed for ACP agents | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1247-1280`, `1256-1257` |
| Sandbox spec registry (no per-task filter) | `PresetSandboxSpecService.specs` is the only list; `search/get_default` are list scans with no capability gating | `openhands/app_server/sandbox/preset_sandbox_spec_service.py:13-48`, `openhands/app_server/sandbox/sandbox_spec_models.py:8-21` |
| Sandbox spec providers (all runtime variants expose the same tool set) | `DockerSandboxSpecService`, `ProcessSandboxSpecService`, `RemoteSandboxSpecService`, `DynamicRemoteSandboxSpecService` all produce a single, identical `SandboxSpecInfo` with no tool catalog | `openhands/app_server/sandbox/docker_sandbox_spec_service.py:35-52`, `process_sandbox_spec_service.py:21-36`, `remote_sandbox_spec_service.py:21-37`, `dynamic_remote_sandbox_spec_service.py:45-99` |
| Auto-forwarded env that affects tool/LLM behavior | `LLM_*`, `LMNR_*` prefixes auto-forwarded to the agent-server; `OH_AGENT_SERVER_ENV` JSON overrides | `openhands/app_server/sandbox/sandbox_spec_service.py:79-139` |
| Skill discovery (proxy only) | `load_skills_from_agent_server` POSTs to `/api/skills` with `load_public/user/project/org` flags | `openhands/app_server/app_conversation/skill_loader.py:264-345` |
| Skill source labels | Sources: `Sandbox`, `Global`, `User`, `Org`, `Project` (`.agents/skills/`, `.openhands/microagents/`, legacy `.openhands/skills/`) | `openhands/app_server/app_conversation/app_conversation_router.py:1320-1324`, `openhands/app_server/app_conversation/app_conversation_service_base.py:97-159` |
| Skill toggle (user-controlled disable) | `disabled_skills` filter on `Settings.disabled_skills` applied after merge | `openhands/app_server/app_conversation/app_conversation_service_base.py:238-240`, `settings_models.py:124` |
| Skills list endpoint (exposed) | `GET /app-conversations/{conversation_id}/skills` | `openhands/app_server/app_conversation/app_conversation_router.py:1307-1403` |
| Hooks list endpoint (separate from tools) | `GET /app-conversations/{conversation_id}/hooks` returns `pre_tool_use`/`post_tool_use`/`user_prompt_submit`/`session_start`/`session_end`/`stop` event types only | `openhands/app_server/app_conversation/app_conversation_router.py:1406-1540`, `app_conversation_models.py:337-347` |
| Skills directory scan (global/user) | `_load_skills_from_dir` parses markdown frontmatter under `skills/` and `~/.openhands/microagents/` | `openhands/app_server/user/skills_router.py:15-154` |
| Confirmation/permission policy (post-tool gate, not tool filter) | `_select_confirmation_policy(confirmation_mode, security_analyzer)` returns `NeverConfirm`/`AlwaysConfirm`/`ConfirmRisky`; security analyzer only `"llm"` or `"none"` | `openhands/app_server/app_conversation/app_conversation_service_base.py:614-700` |
| Tests for tool-flag propagation | `_mock_tools` patches in `test_build_request_*`, `test_build_request_passes_enable_sub_agents_true/false` | `tests/unit/app_server/test_live_status_app_conversation_service.py:879-1170` |
| Tests for skills loader | `tests/unit/app_server/test_skill_loader.py`, `tests/unit/app_server/test_app_conversation_skills_endpoint.py` | referenced from `tests/unit/app_server/` |
| No "list available tools for this conversation" endpoint | Searched: no `/tools`, `/catalog`, `/available-tools` route under `app_server/` | grep evidence above |

## Answers to Dimension Questions

1. **Does every agent see every tool?**
   - No. Tools are selected at conversation-start time from one of two presets (`get_planning_tools` for `AgentType.PLAN`, `get_default_tools` otherwise — `live_status_app_conversation_service.py:1611-1625`). MCP servers add an orthogonal tool layer per-user (`_configure_llm_and_mcp`, `live_status_app_conversation_service.py:1286-1315`). Within the DEFAULT preset the only variation is `enable_browser=True` (hard-coded) and `enable_sub_agents=user.agent_settings.enable_sub_agents` (`live_status_app_conversation_service.py:1621-1622`). The `SwitchLLMTool` is conditionally appended based on profile count (`live_status_app_conversation_service.py:1648-1664`).

2. **Are tools filtered by task?**
   - No. There is no task- or intent-based tool filtering. The same DEFAULT tool set ships to every conversation regardless of the user's prompt, repository, or trigger. The only post-selection filtering is the user-controlled `disabled_skills` list applied to *skills* (not tools) (`app_conversation_service_base.py:238-240`).

3. **Are tools filtered by permission?**
   - No static permission gating. The closest equivalent is `_select_confirmation_policy` (`app_conversation_service_base.py:641-652`), which is a runtime confirmation gate (`NeverConfirm`/`AlwaysConfirm`/`ConfirmRisky`) applied *after* a tool is invoked, not a tool-availability filter. `_create_security_analyzer_from_string` (`app_conversation_service_base.py:614-639`) only recognizes `"llm"` or `"none"`; unknown values are silently dropped to `None` and a warning is logged.

4. **Can tools be hidden from the model?**
   - Effectively no after conversation start. The model sees the static `tools` list passed via `configured_agent_settings` (`live_status_app_conversation_service.py:1627-1640`) plus everything exposed by the merged MCP servers. There is no per-iteration tool-hiding mechanism, no "do not expose `bash` for this turn" API. Hiding is only possible *before* the conversation is created, by changing the agent preset.

5. **Is tool availability explainable?**
   - No. The code emits a single info-level log of the *MCP* config (`_logger.info(f'Final MCP configuration: {sanitize_config(mcp_config)}')` — `live_status_app_conversation_service.py:1313`) and a skills summary (`Loaded {N} skills from agent-server: sources=…, names=…` — `skill_loader.py:340-343`), but **no equivalent exists for the tool inventory**. There is no endpoint exposing "which tools does this agent have", no structured event recording the tool set, and no per-tool audit trail beyond agent-server events.

## Architectural Decisions

- **Two-preset, no-dynamic-selection tool catalog.** `_build_start_conversation_request_for_user` switches between `get_planning_tools` and `get_default_tools` strictly on `AgentType` (`live_status_app_conversation_service.py:1613-1623`). This is the simplest viable scheme and matches OpenHands' public "Plan vs Code" product mode.
- **MCP as the primary extension mechanism.** Rather than ship a sprawling tool catalog, the server exposes a default `default` MCP server (`live_status_app_conversation_service.py:1236-1240`) and lets the user bring their own via `mcp_config.mcpServers` (`live_status_app_conversation_service.py:1247-1280`). The Tavily API key never reaches the sandbox because it is consumed by the proxy in `mcp_router.py:55-69`.
- **Per-conversation authentication.** MCP servers receive `X-OpenHands-ServerConversation-ID` and (optionally) `X-Session-API-Key` (`live_status_app_conversation_service.py:1237-1245`), so MCP-side tools can correlate calls to a specific conversation without global keys.
- **Sandbox-image pins the tool *runtime*, not the tool *catalog*.** `DockerSandboxSpecService`, `ProcessSandboxSpecService`, `RemoteSandboxSpecService` each pin a single image + command (`docker_sandbox_spec_service.py:35-52`, etc.) but the tool *list* comes from the SDK defaults and the user MCP config, not from the spec. Hence the catalog is server-side.
- **ACP agents intentionally bypass the OpenHands MCP layer.** `_merge_custom_mcp_config` short-circuits when `agent_settings` is `ACPAgentSettings` (`live_status_app_conversation_service.py:1256-1257`), because ACP subprocesses handle their own tool discovery.
- **Skills treated as prompts, not tools.** Skills are loaded into `AgentContext.skills` (`live_status_app_conversation_service.py:1635-1638`, `app_conversation_service_base.py:161-187`) so they enter the prompt, not the function-call surface. Discovery is centralised on the agent-server and proxied via `load_skills_from_agent_server` (`skill_loader.py:264-358`).

## Notable Patterns

- **Discriminated-union agent settings with merge semantics.** `Settings.update` uses `apply_agent_settings_diff` (`settings_models.py:215-224`) so per-field agent edits round-trip; `mcp_config` is explicitly wholesale-replaced, not deep-merged (`settings_models.py:211-214`, `tests/unit/app_server/utils/test_jsonpatch_compat.py:42-156`).
- **Default-vs-custom MCP layering.** System MCP server (default + Tavily proxy) is added first (`_add_system_mcp_servers`, `live_status_app_conversation_service.py:1219-1245`); custom user servers are then merged on top (`_merge_custom_mcp_config`, `live_status_app_conversation_service.py:1247-1280`). Order means a user server named `default` would silently overwrite the system one — there is no collision check.
- **Disabled-skill filter** is the only "allow-list"-style gate, applied to skills (`app_conversation_service_base.py:238-240`).
- **Tools not persisted on the conversation row.** `AppConversationInfo` has no `tools` field (`app_conversation_models.py:112-156`), so once chosen, the tool set is only retrievable by replaying the start request — there is no way to enumerate tools for a *past* conversation without re-loading the agent-server.

## Tradeoffs

- **Two presets vs N presets.** Simplicity wins: every code path can assume a known shape. Cost: there is no way to express "browser but not bash", "editor only", or "search-only" without forking the SDK preset.
- **User-MCP vs centrally-managed catalog.** Users can add arbitrary servers; the server does not need to ship a registry, but cannot enforce a max tool count, sanitize tool names, or detect tool conflicts across servers.
- **Skills as agent-context content vs tool calls.** Skills are injected as text, so they can be triggered by keywords (`KeywordTrigger`) or task commands (`TaskTrigger`) (`app_conversation_router.py:1371-1382`). This keeps the function-call surface small at the cost of making skill availability unobservable in tool-call traces.
- **No persistence of the resolved tool set.** Each conversation start re-runs `get_default_tools(...)` and rebuilds the MCP config; audit/replay tooling cannot compare "tools that conversation X saw" without re-running the start logic.
- **Confirmation policy applied at action time, not catalog time.** Tools are always advertised; risky ones are blocked via `ConfirmRisky` only when the user has turned on `confirmation_mode` and selected the `"llm"` analyzer (`app_conversation_service_base.py:641-652`). The model still "sees" the tools in its schema even when they will be denied at runtime.

## Failure Modes / Edge Cases

- **Unknown security-analyzer strings silently become `None`.** `_create_security_analyzer_from_string` logs a warning and returns `None` (`app_conversation_service_base.py:634-639`); a user typing `"LLM"` (uppercase) or `"insecure"` will silently lose their analyzer without an error.
- **Tool-preset import failure is a hard stop.** `from openhands.tools.preset.default import (get_default_tools, register_builtins_agents)` (`live_status_app_conversation_service.py:132-134`) runs at module load; if the wheel is missing the entire conversation service fails to import — no fallback preset.
- **Tavily proxy mount failure is logged, not raised.** `init_tavily_proxy` wraps the entire body in `try/except` and only logs `error` (`mcp_router.py:74-75`), so a misconfigured key leaves the default MCP server registered but the Tavily tool absent — agents will see `tavily_*` calls fail at runtime.
- **User MCP server named `default` shadows the system one.** `_add_system_mcp_servers` writes to `mcp_servers['default']` (`live_status_app_conversation_service.py:1237`) then `_merge_custom_mcp_config` iterates user servers with no collision detection (`live_status_app_conversation_service.py:1269-1270`); user servers win by iteration order with no warning.
- **Disabled-skill list is post-merge only.** If a skill name collides between sources, the *later* source wins during `_merge_skills` (`app_conversation_service_base.py:189-206`), so disabling by name may not affect the merged copy that actually enters the prompt.
- **MCP failures during conversation start degrade silently.** `_configure_llm_and_mcp` is wrapped by `_setup_conversation_secrets`, but custom MCP load failures log and "continue with system-generated MCP config only" (`live_status_app_conversation_service.py:1276-1283`) — agents may be missing tools the user expected without surfacing the cause.
- **Sandbox image drift vs SDK.** Custom `AGENT_SERVER_IMAGE_TAG` values are flagged via `is_custom_agent_server_image()` (`sandbox_spec_service.py:72-77`); if SDK / server major versions diverge, the tool catalog inside the sandbox can be unreachable or incompatible, and the only fix is a rebuild — no automatic rollback.
- **Skill-loader error swallowing.** `load_skills_from_agent_server` returns `[]` on any exception (`skill_loader.py:347-358`); failed skill load looks identical to "no skills configured" in logs.

## Future Considerations

- A first-class `/api/v1/conversations/{id}/tools` endpoint exposing the resolved `tools` list, MCP server set, and skill set would make tool availability explainable and diffable across conversations.
- Capability-based or role-based tool filtering (e.g. `{bash, editor}` vs `{search, jira}` subsets) would unlock smaller per-task tool surfaces and reduce wasted context tokens.
- Persisting the resolved tool/MCP/skill inventory on `AppConversationInfo` (or in events) would enable audit, replay, and post-mortem analysis.
- A central registry for MCP server metadata (display name, auth requirements, capability tags) would let the UI render "what can this server do" without re-fetching every server.
- A `confirmation_mode`-aware tool filter (refuse to register high-risk tools when the user has not opted in) would push the safety gate upstream of the model rather than only at invocation time.

## Questions / Gaps

- Where is the **canonical list** of tools returned by `get_default_tools` / `get_planning_tools`? The implementations live in the external `openhands-tools` package (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:132-138`); this source has no observable inventory of what the model actually sees.
- Is there any **server-side enforcement** that `enable_browser=True` matches the configured sandbox image? `live_status_app_conversation_service.py:1619-1621` unconditionally enables it for DEFAULT agents; the browser tool can fail at runtime if the image lacks a browser binary, with no fallback.
- Are **MCP tools subject to any name-collision check** across multiple user-defined servers? The code in `_merge_custom_mcp_config` (`live_status_app_conversation_service.py:1269-1270`) iterates without collision handling.
- Is **per-conversation tool-set persistence** intentionally absent, or deferred? No `tools` field exists on `AppConversationInfo` (`openhands/app_server/app_conversation/app_conversation_models.py:112-156`), so it appears intentional but undocumented.
- Where is the **observability story** for "which tools were used in this conversation"? Outside of conversation events emitted by the agent-server (not inspected here), there is no app-server-side log or metric recording the resolved tool set at start time. No evidence found in this source.
- Are **built-in sub-agent tool subsets** enforced server-side? `register_builtins_agents(enable_browser=True)` (`live_status_app_conversation_service.py:1619`) runs unconditionally; the SDK decides what each sub-agent sees, and the app-server cannot inspect or restrict that list.
- **No evidence found** in this source for: per-task tool filtering, runtime tool hiding, capability tags on sandbox specs, tool-availability traces in spans/events, or per-tool permission records.