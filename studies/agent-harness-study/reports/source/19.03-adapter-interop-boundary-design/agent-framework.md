# Source Analysis: agent-framework

## 19.03 Adapter and Interop Boundary Design

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (monorepo, 35+ Python packages, 39 .NET projects) |
| Analyzed | 2026-08-27 |

## Summary

Agent Framework treats protocols as **adapter-layer extensions**, not core. Core (`python/packages/core`, `dotnet/src/Microsoft.Agents.AI.Abstractions`) defines narrow, stable abstractions — `SupportsAgentRun` (Python) / `AIAgent` (.NET) for agents and `SupportsChatGetResponse` / `BaseChatClient` for chat clients — and every external protocol lives in an isolated package (`a2a`, `ag-ui`, `hosting-mcp`, `hosting-a2a`, `chatkit`, provider packages like `openai`/`anthropic`/`ollama`). New protocols are added by implementing those protocols without modifying core, and are swappable at runtime via constructor injection (`Agent(client=...)`), protocol duck-typing, and middleware extension points. Conformance is verified per-adapter with extensive unit/integration tests but without a single cross-protocol conformance harness. Interop boundaries are explicitly documented per-package (`AGENTS.md`, `README.md`, ADRs) with trade-offs called out (e.g., hosting-mcp's boundary limitations, ChatKit UI ownership).

## Rating

**7/10 — Clear model with tests, explicit interfaces, and operational safeguards**

Rationale: Core abstractions are minimal, protocol-typed, and reused across Python and .NET. All protocol support is outside core in separate installable adapters requiring zero core changes to extend. Runtime swappability is proven via structural protocols and factory tests. Per-adapter tests are thorough (A2A streaming tasks, AG-UI event conversion, MCP argument allowlisting). Gap versus 9-10: no durable cross-protocol conformance matrix, limited chaos/failure injection coverage, and observability across interop boundaries is provider-specific.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core protocol abstraction — agent | `SupportsAgentRun` `@runtime_checkable Protocol` defining `run()` / `create_session()` / `get_session()`; structural duck-typing explicitly documented | `python/packages/core/agent_framework/_agents.py:233-369` |
| Core base agent | `BaseAgent` minimal base class owning `middleware` and `context_providers`; `Agent`/`RawAgent` compose a `BaseChatClient` without embedding protocols | `python/packages/core/agent_framework/_agents.py:374-472`, `python/packages/core/agent_framework/_agents.py:730-807` |
| Core chat client protocol | `SupportsChatGetResponse` protocol with typed `get_response(messages, stream, options, ...)` overloads; contravariant Options generic | `python/packages/core/agent_framework/_clients.py:84-201` |
| Core chat client base | `BaseChatClient` abstract base requiring only `_inner_get_response(messages, stream, options)`; handles compaction/middleware wrapping centrally | `python/packages/core/agent_framework/_clients.py:217-425` |
| Core embedding protocol | `SupportsGetEmbeddings` protocol and `BaseEmbeddingClient` abstract base — same adapter pattern for embeddings | `python/packages/core/agent_framework/_clients.py:869-912`, `python/packages/core/agent_framework/_clients.py:926-980` |
| .NET core abstraction | Duplicate pattern in .NET: `AIAgent`, `DelegatingAIAgent`, `ChatHistoryProvider`, `AgentSession` in Abstractions assembly | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:1`, `dotnet/src/Microsoft.Agents.AI.Abstractions/DelegatingAIAgent.cs:1` |
| Extension point — middleware | Three pipeline classes `AgentMiddlewarePipeline` / `ChatMiddlewarePipeline` / `FunctionMiddlewarePipeline` + `MiddlewareBundle` indivisible feature grouping | `python/packages/core/agent_framework/_middleware.py:1006-1260`, `python/packages/core/agent_framework/_middleware.py:745-828` |
| Extension point — progressive tools | `FunctionInvocationContext.add_tools()` / `remove_tools()` enabling runtime tool surface mutation (progressive disclosure) | `python/packages/core/agent_framework/_middleware.py:354-437` |
| Adapter: A2A client | `A2AAgent` wraps `a2a.client.Client`/`ClientFactory`, maps `Message` ↔ `A2AMessage` parts, manages `A2AServiceSessionId` continuation | `python/packages/a2a/agent_framework_a2a/_agent.py:209-398` |
| Adapter: A2A executor | `A2AExecutor(AgentExecutor)` bridges arbitrary `SupportsAgentRun` to A2A `RequestContext`/`EventQueue` via `TaskUpdater` | `python/packages/a2a/agent_framework_a2a/_a2a_executor.py:31-184` |
| Adapter: A2A fallback | Transport negotiation with fallback to `minimal_agent_card(url, bindings)` on failure | `python/packages/a2a/agent_framework_a2a/_agent.py:303-331` |
| Adapter: AG-UI client | `AGUIChatClient` extends `BaseChatClient` with `AGUIHttpService` + `AGUIEventConverter`, reusing the `FunctionInvocationLayer`/`ChatTelemetryLayer` stack via decorator | `python/packages/ag-ui/agent_framework_ag_ui/_client.py:115-247` |
| Adapter: AG-UI event converter | `_event_converters.py` + `_message_adapters.py` isolate protocol translation (dedup, sanitization, multimodal media) | `python/packages/ag-ui/agent_framework_ag_ui/_message_adapters.py:1` (file exists), `python/packages/ag-ui/agent_framework_ag_ui/_event_converters.py:1` |
| Adapter: MCP tooling | `MCPTool` family (`MCPStdioTool`, `MCPStreamableHTTPTool`, `MCPWebsocketTool`) + sampling guardrails, task-option lifecycle (SEP-2663) | `python/packages/core/agent_framework/_mcp.py:424-587`, `python/packages/core/agent_framework/_mcp.py:348-387` |
| Adapter: hosting-mcp conversion | Side-effect-free conversion helpers `mcp_to_run` / `mcp_from_run` with explicit boundary docs | `python/packages/hosting-mcp/agent_framework_hosting_mcp/_conversion.py:19-155` |
| Adapter: hosting-mcp tool | `AgentMCPTool` / `WorkflowMCPTool` own only single-tool schema, not server/transport/auth | `python/packages/hosting-mcp/agent_framework_hosting_mcp/_agent_tool.py:1` |
| Adapter: provider packages | 20+ provider adapters (`openai`, `anthropic`, `azure`, `bedrock`, `ollama`, `gemini`, `mistral`, etc.) each as standalone package under `python/packages/` | `python/packages/openai/`, `python/packages/anthropic/`, `python/packages/ollama/` (directory listing) |
| Swappability — duck typing test | `test_mock_client_satisfies_protocol` and `test_plain_class_satisfies_protocol` prove any object with `get_response` satisfies protocol without inheritance | `python/packages/core/tests/core/test_embedding_client.py:76-94`, `python/packages/core/tests/core/test_clients.py:33-52` |
| Swappability — runtime injection | `RawAgent.__init__(client: SupportsChatGetResponse)` + `BaseChatClient.as_agent()` factory demonstrate runtime adapter swapping | `python/packages/core/agent_framework/_agents.py:813`, `python/packages/core/agent_framework/_clients.py:571-658` |
| Conformance: MCP | Extensive MCP tests: argument allowlist, progressive disclosure, sampling deny-by-default, long-running tasks | `python/packages/core/tests/core/test_mcp.py:1` (5286 lines, multiple protocolVersion checks) |
| Conformance: A2A | `test_a2a_agent.py` initialization/binding tests, streaming task polling tests | `python/packages/a2a/tests/test_a2a_agent.py:719-803` |
| Conformance: AG-UI | Message adapter deduplication/sanitization tests, HTTP service, client typed-interrupt tests | `python/packages/ag-ui/tests/ag_ui/test_message_adapters.py:3`, `python/packages/ag-ui/tests/ag_ui/test_http_service.py:156`, `python/packages/ag-ui/tests/ag_ui/test_ag_ui_client.py:562` |
| Conformance: checkpoint storage | `test_checkpoint_storage_protocol_compliance` verifies storage protocol surface | `python/packages/core/tests/workflow/test_checkpoint.py:137-145` |
| Interop documentation — AG-UI | `AGENTS.md` details `CUSTOM` event aliases, multimodal shapes, interrupt/resume protocol fields | `python/packages/ag-ui/AGENTS.md:1-60` |
| Interop documentation — A2A | `AGENTS.md` and `README.md` document client vs executor roles, import paths, and minimal card fallback | `python/packages/a2a/AGENTS.md:1-56`, `python/packages/a2a/README.md:27` |
| Interop documentation — hosting-mcp | Boundary statement: "does not provide server/routes/transport lifecycle/auth" and explicit ownership split | `python/packages/hosting-mcp/AGENTS.md:1-45` |
| Interop documentation — ChatKit | Network restriction guidance: custom frontend via ChatKit server protocol | `python/packages/chatkit/README.md:50` |
| Interop documentation — ADRs | `0010-ag-ui-support`, `0027-hosting-channels`, `0032-dotnet-hosting-protocol-helpers`, `0029-mcp-skill-templates` capture boundary decisions | `docs/decisions/0010-ag-ui-support.md:1`, `docs/decisions/0027-hosting-channels.md:1`, `docs/decisions/0032-dotnet-hosting-protocol-helpers.md:1` |

## Answers to Dimension Questions

**1. Are protocols core or adapter-layer?**
Adapter-layer. Core defines only `SupportsAgentRun`, `SupportsChatGetResponse`/`BaseChatClient` (`python/packages/core/agent_framework/_agents.py:233`, `python/packages/core/agent_framework/_clients.py:84`, `python/packages/core/agent_framework/_clients.py:217`) and middleware abstractions. Every concrete protocol — A2A (`python/packages/a2a/agent_framework_a2a/_agent.py:209`), AG-UI (`python/packages/ag-ui/agent_framework_ag_ui/_client.py:115`), MCP (`python/packages/core/agent_framework/_mcp.py:424`), OpenAI/Anthropic/Bedrock/Ollama, ChatKit — lives in its own installable package under `python/packages/` or `dotnet/src/` (e.g., `Microsoft.Agents.AI.A2A`, `Microsoft.Agents.AI.AGUI`). OpenAI is in-core only because it is the default built-in client, but it still goes through `BaseChatClient`.

**2. Can adapters be added without core changes?**
Yes. Adapter creation requires implementing `SupportsChatGetResponse` or `SupportsAgentRun` (structural protocol, no inheritance). Evidence: provider packages (`anthropic`, `ollama`, `bedrock`, etc.) add dependencies only in their own `pyproject.toml` and subclass `BaseChatClient` implementing `_inner_get_response`. Tests prove duck typing suffices (`python/packages/core/tests/core/test_embedding_client.py:81-82` plain class satisfies `SupportsGetEmbeddings`; same for chat). The monorepo `python/AGENTS.md:60-70` states provider packages extend core with specific integrations and are lazy-loaded.

**3. Are adapters tested for conformance?**
Partially. Per-adapter test suites are strong:
- MCP: progressive disclosure, `allowed_tools`, argument allowlisting, sampling deny-by-default, task lifecycle — `python/packages/core/tests/core/test_mcp.py` ( >6k lines) plus `test_mcp_observability.py`.
- A2A: binding negotiation, task-state mapping, poll/continuation tokens — `python/packages/a2a/tests/test_a2a_agent.py:719`, `python/packages/a2a/tests/test_a2a_group_chat.py`.
- AG-UI: >10 test modules covering message adapters, event converters, HTTP service, snapshots — `python/packages/ag-ui/tests/ag_ui/test_message_adapters.py`, `test_http_service.py:156`, `test_snapshots.py:16`.
- Workflow checkpoint storage protocol compliance — `python/packages/core/tests/workflow/test_checkpoint.py:137`.
Gap: no single cross-protocol conformance harness or matrix validating that all adapters satisfy identical `BaseChatClient`/`SupportsAgentRun` contracts plus failure/chaos injection (e.g., transport loss, SSE disconnect). Each package owns its own assertions.

**4. Are interop boundaries documented?**
Yes, explicitly and per-package. `python/packages/hosting-mcp/AGENTS.md:18-45` enumerates what the adapter does NOT own (server, routes, transport, auth, concurrency). `python/packages/ag-ui/AGENTS.md` documents event aliases, `CUSTOM` vs `CUSTOM_EVENT`, multimodal shapes, `RUN_FINISHED.outcome.interrupts` vs legacy field. `python/packages/a2a/AGENTS.md:1-56` documents `A2AAgent` (client) vs `A2AExecutor` (server bridge) and session shapes (`A2AServiceSessionId`). `docs/decisions/0010-ag-ui-support.md`, `0027-hosting-channels.md`, `0032-dotnet-hosting-protocol-helpers.md` capture rationale. `python/AGENTS.md:50-120` lists package relationships and import paths. Missing: consolidated interop boundary diagram and hosted-tool `server_label` isolation could be more discoverable outside code comments.

## Architectural Decisions

- **Structural protocols over inheritance** (`python/packages/core/agent_framework/_agents.py:233`, `python/packages/core/agent_framework/_clients.py:84`). Decision enables BYO client/agent without subclassing core; validated via `runtime_checkable` tests. Tradeoff: typo-level errors surface late (runtime vs compile-time) in Python.
- **Monorepo with per-protocol installable packages** (`python/packages/*`, `dotnet/src/*`). Decision enforces dependency isolation (e.g., `a2a` requires `a2a-sdk`, `ag-ui` requires `ag-ui-protocol`) and allows independent versioning. Tradeoff: higher CI complexity, duplication of telemetry/observability setup across adapters (each sets `OTEL_PROVIDER_NAME`).
- **Layered middleware composition** (`ChatTelemetryLayer` → `FunctionInvocationLayer` → `ChatMiddlewareLayer` → `BaseChatClient` at `python/packages/ag-ui/agent_framework_ag_ui/_client.py:115-122`; Python `_clients.py:985-1012` docstring alignment). Decision makes telemetry, function calling, and user middleware composable regardless of protocol. Tradeoff: ordering sensitivity; `MiddlewareBundle` introduced to enforce atomic installation (`python/packages/core/agent_framework/_middleware.py:745-828`).
- **Side-effect-free hosting adapters** (`python/packages/hosting-mcp/agent_framework_hosting_mcp/_conversion.py:1`, `python/packages/hosting-mcp/AGENTS.md`). Decision leaves server lifecycle/auth to app, preventing framework from owning network boundaries. Tradeoff: more boilerplate for adopters (must wire `Server`/`ClientSession`).
- **MCP defense-in-depth** (`python/packages/core/agent_framework/_mcp.py:91-126` allowlist + denylist, `sampling_approval_callback` deny-by-default at line 503, `additional_tool_argument_names` isolation). Decision treats MCP servers as untrusted. Tradeoff: verbose configuration for power users.
- **Transport negotiation fallback** (`python/packages/a2a/agent_framework_a2a/_agent.py:303-331`). A2A client falls back to `minimal_agent_card(url)` on negotiation failure. Decision improves robustness against incomplete AgentCards. Tradeoff: masks misconfiguration if fallback succeeds silently (only logged, not surfaced as warning to caller).

## Notable Patterns

- **Protocol adapter as `BaseChatClient` subclass**: AG-UI (`AGUIChatClient`), OpenAI, Anthropic, Ollama all follow identical pattern — implement `_inner_get_response` + declare `OTEL_PROVIDER_NAME`. Mirrored in .NET with `Microsoft.Agents.AI.*` projects implementing `AIAgent`/`DelegatingAIAgent`.
- **Converter pair pattern**: `agent_framework_messages_to_agui` / `convert_tools_to_agui_format` + `_event_converters.AGUIEventConverter`; `mcp_to_run` / `mcp_from_run` — pure conversion isolated from transport, testable without network.
- **Session-shape extension via `service_session_id`**: `A2AServiceSessionId(TypedDict)` (`python/packages/a2a/agent_framework_a2a/_agent.py:60-66`) and `A2AContinuationToken` show how adapters extend opaque `AgentSession.service_session_id` without core schema change (also `SessionStore`/`FileSessionStore` extensibility at `python/packages/core/agent_framework/_sessions.py`).
- **Progressive disclosure gate**: MCP `use_progressive_disclosure` + `always_load` (`python/packages/core/agent_framework/_mcp.py:524-587`, tests in `test_mcp.py`) uses `FunctionInvocationContext.add_tools()` to defer tool surface — reusable pattern also available for Skills.
- **Approval-mode per tool**: `FunctionTool(approval_mode=...)` (`python/packages/core/agent_framework/_tools.py:408`) + `MCPTool` approval callbacks thread hosted-tool isolation (`server_label`) through all adapters.

## Tradeoffs

- **Isolation vs discoverability**: Per-package adapters keep core small and install cost low, but require users to discover the right package (e.g., `hosting-mcp` vs `core/_mcp.py` vs MCP SDK). No registry/capability advertisement beyond `pyproject.toml` extras.
- **Duck typing vs strictness**: Python `runtime_checkable` protocols enable zero-boilerplate BYO adapters but defer interface errors to runtime. .NET compensates with compile-time interfaces on the same concepts.
- **Hosting adapters own no transport**: `hosting-mcp` and `hosting-a2a` are pure adapters; app owns SSE/JSON-RPC lifecycle, auth, and session-key derivation. Reduces framework liability but shifts retry/idempotency and backpressure ownership to callers.
- **Fallback negotiation**: A2A fallback to JSONRPC on negotiation failure improves demo success at cost of hiding binding mismatches; production callers must inspect `supported_protocol_bindings` explicitly.
- **Sampling deny-by-default**: MCP sampling (`sampling_approval_callback=None` → deny) is safest for confused-deputy risk (`python/packages/core/agent_framework/_mcp.py:503-515`) but requires explicit opt-in `lambda _: True` for legacy auto-approve, adding migration friction.
- **Layered wrapping overhead**: Each chat call traverses compaction → telemetry → function invocation → chat middleware → transport. Performance impact is small but cumulative; OTEL span creation per tool call adds per-iteration cost.

## Failure Modes / Edge Cases

- **Lost MCP connection after task_id known vs before**: `python/packages/core/agent_framework/_mcp.py:330-345` — before `task_id`, connection loss raises `ToolExecutionException("connection lost; task state unknown")` without retry (avoids double execution); after `task_id`, reconnect-and-retry once via `_send_with_one_reconnect`. Misuse (supplying `additional_tool_argument_names` incorrectly) could still cause argument widening via server-declared schema.
- **A2A terminal vs abandoned task paths**: `_MCPTaskAbandoned` vs terminal failure distinction (`python/packages/core/agent_framework/_mcp.py:334-341`) — abandonment fires best-effort `tasks/cancel`; terminal failures don't. Non-streaming intermediate updates are accumulated and flushed only on terminal no-content (`python/packages/a2a/agent_framework_a2a/_agent.py:623-670`) — if task never terminates, caller blocks until timeout.
- **AG-UI state injection**: `AGUIChatClient._extract_state_from_messages` (`python/packages/ag-ui/agent_framework_ag_ui/_client.py:282-314`) base64-decodes state from last message's `data` content; malformed data is warning-swallowed and `None` returned — could silently drop state and start empty thread (mitigated by UUID fallback thread_id at line 341).
- **Tool name collision in MCP**: Multiple raw remote tools mapping to same local function name raises `ToolExecutionException` instead of shadowing (`python/packages/core/agent_framework/_mcp.py` description: first-one-wins shadow avoided). Empty `allowed_tools=[]` exposes zero tools (distinct from `load_tools=False`), a subtle misconfiguration trap.
- **Middleware failure semantics**: `MiddlewareFailure` (`python/packages/core/agent_framework/_middleware.py:85-141`) aborts streaming runs only when consumer iterates; synchronous tool bodies already on worker thread complete side effects and are discarded — cooperative cancellation limitation.
- **Approval replay after tool set changed**: AG-UI notes (`python/packages/ag-ui/AGENTS.md:41`) approval responses for tools injected during `before_run` are deferred to in-run middleware; submitting approval before those tools exist would otherwise be rejected or ignored.

## Future Considerations

- Add a cross-protocol conformance harness that instantiates a dummy `BaseChatClient`/`SupportsAgentRun` and runs the same workflow through A2A, AG-UI, and MCP adapters asserting identical `AgentResponse`/`AgentResponseUpdate` semantics, including error and cancellation paths.
- Publish a consolidated interop boundary diagram (core ↔ middleware ↔ adapters ↔ transports) and a capability matrix (streaming, cancellation, auth, state, continuation tokens) per adapter; currently scattered across `AGENTS.md` per package.
- Promote MCP `MCPTaskOptions` polling bounds (`_MCP_TASK_MIN/MAX_POLL_INTERVAL` at `python/packages/core/agent_framework/_mcp.py:324-326`) to tunable config and add `max_task_wait` observability (metric/histogram) for long-running task abandonment rate.
- Consider a plugin/registry API for adapter discovery (current discovery is import-time `pyproject` entry, no runtime `list_adapters()` / `capabilities()`), which would help harness authors swap adapters declaratively.
- Align .NET and Python adapter versioning and feature-usage telemetry (`FeatureIndex.A2A`/`AG_UI`) so cross-runtime interop tests can assert bitmask parity (`docs/specs/feature-usage-bit-registry.md`).

## Questions / Gaps

- No evidence of adapter runtime swappability via hot-reload or DI container; swappability verified only via constructor injection (`Agent(client=...)`) and factory tests (`test_clients.py:59-72`). Hot-swap within a live session not demonstrated.
- No evidence of adapter-specific conformance certification against external reference implementations (e.g., A2A SDK compliance suite beyond `ClientFactory` negotiation; AG-UI protocol version matrix not asserted in tests beyond `test_message_adapters.py`).
- No evidence of failure-injection coverage for hosting adapters (SSE disconnect, auth expiry, backpressure) beyond MCP poll timeout retry (`McpError(code=408)` at `_mcp.py:560` scope). Search boundary: `python/packages/hosting*/tests` and `python/packages/ag-ui/tests/ag_ui` contain functional but not chaos tests.
- Interop boundary for `server_label` scoped approvals is documented in code comments (`python/packages/core/agent_framework/_tools.py`, `_mcp.py`) but not in top-level `README.md`; discoverability gap for adopters composing multiple hosted tools with same name.

---

Generated by `Dimension 19.03: Adapter and Interop Boundary Design` against `agent-framework`.
