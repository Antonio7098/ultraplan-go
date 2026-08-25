# Source Analysis: openai-agents-sdk

## Context Provenance and Integrity (Dimension 11.04)

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ (pydantic, asyncio, OpenAI Responses API; SQLite/Redis/MongoDB/Dapr session backends) |
| Analyzed | 2026-08-25 |

## Summary

The OpenAI Agents SDK treats provenance as a per-subsystem concern rather than a uniform property of every context item. Conversation items (`RunItem` subclasses) carry *origin* metadata — the producing agent, and for tool-backed items a serializable `ToolOrigin` record naming the MCP server or agent-as-tool source — but carry **no freshness timestamps and no trust level** at the item layer. Freshness exists only where storage imposes it: SQLite/Dapr sessions persist `created_at`/`updated_at` columns that are discarded on read-back, while the sandbox memory subsystem embeds `updated_at:`/`rollout_path:`/`terminal_state:` headers in memory files and actively uses them for freshness-ordered selection and consolidation.

Transformation history is strongest at the state boundary: `RunState.to_json()` stamps a schema version backed by an enforced change log (`SCHEMA_VERSION_SUMMARIES`), wraps the user context in a `context_meta` block recording how it was serialized (`serialized_via`, `requires_deserializer`, `omitted`) with the original type's class path stored "for reference only; never auto-import it for safety". The sandbox boundary adds integrity safeguards — closed class-provenance tables decide which mount/strategy/pattern classes are trusted, mount authority (credentials) is redacted from serialized state with a marker that blocks resume until rebind from a trusted manifest, and SHA-256 checksums cover file transfer. Memory consolidation prompts mandate evidence-traceable summaries with secret redaction, treating rollout text as untrusted data.

Net assessment: provenance is real but uneven. It is mature and tested in serialization/resume and sandbox security domains; it is largely absent (implicit ordering only) on ordinary conversation items flowing to the model.

## Rating

**6 / 10** — Present but inconsistent across layers. Explicit, tested provenance interfaces exist for tool origin (`ToolOrigin` + round-trip tests in `tests/test_tool_origin.py`), context serialization metadata (`context_meta` asserted in `tests/test_run_state.py:920-922,983-986,1063-1067`), sandbox memory freshness ordering (`tests/sandbox/test_memory.py:593`), and mount trust classification. However, regular conversation items have no freshness or trust fields, session read-backs drop storage-layer timestamps, compaction discards pre-compaction history without an in-band record of what was summarized, and trust levels exist only as binary SDK-class trust at the sandbox boundary rather than graded authority annotations on context content.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Item-level source reference | `RunItemBase.agent` — "The agent whose run caused this item to be generated" | src/agents/items.py:97-100 |
| Handoff provenance | `HandoffOutputItem.source_agent` / `target_agent` fields | src/agents/items.py:309-322 |
| Tool origin model | `ToolOriginType` enum (function/mcp/agent_as_tool) and frozen `ToolOrigin` dataclass with `mcp_server_name`, `agent_name`, `agent_tool_name` | src/agents/tool.py:285-300 |
| Origin attached to items | `ToolCallItem.tool_origin`, `ToolCallOutputItem.tool_origin`, `ToolApprovalItem.tool_origin` | src/agents/items.py:397-398,444-445,575-576 |
| Occurrence identity | `InputItem.input_id` "durable occurrence identifier used for exactly-once conversation tracking" | src/agents/items.py:163-166 |
| Side-channel data separation | `ToolCallOutputItem.custom_data` — "not sent back to the model when the output item is replayed"; extractor in `normalize_custom_data` | src/agents/items.py:447-452; src/agents/util/_custom_data.py:18-33 |
| Server metadata stripping on replay | `_output_item_to_input_item` strips output-only `created_by` before replaying items to the API | src/agents/items.py:237-256 |
| No freshness on run items | Run item dataclasses contain no timestamp fields; `created_at` appears only inside provider raw payloads | src/agents/items.py:156-176 |
| Session storage freshness | SQLite sessions table has `created_at`/`updated_at`; messages table rows get `created_at DEFAULT CURRENT_TIMESTAMP` | src/agents/memory/sqlite_session.py:233-254 |
| Timestamps dropped on read | `get_items` selects only `message_data`; timestamps never re-enter context | src/agents/memory/sqlite_session.py:288-323 |
| Durable creation timestamp | Dapr session preserves `created_at` across writes using etag-guarded metadata updates | src/agents/extensions/memory/dapr_session.py:191-215,393-416 |
| Sandbox memory provenance header | Raw memories written with `rollout_id:`/`updated_at:`/`rollout_path:`/`rollout_summary_file:`/`terminal_state:` header lines | src/agents/sandbox/memory/manager.py:191-213,332-348 |
| Summary provenance header | Rollout summaries prefixed with `session_id:`, `updated_at:`, `rollout_path:`, `terminal_state:` | src/agents/sandbox/memory/manager.py:351-365 |
| Freshness-driven selection | `_updated_at_sort_key` sorts unknown/missing timestamps last; phase-two selection orders by recency | src/agents/sandbox/memory/storage.py:126-140,201-234 |
| Versioned selection manifest | `write_phase_two_selection` writes `{version: 1, updated_at: <ISO>, selected: [...]}` | src/agents/sandbox/memory/storage.py:189-199 |
| Prompt-enforced provenance | Consolidation prompt requires per-rollout annotations `rollout_id`, `updated_at`, `terminal_state` ("Provenance and metadata" rules) | src/agents/sandbox/memory/prompts/memory_consolidation_prompt.md:291-305 |
| Freshness as conflict rule | "Treat `updated_at` as a first-class signal: fresher validated evidence usually wins" | src/agents/sandbox/memory/prompts/memory_consolidation_prompt.md:320-333 |
| Untrusted-content handling | Extraction prompt: rollout text is data not instructions; secrets replaced with `[REDACTED_SECRET]`; raw rollouts immutable | src/agents/sandbox/memory/prompts/rollout_extraction_prompt.md:18-24 |
| Serialization transformation meta | `_serialize_context_payload` records `serialized_via` ∈ {none, mapping, context_serializer, model_dump, asdict, omitted} plus warnings | src/agents/run_state.py:1460-1583 |
| Context meta schema | `_build_context_meta`: `original_type`, `serialized_via`, `requires_deserializer`, `omitted`, optional `class_path` | src/agents/run_state.py:2321-2340 |
| Safety-conscious class path | class_path stored "for reference only; never auto-import it for safety" | src/agents/run_state.py:2336-2339 |
| Restore-time warning | `_context_meta_warning_message` drives explicit restoration guidance instead of silent type erasure | src/agents/run_state.py:2352-2367; tests/test_run_state.py:920-922,983-986,1063-1067 |
| State format change log | `CURRENT_SCHEMA_VERSION = "1.17"`; every bump must add a summary entry, enforced by assertions; v1.9 added tool-origin persistence, v1.11 custom data, v1.15 "sanitized mount authority and trusted rebind metadata" | src/agents/run_state.py:179-231 |
| Provenance survives serialization | `_serialize_item` emits `input_id`, `source_agent`/`target_agent`, `tool_origin` (via `to_json_dict`), `custom_data`; `$schemaVersion` stamped in payload | src/agents/run_state.py:1767-1775,1951-1989; src/agents/tool.py:302-337 |
| Agent identity keys | `_serialize_agent_reference` adds disambiguating `identity` key for duplicate agent names | src/agents/run_state.py:2402-2412 |
| Trust classification (sandbox) | Closed tables map mount types to exact SDK classes; comment: "Class provenance keeps module reloads stable while ordinary custom subclasses cannot promote themselves into a trusted boundary" | src/agents/sandbox/_mount_security.py:146-257 |
| Provenance enforcement | `_mount_provenance_error` rejects custom mounts/strategies/patterns "at the sandbox credential boundary" without inspecting config values | src/agents/sandbox/_mount_security.py:1337-1370 |
| Redaction marker on serialize | Mount authority sanitized out of persisted state; `__openai_agents_redacted_mount_authority: True` marker recorded | src/agents/sandbox/session/sandbox_session_state.py:349-399; src/agents/sandbox/_mount_security.py:57-58 |
| Redacted state refuses resume | Deserializer rejects redacted/rebind-required state: "resume through Runner with the current trusted manifest" | src/agents/sandbox/session/sandbox_session_state.py:337-347 |
| Content digests | `sha256_file` / `sha256_io` used for artifact integrity in sandbox transfers | src/agents/sandbox/util/checksums.py:8-40 |
| Item occurrence digests | `digest_input_item` computes SHA-256 over canonicalized input-item fingerprints for exactly-once tracking | src/agents/run_internal/items.py:393-412 |
| Compaction transformation marker | `Compaction.process_context` truncates history at the last `compaction` item; `CompactionItem` carries no origin/summary metadata beyond position | src/agents/sandbox/capabilities/compaction.py:210-225; src/agents/items.py:532-540 |

## Answers to Dimension Questions

1. **Does each context item know where it came from?**
   Partially. Every `RunItem` knows its producing `agent` (src/agents/items.py:99), handoff items name both endpoints (src/agents/items.py:316-321), and function-tool-backed items can carry `ToolOrigin` identifying MCP server or parent agent (src/agents/tool.py:293-300). But this is runtime-side metadata about *which code path* produced the item — the substantive content of tool outputs (files fetched, web pages read) has no source annotation, URL, or retrieval record attached when replayed into model context.

2. **Is freshness tracked?**
   Only outside the core item stream. `RunItem`s and `ModelResponse` have no timestamp fields (src/agents/items.py:156-176). Session stores timestamp rows (src/agents/memory/sqlite_session.py:245-252) but `get_items` discards them (src/agents/memory/sqlite_session.py:315-323), so freshness degrades to implicit list order after one round trip. The sandbox memory subsystem is the exception: `updated_at` is written into every memory artifact (src/agents/sandbox/memory/manager.py:342-346), parsed back for recency-sorted selection (src/agents/sandbox/memory/storage.py:226-234), and elevated to a first-class conflict-resolution signal in the consolidation prompt.

3. **Is trust level indicated?**
   Not as a graded field on context content. Trust exists as a binary structural check at the sandbox credential boundary: mounts, strategies, and patterns must be exact SDK classes from closed tables, with custom subclasses explicitly unable to self-promote to trusted (src/agents/sandbox/_mount_security.py:193-257,1348-1370). Elsewhere, trust is procedural rather than annotated: memory prompts instruct models to treat rollout/tool output text as data, not instructions, and to redact secrets (src/agents/sandbox/memory/prompts/rollout_extraction_prompt.md:20-23). A tool output entering the conversation carries no field distinguishing authoritative from untrusted sources.

4. **Are transformations traceable?**
   Yes at the state boundary, partially elsewhere. `context_meta` records the exact serialization path taken by user context and whether a deserializer will be required to restore it (src/agents/run_state.py:1460-1583,2321-2340), tested in tests/test_run_state.py:920-1067. Serialized sandbox state records that mount authority was redacted via a durable marker and refuses resume until rebound from a trusted manifest (src/agents/sandbox/session/sandbox_session_state.py:337-390). The RunState wire format itself carries a versioned, assertion-enforced change log (src/agents/run_state.py:186-231). By contrast, LLM-driven transformations are weakly traceable: compaction truncates everything before a bare `CompactionItem` with no record of what was removed (src/agents/sandbox/capabilities/compaction.py:210-225), and memory summarization relies on prompt rules (traceability annotations, immutable rollouts) rather than enforced schemas — though the two-phase pipeline does keep the raw rollout files alongside derived summaries so the chain remains reconstructable manually.

## Architectural Decisions

- **Origin metadata lives on items, not content.** `tool_origin` describes the SDK mechanism that produced an item (src/agents/tool.py:285-300); nothing describes the external source of the output payload. This keeps the Responses-API wire contract untouched but caps what downstream consumers can verify.
- **Serialization honesty over silent fidelity loss.** Rather than pretending custom contexts survive JSON, `context_meta` records the transformation path and forces restore-time acknowledgment (`requires_deserializer`, `omitted`; src/agents/run_state.py:1460-1583). Fail-loud beats fail-wrong.
- **Structural trust via class provenance.** The sandbox boundary authenticates types, not configuration values — "Validate exact SDK class provenance without copying or inspecting configuration values" (src/agents/sandbox/_mount_security.py:1352). Closed tables prevent subclass spoofing even across module reloads.
- **Redact-then-block for durable credentials.** Authority fields are stripped from serialized state and their absence is itself recorded (`REDACTED_MOUNT_AUTHORITY_KEY`), converting a data-loss event into an explicit resume gate (src/agents/sandbox/session/sandbox_session_state.py:386-390).
- **Versioned state format with enforced changelog discipline.** Schema bumps require human-written summaries enforced by import-time assertions (src/agents/run_state.py:220-231), making format evolution auditable.
- **Prompt-borne provenance policy for model-written memory.** Where transformations are performed by an LLM, the SDK enforces provenance through strict prompt contracts (mandatory `rollout_id=<id>, updated_at=<timestamp>` annotations) rather than post-hoc validation.

## Notable Patterns

- **Header-line metadata in markdown artifacts**: memory files lead with machine-parseable `key: value` lines (src/agents/sandbox/memory/manager.py:341-347) so provenance survives in human-readable form; parsing tolerates missing values ("unknown") and sorts them last (src/agents/sandbox/memory/storage.py:226-234).
- **Dual-write provenance**: each rollout produces both a raw memory and a summary, each carrying overlapping headers plus cross-references (`rollout_summary_file`, `rollout_path`), keeping the derivation chain navigable (src/agents/sandbox/memory/manager.py:191-212).
- **Replay sanitization**: output-only server metadata (`created_by`) is systematically stripped before items re-enter model input, with nested-chunk handling for shell outputs (src/agents/items.py:222-256).
- **Side-channel attachment**: `custom_data` lets applications attach observability metadata to tool outputs with an explicit guarantee it is excluded from model-visible replays (src/agents/items.py:447-452).
- **Exactly-once occurrence identity**: stable SHA-256 fingerprints over canonicalized input items back dedup/tracking across resume (src/agents/run_internal/items.py:393-412).

## Tradeoffs

- **Wire-format compatibility vs provenance richness**: because items must round-trip through the Responses API schema, freshness/trust cannot ride on ordinary items; the SDK chose compatibility, leaving consumers with positional ordering only.
- **Human-readable provenance vs parseability**: markdown header lines are greppable and model-friendly but fragile under model rewriting — the consolidation prompt demands preservation, yet nothing structurally prevents a generated `MEMORY.md` from dropping annotations (mitigated only by phase-two selection tracking in src/agents/sandbox/memory/storage.py:162-199).
- **Security redaction vs resumability**: stripping mount authority makes snapshots safe to persist anywhere, at the cost of mandatory manual rebind before resume (src/agents/sandbox/session/sandbox_session_state.py:337-347).
- **Closed trust tables vs extensibility**: exact-class provenance closes the door on subclass injection but also on legitimate third-party mounts, which must use a narrow documented extension registry (src/agents/sandbox/_mount_security.py:163-191).

## Failure Modes / Edge Cases

- **Corrupt session rows silently skipped**: `get_items` drops undecodable JSON entries with no surfaced error or count (src/agents/memory/sqlite_session.py:303-308), so history can shrink invisibly.
- **Unknown freshness sinks priority**: memory selection treats missing/unknown `updated_at` as oldest (returns `(0, "")`, src/agents/sandbox/memory/storage.py:226-234); a malformed write permanently demotes otherwise-valid memories rather than flagging them.
- **Context type erasure on resume**: Pydantic/dataclass contexts serialize to plain dicts with only a logged warning; restoring without a deserializer yields a mapping-shaped impostor of the original context (src/agents/run_state.py:1527-1566).
- **Compaction information loss is unrecoverable in-band**: everything before the compaction marker is truncated from future context (src/agents/sandbox/capabilities/compaction.py:210-225); recovery depends on externally persisted sessions.
- **Model-reused call IDs rejected loudly**: invocation records fingerprint each provider call ID and raise `ModelBehaviorError` on identity mismatch (src/agents/run_context.py:304-355), protecting output-to-call attribution integrity.
- **Phase-one extraction failure = silent skip**: if rollout artifacts fail validation the memory is dropped without a quarantine record (src/agents/sandbox/memory/manager.py:169-175).

## Future Considerations

- Add optional `source_url`/`retrieved_at`/trust fields to `ToolCallOutputItem.custom_data` conventions (or a typed sibling of src/agents/items.py:447) so externally fetched content enters context with inspectable provenance.
- Surface storage-layer `created_at` in `Session.get_items` results (e.g., a parallel metadata channel) instead of discarding it (src/agents/memory/sqlite_session.py:288-323).
- Record compaction lineage — e.g., extend `CompactionItem` with the range and digest of compacted history (src/agents/items.py:532-540) — enabling audit of what the model no longer sees.
- Validate memory-file headers structurally at read time (schema-check the `key:` preamble in src/agents/sandbox/memory/storage.py:237-256) rather than relying solely on prompt compliance during consolidation.

## Questions / Gaps

- No evidence found of any mechanism attaching trust/authority levels to ordinary tool outputs or retrieved documents; searches for `trust`, `authority_level`, and `provenance` across `src/agents` returned only sandbox-mount security code and none in the item/session layers (search scope: `src/`, `tests/`, `docs/`).
- No evidence found of freshness propagation from session storage back into model-facing context; all read paths return bare payloads.
- The tracing subsystem identifies runs (`trace_id`, `group_id`, src/agents/tracing/traces.py:199-260) but no linkage from a conversation item back to its originating trace span was found, so "where did this context come from" cannot be answered post-hoc for arbitrary items.
- Whether the memory prompts' provenance rules are enforced by any validator could not be confirmed; only selection bookkeeping is tested (`test_phase_two_selection_tracks_added_retained_and_removed_rollouts`, tests/sandbox/test_memory.py:602; `_updated_at_sort_key` ordering, tests/sandbox/test_memory.py:593).

---

Generated by Dimension 11.04 (Context Provenance and Integrity) against `openai-agents-sdk`.
