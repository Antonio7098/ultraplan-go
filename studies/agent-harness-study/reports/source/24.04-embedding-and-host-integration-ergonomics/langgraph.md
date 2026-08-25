# Source Analysis: langgraph

## Dimension 24.04: Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core, checkpointers, SDK, CLI); JS/TS SDK moved to external repo |
| Analyzed | 2026-08-24 |

## Summary

LangGraph is designed embed-first: the core artifact is a pure in-process Python library whose compiled graph implements the LangChain `Runnable` interface (`libs/langgraph/langgraph/pregel/protocol.py:25`, `libs/langgraph/langgraph/pregel/main.py:450-453`), so a host application invokes it with `invoke`/`stream`/`batch` (sync and async) without ceding process ownership. All heavyweight dependencies are injected at compile time — checkpointer (persistence), store (long-term memory), cache, interrupt points, and stream transformers are constructor arguments of `StateGraph.compile(...)` and `Pregel.__init__(...)` (`libs/langgraph/langgraph/graph/state.py:1177-1188`, `libs/langgraph/langgraph/pregel/main.py:758-836`), not globals. Per-run dependencies (context, store handle, stream writer, heartbeat, drain control) travel through an explicit `Runtime` object injected via run config (`libs/langgraph/langgraph/runtime.py:125-238`). Progress is surfaced through seven typed stream modes plus a pull-based v3 projection API (`libs/langgraph/langgraph/types.py:122-136`, `libs/langgraph/langgraph/stream/run_stream.py:37-55`); approvals use a resumable `interrupt()`/`Command(resume=...)` protocol that pauses execution at a durable checkpoint (`libs/langgraph/langgraph/types.py:851-914`). Cancellation and shutdown are explicit and cooperative: hosts pass a `RunControl` to request a drained shutdown that persists a checkpoint for later resume (`libs/langgraph/langgraph/runtime.py:79-104`, `libs/langgraph/langgraph/errors.py:54-62`). Three additional embedding modes exist beyond the library: a server API with first-party Python SDK clients (`libs/sdk-py/langgraph_sdk/_sync/runs.py`), a drop-in `RemoteGraph` proxy usable as a node inside another graph (`libs/langgraph/langgraph/pregel/remote.py:118-127`), and a CLI that builds/launches a dev deployment from a `langgraph.json` manifest (`libs/cli/langgraph_cli/cli.py:278`, `libs/langgraph/tests/example_app/langgraph.json:1-6`). The main caveats are ambient contextvar-based config propagation (documented as broken on async Python < 3.11), import-time environment-variable reads, and an experimental v3 streaming surface.

## Rating

**9 / 10** — Clear, explicit embedding model with strong tests and operational safeguards. Dependency injection is comprehensive (storage, cache, identity, telemetry callbacks, executor, secrets-via-serde), lifecycle/cancellation/shutdown are first-class (`RunControl` drain with checkpoint persistence; per-run executor context managers that cancel, wait, and re-raise), and streaming/approval/error contracts are typed and versioned. It falls short of 10 because: (a) ambient `contextvar` config inheritance creates subtle cross-run coupling and a documented hard failure on async Python < 3.11 (`libs/langgraph/langgraph/config.py:53-56`); (b) a few knobs are read from environment variables at import time rather than injected (`libs/langgraph/langgraph/_internal/_config.py:32-35`); (c) the newest streaming API is explicitly marked experimental (`libs/langgraph/langgraph/stream/run_stream.py:51-54`); and (d) policy enforcement and identity live in the separate server product — this repo only ships the protocol types.

## Evidence Collected

Every entry cites file paths relative to `studies/agent-harness-study/sources/langgraph`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library entry point | `StateGraph.compile(checkpointer, store, cache, interrupt_before/after, debug, name, transformers)` returns a runnable compiled graph | `libs/langgraph/langgraph/graph/state.py:1177-1188` |
| Runnable contract | `PregelProtocol(Runnable[InputT, Any])`; Pregel subclasses it, so hosts get invoke/stream/batch sync+async | `libs/langgraph/langgraph/pregel/protocol.py:25`; `libs/langgraph/langgraph/pregel/main.py:450-453` |
| Explicit DI of runtime deps | `Pregel.__init__` takes `checkpointer`, `store`, `cache`, `retry_policy`, `cache_policy`, `context_schema`, `config`, assigned to instance attrs | `libs/langgraph/langgraph/pregel/main.py:758-836` |
| Low-level direct embedding | Docstring: "for advanced use cases, Pregel can be used directly" with channels/nodes example | `libs/langgraph/langgraph/pregel/main.py:520-549` |
| Functional-API embedding | `@task` / `@entrypoint` decorators wrap plain functions as graphs | `libs/langgraph/langgraph/func/__init__.py:110-132` (see `__all__` at line 59) |
| Run-scoped dependency bundle | `Runtime` dataclass carries `context`, `store`, `stream_writer`, `heartbeat`, `previous`, `execution_info`, `server_info`, `control`; retrieved via `get_runtime()` | `libs/langgraph/langgraph/runtime.py:125-238`, `296-310` |
| Typed run context injection | Host passes `context=` to `invoke/stream`; validated against `context_schema` declared at compile time | `libs/langgraph/langgraph/pregel/main.py:3819-3856` (invoke signature/context doc); `libs/langgraph/langgraph/graph/state.py:1240-1241` |
| Host-provided persistence | `BaseCheckpointSaver` abstract contract: `get_tuple`, `list`, `put`, `put_writes`, `delete_thread`; pluggable `serde` | `libs/checkpoint/langgraph/checkpoint/base/__init__.py:176-329` |
| Reference storage backends | `InMemorySaver` (in-process default) and `PostgresSaver.from_conn_string` + explicit `setup()` migration step | `libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33`; `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:40-108` |
| Host-provided long-term memory | `BaseStore` ABC with abstract `batch`/`abatch` ops; optional semantic index and TTL flags | `libs/checkpoint/langgraph/store/base/__init__.py:708-754` |
| Host-provided node cache | `BaseCache` ABC (`get`/`set`) injectable via `compile(cache=...)` | `libs/checkpoint/langgraph/cache/base/__init__.py:15-33`; `libs/langgraph/langgraph/graph/state.py:1181,1186` |
| Host-supplied thread pool | Sync loop enters `BackgroundExecutor(self.config)`; executor obtained from run config (`get_executor_for_config`) so host controls concurrency | `libs/langgraph/langgraph/pregel/_loop.py:1691`; `libs/langgraph/langgraph/pregel/_executor.py:48-50` |
| Concurrency cap per run | `AsyncBackgroundExecutor` builds semaphore from `config["max_concurrency"]` | `libs/langgraph/langgraph/pregel/_executor.py:131-140` |
| Telemetry/observability hooks | `callbacks` key merged across configs; callback managers built from config tags/metadata | `libs/langgraph/langgraph/_internal/_config.py:180-183`, `236-272` |
| Custom stream transformers | Host can append `StreamTransformer`s at compile time, propagated to subgraphs | `libs/langgraph/langgraph/graph/state.py:1220-1226`; `libs/langgraph/langgraph/pregel/main.py:783,831-833` |
| Streaming modes surfaced to host | `StreamMode = values/updates/custom/messages/checkpoints/tasks/debug` documented | `libs/langgraph/langgraph/types.py:122-136` |
| Typed stream parts (v2) | `StreamPart` family incl. task start/result payloads carrying `error` and `interrupts` fields | `libs/langgraph/langgraph/types.py:57-64`, `144-179`, `264-276` |
| Pull-based v3 stream | `GraphRunStream`: caller's iteration drives the graph; no background pump thread; projections single-consumer with `.tee(n)` | `libs/langgraph/langgraph/stream/run_stream.py:37-55` |
| In-node custom progress | `get_stream_writer()` / `Runtime.stream_writer` lets nodes emit custom chunks to host consumers of `stream_mode="custom"` | `libs/langgraph/langgraph/config.py:126-196`; `libs/langgraph/langgraph/runtime.py:206-207` |
| Approval/human-in-the-loop primitive | `interrupt(value)` raises resumable `GraphInterrupt`, value delivered to client; resumed via `Command(resume=...)`; requires checkpointer | `libs/langgraph/langgraph/types.py:851-914` |
| Interrupt surfacing in results | `__interrupt__` constant (`INTERRUPT = "__interrupt__"`) popped into typed field; tests assert `"__interrupt__" in first` output chunk | `libs/langgraph/langgraph/_internal/_constants.py:9`; `libs/langgraph/langgraph/pregel/main.py:4222`; `libs/langgraph/langgraph/tests/test_graph_callbacks.py:116` |
| Lifecycle callbacks for approvals | `GraphInterruptEvent`/`GraphResumeEvent` payloads with status, checkpoint id/ns, interrupts tuple | `libs/langgraph/langgraph/callbacks.py:22-45` |
| Cooperative shutdown | Host-owned `RunControl.request_drain(reason)`; graph raises `GraphDrained` at superstep boundary after saving checkpoint; resumable later | `libs/langgraph/langgraph/runtime.py:79-104`; `libs/langgraph/langgraph/errors.py:54-62`; raise sites `libs/langgraph/langgraph/pregel/main.py:3015`, `3496` |
| Drain passed as run argument | `stream(..., control: RunControl \| None)` / `astream(..., control=...)` parameters | `libs/langgraph/langgraph/pregel/main.py:2628`, `3075` |
| Cancellation semantics (server) | SDK `runs.cancel(action="interrupt"\|"rollback")`, `cancel_many`, `join`, `join_stream` | `libs/sdk-py/langgraph_sdk/_sync/runs.py:925-1041`, `1042-1079` |
| Background-task hygiene | Executors cancel flagged tasks, wait for pending work, re-raise first error on exit; `GraphBubbleUp` treated as signal not failure; no atexit hooks or daemon threads found | `libs/langgraph/langgraph/pregel/_executor.py:93-121`, `186-211` |
| Error taxonomy for hosts | `ErrorCode` enum, `GraphRecursionError`, `GraphInterrupt`, `NodeCancelledError`, `NodeTimeoutError` etc., each mapped to troubleshooting docs | `libs/langgraph/langgraph/errors.py:34-39`, `67-102` |
| Checkpoint-failure propagation | Faulty checkpointer errors propagate to caller; invalid checkpointer rejected at compile time by `ensure_valid_checkpointer` | `libs/langgraph/langgraph/tests/test_pregel.py:162-197` (tests `test_invalid_checkpointer_type`, `test_checkpoint_errors`); `libs/langgraph/langgraph/graph/state.py:1231` |
| Secrets handling | Pluggable `EncryptedSerializer(SerializerProtocol)` for checkpoint encryption; remote client reads API keys from env (`LANGGRAPH_API_KEY` etc.) only when not supplied programmatically | `libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:8`, `66-71`; `libs/langgraph/langgraph/pregel/remote.py:155` |
| Identity & policy (server mode) | `BaseUser` protocol (identity, permissions), `StudioUser`, `AuthContext`, `@auth.on` handler pattern in SDK auth module | `libs/sdk-py/langgraph_sdk/auth/types.py:182-215`, `218-279` |
| Server metadata injection point | `ServerInfo(assistant_id, graph_id, user)` — "None when running open-source LangGraph without LangSmith deployments" | `libs/langgraph/langgraph/runtime.py:60-76`, `230-231` |
| Remote embedding (client proxy) | `RemoteGraph` implements graph protocol against any LangGraph Server API; "can be used directly as a node in another Graph"; accepts pre-built clients | `libs/langgraph/langgraph/pregel/remote.py:112-193` |
| CLI/deployment mode | `langgraph up` dev server command; `build` produces Docker image; manifest maps graph name → `module:attr` | `libs/cli/langgraph_cli/cli.py:230-278`, `419`; `libs/langgraph/tests/example_app/langgraph.json:1-6` |
| Server registration example app | `entrypoint`-based app with `entrypoint.final(value=..., save=...)` used as deployable artifact | `libs/langgraph/langgraph/tests/example_app/example_graph.py:68-89` |
| Ambient config inheritance | `ensure_config` seeds from `var_child_runnable_config` contextvar; nested invocations inherit ambient configurable unless they set checkpoint coordinates | `libs/langgraph/langgraph/_internal/_config.py:322-367`, `338-345` |
| Async contextvar limitation | Docs warn `get_store()`/`get_stream_writer()` fail under async Python < 3.11 due to contextvar propagation | `libs/langgraph/langgraph/config.py:53-56`, `132-135` |
| Import-time env reads | `LANGGRAPH_DEFAULT_RECURSION_LIMIT`, `LANGGRAPH_DELTA_MAX_SUPERSTEPS_SINCE_SNAPSHOT` read via `getenv` at module import | `libs/langgraph/langgraph/_internal/_config.py:32-35` |
| Config-bound defaults | `with_config` returns a copy (immutable-ish rebinding) rather than mutating global state | `libs/langgraph/langgraph/pregel/main.py:927-931` |
| High-level agent embedding | `create_react_agent(model, tools, checkpointer=..., store=...)` forwards deps to compile | `libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-300`, `996-997` |
| Drain scheduling test | `test_request_drain_allows_inflight_call_scheduling` proves in-flight child tasks complete during drain | `libs/langgraph/langgraph/tests/test_pregel.py:140-160` |

## Answers to Dimension Questions

**1. Can the harness run inside another application without owning the whole process?**
Yes — this is the primary mode. A compiled graph is a `Runnable` object (`libs/langgraph/langgraph/pregel/protocol.py:25`); the host calls `invoke`/`stream`/`ainvoke`/`abatch` in its own process and event loop (`libs/langgraph/langgraph/pregel/main.py:2616-2660`, `3819-3856`). There are no required singletons, no background threads started at import, and no port binding. Node executors are per-run context managers derived from the host-supplied run config and torn down at run exit (`libs/langgraph/langgraph/pregel/_loop.py:1691`, `libs/langgraph/langgraph/pregel/_executor.py:90-121`). The server/CLI path (`libs/cli/langgraph_cli/cli.py:278`) is opt-in for hosts that want a deployment model instead.

**2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?**
Largely yes, with one split across tiers.
- *Storage*: fully injectable — checkpointer (`libs/checkpoint/langgraph/checkpoint/base/__init__.py:176-329`), store (`libs/checkpoint/langgraph/store/base/__init__.py:708-754`), cache (`libs/checkpoint/langgraph/cache/base/__init__.py:15-33`), all passed at `compile()` time (`libs/langgraph/langgraph/graph/state.py:1179-1182`).
- *Tools*: nodes and prebuilt agents accept arbitrary callables/tools; `create_react_agent(tools=..., ...)` forwards them (`libs/prebuilt/langgraph/prebuilt/chat_agent_executor.py:278-300`).
- *Telemetry*: LangChain `callbacks` flow through every config merge (`libs/langgraph/langgraph/_internal/_config.py:180-183`, `236-272`), plus host-supplied stream transformers (`libs/langgraph/langgraph/graph/state.py:1220-1226`).
- *Secrets*: checkpoint payloads can be encrypted via a pluggable serializer (`libs/checkpoint/langgraph/checkpoint/serde/encrypted.py:8`); remote clients accept explicit keys or fall back to env vars (`libs/langgraph/langgraph/pregel/remote.py:155`).
- *Identity/policy*: defined as protocols and hooks — `BaseUser`/`AuthContext` and `@auth.on` authorization handlers in `libs/sdk-py/langgraph_sdk/auth/types.py:182-215` — but enforcement belongs to the (external) LangGraph Server; in-process library runs have no built-in policy layer, and `ServerInfo.user` is `None` outside deployments (`libs/langgraph/langgraph/runtime.py:60-76`). A host embedding the library must implement its own policy above `interrupt()`/node boundaries.

**3. Are lifecycle, cancellation, shutdown, and error propagation explicit?**
Mostly yes, and unusually well-developed:
- *Cancellation/shutdown*: hosts own a `RunControl` object passed per run (`libs/langgraph/langgraph/pregel/main.py:2628`); `request_drain()` stops the graph cooperatively at the next superstep boundary, raises `GraphDrained` with a reason, and leaves a resumable checkpoint (`libs/langgraph/langgraph/runtime.py:79-104`; `libs/langgraph/langgraph/errors.py:54-62`; `libs/langgraph/langgraph/tests/test_pregel.py:140-160` verifies in-flight tasks finish).
- *Lifecycle*: background tasks are cancelled/waited/re-raised deterministically on executor exit, with `GraphBubbleUp` treated as a control-flow signal rather than an error (`libs/langgraph/langgraph/pregel/_executor.py:81-88`, `174-181`).
- *Errors*: a typed taxonomy (`ErrorCode`, `GraphRecursionError`, `NodeCancelledError`, `NodeTimeoutError`, …) with doc-linked troubleshooting codes (`libs/langgraph/langgraph/errors.py:34-102`); task-level failures appear in stream payloads' `error` fields (`libs/langgraph/langgraph/types.py:167-179`); checkpointer faults surface to the caller (`libs/langgraph/langgraph/tests/test_pregel.py:197`).
- *Gaps*: Postgres checkpointer schema setup is manual (`setup()` "MUST be called directly by the user", `libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:103-107`), which is explicit but easy to miss; and ambient contextvar config means some "lifecycle" state (current config, store, stream writer) propagates implicitly rather than via arguments (`libs/langgraph/langgraph/config.py:17-29`).

**4. Does the integration model work for both local-first and service deployments?**
Yes. Local-first: pure library usage with `InMemorySaver` (`libs/checkpoint/langgraph/checkpoint/memory/__init__.py:33`) or embedded SQLite/Postgres checkpointers, no server required. Service: the same graph objects are registered via `langgraph.json` manifests (`libs/langgraph/tests/example_app/langgraph.json:1-6`), packaged by the CLI into Docker images (`libs/cli/langgraph_cli/cli.py:419`), driven over HTTP/SSE by `langgraph_sdk` clients including remote cancellation and stream joining (`libs/sdk-py/langgraph_sdk/_sync/runs.py:74`, `925-1041`, `1042-1079`), or consumed from other processes via `RemoteGraph`, which composes as an ordinary subgraph node (`libs/langgraph/langgraph/pregel/remote.py:118-127`). The `RemoteGraph.stream_events(version="v3")` path rejects several local-only kwargs explicitly rather than silently degrading (`libs/langgraph/langgraph/pregel/remote.py:195-221`). Note the JS SDK has left this monorepo (`libs/sdk-js/README.md:1-5`), so multi-language parity is maintained out-of-tree.

## Architectural Decisions

1. **Compile-time dependency injection over global registries.** All infra (checkpointer/store/cache) is bound per compiled-graph instance (`libs/langgraph/langgraph/pregel/main.py:819-821`), enabling many isolated embeddings in one process. Tradeoff: configuration like `thread_id` must be threaded through every call's config dict (`libs/langgraph/langgraph/graph/state.py:1204-1214`) instead of living in ambient app state.
2. **Adopting the LangChain `Runnable` interface as the universal embedding contract.** Any host that already speaks `Runnable` (or LangServe/LangGraph Studio tooling) gets invoke/stream/batch for free; `RemoteGraph` exploits the same interface to make a network call look like a local node (`libs/langgraph/langgraph/pregel/protocol.py:25`; `libs/langgraph/langgraph/pregel/remote.py:125-127`). Tradeoff: coupling to `langchain_core` types such as `RunnableConfig` throughout the public surface (`libs/langgraph/langgraph/config.py:5`).
3. **Checkpointer-gated resumability.** Interrupts, drains, and human-in-the-loop all require a checkpointer because pause/resume is implemented as persisted state (`libs/langgraph/langgraph/types.py:869-871`). This unifies approval workflows with durability but makes the simplest embedding (no checkpointer) unable to do interactive approvals.
4. **Cooperative draining instead of hard cancellation.** Shutdown is expressed as a host-owned flag checked at superstep boundaries (`RunControl`, `libs/langgraph/langgraph/runtime.py:79-104`), preserving checkpoint consistency. Hard cancellation exists at the server tier with `action="interrupt"/"rollback"` semantics (`libs/sdk-py/langgraph_sdk/_sync/runs.py:941-942`).
5. **Versioned streaming surfaces (v1 dicts → v2 typed `StreamPart` → v3 pull-based projections).** The repo carries all three concurrently with deprecation warnings (`libs/langgraph/langgraph/pregel/main.py:2655-2660`; `libs/langgraph/langgraph/stream/run_stream.py:51-54`). Tradeoff: large compatibility surface for embedders to navigate.
6. **Split-brain packaging: core library vs. platform.** Policy enforcement, identity resolution, and queueing live in the server product, with only protocol shapes (`ServerInfo`, `BaseUser`, auth types) mirrored here (`libs/langgraph/langgraph/runtime.py:60-76`; `libs/sdk-py/langgraph_sdk/auth/types.py:182`). This keeps the embeddable core small but means OSS hosts must reimplement the auth layer.

## Notable Patterns

- **Per-run resource scoping via context managers.** `BackgroundExecutor.__exit__` cancels flagged tasks, waits for stragglers, then re-raises the first task exception only if the body itself didn't raise (`libs/langgraph/langgraph/pregel/_executor.py:93-121`) — a reusable template for embedding harnesses that spawn work.
- **No-op defaults instead of null checks.** `DEFAULT_RUNTIME` supplies `_no_op_stream_writer`/`_no_op_heartbeat` so nodes never need to guard against missing infrastructure (`libs/langgraph/langgraph/runtime.py:107-110`, `285-293`).
- **Reserved-key namespacing in a shared config bag.** Interned `__pregel_*` config keys (`CONFIG_KEY_RUNTIME`, `CONFIG_KEY_SEND`, …) carry framework state through the host-visible config without colliding with user keys (`libs/langgraph/langgraph/_internal/_constants.py:33-77`).
- **Ambient-config inheritance with coordinate reset.** `ensure_config` inherits the enclosing run's configurable, but drops it entirely when a nested invocation specifies its own checkpoint coordinates, preventing subgraphs from writing into the parent's namespace (`libs/langgraph/langgraph/_internal/_config.py:346-367`).
- **Manifest-driven deployment registration.** The same `entrypoint` object is both an in-process callable and a deployable unit referenced by `module:attr` string in `langgraph.json` (`libs/langgraph/langgraph/tests/example_app/example_graph.py:68-89`; `libs/langgraph/tests/example_app/langgraph.json:3-5`).
- **Secret-aware metadata scrubbing.** Configurable keys containing `key/token/secret/password/auth` substrings are excluded from tracing metadata defaults (`libs/langgraph/langgraph/_internal/_config.py:423-447`).

## Tradeoffs

- **Explicitness vs. ergonomics:** every dependency is injectable, but simple flows require boilerplate (`compile(checkpointer=...)` plus `{"configurable": {"thread_id": ...}}` on each call, `libs/langgraph/langgraph/graph/state.py:1204-1214`).
- **Contextvar convenience vs. correctness risk:** `get_store()`/`get_stream_writer()`/`get_runtime()` read ambient state, which silently breaks under `asyncio` on Python < 3.11 and makes behavior depend on call stack rather than arguments (`libs/langgraph/langgraph/config.py:53-56`, `296-310`).
- **Durability-first approvals:** interrupt/resume requires persisted checkpoints, so ephemeral in-process hosts cannot adopt human-in-the-loop without adding a (possibly memory-only) saver.
- **Three streaming generations:** maximum compatibility, but embedders face a moving target — v3 is flagged experimental (`libs/langgraph/langgraph/stream/run_stream.py:51-54`) while `stream_events` v2 paths carry deprecation shims (`libs/langgraph/langgraph/pregel/main.py:3615-3647` overloads).
- **Monorepo boundary drift:** `sdk-js` moved out-of-repo (`libs/sdk-js/README.md:1-5`) and HumanInterrupt primitives were deprecated toward an external `langchain.agents.interrupt` module (`libs/prebuilt/langgraph/prebuilt/interrupt.py:6-10`), fragmenting where embedders find contracts.

## Failure Modes / Edge Cases

- **Faulty storage surfaces mid-run:** a checkpointer whose `get_tuple` throws fails the run at checkpoint boundaries; covered by `test_checkpoint_errors` (`libs/langgraph/langgraph/tests/test_pregel.py:197`).
- **Invalid checkpointer objects:** rejected eagerly at compile with a `TypeError` (`libs/langgraph/langgraph/graph/state.py:1231`; test at `libs/langgraph/langgraph/tests/test_pregel.py:162-176`), avoiding late surprises.
- **Un-migrated database:** `PostgresSaver` requires a manual `setup()` call; skipping it yields SQL errors on first write rather than a guided failure (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:100-108`).
- **Drain vs. in-flight work:** drain lets already-scheduled child tasks finish before exiting (verified by `test_request_drain_allows_inflight_call_scheduling`, `libs/langgraph/langgraph/tests/test_pregel.py:140-160`); hosts needing immediate abort must use server-tier rollback instead.
- **Sync cancellation limits:** the sync executor can only cancel tasks that have not started (`__cancel_on_exit__` comment at `libs/langgraph/langgraph/pregel/_executor.py:59`); long-running sync nodes cannot be interrupted once running.
- **Env-var timing:** `LANGGRAPH_DEFAULT_RECURSION_LIMIT` is read at import time (`libs/langgraph/langgraph/_internal/_config.py:32`), so setting it after import has no effect — a trap for hosts configuring the library dynamically.
- **Cross-run ambient leakage:** nested graph invocations inherit the outer run's configurable unless checkpoint coordinates are supplied (`libs/langgraph/langgraph/_internal/_config.py:346-367`); hosts reusing threads/executors across logical runs should audit what leaks.

## Future Considerations

- Stabilize and document the v3 pull-based stream (`GraphRunStream`) as the single embedding-facing progress contract, retiring the v1/v2 duality (`libs/langgraph/langgraph/stream/run_stream.py:37-55`).
- Provide a supported alternative to contextvar-based `get_runtime()`/`get_store()` (the code itself notes "in an ideal world, we would have a context manager for the runtime that's independent of the config", `libs/langgraph/langgraph/runtime.py:306-308`).
- Move remaining import-time environment reads behind explicit configuration or lazy evaluation (`libs/langgraph/langgraph/_internal/_config.py:32-35`).
- Ship an in-repo reference showing library-mode policy enforcement (e.g., wrapping nodes with auth checks) since `@auth.on` applies only to server deployments.
- Automate checkpointer schema bootstrap or add a first-run diagnostic for unmigrated stores (`libs/checkpoint-postgres/langgraph/checkpoint/postgres/__init__.py:85`).

## Questions / Gaps

- **Where is the server implementation?** The LangGraph Server that enforces `@auth.on`, resolves `ServerInfo`, and executes queued runs is not part of this repository; only its client SDKs, protocol types, and CLI are present. Search scope: entire `libs/` tree plus README references to "LangSmith Deployment". Consequently, policy-enforcement ergonomics could only be assessed from the client-side contract (`libs/sdk-py/langgraph_sdk/auth/types.py:182-279`).
- **JS embedding story unverifiable here.** `libs/sdk-js/README.md:1-5` redirects to the external `langchain-ai/langgraphjs` repo; no JS source exists in this snapshot to evaluate.
- **Heartbeat/idle-timeout wiring depth.** `Runtime.heartbeat` is documented as the only progress signal honored under `TimeoutPolicy(refresh_on="heartbeat")` (`libs/langgraph/langgraph/runtime.py:209-217`); I did not trace the full watchdog implementation in `libs/langgraph/langgraph/pregel/_retry.py:463-467` to verify edge-case behavior under event-loop starvation.
- **No evidence found** for host-controlled log routing: logging appears delegated to standard `logging` and LangChain tracers (e.g., `debug` flag at `libs/langgraph/langgraph/graph/state.py:1185` feeding `get_debug()`, `libs/langgraph/langgraph/pregel/main.py:818`); no dedicated log-injection API was located within this source.

---

Generated by dimension `24.04-embedding-and-host-integration-ergonomics` against `langgraph`.
