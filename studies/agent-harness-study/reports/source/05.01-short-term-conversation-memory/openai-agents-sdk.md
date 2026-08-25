# Source Analysis: openai-agents-sdk

## 05.01 Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (`pyproject.toml:6`, package `openai-agents` v0.22.0, `pyproject.toml:2-4`) |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents SDK (Python) implements short-term conversation memory as a first-class, pluggable **Session** abstraction. A `Session` is a keyed history store (`src/agents/memory/session.py:15-56`) with four operations: `get_items(limit)`, `add_items`, `pop_item`, and `clear_session`. History items are stored in the OpenAI Responses API *input item* format (dicts with `role` / `type`), so no per-turn role conversion happens at the memory layer; conversion to provider wire formats happens later in the model adapters (`src/agents/models/chatcmpl_converter.py:572-695`).

Per run, the runner merges stored history with the new turn's input before the first model call and appends all generated items (user input, assistant messages, tool calls, tool outputs) after each turn. The merge/persist logic lives in `prepare_input_with_session` and `save_result_to_session` (`src/agents/run_internal/session_persistence.py:317-477`, `545-698`). Selection policy is **full history by default**; windowing to the latest N items is opt-in via `SessionSettings(limit=N)` (`src/agents/memory/session_settings.py:30-39`, `resolve_session_limit` at `18-27`). Summarization/compaction exists as a decorator session (`OpenAIResponsesCompactionSession`) that rewrites stored history via the `responses.compact` API once ≥10 compaction-candidate items accumulate (`src/agents/memory/openai_responses_compaction_session.py:28-60`).

Backends shipped in-tree: `SQLiteSession` (`src/agents/memory/sqlite_session.py:42`), `OpenAIConversationsSession` (server-side Conversations API storage, `src/agents/memory/openai_conversations_session.py:27`), plus Redis, SQLAlchemy, MongoDB, Dapr, async-SQLite, advanced-SQLite (branching) and encrypted-wrapper sessions under `src/agents/extensions/memory/`.

## Rating

**9 / 10** — Mature, durable, observable, extensible, and proven under failure conditions.

Rationale against the rubric:
- **Clear model + explicit interfaces**: `Session` protocol and `SessionABC` with documented semantics (`src/agents/memory/session.py:15-106`); the maintainer contract ("latest N items in chronological order", atomic batches, ownership-sensitive rollback) is written down in `.agents/references/session-persistence.md:7-24`.
- **Tests**: ~500 focused tests across `tests/memory/` (156 tests over 6 files, e.g. limit behavior at `tests/memory/test_session_limit.py:19-203`) and `tests/extensions/memory/` (~416 tests across backend suites).
- **Operational safeguards**: cancellation-safe mutations that drain even under repeated cancellation (`_await_mutation`, `src/agents/memory/sqlite_session.py:20-39`); corrupt-row isolation so one bad record cannot hide valid history (`src/agents/memory/sqlite_session.py:300-309`, `408-431`); WAL + cross-process file locks (`212-224`, `118-142`); retry rewind that only deletes an exactly-fingerprinted tail suffix and restores on partial failure (`src/agents/run_internal/session_persistence.py:978-1040`, `1043-1065`); compaction replacement with transactional restore on failure/cancellation (`src/agents/memory/openai_responses_compaction_session.py:271-337`).
- **Not 10** because: default retrieval is unbounded full history — context overflow protection requires explicit configuration (limit, callback, or compaction); the `limit` counts raw items rather than tokens, so a "window" can still be arbitrarily large; and there is no built-in token-budgeted windowing or summarization without user-supplied callbacks.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message store contract | `Session` protocol: `session_id`, `get_items(limit)`, `add_items`, `pop_item`, `clear_session` | `src/agents/memory/session.py:15-56` |
| Internal base class | `SessionABC` mirrors the protocol for concrete backends | `src/agents/memory/session.py:59-106` |
| Windowing config | `SessionSettings(limit)`; `None` = all items; resolved via `resolve_session_limit` | `src/agents/memory/session_settings.py:18-39` |
| SQLite message store | Two tables (`agent_sessions`, `agent_messages`) keyed by `session_id`; messages are JSON-serialized input items with autoincrement ordering | `src/agents/memory/sqlite_session.py:231-261` |
| Latest-N selection with corruption tolerance | Fetch newest rows DESC, expand window while corrupt JSON rows sit among newest, return last N valid items chronologically | `src/agents/memory/sqlite_session.py:300-358` |
| Cancellation-safe persistence writes | `_await_mutation` drains mutations despite repeated caller cancellation | `src/agents/memory/sqlite_session.py:20-39` |
| Server-managed store | `OpenAIConversationsSession` lists/creates/deletes conversation items via OpenAI Conversations API; lazily creates `conversation_id` | `src/agents/memory/openai_conversations_session.py:76-133` |
| Compaction (summarization) trigger | Default hook fires when ≥10 candidate items (non-user-message, non-compaction) exist; custom `should_trigger_compaction` supported | `src/agents/memory/openai_responses_compaction_session.py:28-60,130-134` |
| Compaction history rewrite | Calls `responses.compact`, then clear+re-add underlying session inside a mutation lock with restore-on-failure/cancel | `src/agents/memory/openai_responses_compaction_session.py:170-263,271-337` |
| Session history builder | `prepare_input_with_session`: fetch history → normalize → optional callback merge → drop orphans → dedupe; returns model input AND the new-items-only subset to persist | `src/agents/run_internal/session_persistence.py:317-477` |
| Per-turn persistence | `save_result_to_session` converts RunItems to input items, tracks `_current_turn_persisted_item_count` to avoid duplicate saves during streaming/retries, re-adds missing tool outputs | `src/agents/run_internal/session_persistence.py:545-654` |
| Retry rollback | `rewind_session_items` pops an exact fingerprint-matched tail suffix, restoring popped items on mismatch/failure | `src/agents/run_internal/session_persistence.py:727-831,978-1065` |
| Role handling (storage) | Items persisted as Responses-format dicts (`role`: user/assistant/system/developer; types: `message`, `function_call`, `function_call_output`, `reasoning`, …); approval items excluded from persistence | `src/agents/run_internal/items.py:139-155` (`tool_approval_item` → `None`) |
| Role handling (wire) | Chat Completions adapter converts stored roles to provider params (user/system/developer/assistant) at request time | `src/agents/models/chatcmpl_converter.py:572-695` |
| Tool messages retained | Tool call outputs persist as `function_call_output`; orphan calls without outputs are dropped on replay; reasoning items preceding dropped calls also dropped | `src/agents/run_internal/items.py:171-304` |
| Dedupe policy | `deduplicate_input_items_preferring_latest` keeps latest value for call-id/item-id keys, anchoring causal precursors earliest | `src/agents/run_internal/items.py:681-800` |
| Reasoning ID policy | `"omit"` strips `rs_...` IDs on both read and write paths so replay cannot 404 on stale server IDs | `src/agents/run_internal/items.py:131,726-749`; applied on read at `src/agents/run_internal/session_persistence.py:359-368` |
| Runner integration (non-streaming) | `_run_impl` calls `prepare_input_with_session` before turn 0; server-managed conversations exclude local history (`include_history_in_prepared_input=False`) | `src/agents/run.py:658-682` |
| Runner integration (streaming) | Same preparation at stream start; per-item save helpers track persisted counts | `src/agents/run_internal/run_loop.py:1089-1106,1149-1168` |
| Final pre-call filter | `call_model_input_filter` receives full prepared input + system instructions right before the model call and may edit them; result deduped again | `src/agents/run_internal/run_loop.py:2138-2162`; hook defined at `src/agents/run_config.py:438-446` |
| Config knobs | `RunConfig.session_input_callback`, `call_model_input_filter`, `session_settings`, `reasoning_item_id_policy` | `src/agents/run_config.py:431-464` |
| Custom-merge callback type | `SessionInputCallback(history, new_input) -> combined`; SDK persists only new-turn items even if callback reorders/drops history | `src/agents/memory/util.py:8-20`; disambiguation logic at `src/agents/run_internal/session_persistence.py:389-451` |
| Context-aware sessions | Sessions can opt into receiving `RunContextWrapper` by declaring a `wrapper` kwarg on all four methods | `src/agents/memory/session.py:155-196`; docs `docs/sessions/index.md:690-729` |
| Prompt-cache coupling | Session id feeds prompt-cache grouping (`prompt_cache_key`) so repeated prefixes hit cache | `src/agents/run_internal/run_grouping.py:12-52`; `src/agents/run_internal/prompt_cache_key.py:13-124` |
| Backend ecosystem | Redis/SQLAlchemy/MongoDB/Dapr/AsyncSQLite/AdvancedSQLite(branches)/Encrypted wrapper | `src/agents/extensions/memory/` (e.g., branching at `src/agents/extensions/memory/advanced_sqlite_session.py:997,1077`) |
| Tests: limits & drops | Latest-N retrieval, zero-limit, orphan tool-output dropping when limited | `tests/memory/test_session_limit.py:19-126` |
| Tests: core protocol | 37 tests covering add/get/pop/clear semantics | `tests/memory/test_session.py` |
| Tests: compaction failure paths | Replacement restore, cancellation recovery, deferred compaction | `tests/memory/test_openai_responses_compaction_session.py` (54 tests) |
| Docs tied to implementation | Sessions guide describing read-before/write-after behavior, callbacks, limits | `docs/sessions/index.md:62-133` |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
By default, everything ever stored for the session plus the current turn's input. `prepare_input_with_session` fetches `history = await _session_get_items(...)` (unbounded unless `SessionSettings.limit` resolves a cap) and concatenates it with the normalized new-input list (`src/agents/run_internal/session_persistence.py:350-383`). Within one multi-turn run, each subsequent model call additionally replays the items generated so far in that run (turn input accumulation through `streamed_result.input` / `_model_input_items`, `src/agents/run_internal/run_loop.py:2125-2142`). If a server-managed conversation is active (`conversation_id` / `previous_response_id` / `auto_previous_response_id`), local history is deliberately *excluded* from the request because the server owns the transcript (`include_history_in_prepared_input=False`, `src/agents/run.py:658-668`). System instructions never live in session history; they are fetched fresh per turn and passed as separate `instructions` (`src/agents/run_internal/run_loop.py:2097-2100,2144-2150`).

**2. What gets dropped?**
- Tool **approval** items are never persisted as replayable input (`run_item_to_input_item` returns `None` for `tool_approval_item`, `src/agents/run_internal/items.py:144-145`).
- Orphan function/tool calls whose outputs are missing are removed before the model sees history; dangling reasoning items immediately preceding dropped calls are removed too (Responses API would reject them) (`src/agents/run_internal/items.py:171-304`).
- Duplicate items sharing stable identifiers (`call_id` / `id`) are deduplicated keeping the latest occurrence (`src/agents/run_internal/items.py:768-800`).
- With `SessionSettings.limit=N`, anything older than the latest N items is not fetched (windowing, not deletion — the store still holds it).
- Corrupt JSON rows in `SQLiteSession` are silently skipped on read and dropped on `pop_item` (`src/agents/memory/sqlite_session.py:303-308`, `408-429`).
- Reasoning item IDs are stripped under `reasoning_item_id_policy="omit"` (`src/agents/run_internal/items.py:726-749`).

**3. Are tool messages retained?**
Yes. Function/shell/apply-patch/computer/MCP tool outputs are converted to their Responses input forms (`*_output` items) and appended to the session each turn (`src/agents/run_internal/session_persistence.py:603-622`). The persister even re-includes tool outputs that a partially-completed streaming save missed, ahead of the remaining items (`missing_outputs` fix-up, `src/agents/run_internal/session_persistence.py:576-583`). The compaction wrapper explicitly defers response-chain compaction while a turn still has un-persisted local tool outputs so outputs stay associable with their calls (`src/agents/run_internal/session_persistence.py:656-673`). Only *approval requests* are excluded from replayable history (see Q2).

**4. Is memory per user/thread/session?**
Scoping is entirely by the caller-chosen `session_id` string used as the primary key in every backend (e.g., `SQLiteSession(session_id, db_path)`, schema keyed on `session_id`, `src/agents/memory/sqlite_session.py:55-86,233-254`; `OpenAIConversationsSession.session_id` maps 1:1 to a server conversation, `src/agents/memory/openai_conversations_session.py:51-80`). There is no built-in user→session hierarchy; docs recommend naming conventions like `"user_123"` / `"thread_abc123"` (`docs/sessions/index.md:515-524`). For multi-tenant routing, a session can opt into receiving the run's `RunContextWrapper` on all four methods (`src/agents/memory/session.py:155-196`). Sessions are shareable across agents by design (same store, different agents) (`docs/sessions/index.md:561-580`).

**5. Can history be edited or forked?**
- **Edit tail**: `pop_item()` removes the most recent item; documented pattern for undoing the last exchange (`docs/sessions/index.md:164-193`). Internally the runtime uses guarded pop-based rewind on conversation retries (`src/agents/run_internal/session_persistence.py:727-782`).
- **Edit view vs store**: `RunConfig.session_input_callback(history, new_input)` can reorder/filter/duplicate the *model view* per turn while the SDK persists only genuinely new items (`src/agents/run_config.py:431-436`; identity/frequency reconciliation at `src/agents/run_internal/session_persistence.py:394-451`). A second hook, `call_model_input_filter`, edits final model input just before the call (`src/agents/run_config.py:438-446`).
- **Rewrite store**: `clear_session()` + `add_items()`, or the compaction decorator which replaces history with compacted output including restore-on-failure (`src/agents/memory/openai_responses_compaction_session.py:248-255,271-337`).
- **Fork**: supported out of the box via `AdvancedSQLiteSession.create_branch_from_turn` / `switch_to_branch` (`src/agents/extensions/memory/advanced_sqlite_session.py:997,1077`). Plain sessions can be forked manually by copying items into a new session id; there is no dedicated copy API.

## Architectural Decisions

1. **History stored in provider-native input format.** Sessions persist `TResponseInputItem` dicts verbatim (JSON in SQLite, `src/agents/memory/sqlite_session.py:271-277`), eliminating a translation layer between store and model request; role/provider conversion is deferred to model adapters (`src/agents/models/chatcmpl_converter.py:572-695`).
2. **Protocol-first extensibility.** Third-party backends implement the structural `Session` protocol without inheriting anything (`src/agents/memory/session.py:15-56`; custom-session guide `docs/sessions/index.md:646-688`), with an opt-in `wrapper` keyword contract instead of breaking signature changes (`session.py:155-172`).
3. **Two orthogonal override points for selection.** `session_input_callback` decides what the model *sees* from history+new input; `call_model_input_filter` gets a final pass including instructions (`src/agents/run_config.py:431-446`). Storage remains append-only underneath, which keeps persistence semantics independent of view shaping.
4. **Compaction as a decorator, not a core feature.** Summarization is layered onto any session (`OpenAIResponsesCompactionSession` wraps any non-Conversations session, `openai_responses_compaction_session.py:82-134`) and is forbidden around server-owned conversations (`116-120`), respecting the "one owner of truth" boundary described in `.agents/references/session-persistence.md:5`.
5. **Ownership-sensitive rollback.** Retry cleanup pops only an exact serialized suffix proven to belong to the failed attempt and restores already-popped items on any mismatch (`src/agents/run_internal/session_persistence.py:987-1040`) — history integrity is valued over opportunistic cleanup.
6. **Persistence counted, not assumed.** `_current_turn_persisted_item_count` makes streaming/retry/resume saves idempotent (`src/agents/run_internal/session_persistence.py:563-583`), so a turn's items land in the store exactly once even across partial failures.
7. **Memory doubles as cache key.** The session id participates in prompt-cache grouping (`src/agents/run_internal/run_grouping.py:37-45`, `src/agents/run_internal/prompt_cache_key.py:83-87`), tying short-term memory to cost optimization.

## Notable Patterns

- **Read-modify-append cycle**: get history → merge with new input → call model loop → persist generated items; implemented symmetrically in non-streaming (`src/agents/run.py:658-682`) and streaming (`src/agents/run_internal/run_loop.py:1089-1106`) paths, kept aligned per repo guidance (`.agents/references/session-persistence.md:41`).
- **Window expansion under corruption**: when enforcing `limit`, the SQL fetch window doubles until N *valid* items are decoded, matching `EncryptedSession` and `pop_item` semantics (`src/agents/memory/sqlite_session.py:325-345`).
- **Fingerprint-based identity**: items are compared via canonical JSON fingerprints (optionally ignoring unstable IDs) for dedupe, rewind targeting, and callback reconciliation (`src/agents/run_internal/items.py:355-392`; usage in `session_persistence.py:610-640`).
- **Decorator stacking**: `EncryptedSession` (encryption+TTL) and `OpenAIResponsesCompactionSession` wrap arbitrary underlying sessions (`docs/sessions/index.md:484-509`, `256-310`), composing durability features without backend changes.
- **Cancellation-hardened I/O**: both SQLite mutations (`_await_mutation`, `sqlite_session.py:20-39`) and compaction restore (`_await_restore_despite_cancellation`, `openai_responses_compaction_session.py:316-336`) survive repeated task cancellation — a deliberate hardening pattern rare in similar harnesses.
- **Internal metadata stripping**: SDK-only keys (`_agents_tool_description`, occurrence keys) are stripped before items go back to the model (`strip_internal_input_item_metadata`, `src/agents/run_internal/items.py:715-723`).

## Tradeoffs

- **Full-history default vs overflow risk.** Storing and replaying everything maximizes fidelity but pushes token management onto users (limits count items, not tokens; `SessionSettings.limit`, `src/agents/memory/session_settings.py:38-39`). Mitigations exist (callback, compaction) but none are default.
- **JSON-per-row SQLite simplicity vs scale.** One row per item with autoincrement ordering is simple and correct, but reading "all history" loads the entire transcript into memory each run (`SELECT ... ORDER BY id ASC`, `sqlite_session.py:313-323`); fine for chat, heavy for long agent runs.
- **Strict suffix verification vs cleanup completeness.** Rewind aborts on any tail mismatch (`session_persistence.py:1013-1015`), protecting unrelated history at the cost of sometimes leaving stray items behind (logged, then stripped separately only when provably retry-owned, `808-831`).
- **Server-managed vs client-managed exclusivity.** A session cannot combine with `conversation_id`/`previous_response_id` (`validate_session_conversation_settings`, `src/agents/run.py:621-647`; doc statement `docs/sessions/index.md:7`) — clean ownership, but migrating between modes requires history copying by hand.
- **View-shaping callbacks vs persistence drift.** The callback mechanism must reconstruct "which output items are actually new" via deep copies, object identity, and frequency maps (`session_persistence.py:394-451`) — powerful but intricate; correctness depends on subtle heuristics (documented as such at `docs/sessions/index.md:86`).

## Failure Modes / Edge Cases

- **Corrupt stored records**: silently skipped on `get_items` and dropped on `pop_item`, so history quietly shrinks rather than failing loudly (`src/agents/memory/sqlite_session.py:303-308,408-429`). Safe-by-default, low observability (no user-facing warning).
- **Compaction replacement interruption**: clear succeeds but re-add fails/cancels → previous history is restored best-effort; if the backend also fails during restore, data loss is possible and only logged (`openai_responses_compaction_session.py:286-337`; acknowledged in `docs/sessions/index.md:289`).
- **Concurrent writers**: multiple processes on one SQLite file are serialized via refcounted file locks + WAL + busy-timeout retry (`sqlite_session.py:118-142,211-224`), but cross-machine concurrency relies on backend choice (Redis/Mongo/SQLAlchemy).
- **Stale reasoning IDs**: replaying history with server-assigned `rs_...` IDs can 404 if the server forgot them; handled by applying the omit policy on both write and read paths (`session_persistence.py:359-368` comment explains the historical-data case).
- **Guardrail trips mid-turn**: input persistence is specially reconciled so the accepted user input survives while speculative assistant/tool work is discarded (`persist_session_items_for_guardrail_trip`, `session_persistence.py:480-510`; tested in streaming and non-streaming modes per `.agents/references/session-persistence.md:41`).
- **Streaming double-save**: partially saved turns are completed by counting previously persisted items and force-re-adding missing tool outputs (`session_persistence.py:563-583`), avoiding duplicates *and* gaps.

## Future Considerations

- Token-budgeted history selection: `limit` could accept a size budget or pluggable selector so windowing reflects tokens rather than item counts (`src/agents/memory/session_settings.py:38-39`).
- Built-in rolling summarization outside the OpenAI-specific compaction wrapper (provider-neutral summarize-and-replace strategy), since `select_compaction_candidate_items` currently protects user messages but summarization itself depends on the `responses.compact` endpoint (`openai_responses_compaction_session.py:34-60,232-238`).
- Louder telemetry for silently-skipped corrupt records (currently only internal decode skips, `sqlite_session.py:303-308`).
- First-class fork/copy API between sessions (today only `AdvancedSQLiteSession` branches within one DB, `advanced_sqlite_session.py:997`).

## Questions / Gaps

- **No evidence found** for automatic per-user quota, retention windows, or TTL in core sessions (TTL exists only in the `DaprSession` option and `EncryptedSession` wrapper; searched `src/agents/memory/` and `src/agents/extensions/memory/` for `ttl`, `retention`, `expire`).
- **No evidence found** for built-in semantic retrieval over short-term history (that is the domain of dimension 05.04; nothing in `src/agents/memory/` performs embedding search — verified by inspecting all files in the directory listing of `src/agents/memory/`).
- Exact upstream release provenance of this checkout was not pinned beyond `version = "0.22.0"` in `pyproject.toml:3`; line numbers refer to this snapshot.

---

Generated by `05.01-short-term-conversation-memory` dimension instructions against `openai-agents-sdk`.
