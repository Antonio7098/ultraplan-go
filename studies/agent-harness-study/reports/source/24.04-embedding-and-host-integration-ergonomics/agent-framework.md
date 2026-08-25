# Source Analysis: agent-framework

## Dimension 24.04: Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework, MAF) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C#/.NET (`dotnet/src`), Python (`python/packages`), Go (pointer only: `go/README.md`) |
| Analyzed | 2026-08-25 |

> **Citation note:** All paths below are workspace-relative and rooted at the selected source directory `studies/agent-harness-study/sources/agent-framework/`. Line numbers refer to files in that tree.

## Summary

Agent Framework is designed first and foremost as an embeddable library, not a standalone application. The core embedding unit is a plain object — `AIAgent` in .NET (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:38`) and `Agent` in Python (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_agents.py:1786`) — that a host constructs directly with injected dependencies (chat client, tools, middleware, context providers) and invokes via `RunAsync`/`RunStreamingAsync` or `run(...)` without ceding process control.

On top of that library core, the framework layers several explicit hosting models:

1. **DI-native host integration (.NET)**: `AddAIAgent(...)` extension methods on both `IHostApplicationBuilder` (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hosting/HostApplicationBuilderAgentExtensions.cs:25-96`) and `IServiceCollection` (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hosting/AgentHostingServiceCollectionExtensions.cs:25-113`), returning an `IHostedAgentBuilder` (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hosting/IHostedAgentBuilder.cs:10-26`) for attaching session stores and tools.
2. **HTTP protocol hosting**: batteries-included `MapOpenAIResponses(...)` endpoint mapping (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/EndpointRouteBuilderExtensions.Responses.cs:26-60`), or "bring your own route" conversion helpers that translate OpenAI Responses payloads while the app owns routing/auth/persistence — documented as a deliberate two-option model in `studies/agent-harness-study/sources/agent-framework/dotnet/samples/04-hosting/af-hosting/README.md`.
3. **Helper-first hosting (Python)**: deliberately minimal packages — `agent-framework-hosting` provides only execution-state holders (`AgentState`, `WorkflowState`; `studies/agent-harness-study/sources/agent-framework/python/packages/hosting/agent_framework_hosting/_state.py:74-192,193-280`) and `agent-framework-hosting-responses` provides pure payload conversion functions (`responses_to_run`, `responses_from_run`; `studies/agent-harness-study/sources/agent-framework/python/packages/hosting-responses/agent_framework_hosting_responses/_parsing.py:127-199`). The web framework keeps routes, auth, and response objects.
4. **Platform-managed hosting**: Foundry Hosted Agents surfaces on both stacks (.NET `Microsoft.Agents.AI.Foundry.Hosting`; Python `ResponsesHostServer`/`InvocationsHostServer` in `studies/agent-harness-study/sources/agent-framework/python/packages/foundry_hosting/agent_framework_foundry_hosting/__init__.py:5-6`), plus protocol servers for A2A (`hosting-a2a`, `Microsoft.Agents.AI.Hosting.A2A*`), AG-UI, MCP, Telegram, and DevUI tooling including Aspire resource integration (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Aspire.Hosting.AgentFramework.DevUI/AgentFrameworkBuilderExtensions.cs:49`).
5. **Batteries-included harness**: a pre-assembled coding-style harness — `create_harness_agent` in Python (`studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_harness/_agent.py:302-344`) and `HarnessAgentOptions` in .NET (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:14-344`) — for hosts that want function loops, compaction, todos, file memory, skills, and approval wiring from one call.

The consistent ownership philosophy across stacks: the host retains policy, identity, storage choice, telemetry backend, and UX; the framework supplies protocol translation and state abstractions. Both hosting surfaces are covered by dedicated unit-test projects (`studies/agent-harness-study/sources/agent-framework/dotnet/tests/Microsoft.Agents.AI.Hosting.UnitTests/`, `studies/agent-harness-study/sources/agent-framework/python/packages/hosting/tests/hosting/test_state.py`).

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with explicit interfaces and operational safeguards (7–8 band)**: The embedding contract is explicit at every layer — abstract base classes with documented security remarks (`AIAgent.cs:16-36`), builder pipelines (`studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs:16-198`), protocols for host-supplied storage (Python `SessionStore` at `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1795-1868`; .NET abstract `AgentSessionStore` at `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hosting/AgentSessionStore.cs:48-103`), and samples that spell out exactly which concerns belong to the host vs. framework (`studies/agent-harness-study/sources/agent-framework/python/samples/04-hosting/af-hosting/local_responses/app.py:14-28`).
- **Why not 9–10**: Key state primitives are still experimental (Python `SessionStore` is decorated `@experimental(feature_id=ExperimentalFeature.SESSION_STORE)` at `studies/agent-harness-study/sources/agent-framework/python/packages/core/agent_framework/_sessions.py:1794`); durable agents were moved out of this repo to an external extension (`studies/agent-harness-study/sources/agent-framework/dotnet/samples/04-hosting/DurableAgents/README.md:1-3`); concurrency coordination per session id is explicitly delegated to the host with no provided primitive (`app.py:160-164`); cancellation in Python relies purely on asyncio convention rather than an explicit token parameter.

## Evidence Collected

Every entry includes a file path with line numbers, relative to `studies/agent-harness-study/sources/agent-framework/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library-mode agent abstraction | `public abstract partial class AIAgent` with `RunAsync`/`RunStreamingAsync` overloads taking `CancellationToken` | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:38,251-255,334-338,465-480` |
| Streaming contract | `IAsyncEnumerable<AgentResponseUpdate>` streaming core; `ResponseStream(AsyncIterable[UpdateT], Generic[...])` in Python | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:503-507`; `python/packages/core/agent_framework/_types.py:3064` |
| Python run API | `def run(self, messages=None, *, stream=False, session=None, middleware=..., tools=..., options=...)` returns `Awaitable[AgentResponse]` or `ResponseStream` | `python/packages/core/agent_framework/_agents.py:1849-1880` |
| Agent construction w/ DI of deps | `Agent.__init__(client, instructions, *, tools, context_providers, middleware, compaction_strategy, ...)` | `python/packages/core/agent_framework/_agents.py:1882-1914` |
| Decorator/pipeline composition | `AIAgentBuilder.Use(Func<AIAgent, IServiceProvider, AIAgent>)` builds decorator chains; anonymous delegates accepted | `dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs:76-152` |
| DI registration entry point | `AddAIAgent(this IHostApplicationBuilder builder, string name, string? instructions, ServiceLifetime lifetime = Singleton)` + factory-delegate overload | `dotnet/src/Microsoft.Agents.AI.Hosting/HostApplicationBuilderAgentExtensions.cs:25-96` |
| Service-collection entry point | Five `AddAIAgent(this IServiceCollection, ...)` overloads incl. keyed chat-client resolution | `dotnet/src/Microsoft.Agents.AI.Hosting/AgentHostingServiceCollectionExtensions.cs:25-113` |
| Hosted-agent builder hooks | `WithInMemorySessionStore`, `WithSessionStore(store/factory)`, `WithAITool(s)` on `IHostedAgentBuilder` | `dotnet/src/Microsoft.Agents.AI.Hosting/HostedAgentBuilderExtensions.cs:23-129` |
| Host-suppliable session storage (.NET) | Abstract `AgentSessionStore` with `SaveSessionAsync`/`GetSessionAsync`/`DeleteSessionAsync` | `dotnet/src/Microsoft.Agents.AI.Hosting/AgentSessionStore.cs:48-103` |
| Host-suppliable session storage (Python) | `SessionStore` get/set/delete returning independent copies; `FileSessionStore` with atomic replace | `python/packages/core/agent_framework/_sessions.py:1826-1868,1872+` |
| Workflow checkpoint storage injection | `CheckpointStorage` Protocol; `workflow.run(message, checkpoint_storage=...)` runtime override; InMemory/File impls | `python/packages/core/agent_framework/_workflows/_checkpoint.py:129,202,249`; `_workflow.py:683-703` |
| Azure Blob session store | `WithAzureBlobSessionStore(...)` builder extensions | `dotnet/src/Microsoft.Agents.AI.Hosting.AzureStorage/AzureBlobHostedAgentBuilderExtensions.cs:26,55` |
| Batteries-included HTTP endpoint | `MapOpenAIResponses(endpoints, IHostedAgentBuilder/AIAgent/path+options)` endpoint conventions | `dotnet/src/Microsoft.Agents.AI.Hosting.OpenAI/EndpointRouteBuilderExtensions.Responses.cs:26-60` |
| App-owned route alternative | Sample maps its own `/responses`, calls `OpenAIResponses.ToAgentRunRequest(body)` and runs the agent itself | `dotnet/samples/04-hosting/af-hosting/local_responses/Server/Program.cs:59-105` |
| Helper-first Python hosting | FastAPI route calls `responses_to_run(body)` / `responses_from_run(...)`; app filters request options itself (`ALLOWED_REQUEST_OPTIONS`) | `python/samples/04-hosting/af-hosting/local_responses/app.py:61-67,116-192` |
| Execution-state holder | `AgentState(get_or_create_session/set_session, asyncio.Lock per session_id)`; docstring: does not own routes/middleware/dispatch | `python/packages/hosting/agent_framework_hosting/_state.py:74-117,163-190` |
| Multi-tenant isolation hook | `abstract class AgentIsolationKeyProvider.GetIsolationKeyAsync()`; claims-based impl via `IHttpContextAccessor` | `dotnet/src/Microsoft.Agents.AI.Hosting/AgentIsolationKeyProvider.cs:25-40`; `dotnet/src/Microsoft.Agents.AI.Hosting.AspNetCore/ClaimsIdentityAgentIsolationKeyProvider.cs:41-51` |
| Telemetry opt-in (.NET) | `UseOpenTelemetry(...)` on `AIAgentBuilder` | `dotnet/src/Microsoft.Agents.AI/OpenTelemetryAgentBuilderExtensions.cs:11,47` |
| Telemetry config (Python) | `_get_exporters_from_env` reads standard `OTEL_EXPORTER_OTLP_*` variables; exporter creation helper | `python/packages/core/agent_framework/observability.py:548-564,422` |
| Config/secrets resolution | Settings loader resolves override → .env → env var → default (`os.getenv(env_var_name)`); secrets typed as `SecretString` | `python/packages/core/agent_framework/_settings.py:250-300` |
| Tool approval surfacing | `approval_mode:` constructor arg on tools; `ToolApprovalMiddleware` coordinates queued requests/responses | `python/packages/core/agent_framework/_tools.py:316-331`; `python/packages/core/agent_framework/_harness/_tool_approval.py` (documented contract) |
| Approval plumbing (.NET) | `ApprovalResponseBindingChatClient` stores pending approval requests in `AgentSessionStateBag` between runs | `dotnet/src/Microsoft.Agents.AI/ChatClient/ApprovalResponseBindingChatClient.cs:47-53` |
| Harness one-call assembly | `create_harness_agent(client, *, tools, history_provider, todo_provider, file_memory_store, shell_executor, auto_approval_rules, loop_should_continue, ...)` | `python/packages/core/agent_framework/_harness/_agent.py:302-344` |
| Harness options (.NET) | `HarnessAgentOptions` with Disable*/provider-injection knobs incl. `ChatHistoryProvider`, `AIContextProviders`, `LoopEvaluators` | `dotnet/src/Microsoft.Agents.AI.Harness/HarnessAgentOptions.cs:19-344` |
| Session persistence across restarts | `SerializeSessionAsync`/`DeserializeSessionAsync` public API with PII/security guidance | `dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:187-235` |
| DevUI & Aspire embedding | `MapDevUI(...)` endpoint; Aspire `AddDevUI(resource)` + `WithAgentService<TSource>(...)` | `dotnet/src/Microsoft.Agents.AI.DevUI/DevUIExtensions.cs:11,38`; `dotnet/src/Aspire.Hosting.AgentFramework.DevUI/AgentFrameworkBuilderExtensions.cs:49,167` |
| Platform hosting surface | `ResponsesHostServer`, `InvocationsHostServer`, store providers exported from foundry_hosting | `python/packages/foundry_hosting/agent_framework_foundry_hosting/__init__.py:5-31` |
| Hosting tests exist | Dedicated unit-test projects for hosting surfaces on both stacks | `dotnet/tests/Microsoft.Agents.AI.Hosting.UnitTests/AgentHostingServiceCollectionExtensionsTests.cs`; `python/packages/hosting/tests/hosting/test_state.py` |

## Answers to Dimension Questions

### 1. Can the harness run inside another application without owning the whole process?

**Yes — this is the primary design.** The minimal embedding is constructing an `Agent`/`AIAgent` object and calling it; nothing installs handlers, threads, or event loops. The Python `Agent.run()` signature takes all dependencies as parameters and returns plain awaitables/iterators (`python/packages/core/agent_framework/_agents.py:1849-1880`). The Python hosting package's own docstring states it "does not own routes, middleware, protocol dispatch, or native SDK calls -- web frameworks keep those concerns" (`python/packages/hosting/agent_framework_hosting/_state.py:77-80`), and the .NET af-hosting sample demonstrates an app that "keeps control of routing, auth, and session storage" while the framework only translates protocol (`dotnet/samples/04-hosting/af-hosting/local_responses/Server/Program.cs:3-6`). The only ambient mechanism found is `AIAgent.CurrentRunContext` backed by `AsyncLocal<T>` (`dotnet/src/Microsoft.Agents.AI.Abstractions/AIAgent.cs:40,102-106`), which is async-flow-scoped rather than process-global, and explicitly restored around caller code during streaming (`AIAgent.cs:471-479`). No `atexit` handlers, global signal handlers, or implicit background workers were found in the Python core (`grep` for `atexit|threading.Timer|signal.signal` across `python/packages/core/agent_framework/**` returned no non-test hits).

### 2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?

**Yes, across all six categories, though with different maturity per stack:**

- **Policy**: The host filters/overrides request options before invoking agents (`ALLOWED_REQUEST_OPTIONS` frozenset applied in `python/samples/04-hosting/af-hosting/local_responses/app.py:113-133`); .NET middleware/decorator pipeline allows arbitrary interception (`dotnet/src/Microsoft.Agents.AI/AIAgentBuilder.cs:112-152`).
- **Tools**: Constructor injection (`tools=` at `_agents.py:1890`), builder attachment (`WithAITool`/`WithAITools` at `dotnet/src/Microsoft.Agents.AI.Hosting/HostedAgentBuilderExtensions.cs:87-129`).
- **Identity**: Claims-based isolation keys resolve tenant identity per-request (`dotnet/src/Microsoft.Agents.AI.Hosting.AspNetCore/ClaimsIdentityAgentIsolationKeyProvider.cs:45`); samples treat continuation ids as untrusted pending app authorization (`Program.cs:64-67`, `app.py:20-24`).
- **Storage**: Pluggable session stores on both stacks (`AgentSessionStore.cs:48-103`; `_sessions.py:1795-1868`), plus workflow `CheckpointStorage` protocol (`_workflows/_checkpoint.py:129`); concrete Azure Blob option exists (`AzureBlobHostedAgentBuilderExtensions.cs:26`).
- **Telemetry**: Opt-in OTel decoration in .NET (`OpenTelemetryAgentBuilderExtensions.cs:47`); standard OTEL env-var configuration in Python (`observability.py:552-564`).
- **Secrets**: Typed settings resolution preferring explicit overrides over env vars (`_settings.py:250-300`); samples read credentials from environment and pass them explicitly (`Program.cs:20-22`, `app.py:95`).

### 3. Are lifecycle, cancellation, shutdown, and error propagation explicit?

**Mostly yes, with a stack asymmetry on cancellation.**

- .NET: Every run overload accepts a `CancellationToken` (`AIAgent.cs:245,251-255,465-469`); session save/load APIs support restart scenarios (`AIAgent.cs:187-235`); errors propagate as exceptions through the async pipeline.
- Python: There is **no explicit cancellation-token parameter** anywhere in the run API (searches for `CancellationToken`/`cancel_token` across `python/packages/core/agent_framework/*.py` returned zero hits); cancellation is by asyncio task cancellation convention. Errors are ordinary exceptions — the sample maps `ValueError` from payload parsing to HTTP 400 (`app.py:119-122`).
- Shutdown/cleanup: No dedicated harness-level shutdown hook was found on either stack; ownership follows normal language idioms (DI disposal scopes in .NET; GC/async context managers in Python). This is adequate because embedding is library-shaped, but hosts integrating long-lived resources (e.g., connected MCP sessions) must manage teardown themselves.
- Workflow durability: checkpoint-per-superstep semantics with restore-from-checkpoint are explicit (`_workflows/_workflow.py:241-265,629-655`).

### 4. Does the integration model work for both local-first and service deployments?

**Yes, and this is one of the strongest aspects.** Local-first: console/library use, DevUI, file-based stores, and local Hypercorn/FastAPI samples (`python/samples/04-hosting/af-hosting/local_responses/app.py:30-43`). Service deployments: ASP.NET Core endpoint mapping (`EndpointRouteBuilderExtensions.Responses.cs:26`), Aspire resource orchestration (`AgentFrameworkBuilderExtensions.cs:49,167`), container/Azure Functions sample directories (`python/samples/04-hosting/container`, `python/samples/04-hosting/azure_functions`), and the fully platform-managed Foundry Hosted Agents mode where the platform exposes the protocol (`python/samples/04-hosting/af-hosting/README.md` comparison table). The same agent code moves between modes because hosting concerns are layered on top of, not woven into, the agent abstraction.

## Architectural Decisions

1. **Agent-as-object over agent-as-process.** The base types carry no runtime assumptions — `AIAgent` is an abstract class whose instance methods are pure invocation entry points (`AIAgent.cs:237-507`); Python's `SupportsAgentRun` is even a structural `Protocol`, so third-party agents integrate without inheritance (`python/packages/core/agent_framework/_agents.py:234-260`).

2. **Decorator-pipeline extensibility instead of framework callbacks.** .NET's `AIAgentBuilder.Use(...)` composes wrapping agents with service-provider access (`AIAgentBuilder.cs:76-93`), mirroring Python's middleware lists passed per-construction or per-run (`_agents.py:1855,1893`). Host cross-cutting concerns (logging, policy, retries) attach as decorators rather than framework events.

3. **Two-tier HTTP hosting: batteries-included vs. bring-your-own-route.** Documented explicitly as an either/or decision ("batteries included" `MapOpenAIResponses` vs. conversion helpers under app-owned routes; `dotnet/samples/04-hosting/af-hosting/README.md:5-15`). The Python side generalizes this into tiny "helper-first" packages whose entire public surface is state holders and pure conversion functions (`python/packages/hosting-responses/agent_framework_hosting_responses/__init__.py`; `_parsing.py:127-199`).

4. **State snapshots as the durability seam.** Sessions serialize to plain JSON-capable dicts with registered codecs (`to_dict`/`from_dict` at `_sessions.py:1757-1791`); .NET exposes serialize/deserialize on the agent itself (`AIAgent.cs:187-235`). Storage backends see opaque ids and blobs, keeping hosts free to choose persistence.

5. **Security commentary embedded in contracts.** Trust-boundary guidance lives in XML doc remarks on the core types (prompt-injection, untrusted serialized sessions; `AIAgent.cs:23-35,177-185,216-220`) and in sample route code ("candidate continuation id is untrusted"; `Program.cs:64-67`) — signaling that policy enforcement is deliberately a host obligation.

6. **Isolation keys as a first-class multi-tenancy concept.** An abstract provider resolves per-request isolation keys used to scope shared stores (`AgentIsolationKeyProvider.cs:9-40`), with ASP.NET identity-based and platform-hosted implementations.

## Notable Patterns

- **Protocol-conversion helpers as embedding glue.** Instead of forcing hosts onto a server framework, the framework ships pure functions like `responses_to_run(body) -> AgentRunArgs` (`_parsing.py:173`) and `OpenAIResponses.ToAgentRunRequest(body)` (`Program.cs:62`), letting any transport (FastAPI, aiogram/Telegram, ASP.NET minimal APIs) become a harness front-end.
- **Per-session lock map for state helpers.** `AgentState` keeps `dict[str, asyncio.Lock]` so concurrent turns on different sessions don't serialize each other (`_state.py:112-116,173-174`) — while candidly documenting that concurrent runs on the *same* id remain the app's problem.
- **Feature-usage instrumentation without coupling.** `mark_feature_used(FeatureIndex.HOSTING)` records usage into a bit-mask for diagnostics (`_telemetry.py:137-149`; called at `_state.py:117`), giving maintainers adoption signals without adding host-facing behavior.
- **Knob-heavy harness presets.** `HarnessAgentOptions` uses paired enable/disable + inject properties (e.g., `DisableTodoProvider`, `TodoProvider`-equivalents, `FileMemoryStore`) so hosts can adopt defaults selectively (`HarnessAgentOptions.cs:293-344`); Python's `create_harness_agent` mirrors the same shape with keyword args (`_harness/_agent.py:302-344`).
- **Samples as executable contracts.** Hosting samples encode ownership boundaries in comments and code structure (mutable-head vs. immutable-snapshot session writes; `Program.cs:72-78`, `app.py:157-164`), effectively serving as normative documentation for integrators.

## Tradeoffs

1. **Minimalism shifts operational burden to hosts.** Because `AgentState` intentionally doesn't own routing, auth, or serialization of concurrent writes, every production embedder must implement per-session single-writer coordination, authentication of continuation ids, and tenant partitioning themselves (`app.py:14-28,160-164`; `Program.cs:49-51,72-78`). This maximizes flexibility but raises the floor for correct deployments.

2. **Batteries-included endpoints trade control for speed.** `MapOpenAIResponses` handles protocol/routing/storage but constrains hosting behavior; the escape hatch is rewriting the route with helpers (`af-hosting/README.md:9-15`) — a real fork in implementation effort.

3. **Cross-stack parity is incomplete.** .NET has keyed-service resolution, DI lifetimes, and claims-based isolation; Python has structural protocols and env-var settings. Cancellation tokens exist only on the .NET surface. Teams embedding both stacks must maintain two mental models.

4. **Experimental seams inside otherwise stable APIs.** The very storage abstraction hosts need most — Python `SessionStore`/`FileSessionStore` — is flagged experimental (`_sessions.py:1794,1871`), meaning early embedders accept churn risk on their persistence layer.

5. **Durable execution lives outside the repo.** Durable Agents/Azure Functions samples and presumably implementations moved to `microsoft/agent-framework-durable-extension` (`dotnet/samples/04-hosting/DurableAgents/README.md:1-3`; `python/AGENTS.md` package notes), fragmenting the embedding story evaluation across repositories.

## Failure Modes / Edge Cases

- **Concurrent writers to one conversation id corrupt ordering expectations**: last-writer-wins is documented for `FileSessionStore` (`_sessions.py:1878-1880`) and the samples repeatedly warn that `AgentState` "does not serialize concurrent runs for the same id" (`_state.py` docstring context; `app.py:160-164`). A host missing this gets silent history divergence.
- **Ambient run-context leakage across interleaved streams is guarded but subtle**: `RunStreamingAsync` must re-assert `CurrentRunContext` after each `yield` because caller code executes between iterations (`AIAgent.cs:471-479`); any future refactor dropping that re-assignment would silently break context consumers.
- **Untrusted continuation ids as an auth bypass vector**: if a host skips the authorize step shown in samples, a caller could read/append another tenant's conversation by guessing ids (`Program.cs:64-67`; `app.py:20-24`). The framework surfaces the value but cannot enforce the check.
- **Plaintext session snapshots**: `FileSessionStore` docs state files are plaintext JSON/MessagePack, "not a secret store," with no encryption or cross-process locking (`_sessions.py:1887-1896`).
- **One-shot awaitable targets misconfigured with caching disabled** raise `ValueError` at construction rather than lazily (`_state.py:107-108`) — fail-fast, but a construction-time surprise for dynamic target factories.
- **Missing evidence (explicitly noted):** I searched for harness-provided shutdown/cleanup hooks (e.g., a `Stop()`/`aclose()` lifecycle on hosting state or hosted builders) and found none beyond language-default disposal; likewise, no Python-side equivalent of `AgentIsolationKeyProvider` was located in `python/packages/hosting/` — multi-tenant key resolution appears .NET-only within this repo.

## Future Considerations

- Promote Python `SessionStore`/`FileSessionStore` out of experimental status and stabilize their snapshot format guarantees, since they are the recommended persistence seam for embedders.
- Provide (or document reference implementations for) per-session single-writer coordination, given how consistently samples push this onto hosts — e.g., a lease/CAS variant of `SessionStore.set`.
- Add an explicit Python cancellation surface (or document asyncio-cancellation guarantees per component) to reach parity with .NET's ubiquitous `CancellationToken` parameters.
- Consider a Python counterpart to `AgentIsolationKeyProvider` so multi-tenant scoping has a symmetric, testable contract across stacks.
- Re-integrate or clearly link durable-execution documentation, since the in-repo README now points away from the repo (`DurableAgents/README.md`), which complicates evaluating the full embedding matrix.

## Questions / Gaps

- **Shutdown semantics for long-lived embedded resources** (connected MCP clients, background sub-agents): no central teardown contract was found in either stack; whether DI disposal covers everything in practice needs runtime verification.
- **Go SDK ergonomics could not be assessed in-repo**: `go/README.md` redirects to the external `microsoft/agent-framework-go` repository, and rule 1 (source isolation) forbids reading it.
- **Behavioral verification of `MapOpenAIResponses` under failure** (mid-stream cancellation, store outage): unit tests exist for service collection and state helpers (`Microsoft.Agents.AI.Hosting.UnitTests/`; `hosting/tests/hosting/test_state.py`), but I did not locate tests covering streaming interruption semantics of the mapped endpoint; this was not exhaustively searched.
- **Performance characteristics of deep-copy snapshot stores**: `SessionStore.get/set` deepcopy every session (`_sessions.py:1840-1841,1855`); cost profiles for large histories were not measured here.

---

Generated by `Dimension 24.04: Embedding and Host Integration Ergonomics` against `agent-framework`.
