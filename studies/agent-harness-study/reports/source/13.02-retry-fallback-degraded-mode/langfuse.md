# Source Analysis: langfuse

## Dimension 13.02: Retry, Fallback, and Degraded Mode

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript; Next.js (web), Express + BullMQ (worker), shared package (`packages/shared`), Postgres / ClickHouse / Redis / S3 |
| Analyzed | 2026-08-25 |

## Summary

Langfuse implements retries as a layered concern rather than a single framework. At the base, every BullMQ queue carries declarative `attempts` + exponential `backoff` job options (e.g. `packages/shared/src/server/redis/ingestionQueue.ts:64-72`, `packages/shared/src/server/redis/evalExecutionQueue.ts:63-71`), and Redis connections retry reconnects forever with a capped exponential strategy (`packages/shared/src/server/redis/redis.ts:37-45`). On top of that sit purpose-built application-level retriers: an age-budgeted, jittered re-enqueue for LLM rate limits (`worker/src/features/utils/retry-handler.ts:106-258`), a bounded exponential retry for transient ClickHouse errors with batch-splitting and truncation degradation paths (`worker/src/services/ClickhouseWriter/index.ts:406-498`), and the `exponential-backoff` library applied at webhook, S3-multipart, Stripe-metering, and usage-aggregation call sites.

Degradation is handled through several explicit mechanisms: S3 `SlowDown` rate-limit errors trip a per-project Redis TTL flag that redirects that project's ingestion to an isolated secondary queue and fails open if the check itself errors (`packages/shared/src/server/redis/s3SlowdownTracking.ts:16-74`, `worker/src/queues/ingestionQueue.ts:111-136`); webhook automations auto-disable after consecutive delivery failures (a circuit-breaker pattern, `worker/src/queues/webhooks.ts:56-75`, `428-465`); blob-storage integrations atomically disable themselves on terminal customer-fault config errors (`worker/src/features/blobstorage/handleBlobStorageIntegrationProjectJob.ts:1595-1638`); oversized records are truncated rather than dropped (`worker/src/services/ClickhouseWriter/index.ts:216-286`); per-project ingestion sampling sheds load deterministically (`packages/shared/src/server/ingestion/processEventBatch.ts:358-370`); and the public ingestion API degrades to HTTP 207 partial-success semantics (`web/src/pages/api/public/ingestion.ts:179-182`). A dead-letter-retry cron replays failed delete/batch-action jobs every 10 minutes (`worker/src/services/dlq/dlqRetryService.ts:8-63`).

The one dimension area with **no evidence** is fallback model/provider chains: there is no `withFallback`-style configuration anywhere in the source; LLM failures are instead retried in-place within a time budget and then surfaced as terminal `ERROR` job statuses. The answer to "can the system survive a provider outage without failing all requests?" is largely yes for infrastructure providers (Redis fail-open rate limiting, queue-level retention + DLQ replay, secondary queues), but a prolonged ClickHouse write outage eventually drops buffered records after in-memory attempts are exhausted — an acknowledged gap (`worker/src/services/ClickhouseWriter/index.ts:544`).

## Rating

**8 / 10**

Rationale against the rubric:

- **Clear model with tests**: retry behavior is concentrated in reviewable units — `retryLLMRateLimitError` has a dedicated test suite covering schedule, age-out, attempt-cap, and unavailable-queue outcomes (`worker/src/features/utils/retry-handler.test.ts:49-401`); ClickHouse writer retry/truncate/split behavior is unit-tested (`worker/src/services/ClickhouseWriter/ClickhouseWriter.unit.test.ts:147,194,654`); queue-processor retry classification is tested end-to-end including DELAYED/ERROR state transitions (`worker/src/queues/__tests__/llmAsJudgeExecutionQueueProcessor.test.ts:210-441`).
- **Explicit interfaces**: error classification exposes `isRetryable` as part of typed error info (`packages/shared/src/server/llm/errors.ts:42-51`), retry schedules return discriminated unions (`RetryScheduleResult`, `worker/src/features/utils/retry-handler.ts:87-99`), and retry bookkeeping uses a zod-validated payload contract (`RetryBaggage`, `packages/shared/src/server/queues.ts:388-393`).
- **Operational safeguards**: metrics on every retry path (`recordDistribution(...).retries`, `.total_retry_delay_ms` at `worker/src/features/utils/retry-handler.ts:196-213`; drop counters at `worker/src/services/ClickhouseWriter/index.ts:545,550-556`), OTel span events per retry (`worker/src/services/ClickhouseWriter/index.ts:427-430`), and DLQ replay tooling.
- Not 9-10 because: no fallback provider/model capability at all; ClickHouse-writer retry state is in-memory and exhausted attempts silently drop data (explicit TODO for a dead-letter queue, `worker/src/services/ClickhouseWriter/index.ts:532-548`); retry policies are heterogeneous and partly hardcoded (webhook `numOfAttempts: 4` at `worker/src/queues/webhooks.ts:317`, observation-not-found constants at `worker/src/features/evaluation/retryObservationNotFound.ts:13-14`) rather than unified.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Queue-level retry defaults (ingestion) | `attempts: 6`, exponential backoff `delay: 5000`, failed jobs retained via `removeOnFail: 100_000` | `packages/shared/src/server/redis/ingestionQueue.ts:64-72` |
| Secondary ingestion queue retries | `attempts: 5`, same backoff shape | `packages/shared/src/server/redis/ingestionQueue.ts:146-154` |
| OTEL ingestion retries | Primary `attempts: 6` / secondary `attempts: 5`, exp delay 5000 | `packages/shared/src/server/redis/otelIngestionQueue.ts:69-77`, `152-160` |
| Eval execution retries | `attempts: 10`, exponential `delay: 1000` (both primary and secondary) | `packages/shared/src/server/redis/evalExecutionQueue.ts:63-71`, `150-158` |
| Redis connection retry strategy | Infinite reconnects, delay `min(max(e^t,1s),20s)`; `reconnectOnError` auto-retries READONLY | `packages/shared/src/server/redis/redis.ts:37-45`, `46-56` |
| LLM rate-limit retry handler | Age-budgeted re-enqueue with ±20% jitter, default 4 retries / 120 min window | `worker/src/features/utils/retry-handler.ts:106-258`, constants at `11-12`; env defaults `worker/src/env.ts:167-176` |
| Retry schedule math | Deterministic spread across budget window ("roughly 5m, 43m, 82m, 120m") with elapsed-time compensation | `worker/src/features/utils/retry-handler.ts:14-66` |
| Retry outcome contract | `RetryScheduleResult = scheduled \| skipped(too_old,max_attempts) \| queue_unavailable` | `worker/src/features/utils/retry-handler.ts:87-99` |
| RetryBaggage contract | zod schema `{ originalJobTimestamp, attempt }` persisted in job payloads | `packages/shared/src/server/queues.ts:388-393` |
| Observation-not-found retry | Exponential 30s·2^(n−1), max 5 attempts within 10 min | `worker/src/features/evaluation/retryObservationNotFound.ts:13-24`, gates at `48-78` |
| ClickHouse write retry + degradation | `backOff` with classifier: retryable socket/timeout, string-length → split batch & requeue half, size error → truncate once | `worker/src/services/ClickhouseWriter/index.ts:406-498` |
| Oversized-record truncation | Truncates input/output/metadata fields >1MB with marker text | `worker/src/services/ClickhouseWriter/index.ts:216-286` |
| Write-attempt exhaustion | Requeue with incremented attempts up to env max; else drop with metric + logged IDs; TODO for Redis DLQ | `worker/src/services/ClickhouseWriter/index.ts:532-572`; env default 3 `worker/src/env.ts:121-124` |
| ClickHouse read retry | `backOff` on network-error pattern list, `LANGFUSE_CLICKHOUSE_QUERY_MAX_ATTEMPTS` (default 3) | `packages/shared/src/server/repositories/clickhouse.ts:660-733`; `packages/shared/src/env.ts:379` |
| S3 multipart part retry | Transient-error pattern matcher + per-part `backOff` with `maxPartAttempts` env knob | `packages/shared/src/server/services/BufferedStreamUploader.ts:5-22`, `216-231`; knob `packages/shared/src/env.ts:253`, wired `packages/shared/src/server/services/StorageService.ts:810-811` |
| Webhook delivery retry | `backOff(numOfAttempts: 4)` around POST with abort-timeout; non-2xx throws to trigger next attempt | `worker/src/queues/webhooks.ts:231-318`, timeout at `238-241` |
| Webhook BullMQ retry gating | Only `LangfuseNotFoundError`/`InternalServerError` rethrow for BullMQ retry | `worker/src/queues/webhooks.ts:355-361` |
| Webhook circuit breaker | Consecutive-failure counter auto-disables automation trigger (≥5 failures, 24h TTL Redis counter or AutomationExecution history) | `worker/src/queues/webhooks.ts:56-75`, disable paths `369-401`, `428-465` |
| Blob-integration auto-disable | Terminal customer-fault errors flip `enabled true→false` atomically, tag reason, send notification email | `worker/src/features/blobstorage/handleBlobStorageIntegrationProjectJob.ts:1524-1638`, notification at `1659-1735` |
| S3 SlowDown detection | Error-shape + message matching for AWS SlowDown | `packages/shared/src/server/redis/s3SlowdownTracking.ts:16-33` |
| Per-project slowdown flag | Redis TTL flag (default 3600s), feature-flag gated, fails open on Redis error | `packages/shared/src/server/redis/s3SlowdownTracking.ts:39-74`; env `packages/shared/src/env.ts:342-348` |
| Secondary-queue redirect | Ingestion processor redirects flagged projects to secondary queue (env list or slowdown flag) | `worker/src/queues/ingestionQueue.ts:111-136` (flag set on error at `330-348`) |
| Producer-side slowdown marking | Upload rejection marks project slowdown + ingest failure | `packages/shared/src/server/ingestion/processEventBatch.ts:295-332` |
| Ingestion failure tracking | Redis ZSET of recently-failed projects with TTL feeding `langfuse.ingestion.project_failure.active_projects` gauge | `packages/shared/src/server/redis/ingestionFailureTracking.ts:66-130` |
| Ingestion sampling (load shed) | Per-project deterministic sampling drops events before enqueue, with in/out metrics | `packages/shared/src/server/ingestion/processEventBatch.ts:358-370`; env `packages/shared/src/env.ts:381+` |
| Partial success API | Public ingestion returns HTTP 207 with per-event errors; mixed batches process unaffected event types | `web/src/pages/api/public/ingestion.ts:150-182` |
| Rate-limiter fail-open | Rate-limit errors logged and ignored so ingestion continues | `web/src/pages/api/public/ingestion.ts:124-131` |
| Dead-letter retry cron | `DlqRetryService.retryDeadLetterQueue` replays failed jobs for 5 queues every ~10 min, histogram-instrumented | `worker/src/services/dlq/dlqRetryService.ts:9-63`; registration gated by env `worker/src/app.ts:639-650` |
| In-app-agent DLQ pacer | Delivery-only queue (`attempts: 1`) with periodic runner poking stale failed jobs past heartbeat window | `worker/src/features/in-app-agent-dlq-retry-runner/index.ts:13-91` |
| LLM error classification | `getLLMErrorInfo` unwraps AI SDK `RetryError` cause chains; `isRetryable` derived from provider error unless abort | `packages/shared/src/server/llm/errors.ts:58-149` |
| Evaluator error gating | `classifyEvaluatorLlmError` decides queue-retry vs terminal ERROR | `packages/shared/src/server/evals/classifyEvaluatorLlmError.ts:60-82` |
| Native AI SDK retry option | `maxRetries` plumbed into AI SDK call options; callers choose 0–1 | `packages/shared/src/server/llm/llmText.ts:115,343-359`; callers `web/src/features/search-bar/server/resolveFilterPrompt.ts:144`, `web/src/features/llm-api-key/server/router.ts:166`, `web/src/features/evals/v2/server/evaluators/testEvaluator.ts:120` |
| Code-eval retryable codes | Dispatcher classifies Lambda/user-code errors as `retryable` for BullMQ | `packages/shared/src/server/evals/awsLambdaCodeEvalDispatcher.ts:43-102`, `469` |
| Final-attempt detection helper | `job.opts.attempts` inspected to mask errors only on final attempt ("fail closed") | `worker/src/features/integrations/bullmqAttempts.ts:25` |
| Retry observability | Retry count + cumulative-delay distributions emitted per queue | `worker/src/features/utils/retry-handler.ts:195-213`; span attr `retryBaggage.attempt` asserted in `worker/src/queues/__tests__/llmAsJudgeExecutionQueueProcessor.test.ts:468-498` |
| Tests: retry handler | Schedules delays, skips too_old/max_attempts, falls back when queue missing/enqueue fails | `worker/src/features/utils/retry-handler.test.ts:49-401` |
| Tests: observation retry | Verifies exact exponential delays (30s→…) and growth beyond attempt 4 | `worker/src/__tests__/observation-retry.test.ts:25-32` |
| Tests: ClickHouse writer | Retry-on-error, client-timeout retry within flush, size-error truncation split | `worker/src/services/ClickhouseWriter/ClickhouseWriter.unit.test.ts:147,194,654-722` |
| Tests: queue processor states | DELAYED on scheduled retry; ERROR when not re-enqueued or queue unavailable; unexpected errors rethrown for BullMQ | `worker/src/queues/__tests__/llmAsJudgeExecutionQueueProcessor.test.ts:210-441` |
| Tests: multipart upload | Part retry succeeds on transient error; non-transient errors not retried | `worker/src/__tests__/bufferedStreamUploader.test.ts:145-191` |
| Tests: blob integration degrade | Fatal customer-fault disables integration w/ email; other faults keep retrying silently | `worker/src/__tests__/blobStorageIntegrationProcessing.test.ts:242-492` |
| Fallback models | No production fallback chain; only a Google-AI-Studio model-cycling test helper exists | `worker/src/__tests__/llmConnections.test.ts:87-98` (test-only) |
| Slack message fallback | Unknown/malformed webhook payloads render generic fallback blocks | `worker/src/features/slack/slackMessageBuilder.ts:137-203` |
| Cloud status degradation UI | Status page states incl. `degraded_performance` aggregated into banner status | `web/src/features/cloud-status-notification/server/cloud-status-router.ts:18,83-95` |

## Answers to Dimension Questions

**1. Are retries configurable?**
Largely yes, via environment variables validated through zod: `LANGFUSE_LLM_AS_JUDGE_QUEUE_RETRY_MAX_ATTEMPTS` (default 4) and `LANGFUSE_LLM_AS_JUDGE_QUEUE_RETRY_MAX_AGE_SECONDS` (default 7200) at `worker/src/env.ts:167-176`; `LANGFUSE_INGESTION_CLICKHOUSE_MAX_ATTEMPTS` (default 3) at `worker/src/env.ts:121-124`; `LANGFUSE_CLICKHOUSE_QUERY_MAX_ATTEMPTS` (default 3) at `packages/shared/src/env.ts:379`; `LANGFUSE_S3_UPLOAD_MAX_PART_ATTEMPTS` at `packages/shared/src/env.ts:253`; per-project redirect lists (`LANGFUSE_SECONDARY_INGESTION_QUEUE_ENABLED_PROJECT_IDS`, `worker/src/env.ts:112`) and the slowdown toggle/TTL (`packages/shared/src/env.ts:342-348`). However, several policies are hardcoded constants: webhook HTTP attempts (`numOfAttempts: 4`, `worker/src/queues/webhooks.ts:317`), observation-not-found limits (`worker/src/features/evaluation/retryObservationNotFound.ts:13-14`), Stripe metering attempts (`worker/src/ee/cloudUsageMetering/handleCloudUsageMeteringJob.ts:69`), and the LLM-retry jitter factor (`worker/src/features/utils/retry-handler.ts:12`). Queue `attempts`/`backoff` values are code constants per queue definition, not env-driven.

**2. Are fallback providers available?**
No evidence found. Searches for `withFallback`, `fallbackModel`, `fallback_model`, and `modelFallback` across all TypeScript sources returned no production matches (only a test helper that cycles Google AI Studio models on failure, `worker/src/__tests__/llmConnections.test.ts:87-98`, and UI/message fallbacks unrelated to providers). When LLM calls exhaust retries they transition to terminal `ERROR` job status with the provider message surfaced to users (`worker/src/queues/evalQueue.ts:241-259`); there is no secondary-model or secondary-provider chain. Degradation happens at the *infrastructure* level (secondary queues) rather than the *model* level.

**3. Does the system degrade gracefully?**
Yes, through multiple concrete mechanisms: (a) S3 rate-limit slowdown reroutes a noisy project's ingestion to an isolated secondary queue instead of letting it starve the primary (`worker/src/queues/ingestionQueue.ts:111-136`, flag logic `packages/shared/src/server/redis/s3SlowdownTracking.ts:39-74`); (b) oversized records are truncated with an explicit marker rather than dropped (`worker/src/services/ClickhouseWriter/index.ts:216-286`), and string-length errors halve the batch until writable (`180-214`); (c) per-project ingestion sampling sheds load before enqueue (`packages/shared/src/server/ingestion/processEventBatch.ts:358-370`); (d) the public API returns 207 partial-success so one bad event doesn't sink a batch (`web/src/pages/api/public/ingestion.ts:179-182`); (e) failing webhook automations self-disable after repeated failures (`worker/src/queues/webhooks.ts:428-465`); (f) rate limiter outages fail open (`web/src/pages/api/public/ingestion.ts:127-131`); (g) Redis seen-cache write failures log-and-continue (`worker/src/queues/ingestionQueue.ts:244-264`).

**4. Are circuit breakers used to prevent cascading failure?**
Yes — two breaker-like patterns exist, though neither uses that name. First, the S3 slowdown flag is a per-project, TTL-expiring breaker: sustained `SlowDown` errors open the "circuit" (Redis key, `packages/shared/src/server/redis/s3SlowdownTracking.ts:45-46`), traffic reroutes to the secondary queue while open, and it automatically closes on TTL expiry (default 1h, `packages/shared/src/env.ts:345-348`). Second, the webhook automation auto-disable is a consecutive-failure breaker: a Redis INCR counter with 24h TTL opens after ≥5 failures and disables the trigger entirely, reset on first success (`worker/src/queues/webhooks.ts:56-84`, `369-401`), with the equivalent history-based variant for other action types (`428-465`). Additionally, blob-storage integrations hard-disable on classified terminal customer faults with reason-tagged metrics (`worker/src/features/blobstorage/handleBlobStorageIntegrationProjectJob.ts:1524-1638`). There is no generic library-level circuit breaker around arbitrary outbound calls.

**Bonus (step 5): Is retry state persisted?**
Partially. Application-level retries persist their state two ways: `RetryBaggage` (`attempt`, `originalJobTimestamp`) rides inside the re-enqueued BullMQ job payload (`packages/shared/src/server/queues.ts:388-393`, written at `worker/src/features/utils/retry-handler.ts:172-183`) and thus survives worker restarts in Redis; eval executions additionally record a `DELAYED` status row in Postgres while a retry is pending (`worker/src/queues/evalQueue.ts:225-238`). Failed jobs remain enumerable for replay via `removeOnFail` retention (100k/10k) and the DLQ cron (`worker/src/services/dlq/dlqRetryService.ts:34-61`). By contrast, the ClickhouseWriter's per-record attempt counters live only in process memory (`worker/src/services/ClickhouseWriter/index.ts:662-666`): a restart resets them, and exhausted attempts drop rows with only a metric and logged IDs as trace (`532-572`). Underlying raw events do survive independently in the S3 event log referenced by `fileKey` in retained queue payloads (`worker/src/queues/ingestionQueue.ts:63-82`), enabling redelivery-based recovery.

## Architectural Decisions

1. **Two-tier retry design.** Declarative BullMQ `attempts`+`backoff` covers mechanical/transient failures, while semantic retries (rate limits, missing observations) are implemented as deliberate re-enqueues with fresh job IDs and baggage, keeping BullMQ's attempt counter meaningful for unexpected errors only (`worker/src/queues/evalQueue.ts:177-269` decision diagram at `179-202`).
2. **Time-budgeted rather than purely count-based retries.** The LLM retry handler bounds retries by wall-clock age computed from a persisted creation timestamp plus initial queue delay (`worker/src/features/utils/retry-handler.ts:132-150`), preventing unbounded backlog growth during long provider outages.
3. **Isolation over throttling.** Instead of per-project token buckets, misbehaving projects are routed to sharded secondary queues (also used proactively via env allowlists), containing blast radius without rejecting traffic (`worker/src/queues/ingestionQueue.ts:111-136`; shard counts `packages/shared/src/env.ts:173-194`).
4. **Error classification as a first-class interface.** `isRetryable`/`retryable` fields flow from provider-error introspection (`packages/shared/src/server/llm/errors.ts:86-92`), dispatcher error tables (`packages/shared/src/server/evals/awsLambdaCodeEvalDispatcher.ts:43-102`), and message-pattern matchers (`worker/src/services/ClickhouseWriter/index.ts:137-171`), so processors can distinguish terminal user errors from transient infra faults.
5. **Durability boundary at S3 + queue, not in-process buffers.** Events are persisted to object storage before enqueue (`packages/shared/src/server/ingestion/processEventBatch.ts:279-332`), which makes queue-level retries safe and means the lossy in-memory ClickHouse buffer is a cache in front of durable storage.
6. **Fail-open where availability beats strictness** (rate limiting, slowdown-flag reads, seen-cache writes) versus **fail-closed for data integrity** (S3 upload failure aborts the batch after logging, `processEventBatch.ts:328-332`).

## Notable Patterns

- **Jittered, deterministic retry ladders**: delays are computed to spread attempts evenly across the remaining budget window and multiplied by a random ±20% jitter factor (`worker/src/features/utils/retry-handler.ts:41-66`).
- **Retry-as-new-job**: every application retry enqueues a brand-new job with a `randomUUID()` id and carries `retryBaggage` forward, preserving idempotency and making each attempt independently observable (`worker/src/features/utils/retry-handler.ts:220-230`).
- **Adaptive write strategies inside a retry loop**: the ClickHouse writer mutates its own payload between attempts — splitting halves on string-length errors, truncating fields on size errors — using the retry callback as a repair hook (`worker/src/services/ClickhouseWriter/index.ts:432-484`).
- **Compensating degradation ladder**: single-item string-length failures fall back to truncation to guarantee termination (`worker/src/services/ClickhouseWriter/index.ts:187-203`).
- **TTL-keyed breakers in Redis**: both the slowdown flag and automation-failure counters use Redis TTLs so circuits heal without a background closer (`packages/shared/src/server/redis/s3SlowdownTracking.ts:44-47`; `worker/src/queues/webhooks.ts:68-75`).
- **Metrics-first failure handling**: nearly every retry/skip/drop branch emits `recordDistribution`/`recordIncrement`/`recordHistogram` (e.g. `worker/src/features/utils/retry-handler.ts:195-213`; `worker/src/services/dlq/dlqRetryService.ts:43-49`), plus a dedicated gauge of how many projects currently have active ingest failures (`packages/shared/src/server/redis/ingestionFailureTracking.ts:101-130`).
- **Final-attempt awareness**: processors consult `job.opts.attempts` to mask internal error messages only on the last attempt, keeping retryable noise out of user-facing state (`worker/src/features/integrations/bullmqAttempts.ts:25`; usage `worker/src/queues/__tests__/codeEvalExecutionQueueProcessor.test.ts:205-248`).

## Tradeoffs

- **In-memory ClickHouse buffering vs durability**: batching in process memory gives high write throughput but means crashes lose buffered records and restart resets attempt counters; the authors explicitly acknowledge the missing DLQ with a TODO (`worker/src/services/ClickhouseWriter/index.ts:544`).
- **Secondary queues vs added complexity**: isolation requires duplicated queue definitions, shard-count env vars, and redirect plumbing across web/worker/shared (visible in the many near-identical queue files under `packages/shared/src/server/redis/`).
- **Fail-open rate limiting vs cost control**: continuing on rate-limiter errors protects ingestion availability but can let traffic exceed limits during limiter outages (`web/src/pages/api/public/ingestion.ts:127-131`).
- **Truncation vs fidelity**: oversized inputs/outputs are cut at ~500KB with a marker, trading data completeness for write success (`worker/src/services/ClickhouseWriter/index.ts:220-233`).
- **Message-sniffing error classification vs robustness**: retryability decided by substring matches (`"socket hang up"`, `"Timeout error"`) is simple but brittle across SDK upgrades (`worker/src/services/ClickhouseWriter/index.ts:137-149`).
- **Feature-gated protection**: the S3 slowdown breaker defaults to off (`LANGFUSE_S3_RATE_ERROR_SLOWDOWN_ENABLED` default `"false"`, `packages/shared/src/env.ts:342-344`), so most deployments don't get that safeguard unless operators opt in.
- **DLQ cron breadth**: only five queues participate in automatic failed-job replay (`worker/src/services/dlq/dlqRetryService.ts:9-15`); other queues' failures rely on manual intervention despite 100k-job retention.

## Failure Modes / Edge Cases

- **Prolonged ClickHouse outage**: buffered records exhaust `LANGFUSE_INGESTION_CLICKHOUSE_MAX_ATTEMPTS` in-memory and are dropped permanently (with metric + ID log), even though the queue job may later succeed on a different sub-operation (`worker/src/services/ClickhouseWriter/index.ts:532-572`).
- **Retry queue unavailable**: if the BullMQ queue instance can't be created (e.g., Redis down at instantiation), the retry handler returns `queue_unavailable` and the processor falls through to terminal ERROR handling — the job stops retrying despite being retryable (`worker/src/features/utils/retry-handler.ts:185-193`; tested `worker/src/queues/__tests__/llmAsJudgeExecutionQueueProcessor.test.ts:288-292`).
- **Enqueue failure mid-retry**: a thrown `queue.add` during retry scheduling also degrades to normal error handling (`worker/src/features/utils/retry-handler.ts:231-240`).
- **Clock/age skew**: the retry budget starts at dataset-run/job-execution creation time read from Postgres; if that lookup itself throws, the whole retry handler falls to `queue_unavailable` via the outer catch (`worker/src/features/utils/retry-handler.ts:247-257`).
- **Lost disable races**: concurrent integration disable/user toggle during fault handling is detected via conditional update count and reported distinctly (`worker/src/features/blobstorage/handleBlobStorageIntegrationProjectJob.ts:1623-1626`).
- **Too-fresh agent corpses**: the in-app-agent DLQ pacer deliberately skips jobs that failed within the heartbeat-stale window to avoid ACKing runs whose persistence hasn't reconciled yet (`worker/src/features/in-app-agent-dlq-retry-runner/index.ts:16-20`, `83-91`).
- **Azure parity gap**: `maxPartAttempts` silently has no effect on Azure uploads (SDK owns retries); a warning is logged so operators aren't surprised (`packages/shared/src/server/services/StorageService.ts:447-451`).
- **Duplicate-delivery inflation**: expected validation drops on redelivered batches are excluded from failure metrics to avoid counting once per retry (`worker/src/services/IngestionService/index.ts:725`).

## Future Considerations

- Implement the acknowledged Redis-backed dead-letter queue for ClickHouseWriter drops so max-attempt exhaustion is recoverable rather than lossy (`worker/src/services/ClickhouseWriter/index.ts:544`).
- Introduce configurable fallback model/provider chains for LLM-as-judge evaluations (currently absent; retries are same-model-only), which would directly address the "survive a provider outage" question for the AI-dependent features.
- Unify the scattered retry policies (per-queue literals, hardcoded attempt counts, ad-hoc `withRetry` wrappers like `worker/src/ee/usageThresholds/usageAggregation.ts:27-40`) behind a shared, documented policy type.
- Enable the S3 slowdown breaker by default or document the operational risk of leaving it off (`packages/shared/src/env.ts:342-344`).
- Persist ClickhouseWriter attempt counters outside process memory (e.g., derive from job attempt metadata) so worker restarts don't grant extra retry budget.

## Questions / Gaps

- **No fallback-provider mechanism found**: searches for fallback model/provider configuration (`withFallback*`, `fallbackModel*`) returned only test helpers and UI message fallbacks. If fallback exists, it would have to live outside this repository (e.g., in a gateway behind a user-configured `baseURL`, which `packages/shared/src/server/llm/llmText.ts:285-299` does support via OpenRouter-style endpoints).
- **Webhook retry backoff curve unspecified in code**: `numOfAttempts: 4` relies on `exponential-backoff` defaults (500ms base, ×2); no explicit delay tuning or `Retry-After` header honor was found in `worker/src/queues/webhooks.ts:231-318`.
- **No evidence of persisted state for observation-not-found retries beyond job payload**: the 5-attempt/10-min budget lives entirely in `retryBaggage` within Redis-held jobs (`worker/src/features/evaluation/retryObservationNotFound.ts:83-128`); acceptable given the short window, but noted for completeness.
- **Client-side SDK retries** (the JS/Python ingestion SDKs' own backoff) are out of scope for this repo and were not evaluated; only the server-side acceptance path (`web/src/pages/api/public/ingestion.ts`) was assessed.

---

Generated by `Dimension 13.02: Retry, Fallback, and Degraded Mode` against `langfuse`.
