# Source Analysis: openai-agents-sdk

## Dimension 04.08: Agent-as-Tool and Workflow-as-Tool Composition

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (OpenAI Agents SDK, asyncio-based runner) |
| Analyzed | 2026-08-23 |

## Summary

Composition is a first-class, explicitly designed capability in this SDK. `Agent.as_tool()` (`src/agents/agent.py:583-1040`) converts any `Agent` into a `FunctionTool` whose invocation starts a full nested `Runner.run` / `Runner.run_streamed` execution (`src/agents/agents/../agent.py:889-904`, `src/agents/agent.py:961-976`). The wrapper carries rich metadata (`_is_agent_tool`, `_agent_instance`, `ToolOrigin(type=AGENT_AS_TOOL)` at `src/agents/tool.py:545-557`, `src/agents/tool.py:285-337`, `src/agents/agent.py:1029-1038`) that downstream machinery uses for tracing, approvals routing, and state serialization.

Child runs are bounded per-invocation by `max_turns` (defaulting to `DEFAULT_MAX_TURNS = 10`, `src/agents/run_config.py:45`, applied at `src/agents/agent.py:713`) and cannot disable the bound even by passing `max_turns=None`. Nested execution joins the ambient trace (`src/agents/tracing/context.py:59-61` returns no new trace when one is active; each run emits its own `agent_span`, `src/agents/run_internal/run_loop.py:1528`, `src/agents/run.py:1457`). Token costs roll up into the shared parent `Usage` object (`src/agents/agent.py:720-724`, rebind on resume at `src/agents/agent.py:883-884`), though there is no per-child cost breakdown.

The most mature part of the design is human-in-the-loop through nesting: interruptions raised by tools inside a nested agent surface as interruptions on the outer run, are routed to the owning nested context for approval decisions (`src/agents/tool_context.py:125-228`), survive `RunState` serialization round-trips, and work through arbitrary nesting depth (`tests/test_run_state.py:11673-11750` parametrizes over `nesting_edges`). Cancellation propagates deterministically through both non-streaming (`src/agents/run_internal/tool_execution.py:2168-2203`) and streaming (`src/agents/util/_asyncio_tasks.py:117-152`) paths, backed by dedicated tests (`tests/test_agent_as_tool.py:2837-2928`, `tests/test_agent_as_tool.py:3530-3650`).

The notable gap is recursion safety: there is no cycle detection or depth counter. Each nesting level receives a fresh turn budget, so a self-referential composition (`agent.tools = [agent.as_tool(...)]`) would recurse until model behavior, context limits, or external cancellation stop it. Termination relies on per-level bounds plus `reset_tool_choice` (`src/agents/agent.py:395-397`), not on an explicit guard.

## Rating

**8 / 10** — Clear, explicit composition model (`Agent.as_tool()` with typed input/output contracts), extensive test coverage (~3,700-line dedicated unit suite `tests/test_agent_as_tool.py` plus multi-level resume tests), and strong operational safeguards (nested approval flow, cancellation propagation, trace joining, usage aggregation, tool-origin provenance). Falls short of 9-10 because: (1) no recursion-depth guard or cycle detection for degenerate compositions, (2) cost attribution is aggregate-only with no per-child ledger, (3) nested run results live in process-local module-global dictionaries (`src/agents/agent_tool_state.py:37-48`) rather than a durable store, and (4) agent tools do not expose tool-guardrail options directly (`docs/guardrails.md:65`).

## Evidence Collected

Every entry cites file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent-to-tool conversion API | `Agent.as_tool(tool_name, tool_description, custom_output_extractor, is_enabled, on_stream, run_config, max_turns, hooks, session, needs_approval, parameters, input_builder, include_input_schema) -> FunctionTool` | `src/agents/agent.py:583-605` |
| Nested run invocation (non-streaming) | Tool impl calls `Runner.run(starting_agent=self, input=resolved_input, max_turns=resolved_max_turns, ...)` | `src/agents/agent.py:961-976` |
| Nested run invocation (streaming) | `Runner.run_streamed(...)` with background event dispatch queue when `on_stream` provided | `src/agents/agent.py:887-958` |
| Child run bound | `resolved_max_turns = max_turns if max_turns is not None else DEFAULT_MAX_TURNS`; `DEFAULT_MAX_TURNS = 10` | `src/agents/agent.py:713`; `src/agents/run_config.py:45` |
| Default input contract | `AgentAsToolInput(BaseModel)` with single `input: str` field; strict JSON schema enforced | `src/agents/agent_tool_input.py:21-25`; `src/agents/agent.py:656-658` |
| Structured input contract | Custom `parameters` (dataclass/Pydantic), `input_builder`, `include_input_schema`; schema-summary rendering with injection-hardening preamble | `src/agents/agent.py:650-668`; `src/agents/agent_tool_input.py:13-16`, `src/agents/agent_tool_input.py:79-107` |
| Output contract to parent model | `custom_output_extractor` → `final_output` → last message/`ToolCallOutputItem` fallback chain returning str | `src/agents/agent.py:990-1013` |
| Subagent result schema (internal) | `FunctionToolResult.interruptions` ("Interruptions from nested agent runs (for agent-as-tool)") and `.agent_run_result` carry the full nested result to the harness | `src/agents/tool.py:374-392` |
| Public nested-run metadata | `AgentToolInvocation(tool_name, tool_call_id, tool_arguments)` exposed as `RunResultBase.agent_tool_invocation` | `src/agents/result.py:67-79`, `src/agents/result.py:455-470` |
| Streaming event schema | `AgentToolStreamEvent(TypedDict): event, agent, tool_call` exported publicly | `src/agents/agent.py:130-140`; `src/agents/__init__.py:12` |
| Streaming handoff tracking | Producer loop tracks `current_agent` across `AgentUpdatedStreamEvent` so callbacks see the active nested agent | `src/agents/agent.py:940-956` |
| Nested tracing (trace join) | `create_trace_for_run` returns `None` when a trace is already current → nested spans join parent trace | `src/agents/tracing/context.py:58-61` |
| Per-run agent span | `agent_span(...)` created inside each run loop (parent and nested alike); spans nest via contextvars | `src/agents/run_internal/run_loop.py:1528`; `src/agents/run.py:1457`; `docs/tracing.md:136` |
| Span payload | `AgentSpanData` exports name, handoffs, tools, output_type per span | `src/agents/tracing/span_data.py:28-61` |
| Cost attribution (shared usage) | Nested `ToolContext` constructed with `usage=context.usage` so child turns accrue on parent totals | `src/agents/agent.py:720-730` |
| Usage rebind on cached resume | Resume checkpoints re-bind `resume_state._context.usage = context.usage` before post-resume turns | `src/agents/agent.py:879-884`; verified by `tests/test_agent_as_tool.py:1545-1619` |
| Per-request cost entries | `Usage.request_usage_entries` preserves per-request token breakdown for cost calculation | `src/agents/usage.py:218-229`, `src/agents/usage.py:295-312` |
| Nested result cache | Module-global maps keyed by scope-qualified tool-call identity/signature; peek/consume/drop semantics | `src/agents/agent_tool_state.py:35-48`, `src/agents/agent_tool_state.py:145-156`, `src/agents/agent_tool_state.py:208-276` |
| Cache leak protection | Weakref GC callback drops cached nested results when the tool call object is collected | `src/agents/agent_tool_state.py:131-142` |
| Scope isolation | `_agent_tool_state_scope_id` attached to contexts prevents cross-restored-state collisions | `src/agents/agent_tool_state.py:33-70`; propagated in `src/agents/run_internal/agent_runner_helpers.py:342-352` |
| Durable checkpointing of nested state | `_copy_pending_nested_agent_state...` binds detached nested approval checkpoints into outer `RunState` snapshots | `src/agents/result.py:175-219` |
| Interruption surfacing | Nested interruptions bubble into outer processed response via `_collect_tool_interruptions` (checks `result.interruptions` then `result.agent_run_result.interruptions`) | `src/agents/run_internal/tool_planning.py:685-714` |
| Pending-nested handling | Parent skips tool output while nested interruptions unresolved; replays after decision | `src/agents/run_internal/tool_execution.py:2097-2103`, `src/agents/run_internal/tool_execution.py:2255-2303` |
| Approval routing to nested owner | `ToolContext._find_nested_approval_target` matches interruption identity and routes approve/reject to nested context; ambiguous identity fails closed | `src/agents/tool_context.py:125-192` |
| Multi-level resume proof | `test_resume_recursively_nested_agent_as_tool_decision` builds N nesting edges, serializes to JSON, restores, applies decision, resumes | `tests/test_run_state.py:11673-11750` |
| Cancellation (non-streaming) | `_await_invoke_task` shields invoke task; sibling-failure and parent-cancel paths cancel and drain | `src/agents/run_internal/tool_execution.py:2168-2203` |
| Cancellation (streaming) | `run_producer_consumer` asymmetric failure handling: producer failure drains consumer; consumer failure/parent cancel cancels producer | `src/agents/util/_asyncio_tasks.py:117-152` |
| Cancel propagation test | Slow `on_stream` handler does not block `CancelledError` reraise | `tests/test_agent_as_tool.py:2837-2928` |
| BaseException drain test | Streaming path reraises `BaseException` and drains emitted events without hanging | `tests/test_agent_as_tool.py:3530-3671` |
| Error propagation policy | `failure_error_function` defaults to `default_tool_error_function` (model-visible string); `None` reraises; custom handler supported | `src/agents/agent.py:599`; `tests/test_agent_as_tool.py:3335-3427` |
| Replaced-tool error policy | Failure handler resolves against the current `FunctionTool` instance, so swapped agent tools use their own policy/name | `src/agents/tool.py:617-667`; `tests/test_agent_as_tool.py:3430-3501` |
| Tool-origin provenance | `ToolOriginType.AGENT_AS_TOOL` with `agent_name` + `agent_tool_name` serialized onto items and persisted in RunState | `src/agents/tool.py:285-337`; `src/agents/agent.py:1029-1033` |
| Derived-name collision policy | Ambiguous derived names (e.g. "Refund" vs "refund") raise `UserError` under `collision_policy="error"`, warn-and-drop under `"warn"` | `src/agents/_tool_identity.py:503-538`; `tests/test_agent_as_tool.py:64-172` |
| Recursion guard search | No hits for recursion/cycle/depth guards over agent composition; only schema-copy recursion limits and handoff-graph traversal with `seen_agent_ids` (handoffs only, not agent tools) | grep across `src/agents/**`; `src/agents/run_state.py:4574-4584` |
| Loop-bound mitigations | `reset_tool_choice=True` default documented as preventing infinite tool-choice loops | `src/agents/agent.py:395-397` |
| Docs: orchestration pattern | Agents-as-tools vs handoffs guidance; `max_turns`, `run_config`, `needs_approval`, structured input options documented | `docs/tools.md:656-658`, `docs/tools.md:680-712`; `docs/multi_agent.md:26` |
| Docs: nested HITL | Approvals inside nested `Agent.as_tool()` surface on outer run; two-layer approvals documented | `docs/human_in_the_loop.md:5-7`, `docs/human_in_the_loop.md:180` |
| Examples | Runnable streaming and structured-input examples | `examples/agent_patterns/agents_as_tools_streaming.py:42-46`; `examples/agent_patterns/agents_as_tools_structured.py` |

## Answers to Dimension Questions

**1. Can one agent call another?**
Yes, explicitly. `Agent.as_tool()` (`src/agents/agent.py:583`) wraps the agent in a strict-schema `FunctionTool`; invoking it starts a real nested `Runner.run` (`src/agents/agent.py:961-976`) with its own turn loop, hooks, optional `session`/`conversation_id`, and fresh `ToolContext`. Composition is distinguished from handoffs in the API docstring: the caller retains conversation control and the child receives generated input (`src/agents/agent.py:606-612`). Multiple composition examples ship in-tree (`examples/agent_patterns/agents_as_tools.py`, `agents_as_tools_conditional.py`, `agents_as_tools_streaming.py`, `agents_as_tools_structured.py`).

**2. Are child runs bounded?**
Yes, per child run. `max_turns` defaults to `DEFAULT_MAX_TURNS = 10` (`src/agents/run_config.py:45`) and is applied at `src/agents/agent.py:713`. Notably, passing `max_turns=None` still resolves to the default, so a nested child can never be configured unbounded — the bound is mandatory at this layer. However, bounds are *per nesting level*: there is no cumulative budget across levels, and no wall-clock/token budget specific to child runs beyond what `run_config` carries.

**3. Are child run costs attributed?**
Partially. Costs are *aggregated* correctly: the nested `ToolContext` shares the parent's `Usage` object (`src/agents/agent.py:720-724`), and the cached-resume path explicitly rebinds usage back onto the outer context so post-resume turns are not lost (`src/agents/agent.py:879-884`, tested at `tests/test_agent_as_tool.py:1545-1619`). Per-request entries are retained in `Usage.request_usage_entries` (`src/agents/usage.py:218-229`), enabling post-hoc cost computation. But there is **no per-child attribution**: nothing tags which requests/tokens belonged to which nested agent invocation; the parent sees one merged counter. A user wanting per-child cost must build it themselves from `request_usage_entries`.

**4. Can nested tools recurse forever?**
Not literally forever, but there is **no explicit recursion guard**. Searching `src/agents/` for recursion/cycle/depth guards over agent composition found none (only JSON-schema copy-depth limits in `src/agents/strict_schema.py:53` and a cycle-safe BFS over *handoff* graphs in `src/agents/run_state.py:4574-4584`, which does not traverse agent-tools). Termination rests on three indirect mechanisms: (a) each nested run's own `max_turns` budget (fresh per level, default 10), (b) `reset_tool_choice=True` preventing forced-tool loops (`src/agents/agent.py:395-397`), and (c) model behavior. Deep-but-intentional nesting works and is tested to arbitrary edges (`tests/test_run_state.py:11695-11714` loops `nesting_edges`). A pathological self-referential graph (`a.tools = [a.as_tool()]`) would recurse until context/token exhaustion or external cancellation — the SDK neither detects nor prevents it at construction time.

**5. Does the parent receive structured results?**
To the *model*, the parent receives a string: either the child's `final_output`, the `custom_output_extractor` result, or a last-message fallback (`src/agents/agent.py:990-1013`). To the *harness*, structure is preserved end-to-end: `FunctionToolResult.agent_run_result` holds the complete nested `RunResult` including its `interruptions` (`src/agents/tool.py:388-392`; populated at `src/agents/run_internal/tool_execution.py:2244-2302`), `RunResultBase.agent_tool_invocation` exposes the enclosing call identity to extractors (`src/agents/result.py:455-470`), and callers can opt into fully structured *input* contracts via `parameters`/`input_builder` (`src/agents/agent.py:602-604`). There is no typed/validated output schema for agent tools analogous to function tools' `output_json_schema` — the child's final output type is whatever the nested agent produced.

## Architectural Decisions

1. **Composition reuses the same `FunctionTool` pipeline instead of a parallel mechanism.** An agent tool is a plain `FunctionTool` flagged `_is_agent_tool=True` (`src/agents/agent.py:1015-1040`), inheriting schemas, approvals, timeouts, guardrails hooks, tracing, and failure-error plumbing for free. The cost is that nested-agent-specific concerns (interruption replay, usage rebinding) leak into generic modules like `turn_resolution.py` and `tool_execution.py` (see `src/agents/run_internal/turn_resolution.py:1508-1597`).

2. **Nested results keyed by tool-call identity in process-local registries.** `record/peek/consume/drop_agent_tool_run_result` index by `(scope_id, id(tool_call))` plus a stable content signature (`src/agents/agent_tool_state.py:35-48`). This lets the outer run pause/resume around nested approvals without serializing the whole child result, and weakrefs prevent leaks (`src/agents/agent_tool_state.py:131-142`). Durability is achieved separately by copying nested checkpoint templates into the outer `RunState` (`src/agents/result.py:175-219`).

3. **Interruptions always surface at the top.** Rather than requiring users to drive nested runs directly, nested approvals are collected into the outer step's interruptions (`src/agents/run_internal/tool_planning.py:697-704`), and `ToolContext.approve_tool/reject_tool` route decisions down to the owning nested context by invocation identity, failing closed on ambiguity (`src/agents/tool_context.py:178-191`). This makes HITL uniform regardless of nesting depth.

4. **Mandatory child turn bounds.** `max_turns=None` collapses to the default (`src/agents/agent.py:713`), encoding the judgment that unbounded children are never desirable — a small but deliberate safety choice.

5. **Streaming as opt-in callback, not first-class stream surface.** `on_stream` switches the child to `run_streamed` and pumps events through a queue with a producer/consumer contract (`src/agents/agent.py:887-958`, `src/agents/util/_asyncio_tasks.py:117-152`), keeping slow handlers from blocking event consumption while guaranteeing drain-before-propagation on errors.

## Notable Patterns

- **Provenance metadata on every artifact**: `ToolOrigin(type=AGENT_AS_TOOL, agent_name, agent_tool_name)` flows onto tool-call/output items and survives RunState serialization (`src/agents/tool.py:293-337`), enabling observability consumers to distinguish agent-tool traffic.
- **Scope-qualified identities**: all nested-state lookups take a `scope_id` so independently restored `RunState`s don't collide on identical tool calls (`src/agents/agent_tool_state.py:87-91`).
- **Fail-closed ambiguity checks**: duplicate nested invocation identities raise `UserError` instead of guessing (`src/agents/tool_context.py:179-191`).
- **Derived-name normalization with collision policy**: `Refund`/`refund` agent names deriving the same tool name is caught at planning time with actionable messages (`src/agents/_tool_identity.py:431-538`).
- **Contract-tested behavior**: the dedicated suite covers extractor edge cases (falsey outputs, multi-segment streamed text settling), sync handlers, non-blocking dispatch, and replaced-tool policies (`tests/test_agent_as_tool.py:576-624`, `tests/test_agent_as_tool.py:3011-3160`, `tests/test_agent_as_tool.py:3430-3501`).

## Tradeoffs

- **Shared `Usage` object** gives correct roll-up totals for free but sacrifices per-child attribution; splitting later requires reconstructing ownership from request entries.
- **Process-local nested-result registry** keeps the hot path simple and leak-free within one process, but means nested interruption state is only durable insofar as it has been copied into an outer `RunState` checkpoint (`src/agents/result.py:175-219`); a crash mid-turn loses the in-flight child unless a checkpoint was taken.
- **String-typed child outputs to the model** maximize model compatibility and simplicity, at the cost of no schema validation between parent and child outputs (contrast with function tools' `output_json_schema`, `src/agents/tool.py:521-522`).
- **Generic-pipeline reuse** means agent-tool complexity is spread across `tool_execution.py`, `turn_resolution.py`, `tool_context.py`, and `agent_tool_state.py`; the feature is powerful but reading any single file gives an incomplete picture.

## Failure Modes / Edge Cases

- **Unbounded recursion depth** for cyclic compositions (self-tool or mutual A↔B tools): no detection; terminates only via per-level `max_turns`, resource exhaustion, or cancellation (absence verified by search; see Question 4).
- **Ambiguous nested approval identity** raises `UserError` rather than mis-routing (`src/agents/tool_context.py:179-191`); legacy binding reconstruction is gated behind an explicit compatibility flag (`src/agents/agent.py:788`).
- **Slow/broken `on_stream` handlers**: exceptions are logged and swallowed so they don't fail the tool call (`src/agents/agent.py:909-925`, tested `tests/test_agent_as_tool.py:3150+`), but parent cancellation still reraises promptly (`tests/test_agent_as_tool.py:2837-2928`).
- **Stale/replaced agent tools across resume**: replacement before `RunState` restore surfaces a specific error instructing restoration of the original configuration (`src/agents/run_internal/turn_resolution.py:2096-2100`, marked TODO for persisting owner identity).
- **Cached-resume usage detachment**: deep-copying usage for top-level checkpoints would silently drop nested post-resume turns; explicitly rebound at `src/agents/agent.py:879-884` with a regression test.
- **Nested failures**: converted to model-visible strings by default (`failure_error_function=default_tool_error_function`, `src/agents/agent.py:599`); setting `None` opts into exception propagation to fail the parent run (`tests/test_agent_as_tool.py:3335-3377`).
- **No tool guardrails on agent tools**: `Agent.as_tool()` does not expose `tool_input_guardrails`/`tool_output_guardrails` parameters (`src/agents/agent.py:583-605`; documented limitation at `docs/guardrails.md:65`).

## Future Considerations

- Add a recursion-depth counter or cycle detector over the agent-tool graph (construction-time check using existing `_agent_instance` back-references, `src/agents/tool.py:556`) to complement per-level `max_turns`.
- Optional per-child usage ledger: tag `RequestUsage` entries with the nested invocation identity (`AgentToolInvocation` already has the needed fields, `src/agents/result.py:67-79`).
- Typed output contract for agent tools, mirroring `output_json_schema` on function tools, so parents can validate child results.
- Expose tool-guardrail parameters on `as_tool()` for parity with function tools.
- Persist agent-tool owner identity in `RunState` to remove the replacement-error TODO (`src/agents/run_internal/turn_resolution.py:2096-2100`).

## Questions / Gaps

- **No evidence found** for any recursion/cycle guard over agent-as-tool composition. Searched `recursion|recursive|cycle|depth|nesting|infinite` across `src/agents/**` (source and tests); only unrelated hits (schema copying, handoff-graph BFS, docstring prose).
- **No evidence found** for per-child cost attribution or child-scoped budgets (token/wall-clock) distinct from `RunConfig` inheritance (`tests/test_agent_as_tool.py:873-1023` shows config inheritance/override, not cost scoping).
- Workflow-as-tool specifically: no separate "workflow" abstraction is exposed as a tool; composition is expressed purely through `Agent.as_tool()`. Adjacent wrappers exist (Codex CLI as tool, `src/agents/extensions/experimental/codex/codex_tool.py`; hosted multi-agent beta, `src/agents/extensions/experimental/hosted_multi_agent/model.py`) but were not audited in depth here.
- Whether the module-global nested-result registry behaves correctly under multi-event-loop or multiprocessing deployments is undocumented; the scope-id mechanism addresses cross-state collisions within a process, not cross-process sharing.

---

Generated by `Dimension 04.08: Agent-as-Tool and Workflow-as-Tool Composition` against `openai-agents-sdk`.
