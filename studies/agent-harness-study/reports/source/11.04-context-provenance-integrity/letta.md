# Source Analysis: letta

## Context Provenance and Integrity (Dimension 11.04)

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas, Alembic migrations) |
| Analyzed | 2026-08-25 |

All citations below are relative to the source root (`sources/letta/`). Line numbers verified against the working tree at analysis time.

## Summary

Letta's provenance model is strong at the **storage and API layers** but deliberately stripped at the **model-facing boundaries**. Every persisted context primitive carries lineage: archival/file passages store `source_id`, `file_id`, `file_name`, `archive_id`, free-form `metadata`, and `tags` (`letta/schemas/passage.py:21-32`); messages carry `step_id`, `run_id`, `otid`, `group_id`, `sender_id`, `conversation_id`, and `batch_item_id` (`letta/schemas/message.py:292-310`); blocks carry template lineage (`template_name`, `template_id`, `base_template_id`, `deployment_id`, `entity_id`) plus audit IDs (`letta/schemas/block.py:24-30,73-74`). Freshness is pervasive: every ORM row gets DB-level `created_at`/`updated_at` (`letta/orm/base.py:16-17`), files track `last_accessed_at` which is refreshed on open/close/search (`letta/services/files_agents_manager.py:91,140-166,439-457`), and both memory-search tools surface timestamps — including relative "time_ago" strings — to the model (`letta/services/tool_executor/core_tool_executor.py:176-232`). Transformation history has two concrete mechanisms: context compaction embeds a machine-readable `compaction_stats` block (token/message counts before/after, trigger, mode) into the in-context summary payload (`letta/system.py:207-236`, `letta/services/summarizer/compact.py:415-430`), and core-memory blocks have an explicit checkpoint/undo/redo history table with actor attribution (`letta/orm/block_history.py:12-46`, `letta/services/block_manager.py:841-911`), optionally backed by a full git repository for versioned memory (`letta/services/block_manager_git.py:1-40`). Trust/authority annotation is absent entirely. The decisive weakness is boundary stripping: the agent-facing `archival_memory_search` returns `{id, timestamp, content, tags}` but omits the `file_id`/`source_id` that the database holds (`letta/services/agent_manager.py:2651`), provider-format conversion reduces messages to bare `role`+`content` (`letta/schemas/message.py:1339-1420`), and agent-file export deletes and regenerates all object identities (`letta/serialize_schemas/marshmallow_block.py:17-33`).

## Rating

**6 / 10** — Present and reasonably consistent at the persistence layer, with tests for history mechanisms, but fragile across boundaries: source references do not reach the model via retrieval tools, no trust dimension exists anywhere, transformation identity (which chunker/summarizer produced an artifact) is logged but not stored on artifacts, and serialization regenerates identity.

Rationale against rubric:

- Not 7-8 because: the model itself frequently cannot see *where* retrieved context came from (tool result shapes omit source fields that exist in storage); there is no trust/authority field anywhere; chunker-choice during ingestion fallback is only visible in ephemeral telemetry events (`letta/services/file_processor/embedder/openai_embedder.py:216-224`), not on the passage.
- Better than 4-6 midpoint because: freshness is genuinely operational (relative-time rendering, temporal filters, `last_accessed_at` maintenance), compaction provenance is embedded into live context in machine-readable form, and BlockHistory/git-memory provide real, tested operational safeguards.

## Evidence Collected

Every entry cites a file path with line numbers, relative to `sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source annotations (passages) | `PassageBase` declares `source_id` (marked deprecated in favor of folder linkage), `file_id`, `file_name`, `metadata`, `tags`, `archive_id` | `letta/schemas/passage.py:21-32` |
| Source annotations (ORM) | `SourcePassage` persists file provenance via `FileMixin` + `SourceMixin` FKs and a `file_name` column ("The name of the file that this passage was derived from"); `ArchivalPassage` scopes by `archive_id` | `letta/orm/passage.py:48-53,76-84`; mixins at `letta/orm/mixins.py:43-56` |
| Message lineage | `Message.step_id`, `run_id`, `otid`, `group_id`, `sender_id`, `conversation_id`, `batch_item_id`, `is_err` tie each message to its producing step/run/thread | `letta/schemas/message.py:292-307` |
| Block template lineage | `template_name`, `is_template`, `template_id`, `base_template_id`, `deployment_id`, `entity_id`, `preserve_on_migration` | `letta/schemas/block.py:24-30` |
| Actor attribution on blocks | `created_by_id` / `last_updated_by_id` recorded on `Block` | `letta/schemas/block.py:73-74` |
| Freshness (DB-wide) | ORM base defines `created_at` and `updated_at` with `server_default=func.now()` plus a `set_updated_at()` helper; default ordering is `created_at` | `letta/orm/base.py:16-28`; `letta/orm/sqlalchemy_base.py:124` |
| Freshness (passages/messages) | `Passage.created_at` defaults to UTC (`get_utc_time`); `Message.created_at` is mandatory and normalized to UTC by validator | `letta/schemas/passage.py:47`; `letta/schemas/message.py:310,331-334`; helper at `letta/helpers/datetime_helpers.py:59-62` |
| Freshness (files) | `FileMetadata.created_at`, `updated_at`, `last_accessed_at`; association manager refreshes `last_accessed_at = now()` on open/close/search, including a bulk no-load update path | `letta/schemas/file.py:61-62,80,126`; `letta/services/files_agents_manager.py:91,106,140-166,439-457` |
| Freshness surfaced to model | `conversation_search` formats each hit as `{timestamp, time_ago, role, relevance}` with ISO timestamp converted to the agent timezone and human deltas ("3h ago") | `letta/services/tool_executor/core_tool_executor.py:176-232` |
| Temporal filtering | Both search tools accept `start_datetime`/`end_datetime` (ISO 8601) filtering on creation time; documented in tool docstring ("Only return memories created after this time") | `letta/services/tool_executor/core_tool_executor.py:81-90,278-288`; `letta/functions/function_sets/base.py:200-201` |
| Relevance metadata | Search hits can carry `rrf_score`, `vector_rank`, `fts_rank`, `search_mode` from hybrid retrieval | `letta/services/agent_manager.py:2671-2686`; `letta/services/tool_executor/core_tool_executor.py:231-249` |
| Trust level fields | Searched `trust_level`, `authority`, `credib*` across `letta/**/*.py`: no matches (only incidental word matches). No trust/authority annotation exists on any context schema | Search performed over `letta/schemas/*.py`, `letta/orm/*.py`, and package-wide grep |
| Closest trust proxy | `Block.read_only` flag prevents agent self-editing of a block; enforced in `core_memory_append` before mutation | `letta/schemas/block.py:36`; `letta/services/tool_executor/core_tool_executor.py:320-321` |
| Transformation: compaction stats | Compaction builds `compaction_stats = {trigger, context_tokens_before/after, context_window, messages_count_before/after}` | `letta/services/summarizer/compact.py:415-430` |
| Transformation: in-context marker | Summary payload packaged as JSON `{type: "system_alert", message, time, compaction_stats}` stating "N messages ... hidden from view due to memory constraints" plus the summary text; sliding-window vs "all" modes produce different wording | `letta/system.py:207-236` |
| Transformation: summary role | Summaries persisted as first-class `Message(role="summary", ...)` carrying their own `agent_id/run_id/step_id` (`CompactResult.summary_message`) | `letta/services/summarizer/compact.py:150,171,434-443,468` |
| Transformation: reasoning markers | `RedactedReasoningContent` (provider-redacted thinking, keeps opaque `data`), `OmittedReasoningContent` (known-present but unreturned reasoning, optional `signature`), `SummarizedReasoningContent` (keeps provider item `id`, summary parts, `encrypted_content`) explicitly mark transformed content in-context | `letta/schemas/letta_message_content.py:276-301`; populated at `letta/interfaces/openai_streaming_interface.py:1083-1088` |
| Transformation: block checkpoints | `checkpoint_block_async` snapshots label/value/limit/metadata into `BlockHistory` with `sequence_number`, `actor_type` (`LETTA_AGENT` vs `LETTA_USER`), `actor_id`; undo/redo moves pointer through linear history and truncates redo chains | `letta/orm/block_history.py:12-46`; `letta/services/block_manager.py:841-911,916-942` |
| Transformation: git-backed memory | `GitEnabledBlockManager` writes to git (GCS) first as source of truth, PostgreSQL as cache, "full version history" per agent repo; blocks serialize to Markdown with YAML frontmatter (description/read_only/metadata preserved; `limit` intentionally dropped) | `letta/services/block_manager_git.py:1-50`; `letta/services/memory_repo/block_markdown.py:1-55`; git ops incl. `get_history` at `letta/services/memory_repo/git_operations.py:351,540` |
| Chunking fallback not persisted | When the file-specific chunker fails, ingestion retries with a default `SentenceSplitter`; the choice is recorded only in log/telemetry events, and created `Passage(...)` objects carry no chunk-index or chunker field | `letta/services/file_processor/file_processor.py:98-141`; `letta/services/file_processor/embedder/openai_embedder.py:204-211` |
| Provenance survives serialization? (export/import) | Agent-file export **deletes** `id`, `_created_by_id`, `_last_updated_by_id` and regenerates them on import via `pre_load` — value/content survive, identity provenance does not | `letta/serialize_schemas/marshmallow_block.py:17-33` |
| Provenance survives serialization? (REST API) | `/v1/passages/search` returns full `Passage` objects (including `source_id`, `file_id`, `created_at`, `metadata`) wrapped in `PassageSearchResult` | `letta/server/rest_api/routers/v1/passages.py:59-77` |
| Provenance stripped for the model | `archival_memory_search` results returned to the agent are `{id, timestamp, content, tags, [relevance]}` — no `file_id`, `source_id`, or `folder_id` even though the passage row has them | `letta/services/agent_manager.py:2651` |
| Provenance stripped for providers | `to_openai_dict` emits only `{role, content[, tool_calls, name, tool_call_id]}`; message ids/timestamps/step ids never reach the LLM request; `summary` role downgraded to plain `user` | `letta/schemas/message.py:1339-1420,1396-1402` (same pattern for Anthropic/Google at `1839+`, `1926-1932`, `2205-2211`) |
| In-context directory metadata | Memory compilation renders `<directory name=...>` with description/instructions and per-file `<metadata>` (`read_only`, `chars_current`, `chars_limit`) — structural provenance but no timestamps | `letta/schemas/memory.py:588-633` |
| Git-mode projection paths | Git-enabled memory renders `<projection>$MEMORY_DIR/system/persona.md</projection>` so the model knows each memory section's file path in its versioned repo | `letta/schemas/memory.py:76,205-227` |
| Tests for history mechanism | `test_checkpoint_creates_history`, `test_multiple_checkpoints`, `test_checkpoint_with_agent_id` verify `BlockHistory` rows, sequence ordering, and actor attribution | `tests/managers/test_block_manager.py:774,812,851` |

## Answers to Dimension Questions

**1. Does each context item know where it came from?**
Yes at rest; partially at the boundary. Passages persist `source_id`/`file_id`/`file_name`/`archive_id` (`letta/schemas/passage.py:21-32`, `letta/orm/passage.py:48-53`), messages persist step/run/sender/group/conversation lineage (`letta/schemas/message.py:292-310`), and blocks persist template lineage and creator IDs (`letta/schemas/block.py:24-30,73-74`). However, when context is handed to the *model*, the origin is mostly erased: archival search hits drop file/source references (`letta/services/agent_manager.py:2651`), and provider conversion strips all identifiers (`letta/schemas/message.py:1396-1402`). Only structural labels (block labels, directory names, `<projection>` paths in git mode, `letta/schemas/memory.py:205-227,588-633`) survive into the prompt.

**2. Is freshness tracked?**
Yes, thoroughly. UTC `created_at` on passages and messages (`letta/schemas/passage.py:47`; `letta/schemas/message.py:310,331-334`), DB-side `created_at`/`updated_at` on all rows (`letta/orm/base.py:16-17`), file-level `updated_at`/`last_accessed_at` maintained on access (`letta/services/files_agents_manager.py:140-166,439-457`). Crucially, freshness is surfaced to the model: search results include ISO timestamps, timezone-localized formatting, relative `time_ago` strings, and support date-range filters (`letta/services/tool_executor/core_tool_executor.py:176-232,81-90`). One nuance: passages have no `updated_at` in their Pydantic schema — content edits would not be distinguishable from creation time without dropping to ORM fields.

**3. Is trust level indicated?**
No evidence found. A package-wide search for `trust_level`, `authority`, and `credib*` across `letta/**` returned no relevant matches. The nearest proxies are coarse-grained: organization-scoped FKs everywhere (`letta/orm/mixins.py:19-56`), `Block.read_only` preventing agent self-mutation (`letta/schemas/block.py:36`, enforced at `letta/services/tool_executor/core_tool_executor.py:320-321`), and actor typing in history entries. There is no per-item notion of "this came from the user vs a tool vs an unverified document."

**4. Are transformations traceable?**
Partially, with a clear split. Compaction is well traced: a machine-readable `system_alert` payload with `compaction_stats` (counts/tokens before-after, trigger, mode) is placed directly in live context and persisted under a dedicated `summary` role (`letta/system.py:207-236`; `letta/services/summarizer/compact.py:415-443`). Reasoning-content redaction/omission/summarization is type-marked in-context with provider signatures (`letta/schemas/letta_message_content.py:276-301`). Core-memory edits are checkpointed with actor attribution and linear sequence numbers, tested in `tests/managers/test_block_manager.py:774-851`, with an opt-in git backend providing full commit history (`letta/services/block_manager_git.py:1-40`). Gaps: file→passage chunking does not record chunk index, page origin, or which chunker ran on the artifact itself (`letta/services/file_processor/embedder/openai_embedder.py:204-211`); summarization does not record *which* messages were folded into the summary (only aggregate counts).

## Architectural Decisions

- **Two-tier provenance: rich at rest, minimal in-flight.** Letta stores comprehensive lineage in PostgreSQL/ORM entities but treats the LLM request as a lossy projection built solely from role+content (`letta/schemas/message.py:1339-1348`). This maximizes provider compatibility and token efficiency at the cost of model-visible provenance; anything the model must see is explicitly re-injected as text (e.g., compaction alerts, timestamps inside tool-result JSON).
- **Tool results as the provenance channel.** Rather than annotating raw messages, Letta gives retrieval tools structured return dicts (`timestamp`, `time_ago`, `relevance`, `tags`) that the model reads (`letta/services/tool_executor/core_tool_executor.py:225-249`). Provenance to the model is a tool-design concern, not a transport concern.
- **Explicit checkpointing instead of implicit auditing.** History rows are created by deliberate `checkpoint_*` calls with actor typing (`LETTA_AGENT` vs `LETTA_USER`, `letta/orm/block_history.py:36-37`), keeping the hot path cheap while providing forensically useful snapshots; the git backend (`git-memory-enabled` tag, `letta/services/block_manager_git.py:26-40`) is an opt-in escalation for full durability.
- **Deprecation-with-compat for provenance keys.** `source_id` is retained but marked deprecated in favor of folder-based linkage (`letta/schemas/passage.py:24-26`), showing a migration strategy where old provenance fields remain populated for backward compatibility.
- **Transformation markers as content types.** Instead of side-table logs, provider-native transformation states (redacted/omitted/summarized reasoning) are modeled as discriminated union members of message content (`letta/schemas/letta_message_content.py:276-301`), so the fact-of-transformation travels with the data through serialization.

## Notable Patterns

- **Mixin-based foreign-key provenance**: `OrganizationMixin`, `FileMixin`, `SourceMixin`, `ArchiveMixin` attach scope/origin columns uniformly across tables (`letta/orm/mixins.py:19-56`; applied at `letta/orm/passage.py:48`).
- **Timezone-aware presentation layer**: repeated pattern of converting UTC stamps to the agent's `ZoneInfo` timezone with graceful fallback (`letta/services/agent_manager.py:2646-2668`; `letta/services/tool_executor/core_tool_executor.py:186-218`) — freshness is localized per-agent, not globally formatted.
- **Anti-recursion hygiene in search**: `conversation_search` drops tool-role messages and assistant messages containing the search call itself, preventing provenance chains that reference themselves from compounding (`letta/services/tool_executor/core_tool_executor.py:148-158`).
- **Frontmatter as durable metadata carrier**: git-backed blocks round-trip `description`/`read_only`/`metadata` through YAML frontmatter, deliberately excluding deprecated fields like `limit` (`letta/services/memory_repo/block_markdown.py:27-55`).
- **Structured system alerts**: transient-but-critical notices (context truncation) are injected as typed JSON payloads (`type: "system_alert"`) rather than prose-only, enabling downstream parsing (`letta/system.py:227-235`).

## Tradeoffs

- **Model-blind provenance vs token cost**: including `file_id`/`source_id` in every archival hit would let the model reason about origin and cite better, but inflates every tool result; Letta chose compactness (`letta/services/agent_manager.py:2651`).
- **Checkpoint-on-demand vs always-audit**: `BlockHistory` requires explicit checkpoint calls; un-checkpointed edits leave no trail unless git memory is enabled — cheaper writes, sparser history (`letta/services/block_manager.py:841-911`).
- **Export portability vs continuity**: regenerating IDs on import makes agent files safely importable into any org/database but breaks cross-references between exported agents and shared blocks (`letta/serialize_schemas/marshmallow_block.py:17-33`).
- **Aggregate compaction stats vs item-level traceability**: counting evicted messages is cheap and sufficient for the model to know "something was lost," but it cannot recover or attribute specific lost content (`letta/system.py:207-224`).

## Failure Modes / Edge Cases

- **Silent origin loss in archival recall**: a passage derived from a sensitive file is indistinguishable, from the model's viewpoint, from one the agent wrote itself — the DB row says otherwise (`letta/schemas/passage.py:29-30` vs `letta/services/agent_manager.py:2651`).
- **Chunker-fallback divergence is invisible downstream**: if the file-specific chunker fails mid-ingestion, some passages of a file may come from different splitters; nothing on the passage records this, complicating debugging of odd retrieval quality (`letta/services/file_processor/file_processor.py:98-141`).
- **Naive datetime normalization depends on validator path**: `Message.created_at` gets UTC-enforced in `to_json`/validators (`letta/schemas/message.py:331-334`), while `PassageCreate.created_at` accepts arbitrary client-supplied datetimes (`letta/schemas/passage.py:86`) — backdated provenance is possible via the create API.
- **Redo-chain truncation destroys future history**: after an undo, a new checkpoint permanently deletes later checkpoints to keep history linear (`letta/services/block_manager.py:872-886`) — intentional, but a destructive operation on transformation history.
- **Summary-role downgrade**: in provider payloads, `role="summary"` collapses to `user` (`letta/schemas/message.py:1396-1402`); the provider sees a user uttering what looks like a system notice, relying entirely on the embedded text for correct interpretation.
- **`metadata_` pass-through without schema**: passage metadata is a free-form JSON dict propagated verbatim (`letta/services/passage_manager.py:173,253`) — flexible but unvalidated, so provenance conventions in metadata cannot be relied upon.

## Future Considerations

- Add `source_ref` passthrough to `archival_memory_search` results (the data already exists on the row; one line in the result-dict builder at `letta/services/agent_manager.py:2651`) so the model can attribute retrieved knowledge.
- Introduce a lightweight trust/origin enum (e.g., `user_authored | tool_return | file_derived | agent_written`) on passages and blocks, enforced where tool returns are ingested.
- Persist chunker identity and page/chunk indices on `SourcePassage` (columns exist-ready alongside `file_name`, `letta/orm/passage.py:53`) to make ingestion transformations reproducible.
- Record per-message eviction lists (or message-ID ranges) in `compaction_stats` to upgrade summaries from count-traceable to item-traceable (`letta/system.py:207-236`).
- Preserve an ID mapping (old→new) on agent-file import instead of discarding identities, restoring cross-block references after migration (`letta/serialize_schemas/marshmallow_block.py:26-33`).

## Questions / Gaps

- **Trust modeling**: no evidence of any trust/authority scoring anywhere in the package; whether Letta Cloud layers this externally cannot be verified from this source alone (search boundary: `sources/letta/letta/**`).
- **Sleeptime/self-summarizer provenance**: `self_summarizer.py` and `summarizer_all.py` were located (`letta/services/summarizer/`) but not exhaustively traced; the shared packaging function suggests identical behavior, unconfirmed.
- **Turbopuffer-backed passage storage**: vector-backend paths (`letta/helpers/tpuf_client.py` referenced at `letta/schemas/passage.py:65-67`) were not audited; unclear whether external vector stores retain the same provenance columns.
- Whether any client SDK surfaces `folder_id`-based lineage to end users consistently was out of scope given the deprecation overlap of `source_id` (`letta/schemas/passage.py:24-26`).

---

Generated by `11.04-context-provenance-and-integrity` against `letta`.
