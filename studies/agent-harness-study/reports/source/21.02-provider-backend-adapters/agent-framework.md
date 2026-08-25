# Source Analysis: agent-framework

## 21.02 Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + C#/.NET (Go SDK lives in a separate repository) |
| Analyzed | 2026-08-25 |

## Summary

The framework treats "backend" as a first-class seam in both languages. Python defines structural (`Protocol`) and nominal (`ABC`) abstractions for chat models, embeddings, session/history stores, workflow checkpoint storage, file stores, shell executors, and sandbox runtimes; concrete provider packages (OpenAI, Anthropic, Gemini, Mistral, Ollama, Bedrock, Foundry, …) implement them and are composed via layered mixins ("Raw + Full-Featured" pattern) rather than conditional branches. .NET delegates the model abstraction to `Microsoft.Extensions.AI`'s `IChatClient` and adds its own `AIAgent`, `ChatHistoryProvider`, and `ICheckpointStore` abstractions, with Cosmos DB / Valkey / Filesystem / Foundry implementations. Configuration is externalized through typed env-var settings with documented precedence (`python/packages/core/agent_framework/_settings.py:190-319`) and, for declarative agents, a YAML-driven provider-type mapping that callers can extend (`python/packages/declarative/agent_framework_declarative/_loader.py:61-125`). Backends are selected at composition time (constructor/DI/YAML load); hot-swapping a live client mid-run is not supported, though several seams accept provider callables for late binding. Adapter tests exist at every layer, from protocol-conformance mocks to per-backend unit test projects. The main gaps are the absence of a generic vector-store abstraction, no in-repo queue abstraction (durable queues live in an external extension repo), and an experimental status on several storage APIs.

## Rating

**8 / 10** — Clear model with explicit interfaces, multiple real implementations per backend type, extensive tests, and externalized configuration. Not a 9–10 because: no generic vector-store abstraction exists (vector search is reached only through hosted tools/context providers), durable queue backends are out-of-tree, several storage adapters (`SessionStore`, `FileSessionStore`, `FileHistoryProvider`) are explicitly marked experimental, and runtime re-selection of a bound backend is composition-time only.

## Evidence Collected

Every entry cites paths relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Chat-client protocol (Python) | `SupportsChatGetResponse` Protocol — structural typing means any class with `get_response` qualifies without inheriting | `python/packages/core/agent_framework/_clients.py:85-201` |
| Chat-client ABC (Python) | `BaseChatClient(SerializationMixin, ABC)`; subclasses implement `_inner_get_response()` | `python/packages/core/agent_framework/_clients.py:217-234` |
| Capability detection protocols | `SupportsCodeInterpreterTool` / `WebSearch` / `ImageGeneration` / `MCP` / `FileSearch` / `Shell` / `GetEmbeddings` let agents probe optional backend features | `python/packages/core/agent_framework/_clients.py:668,698,728,758,789,819,871` |
| Embedding abstraction (Python) | `BaseEmbeddingClient(SerializationMixin, ABC)` | `python/packages/core/agent_framework/_clients.py:926` |
| Model providers (Python) | `OpenAIChatClient` + `RawOpenAIChatClient`; same Raw+Full pattern in Anthropic, Gemini, Mistral, Ollama, Bedrock packages | `python/packages/openai/agent_framework_openai/_chat_client.py:377,3430`; `python/packages/anthropic/agent_framework_anthropic/_chat_client.py:236,1614`; `python/packages/gemini/agent_framework_gemini/_chat_client.py:306,1319`; `python/packages/mistral/agent_framework_mistral/_chat_client.py:298,867`; `python/packages/ollama/agent_framework_ollama/_chat_client.py:292`; `python/packages/bedrock/agent_framework_bedrock/_chat_client.py:227` |
| Layered adapter pattern (Python) | `OpenAIChatClient(FunctionInvocationLayer, ChatMiddlewareLayer, ChatTelemetryLayer, RawOpenAIChatClient)` — function-calling, telemetry, middleware are reusable layers wrapped around any raw client | `python/packages/openai/agent_framework_openai/_chat_client.py:3430-3436` |
| Function-invocation layer (Python) | `FunctionInvocationLayer` decorates any chat client's `get_response` with automatic tool calling | `python/packages/core/agent_framework/_tools.py:3036-3044` |
| Model abstraction (.NET) | `ChatClientAgent(IChatClient chatClient, ...)` accepts any M.E.AI pipeline as its model backend | `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:80,116` |
| Decorator adapters (.NET) | `FoundryChatClient : DelegatingChatClient`; internal cross-cutting clients (message injection, history persistence, approval bypass) wrap any `IChatClient` | `dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:54`; `dotnet/src/Microsoft.Agents.AI/ChatClient/MessageInjectingChatClient.cs`, `PerServiceCallChatHistoryPersistingChatClient.cs` |
| Agent abstraction (.NET) | Abstract `AIAgent` base plus concrete agents across providers (`GitHubCopilotAgent`, `CopilotStudioAgent`, `A2AAgent`) | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs`; `dotnet/src/Microsoft.Agents.AI.GitHub.Copilot/GitHubCopilotAgent.cs:24`; `dotnet/src/Microsoft.Agents.AI.CopilotStudio/CopilotStudioAgent.cs:23`; `dotnet/src/Microsoft.Agents.AI.A2A/A2AAgent.cs:26` |
| Checkpoint storage protocol (Python) | `CheckpointStorage(Protocol)` with save/load/list/delete/get_latest/list_checkpoint_ids | `python/packages/core/agent_framework/_workflows/_checkpoint.py:129-199` |
| Checkpoint implementations (Python) | `InMemoryCheckpointStorage` (testing), `FileCheckpointStorage` (durable), Cosmos-backed storage tested in package tests | `python/packages/core/agent_framework/_workflows/_checkpoint.py:202,249`; `python/packages/azure-cosmos/tests/test_cosmos_checkpoint_storage.py:21,106-226` |
| Checkpoint store SPI (.NET) | `ICheckpointStore<TStoreObject>` interface; abstract `JsonCheckpointStore`; `FileSystemJsonCheckpointStore`, `CosmosCheckpointStore<T>`, `FoundryJsonCheckpointStore` implementations | `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/ICheckpointStore.cs:12`; `JsonCheckpointStore.cs:13`; `FileSystemJsonCheckpointStore.cs:30`; `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosCheckpointStore.cs:24`; `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryJsonCheckpointStore.cs:58` |
| Session/history stores (Python) | `SessionStore` (in-memory, swappable by contract docstring), experimental `FileSessionStore`; `InMemoryHistoryProvider`, `FileHistoryProvider` | `python/packages/core/agent_framework/_sessions.py:1795-1807,1871-1899,2087,2172` |
| History providers (multi-backend) | `RedisHistoryProvider`/`RedisContextProvider`, `CosmosHistoryProvider`, `Mem0ContextProvider`, `CosmosMemoryContextProvider` all subclass core bases | `python/packages/redis/agent_framework_redis/_history_provider.py:24`; `_context_provider.py:45`; `python/packages/azure-cosmos/agent_framework_azure_cosmos/_history_provider.py:38`; `python/packages/mem0/agent_framework_mem0/_context_provider.py:44`; `python/packages/azure-cosmos-memory/agent_framework_azure_cosmos_memory/_context_provider.py:74` |
| History provider SPI (.NET) | Abstract `ChatHistoryProvider` with built-in `InMemoryChatHistoryProvider`; Cosmos and Valkey implementations | `dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:51`; `dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosChatHistoryProvider.cs:41`; `dotnet/src/Microsoft.Agents.AI.Valkey/ValkeyChatHistoryProvider.cs:42` |
| File-store abstraction | `AgentFileStore(ABC)` with `InMemoryAgentFileStore` and containment-hardened `FileSystemAgentFileStore` | `python/packages/core/agent_framework/_harness/_file_access.py:513,623,770` |
| Sandbox/shell executors | `SandboxRuntime(Protocol)` (Hyperlight WASM), `MontyExecuteCodeTool`, `LocalShellTool`, Docker backend, `ShellExecutor(Protocol)` | `python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:90-91`; `python/packages/monty/agent_framework_monty/_execute_code_tool.py:167`; `python/packages/tools/agent_framework_tools/shell/_tool.py:65`; `shell/_docker.py:134`; `shell/_executor_base.py:40` |
| Tracing sink configuration | `configure_otel_providers(..., exporters: list[LogRecordExporter \| SpanExporter \| MetricExporter], ...)` accepts arbitrary OTel exporters; OTLP gRPC/HTTP helpers built-in; sticky disable safeguard | `python/packages/core/agent_framework/observability.py:1300-1310,422-460,1238-1261` |
| Env-based config resolution | `load_settings` TypedDict schema: overrides → `.env` file → `<PREFIX><FIELD>` env vars → defaults, with required/mutually-exclusive validation | `python/packages/core/agent_framework/_settings.py:190-319` |
| Provider env config example | `OpenAIChatClient(model=None → OPENAI_CHAT_MODEL/OPENAI_MODEL, api_key → OPENAI_API_KEY, ...)`; Azure fallbacks `AZURE_OPENAI_*` | `python/packages/openai/agent_framework_openai/_chat_client.py:3441-3519` |
| Declarative provider registry (Python) | `PROVIDER_TYPE_OBJECT_MAPPING` maps YAML `provider:` strings to client classes; `AgentFactory(additional_mappings=..., default_provider="Foundry")` extends it without code changes | `python/packages/declarative/agent_framework_declarative/_loader.py:61-125,191-236` |
| DI-based selection (.NET) | Sample wires the model backend through DI: `builder.Services.AddChatClient(chatClient)`; `AIAgentBuilder(Func<IServiceProvider, AIAgent> innerAgentFactory)` resolves agents from the container; `BuildAIAgent` turns an `IChatClient` pipeline into a `ChatClientAgent` | `dotnet/samples/02-agents/DevUI/DevUI_Step01_BasicUsage/Program.cs:56`; `dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs:26-35,76-87`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientBuilderExtensions.cs:47,79,113,146` |
| Lazy provider loading | Provider namespaces resolve via `importlib.import_module` on first attribute access (e.g., `agent_framework.azure`), keeping optional backends optional | `python/packages/core/agent_framework/azure/__init__.py:27-31` |

## Answers to Dimension Questions

**1. Are backends swappable?**
Yes, at composition time, in both languages. In Python the `Agent` constructor takes any `SupportsChatGetResponse` (`python/packages/core/agent_framework/_agents.py:1882-1884`), and the docstring shows a custom client needs only `get_response` to qualify structurally (`python/packages/core/agent_framework/_clients.py:96-127`). Stores follow the same shape: `SessionStore`'s docstring instructs callers needing TTLs or distributed coordination to "provide another implementation with the same async methods" (`python/packages/core/agent_framework/_sessions.py:1796-1802`), and workflows take any `CheckpointStorage` protocol implementation at build time (`python/packages/core/agent_framework/_workflows/_checkpoint.py:129`). In .NET, `ChatClientAgent` is constructed over an arbitrary `IChatClient` pipeline (`dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:80`), which is exactly how OpenAI/Azure/Anthropic/Foundry backends plug in.

**2. Which backends have multiple implementations?**
- **Model/chat clients**: Python ships OpenAI Responses (`python/packages/openai/agent_framework_openai/_chat_client.py:3430`), OpenAI Chat Completions (`_chat_completion_client.py:1267`), Anthropic (`python/packages/anthropic/agent_framework_anthropic/_chat_client.py:1614`), Gemini (`python/packages/gemini/agent_framework_gemini/_chat_client.py:1319`), Mistral (`...mistral/_chat_client.py:867`), Ollama (`...ollama/_chat_client.py:292`), Bedrock (`...bedrock/_chat_client.py:227`), and Foundry (`python/packages/foundry/agent_framework_foundry/_chat_client.py:929`); .NET has Foundry, AzureAI.Persistent, Anthropic, CopilotStudio, GitHub.Copilot packages under `dotnet/src/`.
- **Checkpoint storage**: In-Memory + File (core, `python/packages/core/agent_framework/_workflows/_checkpoint.py:202,249`; `dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/FileSystemJsonCheckpointStore.cs:30`), Cosmos (`dotnet/src/Microsoft.Agents.AI.CosmosNoSql/CosmosCheckpointStore.cs:24`; python azure-cosmos package), Foundry-hosted (`dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/FoundryJsonCheckpointStore.cs:58`).
- **History/session stores**: in-memory + file (core, `python/packages/core/agent_framework/_sessions.py:1795,2087,2172`), Redis, Cosmos, Mem0 (`python/packages/redis/...`, `python/packages/azure-cosmos/...`, `python/packages/mem0/...`), Valkey + Cosmos + in-memory on .NET.
- **Sandboxes/code execution**: Hyperlight WASM, Monty interpreter, local process shell, Docker container shell (`python/packages/hyperlight/...`, `python/packages/monty/...`, `python/packages/tools/.../shell/_tool.py:65`, `shell/_docker.py:134`).
- **Tracing sinks**: pluggable OTLP gRPC vs HTTP exporters plus caller-supplied exporter lists (`python/packages/core/agent_framework/observability.py:1300-1310`).

**3. Can backends be swapped at runtime?**
Not hot-swapped once an agent/workflow instance holds a backend — the client is fixed at construction (`python/packages/core/agent_framework/_agents.py:1882-1884`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:80`). However, several seams support *late* or *per-call* selection: .NET's `AIAgentBuilder(Func<IServiceProvider, AIAgent>)` defers agent creation to the DI container (`dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs:35`); declarative YAML selects the provider class at load time from a string mapping (`python/packages/declarative/agent_framework_declarative/_loader.py:61-125`); MCP skill sources accept a `session_provider` callable so a replaced underlying session keeps working; and credentials can be supplied as callables (`api_key: str | Callable[[], str | Awaitable[str]]`, `python/packages/openai/agent_framework_openai/_chat_client.py:3446`). Switching Postgres→SQLite-style by config change is approximated but not literal: you change the client class/package (or YAML `provider:` key) while env vars carry endpoint/key/model (`python/packages/openai/agent_framework_openai/_chat_client.py:3463-3481`); there is no single connection-string-style switch across all backend types.

**4. Are adapter implementations tested?**
Extensively. Core tests verify protocol conformance without inheritance via `MockChatClient`/`MockBaseChatClient` fixtures (`python/packages/core/tests/core/conftest.py:108,162`; assertions at `test_clients.py:33-51`). Every backend package has its own suite: Cosmos checkpoint storage behavior including missing-settings failure paths (`python/packages/azure-cosmos/tests/test_cosmos_checkpoint_storage.py:106-226`), Redis providers (`python/packages/redis/tests/test_providers.py`), Mem0 (`python/packages/mem0/tests/test_mem0_context_provider.py`), Anthropic clients (`python/packages/anthropic/tests/test_anthropic_client.py`). Workflow checkpoint encode/decode/validation has dedicated tests (`python/packages/core/tests/workflow/test_checkpoint*.py`). On .NET each provider has a UnitTests project (e.g., `dotnet/tests/Microsoft.Agents.AI.CosmosNoSql.UnitTests/CosmosChatHistoryProviderTests.cs`, `CosmosCheckpointStoreTests.cs`, `dotnet/tests/Microsoft.Agents.AI.Workflows.UnitTests/FileSystemJsonCheckpointStoreTests.cs`), with integration tests separated into `*IntegrationTests` projects.

## Architectural Decisions

1. **Two-tier abstraction strategy**: protocols for structural conformance (`SupportsChatGetResponse`, `CheckpointStorage`, `ShellExecutor`, `SandboxRuntime`) so third parties never inherit framework classes, alongside ABCs (`BaseChatClient`, `AgentFileStore`, `ChatHistoryProvider` in .NET) when shared behavior matters (`python/packages/core/agent_framework/_clients.py:85,217`; `python/packages/tools/agent_framework_tools/shell/_executor_base.py:40`; `dotnet/src/Microsoft.Agents.AI.Abstractions/ChatHistoryProvider.cs:51`).
2. **Reuse of platform abstractions in .NET**: the model seam is Microsoft.Extensions.AI's `IChatClient` (decorated via `DelegatingChatClient`), not a bespoke interface (`dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:54`; `dotnet/src/Microsoft.Agents.AI/ChatClient/ChatClientAgent.cs:80`).
3. **Layers over branching**: cross-cutting concerns (function calling, telemetry, middleware) are composable layers wrapped around "raw" clients, so a new provider inherits them by mixin rather than reimplementing (`python/packages/openai/agent_framework_openai/_chat_client.py:3430-3436`; `python/packages/core/agent_framework/_tools.py:3036`).
4. **Optional backends stay optional**: provider integrations live in separate packages with lazy importlib loading, keeping core dependency-free (`python/packages/core/agent_framework/azure/__init__.py:27-31`).
5. **Typed, env-first configuration**: one shared `load_settings` engine gives every backend identical override→`.env`→env-var→default precedence and required-field errors (`python/packages/core/agent_framework/_settings.py:190-319`).
6. **Declarative selection with controlled extensibility**: YAML provider strings map through a code-owned registry; extension is explicit via `additional_mappings`, defaulting to Foundry (`python/packages/declarative/agent_framework_declarative/_loader.py:198-236`).

## Notable Patterns

- **Raw + Full-Featured split** (e.g., `RawOpenAIChatClient` vs `OpenAIChatClient`) separates wire-format translation from composed behavior (`python/packages/openai/agent_framework_openai/_chat_client.py:377,3430`).
- **Decorator pipelines in .NET**: `DelegatingChatClient` subclasses inject message persistence, message injection, and approval bypass around any inner client (`dotnet/src/Microsoft.Agents.AI/ChatClient/PerServiceCallChatHistoryPersistingChatClient.cs`, `MessageInjectingChatClient.cs`).
- **Capability probing protocols**: agents detect hosted-tool support (web search, code interpreter, file search, shell) instead of type-checking concrete providers (`python/packages/core/agent_framework/_clients.py:668-869`).
- **Compatibility shim base classes**: `JsonCheckpointStore` lets new stores implement only serialization while inheriting listing/lookup semantics (`dotnet/src/Microsoft.Agents.AI.Workflows/Checkpointing/JsonCheckpointStore.cs:13`).
- **Operational safeguards on sinks**: sticky instrumentation disable wins over library auto-setup, protecting opt-out intent (`python/packages/core/agent_framework/observability.py:1238-1261`).

## Tradeoffs

- **Composition-time binding vs runtime agility**: constructor injection keeps graphs simple and testable but means changing backends requires rebuilding the agent or restarting with different DI/YAML inputs; there is no admin surface for live rerouting.
- **Protocol flexibility vs contract drift**: Python's structural typing admits duck-typed clients (`python/packages/core/agent_framework/_clients.py:96-97`), which lowers integration cost but shifts contract verification onto tests/consumers since nothing enforces it at runtime.
- **Registry-in-code for declarative providers**: `PROVIDER_TYPE_OBJECT_MAPPING` is safe and typed but adding a provider requires either shipping it in this package or passing `additional_mappings` programmatically — not pure configuration (`python/packages/declarative/agent_framework_declarative/_loader.py:61-125,198`).
- **Breadth vs uniformity**: two full language stacks must keep abstractions aligned (e.g., Python `CheckpointStorage` protocol vs .NET `ICheckpointStore<TStoreObject>`); naming and capability parity is manual.

## Failure Modes / Edge Cases

- **Missing settings fail loudly and early**: `load_settings` raises `SettingNotFoundError` naming both the parameter and env var, and enforces mutually-exclusive groups (e.g., Redis URL vs credential provider) (`python/packages/core/agent_framework/_settings.py:294-317`; `python/packages/redis/agent_framework_redis/_history_provider.py:55-58`).
- **Unsafe backend substitution is guarded by contract docs**: `SessionStore` warns custom implementations must handle backend key restrictions and parameterized queries themselves (`python/packages/core/agent_framework/_sessions.py:1804-1806`); `FileSystemAgentFileStore` rejects symlink/reparse segments to prevent escape (`python/packages/core/agent_framework/_harness/_file_access.py:770`).
- **Sandbox threading hazards are contained**: Hyperlight sandboxes are thread-confined behind an actor because PyO3 marks them unsendable, preventing cross-thread drop panics (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:101-126`).
- **Telemetry opt-out stickiness**: auto-setup paths cannot silently re-enable instrumentation after `disable_instrumentation()` (`python/packages/core/agent_framework/observability.py:1241-1256`).
- **Experimental storage APIs may churn**: `SessionStore`, `FileSessionStore`, `FileHistoryProvider` carry `@experimental` markers (`python/packages/core/agent_framework/_sessions.py:1871`), signaling possible breaking changes for adopters who swap them in.

## Future Considerations

- Introduce a generic vector-store/RAG retrieval abstraction; today vector search surfaces only as hosted tool protocols (`SupportsFileSearchTool`, `python/packages/core/agent_framework/_clients.py:789`) and per-package context providers (Azure AI Search, Mem0, Cosmos memory), so switching vector databases is not interface-uniform.
- Promote experimental storage APIs (`SessionStore`, `FileHistoryProvider`) to stable so external store implementations have a durable contract.
- Consider a shared conformance test kit (the `MockChatClient` fixture pattern generalized) that backend packages run against their adapters, mirroring the checkpoint-storage behavioral tests already present for Cosmos.
- Document a canonical "swap by environment" story per backend family (model, store, sink) since pieces exist (env settings, YAML mappings, DI) but no single guide ties them together.

## Questions / Gaps

- **Queues/durable execution**: No queue or durable-task abstraction was found inside this repository; the AGENTS.md states Durable Task/Azure Functions integrations are maintained in the external `microsoft/agent-framework-durable-extension` repo (see `python/AGENTS.md`, Azure Integrations section). Search boundary: `grep -r "QueueStorage|class .*Queue"` over `python/packages/*/agent_framework*/` returned only unrelated middleware internals; the Go directory contains only a pointer README (`go/README.md:1-20`), so the Go SDK could not be assessed within isolation rules.
- **Vector DB abstraction**: Searched `VectorStore|vector_store` across `python/packages` and `dotnet/src`; matches are limited to hosted tool payloads and specific integrations (e.g., `python/packages/foundry/agent_framework_foundry/_chat_client.py`), not a swappable interface — concluding "no generic abstraction" with high confidence but noting the possibility of an equivalent expressed purely via `ContextProvider`.
- **Runtime hot-swap**: No evidence found of an API to replace a bound chat client or store on a live agent; conclusion based on constructor signatures cited above plus absence of setter APIs in `_agents.py` and `ChatClientAgent.cs`.

---

Generated by dimension 21.02 (Provider and Backend Adapters) against `agent-framework`.
