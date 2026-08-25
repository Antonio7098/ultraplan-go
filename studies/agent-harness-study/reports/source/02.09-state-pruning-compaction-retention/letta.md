# Source Analysis: letta

## State Pruning, Compaction, and Retention

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3 (FastAPI + SQLAlchemy 2.x async + Alembic); PostgreSQL primary, SQLite fallback; Turbopuffer as optional vector store for messages and archival passages; ClickHouse for traces; Redis for token-count caching |
| Analyzed | 2026-07-12 |

## Summary

Letta treats compaction as a first-class, per-agent subsystem (`letta/services/summarizer/`) with four distinct strategies exposed as `mode: Literal["all", "sliding_window", "self_compact_all", "self_compact_sliding_window"]` on `CompactionSettings` (`letta/services/summarizer/summarizer_config.py:48-89`). The trigger threshold defaults to **90 % of context window** (`letta/constants.py:83`), with a model-family override that pushes GPT-5 to proactive 90 % compaction regardless of model (`letta/services/summarizer/thresholds.py:27-41`). Compaction can fire on either (a) a `ContextWindowExceededError` mid-step retry (`letta/agents/letta_agent_v3.py:1218-1294`) or (b) a post-step context check (`letta/agents/letta_agent_v3.py:1439-1509`). State growth outside the in-context window is unbounded — recall memory (full `messages` table) and archival memory (passages) are only ever pruned through explicit user-initiated delete endpoints (`letta/services/message_manager.py:1045-1090`, `letta/services/passage_manager.py:767-893`, `letta/services/conversation_manager.py:556-609`). Soft-delete (`is_deleted`) on `Base` (`letta/orm/base.py:18`) is the universal mutability primitive; `hard_delete_async` is reserved for explicit user actions and cascades. There is **no TTL, age-based retention, or background sweeper** on messages, passages, or runs — conversations and runs grow indefinitely until a user invokes a delete endpoint or runs out of disk.

Replay and debugging preservation is mixed. The compaction result is materialized as a persisted `Message(role=summary)` that lives in the same `messages` table and the same `conversation_messages` junction (`letta/services/summarizer/compact.py:434-466`, `letta/services/conversation_manager.py:752-797`); this means a user inspecting conversation history sees the compaction event. However, the **evicted source messages are kept in the DB** (only their `in_context` flag is flipped on the junction table), so replay remains possible through the recall search path (`letta/services/message_manager.py:1142-1170`). Block-level checkpoint history (`BlockHistory`) is a strictly linear, append-only sequence with explicit future-truncation on a new checkpoint (`letta/services/block_manager.py:840-908`); no retention bound is enforced — `undo_checkpoint_block`/`redo_checkpoint_block` walk as far as the table allows (`letta/services/block_manager.py:952-1048`).

Privacy / deletion is fully supported for in-app objects: agents hard-delete with cascade (`letta/services/agent_manager.py:1320-1396`); conversations soft-delete junction rows and hard-delete any isolated blocks (`letta/services/conversation_manager.py:556-609`); individual messages and passages are individually removable with dual-write to Turbopuffer when enabled (`letta/services/message_manager.py:822-851`, `letta/services/passage_manager.py:767-893`). However there is **no GDPR-style account-level purge job**, no `expires_at` column on any core table (`letta/orm/base.py:14-18`, `letta/orm/message.py:30-35`), and the only background sweeper is for OAuth session expiry (`letta/services/mcp/oauth_utils.py:260-272`, `letta/services/mcp_manager.py:1071-1087`).

Compaction observability is good: `compaction_stats` are embedded in the summary payload (`letta/system.py:207-237`), surfaced via the `SummaryMessage.compaction_stats` schema (`letta/schemas/letta_message.py:411`), and a structured `EventMessage(event_type="compaction")` is emitted before compaction begins (`letta/agents/letta_agent_v3.py:818-846`). Provider traces carry `compaction_settings` dict for summarization calls (`letta/schemas/provider_trace.py:41-61`).

## Rating

**7 / 10 — Clear, mature, multi-strategy compaction model with first-class observability and replay preservation; weakened by absence of TTL/sweeper on recall and archival memory, no automatic conversation expiry, and partial-mode (voice) summarizer that still uses ad-hoc message-count buffers.**

Rationale:
- (+) Four compaction strategies (`all`, `sliding_window`, `self_compact_all`, `self_compact_sliding_window`) with explicit fallback chains (sliding_window → all, self_compact_all → self_compact_sliding_window → all) (`letta/services/summarizer/compact.py:189-348`).
- (+) Compaction triggers are explicit and tested: trigger threshold is a model-aware function (`letta/services/summarizer/thresholds.py:27-41`), with `test_compaction_thresholds.py:23-53` covering GPT-5 vs non-GPT-5 behavior.
- (+) In-context message history is preserved on compaction — the summary becomes a first-class `Message(role=summary)` so the user can rewind (`letta/services/summarizer/compact.py:434-466`, `letta/schemas/letta_message.py:411`).
- (+) Compaction stats (`trigger`, `context_tokens_before/after`, `messages_count_before/after`, `mode`) are persisted into the summary message and surfaced to the client (`letta/agents/letta_agent_v3.py:818-892`, `letta/system.py:207-237`).
- (+) Multi-tier context-window fallback inside the summarizer itself: tool-return clamping → middle-truncate the transcript → error (`letta/services/summarizer/summarizer.py:565-647`).
- (+) Provider-overload fallback for summarization routes Anthropic → Bedrock Opus and ZAI → Baseten GLM-5 (`letta/services/summarizer/summarizer.py:714-816`).
- (+) Explicit user-facing REST endpoints for both agent-level (`/agents/{id}/summarize`, `letta/server/rest_api/routers/v1/agents.py:2430-2508`) and conversation-level (`/conversations/{id}/compact`, `letta/server/rest_api/routers/v1/conversations.py:1029-1136`) compaction.
- (−) **No TTL or background sweeper** on `messages`, `passages`, `runs`, or `conversations`. The `Base` ORM mixin only has `created_at`/`updated_at`/`is_deleted` (`letta/orm/base.py:14-18`); there is no `expires_at` anywhere.
- (−) Recall memory (`messages` table) is only pruned by explicit `delete_messages_by_ids_async` or `delete_all_messages_for_agent_async` (`letta/services/message_manager.py:1045-1138`). A long-running agent accumulates unbounded rows unless the user calls `/messages/delete`.
- (−) Archival memory (`ArchivalPassage`) has no size limit enforced server-side; agents can insert via tools and grow without bound (`letta/services/passage_manager.py:857-893`).
- (−) `SummarizationMode.PARTIAL_EVICT_MESSAGE_BUFFER` legacy path has a "this can't be made sync" comment and re-imports `summarizer_settings` mid-function — it's marked TODO for migration (`letta/services/summarizer/summarizer.py:185-186`).
- (−) Voice/sleeptime path uses message-count buffer (`message_buffer_limit` default 60, `message_buffer_min` default 15, `letta/settings.py:79-80`) rather than token budget, decoupling the voice agent from the main agent's token-aware compaction (`letta/agents/voice_agent.py:88-113`, `letta/agents/voice_sleeptime_agent.py:60-64`).
- (−) Conversation `update_in_context_messages` rewrites positions atomically after compaction (`letta/services/conversation_manager.py:752-797`), but the source messages are *not* deleted — they accumulate forever. The recall path still finds them via `search_messages_async`.
- (−) `InMemoryStore`/`InMemorySaver`-style in-memory fallback does not exist — Letta always persists, but the dual-write to Turbopuffer is best-effort with `strict_mode` opt-in (`letta/services/message_manager.py:1086-1087`, `letta/services/passage_manager.py:789-792`). A failed dual-write can leave vector and SQL stores inconsistent.
- (−) `compact_messages` can succeed without reducing token count; the manual `/summarize` endpoint guards this with HTTP 400 (`letta/server/rest_api/routers/v1/agents.py:2497-2502`), but the in-loop `post_step_context_check` path only logs `logger.critical` and continues (`letta/services/summarizer/compact.py:410`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Summarizer module root | Directory containing all compaction logic | `letta/services/summarizer/__init__.py` |
| Compaction modes enum (legacy class) | `STATIC_MESSAGE_BUFFER`, `PARTIAL_EVICT_MESSAGE_BUFFER` | `letta/services/summarizer/enums.py:4-10` |
| Modern strategy modes | `Literal["all", "sliding_window", "self_compact_all", "self_compact_sliding_window"]` | `letta/services/summarizer/summarizer_config.py:76-78` |
| Per-agent compaction settings (Pydantic) | `CompactionSettings` model with `mode`, `sliding_window_percentage`, `clip_chars`, `prompt_acknowledgement`, `model`, `model_settings` | `letta/services/summarizer/summarizer_config.py:48-89` |
| Default summarizer model per provider | Haiku 4-5 for Anthropic, GPT-5-mini for OpenAI, Gemini 2.5 flash for Google, letta/auto | `letta/services/summarizer/summarizer_config.py:26-32` |
| Default mode-specific prompts | `get_default_prompt_for_mode` — `ALL_PROMPT`, `SLIDING_PROMPT`, `SELF_ALL_PROMPT`, `SELF_SLIDING_PROMPT` | `letta/services/summarizer/summarizer_config.py:35-45` |
| `SummarizerSettings` global config | `mode`, `message_buffer_limit=60`, `message_buffer_min=15`, `enable_summarization=True`, `max_summarizer_retries=3`, `partial_evict_summarizer_percentage=0.30`, `memory_warning_threshold=0.75` | `letta/settings.py:74-109` |
| Compaction trigger threshold (default 90%) | `SUMMARIZATION_TRIGGER_MULTIPLIER = 0.9` | `letta/constants.py:81-83` |
| Model-aware trigger threshold | GPT-5 family gets proactive 90 %; others 100 % | `letta/services/summarizer/thresholds.py:27-41` |
| Compact dispatch | Routes to `self_summarize_all`, `self_summarize_sliding_window`, `summarize_all`, or `summarize_via_sliding_window` with fallback chains | `letta/services/summarizer/compact.py:189-348` |
| Summary message creation | `Message(role=summary)` with compaction_stats in packed JSON | `letta/services/summarizer/compact.py:434-466` |
| `summarize_all` — protect system + last assistant/approval | Cannot evict pending approval; preserves assistant paired with same `step_id` | `letta/services/summarizer/summarizer_all.py:41-61` |
| `summarize_via_sliding_window` — iterative eviction % loop | Increases eviction by 10 % until post-summarize tokens < goal | `letta/services/summarizer/summarizer_sliding_window.py:163-198` |
| Self-summarization path (agent summarizes itself) | `_get_protected_messages`, dummy assistant insertion, agent's LLM used as summarizer | `letta/services/summarizer/self_summarizer.py:24-289` |
| Three-tier summarizer fallback (clamping) | Tool-return clamp → middle-truncate transcript → error | `letta/services/summarizer/summarizer.py:565-647` |
| Provider-overload fallback for summarization | Anthropic → Bedrock Opus, ZAI → Baseten GLM-5 | `letta/services/summarizer/summarizer.py:714-816` |
| Provider fallback in auto mode | `letta/auto` summarizer routes to Haiku 4-5, falls back to zai/glm-5 | `letta/services/summarizer/compact.py:65-77` |
| Legacy partial-evict path | `_partial_evict_buffer_summarization` constructs a summary user-message and persists it | `letta/services/summarizer/summarizer.py:136-242` |
| Legacy static-buffer path | `_static_buffer_summarization` trims to `message_buffer_min`, fires summarizer agent | `letta/services/summarizer/summarizer.py:244-343` |
| `simple_summary` truncation constants | `TOOL_RETURN_TRUNCATION_CHARS = 5000` for clamping tool returns | `letta/constants.py:443`, `letta/services/summarizer/summarizer.py:572-575` |
| Trigger threshold call site (v3 in-loop) | `compaction_trigger_threshold = get_compaction_trigger_threshold(self.agent_state.llm_config)` | `letta/agents/letta_agent_v3.py:938` |
| Retry on `ContextWindowExceededError` (max_summarizer_retries=3) | Compact → recompile system prompt → retry | `letta/agents/letta_agent_v3.py:1218-1294` |
| Post-step context check | Compact if `context_token_estimate > compaction_trigger_threshold` | `letta/agents/letta_agent_v3.py:1439-1509` |
| Compaction `EventMessage` | Emitted before compaction begins, with `trigger` and `context_token_estimate` | `letta/agents/letta_agent_v3.py:818-846` |
| `SummaryMessage` schema | Carries `compaction_stats` dict to client | `letta/schemas/letta_message.py:411` |
| `compaction_stats` packing | `package_summarize_message_no_counts` serializes trigger/mode/tokens before/after into JSON envelope | `letta/system.py:207-237` |
| Manual `/agents/{id}/summarize` endpoint | Validates reduction; returns CompactionResponse | `letta/server/rest_api/routers/v1/agents.py:2430-2508` |
| Manual `/conversations/{id}/compact` endpoint | Per-conversation compaction, validates reduction | `letta/server/rest_api/routers/v1/conversations.py:1029-1136` |
| Voice agent compaction | Uses `Summarizer(STATIC_MESSAGE_BUFFER)` with buffer of 20 / min 10 | `letta/agents/voice_agent.py:88-113` |
| Voice sleeptime compaction | Same path with 20/10 | `letta/agents/voice_sleeptime_agent.py:60-64` |
| Group sleeptime v2 compaction | Buffer 20/min 8 (TODO to make configurable) | `letta/groups/sleeptime_multi_agent_v2.py:317-318` |
| Conversation `delete_conversation` | Soft-delete junction rows; soft-delete messages that aren't shared with another conversation; hard-delete isolated blocks | `letta/services/conversation_manager.py:556-609` |
| Conversation in-context flag update after compaction | `update_in_context_messages` rewrites positions atomically | `letta/services/conversation_manager.py:752-797` |
| Soft-delete mixin | `is_deleted: bool` on every ORM table | `letta/orm/base.py:18` |
| `delete_async` (soft) vs `hard_delete_async` (irreversible) | Base ORM provides both | `letta/orm/sqlalchemy_base.py:673-706` |
| Agent hard-delete cascade | Calls `session.delete(agent)`; SQLAlchemy cascades per FK `ondelete` rules | `letta/services/agent_manager.py:1320-1396` |
| Message single hard-delete with Tpuf dual-write | `delete_message_by_id_async` | `letta/services/message_manager.py:822-851` |
| Bulk message hard-delete | `delete_messages_by_ids_async` and `delete_all_messages_for_agent_async` (with `exclude_ids` and Tpuf cascade) | `letta/services/message_manager.py:1045-1138` |
| Archival passage delete (single) | `delete_agent_passage_by_id_async` — SQL + Turbopuffer | `letta/services/passage_manager.py:767-794` |
| Source passage delete | `delete_source_passage_by_id_async` | `letta/services/passage_manager.py:800-811` |
| Bulk archival passage delete | `delete_agent_passages_async` — `bulk_hard_delete_async` + Tpuf | `letta/services/passage_manager.py:857-893` |
| Block delete | `delete_block_async` (junctions + hard delete) | `letta/services/block_manager.py:275-286` |
| Git-backed block delete | `delete_block_async` writes a git commit, deletes junctions, hard deletes | `letta/services/block_manager_git.py:327-351` |
| Git-backed block hard-delete from Postgres | `_delete_block_from_postgres` | `letta/services/block_manager_git.py:152-180` |
| Block checkpoint (linear history) | `checkpoint_block_async` truncates future BlockHistory beyond current seq | `letta/services/block_manager.py:842-908` |
| Block undo/redo | `undo_checkpoint_block` walks `sequence_number < current` desc | `letta/services/block_manager.py:952-998` |
| Block redo | `redo_checkpoint_block` walks `sequence_number > current` asc | `letta/services/block_manager.py:1004-1048` |
| File delete | `delete_file` hard-deletes with no retention check | `letta/services/file_manager.py:479-487` |
| Run delete | `delete_run` hard-deletes; cleanup-of-approval handled in cancel path | `letta/services/run_manager.py:308-315`, `letta/services/run_manager.py:669-699` |
| Group delete | `delete_group_async` hard-deletes | `letta/services/group_manager.py:192-195` |
| Job delete | `delete_job_by_id_async` hard-deletes | `letta/services/job_manager.py:309-313` |
| Source delete | `delete_source` hard-deletes | `letta/services/source_manager.py:234-238` |
| Sandbox config delete | `delete_sandbox_config_async` and `delete_sandbox_env_var_async` hard-delete | `letta/services/sandbox_config_manager.py:143-147`, `letta/services/sandbox_config_manager.py:269-273` |
| LLM batch delete | `delete_llm_batch_request_async`, `delete_llm_batch_item_async` | `letta/services/llm_batch_manager.py:145-149`, `letta/services/llm_batch_manager.py:470-474` |
| Identity delete | `delete_identity_async` | `letta/services/identity_manager.py:251` |
| OAuth session expiry sweeper | `cleanup_expired_oauth_sessions(max_age_hours=24)` | `letta/services/mcp/oauth_utils.py:260-272`, `letta/services/mcp_manager.py:1071-1087` |
| OAuth session expiry sweeper (server-side path) | `cleanup_expired_oauth_sessions` | `letta/services/mcp_server_manager.py:1267-1283` |
| Token count caching (Redis TTL=3600) | `@async_redis_cache(..., ttl_s=3600)` on Anthropic/Gemini/Tiktoken counters | `letta/services/context_window_calculator/token_counter.py:49-262` |
| `ContextWindowCalculator` system components | Extracts `<base_instructions>`, `<memory_blocks>`, `<memory_filesystem>` from system prompt | `letta/services/context_window_calculator/context_window_calculator.py:167-211` |
| Context-window overview (per-component token counts) | `ContextWindowOverview` aggregates system/core_memory/summary_memory/messages/tools tokens | `letta/services/context_window_calculator/context_window_calculator.py:249-384` |
| `extract_summary_memory` | Detects summary at message index 1 by signature phrase | `letta/services/context_window_calculator/context_window_calculator.py:213-247` |
| Token counter factory | Anthropic → Anthropic API, Gemini → Gemini API, others → ApproxTokenCounter (`bytes/4`) | `letta/services/context_window_calculator/token_counter.py:268-316` |
| ApproxTokenCounter safety margin | `APPROX_TOKEN_SAFETY_MARGIN = 1.3` | `letta/services/summarizer/summarizer_sliding_window.py:24` |
| Block char limit | `CORE_MEMORY_BLOCK_CHAR_LIMIT = 100000` | `letta/constants.py:435` |
| Tool return char limit | `FUNCTION_RETURN_CHAR_LIMIT = 50000` | `letta/constants.py:438-439` |
| `clip_chars` default on CompactionSettings | `clip_chars: int | None = 50000` | `letta/services/summarizer/summarizer_config.py:72-74` |
| Summary char truncation | `SUMMARY_TRUNCATION_SUFFIX = "... [summary truncated to fit]"` | `letta/services/summarizer/constants.py:1-3` |
| Token-counter cache | Redis cache for token counts (1h TTL) | `letta/services/context_window_calculator/token_counter.py:49-253` |
| Compaction stat extraction at replay time | `extract_compaction_stats_from_packed_json` | `letta/agents/letta_agent_v3.py:81-94`, `letta/system.py:233-234` |
| Token counting includes tools | `count_tokens_with_tools` sums message + tool definition tokens | `letta/services/summarizer/summarizer_sliding_window.py:45-95` |
| Threshold tests | Unit tests covering GPT-5 vs non-GPT-5 thresholds | `letta/tests/test_compaction_thresholds.py:23-53` |
| Static-buffer summarizer unit tests | Test no-trim, trim, user-message alignment, JSON parsing failure, all-assistant trim | `letta/tests/test_static_buffer_summarize.py:1-150` |
| Conversation compact integration tests | `test_compact_conversation_history_messages` covers threshold behaviors | `letta/tests/integration_test_conversations_sdk.py:1418-1456` |
| Summarizer streaming-path integration | Anthropic summarization must use streaming to avoid 10-min timeouts | `letta/tests/integration_test_summarizer.py:247-253` |
| Provider trace tagging for summarization | `LLMCallType.summarization` set on `llm_client.set_telemetry_context` | `letta/services/summarizer/summarizer.py:519-530` |
| `Base` ORM audit columns | `created_at`, `updated_at`, `is_deleted` (no `expires_at`) | `letta/orm/base.py:14-18` |
| Index on `messages` | Composite indexes on `(agent_id, conversation_id, sequence_id)` and `(agent_id, sequence_id)` for recall pagination | `letta/orm/message.py:30-35` |

## Answers to Dimension Questions

1. **What grows forever?**
   - **`messages` table** — recall memory. Compaction flips the in-context flag on the junction (`conversation_messages`) but does **not** delete the source rows. A long-lived agent accumulates rows indefinitely (`letta/services/conversation_manager.py:752-797`, `letta/services/summarizer/compact.py:434-466`).
   - **`archival_passages`** — only manually pruned via `delete_agent_passages_async` (`letta/services/passage_manager.py:857-893`).
   - **`block_history`** — strictly append-only per block; no retention bound. `checkpoint_block_async` truncates future sequences beyond current but never deletes old ones (`letta/services/block_manager.py:842-908`).
   - **`runs` / `steps` / `run_metrics`** — hard-deleted only on explicit `/runs/{id}/delete` (`letta/services/run_manager.py:308-315`). No TTL.
   - **Git-backed memory store** — per-agent repo; never garbage-collected; only deleted via `delete_repo` on agent teardown (`letta/services/memory_repo/git_operations.py:629-637`).
   - **Provider traces / clickhouse** — TTL is not configured in code (only OAuth sessions have a sweeper).

2. **What gets summarized?**
   - In-context messages for the active conversation / agent-direct default. The chosen subset (`messages[1:assistant_index]`) is passed to an LLM summarizer via `simple_summary` (`letta/services/summarizer/summarizer.py:487-651`) and the result is persisted as a single `Message(role=summary)` placed at index 1 (`letta/services/summarizer/compact.py:434-466`).
   - The summarizer can be: (a) a different (typically cheaper) model specified by `CompactionSettings.model` (defaults per provider — Haiku 4-5 for Anthropic, GPT-5-mini for OpenAI), or (b) the agent's own LLM in `self_compact_*` modes (`letta/services/summarizer/summarizer_config.py:26-32`, `letta/services/summarizer/self_summarizer.py:24-289`).
   - System prompt and last assistant/approval message are protected from summarization (`letta/services/summarizer/summarizer_all.py:41-61`, `letta/services/summarizer/summarizer_sliding_window.py:134-138`, `letta/services/summarizer/self_summarizer.py:265-289`).

3. **What gets deleted?**
   - **Soft-deleted (toggle `is_deleted`):** conversations, conversation_messages, messages belonging to a single conversation (`letta/services/conversation_manager.py:556-609`). Records remain in DB.
   - **Hard-deleted (irreversible row removal):** agents (with FK cascade), isolated blocks attached to a conversation, individual messages / messages-by-ids / all-messages-for-agent (with Turbopuffer dual-write), passages (single + bulk, with Turbopuffer dual-write), blocks, sources, tools, files, jobs, batch requests/items, runs, groups, sandbox configs/env vars, identities, MCP servers/tools/oauth sessions.
   - **TTL-based deletion:** OAuth sessions older than 24h via `cleanup_expired_oauth_sessions` (`letta/services/mcp/oauth_utils.py:260-272`). This is the only background sweeper in the codebase.
   - **Tool-result inline truncation:** `TOOL_RETURN_TRUNCATION_CHARS=5000` clamps tool returns before summarization, but the source `Message` is not modified (`letta/services/summarizer/summarizer.py:572-575`, `letta/constants.py:443`).
   - **Block future-checkpoint truncation:** truncates BlockHistory rows with `sequence_number > current_seq` on each new checkpoint (`letta/services/block_manager.py:874-883`).

4. **Does compaction break replay?**
   - **In-context replay:** No — the summary is a first-class persisted `Message`, the in-context list is rebuilt as `[system, summary, ...compacted_tail]`. `extract_summary_memory` finds it by signature phrase (`letta/services/context_window_calculator/context_window_calculator.py:213-247`).
   - **Full-history replay:** Yes — evicted messages remain queryable via the recall search path (`search_messages_async`, `letta/services/message_manager.py:1142-1170`). The compaction does **not** delete them; it just removes them from `conversation_messages` junction. Pagination via `sequence_id` is intact (`letta/services/message_manager.py:1002-1030`).
   - **Block-history replay:** Yes — BlockHistory rows are never deleted (only future-truncated beyond current), so `undo_checkpoint_block` and `redo_checkpoint_block` work for as long as the rows exist (`letta/services/block_manager.py:952-1048`).
   - **Bug-era replay risk:** legacy `_partial_evict_buffer_summarization` inserts a `role=user` summary message that the heuristic in `extract_summary_memory` recognizes by content but older replay code that doesn't understand `role=summary` may misinterpret (`letta/services/summarizer/compact.py:434-466` vs `letta/services/context_window_calculator/context_window_calculator.py:213-247`).
   - **Determinism risk:** `package_summarize_message_no_counts` packs `compaction_stats` and `mode` into JSON; `extract_compaction_stats_from_message` parses it back. JSON-encoding of datetimes is environment-dependent unless carefully serialized (`letta/system.py:207-237`).

5. **Can users request deletion?**
   - **Yes, comprehensively.** Manual endpoints exist for: `/agents/{id}` (DELETE → `delete_agent_async`), `/conversations/{id}` (DELETE → `delete_conversation`), `/messages/{id}` (DELETE), `/archival-memory/{id}` (DELETE), `/sources/{id}` (DELETE), `/tools/{id}` (DELETE), `/files/{id}` (DELETE), `/runs/{id}` (DELETE), `/jobs/{id}` (DELETE), `/groups/{id}` (DELETE), `/blocks/{id}` (DELETE).
   - **Manual compaction:** `POST /agents/{id}/summarize` and `POST /conversations/{id}/compact` (`letta/server/rest_api/routers/v1/agents.py:2430-2508`, `letta/server/rest_api/routers/v1/conversations.py:1029-1136`). Both validate that message count actually decreased; return HTTP 400 otherwise.
   - **Account-level purge:** No dedicated user-data-export / GDPR endpoint found. `delete_actor_by_id_async` hard-deletes a user (`letta/services/user_manager.py:79-84`) but no code path I located exposes a public REST endpoint that calls it. `delete_organization_by_id_async` exists (`letta/services/organization_manager.py:79-83`) but is also not wired to a documented public endpoint in this codebase slice.

## Architectural Decisions

- **Per-agent `compaction_settings` is the unit of policy, not global config.** Stored on `agents.compaction_settings` (`letta/orm/agent.py:86`, `letta/schemas/agent.py:96-98`). Override-by-request supported at the REST layer via `model_fields_set` merge (`letta/server/rest_api/routers/v1/agents.py:2466-2485`).
- **Compaction summary is persisted as a `Message`, not as a side-table.** This makes the compaction event observable in conversation history, replayable, and queryable. The cost is that the same `Message` table must store both real messages and synthetic summaries, distinguished by `MessageRole.summary` (`letta/schemas/enums.py` — referenced throughout).
- **Compaction never deletes source messages.** The framework distinguishes "in-context" (junction table flag) from "exists" (row in `messages`). This preserves audit/replay at the cost of unbounded row growth.
- **Mode-specific prompts and provider-specific default summarizer models** are decoupled from the agent's own LLM, so a Claude agent can be summarized by Haiku and a GPT-5 agent by GPT-5-mini (`letta/services/summarizer/summarizer_config.py:26-45`, `letta/services/summarizer/compact.py:65-77`).
- **Defensive triple-fallback inside summarization**: provider overload → Bedrock/Baseten, context window exceeded → tool-return clamp → middle-truncate → error. Failure modes are surfaced via `logger.critical` rather than silently swallowed (`letta/services/summarizer/summarizer.py:562-647`, `letta/services/summarizer/compact.py:409-410`).
- **Trigger threshold is model-family aware**, with proactive 90 % compaction for GPT-5 family to avoid max_output_tokens overruns (`letta/services/summarizer/thresholds.py:27-41`). Documented motivation lives in the function docstring.
- **Soft-delete as the default mutability primitive** with `is_deleted` flag on every table (`letta/orm/base.py:18`). Hard-delete is reserved for explicit user-initiated teardown.
- **Token counting is pluggable** (`AnthropicTokenCounter`, `GeminiTokenCounter`, `TiktokenCounter`, `ApproxTokenCounter`) with Redis-backed 1h TTL cache to avoid recounting (`letta/services/context_window_calculator/token_counter.py:49-253`).
- **Compaction stats are first-class telemetry** — they ride along inside the summary message body as JSON, so any consumer that can read messages can read compaction history (`letta/system.py:207-237`, `letta/schemas/letta_message.py:411`).

## Notable Patterns

- **Two-stage state**: in-context (`conversation_messages` junction with `in_context=True`) vs all messages (`messages` table). Compaction only flips the in-context flag; rows are not destroyed. Lets recall search keep working post-compaction (`letta/services/conversation_manager.py:752-797`).
- **Strategy + fallback dispatch table** in `compact_messages`: every modern strategy has an explicit "if X fails, fall back to Y" path (`letta/services/summarizer/compact.py:192-348`). This is the strongest piece of defensive design in the compaction module.
- **Provider-overload fallback for the summarizer itself** (Anthropic → Bedrock Opus, ZAI → Baseten GLM-5) decouples summarizer availability from agent availability — a stressed Anthropic org doesn't break all in-flight compactions (`letta/services/summarizer/summarizer.py:714-816`).
- **Linear checkpoint history** in `BlockHistory` with explicit future-truncation on a new checkpoint. Tradeoff: simple undo/redo, no branching timelines (`letta/services/block_manager.py:842-908`).
- **Conversation-aware message accounting**: a message shared between two conversations is *not* hard-deleted until all conversations referencing it are deleted (`letta/services/conversation_manager.py:586-597`). This is the only concurrency-safe delete path I located.
- **Pre-compaction `EventMessage` for stream consumers** lets clients show a "compacting…" UI state before the compacted messages arrive (`letta/agents/letta_agent_v3.py:818-846`).
- **Redis-cached token counts** with a 1h TTL — practical decision that bounds the cost of repeated `count_tokens_with_tools` calls during sliding-window eviction (`letta/services/context_window_calculator/token_counter.py:49-262`).

## Tradeoffs

- **Unlimited recall growth in exchange for replay.** The system has no way to bound `messages` row count over time. A year-long agent with 100 turns/day accumulates 36 k rows; this is fine on Postgres with the right indexes (`letta/orm/message.py:30-35`) but creates slow `search_messages_async` over time.
- **No automatic user-data expiry vs predictable cache.** OAuth sessions are the only thing with a sweeper (`letta/services/mcp/oauth_utils.py:260-272`). Everything else relies on either user actions or operator-level DB hygiene.
- **ApproxTokenCounter underestimates, safety margin 1.3× applied.** This may cause earlier-than-needed compaction in edge cases (`letta/services/summarizer/summarizer_sliding_window.py:21-42`).
- **Static-buffer mode in voice agent uses raw message count, not tokens.** This makes voice compaction predictable but uncorrelated with the main agent's `context_window` (`letta/agents/voice_agent.py:88-113`, `letta/agents/voice_sleeptime_agent.py:60-64`).
- **Dual-write to Turbopuffer is best-effort with `strict_mode` opt-in.** A partial failure can leave vector store and SQL store inconsistent. Caller decides whether to raise (`letta/services/passage_manager.py:789-792`, `letta/services/message_manager.py:1084-1087`).
- **Compaction can fail to reduce** — when the summarizer can't fit, code logs `logger.critical` and continues with no size reduction. The manual endpoints catch this with HTTP 400; the in-loop path does not (`letta/services/summarizer/compact.py:409-410`, `letta/server/rest_api/routers/v1/agents.py:2497-2502`).
- **`role=summary` summary messages depend on a parser signature** ("The following is a summary of the previous") to be detected by `extract_summary_memory` (`letta/services/context_window_calculator/context_window_calculator.py:241`). If the summary text drifts, the heuristic misses it and double-counts in token reports.

## Failure Modes / Edge Cases

- **GPT-5 family proactive 90 % trigger**: mis-tuned `sliding_window_percentage` can cause repeated compactions on small steps because the threshold is below `context_window` and the summary token count exceeds goal (`letta/services/summarizer/thresholds.py:27-41`).
- **`post_step_context_check` failure cascade**: if compaction fails (LLM overload, sliding-window loop never converges on `eviction_percentage < 1.0`), the agent stops with `StopReasonType.error` (`letta/agents/letta_agent_v3.py:1511-1529`, `letta/services/summarizer/summarizer_sliding_window.py:193-198`).
- **Turbopuffer inconsistency**: best-effort dual-write in `delete_messages_by_ids_async` / `delete_agent_passages_async` will silently leave vector store containing deleted items unless `strict_mode=True` (`letta/services/message_manager.py:1084-1087`, `letta/services/passage_manager.py:883-891`).
- **No assistant message found**: sliding-window raises `ValueError("No assistant message found for sliding window summarization")` after iterating `eviction_percentage < 1.0`. Caller catches as `Exception` and falls back to `summarize_all` (`letta/services/summarizer/summarizer_sliding_window.py:193-198`, `letta/services/summarizer/compact.py:323-346`).
- **`memory_warning_threshold = 0.75` warn-only path** in legacy `agent.py` is gated on `summarizer_settings.send_memory_warning_message = False` by default — the warning system is dormant out of the box (`letta/settings.py:103`, `letta/agent.py:808`).
- **Approval-pair protection**: if the assistant message and the approval message have different `step_id`, only the approval is protected; the assistant message can be evicted, breaking replay for that step (`letta/services/summarizer/summarizer_all.py:47-58`, `letta/services/summarizer/summarizer_sliding_window.py:134-138`).
- **Compaction stats in `compaction_stats` are JSON-encoded datetimes** — the parser uses `extract_compaction_stats_from_packed_json` which assumes a specific schema (`letta/system.py:207-237`). Schema drift silently drops the stats.

## Future Considerations

- **Retention policy for `messages` and `archival_passages`.** Add `expires_at` to `Base` ORM and a background sweeper analogous to `cleanup_expired_oauth_sessions`. Letta already has the soft-delete primitive; only the scheduling is missing.
- **Account-level deletion endpoint.** Wire `delete_actor_by_id_async` and `delete_organization_by_id_async` to documented REST endpoints for compliance / GDPR workflows.
- **Consistency contract on dual-write.** Make Turbopuffer dual-write strict by default, or surface a per-call boolean in the request to opt into eventual consistency.
- **Replace `memory_warning_threshold` 0.75 default-OFF gate** with a metric-exporting path so operators can see compaction pressure without flipping the boolean.
- **Migrate voice agents to token-aware compaction.** Currently `message_buffer_limit` decouples them from the agent's actual context window.
- **Unify `compact_messages` return path** so `post_step_context_check` and manual endpoint use the same reduction-validation guard.
- **Block-history retention bound.** Currently unbounded; a "keep last N checkpoints" config would bound `BlockHistory` growth.
- **In-memory cache invalidation on compaction.** The `async_redis_cache` for token counts uses 1h TTL — across compaction events, stale cached counts can mislead the trigger threshold check.

## Questions / Gaps

- Where, if anywhere, is `delete_actor_by_id_async` exposed to the public REST surface? `letta/services/user_manager.py:79-84` exists but no router wiring was located in this slice.
- Is there a per-organization retention SLA or operator job that bulk-cleans old `messages` / `runs` rows? Searched for `retention`, `cleanup_expired`, and `sweep` — only OAuth sessions surfaced. No evidence found.
- How is `compaction_stats` validated across schema evolution? The packing code in `letta/system.py:207-237` embeds `compaction_stats` as JSON but there is no migration policy for that schema when compaction parameters change.
- Does `update_in_context_messages` correctly handle the case where compaction produces a summary but `in_context_message_ids` argument is missing summary message IDs? The function sets `in_context = message_id in in_context_set` — if summary is omitted, it would be flipped to `False` even though it should remain in context (`letta/services/conversation_manager.py:752-797`).
- No clear evidence found for automatic cleanup of `runs` / `step_metrics` / provider traces older than N days. Only `cleanup_expired_oauth_sessions` exists. This is the largest year-of-runtime risk.
- Where is the canonical client-side detection that compaction happened? The `SummaryMessage.message_type` is referenced but the consumer list in `letta/schemas/letta_message.py` is not enumerated in this slice.

---

Generated by `02.09-state-pruning-compaction-and-retention` against `letta`.