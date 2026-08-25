# Source Analysis: openhands

## Dimension 05.07 — Memory Privacy, Scope, and Deletion

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands agent-canvas frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Vite, Zustand, TanStack Query, `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

## Summary

This repository is the OpenHands **frontend** (agent-canvas); the agent's actual memory store lives in the sibling `software-agent-sdk` repo. Within this source, "memory" decomposes into five concrete subsystems, each with a different privacy posture:

1. **Persistent agent memory** (`agent_context.load_memory`) — a single opt-in boolean that tells the server-side agent to keep notes under `.openhands/memory/` and reload them each conversation (`src/mocks/settings-handlers.ts:353-370`, `src/i18n/translation.json:2450-2452`). The frontend exposes only an on/off switch; it has **no UI to view, export, or delete** the accumulated memory.
2. **Skills/microagents** — the closest analog to scoped long-term knowledge. They carry an explicit three-scope model (`project` / `personal` / `public`) derived from path heuristics (`src/utils/skill-scope.ts:3-9`) with unit tests, plus user-controlled disable lists filtered at launch (`src/api/agent-server-adapter.ts:749-767`).
3. **Secrets** — names/descriptions listed without values (`src/api/secrets-service.types.ts:11-15`); values round-trip encrypted or redacted, never plaintext to the browser by default (`src/api/settings-service/settings-service.api.ts:115-122`).
4. **Client-local persistence** — per-conversation UI state (including draft message text) in localStorage (`src/utils/conversation-local-storage.ts:14-17`, `src/utils/conversation-local-storage.ts:44`), backend API keys in `openhands-backends` localStorage (`src/api/backend-registry/storage.ts:13-14`, `src/api/backend-registry/storage.ts:98`), and telemetry identity.
5. **Telemetry identity/consent** — consent-gated capture with an explicit GDPR-style "clear all telemetry data" routine that resets PostHog identity (`src/services/telemetry.ts:830-869`).

The overall model is: strong secret hygiene and explicit scope labeling where the frontend owns data; enforcement of agent-memory privacy is delegated to the agent-server, with the frontend providing only coarse toggles.

## Rating

**6 / 10** — Present but inconsistent.

- Scopes for skills are explicit and tested (`src/utils/skill-scope.ts:79-106`, `__tests__/utils/skill-scope.test.ts:14-51`); org-scoped skills are deliberately excluded from queries (`load_org: false`, `src/api/skills-service.ts:54`). Secrets have full CRUD + delete with confirmation and encrypted round-tripping.
- However, persistent agent memory has **no deletion, inspection, or anonymization surface** in this repo (search boundary: `src/**` grep for `memory` returned only the condenser settings page, the toggle schema, and unrelated error strings). There is no retention policy anywhere client-side (localStorage blobs persist until manual deletion), no audit trail of memory reads/writes, and sensitive material (backend API keys, draft messages) rests unencrypted in localStorage.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Memory scope (skills) | Three-scope model `SkillScope = "project" \| "personal" \| "public"` with ordering constant | src/utils/skill-scope.ts:3-9 |
| Scope classification | `getSkillScope()` maps sources via home-dir regexes and dir markers (`/.agents/skills/`, `/.openhands/skills/`, legacy `/.openhands/microagents/`) | src/utils/skill-scope.ts:15-52, 79-106 |
| Scope tests | Unit tests classify public catalog skills, personal home-dir skills, project workspace skills | __tests__/utils/skill-scope.test.ts:15-49 |
| Tenant filter on skill query | Skills fetched with `load_public: false, load_user: true, load_project: true, load_org: false` | src/api/skills-service.ts:48-56 |
| User disable list enforced at query/build time | `buildAgentContext` filters bundled + existing skills against `disabled_skills` set before sending `agent_context.skills` | src/api/agent-server-adapter.ts:760-767 |
| Persistent memory toggle | Schema section "Memory" exposing only `agent_context.load_memory`, boolean, `default: false` (opt-in) | src/mocks/settings-handlers.ts:349-371 |
| Memory description | "The agent keeps notes under .openhands/memory/ and loads them at the start of each new conversation" (all 15 locales) | src/i18n/translation.json:2450-2465 |
| Memory nav surface | Settings nav item `/settings/agent-context` rendering whatever `agent_context` schema exposes | src/constants/settings-nav.tsx:39-49 |
| Agent-context settings screen | `SdkSectionPage` bound to `agent_settings.agent_context` section keys | src/routes/agent-context-settings.tsx:5-10 |
| Global-memory semantics | Comment: `load_memory` is a global user preference stamped server-side onto profile-resolved agents; client must not re-send | src/api/agent-server-adapter.ts:1100-1106 |
| Secret listing w/o values | `CustomSecretWithoutValue = Omit<CustomSecret, "value">`; list endpoint returns name+description only | src/api/secrets-service.types.ts:11-15, src/api/secrets-service.ts:26-41 |
| Secret exposure modes | `X-Expose-Secrets`: undefined→redacted `"**********"`, `"encrypted"`→cipher text safe for round-trip, `"plaintext"` documented as backend-only | src/api/settings-service/settings-service.api.ts:115-122, 421-438 |
| Encrypted conversation start | `getSettingsForConversation()` fetches encrypted settings; explicitly refuses redacted fallback ("conversations should not start with broken/redacted credentials") | src/api/settings-service/settings-service.api.ts:479-509 |
| MCP credential redaction | Redacted placeholders substituted with stored *encrypted* leaves so plaintext never reaches the browser during connectivity tests | src/api/mcp-service/mcp-redacted-credentials.ts:34-58, 77-98 |
| Error-text secret scrubbing | `redactMcpSecrets` strips known values, Bearer tokens, and generic token patterns (ghp_/github_pat_/xox/lin_api/JWT) from displayed errors | src/utils/redact-mcp-secrets.ts:22-31, 100-127 |
| CUSTOM_SECRETS masking backstop | `redactCustomSecrets` masks any unmasked value inside `<CUSTOM_SECRETS>` blocks defensively | src/utils/redact-custom-secrets.ts:1-27 |
| Delete API (secrets) | `DELETE /api/settings/secrets/{name}` wrapper tolerates 404 as success; cloud branch via `deleteCloudSecret` | src/api/secrets-service.ts:123-155 |
| Delete confirmation UI | Trash action routes through `ConfirmationModal` before `useDeleteSecret` mutation | src/components/features/conversation/conversation-overview-secrets-panel.tsx:122-139 |
| Conversation deletion cleanup | `clearConversationLocalStorage(conversationId)` removes the per-conversation blob when a conversation is deleted | src/hooks/mutation/use-delete-conversation.ts:32, src/utils/conversation-local-storage.ts:311-330 |
| Per-conversation local scope | State keyed `conversation-state-{id}`; task-placeholder ids skipped; sanitize drops legacy/junk fields on read | src/utils/conversation-local-storage.ts:237-249, 106-205 |
| Draft retention | `draftMessage` persisted per conversation (`src/utils/conversation-local-storage.ts:44`); pending-task drafts stored under `pending-task-draft-{taskId}` | src/utils/conversation-local-storage.ts:284-294 |
| Backend/org tenant keying | Last-conversation shortcut keyed `backendId::orgId` in localStorage map | src/api/backend-registry/last-conversation-store.ts:12-19 |
| Sensitive data in localStorage | Backend records incl. `apiKey` serialized into `openhands-backends` key | src/api/backend-registry/storage.ts:13-14, 28-33, 98 |
| Telemetry privacy clear | `clearTelemetryData()`: removes consent+first-use markers, clears cloud context, resets PostHog identity (device id included), forces opt-out on failure | src/services/telemetry.ts:830-869 |
| Consent gate & hard disable | Capture only when consent === "granted"; `configureTelemetry(false)` hard-disables for embedding hosts; DNT respected | src/services/telemetry.ts:148-154, 205-212, 662-670 |
| Identity reset semantics | `resetPostHogIdentity` restores canonical Canvas consent after SDK reset; stale `$user_id` mismatch triggers reset | src/services/telemetry.ts:156-188 |
| Session-key auth on all calls | Client options assemble host + `X-Session-API-Key` from active backend registry for every agent-server call | src/api/agent-server-client-options.ts (used at src/api/secrets-service.ts:18, src/api/skills-service.ts:48) |

## Answers to Dimension Questions

**1. Can memory leak between users?**
No direct cross-user leak path found in the frontend, but scoping is per-*backend/org*, not per-user: skills are fetched without org scope (`load_org: false`, `src/api/skills-service.ts:54`) and UX state is keyed by `(backendId, orgId)` (`src/api/backend-registry/last-conversation-store.ts:12-19`). On a shared machine sharing one browser profile and one local agent-server session key, all users see the same memory/skills/secrets — there is no per-OS-user partition beyond browser storage isolation. The persistent memory itself is scoped server-side (out of this repo); the frontend sends a single global `load_memory` flag (`src/api/agent-server-adapter.ts:1100-1106`), which suggests one memory pool per agent-server installation rather than per conversation or per project. No evidence found of conversation-level or workspace-level memory partitioning controls in this codebase.

**2. Can users delete memory?**
Partially, depending on subsystem:
- Secrets: yes — full delete API with 404-tolerant retry (`src/api/secrets-service.ts:130-155`) and confirmation modal (`src/components/features/conversation/conversation-overview-secrets-panel.tsx:122-139`).
- Conversation-local state: yes — deleting a conversation clears its localStorage blob (`src/hooks/mutation/use-delete-conversation.ts:32`).
- Telemetry identity: yes — GDPR-style clear that resets PostHog identity and markers (`src/services/telemetry.ts:830-869`).
- Persistent agent memory (`.openhands/memory/`): **no deletion, viewing, or reset UI exists in this repo**. The only control is the opt-in toggle (`src/routes/agent-context-settings.tsx:5-10`); disabling stops future loading but nothing here deletes accumulated notes. Search boundary: greps over `src/` for `memory`, `MEMORY`, `.openhands/memory` surfaced only the schema mock, i18n copy, condenser settings, and unrelated error strings.
- Skills: disabling is supported (`disabled_skills` filter, `src/api/agent-server-adapter.ts:763-767`), but file deletion of personal/project skills is delegated to the filesystem/server; no delete call exists in `src/api/skills-service.ts:36-65`.

**3. Is sensitive data stored?**
Yes, in three tiers. (a) Secret *values* are well protected: default settings responses redact them, and the only non-redacted mode returns cipher text (`src/api/settings-service/settings-service.api.ts:115-122`), decrypted server-side (`src/api/mcp-service/mcp-redacted-credentials.ts:77-83`). Git provider tokens are stored exclusively on the agent-server, never mirrored to localStorage (AGENTS.md, consistent with `src/api/backend-registry/storage.ts` containing only backend config keys). (b) Backend API keys/bearer tokens *are* stored plaintext in `localStorage["openhands-backends"]` (`src/api/backend-registry/storage.ts:98`). (c) User-authored content — chat drafts and pending-task drafts — persists in localStorage (`src/utils/conversation-local-storage.ts:44`, 284-294) with no encryption or TTL.

**4. Is memory access audited?**
No. There is no audit logging of memory reads, writes, secret access, or settings fetches anywhere in `src/`. The only matches for "audit" are the goal-loop "audit rounds" feature (`src/types/agent-server/core/events/conversation-state-event.ts:89`, `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:24`), which is unrelated. Telemetry events cover funnel milestones, not data access.

**5. Are scopes enforced in queries?**
Yes, where the frontend owns the query. Skill fetching passes explicit scope flags (`load_user/load_project/load_org`, `src/api/skills-service.ts:50-55`); disabled skills are filtered out of the outbound `agent_context.skills` payload (`src/api/agent-server-adapter.ts:763-767`); public skills are pinned to the build-time catalog with `load_public_skills: false` (`src/api/agent-server-adapter.ts:780-781`). Classification itself is heuristic path matching (`src/utils/skill-scope.ts:35-52`), so misfiled legacy paths could land in the wrong bucket — mitigated by keeping the legacy `/.openhands/microagents/` marker precisely to avoid silently misfiling them as public (`src/utils/skill-scope.ts:11-14`). Enforcement of the `load_memory` scope (what the agent may retain across conversations) is entirely server-side and not verifiable from this source.

## Architectural Decisions

1. **Memory as a global preference, not a per-conversation artifact.** The adapter comment states `load_memory` rides the shared `agent_settings` record onto both inline and profile launches, applied by the server like global `mcp_config` (`src/api/agent-server-adapter.ts:1100-1106`). This simplifies the client but forfeits per-project or per-conversation memory boundaries at the API layer.
2. **Opt-in persistent memory.** The schema default is `false` with prominence "major" (`src/mocks/settings-handlers.ts:364,367`) — privacy-conservative default.
3. **Encrypted round-trip instead of plaintext exposure.** Conversation start requires encrypted settings and refuses redacted fallbacks (`src/api/settings-service/settings-service.api.ts:500-502`); MCP probes splice stored *encrypted* leaves into submitted configs so the browser never holds plaintext (`src/api/mcp-service/mcp-redacted-credentials.ts:84-98`).
4. **Defense-in-depth redaction for display surfaces.** Beyond transport modes, two scrubbers clean rendered text: MCP error redaction with generic token patterns (`src/utils/redact-mcp-secrets.ts:100-127`) and a `<CUSTOM_SECRETS>` masking backstop against backend regressions (`src/utils/redact-custom-secrets.ts:4-8`).
5. **Scope classification at the presentation edge.** Rather than trusting a server-provided scope field, the frontend derives `project/personal/public` from source paths (`src/utils/skill-scope.ts:79-106`) — robust against servers that omit the field, but heuristic.

## Notable Patterns

- **Redaction placeholder protocol**: a sentinel (`REDACTED_MCP_SECRET_VALUE`, `<secret-hidden>`) flows through forms; unchanged placeholders are swapped for stored encrypted values only at probe/persist time (`src/api/mcp-service/mcp-redacted-credentials.ts:40-59`), and equality checks skip re-masking sentinels (`src/utils/redact-custom-secrets.ts:20-23`).
- **Consent as a single-owner store**: only `setTelemetryConsent` mutates consent; identity resets restore the canonical decision immediately after SDK resets (`src/services/telemetry.ts:159-162`), preventing reset-induced silent opt-in/out drift.
- **Sanitize-on-read for persisted blobs**: `sanitizeStoredState` strips removed tabs, invalid view modes, and non-boolean flags when rehydrating conversation state (`src/utils/conversation-local-storage.ts:106-205`) — a data-hygiene pattern that also limits ghost-field leakage across upgrades.
- **Skip-list for ephemeral ids**: task-placeholder conversations (`task-{uuid}`) never persist (`src/utils/conversation-local-storage.ts:211-226`).

## Tradeoffs

- **Frontend-only visibility**: all memory-content guarantees (encryption-at-rest of `.openhands/memory/`, multi-user isolation, server-side audit) live outside this repo; the analysis can verify transport hygiene and toggles only.
- **Heuristic scope classification vs. authoritative metadata**: path-based scope inference (`src/utils/skill-scope.ts:35-52`) keeps the client decoupled from server schema changes but can misclassify unusual layouts; tests pin the common cases only (`__tests__/utils/skill-scope.test.ts:15-49`).
- **UX convenience vs. sensitive-at-rest**: storing backend API keys and draft messages in localStorage enables instant tab restores (`src/api/backend-registry/storage.ts:98`) but leaves credentials and user prose readable by any same-origin script or local disk access.
- **Global memory toggle vs. granularity**: one boolean covers all projects and conversations (`src/api/agent-server-adapter.ts:1100-1106`) — simple, but users cannot scope memory to a workspace or purge per-topic.

## Failure Modes / Edge Cases

- **Disable ≠ delete**: turning off `load_memory` halts future loading; previously accumulated notes persist invisibly to the user (no evidence of any purge path in this repo).
- **Secret rename leaves a window**: `updateSecret` implements rename as upsert-new then delete-old (`src/api/secrets-service.ts:118-120`); a crash between steps strands the old secret.
- **404-as-success deletion**: deleting a nonexistent secret is treated as success (`src/api/secrets-service.ts:140-152`), which masks typos but simplifies idempotent retries.
- **Redaction bypass risk in free text**: `redactMcpSecrets` only scrubs known config values plus generic token shapes (`src/utils/redact-mcp-secrets.ts:22-31`); secrets echoed in non-MCP error surfaces (e.g., bash observations) have no equivalent filter in this repo.
- **Shared-machine leakage**: because auth is a single shared session key baked into the stack (`AGENTS.md` launcher security notes; `src/api/agent-server-client-options.ts` assembly), another browser user on the same OS account inherits full access to memory, skills, and secrets listings.
- **Stale encrypted-settings cache**: a 5-minute TTL cache holds encrypted settings in module memory (`src/api/settings-service/settings-service.api.ts:158-181`); a compromised renderer could read cipher text (low impact) but also cached redacted snapshots.

## Future Considerations

- Add a memory management surface (view/export/delete `.openhands/memory/`) once the SDK exposes endpoints — the current `/settings/agent-context` page (`src/constants/settings-nav.tsx:39-49`) is a natural host.
- Introduce retention/TTL options for localStorage-held drafts and last-conversation maps, or move drafts behind the server-persisted `misc_settings.app_preferences` channel already used for other preferences (see AGENTS.md notes on `PersistedSettings.misc_settings`).
- Extend `redactMcpSecrets`-style scrubbing to generic event/error renderers so token-shaped strings are masked app-wide, not just on MCP test panels.
- Emit telemetry/audit events for security-relevant operations (secret create/delete, memory toggle changes) using the existing typed-event infrastructure (`src/hooks/use-tracking.ts` pattern described in AGENTS.md).
- Replace path-heuristic scope inference if/when the SDK returns authoritative scope fields on `SkillInfo`.

## Questions / Gaps

- What the agent-server does with `load_memory=false` history (retain? purge?) is not observable here — no evidence found in this repo; the software-agent-sdk source would be required.
- Whether `.openhands/memory/` content is ever transmitted to Cloud backends or included in telemetry properties: no evidence found; searched `src/services/telemetry.ts` and `src/api/cloud/` for memory references (none).
- Per-user isolation on shared installations: the frontend assumes a single trusted operator per browser profile; no multi-tenant UI gating was found beyond org-id keying in `src/api/backend-registry/last-conversation-store.ts:12-19`.
- Deletion of personal/project skill files: `SkillsService` is read-only (`src/api/skills-service.ts:36-65`); if deletion exists, it is server/CLI-side.

---

Generated by `Dimension 05.07: Memory Privacy, Scope, and Deletion` against `openhands`.
