# Source Analysis: langfuse

## Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript (Next.js + Express/BullMQ + Prisma + ClickHouse), pnpm/turbo monorepo |
| Analyzed | 2026-08-22 |

## Summary

Langfuse is a pnpm/turbo monorepo organized around four runtime packages and three dev packages, with a pinned dependency direction (`web` → `@langfuse/shared`/`@langfuse/ee`; `worker` → `@langfuse/shared`; `ee` → `@langfuse/shared`; `@langfuse/shared` → none of them). Boundary discipline is enforced at three layers: a written contract in the top-level `AGENTS.md` (`sources/langfuse/AGENTS.md:30-36`), a `package.json#exports` map that names explicit server/client subpaths (`sources/langfuse/packages/shared/package.json:17-114`, `sources/langfuse/ee/package.json:6-15`), and an eslint `no-restricted-imports` rule plus `dependency-cruiser` configuration that enforces structural rules as CI warnings (`sources/langfuse/web/eslint.config.mjs:11-37`, `sources/langfuse/web/.dependency-cruiser.js:18-144`). A `project-structure RFC` (LFE-14748) layer (`web/scripts/structure/`) instruments a 20-rule RFC for the web app with a committed `.structure-baseline.json` (2710 known violations, snapshot only shrinks). Modules can be used independently because of the explicit export surfaces and the sandbox package (`@repo/in-app-agent-sandbox-runtime`) is isolated as a Docker image rather than a JS dependency.

## Rating

**8/10 — Clear model with tests, explicit interfaces, and operational safeguards.**

Rationale:
- **+**: explicit, named export surfaces (`@langfuse/shared`, `@langfuse/shared/src/server`, `/src/db`, `/src/env`, `/encryption`, `/query`, `/monitors`, `/in-app-agent`, plus per-feature narrower subpaths) instead of one giant barrel; the AGENTS.md files are kept in lock-step with `package.json#exports` (`sources/langfuse/packages/shared/AGENTS.md:51-113`).
- **+**: monorepo topology and dependency direction are stated as a hard rule in the root AGENTS.md (`sources/langfuse/AGENTS.md:30-36`) and verified empirically (zero `@langfuse/*` imports in `packages/shared/src`).
- **+**: server/client split per feature is structurally enforced: each web feature has both `index.ts` (client-safe) and `server/index.ts` (server-only), and `rfc10-no-client-to-server` forbids client → server imports (`sources/langfuse/web/.dependency-cruiser.js:48-64`, `sources/langfuse/packages/shared/AGENTS.md:159-167`).
- **+**: a webpack/swc-precise eslint rule (`no-restricted-imports` on `^(\.\./)+(packages|ee|worker)/`) prevents relative deep-imports from `web` into other workspaces — that failure mode crashed production deploys (PR #15031, `sources/langfuse/web/eslint.config.mjs:23-36`).
- **+**: an explicit project-structure RFC (20 rules) is instrumented in `web/scripts/structure/`, snapshot-checked through `pnpm structure:stats`, and ratcheted via `.structure-baseline.json` (`sources/langfuse/web/scripts/structure/stats.mjs:231-253`, `sources/langfuse/web/scripts/structure/README.md:28-32`).
- **+**: knip is wired up at the root and the in-app-agent sandbox runtime is a Docker-deployed sibling package rather than a JS consumer of `@langfuse/shared` (`sources/langfuse/packages/in-app-agent-sandbox-runtime/package.json:20-31`, `sources/langfuse/worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:6,89`).
- **−**: 2710 baseline violations indicate the structural model is a migration target, not an achieved state. Top contributors: rule 8 (cross-feature imports not through surface, 1214), rule 12 (pages shim, 616), rule 1 (PascalCase naming convention, 196), rule 5 (kind folders, 102), rule 10 (client → server, 97). Many are organization/cosmetic but rule 8/10 are real coupling signals.
- **−**: dep-cruiser rules are all `severity: "warn"`; the project's own README notes the regex versions "may slightly undercount" (`sources/langfuse/web/.dependency-cruiser.js:3-10`). Real CI enforcement today is the eslint restricted-imports rule + `structure:stats` snapshot, not the cruise rules themselves.
- **−**: knip is configured but explicitly disabled for `files`/`exports`/`types` issues across all workspaces (`sources/langfuse/knip.jsonc:4-11`); unused-export tracking is therefore the file-level orphan kind only (rule 20), and symbol-level coverage is documented as TBD (`sources/langfuse/web/scripts/structure/stats.mjs:200-203`).

Score lands at 8: explicit dependency direction, multi-surface exports, structural RFC with snapshot ratchet, eslint-restricted relative imports, and a deliberate EE split. The migration debt and warn-only dep-cruiser enforcement keep it off the 9-10 tier.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Workspace declaration | `packages: [web, worker, "packages/**", ee]` | `sources/langfuse/pnpm-workspace.yaml:1-6` |
| Top-level task graph and DB codegen | `db:generate, build, test` per-package | `sources/langfuse/turbo.json:11-122` |
| Repo-wide scripts versioned under shared | `release-it` bumps VERSION.ts in shared, web, worker | `sources/langfuse/package.json:75-95` |
| Documented dependency direction | `web → shared,ee; worker → shared; ee → shared; shared → none` | `sources/langfuse/AGENTS.md:30-36` |
| Hardened note against relative deep imports | `Relative paths escaping web/ bypass @langfuse/shared's exports map` + PR #15031 precedent | `sources/langfuse/web/eslint.config.mjs:23-36` |
| Restricted-import regex | `(\.\./)+(packages\|ee\|worker)/` | `sources/langfuse/web/eslint.config.mjs:33` |
| `@langfuse/shared` exports map | `".", "./src/db", "./src/env", "./src/server", "./src/server/llm/llmText", "./src/server/auth/apiKeys", "./instrumentation/bootstrap", "./src/utils/chatml", "./encryption", "./src/server/ee/ingestionMasking", "./query", "./query/server", "./monitors", "./monitors/server", plus 9 in-app-agent subpaths` | `sources/langfuse/packages/shared/package.json:17-114` |
| `@langfuse/shared` default root barrel | ~137 `export *` lines, all server-free (no Prisma) | `sources/langfuse/packages/shared/src/index.ts:1-137` |
| `@langfuse/shared/src/server` barrel | ~212 lines, explicit server-only surfaces | `sources/langfuse/packages/shared/src/server/index.ts:1-212` |
| `@langfuse/shared/src/db` = Prisma only | Exports point at `./dist/src/db.js` | `sources/langfuse/packages/shared/package.json:22-25` |
| `@langfuse/ee` exports map | `".", "./sso"` only | `sources/langfuse/ee/package.json:7-15` |
| `@langfuse/ee` surface | `env`, `isEeAvailable` | `sources/langfuse/ee/src/index.ts:1-2` |
| `@langfuse/ee` only imports one thing from shared | `removeEmptyEnvVariables` | `sources/langfuse/ee/src/env.ts:2` |
| EE consumed in web | `src/ee/*` mirrors deployment-only logic | `sources/langfuse/web/src/ee/README.md` |
| EE consumed in worker | `src/ee/*` mirrors deployment-only logic | `sources/langfuse/worker/src/ee/{cloudSpendAlerts,cloudUsageMetering,dataRetention,meteringDataPostgresExport,usageThresholds}` |
| Web shared-import discipline | Stack rank: frontend-safe root barrel; server-only paths for server code; db only in backend/tests | `sources/langfuse/web/AGENTS.md:36-44` |
| Worker shared-import discipline | Prefers `@langfuse/shared/src/server`; db only in tests | `sources/langfuse/worker/AGENTS.md:18-26` |
| Shared imports zero `@langfuse/*` siblings | `from "@langfuse/"` returns no hits under `packages/shared/src` | empirical grep over `sources/langfuse/packages/shared/src/**/*.ts` |
| Web feature/server split is structural | "A feature has two surfaces, because it is a full-stack slice: index.ts at the root (client-safe) and server/index.ts" | `sources/langfuse/packages/shared/AGENTS.md:163-167`; also `sources/langfuse/web/scripts/structure/README.md:160-167` |
| `rfc08-features-via-index` rule | forbids `features/<x>` → `features/<y>` deep import | `sources/langfuse/web/.dependency-cruiser.js:30-47` |
| `rfc10-no-client-to-server` rule | forbids non-server non-test non-pages/api code → `server/` | `sources/langfuse/web/.dependency-cruiser.js:48-64` |
| `rfc11-no-runtime-cycles` rule | cycles via type-only edges exempt; runtime cycles forbidden | `sources/langfuse/web/.dependency-cruiser.js:21-29` |
| `rfc12-pages-import-page-components` rule | `src/pages` files only import feature Page/feature index | `sources/langfuse/web/.dependency-cruiser.js:91-108` |
| `rfc07-no-component-internals` rules | PascalCase dir internals reachable only via the root file | `sources/langfuse/web/.dependency-cruiser.js:65-90` |
| RFC dashboard entry-point | `pnpm structure:stats` runs `cruise` + TS-parse census, 20 rule detectors | `sources/langfuse/web/scripts/structure/stats.mjs:52-204` |
| Baseline file location and ratchet mode | `.structure-baseline.json` (committed) regenerated only via `--baseline`; Δ shown via `--diff` | `sources/langfuse/web/scripts/structure/stats.mjs:233-252`, `sources/langfuse/web/scripts/structure/README.md:28-32` |
| Baseline totals | 2710 known violations spread across rules 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 16, 18, 19, 20 | `sources/langfuse/web/.structure-baseline.json` |
| Move-tool is part of the structure story | `pnpm structure:move` rewrites imports via TypeScript's LanguageService | `sources/langfuse/web/scripts/structure/README.md:43-119` |
| Worker feature layout | `queues/`, `services/`, `features/{batch*,scores,blobstorage,evaluation,in-app-agent,...}` mirror distinct job families | `sources/langfuse/worker/src/app.ts:9-105` |
| Worker queue contract ownership | Queue names + zod schemas live in `@langfuse/shared/src/server/queues.ts` | `sources/langfuse/packages/shared/AGENTS.md:13-15`, `sources/langfuse/packages/shared/src/server/queues.ts` |
| Web public API placement | `src/pages/api/public/**` with `withMiddlewares` and Fern sources | `sources/langfuse/web/AGENTS.md:27`, `sources/langfuse/web/src/features/public-api/` (components, hooks, server, shared, types) |
| In-app-agent split across three packages | shared = durable contracts; web = UI/tRPC; worker = Mastra runtime; sandbox = Docker image | `sources/langfuse/AGENTS.md:42-46`, `sources/langfuse/packages/shared/AGENTS.md:62-79`, `sources/langfuse/worker/AGENTS.md:43-49` |
| In-app-agent subpath sandboxing | 9 explicit `@langfuse/shared/in-app-agent/...` subpaths | `sources/langfuse/packages/shared/package.json:74-113` |
| Sandbox runtime as a Docker artifact | Only string-referenced from worker as image name `langfuse-in-app-agent-sandbox:latest`; not a JS dep | `sources/langfuse/worker/src/features/in-app-agent/runtime/sandbox/config.ts:6`, `sources/langfuse/packages/in-app-agent-sandbox-runtime/package.json:1-31` |
| Test placement convention (web) | server `__tests__/server/*.servertest.ts`, unit at `__tests__/server/unit/`, client via `*.clienttest.ts(x)`, e2e at `__e2e__/*` | `sources/langfuse/web/AGENTS.md:14-16` |
| Test placement convention (worker) | `src/__tests__/*`, `src/queues/__tests__/*` | `sources/langfuse/worker/AGENTS.md:14-17` |
| Test isolation rule | `rfc19-tests-only-from-tests`, `rfc19-no-cross-feature-tests`, `rfc19-global-tests-never-import-feature-tests` | `sources/langfuse/web/.dependency-cruiser.js:109-143` |
| Vitest workspace split in `web` | 8 projects (server, server-isolated, server-shared-source, server-unit, server-shared-source-unit, in-source, client, storybook, e2e-server) | `sources/langfuse/web/package.json:21-28` |
| Public/internal marks inside shared | `@deprecated` annotations on routing wrappers re-exported at the public server barrel; one `@internal` JSDoc on `DatasetItemValidator` | `sources/langfuse/packages/shared/src/server/index.ts:186-212`, `sources/langfuse/packages/shared/src/server/services/DatasetService/DatasetItemValidator.ts:31`, `sources/langfuse/packages/shared/src/server/repositories/scores-utils.ts:8,59` |
| knip wired but selectively disabled | `ignoreIssues` for files/exports/types in every runtime workspace | `sources/langfuse/knip.jsonc:4-11` |
| EE package isolation rules | "Keep EE-only concerns isolated from OSS package code paths" | `sources/langfuse/ee/AGENTS.md:22-26` |

## Answers to Dimension Questions

### 1. Are modules cleanly separated?

Yes — modules are separated along three axes (container, runtime capability, exposed surface). At the container level `web`/`worker` are separate Next.js / Express services with their own `package.json`/`tsconfig.json` (`sources/langfuse/web/package.json`, `sources/langfuse/worker/package.json:1-94`, `sources/langfuse/web/tsconfig.json:1-36`, `sources/langfuse/worker/tsconfig.json:1-17`). At the capability level `@langfuse/shared` owns cross-runtime contracts (Prisma schema, ClickHouse migrations, queue contracts, repositories, ingestion, dashboard query data model), while feature logic lives in `web/src/features/*` and `worker/src/features/*` (`sources/langfuse/packages/shared/AGENTS.md:10-50`, `sources/langfuse/worker/src/features/*`). At the surface level every workspace exposes named subpaths via `package.json#exports` instead of one unconstrained barrel, e.g. `@langfuse/shared/src/server` vs `@langfuse/shared/src/db` (`sources/langfuse/packages/shared/package.json:17-114`). Caveats: the `.structure-baseline.json` records 97 client-to-server hits (rule 10) and 1214 cross-feature deep imports (rule 8), so the *target* separation is cleaner than the *current* one.

### 2. Do dependencies flow in one direction?

Yes — and the direction is documented as a hard rule. `web` → `@langfuse/shared`, `@langfuse/ee`; `worker` → `@langfuse/shared`; `@langfuse/ee` → `@langfuse/shared`; `@langfuse/shared` → no other langfuse workspace (`sources/langfuse/AGENTS.md:30-36`). The reverse edges are prevented by:
- empirical absence of `from "@langfuse/"` imports anywhere under `packages/shared/src` (grep returns zero matches);
- explicit `exports` maps that pin consumers to `dist/` outputs, never relative paths to other workspaces;
- eslint `no-restricted-imports` `^(\.\./)+(packages|ee|worker)/` (`sources/langfuse/web/eslint.config.mjs:33`) which fails the build if `web` reaches across via relative paths (the rule comment at `sources/langfuse/web/eslint.config.mjs:23-32` cites PR #15031 as a real production-deploy-breaking case).

The exception: in-app-agent *runtime* (`@repo/in-app-agent-sandbox-runtime`) is intentionally **not** a JS dep — it's built into a Docker image and only string-referenced by the worker for sandbox spawning (`sources/langfuse/packages/in-app-agent-sandbox-runtime/package.json:1-31`, `sources/langfuse/worker/src/features/in-app-agent/runtime/sandbox/config.ts:6`).

### 3. Can modules be used independently?

Yes — `pnpm` workspace packages are independently installable/buildable, and `package.json#exports` blocks enforce the boundaries:
- `@langfuse/shared` can be tested in isolation via `pnpm --filter @langfuse/shared run test` and exposes distinct entry points for tests, scripts, web, and worker (`sources/langfuse/packages/shared/package.json:115-140`).
- `@langfuse/ee` can be built standalone (`pnpm --filter @langfuse/ee run build`) and reused by either web or worker (`sources/langfuse/ee/package.json:18-26`).
- Web subpath barriers exist: `web/src/features/in-app-agent/ARCHITECTURE.md` documents that the in-app-agent *runtime* lives in worker, while web owns the UI/tRPC adapters (`sources/langfuse/web/AGENTS.md:42-46`, mirrored in `sources/langfuse/worker/AGENTS.md:43-49`).
- The web vitest workspace intentionally separates `server` from `server-shared-source` projects to keep the `sharedSourceResolve` alias scoped; moving it to root config slowed tests by ~27-30% (`sources/langfuse/web/AGENTS.md:111-114`). This is concrete evidence that module reuse is a perf-sensitive boundary, not just an architectural one.

Granularity inside `@langfuse/shared` also matters: `AGENTS.md` calls out explicit subpaths (`./src/server/auth/apiKeys`, `./src/server/ee/ingestionMasking`, `./src/server/llm/llmText`, `./src/utils/chatml`, the nine in-app-agent subpaths) so consumers don't import the whole package (`sources/langfuse/packages/shared/AGENTS.md:54-58`, `sources/langfuse/packages/shared/package.json:34-113`).

### 4. Are public APIs distinguished from internal ones?

Partially — the runtime/structural separation is explicit and robust, but traditional visibility annotations (`@internal`, `public` modifier) are sparse.

Runtime/structural separation:
- @langfuse/shared has **three** distinct exports: client-safe root (`@langfuse/shared`), server-only (`@langfuse/shared/src/server`), Prisma-only (`@langfuse/shared/src/db`). Server consumers must not import the client barrel or risk shipping Prisma + ClickHouse into a browser bundle — that invariant is stated as a structural requirement (`sources/langfuse/packages/shared/AGENTS.md:163-167`, `sources/langfuse/web/scripts/structure/README.md:160-167`).
- `@langfuse/ee` exposes only `.` and `./sso` (`sources/langfuse/ee/package.json:7-15`).
- EE consumed in web vs worker via parallel `src/ee/` mounts (`sources/langfuse/web/src/ee/README.md`, `sources/langfuse/worker/src/ee/*`).
- "Severability" of the in-app-agent surface: 9 subpaths each pinning to one server concern (`persistence`, `runLifecycle`, `tunables`, `eventCompaction`, `mcpPolicy`, `toolResults`, `toolErrors`, `systemPrompt`, `modelProvider`); the Mastra runtime stays in worker and is **not** a subpath (`sources/langfuse/packages/shared/AGENTS.md:67-79`, `sources/langfuse/packages/shared/package.json:78-113`).

Documentation annotations are minimal: only three `/** @internal */` JSDoc tags exist (on `DatasetItemValidator` and two helpers in `scores-utils`); a handful of `@deprecated` marks on shared routing wrappers (`sources/langfuse/packages/shared/src/server/services/DatasetService/DatasetItemValidator.ts:31`, `sources/langfuse/packages/shared/src/server/repositories/scores-utils.ts:8,59`, `sources/langfuse/packages/shared/src/server/index.ts:186-212`). There is no first-class `public`/`internal` keyword discipline and no SDK-extracted public surface in the source (the SDK is a separate `langfuse` package declared as a dep at `sources/langfuse/web/package.json:131` and `sources/langfuse/worker/package.json:69`). The package-level exports map is the *de facto* API contract.

## Architectural Decisions

- **One package per runtime, exports map as the API surface.** Every cross-package consumer must go through `package.json#exports`, never a relative path (`sources/langfuse/packages/shared/package.json:17-114`, `sources/langfuse/ee/package.json:7-15`). The eslint restricted-import rule (`sources/langfuse/web/eslint.config.mjs:33`) is the second line of defense.
- **Shared package is split into client-safe root + server barrel + Prisma barrel.** The root barrel (`packages/shared/src/index.ts`) re-exports only types, zod schemas, table definitions, prompt/eval/model-pricing helpers, and the unicode-aware JSON serialize helpers (`stringify`, `stringifyForCsv`); the server barrel re-exports repositories, queue helpers, ingestion, AI SDK adapters, logger, and all query/storage services (`sources/langfuse/packages/shared/src/index.ts:1-137`, `sources/langfuse/packages/shared/src/server/index.ts:1-212`). The db entrypoint is exclusively for backend code/tests (`sources/langfuse/packages/shared/AGENTS.md:46-50`).
- **Each web feature has two surfaces: `index.ts` and `server/index.ts`.** Client code imports the root `index.ts` only; server-only code reaches for `server/index.ts`. The README and AGENTS.md are explicit that mixing the two would let Prisma leak into client bundles — "nothing crashes when that happens, which is exactly why it has to be structural" (`sources/langfuse/web/scripts/structure/README.md:163-167`, `sources/langfuse/packages/shared/AGENTS.md:163-167`).
- **Queue payload schemas and queue names are owned in shared.** Single-source-of-truth prevents drift between web producers and worker consumers; queue helpers live next to the schemas in `packages/shared/src/server/redis/*` (`sources/langfuse/packages/shared/AGENTS.md:13-15`, `sources/langfuse/worker/AGENTS.md:25-36`).
- **In-app-agent sandbox is a sibling Docker package, not a JS dep.** `@repo/in-app-agent-sandbox-runtime` only depends on `zod`; the worker string-references `langfuse-in-app-agent-sandbox:latest` when spinning Docker containers (`sources/langfuse/packages/in-app-agent-sandbox-runtime/package.json:20-31`, `sources/langfuse/worker/src/features/in-app-agent/runtime/sandbox/config.ts:6`, `sources/langfuse/worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:89`). This isolates runtime/OS dependencies from the worker JS.
- **Project-structure RFC is instrumented as a ratcheting baseline.** 20 rules, mix of census (TS-parse) and dependency-cruiser graph, with a committed baseline (`web/.structure-baseline.json`) that only shrinks — `pnpm structure:stats --diff` shows progress over time (`sources/langfuse/web/scripts/structure/stats.mjs:231-253`, `sources/langfuse/web/scripts/structure/README.md:28-32`).
- **Knip is wired but selectively disabled** for files/exports/types across all runtime workspaces; the only remaining coverage is file-level orphan detection via rule 20 (symbol-level marked TBD) (`sources/langfuse/knip.jsonc:1-56`, `sources/langfuse/web/scripts/structure/stats.mjs:200-203`).
- **Turbo task graph makes cross-package ordering explicit.** `lint` depends on `@repo/eslint-plugin#build`, `typecheck`/`build`/`test` depend on `db:generate` and `^build`, the sandbox docker-image build depends on `build` (`sources/langfuse/turbo.json:11-122`).

## Notable Patterns

- **Subpath-narrowed exports for sensitive surfaces**: e.g. `@langfuse/shared/instrumentation/bootstrap` is the only sanctioned way to import OpenTelemetry initializers because it must load before `sdk.start()` (`sources/langfuse/packages/shared/package.json:42-44`, `sources/langfuse/packages/shared/AGENTS.md:60-64`).
- **Re-exported deprecated routing wrappers** preserve JSDoc through `export *` hops — a small but concrete example of API surface engineering (`sources/langfuse/packages/shared/src/server/index.ts:186-212`).
- **Structural rules encoded as regex approximations** in CI (`.dependency-cruiser.js`) and as canonical detectors in `scripts/structure/detectors.mjs` run by `stats.mjs`. The README explicitly states the regex versions may undercount and the reference detectors are the source of truth (`sources/langfuse/web/scripts/structure/README.md:142-145`).
- **Three-monorepo-package split for the in-app-agent** (shared contracts + web UI/tRPC + worker Mastra runtime + sandbox Docker image) is the cleanest separation. The shared package's `runMetrics.ts`, `runLifecycle.ts`, `persistence.ts` deliberately avoid web/worker knowledge — they only know about Prisma + AG-UI primitives (`sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:1-28`).
- **Test homes are kind-tagged** (`*.clienttest.ts(x)`, `*.servertest.ts`, `*.test.ts`, `*.stories.tsx`) so a single Vitest grep globs the right test family (`sources/langfuse/web/package.json:21-28`, `sources/langfuse/web/AGENTS.md:14-16`). Worker is simpler: only `__tests__`/`queues/__tests__` (`sources/langfuse/worker/AGENTS.md:14-17`).
- **`server-shared-source` Vitest projects** keep the shared-source alias opt-in instead of global — moving it global slowed Vitest by 27-30%, so the split is in the test config (`sources/langfuse/web/AGENTS.md:111-114`).

## Tradeoffs

- **Warn-only dep-cruiser rules vs hard constraints.** All RFC rules in `.dependency-cruiser.js` are `severity: warn` (`sources/langfuse/web/.dependency-cruiser.js:21-143`). The project compensates with the ratcheting baseline (`pnpm structure:stats`/`.structure-baseline.json`/structured PRs) so the count only goes down — which is a process-level trade-off rather than a code-level one. Two outcomes follow: (a) the baseline will not move on its own, only when someone deliberately widens the snapshot; (b) the rate at which violations shrink is bounded by reviewer attention to RFC rules.
- **Two parallel implementations of the same rules (regex vs TS-parse).** The dep-cruiser config is the cheap regex version that runs in CI; the `scripts/structure/detectors.mjs` (referenced via `next.mjs`/`census.mjs`) is the canonical TS-parse version used by `stats.mjs`. They can drift because `package.json` only depends on the cruise file (`sources/langfuse/web/scripts/structure/README.md:142-145`).
- **Massive tail of "kind folder" violations (rule 5: 102).** Many legacy folders (`utils`, `lib`, etc.) predate the RFC. Migration is incremental; nothing crashes because of them (`sources/langfuse/web/.structure-baseline.json`).
- **`pages` shim sprawl (rule 12: 616).** Next.js Pages Router routes are individually thin; converting them to a single `<Feature>Page.tsx` is mechanical but large.
- **In-app-agent runLifecycle cross-escape.** The shared `runLifecycle.ts` imports `../../server` (`logger`, `recordIncrement`) to record outcomes — a deliberate concession to avoid duplicating infrastructure, but it does mean the in-app-agent server subpaths are not as server-free as the directory name suggests (`sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:9`, `sources/langfuse/packages/shared/src/in-app-agent/server/runMetrics.ts:2`).
- **EE imports exactly one thing from shared: `removeEmptyEnvVariables`.** Almost everything else is hidden behind `isEeAvailable` / `env` (`sources/langfuse/ee/src/index.ts:1-2`, `sources/langfuse/ee/src/env.ts:2`). This is the cleanest possible EE/OSS split but it does mean all EE code lives in `web/src/ee/*` and `worker/src/ee/*` with no upstream contract — drift risk would be high if EE grew materially.
- **Knip stays in `files`/`exports`/`types` ignore mode.** Acknowledged as a workaround for false-positiveing until knip tracks all workspace packages and catalog entries (`sources/langfuse/knip.jsonc:1-11`). Today the only "dead code" signal is rule 20 file-level orphans in structure stats.
- **Sandbox runtime is built into a Docker image but only referenced by `image:`.** This decouples the worker's npm graph from sandbox deps (good), but it also means there's no type-checked contract between the worker and the sandbox — sandbox payloads are validated at runtime in `server.ts` (`sources/langfuse/packages/in-app-agent-sandbox-runtime/src/{contracts.ts,server.ts}`).

## Failure Modes / Edge Cases

- **Public barrel re-exports collapsing to dist mismatches.** The eslint rule explicitly warns that relative paths to other workspaces bypass the exports map and pull shared *source* into the web typecheck program, breaking next-auth augmentation — production deploy PR #15031 is cited (`sources/langfuse/web/eslint.config.mjs:23-32`). Mitigation: hard-coded eslint regex.
- **Dynamic `import()` not covered by `no-restricted-imports`.** The rule comment notes only static imports are checked; tests legitimately need dynamic imports for Vite-transformed shared source copies (`sources/langfuse/web/eslint.config.mjs:27-32`). This is a known gap that could regress.
- **Test-suite DB conflicts via web dev server.** `vitest.config.mts` loads `.env.test` so the test DB is separate from the dev DB; running dev against the test DB by mistake yields a flood of 401s because the API keys created by tests exist in the test DB only (`sources/langfuse/web/AGENTS.md:101-104`). Boundary hygiene relies on the operator starting the right server.
- **String-referenced modules invisible to the graph.** Vi-mock paths, worker URLs, and route strings are not seen by dep-cruiser; a `structure:move` will print surviving hits and ask a human to fix (`sources/langfuse/web/scripts/structure/README.md:111-116`). This is part of the calibration — automated migration stops at known blind spots.
- **Calibration of the reworked traces feature.** Rule 7/9 treat any PascalCase directory as a component boundary; lowercase containers (`components/ui`, `components/table`) are explicit legacy, not boundaries (`sources/langfuse/web/scripts/structure/README.md:170-176`). A new contributor could miss this nuance and accidentally treat `components/ui` like a feature.
- **Type-only cycles are still reported as a survey metric.** Rule 11 forbids runtime cycles only; type-only edges are tracked but don't fail CI. If a refactor lifts a type into a runtime value, the cycle becomes a hazard and the metric is the only signal (`sources/langfuse/web/.dependency-cruiser.js:21-29`, `sources/langfuse/web/scripts/structure/stats.mjs:227-231`).
- **V4 write-mode gates.** Adding a queue consumer that reads from `events_full` must be guarded by `v4WritesToEventsTable(env)` in `worker/src/app.ts:670-684`; otherwise the consumer pulls empty tables in legacy write mode. The boundary is `worker`-internal but the contract is shared (`sources/langfuse/worker/src/app.ts:670-684`).
- **OpenTelemetry bootstrap ordering.** The shared instrumentation bootstrap (`./instrumentation/bootstrap`) must be loaded before `sdk.start()` and must not pull the server barrel — importing the server barrel would transitively load instrumented libraries and silently disable instrumentation (`sources/langfuse/packages/shared/AGENTS.md:60-64`, `sources/langfuse/packages/shared/package.json:42-44`).

## Future Considerations

- **Ship `structure:stats --baseline` updates as code-only PRs.** Today the dashboard gives `Δ` per rule; tightening the baseline is a deliberate act (`sources/langfuse/web/scripts/structure/stats.mjs:236-253`). A regular "shrink the baseline" cadence would turn rule 8 (1214) and rule 12 (616) into tractable milestones.
- **Promote dep-cruiser rules from `warn` to `error`** in small slices (e.g. once `rfc10-no-client-to-server` is under, say, 10 remaining) to gate regressions at PR time. The current model is intentional but it relies on process discipline.
- **Re-enable knip `files`/`exports`/`types`** once knip tracks every workspace and respects catalog entries (the ignore reason is explicit) (`sources/langfuse/knip.jsonc:3-11`). Add `web/src/workers/**` and Next.js `app/` to knip's project globs first.
- **Symbol-level dead-code detection.** Rule 20 is "TBD symbol-level (follow-up)" (`sources/langfuse/web/scripts/structure/stats.mjs:200-203`, `sources/langfuse/web/scripts/structure/README.md:140`). Closing that would require a knip config plus a CI step.
- **EE split is binary today.** Almost no surface area leaks either way — but EE today does not own any payload schemas. As EE grows (SSO already has a subpath: `ee/sso`), consider whether queue payloads should also split cleanly, with shared exports explicitly grouping EE-only contracts (`sources/langfuse/ee/package.json:7-15`).
- **Sandbox → worker contract.** Currently validated at runtime in `contracts.ts` (`sources/langfuse/packages/in-app-agent-sandbox-runtime/src/contracts.ts`). Promoting that to a generated TS types package would catch mismatches at build time without taking on `shared/src/server` as a runtime dep.
- **Promote RFC 19 hard fail.** Currently the cross-feature test-isolation rules are warn-level; given 0 violations today (`sources/langfuse/web/.structure-baseline.json` rule-19 entry), hardening them to error would be free.
- **Public/internal visibility annotations.** Only three `/** @internal */` marks exist; promoting a routine (e.g. `@langfuse/api-extractor` or hand-written `public.d.ts`) would make the SDK contract enforceable rather than "trust the exports map."

## Questions / Gaps

- **Is the EE/OSS split symmetrical across web and worker?** `web/src/ee/*` has a `README.md` but worker has no equivalent docs at the same path; `worker/src/ee/` lists 5 directories (`cloudSpendAlerts`, `cloudUsageMetering`, `dataRetention`, `meteringDataPostgresExport`, `usageThresholds`) without a single index or contract — unclear how they're registered with `@langfuse/ee`'s `isEeAvailable`. Need a worker-side README that mirrors web's.
- **What lives behind the `in-app-agent-sandbox-runtime` runtime contract?** `contracts.ts` declares the input/output schema, but no cross-reference exists in shared's queue payloads (the worker passes conversation JSON to a Docker container). No shared subpath or generated types bridge the two sides — risk of payload drift over time.
- **Are tail-of-baseline rules ever re-elaborated?** Rule 1 (PascalCase, 196) is heavy; rule 13 (components/ui frozen) is policy rather than mechanical; rule 16 (line-level disables, 108) is intentionally allowed. Why is rule 13 ratchet-style while rule 16 is counted but not ratcheted? No comment in `detectors.mjs` resolves this.
- **Why no `tsc`-driven cycle detection in CI?** Only dep-cruiser's `circular:true` rule runs, and it's `warn`. A tsc-based cycle check (or `ts-morph`) would catch dynamic shapes dep-cruiser misses.
- **Where is the "client bundle soundness" gate?** `scripts/scan-client-bundle.mjs` exists (`sources/langfuse/package.json:23`), used in `pnpm run scan:client-bundle`. The shared `package.json#exports` discipline plus rfc10 are upstream defenses; the scan is the catch-all — but only invoked on demand.
- **`pnpm exec knip` runs in CI but ignores exports/types** (`sources/langfuse/knip.jsonc:4-11`). With 2710 structural baseline violations, the project already accepts incomplete hygiene signal — what's the target tolerance?
