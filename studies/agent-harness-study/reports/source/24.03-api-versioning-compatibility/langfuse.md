# Source Analysis: langfuse

## 24.03 API Versioning and Compatibility

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript (Next.js / tRPC / Prisma / ClickHouse), Fern API definitions, pnpm/Turbo monorepo |
| Analyzed | 2026-08-27 |

## Summary

Langfuse uses a monolithic app version (`4.16.0`) plus URL-path API versioning (`/v2/`, `/v3/`, `/otel/v1/`, `/unstable/`) with Fern (`studies/agent-harness-study/sources/langfuse/fern/fern.config.json:3`) as the source of truth. Breaking changes are staged as `availability: deprecated` in Fern YAML with a single Cloud sunset date (`2026-11-16`), mirrored into OpenAPI via a stamping pipeline and into runtime JSON via a `_deprecation` envelope that is only emitted on Cloud. V4 write-mode gating (`legacy`/`dual`/`events_only` via `LANGFUSE_MIGRATION_V4_WRITE_MODE`) provides capability-based rejection of legacy reads when the underlying ClickHouse tables are no longer populated. The model is explicit and tested, but relies on manual message synchronization, lacks header-based negotiation, and leaves SDK/ingestion compatibility to additive optional fields rather than formal contract versioning.

## Rating

**7 / 10** — Clear, operationalized model: Fern-declared deprecations with enforced sunset-date/message parity, machine-readable `_deprecation` signals stamped both in OpenAPI descriptions and live JSON responses, path-versioned successors (`v2/observations`, `v3/scores`, `otel/v1/traces`), and deployment-aware capability gates. Backed by executable tests for deprecation-signal correctness and OpenAPI↔Fern drift. Downgraded from 9 because compatibility beyond the deprecation surface (queue payload evolution, ClickHouse schema, SDK wire format) depends on additive optional fields and deployment flags without formal version fields or exhaustive backwards-compatibility suites, and because unstable vs stable guarantees are path-conventional rather than contract-enforced.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| App version single source | Monorepo version `4.16.0` and `release-it` bumper targets for shared/web/worker `VERSION.ts` and package.json | `package.json:3`, `package.json:61-95`, `packages/shared/src/constants/VERSION.ts:1`, `web/src/constants/VERSION.ts:1` |
| API definition source of truth | Fern organization/version and generators for Python/TS SDKs from Fern definitions | `fern/fern.config.json:2-4`, `fern/apis/server/generators.yml:14-40` |
| Path versioning — stable successors | v2 observations (cursor+field-groups), v3 scores (polymorphic value + field groups), otel v1 traces (OTLP), unstable evaluators/dashboards | `fern/apis/server/definition/observations.yml:36`, `fern/apis/server/definition/scores-v3.yml:30`, `fern/apis/server/definition/opentelemetry.yml:25`, `fern/apis/server/definition/unstable/evaluators.yml:9`, `fern/apis/server/definition/unstable/dashboards.yml:9` |
| Legacy endpoints marked deprecated | 16+ endpoints with `availability: status: deprecated` and Cloud-scoped sunset message naming v4 replacement | `fern/apis/server/definition/ingestion.yml:11-12`, `fern/apis/server/definition/trace.yml:11-12,38-39`, `fern/apis/server/definition/scores.yml:17-18,98-99`, `fern/apis/server/definition/sessions.yml:11-12`, `fern/apis/server/definition/legacy/observations-v1.yml:11-12`, `fern/apis/server/definition/datasets.yml:38-39` |
| Deprecation envelope schema | Shared Zod schema `deprecationResponseZod` with `message`, `replacement`, `docsUrl`, `sunsetAt` (optional fields omitted when empty) | `packages/shared/src/utils/zod.ts:165-172` |
| Centralized sunset/notice constants | Single source of truth `V3_SUNSET_DATE="2026-11-16"`, `V3_NOTICE`, `REPLACEMENT` and `DOCS` maps, exported deprecation objects | `web/src/features/public-api/server/deprecations.ts:11-97` |
| Runtime stamping mechanism | `createAuthedProjectAPIRoute` `deprecation` config and `attachDeprecation()` that adds top-level `_deprecation` only when `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` is set; bypasses non-object bodies; `rejectInEventsOnlyMode` → 404 with deprecation | `web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:89-90,316-318,329-338,482` |
| Depot helper | `attachDeprecation` implementation — non-object/array pass-through, spread into `{...body, _deprecation}` | `web/src/features/public-api/server/deprecations.ts:102-114` |
| OpenAPI generation + stamping | `openapi:export` runs `fern-api export` for three APIs plus `tsx sync-deprecations.ts`; stamping reads Fern `availability` and writes `deprecated:true` + `**Deprecated:**` notice | `package.json:41`, `web/scripts/openapi/sync-deprecations.ts:5-19`, `web/scripts/openapi/stamp-deprecations.ts:64-120`, `web/scripts/openapi/fern-deprecations.ts:71-115` |
| Zod response types carry `_deprecation` | Legacy response schemas extend with `_deprecation: deprecationResponseZod.optional()` | `web/src/features/public-api/types/traces.ts:94,112`, `web/src/features/public-api/types/observations.ts:241,253`, `web/src/features/public-api/types/sessions.ts:39,49`, `fern/apis/server/definition/commons.yml:5-17` |
| V4 write-mode capability env | `LANGFUSE_MIGRATION_V4_WRITE_MODE` enum `legacy|dual|events_only` default `events_only`, helpers `areLegacyWritesActive`/`areEnrichedWritesActive` | `packages/shared/src/env.ts:328-330`, `packages/shared/src/features/analytics-integrations/export-source-policy.ts:153-165` |
| Capability-gated export sources | Per-mode allow/block with deployment-agnostic capability messages for legacy vs enriched sources | `packages/shared/src/features/analytics-integrations/export-source-policy.ts:21-25,202,221`, `packages/shared/src/features/analytics-integrations/export-source-policy.test.ts:120-252` |
| Legacy-read rejection | `rejectInEventsOnlyMode: true` on 20+ routes reading legacy traces/observations/dataset_run_items | `web/src/pages/api/public/traces/index.ts:46`, `web/src/pages/api/public/observations/index.ts:35`, `web/src/pages/api/public/v2/scores/index.ts:18`, `web/src/pages/api/public/sessions/index.ts:19` (representative) |
| Ingestion version negotiation (header) | `x-langfuse-ingestion-version` header read, set as span attribute, rejected if `>4`; lower versions use dual write path | `web/src/pages/api/public/otel/v1/traces/index.ts:261-283` |
| Queue payload evolution via optional fields | Additive optional fields with comments about rolling-deploy / in-flight job compatibility (`ingestionApiKey`, `bucketPrefix`, `ingestionVersion`, `sdkName`, `isLangfuseInternal`) | `packages/shared/src/server/queues.ts:30-40,52-77` |
| SDK version capability table | Minimum semver per language for `appRootObservations`, `experimentRunner`, `experimentLinkDeprecation` used to decide `compatible`/`upgrade_required` | `web/src/features/sdk-version/lib/sdkVersionCapabilities.ts:24-37,76-108` |
| Schema evolution strategy — Prisma | Explicit migration workflow via `prisma/schema.prisma` + `prisma/migrations/*` (playbook documented) | `packages/shared/AGENTS.md:31-40` (references `packages/shared/prisma/schema.prisma`) |
| Observability of deprecation | Health/ready endpoints expose app `VERSION`, ingestion failures tracked via `markProjectIngestFailure` | `web/src/pages/api/public/health.ts:38-45`, `web/src/pages/api/public/ready.ts:22-35`, `web/src/pages/api/public/otel/v1/traces/index.ts:313-317` |
| Backwards-compat tests — deprecation signal | 8 tests assert `_deprecation` shape, `replacement`, `docsUrl`, `sunsetAt` per endpoint family | `web/src/__tests__/server/deprecation-signal.servertest.ts:40-211` |
| Backwards-compat tests — OpenAPI drift | 10 tests prove OpenAPI deprecated ops == Fern deprecated ops, message contains Cloud scope + sunset, stamping idempotent | `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:96-314` |
| Scores v2→v3 migration test | Dedicated suite for v3 scores field-groups, cursor pagination, type-specific value handling | `web/src/__tests__/server/scores-api-v3.servertest.ts:1` (suite exists; spot-checked) |

## Answers to Dimension Questions

**1. Which APIs are stable, experimental, deprecated, or internal?**

*Stable:* Path-versioned public REST under `fern/apis/server/definition/` not marked deprecated — e.g. `GET /api/public/v2/observations` (`fern/apis/server/definition/observations.yml:36`), `GET /api/public/v3/scores` (`fern/apis/server/definition/scores-v3.yml:30`), `POST /api/public/otel/v1/traces` (`fern/apis/server/definition/opentelemetry.yml:25`), plus non-deprecated `scores` create and other entity endpoints. Generated SDKs via `fern/apis/server/generators.yml:14-40` signal stability.

*Deprecated:* Every Fern endpoint with `availability: status: deprecated` — batch ingestion (`fern/apis/server/definition/ingestion.yml:11`), traces list/get (`fern/apis/server/definition/trace.yml:11,38`), observations v1, scores v2, sessions, dataset runs/run-items, metrics v1, etc. All share the same Cloud sunset `2026-11-16` and point to a v4 replacement (OTLP, v2 observations, v3 scores, experiments).

*Experimental/Unstable:* Anything under `base-path: /api/public/unstable` (`fern/apis/server/definition/unstable/evaluators.yml:9`, `.../dashboards.yml:9`). Docs explicitly state “may evolve while underlying evaluation data model is being redesigned.” No stability guarantee; version increments still bump `version` integer per evaluator but no deprecation wrapper.

*Internal:* Queue contracts in `packages/shared/src/server/queues.ts:395-435` (internal `QueueName`/`QueueJobs` enums), ClickHouse internals, and `packages/shared/src/constants/VERSION.ts:1` app version (not an API version). Internal telemetry events carry `isLangfuseInternal` (`packages/shared/src/server/queues.ts:72-77`) and are separated from public ingestion.

**2. How are users warned before breaking changes?**

Three layers, all sourced from Fern `availability.message`:

- *API reference:* `web/scripts/openapi/stamp-deprecations.ts:57-108` writes `deprecated: true` and prepends `**Deprecated:** {message} See the [Langfuse v3 to v4 upgrade guide](https://langfuse.com/self-hosting/upgrade/upgrade-guides/upgrade-v3-to-v4)` to the OpenAPI description for each deprecated operation. `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:112-122` proves the message is present verbatim.

- *Runtime response:* `web/src/features/public-api/server/deprecations.ts:11-97` centralizes `V3_SUNSET_DATE`, `V3_SUNSET_HUMAN`, and per-family `replacement` + `docsUrl`. `web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:316-318,482` stamps `_deprecation: {message, replacement, docsUrl, sunsetAt}` onto every response via `attachDeprecation()`. Messages are Cloud-scoped (“On Langfuse Cloud… Self-hosted deployments are unaffected…”) so self-hosters see the signal only via docs/OpenAPI, not runtime, until they upgrade to v4.

- *Capability rejection:* When a deployment is in `events_only` mode, deprecated reads that lack an `events_core` fallback return `404` with body `{message: "This endpoint is not available on deployments running in Langfuse v4 events_only mode…", _deprecation: …}` (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:324-338`). This converts silent empty results into an explicit migration signal.

No email/changelog automation was found in-repo; changelog lives in `langfuse/langfuse-docs` (outside this source).

**3. Are old clients, plugins, traces, or persisted artifacts still usable?**

*Partially.* The design is “dual write → events_only” rather than permanent dual read:

- *Ingestion:* Old batch-ingestion clients (`POST /api/public/ingestion`) still work while the deployment is not `events_only`, but are deprecated in favor of `POST /api/public/otel/v1/traces`. The OTLP endpoint tolerates both JSON and protobuf with gzip (`web/src/pages/api/public/otel/v1/traces/index.ts:57-109`) and rejects malformed spans early (`:132-236`). Lower `x-langfuse-ingestion-version` values are accepted; only `>4` is hard-rejected (`web/src/pages/api/public/otel/v1/traces/index.ts:270-283`).

- *Queue/persisted jobs:* Compatibility is via additive `optional()` fields. Comments explicitly call out “Optional for rolling deploy compatibility with in-flight jobs” (`packages/shared/src/server/queues.ts:30-40,52-70`). A consumer falls back to reconstructing the S3 prefix if `bucketPrefix` is absent. This keeps in-flight jobs alive across deploys but provides no version field — a breaking rename would still break.

- *ClickHouse persisted data:* Legacy `traces`/`observations` tables stop being populated in `events_only` mode (`packages/shared/src/env.ts:322-330`). Reads via deprecated endpoints are blocked (`rejectInEventsOnlyMode`) rather than served from the new table, so old dashboards/exports that assumed legacy tables break unless migrated to enriched sources. `packages/shared/src/features/analytics-integrations/export-source-policy.ts:21-25,202-221` enforces this with capability errors that name `LANGFUSE_MIGRATION_V4_WRITE_MODE`.

- *Traces/prompts/scores artifacts:* v1 observation schemas still expose deprecated `usage`/`calculated*Cost` alongside `usageDetails`/`costDetails` for backwards compatibility (`fern/apis/server/definition/commons.yml:158-159,202-210`, `web/src/features/public-api/types/observations.ts:81-104`). Scores v2 is deprecated but still serves; v3 changes `BOOLEAN` value from `0|1 double` to `boolean` (`fern/apis/server/definition/scores-v3.yml:199-200` vs v1), a breaking type change gated behind a new versioned path.

**4. Does compatibility rely on policy alone or executable tests?**

*Both, but heavily weighted toward executable checks for the deprecation surface and lightly for data-plane compatibility.*

- *Executable:* `web/src/__tests__/server/deprecation-signal.servertest.ts:40-211` exercises live HTTP against deprecated endpoints and asserts the full `_deprecation` object equals the exported constants. `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:96-157` fails the build if any Fern deprecation message lacks the Cloud sunset wording, human/machine date mismatch, or if OpenAPI `deprecated` flags drift from Fern. `packages/shared/src/features/analytics-integrations/export-source-policy.test.ts:120-252` matrix-tests every `V4WriteMode × source` combination.

- *Policy/manual:* The `fern-deprecations.ts:64-99` reader enforces at OpenAPI generation that every deprecated endpoint has a `message`, but does not enforce semantic versioning (SemVer) on the API itself — the app version `4.16.0` (`packages/shared/src/constants/VERSION.ts:1`) is bumped by `release-it` (`package.json:61-95`) without an explicit API-compat checklist in-repo. Queue/backwards-compat relies on developer discipline to mark new fields `optional()` with a comment, not on a contract test that replays old payloads. Prisma/ClickHouse migrations are reviewed via `scripts/release-preflight.sh:83-146` which diffs migrations against `production` and asks for manual confirmation, but does not auto-test rollback.

## Architectural Decisions

| Decision | Why | Consequence | Evidence |
|----------|-----|-------------|----------|
| Fern as API source of truth with generated OpenAPI + SDKs | Single declarative definition for docs, validation, and client generation | Deprecations must be edited in one YAML; stamping pipeline can enforce parity | `fern/apis/server/definition/api.yml:1-28`, `fern/fern.config.json:2-4`, `fern/apis/server/generators.yml:14-40` |
| Sunset-date as Cloud commitment, self-hosted unaffected | Avoids forcing self-hosters onto a calendar; lets them migrate by upgrading | Deprecation runtime signal is gated on `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:316-318`), so self-hosted users can miss runtime warnings until upgrade | `web/src/features/public-api/server/deprecations.ts:11-16`, `fern/apis/server/definition/ingestion.yml:11-12` |
| `_deprecation` envelope on response body vs header | JSON body is visible to SDKs and to LLM agents parsing responses; header would be invisible to many clients | Array/primitive responses cannot carry it (`web/src/features/public-api/server/deprecations.ts:100-114` passthrough); requires clients to check JSON, not headers | `packages/shared/src/utils/zod.ts:165-172`, `web/src/features/public-api/server/deprecations.ts:102-114` |
| Path-versioned successors instead of header versioning | Simpler routing, explicit deprecation per path, no negotiation logic | No ability for a client to request “latest” or negotiate; migration requires endpoint switch | `fern/apis/server/definition/observations.yml:36` vs `legacy/observations-v1.yml:11`, `fern/apis/server/definition/scores-v3.yml:30` vs `fern/apis/server/definition/scores.yml:17` |
| `LANGFUSE_MIGRATION_V4_WRITE_MODE` tri-state | Allows gradual migration: `legacy` (old only) → `dual` (both) → `events_only` (new only) | Every read path must branch on this env; forgetting `rejectInEventsOnlyMode` risks silent empty results | `packages/shared/src/env.ts:322-330`, `packages/shared/src/features/analytics-integrations/export-source-policy.ts:159-165` |
| Additive `optional()` queue fields for rolling deploys | Zero-downtime deploys without draining queues | No schema version field; breaking renames/removals still unsafe | `packages/shared/src/server/queues.ts:30-40,52-77` |

## Notable Patterns

| Pattern | Where | Notes |
|---------|-------|-------|
| Sunset-date single source | `web/src/features/public-api/server/deprecations.ts:11-12` | `V3_SUNSET_DATE` + `V3_SUNSET_HUMAN` with test `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:148-157` that `new Date(V3_SUNSET_DATE)` formats to `V3_SUNSET_HUMAN`; any drift fails CI |
| OpenAPI description injection | `web/scripts/openapi/stamp-deprecations.ts:29-31,64-114` | `deprecationNotice()` prepends `**Deprecated:** {message} See upgrade guide` and asserts deep equality beyond touched ops; idempotent via `NOTICE_PATTERN` |
| Fern availability → OpenAPI bridge | `web/scripts/openapi/fern-deprecations.ts:71-115` | `getFernDeprecatedOperations()` walks every `*.yml`, joins `base-path`s via `joinApiPath()`, and throws if deprecated endpoint lacks `method`/`path`/`message` |
| Field-group opt-in for breaking expansions | `fern/apis/server/definition/observations.yml:30-31,40-45`, `fern/apis/server/definition/scores-v3.yml:21-27` | v2 observations and v3 scores return minimal core fields unless `fields=` lists groups (`basic,io,metadata,model,usage,…`); new groups can be added without breaking existing callers |
| Ingestion-version header gate | `web/src/pages/api/public/otel/v1/traces/index.ts:261-283` | `x-langfuse-ingestion-version` parsed as int, rejected if `>4`; lower versions implicitly dual-write; future v5 can bump the ceiling without changing path |
| SDK capability matrix | `web/src/features/sdk-version/lib/sdkVersionCapabilities.ts:24-37` | Hard-coded minimum semver per `(capability, language)`; `getSdkVersionCapabilityStatus` does semver tuple compare; used to decide `upgrade_required` vs `compatible` in v4 migration UI |

## Tradeoffs

- **Unified sunset date vs per-endpoint sunset:** One date (`2026-11-16`) for all v3 deprecations simplifies communication (`web/src/features/public-api/server/deprecations.ts:11`) but forces a big-bang migration. A per-endpoint sunset would allow earlier removal of low-risk surfaces (e.g. `metrics-v1`) without holding the entire v3 surface.

- **Cloud-only runtime deprecation signal:** Gating `_deprecation` on `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:316-318`) avoids noise for self-hosters who set their own upgrade cadence, but self-hosted users running outdated SDKs get no in-band warning until they upgrade to a version that has already removed the endpoint.

- **Body envelope vs standard `Deprecation`/`Sunset` headers:** Body `_deprecation` is agent-readable and survives SDK deserialization that might drop headers, but violates the IETF `Deprecation` header standard and cannot be added to non-object responses (`web/src/features/public-api/server/deprecations.ts:107-112` passthrough).

- **Additive optional fields vs versioned queue schema:** Marking every new queue field `.optional()` (`packages/shared/src/server/queues.ts:30-40`) is cheap and keeps rolling deploys green, but accumulates tech debt — consumers must handle `undefined` forever, and there is no way to deprecate/remove a field without a coordinated drain.

- **Path versioning vs header negotiation:** New paths (`/v2/`, `/v3/`, `/otel/v1/`) give a clear migration checklist, but double the route surface (e.g. `GET /api/public/observations`, `GET /api/public/observations/{id}`, `GET /api/public/v2/observations`, `GET /api/public/v2/observations/{id}`) and require clients to change URLs rather than just bumping a header.

- **Enriched vs legacy export capability check:** Failing closed with `400` when a persisted export source is incompatible with the current write mode (`packages/shared/src/features/analytics-integrations/export-source-policy.ts:202,221`) prevents silent data loss, but breaks existing exports on upgrade unless the user has already switched sources.

## Failure Modes / Edge Cases

| Failure Mode | Trigger | Observed Mitigation | Gap |
|--------------|---------|---------------------|-----|
| Silent empty reads in `events_only` if `rejectInEventsOnlyMode` missed | Route forgets the flag but reads legacy ClickHouse table no longer populated | 20+ routes set `rejectInEventsOnlyMode: true` (`web/src/pages/api/public/traces/index.ts:46`); unflagged routes silently return 0 rows | No compile-time check that every legacy-table read has the flag |
| `_deprecation` not emitted self-hosted | `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` unset → `deprecation = undefined` | Intentional (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:316-318`) | Self-hosted users may not learn about deprecation until upgrade breaks |
| Array/primitive body loses deprecation | `attachDeprecation` early-returns for non-objects (`web/src/features/public-api/server/deprecations.ts:107-112`) | Prevents corrupting array shapes | Any future array-returning deprecated endpoint would silently lack the signal; the test suite only covers object bodies |
| Sunset date drift between Fern YAML and code | Editor updates one but not the other | `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:125-157` asserts every Fern message contains `V3_SUNSET_HUMAN` and `V3_SUNSET_DATE` round-trips | No test that the Cloud runtime actually rejects traffic after sunset — it still serves with a deprecation header past the date |
| Ingestion version `5` spuriously accepted | Header typo or SDK sends `5.0` (parseInt yields `5`) | Hard reject `>4` with 400 (`web/src/pages/api/public/otel/v1/traces/index.ts:275-283`); `NaN` also rejected | `parseInt("4.1",10)==4` would pass; no semver parsing for ingestion version |
| Queue consumer crashes on old payload missing new required field | New producer marks field optional but consumer later assumes present | Consumer fallback (e.g. `bucketPrefix` reconstructed) | No schema-registry or consumer contract test replaying golden old payloads |
| `parseIoAsJson=true` breakage | Client still sends legacy param on v2 observations | Explicit 400 with message (`fern/apis/server/definition/observations.yml:61-65`, `web/src/features/public-api/types/observations.ts:328-333`) | Message is in Fern docs but not in `_deprecation` — client gets a 400 without migration hint |
| OpenAPI stamping overwrite | Fern description already starts with `**Deprecated`** | `web/scripts/openapi/stamp-deprecations.ts:93-96` throws instead of double-notice | No CI that the Fern docs themselves aren’t hand-writing a deprecation outside `availability` |

## Future Considerations

- **Add a machine-readable `Deprecation` + `Sunset` header alongside the body envelope** (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:482` could set `res.setHeader('Deprecation', 'true'); res.setHeader('Sunset', V3_SUNSET_DATE)`) so HTTP-aware clients/proxies can warn without parsing JSON, while keeping `_deprecation` for agent ergonomics.

- **Emit `_deprecation` self-hosted behind a feature flag or when `LANGFUSE_MIGRATION_V4_WRITE_MODE` is `dual`** instead of gating solely on `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION`, so self-hosters see warnings during the migration window.

- **Introduce a queue payload `schemaVersion` discriminator** in `packages/shared/src/server/queues.ts:23-78` (e.g. `version: z.literal(1).default(1)`) and add a golden-payload replay test that deserializes fixtures from prior releases, preventing accidental required-field additions.

- **Enforce `rejectInEventsOnlyMode` via lint/type check**: derive the set of routes that import `traces`/`observations` ClickHouse repositories and fail CI if `rejectInEventsOnlyMode` is absent when `LANGFUSE_MIGRATION_V4_WRITE_MODE` is `events_only` (aligns with `packages/shared/src/env.ts:322-330` contract).

- **Per-endpoint or per-group sunset after v4** — replace the single `V3_SUNSET_DATE` with a map (`web/src/features/public-api/server/deprecations.ts:21-31`) once the current big-bang passes, so low-risk legacy surfaces can be removed earlier.

- **Formalize `unstable` stability policy** — document in `fern/apis/server/definition/unstable/*` that breaking changes may occur without sunset, and add a `x-langfuse-stability: unstable` header or OpenAPI `x-stability` extension so SDK generators can surface it.

## Questions / Gaps

| Gap | What was searched | Why it matters |
|-----|-------------------|----------------|
| No in-repo CHANGELOG / migration guide for API breaking changes | Searched `CHANGELOG*`, `fern/` docs, `CONTRIBUTING.md:311-320`, package-level `AGENTS.md`; only external `https://langfuse.com/changelog` and `https://langfuse.com/docs/v4` are referenced (`web/src/features/public-api/server/deprecations.ts:40-42`) | Study cannot verify how self-hosters are notified outside API responses/OpenAPI |
| No version negotiation beyond `x-langfuse-ingestion-version` | Grepped `X-Langfuse`, `Accept-Version`, `capability`, `negotiat` across `web/src/features/public-api` and `packages/shared/src/server`; only ingestion header and SDK capability table found | Clients cannot declare supported API version; server cannot downgrade response shape |
| No backwards-compat tests for persisted ClickHouse data or queue replay | Searched `web/src/__tests__/server/*servertest*`, `packages/shared/src/server/queues.ts`, `packages/shared/clickhouse/migrations/*`; found only deprecation/export-source tests | A ClickHouse column rename could break historical queries; no golden-payload test would catch it |
| Unclear compatibility guarantee for `unstable` vs `stable` SDKs | Inspected `fern/apis/server/definition/unstable/*.yml`, `fern/apis/server/generators.yml:14-40`; unstable types lack `availability` and are not in deprecation suite | Consumers cannot distinguish safe-to-pin vs may-break-next-release without reading prose |
| No automated enforcement that Prometheus/otel SDKs honor `x-langfuse-ingestion-version` | Checked `web/src/pages/api/public/otel/v1/traces/index.ts:261-283`; header is optional, lower versions fall through to dual write | Old SDKs may unknowingly incur dual-write cost without warning |

---

Generated by `24.03-api-versioning-compatibility` against `langfuse`.
