# Source Analysis: letta

## Dimension 05.05 — Memory Write Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas; optional git-backed memory via git CLI + Redis) |
| Analyzed | 2026-08-26 |

*All citations below are relative to the source root `sources/letta/`.*

## Summary

Letta's memory write policy is tool-mediated and multi-layered. The model writes memory by invoking a family of first-class "memory tools" (`core_memory_append`, `core_memory_replace`, `memory_replace`, `memory_insert`, `memory_apply_patch`, `memory_rethink`, plus a filesystem-style `memory` command), which are dispatched to a native `CoreToolExecutor` that mutates in-context `Block`s and persists them through `AgentManager.update_memory_if_changed_async` → `BlockManager.update_block_async` (`letta/services/tool_executor/core_tool_executor.py:319-344`, `letta/services/agent_manager.py:1747-1781`). Archival memory is written via `archival_memory_insert`, which delegates to `PassageManager.insert_passage` with embedding generation (`letta/services/tool_executor/core_tool_executor.py:307-317`, `letta/services/passage_manager.py:543-593`).

Writes happen both explicitly (the main agent calls a memory tool mid-conversation) and automatically (background "sleeptime" agents run every N turns and rewrite shared blocks; voice agents persist evicted dialogue chunks via `store_memories`) (`letta/groups/sleeptime_multi_agent_v4.py:132-148`, `letta/agents/voice_sleeptime_agent.py:153-241`). Validation before storing includes read-only block guards enforced both pre-execution and post-execution on a deep-copied state, exact-match/uniqueness checks for string replacements, line-number-artifact rejection, and null-byte sanitization (`letta/schemas/block.py:51-57`, `letta/agent.py:190-198,1610-1624`). Conflict handling relies on optimistic locking, no-op update detection, an undo/redo `BlockHistory` audit trail with actor attribution, and Redis-locked linear git commits for the git-backed memory filesystem (`letta/services/block_manager.py:225-240,842-911`, `letta/orm/block_history.py:10-59`, `letta/services/memory_repo/git_operations.py:351-409`).

What the policy does *not* have: hard enforcement of block character limits (limits are advisory prompt metadata), any confidence/provenance metadata on memories, sensitive-fact filtering before storage, or a memory-specific user approval gate (approval exists only as a generic per-tool mechanism).

## Rating

**7 / 10.**

Rationale: Letta has a clear, deliberate write model — purpose-differentiated tool sets per agent type (`letta/constants.py:118-150`), explicit natural-language policy in system prompts ("be selective... not every observation warrants a memory edit", `letta/prompts/system_prompts/sleeptime_v2.py:1-39`), operational safeguards (read-only guards pre/post execution, exact-match edit semantics, undo/redo checkpoints, git versioning with locks), and REST surfaces for external correction. It falls short of 8–10 because: block char limits are rendered into prompts but never programmatically enforced (and `bulk_update_block_values_async`'s docstring even claims enforcement that doesn't exist, `letta/services/block_manager.py:797-836`); archival insertion has open TODOs for token-count validation and stores unchunked single passages (`letta/services/passage_manager.py:566-571`); there is zero confidence/provenance metadata; no sensitive-fact exclusion; and the same tools exist in two parallel implementations (legacy `function_sets/base.py` stubs vs. native executor methods) that can drift.

## Evidence Collected

Every entry includes a file path with line numbers relative to `sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Memory write functions (native) | `core_memory_append`, `core_memory_replace`, `memory_replace`, `memory_apply_patch`, `memory_insert`, `memory_rethink`, `memory_delete/create/rename/update_description/str_replace/str_insert`, unified `memory` command dispatcher | `letta/services/tool_executor/core_tool_executor.py:319,328,346,403,683,743,778,860,884,951,1014` |
| Memory tool sets per agent type | `BASE_MEMORY_TOOLS = ["core_memory_append", "core_memory_replace", "memory", "memory_apply_patch"]`; V2/V3 sets; `BASE_SLEEPTIME_TOOLS` includes `memory_rethink` + `memory_finish_edits`; voice set includes `store_memories`, `rethink_user_memory` | `letta/constants.py:118-150` |
| Legacy function-set definitions | `core_memory_append/replace`, `memory_replace`, `memory_rethink` defined as docstring-bearing stubs executed via `ToolType.LETTA_MEMORY_CORE` dispatch | `letta/functions/function_sets/base.py:246-280,311-388,488-517`; dispatch at `letta/agent.py:1616-1624` |
| Automatic writes: sleeptime trigger | `run_sleeptime_agents` gated by `group.sleeptime_agent_frequency` turn counter; runs after main-agent response in `finally` | `letta/groups/sleeptime_multi_agent_v4.py:126-148` |
| Automatic writes: voice eviction extraction | `store_memories(chunks)` with structured `MemoryChunk(start_index, end_index, context)` schema; implemented natively as passage inserts into the *conversation* agent's archive | `letta/functions/function_sets/voice.py:38-66`; impl at `letta/agents/voice_sleeptime_agent.py:143-153,164-180` |
| Sleeptime write policy prompt | Instructs selective editing, precise timestamps, multi-step edits, finish tool; doc-ingest variant mandates `memory_rethink` for source blocks | `letta/prompts/system_prompts/sleeptime_v2.py:4-39`; `letta/prompts/system_prompts/sleeptime_doc_ingest.py:16-27` |
| Persistence path | Every native write ends in `update_memory_if_changed_async`: recompiles memory, diffs against current system message, updates only changed blocks, refreshes from DB | `letta/services/agent_manager.py:1747-1790`; diff logic at `:1768-1784` |
| Read-only guard (pre-write) | Each write tool raises `READ_ONLY_BLOCK_EDIT_ERROR` if `block.read_only` | `letta/services/tool_executor/core_tool_executor.py:320-321,336-337,354-355,530-531,665-666,691-692,744-745`; constant at `letta/constants.py:424` |
| Read-only guard (post-write) | `ensure_read_only_block_not_modified` verifies a deep-copied state after LETTA_MEMORY_CORE/SLEEPTIME_CORE tool execution | `letta/agent.py:190-198,1616-1624` |
| Edit validation: uniqueness & exact match | Replace fails if `old_string` occurs 0 or >1 times (reports offending line numbers); patch hunks must match exactly once | `letta/services/tool_executor/core_tool_executor.py:380-391,495-506` |
| Edit validation: view-artifact rejection | Rejects line-number prefixes and the line-number warning banner inside write payloads | `letta/services/tool_executor/core_tool_executor.py:357-374,418-423,694-705,747-758` |
| Content sanitization | `sanitize_value_null_bytes` strips `\x00` from block values (PostgreSQL safety); `validate_assignment=True` revalidates on attribute set | `letta/schemas/block.py:51-64` |
| Block size limit (soft only) | Default `limit=100000` chars stored and rendered as `chars_current/chars_limit` metadata to the model; no code enforces `len(value) <= limit` anywhere in the write path | `letta/constants.py:435`; `letta/schemas/block.py:20`; rendering at `letta/schemas/memory.py:161-166`; unenforced claim in `letta/services/block_manager.py:811-813` vs implementation `:815-836` |
| Archival insert validation gap | `insert_passage` has `TODO: check to make sure token count is okay for embedding model`, inserts one unchunked passage, falls back to `None` embeddings | `letta/services/passage_manager.py:562-593` |
| No-op / conflict detection | `update_block_async` skips no-op scalar/tag changes; detects prompt-affecting field changes and rebuilds connected agents' system prompts | `letta/services/block_manager.py:32,225-240,266-270` |
| Undo/redo & audit trail | `BlockHistory` rows store full snapshots with `actor_type`/`actor_id` and monotonic sequence; `checkpoint_block_async` truncates future entries for linear history; undo/redo helpers | `letta/orm/block_history.py:14-58`; `letta/services/block_manager.py:842-911,952+` |
| Git-backed memory conflict control | `GitOperations.commit` acquires a per-agent Redis lock (`MemoryRepoBusyError` on contention), commits via git CLI, author attributed to actor | `letta/services/memory_repo/git_operations.py:351-409`; markdown serialization `letta/services/memory_repo/block_markdown.py:27-75`; client wrapper `letta/services/memory_repo/memfs_client_base.py:208-258` |
| User correction APIs | REST `PATCH /v1/blocks/{block_id}` → `update_block_async`; server-level `update_agent_core_memory(label→value)` rebuilds prompt | `letta/server/rest_api/routers/v1/blocks.py:157-166`; `letta/server/server.py:1058-1066` |
| Approval flow (generic, not memory-specific) | `Tool.default_requires_approval` flag; approval responses parsed into approved/denied/client-executed lists in the step loop | `letta/schemas/tool.py:59-61`; `letta/schemas/message.py:178`; `letta/agents/letta_agent_v3.py:973-1017` |
| Custom tools can mutate memory | Sandboxed tool executions persist `tool_execution_result.agent_state.memory` via the same `update_memory_if_changed_async` path | `letta/services/tool_executor/sandbox_tool_executor.py:142` |
| Confidence metadata | `grep -ri confidence` across `letta/` returns zero matches in schemas/services/tools — absent by design omission | Search boundary: `letta/schemas/`, `letta/services/`, `letta/functions/` |
| Tests | `test_memory.py` covers compile/rendering, duplicate file-block pruning, read-only metadata display; no unit tests found for native executor write validation (only legacy data fixtures + integration tests requiring credentials) | `letta/tests/test_memory.py:20-464`; search boundary: `letta/tests/` |

## Answers to Dimension Questions

**1. What causes memory to be written?**
Four triggers: (a) explicit model tool calls during a chat step (`core_memory_append` etc., `letta/services/tool_executor/core_tool_executor.py:319-773`); (b) background sleeptime agents invoked on a turn-frequency counter that rewrite shared core-memory blocks (`letta/groups/sleeptime_multi_agent_v4.py:132-148`); (c) voice-pipeline evictions where `store_memories` summarizes soon-to-be-evicted dialogue into archival passages (`letta/agents/voice_sleeptime_agent.py:143-180`); (d) out-of-band API calls (`PATCH /v1/blocks/{id}`, `letta/server/rest_api/routers/v1/blocks.py:157-166`). Additionally, arbitrary sandboxed custom tools can mutate `agent_state.memory` and their changes are persisted (`letta/services/tool_executor/sandbox_tool_executor.py:142`).

**2. Can the model write arbitrary memory?**
Largely yes, within structural bounds. `memory_rethink` overwrites an entire block with model-chosen text and will create a new block if the label doesn't exist (`letta/services/tool_executor/core_tool_executor.py:743-773`); `memory_create`/`memory_apply_patch` extended mode can add/delete/rename blocks (`:555-657`); `archival_memory_insert` accepts arbitrary content with optional tags (`:307-317`). Constraints are mechanical only: target label must exist (except rethink/create), read-only flags are respected, replacements must match exactly once, and values pass through null-byte sanitization (`letta/schemas/block.py:51-57`). There is no factuality check, deduplication, or content policy applied to what gets stored.

**3. Are facts verified?**
No. Verification is limited to syntactic integrity of the *edit operation*, not truth of the *content*: exact-match/uniqueness checks (`core_tool_executor.py:380-391,495-506`), artifact rejection (`:357-374`), and embedding-request failure propagation in archival inserts (`letta/services/passage_manager.py:572-582`). Nothing cross-examines new memories against existing ones or sources. The sleeptime prompt asks the model to preserve non-outdated information and use precise dates (`letta/prompts/system_prompts/sleeptime_v2.py:12-19`) — stated intent, not enforced behavior.

**4. Can users correct memory?**
Yes, through three mechanisms: direct REST modification of any block (`letta/server/rest_api/routers/v1/blocks.py:157-166`), the server convenience method `update_agent_core_memory` (`letta/server/server.py:1058-1066`), and undo/redo of block history checkpoints (`letta/services/block_manager.py:842-911,952+`). Blocks can also be marked `read_only` so agents can never overwrite user-authored content (`letta/schemas/block.py:36`; enforcement at `letta/services/tool_executor/core_tool_executor.py:320-321` and post-hoc at `letta/agent.py:1620-1624`). However, there is no interactive approve/reject prompt specifically for memory writes — the generic tool-approval mechanism would apply only if an operator sets `default_requires_approval=True` on a memory tool (`letta/schemas/tool.py:59-61`; flow at `letta/agents/letta_agent_v3.py:973-1017`); default memory tool creation sets no such flag (no evidence found in `letta/constants.py:118-150` or tool-registration paths).

**5. Are sensitive facts excluded?**
No evidence of any filtering. A search across `passage_manager.py`, `block_manager.py`, and `core_tool_executor.py` for PII/redaction/sensitive handling found nothing; the only content transformation is null-byte stripping (`letta/schemas/block.py:51-57`). Secret management exists but only for agent environment credentials, entirely separate from memory content (`letta/schemas/secret.py:35-159`). Whatever the model decides to memorize is stored verbatim.

## Architectural Decisions

1. **Memory writes as typed tools, not hidden side effects.** All core-memory mutations flow through registered tools with declared schemas (`letta/constants.py:118-150`), making the write surface enumerable, permission-scopable (`read_only`), and visible to the model as callable API.
2. **Diff-based persistence with idempotent no-op detection.** Rather than blindly saving, `update_memory_if_changed_async` recompiles memory and compares against the stored system message before writing changed blocks (`letta/services/agent_manager.py:1762-1781`); `update_block_async` skips true no-ops (`letta/services/block_manager.py:225-240`). This avoids spurious prompt rebuilds that would break prefix caching.
3. **Two-tier memory with different write semantics.** Core memory (in-context `Block`s) supports precise edit operations mirroring a code editor (str_replace with uniqueness, unified-diff patching, line inserts — `core_tool_executor.py:346-741`), while archival memory is append-only passages with embeddings (`passage_manager.py:543-593`). Corrections to archival content are not modeled at all.
4. **Defense-in-depth on protected blocks.** Read-only enforcement happens both before mutation and again after execution against a deep-copied agent state (`letta/agent.py:1616-1624`), covering both native and legacy tool paths.
5. **Git as the durability substrate for advanced memory.** Git-enabled agents serialize blocks to markdown files with frontmatter and commit through a locked, attributed pipeline (`letta/services/memory_repo/block_markdown.py:27-75`, `git_operations.py:351-409`), gaining history, diffability, and external edit support for free.
6. **Policy lives in prompts, enforcement in code — partially.** Selectivity guidance ("be selective… high recall") exists only in the sleeptime system prompt (`letta/prompts/system_prompts/sleeptime_v2.py:29-32`), whereas size limits exist only as prompt-rendered metadata (`letta/schemas/memory.py:161-166`); neither has a code-side backstop.

## Notable Patterns

- **Editor-metaphor tool design**: `memory_apply_patch` implements codex-style `*** Add/Update/Delete Block` headers with hunk matching exactly-once semantics (`letta/services/tool_executor/core_tool_executor.py:410-681`), and error messages coach the model toward correct usage (reporting matched line numbers, suggesting more context).
- **View-model vs. storage-model separation**: line numbers shown to the model are explicitly view-only, and every write tool rejects attempts to include them (`core_tool_executor.py:357-374`).
- **Actor-attributed history**: `BlockHistory` snapshots record whether a change came from `LETTA_AGENT` or `LETTA_USER` with IDs (`letta/services/block_manager.py:888-900`, `letta/orm/block_history.py:36-39`) — a genuine observability hook for "who wrote this memory".
- **Frequency-gated background processing**: sleeptime cost is tunable via `sleeptime_agent_frequency` turn counting persisted through `GroupManager` (`letta/groups/sleeptime_multi_agent_v4.py:139-146`).
- **Cross-agent shared-state discipline**: block IDs are taken from `new_memory` rather than stale agent state because conversation-isolated blocks may differ (`letta/services/agent_manager.py:1776-1778`), acknowledging the main-agent/sleeptime-agent shared-block race.

## Tradeoffs

- **Model autonomy vs. safety**: giving the model whole-block rewrite (`memory_rethink`) and block lifecycle commands maximizes flexibility but means a hallucinated "correction" silently destroys good memory; the only recovery is manual undo/redo or git history.
- **Soft limits keep prompts flexible but risk overflow**: enforcing `chars_limit` in code could truncate legitimate reorganizations, so it's advisory — but nothing stops unbounded growth until context-window errors surface elsewhere (`letta/llm_api/error_utils.py:22`).
- **Dual implementations (legacy function sets + native executor)** preserve backwards compatibility but duplicate validation logic with subtle differences (legacy `core_memory_append` in `letta/functions/function_sets/base.py:246-260` performs no read-only check; the native one does at `core_tool_executor.py:320-321`).
- **Append-only archival is simple and cheap** but makes contradiction resolution impossible without retrieval-time judgment; no TTL, decay, or supersession mechanism exists.
- **Redis-locked git commits guarantee linear history** (`git_operations.py:351-377`) at the cost of serialization — concurrent writers get `MemoryRepoBusyError` rather than merge resolution.

## Failure Modes / Edge Cases

- **Unenforced size limit**: a sleeptime agent can append past `chars_limit` indefinitely; the docstring of `bulk_update_block_values_async` claims a `ValueError` on limit violation that the implementation never raises (`letta/services/block_manager.py:797-836`).
- **Silent last-writer-wins between agents**: main agent and sleeptime agent share blocks; `update_memory_if_changed_async` mitigates with fresh DB reads per write (`agent_manager.py:1759`), but two near-simultaneous writes can still interleave at the block level (optimistic locking is referenced only in checkpoint flows, `block_manager.py:850-857`).
- **Archival insert is all-or-nothing per call with no chunking**: a single oversized `text` becomes one passage with an acknowledged TODO for embedding token limits (`passage_manager.py:566-567`); embedding failures degrade to `None` embeddings (search-only fallback, `:580-582`).
- **Voice `store_memories` fan-out**: each chunk inserts independently; partial success aggregates to `status="error"` while some passages were already committed (`voice_sleeptime_agent.py:143-153`).
- **Custom-tool memory mutation bypasses tool-level guards**: sandboxed tools mutate a live copy and changes persist via `sandbox_tool_executor.py:142` without the per-tool read-only pre-checks (post-hoc `ensure_read_only_block_not_modified` covers the sync path, `letta/agent.py:1616-1624`, but not the async sandbox path shown here).
- **Approval-response corruption edge case** is handled defensively (malformed approvals abort the step, `letta_agent_v3.py:1007-1017`), showing awareness that human-in-the-loop writes can arrive broken.

## Future Considerations

- Enforce `block.limit` in `BlockManager.update_block_async` (or clamp with an explicit tool-return warning) so the advertised constraint is real.
- Add provenance/confidence fields to `Block`/`Passage` (e.g., source message ID, authoring agent, timestamp already partially available via `BlockHistory.actor_*`) to enable trust-weighted retrieval and later pruning.
- Implement archival-memory reconciliation: dedup, supersession, or contradiction marking on `insert_passage`.
- Add an opt-in approval gate for destructive memory operations (`memory_rethink`, `memory_delete`) leveraging the existing `default_requires_approval` machinery.
- Consolidate legacy `function_sets/base.py` implementations with native executor methods to eliminate divergent guard behavior.
- Content screening hooks (PII/secret redaction) at the `insert_passage`/`update_block` boundaries.

## Questions / Gaps

- **Are memory-tool writes ever approval-gated in practice?** The mechanism exists generically, but no evidence was found of shipped configurations setting `default_requires_approval=True` for memory tools (searched `letta/constants.py`, tool registration in `letta/services/tool_manager.py`, and agent creation paths).
- **Is there test coverage for write-path validation?** `letta/tests/test_memory.py` focuses on prompt compilation/rendering; no unit tests were found exercising `CoreToolExecutor.memory_*` validation branches (uniqueness, read-only, line-number rejection). Integration tests (e.g., `letta/tests/integration_test_sleeptime_agent.py`) require live credentials, so CI verification of write policy is unclear.
- **How do conversation-isolated blocks interact with shared sleeptime edits at scale?** The comment at `letta/services/agent_manager.py:1776-1778` hints at isolation complexity, but the concurrency guarantees under simultaneous main/sleeptime writes could not be fully determined from static reading.
- **Does the git-memory path enforce anything extra?** `MemfsClient.update_block_async` serializes and commits but applies no additional content validation beyond the shared block schema (`letta/services/memory_repo/memfs_client_base.py:208-258`); enterprise memfs service behavior is outside this repository.

---

Generated by `Dimension 05.05: Memory Write Policy` against `letta`.
