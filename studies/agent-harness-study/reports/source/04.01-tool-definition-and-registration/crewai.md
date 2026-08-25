# Source Analysis: crewai

## Tool Definition and Registration

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python 3.11+ / Pydantic v2 / asyncio |
| Analyzed | 2026-07-19 |

## Summary

CrewAI provides four layered tool declaration/registration surfaces that compose into the agent runtime:

1. A canonical class-based registry rooted at `BaseTool` (a Pydantic-v2 `BaseModel` + `ABC`) with subclasses auto-registered in a `__init_subclass__`-populated dictionary (`crewai_tools_base_tool.py:103-112`).
2. A `@tool` decorator factory that wraps any typed function as a `Tool` subclass instance (`crewai_tools_base_tool.py:676-761`).
3. A `CrewStructuredTool` shim that exposes the same surface in a langchain-style `func(args)` API used by tool-calling LLMs (`crewai_tools_structured_tool.py:115-388`).
4. A dynamic MCP layer (`MCPToolResolver`) that turns remote MCP server tool lists into `MCPNativeTool` / `MCPToolWrapper` instances, and `MCPServerAdapter` in the companion `crewai-tools` package (`crewai_mcp_tool_resolver.py:45-640`, `crewai_tools_adapters_mcp_adapter.py:97-235`).

Tools enter the runtime by being assigned to `Agent(tools=[...])`, `Task(tools=[...])`, or via the special `Agent(mcps=[...])` and `Agent(apps=[...])` fields. Registration is therefore explicit at construction time; discovery is dynamic only for MCP/AMP-backed tools, which are resolved lazily on `_prepare_kickoff` / `_prepare_tools`. Names are not stored verbatim: every name passes through `sanitize_tool_name` (`crewai_utilities_string_utils.py:26-54`) which NFKD-normalizes, camelCase-splits, lowercases, collapses non-`[a-z0-9]` to `_`, and enforces a 64-char OpenAI/Bedrock limit. Naming is not strictly stable across the system: agents see the raw `name`, the prompt sees `sanitize_tool_name(name)`, MCP tools are server-prefixed (`MCPNativeTool.__init__:43`), and `_select_tool` falls back to a `SequenceMatcher` ratio > 0.85 fuzzy match (`crewai_tools_tool_usage.py:759-802`).

Multiple tool sets per agent are supported cleanly: every agent carries its own `tools: list[BaseTool]` field validated by `validate_tools` (`crewai_agents_agent_builder_base_agent.py:511-537`), and `_prepare_tools` in the crew extends per-task tools with delegation, platform, MCP, multimodal, memory, code-execution and file tools in a deterministic merge order (`crewai_crew.py:1616-1683`, `_merge_tools` at `crewai_crew.py:1690-1709`).

## Rating

**8 / 10 — Clear model with explicit interfaces, tests, and operational safeguards; some surface bloat and a few fragile edges (fuzzy tool selection, duplicated `BaseTool` vs `CrewStructuredTool` runtime paths, MCP discoverability).

Rationale:
- Strong base class with auto-registration (`__init_subclass__`), stable serializer (`tool_type` computed field), `to_structured_tool` adapter, schema serialization round-trip via `_serialize_schema` / `_deserialize_schema`.
- Three official declaration styles (class, decorator, factory function) are first-class and tested (`tests/tools/test_base_tool.py:14-199`, `tests/tools/test_structured_tool.py:79-146`).
- Explicit registration via Pydantic `tools` field with a `field_validator` that either accepts `BaseTool`, converts langchain-style objects via `Tool.from_langchain`, or raises a typed error (`crewai_agents_agent_builder_base_agent.py:511-537`).
- Dynamic discovery is supported via MCP (`MCPToolResolver.resolve`) with explicit filters (`ToolFilter`, `StaticToolFilter`) and per-server caching (`crewai_mcp_tool_resolver.py:41-42`).
- Multi-agent isolation is real: per-agent list, per-task override, and a deterministic merge order with deduplication by sanitized name (`crewai_crew.py:1690-1709`).
- Operational safeguards: usage lock and `max_usage_count` (`crewai_tools_base_tool.py:192, 295-312`), `cache_function` hook (`crewai_tools_base_tool.py:176-179`), `result_as_answer`, retry/backoff on MCP, schema caching TTL, exponential backoff retry in `MCPClient._retry_operation` (`crewai_mcp_client.py:663-714`).
- Tests: dedicated coverage in `tests/tools/`, `tests/mcp/`, `tests/test_crew.py` covering tool override behavior (`tests/test_crew.py:570-734, 1691-1750, 3909-3982`).

Gaps that hold it below 9:
- Two parallel tool runtimes (`BaseTool._run` vs `CrewStructuredTool.invoke/ainvoke`) with their own usage accounting; the conversion through `parse_tools`/`to_structured_tool` is mandatory before execution (`crewai_utilities_agent_utils.py:96-120`).
- Tool name selection falls back to fuzzy matching at ratio 0.85, which is deterministic but masks real bugs in caller naming (`crewai_tools_tool_usage.py:759-774`).
- `Agent.mcps` accepts mixed `str | MCPServerConfig`, with three different parser branches (native, AMP slug, HTTPS URL) each making distinct HTTP calls (`crewai_mcp_tool_resolver.py:68-88`).
- `BaseTool.from_langchain` is a lossy duck-type bridge and is still the default admission policy for any non-`BaseTool` tool (`crewai_agents_agent_builder_base_agent.py:529-530`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Base class | `BaseTool(BaseModel, ABC)` with `name`, `description`, `args_schema`, `result_schema`, `cache_function`, `max_usage_count` | `lib/crewai/src/crewai/tools/base_tool.py:103-198` |
| Auto-registration of subclasses into a dotted-path registry | `__init_subclass__` writes `f"{cls.__module__}.{cls.__qualname__}"` into `_TOOL_TYPE_REGISTRY` | `lib/crewai/src/crewai/tools/base_tool.py:51,109-112` |
| Tool type discriminator | `computed_field tool_type` plus Pydantic serializer for JSON round-trip | `lib/crewai/src/crewai/tools/base_tool.py:194-198, 132-137` |
| Function-wrapping decorator factory | `@tool` decorator with three call shapes; enforces `__doc__` and `__annotations__` | `lib/crewai/src/crewai/tools/base_tool.py:652-761` |
| Subclass for callable wrappers | `Tool(BaseTool, Generic[P, R])` exposes `func` and overrides `_run`/`_arun` | `lib/crewai/src/crewai/tools/base_tool.py:496-638` |
| Default cache function | Returns True (cache everything) | `lib/crewai/src/crewai/tools/base_tool.py:81-83, 176-179` |
| Usage limit lock | `threading.Lock` + atomic claim via `_claim_usage`; raises on overflow in structured variant | `lib/crewai/src/crewai/tools/base_tool.py:192, 295-312`; `lib/crewai/src/crewai/tools/structured_tool.py:314-317, 366-371` |
| Schema (de)serialization helpers | `_serialize_schema` / `_deserialize_schema` use `model_json_schema` + `create_model_from_schema` | `lib/crewai/src/crewai/tools/structured_tool.py:28-37` |
| Structured tool factory | `CrewStructuredTool.from_function` infers args from signature, deduces description from docstring | `lib/crewai/src/crewai/tools/structured_tool.py:150-210` |
| Structured tool adapter | `BaseTool.to_structured_tool()` produces a `CrewStructuredTool` that retains `_original_tool` for result formatting | `lib/crewai/src/crewai/tools/base_tool.py:393-408` |
| Tool calling payload schemas | `ToolCalling` and `InstructorToolCalling` Pydantic models used by parse layer | `lib/crewai/src/crewai/tools/tool_calling.py:11-24` |
| Tool selector with fuzzy match | `_select_tool` falls back to `SequenceMatcher.ratio() > 0.85` and emits `ToolSelectionErrorEvent` | `lib/crewai/src/crewai/tools/tool_usage.py:759-802` |
| Tool name normalization | `sanitize_tool_name` with NFKD, ASCII, camelCase split, 64-char hash suffix | `lib/crewai/src/crewai/utilities/string_utils.py:26-54` |
| Tool registry guards | `i18n` tool labels for `add_image`, `delegate_work`, `ask_question`, `recall_memory`, `save_to_memory` | `lib/crewai/src/crewai/utilities/i18n.py:78-87` (call sites e.g. `crewai_tools_agent_tools_add_image_tool.py:19-31`, `crewai_tools_memory_tools.py:120-128`) |
| Agent tool field + validator | `tools: list[BaseTool] \| None` validated; langchain duck-typed tools converted via `Tool.from_langchain`; otherwise `ValueError` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:283-285, 511-537` |
| Agent tool resolver hooks | Abstract methods `get_delegation_tools`, `get_platform_tools`, `get_mcp_tools`, `get_multimodal_tools` | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:679-689` |
| Delegation tool builder | `AgentTools.tools()` returns `DelegateWorkTool` + `AskQuestionTool` with crew-mate description injected | `lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:16-36`; classes at `lib/crewai/src/crewai/tools/agent_tools/delegate_work_tool.py:16-30`, `lib/crewai/src/crewai/tools/agent_tools/ask_question_tool.py:14-28` |
| Memory tool factory | `create_memory_tools(memory)` returns `RecallMemoryTool` always, `RememberTool` only when not `read_only` | `lib/crewai/src/crewai/tools/memory_tools.py:104-130` |
| Multimodal tool class | `AddImageTool` defined statically on `BaseAgent.get_multimodal_tools()` | `lib/crewai/src/crewai/tools/agent_tools/add_image_tool.py:16-42`; `lib/crewai/src/crewai/agent/core.py:1206-1210` |
| Crew task tool augmentation | `_prepare_tools` chains delegation / code / multimodal / platform / MCP / memory / file additions | `lib/crewai/src/crewai/crew.py:1616-1683` |
| Crew tool merging with dedup | `_merge_tools` deduplicates by sanitized name and appends new tools | `lib/crewai/src/crewai/crew.py:1690-1709` |
| Platform tool source | `Agent.get_platform_tools` -> `CrewaiPlatformTools(apps=apps)` -> `CrewaiPlatformToolBuilder` fetches `/actions` and emits `CrewAIPlatformActionTool` | `lib/crewai/src/crewai/agent/core.py:1177-1186`; `lib/crewai-tools/src/crewai_tools/tools/crewai_platform_tools/crewai_platform_tools.py:14-27`; `crewai_platform_tool_builder.py:34-110`; `crewai_platform_action_tool.py:18-93` |
| MCP config types | `MCPServerStdio` / `MCPServerHTTP` / `MCPServerSSE` with `tool_filter` and `cache_tools_list` fields | `lib/crewai/src/crewai/mcp/config.py:12-123` |
| Tool filter types | `StaticToolFilter`, `create_static_tool_filter`, `create_dynamic_tool_filter`, `ToolFilterContext` | `lib/crewai/src/crewai/mcp/filters.py:17-163` |
| MCP client lifecycle | `MCPClient` owns transport/session via `AsyncExitStack`; `connect`/`disconnect` with cancel-scope safe cleanup; emits connection lifecycle events | `lib/crewai/src/crewai/mcp/client.py:139-372` |
| MCP retry/backoff | `_retry_operation` with exponential backoff and exception classification (auth, not found, transient) | `lib/crewai/src/crewai/mcp/client.py:663-714` |
| MCP discovery cache | `_mcp_schema_cache` with 5-min TTL keyed by transport type + URL/command | `lib/crewai/src/crewai/mcp/client.py:50-51, 716-735` |
| MCP tool resolver | `MCPToolResolver.resolve` supports AMP slugs, HTTPS URLs, native configs; per-tool filters; per-connection `client_factory` closure | `lib/crewai/src/crewai/mcp/tool_resolver.py:45-640` |
| Native MCP tool wrapper | `MCPNativeTool` builds a fresh `MCPClient` per call via `client_factory` to avoid cancel-scope errors | `lib/crewai/src/crewai/tools/mcp_native_tool.py:16-132` |
| External MCP wrapper | `MCPToolWrapper` for HTTPS MCP servers (legacy path) with timeout | `lib/crewai/src/crewai/tools/mcp_tool_wrapper.py:60-162` |
| Schema JSON -> Pydantic | `_json_schema_to_pydantic` uses `create_model_from_schema` for MCP input schemas | `lib/crewai/src/crewai/mcp/tool_resolver.py:631-640` |
| `crewai-tools` MCP adapter | `MCPServerAdapter` wraps `MCPAdapt` + `CrewAIToolAdapter` (dynamic `BaseTool` subclass); context manager; tool name filtering | `lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:31-235` |
| Tool collection | `ToolCollection(list[T])` with name-cache and `filter_by_names` / `filter_where` | `lib/crewai-tools/src/crewai_tools/adapters/tool_collection.py:12-80` |
| Native `crewai-tools` entry point | `__init__.py` re-exports ~80 tool classes plus `MCPServerAdapter`, `BedrockInvokeAgentTool`, etc. | `lib/crewai-tools/src/crewai_tools/__init__.py:1-333` |
| Example third-party tool | `SerperDevTool(BaseTool)` with `name`, `description`, `args_schema`, `env_vars: list[EnvVar]` | `lib/crewai-tools/src/crewai_tools/tools/serper_dev_tool/serper_dev_tool.py:110-130` |
| Schema helpers for strict providers | `sanitize_tool_params_for_openai/anthropic/bedrock_strict` | `lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:605-633` |
| Tool usage loop | `ToolUsage` parses calls, dispatches, tracks usage, formats observations and emits events | `lib/crewai/src/crewai/tools/tool_usage.py:76-1054` |
| Native-tool dispatch | `execute_single_native_tool_call` resolves original/structured tool by sanitized name, runs hooks, manages cache, emits events | `lib/crewai/src/crewai/utilities/agent_utils.py:1399-1547` |
| Tools handler with cache | `ToolsHandler.on_tool_use` records `last_used_tool`, writes to cache (skipping `CacheTools`) | `lib/crewai/src/crewai/agents/tools_handler.py:15-52` |
| Tool hooks (before/after) | `@before_tool_call` / `@after_tool_call` decorators accept `tools=[...]` filter (sanitized) and `agents=[...]` filter | `lib/crewai/src/crewai/hooks/decorators.py:39-66, 200-249, 261-305` |
| Project tooling discovery | `extract_available_exports` / `_load_tools_from_init` walks `__init__.py`s; `is_valid_tool` checks BaseTool ancestry | `lib/crewai/src/crewai/utilities/project_utils.py:270-359` |
| Decorator vs class tool tests | `test_creating_a_tool_using_annotation`, `test_creating_a_tool_using_baseclass` | `lib/crewai/tests/tools/test_base_tool.py:14-199` |
| Per-task vs per-agent tool override | `test_crew_with_delegating_agents_should_not_override_task_tools`, `test_task_tools_override_agent_tools` | `lib/crewai/tests/test_crew.py:570-734, 680-770` |
| Hierarchical manager tools forbidden | `test_manager_agent_with_tools_raises_exception` | `lib/crewai/tests/test_crew.py:2881` |
| MCP config tests | `test_agent_with_stdio_mcp_config`, `test_agent_with_http_mcp_config`, `test_each_invocation_gets_fresh_client`, `test_parallel_mcp_tool_execution_*` | `lib/crewai/tests/mcp/test_mcp_config.py:45-301` |
| MCP AMP slug tests | `TestParseAmpRef`, `TestGetMCPToolsAmpIntegration` cover `#tool` syntax, dedup, prefix fallback | `lib/crewai/tests/mcp/test_amp_mcp.py:186-510` |
| Tool hooks tests | `TestBeforeToolCallHooks`, `TestAfterToolCallHooks`, `TestBeforeToolCallDecoratorRegistersHook` | `lib/crewai/tests/hooks/test_tool_hooks.py:71-820`; `lib/crewai/tests/hooks/test_decorators.py:128-296` |

## Answers to Dimension Questions

1. **How does a new tool enter the system?**
   Three primary entry points: (a) subclass `BaseTool` (`lib/crewai/src/crewai/tools/base_tool.py:103`) and pass `BaseToolSubclass()` instances via `Agent(tools=[...])` or `Task(tools=[...])`; (b) decorate a function with `@tool` (`lib/crewai/src/crewai/tools/base_tool.py:676-761`) and pass the resulting `Tool` instance; (c) provide `Agent(mcps=[...])` / `Agent(apps=[...])` and let `MCPToolResolver` / `CrewaiPlatformToolBuilder` materialize `BaseTool` instances at kickoff (`lib/crewai/src/crewai/agent/core.py:1188-1197, 1177-1186`). A fourth path is the `crewai-tools` package, whose `__init__.py` re-exports ~80 prebuilt `BaseTool` subclasses (`lib/crewai-tools/src/crewai_tools/__init__.py:1-333`). No tool is added through mutation of a global registry at runtime; all paths are explicit list membership.

2. **Is tool registration explicit?**
   Yes. `Agent.tools` and `Task.tools` are Pydantic fields, and `validate_tools` (a `field_validator`) enforces that every entry is a `BaseTool` or duck-typed langchain object, converting the latter via `Tool.from_langchain` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:511-537`). The crew then runs `_prepare_tools` (`lib/crewai/src/crewai/crew.py:1616-1683`) which applies deterministic augmentations (delegation, platform, MCP, multimodal, memory, files) in a fixed order; each augmentation goes through `_merge_tools` (`lib/crewai/src/crewai/crew.py:1690-1709`) which dedupes by sanitized name. There is no automatic "magic" registration from imports — tools enter only through `Agent(tools=...)`, `Task(tools=...)`, `Agent(mcps=...)`, or `Agent(apps=...)`. The only ambient discovery is the `project_utils.extract_available_exports` CLI helper, used by `crewai` CLI commands, not the runtime (`lib/crewai/src/crewai/utilities/project_utils.py:283-359`).

3. **Can tools be discovered dynamically?**
   Yes for MCP and CrewAI AMP platform tools. `MCPToolResolver` performs live `list_tools()` against MCP servers during `resolve()` and creates one `MCPNativeTool` per discovered tool (`lib/crewai/src/crewai/mcp/tool_resolver.py:313-458`). Discovered tools are cached in `_mcp_schema_cache` with a 5-min TTL (`lib/crewai/src/crewai/mcp/client.py:50-51, 374-404`). `CrewaiPlatformToolBuilder` does HTTP GET to `/actions` per task kickoff (`lib/crewai-tools/src/crewai_tools/tools/crewai_platform_tools/crewai_platform_tool_builder.py:41-80`). Both paths honor per-config filters (`StaticToolFilter` / callable) defined on `MCPServerConfig.tool_filter` (`lib/crewai/src/crewai/mcp/filters.py:38-128`, `lib/crewai/src/crewai/mcp/config.py:43-46, 80-83, 113-117`). Pure in-process tools are NOT discovered — they must be enumerated by the user.

4. **Are names stable?**
   Names are stable per tool instance, but the system has at least four name shapes:
   - The `name` field on `BaseTool` is treated as canonical (used as `tool_type` JSON key, prompt description, MCP `prefixed_name = f"{server_name}_{tool_name}"`) — see `lib/crewai/src/crewai/tools/mcp_native_tool.py:43` and `lib/crewai/src/crewai/tools/base_tool.py:139-141`.
   - Sanitized names are used for selection, cache keys, and LLM-visible identifiers — see `lib/crewai/src/crewai/utilities/string_utils.py:26-54`, `lib/crewai/src/crewai/tools/tool_usage.py:760`, `lib/crewai/src/crewai/agents/tools_handler.py:48-49`, `lib/crewai/src/crewai/utilities/agent_utils.py:198`.
   - Sanitization is deterministic but lossy: "Search memory" -> "search_memory", "Delegate work to coworker" -> "delegate_work_to_coworker" (used by `DELEGATION_TOOL_NAMES`, `lib/crewai/src/crewai/utilities/agent_utils.py:1166-1171`).
   - Over-length names are SHA-256 hashed to a deterministic suffix (`lib/crewai/src/crewai/utilities/string_utils.py:49-53`), so names are still stable but not human-readable.
   - `_select_tool` allows fuzzy match ratio > 0.85, which is deterministic but can mask typos (`lib/crewai/src/crewai/tools/tool_usage.py:768-774`).
   - For native tool calling, the schema uses sanitized names and dedupes with a `_2`, `_3`, ... suffix on collision (`lib/crewai/src/crewai/utilities/agent_utils.py:200-206`).

5. **Can multiple agents see different tool sets?**
   Yes, naturally and explicitly. Each agent owns its `tools` field (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:283-285`) and `apps` / `mcps` fields (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:366-373`). Per-task `Task(tools=[...])` overrides per-agent tools in `prepare_task_execution` (`lib/crewai/src/crewai/crews/utils.py:179-184`). Hierarchical managers are explicitly forbidden from declaring tools (`lib/crewai/src/crewai/crew.py:1498-1505` — sets `manager.tools = []` and raises). Delegation tools only mention other crew agents via `AgentTools(agents=self.agents)` and the crew filters out the calling agent (`lib/crewai/src/crewai/crew.py:1791-1800`). Test coverage: `tests/test_crew.py:570-770`.

## Architectural Decisions

- **Class-based + decorator + factory hybrid**: `BaseTool` is a Pydantic model (so tool objects can be checkpointed, validated, serialized). The `@tool` decorator factory and `CrewStructuredTool.from_function` provide function-wrapping alternatives. All paths converge to the same `BaseTool`/`CrewStructuredTool` pair consumed by agents.
- **Two-runtime split**: `BaseTool._run` is used for the LangChain-style "text tool" loop; `CrewStructuredTool.invoke/ainvoke` is used for native tool calling. `parse_tools` (`lib/crewai/src/crewai/utilities/agent_utils.py:96-120`) is the mandatory normalization, and `to_structured_tool` (`lib/crewai/src/crewai/tools/base_tool.py:393-408`) preserves `_original_tool` for hooks and result formatting.
- **Auto-typed schema inference**: when `args_schema` is unset, `BaseTool._default_args_schema` introspects `_run`/`_arun` signatures and builds a Pydantic model (`lib/crewai/src/crewai/tools/base_tool.py:200-247`). `result_schema` similarly inferred from return annotation when applicable (`lib/crewai/src/crewai/tools/base_tool.py:249-258`).
- **Discriminated `tool_type` for serialization**: `BaseTool.__get_pydantic_core_schema__` and `_TOOL_TYPE_REGISTRY` enable `list[BaseTool]` to round-trip through JSON for checkpointing (`lib/crewai/src/crewai/tools/base_tool.py:51, 109-137`).
- **Per-server MCP client factory**: `MCPNativeTool` receives a closure that produces a fresh `MCPClient` + transport per invocation to guarantee no shared mutable state across concurrent invocations (`lib/crewai/src/crewai/tools/mcp_native_tool.py:26-60, 101-119`).
- **Filters as first-class on MCP config**: every `MCPServer*` config carries an optional `tool_filter` callable, and a default `StaticToolFilter` is provided (`lib/crewai/src/crewai/mcp/config.py:43-46, 80-83, 113-117`; `lib/crewai/src/crewai/mcp/filters.py:38-88`).
- **Hooks intercept tool execution**: `before_tool_call` / `after_tool_call` hooks (`lib/crewai/src/crewai/hooks/decorators.py:39-305`) can filter by tool name and agent, executed inside `execute_single_native_tool_call` (`lib/crewai/src/crewai/utilities/agent_utils.py:1508-1552`).

## Notable Patterns

- **Multi-protocol tool namespaces**: `lib/crewai-tools/src/crewai_tools/__init__.py:1-333` flat-re-exports ~80 tools so users can write `from crewai_tools import SerperDevTool`. `ToolCollection` (`lib/crewai-tools/src/crewai_tools/adapters/tool_collection.py:12-80`) provides dict-like access by name with filters.
- **Namespaced MCP tool naming**: `f"{server_name}_{tool_name}"` to avoid cross-server collisions when multiple MCP servers contribute tools (`lib/crewai/src/crewai/tools/mcp_native_tool.py:43`).
- **Hooks-as-decorator**: `@before_tool_call(tools=[...], agents=[...])` exposes the same filter mechanism as the MCP layer (`lib/crewai/src/crewai/hooks/decorators.py:39-66, 200-249, 261-305`).
- **`EnvVar` introspection**: third-party tools declare required env vars via `env_vars: list[EnvVar]` so credentials can be collected and validated declaratively (`lib/crewai/src/crewai/tools/base_tool.py:96-100`; example: `lib/crewai-tools/src/crewai_tools/tools/serper_dev_tool/serper_dev_tool.py:124-130`).
- **Provider-specific strict schema sanitizers**: `sanitize_tool_params_for_openai/anthropic/bedrock_strict` provide compatibility shims for strict tool-schema validation (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:605-633`).

## Tradeoffs

- **Two runtimes (BaseTool + CrewStructuredTool)** trade simplicity for backwards compatibility with langchain-style code paths. `parse_tools` is mandatory, and `current_usage_count` is mirrored between original and structured (`lib/crewai/src/crewai/tools/structured_tool.py:373-377`).
- **Fuzzy name match (ratio 0.85)** trades strict correctness for forgiveness when LLM output is imprecise (`lib/crewai/src/crewai/tools/tool_usage.py:768-774`). This can mask genuine bugs.
- **AMP + HTTPS + native MCP paths** trade uniformity for ergonomic URLs/slugs but triple the resolver branches (`lib/crewai/src/crewai/mcp/tool_resolver.py:68-88, 118-218, 219-276, 313-463`).
- **Per-call fresh MCP client** trades performance for safety against anyio cancel-scope errors (`lib/crewai/src/crewai/tools/mcp_native_tool.py:73-99`). Each tool invocation pays connect/disconnect.
- **Implicit usage counter mirroring** (`BaseTool.current_usage_count` ↔ `CrewStructuredTool.current_usage_count`) avoids drift but adds two writers; integrity relies on `parse_tools` resetting both to 0 (`lib/crewai/src/crewai/utilities/agent_utils.py:113-115`).
- **Skill-driven registry**: `BaseAgent.skills` accepts paths, `Skill` objects, or `@org/name` refs (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:389-393, 497-509`); resolution uses `discover_skills` + `activate_skill` (`lib/crewai/src/crewai/skills/loader.py:36-143`). Skills are not tools (they inject prompt content), so this is outside the tool registry but uses the same `I18N_DEFAULT`-style external load.

## Failure Modes / Edge Cases

- **Class name collisions in `_TOOL_TYPE_REGISTRY`** are silently overwritten on `__init_subclass__` — no validation that two distinct classes share a fully-qualified path (`lib/crewai/src/crewai/tools/base_tool.py:109-112`). This affects checkpoint deserialization.
- **`from_langchain` admits duck-typed objects** with `name` + `func` + `description` attributes without verifying they implement the langchain `BaseTool` protocol (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:526-536`). Latent issues in langchain-style objects may surface only at run time.
- **Tool schema deserialization**: if the user mutates `args_schema` after construction, `_resolve_tool_dict` may need to round-trip via `_deserialize_schema` to reconstruct the Pydantic model — handled but easy to miss when adding new fields (`lib/crewai/src/crewai/tools/base_tool.py:59-78`).
- **Schema round-trip edge cases**: `result_schema` is optional and silently falls back to `str(raw_result)` when validation fails (`lib/crewai/src/crewai/tools/structured_tool.py:60-83`).
- **MCP discovery failure modes** are bucketed by exception type (auth, not found, transient) with explicit retry, but the resolver also swallows `_get_mcp_tool_schemas` failures into an empty tool list (`lib/crewai/src/crewai/mcp/tool_resolver.py:511-519`). Agents end up with no tools silently.
- **`crewai-tools` optional installs**: `ComposioTool`, `MCPServerAdapter`, etc., rely on optional dependencies and may be missing in slim installs — the code uses broad `try/except ImportError` patterns (`lib/crewai-tools/src/crewai_tools/adapters/mcp_adapter.py:92-94`).
- **Hierarchical manager tool injection**: any tools declared on a manager agent raise at construction, which is intentional but can be surprising (`lib/crewai/src/crewai/crew.py:1498-1505`).
- **`_select_tool` fuzzy ratio 0.85**: typo in MCP-discovered server_name (e.g., `notion_search` vs `notion_searchs`) could still match a wrong tool (`lib/crewai/src/crewai/tools/tool_usage.py:759-774`).
- **`Agent.mcps` mixed list**: an AMP slug and an HTTPS URL share the same field but have different validation rules; invalid entries raise `ValueError` only at kickoff (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:557-590`).

## Future Considerations

- **Single runtime for BaseTool + CrewStructuredTool**: collapse to one execution path by giving `BaseTool` an `invoke/ainvoke` interface, eliminating `parse_tools` and `_original_tool` mirroring (`lib/crewai/src/crewai/utilities/agent_utils.py:96-120`; `lib/crewai/src/crewai/tools/structured_tool.py:373-377`).
- **Strict tool name resolution**: drop the fuzzy match in `_select_tool` or constrain it to allowlisted internal tools (`lib/crewai/src/crewai/tools/tool_usage.py:759-774`).
- **Pluggable tool registries**: today there is no explicit "tool registry" object; tools are gathered on demand by `_prepare_tools`. A `ToolRegistry` class with namespaces and provenance could make multi-source merging more observable (`lib/crewai/src/crewai/crew.py:1616-1683`).
- **Disambiguate MCP paths**: provide one constructor per connection type (no `str | MCPServerConfig` union on `Agent.mcps`); keeps validation surface smaller (`lib/crewai/src/crewai/mcp/tool_resolver.py:68-88`).
- **Class identity registry**: harden `_TOOL_TYPE_REGISTRY` against collisions (e.g., warn or raise on duplicate fully-qualified keys) to prevent silent checkpoint corruption (`lib/crewai/src/crewai/tools/base_tool.py:109-112`).
- **Tool versioning**: add an optional `version` field to `BaseTool` so changes in `args_schema` don't silently invalidate downstream cache or AMP-fetched tools.
- **Observability**: `BaseTool._run` has no built-in span/metric hook beyond the cache and usage counter; agent-level events already exist but per-tool telemetry is implicit (`lib/crewai/src/crewai/tools/base_tool.py:295-354`).
- **Documented dynamic discovery contract**: codify what "AMP reference" / HTTPS MCP / native MCP each discover and when, so operators can reason about egress and latency (`lib/crewai/src/crewai/mcp/tool_resolver.py:118-218`).

## Questions / Gaps

- No central "tool catalog" or registry object exists — tools are scattered across the `crewai-tools` package, the `crewai.tools` namespace, agent fields, and MCP servers. Finding all available tools requires either reading the package's `__init__.py` or hitting an MCP server. Search boundary: scanned `lib/crewai-tools/src/crewai_tools/__init__.py`, `lib/crewai/src/crewai/tools/`, `lib/crewai/src/crewai/agents/agent_tools/`.
- There is no explicit public API to enumerate tools at runtime; `ToolCollection` (`lib/crewai-tools/src/crewai_tools/adapters/tool_collection.py:12-80`) only operates on a list it is given. No evidence found for a runtime registry like `ToolRegistry.tools()`.
- The skill loader produces metadata + instructions for prompt injection but is not a tool-registry mechanism — distinct from MCP/AMP discovery. No clear evidence found of an integrated skills-to-tools bridge.
- Native tool calling supports OpenAI, Anthropic, Bedrock, Gemini via `extract_tool_call_info` (`lib/crewai/src/crewai/utilities/agent_utils.py:1191-1231`); however, there is no abstraction over provider-specific `strict` tool schema rules at the registration layer — sanitizers are applied only at `llms.base_llm` boundaries (`lib/crewai/src/crewai/utilities/pydantic_schema_utils.py:605-633`).
- `crewai_tools/__init__.py` re-exports >80 symbols in a flat namespace; this is convenient but offers no structured grouping or discoverability beyond what the README exposes. No clear evidence found of category metadata inside the package itself.

---

Generated by `04.01-tool-definition-and-registration` against `crewai`.