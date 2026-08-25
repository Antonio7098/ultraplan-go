# Source Analysis: pydantic-ai

## 24.04 Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (3.10+), pydantic v2, httpx/httpx2, OpenTelemetry, anyio; uv workspace of packages (`pydantic_ai_slim`, `pydantic_graph`, `pydantic_evals`, `clai`) |
| Analyzed | 2026-08-24 |

## Summary

Pydantic AI is designed first and foremost as an embeddable library: the public surface (`__all__` in `pydantic/__init__.py:181-375` of the package) is a typed SDK whose central object, the generic `Agent[AgentDepsT, OutputDataT]`, is constructed with explicit dependency-injection parameters (`deps_type`, `tools`, `toolsets`, `capabilities`, `instrumentation`) at `pydantic_ai_slim/pydantic_ai/agent/__init__.py:394` and `pydantic_ai_slim/pydantic_ai/agent/__init__.py:534-555`. A host retains ownership of policy, state, telemetry, and UX through several concrete mechanisms: (1) a typed `RunContext` handed to every tool exposes host deps, usage, cancellation, and metadata (`pydantic_ai_slim/pydantic_ai/_run_context.py:61-225`); (2) tool execution can be delegated back to the host entirely via `ExternalToolset` (`pydantic_ai_slim/pydantic_ai/toolsets/external.py:15`) or paused for human approval via deferred tools (`pydantic_ai_slim/pydantic_ai/_deferred.py:27`, resumed with the `deferred_tool_results=` run kwarg at `pydantic_ai_slim/pydantic_ai/agent/abstract.py:476`); (3) telemetry is injected by passing OTel tracer/meter providers through `InstrumentationSettings` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:63`). Beyond plain library embedding, the repo ships distinct integration modes: direct model requests without an agent (`pydantic_ai_slim/pydantic_ai/direct.py:55,108,164,227`), durable-execution worker wrappers for Temporal/DBOS/Prefect (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:112`), web-protocol adapters for AG-UI and Vercel AI frontends (`pydantic_ai_slim/pydantic_ai/ui/ag_ui/_adapter.py:233`, `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_adapter.py:145`), realtime voice sessions (`pydantic_ai_slim/pydantic_ai/realtime/_session.py:467`), a Starlette-hosted chat UI (`pydantic_ai_slim/pydantic_ai/ui/_web/app.py:165`), and a standalone CLI console script (`clai/pyproject.toml:57-58`). Global state is deliberately minimal: per-instance `ContextVar`s power test overrides (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:696-708`), and no import-time background work was found.

## Rating

**9 / 10** — Embedding is the product's core design goal and it shows: multiple explicit run entry points (`run`, `run_sync`, `run_stream`, `iter`, `run_stream_events`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:470,666,826,1536,1356`), a composable capability system for cross-cutting host concerns (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162`), thread-safe cancellation with state snapshot recovery (`pydantic_ai_slim/pydantic_ai/_cancel.py:42`, `pydantic_ai_slim/pydantic_ai/exceptions.py:268,322`), and versioned OTel instrumentation (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:63`). It falls short of 10 because observability has no standard-logging facade (OTel-only; zero `logging.getLogger` usage found in library code), `capture_run_messages` relies on a module-level `ContextVar` with first-run-only semantics (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2519-2560`), and external-cancel attribution degrades on Python 3.10 (`pydantic_ai_slim/pydantic_ai/_cancel.py:227-230`).

## Evidence Collected

Every entry cites file paths with line numbers relative to the source root `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library SDK entry point | Public exports incl. `Agent`, `CancellationToken`, `InstrumentationSettings`, toolsets, exceptions | `pydantic_ai_slim/pydantic_ai/__init__.py:181-375` |
| Agent constructor DI params | `model`, `output_type`, `instructions`, `deps_type`, `tools`, `toolsets`, `capabilities`, `metadata`, `tool_timeout`, `max_concurrency` | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:534-555` |
| Typed DI parameter | `deps_type` exists "solely to allow you to fully parameterize the agent" for static type checking | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:567-570,646` |
| Run-context injection | `RunContext.deps`, `.usage`, `.messages`, `.tracer`, `.retry`, `.tool_call_id`, `.run_id`, `.conversation_id`, `.metadata`, `.cancel()` support fields | `pydantic_ai_slim/pydantic_ai/_run_context.py:61-225` |
| Toolset extension contract | `AbstractToolset` ABC: `get_tools` :166, `call_tool` :171, async lifecycle `__aenter__` :130 / `__aexit__` :137 | `pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:76-171` |
| Composable wrapper toolsets | `WrapperToolset` base for cross-cutting behavior (per repo rule: extend wrappers, don't modify bases) | `pydantic_ai_slim/pydantic_ai/toolsets/wrapper.py:15`; rule at `pydantic_ai_slim/pydantic_ai/AGENTS.md` (rule:987) |
| Host-executed tools | `ExternalToolset` — tools whose calls are surfaced to the host rather than executed locally | `pydantic_ai_slim/pydantic_ai/toolsets/external.py:15` |
| Human approval roundtrip | `DeferredToolRequests` (calls/approvals/metadata) as output type; `DeferredToolResults.build_results(approvals=..., approve_all=...)` | `pydantic_ai_slim/pydantic_ai/_deferred.py:27-60` |
| Approval resume kwarg | `deferred_tool_results:` accepted by `run`/`iter`/`run_stream` signatures | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:476,502,527` |
| Approval exceptions | `CallDeferred` :150, `ApprovalRequired` :168 raised to host control flow | `pydantic_ai_slim/pydantic_ai/exceptions.py:150,168` |
| Per-run override context | `override()` + per-agent ContextVars `_override_deps`/`_override_model`/`_override_toolsets` | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1661`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:696-708` |
| Model/provider injection | Hosts pass `Model` instances directly; `infer_model(..., provider_factory=...)` lets hosts control provider construction | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1529-1544` |
| Secrets handling | `Provider` ABC + `infer_provider`; missing keys raise `UserError` via `missing_api_key_error` (explicit, not silent) | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:32,42,284` |
| Deferred model env check | `defer_model_check=True` defers named-model env-var validation to first run (test/embedding friendly) | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:593-597` |
| Telemetry provider injection | `InstrumentationSettings(tracer_provider=..., meter_provider=..., include_content=...)`; falls back to global OTel providers if omitted | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:63-149` |
| Versioned span schema | Instrumentation versions 2–6 with `DEFAULT_INSTRUMENTATION_VERSION = 5` and deprecation warnings for old versions | `pydantic_ai_slim/pydantic_ai/_instrumentation.py:32,158-163` |
| Capability system | `AbstractCapability(ABC)` with `CombinedCapability` composition; registered via `capabilities=` ctor param | `pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162`; `pydantic_ai_slim/pydantic_ai/capabilities/combined.py:47`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:618-622,628-632` |
| Streaming event contract | `PartStartEvent` :3817, `PartDeltaEvent` :3855, `FinalResultEvent` :3905, union `ModelResponseStreamEvent` :3918 | `pydantic_ai_slim/pydantic_ai/messages.py:3817-3918` |
| Stream handles | `AgentStream` (per-request deltas, `cancel()` :158) and `StreamedRunResult` (`stream_text` :617, `get_output` :678) | `pydantic_ai_slim/pydantic_ai/result.py:52,473` |
| Event-stream handler injection | `EventStreamHandler` protocol type and `event_stream_handler` field on Agent | `pydantic_ai_slim/pydantic_ai/agent/abstract.py:83-88`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:468,692` |
| Error taxonomy for hosts | `UserError` :229 (developer misuse), `AgentRunError` :251 (base), `UnexpectedModelBehavior` :478, `UsageLimitExceeded` (usage.py:418 raises it), `ModelHTTPError` with `status_code`/`body`/`retry_after` | `pydantic_ai_slim/pydantic_ai/exceptions.py:229,251,268,478,525` |
| Cancellation token | Thread-safe single-use `CancellationToken.cancel()` fans out to attached runs; runs attach via `attach_token` | `pydantic_ai_slim/pydantic_ai/_cancel.py:42-61,188` |
| Cancelled-run state recovery | `RunCancelled.all_messages()/.usage/.run_id` snapshot accessors; `from_cancellation()` recovers state from external `CancelledError` | `pydantic_ai_slim/pydantic_ai/exceptions.py:268-444` |
| Usage limits as policy | `UsageLimits` (request/token/cost limits, enforcement hooks) enforced before each model call and mid-stream | `pydantic_ai_slim/pydantic_ai/usage.py:418-459` |
| Message capture channel | `capture_run_messages()` over module-level `_messages_ctx_var` ContextVar | `pydantic_ai_slim/pydantic_ai/_agent_graph.py:2519-2560` |
| Direct API without Agent | `model_request` :55, `model_request_sync` :108, `model_request_stream` :164, `model_request_stream_sync` :227 | `pydantic_ai_slim/pydantic_ai/direct.py:55-227` |
| Durable-worker embedding | `TemporalAgent(WrapperAgent)` wraps any agent, offloading model requests/tools/MCP to Temporal activities; sibling `dbos/`, `prefect/`; extras `temporal`/`dbos`/`prefect` | `pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:112-150`; `pydantic_ai_slim/pyproject.toml:155-159` |
| Web frontend adapters | `AGUIAdapter(UIAdapter)` :233 and `VercelAIAdapter(UIAdapter)` :145 convert agent streams to AG-UI / Vercel AI protocols; `dispatch_request` returns a Starlette streaming response | `pydantic_ai_slim/pydantic_ai/ui/ag_ui/_adapter.py:233`; `pydantic_ai_slim/pydantic_ai/ui/vercel_ai/_adapter.py:145`; adapter ABC `pydantic_ai_slim/pydantic_ai/ui/_adapter.py:208`; docs `docs/ui/ag-ui.md:36-37` |
| Hosted demo UI app | `create_web_app(...) -> Starlette` serves a chat UI for a given agent | `pydantic_ai_slim/pydantic_ai/ui/_web/app.py:165-236` |
| Realtime embedding | `RealtimeSession` class for voice/realtime provider sessions | `pydantic_ai_slim/pydantic_ai/realtime/_session.py:467` |
| CLI subprocess mode | Console script `clai = "clai:cli"`; implementation `def cli()` | `clai/pyproject.toml:57-58`; `clai/clai/__init__.py:9` |
| Server embedding example | FastAPI chat app example: `fastapi.FastAPI(lifespan=...)` hosting an agent with streaming responses | `examples/pydantic_ai_examples/chat_app.py:22-54` |
| Serializable agent spec | `AgentSpec(BaseModel)` + `Agent.from_spec(deps_type=...)` construction path for config-driven embedding | `pydantic_ai_slim/pydantic_ai/agent/spec.py:33`; `pydantic_ai_slim/pydantic_ai/agent/__init__.py:797-913` |
| Agent-as-wrapper reuse | `WrapperAgent` base enabling hosts/third parties to wrap agents transparently (used by `TemporalAgent`) | `pydantic_ai_slim/pydantic_ai/agent/wrapper.py:52` |
| HTTP client ownership | `create_async_httpx2_client` factory with default timeouts; callers own clients, legacy-client usage warns | `pydantic_ai_slim/pydantic_ai/_http.py:54-96` |

## Answers to Dimension Questions

**1. Can the harness run inside another application without owning the whole process?**
Yes — this is the primary mode. The `Agent` is a plain constructible object (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:534-555`) with four run entry points callable from any event loop or thread (`run`/`iter`/`run_stream` async, `run_sync` sync; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:470,666,826,1536`). There is no server to start, no daemon, and no required registration step. For service deployments, the host's own framework owns the process: the FastAPI example embeds agents inside `fastapi.FastAPI(lifespan=...)` (`examples/pydantic_ai_examples/chat_app.py:48-54`), and `AGUIAdapter.dispatch_request` plugs into an existing Starlette/FastAPI endpoint rather than mounting its own app (`docs/ui/ag-ui.md:37`). The only optional packaged server is a demo chat UI via `create_web_app` (`pydantic_ai_slim/pydantic_ai/ui/_web/app.py:165`).

**2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?**
Yes, each with a concrete mechanism:
- *Tools*: arbitrary `AbstractToolset` subclasses implementing `get_tools`/`call_tool` (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:166-171`), plus `ExternalToolset` where the host executes tool calls itself (`pydantic_ai_slim/pydantic_ai/toolsets/external.py:15`).
- *Policy*: the capability system intercepts runs and event streams (`pydantic_ai_slim/pydantic_ai/capabilities/abstract.py:162`); runtime budgets via `UsageLimits` (`pydantic_ai_slim/pydantic_ai/usage.py:418`); concurrency/backpressure limits via `max_concurrency` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:613-617`).
- *Identity/state*: typed `deps_type` DI delivered as `RunContext.deps` (`pydantic_ai_slim/pydantic_ai/_run_context.py:64`) carries whatever identity/storage objects the host defines; message history is supplied per run and transformed via host-provided history processors.
- *Telemetry*: `InstrumentationSettings(tracer_provider=..., meter_provider=...)` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:63`).
- *Secrets*: providers resolve API keys explicitly or from env, surfacing `UserError` when missing (`pydantic_ai_slim/pydantic_ai/providers/__init__.py:32`); hosts can bypass inference entirely by injecting `Model`/`Provider` objects (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1529-1532`).

**3. Are lifecycle, cancellation, shutdown, and error propagation explicit?**
Yes. Cancellation uses a thread-safe, attachable `CancellationToken` passed per run (`pydantic_ai_slim/pydantic_ai/_cancel.py:42-61`); first-party cancels translate into `RunCancelled` carrying a full state snapshot (`all_messages()`, `usage`, `run_id`; `pydantic_ai_slim/pydantic_ai/exceptions.py:268-444`), while external `Task.cancel()` still wins and remains recoverable via `RunCancelled.from_cancellation` (`pydantic_ai_slim/pydantic_ai/exceptions.py:322`). Errors are a two-tier taxonomy — developer mistakes raise `UserError` (`pydantic_ai_slim/pydantic_ai/exceptions.py:229`), operational failures raise `AgentRunError` subclasses including `ModelHTTPError` with parsed `status_code`/`retry_after` (`pydantic_ai_slim/pydantic_ai/exceptions.py:525`). Shutdown is cooperative and scoped: toolsets expose `__aenter__`/`__aexit__` (`pydantic_ai_slim/pydantic_ai/toolsets/abstract.py:130-137`) so resource lifetime follows Python async-context conventions owned by the host.

**4. Does the integration model work for both local-first and service deployments?**
Yes. Local-first: sync/async library calls, CLI (`clai/pyproject.toml:57-58`), Gradio/HTML examples (`examples/pydantic_ai_examples/weather_agent_gradio.py`, `chat_app.html`). Service: durable-execution wrappers replay agents inside workflow engines with activity configuration (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:112-150`), web protocol adapters stream to browser frontends (`pydantic_ai_slim/pydantic_ai/ui/ag_ui/_adapter.py:233`), and optional extras keep heavy dependencies out of slim embeddings (`pydantic_ai_slim/pyproject.toml:144-159`). One caveat: the strongest service-side conveniences (`dispatch_request`) assume Starlette-compatible request/response objects (`docs/ui/ag-ui.md:37`); Django/Flask hosts must use the lower-level `run_stream()` path (`docs/ui/ag-ui.md:36`).

## Architectural Decisions

- **Library-first with layered surfaces**: the same `Agent` powers plain function calls, CLI, durable workers, and web adapters — `TemporalAgent` wraps an existing agent via `WrapperAgent` instead of requiring a separate authoring model (`pydantic_ai_slim/pydantic_ai/durable_exec/temporal/_agent.py:112`; `pydantic_ai_slim/pydantic_ai/agent/wrapper.py:52`).
- **Typed dependency injection over service locator**: `deps_type` parameterizes the agent generically so tool signatures get static checks (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:567-570`); the repo guideline mandates avoiding `Any` (`pydantic_ai_slim/pydantic_ai/AGENTS.md`, rule:60).
- **Decorator-free composition via capabilities**: cross-cutting behavior (instrumentation, content filter, history processing, deferred handling) is implemented as `AbstractCapability` implementations in `pydantic_ai_slim/pydantic_ai/capabilities/` (e.g., `instrumentation.py:67`, `process_event_stream.py`), composed in `CombinedCapability` (`pydantic_ai_slim/pydantic_ai/capabilities/combined.py:47`) — hosts extend without subclassing `Agent`.
- **Wrapper-based toolset algebra**: `WrapperToolset`, `PrefixedToolset`, `FilteredToolset`, `ApprovalRequiredToolset` etc. compose features onto any toolset (`pydantic_ai_slim/pydantic_ai/toolsets/wrapper.py:15`; export list `pydantic_ai_slim/pydantic_ai/__init__.py:315-331`).
- **Explicit opt-in instrumentation with stable schemas**: `InstrumentationSettings.version` pins span attribute shapes across releases (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:63`; naming logic `pydantic_ai_slim/pydantic_ai/_instrumentation.py:657-707`), protecting host dashboards from churn.
- **Serializable configuration**: `AgentSpec(BaseModel)` enables config-driven/remote agent definitions (`pydantic_ai_slim/pydantic_ai/agent/spec.py:33`).

## Notable Patterns

- **State-snapshotting failures**: `RunCancelled` is not just a signal but a data carrier — hosts can persist partial conversation state after cancellation (`pydantic_ai_slim/pydantic_ai/exceptions.py:364-444`).
- **Per-agent ContextVar overrides**: `override()` swaps model/deps/toolsets within a scope using instance-owned ContextVars, enabling tests and multi-tenant routing without global registries (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:696-708`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:1661`).
- **Backpressured event iteration**: `AgentRunEvents` is a hand-written async iterator with a zero-buffer memory stream and lazy background start, exposing `cancel()`/state accessors to consumers (`pydantic_ai_slim/pydantic_ai/agent/abstract.py:136-308`).
- **Protocol adapters as thin translation layers**: `UIAdapter[RunInputT, MessageT, EventT, ...]` normalizes frontend protocols to agent runs and back (`pydantic_ai_slim/pydantic_ai/ui/_adapter.py:208`), keeping protocol drift out of the core loop.
- **Env-check deferral for testability**: `defer_model_check` documents that constructing a named model eagerly validates env vars, and offers deferral (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:593-597`).

## Tradeoffs

- **OTel-only observability**: there is no `logging.getLogger` anywhere in the library (searched all `pydantic_ai_slim/pydantic_ai/**` modules); hosts without an OTel pipeline lose progress visibility except through event handlers they wire themselves (`EventStreamHandler`, `pydantic_ai_slim/pydantic_ai/agent/abstract.py:83`).
- **Starlette affinity in web adapters**: `dispatch_request` convenience assumes Starlette types; non-Starlette frameworks must do manual adapter wiring (`docs/ui/ag-ui.md:36-37`).
- **Two-channel message visibility**: the clean path is return values/event streams, but the escape hatch `capture_run_messages` is a module-level ContextVar where only the first run in a context populates the buffer (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:2543-2559`) — implicit and easy to misuse under nesting.
- **Optional-extras fragmentation**: embedding with Temporal/DBOS/Prefect/UI adapters requires picking the right extra group (`pydantic_ai_slim/pyproject.toml:144-159`); wrong picks fail at import time rather than install time.
- **Python-version-dependent cancellation fidelity**: distinguishing first-party vs external cancellation relies on `uncancel()` bookkeeping unavailable before 3.11, degrading to first-party-wins on 3.10 (`pydantic_ai_slim/pydantic_ai/_cancel.py:227-230`).

## Failure Modes / Edge Cases

- **Undrained streams**: abandoning streamed iterators leaves pending events; the library models this explicitly (`aclose_events` on `AgentStream`, `pydantic_ai_slim/pydantic_ai/result.py:403`) and historically guarded with `UndrainedPendingMessagesError` (`pydantic_ai_slim/pydantic_ai/exceptions.py:240`, now legacy/no longer raised).
- **Mid-stream interruption visibility**: responses carry a `state` field (`'complete'|'incomplete'|'suspended'|'interrupted'`, `pydantic_ai_slim/pydantic_ai/messages.py:126,2605`) so hosts can detect truncated generations rather than treating them as final.
- **Approval flow requires stateful resume**: after a run ends with deferred requests, the host must persist `DeferredToolResults` keyed by `tool_call_id` and pass them to the next run (`pydantic_ai_slim/pydantic_ai/_deferred.py:27-60`; `pydantic_ai_slim/pydantic_ai/agent/abstract.py:476`) — dropping them silently stalls the conversation.
- **Eager env-var reads on named models**: constructing `Agent('openai:gpt-...')` without `defer_model_check` validates provider env at construction time (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:593-597`), which can break config-driven apps that set env later.
- **Telemetry errors are swallowed by design**: e.g., binary-content redaction falls back to a type name rather than failing the run (`pydantic_ai_slim/pydantic_ai/_instrumentation.py:162-167`) — good for resilience, but hosts should not rely on telemetry completeness for correctness.

## Future Considerations

- Add an optional standard-library logging facade alongside OTel for hosts without tracing infrastructure.
- Reconsider the `capture_run_messages` module-level ContextVar in favor of run-scoped message accessors already present on `RunCancelled`/`AgentRunResult` snapshots (`pydantic_ai_slim/pydantic_ai/exceptions.py:364`; `pydantic_ai_slim/pydantic_ai/run.py:636`).
- Document a canonical multi-tenant recipe (deps + `ConcurrencyLimiter` sharing across agents, `pydantic_ai_slim/pydantic_ai/concurrency.py`) since the primitives exist but the end-to-end pattern is spread across modules.
- Extend `dispatch_request`-style conveniences beyond Starlette-typed frameworks or document an ASGI-passthrough variant.

## Questions / Gaps

- No evidence was found for a hosted/SaaS deployment mode operated by the project itself; all deployment modes are host-owned (searched `docs/`, `examples/`, packaging scripts). This appears intentional per the library philosophy statement in the root `AGENTS.md`.
- Whether `pydantic_graph` node-level stepping is recommended for external hosts could not be confirmed from public docs read during this analysis; the graph builder is internal-facing (`pydantic_ai_slim/pydantic_ai/_agent_graph.py:1477` builds the graph from the agent), while `pydantic_graph/` is exported as a separate package.
- Test coverage specifically exercising embedding into third-party servers (beyond `tests/` usage of `TestModel` and the FastAPI example) was not individually enumerated; the claim of maturity rests on the breadth of integration surfaces cited above rather than an exhaustive test inventory.

---

Generated by dimension `24.04-embedding-and-host-integration-ergonomics` against `pydantic-ai`.
