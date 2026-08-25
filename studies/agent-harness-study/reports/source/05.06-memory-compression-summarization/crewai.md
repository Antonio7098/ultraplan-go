# Source Analysis: crewai

## 05.06 Memory Compression and Summarization

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based framework; LanceDB/Qdrant vector storage; SQLite long-term store) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI implements two distinct compression mechanisms. **(1) Conversation-history compaction**: when the LLM raises a context-length error mid-run, `handle_context_length` either destructively summarizes the message list in place or aborts (`lib/crewai/src/crewai/utilities/agent_utils.py:795-832`). Summaries are structured (Task Overview / Current State / Important Discoveries / Next Steps / Context to Preserve), chunk-parallelized, and spliced back as a single `<summary>` user message (`lib/crewai/src/crewai/utilities/agent_utils.py:1048-1131`). **(2) Durable memory extraction**: after each task/kickoff, the executor builds a `Task/Agent/Result` blob, asks the LLM to extract discrete self-contained memory statements (`extract_memories_from_content`, `lib/crewai/src/crewai/memory/analyze.py:155-197`), and stores them via a batch encoding pipeline with intra-batch dedup plus LLM-driven consolidation (merge/update/delete of similar existing records) (`lib/crewai/src/crewai/memory/encoding_flow.py:75-110`, `lib/crewai/src/crewai/memory/types.py:185-202`). A third minor mechanism distills human feedback into reusable lessons (`lib/crewai/src/crewai/flow/human_feedback.py:301-359`).

The memory path is well engineered: background saves with a read barrier (`drain_writes`), failure events instead of crashes, safe fallbacks on every LLM call, and broad unit + VCR integration tests. The conversation-summarization path is mechanically solid but destructive — raw history is discarded with no coverage tracking, no regeneration, and no summary-quality evaluation; the prompt layer even warns agents that injected memories "may be INCOMPLETE" (`lib/crewai/src/crewai/translations/en.json:10`).

## Rating

**7 / 10.** Clear model with tests, explicit interfaces, and operational safeguards on the memory-extraction side (fallback defaults at `lib/crewai/src/crewai/memory/analyze.py:259-264` and `:318`, save-failure events at `lib/crewai/src/crewai/memory/unified_memory.py:324-348`, write-drain barriers at `lib/crewai/src/crewai/memory/unified_memory.py:350-363` and `lib/crewai/src/crewai/crew.py:1887-1917`). Held back from 8+ because: summarization triggers are reactive-only (fires *after* a context-length exception, `lib/crewai/src/crewai/lite_agent.py:999-1011`), conversation summaries replace history irreversibly with no coverage ranges or drift detection, summaries are never evaluated for fidelity, and consolidation is similarity-triggered rather than scheduled.

## Evidence Collected

Every entry cites `sources/crewai/...` workspace-relative paths.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Summarization trigger (reactive) | Context-length exception caught in agent loop → `handle_context_length`; repeated at three more sites in the crew executor | `sources/crewai/lib/crewai/src/crewai/lite_agent.py:999-1011`; `sources/crewai/lib/crewai/src/crewai/agents/crew_agent_executor.py:444-456` (also :583, :1263, :1382) |
| Trigger policy flag | `respect_context_window: bool = True` — "Keep messages under the context window size by summarizing content." | `sources/crewai/lib/crewai/src/crewai/agent/core.py:251-254` |
| Abort path | If `respect_context_window=False`, raises `SystemExit` instead of summarizing | `sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:824-832` |
| Token budget logic | Conservative `len(text)//4` token estimate; chunks sized to `llm.get_context_window_size()`; oversized single messages split into `[Part i/n]` sub-messages | `sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:835-864`, `:955-992`, `:1080-1081` |
| Summary prompts (conversation) | `summarizer_system_message` + 5-section `summarize_instruction` (Task Overview, Current State, Important Discoveries, Next Steps, Context to Preserve); result re-injected wrapped in `<summary>` tags via `summary` slice | `sources/crewai/lib/crewai/src/crewai/translations/en.json:25-27`; consumed at `sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1030-1041` |
| Summary extraction/parsing | `_extract_summary_tags` regex pulls `<summary>...</summary>`; falls back to full text if tags missing | `sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:995-1009` |
| Parallel chunk summarization | Multi-chunk conversations summarized concurrently via asyncio; merged with `\n\n` join | `sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1012-1045`, `:1107-1121` |
| Replace vs supplement (conversation) | `messages.clear(); messages.extend(system_messages); messages.append(summary_message)` — raw non-system history destroyed; attached files preserved and re-attached | `sources/crewai/lib/crewai/src/crewai/utilities/agent_utils.py:1069-1131` |
| Extraction trigger (post-run) | Executor saves task result to memory after `AgentFinish`: builds `Task/Agent/Expected result/Result` blob → `extract_memories` → `remember_many` | `sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:31-65`; standalone-agent variant `sources/crewai/lib/crewai/src/crewai/lite_agent.py:645-660`, kickoff variant `sources/crewai/lib/crewai/src/crewai/agent/core.py:1729-1753` |
| Memory extraction prompt (coverage of decisions/facts) | `extract_memories_system`: extract decisions, facts, outcomes, preferences, lessons; preserve exact names/numbers/dates; presupposed facts; not vague restatements | `sources/crewai/lib/crewai/src/crewai/translations/en.json:70-71`; schema `ExtractedMemories` at `sources/crewai/lib/crewai/src/crewai/memory/analyze.py:94-100` |
| Extraction fallback | LLM failure returns `[content]` so the full blob persists rather than being dropped | `sources/crewai/lib/crewai/src/crewai/memory/analyze.py:191-197`; tested at `sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:624-632` |
| Save-time analysis + safe defaults | `analyze_for_save` infers scope/categories/importance/metadata; on failure returns `_SAVE_DEFAULTS` (scope `/`, importance 0.5) | `sources/crewai/lib/crewai/src/crewai/memory/analyze.py:259-315`; tested at `sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:602-621` |
| Consolidation (summary refresh) | On save, records with similarity ≥ 0.85 go to LLM consolidation plan (`keep`/`update`/`delete` + `insert_new`); conservative-bias system prompt; threshold/disable knobs `consolidation_threshold=0.85`, `consolidation_limit=5` | `sources/crewai/lib/crewai/src/crewai/memory/analyze.py:321-375`; plan schema `:103-141`; config `sources/crewai/lib/crewai/src/crewai/memory/types.py:185-202`; prompt `sources/crewai/lib/crewai/src/crewai/translations/en.json:75-76` |
| Encoding pipeline | 5-step batch flow: single embed call → intra-batch dedup (cosine ≥ 0.98 dropped) → parallel find-similar → parallel LLM analyze (4 groups A–D) → deduplicated delete/update/insert execution | `sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:75-110` (docstring), `:121-140` (dedup), `:223-347` (analyze groups), `:371-501` (execute plans) |
| Background writes & read barrier | `remember_many` submits to single-worker pool; `recall()` calls `drain_writes()` first; pool-shutdown falls back to synchronous save | `sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:297-322`, `:350-363`, `:561-579`, `:711-713` |
| Failure behavior (memory) | Save failures emit `MemorySaveFailedEvent` without crashing the run; shutdown races silently abandoned; crew drains all agent memories before kickoff-completed event | `sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:324-348`, `:641-650`; `sources/crewai/lib/crewai/src/crewai/crew.py:1887-1917`; tested `sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:1059-1094` |
| Coverage signals at recall time | `MemoryMatch.evidence_gaps` populated when deep-recall exploration reports "missing" info; confidence thresholds route deeper exploration | `sources/crewai/lib/crewai/src/crewai/memory/types.py:87-90`; `sources/crewai/lib/crewai/src/crewai/memory/recall_flow.py:52`, `:293-333`, `:373-378` |
| Incompleteness acknowledgment | Injected memory block warns it "may be INCOMPLETE" and forces multi-query search for counting/listing tasks | `sources/crewai/lib/crewai/src/crewai/translations/en.json:10`, `:65` |
| Raw history retention | Raw task outputs persisted to SQLite (`latest_kickoff_task_outputs.db`) independently of memory extraction; conversation raw history has no durable store (only optional `output_log_file`) | `sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19-64`; `sources/crewai/lib/crewai/src/crewai/crew.py:1876-1885` |
| Crew wiring | `memory=True` creates unified `Memory` with `root_scope=/crew/<name>`; per-agent saves nested under `/crew/<name>/agent/<role>` | `sources/crewai/lib/crewai/src/crewai/crew.py:652-688`; `sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:51-61` |
| Active compression tools | Agent-facing `Save to memory` / `Search memory` tools let the model decide what to compress; read-only memories omit the save tool | `sources/crewai/lib/crewai/src/crewai/tools/memory_tools.py:25-60`, `:75-101`, `:104-130` |
| HITL lesson distillation | Human feedback distilled into generalizable lessons and stored via `remember_many`; strict mode re-raises, default logs and continues | `sources/crewai/lib/crewai/src/crewai/flow/human_feedback.py:301-359`; prompt `sources/crewai/lib/crewai/src/crewai/translations/en.json:42-43` |
| Tests (summarization mechanics) | Files preserved through summarization; system messages never summarized; tool messages formatted with names; only-system no-op; chunk fits raw window incl. prompt overhead; parallel-vs-sync dispatch | `sources/crewai/lib/crewai/tests/utilities/test_agent_utils.py:339-360`, `:443-501`, `:513-535`, `:539-548`, `:730-749`, `:904-945` |
| Tests (provider integration) | VCR-cassette-backed real-LLM summarization tests across OpenAI/Anthropic/Gemini/Azure + file preservation | `sources/crewai/lib/crewai/tests/utilities/test_summarize_integration.py:86-266`; `sources/crewai/lib/crewai/tests/utilities/test_agent_utils.py:1114-1166` |
| Tests (memory pipeline) | Extract→remember wiring verified end-to-end on `Agent.kickoff`; batch dedup keeps merely-similar items; concurrent analysis; drain/failure semantics | `sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:376-433`, `:653-735`, `:763-820`, `:917-989`, `:1038-1094` |

## Answers to Dimension Questions

1. **When does summarization happen?** Two trigger classes. Conversation compaction is *reactive*: only after an LLM call raises a context-length exception, inside the agent loop retry cycle (`lib/crewai/src/crewai/lite_agent.py:999-1011`, `lib/crewai/src/crewai/agents/crew_agent_executor.py:444-456`); there is no proactive pre-flight token check that summarizes before hitting the window — the token estimate exists solely to chunk work *after* the failure. Memory extraction is *event-driven*: after every task completion (`base_agent_executor.py:31-65`), standalone-agent finish (`lite_agent.py:645-682`), and agent kickoff (`agent/core.py:1729-1753`). Human-feedback distillation runs per feedback event (`flow/human_feedback.py:301-359`). No time- or size-based periodic summarization exists.
2. **What evidence does the summary cover?** The conversation prompt explicitly demands Task Overview, Current State, Important Discoveries (facts/data/tool results), Next Steps, and verbatim Context to Preserve (values, names, URLs, code snippets) (`en.json:26`). The memory-extraction prompt targets decisions, facts, outcomes, preferences, lessons, exact numbers/names/dates, user first-person facts, and presupposed facts (`en.json:70`). Uncertainty: neither prompt captures open questions or confidence; the closest signal is recall-side `evidence_gaps` (`types.py:87-90`).
3. **Can summary drift be detected?** Only indirectly. There are no coverage ranges linking a summary to the raw span it replaced, and no staleness checks. Mitigations are heuristic: consolidation compares new content against similar stored memories and can update/delete superseded ones (`analyze.py:321-375`); recency decay de-ranks old records in composite scoring (`types.py:345-380`); and the prompt layer explicitly tells agents injected memories "may be INCOMPLETE" and to re-search before counting/listing answers (`en.json:10`, `:65`). No evidence found for automated drift detection between a produced summary and its source content.
4. **Is raw history retained?** Split. For conversations: no — `summarize_messages` clears the non-system message list and replaces it with one summary message (`agent_utils.py:1121-1131`); raw content survives only in the optional crew output log file if configured (`crew.py:1878-1885`). For task results: yes — full raw outputs are written to SQLite regardless of what the extractor kept (`kickoff_task_outputs_storage.py:66-80`), so memory statements supplement, not replace, the raw record. Attached user files survive conversation summarization by re-attachment (`agent_utils.py:1069-1072`, `:1129-1130`).
5. **Can summaries be regenerated?** Not for conversation summaries: once messages are cleared there is no retained raw corpus to re-summarize from. Memory records can be mutated toward correctness: `forget(...)` deletes by scope/category/date/metadata filter, `update(record_id, ...)` rewrites and re-embeds a record, and consolidation can merge/delete overlapping records on future saves (`unified_memory.py:818-896`; `encoding_flow.py:452-479`). Re-extraction is possible manually via `Memory.extract_memories(content)` since it is a pure, non-storing helper (`unified_memory.py:667-679`).

## Architectural Decisions

- **Two-layer compression strategy**: lossy in-context compaction for the live loop vs. durable statement-level memory across runs. They share no code paths — conversation summaries are never promoted into the memory store, and memory records are never used to rebuild conversation history beyond a top-of-prompt injection block (`lite_agent.py:606-634`).
- **Fail-open design everywhere**: every LLM-dependent compression step has a non-blocking fallback — extraction returns raw content (`analyze.py:191-197`), save-analysis returns defaults (`analyze.py:309-315`), consolidation defaults to insert (`analyze.py:369-375`), query analysis degrades to plain vector search (`analyze.py:244-256`). Compression problems never kill a run.
- **Consolidation over periodic re-summarization**: instead of scheduled summary refresh jobs, the store converges lazily — each new save checks up to `consolidation_limit=5` neighbors above similarity 0.85 and lets the LLM keep/update/delete them (`types.py:185-213`; `encoding_flow.py:155-221`).
- **Async-first writes with serialized ordering**: all memory writes funnel through a single-worker thread pool with pending-future tracking, giving cheap fire-and-forget saves plus a deterministic read barrier (`unified_memory.py:165-171`, `:297-363`).
- **Prompt externalization via i18n**: all compression prompts live in `translations/en.json`, keeping prompt iteration out of code (`en.json:25-27`, `:70-76`).

## Notable Patterns

- **Structured summary contracts**: both mechanisms force parseable output — `<summary>` tag extraction with graceful full-text fallback (`agent_utils.py:995-1009`) and pydantic response models (`ExtractedMemories`, `ConsolidationPlan`) with dual JSON/function-calling parsing paths (`analyze.py:178-190`, `:356-368`).
- **Grouped fast-path batching**: `parallel_analyze` classifies items into four groups (fields provided/not × similar found/not) to skip unnecessary LLM calls entirely — Group A costs zero calls (`encoding_flow.py:223-310`).
- **Chunk-parallel map-reduce summarization**: history is split at message boundaries, oversized messages are exploded into labeled `[Part i/n]` sub-messages, chunks are summarized concurrently, then concatenated (`agent_utils.py:867-992`, `:1107-1121`).
- **Confidence-routed deep recall**: retrieval adapts depth using high/low/complexity thresholds with a finite exploration budget, feeding extracted "missing" lines back as `evidence_gaps` (`recall_flow.py:273-343`).
- **Scope-namespaced durability**: crew memories nest under `/crew/<name>/agent/<sanitized-role>` so per-agent extractions don't collide (`crew.py:652-663`; `base_agent_executor.py:51-61`).

## Tradeoffs

- **Reactive-only conversation compaction**: waiting for a provider error means one failed call per overflow episode and reliance on string-matching the exception (`is_context_length_exceeded`, `agent_utils.py:781-792`); providers with unhelpful errors bypass summarization entirely.
- **Destructive replacement vs. auditability**: in-place `messages.clear()` maximizes freed tokens but forfeits the ability to verify, diff, or regenerate what was lost; coverage tracking was consciously traded away (no summary→source spans exist anywhere in the codebase).
- **~4 chars/token heuristic**: intentionally conservative and provider-independent (`agent_utils.py:835-844`) but can over-split CJK-light content or under-split token-dense content; the 15% window headroom absorbs the error in tests (`test_agent_utils.py:730-749`).
- **LLM-judged consolidation bias**: the "be conservative: prefer 'keep' when unsure" instruction (`en.json:75`) limits destructive merges but lets near-duplicate memories accumulate below the 0.85 threshold.
- **Extraction cost per task**: one extraction LLM call plus up to two analysis calls per item, run in background threads — latency is hidden but token spend scales linearly with task count.

## Failure Modes / Edge Cases

- **Summarizer output without tags**: falls back to treating the entire LLM response as the summary (`agent_utils.py:995-1009`) — acceptable but can leak meta-commentary into the next turn's context.
- **Extraction hallucination risk**: on LLM failure the *entire* raw blob becomes one memory (`analyze.py:191-197`) — durable but potentially huge and unfiltered; conversely a misfiring extractor can invent facts with no verification step (no evidence found for post-extraction validation).
- **Shutdown races**: background saves during process exit raise "cannot schedule new futures"; these are deliberately swallowed and the data lost (`unified_memory.py:641-650`), while ordinary background failures surface as `MemorySaveFailedEvent` without failing the producing task (`:337-348`). The crew drains pools pre-teardown precisely because late events would be orphaned (`crew.py:1896-1899`).
- **Cross-item consolidation conflicts**: two batch items targeting the same existing record are deduplicated first-wins to avoid storage commit conflicts (`encoding_flow.py:383-412`).
- **Delegation noise suppression**: outputs containing delegation actions are excluded from memory saving entirely (`base_agent_executor.py:40-41`) — avoids polluting memory with routing chatter but also drops any genuine facts embedded in delegated work.
- **Counting/aggregation hazard**: acknowledged explicitly — memory blocks warn that selection may be incomplete and instruct multi-query re-search before counts (`en.json:10`, `:65`), an operational guard against compressed-memory cardinality errors.
- **Dimension mismatch persistence**: embedding-dimension changes on reopen raise rather than corrupt, and background saves propagate the mismatch (`tests/memory/test_dimension_mismatch.py:101-166`) — protecting summary integrity at the storage layer.

## Future Considerations

- Add a proactive token-budget trigger (estimate before calling; summarize when projected usage exceeds e.g. 85% of window) to eliminate the wasted failing round-trip; the estimator and chunker already exist (`agent_utils.py:835-992`).
- Persist coverage metadata: record the source span (task ids, message range, timestamps) alongside each summary/extraction so drift detection and regeneration become possible — `KickoffTaskOutputsSQLiteStorage` already retains the raw side of this pairing (`kickoff_task_outputs_storage.py:46-58`).
- Evaluate summaries: the evaluation framework exists (`experimental/evaluation/`) but nothing scores compression fidelity; a spot-check harness comparing extracted memories against raw task outputs would close the loop (searched `src/crewai/experimental/evaluation/` and memory modules — no summary-quality metric found).
- Promote conversation summaries into the durable store (or vice versa) so the two layers share truth instead of diverging.
- Track extraction provenance: `MemoryRecord.source` supports provenance (`types.py:60-66`) but auto-extracted memories do not currently link back to the originating task output id.

## Questions / Gaps

- No evidence found for summary-quality evaluation or regression testing of compression fidelity; integration tests assert mechanics (files preserved, system messages intact, chunk sizing), not information retention (`tests/utilities/test_summarize_integration.py:86-266`).
- No evidence found for coverage-range tracking (summary ↔ source-span linkage) anywhere under `lib/crewai/src/crewai/memory/` or `utilities/`.
- Whether `respect_context_window=False` + `SystemExit` is surfaced as a recoverable condition downstream was not traced beyond the raise site (`agent_utils.py:830-832`).
- Legacy memory types (`long`/`short`/`entity`/`external`) appear only as rejected config keys (`crew.py:2294`); their historical summarization behavior could not be assessed from this tree.
- Docs under `docs/edge/en/` were not consulted for behavioral claims; all findings derive from implementation, tests, and prompt assets as required.

---

Generated by `05.06-memory-compression-and-summarization` against `crewai`.
