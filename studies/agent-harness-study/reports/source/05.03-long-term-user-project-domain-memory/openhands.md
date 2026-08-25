# Source Analysis: openhands

## Dimension 05.03: Long-Term User, Project, and Domain Memory

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + Vite + TanStack Query + Zustand (`package.json:2-4`, deps at `package.json:31`, `package.json:49`, `package.json:69`, `package.json:72`) |
| Analyzed | 2026-08-25 |

## Summary

The analyzed `openhands` source is **Agent Canvas**, the OpenHands frontend (`@openhands/agent-canvas` v1.15.0, `package.json:2-4`). Per its own repo map (`AGENTS.md`, "Repository Map" section), it is only the UI of a multi-repo system; the Python agent-server and SDK (`OpenHands/software-agent-sdk`) own the memory *content*. Consequently this source implements the **control plane** of long-term memory rather than a memory store itself:

1. **Persistent agent memory toggle** — an opt-in `agent_context.load_memory` flag persisted server-side in agent settings; when enabled, "the agent keeps notes under `.openhands/memory/` and loads them at the start of each new conversation" (`src/mocks/settings-handlers.ts:357-359`). The notes themselves are written/read by the agent-server; the frontend only exposes the switch.
2. **Skills (formerly microagents)** — the dominant long-term knowledge mechanism visible here, with explicit **project / personal / public** scopes (`src/utils/skill-scope.ts:3-9`). User/project skills are fetched from the agent-server (`src/api/skills-service.ts:48-57`); public skills are bundled into the JS bundle at build time from the `@openhands/extensions` npm package (`src/api/skills-service.ts:26-34`). At conversation start the frontend merges them into `agent_context.skills` with deny-list filtering (`src/api/agent-server-adapter.ts:749-788`).
3. **Server-side user preferences** — language, git identity, analytics consent, sound notifications, and the `disabled_skills` deny-list persist across sessions under `PersistedSettings.misc_settings.app_preferences` on the agent-server (`src/api/settings-service/settings-service.api.ts:19-49`).
4. **Client-side (localStorage) persistence** — backend registry, per-conversation metadata (repo/branch/workspace/profile), last-conversation-per-backend pointers, recent repositories, onboarding completion, theme, and telemetry consent survive browser restarts but are device-local.

No vector store or embedding-based retrieval exists anywhere in this source: searches for `vector|embedding|recall` return only unrelated hits (a CSS comment in `src/components/features/markdown/markdown-renderer.tsx:56`, XSS guidance in `src/components/features/mcp-page/install-server-modal.tsx:38`). Retrieval of long-term knowledge is keyword-trigger matching performed server-side (`src/api/agent-server-adapter.ts:714-721`).

## Rating

**6 / 10** — Present with a clear model, explicit interfaces, and tests *within its layer* (the frontend control plane), but incomplete as a memory system.

Supporting rationale against the rubric:

- **Clear model with tests**: skill scopes are a typed union with dedicated unit tests (`__tests__/utils/skill-scope.test.ts:14-66`); the memory toggle's read/write behavior is pinned by component tests asserting the exact persisted diff (`__tests__/routes/agent-context-settings.test.tsx:74-94`) and adapter passthrough tests for both OpenHands and ACP launch paths (`__tests__/api/agent-server-adapter.test.ts:514-559`); disabled-skill filtering is tested on both launch paths (`__tests__/api/agent-server-adapter.test.ts:131-178`).
- **Explicit interfaces**: memory-relevant state flows through typed clients (`SkillsClient`, `SettingsClient`, `WorkspacesClient` from `@openhands/typescript-client`; direct HTTP is banned by CI guard `src/api/no-direct-agent-server-calls.test.ts`, described in `AGENTS.md` API Access Rules) and schema-driven settings pages (`src/routes/agent-context-settings.tsx:5-8`).
- **Operational safeguards**: opt-in default-off memory (`src/mocks/settings-handlers.ts:364` — `default: false`); secrets are never mirrored into memory context (single-channel policy tested at `__tests__/api/agent-server-adapter.test.ts:561-575`); deletion flows exist for secrets, conversation metadata, telemetry identity, and workspaces.
- **Why not 7-8**: the layer cannot inspect, correct, or delete the actual accumulated memory content (`.openhands/memory/` notes) — there is no memory-content CRUD surface at all; scope classification relies on brittle filesystem-path string heuristics (`src/utils/skill-scope.ts:21-77`); the public knowledge catalog is frozen at build time (`src/api/skills-service.ts:26-34`); organization-scoped skills exist server-side but are hard-disabled client-side (`load_org: false`, `src/api/skills-service.ts:54`). Freshness management is partial (10-minute query cache only).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Persistent memory store (description) | Schema field description: agent keeps notes under `.openhands/memory/`, loaded at each new conversation start | `src/mocks/settings-handlers.ts:356-370` |
| Memory toggle persistence key | `agent_context.load_memory` boolean, default `false`, prominence `major` | `src/mocks/settings-handlers.ts:359-369` |
| Memory settings page | Route renders schema-driven `SdkSectionPage` filtered to section `agent_context` | `src/routes/agent-context-settings.tsx:3-12` |
| Settings nav entry | Brain-icon nav item to `/settings/agent-context`; comment explains the stored flag rides shared `agent_settings` into ACP conversations | `src/constants/settings-nav.tsx:38-49` |
| Write path for memory toggle | Toggle saved as nested `agent_settings_diff: { agent_context: { load_memory: true } }` via PATCH `/api/settings` | `__tests__/routes/agent-context-settings.test.tsx:89-93` |
| Read-back test | Stored `agent_context.load_memory: true` reflected in the toggle | `__tests__/routes/agent-context-settings.test.tsx:55-72` |
| Launch-time passthrough | `buildStartConversationRequest` spreads stored context so `load_memory` reaches inline OpenHands and inline ACP launches | `__tests__/api/agent-server-adapter.test.ts:514-559` |
| Profile-launch semantics | Comment: `load_memory` is a global user preference stamped onto profile-resolved agents by the agent-server; client must not re-send it | `src/api/agent-server-adapter.ts:1100-1109` |
| Skill scopes | `SkillScope = "project" \| "personal" \| "public"` plus ordering | `src/utils/skill-scope.ts:3-9` |
| Scope directories | Personal markers `/.agents/skills/`, `/.openhands/skills/`, legacy `/.openhands/microagents/`; public marker `/.openhands/cache/skills/` | `src/utils/skill-scope.ts:15-33` |
| Scope classification heuristics | Home-dir regexes and project-prefix checks classify a skill's source path | `src/utils/skill-scope.ts:35-106` |
| Skill scope tests | Public/personal/project classification and grouping covered | `__tests__/utils/skill-scope.test.ts:14-66` |
| Skill retrieval (local) | `SkillsClient.getSkills({ load_public: false, load_user: true, load_project: true, load_org: false, project_dir })`; falls back silently to catalog on error | `src/api/skills-service.ts:46-63` |
| Skill retrieval (cloud) | Paginated cursor walk over cloud `GET /api/v1/skills/search` (limit 100) | `src/api/cloud/skills-service.api.ts:28-49` |
| Public skills bundling | `SKILLS_CATALOG` mapped to `SkillInfo` at module load; build-time immutable snapshot, updates require dependency bump + rebuild | `src/api/skills-service.ts:12-34` |
| SkillInfo type | name/type/source/description/triggers/category/content fields; category absent for user/project skills because the server drops frontmatter metadata | `src/types/settings.ts:72-93` |
| Injection into conversations | `buildAgentContext()` merges existing skills + bundled skills, filters disabled names, sets `load_public_skills: false`, `load_user_skills: true`, `load_project_skills: true` | `src/api/agent-server-adapter.ts:749-788` |
| Bundled skill wire shape | `{ type: "keyword", keywords }` trigger model; `trigger: null` = always-active; SDK performs trigger matching/activation/system-prompt injection | `src/api/agent-server-adapter.ts:699-747` |
| Deny-list write policy | Skills page auto-saves toggles via `saveSettings({ disabled_skills: Array.from(disabledSet) })` after hydration | `src/routes/skills-settings.tsx:84-103` |
| Deny-list enforcement tests | Disabled skills excluded from OpenHands and ACP conversation contexts; enabled ones retained | `__tests__/api/agent-server-adapter.test.ts:131-178` |
| Skill install flow (chat) | `/add-skill` saves skills into `<workspace>/.agents/skills/`; manual folder copy also supported (15 locales) | `src/i18n/translation.json:32727-32743`, `src/components/features/skills/add-skill-modal.tsx:174-181` |
| Install detection | Frontend parses bash observations for `✅ Successfully installed '<name>' to <ws>/.agents/skills/<name>`; installs are inert until next conversation (SDK loads skills once at start) | `src/utils/skill-install-events.ts:19-57`, `__tests__/utils/skill-install-events.test.ts:37-118` |
| Server-side app preferences | `APP_PREFERENCE_FIELDS` (language, consent, sound, git identity, title profile, `disabled_skills`) routed to `misc_settings_diff.app_preferences`; deep-merge semantics, lists replaced wholesale | `src/api/settings-service/settings-service.api.ts:25-33`, `70-85`, `603-644` |
| Server-side workspaces store | Saved workspaces/parents persist on the agent-server at `workspace/.openhands/workspaces.json`; all clients see same list | `src/api/workspaces-service/workspaces-service.api.ts:1-10`, `41-65` |
| Client localStorage stores | Backend registry keys `openhands-backends` / `openhands-active-backend` with validation-on-read and re-seed | `src/api/backend-registry/storage.ts:13-14`, `95-140` |
| Conversation metadata store | `openhands-agent-server-conversation-metadata`: repo/branch/provider/workspace/mode/profile per conversation id | `src/api/conversation-metadata-store.ts:4`, `8-40`, `50-92` |
| Last-conversation pointer | Per-(backend, org) memory of most recent conversation; explicitly "purely a UX shortcut" | `src/api/backend-registry/last-conversation-store.ts:1-11` |
| Recent repositories | Zustand-persisted `home-store` keeps top-3 recent repos + last provider | `src/stores/home-store.ts:26-64` |
| Deletion: metadata removal | `removeStoredConversationMetadata(conversationId)` called when repository detached/deleted | `src/api/conversation-service/agent-server-conversation-service.api.ts:633-643`, `774` |
| Deletion: secrets | `SecretsService.deleteSecret(name)` → `DELETE /api/settings/secrets/:name`; wired to UI mutation | `src/api/secrets-service.ts:130-137`, `src/hooks/mutation/use-delete-secret.ts:5-7` |
| Deletion: telemetry privacy | `clearTelemetryData()` removes consent/first-use/session keys, resets identity ("for privacy/GDPR requests") | `src/services/telemetry.ts:830-858` |
| Staleness: build-time snapshot | Public skill catalog cannot change at runtime; requires npm bump + rebuild | `src/api/skills-service.ts:26-33` |
| Staleness: mid-conversation installs | Skills installed during a conversation stay inert until a new conversation starts in that workspace | `src/utils/skill-install-events.ts:22-28` |
| Freshness: query cache | Skill list cached 10 minutes, no refetch on window focus | `src/hooks/query/use-skills.ts:10-16` |
| Backward compatibility | Legacy `.openhands/microagents/` marker kept so old skills aren't misfiled as "public" | `src/utils/skill-scope.ts:11-14` |
| Org scope present but off | `load_org: false` in local skill fetch — organization scope exists server-side, unused by this client | `src/api/skills-service.ts:54` |
| Single credential channel | Test pins that conversation secrets are NOT mirrored onto `agent_context.secrets`; `request.secrets` is the sole channel | `__tests__/api/agent-server-adapter.test.ts:561-575`, `src/hooks/use-acp-credential-form.ts:53-59` |
| Docs claim tied to implementation | External doc states the Agent Server auto-loads skills from user/project dirs at conversation start | `docs/DefenseClaw.md:90-104` |
| No vector store | Grep for `vector|embedding|recall` yields no retrieval machinery in this source | searched `src/**/*.{ts,tsx}` |

## Answers to Dimension Questions

**1. What persists across sessions?**
Four tiers persist: (a) agent memory content (`.openhands/memory/` notes) on the agent side, gated by the persisted `agent_context.load_memory` flag (`src/mocks/settings-handlers.ts:356-370`); (b) skills on disk — workspace `.agents/skills/` (project), `~/.agents/skills/` and `~/.openhands/skills/` (personal), bundled catalog (public) (`src/utils/skill-scope.ts:15-33`, `src/api/skills-service.ts:26-57`); (c) server-side settings — LLM/agent config diffs plus `misc_settings.app_preferences` including the `disabled_skills` deny-list (`src/api/settings-service/settings-service.api.ts:25-33`, `603-644`) and the workspace list (`workspace/.openhands/workspaces.json`, `src/api/workspaces-service/workspaces-service.api.ts:1-5`); (d) browser localStorage — backends, conversation metadata, recent repos, onboarding flag, telemetry consent (`src/api/backend-registry/storage.ts:13-14`, `src/api/conversation-metadata-store.ts:4`, `src/stores/home-store.ts:61-62`).

**2. Who can write memory?**
The authenticated user through the UI: the memory toggle writes `agent_settings_diff` (`__tests__/routes/agent-context-settings.test.tsx:89-93`); skill deny-lists auto-save from the Skills page (`src/routes/skills-settings.tsx:91-103`); skills are written either by the agent executing the documented `/add-skill` flow (bash writing into `<workspace>/.agents/skills/`, detected post-hoc by `src/utils/skill-install-events.ts:29-57`) or manually by copying folders (`src/components/features/skills/add-skill-modal.tsx:174-181`). The frontend itself never writes `.openhands/memory/` content. All writes go through session-key-authenticated typed clients (`AGENTS.md` API Access Rules). The agent (LLM) can effectively add project-scope skills via shell commands, which the frontend then surfaces.

**3. Who can read memory?**
The agent-server reads everything at conversation start (skills injection `src/api/agent-server-adapter.ts:764-787`; memory loading when `load_memory` is true). Within the UI, any user of the browser profile can read the localStorage tier (no encryption beyond origin scoping), and the Skills page lists all scopes including full skill content from the bundled catalog (`src/types/settings.ts:92` `content?: string`). Secrets are deliberately excluded: GET returns redacted values unless the encrypted-exposure header is used (`src/api/settings-service/settings-service.api.ts:115-122`). There is no multi-user sharing model in this layer — personal scope is home-directory based, and org scope is explicitly not loaded (`src/api/skills-service.ts:54`).

**4. Can memory be corrected?**
Partially. Skills can be corrected by editing/reinstalling their files (re-install keeps the latest event, `src/utils/skill-install-events.test.ts:93-105`) and toggled off without deletion via the deny-list (`src/routes/skills-settings.tsx:153-163`). Settings-level memory preferences are fully editable (toggle + save, `__tests__/routes/agent-context-settings.test.tsx:74-94`). However, there is **no UI to view, edit, or delete the agent's accumulated `.openhands/memory/` notes** — if the agent learned something wrong, the only remedy in this layer is turning memory off entirely (all-or-nothing). No evidence found of any memory-entry CRUD surface; searched `src/**` for `agent_context`, `memory`, `load_memory` beyond the toggle.

**5. Can memory become stale?**
Yes, in several ways: the public skill catalog is a build-time snapshot requiring a dependency bump to update (`src/api/skills-service.ts:26-33`); skills installed mid-conversation remain invisible to the running conversation until a new one starts (`src/utils/skill-install-events.ts:22-28`); the skill list is cached up to 10 minutes (`src/hooks/query/use-skills.ts:14`); localStorage blobs (conversation metadata, recent repos, backend registry entries pointing at dead hosts) have no TTL and are cleaned only by explicit events (`removeStoredConversationMetadata` on detach/delete, `src/api/conversation-service/agent-server-conversation-service.api.ts:641-642`) or validation-on-read (`isValidBackend` filtering, `src/api/backend-registry/storage.ts:130-140`). Server-side settings tolerate version skew gracefully: older servers missing `misc_settings` fall back to defaults (`src/api/settings-service/settings-service.api.ts:59-64`).

## Architectural Decisions

1. **Memory content stays out of the frontend; the frontend ships only the control plane.** The repo map (`AGENTS.md`, "Repository Map") assigns all agent/tool/server-side memory behavior to `software-agent-sdk`; this repo owns settings surfaces and API adaptation. The memory toggle is therefore a thin schema-driven page (`src/routes/agent-context-settings.tsx:3-12`) over a server-owned flag, and even its description string mirrors the server's schema (`src/mocks/settings-handlers.ts:349-353`).
2. **Public knowledge moves from runtime fetch to build-time bundling.** Public skills are baked in from `@openhands/extensions` and injected into `agent_context.skills` with `load_public_skills: false` so the server skips its extensions-repo clone entirely — "the frontend is the sole source of public skills now" (`src/api/agent-server-adapter.ts:764-787`). Tradeoff: zero clone latency vs. frozen catalog until rebuild (`src/api/skills-service.ts:26-33`).
3. **Deny-list instead of deletion for disabling knowledge.** Disabling a skill never removes data; it records names in `disabled_skills` and filters at launch time on both OpenHands and ACP paths (`src/api/agent-server-adapter.ts:763-767`; tests `__tests__/api/agent-server-adapter.test.ts:131-178`). Reversible, auditable, but accumulates stale names.
4. **Global-preference semantics for memory.** `load_memory` is treated as a global user preference, not a per-profile setting: the agent-server stamps the stored value onto profile-launched agents, and the client must not re-send it because `agent_profile_id` and `agent_settings` are mutually exclusive (`src/api/agent-server-adapter.ts:1100-1109`). This makes the memory choice consistent across every launch mode.
5. **Diff-based, deep-merged persistence contract.** All settings writes are partial diffs (`agent_settings_diff`, `conversation_settings_diff`, `misc_settings_diff`), with lists replaced wholesale (`src/api/settings-service/settings-service.api.ts:67-85`, `603-644`). This minimizes clobbering concurrent edits and keeps forward compatibility with newer servers.
6. **Scope classification delegated to path forensics.** Because the agent-server's `/api/skills` drops frontmatter metadata (`src/types/settings.ts:80-84`), the frontend reconstructs scope from source-path strings, including regexes for macOS/Linux home directories and a legacy-directory compat shim (`src/utils/skill-scope.ts:11-106`).

## Notable Patterns

- **Schema-driven settings rendering**: the Memory page renders whatever the server's `agent_context` section exposes — today only `load_memory` — so new server-side memory fields appear without frontend changes; prominence metadata drives UX (a major-only section hides the Basic tab, `src/components/features/settings/sdk-settings/sdk-section-page.tsx:321-326`).
- **Post-hoc event detection as an integration seam**: instead of a skills-install API callback, the frontend pattern-matches bash observation text for the installer's success marker, tolerating soft-timeout continuations and Windows paths (`src/utils/skill-install-events.ts:13-57`), then drives a UI banner.
- **Validated localStorage with graceful degradation**: every store wraps reads/writes in try/catch, validates shapes on read, and treats corruption as "empty" (`readAll()` in `src/api/conversation-metadata-store.ts:50-61`; `readStoredBackends` in `src/api/backend-registry/storage.ts:104-140`).
- **Per-key namespacing of client memory**: composite keys like `${backendId}::${orgId}` partition the last-conversation pointer by backend and org (`src/api/backend-registry/last-conversation-store.ts:15-17`).
- **Project-scope relevance ranking**: conversation overview panels sort skills/automations project-first using the shared scope order (`src/utils/conversation-overview-project-scope.ts:61-100`), keeping domain knowledge contextualized to the open project.

## Tradeoffs

- **Opt-in default-off memory** (`default: false`, `src/mocks/settings-handlers.ts:364`) maximizes privacy and avoids creepy surprise persistence, but means users get no learning benefit unless they find and enable the toggle; combined with no inspection UI, enabled users also cannot audit what was remembered.
- **Build-time public catalog**: removes GitHub clone latency and network failure modes (explicitly noted in the migration comment, `src/api/agent-server-adapter.ts:771-779`) at the cost of stale knowledge between releases and larger JS bundles.
- **Path-heuristic scope classification**: works without server cooperation but misclassifies unusual layouts (e.g., a project checked out under `$HOME` root, non-standard cache paths); the code acknowledges fragility by warning that dropping the legacy marker would "silently misfile legacy skills as 'public'" (`src/utils/skill-scope.ts:11-14`).
- **Silent fallbacks over errors**: if the agent-server's skills endpoint fails, the service swallows the error and returns just the bundled catalog (`src/api/skills-service.ts:58-61`) — high availability, but users may not notice their personal/project skills vanished from the list.
- **localStorage convenience memory** is explicitly labeled disposable ("purely a UX shortcut — losing it just falls back", `src/api/backend-registry/last-conversation-store.ts:1-8`), trading continuity for simplicity; it does not follow users across devices.

## Failure Modes / Edge Cases

- **Wrong-memory lock-in**: with `load_memory` enabled there is no selective correction path in this layer; a poisoned `.openhands/memory/` note propagates into every new conversation until memory is globally disabled or files are edited outside the app (server-side concern; no evidence of frontend tooling).
- **Stale deny-list entries**: `disabled_skills` replaces wholesale and is keyed by name; renaming/uninstalling skills leaves orphaned names indefinitely (merge semantics at `src/api/settings-service/settings-service.api.ts:70-75`).
- **Legacy path drift**: skills under pre-rename `.openhands/microagents/` depend on the marker list staying in sync with the SDK (`src/utils/skill-scope.ts:11-19`); a new SDK location would be misclassified as public.
- **Mid-conversation installs invisible**: a skill installed during the current conversation won't activate until the next conversation (`src/utils/skill-install-events.ts:22-28`) — a freshness gap users could perceive as "the skill didn't take".
- **Version skew**: `misc_settings` requires agent-server ≥ 1.27; older servers silently lose app-preference persistence (`src/api/settings-service/settings-service.api.ts:59-64`). Cloud/local payload divergence for the same fields adds dual-path risk (`src/api/cloud/settings-service.api.ts:161-193`).
- **Corrupted/partial localStorage**: JSON parse failures degrade to empty stores by design (`src/api/conversation-metadata-store.ts:50-61`), which silently discards user context rather than attempting repair.
- **Credential bleed prevention is actively tested**: mirroring conversation secrets onto `agent_context.secrets` would create a dead second channel; a regression test forbids it (`__tests__/api/agent-server-adapter.test.ts:561-575`).

## Future Considerations

- Expose a memory-inspection/correction surface: list, view, edit, and delete individual `.openhands/memory/` entries once the SDK exposes such endpoints — currently the biggest gap for "can memory be corrected?" (searched `src/**` for memory-content APIs; none found).
- Replace path-string scope heuristics with a server-provided scope field on `SkillInfo` (`src/types/settings.ts:74-93`) to eliminate misclassification risk.
- Add a runtime refresh path for the public catalog (or smaller delta updates) to reduce staleness between releases (`src/api/skills-service.ts:26-34`).
- Surface org-scope skills (`load_org` currently hardcoded false, `src/api/skills-service.ts:54`) for team-shared domain knowledge once cloud parity is desired.
- Add TTL/GC for localStorage metadata keyed by deleted conversation ids beyond the explicit removal calls (`src/api/conversation-service/agent-server-conversation-service.api.ts:641-642`).
- Consider surfacing skill-install success/failure toasts directly from a server ack rather than output parsing (`src/utils/skill-install-events.ts:19-20`) to survive installer wording changes.

## Questions / Gaps

- What exactly the agent writes into `.openhands/memory/`, its format, size limits, and update triggers cannot be answered from this source — the only description is the schema help text (`src/mocks/settings-handlers.ts:357-359`); the implementation lives in `software-agent-sdk`, outside the isolation boundary of this study.
- Whether `load_memory` interacts with the condenser (in-session compression settings rendered under the memory-icon nav item at `src/constants/settings-nav.tsx:32-37`, route `src/routes/condenser-settings.tsx:3-12`) is unstated; they appear to be independent mechanisms (context-window management vs cross-session notes).
- No evidence found of memory export/import, retention policies, or per-conversation memory isolation; searched `src/**` and `docs/**` for retention/expiry concepts (only DefenseClaw audit-retention discussion in `docs/DefenseClaw.md:219`, which concerns external scanning, not agent memory).
- The cloud skills endpoint's scope semantics (whether it returns personal vs org skills) are opaque in this layer; the client passes results through unchanged (`src/api/cloud/skills-service.api.ts:21-27`).

---

Generated by `dimensions/05.03-long-term-user-project-and-domain-memory.md` against `openhands`.
