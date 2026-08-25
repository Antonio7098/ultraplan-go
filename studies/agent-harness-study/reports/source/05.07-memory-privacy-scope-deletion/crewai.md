# Source Analysis: crewai

## 05.07 Memory Privacy, Scope, and Deletion

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (pydantic models, LanceDB / Qdrant Edge vector stores, SQLite, Click CLI) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI ships a single unified `Memory` class (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:76`) backed by pluggable storage (LanceDB default, Qdrant Edge optional; protocol at `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/backend.py:45`). Privacy and scope are handled through three complementary mechanisms:

1. **Hierarchical scopes** — every record carries a filesystem-like `scope` path (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:28-31`), with `MemoryScope` views restricted to a root path (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:38`), `MemorySlice` multi-scope read views that are read-only by default (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:227-236`), and automatic per-crew/per-flow `root_scope` namespacing (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:652-680`, `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:832-836`). Scope enforcement in queries is real and tested.
2. **A record-level privacy filter** — `source` provenance plus a boolean `private` flag (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:60-73`), enforced as a post-search filter in both shallow recall (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:746-751`) and deep RecallFlow (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/recall_flow.py:109-114`). Private memories are hidden by default from agent prompt augmentation and memory tools. However, the filter is application-layer only, `source` is an unauthenticated caller-supplied string, storage-level APIs do not honor it, and no unit test covers it.
3. **Deletion APIs** — rich `forget()` criteria (scope/categories/age/metadata/IDs), scoped `reset()` vs `reset_all()`, a `crewai reset-memories` CLI, and verified delete implementations in both backends.

Retention policies, encryption-at-rest, anonymization, and true audit logging are absent: stores are plaintext local directories (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:67-78`), there is no TTL/expiry mechanism for memory records, and "auditing" is limited to opt-in observability events on the event bus. The overall trust model is cooperative/honor-system: suitable for a single operator's local agent runs, not for hostile or strictly multi-tenant deployments despite documentation framing ("multi-user or enterprise deployments", `studies/agent-harness-study/sources/crewai/docs/edge/en/concepts/memory.mdx:480`).

## Rating

**5 / 10** — Present but inconsistent and fragile in places.

- Scoping is a clear model with explicit interfaces and good tests (would score 7–8 alone): root-scope read isolation, reset isolation, and consolidation isolation are all unit-tested (`studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_memory_root_scope.py:841-1054`, `studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_memory_root_scope.py:1123-1186`).
- Deletion is well covered by API + CLI + tests.
- But the privacy filter has **zero test coverage** (searched all of `lib/crewai/tests` for `private`/`include_private` — only unrelated hits such as pydantic private attrs and flow conversation visibility tests were found), is bypassable at the storage layer, and relies on caller-supplied identity.
- Retention and encryption are absent entirely, and audit is best-effort observability rather than an access log.

Per the rubric this sits in the middle band: mechanisms exist and the scope half is solid, but privacy guarantees are fragile and unproven under failure or adversarial use.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Record schema with `scope`, `source`, `private` fields | `MemoryRecord` defines hierarchical scope path, source provenance "used for provenance tracking and privacy filtering", and private flag visible only to same-source recalls | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:28-31`, `:60-73` |
| Scoped view restricted to root path | `MemoryScope._scope_path()` prefixes every operation's scope with the view root | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:91-100` |
| Scoped forget/reset constrained to root | `MemoryScope.forget()`/`reset()` compute prefix from root before delegating | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:174-190`, `:212-215` |
| Read-only slice view | `MemorySlice.read_only=True` default; `remember()` returns None when read-only | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:234-236`, `:269-290` |
| Automatic crew namespace | Crew validator auto-sets `root_scope=f"/crew/{sanitized_name}"` when `memory=True`; user-supplied instances are left untouched | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:652-684` |
| Automatic flow namespace | Flow auto-memory uses `root_scope=f"/flow/{flow_name}"` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/flow/runtime/__init__.py:832-836` |
| Agent saves nested under crew root | Executor extends root scope with `/agent/<sanitized-role>` on save | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:49-63` |
| Scope prefix enforced in queries (LanceDB) | Search adds SQL predicate `scope LIKE '<prefix>%'` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:387-390` |
| Scope prefix enforced in queries (Qdrant) | Ancestor-keyword filter built from scope path ancestors | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:65-78`, `:243-253` |
| Privacy filter, shallow recall | Post-search list comprehension drops records where `r.private and r.source != source` unless `include_private` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:746-751` |
| Privacy filter, deep recall | Same filter applied inside `_do_search` per (embedding × scope) task | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/recall_flow.py:109-114` |
| Default-deny at integration points | Agent prompt augmentation calls `recall(query, limit=5)` without `include_private`/`source`; memory tools likewise call `recall(query, limit=20)` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agent/core.py:646-655`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/memory_tools.py:52` |
| Storage layer ignores `private` | `LanceDBStorage.search/list_records/get_record` never reference the `private` column as a filter (it is only persisted) | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:371-409`, `:494-510`, `:361-369` |
| Delete API with rich criteria | `Memory.forget(scope, categories, older_than, metadata_filter, record_ids)` delegating to `StorageBackend.delete` | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:818-850`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/backend.py:80-100` |
| Verified deletion counts | LanceDB `delete()` counts rows before/after each delete branch; Qdrant deletes across central+local shards | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:411-464`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:386-486` |
| Full-store wipe paths | `reset(scope_prefix=None)` drops the LanceDB table / removes Qdrant shard dirs | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:603-616`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:711-721` |
| Reset race safety | `reset_all()` holds `_reset_lock`, drains pending background writes first; test proves a save cannot interleave between drain and reset | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:1015-1035`; `studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:990-1035` |
| CLI deletion entry point | `crewai reset-memories` maps flags to `crew.reset_memories(command_type=...)` / flow memory reset; legacy types ('long','short','entity','external') normalized to 'memory' | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/reset_memories.py:63-132`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:2281-2322`; `studies/agent-harness-study/sources/crewai/lib/cli/src/crewai_cli/cli.py:456-498` |
| LLM-driven consolidation deletes/updates overlapping records | Consolidation plan actions 'delete'/'update' executed against storage on save | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:398-479` |
| Plaintext local storage location | LanceDB dir defaults to `$CREWAI_STORAGE_DIR/memory` or platform user-data dir; kickoff outputs in plain SQLite file; no encryption parameter exists anywhere | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:67-78`; `studies/agent-harness-study/sources/crewai/lib/crewai-core/src/crewai_core/paths.py:16-26`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:24-29` |
| Observability events (audit-ish) | Memory save/query started/completed/failed events carry query text and content values but no authenticated actor | `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/memory_events.py:23-79`; emitted at `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:474-520`, `:723-816` |
| Documented privacy contract | Docs describe source tracking, private memories, and `include_private=True` admin override | `studies/agent-harness-study/sources/crewai/docs/edge/en/concepts/memory.mdx:444-480` |
| No retention/TTL for memory | Repo-wide search for `retention|TTL|ttl|expire|purge` finds TTLs only in file store, caches, and A2A auth — none in `crewai/memory/` | searched `lib/crewai/src` (89 matches, zero in `memory/`) |

## Answers to Dimension Questions

1. **Can memory leak between users?**
   Yes, under several concrete conditions. (a) Scope isolation is advisory: `recall()` without `root_scope` searches globally by design (`studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_memory_root_scope.py:912-939`), and a user-provided `Memory` instance passed to a crew deliberately receives *no* auto root-scope (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:681-684`). (b) The `private` flag is filtered only after rows are fetched from the store; storage-level `list_records()`, `get_record()`, and raw `search()` return private records unfiltered (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:371-409`). (c) `source` is a caller-supplied string with no authentication, so any code can claim any source or pass `include_private=True`. (d) The Qdrant Edge backend intentionally shares one central shard across all worker processes on a machine (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:1-7`). Cross-OS-user leakage is limited only by filesystem permissions on the user-data directory; no explicit hardening found.

2. **Can users delete memory?**
   Yes — this is the strongest area. Programmatic: `forget()` with scope/category/age/metadata/ID filters (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:818-850`), scoped `reset()` and `reset_all()` (`:1015-1035`), and `MemoryScope.forget()/reset()` confined to the view root (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:174-190`, `:212-215`). Operational: `crewai reset-memories --all|--memory|--knowledge|--kickoff-outputs` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/utilities/reset_memories.py:94-132`). Deletion counts are verified against row counts, and resets are race-safe against in-flight background writes (`studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_unified_memory.py:990-1035`).

3. **Is sensitive data stored?**
   Content is stored verbatim as plaintext, including anything an agent chooses to write — the official docs even demonstrate storing `"Alice's API key is sk-..."` as a private memory (`studies/agent-harness-study/sources/crewai/docs/edge/en/concepts/memory.mdx:466-478`). There is no PII detection, redaction, or anonymization step anywhere in the encode path (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py`). Embeddings are excluded from event/serialization output via `exclude=True` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:54-59`), a token-saving measure rather than a security control. Kickoff task outputs (full task results and inputs) persist indefinitely in plaintext SQLite until manually deleted (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:48-57`, `:203-222`).

4. **Is memory access audited?**
   Partially, as observability rather than audit. Every save/query emits Started/Completed/Failed events including query text and content values (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/memory_events.py:23-79`; emission points at `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:474-520`, `:723-816`), and the tracing listener tracks memory retrieval/save activity (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:441-503`). These events attribute operations to agent role/task but capture no authenticated principal, are delivered to in-process subscribers only, and constitute no tamper-evident log. Deletion operations emit **no** events at all.

5. **Are scopes enforced in queries?**
   Yes, at the storage layer of both backends: LanceDB compiles the prefix into a `LIKE` predicate (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:387-390`, index maintained at `:183-199`), and Qdrant matches a precomputed `scope_ancestors` keyword index (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:243-253`). Enforcement is tested for recall, list_records, list_scopes, info, reset, and consolidation searches (`studies/agent-harness-study/sources/crewai/lib/crewai/tests/memory/test_memory_root_scope.py:841-1054`, `:1123-1186`). Caveat: enforcement depends on callers going through `Memory`'s root_scope plumbing; a caller constructing a `MemoryScope` can pass any absolute-looking scope and the concatenation logic (`memory_scope.py:91-100`) will still nest it under the root — but direct `storage.search(scope_prefix=None)` calls bypass scoping entirely.

## Architectural Decisions

- **One unified memory instead of short/long/entity stores**, with hierarchical path scopes as the sole partitioning primitive (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:28-31`). This makes tenancy a naming convention (`/crew/<name>/...`, `/flow/<name>/...`) enforced by prefix predicates rather than physical separation.
- **Privacy filtering at the orchestration layer, not the storage layer**: backends return raw nearest-neighbors and `Memory.recall()`/`RecallFlow` strip private rows afterward (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:746-751`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/recall_flow.py:109-114`). This keeps the `StorageBackend` protocol simple (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/backend.py:56-78` has no privacy parameters) but means any consumer below the `Memory` facade sees everything.
- **Identity as data, not authentication**: `source` is a free-form provenance string on the record (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:60-66`); there is no principal, session binding, or credential check anywhere in the module.
- **Deletion-first lifecycle**: no expiry daemon or retention config; the system relies on explicit `forget`/`reset` plus an LLM consolidation step that may merge/update/delete near-duplicate records on save (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:398-479`).
- **Read-only as a structural guardrail**: slices default to `read_only=True` and tool factories omit the RememberTool for read-only memory so agents are "never offered a save capability they cannot use" (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/memory_scope.py:234-236`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/tools/memory_tools.py:104-130`).
- **Local-first plaintext persistence** keyed by env var/platform data dir, with a pluggable factory hook for custom backends (`set_memory_storage_factory`, `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/factory.py:33-51`) — encryption/ACL would have to come from such a custom backend.

## Notable Patterns

- **Root-scope composition**: `join_scope_paths(root, inner)` normalizes double/trailing slashes; crews derive `/crew/<sanitized-name>` and agents append `/agent/<role>` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/utils.py:8`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/agents/agent_builder/base_agent_executor.py:51-61`), giving deterministic per-agent namespaces under the crew root.
- **Post-search oversample-then-filter**: recall fetches `limit * _RECALL_OVERSAMPLE_FACTOR` candidates so post-filters (categories, metadata, privacy, time cutoff) still fill the result set (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/types.py:12-17`; `studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/recall_flow.py:97-115`).
- **Write barrier around destructive ops**: `reset()` drains pending background saves under a re-entrant lock before deleting, preventing a queued write from resurrecting data after a reset (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/unified_memory.py:1022-1029`).
- **Verified destructive writes**: every delete path computes `before - count_rows` so callers get an accurate deletion count (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/storage/lancedb_storage.py:422-464`).
- **Docs-as-spec for privacy**: the documented semantics of `source`/`private`/`include_private` match the implementation exactly (`studies/agent-harness-study/sources/crewai/docs/edge/en/concepts/memory.mdx:444-480` vs `unified_memory.py:746-751`), which is good hygiene even though the feature is untested.

## Tradeoffs

- **Simplicity vs. isolation**: keeping privacy out of the storage protocol simplifies backend authorship but creates a privileged bypass surface (`list_records`, `get_record`, raw `search`) and forces every higher-level caller to remember the filter correctly.
- **Convention over enforcement for tenancy**: auto root-scoping is ergonomic, but because it only applies when `memory=True` (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/crew.py:665-684`), mixing shared and user-supplied Memory instances on one machine silently pools their records in one table.
- **Default-deny privacy, honor-system admin**: hiding private records by default protects against accidents (agents/tools don't see them), yet `include_private=True` requires no elevation (`studies/agent-harness-study/sources/crewai/docs/edge/en/concepts/memory.mdx:476-477`).
- **LLM consolidation vs. record immutability**: letting the encoding LLM delete/update "outdated" records keeps memory tidy (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/memory/encoding_flow.py:398-412`) but means deletion is not exclusively user-driven, complicating any future compliance story.
- **Observability events include content**: emitting full query/content text in events aids debugging but would leak sensitive values into any configured listener/log sink (`studies/agent-harness-study/sources/crewai/lib/crewai/src/crewai/events/types/memory_events.py:27-28`, `:57-58`).

## Failure Modes / Edge Cases

- **Private-record displacement**: because private rows occupy ANN candidate slots before being filtered out (`lancedb_storage.py:391-393` fetches only `limit*3` candidates; filter happens later), a scope saturated with another user's private records can crowd out non-private matches within the fetched window.
- **Unauthenticated source spoofing**: `recall(query, source="user:alice")` grants access to Alice's private memories to anyone who can call the function — the string is compared directly (`unified_memory.py:750`).
- **Reset vs. open handles**: `reset(None)` drops the table while other `Memory` instances may hold the old `self._table` handle; tests work around stale-table references with fresh instances (`test_memory_root_scope.py:1046-1054`), hinting at a real multi-instance hazard.
- **Qdrant orphan recovery can lose data**: if an orphaned worker shard's vector dimension cannot be inferred, its records are skipped with a warning during cleanup (`qdrant_edge_storage.py:849-854`); similarly, delete failures on individual shards are swallowed as debug logs (`:408-409`), so a reported deletion count can overstate what was actually removed.
- **No retention failsafe**: absent TTLs and given background saves fire-and-forget into SQLite/LanceDB files (`unified_memory.py:297-322`), sensitive content persists indefinitely if operators never run `reset-memories`.
- **Legacy configs silently unscoped**: `_ensure_memory_kind` backfills pre-1.14.6 dicts (`memory_scope.py:20-35`), meaning old checkpoints restored as bare `Memory` keep `root_scope=None` and regain global visibility.

## Future Considerations

- Push the `private`/`source` predicate down into `StorageBackend.search`/`list_records` (both backends already persist these columns: `lancedb_storage.py:257-258`, `qdrant_edge_storage.py:210-211`) so isolation holds below the facade.
- Add unit tests for `private`/`include_private` behavior in shallow recall, deep RecallFlow, and `MemorySlice.recall` — currently the only privacy-related coverage in the repo concerns flow conversation history (`test_flow_conversation.py:302-316`).
- Introduce retention configuration (per-scope TTL/max-age) hooking into the existing compaction cycle (`lancedb_storage.py:201-231`) and an event for deletions to close the audit gap.
- Offer encryption-at-rest (e.g., encrypted LanceDB/SQLCipher options) or document reliance on full-disk encryption explicitly.
- Bind `source` to an authenticated principal (or hash it server-side) if multi-user deployment remains a documented use case.

## Questions / Gaps

- **No evidence found** for any encryption or access-control configuration of memory stores: repo-wide searches for `encrypt`, `retention`, `anonymi*`, `redact`, `purge`, and `ttl` returned hits only in unrelated subsystems (LLM provider reasoning-content handling, A2A auth token caching, file store TTLs). Searched: `lib/crewai/src/**` and `docs/edge/en`.
- **No evidence found** for deletion/anonymization audit trails: `forget`/`reset` emit no events (checked `events/types/memory_events.py` — no delete event type exists).
- **Untested privacy filter**: no test in `lib/crewai/tests/memory/` exercises `private=True` writes or `include_private` recalls (searched all memory tests; only `test_qdrant_edge_storage.py:275` sets `"private": False` as inert payload). Whether cross-source recall truly hides private rows is therefore implemented-but-unverified behavior.
- Tenant model beyond the single-machine, single-operator assumption (e.g., remote/server memory) could not be assessed: no server-side memory API exists in this source; the A2A subsystem shares no memory state (verified by absence of `Memory` imports under `src/crewai/a2a/`).

---

Generated by `05.07-memory-privacy-scope-deletion` against `crewai`.
