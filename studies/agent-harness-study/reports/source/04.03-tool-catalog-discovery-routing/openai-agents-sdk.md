# Source Analysis: openai-agents-sdk

## 04.03 Tool Catalog, Discovery, and Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, Pydantic, OpenAI Responses / Chat Completions) |
| Analyzed | 2026-07-27 |

## Summary

The OpenAI Agents SDK has a deliberately explicit, layered tool-catalog model. The catalog is rooted on `AgentBase.tools` and `AgentBase.mcp_servers` (`src/agents/agent.py:185`, `src/agents/agent.py:188`). Per-run resolution happens in `AgentBase.get_all_tools` (`src/agents/agent.py:246-266`), which:

1. Pulls MCP tools (with optional name prefixing and reservations) via `MCPUtil.get_all_function_tools` (`src/agents/agent.py:236-244`).
2. Filters the locally-attached function tools through each `FunctionTool.is_enabled` predicate, with sync/async support (`src/agents/agent.py:250-263`).
3. Concatenates them and runs `prune_orphaned_tool_search_tools` plus a Codex-name collision check (`src/agents/agent.py:264-265`).

Handoffs are routed through a separate, parallel `get_handoffs` pipeline (`src/agents/run_internal/turn_preparation.py:88-108`) with its own `Handoff.is_enabled` predicate (`src/agents/handoffs/__init__.py:153-161`) and only get converted into the model-visible tool list at the wire boundary via `Converter.convert_tools` (`src/agents/models/openai_responses.py:1953-1954`).

Tool *capability* selection (which types of tool the agent has) is **per-agent**; there is no global registry. Inside a tool, *capability* is exposed as a small set of declarative attributes: `defer_loading` (`src/agents/tool.py:449-450`), `tool_namespace` metadata (`src/agents/tool.py:481-485`, `src/agents/tool.py:1372-1395`), `needs_approval` (`src/agents/tool.py:426-433`), `timeout_seconds` (`src/agents/tool.py:436-447`), and `is_enabled` (`src/agents/tool.py:412-415`).

*Dynamic* filtering beyond `is_enabled` exists only at two surfaces: (a) per-MCP-server `tool_filter` (static allow/block lists or a callable), declared on `MCPServer` and applied inside `_MCPServerWithClientSession._apply_tool_filter` / `_apply_static_tool_filter` / `_apply_dynamic_tool_filter` (`src/agents/mcp/server.py:621-699`, `src/agents/mcp/server.py:854-857`); and (b) per-tool `is_enabled` callable (`src/agents/tool.py:412`). There is no retrieval/RAG over the tool catalog at all — the model always sees the post-filter concrete list, not a retrieved subset.

*Routing* of a model tool call to a tool instance happens via `build_function_tool_lookup_map` and its canonical key shape (`src/agents/_tool_identity.py:13-17`, `src/agents/_tool_identity.py:352-359`). Handoffs, hosted MCP tools, computer, shell, apply_patch, and local_shell each get their own `next((...))` map for the singleton tools (`src/agents/run_internal/turn_resolution.py:1573-1586`). Cross-namespace ambiguity is rejected by `validate_function_tool_lookup_configuration` (`src/agents/_tool_identity.py:310-349`).

*Tool availability is traced* by attaching the post-filter tool list to the `AgentSpanData.tools` field of the agent span (`src/agents/run.py:1051-1055`, `src/agents/run_internal/run_loop.py:868-873`, `src/agents/tracing/span_data.py:28-61`). MCP list calls get their own `MCPListToolsSpanData` (`src/agents/tracing/span_data.py:427-451`).

Overall the model sees the smallest useful tool set through a documented, test-covered mechanism, with explicit guards against ambiguous routing and clear separation between "what's available" and "what the model was told is available" — but the SDK leans on Responses-API `tool_choice` validation, deferred loading via `ToolSearchTool`, and a single static MCP filter to keep the tool surface narrow. It is a strong implementation, but it has a small number of sharp edges: `is_enabled` is only consulted for `FunctionTool` instances (`src/agents/agent.py:251-253`), `prune_orphaned_tool_search_tools` is currently a no-op (`src/agents/tool.py:1492-1499`), and handoffs are filtered for activation but always sent through `convert_tools` after the per-agent filter.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:

- The catalog, filtering, and routing surface is fully implemented with explicit dataclasses/TypedDicts (`src/agents/tool.py:380-470`, `src/agents/mcp/util.py:118-133`, `src/agents/_tool_identity.py:13-17`) and is covered by tests (`tests/test_function_tool.py:465-498`, `tests/mcp/test_tool_filtering.py:32-77`, `tests/mcp/test_tool_filtering.py:84-181`, `tests/test_handoff_tool.py:386-454`, `tests/test_tool_identity.py:1-167`).
- Operationally, deferred loading and namespace metadata give a real "tool search" model (`src/agents/tool.py:1372-1405`, `src/agents/tool.py:1435-1489`) and `validate_responses_tool_search_configuration` rejects ambiguous combinations (`src/agents/tool.py:1463-1489`).
- Availability is explainable: `AgentSpanData.tools` records the post-filter list per turn (`src/agents/run.py:1051-1055`) and `MCPListToolsSpanData` records the per-server list (`src/agents/tracing/span_data.py:427-451`).
- Tradeoffs that keep it at 7 rather than 8: `is_enabled` is silently ignored for non-`FunctionTool` instances inside `get_all_tools` (`src/agents/agent.py:250-253`); `prune_orphaned_tool_search_tools` is a no-op stub (`src/agents/tool.py:1492-1499`); there is no retrieval/RAG over the tool catalog itself; and the realtime path projects `get_all_tools` into a session payload but the model emits tool calls into a separate dispatch loop (`src/agents/realtime/session.py:816-833`).

## Evidence Collected

Every entry includes a file path and line numbers, formatted as `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-agent tool list (global field) | `AgentBase.tools: list[Tool] = field(default_factory=list)` | `src/agents/agent.py:185` |
| Per-agent MCP server list | `AgentBase.mcp_servers: list[MCPServer] = field(default_factory=list)` | `src/agents/agent.py:188` |
| Per-agent MCP config | `mcp_config: MCPConfig` (convert_schemas_to_strict, failure_error_function, include_server_in_tool_names) | `src/agents/agent.py:199-200`, `src/agents/agent.py:139-156` |
| Reserved-name computation for MCP prefixing | `_get_mcp_tool_reserved_names` adds hand-off tool names to the reserved set | `src/agents/agent.py:202-222` |
| `get_all_tools` pipeline: MCP tools → `is_enabled` filter → merge → orphan-prune → name-collision check | Combined `await get_mcp_tools(...)` + `asyncio.gather(*(_check_tool_enabled))` + `_validate_codex_tool_name_collisions` | `src/agents/agent.py:246-266`, `src/agents/agent.py:95-118` |
| `is_enabled` accepts bool or callable, sync/async | `_check_tool_enabled` inspects attribute type, awaits if needed | `src/agents/agent.py:250-263` |
| `is_enabled` only applied to `FunctionTool` | `if not isinstance(tool, FunctionTool): return True` | `src/agents/agent.py:251-253` |
| `prune_orphaned_tool_search_tools` is a no-op (declared in same module) | `return tools` | `src/agents/tool.py:1492-1499` |
| Codex tool name collision validation | `_validate_codex_tool_name_collisions` raises on duplicate names | `src/agents/agent.py:95-118` |
| MCP tool listing with retries/caching | `_tools_list`, `_cache_dirty`, `list_tools` | `src/agents/mcp/server.py:607-609`, `src/agents/mcp/server.py:829-867` |
| MCP static tool filter (allow/block) | `_apply_static_tool_filter` | `src/agents/mcp/server.py:641-657` |
| MCP dynamic tool filter (callable) | `_apply_dynamic_tool_filter` builds a `ToolFilterContext` | `src/agents/mcp/server.py:659-699` |
| Filter context shape | `ToolFilterContext(run_context, agent, server_name)` | `src/agents/mcp/util.py:89-100` |
| Static filter dict type | `ToolFilterStatic = TypedDict` with `allowed_tool_names` / `blocked_tool_names` | `src/agents/mcp/util.py:118-128` |
| Dynamic filter callable type | `ToolFilterCallable = Callable[[ToolFilterContext, MCPTool], MaybeAwaitable[bool]]` | `src/agents/mcp/util.py:103-115` |
| `create_static_tool_filter` convenience | Builds the dict; returns `None` if both lists are None | `src/agents/mcp/util.py:209-233` |
| MCP → `FunctionTool` conversion (one per server tool) | `MCPUtil.to_function_tool` produces a `FunctionTool` with `mcp_title` and `ToolOrigin(type=MCP, mcp_server_name=...)` | `src/agents/mcp/util.py:498-568` |
| `include_server_in_tool_names` prefixes public names to avoid collisions | `_build_prefixed_tool_name_overrides`, `_shorten_tool_name`, SHA1 suffix for collisions | `src/agents/mcp/util.py:407-496` |
| `Tool` union catalog (what the SDK can route) | `FunctionTool | FileSearchTool | WebSearchTool | ComputerTool | HostedMCPTool | CustomTool | ShellTool | ApplyPatchTool | LocalShellTool | ImageGenerationTool | CodeInterpreterTool | ToolSearchTool` | `src/agents/tool.py:1355-1368` |
| Function-tool capability metadata | `name`, `description`, `params_json_schema`, `strict_json_schema`, `is_enabled`, `needs_approval`, `timeout_*`, `defer_loading` | `src/agents/tool.py:380-470` |
| Tool origin tracking (mcp/agent/function) | `ToolOrigin`, `ToolOriginType`, `get_function_tool_origin` | `src/agents/tool.py:270-322`, `src/agents/tool.py:654-658` |
| `tool_namespace` group for hosted tool search | Sets `_tool_namespace` / `_tool_namespace_description`; rejects reserved synthetic namespaces | `src/agents/tool.py:1372-1395`, `src/agents/_tool_identity.py:294-307` |
| `defer_loading` capability on FunctionTool | `defer_loading: bool = False` | `src/agents/tool.py:449-450` |
| Hosted Responses tool-search configuration validation | `validate_responses_tool_search_configuration` enforces `ToolSearchTool` + searchable surface rules | `src/agents/tool.py:1463-1489` |
| `has_required_tool_search_surface` and friends | Helpers that report whether a config needs `ToolSearchTool()` | `src/agents/tool.py:1435-1460` |
| `ToolSearchTool` (hosted) | `execution: Literal["server", "client"] | None` — client mode is not auto-executed | `src/agents/tool.py:1338-1352` |
| Function-tool lookup key model (3 key kinds) | `BareFunctionToolLookupKey`, `NamespacedFunctionToolLookupKey`, `DeferredTopLevelFunctionToolLookupKey` | `src/agents/_tool_identity.py:10-17` |
| `get_function_tool_lookup_key_for_call` resolves any tool call to a key | Tuple-keyed dispatch | `src/agents/_tool_identity.py:97-102` |
| `build_function_tool_lookup_map` builds dispatch map; last-wins precedence | `tool_map[lookup_key] = tool` for each lookup key | `src/agents/_tool_identity.py:352-359` |
| Cross-tool name/namespace collision validation | `validate_function_tool_lookup_configuration` raises `UserError` on duplicates | `src/agents/_tool_identity.py:310-349` |
| Reserved-synthetic-namespace guard | `is_reserved_synthetic_tool_namespace` (name == namespace is reserved) | `src/agents/_tool_identity.py:52-60`, `src/agents/_tool_identity.py:294-307` |
| Handoff `is_enabled` predicate | `Handoff.is_enabled: bool \| Callable[[RunContextWrapper, AgentBase], MaybeAwaitable[bool]]` | `src/agents/handoffs/__init__.py:153-161` |
| Handoff `input_filter` (input history, not tool list) | `input_filter: Callable[[HandoffInputData], HandoffInputData] | None` | `src/agents/handoffs/__init__.py:192`, `src/agents/extensions/handoff_filters.py:33-77` |
| Handoff tool name derivation | `Handoff.default_tool_name(agent)` → `transfer_to_<name>` | `src/agents/handoffs/__init__.py:172-176` |
| Run-time handoff filter | `get_handoffs(agent, context_wrapper)` runs `is_enabled` on each handoff | `src/agents/run_internal/turn_preparation.py:88-108` |
| Per-turn handoff list captured in `AgentSpanData.handoffs` | `handoff_names = [h.agent_name for h in await get_handoffs(...)]` | `src/agents/run.py:1036-1039` |
| Per-turn tool list captured in `AgentSpanData.tools` | `current_span.span_data.tools = [tool_name for tool in all_tools ...]` | `src/agents/run.py:1051-1055` |
| Streaming mirror | Same assignment in streaming loop | `src/agents/run_internal/run_loop.py:868-873` |
| `AgentSpanData` shape | `name, handoffs, tools, output_type, metadata` | `src/agents/tracing/span_data.py:28-61` |
| `MCPListToolsSpanData` records per-server listed tools | `server`, `result: list[str]` | `src/agents/tracing/span_data.py:427-451` |
| `mcp_tools_span` factory | Sets `span_data.result` to `[tool.name for tool in tools]` | `src/agents/mcp/util.py:333-342` |
| Responses model tool conversion entry point | `Converter.convert_tools` builds a `ConvertedTools` with `tools` + `includes` | `src/agents/models/openai_responses.py:1876-1959` |
| Namespace grouping into `tool_search` payload | `converted_tools.append({"type": "namespace", ...})` | `src/agents/models/openai_responses.py:1944-1951` |
| Handoff tool addition at wire boundary | `converted_tools.append(cls._convert_handoff_tool(handoff))` | `src/agents/models/openai_responses.py:1953-1954` |
| `defer_loading` is forwarded to wire payload | `_convert_function_tool` adds `defer_loading: True` when set | `src/agents/models/openai_responses.py:1961-1977` |
| `tool_choice` validation against deferred surfaces | `_validate_required_tool_choice`, `_validate_named_function_tool_choice` reject broken configurations | `src/agents/models/openai_responses.py:1688-1783` |
| `parallel_tool_calls` derived from tools | `if model_settings.parallel_tool_calls and tools: parallel_tool_calls = True` | `src/agents/models/openai_responses.py:739-744` |
| Function-tool dispatch within a turn | `function_map = build_function_tool_lookup_map(...)` over `all_tools` | `src/agents/run_internal/turn_resolution.py:1574-1576` |
| Singleton tool dispatch (computer/shell/etc.) | `next((... for tool in all_tools if isinstance(tool, X)), None)` | `src/agents/run_internal/turn_resolution.py:1577-1581` |
| Hosted MCP server dispatch | `hosted_mcp_server_map[tool.tool_config["server_label"]] = tool` | `src/agents/run_internal/turn_resolution.py:1582-1586` |
| MCP listing per turn is wrapped in a span | `_list_tools_with_span` opens an `mcp_tools_span` | `src/agents/mcp/util.py:333-342` |
| Computer tool lazy initialization per run context | `resolve_computer` (weakref-keyed cache, per-run computer) | `src/agents/tool.py:764-813` |
| Approval gating before execution | `needs_approval` (FunctionTool / ShellTool / ApplyPatchTool / CustomTool) + MCP `on_approval_request` | `src/agents/tool.py:426-433`, `src/agents/tool.py:883-886`, `src/agents/tool.py:951-956`, `src/agents/tool.py:1219-1231`, `src/agents/tool.py:1267-1279` |
| Capability-based sandbox tool binding | `capability_tools = [tool for capability in capabilities for tool in capability.tools()]` | `src/agents/sandbox/runtime_agent_preparation.py:82`, `src/agents/sandbox/runtime_agent_preparation.py:106` |
| `Prompt` (Responses prompt) replaces local tools when set | `should_omit_tools = prompt is not None and len(converted_tools_payload) == 0` | `src/agents/models/openai_responses.py:778-784` |
| `call_model_input_filter` final input filter (not tool list) | `maybe_filter_model_input` returns a new `ModelInputData` | `src/agents/run_internal/turn_preparation.py:51-85` |
| RunState re-resolution of tool map on resume | `all_tools = await current_agent.get_all_tools(context)` | `src/agents/run_state.py:1731-1735` |
| Tests: `is_enabled` bool/callable for function tools | `tests/test_function_tool.py:465-498` | `tests/test_function_tool.py:465-498` |
| Tests: `is_enabled` for agent-as-tool | `tests/test_agent_as_tool.py:55-218` | `tests/test_agent_as_tool.py:55-218` |
| Tests: deferred tool with `is_enabled` filtering | `tests/test_function_tool.py:501-522` | `tests/test_function_tool.py:501-522` |
| Tests: tool-search surface preserved when only deferred tools are disabled | `tests/test_function_tool.py:541-549` | `tests/test_function_tool.py:541-549` |
| Tests: MCP static filtering (allow/block/both/none) | `tests/mcp/test_tool_filtering.py:32-77` | `tests/mcp/test_tool_filtering.py:32-77` |
| Tests: MCP dynamic filtering (sync/async, context, error handling) | `tests/mcp/test_tool_filtering.py:80-181` | `tests/mcp/test_tool_filtering.py:80-181` |
| Tests: agent integration of dynamic MCP filtering | `tests/mcp/test_tool_filtering.py:187-246` | `tests/mcp/test_tool_filtering.py:187-246` |
| Tests: handoff `is_enabled` bool/callable/async | `tests/test_handoff_tool.py:386-454` | `tests/test_handoff_tool.py:386-454` |
| Tests: tool lookup-key helpers and reserved-namespace guard | `tests/test_tool_identity.py:1-167` | `tests/test_tool_identity.py:1-167` |
| Tests: agent-span snapshot includes `tools` and `handoffs` lists | `tests/test_agent_tracing.py:50-70` | `tests/test_agent_tracing.py:50-70` |

## Answers to Dimension Questions

1. **Does every agent see every tool?**
   No. Each agent has its own `tools` and `mcp_servers` fields (`src/agents/agent.py:185`, `src/agents/agent.py:188`). `Agent.get_all_tools` produces a per-run list scoped to the agent's own configuration plus its MCP servers (`src/agents/agent.py:246-266`). The realtime session also pulls per-agent tools at session start (`src/agents/realtime/openai_realtime.py:442-448`). Handoffs are filtered per agent via `get_handoffs` (`src/agents/run_internal/turn_preparation.py:88-108`).

2. **Are tools filtered by task?**
   Indirectly. Per-tool `is_enabled` is a `(RunContextWrapper, AgentBase) -> bool` callable so it can inspect runtime context (role, state, etc.) (`src/agents/tool.py:412-415`, `src/agents/agent.py:250-263`). Per-MCP-server `tool_filter` accepts a callable that receives `ToolFilterContext` (run_context + agent + server_name) and an `MCPTool` (`src/agents/mcp/server.py:659-699`, `src/agents/mcp/util.py:89-100`). There is no separate "task" identifier handed to the filter; filtering is keyed on run context, agent identity, and tool identity. The `Prompt` (Responses prompt) can also replace the local tool payload with server-managed tools (`src/agents/models/openai_responses.py:766-784`).

3. **Are tools filtered by permission?**
   There is no separate permission model. Permissions are expressed through:
   - `FunctionTool.needs_approval` (bool or `(ctx, params, call_id) -> Awaitable[bool]`) at invocation time (`src/agents/tool.py:426-433`).
   - `Handoff.is_enabled` and `FunctionTool.is_enabled` for *availability* (`src/agents/handoffs/__init__.py:153-161`, `src/agents/tool.py:412-415`).
   - MCP server `require_approval` callback (`src/agents/mcp/util.py:546-548`).
   - Handoff `input_filter` and `RunConfig.handoff_input_filter` strip tool items from a handoff's history (`src/agents/extensions/handoff_filters.py:33-77`, `docs/running_agents.md:141`).
   These act as admission control, not as a static permission table; the SDK delegates "who is allowed to call this" to user-supplied predicates.

4. **Can tools be hidden from the model?**
   Yes, in three distinct ways:
   - `FunctionTool.is_enabled = False` (or a callable returning `False`) removes the tool from `Agent.get_all_tools` and therefore from the wire payload (`src/agents/agent.py:250-263`).
   - `defer_loading=True` marks a tool as not in the initial wire payload; the model can load it via `ToolSearchTool` (`src/agents/tool.py:449-450`, `src/agents/tool.py:1463-1489`).
   - `tool_namespace(...)` groups deferred tools into a `{"type": "namespace", ...}` payload so they are visible only as a discoverable group (`src/agents/tool.py:1372-1395`, `src/agents/models/openai_responses.py:1944-1951`).
   - `HostedMCPTool(tool_config={..., "defer_loading": True})` is similarly hidden until tool search loads it (`src/agents/mcp/util.py:498-568`, `src/agents/tool.py:1435-1460`).

5. **Is tool availability explainable?**
   Yes, primarily through tracing. The post-filter tool list is written to `AgentSpanData.tools` and `AgentSpanData.handoffs` at every turn (`src/agents/run.py:1051-1055`, `src/agents/run_internal/run_loop.py:868-873`, `src/agents/tracing/span_data.py:28-61`). MCP listings emit a dedicated `mcp_tools` span with the resolved names (`src/agents/tracing/span_data.py:427-451`, `src/agents/mcp/util.py:333-342`). RunState re-derives the lookup map at resume time using the agent's own `get_all_tools` (`src/agents/run_state.py:1731-1735`), so the explanation is reproducible. There is no built-in "tool catalog" span that records *why* each tool was filtered, but the agent span plus MCP span together record *what* was passed to the model and *what* was returned by the MCP server.

## Architectural Decisions

- **Per-agent tool list, no global registry.** Tools are owned by `Agent` (or `RealtimeAgent`); there is no central `ToolRegistry` (`src/agents/agent.py:185`, `src/agents/realtime/agent.py:85`). This keeps the data model simple and local.
- **MCP tools are a per-server list, not a per-tool list.** Each `MCPServer` is a single configuration object that exposes its own list and accepts a single `tool_filter` (`src/agents/mcp/server.py:611`, `src/agents/mcp/util.py:131-133`). Server-level filtering keeps the surface narrow even when the underlying server has many tools.
- **Two parallel tool surfaces — handoffs and tools — joined only at the wire boundary.** Handoff filtering runs through `get_handoffs` (`src/agents/run_internal/turn_preparation.py:88-108`); function-tool filtering runs through `Agent.get_all_tools` (`src/agents/agent.py:246-266`). They are merged in `Converter.convert_tools` (`src/agents/models/openai_responses.py:1876-1959`), so the model sees a single catalog but each side can be filtered independently.
- **Tool identity is encoded in a 3-kind key.** `get_function_tool_lookup_key` returns `("bare", name) | ("namespaced", namespace, name) | ("deferred_top_level", name)` (`src/agents/_tool_identity.py:83-94`). The third key is a synthetic shape reserved by hosted tool search to keep the wire dispatch unambiguous (`src/agents/_tool_identity.py:52-60`, `src/agents/_tool_identity.py:228-234`).
- **Hosted Responses tool search is opt-in and validated.** `validate_responses_tool_search_configuration` enforces that `ToolSearchTool` is present whenever deferred surfaces exist, and that at most one `ToolSearchTool` is configured (`src/agents/tool.py:1463-1489`). The validator also rejects ambiguous namespace shapes (`src/agents/_tool_identity.py:294-307`).
- **Capability attribution is per tool, not per agent.** Each `FunctionTool` carries its own `defer_loading`, `needs_approval`, `timeout_*`, and namespace metadata (`src/agents/tool.py:380-470`). There is no separate `ToolCapability` table; the capability *is* the tool.
- **Tool availability is recorded on the agent span, not on a separate span.** The chosen design is to attach `tools: list[str]` to `AgentSpanData` (`src/agents/tracing/span_data.py:28-61`), so a single span describes both handoffs and tools for that agent. This makes "what did this agent see?" one span export away.

## Notable Patterns

- **Allow/block list composition.** `_apply_static_tool_filter` applies the allow list *first*, then the block list (`src/agents/mcp/server.py:641-657`). This makes "allow a few, block one" expressive without forcing users to spell out the intersection.
- **Async-friendly dynamic filter.** Dynamic filters are called with `inspect.isawaitable(result)` and may be sync or async; errors are logged and treated as "exclude the tool" (`src/agents/mcp/server.py:681-697`). The pattern is documented in the MCP guide (`docs/mcp.md:412-439`).
- **Synthesized namespace for deferred top-level tools.** To keep the lookup unambiguous, deferred top-level tools are routed through a synthetic (`deferred_top_level`, name) key and their on-wire calls are stripped of that namespace before dispatch (`src/agents/_tool_identity.py:174-191`, `src/agents/_tool_identity.py:284-291`).
- **Last-wins lookup map with explicit validation.** `build_function_tool_lookup_map` overwrites earlier entries; the precondition is `validate_function_tool_lookup_configuration` which raises `UserError` for ambiguous combinations (`src/agents/_tool_identity.py:310-359`). The runner never has to defensively handle "two tools with the same name" silently.
- **Tool origin metadata.** `ToolOrigin(type=FUNCTION|MCP|AGENT_AS_TOOL, mcp_server_name, agent_name, agent_tool_name)` is attached to each `FunctionTool` and serialized with `to_json_dict`/`from_json_dict` (`src/agents/tool.py:278-322`, `src/agents/tool.py:654-658`). This makes it possible to attribute a tool call to its source after the fact.
- **Prompt-managed tool surface.** When a Responses `Prompt` is set, the SDK may *omit* the local `tools` payload entirely, letting the server define the tool surface (`src/agents/models/openai_responses.py:766-784`). The validator allows an opaque `tool_search` surface in this mode so the prompt can still reference hosted tool search (`src/agents/tool.py:1463-1489`).
- **Handoff isolation of state.** `as_tool()` runs the nested agent with a fresh `ToolContext` and an `agent_tool_state_scope` so the child cannot see or mutate the parent's approval state directly (`src/agents/agent.py:599-697`). This is a tool-isolation pattern more than a tool-discovery one, but it materially shapes what tools the nested agent can observe.

## Tradeoffs

- **Local function tools can be filtered, but hosted tools cannot be `is_enabled`-filtered inside `Agent.get_all_tools`.** The `_check_tool_enabled` lambda short-circuits to `True` for anything that isn't a `FunctionTool` (`src/agents/agent.py:250-253`). This means `WebSearchTool`, `FileSearchTool`, `ComputerTool`, `HostedMCPTool`, etc. are always passed to the model as long as they are on the agent. If you need to gate hosted tools at runtime, the only documented surface today is to remove them from `Agent(tools=[...])` and add them back dynamically (the SDK does not provide a per-call hook here).
- **Handoffs are filtered for activation but not for catalog visibility.** `Handoff.is_enabled` decides whether the handoff is *callable* in this run (`src/agents/run_internal/turn_preparation.py:88-108`). The disabled handoff's tool name never appears in the model wire payload because `Converter.convert_tools` only iterates the filtered list (`src/agents/models/openai_responses.py:1953-1954`). However, the agent span records the *post-filter* list, so an observer sees only the *active* handoffs, not the *available-but-disabled* ones. This is fine, but worth noting if you need a "shadow" explainability.
- **MCP `tool_filter` runs at listing time, not at call time.** If the server caches its tool list (`src/agents/mcp/server.py:829-857`), changes to `tool_filter` are not picked up until `invalidate_tools_cache()` is called (`src/agents/mcp/server.py:715-717`). The model also sees the filtered list, so a tool that is filtered out at listing time never reaches the model.
- **Dynamic tool addition has a narrow channel.** The agent's `tools` list is fixed at construction time, but tests in `tests/test_agent_runner.py:4424` and `tests/test_agent_runner_streamed.py:1634` exercise `test_dynamic_tool_addition_run`, suggesting a mechanism to refresh the tool list mid-run exists. No clear top-level "re-resolve and rebind tools" API is exposed; the behavior is implicit in the run loop.
- **The `prune_orphaned_tool_search_tools` is currently a no-op.** It returns its input unchanged and is annotated as "preserves misconfiguration until request conversion validates it" (`src/agents/tool.py:1492-1499`). In practice this means invalid `ToolSearchTool` configurations are caught later in `validate_responses_tool_search_configuration` (`src/agents/tool.py:1463-1489`) and in `Converter.convert_tools` (`src/agents/models/openai_responses.py:1895-1898`).
- **Realtime tool routing is a separate, simpler path.** The realtime path uses `agent.get_all_tools` and a name→tool map (`src/agents/realtime/session.py:816-833`), so the per-call logic does not go through the rich function-tool lookup machinery used by `run_internal/turn_resolution.py`. Realtime also requires plain `FunctionTool` instances and rejects Responses-only features (`src/agents/realtime/openai_realtime.py:1536-1542`).

## Failure Modes / Edge Cases

- **A function tool can be `is_enabled=False` while another tool uses the same name.** The validator only catches cross-namespace and deferred-name collisions (`src/agents/_tool_identity.py:310-349`); two `FunctionTool`s with the same `name` and no namespace are tolerated (`src/agents/_tool_identity.py:341-342`). In `build_function_tool_lookup_map` this results in a last-wins map (`src/agents/_tool_identity.py:352-359`), which is silently lossy.
- **A `ToolSearchTool()` without a searchable surface raises `UserError` only at conversion time.** `prune_orphaned_tool_search_tools` does not preempt this (`src/agents/tool.py:1492-1499`); the error surfaces when `Converter.convert_tools` runs `validate_responses_tool_search_configuration` (`src/agents/models/openai_responses.py:1895-1898`). If you call `Agent.get_all_tools` outside of a model invocation, you will not see this error.
- **A deferred-loading function tool whose `is_enabled` returns `False` is dropped from the wire payload but `ToolSearchTool` is kept on the agent.** `tests/test_function_tool.py:501-522` documents this; the SDK intentionally keeps `ToolSearchTool` so the model can still search the namespace even if the concrete tool is currently disabled.
- **MCP dynamic filter exceptions silently exclude the tool.** `_apply_dynamic_tool_filter` logs and continues, treating the tool as filtered out (`src/agents/mcp/server.py:691-697`). This is the right default for safety, but it means a bug in the filter hides the tool without a stack trace to the agent runner.
- **A computer tool that is not initialized is a `UserError` at wire-conversion time.** `Converter._convert_preview_computer_tool` raises unless `tool.computer` is a `Computer | AsyncComputer` and has `environment`/`dimensions` (`src/agents/models/openai_responses.py:1979-2002`). `initialize_computer_tools` is called in the run loop to populate it (`src/agents/run.py:1031-1033`).
- **MCP server `include_server_in_tool_names=True` requires the `tools` list to be the agent's MCP server list at resolution time.** Prefixed names are assigned in a single batch (`src/agents/mcp/util.py:436-496`), and collisions with reserved names (existing function tools + active handoffs) force hashing/suffixing (`src/agents/mcp/util.py:482-491`).
- **The Codex tool name collision validator runs only inside `get_all_tools` (`src/agents/agent.py:265`), not at agent construction.** Constructing an `Agent(tools=[...])` with duplicate Codex tool names does not raise; the error only surfaces on the first model call.
- **Two agents with the same name may be ambiguous during approval resolution in legacy `RunState` schemas**; the `turn_resolution` module has a dedicated `_allow_legacy_name_agent_match` to handle this, with schema version gating (`src/agents/run_internal/turn_resolution.py:1140-1146`).

## Future Considerations

- **Activate `prune_orphaned_tool_search_tools` or remove it.** It currently does nothing (`src/agents/tool.py:1492-1499`) and its docstring warns that it *hides* misconfiguration rather than surfacing it. Either implement the prune or delete the symbol to remove the false promise.
- **Generalize `is_enabled` to non-`FunctionTool` tools.** The current filter only applies to `FunctionTool` (`src/agents/agent.py:250-253`); extending it to a `Tool.is_enabled` predicate (with a default `True`) would let users gate hosted tools and codex tools without removing them from the list.
- **Add an explicit "tool catalog" trace span.** Today, the agent span records the names (`src/agents/tracing/span_data.py:28-61`); a dedicated span could record *origin* (function/MCP/agent-as-tool/codex) and *filter decisions* (allow, block, exception). The `ToolOrigin` and `ToolFilterContext` data already exist to populate it.
- **Expose a programmatic API to re-resolve the tool list mid-run.** The runner appears to allow this in tests (`tests/test_agent_runner.py:4424`, `tests/test_agent_runner_streamed.py:1634`), but no public hook is documented. A `set_dynamic_tools(...)` or `before_model_call` hook would let advanced users rebuild the catalog with new MCP servers or capability tools.
- **Bring the realtime and async tool-routing paths closer together.** Realtime builds a name→tool map (`src/agents/realtime/session.py:820-833`) and ignores the function-tool lookup key machinery used by `turn_resolution` (`src/agents/run_internal/turn_resolution.py:1573-1586`). Reusing `build_function_tool_lookup_map` would let deferred-loading / namespace semantics flow into realtime.
- **Add a "smallest useful tool set" assertion API.** Given that the SDK already has `get_all_tools` and `validate_responses_tool_search_configuration`, an `assert_tool_choice_resolvable(tools, handoffs, tool_choice)` would make misconfigurations diagnosable *before* a model call, not at conversion time.
- **Document and test the dynamic-tool-addition path.** The run loop's tool resolution is recomputed per turn (`src/agents/run.py:1030-1033`), but the user-facing surface for mutating the catalog mid-run is undocumented.

## Questions / Gaps

- **How is the canonical "what the model saw" list reconstructed on a resumed `RunState`?** `run_state.py:1731-1735` re-calls `current_agent.get_all_tools(context)`, which means the resumed run's tool list depends on the *current* state of `agent.tools` and the *current* MCP server filters, not on the snapshot taken when the run was first started. There is no public helper that returns the exact wire payload used in a past turn.
- **What guarantees are made about the order of `Agent.tools` vs `MCP` tools in the wire payload?** `convert_tools` iterates tools in their given order (`src/agents/models/openai_responses.py:1904-1940`) but `Agent.get_all_tools` concatenates `[*mcp_tools, *enabled]` (`src/agents/agent.py:264`). Tests do not appear to assert on order; the SDK does not promise one.
- **Is the `is_enabled` callable allowed to mutate `agent.tools` or `RunContextWrapper`?** Nothing in the code prevents it (`src/agents/agent.py:250-263`), but doing so would create surprising behavior. No documentation warns against it.
- **What is the expected behavior when two `RealtimeAgent`s have overlapping function tool names?** The realtime path builds `function_map` keyed by `tool.name` (`src/agents/realtime/session.py:820`), so duplicate names overwrite each other silently — same as `build_function_tool_lookup_map`'s last-wins.
- **No evidence found** for: retrieval/RAG over the tool catalog (i.e., the model *retrieves* tool definitions from a vector store). The only `tool_search` is the hosted Responses one (`src/agents/tool.py:1338-1352`), which is server-driven and pre-validated. If the user expects a model-driven vector search over local tool definitions, it does not exist.
- **No evidence found** for: a tool being dynamically added *between* two `get_all_tools` calls within the same turn. The run loop calls `get_all_tools` once per turn (`src/agents/run.py:1030-1033`, `src/agents/run_internal/run_loop.py:850-851`); mid-turn tool list mutation is not a documented surface.
- **No evidence found** for: per-agent `tool_choice` enforcement. `tool_choice` lives on `ModelSettings` (`src/agents/agent.py:318-320`) and is converted in `Converter.convert_tool_choice` (`src/agents/models/openai_responses.py:1616-1686`); per-tool `tool_choice` is not a first-class concept.
