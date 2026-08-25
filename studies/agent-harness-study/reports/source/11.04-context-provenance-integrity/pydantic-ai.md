# Source Analysis: pydantic-ai

## Dimension 11.04: Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, dataclasses; provider-agnostic GenAI agent framework) |
| Analyzed | 2026-08-25 |

> Path convention: all cited file paths are relative to the selected source root `studies/agent-harness-study/sources/pydantic-ai/`.

## Summary

Pydantic AI treats provenance as a first-class property of its message model rather than an afterthought. Every message part carries a `timestamp`; tool returns carry `tool_name` + `tool_call_id` + `outcome`; every model-produced part and message carries `provider_name` / `provider_details`, and messages are stamped with `run_id` and `conversation_id` at production time (`pydantic_ai_slim/pydantic_ai/_utils.py:560`). File-like content items carry stable content-derived identifiers (`pydantic_ai_slim/pydantic_ai/messages.py:209`) or explicit source URLs/file IDs (`pydantic_ai_slim/pydantic_ai/messages.py:220`, `pydantic_ai_slim/pydantic_ai/messages.py:826`). Provenance survives serialization through a single typed adapter (`ModelMessagesTypeAdapter`, `pydantic_ai_slim/pydantic_ai/messages.py:2768`) that preserves even application-only metadata fields, verified by dedicated round-trip tests (`tests/test_messages.py:910`).

Trust is handled explicitly but *positionally*, not per-item: there is no `trust_level` field on context items. Instead the framework draws a documented trust boundary between trusted server-side history and untrusted client-supplied history, with an operational safeguard (`sanitize_messages`, `pydantic_ai_slim/pydantic_ai/messages.py:2953`) that strips system prompts, non-HTTP file URL schemes, uploaded-file references, dangling tool calls, and compaction provenance stamps from untrusted input. Transformations are traceable where the framework itself transforms history — synthesized repair returns are marked in `metadata` (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2702`), compaction summaries record their producing provider (`pydantic_ai_slim/pydantic_ai/messages.py:2015`), and a private stamp distinguishes self-minted compaction items from external ones (`STANDING_PROMPT_PLANTED_KEY`, `pydantic_ai_slim/pydantic_ai/messages.py:1979`) — but user-supplied history processors can rewrite history arbitrarily without leaving any transformation record.

Overall: freshness is comprehensive, source-of-artifact annotation is strong for framework-generated and file artifacts, transformation traceability is good for framework-owned transformations and absent for user-owned ones, and trust is modeled as channel-level (server vs client) rather than item-level. This is a clear, tested model with operational safeguards — solidly in the 7–8 band of the rubric, held back from 9–10 by the absence of per-item trust/source annotations on arbitrary retrieved context and by unlogged user-defined history transformations.

## Rating

**7/10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Present and explicit**: timestamps on all parts/messages (`pydantic_ai_slim/pydantic_ai/messages.py:191`, `:1095`, `:1332`, `:1670`, `:1842`, `:2559`), provider/run/conversation attribution (`pydantic_ai_slim/pydantic_ai/messages.py:2569`, `:2592`), outcome taxonomy on tool returns (`pydantic_ai_slim/pydantic_ai/messages.py:1335`).
- **Tested**: serialization round-trips for metadata-bearing parts (`tests/test_messages.py:910`), uploaded files (`tests/test_messages.py:1355`), speech parts (`tests/test_messages.py:2407`), and legacy-history compatibility (`tests/test_messages.py:900`).
- **Operational safeguards**: `sanitize_messages` + UI-adapter trust model with warnings on every strip action (`pydantic_ai_slim/pydantic_ai/messages.py:3077`–`:3133`); documented trust boundary (`docs/message-history.md:404`–`:417`).
- **Why not higher**: no per-item trust/authority annotation (trust is inferred from which channel delivered the message); no generic structured "source reference" field for arbitrary retrieved text snippets (apps must repurpose `TextContent.metadata` / `ToolReturn.metadata`, both typed `Any`); user history processors leave no transformation log; timestamps exist but nothing in the core consumes them for staleness/TTL decisions.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Freshness — part timestamps | `SystemPromptPart.timestamp`, default `_now_utc()` | `pydantic_ai_slim/pydantic_ai/messages.py:191` |
| Freshness — user prompt timestamp | `UserPromptPart.timestamp` | `pydantic_ai_slim/pydantic_ai/messages.py:1095` |
| Freshness — tool return timestamp | `BaseToolReturnPart.timestamp` ("when the tool returned") | `pydantic_ai_slim/pydantic_ai/messages.py:1332` |
| Freshness — request/response timestamps | `ModelRequest.timestamp` (None-default so deserialized history isn't re-stamped) and `ModelResponse.timestamp` (local clock; provider ts in `provider_details['timestamp']`) | `pydantic_ai_slim/pydantic_ai/messages.py:1840`, `pydantic_ai_slim/pydantic_ai/messages.py:2559` |
| Source — tool identity linkage | `tool_name` + generated/preserved `tool_call_id` pair call and result | `pydantic_ai_slim/pydantic_ai/messages.py:1302`, `pydantic_ai_slim/pydantic_ai/messages.py:1308` |
| Source — run/conversation correlation | `run_id` / `conversation_id` stamped onto requests and responses | `pydantic_ai_slim/pydantic_ai/messages.py:1851`, `pydantic_ai_slim/pydantic_ai/messages.py:2592` |
| Source — producer attribution | `provider_name`, `provider_url`, `provider_response_id`, `model_name` on responses; per-part `provider_name` required whenever `id`/`signature`/`provider_details` set | `pydantic_ai_slim/pydantic_ai/messages.py:2556`–`:2587`, `pydantic_ai_slim/pydantic_ai/messages.py:1904`, `pydantic_ai_slim/pydantic_ai/messages.py:1955` |
| Source — origin of dynamic prompts/instructions | `SystemPromptPart.dynamic_ref` (ref of generating function); `InstructionPart.dynamic` static/dynamic split for cache-aware ordering | `pydantic_ai_slim/pydantic_ai/messages.py:194`, `pydantic_ai_slim/pydantic_ai/messages.py:1766` |
| Source — file artifact references | `FileUrl.url`; `UploadedFile.file_id` + mandatory `provider_name` (non-portable IDs) | `pydantic_ai_slim/pydantic_ai/messages.py:220`, `pydantic_ai_slim/pydantic_ai/messages.py:826` |
| Content integrity identifiers | SHA1-based stable `identifier` computed field for multimodal items, used by tools to look up files in history | `pydantic_ai_slim/pydantic_ai/messages.py:209`, `pydantic_ai_slim/pydantic_ai/messages.py:274` |
| Trust level — boundary sanitization | `sanitize_messages` strips system prompts, non-HTTP schemes, force-download escalation, uploaded files, dangling calls, compaction stamps from untrusted input | `pydantic_ai_slim/pydantic_ai/messages.py:2953`–`:3135` |
| Trust level — compaction stamp | `STANDING_PROMPT_PLANTED_KEY`: only self-stamped compaction parts are trusted to retain the standing system prompt | `pydantic_ai_slim/pydantic_ai/messages.py:1979`, `pydantic_ai/models/openai.py:4965` |
| Trust level — wire-boundary validation | `parse_tool_kind` degrades unknown client-supplied `tool_kind` to `None` instead of asserting a bogus discriminator | `pydantic_ai_slim/pydantic_ai/messages.py:1282` |
| Trust level — documented boundary | Trust model doc: possession of endpoint is the authorization boundary; no signing/verification by design | `docs/message-history.md:404`–`:417` |
| Transformation log — synthesized returns | `SYNTHESIZED_TOOL_RETURN_METADATA_KEY = 'pydantic_ai_synthesized_tool_return'` marks repaired histories; repair is deterministic/idempotent | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2702`, `docs/message-history.md:245`–`:262` |
| Transformation log — compaction | `CompactionPart.provider_name` + `provider_details['encrypted_content']`; provider-exact window logic decides whether a summary is renderable/honored | `pydantic_ai_slim/pydantic_ai/messages.py:1988`–`:2036`, `pydantic_ai_slim/pydantic_ai/messages.py:2817` |
| Transformation log — malformed args | `INVALID_JSON_KEY` wrapper keeps raw malformed tool args round-trippable instead of dropping them | `pydantic_ai_slim/pydantic_ai/messages.py:35`, `pydantic_ai_slim/pydantic_ai/messages.py:2225` |
| Outcome taxonomy | `outcome: 'success' \| 'failed' \| 'denied' \| 'interrupted'` on every tool return | `pydantic_ai_slim/pydantic_ai/messages.py:1335` |
| Serialization survival | `ModelMessagesTypeAdapter` preserves every field incl. app-only `TextContent.metadata`/`ToolReturn.metadata`; docstring notes UI adapters drop them by design | `pydantic_ai_slim/pydantic_ai/messages.py:2768`, `pydantic_ai_slim/pydantic_ai/messages.py:526` |
| Serialization tests | `test_model_messages_type_adapter_preserves_user_text_prompt_metadata`; pre-v2-history compatibility test asserting missing `conversation_id` deserializes as None | `tests/test_messages.py:910`, `tests/test_messages.py:900` |
| Stamping mechanism | `fill_run_metadata` fills only unset `timestamp`/`run_id`/`conversation_id`, preserving producer-supplied values | `pydantic_ai_slim/pydantic_ai/_utils.py:560` |
| ID resolution rules | `resolve_run_id` never inherits from history and rejects reuse (breaks `new_messages()` boundary detection); `resolve_conversation_id` inherits most recent | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:264`, `pydantic_ai_slim/pydantic_ai/_agent_graph.py:237` |

## Answers to Dimension Questions

**1. Does each context item know where it came from?**
Largely yes, with a type-shaped answer. Framework-generated items carry precise origin: tool returns know their `tool_name`/`tool_call_id` (`pydantic_ai_slim/pydantic_ai/messages.py:1302`); model output knows its `provider_name`, `provider_url`, and `provider_response_id` (`pydantic_ai_slim/pydantic_ai/messages.py:2569`–`:2587`); thinking parts know their provider-scoped `signature` (`pydantic_ai_slim/pydantic_ai/messages.py:1942`); instructions know whether they were static or dynamically generated (`InstructionPart.dynamic`, `pydantic_ai_slim/pydantic_ai/messages.py:1766`; `SystemPromptPart.dynamic_ref`, `:194`); files know their URL, file ID, and owning provider (`pydantic_ai_slim/pydantic_ai/messages.py:220`, `:826`). What does *not* exist is a universal structured `source` slot on arbitrary text context: a document pasted into a user prompt or returned as plain tool text has no first-class origin field. The escape hatch is application-only metadata (`TextContent.metadata`, `ToolReturn.metadata`, both typed `Any` — `pydantic_ai_slim/pydantic_ai/messages.py:526`, `:992`), which is preserved by `ModelMessagesTypeAdapter` but not sent to the LLM and not guaranteed to survive UI adapters (`docs/message-history.md:369`–`:379`). Run/conversation IDs additionally attribute each message to its producing run (`pydantic_ai_slim/pydantic_ai/_utils.py:560`).

**2. Is freshness tracked?**
Yes, comprehensively. Every part kind has a `timestamp` defaulting to `_now_utc()` at construction (`pydantic_ai_slim/pydantic_ai/messages.py:191`, `:1095`, `:1332`, `:1670`), messages carry their own timestamps (`:1842`, `:2559`), and `ModelResponse.timestamp` explicitly documents that it is local-receipt time while provider-side timestamps live in `provider_details['timestamp']` (`:2559`). The design also avoids a classic staleness bug: `ModelRequest.timestamp` defaults to `None` precisely so deserialized historical messages aren't falsely re-stamped with load time (`pydantic_ai_slim/pydantic_ai/messages.py:1840`), and `fill_run_metadata` fills only unset fields (`pydantic_ai_slim/pydantic_ai/_utils.py:561`). Limitation: nothing in the core consumes these timestamps for TTL/staleness decisions — freshness is recorded, not acted upon.

**3. Is trust level indicated?**
Not as a per-item annotation. There is no `trust`/`authority` field anywhere in the message schema (search across `pydantic_ai_slim/pydantic_ai/` for `trust|authority` yields only documentation and sanitization code). Instead, trust is modeled at the *channel* level: `message_history` passed server-side is trusted state, while anything from a browser/UI adapter is untrusted and must pass `sanitize_messages` (`docs/message-history.md:381`–`:402`, `:404`–`:417`). The closest things to item-level trust markers are: (a) the compaction provenance stamp `STANDING_PROMPT_PLANTED_KEY`, where only framework-minted items are trusted to have retained the standing system prompt (`pydantic_ai_slim/pydantic_ai/messages.py:1979`–`:1985`); (b) `sanitize_messages` stripping exactly the parts whose authority could be abused (system prompts, `s3://`/`gs://` URLs fetched with server IAM, uploaded files, unresolved tail tool calls) with a warning per stripped category (`pydantic_ai_slim/pydantic_ai/messages.py:3077`–`:3133`); and (c) capability reveal gating, which explicitly states it is "not a trust boundary" — fabricated-but-coherent history is honored, impossible history is rejected (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2437`–`:2443`).

**4. Are transformations traceable?**
Framework-owned transformations, yes; user-owned transformations, no. Synthesized tool returns inserted by history repair are marked via `metadata['pydantic_ai_synthesized_tool_return']` and carry `outcome='interrupted'` so consumers can distinguish repair artifacts from real results (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2702`; `docs/message-history.md:253`). Compaction is traceable to its producing provider, including whether the summary is plaintext or encrypted opaque state (`CompactionPart`, `pydantic_ai_slim/pydantic_ai/messages.py:1988`–`:2036`), and `post_compaction_window` derives downstream state from that boundary at part-level precision (`:2774`). Malformed tool arguments are preserved verbatim under `INVALID_JSON` wrapping rather than silently normalized (`pydantic_ai_slim/pydantic_ai/messages.py:2225`). However, `ProcessHistory`/history processors replace the history wholesale with no obligation (or mechanism) to log what was dropped or rewritten (`pydantic_ai_slim/pydantic_ai/_history_processor.py:17`–`:26`; `docs/message-history.md:701`–`:709` warns only about preserving tool-pairing markers). Redaction exists only at the telemetry layer, not in stored history. There is no general transformation ledger.

## Architectural Decisions

1. **Provenance lives in typed message parts, not side tables.** Every datum travels inside a discriminated-union part hierarchy (`ModelRequestPart`/`ModelResponsePart`, `pydantic_ai_slim/pydantic_ai/messages.py:2470`, `:2520`) so origin/timestamp/outcome fields are validated and serialized with the content itself.
2. **Producer-supplied provenance wins over framework stamping.** `fill_run_metadata` overwrites nothing that is already set (`pydantic_ai_slim/pydantic_ai/_utils.py:561`–`:569`), and `ModelRequest.timestamp` defaults to `None` rather than "now" to avoid minting fake freshness for old history (`pydantic_ai_slim/pydantic_ai/messages.py:1840`–`:1843`).
3. **Trust is a channel property enforced by sanitization, not an item annotation.** The docs make this a stated design boundary, including why cryptographic signing of history is out of scope (`docs/message-history.md:404`–`:417`).
4. **Self-produced transformations carry self-verifying stamps.** Only compaction parts stamped `STANDING_PROMPT_PLANTED_KEY` by the framework's own compact call take the fast path that skips re-sending the system prompt; externally supplied or spliced items get the prompt re-inserted (`pydantic_ai_slim/pydantic_ai/messages.py:1979`–`:1985`, applied at `pydantic_ai/models/__init__.py:2100`–`:2125`).
5. **One canonical serializer defines what survives.** `ModelMessagesTypeAdapter` is the persistence boundary; lossy surfaces (UI wire protocols) are explicitly declared lossy by design rather than pretending fidelity (`docs/message-history.md:369`–`:379`).
6. **Identity discipline around `run_id`.** It is never inherited from history, must be unique within it, and empty/duplicate values raise `UserError` because they break `new_messages()` boundary detection (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:264`–`:295`).

## Notable Patterns

- **Discriminator-driven typed promotion**: parts deserialize into specialized subclasses via `(part_kind, tool_kind)` registries, and unsubstantiated `tool_kind`s from untrusted input are stripped rather than trusted (`_narrow_call`/`_narrow_return`, `pydantic_ai_slim/pydantic_ai/messages.py:2371`–`:2402`; `parse_tool_kind`, `:1282`).
- **Content-addressed identifiers**: multimodal items get a stable short SHA1 identifier derived from bytes/URL when none is supplied, enabling models to reference files across turns (`pydantic_ai_slim/pydantic_ai/messages.py:209`–`:213`, computed at `:289`, `:652`).
- **Warning-per-sanitization observability**: each category stripped from untrusted input emits a targeted `UserWarning` naming the offending values and how to allow them deliberately (`pydantic_ai_slim/pydantic_ai/messages.py:3085`–`:3133`).
- **Conservative-intersection windows**: `post_compaction_window` uses a provider-agnostic compaction boundary for run-state derivation while `_post_compaction_window_for_response` uses a provider-exact one for dispatch evidence, erring toward refusing rather than admitting unseen state (`pydantic_ai_slim/pydantic_ai/messages.py:2774`–`:2814`, `:2848`–`:2875`).

## Tradeoffs

- **Channel-trust vs item-trust**: positional trust is simpler and honest about what a stateless server can verify, but means a malicious client's fabricated `ToolReturnPart` looks identical to a genuine one once past the sanitizer — mitigations are scoped toolsets and server-side re-validation, pushed to the deployer (`docs/message-history.md:410`–`:414`).
- **Fidelity vs portability**: keeping `metadata: Any` maximizes preservation through the JSON TypeAdapter but forfeits structure (JSON round-trip normalizes tuples→lists, datetimes→ISO strings) and guarantees nothing across UI adapters (`docs/message-history.md:369`–`:375`).
- **Repair transparency vs history purity**: auto-synthesizing interrupted returns keeps providers happy and is idempotent/cache-safe, but adds synthetic parts into history that downstream consumers must filter via the metadata key (`docs/message-history.md:253`, `:258`).
- **Timestamps-everywhere vs staleness-action**: recording time uniformly costs little and aids debugging/cost calculation (`ModelResponse.cost` uses the timestamp, `pydantic_ai_slim/pydantic_ai/messages.py:2688`–`:2700`), yet no consumer implements expiry, so stale context ages invisibly.

## Failure Modes / Edge Cases

- **Client-forged history**: a client can fabricate tool calls/results/approvals; the framework processes them as genuine up to executing named tools. Documented as designed behavior, not a vulnerability (`docs/message-history.md:406`–`:417`).
- **Opaque compaction replay**: a client can replay any compaction item ever produced by the server's provider account; the server cannot inspect encrypted OpenAI compaction blobs (`docs/capabilities/compaction.md:26`).
- **Cross-provider compaction confusion**: a compaction part only bounds history for its producing provider; foreign-provider parts are ignored on the wire but still count in conservative windows, with known disagreement cases tracked upstream (`pydantic_ai_slim/pydantic_ai/messages.py:2817`–`:2845`).
- **Metadata loss at UI boundaries**: application-only `metadata` fields vanish through Vercel/AG-UI adapters by design; apps relying on them for provenance must persist via `ModelMessagesTypeAdapter` (`docs/message-history.md:377`–`:379`).
- **In-place mutation defeats telemetry**: mutating a history message in place is undetectable until end-of-run, when `MessageHistoryMutatedWarning` fires — recorded provenance can go stale relative to mutated objects (`docs/message-history.md:571`).
- **Legacy history tolerance**: pre-v2 serialized messages lacking `conversation_id` deserialize cleanly with `None`, and deprecated aliases (`vendor_details`, `vendor_id`) still validate — provenance schema evolution is backward-compatible (`tests/test_messages.py:900`; `pydantic_ai_slim/pydantic_ai/messages.py:2575`–`:2586`).

## Future Considerations

- A structured, optional `source` annotation on `TextContent`/tool-return text (typed, not `Any`) would let retrieval pipelines attach origin/authority without custom conventions.
- An optional transformation ledger (e.g., a `ProcessHistory` hook contract that records dropped/summarized ranges) would close the main traceability gap for user-owned compaction.
- Consuming existing timestamps for TTL/staleness policy (e.g., marking tool returns older than N as candidates for refresh) would convert recorded freshness into enforced freshness.
- Per-item trust labels remain unnecessary given the documented channel model, but a machine-readable marker distinguishing server-persisted from client-submitted segments would let mixed runs reason about which prefix was sanitized (`docs/message-history.md:387` describes the combination rule; no marker exists today).

## Questions / Gaps

- No evidence found of any per-item trust/authority scoring (searched `pydantic_ai_slim/pydantic_ai/` for `trust|untrusted|authority|credib`; all hits are sanitization code, docs, or skill guidance — e.g., `pydantic_ai_slim/pydantic_ai/messages.py:2963`, `docs/message-history.md:404`).
- No evidence found of freshness-based eviction/TTL logic in the core (timestamps are written and exposed; searched for consumers acting on part age — only cost calculation and OTel attributes read `response.timestamp`, e.g. `pydantic_ai_slim/pydantic_ai/_instrumentation.py:430`).
- No evidence found of a transformation log for `ProcessHistory`/history processors (`pydantic_ai_slim/pydantic_ai/_history_processor.py:11`–`:26` defines bare callable types; `docs/message-history.md:701`–`:709` warns about consequences of dropping parts but records nothing).
- Retrieval-specific provenance (chunk → document citation) is delegated to applications; embeddings docs recommend keeping "source metadata ... with every chunk" as guidance, not framework machinery (`docs/embeddings.md:129`).

---

Generated by `Dimension 11.04: Context Provenance and Integrity` against `pydantic-ai`.
