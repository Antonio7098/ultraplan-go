# Source Analysis: langfuse

## Dimension 09.03: Governance UX and Operator Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (web UI + tRPC + REST), worker (BullMQ), Prisma/Postgres, ClickHouse, Redis; shared package `@langfuse/shared` |
| Analyzed | 2026-08-26 |

## Summary

Langfuse implements governance-as-workflow in two distinct layers. The first is a **human review queue** product surface ("annotation queues") where operators triage traces/observations/sessions against predefined score configs, with server-side item locking (`fetchAndLockNext`), keyboard-first processing (⌘/Ctrl+Enter to complete + advance), per-queue pending/completed counters, RBAC-gated actions on both client and server, and an audit log entry for every mutation. The second is a **human-in-the-loop approval system for the in-app agent**: every non-read MCP tool call is suspended behind an `on_interrupt` event and rendered as an Approve / Always approve / Decline card; decisions are exactly-once via a compare-and-swap on the parked run row, resolved from persisted server-side state (client sends only IDs + boolean), TTL-bounded, rate-limited, and recorded in an append-only conversation event stream plus Postgres audit logs.

Bulk operator actions are implemented as durable async batch jobs (delete traces/scores, add-to-queue, add-to-dataset, run evaluation) with select-all-across-filter semantics, progress/failure counters surfaced in a settings dashboard, conflict guards against concurrent destructive jobs, and audit logging.

What Langfuse does **not** have: a unified cross-project "approvals inbox" for operators, first-class "evidence packs" (the closest analogues are the EE-gated audit-log viewer with before/after JSON snapshots and batch exports), or a rejection state in annotation queues (status enum is only `PENDING`/`COMPLETED`; skipping is purely client-side). Governance surfaces such as audit logs and protected prompt labels are additionally gated behind paid entitlements, so OSS operators get the workflow but not the full evidence trail UI.

## Rating

**8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale: every governance action is double-enforced (UI disablement via `useHasProjectAccess` + server-side `throwIfNoProjectAccess`), mutations are audited, destructive bulk work runs as observable async jobs with failure surfacing, and the agent approval path has unusually strong durability properties (exactly-once CAS, anti-tamper server-side tool resolution, expiry, transactional grant persistence) backed by server tests (`annotation-queue-items-trpc.servertest.ts`, `traces-batch-delete-trpc.servertest.ts`, `audit-log-in-app-agent.servertest.ts`). It falls short of 9–10 because there is no single operator cockpit aggregating what needs review across projects, no rejection/rework states, evidence-pack assembly is not first-class, and key evidence surfaces are entitlement-gated rather than universally available.

## Evidence Collected

Every entry cites workspace-relative paths under `studies/agent-harness-study/sources/langfuse/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Review queue list with pending/completed counts per queue | SQL aggregates `countCompletedItems` / `countPendingItems` per queue | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:64-96` |
| Queue table columns expose "Completed Items" / "Pending Items" to operators | Columns rendered from aggregated counts | `sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueuesTable.tsx:82-95` |
| Per-user queue assignment (`isCurrentUserAssigned`) | Query joins `annotationQueueAssignment` for current user | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:114-134` |
| Item status visible per row (pending vs completed + annotator identity) | `status` column with `StatusBadge`, `annotatorUser` column with avatar | `sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemsTable.tsx:306-357` |
| Server-side status transition PENDING→COMPLETED records annotator | `complete` mutation sets `annotatorUserId`, writes audit log | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:419-455` |
| Work-claiming lease: `fetchAndLockNext` stamps `lockedAt`/`lockedByUserId` | Selects oldest pending item not locked by others within 5 min | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:466-516` |
| Lock staleness check (5 minutes) reused by read path | `isItemLocked` helper | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:32-38` |
| Concurrent-editor warning shown to human operator | Red banner "Currently edited by {name}" when locked by another user | `sources/langfuse/web/src/features/annotation-queues/components/shared/AnnotationDrawerSection.tsx:53-60` |
| Keyboard-first processing under pressure (⌘/Ctrl+Enter complete, ←/→ navigate, `?` cheatsheet) | Keydown handler with typing-target/dialog guards and validity veto before completing | `sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:239-358` |
| Progress indicator "n / total" while processing | Counter chip computed from seen + unseen pending counts | `sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:103-138,433-486` |
| Unseen-pending counter endpoint drives "what still needs review" | `unseenPendingItemCountByQueueId` counts `PENDING` items not yet seen | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:253-279` |
| RBAC scope catalog incl. `annotationQueues:read/CUD`, `auditLogs:read`, `promptProtectedLabels:CUD` | `projectScopes` const array | `sources/langfuse/packages/shared/src/features/rbac/projectAccessRights.ts:5-86` |
| Role→scope matrix (OWNER/ADMIN/MEMBER/VIEWER/NONE) | `projectRoleAccessRights` record | `sources/langfuse/packages/shared/src/features/rbac/projectAccessRights.ts:91-272` |
| Client-side access hook used to disable UI affordances | `useHasProjectAccess` / `hasProjectAccess` | `sources/langfuse/web/src/features/rbac/utils/checkProjectAccess.ts:39-65` |
| Server-side enforcement in every queue mutation/query | `throwIfNoProjectAccess(...)` calls in each procedure | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:79-83,173-177,294-298,381-385,428-432` |
| Locked UI without write access ("Process queue" disabled with lock icon) | Access-gated button branch | `sources/langfuse/web/src/features/annotation-queues/pages/AnnotationQueueItems.tsx:41-73` |
| Bulk delete of queue items with confirm dialog and RBAC gate | Multi-select dropdown → destructive dialog → `deleteMany` | `sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemsTable.tsx:41-126` |
| Audit log written for queue create/update/delete, item create/delete/complete | `auditLog(...)` calls with before/after payloads | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:357-363,412-419,446-452`; `sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:342-353,396-404,447-453` |
| Audit-log writer stores actor (user/API key/in-app-agent key), roles, before/after JSON | `auditLog()` implementation with typed resource union | `sources/langfuse/web/src/features/audit-logs/auditLog.ts:7-52,86-155` |
| Audit-log viewer table (EE): actor, resource, action, Before/After JSON cells | `AuditLogsTable` columns | `sources/langfuse/web/src/ee/features/audit-log-viewer/AuditLogsTable.tsx:56-141` |
| Audit logs exportable via batch export button (evidence pack analogue) | `BatchExportTableButton tableName={BatchExportTableName.AuditLogs}` | `sources/langfuse/web/src/ee/features/audit-log-viewer/AuditLogsTable.tsx:149-161` |
| Audit log viewer gated behind `audit-logs` entitlement (project & org scopes) | `useHasEntitlement("audit-logs")` checks | `sources/langfuse/web/src/ee/features/audit-log-viewer/AuditLogsSettingsPage.tsx:12`; `sources/langfuse/web/src/ee/features/audit-log-viewer/OrgAuditLogsSettingsPage.tsx:12` |
| Bulk row selection incl. select-all-matching across pages | Banner offering "Select all N items across M pages" | `sources/langfuse/web/src/components/table/data-table-multi-select-actions/data-table-select-all-banner.tsx:17-58` |
| Declarative bulk actions with access check + entitlement + disabled reason | `tableActions` entries (`Delete Traces`, `Add to Annotation Queue`) | `sources/langfuse/web/src/components/table/use-cases/traces.tsx:640-669` |
| Destructive bulk delete warns about irreversibility and async delay (up to 24h) | Action description string | `sources/langfuse/web/src/components/table/use-cases/traces.tsx:643-657` |
| Batch job creation is audited and enqueued to worker | `createBatchActionJob`: audit log + `BatchActionQueue.add` | `sources/langfuse/web/src/features/table/server/createBatchActionJob.ts:147-196` |
| Conflict guard: only one active trace-delete batch action per project | Transactional create-or-reset with `CONFLICT` error | `sources/langfuse/web/src/features/table/server/createBatchActionJob.ts:110-145` |
| Batch-actions operations dashboard: status, processed/failed counts, log tooltip | `BatchActionsTable` columns incl. failed count in red and log popover | `sources/langfuse/web/src/features/batch-actions/components/BatchActionsTable.tsx:71-107,155-182` |
| Batch-action detail query exposes `processedCount/failedCount/log/config` | `batchAction.byId` / `batchAction.all` procedures | `sources/langfuse/web/src/features/batch-actions/server/batchActionRouter.ts:16-122` |
| Wizard-style bulk flows (add-to-dataset mapping steps, run-evaluation confirmation) | Step components with preview/status/error banner | `sources/langfuse/web/src/features/batch-actions/components/AddObservationsToDatasetDialog/FinalPreviewStep.tsx:1-15`; `sources/langfuse/web/src/features/batch-actions/components/AddObservationsToDatasetDialog/StatusStep.tsx:102` |
| Agent tool-call approvals: Approve / Always approve / Decline buttons with submitting states | Approval card UI | `sources/langfuse/web/src/features/in-app-agent/components/InAppAgentToolCallCard.tsx:64-144` |
| Approval decision schema records approver identity in event stream | `InAppAgentApprovalDecisionSchema {toolCallId, approved, decidedByUserId}` | `sources/langfuse/packages/shared/src/in-app-agent/approvalEvents.ts:10-18` |
| Interrupt (approval request) parsed from durable `mastra_suspend` event | `parseInAppAgentInterruptEvent` | `sources/langfuse/packages/shared/src/in-app-agent/interrupts.ts:7-37` |
| Exhaustive per-tool approval policy (auto vs approval) tied to RBAC scopes | `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` map (~90 tools) | `sources/langfuse/packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360` |
| Policy computes available/auto-approved sets from caller role + grants | `createInAppAgentToolPolicy` merges RBAC availability and conversation grants | `sources/langfuse/packages/shared/src/in-app-agent/server/mcpPolicy.ts:477-510` |
| Exactly-once approval decision via CAS on parked run; conflict message tells operator to reload | `decideToolApproval` transaction: parent must be `AWAITING_APPROVAL` | `sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:235-297` |
| Approval requests expire (TTL) → run marked FAILED with `APPROVAL_EXPIRED` | Expiry branch inside decision transaction | `sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:263-281` |
| Superseded approvals recorded as `APPROVAL_SUPERSEDED` cancellations | Reconcile loop outcome recording | `sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:210-216` |
| Conversation-scoped grant persisted in same transaction as decision | `alwaysAllowedTools` push + append of decision event | `sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:319-355` |
| Anti-tamper: client sends IDs + boolean only; tool name resolved from persisted interrupt | Doc comment + `getOwnedConversationOrThrow` + interrupt re-parse | `sources/langfuse/web/src/features/in-app-agent/server/router.ts:260-268`; `sources/langfuse/web/src/features/in-app-agent/server/backgroundRunService.ts:379-400` |
| Rejection cannot grant a tool (schema refinement) | `refine(input.approved || approvalScope === "once")` | `sources/langfuse/web/src/features/in-app-agent/server/router.ts:84-88` |
| Approval decisions rate-limited and capacity-checked like fresh runs | `assertInAppAgentRateLimit` + `assertInAppAgentRunCapacity` before deciding | `sources/langfuse/web/src/features/in-app-agent/server/router.ts:283-292` |
| Pending approvals derived from event stream minus decided tool calls | `getPendingToolApprovals` filters decided ids | `sources/langfuse/web/src/features/in-app-agent/server/backgroundRunService.ts:424-444` |
| Deleted-object exception surfaced during review ("ObjectNotFoundCard") | Error-code branch renders dedicated card | `sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:379-401` |
| Complete-of-deleted-item maps Prisma P2025 to friendly NOT_FOUND | Catch block translating DB error | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:461-471` |
| Protected prompt labels prevent non-admins from moving production labels | Settings UI explaining restriction; scope-gated mutations | `sources/langfuse/web/src/features/prompts/components/ProtectedLabelsSettings.tsx:109-146` |
| Hobby-plan quota on number of annotation queues (governance guardrail) | Plan check throws FORBIDDEN after 1 queue | `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:312-332` |
| Tests: RBAC enforced on queue item endpoints | `it("requires annotationQueues:read access")` | `sources/langfuse/web/src/__tests__/server/annotation-queue-items-trpc.servertest.ts:78-80` |
| Tests: trace-delete bulk job idempotency/conflict/comment-filter rejection | Test names cover reset-once, active-conflict, comment-filter guard | `sources/langfuse/web/src/__tests__/server/traces-batch-delete-trpc.servertest.ts:100-395` |
| Tests: in-app agent API keys attribute audit logs to creator | `stores the creator user id on audit logs...` | `sources/langfuse/web/src/__tests__/server/audit-log-in-app-agent.servertest.ts:7-42` |
| Status model has no REJECTED state (PENDING/COMPLETED only) | Prisma enum | `sources/langfuse/packages/shared/prisma/schema.prisma:526-529` |

## Answers to Dimension Questions

**1. Can operators see what needs review?**
Yes, at project scope. Each annotation queue lists pending vs completed counts (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueuesTable.tsx:82-95`, aggregate SQL at `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:75-96`), items show per-row status badges and who completed them (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemsTable.tsx:306-357`), and a dedicated endpoint counts unseen pending items to drive the processor's progress display (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:253-279`). Gaps: there is no cross-project or cross-queue aggregation view ("your pending work everywhere"), and queue assignment does not appear to generate notifications — assignment is stored (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueueAssignmentsRouter.ts`) but no push/notification wiring was found in the assignment router beyond invalidation/toasts.

**2. Can they act on approvals efficiently?**
Yes — this is a strength. The queue processor is keyboard-first: ⌘/Ctrl+Enter completes and advances, arrows navigate, numeric keys pick score options, `?` shows a cheatsheet (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:185-358,558-617`). The handler guards against held-key races while data loads (`:244-255`) and vetoes completion when a numeric score input is invalid (`:270-277`). For agent actions, one click approves/declines, with an explicit "Always approve for this conversation" shortcut scoped and persisted durably (`sources/langfuse/web/src/features/in-app-agent/components/InAppAgentToolCallCard.tsx:98-119,139-143`). Bulk efficiency comes from select-all-matching batch jobs (`sources/langfuse/web/src/components/table/data-table-multi-select-actions/data-table-select-all-banner.tsx:17-58`).

**3. Are exceptions surfaced?**
Largely yes. Concurrent-edit locks render a red warning naming the other user (`sources/langfuse/web/src/features/annotation-queues/components/shared/AnnotationDrawerSection.tsx:53-60`). Deleted source objects render a dedicated not-found card instead of crashing the processor (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:379-401`), and completing a concurrently deleted item returns a friendly NOT_FOUND (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:461-471`). Batch failures surface as red failed-counts plus a log popover in the settings dashboard (`sources/langfuse/web/src/features/batch-actions/components/BatchActionsTable.tsx:94-106,155-182`), and concurrent trace-delete attempts get an explicit CONFLICT message (`sources/langfuse/web/src/features/table/server/createBatchActionJob.ts:110-114`). Agent-side, expired approvals fail loudly with `APPROVAL_EXPIRED` (`sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:263-281`) and stale decisions return "already decided, reload" (`:293-297`). No evidence found of proactive operator notification (email/Slack) for failed batch jobs or expiring approvals within these features; failures are pull-based (operator must visit the dashboard).

**4. Is the governance UI usable under pressure?**
Strongly yes for the review loop: optimistic keyboard flow, progress counter, skip-without-completing, resumable seen-item history (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:65-76,182-210`), and server-side leasing so two annotators never silently duplicate work (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:485-516`). Durability of agent decisions under failure is well engineered (exactly-once CAS, TTL, supersede reconciliation). Weak points: destructive bulk deletes are irreversible with up-to-24h async windows and only a text warning (`sources/langfuse/web/src/components/table/use-cases/traces.tsx:643-656`); there is no scheduled-report/approval digest; and the audit trail UI is unavailable without the `audit-logs` entitlement (`sources/langfuse/web/src/ee/features/audit-log-viewer/AuditLogsSettingsPage.tsx:12`), which hurts incident response for OSS/self-host operators precisely when under pressure.

## Architectural Decisions

1. **Governance enforcement is duplicated at both tiers by design.** Every tRPC procedure opens with `throwIfNoProjectAccess` while the UI independently gates via `useHasProjectAccess` (e.g., `sources/langfuse/web/src/features/annotation-queues/pages/AnnotationQueueItems.tsx:37-46` vs `sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:199-203`). The scope vocabulary itself lives in the shared package so web, worker, and the agent policy share one matrix (`sources/langfuse/packages/shared/src/features/rbac/projectAccessRights.ts:91-289`).

2. **Work distribution through short-lived leases, not distributed locks.** `fetchAndLockNext` claims items by stamping `lockedAt`/`lockedByUserId` and ignores locks older than 5 minutes (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:482-516`), trading strictness for simplicity; the read path surfaces rather than blocks conflicts.

3. **Destructive/bulk operations become durable jobs, not inline mutations.** Row selections escalate to `BatchActionJob`s via a queue (`sources/langfuse/web/src/features/table/server/createBatchActionJob.ts:158-196`), giving observability (`totalCount/processedCount/failedCount/log`), deduplication by deterministic id (`generateBatchActionId`, `:90`), and single-flight protection for deletes (`:116-145`).

4. **Approvals are state-machine transitions over an append-only event stream with a relational CAS.** The interrupt request is persisted; the decision flips the parent run from `AWAITING_APPROVAL` via `updateMany` where-status guard (exactly-once), appends an immutable decision event carrying `decidedByUserId`, persists any conversation-scoped grant in the same transaction, and enqueues a continuation run (`sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:235-370`).

5. **Server-authoritative approval targets.** The decide endpoint accepts only IDs and a boolean; the tool name granted by "always allow" is re-derived from the persisted interrupt, eliminating client tampering (`sources/langfuse/web/src/features/in-app-agent/server/backgroundRunService.ts:396-400`, doc comment at `sources/langfuse/web/src/features/in-app-agent/server/router.ts:260-266`).

6. **Policy-as-data for agent tools.** ~90 MCP tools carry static `auto`/`approval` classification bound to RBAC scopes in one exhaustive map with type-level exhaustiveness assertions (`sources/langfuse/packages/shared/src/in-app-agent/server/mcpPolicy.ts:22-27,360`), so adding a tool forces a governance decision.

## Notable Patterns

- **Keyboard-shortcut cheatsheet + pulse feedback**: shortcuts are discoverable in-flow (`?` dialog) and trigger visual pulses on the corresponding buttons (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:212-234,462-484`).
- **Before/after snapshot auditing**: `auditLog()` serializes full `before`/`after` objects into JSON columns, displayed side-by-side in the viewer (`sources/langfuse/web/src/features/audit-logs/auditLog.ts:88-94`; `sources/langfuse/web/src/ee/features/audit-log-viewer/AuditLogsTable.tsx:121-140`).
- **Disabled-with-reason pattern for bulk actions**: `TableAction` supports `disabledReason` shown up-front (e.g., comment filters unsupported for batch delete) instead of failing at submit time (`sources/langfuse/web/src/components/table/use-cases/traces.tsx:631-650`).
- **Entitlement + access double gates on sensitive settings pages**: e.g., protected labels require both scope `promptProtectedLabels:CUD` and the `prompt-protected-labels` entitlement (`sources/langfuse/web/src/features/prompts/components/ProtectedLabelsSettings.tsx:52-56`).
- **Storybook coverage of approval states**: pending, submitting, and disabled approval card states have stories used as executable spec (`sources/langfuse/web/src/features/in-app-agent/components/InAppAgentToolCallCard.stories.tsx:157-243`).

## Tradeoffs

- **Lease-based claiming (5-min TTL)** avoids lock infrastructure but means an abandoned session's claimed item becomes silently claimable and could be processed twice if the first annotator returns later; conflict resolution is social (warning banner), not blocking.
- **Append-only event stream for approvals** gives replayability and audit history but requires derivation logic to compute current pending set (`sources/langfuse/web/src/features/in-app-agent/server/backgroundRunService.ts:424-444`); correctness depends on every decision path appending events.
- **Async batch deletes** protect the request path but leave a long window where "deleted" traces remain visible (toast says up to 15 min; action copy says up to 24 h — `sources/langfuse/web/src/components/table/use-cases/traces.tsx:557-561,647`), which can confuse operators verifying deletion.
- **Entitlement gating of audit logs** simplifies commercial packaging but creates a governance visibility cliff between plan tiers.
- **Two parallel governance systems** (annotation queues vs agent approvals vs batch-job dashboards) share patterns but not surfaces; an operator must know which subsystem owns "pending work" they care about.

## Failure Modes / Edge Cases

- **Completing a deleted item**: handled — Prisma `P2025` translated to NOT_FOUND with explanatory message (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueueItemsRouter.ts:461-471`).
- **Deleted source object mid-review**: dedicated `ObjectNotFoundCard` keeps the operator in flow (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:380-387`).
- **Stale/duplicate approval decisions**: rejected with reload guidance via CAS conflict (`sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:253-260,293-297`); expired approvals deterministically fail the run.
- **Concurrent destructive bulk ops**: second trace-delete attempt gets CONFLICT until the first leaves active status (`sources/langfuse/web/src/features/table/server/createBatchActionJob.ts:130-144`); regression-tested (`sources/langfuse/web/src/__tests__/server/traces-batch-delete-trpc.servertest.ts:148-199`).
- **Invalid numeric score right before complete**: keyboard handler scans DOM for `input[type=number]:invalid` and focuses it instead of dropping the value silently (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:270-277`).
- **Rejection-with-grant misuse**: zod refinement makes "decline + always allow" unrepresentable (`sources/langfuse/web/src/features/in-app-agent/server/router.ts:85-88`).
- **Not handled / no evidence found**: no operator-visible alerting channel (Slack/email) wired to failed batch actions or expired approvals inside these features; notification features exist elsewhere (`web/src/features/slack`, `web/src/features/monitors`) but were not connected to governance outcomes based on this search.

## Future Considerations

- A unified "review inbox": aggregate pending annotation-queue assignments and awaiting-approval runs across projects into one operator view with deep links; today's building blocks (`countPendingItems`, `isCurrentUserAssigned`, `pendingToolApprovals`) already exist separately (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueuesRouter.ts:114-134`; `sources/langfuse/web/src/features/in-app-agent/components/InAppAiAgentProvider.tsx:154-210`).
- Push notification on queue assignment and batch-action failure using the existing Slack integration surface.
- First-class "evidence pack" export bundling an item's trace, scores, comments, and decision history for external review/compliance; nearest primitives are audit-log batch export (`sources/langfuse/web/src/ee/features/audit-log-viewer/AuditLogsTable.tsx:152-158`) and trace downloads.
- A `REJECTED`/`NEEDS_REWORK` status or disposition reason on queue items; currently skip is invisible server-side because it exists only as local `seenItemIds` (`sources/langfuse/web/src/features/annotation-queues/components/AnnotationQueueItemPage.tsx:68,171-180`).
- Conversation-scoped "always allow" grants currently lack a visible management UI/revocation path in the reviewed code; an admin surface listing `alwaysAllowedTools` per conversation would close the loop.

## Questions / Gaps

- Is there any scheduled or event-driven notification for assigned annotators? The assignment CRUD (`sources/langfuse/web/src/features/annotation-queues/server/annotationQueueAssignmentsRouter.ts`) contains persistence and audit logging only; no notification dispatch was found (searched for notification hooks in the annotation-queues feature).
- Do org-level audit views include project-scoped events, or only org resources? Both viewers exist (`sources/langfuse/web/src/ee/features/audit-log-viewer/OrgAuditLogsSettingsPage.tsx:27`), but retention/scoping semantics were not verified in this pass.
- What is the intended operator story when the in-app agent's approval TTL expires mid-conversation (does the UI prompt re-dispatch)? The server fails the run with `APPROVAL_EXPIRED` (`sources/langfuse/packages/shared/src/in-app-agent/server/runLifecycle.ts:267-281`); the corresponding recovery UX was not traced end-to-end here.
- Rate-limit/capacity errors on the decide endpoint are enforced (`sources/langfuse/web/src/features/in-app-agent/server/router.ts:283-292`) but their user-facing rendering (toast vs inline retry) was not verified.

---

Generated by `Dimension 09.03: Governance UX and Operator Workflow` against `langfuse`.
