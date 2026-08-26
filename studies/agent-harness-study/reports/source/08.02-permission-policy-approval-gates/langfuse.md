# Source Analysis: langfuse

## Dimension 08.02 — Permission Policy and Approval Gates

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo — Next.js app + tRPC (`web/`), BullMQ worker (`worker/`), shared contracts package (`packages/shared/`), Prisma/Postgres + ClickHouse storage |
| Analyzed | 2026-08-26 |

## Summary

Langfuse gates sensitive operations with two distinct layers that this dimension's questions map onto cleanly.

**Layer 1 — product RBAC (no human approval).** All sensitive product operations (delete traces, CRUD API keys, manage members, etc.) are gated by a static, code-defined role→scope matrix. Roles (`OWNER/ADMIN/MEMBER/VIEWER/NONE`) come from the database `Role` enum and are checked against ~50 project scopes (`packages/shared/src/features/rbac/projectAccessRights.ts:5-91`) and 9 organization scopes (`web/src/features/rbac/constants/organizationAccessRights.ts:5-43`). Enforcement is centralized in tRPC procedure middlewares (`web/src/server/api/trpc.ts:297-401` for projects, `:444-499` for orgs) plus per-scope checks in resolvers via `throwIfNoProjectAccess` (`web/src/features/rbac/utils/checkProjectAccess.ts:25-33`). There is no human-approval workflow for these operations; the role *is* the policy.

**Layer 2 — human approval gates for the in-app agent (the harness-relevant part).** The in-app agent executes tools on behalf of a signed-in user through an MCP surface. Every Langfuse MCP tool carries an explicit approval classification (`"auto"` or `"approval"`) plus a required RBAC scope in an exhaustive registry (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`). Approval-classified tools suspend the run (Mastra `requireApproval`), persist a durable interrupt, and wait for a human decision recorded transactionally with an exactly-once CAS (`packages/shared/src/in-app-agent/server/runLifecycle.ts:222-397`). Approvals can be one-shot or conversation-scoped grants; grants are stored on the conversation row (`packages/shared/prisma/schema.prisma:248-249`) but are revalidated against the current user role on every run (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-344`), so a demoted user loses auto-approval even for previously granted tools. The MCP endpoint additionally enforces the allowlist server-side from a run-scoped header (`web/src/pages/api/public/mcp/index.ts:193-217`), backed by an ephemeral per-run API key that is deleted at terminal state (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:382-399`, `packages/shared/src/server/auth/apiKeys.ts:148-162`).

The design principle is stated in the feature README and holds up in code: RBAC is the first gate; approval only permits execution of a tool the user could already use manually and never widens permissions (`web/src/features/in-app-agent/README.md:231`).

## Rating

**8 / 10** — Clear permission model with explicit interfaces, exhaustive tool-policy classification enforced by types and tests, exactly-once decision semantics under concurrency, TTL expiry, supersede/cancel paths, and a handled durability window (`OUTCOME_UNKNOWN`). It misses 9–10 because grant revocation is implicit-only (no per-grant revoke API), sandbox shell tools are auto-approved without per-call confirmation, audit coverage of agent approvals relies on the append-only event stream rather than the audited `auditLog` path, and there is no externalized policy engine — all policy is compile-time constants.

## Evidence Collected

Every entry cites `path:line` relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission schema — project scopes (~50 `Resource:Action` scopes as const tuple) | `projectScopes` incl. sensitive ones like `traces:delete`, `apiKeys:CUD`, `llmApiKeys:CUD` | `packages/shared/src/features/rbac/projectAccessRights.ts:5-86` |
| Role→scope matrix (OWNER/ADMIN/MEMBER/VIEWER/NONE) | `projectRoleAccessRights` — VIEWER read-only, NONE empty | `packages/shared/src/features/rbac/projectAccessRights.ts:91-272` |
| Pure role check used by worker runtime | `hasProjectAccessByRole` mirrors web's role branch | `packages/shared/src/features/rbac/projectAccessRights.ts:282-289` |
| Organization scopes and rights | `organizationScopes`, `organizationRoleAccessRights`; billing/apiKeys/member CUD restricted to OWNER/ADMIN | `web/src/features/rbac/constants/organizationAccessRights.ts:5-43` |
| Session-based access checks throwing FORBIDDEN | `throwIfNoProjectAccess` / `useHasProjectAccess` / `hasProjectAccess` | `web/src/features/rbac/utils/checkProjectAccess.ts:25-65` |
| Org access check | `throwIfNoOrganizationAccess` / `hasOrganizationAccess` | `web/src/features/rbac/utils/checkOrganizationAccess.ts:27-69` |
| tRPC auth middleware chain | `authenticatedProcedure`, `protectedProjectProcedure`, `protectedOrganizationProcedure`, `adminProcedure` | `web/src/server/api/trpc.ts:273-275, 399-401, 497-499, 793-795` |
| Project membership enforcement + admin bypass w/ oversight webhook | `enforceUserIsAuthedAndProjectMember` — non-members UNAUTHORIZED; global admin gets OWNER role and fires webhook | `web/src/server/api/trpc.ts:297-397` |
| Feature-flag gate (policy beyond roles) | `requireFeatureFlag` middleware throws FORBIDDEN when flag unset | `web/src/server/api/trpc.ts:404-418` |
| Approval states (run lifecycle) | `AWAITING_APPROVAL` status; error codes `APPROVAL_EXPIRED`, `APPROVAL_SUPERSEDED`, `APPROVAL_CANCELLED` | `packages/shared/src/features/inAppAgent/types.ts:15, 59-63` |
| Exhaustive per-tool approval policy | `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES`: every tool mapped to `"auto"`/`"approval"` + availability scope; exhaustiveness enforced by type + runtime assertions vs MCP registry | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:22-360` |
| Policy builder (RBAC first, grants second) | `createInAppAgentToolPolicy` filters availability via `hasProjectAccessByRole`, then adds auto-approvals and stored `alwaysAllowedTools` only if still available | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:451-510` |
| Attaching requireApproval to tools | `withInAppAgentToolApproval` marks non-auto-approved tools; docs tools approved by `langfuseDocs_` prefix; sandbox `read/write/edit/bash` local-auto-approved | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:406-421, 551-581` |
| Durable interrupt event contract | Mastra `on_interrupt` custom event parsed into `tool_approval_request` by shared parser (browser/web/worker) | `packages/shared/src/in-app-agent/interrupts.ts:7-37` |
| Approval decision event schema (who decided) | `InAppAgentApprovalDecisionSchema` = `{toolCallId, approved, decidedByUserId}` appended as render-history event | `packages/shared/src/in-app-agent/approvalEvents.ts:6-28` |
| Decision persistence: exactly-once CAS + TTL + grant push in one tx | `decideToolApproval` — parent must be AWAITING_APPROVAL; expires past TTL; `updateMany` CAS marks decided; `alwaysAllowedTools` pushed in same transaction; decision event appended under lock | `packages/shared/src/in-app-agent/server/runLifecycle.ts:235-355` |
| Approval TTL constant (24h) | `IN_APP_AGENT_APPROVAL_TTL_MS = 24 * 60 * 60_000` | `packages/shared/src/in-app-agent/server/tunables.ts:16-17` |
| Read-time reconciliation of expired/superseded approvals | `classifyStaleRun` fails AWAITING_APPROVAL past TTL (`approval_expired`); new message supersedes parked approval (`createQueuedRun` cancels with APPROVAL_SUPERSEDED) | `packages/shared/src/in-app-agent/server/runLifecycle.ts:736-744, 162-174` |
| Grant persistence column | `alwaysAllowedTools String[] @default([])` with comment "revalidated against the registry and owner's role each run" | `packages/shared/prisma/schema.prisma:248-249` |
| Revocation of grants via revalidation | Worker rebuilds policy each run: "Rebuild each run so grants invalidated by role changes drop out." | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-344` |
| Zero-trust decision input | Client sends only IDs+boolean; tool name/args resolved from persisted interrupt server-side ("never client input") | `web/src/features/in-app-agent/server/backgroundRunService.ts:379-400`; `worker/src/features/in-app-agent/executeInAppAgentRun.ts:691-708` |
| Who may approve (owner-only v1) | Router requires project membership + owned conversation; worker re-checks `conversation.createdByUserId === run.triggeredByUserId` at claim time | `web/src/features/in-app-agent/server/backgroundRunService.ts:366-371`; `worker/src/features/in-app-agent/executeInAppAgentRun.ts:218-224` |
| Rejection cannot widen scope | Input refinement "A rejection cannot grant a tool"; rejection produces error tool result + developer guidance not to retry | `web/src/features/in-app-agent/server/router.ts:79-88`; `worker/src/features/in-app-agent/runtime/human-in-the-loop.ts:43-76, 128-141` |
| Server-side allowlist at MCP endpoint | Override header parsed with `InAppAgentMcpRunOverrideSchema`; invalid header falls back to read-only; `canCallTool` enforces `readOnlyHint` or exact allowlist match | `web/src/pages/api/public/mcp/index.ts:193-217`; `web/src/features/mcp/server/registry.ts:170-189` |
| Ephemeral scoped credential per run | Per-run API key minted and linked to run row atomically; deletion refuses non-agent keys (`isInAppAgentKey` guard); cleanup on terminal/reconcile | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:382-399`; `packages/shared/src/server/auth/apiKeys.ts:101-162`; `packages/shared/src/in-app-agent/server/runLifecycle.ts:757-803` |
| One-off override dropped after single call | Adapter recreated after approved call so standing grants remain but one-off does not persist | `worker/src/features/in-app-agent/runtime/agent.ts:634-696` |
| Durability window handling | Approved mutation whose result never persisted → FAILED `outcome_unknown` ("Verify before retrying"), never generically retried | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:156-167` |
| Audit trail of sensitive ops | `auditLog()` records before/after with user or apiKey attribution; agent-key writes attributed to creating user; `auditLogs:read` gated to org OWNER/ADMIN | `web/src/features/audit-logs/auditLog.ts:54-155`; `web/src/features/rbac/constants/organizationAccessRights.ts:14,30,38` |
| Admin-access oversight webhook | Global-admin project/org access triggers deduped webhook (24h window) | `web/src/server/adminAccessWebhook.ts:12-82`; `web/src/server/api/trpc.ts:338-342, 370-376, 472-477` |

## Answers to Dimension Questions

**1. Which actions require approval?**
Within the in-app agent: every mutating Langfuse MCP tool. The exhaustive policy map classifies each tool; e.g. `listPrompts` is `"auto"` while `createTextPrompt`, `updatePromptLabels`, `createScore`, `deleteDatasetItem`, `deleteEvaluationRule` are `"approval"` (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:260-287, 104-107, 176-179, 264-275`). Roughly: reads auto, creates/updates/deletes require approval. Outside the agent, **no human-approval workflow exists** — destructive product operations like `traces:delete` or `apiKeys:CUD` are gated purely by role membership in the RBAC matrix (`packages/shared/src/features/rbac/projectAccessRights.ts:91-149`). Notable exception inside the agent: sandbox tools `read/write/edit/bash` are locally auto-approved (`mcpPolicy.ts:410-421`), so arbitrary sandbox shell runs without a per-call gate.

**2. Who can approve?**
The authenticated project member who owns the agent conversation (v1 owner-only model). The tRPC route runs under `protectedProjectProcedure` and then `getOwnedConversationOrThrow` (`web/src/features/in-app-agent/server/backgroundRunService.ts:355-371`); the worker independently revalidates that the run principal still owns the conversation and is still a member (`executeInAppAgentRun.ts:214-257`). The decider's userId is recorded on the decision event (`approvalEvents.ts:13`). A rejection can never carry a grant (`router.ts:85-88`). There is no delegation or second-person approval (no "maker/checker").

**3. Are approvals scoped and expiring?**
Yes, narrowly scoped. A decision defaults to `"once"` (single tool call); `"conversation"` pushes the prefixed tool name into `alwaysAllowedTools` for that conversation only (`backgroundRunService.ts:396-400`, `runLifecycle.ts:318-343`). Pending requests expire after 24h (`tunables.ts:17`, enforced both at decide time `runLifecycle.ts:262-281` and on read `runLifecycle.ts:736-744`); a newer message supersedes a parked approval (`runLifecycle.ts:162-174`); cancellation settles with `approval_cancelled` (`runLifecycle.ts:509-556`). Standing grants are not time-boxed, but they are capability-scoped (per conversation, per tool name) and **revalidated against RBAC every run**, so they die with role changes or loss of project membership even though the row persists (`executeInAppAgentRun.ts:335-344`, tested in `packages/shared/src/in-app-agent/server/tools.test.ts:6-24`). The runtime credential itself (ephemeral MCP API key) is strictly per-run and deleted on terminal state.

**4. Can policy override model intent?**
Yes, at three independent points. (a) Availability: tools outside the user's RBAC scopes are filtered out before the model ever sees them (`mcpPolicy.ts:481-499`, README statement at `web/src/features/in-app-agent/README.md:231`). (b) Execution: the MCP endpoint ignores what the model requests and enforces `readOnlyHint`/allowlist per key context (`registry.ts:170-189`), with the allowlist built server-side from persisted state, never client input. (c) Continuation: an approved continuation resumes only the specific persisted interrupt's tool call; anything else still requires its own approval (`human-in-the-loop.ts:29-126`). Model intent cannot escalate: approval "does not widen the user's project permissions."

> **Can approval be granted narrowly rather than globally?**
> Yes — this is the strongest property of the design. Granularity is (conversation × tool name × scope): one-shot decisions authorize exactly one `toolCallId`; conversation grants authorize exactly one named tool within one conversation; nothing grants project-wide or cross-conversation authority. The narrowest unit is reinforced by dropping the one-off MCP override after the approved call completes (`agent.ts:634-696`).

## Architectural Decisions

1. **Static code-defined policy matrices over dynamic policy.** Roles→scopes are TypeScript const objects (`projectAccessRights.ts:91-272`), giving type-safe exhaustiveness (`Record<Role, ProjectScope[]>`) at the cost of requiring deploys for policy changes. No external policy engine (OPA/Cedar-style) integration was found.
2. **Exhaustive approval classification keyed to the MCP registry.** New MCP tools fail compilation/tests until classified `"auto"` or `"approval"` (`mcpPolicy.ts:22-26` comment; verified by the web stream servertest asserting registry equality, referenced at `web/src/features/in-app-agent/README.md:233`).
3. **Postgres-owned lifecycle for approvals.** The parked run row (`status=AWAITING_APPROVAL`, `finishedAt` set) plus the parent-run CAS is the exactly-once decision guarantee; the decision event and any standing grant are committed in the same transaction (`runLifecycle.ts:283-343`). No side table for pending approvals — the legacy `InAppAgentPendingToolApproval` table is explicitly unused by background execution (`schema.prisma:315-318`).
4. **Defense in depth across trust boundaries.** Web validates identity/ownership; worker revalidates everything at claim time ("nothing from enqueue time is trusted", `executeInAppAgentRun.ts:196-257`); the MCP endpoint independently enforces key-level tool permissions. Each layer would have to fail simultaneously for an unapproved mutation.
5. **Ephemeral credentials instead of long-lived agent keys.** Each run mints a dedicated project-scoped API key flagged `isInAppAgentKey`, linked to the run row atomically, and deletes it on completion (`executeInAppAgentRun.ts:382-399`); deletion APIs refuse to remove normal keys through the agent path (`apiKeys.ts:124-134`).

## Notable Patterns

- **Gate ordering documented and testable:** RBAC availability → human approval → server-side allowlist → scoped credential. Each stage lives in a different process (browser/web/worker/web-API), yet all consume the same shared parser and policy module (`interrupts.ts:15-16`, `README.md:235`).
- **Zero-trust decision payloads:** the decide mutation accepts only IDs and a boolean; tool identity and args are replayed from the persisted interrupt (`backgroundRunService.ts:396-400`), eliminating tampering and fingerprint-sync bugs.
- **Read-time reconciliation:** stale states (expired approval, lost worker, queue timeout) are failed lazily on read by a pure classifier (`classifyStaleRun`, `runLifecycle.ts:687-747`), avoiding cron-based sweeps while keeping list views honest.
- **Durability honesty:** if an approved mutation executed but its result was never persisted, the run is marked `outcome_unknown` with user-facing guidance to verify before retrying, instead of silently reporting cancel/failure (`executeInAppAgentRun.ts:156-167, 522-538`).
- **Oversight hooks for bypass paths:** global-admin access to foreign orgs/projects fires a deduped webhook from the very middlewares that grant the bypass (`trpc.ts:338-376`).

## Tradeoffs

- **Compile-time policy vs operational flexibility:** changing who can approve what requires a code change; acceptable for a product, but it forecloses tenant-defined policies.
- **Sandbox auto-approval:** `read/write/edit/bash` execute without per-call confirmation (`mcpPolicy.ts:410-421`); this keeps the agent usable but concentrates risk in sandbox isolation (mitigated by MicroVM provider; the dev provider is literally named `dangerous-docker`, `executeInAppAgentRun.ts:357-361`).
- **Grants live forever until revalidation:** `alwaysAllowedTools` has no TTL; safety depends entirely on per-run RBAC revalidation, and there is no UI/API to inspect or revoke individual grants (only deleting the whole conversation clears state implicitly).
- **Event-stream-as-audit:** approval decisions are render-only events in the append-only conversation stream (`approvalEvents.ts:6`), separate from the structured `auditLog` pipeline; querying an approval history across conversations is not supported by the data model.
- **Owner-only approvals (v1):** teammates viewing a conversation cannot approve; simple, but blocks delegation scenarios.

## Failure Modes / Edge Cases

- **Concurrent double-decide:** second decide hits the CAS and gets `CONFLICT` ("already decided") — covered by `web/src/__tests__/server/in-app-agent-background-run.servertest.ts:799-807`.
- **Expired approval decided late:** returns conflict with `approval_expired` recorded on the parent run (`runLifecycle.ts:262-290`).
- **New user message while parked:** prior approval cancelled `approval_superseded` — tested at `in-app-agent-background-run.servertest.ts:642-667`.
- **Grant orphaned by role change:** stored grant no longer matching the user's scope silently stops applying — tested at `tools.test.ts:6-24` ("drops a stored grant the user's role no longer covers") and end-to-end policy gating at `worker/src/features/in-app-agent/runtime/agent.test.ts:1313-1379`.
- **Approved-but-unrecorded outcome (crash mid-mutation):** surfaced as `outcome_unknown`, never retried automatically (`executeInAppAgentRun.ts:156-167`).
- **Malformed override header:** falls back to read-only permissions rather than failing open (`mcp/index.ts:203-216`).
- **Rolling-deploy compatibility:** override payload keeps a singular legacy field so already-enqueued continuations remain executable (`mcpPolicy.ts:399-403`).
- **Legacy session shape inherited-role confusion:** access checks deliberately ignore prototype-inherited `role`/`admin` properties — regression-tested at `web/src/features/rbac/utils/checkAccess.clienttest.ts:7-57`.

## Future Considerations

- Add first-class grant management: list/revoke `alwaysAllowedTools` per conversation (today revocation is implicit via RBAC changes or conversation deletion).
- Consider TTLs on standing grants, or at least last-used timestamps, to bound blast radius of dormant grants.
- Mirror approval decisions into the structured `auditLog` store so approvals get the same queryability/attribution guarantees as other sensitive mutations (`auditLog.ts:86-155`).
- Evaluate a confirmation gate (or stricter egress policy) for sandbox `bash`, which currently skips the human-approval tier entirely (`mcpPolicy.ts:410-421`).
- Replace the deprecated `InAppAgentPendingToolApproval` table once migration windows close (`schema.prisma:315-318`).
- Externalize or version the approval policy map if third-party MCP surfaces grow beyond the internal registry.

## Questions / Gaps

- **No evidence found for delegated/multi-party approval** (e.g., an admin approving on behalf of another user, or maker-checker flows). Search boundary: `grep` for approve/decide across `web/src/features/in-app-agent`, `packages/shared/src/in-app-agent`, `ee/`; only self-approval by the conversation owner appears.
- **No evidence found for time-limited standing grants** — `alwaysAllowedTools` (`schema.prisma:248-249`) has no expiry column; only the pending request has a TTL.
- **Entitlement/plan gating interacts with availability but not with approval classification**: plan checks (e.g., `getPlan`, `assertInAppAgentAvailable`) gate whether the agent runs at all (`web/src/features/in-app-agent/server/router.ts:210-223`); no evidence that plans alter which tools need approval.
- The MCP endpoint rejects org-scoped keys for the in-app agent path (`allowInAppAgentKey` plumbing at `web/src/features/public-api/server/apiAuth.ts:280-281`), but a full enumeration of which REST routes accept agent keys was not performed (out of dimension scope).

---

Generated by `Dimension 08.02: Permission Policy and Approval Gates` against `langfuse`.
