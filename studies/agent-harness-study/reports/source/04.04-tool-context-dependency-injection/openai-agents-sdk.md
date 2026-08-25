# Source Analysis: openai-agents-sdk

## 04.04 — Tool Context and Dependency Injection

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+; dataclasses, Pydantic-based JSON schema, asyncio |
| Analyzed | 2026-07-27 |

## Summary

The OpenAI Agents SDK exposes an explicit, typed runtime-context model rather than
relying on global state. Every run carries a user-supplied app object wrapped in
`RunContextWrapper[TContext]` (`src/agents/run_context.py:44`). Function tools
receive either that wrapper or its richer subclass `ToolContext` (`src/agents/tool_context.py:36`)
as their first injected argument via the `@function_tool` decorator
(`src/agents/tool.py:1899`, `src/agents/function_schema.py:285-314`). Tools can
opt into the lighter or richer type and `_get_function_tool_invoke_context`
(`src/agents/tool.py:1772`) inspects the wrapper's signature annotation to
choose which shape to pass, deliberately avoiding leakage of runtime-only
metadata when the wrapper declares the narrower `RunContextWrapper` contract.
Approval state is attached to the wrapper itself (`src/agents/run_context.py:30`,
`src/agents/run_context.py:178-211`), so permission gating runs alongside the
context. The design is well-typed, isolated from global state, and easy to
boot in tests thanks to a `FunctionTool`/`ToolContext` API that does not
require an `Agent` or `Runner`. The one real fragility point is that anything
the tool wants beyond the context wrapper (logger, secrets, store, cancel
token) must live on the user's `TContext` object, which means safe secret
handling is by convention (`docs/context.md:43`) rather than enforced.

## Rating

**8 / 10** — Clear model with typed wrappers, two contract shapes, explicit
permission state, broad test coverage, and a documented warning about secret
leakage through serialized state. Falls short of 9 only because the SDK does
not provide first-class logging, secret, or cancellation injection and the
`is_tool_approved` flow uses a string-keyed dict whose behaviour depends on
caller-supplied `tool_name` resolution helpers.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Base context wrapper class (typed, generic) | `RunContextWrapper[TContext]` dataclass with `eq=False` to keep hashing by identity | `src/agents/run_context.py:43-63` |
| Tool context subclass with tool-call metadata | `ToolContext(RunContextWrapper[TContext])` adds `tool_name`, `tool_call_id`, `tool_arguments`, `tool_call`, `tool_namespace`, `agent`, `run_config` | `src/agents/tool_context.py:35-58` |
| Tool context required-field assertion | `_assert_must_pass_tool_name/_tool_call_id/_tool_arguments` raise `ValueError` if defaults fire | `src/agents/tool_context.py:20-29` |
| Tool context constructor — keep v0.7 positional compat | `__init__` with `_MISSING` sentinel defaults; preserves positional/keyword compatibility | `src/agents/tool_context.py:60-107` |
| Context inheritance for nested agent-tool calls | `ToolContext.from_agent_context` copies base fields, inherits `agent`/`run_config`, restores tool-state scope | `src/agents/tool_context.py:114-173` |
| Function tool wrapper type | `ToolFunction = ToolFunctionWithoutContext | ToolFunctionWithContext | ToolFunctionWithToolContext` | `src/agents/tool.py:73-81` |
| `FunctionTool.on_invoke_tool` signature contract | `Callable[[ToolContext[Any], str], Awaitable[Any]]` | `src/agents/tool.py:395` |
| Decorator injects context when first arg matches `RunContextWrapper` or `ToolContext` | `function_schema` annotates `takes_context=True` and emits error on non-first usage | `src/agents/function_schema.py:289-314` |
| Decorator decides whether to await / run sync | `is_sync_function_tool` branches between `await the_func(ctx, *args, **kwargs)` and `asyncio.to_thread(...)` | `src/agents/tool.py:1967-2013` |
| Runtime chooses context shape by wrapper annotation | `_get_function_tool_invoke_context` returns `ToolContext` or a forked `RunContextWrapper`; uses `get_type_hints` to read `Annotated[...]` too | `src/agents/tool.py:1772-1803` |
| Fork helper for narrower wrapper | `RunContextWrapper._fork_with_tool_input` shares usage/approvals/turn_input but isolates `tool_input` | `src/agents/run_context.py:470-485` |
| Test: narrow `RunContextWrapper` wrapper receives forked base wrapper | `test_invoke_function_tool_passes_plain_run_context_when_requested` | `tests/test_tool_context.py:224-261` |
| Test: `ToolContext`-typing wrapper receives full object | `test_invoke_function_tool_preserves_tool_context_when_requested` | `tests/test_tool_context.py:264-296` |
| Test: substring user types not mistaken for context types | `test_invoke_function_tool_ignores_context_name_substrings_in_string_annotations` | `tests/test_tool_context.py:299-332` |
| Test: `Annotated[RunContextWrapper, ...]` collapses to plain wrapper | `test_invoke_function_tool_ignores_annotated_string_metadata_when_matching_context` | `tests/test_tool_context.py:335-371` |
| Approval policy state lives on the wrapper | `_ApprovalRecord` with permanent allow/deny or per-call lists; stored in `RunContextWrapper._approvals` | `src/agents/run_context.py:30-61`, `src/agents/run_context.py:171-211` |
| Public approval API on context wrapper | `approve_tool`, `reject_tool`, `get_approval_status`, `get_rejection_message`, `is_tool_approved` | `src/agents/run_context.py:355-445` |
| Approval keys resolve through shared lookup helper | `RunContextWrapper._resolve_approval_key/_keys` use `_tool_identity` for legacy/namespace aliasing | `src/agents/run_context.py:99-124`, `src/agents/_tool_identity.py:362-410` |
| Approval scope per call id demonstrated in tests | `test_run_context_scopes_approvals_to_call_ids` | `tests/test_run_context_wrapper.py:28-37` |
| Rejection messages stored and recalled per call | `test_run_context_stores_per_call_rejection_messages` | `tests/test_run_context_wrapper.py:64-72` |
| `ToolContext` required-field enforcement | `test_tool_context_requires_fields`, `test_tool_context_missing_defaults_raise` | `tests/test_tool_context.py:32-45` |
| Run normalizes user context into wrapper | `ensure_context_wrapper` accepts bare `TContext` or `RunContextWrapper` and wraps it | `src/agents/run_internal/agent_runner_helpers.py:291-297` |
| Run config propagated into `ToolContext` | `from_agent_context` keeps `agent` and `run_config` on the tool wrapper; set per-call by execution layer | `src/agents/tool_context.py:147-152`, `src/agents/run_internal/tool_execution.py:1580-1587` |
| `AgentHookContext` subclasses `RunContextWrapper` for hook handlers | Distinct subclass passed to agent hooks; `RunHooksBase.on_agent_start/end` callback signatures | `src/agents/run_context.py:489`, `src/agents/lifecycle.py:37-59` |
| Lifecyle hook docstring documents which tools get `ToolContext` | "For function-tool invocations, `context` is typically a `ToolContext` instance..." | `src/agents/lifecycle.py:77-82` |
| Guardrails use context-bearing dataclasses, not raw wrapper | `ToolInputGuardrailData.context: ToolContext[Any]`, agent | `src/agents/tool_guardrails.py:120-145` |
| Computer-tool creation respects run context | `ComputerCreate` callback receives `run_context`; per-context cache via `WeakKeyDictionary` | `src/agents/tool.py:331-356`, `src/agents/tool.py:752-813` |
| Computer lifecycle dispose via run context | `ComputerDispose` callback receives `run_context`, `computer`; driven by `dispose_resolved_computers(*, run_context=...)` | `src/agents/tool.py:337-345`, `src/agents/tool.py:816-842` |
| Shell/apply_patch/custom tool approval hooks receive `RunContextWrapper` | `ShellOnApprovalFunction`, `ApplyPatchOnApprovalFunction`, `CustomToolOnApprovalFunction` all take `RunContextWrapper[Any]` first | `src/agents/tool.py:907-956` |
| Handoff callbacks receive `RunContextWrapper` | `on_invoke_handoff: Callable[[RunContextWrapper[Any], str], Awaitable[TAgent]]` and `OnHandoffWithInput` | `src/agents/handoffs/__init__.py:38-116` |
| `is_enabled` callback receives run context & agent | `is_enabled: bool | Callable[[RunContextWrapper[Any], AgentBase], MaybeAwaitable[bool]]` | `src/agents/tool.py:412-415`, `src/agents/agent.py:516-518`, `src/agents/handoffs/__init__.py:153-234` |
| Custom tool executor receives `ToolContext` | `CustomToolExecutor = Callable[[ToolContext[Any], str], MaybeAwaitable[Any]]`; applied per-tool | `src/agents/tool.py:184`, `src/agents/run_internal/tool_actions.py:636-687` |
| Local shell executor receives request dataclass incl. `ctx_wrapper` | `LocalShellCommandRequest`, `LocalShellExecutor` | `src/agents/tool.py:1004-1016` |
| Shell executor receives `ShellCommandRequest` with wrapper | `ShellExecutor = Callable[[ShellCommandRequest], MaybeAwaitable[str | ShellResult]]` where `ShellCommandRequest.ctx_wrapper: RunContextWrapper[Any]` | `src/agents/tool.py:1188-1196` |
| Approval-related callbacks in shell/apply_patch/MCP get run context | `ShellApprovalFunction`, `ApplyPatchApprovalFunction`, `MCPToolApprovalFunction` all carry `RunContextWrapper` or contextual request object | `src/agents/tool.py:889-956` |
| `usage` lives on the wrapper for token accounting | `RunContextWrapper.usage: Usage`, thread-safe to all tools invoked in same run | `src/agents/run_context.py:55-58` |
| `turn_input` lives on the wrapper for tool-call resume | `RunContextWrapper.turn_input: list[TResponseInputItem]` shared across nested tool paths | `src/agents/run_context.py:60-63` |
| Provider data redaction warning in docs | "Avoid putting secrets in `RunContextWrapper.context` if you intend to persist or transmit serialized state" | `docs/context.md:43` |
| HITL docstring on persistent vs transient context | "treat `RunContextWrapper.context` as persisted data and avoid placing secrets there unless you..." | `docs/human_in_the_loop.md:200` |
| Tool-context dataclass is hashable-by-identity (preserved for dict/set keying) | `RunContextWrapper(eq=False)` pattern carried over to `ToolContext`; explicit test | `src/agents/tool_context.py:35`, `tests/test_tool_context.py:15-29` |
| Concrete test showing tool can run without booting app | `tests/test_run_context_wrapper.py:13-15` and `tests/test_tool_context.py` invoke `FunctionTool.on_invoke_tool` directly with a hand-built `ToolContext`; no `Agent`, `Runner`, or network call |
| Convenience factory for test contexts | `make_context_wrapper()` / `make_agent()` helpers | `tests/utils/hitl.py:438-461` |
| Field-inheritance test for derived context | `test_tool_context_from_tool_context_inherits_agent` / `_inherits_run_config` | `tests/test_tool_context.py:140-201` |

## Answers to Dimension Questions

1. **What context does a tool receive?** A tool wrapped with `@function_tool`
   (`src/agents/tool.py:1899`) whose first parameter is annotated
   `RunContextWrapper[TContext]` or `ToolContext[TContext]`
   (`src/agents/function_schema.py:297-314`) receives that wrapper. The wrapper
   carries: the user-supplied `context` object (`src/agents/run_context.py:52`),
   `usage` (`src/agents/run_context.py:55`), `turn_input`
   (`src/agents/run_context.py:60`), per-run approval state
   (`src/agents/run_context.py:61`), and `tool_input` for nested agent-as-tool
   runs (`src/agents/run_context.py:62`). `ToolContext`
   (`src/agents/tool_context.py:36`) extends the wrapper with
   `tool_name`/`tool_call_id`/`tool_arguments`, `tool_namespace`, `agent`, and
   `run_config` (`src/agents/tool_context.py:39-58`).

2. **Is context explicit or global?** Explicit. The wrapper is constructed
   per-run from the `context` argument passed to `Runner.run(...)` via
   `ensure_context_wrapper` (`src/agents/run_internal/agent_runner_helpers.py:291-297`),
   threaded through `context_wrapper=` parameters across the run loop
   (`src/agents/run.py:809`, `src/agents/run.py:1077`, `src/agents/run.py:1816`),
   and derived into `ToolContext` per tool call via
   `ToolContext.from_agent_context` (`src/agents/tool_context.py:114`). There
   are no module-level tool globals beyond the `logger`
   (`src/agents/logger.py:3`).

3. **Are secrets passed safely?** Secrets are the caller's responsibility.
   The wrapper only adds managed runtime fields; the `context` attribute is
   just whatever the user passes in (`src/agents/run_context.py:48-53`), and
   `docs/context.md:43` warns explicitly: "Avoid putting secrets in
   `RunContextWrapper.context` if you intend to persist or transmit
   serialized state." `RunState.to_json` round-trips the wrapper, so
   callers must store secrets elsewhere or live with them being serialized
   into the persisted job payload. There is no first-class secret-store
   injection; sensitive data access must be mediated through the user
   `TContext` object.

4. **Can tools be unit tested?** Yes, fully. `FunctionTool` exposes
   `on_invoke_tool` as a plain `Callable[[ToolContext[Any], str], Awaitable[Any]]`
   (`src/agents/tool.py:395`), and `invoke_function_tool`
   (`src/agents/tool.py:1806`) wraps the callable with timeout handling
   without booting any agent. `tests/test_tool_context.py:224-371` exercises
   `invoke_function_tool` against hand-built `FunctionTool` instances and a
   manually constructed `ToolContext`. `tests/test_function_tool.py` and
   `tests/test_function_tool_decorator.py` cover both narrow and wide
   wrapper shapes via the helper `make_context_wrapper()`
   (`tests/utils/hitl.py:438-440`).

5. **Can context enforce permissions?** Yes. `RunContextWrapper.approve_tool` /
   `reject_tool` (`src/agents/run_context.py:355-375`) record decisions on the
   wrapper's `_approvals` dict, and `get_approval_status`
   (`src/agents/run_context.py:377-445`) resolves per-call and global allow/deny
   keys through `get_function_tool_approval_keys`
   (`src/agents/_tool_identity.py:362-410`). The execution path checks this
   state before dispatching (`src/agents/run_internal/tool_execution.py:1651-1656`)
   and a tool's `needs_approval` callable can decide per-call
   (`src/agents/tool.py:426-433`). Tool-input guardrails and per-tool
   guardrail dataclasses (`src/agents/tool_guardrails.py:120-185`) extend the
   gating surface with allow / reject-content / raise-exception behaviors.

## Architectural Decisions

- **Two-tier wrapper inheritance (`RunContextWrapper` -> `ToolContext`).**
  `ToolContext` is declared with `@dataclass(eq=False)` and subclasses the
  base wrapper (`src/agents/tool_context.py:35-58`), so common operations
  (hashing, approval API, `fork_with_tool_input`) work uniformly. Function
  tools opt in by typing the parameter (`src/agents/function_schema.py:298`).
- **Annotations are the contract.** `_get_function_tool_invoke_context`
  resolves the wrapper annotation via `inspect.signature` plus
  `get_type_hints(..., include_extras=True)` (`src/agents/tool.py:1784-1797`)
  so any `Annotated[...]` decoration still routes correctly. Third-party
  wrappers declaring `RunContextWrapper` get a forked non-tool wrapper to
  avoid leaking runtime-only metadata (`src/agents/tool.py:1778-1802`).
- **Approval state attached, not bolted-on.** `RunContextWrapper._approvals`
  is a `dict[str, _ApprovalRecord]` (`src/agents/run_context.py:30-61`) that
  moves with the wrapper through handoffs and nesting, including the special
  case where `Agent.as_tool()` constructs a fresh `ToolContext`
  (`src/agents/agent.py:631-649`).
- **Tool guards share the same injection mechanism.** `ToolInputGuardrailData`
  carries `context: ToolContext[Any]` (`src/agents/tool_guardrails.py:120`),
  so guardrails have parity with tools; `ComputerCreate` /
  `ComputerDispose` are typed against `RunContextWrapper` to keep lifecycle
  hooks synced (`src/agents/tool.py:331-356`).
- **Constructor compat discipline.** `ToolContext.__init__` keeps the
  v0.7 positional order and uses a `_MISSING` sentinel so older
  `(context, usage, tool_name, tool_call_id, tool_arguments, ...)` call sites
  still work while new fields are appended at the end
  (`src/agents/tool_context.py:60-107`).

## Notable Patterns

- **Type-driven injection** — the function decorator introspects the
  first-parameter annotation once and only exposes `RunContextWrapper` or
  `ToolContext` as legal types (`src/agents/function_schema.py:289-314`).
  Anything else in the first position raises `UserError`
  (`src/agents/function_schema.py:311`).
- **Fork for narrower contracts** — `_fork_with_tool_input` shares
  `_approvals`, `usage`, `turn_input`, but fresh `tool_input`
  (`src/agents/run_context.py:470-477`), which keeps the safety properties
  of the original wrapper while giving third-party wrappers an isolated
  payload.
- **Generic state propagation** — `RunContextWrapper` is `Generic[TContext]`,
  repeated through `Agent[TContext]`, `Guardrail[TContext]`, hooks
  (`src/agents/lifecycle.py:13`), so type checkers can flag mismatches
  between an agent's `TContext` and any registered tool's first-parameter
  generic (`docs/context.md:18`).
- **Approve-alias derivation** — approval keys are computed from the tool's
  name/namespace plus optional bare-name aliasing
  (`src/agents/_tool_identity.py:266-410`) so `tool_namespace(...)` and
  deferred-loading variants hit the same approval record.
- **Per-context resource cache** — `ComputerTool` resolutions are stored in
  a `WeakKeyDictionary` keyed by both the tool and the run context
  (`src/agents/tool.py:752-761`), and `dispose_resolved_computers` cleans
  them up by run-context key (`src/agents/tool.py:816-842`).

## Tradeoffs

- **No first-class DI surface for logger/secrets/cancellation.** Tools only
  get the wrapper and whatever the user stuffed into `TContext`. The SDK
  deliberately avoids shipping a "well-known" dependency container, which is
  flexible but pushes every project to invent its own convention.
- **Serialized state can leak secrets.** Because `RunContextWrapper._approvals`
  and `context` are both serialized (`src/agents/run_state.py:2429-2448`),
  callers must police secret leakage themselves; the docs warn
  (`docs/context.md:43`) but the SDK does not enforce redaction.
- **Approval keyed by string.** All approval storage paths funnel through
  string keys derived from tool name and namespace
  (`src/agents/_tool_identity.py:362-410`). This avoids circular type
  imports but means a malformed `tool_name` can collide; the resolver has
  defensive fall-through logic that helped shape
  (`src/agents/run_context.py:76-145`).
- **`ToolContext` injection requires first-parameter conformance.** Putting
  `RunContextWrapper` or `ToolContext` anywhere other than first position
  raises `UserError` (`src/agents/function_schema.py:306-314`); users
  composing complex nested dataclasses may feel restricted.
- **`_fork_with_tool_input` allocates a new wrapper for narrow contracts.**
  Anything downstream that does `isinstance(ctx, ToolContext)` will see
  `False` even when it functionally is a tool call — this is a deliberate
  trade-off (`src/agents/tool.py:1772-1802`) but worth noting when writing
  hooks that inspect the runtime type.

## Failure Modes / Edge Cases

- **Missing tool-call fields raise immediately.** If `ToolContext.from_agent_context`
  is called without a `tool_call_id` it raises `ValueError("tool_call_id must
  be passed to ToolContext")` (`src/agents/tool_context.py:21`). Several
  tests document this contract (`tests/test_tool_context.py:32-45`).
- **Annotated metadata must not be inspected as a type.** A user can write
  `Annotated[RunContextWrapper[str], "ToolContext note"]` and the resolver
  must still match the context type. Two regression tests cover this
  (`tests/test_tool_context.py:299-371`).
- **Substring matching in user annotations.** Naive annotation matching could
  mistake a custom wrapper named `MyRunContextWrapper` for the SDK class.
  `_get_function_tool_invoke_context` relies on `get_origin(annotation) is
  RunContextWrapper/ToolContext` rather than a substring check
  (`src/agents/tool.py:1772-1803`), which is exactly what
  `test_invoke_function_tool_ignores_context_name_substrings_in_string_annotations`
  verifies.
- **Contextless tool input guardrails.** `ToolInputGuardrail.guardrail_function`
  gets `ToolContext` even if the tool would otherwise receive a plain wrapper
  (`src/agents/tool_guardrails.py:120-145`). Tests should be aware.
- **Approve alias resolution across namespaces.** When `allow_bare_name_alias`
  is set on an approval item but no namespace is present, the resolver still
  resolves per-call decisions correctly
  (`src/agents/run_context.py:117-124`, demonstrated by
  `test_run_context_scopes_approvals_to_call_ids`).
- **Per-call rejections don't persist across new calls.** Documented in
  `test_run_context_clears_rejection_message_after_approval`
  (`tests/test_run_context_wrapper.py:86-94`).
- **Computer-tool init failures are surfaced as `UserError`** when the
  initializer returns something that isn't a `Computer` / `AsyncComputer`
  (`src/agents/tool.py:806-807`).

## Future Considerations

- A typed/structured `TContext` constructor convention. The SDK could expose
  a helper protocol or `Context` base class that surfaces commonly needed
  fields (logger, secrets store, cancellation token) without naming a global
  service registry.
- Built-in secret redaction when serializing `RunState`. Currently this is
  only documented (`docs/context.md:43`); a runtime opt-in (e.g.,
  `RunConfig.redact_secret_keys`) would close the gap.
- Tool-context-specific cancellation tokens. The wrapper exposes `usage`
  and `turn_input` but no explicit per-tool cancel signal beyond
  `result.cancel(...)` (`src/agents/result.py:648-672`); per-tool cancel
  tokens could let long tools observe the same lifecycle without inheritance
  from `RunResultStreaming`.
- Move approval key storage from `dict[str, _ApprovalRecord]` to a typed
  dataclass with a small lookup-key enum. Today, every approval method
  reconstructs the key set independently
  (`src/agents/run_context.py:99-145`), which makes refactors error-prone.
- Standardize `ToolContext` first-parameter convention across custom tools
  (`CustomTool`, `ShellTool`, `ApplyPatchTool`) so a uniform introspection
  helper can iterate "all tool families" without special-casing.

## Questions / Gaps

- **No evidence found** that the SDK injects a logger into tools via the
  wrapper. Tools rely on `agents.logger` (`src/agents/logger.py:3`), which
  is module-level; the codebase offers no DI surface for a per-tool logger.
- **No evidence found** for a first-class cancellation token on the
  wrapper; async cancellation rides on `asyncio.wait_for`
  (`src/agents/tool.py:1818-1847`) but it isn't surfaced to user tool
  code.
- **No evidence found** that the wrapper enforces a max-context-size or
  serialization-redaction policy. The only secret warning is documented
  (`docs/context.md:43`, `docs/human_in_the_loop.md:200`) but unenforced.
- **No evidence found** for a documented stability window around
  `RunContextWrapper` field layout. Adding fields at the end of the
  dataclass preserves positional compatibility
  (`src/agents/tool_context.py:60-107`), but the contract isn't formalized
  in `AGENTS.md` beyond the general "preserve positional compatibility"
  note at lines 102-108.
