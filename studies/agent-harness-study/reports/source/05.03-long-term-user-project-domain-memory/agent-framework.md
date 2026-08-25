# Source Analysis: agent-framework

## Dimension 05.03: Long-Term User, Project, and Domain Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C#/.NET and Python monorepo (Go SDK is external; see README.md:17) |
| Analyzed | 2026-08-25 |

## Summary

Microsoft Agent Framework treats long-term memory as a **pluggable context-engineering concern**, not a built-in singleton store. The core abstraction is the `ContextProvider` two-phase hook (`before_run` retrieves/injects, `after_run` extracts/writes), defined once per language — Python: `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:793`, .NET: `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:42`. On top of this single extension point the repo ships five concrete long-term/backing stores: Mem0 (managed + OSS), Microsoft Foundry Memory Store, Azure Cosmos DB Agent Memory Toolkit (facts/procedural/episodic + user summaries), Redis (tag-scoped text/hybrid vector search plus list-based history), and a file-backed harness memory system (`MEMORY.md` index + topic files + transcript archive with LLM extraction and scheduled consolidation). Conversation-history persistence (`HistoryProvider`, `_sessions.py:943`) and whole-session snapshots (`SessionStore`/`FileSessionStore`, `_sessions.py:1795`/`:1872`) round out what can persist across sessions.

Memory scoping is explicit rather than ambient: Mem0 separates a *storage scope* (`application_id`/`agent_id`/`user_id`) from a *retrieval scope* (`search_*`) that never inherits by default (`python/packages/mem0/agent_framework_mem0/_context_provider.py:50-62`); Foundry requires a caller-supplied `scope` string (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:134-135`; .NET `FoundryMemoryProviderScope.cs:19`); Redis filters on indexed tag fields (`python/packages/redis/agent_framework_redis/_context_provider.py:216-227`); harness file memory namespaces directories by an owner id resolved from session state (`_harness/_memory.py:700-745`). Cross-language scoping taxonomies exist at user / agent / thread(session) / application granularity; there is no first-class organization/global scope beyond the application tag. Privacy posture is unusually deliberate for a framework: injected cross-session content carries `origin_session_ids` attribution for governance/audit (`_sessions.py:624-668`), LLM-derived user summaries are injected as explicitly untrusted context to avoid persistent prompt injection (`packages/azure-cosmos-memory/.../_context_provider.py:385-412`), durable history writes can be gated behind egress verdicts via a run-persistence gate (`_sessions.py:1169-1307`), plaintext-storage security postures are documented in-code (`_sessions.py:1886-1898`, `:2181-2189`), and hosted Foundry deployments physically partition persisted sessions per agent/user (`docs/decisions/0031-hosted-per-user-session-storage-isolation.md`).

Weaknesses: deletion/correction flows are uneven (Python Mem0 exposes no delete/update API at all; only .NET does), no provider implements TTL/expiry so staleness is mitigated only where consolidation or debouncing exists, several stores are marked experimental, and Python vs .NET disagree on whether retrieval scope defaults to storage scope.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Clear model (7–8 band):** one documented SPI (`ContextProvider` at `python/packages/core/agent_framework/_sessions.py:793`; `AIContextProvider` at `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:42`) with multiple swappable backends and explicit, docstring-level scoping contracts.
- **Tests:** 48 focused unit tests for Mem0 scoping alone (`python/packages/mem0/tests/test_mem0_context_provider.py:372`, `:449`, `:500` prove retrieval-scope non-inheritance and cross-user isolation); harness memory has failure-mode tests (`python/packages/core/tests/core/test_harness_memory.py:695`, `:799`, `:837`).
- **Operational safeguards:** untrusted-context framing, cross-session attribution observers, quarantine of corrupt snapshots (`_sessions.py:2054-2065`), consolidation windows that refuse to advance on transient failures (`_harness/_memory.py:1541-1556`).
- **Why not 8+:** no delete/correct API in the Python Mem0 package (grep over `python/packages/mem0/**` for delete/clear/update returns no matches), staleness/TTL is unaddressed outside the harness memory provider, several components are `@experimental` (`_sessions.py:1794`, `:2171`; `[Experimental]` on `FoundryMemoryProviderScope.cs:18`), and the Python/.NET default-retrieval-scope divergence invites porting bugs.

## Evidence Collected

Every entry cites workspace-relative paths into `studies/agent-harness-study/sources/agent-framework/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core SPI (Python) | `ContextProvider` base class; `source_id` attribution; provider-scoped `state` contract; `after_run_once_per_turn` opt-in for history-mutating providers | `python/packages/core/agent_framework/_sessions.py:793-828` |
| Core SPI (.NET) | `AIContextProvider` abstract class; `InvokingAsync`/`InvokedAsync` lifecycle; message filters; security remarks on indirect prompt injection from external stores | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:34-40,42-91,115-116` |
| History persistence | `HistoryProvider` base (load/store flags: `store_inputs`, `store_outputs`, `store_context_from`); built-in `InMemoryHistoryProvider` and experimental `FileHistoryProvider` (JSON Lines / msgspec, append-only) | `python/packages/core/agent_framework/_sessions.py:943-1075,2087-2168,2171-2455` |
| Session snapshots | `SessionStore` (in-memory, no eviction) and `FileSessionStore` (atomic temp-file + `os.replace`, msgspec typed envelope, corrupt-snapshot quarantine, path-traversal guard, documented plaintext security posture) | `python/packages/core/agent_framework/_sessions.py:1794-1868,1871-2084` |
| State type registry | `register_state_type` process-wide codec registry so persisted provider state restores after restart; conflicting registrations fail loudly | `python/packages/core/agent_framework/_sessions.py:325-391` |
| Cross-session attribution | `SessionContext.extend_messages(..., origin_session_ids=...)` stamps `_attribution.origin_session_ids` for governance/audit; observer sample consumes it to defend against chained sub-backdoor attacks (arXiv:2605.06158 cited in-sample) | `python/packages/core/agent_framework/_sessions.py:624-698`; `python/samples/02-agents/context_providers/cross_session_observer.py:9-26,29-87` |
| Egress-gated durability | `_RunPersistenceGate` defers durable persistence until an egress-enforcement middleware verdict permits content; denied content "must never become durable" | `python/packages/core/agent_framework/_sessions.py:1078-1085,1151-1307` |
| Mem0 integration | `Mem0ContextProvider`: storage scope (`user_id`/`agent_id`/`application_id`) stamped on write; separate `search_*` retrieval scope that never inherits; warning when no retrieval scope; partitioned searches merged+deduped; OSS-vs-platform client validation | `python/packages/mem0/agent_framework_mem0/_context_provider.py:44-62,136-235,237-273,277-302` |
| Mem0 isolation tests | `test_search_scope_does_not_inherit_storage_scope`, `test_other_users_memories_are_not_retrieved`, `test_own_memories_are_still_retrieved`, `test_search_agent_id_opts_into_agent_partition` | `python/packages/mem0/tests/test_mem0_context_provider.py:372,394,449,500` |
| Foundry Memory Store | `FoundryMemoryProvider`: required `scope` namespace; static memories fetched once per session then cached in state; incremental `previous_search_id`/`previous_update_id`; `update_delay=300s` debounce before processing updates; all memory errors logged-not-raised ("non-critical") | `python/packages/foundry/agent_framework_foundry/_memory_provider.py:51,81,96-99,134-141,171-187,202-231,262-276` |
| Cosmos Agent Memory Toolkit | `CosmosMemoryContextProvider`: memory types fact/procedural/episodic (`MemoryType` literal); `min_confidence` filter; stable `user_id` resolution with session-id fallback + one-time warning; cadence thresholds (`FACT_EXTRACTION_EVERY_N`, `DEDUP_EVERY_N`, thread/user summaries) configurable, zeroable via `auto_extract=False`; background extraction drained by `flush()` on exit | `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:47,60-72,158,169-185,242-257,262-280,334-414,416-477` |
| Prompt-injection mitigation | LLM-generated user summary injected as user-role "untrusted reference information", never as instructions — comment explains stored-prompt-injection path being closed | `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:385-412` |
| Redis semantic store | `RedisContextProvider`: RediSearch schema with indexed tags `application_id`/`agent_id`/`user_id`/`thread_id`/`conversation_id`; optional hybrid vector query (`AggregateHybridQuery`, alpha 0.7); schema-compatibility validation with explicit `overwrite_index=True` escape hatch | `python/packages/redis/agent_framework_redis/_context_provider.py:45,204-252,305-407,409-412` |
| Redis history + retention | `RedisHistoryProvider`: Redis List per session key; `max_messages` trims oldest via LTRIM; `max_messages=0` retention-disabled writes nothing to Redis (AOF/replica-aware comment); `clear()` deletes the session key | `python/packages/redis/agent_framework_redis/_history_provider.py:43-68,115-117,145-191,203-209` |
| Harness topic memory | `MemoryFileStore` layout `<base>/<encoded source>/<encoded owner>/memory/{MEMORY.md,topics/,transcripts/,state.json}`; owner id from `session.state[owner_state_key]` with traversal rejection; atomic writes | `python/packages/core/agent_framework/_harness/_memory.py:32-41,123-137,655-761,700-710` |
| Extraction & consolidation cadence | Default extraction prompt ("durable facts... at most 5 items"); `consolidation_interval=24h` + `consolidation_min_sessions=5` gate auto-consolidation (`_should_consolidate`); consolidation window advances only if ≥1 topic succeeded | `python/packages/core/agent_framework/_harness/_memory.py:39-68,1505-1573` |
| Model-writable memory tools | `write_memory`, `delete_memory_topic`, `consolidate_memories`, `search_memory_transcripts` tools registered `approval_mode="never_require"`; instructions tell the model to use `read_memory_topic` to inspect/correct a topic before editing | `python/packages/core/agent_framework/_harness/_memory.py:1223-1313,1306` |
| Freshness metadata in topics | `MemoryTopicRecord.updated_at` timestamp and contributing `session_ids` tracked per topic; index pointer lines carry summary + updated_at | `python/packages/core/agent_framework/_harness/_memory.py:144-146,360-400` |
| Session-scoped file memory | `FileMemoryProvider`: working folder derived from `session_id` unless explicit `scope` (e.g., user id) given; flat namespace enforced (nested names rejected); internal files (`*_description.md`, `memories.md`) hidden; 50-entry capped index rebuilt after every write/delete | `python/packages/core/agent_framework/_harness/_file_memory.py:58-78,93-122,220-268,270-299` |
| .NET vector-store memory | `ChatHistoryMemoryProvider`: messages embedded into any M.E.AI `VectorStore`; indexed fields incl. `UserId`, `AgentId`, `ApplicationId`, `SessionId`, `CreatedAt`; PII/redaction safeguards (`Redactor`, trace-log warnings); alternative `OnDemandFunctionCalling` search behavior where the model controls queries (treated as untrusted input) | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:33-52,63-73,120-148` |
| .NET scope objects | `ChatHistoryMemoryProviderScope` (Application/Agent/Session/User); `Mem0ProviderScope` (Application/Agent/Thread/User; ≥1 required); `FoundryMemoryProviderScope` (single caller-controlled string) | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProviderScope.cs:10-53`; `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0ProviderScope.cs:11-57`; `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProviderScope.cs:13-41` |
| .NET delete flow | `Mem0Client.ClearMemoryAsync` issues HTTP DELETE scoped by app/agent/run/user; provider exposes clear-for-scope method reading storage scope from session state | `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Client.cs:115-131`; `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:225-241` |
| Hosted per-user isolation | ADR 0031: physical partition `{root}/a-{agentName}/u-{userId}/c-{contextId}.json` + reject-style traversal guard so a forged conversation id cannot even resolve to another tenant's path; complements strict-resume identity check (403 on user mismatch) | `docs/decisions/0031-hosted-per-user-session-storage-isolation.md` (Context/Decision sections) |
| Hosted scope derivation | `HostedFoundryMemoryProviderScopes.PerUser()` derives the memory scope from `HostedSessionContext.UserId` supplied by the hosting layer | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/HostedFoundryMemoryProviderScopes.cs:17-37` |
| Scoping strategy samples | User-scoped vs agent-scoped vs multi-agent personas with Mem0; comments stress agent-wide retrieval is opt-in because it returns memories written by *any* user | `python/samples/02-agents/context_providers/mem0/mem0_sessions.py:29-166` |
| Session-state mini-memory sample | `UserMemoryProvider` persists extracted user name in provider-scoped session state; survives across turns within a session | `python/samples/01-get-started/04_memory.py:20-61,94-100` |

## Answers to Dimension Questions

**1. What persists across sessions?**
Four tiers, all opt-in via wiring:
- Whole-session snapshots (`session_id`, `service_session_id`, provider `state`) via `SessionStore`/`FileSessionStore` (`python/packages/core/agent_framework/_sessions.py:1795-1796`, `:1872-1899`) — including provider-owned state restored through `register_state_type` (`:325-359`).
- Conversation transcripts via `HistoryProvider` implementations: Redis lists (`python/packages/redis/agent_framework_redis/_history_provider.py:24-29`), JSONL/msgpack files (`_sessions.py:2171-2232`), or service-side storage keyed by `service_session_id` (`_sessions.py:1723-1734`).
- Distilled semantic memory in external services: Mem0 records, Foundry memory-store items, Cosmos facts/procedural/episodic memories + user/thread summaries, Redis hybrid-search documents, .NET vector-store embeddings of chat history (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:129-148`).
- Harness topic memory: durable fact bullets in per-owner `topics/*.md`, indexed by `MEMORY.md`, with raw transcripts archived alongside (`python/packages/core/agent_framework/_harness/_memory.py:32-36`). Without explicit wiring, the default `InMemoryHistoryProvider` keeps everything in-process (`_sessions.py:2094-2099`).

**2. Who can write memory?**
- The **application developer**, by attaching providers and choosing scopes; writes happen automatically in `after_run` from filtered user/assistant/system text (`python/packages/mem0/agent_framework_mem0/_context_provider.py:249-260`; `python/packages/foundry/agent_framework_foundry/_memory_provider.py:246-260`).
- The **LLM itself**, only through tools the framework registers: `write_memory`/`delete_memory_topic`/`consolidate_memories` (`_harness/_memory.py:1223-1275`) and the seven `file_memory_*` tools (`_harness/_file_memory.py:315-485`). Notably these are registered `approval_mode="never_require"`, i.e., the model can persist/delete memory without human approval by default — a policy decision hosts must consciously tighten.
- The **hosting platform**, which supplies trusted identity for scoping (`state["user_id"]` contract, `packages/azure-cosmos-memory/.../_context_provider.py:242-257`; `HostedSessionContext.UserId`, `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/HostedFoundryMemoryProviderScopes.cs:28-31`).

**3. Who can read memory?**
Retrieval is scope-partitioned and injected as context messages during `before_run`. Mem0 reads require an explicitly configured `search_*` scope — with none set, nothing is retrieved and a warning fires (`python/packages/mem0/agent_framework_mem0/_context_provider.py:147-154`), so an unrelated user cannot read another user's memories under a shared agent id without opting in (`tests/test_mem0_context_provider.py:449`). Redis reads AND together whatever identity tags were configured (`_context_provider.py:363-369`). Harness memory reads are confined to the owner directory derived from the session's owner id (`_harness/_memory.py:739-745`), and hosted session files resolve only inside the caller's `u-{userId}` partition (`docs/decisions/0031-...md`). Every injected message is source-attributed, and cross-session origins are exposed for downstream observers (`_sessions.py:646-668`).

**4. Can memory be corrected?**
Partially, and inconsistently across backends. Strongest: harness topic memory supports targeted correction — `read_memory_topic` before editing, `file_memory_replace`/`replace_lines` for surgical edits, `delete_memory_topic`, and `consolidate_memories` to dedupe/drop stale items (`_harness/_memory.py:1214-1275`, `:57-68`). Redis offers `clear()` per session (`_history_provider.py:203-209`); .NET Mem0 offers scoped clearing (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Client.cs:118-131`). Weakest: the Python Mem0 provider exposes only add + search — no update/delete/forget API exists anywhere in `python/packages/mem0/` (verified by grep) — so correcting a wrong Mem0 fact requires out-of-band Mem0 console/API access; Foundry/Cosmos correction is delegated to each service's internal pipelines (dedup/extraction cadence) with no framework-side edit surface.

**5. Can memory become stale?**
Yes. No provider implements TTL or expiry. Mitigations exist only in some stores: harness memory consolidates after ≥5 sessions and ≥24h, dropping stale/transient bullets (`_harness/_memory.py:39-41,1558-1573`); Foundry debounces updates 300s by default and threads incremental `previous_update_id`/`previous_search_id` cursors (`_memory_provider.py:64,210-212,268-272`); Redis caps history length (`max_messages` trim, `_history_provider.py:188-191`); Cosmos filters retrievals by `min_confidence` (default 0.7) and exposes tunable extraction/dedup cadence (`_context_provider.py:97,169-175`); .NET `ChatHistoryMemoryProvider` indexes `CreatedAt` (`ChatHistoryMemoryProvider.cs:142`) but ships no pruning policy in this repo. Mem0 semantic records persist indefinitely unless cleared externally. Staleness is therefore possible everywhere and guaranteed absent active maintenance.

## Architectural Decisions

1. **Memory as a context-provider pipeline, not a memory service.** Both languages converge on a two-phase hook (`before_run`/`after_run` in Python, `InvokingAsync`/`InvokedAsync` in .NET) executed around every invocation (`python/packages/core/agent_framework/_sessions.py:830-870`; `dotnet/.../AIContextProvider.cs:28-31`). Any backend — vector DB, SaaS memory layer, filesystem — plugs in without changing the agent loop.
2. **Explicit, non-inheriting retrieval scopes (Python Mem0).** Storage and retrieval scopes are deliberately decoupled so agent-wide knowledge must be opted into per consumer, preventing a memory written under a shared `agent_id` from leaking to unrelated users (`python/packages/mem0/agent_framework_mem0/_context_provider.py:50-62`).
3. **Owner-namespaced physical storage.** Harness memory encodes source + owner into directory paths with traversal rejection (`_harness/_memory.py:734-745`); hosted Foundry session storage extends the same idea to `{root}/a-{agent}/u-{user}/c-{context}.json` as defense-in-depth beyond identity checks (`docs/decisions/0031-hosted-per-user-session-storage-isolation.md`).
4. **Attribution metadata as a governance primitive.** Injected context messages carry `_attribution.source_id`/`origin_session_ids`, enabling audit and attack detection rather than silently blending origins (`python/packages/core/agent_framework/_sessions.py:646-659`; `cross_session_observer.py:17-21`).
5. **Durability behind an egress gate.** History persistence call sites consult `_RunPersistenceGate` so content denied by egress-enforcement middleware never becomes durable (`python/packages/core/agent_framework/_sessions.py:1078-1085`) — memory writing is treated as data egress.
6. **Fail-open retrieval, fail-loud corruption.** Memory retrieval failures log warnings and continue (`_memory_provider.py:181-183`; cosmos provider `:379-380`), while unreadable persisted state raises or quarantines (`_sessions.py:1955-1978`).

## Notable Patterns

- **Hooks-pattern providers with per-instance `source_id`s** enabling multi-provider composition, per-source filtering (`store_context_from`), and deduplicated message attribution (`python/packages/core/agent_framework/_sessions.py:757-790,1039-1045`).
- **Partition-splitting search**: Mem0 queries user and agent partitions independently and merges/dedupes results to bypass strict logical-AND filter limits (`python/packages/mem0/agent_framework_mem0/_context_provider.py:160-181`).
- **Progressive memory disclosure**: always-loaded `MEMORY.md` table of contents plus keyword-scored selection of ≤3 topic files per turn, with tools to pull more on demand (`_harness/_memory.py:38,1079-1098,1292-1313`).
- **Index sidecars**: both harness memory systems maintain derived markdown indexes (`memories.md`, `MEMORY.md`) rebuilt atomically after mutations and hidden from the agent's own listing/grep surfaces (`_harness/_file_memory.py:76-99,280-299`).
- **Debounced, incremental cloud-memory sync** with cursor ids (`previous_search_id`, `previous_update_id`) so repeated runs don't reprocess the same content (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:202-212,264-272`).
- **Background-extraction drain**: Cosmos schedules cadence-aware fact extraction as asyncio tasks and drains them on context exit so shutdown doesn't discard learning (`packages/azure-cosmos-memory/.../_context_provider.py:262-280,313-332`).
- **State-initializer delegation (.NET)**: scope objects are computed per-session via caller delegates (`Func<AgentSession?, State>`), letting hosting layers inject per-user scopes (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/HostedFoundryMemoryProviderScopes.cs:28-31`).

## Tradeoffs

- **Uniformity vs backend depth.** The common SPI keeps agents portable but means correction/deletion semantics differ per backend (rich in harness/file memory, absent in Python Mem0).
- **Leak safety vs ergonomics.** Python's no-default-retrieval scope is safe but adds friction (a fresh deployment retrieves nothing until `search_*` is set); .NET instead defaults `SearchScope = storageScope` (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:284-287`) — simpler, but mis-scoped apps leak across users. The divergence is itself a migration hazard.
- **Model-writable memory vs approval friction.** Harness memory tools default to `never_require` approval for speed (`_harness/_memory.py:1223-1266`), trading away a HITL checkpoint on persistent-state mutation; contrast with `SkillsProvider`/`FileAccessProvider`, whose analogous defaults require approval (per `python/packages/core/AGENTS.md` harness notes).
- **Fire-and-forget writes vs delivery guarantees.** Foundry memory updates are intentionally not awaited (`_memory_provider.py:262-263` comment "fire and forget"); low latency, but a failed update only logs a warning.
- **Plaintext local persistence vs simplicity.** `FileSessionStore`/`FileHistoryProvider` document that they do not encrypt contents and rely on OS permissions (`_sessions.py:1886-1898`, `:2181-2189`).

## Failure Modes / Edge Cases

- **Eventual-consistency window (documented)**: Mem0 indexes asynchronously; the basic sample hardcodes `await asyncio.sleep(15)` with a comment recommending retries in production (`python/samples/02-agents/context_providers/mem0/mem0_basic.py:64-69`).
- **Total retrieval outage detection**: Mem0 counts failed partition tasks and logs when *all* fail ("unable to verify memory state"), yet still proceeds without memories (`python/packages/mem0/agent_framework_mem0/_context_provider.py:227-228`) — silent degradation by design.
- **Consolidation window sliding**: prevented — the maintenance window advances only if ≥1 topic consolidated successfully, otherwise the next run retries (`_harness/_memory.py:1541-1556`); programmer errors in extractor clients propagate instead of being swallowed (`:83-90`, test at `python/packages/core/tests/core/test_harness_memory.py:837`).
- **Corrupt/partial snapshots**: quarantined with `.corrupt` suffix and version validation, never partially loaded (`_sessions.py:1955-1978,2054-2065`); atomic writes prevent torn files (`:2017-2026`, `_harness/_memory.py:123-137`).
- **Concurrent writers**: last-writer-wins for session snapshots (documented `:1879-1881`); per-topic async locks serialize harness memory merges (`_harness/_memory.py:1472-1503`), tested at `test_harness_memory.py:640`.
- **Persistent prompt injection**: LLM-generated user summaries could become standing high-priority directives; Cosmos frames them as delimited untrusted reference data (`packages/azure-cosmos-memory/.../_context_provider.py:389-412`), and .NET documents the residual risk that vector-store content is accepted as-is (`AIContextProvider.cs:34-40`). Other Python providers inject raw retrieved memories without equivalent framing — an inconsistency.
- **Unscoped writes rejected early**: Mem0/Redis raise `ValueError` when no identity filter is configured (`_context_provider.py:277-280`; redis `:409-412`); Cosmos falls back to ephemeral session-id scoping with a warning, limiting recall to the current session (`_context_provider.py:242-257`).
- **Path abuse**: opaque session ids are b64/sha-encoded into portable filename stems; Windows reserved stems handled; resolved-path containment checked (`_sessions.py:148-177,2072-2084`); nested memory filenames rejected because they'd be unreachable in the flat discovery surfaces (`_harness/_file_memory.py:111-122`).

## Future Considerations

- Add symmetric delete/forget APIs to the Python Mem0 provider (parity with `Mem0Client.ClearMemoryAsync` on .NET) and consider per-record correction hooks for Foundry/Cosmos — currently GDPR-style erasure depends entirely on the backing service.
- Standardize freshness metadata (TTL or `updated_at`-based decay) across providers; today only the harness memory system ages content out, and only via LLM consolidation.
- Reconcile the Python/.NET retrieval-scope default divergence (explicit-none vs inherit-from-storage) to prevent porting-induced leaks.
- Promote experimental stores (`SESSION_STORE`, `FILE_HISTORY`, harness memory, `[Experimental]` Foundry memory) to released status; their current markers (`python/packages/core/agent_framework/_sessions.py:1794,2171,244`; `FoundryMemoryProviderScope.cs:18`) signal instability to adopters.
- Apply the Cosmos-style untrusted-context framing uniformly to all memory injections, not just user summaries.

## Questions / Gaps

- **Organization/domain-wide memory scope**: no evidence found. Searches covered `application_id` tagging (mem0/redis/.NET scopes) and hosting-layer user partitioning; nothing models org/team hierarchies beyond a flat scope string. (Searched: `class .*Memory`, `scope`, `org`, across `python/packages/*` and `dotnet/src/*`.)
- **Vector-store freshness policies (.NET)**: `CreatedAtField` is indexed (`ChatHistoryMemoryProvider.cs:142`) but no pruning/expiry implementation was found in this repo; possibly deferred to each `VectorStore` implementation.
- **Go SDK**: excluded by design — `README.md:17` points to the external `microsoft/agent-framework-go` repository; its memory story was not inspected.
- **Declarative-agent memory wiring**: whether YAML-defined agents (`declarative-agents/`) can bind memory providers declaratively was not examined; no evidence collected either way.
- **Encryption/key management**: no framework-managed encryption-at-rest for file/session stores (explicitly disclaimed at `_sessions.py:1892-1898`); reliance on backend controls for managed stores is stated in .NET XML docs (`ChatHistoryMemoryProvider.cs:44-46`) but not verifiable from this source alone.

---

Generated by `05.03-long-term-user-project-and-domain-memory` against `agent-framework`.
