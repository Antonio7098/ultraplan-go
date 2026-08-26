# Source Analysis: agent-framework

## Dimension 11.04: Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C#/.NET and Python (multi-language monorepo); Go stub |
| Analyzed | 2026-08-26 |

> **Path convention:** all citations below are relative to the source root `studies/agent-harness-study/sources/agent-framework/`.

## Summary

Agent Framework implements context provenance as a first-class, dual-language mechanism for **source attribution** and **transformation lineage**, while treating **freshness** and **trust level** as largely out of scope for individual context items.

On the .NET side, every message that enters an agent run can carry an `AgentRequestMessageSourceAttribution` struct (`dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceAttribution.cs:16-43`) — a `SourceType` + `SourceId` pair persisted in `ChatMessage.AdditionalProperties` under the `_attribution` key (`AgentRequestMessageSourceAttribution.cs:22`). The pipeline enforces a three-way provenance taxonomy (`External`, `AIContextProvider`, `ChatHistory`; `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceType.cs:32-42`) that is actively *consumed*: default filters admit only `External` messages into provider input (`AIContextProvider.cs:45`) and strip `ChatHistory`-tagged messages from history re-loading (`ChatHistoryProvider.cs:54`), which prevents injected context from being re-processed or echoed back as user input.

The Python side mirrors this with an `_attribution` dict on `Message.additional_properties` containing `source_id`, `source_type`, and — uniquely — `origin_session_ids` for cross-session memory provenance, explicitly documented for "governance, audit, or behavioral-analysis purposes" (`python/packages/core/agent_framework/_sessions.py:646-659`). Transformation tracking is the strongest area in Python: the compaction subsystem records exclusion state, exclusion reason, group/token metadata, and bidirectional summary↔source linkage as serializable annotations on messages (`_compaction.py:27-37`, `1373-1392`).

Freshness is weakly modeled: `created_at` exists only at the response level and is typed as a plain string with a TODO questioning the type itself (`_types.py:318`). Trust is not a data annotation at all; instead it is handled through boundary controls (deny-by-default MCP sampling, skills-source trust boundaries) and doc-level security guidance. Provenance survives serialization well in Python because annotations live in `additional_properties`, which round-trips through `to_dict`/`from_dict` and is deliberately preserved in history storage.

## Rating

**6 / 10** — "Present but inconsistent" per the rubric.

Rationale: Source attribution is a clear, tested model with explicit interfaces and operational consumers in both languages (7–8 territory). Transformation lineage in Python is mature — bidirectional summary↔source IDs, machine-readable exclusion reasons, and persistence guarantees (`_compaction.py:1619-1621`). But freshness tracking is nearly absent (response-level string timestamp only, `_types.py:318`), trust level has no representation on context items whatsoever, and .NET transformation lineage is weaker than Python's (no summary→message-id backlink; `SummarizationCompactionStrategy.cs:210-218`). The dimension question "can the model trust context because it knows where it came from and when?" is answered *yes* for "where" and *no* for "when"; trust must be inferred from boundary configuration rather than from the data itself.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source attribution type (.NET) | `AgentRequestMessageSourceAttribution` readonly struct: `SourceType` + nullable `SourceId`, equality/hash implemented | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceAttribution.cs:16-43` |
| Attribution storage key | Stored in `ChatMessage.AdditionalProperties` under `"_attribution"` | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceAttribution.cs:22` |
| Source taxonomy | `External`, `AIContextProvider`, `ChatHistory` constants; open string-based type allows custom sources | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentRequestMessageSourceType.cs:32-42` |
| Attribution accessors/mutator | `GetAgentRequestMessageSourceType`, `GetAgentRequestMessageSourceId`, `WithAgentRequestMessageSource` (clones before tagging) | `dotnet/src/Microsoft.Agents.AI.Abstractions/ChatMessageExtensions.cs:18-74` |
| Provider-side stamping | Context-provider output stamped with `AIContextProvider` + provider type FullName during merge | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:174-176` |
| History stamping | Chat history tagged `ChatHistory` + provider class name when loaded | `dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:151`; `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionProvider.cs:154-155,196-197` |
| Attribution consumption (filtering) | Default input filter admits only `External` messages (`AIContextProvider.cs:45`); history reload excludes `ChatHistory`-tagged messages (`ChatHistoryProvider.cs:54`) | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:45`; `dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:54` |
| Python attribution writer | `SessionContext.extend_messages` attaches `_attribution = {source_id, source_type}` to copies of injected messages; merges with existing attribution | `python/packages/core/agent_framework/_sessions.py:661-694` |
| Cross-session origin provenance | `origin_session_ids` kwarg, deduplicated, documented for governance/audit; absence explicitly means "no origin information supplied" | `python/packages/core/agent_framework/_sessions.py:629,646-659,667-668`; dedup helper `_sessions.py:180-188` |
| Provider identity requirement | `ContextProvider.__init__(self, source_id)` — "Used for message/tool attribution so other providers can filter" | `python/packages/core/agent_framework/_sessions.py:810-828` |
| Citation annotations | `Annotation` TypedDict: `type="citation"`, `title`, `url`, `file_id`, `tool_name`, `snippet`, `annotated_regions` | `python/packages/core/agent_framework/_types.py:387-398` |
| Freshness (response-level only) | `created_at` on `ChatResponse`/`ChatResponseUpdate`; alias `CreatedAtT = str` carries TODO "Use a datetimeoffset type?" | `python/packages/core/agent_framework/_types.py:318,2278,2322,2565,2610` |
| Cache freshness (non-item) | `CachingSkillsSource.refresh_interval` staleness via `time.monotonic()` age check against `_cache_timestamps` | `python/packages/core/agent_framework/_skills.py:3915,3935-3937` |
| Trust guidance (not data) | Security remarks: external data may contain indirect prompt injection; implementers should validate integrity/trustworthiness | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:110-113,216-219` |
| Trust boundary enforcement | MCP sampling deny-by-default for untrusted servers; summarizer trust warnings ("only point client at a summarization service you trust") | `python/packages/core/agent_framework/_mcp.py:131,507,1438`; `python/packages/core/agent_framework/_compaction.py:1210-1236` |
| Governance middleware | Purview package models DLP actions/protection scopes and sensitivity `label_id` in `AccessedResourceDetails` (external policy evaluation, not item metadata) | `python/packages/purview/agent_framework_purview/_models.py:154-170,415-446` |
| Group/token transformation metadata | `_group` annotation `{id, kind, index, has_reasoning, token_count}` keys | `python/packages/core/agent_framework/_compaction.py:27-32` |
| Exclusion recording | `set_excluded` writes `_excluded` + `_exclude_reason`; reasons include `truncation`, `sliding_window`, `tool_call_compaction`, `tool_result_compaction`, `summarized`, `token_budget_fallback` | `python/packages/core/agent_framework/_compaction.py:33-34,718-724,866,912,965,1062,1392,1458,1469` |
| Summary↔source lineage (Python) | Summary message annotated `_summary_of_message_ids` + `_summary_of_group_ids`; each summarized message gets `_summarized_by_summary_id` backlink | `python/packages/core/agent_framework/_compaction.py:35-37,375,1373-1392` |
| Summary lineage (.NET) | Summary flagged via `_is_summary` property key; excluded groups get `ExcludeReason = "Summarized by SummarizationCompactionStrategy"`; rollback restores exclusions on LLM failure | `dotnet/src/Microsoft.Agents.AI/Compaction/CompactionMessageGroup.cs:40`; `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:169,198-203,215-218` |
| Compaction observability | OTel activity tags `GroupsSummarized`, `SummaryLength` | `dotnet/src/Microsoft.Agents.AI/Compaction/SummarizationCompactionStrategy.cs:186-187,212` |
| Annotation persistence (Python) | Excluded/annotated messages intentionally kept in storage so annotations survive; `skip_excluded` filter reads `_excluded` from loaded messages | `python/packages/core/agent_framework/_compaction.py:1619-1621`; `python/packages/core/agent_framework/_sessions.py:2148-2149,2302-2303` |
| Serialization survival (Python) | `Message.DEFAULT_EXCLUDE = {"raw_representation"}` so `additional_properties` serializes; constructors restore via `_restore_compaction_annotation_in_additional_properties` | `python/packages/core/agent_framework/_types.py:289-297,1800,1833` |
| Tests: attribution | Tests assert `_attribution` set/not-overwritten, `source_type` recorded from provider object, origin ids deduplicated, callers' originals unmutated | `python/packages/core/tests/core/test_sessions.py:94-180` |
| Tests: compaction lineage | 74 tests incl. assertions on `SUMMARY_OF_MESSAGE_IDS_KEY` contents and group atomicity | `python/packages/core/tests/core/test_compaction.py:1352,1410,1448-1454` |
| Tests: .NET attribution filtering | In-memory history provider tests tag messages with each source type and verify load-time filtering | `dotnet/tests/Microsoft.Agents.AI.Abstractions.UnitTests/InMemoryChatHistoryProviderTests.cs:76,399-400,428-429` |

## Answers to Dimension Questions

**1. Does each context item know where it came from?**
Yes — this is the framework's strongest provenance dimension. .NET tags request messages with a typed `(SourceType, SourceId)` attribution (`dotnet/src/Microsoft.Agents.AI.Abstractions/ChatMessageExtensions.cs:57-74`) applied automatically by both `AIContextProvider.InvokingCoreAsync` (`AIContextProvider.cs:175`) and `ChatHistoryProvider.InvokingCoreAsync` (`ChatHistoryProvider.cs:151`). Python attaches `_attribution {source_id, source_type}` to every provider-injected message copy (`python/packages/core/agent_framework/_sessions.py:661-666`), and additionally supports `origin_session_ids` for cross-session memory, with ordered deduplication across multiple providers (`_sessions.py:677-692`). Content items carry citation-style source references via the `Annotation` TypedDict (`url`, `file_id`, `tool_name`, `snippet`; `_types.py:390-395`). Coverage is not universal though: raw caller-supplied messages are attributed implicitly (`External` is the default in .NET, `ChatMessageExtensions.cs:26`) rather than stamped.

**2. Is freshness tracked?**
Only partially. A `created_at` field exists on `ChatResponse` and `ChatResponseUpdate` (`_types.py:2278,2565`) but is typed as an opaque string with an explicit unresolved design note — `CreatedAtT = str  # Use a datetimeoffset type? Or a more specific type like datetime.datetime?` (`_types.py:318`). There is no timestamp on `Message` or `Content` items, no ingestion time in memory/history providers (verified by search over `python/packages/mem0/agent_framework_mem0/_context_provider.py` and `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py`: no timestamp fields surfaced), and no TTL semantics for context items. The one operational freshness mechanism is `CachingSkillsSource`'s monotonic-clock staleness interval for cached skill lists (`_skills.py:3935-3937`), which applies to tool/skill discovery, not conversational context. **No evidence found** of per-item freshness metadata flowing into model-bound context.

**3. Is trust level indicated?**
Not as data. Neither `Content.additional_properties` nor the `Annotation` schema defines a trust/authority/sensitivity field (`_types.py:387-398,538-540`), and a repository-wide search found trust discussed only as boundary guidance: `AIContextProvider` remarks warn implementers that vector-DB/memory data "may contain adversarial content... Implementers should validate data integrity and consider the trustworthiness of the data source" (`AIContextProvider.cs:110-113`); MCP servers are treated as untrusted third parties with deny-by-default sampling (`_mcp.py:1438`); skills sources define filesystem trust boundaries with fail-closed symlink checks (`_skills.py:1851-1873`). The closest enforced mechanism is Microsoft Purview integration, which evaluates DLP policies and sensitivity labels out-of-band via middleware (`python/packages/purview/agent_framework_purview/_models.py:154-170,415-446`) rather than annotating the context item itself. Consequence: downstream components cannot programmatically distinguish trusted from untrusted context once it is merged into the message list.

**4. Are transformations traceable?**
Yes in Python, partially in .NET. Python compaction writes a complete audit trail onto messages: group id/kind/index/reasoning/token-count under `_group` (`_compaction.py:27-32`), exclusion with machine-readable reasons (`set_excluded`, `_compaction.py:718-724`; reasons at lines 866, 912, 965, 1062, 1458), and bidirectional summarization lineage — the summary message lists `_summary_of_message_ids`/`_summary_of_group_ids` (`_compaction.py:1373-1379`) while each summarized message receives `_summarized_by_summary_id` (`_compaction.py:375,1390-1391`). These annotations persist in storage by design: the provider "keep[s] all messages (including excluded) in storage so annotations are preserved" (`_compaction.py:1619-1621`), and history providers honor a `skip_excluded` flag when replaying (`_sessions.py:2148-2149`). .NET marks summaries with `_is_summary` and sets `ExcludeReason` strings, restoring exclusions if summarization fails (`SummarizationCompactionStrategy.cs:169,198-203`), but does not record which message IDs were folded into a summary — lineage must be reconstructed from group adjacency. Redaction is the gap in both languages: **No evidence found** of redaction transformations being logged; the `protected_data` content field (`_types.py:490,555`) is an opaque carrier, not a redaction record.

## Architectural Decisions

1. **Provenance rides in extensible side-channels, not dedicated schema fields.** Both languages attach attribution to `AdditionalProperties` (`dotnet/.../AgentRequestMessageSourceAttribution.cs:22`; python `_sessions.py:692-694`) rather than adding typed fields to `ChatMessage`. This keeps the core message contract stable and lets custom source types exist (`AgentRequestMessageSourceType` accepts arbitrary strings, `AgentRequestMessageSourceType.cs:22`), at the cost of weaker compile-time guarantees and discoverability.
2. **Attribution is written by the pipeline, consumed by defaults.** Stamping happens centrally in `InvokingCoreAsync` overrides (`AIContextProvider.cs:146-200`; `ChatHistoryProvider.cs:141-151`), and default filters immediately act on it (`AIContextProvider.cs:45` keeps only `External` inputs so providers never re-ingest their own or each other's injections). This makes provenance operationally load-bearing, not merely informational.
3. **Immutability-by-copy on injection.** Python copies messages before tagging so "the caller's original message objects are never mutated" (`_sessions.py:633-637,671-673`); .NET clones before stamping (`ChatMessageExtensions.cs:69`). Provenance markers cannot silently leak backward into caller-held objects.
4. **Exclusion, not deletion, as the compaction primitive.** Summarized/excluded messages remain in storage carrying their annotations (`_compaction.py:1619-1621`), making transformation history durable and auditable rather than destructive.
5. **Trust externalized to boundaries and governance integrations.** Rather than labeling items, the framework gates capabilities at boundaries (MCP sampling callback, skills approval modes) and delegates policy enforcement to Purview middleware — a deliberate separation between context transport and governance policy.

## Notable Patterns

- **Cross-session origin attribution**: `extend_messages(..., origin_session_ids=[...])` lets a memory provider declare that injected content was produced under different sessions, with merge-and-dedup semantics across providers and an explicit contract that absence means "no origin information supplied" (`python/.../_sessions.py:646-659,681-691`). This is rare capability aimed squarely at audit/governance use cases.
- **Fail-closed transformation recovery**: .NET summarization rolls back exclusions if the LLM call fails, leaving "the conversation ... [in a consistent] state" (`dotnet/.../SummarizationCompactionStrategy.cs:198-203`); Python skips compaction and logs when the summarizer returns empty text (`_compaction.py:1367-1371`).
- **Group atomicity for tool-call pairs**: compaction grouping links non-contiguous function-call/result spans by unambiguous `call_id` union-find so transformations never split a call/result pair (`_compaction.py:105-122,150-198`), preserving semantic integrity of transformed context.
- **Telemetry-tagged transformations**: summarization emits OTel activities with `GroupsSummarized`/`SummaryLength` tags (`dotnet/.../SummarizationCompactionStrategy.cs:186-187,212`), giving operators an observable trail even where message-level lineage is thin.

## Tradeoffs

- **Side-channel flexibility vs. type safety**: storing provenance in `additional_properties` dictionaries means any code can overwrite or drop `_attribution`/`_excluded` keys without compiler help; correctness relies on convention and tests (`python/.../test_sessions.py:103-110` covers non-overwrite, but nothing prevents downstream user code from stripping the keys).
- **Persistence of exclusions vs. storage growth**: keeping excluded + summarized messages forever preserves lineage but inflates session/history stores indefinitely; no eviction/TTL mechanism was found.
- **Default-filter simplicity vs. multi-hop provenance**: .NET's `ProvideInputMessageFilter` admitting only `External` messages (`AIContextProvider.cs:45`) cleanly prevents echo loops, but discards prior provider attribution context for chained providers; Python's merge-based approach retains more lineage at the cost of more complex attribution merging logic (`_sessions.py:677-692`).
- **Trust-at-boundary vs. trust-in-data**: boundary gating scales well operationally but provides zero signal inside the context window itself — a model or downstream middleware cannot treat retrieved RAG fragments differently from user text.

## Failure Modes / Edge Cases

- **Attribution loss across serialization boundaries (.NET)**: the attribution struct is stored as an opaque object in `AdditionalProperties`. No JSON converter for `AgentRequestMessageSourceAttribution` was found in `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentAbstractionsJsonUtilities.cs` (searched; no match), so round-tripping a tagged message through JSON may degrade the typed value. In-memory flows are covered by tests (`InMemoryChatHistoryProviderTests.cs:399-429`), but durable-storage survival is unverified in-repo. **No evidence found** either way for JSON-persisted attributions.
- **Unattributed injection paths**: Python's `SessionContext.extend_instructions` adds instructions without any `_attribution` marker on the resulting messages (`_sessions.py:700-709`), so instruction provenance is coarser than message provenance.
- **Ambiguous call/result pairing**: compaction deliberately leaves genuinely ambiguous duplicate `call_id` declarations unlinked rather than guessing (`_compaction.py:118-122`), which is safe but can leave tool-call groups exempt from summarization unexpectedly.
- **Freshness decay is invisible**: since context items carry no timestamps, a stale memory replayed weeks later is indistinguishable from a fresh one; only response-level `created_at` exists and is best-effort provider data (`_types.py:2294`).
- **Summarizer compromise**: the docs themselves flag that a malicious summarization endpoint becomes persistent trusted context ("permanently becomes part of chat history and is trusted the same as any other assistant [message]"), mitigated only by documentation warnings (`_compaction.py:1210-1236`).

## Future Considerations

- Add a typed freshness field (e.g., real datetime) to `Message`/`Content` and resolve the `CreatedAtT = str` placeholder (`_types.py:318`); thread ingestion timestamps through history/memory providers so stale-context detection becomes possible.
- Introduce an optional trust/sensitivity enum on `Content` or in `Annotation`, with the existing Purview `label_id` flow (`purview/.../_models.py:427-446`) as a natural producer.
- Port Python's bidirectional summary lineage (`_summary_of_message_ids` / `_summarized_by_summary_id`) to the .NET strategies, which currently record only an exclude-reason string and an `_is_summary` flag.
- Provide a durable-storage test/converter guarantee for `.NET` attribution surviving JSON serialization of `AdditionalProperties`.
- Record redaction events as first-class transformation annotations, mirroring the compaction pattern.

## Questions / Gaps

- No evidence found of per-item trust, authority, or sensitivity annotations anywhere in the selected source (searched `trust`, `authority`, `sensitivity`, `label` across `python/packages/core/agent_framework/*.py`; hits were docstrings/boundary logic only).
- No evidence found of redaction logging or transformation records for content sanitization (searched `redact` across core; single unrelated hit in `python/packages/core/agent_framework/_agents.py:705`).
- Whether mem0/Cosmos memory connectors expose upstream timestamps (e.g., mem0 memory `created_at`) into context messages could not be confirmed from provider code (`python/packages/mem0/agent_framework_mem0/_context_provider.py` contains no timestamp handling); if the backing service tracks it, the framework drops it.
- The Go implementation directory (`go/`) contained no substantive modules to analyze; findings above reflect the .NET and Python stacks only.

---

Generated by `dimensions/11.04-context-provenance-integrity.md` against `agent-framework`.
