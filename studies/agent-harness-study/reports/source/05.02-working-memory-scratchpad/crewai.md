# Source Analysis: crewai

## Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based framework; LanceDB/SQLite storage; asyncio + threads) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI has **no single first-class "scratchpad" object**; a repo-wide search for `scratchpad`, `working_memory`, `notepad`, and `notes` returned no implementation symbols (only incidental docstrings). Instead, working state is decomposed into several explicit, typed structures spread across two executor generations:

1. A structured, pydantic-validated **`AgentExecutorState`** holding the ReAct message list, the generated plan, a status-tracked todo list, per-step observations, replan counters, and an audit-only execution log (`lib/crewai/src/crewai/experimental/agent_executor.py:135-170`).
2. A **`TodoList`/`TodoItem`** model with lifecycle statuses (`pending → running → completed/failed`), dependency tracking, results, and replan-aware replacement of pending steps (`lib/crewai/src/crewai/utilities/planning_types.py:27-195`).
3. Deliberately minimal hand-off records between the planner/observer and the per-step worker — `StepExecutionContext`/`StepResult` carry only final dependency results and audit metadata, "never LLM message histories" (`lib/crewai/src/crewai/utilities/step_execution_context.py:1-64`).
4. The legacy `CrewAgentExecutor`, which keeps working state as a plain mutable `messages` list (`lib/crewai/src/crewai/agents/crew_agent_executor.py:192-474`).

The design intent is clearest in the Plan-and-Execute path: hidden planner outputs (plan text, structured steps) live in executor state rather than user-visible task objects, with an in-code comment explicitly forbidding the older behavior of mutating `task.description` with plan text because "it's a shared object that accumulates plan text on re-invoke" (`lib/crewai/src/crewai/experimental/agent_executor.py:382-384`). Working notes are kept separate from facts via an isolated StepExecutor that builds fresh messages per step (`lib/crewai/src/crewai/agents/step_executor.py:66-75`), and only distilled LLM-extracted memories — not raw scratch work — flow into long-term memory (`lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:42-63`). All of this working state is cleared at every invocation boundary (`lib/crewai/src/crewai/experimental/agent_executor.py:2830-2845`).

## Rating

**7 / 10** — CrewAI presents a clear, tested model of working memory: plan/todos/observations are explicit typed state with unit tests (`lib/crewai/tests/agents/test_agent_executor.py:1446-1668`, `lib/crewai/tests/utilities/test_structured_planning.py:266-660`), planner↔executor isolation is enforced by frozen dataclasses, context-window compaction exists as an operational safeguard (`lib/crewai/src/crewai/utilities/agent_utils.py:1048-1131`), and clearing at task boundaries is deterministic. It falls short of 8–10 because (a) there is no unified scratchpad interface — working memory is fragmented across two executors with divergent semantics, (b) the legacy planning path still mutates the shared `task.description` (`lib/crewai/src/crewai/agent/utils.py:55`), and (c) raw internal content leaks into observability channels (event payloads, `TaskOutput.messages`) without redaction.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Structured working state | `AgentExecutorState` fields: `messages`, `iterations`, `plan`, `plan_ready`, `todos`, `replan_count`, `last_replan_reason`, `observations`, `execution_log` | lib/crewai/src/crewai/experimental/agent_executor.py:135-170 |
| Todo state machine | `TodoItem.status` Literal `pending/running/completed/failed`; `TodoList.mark_running/mark_completed/mark_failed`; `replace_pending_todos` for replanning | lib/crewai/src/crewai/utilities/planning_types.py:11,27-42,90-110,185-195 |
| Hidden planner output | `ReasoningPlan(plan, steps, ready)` produced by `AgentReasoning`; stored into `state.plan`/`state.plan_ready`, never written to task | lib/crewai/src/crewai/utilities/reasoning_handler.py:30-48; lib/crewai/src/crewai/experimental/agent_executor.py:374-380 |
| Plan-to-todos conversion | `_create_todos_from_plan` maps `PlanStep`s to pending `TodoItem`s | lib/crewai/src/crewai/experimental/agent_executor.py:390-407 |
| No task pollution (new path) | Comment: "Do NOT mutate task.description — it's a shared object that accumulates plan text on re-invoke" | lib/crewai/src/crewai/experimental/agent_executor.py:382-384 |
| Task pollution (legacy path) | `handle_reasoning` appends plan into shared `task.description`: `task.description += f"\n\nPlanning:\n{...}"` | lib/crewai/src/crewai/agent/utils.py:29-57 |
| Planner/executor isolation contract | "StepExecutor owns its own message list per invocation. It never reads or writes the AgentExecutor's state" | lib/crewai/src/crewai/agents/step_executor.py:64-75 |
| Facts-vs-notes mediation types | `StepExecutionContext` "No LLM message history, no execution traces"; `StepResult` tool details "NOT passed to subsequent steps or the Planner" | lib/crewai/src/crewai/utilities/step_execution_context.py:13-30,44-58 |
| Dependency-result-only context | `_build_context_for_todo` passes only final `dep_todo.result` strings | lib/crewai/src/crewai/experimental/agent_executor.py:610-641 |
| Audit-only internal log | `execution_log` described as "Audit trail for debugging (NOT used for LLM calls)"; observation entries appended | lib/crewai/src/crewai/experimental/agent_executor.py:167-170,678-690 |
| Observations store | `observations: dict[int, StepObservation]` keyed by step number; `key_information_learned` captures new facts per step | lib/crewai/src/crewai/experimental/agent_executor.py:163-166,676; lib/crewai/src/crewai/utilities/planning_types.py:212-278 |
| Boundary clearing (sync) | `invoke()` resets messages, iterations, plan, todos, replan_count, observations, execution_log before each run | lib/crewai/src/crewai/experimental/agent_executor.py:2830-2845 |
| Boundary clearing (async) | Same reset block in `invoke_async` | lib/crewai/src/crewai/experimental/agent_executor.py:2916-2931 |
| Context compaction under pressure | `summarize_messages` rewrites history in place, preserving system messages and attached files; `handle_context_length` summarizes or raises `SystemExit` | lib/crewai/src/crewai/utilities/agent_utils.py:1048-1131,795-832 |
| Compaction wiring | `recover_from_context_length` routes through `handle_context_length` on `is_context_length_exceeded` | lib/crewai/src/crewai/experimental/agent_executor.py:2787-2800 |
| Long-term write distillation | `_save_to_memory` extracts discrete memories from "Task/Agent/Expected result/Result" dump before storing; skips delegation outputs | lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:31-65 |
| Extraction helper | `Memory.extract_memories` is pure (no storage); `extract_memories_from_content` uses LLM analysis | lib/crewai/src/crewai/memory/unified_memory.py:667-679; lib/crewai/src/crewai/memory/analyze.py:14-46 |
| Durable long-term backing | Storage resolution to LanceDB/Qdrant/backed paths | lib/crewai/src/crewai/memory/unified_memory.py:232-251 |
| Write/read barrier | Background save pool with `drain_writes()` read barrier before recall and shutdown | lib/crewai/src/crewai/memory/unified_memory.py:297-364,711-713 |
| Replay persistence of outputs | `KickoffTaskOutputsSQLiteStorage` SQLite table `latest_kickoff_task_outputs` incl. `was_replayed`; fed by `Crew._store_execution_log` | lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19-111; lib/crewai/src/crewai/crew.py:1479-1507 |
| Reset API (clearing) | `reset_memories(command_type)` valid targets include `memory`, `kickoff_outputs`, `all` | lib/crewai/src/crewai/crew.py:2281-2418 |
| Per-task tool-state reset | `self.tools_handler.last_used_tool = None` and `reset_tool_failures()` at task start | lib/crewai/src/crewai/agent/core.py:556-562 |
| User-visible transcript copy | `save_last_messages` sanitizes executor messages into `agent._last_messages` | lib/crewai/src/crewai/agent/utils.py:247-283 |
| Transcript exposure on output | `TaskOutput(messages=agent.last_messages)` | lib/crewai/src/crewai/task.py:729,885,1434,1555 |
| Privacy flag on records | `MemoryRecord.private`: visible only when source matches or `include_private=True` | lib/crewai/src/crewai/memory/types.py:67-71; enforced lib/crewai/src/crewai/memory/unified_memory.py:746-751 |
| Read-only memory guardrails | `read_only=True` makes saves no-ops; RememberTool omitted when read-only | lib/crewai/src/crewai/memory/unified_memory.py:148-151,466-467,561-562; lib/crewai/src/crewai/tools/memory_tools.py:104-130 |
| Namespaced scopes | Crew root scope `/crew/<name>`; agent sub-scope `/…/agent/<role>` | lib/crewai/src/crewai/crew.py:653-688; lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:51-61 |
| Agent-facing scratch tools | `RecallMemoryTool` ("Search memory") and `RememberTool` ("Save to memory") injected when memory available | lib/crewai/src/crewai/tools/memory_tools.py:25-101; wired lib/crewai/src/crewai/crew.py:1790-1802,1681-1683 |
| Event-bus leakage surface | `MemorySaveStartedEvent(value=content)` carries full raw content | lib/crewai/src/crewai/memory/unified_memory.py:474-481 |
| Tests: plan in state | `test_agent_kickoff_with_planning_stores_plan_in_state`, `test_executor_state_contains_plan_after_planning` | lib/crewai/tests/agents/test_agent_executor.py:1446-1467,1534-1568 |
| Tests: todo lifecycle & deps | `test_dependency_aware_execution`, `test_create_todos_from_plan_steps` | lib/crewai/tests/utilities/test_structured_planning.py:624-660,266+ |
| Tests: observation fallbacks | `test_heuristic_observation_reflects_step_success`, `test_observe_fallback_is_conservative_on_llm_error` | lib/crewai/tests/agents/test_agent_executor.py:1396-1445 |
| Tests: memory barriers | `test_recall_drains_pending_writes`, `test_drain_writes_reports_background_save_failure_without_raising` | lib/crewai/tests/memory/test_unified_memory.py:1038-1094 |

## Answers to Dimension Questions

**1. Does the agent keep private task state?**
Yes. The experimental `AgentExecutor` holds private working state in `AgentExecutorState`: conversation messages, the plan, the todo list, per-step observations, replan counters, and an audit-only execution log (`lib/crewai/src/crewai/experimental/agent_executor.py:135-170`). The `StepExecutor` additionally owns a per-step ephemeral message list that is never merged back into orchestrator state (`lib/crewai/src/crewai/agents/step_executor.py:66-68,244-258`). None of these objects are part of the public task/crew API surface.

**2. Is it durable?**
Working state itself is **not durable by default**: each `invoke()` starts by wiping all of it (`lib/crewai/src/crewai/experimental/agent_executor.py:2830-2845`). Durability exists at adjacent tiers: the executor subclasses `Flow[AgentExecutorState]` (`lib/crewai/src/crewai/experimental/agent_executor.py:173-186`), so Flow-level checkpointing with JSON/SQLite providers can persist state snapshots (`lib/crewai/src/crewai/state/provider/sqlite_provider.py:126-159`); task outputs used for replay are persisted in SQLite (`lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:46-58`); and *distilled* memories become durable in LanceDB/Qdrant-backed unified memory with a drain barrier against lost writes (`lib/crewai/src/crewai/memory/unified_memory.py:350-370`). Raw scratch notes are intentionally ephemeral.

**3. Is it exposed to users?**
Partially, by design. The sanitized final conversation is copied to `agent._last_messages` and embedded in every `TaskOutput` (`lib/crewai/src/crewai/agent/utils.py:247-283`; `lib/crewai/src/crewai/task.py:720-731`), so users can inspect the full thought/tool trace after a task. Live internals are observable via the event bus (`PlanStepStarted/Completed`, `StepObservation*`, reasoning events; emitters at `lib/crewai/src/crewai/experimental/agent_executor.py:409-449` and `lib/crewai/src/crewai/agents/planner_observer.py:137-213`). However, the `execution_log` audit trail lives only in in-memory state and has no export/persistence API found.

**4. Does it pollute long-term memory?**
Guarded, with one historical regression. Writes to long-term memory pass through LLM distillation of the task/result summary rather than dumping raw scratch content (`lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:43-63`), delegation outputs are excluded (`base_agent_executor.py:40-41`), and writes land under namespaced root scopes (`lib/crewai/src/crewai/crew.py:653-688`). But the legacy planning path appends plan text directly onto the shared `task.description` (`lib/crewai/src/crewai/agent/utils.py:55`) — exactly what the new executor's comment forbids (`lib/crewai/src/crewai/experimental/agent_executor.py:382-384`) — so plan notes can leak into persisted task definitions on the legacy path.

**5. Can it be audited?**
Largely yes. Three channels exist: (a) the structured `execution_log` and `observations` dict inside state (`lib/crewai/src/crewai/experimental/agent_executor.py:157-170,678-690`); (b) typed event-bus events for planning, step transitions, and memory saves/failures (`lib/crewai/src/crewai/events/types/reasoning_events.py` consumers; memory events emitted at `lib/crewai/src/crewai/memory/unified_memory.py:474-520,793-816`); (c) post-hoc `TaskOutput.messages`. Gaps: no built-in sink/export for `execution_log`, and event payloads embed unredacted content (see Failure Modes).

## Architectural Decisions

- **Typed state over ad-hoc dicts**: all working memory is a validated pydantic `BaseModel` (`AgentExecutorState`), making it serializable and checkpointable via the Flow machinery it inherits (`lib/crewai/src/crewai/experimental/agent_executor.py:135-186`).
- **Plan-and-Act separation**: planner outputs and runtime observations are first-class state fields, while the step worker receives an immutable, results-only context — an explicit anti-pollution contract documented in the type module (`lib/crewai/src/crewai/utilities/step_execution_context.py:1-19`).
- **Distill-before-persist**: only LLM-extracted discrete memories reach long-term storage, decoupling ephemeral scratch from durable knowledge (`lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:42-63`; `lib/crewai/src/crewai/memory/unified_memory.py:667-679`).
- **Ephemeral-by-default working state**: full reset on every invocation makes executions repeatable and prevents cross-task contamination (`lib/crewai/src/crewai/experimental/agent_executor.py:2831-2845`).
- **Dual-generation executors**: the state-machine `AgentExecutor` is the default (`lib/crewai/src/crewai/agent/core.py:345-352`), while the legacy plain-`messages`-list `CrewAgentExecutor` remains opt-in — a transitional architecture evident from `executor_type: Literal["experimental"]` (`lib/crewai/src/crewai/experimental/agent_executor.py:190`) versus `lib/crewai/src/crewai/agents/crew_agent_executor.py`.

## Notable Patterns

- **Status-driven todo lifecycle**: `TodoList.current_todo/next_pending/get_ready_todos` plus `mark_*` mutators implement a small state machine with dependency satisfaction that tolerates failed dependencies instead of deadlocking downstream steps (`lib/crewai/src/crewai/utilities/planning_types.py:50-145`).
- **Replan-preserving replacement**: `replace_pending_todos` swaps only pending items, preserving completed/failed history during replanning (`lib/crewai/src/crewai/utilities/planning_types.py:185-195`).
- **Read barrier for async scratch writes**: memory saves run on a single-worker background pool; any `recall()` drains pending writes first, giving read-your-writes semantics without blocking the agent loop (`lib/crewai/src/crewai/memory/unified_memory.py:297-364,711-713`).
- **Graceful degradation of observation**: if the observation LLM call fails, a conservative default (assume success, keep plan) avoids stalling execution (`lib/crewai/src/crewai/agents/planner_observer.py:191-213`); heuristic zero-LLM observation is used at low effort (`planner_observer.py:87-111`).
- **In-place compaction preserving system prompts and files**: summarization splits at message boundaries, preserves `system` role messages and re-attaches file attachments to the summary message (`lib/crewai/src/crewai/utilities/agent_utils.py:1069-1131`).

## Tradeoffs

- **Fragmentation vs. flexibility**: splitting working memory across messages/plan/todos/observations/log gives each concern precise semantics, but there is no single interface to inspect or snapshot "what the agent currently believes"; consumers must know four-plus fields.
- **Ephemerality vs. resumability**: wiping all working state per invoke guarantees cleanliness but means a mid-plan crash cannot resume the todo list from process state alone (recovery depends on Flow checkpoints being enabled and task-output replay storage).
- **Observability vs. privacy**: emitting raw content on memory-save events and copying the entire transcript into `TaskOutput` maximizes debuggability but widens the sensitive-data surface (see below).
- **LLM-mediated memory writes vs. cost/latency**: distillation adds an LLM call per task completion (`lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:49`) in exchange for cleaner long-term memory.

## Failure Modes / Edge Cases

- **Legacy description mutation**: on the opt-in legacy executor path (`handle_reasoning`, noted in-code as "used by CrewAgentExecutor (legacy path)" at `lib/crewai/src/crewai/agent/utils.py:35`), repeated invocations accumulate `Planning:` blocks in the shared `task.description`, contaminating prompts and any persisted task definitions (`lib/crewai/src/crewai/agent/utils.py:55`).
- **Raw-content leakage through telemetry/events**: `MemorySaveStartedEvent(value=content)` and save-completed events carry unredacted memory content onto the bus (`lib/crewai/src/crewai/memory/unified_memory.py:474-509`); anything subscribed (telemetry, console formatter) sees scratch-derived content.
- **Full transcript in outputs**: `TaskOutput.messages` includes all tool results verbatim (`lib/crewai/src/crewai/task.py:729`) — sensitive tool payloads become user-visible artifacts.
- **Summarization lossiness**: context compaction replaces history with an LLM summary; facts not captured in `<summary>` tags are permanently dropped from working memory (`lib/crewai/src/crewai/utilities/agent_utils.py:1086-1131`), and the operation triggers only reactively on provider errors.
- **Hard exit on context overflow when uncompacting**: with `respect_context_window=False`, overflow raises `SystemExit`, killing the whole process rather than failing the task (`lib/crewai/src/crewai/utilities/agent_utils.py:824-832`).
- **Background save loss at shutdown**: if the process exits mid-save, encoding is silently abandoned when the executor pool is closed ("cannot schedule new futures" branch) — accepted data-loss window (`lib/crewai/src/crewai/memory/unified_memory.py:641-650`).
- **Audit trail non-durability**: `execution_log` exists only in memory and is wiped per invoke, so post-mortems rely on external event listeners having been attached.

## Future Considerations

- Introduce a single named working-memory façade (e.g., expose `state.plan/todos/observations` through one accessor) to unify the legacy and experimental executors and give users one audit point.
- Retire or gate `handle_reasoning`'s description mutation behind an opt-in flag, aligning the legacy path with the no-pollution rule already documented in the new executor.
- Add redaction hooks for event payloads (`MemorySaveStartedEvent`, `TaskOutput.messages`) so scratch content can be observed without leaking secrets.
- Persist or stream `execution_log` to the existing checkpoint/event infrastructure to make audits durable beyond a single invocation.
- Make context compaction proactive (token-budget based) rather than purely reactive to provider overflow errors.

## Questions / Gaps

- **Is `AgentExecutorState` ever actually checkpointed in production flows?** The class inherits Flow persistence capability, but no test or wiring was found that snapshots the executor state mid-run; only Flow-level persistence tests exist (`lib/crewai/tests/test_flow_persistence.py`). Searched: `tests/**` for `AgentExecutorState` + checkpoint references.
- **Which executor ships by default?** The state-machine `AgentExecutor` is the default: `Agent.executor_class` defaults to `AgentExecutor`, with `CrewAgentExecutor` available as an opt-in legacy alternative (`lib/crewai/src/crewai/agent/core.py:342-352`, instantiation at `lib/crewai/src/crewai/agent/core.py:1145-1168`). The description-mutating planning path (`handle_reasoning`) is therefore only active when users explicitly select the legacy executor.
- **No evidence found** for retention/TTL policies over the `latest_kickoff_task_outputs.db` replay store beyond manual `delete_all()`/`reset_memories('kickoff_outputs')`.
- **No clear evidence found** for redaction or PII filtering anywhere between scratch content and the event bus/output transcript; searches for masking/redaction terms in `memory/`, `events/`, and `utilities/` returned nothing relevant.

---

Generated by `05.02-working-memory-and-scratchpad` against `crewai`.
