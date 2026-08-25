# Source Analysis: pydantic-ai

## 03.04 Planner/Executor Separation

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python 3.10-3.13; uv monorepo with three packages: `pydantic_ai_slim` (agent loop, models), `pydantic_graph` (typed FSM engine), `pydantic_evals` (eval framework); uses `pydantic`, async/await, OpenTelemetry |
| Analyzed | 2026-07-14 |

## Summary

Pydantic AI ships **no first-class planner/executor architecture in the core framework**. The agent loop is a small, fully-declarative FSM (`pydantic_graph.Graph`) with three node types — `UserPromptNode`, `ModelRequestNode`, `CallToolsNode` — chained as `start → UserPromptNode → ModelRequestNode ⇄ CallToolsNode → End` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`). There is no separate "planner" agent, no persisted task list, no plan schema, no plan-update tool, and no replanning primitive anywhere in `pydantic_ai_slim/`, `pydantic_graph/`, or `pydantic_evals/`.

What the framework does provide is a **plan-by-convention** substrate: the model emits free-form text and tool calls inside `ModelRequestNode` → `CallToolsNode`, with the next loop iteration decided by the model's own response. The "plan", to the extent one exists, lives only inside the model's context window (system prompt + message history). Two structural preconditions for a real planner are absent:

1. The graph topology is fixed by `build_agent_graph` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121-2139`); only four node classes are registered. There is no way to inject a `PlannerNode` into the agent's loop.
2. `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-180`) — the per-run state object — carries `message_history`, `usage`, `output_retries_used`, `run_step`, `run_id`, `conversation_id`, `metadata`, `pending_messages`. There is **no `plan` field, no task list, no todo, no todo-version counter, no plan-tracker**.

The framework explicitly delegates task-planning to third-party code: `docs/toolsets.md:898-902` recommends `pydantic-ai-todo` (by Vstorm) as a third-party `TodoToolset` with `read_todos`/`write_todos`, included in `pydantic-deep`. The `docs/harness/overview.md:11-20` page positions task planning as a "capability still finding its final shape" that lives in the separate `pydantic-ai-harness` package, not in core. Two test-only files implement `TodoWrite`/`ExitPlanMode` shims (`.github/scripts/pydantic_ai_gh_aw_shim/todo_write.py:19-22`, `.github/scripts/pydantic_ai_gh_aw_shim/exit_plan_mode.py:6-13`) but these are headless CI stubs for the GitHub Action wrapper, not framework features.

Failure handling is **node-local**, not plan-local. `output_retries_used` (`_agent_graph.py:127,162-179`) caps output-validation retries at `max_output_retries`; tool retries are managed per-call by `ToolManager._check_max_retries` (`_agent_graph.py:174`). `ModelRetry` from a validator (`_agent_graph.py:829-835, 979-982, 1027-1040`) inserts a `RetryPromptPart` into the next `ModelRequest` and re-enters `ModelRequestNode` — that is the entire "replanning" loop, and it is implicit in the model loop rather than explicit in any plan schema.

Traceability is therefore per-step, not per-plan: `RunContext.run_step` (`pydantic_ai_slim/pydantic_ai/_run_context.py:78-79`) is a monotonically increasing counter, and `agent_run.all_messages()` (`run.py:159-164`) returns the full message log. But the framework **cannot compare what it planned to what it actually did**, because nothing is ever recorded as the "plan" in the first place.

## Rating

**2 / 10 — Absent.** No plan schema, no planner agent, no plan-update tool, no replanning event, no plan-comparison metric. Planning exists only as a model-driven convention: the model is told (via system prompt / instructions) to think in steps, and the framework captures every step in `message_history`. There is no built-in way to know "what was the plan before step N", only "what messages did the model see and produce by step N". Because planning is a user convention layered on top of the loop, the system **cannot compare what it planned to what it actually did** unless the user also writes the bookkeeping (e.g., via a third-party `TodoToolset` or custom `instructions`).

## Evidence Collected

Every entry includes file path and line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| The agent graph is a 3-node FSM, no planner node | `build_agent_graph` registers exactly `UserPromptNode`, `ModelRequestNode`, `CallToolsNode`, `SetFinalResult` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139` |
| Graph topology wiring (edges from start) | `g.edge_from(g.start_node).to(UserPromptNode[...]); g.node(...)` for each | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2130-2138` |
| Graph instantiation in `Agent.run` | `graph = _agent_graph.build_agent_graph(...)` builds the per-run graph | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1273` |
| Per-run state has no plan field | `GraphAgentState` fields: `message_history, usage, output_retries_used, run_step, run_id, conversation_id, metadata, last_max_tokens, last_model_request_parameters, pending_messages` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-144` |
| State dataclass construction (no plan arg) | `state = _agent_graph.GraphAgentState(message_history=..., usage=..., output_retries_used=0, run_step=0, conversation_id=...)` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:1277-1283` |
| No `plan`, `todo`, or `task_list` in `GraphAgentState` | grep of `_agent_graph.py` returns 0 hits for `plan\|todo\|TaskList\|planner` (excluding `# TODO` comments) | search boundary |
| Loop control: model decides what runs next | `ModelRequestNode._finish_handling` always returns either `CallToolsNode(response)` or `End(...)` — there is no planner-mediated branch | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:964-990` |
| Loop control: tool outcome drives re-request or end | `CallToolsNode._handle_tool_calls` returns `ModelRequestNode(...)` or `End(...)`; `_handle_final_result` returns `End` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1260-1379` |
| No plan schema, plan record, or planner agent in `pydantic_graph` | grep of `pydantic_graph/` returns 0 hits for `Planner\|TodoList\|TaskList\|PlanStep\|plan_record` | search boundary |
| `pydantic_graph` exposes no planner primitive — only `BaseNode`, `Graph`, `Decision`, `Fork`, `Join` | `pydantic_graph/pydantic_graph/__init__.py` public surface | `pydantic_graph/pydantic_graph/__init__.py` |
| `Decision` is a control-flow primitive (branch on type/condition), not a planner | "Decision node for conditional branching in graph execution… evaluates conditions and routes execution" | `pydantic_graph/pydantic_graph/decision.py:40-66` |
| Replanning on validation failure = insert `RetryPromptPart` and re-run | `_build_retry_node` builds `ModelRequestNode([RetryPromptPart])` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1027-1040` |
| Output-retry budget consumed on `ModelRetry` / `ToolRetryError` / empty-response | `consume_output_retry` increments `output_retries_used` and raises `UnexpectedModelBehavior` past `max_output_retries` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:162-179` |
| Empty-response fallback = resubmit the most recent request as an empty `ModelRequest` | "essentially resubmit the most recent request that resulted in an empty response, … in the hope the model will return a non-empty response this time" | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1166-1173` |
| Thinking-only / no-actionable-content fallback = `RetryPromptPart` with hints | "`Please {" or ".join(alternatives)}.`" `RetryPromptPart` (`include your response in a tool call`, `call a tool`, …) | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1243-1249` |
| `SkipModelRequest` short-circuits the model call entirely | `except exceptions.SkipModelRequest as e: ... return await self._finish_handling(ctx, e.response)` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:617-633, 788-794` |
| Per-step counter on `RunContext` (telemetry, not plan versioning) | `run_step: int = 0`, "The current step in the run" | `pydantic_ai_slim/pydantic_ai/_run_context.py:78-79` |
| Per-step counter bumped on each `_prepare_request` | `ctx.state.run_step += 1` | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:856` |
| Toolset refresh per step (not plan refresh) | `ctx.deps.tool_manager = await ctx.deps.tool_manager.for_run_step(run_context)`; `for_run_step` is the per-step hook | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:872`; `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:118-124` |
| Per-step callable model settings (decision logic stays in user code) | `get_model_settings` may return a callable receiving `RunContext` so capability-supplied settings can vary per step | `pydantic_ai_slim/pydantic_ai/_run_context.py:98`; `docs/tools-advanced.md:397` |
| Traceability = message log + step counter, not plan-vs-actual | `agent_run.all_messages()` returns the full message history; `ctx.run_step` records the step | `pydantic_ai_slim/pydantic_ai/run.py:159-164`; `pydantic_ai_slim/pydantic_ai/_run_context.py:78-79` |
| `MessageHistory` is the only persistent state across turns (passed in as `message_history=...` or via `conversation_id`) | `message_history: Sequence[ModelMessage] \| None` is the only cross-run carrier | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1330-1348` |
| Snapshot/persistence is per-graph, not per-plan | `BaseStatePersistence.snapshot_node` records `(state, next_node)`; no plan field | `pydantic_graph/pydantic_graph/persistence/__init__.py:113-123` |
| Graph snapshot payload | `NodeSnapshot(state, node, start_ts, duration, status, kind, id)` | `pydantic_graph/pydantic_graph/persistence/__init__.py:44-67` |
| Third-party `TodoToolset` recommended for task planning | "Toolsets for task planning and progress tracking … pydantic-ai-todo - TodoToolset with read_todos and write_todos tools" | `docs/toolsets.md:898-902` |
| Harness package positions planning as out-of-core | "Context management, memory, guardrails, file system access, code execution, multi-agent orchestration -- these are the building blocks you pick and choose" | `docs/harness/overview.md:16` |
| "Deep Agents" planning is described as a community/third-party capability, not core | "the community maintains packages that bring these concepts together … pydantic-deep" | `docs/multi-agent-applications.md:338-340` |
| Multi-agent delegation pattern (no planner; sub-agent is just a tool) | "agents using another agent via tools" | `docs/multi-agent-applications.md:6, 13-77` |
| Sub-agent capability (image-gen, x-search) — fallback to a sub-agent on different model | "Native when supported; falls back to a subagent running an xAI model" | `pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:200-207`; `docs/capabilities.md:585, 598` |
| GitHub Action shim stubs for Claude Code's `TodoWrite` / `ExitPlanMode` (CI-only, not framework) | `todo_write()` returns `'todos recorded (n):'` ack; `exit_plan_mode()` returns `'Plan acknowledged — proceeding with execution.'` | `.github/scripts/pydantic_ai_gh_aw_shim/todo_write.py:19-22`; `.github/scripts/pydantic_ai_gh_aw_shim/exit_plan_mode.py:6-13` |
| Documentation states the graph iterates over fixed node types | "The AgentRun is an async iterator that yields each node (BaseNode or End) in the flow." | `docs/agent.md:356` |
| Documentation explicitly says agent graph = FSM with fixed transitions | "pydantic-graph — an async graph and state machine library for Python where nodes and edges are defined using type hints" | `docs/graph.md:17-19`; `docs/agent.md:293-295` |

## Answers to Dimension Questions

1. **Is there an explicit plan?**
   No. The closest thing is the conversation `message_history` (a `list[ModelMessage]`) held in `GraphAgentState.message_history` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:125`). There is no plan dataclass, no plan field, no `Plan` or `TodoList` symbol anywhere in the framework. The grep search for `Planner|TodoList|TaskList|PlanStep|plan_record` across `pydantic_ai_slim/`, `pydantic_graph/`, and `pydantic_evals/` returns zero hits. The docs explicitly defer planning to third-party packages (`docs/toolsets.md:898-902` recommends `pydantic-ai-todo`; `docs/multi-agent-applications.md:328-340` describes planning as a "Deep Agent" pattern layered on top).

2. **Does the plan control execution?**
   N/A — there is no framework plan. Execution control is held by:
   - The static FSM topology (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2121-2139`),
   - The model's own response (it chooses either tool calls or final output, and the graph branches in `CallToolsNode._run_stream` at `_agent_graph.py:1213-1249`),
   - Per-step capability hooks (`_agent_graph.py:872` `tool_manager.for_run_step`, capability `get_model_settings` callable — `docs/tools-advanced.md:397`),
   - `ModelRetry` / `ToolRetryError` / `SkipModelRequest` exception flows (`_agent_graph.py:617-633, 788-794, 829-835, 979-982, 1027-1040, 1147-1149, 1171`).
   No artifact inside the agent runtime can be called "the plan" because the model emits both planning thoughts (in `ThinkingPart`) and execution actions (in `ToolCallPart`) in the same `ModelResponse`, and the loop never separates them.

3. **Can the plan change?**
   Not as a first-class concept. The model can revise its own intentions across loop iterations because `message_history` is updated each step (`_agent_graph.py:854, 1025, 1370-1377`) and the model re-reads it. But there is no plan-revision event, no plan-diff primitive, no `plan_update` tool registered by the framework. The only structured re-entry to `ModelRequestNode` is via:
   - `_build_retry_node` inserting a `RetryPromptPart` on validator/tool-retry failure (`_agent_graph.py:1027-1040`),
   - Empty-response resubmission (`_agent_graph.py:1171-1173`),
   - `RetryPromptPart` for "no actionable output" (`_agent_graph.py:1243-1249`).
   These are **failure-driven retries**, not replanning; they insert feedback for the model, not a new plan record.

4. **Are plan changes traceable?**
   Only at the granularity of "every `ModelMessage` in the run, in order." `agent_run.all_messages()` (`pydantic_ai_slim/pydantic_ai/run.py:159-164`) returns the complete log, and `ctx.state.run_step` (`pydantic_ai_slim/pydantic_ai/_run_context.py:78-79`) tags each step. The graph-level snapshot/persistence machinery (`pydantic_graph/pydantic_graph/persistence/__init__.py:44-67, 113-123`) records `(state, next_node)` pairs but `state` is `GraphAgentState` which contains no plan. So the framework can replay "what ran, in what order" (BSM-style), but it cannot replay "what was the plan when this node ran" because the plan is never captured as a separate artifact. OpenTelemetry instrumentation (`pydantic_ai_slim/pydantic_ai/_instrumentation.py`) instruments model requests and tool calls, not plan revisions.

5. **Is planning useful or decorative?**
   Decorative at the framework level. Pydantic AI deliberately exposes `Agent.iter()` and `AgentRun.next(node)` (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:1395-1422`; `pydantic_ai_slim/pydantic_ai/run.py:189-227`) so users can build their own planner by intercepting nodes, mutating state, or skipping them — but that is a **user convention**, not a framework capability. The philosophy in `AGENTS.md:52-56` ("strong primitives, powerful abstractions, and general solutions and extension points … over narrow solutions for specific use cases, opinionated solutions that push a particular approach to agent design that hasn't yet stood the test of time") explains why planning is **deliberately not baked in** — the framework waits for a planning pattern to mature in the ecosystem (e.g., `pydantic-deep` / `pydantic-ai-todo`) before promoting it to core.

## Architectural Decisions

- **Plan is the model's responsibility, not the framework's.** `build_agent_graph` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`) defines a 4-node FSM with `validate_graph_structure=False`. There is no `PlannerNode` slot. The model is expected to plan in its own context, and the framework faithfully captures whatever it does.
- **State is a minimal conversation log.** `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-180`) carries only what every step needs to find its next step: `message_history`, `usage`, `run_step`, `output_retries_used`. Anything domain-specific (plans, todos, scratchpads) lives in `RunContext.deps` (`pydantic_ai_slim/pydantic_ai/_run_context.py`) — i.e., the user supplies it.
- **The retry loop is validator/tool driven, not plan driven.** Output retries (`_agent_graph.py:162-179`), tool retries (`ToolManager._check_max_retries` referenced at `_agent_graph.py:173-174`), and `ModelRetry` (`_agent_graph.py:829-835, 1027-1040`) all converge on the same pattern: append a `RetryPromptPart` to the next `ModelRequest` and re-enter `ModelRequestNode`. This is the only "replanning" the framework does, and it is reactive rather than generative.
- **Per-step tool refresh, not per-plan tool refresh.** `for_run_step` (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:118-124`) is invoked at every `ModelRequestNode._prepare_request` (`_agent_graph.py:872`). This is the framework's one persistent hook into per-step state, but it is structural (tool availability) not plan-driven.
- **SkipModelRequest as a "no-plan-needed" path.** Capabilities can raise `SkipModelRequest` (`_agent_graph.py:617-633, 788-794, 992-1013`) to short-circuit the model call when they can produce a response directly. This bypasses the planner implicitly because there is no planner.
- **Persistence is graph-snapshot, not plan-snapshot.** `BaseStatePersistence` (`pydantic_graph/pydantic_graph/persistence/__init__.py:106-225`) snapshots `(state, next_node)` and `End` markers. `GraphAgentState` has no plan, so the snapshot does not either. Replaying from a snapshot (`Graph.iter_from_persistence`, `pydantic_graph/pydantic_graph/graph.py:261-309`) restores the loop's next step, not a plan it was executing.

## Notable Patterns

- **Loop as FSM, planner as user prompt.** `docs/agent.md:293-295` describes the loop as a finite-state machine over `UserPromptNode` → `ModelRequestNode` → `CallToolsNode` → `End`, and the planning behaviour is delegated to the system prompt / instructions. This is the cleanest pattern in the codebase: the framework gives you a state machine; you give it a planner via instructions.
- **"Plan-by-thinking" via `ThinkingPart`.** `CallToolsNode._run_stream` (`_agent_graph.py:1197-1198`) explicitly drops `ThinkingPart` from output aggregation (`pass`) and resets `text` when a native tool call follows text. This indicates the framework recognises that thinking and acting live in the same `ModelResponse` — the planner and the executor are the same model call.
- **Per-step callable settings as a planning seam.** Capabilities can return a callable from `get_model_settings` (`docs/tools-advanced.md:395-434`) so that settings like `tool_choice` vary per step. This is the closest the framework gets to "the planner decided something before the executor ran" — but the decision is encoded as a per-step setting, not as a plan artifact.
- **Agent delegation as a tool call.** `docs/multi-agent-applications.md:13-77, 80-173` shows a parent agent that calls a child agent inside a tool. The "planner" here is the parent agent's tool-selection; the "executor" is the child agent's full loop. Pydantic AI supports this as a pattern, but neither layer is a planner in the strict sense — both are full `Agent` instances running the same FSM.
- **`deferred_loading` / `for_run_step` for dynamic tool plans.** `docs/toolsets.md:799-871` and `pydantic_ai_slim/pydantic_ai/toolsets/_tool_search.py:333` show that the tool *availability* for each step can change dynamically. This is the framework's most concrete "per-step plan mutation" hook, but it operates on the tool list, not on an LLM-generated plan.

## Tradeoffs

- **Pro:** Because there is no plan schema, the framework avoids the failure mode where a planner's stale plan drifts out of sync with what the executor actually did. Every step is reconciled through the message history.
- **Pro:** Minimal state means cheap persistence — `BaseStatePersistence` only needs to serialize `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:121-144`), which is small and stable.
- **Pro:** The `AgentRun` iterator (`pydantic_ai_slim/pydantic_ai/run.py:189-227`) exposes every node, so a user who wants a planner can intercept any node and rewrite state — without paying the abstraction tax of a separate planner layer.
- **Con:** No out-of-the-box way to know "did the model follow its plan?" — the framework has no plan-vs-actual diff. Users must implement that comparison themselves (e.g., by inspecting `message_history` after the run).
- **Con:** Retry budget is a single scalar (`output_retries_used`, `_agent_graph.py:127, 162-179`); there is no way to give the model more retries for one type of failure vs another based on a plan. This means the framework's recovery is uniform, not plan-aware.
- **Con:** Tool-search and capability loading (`pydantic_ai_slim/pydantic_ai/native_tools/_tool_search.py:333`; `pydantic_ai_slim/pydantic_ai/capabilities/AGENTS.md`) dynamically mutate what the model sees, but the model is not informed that its plan needs to be revised. The framework silently reshapes the executor's world.
- **Con:** Multi-step plans that fail in the middle have no first-class resumption. A user who wants "resume from step 3 of 7" must persist the plan themselves (e.g., via `message_history` + a custom `TodoToolset`).

## Failure Modes / Edge Cases

- **Empty / thinking-only responses trigger silent resubmission.** `CallToolsNode._run_stream` (`_agent_graph.py:1101-1173`) detects empty responses and either returns a `ModelRequestNode` with an empty request (`_agent_graph.py:1171-1172`) or a `RetryPromptPart` (`_agent_graph.py:1243-1249`). Both look like retry, not replanning.
- **`finish_reason == 'length'` is treated as a hard failure, not a plan pause.** `GraphAgentState.check_incomplete_tool_call` (`_agent_graph.py:146-160`) raises `IncompleteToolCall`; `CallToolsNode._run_stream` (`_agent_graph.py:1111-1114`) raises `UnexpectedModelBehavior`. There is no way to "extend the plan" when the model runs out of tokens.
- **Content filter triggers an exception, not a replan.** `_agent_graph.py:1117-1130` raises `ContentFilterError` with the response body for diagnostic purposes. No plan adjustment is attempted.
- **`run_step` is monotone but `output_retries_used` is per-run and uncorrelated.** A user who wants "plan-aware" retries (e.g., "this step in my plan already failed twice, abort the run") has no framework hook to express that.
- **Graph snapshots cannot be replayed into a different plan.** `pydantic_graph/pydantic_graph/persistence/__init__.py:113-123` snapshots `(state, next_node)`; restoring gives you back the loop at step N. If the user's plan was stored only in `message_history` (e.g., a TodoWrite call), restoration works because history is part of state. If the plan was stored only in `RunContext.deps` (which is not snapshotted in `GraphAgentState`), restoration loses it.

## Future Considerations

- A `PlannerNode` could be added to `build_agent_graph` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`) by inserting a new edge before `ModelRequestNode` and registering a new node type. This would require designing a plan schema (probably a typed `list[PlanStep]` in `GraphAgentState`) and a plan-update tool, neither of which exists today.
- The `pydantic-ai-harness` package (`docs/harness/overview.md:1-20`) is positioned as the testing ground for capabilities like planning, file ops, and code execution. If a planning pattern stabilizes there (as `code-mode` is currently moving toward promotion — `docs/harness/overview.md:18`), it could graduate into core.
- The capability abstraction (`pydantic_ai_slim/pydantic_ai/capabilities/AGENTS.md`) already supports per-step model settings via callable (`docs/tools-advanced.md:395-434`). A "Planner" capability could be added as `PlannerCapability(get_model_settings=lambda ctx: {…planner-specific settings…})` and gate tool availability per step via `PrepareTools`. This would let planning be layered without changing `build_agent_graph`.
- The `_enqueue.PendingMessage` mechanism (`pydantic_ai_slim/pydantic_ai/_enqueue.py:61`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:142-144`) is a primitive for injecting messages mid-step; it could be repurposed as a "replan trigger" if a planner concept were introduced.

## Questions / Gaps

- Is there a planned-but-not-yet-released planner node? The `pydantic-ai-harness` package (`docs/harness/overview.md`) is described as the staging area, but it is **not part of this source directory**. Without inspecting that package, I cannot say whether a planner primitive is being prepared for promotion.
- Are there internal `Plan` / `Planner` symbols under a different name? Search boundary: I grepped `pydantic_ai_slim/`, `pydantic_graph/`, `pydantic_evals/`, `clai/`, `examples/`, `docs/`, and `tests/` for `plan|todo|TODO|task_list|planner|sub_agent|subagent|orchestrat` (case-insensitive, excluding `# TODO` comments). The only meaningful hits are: the GitHub Action shim stubs (`.github/scripts/pydantic_ai_gh_aw_shim/todo_write.py`, `exit_plan_mode.py`), the `subagent` mentions in `XSearchTool` / `ImageGeneration` (`pydantic_ai_slim/pydantic_ai/native_tools/__init__.py:200-207`; `docs/capabilities.md:475-476, 585, 598`), the docs recommendations for third-party tools (`docs/toolsets.md:898-902`), and the example's `treatment_plan` (`examples/pydantic_ai_examples/medical_agent_delegation.py:56, 70, 221-237`) which is a domain-specific structured output, not a planner.
- Could `output_retries_used` and `run_step` together approximate a "plan step counter"? No — both are framework-internal scalars; neither carries the meaning "this is the Nth item in the model's plan". A user who wanted this would have to extract it from `message_history` (e.g., by parsing a TodoWrite tool call).
- Is the `pydantic_graph` library itself a planner? No — `pydantic_graph/pydantic_graph/decision.py:40-66` defines `Decision` as a conditional-branching primitive that routes based on input type, not as an LLM-driven planner. `pydantic_graph/pydantic_graph/node.py:60-95` defines `Fork` for parallel branches and `Join` for convergence; neither is a planner. The agent's FSM (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2110-2139`) uses none of these advanced primitives — it is a flat sequence of four nodes.

---

Generated by `03.04-planner-executor-separation` against `pydantic-ai`.