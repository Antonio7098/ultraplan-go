# Source Analysis: letta

## 06.01 Planning Location and Responsibility

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI, Pydantic, SQLAlchemy/Alembic, OpenTelemetry), LLM-adapter pattern |
| Analyzed | 2026-08-27 |

## Summary

Letta does **not** implement planning as a first-class runtime object. There is no `Planner`, `Plan`, `TaskDecomposer`, or workflow DAG in the agent loop. Planning is **implicit and model-owned**: system prompts instruct the LLM to use its inner monologue / reasoning channel to "plan actions or think privately" before each tool call, and the runtime (`LettaAgentV2`/`LettaAgentV3` iterative `step`/`_step` loops) reactively executes whatever single tool the model returns, validates it against `ToolRulesSolver`, and decides whether to continue (`should_continue`, `StopReasonType`). The closest explicit construct is `ToolRulesSolver` (`letta/helpers/tool_rule_solver.py:24`) — a runtime-owned constraint solver that encodes a workflow graph via `ToolRule` subtypes (Init, Child, Conditional, Terminal, etc.) and is compiled into a prompt block (`<tool_usage_rules>`). This provides *guardrails* for sequencing, not a generated multi-step plan. Multi-agent groups (`letta/groups/`) orchestrate turn-taking, not task decomposition. No plan is persisted, versioned, or reusable; visibility is limited to per-step reasoning text and Step/trace telemetry.

## Rating

**Score: 2 / 10 — Absent / Implicit / Ad-hoc**

Rationale: Planning exists only as prose in system prompts and ephemeral LLM reasoning. No runtime `Plan` object, no planner agent, no DAG, no plan validation or repair. `ToolRulesSolver` offers deterministic tool sequencing, but that is workflow configuration, not generative planning. Evidence search for `planner`, `decompose`, `workflow graph`, `plan` in runtime code returns only test fixtures and tool-rule logic; no dedicated planning stage.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner prompts — implicit planning via inner monologue | System prompts instruct: "You should use your inner monologue to plan actions or think privately" | `letta/prompts/system_prompts/memgpt_chat.py:24-26` |
| Planner prompts — workflow agent | "When you write a response, you express your inner monologue ... You should use your inner monologue to plan actions or think privately" + heartbeat-based chaining, no plan object | `letta/prompts/system_prompts/workflow.py:12-13` |
| Planner prompts — ReAct agent | Same inner-monologue planning instruction, no decomposition prompt | `letta/prompts/system_prompts/react.py:15-17` |
| Planning agents — absence | `AgentLoop` is a factory dispatching to `LettaAgentV2`/`V3`/`SleeptimeMultiAgent`; no Planner/Solver agent type | `letta/agents/agent_loop.py:15-63` |
| Workflow graphs — absence of DAG | Grep for `workflow|dag|graph` yields only Temporal webhook wrapping and test graphs, not agent planning graph | `letta/server/rest_api/routers/v1/folders.py:282`, `tests/integration_test_agent_tool_graph.py:33` |
| Task decomposition — absence | No `decompose`, `subtask`, `Planner` class; search for `plann` hits only test-agent dumps and a `knowledge-base.af` query-decompose prompt (user task, not framework planning) | `letta/helpers/tool_rule_solver.py:1-298` (no planner), `tests/test_agent_files/knowledge-base.af:82` |
| Planning config — tool rules as surrogate | `AgentState.tool_rules: Optional[List[ToolRule]]` defines declarative workflow; `ToolRule` discriminator with 8 subtypes | `letta/schemas/agent.py:76`, `letta/schemas/tool_rule.py:360-373` |
| Planning config — ToolRulesSolver categorizes rules | `init_tool_rules`, `child_based_tool_rules`, `terminal_tool_rules`, `required_before_exit`, etc.; `get_allowed_tool_names` intersects valid sets | `letta/helpers/tool_rule_solver.py:24-87`, `letta/helpers/tool_rule_solver.py:96-172` |
| Planning config — compiled into prompt block | `compile_tool_rule_prompts() -> Block(label="tool_usage_rules")` injected via `Memory.compile()` and `PromptGenerator.compile_system_message_async` | `letta/helpers/tool_rule_solver.py:209-237`, `letta/schemas/memory.py:718-724`, `letta/prompts/prompt_generator.py:198-210` |
| Task decomposition code — agent execution loop | `BaseAgentV2.step()` loops `for i in range(max_steps): response = self._step(...); if not self.should_continue: break` — reactive tool-by-tool, no look-ahead plan | `letta/agents/letta_agent_v2.py:232-274`, `letta/agents/letta_agent_v3.py:328-397` |
| Task decomposition code — single-step core | `_step()` = one LLM call + one tool execution + validation via `ToolRulesSolver.should_force_tool_call()`, `get_allowed_tool_names()` | `letta/agents/letta_agent_v3.py:894-970`, `letta/agents/letta_agent_v2.py:444-584` |
| Planning boundaries — max_steps guardrail | `BaseAgentV2.step(max_steps: int = DEFAULT_MAX_STEPS)` and `constants.py:74 # Max steps for agent loop`; `stop_reason = max_steps` terminator, not plan completion | `letta/agents/base_agent_v2.py:51-54`, `letta/constants.py:74`, `letta/agents/letta_agent_v3.py:394-395` |
| Planning boundaries — heartbeat/continue/terminal rules | `ContinueToolRule`, `TerminalToolRule`, `RequiredBeforeExitToolRule` control loop continuation via `is_terminal_tool`, `is_continue_tool`, `get_uncalled_required_tools` | `letta/schemas/tool_rule.py:275-313`, `letta/helpers/tool_rule_solver.py:174-208` |
| Agent types — workflow_agent is memory-autoclear, not planner | `AgentType.workflow_agent = "workflow_agent" # workflow with auto-clearing message buffer`; `message_buffer_autoclear` disables recall, not planning | `letta/schemas/agent.py:47-48`, `letta/schemas/agent.py:139-142` |
| Multi-agent — orchestration, not planning | `RoundRobinMultiAgent.step` cycles `speaker_id = agent_ids[i % len(agent_ids)]`; `DynamicMultiAgent.step` asks manager LLM to choose next speaker via string match — speaker-selection loop, no task plan | `letta/groups/round_robin_multi_agent.py:65-66`, `letta/groups/dynamic_multi_agent.py:74-103` |
| Visibility — Step as execution trace, not plan | `Step` schema captures provider/model/tokens/stop_reason/status; `StepProgression` enum tracks `START→FINISHED`; no `plan` field | `letta/schemas/step.py:16-66`, `letta/schemas/step.py:68-74` |
| Visibility — reasoning channel | `_step` extracts `reasoning_content` from LLM adapter and persists as message content; prompt generator scrubs inner thoughts per model | `letta/agents/letta_agent_v2.py:485-503`, `letta/agents/helpers.py:501-502` |

## Answers to Dimension Questions

**1. Where does planning happen?**

Nowhere as a dedicated stage. Planning is **inside the model call** — inner monologue / `ReasoningContent` before each tool call, as explicitly instructed by system prompts (`letta/prompts/system_prompts/memgpt_chat.py:24-26`, `workflow.py:12-13`, `react.py:15-17`). The runtime loop (`letta/agents/letta_agent_v3.py:328-397`, `letta/agents/letta_agent_v2.py:232-274`) does not pre-compute a plan; it calls `valid_tools = await self._get_valid_tools()` (`letta/agents/letta_agent_v2.py:497`), builds request data, invokes the LLM via adapter, then validates the returned tool against `ToolRulesSolver`. The only “planning-adjacent” code is the declarative `ToolRulesSolver` graph (`letta/helpers/tool_rule_solver.py:24`) which is constructed from `AgentState.tool_rules` (`letta/schemas/agent.py:76`) and compiled into `<tool_usage_rules>` (`letta/schemas/memory.py:718-724`). Grep for `planner|decompose|plan` finds no runtime planner class; hits are limited to test `.af` dumps and a knowledge-base query-decomposition prompt that is domain logic, not framework planning.

**2. Who owns the plan?**

The **model owns** the implicit plan. There is no runtime `Plan` object with an owner; `LettaAgentV2/V3` own execution state (`should_continue`, `stop_reason`, `tool_call_history` in `ToolRulesSolver.tool_call_history:51`) but delegate sequencing decisions to the LLM’s next-token choice, constrained by `ToolRulesSolver.get_allowed_tool_names()` (`letta/helpers/tool_rule_solver.py:96`). Runtime owns **workflow constraints**, not the plan content. No planner agent or external orchestrator (Temporal is referenced only for webhook/job wrapping at `letta/server/rest_api/routers/v1/folders.py:282`, not agent planning).

**3. Is planning required?**

No. Planning is **optional and absent by default**. `AgentState.tool_rules` is `Optional[List[ToolRule]] = Field(default=None)` (`letta/schemas/agent.py:76`); if `None`, `get_allowed_tool_names` returns `available_tools` unfiltered (`letta/helpers/tool_rule_solver.py:119`). `AgentType` enum lists `workflow_agent` etc. but none require a plan (`letta/schemas/agent.py:38-51`). The loop terminates on `max_steps`, `end_turn`, `no_tool_call`, `terminal_tool`, or `insufficient_credits`, not plan completion (`letta/schemas/letta_stop_reason.py` inferred via `letta/agents/letta_agent_v3.py:394-413`). An agent can run with zero tool rules and rely solely on model autonomy.

**4. Is planning visible?**

Partially and ephemerally. There is no persisted plan artifact. Visibility is limited to:
- Per-step `reasoning_content` / inner thoughts yielded as `ReasoningContent` messages (`letta/agents/letta_agent_v2.py:580-584`, `letta/agents/letta_agent_v3.py:1113`).
- Prompt-embedded `<tool_usage_rules>` block (`letta/helpers/tool_rule_solver.py:115-117`).
- Operational telemetry: `Step` (`letta/schemas/step.py:16`), `StepProgression` (`letta/schemas/step.py:68`), `StepMetrics` timing, and `/steps` REST endpoints (`letta/server/rest_api/routers/v1/steps.py:18-172`).
No plan diff, no plan approval UI, no plan-to-execution traceability beyond step history.

**5. Is planning reusable?**

No. No `Plan` is materialized, versioned, or stored; `AgentState` persists `tool_rules`, `blocks`, `tools`, `message_ids` but not a plan (`letta/schemas/agent.py:67-152`). The solver’s `last_prefilled_args_by_tool` cache (`letta/helpers/tool_rule_solver.py:54-61`) is per-step ephemeral (`exclude=True`, not persisted) and only caches argument prefills, not steps. Groups (`letta/groups/dynamic_multi_agent.py:74`, `round_robin_multi_agent.py:32`) re-run speaker selection each turn without caching a plan. Reuse would require re-creating an agent from a template or `.af` dump, which snapshots tool rules, not a generative plan.

## Architectural Decisions

| Decision | Description | Evidence |
|----------|-------------|----------|
| Reactive ReAct-style loop over explicit planning | Single LLM call → single tool execution → validation → repeat until `should_continue == False`. No separate planning pass; `LettaAgentV3._step` is documented as "one LLM call and tool execution" | `letta/agents/letta_agent_v3.py:894-913`, `letta/agents/letta_agent_v2.py:444-478`, `letta/agents/base_agent_v2.py:51-65` |
| Declarative tool sequencing via `ToolRule` DAG | Workflow expressed as typed rules (Init, Child, Conditional, Parent, Terminal, Continue, RequiredBeforeExit, MaxCount, RequiresApproval) solved intersecting valid sets | `letta/schemas/tool_rule.py:64-360`, `letta/helpers/tool_rule_solver.py:96-120` |
| Prompt-injected constraints, not code-enforced state machine | Solver compiles rules to `Block(label="tool_usage_rules")` (`COMPILED_PROMPT_DESCRIPTION`) inserted into system prompt; LLM is *asked* to obey, runtime also filters `allowed_tools` | `letta/helpers/tool_rule_solver.py:21-22`, `letta/helpers/tool_rule_solver.py:209-237`, `letta/schemas/memory.py:688-724` |
| Heartbeat chaining as control flow | `REQUEST_HEARTBEAT_PARAM`, `ToolRulesSolver.should_force_tool_call()` forces `tool_choice="required"` when constrained rule active; otherwise `auto` | `letta/constants.py:208`, `letta/agents/letta_agent_v3.py:956-962`, `letta/helpers/tool_rule_solver.py:273-298` |
| Stateless multi-agent orchestration | `DynamicMultiAgent`/`RoundRobinMultiAgent` implement turn-taking loops without a planner; Sleeptime variants run background `SleeptimeMultiAgentV3/V4` for memory but not planning | `letta/groups/dynamic_multi_agent.py:74-103`, `letta/groups/round_robin_multi_agent.py:65-66`, `letta/agents/agent_loop.py:35-39` |
| No plan persistence layer | `Step`/`Message`/`AgentState` ORM models store execution history, not future plan; `ToolRulesSolver` fields are `exclude=True` (not DB-persisted) | `letta/schemas/step.py:16-66`, `letta/schemas/agent.py:67-152`, `letta/helpers/tool_rule_solver.py:28-51` |

## Notable Patterns

- **Prompt-owned planning**: System prompts in `letta/prompts/system_prompts/*.py` uniformly delegate planning to the model’s private monologue rather than specifying a planning algorithm in code.
- **Constraint solver pattern**: `ToolRulesSolver` acts as a lightweight CSP solver: each `ToolRule.get_valid_tools(history, available)` returns a set, intersection yields final allowlist (`letta/helpers/tool_rule_solver.py:113-120`). Supports prefilled `args` via `ChildToolRule.child_arg_nodes` / `InitToolRule.args`.
- **Adapter funnel**: All public methods (`step`, `stream`, `build_request` in `letta/agents/base_agent_v2.py:36-105`, `letta_agent_v3.py:156-440`) funnel through `self._step(messages, llm_adapter)` to unify blocking/streaming/dry-run paths.
- **Violation hinting**: `_build_rule_violation_result` and `guess_rule_violation` re-inject rule prompts as tool-return hints when the model violates constraints (`letta/agents/helpers.py:501`, `letta/helpers/tool_rule_solver.py:239-271`).
- **Auto memory, not auto plan**: Sleeptime agents (`letta/agents/agent_loop.py:21-39`) provide background memory management, demonstrating the architecture can host secondary agents but does not use them for planning.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| Model-owned implicit plan | Maximum flexibility; no planner to maintain; works with any instruction-tuned LLM | Non-deterministic, no guarantees of decomposition quality; failures are silent until tool violation; hard to test or observe |
| ToolRules as workflow DSL | Human-readable, composable, enforceable without LLM cooperation; supports `conditional` branching on `last_function_response` | Expressiveness limited to tool-level sequencing; cannot encode hierarchical subtasks, deadlines, or resource constraints; authoring requires understanding 8 rule types |
| No plan artifact | Simpler persistence (only steps/messages) | No plan review/approval, no plan reuse, no plan-level metrics or rollback |
| Heartbeat + max_steps loop | Simple termination model; streams well (`SimpleLLMStreamAdapter`) | Long tasks risk `max_steps` truncation (`letta/agents/letta_agent_v3.py:394`) without plan progress tracking |
| Prompt injection of rules (vs hard filter only) | Leverages LLM’s instruction following; keeps system prompt cacheable (`_refresh_messages` skips rebuild unless forced) | Duplicate enforcement (prompt + allowlist) can diverge; violation yields soft hint, not hard state-machine transition |

## Failure Modes / Edge Cases

- **Planning hallucination / omission**: Since plan lives only in reasoning text, the model may skip steps or repeat them; runtime only catches *tool-name* violations, not semantic plan errors. Detected via `LogicError` → `LettaStopReason(no_tool_call/invalid_tool_call)` in `letta/agents/letta_agent_v2.py:582-585` and `_build_rule_violation_result`.
- **Constraint deadlock**: `get_allowed_tool_names(error_on_empty=True)` raises `ValueError("No valid tools found")` when intersecting `Child/Parent/MaxCount` rules yields empty set (`letta/helpers/tool_rule_solver.py:122-123`). No dedicated recovery beyond hint injection; step may end with `error` stop reason.
- **Conditional rule without response**: `ConditionalToolRule.get_valid_tools` raises `ValueError("requires an LLM response")` if `last_function_response` missing (`letta/schemas/tool_rule.py:204`); strict `require_output_mapping=True` can also return empty allowlist.
- **Context overflow mid-plan**: Plan is lost on summarization/compaction; `summarizer` may evict early messages while plan was only in reasoning history. Mitigated by `get_compaction_trigger_threshold` and `compact_messages` but not plan-aware (`letta/agents/letta_agent_v3.py:939`).
- **Multi-agent planning drift**: `DynamicMultiAgent.ask_manager_to_choose_participant_message` relies on substring match `if name.lower() in assistant_message.content.lower()` (`letta/groups/dynamic_multi_agent.py:99-102`); brittle speaker selection, assertion failure terminates run.
- **Approval stall**: `RequiresApprovalToolRule` pauses on `ApprovalRequestMessage`; unchecked approval leaves `pending_approval` dangling (`letta/schemas/agent.py:134-136`), and tool calls are withheld until human responds — no timeout or plan-branch fallback.
- **Credit exhaustion mid-plan**: `CreditVerificationService.verify_credits` check each step; failure sets `insufficient_credits` stop reason and breaks loop (`letta/agents/letta_agent_v3.py:334-339`), leaving partial execution without plan checkpoint.

## Future Considerations

- Materialize planning: Introduce an optional `Plan` schema (`id`, `goal`, `steps[]` with `tool`, `args`, `dependencies`, `status`) persisted alongside `Step`; add `POST /agents/{id}/plan` endpoints similar to `steps` router, enabling visibility, approval, and reuse.
- Separate planner agent: Leverage existing `AgentLoop` factory and manager/participant pattern (`letta/groups/dynamic_multi_agent.py:183-217`) to spawn a dedicated planner that outputs a plan block (e.g., `system/plan`) reviewed before execution; Sleeptime infrastructure already proves background agents are viable.
- Plan-aware tool rules: Extend `ToolRulesSolver` to validate against a committed plan hash, not just local history, and support `plan_step` tool that advances a plan cursor, giving deterministic progress tracking beyond `tool_call_history`.
- Observability: Emit OpenTelemetry span events for plan creation/update/failure and persist plan snapshots in `Step.error_data`/`metadata` pattern (`letta/schemas/step.py:63-65`) for post-mortem.
- Tests: Add contract tests asserting plan generation, plan violation detection, and plan resumption after compaction/cancellation — currently no `tests/test_*plann*` beyond `test_pydantic_task_planning_tool` which tests tool schema, not framework planning.

## Questions / Gaps

- No evidence found of a durable plan store, plan schema, or plan-repair logic despite exhaustive grep across `letta/agents`, `letta/schemas`, `letta/helpers`, `letta/groups`, and `letta/prompts`. If a hidden planner exists (e.g., in enterprise `server` or `otel` packages), it is not in the studied OSS source tree.
- Temporal workflow usage (`folders.py:282`, `WEBHOOK_SETUP.md:18`) appears limited to post-step webhooks and background jobs, not agent reasoning plans — confirmed by lack of workflow DAG code in `letta/groups`.
- The `.af` fixtures `deep_research_agent.af:1` (core_memory labels `research_plan`, `final_report`) suggest *user-level* research-plan patterns stored in memory blocks, but framework provides no semantics for them; they are plain `Block` values (`letta/schemas/memory.py:77-80`), not interpreted as plans by the runtime. Further inspection of template gallery would clarify whether cloud templates treat those blocks as plans.
- How does Letta handle user requests that explicitly ask for a plan (e.g., "create a task plan")? Evidence in `tests/test_sdk_client.py:1301` shows a user-provided `create_task_plan` tool with `StepsList` schema is the expected mechanism — planning is delegated to user-defined tools, not framework-owned.

---
Generated by `06.01-planning-location-and-responsibility` against `letta`.
