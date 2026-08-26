# Source Analysis: langgraph

## 15.03 Shared State and Conflict Resolution

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core framework, checkpointers, stores); TypeScript SDK (`libs/sdk-js`) is API-client only |
| Analyzed | 2026-08-26 |

## Summary

LangGraph's shared-state model is built on three explicit layers. (1) **Channels** are the intra-run shared state: every state key in a `StateGraph` maps to a channel object with a declared merge policy (reducer). Nodes in a superstep write concurrently into per-task buffers; writes are applied to channels only at the superstep boundary by `apply_writes`, which groups writes per channel and lets the channel's own `update()` decide how to merge them (`studies/agent-harness-study/sources/langgraph/libs/langgraph/langgraph/pregel/_algo.py:232-345`). Merge policies range from reject-on-conflict (`LastValue`) to CRDT-like folding (`BinaryOperatorAggregate`) and pub/sub accumulation (`Topic`). (2) **Checkpoints** persist per-thread graph state durably, keyed by `(thread_id, checkpoint_ns)`, with per-task write records keyed so concurrent writers never collide at the storage layer. (3) **Store** (`BaseStore`) provides cross-thread memory via hierarchical namespaces, backed by Postgres upserts (last-write-wins) or an unsynchronized in-memory dict.

Conflict handling is therefore mostly *declarative and fail-fast*: conflicts within a superstep are either resolved deterministically by reducers or rejected with `InvalidUpdateError` carrying error code `INVALID_CONCURRENT_GRAPH_UPDATE`; conflicts across runs/threads are avoided by partitioning (thread-scoped checkpoints) rather than resolved; and cross-thread Store writes use blind last-write-wins upserts with no compare-and-swap. Channel version vectors (`channel_versions` / `versions_seen`) provide stale-read detection for task triggering and power ambiguity detection in external `update_state` calls.

## Rating

**Score: 8/10**

Rationale against rubric:

- **Clear model, explicit interfaces**: conflict semantics are encoded in first-class channel classes with documented update contracts — `LastValue.update` rejects >1 value per step (`.../channels/last_value.py:56-64`), `BinaryOperatorAggregate.update` folds or rejects multiple `Overwrite`s (`.../channels/binop.py:123-144`), `DeltaChannel.update` likewise (`.../channels/delta.py:159-183`). Reducer wiring from type annotations is explicit and signature-validated (`.../graph/state.py:1904-1922`).
- **Tests**: conflict behavior is directly tested — multi-value rejection (`.../tests/test_channels.py:33-49`), parallel-Overwrite conflict raising `InvalidUpdateError` (`.../tests/test_pregel.py:9333-9368`), thread-safety of concurrent runs (`.../tests/test_pregel.py:5334-5369`), parallel last-write ordering (`.../tests/test_pregel.py:9310-9330`).
- **Operational safeguards**: deterministic sorted application of concurrent task writes (`.../pregel/_algo.py:253-256`), durable ERROR writes before failure handling (`.../pregel/_loop.py:1578`), idempotent DB upserts keyed by task identity (`.../checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-158`), in-process locks around connection/pipeline mutation.
- Why not 9–10: the cross-thread `BaseStore` has no optimistic concurrency control (no CAS/version precondition on `put`, `.../store/base/__init__.py:856-935`), so concurrent writers silently lose updates; `InMemoryStore` applies batches with no lock at all (`.../store/memory/__init__.py:206-234`); and there is no dedicated, queryable conflict log — conflicts surface as exceptions, generic warnings, or pending ERROR writes.

## Evidence Collected

Every entry includes a file path with line numbers. All paths below are workspace-relative under `studies/agent-harness-study/sources/langgraph/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared state API (intra-run) | `BaseChannel` abstract base: `update(values) -> bool`, `get()`, `checkpoint()`, `from_checkpoint()` define the read/write/merge contract for all shared state keys | `libs/langgraph/langgraph/channels/base.py` (contract consumed throughout `libs/langgraph/langgraph/pregel/_algo.py:232-345`) |
| Shared state API (schema wiring) | `Annotated[type, reducer]` fields become channels via `_get_channel`; fallback is `LastValue`; binop reducer must have signature `(a, b) -> c` | `libs/langgraph/langgraph/graph/state.py:1850-1873`, `libs/langgraph/langgraph/graph/state.py:1904-1922` |
| Shared state API (last-value) | `LastValue` — "Stores the last value received, can receive at most one value per step" | `libs/langgraph/langgraph/channels/last_value.py:20-21` |
| Shared state API (aggregate) | `BinaryOperatorAggregate(typ, operator)` folds each new value with a user-supplied binary operator | `libs/langgraph/langgraph/channels/binop.py:65-92` |
| Shared state API (pubsub) | `Topic` — "configurable PubSub Topic", flattens lists, optional accumulation across steps | `libs/langgraph/langgraph/channels/topic.py:23-44` |
| Shared state API (barrier) | `NamedBarrierValue` blocks availability until *all* named values received — synchronization primitive between agents | `libs/langgraph/langgraph/channels/named_barrier_value.py:13-24`, availability check `named_barrier_value.py:69-75` |
| Shared state API (cross-thread) | `BaseStore` docstring: "persistence and memory that can be shared across threads, scoped to user IDs, assistant IDs, or other arbitrary namespaces"; abstract `batch`/`abatch` op interface | `libs/checkpoint/langgraph/store/base/__init__.py:708-754` |
| Shared state API (cross-thread put) | `BaseStore.put(namespace, key, value, index, ttl)` — no version/precondition parameter exists | `libs/checkpoint/langgraph/store/base/__init__.py:856-935` |
| Shared state API (durable run state) | `BaseCheckpointSaver.get_tuple/put/put_writes` — per-thread checkpoint + intermediate task writes | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:227-318` |
| Write path (buffering) | Node writes routed through `ChannelWrite.do_write` → `CONFIG_KEY_SEND` callable into per-task buffers; reserved `TASKS` channel is write-protected | `libs/langgraph/langgraph/pregel/_write.py:105-126`, `_write.py:112-119` |
| Conflict detection (reject) | `LastValue.update` raises `InvalidUpdateError` ("Can receive only one value per step") when a step delivers ≥2 values | `libs/langgraph/langgraph/channels/last_value.py:56-64` |
| Conflict detection (error code) | `ErrorCode.INVALID_CONCURRENT_GRAPH_UPDATE` defined and referenced by all rejecting channels | `libs/langgraph/langgraph/errors.py:36`, used at `channels/last_value.py:62`, `channels/binop.py:136`, `channels/delta.py:169` |
| Conflict detection (overwrite clash) | Two `Overwrite` values in one super-step → `InvalidUpdateError` "Can receive only one Overwrite value per super-step" | `libs/langgraph/langgraph/channels/binop.py:129-141`, `libs/langgraph/langgraph/channels/delta.py:162-172` |
| Conflict detection (version vectors) | `checkpoint["versions_seen"]` recorded per task per trigger channel; `_triggers()` fires a node only if current channel version > seen version (stale-read detection) | `libs/langgraph/langgraph/pregel/_algo.py:262-269`, `libs/langgraph/langgraph/pregel/_algo.py:1260-1277` |
| Conflict detection (external ambiguity) | `update_state`: if two nodes updated state at the same time and versions can't disambiguate, raises `"Ambiguous update, specify as_node"` | `libs/langgraph/langgraph/pregel/main.py:1921-1935` |
| Resolution handler (deterministic apply) | `apply_writes` sorts tasks on path for deterministic application order, groups writes by channel, then calls each channel's `update(vals)` once | `libs/langgraph/langgraph/pregel/_algo.py:253-256`, `_algo.py:294-309`, `_algo.py:315-323` |
| Resolution handler (reducer fold) | `BinaryOperatorAggregate.update` folds values sequentially; an `Overwrite` resets the accumulator | `libs/langgraph/langgraph/channels/binop.py:123-144` |
| Resolution handler (explicit override) | `Overwrite` dataclass bypasses reducer; recognized across JSON serialization boundaries via discriminator `"__overwrite__"` | `libs/langgraph/langgraph/types.py:977-1024`, `libs/langgraph/langgraph/channels/binop.py:31-51` |
| Coordination locks (runner) | `PregelRunner` future-tracking uses `threading.Lock` around counter/done-set mutation | `libs/langgraph/langgraph/pregel/_runner.py:97`, `_runner.py:111-113`, `_runner.py:126-132` |
| Coordination locks (postgres store) | `PostgresStore.lock = threading.Lock()` guarding batched pipeline operations; async variant uses `asyncio.Lock` | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:771`, `libs/checkpoint-postgres/langgraph/store/postgres/aio.py:150,558` |
| Coordination locks (checkpointer) | `PostgresSaver.lock` held during setup/upsert sections; SQLite saver/store equivalents | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:43,59,413`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:95,181` |
| Consistency (DB idempotency) | Checkpoint blobs upsert `ON CONFLICT (thread_id, checkpoint_ns, channel, version) DO NOTHING` — content-addressed, replay-safe; writes keyed `(checkpoint_id, task_id, idx)` so concurrent tasks never collide | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:131-135`, `base.py:146-158` |
| Consistency (BSP barrier) | Writes from all concurrent tasks committed only between supersteps via single `apply_writes` call (bulk-synchronous-parallel execution) | `libs/langgraph/langgraph/pregel/_algo.py:232-345`; runner commits per task at completion `libs/langgraph/langgraph/pregel/_runner.py:574-613` |
| Conflict logging (unknown channel) | `logger.warning(f"Task {task.name} ... wrote to unknown channel {chan}, ignoring it.")` | `libs/langgraph/langgraph/pregel/_algo.py:310-313` |
| Conflict logging (unknown push targets) | Warnings for unknown node names and invalid PUSH paths in pending sends | `libs/langgraph/langgraph/pregel/_algo.py:972-978`, `_algo.py:999` |
| Conflict logging (durable errors) | Task exceptions appended as `(ERROR, exception)` writes and persisted via `put_writes`; comment notes "ensure error + ERROR_SOURCE_NODE writes are durable"; failed-task errors replayed into `task.writes` on resume | `libs/langgraph/langgraph/pregel/_runner.py:596-603`, `libs/langgraph/langgraph/pregel/_loop.py:741-795`, `_loop.py:1452`, `_loop.py:1578` |
| Tests (channel conflicts) | `test_last_value` asserts `InvalidUpdateError` on `channel.update([5, 6])` | `libs/langgraph/tests/test_channels.py:33-49` |
| Tests (parallel overwrite) | `test_overwrite_parallel_error`: two parallel nodes both returning `Overwrite` → `pytest.raises(InvalidUpdateError, match="Can receive only one Overwrite value per super-step.")` | `libs/langgraph/tests/test_pregel.py:9333-9368` |
| Tests (parallel last-write) | Parallel fan-out b/c into LastValue key: result `[b, d]` shows deterministic path-order resolution, not exception | `libs/langgraph/tests/test_pregel.py:9310-9330` |
| Tests (thread safety) | `test_concurrent_execution_thread_safety`: 10 threads invoke same graph concurrently; results independent | `libs/langgraph/tests/test_pregel.py:5334-5369` |
| Tests (reducer race hardening) | Comment: message-ID assignment moved before background serialization because "reducers that assign IDs inside apply_writes() race with serialisation" | `libs/langgraph/langgraph/pregel/_loop.py:456-465` |
| Race-hardened counter | `LazyAtomicCounter` double-checked init under module-level `threading.Lock` | `libs/langgraph/langgraph/pregel/_algo.py:1423-1439` |

## Answers to Dimension Questions

### 1. What state is shared between agents?

Four distinct sharing scopes exist:

- **Intra-superstep, same thread**: named **channels**. Every node reads its triggers from channels and writes through `ChannelWrite` entries (`libs/langgraph/langgraph/pregel/_write.py:26-50,105-126`). Channel kinds set the sharing semantics: `LastValue` (single-writer-per-step slot), `BinaryOperatorAggregate` (commutative fold), `Topic` (multi-producer pubsub list, `libs/langgraph/langgraph/channels/topic.py:77-85`), `EphemeralValue`, `UntrackedValue` (never checkpointed), `NamedBarrierValue` (rendezvous).
- **Across supersteps, same thread**: the **checkpoint**, holding `channel_values`, `channel_versions`, and `versions_seen`; persisted via `BaseCheckpointSaver.put` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:277-298`). Intermediate per-task writes survive failures via `put_writes` (`checkpoint/base/__init__.py:300-318`).
- **Across threads/runs (agents)**: the **Store**. `BaseStore` explicitly supports "memory that can be shared across threads" scoped by namespace tuples (`libs/checkpoint/langgraph/store/base/__init__.py:708-714`), with `put/get/search/delete/list_namespaces`. Implementations: `InMemoryStore` (`libs/checkpoint/langgraph/store/memory/__init__.py:136`), `PostgresStore` (`libs/checkpoint-postgres/langgraph/store/postgres/base.py`), SQLite store.
- **Per-run scratchpad**: managed values resolved against a `PregelScratchpad` (`libs/langgraph/langgraph/managed/base.py:18-24`; scratchpad construction `libs/langgraph/langgraph/pregel/_algo.py:1280+`) — deliberately not shared across runs.

### 2. How are conflicts detected?

- **Cardinality check at commit time**: each channel inspects the full batch of writes for one superstep. `LastValue` treats ≥2 values as a conflict (`libs/langgraph/langgraph/channels/last_value.py:59-64`); `BinaryOperatorAggregate` and `DeltaChannel` treat ≥2 `Overwrite`s as conflicts (`binop.py:129-141`; `delta.py:162-172`). Detection is centralized in one place per channel class, tagged `INVALID_CONCURRENT_GRAPH_UPDATE` (`errors.py:36`).
- **Version-vector comparison**: `versions_seen[name][chan]` vs `channel_versions[chan]` detects that a node has not yet observed a newer channel version — used both to trigger nodes (`_triggers`, `libs/langgraph/langgraph/pregel/_algo.py:1260-1277`) and to disambiguate external state updates (`main.py:1921-1935`).
- **Storage-layer uniqueness keys**: conflicting concurrent persistence is structurally prevented — writes are keyed by `(checkpoint_id, task_id, idx)` (`checkpoint/postgres/base.py:146-158`) and blobs by content version (`base.py:131-135`).

### 3. How are conflicts resolved?

Five coexisting strategies:

1. **Deterministic merge (default for accumulators)**: reducers fold all concurrent writes; application order made deterministic by sorting tasks on path before applying (`libs/langgraph/langgraph/pregel/_algo.py:253-256`), so results don't depend on thread scheduling.
2. **Fail-fast rejection**: undeclared concurrent writes to a `LastValue` key abort the superstep with `InvalidUpdateError`; the error becomes a durable `(ERROR, exc)` write via `PregelRunner.commit` (`_runner.py:596-603`) and is replayable after resume (`_loop.py:763-795`). The fix is prescribed in the error message itself: "Use an Annotated key to handle multiple values" (`last_value.py:61`).
3. **Explicit override with mutual exclusion**: `Overwrite` bypasses reducers but only one overwrite per superstep is allowed (`types.py:981-982` documents the invariant; enforced at `binop.py:133-138`). Verified by test `test_overwrite_parallel_error` (`tests/test_pregel.py:9333-9368`).
4. **Human-in-the-loop disambiguation**: ambiguous external updates require `as_node`, chosen via `versions_seen` recency analysis (`main.py:1921-1935`).
5. **Storage-level last-write-wins / idempotency**: Postgres store items upsert `ON CONFLICT (prefix, key) DO UPDATE SET value = EXCLUDED.value` (`checkpoint-postgres/store/postgres/base.py:401-409`) — silent last-writer-wins; checkpoint blob inserts `DO NOTHING` on identical version (idempotent replay); write rows `DO UPDATE`/`DO NOTHING` per task index (`checkpoint/postgres/base.py:149-158`). In-process locks (`threading.Lock` in `PostgresSaver.__init__`/setup, `checkpoint/postgres/__init__.py:59,413`) serialize local pipeline access but do not arbitrate between processes.

### 4. Is shared state consistent?

- **Within a run: yes, by construction.** The BSP discipline (writes buffered per task, applied once at the superstep boundary, `_algo.py:232-345`) means no node ever observes a partially-applied peer write; there is no mid-step interleaving to corrupt state. Deterministic sort + reducer associativity gives reproducible merges. A known race between reducer-assigned message IDs and background serialization was fixed by pre-assigning IDs before serialization (`_loop.py:456-465`), showing active maintenance of this guarantee.
- **Across resumes: yes.** Channel versioning plus content-addressed blobs make checkpoint restore deterministic; `versions_seen` prevents re-triggering on stale values.
- **Across threads via the Store: weakly.** There is no CAS, no version column exposed in the API (`store/base/__init__.py:856-935`), and `InMemoryStore._apply_put_ops` mutates plain dicts with no synchronization (`store/memory/__init__.py:404-416`; `batch` at `206-219` takes no lock). Concurrent cross-thread writers to the same `(namespace, key)` produce silent lost updates. Consistency is achieved by *partitioning* (namespaces per user/thread by convention) rather than by arbitration.

## Architectural Decisions

1. **Bulk-synchronous-parallel (Pregel-style) execution over fine-grained locking** (`_algo.py:232-345`): conflicts between agents are eliminated by scheduling (all writes commit at a barrier), drastically shrinking the surface where races can occur. This is the central design choice that makes the rest tractable.
2. **Merge policy as data, attached to the state schema**: reducers live in `Annotated[...]` metadata and become channel instances at compile time (`graph/state.py:1850-1873,1904-1922`). Conflict behavior is thus declared per-key, statically visible, and validated at graph construction ("Invalid reducer signature", `state.py:1919-1921`).
3. **Reject-don't-block for irreconcilable writes**: instead of queuing or arbitrarily picking a winner for `LastValue` clashes, LangGraph raises with an actionable message and error code (`last_value.py:59-64`, `errors.py:36`) — surfacing modeling mistakes early rather than hiding nondeterminism.
4. **Task-keyed durable writes**: persistence keys writes by `(checkpoint_id, task_id, idx)` (`checkpoint/postgres/base.py:146-158`) so that even concurrent/retrying tasks cannot overwrite each other at the storage layer; merging happens only at logical replay time.
5. **Content-addressed channel versions**: `increment` version function (`_algo.py:227-229`) and `ON CONFLICT ... DO NOTHING` blob upserts (`checkpoint/postgres/base.py:131-135`) give cheap idempotency and stale-read detection without wall-clock timestamps.
6. **Cross-thread sharing delegated to pluggable Store backends**: `BaseStore` is intentionally minimal (op-batch based, `store/base/__init__.py:732-754`), pushing concurrency control down to the backend (Postgres transactions) rather than specifying it in the contract.

## Notable Patterns

- **Channels as CRDT-lite objects**: `BinaryOperatorAggregate` mirrors a commutative replicated data type fold — order-independent for commutative operators; combined with sorted application it approximates deterministic convergence (`binop.py:123-144`).
- **Barrier synchronization as a channel**: `NamedBarrierValue.is_available()` returns true only when the seen-set equals the expected names set (`named_barrier_value.py:69-81`) — rendezvous/join semantics expressed uniformly with data channels.
- **Sentinel-encoded override surviving serialization**: `Overwrite` carries a literal `type: "__overwrite__"` discriminator so override semantics survive JSON round-trips through the API server (`types.py:1020-1024`; recognition logic `binop.py:31-51`).
- **Errors as channel writes**: failures, interrupts, and resume signals travel through reserved pseudo-channels (`ERROR`, `ERROR_SOURCE_NODE`, `INTERRUPT`, `RESUME`) excluded from normal merging (`_algo.py:298-306`; commit path `_runner.py:574-613`), giving uniform durability and observability.
- **Defensive equality for lambdas**: `_operators_equal` treats any lambda reducer as equal to another (`binop.py:54-62`), acknowledging Python lambda identity limits in graph-equivalence checks.

## Tradeoffs

- **Strictness vs ergonomics**: defaulting every unannotated key to `LastValue` (`graph/state.py:1871-1873`) means naive users get loud failures under fan-out; this is safer than silent corruption but pushes developers to understand reducers upfront.
- **Fail-fast vs availability**: an `InvalidUpdateError` aborts the whole superstep (and cancels sibling tasks via `_should_stop_others`, `_runner.py:616-634`) rather than isolating the offending writer — simple and predictable, but one bad node poisons the step.
- **Determinism vs throughput**: sorting all tasks before applying writes (`_algo.py:256`) buys reproducibility at negligible cost, but reducer-based convergence still requires users to supply associative operators; non-commutative reducers yield order-dependent (if deterministic) results.
- **Simplicity of the Store vs lost updates**: blind upserts keep the API tiny and backends portable, but forfeit optimistic concurrency; applications needing read-modify-write cycles must build their own versioning inside the stored value.
- **In-memory convenience vs safety**: `InMemoryStore` and `InMemorySaver` avoid I/O but rely on GIL-level atomicity only (`store/memory/__init__.py:206-234`), acceptable for dev/test, hazardous for multi-threaded production use.

## Failure Modes / Edge Cases

- **Parallel Overwrite collision**: two fan-out branches both issuing `Overwrite` to the same key raise `InvalidUpdateError`; tested at `tests/test_pregel.py:9333-9368`.
- **Undeclared fan-out to LastValue**: any two-node fan-out writing the same unannotated key fails the step (`last_value.py:56-64`).
- **Lost updates in shared Store**: process A and B both `get()` then `put()` the same item — no conflict is detected; B's write silently wins (no CAS anywhere in `store/base/__init__.py:856-944`; Postgres `ON CONFLICT DO UPDATE` at `checkpoint-postgres/store/postgres/base.py:404-408`).
- **Unsynchronized InMemoryStore under threads**: concurrent `batch` calls mutate shared dicts without locks (`store/memory/__init__.py:206-234,404-416`); individual dict ops are GIL-atomic but compound sequences (search-then-put) are not isolated.
- **Reducer/serialization race (fixed)**: reducers assigning IDs inside `apply_writes` raced with background checkpoint serialization producing unstable replays; mitigated by pre-assigning message IDs (`_loop.py:456-465`).
- **Unknown-channel writes degrade to warnings**: a typo'd channel name in a write is logged (`logger.warning`, `_algo.py:310-313`) and dropped rather than failing — a deliberate leniency that can mask bugs.
- **Ambiguous external updates**: `update_state` after a parallel superstep raises unless the caller supplies `as_node` (`main.py:1934-1935`).
- **Shallow checkpointer overwrite**: `ShallowSqlSaver`-style upserts replace the single row per thread (`checkpoint/postgres/shallow.py:123-130`), trading history for space — forks/resumes overwrite rather than branch.

## Future Considerations

- Add an optional compare-and-swap or expected-version parameter to `BaseStore.put` to support safe read-modify-write across threads without changing the default last-write-wins behavior (`store/base/__init__.py:856-935`).
- Introduce a structured, queryable conflict/event log (beyond `logger.warning` at `_algo.py:310-313` and pending ERROR writes) so post-hoc audits of dropped unknown-channel writes or rejected merges are possible.
- Consider documenting or enforcing associativity requirements for `BinaryOperatorAggregate` reducers (e.g., a debug-mode check), since determinism currently depends on sorted application order alone (`binop.py:123-144`, `_algo.py:256`).
- Extend the `versions_seen` mechanism (already powering `_triggers` and ambiguity detection, `_algo.py:1260-1277`, `main.py:1921-1935`) into an observable staleness report for long-running multi-agent deployments.

## Questions / Gaps

- No evidence found of distributed (multi-process) coordination primitives for the Store beyond database transactions — searches for lease/fencing/quorum patterns across `libs/checkpoint*` returned nothing beyond per-instance locks (`checkpoint-postgres/store/postgres/base.py:771`, `checkpoint-postgres/checkpoint/postgres/__init__.py:59`). Cross-process conflict resolution is entirely delegated to Postgres/SQLite semantics.
- No dedicated "conflict log" artifact was found. What exists: raised `InvalidUpdateError`s (with code `INVALID_CONCURRENT_GRAPH_UPDATE`, `errors.py:36`), warning logs for ignored writes (`_algo.py:310-313,972-999`), and durable ERROR writes surfaced in debug output (`pregel/debug.py:118,228`). If a question-grade audit trail is required, it is not implemented in-repo.
- The docs directory was not exhaustively surveyed for stated design goals on shared memory; this analysis prioritizes implementation and tests per study rules. Behavior claims above are implementation-derived, not README-derived.

---

Generated by `15.03-shared-state-and-conflict-resolution` against `langgraph`.
