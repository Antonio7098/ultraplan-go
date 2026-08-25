# Source Analysis: langgraph

## 01.09 - Delivery Guarantees and Idempotency

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: langgraph, checkpoint, checkpoint-postgres, checkpoint-sqlite, prebuilt, sdk-py, sdk-js, cli) |
| Analyzed | 2026-07-03 |

## Summary

LangGraph delivers execution as effectively-once at the *state-write* layer via storage-level conflict resolution, with retries that are safe at the layer of channel writes (because `task.writes` are cleared before each attempt and special-channel writes are upserted). It is best-effort with regard to *external* side effects performed inside node bodies, which the framework does not envelope in a transaction or retry-aware idempotency contract. The user can opt into three durability regimes (`sync`, `async`, `exit`) that change when writes hit the checkpointer; only `sync` provides a write-before-step guarantee, and `exit` defers persistence entirely. Pre-existing successful writes are restored on resume from the persisted checkpoint via `_reapply_writes_to_succeeded_nodes`, while failures / interrupts are intentionally dropped so they re-fire (or are routed to an error handler). Caching with a per-`CachePolicy` key additionally delivers short-circuit/replay for re-entries of the same task.

## Rating

7/10 (Clear model with tests, explicit interfaces, and operational safeguards) — strong at state-write idempotency and clear retry semantics, weaker for external side effects (framework offers no idempotency-key infrastructure for user code) and for non-`sync` durability modes where workers can lose pending writes on crash.

Rationale:

- **Wins**: explicit `RetryPolicy` and `CachePolicy` types (`langgraph/types.py:416-435`, `:518-527`), storage-level `ON CONFLICT` dedup (`base.py:146-159`, `sqlite/__init__.py:463-465`), a conformance test that pins idempotent `put_writes` (`test_put_writes.py:148-167`), dedicated visibility invariants that drain delta-write futures before checkpoint commit (`_loop.py:1507-1524`, `:1759-1778`), and three documented durability modes (`types.py:87-93`).
- **Losses**: no general-purpose idempotency key store for user node side effects, no transactional bundling of node execution and checkpoint write (a worker that crashes between node return and `commit()` causes a re-run on next resume), `durability="exit"` and default `durability="async"` permit pending writes to be lost on crash, and `_runner.py:574-613` writes error info through the same path that delivers normal channel writes (a failure can race with a follow-up retry).

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Durability typing / semantics | `Durability = Literal["sync", "async", "exit"]` docstring | `libs/langgraph/langgraph/types.py:87-93` |
| Default durability is `async`; explicit error when no checkpointer | `durability = config.get(...).get(CONFIG_KEY_DURABILITY, "async")` / "has no effect" guard | `libs/langgraph/langgraph/pregel/main.py:2617-2618`, `:2817-2819` |
| `durability="exit"` skips per-step `put_writes`; otherwise writes are submitted per step | `if self.durability != "exit" and self.checkpointer_put_writes is not None: … self.submit(self.checkpointer_put_writes, …)` | `libs/langgraph/langgraph/pregel/_loop.py:459-498` |
| Pending writes accumulated only on exit; flushed via `_put_exit_delta_writes` | `if self._exit_delta_writes is not None: ... self._exit_delta_writes.append(...)` ; stage stub + drain futures | `libs/langgraph/langgraph/pregel/_loop.py:696-701`, `:1201-1292` |
| Retry policy type, defaults, fields | `RetryPolicy` with `initial_interval`, `backoff_factor`, `max_interval`, `max_attempts`, `jitter`, `retry_on` | `libs/langgraph/langgraph/types.py:416-435` |
| `default_retry_on` predicate (ConnectionError / 5xx retryable; program errors not) | Treats `ConnectionError` and HTTP 5xx as retryable; explicitly non-retryable for many builtins | `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Sync retry loop clears writes before each attempt | `task.writes.clear()` inside `run_with_retry` per iteration | `libs/langgraph/langgraph/pregel/_retry.py:600-682` |
| Async retry loop clears writes and consults cached writes before first attempt | `task.writes.clear()` + `match_cached_writes()` short-circuit | `libs/langgraph/langgraph/pregel/_retry.py:714-838` |
| Backoff math with jitter | `interval = min(max_interval, initial_interval * backoff_factor**(attempts-1))`; `interval + random.uniform(0,1)` if `jitter` | `libs/langgraph/langgraph/pregel/_retry.py:663-674`, `:818-830` |
| Cancellation translated to `NodeCancelledError` (LSD-1507) | `if _is_user_raised_cancelled(): raise NodeCancelledError(task.name) from exc` | `libs/langgraph/langgraph/pregel/_retry.py:777-794` |
| Cache policy + CacheKey | `CachePolicy.key_func`, `CacheKey(ns, key, ttl)` | `libs/langgraph/langgraph/types.py:518-527`, `:615-623` |
| Default cache key picks `(args, kwargs)` | `default_cache_key` returns `pickle.dumps((_freeze(args), _freeze(kwargs)))` | `libs/langgraph/langgraph/_internal/_cache.py:7-31` |
| Cache lookup wired into async path; uses `cache_key` to skip re-execution | `arun_with_retry` checks `match_cached_writes()` then returns if `t is task` | `libs/langgraph/langgraph/pregel/_retry.py:714-718` |
| Sync cache lookup in `PregelLoop.match_cached_writes` | `cached = {(t.cache_key.ns, t.cache_key.key): t for t in self.tasks.values() if t.cache_key and not t.writes}`; populates `task.writes` from `cache.get` | `libs/langgraph/langgraph/pregel/_loop.py:1526-1539` |
| PregelLoop de-dups special-channel writes "last write wins" | `if all(w[0] in WRITES_IDX_MAP for w in writes): writes = list({w[0]: w for w in writes}.values())` | `libs/langgraph/langgraph/pregel/_loop.py:412-414` |
| Null-task writes deduplicated by `(task_id, channel)` | `self.checkpoint_pending_writes = [w for w in self.checkpoint_pending_writes if w[0] != task_id or w[1] not in WRITES_IDX_MAP]` | `libs/langgraph/langgraph/pregel/_loop.py:415-430` |
| `WRITES_IDX_MAP`: special channels get negative indices so they don't collide with regular `idx` | `{ERROR: -1, SCHEDULED: -2, INTERRUPT: -3, RESUME: -4}` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:795` |
| `BaseCheckpointSaver.put_writes` interface | `def put_writes(self, config, writes, task_id, task_path="")` (both sync and async variants) | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:300-318`, `:491-509` |
| Postgres special-channel UPSERT (UPSERT on `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)`) | `ON CONFLICT (...) DO UPDATE SET channel = EXCLUDED.channel, ...` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:146-153` |
| Postgres regular-channel INSERT-NOTHING | `ON CONFLICT (thread_id, checkpoint_ns, checkpoint_id, task_id, idx) DO NOTHING` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:155-159` |
| Postgres saver chooses query based on writes' channel set | `query = self.UPSERT_CHECKPOINT_WRITES_SQL if all(w[0] in WRITES_IDX_MAP for w in writes) else self.INSERT_CHECKPOINT_WRITES_SQL` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:347-379` |
| Postgres table schema with composite PK on `(thread, ns, checkpoint_id, task_id, idx)` | DDL for `checkpoint_writes` | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/base.py:66-76` |
| SQLite `INSERT OR REPLACE` for special channels, `INSERT OR IGNORE` for regular | `INSERT OR REPLACE INTO writes ... if all(w[0] in WRITES_IDX_MAP for w in writes) else INSERT OR IGNORE INTO writes ...` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:445-482` |
| SQLite composite PK mirrors Postgres | `PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` | `libs/checkpoint-sqlite/langgraph/checkpoint/sqlite/__init__.py:152-162` |
| In-memory saver: first-write-wins for positive `idx`; special channels always overwrite | `if inner_key[1] >= 0 and outer_writes_ and inner_key in outer_writes_: continue`; else assign | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:499-509` |
| Conformance test: duplicate `(task_id, idx)` does NOT duplicate writes | `test_put_writes_idempotent` calls `aput_writes` twice for same key, asserts single write | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py:148-167` |
| Conformance test enumeration includes the idempotency case | `ALL_PUT_WRITES_TESTS = [..., test_put_writes_idempotent, ...]` | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put_writes.py:266-277` |
| Checkpoint visibility invariant: drain delta write futures before persisting checkpoint (sync) | `if self._delta_write_futs: futs, self._delta_write_futs = self._delta_write_futs, [] ; concurrent.futures.wait(futs)` then `put(...)` | `libs/langgraph/langgraph/pregel/_loop.py:1507-1524` |
| Same invariant for async loop | `await asyncio.gather(*futs)` before `await checkpointer.aput(...)` | `libs/langgraph/langgraph/pregel/_loop.py:1759-1778` |
| Error-handler marker write future drained before scheduling handler | `if self._error_handler_write_futs: ... concurrent.futures.wait(futs)` before handler is scheduled | `libs/langgraph/langgraph/pregel/_loop.py:1556-1584` |
| Resume: re-applies previously persisted writes for *successful* tasks; skips control signals | `_reapply_writes_to_succeeded_nodes` filters out `ERROR`, `ERROR_SOURCE_NODE`, `INTERRUPT`, `RESUME` | `libs/langgraph/langgraph/pregel/_loop.py:724-737` |
| Resume: error handler re-fires if `ERROR_SOURCE_NODE` marker present | `_resume_error_handlers_if_applicable` schedules a fresh handler task | `libs/langgraph/langgraph/pregel/_loop.py:739-804` |
| Runner commits writes via `put_writes` for the originating task | `self.put_writes()(task.id, task.writes)` in `commit()` | `libs/langgraph/langgraph/pregel/_runner.py:603`, `:613` |
| Runner treats `CancelledError` and ordinary exceptions as failures worth persisting an `ERROR` write to | `task.writes.append((ERROR, exception))` then `put_writes` | `libs/langgraph/langgraph/pregel/_runner.py:579-603` |
| `AbortThread` style idempotency in SDK adapter | `stream.abort()` test asserts `sync_client.runs.cancel.call_count == 1` after two aborts | `libs/langgraph/tests/test_remote_graph_v3.py:233-239` |
| `run.abort()` documented idempotent | `run.abort()  # idempotent` | `libs/langgraph/tests/test_pregel_stream_events_v3.py:491`, `:600` |
| Sync scoped handle `_finish` idempotency under `_finish_lock` | `with self._finish_lock: if self.status != "started": return ; ...` then `_close_inboxes()` | `libs/sdk-py/langgraph_sdk/_sync/stream.py:629-742` |
| StreamController dedup of replayed events across reconnects | `_seen_event_ids = _SeenEventIds(maxsize=10_000)`; `_dedup_iter` filters by `event_id` | `libs/sdk-py/langgraph_sdk/stream/controller.py:33-51`, `:390-398` |
| Sync streamController parallel impl | `self._seen_event_ids: set[str] = set()` plus bounded set | `libs/sdk-py/langgraph_sdk/stream/sync_controller.py:78`, `:256-258` |
| Conformance suite calling sites include async and sync store dedup as well | `test_async_batch_store_deduplication` (a Store-level dedup; not checkpoint) | `libs/checkpoint/tests/test_store.py:517` |

## Answers to Dimension Questions

**1. Can the same work run twice?**

Yes, for *node execution*. Whenever a task is re-prepared (e.g., on resume or after an error), the worker calls `task.proc.invoke(...)` / `.ainvoke(...)` (`langgraph/pregel/_retry.py:617`, `:744`). Three mechanisms bound duplication:

- `task.writes.clear()` at the start of each retry attempt (`langgraph/pregel/_retry.py:615`, `:738`) prevents in-flight state writes from a failed attempt leaking into a successful one.
- `match_cached_writes` short-circuits when the same task identity re-enters with the same `cache_key` and an empty `writes` (`langgraph/pregel/_retry.py:714-718`, `_loop.py:1526-1539`).
- `BaseCheckpointSaver.put_writes` is **idempotent on `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)`**: Postgres and SQLite use `ON CONFLICT ... DO NOTHING` for regular channels and `ON CONFLICT ... DO UPDATE` for the four special channels (`ERROR`, `SCHEDULED`, `INTERRUPT`, `RESUME`) (`base.py:146-153`, `sqlite/__init__.py:463-465`); InMemory has the same first-write-wins / overwrite rule (`memory/__init__.py:499-509`). So duplicate submissions of writes for the same task are deduplicated at the storage boundary.

The worker itself has no "single-flight" lock: the orchestrator (PregelLoop) restarts only when the in-memory task identity matches a persisted checkpoint, and `task.id` is included in the persisted row, so re-submissions of the same `(task_id, idx)` are safe even on retry storms.

**2. Is duplicate execution safe?**

*At the state-write layer, yes:* duplicate `put_writes` for the same `(thread, ns, checkpoint_id, task_id, idx)` is a no-op for regular channels and an overwrite for special channels. This is explicitly exercised by `test_put_writes_idempotent` (`test_put_writes.py:148-167`).

*For external side effects performed inside node bodies, no.* LangGraph does not provide an idempotency-key contract that wraps node execution. If a node calls an external API and then crashes before returning, the API call has already happened; the framework has no mechanism to undo it. The de-facto safety boundary is the *channel write* (state). Side-effect calls inside nodes are the user's responsibility.

*On resume:* `_reapply_writes_to_succeeded_nodes` (`_loop.py:724-737`) re-attaches persisted writes to tasks with the same `task_id`, so previously successful tasks are not re-executed on resume. Tasks that previously failed (have `ERROR`/`ERROR_SOURCE_NODE` markers in `checkpoint_pending_writes`) are explicitly *not* re-applied: they re-execute or route to an error handler (`_loop.py:739-804`).

**3. Which side effects are idempotent?**

Idempotent *by framework construction*:

- Channel writes via `put_writes` (composite-key dedup).
- Updates to the four control channels (ERROR, SCHEDULED, INTERRUPT, RESUME): upsert semantics, so re-emitting a control signal replaces prior state cleanly.
- `task.writes` clearing at retry boundaries.
- Cache writes keyed on `(task.cache_key.ns, task.cache_key.key)`: `cache.set` overwrites (`_loop.py:1594-1602`).
- Stream event dedup on the SDK side via bounded LRU of `event_id`s (controller.py:33-51, :390-398) on reconnect.
- SDK adapter aborts: `SyncScopedStreamHandle._finish` is guarded by `_finish_lock` with `if self.status != "started": return` (`_sync/stream.py:629-742`); the runner exposes `run.abort()` as idempotent (`test_pregel_stream_events_v3.py:491`).

Not idempotent / out of scope:

- External API calls, DB writes through user code, scheduled jobs, payment charges, etc. The framework offers no envelope / saga / idempotency-token store for these.

**4. Does the system claim exactly-once semantics?**

No. The system is explicitly **at-least-once** at the execution layer with **first-write-wins** storage semantics — which together approximate *effectively-once* at the state-write layer.

The closest statements are: the conformance suite asserts that duplicating writes yields a single persisted row (`test_put_writes_idempotent`); the retry docstring calls each attempt's writes "cleared" so partial state cannot persist (`_retry.py:614-615`); and the storage SQL uses `ON CONFLICT DO NOTHING` (or `DO UPDATE` for special channels) to give one-row-per-`(thread, ns, checkpoint_id, task_id, idx)` invariant. None of these constitute a claim of exactly-once for general user code. The default `durability="async"` (`pregel/main.py:2618`) tolerates pending writes being in-flight when a worker crashes.

**5. Are guarantees documented?**

Yes for the framework's own semantics:

- `RetryPolicy` is a public NamedTuple with documented defaults (`types.py:416-435`).
- `CachePolicy` documents TTL semantics (`types.py:518-527`).
- `Durability` enum is documented inline (`types.py:87-93`).
- `BaseCheckpointSaver.put_writes` documents its parameters and contract (`base/__init__.py:300-318`).
- `WRITES_IDX_MAP` is documented to prevent regular writes from clashing with special ones (`base/__init__.py:788-795`).
- Error-handler code path is explained in the long-form comment on `_resume_error_handlers_if_applicable` (`_loop.py:739-759`).
- The idempotency contract is exercised by the conformance suite (`test_put_writes_idempotent`).

No for end-to-end at-least-once / exactly-once delivery: there is no single README section that ties retry + durability + cache together into a top-level "delivery" guarantee. The `_delta_write_futs` / `_error_handler_write_futs` invariants are documented as comments in the source but not in user-facing README.

## Architectural Decisions

- **Composite primary key for writes:** `(thread_id, checkpoint_ns, checkpoint_id, task_id, idx)` (`base.py:66-76`, `sqlite/__init__.py:152-162`) makes the storage engine, not the application, the enforcement point for dedup. This is the load-bearing design decision for "duplicate `put_writes` is harmless".
- **Negative indices for control channels:** `WRITES_IDX_MAP` reserves `{ERROR:-1, SCHEDULED:-2, INTERRUPT:-3, RESUME:-4}` (`base/__init__.py:795`), so a control-channel row never collides with a regular `idx` row in the same `task_id`.
- **Asymmetric INSERT semantics per channel class:** special-channel writes upsert (`EXCLUDED.channel`, `EXCLUDED.type`, `EXCLUDED.blob`), regular writes insert-nothing (`base.py:146-159`). The choice is driven by `all(w[0] in WRITES_IDX_MAP for w in writes)` (`__init__.py:363-367`).
- **Per-attempt write clearing vs. persistent checkpointer state:** the retry loop explicitly clears `task.writes` before each invocation (`_retry.py:615`, `:738`). This means *partial state cannot persist past a failed attempt*; combined with the storage dedup, the system has stable semantics even if an individual attempt partially records.
- **Cache lookup happens before the *first* attempt, not on every attempt:** `arun_with_retry` does `if match_cached_writes is not None and task.cache_key is not None: ...` once before entering the `while True` loop (`_retry.py:714-718`). So a retry that *missed* the cache cannot come back later and hit it without restart.
- **Visibility invariant for delta channels:** checkpoint persistence drains `_delta_write_futs` (and, for error handlers, `_error_handler_write_futs`) before calling `put` on the checkpoint (`_loop.py:1507-1524`, `:1759-1778`). Without this, ancestor walks could see a checkpoint whose writes have not yet been written. The code comments explicitly call this the "visibility invariant".
- **Durability as the only knob:** `Durability = Literal["sync","async","exit"]` is exposed at the API surface (`main.py:2567-2746`, `types.py:87-93`). Default is `async`. Subgraphs inherit from parent (`main.py:2878-2879`).
- **Resume-time reapply is filtered:** `_reapply_writes_to_succeeded_nodes` deliberately drops `ERROR`, `ERROR_SOURCE_NODE`, `INTERRUPT`, `RESUME` (`_loop.py:733-734`) so that previously-failed / interrupted nodes re-execute instead of silently replaying stale state.
- **Abort idempotency at the SDK layer:** `_finish` guards on `_finish_lock` with an early-return on `status != "started"` (`_sync/stream.py:736-742`); the test `test_sync_scoped_handle_finish_is_idempotent` pins this contract.

## Notable Patterns

- **Storage-enforced idempotency through composite PK + `ON CONFLICT` clause** (Postgres, SQLite) and a dict-key check in `InMemorySaver`. Every back-end exposes identical behaviour.
- **Channel class asymmetry** — control vs data writes get different insert policies. Drives the `if all(...) in WRITES_IDX_MAP` test at every call site.
- **Two-tier writing**: state-channels (`checkpoint["channel_values"]`) are durable only via the full checkpoint; intermediate per-task writes go to `checkpoint_writes` and are re-applied on next resume via `_reapply_writes_to_succeeded_nodes`.
- **Future tracking for ordering**: `_delta_write_futs` and `_error_handler_write_futs` are appended at write-submission time and drained at checkpoint-commit time. This is the implementation of the visibility invariant.
- **Bounded LRU dedup for streaming events** (`_SeenEventIds` in `controller.py:33-51`) keeps memory bounded while providing dedup across stream reconnects.
- **Per-attempt write clearing** is the node-level analogue of the storage-level dedup. Together they make retries idempotent at the state-write layer without requiring the node to know about retries.

## Tradeoffs

- **Per-attempt write clearing + retry is "safe" for state but not for external side effects.** Clearing is a sane default (no half-applied state) but pushes the idempotency burden onto user node code; the framework explicitly does not manage it.
- **`durability="async"` (default) is faster but loses pending writes on crash.** Work executed up to the moment of crash is recorded in `_delta_write_futs` (in-memory); a SIGKILL of the worker between node return and checkpoint commit drops the pending writes (because the checkpointer will only see the next checkpoint's persistence). Recovery will re-execute the same task and the storage dedup will keep state correct, but external side effects repeat.
- **`durability="exit"` defers *all* write persistence until graph completion.** Workers running in this mode lose the *entire* state-write history on a crash before exit. Useful only when paired with long-lived threads or for ephemeral workloads.
- **`durability="sync"` blocks each new checkpoint on the previous plus delta writes.** Best guarantee, worst throughput.
- **Cache key uses pickling of (args, kwargs)** (`_internal/_cache.py:7-31`). This works deterministically but means a node whose return is a function of *time* or *external state* (e.g., `now()` inside the function) will miss the cache on every retry attempt.
- **`max_attempts=3`** is the default for `RetryPolicy` (`types.py:428`) — a sensible conservative default, but it is silent about how it composes with `durability` and the cache. A user relying on retries for stability must understand both.
- **`_runner.py:574-613` puts errors through the same `put_writes` path as successes.** When a node fails, the `ERROR`/`ERROR_SOURCE_NODE` row is persisted via the storage upsert. This is good (error survives a crash mid-recovery) but means an upstream retry path can re-cancel an already-handled error task and overwrite the row.

## Failure Modes / Edge Cases

- **Worker crash between node return and `commit()` (sync mode):** in-flight `task.writes` are lost. The next resume reads from the last persisted checkpoint and re-runs the task. If the node did external side effects, those side effects happen twice. No automatic compensation (`_runner.py:574-613`, `_retry.py:614-617`).
- **Worker crash with `durability="async"`:** `_delta_write_futs` is in-process. Pending writes not yet drained at `asyncio.gather` / `concurrent.futures.wait` time are lost. The next checkpoint will write a fresh row with new `(task_id, idx)` (because the *checkpoint_id* changes after each `apply_writes`), so the prior `task_id` is no longer in scope — the lost writes are silently dropped, not redelivered (`_loop.py:1507-1524`, `:1759-1778`, `:1182-1189`).
- **`default_retry_on` classification errors:** a node raising a `ValueError` will *not* be retried (`_internal/_retry.py:13-26`). If the user expects retries on that exception, they must install a custom `retry_on`. The framework cannot tell a "programmer error" `ValueError` from a "transient error" `ValueError`.
- **Cancellation confusion (LSD-1507):** documented as the reason for `NodeCancelledError` — distinguishing framework cancellation from a user `raise asyncio.CancelledError()` is done via `asyncio.Task.cancelling()` which is Python 3.11+ (`_retry.py:51-56`, `:777-794`). On Python ≤ 3.10 this distinction is unavailable, and the framework cannot reliably convert a cancelled node to a normal failure.
- **Resume with stale `RESUME` writes after time travel:** `_first` drops cached `RESUME` writes when `is_time_traveling` is true, so re-fired interrupts can fire (`_loop.py:862-887`). This is correct in intent but means a "do not fire interrupts again" guarantee isn't fully idempotent on time-travel reads.
- **Concurrent same-`task_id` submissions:** tests show that the storage layer collapses duplicates of the same `(task_id, idx)` at the same `checkpoint_id` (`test_put_writes.py:148-167`, `test_put_writes_multiple_tasks.py:69-92`). But writes at different `checkpoint_id`s are independent rows — there is no global cross-checkpoint `(task_id)` uniqueness guarantee. A user treating `task_id` as a deduplication key across runs will be surprised.
- **Cache TTL on retries:** a retry that exceeds the cache `ttl` will re-run even if a prior attempt completed. With `default_cache_key` based on `(args, kwargs)` plus TTL seconds (`types.py:518-527`), a long retry can silently invalidate the cached result.
- **Error-handler writes vs. retries:** `_reapply_writes_to_succeeded_nodes` skips `ERROR_SOURCE_NODE` rows, so on resume the *failed task* re-runs (not the handler). The handler is re-scheduled only if `error_handler_node` is registered (`_loop.py:775-804`). The retry policy still applies to the rerun of the failed task if the user chose to retry the underlying exception (`_retry.py:649-655`).

## Future Considerations

- **General idempotency-key contract for nodes.** Today, idempotency lives only at the `put_writes` layer. If users had access to a `runtime.idempotency_key(node=..., input=...)` that participated in cache + storage dedup at the external-call level, side-effecting nodes would gain the same effectively-once guarantees the framework already provides for state writes.
- **Crash-recovery semantics for `durability="async"`.** Currently, on crash mid-step, pending writes are in-process only. A bounded WAL or write-ahead of `(task_id, idx, channel, value)` triples to the checkpointer before node invocation would close the gap and turn `async` into effectively-`sync` for the writes that matter.
- **At-least-once on the worker-side too, not just the storage side.** A "claim" model (e.g., `task_id` upserted as `pending` before `proc.invoke`, marked `committed` after `commit()` returns) would let multiple workers observe the same pending task and only one execute it (a Postgres `SELECT ... FOR UPDATE SKIP LOCKED`-style pattern). Today there is no such claim and `_runner.py` assumes a single coordinated driver.
- **Explicit at-least-once / exactly-once docs.** The pieces exist in code (RetryPolicy, CachePolicy, WRITES_IDX_MAP, conformance tests), but no top-level section in the public docs names them as "delivery guarantees". A short user-facing page would close the gap.
- **Cross-checkpoint `task_id` uniqueness.** If a tool like `run.abort()` re-runs with a reused `task_id` across different `checkpoint_id`s, dedup is local to the checkpoint. Bumping the row to include `run_id` (the framework already tracks one — `_runner.py:139-145` references "run_id" in `ExecutionInfo`) would make dedup global across runs.
- **Python ≤ 3.10 cancellation distinction.** `_is_user_raised_cancelled()` requires `asyncio.Task.cancelling()` (`_retry.py:51-56`). On older Python, `CancelledError` from a node body is treated as a real cancellation and may result in the runner dropping the exception (`NodeCancelledError` fallback unavailable). A graceful degradation path is not currently documented.

## Questions / Gaps

- **External side effects in nodes.** No evidence found for an idempotency-key registry, a saga coordinator, or a transactional outbox in this source tree. That capability appears to live in the LangGraph Platform (closed source) rather than the OSS checkpoint library, but no evidence of it appears here.
- **Worker-to-worker hand-off across processes.** No evidence found for any inter-process or distributed-coordination primitive (no `FOR UPDATE SKIP LOCKED`, no advisory locks, no leader election). The runner (`_runner.py`) assumes the loop runs in a single in-process event loop. Distributed execution semantics are not implemented in OSS.
- **At-most-once for `durability="async"` write futures.** The `_delta_write_futs` list is in-memory (`_loop.py:205, 488-498`). If a worker crashes mid-`gather`, those writes are not re-attempted. No replay mechanism is documented in the OSS code.
- **Time-travel re-run and cache interaction.** When a user time-travels to an older checkpoint (`is_replaying = True`) and then re-runs, the cache may serve a stale `(input_hash) -> writes` mapping. The conformance tests do not exercise this combination; no explicit policy is asserted.
- **Resume under repeated network failures.** `_runner.py:222-248` re-raises after the worker resolves; whether the same pending writes can be cancelled partway and resumed later is not exercised by OSS tests. The conformance suite only covers `put_writes` semantics, not crash injection.
- **Composite-task `(task_id, idx)` collision across runs.** No assertion found that `task_id` is generated uniquely across runs of a thread; if two runs accidentally reused the same `task_id` at different `checkpoint_id`s, dedup holds per-checkpoint but the rows are not coalesced globally.
- **Order guarantees.** Writes are returned by `SELECT ... ORDER BY task_id, idx` (`sqlite/__init__.py:263`, Postgres `array_agg(... order by cw.task_id, cw.idx)` at `base.py:112`). Idempotent upserts on the same `(task_id, idx)` are well-defined; partial-ordering across tasks relies on this sort. No explicit test was found that pins cross-task ordering under `durability="async"`.

---

Generated by `01.09-delivery-guarantees-and-idempotency` against `langgraph`.
