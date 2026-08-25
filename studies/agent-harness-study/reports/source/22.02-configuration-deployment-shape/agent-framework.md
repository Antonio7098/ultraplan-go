# Source Analysis: agent-framework

## Dimension 22.02 — Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C#/.NET (`dotnet/`), Python (`python/`), Go stub (`go/README.md`) |
| Analyzed | 2026-08-25 |

## Summary

Agent Framework is a multi-language SDK monorepo, not a deployed service, so its "configuration and deployment shape" is that of an **embeddable library plus a family of hosting adapters**. Configuration follows a deliberate two-tier model:

1. **Python**: a single generic loader, `load_settings()`, resolves `TypedDict` settings schemas through a documented four-layer precedence chain — explicit keyword overrides > optional `.env` file > process environment variables (`<PREFIX><FIELD>`) > TypedDict defaults — with required-field and mutually-exclusive-group validation after resolution (`python/packages/core/agent_framework/_settings.py:190-319`). Every provider package reuses it with its own prefix (`OPENAI_`, `AZURE_OPENAI_`, `ANTHROPIC_`, `FOUNDRY_`, `OLLAMA_`, `BEDROCK_`, …), giving uniform per-provider env-var contracts.
2. **.NET**: standard `Microsoft.Extensions` composition — agents are registered on `IHostApplicationBuilder` via `AddAIAgent(...)` overloads with DI lifetimes and keyed chat-client services (`dotnet/src/Microsoft.Agents.AI.Hosting/HostApplicationBuilderAgentExtensions.cs:25-96`); samples follow a repo-wide convention of fail-fast environment-variable reads (`?? throw new InvalidOperationException(...)`) documented in `dotnet/AGENTS.md` ("Config: Read from environment variables with UPPER_SNAKE_CASE naming").

There is no first-class staging/prod environment abstraction inside the SDK itself; instead the framework achieves dev/prod parity by **environment detection**: the same code paths switch backing stores and telemetry posture based on the `FOUNDRY_HOSTING_ENVIRONMENT` variable (Python detection at `python/packages/core/agent_framework/_telemetry.py:86-121`; .NET key constant at `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ServiceCollectionExtensions.cs:388`). Deployment is expressed as a menu of documented hosting models (self-hosted HTTP, A2A service, Foundry hosted agents, Aspire DevUI aggregation, Container Apps push-button deploy, WASM sandboxing via Hyperlight). Feature control exists as *feature-stage lifecycle gating* (`experimental()` / `release_candidate()` decorators) and *feature-usage telemetry masks*, not as runtime feature flags.

## Rating

**8 / 10**

Rationale against the rubric's top bands:

- **Clear model with tests** (7–8 band): the Python config layer has a precise precedence contract enforced by ~40 dedicated unit tests covering overrides-beat-env, dotenv-vs-env ordering, coercion of int/float/bool, secret masking, required fields, and mutual-exclusion errors (`python/packages/core/tests/core/test_settings.py:40-259`). OTel env layering (base endpoint vs signal-specific endpoints, header merging) is likewise tested (`python/packages/core/tests/core/test_observability.py:1013-1259`).
- **Explicit interfaces and operational safeguards**: secrets are masked via `SecretString` repr override (`python/packages/core/agent_framework/_settings.py:52-82`); DevUI fails closed — it refuses to disable auth or bind without a token on non-loopback hosts in both languages (`python/packages/devui/agent_framework_devui/_server.py:145-169`; `dotnet/src/Microsoft.Agents.AI.DevUI/DevUIOptions.cs:17-58`).
- **Why not 9–10**: there is no unified cross-language config schema (Python uses prefixed env vars + `.env`; .NET samples use raw `Environment.GetEnvironmentVariable` reads rather than `IOptions<T>` binding from configuration for provider credentials), there is no staging-environment concept, and no runtime feature-flag system (kill-switches exist only for telemetry emission). Config drift between the two language stacks is possible since each defines its own env-var names independently.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic settings loader (Python) | `load_settings()` documents 4-layer precedence: overrides → `.env` → env vars → defaults | `python/packages/core/agent_framework/_settings.py:199-235` |
| Precedence implementation | Sequential resolution blocks: overrides (255-265), dotenv (269-277), env var (279-286), default (288-292) | `python/packages/core/agent_framework/_settings.py:253-292` |
| Secret masking | `SecretString` subclass masks `repr()` to prevent accidental exposure; keeps `get_secret_value()` for pydantic compat | `python/packages/core/agent_framework/_settings.py:52-82` |
| Type coercion | `_coerce_value` handles unions, SecretString, str/int/float/bool | `python/packages/core/agent_framework/_settings.py:85-115` |
| Override type validation | `_check_override_type` rejects clearly incompatible types, allows callables/lazy providers | `python/packages/core/agent_framework/_settings.py:131-187` |
| Required-field validation | String entries = must be non-None; tuple entries = exactly-one group; raises `SettingNotFoundError` naming the env var to set | `python/packages/core/agent_framework/_settings.py:294-317` |
| Error type | `SettingNotFoundError(AgentFrameworkException)` | `python/packages/core/agent_framework/exceptions.py:228` |
| Per-provider prefixes | `env_prefix="OPENAI_"` then fallback `AZURE_OPENAI_` with `[("base_url","endpoint")]` exclusive group | `python/packages/openai/agent_framework_openai/_shared.py:218-260` |
| Provider prefix survey | ANTHROPIC_, GEMINI_/GOOGLE_, OLLAMA_, BEDROCK_, FOUNDRY_, COPILOTSTUDIOAGENT__, AZURE_SEARCH_, AZURE_COSMOS_, FOUNDRY_LOCAL_ | `python/packages/anthropic/agent_framework_anthropic/_chat_client.py:337`; `python/packages/gemini/agent_framework_gemini/_chat_client.py:363-371`; `python/packages/foundry/agent_framework_foundry/_chat_client.py:200` |
| Settings unit tests | Overrides/env/dotenv ordering, coercion, secrets, required groups (~40 cases) | `python/packages/core/tests/core/test_settings.py:40-379` |
| OTel env layering | Base + signal-specific endpoints/headers merged per OTel spec; protocol grpc vs http | `python/packages/core/agent_framework/observability.py:548-638` |
| Telemetry kill switches | `AGENT_FRAMEWORK_USER_AGENT_DISABLED`, `AGENT_FRAMEWORK_FEATURE_MASK_DISABLED` env vars gate User-Agent and feature-mask emission | `python/packages/core/agent_framework/_telemetry.py:18-22,132-134` |
| Hosted-env detection (Python) | `FOUNDRY_HOSTING_ENVIRONMENT` env var, else probe external `azure.ai.agentserver.core.AgentConfig.from_env().is_hosted`; adds `foundry-hosting` UA prefix | `python/packages/core/agent_framework/_telemetry.py:60-121` |
| Feature-usage mask (Python) | Process-global 128-bit mask, thread-safe, versioned token `(feat=vN.hex)` in User-Agent | `python/packages/core/agent_framework/_telemetry.py:137-176` |
| Feature-stage gating | `experimental()` / `release_candidate()` decorators, one-time warnings with dedup registry, docstring injection | `python/packages/core/agent_framework/_feature_stage.py:435-455,236-301` |
| Experimental inventory | `ExperimentalFeature` enum lists 14 staged feature IDs (HARNESS, MCP_LONG_RUNNING_TASKS, SESSION_STORE, …) | `python/packages/core/agent_framework/_feature_stage.py:43-66` |
| Feature usage (.NET parity) | `FeatureUsage.MarkUsed(index)` with same `AGENT_FRAMEWORK_FEATURE_MASK_DISABLED` switch, 128-bit registry, UA token | `dotnet/src/Microsoft.Agents.AI.Abstractions/FeatureUsage.cs:21-98` |
| DI-based agent registration (.NET) | `AddAIAgent(name, instructions, lifetime)` overloads on `IHostApplicationBuilder`; keyed chat-client resolution | `dotnet/src/Microsoft.Agents.AI.Hosting/HostApplicationBuilderAgentExtensions.cs:25-96` |
| Env-var convention (.NET) | Samples fail fast: `Environment.GetEnvironmentVariable("FOUNDRY_PROJECT_ENDPOINT") ?? throw new InvalidOperationException(...)`; convention codified in dotnet AGENTS.md | `dotnet/samples/02-agents/AgentSkills/Agent_Step01_FileBasedSkills/Program.cs:17-18`; `dotnet/AGENTS.md` (Key Conventions → Config) |
| Foundry hosting keys (.NET) | `FOUNDRY_HOSTING_ENVIRONMENT` documented as platform-populated detection key; `PORT` with default listen port 8088 | `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ServiceCollectionExtensions.cs:382-392` |
| Local/hosted store parity | Same server persists sessions/checkpoints/approvals to files under `~/.agentserver/state_stores` locally (`AGENTSERVER_STATE_ROOT` override) vs Foundry storage when hosted; user isolation automatic when hosted | `python/packages/foundry_hosting/README.md` (State store section) |
| DevUI auth fail-closed (Python) | Refuses to disable auth or run token-less on non-loopback binds; auto-generates token logged at startup; `DEVUI_AUTH_TOKEN` fallback | `python/packages/devui/agent_framework_devui/_server.py:145-169` |
| DevUI modes | `mode='developer'` (full access, verbose errors) vs `'user'` (restricted APIs, generic errors) | `python/packages/devui/agent_framework_devui/_server.py:77-89,167-173` |
| DevUI options (.NET) | Loopback-only default, `AllowRemoteAccess` opt-in, constant-time bearer-token compare, `DEVUI_AUTH_TOKEN` const, insecure-exposure warning | `dotnet/src/Microsoft.Agents.AI.DevUI/DevUIOptions.cs:17-58`; `dotnet/src/Microsoft.Agents.AI.DevUI/DevUIExtensions.cs:97-104` |
| Documented hosting menu (Python) | A2A / Azure Functions / Durable Task / Foundry Hosted Agents / self-hosted protocol helpers, each with selection guidance | `python/samples/04-hosting/README.md:5-21` |
| Push-button cloud deploy (Python) | DevUI `DeploymentManager` shells out to `az containerapp up`, injecting `DEVUI_AUTH_TOKEN` as env var, reusing existing Container App Environment | `python/packages/devui/agent_framework_devui/_deployment.py:340-380` |
| Aspire DevUI aggregation (.NET) | In-process Kestrel aggregator AppHost resource; entity-prefix routing across multiple agent services | `dotnet/src/Aspire.Hosting.AgentFramework.DevUI/README.md` (How it works) |
| Declarative config-driven agents | YAML agent definitions loaded via `DeclarativeLoader` with typed errors | `python/packages/declarative/agent_framework_declarative/_loader.py:340-684` |
| Dev toolchain pinning | uv-managed Python 3.10–3.13 dev env; poethepoet tasks; CI matrix runs 3.10–3.14 | `python/DEV_SETUP.md:33-60`; `.github/workflows/python-tests.yml:19-47` |
| .NET build pinning | `global.json` pins SDK 10.0.303 with `rollForward: minor`, no prerelease | `dotnet/global.json:2-7` |

## Answers to Dimension Questions

**1. Is configuration layered?**
Yes, explicitly on the Python side: `load_settings()` implements a four-layer precedence (overrides > `.env` > env vars > defaults) and documents that "`None` values are ignored so that callers can forward optional parameters without masking env-var / default resolution" (`python/packages/core/agent_framework/_settings.py:199-247`). Layering within a signal is also implemented for OpenTelemetry: base endpoints are overridden by signal-specific ones and base headers merge with signal-specific headers (`python/packages/core/agent_framework/observability.py:585-627`), verified by tests such as `test_get_exporters_from_env_with_individual_endpoints` (`python/packages/core/tests/core/test_observability.py:1047`). On .NET, layering is inherited from the host application's `Microsoft.Extensions.Configuration` rather than implemented in-repo; in-repo .NET config consists of DI extension methods and direct env-var reads in samples/tests.

**2. Are environments managed cleanly?**
Cleanly for a library: there is no in-framework "environment" concept; instead, behavior adapts to detected environments. Hosted-vs-local switching is centralized around `FOUNDRY_HOSTING_ENVIRONMENT` (Python: `python/packages/core/agent_framework/_telemetry.py:86-103`; .NET: internal const documented as "the documented way for container code to detect a Foundry context" at `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ServiceCollectionExtensions.cs:382-388`). The Foundry hosting README specifies concrete local persistence defaults (`~/.agentserver/state_stores`, `AGENTSERVER_STATE_ROOT` override) and automatic per-user isolation when hosted — the same binary runs in both contexts with different backing stores. Developer toolchain environments are managed deterministically (pinned SDK, uv matrix, poe tasks). No evidence found of a dedicated staging environment model anywhere in the source; search boundary was the full `python/`, `dotnet/`, and `.github/workflows/` trees.

**3. Are deployment modes documented?**
Yes, unusually well for an SDK. `python/samples/04-hosting/README.md:5-21` presents a decision table of five hosting options (A2A, Azure Functions, Durable Task, Foundry Hosted Agents, self-hosted helpers) with per-option entry points and shared prerequisites (`FOUNDRY_PROJECT_ENDPOINT`/`FOUNDRY_MODEL`, `az login`). On .NET, each hosting package ships a README documenting its mode — e.g., the Aspire DevUI aggregator explains in-process entity-prefix routing (`dotnet/src/Aspire.Hosting.AgentFramework.DevUI/README.md`), and Foundry.Hosting documents port/listen semantics (`ServiceCollectionExtensions.cs:390-392`). The Python DevUI additionally operationalizes one deployment path end-to-end via `az containerapp up` including auth-token injection (`python/packages/devui/agent_framework_devui/_deployment.py:340-357`).

**4. Are feature flags supported?**
Partially — three distinct mechanisms exist, but none is a classic runtime feature-flag service integration:
- **Feature-stage lifecycle gating** (closest analog): `@experimental(feature_id=...)` and `@release_candidate(...)` decorators wrap classes/callables to emit deduplicated one-time warnings, inject docstring warnings, and tag metadata attributes (`python/packages/core/agent_framework/_feature_stage.py:26-28,386-455`). The `ExperimentalFeature` enum inventories 14 gated features (`_feature_stage.py:43-66`). This gates API maturity, not rollout.
- **Feature-usage telemetry mask**: process-global 128-bit registry appended to User-Agent as `(feat=vN.hex)`, identical design in both languages (`python/packages/core/agent_framework/_telemetry.py:137-176`; `dotnet/src/Microsoft.Agents.AI.Abstractions/FeatureUsage.cs:43-98`), disableable via `AGENT_FRAMEWORK_FEATURE_MASK_DISABLED`.
- **Telemetry kill switches**: `AGENT_FRAMEWORK_USER_AGENT_DISABLED` (`_telemetry.py:18-20`).
No evidence found of percentage rollouts, per-tenant flags, or integration with a flag service (searched `feature.flag|FEATURE_FLAG` across `python/packages/core` and `AppContext.SetSwitch|FeatureSwitch` across `dotnet/src`).

**5. Is configuration validated?**
Yes on Python, defensively on .NET. Python validates post-resolution: required single fields raise `SettingNotFoundError` with the exact env var name to set (`python/packages/core/agent_framework/_settings.py:296-305`), mutually-exclusive groups enforce exactly-one (`_settings.py:307-317`), override values are type-checked (`_check_override_type`, `_settings.py:131-187`), and string→type coercion failures fall back gracefully rather than crashing (`_settings.py:281-286`). All covered by tests (`test_settings.py:214-259`). Secrets get masking (`SecretString`). On .NET, validation is guard-clause style (`Throw.IfNull(builder)` at `HostApplicationBuilderAgentExtensions.cs:27`) plus runtime `InvalidOperationException`s for misconfiguration such as double-registration of the Responses Server SDK (`Foundry.Hosting/ServiceCollectionExtensions.cs:415-426`) and DevUI insecure-exposure warnings (`DevUIExtensions.cs:97-104`); no DataAnnotations-based `IOptions` validation pipeline was found in-repo.

## Architectural Decisions

1. **Replace pydantic-settings with a dependency-free loader.** `_settings.py` states it "replaces the previous pydantic-settings-based `AFBaseSettings` with a lighter-weight, function-based approach" (`python/packages/core/agent_framework/_settings.py:5-8`). Tradeoff: less ecosystem machinery (no nested models, no validators DSL) in exchange for zero extra dependencies and TypedDict-native typing; validation is narrower (documented skip rules for `Literal[...]` and complex annotations at `_settings.py:160-170`).
2. **Environment detection over environment configuration.** Rather than asking operators to declare "this is prod", the SDK detects the Foundry hosting context from a platform-injected variable and adjusts (UA prefix, credential choice — `DefaultAzureCredential` only when hosted, `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ServiceCollectionExtensions.cs:668-670`; strictness of hosted-context handling, `AgentFrameworkResponseHandler.cs:119-136`).
3. **Fail-closed development surfaces.** DevUI treats its own metadata as sensitive ("system instructions, tool definitions… sensitive in production contexts", `dotnet/src/Microsoft.Agents.AI.DevUI/DevUIOptions.cs:9-16`) and enforces loopback-only defaults in both languages.
4. **Cross-language behavioral parity for telemetry.** The Python and .NET feature-mask implementations share the same env var names, bit-width, token format, and UA comment syntax (`_telemetry.py:21-22,70` vs `FeatureUsage.cs:23-24,96-98`), enabling backend-side analytics across languages.
5. **Hosting as composable packages, not a monolith server.** Deployment capability is delivered as separate packages (`hosting`, `hosting-a2a`, `foundry_hosting`, `hyperlight` on Python; `Microsoft.Agents.AI.Hosting*`, `Aspire.Hosting.AgentFramework.DevUI` on .NET), letting applications adopt only the topology they need.

## Notable Patterns

- **TypedDict-as-schema settings**: settings schemas are plain `TypedDict` classes; the loader derives field names/types via `get_type_hints` and computes env var names as `<PREFIX><FIELD_UPPER>` (`python/packages/core/agent_framework/_settings.py:250-267`).
- **Callable-tolerant validation**: override type checks always admit callables so lazy token providers can be passed where strings are expected (`_settings.py:141-143`).
- **Caller-frame-resolved warnings**: experimental warnings use stack walking to attribute the warning to the user's call site, not the decorator internals (`_feature_stage.py:202-233`).
- **Same-name env contract across languages**: e.g. `DEVUI_AUTH_TOKEN` is honored identically by Python (`_server.py:154`) and .NET (`DevUIOptions.cs:23`), easing mixed-language fleets.
- **Deployment reuse heuristic**: the Container Apps deploy path prefers discovering an existing Container App Environment "avoids needing environment creation permissions … cost efficient, no side effects" (`_deployment.py:344-346`).

## Tradeoffs

- **Two config idioms, one repo**: Python's prefix-based env loading vs .NET samples' inline `Environment.GetEnvironmentVariable` means the same logical setting (model name, endpoint) has language-specific wiring; nothing in-repo validates that both stacks agree on names beyond convention (drift risk, e.g. Python `ANTHROPIC_CHAT_MODEL_NAME`-style names vs `GEMINI_`/`GOOGLE_` dual-prefix support at `gemini/_chat_client.py:363-371`).
- **`.env` precedence quirk**: an explicitly provided `.env` file *overrides* real environment variables (`test_dotenv_overrides_env_vars_when_env_file_path_is_set`, `test_settings.py:109`), which inverts the common 12-factor expectation; it is intentional and tested but surprising.
- **Coercion leniency**: failed env-var coercions silently keep the raw string instead of raising (`_settings.py:281-286`), favoring availability over strictness.
- **Detection-by-env-var fragility**: `FOUNDRY_HOSTING_ENVIRONMENT` presence toggles security/credential posture (e.g. strict hosted handling at `AgentFrameworkResponseHandler.cs:136`); a wrongly set/unset value changes production behavior — mitigated only by the platform owning the variable.

## Failure Modes / Edge Cases

- Missing required setting surfaces as `SettingNotFoundError` naming both the parameter and env var (`_settings.py:300-305`) — good operator ergonomics.
- Providing a nonexistent `env_file_path` raises `FileNotFoundError` eagerly (`_settings.py:239-241`), tested at `test_settings.py:145-147`.
- Multiple-of-exclusive-group set raises with all offending names listed (`_settings.py:312-317`).
- Feature indexes outside 0..127 throw (`_telemetry.py:143-144`; `FeatureUsage.cs:50-53`) — but note asymmetry: .NET caches `s_isDisabled` statically at class-load time (`FeatureUsage.cs:28`), while Python re-reads its disable env vars on every call (`_telemetry.py:132-134`), so .NET cannot honor a mid-process toggle.
- DevUI non-loopback binds without tokens raise immediately rather than starting exposed (`_server.py:157-158`).
- Declarative YAML loaders raise typed `DeclarativeLoaderError`s for missing files and unsupported definition kinds (`_loader.py:340,465,512`).

## Future Considerations

- Unify provider env-var naming into a shared cross-language manifest to prevent drift (both stacks already share some names like `FOUNDRY_PROJECT_ENDPOINT`, `DEVUI_AUTH_TOKEN`).
- Add `IOptions<T>` + DataAnnotations validation on the .NET side to reach parity with Python's post-resolution validation story.
- Introduce explicit environment profiles (dev/staging/prod) if the framework grows first-party server deployments beyond hosting adapters.
- Consider making the .NET `FeatureUsage` disabled-state read dynamic to match Python's per-call check (`FeatureUsage.cs:28` vs `_telemetry.py:132-134`).

## Questions / Gaps

- No staging-environment concept exists in-source; whether teams layer one above the SDK is out of observable scope. (Searched `staging`, `environment` concepts in workflows and packages.)
- No runtime feature-flag integration (launch darkly-style, AppContext feature switches) was found in either stack; the question "are feature flags supported" is answered only in the lifecycle-gating sense.
- The Go implementation is a pointer to an external repository (`go/README.md:1-3`), so its configuration shape could not be studied here.
- `FoundryEnvironment` (.NET) and `AgentConfig.from_env()` (Python) resolve hosted state partly inside the external Azure AI Agent Server SDKs, so their full precedence rules are outside this source boundary (referenced at `dotnet/src/Microsoft.Agents.AI.Foundry.Hosting/ServiceCollectionExtensions.cs:397,505`; `python/packages/core/agent_framework/_telemetry.py:114-121`).

---

Generated by dimension 22.02 (Configuration and Deployment Shape) against `agent-framework`.
