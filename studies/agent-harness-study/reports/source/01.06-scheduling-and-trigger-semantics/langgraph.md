# Source Analysis: langgraph

## 01.06: Scheduling and Trigger Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (monorepo: `libs/langgraph`, `libs/sdk-py`, `libs/cli`, `libs/checkpoint*`, `libs/prebuilt`, `libs/sdk-js`) |
| Analyzed | 2026-07-02 |

## Summary

LangGraph splits "what triggers execution" across three layers, and the trigger story is fundamentally **server-mediated**. The open-source core (`libs/langgraph/langgraph/pregel/main.py:449` — `class Pregel`) is an in-process, callable runnable — it has *no* built-in scheduler, queue, cron, or webhook surface. All external triggers live one layer up, in the proprietary LangGraph Server (a separate, closed-source runtime imported by the SDK as `langgraph_api.server` per `libs/sdk-py/langgraph_sdk/_async/client.py:117`), and the SDK (`libs/sdk-py/langgraph_sdk`) is a thin HTTP client that forwards trigger requests to that server. Triggers visible in this source are therefore *contracts about what the server does*, not implementations of the trigger themselves: cron-style scheduled runs (`libs/sdk-py/langgraph_sdk/_async/cron.py:30` — `class CronClient`), delayed one-shot runs (`after_seconds` in `libs/sdk-py/langgraph_sdk/_async/runs.py:441`), batched background runs (`create_batch` at `libs/sdk-py/langgraph_sdk/_async/runs.py:607`), completion webhooks (`webhook=` parameter on every run/cron entry point, e.g. `libs/sdk-py/langgraph_sdk/_async/cron.py:71`), and disconnect-aware streams (`on_disconnect` per `libs/sdk-py/langgraph_sdk/_async/runs.py:93`). Inside a single run, the only internal triggers are channel-driven: a `PregelNode` re-fires whenever any channel in `triggers` is written (`libs/langgraph/langgraph/pregel/_read.py:97` — `class PregelNode`, triggers at `_read.py:107-109`), and the runner then plans/executes/commits in Pregel supersteps (`libs/langgraph/langgraph/pregel/main.py:455-476` — docstring). Retry is per-task, not per-trigger: `RetryPolicy` (`libs/langgraph/langgraph/types.py:416`) drives `run_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:573`) and `arun_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:685`), with backoff + jitter; timeouts are split into `step_timeout` (run-level) and per-node `TimeoutPolicy` (idle/run watchdogs at `libs/langgraph/langgraph/pregel/_retry.py:417-517`). Multitask semantics on a shared thread are explicit and named (`MultitaskStrategy = Literal["reject", "interrupt", "rollback", "enqueue"]` at `libs/sdk-py/langgraph_sdk/schema.py:81`), and run status moves through a typed state machine (`RunStatus = Literal["pending", "running", "error", "success", "timeout", "interrupted"]` at `libs/sdk-py/langgraph_sdk/schema.py:23`). Restarts rely on the checkpoint layer (`libs/checkpoint/`, `libs/checkpoint-postgres/`, `libs/checkpoint-sqlite/`) which the SDK's `Run` rows and cron's `next_run_date` (`libs/sdk-py/langgraph_sdk/schema.py:408`) presume is durable. Because the actual scheduler implementation is closed-source in `langgraph_api.server`, anything below the HTTP contract is opaque from this source tree.

## Rating

**5/10 — Present but inconsistent, weakly documented, or fragile.**

A clean trigger surface exists at the SDK/HTTP layer (`runs.create`, `runs.create_batch`, `crons.create`, `crons.create_for_thread`, `runs.stream`/`runs.wait`/`runs.cancel`/`runs.join_stream`, the `after_seconds`, `webhook`, `multitask_strategy`, `on_disconnect`, `interrupt_before/after`, `feedback_keys` knobs), and the open-source engine has mature **internal** semantics around retry, timeout, durability, and channel triggers. But:

1. **The scheduler itself is not in this source.** Cron, delayed `after_seconds`, webhook delivery, and the disconnect handler all live in `langgraph_api.server` (imported at `libs/sdk-py/langgraph_sdk/_async/client.py:117`); only the **contracts** are documented here.
2. **Idempotency is delegated to the caller.** No client-side dedup key. `runs.create` accepts arbitrary `run_id` only through `RunCreateMetadata`, not as an explicit idempotency token. Crons have `enabled: bool` (`libs/sdk-py/langgraph_sdk/schema.py:412`) but no documented "missed-run" policy.
3. **Observability is event-stream, not log-stream.** The SDK exposes `RunStatus` and `stream_mode` modes (`values | messages | updates | events | tasks | checkpoints | debug | custom | messages-tuple` at `libs/sdk-py/langgraph_sdk/schema.py:51-61`), but there is no first-class OTel/tracing hook in this source — `langsmith_tracing` is only a server-side tracer key (`libs/sdk-py/langgraph_sdk/_async/runs.py:590`).
4. **Foreground vs background execution is identical inside a run** (same `Pregel` runner), but the SDK separates them via `runs.wait` (blocking) vs `runs.create` (returns `Run`) — and that distinction is purely server-mediated.
5. **Inconsistency between the protocol-level docstring and the SDK code.** `CronClient.create` docstring says "The crons client functionality is not supported on all licenses" (`libs/sdk-py/langgraph_sdk/_async/cron.py:48-52`), so the contract is licensed, not open.

This is well above 1-3 because the in-process execution semantics around retry/timeout/channel-triggers/durability are first-class; it's well below 7 because the trigger story as the dimension frames it (scheduler implementation, idempotency, persistence-on-restart, observability, foreground/background parity) is mostly server-side and closed.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core Pregel is an in-process runnable, no scheduler | `class Pregel` only exposes `invoke`/`ainvoke`/`stream`/`astream`/state ops; no background/cron entry | `libs/langgraph/langgraph/pregel/main.py:449` |
| No framework-level cron, queue, or timer in OSS | `grep -rn "asyncio.create_task\|scheduler\|cron"` in `libs/langgraph` finds only timeout watchdogs and internal task plumbing | `libs/langgraph/langgraph/pregel/_retry.py:463`, `_retry.py:467`, `libs/langgraph/langgraph/_internal/_runnable.py:132` |
| Server is the actual trigger host | SDK imports `from langgraph_api.server import app` for in-process ASGI; no OSS replacement | `libs/sdk-py/langgraph_sdk/_async/client.py:117` |
| Cron client exists at SDK layer | `class CronClient` with `create_for_thread`, `create`, `update`, `delete`, `search`, `count` | `libs/sdk-py/langgraph_sdk/_async/cron.py:30` |
| Cron schedule contract | `schedule: str` (cron format), `timezone: str | None`, `end_time: datetime | None`, `enabled: bool`, `next_run_date: datetime | None` | `libs/sdk-py/langgraph_sdk/schema.py:383-413` |
| Cron `next_run_date` is server-populated | `Cron.next_run_date: datetime \| None` — the SDK only reads it | `libs/sdk-py/langgraph_sdk/schema.py:408` |
| Cron `on_run_completed` lifecycle hook | Stateless crons delete/keep the per-tick thread; documented at the SDK contract level only | `libs/sdk-py/langgraph_sdk/schema.py:392`, `libs/sdk-py/langgraph_sdk/_async/cron.py:217-219` |
| Cron `webhook` (completion callback) | `webhook: str \| None` on every cron entry point, forwarded in payload | `libs/sdk-py/langgraph_sdk/_async/cron.py:71`, `c:156`, `c:187`, `c:274`, `c:332`, `c:394` |
| Cron docstring explicitly disclaims server support | "The crons client functionality is not supported on all licenses." | `libs/sdk-py/langgraph_sdk/_async/cron.py:48-52` |
| Delayed one-shot run trigger | `after_seconds: int \| None = None` on `runs.create`, `runs.stream`, `runs.wait` — server schedules dispatch | `libs/sdk-py/langgraph_sdk/_async/runs.py:441`, `r:97`, `r:129`, `r:158`, `r:187`, `r:219` |
| Batched stateless background trigger | `async def create_batch(self, payloads: list[RunCreate])` POSTs `/runs/batch` | `libs/sdk-py/langgraph_sdk/_async/runs.py:607-622` |
| Multitask strategy enum | `Literal["reject", "interrupt", "rollback", "enqueue"]` — explicit conflict resolution per shared thread | `libs/sdk-py/langgraph_sdk/schema.py:81-88` |
| Run lifecycle status enum | `RunStatus = Literal["pending", "running", "error", "success", "timeout", "interrupted"]` | `libs/sdk-py/langgraph_sdk/schema.py:23` |
| Disconnect behavior enum | `DisconnectMode = Literal["cancel", "continue"]` — server decides what happens when the SSE/stream consumer drops | `libs/sdk-py/langgraph_sdk/schema.py:74-79` |
| Stream format version knob | `StreamVersion = Literal["v1", "v2"]` — same trigger, two wire shapes | `libs/sdk-py/langgraph_sdk/schema.py:606-611` |
| Stream modes (trigger-time observability channel) | `StreamMode = Literal["values", "messages", "updates", "events", "tasks", "checkpoints", "debug", "custom", "messages-tuple"]` | `libs/sdk-py/langgraph_sdk/schema.py:51-72` |
| Run creation accepts resumable stream and context | `stream_resumable`, `langsmith_tracing`, `context` (typed by `context_schema`) | `libs/sdk-py/langgraph_sdk/_async/runs.py:373-374`, `r:446`, `r:482-485` |
| Durability knob is shared across runs, crons, and update | `Durability = Literal["sync", "async", "exit"]` — sync/async/exit persistence modes | `libs/sdk-py/langgraph_sdk/schema.py:104-108` |
| `runs.cancel` supports `interrupt` vs `rollback` action | `action: CancelAction = "interrupt"` | `libs/sdk-py/langgraph_sdk/_async/runs.py:949-950`, `libs/sdk-py/langgraph_sdk/schema.py:137-142` |
| `runs.cancel_many` selects by thread or status | `status: BulkCancelRunsStatus = Literal["pending", "running", "all"]` | `libs/sdk-py/langgraph_sdk/_async/runs.py:1001-1057`, `libs/sdk-py/langgraph_sdk/schema.py:144-150` |
| `runs.join_stream` reconnects to in-flight runs | `cancel_on_disconnect: bool`, `last_event_id` resume | `libs/sdk-py/langgraph_sdk/_async/runs.py:1097-1140` |
| LangSmith tracing key (server-side trigger-side trace hook) | `payload["langsmith_tracer"] = langsmith_tracing` | `libs/sdk-py/langgraph_sdk/_async/runs.py:590` |
| In-process node-level triggers | `PregelNode.triggers: list[str]` — "If any of these channels is written to, this node will be triggered in the next step" | `libs/langgraph/langgraph/pregel/_read.py:107-109` |
| Trigger-to-node index | `trigger_to_nodes: Mapping[str, Sequence[str]]` indexed in `Pregel.__init__` | `libs/langgraph/langgraph/pregel/main.py:754`, `_trigger_to_nodes` at `main.py:4190-4196` |
| Node push trigger (dynamic fan-out) | `PUSH_TRIGGER = (PUSH,)` used by `Send`/PUSH tasks scheduled via `CONFIG_KEY_SEND` | `libs/langgraph/langgraph/pregel/_algo.py:516`, `libs/langgraph/langgraph/_internal/_constants.py` (PUSH constant) |
| Task payload reports the triggers that fired it | `triggers: list[str]` on `TaskPayload` (graph-stream/debug event) | `libs/sdk-py/langgraph_sdk/schema.py:626-627` |
| Debug event records per-task triggers | `"triggers": task.triggers` in debug payloads | `libs/langgraph/langgraph/pregel/debug.py:51` |
| Run-level timeout (Pregel-level) | `Pregel.step_timeout: float \| None = None` per Pregel ctor | `libs/langgraph/langgraph/pregel/main.py:726`, `main.py:770` |
| Per-node timeout policy | `TimeoutPolicy` with `idle_timeout`, `run_timeout`, `refresh_on` | `libs/langgraph/langgraph/types.py:444` (coercion) and `libs/langgraph/langgraph/pregel/_retry.py:70-84` (`_ResolvedTimeout`) |
| Idle-timeout watchdog | `wait_for_idle_timeout(idle_timeout_s)` raises `asyncio.TimeoutError` after quiet window | `libs/langgraph/langgraph/pregel/_retry.py:215-223` |
| Run-timeout watchdog | `_run_timeout_watchdog(run_timeout_s)` raises after absolute duration | `libs/langgraph/langgraph/pregel/_retry.py:417-419` |
| Watchdog wiring in async runner | Two parallel `asyncio.create_task` watchdog futures in `asyncio.wait(..., FIRST_COMPLETED)` | `libs/langgraph/langgraph/pregel/_retry.py:460-470` |
| Retry policy shape | `RetryPolicy` namedtuple: `initial_interval`, `backoff_factor`, `max_interval`, `max_attempts`, `jitter`, `retry_on` | `libs/langgraph/langgraph/types.py:416-435` |
| Default retry predicate | `default_retry_on` retries on `ConnectionError`, `5xx` HTTP, treats `ValueError/TypeError/OSError/...` as terminal | `libs/langgraph/langgraph/_internal/_retry.py:1-29` |
| Sync retry loop | `run_with_retry(task, retry_policy, configurable=...)` with backoff + `time.sleep` between attempts | `libs/langgraph/langgraph/pregel/_retry.py:573-682` |
| Async retry loop | `arun_with_retry(task, retry_policy, stream=False, ..., configurable=...)` with `await asyncio.sleep` between attempts | `libs/langgraph/langgraph/pregel/_retry.py:685-838` |
| Retryable predicate selector | First matching policy wins; non-matching exception re-raises | `libs/langgraph/langgraph/pregel/_retry.py:649-655`, `r:803-810` |
| Cached-write match avoids re-running | `if match_cached_writes is not None and task.cache_key is not None: ...` — idempotent on cache hit | `libs/langgraph/langgraph/pregel/_retry.py:714-718` |
| StateGraph default retry policy | `StateGraph.__init__` accepts `retry_policy` defaulting all nodes; per-node override via `add_node(..., retry_policy=...)` | `libs/langgraph/langgraph/graph/state.py:274-326` (constructor), `state.py:867`, `state.py:877`, `state.py:889`, `state.py:901` |
| Pregel-level retry default | `retry_policy: Sequence[RetryPolicy] = ()` (empty disables retry at run level) | `libs/langgraph/langgraph/pregel/main.py:741-742`, `main.py:775` |
| Background execution model inside a step | `PregelRunner.tick` / `atick` submits tasks to `BackgroundExecutor`/`AsyncBackgroundExecutor` with `__reraise_on_exit__` and `__cancel_on_exit__` flags | `libs/langgraph/langgraph/pregel/_runner.py:176-359`, `libs/langgraph/langgraph/pregel/_executor.py:40-122` (sync), `:122-217` (async) |
| Concurrency guard per run | `AsyncBackgroundExecutor` honors `config["max_concurrency"]` via `asyncio.Semaphore` | `libs/langgraph/langgraph/pregel/_executor.py:131-140`, `:214-217` |
| Cancel-aware context for both runners | `__cancel_on_exit__` and `__reraise_on_exit__` flags attached to every submitted task | `libs/langgraph/langgraph/pregel/_executor.py:51-52`, `:64`, `:96-104`, `:166-167`, `:194-197` |
| Checkpoint durability layering | `libs/checkpoint` defines the abstract interface; `checkpoint-postgres` and `checkpoint-sqlite` are the two OSS implementations | `AGENTS.md:25-36`, `libs/checkpoint/`, `libs/checkpoint-postgres/`, `libs/checkpoint-sqlite/` |
| Checkpointer selection at Pregel level | `checkpointer: Checkpointer = None`; an in-memory default is applied by `ensure_valid_checkpointer` | `libs/langgraph/langgraph/pregel/main.py:732`, `main.py:797` |
| Durability mode wired through configurable | `CONFIG_KEY_DURABILITY` propagates the `sync/async/exit` value into the run | `libs/langgraph/langgraph/_internal/_constants.py:68` |
| `StreamPart.task_payload.triggers` exposed to clients | TypedDict event with `triggers: list[str]` for tasks (channel-driven triggers are observable) | `libs/sdk-py/langgraph_sdk/schema.py:617-627` |
| InterruptBefore/After still apply when scheduled | `interrupt_before`/`interrupt_after` forwarded on `runs.create`, `runs.stream`, `runs.wait`, crons | `libs/sdk-py/langgraph_sdk/_async/runs.py:378-379`, `:407-408`, `:435-436`, `libs/sdk-py/langgraph_sdk/_async/cron.py:69-70`, `:185-186`, `:333-334` |
| Integration tests exercise the cron contract | `test_crons.py` creates/deletes crons via the live server and tears them down before tick | `libs/sdk-py/tests/integration/test_crons.py:37-66` |
| Unit tests cover cron client surface | `test_crons_client.py` exercises create/update/search/count with sync+async | `libs/sdk-py/tests/test_crons_client.py:34-649` |
| `RunStatus` is the public state machine | Docstring lists the six states; used by `runs.list(status=...)` and `runs.cancel_many(status=...)` | `libs/sdk-py/langgraph_sdk/schema.py:23`, `libs/sdk-py/langgraph_sdk/_async/runs.py:860`, `r:1008` |
| SDK HTTP client with retry on connection failure | `httpx.AsyncHTTPTransport(retries=5)` for *transport-level* retries only — not for run-level | `libs/sdk-py/langgraph_sdk/_async/client.py:128-129` |
| Thread/cron status webhooks are forwarded, not delivered | SDK only sets `webhook` in payload; delivery is the server's responsibility | `libs/sdk-py/langgraph_sdk/_async/cron.py:156`, `r:274`, `r:394` |
| In-process async background runs share the same `Runner` (BackgroundExecutor/AsyncBackgroundExecutor) | `__aenter__`/`__aexit__` enforces `__cancel_on_exit__` and `__reraise_on_exit__` semantics on every task | `libs/langgraph/langgraph/pregel/_executor.py:122-217` |

## Answers to Dimension Questions

1. **What starts execution?**
   In the open-source code there is exactly one trigger: a Python call to `Pregel.invoke`/`ainvoke`/`stream`/`astream`/`astream_events`/`update_state` etc. (entry points listed at `libs/langgraph/langgraph/pregel/main.py:3798, 3975, 4028, 3630, 3735, 2530, 1589, 1391, 1479`). Every other trigger the dimension calls out (cron, queue job, timer, webhook, internal retry, etc.) is **server-side** and only its *HTTP contract* lives in this repository:
   - Cron / scheduled recurrence → `CronClient.create_for_thread` / `create` (`libs/sdk-py/langgraph_sdk/_async/cron.py:58, 175`) → `POST /threads/{tid}/runs/crons` and `POST /runs/crons`.
   - Delayed one-shot → `runs.create(..., after_seconds=N)` and `runs.stream(..., after_seconds=N)` (`libs/sdk-py/langgraph_sdk/_async/runs.py:441, 97`).
   - Batch / queue-style fan-in → `runs.create_batch(payloads)` → `POST /runs/batch` (`libs/sdk-py/langgraph_sdk/_async/runs.py:607-622`).
   - Completion webhook → `webhook=` field on every run/cron entry point; the SDK forwards it but never delivers (`libs/sdk-py/langgraph_sdk/_async/runs.py:469`, `libs/sdk-py/langgraph_sdk/_async/cron.py:101`).
   - Re-entrant trigger (resume) → `runs.join_stream(thread_id, run_id, last_event_id=...)` (`libs/sdk-py/langgraph_sdk/_async/runs.py:1097-1140`).
   - Internal (channel-write trigger) → `PregelNode.triggers: list[str]` (`libs/langgraph/langgraph/pregel/_read.py:107-109`), indexed by `_trigger_to_nodes` (`libs/langgraph/langgraph/pregel/main.py:4190-4196`).
   - Push/fan-out trigger → `PUSH_TRIGGER = (PUSH,)` set on tasks produced by `Send` writes (`libs/langgraph/langgraph/pregel/_algo.py:516`).
   - CLI trigger → only `langgraph up` / `langgraph build` / `langgraph dockerfile` / `langgraph dev` exist (`libs/cli/langgraph_cli/cli.py:276, 412, 529, 758`) — they deploy a server, they do not trigger runs.
   - Manual interrupt-after-create trigger → `threads.update_state` (`libs/sdk-py/langgraph_sdk/_async/threads.py:621`) is documented to not run nodes (`libs/sdk-py/langgraph_sdk/runtime.py:62-69`) but does set channel triggers so the *next* `invoke`/`stream` evaluates edges from that node.

2. **Are triggers durable?**
   - **OSS side:** durable insofar as the user attaches a `Checkpointer` (`libs/langgraph/langgraph/pregel/main.py:732`); `PregelLoop.__enter__` rehydrates from the saver, `_put_checkpoint` advances `checkpoint_config` (`libs/langgraph/langgraph/pregel/_loop.py:231`), and `_delta_write_futs` are awaited before the next `_checkpointer_put_after_previous` (`libs/langgraph/langgraph/pregel/_loop.py:1515-1524`). So a run that crashes mid-step resumes from the last persisted superstep.
   - **SDK side:** durability is implicit — the `Run` row has `status`, `created_at`, `updated_at` (`libs/sdk-py/langgraph_sdk/schema.py:362-379`), `Cron` exposes `next_run_date` and `enabled` (`libs/sdk-py/langgraph_sdk/schema.py:408, 412`). The SDK *believes* triggers are durable but does not persist them itself.
   - **Cron / `after_seconds` durability is server-side and out-of-source.** The OSS code has no scheduler, so no claim can be made from this tree about whether cron rows survive a server restart. The checkpoint-postgres / checkpoint-sqlite implementations are the OSS surface where durability is observable.
   - **Resume of in-flight stream** is durable via `stream_resumable=True` plus `last_event_id` (`libs/sdk-py/langgraph_sdk/_async/runs.py:1097-1140`), but only at the transport layer.
   - **Webhook delivery** durability: not implemented in this source; the SDK only forwards the URL.

3. **Are duplicate triggers safe?**
   Mixed:
   - **Idempotent by construction in OSS:** `arun_with_retry` checks `match_cached_writes` and short-circuits when the task already has cached writes (`libs/langgraph/langgraph/pregel/_retry.py:714-718`); the `BackgroundExecutor` uses `concurrent.futures.Future` so a re-submit gets the same handle (`libs/langgraph/langgraph/pregel/_executor.py:40-122`).
   - **Idempotent by contract in SDK:** `multitask_strategy` (`reject` / `interrupt` / `rollback` / `enqueue`, `libs/sdk-py/langgraph_sdk/schema.py:81-88`) is the **only** dedup surface for duplicate triggers on the same thread — the SDK does not synthesize idempotency keys for you.
   - **Cron `enabled: bool`** is a kill-switch, not a dedup mechanism (`libs/sdk-py/langgraph_sdk/schema.py:412`).
   - **Webhook retries:** not specified in this source.
   - **Caveat:** `RunCreateMetadata` (`libs/sdk-py/langgraph_sdk/schema.py:917`) is response metadata only; there is no public `Idempotency-Key` header pattern in the SDK.

4. **Can scheduled work be observed like interactive work?**
   Yes, through the same stream surface — but only if the *server* honors the request:
   - **Stream modes** are identical between `runs.stream` (foreground), `runs.create` (background), and crons: `StreamMode = Literal["values", "messages", "updates", "events", "tasks", "checkpoints", "debug", "custom", "messages-tuple"]` (`libs/sdk-py/langgraph_sdk/schema.py:51-72`). Crons forward the same `stream_mode` field (`libs/sdk-py/langgraph_sdk/_async/cron.py:160`).
   - **Per-task triggers** are surfaced on `TaskPayload.triggers` (`libs/sdk-py/langgraph_sdk/schema.py:626-627`) and in debug events (`libs/langgraph/langgraph/pregel/debug.py:51`).
   - **Status polling:** `runs.get`, `runs.list`, `runs.wait` are status-oriented and apply to background runs as well (`libs/sdk-py/langgraph_sdk/_async/runs.py:625, 854, 906`).
   - **`thread.status` lifecycle** is exposed as `Literal["idle", "busy", "interrupted"]` (`libs/sdk-py/langgraph_sdk/schema.py:300-318`); the same field is updated for foreground and background work.
   - **Webhook:** is the only **out-of-band** completion signal; `on_run_completed: Literal["delete", "keep"]` (`libs/sdk-py/langgraph_sdk/schema.py:97-102`) tells the server what to do with the per-tick thread when the run is over.

5. **Do background and foreground execution share semantics?**
   Mostly yes on the engine side, partly no on the wire side:
   - **Same engine:** both go through `PregelLoop` (`libs/langgraph/langgraph/pregel/_loop.py:156`), `PregelRunner.tick`/`atick` (`libs/langgraph/langgraph/pregel/_runner.py:176, 360`), and the same `run_with_retry`/`arun_with_retry` (`libs/langgraph/langgraph/pregel/_retry.py:573, 685`). Retry, timeout, channel triggers, durability, and checkpointing are uniform.
   - **Different caller contract:** `runs.wait` blocks until completion (`libs/sdk-py/langgraph_sdk/_async/runs.py:625-830`), `runs.create` returns immediately with a `Run` row in `pending` state (`libs/sdk-py/langgraph_sdk/_async/runs.py:447-488`).
   - **`on_disconnect: Literal["cancel", "continue"]`** (`libs/sdk-py/langgraph_sdk/schema.py:74-79`) is a server-mediated knob that makes the wire contract depend on disconnect behavior, not the engine.
   - **Cron vs single-shot:** a cron ticks produce the same `Run` shape (`libs/sdk-py/langgraph_sdk/schema.py:362-379`) but each tick gets its own thread id when `on_run_completed="delete"` and `thread_id=None` was supplied (`libs/sdk-py/langgraph_sdk/_async/cron.py:217-219`).
   - **Restart recovery:** on the engine, yes — the loop re-enters from the persisted checkpoint (`libs/langgraph/langgraph/pregel/_loop.py:1466-1505`). On the scheduler side (cron/after_seconds), no in-source evidence — that is the server's responsibility.

## Architectural Decisions

- **Trigger surface lives at the HTTP layer, not the Python layer.** `libs/sdk-py/langgraph_sdk` defines `CronClient`, `RunsClient.stream/create/wait/list/cancel/join/join_stream`, and `OnCompletionBehavior`/`Durability`/`MultitaskStrategy` enums; the engine is deliberately framework-agnostic about *how* it got called. This is the cleanest design decision in the repo — it lets `Pregel` stay a pure `Runnable` while the trigger contract evolves.
- **In-process channel triggers, not external ones.** `PregelNode.triggers` is a list of channel names; the runner's plan step selects nodes whose `triggers` overlap with channels written in the previous step (`libs/langgraph/langgraph/pregel/_algo.py:266, 477, 1260`). This is the "BSP / Pregel" foundation cited in `README.md:81` and means a single trigger primitive (channel write) covers edges, branches, and `Send`.
- **Push triggers use a reserved `PUSH` channel.** `Topic(Send, accumulate=False)` is reserved as the `TASKS` channel (`libs/langgraph/langgraph/pregel/main.py:803-808`), and the special `PUSH_TRIGGER = (PUSH,)` tuple identifies dynamic fan-out tasks (`libs/langgraph/langgraph/pregel/_algo.py:516`). This separates user-defined dataflow from control flow.
- **Background runs share the foreground runner.** `AsyncBackgroundExecutor.submit` is the same future the runner hands to `PregelRunner.atick`; cancellation is per-future, not per-run (`libs/langgraph/langgraph/pregel/_executor.py:122-217`). This is why a foreground `ainvoke` and a "background" `runs.create` execute the same code.
- **Retry policy is per-task and configurable at three levels.** `RetryPolicy` namedtuple (`libs/langgraph/langgraph/types.py:416`) is settable on `Pregel(...)` (`libs/langgraph/langgraph/pregel/main.py:741`), on `StateGraph(...)` as a default (`libs/langgraph/langgraph/graph/state.py:274`), or per-node via `add_node(..., retry_policy=...)` (`libs/langgraph/langgraph/graph/state.py:867`). Resolution is `task.retry_policy or proc.retry_policy or retry_policy` (`libs/langgraph/langgraph/pregel/_retry.py:579, 694`).
- **Timeout is split into `step_timeout` (run-wide) and per-node `TimeoutPolicy` (idle/run).** Step timeout aborts the superstep; idle timeout cancels sibling futures that have not made progress within the window (`libs/langgraph/langgraph/pregel/_retry.py:128-273`); run timeout is an absolute deadline. Watchdogs are coordinated via `asyncio.wait(..., FIRST_COMPLETED)` and `NodeTimeoutError` (`libs/langgraph/langgraph/pregel/_retry.py:496-505`).
- **Durability is explicit and three-valued.** `Durability = Literal["sync", "async", "exit"]` (`libs/sdk-py/langgraph_sdk/schema.py:104-108`) maps to how `put_writes` and `put` are sequenced in `PregelLoop` (`libs/langgraph/langgraph/pregel/_loop.py:215-237`). `exit` mode accumulates delta writes and persists only on success (`libs/langgraph/langgraph/pregel/_loop.py:213-221`).
- **Checkpoint layer is a separate library (`libs/checkpoint`).** `AGENTS.md:25-36` makes the layering explicit; the `checkpoint-postgres` and `checkpoint-sqlite` are sibling libs of `langgraph`. The dependency map in `AGENTS.md:38-50` shows that the OSS engine never assumes a specific store.

## Notable Patterns

- **Trigger contract is a TypedDict, not a class.** `RunCreate`, `Cron`, `CronUpdate`, `Run` are all `TypedDict`s (`libs/sdk-py/langgraph_sdk/schema.py:362, 383, 416, 522`). This makes them JSON-serializable by construction but means there is no method-level polymorphism — every "method" is a REST endpoint and the SDK is a typed RPC client.
- **Single endpoint shape: payload includes scheduling knobs.** `runs.create`, `runs.stream`, `runs.wait` all accept the same `webhook / after_seconds / multitask_strategy / interrupt_before / interrupt_after / durability / on_disconnect` set; crons are a superset of that set with `schedule / timezone / end_time / enabled` added. The reuse pattern is so consistent that `after_seconds` is forwarded through five overloads of `runs.stream` (`libs/sdk-py/langgraph_sdk/_async/runs.py:97, 129, 158, 187, 219`).
- **Two wire shapes for streams.** `StreamVersion = Literal["v1", "v2"]` (`libs/sdk-py/langgraph_sdk/schema.py:606-611`) lets the same trigger send either raw SSE `StreamPart` NamedTuples or typed `StreamPartV2` dicts. `_wrap_stream_v2` does the conversion lazily (`libs/sdk-py/langgraph_sdk/_async/runs.py:46-53`).
- **Concurrency cap via `config["max_concurrency"]`.** `AsyncBackgroundExecutor` reads it once and acquires a semaphore per submit (`libs/langgraph/langgraph/pregel/_executor.py:135-140, 153-154, 214-217`). This is the only knob the engine offers to cap concurrency inside a run.
- **Two-flag cancellation contract.** Every task carries `(__cancel_on_exit__, __reraise_on_exit__)` so the runner can ask the executor to either kill the task on runner exit or wait-and-raise (`libs/langgraph/langgraph/pregel/_executor.py:51-52, 64, 96-104, 166-167, 194-210`). This is the in-process analog of `cancel_on_disconnect`.
- **Status is a closed enum, not free text.** `RunStatus` (six values) and `ThreadStatus` (three values) are exhaustive, which lets the SDK treat filters as exact match (`libs/sdk-py/langgraph_sdk/schema.py:23, 300-318`).
- **Idempotency on retry via cache.** `arun_with_retry` consults `match_cached_writes` before re-running a task; cache hits return immediately (`libs/langgraph/langgraph/pregel/_retry.py:714-718`). This is the only true in-engine idempotency surface.
- **Triggers are reflected back into the event stream.** `TaskPayload.triggers` is exposed to consumers of `stream_mode="events"`/`"tasks"` so a scheduled run's per-task trigger provenance is observable (`libs/sdk-py/langgraph_sdk/schema.py:617-627`). The OSS engine produces these in `debug.py:51`.

## Tradeoffs

- **The scheduler is closed-source.** The OSS repo has no cron daemon, no delayed-run dispatcher, no webhook delivery worker, no background queue. Every "trigger" beyond `Pregel.invoke` is a server-mediated contract. A user who self-hosts only the OSS engine gets none of `runs.create_batch`, `crons`, `after_seconds`, or webhook delivery. (`libs/sdk-py/langgraph_sdk/_async/client.py:117` imports the proprietary `langgraph_api.server`.)
- **Cron is licensed, not guaranteed.** `libs/sdk-py/langgraph_sdk/_async/cron.py:48-52` — "The crons client functionality is not supported on all licenses." A SDK consumer cannot rely on cron being present.
- **Idempotency is the caller's job.** The SDK exposes `multitask_strategy` to choose between `reject` / `interrupt` / `rollback` / `enqueue` for the **same thread** but does not synthesize idempotency tokens for repeat HTTP calls, repeated cron ticks, or webhook redelivery. Duplicate `/runs/crons` POSTs create duplicate crons (the SDK has no `if_exists` for crons — only `OnConflictBehavior = Literal["raise", "do_nothing"]` for assistants at `libs/sdk-py/langgraph_sdk/schema.py:90-95`).
- **Stream transport and stream contract are entangled.** `cancel_on_disconnect` on `runs.stream` is a transport-side kill switch (`libs/sdk-py/langgraph_sdk/_async/runs.py:1102`), but for `runs.wait` the disconnect behavior is `on_disconnect` (`libs/sdk-py/langgraph_sdk/schema.py:74-79`). Two knobs for the same concept is awkward.
- **Stream versioning forces duplication.** `runs.stream` has four `@overload` blocks for `thread_id: str | None` × `version: "v1" | "v2"` (`libs/sdk-py/langgraph_sdk/_async/runs.py:74-193`). Adding a v3 would require another two overloads and a new wire decoder.
- **In-process retries sleep synchronously.** `time.sleep(sleep_time)` inside the sync retry loop (`libs/langgraph/langgraph/pregel/_retry.py:674`) blocks the thread-pool worker; for very long backoffs this pins a thread.
- **`after_seconds` is opaque.** The SDK forwards the value but does not document whether it is wall-clock or monotonic, what the minimum is, or what happens if the server is offline when the delay elapses.
- **Background executor re-raises on exit.** `__reraise_on_exit__` defaults to `True` (`libs/langgraph/langgraph/pregel/_executor.py:46, 64, 102-118, 207-209`). A user who submits a node without flagging it loses the exception if the runner exits cleanly — surprising default.
- **Cron `on_run_completed` deletes threads by default.** Stateless crons delete the per-tick thread after the run (`libs/sdk-py/langgraph_sdk/_async/cron.py:217-219`); users have to opt into `keep`. This conflates "scheduled run" with "temporary thread" and breaks log-retention expectations.
- **Two SDKs (`sdk-py` and `sdk-js`) duplicate the trigger contract.** This repository contains both, and the `AGENTS.md:38-50` dependency map flags `sdk-js` as standalone. Drift between the two implementations is a known cost.
- **CLI does not expose trigger operations.** `langgraph_cli` only deploys; there is no `langgraph cron create` or `langgraph runs cancel` (`libs/cli/langgraph_cli/cli.py:276, 412, 529, 758, 873, 918`). Operators must hit the HTTP API directly.

## Failure Modes / Edge Cases

- **Server restart with pending `after_seconds` runs.** No OSS code owns this state; if `langgraph_api.server` is the only durable holder, a server crash loses the scheduled tick unless the underlying store (Postgres/SQLite checkpoint) is also holding it. Not verifiable in this source.
- **Two duplicate `/runs/crons` POSTs.** Both succeed; the SDK has no `OnConflictBehavior` analog for crons (`libs/sdk-py/langgraph_sdk/_async/cron.py:175-289`).
- **Webhook URL typo / 500.** The SDK never retries a webhook — it only forwards the URL. There is no `webhook_retry` or `webhook_secret` field visible in the public schema (`libs/sdk-py/langgraph_sdk/_async/runs.py:469`, `libs/sdk-py/langgraph_sdk/_async/cron.py:101`).
- **Per-task retry inside a foreground run vs server-level retry of a background run.** These are separate layers — `RetryPolicy` (`libs/langgraph/langgraph/types.py:416`) retries a node, not a run. The server may also retry a failed run from scratch; the SDK has no API to introspect that.
- **`__reraise_on_exit__` re-raises the first exception only.** Subsequent tasks' failures are dropped silently (`libs/langgraph/langgraph/pregel/_executor.py:111-118, 207-209`).
- **Idle-timeout watchdog can fire after the task has returned but before the result is committed.** `arun_with_retry` calls `_finish_timed_attempt` from inside the `finally` of `_arun_with_timeout` (`libs/langgraph/langgraph/pregel/_retry.py:514-516, 748`); if the watchdog wins the race, the attempt is marked `error` even though the node succeeded.
- **`PUSH_TRIGGER` collisions.** If a user names a channel `PUSH` or `__pregel_*`, the validator raises (`libs/langgraph/langgraph/pregel/main.py:803-806`).
- **Disconnect during `runs.stream`.** Default is `on_disconnect="cancel"` (`libs/sdk-py/langgraph_sdk/schema.py:74-79`); choosing `continue` does not buffer events client-side, it only tells the server not to abort. The client must separately call `runs.join_stream` with `last_event_id` to recover (`libs/sdk-py/langgraph_sdk/_async/runs.py:1097-1140`).
- **Webhook + `on_run_completed="delete"`.** A failed webhook delivery combined with thread deletion means the run record and its artifacts are gone — the URL alone is the only callback evidence.
- **`runs.cancel_many(status="all")` may cancel in-flight retries.** The `CancelAction` is `interrupt` by default (`libs/sdk-py/langgraph_sdk/_async/runs.py:949-950`), which translates to a `Command.PARENT`-style interruption on the Pregel loop (`libs/langgraph/langgraph/pregel/_retry.py:627-631`). If a node had partial retry budget left it loses it.
- **Cron `enabled=False` does not delete the cron.** Setting `enabled=False` via `crons.update` (`libs/sdk-py/langgraph_sdk/_async/cron.py:322`) just stops future ticks; the cron row remains and a future `enabled=True` resumes from the original schedule.
- **`step_timeout=0` (or unset) means no step-level bound.** A runaway node will only be bounded by its own `TimeoutPolicy` or the global `step_timeout` set at `Pregel(...)` time (`libs/langgraph/langgraph/pregel/main.py:726, 770`). The default in `Pregel.__init__` is `None`.

## Future Considerations

- **Make the scheduler open-source.** Putting `after_seconds`, cron, and webhook delivery in `libs/langgraph` (not behind `langgraph_api.server`) would let self-hosters get the full trigger surface. Today the OSS engine has the *runner* but not the *trigger*.
- **Add an `Idempotency-Key` header** to `runs.create` / `runs.stream` / `crons.create` that the server uses to dedup retried client requests (e.g. HTTP 409 vs 200). The client surface already has typed metadata; an idempotency token is the natural addition.
- **Surface cron tick history in the SDK.** A `cron.runs(cron_id)` method listing past `Run`s would let operators audit which ticks fired (and which `enabled=False` windows were skipped). Currently only the next tick is exposed (`libs/sdk-py/langgraph_sdk/schema.py:408`).
- **Webhook signing.** A `webhook_secret` field (HMAC-SHA256 of the body) would be a low-cost correctness improvement; the SDK schema has room (`libs/sdk-py/langgraph_sdk/schema.py:433, 543`).
- **Backpressure-aware executor.** `AsyncBackgroundExecutor.submit` accepts an optional semaphore (`libs/langgraph/langgraph/pregel/_executor.py:135-140`), but the runner does not report queue depth. Exposing `pending_count` and a soft limit would help long-lived servers.
- **Sync `time.sleep` replacement in retry loop.** `time.sleep(sleep_time)` (`libs/langgraph/langgraph/pregel/_retry.py:674`) should be replaced with an `Event.wait` or `asyncio.sleep`-friendly analog so the thread is released during backoff.
- **CLI parity.** Add `langgraph cron create/list/delete` and `langgraph runs cancel` so operators don't need to script against the HTTP API.
- **`after_seconds` semantic clarity.** Document whether it is wall-clock, monotonic, or anchored to the server's clock. Today it is just `int | None`.
- **Consolidate `on_disconnect` and `cancel_on_disconnect`.** The two near-identical knobs (`libs/sdk-py/langgraph_sdk/schema.py:74-79` vs `libs/sdk-py/langgraph_sdk/_async/runs.py:1102`) should collapse.
- **Make retry budget observable.** `RetryPolicy.max_attempts` is opaque at runtime — the SDK has no API to ask "how many attempts did this run consume?" The runner sets `node_attempt` in `ExecutionInfo` (`libs/langgraph/langgraph/pregel/_retry.py:609, 728`) but it is internal to the engine.
- **Open the scheduler implementation in this repo.** Until the `langgraph_api` cron/queue code is moved into `libs/langgraph` (or a new `libs/scheduler`), every claim about scheduling semantics rests on a closed-source assumption.

## Questions / Gaps

- No in-source implementation of cron dispatch, `after_seconds` delay, webhook delivery, or queue worker — these are server-only. Searched: `libs/`, `libs/cli/langgraph_cli/`, `libs/langgraph/`, `libs/sdk-py/`, `libs/prebuilt/`, `libs/checkpoint*/`. The only cron-related code in OSS is the SDK client (`libs/sdk-py/langgraph_sdk/_async/cron.py`) and its unit/integration tests (`libs/sdk-py/tests/test_crons_client.py`, `libs/sdk-py/tests/integration/test_crons.py`).
- No documented idempotency-key pattern. Searched: `Idempotency`, `idempotency_key`, `idempotent`. No clear evidence found in this source.
- No webhook signing/HMAC hook in any SDK payload. Searched: `webhook_secret`, `signature`, `hmac`. No clear evidence found in this source.
- No in-process scheduler primitive. Searched: `croniter`, `apscheduler`, `asyncio.create_task` near a tick scheduler. No clear evidence found in this source.
- No OTel/tracing hook in the open-source SDK or engine for trigger-side spans. `langsmith_tracing` is forwarded (`libs/sdk-py/langgraph_sdk/_async/runs.py:590`) but the receiving server is closed-source.
- Cron "missed-run" / catch-up policy is not documented in the OSS schema (`Cron.end_time` exists, but `next_run_date` is read-only). Behavior on missed ticks (server down, `enabled=False` between ticks) cannot be answered from this source.
- The integration test in `libs/sdk-py/tests/integration/test_crons.py:32-34` deliberately uses `_DISTANT_SCHEDULE = "0 0 1 1 *"` (once a year) to avoid any tick during the test — confirming that no in-source harness exercises a *real* cron firing.
- The exact failure handling of duplicate cron rows with overlapping schedules (e.g. two crons targeting the same thread at the same minute) is server-side behavior, not in this source.
- Whether the SDK's `httpx.AsyncHTTPTransport(retries=5)` (`libs/sdk-py/langgraph_sdk/_async/client.py:128-129`) interacts with `after_seconds` delivery (e.g. retried POSTs to `/threads/.../runs` after the server delays the run) is not documented.
- Whether `webhook` delivery in the server honors any retry/backoff policy is not observable in this source.

---

Generated by `01.06-scheduling-and-trigger-semantics` against `langgraph`.