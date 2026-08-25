# Source Analysis: openhands

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (Agent Canvas frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19.2 / React Router 7 / Vite 8 |
| Analyzed | 2026-08-22 |

## Summary

Selected source is the `@openhands/agent-canvas` npm package — a React/TypeScript
frontend for the OpenHands Agent Server that ships three coordinated public
surfaces: (1) a CLI binary `agent-canvas` (`package.json:17` → `bin/agent-canvas.mjs`)
that launches the full local stack via `uvx`, (2) a standalone web app, and (3) a
set of npm subpath exports for embedding individual feature modules
(`package.json:206-248`). The library surface is anchored on `src/index.ts:1`
which re-exports `src/lib/index.ts:1-57`, and the public boundary is enforced
both by `tsconfig.lib.json` (narrowed declaration build) and by a CI test
`__tests__/library-entrypoints.test.ts:10-49` that asserts every published
subpath resolves. The package enforces two strong invariants: agent-server
calls must use `@openhands/typescript-client` (see CI guard at
`src/api/no-direct-agent-server-calls.test.ts:32-79`) and Cloud calls must
flow through `callCloudProxy()` (`src/api/cloud/proxy.ts:18-39`). The
telemetry module is intentionally exposed as a "React-facing consent store"
via `useTelemetry()` (`src/hooks/use-telemetry.ts:61-114`) backed by an
internal `src/services/telemetry.ts` singleton. The intended API audience
is split three ways — operators (CLI / docker image), application
developers (the web app), and extension authors (npm subpath library
consumers) — with the browser-side `AgentServerUIProviders` provider
(`src/components/providers/agent-server-ui-providers.tsx:48-123`) being the
canonical host integration entry point. Examples (`examples/acp-docker/`)
and docs (`docs/architecture.md`, `docs/SELF_HOSTING.md`) cover the operator
CLI but the library consumer path relies almost entirely on the typed
entrypoints rather than runnable examples.

## Rating

**Rating: 7 / 10** — Tier: Clear model with tests, explicit interfaces, and operational safeguards

**Score:** 7
**Score (out of 10):** 7/10
**Tier:** 7-8 band

Rationale: The package presents a deliberate, multi-tier public surface
(CLI, standalone app, npm library subpaths, browser provider). Boundaries
are explicit (`package.json` exports map, `tsconfig.lib.json` narrowed
declaration scope, `__tests__/package-library.test.ts` and
`__tests__/library-entrypoints.test.ts` enforce that the published surface
stays stable, `src/api/no-direct-agent-server-calls.test.ts:32-79`
prevents accidental axios/fetch surface area). Provider scopes and theme
tokens (`AGENT_SERVER_UI_SCOPE_ATTRIBUTE` / `AGENT_SERVER_UI_SCOPE_SELECTOR`
/ `AGENT_SERVER_UI_DEFAULT_CSS_VARIABLES` at `src/styles/agent-server-ui-style-scope.ts:1-87`)
are explicitly published for library customization. The library build is
genuinely production-shaped (`vite.config.ts` with `BUILD_LIB=true`,
preserved modules in `dist/`, `package.json:206-248` `exports` with
`types`/`import`/`require` triplets for every subpath). However, the
library entrypoints contain many sub-modules that are *not* covered by
discoverable runnable examples — `examples/` only contains an ACP-Docker
sample for the operator CLI. The host integration story (drop
`AgentServerUIProviders` into a host app) is documented in JSDoc but not
walked through in any `examples/embed-host-app/` or external docs page.
Some implementation surface (e.g. `Zustand` stores under `src/stores/`,
route files under `src/routes/`) is intentionally internal but the
boundary between "library re-export" and "internal-only" is partly
implicit — `src/lib/index.ts` does not export them, but their existence
is referenced by the type-checked internal imports. The CLI public
surface (`bin/agent-canvas.mjs:26-122`) is well-documented with `--help`
output and is backed by integration tests at `__tests__/bin/agent-canvas.test.ts`.
The reduced score vs. 8-9 is driven by: (a) lack of host-app embedding
example, (b) some "library re-export" surface points to internal
routes (`src/components/conversation/index.ts:1` re-exports
`routes/conversation`) which couples library consumers to the app's
router state, (c) only the CLI surface has user-facing example docs.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Package manifest with bin + exports | `package.json` declares `bin.agent-canvas`, `main`, `module`, `types`, and 8 subpath exports | `package.json:16-18`, `package.json:195-248` |
| Library root entry re-exports the barrel | `src/index.ts` exports the barrel and nothing else | `src/index.ts:1` |
| Library barrel: component domains + providers + telemetry + i18n + style scope | Single barrel file aggregating every public surface | `src/lib/index.ts:1-57` |
| Per-domain barrels (browser, conversation, files, settings, sidebar, terminal) | Each domain re-exports a curated component set | `src/components/browser/index.ts:1-3`, `src/components/conversation/index.ts:1-8`, `src/components/files/index.ts:1-5`, `src/components/settings/index.ts:1-6`, `src/components/sidebar/index.ts:1-5`, `src/components/terminal/index.ts:1` |
| Host embedding provider | `AgentServerUIProviders` exposes `queryClient`, `i18n`, `analytics`, `withStyleRoot` overrides | `src/components/providers/agent-server-ui-providers.tsx:48-123` |
| Standalone scoped root component | `AgentServerUIRoot` accepts `className`, `theme`, `styleOverrides`, `contentClassName` | `src/components/providers/agent-server-ui-root.tsx:21-56` |
| CSS isolation contract — exported constants | `AGENT_SERVER_UI_SCOPE_ATTRIBUTE`, `_SCOPE_SELECTOR`, `_DEFAULT_CSS_VARIABLES`, `_DEFAULT_THEME` published for host CSS targeting | `src/styles/agent-server-ui-style-scope.ts:1-95` |
| Telemetry public hook | `useTelemetry` returns `consent`, `isEnabled`, `showConsentPrompt`, `grantConsent`, `denyConsent`, `track`, `clearData` | `src/hooks/use-telemetry.ts:61-114` |
| Telemetry imperative API | `configureTelemetry`, `setTelemetryConsent`, `getTelemetryConsent`, `trackEvent`, `trackInstall`, `trackSessionStart`, `clearTelemetryData` | `src/services/telemetry.ts:205`, `:392`, `:423`, `:537`, `:666`, `:766`, `:809`, `:820`, `:833` |
| i18n public API | `OPENHANDS_I18N_NAMESPACE`, `AvailableLanguages`, `createAgentServerI18n`, `getDefaultI18n`, `getI18n`, `setI18n`, `translationResources`, `waitForI18n` | `src/i18n/index.ts:13-92` |
| QueryClient singleton (imperative access) | `createAgentServerQueryClient`, `getDefaultQueryClient`, `getQueryClient`, `queryClient` proxy, `setQueryClient` | `src/query-client-config.ts:31-117` |
| Agent-server typed-client access helpers | `getAgentServerClientOptions`, `getAgentServerHttpClientOptions`, `NoBackendAvailableError` | `src/api/agent-server-client-options.ts:22-79` |
| Cloud proxy contract | `callCloudProxy({ backend, method, path, body, headers, authMode, hostOverride, sessionApiKey })` | `src/api/cloud/proxy.ts:5-39` |
| Backend registry types | `Backend`, `BackendKind`, `BackendAuthMode`, `BackendSelection`, `ResolvedActiveBackend` | `src/api/backend-registry/types.ts:1-23` |
| Active backend React context | `ActiveBackendProvider`, `useActiveBackendContext`, `useActiveBackend` | `src/contexts/active-backend-context.tsx:71-251` |
| Routing-decoupled navigation context | `NavigationProvider`, `useNavigation`, `NavigationOptions`, `NavigationContextValue` | `src/context/navigation-context.tsx:7-41` |
| Route-decoupled link component | `NavigationLink` (`to`, `replace`, `end`, className state fn) | `src/components/shared/navigation-link.tsx:11-105` |
| Route table (standalone-app-only) | `routes.ts` lists React Router 7 routes (standalone app, not re-exported by the library barrel) | `src/routes.ts:8-44` |
| CLI binary flag surface | `--version`, `--info`, `--help`, `--public`, `--frontend-only`, `--backend-only`, `-p/--port` | `bin/agent-canvas.mjs:26-122` |
| Runtime env-var surface | `LOCAL_BACKEND_API_KEY`, `OH_AGENT_SERVER_GIT_REF`, `OH_AGENT_SERVER_LOCAL_PATH`, `OH_AGENT_SERVER_VERSION`, `OH_SECRET_KEY` (CLI help) | `bin/agent-canvas.mjs:85-95` |
| Service adapter pattern + naming rules | `src/api/README.md` documents the `feature-service/feature-service.api.ts` pattern, hooks-vs-direct-call guidance, naming | `src/api/README.md:1-98` |
| Backend-registry typed access | `BackendKind`, `Backend`, `BackendSelection` exported from registry types | `src/api/backend-registry/types.ts:1-23` |
| Agent-server version compatibility helper | `compatibility.minimumAgentServer` from defaults.json; `assertAgentServerVersionIsSupported()` in adapter | `src/api/agent-server-compatibility.ts` (mentioned in `AGENTS.md`), `config/defaults.json` (referenced in `__tests__/api/agent-server-compatibility-bundled-pin.test.ts`) |
| Agent-server typed-client rule | "All calls targeting the local agent-server must go through typed client classes from `@openhands/typescript-client`" (enforced by CI) | `src/api/no-direct-agent-server-calls.test.ts:32-79` |
| Cloud backend rule | "Any call from the browser to the cloud backend must go through `callCloudProxy()`" | `src/api/cloud/proxy.ts:18-39`, AGENTS.md "API Access Rules" |
| Library-entrypoint CI guard | `library-entrypoints.test.ts` asserts each published symbol resolves and the removed `AgentServerSettings` no longer leaks | `__tests__/library-entrypoints.test.ts:10-49` |
| Package-library metadata guard | `package-library.test.ts` enforces `name`, `main`, `module`, `types`, `exports`, no git deps, exact-semver pinning, postinstall greeting | `__tests__/package-library.test.ts:23-158` |
| Library i18n namespace contract | `library-namespace.test.ts` asserts `OPENHANDS_I18N_NAMESPACE` and the lazy `translationResources` re-export for host registration | `__tests__/i18n/library-namespace.test.ts:1-64` |
| Provider-stack test for host integration | `agent-server-ui-providers.test.tsx` exercises `AgentServerUIProviders` with custom `queryClient`/`i18n` overrides | `__tests__/agent-server-ui-providers.test.tsx:1-279` |
| Style-scope isolation test | `agent-server-ui-style-scope.test.ts` exercises the `[data-agent-server-ui]` selector transform | `__tests__/agent-server-ui-style-scope.test.ts` |
| Standalone-app entry | `src/root.tsx` mounts the standalone app inside `AgentServerUIRoot` (with its own `data-agent-server-ui=""`) and gates first-run onboarding / recovery screens | `src/root.tsx:92-380` |
| Hydration entry (consumers set `withStyleRoot={false}`) | Standalone app avoids nested `AgentServerUIRoot` shells via `entry.client.tsx` setting `withStyleRoot={false}` | `src/entry.client.tsx:11-49` |
| Docker image | `ghcr.io/openhands/agent-canvas` image exposes port 8000 with unified routing | `docker/Dockerfile`, AGENTS.md "Docker all-in-one image" |
| Backend registry seed default | `BUNDLED_BACKEND_ID = "default-local"`, `makeDefaultLocalBackend()` for first-run | AGENTS.md "Backend registry" |
| Authentication mode public surface | `--public` flag, `LOCAL_BACKEND_API_KEY` env var, `ApiKeyEntryScreen`, `isAgentServerAuthError()` | `bin/agent-canvas.mjs:54`, `src/components/features/backends/api-key-entry-screen.tsx` |
| Locked-to-Cloud configuration | `getLockedCloudHost()`, `getLockedCloudAuthMode()`, `__AGENT_CANVAS_LOCK_TO_CLOUD__` window global | `src/api/agent-server-config.ts:141-181` |
| Test mapping for selective E2E | `tests/e2e/mock-llm/test-mapping.json` driven by `scripts/resolve-affected-tests.mjs` | `tests/e2e/mock-llm/test-mapping.json` (file exists in repo root), AGENTS.md "Selective test execution" |

## Answers to Dimension Questions

1. **What is the intended public API surface?**
   Three coordinated surfaces, each with its own audience:
   - **Operator CLI**: `bin/agent-canvas.mjs` exposes flags
     `-v/--version`, `--info`, `--public`, `--frontend-only`,
     `--backend-only`, `-p/--port`, plus `LOCAL_BACKEND_API_KEY`,
     `OH_AGENT_SERVER_*`, `OH_SECRET_KEY`, `OH_AUTOMATION_*` env vars
     (`bin/agent-canvas.mjs:26-122`).
   - **Standalone web app**: `src/root.tsx` mounts the React Router 7 app
     behind `AgentServerUIRoot` (`src/root.tsx:92-113`), gated by
     first-run onboarding and backend-recovery screens
     (`src/root.tsx:339-380`).
   - **Library consumers**: `package.json:206-248` declares 8 npm
     subpath exports (`./`, `./browser`, `./conversation`, `./files`,
     `./settings`, `./sidebar`, `./terminal`, `./i18n`). Each subpath
     is wired through the corresponding domain barrel under
     `src/components/<domain>/index.ts` and re-exported by
     `src/lib/index.ts:1-57`. The host integration entry is
     `AgentServerUIProviders` (`src/components/providers/agent-server-ui-providers.tsx:48-123`).
2. **Is the stable API easy to distinguish from internal implementation details?**
   Yes, with caveats. The library boundary is enforced three ways:
   (a) `package.json` `exports` map with `types`/`import`/`require`
   triplets per subpath (`package.json:206-248`), (b)
   `tsconfig.lib.json` narrowed declaration emit with `src/library-env.d.ts`
   (AGENTS.md "Library packaging notes"), (c) `__tests__/package-library.test.ts:23-158`
   asserts `main`/`module`/`types`/`exports` shape and that no git
   dependencies leak. `__tests__/library-entrypoints.test.ts:44-48`
   additionally asserts that removed exports (`AgentServerSettings`)
   stay removed. However, the runtime convention that agent-server
   access must use `@openhands/typescript-client`
   (`src/api/no-direct-agent-server-calls.test.ts:32-79`) is enforced
   by CI but not exposed in any published subpath — it is an
   *implementation* rule that has no public symbol. Some
   domain barrels re-export routes (`src/components/conversation/index.ts:1`
   re-exports `routes/conversation`) — those route components carry
   React Router state coupling that library consumers may not want.
3. **Does the API expose the right level of abstraction for agent harness users?**
   Mostly yes. Operator CLI hides the underlying `uvx` agent-server /
   automation backend, the ingress proxy, and the static-server
   behind one binary. Library consumers get a single
   `AgentServerUIProviders` provider that wires up
   QueryClient / i18n / telemetry / ActiveBackendProvider / style scope
   in one shot (`src/components/providers/agent-server-ui-providers.tsx:48-123`).
   The CSS-isolation contract is published as plain constants
   (`src/styles/agent-server-ui-style-scope.ts:1-95`) so host apps can
   target `[data-agent-server-ui]` directly without parsing source.
   Theme tokens live as `--oh-*` CSS variables under
   `[data-agent-server-ui]` so restyling does not require any React
   surface (AGENTS.md "Theme/customization tokens"). The abstraction
   for the "active backend" is a typed `Backend` registry with both
   imperative (`getActiveBackend`, `getEffectiveLocalBackend`) and
   React (`useActiveBackend`, `useActiveBackendContext`) entry points
   (`src/api/backend-registry/active-store.ts`,
   `src/contexts/active-backend-context.tsx:71-251`), but no dedicated
   npm subpath exposes either — they are reachable only through the
   `src` index, not as their own subpath.
4. **Are examples sufficient to use the API correctly without reading internals?**
   Not really. The operator CLI has a strong `--help` output
   (`bin/agent-canvas.mjs:58-122`), README quickstart covering npm /
   Docker / source installs (`README.md:50-118`), ACP Docker example
   (`examples/acp-docker/README.md`), and `docs/SELF_HOSTING.md`
   with `systemd` / `tmux` snippets. For library consumers there are
   no examples under `examples/` — only JSDoc on `AgentServerUIProviders`
   (`src/components/providers/agent-server-ui-providers.tsx:1-123`)
   and inline JSDoc on `useTelemetry`
   (`src/hooks/use-telemetry.ts:30-60`). The library build is
   documented by AGENTS.md "Library packaging notes" but that doc is
   for contributors, not consumers. There is no `examples/embed-host-app/`
   showing how to mount `AgentServerUIProviders` into a host React app.
   `library-namespace.test.ts:1-64` doubles as a usage example for the
   i18n namespace contract, which is the strongest library-side example.

## Architectural Decisions

- **Layered package shape (CLI / app / library)**: `package.json:16-18`
  declares the CLI `bin`, `package.json:74-115` defines dev/build
  scripts for each mode (`dev`, `dev:static`, `dev:minimal`, `dev:mock`,
  `build:app`, `build:lib`, `build:desktop`), and `package.json:195-248`
  maps each library subpath to a `dist/` artifact. Each audience gets a
  distinct contract.
- **Library surface is narrow on purpose**: `src/lib/index.ts:1-57`
  aggregates only providers, theme scope, query/i18n singletons, and
  per-domain component barrels. Stores, routes, services, hooks (except
  `useTelemetry`), and helpers are *not* re-exported.
- **Two hard CI-enforced API rules**: agent-server calls must use
  `@openhands/typescript-client` (`src/api/no-direct-agent-server-calls.test.ts:32-79`)
  and Cloud calls must go through `callCloudProxy`
  (`src/api/cloud/proxy.ts:18-39`, AGENTS.md "API Access Rules").
  These exist to keep the public surface owned by a separate
  `@openhands/typescript-client` package rather than re-implemented
  in this repo.
- **Browser-side scope attribute for CSS isolation**: every public
  library component is rendered under `[data-agent-server-ui]`
  (`src/components/providers/agent-server-ui-root.tsx:21-56`,
  `src/styles/agent-server-ui-style-scope.ts:1-95`). This attribute is
  the *only* official embed hook; host CSS targets it via the exported
  `AGENT_SERVER_UI_SCOPE_SELECTOR` constant.
- **i18n namespace is namespaced and lazy-loaded**: `OPENHANDS_I18N_NAMESPACE = "openhands"`
  (`src/i18n/index.ts:13`), and the 1 MB translation bundle is imported
  through `src/i18n/resources.ts` and only re-exported via a
  `/* @__PURE__ */` annotation so library consumers pay only for what
  they request (`src/i18n/index.ts:11`, AGENTS.md "Bundle/dev-graph
  hygiene").
- **Routing is decoupled from React Router internals** for library
  consumers: components import `useNavigation()` /
  `NavigationProvider` from `src/context/navigation-context.tsx:7-41`,
  and `NavigationLink` (`src/components/shared/navigation-link.tsx:11-105`)
  wraps an `<a>` with `useNavigation().navigate(...)`. The standalone
  app bridges router state with `src/routes/react-router-navigation-provider.tsx`.
  This means the library surface does not require the consumer to wire
  React Router 7.
- **Telemetry is opt-in for hosts and one PostHog client per canvas**:
  `AgentServerUIProviders.analytics` accepts `AgentServerUIAnalyticsConfig`
  (`src/components/providers/agent-server-ui-providers.tsx:28-46`) which
  is either `{provider: "posthog", apiKey?, apiHost?, uiHost?}` or
  `false`/`null` to disable. The single named `agent-canvas` PostHog
  instance is owned by `src/services/telemetry.ts` — there is no
  PostHog SDK access exported (only `useTelemetry` +
  `configureTelemetry`/`trackEvent`/`trackInstall`/`trackSessionStart`).
- **Authentication modes are explicit and enumerated**: local default
  auto-generates and injects the session key, `--public` requires
  `LOCAL_BACKEND_API_KEY` and shows `ApiKeyEntryScreen`
  (`src/components/features/backends/api-key-entry-screen.tsx`,
  AGENTS.md "Auth modes"). `isAgentServerAuthError()` lives at
  `src/api/agent-server-compatibility.ts` and the gate is in
  `src/root.tsx`'s `App` component.

## Notable Patterns

- **Per-domain barrel with `src/lib/index.ts` aggregator**:
  `src/lib/index.ts:1-57` is the *only* module re-exported by
  `src/index.ts:1`. Each domain barrel is one line per symbol, e.g.
  `src/components/terminal/index.ts:1` exports the default `TerminalPanel`.
  This keeps `package.json` exports thin and uniform.
- **Service-adapter object-literal pattern**: `src/api/README.md:1-98`
  documents the convention that each domain owns
  `feature-service/feature-service.api.ts` + `feature.types.ts`.
  Concrete services follow it: `secrets-service.ts`
  (`src/api/secrets-service.ts:26`), `skills-service.ts`
  (`src/api/skills-service.ts:67`), `option-service/option-service.api.ts:48`,
  `mcp-service/mcp-service.api.ts:344`. These are *not* re-exported by
  the library barrel — they are the app's internal adapter layer that
  wraps `@openhands/typescript-client`.
- **Typed-client constructor + shared options helper**:
  `getAgentServerClientOptions()` (`src/api/agent-server-client-options.ts:52-69`)
  is the single way to assemble host/apiKey/workingDir for any SDK
  client constructor (`ConversationClient`, `FileClient`, `ServerClient`,
  `VSCodeClient`). For `RemoteWorkspace` /
  `RemoteEventsList`, the parallel `getAgentServerHttpClientOptions()`
  returns `baseUrl` instead of `host`
  (`src/api/agent-server-client-options.ts:71-79`).
- **Discriminated-union backend registry**: `Backend.kind` is a
  string-literal `"local" | "cloud"` (`src/api/backend-registry/types.ts:1`)
  with `BackendAuthMode = "api-key" | "cookie"` adding cloud auth
  variants. Active selection flows through `BackendSelection` with
  optional `orgId` (`src/api/backend-registry/types.ts:15-23`).
- **Proxy indirection for browser-to-cloud calls**: `callCloudProxy`
  is a thin wrapper that POSTs the request envelope to
  `/api/cloud-proxy` on the local agent-server, which forwards
  server-side to bypass browser CORS
  (`src/api/cloud/proxy.ts:18-39`).
- **Window globals for runtime configuration that npm consumers must
  not bake**: `window.__AGENT_CANVAS_SESSION_API_KEY__` and
  `window.__AGENT_CANVAS_LOCK_TO_CLOUD__` are read by
  `getBakedSessionApiKey()` and `getLockedCloudHost()`
  (`src/api/agent-server-config.ts:119`, `:141`). The static server
  (`scripts/static-server.mjs`) and Docker entrypoint inject them at
  serve time, allowing postinstall flexibility without a rebuild.
- **Operations-bound helper for runtime services**: the
  `<RUNTIME_SERVICES>` block rendered into the agent system prompt is
  built by `buildRuntimeServicesSystemSuffix()`
  (`src/api/agent-server-adapter.ts:215-300`) from a single
  `RuntimeServicesInfo` object whose shape is documented in AGENTS.md
  "`/server_info.runtime_services` shape". This keeps the agent-server
  topology authoritative without leaking internal ports.

## Tradeoffs

- **Library re-exports routes**: `src/components/conversation/index.ts:1`
  re-exports `ConversationView` from `routes/conversation.tsx`. Library
  consumers pulling `ConversationView` will pull the standalone-app
  router contract. This couples library consumers to React Router 7 and
  to a specific `ConversationWebSocketProvider` shape; either the
  barrel should drop `ConversationView` or `ConversationView` should
  be moved to a router-agnostic component. `peerDependencies` pin
  React Router 7.18.2 (`package.json:249-253`), so the consumer is
  forced into that version.
- **Browser-coupled telemetry**: `TelemetryProvider` is wired only by
  `AgentServerUIProviders` (`src/components/providers/agent-server-ui-providers.tsx:98-100`)
  and the standalone `src/entry.client.tsx:40-45` always passes
  `DEFAULT_AGENT_SERVER_ANALYTICS` (`src/components/providers/agent-server-ui-providers.tsx:33-35`).
  Library consumers who want to disable PostHog must explicitly pass
  `analytics={false}` — the default is opt-out, not opt-in.
- **No docs page for library embedding**: the standalone-app audience
  is documented in `README.md`, `docs/SELF_HOSTING.md`,
  `docs/architecture.md`, `docs/ACP_AGENTS.md`, and `examples/acp-docker/`.
  The library-consumer audience has none of these — only JSDoc and
  AGENTS.md contributor notes. Discoverability is poor; a host developer
  evaluating `@openhands/agent-canvas` has to read source to find the
  provider.
- **Tight coupling between library barrel and `src/components/...`**:
  the barrel re-exports from feature-component paths
  (`src/components/features/settings/index.ts:1-6`) which import
  from `routes/*` and from internal hooks. Consumers receive the
  whole graph via Rollup tree-shaking but cannot target a smaller
  feature like `MCPSettings` without pulling `routes/mcp-settings.tsx`.
- **Two CLI launchers behave differently**: `npm run dev` / `dev:static`
  are for source development, but the published `agent-canvas` binary
  is a thin wrapper around `scripts/dev-with-automation.mjs` with
  `staticMode: true` (`bin/agent-canvas.mjs:151-172`). Operator
  expectations set by the `--info` and `--help` output may not match
  dev-mode behavior (e.g. `--backend-only` is a `main()` option that
  works in dev but is also accepted by the binary — they share the
  same launcher entry).
- **`@openhands/typescript-client` exemption from version pinning**:
  `package.json:26-27` pins `@openhands/extensions: 0.18.0` and
  `@openhands/typescript-client: 1.38.1` as stack pins (allowed by
  `package-library.test.ts:24-27`), while direct `dependencies` and
  `devDependencies` are exact-pinned to the patch
  (`__tests__/package-library.test.ts:88-106`). This tradeoff keeps
  the SDK API surface stable but couples npm releases to upstream
  SDK releases.

## Failure Modes / Edge Cases

- **CLI without a build**: `bin/agent-canvas.mjs:138-149` aborts with
  a "packaging error" message if `build/` is missing. The dev fallback
  (`scripts/dev-with-automation.mjs`) is not invoked — npm consumers
  who `npm install` without preinstalling dependencies break.
- **`--public` without `LOCAL_BACKEND_API_KEY`**: documented in the
  CLI help (`bin/agent-canvas.mjs:84-95`) but the runtime enforcement
  is in `scripts/dev-with-automation.mjs` (referenced via `main()` in
  `bin/agent-canvas.mjs:151`). Public-mode failures manifest as a
  blank canvas until `ApiKeyEntryScreen` renders
  (`src/components/features/backends/api-key-entry-screen.tsx`).
- **`--frontend-only --public` is rejected**:
  `bin/agent-canvas.mjs:132-135` rejects the combination explicitly.
- **`--frontend-only --backend-only` is rejected**:
  `bin/agent-canvas.mjs:125-130` rejects the combination explicitly.
- **No backend available**: `getAgentServerClientOptions()` throws
  `NoBackendAvailableError` (`src/api/agent-server-client-options.ts:55-58`)
  when no host, conversationUrl, or backend is supplied. The standard
  pattern in `src/api/README.md` is for service code to let it
  propagate and rely on the QueryClient toast handler
  (`src/query-client-config.ts:31-77`) to render the error.
- **Agent-server compatibility floor**: `assertAgentServerVersionIsSupported()`
  (mentioned in AGENTS.md "Frontend compatibility") gates conversation
  start on `compatibility.minimumAgentServer` from
  `config/defaults.json`. A user on an older agent-server is blocked
  with the unsupported-version recovery screen
  (`src/root.tsx:339-380`).
- **Locked-to-Cloud host mismatch**: `isSameCloudHost()` is a
  normalized host comparison (`src/api/agent-server-config.ts:167-175`);
  a stale persisted Cloud backend pointing at a different host is
  treated as not-the-locked-backend, forcing first-run onboarding.
- **`useTelemetry` first paint**: the install event fires before
  consent (`src/hooks/use-telemetry.ts:77-83`). `VITE_DO_NOT_TRACK=1`
  or browser DNT suppresses it, but library consumers must explicitly
  configure otherwise.
- **Translation completeness**: `npm run check-translation-completeness`
  is enforced in lint-staged (`package.json:124-126`) and as a separate
  CI script (`package.json:103`); a missing locale falls over before
  merge.

## Future Considerations

- **Add a dedicated `examples/embed-host-app/`** showing how to mount
  `AgentServerUIProviders` into a host React app — the JSDoc on
  `AgentServerUIProviders` is the only guidance today
  (`src/components/providers/agent-server-ui-providers.tsx:1-47`).
- **Decouple `ConversationView` from `routes/conversation.tsx`** so
  the `conversation` subpath can offer a router-agnostic chat
  surface; the current re-export forces React Router 7
  (`src/components/conversation/index.ts:1`,
  `package.json:252` peerDeps).
- **Publish a dedicated `./backend-registry` or `./backends` subpath**
  for `ActiveBackendProvider` / `useActiveBackendContext` so library
  consumers do not have to import from `#/contexts/active-backend-context`
  (currently reachable only through `src/lib/index.ts`'s provider
  re-export which only surfaces `AgentServerUIProviders`).
- **Move from peerDependencies to bundled React Router internals** so
  library consumers can pick their own router; today
  `react-router: 7.18.2` is a peer
  (`package.json:249-253`).
- **Consider marking experimental modules with explicit
  `@experimental` JSDoc or `__experimental` prefix** — today the
  public/subpath surface has no stability markers (no `@stable`,
  `@beta`, `@deprecated` tags in any exported JSDoc in the
  `src/components/<domain>/index.ts` files).
- **Extend `--info` output** (`bin/agent-canvas.mjs:32-53`) to
  include the active `compatibility.minimumAgentServer` version plus
  the library `dist/` layout — currently it shows agent-server /
  automation versions but not the embedding contract.
- **Replace `bin/agent-canvas.mjs`'s dynamic `await import("../scripts/dev-with-automation.mjs")`**
  with a stable launcher module so the CLI surface and dev launcher
  can diverge (`bin/agent-canvas.mjs:151-159`).

## Questions / Gaps

- No clear evidence found for an externally published host-app
  embedding example or `examples/embed-host-app/`. The README focuses
  on standalone install options (`README.md:50-118`); the only `examples/`
  entry is `examples/acp-docker/` which is operator-oriented.
- No clear evidence found for a public API stability marker scheme
  (no `@stable`, `@beta`, `@deprecated`, or `__experimental__`
  prefixes in any `src/components/<domain>/index.ts` re-exports).
- No clear evidence found for an externally documented
  `AgentServerUIProviders` migration guide or "Getting Started for
  Embedders" doc under `docs/`. AGENTS.md is contributor-facing.
- No clear evidence found for a CJS-only consumer test; the library
  builds CJS and ESM (`package.json:206-247`) but
  `__tests__/library-entrypoints.test.ts` only verifies symbol
  existence, not CJS resolution shape.
- No clear evidence found for documented behavior of
  `AgentServerUIProviders` when nested inside another
  `[data-agent-server-ui]` ancestor (the CSS-isolation test
  `__tests__/agent-server-ui-style-scope.test.ts` exercises the
  selector transform, but the runtime nested-scope scenario is only
  covered by the AGENTS.md "Public embedding entry points" note).
- The exact runtime contract of `setTelemetryConsent` with respect
  to `syncToCloud` (`src/services/telemetry.ts:537`) is documented
  in AGENTS.md but not exported as part of the public library
  surface — there is no `__tests__/library-entrypoints.test.ts`
  case for it.

---

Generated by `studies/agent-harness-study/reports/source/24.01-public-api-surface/dimension-24.01-public-api-surface.md` against `openhands`.
