# Source Analysis: openai-agents-sdk

## Dimension 15.03: Shared State and Conflict Resolution

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, asyncio-first; pluggable session backends (SQLite, Redis, Dapr, MongoDB, SQLAlchemy); MCP integration; sandbox subsystem |
| Analyzed | 2026-08-26 |

## Summary

The OpenAI Agents SDK (Python) uses a **single shared run context** model: one user-supplied context object plus SDK-owned runtime metadata (approvals, tool invocations, usage) is shared by reference across every agent reached via handoffs and nested `Agent.as_tool()` runs within a single run (`src/agents/run_context.py:72-115`, `docs/context.md:18-41`). This sharing is deliberately unsynchronized — it relies on asyncio's cooperative single-thread scheduling and is documented as user responsibility (`docs/context.md:28-30`).

Where state crosses process/instance boundaries — conversation sessions backed by external stores — the SDK implements **real conflict detection and resolution per backend**: ETag-based optimistic concurrency with bounded retries in `DaprSession`, Redis `WATCH/MULTI` transactions with retryable watch conflicts, MongoDB generation counters with atomic claims, SQLite lock-error retry plus process-wide per-file locks, all exercised by unit tests and container-based integration tests. In-run conflicts are handled with explicit precedence rules (approval beats rejection when sticky decisions conflict, `src/agents/run_context.py:649-652`) and explicit identity-reconciliation classification that returns a literal `"conflict"` verdict for hosted-MCP approval requests (`src/agents/run_internal/tool_execution.py:1321-1360`). Checkpoint/resume isolation deep-copies shared wrapper state to prevent cross-checkpoint contamination (`src/agents/run_context.py:117-130`). The main weaknesses are the absence of any synchronization contract or helper for the user context object, thin conflict logging (most successful conflict resolutions are invisible), and process-global module-level caches for agent-as-tool results whose safety depends on scope IDs.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: External-state conflict handling is mature for an SDK at this layer: explicit `Session` protocol (`src/agents/memory/session.py:15-56`), per-backend optimistic concurrency with bounded backoff retries (`src/agents/extensions/memory/dapr_session.py:250-287`), Redis transactional retry classification (`src/agents/extensions/memory/redis_session.py:470-617`), and a large body of concurrency tests including real-container integration tests (`tests/extensions/memory/test_dapr_redis_integration.py:523-570`). It stops short of 8+ because (a) the primary shared resource — the user context object — has no coordination mechanism or even a documented concurrency contract beyond "you share it", (b) conflict resolutions inside sessions are mostly silent (only degraded metadata writes warn, `src/agents/extensions/memory/dapr_session.py:436-442`), and (c) agent-tool result caches are process-globals guarded by convention (`src/agents/agent_tool_state.py:35-48`).

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared context API | `RunContextWrapper` wraps the user context passed to `Runner.run()`; "every agent, tool function, lifecycle etc for a given agent run must use the same type of context" | `src/agents/run_context.py:72-83`; `docs/context.md:18` |
| Shared runtime metadata | Derived wrappers share `_approvals` and `_tool_invocations` dicts **by reference** via `_share_tool_state_with` | `src/agents/run_context.py:108-115` |
| Tool-context sharing | `ToolContext.from_*` calls `context._share_tool_state_with(tool_context)` so nested agent-tool runs see the same approval state | `src/agents/tool_context.py:290-291` |
| Hook-context sharing | Agent-hook contexts share tool state with the run wrapper | `src/agents/run_internal/run_loop.py:2082`; `src/agents/run_internal/run_loop.py:2414`; `src/agents/run_internal/turn_resolution.py:346` |
| Documented sharing semantics | "Within a single run, derived wrappers share the same underlying app context, approval state, and usage tracking" | `docs/context.md:30-38` |
| Usage aggregation | `Usage.add` mutates in place; run-wide token accounting across agents | `src/agents/usage.py:257-312` |
| Session sharing across agents | Docs show two agents sharing one `SQLiteSession("user_123")` instance ("Both agents will see the same conversation history") | `docs/sessions/index.md:564-577` |
| Session protocol | `Session` Protocol defines `get_items/add_items/pop_item/clear_session` as the shared-memory interface | `src/agents/memory/session.py:15-56` |
| Dapr conflict detector | `_is_concurrency_conflict` matches gRPC `ABORTED`/`FAILED_PRECONDITION` and etag/precondition message markers | `src/agents/extensions/memory/dapr_session.py:256-277` |
| Dapr resolution handler | Read-modify-write with stored etag + `Concurrency.first_write`; retry loop up to 5 attempts with exponential backoff + 10% jitter | `src/agents/extensions/memory/dapr_session.py:65-67`; `src/agents/extensions/memory/dapr_session.py:250-254`; `src/agents/extensions/memory/dapr_session.py:364-393` |
| Dapr graceful degradation | Metadata write failure after items committed logs warning instead of failing the append (avoids double-append on caller retry) | `src/agents/extensions/memory/dapr_session.py:395-443` |
| Dapr documented race gap | Concurrent etag-less creation is explicitly documented last-write-wins | `src/agents/extensions/memory/dapr_session.py:399-402` |
| Redis conflict detector | `EXEC` returning `None` inside pipeline classified as `retryable_watch_conflict`; `WatchError` caught | `src/agents/extensions/memory/redis_session.py:506-513`; `src/agents/extensions/memory/redis_session.py:557-558` |
| Redis resolution handler | `_write_items` loops on retryable watch conflicts; ambiguous EXEC outcomes deliberately not retried | `src/agents/extensions/memory/redis_session.py:587-617`; `tests/extensions/memory/test_redis_session.py:1551` |
| Redis atomic counters | Message ordering via atomic `INCR`; `created_at` protected with `HSETNX` | `src/agents/extensions/memory/redis_session.py:449-452`; `src/agents/extensions/memory/redis_session.py:542-543` |
| MongoDB generation guard | History generation counter queried per operation; clear bumps `_generation`; pop re-checks generation after concurrent clear | `src/agents/extensions/mongodb_session.py:273-286`; `tests/extensions/memory/test_mongodb_session.py:511`; `tests/extensions/memory/test_mongodb_session.py:1257-1292` |
| SQLAlchemy lock retry | `_is_sqlite_lock_error` gates retry logic for SQLite-backed engines | `src/agents/extensions/memory/sqlalchemy_session.py:126-140` |
| SQLite process-local locks | Class-wide per-file `RLock` registry with refcounted release serializes all sessions on one DB file | `src/agents/memory/sqlite_session.py:51-53`; `src/agents/memory/sqlite_session.py:118-142` |
| SQLite WAL + connections | WAL journal mode with busy-timeout retry loop; thread-local connections for file DBs, single shared connection + `RLock` for `:memory:` | `src/agents/memory/sqlite_session.py:89-101`; `src/agents/memory/sqlite_session.py:211-219` |
| Approval decision precedence | Exact call-ID decisions override sticky defaults; if sticky approved and rejected both set, "Approval takes precedence when sticky decisions conflict" | `src/agents/run_context.py:644-661` |
| Hosted-MCP reconciliation | `_classify_hosted_mcp_pending_request` returns `"conflict"` when pending/current request identities mismatch | `src/agents/run_internal/tool_execution.py:1321-1360` |
| Checkpoint isolation | `_copy_for_run_state` deep-copies usage/approvals/tool-invocations so resumed checkpoints don't cross-contaminate (comment documents exact failure mode) | `src/agents/run_context.py:117-130`; `src/agents/run_state.py:902`; `src/agents/result.py:568` |
| Concurrency limiting | `RunConfig.tool_execution.max_function_tool_concurrency` caps concurrent function tools; slot-filling scheduler drains tasks with `asyncio.wait(FIRST_COMPLETED)` | `src/agents/run_config.py:139-156`; `src/agents/run_internal/tool_execution.py:1643-1683` |
| MCP request serialization | Optional `_request_lock` serializes requests over one shared MCP `ClientSession`; manager-level lifecycle lock coordinates connect/close | `src/agents/mcp/server.py:932-960`; `src/agents/mcp/manager.py:212` |
| Compaction serialization | `OpenAIResponsesCompactionSession._mutation_lock` serializes mutations against locked compaction replacement | `src/agents/memory/openai_responses_compaction_session.py:144`; documented caveat at `docs/sessions/index.md:289` |
| Sandbox memory locks | Per-manager `_flush_lock` guards rollout enqueue/flush; storage `_layout_lock` guards workspace layout creation; managers registered per-session in a `WeakKeyDictionary` | `src/agents/sandbox/memory/manager.py:38-40`; `src/agents/sandbox/memory/manager.py:60,97`; `src/agents/sandbox/memory/storage.py:69,95-105` |
| Sandbox cleanup lock | `_SandboxSessionResources._cleanup_lock` makes concurrent cleanup idempotent | `src/agents/sandbox/runtime_session_manager.py:57` |
| Agent-tool result cache | Process-global dicts keyed `(scope_id, id(tool_call))` with signature fallback index and weakref GC hooks; scope IDs isolate independently restored states | `src/agents/agent_tool_state.py:33-48`; `src/agents/agent_tool_state.py:87-91` |
| Conflict logging | Only degraded metadata writes log warnings; exhausted Dapr retries raise; no dedicated conflict event stream | `src/agents/extensions/memory/dapr_session.py:436-443` |
| Integration test under contention | Two `DaprSession` instances on the same `session_id` fire 20 parallel `add_items`; resolution asserted via optimistic concurrency (container-gated) | `tests/extensions/memory/test_dapr_redis_integration.py:523-570` |
| Unit tests for conflict paths | `test_add_items_retries_on_concurrency`, `test_pop_item_retries_on_concurrency`, `test_concurrent_access` (Dapr); `test_concurrent_access`, watch-retry tests (Redis); concurrent pop/claim tests (SQLAlchemy, advanced SQLite) | `tests/extensions/memory/test_dapr_session.py:303,327,571`; `tests/extensions/memory/test_redis_session.py:415,1485`; `tests/extensions/memory/test_sqlalchemy_session.py:241,375`; `tests/extensions/memory/test_advanced_sqlite_session.py:2811,4249` |

## Answers to Dimension Questions

### 1. What state is shared between agents?

Four tiers:

- **App context (user-owned)**: one object passed to `Runner.run(..., context=...)` is visible to every agent, tool, guardrail, and hook in the run through `RunContextWrapper.context` (`src/agents/run_context.py:80-81`), including after handoffs (`docs/handoffs.md:90-98`) and inside nested `Agent.as_tool()` runs (`docs/context.md:30`). This is the intended cross-agent data bus.
- **SDK-owned runtime metadata**: approval records and tool-invocation identity records live on the wrapper and are shared by reference into every derived `ToolContext`/hook context (`src/agents/run_context.py:108-115`, `src/agents/tool_context.py:290`), so an approval granted to one agent's tool is honored when another agent re-triggers the same tool identity.
- **Conversation memory (sessions)**: a `Session` instance can be attached to runs of different agents, making history a shared artifact across agents and processes (`docs/sessions/index.md:564-577`).
- **Environment artifacts**: sandbox workspaces and generated memory files are shared per sandbox session, coordinated by the generation manager registry and its flush/layout locks (`src/agents/sandbox/memory/manager.py:38-64`).

Tools themselves are *not* shared state: each `Agent` holds its own tool list; only MCP server connections are shared objects with their own serialization locks (`src/agents/mcp/server.py:930-960`).

### 2. How are conflicts detected?

- **External stores**: version/condition checks — Dapr etag comparison on conditional write with `Concurrency.first_write` (`src/agents/extensions/memory/dapr_session.py:380-387`); Redis `WATCH` + `MULTI/EXEC` where a `None` EXEC result marks a lost race (`src/agents/extensions/memory/redis_session.py:506-513`); MongoDB generation mismatch detection (`src/agents/extensions/memory/mongodb_session.py:273-286`); SQLite `OperationalError` lock classification (`src/agents/extensions/memory/sqlalchemy_session.py:126`).
- **In-run identity conflicts**: hosted-MCP approval reconciliation compares request ID, server label, and resolved tool name, returning `"conflict"` when identities diverge (`src/agents/run_internal/tool_execution.py:1324-1349`).
- **Decision-record conflicts**: sticky approve+reject flags on the same approval key detected during status resolution (`src/agents/run_context.py:650-652`).

### 3. How are conflicts resolved?

- **Retry with backoff**: Dapr retries up to 5 attempts with exponential backoff plus jitter, re-reading fresh state each attempt (`src/agents/extensions/memory/dapr_session.py:250-287`, `368-393`). Redis re-runs the whole watched transaction (`src/agents/extensions/memory/redis_session.py:593-617`). Ambiguous outcomes (EXEC result never observed) are treated as failures, not retried blindly (`tests/extensions/memory/test_redis_session.py:1551`).
- **Precedence rules**: approval wins over rejection when sticky decisions conflict (`src/agents/run_context.py:649-652`); per-call exact decisions override sticky defaults (`src/agents/run_context.py:637-647`).
- **Isolation instead of merge**: conflicting checkpoint lineages get deep-copied state rather than merging (`src/agents/run_context.py:117-130`); independently restored agent-tool states are isolated by scope ID so identical call signatures don't collide (`src/agents/agent_tool_state.py:87-91`, `214-230` returns `None` when more than one candidate matches — refuse-to-guess semantics).
- **Graceful degradation**: non-critical metadata updates fail soft with a warning after items are durably committed (`src/agents/extensions/memory/dapr_session.py:404-407`).
- **Throttling**: `max_function_tool_concurrency` bounds how many concurrent writers can touch shared context/tools at once (`src/agents/run_config.py:139-156`).

### 4. Is shared state consistent?

Consistency guarantees are layered and honest about gaps:

- Within one asyncio run, single-threaded event-loop execution makes shared-wrapper mutation effectively serialized without locks.
- Across instances/processes, sessions offer read-your-writes per key with optimistic concurrency; Dapr eventual-consistency mode is default but strong consistency is selectable (`src/agents/extensions/memory/dapr_session.py:62-63,94-96`), and Redis/Mongo paths are transactionally atomic per operation.
- Documented residual races exist and are called out in-code and in docs: concurrent etag-less first creation in Dapr is last-write-wins (`src/agents/extensions/memory/dapr_session.py:399-402`); compaction can overwrite a mutation that commits while the remote request is in flight, with docs instructing users not to mutate concurrently (`docs/sessions/index.md:289`).
- The user-owned `context` object has no consistency guarantee whatsoever; correctness under parallel tool calls rests entirely on application code.

## Architectural Decisions

1. **Context is shared-by-reference, not copied.** Handoffs and nested agent-as-tool runs receive the same underlying object; only SDK-managed metadata fields are explicitly shared through `_share_tool_state_with` (`src/agents/run_context.py:108-115`). Copying is reserved for checkpoint boundaries (`src/agents/run_context.py:117-130`), keeping steady-state overhead near zero.
2. **Conflict handling lives in the persistence adapters, not the core loop.** The core `Session` protocol is conflict-agnostic (`src/agents/memory/session.py:15-56`); each backend encodes its store's native mechanism (etag/WATCH/generation/file-lock). This keeps `Runner` free of storage policy.
3. **Refuse-to-guess over heuristic merge.** Where identities cannot be reconciled — hosted MCP approvals (`tool_execution.py:1328-1333`), ambiguous multi-candidate agent-tool cache hits (`agent_tool_state.py:224-225`, `247-249`) — the code returns `"conflict"`/`None` instead of picking arbitrarily.
4. **Bounded optimistic retries with jitter**, mirroring the tracing processors' approach, to avoid thundering herds (`src/agents/extensions/memory/dapr_session.py:253-254`).

## Notable Patterns

- **Scope-keyed process globals with weakref lifecycle**: the agent-as-tool result cache indexes by `(scope_id, id(obj))` plus a value-signature fallback, and registers weakref callbacks to self-clean on GC (`src/agents/agent_tool_state.py:131-156`). Scope IDs minted per restored state prevent resume-lineage collisions (`src/agents/run_context.py:128-130`).
- **Class-level lock registries**: `SQLiteSession._file_locks` maps resolved DB path → refcounted `RLock`, so multiple session instances on one file coordinate without a global bottleneck on unrelated files (`src/agents/memory/sqlite_session.py:118-142`).
- **Per-instance `asyncio.Lock` + backend-level concurrency control**: instance locks (`dapr_session.py:110`, `redis_session.py:393`) serialize intra-instance operations, while backend conditional writes cover inter-instance races — a two-layer scheme.
- **Test-first conflict coverage**: conflict behavior is pinned at three levels — fake-client unit tests simulating etag conflicts (`tests/extensions/memory/test_dapr_session.py:303-344`), real-container integration with two contending session instances (`tests/extensions/memory/test_dapr_redis_integration.py:523-570`), and cancellation/ambiguity edge tests (`tests/extensions/memory/test_redis_session.py:1485,1551`).

## Tradeoffs

- **Simplicity vs safety for user context**: sharing by reference makes handoffs cheap and ergonomically simple, but there are no locks, snapshots, or even warnings for concurrent mutation from parallel tool calls; the SDK outsources this entirely to applications (`docs/context.md:28-30`).
- **Aggregate rewrite vs append-only**: Dapr stores messages as one JSON blob rewritten per append (`src/agents/extensions/memory/dapr_session.py:370-388`) — simple etag semantics but O(n) writes and larger conflict windows than Redis's list `RPUSH` inside a transaction.
- **Serialization vs throughput**: SQLiteSession's single per-file RLock serializes everything for safety (`sqlite_session.py:144-148`), trading concurrency for guaranteed ordering; heavier deployments must move to Redis/Postgres-class backends.
- **Silent success of conflict resolution**: retried-and-committed operations leave little trace, which reduces noise but also observability (see Failure Modes).

## Failure Modes / Edge Cases

- **Exhausted retries raise**: after 5 conflicted attempts Dapr raises the original error (`src/agents/extensions/memory/dapr_session.py:282-283`, `389-393`); callers see failure, but there is no conflict-specific exception type.
- **Last-write-wins creation window**: two sessions creating metadata concurrently without an existing etag can clobber `created_at` — explicitly accepted (`src/agents/extensions/memory/dapr_session.py:399-402`).
- **Compaction vs concurrent mutation**: a committed mutation can be overwritten by an in-flight compaction replacement; recovery restores prior history only if the backend permits (`docs/sessions/index.md:289`, implementation locking at `src/agents/memory/openai_responses_compaction_session.py:144`).
- **Ambiguous commit outcomes**: Redis EXEC responses lost to connection failure are treated as non-retryable to avoid double-appends (`tests/extensions/memory/test_redis_session.py:1551`).
- **Free-threading exposure**: module-global caches in `agent_tool_state.py:37-48` and `manager.py:38-40` rely on GIL-era assumptions; they have no thread synchronization, a latent hazard under free-threaded CPython or multi-threaded drivers.
- **Cross-checkpoint usage bleed**: prevented by design — the comment at `src/agents/run_context.py:120-124` records the exact bug class (shared `Usage` landing tokens on every sibling checkpoint) that deep-copying eliminates.

## Future Considerations

- Provide an optional context-guard utility (per-run `asyncio.Lock` or copy-on-write snapshot API) for applications that mutate shared context from parallel tool calls; today the only lever is `max_function_tool_concurrency` (`src/agents/run_config.py:139`).
- Emit a tracing span or structured log event when a session write succeeds after N conflicts, making contention observable without waiting for exhaustion.
- Consider chunked/append-friendly storage shapes for aggregate-blob backends (Dapr) to shrink conflict windows on long histories.
- Audit module-global registries for free-threaded Python readiness (locks around `_MEMORY_GENERATION_MANAGERS`, `agent_tool_state` maps).

## Questions / Gaps

- No evidence found of a general-purpose, SDK-provided mutex or transactional helper for user context objects; searches covered `src/agents/*.py` for `Lock|Semaphore|threading`, `docs/context.md`, and `docs/handoffs.md`. Sharing is documented, coordination is not.
- No evidence found of conflict-specific telemetry (metrics/counters) anywhere in `src/agents/` or `src/agents/extensions/memory/`; only warning/error logging on terminal outcomes (e.g., `src/agents/extensions/memory/dapr_session.py:436-443`).
- Whether `RedisSession.get_items` reads need serialization against concurrent writers was not verifiable from source alone (Redis list reads are atomic snapshots per command, but multi-read consistency across keys is undocumented in-code); no test pins read-during-write behavior.
- Sandbox workspace file-level conflicts (two agents writing the same workspace path) showed capability plumbing (`src/agents/sandbox/capabilities/capability.py:80-85` accepts lock primitives) but no end-to-end detector/resolver for file-content conflicts; treat sandbox file coordination as out-of-scope for the current implementation.

---

Generated by `Dimension 15.03: Shared State and Conflict Resolution` against `openai-agents-sdk`.
