# Source Analysis: langfuse

## Dimension 22.02 — Configuration and Deployment Shape

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Next.js (web) + Express & BullMQ (worker), pnpm + turbo monorepo, Docker Compose deployment |
| Analyzed | 2026-08-25 |

## Summary

Langfuse manages configuration almost entirely through environment variables validated at process start by per-package Zod schemas. There is no config-file or remote-config layer; instead, four schema files partition configuration by runtime: a large shared backend schema (`packages/shared/src/env.ts:32`), a web-only t3-env schema with server/client split (`web/src/env.mjs:40`), a worker-only schema with post-parse cross-field semantic validation (`worker/src/env.ts:5,694-744`), and a minimal enterprise-edition schema (`ee/src/env.ts:4-9`). Defaults live inside the schemas themselves as `z.coerce.*.default(...)` values, making the schema the single source of truth for both shape and defaults.

Deployment-wise, the repo produces two server images from one codebase — `langfuse-web` (UI + tRPC + public REST) and `langfuse-worker` (queue consumers) — plus a full dependency stack (Postgres, ClickHouse, Redis, MinIO/S3). The same images run in dev, staging-like local, and production with environment-variable changes only; build-time variance is confined to `NEXT_PUBLIC_*` args baked into the web image (`web/Dockerfile:71-114`). Deployment targets are documented externally: docker compose for local/VM and Kubernetes Helm ("preferred production deployment") plus Terraform templates for AWS/Azure/GCP (`README.md:104-123`). Feature flags exist in two layers: typed per-user flags persisted in Postgres (`web/src/features/feature-flags/available-flags.ts:38-45`) and dozens of env-var kill switches/migration gates in the env schemas.

The main weakness is manual synchronization of duplicated keys across the three schemas: shared variables such as `LANGFUSE_MIGRATION_V4_WRITE_MODE` are defined three times with "keep this value in sync" comments (`packages/shared/src/env.ts:322-330`, `worker/src/env.ts:560-564`, `web/src/env.mjs:504-512`) rather than being composed from one definition, creating drift risk that only comments and convention guard against.

## Rating

**8 / 10** — Clear model with explicit interfaces, startup fail-fast validation, cross-field semantic checks, and real operational safeguards (health probes, migration entrypoints, empty-string normalization, credential diagnostics). It falls short of 9–10 because shared keys are duplicated across schemas with comment-enforced sync instead of shared composition, and the V4 cross-field validation logic has no dedicated unit tests (searched `worker/src/__tests__` and `packages/shared` for `validateV4|Invalid V4 config` — no matches).

On the rubric question — *"Can the same binary run in dev, staging, and prod with config changes only?"* — yes for the worker unconditionally; yes for web except `NEXT_PUBLIC_*` variables, which are compile-time in Docker (`web/src/env.mjs:583` warning; `web/Dockerfile:71-114`) and one of which even changes installed dependencies at image build (`dd-trace` install gated on `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION`, `web/Dockerfile:163-167`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared backend config loader | `EnvSchema = z.object({...})` (~156 keys); module-level parse with `DOCKER_BUILD=1` bypass | `packages/shared/src/env.ts:32,584-587` |
| Web config loader | t3-oss `createEnv` with `server:`/`client:` sections, `runtimeEnv` map, `skipValidation: DOCKER_BUILD === "1"`, `emptyStringAsUndefined: true` | `web/src/env.mjs:40,585-618,624,1040-1043` |
| Worker config loader | Own `EnvSchema` (~158 keys); imports shared sub-schema for S3 key bytes | `worker/src/env.ts:5,1-3,72-73` |
| Shared validation fragment reuse | `langfuseS3EventKeyMaxSegmentBytesSchema` defined once in shared, consumed by worker so producer/consumer agree | `packages/shared/src/env.ts:8-13`; `worker/src/env.ts:68-73` |
| Empty-string normalization | `removeEmptyEnvVariables` deletes `""` values so compose `${VAR:-}` defaults don't clobber schema defaults | `packages/shared/src/utils/environment.ts:3-10` |
| EE config | Minimal 2-key zod schema parsed at import | `ee/src/env.ts:4-9` |
| Cross-field semantic validation | `validateV4Flags` hard-errors on data-losing flag combos (e.g. `legacy`+`direct`, `events_only`+`dual_write`) | `worker/src/env.ts:694-721` |
| Cross-field validation #2 | `validateInAppAgentSandboxConfig` requires image/role/region when provider is `lambda-microvm` | `worker/src/env.ts:723-737` |
| Environment discriminator | `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` enum `["US","EU","STAGING","DEV","HIPAA","JP"]` gates cloud-only behavior | `worker/src/env.ts:21-23`; `web/src/env.mjs:589-591` |
| Cloud vs self-hosted switch | `const isLangfuseCloud = Boolean(env.NEXT_PUBLIC_LANGFUSE_CLOUD_REGION)` used to gate procedures/routes | `web/src/server/api/trpc.ts:102,420-424` |
| Build/runtime validation split | `ENV DOCKER_BUILD 1` during build; `ENV DOCKER_BUILD 0` in runner re-enables runtime validation | `web/Dockerfile:62,148-149`; `worker/Dockerfile:87-88` |
| Dev/prod parity via compose | Prod-shaped single-host stack (web+worker+clickhouse+minio+redis+postgres) with healthcheck-gated startup | `docker-compose.yml:6-210` |
| Dev infra compose | Infra-only stack for source-run dev (ClickHouse 25.12 pinned, floci Lambda emulator under `worker-tests` profile) | `docker-compose.dev.yml:1-60` |
| Env example files | `.env.dev.example`, `.env.prod.example`, `.env.test.example`, `.env.dev-azure/-redis-cluster/-oci.example` variants | `.env.dev.example:1-80`; `.env.prod.example:1`; `.env.test.example:1-15` |
| Migration entrypoint | Web entrypoint runs Prisma + ClickHouse migrations before start; disable via `LANGFUSE_AUTO_POSTGRES_MIGRATION_DISABLED` / `LANGFUSE_AUTO_CLICKHOUSE_MIGRATION_DISABLED`; heuristic credential-encoding hints | `web/entrypoint.sh:8-57,89-125`; `worker/entrypoint.sh:3-23` |
| Per-consumer deployment shaping | ~30 `QUEUE_CONSUMER_*_IS_ENABLED` toggles gate each BullMQ consumer registration in app bootstrap | `worker/src/env.ts:234-353`; `worker/src/app.ts:137-686` |
| Consumer default divergence | Compose turns the in-app-agent consumer on while the worker schema defaults it off (ingestion-only processes) | `docker-compose.yml:86-89`; `worker/src/env.ts:239` |
| Health endpoints | Opt-in deep health: DB `SELECT 1`, recent-events check that branches on `LANGFUSE_MIGRATION_V4_WRITE_MODE=events_only` | `web/src/features/public-api/server/health-service.ts:22-133` |
| Liveness thresholds | Stuck-detection knobs for event propagation and queue consumption with k8s probe guidance in comments | `worker/src/env.ts:607-639` |
| Typed user feature flags | `availableFlags` registry incl. preview flags; opt-out token scheme; email-domain-based default-on | `web/src/features/feature-flags/available-flags.ts:3-45`; `web/src/features/feature-flags/utils.ts:10-53` |
| Flag evaluation semantics | Enabled if user flag set OR admin OR `LANGFUSE_ENABLE_EXPERIMENTAL_FEATURES`; client hook mirrors it | `web/src/server/api/trpc.ts:403-418`; `web/src/features/feature-flags/hooks/useIsFeatureEnabled.ts:18-22`; `web/src/features/feature-flags/README.md:11-19` |
| Flags tested | `parseFlags` round-trip and opt-out behavior covered in server tests | `web/src/__tests__/server/userAccount.servertest.ts:27-93` |
| Kill switches & rollout gates | `LANGFUSE_OBSERVATIONS_V2_SUBQUERY_REWRITE`, shadow-query sample rate, v4 write-mode triad, background-migration env gates discovered by prefix scan | `web/src/env.mjs:536-546,496-498`; `worker/src/env.ts:560-599` |
| Cohort rollout boundary | `LANGFUSE_MIGRATION_V4_OTEL_DIRECT_WRITE_ORG_CREATED_CUTOFF` rolls tenant cohorts forward by org creation date | `worker/src/env.ts:568-582` |
| Deployment docs | README lists docker compose (local/VM), Kubernetes Helm as preferred production, Terraform templates AWS/Azure/GCP; helm chart maintained in separate `langfuse-k8s` repo (referenced from a config comment) | `README.md:104-123`; `packages/shared/src/env.ts:333-341` |
| Duplicated keys / sync burden | `LANGFUSE_MIGRATION_V4_WRITE_MODE` defined in all three schemas, each carrying "keep this value in sync" instructions | `packages/shared/src/env.ts:322-330`; `worker/src/env.ts:555-564`; `web/src/env.mjs:504-512` |

## Answers to Dimension Questions

**1. Is configuration layered?**
Layered by package/runtime, not by file precedence or environment overrides. Four independent Zod schemas cover shared backend (`packages/shared/src/env.ts:32`), web server/client (`web/src/env.mjs:40,585`), worker (`worker/src/env.ts:5`), and EE (`ee/src/env.ts:4`). Where a rule must be identical across runtimes, the shared package exports the fragment (S3 key-byte schema reused by the worker, `worker/src/env.ts:72-73`) — but most overlapping keys are copy-pasted with sync comments (`packages/shared/src/env.ts:322-330`). Defaults are embedded in schemas, not in separate config files. Within web, t3-env provides a server/client layering with explicit `runtimeEnv` bridging (`web/src/env.mjs:624`).

**2. Are environments managed cleanly?**
Yes, via strict env-var-only configuration with per-environment example files and compose topologies: `.env.dev.example` vs `.env.prod.example` vs `.env.test.example` (test DB isolation via separate Postgres database and Redis logical DB 1, `.env.test.example:5-14`), plus specialized dev variants for Azure, Redis cluster, and OCI storage. `NODE_ENV` is enum-validated (`packages/shared/src/env.ts:41-43`), cloud regions are an explicit enum including STAGING/DEV (`worker/src/env.ts:21-23`), and dev-only escape hatches are labeled as such (`NEXT_PUBLIC_LANGFUSE_BLOB_EXPORT_CUTOFF`, `packages/shared/src/env.ts:34-40`). The one wrinkle: `NEXT_PUBLIC_*` values are baked at image build (`web/Dockerfile:71-114`), so some environment differences require a rebuild, not just config.

**3. Are deployment modes documented?**
Documented outside the repo's code but anchored in it: `README.md:104-123` enumerates docker compose (local, VM), Kubernetes/Helm ("preferred production deployment"), and Terraform templates; the Helm chart lives in the separate `langfuse/langfuse-k8s` repo referenced from a config-validation comment about a MinIO checksum workaround (`packages/shared/src/env.ts:333-341`). In-repo compose files demonstrate the shapes: full self-contained stack (`docker-compose.yml:6-210`), dev infra only (`docker-compose.dev.yml:1-60`), and build smoke test (`docker-compose.build.yml:1-30`). Functional deployment modes are expressed through the ~30 per-queue consumer toggles (`worker/src/env.ts:234-353` applied in `worker/src/app.ts:137-686`), letting operators run ingestion-only vs full workers, and through the cloud/self-hosted discriminator `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` which also conditionally installs dd-trace in the image (`web/Dockerfile:163-167,200-205`). Embedded/sidecar modes do not apply; the nearest analogue is the pluggable code-eval dispatcher (`insecure-local` vs `aws-lambda`, `packages/shared/src/env.ts:202-204`) and sandbox provider selection (`worker/src/env.ts:723-737`).

**4. Are feature flags supported?**
Yes, two systems. (a) Typed per-user flags stored on the user row, registered in `availableFlags` (`web/src/features/feature-flags/available-flags.ts:38-45`), resolved by `parseFlags` with per-user opt-out tokens and domain-based default-on (`web/src/features/feature-flags/utils.ts:15-53`), enforced server-side by the `requireFeatureFlag` middleware (`web/src/server/api/trpc.ts:403-418`) and client-side by `useIsFeatureEnabled` (`hooks/useIsFeatureEnabled.ts:18-22`); semantics documented in `web/src/features/feature-flags/README.md:11-19` and exercised in tests (`web/src/__tests__/server/userAccount.servertest.ts:27-93`). (b) Env-var kill switches and staged-rollout gates: query-rewrite kill switch and shadow-query sample rate (`web/src/env.mjs:536-546`), the v4 migration triad (`legacy|dual|events_only`, `worker/src/env.ts:560-585`), org-cohort cutoffs (`worker/src/env.ts:580-582`), and background-migration gates discovered by env-prefix scan (`worker/src/env.ts:587-599`). No third-party flag service (LaunchDarkly etc.) is used; dynamism comes from per-user DB flags while env flags require restart.

**5. Is configuration validated?**
Strongly. All four schemas parse at module load, failing fast on boot (`packages/shared/src/env.ts:584-587`; `ee/src/env.ts:9`); web uses t3-env with `skipValidation` only during Docker builds (`web/src/env.mjs:1040-1042`), re-enabled at runtime via `DOCKER_BUILD 0` (`web/Dockerfile:148-149`). Beyond shape validation there is cross-field semantic validation that refuses data-losing combinations (`validateV4Flags`, `worker/src/env.ts:694-721`) and conditional completeness checks for the lambda-microvm sandbox (`worker/src/env.ts:723-737`). Contextual rules exist too: `NEXTAUTH_SECRET` required in production (`web/src/env.mjs:49-52`), `SALT` missing-value error linking to self-hosting docs (`web/src/env.mjs:99-104`), `ENCRYPTION_KEY` fixed 64-hex-length with a generation hint (`packages/shared/src/env.ts:97-103`), and EventBridge ARN regex validation (`web/src/env.mjs:422-428`). Operational validation continues into the entrypoint, which warns about URL-unencoded DB credentials and ClickHouse password characters before migrations run (`web/entrypoint.sh:8-57,100-124`). Gap: no dedicated unit tests target the schema/cross-field validators themselves (searched `worker/src/__tests__`, `packages/shared` test globs for `Invalid V4 config|validateV4` — no matches).

## Architectural Decisions

- **Env-var-only configuration with schema-as-source-of-truth.** Every default, coercion, and constraint lives in Zod schemas (`packages/shared/src/env.ts:32-580`), eliminating a separate defaults layer but making schema edits the only way to change defaults.
- **Fail-fast boot over lazy resolution.** Config parses at import time (`packages/shared/src/env.ts:584-587`), so a bad deployment crashes immediately rather than degrading at request time.
- **Two-process product topology.** Web (request-serving) and worker (queue consumption) are separately deployable images sharing contracts via `@langfuse/shared` (`AGENTS.md` project structure section; `docker-compose.yml:7,98`), enabling independent scaling and per-queue consumer gating (`worker/src/app.ts:137-686`).
- **Build-time/runtime validation split keyed on `DOCKER_BUILD`.** Validation is skipped during image build (when infra vars don't exist) and restored at runtime (`web/Dockerfile:62,148-149`), letting one image serve all environments.
- **Migration execution coupled to container start** with opt-out flags, rather than separate migration jobs (`web/entrypoint.sh:89-115`).
- **Staged data-migration machinery driven by config**: write-mode triads, cohort date cutoffs, and env-gated background migrations (`worker/src/env.ts:560-599`) show configuration doubling as progressive-delivery control plane.

## Notable Patterns

- **Shared schema fragments exported for cross-runtime consistency** — e.g. the S3 key-segment-bytes schema is defined once and imported by the worker specifically "so producer and consumer agree" (`packages/shared/src/env.ts:8-13`; `worker/src/env.ts:68-73`).
- **Sync-comment convention** documenting which duplicated keys must stay aligned across the three schemas (`web/src/env.mjs:504-512`; `worker/src/env.ts:26-33` for the billing cutoff date).
- **Empty-string-as-unset normalization** so Compose `${VAR:-}` placeholders don't shadow schema defaults (`packages/shared/src/utils/environment.ts:3-10`; mirrored by `emptyStringAsUndefined`, `web/src/env.mjs:1043`).
- **Env-prefix-driven feature discovery** — background-migration gates are found by scanning `LANGFUSE_BACKGROUND_MIGRATION_` keys (`worker/src/env.ts:587-593`).
- **Inline operational commentary in schemas** — constraints carry rationale referencing incident numbers and failure modes (Redis socket watchdog rationale, `packages/shared/src/env.ts:15-28`; k8s probe timing caveats, `worker/src/env.ts:607-639`).
- **Region-conditional image contents** — dd-trace is installed into the runtime image only when the cloud-region build arg is present (`web/Dockerfile:163-167`).

## Tradeoffs

- **Duplication vs composition.** Copy-pasting shared keys into web/worker schemas keeps packages decoupled (shared cannot depend on web) but relies on humans honoring sync comments; drift would silently split behavior between containers (the S3-key case shows they are aware and partially mitigate with shared fragments).
- **Build-baked client config.** `NEXT_PUBLIC_*` values are compile-time (`web/src/env.mjs:583` warning; `web/Dockerfile:71-114`), trading runtime flexibility for bundle simplicity; mitigated case-by-case by serving values through tRPC instead (e.g. markdown render limit exposed via public router so it works "on prebuilt Docker images", `web/src/env.mjs:514-521`).
- **Restart-required env flags vs per-user dynamic flags.** Operational toggles need redeploy/restart, but user-facing rollouts get dynamism through DB-backed flags with opt-out (`web/src/features/feature-flags/utils.ts:15-53`).
- **Migrations in the entrypoint** simplify first-run UX but make the web container a migration actor; disable flags exist for multi-replica control (`web/entrypoint.sh:89-115`).
- **CSP computed at build time from runtime env** (`next.config.mjs:19-50`) couples security headers to whichever media endpoint was visible when the image was built.

## Failure Modes / Edge Cases

- **Schema drift between web/worker copies of a key** (e.g. divergent defaults for `LANGFUSE_MIGRATION_V4_WRITE_MODE`) would produce inconsistent reads/writes; currently guarded only by comments and one shared fragment pattern (`packages/shared/src/env.ts:322-330` vs `worker/src/env.ts:560-564`).
- **Invalid flag combinations that lose data are refused at boot**, not silently coerced: `legacy`+`direct` OTel writes, `events_only`+`dual_write`, and `events_only` without preview opt-in all throw (`worker/src/env.ts:694-721`).
- **Web/worker S3 key mismatch** if segment-byte settings differ at deploy time — explicitly called out as a read/write skew hazard and pinned via the shared schema (`worker/src/env.ts:68-73`).
- **Unencoded credentials in `DATABASE_URL`/`CLICKHOUSE_PASSWORD`** cause Prisma P1013 / migration URL breakage; entrypoint detects common offenders and prints fixes (`web/entrypoint.sh:8-57`).
- **Compose empty-string defaults masking intent** — handled centrally by stripping empty strings before parse (`packages/shared/src/utils/environment.ts:3-10`); without it, every optional var set to `""` in Compose would override schema defaults.
- **Wedged workers** are surfaced via configurable stuck-threshold health probes with documented interaction caveats (initial-delay ≥ one cron cycle; multi-replica idle raising the threshold) (`worker/src/env.ts:607-639`; health branch logic `web/src/features/public-api/server/health-service.ts:44-107`).
- **Cookie collisions across shared-domain previews** solved by `NEXTAUTH_COOKIE_NAME_SUFFIX` with regex validation and region-precedence safety note (`web/src/env.mjs:76-92`).

## Future Considerations

- Extract genuinely shared keys (v4 write mode, blob-export cutoffs, CHB cutoff) into shared schema fragments the way `langfuseS3EventKeyMaxSegmentBytesSchema` already is, removing the triple-definition drift risk (`packages/shared/src/env.ts:8-13` as the template).
- Add unit tests for `validateV4Flags` / `validateInAppAgentSandboxConfig`; these encode data-loss invariants but currently have no direct coverage (no matches in `worker/src/__tests__` for the validator names/error strings).
- Reduce `NEXT_PUBLIC_*` build-time coupling by routing more client-visible settings through server-provided config (the `LANGFUSE_MARKDOWN_RENDER_CHARACTER_LIMIT` pattern at `web/src/env.mjs:514-521` generalizes).
- Consider generating `.env*.example` files from the schemas to keep documentation and validation in lockstep (they are manually maintained today, `.env.dev.example:1-2` instructs editors to update the schema but nothing enforces the reverse).

## Questions / Gaps

- No evidence found of hierarchical config files (per-environment YAML/TOML overlays) or remote config service integration; searched for config-loader patterns beyond `process.env` and found none — env vars are the entire story.
- Secrets management (Vault/KMS/sealed secrets) is out of repo scope; only the Helm chart reference (`README.md:120-121`) and secret-bearing env keys (`STRIPE_SECRET_KEY`, `packages/shared/src/env.ts:309`) imply external handling. No in-repo evidence of how staging/prod secrets are injected.
- Whether Langfuse Cloud's internal staging/prod deployments use the exact same images could not be verified from this repository alone; evidence is indirect (region enum includes `STAGING`/`DEV`, `worker/src/env.ts:21-23`, and CI preview builds noted in `web/src/env.mjs:598-600`).
- No dedicated test file for any env schema was found (searched `*.test.ts` globs under `worker/src`, `packages/shared` for validator identifiers); flag-resolution logic is the only configuration-adjacent logic with direct tests (`web/src/__tests__/server/userAccount.servertest.ts:27-93`).

---

Generated by dimension 22.02 (Configuration and Deployment Shape) against `langfuse`.
