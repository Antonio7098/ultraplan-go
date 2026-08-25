# Source Analysis: langfuse

## Security Auditability (Dimension 08.04)

> Citation note: all file paths below are relative to the analyzed source root
> `studies/agent-harness-study/sources/langfuse/` (e.g. `web/src/...` = `studies/agent-harness-study/sources/langfuse/web/src/...`). No files outside that source directory were inspected.

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js app (`web`: UI + tRPC + public REST + MCP server), BullMQ worker (`worker`), shared package (`packages/shared`, Prisma/Postgres + ClickHouse) |
| Analyzed | 2026-08-24 |

## Summary

Langfuse implements security auditability as a first-class product feature with two complementary layers:

1. **A relational audit log** (`audit_logs` Postgres table, Prisma model at `packages/shared/prisma/schema.prisma:852-876`) written through a single central helper `auditLog()` (`web/src/features/audit-logs/auditLog.ts:86-155`) from 100+ mutation call sites across tRPC routers, public-API services, MCP write tools, and billing webhooks. Each row records actor type (USER or API_KEY), the actor's org/project roles *at action time*, resource type/id, action, and stringified `before`/`after` JSON diffs. A notable agent-specific rule: actions taken by in-app-agent MCP API keys are attributed back to the human who created the key (`web/src/features/audit-logs/auditLog.ts:96-122`), closing the "the agent did it" attribution gap.

2. **An append-only, replayable event stream for the in-app agent harness** (`in_app_agent_events` with monotonic `sequenceNumber`, `packages/shared/src/in-app-agent/server/persistence.ts:158-294`) that durably records every tool call lifecycle (START/ARGS/END/RESULT), plus explicit approval-request interrupts (`on_interrupt`, `packages/shared/src/in-app-agent/interrupts.ts:11-40`) and approval-decision events carrying `toolCallId`, `approved`, and `decidedByUserId` (`packages/shared/src/in-app-agent/approvalEvents.ts:6-28`).

Policy decisions are enforced by an exhaustive per-tool policy map (`auto` vs `approval`, each bound to an RBAC scope) at `packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`, recomputed from the user's *current* role on every run (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:330-348`), and re-enforced independently at the MCP HTTP boundary where in-app-agent keys are read-only unless a server-minted allowlist is present (`web/src/features/mcp/server/registry.ts:170-189`, `web/src/pages/api/public/mcp/index.ts:193-217`). Approval decisions are exactly-once via a conditional-update CAS on the parked parent run, expire after a fixed TTL, and persist conversation-scoped "always allow" grants in the same transaction (`packages/shared/src/in-app-agent/server/runLifecycle.ts:222-397`).

The main weaknesses are durability/integrity of the audit trail rather than its construction: audit rows cascade-delete when a project or user is deleted (`packages/shared/prisma/migrations/20240212175433_add_audit_log_table/migration.sql:25,28`), there is no tamper-evidence or policy-decision ID store (policy is implicit in code version), authentication events are not audited, and the audit-log viewer/export is plan-gated away from OSS/self-hosted-pro deployments.

## Rating

**8 / 10** — Clear, well-tested model with explicit interfaces and operational safeguards. The audit writer is centralized and typed (`AuditableResource` union, `web/src/features/audit-logs/auditLog.ts:7-52`), behavior is pinned by integration tests including a fail-closed path when audit writes fail (`web/src/__tests__/server/mcp-public-api-tools.servertest.ts:400-413`) and agent-key attribution (`web/src/__tests__/server/audit-log-in-app-agent.servertest.ts:40-88`), and the human-in-the-loop trail (decision events + CAS + TTL + lineage) is unusually rigorous. It stops short of 9–10 because retention integrity (cascade deletes), tamper evidence, auth-event coverage, and universal viewer availability (entitlement gating) are not addressed.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Security event log schema | `AuditLog` Prisma model: `type` (USER/API_KEY), `apiKeyId`, `userId`, `orgId`, `userOrgRole`, `projectId`, `userProjectRole`, `resourceType`, `resourceId`, `action`, `before`/`after` JSON; indexed on project/apiKey/user/org/createdAt | `packages/shared/prisma/schema.prisma:852-876` |
| Central audit writer | `auditLog(log)` serializes before/after and writes USER-type rows with session userId + org/project role snapshot | `web/src/features/audit-logs/auditLog.ts:86-155` |
| Agent-key attribution | API-key branch resolves `isInAppAgentKey` → writes `createdByUserId` as `userId`, so agent tool mutations are attributed to the owning human | `web/src/features/audit-logs/auditLog.ts:96-122` |
| Auditable surface breadth | `AuditableResource` union enumerates 45+ resources incl. `apiKey`, `orgMembership`, `projectMembership`, `ssoConfig`, `llmApiKey`, `batchAction` | `web/src/features/audit-logs/auditLog.ts:7-52` |
| Write-site coverage | Audit calls in members router (membership CUD), LLM API keys, traces/scores deletion, datasets, automations, Slack OAuth, SSO config, verified domains, Stripe billing | `web/src/features/rbac/server/membersRouter.ts:267-305`; `web/src/features/llm-api-key/server/router.ts:287,353,706`; `web/src/server/api/routers/traces.ts:547-634`; `web/src/ee/features/multi-tenant-sso/server/ssoConfigRouter.ts:160,240`; `web/src/ee/features/billing/server/stripe/stripeWebhookHandler.ts:450-729` |
| MCP write tools audited | Annotation-queue assignment creation increments audit log count; test asserts duplicate calls are audited too | `web/src/__tests__/server/mcp-public-api-tools.servertest.ts:369-398` |
| Fail-closed auditing for MCP | Test mocks `$transaction` rejection ("audit failed") and expects the tool handler to reject — audit failure blocks the mutation on this path | `web/src/__tests__/server/mcp-public-api-tools.servertest.ts:400-413` |
| Agent key creator stored | Integration test: in-app-agent MCP keys persist `createdByUserId`; audit logs written under that key carry the creator's userId | `web/src/__tests__/server/audit-log-in-app-agent.servertest.ts:8-88` |
| Audit read authorization | Project view requires entitlement `audit-logs` AND RBAC scope `auditLogs:read`; org view additionally filters `projectId: null` | `web/src/server/api/routers/auditLogs.ts:86-99,186-221` |
| RBAC scope tables | `projectRoleAccessRights` grants `auditLogs:read` only to OWNER/ADMIN; MEMBER/VIEWER excluded; same at org level | `packages/shared/src/features/rbac/projectAccessRights.ts:91-272` (scope list :76); `web/src/features/rbac/constants/organizationAccessRights.ts:20-43` |
| Export gate + tests | `audit_logs` batch export blocked for MEMBER even on cloud:team and for OWNER without entitlement; allowed combos tested; worker re-checks table name | `web/src/__tests__/server/batchExport-trpc.servertest.ts:89-166`; `web/src/features/batch-exports/server/batchExport.ts:70` |
| Viewer UI columns | Time, Actor (user avatar or apiKey publicKey), Resource Type/ID, Action, Before/After render cells + batch-export button | `web/src/ee/features/audit-log-viewer/AuditLogsTable.tsx:56-160` |
| Entitlement gating of visibility | `audit-logs` absent from `oss` and `self-hosted:pro` entitlements; present on cloud:team/enterprise and self-hosted:enterprise only | `web/src/features/entitlements/constants/entitlements.ts:30-34,99-141,152-182` |
| Capability usage log (agent) | Append-only persisted AG-UI event stream with per-conversation monotonic `sequenceNumber`; append fenced to RUNNING runs in one transaction | `packages/shared/src/in-app-agent/server/persistence.ts:158-267` |
| Tool execution records | Persisted events include TOOL_CALL_START/ARGS/END/RESULT with full args deltas and result content/error | `packages/shared/src/in-app-agent/server/persistence.ts:806-839` |
| Tool-call archive for sandbox | Reconstructs `tool_calls/<ts>_<tool>_<id>.json` files (request/response/error) from persisted events for later sandbox runs | `packages/shared/src/in-app-agent/server/persistence.ts:296-425` |
| OTel tracing of MCP calls | Every MCP tool executes inside a span stamped with projectId/orgId/apiKeyId and outcome attributes `mcp.outcome`, `mcp.error.http_code` | `web/src/features/mcp/core/run-mcp-tool.ts:24-74` |
| Worker-side instrumentation | Records tool approvals (`recordToolCallApproval`), execution start/end times, model calls into an internal Langfuse trace tagged `in-app-agent` | `worker/src/features/in-app-agent/runtime/instrumentation.ts:260-341,312-341` |
| Approval request record | Durable interrupt event `on_interrupt` (toolCallId, toolName, args, runId) parsed in browser and server runtimes | `packages/shared/src/in-app-agent/interrupts.ts:11-40` |
| Approval decision record | `langfuse_approval_decision` custom event schema `{toolCallId, approved, decidedByUserId}` described as "render-only approval history ... append-only event stream" | `packages/shared/src/in-app-agent/approvalEvents.ts:6-28` |
| Exactly-once decision + TTL | `decideToolApproval`: locks conversation, rejects non-AWAITING_APPROVAL parents, expires after `IN_APP_AGENT_APPROVAL_TTL_MS`, CAS `AWAITING_APPROVAL→SUCCEEDED`, then persists decision event + continuation run | `packages/shared/src/in-app-agent/server/runLifecycle.ts:222-397`; TTL constant `packages/shared/src/in-app-agent/server/tunables.ts:17` (24h) |
| Grant persistence | Conversation-scoped `alwaysAllowedTools` push happens in the same transaction as the decision CAS; comment: grants "revalidated against the registry and owner's role each run" | `packages/shared/src/in-app-agent/server/runLifecycle.ts:318-343`; `packages/shared/prisma/schema.prisma:248-249` |
| Run lineage | Continuation runs carry `parentRunId`, `rootRunId`, `continuationNumber`, `approvalRequestedAt`, `approvalDecidedAt`, `triggeredByUserId=decider` | `packages/shared/src/in-app-agent/server/runLifecycle.ts:359-378`; `worker/src/features/in-app-agent/executeInAppAgentRun.ts:300-322` |
| Superseded approvals audited | New input cancels parked approvals with `APPROVAL_SUPERSEDED`; distinct failure message surfaced in UI | `packages/shared/src/in-app-agent/server/runLifecycle.ts:163-174`; `web/src/features/in-app-agent/lib/backgroundExecutionSession.ts:573-590` |
| Policy decision source | Exhaustive map of every Langfuse MCP tool to `auto`/`approval` + availability scope; type-level exhaustiveness against registry documented in comments | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:22-360` |
| Policy recomputation per run | Tool policy rebuilt from current DB role each run so revoked roles drop out; missing triggering user aborts the run ("must never implicitly mean trusted system") | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:235-258,330-348` |
| Boundary enforcement (defense in depth) | MCP route derives in-app-agent context from key flag + signed override header only; registry `canCallTool` allows read-only unless tool is on minted allowlist; grant name resolved from persisted interrupt, never client input | `web/src/pages/api/public/mcp/index.ts:134-148,193-217`; `web/src/features/mcp/server/registry.ts:170-189`; `web/src/features/in-app-agent/server/backgroundRunService.ts:396-413` |
| Read-only default for agent keys | `InAppAgentContext` doc: "read-only unless a prior approval mints a mutating-tool allowlist"; no header ⇒ `{permissions:"read"}`; malformed header ⇒ read-only | `web/src/features/mcp/types.ts:64-76`; `web/src/pages/api/public/mcp/index.ts:201-216` |
| Sandbox tool scoping | `read/write/edit/bash` auto-approved but routed to a sandbox; dev-only `dangerous-docker` provider logs a warning | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:410-421`; `worker/src/features/in-app-agent/executeInAppAgentRun.ts:350-361` |
| Conversation access control | Owner-only assertion with non-enumerating `LangfuseNotFoundError` for deleted/foreign conversations | `packages/shared/src/in-app-agent/server/persistence.ts:52-63,99-122` |
| Retention integrity caveat | FKs `user_id`/`project_id` on `audit_logs` are `ON DELETE CASCADE` — deleting a user or project destroys their audit entries | `packages/shared/prisma/migrations/20240212175433_add_audit_log_table/migration.sql:25,28` |

## Answers to Dimension Questions

1. **Who did what?** Yes, dual-attributed. Session actions record `userId` plus the actor's `userOrgRole`/`userProjectRole` snapshot at action time (`web/src/features/audit-logs/auditLog.ts:124-153`, columns at `packages/shared/prisma/schema.prisma:858-862`). API-key actions record `apiKeyId` (+publicKey shown in the viewer, `web/src/ee/features/audit-log-viewer/AuditLogsTable.tsx:97-104`), and in-app-agent keys are resolved to their human creator so agent-driven mutations remain attributable (`web/src/features/audit-logs/auditLog.ts:96-122`, tested in `web/src/__tests__/server/audit-log-in-app-agent.servertest.ts:40-88`). Agent conversations additionally record `createdByUserId` and every run's `triggeredByUserId` (`packages/shared/prisma/schema.prisma:242,286`). Gap: interactive sign-in/SSO login events are not audited (no `auditLog` usage found under `web/src/features/auth/**`).
2. **What policy allowed it?** Partially reconstructable. The applicable policy is code-defined: RBAC scope tables (`packages/shared/src/features/rbac/projectAccessRights.ts:91-272`) and the exhaustive tool approval/availability map (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`). Because the audit row snapshots the actor's roles, an auditor can infer which policy entries applied. However there is **no policy-decision ID, no policy-version stamp, and no decision-record store** — the answer lives in the repo version that was deployed, which is weaker than an explicit decision log.
3. **Was a human involved?** Explicitly recorded for the agent harness. Every approval request persists an interrupt event and every decision persists `langfuse_approval_decision` with `decidedByUserId` (`packages/shared/src/in-app-agent/approvalEvents.ts:10-14`); continuation runs are attributed to the deciding user (`runLifecycle.ts:359-378`). Standing grants are scoped per-conversation and expire/revalidate per run. For direct UI/API mutations, involvement is implicit (a user or key performed it) but not classified as "approved by another person".
4. **Can auditors reconstruct the decision?** Largely yes within the primary store: before/after JSON diffs (`schema.prisma:866-867`), a replayable ordered event stream (`persistence.ts:158-294, getConversationEvents`), run lineage (`rootRunId`/`continuationNumber`, `runLifecycle.ts:303-316`), sandbox `tool_calls/*.json` archives (`persistence.ts:296-425`), OTel spans per MCP call (`run-mcp-tool.ts:24-74`), and CSV batch export for offline review (`web/src/components/BatchExportTableButton.tsx:88`, gated by `batchExport-trpc.servertest.ts:89-166`). Reconstruction degrades under three documented conditions: project/user deletion cascades away audit rows (`migration.sql:25,28`), there is no tamper-evidence/hash chain, and viewers without the paid entitlement cannot reach the product surfaces at all (`entitlements.ts:30-34,141-162`).

## Architectural Decisions

- **Single choke-point writer**: all audit rows flow through `auditLog()` with a discriminated-union actor payload (session | userId | apiKeyId), making the schema uniform and the writer easy to evolve (`web/src/features/audit-logs/auditLog.ts:54-84,86-155`).
- **Roles frozen into the log row**: storing `userOrgRole`/`userProjectRole` at write time means later role revocation cannot rewrite history — the log answers "what could they do then?" (`packages/shared/prisma/schema.prisma:860,862`).
- **Event-sourced agent transcript**: the durable AG-UI event stream is the source of truth for both UI hydration and audit reconstruction, with monotonic sequence numbers used as watch cursors (`packages/shared/src/in-app-agent/server/persistence.ts:72-83,158-267`).
- **Policy-as-code with exhaustiveness enforcement**: new MCP tools must be classified `auto`/`approval` before the agent can gate them, enforced by `satisfies Record<...>` typing and runtime assertions called out in comments (`mcpPolicy.ts:22-27,360`).
- **Two independent enforcement points**: the worker builds the allowlist from current role + grants, and the web MCP endpoint independently filters calls via `canCallTool`; a compromised worker override cannot widen beyond the parsed header allowlist (`registry.ts:170-189`, `pages/api/public/mcp/index.ts:193-217`).
- **Exactly-once approvals via CAS, not locks alone**: `updateMany` conditioned on `status = AWAITING_APPROVAL` guarantees a decision applies once; late/duplicate deciders get a conflict error (`runLifecycle.ts:283-297`).

## Notable Patterns

- **Fail-closed audit coupling**: the annotation-queue assignment MCP path performs its audit write inside the same `$transaction` as the mutation, and tests prove a failed audit write fails the tool call (`web/src/__tests__/server/mcp-public-api-tools.servertest.ts:388-413`) — audit is on the critical path there, not best-effort.
- **Attribution bridge for non-human actors**: `isInAppAgentKey` keys carry `createdByUserId`, and the audit writer substitutes it, giving agent actions a human owner without weakening the key-based actor record (`auditLog.ts:96-122`; schema flag `packages/shared/prisma/schema.prisma:212`).
- **Grant hygiene**: "always allow" choices are namespaced with the MCP surface prefix to avoid cross-surface collisions (`mcpPolicy.ts:365-367`), deduplicated on push (`runLifecycle.ts:330-343`), and dropped automatically if the user loses the underlying scope because the policy is rebuilt per run (`executeInAppAgentRun.ts:335-339`).
- **Non-enumerating failures**: unauthorized conversation access returns the same "not found" error whether the conversation is foreign or deleted (`persistence.ts:52-63`).
- **Negative-space guardrails encoded as errors**: a run whose `triggeredByUserId` is missing refuses to start with the comment "a missing user must never implicitly mean 'trusted system'" (`executeInAppAgentRun.ts:241-245`).

## Tradeoffs

- **Plan-gated observability vs universal logging**: audit rows are written unconditionally, but reading them via UI/API/export requires the `audit-logs` entitlement (`routers/auditLogs.ts:88-92,188-192`; `entitlements.ts:30-34,152-162`). OSS/self-hosted-pro operators get the data in Postgres but no product surface for it — good for cloud revenue, weaker auditability story for the majority self-hosted deployment.
- **Relational audit simplicity vs immutability**: Postgres rows with cascade FKs keep the implementation simple and queryable but mean deletions of users/projects silently shrink the trail (`migration.sql:25,28`); there is no append-only storage, hash chaining, or external sink option.
- **Policy-as-code vs decision records**: classifying every tool statically gives deterministic behavior and compile-time exhaustiveness, but foregoes runtime decision IDs and makes "what policy was active?" dependent on deployment version archaeology.
- **Convenience grant scope**: "allow for this conversation" trades friction for blast radius; mitigations (per-conversation scope, role revalidation each run, 24h request TTL) are deliberate compensating controls rather than elimination.
- **Org-scoped rate limiting**: the in-app agent rate-limit bucket is org-wide; a per-user submission cap is explicitly deferred in a TODO, with per-user concurrency capped separately (`web/src/features/in-app-agent/server/rateLimit.ts:16-25`).

## Failure Modes / Edge Cases

- **Audit-write failure handling varies by path**: fail-closed in the annotation-queue MCP service (transactional), while several tRPC call sites `await auditLog(...)` after the mutation succeeds — a failed audit write there throws after the fact, leaving the mutation applied without a log entry (e.g. pattern at `web/src/server/api/routers/comments.ts:109,184`). No unified compensation/rollback strategy exists.
- **Expired approvals leave intent unrecorded as an event**: expiry flips the run to `FAILED/APPROVAL_EXPIRED` (`runLifecycle.ts:262-281`) and the UI explains it (`backgroundExecutionSession.ts:581-582`), but unlike explicit approve/reject there is no `langfuse_approval_decision` event with `approved:false` for "ignored until timeout" — reconstruction relies on run status.
- **Cascade deletes**: deleting a project removes its audit logs entirely (FK cascade), so post-deletion investigations are impossible by design.
- **Unawaited/fire-and-forget writers**: some billing webhook paths invoke `auditLog` without awaiting (`stripeWebhookHandler.ts:680,729`), so process crashes can drop those entries.
- **Role snapshot staleness in viewer joins**: the read router resolves actor identity via live membership queries; if a user left the org between write and read, the actor falls back to bare id with null profile (`routers/auditLogs.ts:48-54`), though the role snapshot columns preserve the historical authorization context.
- **Sandbox archive gaps**: failed tool calls intentionally produce no `tool_calls/*.json` file (only successful results are archived, `persistence.ts:320-325,398-400`); the full record remains in the event stream but the convenience archive is success-biased.

## Future Considerations

- Add a policy-decision record: stamp each audit row (or run) with the effective policy version/tool-policy hash and a decision ID, making question 2 answerable without repo archaeology.
- Replace cascade deletes on `audit_logs.user_id`/`project_id` with `ON DELETE SET NULL` (keeping actor ids as opaque strings) or ship an immutable export sink, so trail lifetime is independent of entity lifetime.
- Extend `AuditableResource` coverage to authentication events (sign-in, SSO login failures, API-key usage anomalies), which currently have no audit representation.
- Provide an OSS-visible read path for audit logs (even API-only) or document direct-SQL export guidance for self-hosters.
- Add direct access-control tests for `auditLogs.all` / `allByOrg` (today only the batch-export path has explicit authorization tests, `batchExport-trpc.servertest.ts:89-166`).
- Implement the acknowledged per-user rate-limit cap for agent submissions (`rateLimit.ts:22-25`).

## Questions / Gaps

- **No evidence found** for tamper-evidence or immutability mechanisms on `audit_logs` (searched for hash chains, WORM/S3 sinks, delete guards across `web/src`, `worker/src`, `packages/shared`; only application/test code deletes rows, e.g. `web/src/__tests__/server/blob-storage-integration-trpc.servertest.ts:183`).
- **No evidence found** for audit logging of authentication/login events in `web/src/features/auth/**` or `web/src/pages/api/auth/**`.
- **No evidence found** of a dedicated unit/integration test asserting `auditLogs.all`/`allByOrg` RBAC+entitlement rejection directly (the batch-export equivalent exists); coverage is inferred from router code and the export-path tests.
- Whether the fire-and-forget audit calls in Stripe webhook handlers (`stripeWebhookHandler.ts:680,729`) have downstream retry/compensation could not be determined from the inspected code.
- The `InAppAgentPendingToolApproval` table is explicitly retained only for legacy rows and is unused by background execution (`packages/shared/prisma/schema.prisma:315-331`); its historical data is not part of the active approval trail.

---

Generated by `08.04-security-auditability` against `langfuse`.
