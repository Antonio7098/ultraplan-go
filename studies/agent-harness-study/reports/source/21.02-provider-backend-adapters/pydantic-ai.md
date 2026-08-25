# Source Analysis: pydantic-ai

## 21.02 Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic, anyio, httpx/httpx2, OpenTelemetry; optional SDKs per provider) |
| Analyzed | 2026-08-25 |

## Summary

pydantic-ai is built around a small set of explicit backend abstractions — `Model`, `Provider`, `EmbeddingModel`, `RealtimeModel`, and the durable-execution capability base `BaseDurabilityCapability` — each with many concrete implementations. Two orthogonal axes of swappability exist: **providers** own authentication/base-URL/HTTP-client concerns for one API family, while **model adapters** translate between the provider-neutral message/tool model and a vendor wire format. The same interface can be served by multiple providers (18 providers route through `OpenAIChatModel`; 8 through `OpenAIResponsesModel`), which is the framework's core adapter trick. Runtime selection is by string id (`'openai:gpt-5'`) resolved through factory functions (`infer_model`, `infer_provider_class`, `infer_embedding_model`) with injectable provider factories, plus context-manager overrides (`Agent.override(model=...)`, `Embedder.override(model=...)`). Wrapper/decorator adapters (`WrapperModel`, `WrapperEmbeddingModel`, `FallbackModel`, `InstrumentedModel`, `TemporalModel`) compose behavior over any backend without touching it. Configuration is externalized via environment variables (credentials, base URLs) and per-provider optional dependency groups with lazy imports, but there is no file-based backend registry: switching backends is a code-level string change, not a config-file change.

## Rating

**9 / 10** — Clear, explicit interfaces with dozens of interchangeable implementations; wrapper-based composition; runtime selection and override; operational safeguards (credential-safe model round-tripping across durable boundaries, `ALLOW_MODEL_REQUESTS` kill switch, HTTP client lifecycle management); and unusually strong conformance testing (a matrix test that probes every API-backed model class's outgoing wire payload against documented settings lists, plus per-provider VCR cassettes). Not a 10 because backend selection is not externally configurable via config files, the string-dispatch factories are hand-maintained `if/elif` chains (mitigated by drift-detection tests), and embeddings compatibility for OpenAI-compatible providers is assumed rather than declared (`embeddings/__init__.py:78-80`).

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to the source root `studies/agent-harness-study/sources/pydantic-ai`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model abstraction | `Model(AbstractModel, Generic[InterfaceClient])` ABC with `request()`, `request_stream()`, `count_tokens()`, `compact_messages()` extension points; module docstring states aim "to make a common interface for different LLMs" | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1-5`, `366`, `505-555` |
| Shared model identity | `AbstractModel` ABC shared by request-response models and realtime models (name, system, base_url, OTel `gen_ai.system` semantics) | `pydantic_ai_slim/pydantic_ai/models/_abstract.py:19-48` |
| Streamed response abstraction | `StreamedResponse(ABC)` with abstract `_get_event_iterator()`, cancel/close lifecycle hooks adapters must implement | `pydantic_ai_slim/pydantic_ai/models/__init__.py:952`, `1099-1140` |
| Provider abstraction | `Provider(ABC, Generic[InterfaceClient])`: `name`, `base_url`, `client` abstract properties, `model_profile()` hook, HTTP client ownership with re-entry handling | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:42-136` |
| Provider/model decoupling docstring | "Each provider only supports a specific interface. An interface can be supported by multiple providers" (e.g. `OpenAIChatModel` + `DeepSeekProvider`) | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:47-53` |
| OpenAI-interface reuse | `OpenAIChatCompatibleProvider` literal lists 19 providers; `OpenAIResponsesCompatibleProvider` lists 8 routed through `OpenAIResponsesModel` | `pydantic_ai_slim/pydantic_ai/models/__init__.py:123-159` |
| Provider implementations count | ~35 concrete providers in `providers/` (openai, anthropic, google, bedrock, azure, groq, mistral, cohere, xai, ollama, openrouter, cerebras, snowflake, litellm, gateway, ...) | `pydantic_ai_slim/pydantic_ai/providers/` directory listing |
| Model adapter implementations | ~20 concrete model classes in `models/` (anthropic.py, openai.py, google.py, bedrock.py, groq.py, ollama.py, openrouter.py, cerebras.py, crusoe.py, snowflake.py, zai.py, huggingface.py, ...) | `pydantic_ai_slim/pydantic_ai/models/` directory listing |
| Embedding backend abstraction | `EmbeddingModel(ABC)` with `embed()`, `prepare_embed()`, `count_tokens()`, `max_input_tokens()` | `pydantic_ai_slim/pydantic_ai/embeddings/base.py:8-116` |
| Embedding implementations | OpenAI, Cohere, Google, Bedrock, VoyageAI, SentenceTransformers (local inference), TestEmbeddingModel | `pydantic_ai_slim/pydantic_ai/embeddings/openai.py`, `cohere.py`, `google.py`, `bedrock.py`, `voyageai.py`, `sentence_transformers.py`, `test.py:28` |
| Realtime backend abstraction | `RealtimeModel(AbstractModel)` plus `RealtimeProviderSession` Protocol; OpenAI/Azure/Google/x.ai sessions in realtime/ | `pydantic_ai_slim/pydantic_ai/realtime/model.py:82`, `141` |
| Durable-execution backends | `BaseDurabilityCapability` shared base; concrete `TemporalDurability`, `DBOSDurability`, `PrefectDurability` | `pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:40`; `temporal/_durability.py:132`; `dbos/_durability.py:37`; `prefect/_durability.py:37` |
| Tracing sink swappability | `InstrumentationSettings(tracer_provider=..., meter_provider=...)` falls back to global OTel providers ("Calling `logfire.configure()` sets the global tracer provider") | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:82-148` |
| Instrumentation wrapper factory | `instrument_model(model, instrument)` wraps any `Model` in `InstrumentedModel` unless already instrumented | `pydantic_ai_slim/pydantic_ai/models/instrumented.py:51-59`, `333` |
| Wrapper/decorator pattern | `WrapperModel(Model)` forwards every method/property to `wrapped` and `__getattr__`s the rest; used as base for Fallback/Instrumented/Temporal/MCP-sampling wrappers | `pydantic_ai_slim/pydantic_ai/models/wrapper.py:32-157` |
| Embedding wrapper | `WrapperEmbeddingModel(EmbeddingModel)` mirror pattern | `pydantic_ai_slim/pydantic_ai/embeddings/wrapper.py:16` |
| Failure-driven backend swap | `FallbackModel(default_model, *fallback_models, fallback_on=(ModelAPIError,))` swaps backends at runtime on exception/response handlers | `pydantic_ai_slim/pydantic_ai/models/fallback.py:86-120` |
| Shared OpenAI-compatible provider base | `OpenAICompatibleProvider(Provider[AsyncOpenAI])` centralizes HTTP client lifecycle for all OpenAI-SDK-backed providers | `pydantic_ai_slim/pydantic_ai/providers/_openai_compatible.py:13-55` |
| String→backend factory (models) | `infer_model(model, provider_factory=infer_provider)` — injectable provider factory; dispatches on provider prefix to concrete model classes | `pydantic_ai_slim/pydantic_ai/models/__init__.py:1529-1650` |
| String→backend factory (providers) | `infer_provider_class(provider)` name→class registry; `infer_provider()` instantiates; `gateway/<upstream>` normalization | `pydantic_ai_slim/pydantic_ai/providers/__init__.py:142-293` |
| String→backend factory (embeddings) | `infer_embedding_model(model, *, provider_factory=infer_provider)` | `pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:83-139` |
| Gateway routing adapter | Overloaded `gateway_provider(...)` maps upstream providers to gateway routes; `_GatewayRequestHook` typed callable class | `pydantic_ai_slim/pydantic_ai/providers/gateway.py:36-127`, `316` |
| Runtime swap at agent level | `Agent.override(model=...)` context manager backed by `_override_model` ContextVar; also per-run `model=` kwarg on run methods | `pydantic_ai_slim/pydantic_ai/agent/__init__.py:698`, `1957-2059` |
| Runtime swap at embedder level | `Embedder.override(model=...)` context manager using ContextVar | `pydantic_ai_slim/pydantic_ai/embeddings/__init__.py:227-264` |
| Cross-boundary model round-trip | Durable capabilities carry only `model_id` strings across activity/workflow boundaries; unregistered instances rejected rather than rebuilt from `model_id` (would hit "another endpoint with other credentials"); deps-aware rebuild via capability chain then `infer_model` | `pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:382-397`, `421-456`, `458-494` |
| Capability-flag configuration | `ModelProfile` TypedDict merged DEFAULT → provider → user layers in `Model.profile`; adapter-declared `supported_tool_deferral_modes` intersect profile claims | `pydantic_ai_slim/pydantic_ai/profiles/__init__.py:47`, `217`, `249`; `models/__init__.py:863-903`, `369-377`, `467-477` |
| Env-var externalized credentials | Providers read env keys/base URLs (57 `os.environ`/`getenv` references across providers/); e.g. `OPENAI_API_KEY`, `OPENAI_BASE_URL` with actionable error pointing at `Provider(api_key=...)` | `pydantic_ai_slim/pydantic_ai/providers/openai.py:96-102` |
| Per-provider optional dependencies | Optional dependency groups per provider/integration (`openai`, `anthropic`, `bedrock`, `temporal`, `dbos`, `prefect`, ...); adapters lazily imported so missing SDKs fail only when used | `pydantic_ai_slim/pyproject.toml:72-161`; lazy imports in `providers/__init__.py:152-279` |
| Cost-free testing backends | `TestModel` (assertable canned responses) and `FunctionModel` (user function stands in for LLM); exempt from `ALLOW_MODEL_REQUESTS` global kill switch | `pydantic_ai_slim/pydantic_ai/models/test.py:62`; `function.py:52`; `models/__init__.py:1388-1423` |
| Wire-conformance test matrix | `test_supported_by_lists_match_the_wire` probes each model class's real outbound request against the documented `Supported by:` lists; `test_every_api_backed_model_class_is_probed` walks the package to force new `Model` classes into the matrix | `tests/models/test_model_settings_support.py:584-602`, `623-640` |
| Per-provider test suites | Dedicated tests for ~35 providers under `tests/providers/test_<provider>.py` and per-model suites under `tests/models/test_<provider>.py` with VCR cassettes replaying real API responses | `tests/providers/`, `tests/models/` directories; `tests/AGENTS.md` VCR workflow section |
| Cross-provider portability test | `test_tool_availability_portability.py` pins that tool-availability deltas render correctly when history crosses providers | `tests/test_tool_availability_portability.py` |

## Answers to Dimension Questions

### 1. Are backends swappable?

Yes, comprehensively, along five families:

- **LLM backends**: anything satisfying `Model.request()/request_stream()/count_tokens()` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:505-555`). Selection is a string (`Agent('openai:gpt-5')`) resolved by `infer_model` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:1529`), or an instance — including custom subclasses.
- **Providers (auth/transport)**: `Provider` ABC (`pydantic_ai_slim/pydantic_ai/providers/__init__.py:42`) separates *which API family/wire format* (the model adapter) from *which account/endpoint* (the provider). One interface × N providers is explicit design (`providers/__init__.py:47-53`).
- **Embedding backends**: `EmbeddingModel` ABC (`pydantic_ai_slim/pydantic_ai/embeddings/base.py:8`) with six remote/local implementations.
- **Realtime backends**: `RealtimeModel` (`pydantic_ai_slim/pydantic_ai/realtime/model.py:141`) with per-vendor session implementations.
- **Durable-execution engines**: `BaseDurabilityCapability` with Temporal/DBOS/Prefect implementations sharing one contract (`pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:40`).
- **Tracing sinks**: delegated to OpenTelemetry's global or injected `tracer_provider`/`meter_provider` (`pydantic_ai_slim/pydantic_ai/models/instrumented.py:145-149`); Logfire is just one OTel-compatible option.

Not covered: database stores, vector DBs, and message queues are *not* abstracted by pydantic-ai itself — there is no storage/vector-store interface anywhere in `pydantic_ai_slim/pydantic_ai/`. Vector-adjacent functionality stops at embeddings; queueing is reached indirectly via the three durable-execution engine adapters. Sandboxes likewise have no dedicated abstraction (the only "sandbox" mentions are Temporal's workflow sandbox, e.g. `pydantic_ai_slim/pydantic_ai/providers/__init__.py:63-66`). This is consistent with the library's stated philosophy of primitives over batteries-included scope (root `AGENTS.md`, Philosophy section).

### 2. Which backends have multiple implementations?

- **Model adapters**: ~20 concrete classes in `models/` (anthropic, openai chat+responses, google, bedrock converse, bedrock mantle (chat+responses split), groq, mistral, cohere, xai, huggingface, ollama, openrouter, cerebras, crusoe, snowflake, zai, mcp_sampling, function, test). Additionally behavioral wrappers: `FallbackModel`, `InstrumentedModel`, `ConcurrencyLimitedModel` (listed as non-API-backed classes at `tests/models/test_model_settings_support.py:617-619`).
- **Interface multiplexing**: the strongest evidence of interchangeability — `OpenAIChatCompatibleProvider` names 19 providers that reuse `OpenAIChatModel`, and `OpenAIResponsesCompatibleProvider` names 8 that reuse `OpenAIResponsesModel` (`pydantic_ai_slim/pydantic_ai/models/__init__.py:123-159`). A new OpenAI-compatible vendor needs only a thin `Provider` subclass over `OpenAICompatibleProvider` (`pydantic_ai_slim/pydantic_ai/providers/_openai_compatible.py:13`), not a new model adapter.
- **Providers**: ~35 files under `providers/`.
- **Embeddings**: 6 implementations + test double.
- **Durable engines**: 3 implementations behind one base class.
- **Realtime**: OpenAI, Azure, Google, x.ai sessions.

### 3. Can backends be swapped at runtime?

Yes, at four granularities:

1. **Per-run**: every run method accepts `model=` (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1279`).
2. **Scoped override**: `with agent.override(model='...'):` swaps via ContextVar (`pydantic_ai_slim/pydantic_ai/agent/__init__.py:1957`, `2059`); same for embedders (`embeddings/__init__.py:227-264`).
3. **Mid-run failure swap**: `FallbackModel` moves to the next backend on `ModelAPIError` or custom exception/response handlers (`pydantic_ai_slim/pydantic_ai/models/fallback.py:104-120`).
4. **Cross-process boundary**: durable workflows pass model-id strings and rebuild instances worker-side through a registry/capability chain with `infer_model` as backstop (`pydantic_ai_slim/pydantic_ai/durable_exec/_base.py:458-494`) — deliberately constrained so an unregistered instance fails loudly instead of silently resolving to a different endpoint (`_base.py:450-456`).

What you cannot do is select backends from a static config file consumed by the framework. The nearest externalized mechanisms are environment variables (credentials/base URL: `providers/openai.py:96-102`) and the agent `spec=` dict (`agent/__init__.py:1970`), which can supply a model string but still requires application code to consume it. So the dimension's litmus question — "can you switch Postgres to SQLite with a config change?" — translates here to "can you switch Anthropic to Bedrock with a config change?": switching is a one-string change at construction/run time, but that string lives in code, not in an external config format.

### 4. Are adapter implementations tested?

Extensively, with a distinctive conformance-testing strategy:

- **Per-adapter suites**: every provider has initialization tests in `tests/providers/test_<provider>.py` (~35 files) and wire-behavior tests in `tests/models/test_<provider>.py`, mostly VCR-recorded real requests replayed offline (`tests/AGENTS.md`, "VCR Workflow").
- **Wire-conformance matrix**: `tests/models/test_model_settings_support.py:584-602` probes each API-backed model class's actual outbound request for each generic setting field and asserts it exactly matches the `Supported by:` documentation lists; `test_every_api_backed_model_class_is_probed` (`:623-640`) introspects the package so a newly added `Model` class *cannot* escape the matrix (this test exists because `CrusoeModel`, `SnowflakeModel`, and Bedrock Mantle previously drifted out, per its docstring).
- **Interchangeability tests**: cross-provider history portability is pinned by `tests/test_tool_availability_portability.py` and `Model.prepare_messages` translation logic (`pydantic_ai_slim/pydantic_ai/models/__init__.py:690-783`, which normalizes parts produced by other providers before rendering).
- **Test doubles as first-class adapters**: `TestModel`/`FunctionModel`/`TestEmbeddingModel` implement the same interfaces, so user test setups exercise the identical adapter path (`models/test.py:62`, `models/function.py:52`, `embeddings/test.py:28`).
- **Durable round-trip tests**: workflow-level tests for Temporal/DBOS/Prefect exist (`tests/test_temporal.py`, `tests/test_dbos.py`, `tests/test_prefect.py`).

## Architectural Decisions

1. **Two-axis decomposition: Provider × Model.** Providers answer "who am I talking to and with what credentials"; model adapters answer "what wire shape does this API speak". This lets 19 vendors share the OpenAI chat adapter (`models/__init__.py:123-146`) and lets the same vendor expose two interfaces (`openai` vs `openai-chat` prefixes both map to `OpenAIProvider` but different model classes, `providers/__init__.py:152-155`, `models/__init__.py:1609-1616`).
2. **Generic-over-client typing.** Both `Model` and `Provider` are generic in `InterfaceClient` (`models/__init__.py:366`; `providers/__init__.py:24`, `42`), so the type checker enforces that e.g. `AnthropicModel` receives an `AsyncAnthropic`, not an arbitrary client.
3. **Lazy imports keyed off optional extras.** Every entry in `infer_provider_class` imports its provider module inside the branch (`providers/__init__.py:152-279`), matching the per-provider dependency groups (`pydantic_ai_slim/pyproject.toml:72-161`); the slim install carries no vendor SDKs.
4. **Profiles as the single capability source of truth.** Feature facts live in `ModelProfile` dicts layered DEFAULT → provider → user, intersected with what the adapter class actually implements (`models/__init__.py:863-903`); wire-mechanics facts that belong to the API rather than the model are declared on the adapter class itself (`compaction_requires_encrypted_content`, `models/__init__.py:379-398`) — a deliberate split justified in comments because eight providers route one profile through `OpenAIResponsesModel`.
5. **Strings cross trust boundaries; instances don't.** Durable execution serializes only model-id strings and rejects unregistered instances, explicitly to prevent credential confusion on rebuild (`durable_exec/_base.py:421-456`).
6. **Decorator-style composition over inheritance hierarchies.** Cross-cutting behaviors (instrumentation, fallback, durability, MCP sampling, concurrency limits) wrap any `Model` via `WrapperModel` rather than subclassing concrete adapters (`models/wrapper.py:32-43`), mirroring the `WrapperToolset` rule in `pydantic_ai_slim/pydantic_ai/AGENTS.md` (rule:987).

## Notable Patterns

- **Literal-typed compatibility registries**: `OpenAIChatCompatibleProvider` / `OpenAIResponsesCompatibleProvider` TypeAliasType literals act as declarative route tables consulted by both `infer_model` and `infer_embedding_model` (`models/__init__.py:123-159`; `embeddings/__init__.py:80`, `105-114`).
- **Injectable factory seams**: `infer_model(model, provider_factory=...)` lets callers substitute provider construction wholesale (`models/__init__.py:1530`, `1563`).
- **Self-healing HTTP client lifecycle**: providers track owned clients and recreate closed clients on re-entry (`providers/__init__.py:114-122`, `_set_http_client` injection point at `:107-112`), with event-loop-binding deferred via cached locks for Temporal's sandbox (`:61-66`).
- **Graceful degradation contracts**: unsupported generic settings no-op rather than raise to preserve portability across models, while unrepresentable content raises explicitly — both encoded as review-extracted rules in `pydantic_ai_slim/pydantic_ai/models/AGENTS.md`.
- **Error-message ergonomics as adapter UX**: unknown ids get closest-match suggestions (`models/__init__.py:1549-1561`); missing API keys point at the keyless `Agent('test')` path (`providers/__init__.py:26-39`).

## Tradeoffs

- **Hand-maintained dispatch chains**: adding a provider touches `infer_provider_class` (`providers/__init__.py:142-281`), `infer_model` (`models/__init__.py:1583-1648`), and optionally `KnownModelName`. The chains are plain `if/elif` string matching rather than a registration decorator/entry-point mechanism — simple and greppable, but easy to forget until a test catches it (and the settings-support test was written precisely because some slipped through, `tests/models/test_model_settings_support.py:623-631`).
- **Assumed embedding compatibility**: all chat-compatible providers are assumed to serve embeddings too; a mismatch surfaces only as a runtime `ModelHTTPError` (`embeddings/__init__.py:78-80`) — cheap to maintain, imprecise at runtime.
- **Global mutable switches**: `ALLOW_MODEL_REQUESTS` and `Embedder.instrument_all` are process-global state (`models/__init__.py:1388`; `embeddings/__init__.py:175`, `208-220`), fine for tests, coarse for multi-tenant services.
- **OTel coupling in the core package**: instrumentation lives inside `pydantic_ai` (not a plugin), trading slimmer core for guaranteed spec-conformant spans (enforced by the rule that `_otel_*` modules implement only spec-defined features, `pydantic_ai_slim/pydantic_ai/AGENTS.md` rule:17).

## Failure Modes / Edge Cases

- **Silent endpoint/credential substitution**: rebuilding a `Model` from its id on a worker would target "another endpoint with other credentials"; guarded by rejecting unregistered instances (`durable_exec/_base.py:450-456`).
- **Provider-name stability is load-bearing**: renaming a `Provider.name` breaks replay of message histories whose thinking-tag/native-tool detection reads `provider_name` (`providers/__init__.py:70-78`).
- **Stale closed clients**: a reused provider after context-manager exit recreates its HTTP client via the stored factory (`providers/__init__.py:116-121`); providers managing foreign clients must override `_set_http_client` or the SDK keeps the dead transport.
- **Profile/adapter claim mismatches**: a profile claiming a deferral mode an adapter can't render is neutralized by the `ClassVar` intersection (`models/__init__.py:369-377`), preventing wire shapes the renderer can't produce.
- **Costly accidental requests**: mitigated by the default-on `ALLOW_MODEL_REQUESTS` flip in test fixtures and the always-cheap `TestModel`/`FunctionModel` exemptions (`models/__init__.py:1394-1399`).

## Future Considerations

- A declarative provider registry (decorator or entry-point based) would collapse the three hand-maintained dispatch sites and let third-party packages register providers without patching core functions.
- Declared embedding-endpoint compatibility per provider would replace the blanket chat-compatibility assumption (`embeddings/__init__.py:78-80`).
- An optional file/env-driven agent spec (beyond the existing `spec=` dict) would complete the externalization story for ops teams wanting backend changes without deploys.

## Questions / Gaps

- No evidence found for store/vector-DB/queue/sandbox abstractions within this source; searched `pydantic_ai_slim/pydantic_ai/` for storage-, vector-, queue-, and sandbox-related modules (only hits: Temporal's workflow-sandbox notes and durable-exec engines acting as queues). If such integrations exist they live outside this repository.
- Whether the `spec=` dict is intended as a full configuration externalization surface could not be confirmed from implementation alone; it accepts model strings (`agent/__init__.py:1970`, `2031-2032`) but no evidence shows a canonical YAML/JSON loader beyond the `spec` optional dependency group (`pydantic_ai_slim/pyproject.toml:161`).

---

Generated by dimension 21.02 (Provider and Backend Adapters) against `pydantic-ai`.
