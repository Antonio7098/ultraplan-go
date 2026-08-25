# Source Analysis: langgraph

## 23.02 Persistence vs Escalation Philosophy

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core framework, checkpointers, prebuilt agents); JS/TS SDK for the REST API |
| Analyzed | 2026-08-24 |

## Summary

LangGraph's answer to "persist vs escalate" is unusual and deliberate: it is a **low-level orchestration engine that does not decide persistence policy for you** — it makes *persistence itself* the primitive, then exposes every other behavior (retry, stop, pause, ask-a-human, drain) as configurable mechanisms layered on top of a durable state machine.

Concretely:

1. **Persistence is the substrate, not a strategy.** Every superstep of the Pregel loop writes a checkpoint through a pluggable `BaseCheckpointSaver` (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:176`), with three durability modes — `"sync"`, `"async"`, `"exit"` — selected per invocation or via the `CONFIG_KEY_DURABILITY` configurable key (`libs/langgraph/langgraph/types.py:89-95`, `libs/langgraph/langgraph/_internal/_constants.py:68-69`). A run that crashes can be replayed from any checkpoint because channel values, channel versions, and per-task pending writes are all persisted (`libs/langgraph/langgraph/pregel/_loop.py:1081-1210`, `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:347-379`).

2. **Failure escalation is a tiered ladder with explicit semantics**: node-level retry with exponential backoff and selective retry-on predicates (`libs/langgraph/langgraph/pregel/_retry.py:573-683`), graph-level error handlers that convert failures into recovery tasks (`libs/langgraph/langgraph/graph/state.py:275-332`, `libs/langgraph/langgraph/pregel/_runner.py:222-239`), hard stop via `recursion_limit` → `GraphRecursionError` (`libs/langgraph/langgraph/pregel/main.py:3002-3011`), cooperative shutdown via `RunControl.request_drain()` → `GraphDrained` (`libs/langgraph/langgraph/runtime.py:79-104`, `libs/langgraph/langgraph/errors.py:54-64`), time-based aborts via `TimeoutPolicy` (`libs/langgraph/langgraph/types.py:451-514`), and human-in-the-loop pauses via `interrupt()` / static `interrupt_before`/`interrupt_after` (`libs/langgraph/langgraph/types.py:851-974`, `libs/langgraph/langgraph/pregel/_loop.py:666-671,719-724`).

3. **Everything is observable.** Retry attempts are logged (`libs/langgraph/langgraph/pregel/_retry.py:677-680`), attempt numbers flow through `ExecutionInfo.node_attempt` (`libs/langgraph/langgraph/runtime.py:49-53`), timed attempts emit start/progress/finish lifecycle events to an observer contract consumed by the server (`libs/langgraph/langgraph/pregel/_retry.py:87-127`), and the full decision history is queryable through `get_state_history()` (`libs/langgraph/langgraph/pregel/main.py:1480-1531`) plus `StateSnapshot.interrupts`/`PregelTask.error` (`libs/langgraph/langgraph/types.py:683-701`).

The philosophy is therefore "durable-by-default, escalate-by-configuration": the agent keeps trying exactly as long as its configured policies say to, stops on explicit budget exhaustion rather than silent drift, and escalates to humans by pausing durably instead of guessing.

## Rating

**8/10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Persistence is a first-class, pluggable abstraction with multiple production backends (memory, SQLite, Postgres) and conformance tests (`libs/checkpoint-conformance/`).
- Every escalation path has a dedicated exception type, config key, and test coverage (`tests/test_retry.py`, `tests/test_interruption.py`).
- Observability is unusually strong: checkpoints carry metadata (`source`, `step`, `parents` — `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86`), retries are logged with exception details, and attempt counts are exposed in runtime info.
- Not a 9–10 because: there is no built-in *replanning* or adaptive-persistence layer (retry policies are static per-node configs, not feedback-driven), retry/backoff defaults are hardcoded constants rather than tunable profiles, and cross-attempt "how many times have we retried this task historically" accounting lives outside the core loop.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Durability modes (sync/async/exit) | `Durability = Literal["sync", "async", "exit"]` with docstring explaining each mode; resolved from `CONFIG_KEY_DURABILITY`, defaulting to `"async"` | `libs/langgraph/langgraph/types.py:89-95`; `libs/langgraph/langgraph/pregel/main.py:2602-2603` |
| Checkpoint writer per superstep | `_put_checkpoint` persists channel values/versions each loop iteration gated by durability mode | `libs/langgraph/langgraph/pregel/_loop.py:1132-1148` |
| Per-task pending writes persisted | `put_writes` saves intermediate task writes to the checkpointer when durability != "exit" | `libs/langgraph/langgraph/pregel/_loop.py:456-505` |
| Pluggable saver interface | `BaseCheckpointSaver` with `get_tuple`, `list`, `put`, `put_writes`, `delete_thread`, `copy_thread` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176,239,253,277,300,320` |
| Production backend example | `PostgresSaver.put_writes` upserts serialized writes keyed by thread/checkpoint/task id | `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:347-379` |
| Retry policy schema | `RetryPolicy(initial_interval=0.5, backoff_factor=2.0, max_interval=128.0, max_attempts=3, jitter=True, retry_on=default_retry_on)` | `libs/langgraph/langgraph/types.py:418-437` |
| Default retry predicate | Retries connection errors and 5xx HTTP errors; does not retry ValueError/TypeError/etc. | `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Retry execution loop (sync) | `run_with_retry`: clears prior attempt writes, matches policy per exception, exponential backoff + jitter, logs each retry at INFO, sets `CONFIG_KEY_RESUMING=True` for subgraphs after failure | `libs/langgraph/langgraph/pregel/_retry.py:573-683` |
| Retry execution loop (async) | `arun_with_retry` mirrors sync logic; integrates timeouts and cached-write short-circuit | `libs/langgraph/langgraph/pregel/_retry.py:685-838` |
| Policy selection per exception | `_should_retry_on` accepts an exception class, sequence of classes, or callable | `libs/langgraph/langgraph/pregel/_retry.py:841-854` |
| Multiple policies supported | First matching policy in a `Sequence[RetryPolicy]` wins | `libs/langgraph/langgraph/pregel/_retry.py:647-655` |
| Node-level retry configuration | `add_node(..., retry_policy=...)` and graph-wide defaults via `StateGraph(retry_policy=..., error_handler=...)` | `libs/langgraph/langgraph/graph/state.py:275-332,383,400` |
| Timeout policy | `TimeoutPolicy(run_timeout, idle_timeout, refresh_on="auto"\|"heartbeat")`; watchdog converts breach into `NodeTimeoutError` after clearing task writes and cancelling work | `libs/langgraph/langgraph/types.py:451-514`; `libs/langgraph/langgraph/pregel/_retry.py:417-517` |
| Idle-progress detection | Writes, stream chunks, child-task scheduling, LangChain callbacks, and `runtime.heartbeat()` reset the idle clock | `libs/langgraph/langgraph/pregel/_retry.py:128-209,274-312`; `libs/langgraph/langgraph/runtime.py:209-217` |
| Timeout is retryable by default | `NodeTimeoutError` deliberately does not inherit `TimeoutError` so the default retry policy treats it as retryable | `libs/langgraph/langgraph/errors.py:190-199` |
| Graph-level error handler (replan hook) | Failed tasks are routed to a registered handler node which returns a `Command(update=...)`; handled exceptions are excluded from re-raise via `_handled_exception_ids` | `libs/langgraph/langgraph/pregel/_runner.py:171-174,222-240,598`; `libs/langgraph/langgraph/errors.py:148-165` |
| Error-handler task preparation | `prepare_node_error_handler_task` builds a push task wired to the failed task's input/state | `libs/langgraph/langgraph/pregel/_algo.py:1110-1199` |
| Hard stop on step budget | Loop sets status `out_of_steps` when `step > stop`; `stop = step + recursion_limit + 1`; raises `GraphRecursionError` with remediation message | `libs/langgraph/langgraph/pregel/_loop.py:599-609,1701`; `libs/langgraph/langgraph/pregel/main.py:3002-3011` |
| Recursion limit configurability | `DEFAULT_RECURSION_LIMIT = int(getenv("LANGGRAPH_DEFAULT_RECURSION_LIMIT", "10007"))`; overridable per invocation via `{"recursion_limit": N}` | `libs/langgraph/langgraph/_internal/_config.py:32`; `libs/langgraph/langgraph/errors.py:67-85` |
| Cooperative drain (pause/stop) | `RunControl.request_drain(reason)` flips a flag checked between supersteps → `GraphDrained` with checkpoint saved, resumable later | `libs/langgraph/langgraph/runtime.py:79-104`; `libs/langgraph/langgraph/pregel/_loop.py:657-659`; `libs/langgraph/langgraph/errors.py:54-64`; `libs/langgraph/langgraph/pregel/main.py:3012-3015` |
| Human-in-the-loop dynamic interrupt | `interrupt(value)` raises `GraphInterrupt`, surfaces value to client, resumes via `Command(resume=...)`; requires a checkpointer | `libs/langgraph/langgraph/types.py:851-974` |
| Static interrupts | Compile-time `interrupt_before`/`interrupt_after` raise `GraphInterrupt` between steps; statuses `interrupt_before`/`interrupt_after` | `libs/langgraph/langgraph/pregel/_loop.py:666-671,719-724` |
| Interrupt payload type | `Interrupt` dataclass with stable `id` derived from checkpoint namespace hash; resumable directly by id | `libs/langgraph/langgraph/types.py:573-628` |
| Resume plumbing | Reserved write keys `INTERRUPT`/`RESUME`, `CONFIG_KEY_RESUME_MAP`, scratchpad tracks interrupt index and resume values | `libs/langgraph/langgraph/_internal/_constants.py:9-12,72-73`; `libs/langgraph/libs/langgraph` scratchpad usage at `libs/langgraph/langgraph/types.py:950-974` |
| HITL response vocabulary | `HumanResponse` typed dict: accept / ignore / response / edit; deprecated `HumanInterruptConfig` moved to `langchain.agents.interrupt` | `libs/prebuilt/langgraph/prebuilt/interrupt.py:11-105` |
| Prebuilt agent interrupts | `create_react_agent(..., interrupt_before=[...], interrupt_after=[...])` documented as user confirmation before tool actions | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:302-303,447-451,824-825,998-999` |
| Parent-graph delegation | `ParentCommand` bubbles a `Command` from subgraph to parent; namespace rewriting in retry wrapper assigns it correctly | `libs/langgraph/langgraph/errors.py:129-133`; `libs/langgraph/langgraph/pregel/_retry.py:618-631` |
| Retry logging | Each retry logged via `logger.info` with task name, sleep, attempt count, exception class/message, and traceback | `libs/langgraph/langgraph/pregel/_retry.py:677-680,832-836` |
| Attempt observability contract | `_AttemptContext`/`_AttemptEvent` emit start/progress/finish with attempt number, timestamps, error type to a server-consumed observer | `libs/langgraph/langgraph/pregel/_retry.py:87-127,343-404` |
| ExecutionInfo attempt tracking | `node_attempt` (1-indexed) and `node_first_attempt_time` patched into runtime per attempt | `libs/langgraph/langgraph/runtime.py:49-53`; `libs/langgraph/langgraph/pregel/_retry.py:600-612,719-731` |
| State history API | `get_state_history()` yields `StateSnapshot`s from checkpointer `list()`, filterable, limited, subgraph-aware | `libs/langgraph/langgraph/pregel/main.py:1480-1531` |
| Snapshot observability fields | `StateSnapshot.tasks: tuple[PregelTask, ...]` carries per-task `error` and `interrupts` | `libs/langgraph/langgraph/types.py:683-701,637-647` |
| Checkpoint provenance metadata | `source: "input"\|"loop"\|"update"\|"fork"`, `step`, `parents`, `run_id` recorded per checkpoint | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-62` |
| Failure resume semantics | On resume, successful tasks' writes are restored but ERROR/INTERRUPT/RESUME keys skipped so failed/interrupted tasks re-execute or route to handlers | `libs/langgraph/langgraph/pregel/_loop.py:661-664,736-748` |
| User-raised cancellation surfaced | User nodes raising `CancelledError` converted to `NodeCancelledError` so runs report error instead of silently succeeding | `libs/langgraph/langgraph/errors.py:168-186`; `libs/langgraph/langgraph/pregel/_retry.py:315-334,777-794` |
| Tests: retry behavior | `test_graph_with_max_attempts_exceeded` verifies give-up after `max_attempts=2` with backoff sleep asserted | `libs/langgraph/tests/test_retry.py:447-481` |
| Tests: multiple policies & jitter | `test_graph_with_jitter_retry_policy`, `test_graph_with_multiple_retry_policies` | `libs/langgraph/tests/test_retry.py:332,379` |
| Tests: interruption/resume across durability modes | Parametrized over checkpointer × durability; asserts `get_state(thread).next` progression and checkpoint counts | `libs/langgraph/tests/test_interruption.py:11-52` |
| Tests: recursion limit | `app.invoke(2, {"recursion_limit": 1}, ...)` exercises out-of-steps path | `libs/langgraph/tests/test_pregel.py:589` |

## Answers to Dimension Questions

### 1. Does the agent persist or escalate on failure?

Both, in a fixed priority order determined by layered policies:

- **First: retry in place.** If a `RetryPolicy` matches the exception (default: transient network errors only — `libs/langgraph/langgraph/_internal/_retry.py:1-29`), the task reruns with exponential backoff and jitter until `max_attempts` (`libs/langgraph/langgraph/pregel/_retry.py:641-682`). Prior attempt writes are cleared first so partial output never leaks into state (`libs/langgraph/langgraph/pregel/_retry.py:614-615`).
- **Then: delegate to a recovery node.** If the node has a registered `error_handler`, the runner schedules a handler task with a `NodeError` context; the handler returns e.g. `Command(update={"status": "recovered..."})`, converting failure into a normal state transition (`libs/langgraph/langgraph/pregel/_runner.py:222-239`; docstring example at `libs/langgraph/langgraph/errors.py:156-158`).
- **In parallel throughout: durable pause.** Because every superstep checkpoints (`libs/langgraph/langgraph/pregel/_loop.py:718`), any failure leaves the graph resumable — including mid-node interrupts, which persist the interrupted position as a pending write (`__interrupt__` key, `libs/langgraph/langgraph/_internal/_constants.py:9`).
- **Finally: stop loudly.** Budget exhaustion raises `GraphRecursionError` with a troubleshooting pointer (`libs/langgraph/langgraph/pregel/main.py:3002-3011`); timeouts raise `NodeTimeoutError`; external shutdown raises `GraphDrained` after saving a resumable checkpoint (`libs/langgraph/langgraph/pregel/main.py:3012-3015`). Silent failure is explicitly engineered against — even a user node raising `asyncio.CancelledError` is converted into `NodeCancelledError` so the run reports `error` rather than fake success (`libs/langgraph/langgraph/pregel/_retry.py:777-794`).

### 2. Is persistence configurable?

Extensively, at four granularities:

- **Durability mode** per invocation: `durability="sync"|"async"|"exit"` argument on `invoke/stream/batch` or via `CONFIG_KEY_DURABILITY` in config, defaulting to `"async"` (`libs/langgraph/langgraph/pregel/main.py:2552,2602-2603`; `libs/langgraph/langgraph/types.py:89-95`). Warning issued if set without a checkpointer (`libs/langgraph/langgraph/pregel/main.py:2802-2804`).
- **Storage backend** at compile time: `compile(checkpointer=...)` accepts memory, SQLite, or Postgres savers; subgraphs inherit, force-enable (`True`), or opt out (`False`) (`libs/langgraph/langgraph/types.py:100-106`).
- **Per-node retry/timeouts**: `add_node(fn, retry_policy=RetryPolicy(max_attempts=N, backoff_factor=F, retry_on=E), timeout=TimeoutPolicy(run_timeout=..., idle_timeout=...))`, plus graph-wide defaults (`libs/langgraph/langgraph/graph/state.py:286-320`; `libs/langgraph/langgraph/types.py:418-514`).
- **Run budgets**: `recursion_limit` per invocation with an env-var default override, `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` for delta-snapshot frequency (`libs/langgraph/langgraph/_internal/_config.py:32-35`).

### 3. Are escalation paths clear?

Yes — each path is a distinct, typed mechanism rather than an ad-hoc convention:

| Path | Trigger | Mechanism |
|------|---------|-----------|
| Retry same node | Exception matching policy | `arun_with_retry` backoff loop (`_retry.py:795-838`) |
| Recovery node | Unhandled exception + registered handler | Runner schedules error-handler task (`_runner.py:224-232`) |
| Ask a human | Node calls `interrupt()` or static interrupt point reached | `GraphInterrupt` bubble-up → persisted pause → `Command(resume=...)` (`types.py:967-974`) |
| Delegate upward | Subgraph raises `ParentCommand(Command(graph=PARENT,...))` | Namespace rewrite then re-raise to parent (`_retry.py:618-631,753-772`) |
| Stop: budget | `step > recursion_limit` | `GraphRecursionError` (`main.py:3002-3011`) |
| Stop: time | Run/idle timeout breached | `NodeTimeoutError` after write-clear + cancel (`_retry.py:486-502`) |
| Stop: operator | `RunControl.request_drain()` (e.g., SIGTERM) | `GraphDrained`, checkpoint saved, resumable (`runtime.py:95-104`; `main.py:3012-3015`) |

The one soft spot: the prebuilt HITL vocabulary (`allow_accept/edit/respond/ignore`) was just migrated out of this repo to `langchain.agents.interrupt` — `libs/prebuilt/langgraph/prebuilt/interrupt.py:7-10` marks it deprecated — so agent-level escalation ergonomics currently live in a different package while the mechanism remains here.

### 4. Are persistence decisions observable?

Strongly:

- **Logs**: every retry emits `logger.info("Retrying task {name} after {sleep}s (attempt {n}) after {ExcType} {exc}", exc_info=exc)` (`libs/langgraph/langgraph/pregel/_retry.py:677-680,832-836`).
- **Structured runtime metadata**: `ExecutionInfo.node_attempt` / `node_first_attempt_time` let downstream code know it is in attempt N (`libs/langgraph/langgraph/runtime.py:49-53`).
- **Server-facing attempt events**: `_AttemptEvent(start|progress|finish)` with status/error_type/timestamps dispatched to a `CONFIG_KEY_TIMED_ATTEMPT_OBSERVER` callback — explicitly documented as consumed by langgraph-server (`libs/langgraph/langgraph/pregel/_retry.py:92-95,343-404`).
- **Replayable history**: `get_state_history()` reconstructs every prior snapshot with source/step/parent metadata (`libs/langgraph/langgraph/pregel/main.py:1480-1531`; metadata schema at `libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86`); stream mode `"debug"` additionally emits checkpoint and task events live (`libs/langgraph/langgraph/types.py:122-136`).
- **Task-level outcomes**: `StateSnapshot.tasks[i].error/.interrupts/.result` expose what happened to each unit of work (`libs/langgraph/langgraph/types.py:637-647,683-701`).

## Architectural Decisions

1. **Checkpoint-per-superstep as the universal fallback.** Rather than choosing between persist/retry/escalate per failure class, the loop persists unconditionally between steps (`libs/langgraph/langgraph/pregel/_loop.py:718`), making every other policy recoverable. The durability knob only controls *when* writes flush, never *whether* the model exists.
2. **Policy objects over magic numbers.** `RetryPolicy`/`TimeoutPolicy`/`CachePolicy` are declarative dataclasses attached at `add_node` time (`libs/langgraph/langgraph/types.py:418-529`), keeping the execution engine generic.
3. **Exception-type-driven control flow.** `GraphBubbleUp` subclasses distinguish "stop and surface to human" (`GraphInterrupt`) from "stop and fail" (`GraphRecursionError`, `NodeTimeoutError`) from "delegate" (`ParentCommand`) from "cooperative stop" (`GraphDrained`) — the retry wrapper routes each differently (`libs/langgraph/langgraph/pregel/_retry.py:618-640,773-794`).
4. **Write-clearing before every attempt.** Both retry loops call `task.writes.clear()` first (`_retry.py:614-615,738`), guaranteeing at-most-once commit semantics per successful attempt.
5. **Handled-vs-unhandled exception tracking in the runner.** `_handled_exception_ids` prevents an error routed to a handler from also panicking the run (`libs/langgraph/langgraph/pregel/_runner.py:166-169,235-240`) — a deliberate double-dispatch guard.

## Notable Patterns

- **Idle-timeout as liveness detection**: rather than only wall-clock limits, `TimeoutPolicy.idle_timeout` measures *progress* — writes, streams, callbacks, heartbeats all refresh it — catching hung-but-alive LLM calls (`libs/langgraph/langgraph/pregel/_retry.py:128-209`), with a strict `refresh_on="heartbeat"` mode for nodes whose progress isn't otherwise visible (`libs/langgraph/langgraph/types.py:476-481`).
- **Resume-aware retries**: after a failed attempt, `CONFIG_KEY_RESUMING=True` is injected so subgraphs resume from their own checkpoints instead of restarting (`_retry.py:681-682,837-838`).
- **Cache short-circuit**: completed task writes are cached (`CachePolicy`), and `match_cached_writes` skips re-executing identical work on retry/resume (`_retry.py:714-718`; `main.py:2965-2966`).
- **Drain-before-fail shutdown semantics**: SIGTERM-style drains stop *between* supersteps and save state (`_loop.py:657-659`), treating operator intent as a pause, not an abort.

## Tradeoffs

- **No adaptive persistence**: `RetryPolicy` is static; there is no built-in circuit-breaker, success-rate feedback, or cross-run attempt memory. Repeatedly failing graphs retry identically forever unless the developer wires replanning themselves via error handlers or conditional edges.
- **Default retry predicate is aggressive about unknown exceptions**: anything not in the known-benign list returns `True` (retry) (`libs/langgraph/langgraph/_internal/_retry.py:28-29`), which favors liveness over side-effect safety — reasonable for LLM APIs, riskier for non-idempotent tool nodes.
- **Async-only timeouts**: `TimeoutPolicy` relies on asyncio cancellation; sync-blocking nodes defeat it (documented caveat at `libs/langgraph/langgraph/types.py:455-459`), and sync nodes with timeouts are rejected outright (`_retry.py:580-583`).
- **Complexity cost**: the durability/interrupt/resume machinery spans many cooperating files (`_loop.py` ~2000 lines, `_retry.py` ~850 lines); correctness depends on subtle invariants (e.g., exit-mode checkpoint deduplication at `_loop.py:1092-1095`).
- **HITL ergonomics split across packages**: mechanism in `langgraph.types.interrupt`, agent-level policy recently moved to `langchain.agents.interrupt` (`libs/prebuilt/langgraph/prebuilt/interrupt.py:8`), creating version-skew risk for users.

## Failure Modes / Edge Cases

- **Watchdog races**: `asyncio.FIRST_COMPLETED` may complete both task and watchdog simultaneously; handled by suppressing already-fired timeouts (`_retry.py:477-480`).
- **Framework vs user cancellation ambiguity**: distinguishing sibling-task cancellation from node self-cancellation relies on Python ≥3.11's `Task.cancelling()`; older interpreters always propagate as framework cancellation (`_retry.py:52-56,315-334`).
- **Exit-durability data window**: `durability="exit"` persists only at run end — a hard crash loses the entire run's checkpoints (by design; the tradeoff is explicit in `types.py:89-95` and tested in `tests/test_interruption.py:44-52` where exit mode yields fewer checkpoints).
- **Handler loops**: an error handler that itself fails follows normal retry/error paths; routing excludes handler nodes from being routed to themselves (`_runner.py:171-174`), but a cyclic handler→failing-node graph could ping-pong within recursion_limit — bounded only by the step budget.
- **Partial-write hygiene on timeout**: timeout cancels background work and clears writes before raising (`_retry.py:492-494`), but best-effort stream output may have been emitted externally before close (`_retry.py:137-141` documents dropped-after-close semantics).

## Future Considerations

- Add policy profiles or adaptive retry (e.g., token-budget-aware backoff, per-tool idempotency declarations) to reduce reliance on `retry_on` callables.
- Surface aggregate persistence telemetry (attempt counts, interrupt counts per thread) in the OSS library rather than only via the server observer contract.
- Provide a first-class replanning primitive (handler composition, budgeted re-entry) so the common "LLM failed → reformulate" pattern doesn't require hand-rolled conditional edges.

## Questions / Gaps

- **No evidence found** for automatic replanning or self-correction loops inside the core: searches across `libs/langgraph/langgraph/` for replan/replan-like triggers returned only the error-handler and Command/goto primitives; replanning is left entirely to graph authors.
- The JS ecosystem in-repo (`libs/sdk-js`) is a REST client; whether LangGraph.js implements equivalent retry/interrupt semantics could not be verified from this source (no JS runtime present — only `sdk-js` client bindings).
- Long-term historical retry accounting (e.g., "this task failed 40 times across resumes") is not tracked in checkpoint metadata (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:38-86` records no attempt counters); it appears delegated to the server layer.

---

Generated by `23.02-persistence-vs-escalation-philosophy` against `langgraph`.
