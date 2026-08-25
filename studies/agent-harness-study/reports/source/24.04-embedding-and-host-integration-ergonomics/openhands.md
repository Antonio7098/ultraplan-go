# Source Analysis: openhands

## Dimension 24.04: Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands Agent Canvas) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Vite 8, React Router 7, TanStack Query 5, Zustand, Tailwind/HeroUI; Node ≥22 launchers (`package.json` `engines`) |
| Analyzed | 2026-08-25 |

## Summary

This repository is the OpenHands "Agent Canvas" frontend (`@openhands/agent-canvas`), and its embedding story is explicitly a **UI-component embedding** story, not a headless harness-embedding story. It ships as an npm library with a curated public surface (`src/index.ts:1` → `src/lib/index.ts:1-57`) plus subpath exports for domain panels (`package.json:206`, exports map for `.`, `./browser`, `./conversation`, `./files`, `./settings`, `./sidebar`, `./terminal`, `./i18n`). The central embedding entry point is `AgentServerUIProviders` (`src/components/providers/agent-server-ui-providers.tsx:48-123`), which lets a host inject its own TanStack Query client, i18next instance, analytics config (or hard-disable telemetry), theme, and CSS-variable style overrides, all rendered inside a CSS-scoped shell (`[data-agent-server-ui]`) produced by a build-time selector prefixer (`vite.config.ts:195-214`).

Beyond the library mode, the same codebase supports three more embedding/deployment modes: a CLI subprocess launcher that spawns the Python agent-server and automation backend behind one ingress port (`bin/agent-canvas.mjs:54-56,76-84`), a Docker all-in-one image, and an Electron desktop shell that embeds the whole stack in a BrowserWindow with explicit two-stage readiness waits and graceful process-tree shutdown (`electron/main.mjs:700-748`).

The consistent architectural boundary is **out-of-process delegation**: storage of settings/secrets, tool execution, policy/approvals, and agent identity all live on the external agent-server (reached exclusively through the typed `@openhands/typescript-client` dependency, enforced by a CI test at `src/api/no-direct-agent-server-calls.test.ts:32-79`). The host retains ownership of UX composition, theming, i18n, query caching, and analytics identity — but *not* of policy, state persistence, or tool execution, which are delegated to whatever backend is registered.

## Rating

**6 / 10** — The UI-library embedding path is genuinely well engineered: dependency injection for query client/i18n/analytics is real and tested (`__tests__/agent-server-ui-providers.test.tsx:97-279`), CSS isolation is enforced at build time and covered by regression tests, telemetry isolation uses a named PostHog instance with an explicit host disable switch, and lifecycle cleanup for sockets/processes is deliberate. It falls short of 7–8 because: exported components still require a host-provided `react-router` context despite an incomplete `NavigationContext` abstraction (`src/components/features/chat/chat-interface.tsx:2` vs `src/context/navigation-context.tsx:26-47`); the default query client couples error handling to global toasts (`src/query-client-config.ts:41-77`); there is no headless event-callback API so hosts cannot drive their own UX off harness events from this package; module-level singletons and import-time localStorage reads remain (`src/api/backend-registry/active-store.ts:116-120`); and embedding documentation is a single short docs section (`docs/DEVELOPMENT.md:151-177`). Hosts cannot supply policy, tools, or storage in-process at all — those are structurally delegated to the agent-server process.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Library/SDK entry points | Root export re-exports `src/lib`; public surface includes `AgentServerUIProviders`, `AgentServerUIRoot`, query-client accessors, i18n factory, style-scope constants, telemetry API | `src/index.ts:1`, `src/lib/index.ts:7-57` |
| Package subpath exports | `exports` map: root + browser/conversation/files/settings/sidebar/terminal/i18n subpaths, ESM+CJS+types; `bin.agent-canvas` CLI | `package.json:206` (exports block), `bin` field near top of file |
| Library build | `BUILD_LIB=true` switches Vite to lib mode: ES+CJS output, `preserveModules`, d.ts emit via `tsconfig.lib.json` | `vite.config.ts:108,215-239` |
| CSS isolation | All bundled CSS prefixed under `[data-agent-server-ui]` via `postcss-prefix-selector`; global selectors (`:root/html/body`) remapped onto the scope | `vite.config.ts:195-214`, `src/styles/agent-server-ui-style-scope.ts:1-2,97-117` |
| Provider DI surface | Props: `queryClient?`, `analytics?: AgentServerUIAnalyticsConfig \| false \| null`, `i18n?`, `withStyleRoot?`, plus theme/style overrides | `src/components/providers/agent-server-ui-providers.tsx:24-46` |
| Nested-provider safety | Previous queryClient/i18n saved on first render and restored on unmount | `src/components/providers/agent-server-ui-providers.tsx:65-88` |
| Scoped style root | `data-agent-server-ui` wrapper injects ~70 `--oh-*` CSS variables; `styleOverrides` merged over defaults; `theme` prop sets `dark/light/default` | `src/components/providers/agent-server-ui-root.tsx:21-55`, `src/styles/agent-server-ui-style-scope.ts:3-95` |
| Telemetry hard-disable for hosts | `configureTelemetry(false)` opts out of capturing incl. install event and notifies listeners | `src/services/telemetry.ts:205-231` |
| Telemetry singleton isolation | Named PostHog instance `"agent-canvas"` with dedicated persistence names "isolates Canvas configuration … from a host application's default PostHog singleton" | `src/services/telemetry.ts:53,344-362` |
| Runtime config injection | Session key read from build-time env then `window.__AGENT_CANVAS_SESSION_API_KEY__` injected by static server; auth-required flag and cloud-lock window globals | `src/api/agent-server-config.ts:119-132,214-221,14` |
| Backend registry (connection config) | `Backend {id,name,host,apiKey,kind,authMode,connectionRevision}`; registry snapshot persisted under localStorage keys `openhands-backends` / `openhands-active-backend` | `src/api/backend-registry/types.ts:5-14`, `src/api/backend-registry/storage.ts:13-14` |
| Programmatic backend mutation API | `ActiveBackendContextValue`: `setActive/addBackend/updateBackend/removeBackend`; read hook falls back to synthesized env-derived backend outside provider | `src/contexts/active-backend-context.tsx:31-38,247-251` |
| Default backend seeding | `makeDefaultLocalBackend()` derives host+key from env/window globals; returns null when locked to Cloud so onboarding can collect credentials | `src/api/backend-registry/default-backend.ts:53-70` |
| Typed client boundary | All `/api/*` calls must use `@openhands/typescript-client`; CI guard scans sources for raw axios/fetch/HttpClient usage with a 3-file allowlist | `src/api/no-direct-agent-server-calls.test.ts:32-79` |
| Client options assembly | `getAgentServerClientOptions(overrides)` resolves host/sessionApiKey/workingDir from overrides → active backend → env; throws typed `NoBackendAvailableError` | `src/api/agent-server-client-options.ts:6-13,52-69` |
| Streaming/event plumbing | WebSocket provider owns connection, REST-first history seed, streaming-delta batcher; exposes `connectionState`, `sendMessage`, `reconnect` via React context | `src/contexts/conversation-websocket-context.tsx:82-97`, `src/contexts/websocket-provider-wrapper.tsx:11-45` |
| Socket lifecycle cleanup | Unmount disables reconnect flag before close, clears pending reconnect timers, closes socket only if not already closed | `src/hooks/use-websocket.ts:164-186` |
| Error surfacing (default) | Query/mutation cache `onError` shows global toasts (deduped 3 s) and invalidates auth query on 401 — baked into `createAgentServerQueryClient` | `src/query-client-config.ts:31-81` |
| Global query-client singletons | Module-level `defaultQueryClient`/`activeQueryClient` with `getQueryClient/setQueryClient` and a Proxy `queryClient` that rebinds dynamically | `src/query-client-config.ts:83-117` |
| Import-time side effect | Registry snapshot computed at module load by reading localStorage/sessionStorage | `src/api/backend-registry/active-store.ts:116-120` |
| CLI embedding mode | `bin/agent-canvas.mjs` runs full stack (agent-server via uvx + automation + static frontend); flags `--public`, `--frontend-only`, `--backend-only`; env overrides documented | `bin/agent-canvas.mjs:54-56,76-120` |
| Launcher shutdown semantics | SIGINT/SIGTERM/SIGHUP handler signals each child process tree with SIGTERM then force-kills after 3 s | `scripts/dev-with-automation.mjs:1070-1097` |
| Embedder log tap | `setServiceLogListener(cb)` fires per line of every child stdout/stderr/exit; best-effort, listener errors swallowed | `scripts/dev-with-automation.mjs:617` |
| Electron host integration | Two-stage readiness wait (ingress up, then `/server_info` 200/401) before loading window; `before-quit` triggers SIGTERM-based tree kill with 6 s force-exit safety net | `electron/main.mjs:700-748` |
| Standalone app bootstrap | App hydrates inside `AgentServerUIProviders` with `withStyleRoot={false}` because the router layout renders its own scoped root | `src/entry.client.tsx:40-45` |
| Secrets/identity delegation | `SecretsService` lists/saves secrets via agent-server `SettingsClient` (or Cloud proxy) — no local secret store in this package | `src/api/secrets-service.ts:19-27` |
| Embedding tests | Provider contract tests: custom query client/i18n injection + restore, analytics disabled/runtime config, scoped root presence/removal | `__tests__/agent-server-ui-providers.test.tsx:97-279` |
| Public-surface tests | Entry-point export assertions for all domain barrels and removed symbols | `__tests__/library-entrypoints.test.ts:10-49` |
| Packaging tests | Entrypoint publishing, exact version pins, runtime logger deps for published CLI | `__tests__/package-library.test.ts:23-144` |
| Router coupling of exports | Exported `ChatPanel` imports `useNavigate` directly; `ConversationView` is a route component using `useNavigate/useLocation/useMatch`; `useConversationId` throws without a `conversationId` route param | `src/components/features/chat/chat-interface.tsx:2`, `src/routes/conversation.tsx:1-36`, `src/hooks/use-conversation-id.ts:9-19` |
| Partial navigation decoupling | `NavigationContext` exists so components can avoid react-router, but only some components migrated | `src/context/navigation-context.tsx:12-47` |

## Answers to Dimension Questions

**1. Can the harness run inside another application without owning the whole process?**
Partially. As a React component library, yes: `AgentServerUIProviders` mounts inside any host React tree, supports nested providers with save/restore of prior query client/i18n instances (`src/components/providers/agent-server-ui-providers.tsx:65-88`), and scopes all CSS under `[data-agent-server-ui]` so host styles are unaffected (`vite.config.ts:195-214`). However, it always requires an *external* agent-server process to talk to; the package itself never embeds the agent loop in-process. The CLI/Electron/Docker modes do own the whole process tree by design (`bin/agent-canvas.mjs:1-12`, `electron/main.mjs:700-748`). Within the library, several singletons (query client proxy `src/query-client-config.ts:107-117`, active i18n `src/i18n/index.ts:74-88`, backend registry snapshot `src/api/backend-registry/active-store.ts:116-120`) mean two independent Canvas instances in one page would share registry/storage state.

**2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?**
Mixed, and split along the frontend/backend boundary:
- *Telemetry*: yes, fully — host passes PostHog key/hosts via `analytics` or hard-disables with `false` (`src/components/providers/agent-server-ui-providers.tsx:90-99`; `src/services/telemetry.ts:205-231`), and the named PostHog instance avoids clobbering a host's own PostHog (`src/services/telemetry.ts:344-362`).
- *Identity (backend connection)*: partially — hosts register backends programmatically (`addBackend/updateBackend/removeBackend`, `src/contexts/active-backend-context.tsx:124-204`) but credentials land in a localStorage-persisted global registry (`src/api/backend-registry/storage.ts:13-14,98`).
- *Query caching / i18n / theming*: yes — injected props and `--oh-*` CSS variables (`src/lib/index.ts:16-41`, `src/styles/agent-server-ui-style-scope.ts:17-95`).
- *Policy / tools / approvals*: no in-process injection points found. Tool sets are chosen per-conversation in request payloads (`VITE_ENABLE_BROWSER_TOOLS`, `getAgentTools()` gating described in repo notes), execution happens server-side, and a search for approval flows across `src/` returned only a mock-data match (`src/mocks/automations.mock.ts`). No evidence found of host-callable policy or human-approval callback contracts in this source.
- *Secrets & settings storage*: delegated out-of-process to the agent-server's `SecretsService`/settings APIs (`src/api/secrets-service.ts:19-27`); the host cannot supply an alternate store through the library surface.
- *Telemetry consent*: host builds its own consent UI on the exported `useTelemetry`/`setTelemetryConsent` API (`src/lib/index.ts:44-56`).

**3. Are lifecycle, cancellation, shutdown, and error propagation explicit?**
Mostly yes, with gaps. Socket teardown is careful: unmount disables reconnection before closing, clears timers, and guards against double-close (`src/hooks/use-websocket.ts:164-186`); connection state and manual `reconnect()` are exposed contextually (`src/contexts/conversation-websocket-context.tsx:92-97`). Process-level shutdown is explicit and tested in both launcher (`scripts/dev-with-automation.mjs:1070-1097`) and Electron (`electron/main.mjs:726-748`, including a Windows-specific in-process SIGTERM emit and a 6 s force-exit). Gaps: there is no AbortController/cancellation plumbing anywhere in the API layer (grep for `AbortController|abortSignal|signal:` across `src/api` and `src/hooks/query` returned nothing), so request cancellation relies entirely on React Query's unmount semantics; error propagation is coupled to toast display inside the default query-cache handlers (`src/query-client-config.ts:41-77`) rather than an opt-in error callback contract.

**4. Does the integration model work for both local-first and service deployments?**
Yes — this is arguably the strongest dimension of the design. The same frontend targets: local laptop stacks via npm/uvx (`README.md:63-80`), Docker sandboxes with `PROJECTS_PATH` mounts (`README.md:82-104`), remote VMs/cloud backends selectable from one UI through the multi-backend registry (`src/contexts/active-backend-context.tsx:31-38`; README architecture section `README.md:124-137`), locked-cloud deployments where local seeding is deliberately suppressed (`src/api/backend-registry/default-backend.ts:53-70`), and public-mode deployments where the session key is withheld and users authenticate through a 401-driven entry screen (`bin/agent-canvas.mjs:68-74`; gate referenced in `src/root.tsx:20-28`). Build-time env vars plus runtime window-global injection (`src/api/agent-server-config.ts:102-132`) let the same artifact serve dev, npm-binary, and Docker paths.

## Architectural Decisions

1. **UI-first embedding with out-of-process agent ownership.** The package embeds presentation and orchestration UX; the agent loop, tools, and persistence live in a separate agent-server process reached via the typed client. The CI-enforced ban on ad-hoc HTTP keeps that boundary non-erosible (`src/api/no-direct-agent-server-calls.test.ts:32-79`).
2. **Scoped-CSS-as-a-contract.** Instead of shadow DOM or CSS modules, every generated selector is prefixed at build time with `[data-agent-server-ui]`, with special-case remapping of `:root/html/body` onto the scope element (`src/styles/agent-server-ui-style-scope.ts:97-117`). Theming is then pure CSS custom properties overridable via props or host CSS.
3. **Dual config channels: build-time env + runtime window globals.** `getBakedSessionApiKey()` checks `VITE_SESSION_API_KEY` then `window.__AGENT_CANVAS_SESSION_API_KEY__`, explicitly documented as the mechanism that makes the prebuilt npm binary work without rebuilding (`src/api/agent-server-config.ts:102-132`).
4. **Named telemetry instance + explicit disable.** Rather than piggybacking on a host's PostHog default singleton, Canvas initializes its own named instance with separate persistence keys, and `configureTelemetry(false)` is the documented hard-off for embedders (`src/services/telemetry.ts:53,344-362,205-211`).
5. **Registry-backed multi-backend selection.** A small external store (subscribe/getSnapshot pattern consumed via `useSyncExternalStore`, `src/contexts/active-backend-context.tsx:76-80`) holds all configured backends, persisted to localStorage/sessionStorage, with health-aware fallback picking (`src/api/backend-registry/active-store.ts:52-62`).

## Notable Patterns

- **Save/restore nested providers**: `AgentServerUIProviders` remembers the ambient query client/i18n before overriding and restores both on unmount — a rare but correct pattern for libraries that also expose imperative module-level accessors (`src/components/providers/agent-server-ui-providers.tsx:65-88`).
- **Dynamic proxy facade**: the exported `queryClient` is a Proxy that binds every property access to the currently active client, keeping imperative call sites compatible with provider swaps (`src/query-client-config.ts:107-117`).
- **Two-stage readiness probing in the Electron host**: ingress reachability is insufficient (uvx cold start can take minutes), so the host additionally polls `/server_info` accepting 200 *or* 401 before mounting the window (`electron/main.mjs:219-248` region and shutdown flow `700-748`).
- **Embedder-facing log tap**: `setServiceLogListener` gives the desktop host per-line child-process logs while deliberately swallowing listener exceptions so a buggy embedder cannot crash the stack (`scripts/dev-with-automation.mjs:617`).
- **Contract-by-test**: the embedding surface is pinned by unit tests (provider DI, entrypoint exports, packaging metadata) and browser-level CSS-isolation E2E specs (`__tests__/agent-server-ui-providers.test.tsx:214-278`, `__tests__/library-entrypoints.test.ts:10-49`).

## Tradeoffs

- **UX ownership vs. data ownership**: hosts get full control of rendering and theming, but zero in-process control over policy, tools, secrets, and durable state — those are structurally bound to whichever agent-server the registry points at. A product needing custom approval gates or its own credential vault must fork the service layer or intercept at the server.
- **Singleton convenience vs. multi-instance safety**: module-level query-client/i18n/registry singletons simplify internal call sites (no prop drilling, Proxy rebinding) but make two simultaneously mounted Canvases in one document share registries, consent state, and toast dedup sets (`shownErrors` at `src/query-client-config.ts:29`).
- **Router coupling vs. framework neutrality**: exporting route-shaped components (`ConversationView`) maximizes reuse inside the standalone app but obligates embedders to install React Router with matching path shapes; the `NavigationContext` abstraction (`src/context/navigation-context.tsx:12-47`) shows intent to remove this coupling, yet five component files still call react-router hooks directly.
- **Global toasts vs. host-controlled errors**: baking `displayErrorToast` into the shared query cache guarantees consistent UX standalone but leaks UI side effects into what could have been a neutral data layer; the escape hatch is supplying a custom QueryClient, which then forfeits the built-in 401 handling unless reimplemented.
- **Build-time env + window globals vs. clean DI**: window-global injection (`window.__AGENT_CANVAS_SESSION_API_KEY__`) enables zero-rebuild deployment flexibility at the cost of implicit, stringly-typed coupling between launcher scripts and the bundle.

## Failure Modes / Edge Cases

- **No-backend bootstrap trap**: if neither env nor injected window globals yield host+key, `makeDefaultLocalBackend()` returns null, the registry seeds empty, and the app traps users behind the Manage Backends modal — the code comments describe exactly this failure and the injection fix (`src/api/agent-server-config.ts:110-118`).
- **Stale sandbox URLs**: cloud conversations paused mid-session would otherwise point the WebSocket at a dead host; the wrapper suppresses `conversationUrl` while `sandbox_status === "PAUSED"` until fast polling observes resume (`src/contexts/websocket-provider-wrapper.tsx:24-33`).
- **Late-close races**: replaced WebSocket instances must not clobber replacement state or trigger reconnection; handled with a WeakSet allowlist and late-close filtering (`src/hooks/use-websocket.ts:30-33,100-114,143-147`).
- **Orphaned process groups**: detached children survive parent death by default, so both launcher and Electron must signal whole process trees (SIGTERM → SIGKILL escalation) or subsequent launches fail on occupied ports — the Electron comment documents orphaned ports 8000/18000/18001 as the failure if Windows cleanup is skipped (`scripts/dev-with-automation.mjs:1078-1092`; `electron/main.mjs:700-722`).
- **Telemetry privacy edges**: install tracking fires pre-consent by design (documented tradeoff, `src/services/telemetry.ts:9-13,666-708`), mitigated only by DO_NOT_TRACK signals (`276-305`); `clearTelemetryData` must fall back to forced opt-out if PostHog reset throws, so a privacy clear can never leave capture enabled (`860-873`).
- **Embedder breakage containment**: telemetry initialization failures are swallowed and retried lazily; PostHog load failures disable telemetry rather than the app (`src/services/telemetry.ts:254-270`).

## Future Considerations

- Finish the `NavigationContext` migration so exported panels work without React Router, unlocking non-router hosts (framework-neutral embedding for Svelte/Vue/Angular shells via web components or isolated React islands).
- Introduce an opt-in error/progress callback contract on the provider (e.g., `onError`, `onEvent`) so hosts can replace global toasts and build their own observability without reimplementing query-cache internals.
- Expose a headless conversation-control facade over the existing stores/WebSocket context for hosts that want the events stream without the chat UI; today the only sanctioned programmatic path is the external `@openhands/typescript-client`.
- Provide a storage-port interface for the backend registry (localStorage today) to support SSR, multi-instance pages, and host-managed persistence.
- Document the library embedding path beyond the four paragraphs in `docs/DEVELOPMENT.md:151-177` — including required router setup, the window-global config contract, and telemetry consent expectations for embedders.

## Questions / Gaps

- **Approval/policy surfacing**: No evidence found of human-approval or policy-hook contracts in this source. Searches covered `approval|Approval` across `src/**` (only `src/mocks/automations.mock.ts` matched) and review of the WebSocket event type guards (`src/contexts/conversation-websocket-context.tsx:23-50`). Such logic presumably lives in the sibling `software-agent-sdk` agent-server, which was out of scope under source-isolation rules.
- **Host-supplied storage/tools**: No evidence found of interfaces allowing a host to swap the settings/secrets store or contribute custom tools in-process; all such extension appears to happen server-side (tool sets are named in conversation-start payloads, e.g., `terminal`, `file_editor`, `browser_tool_set`, per repo notes and `src/api/agent-server-adapter.ts`).
- **Multi-instance behavior**: I did not find (nor exhaustively search for) tests covering two simultaneous Canvas instances in one document; the analysis of shared-singleton risk is inferred from module structure (`src/query-client-config.ts:83-105`, `src/api/backend-registry/active-store.ts:116-122`), not demonstrated by a test.
- **SSR behavior of the library**: `waitForI18n` and lazy PostHog import suggest SSR tolerance (`src/i18n/index.ts:89`, `src/services/telemetry.ts:250-270`), but no SSR example or test exists in-tree; the only shipped consumer examples are the standalone app and the Electron shell.

---

Generated by `24.04-embedding-and-host-integration-ergonomics` against `openhands`.
