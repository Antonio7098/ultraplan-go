# Source Analysis: crewai

## Span Hierarchy and Run Tree

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic, OpenTelemetry SDK, httpx; monorepo with `lib/crewai`, `lib/crewai-core`, `lib/crewai-tools`) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI has **two distinct, parallel tracing systems**, and the run tree lives in the second one:

1. **OTel-based product telemetry** (`lib/crewai/src/crewai/telemetry/telemetry.py`, `lib/crewai-core/src/crewai_core/telemetry.py`): an OpenTelemetry `TracerProvider` exporting OTLP/HTTP to CrewAI's collector (`https://telemetry.crewai.com:4319`, `lib/crewai-core/src/crewai_core/telemetry.py:41`). Despite using OTel spans ("Crew Created", "Task Execution", "Tool Usage", ...), these spans are **flat point-in-time markers** — every span is created via `tracer.start_span(name)` with no parent context and closed immediately (`lib/crewai/src/crewai/telemetry/telemetry.py:296,527,563,650`). This system measures feature usage and durations for product analytics, not a run tree.

2. **Event-bus trace collection** (`lib/crewai/src/crewai/events/listeners/tracing/`): the actual run-tree mechanism. Every execution step emits a typed `BaseEvent` onto a singleton in-process event bus (`lib/crewai/src/crewai/events/event_bus.py:952`), and the bus automatically stamps each event with hierarchy metadata — `event_id`, `parent_event_id`, `previous_event_id`, `triggered_by_event_id`, `emission_sequence` (`lib/crewai/events/base_events.py:82-87`, assigned in `event_bus.py:537-570`). A `TraceCollectionListener` buffers events per execution session into a `TraceBatchManager`, which ships them to CrewAI AMP (a hosted backend) and returns a viewer URL.

The parent-child relationship is derived from a **contextvar scope stack**: ~30 event types are declared as "scope starting" and ~40 as "scope ending" with validated start/end pairs (`lib/crewai/events/event_context.py:240-371`). When `crew_kickoff_started` fires it is pushed on the stack; nested `task_started` events take it as `parent_event_id`; `task_completed` pops the stack and pairs against its start (`event_bus.py:548-563`). Coverage is broad: crews, flows and flow methods, agents, tasks, LLM calls (start/complete/fail plus streaming chunks), LLM guardrails, tool usage, memory query/save/retrieval, knowledge retrieval/query, agent reasoning, step observations, skills, MCP connections/tool executions, and A2A delegation conversations (`trace_listener.py:231-759`, `event_context.py:240-320`).

Nesting across harness boundaries is handled explicitly: a crew kicked off inside a flow method parents to the method's event (verified by tests, `lib/crewai/tests/events/test_event_ordering.py:325-354`); flows running inside crews (e.g. an agent's Flow-based executor) must not steal batch ownership from their parent (`trace_listener.py:238-248`); infrastructure flows that suppress lifecycle events claim the batch from flow context so their LLM/tool events are not misattributed (`trace_listener.py:822-854`). Multi-turn conversational sessions defer finalization until the whole session ends (`trace_listener.py:781-788`).

What is absent is **cross-process trace propagation**: there is no W3C `traceparent` injection/extraction anywhere. A2A delegation is traced only on the client side of the call (`lib/crewai/src/crewai/a2a/utils/delegation.py:484-503`); outbound headers carry auth only (`delegation.py:477`). OpenTelemetry baggage is used purely as an *in-process* context carrier for crew identity and flow inputs (`lib/crewai/src/crewai/crew.py:1040-1043`, `lib/crewai/src/crewai/flow/runtime/__init__.py:2138-2139`), not for distributed tracing.

The answer to "can you see the full path from user request to tool result in one trace?" is **yes within a single process/run session** — via the event-tree exported to the CrewAI AMP dashboard — but not across process boundaries, and not through standard OTel spans.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- The hierarchy model is explicit and mechanical (scope-stack pairing + sequence ordering), not inferred from timestamps; pairing violations warn or raise by configuration (`event_context.py:208-223`).
- Well tested: dedicated suites verify task→crew parenting, LLM-call parenting, crew-in-flow-method nesting, linear `previous_event_id` chains, and causal `triggered_by_event_id` chains (`lib/crewai/tests/events/test_event_ordering.py:173,228,292,325,525,633`; `lib/crewai/tests/tracing/test_tracing.py` is ~1,950 lines covering batch lifecycle, retries, auth fallback).
- Operational safeguards everywhere: exporter swallows failures (`crewai_core/telemetry.py:66-74`), batch init retries then degrades gracefully (`trace_batch_manager.py:175-232`), 401/403 falls back to ephemeral anonymous tracing (`trace_batch_manager.py:207-218`), signal handlers flush batches (`trace_listener.py:764-772`), failed batches are marked server-side (`trace_batch_manager.py:240-247`).

Not 8–10 because: no distributed propagation (A2A/MCP remote sides are unlinked); the run tree can only be viewed in the vendor-hosted AMP dashboard (no self-hosted OTLP export of the tree); buffered events are lost if the send fails (`trace_batch_manager.py:308-319`); a single global batch manager means concurrent executions in one process share one batch unless ownership rules happen to apply; and two overlapping tracing systems (flat OTel telemetry vs event tree) create confusion about which is authoritative.

## Evidence Collected

Every entry includes a file path with line numbers. All paths relative to `studies/agent-harness-study/sources/crewai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trace provider #1 (product telemetry) | `Telemetry` builds a private `TracerProvider` + `BatchSpanProcessor(SafeOTLPSpanExporter)` posting OTLP to `telemetry.crewai.com:4319/v1/traces` | `lib/crewai/src/crewai/telemetry/telemetry.py:128-151`; endpoint at `lib/crewai-core/src/crewai_core/telemetry.py:41,213-218` |
| No global provider install | `set_tracer()` deliberately does not set a global `TracerProvider` to avoid hijacking host-app spans; regression-tested with third-party tracers (redis/asgi) never reaching CrewAI's exporter | `lib/crewai/src/crewai/telemetry/telemetry.py:173-191`; `lib/crewai/tests/telemetry/test_tracer_isolation.py:74-80` |
| Flat OTel spans (no hierarchy) | Every span created via `tracer.start_span(name)` with no parent context arg; e.g. "Crew Created", "Task Created", "Task Execution", "Tool Usage" | `lib/crewai/src/crewai/telemetry/telemetry.py:296,527,563,622,650`; grep over `lib/` shows zero `start_span(..., context=...)` uses |
| Only long-lived OTel spans | Task Execution span held open between `task_started`/`task_ended` listeners; Crew Execution span stored on `crew._execution_span` and closed in `end_crew` | `lib/crewai/src/crewai/events/event_listener.py:188-190,246-270`; `lib/crewai/src/crewai/telemetry/telemetry.py:584-609,931-965` |
| Trace provider #2 (run tree) | `TraceCollectionListener` singleton registers ~50+ handlers on the event bus for flow/context/action/A2A/system events | `lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:140-150,203-229` |
| Event hierarchy fields | `BaseEvent` carries `event_id`, `parent_event_id`, `previous_event_id`, `triggered_by_event_id`, `started_event_id`, `emission_sequence` | `lib/crewai/src/crewai/events/base_events.py:82-87` |
| Parent assignment mechanism | `_prepare_event` sets `previous_event_id` = last emitted, `triggered_by_event_id` from causal scope, `emission_sequence` from counter, and `parent_event_id` from scope stack push/pop | `lib/crewai/src/crewai/events/event_bus.py:537-570` |
| Scope-stack definition | `SCOPE_STARTING_EVENTS` (~26 types incl. llm_call_started, tool_usage_started, llm_guardrail_started), `SCOPE_ENDING_EVENTS`, `VALID_EVENT_PAIRS` map | `lib/crewai/src/crewai/events/event_context.py:240-371` |
| Pairing safeguards | Max stack depth 100; empty-pop and start/end mismatch behaviors configurable WARN/RAISE/SILENT; warnings name the missing pair | `lib/crewai/src/crewai/events/event_context.py:21-26,171-223` |
| Causal chains | `triggered_by_scope()` context manager tags all events emitted inside listener-triggered work | `lib/crewai/src/crewai/events/event_context.py:118-133`; used at `lib/crewai/src/crewai/flow/runtime/__init__.py:3290` |
| Ordering guarantee | Finalized events sorted by `(emission_sequence, timestamp)` before send | `lib/crewai/src/crewai/events/listeners/tracing/trace_batch_manager.py:348-356` |
| In-memory tree (replay) | `EventRecord` links nodes by `parent_event_id` edges; checkpoint restore rebuilds scope stack + emission counter so resumed runs continue the same chain | `lib/crewai/src/crewai/state/event_record.py:103-125`; `lib/crewai/src/crewai/crew.py:573-601`; replay preserves ids (`event_bus.py:673-687`) |
| Coverage: LLM calls | `_emit_call_started_event` emits `LLMCallStartedEvent` with model/params/call_id; completed/failed variants include usage + finish_reason | `lib/crewai/src/crewai/llms/base_llm.py:552-613,615-643,645-661` |
| Coverage: tools | `ToolUsageStartedEvent`/`FinishedEvent` emitted around tool execution in `ToolUsage` | `lib/crewai/src/crewai/tools/tool_usage.py:291,548,1058` |
| Coverage: guardrails | `LLMGuardrailStartedEvent` before validation, `LLMGuardrailCompletedEvent` after, with success/error/retry_count | `lib/crewai/src/crewai/utilities/guardrail.py:162-185` |
| Coverage: delegation/A2A | `A2ADelegationStartedEvent` (+ conversation/polling/streaming/server-task events) emitted client-side around remote delegation | `lib/crewai/src/crewai/a2a/utils/delegation.py:484-503`; handler registration `trace_listener.py:620-759` |
| Coverage: memory/knowledge/reasoning/skills | Handlers for memory save/query/retrieval, knowledge retrieval/query, reasoning, step observations, skills | `trace_listener.py:426-618` |
| Batch lifecycle → backend | Batch init/send/finalize/mark-failed REST calls to PlusAPI tracing endpoints, authenticated or ephemeral | `lib/crewai-core/src/crewai_core/plus_api.py:408-488`; manager logic `trace_batch_manager.py:91-127,287-384` |
| Trace viewer/UI | Hosted AMP dashboard link printed after finalization (`.../crewai_plus/trace_batches/{batch_id}`); docs describe viewing traces in AMP | `trace_batch_manager.py:420-455`; `docs/edge/en/observability/tracing.mdx:151-155` |
| Enablement & consent | Priority: explicit override (`Crew(tracing=True)`) > env `CREWAI_TRACING_ENABLED` > stored user consent; first-run auto-collection with interactive prompt and ephemeral handling | `lib/crewai/src/crewai/crew.py:409,635-640`; `lib/crewai/src/crewai/flow/runtime/__init__.py:815-817`; `lib/crewai/src/crewai/events/listeners/tracing/utils.py:108-137,444-464`; `first_time_trace_handler.py:20-69` |
| Nested execution ownership | Flow-start handler refuses to re-claim an existing batch (nested flow inside crew/parent flow); nested crew kickoff defers to flow ownership; infra flows claim batch from `current_flow_id` when lifecycle events suppressed | `trace_listener.py:238-248,291-298,822-854` |
| Session deferral | Conversational/multi-turn flows defer batch finalization via `defer_session_finalization` + flow contextvar | `trace_listener.py:781-788`; `flow/runtime/__init__.py:2567` restores deferred scope |
| Crash safety | Signal handlers flush/finalize the batch; atexit shutdown flushes provider (5s timeout) | `trace_listener.py:764-772`; `telemetry/telemetry.py:193-265` |
| Cross-process propagation | None found: no `traceparent`/propagator inject-extract anywhere in `lib/`; A2A headers are auth-only; searches for W3C propagation returned nothing | `lib/crewai/src/crewai/a2a/utils/delegation.py:477`; negative search across `lib/**` |
| In-process async propagation | Contextvars copied into thread pool for handlers and into new threads for async tasks, preserving parent linkage under concurrency | `event_bus.py:513-516,632-634,715`; `lib/crewai/src/crewai/task.py:616-622` |
| Hierarchy tests | Tests assert `task_started.parent == crew_started.event_id`, `llm_call_started.parent is not None`, `method.parent == flow`, `crew.parent == method` (crew nested in flow), linear `previous_event_id` chains across multiple kickoffs, `triggered_by` for router/parallel/chained listeners | `lib/crewai/tests/events/test_event_ordering.py:173-201,228-256,292-354,463-520,525-632,961-1048` |
| Scope-stack unit tests | Push/pop nesting, depth limit raising, mismatch/empty-pop behavior, all ending events have valid pairs, triggering-id scopes | `lib/crewai/tests/events/test_event_context.py:32-185` |
| Telemetry opt-out | `OTEL_SDK_DISABLED`, `CREWAI_DISABLE_TELEMETRY`, `CREWAI_DISABLE_TRACKING` env gates checked on every operation | `lib/crewai/src/crewai/telemetry/telemetry.py:160-171` |

## Answers to Dimension Questions

**1. Is there a single coherent trace tree?**
Yes, within one execution session, via the event-trace system — not via OpenTelemetry spans. Each event gets a UUID `event_id`, an automatic `parent_event_id` from the scope stack, a `previous_event_id` linear chain, and a monotonic `emission_sequence` (`base_events.py:82-87`, `event_bus.py:545-567`), sorted at finalize time (`trace_batch_manager.py:348-356`). The separately-exported OTel spans are deliberately flat root spans with no parent linkage (`telemetry/telemetry.py:296,527,563`), so if you only look at the OTel output you get disconnected points, not a tree. The two systems also target different backends (OTel → `telemetry.crewai.com`; event tree → AMP REST API), so they cannot be merged downstream without custom glue.

**2. Are all execution steps represented?**
Broadly yes for the major steps: crews, flows/methods, agents (incl. LiteAgents), tasks, LLM calls, tools, guardrails, memory operations, knowledge retrieval, reasoning, skills, MCP tool executions, A2A delegations (`trace_listener.py:231-759`). Notable gaps found in evidence: `LLMStreamChunkEvent`s are dispatched on the bus (`event_bus.py:52,629-630`) but have no trace-listener handler, so token-level streaming detail is not collected (reasonable for volume, but chunk-level timing is lost); human-input pauses emit `FlowPaused`-style lifecycle only for flows (`event_context.py:273`); eval-style test runs get product-telemetry spans (`individual_test_result_span`, `telemetry/telemetry.py:696-725`) whose presence in the event-tree batch depends on the generic crew/train/test events registered in `event_context.py:245-246,279-282`.

**3. Do handoffs and subagent calls nest correctly?**
In-process, yes, with explicit engineering around it. Delegation tools and A2A delegation wrap their work in started/completed event pairs, so delegated work nests under the delegating task's scope (`a2a/utils/delegation.py:484-503`; pair registration `event_context.py:261-264,311-316`). Crews kicked off inside flow methods parent to the method event — asserted directly by `test_crew_parent_is_method` (`tests/events/test_event_ordering.py:325-354`). The subtle cases are owned explicitly rather than left implicit: a flow starting while a batch exists does not re-claim ownership, because reclaiming would let the nested flow finalize the parent's batch prematurely (`trace_listener.py:240-248` comment and code); a crew inside a flow never claims the flow's batch (`trace_listener.py:293-298`); suppressed-lifecycle internal flows claim the batch from `current_flow_id` so their LLM/tool events don't fall into a phantom crew batch (`trace_listener.py:822-854`). What does NOT nest: anything crossing a process boundary — a delegated-to remote A2A agent produces its own separate trace, unlinked to the caller's.

**4. Can you follow a request from start to finish?**
Within one process, one kickoff/session: yes. Enable tracing (opt-in via `tracing=True`/env/consent), and the finalized batch yields an AMP URL where crew → task → agent → LLM/tool/guardrail steps appear as a browsable timeline (`trace_batch_manager.py:420-455`, docs `tracing.mdx:151-155`). Checkpoints preserve and restore the event chain so resumed runs continue the same lineage (`crew.py:573-601`). Limitations: (a) the view is vendor-hosted; events are dropped with a log line if the send fails (`trace_batch_manager.py:308-319`); (b) concurrent kickoffs in a single process merge into whichever batch claimed first (`trace_listener.py:246-247,295-297` — correct for nesting, lossy for genuinely parallel unrelated runs); (c) no end-to-end follow across processes (A2A/MCP/subprocess tools).

## Architectural Decisions

- **Event-sourced run tree instead of OTel span tree.** The hierarchy is built from typed domain events with automatic parent/sequence stamping (`event_bus.py:537-570`) rather than span contexts. This decouples tracing from OpenTelemetry and lets the same events feed console formatting, checkpoints/replay, and the trace batch.
- **Deliberately non-global tracer provider.** `set_tracer()` documents why installing a global `TracerProvider` was reverted — it hijacked host-app library spans — and spans are now created from the private provider (`telemetry/telemetry.py:173-191`), with a regression test suite (`tests/telemetry/test_tracer_isolation.py`).
- **Scope-stack pairing as the parenting rule.** Start/end event types are enumerated in frozensets with a validity map and configurable mismatch policy (`event_context.py:240-371,12-26`), making parentage deterministic and testable rather than timestamp-heuristic.
- **Single-session batch with ownership rules.** One batch per top-level execution; nested flows/crews defer to the owner (`trace_listener.py:238-248,291-298`) — simple, but couples unrelated concurrent executions in-process.
- **Vendor-backend export with ephemeral fallback.** Authenticated users get durable batches; anonymous users (or auth failures) fall back to ephemeral traces with an access-code link (`trace_batch_manager.py:171-218,427-431`). There is no user-configurable exporter endpoint for the event tree.
- **Consent-first collection.** Tracing is opt-in (explicit flag, env var, or first-run prompt with decline persistence) and separate from always-on anonymous telemetry (`utils.py:108-137,444-464`).

## Notable Patterns

- **Automatic context enrichment at emit time:** the bus mutates each event once in `_prepare_event` (`event_bus.py:537-570`) so emitters never wire up parents manually; emitters like `base_llm._emit_call_started_event` just construct the event (`llms/base_llm.py:592-613`).
- **Contextvars as the spine:** emission counter, last-event id, triggering-event id, scope stack, flow id, and tracing-enabled flag are all contextvars, copied into thread-pool workers and spawned threads to keep the tree intact under async/parallel execution (`base_events.py:13-30`, `task.py:616-622`, `event_bus.py:513-515`).
- **Checkpoint-aware continuity:** resume paths rebuild the scope stack, last-event id, and emission counter from the recorded event graph (`crew.py:573-601`, `flow/runtime/__init__.py:2567`), and `replay()` redelivers recorded events without mutating ids (`event_bus.py:673-687`).
- **Graceful-degradation wrappers everywhere:** `SafeOTLPSpanExporter` (`crewai_core/telemetry.py:66-74`), retry-then-give-up batch init (`trace_batch_manager.py:175-205`), mark-failed on finalize errors (`trace_batch_manager.py:240-247,363-367`), signal-driven flush (`trace_listener.py:764-772`).
- **Privacy shaping at the source:** complex events get curated projections instead of raw dumps (`trace_listener.py:959-1017`); error telemetry records exception class names only (`telemetry/telemetry.py:1070-1090`).

## Tradeoffs

- **Two tracing systems, one story.** Product telemetry (flat OTel) and run-tree tracing coexist with different backends, enablement flags, and semantics. The split is documented in code comments but is a real cognitive and integration cost for anyone wanting both trees in one observability stack.
- **Deterministic parenting vs emitter discipline.** Correctness of the tree depends on every start event having exactly one matching end event emitted in the right context; the framework mitigates with pairing warnings and depth limits (`event_context.py:176-223`), but a missed end event silently flattens that subtree (WARN default).
- **Simplicity of the single batch vs concurrency.** One global `TraceBatchManager` keeps nested attribution trivially correct but means two independent crews kicked off concurrently in one process interleave in a single trace (`trace_listener.py:246-247,295-297`).
- **Hosted viewer vs data control.** The polished UI is AMP-only; self-hosting requires reimplementing the batch protocol (`plus_api.py:408-488`), while the OTel side exports to a fixed collector URL with no user-configurable endpoint (`crewai_core/telemetry.py:41`).

## Failure Modes / Edge Cases

- **Backend send failure loses the trace:** `_send_events_to_backend` returns failure and logs "Events will be lost" — no local spool or retry of event payloads (`trace_batch_manager.py:308-325,362-367`).
- **Pairing drift under suppression:** `suppress_flow_events=True` skips lifecycle events; the design compensates by claiming batches from flow context (`trace_listener.py:822-854`), but any future code path emitting action events outside a claimed context falls back to initializing a crew-typed batch (`trace_listener.py:919-932`), potentially mislabeling execution type.
- **Handler timeouts poison the batch:** if pending event handlers don't drain in 2s, the batch is marked failed server-side and the trace is abandoned (`trace_batch_manager.py:336-346`).
- **Concurrent unrelated executions merge:** parallel crews/flows in one process share the first-claimed batch; interleaved sequences remain ordered but semantically mixed (ownership guards at `trace_listener.py:246-247,295-297`).
- **No cross-process continuity:** remote A2A agents and MCP servers produce orphan traces; nothing propagates `trace_id`/span context in request headers (negative evidence: no propagator usage in `lib/`).
- **First-run auto-collection surprise factor:** first execution collects an ephemeral trace before consent, gated on interactive-terminal detection and decline persistence (`utils.py:444-464,487-501`) — mitigated but worth noting as a boundary-crossing default.

## Future Considerations

- Add W3C `traceparent` propagation for A2A delegation and MCP calls (inject on the delegating side, extract+continue on the serving side — the A2A server-side task events already exist, `a2a/utils/task.py:374`) so multi-agent systems get one linked tree.
- Provide a pluggable/local sink for the event tree (e.g., export the same `TraceBatch.to_dict()` payload to an OTLP endpoint or file) to remove vendor lockout and fix data-loss-on-send-failure.
- Unify or bridge the flat OTel telemetry with the event tree (e.g., derive telemetry counters from bus events) to eliminate the dual-system ambiguity.
- Per-execution batch isolation keyed on the runtime scope (rather than first-claim-wins) would make truly parallel runs produce separate coherent traces while preserving nested ownership.

## Questions / Gaps

- No evidence found of any local trace persistence, offline buffer, or replayable export format for the event tree beyond the in-memory `EventRecord` used for checkpoints/resume (`state/event_record.py:103-125`); searched `events/listeners/tracing/*`, `crewai_core/*`.
- No evidence found of trace sampling controls (rate/head/tail) for either system; enablement is binary (`utils.py:108-137`). Searched for `sample`, `Sampler` in `lib/` — none.
- The AMP ingestion side (how `parent_event_id`/`previous_event_id` are rendered into the visualized tree) is server-side and out of reach of this source; the client contract is visible in `TraceEvent.to_dict()` (`listeners/tracing/types.py:23-24`) and the payload typing in `plus_api.py:93-97`, but rendering fidelity could not be verified from this repository.
- Whether `LiteAgent` execution events nest identically to full-agent events was verified only at handler-registration level (`trace_listener.py:361-377`); no dedicated ordering test was found for the LiteAgent path.

---

Generated by Dimension 10.01 (Span Hierarchy and Run Tree) against `crewai`.
