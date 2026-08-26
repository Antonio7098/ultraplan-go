# Source Analysis: letta

## Dimension 05.03: Long-Term User, Project, and Domain Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python 3.12 (`pyproject.toml:1-4`, v0.16.8); FastAPI server, SQLAlchemy ORM on PostgreSQL (+pgvector) or SQLite, optional Turbopuffer/Pinecone vector stores, git-over-object-storage memory repos |
| Analyzed | 2026-08-26 |

## Summary

Letta (the MemGPT successor) is an agent platform whose central abstraction is persistent, tiered memory. Four long-term stores were identified:

1. **Core memory** — always-in-context `Block` rows (persona/human/custom labels) persisted in SQL and shared between agents via pivot tables (`letta/orm/block.py:20-58`, `letta/schemas/block.py:13-44`).
2. **Archival memory** — unbounded semantic store of `Passage` rows attached to shareable `Archive` containers, with embeddings in pgvector or dual-written to Turbopuffer (`letta/orm/passage.py:21-48`, `letta/services/passage_manager.py:543-637`).
3. **Recall memory** — persisted message history with hybrid FTS/vector search (`letta/services/message_manager.py:1142-1172`).
4. **Memory filesystem** — an opt-in per-agent git repository (object storage source of truth, PostgreSQL cache) giving blocks full commit history (`letta/services/block_manager_git.py:1-50`, `letta/services/memory_repo/git_operations.py:49-95`).

Memory scope is layered: organization-scoped by default at the ORM layer (`letta/orm/mixins.py:19-24`, `letta/orm/sqlalchemy_base.py:116-118`), with project-scoped blocks (`letta/schemas/block.py:22`), identity-linked blocks for end-user association (`letta/schemas/identity.py:44-53`), group-shared blocks for multi-agent (sleeptime) setups (`letta/services/group_manager.py:359-417`), and agent-private defaults.

Write policies are explicit and enforced in code: agents write through dedicated memory tools guarded by read-only flags, uniqueness checks, line-number sanitization, and char limits (`letta/services/tool_executor/core_tool_executor.py:41-56, 319-401`); every block mutation can be checkpointed into a linear undo/redo history with actor attribution (`letta/services/block_manager.py:845-909`, `letta/orm/block_history.py:12-48`). Retrieval is either always-on (core memory compiled into the system prompt, `letta/schemas/memory.py:688-732`) or query-driven hybrid search over passages/messages with tag and time filters (`letta/services/agent_manager.py:2534-2663`). Deletion is hard-delete with cross-store fan-out (SQL + Turbopuffer + git prefix), though the dual-write path degrades to log-and-continue on vector-store failure. Freshness is handled via timestamps, relative-time rendering in search results, summarizer warnings before context eviction, and prompt-level staleness guidance — but there is no systematic TTL/expiry mechanism.

## Rating

**Score: 8 / 10** — "Clear model with tests, explicit interfaces, and operational safeguards."

Rationale:

- **Clear model**: four well-separated tiers with first-class schemas (`letta/schemas/memory.py:68-80`, `letta/schemas/passage.py:35-47`, `letta/schemas/archive.py:24-28`) and REST surfaces for each (`letta/server/rest_api/routers/v1/blocks.py:146-176`, `archives.py:58-260`, `agents.py:1206-1524`).
- **Tests**: unit tests cover memory compilation, git-memory rendering, and no-op update suppression (`tests/test_memory.py:20-464`, `tests/test_block_manager_noop_update.py`), plus integration tests for sleeptime and turbopuffer flows (`tests/integration_test_sleeptime_agent.py`, `tests/integration_test_turbopuffer.py`).
- **Operational safeguards**: read-only blocks (`letta/schemas/block.py:36`, enforcement at `core_tool_executor.py:320-321`), optimistic-locking version column on blocks (`letta/orm/block.py:56-58`), org-scoping security warnings when reads lack an actor (`letta/orm/sqlalchemy_base.py:256-258, 496-504`), and actor-attributed history snapshots (`block_manager.py:898-899`).
- **Why not higher**: vector-search across multiple archives raises rather than resolving (`agent_manager.py:2444-2446`); Turbopuffer write failures are swallowed with only a log unless strict mode is set (`passage_manager.py:629-632`), creating silently incomplete recall; staleness control relies on prompts rather than mechanisms; several scoping fields are mid-deprecation (`identity.block_ids` deprecated in `letta/schemas/identity.py:51`, `GroupCreate.shared_block_ids` deprecated in `letta/schemas/group.py:38`); no TTL/retention policy exists for stale memories.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core memory schema | `Memory` model holding `blocks` + `file_blocks`, `git_enabled` flag; rendered into system prompt | letta/schemas/memory.py:68-80 |
| Core memory edit primitives | `core_memory_append` / `core_memory_replace` on `BasicBlockMemory` | letta/schemas/memory.py:804-837 |
| Default chat blocks | `ChatMemory` seeds `persona` and `human` blocks, limit from `CORE_MEMORY_BLOCK_CHAR_LIMIT` (100000) | letta/schemas/memory.py:840-854; letta/constants.py:435 |
| Block persistence | Block ORM row: value, label, limit, read_only, hidden, org/project/template mixins | letta/orm/block.py:20-50 |
| Block sharing | `BlocksAgents`, `GroupsBlocks`, `BlocksTags`, `IdentitiesBlocks` pivot tables | letta/orm/blocks_agents.py; letta/orm/groups_blocks.py; letta/orm/blocks_tags.py; letta/orm/identities_blocks.py |
| Optimistic locking | `version` column documented as concurrency counter | letta/orm/block.py:56-60 |
| Undo/redo history | `BlockHistory` snapshot table w/ sequence numbers + actor type/id; checkpoint creation truncates redo chain | letta/orm/block_history.py:12-48; letta/services/block_manager.py:859-904 |
| Write policy (agent tools) | Function map of 14 memory tools incl. append/replace/rethink/apply_patch/delete/rename/create | letta/services/tool_executor/core_tool_executor.py:41-56 |
| Read-only enforcement | Every mutating tool raises if `block.read_only` | letta/services/tool_executor/core_tool_executor.py:319-321, 336-337, 354-355, 691-692, 744-745 |
| Edit precision guards | Line-number prefix regex rejection + unique-match requirement in replace/patch tools | letta/services/tool_executor/core_tool_executor.py:357-394, 381-391 |
| Multi-block patch ops | `memory_apply_patch` codex-style Add/Delete/Update/Move-to block operations | letta/services/tool_executor/core_tool_executor.py:403-681 |
| Full-block rewrite | `memory_rethink` creates-or-replaces a whole block | letta/services/tool_executor/core_tool_executor.py:743-773 |
| Block deletion = detach | `memory_delete` detaches from agent; underlying block survives as shared object | letta/services/tool_executor/core_tool_executor.py:778-806 |
| Prompt recompile fan-out | Prompt-affecting block fields trigger rebuild for *all* connected agents | letta/services/block_manager.py:32, 61-68, 266-268 |
| No-op write suppression | Scalar/tag change detection skips empty updates | letta/services/block_manager.py:225-240 |
| Archival memory schema | Passage: text, embedding, embedding_config, tags, archive_id, is_deleted flag | letta/schemas/passage.py:14-47 |
| Vector column | pgvector `Vector(MAX_EMBEDDING_DIM=4096)` with padding validator for uniform size | letta/orm/passage.py:34-40; letta/constants.py:93; letta/schemas/passage.py:49-77 |
| Archive container | Shareable collection of passages w/ per-archive `vector_db_provider` (native/tpuf/pinecone) | letta/schemas/archive.py:11-28; letta/schemas/enums.py:277-282 |
| Dual-write archival insert | `insert_passage`: embed → write SQL → mirror to Turbopuffer (log-only on failure unless strict) | letta/services/passage_manager.py:543-637 |
| Archival retrieval | Hybrid vector+FTS with RRF scores, tag ANY/ALL filter, datetime range filter, top_k default 5 | letta/services/agent_manager.py:2534-2663; letta/constants.py:458 |
| Recall search modes | Message search: "vector", "fts", "hybrid", "timestamp" modes | letta/services/message_manager.py:1142-1172 |
| Agent memory tool surface | `conversation_search`, `archival_memory_search/insert` exposed to LLM; results carry time-ago strings | letta/services/tool_executor/core_tool_executor.py:81-273, 278-305 |
| Git-backed memory | Tag-gated `GitEnabledBlockManager`: git (GCS/S3/local) is source of truth, PG is cache | letta/services/block_manager_git.py:1-50, 52-69; letta/services/memory_repo/storage/base.py:7-27 |
| Git commit metadata | Commits record author_type (agent/user/system), author_id, files changed, additions/deletions | letta/schemas/memory_repo.py:19-44 |
| Git deletion | `delete_repo` removes the whole org/agent storage prefix | letta/services/memory_repo/git_operations.py:629-638 |
| Org multi-tenancy | `OrganizationMixin.organization_id` FK on all memory models; access predicate applied on read/list | letta/orm/mixins.py:19-24; letta/orm/sqlalchemy_base.py:265-267 |
| Missing-actor guard | Security warning logged when org-scoped reads bypass filtering | letta/orm/sqlalchemy_base.py:256-258, 499-504 |
| Project scope | `project_id` field on blocks and identities | letta/schemas/block.py:22; letta/schemas/identity.py:49 |
| Identity ↔ block linking | Attach/detach block to identity (external end-user key) via manager + REST | letta/services/identity_manager.py:419-452; letta/server/rest_api/routers/v1/blocks.py:244-264 |
| Group shared memory | Sleeptime groups attach shared blocks to all member agents | letta/services/group_manager.py:359-417 |
| Sleeptime memory agent | Dedicated memory-manager persona instructs absolute dates ("do not write 'today'"), selective edits, finish tool | letta/prompts/system_prompts/sleeptime_v2.py:16-28 |
| Context-eviction safeguard | Warning tells agent to persist facts via core/archival tools before trim | letta/constants.py:414-421 |
| Summarizer retention | Evicted messages summarized into notes about the human before removal from context | letta/services/summarizer/summarizer.py:257-333, 436-452 |
| Deletion (passages) | Hard delete from SQL plus mirrored delete from Turbopuffer namespace | letta/services/passage_manager.py:767-800 |
| Deletion (archives) | `delete_archive_async` hard-deletes archive row | letta/services/archive_manager.py:266-279 |
| Deletion (users) | Actor hard-deleted along with associated records | letta/services/user_manager.py:79-85 |
| Vector namespaces | Per-archive namespace for archival; org-scoped namespaces for messages/tools | letta/helpers/tpuf_client.py:312-346 |
| REST memory APIs | Blocks CRUD + count + agents-for-block; archives CRUD + passages; agent core-memory/archival endpoints; global passage search | letta/server/rest_api/routers/v1/blocks.py:27-264; archives.py:58-260; agents.py:1206-1524; passages.py:82-83 |
| File-block freshness | `FileBlock.last_accessed_at` updated by open/close/search tools | letta/schemas/block.py:107-114 |
| Tests | Memory compile/rendering suite; git-memory rendering cases; noop-update test | tests/test_memory.py:20-464; tests/test_block_manager_noop_update.py |

## Answers to Dimension Questions

### 1. What persists across sessions?

Everything relevant persists server-side. Agents themselves are durable DB entities; core memory blocks live in the `block` table (`letta/orm/block.py:20-23`), archival memories in archival passage tables keyed by archive (`letta/orm/passage.py:76-102`), full message history in the message table searchable via `search_messages_async` (`letta/services/message_manager.py:1142-1154`), and optionally the whole memory tree as a git repo in object storage (`letta/services/block_manager_git.py:1-8`). The in-context window itself is rebuilt from these stores on session start via `rebuild_system_prompt_async` (`letta/services/agent_manager.py:1523-1548`). Only ephemeral agents opt out (a dedicated `ephemeral_agent.py` exists in `letta/agents/`).

### 2. Who can write memory?

Three writer classes with distinct channels:

- **Agents (LLM)**: via the tool map — `core_memory_append/replace`, `memory_replace/insert/rethink/apply_patch`, `archival_memory_insert`, `memory` create/str_replace/insert/delete/rename (`letta/services/tool_executor/core_tool_executor.py:41-56`). Writes are gated by `read_only` block flags (`core_tool_executor.py:320-321`), and the sleeptime variant writes shared blocks on behalf of a group.
- **Users/API actors**: REST CRUD on blocks, archives, passages, core-memory-by-label (`letta/server/rest_api/routers/v1/blocks.py:146-176`, `agents.py:1268-1277`, `archives.py:215-260`). Every create/update records `created_by_id` / `last_updated_by_id` (`letta/schemas/block.py:73-74`; set in `letta/orm/sqlalchemy_base.py:576-577`).
- **Attribution**: checkpoints stamp `actor_type` (LETTA_AGENT vs LETTA_USER) and `actor_id` (`letta/services/block_manager.py:898-899`); git commits record author_type agent/user/system (`letta/schemas/memory_repo.py:28-30`).

### 3. Who can read memory?

Reads default to organization scope: `AccessType.ORGANIZATION` is the default access type and `apply_access_predicate` filters queries by the actor's org (`letta/orm/sqlalchemy_base.py:116-118, 265-267`). Within an org, memory can be deliberately widened: a single block attached to many agents (`blocks_agents` pivot, queried at `block_manager_git.py:99-108`), group-shared blocks for sleeptime pairs (`group_manager.py:372-384`), identity-attached blocks for end-user association (`identity_manager.py:419-437`), and archives attachable to multiple agents (`archive_manager.py:155-198`, "shared between agents" at `letta/schemas/archive.py:25`). Cross-org isolation is reinforced by per-org/per-archive vector namespaces (`tpuf_client.py:312-332`) and org-prefixed git repo paths (`git_operations.py:91-95`). Caveat: passing `actor=None` bypasses the filter and only emits a warning log (`sqlalchemy_base.py:256-258`).

### 4. Can memory be corrected?

Yes, through multiple correction paths:

- **Surgical string replacement** with verbatim-uniqueness enforcement — ambiguous matches abort the edit with line hints (`core_tool_executor.py:380-391`).
- **Unified-diff patching** against one or many blocks, including add/delete/rename operations (`core_tool_executor.py:403-681`).
- **Full rewrite** via `memory_rethink` (`core_tool_executor.py:743-773`).
- **Undo/redo** over a strictly linear checkpoint stack with future-checkpoint truncation (`block_manager.py:874-904`, `956-1040`); git-enabled agents additionally get commit history and revert semantics (`git_operations.py:540-602`).
- **API-level PATCH** on blocks and passage updates (`routers/v1/blocks.py:157-166`; `PassageUpdate` at `letta/schemas/passage.py:89-95`).
- **Deletion-as-correction**: archival passages support hard delete with vector-store mirroring (`passage_manager.py:767-800`).

### 5. Can memory become stale?

Yes — nothing expires automatically. Mitigations are partial and prompt-centric:

- Search results decorate entries with `time_ago` ("3d ago") so the LLM sees age (`core_tool_executor.py:186-227`); archival search accepts start/end datetime filters (`agent_manager.py:2557-2558`).
- The sleeptime memory prompt explicitly forbids relative dates because "the memory is persisted indefinitely" and directs cleanup of "redundant and outdate information" (`sleeptime_v2.py:17-20`).
- Before context eviction, the agent is warned to save important facts into core/archival memory (`constants.py:414-418`), and the summarizer distills evicted messages into notes about the human (`summarizer.py:436-452`).
- No evidence found of any TTL, decay function, scheduled re-validation job, or contradiction-detection pass over stored memories (searched for expiry/TTL/cron patterns under `letta/services/` and `letta/constants.py`; only sleeptime frequency scheduling exists at `group_manager.py:97-98, 264`). Staleness therefore depends entirely on the LLM following prompt guidance.

## Architectural Decisions

1. **Tiered memory as product architecture.** Core (in-context, limited), archival (semantic, unbounded), recall (message log), and memory-filesystem tiers mirror the original MemGPT design; each tier has its own schema, manager, tools, and REST routes (`letta/schemas/memory.py:68-80`, `letta/schemas/passage.py:35-47`, `letta/services/message_manager.py:1142-1172`, `block_manager_git.py:1-8`).
2. **Blocks as first-class shared objects, not per-agent blobs.** A block lives once and is linked to agents/groups/identities via pivot tables (`letta/orm/blocks_agents.py`, `groups_blocks.py`, `identities_blocks.py`); this is what makes user-level and project-level memory possible without duplication (`group_manager.py:372-384`, `identity_manager.py:419-437`).
3. **Org-first multi-tenancy baked into the ORM base class**, not per-endpoint checks — every list/read applies the access predicate by default (`sqlalchemy_base.py:116-118, 265-267`).
4. **Git as optional source of truth for memory**, with PostgreSQL demoted to a read cache — chosen per agent via the `git-memory-enabled` tag (`block_manager_git.py:26-40`), backed by pluggable GCS/S3/local backends (`storage/base.py:7-27`).
5. **Dual-write relational + vector store.** SQL remains canonical; Turbopuffer mirrors embeddings for scale, namespaced per archive/org (`passage_manager.py:586-632`, `tpuf_client.py:312-346`).
6. **Prompt-rebuild propagation.** Because core memory is compiled into the system prompt, edits trigger deterministic rebuilds both directly (`update_memory_if_changed_async` at `agent_manager.py:1747-1772`) and fan-out to every agent sharing the block (`block_manager.py:61-68`), keeping multi-agent views consistent.

## Notable Patterns

- **Manager-per-aggregate service layer**: `BlockManager`, `PassageManager`, `ArchiveManager`, `IdentityManager` etc. under `letta/services/`, all funneling through ORM base CRUD.
- **Rendering strategies selected per agent type/provider**: standard XML blocks, line-numbered blocks (Anthropic + specific agent types), and git-structured `<self>/<memory>` projections (`schemas/memory.py:143-203, 205-349`, selection logic at `688-712`).
- **Edit-safety ergonomics**: displayed line numbers are validated away on input via `MEMORY_TOOLS_LINE_NUMBER_PREFIX_REGEX` so display artifacts never corrupt stored values (`core_tool_executor.py:357-374`).
- **Checkpoint/undo stack with linear-history invariant** — redo branches truncated on new writes (`block_manager.py:874-883`).
- **No-op detection** before committing block updates to avoid spurious prompt rebuilds (`block_manager.py:225-240`; tested in `tests/test_block_manager_noop_update.py`).
- **Anti-recursion retrieval filtering**: `conversation_search` excludes tool messages and prior search calls to prevent exponential nesting of results (`core_tool_executor.py:151-167`).

## Tradeoffs

- **Consistency vs availability in dual-writes**: a Turbopuffer failure leaves a SQL-only passage that vector search cannot see; the code logs and continues unless `strict_mode=True` (`passage_manager.py:606-632`) — favoring write availability over retrieval completeness, with no visible reconciliation job to repair drift.
- **Sharing vs lifecycle coupling**: detached blocks survive as org-visible objects (`core_tool_executor.py:778-789`), which enables reuse but means "delete my memory" from one agent does not remove data others may reference.
- **Expressiveness vs safety in editing**: rich patch/rethink tooling gives precise control but required building multiple guard layers (uniqueness checks, line-number stripping, read-only flags), each of which is more surface area for LLM misuse (`core_tool_executor.py:346-401, 403-430`).
- **Generous limits**: 100k-char default block limit (`constants.py:435`) keeps memory loss rare but allows a single block to consume large context shares; embedding padding to dim 4096 (`passage.py:49-77`) simplifies schemas at the cost of storage bloat.
- **Prompt-based freshness policy**: cheap and flexible, but correctness now depends on model compliance rather than enforced invariants.

## Failure Modes / Edge Cases

- **Silent partial indexing**: non-strict tpuf insert failures produce passages invisible to semantic search until some later rewrite (`passage_manager.py:629-632`); the same best-effort pattern exists in `archive_manager.create_passage_in_archive_async` (`archive_manager.py:366-369`).
- **Multi-archive vector search unsupported**: an agent attached to >1 archive raises `ValueError` instead of merging results (`agent_manager.py:2444-2446`).
- **Actor-less reads degrade to warning-only**: `read_async(..., actor=None)` logs "SECURITY ... bypasses organization filtering" but still executes (`sqlalchemy_base.py:499-504`).
- **Label collision handling**: memory-filesystem rendering tolerates label paths that collide as both leaf and directory, last-writer-wins (`schemas/memory.py:360-399`) — a documented but lossy edge case.
- **Timezone-dependent freshness display**: `time_ago` computations fall back to UTC/ISO silently if the agent timezone is invalid (`core_tool_executor.py:177-184, 215-217`).
- **History is per-block, not transactional across blocks**: a multi-block `memory_apply_patch` applies actions sequentially; a failure midway leaves earlier block edits applied without aggregate rollback (`core_tool_executor.py:623-675`).
- **User hard-delete cascades unevenly**: `delete_actor_by_id_async` hard-deletes the user row (`user_manager.py:79-85`), while block history explicitly tolerates dangling actor references ("may not always exist... e.g. a User deleted", `orm/block_history.py:34-37`).

## Future Considerations

- **Add retention/TTL policies** for archival passages and block revisions (e.g., max history depth, archive-level expiry) to make freshness mechanical rather than prompt-driven.
- **Reconciliation worker** for SQL↔Turbopuffer drift, or make strict-mode the default for interactive writes.
- **Multi-archive vector search** to lift the single-archive restriction (`agent_manager.py:2444-2446`).
- **Finish the scope migration**: remove deprecated `identity.agent_ids/block_ids` (`identity.py:50-51`) and `GroupCreate.shared_block_ids` (`group.py:38`) once replacements stabilize.
- **Memory observability**: expose staleness signals (age distribution, last-touch per block/passages) via API; today only per-file `last_accessed_at` exists (`block.py:111-114`).

## Questions / Gaps

- **No automated PII handling in memory pipelines.** No scrubbing/redaction pass was found between tool insertion (`archival_memory_insert`) and storage; privacy commitments live in prose (`PRIVACY.md:114-131` promises deletion rights) without a corresponding per-memory erasure workflow beyond manual deletes. Searched `letta/helpers/`, `letta/services/` for redact/anonymize/pii — no evidence found.
- **No memory-quality evaluation harness.** Nothing in the repo measures whether persisted memories remain accurate over time; correctness is delegated to the sleeptime agent's judgment (`sleeptime_v2.py:16-25`).
- **Undo/redo coverage for git-enabled agents vs SQL agents differs** (git commits vs `BlockHistory` rows); whether both histories stay consistent when the PG cache lags git (`block_manager_git.py:1-8`) is not covered by any test found in `tests/`.
- **Pinecone scope unclear for archival memory**: Pinecone integration appears wired for file/source passages (`services/file_processor/embedder/pinecone_embedder.py:26`, `source_manager.py:30`), while archival dual-write references only native/tpuf (`passage_manager.py:607`); whether archival-on-Pinecone is supported could not be confirmed from the inspected paths.
- **Cross-user memory within an org is permitted by construction** (org-scoped reads), which is correct for team deployments but means "user memory" is really "org-shared memory unless identities are used"; no per-actor ACL finer than org + read_only flags was found.

---

Generated by `05.03-long-term-user-project-domain-memory` against `letta`.
