# Source Analysis: langfuse

## Dimension 21.03: Extension Compatibility Testing

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript (Next.js Pages Router, tRPC, Prisma, ClickHouse, BullMQ/Redis, Zod, Fern) |
| Analyzed | 2026-08-27 |

## Summary

Langfuse has no first-party "plugin/extension" host model (no `IPlugin`, `Extension` interface, registry, or SDK extension point inside the monorepo). The effective extension contract is the **public API + ingestion surface**, defined in Fern (`fern/apis/server/definition/**`) and enforced at runtime by Zod schemas in `packages/shared/src/server/ingestion/types.ts` and `web/src/features/public-api/**`. Compatibility testing exists as **internal server integration tests** that validate those contracts, plus an unusually rigorous deprecation/stability subsystem ( `_deprecation` envelope, OpenAPI stamping, single-source sunset date). There is no exported conformance harness, fixture package, or runnable example kit that lets a third-party extension author self-verify against the contract in isolation. Examples are inline in Fern YAML and the README quickstart, not a standalone fixture library. Overall: contracts are machine-defined and internally tested, but extension authors cannot locally run a conformance suite without cloning the full stack and its Docker dependencies.

## Rating

**5/10 — Present but inconsistent**

Contracts are explicitly typed (Fern + Zod) and covered by a large internal test corpus with Zod-verified API calls and a novel OpenAPI-vs-Fern drift test. Breaking-change communication is mature (Fern `availability.message`, response-level `_deprecation`, automated OpenAPI stamping, semver release tagging). However, the lack of an external-facing conformance suite, published test fixtures, or self-contained example harness means an extension author cannot answer "does my client satisfy Langfuse's contract?" without running the full server integration suite. Stability guarantees are de-facto, not documented as an extension compatibility policy.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Public API contract source of truth | Fern definitions own every endpoint/type; 38 YAML files, `ingestion.yml` defines `IngestionEvent` discriminated union with 14 variants | `fern/apis/server/definition/ingestion.yml:109-146`, `fern/apis/server/definition/commons.yml:1-1083`, `fern/apis/server/definition/opentelemetry.yml:6-36` |
| Public API generation contract | Version pinned; generation via Fern, hand-edit banned | `fern/fern.config.json:1-4`, `CONTRIBUTING.md:544-552`, `web/AGENTS.md:147-173` |
| Ingestion runtime contract | Single factory `createAllIngestionSchemas({isPublic})` builds all body+event zod schemas; public vs internal env validation split, legacy types kept | `packages/shared/src/server/ingestion/types.ts:415-775` |
| Queue payload contract | All BullMQ payloads are `z.object` with explicit `QueueName`/`QueueJobs` enums and `TQueueJobTypes` map; comments call out rolling-deploy backward compat via `.optional()` | `packages/shared/src/server/queues.ts:23-48`, `packages/shared/src/server/queues.ts:395-436`, `packages/shared/src/server/queues.ts:476-667` |
| Webhook outbound contract | Discriminated `WebhookOutboundEnvelopeSchema` union (`prompt-version` \| monitor \| notification) wrapped in `WebhookInputSchema` | `packages/shared/src/server/queues.ts:296-321` |
| Zod-verified API conformance tests | Server integration tests drive real HTTP server against test DB and validate responses with `makeZodVerifiedAPICall(GetTracesV1Response, ...)` | `web/src/__tests__/server/traces-api.servertest.ts:246-252`, `web/src/__tests__/server/ingestion-api.servertest.ts:133-153`, `web/src/__tests__/server/scores-api-v2.servertest.ts:1` (batch), `web/src/__tests__/test-utils.ts` (helper) |
| Ingestion conformance-style coverage | Parametrized `describe` loops exercise every `eventTypes` variant (`trace-create`, `generation-create`, `span-create`, `agent/tool/chain/...`) and non-default environments | `web/src/__tests__/server/ingestion-api.servertest.ts:58-155`, `web/src/__tests__/server/ingestion-api.servertest.ts:185-435` |
| OpenAPI ↔ Fern drift conformance test | Parses generated `public/generated/api/openapi.yml`, asserts deprecated operation sets match and every `availability.message` is carried into `deprecated:true` + `**Deprecated:**` notice | `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:96-146` |
| Runtime deprecation-signal tests | 9 tests assert top-level `_deprecation` is injected on every legacy family (observations v1, scores v2, traces, sessions, dataset-runs) and carries `replacement`, `docsUrl`, `sunsetAt` | `web/src/__tests__/server/deprecation-signal.servertest.ts:40-211` |
| Deprecation single source of truth | `V3_SUNSET_DATE = "2026-11-16"` and human form, `V3_NOTICE`, `REPLACEMENT` and `DOCS` maps; `attachDeprecation` guards non-object bodies | `web/src/features/public-api/server/deprecations.ts:11-115` |
| Fern deprecation extraction | `getFernDeprecatedOperations` walks every `*.yml`, joins `api.yml` `base-path` + service `base-path` + endpoint `path`, enforces `availability.message` present | `web/scripts/openapi/fern-deprecations.ts:71-115` |
| OpenAPI stamping | `stampDeprecations` sets `deprecated:true`, prefixes `**Deprecated:** <msg> See [upgrade guide](…)` (idempotent, replaces old notice, rejects hand-written notices) and `assert.deepStrictEqual` guards formatting-only drift | `web/scripts/openapi/stamp-deprecations.ts:29-31`, `web/scripts/openapi/stamp-deprecations.ts:64-120` |
| Example payloads (Fern) | Three `examples` blocks on `POST /ingestion` (trace, span, score) and one OTLP JSON example on `POST /otel/v1/traces` | `fern/apis/server/definition/ingestion.yml:44-107`, `fern/apis/server/definition/opentelemetry.yml:35-65` |
| Example payloads (README) | Quickstart shows `pip install langfuse openai` + `@observe` decorator + `openai.chat.completions.create` wired to ingestion | `README.md:186-213` |
| No plugin interface found | `glob **/extension*`, `**/plugin*`, `**/conformance*` returned no files; grep for `extension` hit only CodeMirror/file-extension/notifications noise | search boundary: root glob + grep `extension\|plugin.*interface\|conformance` (no hits) |
| No exported fixture package | Internal test factories (`createTrace`, `createObservation`, `createTraceScore`, `createOrgProjectAndApiKey`) live in `packages/shared/src/server` (server-only barrel), not published for external consumers; `withMiddlewares` and `createAuthedProjectAPIRoute` are internal | `web/src/__tests__/server/traces-api.servertest.ts:1-14`, `packages/shared/AGENTS.md:31-40` (entry points do not expose test fixtures) |
| Versioning / breaking-change tagging | CI generates `semver` Docker tags `{{version}}`, `{{major}}.{{minor}}`, `{{major}}`; `CONVENTIONAL_COMMITS` with `BREAKING CHANGE: !` rule; release is `promote-main-to-production` to Cloud + `release.yml` to OSS | `.github/workflows/pipeline.yml:1312-1314`, `.agents/skills/git-workflow/SKILL.md:33`, `CONTRIBUTING.md:424-444` |
| Public API middleware contract | `withMiddlewares` centralizes `MethodNotAllowedError`, `BaseError`→`httpCode` mapping, Zod 400, ClickHouse 422 with legacy upgrade message | `web/src/features/public-api/server/withMiddlewares.ts:84-206` |

## Answers to Dimension Questions

**1. Are extension contracts tested?**

Partially. The ingest and public REST ingress contracts are tested at depth as *server integration tests*: `web/src/__tests__/server/ingestion-api.servertest.ts:58-491` exercises every `IngestionEvent` type and many edge cases (special S3 chars, `\r` IDs, huge metadata, `null` comment clearing, environment fallback); `web/src/__tests__/server/traces-api.servertest.ts:210-881` covers CRUD, pagination, `fields=` filtering, and advanced filter precedence with `makeZodVerifiedAPICall`. The more interesting contract test is `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:96-146`, which treats Fern as source of truth and fails on drift between Fern `availability` and the generated OpenAPI spec, and `web/src/__tests__/server/deprecation-signal.servertest.ts:30-211` which pins the runtime `_deprecation` envelope. Queue payload correctness is enforced by Zod schemas with intentional `.optional()` fields for rolling deploys (`packages/shared/src/server/queues.ts:32-40`). What is missing is a *conformer-facing* harness: there is no `assertConformsToLangfuseContract(client)` helper or `langfuse-conformance` npm package that an external SDK/integration author could `npm install` and run locally. Existing tests require a running Postgres/ClickHouse/Redis stack (`AGENTS.md:345`, `.env.test` story) and are written for maintainers, not extension authors.

**2. Are fixtures provided for extension authors?**

No exported fixtures. Inside the repo, `web/src/__tests__/test-utils.ts` exposes `makeAPICall` / `makeZodVerifiedAPICall`, and `packages/shared/src/server` exposes factory helpers `createTrace`, `createObservation`, `createTraceScore`, `createOrgProjectAndApiKey`, `createEvent`, `createDataset` etc. used pervasively (`web/src/__tests__/server/traces-api.servertest.ts:1-14`, `web/src/__tests__/server/deprecation-signal.servertest.ts:1-12`). These are invaluable for repo-owned tests but are not published as a test-fixture library and require the internal DB clients (`@langfuse/shared/src/db`, `@langfuse/shared/src/server`). Searched for `**/fixtures/**`, `**/test-utils*` export, and public `generate` clients: only generated API clients (`generated/*`) are shipped, which are clients, not test fixtures. Verdict: fixtures exist for maintainers; none for extension authors.

**3. Are examples provided?**

Yes, but minimal and co-located, not curated as an extension kit. Fern YAML is the best source: `fern/apis/server/definition/ingestion.yml:44-107` embeds three complete `batch: [...]` JSON examples (trace-create, span-create, score-create with request+response), and `fern/apis/server/definition/opentelemetry.yml:35-65` shows a full `resourceSpans` OTLP payload. The README quickstart (`README.md:186-213`) doubles as a live example for the SDK path. What is not present is a `examples/` directory of runnable integration clients per language, a Postman collection in-repo, or a documented "build your own ingester" guide referencing back to Fern types. Example coverage for *webhook* consumers (`WebhookOutboundEnvelopeSchema` in `packages/shared/src/server/queues.ts:309-314`) has no companion example payload file searched under `web/src/features/public-api/**` and `packages/shared/src/server/webhooks/**`.

**4. Are stability guarantees documented?**

Implicit, not formalized as an extension compatibility policy. The repo communicates stability through mechanism, not a `STABILITY.md`:
- Fern `availability: { status: deprecated, message: "On Langfuse Cloud, ... will be removed on November 16, 2026. ..." }` on `fern/apis/server/definition/ingestion.yml:10-12` is the canonical announcement.
- Runtime signal: every legacy v3 response carries ` _deprecation: { message, replacement, docsUrl, sunsetAt }` (`web/src/features/public-api/server/deprecations.ts:44-97`), verified by `deprecation-signal.servertest.ts`.
- OpenAPI spec is automatically stamped with `deprecated:true` + `**Deprecated:**` prefix and upgrade-guide link (`web/scripts/openapi/stamp-deprecations.ts:29-31`, `web/scripts/openapi/fern-deprecations.ts:115`), verified by `openapi-deprecations.servertest.ts:112-146` which also checks the sunset date appears and is scoped to Cloud with "Self-hosted deployments are unaffected".
- Versioning is semver-tagged in CI (` .github/workflows/pipeline.yml:1312`) and commit convention enforces `BREAKING CHANGE` (` .agents/skills/git-workflow/SKILL.md:33`).
There is no public `API_VERSIONING.md`, `BREAKING_CHANGE_POLICY.md`, or SDK compatibility matrix in-repo; docs site (`langfuse.com/docs/api-and-data-platform/...`) is the intended venue but not inspected per isolation rules (sibling source forbidden). Self-hosted deprecation is deliberately soft ("may stop working", not hard date) — see `openapi-deprecations.servertest.ts:136-146`.

## Architectural Decisions

- **Fern as single contract source** (`fern/apis/server/definition/**:1` + `CONTRIBUTING.md:544`). Reasoning: hand-authored OpenAPI via Fern gives typed server clients and docs; drift is blocked by `openapi-deprecations.servertest.ts:96`. Tradeoff: contributors must keep YAML and `web/src/features/public-api/types/**` in sync manually.
- **Zod at the edge, Prisma+ClickHouse behind** (`packages/shared/src/server/ingestion/types.ts:415` factory, `web/src/pages/api/public/ingestion.ts:133-144`). Reasoning: lossy IDs/metadata are rejected with 207 per-event errors, not 4xx whole-batch, preserving partial success. Tradeoff: large zod transforms (usage normalization, cost filtering) add CPU on the hot ingestion path.
- **Envelope-level deprecation rather than HTTP header** (`web/src/features/public-api/server/deprecations.ts:102-115` `attachDeprecation`). Reasoning: coding agents self-correct when JSON contains `_deprecation` with `replacement` + `docsUrl`; header would be invisible to JSON-parsing agents. Tradeoff: non-object bodies (arrays) cannot carry the signal and pass through unchanged.
- **Queue payload backward-compat via optional fields** (`packages/shared/src/server/queues.ts:30-40` comments). Reasoning: rolling deploy of web+worker without draining queues. Tradeoff: optional fields linger indefinitely; removal requires queue drain window.
- **No plugin registry** (absence across `web/src/features/**`, `packages/shared/src/**`). Reasoning: Langfuse is an observability platform, not a harness; extensibility is via open API/SDKs in separate repos (`langfuse/langfuse-python`, `langfuse/langfuse-js` per `.agents/skills/langfuse-codebase-navigator/references/repository-map.md:23`). Tradeoff: no in-process extension sandbox or version negotiation to test.

## Notable Patterns

- **Contract-first with automated drift detection.** Fern → `fernd export` → `public/generated/api/openapi.yml` → `stampDeprecations` → test asserts `parse(openapi).paths[*][method].deprecated` equals `getFernDeprecatedOperations()` set. Few projects test the *generation pipeline* itself (`web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:96-109`).
- **Idempotent code-mod test for OpenAPI.** `stampDeprecations` is tested for idempotence, replacement (not stacking), alias preservation (`*ref_0`), and block-scalar style retention (`|-` vs `>-`) — shows awareness that generated YAML is otherwise brittle (`web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:206-289`, `web/scripts/openapi/stamp-deprecations.ts:115-118`).
- **Zod-verified HTTP tests as contract tests.** `makeZodVerifiedAPICall(GetTracesV1Response, ...)` parses the response through the same zod schemas that define `fern/apis/server/definition/commons.yml` types, making each servertest a de-facto schema conformance check (`web/src/__tests__/server/traces-api.servertest.ts:246`).
- **Sunset-date cohesion.** `V3_SUNSET_DATE` (machine) vs `V3_SUNSET_HUMAN` (prose) is derived-tested via `new Date(...).toLocaleDateString` (`web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:148-157`), preventing doc/code drift.

## Tradeoffs

- **Internal depth vs external reusability.** The test corpus is huge (~100 `*.servertest.ts` files) but tightly coupled to a live database; porting any test as an external conformance check would require extracting zod schemas + factories into a publishable package.
- **Envelope deprecation is discoverable but pollutes the domain.** Adding `_deprecation` to every legacy response is excellent for LLM agents but blurs the API type (each response type gains an ad-hoc field via `.extend()` vs strict schema). The team mitigates by noting the field is added at response level, not on shared item schemas (`web/src/__tests__/server/deprecation-signal.servertest.ts:73-75`).
- **Fern gives stability but no compatibility test harness generation.** Unlike protobuf/gRPC `buf breaking`, Fern does not emit a breaking-change report; the gap is filled by runtime warnings and docs, not a CI `breaking` job.
- **Rolling-deploy friendly schemas vs clean types.** Making `ingestionApiKey`, `bucketPrefix`, etc. `.optional()` keeps deploys safe (`packages/shared/src/server/queues.ts:32-40`) at cost of weaker invariants for new code paths.
- **Docs out-of-repo.** The strongest stability narrative lives in `langfuse/langfuse-docs` (`content/changelog/**` per `.agents/skills/langfuse-codebase-navigator/references/repository-map.md:123`), so this source alone cannot prove breaking-change communication timeliness.

## Failure Modes / Edge Cases

- **Extension author has no offline validator.** An SDK author who constructs an `IngestionEvent` batch cannot validate it without calling a live Langfuse instance; `packages/shared/src/server/ingestion/types.ts:844` `createIngestionEventSchema()` exists but is not exported as a standalone npm validator package nor documented for external use. Sending a `dataset-run-item-create` event via the public path is intentionally always rejected (`packages/shared/src/server/ingestion/types.ts:698-702` refine `() => false`), but the error surfaces only at ingestion (207 error) not at schema-publish time.
- **Environment normalization silently remaps.** Invalid environments fall back to `"default"` via `.catch(DEFAULT_TRACE_ENVIRONMENT)` (`packages/shared/src/server/ingestion/types.ts:245`). An extension sending `langfuse-*` or `!bad` environments receives no error — behavior is silent, not conformance-checked. Tests assert only that the call succeeds, not that the remapping occurred (`web/src/__tests__/server/ingestion-api.servertest.ts:711-740`).
- **Deprecation signal absent on array/primitive bodies.** `attachDeprecation` returns the body unchanged for arrays (`web/src/features/public-api/server/deprecations.ts:108-112`); any future array-top-level endpoint would lose the signal.
- **OpenAPI stamp assumes single-file spec.** `fern-deprecations.ts:74-78` reads `api.yml` plus all definition files; if an endpoint omits `method`/`path`, it throws at generation time — a build failure, not a runtime signal. A missing `availability.message` also throws (`web/scripts/openapi/fern-deprecations.ts:96`, tested in `openapi-deprecations.servertest.ts:184-203`).
- **Full suite requires Docker stack.** Per `CONTRIBUTING.md:362-360` and `AGENTS.md:345`, tests write/delete real data, use separate `langfuse_test` DB and Redis DB 1, and must be run with `pnpm exec dotenv -e .env.test -- pnpm --filter web run test`. An extension author on a laptop without Docker cannot meaningfully exercise contract tests.

## Future Considerations

- **Publish a conformance kit.** Extract `createIngestionEventSchema`, `TraceBody`/`ScoreBody` zod schemas and factory helpers into `@langfuse/conformance` (or extend `@langfuse/shared` with a `test-fixtures` entry point) plus a `langfuse validate --file batch.json` CLI that external SDK/integration authors can run offline.
- **Ship snapshot fixtures.** Commit canonical `IngestionRequest` and `OtelTraceRequest` JSON fixtures alongside Fern YAML (companion to `fern/apis/server/definition/ingestion.yml:44` examples) and test that `z.parse(fixture)` succeeds, so downstream repos can vendor the same fixtures.
- **Add a breaking-change CI gate.** Evaluate `fern check --compat` or custom `getFernDeprecatedOperations`-like diff that fails a PR if a non-deprecated field is removed/renamed without an `availability.message`, mirroring `openapi-deprecations.servertest.ts`'s sunset-cohesion test.
- **Document an explicit extension stability policy.** Add `docs/api-stability.md` (or a section in `CONTRIBUTING.md:544`) stating semver scope, deprecation window (currently ~18 months to 2026-11-16), and that `_deprecation docsUrl` is the notification channel. Reference it from `fern/fern.config.json` and release notes.
- **Unblock array-response deprecation.** If any future endpoint returns a top-level array, wrap it (`{ data: [], _deprecation }`) or emit a `Deprecation:` HTTP header alongside the envelope.
- **Provide language-portable examples.** Keep the README quickstart but add runnable `examples/ingestion/` (Python, JS, Go, curl) that import the generated client from `generated/` and assert `207` handling, so the example itself becomes a conformance probe.

## Questions / Gaps

- No evidence of a public Postman collection or generated `openapi.yml` bundle checked into this repo beyond `public/generated/api/openapi.yml` (path from `web/src/__tests__/server/unit/openapi-deprecations.servertest.ts:32`). Whether the published collection is auto-updated from Fern is described only in `CONTRIBUTING.md:546-552` and not observable without fetching `langfuse-docs`.
- Search for `BREAKING`, `stability`, `semver` policy docs inside `sources/langfuse` returned only release tagging and commit convention, not a stability guarantee doc. Stated gap, not assumed.
- SDK-level conformance (e.g., does `langfuse-python` batch pass `createIngestionEventSchema`?) is out of scope: SDKs live in separate repos per `.agents/skills/langfuse-codebase-navigator/references/repository-map.md:23`; no cross-repo import tested.
- Webhook consumer contract tests were not found under `web/src/__tests__` or `worker/src/**`; `worker/src/__tests__/url-normalization.test.ts:42` tests `validateWebhookURL` but not payload compatibility.

---

Generated by `Dimension 21.03: Extension Compatibility Testing` against `langfuse`.
