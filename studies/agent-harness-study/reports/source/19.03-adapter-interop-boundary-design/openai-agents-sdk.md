# Source Analysis: openai-agents-sdk

## Adapter and Interop Boundary Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (>=3.10) / `openai>=3.0`, `pydantic`, `mcp`, `any-llm`, `litellm` optional |
| Analyzed | 2026-08-27 |

## Summary

The SDK treats interop as a first-class layered design: narrow `abc.ABC` / `Protocol` contracts live in core (`Model`/`ModelProvider`, `MCPServer`, `Session`, `TracingProcessor`, `VoiceModelProvider`) while all wire-specific behavior is pushed into adapters under `src/agents/models/`, `src/agents/extensions/models/`, `src/agents/mcp/server.py`, `src/agents/memory/`, and `src/agents/sandbox/`. Adapters are injectable at runtime via `RunConfig.model_provider`, `Agent.mcp_servers`, `Session` per-run, and `set_trace_provider` — no core patches are required to add a compliant implementation. Third-party routing (LiteLLM, Any-LLM) is exposed as optional-prefix adapters through `MultiProvider`/`MultiProviderMap`, but introduces residual core coupling via hard-coded fallback prefixes. Conformance validation is adapter-local rather than harness-driven, and interop boundaries are extensively documented in `docs/models/index.md` and `docs/mcp.md`, with additional operational safeguards (lifecycle timeouts, credential-safe error redaction, retry advice).

## Rating

**8 / 10 — Clear model with tests, explicit interfaces, and operational safeguards**

Rationale: Protocols are core abstractions with stable typing and dependency-inversion injection; adapters are runtime-swappable and lifecycle-managed; documentation is explicit. Deductions for (a) no single conformance harness that any `Model` must pass, (b) `MultiProvider` hard-codes `litellm`/`any-llm` fallback knowledge, and (c) several failure-mode nuances live only in adapter code/comments rather than a central interop spec.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core protocol abstraction — Model | `class Model(abc.ABC)` with `get_response` + `stream_response` abstract methods and retry/close hooks | `src/agents/models/interface.py:37` |
| Core protocol abstraction — ModelProvider | `class ModelProvider(abc.ABC)` with `get_model` + `aclose` | `src/agents/models/interface.py:138` |
| Core protocol abstraction — ModelTracing | Enum `DISABLED/ENABLED/ENABLED_WITHOUT_DATA` controls adapter tracing data inclusion | `src/agents/models/interface.py:20` |
| Core protocol abstraction — MCP | `class MCPServer(abc.ABC)` abstracting `connect/cleanup/list_tools/call_tool/list_prompts/get_prompt` | `src/agents/mcp/server.py:542` |
| Core protocol abstraction — MCP client-session | `class _MCPServerWithClientSession(MCPServer)` standardizes `ClientSession` + cache/retry/filter logic for all local transports | `src/agents/mcp/server.py:857` |
| Core protocol abstraction — Session | `class Session(Protocol)` + `SessionABC` with `get_items/add_items/pop_item/clear_session` | `src/agents/memory/session.py:16` / `src/agents/memory/session.py:59` |
| Core protocol abstraction — Compaction session | `class OpenAIResponsesCompactionAwareSession(Session, Protocol)` extension point | `src/agents/memory/session.py:134` |
| Core protocol abstraction — Voice | `class VoiceModelProvider(abc.ABC)` / `STTModel` / `TTSModel` / `StreamedTranscriptionSession` | `src/agents/voice/model.py:200` / `src/agents/voice/model.py:147` / `src/agents/voice/model.py:91` / `src/agents/voice/model.py:113` |
| Core protocol abstraction — Tracing | `class TracingProcessor(abc.ABC)` + `TracingExporter` defines `on_trace_start/on_trace_end/on_span_start/on_span_end/shutdown/force_flush` | `src/agents/tracing/processor_interface.py:9` / `src/agents/tracing/processor_interface.py:132` |
| Core protocol abstraction — Sandbox | `class BaseSandboxSession(abc.ABC)` + `SandboxSessionState` + `BaseSandboxClient` for sandboxed tool execution | `src/agents/sandbox/session/base_sandbox_session.py:199` / `src/agents/sandbox/session/sandbox_session_state.py:30` / `src/agents/run_config.py:222` |
| Adapter implementation — OpenAI | `class OpenAIProvider(ModelProvider)` lazy client, `use_responses` / websocket cache, `shared_http_client()` reuse | `src/agents/models/openai_provider.py:45` / `src/agents/models/openai_provider.py:38` / `src/agents/models/openai_provider.py:127` |
| Adapter implementation — OpenAI Responses (HTTP) | `OpenAIResponsesModel` resolved via provider when `use_responses=True` without websocket | `src/agents/models/openai_provider.py:286` |
| Adapter implementation — OpenAI Responses (WS) | `OpenAIResponsesWSModel` per-loop cache `WeakKeyDictionary[AbstractEventLoop,_WSLoopModelCache]` | `src/agents/models/openai_provider.py:127` / `src/agents/models/openai_provider.py:253` |
| Adapter implementation — OpenAI ChatCompletions | `OpenAIChatCompletionsModel` with `strict_feature_validation` and `buffer_streamed_tool_calls` knobs | `src/agents/models/openai_provider.py:268` |
| Adapter implementation — MultiProvider router | `class MultiProvider(ModelProvider)` prefix routing, `ProviderMap` injection, `openai_prefix_mode`/`unknown_prefix_mode` | `src/agents/models/multi_provider.py:62` / `src/agents/models/multi_provider.py:18` |
| Adapter implementation — LiteLLM | `class LitellmProvider(ModelProvider)` + `class LitellmModel(Model)` via `litellm.acompletion` wrapper | `src/agents/extensions/models/litellm_provider.py:9` / `src/agents/extensions/models/litellm_model.py:149` |
| Adapter implementation — Any-LLM | `class AnyLLMProvider(ModelProvider)` + `class AnyLLMModel(Model)` supporting both `responses` and `chat_completions` API selection | `src/agents/extensions/models/any_llm_provider.py:11` / `src/agents/extensions/models/any_llm_model.py:253` |
| Adapter implementation — MCP Stdio | `class MCPServerStdio(_MCPServerWithClientSession)` stdio transport | `src/agents/mcp/server.py:1869` |
| Adapter implementation — MCP SSE | `class MCPServerSse(_MCPServerWithClientSession)` SSE transport (deprecated but supported) | `src/agents/mcp/server.py:2002` |
| Adapter implementation — MCP Streamable HTTP | `class MCPServerStreamableHttp(_MCPServerWithClientSession)` with `httpx2` v2 auth validation and session-id hooks | `src/agents/mcp/server.py:2163` / `src/agents/mcp/server.py:305` |
| Adapter implementation — MCP manager | `class MCPServerManager` lifecycle facade managing connect/reconnect/cleanup across worker tasks | `src/agents/mcp/manager.py:151` |
| Adapter implementation — Sessions | `SQLiteSession`, `AsyncSQLiteSession`, `RedisSession`, `SQLAlchemySession`, `EncryptedSession`, `OpenAIConversationsSession`, `DaprSession`, `MongoDBSession` all extend `SessionABC` | `src/agents/memory/sqlite_session.py:42` / `src/agents/extensions/memory/async_sqlite_session.py:22` / `src/agents/extensions/memory/redis_session.py:357` / `src/agents/extensions/memory/sqlalchemy_session.py:66` / `src/agents/extensions/memory/encrypt_session.py:100` / `src/agents/memory/openai_conversations_session.py:27` / `src/agents/extensions/memory/dapr_session.py:70` / `src/agents/extensions/memory/mongodb_session.py:76` |
| Adapter implementation — Sandbox backends | `DockerSandboxSession`, `UnixLocalSandboxSession`, `E2BSandboxSession`, `ModalSandboxSession`, etc. | `src/agents/sandbox/sandboxes/docker.py:280` / `src/agents/sandbox/sandboxes/unix_local.py:130` / `src/agents/extensions/sandbox/e2b/sandbox.py:701` / `src/agents/extensions/sandbox/modal/sandbox.py:494` |
| Adapter implementation — Voice | `class OpenAIVoiceModelProvider(VoiceModelProvider)` | `src/agents/voice/models/openai_model_provider.py:35` |
| Plugin/extension point — Provider map | `MultiProviderMap.add_provider(prefix, provider)` + `set_mapping/get_provider` allows registration without core edit | `src/agents/models/multi_provider.py:44` / `src/agents/models/multi_provider.py:36` |
| Plugin/extension point — RunConfig injection | `RunConfig.model_provider: ModelProvider = field(default_factory=MultiProvider)` and `RunConfig.model: str|Model|None` | `src/agents/run_config.py:358` / `src/agents/run_config.py:353` |
| Plugin/extension point — Agent-level MCP | `Agent.mcp_servers` + `Agent.mcp_config` (`convert_schemas_to_strict`, `failure_error_function`, `include_server_in_tool_names`) | `src/agents/agent.py:148` (MCPConfig TypedDict) / `docs/mcp.md:60` |
| Plugin/extension point — Runtime session | `Session` is `Protocol` (structural) so third-party sessions need not inherit `SessionABC`; `SessionSettings` override per run | `src/agents/memory/session.py:15` / `src/agents/memory/session_settings.py:31` |
| Plugin/extension point — Tracing | `set_trace_provider(provider: TraceProvider)` / `get_trace_provider()` lazy + atexit shutdown | `src/agents/tracing/setup.py:27` / `src/agents/tracing/setup.py:39` |
| Plugin/extension point — Sandbox | `SandboxRunConfig.client: BaseSandboxClient` + `SandboxRunConfig.session` override | `src/agents/run_config.py:222` |
| Conformance tests — Model adapters | Parameterized AnyLLM tests: chat vs responses routing, retry, usage, stream terminal events, sanitization | `tests/models/test_any_llm_model.py:544` / `tests/models/test_any_llm_model.py:787` / `tests/models/test_any_llm_model.py:1072` |
| Conformance tests — LiteLLM | Tests for reasoning_effort forwarding, streaming, logprobs, tool ordering | `tests/models/test_litellm_extra_body.py` / `tests/models/test_litellm_usage_requests.py` (glob) |
| Conformance tests — OpenAI provider | `test_openai_provider_client_options.py`, `test_map.py` for `MultiProvider` prefix/pass-through | `tests/test_openai_provider_client_options.py` / `tests/models/test_map.py:92` / `tests/models/test_map.py:122` |
| Conformance tests — MCP | 27 MCP test files covering manager lifecycle, auth, v2 HTTP compat, pagination, caching, tool filtering | `tests/mcp/test_mcp_server_manager.py:1` / `tests/mcp/test_mcp_version_compat.py:1` / `tests/mcp/test_tool_filtering.py` (glob) |
| Conformance tests — Negative evidence | No generic `ModelConformanceSuite` asserting all `Model` impls satisfy call-ID, usage, and streaming contracts; docs note keep-real-adapter for wire tests | `docs/testing.md:541` |
| Protocol documentation — Models | Choosing/ mixing models, provider integration table, `MultiProvider` prefix modes, third-party adapter beta notice | `docs/models/index.md:7` / `docs/models/index.md:310` / `docs/models/index.md:682` |
| Protocol documentation — MCP | Transport matrix (Hosted/StreamableHTTP/SSE/stdio), v1/v2 compat, manager behaviors, tool filtering, caching, pagination | `docs/mcp.md:18` / `docs/mcp.md:29` / `docs/mcp.md:367` |
| Operational safeguards — MCP | `_credential_safe_exception_group`, URL sanitization via `get_mcp_server_log_name`, redacted logs | `src/agents/mcp/server.py:249` / `src/agents/mcp/_logging.py:9` / `tests/mcp/test_mcp_server_manager.py:313` |
| Operational safeguards — Lifecycle | `_validate_lifecycle_timeout`, per-loop websocket cache pruning, `retry_backoff_seconds_max` validation | `src/agents/mcp/manager.py:15` / `src/agents/models/openai_provider.py:231` / `src/agents/mcp/server.py:163` |
| Declared dependencies | `mcp>=1.19.0,<3` + optional `litellm` / `any-llm-sdk` / voice `websockets` | `pyproject.toml:16` / `pyproject.toml:39` / `pyproject.toml:40` |

## Answers to Dimension Questions

**1. Are protocols core or adapter-layer?**

Protocols are core abstractions. Six interop boundaries are defined as central contracts in `src/agents/` before any adapter: `Model`/`ModelProvider` (`src/agents/models/interface.py:37`), `MCPServer` (`src/agents/mcp/server.py:542`), `Session` (`src/agents/memory/session.py:16`), `VoiceModelProvider` (`src/agents/voice/model.py:200`), `TracingProcessor` (`src/agents/tracing/processor_interface.py:9`), and `BaseSandboxSession` (`src/agents/sandbox/session/base_sandbox_session.py:199`). Every concrete transport (HTTP/WSS/stdio/SSE, LiteLLM, any-llm, Redis/SQLite/S3/Sandbox backends) is an adapter implementing those interfaces. The package exports reify this layering: `agents/__init__.py:88-97` exposes `Model`, `ModelProvider`, `MultiProvider`, `OpenAIProvider` as public contracts while adapter-specific models stay under `agents.extensions`.

**2. Can adapters be added without core changes?**

Yes for the primary seams, with one qualified exception. Any external class implementing `Model` + `ModelProvider` can be injected via `RunConfig(model_provider=CustomProvider)` (`src/agents/run_config.py:358`) or per-agent `Agent.model` (`docs/models/index.md:319`), and the runner resolves it through `ModelProvider.get_model` without editing core. `Session` is a `@runtime_checkable Protocol` (`src/agents/memory/session.py:15`), so a third-party need not subclass `SessionABC` at all. `MCPServer` is an ABC that custom servers subclass, and `TracingProcessor`/`BaseSandboxClient` are similarly injectable. `MultiProviderMap` (`src/agents/models/multi_provider.py:18`) lets callers register arbitrary `prefix -> ModelProvider` entries at runtime. The qualification is `MultiProvider._create_fallback_provider` (`src/agents/models/multi_provider.py:164`) hard-codes `litellm` and `any-llm` imports — automatic `litellm/model` style routing without an explicit map requires that method; adding a new *implicit* prefix would need a core edit, whereas explicit map registration does not.

**3. Are adapters tested for conformance?**

Partially — adapter-local but not harness-against-contract.

*   **Extensive per-adapter tests exist.** `tests/models/test_any_llm_model.py` alone covers chat vs responses routing (`:544`), parallel tool-calls propagation (`:308`), reasoning sanitization (`:1206`), terminal-event rejection (`:1066`), and provider-wrapper recovery. LiteLLM equivalents live in `tests/models/test_litellm_*` suites. MCP has 27 dedicated files including lifecycle-serialization and credential-redaction tests (`tests/mcp/test_mcp_server_manager.py:522`, `tests/mcp/test_mcp_version_compat.py`). OpenAI provider client options and prefix modes are covered by `tests/test_openai_provider_client_options.py` and `tests/models/test_map.py:92`.

*   **No shared conformance harness was found.** There is no `ModelConformanceSuite` or abstract pytest class that asserts any `Model` implementation respects the call-ID contract (`src/agents/models/interface.py:40`), `Usage` reporting, `ModelTracing` inclusion, or `stream_response` terminal-event ordering. `docs/testing.md:541` explicitly warns *not* to use scripted fakes for wire serialization and to test the real adapter against the network boundary, which implies conformance is checked by duplicating similar assertions per adapter rather than by one canonical suite. Shared behavior is therefore brittle to drift (e.g., LiteLLM’s `_fix_tool_message_ordering` in `src/agents/extensions/models/litellm_model.py:759` and AnyLLM’s equivalent in `src/agents/extensions/models/any_llm_model.py:884` are tested separately).

**4. Are interop boundaries documented?**

Yes, to an unusually thorough level for the two flagship seams.

*   **Models:** `docs/models/index.md:7-708` covers provider-integration table, `ModelProvider` per-run vs global `set_default_openai_client`, `Agent.model` per-agent mixing, `MultiProvider` `openai_prefix_mode`/`unknown_prefix_mode` (`:192-223`), Responses vs ChatCompletions capability matrix, websocket transport, retry policies, and third-party beta disclaimer (`:682`). The code docstrings reinforce this (e.g., `src/agents/models/multi_provider.py:62` explains `openai/` alias semantics).

*   **MCP:** `docs/mcp.md:18-502` documents the transport matrix, v1/v2 compat (`:29`), `MCPServerStdio`/`Sse`/`StreamableHttp` constructors, `MCPServerManager` semantics (`:367`), tool filtering, caching, pagination, tracing, and security warning (`:12`).

*   **Other seams** are lighter: `Session` is documented via the `sessions/` guide and the protocol docstring (`src/agents/memory/session.py:16`); `TracingProcessor` is interface-documented (`src/agents/tracing/processor_interface.py:9`) but lacks a dedicated boundary-risk page; `VoiceModelProvider` and `BaseSandboxSession` rely on docstrings and `docs/sandbox/` rather than a single boundary spec. No central `ARCHITECTURE.md` enumerates all interop versioning or compatibility guarantees in one place.

## Architectural Decisions

*   **Contract-first, adapter-second layering.** Core defines narrow interfaces (`Model.get_response`/`stream_response` at `src/agents/models/interface.py:68`, `MCPServer.connect`/`call_tool` at `src/agents/mcp/server.py:583`) with no provider imports; concrete wire code lives in `src/agents/extensions/` or provider-specific modules. This preserves dependency inversion and matches the rating’s “protocols are core” finding.

*   **Structural typing for sessions, ABC for transports.** Sessions use `@runtime_checkable Protocol` (`src/agents/memory/session.py:15`) to accept external stores without inheritance — maximizing ecosystem extensibility. Transports that need lifecycle guarantees use `abc.ABC` (MCP, Model) so missing methods fail fast at instantiation.

*   **Lazy-instantiated provider clients with shared HTTP pool.** `OpenAIProvider._get_client` (`src/agents/models/openai_provider.py:138`) defers `AsyncOpenAI` creation until first use and reuses `shared_http_client()` (`:38`) — preventing import-time `OPENAI_API_KEY` errors and improving latency via connection-pool sharing.

*   **Prefix-based routing via `MultiProvider` with dual-mode alias.** `MultiProvider._resolve_prefixed_model` (`src/agents/models/multi_provider.py:199`) supports both historical alias stripping (`openai/gpt-4.1 -> gpt-4.1`) and literal pass-through (`openai_prefix_mode="model_id"`). Coupled with `unknown_prefix_mode="model_id"` this lets the same instance talk to OpenAI-compatible gateways that expect literal namespaced model IDs.

*   **Per-loop websocket model caching.** `OpenAIProvider` stores `OpenAIResponsesWSModel` in `WeakKeyDictionary[AbstractEventLoop, ...]` (`src/agents/models/openai_provider.py:127`) and prunes closed loops (`:231`) so that `responses_websocket_session()` can reuse a persistent WSS connection across turns while not leaking across event loops.

*   **MCP local-server unification under `_MCPServerWithClientSession`.** All local MCP transports share `_MCPServerWithClientSession` (`src/agents/mcp/server.py:857`), centralizing tool-cache snapshotting (`_snapshot_tools` at `:131`), `tool_filter` application (`:983`), retry/backoff (`:1230`), and credential-safe error redaction (`:1176`). `MCPServerManager` (`src/agents/mcp/manager.py:151`) then wraps *multiple* servers with per-server worker tasks (`_ServerWorker` `:37`) to preserve AnyIO cancel-scope task affinity (`:117`).

*   **Optional-beta third-party adapters.** LiteLLM and any-llm live under `src/agents/extensions/models/` with optional dependencies (`pyproject.toml:39-40`) and dynamic imports that raise `ImportError` with install hints (`src/agents/extensions/models/litellm_model.py:17`, `src/agents/extensions/models/any_llm_model.py:68`). Docs label them best-effort beta (`docs/models/index.md:682`) and default `MultiProvider` still raises `UserError` on unknown prefixes.

*   **Typed guided errors at boundaries.** Constructors validate via explicit `_validate_*` helpers: `client_session_timeout_seconds` (`src/agents/mcp/server.py:136`), `retry_backoff_seconds_max` (`:163`), `_validate_lifecycle_timeout` (`src/agents/mcp/manager.py:15`), `MultiProvider._validate_openai_prefix_mode` (`src/agents/models/multi_provider.py:177`), and `RunConfig` collision/tools policies (`src/agents/run_config.py:532`). This favors fail-fast over silent misconfiguration.

## Notable Patterns

*   **Provider + Model split (factory + product).** `ModelProvider.get_model(name) -> Model` mirrors abstract-factory; `Model` carries the per-request contract while the provider owns client/caching/retry state.

*   **WeakKeyDictionary event-loop affinity.** WebSocket model instances are keyed by running event loop, preventing cross-loop use after loop close — a loop-scoped multiton.

*   **AsyncExitStack + task-worker for lifecycle serialization.** MCP manager keeps `AsyncExitStack` per server (`src/agents/mcp/server.py:931`) and `_ServerWorker` per server (`src/agents/mcp/manager.py:37`) so `connect`/`cleanup` stay in the same task even when `connect_in_parallel=True`.

*   **Credential-safe error graph.** `_credential_safe_exception_group` / `_safe_transport_cause` (`src/agents/mcp/server.py:249-214`) rebuild `BaseExceptionGroup` with fixed-message descendants and redact URLs via `get_mcp_server_log_name` — an interop-specific security pattern applied consistently to HTTP transports.

*   **Capability probing with fallback.** `AnyLLMModel._supports_responses()` checks `SUPPORTS_RESPONSES` (`src/agents/extensions/models/any_llm_model.py:1160`) and `api` selection defaults to responses when supported else chat. The v2 MCP client probes `server/discover` then falls back to legacy `initialize` (`docs/mcp.md:30`).

*   **Dual-typed input with adapter converters.** `Converter.items_to_messages` and `ChatCmplHelpers` / `OpenAIResponsesConverter` translate SDK `TResponseInputItem` into provider-specific payloads per adapter, keeping the core input type neutral.

*   **Mixin of static + dynamic tool filtering.** `MCPUtil` supports both declarative allow/block lists and async `ToolFilterContext` callbacks (`src/agents/mcp/server.py:983-1067`), giving both static policy and context-aware gating.

## Tradeoffs

*   **Explicit registration vs zero-config fallback.** `MultiProviderMap` gives fine-grained, core-free extensibility, but the built-in `_create_fallback_provider` hard-codes two blessed adapters for convenience. The tradeoff favors discoverability (new users can type `litellm/...` without wiring) at the cost of a closed set of implicit prefixes that needs a core edit to extend.

*   **Structural Protocol flexibility vs weaker static guarantees.** `Session(Protocol)` lets external stores duck-type without inheriting `SessionABC`, maximizing adoption, but loses the exhaustive-check benefit of `ABC` subclass enforcement and makes the `wrapper` opt-in dance (`src/agents/memory/session.py:155-212`) more subtle.

*   **Shared adapter logic duplication vs abstraction leakage.** LiteLLM and AnyLLM both re-implement tool-ordering fixes, reasoning-content extraction, and usage normalization independently. Extracting a shared `ChatCompletionsModelBase` would DRY them, but would also couple two third-party SDKs’ quirks.

*   **Per-loop caching performance vs leak complexity.** WebSocket model reuse cuts connection-setup latency for multi-turn workflows, but adds cross-loop cleanup, `threadsafe` closing, and `WeakKeyDictionary` pruning complexity that plain per-request construction avoids.

*   **Best-effort beta label vs ecosystem demand.** Keeping LiteLLM/any-llm as beta acknowledges upstream variance (provider-specific caveats in `docs/models/index.md:692-701`) while still shipping them by default via `MultiProvider` — trading a stability guarantee for breadth.

*   **Credential-safe redaction vs debuggability.** Transport errors are replaced with fixed messages and no URLs (`src/agents/mcp/server.py:281-284`), which prevents secret leakage but forces operators to enable `DONT_LOG_TOOL_DATA=False` or rely on side-channel logs to diagnose connectivity.

*   **Fail-closed lifecycle timeouts vs liveness.** `MCPServerManager` defaults both timeouts to `10.0s` and rejects `0` (`src/agents/mcp/manager.py:205`) to avoid ambiguous deadlines, but long-running MCP servers on slow networks need explicit `None` tuning.

## Failure Modes / Edge Cases

*   **Unknown model prefix.** With `MultiProvider` defaults, `unknown_prefix_mode="error"` raises `UserError("Unknown prefix: ...")` at `src/agents/models/multi_provider.py:225`; callers pointing at gateways expecting literal IDs must opt into `unknown_prefix_mode="model_id"` or explicit `provider_map`. No evidence found of a connection-time retry that would mask this.

*   **Mismatched HTTP auth types across `mcp` majors.** `_validate_v2_http_auth` (`src/agents/mcp/server.py:330`) and `_validated_v2_http_client_factory` (`:339`) raise `UserError` when `httpx.Auth` vs `httpx2.Auth` or wrong client type is supplied after upgrading to `mcp>=2`. `params["ignore_initialized_notification_failure"]=True` is rejected on v2 (`docs/mcp.md:51`).

*   **Illegal lifecycle timeouts.** `0`, `nan`, `inf`, negative, non-numeric, or boolean values are rejected with `ValueError`/`TypeError` mentioning the field name (`src/agents/mcp/manager.py:15-27`, `tests/mcp/test_mcp_server_manager.py:439`). Missing validation at call-site would stall shutdown.

*   **Any-LLM provider without Responses support.** `_fetch_responses_response` checks `_supports_responses()` and raises `UserError("... does not support the Responses API")` (`src/agents/extensions/models/any_llm_model.py:1057`). Forcing `api="responses"` on an unsupported provider also fails fast at `:_selected_api:1175`.

*   **Prompt-managed requests on AnyLLM.** `_fetch_chat_response:873` and `_fetch_responses_response:1055` reject `prompt != None` with `UserError("... does not currently support prompt-managed requests.")`.

*   **Provider omits `usage`.** Litellm/any-llm non-streaming paths synthesize `Usage(requests=1)` fallback (`src/agents/extensions/models/litellm_model.py:298`, `src/agents/extensions/models/any_llm_model.py:434`) and streamed paths call `_mark_request_completed_without_usage` (`src/agents/extensions/models/any_llm_model.py:505`); tracing still emits a span with `requests:1` rather than dropping the metric.

*   **Content-filtered terminal turn.** LiteLLM/any-llm detect `finish_reason=="content_filter"` with empty message and synthesize a refusal (`src/agents/extensions/models/litellm_model.py:326`, `src/agents/extensions/models/any_llm_model.py:647`) so the runner can surface `ResponseOutputRefusal` instead of looping on an empty turn.

*   **Stream cancellation and cleanup races.** Adapters shield the terminal event and schedule background `aclose` on `CancelledError` (`src/agents/extensions/models/litellm_model.py:444`, `src/agents/extensions/models/any_llm_model.py:550`); `MCPServerManager` serializes overlapping `cleanup_all` calls and shields per-worker close tasks (`src/agents/mcp/manager.py:544-699`), tested by `tests/mcp/test_mcp_server_manager.py:570`.

*   **Reasoning payload type mismatch with any-llm.** `AnyLLMModel` dumps `Reasoning` via `model_dump(mode="json", exclude_none=True)` (`src/agents/extensions/models/any_llm_model.py:1116`) rather than `_to_dump_compatible`, avoiding the Pydantic-as-iterable “pair list instead of mapping” validation failure.

*   **BaseException propagation.** `FatalTaskBoundServer` tests (`tests/mcp/test_mcp_server_manager.py:1156`) verify `BaseException` bypasses normal `Exception` handling, bubbles through `MCPServerManager._attempt_connect`, and still leaves failed-server diagnostics while cleaning the resource.

*   **Cross-loop websocket use after loop close.** `OpenAIProvider._prune_closed_ws_loop_caches` (`src/agents/models/openai_provider.py:231`) forcibly drops connections and clears the loop entry when the loop is closed, preventing reuse of a stale `OpenAIResponsesWSModel` that would hang or error.

*   **Missing or truncated interop edge handling not found.** No evidence of automatic `tool_choice="required"` downgrade when a provider lacks tool support, or of automatic image-collapsing for text-only providers — those are caller responsibilities per `docs/models/index.md:672`.

## Future Considerations

*   **Adopt a shared `ModelConformanceSuite`.** Lift the common assertions currently duplicated across `tests/models/test_any_llm_model.py`, `tests/models/test_litellm_*`, and `tests/models/test_openai_*` into an abstract pytest mixin that any `Model` adapter must pass (call-ID uniqueness, `prev_response_id` passthrough vs drop, usage synthesis, streamed terminal-event ordering). This answers the dimension’s conformance gap without forcing adapters to share wire code.

*   **Decouple `MultiProvider` implicit-prefix registry.** Expose a plugin entry-point or late-registration API for fallback factories so a new prefix like `openrouter-llm/` can be added via `provider_map` *and* via implicit fallback without editing `src/agents/models/multi_provider.py:164`. Existing `MultiProviderMap` already covers explicit paths; the implicit set should either be frozen or made extensible.

*   **Central interop specification document.** Consolidate model capability matrix, MCP version negotiation rules, Session persistence atomicity guarantees, Sandbox workspace-policy semantics, and error-code taxonomy into one `docs/interop.md` so that “adding a new protocol” checklists are discoverable (currently split across `docs/models/index.md`, `docs/mcp.md`, and code docstrings).

*   **Formalize `ModelTracing` contract tests.** Add explicit tests that `ModelTracing.ENABLED_WITHOUT_DATA` actually elides prompts/tools per adapter and that `DONT_LOG_MODEL_DATA` redaction paths are exercised — the enum exists (`src/agents/models/interface.py:20`) but cross-adapter coverage is anecdotal.

*   **Version the interop contracts.** Tag `Model`, `MCPServer`, and `Session` interface revisions with a module-level `__interop_version__` or doc-level changelog so that adapter authors can pin against a known surface, especially as the `mcp` dependency spans two majors.

*   **Consider unifying LiteLLM/AnyLLM shared primitives.** A `chatcmpl_helpers` + `chatcmpl_converter` extraction could host the tool-ordering, logprobs-attachment, and usage-normalization logic now copied across two adapters, reducing drift risk identified in notable patterns.

## Questions / Gaps

*   **No central `AdapterAdder` example.** Search of `docs/models/` and `examples/model_providers/` shows how to use existing providers but not a minimal “hello-world” guide for authoring a brand-new `ModelProvider` + `Model` from scratch. `No clear evidence found` of a contributed-adapter template beyond the test helper `FakeAnyLLMProvider` (`tests/models/test_any_llm_model.py:58`).

*   **Sandbox interop boundary partially inferred.** The study inspected `src/agents/run_config.py:218` and `src/agents/sandbox/session/base_sandbox_session.py:199`, but did not deep-inspect manifest/schema versioning or the WebSocket `run_internal` contract for sandbox tool routing. Whether sandbox session state is considered stable external state is marked as `No clear evidence found` beyond type hints in this report.

*   **Distributed report path for this study.** The 19.03 output template expects `reports/repo/19.03-adapter-and-interop-boundary-design/{repo-name}.md`, while the orchestrator metadata asks for `reports/source/19.03-adapter-interop-boundary-design/{repo-name}.md`. This report was written to the latter (the path supplied in the rendered prompt). Recommend reconciling the prefix (`repo` vs `source`) and dash variance (`19.03-adapter-interop-boundary-design` vs `19.03-adapter-and-interop-boundary-design`) in UltraPlan configuration.

---

Generated by `Dimension 19.03: Adapter and Interop Boundary Design` against `openai-agents-sdk`.
