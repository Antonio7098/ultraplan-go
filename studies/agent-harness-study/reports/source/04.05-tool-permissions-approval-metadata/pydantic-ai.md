# Source Analysis: pydantic-ai

## Tool Permissions and Approval Metadata

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10+ (Pydantic v2, asyncio; provider-agnostic GenAI agent framework) |
| Analyzed | 2026-08-15 |

## Summary

Pydantic AI implements **tool-kind routing** rather than a general-purpose permission enum. Every `ToolDefinition` carries a `kind: ToolKind` discriminator (`ToolKind = Literal['function', 'output', 'external', 'unapproved']` at `pydantic_ai_slim/pydantic_ai/tools.py:682`); the agent graph branches on this kind to either execute the call immediately, validate it as an output schema, route it to an external service, or **defer it for human-in-the-loop approval**.

Two parallel enforcement hooks gate high-risk tools. The **static** hook is `requires_approval: bool = False` on `Tool`, `FunctionToolset.tool`/`add_function`, `Capability.tool`/`tool_plain`, and the `@agent.tool`/`@agent.tool_plain` decorators; setting it stamps the resulting `ToolDefinition` with `kind='unapproved'` (`pydantic_ai_slim/pydantic_ai/tools.py:649`) and prevents the call from ever executing in-line (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1749-1776`). The **dynamic** hook is `AbstractToolset.approval_required(approval_required_func)` which composes an `ApprovalRequiredToolset` wrapper (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:14-32`); the wrapper inspects `(ctx, tool_def, validated_args)` per call and raises `ApprovalRequired` if the predicate returns `True` and `ctx.tool_call_approved` is `False`.

Runtime exposes approval state through `RunContext.tool_call_approved: bool = False` (`pydantic_ai_slim/pydantic_ai/_run_context.py:80`) and `RunContext.tool_call_metadata: Any = None` (`pydantic_ai_slim/pydantic_ai/_run_context.py:82`); both are stamped from `ToolManager._build_tool_context` only when re-validating after a `DeferredToolResults` was supplied (`pydantic_ai_slim/pydantic_ai/tool_manager.py:193-213`).

The framework treats approval as a **first-class invocation kind** with typed approval/denial envelopes. Approvals and denials are the discriminated union `ToolApproved | ToolDenied` (`pydantic_ai_slim/pydantic_ai/tools.py:329-348`); `ToolApproved.override_args` lets the approver edit validated args before re-execution (`pydantic_ai_slim/pydantic_ai/tools.py:333`); `ToolDenied.message` lets the approver inject a model-facing explanation (`pydantic_ai_slim/pydantic_ai/tools.py:343`).

There is **no built-in risk taxonomy**: no `'read-only'` / `'write'` / `'network'` / `'secrets'` / `'money'` field. Classification is a binary `requires_approval=True` (or a runtime predicate). The framework deliberately exposes an opaque `metadata: dict[str, Any] | None` field on every tool and `SetToolMetadata`/`SetMetadataToolset` (`pydantic_ai_slim/pydantic_ai/capabilities/set_tool_metadata.py:16-56`, `pydantic_ai_slim/pydantic_ai/toolsets/set_metadata.py:13-28`) so users can layer their own classification on top — but no convention is built in.

Approvals are **persisted as part of the conversation log**, not as a separate policy store. The protocol shape is `DeferredToolRequests(approvals=[ToolCallPart, ...], metadata={tool_call_id: ...})` (`pydantic_ai_slim/pydantic_ai/tools.py:256-326`); denials surface as `ToolReturnPart(outcome='denied', content=...)` in the next model request (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2003-2009`). Message history plus a `DeferredToolResults` is the second-run input (`docs/deferred-tools.md:101-103`).

Policy enforcement has **two complementary layers**. The **inline layer** is `HandleDeferredToolCalls` capability (`pydantic_ai_slim/pydantic_ai/capabilities/deferred_tool_handler.py:15-75`), which intercepts deferred requests during a run and either resolves them, declines, or lets them bubble up — accumulating across `CombinedCapability` (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:660-682`). The **stop-the-world layer** is `DeferredToolRequests` as an `output_type`: when no handler resolves inline, the run ends with the request and the caller must restart with `DeferredToolResults` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1849-1856`).

**Conditional approval** is first-class: a tool body may `raise ApprovalRequired(metadata=...)` to halt mid-execution (`pydantic_ai_slim/pydantic_ai/exceptions.py:98-114`), and the agent graph classifies that exception into `deferred_calls_by_index[index] = 'unapproved'` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1889-1894`). The same `ctx.tool_call_approved` check inside the tool body distinguishes first call from approved re-execution (`tests/test_tools.py:1536`).

## Rating

**8 / 10** — Clear, typed model with explicit interfaces, tests, and operational safeguards. The model is complete for human-in-the-loop approval but does not expose a general risk classification or cross-tool policy engine; enforcement is per-tool (declarative `requires_approval`) or per-toolset (predicate `approval_required_func`), not a centralized allow/deny list keyed on risk class.

Rationale:
- +3 typed model with explicit interfaces and rich error types (`ToolApproved.override_args`, `ToolDenied.message`, `DeferredToolResults.metadata`).
- +2 dual-flow (inline `HandleDeferredToolCalls` + stop-the-world `DeferredToolRequests`) with capability accumulation (`CombinedCapability.handle_deferred_tool_calls`).
- +1 conditional approval via `raise ApprovalRequired` and `ctx.tool_call_approved` checked inside the tool body.
- +1 persistence: approvals/denials are recorded in `ToolReturnPart.outcome='denied'` on the model-request side.
- +1 comprehensive test coverage for all flows (`tests/test_tools.py:1517-1816`, `tests/test_tools.py:2374-2534`, `tests/test_agent.py:8406-8471`, `tests/test_streaming.py:3541-3761`, `tests/test_capabilities.py:18964-19060`).
- −1 no risk enum or permission taxonomy: `ToolKind` lumps approval into `'unapproved'`; there is no `'read-only'`/`'write'`/`'network'`/`'secrets'`/`'money'` discriminator.
- −1 no centralized policy store or blanket deny; enforcement is per-tool opt-in or per-toolset predicate, so a developer who forgets to mark `requires_approval=True` on a destructive tool gets no protection.
- −1 `metadata` is a free-form `dict[str, dict[str, Any]]`; there is no schema or runtime validation that metadata keys follow a risk-classification convention.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tool-kind discriminator (risk classification by routing) | `ToolKind: TypeAlias = Literal['function', 'output', 'external', 'unapproved']` | `pydantic_ai_slim/pydantic_ai/tools.py:682` |
| `ToolDefinition.kind` field with docstring per variant | `kind: ToolKind = field(default='function')` | `pydantic_ai_slim/pydantic_ai/tools.py:723-732` |
| `kind='unapproved'` stamped from `requires_approval=True` | `kind='unapproved' if self.requires_approval else 'function'` | `pydantic_ai_slim/pydantic_ai/tools.py:649` |
| `ToolDefinition.defer` derived property | `return self.kind in ('external', 'unapproved')` | `pydantic_ai_slim/pydantic_ai/tools.py:875-881` |
| Approval-required exception class | `class ApprovalRequired(Exception)` with optional `metadata` | `pydantic_ai_slim/pydantic_ai/exceptions.py:98-114` |
| `CallDeferred` exception class (sibling kind) | `class CallDeferred(Exception)` for external execution | `pydantic_ai_slim/pydantic_ai/exceptions.py:80-96` |
| Static `requires_approval` on `Tool.__init__` | `requires_approval: bool = False` parameter and assignment | `pydantic_ai_slim/pydantic_ai/tools.py:455`, `pydantic_ai_slim/pydantic_ai/tools.py:482`, `pydantic_ai_slim/pydantic_ai/tools.py:546-576` |
| `requires_approval` on `@agent.tool` / `@agent.tool_plain` | Parameter plumbed into `FunctionToolset.add_function` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2200`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2222`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2279`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2335`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2357`, `pydantic_ai_slim/pydantic_ai/agent/__init__.py:2415` |
| `requires_approval` on `FunctionToolset` | Constructor + decorator accept the flag; toolset default flows to `Tool(requires_approval=...)` | `pydantic_ai_slim/pydantic_ai/toolsets/function.py:72-127`, `pydantic_ai_slim/pydantic_ai/toolsets/function.py:163-249`, `pydantic_ai_slim/pydantic_ai/toolsets/function.py:475-580` |
| `requires_approval` on `Capability` (`tool` / `tool_plain`) | Mirrors `Agent` decorator signatures | `pydantic_ai_slim/pydantic_ai/capabilities/capability.py:140-264` |
| Wrapper-toolset factory `AbstractToolset.approval_required` | Default predicate `lambda ctx, tool_def, tool_args: True` (every call requires approval) | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:230-242` |
| `ApprovalRequiredToolset.call_tool` enforcement | `if not ctx.tool_call_approved and self.approval_required_func(...): raise ApprovalRequired` | `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-32` |
| `RunContext.tool_call_approved` (state visible to runtime) | `tool_call_approved: bool = False` | `pydantic_ai_slim/pydantic_ai/_run_context.py:80` |
| `RunContext.tool_call_metadata` (per-call metadata visible at execution) | `tool_call_metadata: Any = None` | `pydantic_ai_slim/pydantic_ai/_run_context.py:82` |
| `_build_tool_context` stamping `tool_call_approved`/`tool_call_metadata` | `replace(self.ctx, ..., tool_call_approved=approved, tool_call_metadata=metadata, ...)` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:193-213` |
| `ToolManager.validate_tool_call(approved=..., metadata=...)` | Validation re-runs with approval context after deferred re-entry | `pydantic_ai_slim/pydantic_ai/tool_manager.py:419-455` |
| `ToolManager.handle_call` (one-shot validate+execute+resolve) | Surfaces `ToolDenied` as a return value, re-validates `ToolApproved.override_args` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:771-828` |
| `_resolve_single_deferred` (inline approval path) | Builds `DeferredToolRequests(approvals=[call])` and re-runs `validate_tool_call(approved=True)` | `pydantic_ai_slim/pydantic_ai/tool_manager.py:847-919` |
| Agent-graph classification of `ApprovalRequired` to `'unapproved'` kind | `except exceptions.ApprovalRequired as e: deferred_calls_by_index[index] = 'unapproved'` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1889-1894` |
| Agent-graph batches deferred calls only when results are supplied | `calls_to_run.extend(tool_calls_by_kind['unapproved'])` inside `if tool_call_results is not None:` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1682-1698` |
| Deferred-call routing when no results supplied | `calls = [*tool_calls_by_kind['external'], *tool_calls_by_kind['unapproved']]` builds the `DeferredToolRequests` output | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1749-1776` |
| Error if model calls unapproved tool without handler and no `DeferredToolRequests` output type | Raises `UserError` with explicit remediation message | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1850-1855` |
| `ToolDenied` → `ToolReturnPart(outcome='denied')` | `_call_tool` branch on `isinstance(tool_call_result, ToolDenied)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2003-2009` |
| Per-call metadata flows from `DeferredToolResults.metadata` to tool body | `metadata = tool_call_metadata.get(call.tool_call_id)` then `validate_tool_call(..., metadata=metadata)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1541`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1725` |
| `DeferredToolRequests` output object | `calls`, `approvals`, `metadata` fields; `build_results()` and `remaining()` helpers | `pydantic_ai_slim/pydantic_ai/tools.py:256-326` |
| `DeferredToolResults` input object with override args | `approvals: dict[str, bool | DeferredToolApprovalResult]`; `to_tool_call_results()` normalizes bool→`ToolApproved`/`ToolDenied` | `pydantic_ai_slim/pydantic_ai/tools.py:378-421` |
| `ToolApproved` (with `override_args`) | `kind: Literal['tool-approved'] = 'tool-approved'` discriminator | `pydantic_ai_slim/pydantic_ai/tools.py:329-336` |
| `ToolDenied` (with custom message) | `kind: Literal['tool-denied'] = 'tool-denied'`; default `'The tool call was denied.'` | `pydantic_ai_slim/pydantic_ai/tools.py:339-348` |
| `DeferredToolApprovalResult` discriminated union | `Annotated[ToolApproved | ToolDenied, Discriminator('kind')]` | `pydantic_ai_slim/pydantic_ai/tools.py:365` |
| `DeferredToolRequests.build_results(approve_all=True)` convenience | Auto-approves any not explicitly listed | `pydantic_ai_slim/pydantic_ai/tools.py:274-316` |
| `HandleDeferredToolCalls` capability (inline path) | `handler` callable receives `RunContext` and `DeferredToolRequests`, returns `DeferredToolResults` | `pydantic_ai_slim/pydantic_ai/capabilities/deferred_tool_handler.py:15-75` |
| `AbstractCapability.handle_deferred_tool_calls` hook (default returns `None`) | All capabilities can opt in to inline resolution | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:923-947` |
| Handler accumulating across capabilities | `CombinedCapability.handle_deferred_tool_calls` accumulates results and recurses on `remaining` | `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:660-682` |
| Handler hooks via `Hooks` capability | `@hooks.on.deferred_tool_calls` resolves inline; accumulator mirrors `CombinedCapability` | `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:699-707`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:1243-1262` |
| Hook tool-name filter (`tools=['dangerous']`) | `_ToolHookEntry.tools: frozenset[str]` filtered in `_filter_tool_entries` | `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:86-89`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:259-265` |
| `SetToolMetadata` capability merges metadata onto selected tools | Uses `ToolSelector` (name, sequence, dict, or predicate) | `pydantic_ai_slim/pydantic_ai/capabilities/set_tool_metadata.py:16-56` |
| `SetMetadataToolset` (toolset-level metadata merge) | Uses `PreparedToolset` | `pydantic_ai_slim/pydantic_ai/toolsets/set_metadata.py:13-28` |
| `ToolSelector` union | `Literal['all'] | Sequence[str] | dict[str, Any] | ToolSelectorFunc` | `pydantic_ai_slim/pydantic_ai/tools.py:168-183` |
| `FilteredToolset` blocks tools that don't match predicate | Filtering in `get_tools`; filtered tools are not in `ToolManager.tools` and so cannot execute | `pydantic_ai_slim/pydantic_ai/toolsets/filtered.py:14-32` |
| `ExternalToolset` enforces `kind='external'` | `replace(tool_def, kind='external')`; `call_tool` raises `NotImplementedError` | `pydantic_ai_slim/pydantic_ai/toolsets/external.py:16-47` |
| FastMCP exposes `meta`, `annotations`, `output_schema` in metadata | `metadata={'meta': ..., 'annotations': ..., 'output_schema': ...}` | `pydantic_ai_slim/pydantic_ai/toolsets/fastmcp.py:231-235` |
| Sequential-mode interaction with approval | `requires_approval` tool called in sequence with `sequential=True` tools | `tests/test_agent.py:8406-8471` |
| `test_tool_raises_approval_required` — basic approval roundtrip | `raise ApprovalRequired` then resume with `DeferredToolResults` | `tests/test_tools.py:1517-1594` |
| `test_approval_required_with_metadata` — metadata roundtrip | `raise ApprovalRequired(metadata={'reason': ..., 'cost_usd': ...})` flows into `DeferredToolRequests.metadata` | `tests/test_tools.py:1667-1712` |
| `test_approval_required_without_metadata` — backward compat | Empty metadata dict still produced | `tests/test_tools.py:1732-1770` |
| `test_approval_required_toolset` — `approval_required_func` predicate | `toolset.approval_required(lambda ctx, tool_def, tool_args: tool_def.name == 'foo')` only gates `foo` | `tests/test_tools.py:2374-2534` |
| `test_approval_required_with_user_prompt` — approval + new prompt | Verifies `DeferredToolResults` + `user_prompt` resume | `tests/test_tools.py:1597-1647` |
| `test_run_with_unapproved_tool_call_in_history` | History replay surfaces unapproved tool as `DeferredToolRequests` without calling tool | `tests/test_agent.py:9885-9908` |
| `test_tool_requires_approval_no_output_type` — runtime error path | Adding `requires_approval=True` without `DeferredToolRequests` output type is allowed at construction; error fires only when called | `tests/test_agent.py:9287-9296` |
| `test_deferred_tool_handler_approve` — `HandleDeferredToolCalls` inline | Handler `True`s every approval request and run completes in one call | `tests/test_capabilities.py:18964-18986` |
| `test_deferred_tool_handler_deny` — inline denial | Handler returns `ToolDenied('Not allowed.')`; run completes with denial message | `tests/test_capabilities.py:19029-19060` |
| `test_user_prompt_with_deferred_tool_results` — second-run approval flow | Verifies `message_history` + `deferred_tool_results` carries approval to second run | `tests/test_agent.py:9184-9284` |
| `test_wrapper_toolsets_delegate_instructions` — `ApprovalRequiredToolset` chain | Verifies wrapper delegation including `approval_required()` | `tests/test_toolsets.py:1486-1519` |
| Streaming approval path test | `test_tool_raises_approval_required` exercises `agent.run_stream` | `tests/test_streaming.py:3541-3617` |
| `agent.iter` approval pause test | `test_tool_raises_call_deferred_approval_required_iter` exercises iter-based pause | `tests/test_streaming.py:3700-3761` |
| Logfire instrumentation for deferral | Captures approval/deferral telemetry | `tests/test_logfire.py:3573`, `tests/test_logfire.py:3682` |
| Temporal durable wrapper preserves approval semantics | `_ApprovalRequired` dataclass with `kind: Literal['approval_required'] = 'approval_required'` discriminator | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:40-81` |
| Durable run context exposes `tool_call_approved` field | `tool_call_approved` listed among serializable fields | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:20`, `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:56` |
| Prefect durable cache policy preserves `tool_call_approved` | Cache key includes `tool_call_approved` | `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:37-38` |
| AG-UI interrupt lifecycle for tool approval | Inbound/outbound interrupt payloads converted to `ToolApproved`/`ToolDenied` (deny-by-default) | `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_interrupt.py:69-132`, `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_adapter.py:319-342` |
| Vercel AI SDK v6 approval streaming | `iter_tool_approval_responses` extracts `DeferredToolResults.approvals`; emits `approval-requested`/`approval-responded` events | `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_adapter.py:133-248`, `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_adapter.py:621-717` |
| `toolsets/AGENTS.md` architectural guideline | "Prefer wrapper toolsets for cross-cutting behavior such as filtering, prefixing, approval, deferral, metadata…" | `pydantic_ai_slim/pydantic_ai/toolsets/AGENTS.md:5` |
| Slim AGENTS guideline #987 | "Extend `WrapperToolset` for cross-cutting toolset behavior — don't modify base classes or individual toolset implementations" | `pydantic_ai_slim/pydantic_ai/AGENTS.md:18-20` |

## Answers to Dimension Questions

### 1. Are tools risk-classified?

Partially. Pydantic AI uses a single **invocation-semantics** discriminator (`ToolKind = Literal['function', 'output', 'external', 'unapproved']`) at `pydantic_ai_slim/pydantic_ai/tools.py:682` rather than a risk taxonomy. There is no `risk_level`, `side_effects`, `permission_scope`, or similar field. Classification into "needs approval" is binary (`requires_approval=True` or `raise ApprovalRequired`). Tools can carry **opaque metadata** (`metadata: dict[str, Any] | None`) and the framework provides `ToolSelector` (`pydantic_ai_slim/pydantic_ai/tools.py:168-183`) plus `SetToolMetadata` (`pydantic_ai_slim/pydantic_ai/capabilities/set_tool_metadata.py:16-56`) so users can layer their own risk classification on top — but no built-in taxonomy exists.

### 2. Are permissions enforced?

Yes, with two enforcement modes:

- **Static**: `requires_approval=True` on any tool decorator or `FunctionToolset.add_function`/`Tool(...)` constructor sets `kind='unapproved'` (`pydantic_ai_slim/pydantic_ai/tools.py:649`); the agent graph routes the call to `deferred_calls['unapproved']` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1749-1776`) and never invokes the tool body without a prior `DeferredToolResults`.
- **Dynamic**: `ApprovalRequiredToolset.call_tool` (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-32`) checks the predicate and `ctx.tool_call_approved` before calling `super().call_tool`.

A model-emitted `ToolCallPart` for an unapproved tool never reaches the tool body unless `tool_call_approved=True` (set by `ToolManager._build_tool_context:tool_manager.py:204-213` when re-validating after approval).

### 3. Can users approve selectively?

Yes. Approval is **per-call, per-tool-call-id**:

- `DeferredToolResults.approvals` is a `dict[str, bool | DeferredToolApprovalResult]` keyed by `tool_call_id` (`pydantic_ai_slim/pydantic_ai/tools.py:389-392`). The caller can approve some calls and deny others in the same batch (`tests/test_tools.py:2459-2462`).
- `approval_required_func(ctx, tool_def, validated_args)` is called per call and may inspect args, deps, or run context (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:232`).
- The tool body itself can `raise ApprovalRequired` based on args or context (`pydantic_ai_slim/pydantic_ai/exceptions.py:98-114`, `docs/deferred-tools.md:120-124`).
- `ToolApproved.override_args` lets the approver edit the validated arguments before re-execution (`pydantic_ai_slim/pydantic_ai/tools.py:333`).

### 4. Are approvals persisted?

Yes — **as part of the conversation log**, not as a separate policy store:

- `DeferredToolRequests(approvals=[ToolCallPart, ...], metadata={tool_call_id: {...}})` is the run's output when no handler resolves inline (`pydantic_ai_slim/pydantic_ai/tools.py:256-326`).
- Denials surface as `ToolReturnPart(outcome='denied', content=<denied message>)` in the next model request (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2003-2009`, asserted in `tests/test_agent.py:9038-9044`).
- Approved re-execution produces a regular `ToolReturnPart` (`tests/test_agent.py:9032-9037`).
- The agent graph persists the message history; `message_history` plus `DeferredToolResults` is the documented second-run input (`docs/deferred-tools.md:101-103`).
- Durable engines (Temporal, Prefect) carry `tool_call_approved` and `tool_call_metadata` through serialization (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:20-56`, `pydantic_ai_slim/pydantic_ai/durable_exec/prefect/_cache_policies.py:37-38`).

There is no separate approval history table outside the conversation log; replay-based durability relies on the message stream.

### 5. Can policy block a model-requested tool?

**Yes, if the developer opted in**:

- A blanket deny is not built in, but a developer can wrap any toolset in `FilteredToolset` with a predicate that always returns `False` for high-risk tools — the framework then omits them from `ToolManager.tools`, so the agent graph raises `unexpected_model_behavior` rather than executing (`pydantic_ai_slim/pydantic_ai/toolsets/filtered.py:14-32`).
- `SetToolMetadata` with a `ToolSelector` can stamp a metadata convention onto selected tools; a subsequent `ApprovalRequiredToolset(predicate=lambda ctx, td, args: td.metadata.get('destructive'))` can then gate on it.
- Hooks (`before_tool_validate`, `before_tool_execute`, `wrap_tool_execute`) can raise `ModelRetry` or other exceptions to abort execution per `ToolSelector` (`pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:325`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:259-265`).
- The `approval_required_func` predicate can dynamically decide per call and raise `ApprovalRequired`, which the framework treats as a hard halt (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:26-32`).

**Can the runtime stop a high-risk tool even if the model asks for it?** Yes, via the static `requires_approval=True` flag, via `ApprovalRequiredToolset`, via `FilteredToolset` excluding the tool entirely, via hooks returning/raising from `wrap_tool_execute`/`before_tool_execute`, and via the tool body itself raising `ApprovalRequired` (`tests/test_tools.py:1517-1594`, `tests/test_tools.py:2374-2534`, `tests/test_capabilities.py:18964-19060`). What the framework does *not* ship is a built-in risk taxonomy that classifies a tool as "destructive" automatically — classification is up to the developer at registration time.

## Architectural Decisions

1. **Single `kind` discriminator per `ToolDefinition`** (`pydantic_ai_slim/pydantic_ai/tools.py:723-732`) — every tool def carries exactly one of `function`, `output`, `external`, `unapproved`. The graph switches behavior by `kind`, which makes the routing decision local to one field rather than scattered across multiple flags. The cost is that no tool can simultaneously be "external" and "approved" — but that's a reasonable semantic simplification.

2. **Approval is an invocation property, not a metadata flag** (`pydantic_ai_slim/pydantic_ai/tools.py:649`): `requires_approval=True` flips `kind` from `function` to `unapproved`. The model still sees the tool in the request (it's part of the schema), and the runtime refuses to execute without prior `DeferredToolResults`. The framework surfaces `DeferredToolRequests.approvals` to the caller, not to the model.

3. **Two paths for approval** (`docs/deferred-tools.md:14-21`): **inline** (`HandleDeferredToolCalls` capability) and **stop-the-world** (`DeferredToolRequests` as `output_type`). Both compile to the same `DeferredToolResults`; the inline path is for handlers in the same process, the stop-the-world path is for external collectors (e.g., a UI adapter).

4. **Capability accumulation** (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:660-682`): multiple capabilities may each resolve a subset of `DeferredToolRequests`. The chain passes `remaining` requests forward and accumulates the resolved ones. This lets an approval gate layer above a more permissive auto-approve without coupling.

5. **Conditional approval inside the tool body** (`pydantic_ai_slim/pydantic_ai/exceptions.py:98-114`): `raise ApprovalRequired` from inside the tool lets the body decide approval based on the *actual validated args*, not just the tool name. This is the documented path for "delete this file is fine, but delete `.env` is not" — `docs/deferred-tools.md:120-124`. The re-execution path is gated by `ctx.tool_call_approved` (`pydantic_ai_slim/pydantic_ai/_run_context.py:80`), set true only when re-validating after approval (`pydantic_ai_slim/pydantic_ai/tool_manager.py:204-213`).

6. **Override args on approval** (`pydantic_ai_slim/pydantic_ai/tools.py:329-336`): `ToolApproved.override_args: dict[str, Any] | None` lets an approver replace the validated arguments before re-execution. This is a deliberate design — approvals are not pure yes/no gates; they can edit the request. Tests assert this round-trip works through `DeferredToolResults.to_tool_call_results()` (`pydantic_ai_slim/pydantic_ai/tools.py:402-421`).

7. **Wrapper toolsets over concrete-toolset changes** (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:14-32`, `pydantic_ai_slim/pydantic_ai/AGENTS.md:18-20`, `pydantic_ai_slim/pydantic_ai/toolsets/AGENTS.md:5`): `ApprovalRequiredToolset` is a `WrapperToolset` that intercepts `call_tool` rather than modifying each leaf toolset. The project guidance explicitly forbids adding approval behavior to concrete toolsets.

8. **`tool_def.metadata` is opaque** (`pydantic_ai_slim/pydantic_ai/tools.py:734-738`): metadata "is not sent to the model, but can be used for filtering and tool behavior customization." This deliberately keeps policy/risk data off the wire and lets users layer their own classification — at the cost of no built-in convention.

## Notable Patterns

- **Predicate-shaped wrappers**: `ApprovalRequiredToolset`, `FilteredToolset`, `SetMetadataToolset`, `PreparedToolset` all wrap a child toolset and receive the tool defs (or call context) to apply a per-tool decision. The pattern composes: e.g., `mcp.approval_required(lambda ctx, td, args: td.metadata.get('destructive'))` could gate based on a metadata convention.
- **Discriminated union for approval results**: `DeferredToolApprovalResult = Annotated[ToolApproved | ToolDenied, Discriminator('kind')]` (`pydantic_ai_slim/pydantic_ai/tools.py:365`) — Pydantic-driven discriminator enables round-trip serialization via `DeferredToolResults` Pydantic model (`tests/test_tools.py:2536-2590`).
- **`kind: 'unapproved'` discriminator + `defer` property**: `ToolDefinition.defer` returns `True` for both `'external'` and `'unapproved'` (`pydantic_ai_slim/pydantic_ai/tools.py:875-881`), letting callers treat deferred calls uniformly while the agent graph distinguishes them by exact kind.
- **Per-tool-call-id keying**: Every approval/denial is keyed by `tool_call_id`, not by tool name. A single run with two `delete_file` calls can approve one and deny the other (`tests/test_tools.py:2459-2517`).
- **`tool_call_metadata` channel**: `DeferredToolResults.metadata[tool_call_id]` flows through to the tool body as `RunContext.tool_call_metadata` (`pydantic_ai_slim/pydantic_ai/_run_context.py:82`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:204-213`). This is the channel for "the user approved this with rationale X" to reach the tool body.
- **Re-validation after approval** (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1816-1824`, `pydantic_ai_slim/pydantic_ai/tool_manager.py:910-919`): when `override_args` is supplied, the framework re-validates with `approved=True` and `metadata=call_metadata` — override args cannot bypass the JSON schema.
- **Hook tool-name filter** (`pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:259-265`, `pydantic_ai_slim/pydantic_ai/capabilities/hooks.py:86-89`): `@hooks.on.before_tool_execute(tools=['dangerous'])` registers a `_ToolHookEntry(tools=frozenset({'dangerous'}))`; the filter is `call.tool_name not in entry.tools`. This is a name-based allowlist, not a metadata-based one.
- **Deny-by-default inbound resume** (`pydantic_ai_slim/pydantic_ai/ui/ag_ui/_interrupt.py:104-132`): the AG-UI adapter documents and enforces "approval requires an explicit … payload" so a missing or hostile client cannot accidentally execute a tool that requires human approval.

## Tradeoffs

- **No risk taxonomy** in exchange for a small surface: Pydantic AI treats approval as the only risk class the framework knows about. Users wanting read-only vs. write vs. network vs. money must layer their own metadata convention via `SetToolMetadata` + a hook. This keeps the core simple but pushes policy to the user.
- **Model still sees unapproved tool** (`pydantic_ai_slim/pydantic_ai/tools.py:723-732`): the tool appears in the model's tool list with `kind='unapproved'`. This avoids confusing the model with "where did the tool go?" but means the model can attempt to call tools that the runtime will always defer.
- **`requires_approval` is opt-in per tool** (`pydantic_ai_slim/pydantic_ai/tools.py:455-576`): forgetting to set it means the tool runs unguarded. There is no "default deny" or "all tools require approval unless explicitly marked safe" mode. Users wanting defense-in-depth must wrap the agent in `ApprovalRequiredToolset` with a predicate that returns `True` for every tool.
- **Override args re-validate but don't re-trigger approval gate** (`pydantic_ai_slim/pydantic_ai/tool_manager.py:910-919`): once an approval is granted with `override_args`, the call goes straight to `execute_tool_call` without re-checking `approval_required_func`. If the override materially changes risk, the predicate is not re-evaluated. Mitigation: write `approval_required_func` to inspect `ctx.tool_call_metadata` or use a stricter validator instead of relying on override args to be safe.
- **Inline + stop-the-world flows compose** (`docs/deferred-tools.md:85`) — but the bubble-up behavior depends on `output_type` including `DeferredToolRequests`. Without that in the output type, an unhandled approval raises `UserError` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1850-1855`).
- **Per-tool-call metadata is free-form `dict[str, Any]`** (`pydantic_ai_slim/pydantic_ai/tools.py:271-272`): there is no schema validation that the metadata for a `requires_approval=True` tool follows a convention. Users can layer Pydantic validation in their handler if they want.
- **Capability chain is order-sensitive** (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:669-682`): the first capability to resolve a call "wins" for that call. If a strict policy and a permissive policy coexist, ordering matters; the docs do not give explicit ordering semantics beyond "accumulation".
- **MCP provider-side approval disabled by default** (`pydantic_ai_slim/pydantic_ai/models/openai.py:2536`): `mcp_tool['require_approval'] = 'never'` is hardcoded for the OpenAI Responses MCP server tool wrapper, deferring all approval to Pydantic AI's framework-level gate rather than the provider. This is documented as "Pydantic AI doesn't yet support McpApprovalRequest" (`pydantic_ai_slim/pydantic_ai/models/openai.py:2189`).

## Failure Modes / Edge Cases

- **Model calls unapproved tool, no `DeferredToolRequests` in `output_type`, no inline handler** → raises `UserError` with remediation hint (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1850-1855`). Documented behavior; surfaced in `tests/test_agent.py:9287-9296`.
- **Tool body re-raises `ApprovalRequired` after approval** (`pydantic_ai_slim/pydantic_ai/tool_manager.py:847-919`): the call stays in `remaining.approvals` because the framework treats the second raise as a fresh deferral. Tests assert this is intentional.
- **Tool body re-raises `CallDeferred` after approval** (`pydantic_ai_slim/pydantic_ai/tool_manager.py:825-828`): propagates as a `call`, not an `approval`, so the deferred kind changes.
- **Approved tool re-validation failure with `override_args`** (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1819-1824`): defensive try/except asserts retries were exhausted; coverage is `pragma: no cover` so this path is unlikely in practice.
- **Sequential-mode interaction with approval** (`tests/test_agent.py:8406-8471`): unapproved tools execute in sequence with `sequential=True` siblings when `DeferredToolResults` are supplied; verifies the `parallel_tool_call_execution_mode('sequential')` setting composes with approval.
- **Streaming run approval pause** (`tests/test_streaming.py:3541-3617`): `agent.run_stream` correctly defers unapproved tools and the second stream-with-`deferred_tool_results` produces `ToolReturnPart`. Asserts the streaming path uses the same kind-routing.
- **`agent.iter` approval pause** (`tests/test_streaming.py:3700-3761`): the iter API exposes the deferred node so the caller can collect approvals before calling `AgentRun.next()`.
- **MCP toolset exposure** (`pydantic_ai_slim/pydantic_ai/toolsets/fastmcp.py:231-235`): MCP tools expose `meta`, `annotations`, `output_schema` in metadata, so `SetToolMetadata` + a metadata-based `approval_required_func` can gate MCP tools by upstream-declared annotations — but the MCP layer itself does not enforce approval.
- **`external` kind cannot be called** (`pydantic_ai_slim/pydantic_ai/toolsets/external.py:44-47`): `call_tool` raises `NotImplementedError`; the graph short-circuits on `kind='external'` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1749`). A developer using `ExternalToolset` and providing results via `DeferredToolResults.calls` would correctly route; one bypassing `DeferredToolResults` would hit `NotImplementedError`.
- **Predicate returning `True` for every tool by default** (`pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:22-24`): the default `approval_required_func` is `lambda ctx, td, args: True`, so an `approval_required()` wrapper without a predicate gates every tool. This is a safe default; a permissive wrapper would need an explicit predicate.
- **Approver can rewrite `override_args`** (`pydantic_ai_slim/pydantic_ai/tools.py:333`): `ToolApproved.override_args` allows the approver to substitute arguments. Re-validation enforces the JSON schema (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1816-1824`), but there is no semantic check (e.g., "did the override materially change the risk class?"). This is documented and tests verify schema enforcement only.
- **Durable-engine serialization** (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_toolset.py:40-81`): `_ApprovalRequired` dataclass is serialized across activity boundaries with the same `kind: Literal['approval_required']` discriminator. Reconstructing on the worker side requires `_wrap_call_tool_result` to translate between `ApprovalRequired` (slim) and `_ApprovalRequired` (durable). This is a documented translation boundary.

## Future Considerations

- A built-in `risk_class` enum (`read_only`, `write`, `delete`, `network`, `secrets`, `money`) on `ToolDefinition` would let `ApprovalRequiredToolset`'s default predicate key off it, removing the need for users to attach metadata conventions. Currently no such enum exists; the closest analogue is the MCP `annotations` field, but it's only populated for MCP-server tools.
- A **policy registry** at the agent level (e.g., a `PolicyEnforcementCapability`) that lists dangerous tools by name or metadata and forces `requires_approval=True` for them — currently each tool must opt in individually. The project guideline to "prefer a capability over a new `Agent` constructor kwarg" (`pydantic_ai_slim/pydantic_ai/capabilities/AGENTS.md`) would push this to a capability.
- **Re-evaluation of `approval_required_func` on override_args** would close the "override materially changes risk" gap. Currently the predicate runs once before approval; after `ToolApproved.override_args`, the body re-runs without the predicate. A second pass would harden the override path.
- **Typed metadata schemas** (`Metadata` TypedDict on `ToolDefinition`) — currently `metadata: dict[str, Any] | None` is free-form. A schema would let the framework validate that e.g., `risk_class` is one of an allowed enum.
- **Approval TTL / replay protection** — currently an approval is forever; there's no mechanism to require re-approval after a delay or after a model context change. The conversation-log approach allows replay, which may or may not be desired depending on policy.
- **Audit logging** beyond OTel — `tests/test_logfire.py:3573,3682` show that OTel captures deferral/approval events, but there is no first-class audit log separate from the message stream. A dedicated `ApprovalAudit` capability could produce a structured audit trail outside the conversation.
- **Capability to enforce approval on a *metadata key*** — `SetToolMetadata` can stamp metadata, but the framework doesn't ship a capability that takes e.g. `risk_class='destructive'` metadata and converts the tool to `kind='unapproved'`. Users can build this with `PreparedToolset`, but a built-in would be a clean primitive.
- **OpenAI `McpApprovalRequest` integration** — `pydantic_ai_slim/pydantic_ai/models/openai.py:2189` documents that the OpenAI Responses provider's own approval flow is not yet wired through Pydantic AI's framework-level approval; the framework hardcodes `require_approval='never'` for the MCP tool (`pydantic_ai_slim/pydantic_ai/models/openai.py:2536`) and relies on its own gate instead.

## Questions / Gaps

- **How does approval interact with retries?** When `requires_approval=True` and validation fails, `tests/test_tools.py:2358-2371` shows that retries on the validation side are not coupled to approval retries. The `validate_tool_call` flow validates and then `execute_tool_call` runs, with `ApprovalRequiredToolset.call_tool` wrapping `super().call_tool` (i.e., wrapping execution only, not validation). This means the schema is validated even on unapproved calls. The relative ordering of validation retries vs. approval raises is not documented as an explicit invariant.
- **Is there a guard against `requires_approval=True` being silently dropped after a `PreparedToolset` runs?** `PreparedToolset` (`pydantic_ai_slim/pydantic_ai/toolsets/prepared.py:14-41`) lets a user's `prepare` function return a new `ToolDefinition`. If a misbehaving prepare function replaces `kind='unapproved'` with `'function'`, the agent graph would execute the tool unguarded. No clear evidence found that the framework prevents this; the prepare contract allows arbitrary modification.
- **What happens when `approval_required_func` itself raises?** `pydantic_ai_slim/pydantic_ai/toolsets/approval_required.py:29` invokes `self.approval_required_func(...)`. If the predicate raises a non-`ApprovalRequired` exception, it would propagate through `call_tool` and be handled by `ToolManager._check_max_retries` (`pydantic_ai_slim/pydantic_ai/tool_manager.py:178-182`). No clear evidence found in tests for this exact failure mode.
- **Can a parent agent toolset override a child's `requires_approval`?** Composition order matters (capabilities + wrapper toolsets), but the framework does not document an explicit override semantics. No clear evidence found.
- **Is `tool_def.metadata` audited across re-runs?** `pydantic_ai_slim/pydantic_ai/tools.py:734-738` says metadata is "not sent to the model" but does not assert it is or isn't persisted across runs. The replay path uses `message_history`, not metadata persistence. No clear evidence found.

---

Generated by `04.05-tool-permissions-and-approval-metadata` against `pydantic-ai`.