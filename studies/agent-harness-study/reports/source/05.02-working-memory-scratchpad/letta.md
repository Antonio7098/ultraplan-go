# Source Analysis: letta

## Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas, Alembic migrations; MemGPT lineage) |
| Analyzed | 2026-08-25 |

Citations below are relative to the selected source root (`studies/agent-harness-study/sources/letta/`).

## Summary

Letta implements working memory as a first-class, durable, user-visible structure rather than a hidden scratchpad. The agent's "core memory" is a set of labeled `Block` objects (`letta/schemas/block.py:67`) held in a `Memory` container (`letta/schemas/memory.py:68`), rendered into the system prompt as `<memory_blocks>` XML with per-block character budgets (`letta/schemas/memory.py:142-173`, `compile()` at `letta/schemas/memory.py:688-732`). The model edits this working memory itself through a family of memory tools (`core_memory_append`, `memory_replace`, `memory_insert`, `memory_rethink`, `memory_apply_patch`; definitions in `letta/functions/function_sets/base.py:246-527`, execution in `letta/services/tool_executor/core_tool_executor.py:319-858`). Every edit is validated (read-only guard, line-number-prefix rejection), persisted to the database immediately, and the system prompt is recompiled only when the compiled memory string actually changed (`update_memory_if_changed_async`, `letta/services/agent_manager.py:1747-1801`).

There is no separate private scratchpad channel. Instead, Letta's design makes working notes explicit and shared: blocks are exposed to users via REST (`GET/PATCH /v1/agents/{id}/core-memory/blocks`, `letta/server/rest_api/routers/v1/agents.py:1206-1377`) and can be revised out-of-band by background "sleeptime" agents (`letta/groups/sleeptime_multi_agent_v3.py:127-188`). Conversation history is managed separately from working memory: an in-context message pointer list (`message_ids`, `letta/schemas/agent.py:78`) plus compaction/summarization that replaces old messages with a `role=summary` message while leaving core memory untouched (`letta/services/summarizer/compact.py:135-472`). An optional git-backed memory filesystem adds versioned, auditable memory commits for git-enabled agents (`letta/services/block_manager_git.py:1-33`, `letta/services/memory_repo/memfs_client_base.py:240-354`). Provider reasoning traces ("inner thoughts") are captured, stored on messages, and scrubbed from subsequent context windows so thinking does not accumulate as pseudo-facts (`letta/agents/letta_agent_v2.py:760-792`, `letta/helpers/reasoning_helper.py:25`).

## Rating

**9/10** — Mature, durable, observable, and extensible.

Rationale against the rubric:
- **Clear model**: working memory = labeled blocks with char limits, rendered into the prompt with usage metadata the model can see (`chars_current`/`chars_limit`, `letta/schemas/memory.py:161-166`); default budget 100k chars/block (`letta/constants.py:435`).
- **Explicit interfaces**: agent-side tool set (`letta/functions/function_sets/base.py:246-527`) and human-side REST API (`letta/server/rest_api/routers/v1/agents.py:1206-1377`) operate on the same underlying blocks.
- **Durability**: blocks are DB rows (`letta/orm/block.py`); edits persist synchronously inside the step via `update_memory_if_changed_async` (`letta/services/tool_executor/core_tool_executor.py:325`, `399`, `771`); optionally git-versioned with full commit history.
- **Operational safeguards**: read-only block enforcement (`core_tool_executor.py:320-321`), line-number-prefix rejection to prevent corrupt edits (`core_tool_executor.py:357-374`), compaction fallback chains (`letta/services/summarizer/compact.py:210-296`), post-compaction threshold verification (`compact.py:359-412`).
- **Observability**: OTel tracing on compile/edit paths (`@trace_method`, e.g. `letta/schemas/memory.py:112`, `117`), context-window overview API including token accounting of core memory (`letta/schemas/memory.py:23-65`), compaction stats embedded in summary messages (`letta/services/summarizer/compact.py:414-432`), git commit metadata (author/type/timestamp/diffstats, `letta/schemas/memory_repo.py:19-36`).
- **Proven under failure/scale**: layered summarization modes with automatic degradation (`self_compact_all` → `self_compact_sliding_window` → `all`, `letta/services/summarizer/compact.py:192-296`), GPT-5-specific proactive 90% compaction threshold documented from observed failures (`letta/services/summarizer/thresholds.py:27-41`).
- Kept from 10 by: acknowledged fragility in substring-based change detection (`letta/services/agent_manager.py:1562-1563` comment), `hidden` flag semantics that hide from API but not from the model (`letta/services/block_manager.py:349-351` vs `letta/schemas/memory.py:122-130`), and last-writer-wins concurrency on shared blocks.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.py:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Scratchpad fields | `Block.value` + char `limit`; `read_only` permission flag; `hidden` flag on the LLM-context block schema | `letta/schemas/block.py:19-20`, `36`, `41-44` |
| Todo state | No dedicated todo tool exists; todo-list-style scratchpads are anticipated as ordinary memory blocks (docstring example edits a `- [ ] Step 5: ...` todo list) | `letta/functions/function_sets/base.py:326` |
| Core memory container | `Memory.blocks` + `file_blocks`; `get_block`/`set_block`/`update_block_value` mutators; `BasicBlockMemory.core_memory_append/replace` reference implementations | `letta/schemas/memory.py:68-80`, `750-780`, `804-837` |
| Prompt rendering | `Memory.compile()` emits `<memory_blocks>` XML incl. `chars_current`/`chars_limit` metadata; line-numbered variant gated on agent type + Anthropic provider | `letta/schemas/memory.py:142-173`, `175-203`, `688-732`, `696-702` |
| Agent memory-edit tools | `core_memory_append/replace`, `memory_replace`, `memory_insert`, `memory_apply_patch` (multi-block codex-style diffs), `memory_rethink` (full rewrite), `memory_finish_edits` sentinel | `letta/functions/function_sets/base.py:246-280`, `311-388`, `391-450`, `453-485`, `488-517`, `520-527` |
| Tool execution & persistence | Executor writes through to DB after each edit via `agent_manager.update_memory_if_changed_async`; `archival_memory_insert` also forces system-prompt rebuild (counts change) | `letta/services/tool_executor/core_tool_executor.py:319-344`, `346-401`, `743-773`, `307-317` |
| Change detection & recompilation | Compiles new memory, compares against persisted system message content substring-wise, updates only changed block values, then rebuilds system message in place (keeps message id) | `letta/services/agent_manager.py:1747-1801`, `1552-1612` |
| Durable in-context pointer | Agent state persists `message_ids` (in-context window) across turns; helpers prepend/append/trim by id | `letta/schemas/agent.py:78`, `455`; `letta/services/agent_manager.py:1627-1652` |
| Clearing at task boundaries | `message_buffer_autoclear` drops all but the system message each turn while explicitly retaining core-memory/archival state | `letta/schemas/agent.py:138-142`; `letta/agents/helpers.py:81-86` |
| Compaction (working-set eviction) | `compact_messages` replaces evicted transcript with a `role=summary` message carrying packed compaction stats; multi-mode with fallback chain | `letta/services/summarizer/compact.py:135-472`, `192-296`, `427-465` |
| Compaction triggers | Post-step threshold check and context-window-exceeded retry path in v3 loop; GPT-5 proactive 90% threshold | `letta/agents/letta_agent_v3.py:1438-1488`, `938`; `letta/services/summarizer/thresholds.py:27-41` |
| Reasoning scrubbing | `_refresh_messages` scrubs inner thoughts from in-context messages each step start; scrubber implementation | `letta/agents/letta_agent_v2.py:760-792`; `letta/helpers/reasoning_helper.py:25` |
| Background rethinking (sleeptime) | Foreground turn spawns async sleeptime runs fed a transcript slice; sleeptime agent rewrites shared blocks | `letta/groups/sleeptime_multi_agent_v3.py:127-188`, `205-244` |
| Voice scratchpad buffers | In-RAM `message_transcripts` list on the voice sleeptime agent; `rethink_user_memory` writes target block; `store_memory` writes serialized slices to archival | `letta/agents/voice_sleeptime_agent.py:59`, `67-68`, `153-180` |
| Stateless ephemeral agents | `EphemeralAgent` is a thin OpenAI wrapper returning unpersisted messages — no working memory | `letta/agents/ephemeral_agent.py:16-21`, `39-56` |
| User exposure of working memory | REST endpoints: retrieve memory, get/list/modify core-memory blocks, attach/detach | `letta/server/rest_api/routers/v1/agents.py:1206`, `1221`, `1236`, `1268`, `1355`, `1369` |
| Hidden-block visibility rules | Listing filters `hidden=True` unless `show_hidden_blocks=true`; ORM column + index exist; rendering into the prompt does NOT filter hidden | `letta/services/block_manager.py:309`, `349-351`; `letta/orm/block.py:30`, `50`; `letta/schemas/memory.py:122-130` |
| Git-backed audit trail | `GitEnabledBlockManager` writes git first (source of truth), PG as cache; every block create/update/delete is a commit with author/timestamp; `MemoryCommit` records author_type/id, diffstats; local memfs default path `~/.letta/memfs` | `letta/services/block_manager_git.py:1-33`; `letta/services/memory_repo/memfs_client_base.py:33`, `240-354`, `362-388`; `letta/schemas/memory_repo.py:19-36` |
| External git access | `/v1/git/*` proxy supports the git HTTP protocol and triggers block sync after pushes (external editors can modify working memory) | `letta/server/rest_api/routers/v1/git_http.py:48`, `61`, `250` |
| Tests — memory semantics | ~25 unit tests covering compile variants (standard/line-numbered/git-structured), read-only metadata, duplicate file-block pruning, deprecated prompt template | `tests/test_memory.py:20-296` |
| Tests — rebuild efficiency | Rebuild skipped on noop/tag-only/metadata-only updates, triggered on real value changes | `tests/test_block_manager_noop_update.py:36`, `64`, `91`, `122` |
| Tests — background memory | Integration tests: sleeptime group chat, redundant-info removal, block editing, new block attachment | `tests/integration_test_sleeptime_agent.py:63`, `185`, `248`, `294` |
| Tests — external sync | Push-over-git triggers post-stream close sync | `tests/test_git_http_post_push_sync.py:36` |

## Answers to Dimension Questions

1. **Does the agent keep private task state?**
   Partially. The primary task state — core memory blocks — is persistent and model-writable but deliberately *not* private: it renders into the system prompt (`letta/schemas/memory.py:142-173`) and is readable/writable over the public REST API (`letta/server/rest_api/routers/v1/agents.py:1206-1377`). True private/ephemeral state exists only at narrow scopes: the voice sleeptime agent's in-RAM `message_transcripts` buffer (`letta/agents/voice_sleeptime_agent.py:59`, `67-68`), request-scoped caches like `last_function_response` and accumulated `TurnTokenData` within a run (`letta/agents/letta_agent_v3.py:954`, `1394-1400`), and provider reasoning content, which is extracted into structured reasoning fields (`letta/llm_api/openai_client.py:995-997`) and scrubbed out of later context windows (`letta/agents/letta_agent_v2.py:790-791`). There is no general-purpose hidden scratchpad tool; searching for "scratchpad" across `letta/` returns no hits (search boundary: recursive grep of `letta/**/*.py`).

2. **Is it durable?**
   Yes. Blocks are database entities (`letta/orm/block.py`) updated synchronously after every memory-tool call (`letta/services/tool_executor/core_tool_executor.py:325`, `399`, `771` → `letta/services/agent_manager.py:1747-1801`). The in-context message window is a durable id list on agent/conversation state (`letta/schemas/agent.py:78`; conversation-level `in_context_message_ids` at `letta/schemas/conversation.py:19`). For git-enabled agents durability extends to full versioning: git object storage is the source of truth and PostgreSQL acts as cache (`letta/services/block_manager_git.py:1-10`), with repos materialized locally at `~/.letta/memfs` in OSS deployments (`letta/services/memory_repo/memfs_client_base.py:33`).

3. **Is it exposed to users?**
   Yes, by design. Users can read and directly modify any core-memory block over REST (`operation_id`s `retrieve_agent_memory`, `list_core_memory_blocks`, `modify_core_memory_block`, attach/detach at `letta/server/rest_api/routers/v1/agents.py:1206-1377`), and can even push memory edits via the git protocol for git-enabled agents (`letta/server/rest_api/routers/v1/git_http.py:250`, sync hook at `:61`). The one visibility control is `hidden`: hidden blocks are excluded from listings unless `show_hidden_blocks=true` (`letta/services/block_manager.py:349-351`), but they are still rendered into the model's `<memory_blocks>` section because rendering iterates all blocks without a hidden filter (`letta/schemas/memory.py:122-130`). So "hidden" means hidden-from-API, not hidden-from-model — a leak-relevant nuance.

4. **Does it pollute long-term memory?**
   No automatic pollution path was found. Long-term archival grows only via explicit tools (`archival_memory_insert`, `letta/services/tool_executor/core_tool_executor.py:307-317`; voice `store_memory`, `letta/agents/voice_sleeptime_agent.py:164-180`). Compaction summaries are written as `role=summary` messages, not into blocks or passages (`letta/services/summarizer/compact.py:427-442`). Reasoning traces are stripped before the next step so chain-of-thought does not silently become conversational record (`letta/agents/letta_agent_v2.py:760-792`). The sleeptime agent integrates conversation facts into blocks explicitly under its own prompts (`letta/groups/sleeptime_multi_agent_v3.py:205-244`), which is curated promotion rather than leakage.

5. **Can it be audited?**
   Yes for git-enabled agents, partially otherwise. In git mode every create/update/delete produces a commit recording author type/id/name, timestamp, changed files, and diffstats (`letta/services/memory_repo/memfs_client_base.py:240-354`; schema `letta/schemas/memory_repo.py:19-36`), retrievable via `get_history_async` (`memfs_client_base.py:362`) and externally via the git HTTP proxy (`git_http.py:250`). In standard mode there is no first-class block-history table or REST endpoint (no matches for a block-history route in `letta/server/rest_api/routers/v1/`); auditing relies on indirect artifacts: in-place system-message swaps with united-diff logging (`letta/services/agent_manager.py:1590-1608`), OTel spans on compile/edit paths, step records/metrics, and the `ContextWindowOverview` snapshot API (`letta/schemas/memory.py:23-65`).

## Architectural Decisions

1. **Working memory lives in the prompt, not in a side channel.** Blocks are compiled into the system message (`letta/schemas/memory.py:688-732`) and the system message row is swapped in place when compiled memory changes, preserving its message id (`letta/services/agent_manager.py:1594-1608`). This keeps a single canonical context artifact and enables prefix-cache-friendly rebuilds (rebuild is skipped when the compiled string is unchanged, `agent_manager.py:1562-1567`).

2. **Model self-edits working memory through typed tools with immediate write-through.** Each executor method ends in `update_memory_if_changed_async` (`core_tool_executor.py:325`, `343`, `399`, `771`), so mid-task notes survive crashes between steps rather than being flushed at task end.

3. **Separation of conversation buffer from semantic working memory.** Eviction pressure applies to `message_ids` (trim/compaction, `letta/services/agent_manager.py:1627-1638`; `compact_messages`), never automatically to blocks; the autoclear option drops messages but still preserves blocks/archival (`letta/schemas/agent.py:138-142`).

4. **Transparency over secrecy.** Rather than hiding planning state, Letta gives humans first-class read/write access to the same blocks the model uses (`routers/v1/agents.py:1206-1377`), and moves curation work to a background sleeptime agent sharing those blocks (`letta/groups/sleeptime_multi_agent_v3.py:127-188`).

5. **Optional git semantics for memory.** Git-enabled agents get write-through versioning (git first, PG cache) with commit-per-edit and external collaboration via standard git remotes (`letta/services/block_manager_git.py:1-33`; `git_http.py:61`, `250`).

## Notable Patterns

- **Self-monitoring affordances in the render**: each block displays `read_only`, `chars_current`, `chars_limit` inline (`letta/schemas/memory.py:161-166`) so the model can budget its own notes; a view-only line-number gutter plus a warning banner support precise edit targeting, with hard rejection of line-number prefixes in tool args (`letta/constants.py:246`; `core_tool_executor.py:357-374`).
- **Codex-style patching of memory**: `memory_apply_patch` accepts unified diffs and `*** Add/Update/Delete Block:` headers to edit multiple blocks atomically in one call (`letta/services/tool_executor/core_tool_executor.py:403-462`).
- **Terminal-tool choreography**: memory-edit loops are bounded by tool rules — e.g., the voice sleeptime flow chains `store_memories → rethink_user_memory → finish_rethinking_memory` (`letta/agents/voice_sleeptime_agent.py:86-91`), and `memory_finish_edits` is an explicit no-op sentinel ending rethink sessions (`core_tool_executor.py:775-776`).
- **Fallback ladders in compaction**: `self_compact_all → self_compact_sliding_window → all`, with per-mode exception handling (`letta/services/summarizer/compact.py:192-296`) and a final threshold re-check that distinguishes recoverable overflow from an oversized system prompt (`compact.py:359-412`).
- **Change-gated persistence**: both block writes and system-prompt rebuilds are diff-gated (`agent_manager.py:1768-1781`, `1562-1567`), with dedicated tests asserting rebuild skips for noop/tag-only/metadata-only updates (`tests/test_block_manager_noop_update.py:36-122`).

## Tradeoffs

- **Transparency vs. privacy**: making working memory user-readable maximizes trust and debuggability but removes any confidential planning space; the only concealment mechanism (`hidden`) does not affect what the model sees (`block_manager.py:349-351` vs `schemas/memory.py:122-130`).
- **Immediate durability vs. cost**: write-through on every edit triggers compiled-string comparison and potential prompt rebuild per call (`agent_manager.py:1747-1801`); the substring short-circuit mitigates cost but introduces correctness risk (below).
- **Full-rewrite power vs. safety**: `memory_rethink` replaces whole blocks and auto-creates missing ones (`base.py:488-517`), enabling condensation but risking information loss with no undo outside git mode.
- **Shared blocks enable background curation but create contention**: foreground and sleeptime agents can mutate the same block concurrently; resolution is last-writer-wins at the DB update layer (`agent_manager.py:1770-1781`), with no merge semantics outside git mode.
- **Rich render metadata costs tokens**: line-number gutters and warning banners double the surface the model must avoid copying into arguments (hence the regex guards), a deliberate spend of context for edit precision (`constants.py:246`; `memory.py:175-203`).

## Failure Modes / Edge Cases

- **Substring change detection can miss removals.** The skip-rebuild check tests whether the newly compiled memory string is contained in the current system message; an inline comment concedes "(substring match would still work)" if a block were removed (`letta/services/agent_manager.py:1562-1563`) — stale prompt risk on structural changes until a forced rebuild occurs.
- **Ambiguous edit targets fail closed.** `memory_replace` raises when `old_string` appears zero or multiple times, listing colliding lines (`functions/function_sets/base.py:362-373`); repeated partial-failure loops are possible if the model retries blindly.
- **Compaction may not reach the target.** If post-compaction tokens remain above threshold, the code logs critical and continues ("don't brick the agent", `letta/services/summarizer/compact.py:409-412`), leaving an oversized working set; only an oversized system prompt raises `SystemPromptTokenExceededError` (`compact.py:391-407`).
- **Summarizer outage degrades gracefully but lossily**, falling down the mode ladder to plain `all` summarization (`compact.py:210-296`).
- **Hidden-flag misconception**: a developer marking a block `hidden` to keep secrets from the model would be mistaken — only API listings filter it (`block_manager.py:349-351`); the value still ships in the prompt (`schemas/memory.py:122-130`).
- **Non-durable voice scratchpad**: `VoiceSleeptimeAgent.message_transcripts` lives only in the process instance (`voice_sleeptime_agent.py:59`); a crash loses the buffer the `store_memories` indices refer to.
- **Ephemeral agents have none of these guarantees**: `EphemeralAgent.step` returns messages without persistence (`ephemeral_agent.py:51-56`), appropriate for summarization sidecars (`letta/services/summarizer/summarizer.py:43-64` wires `EphemeralSummaryAgent`).

## Future Considerations

- Add a first-class block-history/audit endpoint for non-git agents (e.g., exposing block revisions analogous to `MemoryCommit`), since today's OSS story requires opting into git memory for real auditability.
- Replace substring-based rebuild gating with a hash or revision counter on the compiled memory to eliminate the removal blind spot flagged at `letta/services/agent_manager.py:1562-1563`.
- Extend the `hidden` flag semantics to optionally exclude blocks from prompt rendering, giving a true model-private channel for sensitive notes.
- Provide optimistic concurrency (e.g., expected-value checks or per-block versions) for shared blocks edited simultaneously by foreground and sleeptime agents.
- Persist the voice `message_transcripts` buffer durably (or derive it from message records) to make `store_memories` index arguments crash-safe.

## Questions / Gaps

- No evidence found of a dedicated planner-output channel (hidden plan text injected into prompts but withheld from users); searches for "scratchpad"/"plan state" in `letta/**` returned nothing beyond block-based memory. If such a feature exists, it lives outside this repository (likely Letta Cloud).
- Whether concurrent block writes from multiple processes are serialized at the DB level (row locks vs. last-write-wins) could not be confirmed from application code alone; `update_block_async` performs direct updates without visible optimistic-lock columns (`letta/services/agent_manager.py:1770-1781`).
- The cloud memfs client referenced in comments ("The cloud/enterprise version (memfs_client.py)") is not present in this source tree, so enterprise-side audit/visibility behavior could not be verified (`letta/services/memory_repo/memfs_client_base.py:7-8`).
- Retention/cleanup policies for git-backed memory repos (garbage collection, repo deletion lifecycle) were not found in the inspected paths beyond repo creation/commit operations.

---

Generated by `05.02-working-memory-and-scratchpad` against `letta`.
