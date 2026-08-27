# Source Analysis: letta

## 19.03 Adapter and Interop Boundary Design

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python / FastAPI, Pydantic, OpenAI/Anthropic/Gemini SDKs |
| Analyzed | 2026-08-27 |

## Summary

Letta treats LLM protocol interop as a **core-adapter hybrid** rather than a pure plugin system. A stable abstract core (`LLMClientBase`, `LettaLLMAdapter`) normalizes all providers to OpenAI `ChatCompletionResponse` (`letta/schemas/openai/chat_completion_response.py:158`), while a closed factory (`letta/llm_api/llm_client.py:32`) and hard-coded adapter dispatch select the implementation. MCP (Model Context Protocol) is the only truly swappable runtime interop boundary with typed configs and OAuth, registered via REST (`letta/server/rest_api/routers/v1/mcp_servers.py:36`). LLM providers are not swappable without code changes: adding a provider requires editing `ProviderType` enum, the factory `match`, and adapter branching. Conformance testing is limited to error-mapping and usage-parsing tests, not interface contracts. Documentation is code-centric with no explicit interop-boundary guide.

## Rating

**Score: 5 / 10 — Present but inconsistent, weakly documented, fragile**

**Rationale:** Clear core abstractions (`LLMClientBase`, `LettaLLMAdapter`, `ModelSettingsUnion`) and a normalized response type exist with operational safeguards (stream cancellation, provider trace, fallback token counting). However, adapters are **not runtime-pluggable** for LLM protocols — the `LLMClient.create` factory and `SimpleLLMStreamAdapter`/`LettaLLMStreamAdapter` provider lists are hard-coded and require core edits to extend. MCP demonstrates a more mature adapter pattern but is isolated to tool interop. Conformance tests cover error translation only; there is no contract test ensuring every `LLMClientBase` subclass satisfies `build_request_data`/`stream_async`/`handle_llm_error`. Boundaries are undocumented beyond docstrings.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core protocol abstraction — base client | `LLMClientBase` defines `@abstractmethod` contract: `build_request_data`, `request_async`, `convert_response_to_chat_completion`, `stream_async`, `handle_llm_error`, with telemetry hooks | `letta/llm_api/llm_client_base.py:26-398` |
| Core protocol abstraction — error normalization | Base `handle_llm_error` maps `httpx.RemoteProtocolError` → `LLMConnectionError(ErrorCode.INTERNAL_SERVER_ERROR)` | `letta/llm_api/llm_client_base.py:368-397` |
| Core protocol abstraction — provider enum | `ProviderType` enumerates 22 providers (`anthropic`, `openai`, `google_ai`, `sglang`, `xai`, `bedrock`, etc.) — single source of truth for adapter dispatch | `letta/schemas/enums.py:53-78` |
| Core protocol abstraction — LLM config | `LLMConfig.model_endpoint_type` Literal enumerates supported `model_endpoint_type` values; `model_wrapper`, `context_window`, `put_inner_thoughts_in_kwargs`, `tool_call_parser` | `letta/schemas/llm_config.py:31-60` |
| Core protocol abstraction — unified response | `ChatCompletionResponse` with `UsageStatistics` + `Choice.tool_calls` is the canonical interop DTO all clients must produce | `letta/schemas/openai/chat_completion_response.py:158-170` |
| Core protocol abstraction — typed settings | `ModelSettingsUnion` discriminated union (14 provider-specific settings) with `_to_legacy_config_params()` → `LLMConfig` | `letta/schemas/model.py:550-568` |
| Adapter — abstract adapter | `LettaLLMAdapter(ABC)` defines `async invoke_llm(...) -> AsyncGenerator[LettaMessage]` + `supports_token_streaming`, `aclose`, `log_provider_trace`; holds `llm_client`, `llm_config`, `usage`, `tool_call` state | `letta/adapters/letta_llm_adapter.py:16-131` |
| Adapter — blocking mode | `LettaLLMRequestAdapter.invoke_llm` stores `request_data`, calls `llm_client.request_async`, converts via `convert_response_to_chat_completion`, extracts reasoning/tool_calls/usage/logprobs | `letta/adapters/letta_llm_request_adapter.py:24-103` |
| Adapter — streaming mode | `LettaLLMStreamAdapter.invoke_llm` instantiates `OpenAIStreamingInterface` or `AnthropicStreamingInterface` based on `model_endpoint_type`, calls `llm_client.stream_async`, delegates to `interface.process` | `letta/adapters/letta_llm_stream_adapter.py:74-207` |
| Adapter — simplified variants | `SimpleLLMRequestAdapter` overrides reasoning/content split (`content` vs `reasoning_content`) and sets telemetry via `set_telemetry_context` + `request_async_with_telemetry` | `letta/adapters/simple_llm_request_adapter.py:12-121` |
| Adapter — simplified streaming | `SimpleLLMStreamAdapter` extends `LettaLLMStreamAdapter`; adds Gemini support and parallel tool-call accumulation (`_tool_calls_acc: dict[int, dict]`) with provider dispatch on `ProviderType` | `letta/adapters/simple_llm_stream_adapter.py:24-139` |
| Adapter — SGLang native | `SGLangNativeAdapter(SimpleLLMRequestAdapter)` bypasses OpenAI chat API: tokenizes via `HF AutoTokenizer.apply_chat_template(tokenize=True)` → `SGLangNativeClient.generate(input_ids)` → parses `output_ids`/`output_token_logprobs` | `letta/adapters/sglang_native_adapter.py:266-411` |
| Adapter — streaming interfaces | Per-provider streaming parsers: `OpenAIStreamingInterface`, `SimpleOpenAIStreamingInterface`, `AnthropicStreamingInterface`, `GeminiStreamingInterface` each implement `process`, `get_tool_call_object(s)`, `get_usage_statistics`, `get_content` | `letta/interfaces/openai_streaming_interface.py:78-577` |
| Factory — closed dispatch | `LLMClient.create(provider_type: ProviderType) -> LLMClientBase` uses `match/case` with explicit imports per provider; default falls through to `OpenAIClient` | `letta/llm_api/llm_client.py:10-145` |
| Extension point — plugin system | `plugins/plugins.py:16-25` defines `DEFAULT_PLUGINS = {"summarizer": SummarizerProtocol, "experimental_check": func}` loaded via `settings.plugin_register_dict` (`SERVANT; separated module:path` syntax) — **not used for LLM providers** | `letta/plugins/plugins.py:16-42` |
| Extension point — settings plugin register | `Settings.plugin_register_dict` parses `LETTA_PLUGIN_REGISTER="summarizer=...;experimental_check=..."` — only two plugin slots, no generic adapter registry | `letta/settings.py:496-502` |
| Interop boundary — MCP configs | Typed MCP union `MCPServerUnion = StdioMCPServer|SSEMCPServer|StreamableHTTPMCPServer` with `MCPServerType` enum (`sse`, `stdio`, `streamable_http`), token sanitization, templated env vars `{{ VAR | default }}` | `letta/functions/mcp_client/types.py:36-296`, `letta/schemas/mcp_server.py:17-88` |
| Interop boundary — MCP REST | CRUD + `POST /{id}/tools/{tool_id}/run` + `GET /connect/{id}` SSE OAuth flow (`OauthStreamEvent`); illustrates swappable external protocol with lifecycle (`get_mcp_client`, `connect_to_server`, `cleanup`) | `letta/server/rest_api/routers/v1/mcp_servers.py:36-310` |
| Interop boundary — OpenAI compat API | `POST /v1/{agent_id}/chat/completions` enforces `stream=true`, validates `llm_config.model_endpoint_type == "openai"` — thin proxy over agent loop | `letta/server/rest_api/routers/openai/chat_completions/chat_completions.py:28-132` |
| Interop boundary — Anthropic proxy | `POST /anthropic/v1/messages` forwards to `https://api.anthropic.com` via `httpx.AsyncClient`, injects memory context via `inject_memory_context`, persists via `persist_messages_background` | `letta/server/rest_api/routers/v1/anthropic.py:32-266` |
| Interop boundary — chat completions router | `POST /chat/completions` wraps `StreamingService.create_agent_stream_openai_chat_completions` — Letta-native chat endpoint | `letta/server/rest_api/routers/v1/chat_completions.py:22-147` |
| Hard-coded adapter dispatch (fragility) | `LettaLLMStreamAdapter` hard-codes `if model_endpoint_type in [anthropic, bedrock, minimax]: AnthropicInterface elif in [openai, openrouter, baseten, fireworks]: OpenAIInterface else raise` — missing providers throw at runtime | `letta/adapters/letta_llm_stream_adapter.py:97-123` |
| Hard-coded adapter dispatch (expanded) | `SimpleLLMStreamAdapter` expands list to `deepseek, zai, chatgpt_oauth` + Gemini branch `google_ai/google_vertex`; still exhaustive match — any new `ProviderType` (e.g., `groq`, `together`) hits `ValueError` | `letta/adapters/simple_llm_stream_adapter.py:81-139` |
| Operational safeguard — stream cancellation | `SimpleLLMStreamAdapter.invoke_llm` binds `get_cancellation_event_for_run(run_id)`, passes to interface; `OpenAIStreamingInterface.process` catches `CancelledError/RunCancelledException` and checks `cancellation_event.is_set()` | `letta/adapters/simple_llm_stream_adapter.py:77-128`, `letta/interfaces/openai_streaming_interface.py:807-829` |
| Operational safeguard — provider trace | Both adapter types log `ProviderTrace` via `safe_create_task(telemetry_manager.create_provider_trace_async(...))` gated by `settings.track_provider_trace` | `letta/adapters/letta_llm_stream_adapter.py:211-273`, `letta/adapters/simple_llm_stream_adapter.py:256-321` |
| Operational safeguard — telemetry context | `LLMClientBase.set_telemetry_context` + `request_async_with_telemetry` + `log_provider_trace_async` provide request/response/error logging with actor/org/run correlation | `letta/llm_api/llm_client_base.py:53-198` |
| Conformance test — stream error mapping | Tests mutate `AnthropicClient.stream_async` to raise `APIStatusError(500)`, `413 request_too_large`, `httpx.ReadError/WriteError` and assert `LLMServerError`, `ContextWindowExceededError`, `LLMConnectionError` | `tests/adapters/test_letta_llm_stream_adapter_error_handling.py:28-176` |
| Conformance test — usage parsing | `test_usage_parsing.py` verifies `SimpleLLMRequestAdapter` correctly normalizes `cached_input_tokens`, `reasoning_tokens` across OpenAI/Anthropic/Gemini prefix-cache semantics | `tests/test_usage_parsing.py:1-457` |
| Conformance test — LLM client batch | `test_llm_clients.py` mocks `anthropic.AsyncAnthropic.beta.messages.batches.create` and validates payload shape + `count_tokens` empty-message handling | `tests/test_llm_clients.py:71-258` |
| Provider-specific implementation | `OpenAIClient.build_request_data`, `build_request_data_responses`, `stream_async`, `stream_async_responses` (Responses API), `handle_llm_error` mapping `401/403/404/402 -> LLMBadRequestError` | `letta/llm_api/openai_client.py:175-1071` |
| Provider-specific implementation | `AnthropicClient.build_request_data`, `request_async`, `stream_async`, `convert_response_to_chat_completion` with `count_tokens` sanitization | `letta/llm_api/anthropic_client.py:63-1187` |
| Provider-specific implementation | `GoogleVertexClient/Gemini` streaming uses `AsyncIterator[GenerateContentResponse]` not `AsyncStream[ChatCompletionChunk]` — divergent streaming type | `letta/llm_api/google_vertex_client.py:205`, `letta/adapters/simple_llm_stream_adapter.py:145-146` |
| No formal adapter conformance suite | Search found zero `protocol.*adapter` or `conformance` docs; `grep -r "handle_llm_error\|LLMError"` shows per-client overrides but no shared contract test asserting every `ProviderType` satisfies `LLMClientBase` | `grep llm_api/*: handled per-file, no aggregate suite` |

## Answers to Dimension Questions

**1. Are protocols core or adapter-layer?**

Both. The canonical wire DTO is core: every LLM client must convert provider JSON → `ChatCompletionResponse` (`letta/schemas/openai/chat_completion_response.py:158`) and every adapter must yield `LettaMessage` (`letta/adapters/letta_llm_adapter.py:64`). Provider-specific request building (`build_request_data`) and streaming (`stream_async`) live in adapter-layer `LLMClientBase` subclasses (`letta/llm_api/anthropic_client.py:63`, `letta/llm_api/openai_client.py:175`, `letta/llm_api/google_vertex_client.py:54`). SGLang is the clearest adapter-layer case: it replaces `request_async` with tokenized `input_ids` → `/generate` (`letta/adapters/sglang_native_adapter.py:315-320`). MCP is adapter-layer with core-typed schemas (`letta/functions/mcp_client/types.py:36`). The fragility is that `ProviderType` and `LLMConfig.model_endpoint_type` (`letta/schemas/enums.py:53`, `letta/schemas/llm_config.py:31`) are core enums — any protocol addition requires core changes, so standards are core-governed, not optional adapters.

**2. Can adapters be added without core changes?**

**No for LLM providers; Yes for MCP servers.**

- **LLM**: Adding a new provider requires: (a) extending `ProviderType` (`letta/schemas/enums.py:53`), (b) adding a `Literal` to `LLMConfig.model_endpoint_type` (`letta/schemas/llm_config.py:31`), (c) adding a `case` to `LLMClient.create` (`letta/llm_api/llm_client.py:32`), (d) adding branches to `LettaLLMStreamAdapter` and `SimpleLLMStreamAdapter` (`letta/adapters/letta_llm_stream_adapter.py:97`, `letta/adapters/simple_llm_stream_adapter.py:81`), and (e) adding a `ModelSettings` subclass to `ModelSettingsUnion` (`letta/schemas/model.py:550`). No registry or discovery exists; `settings.plugin_register_dict` (`letta/settings.py:496`) only supports `summarizer` and `experimental_check`. There is no `importlib` auto-discovery for `LLMClientBase`.

- **MCP**: New MCP servers are runtime-swappable via `POST /v1/mcp-servers` (`letta/server/rest_api/routers/v1/mcp_servers.py:36`) with typed `CreateMCPServerUnion` and OAuth (`handle_oauth_flow`). Tools are resynced without restart (`PATCH /{id}/refresh`). This path is isolated to MCP tool interop, not LLM generation.

**3. Are adapters tested for conformance?**

**Partially — error-mapping and usage tests, not interface conformance.**

- `tests/adapters/test_letta_llm_stream_adapter_error_handling.py:28-363` is the strongest adapter test: it fakes `AnthropicClient.stream_async` and asserts provider errors map to `LLMServerError`/`ContextWindowExceededError`/`LLMConnectionError`, plus an empty-response test for `LLMEmptyResponseError`. This tests the adapter → error boundary contract.

- `tests/test_usage_parsing.py:4-457` validates that `SimpleLLMRequestAdapter` extracts `cached_input_tokens`/`reasoning_tokens` correctly across OpenAI/Anthropic/Gemini, covering provider-divergent usage semantics.

- `tests/test_llm_clients.py:71-258` tests `AnthropicClient.send_llm_batch_request_async` and `count_tokens` sanitization (empty string → `"."`, stripping trailing whitespace).

- **Missing**: no test asserts that every `ProviderType` has a `LLMClientBase` subclass satisfying all abstract methods; no contract test for `build_request_data` → `request_async` → `convert_response_to_chat_completion` round-trip per provider; no streaming interface conformance (e.g., that every interface exposes `process`/`get_tool_call_objects`/`get_usage_statistics`). SGLang adapter has no checked-in test.

**4. Are interop boundaries documented?**

**Weakly.** The only boundary docs are `letta/plugins/README.md:1-22` (plugin registration syntax) and docstrings on `LettaLLMAdapter` (`letta/adapters/letta_llm_adapter.py:16`) and `LLMClientBase` (`letta/llm_api/llm_client_base.py:26`). OpenAPI schemas expose `ChatCompletionResponse` and `MCPServerUnion` (`letta/schemas/openai/chat_completion_response.py:158`, `letta/schemas/mcp_server.py:64-88`) but there is no `docs/` guide defining supported protocols, versioning, or extension steps. MCP types are well-commented (`letta/functions/mcp_client/types.py:160`) and routers are operation-id annotated, but there is no deprecation policy or explicit statement of which protocols are core vs. adapter-layer. `grep` for `interop`, `boundary`, `conformance` yields no documentation artifact.

## Architectural Decisions

| Decision | Evidence | Consequence |
|----------|----------|-------------|
| Normalize to OpenAI `ChatCompletionResponse` core DTO | `letta/schemas/openai/chat_completion_response.py:158`, all `convert_response_to_chat_completion` overrides (`letta/llm_api/openai_client.py:856`, `letta/llm_api/anthropic_client.py:1187`) | Simplifies agent loop (single response type) but forces every provider through an OpenAI shim; SGLang must manually construct `ChatCompletionResponse` (`letta/adapters/sglang_native_adapter.py:369-389`) |
| Factory `LLMClient.create` over registry | `letta/llm_api/llm_client.py:10-145` `match provider_type` | Explicit control but Closed-for-extension: OCP violated, each provider touches factory |
| Split streaming interfaces per provider family | `letta/interfaces/openai_streaming_interface.py:78`, `anthropic_streaming_interface.py`, `gemini_streaming_interface.py`; selected by adapter (`letta/adapters/letta_llm_stream_adapter.py:97`) | Accurate per-provider SSE handling + fallback token counting (`num_tokens_from_messages`) at cost of duplicated lifecycle code |
| `SimpleLLM*` vs legacy `LettaLLM*` adapter pair | `letta/adapters/simple_llm_request_adapter.py:12` ("No inner thoughts in kwargs") vs `letta/adapters/letta_llm_request_adapter.py:15`; same for stream | Incremental migration — simpler content model (`TextContent` vs `ReasoningContent`) but doubles branches to maintain |
| `LLMConfig` deprecated wrapper around `ModelSettings` | `letta/schemas/llm_config.py:355-495` `_to_model_settings()` + `letta/schemas/model.py:550` union with `_to_legacy_config_params()` | Two config models to keep compat; new code must handle both `LLMConfig` and `ModelSettingsUnion` |
| MCP as discriminated union with templated env injection | `letta/functions/mcp_client/types.py:14-16` `TEMPLATED_VARIABLE_REGEX`, `resolve_environment_variables` | Enables secret-templated headers (`{{ API_KEY | default }}`) but stdio path is disabled by default (`letta/settings.py:45` `mcp_disable_stdio=True`) |
| Provider trace via fire-and-forget | `letta/adapters/letta_llm_request_adapter.py:105-147` `safe_create_task(create_provider_trace_async)` | Observable without blocking LLM path; requires `settings.track_provider_trace` and actor context or trace is dropped |

## Notable Patterns

- **Adapter + Interface decomposition**: LLM invocation is split into Adapter (telemetry, request lifecycle, finish_reason) → LLMClient (transport, `request_async`/`stream_async`) → StreamingInterface (chunk parsing, token/tool-call accumulation). Example: `SimpleLLMStreamAdapter.invoke_llm` → `llm_client.stream_async` → `interface.process(stream)` (`letta/adapters/simple_llm_stream_adapter.py:142-179`).
- **Per-index tool-call accumulation**: `SimpleOpenAIStreamingInterface._tool_calls_acc: dict[int, dict[str,str]]` with `_tool_call_start_order: list[int]` to support parallel tool calls ordered by provider `index` (`letta/interfaces/openai_streaming_interface.py:631-686`). Anthropic parallel variant uses `SimpleAnthropicStreamingInterface`.
- **Graceful stream cancellation**: Shared `cancellation_event` from `get_cancellation_event_for_run(run_id)` passed to interface; both `SimpleLLMStreamAdapter` and `OpenAIStreamingInterface` check `is_set()` in `finally` and swallow `CancelledError` (`letta/adapters/simple_llm_stream_adapter.py:77`, `letta/interfaces/openai_streaming_interface.py:807`).
- **Fallback token counting**: When `chunk.usage` is absent (LMStudio, proxy providers), `is_openai_proxy` path uses `num_tokens_from_messages` / `num_tokens_from_functions` with `create_token_counter(model_endpoint_type="openai")` (`letta/interfaces/openai_streaming_interface.py:99-250`, `239-250`).
- **SGLang shim pattern**: Construct `output_ids` → `ChatCompletionResponse` with `ChoiceLogprobs` + inline tool-call parser fallback (`_parse_glm47_tool_calls` regex on `<tool_call>`) when `sglang` package absent (`letta/adapters/sglang_native_adapter.py:181-263`).

## Tradeoffs

| Tradeoff | What is gained | What is sacrificed |
|----------|----------------|--------------------|
| Single canonical DTO vs native protocol fidelity | Agent loop can ignore provider differences; `LettaUsageStatistics` normalization via `normalize_cache_tokens`/`normalize_reasoning_tokens` (`letta/schemas/usage.py` via `letta/adapters/letta_llm_request_adapter.py:97-98`) | Native provider features (e.g., Anthropic `BetaMessageBatch`, Gemini `GenerateContentResponse`) are shunted through legacy endpoints; information loss in conversion |
| Exhaustive `match` in factory vs open registry | Type-checked, grep-friendly provider list; IDE exhaustiveness | Cannot ship a new provider as an out-of-tree pip package — requires fork or upstream enum edit |
| Hard-coded streaming dispatch vs capability flag | Simple `if provider_type in [...]` (`letta/adapters/simple_llm_stream_adapter.py:81`) | Silent omission risk: adding `ProviderType.groq` to `enums.py` without updating adapter dispatch yields runtime `ValueError` |
| Blocking adapter yields `None` sentinel | Uniform `AsyncGenerator[LettaMessage | None]` lets agent loop `async for` both modes (`letta/adapters/letta_llm_request_adapter.py:102` vs `letta_llm_stream_adapter.py:154`) | Blocking path yields a dummy chunk, forcing callers to handle `None` |
| MCP swappability limited to tool interop | External tool servers are runtime-extensible with OAuth and encrypted token storage (`letta/schemas/mcp_server.py:173-174`) | LLM generation protocols remain non-extensible notwithstanding MCP maturity |

## Failure Modes / Edge Cases

- **Unhandled provider in streaming branch**: `LettaLLMStreamAdapter` and `SimpleLLMStreamAdapter` raise `ValueError("Streaming not supported for provider {type}")` for any `ProviderType` not in their hard-coded lists (`letta/adapters/letta_llm_stream_adapter.py:123`, `letta/adapters/simple_llm_stream_adapter.py:139`). Example: registering a `Groq` model with `supports_streaming=true` would fail at invocation, not at config validation.

- **Empty streaming response swallowed**: Fixed by regression test `test_letta_llm_stream_adapter_raises_empty_response_error_for_anthropic` — previously Opus 4.6 empty content (`message_start` → `message_stop` with no blocks) completed with `stop_reason=end_turn`. Now raises `LLMEmptyResponseError` (`tests/adapters/test_letta_llm_stream_adapter_error_handling.py:299-363`).

- **413 vs 400 context-window ambiguity**: `AnthropicClient.handle_llm_error` maps both `413` and `"request_too_large"` to `ContextWindowExceededError` (`tests/adapters/test_letta_llm_stream_adapter_error_handling.py:179-206`); `GoogleVertexClient` inspects message substrings (`"tokens allowed"`, `"context"` + `"exceeded"`) (`tests/adapters/test_letta_llm_stream_adapter_error_handling.py:209-238`). Keyword matching is fragile to provider message changes.

- **Telemetry silent drop**: Both `LettaLLMRequestAdapter.log_provider_trace` and `SimpleLLMRequestAdapter` skip tracing if `step_id is None or actor is None` (`letta/adapters/letta_llm_request_adapter.py:118`) or `settings.track_provider_trace is False`. Background compilation paths (batch) may lose traces without warning apart from `logger.warning("Skipping telemetry: actor is None")` (`letta/llm_api/llm_client_base.py:113`).

- **SGLang tokenizer cache & path resolution**: `_tokenizer_cache: dict[str, Any]` is global mutable (`letta/adapters/sglang_native_adapter.py:36`); `_resolve_tokenizer_path` parses `model_name.split("/")` to extract local `/opt/...` (`letta/adapters/sglang_native_adapter.py:52-69`) — fails for Windows paths or unconventional `handle` formats.

- **Tool-call ID loss on chunk boundary**: `OpenAIStreamingInterface` compensates via `last_flushed_function_id` fallback (`letta/interfaces/openai_streaming_interface.py:171-176`) but still `continue`s emitting `tool_call` deltas without `id` (`:945-946` `if resolved_id is None: continue`), risking out-of-order delivery if provider omits `id` on continuation chunks.

- **Proxy provider double-counting**: `fallback_input_tokens` is additive (`num_tokens_from_messages + num_tokens_from_functions`) but real `chunk.usage` is cumulative — the `get_usage_statistics` branch picks `input_tokens if input_tokens else fallback` (`letta/interfaces/openai_streaming_interface.py:206-227`), so mixed fallback + real usage on same stream could misreport.

## Future Considerations

- **Introduce a runtime adapter registry** (`LLMClientRegistry.register(provider_type, factory)`) backed by `importlib` entry-points and a `letta_schemas` extension table, so third-party providers can `pip install letta-provider-foo` without editing `letta/schemas/enums.py:53` or `letta/llm_api/llm_client.py:32`. Pair with a contract test that instantiates every registered `ProviderType`.

- **Replace hard-coded streaming dispatch with capability flag** on `LLMClientBase` (e.g., `supports_streaming: bool` or `streaming_interface_cls`) so `SimpleLLMStreamAdapter` (`letta/adapters/simple_llm_stream_adapter.py:81`) becomes `self.llm_client.get_streaming_interface(...)` instead of an exhaustive `if`.

- **Add adapter contract suite**: property-based tests asserting every `LLMClientBase` subclass returns a `ChatCompletionResponse` with valid `usage` and that every streaming interface satisfies `AsyncGenerator[LettaMessage]` with `get_tool_call_objects`/`get_usage_statistics` — close the gap noted in `tests/test_llm_clients.py` and `tests/adapters/test_letta_llm_stream_adapter_error_handling.py`.

- **Document interop boundaries** (protocol support matrix, versioning, deprecation, extension cookbook) covering: (1) which `ProviderType` are Tier-1 vs experimental, (2) steps to add a provider without core changes (once registry exists), (3) MCP transport capabilities and auth templating (`letta/functions/mcp_client/types.py:14`). Link from `letta/llm_api/README` and `letta/plugins/README.md:1`.

- **Unify config models**: deprecate `LLMConfig` shim fully (`letta/schemas/llm_config.py:19-25` marks deprecated) and standardize on `ModelSettingsUnion` (`letta/schemas/model.py:550`) plus `ProviderType` to reduce dual-path handling in adapters.

## Questions / Gaps

- **Why is `LLMClient.create` asymmetric to MCP extensibility?** Evidence shows MCP allows runtime server registration while LLM does not; no search found a design doc explaining why LLM providers are not registry-driven. Searched `grep "plugin_register" letta/` and `grep "ProviderType" letta/llm_api/` — no ADR or TODO.

- **Is SGLang `tool_call_parser` intended to generalize?** `letta/adapters/sglang_native_adapter.py:342-345` reads `model_settings.tool_call_parser` only for `ProviderType.sglang`; unclear whether other providers should share the `FunctionCallParser` integration.

- **What is the long-term status of `LettaLLMAdapter` vs `SimpleLLM*`?** `simple_llm_*` comments say "Simplifying assumptions: No inner thoughts in kwargs" (`letta/adapters/simple_llm_request_adapter.py:12`) and `SGLangNativeAdapter` extends `SimpleLLMRequestAdapter`; legacy adapters still used by older agent types (`letta/llm_api/llm_client_base.py:205` `AgentType.memgpt_agent`). No migration plan found.

- **Conformance of Bedrock via Anthropic?** `ProviderType.bedrock` reuses `AnthropicClient` and `AnthropicStreamingInterface` (`letta/llm_api/llm_client.py:54`, `letta/adapters/letta_llm_stream_adapter.py:97`), but `tests/` contain no Bedrock-specific fixture — untested boundary.

- **Observability under dual-write telemetry?** `letta/settings.py:570-592` supports `provider_trace_backend: postgres,socket,clickhouse` multi-backends, but `letta/llm_api/llm_client_base.py:137` fallback `log_provider_trace_async` path is the only examined flow — clickhouse/socket durability under failure not evaluated.

---

Generated by `19.03-adapter-and-interop-boundary-design` against `letta`.
