# Source Analysis: openai-agents-sdk

## 03.04 Planner/Executor Separation

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+; `openai-agents` package built on `openai` SDK Responses/Chat Completions APIs |
| Analyzed | 2026-07-14 |

## Summary

The OpenAI Agents SDK has no separate LLM-driven planner and no separate executor role. Both live in the same message-passing loop (`src/agents/run_internal/run_loop.py:1019-1039`, `src/agents/run_internal/turn_resolution.py:629-846`), and the only explicit "plan" that exists is a per-turn tool-execution plan that the runtime constructs _after_ the model emits tool calls.

The "plan" surface is the `ToolExecutionPlan` dataclass defined at `src/agents/run_internal/tool_planning.py:178-190`. A fresh plan is built every turn via `_build_plan_for_fresh_turn` (`src/agents/run_internal/tool_planning.py:236-263`) immediately after `process_model_response` returns (`src/agents/run_internal/turn_resolution.py:652-657`). The same plan is rebuilt differently on a HITL resume via `_build_plan_for_resume_turn` (`src/agents/run_internal/tool_planning.py:266-299`) so that user approval/rejection decisions reshape what executes. The plan object is purely an internal execution work-list — function runs, computer actions, custom tool calls, shell calls, apply-patch calls, local shell calls, MCP approval requests, and pending interruptions (`src/agents/run_internal/tool_planning.py:181-189`). It is consumed by `_execute_tool_plan` (`src/agents/run_internal/tool_planning.py:542-683`), which dispatches all six execution paths in parallel via `asyncio.gather` (`src/agents/run_internal/tool_planning.py:572-624`) or sequentially when `parallel=False` (`src/agents/run_internal/tool_planning.py:625-672`).

The plan does not represent a multi-step user-level plan. There is no todo list, no scheduled task list, no plan-update tool surfaced to the model, no plan-revision prompt, and the codebase returns zero hits for `replan` (verified by `grep -rn "replan" src/agents`). The only `todo_list` references live inside the experimental Codex extension at `src/agents/extensions/experimental/codex/items.py:104-107` (`TodoListItem`) and are not wired into the core runner. Instead, the LLM emits whatever tool calls it wants each turn, and the runtime wraps those calls in a `ToolExecutionPlan` so it can run them in parallel and stitch the outputs back into the next prompt.

Durability exists but only at the level of the last processed response. `_serialize_processed_response` (`src/agents/run_state.py:776-823`) persists the parsed tool calls and interruptions inside `RunState._last_processed_response`, so a HITL pause/resume round trip re-reads the plan and rebuilds a filtered plan via `_select_function_tool_runs_for_resume` (`src/agents/run_internal/tool_planning.py:490-539`) and `_collect_runs_by_approval` (`src/agents/run_internal/tool_planning.py:376-447`). Schema versioning (`CURRENT_SCHEMA_VERSION = "1.11"`, `src/agents/run_state.py:131-149`) maintains forward-compatible read support.

The system cannot compare what it "planned" against what it actually did except at the trivial level of "which tool was called vs which tool returned". There is no plan-vs-actual reconciliation surface; tool call results flow straight back into the conversation and the next model call freely re-plans by emitting new tool calls.

## Rating

**4 / 10 — Present but inconsistent, weakly documented, and scoped only to tool execution (not planner/executor).** The `ToolExecutionPlan` dataclass, the `_build_plan_for_*` helpers, and the persistence via `_last_processed_response` give the SDK a concrete plan object and a clear plan/execute boundary for _tool calls inside a single turn_. However: (a) there is no separate planner role at the LLM level — the "planner" is the same model call that will be invoked on the next turn; (b) plan changes across turns are not surfaced as replanning events, they are simply the model emitting new tool calls; (c) no tests assert "the plan we built matched the plan we executed" — see Evidence §Tests; (d) the `TodoListItem`/`TodoItem` types in the experimental Codex extension are not integrated into the runner, so no first-class user-visible plan object exists.

## Evidence Collected

Every entry includes file path and line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan dataclass | `@dataclass class ToolExecutionPlan` with `function_runs`, `computer_actions`, `custom_tool_calls`, `shell_calls`, `apply_patch_calls`, `local_shell_calls`, `pending_interruptions`, `approved_mcp_responses`, `mcp_requests_with_callback` | `src/agents/run_internal/tool_planning.py:177-193` |
| Plan builder (fresh turn) | `_build_plan_for_fresh_turn` constructs the plan from a `ProcessedResponse` | `src/agents/run_internal/tool_planning.py:236-263` |
| Plan builder (resume turn) | `_build_plan_for_resume_turn` rebuilds the plan honoring pending approvals, output existence, and rejection callbacks | `src/agents/run_internal/tool_planning.py:266-299` |
| Plan executor | `_execute_tool_plan` runs all six tool paths in parallel via `asyncio.gather`, sequentially when `parallel=False` | `src/agents/run_internal/tool_planning.py:542-683` |
| Plan orchestration site (fresh) | `execute_tools_and_side_effects` builds plan then immediately runs it | `src/agents/run_internal/turn_resolution.py:652-679` |
| Plan orchestration site (resume) | `resolve_interrupted_turn` rebuilds plan via `_select_function_tool_runs_for_resume` (function calls) and `_collect_runs_by_approval` (shell, apply_patch, custom) | `src/agents/run_internal/turn_resolution.py:848-1391` |
| Turn / sub-step types | `NextStepHandoff`, `NextStepFinalOutput`, `NextStepRunAgain`, `NextStepInterruption` | `src/agents/run_internal/run_steps.py:155-174` |
| `ProcessedResponse.has_tools_or_approvals_to_run` | gate used by `execute_tools_and_side_effects` to decide whether side effects are needed | `src/agents/run_internal/run_steps.py:132-147`, `src/agents/run_internal/turn_resolution.py:769` |
| Plan persistence | `_serialize_processed_response` writes the parsed `ToolRunFunction`, `ToolRunShellCall`, etc. into `RunState._last_processed_response` | `src/agents/run_state.py:776-823` |
| Schema-versioned storage | `CURRENT_SCHEMA_VERSION = "1.11"`, change summary table | `src/agents/run_state.py:131-149` |
| Plan change (state) | Resume code path rebuilds the plan with approval filtering and tool-state dedupe | `src/agents/run_internal/run_loop.py:730-841`, `src/agents/run.py:840-947` |
| `replan` keyword audit | Zero hits for `replan`, `plan_replan`, `replan_step` across `src/agents` | (search boundary) |
| `todo` keyword audit | Only `TodoItem`/`TodoListItem` in `extensions/experimental/codex/items.py:98-107`; zero hits in core `src/agents` | (search boundary) |
| Planner prompts | `agents.prompts` only exposes `Prompt` TypedDict + `PromptUtil.to_model_input` — no planner prompt template | `src/agents/prompts.py:23-82` |
| `tool_use_behavior` gate (not a planner) | `check_for_final_output_from_tools` decides whether the LLM is called again; uses literal `"run_llm_again"`, `"stop_on_first_tool"`, `StopAtTools`, or callable | `src/agents/run_internal/turn_resolution.py:594-626` |
| Approval routing (plan-level interrupt) | `RunContextWrapper.get_approval_status` decides approve/reject per call, used inside `_collect_runs_by_approval` and `_select_function_tool_runs_for_resume` | `src/agents/run_context.py:180-442`, `src/agents/run_internal/tool_planning.py:396-415`, `src/agents/run_internal/tool_planning.py:510-538` |
| Loop driver (next_step dispatch) | `AgentRunner.run` switches on `turn_result.next_step` (`NextStepFinalOutput`, `NextStepInterruption`, `NextStepHandoff`, `NextStepRunAgain`) | `src/agents/run.py:903-1025`, `src/agents/run_internal/run_loop.py:1065-1153` |
| Loop driver (streaming) | `run_streamed` mirrors the same dispatch in `run_loop._streamed_run_loop` | `src/agents/run_internal/run_loop.py:1065-1153` |
| HITL docs claim of orchestration only | "Agent plus Runner lets the SDK manage turns, tools, guardrails, handoffs, and sessions for you" | `docs/agents.md:7` |
| HITL docs (no plan/replan language) | HITL pause/resume described via `interrupts`, `state.approve`, `state.reject`; no mention of replan | `docs/human_in_the_loop.md:1-180` |
| Tool-execution grouping | `_save_stream_items_with_count` / `_save_resumed_items` — per-step persistence, no plan metadata | `src/agents/run_internal/run_loop.py:629-635`, `src/agents/run_internal/session_persistence.py:215-244` |
| Tracer surfaces per step | `turn_span`, `agent_span`, `function_span`, `tool_use_tracker_snapshot` — no plan span | `src/agents/run_internal/run_loop.py:862-876`, `src/agents/run_internal/run_loop.py:1052-1059` |
| Plan-step tests | `tests/test_run_impl_resume_paths.py:42-107` — only one test asserts that `resolve_interrupted_turn` returns a `NextStepFinalOutput`. It patches `_execute_tool_plan` to no-op (`fake_execute_tool_plan`) and does NOT verify that the plan rebuilt from persisted state matches what was originally planned. | `tests/test_run_impl_resume_paths.py:42-107` |
| Plan-step tests — selection logic | `_select_function_tool_runs_for_resume` is unit-tested for approval gating, but not against a recorded plan/expected execution pair | `tests/test_hitl_error_scenarios.py:1229-1292` |
| Test that mentions plan dataclass | No test imports `ToolExecutionPlan` directly; verified by `grep -l ToolExecutionPlan tests/` | (search boundary) |
| Tool-tracker snapshot (closest analog) | `AgentToolUseTracker.record_processed_response` records tools used; exposed via `RunState._tool_use_tracker_snapshot` | `src/agents/run_internal/tool_use_tracker.py:119-200`, `src/agents/run_state.py:266-267` |
| Approval persistence | `RunState.approve` / `RunState.reject` mutate `_context._approvals` then prompt to "rerun with this state" | `src/agents/run_state.py:332-357` |

## Answers to Dimension Questions

1. **Is there an explicit plan?**  
   Yes, but only at the per-turn tool-execution level. The `ToolExecutionPlan` dataclass (`src/agents/run_internal/tool_planning.py:177-193`) is constructed from a `ProcessedResponse` and consumed by `_execute_tool_plan` (`src/agents/run_internal/tool_planning.py:542-683`). There is no user-facing planner object, no LLM-generated multi-step plan schema, no plan-prompt template in `src/agents/prompts.py`.

2. **Does the plan control execution?**  
   Yes, for _tool calls within the current turn_. `_execute_tool_plan` reads `plan.function_runs`, `plan.computer_actions`, `plan.custom_tool_calls`, `plan.shell_calls`, `plan.apply_patch_calls`, `plan.local_shell_calls`, `plan.mcp_requests_with_callback` and dispatches each through its dedicated executor. `_build_tool_result_items` then orders the results from these execution lanes (`src/agents/run_internal/tool_planning.py:334-355`). Plan-level approval state (auto-approve, always-approve, reject) gates whether individual runs are added to the plan via `_collect_runs_by_approval` (`src/agents/run_internal/tool_planning.py:376-447`) and `_select_function_tool_runs_for_resume` (`src/agents/run_internal/tool_planning.py:490-539`). The plan does **not** gate across turns — `NextStepRunAgain` (`src/agents/run_internal/run_steps.py:164-166`) causes the entire loop to re-invoke the model.

3. **Can the plan change?**  
   Yes, in three ways: (a) every fresh turn rebuilds the plan from the new model output via `_build_plan_for_fresh_turn` (`src/agents/run_internal/tool_planning.py:236-263`); (b) a HITL resume rebuilds the plan via `_build_plan_for_resume_turn` (`src/agents/run_internal/tool_planning.py:266-299`) with approval/output filters applied; (c) `RunState.approve` and `RunState.reject` mutate `RunContextWrapper._approvals` so the next `_build_plan_for_resume_turn` selects a different subset (`src/agents/run_state.py:332-357`, `src/agents/run_context.py:355-442`).

4. **Are plan changes traceable?**  
   The serialized `ProcessedResponse` that feeds the plan is persisted in `RunState._last_processed_response` (`src/agents/run_state.py:776-823`, schema-versioned at `src/agents/run_state.py:131-149`). The set of pending `ToolApprovalItem`s is serialized into `_current_step` (`src/agents/run_state.py:825-854`). However, there is no separate "before plan" / "after plan" record and no structured event emitted on plan revision — only `NextStepRunAgain` / `NextStepInterruption` markers in the run loop (`src/agents/run_internal/run_loop.py:814,837`).

5. **Is planning useful or decorative?**  
   Useful, narrowly. The plan struct is required for parallelism (six execution lanes dispatched together), tool-type isolation (`isolate_function_tool_failures`, `src/agents/run_internal/tool_planning.py:562-571`), HITL filtering, and durable resume. It is **not** a planner in the agent-design sense (plan-then-execute-without-LLM): the "plan" is always derived from the same model turn's tool calls.

> **Can the system compare what it planned to what it actually did?** Only in the trivial sense that tool call inputs vs. outputs are recorded as `RunItem`s. There is no plan-vs-actual reconciliation surface, no `PlanAudit` event, and no test asserts that the plan at execution time matched the plan at construction time (verified by inspecting `tests/test_run_impl_resume_paths.py:42-107`, which monkeypatches `_execute_tool_plan`).

## Architectural Decisions

- **Plan is derived, not authored.** Every plan is built from `ProcessedResponse` (`src/agents/run_internal/turn_resolution.py:2023-2031`), so the LLM's tool call output _is_ the plan. There is no "planner prompt" anywhere in `src/agents`.
- **Per-turn, not multi-turn.** The plan exists for at most one turn. The run loop's outer driver (`src/agents/run.py:1057-1132`) treats `NextStepRunAgain` as a re-invoke-the-model trigger, not a "replan" trigger.
- **Structured per tool family.** The plan fields exactly mirror the tool-type enums (`function_runs`, `computer_actions`, `custom_tool_calls`, `shell_calls`, `apply_patch_calls`, `local_shell_calls`) so each executor lane can be invoked in parallel (`src/agents/run_internal/tool_planning.py:572-624`).
- **HITL is embedded in the plan.** Approval filtering happens during plan construction (`_collect_runs_by_approval`, `_select_function_tool_runs_for_resume`) instead of at execution time, so rejected calls never reach the executors.
- **Persistence stores the parsed plan, not raw bytes.** `RunState._last_processed_response` holds a fully typed `ProcessedResponse` (`src/agents/run_state.py:257-258`), enabling resume to rebuild the plan without re-asking the model.

## Notable Patterns

- **Plan-then-execute-with-dispatch** — `_build_plan_for_fresh_turn` → `_execute_tool_plan` → `_build_tool_result_items` (`src/agents/run_internal/turn_resolution.py:652-689`).
- **Idempotent dedupe** — `_dedupe_tool_call_items` and `_make_unique_item_appender` ensure that re-appending the same `ToolCallItem` to `new_step_items` is a no-op (`src/agents/run_internal/tool_planning.py:158-174`, `src/agents/run_internal/tool_planning.py:358-373`).
- **Approved-by-sticky-decision** — `_get_approval_status_for_key` reads `_context._approvals` so a previously approved call auto-runs even after serialization/deserialization (`src/agents/run_context.py:180-200`).
- **Schema-gated persistence** — `CURRENT_SCHEMA_VERSION` and `SUPPORTED_SCHEMA_VERSIONS = frozenset(SCHEMA_VERSION_SUMMARIES)` enforce strict forward-compatibility (`src/agents/run_state.py:131-150`); only durable persisted formats are first-class.
- **Parallel-by-default, opt-out** — `_execute_tool_plan` defaults `parallel=True`, falling back to sequential only when `parallel=False` (`src/agents/run_internal/tool_planning.py:549,572-672`).
- **Three-lane next-step dispatch** — `NextStepHandoff` switches agent; `NextStepFinalOutput` ends the run; `NextStepInterruption` pauses for approvals; `NextStepRunAgain` re-invokes the model. The driver type-switch is mirrored in both streaming and non-streaming loops (`src/agents/run_internal/run_loop.py:791-839,1065-1153` and `src/agents/run.py:903-1025`).

## Tradeoffs

- **No multi-step plan = easier to reason about for tool dispatch**, but loses the ability to express "do steps 1-3, branch on result, then do step 4-7" without forcing the model to re-issue tool calls across turns.
- **Plan as derived artifact** means the LLM is the only place where "planning" happens. Users cannot pre-stage a plan before the LLM call, cannot inspect a plan between planning and execution, and cannot edit one mid-flight without re-prompting.
- **Per-turn planning only.** Since the plan is rebuilt every turn from `ProcessedResponse`, replanning across failure is implicit (the model's next response). This works for tool-correction, but provides no way to detect "the model diverged from any user-supplied plan" because there is no such thing.
- **HITL filtering embedded in plan** means rejection/execution coupling is tight. You cannot "see the plan and choose to execute tool A but skip tool B after the fact" — selection already happened in `_collect_runs_by_approval`.
- **No plan-trace event.** The tracer records turn spans, agent spans, function spans, tool spans, and tool-use tracker snapshots, but no `plan_span` is emitted. Plan lifecycle is invisible in traces.
- **Parallel execution is robust but loses ordering.** The plan's six lanes dispatch concurrently; their results are concatenated in `_build_tool_result_items` order. This trades causal sequencing for wall-clock latency.

## Failure Modes / Edge Cases

- **Plan never built in streaming path before guardrail tripwire** — `run_single_turn_streamed` calls `raise_if_input_guardrail_tripwire_known()` only after `get_new_response` returns, but before `execute_tools_and_side_effects` (`src/agents/run_internal/run_loop.py:1647-1705`). If an input guardrail trips mid-turn, the plan was never built and any pre-staged `ToolCallItem` will not be executed.
- **Resume rebuild can drop unsent tool calls** — `RunState` distinguishes "already persisted" from "needs to be re-sent" via `_current_turn_persisted_item_count` (`src/agents/run.py:1301-1351`). If the persisted count is wrong, the resume plan will skip or duplicate tool runs.
- **Approval state machine has three terminal states** (approved, rejected, always-approved, always-rejected) but the plan constructor (`_collect_runs_by_approval`) treats `approval_status is None` as "needs evaluation", which can re-invoke user-defined `needs_approval` callbacks that throw — see `_function_requires_approval`'s `except Exception: return True` (`src/agents/run_internal/turn_resolution.py:918-932`).
- **Schema versioning is fail-fast** — older SDKs reject newer or unsupported versions (`src/agents/run_state.py:126-130`). A schema-version mismatch on resume halts the run instead of allowing a best-effort migration.
- **Sequential guardrails only on first turn** — per `AGENTS.md`, `Input guardrails run only on the first turn and only for the starting agent` (see `src/agents/run.py:1172-1232`). If a plan-shaped guardrail policy relies on plan-vs-actual on turn 2+, it will never trigger.
- **Server-managed conversation + session persistence are mutually exclusive** — session is disabled when `server_conversation_tracker` is active (`src/agents/run_internal/run_loop.py:1010-1029`), which means HITL-with-session cannot use Responses-API conversation chaining, removing plan resumability across server resets in that combo.
- **Plan-level interrupt can orphan MCP callbacks** — `_collect_mcp_approval_plan` partitions MCP requests into callback-handled vs. manual (`src/agents/run_internal/tool_planning.py:196-233`); manual ones add to `pending_interruptions`. If a callback raises, it propagates through `await execute_mcp_approval_requests` (`src/agents/run_internal/tool_planning.py:99-137`).
- **No retry-on-fail clause inside the plan.** If `_execute_tool_plan` raises mid-batch (e.g., one tool panics), the executor swallows or surfaces per-tool errors through `isolate_parallel_failures` (`src/agents/run_internal/tool_planning.py:562-571`); the plan itself has no retry step.

## Future Considerations

- A user-exposed `Plan` object with explicit steps, statuses (pending/running/done/skipped), and a `_last_processed_plan` slot on `RunState` would make plan/execute divergence addressable. Currently the closest signal is the bookkeeping inside `AgentToolUseTracker` (`src/agents/run_internal/tool_use_tracker.py:119-200`).
- A `plan_span` in the tracer would close the observability gap; today only turn/agent/function spans exist (`src/agents/run_internal/run_loop.py:862-876`).
- A `replan` event or "plan revision" marker would make schema-versioned resume more predictable — adding a v3 field "plan_revision" to `RunState` would let consumers detect divergence.
- The `TodoListItem` in the experimental Codex extension (`src/agents/extensions/experimental/codex/items.py:104-107`) suggests interest in promoting todo lists to first-class, but it is currently isolated to the extension and not wired into `ToolExecutionPlan`.

## Questions / Gaps

- Is `tools_used` populated from the **plan** or from **execution**? `ProcessedResponse.tools_used: list[str]` (`src/agents/run_internal/run_steps.py:124`) is set during `process_model_response` and is the closest analog to "what was planned", but it does not record per-tool execution status or timings. A future plan-audit would need to enrich this list with `planned` vs `executed` markers.
- Does the model ever see a "plan summary" prompt? Searched `src/agents/prompts.py` and the rest of `src/agents/`: no evidence found. The model sees only its own previous outputs through the conversation history.
- Can a plan be inspected without being executed? No — the only entry points are `_build_plan_for_*` (private, underscore-prefixed, not re-exported in `__init__.py`) followed immediately by `_execute_tool_plan`. External callers cannot observe the plan data structure.
- Is there any store of "plan vs. actual" history? Searched for `plan_audit`, `plan_diff`, `PlanAudit`: no evidence found. The system can only replay from `RunState`, not compare planned vs. executed.
- Does HITL pause touch the plan? Sort of — `_pending_approvals_from_state` reads `_current_step.interruptions` (`src/agents/run_internal/turn_resolution.py:873-884`), so the plan is implicitly changed by the user's approval/rejection. No explicit "diff" between "plan at pause" and "plan at resume" is recorded.
- Are plan changes across turns (e.g., the model issuing different tool calls on retry) logged? Searches for `revised_plan`, `plan_revision`: no evidence found beyond the implicit behaviour in `NextStepRunAgain`.

---

Generated by `03.04-planner-executor-separation` against `openai-agents-sdk`.
