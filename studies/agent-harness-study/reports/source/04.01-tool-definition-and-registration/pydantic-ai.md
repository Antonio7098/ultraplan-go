# Source Analysis: pydantic-ai

## Tool Definition and Registration

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (>=3.10); Pydantic v2; Pydantic Core (`SchemaValidator`); uv workspace with `pydantic_ai_slim`, `pydantic_graph`, `pydantic_evals`, plus optional provider groups (`openai`, `anthropic`, `google`, `mcp`, ...) |
| Analyzed | 2026-07-22 |

## Summary

Pydantic AI exposes **toolsets** as the first-class unit of registration, anchored on an abstract base class `AbstractToolset` with a small port surface (`get_tools`, `call_tool`, `__aenter__/__aexit__`, `for_run`, `for_run_step`, `apply`, `visit_and_replace`, `id`, `label`) at `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:74-279`. Tools themselves are plain dataclasses (`Tool` at `pydantic_ai_slim/pydantic_ai/tools.py:441-671`, `ToolDefinition` at `pydantic_ai_slim/pydantic_ai/tools.py:687-895`), and user-supplied callables enter via either a function-style decorator (`@agent.tool` / `@agent.tool_plain`, `@toolset.tool` / `@toolset.tool_plain`) or explicit constructors (`FunctionToolset(tools=[...])`, `Tool(my_func, ...)`, `MCPToolset(...)`, `ExternalToolset([...])`, `DynamicToolset(...)`, `OutputToolset.build(...)`).

The framework classifies registration styles across five surfaces:

1. **Class + decorator-based function tools** rooted in `FunctionToolset` and the `Tool` dataclass — `pydantic_ai_slim/pydantic_ai/toolsets/function.py:45-658`, `pydantic_ai_slim/pydantic_ai/tools.py:467-671`.
2. **Static list / kwarg registration** on `Agent.__init__(tools=..., toolsets=..., mcp_servers=...)` — `pydantic_ai_slim/pydantic_ai/agent/__init__.py:287-343, 520-538`.
3. **Factory-based dynamic registration** via `@agent.toolset` (a `ToolsetFunc` closure wrapped in `DynamicToolset`) — `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2463-2499`, `pydantic_ai_slim/pydantic_ai/toolsets/_dynamic.py:20-157`.
4. **MCP server as toolset** via `MCPToolset` / deprecated `MCPServer*` family — `pydantic_ai_slim/pydantic_ai/mcp.py:1983-2540, 684-1059`.
5. **Provider-native tool registry** via the `_TYPED_NATIVE_TOOL_REGISTRY` populated automatically by `AbstractNativeTool.__init_subclass__` — `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:33-103, 87`.

Tool assembly is performed per-step inside `_get_toolset(...)` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:2591-2660`), which composes:
- a user-configurable per-agent `_function_toolset` (`_AgentFunctionToolset` at `pydantic_ai_slim/pydantic_ai/agent/__init__.py:3055-3076`),
- `_user_toolsets` (explicit instances),
- `_dynamic_toolsets` (factory-evaluated per run/per step),
- `_cap_toolsets` (capability-contributed via `AbstractCapability.get_toolset()` at `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:331`),
- optional output toolset (`OutputToolset` at `pydantic_ai_slim/pydantic_ai/_output.py:1402-1529`),
- a `CombinedToolset` wrapper (`pydantic_ai_slim/pydantic_ai/toolsets/combined.py:26-114`),
- a `PreparedToolset` for the per-run `prepare_tools` capability hook (`pydantic_ai_slim/pydantic_ai/toolsets/prepared.py:14-41`),
- an optional tool-search wrapper (`ToolSearchToolset` at `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:246-489`).

Wrapper composition follows the `WrapperToolset` base (`pydantic_ai_slim/pydantic_ai/toolsets/wrapper.py:14-74`) and includes shipped cross-cutting wrappers: `FilteredToolset`, `PrefixedToolset`, `RenamedToolset`, `ApprovalRequiredToolset`, `DeferredLoadingToolset`, `SetMetadataToolset`, `IncludeReturnSchemasToolset`, `CapabilityOwnedToolset`, `DeferredCapabilityLoaderToolset`. Conflict detection happens at two layers: `FunctionToolset.add_tool` raises `UserError(f'Tool name conflicts with existing tool: {tool.name!r}')` (`function.py:587-588`); `CombinedToolset.get_tools` cross-checks merged names and surfaces the offending source toolset's label and `tool_name_conflict_hint` (`combined.py:73-77`, `abstract.py:106-108`).

Naming defaults to `function.__name__` for tool functions (`tools.py:558`) and to MCP-server-advertised names for `MCPToolset` (`mcp.py:2442-2464`). Names are stable: tools map by exact name (`toolsets/function.py:587`, `combined.py:73-77`), `ToolSearchToolset` reserves `search_tools` with a `UserError` if a user clashes with it (`toolsets/_tool_search.py:298-301`), and prefix/rename wrappers rewrite names at the wire boundary while preserving the original on `RunContext.tool_name` (`toolsets/prefixed.py:35-40`, `toolsets/renamed.py:36-41`). Each agent and each run can see a different set: per-agent fields are independent (`agent/__init__.py:251-253`), per-run overrides are layered via `ContextVar` (`agent/__init__.py:544-558, 1881-1943`), and `DynamicToolset.for_run` / `for_run_step` produce per-run / per-step copies (`toolsets/_dynamic.py:61-96`).

Multiple registration styles, dynamic factory-based assembly, MCP-as-toolset, wrapper composition without touching leaf toolsets, capability-contributed tools, native-tool registry, durable execution IDs, ContextVar overrides, and full VCR + feature tests (`tests/test_tools.py`, `tests/test_toolsets.py`, `tests/test_tool_search.py`, `tests/test_mcp_toolset.py`, `tests/test_capabilities.py`).

## Rating

**9/10**

Rationale:

- **Strengths**
  - Class-based `Tool` (dataclass) and `ToolDefinition` (dataclass) — typed schema, retries, timeout, prep, validator, metadata, return-schema, deferred loading, kind, capability id (`tools.py:441-895`).
  - Multiple registration surfaces that all funnel into one abstraction: `AbstractToolset` with a stable port surface (`toolsets/abstract.py:74-179`).
  - Decorator-based and direct-instantiation registration both supported (`agent/__init__.py:2210-2449`, `toolsets/function.py:170-298`).
  - Factory-based dynamic registration via `@agent.toolset` + `DynamicToolset` evaluated per-run or per-step (`toolsets/_dynamic.py:20-157`, `agent/__init__.py:529-533, 2463-2499`).
  - MCP modeled as a first-class toolset subclass (`MCPToolset`, `mcp.py:1983-2540`) with caching, transport selection, tool-error behavior, sampling, and task-augmented execution.
  - Wrapper-toolset composition (`WrapperToolset` base, `wrapper.py:14-74`) — adding cross-cutting behavior does not require touching any concrete toolset (`toolsets/AGENTS.md:5-7`): `FilteredToolset`, `PrefixedToolset`, `RenamedToolset`, `Approved`, `Deferred`, `SetMetadata`, `IncludeReturnSchemas`.
  - Conflict detection at registration and at merge time: `FunctionToolset.add_tool` (`function.py:581-593`), `CombinedToolset.get_tools` (`combined.py:66-88`), `ExternalToolset` deduplication helpers.
  - Stable tool naming with reserved-name enforcement (`ToolSearchToolset` reserves `search_tools`; `toolsets/_tool_search.py:298-301`); wraps can rename (`RenamedToolset`, `PrefixedToolset`) while keeping internal identity on `RunContext.tool_name` (`prefixed.py:36-40`).
  - Capability-contributed tools with `defer_loading` semantics: tools become visible only when the owning capability is loaded (`capabilities/abstract.py:185-197, 331`, `toolsets/_capability_owned.py:18-87`, `_deferred_capability_loader.py:31+`).
  - Per-agent isolation via dedicated instance fields (`_function_toolset`, `_user_toolsets`, `_dynamic_toolsets`, `_cap_toolsets`, `_output_toolset`, `agent/__init__.py:251-256, 528-538`) and per-run / per-step overrides via `ContextVar` (`agent/__init__.py:544-558`).
  - Native-tool registry populated by `__init_subclass__` (`native_tools/__init__.py:85-103`); well-typed `Annotated[Union[...], Discriminator(...)]` schema supports round-trip and deserialization.
  - Tests at tool level (`tests/test_tools.py:45-4421`), toolset level (`tests/test_toolsets.py:95-2665`, incl. `test_comprehensive_toolset_composition` at `:389`), tool search (`tests/test_tool_search.py:1-5500`), MCP (`tests/test_mcp_toolset.py`), and capabilities (`tests/test_capabilities.py`).
- **Limitations**
  - Tool discovery is **synchronous relative to the model-facing set** at construction; "discover tools at runtime beyond MCP/factory" still requires a custom toolset (no built-in skills/capability catalog beyond `defer_loading=True` + `ToolSearchToolset`).
  - The `defer_loading` flag on `ToolDefinition` is intentionally dual-purpose (user input vs. live visibility), explicitly flagged as tech debt in `tools.py:748-768`. A `RunContext.loaded_tools` view is described as future work.
  - Multiple dynamic paths (`DynamicToolset`, `_dynamic_toolsets`, capability `for_run`) can compose in non-obvious ways; reading the registration surface requires walking the `_build_toolset_list` order (`agent/__init__.py:2675-2704`).
  - Capability `get_toolset()` is a single toolset; capabilities are documented to use `get_wrapper_toolset` for richer toolsets (`capabilities/abstract.py:331-367`). No first-class capability-owned multi-toolset.
  - The `MCPToolset` schema validator is intentionally permissive (`TOOL_SCHEMA_VALIDATOR = SchemaValidator(schema=core_schema.any_schema())`, `mcp.py:<TOOL_SCHEMA_VALIDATOR>` definition): the framework trusts the MCP server's JSON schema. This is documented but offers no additional validation.

## Evidence Collected

Every entry cites a file with line numbers from inside `studies/agent-harness-study/sources/pydantic-ai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool dataclass + construction (class-based tool definition) | `class Tool(Generic[ToolAgentDepsT])` with fields `function`, `takes_ctx`, `max_retries`, `name`, `description`, `prepare`, `args_validator`, `docstring_format`, `require_parameter_descriptions`, `strict`, `sequential`, `requires_approval`, `metadata`, `timeout`, `defer_loading`, `include_return_schema`, `function_schema` | `pydantic_ai_slim/pydantic_ai/tools.py:441-671` |
| Auto-inferred tool name | `self.name = name or function.__name__` | `pydantic_ai_slim/pydantic_ai/tools.py:558` |
| Tool builder from JSON schema (hand-built, no func introspection) | `Tool.from_schema(function, name, description, json_schema, ...)` constructs a `Tool` whose validator is `SchemaValidator(schema=core_schema.any_schema())` | `pydantic_ai_slim/pydantic_ai/tools.py:583-636` |
| Tool dataclass passthrough to `ToolDefinition` | `tool.tool_def` returns a `ToolDefinition` populated with name, description, parameters_json_schema, strict, sequential, metadata, timeout, defer_loading, kind, return_schema | `pydantic_ai_slim/pydantic_ai/tools.py:638-652` |
| Tool pre-call hook (`prepare`) | `Tool.prepare_tool_def(ctx)` invokes the `prepare` callable and may return `None` to omit the tool for this step | `pydantic_ai_slim/pydantic_ai/tools.py:654-671` |
| Wire-facing tool schema | `@dataclass(repr=False, kw_only=True) class ToolDefinition` covering `name`, `parameters_json_schema`, `description`, `outer_typed_dict_key`, `strict`, `sequential`, `kind`, `metadata`, `timeout`, `defer_loading`, `unless_native`, `with_native`, `tool_kind`, `return_schema`, `include_return_schema`, `capability_id` | `pydantic_ai_slim/pydantic_ai/tools.py:687-895` |
| Reserved name conflict | `_RENAMED_TYPE_ALIASES` + module-level `__getattr__` issuing `PydanticAIDeprecationWarning` | `pydantic_ai_slim/pydantic_ai/tools.py:902-916` |
| Toolset tool definition record | `@dataclass(kw_only=True) class ToolsetTool(Generic[AgentDepsT])` with `toolset`, `tool_def`, `max_retries`, `args_validator`, `args_validator_func` | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:42-71` |
| Abstract toolset port surface | `AbstractToolset(ABC)` with abstract `get_tools`, `call_tool`, abstract `id`; lifecycle hooks `__aenter__/__aexit__`, `for_run`, `for_run_step`, `apply`, `visit_and_replace`, `label`, `tool_name_conflict_hint`; convenience builders `filtered`, `prefixed`, `prepared`, `renamed`, `approval_required`, `defer_loading`, `include_return_schemas`, `with_metadata` | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:74-279` |
| Cross-toolset duplicate detection hint | `tool_name_conflict_hint = 'Rename the tool or wrap the toolset in a PrefixedToolset to avoid name conflicts.'` | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:106-108` |
| Per-run/per-step lifecycle (`for_run`/`for_run_step`) | Default `for_run` returns `self`; `for_run_step` likewise; subclasses override to swap the inner toolset at run/step boundaries (`DynamicToolset` uses both) | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:110-126, _dynamic.py:61-96` |
| Wrapper toolset base | `@dataclass class WrapperToolset(AbstractToolset[AgentDepsT])` with a `wrapped` field and delegation; `apply` and `visit_and_replace` recurse into `wrapped` | `pydantic_ai_slim/pydantic_ai/toolsets/wrapper.py:14-74` |
| Function-style decorator registration on toolset | `FunctionToolset.tool(func)` / `tool_plain(func)` overloads that delegate to `add_function` (with `takes_ctx=None`/`False`), supporting `name`, `description`, `retries`, `prepare`, `args_validator`, `docstring_format`, `require_parameter_descriptions`, `schema_generator`, `strict`, `sequential`, `requires_approval`, `metadata`, `timeout`, `defer_loading`, `include_return_schema` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:145-298, 300-442` |
| Imperative registration on toolset | `FunctionToolset.add_function(...)` / `add_tool(tool)`; `add_tool` raises `UserError(f'Tool name conflicts with existing tool: {tool.name!r}')` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:475-593` |
| Static-list registration on toolset | `FunctionToolset.__init__(tools: Sequence[Tool[AgentDepsT] \| ToolFuncEither[AgentDepsT, ...]] = [], ...)` resolves each `Tool` via `add_tool`, each plain function via `add_function` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:61-139` |
| Per-tool instructions decorator | `@toolset.instructions` registers `SystemPromptRunner` (string-or-callable, sync-or-async, with-or-without `RunContext`); toolset's `get_instructions` merges the results as `InstructionPart`s | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:444-473, 595-607` |
| CombinedToolset (multi-set merge) | `class CombinedToolset(AbstractToolset)` with `AsyncExitStack`-managed entry; `get_tools` iterates child toolsets in parallel via `_utils.gather` and raises `UserError` with the source toolset's label and `tool_name_conflict_hint` on name collision | `pydantic_ai_slim/pydantic_ai/toolsets/combined.py:25-88` |
| Preparation wrapper (per-run tool def rewrite) | `PreparedToolset(WrapperToolset)` invokes a `ToolsPrepareFunc` returning a list of `ToolDefinition`s; warns when the prepare callback returns `None`; raises `UserError` if the prepared set adds or renames tools | `pydantic_ai_slim/pydantic_ai/toolsets/prepared.py:14-41` |
| Filtering wrapper | `FilteredToolset(WrapperToolset)` accepts a sync-or-async predicate on `(RunContext, ToolDefinition)` | `pydantic_ai_slim/pydantic_ai/toolsets/filtered.py:13-32` |
| Prefixing wrapper | `PrefixedToolset` rewrites names to `f'{self.prefix}_{name}'` for `get_tools`, restores the original name in `call_tool`, and sets `ctx.tool_name` back to the original | `pydantic_ai_slim/pydantic_ai/toolsets/prefixed.py:11-41` |
| Renaming wrapper | `RenamedToolset(name_map: dict[str, str])` (new → original); `call_tool` reverses the map and rewrites `ctx.tool_name` | `pydantic_ai_slim/pydantic_ai/toolsets/renamed.py:11-42` |
| Approval gating wrapper | `ApprovalRequiredToolset(WrapperToolset)` raises `ApprovalRequired` when `ctx.tool_call_approved` is false and `approval_required_func(ctx, tool_def, tool_args)` returns true; default predicate is "always require" | `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:15-32` |
| Deferred-loading wrapper | `DeferredLoadingToolset(PreparedToolset)` flips `defer_loading=True` on every tool (or those named in `tool_names`), making them hidden until discovered via `ToolSearchToolset` | `pydantic_ai_slim/pydantic_ai/toolsets/deferred_loading.py:11-37` |
| Metadata wrapper | `SetMetadataToolset(PreparedToolset)` shallow-merges a `metadata` dict onto every tool via `replace(td, metadata={**(td.metadata or {}), **self.metadata})` | `pydantic_ai_slim/pydantic_ai/toolsets/set_metadata.py:12-28` |
| Return-schema wrapper | `IncludeReturnSchemasToolset(PreparedToolset)` flips `include_return_schema=True` for tools that don't already request it | `pydantic_ai_slim/pydantic_ai/toolsets/include_return_schemas.py:11-26` |
| External toolset (declared / deferred execution) | `ExternalToolset(tool_defs: list[ToolDefinition])` returns tools with `kind='external'` and `max_retries=0`; `call_tool` raises `NotImplementedError` (execution happens outside the agent run) | `pydantic_ai_slim/pydantic_ai/toolsets/external.py:15-52` |
| Dynamic (factory) toolset | `DynamicToolset(toolset_func, per_run_step, id)`; `for_run` returns a fresh instance with `per_run_step=False` evaluating immediately; `for_run_step` re-evaluates when `per_run_step=True` and manages enter/exit of the inner toolset | `pydantic_ai_slim/pydantic_ai/toolsets/_dynamic.py:20-157` |
| Tool-search toolset and local search | `ToolSearchToolset(WrapperToolset)` exposes a `search_tools` function tool via `Tool(...)` of `_search_tools_signature`, swaps deferred tools in `get_tools`, reserves the name `search_tools` (`_SEARCH_TOOLS_NAME = TOOL_SEARCH_FUNCTION_TOOL_NAME`), and runs a built-in keywords-overlap algorithm and user-supplied `ToolSearchFunc` | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:84-489` |
| Search-tool signature source of truth | `_search_tools_signature(queries: Annotated[list[str], ...]) -> ToolSearchReturnContent` is wrapped by `Tool(_search_tools_signature).function_schema`; schema/validator memoized via `_build_search_args_schema(parameter_description)` | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:137-182` |
| Capability-owned toolset | `CapabilityOwnedToolset(WrapperToolset)` rewrites every tool's `capability_id`, sets `defer_loading=True` if the capability is deferred, and adds `DEFERRED_CAPABILITY_TOOL_METADATA_KEY` to the metadata | `pydantic_ai_slim/pydantic_ai/toolsets/_capability_owned.py:18-87` |
| Capability base, `get_toolset` hook | `AbstractCapability` with `def get_toolset(self) -> AgentToolset[AgentDepsT] \| None = None` | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:331-333` |
| Native-tool base, registry via `__init_subclass__` | `class AbstractNativeTool(ABC)` with `kind: str`, `unique_id`, `label`; `NATIVE_TOOL_TYPES: dict[str, type[AbstractNativeTool]]` populated by `def __init_subclass__(cls, **kwargs)`; `@classmethod __get_pydantic_core_schema__` builds a `Discriminator`-tagged `Annotated[Union[...]]` from `NATIVE_TOOL_TYPES.values()` | `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:33-103` |
| Concrete native tools (`web_search`, `x_search`, `code_execution`, `web_fetch`, `url_context`, `image_generation`, `memory`, `mcp_server`, `file_search`) and `__post_init__` validation (e.g., `XSearchTool` rejects both `allowed_x_handles` and `excluded_x_handles`) | `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:107-605` |
| Tool search native-tool kind | `ToolSearchTool` from `_tool_search.py` registers itself via `__init_subclass__`; `NATIVE_TOOLS_REQUIRING_CONFIG` includes it alongside `FileSearchTool`, `MCPServerTool`, `MemoryTool` | `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:613, 629-631` |
| Discriminator for native tools | `_tool_discriminator(tool_data)` reads `kind` from dict or instance | `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:616-620` |
| MCP toolset (recommended path) | `@dataclass(init=False, repr=False) class MCPToolset(AbstractToolset[AgentDepsT])` with `client: FastMCPClient[Any]`, `tool_error_behavior`, `max_retries`, `cache_tools/cache_resources/cache_prompts`, `include_instructions`, `include_return_schema`, `process_tool_call`, `sampling_model`, `log_level` | `pydantic_ai_slim/pydantic_ai/mcp.py:1982-2540` |
| MCP `get_tools` (dynamic discovery at run time) | `async def get_tools` walks `await self.list_tools()`, wraps each as `ToolsetTool(toolset=self, tool_def=ToolDefinition(name, description, parameters_json_schema=mcp_tool.inputSchema, metadata={'meta','annotations','task'}, return_schema=mcp_tool.outputSchema, include_return_schema))` | `pydantic_ai_slim/pydantic_ai/mcp.py:2442-2464` |
| MCP `list_tools` cache lifecycle | `cache_tools` short-circuits when `_cached_tools` is set; `__aexit__` clears the cache on the final exit | `pydantic_ai_slim/pydantic_ai/mcp.py:2427-2441, ~1300-1320` |
| MCP task-augmented execution | `use_task = bool((tool.tool_def.metadata or {}).get('task'))`; `direct_call_tool` switches to MCP `task=True` per SEP-1686 when `mcp_tool.execution.taskSupport in ('required','optional')` | `pydantic_ai_slim/pydantic_ai/mcp.py:2446-2456, 2474-2523, 2533` |
| MCP error-behavior toggles | `tool_error_behavior` literal `'retry' \| 'error'`; `'retry'` raises `ModelRetry(message=str(e))` so the model self-corrects | `pydantic_ai_slim/pydantic_ai/mcp.py:2031-2036, 2507-2509` |
| Legacy MCP server classes (deprecated) | `MCPServer` abstract base + `MCPServerStdio` / `MCPServerSSE` / `MCPServerHTTP` / `MCPServerStreamableHTTP` (all marked `@deprecated`) | `pydantic_ai_slim/pydantic_ai/mcp.py:684-1924` |
| Agent field for toolsets and dynamic toolsets | `_function_toolset`, `_output_toolset`, `_user_toolsets`, `_max_output_retries`, `_max_tool_retries`, `_tool_timeout` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:251-256` |
| Agent `__init__` accepts `tools` and `toolsets` (mcp_servers deprecated) | `tools: Sequence[Tool[AgentDepsT] \| ToolFuncEither[AgentDepsT, ...]]`, `toolsets: Sequence[AgentToolset[AgentDepsT]] \| None`; `_deprecated_kwargs` re-routes `mcp_servers` to `toolsets` with a deprecation warning | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:287-289, 322-470` |
| Agent stores `_user_toolsets` vs `_dynamic_toolsets` separately | `agent_toolsets = list(toolsets or []); self._dynamic_toolsets = [DynamicToolset(toolset) for toolset in agent_toolsets if not isinstance(toolset, AbstractToolset)]; self._user_toolsets = [toolset for toolset in agent_toolsets if isinstance(toolset, AbstractToolset)]` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:527-534` |
| Capability-contributed toolsets captured separately | `self._cap_toolsets: list[AgentToolset[AgentDepsT]] = [cap_toolset] if cap_toolset is not None else []` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:537-538` |
| `@agent.tool` decorator (takes `RunContext`) | `@overload` pair + `tool_decorator(func_)` that calls `self._function_toolset.add_function(func_, takes_ctx=True, ...)`; returns `func_` so the decorator is transparent | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2210-2315` |
| `@agent.tool_plain` decorator (no `RunContext`) | Mirrors `@agent.tool` but `takes_ctx=False` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2317-2449` |
| `@agent.toolset` decorator (factory toolset) | `def toolset(...)` accepts a `ToolsetFunc` and appends `DynamicToolset(func_, per_run_step=per_run_step, id=id)` to `self._dynamic_toolsets` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2451-2499` |
| Per-run override context vars for tools and toolsets | `self._override_toolsets`, `self._override_tools`, `self._override_builtin_tools`, `self._override_instructions`, `self._override_metadata`, `self._override_root_capability`, `self._override_model`, `self._override_model_settings`, `self._override_output_retries` — all `ContextVar[_utils.Option[...]]` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:544-570` |
| Agent override API | `Agent.override(tools=..., toolsets=..., native_tools=..., ...)` uses `ContextVar`s; `agent.iter(..., toolsets=...)` and `agent.run(..., toolsets=...)` accept per-call additions | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1881-1943`, `pydantic_ai_slim/pydantic_ai/agent/wrapper.py:298-352` |
| Per-step toolset assembly (`_get_toolset`) | Lists from `_build_toolset_list()`, wraps in `CombinedToolset`, layers `PreparedToolset` for the capability `prepare_tools` hook, layers capability `get_wrapper_toolset` result (e.g., tool-search), finally wraps the `OutputToolset` in its own `PreparedToolset` and `CombinedToolset` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2591-2660` |
| Toolset list composition order | `_build_toolset_list` appends: function toolset, override user toolsets (or stored user + dynamic + cap toolsets). Output tools route through a separate path. | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2675-2704` |
| `_AgentFunctionToolset` (internal subclass) | Overrides `id -> '<agent>'`, `label -> 'the agent'`, carries `output_schema` for per-tool `include_return_schema` resolution | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:3054-3076` |
| `RootCapability` collects tools contributed by capabilities | The capability tree is reduced; `self._root_capability.get_toolset()` is the entry point for capability-contributed toolsets (stored in `_cap_toolsets`) | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:512, 537-538` |
| `ToolManager` consumes the assembled toolset | `ToolManager(toolset, ...)` is the per-step executor; `tools` is cached, `failed_tools` is tracked, `default_max_retries=1`; resolves arguments via `args_validator` then `args_validator_func` then dispatches to the tool's `toolset.call_tool` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:77-100, 130-250, 300-500` (selection) |
| Output toolset (auto-built from `output_type`) | `OutputSchema.build(outputs)` constructs output tool `ToolDefinition`s with `kind='output'`; `OutputToolset.get_tools` returns `ToolsetTool(tool_def=tool_def, max_retries=..., args_validator=processors[name].validator)` | `pydantic_ai_slim/pydantic_ai/_output.py:1402-1529` |
| Tool-result validation hook (`ArgsValidatorFunc`) | Callable receiving schema-validated typed parameters with `RunContext` first; should raise `ModelRetry` on failure | `pydantic_ai_slim/pydantic_ai/tools.py:84-94, 437-450` |
| ToolSelector for capability filtering | `Literal['all'] \| Sequence[str] \| dict[str, Any] \| Callable[[RunContext, ToolDefinition], bool \| Awaitable[bool]]`; `_metadata_includes` enables deep-dict match; `matches_tool_selector` is async-aware | `pydantic_ai_slim/pydantic_ai/tools.py:156-228` |
| Tool search schema | `_DEFAULT_PARAMETER_DESCRIPTION` and `_DEFAULT_TOOL_DESCRIPTION` keep the corpus description documented in code | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:121-134` |
| Legacy discovery metadata (backward compat) | `_LegacyDiscoveryMetadata = TypedDict('discovered_tools', list[str])`, validated via `TypeAdapter` when reading | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:69-81` |
| Parser for previously-discovered tool names | `parse_discovered_tools(messages)` walks `ModelRequest`/`ModelResponse` parts for `ToolSearchReturnPart` / `NativeToolSearchReturnPart` and legacy metadata; used to populate `ctx.discovered_tool_names` for `ToolSearchToolset` | `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:185-216` |
| Common-tools library (`web_fetch_tool(...)`) | `def web_fetch_tool(...) -> Tool[Any]` factory; wraps `WebFetchLocalTool.__call__` and binds name `'web_fetch'`; same pattern for `duckduckgo_search_tool`, `tavily_search_tool`, `exa_search_tool`, `image_generation_tool` | `pydantic_ai_slim/pydantic_ai/common_tools/web_fetch.py:147-187`, `pydantic_ai_slim/pydantic_ai/common_tools/duckduckgo.py`, `pydantic_ai_slim/pydantic_ai/common_tools/tavily.py`, `pydantic_ai_slim/pydantic_ai/common_tools/exa.py`, `pydantic_ai_slim/pydantic_ai/common_tools/image_generation.py` |
| Documentation of composition | `docs/toolsets.md:59-115, 245-345, 367-454, 454-525, 515-560, 559-590, 590-647, 647-797, 797-859, 859-880` (`Function Toolset`, `Combining Toolsets`, `Filtering Tools`, `Prefixing Tool Names`, `Renaming Tools`, `Dynamic Tool Definitions`, `Requiring Tool Approval`, `Deferred Loading`, `Including Return Schemas`, `Setting Tool Metadata`, `Changing Tool Execution`, `External Toolset`, `Dynamically Building a Toolset`) |
| Documentation of decorator-based registration | `docs/tools.md:25-251` (`Registering via Decorator`, `Registering via Agent Argument`) |
| Forward-reference / signature-introspection tests | `tests/test_tools.py:45-115` (`test_tool_no_ctx`, `test_tool_plain_with_ctx`, `test_builtin_tool_registration`, `test_tool_ctx_second`) | `pydantic_ai/tests/test_tools.py:45-115` |
| Docstring-format inference tests | `tests/test_tools.py:143-432` (`test_docstring_google`, `test_docstring_sphinx`, `test_docstring_numpy`) | `pydantic_ai/tests/test_tools.py:143-432` |
| Tool conflict tests | `test_repeat_tool_by_rename` (`:692-743`), `test_repeat_tool` (`:719-760`), `test_tool_return_conflict` (`:743-779`), `test_tool_name_conflict_hint` (`:760-779`) | `pydantic_ai/tests/test_tools.py:692-779` |
| Comprehensive toolset composition test | `test_comprehensive_toolset_composition` constructs three `FunctionToolset`s, wraps each in `PrefixedToolset`, combines with `CombinedToolset`, wraps in `FilteredToolset`, wraps in `PreparedToolset`, asserts on tool definitions under different `deps` | `pydantic_ai/tests/test_toolsets.py:389-660` |
| Toolset context manager tests | `test_context_manager`, `test_context_manager_failed_initialization` | `pydantic_ai/tests/test_toolsets.py:661-705` |
| Retry budget tests across tool / toolset / agent | `test_toolset_max_retries_inherits_from_agent`, `test_toolset_explicit_max_retries_overrides_agent`, `test_tool_explicit_retries_overrides_toolset_and_agent` | `pydantic_ai/tests/test_toolsets.py:861-1010` |
| Function toolset default inheritance | `test_function_toolset_with_defaults`, `test_function_toolset_with_defaults_overridden`, `test_function_toolset_with_toolset_level_overrides` | `pydantic_ai/tests/test_toolsets.py:227-328` |
| Instruction function tests for `FunctionToolset` | `test_function_toolset_get_instructions_string/function/with_ctx/async/multiple/none_default` | `pydantic_ai/tests/test_toolsets.py:1590-1700` |
| Tool search corpus tests | `tests/test_tool_search.py` (5,500 lines; corpus, BM25, keywords, capability-revealed, cross-provider round-trip) | `pydantic_ai/tests/test_tool_search.py` |
| MCP toolset tests | `tests/test_mcp_toolset.py` covers cache invalidation, dynamic discovery, sampling, tasks, error behavior | `pydantic_ai/tests/test_mcp_toolset.py` |
| Capability tests | `tests/test_capabilities.py` covers tool-contribution and defer-loading | `pydantic_ai/tests/test_capabilities.py` |

## Answers to Dimension Questions

1. **How does a new tool enter the system?**

   A new tool enters through one of these well-defined surfaces:

   - **Decorator on an agent (context-taking)**: `@agent.tool` wraps the function and forwards to `self._function_toolset.add_function(..., takes_ctx=True, ...)`, returning the original function so the decorator is transparent (`agent/__init__.py:2210-2315`). Schema is auto-built from the function signature via `function_schema` and stored on the `Tool` (`tools.py:559-566`).
   - **Decorator on an agent (no context)**: `@agent.tool_plain` (`:2317-2449`) similarly forwards to `add_function(..., takes_ctx=False, ...)`.
   - **Decorator on a toolset**: `@toolset.tool` / `@toolset.tool_plain` add to the toolset directly (`toolsets/function.py:145-442`).
   - **Static list on a toolset**: `FunctionToolset(tools=[tool_a, tool_b])` (`:61-139`).
   - **Imperative API on a toolset**: `toolset.add_function(func, ...)` (`:475-579`) and `toolset.add_tool(tool)` (`:581-593`); both run on the same dict-keyed registry (`tools: dict[str, Tool[Any]]`).
   - **Static list on an agent**: `Agent(..., tools=[...])` populates `_function_toolset` (`agent/__init__.py:520-525`).
   - **Toolsets on an agent**: `Agent(..., toolsets=[ts_a, ts_b, ...])` splits each entry into `_user_toolsets` (instances) and `_dynamic_toolsets` (factory functions wrapped as `DynamicToolset`); cap-contributed toolsets land in `_cap_toolsets` (`:527-538`).
   - **Factory toolset decorator**: `@agent.toolset` registers a `DynamicToolset` evaluated each run (`:2463-2499`, `toolsets/_dynamic.py:20-157`).
   - **MCP server**: `MCPToolset(...)` or deprecated `MCPServerStdio(...)` is passed to `toolsets=[...]`; tools are dynamically discovered via `await self.list_tools()` (`mcp.py:2427-2441, 2442-2464`).
   - **External toolset**: `ExternalToolset([ToolDefinition(...)])` registers declared tools whose execution is deferred to the caller (`toolsets/external.py:15-52`).
   - **Output toolset**: `OutputToolset.build(outputs)` builds output tool definitions from `output_type` (`_output.py:1402-1529`); registered automatically as `_output_toolset` by the agent (`:516-518`).
   - **Capability-contributed**: any capability may return an `AgentToolset` from `get_toolset()` (`capabilities/abstract.py:331-333`), which the agent collects into `_cap_toolsets` (`:537-538`).
   - **Wrapper toolset (composes existing)**: `toolset.filtered(...)`, `.prefixed(...)`, `.renamed(...)`, `.prepared(...)`, `.approval_required(...)`, `.defer_loading(...)`, `.include_return_schemas(...)`, `.with_metadata(...)` (`toolsets/abstract.py:182-279`) compose behavior without modifying the underlying toolset.
   - **Native tool**: subclass `AbstractNativeTool` and the registry is auto-populated via `__init_subclass__` (`native_tools/__init__.py:85-87`); registration with the agent happens via `Agent(..., native_tools=[...])` (which is then persisted in `_cap_native_tools` (`:512`)).
   - **Common library**: factory functions like `web_fetch_tool(...)` (`common_tools/web_fetch.py:147-187`) return a configured `Tool[Any]` with bound name and description.
   - **Per-run/per-call override**: `agent.override(tools=..., toolsets=...)` and `agent.iter(..., toolsets=...)` / `agent.run(..., toolsets=...)` use `ContextVar` to override the static set without touching the agent object (`agent/__init__.py:544-558, 1881-1943`).

2. **Is tool registration explicit?**

   Yes, at every layer:

   - Registering a tool means calling `add_function` / `add_tool` on a toolset — there is no implicit scan of imported modules or attached decorators at agent-run time.
   - `Tool` instances list their identity fields (`name`, `description`, `function`, `prepare`, `args_validator`, ...) on `tools.py:441-466`; everything is keyword-only (`@dataclass(init=False)` and explicit `__init__`).
   - Conflict detection is explicit: `FunctionToolset.add_tool` raises `UserError(f'Tool name conflicts with existing tool: {tool.name!r}')` (`function.py:587-588`); `CombinedToolset` raises on cross-set collision with the source toolset label and `tool_name_conflict_hint` (`combined.py:73-77`).
   - `PreparedToolset.get_tools` raises `UserError` if the prepare callback adds or renames tools — preparation can only filter / modify (`prepared.py:33-36`).
   - `ToolSearchToolset` raises `UserError` if a user tool clashes with the reserved `_SEARCH_TOOLS_NAME` (`toolsets/_tool_search.py:298-301`).

3. **Can tools be discovered dynamically?**

   Yes, in three places:

   - **Factory toolsets** via `DynamicToolset` — `for_run` evaluates once (or `for_run_step` evaluates per step), so a toolset can re-evaluate its `toolset_func(ctx)` against the live `RunContext` and `RunContext.deps` / `RunContext.run_step` (`toolsets/_dynamic.py:61-96`).
   - **MCP servers** — `MCPToolset.get_tools` always calls `await self.list_tools()` (with optional cache via `cache_tools`), so providers that change tools and send `notifications/tools/list_changed` are picked up dynamically (`mcp.py:2427-2464`).
   - **Tool discovery over a corpus** — `ToolSearchToolset` hides tools via `defer_loading=True` and exposes a `search_tools` function tool; discovered tools become visible (`toolsets/_tool_search.py:284-345`). Discovery state is parsed from message history (`parse_discovered_tools`, `:185-216`).
   - **Capability-owned deferred loading** — `CapabilityOwnedToolset` flips `defer_loading=True` if the owning capability is deferred; tools become visible when the capability is loaded (`toolsets/_capability_owned.py:18-87`).

4. **Are names stable?**

   Yes:

   - A tool's name defaults to `function.__name__` (`tools.py:558`), and `Tool.from_schema` requires an explicit `name` argument (`:582-636`). Names are stored as a frozen field on `ToolDefinition` (`:687-740`) and used as the registration key (`function.py:587`, `combined.py:73-77`).
   - On the wire, `PrefixedToolset` and `RenamedToolset` rewrite the exposed name while preserving the original on `RunContext.tool_name` (`prefixed.py:36-40`, `renamed.py:36-41`).
   - `ToolSearchToolset` reserves `_SEARCH_TOOLS_NAME = TOOL_SEARCH_FUNCTION_TOOL_NAME` and refuses conflicts (`toolsets/_tool_search.py:63, 298-301`).
   - Native tools carry a stable `kind: str` discriminator; `AbstractNativeTool.unique_id` allows duplicate `kind` instances to be distinguished (default = `kind`) (`native_tools/__init__.py:55-75, 572-574`).
   - MCP tool names come from the server's advertised names and are normalized (no automatic renaming); collisions surface as a `UserError` (`mcp.py:2450, combined.py:73-77`).
   - All `AbstractToolset` instances carry an optional `id` used for durable execution environments (`toolsets/abstract.py:86-95`); the `_AgentFunctionToolset` has `id == '<agent>'` and `OutputToolset.id == '<output>'` (`agent/__init__.py:3071-3072`, `_output.py:1506-1511`).

5. **Can multiple agents see different tool sets?**

   Yes, and in multiple complementary ways:

   - **Per-agent toolset storage** — every `Agent` instance has its own `_function_toolset`, `_user_toolsets`, `_dynamic_toolsets`, `_cap_toolsets`, and `_output_toolset` (`agent/__init__.py:251-256, 520-538`). Two agents constructed independently see disjoint sets.
   - **Per-run override** — `agent.iter(..., toolsets=...)`, `agent.run(..., toolsets=...)`, and `agent.override(..., tools=..., toolsets=...)` layer additional or replacement tools per call without mutating the agent (`agent/__init__.py:1881-1943`, `agent/wrapper.py:298-352`).
   - **ContextVar scoping** — `_override_toolsets`, `_override_tools`, `_override_builtin_tools`, `_override_instructions`, `_override_metadata`, `_override_model`, `_override_model_settings`, `_override_output_retries`, `_override_root_capability` are all `ContextVar`s, so nested `agent.override` blocks compose correctly (`agent/__init__.py:544-570`).
   - **Per-step dynamic toolsets** — `DynamicToolset.per_run_step=True` re-evaluates each step with the live context (`toolsets/_dynamic.py:76-96`); the per-step `CombinedToolset` is also rebuilt for each step via `CombinedToolset.for_run_step` (`combined.py:48-52`).
   - **Tool-search visibility** — a tool hidden via `defer_loading=True` is only seen after the model discovers it; discovery state is per-run (parsed from message history + capability-loaded flags) (`toolsets/_tool_search.py:284-345, 185-216`).
   - **Capability-contributed deferred** — tools contributed by a `defer_loading=True` capability are invisible until the model calls `load_capability` (`capabilities/abstract.py:185-197`, `toolsets/_capability_owned.py:18-43`).

## Architectural Decisions

- **`AbstractToolset` is the only port the rest of the framework talks to.** All toolsets — function, MCP, external, output, dynamic, wrappers — share the same `get_tools`, `call_tool`, `for_run`, `for_run_step`, `__aenter__/__aexit__`, `apply`, `visit_and_replace` (`toolsets/abstract.py:74-279`). This keeps `ToolManager` (`tool_manager.py:77-500`) free of per-toolset knowledge.
- **`Tool` (callable) and `ToolDefinition` (wire shape) are decoupled.** A `Tool` owns the function, validator, retry budget, prepare hook, and metadata; a `ToolDefinition` is what the model sees, with `kind`, `outer_typed_dict_key`, `defer_loading`, `unless_native`, `with_native`, `tool_kind`, `capability_id`. `Tool.tool_def` builds the latter (`tools.py:638-652`). The separation lets tools be renamed, prefixed, prepared, filtered, and orchestrated without touching the callable.
- **Wrappers compose via `WrapperToolset`.** Cross-cutting behavior (`filtered`, `prefixed`, `prepared`, `renamed`, `approval_required`, `defer_loading`, `include_return_schemas`, `with_metadata`, `CapabilityOwnedToolset`, `ToolSearchToolset`, `DeferredCapabilityLoaderToolset`) is implemented as subclasses of `WrapperToolset` so it can wrap any existing toolset, including MCP servers and other wrappers (`toolsets/abstract.py:182-279`, `toolsets/wrapper.py:14-74`). `PreparedToolset` and `DeferredLoadingToolset` build on `PreparedToolset.prepare_func` rather than re-implementing it (`set_metadata.py:12-28`, `include_return_schemas.py:11-26`, `deferred_loading.py:11-37`).
- **Tool discovery is first-class via `DynamicToolset`.** Factory-evaluated toolsets slot into the same list as persistent toolsets (`agent/__init__.py:529-533, 2463-2499`). This gives both per-run factory semantics and per-step factory semantics via `per_run_step` (`toolsets/_dynamic.py:20-96`).
- **MCP is a toolset subclass.** The framework deliberately models MCP servers as toolsets so the same composition primitives (`filtered`, `prefixed`, `renamed`, `defer_loading`, etc.) apply to remote tools without special-casing (`mcp.py:1982-2540`, `docs/toolsets.md:881-895`). `MCPToolset` adds `cache_tools`, `cache_resources`, `cache_prompts`, `include_instructions`, `include_return_schema`, `process_tool_call`, `sampling_model`, `tool_error_behavior`, and a `metadata` (`meta`/`annotations`/`task`) bag (`mcp.py:2030-2099, 2442-2464`).
- **Tool search is both native and local.** `ToolSearchToolset` exposes a `search_tools` function tool with `unless_native='tool_search'` so it gets dropped by the adapter when the provider supports the framework-managed tool-search builtin (`toolsets/_tool_search.py:284-345`). Local fallback uses a built-in keyword-overlap algorithm (`keywords_search_fn`, `:98-118`) and accepts user-supplied `ToolSearchFunc` strategies (`:448-469`).
- **Capabilities can contribute tools.** `AbstractCapability.get_toolset()` is the single hook that returns a toolset to be added to the agent; `_root_capability.get_toolset()` is called at construction and the result stashed in `_cap_toolsets` (`capabilities/abstract.py:331-333`, `agent/__init__.py:537-538`). When `defer_loading=True`, capability-owned tools are marked `defer_loading=True` and only become visible once the capability is loaded (`toolsets/_capability_owned.py:18-43`).
- **Per-run override is layer-driven, not destructive.** The static toolset list is captured at construction; per-run overrides use `ContextVar`s + a fresh `_build_toolset_list` (`agent/__init__.py:544-558, 2591-2660, 2675-2704`). Tool calls that produce new tools via `DynamicToolset` reuse the same machinery and never mutate the agent.
- **Native tools are registry-driven, not class-based.** `NATIVE_TOOL_TYPES` is populated automatically by `AbstractNativeTool.__init_subclass__`; Pydantic builds a `Discriminator` over `kind` for round-trip serialization (`native_tools/__init__.py:33-103`).
- **Errors are surfaced through the same paths as regular results.** `try_call_user_function_in_toolset` (in `tool_manager.py`), `ModelRetry`, `ToolDenied`, `ApprovalRequired`, `SkipToolExecution`, `SkipToolValidation`, and `CallDeferred` all flow back to the model via `ToolReturn` / `RetryPromptPart` (`tools.py:256-421`, `messages.py:ToolPartKind`, `exceptions.py`).
- **Doc-style awareness with strict fallback.** `Tool.function_schema` inspects the docstring (`DocstringFormat = Literal['google', 'numpy', 'sphinx', 'auto']`) and falls back to `auto` (`tools.py:246-253, 467-557`). `require_parameter_descriptions=True` raises when a parameter is missing a description.
- **Durable execution toolsets have stable ids.** `ToolsetTool.toolset.id` is required by durable backends such as Temporal (`toolsets/abstract.py:86-95`), and `DynamicToolset` requires an `id` for the same reason (`toolsets/_dynamic.py:30-39`).

## Notable Patterns

- **Layered decorators on agents and toolsets**: `@agent.tool` / `@agent.tool_plain` / `@agent.toolset` (`agent/__init__.py:2210-2499`) and `@toolset.tool` / `@toolset.tool_plain` / `@toolset.instructions` (`toolsets/function.py:145-473`). Both return the original function so decoration is opaque.
- **Tool-set wrappers as cross-cutting layers**: `FilteredToolset`, `PrefixedToolset`, `RenamedToolset`, `PreparedToolset`, `ApprovalRequiredToolset`, `DeferredLoadingToolset`, `SetMetadataToolset`, `IncludeReturnSchemasToolset`, `CapabilityOwnedToolset`, `DeferredCapabilityLoaderToolset`, `ToolSearchToolset` all subclass `WrapperToolset` and override `get_tools` / `call_tool` minimally (`toolsets/wrapper.py:14-74`).
- **`ToolDefinition.defer_loading` with explicit duality**: a single bool conveys both user intent and live visibility — the docstring owns this trade-off as acknowledged tech debt (`tools.py:748-768`).
- **Source-of-truth `Tool(...)` for the tool-search signature**: `_search_tools_signature` is a pure module-level function; `Tool(_search_tools_signature).function_schema` produces the JSON schema and `SchemaValidator` used by `ToolSearchToolset` (`toolsets/_tool_search.py:137-182, 343-390`).
- **`CombinedToolset` as a parallel merge**: `_utils.gather(*(toolset.get_tools(ctx) for toolset in self.toolsets))` merges child outputs concurrently and raises `UserError` on collision with a source-label-aware message (`combined.py:66-88`).
- **Selective tool-set application**: `apply(visitor)` visits leaves only (e.g., to install an `mcp_sampling_model` on every `MCPServer`), while `visit_and_replace(visitor)` swaps leaves in place — the agent uses both to inject the sampling model in `set_mcp_sampling_model` (`agent/__init__.py:2753-2769`, `toolsets/abstract.py:182-191`, `combined.py:96-103`, `wrapper.py:68-74`).
- **Capability ID-stamped tools**: `CapabilityOwnedToolset` writes `tool_def.capability_id` from the capability's run-resolved id, allowing `RunContext.available_capability_ids` and `loaded_capability_ids` to gate visibility (`toolsets/_capability_owned.py:18-43, 57-87`).
- **`ToolSearchToolset` reserves a name and raises early**: `_SEARCH_TOOLS_NAME` is reserved; a conflicting user tool produces `UserError` before the toolset reaches the wire (`toolsets/_tool_search.py:63, 298-301`).
- **Native-tool schema is auto-constructed**: `AbstractNativeTool.__get_pydantic_core_schema__` reads `NATIVE_TOOL_TYPES` and returns an `Annotated[Union[...], Discriminator(_tool_discriminator)]` (`native_tools/__init__.py:89-103, 616-620`). Subclasses need only set `kind: str` to participate.
- **`Tool.from_schema` for hand-built definitions**: lets users provide a JSON schema directly, useful for testing, code-mode, and reusing declared-shape external APIs (`tools.py:582-636`).
- **Common-tool factories return `Tool[Any]`**: `web_fetch_tool`, `duckduckgo_search_tool`, `tavily_search_tool`, `exa_search_tool`, `image_generation_tool`, `x_search_tool` all expose factory functions that bind name/description and return a configured `Tool` (`common_tools/web_fetch.py:147-187` and siblings).
- **Cross-provider symbol aliases**: `BuiltinToolFunc` / `AgentBuiltinTool` aliases resolve to `NativeToolFunc` / `AgentNativeTool` with `PydanticAIDeprecationWarning` (`tools.py:902-916`); `ToolDefinition.prefer_builtin` / `prefer_native` alias to `unless_native` (`:883-899`). Backward compat is preserved at the registration surface.
- **Docstring-driven schema generation**: tests `test_docstring_google` / `test_docstring_sphinx` / `test_docstring_numpy` show that the schema generator adapts to all three formats (`tests/test_tools.py:143-432`).

## Tradeoffs

- **Declarative registration vs. discovery**: registration is fully explicit (no implicit scan), so a user must remember to register each tool. The benefit is auditable tool scopes; the cost is more boilerplate to set up a multi-tool agent. The framework offsets this with `@agent.tool` / `@agent.toolset` decorators, `DynamicToolset` factories, and `ToolSearchToolset` discovery (`agent/__init__.py:2210-2499`; `toolsets/_dynamic.py:20-96`; `toolsets/_tool_search.py:246-489`).
- **Wrappers everywhere, but each wrapper does one thing**: filtered / prefixed / renamed / prepared / deferred / approval-required are all separate classes. The cost is many small types; the benefit is composability — new cross-cutting behavior can be added by subclassing `WrapperToolset` (`toolsets/AGENTS.md:5-7`).
- **`ToolDefinition.defer_loading` carries dual meaning** (user-input vs. live visibility) — explicitly documented as tech debt (`tools.py:748-768`). A future `RunContext.loaded_tools` view is sketched but not implemented.
- **MCP tool schema is trusted**: `MCPToolset` uses `core_schema.any_schema()` as its `args_validator` and just forwards `mcp_tool.inputSchema` to the model (`mcp.py:2442-2464`). Faster and protocol-correct, but no second-pass schema validation in the framework.
- **Naming collisions are loud, not auto-resolved**: `FunctionToolset.add_tool` raises on duplicates (`toolsets/function.py:587-588`); `CombinedToolset` raises with a `PrefixedToolset` hint (`combined.py:73-77`). Users must explicitly rename (via `PrefixedToolset`, `RenamedToolset`) rather than rely on auto-suffixing — a deliberate design choice that keeps wire names predictable.
- **`DynamicToolset` evaluates per-step when `per_run_step=True`**: lifecycles must be managed by the toolset (exit old, enter new) — see `_enter_inner_toolset` and the careful ordering that ensures `__aexit__` never sees a toolset that wasn't `__aenter__`ed (`toolsets/_dynamic.py:90-110`).
- **Capability registration is constrained to one toolset per capability**: `AbstractCapability.get_toolset()` returns either a single toolset or `None`. Richer contribution requires `get_wrapper_toolset` (`capabilities/abstract.py:331-367`).
- **Output-tool registration is automatic**: `OutputToolset` is built from `Agent(output_type=...)` and stored as `_output_toolset`; it is layered separately from function tools (`agent/__init__.py:516-518, 2591-2660`). Output-toolset overrides are explicit, but a user can't normally inject additional output tools without subclassing `OutputSchema`.
- **`AbstractToolset.id` is required for durable execution**: `DynamicToolset`, `MCPServer`, `ExternalToolset`, `FunctionToolset` all accept an `id` (`toolsets/abstract.py:86-95`). Temporal-style backends rely on it.
- **Reserved name `search_tools`**: user-defined tools with that name collide with `ToolSearchToolset`'s built-in discovery function (`toolsets/_tool_search.py:298-301`).
- **Native tool kinds are not extensible at runtime**: tools must be defined as subclasses of `AbstractNativeTool` at import time to participate in `NATIVE_TOOL_TYPES` (`native_tools/__init__.py:85-87`). Plugin-style registration of native tools is not supported.

## Failure Modes / Edge Cases

- **Duplicate tool name in a single toolset**: `FunctionToolset.add_tool` raises `UserError(f'Tool name conflicts with existing tool: {tool.name!r}')` (`toolsets/function.py:587-588`).
- **Cross-set duplicate on assembly**: `CombinedToolset.get_tools` raises `UserError(f'{Label} defines a tool whose name conflicts with existing tool from {Label}: {name!r}. {toolset.tool_name_conflict_hint}')` (`toolsets/combined.py:73-77`).
- **`prepare_tool_def` returns `None`**: the tool is dropped silently from that step's tool list (`tools.py:654-671`); `PreparedToolset.get_tools` warns via `warn_on_prepare_callback_returned_none` when a per-set `prepare_func` returns `None` (`toolsets/prepared.py:29-32`).
- **`prepare_func` adds or renames tools**: `PreparedToolset.get_tools` raises `UserError('Prepare function cannot add or rename tools. Use FunctionToolset.add_function() or RenamedToolset instead.')` (`prepared.py:33-36`).
- **Reserved tool-search name collision**: `ToolSearchToolset.get_tools` raises `UserError(f"Tool name '{_SEARCH_TOOLS_NAME}' is reserved for tool search. Rename your tool to avoid conflicts.")` (`toolsets/_tool_search.py:298-301`).
- **Function-tool schema validation fails**: surfaced as a `UserError` via `function_schema` during `FunctionToolset.add_function` (`tools.py:559-566`); tests confirm `Error generating schema for ...: First parameter of tools that take context must be annotated with RunContext[...]` (`tests/test_tools.py:45-115`).
- **Built-in function passed to `@agent.tool_plain`**: `UserError(no signature found for builtin <built-in function min>)` (`tests/test_tools.py:79-97`).
- **MCP tool-call error (default `tool_error_behavior='retry'`)**: framework raises `ModelRetry(message=str(e))` so the model self-corrects (`mcp.py:2507-2509`); switch to `'error'` to propagate the underlying `ToolError`.
- **MCP cache invalidation**: `cache_tools` clears on the final `__aexit__` and on `notifications/tools/list_changed`; the user-supplied `Client` path skips the notification handler (`mcp.py:2044-2054, 2427-2441`).
- **MCP tool with `execution.taskSupport='required'` called via local fallback**: the call is upgraded to a SEP-1686 task flow with `use_task=True`; only enabled when the metadata reports `task` truthy (`mcp.py:2446-2456, 2533-2538`).
- **External toolset `call_tool`**: raises `NotImplementedError('External tools cannot be called directly')` — the framework defers execution outside the agent loop (`toolsets/external.py:44-47`).
- **`DynamicToolset` factory raises during `__aenter__`**: the older inner toolset is exited first, ensuring `__aexit__` never sees a toolset it didn't enter (`toolsets/_dynamic.py:88-95`).
- **Concurrency on combined toolsets**: `_utils.gather` concurrent child evaluation can raise (`combined.py:67`). Errors propagate, and the `AsyncExitStack` cleanup closes whatever was opened (`combined.py:54-64`).
- **Retry budget conflicts**: `Retries={'tools': ...}` at run time raises `UserError` — tool retries must be set at agent construction (`agent/__init__.py:1178-1186`).
- **Output-toolset retry budget**: `OutputToolset.max_retries` is set by the agent at construction; per-tool overrides go through `_max_retries_overrides` (`_output.py:1409-1524`).
- **`DeferredToolset` is a deprecated alias** for `ExternalToolset` and emits `DeprecationWarning` on import (`toolsets/external.py:50-52`).
- **`prefer_builtin` / `prefer_native` aliases** are read-access deprecated with `PydanticAIDeprecationWarning` (`tools.py:883-899`); kwarg-level aliases installed via `_utils.install_deprecated_kwarg_alias` (`:898-899`).
- **`UrlContextTool` (deprecated)** maps to `WebFetchTool` via subclassing; `kind` overridden to `'url_context'` for backward-compat deserialization (`native_tools/__init__.py:371-382`).

## Future Considerations

- **`RunContext.loaded_tools` view** is described as future work to disentangle user-input defer_loading from live visibility state (`tools.py:748-768`).
- **Pluggable native-tool registry**: today subclasses must be defined at import time so `__init_subclass__` populates `NATIVE_TOOL_TYPES` (`native_tools/__init__.py:85-87`); runtime plugin registration is not supported.
- **Multiple toolsets per capability**: `AbstractCapability.get_toolset()` returns a single `AgentToolset` or `None`; richer contribution uses `get_wrapper_toolset` (`capabilities/abstract.py:331-367`).
- **Cross-agent tool reuse**: since each `Agent` holds its own toolset list and `FunctionToolset` dict, sharing a toolset across agents is by passing the same instance (`toolsets/function.py:587-593`). A first-class "shared toolset catalog" could let observability tooling enumerate all installed tools.
- **Tool versioning**: there is no version field on `Tool` / `ToolDefinition`. Deprecation today means editing every agent's toolset list.
- **MCP `notifications/tools/list_changed` subscription at agent-level**: `MCPToolset` invalidates its cache on the notification, but the agent's prepared tool list is rebuilt only via `CombinedToolset.for_run_step`. A push-based subscription would narrow the rebuild window.
- **`DeferredCapabilityLoaderToolset`** (`toolsets/_deferred_capability_loader.py:31+`) extends the capability-defer model — currently a single hook; future work may unify it with `ToolSearchToolset` for a shared corpus.
- **Static analysis of `@agent.tool` / `@agent.toolset` decorated members**: the framework relies on runtime introspection; a class-level scan could produce stable tool catalogs at import time.

## Questions / Gaps

- Is there a recommended way to declare a tool whose implementation is provided by a different process / service other than via `ExternalToolset`? `MCPToolset` plus `sampling_model` and `process_tool_call` covers most cases, but a generic "deferred-but-not-MCP" toolset is `ExternalToolset` only.
- Where, if anywhere, is there a public API to add a tool to a toolset mid-run from inside another tool? `FunctionToolset.add_function` is callable at any time (`toolsets/function.py:475-579`), but `ToolManager` does not refresh its cache automatically until the next step.
- How stable is the native-tool `kind` string? It is the Pydantic discriminator (`native_tools/__init__.py:616-620`); renaming a `kind` would break persisted histories. Not documented as a versioning concern.
- How is a multi-tenant shared toolset recovered across runs? `DynamicToolset(for_run_step=True)` re-evaluates each step but does not cache — the per-step factory cost may be non-trivial. No evidence of memoization in the searched files.
- What happens if both `Agent(tools=[...])` and `Agent(toolsets=[FunctionToolset(tools=[my_func])])` are passed and `my_func` exists in both? `FunctionToolset.add_tool` would raise on the second registration in the second toolset (`toolsets/function.py:587-588`); across two distinct toolsets, `CombinedToolset` would raise (`toolsets/combined.py:73-77`). No auto-dedup at construction.
- Are `CapabilityOwnedToolset` and `DeferredCapabilityLoaderToolset` distinct? Both wrap a toolset to gate behavior at the capability boundary; `CapabilityOwnedToolset` stamps `capability_id` and metadata; `DeferredCapabilityLoaderToolset` (not deeply explored) appears to drive the `load_capability` lifecycle. The exact division of labor is best confirmed by reading `toolsets/_deferred_capability_loader.py` end-to-end.

---

Generated by `04.01-tool-definition-and-registration.md` against `pydantic-ai`.
