# Source Analysis: openhands

## Dimension 08.04 — Security Auditability

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (React Router, Zustand, TanStack Query), Vite; frontend-only repo of a multi-repo system (agent-server lives in `OpenHands/software-agent-sdk`, per `studies/agent-harness-study/sources/openhands/AGENTS.md` repository map) |
| Analyzed | 2026-08-24 |

## Summary

The OpenHands agent-canvas is the **frontend** of the OpenHands harness; it does not generate its own security audit log. Instead, it consumes a server-owned, durable event stream in which every security-relevant fact is a typed event: each event carries an `id`, ISO `timestamp`, and a `source` discriminator (`agent | user | environment | hook`) (`studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/event.ts:8-27`, `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/common.ts:56`). On top of that stream the frontend implements four auditability-relevant capabilities:

1. **Risk metadata per action.** Every `ActionEvent` persists a `security_risk` value (`UNKNOWN | LOW | MEDIUM | HIGH`, `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/base/common.ts:59-64`; field at `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/action-event.ts:58-61`), described as "the LLM's assessment of the safety risk of this action" predicted "when LLM risk analyzer is enabled" (`studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/action-event.ts:42-49`). The UI surfaces MEDIUM/HIGH risk inline on bash cards (`studies/agent-harness-study/sources/openhands/src/components/features/chat/tool-visualizers/bash/bash.tsx:24-37`) and escalates HIGH to a red banner at confirmation time (`studies/agent-harness-study/sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:107-118`).

2. **Explicit approval policy derivation.** The user-facing `confirmation_mode` / `security_analyzer` settings (`studies/agent-harness-study/sources/openhands/src/types/settings.ts:128-129`, edited on the Verification settings page, `studies/agent-harness-study/sources/openhands/src/routes/verification-settings.tsx:9-12`) are translated into named server-side policies at conversation start: `NeverConfirm`, `ConfirmRisky { threshold: HIGH, confirm_unknown }`, or `AlwaysConfirm` (`studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:593-605`), plus analyzer selection `LLMSecurityAnalyzer | PatternSecurityAnalyzer | PolicyRailSecurityAnalyzer` (`studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:607-616`). This mapping has unit-test coverage asserting parity with upstream OpenHands behavior (`studies/agent-harness-study/sources/openhands/__tests__/api/agent-server-adapter.test.ts:352-376`).

3. **Human-in-the-loop approval flow.** When the agent reports `waiting_for_confirmation`, the UI renders Accept/Reject buttons with keyboard shortcuts and a high-risk warning (`studies/agent-harness-study/sources/openhands/src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-100`); the decision POSTs `{ accept }` to `/api/conversations/{id}/events/respond_to_confirmation` (`studies/agent-harness-study/sources/openhands/src/hooks/mutation/use-respond-to-confirmation.ts:13-29`, `studies/agent-harness-study/sources/openhands/src/api/event-service/event-service.api.ts:40-69`). Rejections come back as a dedicated persisted `UserRejectObservation` carrying `rejection_reason` and the originating `action_id` (`studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/observation-event.ts:42-51`).

4. **Reconstruction/export paths.** The full raw event log can be downloaded as JSON ("trajectory", up to 10,000 events, `studies/agent-harness-study/sources/openhands/src/api/conversation-service/conversation-service.api.ts:58-64`), and a human-readable Markdown/HTML transcript export renders messages, tool calls, outputs, errors, hook executions, and timestamps with XSS sanitization and a locked-down CSP (`studies/agent-harness-study/sources/openhands/src/utils/transcript-export/index.ts:514-581`, CSP at `studies/agent-harness-study/sources/openhands/src/utils/transcript-export/index.ts:642`).

The weakest link for the dimension's four questions is **attribution and decision identity**: events carry only a coarse `source` role, never a user identity or a policy-decision ID; accepted confirmations leave no first-class trace visible in the chat UI or transcript exports; and durable cross-system audit evidence is delegated to an external governance tool (DefenseClaw) whose deeper integration is documented as future work (`studies/agent-harness-study/sources/openhands/docs/DefenseClaw.md:280-282`).

**Scope caveat:** because this source is the UI tier, "absent" findings below mean *absent from the frontend*; the agent-server (different repository) owns persistence and may hold records the frontend simply does not surface.

## Rating

**5 / 10** — Present but inconsistent and partially fragile.

Rationale against the rubric:

- **Why not lower:** There is a clear, typed model for exactly what the dimension asks about — risk levels on every action (`studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/action-event.ts:61`), a persisted rejection record with reason (`studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/observation-event.ts:42-51`), a hook-execution record that captures policy-enforcement outcomes including block reasons (`studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/hook-execution-event.ts:23-77`), explicit named confirmation policies with tests (`studies/agent-harness-study/sources/openhands/__tests__/api/agent-server-adapter.test.ts:352-376`), and two reconstruction paths (raw trajectory JSON + sanitized transcript).
- **Why not higher:** The accept side of human approval is invisible after the fact (no rendering, no export entry — see Failure Modes); `security_risk` is stripped from transcript exports; there is no user attribution and no policy-decision identifier anywhere in the type system; the browser-side event store is memory-only (`studies/agent-harness-study/sources/openhands/src/stores/use-event-store.ts:153-223`); no tests exist for the confirmation submission path itself (searched `__tests__/` and `tests/e2e/` for `respondToConfirmation`/`respond_to_confirmation`: no matches); and the docs concede durable audit evidence requires external tooling whose bridge is unbuilt (`studies/agent-harness-study/sources/openhands/docs/DefenseClaw.md:282`).

## Evidence Collected

Every entry cites workspace-relative paths under `studies/agent-harness-study/sources/openhands/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Event provenance | `BaseEvent` gives every event a unique `id`, ISO `timestamp`, and `source` role | `src/types/agent-server/core/base/event.ts:8-27` |
| Source roles | `SourceType = "agent" \| "user" \| "environment" \| "hook"` | `src/types/agent-server/core/base/common.ts:56` |
| Risk taxonomy | `SecurityRisk` enum: UNKNOWN/LOW/MEDIUM/HIGH | `src/types/agent-server/core/base/common.ts:59-64` |
| Per-action risk record | `ActionEvent.security_risk` — LLM's safety assessment persisted with the action; `tool_call` retains the raw prediction when the risk analyzer runs | `src/types/agent-server/core/events/action-event.ts:42-61` |
| Confirmation gate state | `ExecutionStatus.WAITING_FOR_CONFIRMATION` drives UI gating | `src/types/agent-server/core/base/common.ts:71`; consumed at `src/hooks/use-agent-state.ts:24` |
| Approval settings | `confirmation_mode: boolean` and `security_analyzer: string \| null` on `Settings` | `src/types/settings.ts:128-129` |
| Settings UI | Verification page renders conversation-owned `verification.confirmation_mode` / `verification.security_analyzer`, de-duplicating deprecated agent copies | `src/routes/verification-settings.tsx:4-36` |
| Policy derivation | `getConversationConfirmationPolicy` → `{kind:"NeverConfirm"}` / `{kind:"ConfirmRisky", threshold:"HIGH", confirm_unknown:true}` / `{kind:"AlwaysConfirm"}` sent as `confirmation_policy` in the start-conversation payload | `src/api/agent-server-adapter.ts:593-605`, applied at `src/api/agent-server-adapter.ts:1118-1121` |
| Analyzer derivation | `LLMSecurityAnalyzer` / `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer` mapped from `security_analyzer` setting into payload | `src/api/agent-server-adapter.ts:607-616`, applied at `src/api/agent-server-adapter.ts:1169-1173` |
| Policy tests | Test asserts `ConfirmRisky` + `LLMSecurityAnalyzer` payloads "the same way as OpenHands" | `__tests__/api/agent-server-adapter.test.ts:352-376` |
| Approval UI | Buttons appear only while awaiting confirmation; duplicate-submission guard via `submittedEventIds`; HIGH risk shows `RiskAlert` | `src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-58`, `92-118` |
| Approval submission | Mutation POSTs `{ accept }` through `EventService.respondToConfirmation` → `/api/conversations/{id}/events/respond_to_confirmation` (local via `ConversationClient`, cloud via session-key proxy) | `src/hooks/mutation/use-respond-to-confirmation.ts:13-29`; `src/api/event-service/event-service.api.ts:40-69` |
| Request schema gap | `ConfirmationResponseRequest` allows optional `reason?: string`, but the mutation never sends one | `src/api/event-service/event-service.types.ts:1-4` vs `src/hooks/mutation/use-respond-to-confirmation.ts:19-22` |
| Rejection record | `UserRejectObservation` persists `rejection_reason` + `action_id` (+ `tool_name`, `tool_call_id`) | `src/types/agent-server/core/events/observation-event.ts:42-51` |
| Hook (policy enforcement) records | `HookExecutionEvent` logs `hook_event_type`, `hook_command`, `success`, `blocked`, `exit_code`, blocking `reason`, `tool_name`, `action_id`, stdout/stderr | `src/types/agent-server/core/events/hook-execution-event.ts:23-77` |
| Block visibility | Blocked hooks render an amber "blocked" badge plus the blocking reason in chat | `src/components/shared/hook-execution-event-message.tsx:44-54`, `110-117` |
| Tool execution trail | Action+Observation pairs carry `tool_name`/`tool_call_id`; bash card shows command, output, exit code | `src/types/agent-server/core/events/action-event.ts:31-40`; `src/components/features/chat/tool-visualizers/bash/bash.tsx:20-45` |
| Durable history access | `EventService.searchEvents` paginates server-side history with timestamp filters; cloud history "survives the runtime sandbox" | `src/api/event-service/event-service.api.ts:18-37`, `102-181` |
| Raw-log export | Trajectory download fetches up to 10k raw events | `src/api/conversation-service/conversation-service.api.ts:58-64`; menu item at `src/components/features/conversation-panel/conversation-card/conversation-card-context-menu.tsx:186-199` |
| Human-readable export | Transcript export emits timestamps, tool summaries/details, errors, hook entries; detail kinds whitelisted; markdown sanitized, HTML carries strict CSP | `src/utils/transcript-export/index.ts:72-110`, `138-150`, `456-466`, `642` |
| Client-side volatility | Browser event store is Zustand in-memory; cleared on conversation switch; no localStorage persistence | `src/stores/use-event-store.ts:66-89`, `153-223` |
| Analytics boundary | PostHog telemetry is consent-gated and tracks funnel events only (e.g. `conversation_exported`, `download_trajectory_button_clicked`), never conversation content | `src/services/telemetry.ts:1-30`; `src/hooks/use-tracking.ts:273-279` |
| External audit layer | DefenseClaw integration doc: durable audit evidence comes from its SQLite audit store; Agent Server→audit bridge listed under "future work" | `docs/DefenseClaw.md:209-211`, `280-282` |
| Stated posture | Security posture section recommends Docker sandboxing/hardening; no audit-logging claims made | `docs/architecture.md:80-83` |

## Answers to Dimension Questions

**1. Who did what?**
Partially answerable. *What* is well covered: every action, observation, message, error, pause, and hook execution is an individually identified, timestamped event (`src/types/agent-server/core/base/event.ts:8-27`), and tool invocations are bound to their results via `action_id`/`tool_call_id` pairs (`src/types/agent-server/core/events/observation-event.ts:17-39`). *Who* is weak: attribution is limited to the four-value `source` role (`src/types/agent-server/core/base/common.ts:56`); no user ID, session ID, or actor identity exists on any event type. The system implicitly assumes a single local principal; Cloud user identity exists only in the consent-gated analytics layer (`AGENTS.md` tracking architecture; `src/services/telemetry-context.ts`), not in the conversation record.

**2. What policy allowed it?**
Only inferable, not recorded per decision. The effective policy is derivable from what was *sent* at conversation start — `confirmation_policy` and `security_analyzer` objects (`src/api/agent-server-adapter.ts:1118-1121`, `1169-1173`) — and enforcement outcomes are observable via `UserRejectObservation` (`rejection_reason`) and blocked `HookExecutionEvent`s (`reason`). But there is no decision identifier linking an executed action to the policy verdict that admitted it, and the frontend never re-reads the active policy to annotate the stream. An auditor must reconstruct "what policy was in force" from conversation-start payloads alone.

**3. Was a human involved?**
Detectable in some paths. Awaiting state is explicit (`WAITING_FOR_CONFIRMATION`, rendered by `src/components/shared/buttons/conversation-confirmation-buttons.tsx:93-100`); pauses are attributed `source: "user"` (`src/types/agent-server/core/events/pause-event.ts:5-8`); and rejections persist a structured record (`UserRejectObservation`, `src/types/agent-server/core/events/observation-event.ts:42-51`). However, an **acceptance** produces no equivalent first-class artifact in the types the frontend knows: after `{accept:true}` the action simply executes, indistinguishable from an auto-approved run unless the auditor knows `confirmation_mode` was enabled and notices the temporal pattern. The optional `reason` field on acceptance requests is never populated (`src/hooks/mutation/use-respond-to-confirmation.ts:19-22`).

**4. Can auditors reconstruct the decision?**
Mostly yes for *actions*, partially for *decisions*. Three channels exist: (a) raw full-fidelity JSON trajectory download (`src/api/conversation-service/conversation-service.api.ts:58-64`); (b) paginated REST event search with timestamp filters against server-persisted history (`src/api/event-service/event-service.api.ts:102-181`); (c) sanitized Markdown/HTML transcript (`src/utils/transcript-export/index.ts:514-678`). Reconstruction gaps: `security_risk` is not emitted into either export format (risk text appears only transiently in chat content helpers, `src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:87-101`); rejected actions are filtered out of the rendered stream entirely (see Failure Modes); and nothing is signed or hash-chained, so exported artifacts have no tamper evidence. Durable cross-system correlation is explicitly deferred to external tooling (`docs/DefenseClaw.md:280-282`).

## Architectural Decisions

1. **Server-owned ledger, thin client.** All durability is delegated to the agent-server/cloud backend; the frontend keeps events only in a memory-resident Zustand store that is deliberately wiped on conversation switch (`src/stores/use-event-store.ts:76-89`, `216-222`). This avoids divergent client-side audit copies (and keeps sensitive content out of localStorage), but makes the browser unable to answer audit questions offline or after history truncation.
   - Tradeoff: privacy/simplicity vs. local forensic capability.

2. **Policy-as-payload at conversation start.** Rather than enforcing confirmation in the UI, the frontend compiles user preferences into declarative policy objects (`NeverConfirm` / `ConfirmRisky` / `AlwaysConfirm`, analyzers) handed to the server (`src/api/agent-server-adapter.ts:593-616`). Enforcement therefore cannot be bypassed by client manipulation, and the sent payload is a reviewable artifact of intent.
   - Tradeoff: strong design, but the payload is write-once — later settings changes require a new conversation, and nothing echoes the active policy back onto subsequent events.

3. **Typed event union as the audit vocabulary.** One discriminated union (`OpenHandsEvent`, `src/types/agent-server/core/openhands-event.ts:11-52`) covers everything auditors might need — including `HookExecutionEvent` and `UserRejectObservation` — so new security-relevant records arrive fully typed and renderable through a single pipeline.

4. **Hooks as the policy-enforcement point, with first-class observability.** `PreToolUse`/`PostToolUse` hook executions are logged as events containing the block verdict, reason, exit code, and command output (`src/types/agent-server/core/events/hook-execution-event.ts:23-77`) — the closest thing in this source to policy decision records, and they are rendered, not hidden (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:119-122`).

5. **Consent-gated, content-free telemetry.** Product analytics are strictly separated from the audit stream: PostHog events carry controlled enums and IDs, never device codes, keys, or conversation content (`AGENTS.md` "OAuth device authorization…" section; `src/hooks/use-tracking.ts:68-80`). This prevents the analytics channel from becoming a shadow audit log with weaker guarantees (or a privacy liability).

## Notable Patterns

- **Risk surfaced at the moment of judgment, not just in logs**: HIGH-risk pending actions get a red `RiskAlert` banner before the human decides (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:107-118`), and MEDIUM/HIGH bash commands are annotated in-line (`src/components/features/chat/tool-visualizers/bash/bash.tsx:29-37`). Auditability here is paired with informed consent.
- **Duplicate-submission guard on approvals**: `submittedEventIds` prevents double-POSTing a confirmation decision (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:16-47`, `93-98`) — small, but it protects the integrity of the approval record.
- **Whitelist-based export hygiene**: transcript details are emitted only for explicitly allow-listed action/observation kinds (`SAFE_ACTION_DETAIL_KINDS` / `SAFE_OBSERVATION_DETAIL_KINDS`, `src/utils/transcript-export/index.ts:72-110`), and all dynamic text passes HTML escaping, javascript:-URI neutralization, and a `default-src 'none'` CSP (`index.ts:126-150`, `642`). Exported audit artifacts are treated as an attack surface.
- **Strict pagination for exporters**: transcript export sets `strictPagination` so a partial history can never be mistaken for a complete one (`src/api/event-service/event-service.types.ts:26-33`) — a subtle but important correctness property for anyone exporting evidence.

## Tradeoffs

- **Frontend completeness vs. system truth.** Several "gaps" in this report may be satisfied server-side (e.g., whether accepts generate an observation event). The frontend's own types and rendering pipeline demonstrably do not surface them, so even if data exists, the human-reviewable layer loses it.
- **Privacy-first non-persistence vs. forensic durability.** Memory-only client store and content-free telemetry minimize leakage, but also mean the browser holds no independent evidence if server retention is short.
- **Human-readable export vs. fidelity.** The Markdown/HTML transcript is sanitized and readable but lossy (no risk levels, no rejections, grouped actions collapsed); the JSON trajectory is faithful but unredacted — two audiences, two tools, neither labeled as "audit".
- **Single-principal simplicity vs. attribution.** Dropping actor identity keeps local use frictionless but makes multi-actor scenarios (shared server, cloud org) unauditable at the event level.

## Failure Modes / Edge Cases

- **Accepted approvals vanish from the reviewable stream.** `shouldRenderEvent` returns `false` for any event kind it does not explicitly handle (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:143-144`), and `UserRejectObservation` matches none of the handled branches (it lacks an `observation` payload, failing `isObservationEvent`, `src/types/agent-server/type-guards.ts:67-74`). Rejections are therefore invisible in chat and absent from transcript exports — an auditor reading the transcript sees risky commands execute with no sign a human vetoed others.
- **No test coverage on the approval submission path.** Searching `__tests__/` and `tests/e2e/` for `respondToConfirmation` / `respond_to_confirmation` returns no matches; the duplicate-guard and accept/reject wiring are untested (only the policy *derivation* is tested).
- **Cloud pagination degradation can truncate history silently**: when the cloud backend lacks filter support, `searchEvents` returns an empty page after a console warning instead of failing (`src/api/event-service/event-service.api.ts:149-163`). Callers that forget `strictPagination` could build incomplete exports; the mitigation exists but is opt-in per caller.
- **Risk labels depend on an optional analyzer.** `security_risk` defaults toward `UNKNOWN` semantics when no analyzer runs; UNKNOWN is rendered as such in content helpers (`get-action-content.ts:38-41`) but treated as non-alerting in the confirmation UI (`conversation-confirmation-buttons.tsx:105-107`), so a misconfigured analyzer quietly downgrades the warning surface. (Mitigating control: `ConfirmRisky` sets `confirm_unknown: true` server-side, `agent-server-adapter.ts:601`.)
- **Goal-loop reprompts are indistinguishable from user input at the data layer**: filtering relies on brittle prompt-text prefix matching because "the persisted event carries no marker distinguishing them from real user input" (`should-render-event.ts:15-26`). For audit purposes this means synthetic agent-generated "user" turns exist in the historical record — a who-did-what hazard acknowledged in-code.

## Future Considerations

- Emit (and render/export) a symmetric record for accepted confirmations, e.g. an `ActionApprovedObservation` mirroring `UserRejectObservation`, so human involvement is reconstructible for both branches.
- Thread `reason` through the acceptance/rejection mutation (`ConfirmationResponseRequest.reason` already supports it, `event-service.types.ts:1-4`) and display it alongside the rejection record.
- Include `security_risk` (and confirmation-policy context) in both export formats, clearly marking exports as audit artifacts; consider hashing/manifests for tamper evidence.
- Add actor identity (session/user ID) to events, or at minimum to approval submissions, to answer "who" beyond the role-level `source`.
- Land the planned Agent Server → external-audit webhook bridge described in `docs/DefenseClaw.md:280-282` to correlate harness decisions with guardrail verdicts in one store.

## Questions / Gaps

- Does the agent-server persist an artifact for *accepted* confirmations (beyond the executed action)? Not answerable from this source — the response type is only `{ success: boolean }` (`event-service.types.ts:6-8`) and the SDK repo is out of scope per isolation rules. Searched: `src/types/`, `src/api/`, `__tests__/`, `tests/e2e/`.
- Is there any tamper-evidence or integrity mechanism on the event stream? No evidence found; searched for `signature`, `hash`, `integrity`, `chain` across `src/` — only Anthropic thinking-block `signature` fields match, which are unrelated to audit integrity (`src/types/agent-server/core/base/event.ts:63-75`).
- Are `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer` selectable in the shipped UI? The adapter supports them (`agent-server-adapter.ts:607-616`), but the mock schema advertises only `["llm","none"]` (`src/mocks/settings-handlers.ts:434-443`, `669`); real backend schema availability could not be verified from this repo.
- Retention, access control, and immutability of the server-side event store are owned by `OpenHands/software-agent-sdk` and were not inspectable here.

---

Generated by `dimension 08.04-security-auditability` against `openhands`.
