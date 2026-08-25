# Source Analysis: crewai

## 04.03 Tool Catalog, Discovery, and Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic-based agents, LiteLLM/OpenAI/Anthropic/Bedrock/Gemini/Azure, native MCP, in-repo skill loader) |
| Analyzed | 2026-07-27 |

## Summary

CrewAI has a wide but **ad-hoc** tool catalog surface. Tools enter the
catalog from six, mostly independent, sources: an explicit `tools`
list on `Agent` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:283-285`),
an explicit `tools` list on `Task` (`lib/crewai/src/crewai/task.py:210-213`),
an `apps` list resolved through `CrewaiPlatformTools`
(`lib/crewai/src/crewai/agent/core.py:1177-1186`), an `mcps` list
resolved through `MCPToolResolver`
(`lib/crewai/src/crewai/agent/core.py:1188-1197`,
`lib/crewai/src/crewai/mcp/tool_resolver.py:68-89`), an auto-injected
delegation pair (`lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:16-36`),
and an auto-injected memory pair
(`lib/crewai/src/crewai/tools/memory_tools.py:104-130`,
`lib/crewai/src/crewai/crew.py:1653-1654`).

There is no central "tool catalog" object. The list a given task sees is
assembled at task-prep time by `Crew._prepare_tools`
(`lib/crewai/src/crewai/crew.py:1616-1683`), which is a long
conditional that conditionally injects each tool category. Tasks may
further override the agent's own `tools` list — `task.tools` shadows
`agent.tools` (`lib/crewai/src/crewai/crews/utils.py:179`,
`lib/crewai/src/crewai/task.py:542-546`) — and per-task overrides are
**not** merged with delegation, platform, MCP, or memory injection; the
order is "task first, then augmentations".

Discovery happens at three places: (1) MCP servers are discovered by
`MCPToolResolver` at `kickoff` time via `list_tools()`
(`lib/crewai/src/crewai/mcp/tool_resolver.py:333-402`); (2) MCP tool
schemas are cached in a module-level `_mcp_schema_cache` for 300 s
(`lib/crewai/src/crewai/mcp/tool_resolver.py:41-42`,
`lib/crewai/src/crewai/mcp/tool_resolver.py:494-519`); (3) agent
"skills" are discovered on disk through `discover_skills` and
`activate_skill` (`lib/crewai/src/crewai/skills/loader.py:36-143`).
Skills, however, are **prompt-injected metadata**, not tools — they
never reach `parse_tools` or the executor's `tools` list.

Routing is name-based and lives in two parallel paths: a text/ReAct path
in `ToolUsage._select_tool` that uses `SequenceMatcher` with a 0.85
threshold to pick a tool by name
(`lib/crewai/src/crewai/tools/tool_usage.py:759-802`), and a
native-tool-call path in `AgentExecutor._execute_single_native_tool_call`
that picks by `sanitize_tool_name(tool.name)` from
`self._available_functions` and `self._tool_name_mapping`
(`lib/crewai/src/crewai/experimental/agent_executor.py:1860-2024`).
Failures in both paths are observable: a `ToolSelectionErrorEvent`
fires with the requested name and the full `tools_description`
(`lib/crewai/src/crewai/tools/tool_usage.py:786-802`,
`lib/crewai/src/crewai/events/types/tool_usage_events.py:86-90`).

Dynamic filtering is supported **only for MCP servers**, via
`StaticToolFilter` (allow/block lists) and `create_dynamic_tool_filter`
(callable with `ToolFilterContext`)
(`lib/crewai/src/crewai/mcp/filters.py:17-163`,
`lib/crewai/src/crewai/mcp/tool_resolver.py:383-402`). Non-MCP tools
have no runtime filter — once in `agent.tools` they remain in the
catalog for that agent for the lifetime of the agent. The only
capability-style filter outside MCP is the **read-only memory flag**:
`Memory._read_only` removes `RememberTool` from the auto-injected memory
catalog (`lib/crewai/src/crewai/tools/memory_tools.py:123-129`).

There is no retrieval step for tools. The agent always sees every tool
on its `tools` list plus whatever delegation/platform/MCP/memory
augmentations the crew adds for the active task; nothing is queried,
ranked, or omitted based on task content.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational
safeguards** (the upper end of the band, leaning toward 7).

Justification:

- The catalog model is explicit: there are concrete types
  (`BaseTool`, `CrewStructuredTool`, `MCPNativeTool`, `MCPToolWrapper`,
  `ReadFileTool`, `RecallMemoryTool`, `RememberTool`,
  `DelegateWorkTool`, `AskQuestionTool`,
  `CrewAIPlatformActionTool`), each with a defined role and an
  injection site in `Crew._prepare_tools`.
- Discovery, where it exists, is well-defined: MCP `list_tools()` +
  cached schemas + retry-with-backoff, plus `discover_skills` /
  `activate_skill` for SKILL.md-style metadata.
- Routing is name-based and **observable**: the dispatcher
  (`AgentExecutor._execute_single_native_tool_call`,
  `ToolUsage._select_tool`) emits `ToolSelectionErrorEvent` with the
  rejected name and the full available list, so "why was this tool
  hidden" is partly traceable through events.
- The per-task override semantics are documented in
  `Crew._prepare_tools` (tool categories are gated on
  `agent.tools`/`task.tools` rather than replacing them) and in
  `Task.check_tools` (`lib/crewai/src/crewai/task.py:542-546`).

Deductions (why this is not 8+):

- **No central tool catalog object.** The catalog is the union of five
  separate lists built across two different processes (kickoff path
  and task-execution path); the executor just receives whatever list
  `_prepare_tools` happened to assemble. There is no class that says
  "this is the tool catalog; here is its source and its filter policy".
- **Dynamic filtering is MCP-only.** Non-MCP tools cannot be hidden
  based on agent role, run context, or any other predicate; the only
  per-tool off-switch is `max_usage_count` and `result_as_answer`
  (`lib/crewai/src/crewai/tools/base_tool.py:180-187`). A search for
  `capability` and `permission` returns no runtime tool-filtering
  primitive (the only hits are A2A agent-card extensions and security
  fingerprints, which are not tool filters).
- **No retrieval / ranked discovery.** There is no analog to "top-k
  tool descriptions by similarity to the current step". The closest
  thing is `discover_skills`, which produces metadata injected into
  the prompt, not callable tools
  (`lib/crewai/src/crewai/skills/loader.py:36-143`,
  `lib/crewai/src/crewai/utilities/prompts.py:117-133`).
- **Task-level override shadows agent-level silently.**
  `prepare_task_execution` prefers `task.tools or agent.tools or []`
  (`lib/crewai/src/crewai/crews/utils.py:179`), then layers
  delegation/platform/MCP/memory on top. A user who passes a small
  `task.tools=[search]` does *not* lose memory tools, but they do
  keep them; conversely, an author who expects `task.tools` to be a
  *whitelist* gets more tools than they asked for. There is no
  documented `allowlist` semantics on `Task.tools`.
- **Routing has two implementations** (`ToolUsage._select_tool` and
  `AgentExecutor._execute_single_native_tool_call`). They agree on the
  `sanitize_tool_name` contract and both surface selection errors, but
  only the experimental `AgentExecutor` path has the structured
  `_available_functions` map that lets `_tool_name_mapping` be queried
  by `func_name` (`lib/crewai/src/crewai/experimental/agent_executor.py:1889-1898`).
  The legacy `ToolUsage._select_tool` falls back to a fuzzy match with
  ratio 0.85 (`lib/crewai/src/crewai/tools/tool_usage.py:759-774`),
  which is observable but not explainable beyond "name was close".

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent-level `tools` field | `tools: list[BaseTool] \| None` on `BaseAgent`, default factory `list` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:283-285` |
| Task-level `tools` field | `tools: list[BaseTool] \| None` on `Task`, "Tools the agent is limited to use for this task" | `lib/crewai/src/crewai/task.py:210-213` |
| Task shadow of agent tools | `Task.check_tools` copies `agent.tools` into `self.tools` if empty | `lib/crewai/src/crewai/task.py:541-546` |
| Task shadow ordering in crew | `tools_for_task = task.tools or agent_to_use.tools or []` then augmented by `crew._prepare_tools` | `lib/crewai/src/crewai/crews/utils.py:179-184` |
| Per-task tool augmentation | `Crew._prepare_tools` injects delegation, code, multimodal, platform, MCP, memory, file tools | `lib/crewai/src/crewai/crew.py:1616-1683` |
| Delegation injection gate | Only added when `agent.allow_delegation=True` and `len(agents)>1` | `lib/crewai/src/crewai/crew.py:1791-1801` |
| Manager-tool discipline | Manager agent tools are wiped with a hard exception if any are set | `lib/crewai/src/crewai/crew.py:1494-1505` |
| Delegation tool factory | `AgentTools(agents).tools()` returns `DelegateWorkTool` + `AskQuestionTool` | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:16-36` |
| Delegation execute handler | Sanitizes role string, runs `Task` on selected agent | `lib/crewai/src/crewai/tools/agent_tools/base_agent_tools.py:46-124` |
| Delegate tool schema | `task`, `context`, `coworker` | `lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:8-14` |
| Ask tool schema | `question`, `context`, `coworker` | `lib/crewai/src/crewai/tools/agent_tools/ask_question_tool.py:8-12` |
| Delegation tracking | `DELEGATION_TOOL_NAMES` frozenset + `track_delegation_if_needed` on tool execution | `lib/crewai/src/crewai/utilities/agent_utils.py:1166-1189` |
| Platform apps → tools | `Agent.get_platform_tools` → `CrewaiPlatformTools(apps=apps)` | `lib/crewai/src/crewai/agent/core.py:1177-1186` |
| Platform tool builder | Fetches action schemas from `{base}/actions?apps=` and creates `CrewAIPlatformActionTool` per action | `lib/crewai-tools/src/crewai_tools/tools/crewai_platform_tools/crewai_platform_tool_builder.py:34-98` |
| MCP string-refs accepted | URL form, AMP slug form, with `#tool` suffix | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:557-590`, `lib/crewai/src/crewai/mcp/tool_resolver.py:107-180` |
| MCP discovery | `MCPToolResolver.resolve` routes by URL/native/AMP, discovers via `list_tools()` | `lib/crewai/src/crewai/mcp/tool_resolver.py:68-89`, `lib/crewai/src/crewai/mcp/tool_resolver.py:333-402` |
| MCP discovery retries | `MCP_MAX_RETRIES=3` with exponential backoff | `lib/crewai/src/crewai/mcp/tool_resolver.py:39`, `lib/crewai/src/crewai/mcp/tool_resolver.py:530-554` |
| MCP schema cache | Module-level `_mcp_schema_cache`, TTL=300s | `lib/crewai/src/crewai/mcp/tool_resolver.py:41-42`, `lib/crewai/src/crewai/mcp/tool_resolver.py:494-519` |
| MCP discovery timeout | `MCP_DISCOVERY_TIMEOUT=15`, `MCP_CONNECTION_TIMEOUT=10` | `lib/crewai/src/crewai/mcp/tool_resolver.py:36-39` |
| MCP tool wrappers | `MCPToolWrapper` (HTTPS path, on-demand connect) vs `MCPNativeTool` (per-invocation client) | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:16-203`, `lib/crewai/src/crewai/tools/mcp_native_tool.py:16-132` |
| MCP tool name prefixing | Server name + tool name joined as `f"{server_name}_{tool_name}"` | `lib/crewai/src/crewai/tools/mcp_native_tool.py:43`, `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:34` |
| MCP static filter | `StaticToolFilter` with `allowed_tool_names`/`blocked_tool_names`, blocked precedence | `lib/crewai/src/crewai/mcp/filters.py:38-88` |
| MCP dynamic filter | `create_dynamic_tool_filter(filter_func)` with `(ToolFilterContext, tool) -> bool` | `lib/crewai/src/crewai/mcp/filters.py:130-163` |
| MCP filter wired into resolver | `if mcp_config.tool_filter: ... filtered_tools` over `tools_list` | `lib/crewai/src/crewai/mcp/tool_resolver.py:383-402` |
| Filter context | `ToolFilterContext(agent, server_name, run_context)` | `lib/crewai/src/crewai/mcp/filters.py:17-29` |
| Memory tool factory | `create_memory_tools(memory)` returns `[RecallMemoryTool]`; appends `RememberTool` only if not `read_only` | `lib/crewai/src/crewai/tools/memory_tools.py:104-130` |
| Memory tools wired into kickoff | `_prepare_kickoff` adds memory tools, deduping by `sanitize_tool_name(t.name)` | `lib/crewai/src/crewai/agent/core.py:1436-1445` |
| Memory tools wired into crew prep | `Crew._prepare_tools` adds memory tools when `agent.memory or self._memory` is non-None | `lib/crewai/src/crewai/crew.py:1652-1654`, `lib/crewai/src/crewai/crew.py:1761-1773` |
| File-injection tool | `ReadFileTool` with `set_files`, added for non-auto-injected content types | `lib/crewai/src/crewai/tools/agent_tools/read_file_tool.py:24-82`, `lib/crewai/src/crewai/crew.py:1675-1682` |
| Multimodal injection | `AddImageTool` returned by `Agent.get_multimodal_tools` | `lib/crewai/src/crewai/tools/agent_tools/add_image_tool.py`, `lib/crewai/src/crewai/agent/core.py:1206-1210` |
| Tool validation hook | `BaseAgent.validate_tools` rejects anything lacking `name`, `func`, `description` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:511-537` |
| Tool type registry | `_TOOL_TYPE_REGISTRY` populated by `BaseTool.__init_subclass__`; `_resolve_tool_dict` reconstructs concrete subclasses from a `tool_type` key | `lib/crewai/src/crewai/tools/base_tool.py:51-78`, `lib/crewai/src/crewai/tools/base_tool.py:109-112` |
| Conversion to executor type | `parse_tools` → list of `CrewStructuredTool`, with `current_usage_count` zeroed | `lib/crewai/src/crewai/utilities/agent_utils.py:96-120` |
| `render_text_description_and_args` | Joins `tool.description` strings; used by both ReAct prompts and native schema | `lib/crewai/src/crewai/utilities/agent_utils.py:135-151` |
| `get_tool_names` | Comma-joined sanitized names | `lib/crewai/src/crewai/utilities/agent_utils.py:123-132` |
| Native tool schema build | `convert_tools_to_openai_schema` builds OpenAI schema; auto-dedupes via `_2`, `_3` suffix | `lib/crewai/src/crewai/utilities/agent_utils.py:154-221` |
| Native tool routing state | `_openai_tools`, `_available_functions`, `_tool_name_mapping` on `AgentExecutor` | `lib/crewai/src/crewai/experimental/agent_executor.py:240-247` |
| Native routing check | `AgentExecutor._check_native_tool_support` returns `supports_function_calling() and bool(tools)` | `lib/crewai/src/crewai/experimental/agent_executor.py:236-238`, `lib/crewai/src/crewai/utilities/agent_utils.py:1267-1282` |
| Native-tool fallback | `_downgrade_to_text_tool_calling` clears `_openai_tools` and injects text-tool instructions when provider rejects native tool calls | `lib/crewai/src/crewai/experimental/agent_executor.py:249-264` |
| Native tool dispatch | `func_name in self._available_functions` gate; on miss, returns "Tool not found" and emits `ToolUsageErrorEvent` | `lib/crewai/src/crewai/experimental/agent_executor.py:1979-2018` |
| Tool parallelization guard | `_should_parallelize_native_tool_calls` forbids parallel execution when any tool has `result_as_answer=True` or `max_usage_count is not None` | `lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858` |
| Legacy dispatcher | `ToolUsage._select_tool` uses `SequenceMatcher` with 0.85 ratio; emits `ToolSelectionErrorEvent` on miss | `lib/crewai/src/crewai/tools/tool_usage.py:759-802` |
| Tool selection error event | `ToolSelectionErrorEvent` carries `tool_name`, `tool_class=tools_description` | `lib/crewai/src/crewai/events/types/tool_usage_events.py:86-90` |
| `max_usage_count` enforcement | Atomic `_claim_usage`; returns error string at limit | `lib/crewai/src/crewai/tools/base_tool.py:295-312` |
| `max_usage_count` declared | `BaseTool.max_usage_count: int \| None` validated as positive | `lib/crewai/src/crewai/tools/base_tool.py:184-187`, `lib/crewai/src/crewai/tools/base_tool.py:260-265` |
| `result_as_answer` declared | `BaseTool.result_as_answer: bool` | `lib/crewai/src/crewai/tools/base_tool.py:180-183` |
| Per-tool hooks (allow/block) | `before_tool_call` hook returning `False` blocks execution | `lib/crewai/src/crewai/experimental/agent_executor.py:1953-1980`, `lib/crewai/src/crewai/utilities/tool_utils.py:96-117` |
| Skill discovery | `discover_skills` walks a directory for `SKILL.md`; emits `SkillDiscovery*` events | `lib/crewai/src/crewai/skills/loader.py:36-109` |
| Progressive disclosure | `METADATA < INSTRUCTIONS < RESOURCES` defined as `Literal[1,2,3]` | `lib/crewai/src/crewai/skills/models.py:25-40` |
| Skill frontmatter `allowed-tools` | `SkillFrontmatter.allowed_tools: list[str] \| None` parsed from space-delimited YAML | `lib/crewai/src/crewai/skills/models.py:78-93` |
| Skills injected into prompt | `_build_skill_block` wraps in `<skill>` XML and emits as agent-scoped system-prompt prefix | `lib/crewai/src/crewai/utilities/prompts.py:117-133` |
| Skill registry resolution | `is_registry_ref`/`parse_registry_ref`/`resolve_registry_ref` handle `@org/name` with local → cache → download order | `lib/crewai/src/crewai/experimental/skills/registry.py:31-132` |
| `from_repository` loader | `load_agent_from_repository` imports `tools` as `{module, name, init_params}` and reconstructs | `lib/crewai/src/crewai/utilities/agent_utils.py:1104-1163` |
| Cache keys for tools | `CacheHandler.read(tool=..., input=...)` keyed by sanitized tool name + JSON-serialized input | `lib/crewai/src/crewai/agents/cache/cache_handler.py`, `lib/crewai/src/crewai/experimental/agent_executor.py:1928-1935` |
| `cache_function` per-tool | Default is "always cache"; overridable via `cache_function` | `lib/crewai/src/crewai/tools/base_tool.py:81-83`, `lib/crewai/src/crewai/tools/base_tool.py:176-179` |
| `ToolsHandler` lifecycle | Created per-agent; `last_used_tool` set after each call; cache write skipped for the cache tool itself | `lib/crewai/src/crewai/agents/tools_handler.py:15-52` |
| Suggested tool per plan step | `TodoItem.tool_to_use: str \| None`; checked after step via `_validate_expected_tool_usage` | `lib/crewai/src/crewai/utilities/planning_types.py:27-42`, `lib/crewai/src/crewai/agents/step_executor.py:504-526` |
| Plan-Act step tool routing | Executor injects "Suggested tool: {tool_to_use}" into the step prompt; fails step if not called | `lib/crewai/src/crewai/agents/step_executor.py:293-301`, `lib/crewai/src/crewai/agents/step_executor.py:504-526` |
| `LiteAgent` tool parsing | `_parsed_tools = parse_tools(self.tools)` on init; adds memory tools when `_memory` set | `lib/crewai/src/crewai/lite_agent.py:317-321`, `lib/crewai/src/crewai/lite_agent.py:497-508` |
| `LiteAgent` system prompt | `lite_agent_system_prompt_with_tools` includes `tools` and `tool_names` | `lib/crewai/src/crewai/lite_agent.py:790-826` |

## Answers to Dimension Questions

1. **Does every agent see every tool?**
   No. Each `Agent` declares its own `tools: list[BaseTool]` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:283-285`). At task time,
   `Crew._prepare_tools` augments that list conditionally: delegation tools
   only when `allow_delegation=True` and other agents exist
   (`lib/crewai/src/crewai/crew.py:1791-1801`); platform tools only when
   `agent.apps` is set (`lib/crewai/src/crewai/crew.py:1722-1732`);
   MCP tools only when `agent.mcps` is set
   (`lib/crewai/src/crewai/crew.py:1734-1743`); memory tools only when
   `agent.memory or self._memory` is non-None
   (`lib/crewai/src/crewai/crew.py:1652-1654`); read-file tool only when
   there are input files not auto-injected by the LLM provider
   (`lib/crewai/src/crewai/crew.py:1656-1682`). A "manager" agent is
   forced to drop any explicit `tools` (`lib/crewai/src/crewai/crew.py:1494-1505`).
   That said, **once an agent has a tool, it stays in that agent's
   catalog** — there is no dynamic per-task or per-turn hide, except
   `max_usage_count` (`lib/crewai/src/crewai/tools/base_tool.py:184-187`)
   and `result_as_answer` short-circuiting (`lib/crewai/src/crewai/tools/base_tool.py:180-183`).

2. **Are tools filtered by task?**
   Indirectly. The **list** that goes to the executor is filtered by
   task: `prepare_task_execution` resolves `task.tools or agent.tools`
   (`lib/crewai/src/crewai/crews/utils.py:179`) and lets the crew layer
   delegation/platform/MCP/memory on top. But within that list, the
   LLM sees every tool description on every step — there is no
   per-task tool selection or capability matching; the same `tools`
   array and the same `tools_description` string is fed into the LLM
   on every ReAct iteration
   (`lib/crewai/src/crewai/experimental/agent_executor.py:188-194`,
   `lib/crewai/src/crewai/utilities/agent_utils.py:135-151`). When
   planning is enabled, the planner may mark a step with
   `TodoItem.tool_to_use`, and `StepExecutor._validate_expected_tool_usage`
   requires that tool to have been called
   (`lib/crewai/src/crewai/agents/step_executor.py:504-526`); this is
   the closest thing to "the model is steered toward a specific tool
   for this step", but it is enforced *after* the model emits its tool
   call, not by hiding alternatives.

3. **Are tools filtered by permission?**
   No first-class permission filter. The system has `security_config`
   (`lib/crewai/src/crewai/security/security_config.py`,
   `lib/crewai/src/crewai/security/fingerprint.py`), but `security_config`
   is a fingerprint/identity carrier, not a tool gate. The only
   permission-like controls are: (a) `Memory._read_only` removes the
   write-memory tool (`lib/crewai/src/crewai/tools/memory_tools.py:123-129`);
   (b) `StaticToolFilter.blocked_tool_names` on an MCP server blocks
   specific tools from that server
   (`lib/crewai/src/crewai/mcp/filters.py:38-88`); (c) `before_tool_call`
   hooks can return `False` to block an individual invocation
   (`lib/crewai/src/crewai/experimental/agent_executor.py:1953-1980`,
   `lib/crewai/src/crewai/utilities/tool_utils.py:96-117`). There is no
   "user X may not call tool Y" or "agent role Z may not call tool W"
   primitive.

4. **Can tools be hidden from the model?**
   Partially. Tools can be hidden *at the catalog level* by simply not
   adding them to `agent.tools` or `task.tools`. Tools can be *blocked
   per call* via `before_tool_call` hooks. But once a tool is in the
   catalog, its description is rendered into every prompt via
   `render_text_description_and_args`
   (`lib/crewai/src/crewai/utilities/agent_utils.py:135-151`) and into
   every native schema via `convert_tools_to_openai_schema`
   (`lib/crewai/src/crewai/utilities/agent_utils.py:154-221`). There is
   no `hide_from_model(...)` toggle on `BaseTool`. There is also no
   per-turn hiding — `state.use_native_tools`, `_openai_tools`, and
   `_available_functions` are stable for the lifetime of an executor
   (`lib/crewai/src/crewai/experimental/agent_executor.py:236-264`).

5. **Is tool availability explainable?**
   Yes at the *selection* boundary (when the model picks a tool), but
   no at the *visibility* boundary (when a tool gets onto the catalog).
   - On a selection miss, both paths emit `ToolSelectionErrorEvent`
     carrying `tool_name` and the full `tools_description`
     (`lib/crewai/src/crewai/tools/tool_usage.py:786-802`,
     `lib/crewai/src/crewai/events/types/tool_usage_events.py:86-90`).
   - Tool usage events (`ToolUsageStartedEvent`, `ToolUsageFinishedEvent`,
     `ToolUsageErrorEvent`) carry `agent_key`, `agent_role`,
     `tool_name`, `tool_class`, plus `plan_step_number`/`plan_step_description`
     (`lib/crewai/src/crewai/events/types/tool_usage_events.py:10-90`),
     and the `AgentExecutor._should_parallelize_native_tool_calls` check
     surfaces `result_as_answer`/`max_usage_count` reasons
     (`lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858`).
   - There is **no** built-in audit for "this tool is on agent X
     because it came from MCP server Y, filtered through `tool_filter`".
     `CrewaiPlatformTools` only logs failures
     (`lib/crewai-tools/src/crewai_tools/tools/crewai_platform_tools/crewai_platform_tool_builder.py:55-57`),
     and `MCPToolResolver` logs warnings on discovery failures
     (`lib/crewai/src/crewai/mcp/tool_resolver.py:404-409`) but does
     not emit a "these tools were admitted" event.

## Architectural Decisions

- **Tools are explicit instances, not declarations.** Every tool is a
  concrete Python class instance (`BaseTool`, `Tool`, `CrewStructuredTool`)
  with a `name`, `description`, optional `args_schema`, optional
  `result_schema`, optional `result_as_answer`, optional `max_usage_count`,
  and optional `cache_function` (`lib/crewai/src/crewai/tools/base_tool.py:139-192`).
  This is a Pydantic-first model; tools are reconstructable across
  checkpoints via `_TOOL_TYPE_REGISTRY` and `tool_type` discriminant
  (`lib/crewai/src/crewai/tools/base_tool.py:51-78`, `lib/crewai/src/crewai/tools/base_tool.py:109-112`,
  `lib/crewai/src/crewai/tools/base_tool.py:194-198`).

- **The catalog is the union, not a registry.** CrewAI does not have a
  `ToolCatalog` singleton or class; the "catalog" for a given
  execution is the `list[CrewStructuredTool]` assembled inside the
  executor (`lib/crewai/src/crewai/experimental/agent_executor.py:188-194`).
  Six sources feed it (`agent.tools`, `task.tools`, platform,
  delegation, MCP, memory, read-file), and the merge order is
  deterministic but buried in
  `Crew._prepare_tools`/`Agent._prepare_kickoff`
  (`lib/crewai/src/crewai/crew.py:1616-1683`,
  `lib/crewai/src/crewai/agent/core.py:1411-1456`).

- **Two parallel routing implementations, one stable name contract.**
  Both the legacy ReAct path (`ToolUsage._select_tool`,
  `lib/crewai/src/crewai/tools/tool_usage.py:759-802`) and the modern
  native path (`AgentExecutor._execute_single_native_tool_call`,
  `lib/crewai/src/crewai/experimental/agent_executor.py:1860-2024`)
  match tools on `sanitize_tool_name(tool.name)`. The legacy path
  adds a fuzzy `SequenceMatcher` fallback with threshold 0.85; the
  modern path is exact-match via a pre-built dict
  (`_tool_name_mapping`). Naming convention is centralized in
  `sanitize_tool_name` (`lib/crewai/src/crewai/utilities/string_utils.py`).

- **Routing failures are events, not exceptions, on the modern path.**
  `AgentExecutor._execute_single_native_tool_call` catches the
  "no matching callable" case and returns a `NativeToolCallResult` with
  `"Tool not found"` plus a `ToolUsageErrorEvent`, never throwing
  (`lib/crewai/src/crewai/experimental/agent_executor.py:1979-2018`).
  The legacy path raises and emits `ToolSelectionErrorEvent`
  (`lib/crewai/src/crewai/tools/tool_usage.py:793-802`). This split
  is observable behavior the model can recover from on the modern
  path but not on the legacy path.

- **Filters are server-scoped, not tool-scoped.** MCP is the only
  source with first-class filter plumbing
  (`StaticToolFilter` / `create_dynamic_tool_filter` /
  `ToolFilterContext`); filters live on `MCPServerConfig.tool_filter`
  (`lib/crewai/src/crewai/mcp/config.py:43-46`, `:80-83`, `:113-116`)
  and are applied during `_resolve_native`
  (`lib/crewai/src/crewai/mcp/tool_resolver.py:383-402`). For all
  other tool sources, "filtering" means "do not add this tool".

- **Skills are prompt content, not tools.** `discover_skills` +
  `activate_skill` + `format_skill_context` produce XML blocks
  (`<skill name="...">...</skill>`) that `_build_skill_block`
  injects into the system prompt
  (`lib/crewai/src/crewai/utilities/prompts.py:117-133`). They are
  agent-scoped and live in the cache-stable system prefix
  (`lib/crewai/src/crewai/utilities/prompts.py:120-122`). They never
  become `BaseTool` instances and never appear in `parse_tools`
  output. The relationship between a skill's
  `frontmatter.allowed_tools` (`lib/crewai/src/crewai/skills/models.py:78-93`)
  and the agent's runtime catalog is **not** enforced anywhere in the
  codebase — `allowed_tools` is parsed but unused outside the schema.

- **Read-only memory is the only capability-style gate.** Memory is
  the only category whose visibility is conditional on a runtime
  flag: `_read_only` removes the write tool
  (`lib/crewai/src/crewai/tools/memory_tools.py:123-129`). This is
  the only place where a tool is conditionally absent based on a
  capability flag rather than an explicit user list.

- **Tool fingerprints and source tracing.** `ToolUsageEvent` carries
  `source_fingerprint`/`source_type` populated from the agent's
  `security_config.fingerprint`
  (`lib/crewai/src/crewai/events/types/tool_usage_events.py:46-53`,
  `lib/crewai/src/crewai/security/fingerprint.py:55-65`). Every tool
  call is therefore tied to the agent that issued it, which gives a
  partial answer to "which agent called this tool" but not to "which
  catalog did this tool come from".

## Notable Patterns

- **Sanitized-name contract.** Every tool lookup, dispatch, and event
  label flows through `sanitize_tool_name` (`lib/crewai/src/crewai/utilities/string_utils.py`).
  Tool names in the model-facing schema and in the executor's
  dispatch table are guaranteed to share a canonical form, so the
  dict-based `_available_functions` and the OpenAI schema are
  index-compatible.

- **Static + dynamic filter on a single source.** `StaticToolFilter`
  and `create_dynamic_tool_filter` share `ToolFilterContext`
  (`lib/crewai/src/crewai/mcp/filters.py:17-29`), and the resolver
  picks the call signature at runtime via `try/except (TypeError, AttributeError)`
  (`lib/crewai/src/crewai/mcp/tool_resolver.py:386-401`). This is the
  pattern to copy for any future non-MCP filter.

- **Per-tool usage limit is atomic.** `_claim_usage` uses
  `threading.Lock` so `max_usage_count` is enforced safely under
  parallel tool dispatch
  (`lib/crewai/src/crewai/tools/base_tool.py:192`,
  `lib/crewai/src/crewai/tools/base_tool.py:295-312`). This is the
  only per-tool runtime enforcement in the system that is not just a
  check after the fact.

- **`result_as_answer` short-circuits parallel execution.**
  `_should_parallelize_native_tool_calls` refuses to parallelize a
  batch when any tool has `result_as_answer=True` or a non-`None`
  `max_usage_count`
  (`lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858`,
  same logic mirrored in
  `lib/crewai/src/crewai/agents/crew_agent_executor.py:696-724`).
  This keeps tool ordering observable when the tool's effect on the
  conversation is non-additive.

- **Plan-and-Act pins tools to steps.** `TodoItem.tool_to_use`
  (`lib/crewai/src/crewai/utilities/planning_types.py:27-42`) plus
  `StepExecutor._validate_expected_tool_usage`
  (`lib/crewai/src/crewai/agents/step_executor.py:504-526`) gives the
  planner a way to *constrain* tool choice per step. It is advisory
  injection, not capability-based hiding, but it is the only place
  CrewAI ties a step to a specific tool.

- **Skill blocks cached as system prefix.** `_build_skill_block`
  emits skills before user/task content
  (`lib/crewai/src/crewai/utilities/prompts.py:117-133`), and the
  crew-agent executor wraps the system message in
  `mark_cache_breakpoint` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:189-199`),
  so skill metadata is a prompt-cache anchor across ReAct iterations.

- **Skills are versioned and registry-fetched.** Registry refs
  (`@org/name`) resolve through a 3-tier order: project-local
  `./skills/{name}/`, global `~/.crewai/skills/{org}/{name}/`, then
  network download gated by `CI`/`CREWAI_NONINTERACTIVE`
  (`lib/crewai/src/crewai/experimental/skills/registry.py:71-132`,
  `lib/crewai/src/crewai/experimental/skills/cache.py`).

- **Deterministic tool names for agents loaded from a repository.**
  `load_agent_from_repository` reconstructs tools from
  `{module, name, init_params}` JSON
  (`lib/crewai/src/crewai/utilities/agent_utils.py:1141-1162`); the
  `_TOOL_TYPE_REGISTRY` is the lookup table
  (`lib/crewai/src/crewai/tools/base_tool.py:51-78`).

## Tradeoffs

- **Per-agent static catalog vs. dynamic capability matching.** The
  choice to keep the catalog static for the agent's lifetime
  simplifies dispatch (`_available_functions` is built once) and
  reasoning (no surprise tool removal mid-loop), at the cost of
  prompt bloat: every tool description is re-rendered into every
  prompt for every agent that has any tool
  (`lib/crewai/src/crewai/utilities/agent_utils.py:135-151`). There
  is no per-step tool subsetting for non-plan agents.

- **Task shadow vs. task whitelist.** `Task.tools` is documented as
  "Tools the agent is limited to use for this task"
  (`lib/crewai/src/crewai/task.py:212-213`), which reads like a
  whitelist, but the code prefers it over `agent.tools` and still
  layers delegation/MCP/platform/memory on top
  (`lib/crewai/src/crewai/crews/utils.py:179`,
  `lib/crewai/src/crewai/crew.py:1616-1683`). The mental model is
  "task overrides the user's tool list, system still augments".

- **Filter is per-MCP-server, not per-tool-source.** `tool_filter`
  lives on `MCPServerConfig` (`lib/crewai/src/crewai/mcp/config.py:43-46`,
  `:80-83`, `:113-116`). This pushes the policy decision to the server
  boundary, which is good for "notion has too many actions, narrow it"
  but bad for "agents of role X may not call any tool that touches
  email". The pattern is reusable (callable, `ToolFilterContext`), so
  the cost is "no one has built it for non-MCP tools yet".

- **Fuzzy matching in the legacy path.** `SequenceMatcher` with
  threshold 0.85 (`lib/crewai/src/crewai/tools/tool_usage.py:759-774`)
  is a pragmatic fix for sloppy LLM output, but it makes
  selection-error observability harder: a hit at 0.87 looks like a
  clean dispatch but was actually a typo-recovery. The modern path
  has dropped this fallback in favor of exact-match + structured error
  (`lib/crewai/src/crewai/experimental/agent_executor.py:1979-2018`).

- **Skill `allowed_tools` is parsed but unused outside the schema.**
  `SkillFrontmatter.allowed_tools` is documented as "Pre-approved tool
  names the skill may use"
  (`lib/crewai/src/crewai/skills/models.py:78-93`), but no enforcement
  code consumes it. This is half a feature: the schema exists, the
  runtime does not honor it. The trade-off is presumably to avoid
  blocking on skills loading before the agent catalog is finalized,
  but it leaves a docs-vs-runtime gap.

- **No native tool retrieval.** The model sees every tool description
  every time. With large catalogs (e.g., 30 platform apps × 10 actions
  = 300 entries), the system-prompt cost grows linearly. There is no
  retrieval-augmented tool selection anywhere — skills are the only
  thing indexed, and they are metadata-only.

- **MCP filtering happens once at discovery.** `_resolve_native`
  applies `tool_filter` against the discovered `tools_list` and stores
  the filtered set as the agent's MCP tools
  (`lib/crewai/src/crewai/mcp/tool_resolver.py:383-402`). The schema
  cache (TTL 300s) reuses the same filtered list
  (`lib/crewai/src/crewai/mcp/tool_resolver.py:494-519`). This is
  correct, but it means filters do not re-evaluate mid-run; if the
  agent's role changes after discovery, the filter result does not.

## Failure Modes / Edge Cases

- **Duplicate tool names.** `convert_tools_to_openai_schema` auto-dedupes
  by appending `_2`, `_3`, ...
  (`lib/crewai/src/crewai/utilities/agent_utils.py:200-206`), and
  `Agent._prepare_kickoff` dedupes by sanitized name when adding
  memory tools
  (`lib/crewai/src/crewai/agent/core.py:1440-1445`). However, the
  non-memory merge path
  (`lib/crewai/src/crewai/crew.py:1616-1683`,
  `lib/crewai/src/crewai/crew.py:1690-1709`) does not dedupe; if a
  user adds the same tool to `agent.tools` and `task.tools` and then
  via `apps`, the executor's `_tool_name_mapping` will see only the
  first occurrence and the rest will fail to dispatch with
  `ToolUsageErrorEvent`.

- **Tool selection miss on the modern path returns "Tool not found"
  silently in the message.** `AgentExecutor._execute_single_native_tool_call`
  emits `ToolUsageErrorEvent` and writes `"Error executing tool: …"`
  to the tool message (`lib/crewai/src/crewai/experimental/agent_executor.py:2001-2018`),
  which is then reflected in the model's conversation as a "tool"
  role message. The model can recover, but the user has to read the
  event bus to know the tool name was unrecognized. `ToolSelectionErrorEvent`
  exists but only fires from the legacy path.

- **MCP native tools fail under cancel-scope errors.** `MCPNativeTool`
  creates a fresh `MCPClient` per invocation to avoid anyio cancel-scope
  issues (`lib/crewai/src/crewai/tools/mcp_native_tool.py:73-94`),
  but `MCPToolResolver._resolve_native` itself mixes
  `asyncio.get_running_loop()` with `ThreadPoolExecutor` and `asyncio.run`
  (`lib/crewai/src/crewai/mcp/tool_resolver.py:355-381`); if the calling
  context already holds an event loop, this can fail with a
  `RuntimeError` that is caught and re-raised as `ConnectionError`
  with a generic message — the actual cause is lost.

- **MCP schema cache is a global module-level dict with no
  invalidation hook.** `_mcp_schema_cache` (`lib/crewai/src/crewai/mcp/tool_resolver.py:41`)
  is shared across all agents and crews in the process. A change to
  an MCP server (new tools, removed tools) is invisible until the 300 s
  TTL expires. There is no `cache_tools_list=False` override that
  bypasses the cache because the cache is separate from
  `MCPServerConfig.cache_tools_list`
  (`lib/crewai/src/crewai/mcp/config.py:47-50`,
  `:84-87`, `:117-120`) — the config controls the *server-side*
  list cache, while `_mcp_schema_cache` is the *client-side* schema
  cache. Conflation here is easy to miss.

- **Manager agent must have empty tools.** `_create_manager_agent`
  raises `Exception("Manager agent should not have tools")`
  (`lib/crewai/src/crewai/crew.py:1494-1505`), but the check happens
  before tools are assigned, and the manager's tools are then
  unconditionally replaced with `AgentTools(agents=self.agents).tools()`
  (`lib/crewai/src/crewai/crew.py:1513`). If a user passes a custom
  `manager_agent` with delegation + MCP tools set, the code wipes
  the user's tools *and* raises. The recovery is to pass an
  `manager_agent` with `tools=None` (the default) and let `_create_manager_agent`
  build the delegation set.

- **Parallel native tool dispatch refuses `max_usage_count` tools.**
  `_should_parallelize_native_tool_calls` (`lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858`)
  falls back to sequential execution, which means a single LLM turn
  with `max_usage_count` tools can become `N` sequential rounds. With
  a small `max_iter` (default 25,
  `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:286-288`)
  this can starve the agent of iterations.

- **Skill `allowed_tools` parsed, not enforced.** As above. If a
  skill says `allowed-tools: search`, the model still sees all
  `agent.tools` regardless of which skill is "active". This is
  silent; no event or warning fires
  (`lib/crewai/src/crewai/skills/models.py:78-93`,
  `lib/crewai/src/crewai/skills/loader.py:36-143`).

- **`cache_tools_list` default is `False` for all MCP transports.**
  This is correct for repeated invocations against a stable server,
  but it forces a new `list_tools()` call on the first call of every
  agent, which combined with the 300 s module cache is at best
  slightly redundant and at worst a stale view for 300 s after a
  server change (`lib/crewai/src/crewai/mcp/config.py:47-50`,
  `:84-87`, `:117-120`).

- **Read-file tool is only injected when files are present.**
  `Crew._prepare_tools` only calls `_add_file_tools` if there are
  *non-auto-injected* files for the LLM
  (`lib/crewai/src/crewai/crew.py:1656-1682`). For multimodal
  providers that auto-inject everything, the read-file tool never
  appears, even if the user expected a fallback path.

## Future Considerations

- **Lift the catalog into a first-class object.** A `ToolCatalog`
  with explicit `add(category, tools, filter=...)`, `resolve(agent,
  task)`, `audit()` would replace the conditional ladder in
  `Crew._prepare_tools` (`lib/crewai/src/crewai/crew.py:1616-1683`).
  The `_openai_tools`/`_available_functions`/`_tool_name_mapping`
  triple on `AgentExecutor`
  (`lib/crewai/src/crewai/experimental/agent_executor.py:240-247`)
  is already the right shape; it just needs an explicit constructor
  surface.

- **Generalize `tool_filter` beyond MCP.** The `StaticToolFilter` /
  `create_dynamic_tool_filter` / `ToolFilterContext` triple
  (`lib/crewai/src/crewai/mcp/filters.py:17-163`) is generic enough
  to apply to `agent.tools`, `task.tools`, and platform apps. Adding
  a `tool_filter` parameter to `Agent` (or a separate `ToolPolicy`
  pydantic model on `Agent`) would close the
  "agents of role X cannot call tool Y" gap.

- **Retrieve tools by capability for long catalogs.** With 30+
  platform apps × ~10 actions, the system prompt already bloats. A
  similarity-based tool retrieval step (analogous to the agent's
  `knowledge_search_query`
  (`lib/crewai/src/crewai/agent/core.py:1299-1302`,
  `lib/crewai/src/crewai/agent/core.py:1346-1399`)) would keep the
  catalog static while shrinking the prompt.

- **Honor `SkillFrontmatter.allowed_tools`.** Either enforce it
  (hide unlisted tools while the skill is `INSTRUCTIONS`-level) or
  remove the field. The current state is worse than either — it
  promises a contract it does not keep
  (`lib/crewai/src/crewai/skills/models.py:78-93`).

- **Unify the two routing paths.** `ToolUsage._select_tool`
  (`lib/crewai/src/crewai/tools/tool_usage.py:759-802`) and
  `AgentExecutor._execute_single_native_tool_call`
  (`lib/crewai/src/crewai/experimental/agent_executor.py:1860-2024`)
  share the same intent (match by `sanitize_tool_name`, emit an
  event on miss) but diverge on fuzzy match vs. structured result.
  Since `CrewAgentExecutor` is deprecated
  (`lib/crewai/src/crewai/agents/crew_agent_executor.py:143-151`),
  the legacy path will eventually retire, but until it does, both
  need to be maintained.

- **Surface "tool admitted" events.** A `ToolAdmittedEvent` (or
  `ToolCatalogSnapshotEvent`) with `{agent, category, source, filter,
  tool_names}` would give operators a single answer to "why does this
  agent have these tools". Today, the closest evidence is the agent's
  own `tools` field plus the kickoff `LiteAgentExecutionStartedEvent`
  that carries `tools` (`lib/crewai/src/crewai/agent/core.py:1623-1628`,
  `lib/crewai/src/crewai/events/types/agent_events.py`).

- **Cache invalidation hooks for MCP.** A simple "bust cache on
  connection error" or "version-aware cache key" would prevent the
  300 s window of stale schema. Today there is no signal
  (`lib/crewai/src/crewai/mcp/tool_resolver.py:41-42`,
  `:494-519`).

- **Refine Task.tools semantics.** Pick whitelist or shadow,
  document it, and make `_prepare_tools` honor it. Either remove the
  automatic delegation/MCP/memory augmentation when `task.tools` is
  set, or rename it to make the augmentation explicit
  (`lib/crewai/src/crewai/crews/utils.py:179`,
  `lib/crewai/src/crewai/crew.py:1616-1683`).

## Questions / Gaps

- **What is the canonical "tool list" for an agent?** CrewAI has at
  least three candidates: `agent.tools`
  (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:283-285`),
  the kickoff-time `raw_tools`
  (`lib/crewai/src/crewai/agent/core.py:1434`), and the executor's
  `_openai_tools`/`_available_functions`/`_tool_name_mapping`
  (`lib/crewai/src/crewai/experimental/agent_executor.py:240-247`).
  They are populated at different times by different code paths and
  are not always identical.

- **Why is `_should_parallelize_native_tool_calls` a duplicate of
  the same logic in `CrewAgentExecutor._handle_native_tool_calls`?**
  Both paths implement the same `result_as_answer`/`max_usage_count`
  parallel-skip check
  (`lib/crewai/src/crewai/experimental/agent_executor.py:1827-1858`,
  `lib/crewai/src/crewai/agents/crew_agent_executor.py:696-724`).
  The duplication is consistent but is a maintenance hazard now that
  `CrewAgentExecutor` is deprecated.

- **How should a custom `BaseAgent` subclass report its catalog?**
  `get_delegation_tools`, `get_platform_tools`, `get_mcp_tools` are
  declared abstract on `BaseAgent`
  (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:680-689`),
  but the crew only checks `hasattr` for these methods
  (`lib/crewai/src/crewai/crew.py:1717-1743`); a subclass that
  returns `None` or raises will silently miss tool categories.

- **What happens to MCP tools if the agent is copied?** `Agent.copy`
  excludes `_mcp_resolver` and reuses `self.tools`
  (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:691-740`),
  so the copy has the resolved MCP tools but no resolver to clean
  them up. Cleanup happens in `Agent._cleanup_mcp_clients`, which is
  called from `_finalize_task_execution`
  (`lib/crewai/src/crewai/agent/core.py:1199-1203`,
  `:681`); copied agents therefore have tool entries without
  ownership of their client lifecycles.

- **Does `ToolSelectionErrorEvent` fire on the modern path?** No.
  The native path returns a `NativeToolCallResult` with a
  `"Tool not found"` string and emits `ToolUsageErrorEvent`
  (`lib/crewai/src/crewai/experimental/agent_executor.py:2001-2018`).
  `ToolSelectionErrorEvent` is only fired by `ToolUsage._select_tool`
  (`lib/crewai/src/crewai/tools/tool_usage.py:786-802`). The event
  type still exists in `events/types/tool_usage_events.py:86-90`, but
  it is a legacy signal. Any new tooling watching this event will be
  blind to modern-path misses.

---

Generated by `04.03-tool-catalog-discovery-and-routing` against `crewai`.