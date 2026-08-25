# Source Analysis: openai-agents-sdk

## Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, pydantic dataclasses, asyncio, OpenAI Responses API |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents SDK tracks objectives and progress through a **structural, runner-owned completion model** rather than an explicit goal representation. There is no goal object, plan tree, or milestone record anywhere in the SDK. Instead, "the goal" is defined implicitly by three things: the caller's input, the agent's `instructions`, and `Agent.output_type` (`src/agents/agent.py:360-367`) — a typed contract for what "done" looks like. A run terminates when the runner classifies a model response as final output (message of the desired type with no pending tool calls or handoffs, `docs/running_agents.md:43-45`), when `max_turns` is exhausted (`src/agents/run_config.py:45`, `src/agents/run.py:1466`), or when a guardrail tripwire fires.

Progress is observable through four parallel channels: (1) a turn counter exposed live on `RunResultStreaming.current_turn` / `max_turns` / `is_complete` (`src/agents/result.py:605-622`), incremented once per LLM invocation (`src/agents/run_internal/run_loop.py:1536-1537`); (2) a stream event taxonomy of raw token deltas, semantic item events (`tool_called`, `tool_output`, `handoff_requested`, …), and agent-change events (`src/agents/stream_events.py:10-61`); (3) lifecycle hooks (`on_llm_start/end`, `on_tool_start/end`, `on_agent_start/end`, `on_handoff`, `src/agents/lifecycle.py:18-103`); and (4) tracing spans — including dedicated `task` and `turn` spans that carry per-span token usage (`src/agents/tracing/span_data.py:64-132`, `src/agents/run_internal/run_loop.py:1737-1784`). Token/request usage is aggregated into a serializable `Usage` object with per-request breakdowns (`src/agents/usage.py:195-229`).

Completion is verified, not merely declared: structured outputs are JSON-validated against the output schema before being accepted (`src/agents/run_internal/turn_resolution.py:999-1035`), refusals are detected and raised as `ModelRefusalError` (`src/agents/run_internal/turn_resolution.py:963-964`), and user-supplied output guardrails run on every final output — including synthesized max-turns fallbacks — and can halt the run with a tripwire (`src/agents/run_internal/guardrails.py:171-224`). Blockers are first-class: tool approvals pause the run as `ToolApprovalItem` interruptions captured in a versioned, serializable `RunState` (`src/agents/result.py:515-516`, `src/agents/run_state.py:182`, `src/agents/run_state.py:985-1017`). The model cannot fake mechanical progress (turn counts, usage, executed-tool tracking are runner-accounted), but for plain-text agents without guardrails, semantic success — whether the answer actually achieves the goal — is never independently checked; it is whatever the model's last message says.

## Rating

**7 / 10** — Clear completion model with explicit interfaces, strong tests, and operational safeguards (schema validation, tripwires, resumable interruption state, streaming error re-checking). It loses points because progress is activity-centric (turns, tokens, items) rather than goal-centric: there are no goal objects or milestone records, semantic success verification is entirely delegated to optional user-supplied guardrails, and plain-text agents terminate successfully with no independent check at all.

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal representation (implicit) | `output_type` defines the shape of "done"; docs state final output = text of desired type with no tool calls | `src/agents/agent.py:360-367`; `docs/running_agents.md:43-45` |
| Run identity for traces | `workflow_name`, `trace_id`, `group_id`, `trace_metadata` name/group the run | `src/agents/run_config.py:412-429` |
| Loop termination contract | Runner docstring: loop until final output; `MaxTurnsExceeded` and tripwire exceptions | `src/agents/run.py:271-286` |
| Turn limit default | `DEFAULT_MAX_TURNS = 10`; `max_turns=None` disables the limit | `src/agents/run_config.py:45`; `src/agents/run.py:296-298` |
| Live progress fields | `current_agent`, `current_turn`, `max_turns`, `final_output`, `is_complete` on streaming result | `src/agents/result.py:605-622` |
| Turn increment | `current_turn += 1` mirrored onto `streamed_result.current_turn` each iteration | `src/agents/run_internal/run_loop.py:1536-1537` |
| Max-turns enforcement (streaming) | Span error `{"max_turns": ...}` attached, then `MaxTurnsExceeded` or handler-synthesized output | `src/agents/run_internal/run_loop.py:1542-1656` |
| Max-turns enforcement (non-streaming) | `if max_turns is not None and current_turn > max_turns:` branch with handler support | `src/agents/run.py:1466-1554` |
| Progress event taxonomy | `RawResponsesStreamEvent`, `RunItemStreamEvent` (11 semantic names), `AgentUpdatedStreamEvent` | `src/agents/stream_events.py:10-61` |
| Semantic event documentation | Events let UIs push "message generated"/"tool ran" level updates | `docs/streaming.md:70-88` |
| Lifecycle hooks | `on_llm_start/end`, `on_agent_start/end`, `on_handoff`, `on_tool_start/end` callbacks | `src/agents/lifecycle.py:13-103` |
| Usage aggregation | `Usage` with requests/input/output/total tokens plus per-request `request_usage_entries` | `src/agents/usage.py:195-229` |
| Usage on spans | `snapshot_usage`/`usage_delta`/`attach_usage_to_span` feed task and turn spans | `src/agents/run_internal/agent_runner_helpers.py:76-149` |
| Task/turn span data | `TaskSpanData` ("one top-level Runner run") and `TurnSpanData` ("one agent loop turn") with usage | `src/agents/tracing/span_data.py:64-132` |
| Final output from tools | `check_for_final_output_from_tools`: `stop_on_first_tool`, `stop_at_tool_names`, custom callable | `src/agents/run_internal/turn_resolution.py:753-781` |
| `tool_use_behavior` options | Literal `"run_llm_again"` / `"stop_on_first_tool"` / StopAtTools / callable documented | `src/agents/agent.py:373-393` |
| Infinite-loop guard | `reset_tool_choice=True` resets tool choice after any tool call to avoid tool-use loops | `src/agents/agent.py:395-397` |
| Structured-output validation | `output_schema.validate_json(...)` raises `ModelBehaviorError` on invalid final JSON | `src/agents/run_internal/turn_resolution.py:999-1035`; `src/agents/agent_output.py:52-57` |
| Refusal detection | Refusal extracted from last message; `ModelRefusalError` raised or routed to handler | `src/agents/run_internal/turn_resolution.py:952-998`; `tests/test_max_turns.py:165-199` |
| Output guardrails check | `GuardrailFunctionOutput.tripwire_triggered` halts execution; `OutputGuardrail` validates final output | `src/agents/guardrail.py:19-33`, `133-152` |
| Guardrail execution engine | Parallel guardrails, tripwire cancels siblings, attaches span error, raises exception | `src/agents/run_internal/guardrails.py:171-224` |
| Blockers as interruptions | `RunResult.interruptions: list[ToolApprovalItem]` and resume via `to_state()` | `src/agents/result.py:515-516`, `541-589` |
| Interruption state access | `RunState.get_interruptions()`, `approve()`, `reject()` on persisted step | `src/agents/run_state.py:985-1017`, `1255-1270` |
| Blocked-state constraints | `add_input()` rejects terminal/no-turns-left/interrupted states — blockers enforced on resume | `src/agents/run_state.py:941-979` |
| Durable progress state | `CURRENT_SCHEMA_VERSION = "1.17"`, `to_json`/`from_json` round-trip run progress | `src/agents/run_state.py:170-182`, `1704`, `2174` |
| Streaming error re-check | Consumer `_check_errors()` independently re-detects max-turns, tripwires, task exceptions | `src/agents/result.py:1047-1088` |
| Silent-failure detection | `run_loop_exception` property surfaces failures that bypassed the event queue | `src/agents/result.py:791-816` |
| UI status rendering | Pretty-print shows Current turn, Max turns, Is complete, Final output, item counts | `src/agents/util/_pretty_print.py:56-71` |
| Static graph visualization | `draw_graph(agent)` renders agent/tool/handoff topology, not live progress | `src/agents/extensions/visualization.py:406`; `docs/visualization.md` |
| Cancellation semantics | `cancel(mode="immediate"\|"after_turn")` documented with graceful drain | `src/agents/result.py:818-864`; `docs/streaming.md:58-68` |
| asyncio self-progress probe | Best-effort deadline inspection of cancelled tool tasks ("can this make progress?") | `src/agents/run_internal/_asyncio_progress.py:1-8`, `179-191` |
| Max-turns tests | Non-streamed/streamed `MaxTurnsExceeded`, `max_turns=None` disables, handler fallback paths | `tests/test_max_turns.py:38-126`, `353-497` |

## Answers to Dimension Questions

### 1. What is the goal?

There is **no explicit goal representation** — no goal object, task node, milestone, or plan record exists in the SDK (searched for `goal`, `milestone`, `objective`, `plan` across `src/agents/`; only planning-of-tool-calls matches in `src/agents/run_internal/tool_planning.py`). The goal is implicit in three artifacts: (a) the caller-provided `input`, (b) `Agent.instructions` (`src/agents/agent.py:408-415`), and (c) `Agent.output_type` — the typed contract for the deliverable (`src/agents/agent.py:360-367`). At the run level, `RunConfig.workflow_name` gives the whole run a logical name used only for tracing (`src/agents/run_config.py:412-415`). The documented completion rule is purely structural: *"the rule for whether the LLM output is considered a 'final output' is that it produces text output with the desired type, and there are no tool calls"* (`docs/running_agents.md:43-45`).

### 2. How is progress measured?

Progress is measured **structurally and mechanically**, through four mechanisms:

1. **Turn counting**: each LLM invocation increments `current_turn` (`src/agents/run_internal/run_loop.py:1536-1537`); exceeding `max_turns` raises `MaxTurnsExceeded` (`src/agents/run_internal/run_loop.py:1542-1574` non-streaming twin at `src/agents/run.py:1466-1490`).
2. **Item production**: every generated message/tool-call/handoff/reasoning item lands in `new_items` and is emitted as a named `RunItemStreamEvent` (`src/agents/stream_events.py:23-48`; `docs/results.md:66-83`).
3. **Token/request accounting**: aggregated into `Usage` with per-request entries (`src/agents/usage.py:195-229`), snapshotted and delta-attached to `task`/`turn` spans so observers see cost consumed per unit of progress (`src/agents/run_internal/agent_runner_helpers.py:76-149`; `src/agents/run_internal/run_loop.py:1737-1744`).
4. **Model judgment**: the LLM itself decides when to stop calling tools; progress between turns is inferred by the runner classifying each response into `NextStepFinalOutput` / `NextStepHandoff` / `NextStepInterruption` / `NextStepRunAgain` (`src/agents/run_internal/run_steps.py:161-199`).

Notably, progress is measured by **model judgment + structural signals**, not by tests or user approval. User approval appears only as a pause point (approvals), not as a progress signal.

### 3. Can the model fake progress?

**Mechanically, no; semantically, yes.**

- The model cannot fake turn counts, token usage, or executed work: turns are counted by the runner (`src/agents/run_internal/run_loop.py:1536-1537`), usage comes from provider responses (`src/agents/run_internal/run_loop.py:2306`), and tool use is recorded from processed responses keyed by tool identity (`src/agents/run_internal/tool_use_tracker.py:53-90`). Claiming "I ran the tool" without emitting a tool call produces no progress marker.
- The model cannot declare completion: the runner decides via `has_tools_or_approvals_to_run()` plus message extraction (`src/agents/run_internal/turn_resolution.py:952-958`). A response with pending tool calls always continues the loop.
- However, the model **can assert success semantically**: a plain-text agent's last message becomes `final_output` with zero independent verification unless the developer attaches output guardrails (`src/agents/guardrail.py:133-152`). Nothing checks that the content of the message achieves the stated goal.
- Two configuration choices shift trust further toward unverified outputs: `tool_use_behavior="stop_on_first_tool"` / `stop_at_tool_names` promotes a raw tool result straight to `final_output` without the LLM reviewing it (`src/agents/run_internal/turn_resolution.py:762-773`), and `max_turns=None` removes the loop bound entirely (`src/agents/run.py:296-298`; tested at `tests/test_max_turns.py:62-86`).
- Mitigations exist against runaway activity rather than fake claims: `reset_tool_choice` breaks tool-use cycles (`src/agents/agent.py:395-397`).

### 4. Are blockers recorded?

Yes — this is one of the SDK's strongest areas. Tools requiring approval pause the run as a `NextStepInterruption` whose pending `ToolApprovalItem`s surface on the result as `result.interruptions` (`src/agents/result.py:515-516`, `src/agents/run_internal/run_loop.py:1897-1928`). The full paused context is captured in a `RunState` via `to_state()` with a documented approve/reject/resume flow (`src/agents/result.py:541-589`; `src/agents/run_state.py:985-1017`, `1255-1270`), serialized under a versioned schema (`CURRENT_SCHEMA_VERSION = "1.17"`, `src/agents/run_state.py:170-182`) so blocks survive process restarts. The state machine actively enforces blocked-ness: `RunState.add_input()` refuses terminal states, states with no remaining turns, and interrupted states whose next tool result could end the run (`src/agents/run_state.py:941-979`). Failures also carry diagnostics: `AgentsException.run_data` is populated with input, items, responses, and all guardrail results (`src/agents/run_internal/run_loop.py:1948-1965`), and streaming exposes `run_loop_exception` to catch failures that bypassed the event queue (`src/agents/result.py:791-816`).

### 5. Is final success independently checked?

**Partially — type-checked and policy-checked, but not goal-checked.**

- **Structured outputs**: when `output_type` is set, the candidate final text is parsed and validated against the schema; invalid JSON raises `ModelBehaviorError` (optionally routed to an error handler) instead of completing the run (`src/agents/run_internal/turn_resolution.py:999-1035`; interface contract at `src/agents/agent_output.py:44-57`). Strict-schema mode constrains the model at generation time (`src/agents/agent_output.py:118-126`).
- **Output guardrails**: every accepted final output passes through user-supplied output guardrails, which run in parallel, cancel siblings on a tripwire, attach a trace error, and halt the run with `OutputGuardrailTripwireTriggered` (`src/agents/run_internal/guardrails.py:171-224`; exceptions at `src/agents/exceptions.py:532`). Crucially this applies even to synthesized fallback outputs from max-turn handlers (`src/agents/run_internal/run_loop.py:1583-1648`), and tests lock in the session/persistence semantics of tripped fallbacks (`tests/test_max_turns.py:616-703`).
- **Refusal detection**: model refusals on structured runs become `ModelRefusalError` rather than a silently "successful" refusal string (`src/agents/run_internal/turn_resolution.py:963-998`).
- **Streaming double-check**: the stream consumer independently re-evaluates max-turns, guardrail queues, and background task exceptions after the run loop finishes (`src/agents/result.py:1047-1088`, `996-1000`).
- **Gap**: for a plain-text agent with no output guardrails, nothing independently verifies success — the final message is trusted as-is. Even the convenience cast `final_output_as(cls)` performs no runtime check unless `raise_if_incorrect_type=True` is passed (`src/agents/result.py:413-429`). Verification quality is therefore entirely proportional to what the integrator configures.

## Architectural Decisions

1. **Structural completion over declarative goals.** The runner owns a deterministic classification of each model response into four next-steps (`src/agents/run_internal/run_steps.py:161-199`). Completion is a property of response shape (no tools + valid typed text), making termination predictable and provider-independent, at the cost of having no vocabulary for partial achievement of a multi-step goal.
2. **Runner-owned counters, not model-owned.** Turn counts and usage accumulate in runner/context objects (`context_wrapper.usage.add(...)`, `src/agents/run_internal/run_loop.py:2306`) and are snapshotted per span boundary (`src/agents/run_internal/agent_runner_helpers.py:97-120`). Progress quantities are tamper-resistant because the model never writes them.
3. **Verification as pluggable gates, not baked-in judges.** Schema validation (`src/agents/agent_output.py:52-57`) is always-on for typed outputs; semantic verification is a developer-supplied `@output_guardrail` (`src/agents/guardrail.py:305-343`). The SDK verifies form; developers verify meaning.
4. **Blockers as resumable state, not errors.** Approvals serialize the entire run into a schema-versioned `RunState` and resume later (`src/agents/run_state.py:1704`, `2174`), treating human-in-the-loop pauses as a normal control-flow outcome rather than a failure.
5. **Four redundant observability channels.** Stream events, hooks, tracing spans, and result-field introspection expose overlapping views of the same progress, so consumers can pick their abstraction level (token-level UI vs. post-hoc audit).

## Notable Patterns

- **Terminal-state sentinel pattern**: the streaming event queue ends with a private `QueueCompleteSentinel` so consumers deterministically detect completion and flush late guardrail exceptions (`src/agents/run_steps.py` import at `src/agents/result.py:44-48`; consumption at `src/agents/result.py:966-976`).
- **Delta accounting**: usage snapshots taken before each turn/task and diffed afterward keep span attribution exact even across retries (`apply_retry_attempt_usage` folds failed-attempt tokens back in, `src/agents/run_internal/run_loop.py:2274-2287`).
- **Self-progress introspection**: `_asyncio_progress.py` answers "when can this cancelled tool task next make progress?" by walking coroutine frames and loop timers, failing safe to `None` (`src/agents/run_internal/_asyncio_progress.py:1-8`, `179-191`) — progress reasoning applied even to cancellation paths.
- **Handler-parity testing**: nearly every completion edge case (max-turns handler, refusal handler, guardrail-tripped fallback) is tested on both streamed and non-streamed paths and asserted equal (`tests/test_max_turns.py:500-610`).
- **Honest pretty-printer**: the debug string exposes `Current turn / Max turns / Is complete / Final output` verbatim (`src/agents/util/_pretty_print.py:56-71`), mirroring the exact fields a UI would bind to.

## Tradeoffs

- **Simplicity vs. goal fidelity**: structural completion keeps the loop small and auditable, but the SDK cannot express "80% done", sub-goal completion, or ordered milestones; integrators must build that above `new_items`.
- **Trust placement**: `stop_on_first_tool` trades model review for latency/determinism, promoting unreviewed tool output to final status (`src/agents/run_internal/turn_resolution.py:764-765`).
- **Optional verification**: guardrails are opt-in; the safe path (typed outputs + guardrails) is documented but not defaulted for plain-text agents.
- **Unbounded runs**: `max_turns=None` is supported and tested (`tests/test_max_turns.py:62-86`), trading cost safety for long-horizon flexibility; cost overrun risk is mitigated only by external monitoring of `Usage`.
- **Event-name stability vs. clarity**: the misspelled `handoff_occured` event name is preserved for backward compatibility (`src/agents/stream_events.py:30-33`; `docs/streaming.md:90`).

## Failure Modes / Edge Cases

- **Silent streaming failure**: a run-loop exception before events flow (e.g., early sandbox init) can evade the stream; `run_loop_exception` exists specifically to re-surface it (`src/agents/result.py:791-816`, comment at `996-1000`).
- **Max-turns with handler**: the handler's synthetic output is schema-validated and guardrail-checked; an invalid handler output raises `UserError` mid-finalization (`tests/test_max_turns.py:293-308`; `src/agents/run_internal/run_loop.py:1583-1587`), leaving session persistence carefully rolled back (`tests/test_max_turns.py:763-780`).
- **No final response**: a stream that ends without a terminal response raises `ModelBehaviorError("Model did not produce a final response!")` (`src/agents/run_internal/run_loop.py:2303-2304`).
- **Tripped output guardrail on terminal tool output**: the run must persist the already-executed tool side effect while redacting the blocked output — handled with dedicated sanitization and retention logic (`src/agents/run_internal/run_loop.py:515-573`).
- **Blocked-state misuse**: adding input to a terminal or out-of-turns state raises `UserError` instead of silently queuing work (`src/agents/run_state.py:950-953`).
- **Usage-less providers**: adapters must explicitly mark completed requests without usage so request counts stay accurate without fabricating token totals (`src/agents/usage.py:315-343`).

## Future Considerations

- A first-class goal/checklist object (referenced by `output_type` validators or guardrails) would let runners report partial achievement instead of binary done/not-done.
- Surfacing `current_turn`/`is_complete` as emitted events (rather than polled result fields) would unify the progress channel for non-streaming consumers.
- An optional built-in "self-verification" turn (model critiques its own final output against declared criteria before commit) would close the plain-text trust gap while keeping guardrails user-owned.
- Cost-based loop bounds (e.g., `max_total_tokens` alongside `max_turns`) would complement the existing `Usage` accounting (`src/agents/usage.py:195-229`).

## Questions / Gaps

- No evidence of any milestone/subtask progress record: searches for `goal`, `milestone`, `objective`, and progress-named types found only `_asyncio_progress.py` (cancellation introspection, `src/agents/run_internal/_asyncio_progress.py:1`) and tool-call *planning* (`src/agents/run_internal/tool_planning.py`), neither of which models goal attainment.
- No independent evaluation harness inside the core runner: quality checking lives outside the SDK (e.g., `src/agents/sandbox/memory/rollouts.py:160` consumes `final_output` but imposes no success criterion visible in the studied scope).
- Whether hosted tools (web search, file search) can influence completion timing differently from local function tools is asserted in docstrings (`src/agents/agent.py:391-393`) but was not traced end-to-end in this study; boundary is limited to the runner classification code cited above.

---

Generated by `dimensions/06.05-objective-progress-tracking.md` against `openai-agents-sdk`.
