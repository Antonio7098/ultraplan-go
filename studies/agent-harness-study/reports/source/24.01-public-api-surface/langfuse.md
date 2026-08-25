# Source Analysis: langfuse

## Public API Surface

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript (Next.js API routes + Fern YAML) on Node.js 24; Zod for runtime validation |
| Analyzed | 2026-08-22 |

## Summary

Langfuse exposes a deliberately layered public API surface rooted in three
concrete artefacts: the **REST handlers** under `web/src/pages/api/public/**`,
the **Zod contract types** under `web/src/features/public-api/types/**`, and the
**Fern API definitions** under `fern/apis/{server,client,organizations}/**` that
generate OpenAPI specs (`web/public/generated/**/openapi.yml`). Every project-
scoped REST route funnels through two homegrown wrappers —
`withMiddlewares` (`web/src/features/public-api/server/withMiddlewares.ts:84-206`)
and `createAuthedProjectAPIRoute` (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:307-492`)
— which provide method-routing, CORS, authentication, rate-limiting, ClickHouse
resource advisories, OpenTelemetry context propagation, and structured error
shaping. A clearly segregated "unstable" namespace
(`web/src/pages/api/public/unstable/**`, mounted on `base-path: /api/public/unstable`,
`fern/apis/server/definition/unstable/evaluators.yml:9`) carries a distinct
error contract with machine-readable codes
(`web/src/features/public-api/shared/unstable-public-api-error-schema.ts:3-23`)
and a documented evolution policy. A versioned family (`/api/public/v2/observations`,
`/api/public/v2/metrics`, `/api/public/v2/datasets`, `/api/public/v2/prompts`,
`/api/public/v2/scores`, `/api/public/v3/scores`) coexists with deprecated v1
endpoints whose `_deprecation` payload is auto-stamped by the
`attachDeprecation` helper (`web/src/features/public-api/server/deprecations.ts:102-114`)
and synchronised from Fern `availability.status: deprecated` markers into
OpenAPI by `scripts/openapi/sync-deprecations.ts:17-25`. SDK ingestion runs
through two ingest paths — the Langfuse-native batch ingestion
(`web/src/pages/api/public/ingestion.ts`) and the OpenTelemetry HTTP/Protobuf
endpoints (`web/src/pages/api/public/otel/{v1,otlp-proto}/**`). Org/admin and
client-facing surfaces are separate Fern APIs with their own auth (Bearer
admin key vs Basic) (`fern/apis/organizations/definition/api.yml:24`,
`fern/apis/client/definition/api.yml:4`). Stable import paths and behaviour are
documented inline via AGENTS.md (`AGENTS.md:21-39`, `web/AGENTS.md:25-31`),
which distinguishes "Public API contracts" as a hand-edit boundary. The
overall surface is mature, observable, contract-tested, and version-aware, but
not free of accidental surface area or migration friction.

## Rating

**8/10 — Clear model with explicit interfaces, multiple version families,
operational safeguards, but with versioning sprawl and a few ad-hoc
controllers.**

Rationale: routes are uniformly wrapped, contracts are strict Zod, errors are
normalised through one middleware, deprecations and version migrations are
codified, rate-limit upgrade paths are first-class, and a separate unstable
namespace enforces an evolution contract. The deductions are for surface
inconsistencies (legacy OTel traces + ingestion.ts hand-rolled rather than via
the standard wrapper, ad-hoc SCIM responses bypassing the contract wrapper,
`z.any()` schemas in OTel endpoints, sprawling file count under
`pages/api/public/**`) and the cumulative discoverability cost of three
parallel route families (`/api/public`, `/api/public/v2`, `/api/public/v3`,
`/api/public/unstable`, `/api/public/otel/v1`, `/api/public/otel/otlp-proto`)
without a single generated index table.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Project-wide layout & dep direction | `AGENTS.md` declares `web` exposes public REST, worker consumes shared, `@langfuse/shared` is dependency leaf. | `AGENTS.md:21-39` |
| Public REST routes inventory | Top-level `pages/api/public/` directory lists endpoints: `health.ts`, `ingestion.ts`, `events.ts`, `feedback.ts`, `prompts.ts`, `spans.ts`, `dataset-run-items.ts`, plus grouped subfolders (`traces/`, `scores/`, `prompts/`, `sessions/`, `datasets/`, `score-configs/`, `comments/`, `observations/`, `models/`, `projects/`, `media/`, `metrics/`, `integrations/blob-storage/`, `llm-connections/`, `mcp/`, `scim/`, `organizations/`, `annotation-queues/`, `slack/`, `v2/`, `v3/`, `unstable/`, `otel/`). | `web/src/pages/api/public/` (dir entries) |
| Public types co-located with handlers | Each public endpoint has a matching strict Zod contract file in `features/public-api/types/`. | `web/src/features/public-api/types/` (dir entries) |
| Central middleware for public routes | `withMiddlewares` wraps handlers, applies CORS, routes per HTTP verb, normalises errors (BaseError → status, ZodError → 400, ClickHouseResourceError → 422 with custom advice, Prisma exception → 500), and supports per-route `errorContract` and `clickHouseResourceErrorMessage`. | `web/src/features/public-api/server/withMiddlewares.ts:84-206` |
| Stable route wrapper | `createAuthedProjectAPIRoute` is the single point that authenticates (Basic/Bearer + optional admin API key for self-hosted), rate-limits, parses Zod query/body, optionally rejects in v4 `events_only` mode, stamps `_deprecation`, attaches OpenTelemetry context, and serialises the response. | `web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:307-492` |
| Route config is type-safe | Generics `TQuery extends ZodType`, `TBody extends ZodType`, `TResponse extends ZodType` infer handler arg types; `responseSchema.safeParse` runs in dev only to catch drift. | `web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:43-100, 466-471` |
| Authentication service | `ApiAuthService.verifyAuthHeaderAndReturnScope` resolves Basic/Bearer, hashes + verifies, enforces in-app agent key isolation, populates plan / org / project / rate-limit override scope. | `web/src/features/public-api/server/apiAuth.ts:35-291` |
| Admin API key auth (self-hosted only) | Bearer + `x-langfuse-admin-api-key` + `x-langfuse-project-id` triplet, gated by `env.NEXT_PUBLIC_LANGFUSE_CLOUD_REGION`. | `web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:172-259` |
| Rate limit service | `RateLimitService` is a Redis-backed singleton with explicit "fail open if Redis missing" semantics; resources include `public-api`, `public-api-legacy`, `public-api-metrics`, `ingestion`. | `web/src/features/public-api/server/RateLimitService.ts:52-100` |
| Rate-limit upgrade paths | `legacyPublicApiRateLimitUpgradePaths` maps every v1 family to its v2/v3 replacement endpoint and docs URL. | `web/src/features/public-api/server/rateLimitUpgradePaths.ts:10-47` |
| Deprecation stamping on responses | `attachDeprecation` adds `_deprecation: ApiDeprecationInfo` to object responses only; arrays/primitives pass through. | `web/src/features/public-api/server/deprecations.ts:99-114` |
| Family-level deprecation constants | `OBSERVATIONS_V1_DEPRECATION`, `TRACES_DEPRECATION`, `SESSIONS_DEPRECATION`, `SCORES_DEPRECATION`, `METRICS_DEPRECATION`, `DATASET_RUN_ITEMS_DEPRECATION`, `DATASET_RUNS_DEPRECATION`, all share the same `V3_SUNSET_DATE = "2026-11-16"`. | `web/src/features/public-api/server/deprecations.ts:11-97` |
| Fern → OpenAPI deprecation sync | Script reads Fern YAMLs, detects `availability.status: deprecated`, stamps `deprecated: true` + description into generated OpenAPI. | `web/scripts/openapi/sync-deprecations.ts:1-25`; `web/scripts/openapi/fern-deprecations.ts:71-114` |
| OpenAPI regeneration | `pnpm run openapi:export` runs Fern for `server`, `client`, `organizations`, then runs the sync-deprecations post-step. | `package.json:41` |
| Generated artefacts (must not hand-edit) | `web/public/generated/api/openapi.yml`, `…/api-client/openapi.yml`, `…/organizations-api/openapi.yml`. | `web/public/generated/{api,api-client,organizations-api}/openapi.yml` |
| Fern API root, Basic auth, base path | `name: langfuse`, `auth: basic`, `base-path: /api/public`, error-discrimination by status-code, custom `X-Langfuse-Sdk-*` headers accepted. | `fern/apis/server/definition/api.yml:1-28` |
| Health endpoint contract | `service.auth: false`, `GET /api/public/health`, returns `{version, status}`, 503 on failure. | `fern/apis/server/definition/health.yml:1-28`; `web/src/pages/api/public/health.ts:14-48` |
| Observations v2 contract | Cursor-based pagination, `fields` field-group selection, `expandMetadata`, `isRootObservation`, 10 field groups documented inline. | `fern/apis/server/definition/observations.yml:1-100`; `web/src/pages/api/public/v2/observations/index.ts:14-93` |
| Fern `availability: deprecated` markers | Used by `trace.yml` (both `get` and `list`), `sessions.yml`, `legacy/metrics-v1.yml`, `legacy/observations-v1.yml`, `legacy/score-v1.yml`, `scores.yml`, `datasets.yml`, `dataset-run-items.yml`, `ingestion.yml`, `scim.yml`. | `fern/apis/server/definition/trace.yml:9-12, 36-39` (and 14 other files) |
| Unstable namespace contract | `/api/public/unstable/evaluators` is its own service definition with explicit `unstable` evolution note. | `fern/apis/server/definition/unstable/evaluators.yml:1-56` |
| Unstable error contract | Distinct error code enum (`authentication_failed`, `invalid_query`, `evaluator_in_use`, `evaluator_preflight_failed`, `unprocessable_content`, `rate_limited`, `conflict`, etc.) bound by Zod with strict `.strict()`. | `web/src/features/public-api/shared/unstable-public-api-error-schema.ts:3-68` |
| Unstable error mapping | Central `toUnstablePublicApiError` converts every internal error class to the unstable envelope; rate-limit carries `retryAfterSeconds`, `limit`, `remaining`, `resetAt`. | `web/src/features/public-api/server/unstable-public-api-error-contract.ts:160-262` |
| Routes that opt into unstable contract | `unstable/evaluators/{index,[evaluatorId]}.ts`, `unstable/evaluation-rules/...`, `unstable/dashboards/...`, `unstable/dashboard-widgets/...`. | `web/src/pages/api/public/unstable/evaluators/[evaluatorId].ts:1-43` |
| Unstable contract types | `unstable-public-evals-contract.ts` owns shared building blocks (filter factories, mapping schemas, evaluator union, pagination) reused by both `evaluators` and `evaluation-rules`. | `web/src/features/public-api/types/unstable-public-evals-contract.ts:1-362` |
| Unstable contract type surface | Discriminated `APIEvaluator` (`type: llm_as_judge \| code`), per-type POST body via `discriminatedUnion("type", …)` plus `z.preprocess` to default `type=llm_as_judge` for back-compat. | `web/src/features/public-api/types/unstable-evaluators.ts:30-122` |
| CLI commands | `infra:dev:up`, `db:generate`, `db:migrate`, `openapi:export`, `release`, `release:cloud`; only `openapi:export` and `release*` touch the public contract. | `package.json:10-46` |
| Native batch ingestion | Single POST handler, in-process validation + S3 upload + BullMQ enqueue, with `LANGFUSE_MIGRATION_V4_WRITE_MODE=events_only` per-event rejection of trace/observation events. | `web/src/pages/api/public/ingestion.ts:55-309` |
| Event ingest (single) | `POST /api/public/events` writes an `OBSERVATION_CREATE` event with `type: EVENT`; also rejects in `events_only` mode. | `web/src/pages/api/public/events.ts:23-55` |
| Health check service | `runHealthCheck` optionally checks Postgres (`failIfDatabaseUnavailable`) and ClickHouse freshness (legacy `traces` or `events_core` depending on v4 mode). | `web/src/features/public-api/server/health-service.ts:22-80` |
| LLM connections CRUD | `GET/PUT /api/public/llm-connections` allow admin API key auth on self-hosted; explicitly select only safe fields and exclude `secretKey`/`extraHeaders`. | `web/src/pages/api/public/llm-connections/index.ts:22-183` |
| SCIM | Hand-rolled RFC 7644 responses (`schemas: urn:ietf:params:scim:...`) with entitlement gate (`admin-api`); bypasses `createAuthedProjectAPIRoute`/Zod. | `web/src/pages/api/public/scim/Users/index.ts:1-313` |
| MCP public surface | `web/src/pages/api/public/mcp/index.ts` exposes MCP over the public API; covered by `mcp-public-api-tools.servertest.ts`. | `web/src/pages/api/public/mcp/index.ts`; `web/src/__tests__/server/mcp-public-api-tools.servertest.ts` |
| OpenTelemetry ingestion | `POST /api/public/otel/v1/traces` and `/metrics`; JSON or `application/x-protobuf`, optional gzip; `bodyParser: false` (raw stream). | `web/src/pages/api/public/otel/v1/traces/index.ts:20-90`; `web/src/pages/api/public/otel/v1/metrics/index.ts` |
| OTel proto types | Static `web/src/pages/api/public/otel/otlp-proto/generated/root.ts` is compiled from `@opentelemetry/otlp-transformer` and explicitly marked do-not-touch. | `web/src/pages/api/public/otel/otlp-proto/README.md:1-8` |
| Organizations admin API (Fern) | Separate Fern API with `auth: bearer`, doc note "only available on self-hosted instances", requires `ADMIN_API_KEY`. | `fern/apis/organizations/definition/api.yml:1-31` |
| Client bearer API (Fern) | Separate Fern API with `auth: bearer` and `base-path: /api/public`; scoped to score creation only (so SDKs with limited-scope keys can write scores without full read). | `fern/apis/client/definition/api.yml:1-11`; `fern/apis/client/definition/score.yml:7-10` |
| Project/org API key management | `pages/api/public/projects/[projectId]/{apiKeys,memberships}/...` and `pages/api/public/organizations/{apiKeys,memberships,projects}/...`. | `web/src/pages/api/public/projects/[projectId]/` (dir entries); `web/src/pages/api/public/organizations/` (dir entries) |
| Test coverage of public surface | Server tests named after endpoints: `observation-api`, `observations-api-v2`, `metrics-api`, `metrics-v2-api`, `scores-api*`, `datasets-api`, `comments-api`, `experiments-public-api`, `mcp-public-api-tools`, `ingestion-api`, `otel-api`, `projects-api`, `organizations-api`, `memberships-api`, `scim-api`, `daily-metrics-api`, `llm-connections-api`, `blob-storage-integration-api`, `annotation-queue*-api`. | `web/src/__tests__/server/*.servertest.ts` |
| AGENTS contract | "Public API routes should use `src/features/public-api/server/withMiddlewares.ts`, define strict request and response types in `src/features/public-api/types/*`, add server tests, and update Fern sources when the contract changes." | `web/AGENTS.md:127-128` |
| AGENTS contract | "Public API contract changes must update Fern sources in `fern/apis/**` and regenerated outputs. Never hand-edit `generated/**`." | `AGENTS.md:96-99` |

## Answers to Dimension Questions

### 1. What is the intended public API surface?

Three families, each with its own contract source of truth and auth model:

- **Project REST API** at `/api/public/...` (Basic auth, project-scoped) —
  defined in `fern/apis/server/definition/*.yml` and serialised to
  `web/public/generated/api/openapi.yml`. Covers traces, observations (v1 + v2),
  scores (v1, v2, v3), sessions, datasets (v1 + v2), dataset items, dataset
  runs, prompts (v1 + v2), experiment items, experiments, metrics (v1 + v2),
  models, score configs, media, annotation queues, LLM connections, blob
  storage integrations, comments, projects (apiKeys, memberships), health,
  and feedback.
- **Organization admin REST API** at `/api/public/organizations/...` (Bearer
  `ADMIN_API_KEY`, self-hosted only) — defined in
  `fern/apis/organizations/definition/api.yml` and serialised to
  `web/public/generated/organizations-api/openapi.yml`. Covers organization
  apiKeys, memberships, and projects.
- **Client bearer REST API** at `/api/public/...` (Bearer public key only,
  limited to score creation) — defined in `fern/apis/client/definition/*.yml`
  and serialised to `web/public/generated/api-client/openapi.yml`. This is the
  reduced-scope surface the Python/JS SDKs use when shipped with a public-key
  only credential.

In addition, three non-Fern ingestion endpoints exist for SDK and OTel data:
`POST /api/public/ingestion` (batch Langfuse events),
`POST /api/public/events` (single observation event), and
`POST /api/public/otel/{v1,otlp-proto}/traces|metrics` (OTLP/JSON+protobuf
with optional gzip). They are deliberately separate from the Fern surface
because the Fern generation does not yet model them.

The `unstable/{evaluators,evaluation-rules,dashboards,dashboard-widgets}`
namespace sits underneath `/api/public/unstable/...` and uses a distinct,
machine-readable error envelope. It is documented in
`fern/apis/server/definition/unstable/*.yml` and stitched into the same
generated OpenAPI as the server API.

Finally, two non-Fern admin surfaces remain:
- `SCIM` (`/api/public/scim/Users`, `/Schemas`, `/ServiceProviderConfig`,
  `/ResourceTypes`) for IdP-driven user provisioning.
- `Slack` (`/api/public/slack/install`, `/api/public/slack/oauth`) for the
  Slack integration.

No public CLI commands exist in `package.json` — the only CLI scripts target
infrastructure (`infra:dev:up`) and DB lifecycle (`db:generate`, `db:migrate`,
`db:seed`). `openapi:export`, `release`, and `release:cloud` are
maintenance/release scripts, not user-facing commands.

### 2. Is the stable API easy to distinguish from internal implementation details?

Yes, with one caveat. The directory layout itself (`web/src/pages/api/public/`)
is the stability boundary, and AGENTS.md codifies this rule
(`web/AGENTS.md:127-128`, `AGENTS.md:96-99`). Stable endpoints always go
through `withMiddlewares` + `createAuthedProjectAPIRoute`, which forces:

- A Zod-typed `querySchema` and `bodySchema` (enforced at runtime).
- A Zod-typed `responseSchema` (enforced in dev to catch handler drift;
  see `createAuthedProjectAPIRoute.ts:466-471`).
- A route `name` and `rateLimitResource` namespace.
- Optional `deprecation`, `rejectInEventsOnlyMode`, `allowInAppAgentKey`,
  `isAdminApiKeyAuthAllowed`, and `errorContract` flags.

The unstable contract is unambiguously labelled by the `/api/public/unstable/`
URL prefix, by the `unstable-evaluators.ts`/`unstable-evaluation-rules.ts`
type file names, and by the `unstable-public-evals-contract.ts` barrel
re-exported under `features/public-api/types/unstable-public-evals-contract.ts:1-27`.

The caveats are:
- Some endpoints (`ingestion.ts`, `events.ts`, `otel/v1/traces/index.ts`,
  `otel/v1/metrics/index.ts`, all `scim/Users/*.ts`) bypass the wrapper and
  roll their own auth + error handling. SCIM in particular hand-builds
  RFC 7644 JSON envelopes instead of Zod schemas.
- The wrapper itself imports from `@/src/...` and `@langfuse/shared/src/server`,
  not from `@langfuse/shared`, so the API module surface is not actually
  reusable outside the `web` package; stability is enforced by convention +
  AGENTS.md rather than by the package export map.
- `web/src/pages/api/public/otel/otlp-proto/generated/root.ts` is committed
  generated code; the README warns "Unless there are relevant updates to
  the OpenTelemetry specification, there should be no need to ever touch
  this" (`web/src/pages/api/public/otel/otlp-proto/README.md:7-8`), but it
  is not regenerated by `openapi:export`.

### 3. Does the API expose the right level of abstraction for agent harness users?

For application developers (ingestion + read of traces/observations/scores)
the abstraction is excellent: the v2 observations API at
`/api/public/v2/observations` introduces field-group selection (`fields=core,usage,model`),
metadata expansion (`expandMetadata=trace_id,user_id`), cursor-based pagination,
`isRootObservation`, and optional `useEventsTable` reads — all the things an
agent harness needs to do high-volume, low-latency reads. The v3 scores API
at `/api/public/v3/scores` removes the trace JOIN requirement and adds
field selection. Both were designed as drops-in replacements for the v1
endpoints with explicit rate-limit upgrade guidance
(`rateLimitUpgradePaths.ts:23-46`).

For extension authors (eval/dashboard builders) the abstraction is also good:
the unstable namespace enforces a stable, machine-readable error contract so
agents can retry on `evaluator_preflight_failed` without parsing free-form
messages (`unstable-public-api-error-schema.ts:3-23`,
`unstable-evaluators.ts:30-122`).

Where the abstraction leaks: the v1 endpoints (`/api/public/traces`,
`/api/public/observations`, `/api/public/sessions`, `/api/public/metrics`,
`/api/public/scores`, `/api/public/dataset-run-items`) still expose the
legacy traces/observations ClickHouse tables, and their `rejectInEventsOnlyMode`
flag (`createAuthedProjectAPIRoute.ts:82-88, 324-338`) returns a `404` only
in v4 events-only deployments. An agent that reads a v1 endpoint in a
self-hosted v3 instance still gets the full response, so the deprecation is
behaviour-gated rather than runtime-gated. The `_deprecation` envelope
provides the machine-readable signal for migration.

### 4. Are examples sufficient to use the API correctly without reading internals?

Yes for v2/v3/unstable. The unstable evaluator endpoint ships with a full
worked example (a `POST /api/public/unstable/evaluators` request body) plus
recovery guidance inline (`unstable/evaluators.yml:11-55`). The v2
observations endpoint documents every field group inline
(`observations.yml:10-31`). The OpenAPI descriptions under
`web/public/generated/api/openapi.yml` are derived from these Fern `docs:`
strings and therefore inherit the same richness.

Less consistent for the legacy v1 endpoints — `trace.yml`, `observations.yml`,
and `scores.yml` define `availability: deprecated` plus a `message` but no
full request/response example. The `_deprecation` envelope tells callers
what to migrate to, but does not show what a v2 GET looks like. New
integrations still need to read the v2 Fern file to see an example.

Score creation in the bearer-only client API does include inline `examples:`
(`fern/apis/client/definition/score.yml:38-40`), so SDK maintainers reading
the client OpenAPI get a runnable shape.

## Architectural Decisions

1. **One wrapper, one auth model.** Every project-scoped read/write route
   flows through `withMiddlewares` + `createAuthedProjectAPIRoute`. The
   wrapper is the single point that wires CORS, auth, rate-limit, Zod
   validation, deprecation stamping, and OpenTelemetry context. This means
   changing auth semantics, error envelope, or rate-limit headers happens in
   exactly two files (`withMiddlewares.ts:84-206`,
   `createAuthedProjectAPIRoute.ts:307-492`).
2. **Three auth modes bound to three Fern APIs.** Basic (project keys),
   Bearer (limited-scope client/score keys, `accessLevel: "scores"`),
   admin Bearer (`ADMIN_API_KEY`, self-hosted only). Each has its own Fern
   definition (`fern/apis/server/definition/api.yml:16`,
   `fern/apis/client/definition/api.yml:4`,
   `fern/apis/organizations/definition/api.yml:24`).
3. **OpenAPI is generated, not handwritten.** `pnpm run openapi:export`
   drives `fern-api export` for all three APIs and post-processes
   `web/public/generated/api/openapi.yml` with the deprecation sync script
   (`package.json:41`, `web/scripts/openapi/sync-deprecations.ts:1-25`).
   Generated files are explicitly off-limits to hand edits
   (`AGENTS.md:96-99`).
4. **Versioned route families are explicit.** `/api/public/v2/observations`,
   `/api/public/v2/metrics`, `/api/public/v2/datasets`, `/api/public/v2/scores`,
   plus the legacy `/api/public/v3/scores`, sit beside the v1 surfaces rather
   than replacing them. Deprecations are version-aware (`SCORES_DEPRECATION`
   points at `v3`, `OBSERVATIONS_V1_DEPRECATION` points at `v2`,
   `deprecations.ts:11-97`).
5. **An "unstable" namespace carries an evolved error contract.** Unstable
   routes opt into `unstablePublicEvalsErrorContract`
   (`unstable-public-api-error-contract.ts:24-25`), get typed
   `UnstablePublicApiError` responses with structured `details.issues`,
   `retryAfterSeconds`, `variable`, etc., and are documented as "may evolve
   while the underlying evaluation data model is being redesigned"
   (`fern/apis/server/definition/unstable/evaluators.yml:41-42`).
6. **Rate-limit upgrade paths are first-class.** Every legacy route carries
   a `rateLimitUpgradePath` that points to the v2/v3 replacement and a docs
   URL, and the `RateLimitResponse.sendRestResponseIfLimited` machinery
   propagates that path into the 429 response
   (`web/src/features/public-api/server/RateLimitService.ts:391-396`,
   `rateLimitUpgradePaths.ts:10-47`).
7. **Ingestion is decoupled from reads.** `POST /api/public/ingestion`,
   `POST /api/public/events`, and the OTel endpoints share only the
   `processEventBatch` and `createIngestionAttribution` helpers from
   `@langfuse/shared/src/server`, then enqueue to BullMQ for the worker
   (`web/src/pages/api/public/ingestion.ts:14, 176-178`). Read endpoints hit
   ClickHouse directly via repository modules in `@langfuse/shared/src/server`.
8. **Strict Zod everywhere, plus dev-mode response validation.** Every
   contract type uses `.strict()` on object schemas
   (`web/src/features/public-api/types/unstable-evaluators.ts:28, 35, 42, 56`,
   `web/src/features/public-api/types/scores.ts:1-6` re-exports shared
   schemas), and `createAuthedProjectAPIRoute` runs
   `responseSchema.safeParse` only when `NODE_ENV === "development"` to avoid
   production overhead while still surfacing handler drift in dev
   (`createAuthedProjectAPIRoute.ts:466-471`).
9. **Deprecation is two-sided.** Fern source has `availability.status: deprecated`
   plus a `message`; the OpenAPI sync writes `deprecated: true` and the
   description. The web runtime stamps `_deprecation` on the response body
   and short-circuits with 404 in v4 `events_only` mode when the handler
   reads from tables no longer populated.
10. **ClickHouse resource errors carry upgrade advice.**
    `LEGACY_PUBLIC_API_OBSERVATIONS_CLICKHOUSE_RESOURCE_ERROR_MESSAGE` and
    `LEGACY_PUBLIC_API_METRICS_CLICKHOUSE_RESOURCE_ERROR_MESSAGE`
    (`withMiddlewares.ts:47-60`) tell callers exactly which v2 endpoint to
    switch to when ClickHouse returns a resource error.

## Notable Patterns

- **Pluggable auth via `allowedAccessLevels`.** `createAuthedProjectAPIRoute`
  accepts `["project", "scores"]` so the same route factory serves both
  full-key and limited-scope flows; `ScoresApiService` then enforces that a
  `scores`-scoped key cannot pass project-only checks
  (`createAuthedProjectAPIRoute.ts:70-75`, `apiAuth.ts:209-253`).
- **Resource-attached deprecation.** The `_deprecation` object lives on the
  JSON response (not in headers) so SDKs can read structured `replacement`,
  `docsUrl`, and `sunsetAt` fields directly off the body.
- **Discriminated unions for type-safe request bodies.** The unstable
  evaluator POST body is a `z.discriminatedUnion("type", [...])` plus a
  `z.preprocess` that defaults `type: llm_as_judge` for backwards
  compatibility — explicit, strict, yet forward-compatible
  (`unstable-evaluators.ts:99-115`).
- **Field-group selection.** v2 observations lets the caller say
  `fields=core,basic,usage,model` and only the listed columns come back,
  trading a larger `data` array for a smaller wire footprint
  (`observations.yml:39-44`).
- **Cursor-based pagination with `encodeCursor`/`decodeCursor`.** v2 endpoints
  return an opaque cursor; v3 scores uses a typed `EncodedScoresCursorV3`
  schema (`web/src/features/public-api/types/scores.ts:1-5`,
  `web/src/pages/api/public/v2/observations/index.ts:77-84`).
- **Filter schema factories.** The unstable contract centralises 10 filter
  types (datetime, string, number, stringOptions, categoryOptions,
  arrayOptions, stringObject, numberObject, boolean, null) and builds
  target-specific filter unions from column lists
  (`unstable-public-evals-contract.ts:201-244`).
- **Strict Zod with `.loose()` for read paths.** `UnstablePublicApiValidationIssue`
  uses `.loose()` so upstream Zod version drift does not blow up consumers
  (`unstable-public-api-error-schema.ts:27-33`).
- **Cross-runtime dual import.** `ApiAuthService` is in `web/`, but the
  hash/verify/cache primitives come from `@langfuse/shared/src/server`,
  which is also consumed by the worker. This keeps the security-relevant
  crypto in one place while leaving the route glue in `web/`
  (`apiAuth.ts:1-19, 320-413`).

## Tradeoffs

- **Wrapper discipline is enforced socially.** AGENTS.md and the public-API
  route playbook tell contributors to use `withMiddlewares` and define
  strict types, but SCIM, OTel traces, native ingestion, and OTel metrics
  bypass it. The codebase accepts this cost because those endpoints have
  wire formats (SCIM RFC 7644, OTel OTLP, multi-content-type binary) that
  do not map cleanly to Zod; the wrapper would have to grow special cases
  to handle them.
- **Strictness is dev-only on the response path.** Validating responses in
  dev catches handler drift but costs nothing in prod. The cost is that
  prod regressions only show up via end-to-end tests, which depend on a
  running web server (`web/AGENTS.md:124-127`).
- **Three parallel route trees.** The `/api/public/v2/*`, `/api/public/v3/*`,
  and `/api/public/unstable/*` trees duplicate the v1 patterns because
  Next.js Pages Router cannot route different versions of the same logical
  endpoint through one handler. The trade is fewer cross-cutting changes
  (each v2 handler is small and self-contained) at the cost of file
  proliferation and a discoverability gap for newcomers.
- **Deprecation sunset is tied to a Langfuse version, not the endpoint.**
  `V3_SUNSET_DATE = "2026-11-16"` refers to Langfuse Cloud v3 → v4 migration,
  not to API version 3 (`deprecations.ts:11-16`). Self-hosted deployments
  ignore the date until they upgrade, and v3 scores are unaffected.
  This is documented in the Fern `availability.message` strings but is a
  recurring source of confusion.
- **OTel ingest uses `z.any()` for query/body schemas.** Because OTel
  payloads vary by SDK, `otel/v1/traces/index.ts:29-30` and the metrics
  counterpart set `querySchema: z.any(), responseSchema: z.any()`. The
  upstream gateway logic in `@opentelemetry/otlp-transformer` does the
  actual validation, so the route factory accepts whatever the gateway
  validates. This loses the Zod safety net for one of the most important
  ingestion paths.
- **OpenAPI regeneration is opt-in.** `pnpm run openapi:export` must be
  run manually; CI does not appear to enforce that generated files are
  in sync with Fern sources beyond linting the Fern YAMLs.

## Failure Modes / Edge Cases

- **JSON body too large.** `createAuthedProjectAPIRoute` catches
  `RangeError("Invalid string length")` from `res.json` and throws
  `PayloadTooLargeError` (`createAuthedProjectAPIRoute.ts:40-41, 481-488`).
- **ClickHouse resource limits.** Both ingest and read surfaces convert
  `ClickHouseResourceError` to either a `422` with the docs URL
  (`withMiddlewares.ts:42-45, 117-139`) or to the unstable `422
  unprocessable_content` envelope (`unstable-public-api-error-contract.ts:230-239`).
- **Lua/Redis missing.** `RateLimitService` fails open with a logger warning
  (`RateLimitService.ts:93-96`).
- **Ingestion suspended.** Cloud-only `isIngestionSuspended` flag (per
  `apiAuth.ts:201, 247-249`) returns `403` from `POST /api/public/ingestion`
  and from OTel traces.
- **v4 events-only migration.** `/api/public/ingestion`,
  `/api/public/observations`, `/api/public/traces`, `/api/public/sessions`,
  `/api/public/scores`, `/api/public/metrics`, `/api/public/dataset-run-items`,
  `/api/public/events`, `/api/public/dataset-run-items` short-circuit
  (in-process or per-event) when `LANGFUSE_MIGRATION_V4_WRITE_MODE=events_only`
  so legacy table reads return stale/empty data
  (`createAuthedProjectAPIRoute.ts:82-88, 324-338`,
  `ingestion.ts:154-174, 262-308`).
- **In-app agent key isolation.** `ApiAuthService.verifyAuthHeaderAndReturnScope`
  refuses `isInAppAgentKey=true` keys unless the route explicitly opts in
  via `allowInAppAgentKey: true` (`apiAuth.ts:278-288`,
  `createAuthedProjectAPIRoute.ts:77-80, 350`).
- **Admin API key theft on Cloud.** Cloud deployments reject admin API key
  auth entirely (`createAuthedProjectAPIRoute.ts:185-192`).
- **SCIM password smuggling.** The SCIM Users POST handler accepts and
  silently ignores `password` to avoid creating a usable credential via an
  org-scoped key (`web/src/pages/api/public/scim/Users/index.ts:175-184`).
- **OpenAPI ↔ Fern drift.** A drift test (`openapi-deprecations.servertest.ts`,
  referenced in `deprecations.ts:10`) fails the build if the V3 sunset date
  drifts between Fern availability messages, the `V3_NOTICE` constant, and
  the `V3_SUNSET_DATE` literal.

## Future Considerations

- **`web/src/pages/api/public/otel/otlp-proto/generated/root.ts`** is
  committed generated code outside the `pnpm run openapi:export` pipeline;
  upgrading `@opentelemetry/otlp-transformer` requires manual re-import of
  the protobuf definitions. Worth wiring into the openapi:export script or
  the `pnpm install` postinstall hook.
- **MCP public endpoint** (`web/src/pages/api/public/mcp/index.ts`) is the
  newest agent-facing surface and is only covered by one test
  (`mcp-public-api-tools.servertest.ts`); as more MCP tools ship, the
  tool registry and rate-limit interaction need explicit policy.
- **v1 endpoint sunset.** The Fern `availability.status: deprecated`
  markers across `trace.yml`, `legacy/observations-v1.yml`,
  `legacy/score-v1.yml`, `legacy/metrics-v1.yml`, `sessions.yml`,
  `datasets.yml`, `dataset-run-items.yml`, `scores.yml`, and `ingestion.yml`
  all cite the same `2026-11-16` date; removing them after that date will
  require sweeping both Fern YAMLs and the web handler config.
- **Org/scope key model.** `allowedAccessLevels` currently supports
  `["project"]` and `["project", "scores"]`. The recent `isInAppAgentKey`
  gate suggests more access-level variants will land; the
  `RouteAccessLevel = Exclude<ApiAccessLevel, "organization">` type
  (`createAuthedProjectAPIRoute.ts:36`) will need to evolve with them.
- **Strict response validation in production.** The dev-only safety net in
  `createAuthedProjectAPIRoute.ts:466-471` could be promoted to a periodic
  background contract check (e.g. nightly snapshot diff) so prod drift
  surfaces without an HTTP round-trip.
- **Wrapper coverage.** SCIM, OTel traces/metrics, native ingestion, and
  events all bypass `createAuthedProjectAPIRoute`. If the unstable envelope
  becomes the long-term error contract, a generic "binary ingest" route
  factory with the same auth/rate-limit semantics would close the
  consistency gap.

## Questions / Gaps

- No single generated index (e.g. a manifest of route → version → auth
  → deprecation status) exists. A new integrator must read both
  `fern/apis/server/definition/*.yml` and `web/src/pages/api/public/**` to
  know which endpoints are public, which are versioned, and which are
  unstable. The Fern-to-OpenAPI flow partially addresses this for docs,
  but no machine-readable route manifest is exported from the build.
- The MCP public endpoint is documented by tests only; no Fern YAML
  describes it, so it is invisible to the OpenAPI reference and any client
  codegen.
- No policy is documented for what counts as "unstable" versus "experimental"
  versus "internal preview"; the codebase uses "unstable" as both the URL
  prefix and the error contract name, but the criteria for graduating out
  of unstable are not codified.
- `web/src/pages/api/public/otel/otlp-proto/generated/root.ts` is generated
  but not regenerated by any script in `package.json`; its lifecycle is
  hand-managed.
- The "client" Fern API at `fern/apis/client/definition/api.yml` declares
  `base-path: /api/public`, but the runtime endpoints (e.g. score creation)
  also live at `/api/public/scores`, so the bearer-only and basic-auth flows
  share the URL space and are separated only by auth header. This is
  correct but easy to miss when integrating.
- `AGENTS.md` does not call out the SCIM or Slack subdirectories as
  intentionally hand-rolled, so a future contributor might "fix" them by
  retrofitting the public-API wrapper and break the SCIM/OAuth wire
  format.
