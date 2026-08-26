# Source Analysis: langgraph

## 07.03 — Idempotency and Retry Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph` core, `libs/prebuilt` agents/tools, `libs/checkpoint*` persistence, `libs/sdk-py`/`libs/sdk-js` clients) |
| Analyzed | 2026-08-26 |

## Summary

LangGraph separates retry into two orthogonal layers with very different idempotency guarantees:

1. **Framework-level task retries** (`RetryPolicy` + `run_with_retry`/`arun_with_retry`) re-execute a whole node attempt on transient failures. They protect *graph state* consistency by clearing the failed attempt's pending writes before each retry (`task.writes.clear()`, `libs/langgraph/langgraph/pregel/_retry.py:615` and `:738`), but they offer **zero protection for external side effects** — if a node already sent an email before raising, the retry will send it again. There are no idempotency keys, no dedup store, and no side-effect ledger anywhere in the codebase (a repo-wide search for `idempot` returns hits only in SDK stream-handle bookkeeping and checkpoint conformance tests, never in tool execution paths).
2. **Checkpoint-based execution dedup** makes *task completion* idempotent across process crashes and resumes. Task IDs are deterministic hashes of `(checkpoint_id, checkpoint_ns, step, node_name, kind, triggers)` (`libs/langgraph/langgraph/pregel/_algo.py:614-624`, hash functions at `_algo.py:1395-1409`), so a resumed run regenerates the same task ID; persisted writes are upserted under that key with `ON CONFLICT ... DO NOTHING/UPDATE` semantics (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-159`, `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:463-465`), and on resume, tasks whose writes were already saved are skipped instead of re-executed (`_reapply_writes_to_succeeded_nodes`, `libs/langgraph/langgraph/pregel/_loop.py:736-749`; runner filter `[t for t in loop.tasks.values() if not t.writes]`, `libs/langgraph/langgraph/pregel/main.py:2968`). This is proven by `test_send_dedupe_on_resume` (`libs/langgraph/tests/test_pregel_async.py:2530`) and `test_put_writes_idempotent` (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py:148`).

For non-idempotent tools specifically (payment/email/delete), LangGraph's answer is **not automatic** — it is architectural: opt-in human-in-the-loop interrupts before execution (`interrupt()`, `libs/langgraph/langgraph/types.py:851-974`; `HumanInterruptConfig`, `libs/prebuilt/langgraph/prebuilt/interrupt.py:11-26`), error-to-model feedback loops via `ToolMessage(status="error")` (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012`), and optional node caching keyed by input hash (`CachePolicy`, `libs/langgraph/langgraph/types.py:520-529`). A naive `RetryPolicy(retry_on=<anything>)` on such a node **can duplicate side effects**, because retries happen at whole-node granularity after any external call may already have landed.

Rating rationale follows below.

## Rating

**7 / 10.**

The framework has a clear, well-tested retry model for its own unit of work (the task): explicit policy dataclass, pluggable exception classification (`default_retry_on`), exponential backoff with jitter, first-match-wins multi-policy support, per-attempt write clearing, timeout integration, and deterministic-ID-based dedup of completed work across crashes — all backed by an extensive dedicated test file (~2,900 lines, `libs/langgraph/tests/test_retry.py`) and conformance tests for saver idempotency. What keeps it out of 8–10 territory: there is no mechanism whatsoever for making *external* side effects idempotent (no idempotency-key plumbing, no attempt records exposed to tools, no transactional outbox); default retry classification is heuristic ("everything unknown is retried", `_internal/_retry.py:29`), which is the dangerous default for side-effecting nodes; and in-process retry attempts are invisible in streams (only server logs plus an underscore-private observer contract).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Retry wrapper (sync) | `run_with_retry(task, retry_policy)` loop with backoff/jitter/max_attempts | libs/langgraph/langgraph/pregel/_retry.py:573-683 |
| Retry wrapper (async) | `arun_with_retry(...)` same semantics + timeout/observer integration | libs/langgraph/langgraph/pregel/_retry.py:685-838 |
| Policy shape | `RetryPolicy(initial_interval=0.5, backoff_factor=2.0, max_interval=128.0, max_attempts=3, jitter=True, retry_on=default_retry_on)` | libs/langgraph/langgraph/types.py:418-437 |
| Error classification | `default_retry_on`: retry ConnectionError & HTTP 5xx; do NOT retry ValueError/TypeError/OSError/etc.; retry unknown exceptions by default | libs/langgraph/langgraph/_internal/_retry.py:1-29 |
| Classification dispatch | `_should_retry_on`: `retry_on` as class / sequence / callable | libs/langgraph/langgraph/pregel/_retry.py:841-854 |
| Multi-policy matching | First matching policy wins per exception (`matching_policy = None; for policy in retry_policy: ...`) | libs/langgraph/langgraph/pregel/_retry.py:647-654 |
| Attempt-write hygiene | `task.writes.clear()` before every attempt (sync+async) prevents partial graph-state leakage between attempts | libs/langgraph/langgraph/pregel/_retry.py:615, 738 |
| Timeout × retry | Timeout watchdogs convert to `NodeTimeoutError`; stale writes cleared on timeout (`task.writes.clear()`); NodeTimeoutError is retryable when policy allows | libs/langgraph/langgraph/pregel/_retry.py:486-502; tests at libs/langgraph/tests/test_retry.py:786, 1091-1156 |
| Subgraph resume signal | On retry, `CONFIG_KEY_RESUMING=True` patched into config so subgraphs resume instead of restart | libs/langgraph/langgraph/pregel/_retry.py:682, 838 |
| Deterministic task IDs (idempotency key) | `task_id = xxh3(checkpoint_id, checkpoint_ns, step, name, PULL, triggers)`; uuid5 variant for old checkpoints | libs/langgraph/langgraph/pregel/_algo.py:550, 614-624, 1395-1409 |
| Saver-level dedup (Postgres) | `UPSERT_CHECKPOINT_WRITES_SQL ... ON CONFLICT (...) DO UPDATE` / `INSERT_CHECKPOINT_WRITES_SQL ... DO NOTHING` keyed on (thread, ns, checkpoint_id, task_id, idx) | libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-159 |
| Saver-level dedup (SQLite) | `INSERT OR REPLACE` / `INSERT OR IGNORE INTO writes ...` keyed on task_id+idx | libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:463-465 |
| Conformance test for write idempotency | `test_put_writes_idempotent`: duplicate `(task_id, idx)` must not duplicate rows | libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py:148-167 |
| Crash-resume dedup | `_reapply_writes_to_succeeded_nodes` restores persisted writes onto replayed tasks (skipping ERROR/INTERRUPT/RESUME) so runner skips them | libs/langgraph/langgraph/pregel/_loop.py:736-749 |
| Runner skip filter | Runner executes only `[t for t in loop.tasks.values() if not t.writes]` | libs/langgraph/langgraph/pregel/main.py:2965-2968 (async twin at :3438) |
| Resume-dedup test | `test_send_dedupe_on_resume`: "node '2' doesn't get called again, as we recover writes saved before" | libs/langgraph/tests/test_pregel_async.py:2530-2604 |
| Failed-task handling on resume | Failed tasks keep empty writes (ERROR channel skipped), routed to error handler instead of blind re-execution | libs/langgraph/langgraph/pregel/_loop.py:751-816 |
| Error persistence | `commit()` appends `(ERROR, exc)` (+ `ERROR_SOURCE_NODE`) and persists via `put_writes`; successful no-op tasks get `(NO_WRITES, None)` marker | libs/langgraph/langgraph/pregel/_runner.py:574-613 |
| Nested `@task` duplicate detection | If parent was retried and child task already ran, reuse its RETURN/ERROR future instead of rescheduling | libs/langgraph/langgraph/pregel/_runner.py:734-756 |
| Node result cache | `CachePolicy(key_func=default_cache_key, ttl)` → `CacheKey(ns=CACHE_NS_WRITES, key=xxh3(input))`; cache hit short-circuits execution in `arun_with_retry` (`match_cached_writes`) | libs/langgraph/langgraph/types.py:520-529; libs/langgraph/langgraph/pregel/_retry.py:714-718; libs/langgraph/langgraph/pregel/_loop.py:1549-1562; libs/langgraph/langgraph/_internal/_cache.py:26-31 |
| Cache backend interface | `BaseCache.get/set/clear` abstract contract | libs/checkpoint/langgraph/cache/base/__init__.py:15-48 |
| HITL guard for risky tools | `interrupt()` raises GraphInterrupt, resumes by replaying stored resume values; doc states node logic re-executes from start on resume | libs/langgraph/langgraph/types.py:851-974 (esp. :864, :955-965) |
| HITL response schema | `HumanResponse` accept/ignore/response/edit | libs/prebuilt/langgraph/prebuilt/interrupt.py:87-105 |
| Tool errors → model visibility | Handled tool exceptions become `ToolMessage(content=..., status="error")` returned to the LLM | libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012 |
| Tool error templates | "Please fix your mistakes." / "Please fix the error and try again." prompts drive model-driven retry of tool calls | libs/prebuilt/langgraph/prebuilt/tool_node.py:108-121 |
| Tool-call wrapper hook for custom retry | `wrap_tool_call(request, execute)` — "execute CAN BE CALLED MULTIPLE TIMES" for retry middleware | libs/prebuilt/langgraph/prebuilt/tool_node.py:202-217 |
| Retry observability (logs) | `logger.info("Retrying task {name} after {sleep:.2f} seconds (attempt N) ...")` | libs/langgraph/langgraph/pregel/_retry.py:677-680, 833-836 |
| Retry observability (server contract) | `_AttemptContext`/`_AttemptEvent` start/progress/finish events incl. `attempt` number; explicitly internal for langgraph-server | libs/langgraph/langgraph/pregel/_retry.py:87-127, 343-404 |
| Attempt counter exposed to nodes | `Runtime.execution_info.node_attempt` (1-indexed) and `node_first_attempt_time` | libs/langgraph/langgraph/runtime.py:49-52; set at libs/langgraph/langgraph/pregel/_retry.py:600-612 |
| Durability tiers governing crash-recovery strength | `Durability = Literal["sync","async","exit"]` — sync waits for checkpoint future each step (main.py:2987-2988) | libs/langgraph/langgraph/types.py:89-95 |

## Answers to Dimension Questions

### 1. Which tool failures are retried?

Retries apply to **node/task failures**, not to individual tool calls. The default classifier `default_retry_on` (`libs/langgraph/langgraph/_internal/_retry.py:1-29`) retries: `ConnectionError`, `httpx.HTTPStatusError` with status ≥500, `requests.HTTPError` with status ≥500 or missing response, and — critically — **any exception type not on the explicit non-retry list**. Non-retried by default: `ValueError`, `TypeError`, `ArithmeticError`, `ImportError`, `LookupError`, `NameError`, `SyntaxError`, `RuntimeError`, `ReferenceError`, `StopIteration`, `StopAsyncIteration`, `OSError`. This whitelist-the-rest-retry posture means application-specific exceptions from tools (custom `Exception` subclasses) are retried by default, verified in `test_should_retry_default_retry_on` (`libs/langgraph/tests/test_retry.py:175-246`, custom exception → `True`). Users override per node or per `Send`/`@task` call via `RetryPolicy.retry_on` as class, sequence, or predicate (`libs/langgraph/langgraph/pregel/_retry.py:841-854`), with multiple policies matched first-match-wins (`libs/langgraph/langgraph/pregel/_retry.py:647-654`, test at `libs/langgraph/tests/test_retry.py:379-444`). `NodeTimeoutError` is retryable by default (`libs/langgraph/tests/test_retry.py:233-240`). At the tool layer, `ToolNode` does not auto-retry; unhandled exceptions propagate as node failures, while handled ones become error `ToolMessage`s (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012`).

### 2. Are repeated attempts safe?

Safe with respect to **graph state**: every attempt starts with `task.writes.clear()` (`libs/langgraph/langgraph/pregel/_retry.py:615, 738`), timeouts also clear stale writes including late writes from cancelled background tasks (`libs/langgraph/langgraph/pregel/_retry.py:492-502`; tests `test_arun_with_retry_timeout_discards_pre_timeout_writes`, `...discards_stale_executor_writes`, `libs/langgraph/tests/test_retry.py:1091-1156`), and the guarded-config scope blocks writes after the timeout boundary (`_TimedAttemptScope._guard_send`, `libs/langgraph/langgraph/pregel/_retry.py:225-235`). Not safe with respect to **external systems**: nothing tracks whether a retried node's earlier attempt already mutated an external service. The only mitigations are opt-in: `interrupt()`-based approval before the side effect (`libs/langgraph/langgraph/types.py:851-974`), `CachePolicy` to skip re-execution for identical inputs (`libs/langgraph/langgraph/types.py:520-529`), or user-written `wrap_tool_call` retry middleware (`libs/prebuilt/langgraph/prebuilt/tool_node.py:202-217`).

### 3. Is retry state persisted?

Partially. In-flight retry state (attempt counter, backoff clock) is **not persisted** — it lives in local variables of `run_with_retry`/`arun_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:584-600, 698-719`); a crash mid-retry-sequence loses the count. However, the *outcomes* of attempts are persisted durably: success writes, `NO_WRITES` markers, `ERROR`/`ERROR_SOURCE_NODE` records, `INTERRUPT`, and `RESUME` values are all stored as checkpoint pending writes keyed by deterministic task IDs (`commit()` at `libs/langgraph/langgraph/pregel/_runner.py:574-613`; deterministic IDs at `libs/langgraph/langgraph/pregel/_algo.py:616-623`). On resume this yields well-defined behavior without re-running succeeded work (`libs/langgraph/langgraph/pregel/_loop.py:736-749`) and routing previously-failed tasks to error handlers rather than blind re-execution (`libs/langgraph/langgraph/pregel/_loop.py:751-816`; crash-resume tests at `libs/langgraph/tests/test_retry.py:2689-2825`). How much history exists to recover depends on durability mode (`sync`/`async`/`exit`, `libs/langgraph/langgraph/types.py:89-95`): under `exit`, intermediate checkpoints aren't written during the run, weakening crash-recovery dedup.

### 4. Are non-idempotent tools protected?

Not automatically. The framework provides building blocks, none of which fire by default: (a) human approval interrupts with accept/edit/response options (`libs/prebuilt/langgraph/prebuilt/interrupt.py:11-26, 87-105`); (b) error-handler nodes that run after retry exhaustion and can route with `Command` (`test_graph_error_handler_runs_after_retry_exhaustion`, `libs/langgraph/tests/test_retry.py:2009`); (c) input-hash node caching (`libs/langgraph/langgraph/pregel/_loop.py:1609-1625`). Note that even the interrupt mechanism documents that on resume "the graph resumes from the start of the node, **re-executing** all logic" (`libs/langgraph/langgraph/types.py:864`) — so code placed *before* an `interrupt()` call (e.g., a charge already issued) still re-runs; the pattern only protects effects sequenced *after* the approved gate. There is no idempotency-key parameter on tools, no attempt token injected into `ToolRuntime`, and no duplicate-suppression window.

### 5. Can retries create duplicate side effects?

Yes, by design. `run_with_retry` re-invokes `task.proc.invoke(task.input, config)` wholesale (`libs/langgraph/langgraph/pregel/_retry.py:617`); any HTTP POST, email send, or DB mutation performed inside the node before the exception will be repeated on attempt N+1. The default `retry_on` classifier amplifies this risk by retrying unknown/custom exceptions (`libs/langgraph/langgraph/_internal/_retry.py:29`). Duplicate *execution* (as opposed to external effects) is what the system guards against: deterministic task IDs + `ON CONFLICT` write upserts + the `not t.writes` runner filter prevent double-commit of results across retries and crash resumes (`libs/langgraph/langgraph/pregel/_algo.py:614-624`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-159`, `libs/langgraph/langgraph/pregel/main.py:2968`, `libs/langgraph/tests/test_pregel_async.py:2530-2604`). For `@task` functional calls, an already-completed child is detected and its recorded RETURN/ERROR reused instead of rescheduling (`libs/langgraph/langgraph/pregel/_runner.py:742-756`).

## Architectural Decisions

1. **Retry at task granularity, not tool-call granularity.** The retry unit is a Pregel task (node invocation or `@task` call). Tools get no framework-managed retry; `ToolNode` deliberately converts handled errors into model-visible `ToolMessage`s (`libs/prebuilt/langgraph/prebuilt/tool_node.py:1005-1012`), delegating "should we try again?" to the LLM — which makes model-driven retries fully auditable in message history.
2. **Deterministic task identity as the idempotency substrate.** Instead of idempotency keys passed to tools, langgraph derives stable task IDs from checkpoint coordinates (`libs/langgraph/langgraph/pregel/_algo.py:614-624`). All dedup — write upserts, resume skips, cached-write matching — hangs off this identity. Versioned hash functions (`xxh3` vs `uuid5` selected by checkpoint version, `_algo.py:550`) preserve compatibility with old persisted state.
3. **Whitelist-blacklist inversion in default classification.** `default_retry_on` treats known programming errors as fatal and everything else as transient (`libs/langgraph/langgraph/_internal/_retry.py:11-29`). This favors liveness over safety and pushes the burden of marking non-idempotent operations onto the developer via explicit policies.
4. **Writes are buffered per attempt and committed atomically at commit time.** Node outputs accumulate in `task.writes` (deque) and reach the checkpointer only via `commit()` (`libs/langgraph/langgraph/pregel/_runner.py:604-613`), which is why `writes.clear()` between attempts gives all-or-nothing graph-state semantics per attempt.
5. **Crash recovery distinguishes "done" from "failed".** Persisted ERROR-channel writes mark failed tasks; on resume they are not silently re-run but handed to error handlers (`libs/langgraph/langgraph/pregel/_loop.py:751-816`). Retrying a failed task across processes requires an explicit new invocation (e.g., `Command(resume=...)` or time-travel fork).

## Notable Patterns

- **Guarded-config scope for timed attempts** (`_TimedAttemptScope.wrap_config`, `libs/langgraph/langgraph/pregel/_retry.py:169-193`): send/stream/call/runtime functions are wrapped so post-timeout writes are physically blocked and progress signals refresh idle timers — a disciplined way to fence zombie attempts.
- **Attempt metadata as a named internal contract**: `_AttemptContext`/`_AttemptEvent` carry `attempt`, timestamps, and error types to the server observer while being kept out of the public API (`libs/langgraph/langgraph/pregel/_retry.py:87-127`).
- **Conformance-test enforcement of saver idempotency**: duplicate-write suppression is part of the checkpointer spec every backend must pass (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py:148-167`), not just an implementation detail of Postgres.
- **Resume-value replay for interrupts** (`libs/langgraph/langgraph/types.py:955-965`): positional matching of resume values against a scratchpad counter makes repeated node executions converge instead of re-prompting.
- **First-match-wins policy lists** let users express "retry ValueError twice, KeyError three times" declaratively (`libs/langgraph/tests/test_retry.py:379-444`).

## Tradeoffs

- **Liveness vs. safety in defaults:** retry-by-default for unknown exceptions maximizes recovery from transient provider faults but is exactly wrong for payment/delete-style tools whose failure modes surface as custom exceptions.
- **Task-granularity retry simplicity vs. wasted work / repeated effects:** whole-node re-execution is simple and state-consistent, but re-does all side effects and computation rather than resuming mid-node.
- **In-memory attempt counters vs. durable retry ledgers:** cheap and race-free, but a crash during backoff loses retry history; combined with `durability="exit"` (`libs/langgraph/langgraph/types.py:89-95`), crash windows can lose more work.
- **Model-driven tool retries vs. programmatic retries:** error `ToolMessage`s give explainability and human-readable audit trail, but consume tokens/turns and depend on model judgment; the framework offers `wrap_tool_call` for code-level control but ships no built-in retry middleware for tools (`libs/prebuilt/langgraph/prebuilt/tool_node.py:202-217`).

## Failure Modes / Edge Cases

- **Duplicate external effects on retry** — inherent, see Q5. No mitigation unless developers adopt interrupts/caching/idempotent APIs.
- **Zombie async writes after cancellation/timeout** — actively defended: scope close + write guards + drain callbacks (`libs/langgraph/langgraph/pregel/_retry.py:211-213, 337-340, 492-502`).
- **User-raised `CancelledError` masquerading as shutdown** — converted to `NodeCancelledError` so runs fail loudly instead of reporting success (LSD-1507 fix, `libs/langgraph/langgraph/pregel/_retry.py:315-334, 777-794`; tests at `libs/langgraph/tests/test_retry.py:2826-2941`). On Python <3.11 the detection degrades (documented fallback, `libs/langgraph/tests/test_retry.py:76-86`).
- **Sync-node timeout misuse** — rejected eagerly at compile time and defensively at runtime (`libs/langgraph/langgraph/pregel/_retry.py:580-583`; validation tests `libs/langgraph/tests/test_retry.py:1207-1440`), since cooperative asyncio cancellation cannot preempt GIL-bound sleeps (noted in `TimeoutPolicy` docstring, `libs/langgraph/langgraph/types.py:455-459`).
- **Time-travel forks inheriting stale RESUME/INTERRUPT writes** — scrubbed before fork checkpoints to avoid confusing later resumes (`libs/langgraph/langgraph/pregel/_loop.py:896-900, 960-971`).
- **Jitter-induced duplicate progress events** — acknowledged benign race in the progress rate limiter; observers must tolerate duplicates (`libs/langgraph/langgraph/pregel/_retry.py:195-209`).
- **Cache poisoning via pickle-based keys** — `default_cache_key` pickles inputs (`libs/langgraph/langgraph/_internal/_cache.py:26-31`); mutable or non-deterministic inputs yield unstable keys, and unhashable deep objects fall through `_freeze` unchanged.

## Future Considerations

- An opt-in **side-effect ledger / attempt-token** surfaced through `Runtime.execution_info` would let tools implement their own idempotency using the existing deterministic task ID (`libs/langgraph/langgraph/pregel/_algo.py:694-700`) — the identity infrastructure already exists.
- Exposing retry events as a **public stream/callback surface** (today only `logger.info` and the private timed-attempt observer, `libs/langgraph/langgraph/pregel/_retry.py:677-680, 93-96`) would make retries observable to UIs and eval harnesses without server coupling.
- A **built-in retry `wrap_tool_call` middleware** with per-tool policy would close the gap between node-level `RetryPolicy` and tool-level needs (`libs/prebuilt/langgraph/prebuilt/tool_node.py:215-217` already anticipates multiple `execute` invocations).

## Questions / Gaps

- **No idempotency-key concept anywhere** in tool execution paths — searched repo-wide for `idempot`/`dedupe`/`duplicate`; only hits are checkpoint-write conformance tests, SDK stream-handle bookkeeping (`libs/sdk-py/tests/streaming/`), and a Send-resume dedup test. Confirms external-effect protection is entirely delegated to users.
- **No user-facing docs for retries in this snapshot** — `docs/` contains only `redirects.json` mentioning retry; guidance lives in docstrings (`libs/langgraph/langgraph/types.py:419-437`). Behavior must be inferred from `libs/langgraph/tests/test_retry.py`.
- **Retry budget interaction with `recursion_limit`/step bounds is undefined** — retries occur inside a single superstep (`run_with_retry` loops within one tick), so they don't consume steps, but no evidence documents whether long backoff sequences can stall the step indefinitely under `durability="async"` (no max-total-retry-time found beyond per-attempt `TimeoutPolicy`).
- **Whether LangGraph Server exposes `node_attempt` in public APIs** could not be verified from this source alone; `ExecutionInfo.node_attempt` exists (`libs/langgraph/langgraph/runtime.py:49`) and the observer contract is explicitly reserved for the server (`libs/langgraph/langgraph/pregel/_retry.py:93-96`), but the server itself is outside this repository boundary.

---

Generated by `07.03-idempotency-and-retry-semantics` against `langgraph`.
