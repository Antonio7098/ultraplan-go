# Source Analysis: agent-framework

## 06.01 Planning Location and Responsibility

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (.NET, Go) - Python `agent-framework-core` + `agent-framework-orchestrations`, .NET `Microsoft.Agents.AI` |
| Analyzed | 2026-08-27 |

## Summary

Planning in Microsoft Agent Framework is **dual-located and explicitly optional**, not a monolithic planner. There are two distinct runtime-owned planning subsystems plus optional graph-level orchestration:

1. **Harness-level implicit planning via `TodoProvider` + `AgentLoopMiddleware`** (`python/packages/core/agent_framework/_harness/_todo.py:446`, `python/packages/core/agent_framework/_harness/_loop.py:217`). Planning is prompt-driven (`DEFAULT_TODO_INSTRUCTIONS` at `python/packages/core/agent_framework/_harness/_todo.py:25`) but reified as a durable runtime object (`TodoItem` `python/packages/core/agent_framework/_harness/_todo.py:51`, `TodoStore` `python/packages/core/agent_framework/_harness/_todo.py:228`) exposed as tools (`todos_add`, `todos_complete`, etc. at `python/packages/core/agent_framework/_harness/_todo.py:505-589`). An `AgentLoopMiddleware` loops until `todos_remaining()` (`python/packages/core/agent_framework/_harness/_loop.py:925`) is false. This is model-owned creation but runtime-owned persistence/evaluation.

2. **Orchestration-level explicit planning via `MagenticOrchestrator`/`StandardMagenticManager`** (Python `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:864` / .NET `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:82`). Here a dedicated **manager/planner agent** generates a first-class `TaskLedger`/`_MagenticTaskLedger` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:272`, .NET `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticTaskContext.cs:1` - `TaskLedger` record) and `MagenticProgressLedger` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:308`, .NET `MagenticProgressLedger`), with replanning on stall (`MagenticManager.UpdatePlanAsync` at `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:36`).

3. **Workflow graph as static plan**: `WorkflowBuilder` (`python/packages/core/agent_framework/_workflows/_workflow_builder.py:53`) encodes the plan as a DAG via `add_edge`/`add_chain`/`add_fan_out_edges`/`add_switch_case_edge_group`. No dynamic planner; the graph *is* the plan. Agentic UI sample shows ad-hoc planning via explicit tools `create_plan`/`update_plan_step` (`dotnet/samples/05-end-to-end/AGUIClientServer/AGUIDojoServer/AgenticUI/AgenticPlanningTools.cs:9`).

Planning is **never required** (`disable_todo` at `python/packages/core/agent_framework/_harness/_agent.py:318`, `loop_should_continue=None` at `python/packages/core/agent_framework/_harness/_agent.py:337`, `require_plan_signoff=False` default at `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:888`). Visibility is high (session state, workflow events, checkpointing) but ownership is split.

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale: Planning is a real runtime object (typed `TodoItem`/`TaskLedger`/`MagenticProgressLedger` with serialization, file/session stores, workflow events, checkpoint restore), not prose. Both harness and Magentic paths have explicit prompts, tool schemas, tests (`python/packages/core/tests/core/test_harness_todo.py:242`, `python/packages/orchestrations/tests/test_magentic.py:1148`), safeguards (atomic writes, per-session locks, stall/replan limits, approval gate), and observability (workflow events `MagenticPlanCreatedEvent`/`MagenticReplannedEvent`, devui intermediate mapping). Deductions: no single unified planner, planning is opt-in and bifurcated (harness todo vs Magentic vs static graph), and reusability is session/graph-scoped rather than cross-run library.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Planner prompts - todo | `DEFAULT_TODO_INSTRUCTIONS` instructs model: complex->todos_add, simple->direct, with guidelines | `python/packages/core/agent_framework/_harness/_todo.py:25-48` |
| Planner prompts - Magentic facts | `ORCHESTRATOR_TASK_LEDGER_FACTS_PROMPT`, `ORCHESTRATOR_TASK_LEDGER_PLAN_PROMPT` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108-145` |
| Planner prompts - Magentic full ledger | `ORCHESTRATOR_TASK_LEDGER_FULL_PROMPT` renders combined plan event text | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:147-167` |
| Planner prompts - Magentic update | `ORCHESTRATOR_TASK_LEDGER_FACTS_UPDATE_PROMPT`, `ORCHESTRATOR_TASK_LEDGER_PLAN_UPDATE_PROMPT` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:169-192` |
| Planner prompts - progress ledger | `ORCHESTRATOR_PROGRESS_LEDGER_PROMPT` JSON schema for 5-way decision | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:194-243` |
| Planner prompts - .NET | `TaskLedgerPlanPrompt`, `TaskLedgerPlanUpdatePrompt`, `ProgressLedgerPrompt` via `MagenticDefaultPrompts` | `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticDefaultPrompts.cs:1` |
| Planning agents - harness | `create_harness_agent` assembles TodoProvider+ModeProvider+Loop | `python/packages/core/agent_framework/_harness/_agent.py:302` |
| Planning agents - Magentic manager | `MagenticManagerBase` abstract plan/replan/create_progress_ledger; `StandardMagenticManager` LLM-backed | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:469`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:514` |
| Planning agents - .NET MagenticManager | `MagenticManager.UpdatePlanAsync` creates facts+plan via managerAgent | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:36-63` |
| Workflow graphs - builder | `WorkflowBuilder` with `add_edge`, `add_fan_out_edges`, `add_switch_case_edge_group`, `add_chain`, `add_fan_in_edges` | `python/packages/core/agent_framework/_workflows/_workflow_builder.py:230-618` |
| Workflow graphs - explicit plan sample | `AgenticPlanningTools.CreatePlan` / `UpdatePlanStepAsync` create/update PlanSteps as JSONPatch | `dotnet/samples/05-end-to-end/AGUIClientServer/AGUIDojoServer/AgenticUI/AgenticPlanningTools.cs:9-46` |
| Task decomposition code - TodoProvider | `TodoProvider.before_run` injects 5 tools + instructions + current list message | `python/packages/core/agent_framework/_harness/_todo.py:493-615` |
| Task decomposition code - TodoItem types | `TodoItem`, `TodoInput`, `TodoCompleteInput` with serialization/validation | `python/packages/core/agent_framework/_harness/_todo.py:51-177` |
| Task decomposition storage | `TodoStore` ABC, `TodoSessionStore` (AgentSession.state), `TodoFileStore` (per-session JSON, atomic replace) | `python/packages/core/agent_framework/_harness/_todo.py:228-444` |
| Planning config - harness disable | `disable_todo`, `todo_provider`, `disable_mode`, `mode_provider` | `python/packages/core/agent_framework/_harness/_agent.py:318-321`, `python/packages/core/agent_framework/_harness/_agent.py:145-177` |
| Planning config - loop | `loop_should_continue`, `loop_next_message`, `loop_max_iterations`, `AgentLoopMiddleware` insertion outermost via `create_harness_agent` | `python/packages/core/agent_framework/_harness/_agent.py:337-354`, `python/packages/core/agent_framework/_harness/_agent.py:643-658` |
| Planning config - Magentic limits | `max_stall_count`, `max_reset_count`, `max_round_count`, `progress_ledger_retry_count`, `require_plan_signoff` | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:472-482`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:888-900` |
| Planning config - Workflow | `max_iterations`, `output_from`, `intermediate_output_from`, `checkpoint_storage` | `python/packages/core/agent_framework/_workflows/_workflow_builder.py:91-102` |
| Planning visibility - todo | `todos_get_remaining`, `todos_get_all`, session state, before_run current list message | `python/packages/core/agent_framework/_harness/_todo.py:577-615` |
| Planning visibility - loop progress | `todos_remaining` predicate (`python/packages/core/agent_framework/_harness/_loop.py:925`), `todos_remaining_message` (`python/packages/core/agent_framework/_harness/_loop.py:989`), `background_tasks_running` helpers | `python/packages/core/agent_framework/_harness/_loop.py:925-1021` |
| Planning visibility - Magentic events | `MagenticPlanCreatedEvent`, `MagenticReplannedEvent`, `MagenticProgressLedgerUpdatedEvent` (dotnet); `MagenticOrchestratorEvent` PLAN_CREATED/REPLANNED/PROGRESS_LEDGER_UPDATED (python) | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:14-58`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:786-800` |
| Planning visibility - workflow events | `WorkflowEvent` type `magentic_orchestrator`, `WorkflowBuilder` output filtering `get_outputs`/`get_intermediate_outputs` | `python/packages/core/agent_framework/_workflows/_events.py:1`, `python/packages/core/agent_framework/_workflows/_workflow_builder.py:750-804` |
| Planning reuse - checkpointing | Magentic orchestrator `OnCheckpointingAsync`/`OnCheckpointRestoredAsync` persists `MagenticTaskContext` + `CurrentSpeaker`; `StandardMagenticManager.on_checkpoint_save/restore` persists `task_ledger` | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:357-404`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:753-768` |
| Planning reuse - todo file store | `TodoFileStore._save_state_sync` atomic write via temp+replace, path traversal guards | `python/packages/core/agent_framework/_harness/_todo.py:431-443`, `python/packages/core/agent_framework/_harness/_todo.py:344-372` |
| Testing - todo | `test_todo_provider_tools_manage_session_state`, `test_todo_file_store_round_trips_state`, concurrent mutations | `python/packages/core/tests/core/test_harness_todo.py:271-367` |
| Testing - loop | `test_harness_loop.py` (AgentLoopMiddleware streaming/non-streaming, judge, todo gating) inferred via search | `python/packages/core/tests/core/test_harness_loop.py:1` |
| Testing - Magentic | `_MagenticTaskLedger` serialization, manager plan/replan/progress ledger tests | `python/packages/orchestrations/tests/test_magentic.py:1148-1296`, `python/packages/core/tests/workflow/test_workflow_kwargs.py:1` |
| Documentation - harness instructions | `DEFAULT_HARNESS_INSTRUCTIONS` "Think through the task before acting. Break complex work into clear steps." | `python/packages/core/agent_framework/_harness/_agent.py:54-69` |

## Answers to Dimension Questions

**1. Where does planning happen?**

Planning is **distributed across four locations**, none of which is a single global planner:

- **Inside prompts** (`python/packages/core/agent_framework/_harness/_todo.py:25` - todo guidelines; `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:108-243` - Magentic fact/plan/progress prompts; `dotnet/src/Microsoft.Agents.AI.Workflows/MagenticDefaultPrompts.cs:1`). The harness prompt tells the model *when* to plan (complex=>todos, simple=>direct) and *how* to maintain the list. This is the implicit planning lever.
- **Inside runtime code / ContextProvider** (`python/packages/core/agent_framework/_harness/_todo.py:493` `before_run` injects tools/instructions/messages; `python/packages/core/agent_framework/_workflows/_workflow_builder.py:53` graph construction). `TodoProvider` is a `ContextProvider` (`python/packages/core/agent_framework/_harness/_todo.py:446`) that runs every turn.
- **Inside a planner agent** (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:514` `StandardMagenticManager` wraps a `SupportsAgentRun` manager agent; `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:36` `MagenticManager` wraps `managerAgent`). Magentic delegates facts+plan generation to the manager agent via separate LLM calls.
- **Inside workflow graph** (`python/packages/core/agent_framework/_workflows/_workflow_builder.py:230` edges are the static plan). For deterministic workflows the builder is the plan.
- **Sample-level external planning**: `dotnet/samples/05-end-to-end/AGUIClientServer/AGUIDojoServer/AgenticUI/AgenticPlanningTools.cs:9` shows planning as explicit UI tools (`create_plan`/`update_plan_step`) interpreted by the frontend, not the framework runtime.

No evidence of an external planning system (foundry hosting is deployment, not planning).

**2. Who owns the plan?**

Ownership is **split by subsystem, always runtime-anchored but model-authored**:

- **Harness todo plan**: **Model creates, runtime owns persistence & evaluation.** The model calls `todos_add` (`python/packages/core/agent_framework/_harness/_todo.py:505`) to author items; `TodoProvider` owns `TodoStore` (`python/packages/core/agent_framework/_harness/_todo.py:480`) and serializes via `TodoSessionStore`/`TodoFileStore`. The loop predicate `todos_remaining()` (`python/packages/core/agent_framework/_harness/_loop.py:925`) reads that store to decide `should_continue`, not the model's self-report. This mirrors `TodoCompletionLoopEvaluator` in .NET (`dotnet/src/Microsoft.Agents.AI/Harness/Loop/TodoCompletionLoopEvaluator.cs:1`).
- **Magentic plan**: **Orchestrator runtime owns `TaskLedger`/`MagenticContext`; manager agent authors it.** `MagenticOrchestrator._magentic_context` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:906`, `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:93`) holds `task_ledger` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:525`). `StandardMagenticManager.plan()` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:621`) produces the ledger but `MagenticOrchestrator` stores, emits, checkpoints, and broadcasts it.
- **Workflow graph plan**: **Developer owns the graph; runtime owns execution.** `WorkflowBuilder` (`python/packages/core/agent_framework/_workflows/_workflow_builder.py:91`) is built by developer code, then immutable `Workflow` owns edge Groups and checkpoint state.

Thus "who owns the plan?" answer: runtime owns the *object* and its lifecycle; model/developer owns the *authorship*.

**3. Is planning required?**

**No — planning is strictly optional and off by default in some paths.**

- Harness: `create_harness_agent(..., disable_todo: bool=False)` default ON but explicitly disableable (`python/packages/core/agent_framework/_harness/_agent.py:318`), and `disable_todo=True` removes `TodoProvider` entirely (`python/packages/core/agent_framework/_harness/_agent.py:176`). `loop_should_continue` defaults `None` → no loop (`python/packages/core/agent_framework/_harness/_agent.py:337`); without it todos are just tools with no forcing function. Tests cover disabled paths.
- Workflows: `WorkflowBuilder` requires no planner; you can `WorkflowBuilder(start_executor=a).add_edge(a,b).build()` (`python/packages/core/agent_framework/_workflows/_workflow_builder.py:82`) with zero decomposition logic.
- Magentic: only active if you instantiate `MagenticBuilder`/`MagenticOrchestrator` (`python/packages/orchestrations/agent_framework_orchestrations/__init__.py:9`); its `require_plan_signoff` defaults `False` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:888`). Even there `plan()` is one-time; workflow functions without replanning if stall limits not hit.

The default harness instructions (`python/packages/core/agent_framework/_harness/_agent.py:54`) say "Break complex work into clear steps" but it is guidance, not enforcement — `todos_add` approval_mode `never_require` (`python/packages/core/agent_framework/_harness/_todo.py:505`) means model can ignore it.

**4. Is planning visible?**

**Yes — planning artifacts are first-class observable runtime objects, not hidden chain-of-thought.**

- **Todo visibility**: `todos_get_all`/`todos_get_remaining` tools (`python/packages/core/agent_framework/_harness/_todo.py:577-589`), `before_run` injects `"### Current todo list"` user message (`python/packages/core/agent_framework/_harness/_todo.py:597`), `todos_remaining_message` renders open items (`python/packages/core/agent_framework/_harness/_loop.py:989`). Session-scoped: `AgentSession.state[source_id]` (`python/packages/core/agent_framework/_harness/_todo.py:245`). DevUI maps Magentic intermediate messages via `test_mapper.py` `Planning: ...` handling.
- **Magentic visibility**: Three workflow events: `MagenticPlanCreatedEvent`/`MagenticReplannedEvent`/`MagenticProgressLedgerUpdatedEvent` (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:14-58`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:786-800`) emitted via `ctx.add_event(WorkflowEvent("magentic_orchestrator", ...))` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:944-953`). Full task ledger is a `Message` broadcast to participants and checkpointed.
- **Workflow visibility**: `WorkflowBuilder` `output_from`/`intermediate_output_from` controls `type='output'` vs `intermediate'` event routing (`python/packages/core/agent_framework/_workflows/_workflow_builder.py:118-142`), enabling planners to be observed without polluting final output. `get_intermediate_outputs()` exposes planner traces.
- **Human sign-off visibility**: `MagenticPlanReviewRequest`/`Response` with `request_info` port (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:105-122`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:835-858`) surfaces plan for approval.

Gaps: No unified dashboard; visibility requires subscribing to WorkflowEvent stream or querying session state.

**5. Is planning reusable?**

**Partially — reusable within a session/workflow execution and via checkpointing, but not as a cross-run library.**

- **Todo reuse**: `TodoSessionStore` preserves across turns in same `AgentSession` (`python/packages/core/agent_framework/_harness/_todo.py:245`); `TodoFileStore` persists per `session_id` + `source_id` to JSON file (`python/packages/core/agent_framework/_harness/_todo.py:288-444`) with atomic replace and path guards, enabling durable reuse across process restarts for that session. However no export/import as template; `TodoInput`/`TodoItem` not cataloged.
- **Magentic reuse**: `StandardMagenticManager.on_checkpoint_save/restore` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:753-768`) and dotnet `OnCheckpointingAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:357-404`) serialize `task_ledger` and `MagenticTaskContext` (`task`, `chat_history`, `round_count`, etc.). Rehydrated workflow resumes from same plan. The builder itself is reusable (create new Workflow from same participants/manager) but the generated plan is per-run.
- **Workflow graph reuse**: `Workflow` is immutable and reusable across runs (`workflow.run("hello")` then `workflow.run("world")` docs at `python/packages/core/agent_framework/_workflows/_workflow_builder.py:764`), but graph is static, not a learned plan library.
- **Not reusable**: No plan registry, no skill-like packaging of plans, no search over prior plans. Each harness session starts empty (`next_id=1` `python/packages/core/agent_framework/_harness/_todo.py:267`).

## Architectural Decisions

| Decision | Evidence | Tradeoff |
|----------|----------|----------|
| **Prompt + Tool duality for planning** — TodoProvider pairs `DEFAULT_TODO_INSTRUCTIONS` with 5 tools, making planning model-initiated but runtime-verifiable. | `python/packages/core/agent_framework/_harness/_todo.py:25`, `python/packages/core/agent_framework/_harness/_todo.py:493-595` | Pro: lightweight, no separate planner agent, works with any chat client. Con: relies on model compliance; model can ignore instructions (no guardrail). |
| **Plan as durable typed object not prose** — `TodoItem`/`TodoStore`/`_MagenticTaskLedger` with `to_dict`/`from_dict` and explicit store abstraction. | `python/packages/core/agent_framework/_harness/_todo.py:51-228`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:271-287` | Pro: observable, testable, checkpointable. Con: dual representations (harness todo vs Magentic ledger) confuse mental model. |
| **Optional harness vs explicit orchestration split** — `create_harness_agent` wires todo/mode/memory/loop optionally; `MagenticOrchestrator` is separate orchestrations package. | `python/packages/core/agent_framework/_harness/_agent.py:302-350`, `python/packages/core/agent_framework/orchestrations/__init__.py:16-69` | Pro: progressive complexity; simple agents don't pay Magentic cost. Con: two planning idioms to learn; no bridge between todo list and Magentic plan. |
| **Loop middleware as outer wrapper for planning enforcement** — `AgentLoopMiddleware` + `todos_remaining` helper; harness inserts loop outermost over approval middleware. | `python/packages/core/agent_framework/_harness/_loop.py:217`, `python/packages/core/agent_framework/_harness/_agent.py:643-658` | Pro: approval-safe (`_has_pending_approval_request` escape hatch at `python/packages/core/agent_framework/_harness/_loop.py:442`), bounded by `max_iterations`. Con: `fresh_context` vs accumulation semantics subtle. |
| **Manager-agent pattern for Magentic** — Manager is itself a `SupportsAgentRun` agent, called fresh-session per ledger creation (`create_session()` each call). | `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:592-612`, `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:42-58` | Pro: reuses all agent features (tools, history providers), stateless prevents duplication bug (comment at `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:599-610`). Con: extra LLM cost/latency per plan/replan + progress ledger. |
| **Checkpoint-first durability** — Both todo (`TodoFileStore` atomic replace `python/packages/core/agent_framework/_harness/_todo.py:431`) and Magentic (`OnCheckpointingAsync` `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:357`) treat plan as state to survive restarts. | `python/packages/core/agent_framework/_harness/_todo.py:431-443`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:753` | Pro: durable long-running plans. Con: checkpoint storage is caller-provided; mis-scoping leaks isolation. |
| **Human-in-the-loop plan signoff as port** — `MagenticPlanReviewRequest/Response` via `request_info` / `AddPortHandler<MagenticPlanReviewRequest, MagenticPlanReviewResponse>`. | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:105-160`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:835-1055` | Pro: first-class HITL without polling. Con: only Magentic supports it; harness todo has no signoff. |

## Notable Patterns

- **ContextProvider progressive disclosure**: `TodoProvider` extends `ContextProvider` (`python/packages/core/agent_framework/_harness/_todo.py:446`) and uses `SessionContext.extend_instructions/extend_tools/extend_messages` pattern shared with `FileMemoryProvider`, `SkillsProvider`. Planning is injected per-run, not baked into agent.
- **Per-session lock + WeakKeyDictionary**: `TodoProvider._mutation_locks: WeakKeyDictionary[AgentSession, asyncio.Lock]` (`python/packages/core/agent_framework/_harness/_todo.py:482`) serializes concurrent `todos_add`/`todos_complete` calls (`python/packages/core/tests/core/test_harness_todo.py:322-367` shows gather test) and auto-GCs on session deletion (`python/packages/core/tests/core/test_harness_todo.py:200-212`).
- **Judge / todo loopEvaluator polymorphism**: .NET `LoopAgent` + `TodoCompletionLoopEvaluator` / `AIJudgeLoopEvaluator` (`dotnet/src/Microsoft.Agents.AI/Harness/Loop/`) and Python `AgentLoopMiddleware.with_judge` (`python/packages/core/agent_framework/_harness/_loop.py:349`) share same middleware shape — `should_continue` predicate controls continuation, `next_message` builds nudge, `progress` injection carries continuity.
- **Atomic plan persistence**: `TodoFileStore._save_state_sync` writes to `*.tmp.{pid}` then `os.replace` (`python/packages/core/agent_framework/_harness/_todo.py:431-443`) to avoid corruption on crash — same pattern as file-backed session stores.
- **Structured progress ledger as JSON decision** (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:194-242`): 5-way JSON (`is_request_satisfied`, `is_in_loop`, `is_progress_being_made`, `next_speaker`, `instruction_or_question`) with retry/backoff on parse failure (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:719-739`, `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:67-106`).
- **Workflow graph as plan DSL**: `WorkflowBuilder` fan-out/fan-in/switch-case (`python/packages/core/agent_framework/_workflows/_workflow_builder.py:282-563`) lets developer declaratively encode parallel vs sequential decomposition without LLM planning.

## Tradeoffs

| Tradeoff | Pro | Con |
|----------|-----|-----|
| **Model-authored todo vs deterministic workflow graph** | Harness todo is adaptive at runtime (model decomposes per request). | Non-deterministic; plan quality varies by model; no validation that plan covers request. |
| **Separate harness todo and Magentic task ledger** | Each optimized for its scale (single-agent todo vs multi-agent team plan). | No interoperability: a todo list cannot become a Magentic plan and vice versa. Duplicate prompt/ledger concepts. |
| **Opt-in planning (disable_todo, optional loop)** | Zero overhead when not needed; simple `Agent(client).run()` still works. | Easy to forget to wire `todos_remaining` loop, leaving todos as decorative tools with no enforcement. |
| **Manager agent per-plan LLM call** (Magentic) | Leverages full agent capabilities (tools, history) to ground plan; prompt overrides allow customization. | 2 LLM calls for initial plan + 2 for replan + 1 per progress ledger → O(rounds) extra cost/latency. |
| **Session-state vs file-state storage** | Session store is zero-config; file store (`TodoFileStore`) durable across restarts with encoding guards. | File store is `@experimental` (`python/packages/core/agent_framework/_harness/_todo.py:287`) and requires owner/session isolation reasoning; session store lost on process exit unless checkpointing. |
| **Loop outermost over approval** | Prevents approval deadlock; `_has_pending_approval_request` (`python/packages/core/agent_framework/_harness/_loop.py:442`) stops loop to surface approval prompt. | Loop controls message list (`context.messages = next_messages` at `python/packages/core/agent_framework/_harness/_loop.py:588`) — bugs could orphan conversation. |
| **Progress ledger retries but facts/plan don't** (`StandardMagenticManager._complete` no retry vs progress ledger retry count 3) | Fail fast on plan generation keeps workflow simple. | Transient model failure during plan generation aborts entire workflow; progress ledger retry handles only JSON parse, not full semantic errors. |

## Failure Modes / Edge Cases

| Failure | Behavior | Evidence |
|---------|----------|----------|
| **Model ignores todo instructions** | Todos remain empty; `todos_remaining()` returns False immediately, loop exits after one iteration (if wired) — silent success with no decomposition. | `python/packages/core/agent_framework/_harness/_loop.py:980-986` returns False when no open todos; `DEFAULT_TODO_INSTRUCTIONS` (`python/packages/core/agent_framework/_harness/_todo.py:25`) is guidance not enforced. |
| **Concurrent todo mutations collide** | Without lock, IDs duplicate/lost. Mitigated by per-session `asyncio.Lock` + `_safe_next_id` clamping (`python/packages/core/agent_framework/_harness/_todo.py:223-225`). | `python/packages/core/agent_framework/_harness/_todo.py:485-491`, test at `python/packages/core/tests/core/test_harness_todo.py:322-367` asserts unique IDs under `asyncio.gather`. |
| **Corrupted session state** | `TodoSessionStore.load_state` validates dict/list/int types and item mappings, raising `ValueError` with index (`python/packages/core/agent_framework/_harness/_todo.py:249-275`, `python/packages/core/agent_framework/_harness/_todo.py:193-202`). | Tests `python/packages/core/tests/core/test_harness_todo.py:152-178` cover non-mapping, non-dict, bad next_id. |
| **File store path traversal / symlink escape** | `TodoFileStore._path_segment` rejects `/` `\` separators, leading dots, Windows reserved names, checks `is_relative_to(_base_root)` (`python/packages/core/agent_framework/_harness/_todo.py:344-372`). | Test `python/packages/core/tests/core/test_harness_todo.py:214-221` `session_id="../escape"` raises. |
| **Crash mid-write corrupts plan** | Atomic temp+replace (`os.replace`) prevents truncated `todos.json` (`python/packages/core/agent_framework/_harness/_todo.py:431-443` comment cites crash safety). | Test `python/packages/core/tests/core/test_harness_todo.py:130-149` monkeypatches `os.replace` to simulate disk full, asserts original intact. |
| **Manager returns empty/multiple messages** | `MagenticManager.CheckResponseAsync` throws on empty, warns and uses last on multiple (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:19-30`). Python `_complete` mirrors (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:614-617`). | Warning "Planner Agent did not return any messages." / "returned multiple messages". |
| **Progress ledger unparseable JSON** | Retries up to `progress_ledger_retry_count` (default 3) with 250ms backoff, emits `WorkflowWarningEvent`, falls back to `ResetAndReplan` on ultimate failure. | `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticManager.cs:71-106`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:719-739`. |
| **Stall detection loop never progressing** | `stallCount` increments when `is_progress_being_made==False` or `is_in_loop==True`, decrements otherwise, capped at 0, triggers `ResetAndReplan` when `>max_stall_count` (default 3) (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1117-1125`, `dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:290-304`). | Can still loop forever if LLM keeps returning `progress=True` falsely — no ground-truth verification. |
| **Round/reset limit hit** | `CheckLimits` / `_check_within_limits_or_complete` yields termination message `Workflow terminated due to reaching maximum {round|reset} count` and sets `IsTerminated=True` (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:248-262`, `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1229-1265`). | Previously participants continue broadcasting, now terminated workflow throws on next `TakeTurnAsync` (`dotnet/src/Microsoft.Agents.AI.Workflows/Specialized/Magentic/MagenticOrchestrator.cs:188-191`). |
| **Loop with `todos_remaining` and `AgentModeProvider`** | `todos_remaining(looping_modes=[...])` gates continuation by current mode; if mode mismatched, returns False (`python/packages/core/agent_framework/_harness/_loop.py:925-984`). Empty `looping_modes` raises `ValueError`. | `python/packages/core/agent_framework/_harness/_loop.py:952-954` validates non-empty. |
| **Approval request inside loop** | `_has_pending_approval_request` scans `function_approval_request` contents (`python/packages/core/agent_framework/_harness/_loop.py:442-459`); if present, loop breaks to surface to caller instead of consuming approval internally. | Prevents HITL deadlock described in docstring (`python/packages/core/agent_framework/_harness/_loop.py:442-451`). |
| **Fresh-context vs history accumulation** | `fresh_context=True` snapshots session via `to_dict` before loop and `from_dict` restore (`python/packages/core/agent_framework/_harness/_loop.py:461-476`, `python/packages/core/agent_framework/_harness/_loop.py:583-587`). Without it, history grows linearly per iteration. | `AgentLoopMiddleware` note at `python/packages/core/agent_framework/_harness/_loop.py:312-320` warns in-loop working-state discarded, continuity only via progress log. |

## Future Considerations

- **Unify harness todo and Magentic ledger**: Currently two plan representations. A bridge (todo items → task ledger facts/plan or vice versa) or a shared `Plan` interface would reduce conceptual overhead. Could promote `_MagenticTaskLedger` to public and allow `TodoProvider` to feed its initial facts.
- **Enforce planning completeness**: Add optional `require_todos` or `min_todos` guard that rejects empty plan before proceeding (parallel to `require_plan_signoff` for Magentic). Currently model can skip planning.
- **Plan templates / reusable libraries**: `WorkflowBuilder` graphs are reusable but hand-coded; expose a plan registry (e.g., saved `TodoProvider` snapshots or serialized `Workflow` JSON) for cross-session replay — useful for repetitive enterprise workflows.
- **Progress ledger ground-truth signals**: Current stall detection relies on LLM self-assessment (`is_progress_being_made`). Adding tool-observation signals (file writes, search results) or human feedback as objective progress would harden replan trigger.
- **Graduate `TodoFileStore`**: Still `@experimental` (`python/packages/core/agent_framework/_harness/_todo.py:287`) despite being used in `test_harness_todo.py:87` — stabilizing its `base_path`/`owner_state_key` contract and documenting multi-tenant isolation would support production use.
- **Observability integration**: Wire todo/magentic plan events to OpenTelemetry (`otel_provider_name` at `python/packages/core/agent_framework/_harness/_agent.py:340`) as structured spans with plan diff attributes, not just generic workflow events.
- **Agentic planning sample productization**: `AgenticPlanningTools` (`dotnet/samples/05-end-to-end/AGUIClientServer/AGUIDojoServer/AgenticUI/AgenticPlanningTools.cs:9`) shows UI-driven plan patching as JSONPatch; promoting this to a core provider would make plans visible to the frontend without custom tools.

## Questions / Gaps

| Gap | Search Boundary | Impact |
|-----|-----------------|--------|
| No plan optimizer/selector among alternatives | Searched `python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:1`, `python/packages/core/agent_framework/_workflows/` — no A* or cost-based planning, only single plan + replan. | Framework cannot compare candidate plans; replan is reactive, not proactive. |
| No cross-source plan transfer | `TodoProvider` and `MagenticOrchestrator` share no conversion; `declarative-agents/` YAML not inspected for plan primitives (glob found none). | Plans not portable across harness/workflow/declarative boundaries. |
| `TodoFileStore` experimental lifecycle unclear | Annotated `@experimental(feature_id=HARNESS)` at `python/packages/core/agent_framework/_harness/_todo.py:287` despite extensive tests (`python/packages/core/tests/core/test_harness_todo.py:87`). | Consumers uncertain about stability commitment. |
| Plan signoff only for Magentic, not harness todos | `require_plan_signoff` exists only on `MagenticBuilder`/`MagenticOrchestrator` (`python/packages/orchestrations/agent_framework_orchestrations/_magentic.py:888`), no `require_todo_signoff`. | Harness plans cannot be human-approved before execution. |
| No declarative YAML planning primitive | Glob of `declarative-agents/` shows samples but no `plan:` key in tool search; `docs/decisions/` contains no ADR about planning. | Declarative infrastructure cannot author plans without code. |
| Duration/scale limits not stress-tested in repo | `max_stall_count=3`, `DEFAULT_MAX_ITERATIONS=10` (`python/packages/core/agent_framework/_harness/_loop.py:122`), `DEFAULT_JUDGE_MAX_ITERATIONS=5` — unit tests only small counts. | Unknown behavior under 100s of todos or long Magentic runs. |

---

Generated by `06.01-planning-location-and-responsibility` against `agent-framework`.
