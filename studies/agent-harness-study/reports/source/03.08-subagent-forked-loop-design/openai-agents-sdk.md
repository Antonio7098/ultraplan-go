# Source Analysis: openai-agents-sdk

## 03.08 — Subagent and Forked-Loop Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, Pydantic v2), SDK of the `openai-agents` PyPI package |
| Analyzed | 2026-07-17 |

## Summary

OpenAI Agents SDK exposes **two distinct, first-class delegation mechanisms** with clearly different semantics, plus a third orchestration-by-code path that callers combine manually:

1. **Handoffs** — In-process control transfer where the active agent changes mid-turn. A `Handoff` (`src/agents/handoffs/__init__.py:94-184`) is exposed to the LLM as a tool named `transfer_to_<agent_name>` (`src/agents/handoffs/__init__.py:172-176`); when the model returns a tool call, the run loop calls `on_invoke_handoff` (`src/agents/handoffs/__init__.py:278-306`) which returns the next agent, the loop rebuilds `current_agent` and re-iterates (`src/agents/run.py:1011-1023`, `src/agents/run_internal/run_steps.py:154-158`). Multiple handoffs in a single turn are explicitly collapsed — only `run_handoffs[0]` is honored and the rest become `ToolCallOutputItem` placeholders returning `"Multiple handoffs detected, ignoring this one."` (`src/agents/run_internal/turn_resolution.py:426-438`).
2. **Agent-as-tool** — Sub-agent wrapped in a `FunctionTool` via `Agent.as_tool(...)` (`src/agents/agent.py:508-936`). When called, it transparently invokes a fresh `Runner.run(...)` or `Runner.run_streamed(...)` (`src/agents/agent.py:786-875`) using the parent's tool state, scoped context, and trace. Output is fed back to the caller as a `str` tool message; if no `custom_output_extractor` is provided, the nested `RunResult.final_output` is returned (`src/agents/agent.py:888-911`).
3. **External orchestration** — Callers compose top-level `Runner.run(...)` calls inside `asyncio.gather(...)` (`examples/agent_patterns/parallelization.py:30-43`) or chain via `trace(...)` grouping (`examples/agent_patterns/agents_as_tools.py:66-77`).

Both paths operate **inside a single trace**: nested runs do **not** create a child trace — `create_trace_for_run` returns `None` when a trace is already active (`src/agents/tracing/context.py:59-61`). Trace nesting is conveyed via span tree only: handoff transitions emit `HandoffSpanData(from_agent, to_agent)` (`src/agents/tracing/span_data.py:244-265`, factory `src/agents/tracing/create.py:262`), tool invocations emit `function_span` (`src/agents/run_internal/tool_execution.py:1019-1038`), and the nested `Runner.run` itself creates its own task/turn/agent span stack (`src/agents/run.py:645-647`, `src/agents/run_internal/run_loop.py:862-867`). The parent's `current_span` is finished and reset on every handoff transition (`src/agents/run.py:1019-1021`, mirrored in streaming `src/agents/run_internal/run_loop.py:1100-1101`), so spans from sibling agents are siblings under the trace, not nested.

State isolation is **partial by design**: handoffs share `RunContextWrapper.context`, `RunState`, usage, history, and tool-state scope (`src/agents/run_state.py:200-218`); the only state that flips is `RunState._current_agent` (`src/agents/run_state.py:203`, mutated at `src/agents/run.py:1015-1016`). `Agent.as_tool()` creates a fresh `ToolContext` (or `RunContextWrapper` when invoked outside a tool) so the nested run shares `context.context`, `usage`, and `tool_state_scope_id` but gets its own approval surface (`src/agents/agent.py:630-660`). Conditional enablement for both surfaces uses `is_enabled: bool | Callable[[ctx, agent], MaybeAwaitable[bool]]` (`src/agents/handoffs/__init__.py:153-161`, `src/agents/agent.py:515-516`).

Error propagation differs by path: handoff failures from the LLM side bubble as `ModelBehaviorError`/`UserError` (`src/agents/handoffs/__init__.py:283-289`, `src/agents/run_internal/turn_resolution.py:528-547`); `as_tool()` failures are **localized by default** through `failure_error_function=default_tool_error_function` (`src/agents/agent.py:524`, `src/agents/tool.py:1609-1618`) so the parent LLM receives a recoverable error string instead of an exception. Setting `failure_error_function=None` re-raises the original exception.

## Rating

**8 / 10** — Clear delegation model with two well-distinguished first-class patterns (`Handoff` and `Agent.as_tool`), strong type- and runtime-test coverage (`tests/test_handoff_tool.py:1-470`, `tests/test_agent_as_tool.py:1-1749+`), explicit lifecycle hooks (`src/agents/lifecycle.py:61-68`, `src/agents/lifecycle.py:138-146`), input filtering and nested-history options (`src/agents/handoffs/__init__.py:126-147`), HITL approvals that survive nested resume (`src/agents/agent.py:694-758`), and trace-visible handoff transitions with `from_agent`/`to_agent` metadata (`src/agents/tracing/span_data.py:244-265`). Deductions for: missing an explicit `parent_run_id`/`sub_run_id` link in trace payloads (only `group_id`/`trace_id` correlate), no separate child trace for nested `Runner.run` (`src/agents/tracing/context.py:59-61`), multiple-handoff handling is silent rather than strict (`src/agents/run_internal/turn_resolution.py:426-438`), and `failure_error_function` defaulting silently localizes errors in `as_tool()` which can hide bugs.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Handoff dataclass and `handoff(...)` factory | `Handoff` dataclass with `tool_name`, `tool_description`, `input_json_schema`, `on_invoke_handoff`, `agent_name`, `input_filter`, `nest_handoff_history`, `strict_json_schema`, `is_enabled` | `src/agents/handoffs/__init__.py:93-184` |
| Handoff `_invoke_handoff` and on-handoff validation | `_invoke_handoff` validates JSON via `TypeAdapter`, calls `on_handoff`, returns the next `agent`; raises `ModelBehaviorError` on bad input | `src/agents/handoffs/__init__.py:278-306` |
| Default tool naming and weakref to target agent | `default_tool_name` → `transfer_to_<agent>`, `default_tool_description` interpolates `agent.handoff_description`; `_agent_ref` weakref keeps targets non-owning | `src/agents/handoffs/__init__.py:163-183`, `src/agents/handoffs/__init__.py:335` |
| `HandoffInputData` (history, pre-handoff items, new items, optional `input_items`) | Frozen dataclass with `input_history`, `pre_handoff_items`, `new_items`, `run_context`, optional `input_items`; `clone()` helper | `src/agents/handoffs/__init__.py:42-83` |
| `RunConfig`-level handoff overrides | `RunConfig.handoff_input_filter`, `RunConfig.nest_handoff_history`, `RunConfig.handoff_history_mapper` referenced in `execute_handoffs` | `src/agents/run_internal/turn_resolution.py:487-495` |
| Server-managed-conversation handoff caveats | Warns and disables `nest_handoff_history` when `conversation_id`/`previous_response_id`/`auto_previous_response_id` are active; explicitly disallows `input_filter` | `src/agents/run_internal/turn_resolution.py:371-400` |
| Execution dispatch in run loop | `execute_handoffs(...)` invoked when `run_handoffs` is non-empty; collapses to first | `src/agents/run_internal/turn_resolution.py:403-591`, `src/agents/run_internal/turn_resolution.py:732-746` |
| Multiple-handoff collapse | Only `run_handoffs[0]` honored; rest get "Multiple handoffs detected, ignoring this one." in `ToolCallOutputItem` | `src/agents/run_internal/turn_resolution.py:426-438` |
| Handoff hooks fires `on_handoff` | `asyncio.gather` of `RunHooks.on_handoff(ctx, from_agent, to_agent)` and per-agent `AgentHooks.on_handoff(ctx, agent, source)` | `src/agents/run_internal/turn_resolution.py:470-485` |
| Handoff span | `with handoff_span(from_agent=public_agent.name) as span_handoff: ... span_handoff.span_data.to_agent = new_agent.name` | `src/agents/run_internal/turn_resolution.py:441-446` |
| `HandoffSpanData` payload | `from_agent`, `to_agent`; type `"handoff"` | `src/agents/tracing/span_data.py:244-265` |
| `handoff_span(...)` factory | Creates `Span[HandoffSpanData]` with explicit `from_agent`/`to_agent` | `src/agents/tracing/create.py:262-277` |
| `HandoffCallItem` and `HandoffOutputItem` items | `HandoffCallItem` records the tool-call; `HandoffOutputItem` carries `source_agent`/`target_agent` with weak refs to avoid leaks | `src/agents/items.py:266-288`, `src/agents/items.py:300-319` |
| Handoff `NextStepHandoff(new_agent)` | Returned by `execute_handoffs` so the outer loop pivots to the new agent | `src/agents/run_internal/run_steps.py:154-158`, `src/agents/run_internal/turn_resolution.py:582-591` |
| Current-agent swap on handoff | `current_agent = turn_result.next_step.new_agent`; `RunState._current_agent = current_agent`; `current_span.finish(reset_current=True)` and reset | `src/agents/run.py:1011-1023` |
| Mirror in streaming loop | Same swap in `start_streaming` with `AgentUpdatedStreamEvent(new_agent=current_agent)` and current_span finish | `src/agents/run_internal/run_loop.py:803-815`, `src/agents/run_internal/run_loop.py:1091-1112` |
| `get_handoffs` enablement filter | Filters via `is_enabled` bool or callable; `asyncio.gather` over `check_handoff_enabled` | `src/agents/run_internal/turn_preparation.py:88-108` |
| `Agent.as_tool` parameters | `tool_name`, `tool_description`, `custom_output_extractor`, `is_enabled`, `on_stream`, `run_config`, `max_turns`, `hooks`, `previous_response_id`, `conversation_id`, `session`, `failure_error_function`, `needs_approval`, `parameters`, `input_builder`, `include_input_schema` | `src/agents/agent.py:508-559` |
| `as_tool` nested invocation | `_run_agent_impl` calls `Runner.run_streamed(...)` (if `on_stream` set) or `Runner.run(...)`; nested context is a fresh `ToolContext` (preserves `context.context`, `usage`, tool namespace) or `RunContextWrapper` | `src/agents/agent.py:599-875` |
| Nested-tool context isolation | New `ToolContext` from parent `ToolContext` carries approvals, usage, namespace; scope id preserved via `set_agent_tool_state_scope` | `src/agents/agent.py:630-660` |
| Nested-HITL approval mirroring | `_nested_approvals_status` resolves mirrored approval records; `_apply_nested_approvals` calls `nested_context.approve_tool`/`reject_tool` for parent decisions before resume | `src/agents/agent.py:665-758` |
| Nested-run resumption via `agent_tool_state` | `peek_agent_tool_run_result` and `consume_agent_tool_run_result`; pending `interruptions` are recorded on the parent `tool_call` | `src/agents/agent.py:760-782`, `src/agents/agent_tool_state.py:108` |
| Streaming dispatch | `on_stream` runs an `asyncio.Queue` + dispatch task so slow user callbacks do not block event consumption; nested `AgentUpdatedStreamEvent` updates `current_agent` | `src/agents/agent.py:800-861` |
| Custom output extraction | `custom_output_extractor(run_result)`; exposes `result.agent_tool_invocation` from `ToolContext` metadata | `src/agents/agent.py:545`, `src/agents/result.py:307-322`, `src/agents/agent.py:888-911` |
| `ToolOrigin` for nested agent tools | `ToolOriginType.AGENT_AS_TOOL` records `agent_name`/`agent_tool_name` on the tool-origin metadata so downstream telemetry distinguishes it from plain functions/MCP | `src/agents/tool.py:270-296`, `src/agents/agent.py:927-932` |
| `AgentToolInvocation` (frozen metadata) | `tool_name`, `tool_call_id`, `tool_arguments` exposed on `RunResult` produced via `as_tool` | `src/agents/result.py:57-68`, `src/agents/result.py:307-322` |
| Default error localization for `as_tool` | `failure_error_function=default_tool_error_function`; default impl returns string instead of raising | `src/agents/agent.py:524`, `src/agents/tool.py:1609-1618` |
| Tool-call status on `RunContextWrapper` | `approve_tool`, `reject_tool`, `get_approval_status`, `_approvals` dict | `src/agents/run_context.py:30-110` |
| Tool `needs_approval` flow | Plans emit `NextStepInterruption(interruptions=...)` when `pending_interruptions` is set; `execute_mcp_approval_requests` plus per-tool gating | `src/agents/run_internal/tool_planning.py:707-723`, `src/agents/agent.py:526, 678` |
| Parallel tool execution (incl. `as_tool`) | Tool planner uses `asyncio.gather` of `execute_function_tool_calls`, `execute_computer_actions`, `execute_custom_tool_calls`, etc. when `plan.parallel` is True | `src/agents/run_internal/tool_planning.py:572-595` |
| `nest_handoff_history` and marking wrappers | `nest_handoff_history(handoff_input_data, ...)` collapses prior transcript into a single `<CONVERSATION HISTORY>` summary message; default and override mappers | `src/agents/handoffs/history.py:71-122`, `src/agents/handoffs/history.py:40-68` |
| Hook for handoff (per-agent and global) | `RunHooksBase.on_handoff`, `AgentHooksBase.on_handoff` (`source`/`agent`) | `src/agents/lifecycle.py:61-68`, `src/agents/lifecycle.py:138-146` |
| Trace-aware `create_trace_for_run` | Returns `None` if `current_trace` is set — so nested `as_tool` invocations do NOT spawn a child trace | `src/agents/tracing/context.py:47-88` |
| Default `try`-shape nested agent span | Nested `Runner.run` puts its own `task_span` + `agent_span` + `turn_span` under the parent's `current_trace` | `src/agents/run.py:645-647`, `src/agents/run_internal/run_loop.py:862-867` |
| Function span wrapper for tools | `with_tool_function_span` wraps invocation in `function_span` when trace active | `src/agents/run_internal/tool_execution.py:1019-1038` |
| Span parent linkage | `SpanImpl(trace_id=..., span_id=..., parent_id=parent.span_id)`; parent_id derived from current_span via context | `src/agents/tracing/provider.py:391-425` |
| Doc — handoff patterns and `input_type` semantics | Distinguishes metadata (`input_type`) from input-filter/nest-handoff-history; raises UserError for empty filtering on server-managed conversations | `docs/handoffs.md:1-152` |
| Doc — orchestration patterns | "Agents as tools" (manager keeps control) vs "Handoffs" (specialist takes over) summary table; combo patterns allowed | `docs/multi_agent.md:24-32` |
| Example — `agents_as_tools.py` | Orchestrator wraps three translator agents as tools, single `trace("Orchestrator evaluator")` | `examples/agent_patterns/agents_as_tools.py:30-79` |
| Example — conditional enablement | `is_enabled=...` callable gates tools dynamically; works with HITL `needs_approval=True` | `examples/agent_patterns/agents_as_tools_conditional.py:62-80` |
| Example — input filter | `message_filter.py` drops prior tool calls before passing history to next agent | `examples/handoffs/message_filter.py:1+` (path) |
| Example — parallel `Runner.run` via `asyncio.gather` | Three translations in parallel then synthesizer picks best | `examples/agent_patterns/parallelization.py:30-43` |
| Test — `test_handoff_tool.py` | 470 lines: single/multi setup, custom tool_name override, `is_enabled` bool+sync+async, callable-class detection, `input_type` validation, hook firing, `HandoffInputData` shape | `tests/test_handoff_tool.py:1-470` |
| Test — `test_agent_as_tool.py` | ~1749+ lines: enablement, custom output extractor, fallback to message, streaming via `on_stream`, structured input, namespace, scope, nested-HITL resume, namespaced always-approve through resume | `tests/test_agent_as_tool.py:1-1749+` |
| Test — `test_handoff_history_duplication.py` | Verifies history mapping & dedup behavior | `tests/test_handoff_history_duplication.py:1+` (path) |
| Test — `test_handoff_prompt.py` | Asserts recommended prompt prefix is used | `tests/test_handoff_prompt.py:1+` (path) |

## Answers to Dimension Questions

1. **Can agents delegate?**
   Yes, in two complementary ways. **Handoffs** transfer the active agent mid-turn via the `transfer_to_<agent_name>` tool (`src/agents/handoffs/__init__.py:172-176`, `src/agents/run_internal/turn_resolution.py:441-446`). **`Agent.as_tool(...)`** wraps an agent as a `FunctionTool` so any agent can call it like a regular tool (`src/agents/agent.py:508-936`). External code can also chain `Runner.run(...)` inside `asyncio.gather`/`trace(...)` (`examples/agent_patterns/parallelization.py:30-43`, `examples/agent_patterns/agents_as_tools.py:66-77`).

2. **Is delegation just a tool call or a real child run?**
   Both forms are real child runs, but they differ in lifecycle. Handoffs execute the tool in-line but the child agent's run shares the same `RunState` and `current_turn` count — the "child" run is just the continuation of the same `Runner.run` with `current_agent` swapped (`src/agents/run.py:1011-1023`). `as_tool()` invokes a brand-new `Runner.run(...)` from inside the parent loop's tool execution phase (`src/agents/agent.py:786-875`), complete with its own task span, max_turns ceiling, lifecycle hooks, and approval surface — though it still lives inside the parent's trace (`src/agents/tracing/context.py:59-61`).

3. **Is child state isolated?**
   Partially. Handoffs share **everything** (context, usage, history, `RunState`, tool-state scope) — the only state that flips is `RunState._current_agent` (`src/agents/run_state.py:203`, `src/agents/run.py:1015-1016`). `as_tool()` deliberately **inherits** the caller's `context`, `usage`, `_approvals`, tool namespace, and tool-state scope, but creates a fresh `ToolContext`/`RunContextWrapper` for the nested invocation so it owns its own approval state and `tool_input` (`src/agents/agent.py:630-660`). Both forms reuse the parent's `RunConfig` unless overridden (`src/agents/agent.py:626-629`).

4. **Are child traces linked to parent traces?**
   Yes for span lineage, no for trace lineage. Nested runs **share the parent trace** (`src/agents/tracing/context.py:59-61`), but every span is created with the live `current_span.span_id` as `parent_id`, so the span tree under the trace shows parent→child edges for tool, handoff, agent, and turn spans (`src/agents/tracing/provider.py:391-425`). Handoff transitions additionally emit a dedicated `HandoffSpanData(from_agent, to_agent)` (`src/agents/tracing/span_data.py:244-265`, `src/agents/tracing/create.py:262-277`). `tracing/group_id` on `Trace` (`src/agents/run_internal/run_grouping.py:12-48`) is the only stable cross-run correlation key — there is no explicit `parent_run_id`/`sub_run_id` link in trace payloads beyond `group_id`.

5. **Can child loops run in parallel?**
   Within a single turn, multiple `as_tool()` invocations can run in parallel because `execute_tools_and_side_effects` parallelizes tool categories with `asyncio.gather` (`src/agents/run_internal/tool_planning.py:572-595`). Handoffs cannot — if the model returns multiple handoff tool calls in one turn, only the first is honored and the rest are converted into "Multiple handoffs detected, ignoring this one." outputs (`src/agents/run_internal/turn_resolution.py:426-438`). Across top-level runs, callers can run multiple `Runner.run`s in parallel via `asyncio.gather` (`examples/agent_patterns/parallelization.py:30-43`) and group them under one `trace(group_id=...)` (`docs/tracing.md`, `src/agents/run_internal/run_grouping.py:30-33`).

## Architectural Decisions

- **Two delegation primitives by intent.** The SDK keeps handoffs (control transfer) and `Agent.as_tool()` (sub-call) as **distinct, named, statically typed objects**. The doc string at `src/agents/agent.py:531-537` explicitly contrasts them: "In handoffs, the new agent receives the conversation history... In this tool, the new agent is called as a tool, and the conversation is continued by the original agent."
- **Handoff as a tool, not a hidden side channel.** A handoff is modeled as an ordinary function tool the model can choose, sharing the same tool-call and tool-output infrastructure as everything else (`src/agents/handoffs/__init__.py:172-183`). This keeps a single dispatch surface and lets handoffs participate in tool guardrails, approval flows, and `function_span` tracing uniformly.
- **`max_turns` and `current_turn` are shared across handoffs.** Handoffs do not have their own turn budget; the parent's `max_turns` still applies (`src/agents/run.py:1058-1066`). This is enforced by AGENTS.md guidance that "Input guardrails run only on the first turn and only for the starting agent" (`AGENTS.md:73-75`) — handoffs are not a hard reset.
- **Nested runs inherit the active trace, never spawn a new one.** `create_trace_for_run` short-circuits to `None` when a `current_trace` is set (`src/agents/tracing/context.py:59-61`). Causality is conveyed by the span tree plus the optional `group_id`. This avoids trace explosion for deeply nested orchestrations but means operators must use `group_id` to correlate runs.
- **`failure_error_function` defaults to localizing `as_tool` errors.** Errors thrown by the nested run become a string returned to the parent LLM via `default_tool_error_function` (`src/agents/agent.py:524`, `src/agents/tool.py:1609-1618`). The author can opt into `failure_error_function=None` to re-raise. This trade-off favors LM-driven recovery over hard failure.
- **HITL is preserved through resume.** A nested `as_tool()` run with pending interruptions records the result keyed by the parent's `tool_call` (`src/agents/agent_tool_state.py:108`, `src/agents/agent.py:880-886`). On resume, parent approval decisions are mirrored into the nested context via `_apply_nested_approvals` (`src/agents/agent.py:694-758`), so a user can approve at the outer level once and have it apply inside the nested run.
- **Span emission is symmetric across streaming and non-streaming paths.** The handoff span `from_agent`/`to_agent` and the agent-span `name`/`handoffs[]`/`tools[]`/`output_type` shape are produced identically in sync (`src/agents/run.py:1035-1055`) and streaming (`src/agents/run_internal/run_loop.py:853-873`).
- **History rewriting is opt-in and additive.** `nest_handoff_history` collapses prior transcript into a single assistant summary wrapped in `<CONVERSATION HISTORY>`…`</CONVERSATION HISTORY>` markers (`src/agents/handoffs/history.py:71-122`). Server-managed conversations disable nesting with a warning (`src/agents/run_internal/turn_resolution.py:391-400`). The helper preserves `new_items` for session history while using `input_items` for the next agent's model input (`src/agents/handoffs/__init__.py:66-72`, `src/agents/run_internal/turn_resolution.py:556-577`).

## Notable Patterns

- **`ToolOrigin` taxonomy** explicitly distinguishes `function`, `mcp`, and `agent_as_tool` sources (`src/agents/tool.py:270-296`). The `tool_origin` is propagated onto the wrapped `_tool_origin` so consumers can decide whether a `ToolCallItem` came from an embedded agent vs. an MCP server vs. a plain function (`src/agents/agent.py:927-932`).
- **Weakref-managed handoff targets** — `Handoff._agent_ref` is a `weakref.ReferenceType` so the target agent is not pinned by closure (`src/agents/handoffs/__init__.py:163-166`). `HandoffOutputItem` mirrors the pattern for `source_agent` and `target_agent` to avoid leaks during long-lived streams (`src/agents/items.py:290-319`).
- **`on_stream` bridge without blocking the consumer** — Stream events from the nested run are pushed to an `asyncio.Queue`, the user callback runs in a worker task, and `event_queue.join()` ensures the loop drains before exit (`src/agents/agent.py:802-861`).
- **`AgentToolInvocation` self-identification** — a `RunResult` produced by `as_tool` exposes a frozen `AgentToolInvocation(tool_name, tool_call_id, tool_arguments)`, derived from the surrounding `ToolContext` (`src/agents/result.py:57-68`, `src/agents/result.py:307-322`); this lets custom output extractors know exactly which parent tool call they are post-processing.
- **Cancel mode reservation on streamed runs** — `_cancel_mode` (`"after_turn"` etc.) lets callers halt between turns during handoff transitions (`src/agents/run_internal/run_loop.py:842-845`, `src/agents/run_internal/run_loop.py:1109-1112`).

## Tradeoffs

- **No separate child trace for `as_tool()`** (`src/agents/tracing/context.py:59-61`). Pro: avoids trace explosion across deep orchestrations and keeps a single coherent timeline. Con: observability backends must rely on `group_id` and span tree depth to attribute usage/cost. Cost/usage is still per nested run inside `task_span.usage` (`src/agents/run.py:645-647`).
- **Multiple-handoff collapse is silent** (`src/agents/run_internal/turn_resolution.py:426-438`). Pro: deterministic single-agent step per turn. Con: callers cannot easily detect that the model attempted more than one handoff in the same turn except by inspecting `new_items` for "Multiple handoffs detected, ignoring this one." text.
- **`failure_error_function` defaults to localizing errors for `as_tool`** (`src/agents/agent.py:524`, `src/agents/tool.py:1609-1618`). Pro: keeps LM reasoning alive when a child agent crashes. Con: silent error swallowing can mask bugs in nested runs unless callers explicitly opt out.
- **Shared `usage` and approvals across `as_tool`** (`src/agents/agent.py:635-639`, `src/agents/agent.py:665-758`). Pro: nested runs count toward the parent's usage budget and respect pre-existing HITL decisions. Con: nested runs cannot be billed/quotas'd independently.
- **Disabled handoffs are filtered at request time, not invocation time** (`src/agents/run_internal/turn_preparation.py:88-108`). Pro: efficient. Con: a static `is_enabled=True` does not allow per-tool-call state mutability — only the callable form gives dynamic gating.
- **History filtering and nesting turn off under server-managed conversations** (`src/agents/run_internal/turn_resolution.py:371-400`). Pro: enforces predictable state deltas. Con: a `conversation_id` workflow effectively opts out of the most expressive handoff customization.

## Failure Modes / Edge Cases

- **Invalid handoff args (`input_type`) → `ModelBehaviorError`** (`src/agents/handoffs/__init__.py:283-289`). Empty JSON with non-empty schema raises rather than silently defaulting.
- **Multiple handoffs in one turn** — all but the first are dropped (`src/agents/run_internal/turn_resolution.py:426-438`); the dropped ones still cost a tool call against the LLM round-trip.
- **Server-managed conversations + `input_filter`** → `UserError` with concrete remediation message (`src/agents/run_internal/turn_resolution.py:384-389`).
- **`as_tool()` whose nested run is interrupted** — the nested `RunResult` is recorded by parent `tool_call` via `record_agent_tool_run_result` (`src/agents/agent.py:880-886`). Status flow: outer rejects → resume state replays with `nested_context.reject_tool` applied (`src/agents/agent.py:747-758`); outer approves → `nested_context.approve_tool(..., always_approve=...)` (`src/agents/agent.py:747-752`).
- **Missing `on_invoke_handoff` callable** → `UserError` at factory time (`src/agents/handoffs/__init__.py:258-276`). Validation catches missing `input_type`/`on_handoff` pairs up front.
- **Streaming consumer cancels while callbacks are in flight** — `stream_iteration_cancelled` flag cancels `dispatch_task` and awaits it under a `try/except asyncio.CancelledError` (`src/agents/agent.py:830-860`).
- **`asyncio.gather` of stream callables** — failure in any callback is logged but does not terminate dispatch queue (`src/agents/agent.py:804-815`). Caller-provided callbacks must manage their own back-pressure.
- **Approval key collisions** when nested and outer tools share the same qualified name — resolved by `_find_mirrored_approval_record` searching `_resolve_approval_keys` plus function-tool approval keys including legacy deferred keys (`src/agents/agent.py:699-725`).
- **`max_turns` exhaustion during a long handoff chain** — `MaxTurnsExceeded` is set on the agent span via `_error_tracing.attach_error_to_span` (`src/agents/run.py:1059-1066`); `RunErrorHandlers` may override (`src/agents/run_internal/run_loop.py:1058-1130`).

## Future Considerations

- **Explicit `parent_run_id`/`sub_run_id` field on `RunResult`/`RunState`.** Today the only cross-run correlation key is `group_id` (`src/agents/run_internal/run_grouping.py:30-33`); adding a stable parent pointer would simplify per-call cost attribution and per-nested-run explainability.
- **Allow `as_tool` to spawn a child trace when explicitly requested.** Would let orchestrators opt into heavier observability on demand, while the default "share parent trace" remains the default (`src/agents/tracing/context.py:59-61`).
- **Configurable multi-handoff policy.** Either fan-out to a deterministic sequence or fail loudly instead of silently dropping extras (`src/agents/run_internal/turn_resolution.py:426-438`).
- **Schema for parallel handoffs.** The `InputGuardrail.run_in_parallel` knob exists (`src/agents/guardrail.py:100`, `src/agents/guardrail.py:217`), but the planner collapses parallel handoffs. Adding an explicit "all-handoffs-receive-a-summary" mode would let triage agents express multi-cast routing.
- **Standardize tracing payloads for `as_tool` children.** Already partially done via `ToolOriginType.AGENT_AS_TOOL` (`src/agents/tool.py:275`), `AgentToolInvocation` (`src/agents/result.py:57-68`), and `agent_tool_invocation` (`src/agents/result.py:308-322`). Extending `group_id` to nest under a per-tool-call sub-group would let backends diff nested runs from siblings.

## Questions / Gaps

- Is there an explicit test that asserts **parent → child span edges across an `as_tool` run**? Search boundary: `tests/` mentions of "parent" + "span"; I found that `with_tool_function_span` wraps the call (`src/agents/run_internal/tool_execution.py:1019-1038`) but did not find a dedicated snapshot test asserting the span tree shape across a nested invocation.
- Does `RunState._current_agent` propagate to the nested `Runner.run` when `resume_state is passed back in? No explicit evidence of a sub-run carrying a serialized handoff lineage under `_starting_agent`/`_current_agent`; the field is documented as "the agent currently handling the conversation" (`src/agents/run_state.py:203-206`) but cross-run identity is not formally specced.
- What is the **relationship between `group_id` set by `run_config.group_id` and prompt-cache grouping?** A comment claims the order matches prompt-cache grouping (`src/agents/run_internal/run_grouping.py:18-21`) but no prompt-cache test was found to corroborate.
- The codebase has references to **legacy deferred approval keys** and `tool_lookup_key=("deferred_top_level", ...)` (`src/agents/agent.py:1378-1441`) — no clear public docs on when to use this path; treat as observed behavior, not a stable contract.
- Docs claim **`input_filter` is incompatible with server-managed conversations** (`src/agents/run_internal/turn_resolution.py:384-389`, `docs/handoffs.md:115`) but I did not find a regression test asserting the resulting UserError; consider adding one.

---

Generated by `03.08-subagent-and-forked-loop-design.md` against `openai-agents-sdk`.
