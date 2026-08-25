# Source Analysis: openhands

## Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Vite 8, TanStack Query, Zustand; Node 22 CLI launcher; Electron desktop shell |
| Analyzed | 2026-08-24 |

> Citation convention: all `path:line` citations below are relative to the source root
> `studies/agent-harness-study/sources/openhands/`. Prefix that path for full workspace-relative resolution.
> Note: this source is the OpenHands **frontend/UI harness** (Agent Canvas). The agent loop, tools,
> and policy execution live in a separate Python repo (`software-agent-sdk`), so "embedding" here means
> embedding the agent UI + its client stack into a host product.

## Summary

OpenHands Agent Canvas is explicitly designed as an embeddable UI harness with five distribution modes: (1) an npm React component library (`@openhands/agent-canvas`, package.json:206-248) with per-domain subpath exports (conversation, terminal, browser, files, settings, sidebar, i18n); (2) a standalone React Router SPA; (3) a CLI binary (`bin/agent-canvas.mjs:1-173`, declared at package.json:16) that orchestrates the full local stack — Python agent-server via `uvx`, automation backend, ingress proxy, static frontend; (4) a Docker all-in-one image behind one unified ingress port; and (5) an Electron desktop shell reusing the same launcher (`electron/main.mjs`).

The embedding contract is centered on two public components: `AgentServerUIProviders` (`src/components/providers/agent-server-ui-providers.tsx:48-123`) which accepts host-supplied `queryClient`, `i18n` instance, analytics config (or hard-disable via `false`), and theme/style overrides, and `AgentServerUIRoot` (`src/components/providers/agent-server-ui-root.tsx:21-56`) which mounts the `[data-agent-server-ui]` CSS-isolation scope. All bundled CSS is namespaced under that attribute at build time via a PostCSS selector prefixer (`vite.config.ts:195-214`), so the UI does not leak styles into a host page. Configuration injection is layered: build-time Vite env vars, runtime window-globals injected by launchers (`window.__AGENT_CANVAS_SESSION_API_KEY__`, read in `src/api/agent-server-config.ts:119-132`), and a localStorage backend registry seeded from those values (`src/api/backend-registry/storage.ts:104-149`).

Telemetry is deliberately host-friendly: PostHog is initialized under a named instance `"agent-canvas"` to avoid colliding with a host's default PostHog singleton (`src/services/telemetry.ts:343-361`), is consent-gated, honors Do Not Track (`telemetry.ts:276-305`), and can be hard-disabled by the embedding host with `configureTelemetry(false)` or `analytics={false}` (`telemetry.ts:205-231`). Lifecycle of spawned services in CLI/desktop modes is handled with process-group signaling and shutdown-hook registries (`scripts/dev-process-utils.mjs:85-150`).

The main ergonomic gaps: browser storage backends are not injectable (localStorage keys are hardcoded, e.g. `src/api/backend-registry/storage.ts:13-14`), error surfacing to hosts is toast-based rather than a typed callback channel, and the flagship exported component `ConversationView` still requires the host to run inside a React Router context with matching route shapes because it imports router hooks directly (`src/routes/conversation.tsx:2`).

## Rating

**7 / 10** — Clear model with tests and explicit interfaces. The provider/root API, subpath exports, CSS isolation, telemetry isolation/disable, version-compatibility floor, and process-tree shutdown are deliberate, documented, and covered by dedicated unit/E2E tests (`__tests__/agent-server-ui-providers.test.tsx:98-279`, `__tests__/library-entrypoints.test.ts:10-49`, `__tests__/package-library.test.ts:29-158`). It falls at the bottom of the 7–8 band because storage and identity are not injectable, error propagation to hosts rides on global toasts instead of callbacks, the exported conversation surface couples hosts to React Router internals, and embedding documentation beyond styling is thin (`docs/DEVELOPMENT.md:151-177` covers providers/CSS only).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library entry points | Root export barrel exposing components, providers, telemetry, i18n, style-scope constants | `src/index.ts:1`; `src/lib/index.ts:1-57` |
| Package exports map | Subpath exports `.`, `/browser`, `/conversation`, `/files`, `/settings`, `/sidebar`, `/terminal`, `/i18n` (ESM+CJS+d.ts); peer deps react/react-dom/react-router | `package.json:206-248`, `package.json:249-253` |
| Library build pipeline | `BUILD_LIB=true` switches Vite to library mode; externalizes react/react-dom/jsx-runtime/react-router; preserveModules ESM+CJS output | `vite.config.ts:108`, `vite.config.ts:215-246` |
| Provider injection API | `AgentServerUIProvidersProps`: host-supplied `queryClient`, `analytics` (PostHog config \| `false` \| `null`), `i18n`, `withStyleRoot`, theme/style props | `src/components/providers/agent-server-ui-providers.tsx:37-46` |
| Provider swap/restore | On mount sets active queryClient/i18n singletons; on unmount restores previous instances so hosts keep their own context intact | `src/components/providers/agent-server-ui-providers.tsx:60-88` |
| Scoped style root | `AgentServerUIRoot` renders `data-agent-server-ui` wrapper, injects ~80 `--oh-*` CSS variables + `styleOverrides`, applies `data-theme` | `src/components/providers/agent-server-ui-root.tsx:21-56`; `src/styles/agent-server-ui-style-scope.ts:17-95` |
| CSS isolation build step | postcss-prefix-selector scopes every bundled selector under `[data-agent-server-ui]`; `transformAgentServerUISelector` remaps `:root`/`html`/`body` and supports `:host` | `vite.config.ts:195-214`; `src/styles/agent-server-ui-style-scope.ts:97-117` |
| Runtime config injection | `getBakedSessionApiKey()` reads `VITE_SESSION_API_KEY` then launcher-injected `window.__AGENT_CANVAS_SESSION_API_KEY__`; `isAuthRequired()` mirrors `window.__AGENT_CANVAS_AUTH_REQUIRED__`; locked-cloud via `__AGENT_CANVAS_LOCK_TO_CLOUD__` | `src/api/agent-server-config.ts:119-132`, `:214-221`, `:141-155` |
| Backend registry seeding | First read of `openhands-backends` seeds a default local backend from env/injected host+key; key rotation re-syncs the seeded entry at module init | `src/api/backend-registry/storage.ts:118-148`, `:70-93`; `src/api/backend-registry/default-backend.ts:53-70` |
| Multi-backend host model | Registry of `Backend {id,name,host,apiKey,kind:"local"\|"cloud"}` with active selection, URL-pinned selection, health store, subscribe API | `src/api/backend-registry/types.ts:4-22`; `src/api/backend-registry/active-store.ts:128-185` |
| Client option assembly | `getAgentServerClientOptions(overrides)` resolves host/key/workingDir from active backend; throws typed `NoBackendAvailableError` when unconfigured | `src/api/agent-server-client-options.ts:52-69`, `:22-36` |
| Telemetry host control | `configureTelemetry(false)` disables all capture incl. install event; runtime apiKey/apiHost/uiHost override for precompiled consumers; DNT honored | `src/services/telemetry.ts:205-231`, `:73-82`, `:276-305` |
| Telemetry singleton isolation | Named PostHog instance `"agent-canvas"` with separate persistence names isolates Canvas consent/identity from host default singleton | `src/services/telemetry.ts:53`, `:343-362` |
| Consent store | `setTelemetryConsent`, `subscribeTelemetryConsent` (storage-event based external store), pending-cloud-sync semantics | `src/services/telemetry.ts:537`, `:447-468` |
| Telemetry lifecycle provider | `TelemetryProvider` configures bootstrap/runtime options, eagerly initializes service, owns install/session events; renders nothing extra when disabled | `src/components/providers/telemetry-provider.tsx:71-106` |
| i18n injection | Host may pass its own i18next instance; `createAgentServerI18n`/`getDefaultI18n`/`setI18n`/`waitForI18n`; translations loadable via `@openhands/agent-canvas/i18n` subpath | `src/i18n/index.ts:11`, `:69-92`; `package.json:242-246` |
| Global error surfacing | Shared QueryClient emits error toasts from QueryCache/MutationCache onError with 3s dedupe set and per-query `disableToast` meta opt-out | `src/query-client-config.ts:29-81` |
| Toast channel | `displayErrorToast` maps CORS/network/timeout errors to localized copy; styled with `--oh-*` tokens so toasts match embedded theme | `src/utils/custom-toast-handlers.tsx:96-108`, `:15-29` |
| Streaming/event transport | `useWebSocket` with reconnect backoff (1s→30s cap), handshake watchdog aborting stuck CONNECTING sockets, `onMessage` callback contract (no per-frame state writes) | `src/hooks/use-websocket.ts:12-20`, `:64`, `:76-81`, `:110-120` |
| Cloud sandbox lifecycle gating | WebSocket URL suppressed while cloud sandbox PAUSED; resume via POST resume + fast polling before socket connect | `src/contexts/websocket-provider-wrapper.tsx:24-45` |
| Approvals/confirmation | `confirmation_mode` setting flag; agent states `AWAITING_USER_CONFIRMATION` / `WAITING_FOR_CONFIRMATION` surfaced through the event stream | `src/types/settings.ts:128`; `src/types/agent-state.tsx:12`; `src/types/agent-server/core/base/common.ts:71` |
| Secrets delegation | Conversation-start payload attaches saved secrets as `LookupSecret` URLs pointing back at the server store with session-auth headers — credentials never live in the UI | `src/api/agent-server-adapter.ts:1203-1228` |
| Policy/tool ownership | Tools/policy executed by external agent-server; UI gates tool attachment on server-advertised `usable_tools` and forwards `runtime_services` as `system_message_suffix` | `src/api/agent-server-compatibility.ts:25-39`; `src/api/agent-server-adapter.ts:749-788` |
| Version compatibility floor | `assertAgentServerVersionIsSupported` enforces `compatibility.minimumAgentServer` from config/defaults.json with typed unsupported/unknown errors | `src/api/agent-server-compatibility.ts:15-23`, `:72-80` |
| CLI embedding mode | `bin/agent-canvas.mjs` parses `--public/--frontend-only/--backend-only/-p`, validates build presence, delegates to stack launcher `main()` | `bin/agent-canvas.mjs:54-56`, `:137-149`, `:161-167` |
| Process-tree shutdown | POSIX process-group kill (`kill(-pid)`), Windows `taskkill /t /f`; SIGINT/SIGTERM/SIGHUP handlers plus shutdown-hook registry | `scripts/dev-process-utils.mjs:85-105`, `:118-129`, `:131-150`; `scripts/dev-with-automation.mjs:1070-1097` |
| Desktop embedding readiness | Electron waits for real agent-server `/server_info` (200 or 401) before loading the window; `main()` returns `{config, agentServerReady}`; bundles real Node dist for child-process fidelity | `electron/main.mjs:240`, `:601-675`, `:170-177` |
| Launcher progress hook | `setServiceLogListener(cb)` exposes per-service stdout/stderr lines (best-effort, listener errors swallowed) used for first-run install progress UI | `scripts/dev-with-automation.mjs:617` |
| API access discipline | CI guard test forbids raw axios/fetch/HttpClient against agent-server outside an allowlist — all traffic must use `@openhands/typescript-client` | `src/api/no-direct-agent-server-calls.test.ts:32-60` |
| Embedding tests | Providers test asserts default/custom queryClient+i18n injection, restore-on-unmount, analytics disable/config pass-through, scoped style root behavior | `__tests__/agent-server-ui-providers.test.tsx:98-279` |
| Public-surface tests | Entrypoint test pins exported symbols (`ConversationView`, panels, providers, scope constants); package test pins exports map, exact pins, no git deps | `__tests__/library-entrypoints.test.ts:10-49`; `__tests__/package-library.test.ts:29-106` |
| Style-isolation regression | Browser-level CSS-isolation spec included in mock-LLM E2E suite | `tests/e2e/mock-llm/regressions/mock-llm-ui-regressions.spec.ts` |
| Standalone app uses same contract | App hydrates inside `AgentServerUIProviders` with `withStyleRoot={false}` (root layout renders its own scoped shell) | `src/entry.client.tsx:35-48` |
| Docs | Architecture doc states dual packaging goal; DEVELOPMENT.md documents embedding + customization strategy and env vars | `docs/architecture.md:12`, `:58-66`; `docs/DEVELOPMENT.md:151-194` |

## Answers to Dimension Questions

### 1. Can the harness run inside another application without owning the whole process?

Yes, for the UI layer — this is the primary design goal. Hosts render `AgentServerUIProviders`/domain components inside their own React tree with optional injected queryClient/i18n (`src/components/providers/agent-server-ui-providers.tsx:48-123`); the library build externalizes only react/react-dom/react-router (`vite.config.ts:224-225`) and ships preserve-modules ESM+CJS (`vite.config.ts:226-245`). CSS cannot leak into the host because every selector is prefixed under `[data-agent-server-ui]` at build time (`vite.config.ts:195-214`), with regression coverage (`__tests__/agent-server-ui-style-scope.test.ts:7-45`, browser-level E2E in `tests/e2e/mock-llm/regressions/mock-llm-ui-regressions.spec.ts`). However, the harness never runs the *agent itself* in-process: it always assumes an external agent-server reachable over HTTP/WebSocket, and most service calls resolve their target from the module-level backend registry rather than props (`src/api/agent-server-client-options.ts:52-58`). Two caveats limit "without owning the whole process": the exported `ConversationView` calls `useNavigate/useLocation/useMatch` directly (`src/routes/conversation.tsx:2`) and navigates to hardcoded `/conversations...` paths (`conversation.tsx:100`, `:118`), so hosts must run a compatible React Router tree; and several stores persist to hardcoded localStorage keys (`src/api/backend-registry/storage.ts:13-14`), which is browser-global even if not process-global.

### 2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?

Partially — telemetry yes, i18n/query plumbing yes, storage no, policy/tools/secrets delegated by design:

- **Telemetry**: fully host-controllable. Inject own PostHog key/host via `analytics` prop or `configureTelemetry()` (`src/services/telemetry.ts:73-82`, `:205-231`), or hard-disable with `false` (`telemetry-provider.tsx:79-99` wires `config=false` → no lifecycle, no init). Named instance avoids clobbering host PostHog (`telemetry.ts:343-361`).
- **Identity**: user auth identity is owned by the configured backends (session API key per local backend, bearer/cookie per cloud backend — `src/api/backend-registry/types.ts:4-13`, `src/api/backend-registry/auth.ts:9`). There is no pluggable identity-provider hook in the UI itself.
- **Storage**: not injectable. Backends, active selection, telemetry consent, and per-conversation UI state persist to fixed localStorage/sessionStorage keys written directly (`src/api/backend-registry/storage.ts:95-102`, `:221-243`; telemetry consent keys `src/services/telemetry.ts:45-52`). A host wanting server-side or encrypted storage would have to fork these modules.
- **Tools/policy**: intentionally out of scope for this repo — the agent-server advertises `usable_tools` and the UI only gates what it sends (`src/api/agent-server-compatibility.ts:34-39`); confirmation/approval policy is a backend-interpreted setting (`src/types/settings.ts:128`). This is clean separation, but a host cannot implement custom approval flows through this UI without modifying components.
- **Secrets**: handled well for its model — secrets are stored server-side and referenced at spawn time via authenticated `LookupSecret` pointers, never mirrored into the browser (`src/api/agent-server-adapter.ts:1203-1228`).

### 3. Are lifecycle, cancellation, shutdown, and error propagation explicit?

Mostly yes, with one weak spot. For spawned stacks (CLI/Docker/Electron), shutdown is explicit and cross-platform: detached process groups plus `signalProcessTree` (`scripts/dev-process-utils.mjs:85-105`), Windows tree kill (`:118-129`), SIGINT/SIGTERM/SIGHUP wiring with delayed SIGKILL escalation and a shutdown-hook registry (`scripts/dev-with-automation.mjs:1070-1097`, `:606-631`); graceful shutdown also releases agent-server conversation leases (`scripts/dev-safe.mjs:1110-1143`). Frontend lifecycle is explicit where it matters: providers save/restore prior queryClient/i18n instances on unmount (`src/components/providers/agent-server-ui-providers.tsx:65-88`), WebSocket reconnection stops only via explicit `disconnect()` (`src/hooks/use-websocket.ts:29-31`) with capped exponential backoff (`:18-20`), and cloud sandbox pause/resume gates the socket URL deterministically (`src/contexts/websocket-provider-wrapper.tsx:24-45`). Error propagation is the weak spot: failures funnel into a global QueryCache handler that fires UI toasts (`src/query-client-config.ts:41-77`) — there is no typed host-facing `onError`/event-emitter contract at the provider boundary. Hosts that want programmatic handling must inject a custom QueryClient and replicate cache-level behavior themselves.

### 4. Does the integration model work for both local-first and service deployments?

Yes, unusually well. The same frontend targets: laptop-local stacks (`npm run dev`, `bin/agent-canvas.mjs`), Docker sandboxes (`docker/entrypoint.sh` crash-resilient supervision keeps ingress returning 502s instead of dying when a backend child exits), remote VMs (backends are just `{host, apiKey}` entries switchable at runtime, `src/api/backend-registry/types.ts:4-13`), cloud (cloud kind + cookie/api-key auth modes + device-flow login components, `src/components/features/backends/device-flow-auth.tsx`), and hosted embeds (Vercel preset, `VITE_BASE_PATH` subpath serving). Version skew between embedded frontend and deployed backend is guarded by an enforced minimum-version floor with distinct unavailable/auth-failure/unsupported states (`src/api/agent-server-compatibility.ts:15-23`, `:72-80`). Auth has both zero-config mode (auto-generated session key injected at serve time, `src/api/agent-server-config.ts:119-132`) and hardened `--public` mode where the key is never baked into the bundle (`bin/agent-canvas.mjs:68-74`).

## Architectural Decisions

1. **Two-component embedding contract.** All host integration funnels through `AgentServerUIProviders` (convenience: providers + scoped root) and `AgentServerUIRoot` (manual style scope), re-exported from the root barrel (`src/lib/index.ts:8-15`). The standalone app consumes the same provider with `withStyleRoot={false}` (`src/entry.client.tsx:40-43`), so there is exactly one initialization path exercised by both distributions.
2. **Build-time CSS scoping over runtime shadow DOM.** Scoping is done by a PostCSS prefixer keyed off a stable attribute constant shared between build config and runtime code (`vite.config.ts:198-211` imports `AGENT_SERVER_UI_SCOPE_SELECTOR` from `src/styles/agent-server-ui-style-scope.ts:2`). This avoids iframe/shadow-DOM complexity but makes the attribute name a frozen public contract (it appears in host-facing docs, `docs/DEVELOPMENT.md:163`).
3. **Get/set indirection around module singletons.** Instead of exporting raw singletons, queryClient/i18n are accessed through `getQueryClient()`/`getI18n()` proxies that honor an "active" override set by the provider (`src/query-client-config.ts:83-117`, `src/i18n/index.ts:71-130`). Multiple embedded instances can coexist and imperative internal code (e.g. toast i18n lookups, `src/utils/custom-toast-handlers.tsx:97`) automatically resolves the host-injected instance.
4. **Layered configuration: env → window globals → localStorage registry.** Build-time env provides defaults; the published-binary path injects runtime values via head-script window globals (`getBakedSessionApiKey`, `src/api/agent-server-config.ts:102-132`) because prebuilt bundles ship without baked keys; values then seed an ordinary, editable backend registry (`src/api/backend-registry/storage.ts:118-148`). Key rotation across restarts is reconciled by re-syncing only the seeded loopback entry (`storage.ts:70-93`).
5. **Named telemetry instance + consent store as a library boundary.** Treating telemetry as a first-class embedding concern (named PostHog instance, separate persistence keys, external-store consent subscription, immutable attribution properties injected in `before_send`, `src/services/telemetry.ts:53-54`, `:132-146`, `:447-468`) reflects that embedded deployments inherit the host's privacy posture.
6. **Launcher-as-product.** The npm package ships a CLI that composes third-party processes (uvx-managed Python servers + Node ingress/static server) rather than bundling a server, keeping the JS artifact pure-frontend while still delivering a one-command experience (`bin/agent-canvas.mjs:151-167`).

## Notable Patterns

- **Provider-scoped dependency overrides with restore-on-unmount**: nested `AgentServerUIProviders` trees temporarily swap the active queryClient/i18n and put back whatever was there before (`src/components/providers/agent-server-ui-providers.tsx:65-88`) — verified by test asserting restoration after unmount (`__tests__/agent-server-ui-providers.test.tsx:164-171`).
- **Proxy-based facade exports**: `queryClient` and `i18n` defaults are `Proxy` objects binding to the currently-active instance, so legacy call sites stay instance-correct without prop drilling (`src/query-client-config.ts:107-117`, `src/i18n/index.ts:109-130`).
- **CI-enforced architectural rules**: a source-scanning test blocks ad-hoc HTTP clients so all agent-server access stays behind the typed client (`src/api/no-direct-agent-server-calls.test.ts:32-60`), and a package-metadata test pins the exports map, exact versions, and bans git dependencies that break global installs (`__tests__/package-library.test.ts:70-106`).
- **Readiness gating instead of blind startup**: Electron performs a two-stage wait (ingress up → real `/server_info` responds 200/401) before mounting the window, treating uvx cold-start latency (~minutes) as normal (`electron/main.mjs:213-240`, `:601-675`).
- **Cross-platform process management as a reusable utility**: spawn options, liveness checks, tree signaling, and Windows command resolution are factored into `dev-process-utils.mjs` and consumed by every launcher, with regression tests under `__tests__/scripts/`.
- **Theme tokens as public API**: ~80 `--oh-*` CSS variables are exported as typed constants (`AgentServerUICssVariableName`, `src/styles/agent-server-ui-style-scope.ts:89-95`) and settable via `styleOverrides` — theming is a compile-checked surface, not stringly-typed CSS hacking.

## Tradeoffs

- **localStorage convenience vs. host storage ownership**: fixed storage keys make zero-config embedding trivial but prevent hosts from relocating state (multi-tenant, server profiles, SSR). No storage interface exists anywhere in the backend-registry modules (`src/api/backend-registry/*.ts`).
- **Toast-centric errors vs. programmatic observability**: deduped global toasts give good end-user UX out of the box (`src/query-client-config.ts:48-61`) but make machine-readable failure ingestion awkward; the escape hatch (`meta.disableToast` + injected QueryClient) pushes complexity onto the host.
- **React Router coupling vs. component portability**: exporting route files directly (`src/components/conversation/index.ts:1` exports `../../routes/conversation`) maximizes feature fidelity but binds hosts to React Router v7 context and internal path conventions; the internal decoupling layer (`NavigationProvider`, `src/context/navigation-context.tsx:16-37`) was built for this purpose yet the flagship route bypasses it for navigation actions.
- **PostCSS scoping vs. portals/portals-like overlays**: attribute-prefix scoping handles ordinary DOM, but any UI rendered outside the scope element (browser `window.alert`-style APIs, document-title, toasts rendered at body level unless styled via tokens) needs manual care; the team mitigates by styling toasts with `--oh-*` tokens (`src/utils/custom-toast-handlers.tsx:15-29`).
- **Process-spawning CLI vs. library-only SDK**: shipping `uvx` orchestration gives instant usability but ties the JS release cadence to Python SDK versions (enforced via `config/defaults.json` sync check, referenced from `bin/agent-canvas.mjs:42`) and inherits uvx cold-start latency that the Electron loader must special-case.

## Failure Modes / Edge Cases

- **Missing injected key bricks onboarding**: without the launcher-injected window-global session key, `makeDefaultLocalBackend()` returns null, the registry seeds empty, and users land on the Manage Backends modal instead of onboarding (`src/api/agent-server-config.ts:114-117` docblock; seed logic `src/api/backend-registry/default-backend.ts:53-70`). This dependency of UX flow on a hidden global is documented but fragile for non-launcher embeds.
- **Locked-cloud deployments must suppress local seeding**, else a stale Local backend strands users behind recovery UI — handled explicitly (`default-backend.ts:46-56`).
- **Stale WebSocket close events**: replaced sockets' late close events are filtered via WeakSet/instance tracking so they don't clobber the replacement's connected state (`src/hooks/use-websocket.ts:30-31`, `:101-108`).
- **Cloud paused-sandbox race**: connecting to a stale `conversation_url` during PAUSED would immediately fail; the wrapper nulls the URL and fast-polls until resumed (`src/contexts/websocket-provider-wrapper.tsx:24-33`).
- **Orphaned grandchildren on shutdown**: naive `proc.kill` leaves uvx/python children holding ports; mitigated by negative-PID group kills and Windows taskkill trees (`scripts/dev-process-utils.mjs:77-129`).
- **Host teardown leaving telemetry state**: `configureTelemetry(false)` after init opt-outs capturing and notifies subscribers; re-enabling restores consent and identity (`src/services/telemetry.ts:205-231`) — but there is no per-provider dispose; telemetry state is process-global by design.
- **Backend-switch mid-route**: conversation subtree unmounts before foreign-backend fetches fire, preventing misleading 404 toasts and data-validation errors (`src/routes/conversation.tsx:180-192`).

## Future Considerations

- Introduce a storage-port interface (e.g., `storage?: StorageAdapter` on `AgentServerUIProviders`) so hosts can redirect backend-registry/consent persistence; today's hardcoded keys are the largest blocker for service-embedded deployments.
- Add a typed host callback surface (e.g., `onError`, `onEvent`, `onApprovalRequested` props or an emitter next to the providers) so hosts can log/route failures instead of parsing toasts; the QueryCache already centralizes the right chokepoint (`src/query-client-config.ts:31-81`).
- Finish the router decoupling for exported route components: route `ConversationView` through `NavigationContext` for navigation and expose conversation id as a prop with a context fallback (`src/routes/conversation.tsx:2`, `:100`, `:118`; existing seam at `src/context/navigation-context.tsx:28-41`).
- Publish a dedicated embedding guide covering conversation creation, working-dir overrides, secret provisioning, and backend pre-seeding — current docs stop at styling/providers (`docs/DEVELOPMENT.md:155-177`).
- Provide a reference host app under `examples/` (the directory currently contains only an ACP docker sample) exercising the library subpaths against a scripted agent-server.

## Questions / Gaps

- **No evidence found** of a supported headless/programmatic embedding API (e.g., driving conversations from host code without rendering UI): the library surface is exclusively React components (`__tests__/library-entrypoints.test.ts:10-49`); search boundary: `src/lib/**`, `src/components/*/index.ts`, package exports.
- **No evidence found** of injectable storage, HTTP transport, or clock/retry policies: searched `src/api/backend-registry/*`, `src/services/*`, and provider props; all persistence goes straight to Web Storage APIs.
- Whether any production host actually embeds the npm library could not be verified from this repository alone — the strongest in-repo signal is the dedicated test/mutation/CI investment around the library build (`__tests__/package-library.test.ts`, `vite.config.ts:215-246`, CI quality gates listing `build:lib` at `docs/architecture.md:68-78`), plus AGENTS.md guidance for "npm consumers" overriding analytics at runtime.
- Tool/policy customization depth (custom tool registration from the host) is out of this repo's boundary and lives in the sibling `software-agent-sdk`; within this source the only lever is which registered tools get attached to new-conversation payloads (`src/api/agent-server-adapter.ts:1195-1197`).

---

Generated by `dimensions/24.04-embedding-and-host-integration-ergonomics` against `openhands`.
