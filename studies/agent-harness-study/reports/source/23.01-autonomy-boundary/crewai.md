# Source Analysis: crewai

## Dimension 23.01: Autonomy Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (monorepo; main package in `lib/crewai`, tools in `lib/crewai-tools`) |
| Analyzed | 2026-08-24 |

## Summary

crewAI's autonomy boundary is a **developer-configured, opt-in human review model** layered over an otherwise fully autonomous execution loop. There is no centralized policy engine or permission system that decides what agents may do autonomously; instead, the framework exposes several independent mechanisms, each of which a developer must wire up explicitly:

1. **Task-level final-answer review** — the `human_input` flag on `Task` (`lib/crewai/src/crewai/task.py:233-236`, default `False`). When enabled, after the agent produces its final answer, a human-input provider prompts the user in the terminal and can iterate with feedback until an empty Enter approves (`lib/crewai/src/crewai/core/providers/human_input.py:256-264`, `319-369`). The flag flows from task → executor inputs as `ask_for_human_input` (`lib/crewai/src/crewai/agent/core.py:941-947`) and is consumed post-answer (`lib/crewai/src/crewai/agents/crew_agent_executor.py:227,243-244`).
2. **Tool-call gating via hooks** — `before_tool_call` hooks may mutate tool input in place, block execution by returning `False` (mapped to `HookAborted`, `lib/crewai/src/crewai/hooks/tool_hooks.py:142-150`), and prompt for interactive approval via `ToolCallHookContext.request_human_input()` (`lib/crewai/src/crewai/hooks/tool_hooks.py:86-128`). Blocked calls never execute; the agent receives the string `"Tool execution blocked by hook. Tool: <name>"` (`lib/crewai/src/crewai/agents/crew_agent_executor.py:962-969`). The same contract exists for LLM calls (`lib/crewai/src/crewai/hooks/llm_hooks.py:114`, `172-176`). This is the only mechanism that gates *during* execution — but it contains no built-in policy: every gate is developer-written.
3. **Flow-level HITL** — the `@human_feedback` decorator stamps config onto flow methods (`lib/crewai/src/crewai/flow/dsl/_human_feedback.py:23-57`); the runtime runs the feedback step after the method completes (`lib/crewai/src/crewai/flow/runtime/__init__.py:2897-2901`) and can collapse free-text feedback into routing outcomes with an LLM (`emit`, `lib/crewai/src/crewai/flow/human_feedback.py:158-182`). Async providers raise `HumanFeedbackPending` to pause the flow with persisted state (`lib/crewai/src/crewai/flow/async_feedback/types.py:148-195`).
4. **Agent capability flags** — delegation is off by default and only adds delegate/ask tools when `allow_delegation=True` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297-300`; `lib/crewai/src/crewai/crew.py:1645-1660`); MCP servers support static/dynamic allow/block tool filters (`lib/crewai/src/crewai/mcp/filters.py:100-163`).
5. **Automated output guardrails** — function- or LLM-based validators with bounded retries (`guardrail_max_retries=3`, `lib/crewai/src/crewai/task.py:279-281`; `lib/crewai/src/crewai/utilities/guardrail.py:123-187`; `lib/crewai/src/crewai/tasks/llm_guardrail.py:49`). These are machine checks, not human gates.

The default posture is **fully autonomous**: every tool call executes without approval unless a hook blocks it, and final answers ship without review unless `human_input=True`. Training mode is the one place the framework itself flips autonomy off, forcing `human_input=True` on all tasks and disabling delegation (`lib/crewai/src/crewai/crew.py:927-938`). Webhook-based HITL ("Pending Human Input" state, resume endpoints) is documented but Enterprise-only — no OSS implementation was found. No escalation concept exists anywhere in the codebase (searched `escalat|Escalat`: zero matches).

## Rating

**6 / 10** — Present but inconsistent across layers. The building blocks are real, tested, and well documented (hook blocking semantics have dedicated tests at `lib/crewai/tests/hooks/test_human_approval.py:223-259`; scoped hooks at `lib/crewai/tests/hooks/test_crew_scoped_hooks.py:42-66`; task flag behavior at `lib/crewai/tests/agents/test_agent.py:753-806`), and interfaces are explicit protocols (`HumanInputProvider`, `HumanFeedbackProvider`). However: there is no unified autonomy/policy model — four mechanisms with different semantics (task flag = post-hoc review of the *final answer only*; hooks = pre-execution gating; flow decorator = step-level review; guardrails = automated) — defaults are fully autonomous, hook blocking degrades to a soft error string the agent can retry around, the OSS human-input path is terminal-stdin-bound (unusable headless), and `SecurityConfig` still lists auth/scoping/delegation tokens as TODOs (`lib/crewai/src/crewai/security/security_config.py:25-28`). It does not clearly "know when it is out of its depth": nothing escalates to a human automatically.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Task human-review flag | `human_input: bool \| None` field, default `False`, described as "Whether the task should have a human review the final answer" | `lib/crewai/src/crewai/task.py:233-236` |
| Flag propagation to executor | `"ask_for_human_input": task.human_input` passed in executor invoke inputs | `lib/crewai/src/crewai/agent/core.py:946` (async twin `:1069`) |
| Post-answer feedback loop | Executor reads `ask_for_human_input` from inputs, then `_handle_human_feedback` after final answer | `lib/crewai/src/crewai/agents/crew_agent_executor.py:227,243-244` (experimental executor `lib/crewai/src/crewai/experimental/agent_executor.py:2858-2873`) |
| Feedback iteration until approval | Loop appends feedback messages and re-invokes until empty input sets `ask_for_human_input=False` | `lib/crewai/src/crewai/core/providers/human_input.py:256-264` (sync), `:308-316` (async) |
| Terminal-based prompt UI | Rich panel + blocking `input()`; pauses/resumes live console updates | `lib/crewai/src/crewai/core/providers/human_input.py:318-369` |
| Swappable provider | `HumanInputProvider` Protocol + context-var `set_provider`/`get_provider` | `lib/crewai/src/crewai/core/providers/human_input.py:59-130,445-474` |
| Tool gating hook contract | Returning `False` raises `HookAborted`; input mutated in place | `lib/crewai/src/crewai/hooks/tool_hooks.py:142-150` |
| Hook dispatch + block result | `run_before_tool_call_hooks` returns True when blocked; blocked call yields soft error string to agent | `lib/crewai/src/crewai/hooks/tool_hooks.py:173-190`; `lib/crewai/src/crewai/agents/crew_agent_executor.py:962-969` (also `lib/crewai/src/crewai/experimental/agent_executor.py:2024`; `lib/crewai/src/crewai/utilities/agent_utils.py:1738`) |
| Interactive approval in hooks | `request_human_input(prompt)` on tool and LLM hook contexts | `lib/crewai/src/crewai/hooks/tool_hooks.py:86-128`; `lib/crewai/src/crewai/hooks/llm_hooks.py:114-135` |
| LLM call gating | Before-LLM-call hooks abort via `HookAborted` on `False` | `lib/crewai/src/crewai/hooks/llm_hooks.py:172-176`; dispatch in `lib/crewai/src/crewai/llms/base_llm.py:1017-1040` |
| Flow HITL decorator | Pure metadata stamper recording `HumanFeedbackConfig` | `lib/crewai/src/crewai/flow/dsl/_human_feedback.py:23-57` |
| Flow feedback step invocation | Runtime runs `_run_human_feedback_step` after method completes | `lib/crewai/src/crewai/flow/runtime/__init__.py:2897-2901` |
| Outcome routing config | `message`, `emit` outcomes, collapsing LLM, `default_outcome`, custom `provider` | `lib/crewai/src/crewai/flow/human_feedback.py:158-182` |
| Async pause signal | `HumanFeedbackPending` exception pauses flow, persists state, returned (not raised) to caller | `lib/crewai/src/crewai/flow/async_feedback/types.py:148-195` |
| Delegation autonomy flag | `allow_delegation` default `False`; delegation tools added only if enabled | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297-300`; `lib/crewai/src/crewai/crew.py:1645-1660` |
| Training mode flips autonomy | `_setup_for_training` forces `task.human_input = True` and `agent.allow_delegation = False` | `lib/crewai/src/crewai/crew.py:927-935` |
| Runaway bounds | `max_iter=25` default per agent; `max_rpm` rate limit | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:293-306` |
| Automated guardrails | Function/string guardrails, `guardrail_max_retries=3`, `process_guardrail` emits events | `lib/crewai/src/crewai/task.py:252-281`; `lib/crewai/src/crewai/utilities/guardrail.py:60-120,123-187` |
| LLM guardrail impl | `LLMGuardrail` validates output via agent's own LLM | `lib/crewai/src/crewai/tasks/llm_guardrail.py:39-98` |
| MCP tool filtering | Static allowed/blocked name lists; dynamic context-aware filter fn | `lib/crewai/src/crewai/mcp/filters.py:100-163` |
| Security module scope | `SecurityConfig` covers fingerprints only; auth/scoping/delegation tokens marked `*TODO*` | `lib/crewai/src/crewai/security/security_config.py:20-32` |
| Checkpoint regression vector | Callable guardrails dropped with warning during JSON checkpointing → restored runs skip validation | `lib/crewai/src/crewai/utilities/guardrail.py:12-31` |
| Tests: human approval hooks | Approval allows/blocks execution; KeyboardInterrupt restores console state | `lib/crewai/tests/hooks/test_human_approval.py:136-259` |
| Tests: task flag end-to-end | `test_agent_human_input` asserts provider wiring when `human_input=True` | `lib/crewai/tests/agents/test_agent.py:753-848` |
| Docs: task flag | "set the `human_input` flag … prompts the user before delivering its final answer" | `docs/edge/en/learn/human-input-on-execution.mdx:15-16`; attribute table `docs/edge/en/concepts/tasks.mdx:57` |
| Docs: HITL approaches | Flow decorator vs Enterprise webhook table; webhook/resume protocol OSS-absent | `docs/edge/en/learn/human-in-the-loop.mdx:12-21,33-115` |
| Docs: hook gating | Tool/LLM hook guides document `request_human_input` and blocking API | `docs/edge/en/learn/tool-hooks.mdx:76,157,264-265`; `docs/edge/en/learn/llm-hooks.mdx:69,146` |
| LiteAgent has no HITL | Searched `human` in `lite_agent.py`: zero matches | `lib/crewai/src/crewai/lite_agent.py` |

## Answers to Dimension Questions

**1. What determines agent autonomy?**
Developer configuration, not a policy engine. Four independent switches decide it: per-task `human_input` (`lib/crewai/src/crewai/task.py:233`), registered tool/LLM hooks (`lib/crewai/src/crewai/hooks/tool_hooks.py:208-242`), per-agent capability booleans like `allow_delegation` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:297`), and per-method `@human_feedback` decorators in flows (`lib/crewai/src/crewai/flow/dsl/_human_feedback.py:23`). Absent these, execution is fully autonomous: tools run unattended (`lib/crewai/src/crewai/agents/crew_agent_executor.py:970-997` executes immediately when not blocked), and final answers are delivered without review.

**2. Are autonomy levels configurable?**
Yes, but only as binary, per-artifact toggles — there is no graded autonomy level (no read-only/supervised/autonomous tiers). Granularity varies by mechanism: task-level (final answer), tool-call-level (hooks), step-level (flows), agent-capability-level (delegation). Extensibility points are genuine: swappable `HumanInputProvider` via context var (`lib/crewai/src/crewai/core/providers/human_input.py:445-474`), async `HumanFeedbackProvider` protocol for Slack/ticket systems (`lib/crewai/src/crewai/flow/async_feedback/types.py:221-269`), and crew-scoped hook methods on `@CrewBase` classes (`lib/crewai/tests/hooks/test_crew_scoped_hooks.py:42-66`).

**3. Are boundaries documented?**
Well documented per mechanism: `docs/edge/en/learn/human-input-on-execution.mdx:15` (flag semantics), `docs/edge/en/concepts/tasks.mdx:57,301-309` (attribute tables, guardrail types), `docs/edge/en/learn/tool-hooks.mdx:76,157` (approval-gate pattern), `docs/edge/en/learn/human-in-the-loop.mdx:12-21` (explicitly splits OSS flow-decorator approach from Enterprise webhooks). What is *not* documented is a unified autonomy model — which mechanism to choose for which risk class, and how they compose.

**4. Does the system respect autonomy boundaries?**
Mostly yes within each mechanism's narrow scope. Hook blocks deterministically prevent tool execution (`lib/crewai/src/crewai/agents/crew_agent_executor.py:962-969`; tests `lib/crewai/tests/hooks/test_human_approval.py:252-259`), and empty-Enter is the only way to exit the feedback loop (`lib/crewai/src/crewai/core/providers/human_input.py:256-264`). But respect is partial: (a) a block is communicated to the agent as an ordinary string result, so the agent may simply try another route — there is no hard stop or human notification; (b) callable guardrails are silently dropped on checkpoint round-trips (`lib/crewai/src/crewai/utilities/guardrail.py:24-30`), so a restored run loses its validation boundary; (c) nothing re-checks boundaries after replays or state restoration.

## Architectural Decisions

- **Opt-in HITL rather than deny-by-default**: all gates default off (`human_input=False` at `lib/crewai/src/crewai/task.py:235`, `allow_delegation=False` at `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:298`), placing the safety burden on application developers.
- **Review targets outputs, not actions, by default**: the flagship `human_input` feature reviews the agent's *final answer*, not intermediate tool calls (`lib/crewai/src/crewai/task.py:234` description; consumption at `lib/crewai/src/crewai/agents/crew_agent_executor.py:243-244`). Pre-action gating requires the separate hooks layer.
- **Hooks as the universal interception seam**: one ordered queue per interception point shared by legacy registrations and new `@on(InterceptionPoint.PRE_TOOL_CALL)` dialect (`lib/crewai/src/crewai/hooks/tool_hooks.py:131-139`), with `HookAborted` as the single abort contract (`lib/crewai/src/crewai/hooks/dispatch.py:64`).
- **Provider pattern for human channels**: terminal stdin in OSS (`SyncHumanInputProvider`), protocol-defined providers for external systems in flows (`lib/crewai/src/crewai/flow/async_feedback/types.py:222`); enterprise webhook HITL delegated to the paid product (`docs/edge/en/learn/human-in-the-loop.mdx:17`).
- **Training mode as a first-class autonomy override**: the framework itself disables autonomy (human reviews every task, delegation forbidden) when collecting training data (`lib/crewai/src/crewai/crew.py:927-935`).

## Notable Patterns

- **Feedback-as-conversation**: human feedback is appended as a message and the loop re-invokes, allowing multiple rounds until silent approval (`lib/crewai/src/crewai/core/providers/human_input.py:256-264`).
- **Outcome collapsing**: flows convert free-text human feedback into named routing outcomes using a small LLM, enabling deterministic branching on subjective review (`emit`, `default_outcome`; `lib/crewai/src/crewai/flow/human_feedback.py:158-182`).
- **Pause-and-persist control-flow signal**: `HumanFeedbackPending` converts a blocking human wait into a resumable, state-persisted pause returned from `kickoff()` (`lib/crewai/src/crewai/flow/async_feedback/types.py:148-195`).
- **Console-state hygiene around prompts**: every human prompt pauses live event-stream rendering and guarantees resume even on `KeyboardInterrupt` (`lib/crewai/src/crewai/hooks/tool_hooks.py:116-128`; test `lib/crewai/tests/hooks/test_human_approval.py:202-220`).

## Tradeoffs

- **Simplicity vs enforceability**: soft-blocking tools via a string result keeps the loop resilient, but means "blocked" is advisory — the agent sees the denial text and can plan around it; no audit trail or mandatory stop exists beyond the hook author's code.
- **Composability vs coherence**: four mechanisms cover different scopes (answer, action, step, capability) but have different semantics (post-hoc vs pre-hoc, blocking vs pausing); developers must reason about interactions themselves (e.g., a task with both `guardrails` and `human_input` runs machine validation then human review — ordering is implicit in `lib/crewai/src/crewai/task.py:713-750,869-906` and executor code).
- **OSS simplicity vs production needs**: terminal `input()` prompts (`lib/crewai/src/crewai/core/providers/human_input.py:364`) work for local dev but are dead-ends for headless/server deployments; the resumable path exists only in flows (`HumanFeedbackPending`) or Enterprise, not crews.
- **Flexibility vs safety defaults**: everything-autonomous-by-default maximizes ease of adoption and minimizes friction, at the cost of dangerous actions (file deletion, DB writes) executing unattended unless the developer anticipates them.

## Failure Modes / Edge Cases

- **Hook failure tolerance**: exceptions raised by non-aborting hooks are swallowed with a warning (verbose-gated), so a buggy approval hook can silently become an allow-all (`lib/crewai/src/crewai/hooks/tool_hooks.py:164-190`; dispatcher notes in `lib/crewai/src/crewai/hooks/dispatch.py:21`).
- **Checkpoint restore drops validation**: JSON checkpointing cannot serialize callable guardrails; restored checkpoints run without them, with only a warning at save time (`lib/crewai/src/crewai/utilities/guardrail.py:12-31`).
- **Unbounded human-feedback loops**: the regular feedback cycle iterates as long as the human types non-empty input — no cap (`lib/crewai/src/crewai/core/providers/human_input.py:256-264`), though bounded `guardrail_max_retries` and `max_iter=25` cap the automated paths (`lib/crewai/src/crewai/task.py:279-281`; `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:304-306`).
- **Stdin coupling**: sync prompts call blocking `input()`; in headless environments this stalls or errors, and the async fallback still reads stdin via `asyncio.to_thread` (`lib/crewai/src/crewai/core/providers/human_input.py:364,425-442`).
- **No escalation path**: repeated guardrail failures exhaust retries and surface errors; max iterations abort; nothing hands control to a human automatically (searched `escalat`: zero matches in `lib/crewai/src`).
- **LiteAgent bypass**: the lightweight agent path implements no HITL at all, so autonomy guarantees do not transfer across execution engines (zero matches in `lib/crewai/src/crewai/lite_agent.py`).

## Future Considerations

- Introduce a graded, declarative autonomy/policy layer (e.g., per-tool risk classes with deny/ask/allow semantics) so gating is configuration rather than hand-written hooks; `SecurityConfig` already reserves the namespace (`lib/crewai/src/crewai/security/security_config.py:25-28`).
- Make hook blocks observable and terminal: emit structured events on block, optionally notify humans instead of feeding the denial back into the same autonomous loop (`lib/crewai/src/crewai/agents/crew_agent_executor.py:965`).
- Ship a headless OSS implementation of the pending/resume HITL state machine that flows already demonstrate (`lib/crewai/src/crewai/flow/async_feedback/types.py:148-218`), closing the gap between the OSS console UX and the documented Enterprise webhook workflow.
- Preserve guardrails across checkpoint restore (e.g., registry of named callables) to eliminate the silent boundary loss at `lib/crewai/src/crewai/utilities/guardrail.py:24-30`.
- Add fail-closed semantics or health checks for approval hooks so a broken hook cannot degrade into allow-all (`lib/crewai/src/crewai/hooks/dispatch.py:21`).

## Questions / Gaps

- **No evidence found** for any built-in tool-risk classification, permission engine, or "dangerous action" registry — searched `permission|approval|allowlist|denylist|blocked_tools|dangerous` across `lib/crewai/src/crewai`; hits were docstrings/examples (`lib/crewai/src/crewai/hooks/tool_hooks.py:95,234`) and MCP filter docs (`lib/crewai/src/crewai/mcp/filters.py:149`).
- **No evidence found** for escalation logic of any kind (searched `escalat|Escalat`: zero matches in `lib/crewai/src`).
- **No evidence found** of an OSS implementation of `humanInputWebhook` / "Pending Human Input" resume endpoints; they appear only in Enterprise-facing docs (`docs/edge/en/learn/human-in-the-loop.mdx:33-115`), not in `lib/crewai/src`.
- Whether hierarchical-process manager agents impose additional gating on worker actions was not established beyond delegation-tool injection (`lib/crewai/src/crewai/crew.py:1651-1653`); no evidence of extra review steps was found.
- Composition/ordering guarantees among `guardrails`, `human_input`, and tool hooks in a single task execution are inferable from code paths but nowhere specified or tested as a combined scenario.

---

Generated by `23.01-autonomy-boundary` against `crewai`.
