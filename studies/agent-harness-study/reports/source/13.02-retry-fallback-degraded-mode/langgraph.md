# Source Analysis: langgraph

## Dimension 13.02: Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core framework + checkpointers + sdk-py); `libs/sdk-js` contains only a README in this checkout, so JS behavior is not verifiable here |
| Analyzed | 2026-08-25 |

## Summary

LangGraph implements retry as a first-class engine at the task-execution layer of its Pregel runtime, not at the model/client layer. A single public config type (`RetryPolicy`, `libs/langgraph/langgraph/types.py:418-437`) drives exponential backoff with optional jitter and exception-classified retry decisions. Policies are configurable at three scopes — per node (`add_node(retry_policy=...)`), graph-wide defaults (`StateGraph.set_node_defaults(...)`), and the functional API (`@task`/`entrypoint`) — with per-node values overriding graph-wide defaults inside the runner.

Retry state itself (attempt counters) is **in-process only**; what *is* persisted is the **outcome**: on final failure the runner commits an `(ERROR, exception)` pending write to the checkpointer (`Runner.commit`, `libs/langgraph/langgraph/pregel/_runner.py:574-603`). On resume, failed tasks are deliberately re-executed from scratch with a fresh attempt budget, while succeeded tasks' writes are restored (`_reapply_writes_to_succeeded_nodes`, `libs/langgraph/langgraph/pregel/_loop.py:736-749`; proven by `test_pregel.py:978` where a node runs exactly 2 fresh attempts after restart).

There is **no fallback model/provider chain** anywhere in this repository (grep for `with_fallbacks|fallbacks` yields only incidental hits in serde and property access), and there is **no circuit breaker implementation**. Degraded mode is expressed as user-authored error-handler nodes with explicit ordering guarantees (handlers run only after retries are exhausted), and the routing decision survives process restarts via persisted `ERROR_SOURCE_NODE` markers (`_loop.py:751-816`). The Python SDK adds an independent transport-level resilience layer: SSE reconnect with capped attempts and its own exponential backoff plus proportional jitter.

Against the dimension's guiding question — "Can the system survive a provider outage without failing all requests?" — LangGraph survives *transient* outages via retries and enables user-authored graceful degradation for sustained ones, but it does not provide automatic provider failover or cascade-failure protection out of the box.

## Rating

**7/10**

Rationale: The retry model itself is mature for its scope — a clear declarative interface (`RetryPolicy` NamedTuple), three configuration scopes with documented precedence, exception-classification with sensible transient-vs-programming-error defaults, jitter and interval caps as operational safeguards, and an exceptionally thorough dedicated test suite (`libs/langgraph/tests/test_retry.py`, ~2,900 lines) asserting exact sleep timings, attempt counts, and resume semantics. Persistence of failure outcomes with deterministic re-execution on resume is well-designed and tested across checkpointer implementations. However: (1) no circuit breaker or rate-limit auto-retry exists, so a sustained provider outage costs every caller full `max_attempts × backoff` latency with no cascade protection; (2) no built-in fallback/failover mechanism — degraded mode requires users to author handler nodes themselves; (3) retry attempt counters are not durable, giving crash-restarted workloads a fresh attempt budget (documented but potentially load-amplifying); (4) backoff math is duplicated between sync/async paths and independently reinvented in the SDK. These gaps place it solidly in the "clear model with tests, explicit interfaces, and operational safeguards" band (7–8) rather than "proven under failure or scale" (9–10).

## Evidence Collected

Every entry includes a file path with line numbers relative to the source root `studies/agent-harness-study/sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public retry config type | `RetryPolicy(NamedTuple)` with `initial_interval=0.5`, `backoff_factor=2.0`, `max_interval=128.0`, `max_attempts=3`, `jitter=True`, `retry_on=default_retry_on` | `libs/langgraph/langgraph/types.py:418-437` |
| Core sync retry engine | `run_with_retry()`: node policy wins (`task.retry_policy or retry_policy`), clears prior-attempt writes, gives up at `max_attempts`, computes backoff | `libs/langgraph/langgraph/pregel/_retry.py:573-617` |
| Backoff formula | `interval = min(max_interval, initial_interval * backoff_factor ** (attempts - 1))` | `libs/langgraph/langgraph/pregel/_retry.py:663-668` |
| Jitter | additive `random.uniform(0, 1)` when `jitter=True`, then `time.sleep(sleep_time)` | `libs/langgraph/langgraph/pregel/_retry.py:671-674` |
| Give-up condition | `if attempts >= matching_policy.max_attempts: raise` (original error re-raised) | `libs/langgraph/langgraph/pregel/_retry.py:657-661` |
| Subgraph resume-on-retry | patches `CONFIG_KEY_RESUMING: True` before sleeping so nested interrupted subgraphs resume instead of restarting on next attempt | `libs/langgraph/langgraph/pregel/_retry.py:682` (sync), `837-838` (async) |
| Async twin | `arun_with_retry()` mirrors backoff/jitter with `asyncio.sleep`; integrates `TimeoutPolicy` and cache-hit short-circuit | `libs/langgraph/langgraph/pregel/_retry.py:685-838` |
| Exception matching | `_should_retry_on()` accepts one class, sequence of classes, or callable predicate; raises `TypeError` otherwise | `libs/langgraph/langgraph/pregel/_retry.py:841-854` |
| Default classification | `default_retry_on`: always retries `ConnectionError`; retries httpx/requests errors only for 5xx; never retries ValueError/TypeError/ArithmeticError/OSError etc.; retries unknown exceptions by default | `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Timeout designed to be retryable | `NodeTimeoutError` deliberately does NOT subclass built-in `TimeoutError` (an `OSError`) so default policy treats it as retryable | `libs/langgraph/langgraph/errors.py:190-199` |
| Cancellation semantics | user-raised `CancelledError` converted to `NodeCancelledError` so it flows through the failure path instead of silent teardown | `libs/langgraph/langgraph/errors.py:168-187`, `libs/langgraph/langgraph/pregel/_retry.py:635-640` |
| Graph-wide defaults API | `set_node_defaults(retry_policy=..., timeout=..., error_handler=...)`; docstring example names the handler `my_fallback_handler`; notes defaults apply to error-handler nodes too and are not inherited by subgraphs | `libs/langgraph/langgraph/graph/state.py:272-330` |
| Per-node override API | `add_node(..., retry_policy=...)` threaded through builder; docs state per-node takes precedence | `libs/langgraph/langgraph/graph/state.py:106,272-330,383,453` |
| Low-level node API | `PregelNode.add_retry_policies(*policies)`; single `RetryPolicy` coerced to 1-tuple | `libs/langgraph/langgraph/pregel/main.py:349-352`, `libs/langgraph/langgraph/pregel/_read.py:122,164-180` |
| Precedence resolution point | task preparation resolves `proc.retry_policy or retry_policy` (per-node wins) | `libs/langgraph/langgraph/pregel/_algo.py:752,928,1098,1166` |
| Runner wiring | Runner passes `retry_policy` into `run_with_retry`/`arun_with_retry` for every task future (sync + async) | `libs/langgraph/langgraph/pregel/_runner.py:207-214,396-405` |
| Functional API | `@task(retry_policy=...)` and `entrypoint(retry_policy=...)` parameters | `libs/langgraph/langgraph/pregel/_call.py:261-293` |
| Failure outcome persistence | `Runner.commit()` appends `(ERROR, exception)` to task.writes and calls `put_writes()` to the checkpointer; adds `ERROR_SOURCE_NODE` marker when routed to a handler | `libs/langgraph/langgraph/pregel/_runner.py:574-603` |
| Resume skips control signals | `_reapply_writes_to_succeeded_nodes()` restores success writes but explicitly skips `ERROR, ERROR_SOURCE_NODE, INTERRUPT, RESUME` so failed tasks re-execute | `libs/langgraph/langgraph/pregel/_loop.py:736-749` |
| Error-handler routing survives restarts | `_resume_error_handlers_if_applicable()` scans persisted pending writes for `ERROR_SOURCE_NODE`+`ERROR` pairs, marks original done, schedules handler task | `libs/langgraph/langgraph/pregel/_loop.py:751-816` |
| Handler durability ordering | `schedule_error_handler()` awaits error-write futures "so error + ERROR_SOURCE_NODE writes are durable before handler runs"; handler inherits failed node's retry policy | `libs/langgraph/langgraph/pregel/_loop.py:1572-1599`, `libs/langgraph/langgraph/pregel/_algo.py:1098-1166` |
| Checkpoint data model | `PendingWrite = tuple[str, str, Any]`; `CheckpointTuple.pending_writes` persists ERROR/interrupt writes per thread | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:31,146` |
| Fallback providers absent | grep for `with_fallbacks\|fallbacks` across repo returns only incidental hits (serde pickle fallback, property-access comment); no `.with_fallbacks` chain exists | `libs/langgraph/langgraph/_internal/_future.py:21`, `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:83` (both unrelated) |
| Circuit breakers absent | grep for `circuit\|breaker\|failover` finds no implementation; only a docstring metaphor about warning dedup acting as a circuit breaker | `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:52` (comment only) |
| Rate-limit typing (no auto-retry) | `RateLimitError(APIStatusError)` with `status_code: Literal[429]`; mapped in `_map_status_error()` — typed for consumers, but SDK performs no automatic 429 retry | `libs/sdk-py/langgraph_sdk/errors.py:132-133,206-207` |
| SSE transport reconnect | streaming loop tracks `Last-Event-ID`, follows server `Location` redirect for reconnect, caps at `max_reconnect_attempts = 5`, retries mid-stream `httpx.HTTPError` | `libs/sdk-py/langgraph_sdk/_async/http.py:211-287`; sync mirror `libs/sdk-py/langgraph_sdk/_sync/http.py:227-279` |
| SDK stream backoff | `_reconnect_sleep`: `delay = min(cap, base * 2**attempt)` plus proportional jitter `uniform(0, delay*0.25)` | `libs/sdk-py/langgraph_sdk/stream/controller.py:276-282`, loop `298-313` |
| SSE protocol retry hint | `SSEDecoder` parses the SSE `retry:` field into `_retry` | `libs/sdk-py/langgraph_sdk/sse.py:83,133-135` |
| Retry observability | `logger.info("Retrying task ... after {sleep_time:.2f} seconds (attempt {attempts}) ...")` per retry; `runtime.execution_info.node_attempt` exposed 1-indexed to node bodies | `libs/langgraph/langgraph/pregel/_retry.py:607-609,677-679` |

## Answers to Dimension Questions

### 1. Are retries configurable?

**Yes, extensively.** Configuration surface spans four layers:

- **Policy fields**: all knobs are public on `RetryPolicy` — interval, factor, cap, attempts, jitter, and the `retry_on` matcher which accepts a single exception class, a sequence of classes, or a callable predicate (`libs/langgraph/langgraph/types.py:418-437`; matcher dispatch at `libs/langgraph/langgraph/pregel/_retry.py:841-854`).
- **Scopes**: per-node via `add_node(retry_policy=...)` (`libs/langgraph/langgraph/graph/state.py:106`), graph-wide via `set_node_defaults(retry_policy=...)` (`libs/langgraph/langgraph/graph/state.py:272-330`), low-level via `PregelNode.add_retry_policies(...)` (`libs/langgraph/langgraph/pregel/main.py:349-352`), and functional API via `@task(retry_policy=...)`/`entrypoint(retry_policy=...)` (`libs/langgraph/langgraph/pregel/_call.py:261-293`).
- **Precedence**: resolved at task preparation as `proc.retry_policy or retry_policy` — per-node wins over graph defaults (`libs/langgraph/langgraph/pregel/_algo.py:1166`; tested in `libs/langgraph/tests/test_retry.py:2500-2570`).
- **Multiple policies per node**: a node can carry a sequence of policies; the first whose `retry_on` matches the exception applies (`_retry.py:648-652`; test `libs/langgraph/tests/test_retry.py:379-444`).

### 2. Are fallback providers available?

**No — not within this repository.** There is no `.with_fallbacks` or fallback-model-chain mechanism anywhere in the source; searches for `with_fallbacks|fallbacks` return only incidental hits unrelated to provider failover (`libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:83` is a serializer fallback). Model-level fallbacks are a langchain-core concept that lives outside this source boundary. The closest analog is the **error-handler node** mechanism: `StateGraph.set_node_defaults(error_handler=my_fallback_handler)` (`libs/langgraph/langgraph/graph/state.py:298-302,311-324`) receives a structured `NodeError(node, error)` context (`libs/langgraph/langgraph/errors.py:148-165`) and may return e.g. `Command(update={"status": "recovered from ..."})` — i.e., degraded mode is user-authored application logic, not built-in provider switching.

### 3. Does the system degrade gracefully?

**Partially — yes where designed, by omission elsewhere.** Graceful degradation exists along two axes:

- **Ordered hand-off to handlers**: error-handler nodes run only after retries are exhausted (`libs/langgraph/tests/test_retry.py:2009-2055` asserts the handler sees `attempts == 2` with `max_attempts=2`), and handler failures fail the run rather than recursing (`state.py:300-302`; test `test_retry.py:2100`). Handler tasks inherit the failed node's retry policy and their scheduling waits for the error writes to be durable first (`libs/langgraph/langgraph/pregel/_loop.py:1572-1599`).
- **Restart-survivable degradation**: because `ERROR` + `ERROR_SOURCE_NODE` are persisted pending writes, a process restart routes the failed task to its handler instead of re-running the node (`_loop.py:751-816`), while sibling successes are replayed from checkpoint (`_loop.py:736-749`). This ordering is asserted in `libs/langgraph/tests/test_pregel.py:959-978`.

What is *not* graceful: with no circuit breaker or 429 auto-handling, a hard provider outage means every invocation independently burns its full attempt budget with sleeps before failing.

### 4. Are circuit breakers used to prevent cascading failure?

**No.** No circuit-breaker, bulkhead, or failover implementation exists in any library in this monorepo (verified by searching `circuit|breaker|degrad|failover`; sole hit is a docstring metaphor at `libs/checkpoint/langgraph/checkpoint/serde/jsonplus.py:52`). The nearest protective mechanisms are indirect: bounded `max_interval` caps backoff latency per task (`types.py:428-429`), the default `retry_on` refuses to retry programming errors (`_internal/_retry.py:11-28`) limiting pointless hammering, and the runner cancels sibling tasks when one fails (`_should_stop_others`, `libs/langgraph/langgraph/pregel/_runner.py:616-624`) — a super-step abort rather than a breaker.

## Architectural Decisions

1. **Retry belongs to the execution layer, not the client layer.** The Pregel runner wraps every task execution in `run_with_retry`/`arun_with_retry` (`libs/langgraph/langgraph/pregel/_runner.py:207-214,396-405`), so retry semantics are uniform across nodes regardless of what HTTP client or model a node uses. Consequence: LangGraph cannot honor provider-specific signals like `Retry-After` headers — it knows only exceptions, statuses filtered through `default_retry_on`, and time-based backoff.

2. **Persist outcomes, not attempt state.** Only the terminal `(ERROR, exception)` write reaches the checkpointer (`_runner.py:597-603`); attempt counters live in the loop-local variable `attempts` (`_retry.py:584,658`). On resume, a previously-failed task re-executes with a fresh budget — `test_pregel.py:978` proves `two.calls == 4` (two attempts pre-crash + two post-resume) with `RetryPolicy(max_attempts=2)` (`test_pregel.py:891-1019`). This keeps checkpoint semantics simple and idempotent, at the cost of unbounded cumulative attempts across repeated crashes.

3. **Exception taxonomy engineered around the default classifier.** `NodeTimeoutError` deliberately avoids inheriting built-in `TimeoutError` (an `OSError`) specifically so `default_retry_on` treats timeouts as retryable (`errors.py:190-199`), while user-raised `CancelledError` is re-wrapped as `NodeCancelledError` so framework-initiated teardown stays distinct from user failure (`errors.py:168-187`, `_retry.py:635-640`).

4. **Subgraphs resume rather than restart on retry.** Before each backoff sleep, the config is patched with `CONFIG_KEY_RESUMING: True` (`_retry.py:681-682,837-838`) so a nested subgraph interrupted mid-run resumes from its checkpoint on the next attempt rather than restarting from scratch — critical for expensive nodes containing interrupts.

5. **Writes are cleared between attempts.** `task.writes.clear()` before each invocation (`_retry.py:613-615`) prevents partial writes from a failed attempt contaminating the commit of the successful one.

6. **Degraded mode is a graph construct, not a runtime feature.** Rather than built-in failover, LangGraph provides `error_handler` nodes with formal semantics (ordered after retries, non-recursive, persistently routed) — pushing fallback policy into the user's state machine, which is consistent with the framework's philosophy that control flow lives in the graph.

## Notable Patterns

- **Sync/async twin engines**: `run_with_retry` (`_retry.py:573-682`) and `arun_with_retry` (`_retry.py:685-838`) duplicate identical backoff/jitter logic — a maintainability smell, though kept consistent and co-tested.
- **Exact-timing tests**: the suite mocks `time.sleep`/`random.uniform` to assert precise sleep sequences (e.g., `[0.01, 0.02]` for `backoff_factor=2.0` at `libs/langgraph/tests/test_retry.py:249-306`; jitter composition `sleep(0.01 + 0.05)` at `test_retry.py:332-376`).
- **Attempt observability injected into node runtime**: `runtime.execution_info.node_attempt` (1-indexed) and stable identity fields (`thread_id`, `checkpoint_ns`, `node_first_attempt_time`) let nodes adapt behavior per attempt (`_retry.py:600-612`; stability tested at `test_retry.py:482-539`).
- **Independent reinvention of backoff in the SDK**: `stream/controller.py:280-281` uses multiplicative backoff with 25%-of-delay proportional jitter — a different formula and jitter style than core's additive `U(0,1)`, showing the transport layer evolved its own resilience vocabulary.
- **Protocol-level retry hints honored**: the SDK parses SSE `retry:` fields (`libs/sdk-py/langgraph_sdk/sse.py:133-135`) and server-driven reconnect via `Location` headers (`_async/http.py:247-252`) — server-coordinated degradation.
- **Deprecation hygiene**: legacy `retry=` kwarg guarded against in favor of `retry_policy=` (`libs/langgraph/tests/test_deprecation.py:35-55`).

## Tradeoffs

- **Fresh attempt budget per process life** vs. simplicity/idempotency: crash-looping workloads get unlimited cumulative retries across restarts; conversely, no risk of permanently poisoned checkpoints that skip a node forever.
- **Default-retry-on unknown exceptions** (`_internal/_retry.py:29` returns `True`): maximizes survival for unfamiliar transient errors, but will retry non-idempotent operations (e.g., double-charging side effects) unless authors opt out.
- **Bounded-but-uncoordinated backoff**: `max_interval=128s` cap bounds individual task latency, yet with no shared breaker a fleet of concurrent graphs multiplies request volume during an outage exactly when the provider can least absorb it.
- **User-authored fallback flexibility vs. footgun surface**: error handlers are powerful (arbitrary `Command` routing, `NodeError` introspection) but nothing stops a handler from masking systemic failures; mitigation is the non-recursion rule (`state.py:300-302`).
- **In-repo fallback absence** keeps LangGraph provider-agnostic (delegating chains to langchain-core outside this boundary), but means graph-level "try model B if model A dies" requires manual handler wiring.

## Failure Modes / Edge Cases

- **Exhaustion**: original exception re-raised after `max_attempts` (`_retry.py:659-661`), then committed as ERROR pending write (`_runner.py:596-603`); observed persisted shape `[(AnyStr(), ERROR, 'ConnectionError(...)')]` in `tests/test_pregel.py:845-888`.
- **Non-matching exception types**: immediately re-raised without consuming an attempt (`_retry.py:648-655`).
- **Interrupts never retried**: `GraphBubbleUp` propagates untouched (`_retry.py:632-634`) — human-in-the-loop pauses are not treated as failures.
- **Framework cancellation vs. user cancellation**: external `CancelledError` propagates without retry (tested `test_retry.py:2826-2900`); node-body-raised `CancelledError` becomes `NodeCancelledError` and flows through normal retry/error handling (`_retry.py:635-640`).
- **Timeout × retry interaction**: idle/run timeouts produce `NodeTimeoutError` (retryable by default); stale executor writes from timed-out attempts are discarded (`tests/test_retry.py:776-1197`); timeouts compose with `retry_on=[NodeTimeoutError]` explicitly (`test_retry.py:1480-1508`).
- **Handler failure fails the run** rather than falling back recursively (`test_retry.py:2100+`).
- **Mid-stream transport drops**: SDK reconnects up to 5 times resuming from `Last-Event-ID`, then raises `TransportError("Exceeded maximum SSE reconnection attempts")` (`_async/http.py:264-286`).
- **Parent-directed Commands**: `ParentCommand` bypasses retry entirely and bubbles up with rewritten namespace (`_retry.py:618-631`).

## Future Considerations

- **Durable attempt budgets**: persisting attempt counters in pending writes would enable crash-safe budgets (and per-thread cooldowns) while retaining current re-execution semantics as an option — engineering work localized to `Runner.commit` (`_runner.py:574-613`) and `_reapply_writes_to_succeeded_nodes` (`_loop.py:736-749`).
- **Rate-limit-aware retry**: honoring `Retry-After` / parsing 429 responses in `default_retry_on` (`_internal/_retry.py:7-10`) would materially improve provider-outage behavior at near-zero complexity.
- **Circuit breaker at the runner level**: a shared failure-rate gate ahead of task dispatch would address the cascade-failure gap flagged by the dimension question.
- **Configurable jitter strategy**: today jitter is boolean additive `U(0,1)` (`_retry.py:671-673`); full/equal/decorrelated modes are standard extensions.
- **Unify backoff math**: extracting the duplicated sync/async formulas (`_retry.py:663-674` vs `818-830`) and aligning SDK controller jitter (`controller.py:276-282`) would reduce drift risk.
- **sdk-js parity unverifiable here**: `libs/sdk-js` contains only `README.md` in this checkout; JS-side retry behavior could not be assessed.

## Questions / Gaps

- **Fallback model chains**: confirmed absent in-repo, but whether upstream integration expects langchain-core `.with_fallbacks` inside node bodies cannot be verified from this source boundary alone (searches limited to `studies/agent-harness-study/sources/langgraph`).
- **No evidence found** of metrics/tracing emission specific to retry events beyond the `logger.info` line (`_retry.py:676-679`); searched for retry-related otel/metric hooks without hits — retry observability is logs + `execution_info.node_attempt` only.
- **sdk-js**: no TypeScript source present in this checkout, so questions about JS retry/reconnect behavior have no answerable evidence ("No clear evidence found"; search boundary was `libs/sdk-js/**`, which contains only `README.md`).
- **Distributed-mode retry**: `run_with_retry` synthesizes missing `execution_info` for a distributed runtime path (`tests/test_retry.py:593-640`) and `TaskNotFound` exists (`errors.py:142-145`), but the distributed executor itself lives outside this repository; how retries coordinate across workers is not determinable from this source.

---

Generated by `13.02-retry-fallback-and-degraded-mode` against `langgraph`.
