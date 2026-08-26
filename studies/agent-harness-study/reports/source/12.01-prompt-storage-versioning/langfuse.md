# Source Analysis: langfuse

## Dimension 12.01 — Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo (Next.js web + BullMQ worker + shared package; PostgreSQL/Prisma, ClickHouse, Redis) |
| Analyzed | 2026-08-25 |

## Summary

Langfuse treats prompts as first-class, versioned data stored in PostgreSQL, not as code artifacts. The `Prompt` Prisma model (`packages/shared/prisma/schema.prisma:721-748`) stores prompt content as JSON alongside an integer `version`, mutable `labels`/`tags` arrays, a structured `config`, and an optional `commitMessage`. Versions are append-only: creating a "new version" always inserts a new row with `max(version)+1` under a `[projectId, name, version]` unique constraint (`packages/shared/prisma/schema.prisma:742`, `web/src/features/prompts/server/actions/createPrompt.ts:106-109,170`); content is never mutated in place. Reads go through a Redis-backed `PromptService` (`packages/shared/src/server/services/PromptService/index.ts:22`) whose cache is invalidated via project-scoped epoch-token rotation rather than key deletion. Run-to-prompt traceability is built into ingestion: generation events carry `promptName`/`promptVersion`, the worker resolves them to the canonical `promptId` at ingest time (`worker/src/services/IngestionService/index.ts:321-333,405-408`), and persists `prompt_id`/`prompt_name`/`prompt_version` columns on ClickHouse observations (`packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:25-27`). Prompts can be created, promoted (label changes), listed for sync, exported/imported as JSON, and changed entirely without redeploying application code, through a versioned public REST API (Fern-defined), the UI, and MCP tools.

## Rating

**9 / 10**

Rationale against the rubric:

- **Clear model**: storage (Postgres rows), identity (`projectId, name, version` unique constraint, `schema.prisma:742`), and promotion semantics (unique-across-versions labels like `production`/`latest`, reserved-label rules in `web/src/features/prompts/server/handlers/promptVersionHandler.ts:9-16`) are explicit and enforced.
- **Tests proving behavior under failure**: dedicated cache tests inject Redis failures and verify DB fallback (`web/src/__tests__/server/promptCache.servertest.ts:237-290`), verify numeric labels do not collide with versions in cache (`promptCache.servertest.ts:105`), and API tests cover production-label default fetch, label overwrite, and run association (`web/src/__tests__/server/prompts.v2.servertest.ts:304,363,421,771`).
- **Operational safeguards**: audit logging on create/update/delete of prompts (`web/src/features/prompts/server/handlers/promptNameHandler.ts:89-99`, `web/src/features/prompts/server/prompt-api-service.ts:74-82,121-130`), protected-label configuration blocking accidental overwrites (`web/src/features/prompts/server/utils/checkHasProtectedLabels.ts:11-25`), concurrency conflict surfaced as retryable error (`createPrompt.ts:228-236`), row locking during label moves (`web/src/features/prompts/server/actions/updatePrompts.ts:42`).
- **Observability**: cache hit/miss metrics emitted per read (`PromptService/index.ts:57-61,413-419`); change events queued for webhook automations with persisted execution records (`worker/src/features/entityChange/promptVersionProcessor.ts:156-199`).
- **Why not 10**: legacy `isActive` column still lingers deprecated (`schema.prisma:735`); any prompt write invalidates all cached prompts project-wide (`PromptService/index.ts:216-220`); post-commit event-sourcing/webhook failures are logged but not retried inline (`createPrompt.ts:273-287`); cross-version label uniqueness is enforced in app transactions, not by a DB constraint; there is no first-class environment dimension on prompts.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Prompt storage model | `model Prompt` with `prompt Json`, `version Int`, `config Json`, `tags String[]`, `labels String[]`, `commitMessage String?` | `packages/shared/prisma/schema.prisma:721-748` |
| Version uniqueness | `@@unique([projectId, name, version])` on prompts table | `packages/shared/prisma/schema.prisma:742` |
| Prompt composition table | `PromptDependency` (parentId → childName/childLabel/childVersion) | `packages/shared/prisma/schema.prisma:750-768` |
| Label protection config | `PromptProtectedLabels` per-project label registry | `packages/shared/prisma/schema.prisma:770-780` |
| Canonical label constants | `PRODUCTION_LABEL = "production"`, `LATEST_PROMPT_LABEL = "latest"` | `packages/shared/src/features/prompts/constants.ts:1-2` |
| Auto-increment versioning | New version = latest found by desc order + 1, else 1 | `web/src/features/prompts/server/actions/createPrompt.ts:106-109,146,170` |
| Append-only creation | Prompt + dependency rows written in one transaction; P2002 conflict → retryable `LangfuseConflictError` | `web/src/features/prompts/server/actions/createPrompt.ts:160-188,222-236` |
| `latest` auto-managed | Newly created prompts always labeled `latest`; previous versions stripped of it | `web/src/features/prompts/server/actions/createPrompt.ts:129,190-203` |
| Tag consistency across versions | Tags propagated to all prior versions on create | `web/src/features/prompts/server/actions/createPrompt.ts:205-219` |
| commit message support | `commitMessage` accepted on create schemas (text & chat) | `packages/shared/src/features/prompts/types.ts:39-41,54-56` |
| Read path with cache | `PromptService.getPrompt`: cache-first, DB fallback, re-cache | `packages/shared/src/server/services/PromptService/index.ts:49-81` |
| Lookup by version XOR label | `findPrompt` branches on exact `version` vs `labels has` | `packages/shared/src/server/services/PromptService/index.ts:102-130` |
| Cache keying | `prompt:<projectId>:<epoch>:<name>:version:N\|label:L` | `packages/shared/src/server/services/PromptService/index.ts:194-214` |
| Cache invalidation | Epoch token rotation with 7-day TTL instead of key deletes | `packages/shared/src/server/services/PromptService/index.ts:26-28,179-192,216-240` |
| Cache enablement env | `LANGFUSE_CACHE_PROMPT_ENABLED`, `LANGFUSE_CACHE_PROMPT_TTL_SECONDS` | `packages/shared/src/server/services/PromptService/index.ts:42-46` |
| Dependency graph resolution | Recursive resolution of `@@@langfusePrompt:name=...\|version=...@@@` tags with depth cap 5 and cycle detection | `packages/shared/src/server/services/PromptService/index.ts:19,242-403` |
| Default fetch = production | No version/label given → resolves `production` label | `web/src/features/prompts/server/actions/getPromptByName.ts:49-55` |
| Public GET/DELETE v2 | Auth, rate limit, `isActive` derived from production label; DELETE audited per version | `web/src/features/prompts/server/handlers/promptNameHandler.ts:16-58,61-111` |
| Public PATCH labels | `PATCH /prompts/{name}/versions/{version}` rejects `latest`; label-only mutation | `web/src/features/prompts/server/handlers/promptVersionHandler.ts:9-33` |
| Legacy v1 compat | v1 route maps boolean `isActive` ⇄ `production` label | `web/src/pages/api/public/prompts.ts:73-76,91-101` |
| List/sync metadata API | `GetPromptsMetaSchema` incl. `fromUpdatedAt`/`toUpdatedAt` filters for external sync | `packages/shared/src/features/prompts/types.ts:79-88` |
| Fern contract source | Prompt endpoints defined in API definition files (regenerated clients, not hand-edited) | `fern/apis/server/definition/prompt-version.yml:12-40`, `fern/apis/server/definition/prompts.yml` |
| Ingestion schema carries prompt ref | Generation bodies require `promptName`+`promptVersion` together or neither | `packages/shared/src/server/ingestion/types.ts:493-500` |
| Run→prompt pinning at ingest | Worker looks up prompt id by name+version before persisting | `worker/src/services/IngestionService/index.ts:320-333` |
| Observation-level columns | `prompt_id`, `prompt_name`, `prompt_version` persisted on observations | `packages/shared/clickhouse/migrations/unclustered/0002_observations.up.sql:25-27` |
| Domain model fields | `promptId`, `promptName`, `promptVersion` nullable zod fields | `packages/shared/src/domain/observations.ts:77-79` |
| Association test | "should relate generation to prompt": ingested generation ends up with resolved `promptId` | `web/src/__tests__/server/prompts.v2.servertest.ts:421-489` |
| Cache failure-mode tests | Redis get/set errors fall back to DB safely; disabled cache and null-Redis paths tested | `web/src/__tests__/server/promptCache.servertest.ts:105,152,186-235,237-290` |
| Change event sourcing | Every create/update enqueues `entityType: "prompt-version"` events | `web/src/features/prompts/server/promptChangeEventSourcing.ts:14-56` |
| Webhook deployment hook | Prompt triggers filter by name/action, enqueue webhook payloads containing full prompt version | `worker/src/features/entityChange/promptVersionProcessor.ts:38-113,180-199` |
| JSON import/export | UI import accepts "JSON file exported from Langfuse", batch-capped | `web/src/features/prompts/components/ImportPromptsButtonDialogController.tsx:28-33,163` |
| MCP access surface | MCP tools: get/list/create text & chat prompts, update labels | `web/src/features/mcp/features/prompts/tools/getPrompt.ts`, `listPrompts.ts`, `createTextPrompt.ts`, `updatePromptLabels.ts` |
| Deletion safety | Deletion checks dependent prompts before removing versions | `web/src/features/prompts/server/actions/deletePrompt.ts:13-50` |
| Internal app prompt (contrast) | Langfuse's own LLM feature ships one static JSON prompt file in code — exception, not user-prompt mechanism | `web/src/features/search-bar/server/prompts/search-bar-filter.prompt.json:1` |

## Answers to Dimension Questions

**1. Where are prompts stored?**
Prompts live as rows in PostgreSQL (`prompts` table via the `Prompt` model, `packages/shared/prisma/schema.prisma:721-748`) — prompt bodies are JSON data (`prompt Json` for text or chat-message arrays), never compiled into application code. Two satellite tables support the model: `PromptDependency` records composition edges (`schema.prisma:750-768`) and `PromptProtectedLabels` stores per-project protected-label settings (`schema.prisma:770-780`). At runtime, resolved prompts are cached in Redis keyed by project/epoch/name/version-or-label (`PromptService/index.ts:194-214`). The only prompt-in-code instance found is an internal JSON file powering Langfuse's own search-bar LLM feature (`web/src/features/search-bar/server/prompts/search-bar-filter.prompt.json:1`), which is application plumbing, distinct from the user-facing prompt store.

**2. Are prompt versions tracked?**
Yes — integer versions namespaced per project and name, unique at the DB level (`schema.prisma:742`). Creation is strictly append-only: the service reads the current max version and inserts N+1 (`createPrompt.ts:106-109,170`); concurrent creations hitting the unique constraint produce an explicit retryable conflict (`createPrompt.ts:55-69,228-236`). There is no content-edit operation: only `labels` (with cross-version uniqueness maintained transactionally, `createPrompt.ts:190-203`, `web/src/features/prompts/server/utils/updatePromptLabels.ts:3-38`, guarded by `FOR UPDATE` locks in `updatePrompts.ts:42`) and `tags` (kept uniform across all versions, `createPrompt.ts:205-219`) mutate. Each version optionally carries a human-authored `commitMessage` (`types.ts:40,55`), and the UI renders version history/diffs (`web/src/features/prompts/components/timeline.tsx`, `PromptVersionDiffDialog.tsx`, `prompt-history.tsx`).

**3. Can a run be traced to the exact prompt version used?**
Yes. The ingestion contract accepts `promptName` and `promptVersion` on generations as an all-or-nothing pair (`packages/shared/src/server/ingestion/types.ts:493-500`). During ingestion the worker resolves that pair to the canonical row via `PromptService.getPrompt(...)` and stamps `prompt_id`, `prompt_name`, and `prompt_version` onto the ClickHouse observation record (`worker/src/services/IngestionService/index.ts:320-333,405-408`); the columns exist since the original observations migration (`0002_observations.up.sql:25-27`). The end-to-end guarantee is covered by the integration test "should relate generation to prompt" (`prompts.v2.servertest.ts:421-489`), which asserts the stored observation's `promptId` equals the created prompt's id. Because `prompt_id` pins the immutable `(name, version)` row, later label promotions do not retroactively rewrite history. One nuance: composite prompts resolved from dependency graphs are recorded only by their root id/name/version on the run; the child versions consumed during resolution appear in the returned `resolutionGraph` (`PromptService/index.ts:250-257,350-356`) but are not individually stamped onto the observation.

**4. Can prompts be updated without redeploying code?**
Yes, through four independent channels: (a) the versioned public REST API — list/get/create/delete plus `PATCH .../versions/{version}` for label promotion (`fern/apis/server/definition/prompt-version.yml:12-40`, `promptVersionHandler.ts:16-33`), with a legacy v1 route translating `isActive` booleans for old clients (`web/src/pages/api/public/prompts.ts:80-107`); (b) the web UI (create, duplicate folders with reference rewriting, label/tag management, JSON import — `ImportPromptsButtonDialogController.tsx:163`); (c) MCP tools exposing get/list/create/update-labels (`web/src/features/mcp/features/prompts/tools/*`); (d) outbound automation — every create/update emits a change event (`promptChangeEventSourcing.ts:14-56`) that trigger-matching turns into webhooks carrying the full prompt version payload (`promptVersionProcessor.ts:38-113,180-199`), enabling downstream CI/CD pipelines. The list endpoint's `fromUpdatedAt`/`toUpdatedAt` filters explicitly support external synchronization sweeps (`types.ts:86-87`). Deploying new *application* code is never required to change what `production` serves; only adding new server-side behavior is.

## Architectural Decisions

1. **Prompts-as-data in relational storage, immutable per version.** Content lives in Postgres JSON columns with append-only versioning enforced by a composite unique key (`schema.prisma:731-742`); mutations of meaning require a new row, making every historical version reproducible.
2. **Label-based indirection over direct version references for consumers.** SDKs fetch by `production`/`latest` label by default (`getPromptByName.ts:49-55`); promoting a version is a metadata patch (`promptVersionHandler.ts:16-33`), decoupling rollout from code deployment while ingestion-time pinning keeps run history exact.
3. **Epoch-rotation cache invalidation.** Instead of deleting keys, writes rotate a project-scoped epoch token embedded in every cache key; stale entries expire naturally via TTL (`PromptService/index.ts:179-240`). This avoids scan-and-delete races across a cluster and handles transitive dependencies (the epoch is deliberately project-scoped, `index.ts:216-220`).
4. **Resolve-at-write validation for prompt composition.** Dependency tags are parsed and the whole graph resolved inside the create transaction boundary, so unresolvable/cyclic composites are rejected before persistence (`createPrompt.ts:136-158`, depth cap and cycle checks at `PromptService/index.ts:265-283`).
5. **Event-driven extensibility for downstream deployment.** A generic entity-change queue fans out prompt-version events to user-configured triggers/webhooks with persisted execution records (`promptVersionProcessor.ts:126-199`), treating "prompt changed" as an integration point rather than baking CI/CD into the core.

## Notable Patterns

- **Reserved-label governance:** `latest` cannot be set manually (`promptVersionHandler.ts:9-16`) and is force-assigned/stripped on create (`createPrompt.ts:129,190-203`); organizations can additionally protect arbitrary labels (e.g., `production`) from overwrite via `PromptProtectedLabels` (`checkHasProtectedLabels.ts:11-25`).
- **Cache-key selector hygiene:** numeric labels are deliberately isolated from equal-valued version selectors to prevent aliasing (`prompt:`...`:version:N` vs `label:L`, `PromptService/index.ts:200-206`), with a regression test (`promptCache.servertest.ts:105`).
- **Graceful degradation:** every Redis interaction in the read/write paths is wrapped in try/catch falling back to Postgres (`PromptService/index.ts:149-177,237-257`), and the cache can be fully disabled via env or null client (`index.ts:39-46`, tested at `promptCache.servertest.ts:186-235`).
- **Contract-first public API:** request/response schemas in `packages/shared/src/features/prompts/types.ts` mirror Fern definitions under `fern/apis/server/definition/`, with generated clients kept out of hand-edits (root `AGENTS.md` policy).
- **Backward compatibility shims:** the deprecated `isActive` flag survives only as a derived view of the `production` label at the API edge (`promptNameHandler.ts:57`, `prompts.ts:75,94-96`).

## Tradeoffs

- **Project-wide invalidation vs fine-grained eviction:** any prompt write rotates the epoch for *all* prompts of a project (`PromptService/index.ts:216-220`), trading cache efficiency for simplicity and correctness under transitive dependencies — a cold-cache thundering herd follows bulk imports.
- **App-enforced label uniqueness:** only one version may hold a given label, but this invariant lives in transactional app logic (`createPrompt.ts:190-203`, `updatePrompts.ts` row locking) rather than a DB constraint, concentrating correctness risk in code paths that are harder to prove than schema rules.
- **Best-effort post-commit side effects:** cache invalidation failure after commit is logged, not retried (`createPrompt.ts:241-248`), leaving staleness bounded only by TTL; webhook/event publication uses `Promise.allSettled` with error logging (`createPrompt.ts:273-287`), relying on queue retries once enqueued but silently dropping enqueue failures.
- **Root-only run attribution for composites:** runs record the root prompt version; child prompt versions pulled in by dependency resolution are visible in the response-time `resolutionGraph` but not pinned onto the observation, slightly weakening forensic precision for composed prompts.
- **No environment dimension:** unlike traces/observations which carry `environment` (`IngestionService/index.ts:380`), prompts distinguish deployments solely through ad-hoc labels, pushing environment-promotion conventions onto users.

## Failure Modes / Edge Cases

- **Concurrent same-version creation:** two simultaneous creates compute the same next version; loser receives `LangfuseConflictError` ("A prompt version was created concurrently. Please retry.", `createPrompt.ts:228-236`), and the API layer converts residual P2002s into a backoff-hinting message (`prompt-api-service.ts:54-72`).
- **Redis outage mid-flight:** reads transparently hit Postgres and skip caching; writes still succeed (`promptCache.servertest.ts:237-290`); `invalidateCache` failing hard surfaces as a logged post-commit error (`createPrompt.ts:241-248`) — worst case is TTL-bounded staleness, not wrong-data corruption of the source of truth.
- **Numeric-label ambiguity:** a label like `"3"` could alias version 3; prevented by distinct cache-key namespaces (`PromptService/index.ts:200-206`) and tested (`promptCache.servertest.ts:105`).
- **Cyclic or too-deep composition:** self-referential or >5-deep dependency graphs raise `LangfuseConflictError` at both create and resolve time (`PromptService/index.ts:265-283`); deleting a referenced prompt version is blocked by dependency checks (`deletePrompt.ts:13-50`).
- **Type drift within a prompt name:** mixing `text` and `chat` types across versions of the same name is rejected (`createPrompt.ts:111-115`), keeping consumers' parsing assumptions safe.
- **Legacy field confusion:** `isActive` remains in the schema as deprecated (`schema.prisma:735`); duplicated prompts set it defensively true (`createPrompt.ts:369`) even though the API derives status from labels — a lingering dual-source-of-truth risk if read directly from DB.

## Future Considerations

- Replace project-wide epoch invalidation with name-scoped invalidation now that dependency edges are materialized in `PromptDependency`, cutting cache churn for large projects.
- Enforce cross-version label uniqueness at the database level (e.g., partial unique index per project/name/label) to remove reliance on transactional discipline.
- Persist the full resolved dependency set (child prompt ids/versions) onto observations at ingestion time so composite-provenance queries don't depend on recomputing resolution graphs.
- Retire the deprecated `isActive` column once label semantics have fully replaced it (`schema.prisma:735` marks this intent).
- Add durable outbox semantics for prompt change events so webhook/automation delivery survives enqueue-time failures (`createPrompt.ts:273-287`).

## Questions / Gaps

- **Client-side caching in official SDKs:** the SDK packages (fetching prompts, local TTL caches) are separate repositories and are not part of this source tree, so end-to-end "which version did the client actually use" behavior beyond the ingestion contract could not be verified here; searched `packages/` and `web/` for SDK runtime code and found none.
- **Prompt experiment attribution:** experiments reference prompts by id (`web/src/features/experiments/server/router.ts:121-148`) and resolve them at run creation; whether each experiment *item execution* re-pins to the same resolved row was not traced further (dataset-run item persistence lives outside the files inspected).
- **Retention/deletion policy for prompt versions:** no scheduled cleanup job for orphaned prompt rows was found (searched `worker/src/features/*` for prompt retention); deletion appears purely user-initiated via API/UI (`deletePrompt.ts`), so unbounded version accumulation may be possible.

---

Generated by `dimensions/12.01-prompt-storage-versioning.md` (Dimension 12.01) against `langfuse`.
