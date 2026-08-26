# Source Analysis: langgraph

## Dimension 11.04: Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core + checkpoint/store libs), JS/TS SDK (stub only) |
| Analyzed | 2026-08-26 |

## Summary

LangGraph approaches context provenance at two distinct layers rather than through a single per-item provenance record. The **checkpoint layer** carries a first-class, well-tested provenance model: every state snapshot is annotated with a machine-set `source` discriminator (`input`/`loop`/`update`/`fork`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:41`), a step counter (`:49`), parent-checkpoint lineage (`:56`), the originating run id (`:61`), and an ISO-8601 timestamp (`:102-103`) whose checkpoint ID itself is a monotonic uuid6 — freshness is encoded into identity (`libs/checkpoint/langgraph/checkpoint/base/id.py:92-98`). The **store (long-term memory) layer** provides weaker, timestamp-only provenance: `Item` records carry `created_at`/`updated_at` and a hierarchical namespace (`libs/checkpoint/langgraph/store/base/__init__.py:51-115`) plus TTL machinery, but no source or trust fields.

Trust is handled not as a per-item annotation but as a **system-level integrity gate**: deserialization of anything from persisted state passes through allowlisted type revival with strict-mode blocking, audit events, and optional encryption (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:85-95`, `:559-609`; `libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:17-36`). Transformation traceability exists for state reconstruction — `DeltaChannel` persists its full write history and replays it deterministically (`libs/langgraph/langgraph/channels/delta.py:25-63`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`) — and serialization envelopes preserve class identity `(module, name)` for every object (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:305-534`). However, there is no transformation log for content-level changes (summarization/redaction); trace-only transforms are explicitly non-mutating and marked unsuitable for redaction (`libs/langgraph/langgraph/types.py:540-558`). Provenance survives serialization durably: metadata persists as JSONB in Postgres (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:342`), and store items round-trip their timestamps through msgpack in tests (`libs/checkpoint/tests/test_jsonplus.py:147-153`, `:187-190`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

The checkpoint provenance model alone would score 8+: explicit `source` literal types, parent lineage, monotonic IDs, timestamps, and extensive test coverage asserting these values (`libs/langgraph/tests/test_pregel.py:1519-1560`, `libs/langgraph/tests/test_time_travel.py:2612-2661`). The score is held at 7 because (a) there is no per-context-item trust/authority level anywhere; (b) content-level transformations (summarization/redaction) are not recorded; (c) freshness tracking is inconsistent between store implementations — `InMemoryStore` resets `created_at` on every put while Postgres preserves it (`libs/checkpoint/langgraph/store/memory/__init__.py:410-416` vs `libs/checkpoint-postgres/langgraph/store/postgres/base.py:404-408`); and (d) integrity degradation on blocked revival silently returns raw data or `None` (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:645-653`, `:704-710`).

## Evidence Collected

Every entry includes a file path with line numbers relative to `studies/agent-harness-study/sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source annotations | `CheckpointMetadata.source: Literal["input", "loop", "update", "fork"]` discriminates how each snapshot was created | libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-48 |
| Source population (loop) | Loop writes checkpoints tagged `{"source": "loop"}` after each superstep | libs/langgraph/langgraph/pregel/_loop.py:718 |
| Source population (input) | Input application writes `{"source": "input"}` checkpoints | libs/langgraph/langgraph/pregel/_loop.py:1033 |
| Source population (fork) | Time-travel replay forks write `{"source": "fork"}` to avoid corrupting the parent chain | libs/langgraph/langgraph/pregel/_loop.py:952-971 |
| Source population (update/fork/copy) | `bulk_update_state` tags manual updates `"update"`/`"input"`/`"fork"` with inherited `parents` | libs/langgraph/langgraph/pregel/main.py:1732-1736, :1771-1777, :1818-1822 |
| Lineage | `parents: dict[str, str]` maps checkpoint namespace → parent checkpoint ID; populated from `CONFIG_KEY_CHECKPOINT_MAP` | libs/checkpoint/langgraph/checkpoint/base/__init__.py:56-60; libs/langgraph/langgraph/pregel/_loop.py:1126 |
| Step/run attribution | Metadata enriched with `step` before persisting; `run_id` merged from run config by `get_checkpoint_metadata` | libs/langgraph/langgraph/pregel/_loop.py:1125; libs/checkpoint/langgraph/checkpoint/base/__init__.py:757-775 |
| Run re-entry detection | Same-`run_id` comparison against checkpoint metadata distinguishes resume from fresh input | libs/langgraph/langgraph/pregel/_loop.py:858-871 |
| Task provenance metadata | Every task's config carries `langgraph_step`, `langgraph_node`, `langgraph_triggers`, `langgraph_path`, `langgraph_checkpoint_ns` | libs/langgraph/langgraph/pregel/_algo.py:654-660 |
| Write attribution | `put_writes(config, writes, task_id, task_path)` links intermediate writes to the producing task | libs/checkpoint/langgraph/checkpoint/base/__init__.py:300-318 |
| Freshness: timestamps | `Checkpoint.ts` ISO 8601 UTC set in `create_checkpoint`/`empty_checkpoint` | libs/checkpoint/langgraph/checkpoint/base/__init__.py:102-103, :814-826, :829-860 |
| Freshness: monotonic IDs | Checkpoint IDs are uuid6 with strictly-increasing embedded timestamps (`timestamp <= last → +1`) | libs/checkpoint/langgraph/checkpoint/base/id.py:92-98; used at libs/checkpoint/langgraph/checkpoint/base/__init__.py:819, :854 |
| Freshness: channel versions | `channel_versions` / `versions_seen` track per-channel monotonically increasing versions and which node saw them | libs/checkpoint/langgraph/checkpoint/base/__init__.py:109-120 |
| Freshness: store items | `Item.created_at`/`updated_at` (accepts ISO strings on deserialize); surfaced via `Item.dict()` | libs/checkpoint/langgraph/store/base/__init__.py:51-115 |
| Freshness: TTL | `PutOp.ttl` (minutes, refreshes on read/write), `TTLConfig` (`refresh_on_read`, `omit_expired`, `default_ttl`, sweep), `GetOp.refresh_ttl` | libs/checkpoint/langgraph/store/base/__init__.py:526-534, :545-575, :194-200 |
| Freshness inconsistency | InMemoryStore stamps `created_at=datetime.now()` on *every* put, destroying creation time on update | libs/checkpoint/langgraph/store/memory/__init__.py:404-416 |
| Freshness preserved (Postgres) | Upsert updates `value, updated_at, expires_at, ttl_minutes` but leaves `created_at` untouched | libs/checkpoint-postgres/langgraph/store/postgres/base.py:400-409 |
| Relevance signal | `SearchItem.score` attaches similarity score to search results (quality ranking, not trust) | libs/checkpoint/langgraph/store/base/__init__.py:118-154 |
| Trust: strict serde | Security note; `LANGGRAPH_STRICT_MSGPACK=true` restricts revival to `SAFE_MSGPACK_TYPES`; explicit `allowed_msgpack_modules` constructor arg | libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:85-95, :107-119 |
| Trust: JSON allowlist gate | lc:2 constructor revival refuses non-allowlisted modules with actionable error; no prefix wildcards allowed | libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:160-179, :223-256 |
| Trust: msgpack ext hook | `_check_allowed` gates each ext payload's `(module, name)`; blocked types emit events and warn once | libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:559-609 |
| Trust: audit events | `SerdeEvent` listener registry (`msgpack_unregistered_allowed`, `msgpack_blocked`, `msgpack_method_blocked`) isolates listener failures | libs/checkpoint/langgraph/checkpoint/serde/event_hooks.py:13-52 |
| Trust: encryption | `EncryptedSerializer` wraps serde, encodes cipher name in type string (`"{typ}+{ciphername}"`) | libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:17-36 |
| Trust: anti-forgery | ToolNode strips caller-supplied values for injected args and re-adds only trusted runtime values, preventing LLM forgery of hidden `InjectedToolArg`s | libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1429 |
| Transformation: delta history | `DeltaChannel` stores sentinel blobs; full write history persisted and replayed deterministically (`reducer(state, xs+ys)` associativity contract) | libs/langgraph/langgraph/channels/delta.py:25-63, :139-157 |
| Transformation: ancestor walk | `get_delta_channel_history` walks parent chain accumulating per-channel writes until a seed value/snapshot | libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649 |
| Transformation: snapshot counters | `counters_since_delta_snapshot` metadata tracks (updates, supersteps) per channel; snapshot forced at frequency or 5000-superstep bound | libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-86 |
| Transformation: type envelopes | Every serialized object embeds `(module, ClassName)` plus construction args in msgpack ext payloads (pydantic v1/v2, dataclass, namedtuple, datetime, Send, Item…) | libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-302, :305-534 |
| Transformation: trace-only | `TracePolicy.process_inputs/process_outputs` transform what tracing records ("omit or summarize large payloads") but explicitly do not affect execution and are "not intended to redact secrets" across runs | libs/langgraph/langgraph/types.py:535-567; libs/langgraph/langgraph/graph/state.py:700-703 |
| Message identity | `add_messages` assigns UUIDs to messages missing ids and merges/replaces by id (append-unless-same-id semantics) | libs/langgraph/langgraph/graph/message.py:202-234 |
| Message stream provenance | Streamed messages emitted tagged with subgraph namespace + run metadata tuple `(ns, metadata)` derived from `langgraph_checkpoint_ns` | libs/langgraph/langgraph/pregel/_messages.py:97-104, :141-149 |
| Serialization survival: metadata as JSONB | Postgres saver stores `get_serializable_checkpoint_metadata(...)` as JSONB column | libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:342 |
| Serialization survival: Item round-trip | Tests serialize/deserialize `Item` with `created_at`/`updated_at` via msgpack and JSON mode (timestamps preserved as ISO strings) | libs/checkpoint/tests/test_jsonplus.py:147-153, :187-190, :313-314 |
| Serialization survival: format versioning | `Checkpoint.v` version field (currently 1/`LATEST_VERSION=2`) marks on-disk format evolution | libs/checkpoint/langgraph/checkpoint/base/__init__.py:92-96, :811 |
| Metadata key hygiene | `EXCLUDED_METADATA_KEYS` prevents config keys (`thread_id`, `langgraph_*`, …) from being double-persisted; NUL chars stripped from strings | libs/checkpoint/langgraph/checkpoint/base/__init__.py:761-775, :797-807 |
| Tests: source assertions | History tests assert exact `Counter(metadata["source"])` distributions and fork/update discrimination | libs/langgraph/tests/test_pregel.py:1519-1560; libs/langgraph/tests/test_time_travel.py:2612-2661; libs/langgraph/tests/test_delta_channel_update_state.py:337 |
| Tests: TTL behavior | Postgres store TTL tests verify expiry, refresh-on-read only refreshing live rows, omit-expired semantics | libs/checkpoint-postgres/tests/test_store.py:967-1087 |

## Answers to Dimension Questions

### 1. Does each context item know where it came from?

**Yes at the state/checkpoint granularity, partially at item granularity.**

- Every checkpoint knows its origin through `source` ∈ {`input`, `loop`, `update`, `fork`} (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:41-48`), the writing run (`run_id`, `:61-62`), and its full ancestry (`parents`, `:56-60`). The four values are set mechanically at exactly four call sites (`libs/langgraph/langgraph/pregel/_loop.py:718`, `:971`, `:1033`; `libs/langgraph/langgraph/pregel/main.py:1733`) — not user-forgeable in normal flow.
- Every task execution carries `langgraph_node`/`langgraph_triggers`/`langgraph_path`/`langgraph_checkpoint_ns` metadata identifying the producing node and call path (`libs/langgraph/langgraph/pregel/_algo.py:654-660`), and intermediate writes are attributed to a `task_id` via `put_writes` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:300-318`).
- Store items know their hierarchical namespace but nothing about which agent/run wrote them: `PutOp` has namespace/key/value/index/ttl and no writer field (`libs/checkpoint/langgraph/store/base/__init__.py:431-534`).
- Individual messages get a stable UUID identity that enables overwrite-by-id merging (`libs/langgraph/langgraph/graph/message.py:202-234`) — identity, but not origin (role comes from langchain-core message types, not LangGraph).

### 2. Is freshness tracked?

**Yes, strongly for state; inconsistently for memory.**

- Checkpoints stamp ISO-8601 UTC `ts` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:102-103, :839`) and use uuid6 IDs whose embedded timestamp is guaranteed strictly increasing within the process (`libs/checkpoint/langgraph/checkpoint/base/id.py:96-98`), making checkpoint ordering time-verifiable.
- Channel-level staleness is tracked structurally: `channel_versions` and `versions_seen` let the loop determine which nodes have seen which channel state (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:109-120`).
- Store items expose `created_at`/`updated_at` and a complete TTL subsystem with read-refresh and expiry filtering (`libs/checkpoint/langgraph/store/base/__init__.py:545-575`), verified by Postgres integration tests (`libs/checkpoint-postgres/tests/test_store.py:967-1087`).
- **Inconsistency**: `InMemoryStore._apply_put_ops` recreates the `Item` on every put with both timestamps set to now (`libs/checkpoint/langgraph/store/memory/__init__.py:410-416`), so dev/testing environments report corrupted creation times while Postgres preserves `created_at` through upserts (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:404-408`).

### 3. Is trust level indicated?

**No per-item trust annotation exists; trust is enforced as a system-level integrity boundary instead.**

- A repo-wide search for `trust|authority|credib|confiden` produced no per-item trust fields in any context/state/store schema; the only semantic hit is anti-forgery logic in ToolNode.
- The real mechanism is deserialization gating: `JsonPlusSerializer` documents that checkpoint data must not be treated as trusted (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:85-95`), and both the JSON lc:2 reviver (`:223-256`) and msgpack ext hook (`:559-609`) refuse types outside `SAFE_MSGPACK_TYPES`/explicit allowlists unless `LANGGRAPH_STRICT_MSGPACK` is relaxed or modules are explicitly allowed. Decisions are observable through the `SerdeEvent` audit hook (`libs/checkpoint/langgraph/checkpoint/serde/event_hooks.py:41-52`).
- Confidentiality is optionally layered via `EncryptedSerializer` with cipher negotiation in the type tag (`libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:17-36`).
- One genuine per-interaction trust control: ToolNode strips LLM-supplied arguments for injected parameters and substitutes framework-computed values, so a model cannot forge hidden inputs (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1421-1429`).

### 4. Are transformations traceable?

**For state reconstruction, yes — unusually well. For content transformations, no.**

- `DeltaChannel` makes the write history itself durable: non-snapshot steps persist only a sentinel, and reconstruction deterministically replays accumulated ancestor writes fetched by `get_delta_channel_history`, terminating at a `_DeltaSnapshot` seed (`libs/langgraph/langgraph/channels/delta.py:139-157`; `libs/checkpoint/langgraph/checkpoint/base/__init__.py:582-649`; blob marker at `libs/checkpoint/langgraph/checkpoint/serde/types.py:19-31`). Reducers must satisfy a batching-invariance law so replayed batches yield identical state (`libs/langgraph/langgraph/channels/delta.py:41-48`). Snapshot cadence is tracked in metadata (`counters_since_delta_snapshot`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:63-86`).
- Every serialized object carries its type provenance — `(module, name)` and constructor strategy inside typed msgpack extension codes (`EXT_PYDANTIC_V2=5`, `EXT_DELTA_SNAPSHOT=7`, etc., `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-302`, `:305-534`) — so post-deserialization you know what produced a value even if revival degrades.
- Content-level transforms are not logged. Message reducers like `add_messages` replace-by-id without recording that a replacement happened beyond the new object itself (`libs/langgraph/langgraph/graph/message.py:216-234`). Summarization/omission exists only as `TracePolicy` hooks that alter *trace records*, deliberately never the executing context, and are explicitly flagged as unsuitable for cross-run redaction (`libs/langgraph/langgraph/types.py:535-567`).

## Architectural Decisions

1. **Provenance lives on snapshots, not items.** The atomic unit with provenance is the checkpoint (`source`/`step`/`parents`/`ts`/`id`), not individual channel values or messages. This keeps hot-path writes cheap (no per-item envelope) and matches the time-travel debugging use case, but leaves store items and messages with only timestamps/IDs (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-124` vs `libs/checkpoint/langgraph/store/base/__init__.py:51-115`).
2. **Trust = controlled revival, not labels.** Rather than annotating trust levels, the system assumes all persisted bytes are hostile and constrains what can be revived (allowlists + strict mode + audit events). This converts "is this context trustworthy?" into "can this context be reconstructed at all?" — a binary enforced at load time (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:85-95`, `:559-609`).
3. **Monotonic identity as freshness substrate.** Choosing uuid6 with a process-wide strictly-increasing guarantee (`libs/checkpoint/langgraph/checkpoint/base/id.py:92-98`) means checkpoint sort order equals time order without parsing `ts`, enabling cheap lineage walks (`parent_config` chains) used by delta-history reconstruction (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:625-643`).
4. **Degrade-with-warning over fail-closed for legacy compatibility.** Blocked/unrevivable types return the raw kwargs dict or `None` with a warning instead of raising (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:645-653`, `:691-710`) — availability chosen over strictness, mitigated by opt-in strict mode and audit events.
5. **Metadata coalescing at persistence.** `get_checkpoint_metadata` folds scalar run-config keys into checkpoint metadata while excluding structural keys via `EXCLUDED_METADATA_KEYS` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:757-807`) — provenance piggybacks on user metadata rather than a fixed schema.

## Notable Patterns

- **Discriminated source literals with doc-tested semantics**: the `Literal[...]` type plus per-value docstrings acts as the contract; tests pin exact distributions (`Counter(c.metadata["source"]) == {...}`, `libs/langgraph/tests/test_pregel.py:1519-1560`).
- **Write-ahead attribution**: `PendingWrite = tuple[str, str, Any]` (task/channel/value shape at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:31`) plus negative-index special writes for ERROR/SCHEDULED/INTERRUPT/RESUME (`:788-795`) give every persisted write a task identity and kind.
- **Typed extension-code registry**: numbered msgpack ext codes form a stable wire vocabulary for type provenance (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:295-302`).
- **Observability hooks for integrity decisions**: a global listener registry with failure isolation emits structured serde events (`kind/module/name/method`, `libs/checkpoint/langgraph/checkpoint/serde/event_hooks.py:13-52`).
- **Namespace reservation**: store namespaces rooted at `"langgraph"` are rejected (`libs/checkpoint/langgraph/store/base/__init__.py:1280-1283`) — a lightweight trust-boundary convention protecting system namespaces.

## Tradeoffs

- **Snapshot-granularity provenance vs per-item cost**: cheap fast paths, but a downstream consumer of one store item cannot answer "which run/node produced this?" without external bookkeeping.
- **Warn-and-degrade vs fail-closed**: maximizes resumption odds after code changes (a renamed pydantic field still yields raw kwargs), but silently produces `None`-substituted context on revival failure (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:207-221` logs then returns `None`), which can masquerade as legitimately empty context.
- **Config-metadata folding vs unbounded schema**: folding arbitrary scalar config keys into persisted metadata aids observability but means the metadata schema is open-ended and consumer-dependent; only `str|int|bool|float` survive (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:768-774`).
- **DeltaChannel compression vs pruning complexity**: storing deltas shrinks storage dramatically but makes garbage collection dangerous — naive pruning severs reconstruction chains and channels silently reconstruct empty (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:374-415` spells out three safe strategies).
- **Dual sync/async API surface**: every provenance-bearing operation is duplicated (`put`/`aput`, `get_tuple`/`aget_tuple`, `get_delta_channel_history`/`aget_delta_channel_history`, `libs/checkpoint/langgraph/checkpoint/base/__init__.py:227-690`), doubling the surface where implementations can diverge (as InMemoryStore already has on `created_at`).

## Failure Modes / Edge Cases

- **Silent freshness corruption (dev parity trap)**: updating an item via `InMemoryStore` resets `created_at` to now (`libs/checkpoint/langgraph/store/memory/__init__.py:410-416`); the same workload on Postgres preserves it (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:404-408`). Age-based logic tested in dev will behave differently in production.
- **Provenance loss under blocked revival**: when a type fails the allowlist, the ext hook returns the inner raw data (`tup[2]`) or `None` on exception (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:645-653`, `:652-653`, `:724-730`) — the value survives but its type identity does not; consumers cannot distinguish degraded from intact context except via warnings/events.
- **Pickle fallback escape hatch**: opting into `pickle_fallback=True` reintroduces arbitrary-code-execution on load for non-msgpack-encodable objects (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:266-271`, `:287-288`) — documented, but a foot-gun adjacent to the trust model.
- **Chain-severing mutations**: `delete_for_runs`, `copy_thread`, and `prune` can remove writes/snapshots that live threads' DeltaChannels depend on; docs warn reconstruction then fails *silently* (no seed → empty state) (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:331-348`, `:350-372`, `:387-415`).
- **Interrupt-time fork safety**: replaying from an old checkpoint without a fork would overwrite the head; the loop proactively writes a `{"source": "fork"}` checkpoint and clears stale INTERRUPT writes to keep future resumes correct (`libs/langgraph/langgraph/pregel/_loop.py:952-971`).
- **NUL-byte poisoning of metadata**: string metadata values are scrubbed of `\u0000` before persistence (Postgres text safety) (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:761-764`) — provenance strings are mutated, though only by removing invalid characters.

## Future Considerations

- Add writer provenance to store items: extending `PutOp`/`Item` with an optional `writer` (run_id/node) field would close the biggest gap between checkpoint-grade and store-grade provenance; the `total=False`-style evolution pattern already used by `CheckpointMetadata` shows this is compatible practice.
- Reconcile `InMemoryStore._apply_put_ops` with Postgres upsert semantics (preserve `created_at`, refresh only `updated_at`) so age-based behaviors match across adapters.
- Promote serde degradation to first-class signals: attach a "degraded" marker to values revived as raw dicts/`None` so downstream nodes can branch on integrity rather than relying on log warnings.
- Record content transformations: a lightweight append-only journal keyed by message/item ID (analogous to `checkpoint_writes`) would make summarization/trim auditable, complementing the existing state-reconstruction history.
- Stabilize the DeltaChannel provenance surface (currently beta-flagged at `libs/checkpoint/langgraph/channels/delta.py:29-36` and `libs/checkpoint/langgraph/checkpoint/base/__init__.py:149-173`) before third-party savers proliferate incompatible prune/copy strategies.

## Questions / Gaps

- No evidence found of any trust/authority/confidence annotation attached to individual context items; searched `trust|authority|credib|confiden` across `libs/` (only unrelated CLI/CORS and lockfile hits). If per-source trust is desired, it must be built at the application layer.
- No evidence found of transformation logging for summarization/redaction of live context: searched `summariz` across `libs/langgraph`, `libs/prebuilt`, `libs/checkpoint` — hits are limited to TracePolicy documentation (`libs/langgraph/langgraph/types.py:550`, `:556`; `libs/langgraph/langgraph/graph/state.py:702`) and prebuilt docstrings (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:397`), none of which mutate or record actual context.
- `libs/sdk-js` contains only a README stub (no implementation), and `libs/sdk-py` is a thin REST client — neither carries provenance semantics worth separate analysis; server-side provenance handling (e.g., LangGraph Platform) is out of this source's scope.
- Whether `parents` map completeness holds for deep subgraph nesting under all durability modes was not exhaustively traced (`CONFIG_KEY_CHECKPOINT_MAP` population at `libs/langgraph/langgraph/pregel/_algo.py` and consumption at `libs/langgraph/langgraph/pregel/_loop.py:1126` was verified, but not every nested-retry path).

---

Generated by `Dimension 11.04: Context Provenance and Integrity` against `langgraph`.
