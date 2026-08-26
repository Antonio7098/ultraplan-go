# Source Analysis: openai-agents-sdk

## Dimension 15.02: Message Routing and Termination

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, asyncio, pydantic (OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK implements multi-agent routing as **LLM-driven tool calling**, not a deterministic speaker-rotation algorithm. Each handoff is surfaced to the model as a function tool named `transfer_to_<agent_name>` (`src/agents/handoffs/__init__.py:207-211`), and the run loop treats the model's choice of that tool as a routing instruction. A central classifier, `process_model_response` (`src/agents/run_internal/turn_resolution.py:2684-3436`), maps every model output item into a typed plan (handoffs, functions, computer actions, shell calls, MCP approvals, custom tools), and `execute_tools_and_side_effects` (`src/agents/run_internal/turn_resolution.py:784-1098`) resolves each turn into one of four explicit next steps: `NextStepHandoff`, `NextStepFinalOutput`, `NextStepRunAgain`, or `NextStepInterruption` (`src/agents/run_internal/run_steps.py:155-181`). Termination is achieved when an agent produces a message with no pending tool work (or tool results are promoted directly to final output via `tool_use_behavior`); a hard ceiling of `DEFAULT_MAX_TURNS = 10` (`src/agents/run_config.py:45`) bounds any conversation, raisable `MaxTurnsExceeded` or convertible into a fallback output by an error handler. There is no handoff-graph cycle detector: ping-pong handoff loops are legal and are bounded only by `max_turns`, which can be explicitly disabled by passing `None`. Both streaming and non-streaming loops enforce identical turn accounting and termination semantics.

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 tier):** Routing is a typed state machine — every turn resolves to exactly one of four `NextStep` variants (`src/agents/run_internal/run_steps.py:155-181`), and handoff contracts are first-class dataclasses with documented fields (`src/agents/handoffs/__init__.py:126-201`).
- **Tests:** Extensive coverage exists for both routing and termination: 31 tests in `tests/test_handoff_tool.py` (setup, enablement filtering, name collisions, strict JSON input) and ~40 tests in `tests/test_max_turns.py` covering streamed/non-streamed parity, `max_turns=None`, error-handler conversion, session persistence of synthesized outputs, and guardrail interactions.
- **Operational safeguards:** max-turns enforcement with span error attachment (`src/agents/run.py:1466-1490`), duplicate call-ID reuse rejection (`src/agents/run_internal/turn_resolution.py:3439-3534`), tool-name collision policy (`src/agents/run_internal/turn_resolution.py:1946-1950`), and human-in-the-loop approval interruptions.
- **Why not 9–10:** No cycle/deadlock detection over the handoff graph exists anywhere in `src/`; termination semantics are implemented twice (non-streaming loop in `src/agents/run.py:965+` and streaming loop in `src/agents/run_internal/run_loop.py:1184+`) and must be kept aligned manually (a burden the repo itself acknowledges in its contributor guide); and `max_turns=None` deliberately permits unbounded runs, so "can it terminate without human intervention" is only guaranteed *yes* when operators keep the default limit.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Router implementation | `process_model_response` classifies each model output item into typed run plans (handoffs, functions, computer/shell/apply-patch actions, MCP approvals) via ordered type dispatch | `src/agents/run_internal/turn_resolution.py:2684-3436` |
| Handoff-vs-tool discrimination | `is_handoff_tool_call`: only bare (non-namespaced) function-call names matching handoff tool names route to handoffs | `src/agents/run_internal/turn_resolution.py:206-213` |
| Handoff-as-tool naming | `Handoff.default_tool_name` derives `transfer_to_<agent_name>`; description includes `handoff_description` | `src/agents/handoffs/__init__.py:207-218` |
| Handoff contract | `Handoff` dataclass: `tool_name`, `on_invoke_handoff` returning an agent, `input_filter`, `nest_handoff_history`, `is_enabled`, weakref `_agent_ref` | `src/agents/handoffs/__init__.py:126-201` |
| Handoff construction validation | `handoff()` rejects `input_type` without `on_handoff`, wrong arity callbacks; always returns the captured agent (no dynamic destination) | `src/agents/handoffs/__init__.py:293-311, 343` |
| Speaker selection (enabled set) | `get_handoffs` resolves plain `Agent` entries into `Handoff`s and filters by `is_enabled` (bool or context-aware callable) in parallel with sibling cancellation | `src/agents/run_internal/turn_preparation.py:96-116` |
| Multiple-handoff arbitration | First handoff wins; extra ones get `"Multiple handoffs detected, ignoring this one."` outputs plus a span error listing requested agents | `src/agents/run_internal/turn_resolution.py:563-597` |
| Handoff execution & hooks | `execute_handoffs` invokes `on_invoke_handoff`, emits `HandoffOutputItem` with transfer message, fires run-level + agent-level hooks concurrently | `src/agents/run_internal/turn_resolution.py:527-628` |
| Transfer message format | `get_transfer_message` returns `{"assistant": agent.name}` JSON | `src/agents/handoffs/__init__.py:203-204` |
| Input filter pipeline | Per-handoff `input_filter` overrides run-level filter; result validated (`UserError` on non-`HandoffInputData`); `input_items` split model-input from session history | `src/agents/run_internal/turn_resolution.py:630-711` |
| Nested history on handoff | `nest_history` wraps prior transcript into summary payload via `_nest_handoff_history_with_provenance` or user mapper | `src/agents/run_internal/turn_resolution.py:546-561` |
| Server-managed constraints | Input filters raise `UserError`; nesting auto-disabled with warning under `conversation_id`/`previous_response_id` | `src/agents/run_internal/turn_resolution.py:495-524` |
| Loop next-step dispatch (sync) | `NextStepHandoff` swaps `current_agent` and continues the same loop; `NextStepRunAgain` continues; `NextStepFinalOutput` returns `RunResult` | `src/agents/run.py:1392-1404, 1217-1218, 1230+` |
| Loop next-step dispatch (streaming) | Mirrored handling incl. `AgentUpdatedStreamEvent` publication and per-turn persistence | `src/agents/run_internal/run_loop.py:1856-1939` |
| Final-output from messages | When no tools/approvals remain, last message text becomes final output (plain text or schema-validated) | `src/agents/run_internal/turn_resolution.py:958-1088` |
| Tool-driven termination | `check_for_final_output_from_tools`: `run_llm_again` / `stop_on_first_tool` / `stop_at_tool_names` dict / callable `tool_use_behavior` | `src/agents/run_internal/turn_resolution.py:753-781` |
| Refusal termination | Model refusal raises `ModelRefusalError` unless a `model_refusal` error handler supplies fallback output | `src/agents/run_internal/turn_resolution.py:963-998` |
| Max-turns default | `DEFAULT_MAX_TURNS = 10` | `src/agents/run_config.py:45` |
| Max-turns check (sync) | `current_turn += 1` then `if max_turns is not None and current_turn > max_turns:` → attach span error, build `MaxTurnsExceeded` | `src/agents/run.py:1465-1474` |
| Max-turns check (streaming) | Identical check at `src/agents/run_internal/run_loop.py:1536-1574` |
| MaxTurnsExceeded exception | Public exception carrying optional `run_data`; subclasses `AgentsException` | `src/agents/exceptions.py:444-451` |
| Max-turns error handler | `error_handlers={"max_turns": ...}` converts the trip into a schema-validated synthesized final output persisted like a real turn | `src/agents/run_internal/run_loop.py:742-786`; `src/agents/run.py:1496-1563` |
| Disabling the bound | `max_turns=None` skips the check; `RunState._is_complete` also guards `self._max_turns is not None` | `tests/test_max_turns.py:63-86`; `src/agents/run_state.py:952` |
| Interruptions pause the group | `NextStepInterruption` carries `ToolApprovalItem`s awaiting human decision; loop returns a result with interruptions instead of proceeding | `src/agents/run_internal/run_steps.py:170-181`; `src/agents/run.py:1170-1215` |
| Guardrail tripwires terminate | Input tripwires raise mid-loop; output tripwires raise at finalization, both ending the run | `src/agents/run.py:1637+, 1252+`; `src/agents/run_internal/run_loop.py:1218-1232` |
| Call-ID reuse rejection | Preflight raises `ModelBehaviorError` when the model reuses a call ID for a different invocation within one response | `src/agents/run_internal/turn_resolution.py:3506-3534` |
| Tool-name collision resolution | `resolve_tool_name_collisions(current_tool_inventory, available_handoffs, collision_policy=...)` before routing decisions | `src/agents/run_internal/turn_resolution.py:1946-1950`; tests `tests/test_handoff_tool.py:90-180` |
| Forced-tool-choice reset | `maybe_reset_tool_choice` clears `tool_choice` after use when `agent.reset_tool_choice` to prevent forced-tool livelock | `src/agents/run_internal/tool_execution.py:556-564` |
| Nested agent-tool bound | `Agent.as_tool()` nested runs resolve their own max turns defaulting to `DEFAULT_MAX_TURNS` | `src/agents/agent.py:713` |
| Streaming cancel modes | `after_turn` cancellation completes the stream between turns rather than mid-execution | `src/agents/run_internal/run_loop.py:356-366` |
| Prompt-level routing guidance | `RECOMMENDED_PROMPT_PREFIX` instructs models how transfers work and not to narrate them | `src/agents/extensions/handoff_prompt.py:3-12` |

## Answers to Dimension Questions

### 1. How are messages routed?

Routing is a two-stage process. Stage one classifies: `process_model_response` (`src/agents/run_internal/turn_resolution.py:2684-3436`) walks every item in the model response and files it by type — assistant messages become `MessageOutputItem`s, function calls are looked up in a handoff map first (`is_handoff_tool_call`, `src/agents/run_internal/turn_resolution.py:206-213`) and then a function-tool lookup map, with computer/shell/apply-patch/custom/MCP items queued into dedicated run plans on `ProcessedResponse` (`src/agents/run_internal/run_steps.py:117-152`). Only bare names can match a handoff — namespaced calls never resolve to one, which prevents cross-agent namespace hijacking. Unknown tool names either raise `ModelBehaviorError` or, under `run_config.tool_not_found_behavior="return_error_to_model"`, produce a `ToolCallOutputItem` feeding the error back to the model (`src/agents/run_internal/turn_resolution.py:3387-3399`). Stage two executes: `execute_tools_and_side_effects` (`src/agents/run_internal/turn_resolution.py:784-1098`) orders resolution as interruptions → handoffs → tool-promoted final output → message final output → `NextStepRunAgain`. Every invoked call identity is validated against previously executed invocations, and reused call IDs with mismatched fingerprints abort the turn (`src/agents/run_internal/turn_resolution.py:3439-3534`).

### 2. How is the next speaker selected?

There is no scheduler or rotation: selection is delegated to the LLM through handoff tools. Agents declare `handoffs=[...]` where entries may be raw `Agent`s (auto-wrapped by `handoff()`, `src/agents/run_internal/turn_preparation.py:99-103`) or customized `Handoff` objects. The effective candidate set is recomputed each turn via `get_handoffs`, which honors dynamic `is_enabled` predicates evaluated against the run context (`src/agents/run_internal/turn_preparation.py:105-115`). The destination is fixed at construction time — `handoff()`'s closure always returns the captured agent (`src/agents/handoffs/__init__.py:343`), and docs confirm custom `Handoff.on_invoke_handoff` should be used only for side effects, not dynamic destination selection (`docs/handoffs.md:44`). If the model emits multiple handoff calls in one response, the first wins and the rest receive an explicit ignore notice plus a recorded span error (`src/agents/run_internal/turn_resolution.py:563-597`). On `NextStepHandoff` the loop swaps `current_agent`, resets the agent span, re-arms start hooks, and continues the same while-loop — the "group" is really a single sequential chain of agent turns.

### 3. How are handoffs managed?

`execute_handoffs` (`src/agents/run_internal/turn_resolution.py:527-750`) defines the contract. It (a) invokes `on_invoke_handoff` with the context and model-generated arguments (strict-JSON validated when `input_type` is set, `src/agents/handoffs/__init__.py:313-343`), (b) appends a `HandoffOutputItem` whose payload is `{"assistant": "<name>"}` (`src/agents/handoffs/__init__.py:203-204`), (c) fires run-level `hooks.on_handoff` and agent-level hooks concurrently with sibling cancellation (`src/agents/run_internal/turn_resolution.py:613-628`), and (d) reshapes what the receiving agent sees: an optional `input_filter` (per-handoff overrides `RunConfig.handoff_input_filter`) may rewrite history/new-items, with `input_items` separating model-visible input from session history (`src/agents/run_internal/turn_resolution.py:663-711`), and/or `nest_handoff_history` collapses the prior transcript into a nested summary (`src/agents/run_internal/turn_resolution.py:546-561`). Under server-managed conversations, filters are rejected outright and nesting is downgraded with a warning (`src/agents/run_internal/turn_resolution.py:495-524`). Resume flows reconcile already-executed handoff calls by call ID so a resumed state does not double-execute them (`executed_handoff_call_ids`, `src/agents/run_internal/turn_resolution.py:1964-1969, 2620-2625`).

### 4. When does a group conversation terminate?

Six distinct terminal conditions exist:

1. **Natural completion** — the current agent produces a message, no local tool work remains (`has_tools_or_approvals_to_run()`, `src/agents/run_internal/run_steps.py:133-148`), and the extracted/refusal/validated text becomes `NextStepFinalOutput` (`src/agents/run_internal/turn_resolution.py:958-1088`).
2. **Tool-forced completion** — `tool_use_behavior` promotes a tool result directly to final output (`stop_on_first_tool`, `stop_at_tool_names`, or a callable predicate; `src/agents/run_internal/turn_resolution.py:753-781`).
3. **Refusal** — a structured-output refusal raises `ModelRefusalError` or is converted by a `model_refusal` handler (`src/agents/run_internal/turn_resolution.py:963-998`).
4. **Max turns exceeded** — the hard backstop. Both loops increment `current_turn` and compare against `max_turns` (default 10, `src/agents/run_config.py:45`): sync at `src/agents/run.py:1465-1563`, streamed at `src/agents/run_internal/run_loop.py:1536-1654`. This raises `MaxTurnsExceeded` (`src/agents/exceptions.py:444-451`) with full `run_data`, or yields a handler-synthesized, schema-validated final output (`finalize_max_turns_handler_output`, `src/agents/run_internal/run_loop.py:742-786`). Passing `max_turns=None` disables the bound entirely (`tests/test_max_turns.py:63-86`).
5. **Guardrail tripwires** — input guardrails raise before sandbox/model execution; output guardrails raise at finalization (`src/agents/run.py:1637+, 1252+`).
6. **Human interruption** — `NextStepInterruption` pauses the run and returns pending `ToolApprovalItem`s; this is a *pause*, not termination, resumable via `RunState`.

Streaming cancellation (`after_turn` mode) additionally ends a run cleanly between turns (`src/agents/run_internal/run_loop.py:356-366`). So yes — a conversation terminates without human intervention whenever the model stops calling tools or the turn budget trips; but if `max_turns=None` is set, only the model's own cooperation guarantees termination.

### 5. Is deadlock possible?

**Yes, unbounded looping is possible; true deadlock is not.** There is no cycle detector over the handoff graph: agents A↔B can ping-pong indefinitely, and with `max_turns=None` this never halts. With the default limit, the loop always terminates within N model turns because the check is unconditional and outside model control. Mitigating mechanisms that prevent *stuck* states short of infinite loops:

- Duplicate/mismatched call-ID reuse fails fast with `ModelBehaviorError` rather than silently replaying (`src/agents/run_internal/turn_resolution.py:3506-3534`).
- Multiple simultaneous handoffs collapse deterministically (first-wins) instead of deadlocking on ambiguity (`src/agents/run_internal/turn_resolution.py:563-575`).
- Pending approvals dedupe by call-ID key so repeated responses cannot stack unbounded interruptions (`pending_interruption_keys`, `src/agents/run_internal/turn_resolution.py:1311-1549`).
- `reset_tool_choice` prevents a forced `tool_choice` from livelocking the model into the same tool forever (`src/agents/run_internal/tool_execution.py:556-564`).
- Nested `Agent.as_tool()` sub-runs inherit their own default turn cap (`src/agents/agent.py:713`).

No dedicated deadlock-detector component was found (searched `src/agents/**` for cycle/livelock/deadlock logic; none exists). Deadlock prevention is entirely emergent from the turn budget plus fail-fast validation.

## Architectural Decisions

1. **Routing-as-tools.** Handoffs are ordinary function tools derived from agent names (`src/agents/handoffs/__init__.py:207-211`). This reuses the provider's native tool-calling path for routing and avoids a bespoke inter-agent protocol; the cost is that routing quality depends on the model, and the SDK compensates with arbitration rules rather than stronger guarantees.

2. **Explicit four-state step machine.** Every turn resolves to `NextStepHandoff | NextStepFinalOutput | NextStepRunAgain | NextStepInterruption` (`src/agents/run_internal/run_steps.py:155-181`). This makes control flow auditable and lets streaming/sync paths share semantics.

3. **Fixed handoff destinations.** `handoff()` closes over one target agent (`src/agents/handoffs/__init__.py:343`); multi-destination routing requires registering multiple handoffs. This trades flexibility for predictability and simpler tracing (`handoff_span(from_agent/to_agent)`, `src/agents/run_internal/turn_resolution.py:578-587`).

4. **Turn budget as universal liveness bound.** One counter (`current_turn`) governs all progress — including handoffs, which consume turns — making termination analysis trivial: the loop cannot exceed `max_turns` model round-trips regardless of graph shape (`src/agents/run.py:1465`; `src/agents/run_internal/run_loop.py:1542`).

5. **Error handlers as soft-termination valves.** `max_turns`, `model_refusal`, and `invalid_final_output` handler kinds (`src/agents/run_internal/error_handlers.py:41`) let applications convert hard failures into valid final outputs, keeping the public surface exception-based while enabling graceful degradation.

6. **Dual loop implementations kept behaviorally aligned.** The non-streaming loop (`run.py`) and streaming generator (`run_internal/run_loop.py`) duplicate turn accounting, max-turns handling, and next-step dispatch; repo guidance explicitly instructs contributors to keep them aligned (AGENTS.md, Runner lifecycle section).

## Notable Patterns

- **Bare-name handoff matching with namespacing carve-out**: namespaced tool calls can never trigger handoffs, isolating sub-agent tool namespaces (`src/agents/run_internal/turn_resolution.py:206-213`).
- **First-wins multi-handoff arbitration with observability**: losers get synthetic tool outputs so the model sees why they were ignored, plus a `SpanError` listing all requested agents (`src/agents/run_internal/turn_resolution.py:563-597`).
- **Model-input vs session-history separation**: `HandoffInputData.input_items` allows filtering what the next agent *sees* while preserving complete history for sessions (`src/agents/handoffs/__init__.py:94-99`; applied at `src/agents/run_internal/turn_resolution.py:702-708`).
- **Nested handoff history with provenance tracking**: rewritten history items are tracked (`nested_history_owned_items`) so resume/reconciliation can distinguish owned vs original items (`src/agents/run_internal/run_steps.py:212-217`; reconciliation at `src/agents/run_internal/run_loop.py:1828-1838`).
- **Dynamic capability gating**: both handoff enablement and tool enablement support async predicates evaluated per-turn with parallel fan-out and cancellation (`src/agents/run_internal/turn_preparation.py:105-115`; `src/agents/run_internal/tool_execution.py:573-589`).
- **Prompt-side routing contract**: a recommended system-prompt prefix teaches models the `transfer_to_*` convention (`src/agents/extensions/handoff_prompt.py:3-12`) — routing policy partly lives in prompts, not just code.

## Tradeoffs

- **LLM-selected routing is flexible but non-deterministic.** Any agent can reach any enabled peer; correctness relies on prompt guidance and the turn budget. There is no static topology enforcement (contrast with fixed workflow graphs).
- **`max_turns=None` is a footgun.** It is fully supported and tested (`tests/test_max_turns.py:63-86`), meaning the harness permits non-terminating configurations by design; safety depends entirely on caller discipline.
- **Duplicated loop logic doubles maintenance risk.** Sync and streaming paths implement termination independently (~600 lines apart); behavioral parity is tested (e.g., `tests/test_max_turns.py:500-554` asserts equal session outcomes) but must be perpetually maintained.
- **First-wins handoff arbitration silently drops requests.** Deterministic and observable via spans, but a model that "changes its mind" twice in one response gets no retry — the ignored call is final for that turn.
- **Server-managed conversations restrict routing features.** Input filters and nesting are unavailable or degraded (`src/agents/run_internal/turn_resolution.py:495-524`), forcing feature tradeoffs when using `conversation_id`/`previous_response_id`.
- **No cycle detection keeps the core simple but pushes liveness wholly onto `max_turns`.** An explicit detector could give better diagnostics (e.g., "A→B→A repeated 5×") than a generic turn-count error.

## Failure Modes / Edge Cases

- **Ping-pong livelock**: mutual handoffs consume turns until `MaxTurnsExceeded`; recoverable via `max_turns` handler fallback output.
- **Self-handoff**: nothing prevents an agent from handing off to itself (the tool list is built from declared handoffs only, so it requires explicit self-registration; no runtime guard exists — verified by absence in `src/agents/run_internal/turn_preparation.py:96-116` and `src/agents/handoffs/__init__.py`).
- **Duplicate-name agents**: historically ambiguous for approval ownership; legacy serialized states accept same-name matches while schema ≥1.7 requires object identity (`src/agents/run_internal/turn_resolution.py:1279-1301`).
- **Tool-name collisions between handoffs/tools**: rejected at setup or resolved by configurable collision policy (`tests/test_handoff_tool.py:90-180`; `src/agents/run_internal/turn_resolution.py:1946-1950`).
- **Call-ID reuse by the model**: fails fast with `ModelBehaviorError`, preventing replay confusion across resumes (`src/agents/run_internal/turn_resolution.py:3506-3534`).
- **Interrupted handoff resume**: executed handoff call IDs are detected and skipped so resume doesn't re-transfer (`src/agents/run_internal/turn_resolution.py:1964-1969`).
- **Max-turns handler output failing validation**: surfaces as `UserError` and persists only pre-existing input (`tests/test_max_turns.py:294-309, 763-781`).
- **Session save cancellation during max-turns fallback**: output is not published and the run reports cancellation without corrupting state (`tests/test_max_turns.py:851-889`).

## Future Considerations

- Add optional handoff-graph cycle detection or per-edge hop counters to emit richer diagnostics than `MaxTurnsExceeded` (e.g., naming the repeating agent pair).
- Unify sync/streaming loop scaffolding around a shared turn-accounting primitive to eliminate dual-maintenance drift risk (repo already flags alignment as a standing obligation).
- Consider opt-in topology constraints (e.g., declarative allowed-transition sets) for teams wanting deterministic routing guarantees on top of the LLM-driven default.
- Surface routing telemetry (handoff counts per pair, turn histograms) as first-class usage metrics; today this requires parsing trace spans.

## Questions / Gaps

- No evidence found of any deadlock/cycle detection mechanism: searches across `src/agents/` for cycle, livelock, and repeat-visit logic returned only unrelated matches (e.g., polling `while True` loops in `src/agents/models/openai_responses.py:1309`). The question "Is deadlock possible?" is answered from the turn-budget mechanism alone.
- Realtime sessions implement a parallel handoff system (`src/agents/realtime/handoffs.py:32-80`) with its own enablement filtering; whether realtime sessions share the turn-budget guarantee was out of scope for this dimension (they are event-driven, not turn-looped). No max-turns equivalent was found in `src/agents/realtime/session.py` — realtime termination appears to depend on connection lifecycle rather than a turn budget.
- The experimental `hosted_multi_agent` extension (`src/agents/extensions/experimental/hosted_multi_agent/model.py:765`) contains its own loop; its routing/termination semantics were not deeply audited here as it is marked experimental.

---

Generated by `Dimension 15.02: Message Routing and Termination` against `openai-agents-sdk`.
