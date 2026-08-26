# Source Analysis: openhands

## Dimension 09.01: Policy Injection Points

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (agent-canvas frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Vite, React Router 7, TanStack Query; consumes the Python `software-agent-sdk` agent-server via `@openhands/typescript-client` |
| Analyzed | 2026-08-26 |

All citations below are relative to the source root above (`studies/agent-harness-study/sources/openhands/`).

## Summary

This repository is only the frontend of a multi-repo system (see `AGENTS.md`, "Repository Map"), so governance rules enter it through four distinct injection points rather than a single policy engine:

1. **User-configurable verification settings** — `confirmation_mode` and `security_analyzer` are typed settings fields (`src/types/settings.ts:128-129`) with defaults in `src/services/settings.ts:13-14` (`confirmation_mode: false`, `security_analyzer: "llm"`), editable at runtime through schema-driven settings pages whose field definitions come from the backend (`/api/settings/agent-schema` / `/api/settings/conversation-schema`).
2. **A client-side policy translation layer** — `getConversationConfirmationPolicy()` (`src/api/agent-server-adapter.ts:593-605`) and `getConversationSecurityAnalyzer()` (`src/api/agent-server-adapter.ts:607-618`) compile those settings into wire-level policy objects (`NeverConfirm` / `ConfirmRisky{threshold:HIGH}` / `AlwaysConfirm`; `LLMSecurityAnalyzer` / `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer`) that are stamped onto every conversation-start payload (`src/api/agent-server-adapter.ts:1120-1121,1169-1173`). Enforcement itself lives in the agent-server; the frontend is the injector.
3. **Human-in-the-loop enforcement UI** — when the server pauses an action (`ExecutionStatus.WAITING_FOR_CONFIRMATION`, `src/types/agent-server/core/base/common.ts:71`), the UI surfaces per-action LLM-predicted `security_risk` (`src/types/agent-server/core/events/action-event.ts:46,61`) with a high-risk alert and accept/reject controls that POST to `/events/respond_to_confirmation` (`src/api/event-service/event-service.api.ts:40-69`).
4. **Manifest admission policies** — extension-authored automation manifests are treated as untrusted data and admitted only after strict host-side validation (`src/manifests/manifest-validation.ts:1-17`, `src/manifests/interface-validation.ts:1-9`), fail-closed on unknown versions.

Policies **can** be changed without code changes for values (schema-driven fields, PATCH-diff persistence), but **not** for new rule kinds: the analyzer-name→kind mapping is a hardcoded switch in adapter code, and parity with the server's policy model is maintained by convention plus one named test. There is no audit trail or durable versioning of policy changes anywhere in this repo; rejections of manifests go to `console.warn` only.

## Rating

**6 / 10** — A clear, explicit model exists (typed settings → tested translation layer → server-side enforcement → human confirmation loop) and manifest admission is an unusually well-reasoned trust boundary with fail-closed versioning and dedicated tests. It falls short of 7–8 because: (a) policy *changes* are unaudited plain settings writes with no history; (b) cross-repo parity of policy semantics ("derives confirmation and security settings the same way as OpenHands", `__tests__/api/agent-server-adapter.test.ts:349`) is enforced only by a test name, not a shared contract; (c) inconsistencies exist between the mapped analyzers (`pattern`, `policy_rail`) and what any schema/mock exposes (`llm`/`none` only, `src/mocks/settings-handlers.ts:433-436`); and (d) a legacy mock-only options endpoint (`/api/options/security-analyzers`, `src/mocks/settings-handlers.ts:804`) has no production consumer.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Policy definition (settings types) | `confirmation_mode: boolean` and `security_analyzer: string \| null` are first-class `Settings` fields | src/types/settings.ts:128-129 |
| Policy defaults | `DEFAULT_SETTINGS`: `confirmation_mode: false`, `security_analyzer: "llm"`; nested `conversation_settings` mirror them with `schema_version: 1` | src/services/settings.ts:13-14,54-58 |
| Confirmation policy translation | `getConversationConfirmationPolicy()` returns `{kind:"NeverConfirm"}`, `{kind:"ConfirmRisky", threshold:"HIGH", confirm_unknown:true}`, or `{kind:"AlwaysConfirm"}` from two settings fields | src/api/agent-server-adapter.ts:593-605 |
| Security-analyzer translation | switch maps `"llm"`→`LLMSecurityAnalyzer`, `"pattern"`→`PatternSecurityAnalyzer`, `"policy_rail"`→`PolicyRailSecurityAnalyzer`; unknown/none → `undefined` (field omitted) | src/api/agent-server-adapter.ts:607-618 |
| Policy injection into runtime payload | start-conversation payload carries `confirmation_policy` (always) and optional `security_analyzer` | src/api/agent-server-adapter.ts:1008-1009,1120-1121,1169-1173 |
| Runtime updateability | PATCH diffs `conversation_settings_diff` / `agent_settings_diff` deep-merge into persisted settings | src/hooks/mutation/use-save-settings.ts:21-45; src/mocks/settings-handlers.ts:1050-1105 |
| Schema-driven policy UI | backend-supplied schemas render verification fields; visibility gated by `depends_on` (`isSettingsFieldVisible` requires dependency === true) | src/utils/sdk-settings-schema.ts:313-318,456-474 |
| Precedence: conversation vs legacy agent fields | Verification screen renders only conversation-owned copies and excludes deprecated `verification.confirmation_mode` / `verification.security_analyzer` from `agent_settings` | src/routes/verification-settings.tsx:4-12,24-36 |
| Precedence: server vs defaults | `normalizeSettingsResponse`: server value `?? DEFAULT_SETTINGS` fallback per field | src/hooks/query/use-settings.ts:89-94 |
| Derived-field sync precedence | `syncDerivedSettings` overlays conversation settings onto cloned defaults (conversation wins) | src/api/settings-service/settings-service.api.ts:358-411 |
| Per-action risk signal | `ActionEvent.security_risk` "predicted by LLM when LLM risk analyzer is enabled"; enum UNKNOWN/LOW/MEDIUM/HIGH; `WAITING_FOR_CONFIRMATION` status | src/types/agent-server/core/events/action-event.ts:46,61; src/types/agent-server/core/base/common.ts:59-75 |
| Human confirmation gate UI | high-risk alert + reject/confirm buttons; duplicate-submission guard via `submittedEventIds`; keyboard shortcuts | src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-58,93-118 |
| Confirmation transport | POST `/api/conversations/{id}/events/respond_to_confirmation` via cloud proxy or typed client | src/api/event-service/event-service.api.ts:29-30,48-68 |
| Risk surfaced in transcript | risk text appended for MEDIUM/HIGH events; bash visualizer shows HIGH/MEDIUM badge | src/components/conversation-events/chat/event-content-helpers/get-action-content.ts:30-38,94-97; src/components/features/chat/tool-visualizers/bash/bash.tsx:24-33 |
| Manifest admission policy (setup) | header declares validation "a trust boundary, not a convenience check"; rejects whole manifest rather than rendering partial UI | src/manifests/manifest-validation.ts:1-17 |
| Fail-closed manifest versioning | `validateSetupEntry` checks `setup.version !== SETUP_VERSION` first and refuses; semver pattern for template/bundle versions | src/manifests/manifest-validation.ts:37-38,570-592; src/manifests/types.ts:13 |
| Manifest sandboxing rules | markup ban `/<[A-Za-z/!]/`; command/path segment checks refuse traversal and absolute entrypoints | src/manifests/manifest-validation.ts:27-28,148-178 |
| Admission registry behavior | invalid entries dropped loudly (`console.warn`), duplicate ids rejected; no partial rendering | src/manifests/manifest-registry.ts:25-46 |
| Interface-manifest admission & gating | fail-closed `version !== INTERFACE_VERSION` check; rejected manifest ⇒ `hasAutomationInterface() === false`, nav/routes 404 | src/manifests/interface-validation.ts:703-721; src/manifests/automation-interface.ts:65-92 |
| Prompt-based rule injection | `<RUNTIME_SERVICES>` markdown block built and attached as `agent_context.system_message_suffix`, including trust directives ("Trust this block over guessing") | src/api/agent-server-adapter.ts:215-300,784-786 |
| Skill deny-list policy | `disabled_skills` filters bundled skills out of `agent_context.skills` at conversation build time | src/api/agent-server-adapter.ts:763-767; src/routes/skills-settings.tsx:87-95 |
| Client-side tool gating | `task_tool_set` attached only when `enable_sub_agents === true`; browser tools behind env+server capability flags | src/api/agent-server-adapter.ts:631-659 |
| Parity test for policy derivation | test name asserts derivation "the same way as OpenHands": expects `ConfirmRisky/HIGH/confirm_unknown` + `LLMSecurityAnalyzer` | __tests__/api/agent-server-adapter.test.ts:349-376 |
| Settings shape migration marker | `LATEST_SETTINGS_VERSION = 5`; agent_settings `schema_version: 6` | src/services/settings.ts:3,36 |
| Profile revision (server-side bumping) | profile saves strip id/name/revision; "server … bumps the revision itself"; conversations record `LaunchedAgentProfile.revision` | src/components/features/settings/agent-profiles/merge-agent-profile-save-input.ts:16-20; src/api/conversation-service/agent-server-conversation-service.types.ts:129-132 |
| Mock-only analyzer options endpoint | GET `*/api/options/security-analyzers` exists only in MSW mocks, choices `["llm","none"]`; no `src/` consumer found | src/mocks/settings-handlers.ts:804,433-436 |

## Answers to Dimension Questions

### 1. Where do governance rules live?

Four places, none a standalone policy engine:

- **Backend-owned settings schema + persisted values**: the canonical policy knobs (`confirmation_mode`, `security_analyzer`) live in server-persisted `conversation_settings` (with a deprecated back-compat copy under `agent_settings.verification` that the UI deliberately hides — `src/routes/verification-settings.tsx:4-12`). Field definitions (labels, choices, `depends_on`, prominence) arrive from `/api/settings/agent-schema` / `conversation-schema` and drive generic rendering (`src/utils/sdk-settings-schema.ts:244-264,456-474`). The mock shows the expected shape: a `verification` section with `confirmation_mode` (boolean, major) and `security_analyzer` (string choice, `depends_on: ["confirmation_mode"]`) (`src/mocks/settings-handlers.ts:403-443`).
- **Frontend translation code**: the mapping from settings to SDK policy objects is code in `src/api/agent-server-adapter.ts:593-618`.
- **Prompt content**: behavioral rules injected as `AgentContext.system_message_suffix` (the `<RUNTIME_SERVICES>` block, `src/api/agent-server-adapter.ts:281-297`) and skill content bundled into `agent_context.skills` (`src/api/agent-server-adapter.ts:771-783`).
- **Host-side admission validators**: regex/invariant policies over extension-authored manifests (`src/manifests/manifest-validation.ts:25-62`, `src/manifests/interface-validation.ts:28-74`).

Actual enforcement (pausing actions, evaluating risk) is server-side; this repo only injects policy parameters and renders the resulting human decision point.

### 2. Can policies be updated at runtime?

Yes for values, no for kinds.

- Values: the settings pages PATCH deep-merge diffs (`conversation_settings_diff`, `agent_settings_diff`, `misc_settings_diff` — `src/hooks/mutation/use-save-settings.ts:21-45`; merge semantics documented in `src/api/settings-service/settings-service.api.ts:70-80`). Because every new conversation rebuilds its payload from current settings (`buildStartConversationRequest`, `src/api/agent-server-adapter.ts:1050-1132`), flipping `confirmation_mode` takes effect on the next conversation without redeploying anything.
- Schema additions (e.g., a new analyzer choice) also need no frontend change if they reuse existing keys, since the UI is schema-driven.
- New rule kinds require code: `getConversationSecurityAnalyzer`'s switch (`src/api/agent-server-adapter.ts:608-617`) must learn each new analyzer name, and `getConversationConfirmationPolicy` hardcodes which analyzer earns threshold-based confirming (`src/api/agent-server-adapter.ts:600-604`). Notably, `pattern` and `policy_rail` are mapped here but exposed by no settings surface in this repo (mock choices are only `llm`/`none`, `src/mocks/settings-handlers.ts:433-436`).

### 3. What happens when policies conflict?

There is no general conflict-resolution engine; instead, targeted precedence rules:

- **Conversation vs deprecated agent copy**: the verification screen filters out `agent_settings` duplicates so only the conversation-owned values render/edit (`src/routes/verification-settings.tsx:9-12,31-35`). Resolution is by exclusion, not comparison.
- **Server value vs default**: `pickFirstBoolean(...) ?? DEFAULT_SETTINGS.confirmation_mode` (`src/hooks/query/use-settings.ts:89-94`) — server wins, defaults fill gaps; `syncDerivedSettings` clones defaults then overlays response values (`src/api/settings-service/settings-service.api.ts:368-377`).
- **Analyzer setting vs confirmation mode**: the confirmation policy collapses to `AlwaysConfirm` whenever mode is on and the analyzer is not exactly `"llm"` (`src/api/agent-server-adapter.ts:600-604`) — i.e., the confirmation-mode boolean outranks analyzer sophistication; a `pattern`/`policy_rail` analyzer still yields confirm-everything.
- **Published manifest data vs host rules**: host validation always wins — "it deliberately does not defer to a schema shipped alongside the manifests it would be validating" (`src/manifests/manifest-validation.ts:6-8`); failing entries are dropped wholesale (`src/manifests/manifest-registry.ts:30-42`), never partially trusted.
- **Client tool gating vs server advertisement**: even if `/server_info` advertises `task_tool_set`, the client strips it unless the local setting allows (`shouldIncludeTool`, `src/api/agent-server-adapter.ts:631-644`).

### 4. Are policy changes audited?

No evidence found of policy-change auditing. Searches across `src/` for audit/version-history mechanisms found: (a) unrelated "audit rounds" concepts in goal-interceptor code (`src/hooks/chat/use-goal-interceptor.ts:11`, `src/types/agent-server/core/events/conversation-state-event.ts:89`); (b) settings-shape migration constants (`LATEST_SETTINGS_VERSION = 5`, `src/services/settings.ts:3`; `schema_version` markers, `src/services/settings.ts:36,55`); (c) server-bumped profile `revision` numbers carried on launches (`src/api/conversation-service/agent-server-conversation-service.types.ts:129-132`) — provenance/versioning for profiles, not an audit trail; and (d) manifest template semver strings validated at admission (`src/manifests/manifest-validation.ts:602-608`). Manifest rejections are logged only to `console.warn` (`src/manifests/manifest-registry.ts:32`, `src/manifests/automation-interface.ts:75-79`) — observable in devtools, not durable or queryable. Who changed `confirmation_mode`, when, and from what value is not recorded anywhere in this repo.

## Architectural Decisions

1. **Inject, don't enforce.** The frontend compiles user intent into policy objects and sends them once per conversation (`confirmation_policy` always present; `security_analyzer` omitted when unmapped — `src/api/agent-server-adapter.ts:1120-1121,1169-1173`). This keeps a single enforcement authority (the agent-server) but means policy semantics are duplicated in two repos, bridged only by the parity test at `__tests__/api/agent-server-adapter.test.ts:349-376`.
2. **Schema-driven configuration surface.** Rather than hand-built policy forms, the backend publishes field schemas and the frontend renders generically with `depends_on` gating and prominence tiers (`src/utils/sdk-settings-schema.ts:33-37,313-318`). Adding or renaming a policy knob is mostly a backend change.
3. **Fail-closed admission for third-party instruction data.** Both manifest validators check version before anything else and refuse unrecognized formats outright (`src/manifests/manifest-validation.ts:587-592`; `src/manifests/interface-validation.ts:716-721`), reject all-or-nothing, and gate entire UI surfaces behind admission success (`hasAutomationInterface()`, `src/manifests/automation-interface.ts:84-92`).
4. **Deprecated-source exclusion instead of migration code.** Legacy `agent_settings.verification.*` copies are hidden from editing while remaining tolerated on the wire (`src/routes/verification-settings.tsx:4-12`).
5. **Human confirmation as a UI-rendered protocol step**, with deduplication of submissions (`submittedEventIds`, `src/components/shared/buttons/conversation-confirmation-buttons.tsx:44-47`) and cloud/local transport branching inside the service (`src/api/event-service/event-service.api.ts:46-68`).

## Notable Patterns

- **Two-key policy derivation**: a single boolean (`confirmation_mode`) selects among three policy shapes, and the analyzer string refines one branch into threshold-based confirmation (`src/api/agent-server-adapter.ts:596-604`) — a compact, easily-tested decision table.
- **Trust-boundary comments as specification**: the admission modules open with prose stating *why* validation exists and what invariant each check protects (e.g., path-segment checks defeating `..` traversal despite allowed characters, `src/manifests/manifest-validation.ts:141-178`) — documentation tied line-for-line to implementation.
- **Closed-vocabulary allowlists everywhere**: trigger kinds, field types, icon slugs, endpoint names, git providers are fixed unions checked with `isOneOf` (`src/manifests/manifest-validation.ts:51-62`; `src/manifests/interface-validation.ts:41-74`), so manifests cannot invent capabilities the host lacks.
- **Prompt-as-policy with self-authenticating scope**: the `<RUNTIME_SERVICES>` suffix anchors URLs to actual topology and explicitly ranks itself over agent guessing (`src/api/agent-server-adapter.ts:281-297`).
- **Deny-list composition at build time**: bundled public skills merged then filtered by the user's `disabled_skills` set before entering `agent_context` (`src/api/agent-server-adapter.ts:758-767`).

## Tradeoffs

- **Duplication vs autonomy**: translating policies client-side keeps the server generic, but every semantic change (e.g., a new analyzer kind or a different risk threshold) needs synchronized releases across frontend, typescript-client, and SDK; the repo pins the client to released npm versions precisely to manage this (`AGENTS.md`, typescript-client note).
- **Schema-driven flexibility vs dead mappings**: because choices come from the server, the frontend carries mappings (`pattern`, `policy_rail`) that no current surface offers — harmless today, but silently untestable end-to-end and easy to rot.
- **Fail-closed manifests vs availability**: one bad field in a published manifest removes the whole automation surface (nav entries vanish, routes 404 — `src/manifests/automation-interface.ts:8-11,72-80`). Safe, but a single upstream typo becomes an outage for the feature.
- **Omission-as-default**: unknown analyzers cause the `security_analyzer` field to be dropped entirely (`src/api/agent-server-adapter.ts:615-616`), delegating to whatever the server default is — clean, but invisible in the payload and easy to misdiagnose.
- **Console-only rejection reporting**: cheap and simple, but provides zero operational observability for admission decisions (`src/manifests/manifest-registry.ts:32`).

## Failure Modes / Edge Cases

- **Stale policy on running conversations**: changes apply at next conversation start; nothing in the frontend updates the policy of a live conversation (payload is built once in `buildStartConversationRequest`, `src/api/agent-server-adapter.ts:1050-1132`).
- **Silent downgrade to AlwaysConfirm**: enabling confirmation with any non-`llm` analyzer yields confirm-on-every-action (`src/api/agent-server-adapter.ts:600-604`); a user selecting a lighter analyzer gets a stricter experience with no explanation.
- **Awaiting-action detection is state-only**: `awaitingAction` picks the last agent event purely because `curAgentState === AWAITING_USER_CONFIRMATION`, not because the event is the pending one (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:30-36`); a non-action last event falls back to `SecurityRisk.UNKNOWN` (lines 102-105), losing the risk badge.
- **Duplicate-submission guard is client-memory only**: `submittedEventIds` lives in a Zustand store; a refresh during `waiting_for_confirmation` clears it, allowing re-submission of the same accept/reject (`src/components/shared/buttons/conversation-confirmation-buttons.tsx:17-22,96-97`).
- **Schema-absent degradation**: malformed/missing schema responses are guarded so pages don't crash (`isValidSettingsSchema`, `src/utils/sdk-settings-schema.ts:39-55`), but with no schema there is no policy UI at all — recovery depends entirely on backend health.
- **Version skew**: older pinned packages predating interface exports yield `undefined` candidates handled gracefully (`src/manifests/manifest-sources.ts:22-32`), but a manifest published with a future `SETUP_VERSION`/`INTERFACE_VERSION` is refused outright by design (`src/manifests/manifest-validation.ts:587-592`).

## Future Considerations

- Add a shared contract (generated types or conformance fixtures from the SDK) so policy-kind parity is machine-checked rather than asserted by a test name (`__tests__/api/agent-server-adapter.test.ts:349`).
- Surface analyzer choices from a real backend options endpoint (the mock's `/api/options/security-analyzers`, `src/mocks/settings-handlers.ts:804`, has no consumer) and reconcile it with the adapter switch so mapped-but-unexposed kinds (`pattern`, `policy_rail`) become reachable or removed.
- Record policy-change history server-side (who/when/from→to for `confirmation_mode`/`security_analyzer`) if governance ever becomes multi-user; the existing profile `revision` mechanism (`merge-agent-profile-save-input.ts:16-20`) is a natural pattern to extend.
- Make the pending-confirmation event identity server-authoritative (e.g., echo the awaited action id in the state event) instead of inferring "last agent event" in the UI (`conversation-confirmation-buttons.tsx:30-36`).

## Questions / Gaps

- **Unanswerable in this source**: how the agent-server interprets `ConfirmRisky.threshold`, `confirm_unknown`, and `PatternSecurityAnalyzer`/`PolicyRailSecurityAnalyzer` — enforcement lives in `software-agent-sdk`, outside this study's isolation boundary. Only the payload contract is visible (`src/api/agent-server-adapter.ts:593-618`).
- **No evidence found** of any audit log, change history, or diff view for policy settings; searched `audit`, `revision`, `history` patterns across `src/`.
- **No evidence found** of role-based or org-level policy restrictions: settings scope is only `"personal"` (`src/types/settings.ts:95`); org-profile management permission checks exist (`src/hooks/use-can-manage-org-profiles.ts:34`) but govern profile CRUD, not security policy.
- Whether real backends expose more than `llm`/`none` analyzer choices could not be verified — the only in-repo schema sample is the mock (`src/mocks/settings-handlers.ts:433-436`); the live answer depends on the deployed SDK version.

---

Generated by `Dimension 09.01: Policy Injection Points` against `openhands`.
