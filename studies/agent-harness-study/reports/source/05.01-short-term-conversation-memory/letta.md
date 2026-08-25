# Source Analysis: letta

## Dimension 05.01: Short-Term Conversation Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas; PostgreSQL/SQLite persistence) |
| Analyzed | 2026-08-25 |

## Summary

Letta (formerly MemGPT) treats short-term conversation memory as an explicit, durable, database-backed construct rather than an ephemeral client-side transcript. Every message — user, assistant, tool, approval, and summary — is persisted to a `messages` table (`letta/orm/message.py:23-96`) with a monotonically increasing `sequence_id`, and the *in-context window* is a separate, mutable pointer: an ordered list of message IDs stored on agent state (`message_ids`, `letta/schemas/agent.py:78`) or, in conversation mode, in a `conversation_messages` join table with `position` + `in_context` flags (`letta/services/conversation_manager.py:613-650`, `letta/services/conversation_manager.py:752-762`). At the start of every step, `_prepare_in_context_messages_no_persist_async` hydrates exactly those IDs into `Message` objects (`letta/agents/helpers.py:149-220`) — so "what the model sees" is whatever the pointer says, and eviction never deletes history, it only rewrites the pointer.

Context overflow is handled by two coexisting generations of machinery: (1) a legacy count-based `Summarizer` with `STATIC_MESSAGE_BUFFER` (hard buffer of 60 messages, keep last 15) and `PARTIAL_EVICT_MESSAGE_BUFFER` (evict ~30%, insert recursive summary) modes configured via env-prefixed settings (`letta/settings.py:74-111`, `letta/services/summarizer/summarizer.py:36-343`), and (2) a newer token-based `compact_messages` pipeline with four modes (`all`, `sliding_window`, `self_compact_all`, `self_compact_sliding_window`), provider-specific lightweight summarizer models, a fallback chain between modes, and post-compaction token verification (`letta/services/summarizer/compact.py:135-472`, `letta/services/summarizer/summarizer_config.py:48-89`). The current v3 agent loop triggers compaction proactively when observed prompt tokens exceed 90% of the context window (`letta/constants.py:82-83`, `letta/agents/letta_agent_v3.py:1438-1505`), reactively on `ContextWindowExceededError` retries (`letta/agents/letta_agent_v3.py:1218-1294`), and on demand via API endpoints.

Role handling is centralized in `Message.to_openai_dict` (`letta/schemas/message.py:1339-1498`): internal roles `summary` and `approval` are folded into provider-visible `user`/`assistant` roles, tool returns are emitted as native `tool` messages keyed by `tool_call_id`, and system messages can be upgraded to `developer`. Tool messages are fully retained in context, and both compaction cutoff selection and approval idempotency checks contain explicit logic to avoid splitting assistant/tool-call/approval groups. Memory is scoped per agent and optionally per conversation, with fork support that shares message objects across conversations — which is precisely why message editing was removed from the REST API (`letta/server/rest_api/routers/v1/agents.py:1627-1644`).

## Rating

**8 / 10** — Clear model with explicit interfaces, durable storage, multiple compaction strategies with fallback chains, observable compaction stats/stop-reasons, and unit + integration tests covering thresholds, buffer edge cases, and forking. Not a 9–10 because two generations of summarization code coexist (legacy `Summarizer` vs. `compact_messages`), several v3 paths are commented out mid-migration, the legacy partial-evict mode evicts by message *count* rather than tokens by its own admission (`letta/services/summarizer/summarizer.py:145`), and maintainers' TODOs flag config-dependent fragility (`letta/agents/letta_agent.py:1587-1588`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Message store (DB table) | `messages` table: role, content parts, tool_calls, tool_call_id, step/run/conversation FKs, `sequence_id` cursor | `sources/letta/letta/orm/message.py:23-96` |
| Ordering guarantees | Composite indexes on `(agent_id, conversation_id, sequence_id)`; SQLite sequence reservation for bulk inserts | `sources/letta/letta/orm/message.py:27-37`, `sources/letta/letta/orm/message.py:130-201` |
| Role taxonomy | `MessageRole`: assistant/user/tool/function/system/approval/summary | `sources/letta/letta/schemas/enums.py:110-117` |
| Role validation on write | Pydantic validator restricts persisted roles to the six-value set | `sources/letta/letta/schemas/message.py:319-325` |
| In-context pointer (default mode) | `AgentState.message_ids` = ordered IDs of messages currently in context | `sources/letta/letta/schemas/agent.py:78` |
| In-context pointer (conversation mode) | `conversation_messages` rows with `position` and `in_context` flag; loader filters `in_context == True` ordered by position | `sources/letta/letta/services/conversation_manager.py:613-650`, `sources/letta/letta/services/conversation_manager.py:752-762` |
| History hydration per step | `_prepare_in_context_messages_no_persist_async` loads full list via `get_messages_by_ids_async`; errors if pointer empty | `sources/letta/letta/agents/helpers.py:177-220`, `sources/letta/letta/agents/helpers.py:206-212` |
| Autoclear mode | `message_buffer_autoclear=True` loads only the system message (index 0) each turn | `sources/letta/letta/agents/helpers.py:81-86`, `sources/letta/letta/schemas/agent.py:139-142` |
| Checkpointing new messages | `_checkpoint_messages` persists messages once per safe step, then updates conversation tracking or `agent.message_ids` | `sources/letta/letta/agents/letta_agent_v3.py:758-816` |
| Legacy summarizer modes | `STATIC_MESSAGE_BUFFER` (buffer limit/min) and `PARTIAL_EVICT_MESSAGE_BUFFER`; mode dispatch in `summarize()` | `sources/letta/letta/services/summarizer/summarizer.py:104-122`, `sources/letta/letta/settings.py:74-111` |
| Static buffer eviction | Evict everything between system msg and trim index; walk forward to a `user` boundary; keep assistant+approval same-step pairs together | `sources/letta/letta/services/summarizer/summarizer.py:275-312` |
| Partial-evict recursion | Evicts bottom 30% of messages, summarizes them via LLM, inserts summary as role-user at index 1 | `sources/letta/letta/services/summarizer/summarizer.py:136-242` |
| New compaction pipeline | `compact_messages(...)` with mode dispatch, fallback chain, trigger metadata | `sources/letta/letta/services/summarizer/compact.py:135-348` |
| Sliding-window compaction | Token-counted cutoff search that grows eviction % by 10% steps until under goal tokens; cutoff must be assistant/approval-with-tool-calls | `sources/letta/letta/services/summarizer/summarizer_sliding_window.py:99-232` |
| Compaction settings schema | `CompactionSettings`: model handle, prompt, clip_chars, mode literal, sliding_window_percentage | `sources/letta/letta/services/summarizer/summarizer_config.py:48-89` |
| Summarizer default models | Provider-specific defaults (claude-haiku-4-5, gpt-5-mini, gemini-2.5-flash) | `sources/letta/letta/services/summarizer/summarizer_config.py:26-32` |
| Proactive trigger threshold | 90% multiplier constant; GPT-5 family documented as proactive at 90% | `sources/letta/letta/constants.py:82-83`, `sources/letta/letta/services/summarizer/thresholds.py:27-41` |
| Post-step compaction check | v3 compares `context_token_estimate > compaction_trigger_threshold`, calls `self.compact(...)`, checkpoints summary | `sources/letta/letta/agents/letta_agent_v3.py:1438-1505` |
| Reactive retry on overflow | On `ContextWindowExceededError`, compact and retry LLM request up to `max_summarizer_retries` (3) | `sources/letta/letta/agents/letta_agent_v3.py:1218-1294`, `sources/letta/letta/settings.py:96` |
| Context estimate source | `self.context_token_estimate = llm_adapter.usage.total_tokens` after each request | `sources/letta/letta/agents/letta_agent_v3.py:1306-1307` |
| Post-compaction verification | Re-count tokens incl. tools; fallback to `all` mode; raise `SystemPromptTokenExceededError` if system prompt alone overflows | `sources/letta/letta/services/summarizer/compact.py:350-412` |
| Summary message format | role=`summary` message; packed JSON `system_alert` with optional `compaction_stats` | `sources/letta/letta/services/summarizer/compact.py:426-465`, `sources/letta/letta/system.py:207-236` |
| Summary → provider mapping | `summary` role converted to `user` at request time; stats parsed back out for display | `sources/letta/letta/schemas/message.py:1396-1402`, `sources/letta/letta/schemas/letta_message.py:406-449` |
| Role conversion (system) | `system` → `developer` when model supports it | `sources/letta/letta/schemas/message.py:1383-1387`, `sources/letta/letta/llm_api/openai_client.py:548` |
| Role conversion (approval) | `approval` → `assistant` w/ tool_calls; approval without tool_calls dropped (returns None) | `sources/letta/letta/schemas/message.py:1352-1353`, `sources/letta/letta/schemas/message.py:1404-1454` |
| Tool messages retained | `role=tool` emitted with `tool_call_id` + truncation cap; tool_returns persisted on ORM row | `sources/letta/letta/schemas/message.py:1456-1478`, `sources/letta/letta/orm/message.py:55-57` |
| Per-request tool return cap | Dynamic cap ≈ 20% of context window in chars, min 5000 | `sources/letta/letta/agents/letta_agent_v3.py:143-153` |
| Approval idempotency scan | Duplicate approvals detected by scanning last 10 in-context messages for matching tool return | `sources/letta/letta/agents/helpers.py:230-265` |
| Forking | `fork_conversation` copies non-system message IDs into new conversation, recompiles fresh system message; REST route exposed | `sources/letta/letta/services/conversation_manager.py:105-172`, `sources/letta/letta/server/rest_api/routers/v1/conversations.py:122-158` |
| Immutability of history | `modify_message` endpoint returns 405: messages immutable because shared across forks | `sources/letta/letta/server/rest_api/routers/v1/agents.py:1627-1644` |
| Manual compaction APIs | `POST /{conversation_id}/compact`; `summarize_conversation_history(force=True)` | `sources/letta/letta/server/rest_api/routers/v1/conversations.py:1029-1058`, `sources/letta/letta/agents/letta_agent.py:1620-1631` |
| Listing/query interface | Cursor-paginated `list_messages` with role/text/run/conversation filters over `is_deleted == False` | `sources/letta/letta/services/message_manager.py:895-955` |
| Tests: thresholds | GPT-5 vs non-GPT-5 threshold behavior | `sources/letta/tests/test_compaction_thresholds.py:5-45` |
| Tests: static buffer | No-trim, trim-to-user-boundary, JSON-parse-failure, all-user/all-assistant edge cases | `sources/letta/tests/test_static_buffer_summarize.py:41-140` |
| Tests: integration summarize | Empty buffer, small conversation, large tool calls; enforces streaming summarizer path | `sources/letta/tests/integration_test_summarizer.py:199-485`, `sources/letta/tests/integration_test_summarizer.py:249-253` |
| Tests: conversations/fork | Conversation isolation and REST fork sharing non-system messages | `sources/letta/tests/integration_test_conversations_sdk.py:313-400` |

## Answers to Dimension Questions

**1. What conversation history does the model see?**
The full ordered contents of the in-context pointer: `[system message] + [prior summary if any] + [all message_ids since last eviction]`. Default mode hydrates `agent_state.message_ids` (`letta/agents/helpers.py:206-220`); conversation mode hydrates `conversation_messages` where `in_context == True` ordered by `position` (`letta/agents/helpers.py:177-205`, `letta/services/conversation_manager.py:619-630`). If `message_buffer_autoclear=True`, only the system message is loaded each turn (`letta/agents/helpers.py:213-217`). Before each request, inner thoughts are scrubbed according to llm config (`letta/agents/letta_agent.py:1660-1661`) and messages are refreshed each step (`letta/agents/letta_agent_v3.py:965-969`). The compiled memory lives inside the system message, which is rebuilt when blocks change (`letta/agents/base_agent.py:93-186`).

**2. What gets dropped?**
Nothing is deleted — eviction is pointer-rewriting. Legacy static-buffer mode drops all messages between index 1 and the trim boundary (aligned to a `user` message, `letta/services/summarizer/summarizer.py:290-306`); partial-evict drops the oldest 30% up to the nearest assistant message and replaces them with one recursive summary (`letta/services/summarizer/summarizer.py:160-179`). New sliding-window compaction summarizes `messages[1:cutoff]` where cutoff lands on an assistant (or approval-with-tool-calls) message, growing the eviction percentage in 10% increments until the remaining buffer fits the token goal (`letta/services/summarizer/summarizer_sliding_window.py:144-198`). At request-build time, content-level drops also occur: reasoning-only assistant messages may serialize to `None` (`letta/schemas/message.py:1404-1417`), approval requests without tool calls are omitted (`letta/schemas/message.py:1352-1353`), tool returns are truncated to a computed char cap (`letta/agents/letta_agent_v3.py:143-153`), and images become placeholders (`letta/schemas/message.py:1362-1373`). Evicted messages remain queryable in the DB (`list_messages` filters only soft-deletes, `letta/services/message_manager.py:955`).

**3. Are tool messages retained?**
Yes, first-class. Tool returns are persisted with `tool_returns`/`tool_call_id` columns (`letta/orm/message.py:46-57`), serialized back as native provider `tool` messages with required `tool_call_id` (`letta/schemas/message.py:1456-1478`), and compaction explicitly avoids orphaning them: cutoffs must be assistant messages or approvals carrying tool calls (`letta/services/summarizer/summarizer_sliding_window.py:156-161`), pending approval requests are excluded from eviction ranges (`letta/services/summarizer/summarizer_sliding_window.py:133-137`), and static-buffer trimming backs up to include an assistant message paired with a retained approval of the same step (`letta/services/summarizer/summarizer.py:295-304`). Approval flows validate tool-call ID symmetry against persisted history (`letta/agents/helpers.py:102-145`).

**4. Is memory per user/thread/session?**
Scoping is layered, not user-session based. Messages are organization-scoped (`OrganizationMixin`) and agent-scoped (`AgentMixin`) (`letta/orm/message.py:23`); every manager call takes an `actor` for permission checks (`letta/services/message_manager.py:351-376`). The short-term buffer belongs to an *agent* by default (`agent_state.message_ids`) and optionally to a *conversation* (thread) within that agent, with isolated memory blocks available per conversation (`letta/services/conversation_manager.py:894-986`). Runs/steps are tracked via FKs for attribution (`letta/orm/message.py:48-53`). There is no automatic per-end-user session partitioning beyond what identities/conversations the caller creates.

**5. Can history be edited or forked?**
Editing was deliberately removed: the REST `modify_message` endpoint now raises 405 because messages may be shared across forked conversations (`letta/server/rest_api/routers/v1/agents.py:1627-1644`). Forking is supported: `POST /v1/conversations/{id}/fork` creates a new conversation sharing the same non-system Message objects plus a freshly compiled system message capturing latest block values (`letta/services/conversation_manager.py:105-172`); the agent-direct variant forks `message_ids` similarly (`letta/services/conversation_manager.py:175-219`); verified by integration test (`tests/integration_test_conversations_sdk.py:372-400`). Deletion primitives exist at manager level (`delete_message_by_id_async`, `delete_messages_by_ids_async`, `letta/services/message_manager.py:822`, `sources/letta/letta/services/message_manager.py:1094`) and manual compaction/reset are exposed (`POST /{conversation_id}/compact`, `letta/server/rest_api/routers/v1/conversations.py:1029`).

## Architectural Decisions

1. **Durable append-only store + mutable in-context pointer.** Rather than keeping a live transcript object, Letta stores all messages immutably in SQL and maintains a separate ordered ID list defining "in context." Compaction = rewriting the pointer (`_checkpoint_messages`, `letta/agents/letta_agent_v3.py:758-816`; `update_in_context_messages`, `letta/services/conversation_manager.py:752-762`). This makes eviction reversible-by-query and enables recall tooling, but adds a two-table consistency surface (messages vs. pointer).
2. **Summary-as-message.** Compaction output is itself a persisted `Message` with role `summary` (`letta/services/summarizer/compact.py:434-442`), downgraded to `user` role only at serialization time (`letta/schemas/message.py:1396-1402`), wrapped in a JSON `system_alert` envelope carrying machine-readable `compaction_stats` (`letta/system.py:207-236`). Summaries therefore survive restarts and are auditable.
3. **Layered trigger policy: proactive + reactive + manual.** Post-step usage above 90% of window triggers compaction before the next turn (`letta/agents/letta_agent_v3.py:1438-1444`); provider `ContextWindowExceededError` triggers compact-and-retry within the step, bounded by retries (`letta/agents/letta_agent_v3.py:1218-1222`); developers can force compaction via API. Thresholds encode model-specific knowledge (GPT-5 proactive-at-90% rationale documented, `letta/services/summarizer/thresholds.py:33-40`).
4. **Fallback chain across compaction strategies.** `self_compact_all → self_compact_sliding_window → all` and `sliding_window → all` degrade gracefully on failure (`letta/services/summarizer/compact.py:192-346`), followed by a verification pass that re-counts tokens including tools and escalates to a typed error if the system prompt itself cannot fit (`letta/services/summarizer/compact.py:350-412`).
5. **Conversation threads as a first-class layer over agents.** Conversations add per-thread pointers, per-thread block isolation, and fork semantics without duplicating message rows (`letta/services/conversation_manager.py:50-98`, `894-986`) — the reason message mutation was retired.
6. **Self-compaction option.** The `self_compact_*` modes let the agent's own model produce the summary (preserving cache-friendly prefixes; note at `letta/agents/letta_agent_v3.py:1461`), versus cheap dedicated summarizer models for `sliding_window`/`all` modes (`letta/services/summarizer/summarizer_config.py:26-32`).

## Notable Patterns

- **Boundary-aware eviction.** Both generations refuse to cut mid-group: static buffer advances the trim index to a `user` message and protects assistant+approval same-step pairs (`letta/services/summarizer/summarizer.py:290-304`); sliding window snaps cutoffs backward to valid assistant/approval indices and grows eviction in 10% increments (`letta/services/summarizer/summarizer_sliding_window.py:156-191`).
- **Idempotency via history inspection.** Duplicate approval deliveries are detected by scanning recent in-context tool returns (last 10) and, post-compaction, the full DB history (`letta/agents/helpers.py:230-265`) — history doubles as a dedup ledger.
- **Estimate-driven loop control.** The v3 loop trusts provider-reported `total_tokens` from the previous request as its context pressure signal (`letta/agents/letta_agent_v3.py:125-129`, `1306-1307`) instead of recounting locally each step, reserving local counting (`count_tokens_with_tools`) for compaction validation (`letta/services/summarizer/compact.py:350-356`).
- **Env-configurable defaults.** All summarization knobs (mode, buffer limit/min, eviction percentage, retry caps, warning thresholds) are env-tunable via a single settings object with prefix `LETTA_SUMMARIZER_` (`letta/settings.py:74-111`).
- **Transcript rendering for summarizers.** `format_transcript` renders messages as `role: text` lines, skipping empty heartbeats and `send_message` tool noise, substituting image placeholders (`letta/services/summarizer/summarizer.py:654-717`) — the summarizer sees a cleaned narrative, not raw payloads.
- **Checkpoint-on-success persistence.** Messages persist only after a step completes safely; exceptions roll back to previous state (`letta/agents/letta_agent_v3.py:1402-1410`, comment at `1513`), preventing half-written turns from entering context.

## Tradeoffs

- **Full-history-until-threshold vs. fixed window.** Sending the entire pointer contents maximizes continuity and leverages prompt caching (explicitly preserved by skipping system refreshes except around compaction, `letta/agents/letta_agent_v3.py:966-967`), but means long sessions pay growing latency/cost until the 90% cliff, and eviction events cause sharp context discontinuities.
- **Count-based legacy vs. token-based modern compaction.** The legacy partial-evict mode measures eviction in message counts ("using message count instead of token count", `letta/services/summarizer/summarizer.py:145`), which is cheap but imprecise for fat tool returns; the newer pipeline counts tokens properly but requires provider counters or approximations (`letta/services/summarizer/token_counter.py:87-127`).
- **Immutability vs. debuggability.** Making messages immutable protects fork integrity but removes the ability to redact/fix bad content in place; operators must rely on compaction or deletion managers instead.
- **Summary fidelity vs. safety margin.** Sliding-window summaries are character-clipped (`clip_chars` default 50000, `letta/services/summarizer/summarizer_config.py:72-74`) and verified against thresholds, yet a lossy summary permanently replaces detail for future turns (the underlying messages remain in DB but outside context).
- **Autoclear simplicity vs. amnesia.** `message_buffer_autoclear` gives O(1) context but discards all conversational continuity except core-memory blocks — flagged "not recommended unless you have an advanced use case" in its own field description (`letta/schemas/agent.py:139-142`).

## Failure Modes / Edge Cases

- **Config-dependent breakage acknowledged in-code.** "This can be broken by bad configs, e.g. lower bound too high, initial messages too fat" and unused `force`/`clear` params flagged in the legacy rebuild path (`letta/agents/letta_agent.py:1587-1588`).
- **No-valid-cutoff failures.** Partial-evict raises if no assistant message exists in the eviction range (`letta/services/summarizer/summarizer.py:170-175`); sliding window raises if no assistant/approval cutoff can be found or the cutoff would consume the trailing message (`letta/services/summarizer/summarizer_sliding_window.py:179-198`).
- **Unreliable summarizer output.** Static-buffer tests cover JSON parsing failure of summarizer responses (`tests/test_static_buffer_summarize.py:94-111`); compaction falls through the mode chain and finally logs critical but continues rather than bricking the agent (`letta/services/summarizer/compact.py:409-410`).
- **System-prompt overflow is unrecoverable by design.** If the compiled memory/system message alone exceeds the window, compaction aborts with `SystemPromptTokenExceededError` and a distinct stop reason (`context_window_overflow_in_system_prompt`) surfaces to clients (`letta/agents/letta_agent_v3.py:743-756`, `1506-1509`).
- **Stale context estimates.** `context_token_estimate` starts as `None` (warned at `letta/agents/letta_agent_v3.py:935-936`) and reflects only the last completed request, so the first turn of a session skips the proactive check.
- **Bounded idempotency window.** Duplicate-approval detection scans only the 10 most recent in-context messages before falling back to a DB probe conditioned on an empty non-system buffer (`letta/agents/helpers.py:238`, `248-265`); a match older than 10 messages with a non-empty buffer would not be caught.
- **Mid-run exception halts the loop.** Any exception stops stepping to prevent wasteful retries, with rollback semantics (`letta/agents/letta_agent_v3.py:1511-1529`) — safe, but a transient post-step compaction failure ends the run.

## Future Considerations

- Consolidate the two summarization generations: retire the legacy `Summarizer` count-based modes or port their voice-agent-specific behaviors (background self-summarizing block writer, `letta/services/summarizer/summarizer.py:314-341`) onto `CompactionSettings`.
- Replace commented-out proactive-summarization blocks in v2/v3 step loops with a single tested policy hook (`letta/agents/letta_agent_v3.py:365-408`, `620-642`).
- Track context pressure with exact pre-request counting (or cached deltas) rather than last-step usage, closing the first-turn blind spot.
- Generalize the tool-return char-cap heuristic (currently hardcoded 20%/5000 min, `letta/agents/letta_agent_v3.py:143-153`) into `CompactionSettings`.
- Expose retrieval of evicted-but-persisted history as an explicit recall affordance tied to the pointer rewrite (today it is implicit via listing APIs).

## Questions / Gaps

- **Anthropic/Gemini-specific serialization:** this analysis verified OpenAI-format role conversion (`letta/schemas/message.py:1339-1498`, `letta/llm_api/openai_client.py:502-701`) but did not line-audit `llm_api/anthropic_client.py` / `google_*_client.py` equivalents; per-provider divergence risk is unverified here.
- **Legacy `function` role reachability:** `MessageRole.function` exists (`letta/schemas/enums.py:114`) but the Pydantic validator excludes it (`letta/schemas/message.py:319-325`) and `to_openai_dict` raises on it (`letta/schemas/message.py:1480-1481`); likely vestigial, but no deprecation note was found.
- **Multi-agent group message routing:** messages carry `group_id` (`letta/orm/message.py:58`) and listing supports it, but how group participants share or isolate the short-term buffer was out of scope for this dimension and not traced end-to-end.
- **No evidence found** for automatic scheduled/background compaction independent of the step loop (Temporal references appear only in comments, e.g. `letta/services/summarizer/thresholds.py:30-32`); no worker implementation was located within the searched boundary (`letta/agents/`, `letta/services/summarizer/`, `letta/server/rest_api/routers/v1/`).

---

Generated by `05.01-short-term-conversation-memory` against `letta`.
