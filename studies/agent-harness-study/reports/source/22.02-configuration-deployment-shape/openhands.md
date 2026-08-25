# Source Analysis: openhands

## Dimension 22.02 — Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (React Router + Vite), Node.js launcher scripts, Bash (Docker entrypoint), Helm/Kubernetes manifests, Electron |
| Analyzed | 2026-08-25 |

## Summary

The repository (OpenHands "Agent Canvas" frontend plus its launch/orchestration tooling) implements an unusually deliberate configuration and deployment model for a frontend-centric project. A single JSON file, `config/defaults.json`, is the declared "single source of truth" for version pins, ports, persistence paths, and telemetry keys (`config/defaults.json:1-2`), and it is consumed by four independent artifact classes: JS launcher scripts (`scripts/dev-safe.mjs`, `scripts/docker-build.mjs:22-24`), the frontend bundle via import (`src/api/agent-server-compatibility.ts:13`, `src/services/telemetry.ts:31`), a Docker build stage that compiles it to shell-sourceable `defaults.env` (`docker/Dockerfile:49-70`), and CI workflows that extract values with `node -p` (`.github/workflows/docker.yml:86-94`).

Configuration is layered across three planes: (1) build-time Vite env vars (`VITE_*`, documented in `.env.sample:6-27`), (2) runtime injection into served HTML by the static server via `window.__AGENT_CANVAS_*__` globals (`scripts/static-server.mjs:325-374`), and (3) user-editable backend registry state in localStorage with validation and seeding (`src/api/backend-registry/storage.ts:104-149`). Deployment shape is genuinely multi-modal and documented: local dev stack launchers, a published npm CLI binary (`bin/agent-canvas.mjs`), an all-in-one Docker image (`docker/Dockerfile`), a Helm chart for Kubernetes (`helm/agent-canvas/values.yaml`), an Electron desktop app (`electron/main.mjs`), an embeddable React library with runtime props (`src/components/providers/agent-server-ui-providers.tsx:37-46`), and a self-hosting VM guide (`docs/SELF_HOSTING.md`). Validation exists at every layer boundary but is hand-rolled (type guards, fail-fast checks, collision guards) rather than schema-driven; feature flags exist only as a vestigial typed interface with hardcoded values.

## Rating

**8 / 10.**

Rationale against the rubric: this is a clear configuration model with tests, explicit interfaces, and operational safeguards — squarely the 7–8 band, near the top of it.

What earns the score:
- One canonical config file consumed by scripts, app code, Docker build, and CI, with drift guards (`scripts/check-sdk-version-sync.mjs:14-18` verifies released PyPI automation dependencies still match `versions.agentServer`; `__tests__/scripts/docker-vscode-route-sync.test.ts:9-12` extracts and tests the real entrypoint config block so Docker and npm wiring cannot silently diverge).
- Secure-by-default operational behavior: the Docker entrypoint auto-generates and persists the session API key and `OH_SECRET_KEY` with `chmod 600` instead of shipping known defaults (`docker/entrypoint.sh:179-209`), and `--public` mode refuses to start without `LOCAL_BACKEND_API_KEY` (`scripts/dev-with-automation.mjs:412-420`).
- Frontend enforces a backend compatibility floor at bootstrap with typed errors (`src/api/agent-server-compatibility.ts:293-318`) so mismatched stacks fail loudly within a 5-second probe window (`src/api/agent-server-compatibility.ts:15`).
- Launcher-level fail-fast validation for paths, ports, flag combinations, and route collisions (`scripts/dev-safe.mjs:424-436`, `bin/agent-canvas.mjs:125-135`, `scripts/static-server.mjs:181-203`, `docker/entrypoint.sh:100-158`).

What keeps it from 9–10:
- Feature flags are a hardcoded stub (`src/api/option-service/option-service.api.ts:33-37`), not a real system; no remote-flag integration exists.
- The same logical setting is addressable through several alias chains (e.g., VS Code port resolution falls back through `OH_VSCODE_PORT` → `VSCODE_PORT` → `CONFIG_VSCODE_PORT` → literal default, `docker/entrypoint.sh:94-95`), which is documented but adds cognitive load.
- Some config remains build-time-baked (`VITE_POSTHOG_API_KEY`, `VITE_BASE_PATH`, `docker/Dockerfile:43-47`), so "same artifact for all environments" holds only if builds are parameterized correctly; runtime injection covers auth/session/mode but not these.
- The backend registry stores API keys in localStorage, a tradeoff explicitly acknowledged in the docs (`docs/SELF_HOSTING.md:95-104`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Centralized defaults file | `_comment` declares single source of truth; versions, compatibility floor, ports, paths, packages, telemetry key | `config/defaults.json:1-45` |
| Script-side consumption | docker-build helper reads defaults.json to derive `--build-arg`s | `scripts/docker-build.mjs:22-24,50-55` |
| App-side consumption | Compatibility floor imported from defaults.json; telemetry default key from defaults.json | `src/api/agent-server-compatibility.ts:13,18-19`; `src/services/telemetry.ts:31,58-60` |
| Docker-side consumption | `config-gen` stage converts JSON → `defaults.env`; entrypoint sources it | `docker/Dockerfile:51-70,141-142`; `docker/entrypoint.sh:62-71` |
| CI-side consumption | Workflow extracts versions/ports from defaults.json via `node -p` | `.github/workflows/docker.yml:86-94` |
| Build-time env layer | `.env.sample` documents all supported `VITE_*` vars incl. backend host/base URL, session key, working dir, base path | `.env.sample:6-19` |
| Build-time env defaults | Vite `loadEnv` with defaults for backend host/TLS/port/insecure-skip-verify | `vite.config.ts:91-117` |
| Runtime injection layer | Static server injects `window.__AGENT_CANVAS_SESSION_API_KEY__`, `__AUTH_REQUIRED__`, `__RUNTIME_SERVICES_INFO__`, `__LOCK_TO_CLOUD__`, `__BASE_PATH__`, `__VSCODE_BASE_PATH__` into served HTML | `scripts/static-server.mjs:282-374` |
| Env→window-global fallback | `getBakedSessionApiKey()` reads `VITE_SESSION_API_KEY` then window global; `getLockedCloudHost()` and `isAuthRequired()` follow the same two-source pattern | `src/api/agent-server-config.ts:119-132,141-155,214-221` |
| Backend registry layer | localStorage-backed registry with per-field type validation, first-run seeding, empty-registry re-seed | `src/api/backend-registry/storage.ts:13-14,24-40,104-149` |
| Registry key rotation sync | Stored seeded-local backend apiKey overwritten from launcher value on load | `src/api/backend-registry/storage.ts:70-93` |
| Client option assembly | Overrides > active registry backend > env-derived working dir; throws `NoBackendAvailableError` when unresolvable | `src/api/agent-server-client-options.ts:52-69` |
| Backend version selection precedence | `OH_AGENT_SERVER_LOCAL_PATH` > `OH_AGENT_SERVER_GIT_REF` > `OH_AGENT_SERVER_VERSION` > pinned default | `scripts/dev-safe.mjs:407-426`; `.env.sample:34-37` |
| Environment promotion (staging→prod telemetry) | Prod PostHog key only for `v*` tags; staging key for PR/main images | `.github/workflows/docker.yml:174-181,203` |
| npm publish prod key | `POSTHOG_PROD_KEY` baked into packaged app and library builds | `.github/workflows/npm-publish.yml:64-72` |
| Telemetry opt-out | `VITE_DO_NOT_TRACK=1` disables telemetry; propagated to agent-server as `DO_NOT_TRACK` in Docker | `src/services/telemetry.ts:24-26,284`; `docker/entrypoint.sh:238-240` |
| Dev environment modes | `dev`, `dev:static`, `dev:minimal`, `dev:frontend`, `dev:mock` script matrix | `package.json:75-80` |
| Deployment mode: npm CLI binary | `npx @openhands/agent-canvas` runs full stack (uvx agent-server + automation + prebuilt static frontend); `--public`, `--frontend-only`, `--backend-only`, `--info` flags | `bin/agent-canvas.mjs:54-135` |
| Deployment mode: Docker all-in-one | Single image bundles agent-server + automation + static frontend behind port 8000 ingress with path routing | `docker/Dockerfile:1-16,166-170`; `docker/entrypoint.sh:384-403` |
| Deployment mode: Helm/K8s | StatefulSet chart, single replica, PVC subPath mounts, `/alive` probes, RBAC knobs, K8s Secret seeding for keys, `extraEnv` passthrough | `helm/agent-canvas/values.yaml:108-128,131-163,199-221` |
| Deployment mode: Electron desktop | Same stack inside BrowserWindow; bundled Node/uv; two-stage readiness wait for cold uvx starts | `electron/main.mjs:170-171,240-249`; `package.json:108-111` |
| Deployment mode: embeddable library | `AgentServerUIProvidersProps` accepts runtime analytics/i18n/theme/styleOverrides config | `src/components/providers/agent-server-ui-providers.tsx:37-46,90-99` |
| Runtime deployment-mode advertisement | `/server_info.runtime_services.mode` ("docker", "dev:automation", …) built by shared module used by both dev launchers and Docker CLI | `scripts/runtime-services-info.mjs:60-149,151-186`; `src/api/agent-server-adapter.ts:202-206` |
| Feature flag interface | Typed `WebClientFeatureFlags { hide_llm_settings, hide_users_page }` on `WebClientConfig` | `src/api/option-service/option.types.ts:20-26` |
| Feature flags are hardcoded | `getConfig()` returns static `hide_llm_settings: false, hide_users_page: true` | `src/api/option-service/option-service.api.ts:30-45` |
| Flag consumers | Settings nav filtering and sidebar gating read `config.feature_flags` | `src/utils/settings-utils.ts:52-78`; `src/hooks/use-settings-nav-items.ts:18-21`; `src/components/features/sidebar/sidebar.tsx:126,135` |
| Config validation: registry | `isValidBackend`/`isValidKind`/`isValidAuthMode` guards filter corrupt stored state | `src/api/backend-registry/storage.ts:16-40` |
| Config validation: compatibility floor | `assertAgentServerVersionIsSupported()` throws typed unsupported/unknown-version errors; semver compare incl. prerelease handling | `src/api/agent-server-compatibility.ts:243-271,293-318` |
| Config validation: public-mode key required | Launcher exits unless `LOCAL_BACKEND_API_KEY` set in `--public` mode | `scripts/dev-with-automation.mjs:412-420` |
| Config validation: mutual exclusion | `--session-api-key` and `--auth-required` rejected together; vscode prefix requires matching route | `scripts/static-server.mjs:181-203` |
| Config validation: entrypoint guards | Base-path normalization, single-segment restriction, reserved-prefix collision rejection, numeric port check | `docker/entrypoint.sh:100-158` |
| Cross-artifact consistency tests | Tests execute the extracted Docker entrypoint config block; launcher/static-server arg parsing covered by dedicated suites | `__tests__/scripts/docker-vscode-route-sync.test.ts:53-92,117+`; `__tests__/scripts/static-server.test.ts:109-173` |
| Drift detection in CI | SDK version sync check fails when released automation deps ≠ `versions.agentServer` | `scripts/check-sdk-version-sync.mjs:14-18,144-157,341-408` |
| No schema-validation library | No zod/joi/yup in dependencies; ajv appears only as a scoped security override | `package.json:188` |

## Answers to Dimension Questions

**1. Is configuration layered?**
Yes, deliberately and across four layers. Layer order for a given setting is explicit: build-time `VITE_*` env var first, then a runtime-injected `window.__AGENT_CANVAS_*__` global, then user-managed registry/localStorage state, then the centralized `config/defaults.json`. The session API key demonstrates the pattern end-to-end: `getBakedSessionApiKey()` prefers `VITE_SESSION_API_KEY` then the injected window global (`src/api/agent-server-config.ts:119-132`), the static server injects that global and also overwrites the legacy localStorage copy at serve time (`scripts/static-server.mjs:335-353`), and the resulting value seeds the default local backend in the registry (`src/api/backend-registry/default-backend.ts:53-69`). On the server-launcher side, agent-server provenance has a documented precedence chain (local checkout > git ref > pinned PyPI version, `scripts/dev-safe.mjs:407-426`), and Docker settings resolve through env var > generated `CONFIG_*` default > hard fallback (`docker/entrypoint.sh:62-71`).

**2. Are environments managed cleanly?**
Mostly yes, via a "same artifact, different parameters" model rather than per-environment configs. There are no dev/staging/prod config files; instead, environment identity is carried by build args and runtime env. The clearest example is telemetry: the staging PostHog key is the committed default (`config/defaults.json:42`, `src/services/telemetry.ts:56-60`), production is injected only by release workflows (`npm-publish.yml:64-72`) or tagged Docker builds (`docker.yml:174-181`), and any deployment can disable tracking entirely with `VITE_DO_NOT_TRACK=1` (`telemetry.ts:284`; mirrored to `DO_NOT_TRACK` for the agent-server at `docker/entrypoint.sh:238-240`). Backend stack versions are pinned centrally and locally overridable per developer via `OH_AGENT_SERVER_LOCAL_PATH`/`GIT_REF`/`VERSION` without touching committed config (`scripts/dev-safe.mjs:407-426`). The residual weakness: `VITE_BASE_PATH` and the PostHog key are compile-time constants of the produced bundle (`vite.config.ts:97,117`; `docker/Dockerfile:43-47`), so environment parity depends on building with correct args rather than reconfiguring one artifact — mitigated, not eliminated, by the runtime window-global mechanism for the other settings.

**3. Are deployment modes documented?**
Yes — unusually well, in executable form. Five distinct modes ship with first-class support: the npm CLI binary (`bin/agent-canvas.mjs:60-121` includes full usage/auth-mode docs and `--info` printing stack versions and ports), the all-in-one Docker image whose header comment documents the routing table (`docker/Dockerfile:3-16`) with volume/persistence guidance (`docker/Dockerfile:159-164`), a Helm chart with inline rationale for replica count, persistence layout, RBAC scope, and probe tuning (`helm/agent-canvas/values.yaml:5-9,90-128,199-235`), the Electron desktop packaging (`package.json:108-111`), and the embeddable library surface (`src/components/providers/agent-server-ui-providers.tsx`). `docs/SELF_HOSTING.md:23-59` documents the VM topology as a mermaid diagram including the public/private auth-mode distinction. Additionally, each running instance *self-reports* its deployment mode at runtime through `/server_info.runtime_services.mode` ("docker", "dev:automation", …), assembled by one shared module used by both dev launchers and the Docker entrypoint (`scripts/runtime-services-info.mjs:10-22`; consumed at `src/api/agent-server-adapter.ts:202-206`), which the frontend even uses to conditionally patch marketplace catalog entries per environment (Docker-only native `github-mcp-server` transport).

**4. Are feature flags supported?**
Only nominally. The typed interface exists (`WebClientFeatureFlags`, `src/api/option-service/option.types.ts:20-23`) and consumers correctly gate UI on it (`settings-utils.ts:52-58`, `use-settings-nav-items.ts:18-21`, `sidebar.tsx:126`), but the sole producer returns hardcoded literals — `hide_llm_settings: false`, `hide_users_page: true` (`option-service.api.ts:33-37`). There is no remote flag service: a repo-wide search found no use of PostHog's `getFeatureFlag`/`isFeatureEnabled` despite PostHog being integrated for analytics, and no other flag store. The interface is a vestige of the hosted OpenHands product kept for shape compatibility. De facto feature gating in this repo happens through env vars (`VITE_ENABLE_BROWSER_TOOLS`, `.env.sample:13`), server-advertised capabilities (`usable_tools` gating at `src/api/agent-server-compatibility.ts:149-155`), and backend-schema-driven settings — capability-gating patterns rather than flags.

**5. Is configuration validated?**
Yes, extensively but hand-rolled — there is no schema-validation library (no zod/joi/yup in `package.json`; ajv appears only as a transitive security override). Validation occurs at five boundaries: (a) registry reads validate every stored backend field with type guards and drop/re-seed invalid state (`storage.ts:16-40,130-142`); (b) the frontend bootstrap enforces a minimum agent-server version parsed from defaults.json, throwing typed `AgentServerUnsupportedVersionError`/`UnknownVersionError` that drive recovery UI (`agent-server-compatibility.ts:293-318`), and validates stored session keys against a protected endpoint when auth is required (`agent-server-compatibility.ts:372-395`); (c) launchers fail fast on bad input — public mode without a key (`dev-with-automation.mjs:412-420`), non-absolute/nonexistent local SDK paths (`dev-safe.mjs:424-436`), mutually exclusive CLI flags (`bin/agent-canvas.mjs:125-135`, `static-server.mjs:181-189`); (d) the Docker entrypoint validates route-prefix collisions and malformed values before starting services (`entrypoint.sh:100-158`); and (e) CI runs cross-artifact consistency checks: a test suite executes the actual extracted entrypoint config block so Docker routing can't drift from tests (`docker-vscode-route-sync.test.ts:53-92`), and the SDK version-sync check fails the build when published PyPI dependency pins diverge from `versions.agentServer` (`check-sdk-version-sync.mjs:387-408`).

## Architectural Decisions

1. **One JSON file as cross-stack config contract.** Rather than duplicating ports/versions across Makefiles, workflows, and app code, everything derives from `config/defaults.json` (`config/defaults.json:1-2`). The Docker `config-gen` stage pre-compiles it to `defaults.env` specifically to avoid requiring jq/python at container runtime (`docker/Dockerfile:49-50`) — an explicit tradeoff of build complexity for runtime simplicity.

2. **Build-time env with runtime-injection escape hatch.** Because Vite bakes `import.meta.env.*` into the bundle, the team added a second channel: the serving layer injects `window.__AGENT_CANVAS_*__` globals into `index.html` at request time (`scripts/static-server.mjs:282-324`). This is what lets one pre-built bundle serve both auto-authenticated local mode and paste-a-key public mode (`isAuthRequired()` dual source, `src/api/agent-server-config.ts:214-221`).

3. **User-owned backend registry instead of a fixed backend.** Connection targets are records in a validated localStorage registry seeded from launcher config, not singleton globals (`src/api/backend-registry/types.ts:4-13`; seeding at `default-backend.ts:53-69`). All API clients resolve host/key through one assembly point (`getAgentServerClientOptions`, `src/api/agent-server-client-options.ts:52-69`), so multi-backend support didn't fork the config model.

4. **Secure-by-default secrets lifecycle.** Both Docker and npm launchers auto-generate session keys and the encryption key, persist them under `~/.openhands/agent-canvas/` with `chmod 600`, and reuse them across restarts (`docker/entrypoint.sh:179-209`; `dev-with-automation.mjs:450-464` via `buildSafeDevConfig`). The image "never runs with a known default" (`entrypoint.sh:175-178`).

5. **Fail-fast launcher philosophy with tested extraction.** Configuration errors terminate startup with actionable messages rather than degrading at request time (port conflicts checked up front, `dev-with-automation.mjs:433-446`; vscode route collision exits 1, `entrypoint.sh:144-149`). The riskiest config block is literally extracted into a test file via markers so tests exercise production code (`entrypoint.sh:87-88`; `docker-vscode-route-sync.test.ts:53-64`).

## Notable Patterns

- **Config alias collapse at a single choke point.** Multiple names for one setting (`OH_VSCODE_PORT`/`VSCODE_PORT`/`CONFIG_VSCODE_PORT`) are resolved once, before anything reads them, precisely so the advertised URL and proxy route cannot diverge (`docker/entrypoint.sh:73-106,160-164`).
- **Self-describing deployments.** The `runtime_services` metadata appended to `/server_info` gives both the frontend and the agent itself authoritative topology information, replacing port-probing heuristics (`scripts/runtime-services-info.mjs:1-22`; rendered into the agent's system prompt at `src/api/agent-server-adapter.ts:215-269`).
- **Key rotation resilience.** At module init, stored seeded-local backends have their apiKey overwritten from current launcher config when the host matches a loopback equivalence check (`syncLauncherDefaultLocalBackend`, `src/api/backend-registry/storage.ts:56-93`) — stale-key recovery without user action.
- **Environment-tagged analytics.** Every telemetry event gets immutable `client_source`/`client_version` attribution injected in `before_send` so reset cannot strip it (`src/services/telemetry.ts:110-115`), letting ops distinguish npm vs Docker vs library traffic.
- **CI as config linter.** Beyond unit tests, CI verifies that the released automation package's SDK dependency pins match the repo's declared `versions.agentServer`, catching cross-repo drift (`scripts/check-sdk-version-sync.mjs:14-18`).

## Tradeoffs

- **Compile-time baking vs artifact uniformity.** `VITE_BASE_PATH` and `VITE_POSTHOG_API_KEY` differ between builds (`docker/Dockerfile:43-47`), so the "same bytes in every environment" ideal is sacrificed for these two values; everything else moves to runtime injection at the cost of a second config channel (`window.__AGENT_CANVAS_*__`) that developers must learn.
- **localStorage as a config store.** User-visible convenience (multi-backend, no re-entry) trades against security: API keys sit in localStorage, and the docs explicitly warn that the path-prefixed editor shares origin storage with the canvas, exposing every registered backend's key to anything on that origin (`docs/SELF_HOSTING.md:95-104`).
- **Deep alias chains vs operator flexibility.** Supporting `OH_*`, bare, and `CONFIG_*` spellings maximizes compatibility with existing deployments but means a typo can silently fall through to a default; the entrypoint mitigates with normalization and collision rejection rather than eliminating aliases (`docker/entrypoint.sh:93-95,100-158`).
- **Hand-rolled validation vs schema libraries.** Zero-dependency guards keep bundles lean and work identically in browser, Node scripts, and bash, but lack declarative schemas; correctness rests on the guard functions and their tests rather than a single auditable spec.

## Failure Modes / Edge Cases

- **Stale baked key after server restart:** In public mode a rotated `LOCAL_BACKEND_API_KEY` surfaces as HTTP 401 during the bootstrap probe and triggers the key-entry screen (`agent-server-compatibility.ts:123-133,355-360`); the code comments acknowledge a narrow race where a mid-boot connection drop loads the app with an unvalidated key until refresh (`agent-server-compatibility.ts:381-385`).
- **Corrupt/empty registry:** Malformed JSON or arrays failing validation fall back to re-seeding from launcher config, and total absence yields an explicit `noBackendConfigured` error driving the manage-backends modal rather than a broken home page (`storage.ts:130-148`; `agent-server-compatibility.ts:320-340`).
- **Route-prefix takeover:** Because static-server routes match longest-prefix and register the editor route last, a misconfigured `VSCODE_BASE_PATH=/api` would hijack all API calls; the entrypoint rejects reserved/colliding/non-single-segment prefixes at startup (`entrypoint.sh:122-149`), and the site-root case is fatal too (`entrypoint.sh:107-110`).
- **Locked-to-cloud seeding trap:** When `--lock-to-cloud` is set, seeding a local backend from an injected session key would strand users on the recovery modal, so `makeDefaultLocalBackend()` deliberately returns null in locked mode (`default-backend.ts:38-56`), and locked deployments force-overwrite the registry (`storage.ts:108-116`).
- **Backend crash tolerance:** The Docker supervisor intentionally tolerates agent-server/automation crashes (proxy answers 502) and only exits the container when the ingress dies, matching non-Docker semantics where services are independent processes (`entrypoint.sh:458-471`).

## Future Considerations

- Replace the hardcoded `WebClientConfig` stub with a real flag source (backend-driven or PostHog flags) or delete the interface; today it invites dead branches like `hide_users_page: true` hiding a page that may not exist (`option-service.api.ts:30-45`).
- Move remaining bake-time values (base path, telemetry key) onto the existing window-global injection channel to achieve true single-artifact parity, since the mechanism already exists (`scripts/static-server.mjs:314-317` already injects base path for asset resolution).
- Consider consolidating the `OH_*`/bare/`CONFIG_*` alias families into one documented naming scheme with a deprecation path; each addition multiplies the collapse logic at `docker/entrypoint.sh:93-95`.
- The localStorage key-storage model has a known, tracked exposure via the shared-origin editor (#16492, referenced at `docs/SELF_HOSTING.md:100-104`); migrating secrets to the agent-server's encrypted secret store for all backend kinds would close it.

## Questions / Gaps

- **No evidence found** for any remote/dynamic feature-flag evaluation: searches across `src/` for `getFeatureFlag`, `isFeatureEnabled`, and PostHog flag APIs returned nothing; the only flag surface is the hardcoded `WebClientFeatureFlags`.
- **No evidence found** for a staging-vs-production *backend* environment split (separate agent-server clusters, config promotion pipelines): environment differentiation in-repo is limited to telemetry keys, base paths, and version overrides. If such environments exist, they live outside this repository (likely the Cloud backend), which was out of scope under source-isolation rules.
- The Helm chart's relationship to release automation is thin: `bump-chart.yml` exists in `.github/workflows/`, but I did not audit how chart `appVersion` tracks `config/defaults.json:versions.agentCanvas`; the values file comments imply manual pinning discipline (`helm/agent-canvas/values.yaml:14-17`).
- `vercel.json` contains only an install-command override pointing at `scripts/vercel-install.sh` (`vercel.json:1-4`), suggesting Vercel is a secondary/experimental target; no environment documentation for it was found beyond AGENTS.md notes about React Router/Vercel preset pitfalls.

---

Generated by `dimensions/22.02-configuration-deployment-shape.md` against `openhands`.
