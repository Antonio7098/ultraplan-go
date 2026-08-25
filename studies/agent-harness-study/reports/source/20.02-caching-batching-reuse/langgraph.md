# Source Analysis: langgraph

## Dimension 20.02 — Caching, Batching, and Reuse

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: core `libs/langgraph`, checkpoint/cache backends under `libs/checkpoint*`, JS/TS SDK under `libs/sdk-js`), TypeScript SDK |
| Analyzed | 2026-08-25 |

## Summary

LangGraph implements caching as an explicit, opt-in subsystem for **task results** (node/channel-write tuples), not for model responses or prompts. A pluggable `BaseCache` interface (`libs/checkpoint/langgraph/cache/base/__init__.py:15`) with three shipped backends — in-memory (`libs/checkpoint/langgraph/cache/memory/__init__.py:11`), SQLite (`libs/checkpoint-sqlite/langgraph/cache/sqlite/__init__.py:13`), and Redis (`libs/checkpoint/langgraph/cache/redis/__init__.py:10`) — is wired into the Pregel execution loop. Tasks declare a `CachePolicy` (`libs/langgraph/langgraph/types.py:521`); the loop consults the cache before scheduling a task and stores successful task writes after completion (`libs/langgraph/langgraph/pregel/_loop.py:1549`, `_loop.py:1609`). Keys are namespaced xxh3-128 hashes of a policy-defined key function (pickle-based by default, `libs/langgraph/langgraph/_internal/_cache.py:26`). TTLs are first-class.

Beyond the node cache there are two smaller reuse mechanisms: (1) a per-superstep **input deduplication cache** so identical node inputs are computed once per tick (`libs/langgraph/langgraph/pregel/_algo.py:437`, `_algo.py:1348-1392`), and (2) a **store operation batcher/deduper** (`AsyncBatchedBaseStore`, `libs/checkpoint/langgraph/store/base/batch.py:58`) that coalesces memory-store operations queued within one event-loop tick into a single `abatch` call, dropping duplicate reads and collapsing duplicate puts. Embedding computation inside store implementations is batched per operation batch (e.g., all search queries embedded with one `embed_documents` call, `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1070-1088`) but embeddings are **not cached or reused across calls** — each write re-embeds its content.

Notably absent: any model-response cache, any framework-managed prompt/prefix caching (Anthropic `cache_control` blocks merely pass through message content unchanged), retrieval caches, and LLM request batching. Model-call efficiency comes from *concurrency* (thread-pool / asyncio execution of tasks per superstep, `libs/langgraph/langgraph/pregel/_executor.py:40`, `_executor.py:122`), not from batching requests.

## Rating

**7 / 10**

Rationale against the rubric:

- The mechanism that exists has a clear model: abstract interface + three backends, explicit opt-in policies (`CachePolicy`), deterministic hashed keys, TTL support, invalidation APIs (`Pregel.clear_cache` at `libs/langgraph/langgraph/pregel/main.py:4136`; functional-task `clear_cache` at `libs/langgraph/langgraph/func/__init__.py:96`), subgraph propagation via config (`libs/langgraph/langgraph/_internal/_constants.py:44`), and broad test coverage across all three backends (`libs/langgraph/tests/conftest.py:60-84`, `tests/test_pregel.py:2807`, `tests/test_pregel.py:5745`, `libs/checkpoint/tests/test_redis_cache.py`).
- It falls short of 9-10 because coverage is narrow relative to the dimension's full scope: no response/prompt/embedding/retrieval caching at all; the Redis backend silently swallows failures and its async methods just wrap sync calls (`libs/checkpoint/langgraph/cache/redis/__init__.py:80-82`, `110-112`); there is no observability (no metrics/counters around hit/miss); and the default pickle-based key function is fragile for mutable inputs (see Failure Modes).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Cache abstraction | `BaseCache` ABC: `get/aget/set/aset/clear/aclear` over `(Namespace, key)` pairs with optional TTL; values serialized via `JsonPlusSerializer(pickle_fallback=False)` | `libs/checkpoint/langgraph/cache/base/__init__.py:15-48` |
| In-memory backend | Thread-safe dict-of-namespaces with lazy expiry purge on read; `RLock` guard | `libs/checkpoint/langgraph/cache/memory/__init__.py:11-32` |
| SQLite backend | WAL-mode table `(ns,key)->(expiry,encoding,val)`, batched `IN (...)` lookup, expired-entry purge on read, async via `asyncio.to_thread` | `libs/checkpoint-sqlite/langgraph/cache/sqlite/__init__.py:13-114` |
| Redis backend | Key prefixing, `MGET` multi-read, pipelined writes with `SETEX` TTL; failures swallowed (return `{}` / pass) | `libs/checkpoint/langgraph/cache/redis/__init__.py:52-78`, `84-108` |
| Cache policy type | `CachePolicy(key_func, ttl)` dataclass; default key func pickles frozen args/kwargs | `libs/langgraph/langgraph/types.py:521-529`; `libs/langgraph/langgraph/_internal/_cache.py:7-31` |
| Cache key construction | Node/PULL tasks: namespace `(CACHE_NS_WRITES, identifier(proc), node_name)` + `xxh3_128_hexdigest(args_key)`; PUSH/Send tasks and functional `@task` calls build analogous keys | `libs/langgraph/langgraph/pregel/_algo.py:856-872`, `_algo.py:1019-1034`, `_algo.py:1166-1182` |
| Loop-side cache read | `match_cached_writes()` collects tasks that have a `cache_key` and no writes yet, bulk-fetches via one `cache.get(...)`, and replays stored writes; async twin `amatch_cached_writes` | `libs/langgraph/langgraph/pregel/_loop.py:1549-1562`, `_loop.py:1804-1817` |
| Loop-side cache write | `put_writes()` persists task writes to cache after success; async path skips `INTERRUPT`/`ERROR` writes ("only cache successful tasks") | `libs/langgraph/langgraph/pregel/_loop.py:1609-1625`, `_loop.py:1864-1883` |
| Graph-level wiring | `compile(cache=...)`, graph-wide default `cache_policy`, per-node `add_node(..., cache_policy=...)`, defaults applied only to regular nodes | `libs/langgraph/langgraph/graph/state.py:1177-1181`, `state.py:276-330`, `state.py:1288-1334`, `state.py:1541` |
| Subgraph propagation | `CONFIG_KEY_CACHE` carries the `BaseCache` into subgraphs | `libs/langgraph/langgraph/_internal/_constants.py:43-44`; `libs/langgraph/langgraph/pregel/main.py:2598-2601` |
| Invalidation API | `Pregel.clear_cache(nodes)` / `aclear_cache` raise if no cache configured; clear per-node namespaces; `@task.clear_cache(cache)` | `libs/langgraph/langgraph/pregel/main.py:4136-4172`; `libs/langgraph/langgraph/func/__init__.py:96-106` |
| Per-tick input dedup | `input_cache` dict shared while preparing a superstep's tasks; `_proc_input` returns shallow copy on hit, keyed by `input_cache_key` (mapper + channels tuple) | `libs/langgraph/langgraph/pregel/_algo.py:437`, `_algo.py:1357-1392`; `libs/langgraph/langgraph/pregel/_read.py:250-259` |
| Store op batching | `AsyncBatchedBaseStore` queues ops per event-loop tick, drains queue into single `abatch(dedupped)`; `_dedupe_ops` collapses duplicate get/search ops and last-writer-wins puts | `libs/checkpoint/langgraph/store/base/batch.py:58-101`, `batch.py:326-371`, `batch.py:283-323` |
| Batched embedding requests | Postgres store gathers embedding texts from a whole op batch and issues one `embed_documents(...)` call (write path and search path) | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:340-423`, `base.py:1040-1065`, `base.py:1070-1088` |
| No embedding reuse | Vectors recomputed on every put/search; no vector cache found anywhere in store code | `libs/checkpoint-postgres/langgraph/store/postgres/base.py:1051`, `1079`; aio twin `libs/checkpoint-postgres/langgraph/store/postgres/aio.py:482-510` |
| Prompt caching stance | Only pass-through: docs show OpenAI-format content block carrying Anthropic-style `cache_control` field; framework neither adds nor manages it | `libs/langgraph/langgraph/graph/message.py:143-160` |
| No model-response cache | Search for response/result caching of LLM output found nothing; prebuilt tool-result caching exists only as a user-implemented middleware example | `libs/prebuilt/langgraph/prebuilt/tool_node.py:267-276` |
| Concurrency instead of batching | Tasks execute concurrently via `BackgroundExecutor` (thread pool from langchain config) / `AsyncBackgroundExecutor`; no `.batch(` call sites in core lib | `libs/langgraph/langgraph/pregel/_executor.py:40-71`, `_executor.py:122+` |
| Server-run batching (SDK only) | `create_batch` posts multiple stateless run payloads to `/runs/batch` — orchestration-level fan-out, not model-request batching | `libs/sdk-py/langgraph_sdk/_async/runs.py:607-622` |
| Tests: cross-backend fixture | `cache` fixture parametrized over sqlite/memory/redis (redis gated on Docker, worker-scoped prefix) | `libs/langgraph/tests/conftest.py:60-84` |
| Tests: node-cache behavior | Fan-out graph test parametrized `with_cache` True/False verifying `rewrite_query_count` stays flat when cached; functional-API interrupt/resume cache test asserting recompute after `clear_cache` | `libs/langgraph/tests/test_pregel.py:2807-2874+`, `test_pregel.py:5745-5814`; async mirror `tests/test_pregel_async.py:4699`, `6792-6798` |
| Tests: no redundant writes for cached tasks | Dedicated tests assert cached tasks do not emit duplicate checkpoint writes | `libs/langgraph/tests/test_pregel.py:5907`; `tests/test_pregel_async.py:8134` |
| Tests: Redis failure modes | Unavailable server returns `{}` on get, silently skips set/clear; corrupted entries skipped | `libs/checkpoint/tests/test_redis_cache.py:223-266`, `273-281` |

## Answers to Dimension Questions

1. **Are model responses cached?**
   No. There is no cache anywhere in the repo keyed on LLM/model invocation input or output. The only caching layer caches *node/task channel writes* (`libs/langgraph/langgraph/pregel/_loop.py:1609-1625`), which indirectly covers a node whose body calls a model, but it is opt-in per node/task (`CachePolicy`), keys off the node input rather than the exact prompt sent to the provider, and stores state updates rather than raw completions. Prebuilt agents add no model-result caching; the "cached result" references in `libs/prebuilt/tests/test_on_tool_call.py:417-447` exercise a user-supplied middleware hook pattern documented in `libs/prebuilt/langgraph/prebuilt/tool_node.py:267-276`.

2. **Are embeddings reused?**
   Not across operations. Embeddings are computed on demand inside store implementations and batched within a single store-operation batch: the Postgres store collects every text needing an embed in one batch and calls `embed_documents` once (`libs/checkpoint-postgres/langgraph/store/postgres/base.py:1040-1065` for writes, `base.py:1070-1088` for searches). But no persistent or in-process embedding cache exists — repeated identical `put`/`search` payloads pay the embedding cost each time. Reuse is left to users, e.g., by storing vectors under a `("cache","embeddings",...)`-style namespace in the store itself (the namespace pattern is supported generically, `libs/checkpoint/langgraph/store/base/__init__.py:319,458-461`).

3. **Is retrieval cached?**
   No dedicated retrieval cache. Two adjacent mechanisms exist: (a) node-level result caching can memoize a retriever *node's* output if the author attaches a `CachePolicy` — the canonical fan-out test uses a `rewrite_query` node exactly this way (`libs/langgraph/tests/test_pregel.py:2823-2858`); (b) the per-superstep `input_cache` avoids recomputing identical inputs for multiple tasks reading the same channels within one tick (`libs/langgraph/langgraph/pregel/_algo.py:1357-1360`). Neither caches retrieval results keyed by query text across runs unless the user opts into the node cache with a suitable key function.

4. **Are model calls batched efficiently?**
   No request batching for model calls exists in the framework. Parallelism is provided instead: each superstep's ready tasks run concurrently on a thread pool (`BackgroundExecutor`, `libs/langgraph/langgraph/pregel/_executor.py:40-71`) or asyncio (`AsyncBackgroundExecutor`, `_executor.py:122+`). A grep for `.batch(`/`abatch(`/`batch_as_completed` across `libs/langgraph/langgraph` returned no call sites. Genuine batching exists only for *store* operations (`AsyncBatchedBaseStore` micro-batching, `libs/checkpoint/langgraph/store/base/batch.py:326-371`), for *checkpoint writes* (futures coalesced before `checkpointer.put`, `libs/langgraph/langgraph/pregel/_loop.py:1795-1802`), and for *server run creation* in the Python SDK (`create_batch`, `libs/sdk-py/langgraph_sdk/_async/runs.py:607-622`). Whether concurrent-not-batched is "efficient" depends on the provider: it preserves streaming semantics but forfeits provider-side batch discounts and shared prefix cache hits across simultaneous calls.

## Architectural Decisions

1. **Cache state transitions, not function outputs.** The unit of caching is the task's channel writes (what would be applied to state), not the callable's return value. This makes a cache hit equivalent to having executed the node, preserving downstream reducer semantics — evidenced by writes being replayed directly into `output_writes(..., cached=True)` on a hit (`libs/langgraph/langgraph/pregel/_loop.py:1558-1561`) and by the fan-out test asserting identical end state with and without cache (`libs/langgraph/tests/test_pregel.py:2807-2874`).
2. **Opt-in at three granularities.** Per node (`add_node(..., cache_policy=...)`), per graph default applied only where nodes don't specify their own and never to error-handler/interrupt bookkeeping nodes (`libs/langgraph/langgraph/graph/state.py:1288-1334`), and per functional task (`@task(cache_policy=...)`, `libs/langgraph/langgraph/func/__init__.py:115`). Caching is never implicit.
3. **Pluggable backends behind a minimal interface.** Six-method ABC with serde injection (`libs/checkpoint/langgraph/cache/base/__init__.py:15-48`); backends live in separate distribution units (core `checkpoint` lib ships base+memory+redis; sqlite ships its own), matching the repo's dependency map (`AGENTS.md` dependency diagram: checkpoint → langgraph).
4. **Key derivation is delegated but hash-stabilized.** User-suppliable `key_func` produces a string/bytes; the framework then applies non-cryptographic `xxh3_128` hashing and namespacing (`libs/langgraph/langgraph/pregel/_algo.py:1021-1031`), keeping keys short and collision-safe enough for a cache while allowing domain-specific key semantics.
5. **Micro-batching at the storage boundary, not the model boundary.** Batching effort targets store ops and DB round trips (`libs/checkpoint/langgraph/store/base/batch.py`), reflecting LangGraph's position as an orchestrator: it treats model providers as external and leaves request shaping to them.

## Notable Patterns

- **Bulk cache fetch per superstep**: all candidate tasks' keys are gathered and fetched with a single `cache.get(tuple(cached))` / `aget` call rather than per-task lookups (`libs/langgraph/langgraph/pregel/_loop.py:1553-1558`, `1808-1813`) — the cache API itself is batch-shaped (`Sequence[FullKey] -> dict`), pushing batching into the contract.
- **Write-behind caching**: cache population happens asynchronously via `self.submit(self.cache.set, ...)` off the critical path (`_loop.py:1617-1625`, `1875-1883`), so persistence latency doesn't block the superstep.
- **Negative filtering on what gets cached**: interrupts and errors are explicitly excluded from caching on the async path (`if writes[0][0] in (INTERRUPT, ERROR): return`, `_loop.py:1872-1874`), preventing poisoning of the cache with resumable-control records.
- **Dedupe-with-order-preserving batcher**: `_dedupe_ops` maps duplicate ops to the first occurrence's index so results can be scattered back to original callers (`libs/checkpoint/langgraph/store/base/batch.py:283-323`), with last-writer-wins collapse for same-key puts.
- **Graceful degradation in Redis backend**: connection failures degrade to "no cache" behavior (empty results, silent skip) (`libs/checkpoint/langgraph/cache/redis/__init__.py:61-65`, `104-108`) — availability chosen over consistency, with tests locking this in (`libs/checkpoint/tests/test_redis_cache.py:223-247`).

## Tradeoffs

- **Correctness surface vs. cost savings**: because a cache hit replays prior writes, stale entries (e.g., after tool-side effects change the world) return outdated state until TTL expiry; the framework offers TTLs (`types.py:528`) and manual invalidation (`main.py:4136`) but no automatic coherence mechanism.
- **Default key fragility vs. convenience**: the default key function pickles frozen args (`_internal/_cache.py:26-31`); freezing recurses only 10 levels deep (`_freeze(..., depth=10)`, line 7) and objects without `tobytes` fall back to identity hashing, so two equal-but-distinct objects may miss while mutated-in-place inputs may falsely hit. Custom `key_func` is the escape hatch, at the cost of user diligence.
- **Redis silent-failure mode**: swallowing errors maximizes uptime but means a degraded Redis silently disables caching (cost regression invisible to operators); SQLite/in-memory backends surface errors loudly instead.
- **Concurrency vs. batching for models**: per-task concurrency keeps streaming/interrupt semantics intact but gives up provider batch pricing and cross-request prefix reuse; the design pushes that responsibility onto model wrappers outside this repo.
- **Embedding freshness vs. spend**: always re-embedding guarantees correctness when documents change but doubles cost on churn-heavy workloads (every `put` re-embeds even unchanged fields unless `index=False` is passed by the caller).

## Failure Modes / Edge Cases

- **Cache + interrupt interplay**: resuming a thread must not double-execute cached tasks; covered by `test_multiple_interrupts_functional_cache` asserting `counter == 3` across six resumes and again on a fresh thread (`libs/langgraph/tests/test_pregel.py:5745-5797`), and redundant-write suppression tested separately (`test_pregel.py:5907`).
- **Expired-entry handling divergence**: in-memory and SQLite backends lazily purge expired entries on read (`memory/__init__.py:30-31`, `sqlite/__init__.py:63-67`) while Redis relies on server-side `SETEX` expiry (`redis/__init__.py:99-100`) — consistent externally, different internally.
- **Redis key round-trip ambiguity**: namespaces are joined with `:` and split back on parse; namespace components containing `:` would corrupt round-tripping (`redis/__init__.py:31-50`), though the framework's own namespace tuples (`__pregel_ns_writes`, identifiers) avoid colons in practice.
- **Corrupted Redis values skipped individually** rather than failing the batch (`redis/__init__.py:70-76`), trading error visibility for partial-hit tolerance.
- **Cancelled futures in store batcher**: the drain loop checks `fut.done()` before setting results and skips cancelled waiters (`store/base/batch.py:331-333`, `341-343`, `358-361`) to avoid spurious `InvalidStateError`s.
- **Sync store calls on the event loop are rejected up front** to prevent deadlock between the batcher task and the caller (`store/base/batch.py:33-55`).
- **Known security consideration**: the cache layer serializes with `JsonPlusSerializer(pickle_fallback=False)` but inherits msgpack deserialization behavior flagged in the threat model (GHSA-mhr3-j7m5-c7c9, Medium, cache-layer RCE via ext hooks) (`.github/THREAT_MODEL.md:118`, `292`, `461`) — relevant when cache backends share a trust boundary with untrusted writers.

## Future Considerations

- **Response/prompt-cache integration**: a provider-aware prompt-prefix manager (e.g., auto-inserting Anthropic `cache_control` breakpoints or OpenAI automatic-caching hints) would fit naturally next to `CachePolicy`; today the `cache_control` field is only preserved pass-through (`libs/langgraph/langgraph/graph/message.py:149`).
- **Embedding memoization layer**: an LRU or content-hash-keyed vector cache wrapping `embed_documents` in `ensure_embeddings` (`libs/checkpoint/langgraph/store/base/embed.py`) would eliminate repeat costs on churn-heavy stores without touching store implementations.
- **Cache observability**: hit/miss counters, latency, and eviction metrics surfaced through callbacks would make silent Redis degradation visible and enable tuning of TTLs/key functions.
- **True async backends**: `RedisCache.aget/aset` currently delegate to the synchronous client on the event loop thread (`redis/__init__.py:80-82`, `110-112`); native async clients (or `asyncio.to_thread` like the SQLite backend, `sqlite/__init__.py:74-76`) would remove event-loop blocking under load.
- **Model-call batching hook**: a middleware point where independent same-model invocations within a superstep could be grouped for provider batch endpoints, complementing the existing concurrency executor.

## Questions / Gaps

- **No evidence found** for any retrieval-specific cache (e.g., document-store query memoization): searched `libs/` for `retriev.*cach`, `lru_cache` (only unrelated uses at `libs/checkpoint/langgraph/store/memory/__init__.py:479` and `store/base/embed.py:419`), and response-cache patterns around model invocation.
- **No evidence found** for cache hit-rate instrumentation: searched for counters/metrics near `match_cached_writes` and cache backends; none exist.
- Documentation for the caching feature is not present in-repo (the `docs/` directory contains only redirect-generation scripts; no `*.md` mentions `cache_policy`), so operational guidance (TTL sizing, key-function pitfalls) could not be assessed against implementation — inferred intent above is derived from docstrings and tests only.
- Whether `SqliteCache` is exercised by a dedicated unit-test file could not be confirmed (only the parametrized `conftest.py` fixture and `libs/checkpoint/tests/test_redis_cache.py` were found); coverage breadth for the SQLite backend beyond the pregel integration tests is unknown.

---

Generated by `20.02-caching-batching-and-reuse` against `langgraph`.
