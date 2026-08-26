# Source Analysis: openhands

## Dimension 08.02: Permission Policy and Approval Gates

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands "agent-canvas" frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (React Router, TanStack Query, Zustand), consuming `@openhands/typescript-client` against a Python agent-server; Vite build, Vitest + Playwright tests |
| Analyzed | 2026-08-26 |

> All citations below are relative to the source root `studies/agent-harness-study/sources/openhands/`. This repo is explicitly the frontend only (`AGENTS.md`, "Repository Map": backend enforcement lives in the separate `software-agent-sdk` repo), so this study covers how permission policy is **modeled, configured, transported, and surfaced**, and flags where enforcement is delegated out of source.

## Summary

The approval gate in this source is a **confirmation-mode pipeline** with three layers:

1. **Policy configuration** — a per-user `conversation_settings` block holds `confirmation_mode` (boolean) and `security_analyzer` (`"llm" | "pattern" | "policy_rail" | null`), typed at `src/types/settings.ts:128-129` and defaulted off with the LLM analyzer enabled at `src/services/settings.ts:13-14` and `src/services/settings.ts:56-58`. The settings UI exposes these under a "Verification" section whose schema declares `security_analyzer` as depending on `confirmation_mode` (`src/mocks/settings-handlers.ts:407-441`, `depends_on` at line 437).

2. **Policy derivation at launch** — when a conversation starts, the client translates those flat settings into SDK-shaped policy objects: `NeverConfirm`, `AlwaysConfirm`, or `ConfirmRisky { threshold: "HIGH", confirm_unknown: true }` (`getConversationConfirmationPolicy`, `src/api/agent-server-adapter.ts:593-605`), plus a matching analyzer object (`LLMSecurityAnalyzer`, `PatternSecurityAnalyzer`, `PolicyRailSecurityAnalyzer`; `src/api/agent-server-adapter.ts:607-618`). Both are stamped into the `POST /api/conversations` payload at `src/api/agent-server-adapter.ts:1120-1121` and `src/api/agent-server-adapter.ts:1169-1173`. Actual enforcement (pausing the loop, vetoing tool calls) is performed by the agent-server, outside this repo.

3. **Approval UX and transport** — while executing, each action carries a `security_risk` field (`src/types/agent-server/core/events/action-event.ts:58-61`, enum at `src/types/agent-server/core/base/common.ts:59-64`). When execution status becomes `WAITING_FOR_CONFIRMATION` (`src/types/agent-server/core/base/common.ts:71`), it maps to `AgentState.AWAITING_USER_CONFIRMATION` (`src/hooks/use-agent-state.ts:24-25`), which renders Confirm/Reject buttons bound to a session-key-authenticated `POST .../events/respond_to_confirmation` call (`ConversationConfirmationButtons` at `src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-58`; transport split local/cloud at `src/api/event-service/event-service.api.ts:40-69`). High-risk actions get an alert banner and risk labels across the chat UI.

Separately from agent-action gating, there is an **organizational permission model** for Cloud: the server returns the caller's role plus a server-defined `permissions` array from `GET /api/organizations/{orgId}/me` (`src/api/cloud/organization-service.api.ts:85-126`), and mutating org-scoped LLM/agent profiles requires the `edit_org_settings` permission with an owner/admin role fallback (`useCanManageOrgProfiles`, `src/hooks/use-can-manage-org-profiles.ts:10,50-57`). In OSS mode this collapses to a stub hook that grants everything (`src/hooks/use-has-permission.ts:4-7`).

## Rating

**6 / 10 — Present but inconsistent and partially fragile.**

Rationale against the rubric:

- **For (toward 7–8):** The confirmation model is explicit and well-typed end to end — settings type (`src/types/settings.ts:128-129`), policy derivation (`src/api/agent-server-adapter.ts:593-605`), wire payload (`src/api/agent-server-adapter.ts:1120-1121`), state mapping (`src/hooks/use-agent-state.ts:17-35`), and response transport (`src/api/event-service/event-service.api.ts:40-69`) are all traceable. Policy derivation has a dedicated unit test asserting exact payload shapes (`__tests__/api/agent-server-adapter.test.ts:349-376`); persistence round-trips are tested (`__tests__/api/settings-service.test.ts:61-88`); the verification settings page and legacy-field dedup have render tests (`__tests__/routes/verification-settings.test.tsx:126-229`). Risk metadata is surfaced redundantly (card text, bash badge, banner), which is good defensive UX.
- **Against (holding at 6):** Enforcement is not in this source, so the gate's actual strength cannot be verified here (the client only *requests* a policy). Approval is strictly binary per pending action — no scoping, no expiry, no revocation mechanism exists anywhere in the tree. The OSS permission hook is a hard-coded `return true` (`src/hooks/use-has-permission.ts:4-7`). The pending-action detection heuristic is loose (any last agent event while awaiting confirmation, see Failure Modes). There is no test that exercises the actual confirm/reject mutation loop (no reference to `respond_to_confirmation` under `__tests__/` or `tests/e2e/` beyond live-test setup utilities). The mock schema's analyzer choices (`llm|none`, `src/mocks/settings-handlers.ts:433-436`) diverge from what the adapter actually understands (`llm|pattern|policy_rail`, `src/api/agent-server-adapter.ts:608-614`), hiding two analyzers from users.

## Evidence Collected

Every entry cites file paths relative to `studies/agent-harness-study/sources/openhands/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Agent states incl. confirmation trio | `AWAITING_USER_CONFIRMATION`, `USER_CONFIRMED`, `USER_REJECTED` enum members | src/types/agent-state.tsx:12-14 |
| Execution status incl. `WAITING_FOR_CONFIRMATION` | `ExecutionStatus.WAITING_FOR_CONFIRMATION = "waiting_for_confirmation"` | src/types/agent-server/core/base/common.ts:67-75 |
| Security-risk enum (approval metadata) | `SecurityRisk.UNKNOWN\|LOW\|MEDIUM\|HIGH` | src/types/agent-server/core/base/common.ts:59-64 |
| Per-action risk field | `ActionEvent.security_risk: SecurityRisk` — "The LLM's assessment of the safety risk of this action"; comment notes the LLM can predict `security_risk` inside the tool call itself | src/types/agent-server/core/events/action-event.ts:43-61 |
| Confirmation-mode setting type | `confirmation_mode: boolean; security_analyzer: string \| null` on `Settings` | src/types/settings.ts:128-129 |
| Defaults | `confirmation_mode: false`, `security_analyzer: "llm"` (flat + nested `conversation_settings`) | src/services/settings.ts:13-14, 56-58 |
| Settings schema shape | Verification section: `confirmation_mode` (boolean, major prominence) + `security_analyzer` with `depends_on: ["confirmation_mode"]` (mocked server schema) | src/mocks/settings-handlers.ts:403-441 |
| Settings UI route | `VerificationSettingsScreen` renders conversation-owned `verification` section; dedups deprecated agent-side copies | src/routes/verification-settings.tsx:4-12, 23-40 |
| Policy derivation | `getConversationConfirmationPolicy`: `NeverConfirm` / `ConfirmRisky{threshold:"HIGH",confirm_unknown:true}` / `AlwaysConfirm` | src/api/agent-server-adapter.ts:593-605 |
| Analyzer selection | `getConversationSecurityAnalyzer`: `llm→LLMSecurityAnalyzer`, `pattern→PatternSecurityAnalyzer`, `policy_rail→PolicyRailSecurityAnalyzer` | src/api/agent-server-adapter.ts:607-618 |
| Payload stamping | `confirmation_policy:` and optional `payload.security_analyzer` added to start-conversation request | src/api/agent-server-adapter.ts:1120-1121, 1169-1173 |
| Policy derivation test | `"derives confirmation and security settings the same way as OpenHands"` asserts `{kind:"ConfirmRisky",threshold:"HIGH",confirm_unknown:true}` + `{kind:"LLMSecurityAnalyzer"}` | __tests__/api/agent-server-adapter.test.ts:349-376 |
| State mapping | `ExecutionStatus.WAITING_FOR_CONFIRMATION → AgentState.AWAITING_USER_CONFIRMATION` | src/hooks/use-agent-state.ts:24-25 |
| State-mapping test | cached-status fallback test expects `AWAITING_USER_CONFIRMATION` | __tests__/hooks/use-agent-state.test.tsx:45-60 |
| Confirmation UI | Buttons shown only when awaiting + not yet submitted; high-risk `RiskAlert`; keyboard shortcuts (⌘↩ accept, ⇧⌘⌫ reject) | src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36, 92-118, 60-90 |
| Duplicate-submission guard | `submittedEventIds` Zustand store; id recorded before mutation | src/stores/event-message-store.ts:6-20; src/components/shared/buttons/conversation-confirmation-buttons.tsx:43-47 |
| Confirm/Reject buttons | `data-testid="action-confirm-button"` / `action-reject-button`, labeled Continue/Cancel | src/components/shared/action-tooltip.tsx:11-43 |
| Approval request type | `ConfirmationResponseRequest { accept: boolean; reason?: string }`, response `{ success }` | src/api/event-service/event-service.types.ts:1-8 |
| Approval transport | `EventService.respondToConfirmation`: cloud → proxied POST to runtime sandbox host w/ `X-Session-API-Key`; local → `ConversationClient.respondToConfirmation` | src/api/event-service/event-service.api.ts:39-69 |
| Chat input lockout | Input disabled while `AWAITING_USER_CONFIRMATION` | src/components/features/chat/interactive-chat-box.tsx:62-65 |
| Mode-enabled indicator | Lock icon tooltip rendered only when `settings.confirmation_mode` truthy | src/components/features/chat/confirmation-mode-enabled.tsx:7-27 |
| Risk surfacing (bash) | HIGH/MEDIUM risk label under command in terminal visualizer | src/components/features/chat/tool-visualizers/bash/bash.tsx:24-37 |
| Risk surfacing (cards) | Bash/MCP card content appends localized risk text for HIGH/MEDIUM only | src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-42, 87-101 |
| High-risk banner component | Only `severity === "high"` renders; medium/low return `null` (comment admits single-severity support) | src/components/shared/risk-alert.tsx:19-35 |
| Attention notification | Sound fires on transitions into `AWAITING_USER_CONFIRMATION` | src/hooks/use-agent-notification.ts:6-10, 36-48 |
| Render placement | Confirmation buttons appended to last message event cards | src/components/conversation-events/chat/event-message-components/user-assistant-event-message.tsx:127; generic-event-message-wrapper.tsx:126 |
| Settings persistence (local) | `conversation_settings.confirmation_mode/security_analyzer` hoisted into merged view model; saved via `PATCH /api/settings` `conversation_settings_diff` | src/api/settings-service/settings-service.api.ts:401-411, 631-640 |
| Settings persistence (cloud) | Flat cloud fields derived into nested `conversation_settings`; saved via `POST /api/v1/settings` with `conversation_settings_diff` passthrough | src/api/cloud/settings-service.api.ts:30-31, 100-123, 186-191; src/api/settings-service/settings-service.api.ts:668-671 |
| Persistence round-trip test | Save `{confirmation_mode:true, security_analyzer:"llm", max_iterations:33}` then assert normalized read-back | __tests__/api/settings-service.test.ts:61-88 |
| Org permission schema (Cloud) | `/api/organizations/{id}/me` returns `role` (`owner|admin|member`) + server-defined `permissions` array (e.g. `edit_org_settings`); null permissions on older servers | src/api/cloud/organization-service.api.ts:85-126 |
| Org permission gate | `useCanManageOrgProfiles`: local always true; cloud requires `permissions.includes("edit_org_settings")`, falls back to `owner|admin` roles; returns false while loading | src/hooks/use-can-manage-org-profiles.ts:10, 16-28, 50-58 |
| OSS permission stub | `useHasPermission(permission)` unconditionally `return true` ("In OSS mode, every permission string is granted") | src/hooks/use-has-permission.ts:1-7 |
| Stub consumers | Automations UI gates manage controls behind `useHasPermission("manage_automations")` | src/routes/automation-git-sync.tsx:56; src/components/features/automations/automation-card.tsx:64 |
| Conversation-level access errors | "not exist or no permission" toast path; `permissions: "public"\|"private"` on legacy conversation type | src/routes/conversation.tsx:117; src/hooks/query/use-user-conversation.ts:49; src/api/open-hands.types.ts:39 |
| MCP credential probes (read-only policy) | Validation `toolCall`s documented as MUST be read-only; run on every pre-save test | src/utils/mcp-credential-validation.ts:12-21, 52-84 |

## Answers to Dimension Questions

**1. Which actions require approval?**
Determined by the client-derived `confirmation_policy` sent at conversation launch (`src/api/agent-server-adapter.ts:593-605,1120-1121`):
- `confirmation_mode === false` → `{ kind: "NeverConfirm" }` — nothing requires approval (default, `src/services/settings.ts:13`).
- `confirmation_mode === true` + `security_analyzer === "llm"` → `{ kind: "ConfirmRisky", threshold: "HIGH", confirm_unknown: true }` — risky actions require approval.
- `confirmation_mode === true` + any other analyzer → `{ kind: "AlwaysConfirm" }` — every action requires approval.

Which specific action triggers the pause is decided server-side using the per-event `security_risk` (`src/types/agent-server/core/events/action-event.ts:58-61`) — the frontend never decides *which* actions pause, it only configures the policy and reacts to `WAITING_FOR_CONFIRMATION`.

**2. Who can approve?**
Whoever controls the conversation session: the approval POST authenticates with the conversation's `X-Session-API-Key` (cloud runtime path) or the local backend session key (`src/api/event-service/event-service.api.ts:46-68`; auth header conventions documented at lines 27-30). There is no distinct approver identity, role check, or second-person approval for agent actions in this source. For *administrative* mutations (org-scoped LLM/agent profiles), Cloud restricts to holders of the server-defined `edit_org_settings` permission, falling back to `owner|admin` roles (`src/hooks/use-can-manage-org-profiles.ts:50-58`); in OSS every authenticated user passes (`src/hooks/use-has-permission.ts:4-7`).

**3. Are approvals scoped and expiring?**
No evidence of scoping or expiry. An approval is a bare boolean `accept` on a single pending event (`src/api/event-service/event-service.types.ts:1-4`); no TTL, no per-tool/per-path grant objects, no stored grant records exist in this tree. The only "statefulness" is a client-memory guard preventing double submission of the same event ID (`submittedEventIds`, `src/stores/event-message-store.ts:6-20`), which does not survive a reload and confers no security property.

**4. Can policy override model intent?**
Structurally yes, but enforcement is out of source. The model's chosen action is tagged with its own claimed `security_risk` (and the LLM may even embed a predicted risk inside the tool call arguments, `src/types/agent-server/core/events/action-event.ts:43-49`), yet the configured policy (`threshold: "HIGH"`, `confirm_unknown: true`) is evaluated by the agent-server, which pauses execution into `WAITING_FOR_CONFIRMATION`. On the client side, intent is effectively frozen while awaiting approval — the chat composer is disabled (`src/components/features/chat/interactive-chat-box.tsx:62-65`) and rejection sends `accept:false`. Whether the server truly vetoes despite a model claiming `LOW` risk cannot be verified from this repository (no enforcement code present); the contract implies it does.

**Dimension focus question — can approval be granted narrowly rather than globally?**
Only along one axis: risk-threshold narrowing (`ConfirmRisky` with `HIGH` threshold vs. blanket `AlwaysConfirm`, `src/api/agent-server-adapter.ts:596-604`). There is no evidence of per-tool, per-command-pattern, per-path, or time-boxed grants anywhere in this source — the settings surface exposes exactly two knobs (`confirmation_mode`, `security_analyzer`; `src/mocks/settings-handlers.ts:407-441`). Narrow grants would have to be implemented in the agent-server/SDK repos.

## Architectural Decisions

- **Client-side policy compilation.** Flat user-facing booleans/strings are compiled into SDK discriminator-style policy objects (`{ kind: "NeverConfirm" | "AlwaysConfirm" | "ConfirmRisky", ... }`) at conversation-start time (`src/api/agent-server-adapter.ts:593-618`). This keeps the settings UI dumb and pushes vocabulary alignment with the Python SDK into one adapter function — but means both sides must agree on kind names/threshold strings with no shared schema validation visible here.
- **Canonical home migration for verification fields.** `confirmation_mode`/`security_analyzer` moved from `agent_settings.verification` to top-level `conversation_settings`; the UI defensively hides legacy copies (`src/routes/verification-settings.tsx:4-12`) and a regression test pins the dedup behavior (`__tests__/routes/verification-settings.test.tsx:151-229`).
- **Dual-backend approval transport.** The same logical endpoint is reached two ways: typed `ConversationClient` locally vs. cloud-proxy POST with `hostOverride` to the per-conversation runtime sandbox and session-key auth (`src/api/event-service/event-service.api.ts:18-38,46-69`). Approval traffic therefore works identically against localhost and Cloud sandboxes.
- **Server-defined permissions over hardcoded roles.** The org gate prefers the server's `permissions[]` array and only falls back to role-name checks for older backends (`src/api/cloud/organization-service.api.ts:120-124`, `src/hooks/use-can-manage-org-profiles.ts:51-57`) — an extensible pattern that lets the backend evolve the permission set without frontend releases.
- **Fail-closed UI during unknown permission state.** `useCanManageOrgProfiles` deliberately returns `false` while loading so member users never see mutating controls flash (`src/hooks/use-can-manage-org-profiles.ts:26-28`) — the opposite of the OSS stub, which is fail-open by design (`return true`).
- **Deny-by-default chat input while gated.** Composer disabled during `AWAITING_USER_CONFIRMATION` prevents interleaving new instructions mid-decision (`src/components/features/chat/interactive-chat-box.tsx:62-65`).

## Notable Patterns

- **Layered risk surfacing.** The same `security_risk` value drives three independent renderings: plain-text annotation in card content (`src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:93-98`), colored label in the terminal visualizer (`src/components/features/chat/tool-visualizers/bash/bash.tsx:29-37`), and a full-width red banner above the decision buttons for HIGH (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:111-118`).
- **Status-driven UI instead of event-driven flags.** Rather than trusting a "this event needs confirmation" flag on the event, the whole confirmation UI keys off the conversation's `execution_status` mapped through a pure function (`src/hooks/use-agent-state.ts:10-35`), making the mapping unit-testable (`__tests__/hooks/use-agent-state.test.tsx:26-60`).
- **Schema-driven settings with dependency edges.** Field schemas carry `depends_on` so the analyzer selector only appears once confirmation mode is on (`src/mocks/settings-handlers.ts:437`; asserted in `__tests__/routes/verification-settings.test.tsx:145-148`).
- **Optimistic duplicate-suppression store.** A tiny Zustand store of submitted event IDs guards against React re-renders/double-clicks firing the acceptance mutation twice (`src/stores/event-message-store.ts:9-20`).
- **Keyboard-first approval.** ⌘↩ / ⇧⌘⌫ shortcuts mirror the button pair, registered/cleaned up in an effect scoped to the pending state (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:60-90`).

## Tradeoffs

- **Config/deploy coupling over enforcement control.** Compiling policy client-side keeps the frontend simple but means a frontend bug or stale build silently changes safety semantics (e.g., shipping `NeverConfirm` when the user believes they enabled confirmation). No client-side integrity check ties the sent policy back to displayed settings.
- **Coarse fallback policy.** Any analyzer value other than `"llm"` collapses to `AlwaysConfirm` (`src/api/agent-server-adapter.ts:604`) — safe, but it makes the more surgical `pattern`/`policy_rail` analyzers unusable in combination with risk-threshold confirmation; users must choose full-friction confirmation or none.
- **Binary approvals.** `accept: boolean` with an unused optional `reason` (`src/api/event-service/event-service.types.ts:1-4`) captures no nuance (no "edit before approve", no partial grants). The `reason` field being defined-but-unwired suggests an intended richer flow that never landed.
- **Fail-open OSS posture.** Hard-granting all permissions in OSS (`src/hooks/use-has-permission.ts:4-7`) is pragmatic for a single-user desktop app but means any future consumer of `useHasPermission("...")` inherits a vacuous guarantee unless they know the stub's context.
- **Mock/schema drift.** The mock conversation-schema offers only `llm` and `none` analyzer choices (`src/mocks/settings-handlers.ts:433-436`) while the adapter also maps `pattern` and `policy_rail` (`src/api/agent-server-adapter.ts:611-614`) — mocked E2E coverage cannot exercise the analyzers the code supports.

## Failure Modes / Edge Cases

- **Loose pending-action inference.** `awaitingAction` is computed as the last agent-sourced event whenever the state is `AWAITING_USER_CONFIRMATION`; the `.find()` predicate ignores the event entirely (`if (ev.source !== "agent") return false; return curAgentState === ...`, `src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`). If status and event stream ever disagree (e.g., a late observation arrives after the pause), the UI could attach accept/reject to the wrong event and display the wrong risk level.
- **Reload loses the duplicate-submission guard.** `submittedEventIds` lives only in memory (`src/stores/event-message-store.ts`); refreshing the tab while a decision is pending re-enables the buttons for an already-answered event. Whether the server tolerates a second response is unverifiable here.
- **Stale-status fallback race.** `useAgentState` prefers the live websocket status but falls back to the cached conversation record (`src/hooks/use-agent-state.ts:45-52`); if the cache says `WAITING_FOR_CONFIRMATION` after the user already answered via another tab, stale buttons can render (mitigated only by the in-memory submitted-ID list).
- **Single-severity banner.** `RiskAlert` renders only for `severity === "high"` and explicitly returns `null` otherwise (`src/components/shared/risk-alert.tsx:19-35`); a future MEDIUM-severity caller gets silent no-render rather than a degraded banner.
- **Unknown-risk asymmetry between modes.** `confirm_unknown: true` is applied only on the `llm` path (`src/api/agent-server-adapter.ts:600-601`); with `AlwaysConfirm`, UNKNOWN-risk actions are confirmed like everything else, and with `NeverConfirm` nothing stops them — the safety envelope for UNKNOWN risk depends entirely on which analyzer string is set.
- **Permission-check flash-out handled, error state less so.** If `GET /api/organizations/{id}/me` fails on Cloud, `useCanManageOrgProfiles` yields `false` (`retry: false`, `src/hooks/use-can-manage-org-profiles.ts:44-48`) — owners get locked out of profile editing during transient API errors with no retry affordance beyond query invalidation.

## Future Considerations

- **Narrow, scoped grants.** Extend `confirmation_policy` plumbing toward per-tool or risk-band allowlists (e.g., "auto-confirm LOW file edits, confirm HIGH bash"), leveraging the existing kind/threshold object shape (`src/api/agent-server-adapter.ts:593-605`) without changing the settings UI contract drastically.
- **Wire the `reason` field.** Surface an optional rejection rationale input feeding `ConfirmationResponseRequest.reason` (`src/api/event-service/event-service.types.ts:1-4`) so rejections become actionable context for the agent loop.
- **Persistent/idempotent decision record.** Move the submitted-ID guard server-side or into durable storage so answered confirmations survive reloads and multi-tab use.
- **Close the mock gap.** Add `pattern`/`policy_rail` choices to the mocked schema (`src/mocks/settings-handlers.ts:433-436`) and an E2E trajectory exercising a real confirm/reject round-trip through `respond_to_confirmation`, which currently has zero direct test coverage.
- **Unify the two permission systems.** The OSS stub (`use-has-permission.ts`) and the Cloud permission-set reader (`use-can-manage-org-profiles.ts`) could share one interface backed by a capability provider, so automations UI (`manage_automations` checks at `src/routes/automation-git-sync.tsx:56`) gains real enforcement when a non-OSS automation backend appears.

## Questions / Gaps

- **Where is the actual veto enforced?** The code that pauses the loop on `ConfirmRisky`/`AlwaysConfirm`, evaluates `security_risk` against thresholds, and rejects further tool calls lives in `OpenHands/software-agent-sdk` (per `AGENTS.md` Repository Map), which is outside this source's isolation boundary. All enforcement claims here are inferred from the wire contract (`src/api/agent-server-adapter.ts:1120-1121`) and status enums (`src/types/agent-server/core/base/common.ts:71`).
- **No approval audit trail found.** Searches for `revoke`, approval history, or decision-log surfaces found only telemetry-consent revocation (`src/services/telemetry.ts`) and `URL.revokeObjectURL` noise; no evidence that who-approved-what-and-when is persisted or displayable.
- **Are repeat responses idempotent server-side?** Unverifiable from this repo; the client guard is memory-only (see Failure Modes).
- **Does `USER_CONFIRMED`/`USER_REJECTED` (`src/types/agent-state.tsx:13-14`) ever drive UI?** No consumers were found outside the enum definition; they appear to be vestigial states superseded by `ExecutionStatus` polling.
- **Is there a per-conversation override of the global confirmation setting?** The start-request derives policy exclusively from persisted global `conversation_settings` (`src/api/agent-server-adapter.ts:1120-1121`); no per-launch toggle was found in the conversation-creation UI paths searched (`src/hooks/mutation/use-create-conversation.ts`, `use-unified-start-conversation.ts`). Searched terms: `confirm`, `permission`, `approval`, `security_analyzer`, `AWAITING_USER_CONFIRMATION` across `src/`, `__tests__/`, `tests/`, `docs/`, `specs/`.

---

Generated by `08.02-permission-policy-and-approval-gates` against `openhands`.
