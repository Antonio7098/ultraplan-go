# Source Analysis: openai-agents-sdk

## Dimension 05.02: Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (asyncio SDK; pydantic, sqlite3, OpenAI Responses/Chat Completions APIs) |
| Analyzed | 2026-08-26 |

All citations below are workspace-relative to the studied source root `studies/agent-harness-study/sources/openai-agents-sdk/` and use the form `path:NN`.

## Summary

The SDK has **no single first-class "scratchpad" object**. A repo-wide search for `scratchpad`, `scratch_pad`, `working_memory`, `notes_to_self`, and `private_state` returned no matches; the only "todo"-named construct is a passthrough thread-item type in the experimental Codex extension (`src/agents/extensions/experimental/codex/items.py:97-107`). Instead, working memory is decomposed into several explicit, typed layers with different lifetimes and visibility rules:

1. **Run context (private, per-run)** — `RunContextWrapper` carries a user dependency object, token usage, per-turn input, approval decisions, and tool-invocation lifecycle records. Its docstring states plainly: *"Contexts are not passed to the LLM"* (`src/agents/run_context.py:72-81`).
2. **Serializable run state (durable snapshot)** — `RunState` is the pause/resume boundary for human-in-the-loop flows, storing filtered model-view items, full session items, pending staged input, tool-use tracker snapshots, trace state, and sandbox resume payloads under a versioned schema (`src/agents/run_state.py:748-851`, schema version `1.17` at `src/agents/run_state.py:182`).
3. **Ephemeral nested-agent scratchpad** — module-level, scope-keyed maps that hold nested agent-as-tool run results between the tool call and its consumption, cleaned by weakref GC hooks (`src/agents/agent_tool_state.py:35-48`, `131-142`).
4. **Model reasoning (provider-managed thinking)** — reasoning items with opaque encrypted content are captured as first-class items and replayed into next-turn model input under a configurable ID policy ("preserve"/"omit", `src/agents/run_config.py:459-464`; application at `src/agents/run_internal/items.py:726-749`).
5. **Conversation memory (sessions)** — a `Session` protocol with SQLite/Conversations/encrypted/compacting implementations persists history across runs (`src/agents/memory/session.py:15-56`), with server-side compaction rewriting long histories (`src/agents/memory/openai_responses_compaction_session.py:82-88`).
6. **Handoff transcript summaries** — an opt-in beta compresses prior transcripts into numbered summary messages wrapped in `<CONVERSATION HISTORY>` markers when control passes to another agent (`src/agents/run_config.py:374-381`; builder at `src/agents/handoffs/history.py:376-398`).

The design principle is separation of *model-visible* conversation state from *runtime-private* decision state: hidden state (approvals, invocation fingerprints, pending input, nested checkpoints) lives behind underscore-prefixed fields and internal modules, while users see a public item stream (`new_items`) that does include model reasoning items.

## Rating

**8 / 10** — Clear layered working-memory model with explicit interfaces, extensive tests, and real operational safeguards.

Why this score:

- **Explicit interfaces**: `Session` protocol + ABC (`src/agents/memory/session.py:15-106`), versioned `RunState` schema with per-version change summaries (`src/agents/run_state.py:186-217`) and fail-fast forward compatibility (`src/agents/run_state.py:175-182`).
- **Tests**: dedicated suites cover session memory (`tests/memory/test_session.py:91`), compaction candidate selection (`tests/memory/test_openai_responses_compaction_session.py:67-96`), ephemeral agent-tool state including weakref GC drop (`tests/test_agent_tool_state.py:103`) and scope isolation (`tests/test_agent_tool_state.py:89`), pending-input ordering and round-trips (`tests/test_run_state_pending_input.py:93`), reasoning persistence sanitization (`tests/memory/test_session_persistence_sanitize.py:129-157`), plus a serialized-schema compatibility corpus (`tests/test_run_state_compatibility_corpus.py`).
- **Operational safeguards**: transactional compaction replacement with rollback-on-cancellation restore (`src/agents/memory/openai_responses_compaction_session.py:271-299`, `316-336`); deep-copy isolation of usage/approvals when checkpointing (`src/agents/run_context.py:117-131`); provider metadata stripped before persistence (`src/agents/run_internal/session_persistence.py:910-930`).
- Not 9–10 because: there is no unified, documented scratchpad API for agent authors to stash intermediate notes as a supported concept (the user `context` object is freeform and explicitly unmanaged, `src/agents/run_state.py:756-762` warns custom contexts may need bespoke serializers or fail to round-trip); the nested-agent scratchpad relies on module-level global dicts (`src/agents/agent_tool_state.py:35-48`); both handoff-history summarization and responses-compaction are beta/OpenAI-only (`src/agents/run_config.py:374-381`, `src/agents/memory/openai_responses_compaction_session.py:83-88`).

## Evidence Collected

Every entry cites a file path with line numbers relative to `studies/agent-harness-study/sources/openai-agents-sdk/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Private context contract | Docstring: "Contexts are not passed to the LLM. They're a way to pass dependencies and data to code you implement" | `src/agents/run_context.py:72-81` |
| Per-run private fields | `usage`, `turn_input`, `_approvals`, `_tool_invocations` on the wrapper | `src/agents/run_context.py:83-94` |
| Per-turn working input reset | `context_wrapper.turn_input = list(turn_input)` set fresh each turn (empty when input is empty) | `src/agents/run_internal/run_loop.py:2071-2074`, `2403-2406`; also `src/agents/run_internal/turn_resolution.py:1273-1275` |
| Tool invocation ledger | `_tool_invocation_status` registers canonical invocation identity (type, call id, fingerprint) and raises on call-ID reuse | `src/agents/run_context.py:304-355` |
| Approval decisions as hidden state | `approve_tool`/`reject_tool` mutate `_approvals`; sticky vs per-call scoping | `src/agents/run_context.py:1043-1063`, `888-1041` |
| Checkpoint isolation | `_copy_for_run_state` deep-copies usage/approvals/invocations so resumed snapshots don't share mutable state | `src/agents/run_context.py:117-131` |
| Durable run snapshot | `RunState` dataclass: `_generated_items` (filtered model view), `_session_items` (full history), `_pending_input`, `_tool_use_tracker_snapshot`, `_trace_state`, `_sandbox` | `src/agents/run_state.py:764-851` (fields at 776-789, 835-848) |
| Versioned snapshot schema | `CURRENT_SCHEMA_VERSION = "1.17"`; chronological `SCHEMA_VERSION_SUMMARIES`; forward-compat is fail-fast | `src/agents/run_state.py:175-232` |
| Pending input staging | `add_input()` stages input for next resumed model call, rejects terminal states and exhausted turns; `clear_pending_input()` empties it | `src/agents/run_state.py:941-983` |
| Conservative context serialization | Mapping contexts round-trip directly; custom contexts may need serializer/deserializer or snapshot emits warnings + rebuild metadata | `src/agents/run_state.py:750-762` |
| Ephemeral nested-agent scratchpad | Module-level `_agent_tool_run_results_by_obj/_by_signature` maps keyed by `(scope_id, id(tool_call))`; comment "Ephemeral maps linking tool call objects to nested agent results within the same run" | `src/agents/agent_tool_state.py:33-48` |
| Scratchpad leak prevention | Weakref callback drops cached results when the tool-call object is garbage collected; consume/drop APIs | `src/agents/agent_tool_state.py:131-142`, `208-276` |
| Scope isolation of scratchpad | `get/set_agent_tool_state_scope` attach a private scope id to context wrappers so restored states don't collide | `src/agents/agent_tool_state.py:51-70`; consumed by `src/agents/run_state.py:893-895` |
| Run-scoped tool-use memory | `AgentToolUseTracker` records which tools each agent used "to support model_settings resets"; serializes into RunState | `src/agents/run_internal/tool_use_tracker.py:53-125`, `128-166`; field at `src/agents/run_state.py:838-839` |
| Model reasoning as items | `ReasoningItem` wraps `ResponseReasoningItem`; streaming handler stores opaque `encrypted_content` signature | `src/agents/items.py:492-499`; `src/agents/models/chatcmpl_stream_handler.py:558` |
| Reasoning replay policy | `reasoning_item_id_policy` ("preserve"/"omit") controls whether reasoning IDs are stripped from next-turn model input | `src/agents/run_config.py:459-464`; applied at `src/agents/run_internal/items.py:726-749`; persisted via `src/agents/run_state.py:811`, `2088-2090` |
| Session protocol (cross-run memory) | `Session` protocol: `get_items`/`add_items`/`pop_item`/`clear_session`; optional opt-in `wrapper` parameter for context-aware sessions | `src/agents/memory/session.py:15-56`, `155-212` |
| SQLite session store | Default `db_path=":memory:"` "lost when the process ends"; file-backed otherwise; `clear_session` deletes all rows for the session | `src/agents/memory/sqlite_session.py:42-48`, `435-450` |
| Compaction (history rewrite) | `OpenAIResponsesCompationSession` triggers `responses.compact` when ≥10 candidate items; user messages and prior compaction items excluded from candidates | `src/agents/memory/openai_responses_compaction_session.py:28`, `34-60`, `82-88` |
| Compaction atomicity | clear→add treated as one replacement transaction; failure/cancellation restores previous history even under repeated cancellation | `src/agents/memory/openai_responses_compaction_session.py:271-299`, `316-336` |
| Compaction item type | `CompactionItem` represents compaction output and replays verbatim as model input; Chat Completions path rejects it | `src/agents/items.py:532-540`; `src/agents/models/chatcmpl_converter.py:956-960` |
| Encrypted session store (extension) | `EncryptedSession`: Fernet encryption with HKDF per-session keys and TTL; expired tokens silently skipped on read | `src/agents/extensions/memory/encrypt_session.py:100-135`, `163-172` |
| Handoff transcript summarization | Opt-in beta `nest_handoff_history` compacts prior run history into assistant summary segments; mapper hook exposed | `src/agents/run_config.py:374-389` |
| Summary message format | Numbered transcript lines inside `<CONVERSATION HISTORY>` markers, prefixed "For context, here is the conversation so far…", emitted as a synthetic assistant message | `src/agents/handoffs/history.py:29-34`, `376-398` |
| What gets summarized vs forwarded | `_SUMMARY_ONLY_INPUT_TYPES = {function_call, function_call_output, reasoning}` — tool traffic and reasoning summarized, not forwarded verbatim | `src/agents/handoffs/history.py:44-49` |
| Persistence sanitization | Provider `id`s stripped except required types; reasoning keeps `id`/`encrypted_content`; `provider_data` always stripped before persistence | `src/agents/run_internal/session_persistence.py:910-942` |
| Public visibility of items | `RunResultBase.new_items` is public and includes reasoning/tool items; `to_input_list()` offers `preserve_all` vs `normalized` views | `src/agents/result.py:307-338`, `230-290` |
| Trace leakage control | `trace_include_sensitive_data` defaults true via env `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA`; when false, spans still emit but without sensitive data | `src/agents/run_config.py:53-56`, `404-410`; `src/agents/tracing/model_tracing.py:6-14` |
| Voice pipeline defaults | `trace_include_sensitive_data=True` and `trace_include_sensitive_audio_data=True` by default | `src/agents/voice/pipeline_config.py:26-30` |
| Todo lists (Codex extension) | `TodoItem(text, completed)` / `TodoListItem` parsed from Codex CLI JSONL stream events — surfaced plan state, not SDK-managed memory | `src/agents/extensions/experimental/codex/items.py:97-107`, `226-232` |
| Tests: sessions & compaction | Basic multi-session memory behavior; candidate selection excludes user messages/compaction items | `tests/memory/test_session.py:91`; `tests/memory/test_openai_responses_compaction_session.py:67-96` |
| Tests: scratchpad lifecycle | Scope isolation, ambiguous-signature rejection, weakref-GC drop of nested results | `tests/test_agent_tool_state.py:69-115` |
| Tests: pending input | Order preservation + JSON round-trip; unresolved approvals keep pending input until tool finishes | `tests/test_run_state_pending_input.py:93`, `286` |
| Tests: persistence hygiene | Reasoning `id`/`encrypted_content` preserved for Conversations; `provider_data` stripped | `tests/memory/test_session_persistence_sanitize.py:129-171` |

## Answers to Dimension Questions

**1. Does the agent keep private task state?**
Yes, across several layers. The runtime maintains hidden per-run state invisible to the model: approval records and canonical tool-invocation ledgers on the context wrapper (`src/agents/run_context.py:89-94`, `304-355`), staged-but-unadmitted input on `RunState._pending_input` (`src/agents/run_state.py:788-789`), the tool-use tracker (`src/agents/run_internal/tool_use_tracker.py:53-62`), and the ephemeral nested-agent result cache keyed by object identity (`src/agents/agent_tool_state.py:35-48`). None of these are sent to the LLM; the context wrapper docstring makes the model/runtime boundary explicit (`src/agents/run_context.py:76-77`). There is, however, no general-purpose named scratchpad where the agent itself can write private notes distinct from tool outputs and messages.

**2. Is it durable?**
Layered by design. Ephemeral: the nested-agent scratchpad lives only for the duration of one tool call and is dropped on consumption or GC (`src/agents/agent_tool_state.py:208-230`, `131-142`); `turn_input` is rebuilt every turn (`src/agents/run_internal/run_loop.py:2071-2074`). Run-scoped: the context wrapper survives across turns within one run and can be checkpointed (`_copy_for_run_state`, `src/agents/run_context.py:117-131`). Durable: `RunState.to_json()/from_json()` serializes the full resume boundary under a versioned schema (`src/agents/run_state.py:1796` region; version registry `186-217`), and Sessions persist history across processes (`src/agents/memory/sqlite_session.py:42-48`). Durability of the user's own context object is best-effort only — serialization is intentionally conservative and may emit warnings instead of guaranteeing round-trip (`src/agents/run_state.py:756-762`).

**3. Is it exposed to users?**
Partially, and deliberately. Users receive `new_items` including `ReasoningItem`s (model thinking, possibly encrypted-opaque content, `src/agents/items.py:492-499`) and can inspect continuation input through `to_input_list()` (`src/agents/result.py:230-290`). Approval state is queryable via public methods (`is_tool_approved`, `get_approval_status`, `src/agents/run_context.py:612-621`, `1065+`). But the runtime-private machinery (`_tool_invocations`, `_pending_input`, nested checkpoints) is underscore/internal-only, and the user context object is never shown to the model (`src/agents/run_context.py:76-77`). Traces expose inputs/outputs by default unless disabled (`src/agents/run_config.py:404-410`), which is an operator-facing exposure channel rather than an end-user one.

**4. Does it pollute long-term memory?**
Working notes do flow into durable stores by default: sessions persist the full converted item stream (including tool call/output items and reasoning where persistable) as plain JSON rows (`src/agents/memory/sqlite_session.py:263-286`), and reasoning items remain in next-turn input unless the omit policy is set (`src/agents/run_internal/items.py:726-749`). Mitigations exist: compaction rewrites accumulated non-user history into compacted items once ≥10 candidates accrue (`src/agents/memory/openai_responses_compaction_session.py:28`, `34-55`); handoff summarization replaces verbatim tool traffic and reasoning with a single numbered-transcript assistant message when enabled (`src/agents/handoffs/history.py:44-49`, `376-398`); unpersistable reasoning (no `id` and no `encrypted_content`) is counted but withheld from Conversations storage (`src/agents/run_internal/session_persistence.py:938-942`). Both major mitigations are opt-in/beta, so uncontrolled growth is the default trajectory.

**5. Can it be audited?**
Yes, unusually well for this dimension. `RunState` snapshots are versioned JSON with documented per-version change summaries and a compatibility corpus test (`src/agents/run_state.py:186-217`; `tests/test_run_state_compatibility_corpus.py`), so hidden state captured at interruption time is inspectable and diffable. Session contents are directly readable via `get_items(limit)` (`src/agents/memory/session.py:26-36`) — the same data the model sees. Tracing provides span-level audit trails with a sensitive-data switch (`src/agents/tracing/model_tracing.py:6-14`). Gaps: the ephemeral agent-tool maps have no inspection API beyond tests, and trace redaction defaults to include-everything (`src/agents/run_config.py:53-56`), so audit completeness depends on configuration.

Additional step coverage: **cleared at task boundaries?** Nothing auto-clears between tasks; sessions persist until explicit `clear_session()` (`src/agents/memory/sqlite_session.py:435-450`) or TTL expiry in `EncryptedSession` (`src/agents/extensions/memory/encrypt_session.py:107-111`); terminal `RunState`s refuse further input rather than clearing (`src/agents/run_state.py:950-953`); compaction caches reset only on session clear (`src/agents/memory/openai_responses_compaction_session.py:431-436`). **Sensitive-content leaks?** Provider-specific fields are stripped before persistence (`src/agents/run_internal/session_persistence.py:910-930`), URL credentials are hidden in persisted MCP origin metadata (`tests/test_tool_origin.py:170`), and encryption-at-rest is available via the extension — but traces default to including sensitive data (`src/agents/run_config.py:53-56`) and voice pipelines default both audio and text tracing on (`src/agents/voice/pipeline_config.py:26-30`).

## Architectural Decisions

1. **Layered memory over a monolithic scratchpad.** Rather than one scratchpad abstraction, lifetime-scoped containers are used: per-turn (`turn_input`), per-run (`RunContextWrapper`, `AgentToolUseTracker`), per-interruption (`RunState`), cross-run (`Session`), and per-tool-call (agent-tool maps). Each layer has its own copy/serialization semantics (e.g., `pending_input` property returns a deepcopy, `src/agents/run_state.py:936-939`).
2. **Explicit model/runtime/user visibility boundaries.** Contexts are documented as never reaching the LLM (`src/agents/run_context.py:76-77`); model-visible history is a *projection* (`_generated_items`) kept separate from the full record (`_session_items`, `src/agents/run_state.py:782-786`), letting handoff filters reshape what the next agent sees without losing the audit trail.
3. **Versioned durable-state contracts.** The RunState snapshot format is a released compatibility boundary with chronological change log, mandatory summaries enforced by assertion (`src/agents/run_state.py:220-232`), and fail-fast rejection of newer schemas (`175-182`).
4. **Identity-keyed deduplication as memory hygiene.** Tool invocations get canonical identities (type, call id, fingerprint) so repeated turns/resumes cannot double-execute or conflate calls (`src/agents/run_context.py:304-355`; dedupe in `src/agents/run_internal/tool_planning.py:339-554`).
5. **Compensation-first persistence.** History rewrites (compaction) are transactions with restore paths hardened against cancellation storms (`src/agents/memory/openai_responses_compaction_session.py:271-336`), reflecting that working-memory loss is treated as a failure mode worth engineering against.

## Notable Patterns

- **Weakref-coupled ephemeral caches**: the nested-agent scratchpad ties cache entries to tool-call object lifetimes via weakref callbacks, preventing both leaks and stale reuse (`src/agents/agent_tool_state.py:131-142`, verified by `tests/test_agent_tool_state.py:103`).
- **Scope ids instead of global keys**: independently restored checkpoints stamp context wrappers with a random scope id (`uuid4().hex` at `src/agents/run_context.py:130`) so identical tool-call signatures from different resumes cannot collide (`tests/test_agent_tool_state.py:89`).
- **Summary-marker envelope**: handoff summaries are delimited by configurable `<CONVERSATION HISTORY>` markers and later re-parseable back into structured items (`src/agents/handoffs/history.py:29-34`, `52-80`, parsers around `476-580`), making compressed working notes losslessly recoverable.
- **Policy-driven reasoning retention**: a single literal-typed knob (`preserve`/`omit`) threads through model-input building, streaming results, and the persisted state so the choice survives resume (`src/agents/run_config.py:459-464`; `src/agents/run_state.py:811`, `4261-4265`).
- **Decorator-style session composition**: compaction and encryption wrap any underlying `Session` transparently (`OpenAIResponsesCompactionSession.__init__` rejects wrapping the server-managed conversations session to avoid double bookkeeping, `src/agents/memory/openai_responses_compaction_session.py:116-120`).

## Tradeoffs

- **No first-class scratchpad means ad-hoc user patterns.** Agents needing private notes must abuse the freeform `context` object, whose serialization safety is the developer's burden (`src/agents/run_state.py:756-762`).
- **Module-level globals for ephemeral state.** Simple and fast, but process-global mutable state complicates multi-event-loop embedding and testing; mitigated by scope ids and weakrefs but still global (`src/agents/agent_tool_state.py:35-48`).
- **Default-permissive observability.** Sensitive tracing defaults to on for discoverability/debuggability at the cost of leakage risk unless operators set `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA=false` (`src/agents/run_config.py:53-56`).
- **Beta-stage memory compression.** Both handoff summarization and responses-compaction carry beta caveats (server-managed conversations disable them with a warning; Chat Completions rejects compaction items outright, `src/agents/models/chatcmpl_converter.py:956-960`), so long-task working-memory relief isn't universally available.
- **Full-fidelity durability vs size.** `_session_items` keeps the unfiltered record for auditing while `_generated_items` feeds the model — correctness win, doubled in-memory footprint during runs (`src/agents/run_state.py:782-786`).

## Failure Modes / Edge Cases

- **Unserializable contexts degrade to warnings, not errors**: a snapshot is still written with rebuild metadata when no safe serializer exists (`src/agents/run_state.py:758-762`) — resume may then fail far from the cause.
- **Call-ID reuse by the model fails fast**: reused or ambiguous tool call IDs raise `ModelBehaviorError` rather than silently corrupting invocation memory (`src/agents/run_context.py:327-355`; `src/agents/run_internal/tool_planning.py:386-391`).
- **Compaction output quirks**: compacted histories can contain orphaned assistant message IDs after reasoning stripping, which would 400 on replay; the session strips them defensively (`src/agents/memory/openai_responses_compaction_session.py:458-484`).
- **Cancellation during history rewrite**: repeated cancellation during compaction restore is drained until settlement so a cancel cannot leave an empty session (`src/agents/memory/openai_responses_compaction_session.py:316-336`).
- **Corrupt session rows are skipped, not fatal**: invalid JSON rows are dropped during reads/pops, with limit-window expansion to still return N valid items (`src/agents/memory/sqlite_session.py:300-309`, `325-345`, `408-431`).
- **Expired encrypted memory disappears silently**: `EncryptedSession` TTL expiry skips items without error (`src/agents/extensions/memory/encrypt_session.py:107-111`) — intentional, but a surprise source of amnesia under clock drift.
- **Ambiguous nested-result lookups return nothing rather than guessing**: signature fallback requires exactly one candidate match (`src/agents/agent_tool_state.py:220-230`).

## Future Considerations

- Expose a documented, serializable scratchpad surface (e.g., a typed section of `RunState` with read/write APIs) so agents can keep private task notes without overloading the freeform context.
- Promote handoff-history summarization and responses-compaction out of beta and define provider-neutral compaction equivalents (Chat Completions currently hard-rejects compaction items, `src/agents/models/chatcmpl_converter.py:956-960`).
- Add an inspection/dump API for the ephemeral agent-tool maps to close the last unauditable memory layer.
- Flip tracing sensitive-data default or add scoped redaction, given the current include-by-default posture (`src/agents/run_config.py:53-56`).

## Questions / Gaps

- No evidence found of any mechanism that distinguishes "working notes" from "facts" inside model-visible history itself (e.g., tagging items as provisional); the separation happens only at the runtime/state layer. Searched: `scratchpad|scratch_pad|working_memory|notes_to_self|private_state|plan state` across `src/`.
- Whether reasoning *text* (as opposed to IDs/encrypted blobs) is ever replayed into input could not be fully traced for all providers; evidence shows encrypted-content round-tripping for Chat Completions conversions (`src/agents/models/chatcmpl_converter.py:866-952`) and summary-text handling in the streaming handler (`src/agents/models/chatcmpl_stream_handler.py:501-585`), but a complete per-provider matrix was out of scope.
- Long-term memory (user-profile style extraction, vector stores) is absent from the core SDK; only conversation-history sessions exist. If the dimension intended LTM-style memory, the finding is "not implemented in this source" based on the `src/agents/memory/` inventory.

---

Generated by `Dimension 05.02: Working Memory and Scratchpad` against `openai-agents-sdk`.
