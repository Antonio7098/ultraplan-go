# Source Analysis: crewai

## Human-in-the-Loop Trigger Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based framework; `lib/crewai/src/crewai`) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI implements human-in-the-loop (HITL) through **four developer-configured trigger mechanisms**, none of which fire automatically from risk, uncertainty, budget, or failed validation:

1. **Task-level review flag**: `human_input: bool` on every Task (`lib/crewai/src/crewai/task.py:233-236`). When set, the agent executor pauses after the agent's *final answer* and enters an interactive terminal feedback loop (`lib/crewai/src/crewai/agents/crew_agent_executor.py:227,243-244`; provider loop in `lib/crewai/src/crewai/core/providers/human_input.py:237-264`).
2. **Training mode**: `Crew._setup_for_training` force-enables `task.human_input = True` on all tasks and disables delegation (`lib/crewai/src/crewai/crew.py:927-932`), persisting each round of feedback as training data (`lib/crewai/src/crewai/agents/crew_agent_executor.py:1539-1586`).
3. **Flow-level `@human_feedback` decorator**: a metadata-stamping decorator (`lib/crewai/src/crewai/flow/dsl/_human_feedback.py:23-57`) whose config the Flow engine executes after the decorated method completes (`lib/crewai/src/crewai/flow/runtime/__init__.py:2897-2901`, collection pipeline at `3518-3604`). This is the richest mechanism: optional LLM collapse of free-form feedback into discrete routing outcomes, custom providers, non-blocking pause/persist/resume for async approval, and an optional "learning" loop that distills lessons from feedback into memory.
4. **Hook-context explicit requests**: `ToolCallContext.request_human_input()` (`lib/crewai/src/crewai/hooks/tool_hooks.py:86-128`) and `LLMCallHookContext.request_human_input()` (`lib/crewai/src/crewai/hooks/llm_hooks.py:114-155`) let developers build arbitrary approval gates — e.g., a `@before_tool_call(tools=["delete_file"])` hook that prompts for approval and returns `False` to block execution (`lib/crewai/src/crewai/hooks/decorators.py:230-235`; block semantics at `lib/crewai/src/crewai/hooks/tool_hooks.py:142-150,173-190`).

Additionally, Flows expose `ask()` for mid-method user input with optional timeout and provider routing (`lib/crewai/src/crewai/flow/runtime/__init__.py:3422-3516`).

Trigger decisions are auditable on the Flow path via typed events, telemetry spans, checkpointable event types, and durable pending-feedback persistence; on the Crew/Task path auditing is much thinner (console output plus telemetry booleans). The LLM itself can never initiate a request for human help — `AskQuestionTool` targets coworker agents only (`lib/crewai/src/crewai/tools/agent_tools/ask_question_tool.py:14-28`).

## Rating

**7 / 10.** The HITL model is clear, explicitly interfaced (two `runtime_checkable` Protocols), well tested, and operationally hardened where it matters most — async flows persist paused state automatically and resume across processes (`lib/crewai/tests/test_async_human_feedback.py:936-1036`). What keeps it out of 8+: triggers are purely static declarations with no policy layer combining conditions or reacting to risk/uncertainty/budget; guardrail validation failure retries then raises without any human escalation option (`lib/crewai/src/crewai/task.py:1337-1400`); and the task-level mechanism emits no typed audit events.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trigger: task review flag | `human_input: bool \| None = Field(description="Whether the task should have a human review the final answer...", default=False)` | `lib/crewai/src/crewai/task.py:233-236` |
| Flag plumbing (sync) | `invoke()` passes `"ask_for_human_input": task.human_input` to executor | `lib/crewai/src/crewai/agent/core.py:941-948` |
| Flag plumbing (async) | Same key in `aexecute_task` | `lib/crewai/src/crewai/agent/core.py:1064-1071` |
| Trigger evaluation point | Executor reads flag from inputs after loop completes, then `_handle_human_feedback` | `lib/crewai/src/crewai/agents/crew_agent_executor.py:227,243-244` |
| Trigger evaluation (experimental executor) | `state.ask_for_human_input = bool(inputs.get(...))` then feedback handling post-`AgentFinish` | `lib/crewai/src/crewai/experimental/agent_executor.py:2858-2873,2944-2961` |
| Feedback iteration loop | Empty input ends review; text appends feedback message and re-invokes loop until satisfied | `lib/crewai/src/crewai/core/providers/human_input.py:256-264,308-316` |
| Provider protocol | `HumanInputProvider` Protocol with `setup_messages`/`handle_feedback`/`handle_feedback_async` | `lib/crewai/src/crewai/core/providers/human_input.py:59-130` |
| Provider injection | ContextVar-backed `get_provider`/`set_provider`/`reset_provider` | `lib/crewai/src/crewai/core/providers/human_input.py:445-474` |
| Training-mode trigger | `_setup_for_training` sets `task.human_input = True` for all tasks | `lib/crewai/src/crewai/crew.py:927-932` |
| Training data recording | `_handle_crew_training_output` writes `{initial_output, human_feedback}` per agent/iteration to JSON file | `lib/crewai/src/crewai/agents/crew_agent_executor.py:1561-1586` |
| Flow trigger declaration | `human_feedback(message=..., emit=..., llm=..., default_outcome=..., provider=..., learn=...)` stamps `__human_feedback_config__` | `lib/crewai/src/crewai/flow/dsl/_human_feedback.py:23-57` |
| Decorator config validation | `emit` requires `llm`; `default_outcome` must be in `emit` | `lib/crewai/src/crewai/flow/human_feedback.py:206-225` |
| Engine trigger point | After method returns, if definition has `human_feedback`, run `_run_human_feedback_step` | `lib/crewai/src/crewai/flow/runtime/__init__.py:2897-2901` |
| Collection pipeline | Provider resolution → `request_feedback` → `_finalize_human_feedback` → lesson distillation | `lib/crewai/src/crewai/flow/runtime/__init__.py:3518-3604` |
| Outcome collapsing | Free-form feedback collapsed to one of `emit` outcomes using structured outputs (`Literal[outcomes_tuple]`), with string-match fallbacks | `lib/crewai/src/crewai/flow/runtime/__init__.py:3739-3844` |
| Async pause signal | `HumanFeedbackPending` exception returned to caller (not raised); auto-persists state | `lib/crewai/src/crewai/flow/async_feedback/types.py:148-218` |
| Pending context serialization | `PendingFeedbackContext` with flow_id, method_name, emit, llm, requested_at, execution_uuid + `to_dict/from_dict` | `lib/crewai/src/crewai/flow/async_feedback/types.py:18-145` |
| Auto-persist on pause | Kickoff catches pending, creates default persistence, saves context + state, emits `FlowPausedEvent`, returns exception object | `lib/crewai/src/crewai/flow/runtime/__init__.py:2355-2406` (loop case: `1524-1557`) |
| Resume API | `from_pending()` loads persisted context; `resume(feedback)`/`resume_async(feedback)` finalize and route | `lib/crewai/src/crewai/flow/runtime/__init__.py:1242-1266,1285-1373` |
| Persistence backends | `save_pending_feedback`/`load_pending_feedback`/`clear_pending_feedback` on SQLite and base classes | `lib/crewai/src/crewai/flow/persistence/sqlite.py:205,246`; `lib/crewai/src/crewai/flow/persistence/base.py:70,90` |
| Hook approval gate | `ToolCallHookContext.request_human_input(prompt, default_message)` pauses live updates, blocks on `input()` | `lib/crewai/src/crewai/hooks/tool_hooks.py:86-128` |
| Hook block semantics | `before_tool_call_reducer` maps `False`/`HookAborted` to blocked tool call | `lib/crewai/src/crewai/hooks/tool_hooks.py:142-150,173-190` |
| Filtered hook registration | `@before_tool_call(tools=["delete_file","execute_code"], agents=[...])` with approve-prompt example | `lib/crewai/src/crewai/hooks/decorators.py:208-249` |
| Mid-flow user input | `ask()` emits `FlowInputRequestedEvent`, supports timeout + custom input providers, appends to `_input_history` | `lib/crewai/src/crewai/flow/runtime/__init__.py:3427-3516` |
| Audit: flow events | `HumanFeedbackRequestedEvent` / `HumanFeedbackReceivedEvent` emitted around prompt | `lib/crewai/src/crewai/events/types/flow_events.py:244-287`; emission at `lib/crewai/src/crewai/flow/runtime/__init__.py:3695-3705,3724+` |
| Audit: telemetry spans | Listener converts events to `human_feedback_span(event_type, has_routing, num_outcomes, feedback_provided, outcome)` | `lib/crewai/src/crewai/events/event_listener.py:505-529`; `lib/crewai/src/crewai/telemetry/telemetry.py:1218-1249` |
| Audit: checkpoints | `human_feedback_requested`, `human_feedback_received`, `flow_paused` are valid checkpoint trigger events | `lib/crewai/src/crewai/state/checkpoint_config.py:42,47-49` |
| Audit: task telemetry only | Task-level path records just `"human_input?": task.human_input` attribute on spans | `lib/crewai/src/crewai/telemetry/telemetry.py:411,494,908` |
| Guardrails do NOT escalate | Failed guardrail retries up to `guardrail_max_retries`, then raises validation error | `lib/crewai/src/crewai/task.py:1337-1400` |
| Agent cannot ask humans | `AskQuestionTool` routes questions to coworker agents, not users | `lib/crewai/src/crewai/tools/agent_tools/ask_question_tool.py:14-28` |
| Tests: executor trigger state | `test_ask_for_human_input_state_roundtrip`, provider hook contract tests | `lib/crewai/tests/agents/test_agent_executor.py:95,117-123,132-163` |
| Tests: task flag propagation | `invoke` called with `"ask_for_human_input": True` asserted twice across retries | `lib/crewai/tests/agents/test_agent.py:1292-1346` |
| Tests: YAML config | `"human_input": True` loaded from crew YAML into `task.human_input` | `lib/crewai/tests/project/test_crew_loader.py:429,445` |
| Tests: hook prompts | `TestLLMHookHumanInput`/`TestToolHookHumanInput`: response return, Enter→empty, whitespace strip, pause/resume, KeyboardInterrupt | `lib/crewai/tests/hooks/test_human_approval.py:50-230` |
| Tests: async pause/resume | kickoff returns `HumanFeedbackPending`; persistence auto-created; resume without feedback uses default outcome | `lib/crewai/tests/test_async_human_feedback.py:150-220,936-991,1010-1036,1342` |
| Docs (design intent) | `docs/edge/en/learn/human-feedback-in-flows.mdx`, `docs/edge/en/learn/human-in-the-loop.mdx`, `docs/edge/en/learn/human-input-on-execution.mdx` document the implemented decorator/flag/provider surfaces | `docs/edge/en/learn/human-feedback-in-flows.mdx:14-77` |

## Answers to Dimension Questions

### 1. What triggers human review?

Only developer-declared, statically configured triggers:

- **Post-answer review (Crews/Tasks)**: `task.human_input=True` causes the executor, after the agent produces its final `AgentFinish`, to display the result and enter an interactive feedback loop (`lib/crewai/src/crewai/task.py:233-236`; `lib/crewai/src/crewai/agents/crew_agent_executor.py:227,243-244`; loop at `lib/crewai/src/crewai/core/providers/human_input.py:237-264`). Pressing Enter accepts the answer; typing text injects the feedback as a message and re-runs the agent, repeating until Enter.
- **Training mode**: `crew.train()` calls `_setup_for_training`, which flips `task.human_input = True` on every task and disables delegation (`lib/crewai/src/crewai/crew.py:927-932`), collecting one structured feedback/improvement pair per iteration (`lib/crewai/src/crewai/agents/crew_agent_executor.py:215-235`).
- **Post-method review (Flows)**: `@human_feedback` decorated methods trigger collection after the method returns, driven by the flow definition rather than user code (`lib/crewai/src/crewai/flow/runtime/__init__.py:2897-2901`).
- **Programmatic gates**: hooks calling `context.request_human_input()` at pre/post model-call or tool-call interception points (`lib/crewai/src/crewai/hooks/dispatch.py:40-62`), typically returning `False` to veto a dangerous tool call.
- **Mid-flow input requests**: `self.ask(...)` inside flow methods.

Notably absent: there are **no automatic triggers** based on tool risk scores, output uncertainty, token/cost budget exhaustion, or validation failure. Guardrail failure exhausts `guardrail_max_retries` and raises `TaskFailedValidationError` — it never escalates to a human (`lib/crewai/src/crewai/task.py:1376-1384`).

### 2. Are triggers configurable?

Yes, at several granularities:

- Per-task boolean, settable in code or YAML config (`lib/crewai/tests/project/test_crew_loader.py:429,445`).
- Per-flow-method decorator parameters: `message`, `emit` (routing outcomes), `llm` (collapse model), `default_outcome`, `metadata`, `provider`, `learn`/`learn_source`/`learn_strict` (`lib/crewai/src/crewai/flow/dsl/_human_feedback.py:23-33`), validated at decoration time (`lib/crewai/src/crewai/flow/human_feedback.py:206-225`).
- Swappable collection strategy at three scopes: process/context scope via `set_provider()` ContextVar (`lib/crewai/src/crewai/core/providers/human_input.py:465-474`), global default via `flow_config.hitl_provider` (`lib/crewai/src/crewai/flow/flow_config.py:35-40`), and per-decorator `provider=` override resolved per invocation (`lib/crewai/src/crewai/flow/runtime/__init__.py:3652-3668`).
- Hooks can be filtered by tool name and/or agent role when registered (`lib/crewai/src/crewai/hooks/decorators.py:230-241`).
- Visualization/SDK consumers can hide HITL fields entirely via the `"hitl"` skip flag (`lib/crewai/src/crewai/flow/skill.py:427`).

However, configuration selects *where* review happens — it does not express *conditions* (no threshold, risk, or budget predicates exist anywhere in the trigger layer; searched `requires_approval`, `needs_review`, `escalat`, `risky`, `dangerous` — matches were only docstring examples and an MCP tool filter comment at `lib/crewai/src/crewai/mcp/filters.py:149`).

### 3. Can users request human review?

- **Developers** request it declaratively (flag/decorator) or imperatively (hooks, `ask()`) — this is the only way review points come into existence.
- **Agents cannot** spontaneously summon a human. The closest tool, `AskQuestionTool`, asks coworker *agents* (`"Tool for asking questions to coworkers"`, `lib/crewai/src/crewai/tools/agent_tools/ask_question_tool.py:14-17`); no ask-the-human tool exists.
- **End users** interact by responding to the prompts: empty input signals approval/skip in both the task loop (`lib/crewai/src/crewai/core/providers/human_input.py:257-258`) and flow collection (Enter-to-skip, `lib/crewai/src/crewai/flow/runtime/__init__.py:3718-3722`); non-empty input becomes iterative refinement rounds (task path) or free-form feedback collapsed by an LLM to a routing outcome (flow path).

### 4. Are trigger decisions auditable?

Split across mechanisms:

- **Flow HITL — yes, thoroughly**: typed bus events `HumanFeedbackRequestedEvent`/`HumanFeedbackReceivedEvent` carry method name, output shown, message, emit options, feedback text, outcome, and a correlating `request_id` (`lib/crewai/src/crewai/events/types/flow_events.py:244-287`), emitted around the console prompt (`lib/crewai/src/crewai/flow/runtime/__init__.py:3695-3705,3724-3735`). A listener turns them into telemetry spans (`lib/crewai/src/crewai/events/event_listener.py:505-529`; span attributes at `lib/crewai/src/crewai/telemetry/telemetry.py:1236-1247`). Both event types plus `flow_paused` are first-class checkpoint trigger types (`lib/crewai/src/crewai/state/checkpoint_config.py:42,47-49`), and paused contexts are durably persisted with timestamps and execution UUIDs (`lib/crewai/src/crewai/flow/async_feedback/types.py:59-115`; SQLite store at `lib/crewai/src/crewai/flow/persistence/sqlite.py:205-246`).
- **Task/Crew HITL — weakly**: the sync path records no dedicated events. Audit evidence is limited to terminal output, the training-data JSON file when training mode produced it (`lib/crewai/src/crewai/agents/crew_agent_executor.py:1561-1586`), and a static `"human_input?"` boolean attribute on task telemetry spans (`lib/crewai/src/crewai/telemetry/telemetry.py:411,494,908`). There is no record of *whether* feedback was given or what it was outside training mode.
- **Hook prompts — no built-in audit**: `request_human_input` returns a string to the hook; nothing logs the question or response (`lib/crewai/src/crewai/hooks/tool_hooks.py:116-128`).

## Architectural Decisions

1. **Declaration/orchestration split for flow HITL.** The `@human_feedback` decorator is "a pure metadata stamper"; the Flow engine collects feedback after the method completes, driven by the flow definition (`lib/crewai/src/crewai/flow/human_feedback.py:1-11`; stamper at `lib/crewai/src/crewai/flow/dsl/_human_feedback.py:34-38`). This makes HITL points visible to introspection/visualization (the `hitl` skip in `lib/crewai/src/crewai/flow/skill.py:427`) and lets persistence serialize the whole decision (LLM config included, `lib/crewai/src/crewai/flow/runtime/__init__.py:1451-1453`).
2. **Protocol-first extensibility.** Two `runtime_checkable` Protocols define the seams: `HumanInputProvider` for executor-level loops (`lib/crewai/src/crewai/core/providers/human_input.py:59-130`) and `HumanFeedbackProvider` for flow-level strategies (`lib/crewai/src/crewai/flow/async_feedback/types.py:222-298`), each with a concrete sync default (`SyncHumanInputProvider`; console provider resolution at `lib/crewai/src/crewai/flow/runtime/__init__.py:3658-3661`).
3. **Pause-as-value control flow.** `HumanFeedbackPending` subclasses `Exception` but is deliberately *returned* by `kickoff()` so callers handle the paused state without try/except ("Not an error, a control flow signal", `lib/crewai/src/crewai/flow/async_feedback/types.py:148-167`; implemented at `lib/crewai/src/crewai/flow/runtime/__init__.py:2404-2406`).
4. **LLM-mediated outcome collapsing.** Free-form human feedback is mapped onto a closed vocabulary via structured outputs (`Literal[outcomes_tuple]` Pydantic field, `lib/crewai/src/crewai/flow/runtime/__init__.py:3780-3799`) so a human decision becomes a routable event name that drives `@listen(...)` wiring (`lib/crewai/src/crewai/flow/runtime/__init__.py:1513-1523`).
5. **Framework-owned durability.** Providers only notify external systems and raise; the framework persists pending context + state automatically, even lazily creating a default persistence backend (`lib/crewai/src/crewai/flow/runtime/__init__.py:2357-2375`; contract documented at `lib/crewai/src/crewai/flow/async_feedback/types.py:169-172`).

## Notable Patterns

- **Enter-means-approve sentinel.** Empty feedback terminates the review loop on the task path (`lib/crewai/src/crewai/core/providers/human_input.py:257-258`) and selects `default_outcome` (else first `emit`) on the flow path (`lib/crewai/src/crewai/flow/runtime/__init__.py:3618-3622`).
- **Console-safe prompting.** Every prompt path pauses the rich live-updating formatter and resumes in `finally` (`lib/crewai/src/crewai/core/providers/human_input.py:333-334,368-369`; `lib/crewai/src/crewai/hooks/tool_hooks.py:116-128`).
- **Learning-from-feedback loop (opt-in).** With `learn=True`, past lessons are recalled to pre-review output before humans see it, and new feedback is distilled into reusable lessons stored in memory with source `"hitl"` (`lib/crewai/src/crewai/flow/human_feedback.py:180-181,247-359`), with `learn_strict` controlling whether pipeline failures propagate (`285-298`).
- **Event-loop hygiene.** Sync feedback providers run via `asyncio.to_thread` inside async kickoffs (`lib/crewai/src/crewai/flow/runtime/__init__.py:3560-3562`); async stdin reading falls back to a worker thread when pipe I/O is unsupported (`lib/crewai/src/crewai/core/providers/human_input.py:425-442`); `resume()` refuses to run under a live loop and redirects to `resume_async` (`lib/crewai/src/crewai/flow/runtime/__init__.py:1330-1334`).
- **Trace continuity across pauses.** `PendingFeedbackContext.execution_uuid` is captured at request time and restored on resume "so traces stay on the same run after HITL pause" (`lib/crewai/src/crewai/flow/async_feedback/types.py:40-43`; restore at `lib/crewai/src/crewai/flow/runtime/__init__.py:1359`).

## Tradeoffs

- **Predictable but blind triggering.** Static flags guarantee review happens where declared, but nothing observes runtime risk: an agent about to run a destructive tool gets no gate unless a developer wrote a hook, and a failing guardrail burns retries then crashes instead of consulting a human (`lib/crewai/src/crewai/task.py:1376-1384`).
- **Two parallel HITL stacks.** The Crew/Task mechanism (executor loop + provider ContextVar) and the Flow mechanism (decorator + definitions + persistence + events) share almost no code or audit surface. Behavior differs subtly — e.g., multi-round iteration exists only on the task path, while outcome routing and pause/resume exist only on the flow path — raising integration cost and inconsistent audit depth.
- **Blocking-by-default interactivity.** The default providers call `input()` directly (`lib/crewai/src/crewai/core/providers/human_input.py:364`; `lib/crewai/src/crewai/hooks/tool_hooks.py:121`), which wedges headless/server deployments. Remote approval channels (Slack/webhook) are achievable but only through the Flow provider abstraction; the Crew/Task path has no equivalent remote story beyond replacing the ContextVar provider.
- **Nondeterministic routing through an LLM judge.** Collapse quality depends on the chosen model; misreads silently reroute the workflow. Structured outputs plus exact/partial-match fallbacks mitigate this (`lib/crewai/src/crewai/flow/runtime/__init__.py:3801-3844`), but the ultimate fallback is `outcomes[0]` (`3637-3639,3812,3819`), which hard-codes "first option wins" semantics on ambiguity.
- **No timeout on the primary review loop.** `input()` on the task path waits forever; only the flow `ask()` supports timeouts (`lib/crewai/src/crewai/flow/runtime/__init__.py:3453-3470`).

## Failure Modes / Edge Cases

- **Guardrail exhaustion never reaches a human**: after `guardrail_max_retries`, the task raises with "Task failed ... validation after N retries" (`lib/crewai/src/crewai/task.py:1383-1384`) — automation halts without escalation unless the developer independently wires hooks.
- **Silent first-outcome routing**: if the collapse LLM is unset or unparseable, flows route to `default_outcome or emit[0]` (`lib/crewai/src/crewai/flow/runtime/__init__.py:3636-3639`) — an ambiguous "sort of approved" could take the `approved` branch merely by list order.
- **KeyboardInterrupt during prompts** is handled and surfaced as resume-of-updates with defined behavior in tests (`lib/crewai/tests/hooks/test_human_approval.py:204-218`), but an interrupted task-level loop leaves `ask_for_human_input` state per-execution (reset each invoke, `lib/crewai/src/crewai/experimental/agent_executor.py:2858`), so restarts begin fresh.
- **Resume misuse guards**: `from_pending` on an unknown id raises `ValueError` (`lib/crewai/src/crewai/flow/runtime/__init__.py:1248-1249`); `resume()` in an async context raises with guidance (`1330-1334`); missing pending context raises (`1354-1357`).
- **Concurrent executor reuse** is blocked by an execution lock (`lib/crewai/src/crewai/experimental/agent_executor.py:2821-2827`), preventing two threads from interleaving their feedback loops.
- **Cross-process resume fidelity** depends on serialized context: the LLM config is carried in full ("the single source for cross- and same-process resume", `lib/crewai/src/crewai/flow/runtime/__init__.py:1451-1453`); legacy rows lacking `execution_uuid` fall back to a fresh trace UUID (`lib/crewai/src/crewai/flow/async_feedback/types.py:40-43`).

## Future Considerations

- **Unify audit coverage**: emit `HumanFeedbackRequested/ReceivedEvent` (or equivalents) from the task-level `_handle_human_feedback` path so all HITL interactions get the same event/checkpoint/telemetry treatment the Flow path enjoys (`lib/crewai/src/crewai/events/types/flow_events.py:244-287` currently only fired at `lib/crewai/src/crewai/flow/runtime/__init__.py:3695-3705`).
- **Add conditional trigger policies**: allow predicate-style triggers (`when=lambda output, ctx: confidence < x` or tool-risk lookups) layered onto `human_input` / `@human_feedback`, closing the gap between "declared everywhere" and "needed nowhere."
- **Wire guardrail exhaustion to HITL**: offer an `on_guardrail_exhausted="escalate"` option instead of unconditional raise (`lib/crewai/src/crewai/task.py:1376-1384`).
- **Timeouts and idempotent prompts for the task loop**: port `ask()`'s timeout machinery (`lib/crewai/src/crewai/flow/runtime/__init__.py:3453-3470`) to the executor feedback loop.
- **Record hook prompt Q&A**: give `request_human_input` on hook contexts an opt-in audit sink.

## Questions / Gaps

- **No evidence found** for automatic HITL triggers based on tool risk classification, model uncertainty/confidence, budget thresholds, or repeated failures. Searched the selected source for `requires_approval`, `needs_review`, `escalat*`, `risky`, `dangerous`, `human_approval`, `ask_human`; the only matches were hook/docstring examples (`lib/crewai/src/crewai/hooks/tool_hooks.py:234-240`, `lib/crewai/src/crewai/hooks/decorators.py:233-235`) and a tool-filter comment (`lib/crewai/src/crewai/mcp/filters.py:149`).
- **No evidence found** that the LLM/agent can self-initiate human review; delegation tools target peer agents only (`lib/crewai/src/crewai/tools/agent_tools/agent_tools.py:31-35`).
- **Multiple trigger conditions do not combine** within one mechanism: `human_input` is a bare boolean with no composition operators, and there is no policy engine that AND/ORs conditions. Independent mechanisms can coexist in one run (e.g., a tool approval hook firing mid-task and a post-answer review), but their combination is emergent from placement, not expressed policy.
- Whether enterprise/platform deployments add richer trigger or audit behavior cannot be assessed from this OSS source; the `metadata` field on feedback configs hints at platform integration but contains only developer-supplied data here (`lib/crewai/src/crewai/flow/human_feedback.py:170`).

---

Generated by `dimensions/14.01-human-in-the-loop-trigger-policy.md` against `crewai`.
