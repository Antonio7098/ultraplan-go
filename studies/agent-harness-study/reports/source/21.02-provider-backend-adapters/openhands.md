# Source Analysis: openhands

## Dimension 21.02 — Provider and Backend Adapters

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (React Router 7), Vite, `@openhands/typescript-client`, MSW for mocks; this repo is the OpenHands "Agent Canvas" **frontend** (backend endpoints live in the sibling `software-agent-sdk` repo and are out of scope here) |
| Analyzed | 2026-08-25 |

## Summary

The swappable-backend story in this source is organized around a **frontend backend registry** rather than in-process provider interfaces. A `Backend` record (`kind: "local" | "cloud"`, `authMode: "api-key" | "cookie"`) is defined in `src/api/backend-registry/types.ts:1-13`; any number of backends can be registered, edited, activated, and removed at runtime through `ActiveBackendProvider` (`src/contexts/active-backend-context.tsx:71-223`). Every service-layer call then branches on the active backend's `kind`: local agent-server traffic goes through typed clients from `@openhands/typescript-client` assembled by an options factory (`src/api/agent-server-client-options.ts:52-69`), while cloud traffic is funneled through a single proxy adapter `callCloudProxy()` (`src/api/cloud/proxy.ts:18-39`) that POSTs an envelope to the local agent-server's `/api/cloud-proxy` to bypass browser CORS. The transport rule is enforced in CI by an architectural guard test that scans all of `src/` for raw axios/fetch/HttpClient usage (`src/api/no-direct-agent-server-calls.test.ts:32-79`).

Multiple implementations exist per logical service: conversation CRUD (`src/api/conversation-service/agent-server-conversation-service.api.ts:357-447`), settings persistence (`src/api/settings-service/settings-service.api.ts:444-601`), model/provider discovery (`src/api/config-service/config-service.api.ts:65-185`), bash-event reads (`src/api/bash-service/bash-service.api.ts:77-110`), events (`src/api/event-service/event-service.api.ts:48-140`), secrets, MCP config, automations, skills, profiles, and sandbox lookup. At the agent level, a second axis of swappability exists: agent *kinds* (OpenHands litellm agent vs ACP CLI subprocesses such as claude-code/codex/gemini-cli) are dispatched by strategy functions in the payload builder (`src/api/agent-server-adapter.ts:949-956`), with LLM auth strategies (API key vs ChatGPT subscription) handled inside `buildConfiguredOpenHandsAgentSettings` (`src/api/agent-server-adapter.ts:883-947`). A third full implementation of the REST surface — MSW mock handlers toggled by `VITE_MOCK_API` (`src/mocks/handlers.ts:27-40`, `src/mocks/should-start-mock-worker.ts:1-9`) — backs dev/test/e2e modes.

The model is explicit and heavily tested (unit tests for registry/storage/fallbacks, cloud proxy tests, plus E2E suites for auth modes, multi-backend cross-connect, and partial-stack failure), but it is implemented as repeated `if (active.kind === "cloud")` branches across ~15+ services instead of one polymorphic interface, so adding a third backend kind would require touching many files.

## Rating

**Score: 7 / 10** — "Clear model with tests, explicit interfaces, and operational safeguards" (top of the 7–8 band).

Rationale:
- Clear abstraction boundary: `Backend` type + registry + two named transports (typed local client vs `callCloudProxy`), enforced mechanically by a CI guard test (`src/api/no-direct-agent-server-calls.test.ts:32-79`).
- Runtime swapping is real and observable: users add/edit/remove/activate backends live (`src/contexts/active-backend-context.tsx:89-204`), queries re-key on backend identity so data refetches per backend (`src/contexts/active-backend-context.tsx:112-119`), and health polling with a consecutive-failure disable cap provides operational safeguards (`src/hooks/query/use-backends-health.ts:34-35,90-120`; `src/api/backend-registry/health-store.ts:46-63`).
- Strong test coverage at three levels: unit (`__tests__/api/backend-registry/*.test.ts`, `__tests__/api/cloud/proxy.test.ts`), hook-level (`__tests__/hooks/query/use-backends-health.test.tsx`), and E2E including failure modes (`tests/e2e/mock-llm/backends/mock-llm-partial-stack.spec.ts:174-390`, `tests/e2e/mock-llm/backends/mock-llm-auth-modes.spec.ts:46-330`).
- It does not reach 9–10 because: (1) there is no single `BackendAdapter` interface — the cloud/local split is duplicated as if-chains in every service (e.g. nine separate branch sites inside `src/api/settings-service/settings-service.api.ts:451-646` alone), making a third backend kind expensive; (2) capability asymmetries are caller-gated ad hoc rather than negotiated (git repository search silently returns empty pages locally, `src/api/git-service/git-service.api.ts:42-44`; `ProviderConnectionsService` documents "Cloud has no equivalent yet" and relies on callers to gate, `src/api/provider-connections-service/provider-connections-service.api.ts:8-9`); (3) backend API keys are persisted in plaintext localStorage (`src/api/backend-registry/storage.ts:95-102`).

## Evidence Collected

Every entry cites file paths relative to the source root with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Backend abstraction | `Backend {id, name, host, apiKey, kind: "local"\|"cloud", authMode?: "api-key"\|"cookie", connectionRevision?}`; `BackendSelection {backendId, orgId?}` | `src/api/backend-registry/types.ts:1-18` |
| Registry store w/ fallback policy | `pickFallbackBackend()` prefers healthy local → any local → first registered; `NO_BACKEND` sentinel must be checked before reading fields | `src/api/backend-registry/active-store.ts:29-62` |
| Registry persistence | `openhands-backends` localStorage key, schema validation `isValidBackend()`, first-run seeding via `makeDefaultLocalBackend()` / locked-cloud seeding | `src/api/backend-registry/storage.ts:13-149` |
| Default backend factories | `makeDefaultLocalBackend()` from env host+key; `makeLockedCloudBackend()` cookie-auth cloud when `VITE_LOCK_TO_CLOUD` set | `src/api/backend-registry/default-backend.ts:16-70` |
| Auth adapter per kind | `buildAuthHeaders()`: cloud+bearer → `Authorization: Bearer`, local → `X-Session-API-Key`, cloud+cookie → `{}` | `src/api/backend-registry/auth.ts:9-18` |
| Client-options factory | `getAgentServerClientOptions()` resolves host/apiKey/workingDir from effective local backend or overrides; throws `NoBackendAvailableError` | `src/api/agent-server-client-options.ts:22-69` |
| Cloud transport adapter | `callCloudProxy(req)` wraps `CloudClient.request` with `authMode: bearer\|session-api-key\|none` and optional runtime `hostOverride` | `src/api/cloud/proxy.ts:5-39` |
| Cloud client factory | `createCloudClient()` builds `CloudClient` with org header injection + local-agent-server proxy; `createCloudClientForRuntime()` requires the proxy | `src/api/cloud/client.ts:33-63` |
| Conversation dual impl | `sendMessage`: cloud resolves sandbox URL then proxies; local uses `ConversationClient.sendEvent`. `createConversation`: cloud flat `AppConversationStartRequest` vs local encrypted-settings payload | `src/api/conversation-service/agent-server-conversation-service.api.ts:357-447` |
| Settings dual impl | `getSettings`/`getSettingsSchema`/MCP CRUD each branch cloud (`fetchCloudSettings`, `saveCloudSettings`) vs local `SettingsClient` | `src/api/settings-service/settings-service.api.ts:444-601` |
| Model/provider discovery dual impl | `ConfigService.searchModels/searchProviders`: cloud `/api/v1/config/{models,providers}/search` vs local reconstruction from `LLMMetadataClient` | `src/api/config-service/config-service.api.ts:65-185` |
| Bash events dual impl | Cloud tunnels with runtime `hostOverride` + session key; local uses SDK `BashClient` against active backend host | `src/api/bash-service/bash-service.api.ts:26-110` |
| Events dual impl | `EventService.respondToConfirmation/getEventCount/searchEvents` branch on `active.kind === "cloud"` | `src/api/event-service/event-service.api.ts:40-140` |
| Git providers (cloud-only) | All repository/branch/installation searches return empty pages unless cloud is active | `src/api/git-service/git-service.api.ts:42-142` |
| Provider connections (local-only) | Docblock: endpoints exist only on the agent-server; "Cloud has no equivalent yet, so callers must gate usage" | `src/api/provider-connections-service/provider-connections-service.api.ts:1-14` |
| Sandbox lookup (cloud-only) | `batchGetCloudSandboxes()` mirrors `SandboxService.batchGetSandboxes`, throws unless active backend is cloud | `src/api/cloud/sandbox-service.api.ts:6-39` |
| Mock backend (MSW) | Handler groups for files/secrets/profiles/git/settings/conversations/auth/analytics/automations/MCP/workspaces; enabled iff `VITE_MOCK_API === "true"` | `src/mocks/handlers.ts:27-40`; `src/mocks/should-start-mock-worker.ts:1-9` |
| Agent-kind strategy dispatch | `buildConfiguredAgentSettings` picks ACP vs OpenHands settings builder via `isAcpAgent()` | `src/api/agent-server-adapter.ts:790-793,949-956` |
| ACP provider registry | Built-in ACP providers (claude-code, codex, gemini-cli) sourced from `@openhands/typescript-client` mirror of Python `acp_providers`; default launch commands + models | `src/constants/acp-providers.ts:66-100+` |
| LLM auth strategies | Subscription vendor auth deletes api_key/base_url and sets `auth_type`/`subscription_vendor`; pre-flight gate `assertSubscriptionAuthReady()` | `src/api/agent-server-adapter.ts:913-921,1233-1252` |
| Runtime selection API | `setActive/addBackend/updateBackend/removeBackend` mutate registry; React Query keys include backend id/org so switches refetch once | `src/contexts/active-backend-context.tsx:89-121` |
| Health probing safeguard | Per-backend probe differs by kind (local: settings+server_info; cloud: `/api/keys/current` or orgs); failures recorded, disabled after cap | `src/hooks/query/use-backends-health.ts:74-120`; `src/api/backend-registry/health-store.ts:46-63` |
| URL-pinned selection | `?backend=<id>&org=<id>` query params pin new tabs to owning backend; unknown ids ignored | `src/api/backend-registry/url-selection.ts:17-77` |
| Credential-change invalidation | `connectionRevision` bumped on host/key edit; health reset + bootstrap re-probe | `src/api/backend-registry/types.ts:11-12`; `src/contexts/active-backend-context.tsx:141-182` |
| Config externalization (env) | `VITE_BACKEND_BASE_URL`, `VITE_SESSION_API_KEY` (+ window-injected fallback), `VITE_LOCK_TO_CLOUD`, `VITE_WORKING_DIR`, `isAuthRequired` window flag | `src/api/agent-server-config.ts:98-132,141-155,196-201,214-226` |
| Version-floor compatibility gate | `MINIMUM_COMPATIBLE_AGENT_SERVER_VERSION` read from `config/defaults.json` (`compatibility.minimumAgentServer = "1.28.0"`); unsupported servers raise typed error | `src/api/agent-server-compatibility.ts:17-19`; `config/defaults.json:9` |
| Telemetry sink configurability | PostHog key/host overridable via `VITE_POSTHOG_API_KEY` / `VITE_POSTHOG_HOST` and runtime `configureTelemetry()`; hard disable via `VITE_DO_NOT_TRACK=1` | `src/services/telemetry.ts:12-23,58-66,205` |
| Architectural CI guard | Test scans every src file for direct axios/fetch-to-/api/HttpClient usage; only 3 whitelisted infrastructure files allowed | `src/api/no-direct-agent-server-calls.test.ts:5-79` |

## Answers to Dimension Questions

**1. Are backends swappable?**
Yes — along several axes. Remote compute backends are user-swappable at runtime: the registry stores N backends (`src/api/backend-registry/storage.ts:95-149`) and `setActive()` switches between them live (`src/contexts/active-backend-context.tsx:89-121`). LLM providers are server-side-configurable: the frontend manages provider connections (shared `api_key` + `base_url` resolved into runnable LLMs by the agent-server, `src/api/provider-connections-service/provider-connections-service.api.ts:1-14`) and model catalogs are discovered from `/api/llm/providers|models` locally or cloud search APIs (`src/api/config-service/config-service.api.ts:65-185`). Agent runtimes are swappable between the built-in litellm agent and external ACP CLIs (`src/api/agent-server-adapter.ts:949-956`). However, the *transport* implementations themselves (local typed client vs cloud proxy) are not pluggable extension points — a new transport requires code changes, since the dispatch is hardcoded if-chains, not a registry of adapters.

**2. Which backends have multiple implementations?**
Remote conversation/settings/events/bash/secrets/MCP/skills/profiles/automations have both local-agent-server and cloud implementations behind shared static service classes (see evidence table). Three notable asymmetric cases: git repository search is cloud-only (local returns empty pages, `src/api/git-service/git-service.api.ts:42-44`); cloud sandboxes exist only on cloud (`src/api/cloud/sandbox-service.api.ts:6-12`); provider connections exist only locally (`src/api/provider-connections-service/provider-connections-service.api.ts:8-9`). The entire REST surface additionally has a mock implementation (MSW handlers, `src/mocks/handlers.ts:27-40`) and a scripted mock LLM used by E2E (`tests/e2e/mock-llm/scripts/mock-llm-server.py` referenced by `playwright.mock-llm.config.ts`).

**3. Can backends be swapped at runtime?**
Yes. Switching is synchronous store mutation + subscriber notification (`src/api/backend-registry/active-store.ts:158-185`); React subscribes via `useSyncExternalStore` (`src/contexts/active-backend-context.tsx:76-80`). Data correctness on switch is handled by embedding backend id/org in React Query keys so each backend gets its own cache entries ("No blanket invalidateQueries… fetches automatically — once", `src/contexts/active-backend-context.tsx:112-119`); credential edits bump `connectionRevision` to invalidate keyed data (`src/api/backend-registry/types.ts:11-12`). Health state survives reloads via a persisted health map with a disable cap (`src/api/backend-registry/health-store.ts:40-63`), and links carry `?backend=&org=` so new tabs land on the owning backend (`src/api/backend-registry/url-selection.ts:3-50`). E2E tests prove real switching between two backends (`tests/e2e/mock-llm/backends/mock-llm-cross-connect.spec.ts:452-653`).

**4. Are adapter implementations tested?**
Extensively. Unit: registry fallback matrix incl. unhealthy-local and removed-selection cases (`__tests__/api/backend-registry/active-store.test.ts:72-260`), storage validation/seeding/key-sync (`__tests__/api/backend-registry/storage.test.ts:34-361`), cloud proxy envelope (`__tests__/api/cloud/proxy.test.ts`), per-cloud-endpoint suites under `__tests__/api/cloud/`, conversation-service dual-path tests (`__tests__/api/agent-server-conversation-service.test.ts`), adapter payload tests (`__tests__/api/agent-server-adapter.test.ts:86-514`), and health-hook tests (`__tests__/hooks/query/use-backends-health.test.tsx`). Integration/E2E: auth modes incl. stale-key rotation and public-mode 401 gating (`tests/e2e/mock-llm/backends/mock-llm-auth-modes.spec.ts:116-330`), frontend→separate-backend cross-connect and two-backend switching (`tests/e2e/mock-llm/backends/mock-llm-cross-connect.spec.ts:329-653`), partial-stack startup failures (`tests/e2e/mock-llm/backends/mock-llm-partial-stack.spec.ts:174-390`). The architecture itself is tested by the no-direct-calls guard test (`src/api/no-direct-agent-server-calls.test.ts:32-79`).

**Dimension headline question — "Can you switch from Postgres to SQLite with a config change?"**
Not literally applicable: this frontend owns no database. Its analogue holds: you can repoint the whole app from a local agent-server to OpenHands Cloud (or another agent-server host) with a UI form submission or env var (`VITE_BACKEND_BASE_URL`, `src/api/agent-server-config.ts:98-100,181-190`), with no rebuild — except that build-time-baked flags like `VITE_MOCK_API` and `VITE_ENABLE_BROWSER_TOOLS` do require a rebuild (`src/mocks/should-start-mock-worker.ts:2`; `src/api/agent-server-adapter.ts:117-119`).

## Architectural Decisions

1. **Registry-over-inheritance**: backends are data records (`Backend`) in a store, not class instances implementing a transport interface; behavior is selected by branching on `kind` (`src/api/backend-registry/active-store.ts:64-93`). This keeps serialization/persistence trivial but spreads dispatch logic across services.
2. **Single choke points per transport**: all local calls must go through client-option assembly (`src/api/agent-server-client-options.ts:52-69`) and all cloud calls through `callCloudProxy` (`src/api/cloud/proxy.ts:18-39`); compliance is enforced by a source-scanning test rather than convention (`src/api/no-direct-agent-server-calls.test.ts:33-78`).
3. **CORS-driven cloud proxying**: cloud and runtime-sandbox hosts don't allow CORS from localhost, so requests are relayed server-side through the local agent-server's `/api/cloud-proxy` with selectable `authMode` (`src/api/cloud/proxy.ts:13-16`, docblock in `AGENTS.md` Rule 2 mirrored in code).
4. **Capability negotiation via server metadata, partially**: tool availability is gated on `/server_info.usable_tools` (`src/api/agent-server-compatibility.ts:27-34` consumed in `src/api/agent-server-adapter.ts:631-644`) and version floor from config (`config/defaults.json:9`), but feature parity between local/cloud is otherwise encoded implicitly in each service's branch.
5. **Spec-tagged behaviors**: registry behaviors carry `@spec BM-001/BM-003` tags tying auto-switch-on-add and fallback-on-remove to tracked specs (`src/contexts/active-backend-context.tsx:123`; `src/api/backend-registry/active-store.ts:82`).

## Notable Patterns

- **Anti-corruption layer**: `toAppConversation`/`toConversationPage` translate the raw SDK wire shape (including ACP discriminator quirks and sentinel values) into the GUI's `AppConversation` (`src/api/agent-server-adapter.ts:317-412`).
- **Strategy dispatch for agent kinds**: `buildConfiguredAcpAgentSettings` vs `buildConfiguredOpenHandsAgentSettings`, chosen by `isAcpAgent()`, with per-provider defaults resolved from a registry (`src/api/agent-server-adapter.ts:817-956`; `src/constants/acp-providers.ts:66+`).
- **Externalized secrets without mirroring**: conversation-start payloads reference server-side secrets via `LookupSecret` URLs with auth headers instead of shipping values (`src/api/agent-server-adapter.ts:995-1000,1208-1228`).
- **Environment/window-global config layering**: build-time env vars fall back to serve-time injected window globals (`window.__AGENT_CANVAS_SESSION_API_KEY__`, `__AGENT_CANVAS_LOCK_TO_CLOUD__`) so the same prebuilt bundle works across launch modes (`src/api/agent-server-config.ts:119-132,141-155`).
- **Self-healing seed + key sync**: stored default-local backends whose host matches (loopback-equivalent) the launcher's current default get their API key resynced at module init (`src/api/backend-registry/storage.ts:56-93`).

## Tradeoffs

- **If-chain duplication vs polymorphism**: ~15 services each own their cloud/local branch (e.g., `src/api/settings-service/settings-service.api.ts:451,515,524,538,560,577,591,646`; `src/api/automation-service/automation-service.api.ts:252-767` has ~20 branch sites). Consistency relies on discipline and review, though the centralization of transports (two choke points) limits blast radius.
- **Cloud features degrade silently in some paths**: local git-provider search returns empty pages rather than a typed "unsupported" signal callers could surface distinctly (`src/api/git-service/git-service.api.ts:42-44`).
- **Plaintext credentials in localStorage**: backend `apiKey`s ride in `openhands-backends` with quota errors swallowed (`src/api/backend-registry/storage.ts:95-102`); contrast with git tokens which are deliberately kept server-side.
- **Mock fidelity is a maintained surface**: MSW handlers must track real agent-server routes (documented route list in `AGENTS.md`, implemented in `src/mocks/*-handlers.ts`), an accepted maintenance cost for fast dev/test loops.

## Failure Modes / Edge Cases

- **Unreachable/removed backend**: selection pointing at a removed backend falls back to healthy-local-first policy; `NO_BACKEND` sentinel prevents null-field reads (`src/api/backend-registry/active-store.ts:37-62,77-86`).
- **Flapping backends**: after `MAX_CONSECUTIVE_FAILURES` probes fail, the backend is marked `disabled` and polling stops until config changes reset health (`src/api/backend-registry/health-store.ts:46-85`).
- **Stale credentials across restarts**: launcher key rotation is reconciled on load for loopback-equivalent hosts (`src/api/backend-registry/storage.ts:70-93`); public-mode 401 triggers an inline key-entry recovery screen (auth flow described in `tests/e2e/mock-llm/backends/mock-llm-auth-modes.spec.ts:199-330`).
- **Cross-tab confusion**: tab-scoped sessionStorage selection with URL pinning prevents a new tab from resolving a conversation against the wrong backend (`src/api/backend-registry/url-selection.ts:3-16`; `src/api/backend-registry/storage.ts:205-219`).
- **Proxy dependency**: runtime-sandbox calls require the local proxy; without it `createCloudClientForRuntime` throws `NoBackendAvailableError` (`src/api/cloud/client.ts:57-63`).
- **Version skew**: older/newer agent-servers are rejected above a configured floor with actionable error codes (`src/api/agent-server-compatibility.ts:17-19` + typed error classes in the same file).

## Future Considerations

- Introduce a single `BackendTransport`/capability-descriptor interface so services consume declared capabilities instead of repeating `kind === "cloud"` checks; today adding e.g. an "enterprise self-hosted" kind means auditing every service file (branch inventory visible via grep across `src/api/**`).
- Promote capability asymmetries (cloud-only git search, local-only provider connections) into the `/server_info` metadata contract alongside `usable_tools`.
- Move backend API keys out of localStorage into the agent-server secret store, matching the pattern already used for git-provider tokens.
- Replace silent empty-page degradation (`src/api/git-service/git-service.api.ts:42-43`) with explicit unsupported-feature errors surfaced in UI.

## Questions / Gaps

- No vector DB, object store, or queue abstractions exist in this source (it is a frontend); whether such backends are swappable cannot be answered from this repo — they belong to `software-agent-sdk`, which was outside the permitted inspection boundary. Searched `src/` for storage/vector/queue adapter patterns; only browser `localStorage`/`sessionStorage` wrappers were found (`src/api/backend-registry/storage.ts:174-203`).
- Whether cloud-side sandbox creation supports alternative sandbox technologies (Docker vs remote microVM) is not decidable here; the frontend only reads `sandbox_status`/`exposed_urls` (`src/api/cloud/sandbox-service.api.ts:14-24`).
- The telemetry sink is fixed to PostHog; only endpoint/key/host are configurable (`src/services/telemetry.ts:12-23,58-66`). No sink-interface abstraction was found.

---

Generated by `dimension 21.02-provider-and-backend-adapters` against `openhands`.
