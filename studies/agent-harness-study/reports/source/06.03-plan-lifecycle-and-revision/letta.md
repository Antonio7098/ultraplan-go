# Source Analysis: letta

## 06.03 Plan Lifecycle and Revision

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI, Pydantic, SQLAlchemy/Alembic, Redis, OpenTelemetry) |
| Analyzed | 2026-08-27 |

## Summary

Letta has **no first-class `Plan` object and therefore no plan lifecycle**. The dimension maps to an empty feature surface: there is no `Plan`, `Planner`, `TaskPlan`, or plan-state machine in `letta/schemas/`, `letta/orm/`, `letta/agents/`, or `letta/services/`. Planning is delegated to the LLM's ephemeral inner monologue (`letta/prompts/system_prompts/memgpt_chat.py:25`, `letta/prompts/system_prompts/workflow.py:13`, `letta/prompts/system_prompts/react.py:17` — "You should use your inner monologue to plan actions or think privately") executed one step at a time by the reactive loop `LettaAgentV3.step()` / `_step()` (`letta/agents/letta_agent_v3.py:328-397`, `letta/agents/letta_agent_v3.py:894-970`). The closest runtime-owned construct is `ToolRulesSolver` (`letta/helpers/tool_rule_solver.py:24`) which encodes a declarative workflow via eight typed `ToolRule` subtypes (`letta/schemas/tool_rule.py:360-373`) and is compiled into a prompt block `Block(label="tool_usage_rules")` (`letta/helpers/tool_rule_solver.py:209-237`, `letta/schemas/memory.py:718-724`). This provides sequencing constraints, not a generative multi-step plan. There are no plan events, no `PlanStatus` enum, no replanning trigger, no revision history, no completion validator, and no drift detector. Lifecycle is actually **run/step lifecycle**: `Run.status` / `Step.status` with `LettaStopReason` / `StopReasonType` (`letta/schemas/letta_stop_reason.py:9-58`, `letta/schemas/run.py:17-51`, `letta/schemas/step.py:16-65`). Plan revisions would be lossy on compaction because the model-owned plan lives only in reasoning text that the summarizer may evict (`letta/prompts/summarizer_prompt.py:178` acknowledges "drift in task interpretation" but the mitigations are prompt instructions, not code). The `create_task_plan` tool (`letta/functions/schema_generator.py:247-257`) is a user-defined tool schema, not a framework-owned plan artifact. Consequently every question in this dimension is negative or "no evidence found."

## Rating

**2 / 10 — Absent / Implicit / Ad-hoc**

Rationale: Plan creation, update, invalidation, completion, revision persistence, and drift detection are all absent as runtime concepts. `ToolRulesSolver` offers explicit, tested sequencing (`tests/test_tool_rule_solver.py:55-576`, `tests/test_run_status_conversion.py:13-80` cited in sibling analyses) but that is workflow configuration, not plan lifecycle. No `Plan` schema, no `plan_updated` event, no `revision_id`, no `reason` field, no `plan_history` table, no validator. The system cannot explain why a plan changed because it does not model a plan. Score 2 (not 1) because the surrogate workflow DSL plus `BlockHistory` for memory and `Step`/`Run` for execution provide a coherent execution lifecycle that users co-opt for planning via memory blocks, but that reuse is user-owned, not framework-owned.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner prompts — implicit planning via inner monologue | System prompts instruct: "You should use your inner monologue to plan actions or think privately" | `letta/prompts/system_prompts/memgpt_chat.py:25` |
| Planner prompts — workflow agent | Same instruction, heartbeat chaining, no plan object | `letta/prompts/system_prompts/workflow.py:13` |
| Planner prompts — ReAct agent | Same inner-monologue instruction | `letta/prompts/system_prompts/react.py:17` |
| Planning absence — no Plan schema | Grep `class Plan` across `letta/schemas/` yields zero; only `ToolRule`, `AgentState`, `Memory`, `Block`, `Step`, `Run` exist | `letta/schemas/tool_rule.py:13`, `letta/schemas/agent.py:67-152`, `letta/schemas/block.py:13-67`, `letta/schemas/step.py:16-65` |
| Planning absence — no planner class | `AgentLoop` factory dispatches `LettaAgentV2`/`V3`/`SleeptimeMultiAgent`; no Planner, Decomposer, or DAG node | `letta/agents/agent_loop.py:15-63` |
| Task decomposition absence | No `decompose`, `subtask`, `plann` hits except test fixtures and `knowledge-base.af` query-decompose prompt (domain logic, not framework) | `letta/helpers/tool_rule_solver.py:1-298` (no planner), `letta/functions/schema_generator.py:247-257` |
| Surrogate — ToolRulesSolver as workflow graph | `ToolRulesSolver` categorizes `init_tool_rules`, `child_based_tool_rules`, `terminal_tool_rules`, `required_before_exit`, etc.; `get_allowed_tool_names` intersects valid sets | `letta/helpers/tool_rule_solver.py:24-87`, `letta/helpers/tool_rule_solver.py:96-172` |
| Surrogate — compiled into prompt block | `compile_tool_rule_prompts() -> Block(label="tool_usage_rules")` injected via `Memory.compile()` | `letta/helpers/tool_rule_solver.py:209-237`, `letta/schemas/memory.py:718-724` |
| Surrogate — ToolRule DSL | Eight subtypes via discriminator: `ChildToolRule`, `InitToolRule`, `TerminalToolRule`, `ConditionalToolRule`, `ContinueToolRule`, `RequiredBeforeExitToolRule`, `MaxCountPerStepToolRule`, `ParentToolRule`, `RequiresApprovalToolRule` | `letta/schemas/tool_rule.py:360-373` |
| Execution loop — no plan, single-step ReAct | `LettaAgentV3.step()` loops `for i in range(max_steps): response = await self._step(); if not should_continue: break` — no look-ahead plan | `letta/agents/letta_agent_v3.py:328-397` |
| Execution loop — single-step core | `_step()` = one LLM call + one tool execution + validation via `should_force_tool_call`, `get_allowed_tool_names` | `letta/agents/letta_agent_v3.py:894-970` |
| Plan creation absence | No `Plan` creation API; `AgentState.tool_rules: Optional[List[ToolRule]] = Field(default=None)` is the only workflow-creation surface, optional and static | `letta/schemas/agent.py:76` |
| Plan update rules absence | No `update_plan`, `revise_plan`, or `replan` API; `ToolRulesSolver.tool_call_history` is append-only in-memory and `exclude=True` (not persisted) | `letta/helpers/tool_rule_solver.py:51`, `letta/schemas/tool_rule.py:32-37` |
| Replanning triggers absence | No `replan_if`, `on_failure_replan`, or validator hook; `_decide_continuation` decides loop continuation from structural facts, not plan validity | `letta/agents/letta_agent_v3.py:1967-2036` |
| Plan completion checks — stop reasons, not plan validator | `StopReasonType` enum with 12 reasons (`end_turn`, `max_steps`, `tool_rule`, `requires_approval`, `error`, `cancelled`, etc.) mapped to `RunStatus` via `run_status` property | `letta/schemas/letta_stop_reason.py:9-49` |
| Plan completion — harness-owned, not model-owned | v3 strips `request_heartbeat` (`letta/agents/letta_agent_v3.py:1776`) and disables heartbeat injection (`request_heartbeat=False` at `letta/agents/letta_agent_v3.py:2071`); model cannot declare success except via `TerminalToolRule` exit tool | `letta/agents/letta_agent_v3.py:2010-2012` |
| Revision history absence — no PlanHistory table | `BlockHistory` exists for memory blocks (`letta/orm/block_history.py:12-48`, `letta/orm/block.py:107-112`) with `sequence_number`, `undo/redo` via `block_manager.py:956-1034`, but no `PlanHistory` or `PlanRevision` model | `letta/orm/block_history.py:12`, `letta/services/block_manager.py:851-916` |
| Revision history — Step/Run as execution history, not plan history | `Step` captured as `PENDING` then `SUCCESS`/`FAILED`/`CANCELLED` with `StepProgression` enum; `Run` aggregates `num_steps`, `tools_used` — history of what ran, not what was planned | `letta/schemas/step.py:68-74`, `letta/services/step_manager.py:419-476`, `letta/services/run_manager.py:412-439` |
| User-level task plan tool — not framework plan | `create_task_plan(steps: list[Step])` schema with `name="create_task_plan"` is a built-in function-set tool definition, not a persisted framework artifact | `letta/functions/schema_generator.py:247-257` |
| Plan drift — no code, only prompt instruction | Summarizer prompts mention preserving "plan files" verbatim and warning "where you left off… to ensure there's no drift in task interpretation" — prompt-level mitigation, no `DriftDetector` class | `letta/prompts/summarizer_prompt.py:11-12`, `letta/prompts/summarizer_prompt.py:178` |
| Status transitions — Run/Step, not Plan | `RunStatus` (`created`/`running`/`completed`/`failed`/`cancelled`) and `StepStatus` managed via `update_run_by_id_async` (logs illegal re-terminal transitions, does not reject) and `update_step_*_async` | `letta/services/run_manager.py:341-356`, `letta/services/step_manager.py:368-555`, `letta/schemas/enums.py:164-173` |
| Completion validators absence | No `PlanValidator` or `goal_achieved` check; `end_turn` simply means model produced content without tool call and required tools satisfied (`letta/agents/letta_agent_v3.py:1998-2002`), `max_steps` also maps to `completed` | `letta/schemas/letta_stop_reason.py:26-32` |
| Tests — no plan lifecycle tests | No `test_*plan*` for framework planning; `test_tool_rule_solver.py` tests sequencing, `test_run_status_conversion.py` tests stop-reason mapping | `tests/test_tool_rule_solver.py:55-64` cited in 06.05, `No evidence found` for plan tests |
| Multi-agent — orchestration, not plan revision | `RoundRobinMultiAgent.step` cycles `speaker_id = agent_ids[i % len(agent_ids)]`; `DynamicMultiAgent.step` asks manager LLM to pick next speaker — no plan merge | `letta/groups/round_robin_multi_agent.py:65-66`, `letta/groups/dynamic_multi_agent.py:74-103` |

## Answers to Dimension Questions

**1. Can plans change?**
No — there is no framework plan to change. The implicit plan is the LLM's inner monologue emitted as `ReasoningContent` per step (`letta/agents/letta_agent_v2.py:485-503`, `letta/agents/letta_agent_v3.py:1113`). It is regenerated fresh each `_step()` call and not stored as a durable object. The only mutable plan-like state is `ToolRulesSolver.tool_call_history` (`letta/helpers/tool_rule_solver.py:51`) which is per-step in-memory and append-only, and `AgentState.tool_rules` (`letta/schemas/agent.py:76`) which is developer-authored configuration set at agent creation and not mutated by the agent during execution. A user can co-opt a memory `Block` labeled `research_plan` in `BasicBlockMemory`/`ChatMemory` (`letta/schemas/memory.py:783-840`) but framework treats it as opaque `Block.value` (`letta/orm/block.py:20-60`), not a plan.

**2. Are changes justified?**
No — no justification field exists. There is no `PlanUpdate{reason, author, timestamp}` or `revision.reason` on any plan-like path. `ToolRulesSolver.get_allowed_tool_names` intersects `get_valid_tools` sets (`letta/helpers/tool_rule_solver.py:113-120`) without capturing why a transition was allowed; `BlockHistory` rows (`letta/orm/block_history.py:12-48`) store `value` snapshots with `sequence_number` but no `reason`; `Step.error_data` stores technical errors, not plan rationale (`letta/schemas/step.py:63-65`). When the model changes its implicit plan, the only trace is the new reasoning text and tool-call choice in the persisted `Message`/`Step` history.

**3. Is old plan history preserved?**
No as plan history. What is preserved is execution history: `Step` rows (with `StepProgression` watermark `START→FINISHED` at `letta/schemas/step.py:68-74`) created `PENDING` before each LLM call (`letta/agents/letta_agent_v2.py:941-966`) and finalized via `update_step_success_async` / `update_step_error_async` (`letta/services/step_manager.py:419-476`, `368-414`); `Run` rows (`letta/orm/run.py:22-77`); and `Message` rows. Memory `Block` edits are versioned via `BlockHistory` with undo/redo (`letta/services/block_manager.py:956-1034`, `letta/orm/block_history.py:12`). None of these is a typed plan revision log with diff or `parent_revision_id`. The prior implicit plan survives only as ancestor reasoning text in the message history, subject to compaction eviction (`letta/agents/letta_agent_v3.py:1438-1470`).

**4. Can the agent abandon a plan?**
Yes, trivially and implicitly, because there is no plan to uphold. The agent abandons its implicit trajectory whenever the harness stops the loop: `TerminalToolRule` exit (`is_terminal_tool` at `letta/helpers/tool_rule_solver.py:174-176` → `continue_stepping=False` with `StopReasonType.tool_rule` at `letta/agents/letta_agent_v3.py:2010-2012`), `max_steps` exhaustion (`StopReasonType.max_steps` at `letta/agents/letta_agent_v3.py:394-395,628-629`), `end_turn` (no tool call and required tools satisfied at `letta/agents/letta_agent_v3.py:1998-2002`), `requires_approval` pause (`letta/agents/letta_agent_v3.py:1682-1709` with `pending_approval` at `letta/schemas/agent.py:134`), `insufficient_credits` (`letta/agents/letta_agent_v3.py:333-339`), or `cancelled`/`error`/`llm_api_error` paths. No `abandon_plan` event or `PlanStatus.abandoned` exists; abandonment is indistinguishable from normal termination except via `stop_reason` value, and most stop reasons map to `RunStatus.completed` (`letta/schemas/letta_stop_reason.py:26-32`).

**5. Can plan drift be detected?**
No. There is no drift metric, threshold, or validator. Search for `drift`, `validator`, `replan`, `revision` across `letta/agents/`, `letta/helpers/`, `letta/schemas/`, `letta/services/` yields only `BlockHistory` versioning and Alembic `revision_id` (`letta/services/agent_serialization_manager.py:488`, `letta/utils.py:1335-1350`), neither plan-drift. The closest mechanisms are: (a) `ToolRulesSolver` constraint checks that detect tool-name violations and re-inject rule prompts as hints (`guess_rule_violation` at `letta/helpers/tool_rule_solver.py:239-271`, `helpers.py:501`), which catches sequencing drift, not semantic plan drift; (b) summarizer contracts to preserve "important plan files" verbatim and to quote "where you left off… to ensure there's no drift in task interpretation" (`letta/prompts/summarizer_prompt.py:11-12,178`), which is prompt instruction, not code enforcement; (c) `get_compaction_trigger_threshold` / `compact` (`letta/agents/letta_agent_v3.py:938-940,1241-1246`) which evicts history that may contain the plan, creating drift risk rather than detecting it. Framework provides no `remaining_plan_still_valid`, `plan_stale`, or periodic `should_replan` predicate.

## Architectural Decisions

| Decision | Description | Evidence |
|----------|-------------|----------|
| Reactive ReAct over explicit planning | Single LLM call → single tool execution → validation → repeat until `should_continue == False`. No separate planning pass. | `letta/agents/letta_agent_v3.py:894-913`, `letta/agents/letta_agent_v2.py:444-478`, `letta/agents/base_agent_v2.py:51-65` |
| Declarative tool sequencing via `ToolRule` DSL | Workflow expressed as typed rules (Init, Child, Conditional, Parent, Terminal, Continue, RequiredBeforeExit, MaxCount, RequiresApproval) solved by intersecting valid sets. | `letta/schemas/tool_rule.py:64-360`, `letta/helpers/tool_rule_solver.py:96-120` |
| Prompt-injected constraints, not hard state machine | Solver compiles rules to `Block(label="tool_usage_rules", value="\n".join(prompts))` inserted into system prompt; runtime also filters `allowed_tools`. | `letta/helpers/tool_rule_solver.py:21-22,209-237`, `letta/schemas/memory.py:718-724` |
| No plan persistence layer | `Step`/`Message`/`Run`/`BlockHistory` store execution/memory history, not future plan; `ToolRulesSolver` fields are `exclude=True` (not DB-persisted). | `letta/schemas/step.py:16-66`, `letta/schemas/agent.py:67-152`, `letta/helpers/tool_rule_solver.py:28-61` |
| Heartbeat removal in v3 — harness-owned continuation | v3 strips model-supplied `request_heartbeat` and disables its injection; continuation decided by `ToolRulesSolver` + `_decide_continuation` + `StopReasonType`. | `letta/agents/letta_agent_v3.py:1776,2068-2073`, `letta/constants.py:208` |
| Memory blocks as user-level plan substrate | `BasicBlockMemory`/`ChatMemory` allow arbitrary blocks (e.g., `research_plan`) but framework interprets them as opaque `Block.value` with generic `BlockHistory` versioning. | `letta/schemas/memory.py:783-840`, `letta/orm/block.py:20-115` |

## Notable Patterns

- **Prompt-owned planning**: All system prompts in `letta/prompts/system_prompts/*.py` delegate planning to private monologue rather than specifying an algorithm (`letta/prompts/system_prompts/memgpt_chat.py:25`, `workflow.py:13`, `react.py:17`).
- **Constraint solver pattern**: `ToolRulesSolver.get_allowed_tool_names` intersects per-rule `get_valid_tools` sets (`letta/helpers/tool_rule_solver.py:113-120`) and caches prefilled `args` via `ChildToolRule.child_arg_nodes` / `InitToolRule.args` (`letta/helpers/tool_rule_solver.py:144-167`).
- **Adapter funnel**: `step`, `stream`, `build_request` in `letta/agents/base_agent_v2.py:36-105` and `letta_agent_v3.py:156-440` funnel through `self._step(messages, llm_adapter)` to unify paths.
- **Violation hinting**: `_build_rule_violation_result` and `guess_rule_violation` re-inject rule prompts as tool-return hints when sequencing violated (`letta/agents/helpers.py:501`, `letta/helpers/tool_rule_solver.py:239-271`).
- **Execution-history-as-plan-history fallback**: Users can reconstruct implicit plan evolution from `Step`/`Message` history and `BlockHistory` diffs (`letta/services/block_manager.py:762-1034`), but framework provides no plan-diff API.
- **Sentinel-delimited termination**: Every stream terminates with `stop_reason` → usage → `[DONE]` (`letta/agents/letta_agent_v2.py:1476-1487` in sibling analyses), with `StopReasonType.run_status` canonical mapping (`letta/schemas/letta_stop_reason.py:24-49`) — the only lifecycle observable for "plan completion."

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Model-owned implicit plan | Maximum flexibility; works with any instruction-tuned LLM; no planner to maintain | Non-deterministic, untestable, not versioned; failures silent until tool violation; cannot explain why plan changed |
| ToolRules as workflow DSL | Human-readable, composable, enforceable without LLM cooperation; supports conditional branching on `last_function_response` | Expressiveness limited to tool sequencing; cannot encode hierarchical subtasks, deadlines, or resource constraints; 8 rule types to learn |
| No plan artifact | Simpler persistence (only steps/messages/blocks); no plan migration debt | No plan review/approval, no reuse, no plan-level metrics, rollback, or time-travel |
| Heartbeat removal + max_steps guard | Deterministic termination; harness controls progress, not model hallucination | Long tasks risk `max_steps` truncation reported as `completed` (`letta/schemas/letta_stop_reason.py:26-32`) without plan progress tracking |
| Prompt injection of rules (vs hard filter only) | Leverages instruction following; keeps system prompt cacheable (`_refresh_messages` skips rebuild unless forced) | Duplicate enforcement (prompt + allowlist) can diverge; violation yields soft hint, not hard state-machine transition |
| BlockHistory for memory, not plan | Generic versioning for any block; undo/redo supported (`letta/services/block_manager.py:956-1034`) | No plan-specific metadata (goal, status, parent plan); plan stored in block is indistinguishable from notes |

## Failure Modes / Edge Cases

- **Planning hallucination / omission**: Plan lives only in reasoning text; model may skip or repeat steps. Runtime only catches tool-name violations via `get_allowed_tool_names(error_on_empty=True)` → `ValueError("No valid tools found")` (`letta/helpers/tool_rule_solver.py:122-123`) and `_build_rule_violation_result`; semantic errors undetected until downstream tool fails, surfaced as `invalid_tool_call` → `RunStatus.failed`.
- **Constraint deadlock**: Intersecting `Child`/`Parent`/`MaxCount` rules can yield empty allowlist; no dedicated recovery beyond hint injection, step may end with `error` stop reason (`letta/agents/letta_agent_v3.py:1015-1016`).
- **Conditional rule without response**: `ConditionalToolRule.get_valid_tools` raises `ValueError("requires an LLM response")` if `last_function_response` missing (`letta/schemas/tool_rule.py:204`); strict `require_output_mapping=True` can return empty set, deadlocking the next step.
- **Context overflow mid-plan**: Implicit plan lost on summarization/compaction; summarizer may evict early reasoning while plan was only in history. Mitigated by compaction threshold (`letta/agents/letta_agent_v3.py:938-939`) but not plan-aware; prompt asks to preserve "important plan files" verbatim (`letta/prompts/summarizer_prompt.py:11`) but no guarantee.
- **Premature exit via `TerminalToolRule`**: Any model invocation of a terminal tool ends run successfully regardless of task state (`letta/agents/letta_agent_v3.py:2010-2012`), mapped to `RunStatus.completed` — indistinguishable from genuine completion without inspecting `stop_reason`.
- **Approval stall**: `RequiresApprovalToolRule` pauses on `ApprovalRequestMessage` (`letta/agents/letta_agent_v3.py:1682-1709`); unchecked `pending_approval` (`letta/schemas/agent.py:134`) leaves plan dangling with no timeout or fallback branch.
- **Truncation masked as success**: 50-step run cut off by `DEFAULT_MAX_STEPS = 50` (`letta/constants.py:75`) reports `completed` with `stop_reason=max_steps` (`letta/schemas/letta_stop_reason.py:26-32`); clients cannot distinguish achievement from budget exhaustion.
- **Credit exhaustion mid-plan**: `CreditVerificationService.verify_credits` check each step; failure sets `insufficient_credits` stop reason and breaks loop (`letta/agents/letta_agent_v3.py:333-339`) leaving partial execution without plan checkpoint.
- **In-memory `tool_call_history` loss on crash**: `ToolRulesSolver.tool_call_history` and `last_prefilled_args_by_tool` are `exclude=True` and process-local; crash mid-run loses sequencing state, next instance rebuilds from DB `Run`/`Step` history which does not include the solver's history.
- **Drift after compaction undetectable**: No validator re-reads `Block` plan vs. recent `Message` history to score drift; small iterative description tweaks via tool calls are indistinguishable from faithful execution.

## Future Considerations

- Materialize planning: Introduce optional `Plan` schema (`id`, `goal`, `steps[]` with `tool`, `args`, `dependencies`, `status`, `revision`, `parent_revision`, `reason`, `author`) persisted alongside `Step`; add `POST /agents/{id}/plan` and `PATCH /agents/{id}/plan` endpoints parallel to `/steps` router (`letta/server/rest_api/routers/v1/steps.py:18-172`), enabling visibility, approval, and reuse.
- Separate planner agent: Leverage existing `AgentLoop` factory and manager/participant pattern (`letta/groups/dynamic_multi_agent.py:183-217`) to spawn a dedicated planner that outputs a `system/plan` block reviewed before execution; Sleeptime infrastructure proves background agents are viable (`letta/agents/agent_loop.py:21-39`).
- Plan-aware tool rules: Extend `ToolRulesSolver` to validate against a committed `plan_hash`, not just local history, and support `plan_step` tool that advances a plan cursor, giving deterministic progress tracking beyond `tool_call_history`.
- Justified revisions: Require `reason: str` on plan updates and persist it to `PlanHistory.reason` and to `Step.error_data`/`metadata` pattern (`letta/schemas/step.py:63-65`); emit OTel span events for `plan.created` / `plan.updated` / `plan.abandoned`.
- Drift detection: Add pluggable `PlanValidator(plan, recent_steps) -> DriftReport{score, stale_steps, missing_deps}` invoked before each `tick()` or via `pre_model_hook`; threshold auto-triggers `ToolRulesSolver` violation hint or `_decide_continuation` remediation. Guard compaction by re-anchoring plan block before `compact()` (`letta/agents/letta_agent_v3.py:1241-1246`).
- Completion validators: Decouple `RunStatus.completed` from `max_steps`/`tool_rule` (introduce `truncated`/`abandoned` terminal states) and add post-hoc verifier hook (evaluator tool or webhook-side contract) to check goal attainment independently of loop exit.
- Tests: Add contract tests for plan creation, violation detection, replan after `invalid_tool_call`, and resumption after compaction/cancellation — currently no `tests/test_*plan*` beyond `create_task_plan` tool schema.
- Plan observability: Add `StreamMode="plan"` via `StreamTransformer` emitting `PlanDiffPayload` and persist plan snapshots in `Message`/`Step` metadata for post-mortem; surface `GET /agents/{id}/plan/history` with diffs analogous to `BlockHistory` ancestry walk (`letta/services/block_manager.py:762-916`).

## Questions / Gaps

- No evidence found of a durable plan store, plan schema, or plan-repair logic despite exhaustive grep across `letta/agents`, `letta/schemas`, `letta/helpers`, `letta/groups`, `letta/prompts`, and `letta/orm`. If a hidden planner exists in enterprise `server` or `letta-code` repo, it is not in the studied OSS source tree (deprecated per `AGENTS.md`).
- How does Letta handle user requests that explicitly ask for a plan (e.g., "create a task plan")? Evidence at `letta/functions/schema_generator.py:247-257` shows a user-provided `create_task_plan` tool with `StepsList` schema is the expected mechanism — planning delegated to user-defined tools, not framework-owned. Whether cloud templates treat `Block(label="research_plan")` as a plan is not enforced by runtime.
- Does compaction preserve the implicit plan? Summarizer prompts instruct to carry forward "important plan files" (`letta/prompts/summarizer_prompt.py:11-12,178`) but no test validates verbatim preservation under token pressure; search boundary: `grep plan letta/services/summarizer/` yields only prompt text.
- Are `ToolRule` updates mid-run possible? `AgentState.tool_rules` is persisted on the `Agent` row (`letta/orm/agent.py:45-204`) and loaded via `to_pydantic` (`letta/orm/agent.py:214-315`), but no `PATCH /agents/{id}/tool-rules` mid-run API was found; updates would require agent recreation.
- `leta/orm/provider_trace.py:16` `plan_type` refers to billing subscription, not task plan — false positive filtered.
- The `research_plan` / `final_report` core memory labels seen in `.af` fixtures (`tests/test_agent_files/knowledge-base.af:82` in 06.01 analysis) are plain `Block` values, not interpreted by runtime; confirmation would require inspecting template gallery which is not in this source tree.

---

Generated by `06.03-plan-lifecycle-and-revision` against `letta`.
