# Source Analysis: crewai

## Dimension 14.03: Human Intervention and Takeover

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (Pydantic-based framework; Click CLI; asyncio + threads) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI implements human intervention as a first-class, layered subsystem rather than an ad-hoc afterthought. There are five distinct intervention surfaces:

1. **Task/crew-level final-answer review** (`human_input=True` on `Task`, `lib/crewai/src/crewai/task.py:233-236`): after the agent produces a final answer, a pluggable `HumanInputProvider` prompts the human, appends the feedback as an LLM message, and re-invokes the agent loop — multi-round correction without restarting the run (`lib/crewai/src/crewai/core/providers/human_input.py:237-264`).
2. **Flow-level `@human_feedback` decorator**: a pure metadata stamper (`lib/crewai/src/crewai/flow/dsl/_human_feedback.py:34-57`) whose feedback collection, LLM-based outcome collapsing, and routing are driven by the flow engine (`_run_human_feedback_step`, `lib/crewai/src/crewai/flow/runtime/__init__.py:3518-3604`). Supports synchronous blocking providers and asynchronous providers that pause the flow via the `HumanFeedbackPending` control-flow exception (`lib/crewai/src/crewai/flow/async_feedback/types.py:148-218`).
3. **Pause / persist / resume for async HITL**: when a provider raises `HumanFeedbackPending`, the engine persists state plus a `PendingFeedbackContext` to SQLite (`save_pending_feedback`, `lib/crewai/src/crewai/flow/persistence/sqlite.py:205-244`) and returns the pending signal to the caller; `Flow.from_pending()` + `resume()`/`resume_async()` restore and continue in a later process (`lib/crewai/src/crewai/flow/runtime/__init__.py:1199-1266` and `1285-1387`).
4. **Checkpoint fork/resume across Crew, Flow, and Agent**: event-driven automatic checkpointing with branch/parent lineage (`lib/crewai/src/crewai/state/checkpoint_listener.py:113-216`), `RuntimeState.fork()` creating named branches (`lib/crewai/src/crewai/state/runtime.py:352-389`), class-level `from_checkpoint()`/`fork()` on Crew (`lib/crewai/src/crewai/crew.py:429-476`), Flow (`lib/crewai/src/crewai/flow/runtime/__init__.py:599-680`), and Agent (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:425-466`), `from_checkpoint=` parameters on all kickoff methods (e.g., `lib/crewai/src/crewai/crew.py:996`, `1211`; `lib/crewai/src/crewai/agent/core.py:1597`, `1974`), and an operational CLI (`crewai checkpoint list/info/resume/diff` + TUI, `lib/cli/src/crewai_cli/cli.py:1266-1307`, `lib/cli/src/crewai_cli/checkpoint_cli.py:467-508`, `526+`).
5. **Approval-gate hooks at interception points**: before-tool-call hooks can block execution or prompt the human mid-run (`request_human_input`, `lib/crewai/src/crewai/hooks/tool_hooks.py:86-128`); before-LLM-call hooks have the same capability (`lib/crewai/src/crewai/hooks/llm_hooks.py:114-155`) and both hook contexts expose mutable tool inputs and conversation messages (`tool_hooks.py:39-49`, `llm_hooks.py:39-46`).

There is no "sandbox takeover" concept in the OS sense (no shell/container handoff); the closest equivalents are the interactive chat mode (`chat_loop`, `lib/crewai/src/crewai/utilities/crew_chat.py:189-257`, wired to `crewai chat` at `lib/cli/src/crewai_cli/cli.py:997-1006`) and resume/fork from checkpoints. Direct arbitrary state editing by humans is deliberately not exposed as an API; intervention is expressed through feedback text, approve/block decisions, and checkpoint restore/fork.

## Rating

**8 / 10.**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band):** every mechanism is a named protocol or decorator — `HumanInputProvider` (`core/providers/human_input.py:59-144`), `HumanFeedbackProvider` (`flow/async_feedback/types.py:221-298`), `@human_feedback` (`flow/dsl/_human_feedback.py:23-59`), `CheckpointConfig` (`state/checkpoint_config.py:160-212`) — each documented with docstrings and worked examples.
- **Tests:** substantial dedicated suites exist: `lib/crewai/tests/test_async_human_feedback.py` (≈1400 lines covering serialization round-trips, provider protocol compliance, pause/resume lifecycle, outcome collapsing fallbacks — e.g., `test_full_async_flow_cycle` at line 948, `test_resume_routing` at line 733), `test_checkpoint.py` (fork/branch tests at lines 285-308, lineage round-trips at lines 238-268), `test_flow_human_input_integration.py`, `test_human_feedback_decorator.py`, and `test_checkpoint_cli.py` (resume error paths at lines 364-401).
- **Operational safeguards:** pause persistence is automatic when providers raise `HumanFeedbackPending` (`async_feedback/types.py:169-172`; enforced in `flow/runtime/__init__.py:1524-1557`); checkpoints emit started/completed/failed/pruned events and log the exact resume command (`checkpoint_listener.py:171-189`); prune failures are logged but non-fatal (`checkpoint_listener.py:196-203`).
- **Not 9–10 because:** the default sync providers block on terminal `input()` (`human_input.py:364`, `runtime/__init__.py:3722`), which is fragile for server deployments unless operators know to install a custom provider via the ContextVar (`set_provider`, `human_input.py:465-474`) or `flow_config.hitl_provider` (`lib/crewai/src/crewai/flow/flow_config.py:21-42`); the HITL loop is duplicated across two executors (`crew_agent_executor.py` vs `experimental/agent_executor.py:2858-2873, 2944-2958`), a drift risk; crew-level feedback is not durably audited outside training mode; and pending-feedback rows have `created_at` but no expiry/TTL enforcement (`persistence/sqlite.py:229-244`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Task-level human review flag | `human_input: bool \| None = Field(description="Whether the task should have a human review the final answer...")` | `lib/crewai/src/crewai/task.py:233-236` |
| Executor triggers HITL after final answer | `self.ask_for_human_input = bool(inputs.get("ask_for_human_input", False))` then `_handle_human_feedback(formatted_answer)` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:227, 243-244` |
| Multi-round feedback loop (mid-flight correction) | while-loop appends feedback message and re-invokes `_invoke_loop()` until empty input | `lib/crewai/src/crewai/core/providers/human_input.py:256-264` |
| Pluggable provider registry | ContextVar-backed `get_provider`/`set_provider`/`reset_provider` | `lib/crewai/src/crewai/core/providers/human_input.py:445-483` |
| Provider protocol (sync + async) | `HumanInputProvider.handle_feedback` / `handle_feedback_async` abstract surface | `lib/crewai/src/crewai/core/providers/human_input.py:59-130` |
| Async executor parity | experimental executor reads `ask_for_human_input` from state and calls provider | `lib/crewai/src/crewai/experimental/agent_executor.py:2858-2873` |
| Flow `@human_feedback` decorator | metadata stamper storing `__human_feedback_config__` | `lib/crewai/src/crewai/flow/dsl/_human_feedback.py:34-57` |
| Flow feedback step (provider dispatch, LLM collapse) | `_run_human_feedback_step` resolves provider, requests feedback, finalizes outcome | `lib/crewai/src/crewai/flow/runtime/__init__.py:3518-3604` |
| Console feedback prompt with events | emits `HumanFeedbackRequestedEvent` before `input()` and `HumanFeedbackReceivedEvent` after | `lib/crewai/src/crewai/flow/runtime/__init__.py:3695-3733` |
| LLM outcome collapsing for routing | `_collapse_to_outcome` maps free text to `emit` options via structured outputs | `lib/crewai/src/crewai/flow/runtime/__init__.py:3739-3759` |
| Pause signal type | `HumanFeedbackPending(Exception)` returned (not raised) to caller of `kickoff()` | `lib/crewai/src/crewai/flow/async_feedback/types.py:148-195` |
| Resume context schema | `PendingFeedbackContext` with flow_id, method_name, method_output, message, emit, llm, requested_at, execution_uuid | `lib/crewai/src/crewai/flow/async_feedback/types.py:19-115` |
| Pending persistence (SQLite) | `save_pending_feedback` stores context_json + state_json; `load_pending_feedback` restores both | `lib/crewai/src/crewai/flow/persistence/sqlite.py:205-278` |
| Auto-persist on pause | engine saves pending feedback and emits `FlowPausedEvent` when `HumanFeedbackPending` bubbles | `lib/crewai/src/crewai/flow/runtime/__init__.py:1524-1557` |
| Restore paused flow | `from_pending(flow_id)` loads state + context; `pending_feedback` property exposes it | `lib/crewai/src/crewai/flow/runtime/__init__.py:1236-1283` |
| Resume APIs | `resume(feedback)` / `resume_async(feedback)` continue execution with the human's feedback | `lib/crewai/src/crewai/flow/runtime/__init__.py:1285-1387` |
| Feedback history kept in flow state | `human_feedback_history: list[HumanFeedbackResult]`, `last_human_feedback` fields | `lib/crewai/src/crewai/flow/runtime/__init__.py:580-581` |
| Feedback result audit record | `HumanFeedbackResult` includes output, feedback, outcome, timestamp, method_name, metadata | `lib/crewai/src/crewai/flow/human_feedback.py:118-155` |
| HITL learning loop | `learn=True` pre-reviews with distilled lessons and stores new lessons (`mem.remember_many(..., source="hitl")`) | `lib/crewai/src/crewai/flow/human_feedback.py:247-359` |
| Tool-call approval gate | hooks can `request_human_input` and return False to block tool execution | `lib/crewai/src/crewai/hooks/tool_hooks.py:86-149` |
| LLM-call approval gate + conversation editing | `LLMCallHookContext.messages` is mutable in-place; `request_human_input` available | `lib/crewai/src/crewai/hooks/llm_hooks.py:39-46, 114-155` |
| Checkpoint config model | `CheckpointConfig(location, on_events, provider, max_checkpoints, restore_from)`; JSON + SQLite providers | `lib/crewai/src/crewai/state/checkpoint_config.py:160-204` |
| Event-driven auto-checkpointing | listener writes checkpoint on configured events, records trigger/task/agent context | `lib/crewai/src/crewai/state/checkpoint_listener.py:113-244` |
| Fork semantics (branch lineage) | `RuntimeState.fork(branch)` sets new branch, emits fork events; auto-names `fork/{checkpoint}_{hex}` | `lib/crewai/src/crewai/state/runtime.py:352-389` |
| Restore from checkpoint | `RuntimeState.from_checkpoint` detects provider, validates JSON, chains parent id | `lib/crewai/src/crewai/state/runtime.py:392-442` |
| Crew restore + partial-resume bookkeeping | `Crew.from_checkpoint`; `_restore_runtime` replays event record to find in-flight tasks and marks executors `_resuming` | `lib/crewai/src/crewai/crew.py:429-524` |
| Kickoff-time restore parameter | `kickoff(..., from_checkpoint=...)` supported on Crew kickoff variants | `lib/crewai/src/crewai/crew.py:996, 1131, 1211` |
| Flow fork classmethod | `Flow.fork(config, branch)` restores then forks runtime state with new instance id | `lib/crewai/src/crewai/flow/runtime/__init__.py:649-680` |
| Agent fork | `BaseAgent.from_checkpoint` / `BaseAgent.fork` mirror Crew API | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:425-466` |
| CLI takeover tooling | `crewai checkpoint list/info/resume/diff` subcommands + TUI; `resume_checkpoint` rebuilds entity from metadata and kicks off | `lib/cli/src/crewai_cli/cli.py:1266-1307`; `lib/cli/src/crewai_cli/checkpoint_cli.py:467-508, 526` |
| Training-mode takeover | `_setup_for_training` forces `task.human_input = True` on all tasks and disables delegation | `lib/crewai/src/crewai/crew.py:927-938` |
| Training data capture of human edits | `_handle_crew_training_output` stores `{initial_output, human_feedback, improved_output}` per agent/iteration | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1539-1586` |
| Chat conversational takeover | `chat_loop` maintains persistent message list, user drives turns until "exit" | `lib/crewai/src/crewai/utilities/crew_chat.py:189-257` |
| Intervention telemetry | event listener registers handlers for `HumanFeedbackRequestedEvent`/`ReceivedEvent` → telemetry spans | `lib/crewai/src/crewai/events/event_listener.py:505-523` |
| Event timestamps for audit | `BaseEvent.timestamp: datetime` defaults to UTC now | `lib/crewai/src/crewai/events/base_events.py:69` |
| Checkpoint trigger vocabulary includes HITL events | `"human_feedback_requested"`, `"human_feedback_received"`, `"flow_paused"` are valid checkpoint triggers | `lib/crewai/src/crewai/state/checkpoint_config.py:47-50` |
| Test coverage — pause/resume | `test_full_async_flow_cycle`, `test_auto_persistence_when_none_provided`, `test_resume_without_pending_raises_error` | `lib/crewai/tests/test_async_human_feedback.py:948, 1009, 567` |
| Test coverage — fork/branch | `test_fork_sets_branch`, `test_fork_auto_branch`, `test_fork_no_checkpoint_id_unique`, branch-aware prune | `lib/crewai/tests/test_checkpoint.py:285-308, 388` |
| Test coverage — CLI resume | resume errors for missing/nonexistent checkpoints; hint text asserted | `lib/crewai/tests/test_checkpoint_cli.py:364-401` |
| Documented design intent | docs describe task-level human input and flow human feedback incl. async Slack-style providers | `docs/edge/en/learn/human-input-on-execution.mdx:15-16`; `docs/edge/en/learn/human-feedback-in-flows.mdx` |

## Answers to Dimension Questions

**1. Can humans edit agent state?**
Indirectly, yes; arbitrarily, no. Humans cannot write arbitrary flow/crew state through a public API. Their influence paths are: (a) feedback text that is appended into the executor's LLM message list and re-executed (`core/providers/human_input.py:260-261`); (b) hooks that programmatically mutate tool inputs in place (`hooks/tool_hooks.py:40-43`) and conversation messages between iterations (`hooks/llm_hooks.py:39-46`) — these are developer-written interventions, not end-user ones; (c) restoring a checkpoint and modifying serialized state offline before re-kickoff (the checkpoint payload is plain JSON via `JsonProvider` default, `state/checkpoint_config.py:167-183`). In Flows, human feedback outcomes are recorded into typed state (`human_feedback_history`, `flow/runtime/__init__.py:580`) which downstream methods can read via `self.human_feedback` (documented usage, `flow/human_feedback.py:139-147`). No evidence found of a general-purpose "edit any field of a running agent's state" endpoint; searches for direct state-mutation APIs beyond the above returned nothing.

**2. Can humans provide mid-run feedback?**
Yes, at four distinct points during a single run: after a task's final answer (`task.py:233` → `crew_agent_executor.py:243-244`), after any flow method marked `@human_feedback` (`dsl/_human_feedback.py` → `flow/runtime/__init__.py:3518`), before individual tool executions (`tool_hooks.py:86-149`), and before individual LLM calls (`llm_hooks.py:114-155`). The task-level loop explicitly supports multiple rounds of feedback until the human presses Enter empty-handed (`core/providers/human_input.py:349` prompt copy; loop at lines 256-264). For flows with async providers, feedback can also arrive *after* the process ends, via persisted pause + `resume(feedback)` (`flow/runtime/__init__.py:1285-1336`).

**3. Can humans take over execution?**
There is no sandbox/process-level takeover primitive. The practical equivalents: interactive chat mode where the human drives turn-by-turn (`utilities/crew_chat.py:189-257`); approval gates that let a human veto any tool call or LLM call (hook contexts returning `False`/raising `HookAborted`, `tool_hooks.py:142-150`); and full resume/fork from checkpoints, including from a different process via `crewai checkpoint resume <id>` (`checkpoint_cli.py:467-508`), which reconstructs the Crew/Flow/Agent and continues (`crew.py:478-524` even replays the event record to resume in-flight tasks correctly). Forking creates independent branches with lineage (`state/runtime.py:352-389`), so a human can explore alternate continuations without destroying the original run.

**4. Are human interventions traceable?**
Partially, and best on the Flow path. Every flow feedback request/receipt emits bus events carrying method name, output shown, message, and feedback text (`flow_events.py:244-287` definitions; emissions at `flow/runtime/__init__.py:3695-3733`), stamped with UTC timestamps (`base_events.py:69`) and mirrored to telemetry spans (`event_listener.py:505-523`). Feedback results accumulate in durable flow state with timestamps (`HumanFeedbackResult.timestamp`, `human_feedback.py:150-155`; state fields `flow/runtime/__init__.py:580-581`) and are therefore captured inside checkpoints. Pause points persist full `PendingFeedbackContext` including `requested_at` (`async_feedback/types.py:68, 113`). Checkpoint lineage records parent ids and branches (`checkpoint_listener.py:126-154`). Gaps: crew/task-level feedback lives only in the in-memory executor message list unless training mode captures it (`crew_agent_executor.py:1566-1570`) or checkpointing happens to fire; hook-mediated approvals have no dedicated audit event (only generic hook/abort events, `events/types/hook_events.py:19`); training data uses pickle files keyed by agent id with no tamper-evident trail (`training_handler.py:7-31`).

## Architectural Decisions

1. **Provider-protocol abstraction over hardcoded prompting.** Both HITL systems define `Protocol` interfaces (`core/providers/human_input.py:59-66`, `flow/async_feedback/types.py:221-272`) so terminal I/O, Slack bots, or webhook queues can be swapped without touching the engines. The sync/async split is explicit in the protocols (`AsyncExecutorContext`, `human_input.py:51-56`).
2. **Control-flow-via-exception for pausing.** `HumanFeedbackPending` is deliberately an exception subclass ("Not an error, a control flow signal", `async_feedback/types.py:148`) that the engine catches, persists state for, and converts into a *return value* so callers avoid try/except (`types.py:156-167`; engine handling `flow/runtime/__init__.py:1524-1557`).
3. **Metadata-stamper decorator + definition-driven engine.** `@human_feedback` only attaches config to the function (`dsl/_human_feedback.py:55-57`); the flow definition builder lifts it and the engine executes it. This keeps decorators serializable and lets declarative/DSL flows carry HITL config.
4. **Unified checkpoint layer under Crew/Flow/Agent.** A single `RuntimeState` graph with pluggable JSON/SQLite providers, event-triggered writes, branch labels, parent chaining, and pruning serves restore *and* fork for all three entity types (`state/checkpoint_config.py:160-234`, `state/runtime.py:280-497`).
5. **Lineage as part of the state model, not a side log.** `_branch` and `_parent_id` live on the serialized state and are chained on every write (`state/runtime.py:307`; `checkpoint_listener.py:126-154`), so forks are queryable and prunes are branch-aware (`tests/test_checkpoint.py:388`).
6. **Hooks as the policy layer.** Approval gating is delegated to user-space hooks at stable interception points with a shared dispatcher (`hooks/dispatch.py` used by `tool_hooks.py:173-205`), keeping the core loop free of policy logic.

## Notable Patterns

- **Pause → persist → resume as a reusable triad:** the same pattern appears for async HITL (SQLite pending table, `persistence/sqlite.py:205-296`) and for crash recovery (checkpoints + event-record replay, `crew.py:478-524`), giving one mental model for "stop now, continue later."
- **Console formatter coordination:** every human-prompt path pauses live console updates and resumes them in a `finally` block so prompts don't interleave with streaming output (`human_input.py:333-369`; `flow/runtime/__init__.py:3707-3737`; `tool_hooks.py:116-128`).
- **Outcome collapsing:** free-form human feedback is mapped onto a closed set of routing outcomes by an LLM with structured outputs and deterministic fallbacks (`_collapse_to_outcome`, `flow/runtime/__init__.py:3739+`; tested fallbacks in `test_async_human_feedback.py:1049-1127`).
- **HITL-to-memory distillation:** optional `learn=True` turns repeated human corrections into stored "lessons" recalled before future reviews (`human_feedback.py:247-359`) — feedback that compounds instead of being discarded.
- **Operational hints embedded in logs:** completed checkpoints log `Resume with: crewai checkpoint resume {id}` (`checkpoint_listener.py:172-175`), closing the loop between library behavior and CLI tooling.

## Tradeoffs

- **Ergonomics vs deployability:** the zero-config default (blocking `stdin` prompt, `human_input.py:364`) is ideal for local CLIs but unusable in servers/headless runs until an operator discovers `set_provider`/`hitl_provider`. The async-provider escape hatch exists (`async_feedback/providers.py`) but is opt-in per decorator or global config.
- **Simplicity vs safety for state edits:** exposing raw state editing would be dangerous; CrewAI channels human intent through narrow, validated surfaces (feedback strings, approve/block, checkpoint restore). The cost is that legitimate bulk corrections require code or offline JSON surgery.
- **Duplication for flexibility:** maintaining parallel sync/async implementations of every HITL path (e.g., `_handle_regular_feedback` vs `_handle_regular_feedback_async`, `human_input.py:237-316`; two full executors with the same flag plumbing, `crew_agent_executor.py:227-244` vs `experimental/agent_executor.py:2858-2873`) doubles the drift surface.
- **Durability vs overhead in checkpointing:** event-triggered checkpointing defaults to `task_completed` only (`checkpoint_config.py:172-176`); fine-grained recovery requires opting into more events (or `"*"`), trading I/O cost for granularity.

## Failure Modes / Edge Cases

- **Blocking stdin in non-TTY contexts:** `input()` raises EOFError/RuntimeError in headless environments if no provider override is installed; async stdin falls back to `asyncio.to_thread(input)` where pipes are unsupported (`human_input.py:441-442`).
- **Fork without prior checkpoint:** `fork()` works even with no checkpoint id by generating a random branch (`state/runtime.py:365-368`), but then the initial fork-point checkpoint cannot be written meaningfully — the API relies on `from_checkpoint` having succeeded first (`crew.py:469-474` raises a clear `RuntimeError` otherwise).
- **Restore mismatch risks:** `from_checkpoint` raises `ValueError` if the checkpoint contains no matching entity (`crew.py:451`; `flow/runtime/__init__.py:646`); definition-built flows need the `definition=` argument re-supplied since "checkpoints carry no callables" (`flow/runtime/__init__.py:610-613`).
- **Stale pending feedback:** rows in the `pending_feedback` table persist indefinitely with `created_at` recorded but no TTL/cleanup job (`sqlite.py:229-244`); resuming twice is guarded only by clearing after success (`sqlite.py:280-296`).
- **Silent degradation:** auto-checkpoint failures are swallowed to warnings to protect the run (`checkpoint_listener.py:241-244`), which favors availability over durability guarantees.
- **Training-data fragility:** training capture requires a valid `_train_iteration` int and existing iteration entry for the improved-output pass, else it warns and skips (`crew_agent_executor.py:1553-1583`); data format is pickle (not portable/tamper-safe).

## Future Considerations

- Consolidate the HITL loop shared by `CrewAgentExecutor` and `experimental/AgentExecutor` to eliminate duplicated flag plumbing (`ask_for_human_input` handled in three places: `crew_agent_executor.py:227/1105`, `experimental/agent_executor.py:2858/2944/3253`).
- Add TTL/expiry and idempotency keys to pending-feedback storage for webhook-driven resume at scale.
- Emit dedicated audit events for hook-mediated human approvals (currently only generic abort info exists, `events/types/hook_events.py:19`).
- Provide a first-class remote provider (HTTP queue) in-tree, since the protocol already anticipates Slack/webhook integrations (`async_feedback/types.py:225-271`) but ships only console defaults.
- Consider surfacing crew-level human feedback into durable state (like Flow's `human_feedback_history`) so traceability does not depend on training mode or checkpoint timing.

## Questions / Gaps

- **No evidence found** for any public API allowing a human to directly edit arbitrary conversation messages or flow state values mid-run; the search boundary was the full `lib/crewai/src` tree (patterns: `messages.append` outside internal flows, `edit_state`, `update_state`, `set_state`) — only hook-context mutation and provider feedback insertion exist.
- **No evidence found** for timeouts or deadline enforcement on human responses in either the sync prompt loops or async pending storage; a stalled reviewer blocks the run indefinitely in sync mode.
- The enterprise/plus tier references in docstrings ("metadata for enterprise integrations", `human_feedback.py:136-137`) suggest additional takeover features outside this OSS source; they could not be verified here.
- Whether `crewai checkpoint resume` handles version skew between the checkpointing library version and restored payload is only partially addressed (event-record migration warnings exist in `tests/test_checkpoint.py:212-235`), but no full compatibility matrix was found.

---

Generated by `dimensions/14.03-human-intervention-takeover.md` against `crewai`.
