# Source Analysis: openhands

## 19.01 Protocol Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite, React Router 7), @openhands/typescript-client 1.38.1 |
| Analyzed | 2026-08-26 |

## Summary

`openhands` in this study is **only the agent-canvas frontend** (`AGENTS.md:10`, `package.json:2`), not the Python agent-server SDK. Protocol compatibility is therefore evaluated as a thin UI/adapter layer over the SDK.

MCP is the only first-class open protocol: full UI, marketplace, custom-server editor, health probing, OAuth, and credential validation all proxy through `MCPClient` from `@openhands/typescript-client` (`src/api/mcp-service/mcp-service.api.ts:1`, `src/types/mcp-server.ts:13`). No MCP server is implemented here — the frontend is MCP client-side only.

No OTLP/OpenTelemetry exporter exists in the frontend (`package.json:22-72` has no `@opentelemetry/*`; grep for `opentelemetry|otel|OTLP` yields 0 hits in `src/`). Observability mentions in `docs/DefenseClaw.md:227` are documentation only.

JSON Schema is used manually for the two Canvas-owned client tools (`src/api/canvas-ui-client-tool.ts:60`, `src/api/launch-child-conversation-client-tool.ts:61`) and consumed as SDK-supplied `agent_settings_schema`/`conversation_settings_schema` (`src/hooks/query/use-agent-settings-schema.ts:7`). No generator (`zod`, `@cfworker/json-schema`, etc.) or validation library is bundled.

OpenAPI is advertised but not consumed: `RuntimeServicesInfo.automation.openapi_url` is built in `scripts/runtime-services-info.mjs:139` and rendered into `AgentContext.system_message_suffix` (`src/api/agent-server-adapter.ts:265`), and `vite.config.ts:431` proxies `/openapi.json` to the agent-server. No OpenAPI-to-tool importer exists; Jira's HTTP/OpenAPI option is explicitly called non-installable (`src/utils/mcp-marketplace-utils.ts:119`).

Model-independent tool schemas are partially achieved: the two client tools use standard JSON Schema (`type: object`, `enum`, `properties`) sent as `client_tools` in `buildStartConversationRequest()` (`src/api/agent-server-adapter.ts:1116`), which the agent-server forwards model-agnostically. However MCP config handling is tightly coupled to the SDK's `MCPConfig`/`MCPServer` types (`src/utils/mcp-config.ts:1`), not a provider-neutral abstraction.

External tools can be added without code only via the MCP marketplace/custom-server path; adding a new non-MCP client tool still requires a code change to register a `ClientToolSpec`.

## Rating

**5 / 10 — Present but inconsistent, weakly documented, or fragile.**

MCP client support is mature and tested (marketplace, health store, credential validation, OAuth, secret redaction), justifying 7-8 in isolation. OTLP is absent, OpenAPI is URL-plumbing only, and JSON Schema is hand-authored without generation/validation. The frontend delegates all server-side protocol work to the SDK (`@openhands/software-agent-sdk` per `AGENTS.md:35`), so portability is bounded by that external contract. No evidence of unified protocol adapter or provider-agnostic tool registry in this repo.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP — marketplace & transport matching | `getMcpConnectionOptions`, `isMcpInstallableEntry`, `transportMatchesServer` handle `shttp`/`sse`/`stdio` and Jira HTTP/OpenAPI exclusion | `src/utils/mcp-marketplace-utils.ts:30`, `src/utils/mcp-marketplace-utils.ts:122`, `src/utils/mcp-marketplace-utils.ts:186` |
| MCP — server type model | `MCPServerType`, `MCPServerConfig` (`sse\|stdio\|shttp`, `url`, `command`, `env`, `auth`, `enabled`) | `src/types/mcp-server.ts:11`, `src/types/mcp-server.ts:13` |
| MCP — auth model | `MCP_AUTH_STRATEGIES`, `isMcpAuthCredential` supporting `none\|api_key\|bearer\|basic\|header\|oauth2` | `src/types/mcp-auth.ts:18`, `src/types/mcp-auth.ts:30` |
| MCP — SDK client adapter | `McpService.testServer` builds `AgentServerMCPTestRequest` via `toMcpServer` and `MCPClient.testServer`; cloud short-circuit `{ok:true, tools:[]}` | `src/api/mcp-service/mcp-service.api.ts:1`, `src/api/mcp-service/mcp-service.api.ts:23`, `src/api/mcp-service/mcp-service.api.ts:179` |
| MCP — OAuth flow | `startOAuth`, `getOAuthStatus`, `authorizeOAuth` with 120s timeout + popup polling loop | `src/api/mcp-service/mcp-service.api.ts:21`, `src/api/mcp-service/mcp-service.api.ts:212`, `src/api/mcp-service/mcp-service.api.ts:248` |
| MCP — config normalization | `parseMcpConfig`, `toCanonicalMcpServer`, `buildMcpServerPatch`, `buildRenameMcpConfigPatch`, `normalizeTransport` mapping `streamable-http→http` | `src/utils/mcp-config.ts:60`, `src/utils/mcp-config.ts:77`, `src/utils/mcp-config.ts:140`, `src/utils/mcp-config.ts:319` |
| MCP — credential validation | Per-entry `VALIDATION_BY_ENTRY_ID` (github `get_me`, linear `list_teams`, slack `slack_list_channels` with `SLACK_AUTH_FAILURES`) | `src/utils/mcp-credential-validation.ts:33`, `src/utils/mcp-credential-validation.ts:52` |
| MCP — health probing | `probeMcpServerHealth`, `interpretMcpTestResponse` with `AUTH_FAILURE_TEXT` sniffing and `verified` vs `connectivity-only` | `src/api/mcp-health/probe-mcp-server-health.ts:22`, `src/api/mcp-health/probe-mcp-server-health.ts:46`, `src/api/mcp-health/probe-mcp-server-health.ts:75` |
| MCP — health store | In-memory `McpHealthMap` with `beginMcpHealthCheck`/`resolveMcpHealthCheck` stale-write guard | `src/api/mcp-health/mcp-health-store.ts:14`, `src/api/mcp-health/mcp-health-store.ts:35`, `src/api/mcp-health/mcp-health-store.ts:50` |
| MCP — routes/UI | Top-level `/mcp` route, marketplace grid, install modal, custom server editor | `src/routes/mcp.tsx:1`, `src/routes/mcp-settings.tsx:1`, `src/components/features/mcp-page/mcp-toolbar.tsx:1` |
| MCP — mocks | MSW handler for `POST /api/mcp/test` returning `{ok:true, tools:[]}` for `dev:mock` | `src/mocks/mcp-handlers.ts:14`, `src/mocks/mcp-handlers.ts:20` |
| MCP — e2e coverage | Mock-LLM MCP specs for GitHub hosted URL and Slack credential validation | `tests/e2e/mock-llm/mcp/mock-llm-mcp-github.spec.ts:30`, `tests/e2e/mock-llm/mcp/mock-llm-mcp-slack-credentials.spec.ts:8` |
| JSON Schema — client tool schemas | `CANVAS_UI_CLIENT_TOOL.parameters` (JSON Schema with `additionalProperties:false`) and `LAUNCH_CHILD_CONVERSATION_CLIENT_TOOL.parameters` | `src/api/canvas-ui-client-tool.ts:60`, `src/api/launch-child-conversation-client-tool.ts:61` |
| JSON Schema — annotations | `annotations` (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) mirroring MCP/tool annotation spec | `src/api/canvas-ui-client-tool.ts:85`, `src/api/launch-child-conversation-client-tool.ts:103` |
| JSON Schema — settings schemas | Frontend consumes `agent_settings_schema`/`conversation_settings_schema` via `useAgentSettingsSchema` and `withLlmSubscriptionSchemaFields` | `src/hooks/query/use-agent-settings-schema.ts:7`, `src/mocks/settings-handlers.ts:68` |
| OpenAPI — URL advertisement | `buildRuntimeServicesInfo` sets `openapi_url: ${automationBaseUrl}${apiPrefix}/openapi.json`; rendered into `<RUNTIME_SERVICES>` suffix | `scripts/runtime-services-info.mjs:139`, `src/api/agent-server-adapter.ts:265` |
| OpenAPI — proxy | Vite dev proxy forwards `/openapi.json` to `API_URL`; ingress/static-server also route it | `vite.config.ts:431`, `docker/entrypoint.sh:400`, `scripts/static-server.mjs:7` |
| OpenAPI — type mirror | `RunStatus` comment `Mirrors RunStatus in the automation service's OpenAPI schema` shows schema is documentation, not codegen | `src/types/automation.ts:65` |
| Provider APIs — agent-server typed clients | All agent-server calls must use `@openhands/typescript-client` clients (`ConversationClient`, `MCPClient`, `ServerClient`, `RemoteWorkspace`, `RemoteEventsList`) via `getAgentServerClientOptions` | `src/api/no-direct-agent-server-calls.test.ts:32`, `src/api/agent-server-client-options.ts:1` |
| Provider APIs — LLM model portability | `buildConfiguredOpenHandsAgentSettings` normalizes `llm.model` fallback to `DEFAULT_SETTINGS.llm_model`; server caches `ClientToolSpec` per name and rejects schema conflicts | `src/api/agent-server-adapter.ts:890`, `src/api/agent-server-adapter.ts:1111` |
| Telemetry — not OTLP | PostHog-only telemetry via `src/services/telemetry.ts` (`agent-canvas` client); no OTel SDK dependency | `package.json:48`, `AGENTS.md:79` |
| Protocol isolation guard | CI enforces no raw axios/fetch to `/api/*`, only allowlisted infra files | `src/api/no-direct-agent-server-calls.test.ts:7` |

## Answers to Dimension Questions

**1. Which open protocols are supported?**

- **MCP (Model Context Protocol) — Full client support:** Marketplace catalog from `@openhands/extensions/integrations`, three transports (`stdio`, `sse`, `shttp`/`streamable-http`) (`src/utils/mcp-config.ts:60`), all auth strategies (`src/types/mcp-auth.ts:18`), OAuth authorization (`src/api/mcp-service/mcp-service.api.ts:212`), health store (`src/api/mcp-health/mcp-health-store.ts:14`), and connectivity+credential probes (`src/utils/mcp-credential-validation.ts:52`). Transport is normalized via `normalizeTransport` and parsed via `parseMcpConfig` (`src/utils/mcp-config.ts:77`).
- **JSON Schema — Consumed + hand-authored:** SDK schemas for settings (`src/hooks/query/use-agent-settings-schema.ts:7`) and manual JSON Schema for two client tools (`src/api/canvas-ui-client-tool.ts:63`, `src/api/launch-child-conversation-client-tool.ts:64`). No generator library.
- **OpenAPI — Advertised only:** Automation `openapi_url` is built (`scripts/runtime-services-info.mjs:139`) and appended to `AgentContext.system_message_suffix` (`src/api/agent-server-adapter.ts:265`). No OpenAPI import-to-tool pipeline; Jira HTTP/OpenAPI catalog entries are explicitly non-installable (`src/utils/mcp-marketplace-utils.ts:119`).
- **OTLP/OpenTelemetry — Absent:** Zero `@opentelemetry/*` dependencies (`package.json:22`), zero OTel imports in `src/`; only doc mention in `docs/DefenseClaw.md:227`.
- **Provider APIs — Via SDK typed clients:** Enforced via `src/api/no-direct-agent-server-calls.test.ts:32`; `MCPClient`, `ServerClient`, `ConversationClient` from `@openhands/typescript-client` (`src/api/mcp-service/mcp-service.api.ts:1`).

**2. Is MCP supported?**

Yes, as a **frontend client/proxy** to the SDK's MCP runtime. Evidence:

- **Tools:** `MCPClient.testServer` lists tools (`response.tools`) and optionally runs a read-only `tool_call` (`src/api/mcp-service/mcp-service.api.ts:62`).
- **Resources/Prompts:** Not exposed in the frontend. The `MCPTestResponse` surface is `tools` + `tool_result` + `oauth_state` (`src/types/mcp-server.ts:36`); no resource/prompt browsing UI found in `src/components/features/mcp-page/`.
- **OAuth:** Full start/status/callback + interactive `authorizeOAuth` with popup polling (`src/api/mcp-service/mcp-service.api.ts:248`). `mcp_health` store tracks OAuth state (`src/types/mcp-server.ts:68`).
- **Transports:** `stdio` (local `command`/`args`/`env`), `sse`, `shttp` (streamable HTTP) (`src/types/mcp-server.ts:11`, `src/utils/mcp-config.ts:60`). Docker patches GitHub's `docker run` to native `github-mcp-server stdio` (`AGENTS.md:220`).
- **Marketplace:** `getMcpMarketplaceCatalog` filters `INTEGRATION_CATALOG` for `provider==mcp` entries (`src/utils/mcp-marketplace-utils.ts:110`, `src/utils/mcp-credential-validation.ts:1`), with `findInstalledMatch` URL matching (`src/utils/mcp-marketplace-utils.ts:166`).

Limitation: The MCP **server process** runs inside the agent-server container (stdio spawn or remote HTTP), not in the frontend. Frontend delegates via `getMcpProbeOptions` local-backend resolution (`src/api/mcp-service/mcp-service.api.ts:124`), and short-circuits to synthetic success for cloud backends (`src/api/mcp-service/mcp-service.api.ts:193`).

**3. Is OpenTelemetry supported?**

No. Search `opentelemetry|otel|OTLP|tracing` in `src/` returns only `automation-debug-prompt.ts` tracebacks and `i18n` strings, zero exporter or span code. `package.json:22-72` lists `posthog-js` for analytics and `winston` for logging, but no `opentelemetry` packages. `vite.config.ts` and `src/services/telemetry.ts` reference only PostHog. The only OTLP string in the repo is `docs/DefenseClaw.md:227` describing `OTLP (Prometheus/Grafana/Honeycomb)` as a deployment option, not implemented. `AGENTS.md:79` confirms telemetry is PostHog-only. No evidence of `OTLPTraceExporter`, `OTLPMetricExporter`, or `sdk-trace` setup.

**4. Are tool schemas portable across providers?**

**Partially.** The two canvas-owned client tools use plain JSON Schema (`type: object`, `properties`, `enum`, `required`, `additionalProperties:false`) (`src/api/canvas-ui-client-tool.ts:63`, `src/api/launch-child-conversation-client-tool.ts:64`) plus standard `annotations` (`readOnlyHint` etc.), which are model-transport agnostic. They are sent as `client_tools: ClientToolSpec[]` in `buildStartConversationRequest()` (`src/api/agent-server-adapter.ts:1116`); `AGENTS.md:240` notes the agent-server caches each tool schema per name for process lifetime (`ClientToolSchemaConflictError`).

However:
- Portability is **bounded by the agent-server**: the frontend hardcodes tool names `canvas_ui_control` and `launch_child_conversation` (`src/constants/canvas-ui.ts:2`, `src/constants/child-conversation.ts:6`), and the agent-server must have registered matching handlers. Changing `parameters` requires restarting the agent-server (`src/api/agent-server-adapter.ts:1111`).
- MCP tool schemas are **SDK-coupled**: `MCPConfig`/`MCPServer` types come from `@openhands/typescript-client` (`src/utils/mcp-config.ts:1`), and the `MCPClient` wire type (`AgentServerMCPTestRequest["server"]`) uses `type: "http"` vs frontend `shttp` (`src/api/mcp-service/mcp-service.api.ts:35`). The mapping is manual, not via a shared JSON Schema registry.
- LLM provider portability for built-ins: `buildConfiguredOpenHandsAgentSettings` vs `buildConfiguredAcpAgentSettings` diverge on `llm.model` vs `acp_model`/`acp_server` (`src/api/agent-server-adapter.ts:881`), but the tool payload itself does not vary per provider. No evidence of provider-specific tool-schema transforms (e.g., Bedrock vs OpenAI function format) in this repo — that lives in the SDK.

So `>Can external tools be added without writing custom adapters?` — **Yes for MCP** (marketplace click or custom server form adds any stdio/shttp server without frontend code), **No for non-MCP client tools** (requires adding a new `ClientToolSpec` constant and wiring it in `buildStartConversationRequest`).

## Architectural Decisions

- **Frontend as thin adapter, SDK as runtime:** `AGENTS.md:31` declares this repo owns only `src/api/` call-site adaptation; tool execution, MCP spawning, and OpenAPI serving belong to `OpenHands/software-agent-sdk`. Evidence: `src/api/mcp-service/mcp-service.api.ts:1` imports `MCPClient` from `@openhands/typescript-client`; `vite.config.ts:431` proxies `/openapi.json` to the agent-server rather than serving it. *Tradeoff:* frontend protocol work is version-pinned to `typescript-client@1.38.1` (`package.json:27`) and cannot evolve independently.
- **Typed-client enforcement via CI guard:** `src/api/no-direct-agent-server-calls.test.ts:32` bans raw `axios`/`fetch` to `/api/*` and `createHttpClient`/`HttpClient` direct use, forcing all protocol traffic through generated clients. *Tradeoff:* safety at cost of indirection; allowed exceptions are narrowly listed (`automation-service.api.ts`, `cloud/proxy.ts`, `main-app-auth.ts`).
- **Runtime-services info as agent system suffix:** Instead of a service-discovery protocol, `scripts/runtime-services-info.mjs:60` builds a JSON blob (agent_server, ingress, frontend, automation + openapi_url/docs_url) that `src/api/agent-server-adapter.ts:215` renders as `<RUNTIME_SERVICES>` markdown injected into `AgentContext.system_message_suffix` (`src/api/agent-server-adapter.ts:784`). *Tradeoff:* human-readable, LLM-friendly, but not machine-parsable; no health/auth validation of advertised URLs.
- **Client-tool schema caching by name:** `src/api/agent-server-adapter.ts:1111` documents the agent-server caches `ClientToolSpec` per `name` and rejects re-registration with a different schema. *Tradeoff:* dev ergonomics penalty (restart agent-server to change tool shape) for runtime stability.
- **Health store with checkId stale-write guard:** `src/api/mcp-health/mcp-health-store.ts:50` only applies `resolveMcpHealthCheck` if entry is still `checking` with matching `checkId`, preventing slow probes from overwriting newer results. *Tradeoff:* correctness over freshness; no retry/backoff logic.

## Notable Patterns

- **MCP transport normalization:** `normalizeTransport` maps legacy `streamable-http`, `http`, `shttp`, `undefined` to canonical `http` or `stdio`/`sse` (`src/utils/mcp-config.ts:60`), and `toCanonicalMcpServer` converts frontend `shttp` to wire `http` (`src/utils/mcp-config.ts:152`).
- **Redacted-secret leaf patching:** `buildMcpServerPatch` uses `buildRedactionSafeNestedPatch` and `buildStringMapPatch` to omit `**********` leaves (`src/utils/mcp-config.ts:207`), and `redactMcpSecrets` scrubs errors/tool results (`src/api/mcp-service/mcp-service.api.ts:68`). OAuth patches have field-level allowlist `OAUTH_EDITABLE_AUTHENTICATION_FIELDS` (`src/utils/mcp-config.ts:200`).
- **Credential validation as interpreter:** `CredentialValidation` pairs a read-only `toolCall` with an `interpret` function; `finalizeMcpTestResponse` only applies interpretation when `tools.includes(toolCall.name)` (`src/api/mcp-service/mcp-service.api.ts:110`), with Slack-specific JSON parsing of `{ok, error}` (`src/utils/mcp-credential-validation.ts:69`).
- **Health verification tiering:** `interpretMcpTestResponse` classifies health as `verified` (probe tool advertised + succeeded) vs `connectivity-only` vs `failed` with `AUTH_FAILURE_TEXT` regex sniffing 401/403 (`src/api/mcp-health/probe-mcp-server-health.ts:22`).
- **Catalog patching for Docker:** `getMcpMarketplaceCatalog` pipes entries through `patchLinearEntry`/`patchGitHubEntry` gated by `getDeploymentMode()` (`AGENTS.md:240`), rewriting Linear SSE→HTTP and GitHub `docker run`→native binary.

## Tradeoffs

- **MCP richness vs no OTel:** Engineering effort is concentrated on MCP UX (marketplace, health, OAuth) with mock handlers (`src/mocks/mcp-handlers.ts:20`) and e2e coverage (`tests/e2e/mock-llm/mcp/*.spec.ts`), while OTel is zero. Teams to evaluate cost/benefit of adding OTel versus staying PostHog-only.
- **Delegation vs self-contained:** By delegating MCP execution to the SDK, the frontend avoids spawning processes but cannot independently version or extend the MCP protocol. A `typescript-client` bump is required for any new MCP field (`src/types/mcp-server.ts:54` notes missing `enabled` in client 1.36.1).
- **Hand-authored JSON Schema vs codegen:** Client tools are literal objects (`src/api/canvas-ui-client-tool.ts:60`), simple and reviewable, but lack compile-time parity with TypeScript types (no `zod` → schema derivation). `tool_module_qualnames` stripping of canvas tools (`src/api/agent-server-adapter.ts:1192`) is manual.
- **URL advertisement vs discovery:** `<RUNTIME_SERVICES>` gives agents `openapi_url`/`docs_url` without a fetch, but agents must still `curl` the OpenAPI JSON themselves; no typed SDK is generated for `automation` routes in the frontend.
- **Cloud short-circuit health:** `McpService.testServer` returns synthetic success for cloud backends (`src/api/mcp-service/mcp-service.api.ts:193`) to unblock saves, deferring real validation to conversation runtime. This hides misconfiguration until run time.

## Failure Modes / Edge Cases

- **Stale health write:** Without the `checkId` guard, a slow `testServer` probe could overwrite a newer probe's result after the server config was edited; the guard drops it (`src/api/mcp-health/mcp-health-store.ts:56`). Health is also in-memory only (no persistence) — reload resets to `unchecked` (`src/api/mcp-health/mcp-health-store.ts:9` comment).
- **Cloud MCP probe unreachable:** Frontend throws `NoBackendAvailableError` if no local backend is registered when OAuth is attempted (`src/api/mcp-service/mcp-service.api.ts:140`); cloud test short-circuit masks real connectivity errors until conversation start.
- **Redacted credential rename risk:** Renaming a server whose stored `env`/`auth`/`headers` contain `**********` throws `MCP_RENAME_CREDENTIAL_ERROR` (`src/utils/mcp-config.ts:403`) to prevent silent secret loss; callers must clear/re-enter credentials first.
- **Header credential patch limitation:** Removing an individual header from `strategy: header` is explicitly unsupported and throws `MCP_HEADER_REMOVAL_ERROR` (`src/utils/mcp-config.ts:271`), requiring full credential replacement.
- **Client-tool schema conflict:** Editing `parameters` for `canvas_ui_control` or `launch_child_conversation` without restarting the agent-server yields `ClientToolSchemaConflictError` on next conversation (`src/api/agent-server-adapter.ts:1111`). No frontend warning enforces this.
- **Jira-type OpenAPI entries:** Entries whose only connection option is `provider!=mcp` or `transport` missing are correctly excluded by `isMcpInstallableEntry` (`src/utils/mcp-marketplace-utils.ts:122`), but the UI gives no guidance to use the HTTP integration instead.
- **Auth sniff misclassification:** `AUTH_FAILURE_TEXT` regex (`\b(401|403)\b|unauthorized|...`) can misclassify a `connection` failure containing those substrings as `credentials` (`src/api/mcp-health/probe-mcp-server-health.ts:52`), and conversely miss non-English auth errors.
- **Secret leakage via URL:** MCP `url`/`headers`/`env` values are scrubbed via `redactMcpSecrets` in `redactMcpTestResponse` and `failedHealth` (`src/api/mcp-service/mcp-service.api.ts:68`, `src/api/mcp-health/probe-mcp-server-health.ts:30`), but only against the `server` and `substituted` sources — a leak from a nested OAuth state blob not in those sources would pass through.

## Future Considerations

- **Add OTLP exporter if cross-repo tracing is needed:** Mirror `software-agent-sdk` OTel setup in the frontend (e.g., `@opentelemetry/api` + OTLP HTTP exporter) and route through existing ingress (`/api/automation` already has auth). Without it, canvas-side spans (health probes, MCP test latency) remain invisible.
- **Generate ClientToolSpec from TypeScript types:** Replace literal `parameters` objects with a `zod`→`jsonSchema` generator so `ClientToolSpec` stays in sync with action types (`ClientAction_canvas_ui_control` per `src/constants/canvas-ui.ts:9`). Guard with `src/api/no-direct-agent-server-calls.test.ts`-style test that schemas validate.
- **OpenAPI import for automations:** Evaluate an OpenAPI→`ClientToolSpec` importer for the automation backend's `openapi.json` so curl-based `RUNTIME_SERVICES` discovery becomes typed tool calls; reuse `MCPClient` pattern (`src/api/mcp-service/mcp-service.api.ts:143`) for an `AutomationClient`.
- **MCP resources/prompts UI:** Extend `ExtendedMCPTestResponse` beyond `tools` to surface resources/prompts if the SDK adds them; current type is `tools: string[]` only (`src/types/mcp-server.ts:49`).
- **Unify two ClientToolSpec definitions:** `canvas-ui-client-tool.ts` and `launch-child-conversation-client-tool.ts` duplicate the `ClientToolSpec` interface; extract to `src/types/client-tool.ts` and add JSON Schema validation (Ajv) in tests.

## Questions / Gaps

- **No OTel evidence found:** Searched `src/` for `opentelemetry|otel|OTLP|exporter|trace.*span` — only `traceback` strings and `i18n` (`src/utils/automation-debug-prompt.ts:8`). If OTel exists in another repo (`software-agent-sdk`), this study cannot cite it per isolation rule; mark as absent for frontend.
- **No OpenAPI importer found:** Searched `src/` for `openapi|swagger|OpenAPI` + `importer|generator|fetch.*openapi` — only `openapi_url` plumbing (`scripts/runtime-services-info.mjs:139`, `src/api/agent-server-adapter.ts:265`). No code parses the fetched JSON into tools.
- **No JSON Schema generator found:** Searched `src/` for `zod|jsonschema|JsonSchema|toJsonSchema` — zero hits. `package.json:22` confirms no `zod`/`ajv`/`json-schema` dependency.
- **ACP via MCP?** `src/api/agent-server-adapter.ts:830` notes `mcp_config` is forwarded for ACP subprocesses, but no documentation confirms whether ACP servers also use MCP `resources`/`prompts`. Requires SDK inspection out of scope.
- **Automation OpenAPI coverage incomplete:** `vite.config.ts:431` proxies `/openapi.json` only for Vite dev; `scripts/static-server.mjs:7` and `docker/entrypoint.sh:400` also proxy it, but no test asserts the automation `openapi_url` is reachable from the agent sandbox (network alias `agentHostAlias`).

---

Generated by `dimensions/19.01-protocol-compatibility.md` against `openhands`.
