# Source Analysis: langfuse

## Dimension 09.01: Policy Injection Points

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Next.js (web), Express+BullMQ (worker), shared package, Prisma/Postgres, ClickHouse, Redis |
| Analyzed | 2026-08-26 |

## Summary

Langfuse injects governance policy through five distinct layers, each with a different update mechanism:

1. **Code-defined policy catalogs** — the RBAC scope vocabulary (`projectScopes`) and role→scope mappings are compile-time constants in the shared package (`packages/shared/src/features/rbac/projectAccessRights.ts:5-86`, `packages/shared/src/features/rbac/projectAccessRights.ts:91-272`); the in-app agent's per-tool approval classification (`auto` vs `approval`) is an exhaustive constant map (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`); the plan→entitlement matrix is hardcoded (`web/src/features/entitlements/constants/entitlements.ts:55-183`). Changing these requires a code change and deploy.
2. **Database-stored policy instances** — role *assignments* (org/project memberships), prompt protected labels (`web/src/features/prompts/server/utils/checkHasProtectedLabels.ts:14-18`), org rate-limit overrides inside the `cloudConfig` JSON blob (`packages/shared/src/interfaces/cloudConfigSchema.ts:70`), and per-conversation "always allowed" tool grants (`packages/shared/src/in-app-agent/server/runLifecycle.ts:333-343`) are all runtime-editable without deploys.
3. **Environment-variable policies** — the external ingestion-masking callback (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:19-35`), the admin-access webhook (`web/src/server/adminAccessWebhook.ts:37`), and instance/feature gates (`web/src/features/in-app-agent/server/availability.ts:21`).
4. **Tool metadata annotations** — MCP tools declare `readOnlyHint`/`destructiveHint`/`expensiveHint` at definition time (`web/src/features/mcp/core/define-tool.ts:39-47`); the MCP registry enforces in-app-agent key restrictions against `readOnlyHint` (`web/src/features/mcp/server/registry.ts:170-189`).
5. **Prompt-based rules** — the in-app agent's behavioral rules include a `<permissions>` section telling the model that mutating tools require human confirmation (`packages/shared/src/in-app-agent/server/systemPrompt.ts:52-58`). Production loads this prompt from Langfuse prompt management, making the governed prompt itself versioned and label-addressable (`worker/src/features/in-app-agent/runtime/agent.ts:1488-1499`).

Enforcement is centralized in tRPC middlewares (`protectedProjectProcedure`, `web/src/server/api/trpc.ts:297-401`) and route-level auth services, with a deliberate defense-in-depth chain for the agent harness: RBAC → human approval → API-key capability restriction on every MCP call. Auditability is a first-class product surface: an `auditLog()` helper records actor, action, and before/after state (`web/src/features/audit-logs/auditLog.ts:86-155`), and approval decisions are persisted as durable events tied to user IDs (`packages/shared/src/in-app-agent/approvalEvents.ts:10-18`).

**Answer to the dimension question ("Can a governance rule be added without modifying agent code?"):** only partially. Rule *instances* (who has which role, which labels are protected, what rate limits apply, which tools a conversation pre-approved) can be changed at runtime. Rule *logic* (the scope catalog, the role→scope map, the tool approval classification, the entitlement matrix) is compiled into the deployment and requires a PR plus rollout — deliberately, backed by an exhaustiveness test that refuses to let a new tool ship unclassified (`web/src/__tests__/server/unit/in-app-agent-mcp-policy.servertest.ts:14-27`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Layered injection points with clear ownership boundaries documented in-feature (`web/src/features/in-app-agent/README.md:216-241` documents the full MCP authorization chain).
- Server-side enforcement is centralized and consistently applied; the client-side UI check mirrors the same shared constants (`web/src/features/rbac/utils/checkProjectAccess.ts:53-65` imports `projectRoleAccessRights` from `@langfuse/shared`).
- Failure behavior is explicit where it matters most: approval decisions use an exactly-once CAS transition (`packages/shared/src/in-app-agent/server/runLifecycle.ts:283-297`), and grants are re-evaluated on every run so revoked roles drop stale approvals (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-339`).
- Gaps keeping it below 8–9: there is no unified policy engine or precedence document — each subsystem resolves conflicts differently (custom-over-plan for rate limits, admin-over-all for RBAC, OR-composition for feature flags); audit logging is opt-in per handler (72 files call `auditLog(...)`) rather than intercepted automatically, so coverage depends on developer discipline; the RBAC catalog itself has no version history beyond git; and rate limiting fails open when Redis is unavailable (`web/src/features/public-api/server/RateLimitService.ts:117-123`, `160-164`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| RBAC scope catalog (code-defined) | `projectScopes` const array defines ~45 `Resource:Action` scopes incl. `prompts:CUD`, `promptProtectedLabels:CUD`, `llmTools:CUD` | `packages/shared/src/features/rbac/projectAccessRights.ts:5-86` |
| Role→scope policy map | `projectRoleAccessRights` maps OWNER/ADMIN/MEMBER/VIEWER/NONE to scope lists | `packages/shared/src/features/rbac/projectAccessRights.ts:91-272` |
| Org-level role rights | `organizationRoleAccessRights` with OWNER/ADMIN/MEMBER/VIEWER/NONE | `web/src/features/rbac/constants/organizationAccessRights.ts:20-43` |
| Role hierarchy ordering | `orderedRoles` numeric ranking used for privilege comparisons | `web/src/features/rbac/constants/orderedRoles.ts:3-9` |
| Server enforcement middleware | `enforceUserIsAuthedAndProjectMember` validates project membership from session and injects `projectRole` into context; `protectedProjectProcedure` composition | `web/src/server/api/trpc.ts:297-401` |
| Per-scope check helper | `throwIfNoProjectAccess` throws TRPC FORBIDDEN unless role includes scope | `web/src/features/rbac/utils/checkProjectAccess.ts:25-65` |
| Pure role check reused outside web | `hasProjectAccessByRole` — "Mirrors the role branch of web's hasProjectAccess" for the in-app agent runtime | `packages/shared/src/features/rbac/projectAccessRights.ts:282-289` |
| Global admin bypass | `session.user.admin === true` short-circuits project membership and gets synthetic OWNER role; fires audit webhook | `web/src/server/api/trpc.ts:320-361` |
| Admin-bypass audit webhook | `sendAdminAccessWebhook` posts email/project/org to `LANGFUSE_ADMIN_ACCESS_WEBHOOK` with 24h dedupe | `web/src/server/adminAccessWebhook.ts:12-57` |
| Plan→entitlement matrix (code-defined) | `entitlementAccess: Record<Plan, {entitlements, entitlementLimits}>`; `rbac-project-roles`, `audit-logs`, `prompt-protected-labels` gated to `cloud:team`+ | `web/src/features/entitlements/constants/entitlements.ts:55-183` |
| Entitlement check | `hasEntitlement` reads org plan off session JWT; `throwIfNoEntitlement` throws FORBIDDEN | `web/src/features/entitlements/server/hasEntitlement.ts:17-52` |
| Feature-flag gate middleware | `requireFeatureFlag`: enabled = user flag OR global admin OR experimental-features env | `web/src/server/api/trpc.ts:403-418` |
| Flag resolution sources | `parseFlags(dbFlags, context)` — per-user DB flags plus email-domain-based preview defaults (`@langfuse.com`, `@clickhouse.com`) | `web/src/features/feature-flags/utils.ts:28-53` |
| Agent tool approval policy (code-defined) | `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` classifies ~90 MCP tools as `auto`/`approval` and binds each to a required `ProjectScope` | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:22-360` |
| Availability = RBAC intersection | `isInAppAgentLangfuseMcpToolAvailable` calls `hasProjectAccessByRole` per tool scope; unavailable tools are hidden from the model | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:451-470` |
| Policy assembly + one-off approvals | `createInAppAgentToolPolicy(userAccess, alwaysAllowedTools)`; `alwaysAllowedTools` can only promote tools already RBAC-available (`available.has(toolName)` guard) | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:477-510` |
| Grants rebuilt per run | Worker comments: "Rebuild each run so grants invalidated by role changes drop out" | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:330-344` |
| Durable grant persistence + audit event | Always-allow grant pushed to conversation row in same transaction as exactly-once approval CAS; `langfuse_approval_decision` event records `decidedByUserId` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:283-355` |
| Approval TTL expiry policy | Approvals older than fixed `IN_APP_AGENT_APPROVAL_TTL_MS` fail the parent run with `APPROVAL_EXPIRED` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:263-281` |
| Approval decision schema | `InAppAgentApprovalDecisionSchema` requires `toolCallId`, `approved`, `decidedByUserId` | `packages/shared/src/in-app-agent/approvalEvents.ts:10-18` |
| MCP tool metadata annotations | `readOnlyHint`, `destructiveHint`, `expensiveHint` options on every tool definition | `web/src/features/mcp/core/define-tool.ts:39-47,161-167` |
| MCP server-side key enforcement | `canCallTool`: in-app-agent keys need `readOnlyHint === true`, or explicit allowlist match under `tool-allowlist` permissions | `web/src/features/mcp/server/registry.ts:170-189` |
| Override header parsing (defense against tampering) | `getInAppAgentContext` parses `x-langfuse-in-app-agent-tool-override` via zod; invalid/absent ⇒ read-only | `web/src/pages/api/public/mcp/index.ts:193-217` |
| In-app-agent key capability scoping | API keys carry `isInAppAgentKey` flag; rejected everywhere except explicitly opted-in endpoints (`allowInAppAgentKey`) | `web/src/features/public-api/server/apiAuth.ts:190-204,278-288`; `packages/shared/prisma/schema.prisma:212` |
| Rate limit precedence | `getRateLimitConfig`: custom org override (`cloudConfig.rateLimitOverrides`) wins over hardcoded plan defaults | `web/src/features/public-api/server/RateLimitService.ts:235-245` |
| Rate limit applicability + fail-open | Limits only in cloud deployments; Redis errors return undefined → request proceeds | `web/src/features/public-api/server/RateLimitService.ts:84-96,116-123,160-164` |
| Runtime-updatable rate limits | `rateLimitOverrides` stored in org `cloudConfig` JSON, written by billing webhooks | `packages/shared/src/interfaces/cloudConfigSchema.ts:69-70` |
| Entitlement-limited resource ceilings | e.g. `"monitor-count": 2` (hobby) vs `50` (pro); enforced via `hasEntitlementLimit` | `web/src/features/entitlements/constants/entitlements.ts:62-97`; `web/src/features/prompts/server/routers/promptRouter.ts:50` |
| DB-driven protected labels policy | Protected labels fetched from `promptProtectedLabels` table per project; creating/updating a prompt with such labels requires `promptProtectedLabels:CUD` scope | `web/src/features/prompts/server/utils/checkHasProtectedLabels.ts:14-24`; `web/src/features/prompts/server/routers/promptRouter.ts:315-329` |
| Prompt immutability + change events | New prompt versions are creates; every change queues an `EntityChangeJob` with full prompt payload and acting user | `web/src/features/prompts/server/promptChangeEventSourcing.ts:14-55` |
| Audit log writer | `auditLog()` persists resourceType/action/before/after with typed actor (user session vs API key, incl. attribution of in-app-agent keys to their creator) | `web/src/features/audit-logs/auditLog.ts:7-155` |
| Audit log read gating | Reads require BOTH `audit-logs` entitlement AND `auditLogs:read` scope | `web/src/server/api/routers/auditLogs.ts:88-99` |
| External policy-engine hook | Ingestion masking POSTs OTEL events to `LANGFUSE_INGESTION_MASKING_CALLBACK_URL` before ClickHouse storage; EE-license-gated; `failClosed` env switch decides drop-vs-passthrough after retries | `packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:19-57,190-216`; applied in `worker/src/queues/otelIngestionQueue.ts:519-520` |
| Prompt-based governance rules | Seeded agent system prompt contains `<behavioral_rules>`, `<data_scope>`, and `<permissions>` sections; approval UX promised to the model in prose | `packages/shared/src/in-app-agent/server/systemPrompt.ts:10-58` |
| Managed prompt as policy artifact | Production loads the system prompt via `langfuseClient.getPrompt(...)` and records resolved `name`/`version` metadata with the run | `worker/src/features/in-app-agent/runtime/agent.ts:1459-1499` |
| Policy-classification test (change control) | Test asserts type equality and runtime equality between MCP registry tool names and the approval-policy map keys | `web/src/__tests__/server/unit/in-app-agent-mcp-policy.servertest.ts:13-27` |
| Authz tests for agent routes | Watch-route authorization cases incl. entitlement rejection and cross-member isolation | `web/src/__tests__/server/in-app-agent-api-route-auth.servertest.ts:175-283` |
| Instance/org-level availability policy | `assertInAppAgentAvailable`: instance env flag + `in-app-agent` entitlement + org `aiFeaturesEnabled` column | `web/src/features/in-app-agent/server/availability.ts:12-63` |

## Answers to Dimension Questions

### 1. Where do governance rules live?

Four locations, by kind:

- **Runtime code (compile-time constants):** the RBAC scope catalog and role→scope map (`packages/shared/src/features/rbac/projectAccessRights.ts:5-86,91-272`), org-role rights (`web/src/features/rbac/constants/organizationAccessRights.ts:20-43`), the agent's per-tool approval classification bound to required scopes (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`), the plan→entitlement matrix (`web/src/features/entitlements/constants/entitlements.ts:55-183`), and hardcoded plan-based rate limits (`web/src/features/public-api/server/RateLimitService.ts:247-344`).
- **Tool metadata:** MCP annotations (`readOnlyHint` etc.) declared per tool (`web/src/features/mcp/core/define-tool.ts:39-47`) and consumed by both the registry gate (`web/src/features/mcp/server/registry.ts:170-189`) and the agent auto-approval derivation (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:406-421`).
- **Deployment/env config:** masking callback URL + timeout + fail-closed switch + propagated headers (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:19-35`), admin-access webhook URL (`web/src/server/adminAccessWebhook.ts:37`), Bedrock model ID (`web/src/features/in-app-agent/server/availability.ts:66-75`), experimental-features toggle (`web/src/server/api/trpc.ts:408-410`).
- **External policy engine:** the enterprise ingestion-masking callback is a genuine external decision point — an operator-supplied HTTP service rewrites or rejects events mid-ingestion (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:145-216`, wired at `worker/src/queues/otelIngestionQueue.ts:519-520`).
- **Prompt-based rules:** the agent's `<behavioral_rules>`/`<permissions>` sections (`packages/shared/src/in-app-agent/server/systemPrompt.ts:10-58`); production serves this from Langfuse prompt management so rule text is itself version-controlled and rollable (`worker/src/features/in-app-agent/runtime/agent.ts:1488-1499`).

### 2. Can policies be updated at runtime?

Split answer:

- **Yes (data/config layer):** role assignments via membership APIs (`web/src/features/rbac/server/membersRouter.ts` calls `auditLog` at lines 267, 299, 322, 372, 468 while mutating roles); protected labels CRUD'd by project admins (`web/src/features/prompts/server/routers/promptRouter.ts:603,694,875`); per-org rate-limit overrides in `cloudConfig` (`packages/shared/src/interfaces/cloudConfigSchema.ts:70`); per-conversation always-allowed tools (`packages/shared/src/in-app-agent/server/runLifecycle.ts:333-343`); the agent's governing prompt (new version in prompt management takes effect on next run). Critically, role revocations take effect immediately for agent grants because the worker recomputes the tool policy on every run (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-339`).
- **No (logic layer):** adding a scope, changing what a role may do, reclassifying a tool's approval mode, or changing an entitlement tier all require editing shipped constants. The repo treats this as a feature: a test enforces that no MCP tool ships without an explicit approval classification (`web/src/__tests__/server/unit/in-app-agent-mcp-policy.servertest.ts:13-27`), i.e., review-gated policy changes instead of live edits.

### 3. What happens when policies conflict?

There is no single precedence resolver; each subsystem encodes its own:

- **Global admin > everything:** `session.user.admin === true` bypasses project membership (`web/src/server/api/trpc.ts:320-361`), all RBAC scope checks (`web/src/features/rbac/utils/checkProjectAccess.ts:54-55`), role-based tool availability (`packages/shared/src/features/rbac/projectAccessRights.ts:287`), and entitlement checks (`web/src/features/entitlements/server/hasEntitlement.ts:18`). The mitigation is observability: every admin cross-project entry fires a deduped webhook (`web/src/server/adminAccessWebhook.ts:32-54`).
- **Custom override > plan default** for rate limits (`web/src/features/public-api/server/RateLimitService.ts:239-244`).
- **OR-composition** for feature flags: user flag OR admin OR instance-wide experimental flag (`web/src/server/api/trpc.ts:406-410`).
- **Intersection, never union, for agent capabilities:** approval can only promote a tool the user's role already makes *available*; `createInAppAgentToolPolicy` guards `alwaysAllowedTools` promotion behind `available.has(toolName)` (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:501-507`), and the resulting override header is re-validated server-side against the same allowlist semantics (`web/src/features/mcp/server/registry.ts:179-186`). Human approval therefore relaxes workflow friction, not permissions — stated design intent at `web/src/features/in-app-agent/README.md:231`.
- **Fail-open/fail-closed is an explicit knob** for the external masking policy: default fail-open, `LANGFUSE_INGESTION_MASKING_CALLBACK_FAIL_CLOSED=true` drops events instead (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:200-215`).

### 4. Are policy changes audited?

Substantially, though opt-in per write path:

- A typed audit-log facility records resourceType, action, and serialized before/after state, distinguishing USER actors (with org/project roles at decision time) from API_KEY actors, including attributing in-app-agent-key actions to the key's creator (`web/src/features/audit-logs/auditLog.ts:7-155`). It is invoked across 72 files covering prompts, memberships, API keys, SCIM user changes, integrations, traces, and admin-API mutations.
- Reading the audit trail is itself a governed operation: `audit-logs` entitlement plus `auditLogs:read` scope (`web/src/server/api/routers/auditLogs.ts:88-99`).
- Agent-specific governance actions get dedicated durable records: approval decisions store `decidedByUserId` in an append-only event stream (`packages/shared/src/in-app-agent/approvalEvents.ts:7-18`), committed atomically with the exactly-once approval CAS (`packages/shared/src/in-app-agent/server/runLifecycle.ts:283-355`); admin bypasses emit webhooks (`web/src/server/adminAccessWebhook.ts:32-82`); prompt changes queue entity-change events carrying the acting user (`web/src/features/prompts/server/promptChangeEventSourcing.ts:23-39`).
- Limitation: nothing mechanically guarantees a mutation calls `auditLog` — it is convention enforced by review, not by middleware interception.

## Architectural Decisions

1. **Shared-package policy catalogs with dual-surface consumers.** The scope catalog lives in `@langfuse/shared` so web (tRPC resolvers), worker (agent runtime), and even browser bundles import identical definitions; a pure function `hasProjectAccessByRole` was extracted specifically so the worker's agent runtime mirrors web's check without importing server-only code (`packages/shared/src/features/rbac/projectAccessRights.ts:277-289`).
2. **Middleware-layered tRPC procedures as the enforcement chokepoint.** `publicProcedure → authenticatedProcedure → protectedProjectProcedure/protectedOrganizationProcedure` compose auth, membership, and role-context injection (`web/src/server/api/trpc.ts:250-499`); individual handlers then add fine-grained scope checks (`throwIfNoProjectAccess`) and entitlement checks (`throwIfNoEntitlement`) — e.g., the prompt router stacks procedure + two scope checks + protected-label escalation + audit log for one mutation (`web/src/features/prompts/server/routers/promptRouter.ts:306-357`).
3. **Three independent gates between the agent and side effects.** (a) RBAC filters the tool list before the model ever sees it (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:484-499`); (b) `approval`-class tools raise a Mastra interrupt surfaced to a human (`web/src/features/in-app-agent/README.md:235`); (c) the transport layer independently restricts the temporary API key by annotation + signed override header (`web/src/features/mcp/server/registry.ts:170-189`, `web/src/pages/api/public/mcp/index.ts:134-148`). Compromise of any one layer leaves the others intact.
4. **Client sends only IDs for approvals.** `decideToolApproval` accepts IDs + boolean; the tool name/args are read server-side from the persisted interrupt event, eliminating client-side tampering ("nothing to tamper with on the way back", `web/src/features/in-app-agent/server/router.ts:260-305`).
5. **Exhaustive-map policy style with test-enforced completeness.** The approval policy uses `satisfies Record<string, InAppAgentMcpToolPolicy>` and derives the tool-name type from its own keys (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:360-363`), so adding an MCP tool breaks compilation until classified — policy drift becomes a type error plus a failing test.
6. **Fixed lifecycle policy values over configurables.** Approval TTL, max run duration, and heartbeat timings are shared constants precisely "so web and worker cannot diverge" (`web/src/features/in-app-agent/README.md:170-176`) — an intentional choice to keep some policy out of operators' hands.

## Notable Patterns

- **Scope-string grammar** (`Resource:Action`, e.g. `prompts:CUD`) as a flat, greppable policy vocabulary (`packages/shared/src/features/rbac/projectAccessRights.ts:5-86`); UI hides controls via the same constants (`useHasProjectAccess`, `web/src/features/rbac/utils/checkProjectAccess.ts:39-50`), giving consistent client/server behavior from one source.
- **Escalating scope checks inside one handler:** ordinary prompt creation needs `prompts:CUD`; touching a protected label silently escalates to `promptProtectedLabels:CUD` with a user-facing message listing offending labels (`web/src/features/prompts/server/routers/promptRouter.ts:315-329`) — a sub-resource permission overlay implemented as data (`web/src/features/prompts/server/utils/checkHasProtectedLabels.ts:14-24`).
- **Capability-typed API keys.** Keys carry semantic flags (`isInAppAgentKey`) and per-endpoint opt-ins (`allowInAppAgentKey`) rather than raw scopes, with tests pinning each combination (`web/src/__tests__/server/in-app-agent-api-route-auth.servertest.ts:77-102`).
- **Policy-as-data for rate limiting:** plan defaults in code, per-org exceptions in DB JSON validated by zod (`packages/shared/src/interfaces/cloudConfigSchema.ts:70`), resolved at request time (`web/src/features/public-api/server/RateLimitService.ts:239-244`).
- **Governance provenance on AI outputs:** each agent run stores which prompt name+version produced its instructions (`worker/src/features/in-app-agent/runtime/agent.ts:1477-1481,1494-1499`), linking behavior back to the exact policy artifact version.
- **Metrics on policy enforcement:** rate-limit exceedances emit `langfuse.rate_limit.exceeded` tagged with org/plan/resource (`web/src/features/public-api/server/RateLimitService.ts:167-173`); masking callbacks emit duration/success/failure histograms including the fail-closed bit (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:91-99,170-193`).

## Tradeoffs

- **Deploy-gated policy logic vs. operational safety.** Compile-time catalogs make unauthorized policy change impossible without code review, but mean a misclassification fix (e.g., demoting a mutating tool from `auto` to `approval`) waits on a release cycle.
- **Admin superuser breadth vs. break-glass simplicity.** One flag overrides RBAC, entitlements, and flags across all surfaces; simple to operate, but the safety net is reactive (webhooks/dedupe, `web/src/server/adminAccessWebhook.ts:19-30`) rather than preventive (no per-project admin allowlist found).
- **Opt-in audit calls vs. uniform capture.** Flexible and cheap, but a new public-API route without an `auditLog` call is silent until review catches it; there is no lint rule or base-class guarantee tying writes to audit emission.
- **Duplicated RBAC surfaces (UI + resolver + pure fn).** Three call sites share constants, minimizing drift, yet the org-level rights table exists separately in web (`web/src/features/rbac/constants/organizationAccessRights.ts:20-43`) while project rights live in shared — two homes for conceptually identical structures.
- **Cloud-first rate limiting.** Self-hosted deployments skip rate limiting entirely (`web/src/features/public-api/server/RateLimitService.ts:85-87`), trading abuse protection for zero-config OSS experience.
- **Fail-open defaults.** Both rate limiting (Redis down) and ingestion masking (default) prefer availability over strictness; only masking offers the stricter mode, and only via env var.

## Failure Modes / Edge Cases

- **Stale-grant window closed by design:** a user who approved a tool then lost the role cannot have that grant replayed — the worker rebuilds policy per run (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-344`) and `getInAppAgentMcpAllowedToolNames` only emits names still in `policy.available` transitively (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:512-532`).
- **Approval races handled by CAS:** double-clicking approve yields `LangfuseConflictError("already decided")` because the status-transition `updateMany` matches 0 rows the second time (`packages/shared/src/in-app-agent/server/runLifecycle.ts:283-297`); expired approvals fail the parked run deterministically with `APPROVAL_EXPIRED` (`runLifecycle.ts:263-281`).
- **Override-header spoofing contained twice:** a non-agent key sending the override header is ignored (context undefined ⇒ unrestricted-but-normal key, but such keys are blocked from agent-only endpoints elsewhere); an agent key sending a malformed or unclassified tool list falls back to read-only (`web/src/pages/api/public/mcp/index.ts:201-216`), and any listed tool must also carry `readOnlyHint` or appear in the parsed allowlist at call time (`web/src/features/mcp/server/registry.ts:179-186`).
- **Rolling-deploy compatibility of policy wire formats:** the override payload writes both singular and plural fields so older web pods survive (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:396-404`).
- **Unknown billing plan codes degrade loudly-by-design:** an unrecognized CHB plan code nulls `cloudConfig.planCode` (documented blast radius includes losing rate-limit overrides), with unknown codes rejected at webhook ingress (`packages/shared/src/interfaces/cloudConfigSchema.ts:38-48`).
- **Masking outage behavior differs by mode:** fail-open logs and continues with raw data; fail-closed returns failure so the ingestion event is dropped — both instrumented for alerting (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:190-216`).
- **Session-carried policy data staleness:** roles ride inside the NextAuth JWT/session object (`ctx.session.user.organizations...role`, `web/src/server/api/trpc.ts:313-317`), so membership changes propagate at session refresh cadence rather than instantly for UI-session paths (API-key paths resolve from DB per request, `web/src/features/public-api/server/apiAuth.ts:190-204`). No TTL documentation was found in-code for this staleness window.

## Future Considerations

- Centralize conflict resolution: a single precedence evaluator (env < DB override < plan default < admin) would remove the need to re-derive ordering rules per subsystem.
- Emit audit records via tRPC/REST middleware interception keyed on mutation verbs, making audit coverage structural instead of conventional.
- Consider promoting the approval-policy map to operator-configurable data (with the existing exhaustiveness test as migration validation), enabling hot reclassification of tools without deploys while retaining review gates.
- Add a documented staleness bound (or DB re-validation) for session-embedded roles on sensitive mutations.
- Version the RBAC scope catalog itself (e.g., append-only scope additions with deprecation notes) so downstream SDK/UI consumers can detect capability drift.

## Questions / Gaps

- No evidence found of a declarative policy engine (OPA/Cedar-style) integration; searched for `policy`, `guardrail`, `rego`, `cedar` across `web/src`, `worker/src`, `packages/shared/src`, and `ee/` — the only external decision hooks are the ingestion-masking callback and the admin-access webhook cited above.
- No evidence found of automated verification that every mutating tRPC/public-API handler emits an audit record; coverage could not be quantified beyond counting call sites (72 files).
- The mechanism refreshing NextAuth sessions after role changes (staleness window size) was not located in the reviewed auth files; the session shape is consumed at `web/src/server/api/trpc.ts:313-317` but the refresh trigger lives in NextAuth configuration not examined here.
- Whether `ee/` cloud SSO/SCIM configurations impose additional per-request policy (beyond the SCIM audit calls seen at `web/src/pages/api/public/scim/Users/index.ts:9`) was not fully traced; the license-gate primitive exists at `packages/shared/src/server/ee/licenseCheck`.

---

Generated by `Dimension 09.01: Policy Injection Points` against `langfuse`.
