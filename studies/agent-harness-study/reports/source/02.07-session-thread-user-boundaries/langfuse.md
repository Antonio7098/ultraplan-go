# Source Analysis: langfuse

## Session, Thread, and User Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript, Next.js (Pages Router) + tRPC + worker; Postgres (Prisma) + ClickHouse + Redis |
| Analyzed | 2026-07-10 |

## Summary

Langfuse is an LLM observability/engineering platform, not a single-agent harness, so its "session/thread/user" boundaries exist at two distinct layers. The **platform tenancy layer** scopes all state through a strict `Organization -> Project` hierarchy with membership + RBAC roles (`packages/shared/prisma/schema.prisma:95-181`, `318-380`). The **observability data layer** treats `session_id` and `user_id` as *tracked-application* attributes (end-user of the customer's app), which are always sub-keys under `project_id` in both Postgres and ClickHouse (`packages/shared/prisma/schema.prisma:382-395`, `packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:9-31`).

The closest thing to an agent "thread" is the EE **in-app agent** feature: `InAppAgentConversation` / `InAppAgentRun` / `InAppAgentEvent`, keyed by `(id, projectId)` and owned by `createdByUserId` (`packages/shared/prisma/schema.prisma:218-277`). Isolation is enforced entirely at the **application layer**: every tRPC and REST request is filtered by `projectId` (there is no Postgres row-level security), with the authenticated session/API-key supplying the allowed project set. This is backed by explicit cross-user and cross-project isolation tests (`web/src/__tests__/server/in-app-agent-persistence.servertest.ts:1248-1332`).

## Rating

**7/10** — Clear, consistent scoping model with explicit interfaces (composite `(id, projectId)` keys, `ApiKeyScope`, RBAC roles) and dedicated isolation tests. Points held back because isolation is *purely application-enforced* (no DB-level RLS, so a missing `projectId`/`userId` filter in any new query silently leaks), the platform `Session` (NextAuth) and observability `TraceSession` share a confusing name, and there is no automated guard preventing an unscoped ClickHouse/Prisma query.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Tenant hierarchy | `Organization` -> `Project (orgId)` cascade | `packages/shared/prisma/schema.prisma:95-181` |
| Auth session (NextAuth) | `Session.userId` / `sessionToken`, distinct from observability session | `packages/shared/prisma/schema.prisma:40-46` |
| Membership + RBAC | `OrganizationMembership`, `ProjectMembership`, `Role` enum (OWNER..NONE) | `packages/shared/prisma/schema.prisma:318-380` |
| API key scope | `ApiKeyScope { ORGANIZATION, PROJECT }`, key bound to `projectId`/`orgId` | `packages/shared/prisma/schema.prisma:183-211` |
| Observability session | `TraceSession` PK `@@id([id, projectId])`, sparse metadata table | `packages/shared/prisma/schema.prisma:382-395` |
| Agent thread model | `InAppAgentConversation` PK `[id, projectId]`, `createdByUserId`, `visibilityScope` | `packages/shared/prisma/schema.prisma:213-237` |
| Agent run/event keys | `InAppAgentRun`/`InAppAgentEvent` composite keys under `projectId` | `packages/shared/prisma/schema.prisma:239-277` |
| tRPC project guard | `enforceUserIsAuthedAndProjectMember` checks `projectId` ∈ session orgs/projects | `web/src/server/api/trpc.ts:284-373` |
| tRPC org guard | `enforceIsAuthedAndOrgMember` checks `orgId` membership | `web/src/server/api/trpc.ts:412-449` |
| Session-access guard | public-session fallback else project-membership check | `web/src/server/api/trpc.ts:588-657` |
| Session scoping source | session built from `organizationMemberships -> projects` + resolved role, filtered by `project:read` | `web/src/server/auth.ts:870-917` |
| API-key -> scope | `accessLevel` + `projectId` derived from `ApiKeyScope`; org keys have `projectId: null` | `web/src/features/public-api/server/apiAuth.ts:176-235`; `packages/shared/src/server/auth/types.ts:23-78` |
| REST project enforcement | rejects org key (`projectId` null) on project routes, checks access level | `web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:106-146` |
| ClickHouse default filter | every trace/score/observation query gets `project_id = {projectId}` | `packages/shared/src/server/queries/clickhouse-sql/factory.ts:217-252` |
| ClickHouse trace layout | `ORDER BY (project_id, toDate(timestamp), id)`, `user_id`/`session_id` nullable | `packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:1-32` |
| Sessions read query | ClickHouse session aggregation hardcodes `project_id = {projectId: String}` | `packages/shared/src/server/services/sessions-ui-table-service.ts:274-320` |
| Agent per-user read | `getOwnedConversationOrThrow` filters `projectId + createdByUserId` | `web/src/ee/features/in-app-agent/server/persistence.ts:56-76` |
| Agent ownership check | `ensureOwnedConversation` throws if `createdByUserId !== userId` | `web/src/ee/features/in-app-agent/server/persistence.ts:78-110` |
| Agent list scoping | `listConversations` filters `createdByUserId: ctx.session.user.id` | `web/src/ee/features/in-app-agent/server/router.ts:54-68` |
| Cross-user isolation test | "does not expose another user's conversation in the same project" | `web/src/__tests__/server/in-app-agent-persistence.servertest.ts:1248-1291` |
| Cross-project id reuse test | ids may repeat across projects safely | `web/src/__tests__/server/in-app-agent-persistence.servertest.ts:1293-1332` |

## Answers to Dimension Questions

**1. What scopes state?**
Two nested scoping systems. Platform state is scoped by `orgId` then `projectId`, resolved from `OrganizationMembership`/`ProjectMembership` (`packages/shared/prisma/schema.prisma:318-351`) and surfaced in the auth session (`web/src/server/auth.ts:870-917`). Every mutable/queryable observability entity carries `projectId`; trace-level `user_id`/`session_id` are *sub-scopes within a project* representing the customer's end users and their groupings, not platform identity (`packages/shared/clickhouse/migrations/clustered/0001_traces.up.sql:5-15`). Agent threads (`InAppAgentConversation`) add a third scope: `createdByUserId` within a project (`packages/shared/prisma/schema.prisma:218-236`).

**2. Can state leak across users or sessions?**
Not through the sanctioned paths. tRPC's `enforceUserIsAuthedAndProjectMember` rejects any `projectId` not in the caller's session projects (`web/src/server/api/trpc.ts:298-350`); REST routes reject org-scoped keys and enforce `projectId` from the key (`web/src/features/public-api/server/createAuthedProjectAPIRoute.ts:124-146`); ClickHouse reads are forced to `project_id = {projectId}` (`packages/shared/src/server/queries/clickhouse-sql/factory.ts:225-251`). For agent threads, cross-user reads are explicitly prevented and tested (`web/src/__tests__/server/in-app-agent-persistence.servertest.ts:1248-1291`). The residual risk is that this is *convention, not DB-enforced* — a new query that forgets the `projectId`/`createdByUserId` filter would leak, since there is no Postgres RLS (no `ENABLE ROW LEVEL SECURITY` in migrations).

**3. Is cross-thread memory explicit?**
Yes and it is deliberately narrow. Agent conversation history is reconstructed only from events of one `conversationId` within one `projectId` (`web/src/ee/features/in-app-agent/server/persistence.ts:268-294`); there is no implicit cross-conversation memory. `visibilityScope { PERSONAL, PROJECT }` (`packages/shared/prisma/schema.prisma:213-216`) is the explicit mechanism intended to widen a thread beyond its owner, though current read paths still gate on `createdByUserId` ownership (`persistence.ts:56-76`, `router.ts:54-68`).

**4. Are tenant boundaries enforced at the storage level?**
Partially. Composite primary keys embed the tenant: `TraceSession @@id([id, projectId])`, `InAppAgentConversation @@id([id, projectId])` (`packages/shared/prisma/schema.prisma:392`, `234`), and ClickHouse orders/partitions by `project_id` first (`0001_traces.up.sql:24-31`). But these are structural, not access-controlling — actual isolation is enforced in application middleware and repository query builders, not by database RLS/policies. Cascade deletes (`onDelete: Cascade` from Project/Org) do give storage-level lifecycle containment (`schema.prisma:130`, `321`).

**5. Can users fork sessions safely?**
For agent threads, yes: conversation/run ids are project-local, so the same id can coexist in different projects without collision, verified by test (`web/src/__tests__/server/in-app-agent-persistence.servertest.ts:1293-1332`). Concurrency within a thread is guarded — `createRun` locks the conversation and rejects a second active run with `LangfuseConflictError` (`web/src/ee/features/in-app-agent/server/persistence.ts:112-167`). There is no first-class "fork/branch a session" primitive; a fork would be a new conversation id.

## Architectural Decisions

- **Two-level tenancy (Org -> Project) as the universal scope.** Nearly every model carries `projectId` and cascades from `Project`/`Organization` (`packages/shared/prisma/schema.prisma:119-181`).
- **API keys are scoped, not global.** `ApiKeyScope` distinguishes `PROJECT` vs `ORGANIZATION`; project routes explicitly reject org keys (`packages/shared/src/server/auth/types.ts:23-32`; `createAuthedProjectAPIRoute.ts:135-141`).
- **Session as authorization context.** The NextAuth session precomputes the user's orgs/projects/roles once (`web/src/server/auth.ts:870-917`), so per-request checks are in-memory membership lookups (`web/src/server/api/trpc.ts:300-304`) rather than DB round-trips.
- **Composite `(id, projectId)` keys** so per-project id namespaces are independent (`schema.prisma:234`, `392`).
- **Centralized `project_id` filter factory** for analytical queries (`clickhouse-sql/factory.ts:217-252`) instead of ad-hoc per-query filters.
- **Agent threads owned per user** with an opt-in `visibilityScope` for future project-wide sharing (`schema.prisma:213-236`).

## Notable Patterns

- Middleware-based scope enforcement with a shared context shape (`{ orgId, orgRole, projectId, projectRole }`) injected into every protected procedure (`web/src/server/api/trpc.ts:360-373`, `719-729`).
- Public-session escape hatch: `TraceSession.public` allows anonymous read of a specific session, bypassing membership (`web/src/server/api/trpc.ts:605-637`; `schema.prisma:389`).
- Admin override path: platform `admin` users and the self-hosted `ADMIN_API_KEY` can access any project but emit a `sendAdminAccessWebhook` audit signal (`web/src/server/api/trpc.ts:307-357`; `createAuthedProjectAPIRoute.ts:164-251`).
- `environment` as an additional intra-project partition dimension for traces/sessions (`schema.prisma:390`; `traces.ts:1802-1808`).

## Tradeoffs

- **Application-enforced isolation vs. DB-enforced (RLS).** Keeps queries portable across Postgres + ClickHouse and simplifies the schema, but means correctness depends on every author remembering the `projectId`/`createdByUserId` filter. No storage-layer backstop (no RLS found in `packages/shared/prisma/migrations`).
- **Precomputed session scope vs. freshness.** Membership/role changes only take effect on session refresh; the tRPC guard reads `ctx.session.user.organizations` (`web/src/server/api/trpc.ts:300-304`) rather than re-querying, trading immediate revocation for performance.
- **Naming collision.** `Session` (auth) vs `TraceSession` (observability) vs in-app agent "conversation" — three "session-like" concepts increase the chance of a developer wiring state to the wrong scope.

## Failure Modes / Edge Cases

- **Unscoped query regression:** a new repository query omitting the `project_id` filter would silently cross tenants; there is no DB policy to catch it. Mitigation today is convention + `getProjectIdDefaultFilter` (`clickhouse-sql/factory.ts:217-252`) and targeted tests.
- **Public session exposure:** setting `TraceSession.public = true` makes that session readable without membership (`web/src/server/api/trpc.ts:618-637`); a mistaken publish leaks a whole session's traces.
- **Stale/orphaned agent runs:** a foreground stream that dies leaves an unfinished run; handled by lazily marking runs stale after `ACTIVE_RUN_STALE_AFTER_MS` before allowing a new run (`web/src/ee/features/in-app-agent/server/persistence.ts:112-141`).
- **Org key on project route:** returns 403 with an explicit "Are you using an organization key?" message rather than silently succeeding (`createAuthedProjectAPIRoute.ts:135-141`).
- **Admin blast radius:** admin/`ADMIN_API_KEY` can reach any project; only audited via webhook, not blocked (`web/src/server/api/trpc.ts:325-357`).

## Future Considerations

- Consider Postgres RLS or a lint/test guard that fails any Prisma/ClickHouse query lacking a `project_id` predicate, to convert the isolation invariant from convention to enforcement.
- Finish wiring `InAppAgentConversationVisibilityScope.PROJECT` into read paths (currently reads still gate on `createdByUserId`) so project-shared agent threads are a supported, tested flow (`schema.prisma:213-216`; `persistence.ts:56-76`).
- Disambiguate the three "session" concepts in naming/docs to reduce mis-scoping risk.
- Add a first-class conversation fork/branch primitive if multi-branch agent exploration becomes a use case (today only id-per-project reuse exists).

## Questions / Gaps

- No evidence of database-level (RLS) tenant isolation was found in `packages/shared/prisma/migrations`; confirmation that isolation is exclusively application-layer is based on absence of `ENABLE ROW LEVEL SECURITY` and the presence of application filters.
- The exact runtime read behavior of `visibilityScope = PROJECT` (whether any path surfaces another user's PROJECT-visible conversation) is not exercised by the isolation tests reviewed (`in-app-agent-persistence.servertest.ts:1248-1291` only asserts PERSONAL isolation); treated as inferred-intent, not confirmed behavior.
- Redis-cached API keys (`packages/shared/src/server/auth/apiKeyCache.ts`) were not deeply inspected for scope-cache invalidation timing; potential staleness window on key scope changes is a gap.

---

Generated by `02.07-session-thread-and-user-boundaries` against `langfuse`.
