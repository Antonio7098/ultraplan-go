# Source Analysis: openai-agents-sdk

## 06.01 Planning Location and Responsibility

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python / OpenAI Agents SDK (Pydantic, Responses API, asyncio) |
| Analyzed | 2026-08-27 |

## Summary

OpenAI Agents SDK has **no dedicated planning subsystem**. Planning is **implicit, model-owned, and prompt-resident**: the LLM decides the next action (tool calls, handoffs) and the runtime (`AgentRunner`/`Runner`) merely executes the loop described in `src/agents/run.py:274-281` until final output, handoff, or tool side-effect requires another turn. The only runtime object named `*Plan*` is `ToolExecutionPlan` in `src/agents/run_internal/tool_planning.py:557`, which is **tool-dispatch scheduling within a single turn** (partitioning already-returned model tool calls by type and approval status), not task decomposition. User-visible "planning" exists only as **application-level orchestration patterns**: (a) LLM-driven autonomous planning via `Agent.instructions`/`handoffs`/`Agent.as_tool()` (`docs/multi_agent.md:12-41`; `src/agents/agent.py:583`), and (b) code-driven chaining of agents (`docs/multi_agent.md:43-50`; `examples/research_bot/manager.py:55`, `examples/financial_research_agent/manager.py:122`). Both are documented, not framework-enforced. Planning is optional, invisible as a reified artifact, and not reusable/cached by the SDK.

## Rating

**2 / 10 — Absent, implicit, ad-hoc**

Rationale: No planner prompts, planning agents, workflow-graph planner, task-decomposition code, or planning config exist in `src/agents/`. Grep for `planner|PlanningConfig|reasoning_handler` is empty outside `ToolExecutionPlan` (tool scheduling) and example-level user agents (`examples/research_bot/agents/planner_agent.py:25`). Planning is prose in `Agent.instructions` (`src/agents/agent.py:309`) and model choice of tools/handoffs, per `docs/multi_agent.md:10-15`. The internal `ToolExecutionPlan` is not planning in the dimension sense; its absence of durability, observability, or reuse confirms the dimension question "Is planning a real runtime object or just prose in a prompt?" answers **prose in a prompt** here. Downgraded to 2 (not 1) because the loop + `RunState` pause/resume for HITL approvals does give a durable interrupt boundary and the code-orchestration alternative is explicitly documented.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent prompt ownership | `Agent.instructions: str \| Callable[[RunContextWrapper, Agent], MaybeAwaitable[str]]` — system prompt for the agent, strongly recommended; `prompt: Prompt \| DynamicPromptFunction` for Responses API prompts | `src/agents/agent.py:309-329` |
| Dynamic instructions resolution | `get_system_prompt()` resolves static string or async callable `(context, agent) -> str` with arity check | `src/agents/agent.py:1042-1064` |
| Runner loop definition (planning location = LLM) | Docstring: loop 1. agent invoked 2. final output terminates 3. handoff re-loops 4. else run tool calls and re-loop | `src/agents/run.py:272-281` |
| Turn orchestration (no plan graph) | `AgentRunner._run_impl` while-True loop validating guardrails, calling `run_single_turn`/`resolve_interrupted_turn`, branching on `SingleStepResult.next_step` (`NextStepFinalOutput \| NextStepHandoff \| NextStepRunAgain \| NextStepInterruption`) | `src/agents/run.py:964-1150` |
| Tool dispatch "plan" (not task planning) | `@dataclass class ToolExecutionPlan { function_runs, computer_actions, custom_tool_calls, shell_calls, apply_patch_calls, local_shell_calls, pending_interruptions, approved_mcp_responses, mcp_requests_with_callback }` + `has_interruptions` | `src/agents/run_internal/tool_planning.py:557-574` |
| Plan builders for fresh/resume turns | `_build_plan_for_fresh_turn(...) -> ToolExecutionPlan` and `_build_plan_for_resume_turn(...) -> ToolExecutionPlan` via `_collect_mcp_approval_plan` | `src/agents/run_internal/tool_planning.py:619-682` |
| Plan execution is tool scheduling | `_execute_tool_plan(plan: ToolExecutionPlan, ...) -> (function_results, computer_results, ...)` fans out to `execute_function_tool_calls`, `execute_computer_actions`, etc., with `parallel` + `isolate_function_tool_failures` | `src/agents/run_internal/tool_planning.py:944-1101` |
| Turn-resolution use of plan | `plan = _build_plan_for_fresh_turn(...)` then `await _execute_tool_plan(plan=plan, ...)` + `pending_interruptions/approved_mcp_responses` handling; second path `plan = _build_plan_for_resume_turn(...)` | `src/agents/run_internal/turn_resolution.py:828-860`, `src/agents/run_internal/turn_resolution.py:2403-2472` |
| Run-loop re-export of tool-plan helpers | `run_loop.py:202` imports `execute_mcp_approval_requests` only — no workflow planner import | `src/agents/run_internal/run_loop.py:202` |
| Orchestration via LLM vs via code | Docs split: "Allowing LLM to make decisions: uses intelligence of LLM to plan..." vs "Orchestrating via code: determining flow via your code..." — table of `Agents as tools` vs `Handoffs` | `docs/multi_agent.md:10-50` |
| Handoffs as delegation (no planner) | `Agent.handoffs: list[Agent \| Handoff]` + `Handoff` description — LLM chooses handoff, runtime executes transfer message + optional input filter/nest | `src/agents/agent.py:331-333`, `src/agents/run_internal/turn_resolution.py:527-750` |
| Agent-as-tool as manager pattern | `Agent.as_tool(tool_name, tool_description, ...) -> FunctionTool` wrapping `Runner.run` with structured input schema; invoked as normal function tool | `src/agents/agent.py:583-1040` |
| Tool-use behavior controls final-output vs re-loop | `tool_use_behavior: "run_llm_again" \| "stop_on_first_tool" \| StopAtTools \| ToolsToFinalOutputFunction` | `src/agents/agent.py:373-393` |
| Example-level "planner" is user code, not framework | `planner_agent = Agent(name="PlannerAgent", instructions=PROMPT, output_type=WebSearchPlan)` producing `WebSearchPlan{ searches: list[WebSearchItem{reason, query}> }` — consumed by `manager.py` chaining `Runner.run(planner_agent) -> search` | `examples/research_bot/agents/planner_agent.py:6-31`, `examples/research_bot/manager.py:55` |
| Second example planner agent | `planner_agent = Agent(name="FinancialPlannerAgent", instructions=PROMPT, output_type=FinancialSearchPlan)` + `manager.py` chaining | `examples/financial_research_agent/agents/planner_agent.py:9-35`, `examples/financial_research_agent/manager.py:16-122` |
| Durable pause boundary is approvals, not plan | `RunState` persists interruptions, `pending_input`, `next_step`, schema `CURRENT_SCHEMA_VERSION="1.17"` with summaries for approvals/tool identity, not plan artifacts | `src/agents/run_state.py:182-217`, `src/agents/run_state.py:748-895` |
| Prompts: no planner prompt | `src/agents/prompts.py:8-81` defines only `Prompt{id, version, variables}` + `DynamicPromptFunction` forwarding to `PromptUtil.to_model_input`; no planning template | `src/agents/prompts.py:8-81` |
| Sandbox instructions: progress hint only | Sandbox prompt mentions "For longer tasks...provide progress updates" — narrative guidance, not planner | `src/agents/sandbox/instructions/prompt.md:102` |
| Planning config absent | Grep `planning` in `src/agents` returns only `tool_planning.py` (tool scheduling) + `blocked_output_cleanup_plan` + sandbox skill index comment — no `PlanningConfig`/`reasoning_effort`/`TodoList` | `src/agents/run_internal/tool_planning.py:75-77`, `src/agents/run_internal/blocked_output.py:635` |
| Workflow graph absent | No `Flow`/`flow.py` DAG, no `workflow` planner; only `Runner`, `AgentRunner`, `RunState`, `Session`, `SandboxRuntime` | `src/agents/run.py:1-60`, `src/agents/__init__.py:1-100` |
| Visibility surface is items/events, not plans | `RunResult.new_items`, `StreamEvent`, `RunState._current_step` are the only observables; no plan artifact emitted | `src/agents/result.py:1-80`, `src/agents/stream_events.py:1-40` |

## Answers to Dimension Questions

**1. Where does planning happen?**
Planning happens **inside the model (prompt-resident)**, not in runtime code, a planner agent, workflow graph, or external system. The application author provides `Agent.instructions` (`src/agents/agent.py:309`) and tools/handoffs (`src/agents/agent.py:331`, `src/agents/agent.py:583`), the model autonomously decides which tools/handoffs to invoke each turn, and `AgentRunner._run_impl` (`src/agents/run.py:964`) + `execute_tools_and_side_effects`/`resolve_interrupted_turn` (`src/agents/run_internal/turn_resolution.py:784`, `src/agents/run_internal/turn_resolution.py:1134`) merely execute whatever the model returned. The only runtime "planning" is `ToolExecutionPlan` (`src/agents/run_internal/tool_planning.py:557`) which **partitions the model's already-produced tool calls by kind/approval** before parallel execution — a scheduling concern, not decomposition. Docs explicitly frame the choice as "LLM decides planning" vs "you orchestrate via code" (`docs/multi_agent.md:1-9`, `docs/multi_agent.md:43-50`), with chaining (`examples/research_bot/manager.py:55`) being the code-owned alternative. No workflow graph planner exists.

**2. Who owns the plan?**
The **model owns the plan content, the runtime owns the loop, and application code owns any decomposition it chooses to encode**. Model: generates tool calls/handoffs via its output. Runtime (`AgentRunner`, `RunContextWrapper`, `RunState`) owns turn sequencing, approval gating, guardrail ordering, and persistence (`src/agents/run.py:964`, `src/agents/run_state.py:748`). Application: if it wants explicit decomposition it must build it itself, e.g., defining a `WebSearchPlan` Pydantic type and a `planner_agent` that returns it (`examples/research_bot/agents/planner_agent.py:20-31`) then chaining agents in Python (`examples/research_bot/manager.py:55`). There is no framework-owned plan registry; `ToolExecutionPlan` is ephemeral per turn and not persisted to `RunState`.

**3. Is planning required?**
No — **planning is fully optional and implicit**. A minimal `Agent(name="...", instructions="...")` + `Runner.run(agent, "query")` requires no planning artifact. The loop terminates when the model returns text matching `output_type` or when `tool_use_behavior` maps tool results to final output (`src/agents/agent.py:373-393`, `src/agents/run_internal/turn_resolution.py:753-781`). No `planning: bool` flag, `PlanningConfig`, or `max_steps/max_replans` exists to enable/disable. Optional planning emerges only if the user writes a planner agent or chains agents via code; otherwise execution is single-turn LLM + optional tool loop bounded by `max_turns`.

**4. Is planning visible?**
**Not as a reified plan.** There is no `plan`/`todos` field on `RunResult`/`RunState`/`AgentExecutorState`. Visibility is downstream effects only: `RunItem`s (`ToolCallItem`, `HandoffCallItem`, `MessageOutputItem`), `RunState._current_step` (`NextStepInterruption` with `ToolApprovalItem`s), `StreamEvent`s, and tracing spans (`src/agents/tracing/spans.py`, `src/agents/run_loop.py:88`). `ToolExecutionPlan` is internal to `turn_resolution.py:828` and not exposed, logged, or event-emitted; it is constructed, executed, and dropped. By contrast, CrewAI-style `TodoList`/`StepObservation` events are absent. The sandbox instructions prompt hint to "provide progress updates" (`src/agents/sandbox/instructions/prompt.md:102`) is prose guidance for the model's final answer, not a plan artifact.

**5. Is planning reusable?**
No plan-artifact reuse exists. Reusable units are **agents, tools, and handoffs**, not plans. `Agent.clone(instructions=...)` shallow-copies agent config (`src/agents/agent.py:548`) and chaining functions (`examples/research_bot/manager.py:55` `Runner.run(planner_agent, ...)`) can be reused across invocations, but generated plans are not cached, versioned, or persisted. `RunState` serializes `pending_input`, `model_responses`, `generated_items`, approvals (`src/agents/run_state.py:782-847`) and can resume an interrupted run via `Runner.run(..., input=run_state)` (`src/agents/run.py:590`), but does not store a plan to replay for a new query. No plan registry, plan cache keyed by task hash, or external planner interface is present.

## Architectural Decisions

| Decision | Evidence | Consequence |
|----------|----------|-------------|
| **Delegate all decomposition to the model** | `docs/multi_agent.md:12-15` "given open-ended task, LLM can autonomously plan" + `Agent.instructions` as system prompt (`src/agents/agent.py:309`) + loop in `src/agents/run.py:272` | Minimal framework surface; planning quality tracks model + prompt quality; no SDK-side decomposition to maintain; but no guardrails on plan soundness or step tracking |
| **Expose no planner; expose orchestration primitives (tools, handoffs, as_tool)** | `Agent.handoffs` (`src/agents/agent.py:331`), `Agent.as_tool` (`src/agents/agent.py:583`), `docs/multi_agent.md:23-31` table | Users compose via patterns rather than configuring a planner; flexible, but every planning pattern must be hand-built in application code |
| **Internal `ToolExecutionPlan` for per-turn tool scheduling** | Dataclass + builders/executor in `src/agents/run_internal/tool_planning.py:557-944` used by `src/agents/run_internal/turn_resolution.py:828` | Cleanly separates approval-gated dispatch from LLM semantics; enforces approval/retry/cancellation semantics; yet naming invites misreading as task planning |
| **Guardrails + approvals as the durable boundary, not plans** | `RunState` schema versions tracking approvals/tool identity/mount authority (`src/agents/run_state.py:186-217`), `_tool_invocation` helpers, HITL resume via `RunState.get_interruptions()` | Durability targets safety/consistency (HITL), not planning; resume does not preserve or revise a plan |
| **Code-orchestration documented as first-class alternative** | `docs/multi_agent.md:43-50` (structured outputs, chaining, while-loop evaluator, `asyncio.gather` parallelism) + example managers (`examples/research_bot/manager.py:55`) | Gives deterministic/speed/cost control for users who distrust LLM planning; but no SDK assistance for dependency-aware scheduling or replan-on-failure |

## Notable Patterns

- **Loop-not-graph execution**: Single `while True` in `AgentRunner._run_impl` (`src/agents/run.py:964`) with `NextStep*` discriminants, contrasting Flow/DAG planners in other harnesses.
- **ToolExecutionPlan as ephemeral scheduler**: Exists only between `_dedupe_processed_response_invocations` and `_execute_tool_plan` within one `SingleStepResult` production; never leaves `run_internal/`.
- **Planner-as-user-agent pattern**: Examples demonstrate the canonical way to get "planning" — define a typed `output_type` (`WebSearchPlan` containing `list[WebSearchItem]`), let a planner agent emit it, then fan out in Python (`examples/research_bot/agents/planner_agent.py:20-31`, `examples/financial_research_agent/agents/planner_agent.py:25-35`).
- **Instructions + handoff_description as planning affordance**: `handoffs` list plus `handoff_description` (`src/agents/agent.py:188`) lets the model self-route without a central planner.
- **Sandbox skill indexing sans materialization**: `skills.py:873` "indexed for planning, but not materialized" shows capability-discovery planning exists for sandbox agents, but is capability listing, not task decomposition.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| **Model-owned vs runtime-owned planning** | Zero SDK planning code to maintain; leverages frontier-model reasoning; prompt iteration suffices | Non-deterministic, unobservable plan; no dependency tracking, parallel ready-set, or effort-graded observation; debugging requires tracing + prompt forensics |
| **No plan artifact → minimal state** | `RunState` stays small (items + approvals + responses); schema evolution simpler (`src/agents/run_state.py:182`) | Cannot inspect, patch, cache, or visualize a plan; cannot diff plans across runs; no plan-level HITL (only tool-approval HITL) |
| **LLM orchestration vs code orchestration duality** (`docs/multi_agent.md:10-50`) | Users choose adaptivity vs determinism per task; manager pattern (`as_tool`) retains single-agent guardrail/tracing context | Two patterns with different failure modes; no unified scheduler; parallel execution only via manual `asyncio.gather` or within-turn tool parallelism |
| **ToolExecutionPlan internal but named like planning** | Precise approval/identity semantics (dedupe, canonical routing) isolated in `src/agents/run_internal/tool_planning.py` | Misleading for dimension analysis; invites confusion between tool scheduling and task planning; not user-configurable |
| **Chaining via typed output vs central planner** | Structured outputs (`output_type=WebSearchPlan`) make example planners strongly typed and testable | Each application reimplements planning loop; no shared refinement/replan policy or observation loop |

## Failure Modes / Edge Cases

- **Model hallucinates bad decomposition**: Since planning is model-owned, a planner agent can emit an irrelevant or over-long `WebSearchPlan` (5-20 or 5-15 queries per examples) with no framework validation beyond Pydantic schema; downstream `manager.py` blindly executes the plan, amplifying cost/latency.
- **Prompt brittleness**: Behavior depends entirely on `instructions` quality (`src/agents/agent.py:309`); under-specified prompts degrade planning silently — no plan-validation hook or fallback degrader exists.
- **Handoff contention without planner arbitration**: Multiple handoff tool calls in one response are resolved by dropping all but the first (`src/agents/run_internal/turn_resolution.py:563-577`), emitting tool outputs "Multiple handoffs detected, ignoring..." — a coordination failure with no planner to serialize.
- **Tool-use loop without progress**: With `tool_use_behavior="run_llm_again"` (default), the model can repeatedly call tools without reaching final output until `max_turns` triggers `MaxTurnsExceeded`; no plan-level progress check (`goal_already_achieved` heuristic in CrewAI) exists to short-circuit.
- **Approval deadlock, no plan fallback**: If `needs_approval` tools are pending and the caller never approves, `RunState` remains `NextStepInterruption` indefinitely; the plan cannot auto-revise to avoid blocked tools (contrast `PlannerObserver` refinement).
- **Duplicate tool call IDs collide**: `_dedupe_processed_response_invocations` (`src/agents/run_internal/tool_planning.py:339`) raises `ModelBehaviorError` on same-ID different-identity reuse — a planning-adjacent identity error surfacing as hard failure rather than replanned alternative.
- **Example chaining ignores per-step observation**: `examples/research_bot/manager.py:55` executes the planner's `WebSearchPlan` as a fixed list; there is no observation-driven `remaining_plan_still_valid` check or `needs_full_replan` signal, so partial search failures do not trigger plan adaptation.

## Future Considerations

- Add an **opt-in, type-visible planner abstraction** (e.g., `PlanningConfig` + `PlanStep`/`TodoList` types) if harness studies show demand, without breaking model-owned default; expose it as `RunResult.plan`/`RunState.plan` artifact for inspection and reuse.
- Consider a **plan cache** keyed by `(instructions hash, tools hash, input hash)` to avoid repaying planning tokens for repeated identical decompositions, if opt-in planning is added.
- Unify naming: rename `ToolExecutionPlan` to `ToolDispatchPlan`/`ToolBatch` to avoid conflating with task planning in docs and studies; update `src/agents/run_internal/tool_planning.py:557` and consumers.
- Provide **plan visualization via tracing/events** (e.g., `PlanCreated`/`PlanStepCompleted` events on `crewai_event_bus` analogue) so model-owned plans become observable without a dedicated planner.
- For HITL, explore **plan-level interruptions** (approve/revise a pending `WebSearchPlan`) beyond current tool-level `ToolApprovalItem` gating, enabling human steering of decomposition itself.
- If code-orchestration remains primary deterministic path, publish a **decomposition helper library** (dependency-aware `TodoList.get_ready_todos()`-style scheduler) as a utility, not a framework mandate.

## Questions / Gaps

- No evidence found for a planner prompt, `PlanningConfig`, `TodoList`, or workflow DAG planner in `src/agents/`; search of `src/agents/**`, `docs/**`, `src/agents/run_internal/**`, and `pyproject.toml:1-80` returned only `ToolExecutionPlan` and example-level `planner_agent` user code.
- Unclear whether future roadmap intends to add a first-class planner or to keep planning external in application code; `docs/multi_agent.md:43` framing suggests the latter is intentional but not stated as a design principle.
- No evidence of plan durability, reuse, or cross-agent plan sharing; `RunState` persists approvals and items but not a plan artifact, and `Agent.clone` does not carry a plan.
- Relationship between `ToolExecutionPlan` and hypothetical future task planning is unaddressed in codebase comments or docs.
- No dedicated load/stress test for large decompositions (e.g., 20-step plans with `depends_on` fan-out) exists because no decomposition mechanism exists to stress-test.

---

Generated by `06.01-planning-location-and-responsibility` against `openai-agents-sdk`.
