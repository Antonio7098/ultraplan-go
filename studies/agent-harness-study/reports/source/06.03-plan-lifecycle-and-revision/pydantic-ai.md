# Source Analysis: pydantic-ai

## Dimension 06.03: Plan Lifecycle and Revision

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python — `pydantic-ai-slim`, `pydantic-graph`, `pydantic-evals` (async, type-hinted, uv workspace) |
| Analyzed | 2026-08-27 |

## Summary

`pydantic-ai` does not define a first-class `Plan` artifact. The framework ships two execution substrates: an **implicit agent loop** (`UserPromptNode → ModelRequestNode → CallToolsNode → loop`) and an **explicit typed graph** (`pydantic_graph`). Both encode control flow as node return values and builder-declared edges rather than a mutable, versioned plan document. Consequently all six dimension steps (creation, update, replanning, completion, persistence, drift) are either absent, delegated to application code, or realized only through generic graph/capability hooks. There is no `Plan` class, no revision history, no justification field, and no drift detector. Message history is the sole durable trace. Durable-execution wrappers (Temporal/DBOS/Prefect) checkpoint the *run* for process recovery, not plan revisions.

## Rating

**Score: 2 / 10 — Absent / Implicit**

No explicit plan model exists. Plan-like behavior is emergent: node return types imply next steps, and capability hooks (`before_node_run`/`after_node_run`/`override_next`) allow ad-hoc redirection. There are no plan creation/update/replanning APIs, no revision log, no abandonment/justification contract, and no drift detection. Graph structure validation and message-history persistence provide only incidental coverage. This matches rubric 1-3: ad-hoc or unsafe for plan-lifecycle questions.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan creation — no Plan type | `rg "class.*Plan"` returns zero hits in `pydantic_ai_slim/pydantic_ai` and `pydantic_graph`; grep for `plan` only hits billing `plan: str`, data-plane prose, and docs phrases | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:1` (no Plan), `pydantic_graph/basenode.py:33` |
| Implicit plan via node returns | `BaseNode.run(ctx) -> BaseNode | End` signature is the plan edge; builder infers destinations from return type hints | `pydantic_graph/basenode.py:37-52`, `pydantic_graph/graph_builder.py:1641-1708` `_edge_from_return_hint` |
| Implicit agent loop wiring | Three canonical agent nodes; `ModelRequestNode` and `CallToolsNode` implement the request→handle→request cycle; `End` terminates | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:501-523` `UserPromptNode`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1120+` `ModelRequestNode`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1815-2298` `CallToolsNode` |
| Plan update / rerouting | `before_node_run` can replace node, `after_node_run`/`on_node_run_error` can replace result (`End`↔node), `GraphRun.override_next`/`AgentRun._sync_graph_state` rewrite internal `_next` | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:567-652`, `pydantic_graph/graph_builder.py:586-605` `override_next/_set_next`, `pydantic_ai_slim/pydantic_ai/run.py:268-278` `_sync_graph_state`, `pydantic_ai_slim/pydantic_ai/run.py:299-364` `_wrap_and_advance` |
| Replanning triggers (model-driven) | No replanning primitive; closest are retry/continuation loops: `consume_output_retry`, `ModelRetry` → `ModelRequestNode` with `RetryPromptPart`, suspended→polling (`MAX_GENERATION_CONTINUATIONS`/`MAX_BACKGROUND_POLLS`) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:362-380` `GraphAgentState.consume_output_retry`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1797-1811` `_build_retry_node`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:949-984` continuation ceilings |
| Completion validator | `End` wrapper signals completion; `GraphRun.output` returns value iff `_next is EndMarker`; `CallToolsNode._handle_final_result` appends `ModelRequest` with tool returns then returns `End(FinalResult)`; output validators run via `run_output_with_hooks` | `pydantic_graph/basenode.py:61-66` `End`, `pydantic_graph/graph_builder.py:616-625` `GraphRun.output`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2276-2296` `_handle_final_result`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2224-2250` `_handle_text_response` |
| Abandon / cancellation | `AgentRun.cancel()` drives `RunCancellation.cancel()`; `GraphRun` handles `ErrorMarker`→`override_next` recovery and `EndMarker` early-out with sibling cancellation; cancellation is terminal (hooks cannot recover to success) | `pydantic_ai_slim/pydantic_ai/run.py:555-584` `cancel`, `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:536` `wrap_run` cancellation note, `pydantic_graph/graph_builder.py:686-707` `ErrorMarker`/`EndMarker` handling, `pydantic_graph/graph_builder.py:1096-1106` `_cancel_sibling_tasks` |
| Persistence of trajectory vs plan | `GraphAgentState.message_history: list[ModelMessage]` is the durable trace; `all_messages_json`/`ModelMessagesTypeAdapter.dump_json` serializes it; `mcp_tool_defs_cache`, `run_id`/`conversation_id`, `pending_messages` are per-run ephemeral; no plan revision store | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:299-336` `GraphAgentState`, `pydantic_ai_slim/pydantic_ai/run.py:636-700` `AgentRunResult.all_messages`, `pydantic_ai_slim/pydantic_ai/messages.py:1980+` (provider metadata helpers) |
| Durable execution does not version plans | AGENTS guidance: keep model messages out of workflow state; persist `plans, next step, joins, pending actions, counters, domain progress` in *typed workflow record or durable runtime*; pass only `message_history` to model — explicitly application-owned | `pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:176`, `pydantic_ai_slim/pydantic_ai/durable_exec/AGENTS.md:1` (separate seams) |
| Drift / staleness checks | Build-time graph validation (`_validate_graph_structure` checks 1..5: start edges, end reachability, dead ends, reachability) but no runtime plan-drift detector; `_clean_message_history` repairs dangling tool calls, not plan divergence | `pydantic_graph/graph_builder.py:1752-1860` `_validate_graph_structure`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:531-558` `_repair_dangling_tool_calls` context |
| Revision justification absent | No justification/reason field on any override path; `after_node_run` returns `NodeResult` without `reason`; `ModelRetry.message` is the only user-visible justification and it is for model retry, not plan change | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:576-594` `after_node_run` signature, `pydantic_ai_slim/pydantic_ai/exceptions.py:1` `ModelRetry` |
| Docs explicitly say graph is not a checkpoint replacement | `pydantic_graph is not a checkpoint replacement: current graph APIs do not supply LangGraph-style built-in persistence. Wrap the workflow in a durable runtime or persist transitions in application code.` | `pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:180` |

## Answers to Dimension Questions

### 1. Can plans change?

**Yes, but only as implicit control flow, not as a mutable Plan artifact.**

There is no `Plan` type to mutate. Change happens via (a) node `run` returning a different `BaseNode` subclass or `End` (`pydantic_graph/basenode.py:37`), (b) `GraphBuilder` edge rewiring at build time (`pydantic_graph/graph_builder.py:1408-1486`), and (c) runtime hook redirection: `before_node_run` can substitute the node, `after_node_run`/`on_node_run_error` can substitute the result and `AgentRun._sync_graph_state` calls `GraphRun.override_next` to rewrite the runner's pending step (`pydantic_ai_slim/pydantic_ai/run.py:268-278`, `pydantic_graph/graph_builder.py:586-598`). Capability-driven tool filtering (`prepare_tools`/`prepare_output_tools` in `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:438-475`) also changes what the model can reach next turn, which is functionally a plan mutation. All are imperative and unversioned.

### 2. Are changes justified?

**No systematic justification.**

`after_node_run` and `on_node_run_error` return only `NodeResult` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:576-652`) with no `reason`/`justification` field. `wrap_node_run` can short-circuit past the graph without logging why (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:596-632`). `GraphRun.override_next` records new `GraphTaskRequest`s but not *why* the override happened (`pydantic_graph/graph_builder.py:586-605`). The only justification-adjacent mechanism is `ModelRetry.message` feeding a `RetryPromptPart` to the model (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1806-1810`), which is model-facing retry feedback, not a plan-revision audit trail. Instrumentation spans carry per-step tool definitions but not plan-change reasons (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:386-407`).

### 3. Is old plan history preserved?

**No. Only message history is preserved; plan revisions are not.**

`GraphAgentState.message_history` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:303`) plus `run.py:AgentRunResult.all_messages` is the durable artifact, serializable via `ModelMessagesTypeAdapter.dump_json` (`pydantic_ai_slim/pydantic_ai/run.py:636-667`). `pending_messages`, `event_stream_buffer`, and `mcp_tool_defs_cache` are per-run and reconstructed on replay (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:336-344`). Graph structure (`Graph.nodes`, `edges_by_source`) is immutable after `GraphBuilder.build()` (`pydantic_graph/graph_builder.py:1711-1749`) and not versioned per run. No table, file, or event stream stores prior plan versions. Durable runtimes re-execute activities but do not retain a plan diff log; the docs explicitly tell adopters to persist plans themselves (`pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:176`, `pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/CONCEPT-MAPPING.md:155`).

### 4. Can the agent abandon a plan?

**Yes — via early `End`, exception, or cancellation — but without a dedicated abandonment contract.**

A node can `return End(data)` at any time (`pydantic_graph/basenode.py:61`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2276-2296`). A capability's `after_node_run` can convert a pending node result into `End` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:576-594` + `pydantic_ai_slim/pydantic_ai/run.py:358-363`). `AgentRun.cancel()` (`pydantic_ai_slim/pydantic_ai/run.py:555-584`) and external `asyncio.Task.cancel()` are terminal — hooks may observe via `wrap_run`/`wrap_node_run` but cannot recover to success (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:536-537`, `pydantic_ai_slim/pydantic_ai/run.py:569-570`). `GraphRun` cancels siblings on `EndMarker` (`pydantic_graph/graph_builder.py:700-706`). None of these emit a structured "abandoned plan" event or require a justification.

### 5. Can plan drift be detected?

**No runtime drift detection exists.**

Build-time validation (`pydantic_graph/graph_builder.py:1752-1860`) detects structural issues (no start edges, unreachable nodes, dead ends, end not reachable) but not divergence of an in-flight run from an authored plan. At runtime the only guard is `_clean_message_history` repairing dangling tool calls (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:531-558`) and `_check_continuation_usage` enforcing token limits mid-continuation (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:875-894`). There is no comparison of actual step sequence versus intended plan, no staleness timestamp, no `plan_version` check, and no alert. `ProcessHistory` / `before_model_request` can rewrite `ModelRequestContext.messages` arbitrarily (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:686-692`) with no drift flag.

## Architectural Decisions

- **No Plan primitive — graph is the plan.** The framework deliberately offers strong primitives (typed nodes/edges, `GraphBuilder`, `BaseNode.run` return-type inference in `pydantic_graph/graph_builder.py:1641-1708`) instead of an opinionated plan schema. This keeps the library lightweight but pushes plan-versioning to the application (see `pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:180` which says `pydantic_graph is not a checkpoint replacement`).

- **Hooks over state machine.** Mutations flow through `AbstractCapability` hooks and `GraphRun.override_next` (`pydantic_ai_slim/pydantic_ai/run.py:268-278`, `pydantic_graph/graph_builder.py:586-605`) rather than a versioned plan store. This enables composable middleware but makes plan changes invisible unless the hook author logs explicitly.

- **Message history as the only durable log.** `GraphAgentState.message_history` plus `ModelMessagesTypeAdapter` is the wire-truthful record (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:299-310`, `pydantic_ai_slim/pydantic_ai/run.py:646-667`). Plans, next-step, joins, and counters are out-of-scope for framework persistence (`pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/WORKAROUND-RECIPES.md:176`).

- **Durable execution is workflow-hosted, not plan-hosted.** Temporal/DBOS/Prefect wrappers (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/`, `dbos/`, `prefect/`) durable-sleep via `set_agent_graph_sleep` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:137-177`) and dispatch `model.request` through activities/tasks, but they checkpoint the *execution*, not a user-visible plan diff.

- **Completion via `End` sentinel.** Both `pydantic_graph` (`pydantic_graph/basenode.py:61-66`) and the agent graph (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2276-2296`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2224-2250`) treat `End` as terminal; `GraphRun.output` (`pydantic_graph/graph_builder.py:616-624`) and `AgentRun.result` (`pydantic_ai_slim/pydantic_ai/run.py:144-161`) expose it. No separate completion validator object.

## Notable Patterns

- **Return-type-driven edge inference.** `GraphBuilder.node()` reads `BaseNode.run` return annotations to synthesize `Decision` edges automatically (`pydantic_graph/graph_builder.py:1586-1619`, `pydantic_graph/graph_builder.py:1641-1708`) — a typed alternative to hand-wired transitions.

- **`GraphRunContext` + `RunContext` layering.** Graph-level `GraphRunContext[StateT, DepsT]` (`pydantic_graph/basenode.py:23-31`) and agent-level `RunContext[DepsT]` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:3338-3365`) co-exist; `build_run_context` bridges them for hook access.

- **ErrorMarker→override_next recovery pattern.** `GraphRun.__anext__` yields `ErrorMarker` instead of raising (`pydantic_graph/graph_builder.py:688-693`); caller recovers by sending `Sequence[GraphTaskRequest]` or `EndMarker` via `override_next` (`pydantic_graph/graph_builder.py:586-598`). `AgentRun._wrap_and_advance` mirrors this with `on_node_run_error` (`pydantic_ai_slim/pydantic_ai/run.py:332-353`).

- **Hook-ordered capability chain.** `CombinedCapability` topologically sorts capabilities (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:337-388`) and fires `before_node_run → wrap_node_run → on_node_run_error → after_node_run` (`pydantic_ai_slim/pydantic_ai/run.py:368-392`) — plan changes are interleaved here without a central ledger.

- **Tool-availability deltas as control state.** `ToolReturn(tools=[...])` → `ToolAvailabilityDeltaPart` records deferred-capability reveals in message history (`pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/ON-DEMAND-CAPABILITIES.md:9`, `pydantic_ai_slim/pydantic_ai/models/__init__.py:717-751`) — the only in-history record of a plan-capability change, but it logs names not plan diffs.

- **Mermaid rendering for observability.** `Graph.render()` / `__str__` emit `stateDiagram-v2` (`pydantic_graph/graph_builder.py:363-386`, `docs/graph.md:192-205`) — the graph itself is visualizable, but runtime traversals are not diffed against it.

## Tradeoffs

- **Generality vs plan auditability.** By not enshrining a Plan schema, the library stays unopinionated and composable for any workflow, but loses the ability to answer "why did the plan change?" or "what was the previous plan?" without application scaffolding.

- **Immutability vs evolvability.** `Graph` is frozen after `build()` (`pydantic_graph/graph_builder.py:1711-1749`) which simplifies reasoning and validation, but mid-run graph mutation requires building a new graph instance or hook shims rather than a first-class `revise_plan()` call.

- **Flexibility vs determinism.** `before_model_request` can arbitrarily rewrite `ModelRequestContext.messages` and `model_request_parameters` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:686-692`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1515-1523`); this enables prompt engineering but permits silent plan divergence with no guard.

- **Durability delegated outward.** Relying on Temporal/DBOS/Prefect for checkpointing avoids baking a storage backend into the slim package, but means open-source users without those integrations get no plan/history durability beyond in-memory `message_history` + manual `ModelMessagesTypeAdapter` serialization.

- **Hook-centric extensibility.** Capabilities can intercept every lifecycle point (`wrap_run`, `wrap_model_request`, `before_tool_execute`, etc.), yet there is no typed `PlanRevisionEvent` — consumers must invent their own event schema if they need one.

## Failure Modes / Edge Cases

- **Silent hook shadowing.** Two capabilities both overriding `after_node_run` can overwrite each other's redirection (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:348-357` runs sequentially, last writer wins) with no conflict warning or justification trail.

- **Short-circuit without teardown.** `wrap_node_run` that never calls `handler(node)` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:596-632`) skips the node entirely and `AgentRun._graph_reflects` must later reconcile via `_sync_graph_state` — if the hook forgets to sync, `GraphRun.next_task` and `AgentRun.result` diverge (`pydantic_ai_slim/pydantic_ai/run.py:299-353` guards this but only for known paths).

- **`run_stream` mid-stream completion skips hooks.** The final `ModelRequestNode` under `run_stream()` fires only `before_node_run`, not `wrap_node_run`/`after_node_run` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:618-619`, `docs/hooks.md:138`) — a plan-change hook placed in `after_node_run` will not fire for the termination step, causing inconsistent revision logging.

- **Resumed suspended response mishandling.** `_split_resume_seed` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:859-872`) and `_prepare_resume_request` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1625-1725`) require the history to end in a `state=='suspended'` `ModelResponse`; providing a new `user_prompt` on top of a suspended turn raises `UserError` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:620-627`), and attempting to handle a suspended response in `CallToolsNode` also raises (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1898-1907`) — easy to trip without docs.

- **Lost plan on cancellation race.** `AgentRunEvents._attach_run_state` rebuilds history from live `agent_run.ctx` after drain (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:246-261`), but if the run never started `agent_run` is `None` and the snapshot is empty (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:254-256`).

- **In-place message mutation breaks history.** `exceptions.py:669-682` warns about `HistoryMutationWarning` when user code mutates `ModelMessage.parts` in place; such mutations can silently corrupt `message_history` persistence and downstream plan reconstruction.

- **Stale `pending_messages` on concurrent `enqueue`.** `AgentRun.enqueue` docstring warns `queue[:] = remaining` is not atomic against concurrent appends from another thread (`pydantic_ai_slim/pydantic_ai/run.py:514-526`), so plan-injected messages can be dropped without error.

## Future Considerations

- **Introduce a typed `Plan`/`PlanRevision` value object** (even if optional) with `id`, `version`, `parent_version`, `reason`, and `timestamp` so `after_node_run`/`on_node_run_error` overrides can be justified and audited. Keep it application-optional to preserve the lightweight posture, but provide a reference implementation and a `capabilities.PlanTracking` helper.

- **Emit structured plan lifecycle events.** Add `PlanCreatedEvent`, `PlanRevisedEvent(reason)`, `PlanAbandonedEvent`, `PlanCompletedEvent` analogous to `AgentStreamEvent` (`pydantic_ai_slim/pydantic_ai/messages.py:2580+`) and surface them via `wrap_run_event_stream`. This would let UI adapters and Logfire render revision history without inferring it from messages.

- **Persist revision history via an opt-in capability** backed by `ModelMessagesTypeAdapter` serialization plus a small plan store (e.g. SQLite or the existing workflow-store seam mentioned in `docs/graph.md:512-540`). Document the contract alongside `conversation_id`/`run_id` resolution (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:238-296`).

- **Add runtime drift detection hook.** A `before_node_run` guard that compares `ctx.state.run_step` / `GraphRun.next_task` against an expected plan trajectory (provided by the app) and emits a warning or raises when they diverge — similar to `_validate_graph_structure` but for live execution.

- **Make `run_stream` termination consistent.** Either fire `after_node_run`/`wrap_node_run` for the terminal node or document a migration path so a single plan-revision hook works under both `run()` and `run_stream()` (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:618` note).

- **Unify justification shape with `ModelRetry`.** If a `reason` field is added to plan revisions, align its redaction/serialization with the existing `RetryPromptPart` handling in `process_tool_calls` to avoid leaking sensitive plan rationale to the model.

## Questions / Gaps

- **No evidence of first-class plan status transitions.** Searched `AbstractCapability`, `GraphRun`, `AgentRun`, `AgentGraphSleep`, and docs for `status`, `state` transitions tied to a plan — found only `ModelResponseState` (`pydantic_ai_slim/pydantic_ai/messages.py:127-146`) and `ModelRequestState` (`pydantic_ai_slim/pydantic_ai/messages.py:148-149`), which track wire state, not plan status. If a plan-status FSM exists outside `pydantic-ai-slim`/`pydantic-graph` it was not in the studied source.

- **No evidence of plan completion validators beyond output-schema checks.** `output_validators` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:407-408`) validate `OutputDataT` shape, not plan completeness criteria (e.g. task checklist). Whether an output that type-checks but violates a business plan invariant is caught is unanswered — search of `output.py` and `capabilities` found no plan-aware validator.

- **Whether plan revisions survive process kill without durable wrapper.** `docs/graph.md:180` and `WORKAROUND-RECIPES.md:180` state that `pydantic_graph` is not a checkpoint. No crash-injection test for plan recovery was found in `tests/` (grep for `crash`, `restart`, `replay` in `tests/` returned only durable-exec tests). Confirmation would require a dedicated persistence test.

- **No observable plan drift metric.** No counter, gauge, or Logfire span attribute for drift was found in `pydantic_ai_slim/pydantic_ai/_instrumentation.py:1` (spans are model/tool-centric). This gap is structural given the absence of a plan type.

---

Generated by `Dimension 06.03: Plan Lifecycle and Revision` against `pydantic-ai`.
