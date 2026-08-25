# Source Analysis: letta

## 05.08 Memory Evaluation and Freshness

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, SQLAlchemy ORM, Pydantic schemas, pytest) |
| Analyzed | 2026-08-25 |

## Summary

Letta treats memory as a first-class, persistent, and *editable* data structure rather than an ephemeral prompt artifact, and this shapes its freshness/correction story: core memory is a set of typed `Block` objects (`letta/schemas/block.py:13-85`) rendered into the system prompt (`letta/schemas/memory.py:688-732`), agents and background "sleeptime" agents mutate it through guarded edit tools (`letta/services/tool_executor/core_tool_executor.py:319-389`), every block mutation is versioned for undo/redo (`letta/services/block_manager.py:757-930`, `letta/orm/block_history.py:11`) or committed into a git-backed memory repository with author attribution (`letta/schemas/memory_repo.py:19-36`). Freshness is surfaced in three places: relative timestamps (`time_ago`) on recall-search results (`letta/services/tool_executor/core_tool_executor.py:186-229`), a `<memory_metadata>` block injected into the system prompt with "System prompt last recompiled" plus recall/archival counts (`letta/prompts/prompt_generator.py:26-89`), and `last_accessed_at` tracking on file blocks (`letta/schemas/block.py:111-114`).

However, on the *evaluation* side of this dimension the picture is thin: retrieval quality is exercised functionally (tag/date/top-k filters in `tests/managers/test_passage_manager.py:1178-1420`) and relevance scores are exposed per result (`rrf_score`, `fts_rank`, `vector_rank` — `letta/schemas/message.py:2703-2710`), but there are no retrieval-accuracy metrics (no precision/recall/hit-rate), no memory benchmark datasets (searched for LoCoMo/LongMemEval/needle-in-haystack — nothing), no outcome comparisons proving memory improves task success, and the single behavioral memory-quality test (sleeptime deduplication) is marked `@pytest.mark.skip` (`tests/integration_test_sleeptime_agent.py:183-185`). The team has built strong *machinery* to correct and trace memory, but cannot yet *prove* memory helps rather than hurts.

## Rating

**5 / 10 — Present but inconsistent; correction/freshness infrastructure is solid and tested at the mechanism level, while quality evaluation itself is ad-hoc, integration-gated, and partially skipped.**

Rationale against the rubric:

- **Why above 4:** Explicit interfaces exist for every correction flow (`core_memory_replace`, `memory_replace`, `rethink_memory`), stale-memory handling is addressed by design (background sleeptime reorganization with anti-relative-date instructions, `letta/prompts/system_prompts/sleeptime_v2.py:16-20`; voice variant explicitly says "Remove or correct outdated or contradictory information", `letta/prompts/system_prompts/voice_sleeptime.py:62`), wrong memory is correctable via exact-match replace with ambiguity rejection and full block history undo (`tests/managers/test_block_manager.py:985-1016`).
- **Why not 7+:** No retrieval-accuracy measurement exists anywhere; the flagship memory-quality behavioral test is skipped (`@pytest.mark.skip` before `test_sleeptime_removes_redundant_information`, `tests/integration_test_sleeptime_agent.py:183-185`); remaining memory-quality tests require live Anthropic API + running server (`tests/integration_test_sleeptime_agent.py:63,248`) and one asserts only a weak condition (`fact_block.value.count("Inter Miami") > 1`, line 290); no eval datasets, no regression suite tying memory behavior to task outcomes.

## Evidence Collected

Every entry includes a file path with line numbers relative to `sources/letta/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core memory schema | `BaseBlock` with `value`, `limit`, `label`, `description`, `read_only`, `metadata`; `FileBlock.last_accessed_at` freshness field | `letta/schemas/block.py:13-44`, `letta/schemas/block.py:107-114` |
| Memory rendering | `Memory.compile()` renders blocks with `chars_current`/`chars_limit` metadata into prompt | `letta/schemas/memory.py:688-732`, `letta/schemas/memory.py:143-173` |
| Correction tool: append/replace | Agent-facing `core_memory_append`/`core_memory_replace` persist via `update_memory_if_changed_async` | `letta/services/tool_executor/core_tool_executor.py:319-344` |
| Correction guardrails | `memory_replace` rejects non-unique/multi-match `old_string` and line-number-prefixed args | `letta/services/tool_executor/core_tool_executor.py:346-389`, `letta/functions/function_sets/base.py:311-388` |
| Reorganization tool | `rethink_memory` docstring: rewrite removing info that is "not outdated or inconsistent" | `letta/functions/function_sets/base.py:283-302` |
| Edit-loop termination | `memory_finish_edits` ends sleeptime editing loop; listed in default sleeptime tools | `letta/functions/function_sets/base.py:520-522`, `letta/constants.py:139` |
| Read-only protection | `core_memory_append/replace` raise on `read_only` blocks | `letta/services/tool_executor/core_tool_executor.py:320-321,336-337` |
| Freshness in recall results | `conversation_search` returns ISO `timestamp` plus computed `time_ago` ("5m ago"/"3d ago") per message | `letta/services/tool_executor/core_tool_executor.py:186-229` |
| Retrieval relevance metadata | Results carry `rrf_score`, `vector_rank`, `fts_rank`, `search_mode`; hybrid search default | `letta/schemas/message.py:2703-2710`, `letta/services/message_manager.py:1142-1165,1236` |
| Temporal filtering of archival search | `archival_memory_search(start_datetime, end_datetime)`; SQL filter `created_at >= start_date` | `letta/services/tool_executor/core_tool_executor.py:278-300`, `letta/services/helpers/agent_manager_helper.py:979-984` |
| Passage timestamps | Passages store `created_at`; DB indexes on `created_at` for source & archival passages | `letta/schemas/passage.py:47`, `letta/orm/passage.py:65,95` |
| In-context freshness metadata | `<memory_metadata>` includes "System prompt last recompiled" timestamp, recall/archival counts, archival tags | `letta/prompts/prompt_generator.py:26-89` |
| File-access freshness | `last_accessed_at` refreshed on open/close/search operations | `letta/services/files_agents_manager.py:43,91,140,166` |
| Stale-memory policy in prompts | Sleeptime prompt demands absolute dates ("do not write 'today'… because the memory is persisted indefinitely") and removal of redundant/outdated content | `letta/prompts/system_prompts/sleeptime_v2.py:16-20`, `letta/prompts/system_prompts/voice_sleeptime.py:62-65` |
| Block history versioning | `checkpoint_block_async` snapshots state into `BlockHistory`; `_move_block_to_sequence` restores; undo/redo supported | `letta/services/block_manager.py:757-930`, `letta/orm/block_history.py:11-50` |
| Git-backed memory commits | `MemoryCommit` records sha, parent, author_type (`agent`/`user`/`system`), timestamp, files_changed; `GitOperations.commit/get_history` | `letta/schemas/memory_repo.py:19-36`, `letta/services/memory_repo/git_operations.py:351,540`, `letta/services/block_manager_git.py:533-562` |
| Change detection | `update_memory_if_changed_async` compares compiled memory string against system message substring; only changed blocks persisted | `letta/services/agent_manager.py:1747-1799` |
| Functional retrieval tests | Search by query/tag(any/all)/top_k/datetime ranges asserted against seeded passages | `tests/managers/test_passage_manager.py:1178-1420` |
| Block-history regression tests | Checkpoint creates exactly one history row; idempotent checkpoints; concurrency/version checks; undo restores prior value | `tests/managers/test_block_manager.py:776-1016` |
| Noop-update regression test | Identical block update skips persistence and downstream prompt rebuilds | `tests/test_block_manager_noop_update.py:31-49` |
| Rendering regression tests | Recompile output changes after block value/label/description edits (mechanism behind rebuild detection) | `tests/test_memory.py:550-641` |
| Cache invalidation on memory edit | Prompt-caching test asserts cache invalidated after memory update | `tests/test_prompt_caching.py:522` |
| Sleeptime behavioral test (SKIPPED) | `test_sleeptime_removes_redundant_information` asserts duplicate "fiddle leaf" lines collapse — disabled via `@pytest.mark.skip` | `tests/integration_test_sleeptime_agent.py:183-238` |
| Stale-fact correction test (integration-only) | `test_sleeptime_edit` seeds outdated Messi facts, sends update, asserts block now references Inter Miami; requires live Anthropic model | `tests/integration_test_sleeptime_agent.py:248-290` |
| Tool topology test | Main agent must NOT have memory-edit tools; sleeptime agent must have `memory_rethink`/`memory_insert`/`memory_replace`/`memory_finish_edits` | `tests/integration_test_sleeptime_agent.py:83-87,115-121` |
| Absent: accuracy metrics/benchmarks | Searched `accuracy|precision|recall@|hit_rate`, `locomo|longmemeval|needle|haystack`, `eval|benchmark` across repo — no memory eval harness or dataset found (only unrelated timing "precision" hits, e.g. `letta/schemas/run.py:48`) | N/A (negative result) |
| Absent: outcome comparison | `RunMetrics` track steps/time/tools used, not memory-quality or task-success deltas | `letta/schemas/run_metrics.py:13-23` |

## Answers to Dimension Questions

1. **Is memory quality tested?** Partially, and weakly. Mechanism-level tests are solid (rendering `tests/test_memory.py:41-102`, history/undo `tests/managers/test_block_manager.py:776-1016`, noop updates `tests/test_block_manager_noop_update.py:31-49`). But behavioral memory-quality testing is nearly absent: the deduplication test is skipped (`tests/integration_test_sleeptime_agent.py:183-185`), and the stale-fact correction test (`:248-290`) is an integration test requiring a live server + Anthropic key, asserting only `count("Inter Miami") > 1`. There is no offline eval suite, no golden dataset, no scoring function.
2. **Are stale memories detected?** Not automatically detected by code. Staleness is handled *generatively*: sleeptime agents are instructed to keep blocks "comprehensive, readable, and up to date" and to purge redundant/outdated information during background runs (`letta/prompts/system_prompts/sleeptime_v2.py:16,20`; orchestration in `letta/groups/sleeptime_multi_agent_v3.py:127`). Detection therefore depends on LLM judgment, with no staleness heuristic, TTL, or decay mechanism found anywhere in the runtime.
3. **Can wrong memory be corrected?** Yes — this is the strongest area. Agents self-correct via exact-match replace with failure on missing/ambiguous matches (`letta/services/tool_executor/core_tool_executor.py:339-344,381-389`); users/API clients can edit blocks directly (`tests/test_sdk_client.py:753-785`); every edit is checkpointed and revertible via undo (`letta/services/block_manager.py:842-892`, verified in `tests/managers/test_block_manager.py:985-1016`); git-enabled agents get full commit history with author attribution (`letta/services/block_manager_git.py:533-562`).
4. **Does memory improve outcomes?** No evidence found. Nothing measures task outcomes against memory state: `RunMetrics` capture steps/tokens/tools (`letta/schemas/run_metrics.py:13-23`), OpenTelemetry spans wrap memory functions (`trace_method` decorators, e.g. `letta/schemas/memory.py:112`), but no A/B comparison, benchmark score, or success-rate metric ties memory to performance. The design bet (MemGPT lineage) is stated in prompts/docs but not measured in this repo.
5. **Is memory usage traceable?** Substantially yes. Edits record `created_by_id`/`last_updated_by_id` (`letta/schemas/block.py:73-74`), `BlockHistory` rows store `actor_type`/`actor_id` per snapshot (`letta/orm/block_history.py:37-40`), git-memory commits attribute changes to agent/user/system (`letta/schemas/memory_repo.py:28-32`), and retrieval results expose their scoring provenance (`search_mode`, rank fields — `letta/services/tool_executor/core_tool_executor.py:231-246`).

## Architectural Decisions

1. **Memory as shared relational entities with versioning, not prompt text.** Blocks are DB rows attachable to multiple agents (shared between main + sleeptime agents, asserted in `tests/integration_test_sleeptime_agent.py:107-113`), which makes correction flows transactional and auditable rather than string surgery on prompts.
2. **Division of memory labor between agents.** The main conversational agent is stripped of memory-edit tools while a background sleeptime agent owns them (`tests/integration_test_sleeptime_agent.py:83-87,115-121`) — a deliberate decision that memory maintenance should be asynchronous, reviewable work rather than inline tool calls competing with task execution.
3. **Change detection by compiled-string comparison.** Persistence happens only when the newly compiled memory string is not a substring of the current system message (`letta/services/agent_manager.py:1768`), with a dedicated noop-update test protecting the fast path (`tests/test_block_manager_noop_update.py:31-49`). Cheap, but fragile against formatting coincidences (acknowledged indirectly by `tests/test_memory.py:550-590` codifying the invariant).
4. **Dual version-control backends.** Postgres `BlockHistory` sequences for standard agents (`letta/orm/block_history.py`) and real git repositories (commit/history APIs, `letta/services/memory_repo/git_operations.py:351,540`) for git-enabled agents — durability of memory provenance is treated as infrastructure.
5. **Freshness communicated to the model, not just the logs.** Relative-time annotations on recall results (`core_tool_executor.py:186-229`), "System prompt last recompiled" metadata (`letta/prompts/prompt_generator.py:73`), and instructions to write absolute dates instead of "today/recently" (`letta/prompts/system_prompts/sleeptime_v2.py:17`) all target the *model's* ability to reason about memory age.

## Notable Patterns

- **Guardrail-by-docstring-and-code pairing:** `memory_replace`'s docstring shows bad/good examples including line-number pitfalls (`letta/functions/function_sets/base.py:331-338`), mirrored by runtime regex enforcement (`core_tool_executor.py:357-374`) — documentation and validation kept deliberately in sync.
- **Reciprocal-rank-fusion transparency:** instead of hiding hybrid-search internals, RRF/vector/FTS ranks are surfaced both in schema (`letta/schemas/message.py:2703-2710`) and inside agent-visible tool output (`core_tool_executor.py:231-246`), giving the agent (and developers) material to judge retrieval trustworthiness even though no offline metric consumes these scores today.
- **Skipped-test-as-documentation:** `test_sleeptime_removes_redundant_information` encodes the intended memory-hygiene contract (deduplication) even though it is disabled (`tests/integration_test_sleeptime_agent.py:183-238`) — intent is captured, enforcement is not.
- **Self-verification hooks in tests:** `test_compile_git_structured_recompile_after_block_edit` explicitly validates the substring-mismatch mechanism that production rebuild logic relies on (`tests/test_memory.py:550-590`), i.e., tests target the *detection machinery*, not just outputs.

## Tradeoffs

- **Generative staleness handling vs. deterministic guarantees:** relying on sleeptime prompts to purge outdated content avoids brittle heuristics but means correctness varies per model run; the only such behavioral test is skipped, so regressions would ship silently.
- **Integration-gated quality tests vs. CI speed:** memory-quality verification requires live Anthropic models and a running server (`tests/integration_test_sleeptime_agent.py:15-49,248-277`), so it cannot run as a cheap unit gate — quality assurance was traded for cost/latency.
- **Exact-match replacement safety vs. usability:** requiring verbatim unique `old_string` prevents collateral damage but raises failure rates for paraphrasing models; failures surface as tool errors the agent must retry around (`core_tool_executor.py:381-389`).
- **Substring change-detection vs. structured diffs:** comparing compiled strings is O(1)-ish and backend-agnostic, but can miss label-ordering edge cases or be fooled by nested identical sections (partially mitigated by `tests/test_context_window_calculator.py:216-264`).

## Failure Modes / Edge Cases

- **No automatic staleness alarm:** if the sleeptime agent fails, is misconfigured, or its frequency is too low, stale/wrong facts persist indefinitely with no detector raising a flag (no TTL/decay code exists; frequency is operator-set, e.g. patched to 2 in `tests/integration_test_sleeptime_agent.py:89-104`).
- **Ambiguous-replace deadlocks:** multi-occurrence `old_string` hard-fails listing offending lines (`base.py:368-373`); an agent that cannot disambiguate will repeatedly error without a fallback merge strategy.
- **Weak assertion tolerance:** `assert fact_block.value.count("Inter Miami") > 1` (`tests/integration_test_sleeptime_agent.py:290`) passes even if contradictory old facts remain — false positives in the correction pipeline are invisible.
- **Timezone-dependent freshness strings:** `time_ago` computation falls back silently on timezone errors (`core_tool_executor.py:215-220`), so displayed ages can be wrong without any signal.
- **History pruning semantics:** checkpoint after undo deletes "future" states to keep linear history (`letta/services/block_manager.py:854-857`) — redo beyond an undo point is impossible, and the corresponding regression test is commented out (`tests/managers/test_block_manager.py:1020-1048`).

## Future Considerations

- Build an offline memory eval: seed blocks/passages with known facts, script sleeptime runs against a frozen small model, and score deduplication/stale-fact removal deterministically — the skipped test (`tests/integration_test_sleeptime_agent.py:185`) is a ready-made spec.
- Consume the already-exposed `rrf_score`/rank telemetry (`letta/schemas/message.py:2703-2710`) into aggregate dashboards (hit-rate proxies) instead of letting them evaporate per-call.
- Add a deterministic staleness signal (e.g., block `updated_at` age thresholds rendered alongside `chars_current` in `_render_memory_blocks_standard`, `letta/schemas/memory.py:161-166`) so the model sees memory age structurally, not only for recall results.
- Track memory-edit outcomes in run metrics (`tools_used` already lists memory tools, `letta/schemas/run_metrics.py:21`) to correlate memory activity with task completion.
- Restore and stabilize the commented-out redo-after-undo regression test (`tests/managers/test_block_manager.py:1020-1048`).

## Questions / Gaps

- **Unanswered: does memory measurably improve task outcomes?** No benchmark, A/B harness, or success-metric correlation exists in this repo (searched `accuracy`, `benchmark`, `eval`, `locomo`, `longmemeval`, `needle` across `letta/` and `tests/`; negative). If Letta measures this, it lives outside the source tree.
- **Unanswered: what triggered skipping the dedup test?** No comment explains `@pytest.mark.skip` at `tests/integration_test_sleeptime_agent.py:183`; flakiness vs. product change is unknowable from the source alone.
- **Partial gap: recall-memory freshness for standard agents.** `time_ago` enrichment exists for `conversation_search`, but archival `ArchivalMemorySearchResult` exposes only a raw `timestamp` string (`letta/schemas/memory.py:875-884`) — no relative-age annotation for archival hits.
- **Not assessable from repo: production sleeptime efficacy.** Frequency defaults, skip rates (`memory_finish_edits` with no edits), and post-edit conflict rates are operational metrics not present here.

---

Generated by `05.08-memory-evaluation-and-freshness` against `letta`.
