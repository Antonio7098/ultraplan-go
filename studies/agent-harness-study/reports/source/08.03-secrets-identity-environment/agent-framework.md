# Source Analysis: agent-framework

## Dimension 08.03: Secrets, Identity, and Environment Handling

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Polyglot: Python (`python/packages/*`), .NET/C# (`dotnet/src/*`), Go placeholder (`go/README.md` only — no Go implementation to analyze) |
| Analyzed | 2026-08-26 |

## Summary

Agent Framework treats secrets as **client-construction concerns**, not model-visible state. Credentials are resolved through a layered settings loader (explicit override → `.env` file → environment variable → default; `python/packages/core/agent_framework/_settings.py:190-319`) and wrapped in a `SecretString` type whose `repr()` masks the value (`_settings.py:52-82`). Azure workloads use standard `TokenCredential`/`AsyncTokenCredential` abstractions rather than raw keys (`python/packages/foundry/agent_framework_foundry/_chat_client.py:55-56,226-227`; `dotnet/src/Microsoft.Agents.AI.Purview/PurviewClient.cs:56-118`). Secret *injection into tools* is deliberately indirect: MCP tools receive credentials via a per-request `header_provider` callback or a stdio `env` mapping, both outside the tool-argument surface the model controls (`python/packages/core/agent_framework/_mcp.py:3054-3094`, `_mcp.py:2893-2914`). Trace redaction is deny-by-default: sensitive content (messages, tool arguments/results) is only emitted when an explicit opt-in flag is set (`python/packages/core/agent_framework/observability.py:768-780,833`), and OTel tool-definition serialization whitelists only `type/name/description/parameters`, dropping any auth tokens embedded in tool specs (`observability.py:2631-2680`; tests `python/packages/core/tests/core/test_observability.py:4166-4340`). The .NET side integrates `Microsoft.Extensions.Compliance.Redaction` with a `<redacted>` replacing redactor as the default for memory/search providers (`dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:95`, `dotnet/src/Shared/Redaction/ReplacingRedactor.cs:12-30`). Environment isolation is handled at execution boundaries: clean-environment shell modes with a preserved-variable allowlist (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/EnvironmentSanitizer.cs:20-60`), Docker executors with read-only roots by default (`python/packages/tools/agent_framework_tools/shell/_docker.py:338-343`), and sandboxed code execution tested against symlink-based host-secret exfiltration (`python/packages/hyperlight/tests/hyperlight/test_hyperlight_codeact.py:861-1000`).

**Answer to the guiding question — "Can a trace be shared without leaking credentials?"**: By default yes for framework-emitted telemetry, because content capture is off and tool-definition attributes are field-whitelisted. Two caveats: (1) enabling `ENABLE_SENSITIVE_DATA` captures chat messages and tool arguments/results verbatim into spans/events (`observability.py:771-774`), so traces from such runs are not shareable; (2) DevUI enables sensitive data by design for local debugging (`python/packages/devui/agent_framework_devui/__init__.py:136-137`) — those traces must not be exported.

## Rating

**Score: 7 / 10**

Rationale: There is a clear, documented model with explicit interfaces and real test coverage: secret-typed settings (`SecretString` + precedence rules), credential objects instead of string keys for Azure paths, deny-by-default sensitive telemetry with sticky-disable semantics (`observability.py:1238-1261`), whitelist-based OTel attribute building with dedicated secret-leak tests (`test_observability.py:4166-4340`), origin-scoped header injection to prevent token leakage on redirects (`_mcp.py:3045-3063`), and hardened execution environments (clean-env allowlists, read-only containers, symlink checks). It falls short of 8–9 because of acknowledged footguns and uneven depth: secrets passed via shared `function_invocation_kwargs` can reach every attached MCP server whose schema declares that name — the code documents this as a caller responsibility rather than enforcing it (`_mcp.py:3089-3094`); `SecretString` masks only `repr()`, so f-string interpolation still prints the secret by design (`_settings.py:62-69`); redaction in .NET exists only inside specific providers (Mem0/TextSearch/Memory), not pipeline-wide; and the strongest mechanism (FIDES information-flow labeling) is experimental.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Secret storage | `SecretString(str)` masks value in `repr()`; `get_secret_value()` for pydantic compat | `python/packages/core/agent_framework/_settings.py:52-82` |
| Secret storage | `load_settings` precedence: overrides → `.env` file → env vars → defaults; typed coercion incl. `SecretString` | `python/packages/core/agent_framework/_settings.py:199-292` |
| Secret storage | OpenAI/AzureOpenAI settings declare `api_key: SecretString \| None`; `.get_secret_value()` consumed only at client construction | `python/packages/openai/agent_framework_openai/_shared.py:114,128,236-238,298-299` |
| Secret storage | Callable/lazy API keys supported (`Callable[[], str \| Awaitable[str]]`) for token rotation | `python/packages/openai/agent_framework_openai/_chat_completion_client.py:281,300` |
| Secret providers | Azure `TokenCredential` required when `project_endpoint` used without prebuilt client | `python/packages/foundry/agent_framework_foundry/_chat_client.py:226-230` |
| Secret providers | `DefaultAzureCredential` created only when Foundry-hosted | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ServiceCollectionExtensions.cs:669-670` |
| Secret providers | Purview obtains scoped tokens via `TokenCredential.GetTokenAsync` with optional tenant id | `dotnet/src/Microsoft.Agents.AI.Purview/PurviewClient.cs:107-118` |
| Secret injection | MCP `header_provider` injects auth headers per request; origin-scoped so tokens are not sent on cross-origin redirects | `python/packages/core/agent_framework/_mcp.py:3054-3067,3159-3175` |
| Secret injection | Documented leak path: `function_invocation_kwargs` are forwarded to any server declaring the property; guidance to source creds via ContextVar/custom client/stdio `env` | `python/packages/core/agent_framework/_mcp.py:3084-3094,2866-2868` |
| Secret injection | ContextVar-sourced credential kept out of tool arguments (test proves header changes per request while args stay clean) | `python/packages/core/tests/core/test_mcp.py:7967-8027` |
| Secret injection | Stdio MCP servers get process-scoped `env` mapping instead of arguments | `python/packages/core/agent_framework/_mcp.py:2893-2914` |
| Redaction | Sensitive span/event capture off unless `enable_sensitive_data=True` (env `ENABLE_SENSITIVE_DATA`); docstring warns payloads become trace-visible | `python/packages/core/agent_framework/observability.py:768-780,833,1297` |
| Redaction | All message/tool-content span writes gated on `OBSERVABILITY_SETTINGS.SENSITIVE_DATA_ENABLED` | `python/packages/core/agent_framework/observability.py:1655,1786,1831,1997,2135` |
| Redaction | Sticky `disable_instrumentation()` wins over framework re-enable attempts | `python/packages/core/agent_framework/observability.py:1238-1261` |
| Redaction | OTel tool definitions keep only type/name/description/parameters — `authorization`/`headers` dropped; covered by named tests | `python/packages/core/agent_framework/observability.py:2631-2680`; `python/packages/core/tests/core/test_observability.py:4166-4340` |
| Redaction | .NET providers redact log data via `Microsoft.Extensions.Compliance.Redaction`; default `ReplacingRedactor("<redacted>")`, `NullRedactor` only on explicit opt-in | `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:95,301`; `dotnet/src/Microsoft.Agents.AI/TextSearchProvider.cs:94,339`; `dotnet/src/Microsoft.Agents.AI/Memory/ChatHistoryMemoryProvider.cs:122,497`; `dotnet/src/Microsoft.Agents.AI.Foundry/Memory/FoundryMemoryProvider.cs:84,420`; `dotnet/src/Shared/Redaction/ReplacingRedactor.cs:12-30` |
| Redaction | Declarative HTTP diagnostics truncate response bodies to 256 chars because bodies "can echo secrets" | `python/packages/declarative/agent_framework_declarative/_workflows/_executors_http.py:9-12,43-44,72-87` |
| Redaction | PowerFx eval failures logged with "(details redacted)" mode | `python/packages/declarative/agent_framework_declarative/_models.py:76-79` |
| Env config | Shell tool `clean_env=True` stops inheriting `os.environ`; `env=` seeds exactly what commands see | `python/packages/tools/agent_framework_tools/shell/_tool.py:113-118,149,178-185` |
| Sandbox mounts | Docker executor: read-only rootfs and read-only host mount by default; env passed explicitly via `--env` | `python/packages/tools/agent_framework_tools/shell/_docker.py:299-343,501-504,614-616` |
| Sandbox mounts | Hyperlight sandbox output checked against symlink/junction escapes so host secrets cannot be exfiltrated through output files | `python/packages/hyperlight/tests/hyperlight/test_hyperlight_codeact.py:861-1000` |
| Identity | FIDES labels include `ConfidentialityLabel.USER_IDENTITY` with user-id metadata; most-restrictive label combination | `python/packages/core/agent_framework/security.py:109-124,145-149,212-264` |
| Identity | Purview middleware resolves one consistent `user_id` (token > message properties > author GUID) across pre/post checks | `python/packages/purview/agent_framework_purview/_middleware.py:76-122`; `python/packages/purview/tests/purview/test_processor.py:618-664` |
| Identity | Mem0 memory scopes storage and search by UserId/ThreadId/AgentId/ApplicationId (at least one required) | `dotnet/src/Microsoft.Agents.AI.Mem0/Mem0Provider.cs:101-116` |
| Identity | User-Agent identity carries feature mask, not user data | `python/packages/core/agent_framework/_telemetry.py:124-129,163-176` |

## Answers to Dimension Questions

**1. Can the model see secrets?**
Not by design, but there is no hard enforcement barrier. Secrets live in chat-client constructors (`api_key`, `credential`), never in messages or tool schemas (`python/packages/openai/agent_framework_openai/_shared.py:236-238`). However, if a developer places a credential into `function_invocation_kwargs`, it is forwarded to *every* attached MCP server whose advertised schema declares a property of that name — the docs call this out explicitly and recommend ContextVar-sourced providers instead, but the framework does not block it (`python/packages/core/agent_framework/_mcp.py:3084-3094`). Model-supplied tool-call arguments are filtered to a schema-derived allowlist before reaching servers (`_mcp.py`, argument allowlist described at `_mcp.py:2170-2202`), which limits but does not eliminate model-initiated exposure. The experimental FIDES layer (`security.py`) is the only mechanism that can structurally prevent labeled-sensitive content from re-entering model context, and it is marked `@experimental`.

**2. Can tools use secrets without exposing them?**
Yes, via three supported patterns: (a) `header_provider` callbacks that compute auth headers per invocation, origin-scoped so tokens never follow cross-origin redirects (`_mcp.py:3054-3067,3159-3175`); (b) reading ambient context (e.g., a `ContextVar`) inside the provider so the credential never appears in runtime kwargs — proven by test `test_header_provider_reading_contextvar_keeps_credential_out_of_arguments` (`python/packages/core/tests/core/test_mcp.py:7967-8027`); (c) stdio transport `env` maps that scope the secret to the server subprocess (`_mcp.py:2893-2914`). A lock serializes calls when a provider is set, preventing wrong-credential races on parallel invocations (`_mcp.py:3123-3130`).

**3. Are secrets redacted in traces?**
Largely yes by default. Framework telemetry emits metadata only unless sensitive capture is enabled (`observability.py:768-780`); all content-bearing span writes check the gate (`observability.py:1655` etc.); and OTel tool definitions drop everything beyond type/name/description/parameters so MCP specs carrying `authorization` or `headers` cannot leak them (`observability.py:2631-2680`, tested at `test_observability.py:4166-4340`). .NET memory/search providers route diagnostic strings through `Microsoft.Extensions.Compliance.Redaction` redactors with `<redacted>` defaults (`Mem0Provider.cs:95,301`). Residual risks: once `ENABLE_SENSITIVE_DATA` is on, raw tool arguments/results enter spans verbatim; and custom `http_client`s bypass the framework's origin-scoping guarantee, shifting responsibility to the developer (`_mcp.py:3045-3053`).

**4. Are identities scoped per user/task?**
Partially, at the provider layer rather than the core. FIDES defines a `USER_IDENTITY` confidentiality level keyed to user ids in label metadata (`security.py:115-120,145-149`). Purview resolves a consistent per-request `user_id` from the auth token with documented precedence (`_middleware.py:76-122`). Mem0 requires explicit scoping keys including `UserId` and throws otherwise (`Mem0Provider.cs:101-116`). But core `ChatClientAgent`/session abstractions carry no built-in per-user identity concept — identity propagation (e.g., on-behalf-of flows) is delegated entirely to the host application and Azure credential configuration. No evidence of a first-class per-task identity primitive was found in `python/packages/core` or `dotnet/src/Microsoft.Agents.AI`.

## Architectural Decisions

1. **Credentials at construction time, not prompt time.** All provider clients resolve keys/credentials during instantiation (`_shared.py:226-238`; `_chat_client.py:226-230`), so the model-facing protocol surface never carries them.
2. **Opt-in sensitive telemetry with a sticky kill switch.** Instrumentation is on by default but content capture requires `ENABLE_SENSITIVE_DATA`, and `disable_instrumentation()` cannot be overridden except by explicit `force=True` (`observability.py:777-780,1238-1261`) — user intent outranks library auto-setup.
3. **Whitelist, not blacklist, for telemetry fields.** OTel tool definitions emit only known-safe GenAI fields and drop the rest (`observability.py:2666-2680`), which stays safe as new spec fields (with unknown secrets) appear.
4. **Reuse platform security infrastructure.** .NET leans on `Microsoft.Extensions.Compliance.Redaction` and `IConfiguration` user-secrets (`BaseSample.cs:63-67`; `Shared/Workflows/Settings/Application.cs:82`) instead of inventing parallel mechanisms.
5. **Execution isolation at the boundary.** Shell/code-execution features treat approval gating plus sandbox tier (Docker read-only root, Hyperlight WASM sandbox) as the real defense, with regex policies explicitly demoted to "UX pre-filter" (`_tool.py:118-136`).

## Notable Patterns

- **Masking-by-type**: a `str` subclass makes accidental repr/logging leaks unlikely while keeping normal string ergonomics (`_settings.py:52-74`).
- **Origin-scoped header injection**: request hooks compare scheme/host/port against the configured URL before attaching provider headers, defending against redirect-based token leakage (`_mcp.py:3159-3175`).
- **Allowlist-constrained env fallback**: declarative workflows expose `Env.<name>` PowerFx access only when `safe_mode=False` (default `True` blocks it), and the workflow factory restricts `os.environ` fallback to names referenced in the definition so unrelated variables never enter evaluation scope (`_models.py:37-40,71-74`; `declarative/agent_framework_declarative/_workflows/_declarative_base.py:75-119`; `_workflows/_factory.py:130-142,435`).
- **Preserved-variable allowlist for shells**: `EnvironmentSanitizer` keeps only PATH/HOME/USER-class variables under `cleanEnvironment`, stripping inherited secrets wholesale (`EnvironmentSanitizer.cs:20-60`).
- **Dev-only secret surfacing**: DevUI deliberately flips sensitive telemetry on locally and generates CSRF-style tokens with `secrets.token_urlsafe` compared via `compare_digest` (`devui/__init__.py:136-137`; `devui/_server.py:159,448`).

## Tradeoffs

- **Usability vs enforcement**: the shared `function_invocation_kwargs` dict is convenient but creates the documented cross-server credential-forwarding hazard; the framework chose documentation over prohibition (`_mcp.py:3089-3094`).
- **repr-masking vs true secrecy**: `SecretString` prints normally in f-strings and comparisons (`_settings.py:65-67`), trading stronger guarantees for pydantic compatibility.
- **Default-on instrumentation vs default-off content**: metadata telemetry always ships; content requires opt-in — good balance, but operators must audit that exporters configured later don't combine with an opted-in flag in shared config.
- **Per-provider redaction vs pipeline-wide redaction**: .NET redactors protect specific providers' logs; nothing enforces redaction for user-added middleware or custom clients.
- **Header snapshot + lock**: serializing tool calls when a header_provider is set guarantees correct credentials but sacrifices concurrency (`_mcp.py:3123-3130`).

## Failure Modes / Edge Cases

- **Ambient requests with kwarg-dependent providers**: initialize/discovery calls invoke `header_provider({})`; providers indexing a required kwarg raise `KeyError`, tolerated only for connect (`_mcp.py:3183-3199`) — misconfiguration surfaces as connect-time errors that historically manifested as cryptic anyio cancel-scope failures (see comment at `security.py:3427-3431`).
- **Custom `http_client` bypasses origin scoping**: unconditional headers or `follow_redirects=True` without an origin check can leak tokens to third-party origins; the framework warns but cannot prevent it (`_mcp.py:3045-3053`).
- **Sensitive-data flag in dev tools leaking to prod exports**: DevUI sets `enable_sensitive_data=True` (`devui/__init__.py:136-137`); copying its setup into production would put raw prompts and tool results into telemetry backends.
- **Persistent shell sessions carry state between calls**: exported variables (potentially containing pasted secrets) persist within a session; .NET docs mandate single-session ownership because sharing leaks state across conversations (`ShellMode.cs:24-36`). Stateless mode resets cwd/env per call (`ShellMode.cs:10-14`).
- **Symlink/planted-path exfiltration from sandboxes**: Hyperlight guards output reads/writes against link and reparse-point escapes targeting host files like `/host/secret` (`hyperlight/_execute_code_tool.py:689,749-757`; tests `test_hyperlight_codeact.py:861-1000`).

## Future Considerations

- Promote the FIDES information-flow system (`security.py:91-3532`) from experimental to released so confidentiality enforcement becomes structural rather than advisory.
- Add an opt-in "credentials never in runtime kwargs" mode that rejects or strips `function_invocation_kwargs` entries matching credential-shaped declarations, closing the documented MCP forwarding gap (`_mcp.py:3084-3094`).
- Extend `SecretString` masking beyond `repr()` (e.g., warn or redact on common formatting paths) or adopt pydantic `SecretStr` end-to-end.
- Centralize .NET redaction so every provider/middleware composes a single `Redactor` pipeline instead of each provider constructing its own (`Mem0Provider.cs:95` pattern ×4).
- Provide a vault abstraction (Key Vault / managed identity resolver) so Python parity with .NET's hosted `DefaultAzureCredential` path is documented rather than ad hoc (`ServiceCollectionExtensions.cs:669-670` vs no Python equivalent).

## Questions / Gaps

- No evidence found of secret-value detection/redaction applied to *user-provided* log statements or arbitrary middleware output in either language; searches for scrubbers in `python/packages/core` surfaced only message-history sanitizers (`ag-ui/_message_adapters.py:78`) unrelated to secrets.
- No evidence found of per-tool ACLs binding specific identities to specific tools (identity scoping exists only inside Purview/Mem0/FIDES contexts); searched `dotnet/src/Microsoft.Agents.AI` and `python/packages/core/agent_framework` for identity-to-tool bindings.
- The `go/` directory contains only `README.md` — no Go implementation exists in-tree to assess.
- Whether OTel exporter endpoints themselves (which may carry auth via `OTEL_EXPORTER_OTLP_HEADERS`, `observability.py:617-623`) are excluded from any span/log output was not verified beyond the absence of such logging in the module; no test was found covering that specific case.

---

Generated by `dimensions/08.03-secrets-identity-and-environment-handling.md` against `agent-framework`.
