# Source Analysis: langfuse

## 13.03 Failure Visibility

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo (Next.js web app, Express + BullMQ worker, shared package; Postgres/ClickHouse/Redis/S3) |
| Analyzed | 2026-08-23 |

## Summary

Langfuse implements failure visibility as an explicit, four-audience contract rather than a byproduct of exception handling. SDK clients ("the model side" of this platform — the agents/LLM apps that ingest traces) receive a per-event HTTP 207 multi-status response with typed error objects (`sources/langfuse/web/src/pages/api/public/ingestion.ts:182`, `sources/langfuse/packages/shared/src/server/ingestion/processEventBatch.ts:462-524`). UI users get status-code-titled crash pages carrying a Sentry Error ID for support escalation (`sources/langfuse/web/src/pages/_error.tsx:63-77`, `sources/langfuse/web/src/components/CrashModal/CrashModal.tsx:37-50`), plus tRPC error messages deliberately differentiated between cloud and self-hosted deployments and stripped of stack traces (`sources/langfuse/web/src/server/api/trpc.ts:176-216`). Developers get winston JSON logs correlated to OTel trace/span IDs and Datadog fields (`sources/langfuse/packages/shared/src/server/logger.ts:6-25`), a `traceException` helper that records exception events, `error.*` span attributes, and ERROR span status (`sources/langfuse/packages/shared/src/server/instrumentation/index.ts:132-179`), and a heavily governed Sentry pipeline with tested noise filters (`sources/langfuse/web/instrumentation-client.ts:16-64`, `sources/langfuse/web/src/utils/sentryFilters.clienttest.ts`). Operators get layered health endpoints with opt-in deep probes (`failIfDatabaseUnavailable`, `failIfNoRecentEvents`, `failIfQueueConsumptionStuck`, `failIfEventPropagationStuck`), queue rate/depth/DLQ-age metrics, DLQ auto-retry, and a Redis-backed "projects currently failing ingestion" gauge. The dominant design value is *right-sizing*: internal exceptions (Prisma, ClickHouse internals) are masked at each boundary while remediation guidance (docs URLs, migration bridges, advice messages) is pushed outward instead.

## Rating

**8 / 10** — A clear, deliberate model with explicit interfaces per stakeholder, operational safeguards (health probes, DLQ gauges + retry, ingestion-failure tracking), and real test coverage (ingestion 207 contract asserted across dozens of server tests; a 1,199-line test file guarding Sentry noise filters). It falls short of 9–10 because detail-level decisions are inconsistent at one seam — REST fallback paths return raw internal `error.message` on 500s where tRPC masks them — and user-facing mutation failures rely on ad-hoc per-component toasts without a unified reporting seam equivalent to the client-side `reportError`.

## Evidence Collected

Every entry cites file paths with line numbers relative to `studies/agent-harness-study/sources/langfuse/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model-facing: batch result contract | Ingestion returns HTTP 207 with `{successes:[{id,status}], errors:[{id,status,message,error}]}` so clients learn exactly which events failed | `web/src/pages/api/public/ingestion.ts:176-182` |
| Model-facing: per-event error typing | `aggregateBatchResult` maps `InvalidRequestError`→400, `UnauthorizedError`→401, `LangfuseNotFoundError`→404, masks everything else as 500 "Internal Server Error" | `packages/shared/src/server/ingestion/processEventBatch.ts:479-507` |
| Model-facing: BaseError surface | `BaseError` carries `httpCode`, `isOperational`, `isUserError()`; handler emits `{error: name, message}` with matching status | `packages/shared/src/errors/BaseError.ts:1-27`, `web/src/pages/api/public/ingestion.ts:193-210` |
| Model-facing: validation errors | Zod parse failure → 400 `{message:"Invalid request data", errors:[...issue messages]}` | `web/src/pages/api/public/ingestion.ts:138-146` |
| Model-facing: internal masking | Prisma exceptions are collapsed to bare "Internal Server Error"; non-typed errors fall back to raw `error.message` (inconsistency, see Tradeoffs) | `web/src/pages/api/public/ingestion.ts:230-241` |
| Model-facing: actionable rejection text | events_only-mode rejections embed remediation steps (upgrade SDK, set `LANGFUSE_MIGRATION_V4_WRITE_MODE=dual`) and a docs URL in the per-event error message | `web/src/pages/api/public/ingestion.ts:253-260,296-305` |
| REST middleware error mapping | `withMiddlewares` maps `ClickHouseResourceError`→422 with advice + docs link, BaseError→own code/message, Zod→400 with issues, Prisma→masked 500 | `web/src/features/public-api/server/withMiddlewares.ts:116-203` |
| User-facing: crash page | `_error` page renders per-status titles, captures Sentry event ID via `captureUnderscoreErrorException`, offers Return-home | `web/src/pages/_error.tsx:15-32,63-77` |
| User-facing: support handoff | CrashModal displays the Sentry "Error ID" so users can quote it to support; client-side exceptions point users to the browser console | `web/src/components/CrashModal/CrashModal.tsx:28-50`, `web/src/pages/_error.tsx:34-38` |
| User-facing: tRPC masking policy | 4xx messages exposed verbatim; 5xx replaced by "Internal error. We have been notified…" (cloud) or "…check error logs in your self-hosted deployment."; `cause:null` strips stacks | `web/src/server/api/trpc.ts:194-212` |
| User-facing: CH resource advice | ClickHouse resource-limit errors become `UNPROCESSABLE_CONTENT` with `ERROR_ADVICE_MESSAGE`; formatter hides CH stack traces but keeps zodError flattened for the frontend | `web/src/server/api/trpc.ts:106-124,180-193` |
| Developer: structured logging | Winston JSON logs with stack traces, timestamps; `LANGFUSE_LOG_LEVEL`/`LANGFUSE_LOG_FORMAT` configurable | `packages/shared/src/server/logger.ts:27-57`, `packages/shared/src/env.ts:236-240` |
| Developer: log↔trace correlation | Every log entry gets `trace_id`/`span_id` plus Datadog `dd.trace_id`/`dd.span_id`; baggage entries merged into log lines | `packages/shared/src/server/logger.ts:6-25` |
| Developer: exception → span | `traceException` records an OTel exception event, sets `error.stack/error.message/error.type`, marks span ERROR; `instrumentAsync/Sync` do this automatically on throw | `packages/shared/src/server/instrumentation/index.ts:132-179,61-69` |
| Developer: typed error hierarchy | Shared `errors/` module: ApiError family (NotFound/Unauthorized/PayloadTooLarge/ServiceUnavailable…) built on `BaseError`, with centralized messages | `packages/shared/src/errors/BaseError.ts:1-40`, `packages/shared/src/errors/index.ts` |
| Developer: queue failure logs | Worker `failed`/`error`/`stalled` handlers log job id + queue name, call `traceException`, increment typed metrics | `worker/src/queues/workerManager.ts:174-208` |
| Developer: client capture seam | `reportError` coerces unknowns into legible Errors, tags by `area`, breadcrumbs expected failures, forbids PII in messages; enforced by ESLint import ban on direct `captureException` | `web/src/utils/reportError.ts:53-130`, `web/eslint.config.mjs:48` |
| Operator: web health endpoint | `/api/public/health` returns 200/503 `{status, version}`; opt-in deep checks via query params, each failure yielding a distinct human-readable status string | `web/src/pages/api/public/health.ts:28-47`, `web/src/features/public-api/server/health-service.ts:22-133` |
| Operator: worker health probes | Container health checks DB (`SELECT 1`), Redis ping with 2 s timeout, SIGTERM readiness (500), opt-in `failIfEventPropagationStuck`/`failIfQueueConsumptionStuck` returning structured `details` JSON | `worker/src/features/health/index.ts:128-221` |
| Operator: queue metrics | Per-queue `.rate{type=request\|completed\|failed\|error\|stalled}`, `.time_distribution{wait\|processing}`, `.depth`, `.dlq_oldest_age` gauges (empty DLQ emits 0 so monitors see reset) | `worker/src/queues/workerManager.ts:59-110,163-208`, `worker/src/features/queue-metrics-runner/index.ts:114-199` |
| Operator: DLQ auto-retry | Cron every 10 min retries failed jobs in 5 named queues, recording `langfuse.dlq_retry_delay` histogram per project/queue | `worker/src/services/dlq/dlqRetryService.ts:9-63` |
| Operator: ingestion-failure tracking | Redis sorted set of projects failing ingestion (typed source/reason), TTL'd, exported as gauge `langfuse.ingestion.project_failure.active_projects`; marked from API and processing paths | `packages/shared/src/server/redis/ingestionFailureTracking.ts:6-99`, `web/src/pages/api/public/ingestion.ts:183-204` |
| Operator: metric backends | dogstatsd always on; CloudWatch publishing gated by `ENABLE_AWS_CLOUDWATCH_METRIC_PUBLISHING`; dd-trace runtime metrics enabled | `packages/shared/src/server/instrumentation/index.ts:311-351`, `web/src/observability.config.ts:104-107` |
| Operator: runbook docs | Datadog query recipes skill documents how to query queue consumer `.rate/.depth/.dlq_oldest_age` and tenant usage metrics | `.agents/skills/datadog-query-recipes/references/queue-consumers.md:199-215` |
| Configurability: tracing | OTLP exporter + `OTEL_TRACE_SAMPLING_RATIO` sampler (0–1], service name/version attributes; health-check spans excluded from traces | `web/src/observability.config.ts:109-167,121-126`, `web/src/env.mjs:305-307` |
| Configurability: Sentry | DSN/env/release via `NEXT_PUBLIC_SENTRY_*`; sample rates via `NEXT_PUBLIC_LANGFUSE_TRACING_SAMPLE_RATE`; replay masks all text/inputs | `web/instrumentation-client.ts:11-122` |
| Tests: ingestion contract | Dozens of assertions pinning 207 multi-status behavior incl. mixed success/failure batches | `web/src/__tests__/server/ingestion-api.servertest.ts:137,414,476,536,603,639,…,1004` |
| Tests: Sentry noise governance | 1,199-line suite proving denylist rules keep real errors while dropping known noise; chunk-parse errors grouped not dropped | `web/src/utils/sentryFilters.clienttest.ts:204,801,871` |

## Answers to Dimension Questions

**1. Is the model informed of failures?**
Yes, precisely. The ingestion API never fails silently per event: it responds HTTP 207 with per-event success/error entries (`web/src/pages/api/public/ingestion.ts:176-182`). Each error carries the event `id`, an HTTP-like `status`, a stable `message`, and a specific `error` reason derived from typed exceptions (`packages/shared/src/server/ingestion/processEventBatch.ts:479-507`). Rejections under v4 migration mode even include step-by-step remediation and a docs URL in the error string itself (`web/src/pages/api/public/ingestion.ts:296-305`). Internal failures are masked as generic 500 "Internal Server Error" so clients can retry without parsing internals.

**2. Is the user informed appropriately?**
Yes, with deliberate tiering. Page-level failures render a crash page whose title encodes the status code and which displays a Sentry Error ID for support escalation (`web/src/pages/_error.tsx:15-38`, `web/src/components/CrashModal/CrashModal.tsx:37-50`). For data/API failures inside the app, 4xx validation and permission messages are shown verbatim while 5xx messages are replaced with deployment-aware copy — cloud users see "We have been notified and are working on it", self-hosted operators see "check error logs in your self-hosted deployment" (`web/src/server/api/trpc.ts:198-211`). ClickHouse overload surfaces an actionable advice message instead of raw engine output (`web/src/server/api/trpc.ts:180-193`). Mutation failures appear as short toasts (e.g. `web/src/ee/features/billing/components/BillingDiscountCodeButton.tsx:45`).

**3. Can developers debug failures?**
Yes. Logs are JSON (or configurable text) with embedded stack traces and automatic `trace_id`/`span_id`/Datadog correlation fields (`packages/shared/src/server/logger.ts:31-49,6-25`). Exceptions flow into OTel spans as exception events with `error.*` attributes and ERROR status (`packages/shared/src/server/instrumentation/index.ts:132-179`), and BullMQ failures log job id + queue before tracing (`worker/src/queues/workerManager.ts:174-195`). Client-side, a single `reportError` seam guarantees area-tagged, PII-safe Sentry issues with breadcrumbs for expected states (`web/src/utils/reportError.ts:89-130`). Log verbosity scales with severity: 404/401 → info, other 4xx → warn, 5xx → error (`web/src/server/api/trpc.ts:151-173`).

**4. Can operators detect failure patterns?**
Yes, through several complementary channels: (a) layered health endpoints with opt-in deep probes that force restart loops only on dedicated probes — DB reachability, recent-ingest liveness, event-propagation cursor staleness, queue-consumption liveness — returning machine-readable failure details (`web/src/features/public-api/server/health-service.ts:22-133`, `worker/src/features/health/index.ts:149-221`); (b) per-queue request/completed/failed/error/stalled counters and `.dlq_oldest_age`/`.depth` gauges with documented DataDog query recipes (`worker/src/queues/workerManager.ts:163-208`, `.agents/skills/datadog-query-recipes/references/queue-consumers.md:199-215`); (c) a gauge counting projects currently failing ingestion, tagged by failure source/reason (`packages/shared/src/server/redis/ingestionFailureTracking.ts:66-99`); and (d) DLQ auto-retry with delay histograms (`worker/src/services/dlq/dlqRetryService.ts:18-63`).

**Configurable detail levels?** Substantially. `LANGFUSE_LOG_LEVEL`/`LANGFUSE_LOG_FORMAT` (`packages/shared/src/env.ts:236-240`), `OTEL_TRACE_SAMPLING_RATIO` (`web/src/env.mjs:307`), CloudWatch publishing toggle (`packages/shared/src/server/instrumentation/index.ts:320-334`), Sentry rates/DSN (`web/instrumentation-client.ts:11-102`), health-probe depth via query params (`web/src/pages/api/public/health.ts:30-33`), and cloud-vs-self-hosted message selection at runtime (`web/src/server/api/trpc.ts:200-202`).

## Architectural Decisions

1. **One typed error hierarchy, many renderers.** All surfaces throw `BaseError` subclasses from `packages/shared/src/errors/` and let boundary middlewares decide presentation: tRPC maps cause→code (`web/src/server/api/trpc.ts:141-149`), REST middleware maps httpCode→status+JSON (`web/src/features/public-api/server/withMiddlewares.ts:165-174`), ingestion aggregates per-event. This makes detail-level policy auditable in ~3 choke points.
2. **Mask internals at boundaries, push remediation outward.** Prisma exceptions collapse to opaque 500s (`web/src/pages/api/public/ingestion.ts:230-234`), ClickHouse stacks are stripped from tRPC responses because they "may contain sensitive info" (`web/src/server/api/trpc.ts:117-121`), yet users still receive docs links and advice messages (`web/src/features/public-api/server/withMiddlewares.ts:42-60`).
3. **Opt-in deep health probes.** Expensive/sticky checks (`failIfQueueConsumptionStuck`, `failIfEventPropagationStuck`) are query-param gated so only dedicated restart-forcing probes pay their cost (`worker/src/features/health/index.ts:131-143`).
4. **Governed noise filtering instead of blanket suppression.** Client Sentry events pass a documented, individually-tested rule set; stale-chunk errors are fingerprint-grouped rather than dropped so real breakage still spikes one issue (`web/instrumentation-client.ts:54-61`, `web/src/utils/sentryFilters.clienttest.ts:871`).
5. **Failure signals as first-class metrics.** Ingestion failures write a TTL'd Redis zset exported as a gauge; queue consumers emit typed rate counters; DLQ age is a monitored SLO-style signal with an empty-set-zero contract so dashboards show recovery (`worker/src/queues/workerManager.ts:59-67`).

## Notable Patterns

- **207 Multi-Status batching**: partial-success visibility for ingestion, so one bad event doesn't hide the fate of the rest (`web/src/pages/api/public/ingestion.ts:176-182`).
- **Log↔trace stitching**: every JSON log line carries `trace_id`, `span_id`, and Datadog-compatible `dd.*` ids, enabling cross-tool jump-debugging (`packages/shared/src/server/logger.ts:6-25`).
- **Deployment-aware messaging**: identical 5xx produces different user copy depending on `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION`, respecting who can actually read the logs (`web/src/server/api/trpc.ts:200-202`).
- **Support-handoff IDs**: surfacing the Sentry event id in the CrashModal turns "something went wrong" into an actionable ticket reference (`web/src/components/CrashModal/CrashModal.tsx:37-50`).
- **Rate-limited failure marking**: ingestion-failure marks are capped per window to prevent metric cardinality/flooding during incidents (`packages/shared/src/server/redis/ingestionFailureTracking.ts:11-47`).
- **ESLint-enforced capture discipline**: importing `captureException` directly is restricted, forcing the classified seam (`web/eslint.config.mjs:48`).

## Tradeoffs

- **Detail vs. leak safety is inconsistent on REST fallbacks.** tRPC replaces 5xx messages, but both the ingestion handler and `withMiddlewares` return raw `error.message` to API clients on untyped 500s (`web/src/pages/api/public/ingestion.ts:236-241`, `web/src/features/public-api/server/withMiddlewares.ts:196-202`). More debuggable for SDK integrators, but leaks internal wording (table names, driver strings) that other boundaries deliberately mask.
- **Noise filters vs. hidden truth.** The beforeSend denylist is narrow and tested, but its 479-line filter module is a standing risk that a new genuine failure signature matches a rule; the repo mitigates this with per-rule rationale comments and tests asserting near-miss events survive (`web/src/utils/sentryFilters.ts`, `web/src/utils/sentryFilters.clienttest.ts:801-871`).
- **Toast-based mutation errors are ad hoc.** Individual components choose whether to show `err.message` or a static string (compare `web/src/ee/features/billing/components/BillingDiscountCodeButton.tsx:45` vs `web/src/features/setup/components/TracesSetupOnboardingCard.tsx:89`), so user-facing detail varies by feature team.
- **Metrics backend coupling.** Trace-correlated logging assumes Datadog field conventions (`dd.trace_id`), and CloudWatch export duplicates the dogstatsd path — useful on Langfuse Cloud, extra moving parts for minimal self-hosters (`packages/shared/src/server/instrumentation/index.ts:12-14,320-323`).

## Failure Modes / Edge Cases

- **Partial batch failure**: handled head-on — successes and failures are returned together per event id (`packages/shared/src/server/ingestion/processEventBatch.ts:462-524`); mixed-batch behavior under events_only mode is explicitly specified so score events in a rejected batch still process (`web/src/pages/api/public/ingestion.ts:150-153`).
- **Health check during fresh boot**: propagation-stuck evaluation treats "no heartbeat yet" as NOT stuck to avoid restart loops before the first scheduled run (`worker/src/features/health/index.ts:36-41`).
- **DLQ drain observability**: emptying a DLQ emits gauge 0 rather than vanishing, preventing alert flapping (`worker/src/queues/workerManager.ts:59-60`).
- **Redis outage during failure marking**: marking failures degrades to logged errors and skips silently when Redis is absent, so telemetry failure cannot take down ingestion (`packages/shared/src/server/redis/ingestionFailureTracking.ts:73-76,123-129`).
- **Rate limiter outage**: ingestion fails open — rate-limit errors are logged and processing continues (`web/src/pages/api/public/ingestion.ts:127-131`).
- **Unrecoverable jobs**: workers distinguish `UnrecoverableError` to stop pointless BullMQ retries (`worker/src/errors/UnrecoverableError.ts:3-17`).

## Future Considerations

- Unify the REST 500 fallback with the tRPC masking policy (or make exposure explicitly configurable) so internal message leakage is a decision, not an accident (`web/src/features/public-api/server/withMiddlewares.ts:196-202`).
- Introduce a shared client-side mutation-error seam (analogous to `reportError`) for toasts so detail level and copy are consistent across features.
- Document the operator health-probe matrix (which probe for liveness vs readiness vs restart) next to the DataDog recipes, since probe semantics live in docstrings today (`worker/src/features/health/index.ts:128-144`).

## Questions / Gaps

- No evidence found of a server-side Sentry `Sentry.init` in application runtime code within this snapshot (client init at `web/instrumentation-client.ts:11`; only `SENTRY_AUTH_TOKEN`/CSP keys exist in `web/src/env.mjs:429-430`). Server-side error aggregation appears to rely primarily on OTel/Datadog (`traceException`) — searched `Sentry.init`, `captureException`, `beforeSend` across `web/`.
- Whether the raw-`error.message` fallback in REST 500s has ever caused an incident or was a conscious choice could not be determined from the repository (no ADR found; no comments at the cited lines).
- Alert definitions (thresholds, routing) for the emitted metrics are externalized to the monitoring provider; only query recipes are in-repo (`.agents/skills/datadog-query-recipes/references/queue-consumers.md`), so actual detection coverage could not be verified from source alone.

---

Generated by `dimensions/13.03-failure-visibility` against `langfuse`.
