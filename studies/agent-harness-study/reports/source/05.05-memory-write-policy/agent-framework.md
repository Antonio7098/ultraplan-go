# Source Analysis: agent-framework

## Dimension 05.05: Memory Write Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#); multi-package monorepo (Microsoft Agent Framework) |
| Analyzed | 2026-08-25 |

## Summary

The framework does not have a single memory-write policy; it ships a **portfolio of memory providers, each with a distinct write policy**, layered on a shared `ContextProvider` lifecycle (`before_run` for recall, `after_run` for write). The most deliberate policy lives in the experimental harness `MemoryContextProvider`: after every run it persists the transcript and then invokes an LLM extractor with a fixed prompt that returns JSON "durable memory candidates", validates the output schema item-by-item, caps extraction volume (`max_extractions=5`, default at `python/packages/core/agent_framework/_harness/_memory.py:40`), merges candidates into per-topic markdown files under per-topic asyncio locks, and later consolidates topics with an LLM cleanup pass whose cadence is state-tracked so transient failures never silently slide the maintenance window. The model can also write memory explicitly via a `write_memory` tool. A second harness provider, `FileMemoryProvider`, is on by default in `create_harness_agent` (`python/packages/core/agent_framework/_harness/_agent.py:185-187`) and gives the model raw file CRUD tools over a session-scoped folder.

By contrast, the integration providers are mostly **automatic pass-through writers**: Foundry forwards every non-empty user/assistant message to a service-side memory store with a debounced fire-and-forget update (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:262-272`), mem0 posts the whole turn to Mem0's API which decides what to extract (`python/packages/mem0/agent_framework_mem0/_context_provider.py:237-273`), Cosmos writes turns and delegates cadence-based fact extraction to the Azure Agent Memory Toolkit (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:468-474`), and .NET `ChatHistoryMemoryProvider` upserts every request/response message into a vector store verbatim (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:260-308`). Notably, **no provider filters sensitive facts before storing, no user approval gate exists on any memory write tool** (all are registered `approval_mode="never_require"`), and confidence metadata exists only in the Cosmos path.

## Rating

**7 / 10** — Clear write model with tests and operational safeguards in the core harness; held back from higher by missing approval flows, absent sensitive-fact exclusion, and no confidence metadata in the primary (harness) policy.

Rationale against rubric:
- **Clear model with explicit interfaces (7-8 criteria met):** write triggers are explicit and enumerated — automatic post-turn extraction (`python/packages/core/agent_framework/_harness/_memory.py:1377-1382`), explicit model tools (`_memory.py:1223-1275`), and transcript persistence gated by constructor flags (`_memory.py:988-994`). The `MemoryStore` interface defines `write_topic`/`delete_topic` contracts (`_memory.py:598-604`).
- **Operational safeguards:** atomic writes via temp-file + `os.replace` (`_memory.py:123-136`), per-topic/per-state asyncio locks (`_memory.py:1025-1050`), owner-ID path-traversal rejection (`_memory.py:708-709`), markdown heading-escaping so stored LLM output cannot forge section markers (`_memory.py:107-111`, `457-459`).
- **Tests proving failure behavior:** atomic write preserves prior file on simulated disk-full (`python/packages/core/tests/core/test_harness_memory.py:695-737`); transient consolidation failure preserves the consolidation window (`test_harness_memory.py:799-834`); programmer errors propagate instead of being swallowed (`test_harness_memory.py:837-859`); concurrent writes to the same topic are preserved (`test_harness_memory.py:640`).
- **Why not 8+:** every memory write tool bypasses human approval (`approval_mode="never_require"` at `_memory.py:1223,1240,1266` and `python/packages/core/agent_framework/_harness/_file_memory.py:315-472`) — a stark asymmetry versus `FileAccessProvider`, which defaults its write tools to `always_require` (`python/packages/core/agent_framework/_harness/_file_access.py:1447`). No sensitive-data/PII screening exists anywhere in the write paths. Confidence scores exist only in the Cosmos provider, sourced from the external toolkit.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Extraction trigger (automatic) | `after_run` calls `_extract_memories` with the run's input+response messages | python/packages/core/agent_framework/_harness/_memory.py:1348-1396 |
| Extraction prompt (what qualifies as memorable) | Rules: durable facts/preferences/decisions only; exclude transient tasks; ≤5 items; empty list when nothing worth remembering | python/packages/core/agent_framework/_harness/_memory.py:44-56 |
| Extraction cap | `DEFAULT_MEMORY_MAX_EXTRACTIONS = 5`; enforced as `raw_items[: self.max_extractions]`; `max_extractions=0` disables extraction | python/packages/core/agent_framework/_harness/_memory.py:40,1444,1406 |
| Extractor output validation | JSON parse, fenced-block stripping, `memories` must be list, each item needs string `topic`+`memory`; invalid items logged and skipped | python/packages/core/agent_framework/_harness/_memory.py:1424-1459 |
| Explicit model write tool | `write_memory(topic, memory)` tool → `_merge_memory` → topic record append → index rebuild | python/packages/core/agent_framework/_harness/_memory.py:1223-1238 |
| Approval mode of memory tools | All six memory tools registered `approval_mode="never_require"` | python/packages/core/agent_framework/_harness/_memory.py:1203-1266 |
| FileMemory write tool | `file_memory_write` with Pydantic `_WriteFileInput` schema; rejects nested paths and reserved internal names | python/packages/core/agent_framework/_harness/_file_memory.py:315-349 |
| Harness default wiring | `create_harness_agent` appends `FileMemoryProvider` unless `disable_file_memory`; default store rooted at `{cwd}/agent-file-memory` | python/packages/core/agent_framework/_harness/_agent.py:185-187,322-323,439-443 |
| Merge/conflict handling at write time | `_merge_memory` appends new memory line to existing topic record under per-topic lock; exact-dup removal only (`_dedupe_strings` casefold) | python/packages/core/agent_framework/_harness/_memory.py:1472-1503,149-161 |
| Conflict resolution (semantic, deferred) | Consolidation prompt instructs LLM to remove duplicates/overlaps, drop stale/transient items | python/packages/core/agent_framework/_harness/_memory.py:57-68,1575-1657 |
| Consolidation cadence guard | `_should_consolidate` requires ≥`consolidation_min_sessions` sessions AND ≥24h since last run; window advances only if ≥1 topic succeeded | python/packages/core/agent_framework/_harness/_memory.py:39-41,1541-1549,1558-1573 |
| Atomic persistence | `_atomic_write_text` temp-file + `os.replace`; tested against disk-full | python/packages/core/agent_framework/_harness/_memory.py:123-136; tests/core/test_harness_memory.py:695-737 |
| Concurrency safety | Per-topic asyncio locks keyed by source/owner/slug; concurrent-writes test | python/packages/core/agent_framework/_harness/_memory.py:1025-1050; tests/core/test_harness_memory.py:640-663 |
| Transcript pre-save filter hook | `history_message_filter` callback can rewrite or drop messages before transcript save | python/packages/core/agent_framework/_harness/_memory.py:982,1153-1162 |
| Injection-surface hardening | Stored summary/memory lines markdown-escaped so persisted LLM output cannot re-interpret as headings on read-back | python/packages/core/agent_framework/_harness/_memory.py:107-111,450-475; test at tests/core/test_harness_memory.py:675-693 |
| Foundry automatic write | `after_run` filters roles {user, assistant} + non-empty text, sends `begin_update_memories` fire-and-forget with `update_delay` debounce (default 300s) | python/packages/foundry/agent_framework_foundry/_memory_provider.py:81,233-276 |
| .NET Foundry write | `StoreAIContextAsync` concatenates request+response messages into one `UpdateMemoriesAsync` call; errors swallowed to log | dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:183-246 |
| mem0 automatic write | Whole turn (user/assistant/system, non-empty text) posted via `mem0_client.add(...)` — Mem0 platform owns extraction policy | python/packages/mem0/agent_framework_mem0/_context_provider.py:237-273 |
| Cosmos turn writes + delegated extraction | Turn writes skip empty/whitespace content; background extraction driven by toolkit cadence thresholds; `auto_extract=False` zeroes thresholds | python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:416-477,100,130-134,169-175 |
| Cosmos confidence metadata | `min_confidence` retrieval threshold (default 0.7); memories formatted `[type] content (confidence: X.XX)` incl. zero-confidence edge | python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:97,479-506; tests/test_context_provider.py:614-652 |
| Read-path injection defense | Cosmos user-summary injected as untrusted user-role message, not instructions, to mitigate poisoned-memory prompt injection | python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:382-412 |
| .NET raw-history writer | `ChatHistoryMemoryProvider.StoreAIContextAsync` upserts every request/response message with scope fields; no durability filter | dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:260-308 |
| Deletion/correction surface | `delete_memory_topic` tool; instructions tell the model to read-then-correct topic files | python/packages/core/agent_framework/_harness/_memory.py:1240-1252,1306 |
| Memory update observability | Only telemetry feature marks (`mark_feature_used(CORE_MEMORY_PROVIDER)`); no per-write events emitted | python/packages/core/agent_framework/_harness/_memory.py:1176 |

## Answers to Dimension Questions

1. **What causes memory to be written?**
   Four distinct triggers across the portfolio: (a) *post-turn LLM extraction* — every completed run feeds input+response messages to an extractor prompt, and returned durable-fact candidates are merged into topic files (`python/packages/core/agent_framework/_harness/_memory.py:1398-1470`); (b) *model-invoked tools* — `write_memory` / `file_memory_write` during a run (`_memory.py:1223-1238`; `python/packages/core/agent_framework/_harness/_file_memory.py:315`); (c) *unconditional transcript persistence* — `store_inputs`/`store_outputs` default true, archiving raw turns regardless of durability (`_memory.py:988-994,1359-1366`); (d) *provider pass-through* — Foundry/mem0/Cosmos/.NET ChatHistory forward entire turns to their backing services after each run (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:246-272`; `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:290-293`).

2. **Can the model write arbitrary memory?**
   Yes. The `write_memory` tool takes arbitrary topic/text strings and all memory-mutation tools are registered `approval_mode="never_require"` (`python/packages/core/agent_framework/_harness/_memory.py:1223,1240`; `python/packages/core/agent_framework/_harness/_file_memory.py:315-472`), so nothing routes them through the host approval flow (`ToolApprovalMiddleware` exists but is opt-in). The only constraints are structural: normalization/non-empty checks (`_memory.py:93-104`), flat-name and internal-name rejection in FileMemory (`_file_memory.py:111-122,322-331`), and index caps. Contrast: `FileAccessProvider` write tools default to `always_require` (`python/packages/core/agent_framework/_harness/_file_access.py:1447`), showing the framework has the mechanism but chose not to apply it to memory.

3. **Are facts verified?**
   Not semantically. Validation is structural only: extractor output must parse as JSON with the expected shape, items missing `topic`/`memory` strings are skipped, and counts are capped (`_memory.py:1424-1459`). There is no source-grounding check, contradiction check against existing memories, or provenance recording beyond `session_ids`. Durability judgment ("include only durable facts... return {"memories": []} when nothing should be remembered") is delegated entirely to the extractor prompt (`_memory.py:49-55`). Semantic cleanup happens only later, probabilistically, via consolidation (`_memory.py:1575-1657`).

4. **Can users correct memory?**
   Indirectly. There is no user-facing correction API or approval prompt. Correction paths are: (a) instructing the agent, which the injected instructions encourage ("Use read_memory_topic to inspect or correct a specific topic file before editing it", `_memory.py:1306`) to fix via `write_memory`/`file_memory_replace*`; (b) deleting topics via `delete_memory_topic` (`_memory.py:1240-1252`); (c) editing the plain markdown files by hand — feasible because the format is documented and round-trip tested (`tests/core/test_harness_memory.py:114-132`). Hosts can also drop messages pre-persistence via `history_message_filter` (`_memory.py:982`).

5. **Are sensitive facts excluded?**
   No evidence found. Searched all memory providers for sensitive/redact/scrub/PII logic in write paths — none exists; the extraction prompt rules (`_memory.py:49-55`), the Foundry role filter (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:253`), the mem0 role/content filter (`python/packages/mem0/agent_framework_mem0/_context_provider.py:256-260`), and the .NET ChatHistory upsert (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:272-288`) contain no content screening. The closest mitigation is read-path only: Cosmos injects the derived user summary framed as untrusted reference data to prevent poisoned memories becoming persistent instructions (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:389-412`).

## Architectural Decisions

- **Policy split between "durable memory" and "raw history".** The harness keeps two stores: curated topic files (written only through extraction/tool policies, `_memory.py:1472-1503`) and an unfiltered transcript archive (`save_messages`, `_memory.py:1130-1165`). This avoids forcing lossy curation onto conversation continuity.
- **LLM-as-extractor with fixed, overridable prompts.** Both extraction and consolidation prompts are constructor arguments (`extraction_prompt`, `consolidation_prompt`, `_memory.py:1018-1019`), so hosts can tighten the write policy without subclassing.
- **Consolidation as deferred conflict resolution with a crash-safe window.** Maintenance state tracks `last_consolidated_at` + queued session ids; the window advances only when at least one topic consolidated, otherwise the next `after_run` retries (`_memory.py:1541-1549`), verified by `test_memory_consolidation_transient_failure_preserves_state` (`tests/core/test_harness_memory.py:799-834`).
- **Separation of extraction model vs consolidation model.** A dedicated `consolidation_client` lets hosts run cleanup on a cheaper model (`_memory.py:957`, used at `_memory.py:1270`).
- **Service-side write delegation in integrations.** Foundry, mem0, and Cosmos deliberately push the "what is worth remembering" decision to the backing service/toolkit, keeping the framework's role to scoping and transport (e.g., mem0 storage/retrieval scope separation, `python/packages/mem0/agent_framework_mem0/_context_provider.py:50-62`).
- **File memory as the always-on default in the harness.** `create_harness_agent` wires `FileMemoryProvider` by default (`python/packages/core/agent_framework/_harness/_agent.py:185-187`), making tool-driven writes part of the out-of-box agent loop rather than an add-on.

## Notable Patterns

- **Write amplification control:** caps at three levels — ≤5 extracted items per turn (`max_extractions`, `_memory.py:40,1444`), ≤200 index pointer lines (`index_line_limit`, `_memory.py:36`), ≤50 entries in the FileMemory index (`_MAX_INDEX_ENTRIES`, `python/packages/core/agent_framework/_harness/_file_memory.py:78`).
- **Idempotent, diff-aware index writes:** `MEMORY.md` is rewritten only when content differs (`_memory.py:834-835`), tested by `test_memory_context_provider_does_not_rewrite_unchanged_index` (`tests/core/test_harness_memory.py:292`).
- **Read paths never mutate disk:** pure-read calls skip directory creation (`_memory.py:1126-1128`; test at `tests/core/test_harness_memory.py:740-754`).
- **Lock granularity:** per-topic locks let independent topics merge concurrently while serializing same-topic writes; state locks are released before multi-second LLM calls to avoid blocking concurrent runs (`_memory.py:1513-1515` comment, `1516-1526`).
- **Failure-mode triage in extraction/consolidation:** transient errors (`ChatClientException`, timeout, OSError) are swallowed with a warning; programmer errors (AttributeError etc.) propagate loudly (`_memory.py:86-90,1420-1422`; tests `tests/core/test_harness_memory.py:799-859`).
- **Untrusted-channel injection of derived content:** both harness memory context and Cosmos summaries are injected as `user`-role messages, never system instructions (`_memory.py:1332-1344`; cosmos `_context_provider.py:398-412`).

## Tradeoffs

- **Automatic extraction runs on the main agent client** by default, adding a latency/cost tail to every `after_run` when a client is available; opting out requires setting `max_extractions=0` (`_memory.py:1406`) — there is no separate extraction-client override (unlike consolidation).
- **Append-first merging favors durability over accuracy:** `_merge_memory` appends without checking the new fact against existing bullets (`_memory.py:1494-1501`), so a corrected preference coexists with the stale one until consolidation eventually dedupes — and consolidation only runs after `consolidation_min_sessions` sessions AND 24h (`_memory.py:39-41,1558-1573`).
- **Prompt-policy dependence:** whether anything is written at all hinges on the extractor prompt's notion of "durable"; hosts overriding prompts inherit full responsibility for quality (`_memory.py:44-56,1018`).
- **Pass-through providers trade policy for simplicity:** Foundry/mem0/ChatHistory store everything non-empty, shifting trust to service-side extraction (Foundry) or third-party pipelines (mem0); errors are swallowed to logs, making silent write loss possible (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:274-276`; `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:235-245`).
- **No-approval-by-default maximizes autonomy, minimizes oversight:** memory writes never pause for HITL even though the framework's approval infrastructure supports it.

## Failure Modes / Edge Cases

- **Disk-full mid-write:** prior topic/index file survives via atomic replace; leftover temp files cleaned (`_memory.py:123-136`; test `tests/core/test_harness_memory.py:695-737`).
- **Malformed extractor output:** non-JSON, non-object payloads, missing fields, or non-string values are skipped per-item with warnings; extraction returns partial results (`_memory.py:1427-1459`).
- **Transient extractor/consolidator outage:** extraction skipped for the turn (transcript still archived, so content isn't lost); consolidation window preserved for retry (`_memory.py:1420-1422,1541-1549`).
- **Concurrent same-topic writers:** serialized by per-topic asyncio locks; test proves both writes survive (`tests/core/test_harness_memory.py:640-663`).
- **LLM-supplied markdown injection:** heading-like lines in stored summaries/memories are escaped on write and unescaped on read, preventing forged sections (`_memory.py:107-120,457-461`; test `:675-693`).
- **Path escape via owner id or topic slug:** absolute/traversal owner IDs rejected (`_memory.py:708-709`); slugs forced through `[a-z0-9-]` (`_memory.py:139-141`).
- **Corrupt FileMemory index:** read failure logs a warning and skips index injection for the run; self-heals on next write (`python/packages/core/agent_framework/_harness/_file_memory.py:501-508`).
- **Lost background extraction at shutdown (Cosmos):** toolkit cancels pending tasks on close; provider drains via `flush()` in `__aexit__` (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:262-280,313-332`).
- **Junk turns:** empty/whitespace-only messages are excluded from Cosmos turn writes (`_context_provider.py:443-466`) and from mem0 items (`python/packages/mem0/agent_framework_mem0/_context_provider.py:256-260`).

## Future Considerations

- Add an optional approval gate for memory mutations (mirroring `FileAccessProvider`'s `always_require` default and auto-approval-rule pattern at `python/packages/core/agent_framework/_harness/_file_access.py:1447`) so hosts can require sign-off on `write_memory`/`delete_memory_topic`.
- Introduce confidence/provenance metadata on harness memories (source session, extraction timestamp already exist as `session_ids`/`updated_at`; a score field would align with the Cosmos provider's model at `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:97`).
- Screen write candidates for sensitive content (secrets/PII heuristics) inside `_extract_memories`/`_merge_memory` before persistence.
- Emit observable memory-update events beyond feature-usage telemetry (currently only `mark_feature_used`, `_memory.py:1176`) to support auditing who wrote what.
- Allow an extraction-client override analogous to `consolidation_client` to keep extraction cost off the primary model.

## Questions / Gaps

- **No user-approval flow specific to memory could be found.** Searched `_tools.tool` registration sites in all harness providers and the `ToolApprovalMiddleware` docs; memory tools are uniformly `never_require`. Whether this is intentional autonomy-by-default or a gap is undocumented in-repo.
- **No evidence of conflict detection at write time** beyond exact-duplicate string dedup (`_memory.py:149-161`); semantic contradiction handling is assumed to be consolidation's job but there is no guarantee a contradictory pair is ever seen together by the consolidator (per-topic scope only).
- **Sensitive-fact exclusion: no evidence found** in any provider's write path (searched `sensitive|redact|scrub|PII` across `python/packages`); only read-path untrusted framing exists (Cosmos).
- **Confidence metadata is external-only:** the core `MemoryTopicRecord` schema (`_memory.py:349-358`) has no confidence field; only the Cosmos toolkit surfaces scores. How the toolkit computes confidence is outside this source's boundary (external `azure-cosmos-agent-memory` dependency).
- **.NET parity for extraction-style curation was not found:** the .NET side ships `FileMemoryProvider` (tool-driven writes, `dotnet/src/Microsoft.Agents.AI/Harness/FileMemory/FileMemoryProvider.cs:423-437`) and pass-through providers, but no LLM-extraction pipeline equivalent to the Python `MemoryContextProvider` was located; searched `dotnet/src/**/Memory/**` and harness directories.

---

Generated by `05.05-memory-write-policy` against `agent-framework`.
