# Source Analysis: openai-agents-sdk

## Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+ library (Pydantic v2, httpx/openai client, optional extras for Redis/SQLAlchemy/LiteLLM/etc.) |
| Analyzed | 2026-08-24 |

## Summary

The OpenAI Agents SDK is a **library-first harness**: it ships as an importable package (`pyproject.toml:2` declares `name = "openai-agents"` with **no `[project.scripts]` console entry points**), and the primary embedding surface is three classmethods — `Runner.run`, `Runner.run_sync`, `Runner.run_streamed` (`src/agents/run.py:256`, `src/agents/run.py:361`, `src/agents/run.py:464`) — that accept per-call dependency injection through parameters (`context`, `session`, `hooks`, `run_config`, `error_handlers`; `src/agents/run.py:256-270`). Nearly every host concern is behind a small typed protocol or ABC that host code can implement structurally: model backends (`Model`/`ModelProvider`, `src/agents/models/interface.py:37,138`), storage (`Session` protocol, `src/agents/memory/session.py:15-56`), telemetry sinks (`TracingProcessor`, exported from `src/agents/tracing/__init__.py:94-112`), tool execution callbacks (`ShellExecutor`, `LocalShellExecutor`, `ComputerProvider`, `ApplyPatchEditor` in `src/agents/__init__.py:146-171`), and sandbox execution (`BaseSandboxClient` injected via `SandboxRunConfig.client`, `src/agents/run_config.py:222`). Progress is surfaced via semantic stream events (`src/agents/stream_events.py:11-52`) and lifecycle hooks (`src/agents/lifecycle.py:13-207`); approvals are surfaced as resumable interruptions with a durable serialized state boundary (`RunState.to_string/from_string`, `src/agents/run_state.py:2042,2100`).

The main ergonomic caveats are (1) several process-wide singletons with lazy side effects — a global trace provider registered at `atexit` (`src/agents/tracing/setup.py:11,34-36`), a lazily spawned daemon export thread (`src/agents/tracing/processors.py:584-595`), module-global OpenAI defaults (`src/agents/models/_openai_shared.py:9-15`), and a swappable global default runner (`src/agents/run.py:164,191-197`); (2) tracing export to the OpenAI backend is on by default unless disabled (`OPENAI_AGENTS_DISABLE_TRACING`, `src/agents/tracing/provider.py:346-352`); and (3) `run_sync` deliberately creates/reuses a thread-persistent event loop and refuses to run where a loop is already active (`src/agents/run.py:2256-2279`), which pushes async hosts onto the async API.

## Rating

**8 / 10.** Clear embedding model with explicit interfaces, tests, operational safeguards, and rich examples for local-first *and* service deployments (FastAPI/Celery patterns documented in `docs/tracing.md:61-100`, FastAPI lifespan pattern in `MCPServerManager` docstring, `src/agents/mcp/manager.py:167-174`). It falls short of 9–10 because telemetry egress plus a background export thread are enabled by default (hosts must know to disable or redirect them), several mutable process globals exist as convenience shortcuts, shutdown beyond `atexit` requires explicit `flush_traces()` calls, and there is no first-class server/CLI packaging — hosts build those themselves.

## Evidence Collected

Every entry includes file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library-only packaging | `[project] name = "openai-agents"`; no `[project.scripts]` section exists; extras for voice/redis/sqlalchemy/docker/temporal | `pyproject.toml:2,36-59` |
| Primary entry points | `Runner.run` / `run_sync` / `run_streamed` classmethods delegating to swappable `DEFAULT_AGENT_RUNNER` | `src/agents/run.py:254-540` |
| Global runner swap (explicitly non-public) | `set_default_agent_runner` docstring: "WARNING: this class is experimental and not part of the public API" | `src/agents/run.py:191-197` |
| Per-run config object | `RunConfig` dataclass: `model_provider`, `model_settings`, guardrails, tracing flags, `workflow_name`, `trace_id`, `group_id`, filters/formatters, `sandbox`, `tool_execution` | `src/agents/run_config.py:350-496` |
| Dict-based config coercion at boundaries | `_coerce_run_config(value)` accepts dicts; used by `AgentRunner.run_streamed` | `src/agents/run_config.py:606-608`, `src/agents/run.py:2333` |
| Model backend injection | `Model` ABC (`get_response`, `stream_response`, `close`, `_cleanup_on_run_end`); `ModelProvider.get_model` + `aclose` | `src/agents/models/interface.py:37-135,138-161` |
| Multi-provider registry | `MultiProvider` maps prefixes (`openai/`, `litellm/`, `any-llm/`, custom) to providers; constructor takes explicit keys/client overrides | `src/agents/models/multi_provider.py:62-153` |
| Process-global OpenAI defaults | Module-level `_default_openai_key/_default_openai_client/_use_responses_by_default`; setters re-exported as public API | `src/agents/models/_openai_shared.py:9-15,18-57`, `src/agents/__init__.py:278-338` |
| Host-supplied storage contract | `Session` runtime-checkable Protocol (`get_items/add_items/pop_item/clear_session`); `SessionABC` says third parties should implement the Protocol; optional opt-in `wrapper` param detection | `src/agents/memory/session.py:15-67,155-196` |
| Built-in + extension storage | SQLiteSession lazy export; extensions for SQLAlchemy, Redis, MongoDB, Dapr, encrypted sessions | `src/agents/__init__.py:264-275`, `src/agents/extensions/memory/` (sqlalchemy_session.py, redis_session.py, mongodb_session.py, dapr_session.py, encrypt_session.py) |
| Host-supplied tools/executors | Exports include `ShellExecutor`, `LocalShellCommandRequest`, `ComputerProvider`, `ApplyPatchEditor`, `ToolCaller`, `function_tool` | `src/agents/__init__.py:159-171,202-205,559-562` |
| Policy injection | Per-run input/output guardrails; tool guardrails; `tool_not_found_behavior`, `tool_name_collision_policy`, `output_guardrail_blocked_message` | `src/agents/run_config.py:391-395,472-496` |
| Telemetry sink injection | `add_trace_processor`, `set_trace_processors`, `set_trace_provider`, `set_tracing_disabled`, `set_tracing_export_api_key`, `flush_traces` | `src/agents/tracing/__init__.py:94-130` |
| Per-run tracing config | `RunConfig.tracing: TracingConfig` (api_key, include_task_and_turn_spans), `trace_include_sensitive_data` env-default true | `src/agents/run_config.py:397-429`, `src/agents/tracing/config.py:6-18`, `src/agents/run_config.py:53-56` |
| Global trace provider singleton | `GLOBAL_TRACE_PROVIDER` lazily initialized on first access; `atexit.register(_shutdown_global_trace_provider)` once | `src/agents/tracing/setup.py:11-16,27-64` |
| Hidden background work | `BatchTraceProcessor` spawns daemon worker thread on first span; drops items when queue full; exports to OpenAI by default | `src/agents/tracing/processors.py:541-595,597-621`, `docs/tracing.md:9,63` |
| Tracing disable switch | Env `OPENAI_AGENTS_DISABLE_TRACING` honored lazily; manual override takes precedence | `src/agents/tracing/provider.py:339-356` |
| Async-safe run context | Current trace/span held in `contextvars.ContextVar`, not thread globals | `src/agents/tracing/scope.py:11-17` |
| Lifecycle hooks contract | `RunHooksBase` (on_llm_start/end, on_agent_start/end, on_handoff, on_tool_start/end) and per-agent `AgentHooksBase` | `src/agents/lifecycle.py:13-207` |
| Streaming output contract | `RunResultStreaming.stream_events()` yields semantic events (`RawResponsesStreamEvent`, `RunItemStreamEvent`, `AgentUpdatedStreamEvent`) | `src/agents/result.py:882-1024`, `src/agents/stream_events.py:11-62` |
| Cancellation API | `cancel(mode="immediate"\|"after_turn")` with documented semantics and "continue consuming stream_events" note; `_cleanup_tasks()` cancels loop+guardrail tasks | `src/agents/result.py:818-864,1090-1098` |
| Sync-entry cancellation safety | `run_sync` cancels lingering task on caller abort and shuts down async generators; raises RuntimeError if a loop is already running | `src/agents/run.py:2256-2279,2300-2315` |
| Run-end cleanup ownership | `finally`: `cleanup_models_after_run`, `sandbox_runtime.cleanup()`, `dispose_resolved_computers`, span finish | `src/agents/run.py:2146-2195` |
| Error surfacing to host | Typed exceptions (`AgentsException` hierarchy) carry `run_data: RunErrorDetails`; `error_handlers` keyed by kind accepted per run | `src/agents/exceptions.py:414-482`, `src/agents/run_error_handlers.py:17-50`, `src/agents/run_config.py:602` |
| Silent streaming failure visibility | `run_loop_exception` property documents early-failure case and gives callers a reliable check | `src/agents/result.py:790-816` |
| Approvals surfaced as data | `interruptions: list[ToolApprovalItem]` on results; `RunState.approve/reject`; durable JSON serialization incl. approvals and pending step | `src/agents/result.py:516-517,650-651`, `src/agents/run_state.py:1255-1298,1704,2042` |
| Pause/resume boundary | `RunState` docstring: "durable pause/resume boundary"; conservative context serialization with optional serializer/deserializer | `src/agents/run_state.py:749-762,2042-2064,2100-2126` |
| MCP lifecycle ownership | `Agent.mcp_servers` docstring: hosts must call `connect()`/`cleanup()`; `MCPServerManager` context manager with FastAPI lifespan example, failure tolerance and reconnect | `src/agents/agent.py:202-205`, `src/agents/mcp/manager.py:151-190`, `src/agents/mcp/server.py:583-607` |
| Worker/service deployment docs | Celery and FastAPI BackgroundTasks examples calling `flush_traces()` after each unit of work | `docs/tracing.md:61-100` |
| Embedded server examples | Realtime apps embed `RealtimeRunner`/`RealtimeSession` in FastAPI/websocket servers (`__aenter__`/`close` lifecycle) | `examples/realtime/app/server.py`, `src/agents/realtime/session.py:175,266,308,342` |
| Human-in-the-loop embedding example | Full pause → serialize → approve/reject → resume loop using `result.interruptions` + `state.to_json()` | `examples/agent_patterns/human_in_the_loop.py:82-100` |
| Test doubles for embedders | `ScriptedModel(Model)` and scripted sandbox session utilities shipped inside the package (`agents.testing`) | `src/agents/testing/model.py:249`, `src/agents/testing/sandbox.py:56-150` |
| Demo REPL helper | `run_demo_loop` importable helper (no console script) | `src/agents/repl.py:15` |
| Secret hygiene in errors | Data-redacted error machinery detaches tracebacks and clears locals before re-raising redacted failures out of `Runner.run` | `src/agents/exceptions.py:40-68`, `src/agents/run.py:340-358` |

## Answers to Dimension Questions

**1. Can the harness run inside another application without owning the whole process?**
Yes. It is a pure library: importing `agents` has no required side effects by design ("Keep top-level imports free of optional-dependency failures and runtime side effects; use lazy exports when needed" — `AGENTS.md`, Public API Compatibility section; implemented via lazy `SQLiteSession` export `src/agents/__init__.py:268-273` and lazy trace-provider init "so importing the SDK does not create network clients or threading primitives", `src/agents/tracing/setup.py:42-44`). A run is driven by a `Runner` call scoped to one asyncio task, with cleanup in `finally` blocks (`src/agents/run.py:2146-2195`). Caveats: the first traced run starts a daemon exporter thread (`src/agents/tracing/processors.py:584-595`), and `atexit` handlers are registered once tracing initializes (`src/agents/tracing/setup.py:34-36,62-64`). `run_sync` also pins a persistent per-thread default event loop (`src/agents/run.py:2264-2279`).

**2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?**
Yes, all six:
- Policy: per-run guardrails and tool-behavior policies on `RunConfig` (`src/agents/run_config.py:391-395,472-496`); tool/MCP approval predicates (`needs_approval`, `src/agents/mcp/server.py:79-83,710-846`).
- Tools: function tools, shell/computer/apply_patch executor callbacks, custom `Tool` implementations (`src/agents/__init__.py:140-205`).
- Identity/secrets: explicit API keys per provider (`MultiProvider(openai_api_key=..., openai_client=...)`, `src/agents/models/multi_provider.py:80-92`), per-run tracing key (`TracingConfig.api_key`, `src/agents/tracing/config.py:9`), or host-owned `AsyncOpenAI` client (`set_default_openai_client`, `src/agents/__init__.py:293-303`). Per-run user identity flows through the typed `context: TContext` parameter wrapped in `RunContextWrapper` (`src/agents/run_context.py`, referenced throughout `src/agents/run.py:260`).
- Storage: `Session` protocol implementations are structurally checked, so any object works (`src/agents/memory/session.py:15-56`).
- Telemetry: `TracingProcessor` registration or full provider replacement (`src/agents/tracing/__init__.py:94-112`).

**3. Are lifecycle, cancellation, shutdown, and error propagation explicit?**
Largely yes. Hooks define lifecycle callbacks (`src/agents/lifecycle.py:18-103`); cancellation is a documented two-mode API on streaming runs (`src/agents/result.py:818-864`) with internal task teardown (`src/agents/result.py:1090-1098`) and sync-path abort handling (`src/agents/run.py:2300-2315`); resource release is deterministic in `finally` blocks (`src/agents/run.py:2146-2195`) and mirrored on the model/provider contracts (`Model.close`, `ModelProvider.aclose`, `src/agents/models/interface.py:47-57,155-161`). Shutdown of telemetry is less explicit: it relies on `atexit` plus manual `flush_traces()` for delivery guarantees in workers (`src/agents/tracing/setup.py:16-24`; `docs/tracing.md:63-69`). Errors propagate as a typed hierarchy carrying structured `run_data` (`src/agents/exceptions.py:414-482`), with an optional keyed handler map (`src/agents/run_error_handlers.py:50`) and a documented escape hatch for silent streaming failures (`result.run_loop_exception`, `src/agents/result.py:790-816`).

**4. Does the integration model work for both local-first and service deployments?**
Yes. Local-first: `run_sync` quickstart path (`README.md:54-71`) and REPL demo (`src/agents/repl.py:15`). Service deployments: documented FastAPI/Celery integration including immediate-export guidance (`docs/tracing.md:61-100`), a FastAPI lifespan pattern for MCP connection ownership (`src/agents/mcp/manager.py:167-174`), realtime voice servers under `examples/realtime/app/server.py`, and cross-process pause/resume via serialized `RunState` (`src/agents/run_state.py:2042,2100`) exercised in `examples/agent_patterns/human_in_the_loop.py:96-100`. A Temporal extra (`pyproject.toml:56-59`) and temporal sandbox example (`examples/sandbox/extensions/temporal/temporal_session_manager.py`) show workflow-engine embedding.

## Architectural Decisions

- **Library over service/CLI**: no console scripts, no bundled server; embedding is achieved purely through imports and per-call parameters (`pyproject.toml`; `src/agents/run.py:254-541`). This keeps process ownership with the host but means every serving pattern is DIY.
- **Per-run configuration object with dict coercion**: `RunConfig` centralizes policy/tracing/tool settings and accepts plain dicts at public boundaries (`src/agents/run_config.py:350-496,606-608`), lowering friction for hosts building configs from external stores.
- **Structural protocols for host dependencies**: `Session` is a `runtime_checkable` Protocol explicitly recommended over the ABC for third parties (`src/agents/memory/session.py:15-67`); models use an ABC but ship in-package scripted test doubles (`src/agents/testing/model.py:249`) so embedders can validate integrations without network access.
- **Global defaults as opt-in convenience, not requirement**: shared OpenAI key/client/API-mode globals (`src/agents/models/_openai_shared.py:9-68`) are consulted only when a call site does not pass an explicit client/provider; `MultiProvider` allows fully instance-scoped configuration (`src/agents/models/multi_provider.py:76-153`).
- **Durable interruption boundary instead of in-memory callbacks for approvals**: approvals materialize as `ToolApprovalItem`s plus a serializable `RunState`, enabling approval UX to live in a different process than the agent loop (`src/agents/run_state.py:749-762`; `examples/agent_patterns/human_in_the_loop.py:82-100`).
- **Contextvars-based trace scoping**: concurrent runs in one process keep independent trace/span stacks without thread-global locks (`src/agents/tracing/scope.py:11-17`).

## Notable Patterns

- **Graceful vs hard cancel**: `cancel(mode="after_turn")` lets the current LLM turn finish, executes pending tools, saves session state, then stops; `"immediate"` tears down now and unblocks consumers with a sentinel (`src/agents/result.py:818-864`). This is a host-friendly contract rarely seen in similar libraries.
- **Redacted-error discipline**: exceptions that may carry sensitive payloads are marked, their tracebacks detached/locals cleared before crossing the public boundary, so hosts can log raised errors without leaking inputs (`src/agents/exceptions.py:40-68`; `src/agents/run.py:340-358,2200-2226`).
- **Failure-tolerant MCP manager**: connects servers on the calling task, exposes only connected ones, records failures for `reconnect(failed_only=True)`, and keeps cleanup task-affinity (`src/agents/mcp/manager.py:151-190`).
- **Lazy everything**: default trace processor/exporter, SQLiteSession import, and worker threads are created on first use to stay fork/import safe (`src/agents/tracing/processors.py:720-724`; `src/agents/__init__.py:268-275`; `src/agents/tracing/setup.py:42-44`).
- **Operational safeguards on shutdown**: bounded shutdown timeouts, non-fatal drop warnings, and exporter exception isolation so a failing exporter cannot strand queued spans (`src/agents/tracing/provider.py:177-204`; `src/agents/tracing/processors.py:623-651,668-717`).

## Tradeoffs

- **Default-on telemetry egress vs zero-config**: traces export to OpenAI's backend unless `OPENAI_AGENTS_DISABLE_TRACING` is set or processors are replaced (`src/agents/tracing/provider.py:346-352`; `docs/tracing.md:9`); `trace_include_sensitive_data` also defaults to true from env (`src/agents/run_config.py:53-56`). Great for quickstarts, risky for hosts with strict egress/compliance postures that miss the flag.
- **Convenience globals vs multi-tenancy**: `set_default_openai_key/client/api` mutate process-wide state (`src/agents/__init__.py:278-310`), which is unsafe for per-request credentials; hosts must instead construct explicit clients/providers per tenant.
- **Sync ergonomics vs async purity**: `run_sync` refuses to run inside an existing loop (`src/agents/run.py:2256-2262`) and intentionally leaks a persistent default loop per thread (`src/agents/run.py:2277-2279`); simple for scripts, surprising inside hybrid apps mixing sync and async entry points.
- **Streaming power vs complexity**: hosts get semantic events, consumer accounting, drain semantics, and weakref-based agent release (`src/agents/result.py:882-1013,384-410`) — powerful, but correct consumption requires following subtle rules like continuing to drain after `cancel()` (`src/agents/result.py:841-842`).
- **MCP ownership delegated to host**: flexible, but the host must remember connect/cleanup; the SDK mitigates with `MCPServerManager` rather than taking ownership (`src/agents/agent.py:202-205`).

## Failure Modes / Edge Cases

- **Queue-full trace loss**: when the 8192-item export queue fills, spans/traces are dropped with only a warning (`src/agents/tracing/processors.py:551,597-621`), and shutdown timeouts likewise drop queued items (`src/agents/tracing/processors.py:637-645`). Hosts needing audit-grade telemetry must provide their own processor.
- **Silent streaming failure before events flow**: early failures (e.g., sandbox init) may not re-raise through `stream_events`; the SDK adds `run_loop_exception` as an explicit check (`src/agents/result.py:790-816`), but hosts that skip the check can miss errors.
- **`atexit`-only flush in long-lived processes**: hosts that never exit must call `flush_traces()` themselves for prompt export (`docs/tracing.md:63-69`).
- **Cancellation correctness burden**: after `cancel()`, the host must keep consuming `stream_events()` until completion, else cleanup ordering races session persistence (`src/agents/result.py:841-842,983-1009`).
- **Event-loop pinning in `run_sync`**: because loop-bound primitives (locks in Redis/SQLAlchemy sessions) assume the same default loop, mixing `asyncio.run()` with `run_sync` on one thread can break session reuse (`src/agents/run.py:2244-2279`).
- **Server-managed conversation constraints**: handoff input filters and some history features are unsupported with `conversation_id`/`previous_response_id` modes, surfacing as warnings/errors at runtime rather than construction time (`src/agents/run_config.py:366-389`).

## Future Considerations

- Make the default tracing posture opt-in (or emit a one-time warning when export is enabled implicitly), since the current default sends process data off-host (`src/agents/tracing/provider.py:346-352`).
- Provide an explicit `shutdown()`/context-manager wrapper around the whole SDK (tracing provider + default runner resources) so long-lived hosts don't depend on `atexit`.
- Promote an official serving recipe (the pieces exist: FastAPI lifespan pattern in `src/agents/mcp/manager.py:167-174`, worker flush guidance in `docs/tracing.md:61-100`) into a supported higher-level embedding API.
- Consider scoped (non-global) variants of `set_default_openai_*` helpers for multi-tenant hosts, reducing reliance on process-wide mutation.

## Questions / Gaps

- No dedicated "embedding" documentation page was found. Searches covered `docs/` top level (`running_agents.md`, `tracing.md`, `results.md`, `streaming.md`, `testing.md`), `mkdocs.yml` nav, and README; integration guidance is distributed across tracing/worker docs, MCP manager docstrings, and examples rather than consolidated.
- No evidence found of a first-class CLI or hosted-service mode anywhere in the repo (checked `pyproject.toml` for scripts, `src/agents/` for argparse/click entry points); `repl.py:15` is the closest artifact and is a library function.
- The interaction between the persistent `run_sync` default loop and hosts that also call `asyncio.run()` on the same thread is described in comments (`src/agents/run.py:2244-2279`) but I did not find a test demonstrating the failure mode; behavior under mixed-loop usage remains inferred intent.

---

Generated by `Dimension 24.04: Embedding and Host Integration Ergonomics` against `openai-agents-sdk`.
