# Source Analysis: openai-agents-sdk

## 06.02 — Task Decomposition Representation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ with `asyncio`; core runtime in `src/agents/`, Codex subprocess extension in `src/agents/extensions/experimental/codex/` |
| Analyzed | 2026-08-27 |

## Summary

OpenAI Agents SDK has **no first-class task decomposition model**. There is no `Plan`, `Todo`, `Task`, or `Subtask` schema with typed fields, status, or dependency edges that a planner or scheduler can manipulate. Decomposition is **implicit and LLM-authored inside the turn loop**: each `Runner.run` iteration (`src/agents/run.py:768`) calls the model, classifies the output into a `ProcessedResponse` (`src/agents/run_internal/run_steps.py:117-152`), builds an ephemeral `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:558-573`), and executes it via `_execute_tool_plan` (`src/agents/run_internal/tool_planning.py:944-1101`). The plan lives for one turn, has no persistence, no stable IDs, no status beyond success/failure of individual tool calls, and no dependency graph.

Handoffs (`NextStepHandoff` at `src/agents/run_internal/run_steps.py:155-157`) provide coarse delegation to a sibling `Agent`, and `Agent.as_tool()` (`src/agents/agent.py` via `src/agents/tool.py`) allows an agent to be invoked as a function tool, but neither is tracked as a typed subtask with `pending|running|blocked|completed` lifecycle. The only artifact that resembles a todo list is the **experimental Codex bridge**: `TodoItem{text, completed}` and `TodoListItem{id, items: list[TodoItem]}` (`src/agents/extensions/experimental/codex/items.py:98-107`) are **read-only observations** parsed from the Codex CLI JSONL stream (`src/agents/extensions/experimental/codex/items.py:226-232`), not a writable plan the SDK creates or schedules.

Multi-step work in examples and docs (e.g., `examples/agent_patterns/README.md:11`, `docs/multi_agent.md:29`) is described as prompt-level guidance to "break down a task into smaller steps" or wiring multiple `Agent`s via handoffs/agents-as-tools, not via a framework-owned plan registry. The SDK therefore cannot answer "which subtask is blocking progress?" — progress is only the turn counter and the `SingleStepResult.next_step` discriminant (`src/agents/run_internal/run_steps.py:184-199`).

## Rating

**Score: 2 / 10 — Absent, implicit, ad-hoc.**

Rationale:
- **No plan/task schema**: exhaustive search for `Todo*`, `PlanStep`, `Task.*depends_on`, `Subtask` across `src/agents/` yields only `ToolExecutionPlan` (per-turn, 8-field dataclass grouped by tool kind, not by logical subtask) and the Codex `TodoItem` (experimental, external). No `TodoStore`, `TodoProvider`, or `Plan` type exists in the main runtime.
- **No status tracking**: `ToolExecutionPlan` has no `status` field; `TodoItem.completed` is binary and read-only; `SingleStepResult.next_step` tracks only the turn outcome (`NextStepRunAgain|Handoff|FinalOutput|Interruption` at `src/agents/run_internal/run_steps.py:199`), not subtask lifecycle.
- **No dependencies**: `ToolExecutionPlan` fields are independent lists; `ProcessedResponse` lists are independent; there is no `depends_on`, `blocked_by`, topological sort, or `get_ready_todos` equivalent.
- **No mapping to tool calls beyond per-turn batch**: tool calls *are* the decomposition, but mapping is 1:1 between model output and plan fields — not a typed subtask that can be reordered, retried, or evaluated.
- **No reorder/evaluation**: no `reorder`, `reprioritize`, `replace_pending`, or `evaluate_plan` API; `_execute_tool_plan` either runs in parallel (`gather_with_cancel` at `src/agents/run_internal/tool_planning.py:984`) or sequential fallback, but order is fixed by model output order.
- **Conclusion for dimension question**: the system cannot tell which subtask is blocking because subtasks do not exist as addressable objects; blocking is only visible as "awaiting model" or "awaiting approval" (`NextStepInterruption`).

The two points (versus 1) are for the explicit per-turn `ToolExecutionPlan`/`ProcessedResponse`/`SingleStepResult` plumbing and the observable `RunItem`/`ModelResponse` audit trail, which together provide a *de facto* step record even if not a plan model.

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Turn-loop driver (no plan) | `AgentRunner.run` outer `while True` dispatches on `turn_result.next_step`; no plan registry | `src/agents/run.py:768-1497` |
| Step discriminant (turn-level only) | `NextStepHandoff`, `NextStepFinalOutput`, `NextStepRunAgain`, `NextStepInterruption` dataclasses; `SingleStepResult.next_step` union | `src/agents/run_internal/run_steps.py:155-199` |
| Model-output classification | `ProcessedResponse{new_items, handoffs, functions, computer_actions, shell_calls, apply_patch_calls, mcp_approval_requests, interruptions}` with `has_tools_or_approvals_to_run()` | `src/agents/run_internal/run_steps.py:116-152` |
| Ephemeral per-turn plan | `@dataclass class ToolExecutionPlan{function_runs, computer_actions, custom_tool_calls, shell_calls, apply_patch_calls, local_shell_calls, pending_interruptions, approved_mcp_responses, mcp_requests_with_callback}` | `src/agents/run_internal/tool_planning.py:558-573` |
| Plan construction (fresh turn) | `_build_plan_for_fresh_turn(processed_response, agent, context_wrapper, ...)` returns `ToolExecutionPlan` by copying `ProcessedResponse` lists verbatim | `src/agents/run_internal/tool_planning.py:619-646` |
| Plan construction (resume) | `_build_plan_for_resume_turn(..., function_runs, computer_actions, ...)` rebuilds plan from approved runs after interruption | `src/agents/run_internal/tool_planning.py:649-682` |
| Plan execution | `_execute_tool_plan(plan, bindings, hooks, ...)` runs typed tool groups via `gather_with_cancel` (parallel) or sequential; returns `function_results, computer_results, ...` | `src/agents/run_internal/tool_planning.py:944-1101` |
| Tool groups are flat lists | `_execute_tool_plan` passes `plan.function_runs -> execute_function_tool_calls`, `plan.shell_calls -> execute_shell_calls`, etc., with no inter-group dependencies | `src/agents/run_internal/tool_planning.py:985-1034` |
| No Todo/Plan schema in core | `grep -rn "class Todo\|TodoList\|PlanStep"` in `src/agents/` returns 0 hits outside `experimental/codex` | `src/agents/run_internal/tool_planning.py:67-86` (exports list shows no Todo/Plan) |
| Experimental Codex todo (read-only) | `@dataclass(frozen=True) class TodoItem(text: str, completed: bool)` and `TodoListItem(id: str, items: list[TodoItem])` | `src/agents/extensions/experimental/codex/items.py:98-107` |
| Codex todo coercion | `coerce_thread_item` parses `type=="todo_list"` into `TodoListItem` via `TodoItem(text, completed)` | `src/agents/extensions/experimental/codex/items.py:226-232` |
| Codex todo is external | `ThreadItem` union includes `TodoListItem` as one of 9 item types emitted by Codex CLI JSONL stream | `src/agents/extensions/experimental/codex/items.py:117-127` |
| Agent definition (no subtask fields) | `class Agent(AgentBase)` with `name, instructions, tools, handoffs, mcp_servers, model_settings` — no `tasks`, `plan`, `todos`, `depends_on` | `src/agents/agent.py:180-450` |
| RunItem step records (turn history, not plan) | `RunItem` union (`MessageOutputItem`, `ToolCallItem`, `ToolCallOutputItem`, `HandoffCallItem`, `ReasoningItem`, `ToolApprovalItem`, etc.) collected per step into `SingleStepResult{pre_step_items, new_step_items, processed_response}` | `src/agents/items.py:686-740`, `src/agents/run_internal/run_steps.py:184-232` |
| Handoff as coarse delegation (no subtask status) | `ToolRunHandoff{handoff: Handoff, tool_call}` + `NextStepHandoff{new_agent}` — single delegation, no `status` or `depends_on` | `src/agents/run_internal/run_steps.py:62-65`, `src/agents/run_internal/run_steps.py:155-157` |
| Doc guidance is prompt-level only | "A common tactic is to break down a task into a series of smaller steps. Each task can be performed by an agent" — example prose, no framework API | `examples/agent_patterns/README.md:11` |
| Multi-agent doc: agents-as-tools vs handoffs | "Use agents as tools when specialist should help with bounded subtask but should not take over conversation" — pattern guidance, no subtask type | `docs/multi_agent.md:29-31` |
| Agent-as-tool composition | `Agent` can be wrapped as `FunctionTool` via tool builders; invocation tracked via `RunContextWrapper` approval bindings, not plan status | `src/agents/tool.py`, `src/agents/run_internal/tool_planning.py:330-336` |
| Search boundary: no plan update tools | `__all__` in `tool_planning.py` exposes `_build_plan*`, `_execute_tool_plan`, collectors — no `add_todo`, `complete_todo`, `reorder`, `evaluate` | `src/agents/run_internal/tool_planning.py:67-86` |
| Tests pin turn discriminant, not plan | `tests/test_run_step_execution.py`, `tests/test_agent_runner.py`, `tests/test_agent_runner_streamed.py` assert `next_step` variants, not plan decomposition | `tests/test_run_step_execution.py:2749-2780` (from prior 01.01 analysis) |
| Search boundary: no dependency graph | No `depends_on`, `blocked_by`, `get_ready_todos`, `can_parallelize`, `topological` in `src/agents/run_internal/` outside tool-call deduping (`_dedupe_processed_response_invocations` at `src/agents/run_internal/tool_planning.py:339-554` is identity deduping, not dependency scheduling) | `src/agents/run_internal/tool_planning.py:339-554` |

## Answers to Dimension Questions

**1. How is a task decomposed?**
Ad-hoc by the LLM within the turn loop, not by a framework plan type. The model returns a `Response` containing 0..N tool calls (plus hosted-tool outputs, reasoning, handoffs). `process_model_response` classifies those calls into `ProcessedResponse` buckets (`src/agents/run_internal/run_steps.py:116-132`; actual classifier is `src/agents/run_internal/turn_resolution.py:1551-1999` per 01.01 report). `_build_plan_for_fresh_turn` copies those buckets into a `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:619-646`), which `_execute_tool_plan` then runs. Decomposition therefore equals "what the model emitted this turn" — there is no durable `Plan{steps}` object the agent can author via a `create_plan` tool. The only alternative decomposition is developer-wired multi-agent delegation: a `Handoff` (`src/agents/handoffs/__init__.py`) or `Agent.as_tool()` lets one agent delegate a bounded subtask to another, but the delegating agent must decide routing in prompt; the SDK does not decompose a user request into `Task` objects automatically.

**2. Are subtasks typed?**
No. `ToolExecutionPlan` is typed only by *tool kind*, not by logical subtask: `function_runs: list[ToolRunFunction]`, `computer_actions`, `shell_calls`, etc. (`src/agents/run_internal/tool_planning.py:561-566`). Each `ToolRunFunction{tool_call, function_tool}` carries `tool_name` implicitly via `function_tool.name`, but there is no `type: Research|Code|Review` taxonomy, no `priority`, no `effort`. The Codex `TodoItem` (`src/agents/extensions/experimental/codex/items.py:98-100`) has only `text: str` and `completed: bool` — free-form text, not a typed enum. `RunItem` subclasses are typed by item kind (`ToolCallItem`, `HandoffCallItem`, etc. at `src/agents/items.py:180-560`) for history replay, not for plan scheduling. Callers cannot filter "all research subtasks" without parsing `text` or `tool_call.name`.

**3. Can dependencies be represented?**
No. Neither `ToolExecutionPlan` nor `ProcessedResponse` has `depends_on`, `blocked_by`, or `after` edges. The plan fields are independent; `_execute_tool_plan` runs groups concurrently via `gather_with_cancel` (`src/agents/run_internal/tool_planning.py:984-1036`) or sequentially, but there is no cross-group ordering beyond "functions vs shells vs computer actions run in parallel by default." Within a single turn, tool calls cannot declare "run `shell_calls[1]` only after `function_runs[0]` succeeds." Deduplication logic (`_dedupe_processed_response_invocations` at `src/agents/run_internal/tool_planning.py:339-554`) guards against duplicate `call_id` reuse and validates invocation identity, but does not schedule a DAG. Multi-agent dependencies are not represented either: `Task.context: list[Task]` style DAG (as in CrewAI) does not exist; handoff ordering is whatever the turn loop decides next (`NextStepHandoff` replaces the agent and re-enters `while True` at `src/agents/run.py:768`).

**4. Are statuses tracked?**
No, at the plan level. `ToolExecutionPlan` has no `status` field; it is constructed, executed, and dropped (`src/agents/run_internal/tool_planning.py:558-573`). Individual tool results return `RunItem`s with implicit success/failure via `ToolCallOutputItem` or `ToolApprovalItem`, but there is no `TodoStatus=pending|running|completed|failed|blocked`. The Codex `TodoItem.completed: bool` (`src/agents/extensions/experimental/codex/items.py:100`) is binary and read-only (parsed from the subprocess, never mutated via SDK tool). Turn-level status exists: `SingleStepResult.next_step` is one of four variants (`src/agents/run_internal/run_steps.py:199`) and `RunState._current_step: NextStepInterruption|None` persists interruptions (`src/agents/run_state.py:254` per 01.01 report), but these track *approval resumption*, not subtask progress. There is no `get_ready_todos`, `running_count`, or `blocked_todos` API; the answer to "which subtask is blocking?" is not computable — the system can only say "the run is in `NextStepInterruption` awaiting approval for call_id X" or "the loop is awaiting model output."

**5. Can decomposition be evaluated?**
Not structurally; only indirectly via turn outcomes. Because there is no plan object, there is no `validate_plan`, `is_coverage_complete`, `is_minimal`, or `remaining_plan_still_valid` check. What exists: (a) `MaxTurnsExceeded` guard (`src/agents/run.py:1058-1144`) aborts after `max_turns` iterations, (b) `ToolsToFinalOutputResult.is_final_output` (`src/agents/agent.py:180-200`) lets a tool decide when to terminate, and (c) output guardrails (`OutputGuardrailResult` at `src/agents/run_internal/run_steps.py:219-220`) can reject a final output and force `NextStepRunAgain`. These are termination heuristics, not plan-coverage verifiers. The Codex side has no evaluation either: `TodoListItem` is displayed, not validated. A judge loop could be built by the user (an `Agent` whose `output_type` is a verdict schema), but the SDK provides no built-in `PlanEvaluator` or `todos_remaining` predicate.

## Architectural Decisions

| Decision | Why it was made (inferred) | Consequence for this dimension |
|----------|-----------------------------|--------------------------------|
| **Model-is-the-planner, not the framework** — no `Plan`/`Todo` type; tool calls *are* the plan | Keep the tool surface minimal and let any LLM decompose via free-form `function_call` generation; matches OpenAI Responses API shape (`ResponseFunctionToolCall`) | Enables flexible decomposition without teaching a DSL, but sacrifices dependency tracking, status queries, and blocking analysis |
| **Ephemeral `ToolExecutionPlan` per turn** — built from `ProcessedResponse`, executed, dropped (`src/agents/run_internal/tool_planning.py:558-646`) | Isolate per-turn work; support streaming and interruption resume with a small immutable struct | No durable plan across turns; cannot reorder, reprioritize, or evaluate a multi-turn plan; plan lifetime equals one `get_new_response` cycle |
| **Group-by-tool-kind rather than by logical subtask** (`function_runs`, `shell_calls`, `computer_actions`, etc. at `src/agents/run_internal/tool_planning.py:561-569`) | Route each kind to its dedicated executor (`execute_function_tool_calls`, `execute_shell_calls`, etc.) with proper hooks/guardrails per kind | Dependencies cannot be expressed across kinds; subtask identity is lost after grouping; routing is by tool implementation, not by plan semantics |
| **Handoffs and agents-as-tools as delegation, not subtask objects** (`NextStepHandoff{new_agent}` at `src/agents/run_internal/run_steps.py:155-157`, `Agent.as_tool` pattern at `docs/multi_agent.md:29`) | Reuse the existing turn loop for delegation — a handoff just swaps `agent` and continues `while True`; agents-as-tools reuse the function-tool path | No subtask registry: a handoff is a control-flow branch, not a trackable item with `status` or `depends_on`; blocking cannot propagate |
| **Codex todo as external read-only projection** (`TodoItem`/`TodoListItem` at `src/agents/extensions/experimental/codex/items.py:98-107`) | Surface Codex CLI's own todo state without owning it; keep SDK experimental and subprocess-isolated | Todo exists but is not writable via SDK, not linked to `ToolExecutionPlan`, and not queryable as a plan API; useful for Codex users only |
| **No plan persistence; `RunState` persists only interruption + history** (`RunState._current_step: NextStepInterruption\|None` at `src/agents/run_state.py:254`, `src/agents/run_state.py:184-321`) | `RunState` is for durable approval resume, not for plan replay; history is `RunItem`s, not `PlanStep`s | Cannot checkpoint a half-finished decomposition and resume it after crash; `ToolExecutionPlan` is not serializable and not stored |
| **Parallel-by-default tool execution via `gather_with_cancel`** (`src/agents/run_internal/tool_planning.py:984-1036`) | Minimize latency when model emits multiple independent calls in one turn | Correct but opaque: no `depends_on` to justify ordering; concurrent `needs_approval` checks add `pending_interruptions` instead of dependency edges |

## Notable Patterns

- **Plan-as-output, not plan-as-input**: the model emits tool calls, the SDK classifies and groups them — there is no `create_plan(steps:...)` tool the model must call. This is the inverse of CrewAI's `AgentExecutor.generate_plan` or agent-framework's `todos_add`.
- **Ephemeral plan + durable history**: `ToolExecutionPlan` is transient; `RunItem` history (`MessageOutputItem`, `ToolCallItem`, `ToolCallOutputItem`, `ReasoningItem` at `src/agents/items.py:180-560`) is the durable artifact, stored via `RunState` or `Session` (`src/agents/memory/session.py`). History can be replayed, but it is not a plan.
- **Approval as the only blocking primitive**: `ToolApprovalItem` (`src/agents/items.py:556-680`) and `NextStepInterruption{interruptions: list[ToolApprovalItem]}` (`src/agents/run_internal/run_steps.py:170-181`) are the sole modeled blocker; there is no `blocked_by_subtask` edge, only `awaiting human decision on call_id`.
- **Codex bridge as opaque stream**: `Thread.run_streamed` parses line-delimited JSON via `coerce_thread_item` (`src/agents/extensions/experimental/codex/thread.py:96-160`) and surfaces `TodoListItem` as one `ThreadItem` variant — the SDK observes decomposition but does not orchestrate it.
- **Group fan-out without DAG**: `_execute_tool_plan` fans out to 6 executors in one `gather_with_cancel` call (`src/agents/run_internal/tool_planning.py:984-1036`), then collects `ToolApprovalItem`s via `_collect_tool_interruptions` (`src/agents/run_internal/tool_planning.py:685-714`). The fan-out is static by kind, not by plan edge.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Ephemeral per-turn plan | Minimal schema; any LLM can author tool calls without learning a plan DSL; trivial to test (`tests/test_run_step_execution.py` asserts `next_step` variants) | No cross-turn plan identity; cannot reorder, dedupe, or evaluate multi-step decomposition; progress is only `current_turn` count |
| Group-by-tool-kind | Clean executor dispatch per kind with isolated hooks/guardrails; `sibling_category_failure` signal (`src/agents/run_internal/tool_planning.py:976`) coordinates parallel groups | No dependency-aware scheduling; a shell subtask that logically depends on a function result cannot express `depends_on`; must wait for next turn |
| LLM-authored decomposition | Domain-agnostic; no taxonomy to maintain; leverages model reasoning | Untyped `text`/`tool_call.arguments`; downstream routing must parse free-form tool names/args; no machine-readable `type` to route subtasks to specialist agents without title parsing |
| Handoffs/agents-as-tools for delegation | Simple control flow: handoff swaps `agent`, agent-as-tool reuses function path; documented in `docs/multi_agent.md:29` | No subtask tracking: completing a delegated call never updates a parent plan status; no `blocked` propagation across agents |
| Read-only Codex todo | Adds todos for Codex subprocess users without framework commitment | Fragmentation: todo state lives in Codex subprocess, `ToolExecutionPlan` lives in main loop — two decompositions not composable; Codex todo never advances a `ToolExecutionPlan` executor |
| No plan persistence | Small `RunState` (interruption + history only); atomic file/session stores not required for plan | Cannot checkpoint a 10-step decomposition and resume after failure; crash loses the in-flight `ToolExecutionPlan` without trace beyond `RunItem`s |

## Failure Modes / Edge Cases

- **No blocked detection → next-turn stall or redundant tool calls**: with no `depends_on`, two logical subtasks like "create file" then "test file" have no machine relation; if the model omits the second call in the same turn, it must emit it next turn. If it emits both but the first fails, the second still ran (parallel groups at `src/agents/run_internal/tool_planning.py:984-1036` do not abort siblings on single failure until `sibling_category_failure` is set, and only `isolate_parallel_failures` logic at `src/agents/run_internal/tool_execution.py` isolates function tools). No blocker explains why a subtask was skipped.
- **Lost reason / no completion audit for logical subtasks**: `ToolExecutionPlan` fields are lists of `ToolRun*` with no `reason` or `result_summary`; after `_execute_tool_plan` returns `RunItem`s, the justification (model reasoning `ReasoningItem`) is separate from the tool result. No plan-level `completed_reason` or `completed_at` persists beyond `RunItem` history.
- **Duplicate call_id reuse fails closed, not rescheduled**: `_dedupe_processed_response_invocations` (`src/agents/run_internal/tool_planning.py:339-554`) raises `ModelBehaviorError` if the model reuses a `call_id` for a different invocation instead of returning a new ID. This aborts the turn rather than treating it as a retry of a subtask — correct for safety, but a fragile decomposition that reuses IDs cannot be retried as a subtask.
- **Approval interruption loses plan granularity**: `pending_interruptions: list[ToolApprovalItem]` (`src/agents/run_internal/tool_planning.py:567`) serializes into `RunState._current_step: NextStepInterruption` (`src/agents/run_state.py:254`). After `state.approve()/reject()` and `Runner.run(state)`, `_build_plan_for_resume_turn` (`src/agents/run_internal/tool_planning.py:649-682`) rebuilds the plan from approved runs only. Rejected or pending subtasks never get `failed` status in a plan registry — they vanish from the next `ToolExecutionPlan`.
- **Codex todo diverges from main loop**: `TodoListItem{id, items: list[TodoItem]}` (`src/agents/extensions/experimental/codex/items.py:104-107`) is parsed from `type=="todo_list"` events (`src/agents/extensions/experimental/codex/items.py:226-232`) but never reconciled with `RunItem` history. A human amendment via Codex thread (`src/agents/extensions/experimental/codex/thread.py:96-160`) updates Codex's list but not any SDK plan, so the two sources of truth drift.
- **No plan validation**: workflow edge validation (`src/agents/run_internal/tool_planning.py:336-554` validates invocation identity and call_id uniqueness) checks tool invocation safety, but there is no `validate_plan` for decomposition — duplicate, contradictory, or missing logical subtasks are accepted as long as tool calls are well-formed.
- **Max-turns abort drops pending logical work without plan error**: `MaxTurnsExceeded` (`src/agents/run.py:1058-1144`) raises after `current_turn > max_turns`; in-flight `ToolExecutionPlan` work from the last turn is not persisted as `pending` subtasks for resume — it is lost except as history.

## Future Considerations

- Add an opt-in structured planning mode: a `Plan{items: list[PlanStep{description, tool_hint: str|None, depends_on: list[int], status: Pending|Running|Completed|Failed}}` with `TodoProvider`-style `create_plan/get_plan/update_step` tools, persisted in `RunState` alongside `RunItem` history, without breaking the current tool-call-is-plan default. Keep it behind a model-config flag to avoid DSL tax for simple agents.
- If adding subtask typing, keep it loose: `kind: str|None` + `tool_to_use: str|None` (like CrewAI) rather than a closed enum, so routing can be `kind=="research"` → search agent without constraining domain tasks. Wire to `Handoff` routing table.
- Introduce `depends_on` and `get_ready_subtasks` at the plan layer, reusing the existing `ToolExecutionPlan` grouping but adding a DAG scheduler before `_execute_tool_plan` — then `NextStepRunAgain` could signal "plan has ready subtasks" vs "awaiting model" distinctly, enabling blocking analysis.
- Persist subtask status (`pending|running|completed|failed|blocked`) in `RunState` with `completed_reason` and `completed_at`, and surface `blocked_subtasks` via a tool so `Runner` can answer "which subtask is blocking progress?" and dashboards can highlight the critical path.
- Bridge Codex todo ↔ main plan: a `TodoListItem` → `Plan` adapter that materializes `list[TodoItem]` into a `ToolExecutionPlan` or `Plan` fan-out (e.g., `PlanStep` per `TodoItem`) so the two decomposition surfaces become one artifact. Remove experimental gate once schema stabilizes.
- Add `validate_plan` and `evaluate_plan` (LLM judge or coverage heuristic) that checks for duplicate/contradictory subtasks and `is_request_satisfied`, enabling `replace_pending_steps` / `reorder` operations without requiring a full model replan.

## Questions / Gaps

- **No evidence found** for a `Plan`/`Todo`/`Subtask` schema with `status` or `depends_on` in the main runtime — searched `src/agents/run_internal/tool_planning.py:558-573`, `src/agents/run_internal/run_steps.py:116-199`, `src/agents/agent.py`, `src/agents/items.py:686-740`, and `src/agents/run_state.py:184-321`; all confirm ephemeral per-turn structures or binary `TodoItem.completed` in the Codex experiment only.
- **No evidence found** for a hierarchical or flat plan registry that survives across turns beyond `RunState` history — `RunState` holds `model_responses`, `generated_items`, and `_current_step: NextStepInterruption|None` (`src/agents/run_state.py:254`), not a `Plan` object.
- **No evidence found** for `reorder`/`reprioritize`/`blocked_by`/`evaluate` APIs — the five `__all__` exports in `tool_planning.py` (`src/agents/run_internal/tool_planning.py:67-86`) are exhaustive for planning, and public `src/agents/__init__.py` re-exports do not include any plan helpers.
- **No evidence found** for automatic mapping from logical subtasks to `Executor` or tool calls beyond the 1:1 `ProcessedResponse` → `ToolExecutionPlan` copy — tool/result tracking lives in `RunItem`/`ModelResponse` and `RunContextWrapper._tool_invocations`, not in a plan store.
- **Cannot fully answer** whether decomposition survives durable replay end-to-end: `SqliteSession`/`OpenAIConversationsSession` (`src/agents/memory/`) persist `RunItem` history, but `ToolExecutionPlan` is explicitly non-serializable and `Codex` `asyncio.Task` objects are not replayable — logical subtasks backed by a plan that was never stored are not durably resumable.

---

Generated by `dimensions/06.02-task-decomposition-representation.md` against `openai-agents-sdk`.
