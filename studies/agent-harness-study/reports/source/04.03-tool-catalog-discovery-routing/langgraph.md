# Source Analysis: langgraph

## 04.03 Tool Catalog, Discovery, and Routing

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (LangGraph core `libs/langgraph`, `libs/prebuilt`) |
| Analyzed | 2026-07-27 |

## Summary

LangGraph exposes a small, narrow tool catalog surface. The authoritative
catalog is a single per-node dict, `ToolNode._tools_by_name`, built once
at `ToolNode.__init__` time (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`).
Routing is name-based: every model-emitted `tool_call["name"]` is looked
up in that dict by `ToolNode._run_one` / `_arun_one`
(`libs/prebuilt/langgraph/prebuilt/tool_node.py:1032`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1179`),
and a hard allowlist of `_tools_by_name` is enforced by
`ToolNode._validate_tool_call` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`).
There is no retrieval-time discovery, no per-task capability filter, and
no per-agent tool list — every model-driven tool invocation is bound to
the same static set of `tools` passed to `create_react_agent`
(`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-588`).

Discovery is a single forward step taken at agent construction: if the
chat model is not already tool-bound, `_should_bind_tools` either calls
`model.bind_tools(tool_classes + llm_builtin_tools)` or refuses the call
(`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217`,
`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:582-588`). The
catalog presented to the model is therefore the exact list that
`ToolNode` will later route against; no second filtering step happens
between the model and the dispatcher.

Dynamic extension is supported via a single, explicit hook: the
`wrap_tool_call` / `awrap_tool_call` middleware on `ToolNode`
(`libs/prebuilt/langgraph/prebuilt/tool_node.py:755-778`). The hook sees
a `ToolCallRequest` whose `tool` field is `None` for unregistered names
(`libs/prebuilt/langgraph/prebuilt/tool_node.py:1032-1040`,
`libs/prebuilt/langgraph/prebuilt/tool_node.py:1179-1187`), and may
override the request to point at a `BaseTool` that was not in the
catalog at init time. `_inject_tool_args` then re-derives the injected
args for the dynamic tool on the fly (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361`).
This is the only dynamic-tool code path.

Tool availability is partially traceable: the full tool list is exposed
to the model itself via `model.bind_tools(...)` and to every tool via
`ToolRuntime.tools` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:813`,
`libs/prebuilt/langgraph/prebuilt/tool_node.py:848`), the failure path
enumerates registered names
(`libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110`,
`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`), and the
validation/normalization paths emit `status="error"` `ToolMessage`s
with descriptive strings. There is no first-class "why is this tool
visible?" trace; observability is achieved through the existing
callback and stream machinery (`libs/langgraph/langgraph/pregel/_tools.py:35-268`).

Routing is two-fan-out: `tools_condition` chooses between a literal
`"tools"` node and `END` on the agent loop (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659`),
and `should_continue` then dispatches each tool call to its own
`ToolNode` invocation via the `Send` API and the
`ToolCallWithContext` payload
(`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859`,
`libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`,
`libs/prebuilt/langgraph/prebuilt/tool_node.py:1224-1266`).

The remaining public surface (`ValidationNode`,
`tests/test_validation_node.py`, `tool_validator.py`) is marked
deprecated and is documented as having been moved to `langchain.agents`
(`libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46`).
`create_react_agent` itself is in the same situation
(`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`).
The companion source where the modern `create_agent` lives is out of
scope for this study.

## Rating

**6 / 10 — Present, tested, but static and coarse-grained.**

The model is explicit (one dict per node, one allowlist check, one
`bind_tools` decision) and the catalog/dynamic-tool split is well
documented, but the "smallest useful tool set" question is answered by
the user at construction time, not by the framework. There is no
per-task pruning, no permission layer, no retrieval index, and no
ceiling on how many tools a developer can shovel into `ToolNode`. The
deprecation of the public, user-facing APIs (`create_react_agent`,
`ValidationNode`, `create_tool_calling_executor`) pushes the durable
answer to `langchain.agents`, which is not present in this source.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool catalog (per-node dict) | `ToolNode._tools_by_name: dict[str, BaseTool]` populated once at `__init__` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:773` |
| Tool catalog population | Loop coerces plain callables via `create_tool` and stores by `tool.name` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:779-786` |
| Catalog accessor | `tools_by_name` property is the only public lookup | `libs/prebuilt/langgraph/prebuilt/tool_node.py:788-791` |
| Catalog forwarded to each tool | `ToolRuntime(..., tools=list(self.tools_by_name.values()), ...)` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:813` |
| Catalog forwarded (async) | Same construction in the async path | `libs/prebuilt/langgraph/prebuilt/tool_node.py:848` |
| Routing lookup (sync) | `tool = self.tools_by_name.get(call["name"])` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1032` |
| Routing lookup (async) | `tool = self.tools_by_name.get(call["name"])` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1179` |
| Routing allowlist enforcement | `_validate_tool_call` returns a `ToolMessage(status="error")` when name not in `tools_by_name` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279` |
| Invalid-tool error template | `INVALID_TOOL_NAME_ERROR_TEMPLATE` lists the registered names verbatim | `libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110` |
| Agent-level catalog construction | `create_react_agent` builds `ToolNode` from sequence or reuses one supplied | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-561` |
| Model-side binding | `model.bind_tools(tool_classes + llm_builtin_tools)` when LLM is not already bound | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:582-588` |
| `_should_bind_tools` | Inspects `RunnableBinding.kwargs["tools"]` and refuses tool lists that don't match | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217` |
| `_should_bind_tools` failure modes | Raises `ValueError` on count mismatch or missing bound tools | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:194-215` |
| Parallel dispatch via `Send` | `should_continue` returns one `Send("tools", ToolCallWithContext(...))` per call | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859` |
| `ToolCallWithContext` payload | `TypedDict` with `__type="tool_call_with_context"` discriminator | `libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306` |
| `ToolNode` accept of `ToolCallWithContext` | `_parse_input` recognizes the dict shape and routes to `input_type="tool_calls"` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1236-1245` |
| Direct tool-call input | `input_type="tool_calls"` path when last element is a dict with `type=="tool_call"` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1229-1233` |
| ReAct-style conditional edge | `tools_condition` returns `"tools"` or `"__end__"` based on last `AIMessage.tool_calls` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659` |
| Conditional wiring in agent | `add_conditional_edges("agent", should_continue, path_map=agent_paths)` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:964-968` |
| `return_direct` shortcut | `should_return_direct` set composed once at agent construction; `route_tool_responses` checks the last tool result | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:595-597`, `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:970-988` |
| Dynamic-tool hook | `wrap_tool_call` / `awrap_tool_call` callable on `ToolNode` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:755-778` |
| Dynamic-tool shortcut | `_run_one` defers validation to `_execute_tool_sync` so the hook can override `tool` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1030-1040` |
| Dynamic-tool injection | `_inject_tool_args` re-derives `_InjectedArgs` for a tool not seen at init | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361` |
| Dynamic-tool regression test | Asserts dynamic tool is not in `tools_by_name` but is callable via `wrap_tool_call` | `libs/prebuilt/tests/test_tool_node.py:1946`, `libs/prebuilt/tests/test_tool_node.py:1998` |
| Interceptor patterns | Whole-file catalog of `wrap_tool_call` behaviors (passthrough, override, short-circuit, command, retry, fail) | `libs/prebuilt/tests/test_on_tool_call.py:1-1473` |
| ValidationNode (deprecated) | `@deprecated` decorator points to `langchain.agents` | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46` |
| `ValidationNode` catalog | Builds `schemas_by_name: dict[str, type[BaseModel]]` from `BaseTool`/`BaseModel`/callables | `libs/prebuilt/langgraph/prebuilt/tool_validator.py:141-166` |
| `create_react_agent` deprecation | `@deprecated` decorator points to `langchain.agents.create_agent` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277` |
| `AgentState` family deprecation | All `AgentState*` TypedDicts/Pydantic moved to `langchain.agents` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-117` |
| Interrupt TypedDicts deprecation | `HumanInterrupt*`, `ActionRequest` moved to `langchain.agents.interrupt` | `libs/prebuilt/langgraph/prebuilt/interrupt.py:7-49` |
| Tool-call streaming channel | `StreamToolCallHandler` emits `tool-started`/`tool-output-delta`/`tool-finished`/`tool-error` | `libs/langgraph/langgraph/pregel/_tools.py:35-201` |
| Tool-call sink writable from tools | `_tool_call_writer` ContextVar + `ToolRuntime.emit_output_delta` | `libs/langgraph/langgraph/pregel/_tools.py:22-32`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1732-1750` |
| `tools_condition` defaults | `messages_key: str = "messages"` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1585` |
| Tags from `BaseTool` exposed on `ToolNode` | `tags: list[str] \| None = None` accepted by `__init__` and forwarded to `RunnableCallable` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:746-772` |
| Default error handler | `_default_handle_tool_errors` translates `ToolInvocationError` to message | `libs/prebuilt/langgraph/prebuilt/tool_node.py:383-391` |
| Error handling config types | `bool`, `str`, `Callable[..., str]`, `type[Exception]`, `tuple[type[Exception], ...]` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:749-753` |
| Injected args are stripped from LLM view | `tool_call_schema` filtering (in langchain-core) plus dict merge order in `_inject_tool_args` | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1429` |
| Catalog visible to model | `model.bind_tools(tool_classes + llm_builtin_tools)` | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:586-588` |
| `ToolNode.tools_by_name` exposed to all tools | `runtime.tools` is the full catalog | `libs/prebuilt/langgraph/prebuilt/tool_node.py:813`, `libs/prebuilt/langgraph/prebuilt/tool_node.py:1728` |
| Catalog-routing test | `test_should_bind_tools` parametrizes openai/anthropic and asserts mismatched lists raise | `libs/prebuilt/tests/test_react_agent.py:1491-1527` |
| Catalog-routing test | `test_tool_node_incorrect_tool_name` asserts the error message lists valid tools | `libs/prebuilt/tests/test_tool_node.py:565-591` |
| `tool_call_with_context` round-trip test | `test_state_extraction_with_tool_call_with_context` | `libs/prebuilt/tests/test_on_tool_call.py:1227-1313` |
| Threat model: tool-name allowlist | `ToolNode._validate_tool_call` is the routing-by-name gate | `sources/langgraph/.github/THREAT_MODEL.md:406` |
| Threat model: injection merge order | `{**stripped_args, **injected_args}` — system-injected args win | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1424-1429` |

## Answers to Dimension Questions

1. **Does every agent see every tool?**
   Effectively yes, in the statically-configured case. There is one
   `ToolNode` per graph, and `create_react_agent` either accepts a
   `ToolNode` directly or builds one from the `tools` sequence
   (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:554-561`).
   The same `tool_classes` list is both bound to the model
   (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:586-588`)
   and registered in `_tools_by_name`
   (`libs/prebuilt/langgraph/prebuilt/tool_node.py:779-786`). The
   model can only "see" tools that are present in both, and the
   `ToolNode` only routes to that same set. There is no per-turn
   narrowing.

2. **Are tools filtered by task?**
   No. The catalog is bound once at `create_react_agent` / `ToolNode`
   construction and is never re-derived from the current state, the
   current message, or any per-call policy. The closest analogue is
   `tools_condition` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1582-1659`),
   which decides whether to *invoke* the tool node, not which subset
   of tools to surface to the model. `wrap_tool_call` middleware can
   change which tool executes for a given call, but only by overwriting
   `request.tool` on a name the model already emitted — it does not
   prune the model-visible list.

3. **Are tools filtered by permission?**
   No. There is no permission, role, auth, or ACL layer in this
   source. `ExecutionInfo` / `ServerInfo` and `BaseUser` are passed
   through the runtime (`libs/langgraph/langgraph/runtime.py:26-77`)
   but `ToolNode` does not consult them when routing. The threat model
   confirms that any per-tool ACL is the project owner's responsibility
   (`sources/langgraph/.github/THREAT_MODEL.md:274`,
   `sources/langgraph/.github/THREAT_MODEL.md:421`).

4. **Can tools be hidden from the model?**
   Partially. Static filtering is possible: the project can pass a
   subset of `tools` to `create_react_agent` and a different subset
   to each `ToolNode` instance. The agent factory does not enforce
   that `bind_tools` and `ToolNode` agree beyond the sanity check in
   `_should_bind_tools` (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217`).
   At runtime, `wrap_tool_call` can short-circuit an unregistered
   tool name with a synthetic `ToolMessage` or `Command` without
   making the LLM-aware tool list smaller
   (`libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:48-107`,
   `libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:288-335`).

5. **Is tool availability explainable?**
   Indirectly. The available set is observable in three places: the
   model receives it via `bind_tools`, every tool receives it via
   `ToolRuntime.tools` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:813`,
   `libs/prebuilt/langgraph/prebuilt/tool_node.py:1728`), and the
   invalid-name error explicitly enumerates the registered names
   (`libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110`,
   `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`). There is
   no first-class "why was this tool visible?" trace, no per-tool
   availability metadata, and no audit log of which tool ran for
   which branch. Observability flows through the existing
   `tools` stream channel (`libs/langgraph/langgraph/pregel/_tools.py:35-201`).

## Architectural Decisions

- **One catalog per node, lifetime = graph lifetime.** `ToolNode` stores its
  catalog in `self._tools_by_name` and rebuilds it only on `__init__`
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`). There is
  no public mutation API, no `add_tool`-style registration, and no
  garbage collection. Decisions made at construction time are
  authoritative for the entire run.

- **Routing = dict lookup.** `ToolNode._run_one` does
  `self.tools_by_name.get(call["name"])` and dispatches the `None`
  case through `_validate_tool_call`
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1032`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`). This is
  the only routing primitive; the same code path handles both "valid,
  registered" and "invalid, must error" cases.

- **Model catalog and runtime catalog are checked for consistency.**
  `_should_bind_tools` rejects `create_react_agent` calls where the
  set the model was bound to does not match the set passed to the
  factory (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:173-217`).
  This is the framework's only "catalog-versus-routing" invariant.

- **Allowlist-over-blocklist enforcement naming.** The invalid-name
  error template names the registered tools verbatim
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:108-110`),
  mirroring allowlist semantics documented in the threat model
  (`sources/langgraph/.github/THREAT_MODEL.md:406`).

- **Dynamic tools are middleware, not catalog.** `_run_one` defers
  validation of `tools_by_name.get(...)` so the `wrap_tool_call` hook
  can override `request.tool` for unregistered names
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1030-1040`).
  `_inject_tool_args` then re-derives the injection plan on the fly
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1354-1361`). Tools
  added this way are not in the model-visible catalog; the model must
  still call a name it knows about.

- **Parallel fan-out via `Send`, not internal state.** `should_continue`
  emits one `Send("tools", ToolCallWithContext(...))` per
  `tool_call`, traded off against v1's in-process parallelism
  (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:831-859`,
  `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:298-305`).
  The typed payload is shaped so `ToolNode._parse_input` can recover
  the per-call state via `CONFIG_KEY_READ`
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1224-1266`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:1281-1313`).

- **Inject-on-call, not inject-on-build.** The injected-args map is
  cached per-tool at init (`libs/prebuilt/langgraph/prebuilt/tool_node.py:786`)
  but the actual merge happens in `_inject_tool_args` right before
  `tool.invoke` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:952-954`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:1100-1102`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:1315-1430`). This
  keeps `ToolRuntime` and `InjectedState` values fresh for the
  call's state snapshot.

- **Deprecation policy: public API moves to `langchain.agents`.** The
  LangGraph-OSS surface for tool calling is being thinned: `create_react_agent`,
  `ValidationNode`, `AgentState*`, `HumanInterrupt*`, and `ActionRequest`
  all carry `@deprecated` shims pointing at `langchain.agents`
  (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:53-117`,
  `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`,
  `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46`,
  `libs/prebuilt/langgraph/prebuilt/interrupt.py:7-49`). Where the
  next-generation routing, catalog, and middleware live is outside
  the selected source.

## Notable Patterns

- **Per-call interceptor as the dynamic-extension point.** The
  `wrap_tool_call` hook is the one place where catalog membership
  can be expanded at runtime
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:755-778`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:1030-1070`).
  `ToolCallRequest` exposes `override()` for immutable request
  mutation (`libs/prebuilt/langgraph/prebuilt/tool_node.py:170-199`),
  and `request.tool is None` is the explicit signal for an
  unregistered call
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1032-1040`,
  `libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:515-582`).

- **All registered tools visible to every tool.** `ToolRuntime.tools`
  is constructed as `list(self.tools_by_name.values())` for every
  invocation (`libs/prebuilt/langgraph/prebuilt/tool_node.py:813`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:848`). A tool can
  read the full catalog and dispatch to other tools by name and
  `tool_call_id`, mirroring a manual sub-agent pattern.

- **Tool-call streaming surface.** The `tools` stream channel emits
  `tool-started` / `tool-output-delta` / `tool-finished` / `tool-error`
  events keyed by `tool_call_id`
  (`libs/langgraph/langgraph/pregel/_tools.py:130-201`).
  `ToolRuntime.emit_output_delta` reads the per-call writer
  ContextVar (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1732-1750`)
  so tool bodies can stream partial output without threading the
  writer through their signature.

- **Deferred validation.** Validation of "is this tool registered?" is
  deferred to `_execute_tool_sync` /
  `_execute_tool_async` so the interceptor can short-circuit
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1030-1040`,
  `libs/prebuilt/langgraph/prebuilt/tool_node.py:1177-1187`).
  This is the affordance that makes dispatch-time override possible.

- **Injected-args are stripped from LLM-visible schema.** The
  framework relies on `tool_call_schema` filtering in langchain-core
  plus a final merge `{**stripped_args, **injected_args}` so system
  values overwrite any LLM-supplied collisions
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1429`,
  `sources/langgraph/.github/THREAT_MODEL.md:207`).

- **Validated `tool_call_with_context` discriminator.** The Send-API
  payload uses a `__type` marker that is namespaced with `__` to
  avoid collision with user state keys
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:286-306`).

## Tradeoffs

- **Static, declarative catalog vs. fine-grained control.** The
  framework's choice to make one `ToolNode` own the entire catalog
  keeps routing deterministic and observable, but it pushes all
  sophisticated selection (e.g. "show only tools relevant to the
  current task") into the user's responsibility. There is no
  callback or predicate that decides per-step which tools the model
  can call.

- **Single-turn model catalog vs. dynamic middleware.** The model
  sees the catalog exactly once at `bind_tools` time. After that,
  the only way to expand the executable set is the `wrap_tool_call`
  hook, which requires the model to call a name it already knows.
  This blocks "discover a new tool at runtime" patterns without
  custom DSL.

- **Err-on-error-over-skip for unknown tools.** `_validate_tool_call`
  always returns a `ToolMessage(status="error")` for an unknown name,
  even when `handle_tool_errors=False`
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`,
  `libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:702-733`).
  This is the safe default — skip-as-silent-success would be a
  prompt-injection foothold — but it means routing errors are
  surfaced as model-visible strings rather than raised exceptions.

- **Coarse-grained fan-out.** v2 uses `Send` to fan out one tool
  call per `ToolNode` invocation
  (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:846-859`).
  This improves parallelism and human-in-the-loop support but
  increases the cost of cross-tool reasoning (no shared in-memory
  cache between parallel tool calls).

- **De-recated public API.** `create_react_agent` and `ValidationNode`
  are deprecated, but the replacements are in `langchain.agents`,
  not in this source. The durable tool-catalog story is therefore
  partially outside the boundary of this study.

- **No native per-tool permission/role.** The threat model explicitly
  delegates permission checks to the project
  (`sources/langgraph/.github/THREAT_MODEL.md:274`,
  `sources/langgraph/.github/THREAT_MODEL.md:421`). Forcing every
  LangGraph user to write their own ACL is a meaningful footgun.

## Failure Modes / Edge Cases

- **Unknown tool name → error-shaped `ToolMessage`.** When the model
  emits a name not in `_tools_by_name`, `ToolNode` returns a
  `ToolMessage(status="error")` rather than raising
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`,
  `libs/prebuilt/tests/test_tool_node.py:565-591`).

- **Interceptor that calls `execute` for an unregistered tool.** The
  execute callable raises `TypeError("Tool ... is not registered with
  ToolNode")` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:949-950`),
  but the underlying error is funneled through `_validate_tool_call`
  to produce a recoverable string
  (`libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:172-227`).

- **Catalog/binding mismatch.** `_should_bind_tools` raises
  `ValueError` when the model is pre-bound with a different tool set
  (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:194-215`).
  This is the framework's only check against an out-of-sync
  model/runtime catalog.

- **Schema mismatch on inject.** `InjectedState("foo")` for a state
  field that doesn't exist raises `KeyError`/`AttributeError`
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1394-1405`). The
  validation error is filtered through `_filter_validation_errors`
  so the LLM does not see injected-arg failures
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:510-563`).

- **Tool list is forwarded verbatim to every tool, including
  itself.** `ToolRuntime.tools` is the full `_tools_by_name` dict
  for every call, so a tool can recurse into other tools without
  going through the graph. This is intentional but undocumented
  anywhere explicit; the only wiring is the test
  `tests/test_tool_node.py:2032-2084`.

- **Send-payload regression.** `ToolCallWithContext` is the only
  supported payload shape for parallel tool calls; using a generic
  message list with tool calls in v2 will route through
  `_extract_state`'s `CONFIG_KEY_READ` fallback path
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1281-1313`).

- **Same tool name in `tools` and `llm_builtin_tools`.** The
  dedup story is implicit: `bind_tools(tool_classes + llm_builtin_tools)`
  concatenates them with `+` and lets langchain-core
  `bind_tools` decide. There is no explicit dedup in
  `create_react_agent`.

- **Dynamic tools require the model to know the name.** Because the
  catalog is bound at construction, the only way for a tool to be
  dynamically invoked is for the model to call a name that
  `wrap_tool_call` then overrides. A discoverable tool that the
  model has never seen cannot be invoked.

## Future Considerations

- **Move to `langchain.agents`-style middleware.** The current
  `wrap_tool_call` hook is the seed of a richer middleware system;
  the deprecation messages
  (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:274-277`,
  `libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-46`,
  `libs/prebuilt/langgraph/prebuilt/interrupt.py:7-49`) suggest the
  next-generation routing lives there. Studying that source is
  required before claiming the catalog/discovery story is
  complete.

- **Per-task tool pruning.** The static catalog is the most
  obvious next gap. Adding a predicate like `select_tools(state,
  tool_classes) -> list[BaseTool]` that runs each step would
  answer the "smallest useful tool set" question without breaking
  the existing dispatch path.

- **Per-tool permissions.** The threat model explicitly leaves
  permission policy to the user
  (`sources/langgraph/.github/THREAT_MODEL.md:274`,
  `sources/langgraph/.github/THREAT_MODEL.md:421`). A
  `Runtime.server_info.user` / `ExecutionInfo.thread_id`-aware
  decorator on `BaseTool` would close the gap without breaking
  static routing.

- **Structured "why is this tool available?" trace.** Combine the
  existing `tools` stream channel with a registry-aware index so
  debugging can answer "which subset of `_tools_by_name` was active
  when this call was routed?".

- **Tool discovery via retrieval.** The store primitive
  (`BaseStore`) is already injected into tools
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1408-1415`);
  there is no retrieval-based tool picker yet. This is a natural
  extension point but would require changes to `bind_tools`
  semantics.

## Questions / Gaps

- **What is the durable tool-catalog API in `langchain.agents`?**
  Several public APIs in `libs/prebuilt` are deprecated with
  redirects to `langchain.agents` (`create_react_agent`,
  `ValidationNode`, `AgentState*`, `HumanInterrupt*`). The catalog,
  discovery, and routing primitives for the modern path are out of
  scope for this source; a follow-up study against the `langchain`
  package is required to characterize the durable shape.

- **Is there a per-node tool list outside of `ToolNode`?** Manual
  dispatch in `libs/langgraph/tests/test_large_cases.py:1651-1705`
  builds `tools_by_name = {t.name: t for t in tools}` but does not
  use `ToolNode` (`libs/langgraph/tests/test_large_cases.py:1651`).
  This is a test fixture, not a documented dispatch pattern, so it
  was excluded from the main analysis.

- **Does `ToolNode` expose a hook to mutate the catalog at runtime?**
  No public API was found. The catalog is set in `__init__` and
  `tools_by_name` is a read-only property
  (`libs/prebuilt/langgraph/prebuilt/tool_node.py:788-791`). Any
  runtime mutation would have to be done via a wrapper that
  subclasses `ToolNode` or replaces it wholesale.

- **Does `wrap_tool_call` change the model-visible catalog?** No.
  The hook intercepts execution on a per-call basis but does not
  edit `model.bound.kwargs["tools"]`. The model's view of the
  catalog is fixed at `bind_tools` time.

- **Can a tool be hidden from the model but offered to other tools?**
  Not directly. `ToolRuntime.tools` is the same list as the model's
  binding. There is no separate "executable-only" catalog.

---

Generated by `04.03-tool-catalog-discovery-and-routing` against `langgraph`.
