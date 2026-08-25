# Source Analysis: openai-agents-sdk

## Dimension 21.02: Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python 3.10+, async (`asyncio`, `anyio`), OpenAI Python client, pydantic v2 |
| Analyzed | 2026-08-25 |

## Summary

The SDK is organized around small, explicit abstract interfaces at every backend boundary: LLM access (`Model`, `ModelProvider`), tracing sinks (`TracingProcessor`, `TracingExporter`, `TraceProvider`), conversation storage (`Session` protocol / `SessionABC`), sandbox execution (`BaseSandboxClient`/`BaseSandboxSession` with a type-discriminated options registry), MCP transport (`MCPServer`), voice (`VoiceModelProvider`), and realtime audio (`RealtimeModel`). Each abstraction ships multiple first-party implementations (e.g., two core sandbox backends plus seven hosted extensions; Responses, Chat Completions, and WebSocket model surfaces; SQLite and server-managed conversation sessions). Backend selection happens at run scope via `RunConfig.model_provider` / `RunConfig.sandbox.client` or via global registration hooks for tracing, and configuration is externalized through constructor parameters plus environment variables (`OPENAI_BASE_URL`, `OPENAI_DEFAULT_MODEL`, tracing keys). Selection is programmatic rather than declarative (no config-file-driven backend table), which keeps the adapters strongly typed but means swapping backends is a code change, not purely a settings change. Adapter behavior is heavily tested, including routing, shutdown/failure semantics, and JSON round-trips of polymorphic options.

## Rating

**9 / 10** — Mature, durable, observable, and extensible. Clear ABCs/protocols exist for every backend family with multiple interchangeable implementations, runtime selection per run, documented extension recipes, and operational safeguards (error-aggregating `aclose`, shutdown deadlines, backend-ID mismatch guards on resume, batch export retry/backoff). It misses a perfect score only because selection is code-first (no single declarative config surface) and some registries are process-global singletons.

## Evidence Collected

Every entry includes a file path with line numbers, relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model abstraction | `Model` ABC with `get_response`/`stream_response`, resource lifecycle (`close`, `_cleanup_on_run_end`) and provider-specific retry hook (`get_retry_advice`) | `src/agents/models/interface.py:37-65` |
| Provider abstraction | `ModelProvider` ABC resolving models by name, with `aclose()` lifecycle | `src/agents/models/interface.py:138-161` |
| Prefix-routing provider | `MultiProvider` maps `prefix → ModelProvider`; default routes `openai/`|bare → OpenAI, `litellm/`, `any-llm/` fallbacks; `openai_prefix_mode`/`unknown_prefix_mode` knobs | `src/agents/models/multi_provider.py:62-74`, `src/agents/models/multi_provider.py:199-225` |
| Mutable provider map | `MultiProviderMap` with `add_provider`/`remove_provider`/`set_mapping` for runtime route edits | `src/agents/models/multi_provider.py:18-59` |
| LiteLLM adapter | `LitellmProvider(ModelProvider)` in optional extensions package | `src/agents/extensions/models/litellm_provider.py:9-23` |
| any-llm adapter | `AnyLLMProvider(ModelProvider)` with api_key/base_url/api-surface options | `src/agents/extensions/models/any_llm_provider.py:11-36` |
| Wire-protocol backends | `OpenAIProvider.get_model` returns Chat Completions vs Responses vs WebSocket models based on config flags | `src/agents/models/openai_provider.py:243-291` |
| Externalized client config | Lazy client construction honoring `OPENAI_BASE_URL` and `OPENAI_WEBSOCKET_BASE_URL` env vars | `src/agents/models/openai_provider.py:138-178` |
| Default model via env | `get_default_model()` reads `OPENAI_DEFAULT_MODEL` | `src/agents/models/default_models.py:99-103` |
| Run-scope injection | `RunConfig.model: str \| Model` and `model_provider: ModelProvider = field(default_factory=MultiProvider)` | `src/agents/run_config.py:353-359` |
| Runtime resolution order | `get_model()`: explicit `Model` instance > string name via provider > agent-level model | `src/agents/run_internal/turn_preparation.py:134-143` |
| Global client/key setters | `set_default_openai_key/client`, `set_use_responses_by_default`, transport setter | `src/agents/models/_openai_shared.py:18-68` |
| Tracing processor SPI | `TracingProcessor` ABC (on_trace/span start/end, shutdown, force_flush); docstring shows custom sink recipe | `src/agents/tracing/processor_interface.py:9-129` |
| Tracing exporter SPI | `TracingExporter` ABC with single `export(items)` method | `src/agents/tracing/processor_interface.py:132-142` |
| Swappable trace provider | `TraceProvider` ABC + `DefaultTraceProvider`; global replacement via `set_trace_provider` with atexit shutdown | `src/agents/tracing/provider.py:222-297`, `src/agents/tracing/setup.py:27-66` |
| Fan-out multiplexer | `SynchronousMultiTracingProcessor` forwards events to N registered processors, isolating exceptions | `src/agents/tracing/provider.py:93-219` |
| Built-in exporters | `ConsoleSpanExporter` and `BackendSpanExporter` (endpoint/api-key/org/project injectable, default OpenAI ingest endpoint) | `src/agents/tracing/processors.py:27-41`, `src/agents/tracing/processors.py:44-116` |
| Batched sink | `BatchTraceProcessor(exporter)` decouples processing from export; exporter failure is contained | `src/agents/tracing/processors.py:541-720` |
| Public tracing controls | `add_trace_processor`, `set_trace_processors`, `set_tracing_disabled`, `set_tracing_export_api_key`, `flush_traces` | `src/agents/tracing/__init__.py:94-122` |
| Session abstraction | `Session` runtime-checkable Protocol + internal `SessionABC`; capability probe protocol for compaction | `src/agents/memory/session.py:15-56`, `src/agents/memory/session.py:133-152` |
| SQLite store | `SQLiteSession(SessionABC)` with `:memory:` default and file-path persistence | `src/agents/memory/sqlite_session.py:42-80` |
| Server-managed store | `OpenAIConversationsSession(SessionABC)` backed by the Conversations API | `src/agents/memory/openai_conversations_session.py:15-49` |
| Community stores | Extension sessions: Redis, SQLAlchemy, MongoDB, Dapr, encrypted, advanced/async SQLite | `src/agents/extensions/memory/redis_session.py:1`, `src/agents/extensions/memory/sqlalchemy_session.py:1`, `src/agents/extensions/memory/dapr_session.py:1` |
| Sandbox backend SPI | `BaseSandboxClient` ABC with `backend_id` and `supports_default_options` | `src/agents/sandbox/session/sandbox_client.py:109-119` |
| Polymorphic options registry | `BaseSandboxClientOptions`: `type` discriminator auto-registers subclasses; `parse()` resolves dict payloads to concrete options class | `src/agents/sandbox/session/sandbox_client.py:29-106` |
| Core sandbox backends | `DockerSandboxClient(backend_id="docker")`, `UnixLocalSandboxClient(backend_id="unix_local")`; Docker import kept optional | `src/agents/sandbox/sandboxes/docker.py:1501-1502`, `src/agents/sandbox/sandboxes/unix_local.py:1092-1093`, `src/agents/sandbox/sandboxes/__init__.py:30-41` |
| Hosted sandbox backends | Extension clients with distinct `backend_id`s: e2b, modal, daytona, vercel, blaxel, runloop, cloudflare | `src/agents/extensions/sandbox/e2b/sandbox.py:1675`, `src/agents/extensions/sandbox/modal/sandbox.py:2037`, `src/agents/extensions/sandbox/daytona/sandbox.py:1185`, `src/agents/extensions/sandbox/vercel/sandbox.py:1519`, `src/agents/extensions/sandbox/blaxel/sandbox.py:1045`, `src/agents/extensions/sandbox/runloop/sandbox.py:1547`, `src/agents/extensions/sandbox/cloudflare/sandbox.py:1548` |
| Sandbox run-scope injection | `SandboxRunConfig.client/options/session/session_state` fields; dict options coerced against the registered options class for the selected `backend_id` | `src/agents/run_config.py:218-315` |
| Explicit-client requirement | Runtime raises if neither `sandbox.client` nor a live session is provided (no hidden default backend) | `src/agents/sandbox/runtime_session_manager.py:492-499` |
| Cross-backend resume guard | Resume validates stored `backend_id` matches configured client, else `ValueError` | `src/agents/sandbox/runtime_session_manager.py:444-449` |
| MCP transport SPI | `MCPServer` ABC with Stdio/SSE/StreamableHttp implementations | `src/agents/mcp/server.py:542`, `src/agents/mcp/server.py:1869`, `src/agents/mcp/server.py:2002`, `src/agents/mcp/server.py:2163` |
| Voice provider SPI | `VoiceModelProvider` ABC; `OpenAIVoiceModelProvider` default injected into `VoicePipelineConfig` | `src/agents/voice/model.py:200`, `src/agents/voice/models/openai_model_provider.py:35`, `src/agents/voice/pipeline_config.py:17` |
| Realtime model SPI | `RealtimeModel` ABC for realtime transports | `src/agents/realtime/model.py:151` |
| Shipped scripted doubles | Public testing fakes implementing the same SPIs: `ScriptedModel(Model)`, `ScriptedSandboxSession(BaseSandboxSession)` | `src/agents/testing/model.py:249`, `src/agents/testing/sandbox.py:301` |
| Routing tests | 16+ tests over MultiProvider prefix routing, alias/model-id modes, unknown-prefix policy, aclose failure aggregation | `tests/models/test_map.py:20-268` |
| Options round-trip tests | Type-registry parse, round-trip preserving discriminator, duplicate-type rejection, unknown-type rejection | `tests/sandbox/test_client_options.py:16-121` |
| Trace pipeline tests | Batch processor queue-full, force_flush waits, shutdown deadline propagation, exporter exception survival | `tests/test_trace_processor.py:91-291` |
| Provider option validation tests | Explicit client conflicts rejected; explicit options override default client | `tests/models/test_openai_provider_client_options.py:16-73` |

## Answers to Dimension Questions

1. **Are backends swappable?** Yes, uniformly. Every backend family is behind an ABC or Protocol (`src/agents/models/interface.py:37`, `src/agents/models/interface.py:138`; `src/agents/memory/session.py:15-16`; `src/agents/sandbox/session/sandbox_client.py:109`; `src/agents/tracing/provider.py:222`; `src/agents/mcp/server.py:542`), and injection points exist at run scope (`RunConfig.model_provider` at `src/agents/run_config.py:358`, `RunConfig.sandbox.client` at `src/agents/run_config.py:222`) or globally for tracing (`src/agents/tracing/setup.py:27-36`). A `Model` instance can also be passed directly per agent or per run (`src/agents/run_internal/turn_preparation.py:136-141`).

2. **Which backends have multiple implementations?**
   - Models: OpenAI Responses HTTP, Responses WebSocket, Chat Completions (`src/agents/models/openai_provider.py:267-291`), plus LiteLLM and any-llm adapters (`src/agents/extensions/models/litellm_provider.py:9`, `src/agents/extensions/models/any_llm_provider.py:11`).
   - Session stores: SQLite (`src/agents/memory/sqlite_session.py:42`), OpenAI Conversations (`src/agents/memory/openai_conversations_session.py:27`), plus six extension stores under `src/agents/extensions/memory/`.
   - Sandboxes: unix_local and docker in core (`src/agents/sandbox/sandboxes/unix_local.py:1092`, `src/agents/sandbox/sandboxes/docker.py:1501`), seven hosted providers in extensions (see evidence table).
   - MCP transports: stdio, SSE, streamable HTTP (`src/agents/mcp/server.py:1869,2002,2163`).
   - Tracing sinks: console exporter and OpenAI backend exporter (`src/agents/tracing/processors.py:27,44`) plus user-supplied processors/exporters.
   - Voice/realtime: default OpenAI implementations of `VoiceModelProvider` (`src/agents/voice/models/openai_model_provider.py:35`) and `RealtimeModel` (`src/agents/realtime/model.py:151`).

3. **Can backends be swapped at runtime?** Yes within a process, before/at run construction. `MultiProviderMap.add_provider/remove_provider/set_mapping` mutate live prefix routing (`src/agents/models/multi_provider.py:32-59`); `set_trace_processors` replaces the processor list (`src/agents/tracing/provider.py:110-115`); `set_trace_provider` swaps the whole tracing provider (`src/agents/tracing/setup.py:27-36`). Per-run isolation is guaranteed because each `Runner.run` resolves the model from its own `RunConfig` (`src/agents/run_internal/turn_preparation.py:134-143`). There is no mid-run hot-swap of a session's model or sandbox client; sandbox resumes additionally enforce backend identity (`src/agents/sandbox/runtime_session_manager.py:444-449`), which is a deliberate safety constraint rather than a limitation.

4. **Are adapter implementations tested?** Yes, extensively and at the right seams: prefix routing and close semantics (`tests/models/test_map.py:20-268`), provider client-option conflict rules (`tests/models/test_openai_provider_client_options.py:16-73`), sandbox options type registry incl. duplicate/unknown rejection and JSON round-trips (`tests/sandbox/test_client_options.py:16-121`), batch tracing under queue pressure, flush, shutdown deadlines, and exporter faults (`tests/test_trace_processor.py:91-291`), plus Docker backend suites (`tests/sandbox/test_docker.py`). The SDK also publishes first-party scripted fakes conforming to the production SPIs (`src/agents/testing/model.py:249`, `src/agents/testing/sandbox.py:301`), and repo policy directs contributors to prefer them (AGENTS.md, "Testing & Automated Checks").

## Architectural Decisions

1. **Two-layer model indirection (name → provider → model).** Agents hold either a `Model` instance or a string; strings resolve through an injectable `ModelProvider` at run time (`src/agents/run_internal/turn_preparation.py:136-143`). This lets one codebase target OpenAI, LiteLLM, or any-llm without touching agent definitions.

2. **Prefix-based multi-provider routing with ambiguity controls.** `MultiProvider` defaults to historical aliasing but exposes `openai_prefix_mode` and `unknown_prefix_mode` so OpenAI-compatible endpoints can receive literal namespaced IDs (`src/agents/models/multi_provider.py:69-74`, `199-225`); explicit map entries always win (`src/agents/models/multi_provider.py:206-212`).

3. **Protocol-first storage contract.** `Session` is a `runtime_checkable` Protocol so third parties implement structurally, while `SessionABC` serves internal concrete classes; capability extensions (compaction, context wrapper opt-in) are probed duck-typed via helper predicates instead of bloating the base interface (`src/agents/memory/session.py:59-67`, `142-196`).

4. **Type-discriminated polymorphic config for sandboxes.** Options classes self-register on subclassing keyed by their `type` literal, enabling dict→class coercion and serialized-state restoration across processes (`src/agents/sandbox/session/sandbox_client.py:53-106`); duplicates and missing discriminators fail loudly (`tests/sandbox/test_client_options.py:114-121`).

5. **Lazy everything at import time.** The OpenAI client is constructed only on first use to avoid API-key errors when unused (`src/agents/models/openai_provider.py:136-137`), the global trace provider initializes on first access (`src/agents/tracing/setup.py:39-60`), and litellm/any-llm providers import lazily inside the fallback path (`src/agents/models/multi_provider.py:164-174`). Optional extras never break top-level imports (mirrored for Docker at `src/agents/sandbox/sandboxes/__init__.py:30-41`).

6. **No implicit sandbox default.** The runtime refuses to guess a backend and demands an explicit client or live session (`src/agents/sandbox/runtime_session_manager.py:492-499`) — a security-motivated choice given sandbox escape risk.

## Notable Patterns

- **Adapter factory via registry**: `BaseSandboxClientOptions.parse(payload)` resolves `"type"` strings to concrete options classes through `_subclass_registry` (`src/agents/sandbox/session/sandbox_client.py:76-106`); the same pattern recurs for manifest entries and mount strategies (`src/agents/sandbox/entries/base.py:103-134`, `src/agents/sandbox/entries/mounts/base.py:134-152`).
- **Composite/fan-out**: `SynchronousMultiTracingProcessor` broadcasts to all registered processors while catching per-processor exceptions so one bad sink cannot kill a run (`src/agents/tracing/provider.py:117-175`).
- **Decorator-style resilience wrappers**: `MultiProvider.aclose` aggregates child-provider failures, preserves the first error, and re-raises `CancelledError` untouched (`src/agents/models/multi_provider.py:254-279`).
- **Loop-affine resource caching**: websocket-backed models are cached per event loop with weak references and cross-loop-aware closing (`src/agents/models/openai_provider.py:125-129`, `186-208`).
- **Env-var fallback chain**: constructor arg → module-level setter → environment variable for keys, base URLs, default model, and tracing credentials (`src/agents/models/openai_provider.py:150-176`, `src/agents/models/default_models.py:99-103`, `src/agents/tracing/processors.py:106-116`).
- **Shipped test doubles as part of the adapter story**: `ScriptedModel` and `scripted_sandbox_session()` let consumers exercise the exact SPI contracts deterministically (`src/agents/testing/model.py:249`, `src/agents/testing/sandbox.py:573`).

## Tradeoffs

- **Code-first vs config-file selection.** Switching Postgres→SQLite-equivalent backends (e.g., SQLiteSession→SQLAlchemySession, or unix_local→Docker) requires editing construction code; env vars cover endpoints/keys/model names only. This yields full type safety and IDE discoverability at the cost of ops-only reconfiguration (docs describe programmatic recipes at `docs/models/index.md:187-224`).
- **Global singletons for tracing and shared clients.** `GLOBAL_TRACE_PROVIDER` (`src/agents/tracing/setup.py:11`), `_default_openai_client` (`src/agents/models/_openai_shared.py:10`), and the process-wide shared httpx client (`src/agents/models/openai_provider.py:31-42`) simplify wiring but make per-process multi-tenant configurations awkward.
- **Default tracing sink is vendor-coupled.** `BackendSpanExporter` posts to the OpenAI ingest endpoint (`src/agents/tracing/processors.py:45`); third-party backends integrate as custom processors/exporters rather than prebuilt adapters, so users write glue code (though the interface is minimal — `src/agents/tracing/processor_interface.py:132-142`).
- **Extension adapters intentionally thin.** `LitellmProvider.get_model` constructs a bare `LitellmModel` and its docstring tells advanced users to copy-paste the class (`src/agents/extensions/models/litellm_provider.py:16-19`) — low maintenance burden, but limited configurability without forking.
- **Prefix routing heuristics carry legacy weight.** The alias-vs-model-id ambiguity for `openai/…` names required two compatibility knobs and careful documentation (`docs/models/index.md:189-222`), showing the cost of evolving a routing scheme without breaking released behavior.

## Failure Modes / Edge Cases

- **Unknown model prefix fails fast**: `UserError("Unknown prefix: ...")` unless `unknown_prefix_mode="model_id"` opts into pass-through (`src/agents/models/multi_provider.py:222-225`).
- **Misconfigured provider constructors raise immediately**: supplying both `openai_client` and credential fields raises `UserError` (`src/agents/models/openai_provider.py:90-98`); invalid prefix modes validated eagerly (`src/agents/models/multi_provider.py:176-188`).
- **Shutdown under pressure**: trace processor shutdown honors deadlines and skips stragglers with a non-fatal warning (`src/agents/tracing/provider.py:177-204`); batch export survives exporter exceptions and logs `[non-fatal]` (`src/agents/tracing/processors.py:696-713`), verified by tests (`tests/test_trace_processor.py:192-291`).
- **Resume across mismatched sandbox backends is blocked**, preventing state corruption from applying e.g. Docker session state onto unix_local (`src/agents/sandbox/runtime_session_manager.py:444-449`).
- **Missing API key degrades quietly where safe**: trace export is skipped with a warning when no key is present (`src/agents/tracing/processors.py:131-134`), while model calls defer the key error until first use (`src/agents/models/openai_provider.py:136-137`).
- **Duplicate type registrations are compile-time-ish errors**: re-registering a `type` discriminator raises `TypeError` (`src/agents/sandbox/session/sandbox_client.py:62-74`).

## Future Considerations

- A declarative backend-selection layer (config object mapping prefixes/backend IDs to factories) would let operators swap stores/sandboxes without code changes; the `MultiProviderMap` and options registries already provide most of the machinery.
- Prebuilt exporters for common observability stacks (OTel, Langfuse-style sinks) would reduce the custom-processor boilerplate currently expected of users; today the docs point to community integrations rather than in-repo adapters.
- The per-loop websocket model cache and global HTTP client could be scoped per provider instance to improve multi-tenant isolation (`src/agents/models/openai_provider.py:31-42`, `127-129`).

## Questions / Gaps

- No vector-DB or retrieval-store abstraction was found in this source (searches over `src/agents/**` surfaced file-search tool usage of the Responses API rather than a swappable local vector store interface); retrieval appears delegated to OpenAI platform tools.
- Queue backends are absent entirely — work distribution is out of scope for the SDK (no evidence found in `src/agents/`; searches for queue/broker abstractions returned nothing relevant).
- Whether the seven hosted sandbox extension clients share behavioral parity with core backends (snapshot/resume semantics) could not be fully verified from unit tests alone; integration coverage lives under `tests/sandbox/integration_tests/` and requires service credentials, so parity claims rest primarily on the shared `BaseSandboxClient`/`BaseSandboxSession` contracts (`src/agents/sandbox/session/sandbox_client.py:109`).

---

Generated by `dimensions/21.02-provider-backend-adapters.md` against `openai-agents-sdk`.
