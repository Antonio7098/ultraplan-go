# Source Analysis: openai-agents-sdk

## Dimension 14.03: Human Intervention and Takeover

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (asyncio, Pydantic; OpenAI Responses API) |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK implements human intervention as a first-class **pause → decide → resume** lifecycle rather than in-process mid-flight steering. The core primitive is `RunState` (`src/agents/run_state.py:749`), a serializable snapshot of a run explicitly documented as "the durable pause/resume boundary for human-in-the-loop flows" (`src/agents/run_state.py:752`). Tools declare `needs_approval` (bool or async callable, `src/agents/tool.py:2465-2466`); when the model emits such a call without a stored decision, the runner stops and returns `RunResult.interruptions` containing `ToolApprovalItem`s (`src/agents/result.py:515-516`). The host application serializes state with `to_json()`/`to_string()` (`src/agents/run_state.py:1704`, `src/agents/run_state.py:2042`), records decisions via `state.approve(...)` / `state.reject(...)` (`src/agents/run_state.py:1255`, `src/agents/run_state.py:1270`), optionally stages new user input via `add_input()` (`src/agents/run_state.py:941`), and resumes by passing the `RunState` back into `Runner.run(agent, state)` (`src/agents/run.py:256-259`). Realtime sessions get true live-session approval via `approve_tool_call`/`reject_tool_call` while the session stays open (`src/agents/realtime/session.py:969`, `src/agents/realtime/session.py:1015`). Sandbox takeover is indirect: sandbox sessions are serializable/resumable state machines with audit event sinks, but there is no interactive attach-to-live-shell mechanism. Decisions are durable and versioned (schema v1.17 changelog at `src/agents/run_state.py:186-217`), though no actor identity (who approved) is captured.

## Rating

**8 / 10** — Clear model with explicit public interfaces (`ToolApprovalItem`, `RunState.approve/reject/add_input`, `RunContextWrapper.approve_tool/reject_tool`, `RealtimeSession.approve_tool_call/reject_tool_call`), extensive tests (320 tests in `tests/test_run_state.py`; 67 error-scenario tests in `tests/test_hitl_error_scenarios.py`; session-scenario tests in `tests/test_hitl_session_scenario.py:89,144`), operational safeguards (fail-closed approval rules, schema fail-fast validation, per-call vs sticky decision semantics), and streaming/non-streaming/realtime parity. It falls short of 9–10 because interventions are not attributable to an actor (no user identity on any approve/reject call), approval decisions have no dedicated tracing span (only serialized-state persistence and sandbox-op audit events), and forking is only implicitly possible via independent `to_state()` checkpoints rather than a supported fork API.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| State edit APIs — approve/reject on paused run | `RunState.approve(approval_item, always_approve)` and `RunState.reject(..., rejection_message=...)` mutate context-owned approval records | `src/agents/run_state.py:1255-1298` |
| Context-level decision ledger | `_ApprovalRecord` stores `approved`/`rejected` as bool (sticky) or list of call IDs (per-call), plus per-call `rejection_messages` and `sticky_rejection_message`/`sticky_scope` | `src/agents/run_context.py:56-68` |
| Public context mutation entry points | `RunContextWrapper.approve_tool` / `reject_tool` delegate to `_apply_approval_decision`, which validates canonical invocation identity before recording | `src/agents/run_context.py:1043-1063`, `src/agents/run_context.py:938-1041` |
| Inject feedback into paused run | `RunState.add_input()` stages input for admission immediately before the next resumed model call; rejects terminal states, exhausted max_turns, accepted-response-pending, and run-ending tool results | `src/agents/run_state.py:941-979` |
| Staged input is durable & guarded | Pending input survives serialization round-trips, runs input guardrails on resume, and is admitted exactly once | `src/agents/run_state.py:788-789`, `docs/results.md:140-154` |
| Interruption surface | `RunResult.interruptions: list[ToolApprovalItem]` returned when approvals are pending | `src/agents/result.py:515-516` |
| Approval item type | `ToolApprovalItem` dataclass carries raw tool call, tool name, namespace, origin, canonical lookup key; raw items span function/custom/shell/MCP/local-shell calls or dicts | `src/agents/items.py:543-609` |
| Tool-level trigger policy | `needs_approval: bool \| Callable[[RunContextWrapper, dict, str], Awaitable[bool]]` on `function_tool` | `src/agents/tool.py:2465-2466` |
| Fail-closed callable rules | `parse_function_tool_arguments` returns `None` (forcing manual approval) for malformed/non-object/NaN-Inf JSON; `evaluate_needs_approval_setting` raises `UserError` on invalid settings | `src/agents/util/_approvals.py:18-29`, `src/agents/util/_approvals.py:32-51` |
| Sticky vs per-call decisions | Exact-call decisions override sticky defaults; sticky decisions bind to approval scope | `src/agents/run_context.py:649-668`, `src/agents/run_context.py:998-1037` |
| Resume entry point | `Runner.run(starting_agent, input: str \| list[TResponseInputItem] \| RunState[TContext], ...)` accepts a `RunState` directly | `src/agents/run.py:256-259` |
| Resume execution path | On resume with `NextStepInterruption`, runner calls `resolve_interrupted_turn` replaying the last processed response with recorded decisions | `src/agents/run.py:1063-1135`, `src/agents/run_internal/turn_resolution.py:1134-1149` |
| Checkpoint independence | `result.to_state()` builds a `RunState` over a deep-copied context (`_copy_for_run_state` copies usage, approvals, invocation ledger) enabling multiple resumable checkpoints | `src/agents/result.py:541-579`, `src/agents/run_context.py:117-131` |
| Streaming parity | `RunResultStreaming.to_state()` mirrors non-streaming checkpointing after draining `stream_events()` | `src/agents/result.py:1139-1179` |
| Nested agent-as-tool routing | Outer `RunState` finds the nested agent-tool state owning an interruption via `_find_nested_approval_state` (recursive) and forwards decisions | `src/agents/run_state.py:1095-1253` |
| Ambiguity safeguard | Duplicate invocation identities across current/nested runs raise `UserError("Cannot apply approval because...")` instead of silently approving one | `src/agents/run_state.py:1087-1093`, `src/agents/run_state.py:1241-1252` |
| Durable serialization + versioning | `to_json` emits `$schemaVersion` 1.17; versioned changelog since 1.0 ("Initial RunState snapshot format for HITL pause/resume flows"); fail-fast validation of unsupported versions | `src/agents/run_state.py:182-218`, `src/agents/run_state.py:3842-3860` |
| Decision persistence in snapshots | Approvals, hosted-MCP decisions, and the tool-invocation ledger are serialized into the state payload | `src/agents/run_state.py:1300-1339`, `src/agents/run_state.py:1341-1384` |
| Restore-time hardening | `from_json` sanitizes untrusted payloads (`_validate_run_state_json_value`, mount-authority sanitization) and redacts error data | `src/agents/run_state.py:2201-2249` |
| Realtime live takeover | `RealtimeSession.approve_tool_call(call_id, always=...)` / `reject_tool_call(..., rejection_message=...)` resolve pending calls while the voice/WebSocket session continues | `src/agents/realtime/session.py:969-1013`, `src/agents/realtime/session.py:1015-1049` |
| Custom rejection messaging | Per-call `rejection_message` plus run-wide `RunConfig.tool_error_formatter` (`kind == "approval_rejected"`); per-call wins | `src/agents/run_config.py:83-105`, `src/agents/run_config.py:448`, `src/agents/run_state.py:1277-1281` |
| Conversation history editing | `Session` protocol exposes `get_items`/`add_items`/`pop_item`/`clear_session` for host-driven history edits | `src/agents/memory/session.py:15-56` |
| Sandbox resumability (no attach) | Sandbox sessions serialize/deserialize state and resume via `SandboxClient.resume`; snapshot persist/restore lifecycle handles workspace continuity | `src/agents/sandbox/session/sandbox_client.py:174-195`, `src/agents/sandbox/session/sandbox_client.py:280-286`, `src/agents/sandbox/session/snapshot_lifecycle.py:21-51` |
| Sandbox audit trail | Every sandbox op emits start/finish audit events with `event_id`, timestamps, span/trace correlation IDs to configurable sinks (JSONL/HTTP) | `src/agents/sandbox/session/events.py:32-53`, `src/agents/sandbox/session/manager.py:17`, `src/agents/sandbox/session/sinks.py:51-58` |
| Hosted-shell restriction | ShellTool/ApplyPatchTool reject `needs_approval`/`on_approval` when backed by a hosted environment | `src/agents/tool.py:1405-1409` |
| Documentation contract | HITL guide specifies pause/approve/resume flow, sticky persistence across serialization, partial resolution of mixed interruptions, long-running approvals | `docs/human_in_the_loop.md:45-57`, `docs/human_in_the_loop.md:187-203` |
| Test coverage | HITL session scenarios, error scenarios, approval-record tests, RunState corpus incl. historical fixture compatibility | `tests/test_hitl_session_scenario.py:89-144`, `tests/test_hitl_error_scenarios.py` (67 tests), `tests/test_run_context_approvals.py` (25 tests), `tests/test_run_state_compatibility_corpus.py:165-250` |

## Answers to Dimension Questions

### 1. Can humans edit agent state?

Yes, through a controlled public API rather than free-form field access. `RunState.approve`/`reject` record decisions into the run context's `_ApprovalRecord` ledger (`src/agents/run_state.py:1255-1298`, `src/agents/run_context.py:56-68`), supporting both sticky (tool-wide) and exact-call scoping. `RunState.add_input()` lets humans append new user messages to a paused or between-turns state, validated against terminal/exhausted/run-ending states before mutating (`src/agents/run_state.py:950-976`); staged input survives serialization and runs guardrails on resume (`docs/results.md:140-154`). Host apps can also directly edit conversation history through the `Session` protocol's `add_items`/`pop_item`/`clear_session` (`src/agents/memory/session.py:38-56`) and replace context wholesale via `context_override` on restore (`src/agents/run_state.py:2104-2107`). Raw `RunState` internals are underscore-private; there is no sanctioned arbitrary-field editing, which prevents inconsistent manual surgery but also means "edit any part of the transcript" requires dropping to Session APIs.

### 2. Can humans provide mid-run feedback?

Not synchronously inside a running turn in the default Runner: the loop halts and returns `interruptions`, and feedback happens between `Runner.run` invocations (`src/agents/run.py:1197-1215`; `docs/human_in_the_loop.md:47-51`). The SDK compensates in three ways: (a) partial resolution is supported — you may decide some interruptions now and leave others pending; unresolved ones re-pause the run (`docs/human_in_the_loop.md:57`); (b) `add_input()` injects new human text into the same logical run before its next model call rather than forcing a fresh conversation (`src/agents/run_state.py:941`); and (c) **Realtime sessions are genuinely live**: `approve_tool_call`/`reject_tool_call` resolve pending tool calls mid-session while audio keeps flowing (`src/agents/realtime/session.py:969-1049`). Rejection itself is rich feedback: a custom message can be sent back to the model so it can self-correct instead of restarting (`src/agents/run_state.py:1277-1281`, `src/agents/run_config.py:448`).

### 3. Can humans take over execution?

There is no interactive attach/take-over of a live process or sandbox shell. Takeover is expressed as checkpoint ownership: the human (via host code) holds a serialized `RunState` indefinitely — including across process restarts (`docs/human_in_the_loop.md:187-189`) — and decides whether each gated action proceeds. For sandboxes, the equivalent is session-state custody: sandbox sessions serialize to portable state and can be deleted, recreated, and resumed later with snapshot restoration (`src/agents/sandbox/session/sandbox_client.py:174-195`, `src/agents/snapshot_lifecycle` at `src/agents/sandbox/session/snapshot_lifecycle.py:21-51`), and all agent operations inside the sandbox are observable externally through audit-event sinks (`src/agents/sandbox/session/events.py:32-53`). PTY primitives exist (`pty_exec_start`/`pty_write_stdin` at `src/agents/sandbox/session/sandbox_session.py:586-621`) but are wired for agent tool use, not exposed as a human console. Notably, hosted shell environments refuse local approval hooks entirely (`src/agents/tool.py:1405-1409`), ceding control to the provider side.

### 4. Are human interventions traceable?

Partially. Interventions are **durable and inspectable**: every decision (including sticky scopes and custom rejection messages) is serialized into the state payload (`src/agents/run_state.py:1300-1324`, `src/agents/run_state.py:1341-1384`), the invocation ledger records executed/completed status per call ID (`src/agents/run_state.py:1326-1339`), and the format is versioned with a fail-fast reader plus historical-fixture regression tests (`src/agents/run_state.py:3842-3860`, `tests/test_run_state_compatibility_corpus.py:209-230`). However, two observability gaps remain: (a) **no actor attribution** — neither `state.approve()` nor `context_wrapper.approve_tool()` accepts a user/principal identity, so "who decided" lives only in application code; (b) **no dedicated tracing span** for approval decisions — I searched `src/agents/tracing/span_data.py` and found no approval/intervention span type; traceability of decisions is confined to serialized state, whereas the separate sandbox audit-event stream covers *operations* (exec/write) with trace/span correlation IDs but not human decisions. A rejected call does become visible to the model as a synthetic tool output explaining why (`src/agents/run_internal/approvals.py:24-43`).

## Architectural Decisions

1. **Checkpoint-based HITL, not callback-based blocking.** The run returns a serializable `RunState` instead of holding an await open for a human. This makes approvals survive process restarts, queues, and cross-service handoff (`src/agents/run_state.py:750-762`, `docs/human_in_the_loop.md:187-197`), trading away synchronous mid-turn steering.
2. **Decision state lives in the run context, not the item.** `_ApprovalRecord` keyed by tool name/hosted-MCP identity, with per-call-ID lists for scoped decisions, separates *what was called* (items) from *what was decided* (context) (`src/agents/run_context.py:56-68`).
3. **Canonical invocation identity for safety.** Approval decisions require a canonical `(invocation_type, call_id, approval_scope, fingerprint)` tuple and raise `ModelBehaviorError` otherwise, preventing misattributed approvals (`src/agents/run_context.py:938-952`, `src/agents/_tool_invocation.py:164-236`).
4. **Fail-closed policy evaluation.** If arguments cannot be safely parsed, the callable rule is skipped and manual approval is forced (`src/agents/util/_approvals.py:18-29`; documented at `docs/human_in_the_loop.md:15`).
5. **Versioned durable boundary.** `$schemaVersion` with chronological changelog, explicit backward-read policy, and fail-fast forward-compat rejection treat RunState JSON as a released external contract (`src/agents/run_state.py:176-232`, `src/agents/run_state.py:180`).
6. **Nested-interruption ownership resolution.** Approvals raised inside `Agent.as_tool()` sub-runs are owned by nested states but surfaced on the outer result; the outer state routes decisions down recursively and errors on ambiguous identities (`src/agents/run_state.py:1095-1253`, `docs/human_in_the_loop.md:5-7`).

## Notable Patterns

- **Defensive copying everywhere:** `get_interruptions()` returns hardened detached copies of pending approvals, with redacted-error handling if copying fails (`src/agents/run_state.py:985-1017`); `to_state()` deep-copies context so multiple checkpoints don't share mutable approval/usage state (`src/agents/run_context.py:117-131`).
- **Guardrails applied to injected input:** staged `pending_input` passes input guardrails on resume exactly like initial input, keeping the trust boundary uniform (`docs/results.md:152`).
- **Streaming/non-streaming behavioral parity:** both `RunResult.to_state()` (`src/agents/result.py:541`) and `RunResultStreaming.to_state()` (`src/agents/result.py:1139`) produce identical checkpoints, mandated by the repo's runner-lifecycle guidance.
- **Programmatic escape hatches alongside HITL:** `on_approval` (shell/apply_patch) and `on_approval_request` (hosted MCP) let code auto-decide without pausing, so HITL is opt-in per tool, not global friction (`docs/human_in_the_loop.md:89-97`).
- **Examples-as-contracts:** end-to-end pause/persist/reload/approve/resume examples ship for plain, streaming, custom-rejection, MCP, session-memory, and realtime surfaces (`examples/agent_patterns/human_in_the_loop.py`, `examples/tools/shell_human_in_the_loop.py`, `examples/memory/memory_session_hitl_example.py`), mirrored by scenario tests (`tests/test_hitl_session_scenario.py:89-144`).

## Tradeoffs

- **Durability vs immediacy:** serializable pauses enable hours-long approvals and cross-process resume, but a chat-style app cannot simply ask the user a question mid-turn without ending the run segment; realtime is the only truly concurrent path.
- **Safety vs ergonomics in identity matching:** strict canonical-identity requirements plus ambiguity errors (`src/agents/run_state.py:1241-1252`) prevent wrong approvals but push burden onto hosts to keep call IDs unique across nested runs.
- **Conservative context serialization:** mapping contexts round-trip freely; dataclasses/Pydantic models warn that type information is lost unless a deserializer is provided (`src/agents/run_state.py:1527-1566`) — honest, but means long-lived HITL workflows need extra plumbing for typed contexts.
- **Sticky decisions vs blast radius:** `always_approve=True` reduces prompting fatigue but grants tool-wide authorization for the remainder of the run; scope binding for hosted MCP (server_label + tool name) mitigates cross-server leakage (`docs/human_in_the_loop.md:55`, `src/agents/run_context.py:979-988`).

## Failure Modes / Edge Cases

- **Terminal/exhausted states reject injection:** `add_input` raises `UserError` on terminal states, exhausted max turns, accepted-but-unprocessed responses, and interrupted states whose tool result would end the run (`src/agents/run_state.py:950-976`) — surfaced clearly rather than silently dropped.
- **Unresolvable approval copies degrade safely:** if a pending approval cannot be safely copied, `get_interruptions` raises a redacted `UserError` instructing JSON-compatible payloads and unique call IDs (`src/agents/run_state.py:1004-1017`).
- **Duplicate identities blocked:** applying a decision when the same invocation identity exists in both current and nested runs raises rather than guessing (`src/agents/run_state.py:1241-1252`).
- **Forward-versioned snapshots fail fast:** older SDKs refuse newer schema versions with a supported-versions list (`src/agents/run_state.py:3852-3859`), avoiding silent field loss; docs recommend storing your own version markers for very long-lived approvals (`docs/human_in_the_loop.md:201-203`).
- **Hosted-environment mismatch caught at construction:** configuring `needs_approval`/`on_approval` on a hosted ShellTool raises immediately (`src/agents/tool.py:1405-1409`).
- **Resumed-run corruption guards:** resume validates presence of model responses and processed responses before continuing, raising `UserError` with remediation hints (`src/agents/run.py:1066-1074`).

## Future Considerations

- **Actor attribution:** adding an optional principal/identity parameter to `approve`/`reject` (and serializing it) would make interventions fully auditable without changing decision semantics.
- **Intervention tracing spans:** an SDK span (or trace attribute) emitted on approve/reject would unify decision history with existing run traces; currently only sandbox ops get correlated audit events (`src/agents/sandbox/session/events.py:46-50`).
- **First-class fork API:** independent checkpoints already make "resume twice from one pause" technically possible (`_copy_for_run_state`, `src/agents/run_context.py:117-131`); documenting or exposing an explicit `fork()` would formalize branch-and-compare workflows.
- **Mid-run injection for standard Runner:** a subscription/callback channel (like realtime's live approve) for non-voice runs would remove the stop-the-world requirement for hosts that keep processes alive.

## Questions / Gaps

- **Who approved?** No evidence found anywhere in `src/agents/` of user/principal capture on approval decisions (searched `approve`, `actor`, `user`, `principal`, `audit` across `run_state.py`, `run_context.py`, `realtime/session.py`). Application code must layer this itself.
- **Is there a supported way to rewind/fork conversation history mid-run?** Only `Session.pop_item()` offers last-item removal (`src/agents/memory/session.py:46-52`); no branching API exists (searched `fork`, `branch.*run`; only internal `_fork_with_tool_input` context helpers matched). Inference: forking is possible via duplicate checkpoints but is undocumented intent, not implemented behavior.
- **Do approval decisions appear in traces?** No evidence found in `src/agents/tracing/` of approval-specific spans or attributes; what I searched: `span_data.py`, grep for `approval.*span`. Serialized state and the model-visible synthetic rejection output are the only in-SDK records.
- **Sandbox interactive takeover (human terminal into running container):** No evidence found; searched sandbox session interfaces (`base_sandbox_session.py`, `sandbox_client.py`) — PTY methods serve agent tools, and no human-console surface exists.

---

Generated by `dimensions/14.03-human-intervention-and-takeover` against `openai-agents-sdk`.
