# Source Analysis: langgraph

## Tool Permissions and Approval Metadata

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (LangGraph monorepo: Pydantic v1/v2, LangChain Core integration) |
| Analyzed | 2026-08-15 |

## Summary

LangGraph ships **no built-in risk classification, permission enum, or per-tool approval policy**. The `ToolNode` and `BaseTool` integration points carry only structural metadata (`name`, `description`, `args_schema`, `tags`) — there is no `risk_level`, `requires_approval`, `side_effects`, or `permission_scope` field exposed to the runtime (`libs/prebuilt/langgraph/prebuilt/tool_node.py:622-787`). The `tool_validator.py` validator operates on Pydantic argument schemas, not on permissions (`libs/prebuilt/langgraph/prebuilt/tool_validator.py:34-221`).

The only authorization mechanisms LangGraph exposes are:

1. **Static interrupt hooks** at the graph level: `interrupt_before` and `interrupt_after` accept a list of node names (`libs/langgraph/langgraph/pregel/_loop.py:660-664,708-712`) and raise `GraphInterrupt` regardless of which tools are in flight. The runtime cannot distinguish between "stop and ask about the agent's text" vs. "stop and ask about a tool call".
2. **In-tool `interrupt(value)` calls** that any tool may invoke (`libs/langgraph/langgraph/types.py:811-934`). Tools opt in to human review; there is no policy that requires them to.
3. **Pluggable per-tool wrappers** via `wrap_tool_call` / `awrap_tool_call` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:755-778,1014-1067,1161-1222`). These are user-supplied callbacks that receive `ToolCallRequest` and may either short-circuit or call `execute()`.
4. **Tool registry filtering** in `_validate_tool_call` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`): any tool the model names that isn't registered gets an error `ToolMessage` and is **not** invoked. This is name-membership, not risk-based.

Permission metadata is **not visible to the runtime** — `ToolRuntime` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1662-1730`) carries `state`, `config`, `store`, `tools`, `context`, `stream_writer`, `tool_call_id`, `execution_info`, `server_info`, but no permission/risk descriptor. The server-side `Auth` system in `libs/sdk-py/langgraph_sdk/auth/__init__.py:13-876` covers threads/assistants/crons/store; **there is no `tools` resource** (`libs/sdk-py/langgraph_sdk/auth/__init__.py:322-326`).

Approval decisions are **not persisted**. `interrupt()` values are stashed in the per-task `scratchpad` (`libs/langgraph/langgraph/types.py:912-924`) and re-supplied on resume; the framework never records "user said yes to `delete_file`" as durable state.

**Can the runtime stop a high-risk tool even if the model asks for it?** Only if a user-supplied `wrap_tool_call` callback explicitly denies it, or if the tool was never registered with the `ToolNode`. There is no declarative allow/deny list keyed on risk class. The `interrupt_before` mechanism can pause *any* node, but it cannot selectively pause only dangerous tools.

## Rating

**3/10** — Absent as a built-in capability; user-supplied glue only.

Rationale:
- There is **no permission enum, no risk classifier, no policy object, no persisted approval record, and no per-tool metadata field** anywhere in `ToolNode`, `BaseTool`, or `ToolRuntime` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:622-787,1662-1730`).
- `wrap_tool_call` is a genuine enforcement hook — a wrapper can short-circuit or reject a tool call before `tool.invoke` is called (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1044-1067`), which is the one durable stop-the-tool mechanism. But the wrapper is entirely user code; nothing in the framework declares it must exist for dangerous tools.
- `interrupt_before` / `interrupt_after` work and are tested end-to-end (`libs/prebuilt/tests/test_react_agent.py:1421-1488`, `libs/langgraph/langgraph/pregel/_loop.py:660-664`), but they operate on **node names**, not on tool metadata. Pairing them with a tool is a user convention.
- The `tool_validator.py` module name is misleading: it validates Pydantic *argument schemas*, not risk or approval (`libs/prebuilt/langgraph/prebuilt/tool_validator.py:47-114`).
- The deprecated `interrupt.py` `HumanInterruptConfig` (`libs/prebuilt/langgraph/prebuilt/interrupt.py:11-26`) is the only artifact in the codebase that exposes `allow_ignore / allow_respond / allow_edit / allow_accept` flags, and even those are shimmed out with `@deprecated` redirects to `langchain.agents.interrupt`.
- The SDK `Auth` class is resource-scoped (threads/assistants/crons/store) and has no `tools` resource at all (`libs/sdk-py/langgraph_sdk/auth/__init__.py:322-326`), so server-side tool permission gating is delegated to user-defined auth handlers that intercept HTTP requests, not tool invocations.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission enums | **None found.** No `Permission`, `Risk`, `SideEffect`, `Approval`, `Scope` enum/class in `libs/prebuilt/langgraph/prebuilt/` or `libs/langgraph/langgraph/`. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:622-787` |
| Tool metadata fields on `ToolNode` | Constructor accepts `tools`, `name`, `tags`, `handle_tool_errors`, `messages_key`, `wrap_tool_call`, `awrap_tool_call` — no risk/permission fields. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:743-787` |
| Tool metadata fields on `BaseTool` | Inherited from `langchain_core.tools.BaseTool`; only `name`, `description`, `args_schema`, `tags` (decorator metadata) are referenced. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:779-786` |
| Approval policy | None as a policy object. Static `interrupt_before` / `interrupt_after` lists of *node names* are the closest analog. | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:302-303,447-454` |
| Approval registration | `create_react_agent(..., interrupt_before=[...], interrupt_after=[...])` is the sole configuration surface. | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:447-454,824-825,998-999` |
| Interrupt execution | `should_interrupt` decides which tasks to interrupt, then `raise GraphInterrupt()`. | `libs/langgraph/langgraph/pregel/_loop.py:660-664,708-712` |
| `should_interrupt` rule | If `interrupt_nodes == "*"` or `task.name in interrupt_nodes`, that task becomes interruptible. No tool-name gating. | `libs/langgraph/langgraph/pregel/_algo.py:155-185` |
| In-tool interrupt primitive | `interrupt(value)` raises `GraphInterrupt((Interrupt(value, id),))` on first call; on resume returns `scratchpad.resume[idx]`. | `libs/langgraph/langgraph/types.py:811-934` |
| `Interrupt` data class | Holds only `value: Any` and `id: str` — no action, no approver, no risk tag. | `libs/langgraph/langgraph/types.py:533-588` |
| Tool wrapper hook (real enforcement surface) | `wrap_tool_call(request, execute)` and `awrap_tool_call(...)` receive `ToolCallRequest` and may return early without invoking `execute()`. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:755-778,1044-1067,1161-1222` |
| `ToolCallRequest` payload | `tool_call`, `tool`, `state`, `runtime` — no permission/risk descriptor on the request object. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:132-199` |
| Registry check (name-only) | `_validate_tool_call` returns an error `ToolMessage` when the tool name is not in `self.tools_by_name`. This is membership, not risk. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279` |
| Permission metadata visible to runtime | `ToolRuntime` exposes `state`, `config`, `context`, `store`, `stream_writer`, `tool_call_id`, `tools`, `execution_info`, `server_info` — no permission field. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1662-1750` |
| Server info (auth context) | `ServerInfo.user: BaseUser | None` is injected by LangSmith/Server; `permissions` may exist via `__getattr__` on `ProxyUser`. | `libs/langgraph/langgraph/pregel/main.py:4335` (comment) |
| Injected-args stripping | `ToolNode._inject_tool_args` strips caller-supplied injected keys to prevent LLM-forged fields. This is argument safety, not permission gating. | `libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1430` |
| Deprecated approval schema | `HumanInterruptConfig{allow_ignore, allow_respond, allow_edit, allow_accept}` — the only artifact with explicit approval flags; deprecated and moved to `langchain.agents.interrupt`. | `libs/prebuilt/langgraph/prebuilt/interrupt.py:11-105` |
| Human response shape | `HumanResponse.type ∈ {"accept","ignore","response","edit"}` — generic interrupt semantics, not per-tool. | `libs/prebuilt/langgraph/prebuilt/interrupt.py:87-105` |
| Server-side `Auth` resources | Resources are `threads`, `crons`, `assistants`; actions are `create/read/update/delete/search/create_run`. **No `tools` resource.** | `libs/sdk-py/langgraph_sdk/auth/__init__.py:322-326,463-495` |
| Tests for denied tools | `test_tool_node_node_interrupt` proves a tool calling `interrupt(...)` blocks its sibling calls in v1 but not in v2; **no test denies a tool based on risk metadata**. | `libs/prebuilt/tests/test_react_agent.py:1421-1488,594-637` |
| Tool interceptor test coverage | Many `wrap_tool_call` tests, none assert a framework-side permission gate. | `libs/prebuilt/tests/test_tool_node_interceptor_unregistered.py:1-784`, `libs/prebuilt/tests/test_on_tool_call.py:1-1473` |

## Answers to Dimension Questions

1. **Are tools risk-classified?** No. `BaseTool` / `ToolNode` do not expose a `risk` or `permission` field. The only `tags` attribute on `ToolNode` is opaque strings used for filtering/visualization (`libs/prebuilt/langgraph/prebuilt/tool_node.py:748,763-764`). No risk enum exists in the source tree.

2. **Are permissions enforced?** Only via user-supplied wrappers. A `wrap_tool_call` callback may return a `ToolMessage` (or `Command`) without invoking `execute`, which is the only hard-stop mechanism in `ToolNode` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1044-1067,1161-1222`). There is no declarative `allow=[...]` / `deny=[...]` parameter. The registry check `_validate_tool_call` blocks unregistered names but is name-membership, not risk (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`).

3. **Can users approve selectively?** No out-of-the-box UI. Users must write a `wrap_tool_call` wrapper that inspects `request.tool_call["name"]` / `request.tool_call["args"]` and either raises `interrupt(...)` or returns a denial message. The deprecated `HumanInterruptConfig` exposes `allow_ignore / allow_respond / allow_edit / allow_accept` (`libs/prebuilt/langgraph/prebuilt/interrupt.py:23-26`), but it has been moved to `langchain.agents.interrupt` and is no longer the recommended path.

4. **Are approvals persisted?** No. `interrupt(value)` stores the request value and resume value in the per-task scratchpad for the current thread (`libs/langgraph/langgraph/types.py:911-924`), but no durable "this user approved `send_email` for thread X" record exists. The checkpoint captures the interrupt payload but the framework does not aggregate it into an approval log.

5. **Can policy block a model-requested tool?** Only if (a) the tool was never registered with `ToolNode` (handled by `_validate_tool_call`), or (b) a user-supplied `wrap_tool_call` wrapper decides to deny it. There is no framework-level policy engine keyed on tool identity, args, or risk. The `interrupt_before=["tools"]` mechanism pauses *all* tool calls in that node and cannot selectively gate by tool name (`libs/langgraph/langgraph/pregel/_loop.py:660-664,708-712`; `libs/langgraph/langgraph/pregel/_algo.py:155-185`).

**Direct answer to the dimension question — "Can the runtime stop a high-risk tool even if the model asks for it?"** Only if a user has registered a denial hook; LangGraph itself does not know which tools are high-risk.

## Architectural Decisions

- **No declarative risk metadata.** `ToolNode` accepts a `Sequence[BaseTool | Callable]` and stores them in `self._tools_by_name` keyed by `tool.name` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:773-786`). Any tool can be added; no metadata is required to declare side effects or risk.
- **Per-tool wrapper is the policy surface.** `wrap_tool_call` / `awrap_tool_call` callbacks are the only place where a user can intercept a specific tool call before `tool.invoke`/`tool.ainvoke` runs (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1044-1067,1191-1210`). Wrappers may short-circuit by returning a `ToolMessage` or `Command`.
- **Injected args strip LLM control.** `_inject_tool_args` removes caller-supplied values for any parameter annotated with `InjectedState` / `InjectedStore` / `ToolRuntime`, so the model cannot smuggle hidden context (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1430`). This is argument safety, not risk gating.
- **Interrupt model is value-payload, not schema-typed.** `interrupt(value)` and the deprecated `HumanInterrupt` shape accept arbitrary payloads. There is no shared schema that would let a UI render a generic "approve / deny / edit" prompt for any tool (`libs/langgraph/langgraph/types.py:811-934`; `libs/prebuilt/langgraph/prebuilt/interrupt.py:51-105`).
- **Server-side auth is per-resource, not per-tool.** `Auth` in `libs/sdk-py/langgraph_sdk/auth/__init__.py:13-876` gates threads, assistants, crons, and the store; tool invocation is not a recognized resource.
- **Tool refusal is name-based.** `_validate_tool_call` only checks registry membership (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`). There is no fallback that says "this tool exists but is denied".

## Notable Patterns

- **Wrap-and-execute hook** — `ToolCallRequest` + `execute` closure pattern (`libs/prebuilt/langgraph/prebuilt/tool_node.py:202-282,1014-1067`). Clean separation between the framework's execution path and user-supplied interception.
- **Injected-arg scraping** — `_get_all_injected_args` walks the schema and signature once at `ToolNode.__init__`, then reuses the result for every call (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1967-2030`).
- **Interrupt scratchpad** — `interrupt()` persists resume values on the `PregelScratchpad` (`libs/langgraph/langgraph/types.py:911-924`) so the *same* node re-run can pick up answers without the model re-calling.
- **`Send` API per-tool dispatch** — v2 of `create_react_agent` fans out tool calls with `Send`, which is what enables per-tool interrupt without affecting siblings (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:460-465`).
- **StreamToolCallHandler as lifecycle notifier** — emits `tool-started`/`tool-output-delta`/`tool-finished`/`tool-error` for observability (`libs/langgraph/langgraph/pregel/_tools.py:35-268`). Does not gate execution.
- **Auth decorator surface** — `@auth.on.threads.create` etc. is a clean DSL for per-resource authorization (`libs/sdk-py/langgraph_sdk/auth/__init__.py:727-738`). No equivalent for tools.

## Tradeoffs

- **Simplicity vs. safety**: `ToolNode` is minimal and lets users layer policies on top via `wrap_tool_call`. The cost is that the framework never warns about an unsafe tool; safety is a user responsibility.
- **Interrupt generality vs. precision**: `interrupt(value)` accepts any payload, which is flexible but forces every UI integration to invent its own approval UX. There is no canonical `approve / deny / edit` widget.
- **Wrapper ergonomics vs. framework contract**: `wrap_tool_call` is a powerful escape hatch but the framework does not require any wrapper for any tool. A forgotten wrapper on a destructive tool means no safety net.
- **Auth-resource scope vs. tool scope**: `Auth` covers persistence resources (threads, store) where user identity makes sense, but tool invocation has no resource abstraction. A user wanting per-tool auth must build a `wrap_tool_call` that consults `runtime.context` / `server_info.user`.
- **Deprecation without replacement**: `HumanInterruptConfig` was deprecated and moved to `langchain.agents.interrupt` (`libs/prebuilt/langgraph/prebuilt/interrupt.py:7-9`). The replacement lives in a different package, leaving `langgraph.prebuilt.interrupt` with only the deprecated shims.

## Failure Modes / Edge Cases

- **No deny-by-default.** A `ToolNode` constructed without a `wrap_tool_call` will execute any tool the model names that is in `tools_by_name`. There is no opt-in "require approval for tools with names matching X" flag.
- **Interrupt-before-tools pauses all tools.** `interrupt_before=["tools"]` pauses the *whole* tool node, including read-only tools (`libs/langgraph/langgraph/pregel/_loop.py:660-664`). No mechanism to pause only side-effecting tools.
- **Unregistered tools are reported as errors, not silently dropped.** `_validate_tool_call` returns a `ToolMessage(content=..., status="error")` and feeds it back to the model (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1268-1279`). The model may then re-attempt or escalate — there is no hard stop unless the model cooperates.
- **Tool-result errors do not block re-execution.** `_handle_tool_error` and `handle_tool_errors=True` convert exceptions into `ToolMessage(status="error")` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:984-1012`). An exception from a destructive tool (e.g. permission denied at the OS level) is reported to the model, not used as a policy signal.
- **`wrap_tool_call` exceptions flow through `handle_tool_errors`.** If the wrapper itself throws and `handle_tool_errors=True`, the wrapper's exception is converted to a `ToolMessage` and the model sees a tool result instead of a hard denial (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1056-1067`).
- **Server `Auth` does not see tool invocations.** Since there is no `tools` resource, a server-side `@auth.on` handler cannot deny a tool call; it can only deny the `runs.create` request that contains the tool list (`libs/sdk-py/langgraph_sdk/auth/__init__.py:322-326`).
- **Async path can fall back to sync wrapper.** If `awrap_tool_call` is unset but `wrap_tool_call` is set, async execution runs the sync wrapper (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1191-1210`). User-supplied async I/O in a `wrap_tool_call` will be called from the async path, potentially blocking the loop.

## Future Considerations

- A first-class `risk: Literal["read","write","delete","network","external"]` field on `ToolNode` (or on `BaseTool` via `langchain-core`) would make policies declarative instead of wrapper-only.
- A canonical approval schema (e.g., `ApproveToolCall`, `DenyToolCall`) shared between `interrupt()` callers and UI renderers would replace the deprecated `HumanInterruptConfig` (`libs/prebuilt/langgraph/prebuilt/interrupt.py:11-26`).
- Adding a `tools` resource to `Auth` (`libs/sdk-py/langgraph_sdk/auth/__init__.py:322-326`) would enable server-side per-tool authorization without `wrap_tool_call` plumbing.
- An `interrupt_before_tools=[...]` analogue that takes *tool names* (or tool descriptors) instead of node names would let users gate specific tools without wrapping the whole node (`libs/langgraph/langgraph/pregel/_algo.py:155-185`).
- Persisted "approved tool list" in the checkpointer alongside `messages` would let the runtime auto-approve safe tools after first approval.

## Questions / Gaps

- **No evidence of a `langgraph.prebuilt.tool_node.Risk` enum or similar.** Search of the prebuilt library for `risk`, `permission`, `allow`, `deny`, `safe` returned only docstring prose about validation/argument handling, not risk classification.
- **`tool_validator.py` is misleading.** The class is `ValidationNode` (deprecated) and validates Pydantic schemas, not permissions (`libs/prebuilt/langgraph/prebuilt/tool_validator.py:43-114`). A future reader might assume this is the approval surface.
- **No tests assert a framework-side denial path.** Every test in `test_tool_node_interceptor_unregistered.py` and `test_on_tool_call.py` uses a user-supplied `wrap_tool_call`. No test exercises a built-in policy.
- **`interrupt.py` is fully deprecated.** The `HumanInterruptConfig` shim (`libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10`) is the only "approval flag" surface in the source tree, and it is on its way out.
- **The dimension's exact question — "Can the runtime stop a high-risk tool even if the model asks for it?" — has no positive evidence.** The answer is "only via user code".