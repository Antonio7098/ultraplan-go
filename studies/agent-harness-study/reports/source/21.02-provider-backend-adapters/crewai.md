# Source Analysis: crewai

## Dimension 21.02 — Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python 3.10–3.14 (uv workspace monorepo: `lib/crewai`, `lib/crewai-core`, `lib/crewai-tools`, `lib/crewai-files`, `lib/cli`, `lib/devtools`; see `studies/agent-harness-study/sources/crewai/pyproject.toml:5`) |
| Analyzed | 2026-08-25 |

## Summary

CrewAI implements a deliberate adapter layer for nearly every backend category an agent harness touches: LLMs, memory vector stores, knowledge/RAG vector clients, embedding providers, distributed locks, and event/tracing sinks. The dominant pattern is a Protocol or ABC plus a factory seam:

- **LLMs**: a single abstract `BaseLLM` (`lib/crewai/src/crewai/llms/base_llm.py:150`) with seven native SDK provider implementations plus a LiteLLM catch-all. The `LLM.__new__` constructor is itself a factory that routes on model-string prefix or explicit `provider=` kwarg (`lib/crewai/src/crewai/llm.py:393-512`). Users can also pass any custom `BaseLLM` subclass.
- **Memory**: a `StorageBackend` runtime-checkable Protocol (`lib/crewai/src/crewai/memory/storage/backend.py:44-45`) with two built-in implementations (LanceDB, Qdrant Edge) and a process-wide pluggable factory (`set_memory_storage_factory`, `lib/crewai/src/crewai/memory/storage/factory.py:33`).
- **Knowledge/RAG**: a `BaseClient` Protocol (`lib/crewai/src/crewai/rag/core/base_client.py:66-67`) with ChromaDB and Qdrant implementations and a per-provider client-factory registry (`register_rag_client_factory`, `lib/crewai/src/crewai/rag/factory.py:25-33`).
- **Embeddings**: 18 providers selected from a dict spec via a `PROVIDER_PATHS` registry (`lib/crewai/src/crewai/rag/embeddings/factory.py:90-110`).
- **Locks**: Redis vs file-lock selection by env var plus a replaceable backend setter (`set_lock_backend`, `lib/crewai-core/src/crewai_core/lock_store.py:45`).
- **Events/tracing**: a central bus with register/unregister handlers at runtime (`on`/`off`, `lib/crewai/src/crewai/events/event_bus.py:245,368`) and listener classes derived from `BaseEventListener` (`lib/crewai/src/crewai/events/base_event_listener.py:8-20`).

Not everything is swappable: the product-telemetry OTLP endpoint is hard-coded to CrewAI's collector (`lib/crewai/src/crewai/telemetry/constants.py:9`), the kickoff-output store is fixed SQLite (`lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19-27`), and the file cache is fixed to in-memory aiocache (`lib/crewai/src/crewai/utilities/file_store.py:20-30`). Overall this is a clear, well-tested adapter model with a few non-pluggable outliers.

## Rating

**8 / 10** — Clear model with explicit interfaces (Protocols/ABCs), multiple implementations per backend, factory registries, externalized config (model strings, dict specs, env vars), and dedicated seam tests. Falls short of 9–10 because three backends are hard-wired (telemetry sink, kickoff SQLite store, in-memory file cache), some seams are process-wide globals rather than scoped, and several error paths swallow exceptions silently (e.g., knowledge search returns `[]`, `lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:85-89`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| LLM abstraction | `BaseLLM(BaseModel, ABC)` with abstract `call()`; docstring invites user subclasses "that don't rely on litellm" | `lib/crewai/src/crewai/llms/base_llm.py:150-167,314-350` |
| LLM routing factory | `LLM.__new__` routes to native SDK class or LiteLLM fallback based on prefix/provider kwarg/constants | `lib/crewai/src/crewai/llm.py:393-512` |
| Native provider registry | `SUPPORTED_NATIVE_PROVIDERS` list of 17 aliases; `_get_native_provider` maps names → classes | `lib/crewai/src/crewai/llm.py:327-345,665-715` |
| Provider implementations | `OpenAICompletion`, `AnthropicCompletion`, `AzureCompletion`, `GeminiCompletion`, `BedrockCompletion`, `OpenAICompatibleCompletion(OpenAICompatibility base)`, `SnowflakeCompletion(OpenAICompletion)` | `lib/crewai/src/crewai/llms/providers/openai/completion.py:180`, `.../anthropic/completion.py:211`, `.../azure/completion.py:71`, `.../gemini/completion.py:37`, `.../bedrock/completion.py:204`, `.../openai_compatible/completion.py:113`, `.../snowflake/completion.py:49` |
| Custom LLM injection point | `BaseAgent.llm: str \| BaseLLM \| None` field accepts any BaseLLM instance | `lib/crewai/src/crewai/agents/agent_builder/base_agent.py:332-336` |
| LLM config coercion | `create_llm()` accepts instance, model string, dict, attribute-bearing object, or env fallback | `lib/crewai/src/crewai/utilities/llm_utils.py:13-87,97-200` |
| Provider env-var map | `ENV_VARS` dict maps provider → API-key env prompts (OPENAI_API_KEY, ANTHROPIC_API_KEY, GEMINI_API_KEY, AZURE_*) | `lib/crewai/src/crewai/constants.py:29-109` |
| LLM round-trip config | `to_config_dict()` serializes LLM for reconstruction; subclasses add fields (e.g., project/location) | `lib/crewai/src/crewai/llms/base_llm.py:292-312` |
| HTTP interception seam | `HTTPTransport`/`AsyncHTTPTransport` wrap httpx transports with a `BaseInterceptor` for request/response mutation | `lib/crewai/src/crewai/llms/hooks/transport.py:54-90,112` |
| Memory storage protocol | `@runtime_checkable StorageBackend(Protocol)` with save/search/delete/update/list/reset + async twins; typed `MemoryRecord`s | `lib/crewai/src/crewai/memory/storage/backend.py:44-212` |
| Memory backends | `LanceDBStorage` (local vector store) and `QdrantEdgeStorage` (sharded local qdrant) | `lib/crewai/src/crewai/memory/storage/lancedb_storage.py:42`, `lib/crewai/src/crewai/memory/storage/qdrant_edge_storage.py:81` |
| Memory selection | `Memory.storage: StorageBackend \| str = "lancedb"`; string specs resolved via factory → `"qdrant-edge"` → `"lancedb"` → path | `lib/crewai/src/crewai/memory/unified_memory.py:92-93,232-251` |
| Memory factory seam | `MemoryStorageFactory = Callable[[str], StorageBackend \| None]`; `set_memory_storage_factory` for app startup; explicit instance always wins | `lib/crewai/src/crewai/memory/storage/factory.py:28-55` |
| Knowledge storage abstraction | `BaseKnowledgeStorage(BaseModel, ABC)`; built-in `KnowledgeStorage` delegates to RAG `BaseClient` | `lib/crewai/src/crewai/knowledge/storage/base_knowledge_storage.py:13`, `lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:22-57` |
| Knowledge factory seam | `KnowledgeStorageFactory(embedder, collection_name) -> BaseKnowledgeStorage \| None`; `set_knowledge_storage_factory` | `lib/crewai/src/crewai/knowledge/storage/factory.py:29-56` |
| RAG client protocol | `@runtime_checkable BaseClient(Protocol)` covering collections/add/search/delete/reset sync+async; notes it replaces former `BaseRAGStorage` | `lib/crewai/src/crewai/rag/core/base_client.py:66-98,114-448` |
| RAG client registry | Provider-keyed `_factories: dict[str, RagClientFactory]`; `register/unregister_rag_client_factory`; `create_client` dispatches chromadb/qdrant then raises `ValueError` | `lib/crewai/src/crewai/rag/factory.py:20-78` |
| RAG backends | `ChromaDBClient(BaseClient)` and `QdrantClient(BaseClient)` with per-provider factories | `lib/crewai/src/crewai/rag/chromadb/client.py:39`, `lib/crewai/src/crewai/rag/chromadb/factory.py:16-36`, `lib/crewai/src/crewai/rag/qdrant/client.py:32`, `lib/crewai/src/crewai/rag/qdrant/factory.py:9-26` |
| Runtime RAG swap | `_rag_context` ContextVar; `set_rag_config(config)` builds and installs a live client; `get_rag_client()` lazily materializes defaults | `lib/crewai/src/crewai/rag/config/utils.py:26-81` |
| Embedder registry | `PROVIDER_PATHS`: 18 dotted-path entries (openai, azure, bedrock, cohere, google/generativeai/vertex, huggingface, instructor, jina, ollama, onnx, openclip, roboflow, sentence-transformer, text2vec, voyageai, watsonx, custom); unknown provider raises listing options | `lib/crewai/src/crewai/rag/embeddings/factory.py:90-110,242-263` |
| Embedder spec types | `ProviderSpec` TypeAlias union over per-provider TypedDicts; `EmbedderConfig` alias consumed by knowledge/agent config | `lib/crewai/src/crewai/rag/embeddings/types.py:32,74` |
| Embedding provider ABC | `BaseEmbeddingsProvider(BaseSettings, Generic[T])` requires `embedding_callable: type[T]` | `lib/crewai/src/crewai/rag/core/base_embeddings_provider.py:14-24` |
| Lock backend swap | `LockBackend = Callable[..., AbstractContextManager[None]]`; `set_lock_backend()`; default selects Redis lock when `REDIS_URL`+redis installed else temp-file portalocker | `lib/crewai-core/src/crewai_core/lock_store.py:39-54,99-121` |
| Event bus | Singleton `CrewAIEventsBus`; `on()` decorator, `off()`, `emit()`/`aemit()`, scoped handlers via context manager | `lib/crewai/src/crewai/events/event_bus.py:95,245-280,368,572,771,832` |
| Event listener extension point | `BaseEventListener(ABC)` auto-registers via `setup_listeners(bus)`; `TraceCollectionListener` ships as built-in trace sink | `lib/crewai/src/crewai/events/base_event_listener.py:8-25`, `lib/crewai/src/crewai/events/listeners/tracing/trace_listener.py:140,203` |
| Product telemetry (fixed sink) | Singleton `Telemetry` exports OTLP spans to hard-coded `CREWAI_TELEMETRY_BASE_URL = https://telemetry.crewai.com:4319`; only opt-out via `OTEL_SDK_DISABLED`/`CREWAI_DISABLE_TELEMETRY`/`CREWAI_DISABLE_TRACKING` | `lib/crewai/src/crewai/telemetry/telemetry.py:94-167`, `lib/crewai/src/crewai/telemetry/constants.py:9` |
| Kickoff outputs (fixed backend) | `KickoffTaskOutputsSQLiteStorage` hard-wires sqlite3 with optional `db_path` override only | `lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19-27` |
| File cache (fixed backend) | Module-level `_file_store = Cache(Cache.MEMORY, ...)` from aiocache; features disabled if import fails | `lib/crewai/src/crewai/utilities/file_store.py:20-30` |
| Sandbox adapters | E2B sandbox tools parameterized by env (`E2B_API_KEY`, `E2B_DOMAIN`), template, idle timeout, and attach-by-`sandbox_id`; no abstract Sandbox interface (single-vendor tools) | `lib/crewai-tools/src/crewai_tools/tools/e2b_sandbox_tool/e2b_base_tool.py:16-137` |
| Adapter tests (routing) | 56 tests in test_llm.py incl. native-vs-LiteLLM routing, prefix handling, explicit-provider priority | `lib/crewai/tests/test_llm.py:809-957` |
| Adapter tests (custom LLM) | End-to-end custom `BaseLLM` implementation inside a Crew, JWT auth variant, timeout variant | `lib/crewai/tests/test_custom_llm.py:83-333` |
| Adapter tests (providers) | Per-provider suites: 27 test files under `tests/llms/{openai,anthropic,azure,google,bedrock,snowflake,litellm,openai_compatible,hooks}` | `lib/crewai/tests/llms/` (directory) |
| Adapter tests (memory seam) | Factory registration, raw-spec passthrough, explicit-instance bypass | `lib/crewai/tests/memory/test_storage_factory.py:25-58` |
| Adapter tests (knowledge seam) | Registration, embedder/collection args, bypass, falsy-instance honored | `lib/crewai/tests/knowledge/test_storage_factory.py:63-110` |
| Adapter tests (RAG registry) | Registered factory used, overrides built-in provider, unregister semantics, unknown provider still raises | `lib/crewai/tests/rag/test_client_factory_registry.py:27-64` |
| Adapter tests (locks) | Redis vs file selection, custom backend used, clearing restores default | `lib/crewai/tests/utilities/test_lock_store.py:36-101` |
| Adapter tests (embedders/RAG clients) | `test_embedding_factory.py`, `test_factory_azure.py`, `test_backward_compatibility.py`, `tests/rag/chromadb/test_client.py`, `tests/rag/qdrant/test_client.py` | `lib/crewai/tests/rag/embeddings/`, `lib/crewai/tests/rag/chromadb/`, `lib/crewai/tests/rag/qdrant/` |

## Answers to Dimension Questions

1. **Are backends swappable?**
Yes, for the core categories. LLMs: any `BaseLLM` subclass can be injected through `Agent.llm` (`lib/crewai/src/crewai/agents/agent_builder/base_agent.py:332-336`) or routed automatically via model string (`lib/crewai/src/crewai/llm.py:393-512`). Memory: `StorageBackend` instances or spec strings (`lib/crewai/src/crewai/memory/unified_memory.py:92,232-251`). Knowledge: `BaseKnowledgeStorage` instances or registered factories (`lib/crewai/src/crewai/knowledge/storage/factory.py:36`). RAG clients: ChromaDB/Qdrant interchangeable behind `BaseClient`, both selectable purely by config object passed to `create_client` (`lib/crewai/src/crewai/rag/factory.py:41-78`). Locks: Redis ↔ file ↔ custom callable (`lib/crewai-core/src/crewai_core/lock_store.py:45-54`). Not swappable: telemetry sink (hard-coded URL), kickoff outputs (SQLite only), file cache (in-memory aiocache only).

2. **Which backends have multiple implementations?**
LLMs: seven native classes + OpenAICompatible generic + LiteLLM fallback (`lib/crewai/src/crewai/llm.py:665-715`). Memory stores: LanceDB and QdrantEdge plus any factory-provided backend (`lib/crewai/src/crewai/memory/unified_memory.py:238-251`). RAG clients: ChromaDB and Qdrant (`lib/crewai/src/crewai/rag/chromadb/client.py:39`, `lib/crewai/src/crewai/rag/qdrant/client.py:32`). Embeddings: 18 providers in `PROVIDER_PATHS` (`lib/crewai/src/crewai/rag/embeddings/factory.py:90-110`). Locks: Redis and file-based (`lib/crewai-core/src/crewai_core/lock_store.py:99-121`). Single-implementation backends: telemetry exporter, kickoff-output store, file cache.

3. **Can backends be swapped at runtime?**
Partially, and the seams differ in scope:
   - **Per-context**: the RAG client lives in a `ContextVar`; `set_rag_config()` swaps the active client immediately, including concurrent-scoped swaps (`lib/crewai/src/crewai/rag/config/utils.py:26-37`).
   - **At any time**: event-bus handlers can be added/removed while running via `on()`/`off()` (`lib/crewai/src/crewai/events/event_bus.py:245,368`).
   - **Process-wide, startup-intent**: `set_memory_storage_factory`, `set_knowledge_storage_factory`, and `set_lock_backend` explicitly document "one-time setup at startup"; only instances constructed afterwards are affected, and an explicit constructor argument always wins (`lib/crewai/src/crewai/memory/storage/factory.py:34-43`, `lib/crewai-core/src/crewai_core/lock_store.py:48-51`).
   - **Construction-time only**: LLM provider choice is fixed at instantiation by `__new__` routing; there is no post-hoc re-route.

4. **Are adapter implementations tested?**
Extensively. Routing logic has 56 dedicated tests covering native-vs-LiteLLM decisions, prefixed models, pattern matches, and explicit-provider priority (`lib/crewai/tests/test_llm.py:809-957`); each native provider has its own suite (27 files under `lib/crewai/tests/llms/`); custom `BaseLLM` substitution is tested end-to-end within a Crew (`lib/crewai/tests/test_custom_llm.py:83-128`); each factory seam has focused tests asserting registration, precedence, bypass, and teardown (`lib/crewai/tests/memory/test_storage_factory.py:25-58`, `lib/crewai/tests/knowledge/test_storage_factory.py:63-110`, `lib/crewai/tests/rag/test_client_factory_registry.py:27-64`, `lib/crewai/tests/utilities/test_lock_store.py:82-101`). Both vector-store clients have client-level suites (`lib/crewai/tests/rag/chromadb/test_client.py`, `lib/crewai/tests/rag/qdrant/test_client.py`).

**Dimension lens question — "Can you switch from Postgres to SQLite with a config change?"**
For the vector-backed subsystems, yes in spirit: switching LanceDB → Qdrant-edge → a custom remote service is a one-line `storage=` string, a dict embedder spec, or a registered factory (`lib/crewai/src/crewai/memory/unified_memory.py:232-251`); ChromaDB ↔ Qdrant is a config-object swap (`lib/crewai/src/crewai/rag/factory.py:54-76`). For relational persistence specifically, no: the only SQL store in the package is the kickoff-output SQLite database and it is not behind an interface (`lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19-27`). No evidence found of any Postgres-capable relational adapter anywhere in the source (searched `postgres`, `sqlite`, `psycopg` across `lib/`; only sqlite3 usage in the kickoff store and type stubs in dev dependencies).

## Architectural Decisions

1. **Constructor-as-factory for LLM selection.** Rather than a separate builder, `LLM.__new__` inspects `provider=`/model-prefix/model-patterns and *returns an instance of a different class* (native SDK completion or LiteLLM-backed `LLM`) (`lib/crewai/src/crewai/llm.py:393-512`). Callers get one entry point; the cost is magic behavior in `__new__` and reliance on hard-coded model-name constants/patterns (`lib/crewai/src/crewai/llm.py:514-519`).
2. **Protocol-based seams instead of inheritance for storage.** `StorageBackend` and `BaseClient` are `runtime_checkable` Protocols, so third-party backends need not inherit framework classes (`lib/crewai/src/crewai/memory/storage/backend.py:44`, `lib/crewai/src/crewai/rag/core/base_client.py:66-67`).
3. **Two distinct extension registries matched to dispatch shape.** Memory/knowledge use a *single-default* setter (one global factory consulted first, returning `None` to defer); RAG uses a *per-provider-keyed* registry because `create_client` already dispatches on `config.provider`. Both rationales are documented inline (`lib/crewai/src/crewai/rag/factory.py:15-19`).
4. **Optional heavy dependencies with graceful degradation.** litellm is lazy-loaded and optional (`lib/crewai/src/crewai/llm.py:113-156,493-510`); chromadb/qdrant modules are imported through a `require()` helper at factory time (`lib/crewai/src/crewai/rag/factory.py:58-76`); aiocache absence disables the file store rather than crashing (`lib/crewai/src/crewai/utilities/file_store.py:22-30`).
5. **Telemetry isolation over global OTel takeover.** `set_tracer()` deliberately avoids installing a global `TracerProvider` so host-app instrumentation isn't hijacked; spans come from a private provider (`lib/crewai/src/crewai/telemetry/telemetry.py:173-191`).

## Notable Patterns

- **Adapter hierarchy with shared base behavior**: all native LLM providers extend `BaseLLM`, inheriting event emission (`_emit_call_started_event` etc., `lib/crewai/src/crewai/llms/base_llm.py:552-719`), stop-word truncation (`:455-495`), multimodal message formatting (`:852-901`), and hook dispatch (`:988-1102`) so adapters only implement provider deltas.
- **Specialization by inheritance for OpenAI-compatible vendors**: `OpenAICompatibleCompletion(OpenAICompletion)` and `SnowflakeCompletion(OpenAICompletion)` reuse the OpenAI adapter wholesale (`lib/crewai/src/crewai/llms/providers/openai_compatible/completion.py:113`, `.../snowflake/completion.py:49`).
- **TypedDict-discriminated provider specs**: embedding configuration is a union of per-provider TypedDicts validated before dynamic import (`lib/crewai/src/crewai/rag/embeddings/types.py:32-72`; import validation via `import_and_validate_definition`, `factory.py:253`).
- **Dotted-path registry**: `PROVIDER_PATHS` maps provider name → `module.Class` string, deferring imports until build time (`lib/crewai/src/crewai/rag/embeddings/factory.py:90-110`).
- **Call-scoped overrides via contextvars**: stop/stream overrides and RAG client selection use `contextvars.ContextVar`, keeping swaps concurrency-safe (`lib/crewai/src/crewai/llms/base_llm.py:79-87`, `lib/crewai/src/crewai/rag/config/utils.py:26`).
- **Transport-level interception**: LLM request/response hooks are implemented by swapping the httpx transport rather than wrapping call methods (`lib/crewai/src/crewai/llms/hooks/transport.py:54-112`).

## Tradeoffs

- **One magic entry point vs explicit builders**: `LLM(model=...)` hiding seven concrete classes simplifies adoption but makes routing correctness depend on maintained model lists; unmatched models silently drop to the LiteLLM path (`lib/crewai/src/crewai/llm.py:468-471`), which changes streaming/auth behavior without an error.
- **Global mutable seams vs dependency injection**: `set_*_factory` functions avoid threading arguments through every constructor (documented rationale in `lib/crewai/src/crewai/memory/storage/factory.py:6-14`) but create hidden global state; tests must remember to reset, and parallel suites share the same slot.
- **Protocols vs base-class contracts**: Protocol typing maximizes third-party freedom but provides no shared code and only structural checks; `isinstance` against `@runtime_checkable` verifies method presence, not signatures.
- **Broad native coverage vs maintenance cost**: shipping seven native SDK adapters duplicates retry/streaming/tool-parsing logic per vendor (each `completion.py` is large), traded against escaping litellm's abstraction leaks (motivation stated in `lib/crewai/src/crewai/llms/base_llm.py:153-155`).

## Failure Modes / Edge Cases

- **Silent search degradation**: `KnowledgeStorage.search/save/reset` catch all exceptions, log, and return empty results or continue (`lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:85-89,100-103`) — a broken vector backend looks like "no knowledge found."
- **ImportError masking real bugs**: any exception during native-provider construction is re-raised as `ImportError("Error importing native provider: ...")` regardless of cause (`lib/crewai/src/crewai/llm.py:488-491`).
- **Missing litellm dead-end**: unprefixed/unmatched models fail hard with install instructions when litellm is absent (`lib/crewai/src/crewai/llm.py:494-510`).
- **Stale env snapshot for locks**: `_REDIS_URL` is captured at module import; changing `REDIS_URL` after import has no effect (`lib/crewai-core/src/crewai_core/lock_store.py:33`), and backend swapping mid-flight is acknowledged unsynchronized (`:48-51`).
- **Embedding-dimension drift**: cross-store dimension mismatch raises a purpose-built, actionable error explaining the default-embedder change and remediation (`lib/crewai/src/crewai/memory/storage/backend.py:11-41`); knowledge storage detects the same condition heuristically by message text (`knowledge_storage.py:124-134`).
- **Factory-decline contract**: a memory factory must return `None` for unrecognized specs or it will shadow built-ins for every string spec (`lib/crewai/src/crewai/memory/storage/factory.py:40-43`).
- **File-store silent disable**: if aiocache is missing, file features no-op with only a debug log (`lib/crewai/src/crewai/utilities/file_store.py:27-30`).

## Future Considerations

- Introduce an interface (or at least a factory seam mirroring memory/knowledge/RAG) for the kickoff-output store, enabling Postgres/MySQL deployments — currently impossible without patching (`lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:19`).
- Make the product-telemetry exporter endpoint configurable for self-hosted collectors, or route it through the existing event bus so users can substitute sinks (`lib/crewai/src/crewai/telemetry/telemetry.py:142-149`).
- Allow the file cache backend to be selected (Redis/disk) via the established `set_*_backend` idiom instead of the hard-coded memory Cache (`lib/crewai/src/crewai/utilities/file_store.py:26`).
- Consider scoping `set_memory_storage_factory`/`set_knowledge_storage_factory`/`set_lock_backend` to contexts (like the RAG ContextVar) to support multi-tenant processes safely.
- Replace heuristic string matching (`"dimension mismatch" in str(e).lower()`) with typed exceptions crossing the RAG boundary (`lib/crewai/src/crewai/knowledge/storage/knowledge_storage.py:124`).

## Questions / Gaps

- No evidence found of a sandbox abstraction beyond E2B-specific tool classes; agents explicitly deprecate built-in code interpretation in favor of external services ("Use dedicated sandbox services like E2B or Modal", `lib/crewai/src/crewai/agent/core.py:378`), but no Modal adapter exists in-tree (searched `sandbox`, `modal`, `code interpreter` under `lib/`).
- Queue backends: none found. There is no queue/messaging adapter surface in the source (searched `queue`, `broker`, `celery`, `kafka` under `lib/crewai/src`); asynchronous fan-out happens through the in-process event bus executor (`lib/crewai/src/crewai/events/event_bus.py:166-210`).
- Whether `to_config_dict()` round-trips are covered by tests for every provider was not verified; only the mechanism itself was confirmed (`lib/crewai/src/crewai/llms/base_llm.py:292-312`).

---

Generated by `21.02-provider-and-backend-adapters` against `crewai`.
