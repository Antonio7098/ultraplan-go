# Source Analysis: agent-framework

## Dimension 05.07: Memory Privacy, Scope, and Deletion

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (multi-package monorepo: `python/packages/*`, `dotnet/src/*`) |
| Analyzed | 2026-08-25 |

## Summary

The framework ships several independent memory subsystems rather than one memory engine: a session-scoped file-memory harness (`FileMemoryProvider`), an owner-scoped durable memory with LLM extraction/consolidation (`MemoryContextProvider` + `MemoryFileStore`), and service-backed semantic-memory providers for Foundry Memory, Mem0, and Cosmos DB Agent Memory Toolkit, mirrored on the .NET side (`ChatHistoryMemoryProvider`, `FoundryMemoryProvider`, `CosmosChatHistoryProvider`, `ValkeyChatHistoryProvider`, harness `FileMemoryProvider`).

Privacy posture is **scoping-first, everything-else-delegated**. Scoping is explicit and enforced at query time or via physical partitioning, with real tests. Beyond scoping, the framework's privacy controls are thin: there is no content-level PII filter before storage (the only redaction is for *log output* via `Microsoft.Extensions.Compliance.Redaction`), encryption-at-rest and access control are consistently deferred to backend configuration through "security considerations" doc remarks, retention exists only as Cosmos DB TTL (24 h default) plus an explicit "no TTL" admission in Valkey, deletion APIs exist on some providers but not others (notably absent from .NET `ChatHistoryMemoryProvider` and all Python service-backed providers), and there is no audit trail of memory reads/writes/deletes — only operational logging.

## Rating

**6 / 10** — Present but inconsistent.

Rationale against the rubric: the scoping model is clear, typed, tested, and in places fail-closed (`Mem0ContextProvider` retrieval scope; required non-empty Foundry scope), which clears the 4–6 band's floor comfortably and approaches the 7–8 band ("clear model with tests, explicit interfaces"). But operational safeguards are uneven: retention is implemented on only one .NET backend and none of the Python providers; delete coverage is patchy across the provider matrix; no auditing anywhere; encryption/access-control is documentation-deferred; nullable scope fields default to "span all users" semantics; and harness file memory is enabled by default writing to `{cwd}/agent-file-memory` with `never_require` approval tools. That inconsistency keeps it out of the 7–8 band.

## Evidence Collected

Every entry cites file paths relative to the selected source directory with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Session-scoped file memory (Python) | Working folder derived from `session_id` by default; explicit `scope` kwarg groups memories across sessions | `python/packages/core/agent_framework/_harness/_file_memory.py:270-278`, `python/packages/core/agent_framework/_harness/_file_memory.py:239-264` |
| File-memory delete tool | `file_memory_delete` removes the memory file plus its `_description.md` sidecar and rebuilds the index | `python/packages/core/agent_framework/_harness/_file_memory.py:368-389` |
| Flat-namespace guards | Nested paths rejected (`_is_nested_path`) and internal files (`memories.md`, sidecars) reserved | `python/packages/core/agent_framework/_harness/_file_memory.py:111-122`, `python/packages/core/agent_framework/_harness/_file_memory.py:93-99` |
| Harness default store | `create_harness_agent` wires `FileMemoryProvider` by default to `FileSystemAgentFileStore(Path.cwd() / "agent-file-memory")`; opt-out via `disable_file_memory` | `python/packages/core/agent_framework/_harness/_agent.py:183-186`, `python/packages/core/agent_framework/_harness/_agent.py:322-323` |
| Memory tools need no approval | All seven `file_memory_*` tools registered with `approval_mode="never_require"` | `python/packages/core/agent_framework/_harness/_file_memory.py:315`, `python/packages/core/agent_framework/_harness/_file_memory.py:351`, `python/packages/core/agent_framework/_harness/_file_memory.py:368` |
| Owner-scoped durable memory (Python) | Owner ID read from `session.state[owner_state_key]`; absolute paths and `..` segments rejected | `python/packages/core/agent_framework/_harness/_memory.py:700-710` |
| Storage containment | b64-encoded owner/source path components under resolved root; escape raises `ValueError` | `python/packages/core/agent_framework/_harness/_memory.py:734-745` |
| Topic deletion API/tool | `MemoryStore.delete_topic` abstract method + `delete_memory_topic` tool under per-topic lock | `python/packages/core/agent_framework/_harness/_memory.py:602-604`, `python/packages/core/agent_framework/_harness/_memory.py:1240-1252` |
| Transcript search scoping | Transcript archive searched within owner directory only; optional exact `session_id` filter | `python/packages/core/agent_framework/_harness/_memory.py:884-927` |
| Cross-session provenance | Injected memory messages carry deduplicated `origin_session_ids` attribution so hosts can distinguish injected memory from native content | `python/packages/core/agent_framework/_harness/_memory.py:1317-1346`, `python/packages/core/agent_framework/_sessions.py:180-185` |
| Data-minimization prompt (heuristic) | Extraction prompt restricts memories to "durable facts, preferences, decisions" — not a content filter | `python/packages/core/agent_framework/_harness/_memory.py:44-56` |
| Foundry memory scope required (Python) | Empty `scope` raises `ValueError`; every search/update passes `scope=self.scope or context.session_id` server-side | `python/packages/foundry/agent_framework_foundry/_memory_provider.py:132-139`, `python/packages/foundry/agent_framework_foundry/_memory_provider.py:203-208`, `python/packages/foundry/agent_framework_foundry/_memory_provider.py:262-270` |
| Mem0 storage/retrieval split (Python) | `user_id`/`agent_id` stamp writes; separate `search_*` values gate reads; reads fail closed with warning when unset | `python/packages/mem0/agent_framework_mem0/_context_provider.py:50-62`, `python/packages/mem0/agent_framework_mem0/_context_provider.py:147-154`, `python/packages/mem0/AGENTS.md:14-18` |
| Cosmos memory user scoping (Python) | `user_id` resolved from provider state else session id else `"default"`; search and turn-writes keyed by it | `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:242-257`, `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:365-371` |
| Stored-injection mitigation (Python) | User summary injected as user-role "untrusted reference", explicitly not as instructions | `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:392-412` |
| Provider-level message filters (.NET) | `AIContextProvider` filters inputs before provide/store; defaults to External-source-only messages (provenance filter) | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:44-65`, `dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:294-305` |
| Multi-dimensional scope type (.NET) | `ApplicationId`/`AgentId`/`SessionId`/`UserId`; docs state unset fields span all applications/users | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProviderScope.cs:31-52` |
| Scope enforced in queries (.NET) | Non-null scope fields combined into an AND equality filter passed to vector search | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:380-410` |
| Log redaction (.NET) | Default `ReplacingRedactor("<redacted>")`; `EnableSensitiveTelemetryData=true` swaps in `NullRedactor` | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:122`, `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:497`, `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProviderOptions.cs:47-64` |
| PII risk documentation (.NET) | Explicit bullets: stored messages may contain PII; trace logs may contain full queries/results; store accepted as-is | `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:39-53` |
| Scope-delete API (Foundry .NET) | `EnsureStoredMemoriesDeletedAsync` → `MemoryStores.DeleteScopeAsync`, tolerant of 404 | `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:248-285` |
| Required scope object (Foundry .NET) | `FoundryMemoryProviderScope` throws on null/whitespace; scope persisted in session state | `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProviderScope.cs:26-30`, `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:90-91` |
| Retention TTL (Cosmos .NET) | `MessageTtlSeconds = 86400` default, applied on write; null disables | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:84-88`, `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:452-458` |
| Tenant/user physical isolation (Cosmos .NET) | Hierarchical partition key `TenantId→UserId→ConversationId` when tenant+user set; all queries partition-scoped | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:194-215`, `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:248-258` |
| Bulk conversation delete (Cosmos .NET) | `ClearMessagesAsync` deletes all messages in the partition via transactional batches | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:501-557` |
| No-retention admission (Valkey .NET) | Docs: "no TTL and persist indefinitely… Callers are responsible for implementing data retention policies"; `ClearMessagesAsync` provided | `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:25-28`, `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:181` |
| Encryption/access-control delegation (.NET) | Remarks instruct implementers/deployers to ensure access controls + encryption at rest (Cosmos); Mem0 remarks require configuring retention/access controls on the service | `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:23-36`, `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:29-37` |
| Confidentiality labels exist but unwired (Python) | Experimental `ConfidentialityLabel` + `check_confidentiality_allowed` exfiltration guard; referenced only by its own tests, not by any memory provider | `python/packages/core/agent_framework/security.py:109-124`, `python/packages/core/agent_framework/security.py:267-279` |
| Isolation tests (Python) | Two sessions sharing a store see disjoint memories; explicit scope shares; traversal/reserved-name rejection surfaced as tool messages | `python/packages/core/tests/core/test_harness_file_memory.py:224-252`, `python/packages/core/tests/core/test_harness_file_memory.py:255-280` |
| Traversal test (Python) | Owner id `../escape` raises and creates nothing outside base path; source-id namespacing prevents provider collisions | `python/packages/core/tests/core/test_harness_memory.py:216-231`, `python/packages/core/tests/core/test_harness_memory.py:234-275` |
| Filter/redaction tests (.NET) | Filter expression construction asserted verbatim; combined filter compiled and checked match/non-match; redaction behavior parameterized | `dotnet/tests/Microsoft.Agents.AI.UnitTests/Memory/ChatHistoryMemoryProviderTests.cs:408-443`, `dotnet/tests/Microsoft.Agents.AI.UnitTests/Memory/ChatHistoryMemoryProviderTests.cs:464-531`, `dotnet/tests/Microsoft.Agents.AI.UnitTests/Memory/ChatHistoryMemoryProviderTests.cs:281-335` |
| Scope validation tests (Foundry .NET) | Null/empty scope and null state-initializer results throw | `dotnet/tests/Microsoft.Agents.AI.Foundry.UnitTests/Memory/FoundryMemoryProviderTests.cs:72-92` |

No evidence found for: content-level privacy/sensitive-data filters applied before persisting memories; any audit log of memory access; encryption-at-rest implemented inside this repo; retention/TTL configuration in any Python memory provider; delete APIs in Python `FoundryMemoryProvider`/`Mem0ContextProvider`/`CosmosMemoryContextProvider` or .NET `ChatHistoryMemoryProvider`. Searches covered `retention|ttl|expir`, `redact|PII|sensitive`, `audit`, `encrypt`, and `delete` across `python/packages/**` and `dotnet/src/**` memory-related modules.

## Answers to Dimension Questions

### 1. Can memory leak between users?

Largely mitigated by design, but correctness is caller-owned. Concrete mechanisms:

- Fail-closed retrieval: Mem0 refuses to retrieve unless an explicit `search_*` scope is configured, preventing shared-agent memories from being read by unrelated users (`python/packages/mem0/agent_framework_mem0/_context_provider.py:147-154`, rationale at `:58-61`). This is the strongest anti-leak design in the repo.
- Query-time enforcement: .NET `ChatHistoryMemoryProvider` AND-combines every non-null scope field into the vector-search filter (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:385-410`), verified by a compiled-filter test that asserts matching vs non-matching records (`dotnet/tests/Microsoft.Agents.AI.UnitTests/Memory/ChatHistoryMemoryProviderTests.cs:464-531`).
- Physical isolation: Cosmos hierarchical partition keys make cross-user reads structurally impossible when tenant+user are supplied (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:197-215`).
- File isolation: Python/.NET file memory resolves a per-session/per-scope working folder with traversal and containment checks (`python/packages/core/agent_framework/_harness/_file_memory.py:270-278`, `python/packages/core/agent_framework/_harness/_memory.py:708-745`), tested at `python/packages/core/tests/core/test_harness_file_memory.py:224-239`.

Residual leak vectors: (a) `ChatHistoryMemoryProviderScope` fields are individually nullable and unset fields deliberately "span all users/applications" (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProviderScope.cs:34-52`) — a misconfigured host silently widens scope; (b) `CosmosMemoryContextProvider._resolve_user_id` falls back to the literal string `"default"` when neither state nor session id exists (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:257`), which would coalesce anonymous callers into one shared memory namespace; (c) scopes are plain caller-supplied strings with no authentication binding anywhere — nothing verifies that a session is entitled to its claimed scope value.

### 2. Can users delete memory?

Partially. Coverage matrix:

| Provider | Agent-facing delete | Host-facing bulk/scope delete |
|---|---|---|
| Python `FileMemoryProvider` | Yes — `file_memory_delete` incl. sidecar cleanup + index rebuild (`_file_memory.py:368-389`) | No |
| Python `MemoryContextProvider` | Yes — `delete_memory_topic` (`_memory.py:1240-1252`) over `MemoryStore.delete_topic` (`:807-812`) | No |
| .NET harness `FileMemoryProvider` | Yes — `file_memory_delete` (`dotnet/src/Microsoft.Agents.AI/Harness/FileMemory/FileMemoryProvider.cs:51`) | No |
| .NET `FoundryMemoryProvider` | No | Yes — `EnsureStoredMemoriesDeletedAsync` → `DeleteScopeAsync` with 404 tolerance (`FoundryMemoryProvider.cs:248-285`) |
| .NET `CosmosChatHistoryProvider` | No | Yes — `ClearMessagesAsync` (`CosmosChatHistoryProvider.cs:508-557`) |
| .NET `ValkeyChatHistoryProvider` | No | Yes — `ClearMessagesAsync` (`ValkeyChatHistoryProvider.cs:181`) |
| .NET `ChatHistoryMemoryProvider` | No | **No evidence found** — no delete method exists |
| Python `FoundryMemoryProvider` | No | No — only `search_memories`/`begin_update_memories` are called (`_memory_provider.py:175-270`) |
| Python `Mem0ContextProvider` | No | No |
| Python `CosmosMemoryContextProvider` | No | No — delegated to toolkit client |

Notably, `EnsureStoredMemoriesDeletedAsync` has zero test references (`grep` over `dotnet/tests` returns only the definition site).

### 3. Is sensitive data stored?

Yes, raw conversation text is stored by design. `ChatHistoryMemoryProvider.StoreAIContextAsync` persists full message text plus embeddings (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:272-293`); Python Foundry/Mem0 providers ship user+assistant text to external services (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:246-260`, `python/packages/mem0/agent_framework_mem0/_context_provider.py:249-273`). The framework acknowledges this openly: "Conversation messages … may contain PII or sensitive information. Ensure the vector store is configured with appropriate access controls and encryption at rest." (`ChatHistoryMemoryProvider.cs:44-46`). Mitigations that do exist are indirect: provenance-based message filters exclude framework-internal messages from storage (`dotnet/src/Microsoft.Agents.AI.Abstractions/AIContextProvider.cs:62-64`), extraction prompts ask only for durable facts (LLM-enforced heuristic, `python/packages/core/agent_framework/_harness/_memory.py:44-56`), and redaction covers telemetry only — never stored content (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:122,497`).

### 4. Is memory access audited?

No audit trail exists. What exists is operational logging: result counts and scope names logged at Information level with redacted sensitive fields (`dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:428-437`, `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:140-156`), failure logging at Warning/Error, and Python-side provenance attribution (`origin_session_ids`) that lets downstream observers see *which sessions* contributed injected memory (`python/packages/core/agent_framework/_harness/_memory.py:1317-1346`). The docs also flag the inverse risk: Trace-level logging may emit full memory queries/results containing PII (`ChatHistoryMemoryProvider.cs:50-51`). A hosting-layer remark notes audit sinks are the host's concern (`dotnet/src/Microsoft.Agents.AI.Hosting/AgentSessionStore.cs:43`). Searches for `audit` across memory modules returned no audit-log implementation.

### 5. Are scopes enforced in queries?

Yes, where scope is modeled — but enforcement strength varies by tier:

- **Physical**: Cosmos partition keys (`CosmosChatHistoryProvider.cs:203-215,248-258`) — strongest; batch operations even assert all documents share identical partition components (`:359-379`).
- **Query-filter (logical)**: .NET vector-provider expression filters (`ChatHistoryMemoryProvider.cs:385-410`) with compile-and-match tests.
- **Server-side parameter**: Foundry `scope` sent with every search/update call (Python `_memory_provider.py:177,205,266`; .NET `FoundryMemoryProvider.cs:114,203`).
- **Path-prefix containment**: Python `MemoryFileStore` b64-encoded owner directories with root-containment check (`_memory.py:739-745`), and file-memory working folders normalized through `_normalize_relative_path` (`_file_access.py:132-150`) — traversal-tested.
- **Fail-closed gate**: mem0 retrieval requires explicit opt-in (`_context_provider.py:147-154`).

## Architectural Decisions

1. **Scoping is a first-class, typed concept, decided by the host.** Both languages model scope explicitly (`.NET` `ChatHistoryMemoryProviderScope`/`FoundryMemoryProviderScope` classes; Python `scope` constructor args) and resolve it once per invocation from session state (`dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:99-100`, `python/packages/core/agent_framework/_harness/_file_memory.py:270-278`). The framework never derives identity itself; hosts inject it.
2. **Storage scope ≠ retrieval scope (mem0).** Writes are stamped independently of reads, and retrieval defaults to nothing rather than inheriting write scope (`python/packages/mem0/agent_framework_mem0/_context_provider.py:50-62`) — a deliberate fix for the classic "shared agent_id leaks across users" bug, documented in package guidance (`python/packages/mem0/AGENTS.md:14-18`).
3. **Privacy beyond scoping is contractually delegated.** Every backend provider carries XML-doc "Security considerations" that assign encryption-at-rest, access control, and network security to deployment configuration (`CosmosChatHistoryProvider.cs:23-36`, `ValkeyChatHistoryProvider.cs:31-35`, `Mem0Provider.cs:29-37`), and the base class warns that provider data is merged into requests unvalidated (`AIContextProvider.cs:34-40`).
4. **Redaction applies to telemetry, not data.** The `Redactor` abstraction (default `ReplacingRedactor("<redacted>")`, opt-out `EnableSensitiveTelemetryData`) sanitizes log arguments such as UserId while ApplicationId/AgentId/SessionId pass unredacted (`ChatHistoryMemoryProvider.cs:245-253`).
5. **Retention is a backend capability, surfaced where the backend supports it.** Cosmos TTL is exposed as `MessageTtlSeconds` and serialized carefully so null disables TTL rather than sending an invalid property (`CosmosChatHistoryProvider.cs:633-635`); Valkey states flatly that retention is the caller's job (`ValkeyChatHistoryProvider.cs:25-28`).
6. **File memory is isolated-by-default and flat.** Sessions get disjoint working folders derived from session id; nested paths and reserved internal filenames are rejected to keep discovery surfaces scoped (`python/packages/core/agent_framework/_harness/_file_memory.py:111-122,322-331`).

## Notable Patterns

- **Fail-closed defaults**: mem0 retrieves nothing without explicit search scope; Foundry rejects empty scope at construction (Python `ValueError` at `_memory_provider.py:134-135`; .NET `ArgumentException` at `FoundryMemoryProviderScope.cs:28`).
- **Defense-in-depth on file paths**: normalization → nesting check → reserved-name check → containment re-check at the store layer, each step returning tool-visible errors instead of raising (`test_harness_file_memory.py:268-280`), plus symlink/reparse-point hardening documented for the disk store (`python/packages/core/agent_framework/_harness/_file_access.py:771-789`).
- **Provenance metadata as a privacy observability seam**: `origin_session_ids` on injected context messages (`python/packages/core/agent_framework/_sessions.py:180-185`) gives hosts a hook to distinguish memory-derived content — a prerequisite for downstream audit/DLP without being audit itself.
- **Untrusted-channel injection**: retrieved memories and LLM-derived user summaries are injected as `user`-role messages framed as untrusted reference data, blocking stored-content → instruction privilege escalation (`python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:392-412`).
- **An unused-but-available primitive**: experimental FIDES confidentiality labels with `check_confidentiality_allowed` exfiltration gating exist in core (`python/packages/core/agent_framework/security.py:267-279`) but are wired into no memory provider — an obvious integration point left open.

## Tradeoffs

- **Caller-owned scope strings maximize flexibility and transfer liability.** Any provider works with any identity scheme, but nothing binds a scope to an authenticated principal; a host bug (or the `"default"` fallback at `azure-cosmos-memory/_context_provider.py:257`) silently merges tenants.
- **Nullable multi-dimensional scopes trade convenience for foot-guns.** Omitting `UserId` legitimately models agent-wide memory but also makes accidental global scoping invisible (`ChatHistoryMemoryProviderScope.cs:34-52`).
- **Documentation-delegated security keeps the framework backend-agnostic** but means the shipped code alone provides no encryption/access-control guarantee; two integrations of the same provider can differ completely in protection.
- **Approval-free memory tools improve autonomy, reduce oversight.** All `file_memory_*` tools run without human approval (`_file_memory.py:315-477`), so an agent can read or destroy its memory corpus unprompted; contrast with `FileAccessProvider`, whose shared-store tools default to `always_require` (`python/packages/core/AGENTS.md`, FileAccess section).
- **Default-on file memory favors continuity over data minimization**: `create_harness_agent` persists agent-written memory files under `{cwd}/agent-file-memory` unless opted out (`_agent.py:183-186`).

## Failure Modes / Edge Cases

- **Silent scope widening**: forgetting `UserId` in a .NET vector scope yields application-wide retrieval with no warning; only the docs reveal the semantics.
- **Shared anonymous namespace**: `CosmosMemoryContextProvider` maps missing user/session ids to `"default"` (`_context_provider.py:257`), pooling unrelated users' long-term memories in degraded deployments.
- **Cross-user leakage through consolidation**: `MemoryTopicRecord.session_ids` tracks contributing sessions and cross-session origins are attributed (`_memory.py:1317-1346`), but if a host sets a broad `owner_state_key` value, consolidation merges transcripts from all of them into one owner directory with no secondary check.
- **Trace-log PII exposure**: enabling Trace logging emits full search input/output including memory contents unless a custom redactor handles it (`ChatHistoryMemoryProvider.cs:148-155,339-349`); `EnableSensitiveTelemetryData=true` disables redaction entirely by design (`ChatHistoryMemoryProviderOptions.cs:47-54`).
- **Deletion asymmetry**: an operator can wipe Foundry scopes or Cosmos conversations but has no equivalent for `.NET ChatHistoryMemoryProvider` records or any Python service-backed provider; stale memories survive "delete my data" flows implemented against those providers.
- **Untested delete path**: `EnsureStoredMemoriesDeletedAsync` (including its 404 tolerance branch) has no unit-test coverage (`grep` over `dotnet/tests` shows no call sites).
- **Extraction is LLM-gated, not policy-gated**: whether sensitive content becomes a durable "fact" depends entirely on the extractor model honoring the prompt rules (`_memory.py:44-56,1406-1470`); transient extractor failures skip persistence safely (`:1420-1422`), but successful extractions have no sensitivity review.

## Future Considerations

- Wire the existing `ConfidentialityLabel`/`check_confidentiality_allowed` machinery into context providers so writes can be gated by label (e.g., refuse to persist `USER_IDENTITY`-labeled content into shared scopes) — the primitive already exists at `python/packages/core/agent_framework/security.py:267-312`.
- Add delete surfaces for the gaps found above (at minimum: `.NET ChatHistoryMemoryProvider`, Python Foundry/Mem0/Cosmos-memory providers) and add tests for `EnsureStoredMemoriesDeletedAsync`.
- Make scope semantics safe-by-default: either require non-null user dimensions or warn loudly when a dimension is omitted, mirroring the mem0 fail-closed pattern.
- Replace the `"default"` user-id fallback with an error or an ephemeral-random namespace in `CosmosMemoryContextProvider._resolve_user_id`.
- Introduce an optional audit hook on `MemoryStore`/`AIContextProvider` (read/write/delete events with scope + principal) so hosts can satisfy compliance requirements without reimplementing providers.
- Document retention expectations uniformly; today a reader must inspect five different providers to learn that only Cosmos expires data automatically.

## Questions / Gaps

- No evidence found of any mechanism binding memory scopes to authenticated principals — is tenant authorization expected to live entirely in the host/session-store layer? (`dotnet/src/Microsoft.Agents.AI.Hosting/AgentSessionStore.cs:43` hints yes but only as a remark.)
- Why does .NET `FoundryMemoryProvider` expose scope deletion while the Python counterpart does not — platform SDK gap or deliberate omission? The Python provider calls only `search_memories`/`begin_update_memories` (`python/packages/foundry/agent_framework_foundry/_memory_provider.py:175-270`); no comment explains the absence.
- Does the Azure Cosmos DB Agent Memory Toolkit (external dependency) expose deletion/TTL controls that `CosmosMemoryContextProvider` simply does not surface? Not determinable from this source tree.
- Whether Foundry's server-side `scope` parameter enforces isolation physically or logically is defined by the service, not this repo; no client-side verification exists.

---

Generated by `05.07-memory-privacy-scope-deletion` dimension study against `agent-framework`.
