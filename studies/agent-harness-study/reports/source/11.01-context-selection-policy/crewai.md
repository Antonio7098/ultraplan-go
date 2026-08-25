# Source Analysis: crewai

## Dimension 11.01: Context Selection Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic-based framework; LiteLLM-backed multi-provider LLM layer; LanceDB/Qdrant vector storage) |
| Analyzed | 2026-08-25 |

> Citation convention: paths below are relative to the source root `studies/agent-harness-study/sources/crewai/`.

## Summary

CrewAI assembles model context through a layered, mostly imperative pipeline rather than a single declarative policy object. For Crew task execution the pipeline is: static role/task prompt slices from an i18n template table (`lib/crewai/src/crewai/utilities/prompts.py:93-141`) → output-schema instructions (`lib/crewai/src/crewai/agent/utils.py:60-85`) → inter-task context aggregation of prior task raw outputs (`lib/crewai/src/crewai/crew.py:1866-1874`, `lib/crewai/src/crewai/utilities/formatter.py:16-26`) → memory recall appended to the prompt (`lib/crewai/src/crewai/agent/core.py:619-682`) → LLM-rewritten knowledge/RAG retrieval appended as "Additional Information" (`lib/crewai/src/crewai/agent/utils.py:119-198`, `lib/crewai/src/crewai/knowledge/utils/knowledge_utils.py:4-12`) → final system/user messages with prompt-cache breakpoints (`lib/crewai/src/crewai/agents/crew_agent_executor.py:170-206`). Selection is therefore hybrid: the scaffolding is static per-agent, while memory, knowledge, files, date, and skill catalogs are injected dynamically per execution.

The system is notable in three ways. First, retrieval into context is scored and observable: memory matches carry composite scores and match reasons (`semantic`, `recency`, `importance`) that are rendered into the prompt itself (`lib/crewai/src/crewai/memory/types.py:92-106`, `lib/crewai/src/crewai/memory/types.py:374-378`), directly answering the dimension's "can the system explain why a document was included" question for memories. Second, the model can actively influence what it sees: memory is surfaced as `Search memory` / `Save to memory` tools (`lib/crewai/src/crewai/tools/memory_tools.py:25-130`), skills use progressive disclosure where only a metadata catalog enters the prompt and the model loads full instructions on demand via a loader tool (`lib/crewai/src/crewai/utilities/prompts.py:165-209`), and the automatic memory slice explicitly instructs the model not to trust it for counting tasks and to re-query (`translations/en.json`, `slices.memory`). Third, overflow handling is reactive compaction: when a context-length error is detected, history is chunked, summarized by the LLM with a structured five-section summary contract, and replaced in place while preserving system messages and attached files (`lib/crewai/src/crewai/utilities/agent_utils.py:795-832` and `1048-1131`).

Weaknesses are concrete: there is no PII/sensitive-data redaction anywhere in the context path (the security module covers only fingerprinting, `lib/crewai/src/crewai/security/security_config.py:20-87`; the only "redaction" found is Anthropic's protocol-level `redacted_thinking` blocks, `lib/crewai/src/crewai/llms/providers/anthropic/completion.py:678-690`); inter-task context concatenation is unbounded (`formatter.py:26`); token budgeting uses a crude `len(text) // 4` estimate (`agent_utils.py:835-844`); and `respect_context_window` defaults to `False`, meaning default behavior on overflow is `SystemExit`, not graceful degradation (`lib/crewai/src/crewai/agents/crew_agent_executor.py:126`, `agent_utils.py:824-832`).

## Rating

**7 / 10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:
- **Explicit, configurable selection signals**: memory relevance is a weighted, user-tunable composite score with documented fields (`semantic_weight=0.5`, `recency_weight=0.3`, `importance_weight=0.2`, half-life, oversampling, confidence thresholds) at `lib/crewai/src/crewai/memory/types.py:135-286`; knowledge retrieval has explicit `results_limit=5, score_threshold=0.6` defaults (`lib/crewai/src/crewai/knowledge/knowledge.py:135-152`).
- **Tested**: 99 tests in `lib/crewai/tests/utilities/test_agent_utils.py` cover summarization helpers (file preservation, role-label formatting, tag extraction, message-boundary chunking); integration tests for compaction across OpenAI/Anthropic/Gemini/Azure exist in `lib/crewai/tests/utilities/test_summarize_integration.py:89-258`; recovery from context errors is unit-tested at `lib/crewai/tests/agents/test_agent_executor.py:985-994`; memory recall/scoring tests at `lib/crewai/tests/memory/test_unified_memory.py`.
- **Observable**: knowledge and memory retrieval emit started/completed/failed events with retrieved content and timing (`lib/crewai/src/crewai/agent/utils.py:146-197`, `lib/crewai/src/crewai/agent/core.py:632-669`).
- **Not higher (8+)** because: no sensitive-data filtering before inclusion; no single policy abstraction (selection logic is scattered across `crew.py`, `agent/core.py`, `agent/utils.py`, executors); unbounded cross-task context strings; heuristic token estimation; lossy one-shot summarization; and the dangerous default of exiting on context overflow.

## Evidence Collected

Every entry cites `path:line` relative to `studies/agent-harness-study/sources/crewai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Prompt assembly (static scaffolding) | `Prompts.task_execution()` composes i18n slices `role_playing` + `tools`/`no_tools` + task slice; appends skill block and date block | lib/crewai/src/crewai/utilities/prompts.py:93-141 |
| Variable substitution | `{goal}`, `{role}`, `{backstory}` substituted from agent fields into every prompt slice | lib/crewai/src/crewai/utilities/prompts.py:253-257 |
| Date injection policy | Opt-in via `agent.inject_date`; format validated against allowlist `VALID_DATE_FORMAT_CODES`; placed at prompt tail as cache anchor | lib/crewai/src/crewai/utilities/prompts.py:13-24, 143-163 |
| Skill progressive disclosure | Metadata-only `<available_skills>` catalog always in prompt; full SKILL.md body only loaded when model calls the loader tool | lib/crewai/src/crewai/utilities/prompts.py:165-209 |
| Skill disclosure levels | `METADATA` vs `INSTRUCTIONS` disclosure constants; `disclosure_level` field defaults to METADATA | lib/crewai/src/crewai/skills/models.py:29-34, 117-118 |
| Inter-task context | `Crew._get_context` joins prior task outputs with `\n\n----------\n\n` dividers; no truncation or scoring | lib/crewai/src/crewai/crew.py:1866-1874; lib/crewai/src/crewai/utilities/formatter.py:13-45 |
| Context formatting template | `task_with_context` slice renders "{task}\n\nThis is the context you're working with:\n{context}" | translations/en.json (`slices.task_with_context`); applied at lib/crewai/src/crewai/agent/utils.py:88-104 |
| Memory auto-recall | `_retrieve_memory_context` calls `unified_memory.recall(task.description, limit=5)` and appends via i18n `memory` slice | lib/crewai/src/crewai/agent/core.py:619-682 |
| Full pipeline order | `execute_task`: prepare (schema→context→memory) → knowledge retrieval → finalize (training data) → execute | lib/crewai/src/crewai/agent/core.py:540-566, 816-851 |
| Knowledge query rewriting | `_get_knowledge_search_query` asks the LLM to rewrite the task prompt into a retrieval-optimized query before search | lib/crewai/src/crewai/agent/core.py:1364-1417 |
| Knowledge retrieval params | `query(..., results_limit=5, score_threshold=0.6)` against RAG storage | lib/crewai/src/crewai/knowledge/knowledge.py:135-152 |
| Knowledge context rendering | Snippets joined and wrapped as "Additional Information: {snippet}"; empty result yields empty string (no injection) | lib/crewai/src/crewai/knowledge/utils/knowledge_utils.py:4-12 |
| Memory composite scoring | score = semantic·w + recency·decay + importance·w; match reasons recorded | lib/crewai/src/crewai/memory/types.py:345-380 |
| Scored match rendering | `MemoryMatch.format()` emits `- (score=0.87) content` plus categories/metadata into the prompt | lib/crewai/src/crewai/memory/types.py:92-106 |
| Recall configuration surface | `MemoryConfig`: weights, half-life, confidence thresholds, exploration budget, oversample factor, query-analysis char threshold | lib/crewai/src/crewai/memory/types.py:135-286 |
| Adaptive recall flow | `RecallFlow`: LLM distills sub-queries, parallel multi-scope search, confidence-based iterative deepening, evidence-gap tracking | lib/crewai/src/crewai/memory/recall_flow.py:1-68 |
| Privacy filter in recall | `private` records excluded unless caller supplies matching `source` or passes `include_private=True` | lib/crewai/src/crewai/memory/unified_memory.py:746-751; flag defined at lib/crewai/src/crewai/memory/types.py:67-73 |
| Model-initiated context (memory tools) | `Search memory` tool recalls with limit=20 and dedups; `Save to memory` writes; read-only memory omits save tool | lib/crewai/src/crewai/tools/memory_tools.py:25-130 |
| Memory tool registration | Added per-task when agent/crew memory resolves non-None | lib/crewai/src/crewai/crew.py:1681-1683, 1790-1802 |
| Anti-hallucination instruction | Auto-injected memory block warns it "may be INCOMPLETE" and mandates using Search memory before counting/listing tasks | translations/en.json (`slices.memory`) |
| Message construction w/ caching | Executor builds system+user messages and marks cache breakpoints on stable prefixes (per-agent and per-task anchors) | lib/crewai/src/crewai/agents/crew_agent_executor.py:170-206; marker impl at lib/crewai/src/crewai/llms/cache.py:27-32 |
| Multimodal file injection | Crew/task store files merged with input files (inputs win), attached to last user message; unsupported types become ReadFileTool instead | lib/crewai/src/crewai/agents/crew_agent_executor.py:249-276; lib/crewai/src/crewai/crew.py:1685-1710 |
| Overflow detection & policy | `is_context_length_exceeded` string-matches provider errors; `handle_context_length` summarizes if `respect_context_window` else raises `SystemExit` | lib/crewai/src/crewai/utilities/agent_utils.py:781-792, 795-832 |
| Summarization compaction | Preserves system msgs and user files; chunks by estimated tokens; structured `<summary>` contract (Task Overview/Current State/Discoveries/Next Steps/Context to Preserve); parallel chunk summarization | lib/crewai/src/crewai/utilities/agent_utils.py:900-1131 |
| Token estimation heuristic | `_estimate_token_count = len(text) // 4` ("roughly 1 token per 4 characters") | lib/crewai/src/crewai/utilities/agent_utils.py:835-844 |
| Context window sizing | 85% usage ratio over known window sizes (default base 8192); clamped 1024–2097152 | lib/crewai/src/crewai/llm.py:325-326, 2450-2476 |
| Overflow default off | `respect_context_window: bool = Field(default=False)` on executor and agent | lib/crewai/src/crewai/agents/crew_agent_executor.py:126; lib/crewai/src/crewai/experimental/agent_executor.py:206; lib/crewai/src/crewai/agent/core.py:251 |
| In-flow recovery (experimental executor) | `recover_from_context_length` invokes summarizer and returns to loop start; tested | lib/crewai/src/crewai/experimental/agent_executor.py:2787-2800; test at lib/crewai/tests/agents/test_agent_executor.py:985-994 |
| Guardrails gate downstream context | Task guardrail failure retries execution with validation-error text as the next task's `context`; success may rewrite `task_output.raw` | lib/crewai/src/crewai/task.py:1321-1439 |
| Observability events | `KnowledgeRetrievalStarted/Completed/Failed`, `MemoryRetrievalStarted/Completed/Failed` events carry query, content, latency | lib/crewai/src/crewai/agent/utils.py:146-197; lib/crewai/src/crewai/agent/core.py:632-680 |
| Sensitive-data redaction | No PII/masking logic in context path; `SecurityConfig` handles fingerprints only; telemetry doc claims no prompt content leaves process | lib/crewai/src/crewai/security/security_config.py:20-87; lib/crewai/src/crewai/telemetry/telemetry.py:1-4 |
| Flow-path parity | `LiteAgent._inject_memory_context` recalls limit=10 into system message and registers memory tools for Flow agents | lib/crewai/src/crewai/lite_agent.py:521-532, 599-620 |

## Answers to Dimension Questions

**1. What decides what goes into context?**
A fixed composition order decided in code, not config: (a) static persona/tool slices chosen by capability flags (`has_tools`, `use_native_tool_calling`) at `lib/crewai/src/crewai/utilities/prompts.py:99-131`; (b) output-format schema instructions when `output_json`/`output_pydantic` set (`lib/crewai/src/crewai/agent/utils.py:72-85`); (c) upstream task outputs when the task participates in crew context (`lib/crewai/src/crewai/crew.py:1866-1874`); (d) top-5 recalled memories when any memory is available (`lib/crewai/src/crewai/agent/core.py:629-657`); (e) top-5 knowledge snippets above 0.6 similarity when `agent.knowledge` or `crew.knowledge` exists (`lib/crewai/src/crewai/agent/utils.py:143-177`); (f) attached files and an opt-in date line. Conversation history accumulates in the executor's message list during the ReAct/native-tool loops (`lib/crewai/src/crewai/agents/crew_agent_executor.py:330-484`).

**2. Is selection policy explicit or implicit?**
Mixed. The *scoring* policy for memory is explicit and tunable (`MemoryConfig`, `lib/crewai/src/crewai/memory/types.py:135-286`), and knowledge thresholds are explicit parameters (`lib/crewai/src/crewai/knowledge/knowledge.py:136`). But the *inclusion* policy — which sources enter the prompt at all, in what order, with what budgets — is implicit, hard-coded across `execute_task`/`_prepare_task_execution` (`lib/crewai/src/crewai/agent/core.py:540-566`) and duplicated between sync/async variants and three executors (`crew_agent_executor.py`, `experimental/agent_executor.py:310-335`, `lite_agent.py:548-551`). There is no declarative context-policy object.

**3. Can the model influence what it sees?**
Yes, through three mechanisms: (i) `Search memory` tool for active recall (limit=20 per query, dedup by record id, `lib/crewai/src/crewai/tools/memory_tools.py:33-60`); (ii) skill loader tool for on-demand loading of full instructions, with the catalog deliberately kept as a stable cache anchor (`lib/crewai/src/crewai/utilities/prompts.py:165-209`); (iii) the memory slice's instruction to re-query rather than trust the automatic selection (`translations/en.json`, `slices.memory`). Indirectly, tool results and delegation outputs also enter history as ordinary messages.

**4. Are sensitive fields redacted?**
No. A repo-wide search for redact/mask/PII logic found nothing in the context path; the only hit is Anthropic's protocol-level `redacted_thinking` block passthrough (`lib/crewai/src/crewai/llms/providers/anthropic/completion.py:678-690`), which is not data filtering. The closest safeguards are: the opt-in per-record `private` flag enforced during memory recall (`lib/crewai/src/crewai/memory/unified_memory.py:746-751`), embedding exclusion from serialization (`lib/crewai/src/crewai/memory/types.py:54-59`), AWS credential names blocked when mapping env vars to LLM params (`lib/crewai/src/crewai/utilities/llm_utils.py:90-94, 165-181`), and the telemetry module's stated no-prompt-content policy (`lib/crewai/src/crewai/telemetry/telemetry.py:1-4`). Anything placed in inputs, task outputs, or knowledge sources flows into prompts verbatim.

## Architectural Decisions

1. **Prompt-as-template-table**: all scaffolding text lives in `translations/en.json` slices consumed via `I18N_DEFAULT.slice(...)`, keeping wording out of control flow but making inclusion decisions invisible to grep-by-template (`lib/crewai/src/crewai/utilities/prompts.py:230-234`).
2. **Append-only enrichment of the task string**: each stage returns an augmented `task_prompt` string (schema → context → memory → knowledge → training data), producing a single flat user payload rather than structured per-source blocks (`lib/crewai/src/crewai/agent/core.py:563-566, 840-851`).
3. **Reactive, exception-driven compaction**: context-window management triggers on provider errors detected by substring matching (`LLMContextLengthExceededError._is_context_limit_error`, `lib/crewai/src/crewai/utilities/agent_utils.py:781-792`) rather than proactive pre-flight token accounting.
4. **Dual-mode memory access**: passive auto-recall (limit=5) plus active tools (limit=20), so selection responsibility is split between system and model.
5. **Cache-aware ordering**: volatile blocks (date, per-task payload) are pushed to the tail and breakpoint markers mark stable prefixes so provider adapters can attach cache directives (`lib/crewai/src/crewai/llms/cache.py:1-32`, `prompts.py:144-148`).
6. **Progressive disclosure for skills** mirrors the same philosophy used by coding agents: metadata in-context, bodies behind a tool call.

## Notable Patterns

- **Explainable inclusion via scores in-prompt**: memory lines embed `(score=...)` and categories directly in the text the model sees (`lib/crewai/src/crewai/memory/types.py:99-105`) — an unusual, transparent choice; `match_reasons` and `evidence_gaps` are carried on `MemoryMatch` (`types.py:83-90`).
- **Oversample-then-trim retrieval**: fetch 2× candidates, then apply composite scoring/dedup/category filters before trimming to `limit` (`lib/crewai/src/crewai/memory/types.py:12-17`, `unified_memory.py:739-762`).
- **Confidence-routed deep recall**: below confidence thresholds and with remaining budget, recall runs another LLM-guided exploration round (`lib/crewai/src/crewai/memory/types.py:226-265`, `recall_flow.py:58-68`).
- **Structured summarization contract**: the summarizer must produce five named sections inside `<summary>` tags; extraction falls back to full text if tags are missing (`lib/crewai/src/crewai/utilities/agent_utils.py:995-1009`, `translations/en.json` `slices.summarize_instruction`).
- **File fidelity under compaction**: user-message file attachments are collected before summarization and re-attached to the replacement summary message (`lib/crewai/src/crewai/utilities/agent_utils.py:1069-1072, 1129-1131`; tests at `lib/crewai/tests/utilities/test_agent_utils.py:338-441`).
- **Guardrail retry feeds error back as context**: failed validation becomes the next attempt's `context` string (`lib/crewai/src/crewai/task.py:1394-1408`).

## Tradeoffs

- **Simplicity vs safety**: appending everything to one flat prompt string keeps plumbing trivial but forfeits per-source budgets, deduplication, and provenance labels in the final prompt (knowledge snippets arrive unlabeled beyond "Additional Information", unlike memory scores).
- **Reactive vs proactive compaction**: waiting for a provider 4xx avoids double-accounting tokens but means one wasted round-trip per overflow and reliance on fragile error-string matching across providers.
- **Model agency vs determinism**: memory/skill tools make context partially model-controlled, improving precision but making effective context non-reproducible run-to-run.
- **Exit-by-default on overflow**: `respect_context_window=False` prevents silent lossy summarization of long histories but turns an operational hiccup into process death (`SystemExit`, `agent_utils.py:830-832`) — a deliberate footgun for production crews.
- **Static defaults over adaptive budgets**: fixed limits (memory 5/auto and 20/tool, knowledge 5 @ 0.6) are predictable but ignore task complexity and window size.

## Failure Modes / Edge Cases

- **Unbounded inter-task context**: `aggregate_raw_outputs_from_task_outputs` concatenates *all* prior task raws with no cap (`formatter.py:26`); a chain of verbose tasks can push the very first agent call over the window, triggering the SystemExit path.
- **Token misestimation**: `len//4` underestimates CJK/code-heavy content and can let oversized chunks reach the provider, re-triggering overflow mid-compaction (`agent_utils.py:835-844`).
- **Lossy single-pass summary**: everything outside `system` roles collapses to one merged summary; numeric/tabular details survive only if the summarizer LLM preserves them despite the "preserve all information" instruction (`agent_utils.py:1121-1131`).
- **Knowledge query rewriting depends on main LLM**: if the agent's LLM isn't a `BaseLLM`, retrieval silently degrades to no-knowledge (`None` return, warning log, `lib/crewai/src/crewai/agent/core.py:1378-1391`); retrieval exceptions are swallowed and emitted as events, leaving the prompt unaugmented (`agent/utils.py:188-197`).
- **Memory recall reads stale state without drain on the tool path**: `recall()` drains pending background writes first (`unified_memory.py:711-713`), but the auto-recall path's effectiveness still depends on write completion timing across concurrent tasks.
- **Deprecated executor drift**: `CrewAgentExecutor` warns it is deprecated in favor of `crewai.experimental.AgentExecutor` (`crew_agent_executor.py:143-151`); context-selection logic now exists in two places that can diverge (e.g., experimental executor stores plan/todos in state rather than mutating `task.description`, `experimental/agent_executor.py:382-385`).
- **Private-memory leak valve**: `include_private=True` disables the privacy filter entirely for callers who pass it (`unified_memory.py:706`); nothing in the auto-recall path documents whether user code can flip this.

## Future Considerations

- Introduce a declarative `ContextPolicy` (per-source budgets, priority order, redaction hooks) consulted by a single assembler, replacing the distributed append-chain in `agent/core.py:540-566` / `agent/utils.py`.
- Add proactive pre-flight token accounting (real tokenizer per provider) with tiered eviction: truncate oldest tool results → compress tool results → summarize, instead of one-shot whole-history summarization.
- Extend the memory-match pattern (score + reasons) to knowledge snippets so every retrieved block in a prompt carries provenance; persist these annotations into event payloads for audit replay.
- Wire sensitive-data scanning (secrets patterns, PII detectors) as a mandatory pre-inclusion filter for inputs/files/knowledge, complementing the existing per-record `private` flag.
- Bound inter-task context (e.g., map-summarize upstream outputs above N chars) and make `respect_context_window=True` safe enough to be the default by preserving verbatim recent turns.

## Questions / Gaps

- **No evidence found** for any redaction/allow-list mechanism governing what user-supplied `inputs` keys reach the prompt beyond simple `{key}` interpolation in `_format_prompt` (`crew_agent_executor.py:1589-1601`); searched `redact|mask|pii|sensitive|filter` across `lib/crewai/src/crewai`.
- Whether the hosted AMP/platform layer applies additional context filtering cannot be determined from this repository; only hook points exist (`before_llm_call_hooks`, `crew_agent_executor.py:134-139`).
- The exact behavior when both custom templates (`system_template`/`prompt_template`) and system-prompt mode could apply is resolved by precedence in `task_execution()` (`prompts.py:120-141`), but no test pins the mixed case.
- `LiteAgent` injects memory into the system message (`lite_agent.py:599-620`) while Crew agents append to the user task prompt (`agent/core.py:656-657`) — likely intentional (cache stability), but undocumented; no test asserts the difference.
- Long-term durability of the summarizer's `<summary>` tag extraction against providers that wrap or escape tags is untested beyond fallback behavior (`agent_utils.py:995-1009`).

---

Generated by `dimensions/11.01-context-selection-policy` against `crewai`.
