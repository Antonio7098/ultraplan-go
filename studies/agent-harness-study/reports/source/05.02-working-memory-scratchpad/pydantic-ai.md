# Source Analysis: pydantic-ai

## 05.02 Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, pydantic-graph, async agent loop; provider adapters for OpenAI/Anthropic/Google/etc.) |
| Analyzed | 2026-08-25 |

## Summary

Pydantic AI has **no dedicated, named scratchpad or todo abstraction** — a repo-wide search for `scratchpad`, `notepad`, `working memory`, and `todo` across the framework package returns no hits. Instead, working memory is realized as an explicit three-layer design:

1. **Runtime-private per-run state.** `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343`) holds the mutable working set of one run: the message history list, cumulative `RunUsage`, retry counters, step counter, run/conversation IDs, the pending-message queue, the event-stream buffer, and an MCP tool-definition cache. The model-facing view is `RunContext` (`pydantic_ai_slim/pydantic_ai/_run_context.py:60-260`), which shares those objects **by reference** into every step (`build_run_context`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2338-2378`) plus framework-derived sets (`discovered_tool_names`, `loaded_capability_ids`, private `_anchored_evidence`).

2. **Model-internal reasoning as protocol history parts.** The model's "thinking" is captured as `ThinkingPart` (`pydantic_ai_slim/pydantic_ai/messages.py:1928-1976`) inside durable message history — not a separate hidden channel — with signatures/encrypted blobs preserved for provider round-trips (`pydantic_ai_slim/pydantic_ai/models/anthropic.py:1934-1959`). Thinking parts are deliberately excluded from final result assembly (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1997-1998`), keeping working notes out of user-visible facts.

3. **A durable-by-reconstruction discipline.** Framework state that feeds future requests is *derived from* serializable history rather than kept in memory-only attributes: `discovered_tool_names`/`loaded_capability_ids` are re-parsed from history each request (`pydantic_ai_slim/pydantic_ai/_run_context.py:225-252`; `_agent_graph.py:2396-2398`), compaction resets them at boundaries via `post_compaction_window` (`pydantic_ai_slim/pydantic_ai/messages.py:2774-2814`), and cross-process durability is handled by explicit serialization contracts (`TemporalRunContext`, `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:64-94`).

There is also a mid-run injection queue (`RunContext.enqueue`, `pydantic_ai_slim/pydantic_ai/_run_context.py:413-465`) that acts as a runtime-private scratchpad which is *promoted* into conversation history when drained (`pydantic_ai_slim/pydantic_ai/capabilities/_pending_messages.py:84-111`). Long-term memory does not exist in the framework at all; migration guidance explicitly assigns it to application code (`pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/SEMANTIC-GAPS.md:113-120`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

- The boundary between model-visible history, runtime-private working state, and user-owned deps is precisely drawn and heavily documented in-code (`_agent_graph.py:413-421` states a shared-identity invariant with failure-mode commentary).
- Derived working state is reconstructed from durable history, so it survives failover and restarts by construction rather than by developer diligence (`messages.py:2774-2795`).
- Sensitive-content handling is deliberate: raw chain-of-thought is stored in `provider_details['raw_content']`, not `content`, following OpenAI's guidance not to show raw reasoning to users (`models/openai.py:2317-2344`, documented at `docs/capabilities/thinking.md:97`); client-submitted history is sanitized (`messages.py:2953-3028`).
- It stops short of 8–10 because there is no first-class scratchpad/todo surface (apps must build one over deps/tools), thinking visibility to UI consumers is all-or-nothing with no harness-level redaction knob, and the shared-mutable-state design depends on identity discipline that is documented but not enforced by types.

## Evidence Collected

Every entry includes file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Per-run working state | `GraphAgentState`: message_history, usage, output_retries_used, run_step, pending_messages queue, event_stream_buffer, mcp_tool_defs_cache | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343` |
| Run-scoped counters | `check_incomplete_tool_call` guards truncated tool-call args; `consume_output_retry` enforces retry budget | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359`, `361-378` |
| RunContext facade | `RunContext` dataclass: deps, messages, usage, retries, tool_call_id/retry/max_retries/run_step | `pydantic_ai_slim/pydantic_ai/_run_context.py:60-124` |
| Private runtime fields | `_cancellation`, `_event_stream_buffer`, `_mcp_tool_defs_cache` marked "Private implementation detail — not part of the public API" | `pydantic_ai_slim/pydantic_ai/_run_context.py:157-185` |
| History-derived working state | `loaded_capability_ids` ("Derived from message history ... before each request"), `discovered_tool_names` ("Raw evidence ... Populated during run preparation from message history") | `pydantic_ai_slim/pydantic_ai/_run_context.py:225-234`, `242-252` |
| Dispatch-time anchored evidence | `AnchoredEvidence` widens availability checks for calls the serving provider already saw, without mutating forward-looking state | `pydantic_ai_slim/pydantic_ai/_run_context.py:38-57`, `254-260` |
| Shared-by-reference invariant | Comment: both sets "shared by reference into every RunContext this run ... only ever mutated in place"; reassigning "would silently break in-step tool reveals" | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:413-421` |
| Context construction | `build_run_context` passes `ctx.state.message_history`, usage, pending_messages, event buffer, MCP cache by reference; only `validation_context` may be `replace`d | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2338-2378` |
| Model reasoning capture | `ThinkingPart` dataclass with `content`, `signature`, `provider_name`, `provider_details` | `pydantic_ai_slim/pydantic_ai/messages.py:1928-1976` |
| Tag-splitting fallback | `split_content_into_text_and_thinking` converts `<think>`-tagged text into ThinkingParts for models without native reasoning channels | `pydantic_ai_slim/pydantic_ai/_thinking_part.py:6-31` |
| Streaming assembly | Parts manager creates/appends ThinkingParts from deltas during streaming | `pydantic_ai_slim/pydantic_ai/_parts_manager.py:261-271` |
| Working notes ≠ facts | Final-output assembly skips ThinkingPart entirely; text before a native tool call is reset because it is "essentially thoughts" | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1982-1998` |
| Thinking-only responses | `is_thinking_only` detection avoids pointless retries when models emit only thinking after completing work via tools | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:1915-1952` |
| Provider round-trip | Anthropic adapter re-emits signed/redacted thinking blocks on later turns; unsigned thinking downgrades to tagged text | `pydantic_ai_slim/pydantic_ai/models/anthropic.py:1934-1959` (creation at `1294`) |
| Raw CoT containment | gpt-oss raw reasoning stored in `provider_details['raw_content']`, not `content` | `pydantic_ai_slim/pydantic_ai/models/openai.py:2317-2344`; rationale at `docs/capabilities/thinking.md:97` |
| Mid-run injection queue | `RunContext.enqueue(...)` appends to `pending_messages`; drained between graph nodes only | `pydantic_ai_slim/pydantic_ai/_run_context.py:413-465` |
| Queue drain semantics | `PendingMessageDrainCapability` drains `'asap'` before each request and redirects end-of-run for `'when_idle'`; drained messages appended to both request and `ctx.messages` | `pydantic_ai_slim/pydantic_ai/capabilities/_pending_messages.py:59-178` |
| Compaction boundary | `CompactionPart` summarizes prior history; `post_compaction_window` defines what the model effectively works from afterwards | `pydantic_ai_slim/pydantic_ai/messages.py:1989-2036`, `2774-2814` |
| Provenance stamp | `STANDING_PROMPT_PLANTED_KEY` marks self-produced compaction items so system-prompt retention can be trusted; foreign items get the prompt re-inserted | `pydantic_ai_slim/pydantic_ai/messages.py:1979-1985` |
| Untrusted-history sanitization | `sanitize_messages` strips client-supplied system prompts, unsafe URLs, dangling tool calls, optional compaction parts | `pydantic_ai_slim/pydantic_ai/messages.py:2953-3028` |
| Persistence contract | `ModelMessagesTypeAdapter` serializes full history (including app-only metadata) for storage/reload across runs | `pydantic_ai_slim/pydantic_ai/messages.py:2768-2771`; docs at `docs/message-history.md:322-379` |
| User exposure of history | `all_messages()` / `new_messages()` expose live history (run handle and final result) | `pydantic_ai_slim/pydantic_ai/run.py:163-191`; `pydantic_ai_slim/pydantic_ai/result.py:527-544` |
| UI visibility of thinking | UI event streams dispatch `ThinkingPart`/delta to `handle_thinking_start/delta/end` hooks (AG-UI, Vercel AI adapters) | `pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:509-603`, `711-737`; `ui/ag_ui/_thinking_0_13.py:33-68` |
| Cross-process durability | `TemporalRunContext` whitelists serializable fields; reading excluded fields raises `UserError` instead of silently returning defaults | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:64-94` |
| Usage accounting | `RunUsage` accumulates requests/tool_calls/tokens/cost per run; `UsageLimits` enforces budget ceilings including per-request input-token caps | `pydantic_ai_slim/pydantic_ai/usage.py:338-390`, `418-457` |
| Tests: enqueue queue | ~20 enqueue tests incl. delivery events, priority redirect, serialization round-trip, no-live-queue error | `tests/test_capabilities.py:16719-17804` |
| Tests: compaction window | `test_post_compaction_window_returns_history_unchanged_without_compaction`, `..._slices_at_the_latest_compaction_part`, minimal-sequence case | `tests/test_messages.py:2788-2829` |
| Tests: boundary resets evidence | `test_compaction_inside_serving_response_does_not_reset_tool_evidence`; capability tools refused again after compaction | `tests/test_tool_availability.py:151`, `300` |
| Tests: reasoning wire | `test_reasoning_wire_contract` verifies per-provider disable signals and effort mapping | `tests/test_thinking_wire_contract.py:274-290` |
| Tests: thinking parsing | Delta application, signature preservation, tag splitting | `tests/test_thinking_part.py:76-110` |
| Docs: persistence & trust | Storing/loading messages to JSON; loading untrusted history; client-supplied history trust boundary | `docs/message-history.md:322-379`, `381-403`, `404-417` |
| Docs: compaction strategy | Provider-native + model-agnostic compaction; derived state should be recomputed from `post_compaction_window`, "not remembered in instance attributes" | `docs/capabilities/compaction.md:3-16`, `28-37` |
| No long-term memory | Migration skill assigns long-term/cross-thread memory to "an explicit repository/service in dependencies" | `pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/SEMANTIC-GAPS.md:113-120`; `CONCEPT-MAPPING.md:156` |

## Answers to Dimension Questions

1. **Does the agent keep private task state?**
   Yes, but it is structured rather than a free-form scratchpad. Runtime-private state lives on `GraphAgentState` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:298-343`): pending-message queue, event buffer, retry counters, MCP cache — none of which are sent to the model. Framework-derived sets like `discovered_tool_names` are described as "raw evidence, not a verdict" and are explicitly marked "Managed by the framework: safe to read, but don't mutate it directly" (`pydantic_ai_slim/pydantic_ai/_run_context.py:242-252`). Application task state has a designated home in user deps injected into every hook/tool (`pydantic_ai_slim/pydantic_ai/_run_context.py:64-65`; pattern documented at `docs/dependencies.md:1-5`). There is no built-in todo/plan tool: searches for `scratchpad|notepad|working memory|todo` across `pydantic_ai_slim/pydantic_ai` return nothing; a "Planning" page exists in docs navigation (`docs/navigation.yml:310-313`) with `source: "harness"` but the file is not part of this repository — planning belongs to the separate Pydantic AI Harness product.

2. **Is it durable?**
   Two tiers. Within a process, working state is per-run and dies with the run; the *model-relevant* subset is durable by reconstruction: capability-load and tool-discovery state is re-parsed from serializable message history before each request (`pydantic_ai_slim/pydantic_ai/_run_context.py:228-234`; `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2396-2398`), and full history round-trips through `ModelMessagesTypeAdapter` (`pydantic_ai_slim/pydantic_ai/messages.py:2768-2771`). Across process boundaries, durability is opt-in and explicit: Temporal activities receive a whitelisted, serialized `TemporalRunContext` where absent fields raise `UserError` rather than masquerading as empty state (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:64-94`). The `_mcp_tool_defs_cache` docstring shows the replay-determinism concern directly: it lives on the run "recreated for each agent run and reconstructed identically on durable replay/recovery" (`pydantic_ai_slim/pydantic_ai/_run_context.py:176-185`).

3. **Is it exposed to users?**
   Mostly yes — by design. Message history (including every `ThinkingPart`) is readable via `all_messages()`/`new_messages()` (`pydantic_ai_slim/pydantic_ai/run.py:163-191`) and streamed to frontends through UI adapters' `handle_thinking_*` hooks (`pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:509-603`, `711-737`). Genuinely private items are the fields explicitly labeled "Private implementation detail — not part of the public API" (`pydantic_ai_slim/pydantic_ai/_run_context.py:158`, `167`, `177`) and the internal graph state object itself. One nuance: raw chain-of-thought from some models is deliberately *not* put in the user-visible `content` field — it is parked in `provider_details['raw_content']` following OpenAI's guidance against showing raw reasoning (`pydantic_ai_slim/pydantic_ai/models/openai.py:2320-2324`; `docs/capabilities/thinking.md:97`) — but it remains programmatically accessible in the same object, so "exposed" holds at the API level even where UI presentation is discouraged.

4. **Does it pollute long-term memory?**
   There is no framework long-term memory to pollute: cross-thread facts are assigned to application-owned services in migration guidance (`pydantic_ai_slim/pydantic_ai/.agents/skills/migrating-langchain-to-pydantic-ai/references/SEMANTIC-GAPS.md:113-120`). Within the framework's own scope the pollution risk runs the other way — long conversations accumulating stale working notes — and is managed by compaction: `CompactionPart` replaces pre-boundary history and resets prospective derived state (tool reveals, capability loads) so they are re-advertised afterwards (`docs/capabilities/compaction.md:16`; `pydantic_ai_slim/pydantic_ai/messages.py:2774-2814`). Capability authors are told to derive anything the model must have seen from `post_compaction_window` "rather than remembering it in instance attributes, so it self-heals when compaction replaces the history that carried it" (`pydantic_ai_slim/pydantic_ai/messages.py:2782-2787`).

5. **Can it be audited?**
   Yes, strongly. All conversation-visible working content is ordinary typed dataclasses inspectable via `all_messages()` and JSON-serializable (`pydantic_ai_slim/pydantic_ai/messages.py:2768-2771`); streaming events (including enqueued-message deliveries and thinking deltas) flow through a single observable event stream (`pydantic_ai_slim/pydantic_ai/ui/_event_stream.py:509-603`); OTel instrumentation maps messages — including a `ThinkingPart` shape — onto GenAI semantic conventions with content gated behind instrumentation settings (`pydantic_ai_slim/pydantic_ai/_otel_messages.py:100-105`). Trust-sensitive provenance is auditable too: client-supplied histories pass through `sanitize_messages`, whose stripping behavior (system prompts, URL schemes, dangling tool calls, compaction stamps) is enumerated and tested (`pydantic_ai_slim/pydantic_ai/messages.py:2953-3028`; `tests/test_sanitize_messages.py`).

## Architectural Decisions

- **History as the source of truth for derived working state.** Rather than shadowing model-relevant facts in runtime variables, the runtime re-derives them from the durable protocol record each request (`pydantic_ai_slim/pydantic_ai/_run_context.py:228-252`). This makes working memory crash-safe and failover-safe (the conservative provider-agnostic window keeps state valid across `FallbackModel` switches, `messages.py:2789-2800`) at the cost of parse work per request.
- **Working notes live inside the same history as facts, distinguished by type.** `ThinkingPart` vs `TextPart` (`pydantic_ai_slim/pydantic_ai/messages.py:1917`, `1969`) lets one durable record carry both while downstream consumers pick: final-result assembly ignores thinking (`_agent_graph.py:1997-1998`), providers round-trip it with signatures (`models/anthropic.py:1934-1959`), UIs render it.
- **Shared-mutable-state by reference with a written invariant.** `build_run_context` shares queues/sets/buffers by identity across `replace()` shallow copies, with an explicit warning that reassignment would "silently break in-step capability loads / tool reveals / message enqueues" (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2369-2377`).
- **Fail-closed serialization boundaries.** `TemporalRunContext.__getattribute__` raises `UserError` for fields that did not cross the activity boundary "so a field that didn't cross the boundary can't be mistaken for real run state" (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_run_context.py:67-93`).
- **Primitives over batteries.** No todo/scratchpad tool ships; the repo's own philosophy statement prefers "strong primitives, powerful abstractions, and general solutions" over opinionated agent-design batteries (`AGENTS.md`, Philosophy section). Apps compose deps + tools + `enqueue` to build their own.

## Notable Patterns

- **Queue-promoted scratchpad.** `enqueue()` gives tools/hooks a private list (`pending_messages`) whose contents only become model-visible conversation when the drain capability runs between graph nodes (`pydantic_ai_slim/pydantic_ai/_run_context.py:147-155`, `413-465`; `pydantic_ai_slim/pydantic_ai/capabilities/_pending_messages.py:102-110`). Priority classes (`'asap'`/`'when_idle'`) decide whether content steers the next request or extends the run at its end.
- **Anchored evidence split.** Forward-looking state (`what the next request should advertise`) is kept strictly separate from dispatch-time judgment about what the *serving* provider already saw (`AnchoredEvidence`, `pydantic_ai_slim/pydantic_ai/_run_context.py:38-57`) — a two-ledger pattern preventing both over-disclosure and false refusals.
- **Thought/text demotion at output time.** Text preceding a native tool call is discarded from the result because it is "essentially thoughts" (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1989-1993`), and thinking-only responses avoid triggering retries (`1915-1952`).
- **Tag-based reasoning normalization.** Models without native reasoning channels get `<think>` text split into proper ThinkingParts (`pydantic_ai_slim/pydantic_ai/_thinking_part.py:6-31`), normalizing the working-notes representation across 15+ providers.
- **Provenance stamping for self-produced summaries.** Only compaction items minted by the framework's own compact call carry `STANDING_PROMPT_PLANTED_KEY`; everything else gets the standing prompt re-inserted (`pydantic_ai_slim/pydantic_ai/messages.py:1979-1985`).

## Tradeoffs

- **Reconstruction cost vs resilience:** re-parsing history for derived state each request buys failover-safety but adds per-request work and couples correctness to history fidelity — a history processor that drops reveal records silently hides tools again (documented in `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/ON-DEMAND-CAPABILITIES.md:125`).
- **Identity-based sharing vs safety:** sharing mutable sets/lists by reference across `replace()` copies is efficient and simple, but correctness relies on contributors honoring a comment-documented invariant (`_agent_graph.py:413-421`) rather than type enforcement.
- **Transparency vs leakage:** exposing ThinkingParts everywhere maximizes auditability, but any frontend consumer receives full reasoning content; the only suppression mechanisms are provider-native redaction (`redacted_thinking`, `models/anthropic.py:1938-1944`) or the OpenAI raw-content convention (`openai.py:2320-2324`) — there is no harness-level redaction filter.
- **Explicitness vs ergonomics:** apps wanting a plan/todo scratchpad must wire deps mutation, tools, and history themselves; the framework provides no ready-made pattern, though `enqueue` covers the injection half.

## Failure Modes / Edge Cases

- **Truncated working state:** a response cut off mid-tool-call raises `IncompleteToolCall` instead of feeding garbage args onward (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:345-359`).
- **Interrupted streams produce unusable signatures:** empty-signature thinking is never sent as a thinking block — the adapter falls back to tagged text to avoid a provider 400 (`pydantic_ai_slim/pydantic_ai/models/anthropic.py:1935-1937`).
- **Stale load records without reveals:** history processing that drops `ToolAvailabilityDeltaPart`s while keeping load pairs would strand deferred tools permanently; `is_tool_available` treats capability loads as bundled reveals to prevent exactly this (`pydantic_ai_slim/pydantic_ai/_run_context.py:377-404`).
- **Client-forged history:** a client-supplied `CompactionPart` could hide server-side history from the model; sanitization strips compaction parts by default in mixed-trust scenarios and never trusts them to stand in for the system prompt (`pydantic_ai_slim/pydantic_ai/messages.py:2995-3005`; `docs/message-history.md:381-403`).
- **Cross-run state bleed via shared instances:** capability instances default to reuse across runs, so instance-field mutations leak between sequential and concurrent runs unless authors override `for_run()` — spelled out with required patterns in `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:116` context and `pydantic_ai_slim/pydantic_ai/.agents/skills/building-pydantic-ai-agents/references/CAPABILITIES-AND-HOOKS.md:123-169`.
- **Budget exhaustion mid-thinking:** token-limit failures during thinking are detected and surfaced distinctly instead of looping retries (`_agent_graph.py:1927-1931`).

## Future Considerations

- A harness-level redaction/visibility policy for `ThinkingPart` (e.g., strip-before-UI, keep-for-provider) would close the gap between "auditable server-side" and "shown to every frontend."
- Enforcing the shared-identity invariant structurally (e.g., frozen views or ownership types) would convert a documented discipline into a checked one.
- First-class plan/task-list state (as the separate Harness product's "Planning" page suggests exists outside this repo, `docs/navigation.yml:310-313`) could standardize what apps currently hand-roll over deps/tools.

## Questions / Gaps

- **No built-in todo/plan/scratchpad tool found.** Searches covered `scratchpad`, `notepad`, `working memory`, `todo`, `planner`, and `update_plan` across `pydantic_ai_slim/pydantic_ai`, `clai`, and `docs`. The navigation entry for Planning (`source: "harness"`, `docs/navigation.yml:310-313`) references a page not present in this tree; whether the Harness implementation matches this analysis could not be verified from this source alone.
- **OTel content gating details:** `trace_include_content` gates message content in spans (`pydantic_ai_slim/pydantic_ai/_run_context.py:93-94`), but the exact per-part redaction path for thinking content under instrumentation v4+ was not traced end-to-end in this study (entry point: `pydantic_ai_slim/pydantic_ai/_otel_messages.py:100-105`).
- **Realtime-session working state** (transcript accumulation, `SpeechPart` handling at `pydantic_ai_slim/pydantic_ai/messages.py:2077-2079`) was treated as out of scope beyond noting it exists; a realtime-specific pass would be needed for session-scoped scratch semantics.

---

Generated by `dimensions/05.02-working-memory-scratchpad.md` against `pydantic-ai`.
