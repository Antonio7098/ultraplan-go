# Source Analysis: openhands

## Adapter and Interop Boundary Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite + React Router) + Python SDK backend via `@openhands/typescript-client` |
| Analyzed | 2026-08-27 |

## Summary

OpenHands in this repo is **only the frontend (agent-canvas)** — the multi-repo boundary is enforced by design (`AGENTS.md:31-55`). Adapter/interop logic is therefore concentrated in a thin service-adapter layer in `src/api/` that delegates every backend protocol to typed SDK clients from `@openhands/typescript-client`. A CI-enforced guard (`src/api/no-direct-agent-server-calls.test.ts:7-79`) makes direct `axios`/`fetch`/`HttpClient` usage a failure, with only three whitelisted infra files. Core protocols are **adapter-layer, not core**: local agent-server (via SDK clients), cloud App API and cloud runtime sandbox (via `callCloudProxy`/`CloudClient`), ACP (Agent Client Protocol) as subprocess config, and MCP/Plugins/Skills as typed-client wrappers. Runtime backend selection is swappable (local vs cloud), but MCP/ACP provider definitions and Skills catalog are build-time artifacts — adding a new protocol/provider requires core frontend changes and a dependency bump. Tests are unit-level adapter tests plus Playwright mock-LLM E2E; formal conformance/schema contract tests are absent. Interop boundaries are documented in `docs/ACP_AGENTS.md` and `AGENTS.md` but split across code comments rather than a single interop spec.

## Rating

**6 / 10** — Present but inconsistently extensible.

*Rationale:* Clear adapter model with explicit typed-client interfaces and an operational guard (`no-direct-calls` test, centralized `getAgentServerClientOptions`). Operational safeguards exist (auth header injection, secret redaction, encrypted-secret round-trip). However adapters are **not swappable at runtime** as plugins and new protocols require core code changes (ACP provider UI table, MCP transport patching, Skills catalog bump). Conformance testing is weak: unit mocks of SDK clients, no contract/schema validation against OpenAPI/ACP/MCP specs, no adapter conformance suite. Documentation is good for ACP but fragmented for the overall boundary.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Core protocol abstraction | Typed-client-only rule: local calls must use `@openhands/typescript-client` clients via `getAgentServerClientOptions`/`getAgentServerHttpClientOptions`; raw `axios`/`fetch`/`HttpClient` is a violation except 3 files | `src/api/no-direct-agent-server-calls.test.ts:7-11,33-78` |
| Core protocol abstraction | Centralized host/apiKey/workingDir resolution via `getAgentServerClientOptions` and `getAgentServerHttpClientOptions`; `NoBackendAvailableError` sentinel | `src/api/agent-server-client-options.ts:52-79` |
| Adapter implementations — local | `ConversationClient`, `MCPClient`, `PluginsClient`, `SkillsClient`, `ServerClient`, `FileClient`, `VSCodeClient`, `RemoteWorkspace`, `RemoteEventsList` all via SDK | `src/api/mcp-service/mcp-service.api.ts:1` , `src/api/plugins-service.ts:1-3` , `src/api/skills-service.ts:1,48` , `src/api/event-service/event-service.api.ts:1-2` , `src/api/conversation-service/agent-server-conversation-service.api.ts:8` |
| Adapter implementations — cloud | `callCloudProxy` envelope POST to local `/api/cloud-proxy` to avoid CORS; `createCloudClient`/`createCloudClientForRuntime` wrap `CloudClient` with proxy base + `X-Org-Id` scoping | `src/api/cloud/proxy.ts:18-39` , `src/api/cloud/client.ts:33-62` |
| Adapter implementations — cloud vs local branching | Every service branches on `getActiveBackend().backend.kind === "cloud"` (event history vs runtime sandbox, plugin catalog, MCP test short-circuit) | `src/api/event-service/event-service.api.ts:46-58,108-164` , `src/api/plugins-service.ts:77-79,104-106` , `src/api/mcp-service/mcp-service.api.ts:193-195` |
| Adapter implementations — runtime services interop | `RuntimeServicesInfo` shape + `parseRuntimeServicesInfo`/`fetchBackendRuntimeServicesInfo`/`buildRuntimeServicesSystemSuffix` injected as `<RUNTIME_SERVICES>` system prompt suffix | `src/api/agent-server-adapter.ts:128-300` |
| Plugin/extension points — Skills | `SKILLS_CATALOG` from `@openhands/extensions/skills` mapped via `buildBundledSkills()` and merged into `agent_context.skills`; `PUBLIC_SKILLS` is immutable build-time snapshot | `src/api/agent-server-adapter.ts:722-747` , `src/api/skills-service.ts:34,48-56` |
| Plugin/extension points — Plugins | `PluginsService.getPluginsMarketplace()` and `getLocalPlugins()` via `PluginsClient`; `PluginsManagementService` adds install/enable/disable/uninstall/refresh via `PluginsClient` management methods (pending SDK export) | `src/api/plugins-service.ts:76-117` , `src/api/plugins-management-service.ts:36-63,76-133` |
| Plugin/extension points — MCP catalog patching | `getMcpMarketplaceCatalog` + `patchLinearEntry`/`patchGitHubEntry` + `getDeploymentMode` from `runtime_services.mode === "docker"`; GitHub Go binary pre-installed | `src/utils/mcp-marketplace-utils.ts:110-124` , `AGENTS.md` (MCP catalog runtime patching section) |
| ACP adapter | `ACP_PROVIDERS` enriched from `@openhands/typescript-client` registry + Canvas UI metadata; `buildAcpAgentSettingsDiff`, `getAcpProviderSecrets`, `resolveEffectiveAcpModel` | `src/constants/acp-providers.ts:151-167,46-65,359-385,484-526` |
| ACP auth probe adapter | Per-provider login detection via `BashClient.executeCommand` (Claude/Codex/Gemini probes) with `classify*` parsers | `src/api/acp-service/acp-service.api.ts:70-108` |
| Backend registry — swappable runtime backend | `pickFallbackBackend`, `computeSnapshot`, `getActiveBackend`, `getEffectiveLocalBackend` (only active local eligible), `subscribeActiveBackend` | `src/api/backend-registry/active-store.ts:52-62,64-93,128-144,180-185` |
| Toolset gate (adapter filtering) | `getAgentTools` filters `browser_tool_set`/`task_tool_set` via `VITE_ENABLE_BROWSER_TOOLS` and `isAgentServerToolAvailable` + `enable_sub_agents` | `src/api/agent-server-adapter.ts:631-677` |
| Conformance tests — guard | Architectural conformance test that scans `src/**` for violations | `src/api/no-direct-agent-server-calls.test.ts:32-79` |
| Conformance tests — service units | Mocked `MCPClient`/`PluginsClient` unit tests covering credential substitution, OAuth flows, marketplace catalog | `src/api/mcp-service/mcp-service.api.test.ts:23-238` , `src/api/plugins-service.test.ts:35-159` , `src/api/agent-server-adapter.test.ts:39-259` |
| Conformance tests — E2E | Mock-LLM E2E exercises real agent-server + mock LLM + automation via ingress; `scripts/runtime-services-info.mjs` rendering tested via `getMockLLMRequests()` | `AGENTS.md` (Mock-LLM E2E section) |
| Protocol documentation | ACP overview (JSON-RPC over stdio, credential flow, switching), architecture module map | `docs/ACP_AGENTS.md:10-27,37-46` , `docs/architecture.md:37` , `AGENTS.md:31-55` |
| Interop boundary — MCP health | `interpretMcpTestResponse`, `probeMcpServerHealth`, `finalizeMcpTestResponse` maps SDK `MCPTestResponse` to `McpServerHealth` with credential reinterpretation | `src/api/mcp-health/probe-mcp-server-health.ts:46-88` , `src/api/mcp-service/mcp-service.api.ts:100-122` |

## Answers to Dimension Questions

**1. Are protocols core or adapter-layer?**
Adapter-layer. No protocol logic lives in core React render code; all interop is delegated to `src/api/` service adapters that wrap `@openhands/typescript-client` typed clients (`src/api/no-direct-agent-server-calls.test.ts:33-78` enforces this, `AGENTS.md:31-55` declares the repo boundary). Even ACP — a stdio JSON-RPC standard (`docs/ACP_AGENTS.md:10-15`) — is surfaced only as configuration (`acp_server`/`acp_command`/`acp_model` in `src/constants/acp-providers.ts:151-167` and `src/api/agent-server-adapter.ts:817-881`) that the Python agent-server spawns; the frontend never implements the ACP wire protocol. MCP is similarly adapter-layer via `MCPClient` in `src/api/mcp-service/mcp-service.api.ts:143-209` (stdio/sse/shttp transports). Skills are a build-time catalog adapter (`src/api/skills-service.ts:34` + `src/api/agent-server-adapter.ts:722-747`).

**2. Can adapters be added without core changes?**
No — for new *protocol kinds*, Yes — for new *instances* of existing kinds. Adding a new ACP provider requires SDK + `typescript-client` release and a Canvas entry in `ACP_PROVIDER_UI` (`src/constants/acp-providers.ts:129-145` → `ACP_PROVIDERS` map). Adding a new MCP transport semantics or patching catalog entries required changes to `mcp-marketplace-utils` + Docker pre-install (documented in `AGENTS.md` Linear/GitHub patch notes). Adding a new Skills catalog entry requires bumping `@openhands/extensions` and rebuilding. However, adding a new **instance** of an existing protocol (e.g., a custom ACP command via `custom` preset `src/constants/acp-providers.ts:169`, a new MCP server via `patchMcpServer/createMcpServer` `src/api/settings-service/settings-service.api.ts:556-601`, or a new plugin via `PluginsManagementService.installPlugin` `src/api/plugins-management-service.ts:92-101`) needs no core change — it is data-driven at runtime via settings patches.

**3. Are adapters tested for conformance?**
Partially, and weakly. The strongest conformance guard is the **architectural lint** (`src/api/no-direct-agent-server-calls.test.ts:32-79`) that fails CI if adapters bypass the typed-client boundary. Service adapters have **unit tests with mocked SDK clients** (`src/api/mcp-service/mcp-service.api.test.ts:23-238`, `src/api/plugins-service.test.ts:35-159`, `src/api/agent-server-adapter.test.ts:39-259`) covering credential substitution, OAuth probe flows, encrypted-secret round-trips, and disabled-skill filtering. Playwright mock-LLM E2E (`AGENTS.md` Mock-LLM section, `playwright.mock-llm.config.ts`) exercises the full stack through real `agent-server` + Python mock LLM (`tests/e2e/mock-llm/scripts/mock-llm-server.py` via `TestLLM`). Missing: no schema/contract tests against OpenAPI, no MCP JSON-RPC schema validation, no ACP subprocess contract suite, no `typescript-client` version compatibility matrix tested in CI beyond `assertAgentServerVersionIsSupported` in `src/api/agent-server-compatibility.ts`.

**4. Are interop boundaries documented?**
Partially. `docs/ACP_AGENTS.md:1-239` documents the ACP boundary thoroughly (diagram, provider table, auth precedence, container credential materialization, isolation caveats). `AGENTS.md` (Repository Map, API Access Rules, MCP catalog patching, Cloud conversation resume gating) documents the frontend↔agent-server and frontend↔cloud proxy boundaries with code references. `docs/architecture.md:37` lists the `src/api/` adapter areas at high level. Gaps: no single `docs/interop.md` or OpenAPI reference; MCP marketplace patching rationale lives only in `AGENTS.md` comments and `src/utils/mcp-marketplace-utils.ts` code; the `LookupSecret` uniformity and `secrets_encrypted` Fernet semantics are documented only in code comments (`src/api/agent-server-adapter.ts:1202-1228`, `docs/ACP_AGENTS.md:165-181`).

## Architectural Decisions

| Decision | Evidence | Effect |
|----------|----------|--------|
| Typed-client-only gateway with CI guard | `src/api/no-direct-agent-server-calls.test.ts:7-11,32-78` + `src/api/agent-server-client-options.ts:52-79` | Forces all local protocol traffic through versioned SDK; prevents ad-hoc HTTP drift but couples frontend release to `typescript-client` release |
| Cloud calls via local proxy (`callCloudProxy`) | `src/api/cloud/proxy.ts:18-39` + `src/api/cloud/client.ts:33-55` + branching in `src/api/event-service/event-service.api.ts:46-164` | Avoids browser CORS to cloud/runtime hosts; splits App API (bearer, `backend.host`) vs runtime sandbox (`hostOverride` + session key) correctly but hardcodes the split per service |
| Build-time Skills catalog snapshot | `src/api/skills-service.ts:34` + `src/api/agent-server-adapter.ts:722-747,769-782` `load_public_skills: false` | Eliminates runtime extensions repo clone; updating catalog requires dependency bump + rebuild — durable but not runtime-extensible |
| ACP provider registry mirrored from SDK | `src/constants/acp-providers.ts:151-167` `getClientAcpProvider` + `ACP_PROVIDER_UI` | Adding provider is upstream SDK work; Canvas only owns icon/i18n — reduces drift but makes new ACP provider a multi-repo change |
| Per-provider Bash probes for ACP auth detection | `src/api/acp-service/acp-service.api.ts:70-108` | No dedicated agent-server endpoint; reuses existing `BashClient.executeCommand` — zero model cost but fragile to CLI output changes |
| MCP health re-interpretation layer | `src/api/mcp-health/probe-mcp-server-health.ts:46-88` + `src/api/mcp-service/mcp-service.api.ts:100-122` | Turns generic `connection`/`unknown` failures into actionable `credentials` failures and verifies read-only probe tool; prevents silent credential misreporting |

## Notable Patterns

- **Adapter-per-concern with `kind` discriminator**: `src/api/plugins-service.ts:76-79` (catalog vs `plugins-management-service`), `src/api/skills-service.ts` vs `src/api/plugins-service.ts`, `src/api/mcp-service/mcp-service.api.ts:193-195` (cloud short-circuit returns synthetic success), `src/api/event-service/event-service.api.ts:46-58` — uniform `if (kind === "cloud")` branching keeps local SDK path pure.
- **Centralized client-option factory**: `getAgentServerClientOptions` / `getAgentServerHttpClientOptions` in `src/api/agent-server-client-options.ts:52-79` is the sole place host/apiKey/workingDir resolution lives; all SDK constructors consume it, enabling backend-registry swaps without changing call sites.
- **Secret uniformization via `LookupSecret`**: `src/api/agent-server-adapter.ts:995-999,1202-1228` — every saved secret (LLM key, ACP OAuth token, MCP bearer) rides the same `{ kind: "LookupSecret", url: "/api/settings/secrets/..." }` shape with loopback auth headers; ACP off-loop resolution noted at `src/api/agent-server-adapter.ts:1206-1207`.
- **Build-time catalog + runtime patch**: `src/utils/mcp-marketplace-utils.ts:31-84` filters installable options; `AGENTS.md` Linear SSE→shttp + GitHub `docker→github-mcp-server` patches keyed on `getDeploymentMode()` (`src/api/agent-server-adapter.ts:202-206`) — demonstrates patch-not-fork extensibility.
- **Version gating via `assertAgentServerVersionIsSupported`**: `src/api/agent-server-compatibility.ts` + `config/defaults.json:compatibility.minimumAgentServer` (referenced in `AGENTS.md`) — frontend refuses incompatible servers at bootstrap.

## Tradeoffs

| Tradeoff | Benefit | Cost |
|----------|---------|------|
| Strict typed-client gateway | Type-safe, single integration point, auto-picked up version pins in `config/defaults.json` | Frontend blocked until `typescript-client` publishes new SDK methods (e.g., `acp_isolate_data_dir` comment `src/api/agent-server-adapter.ts:831-836`, `plugins-management-service.ts:36-40` cast pending PR #222/#223) |
| Local proxy for cloud | No CORS, browser never holds cloud secrets directly | Extra local hop; `NoBackendAvailableError` when proxy absent (`src/api/cloud/client.ts:58-62`); runtime-sandbox `hostOverride` must be supplied per call |
| Build-time Skills catalog | Zero latency, no Git clone, deterministic bundle | Stale until rebuild; `AGENTS.md` notes migration from `EXTENSIONS_REF` clone to npm catalog |
| Cloud adapter returns empty/synthetic on unsupported | UI never hard-fails on cloud where feature absent (`src/api/plugins-service.ts:78-89`, `src/api/mcp-service/mcp-service.api.ts:193-195`) | Failures silent — real connection errors surface only later in sandbox runtime, not at install time |
| Single `agent-server-adapter.ts` enriches `agent_settings` | Centralizes LLM default, tool gating, skill merging, `RuntimeServicesInfo` suffix | 1300-line module mixes MCP encryption checks, ACP fallbacks, client-tool stamping; profile vs inline `agent_settings` boundary comment (`src/api/agent-server-adapter.ts:1087-1106`) is the only interop spec |

## Failure Modes / Edge Cases

- **Stale `typescript-client` pin**: ACP field `acp_isolate_data_dir` supported server-side (`software-agent-sdk#3492`) but not in pinned `@openhands/typescript-client` — sending it risks validation error on older servers (`src/api/agent-server-adapter.ts:831-836`). Same for `PluginsManagementClient` interface cast (`src/api/plugins-management-service.ts:42-53`).
- **Cloud MCP probe short-circuit masks real failures**: `McpService.testServer` returns `{ ok: true, tools: [] }` on cloud (`src/api/mcp-service/mcp-service.api.ts:193-195`) so install proceeds; actual connection failure only appears inside conversation runtime — no pre-flight.
- **Missing local backend for OAuth probe**: `getMcpProbeOptions()` throws `OAuth authorization requires a reachable local backend` when no local registered (`src/api/mcp-service/mcp-service.api.ts:124-141`) — cloud-active users with an MCP that requires OAuth cannot re-authorize.
- **ACP probe brittleness**: Gemini probe checks `test -f "$HOME/.gemini/oauth_creds.json"` (`src/api/acp-service/acp-service.api.ts:80-81`); Codex/Claude regex on stdout/stderr (`src/api/acp-service/acp-service.api.ts:48-52`) — CLI output change or missing CLI yields `unknown` (safe fallback but loses optimization).
- **Profile vs inline `agent_settings` split**: `agent_profile_id` and `agent_settings` are mutually exclusive; Canvas enrichments (default toolset `terminal`/`file_editor`/`task_tracker`, bundled skills) on the profile path are the server's responsibility (`software-agent-sdk#3967` tracked at `src/api/agent-server-adapter.ts:1088-1106`). If server fix regresses, profile-launched agents silently lose default tools/public skills.
- **Cloud event pagination fallback truncation**: `EventService.searchEvents` catches pagination-support 500s and returns empty page to avoid infinite dedup loop (`src/api/event-service/event-service.api.ts:149-162`) — older cloud backends lose older-event pagination rather than surfacing error.
- **Encrypted-secret detection fragile**: `hasEncryptedMcpSecrets` sniffs `gAAAAA` Fernet prefix (`src/api/agent-server-adapter.ts:540-590`); non-Fernet encryption or prefix collision causes wrong `secrets_encrypted` flag.

## Future Considerations

- Expose `acp_isolate_data_dir` once `typescript-client` surfaces it, gated on server version (tracked `agent-canvas#1019`, `software-agent-sdk#3492`) — enables per-conversation HOME isolation for concurrent same-provider ACP conversations (`docs/ACP_AGENTS.md:205-213`).
- Replace build-time Skills snapshot with a versioned remote catalog fetch (like MCP) to allow runtime updates without rebuild; requires agent-server endpoint + cache invalidation.
- Introduce a formal adapter conformance suite: OpenAPI snapshot tests for `typescript-client` methods used by Canvas, JSON-RPC schema validation for MCP test responses, and ACP `initialize`/`session/new` contract tests via the Python mock ACP server (`tests/e2e/mock-llm/scripts/mock-acp-server.py`).
- Promote MCP marketplace patches (`patchLinearEntry`/`patchGitHubEntry` in `AGENTS.md`) to a declarative `catalog-overrides.json` so new patches don't require core code changes.
- Document the full interop matrix (local / cloud App API / cloud runtime sandbox) in a single `docs/interop-boundaries.md` instead of scattering across `AGENTS.md` and code comments; include sequence diagrams for `LookupSecret` resolution and `callCloudProxy` hop.
- Add a `Plugin`/`MCP` adapter interface (Strategy pattern) so a new protocol can be registered via a module implementing `test/install/enable/health` rather than editing multiple service files.

## Questions / Gaps

- No evidence of adapter version-negotiation beyond `minimumAgentServer` floor — how does Canvas behave when agent-server introduces a breaking change within a supported range? `src/api/agent-server-compatibility.ts:1-60` only checks floor, not feature detection.
- No evidence of adapter observability (metrics/traces per protocol call, latency by `kind`, error-kind breakdown) — only functional health via `useBackendsHealth` polling (`AGENTS.md` Backend dropdown connectivity section).
- `src/api/cloud/client.ts:58-62` `createCloudClientForRuntime` throws `NoBackendAvailableError` if no proxy — is this path exercised on Vercel/hosted deployments where no local proxy exists?
- `docs/architecture.md:37` claims `src/api/` covers automations, but automation adapter (`src/api/automation-service/automation-service.api.ts`) bypasses the typed-client guard via allowed `axios` — is this an intentional permanent exception or pending migration to `typescript-client`?
- No dedicated MCP/ACP/Plugins extension-point manifest or plugin-loader — would new-protocol extensibility require a breaking package.json export change (cf. `package.json:206-247` subpath exports)?

---

Generated by `19.03-adapter-and-interop-boundary-design` against `openhands`.
