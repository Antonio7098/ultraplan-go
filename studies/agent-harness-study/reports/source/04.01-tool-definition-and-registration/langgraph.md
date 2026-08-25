# Source Analysis: langgraph

## Tool Definition and Registration

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (>=3.10), Pydantic v1/v2, LangChain Core, asyncio |
| Analyzed | 2026-07-19 |

## Summary

LangGraph does not maintain a global tool registry. Tool registration is **explicit, instance-scoped, and static**: a tool enters the system by being passed to a `ToolNode` (or to `create_react_agent`, which builds one internally) and is then indexed by name into a private `dict[str, BaseTool]` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`). Names are inherited from LangChain Core's `BaseTool` / `@tool` decorator — there is no LangGraph-specific namespace prefix or schema version stamped on registered tools.

The `ToolNode` class (`libs/prebuilt/langgraph/prebuilt/tool_node.py:622`) extends `RunnableCallable` from `langgraph._internal._runnable` (`libs/langgraph/langgraph/_internal/_runnable.py:278`), so a registered tool set is wired into a `StateGraph` by adding the `ToolNode` itself as a node (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:872`; `libs/prebuilt/langgraph/prebuilt/tool_node.py:1627`). The same `ToolNode` can host many tools, but each tool must be declared up front; the only dynamic extension surface is the `wrap_tool_call` / `awrap_tool_call` interceptor hooks (`libs/prebuilt/langgraph/prebuilt/tool_node.py:755-756, 1044-1067, 1191-1222`) which let a registered interceptor swap in an *unregistered* tool at call time (`libs/prebuilt/tests/test_tool_node.py:1913-1999`).

Injection metadata (`InjectedState`, `InjectedStore`, `ToolRuntime`, custom `InjectedToolArg` subclasses) is discovered from function/parameter annotations via reflection (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1967-2030`), and registered as a per-tool `_InjectedArgs` record at `ToolNode` init (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`). This is fragile — the registration-time pass is the only place where injection metadata is frozen — but it is durable: the LLM cannot forge injection slots, because stripped args are re-added from trusted values (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1422-1429`).

## Rating

**7/10**

Rationale:

- **Strengths**: explicit, instance-scoped registration (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`); typed injection annotations with documented security stripping (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1422-1429`, `libs/prebuilt/tests/test_tool_node.py:2146-2238`); pluggable interceptor hooks (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1044-1067`); first-class streaming via `StreamToolCallHandler` + `ToolCallTransformer` (`libs/langgraph/langgraph/pregel/_tools.py:35-268`, `libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:44-165`); per-call `ToolRuntime` dataclass that surfaces every registered tool back into the body (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1663-1750`); `create_react_agent` factory path validates tool↔bound-tool consistency (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217`); `validation_error` filtering strips injected args from LLM-visible error messages (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563`).
- **Limitations**: registration is *not* dynamic — there is no public `add_tool`/`register_tool` on `ToolNode` after `__init__`, so dynamic agent-assembled tool sets require the `wrap_tool_call` workaround documented in `test_tool_node_inject_runtime_dynamic_tool_via_wrap_tool_call` (`libs/prebuilt/tests/test_tool_node.py:1913-1999`); tool naming is fully delegated to LangChain Core's `BaseTool.name` with no LangGraph-level namespace (no `mcp__server__tool` or similar); there is no `ToolNode.tools` mutation API, no audit log of registration events, and no `__all__` for tool re-export from the package root beyond the prebuilt top-level (`libs/prebuilt/langgraph/prebuilt/__init__.py:14-23`); `create_react_agent` and `ValidationNode` are marked deprecated and the canonical entry point has moved to `langchain.agents.create_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`, `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46`), so the registration pattern is in a transitional state; `ToolNode._injected_args` is computed once at init (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`), so dynamic tools added via `wrap_tool_call` have to be re-analyzed on each call (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361`).

## Evidence Collected

Every entry cites `path/to/file.py:NN` from inside the selected source.

| Area | Evidence | File:Line |
|------|----------|-----------|
| `ToolNode` — primary tool-bearing node class | `class ToolNode(RunnableCallable)` with default `name: str = "tools"` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:622, 741` |
| `ToolNode.__init__` — explicit registration loop | Builds `_tools_by_name` dict and precomputes `_InjectedArgs` for every tool in one pass | `libs/prebuilt/langgraph/prebuilt/tool_node.py:743-786` |
| Tool name → `BaseTool` lookup table | `self._tools_by_name: dict[str, BaseTool] = {}` populated by `self._tools_by_name[tool_.name] = tool_` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:773-784` |
| Public `tools_by_name` accessor | `@property tools_by_name -> dict[str, BaseTool]` (read-only view) | `libs/prebuilt/langgraph/prebuilt/tool_node.py:788-791` |
| Tool→BaseTool coercion for plain callables | `if not isinstance(tool, BaseTool): tool_ = create_tool(...)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:779-783` |
| Call-time tool resolution | `tool = self.tools_by_name.get(call["name"])` (deferred to allow interceptor to swap tools) | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1032, 1179` |
| Unknown-tool error message template | `INVALID_TOOL_NAME_ERROR_TEMPLATE = "Error: {requested_tool} is not a valid tool, try one of [{available_tools}]."` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110` |
| Unknown tool → error `ToolMessage` | `_validate_tool_call` returns a `ToolMessage` with the registered names listed | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279` |
| `InjectedState` annotation class | Subclass of LangChain Core's `InjectedToolArg` with optional `field: str \| None` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1753-1826` |
| `InjectedStore` annotation class | Marker subclass for store injection | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1829-1901` |
| `ToolRuntime` dataclass — registered to all tools | Bundles `state`, `tool_call_id`, `config`, `context`, `store`, `stream_writer`, `tools`, `execution_info`, `server_info`; includes `emit_output_delta` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1662-1750` |
| Tool functions exposed via `runtime.tools` | `tools=list(self.tools_by_name.values())` passed into every `ToolRuntime` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:813, 848` |
| Injection-arg discovery (reflection-based) | `_get_all_injected_args(tool)` inspects both schema and function signature in a single pass | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1967-2030` |
| `_is_injection` / `_get_injection_from_type` utilities | Unwrap `Union`/`Annotated` to detect `InjectedState`/`InjectedStore`/`ToolRuntime` markers | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1904-1964` |
| `_InjectedArgs` dataclass — per-tool frozen record | Captures state, store, runtime arg names plus the union of all injected keys for stripping | `libs/prebuilt/langgraph/prebuilt/tool_node.py:566-619` |
| Security: LLM cannot supply injected values | `_inject_tool_args` strips caller-supplied args for any key in `all_injected_keys` and re-adds trusted values | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1422-1429` |
| Error filtering — exclude injected-arg errors | `_filter_validation_errors` removes injected arg keys before sending errors back to the LLM | `libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563` |
| `ToolCallRequest` interceptor payload | Dataclass with `tool_call`, `tool`, `state`, `runtime`; `tool` is `None` for unregistered names to allow interceptors to short-circuit | `libs/prebuilt/langgraph/prebuilt/tool_node.py:132-205` |
| `wrap_tool_call` / `awrap_tool_call` interceptor hooks | Callable `ToolCallWrapper` injected into `_run_one`/`_arun_one`; falls back from async to sync wrapper | `libs/prebuilt/langgraph/prebuilt/tool_node.py:202-283, 1044-1067, 1191-1222` |
| Dynamic tool resolution via interceptor | `_inject_tool_args` re-runs `_get_all_injected_args` for tools supplied via `wrap_tool_call` since they bypass init-time registration | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361` |
| Test: dynamic tools supplied through interceptor work | `test_tool_node_inject_runtime_dynamic_tool_via_wrap_tool_call*` asserts dynamic tool is not in `tools_by_name` but still executes with runtime injection | `libs/prebuilt/tests/test_tool_node.py:1913-1999` |
| Test: InjectedToolArg stripping | `test_tool_node_strips_plain_injected_tool_arg` and `test_tool_node_strips_custom_injected_tool_arg_subclass` | `libs/prebuilt/tests/test_tool_node.py:2149-2206` |
| Test: InjectedState overrides LLM-supplied values | `test_tool_node_injected_state_overwrites_llm_value` | `libs/prebuilt/tests/test_tool_node.py:2208-2238` |
| `create_react_agent` — factory path that builds the `ToolNode` and binds tools to the model | Conditionally constructs `ToolNode([t for t in tools if not isinstance(t, dict)])` and calls `model.bind_tools(tool_classes + llm_builtin_tools)` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-588` |
| `create_react_agent` — tool/bound-tool consistency check | `_should_bind_tools` validates that the set of tool names matches the model-side `bind_tools` set, raises `ValueError` on mismatch | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217` |
| `create_react_agent` — pre-built `ToolNode` accepted | `if isinstance(tools, ToolNode): tool_classes = list(tools.tools_by_name.values()); tool_node = tools` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:555-561` |
| `tools_condition` — routing helper tied to a registered `tools` node | Returns `"tools"` when last AIMessage has `tool_calls`, else `__end__` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659` |
| `ValidationNode` (deprecated) — schema-only registration | `schemas_by_name: dict[str, type[BaseModel]]` populated from `BaseTool`/`BaseModel`/callable input | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-166` |
| `ToolCallTransformer` — stream-side projection of tool events | `StreamTransformer` subclass; native projection on `run.tool_calls` keyed by `tool_call_id`; pass-through for `tools` channel consumers | `libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:44-165` |
| `StreamToolCallHandler` — emits `tools` stream events from LangChain callbacks | `BaseCallbackHandler`; attaches a per-`run_id` writer to the `_tool_call_writer` ContextVar so tool bodies can stream partial output | `libs/langgraph/langgraph/pregel/_tools.py:35-268` |
| `StreamToolCallHandler` registration (Pregel) | Attached by `Pregel.stream`/`astream` when `"tools"` in `stream_modes` | `libs/langgraph/langgraph/pregel/main.py:2844-2853, 3272-3279` |
| Pregel `stream_transformers` — compile-time slot | `stream_transformers: Sequence[Callable[[tuple[str, ...]], Any]] \| None = None` | `libs/langgraph/langgraph/graph/state.py:1174, 1207-1213, 1356` |
| `StreamMux` registration — appends and de-conflicts transformer projections | `_register` rejects duplicate projection keys; `_on_register` hook for cross-transformer wiring | `libs/langgraph/langgraph/stream/_mux.py:227-267` |
| `add_node` accepts a `ToolNode` instance | `builder.add_node("tools", ToolNode(tools))` is the standard wiring pattern; node name comes from `action.get_name()` when no string is supplied | `libs/langgraph/langgraph/graph/state.py:768-790` |
| `ToolNode` *is* a `RunnableCallable`, so it participates in graph wiring as a single node | `super().__init__(self._func, self._afunc, name=name, tags=tags, trace=False)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:772` |
| `RunnableCallable` parent | `class RunnableCallable(Runnable)`; injects `config`/`store`/`runtime`/`writer` based on signature inspection | `libs/langgraph/langgraph/_internal/_runnable.py:278-343` |
| `langgraph.prebuilt` public surface | Re-exports `ToolNode`, `InjectedState`, `InjectedStore`, `ToolRuntime`, `ToolCallTransformer`, `ValidationNode`, `create_react_agent`, `tools_condition` | `libs/prebuilt/langgraph/prebuilt/__init__.py:1-23` |
| `Runtime` dataclass — sibling to `ToolRuntime` for nodes | Distinct from `ToolRuntime`: lacks `state`, `tool_call_id`, `tools` | `libs/langgraph/langgraph/runtime.py:124-196` |

## Answers to Dimension Questions

### 1. How does a new tool enter the system?

A new tool enters by being instantiated as a `langchain_core.tools.BaseTool` (typically via the `@tool` decorator, `create_tool`, or a hand-rolled `BaseTool` subclass) and then passed positionally to `ToolNode.__init__` as part of the `tools: Sequence[BaseTool | Callable]` argument (`libs/prebuilt/langgraph/prebuilt/tool_node.py:743-786`). Plain callables are coerced into `BaseTool` via `langchain_core.tools.tool` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:779-783`). The `ToolNode` is then added as a node to a `StateGraph` (`libs/langgraph/langgraph/graph/state.py:768-790`). The factory path `create_react_agent(model, tools)` constructs the `ToolNode` internally and binds the resulting tools to the model (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-588`).

### 2. Is tool registration explicit?

Yes — registration is explicit, instance-scoped, and constructor-bound. There is no global LangGraph tool registry, no module-level `register()` call, and no `@tool` decorator defined in LangGraph itself (`@tool` is re-exported from `langchain_core`). `ToolNode` is the unit of registration: the dict `self._tools_by_name` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773, 784`) is the authoritative set of tools the node can execute.

### 3. Can tools be discovered dynamically?

Discovery beyond registration-time introspection is **not first-class** but is possible through one of two channels:

1. **Dynamic tools via interceptor** — `wrap_tool_call` / `awrap_tool_call` can override `request.tool` with a `BaseTool` that was never registered in `ToolNode._tools_by_name`. The interceptor is allowed to return early (without calling `execute`) for unregistered tool names (`libs/prebuilt/langgraph/prebuilt/tool_node.py:944-950`, `libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:48-225`); if it does call `execute`, validation against `_validate_tool_call` still runs (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`).
2. **Pregel stream mode discovery** — the `tools` stream channel is opt-in per run (`libs/langgraph/langgraph/pregel/main.py:2844-2853`); `StreamToolCallHandler` decorates events but does not affect registration.

There is **no** public `add_tool`/`unregister_tool`/`set_tools` API on `ToolNode` after construction.

### 4. Are names stable?

Names are stable in the sense that they are inherited from `BaseTool.name` (which the LangChain Core `@tool` decorator derives from the function name) and used as the dict key. However, LangGraph performs **no** name normalization, namespacing, prefixing, or uniquification. Two registered tools with the same `name` silently collide: the second overwrites the first in `self._tools_by_name[tool_.name] = tool_` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:784`). This is an undocumented failure mode — `ToolNode.__init__` does not assert uniqueness.

`ToolNode` itself carries a stable `name: str = "tools"` used as the graph node key (`libs/prebuilt/langgraph/prebuilt/tool_node.py:741`), which means the *node* identity in the graph is the registered "tools" name unless overridden in the constructor.

### 5. Can multiple agents see different tool sets?

Yes — each `ToolNode` is its own registry (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773`). Subgraphs can mount their own `ToolNode` with a different tool list (`libs/langgraph/langgraph/tests/test_tool_stream_handler.py:263-275`), and the prebuilt `StreamToolCallHandler` reports the containing subgraph's namespace in every event so the runtime can disambiguate (`libs/langgraph/langgraph/pregel/_tools.py:87-119`). There is no inheritance or sharing mechanism: each `ToolNode` must be passed tools explicitly. `create_react_agent` accepts either a pre-built `ToolNode` (preserving its existing set) or a list (creating a new `ToolNode`) (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:555-561`).

## Architectural Decisions

- **Tool definition lives in `langchain_core`, not LangGraph.** The `@tool` decorator, `BaseTool`, `InjectedToolArg`, and `InjectedToolCallId` all come from `langchain_core.tools`. LangGraph supplies *only* the injection metadata classes (`InjectedState`, `InjectedStore`, `ToolRuntime`) and the execution node (`ToolNode`). This keeps the schema definition single-sourced and avoids duplicating Pydantic v1/v2 compatibility handling.
- **`ToolNode` is one node, many tools.** All registered tools share a single Pregel node, single error-handling policy, and single execution context. This optimizes for the common case of an LLM with a small handful of tools, at the cost of granularity (you cannot give one tool a different `handle_tool_errors` policy than another).
- **Injection is computed at init time, not lazily.** `_get_all_injected_args` walks the function signature once per tool (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1967-2030`); the result is cached in `self._injected_args[tool_.name]` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:786`). Tradeoff: fast lookups during execution; dynamic tools must recompute on each call (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361`).
- **Deferred validation.** The `ToolNode` only resolves a tool by name when a `tool_call` arrives (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1032, 1179`), allowing interceptors to substitute unregistered tools before validation runs (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1030-1067`).
- **Interceptors are the extension point.** `wrap_tool_call` / `awrap_tool_call` is the single hook for caching, retries, request rewriting, short-circuiting, and dynamic tool injection (`libs/prebuilt/langgraph/prebuilt/tool_node.py:202-283, 1044-1067, 1191-1222`). The `ToolCallRequest` dataclass carries `state` and `runtime` so interceptors can be context-aware without re-discovering them.
- **`tools_condition` couples routing to the literal node name `"tools"`.** Routing assumes the tool execution node is named `"tools"` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659`). This convention is also baked into `create_react_agent`'s `route_tool_responses` and `should_continue` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-987`).
- **Stream mode is opt-in.** `tools` is not a default stream mode; consumers must pass `stream_mode="tools"` or include it in a list (`libs/langgraph/langgraph/pregel/main.py:2844-2853`). The handler attaches via LangChain Core's `inheritable_handlers` so it follows the run into subgraphs unless explicitly suppressed with `subgraphs=False`.
- **Tool-call events are a separate `StreamTransformer` on the runtime side.** `ToolCallTransformer` (`libs/prebuilt/langgraph/prebuilt/_tool_call_transformer.py:44-165`) is *not* a default built-in — consumers opt in via `compile(transformers=[ToolCallTransformer])` (id. at line 60-64). This keeps the protocol decoupled from the projection layer.

## Notable Patterns

- **Single-pass annotation introspection.** `_get_all_injected_args` merges schema annotations and function-signature hints (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1979-1988`), then walks each parameter once. This is faster than the previous multi-pass approach and is the reason dynamic tools pay a one-call penalty.
- **Immutable override pattern for interceptor requests.** `ToolCallRequest.override(**kwargs)` returns a new request via `dataclasses.replace` and warns on direct attribute assignment (`libs/prebuilt/langgraph/prebuilt/tool_node.py:151-200`). This makes interceptor chaining composable.
- **Strip-then-reinject security pattern.** `_inject_tool_args` first drops any caller-supplied values for keys in `all_injected_keys`, then merges trusted values back in (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1422-1429`). The same `all_injected_keys` set is computed from `InjectedToolArg` annotations — including custom subclasses — at registration time.
- **Edge-aware routing helper.** `tools_condition` inspects the last `AIMessage` and routes to the literal node name `"tools"` or `END` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659`). This is the only routing helper that is namespacely aware of the tool node convention.
- **Send-distributed execution.** `create_react_agent` v2 emits one `Send("tools", ToolCallWithContext(tool_call=call, state=state))` per call (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`), letting the scheduler fan out parallel tool invocations across multiple `ToolNode` instances while still routing them through the same node identifier.
- **Optional state args.** `InjectedState` can be marked optional in the schema; missing keys are tolerated (`libs/prebuilt/langgraph/prebuilt/tool_node.py:2011-2013, 1396-1397, 1404-1405`).

## Tradeoffs

- **Static registration ⇒ static tool set per node.** Tools added after `ToolNode.__init__` are not visible to `tools_by_name`. The only mitigation is the `wrap_tool_call` escape hatch, which requires writing a custom interceptor. This is awkward for agents that need to mutate their tool list between invocations (e.g., MCP-style dynamic discovery).
- **No name uniqueness check.** `ToolNode.__init__` will silently overwrite a duplicate (`libs/prebuilt/langgraph/prebuilt/tool_node.py:784`); the threat model notes the lookup dict but not the lack of validation (`libs/langgraph/.github/THREAT_MODEL.md:268`). Users must guard against collisions themselves.
- **`create_react_agent` and `ValidationNode` are deprecated.** The canonical entry point has moved to `langchain.agents.create_agent` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`, `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46`). The registration model in the new entry point is not visible from this source — it lives in the separate `langchain` package and is not studied here.
- **`Runtime` vs `ToolRuntime` split.** `ToolRuntime` is a *subclass*-style extension of `Runtime` (via `_DirectlyInjectedToolArg`) that adds `state`, `tool_call_id`, `tools` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1662-1728`). The split is documented (`libs/langgraph/langgraph/runtime.py:138-141`) but creates two parallel runtime objects that the user has to know about.
- **Per-tool injection analysis is computed once but reused many times.** Cheap when the tool set is fixed; expensive when dynamic tools appear via interceptor (recomputed every call at `libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361`).
- **No tool-call validation against a schema at registration time.** Tools are registered as opaque `BaseTool` objects; there is no compile-time check that the model binding's tool list matches the `ToolNode`'s tools (this check only runs in `create_react_agent` and only when tools are pre-bound at the model level — `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217`).

## Failure Modes / Edge Cases

- **Unknown tool name → error `ToolMessage`** (not exception): `_validate_tool_call` returns a `ToolMessage` with `status="error"` and a list of registered names (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`). Interceptors that hand off unregistered tools to `execute()` get this error path.
- **Duplicate registration** silently overwrites. No exception, no warning.
- **Injected-arg spoofing** by the LLM is stripped: even if the LLM supplies a value for an `InjectedState`/`InjectedStore`/`ToolRuntime`/custom-`InjectedToolArg` parameter, `_inject_tool_args` removes it and substitutes the trusted value (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1422-1429`). Verified by `test_tool_node_strips_*` (`libs/prebuilt/tests/test_tool_node.py:2149-2206`).
- **Validation errors filtered to LLM-visible args only.** Injected-arg failures do not leak into error messages (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563, 961-966`).
- **`GraphBubbleUp` always re-raises**, regardless of `handle_tool_errors` setting (`libs/prebuilt/langgraph/prebuilt/tool_node.py:982-983, 1129-1130`). This is the escape hatch for `GraphInterrupt` inside tools.
- **Async wrapper fallback to sync wrapper.** If `awrap_tool_call` is `None` and `wrap_tool_call` is set, async paths fall back to the sync wrapper (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1191-1210`). Users must accept sync-only interception semantics in async runs, or supply both.
- **ContextVar lifetime for streaming.** `StreamToolCallHandler._reset_writer` swallows `ValueError` from `ContextVar.reset` if `on_tool_end` runs in a different context than `on_tool_start` (e.g., LangChain hands off to a thread worker) (`libs/langgraph/langgraph/pregel/_tools.py:213-222`).
- **`Send` payload version drift.** `ToolCallWithContext` is a typed-dict discriminated by `__type == "tool_call_with_context"` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`); `_extract_state` knows two shapes (inlined-state dict and bare-tool-call list) and hydrates state from channels via `CONFIG_KEY_READ` for the newer shape (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1281-1313`). Tools that branch across both shapes must tolerate hydration returning `{}`.
- **Tool-returning `Command` must terminate with a `ToolMessage` matching `tool_call["id"]`** for `graph=None` commands (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1503-1579`). Without it, `ValueError` is raised.

## Future Considerations

- The `langchain.agents.create_agent` factory (referenced as the canonical replacement in `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277` and `libs/prebuilt/langgraph/prebuilt/tool_node.py:633`) implies a registration model that lives outside this source. Whether it carries the `ToolNode` model forward or replaces it with a middleware-only registration is not visible from the langgraph source tree.
- `wrap_tool_call`-based dynamic registration has a known regression test pinning the behavior (`libs/prebuilt/tests/test_tool_node.py:1913-1999`) — issue reference `langchain-ai/langchain#35305`. This indicates the pattern is officially supported but is a workaround for missing first-class APIs.
- A public `ToolNode.add_tool()`/`unregister()` API would close the loop on dynamic tool sets without going through `wrap_tool_call`. No such API is exposed in the current source.
- Schema-versioned tool identities would help with replay/durability — currently, registered tools are matched by `name` only, and renames break history (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1032`).
- The `Required tool → MCP tool` flow is not modeled; there is no `MCPServerAdapter` in this source tree. MCP integration is presumably handled upstream in `langchain-mcp-adapters` or the future `langchain.agents` factory.

## Questions / Gaps

- **Tool identity across replay:** How does the langgraph runtime match a registered `BaseTool` to a `ToolMessage.tool_call_id` when the tool has been renamed between runs? No evidence of a stable tool UID beyond `name`.
- **Multi-agent tool isolation enforcement:** Is there a hard guarantee that a subgraph's `ToolNode` cannot see the parent's tools, or vice versa? The `StreamToolCallHandler` filters by namespace (`libs/langgraph/langgraph/pregel/_tools.py:117-118`), but no code path was found that *prevents* a subgraph from holding a `ToolNode` containing tools shared with the parent. The model is "trust the user to build the right graph."
- **`create_agent` registration semantics:** Not in this source. The deprecation notices (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:633`, `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46`) point to a separate package whose tool registration model is not in scope.
- **Tool-call concurrency:** Multiple parallel tool calls fan out via `Send` in v2 (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:849-859`), but the registration-side guarantee that two `BaseTool` objects with the same `name` cannot both be in `tools_by_name` at the same time is unenforced.
- **MCP / external catalog:** No registry or dynamic-discovery code path was found in this source. MCP-style servers (where tools are enumerated at runtime) must be implemented downstream.
- **Tool call to LLM tool-call ID stability:** `tool_call_id` is generated by the LLM provider and matched into `_validate_tool_call` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268`). LangGraph does not generate IDs itself; replay relies on the provider being deterministic.

---

Generated by `04.01-tool-definition-and-registration` against `langgraph`.
