# Source Analysis: openhands

## Dimension 12.01: Prompt Storage and Versioning

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (React Router 7, Vite, TanStack Query); the OpenHands "agent-canvas" frontend |
| Analyzed | 2026-08-26 |

## Summary

This source is only the **frontend** of a multi-repo system (`AGENTS.md` explicitly scopes it: "This repo (`OpenHands/OpenHands`) is **only the agent-canvas frontend**"). The core LLM system prompt templates are owned by a separate Python backend (`OpenHands/software-agent-sdk`) and never appear as template files here. What this repo does own is a layered set of prompt *inputs* and a strong run-time observability story:

1. **Core system prompt** — lives in the backend SDK; the frontend consumes it post-hoc as an immutable, persisted `SystemPromptEvent` in each conversation's event stream (`src/types/agent-server/core/events/system-event.ts:5-26`).
2. **Public skill prompts** — bundled at build time from the `@openhands/extensions` npm package via `SKILLS_CATALOG` (`src/api/skills-service.ts:26-34`) and injected per-conversation through `agent_context.skills` in the start-conversation payload (`src/api/agent-server-adapter.ts:722-747`, `src/api/agent-server-adapter.ts:764-781`). Updating them requires a dependency bump + rebuild.
3. **User/project skill prompts** — plain markdown files on disk (`~/.agents/skills/`, `~/.openhands/skills/`, legacy `.openhands/microagents/`, `{workspace}/.agents/skills/`), loaded from the agent-server's `/api/skills` endpoint with scope flags (`src/api/skills-service.ts:48-56`; path markers in `src/utils/skill-scope.ts:15-19`).
4. **Automation prompts** — user-authored free text stored server-side in the automation backend's SQLite DB (`automations.db`, `config/defaults.json` → `paths.automationDb`), managed purely over REST CRUD (`src/api/automation-service/automation-service.api.ts:320-409`).
5. **Runtime-injected prompt suffix** — the client renders a `<RUNTIME_SERVICES>` markdown block and attaches it as `AgentContext.system_message_suffix` on conversation start (`src/api/agent-server-adapter.ts:125`, `src/api/agent-server-adapter.ts:784-786`).

Versioning is the weak axis: there are no prompt-version identifiers or content hashes anywhere (searched for `prompt_version|promptHash|contentHash` — no matches). A `version` field exists on the skill wire types but is rarely populated, and the UI's version display slots are hardcoded to `null` (`src/utils/system-message-adapter.ts:32-33`). Run-to-prompt traceability instead relies on replaying the full persisted prompt text from the event stream plus per-message `activated_skills` lists.

## Rating

**5 / 10** — Present but inconsistent.

The storage model is clear and well-layered with explicit ownership boundaries, and run-time observability is genuinely strong (every run persists its exact system prompt text, tool list, dynamic context, and per-message skill activations). But explicit versioning is mostly absent: no version IDs or hashes on any prompt artifact, a defined-but-unpopulated `version` field, dead version-display UI, schema-version-only envelopes for export, and public-skill updates gated behind a rebuild. That places it squarely in the rubric's "present but inconsistent" band rather than the "clear model with tests and operational safeguards" band.

## Evidence Collected

Every entry cites `path:line` relative to the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Repo scope: frontend only, prompts owned by backend SDK | Multi-repo table: SDK repo owns "agents, tools, conversations, events"; extensions repo owns skills | AGENTS.md (Repository Map section) |
| System prompt arrives as a persisted event, not a local template | `SystemPromptEvent { system_prompt: TextContent; tools; dynamic_context? }` | src/types/agent-server/core/events/system-event.ts:5-26 |
| Type guard for the event | `"system_prompt" in event && typeof event.system_prompt === "object"` | src/types/agent-server/type-guards.ts:203-211 |
| Public skills are a build-time snapshot | Comment: catalog is "baked into the bundle at `npm run build`… Updating requires bumping `@openhands/extensions` and rebuilding" | src/api/skills-service.ts:26-34 |
| Catalog → SkillInfo mapping drops version/category | Mapping sets name/type/source/description/triggers/category/content/license/compatibility only | src/api/skills-service.ts:12-24 |
| User/project skills loaded from agent server, public skipped | `SkillsClient.getSkills({ load_public: false, load_user: true, load_project: true })`; silent fallback to bundled catalog on error | src/api/skills-service.ts:46-63 |
| Skill storage locations on disk | Markers `/.agents/skills/`, `/.openhands/skills/`, legacy `/.openhands/microagents/` | src/utils/skill-scope.ts:11-19 |
| Cloud skills path | Paginated `GET /api/v1/skills/search` walked to exhaustion | src/api/cloud/skills-service.api.ts:28-49 |
| Bundled skills injected per-run into agent context | `buildBundledSkills()` maps catalog entries to SDK `Skill` JSON incl. SKILL.md source path via `__EXTENSIONS_SKILLS_DIR__` | src/api/agent-server-adapter.ts:714-747 |
| Server-side clone disabled; frontend is sole source of public skills | `skills: mergedSkills, load_public_skills: false, load_user_skills: true, load_project_skills: true` | src/api/agent-server-adapter.ts:769-783 |
| Runtime-injected prompt tail | `system_message_suffix` attached when runtime-services info present | src/api/agent-server-adapter.ts:784-786 |
| Build-time injection of extensions dir constant | `__EXTENSIONS_SKILLS_DIR__` define (empty string for library builds) | vite.config.ts:121-128; src/api/agent-server-adapter.ts:732-734 |
| Automation prompts stored server-side, edited over REST | `createAutomation` POST `/api/automation/v1/preset/prompt` (no plugins) or plugin path; PATCH update | src/api/automation-service/automation-service.api.ts:184-207, 390-409 |
| Preset path test | Asserts POST to `/api/automation/v1/preset/prompt` when no plugins configured | src/api/automation-service/automation-service.api.test.ts:266-277 |
| Automation record carries raw prompt, timestamps, no version | `Automation { … created_at; updated_at; prompt: string \| null }` | src/types/automation.ts:38-46 |
| Automation run links to conversation, not prompt revision | `AutomationRun { id, status, conversation_id, bash_command_id, cost?, started_at, completed_at }` | src/types/automation.ts:75-98 |
| Export envelope has schema version only | `{ version: envelope.fileVersion, kind, spec }`; import rejects mismatched version | src/utils/automation-export.ts:170-196 |
| Envelope version pinned to literal 1 by interface validation | `if (fileVersion !== 1) check.fail("importExport.fileVersion", "must be 1")` | src/manifests/interface-validation.ts:592-593 |
| Skill version field exists on both GUI and wire types | `SkillInfo.version?: string`; wire `Skill.version?: string` | src/types/settings.ts:85; src/api/conversation-service/agent-server-conversation-service.types.ts:229 |
| Version rendered only when present | Version pill gated on `if (skill.version)` | src/components/features/skills/build-skill-pills.tsx:68-85 |
| Local skill metadata dropped by server | Comment: agent-server `/api/skills` response "drops SKILL.md frontmatter metadata, so a local `category` cannot reach us" | src/types/settings.ts:80-84 |
| Version display slots hardcoded null | `openhands_version: null, agent_class: null` returned by adapter | src/utils/system-message-adapter.ts:32-33 |
| Modal header would show version if present | Conditional render of `openhandsVersion` / `agentClass` | src/components/features/conversation-panel/system-message-modal/system-message-header.tsx:25-48 |
| Run→prompt traceability: modal reads live event store | `adaptSystemMessage(events)` where events come from `useEventStore` | src/hooks/use-conversation-name-context-menu.ts:39-56 |
| Dynamic context appended + secrets redacted before display | Adapter appends `dynamic_context.text` and runs `redactCustomSecrets` | src/utils/system-message-adapter.ts:22-27 |
| Adapter behavior under test | Tests cover v1 shape, absent dynamic context, appending, secret redaction | __tests__/utils/system-message-adapter.test.ts:27-77 |
| Per-message skill activation recorded in stream | `MessageEvent.activated_skills: string[]` + `extended_content` | src/types/agent-server/core/events/message-event.ts:11-19 |
| Activation surfaced as synthetic UI event | `createSkillReadyEvent(userEvent)` builds "Skill Ready" card from activated skills | src/components/conversation-events/chat/event-content-helpers/create-skill-ready-event.ts:35-59 |
| E2E proves file-based skills activate per keyword | Mock-LLM spec asserts `activated_skills` includes project/user skill names | tests/e2e/mock-llm/skills/mock-llm-skills.spec.ts:152-245 |
| E2E proves deletion changes behavior | Header comment: "Skill deletion: removing a skill file means it is NOT loaded" | tests/e2e/mock-llm/skills/mock-llm-skills.spec.ts:17-19 |
| Disabled skills filtered out at launch (both agent kinds) | Unit tests assert disabled names excluded, bundled `add-javadoc` included | src/api/agent-server-adapter.test.ts:131-178 |
| Profile-launch path loses client-side skill enrichment | Comment: server rebuilds agent from stored profile; public-skill restoration tracked in software-agent-sdk#3967 | src/api/agent-server-adapter.ts:1086-1098 |
| Backend/automation versions centrally pinned | `versions.agentServer: "1.42.1"`, `automation: "1.8.0"`, `compatibility.minimumAgentServer: "1.28.0"` | config/defaults.json:4-12 |
| Automation git-sync tracks last synced commit SHA | `GitSyncStatus.last_synced_commit: string \| null` | src/types/git-sync.ts:9 |
| Skills authored via chat, not registry write | Add-skill modal instructs asking the agent to create the file; no POST skill API in SkillsService | src/components/features/skills/add-skill-modal.tsx:114-196 |
| No prompt hashing/version identifiers anywhere | Searches for `prompt_version|promptVersion|prompt_hash|contentHash` returned zero matches | (search boundary: all of `src/`, `tests/`, `scripts/`) |

## Answers to Dimension Questions

**1. Where are prompts stored?**
Four tiers, none of which is a classic "prompt template file in this repo":
- Core system prompt: backend SDK repo (out of scope of this directory). The frontend only ever sees materialized instances as events (`src/types/agent-server/core/events/system-event.ts:5-26`).
- Public skills: compiled into the JS bundle from the `@openhands/extensions` npm package (`src/api/skills-service.ts:26-34`), with the original `SKILL.md` paths injected at build time for resource resolution (`vite.config.ts:121-128`).
- User/project skills: markdown files on the user/workspace filesystem, read by the agent-server (`src/utils/skill-scope.ts:15-19`, `src/api/skills-service.ts:48-56`).
- Automation prompts: rows in the automation backend's SQLite DB, accessed only via REST (`src/api/automation-service/automation-service.api.ts:50`, `config/defaults.json` `paths.automationDb`).

**2. Are prompt versions tracked?**
Largely no. There is no version identifier, hash, or revision counter for any prompt artifact in this codebase (zero grep hits). The type system anticipates it — `SkillInfo.version` (`src/types/settings.ts:85`) and wire `Skill.version` (`src/api/conversation-service/agent-server-conversation-service.types.ts:229`) — but the catalog mapping omits it (`src/api/skills-service.ts:12-24`) and the server reportedly drops frontmatter metadata for local skills (`src/types/settings.ts:80-84`). Automations carry only `created_at`/`updated_at` (`src/types/automation.ts:38-39`). Implicit versioning exists indirectly: exact-pinned npm dependency (`@openhands/extensions` in package.json), pinned backend/automation versions (`config/defaults.json:4-12`), and git-sync's `last_synced_commit` (`src/types/git-sync.ts:9`). Export envelopes carry a schema format version (must equal 1), not content versions (`src/manifests/interface-validation.ts:592-593`).

**3. Can a run be traced to the exact prompt version used?**
To the exact prompt **text**, yes — this is the strongest part of the model. Every conversation persists a `SystemPromptEvent` containing the full system prompt string, the tool list, and a `dynamic_context` block (`src/types/agent-server/core/events/system-event.ts:5-26`), which the UI replays from the event store into a viewer modal (`src/hooks/use-conversation-name-context-menu.ts:55-56`, `src/utils/system-message-adapter.ts:13-34`). Additionally, every assistant turn records which skills were activated (`MessageEvent.activated_skills`, `src/types/agent-server/core/events/message-event.ts:14`), verified end-to-end against real skill files (`tests/e2e/mock-llm/skills/mock-llm-skills.spec.ts:224-245`). To a named **version**, no — nothing records which catalog snapshot, dependency version, or skill-file revision produced the text, and the modal's version slots render nothing because they are hardcoded `null` (`src/utils/system-message-adapter.ts:32-33`). Transcript export also omits the system prompt entirely (`src/utils/transcript-export/index.ts` handles only actions/messages/observations).

**4. Can prompts be updated without redeploying code?**
Partially, by tier:
- Yes — user/project skills: edit or delete a markdown file and the next conversation picks it up (`load_project_skills` always on, `src/api/agent-server-adapter.ts:782-783`; deletion semantics proven e2e, `tests/e2e/mock-llm/skills/mock-llm-skills.spec.ts:17-19`). Notably there is no management API — the intended authoring flow is asking the agent itself to write the file (`src/components/features/skills/add-skill-modal.tsx:136-181`).
- Yes — automation prompts: first-class CRUD over REST (`PATCH`, `src/api/automation-service/automation-service.api.ts:390-409`), deployable across local/cloud backends, optionally mirrored to git with commit tracking (`src/types/git-sync.ts:1-22`).
- No — public skills: explicitly a build-time snapshot; updates require bumping `@openhands/extensions` and rebuilding (`src/api/skills-service.ts:29-33`), though this was a deliberate tradeoff to eliminate clone latency.
- Indirectly — core system prompt: owned by the pinned backend version (`config/defaults.json:4-12`), so prompt changes arrive via server redeployment, not frontend releases.

## Architectural Decisions

1. **Frontend as sole distributor of public skills.** Bundled skills ride inside the conversation-start payload and `load_public_skills: false` tells the server to skip its own extension-repo clone ("the frontend is the sole source of public skills now", `src/api/agent-server-adapter.ts:769-781`). This trades runtime freshness for determinism and startup latency.
2. **Prompts-as-events for observability.** Rather than a registry lookup, the system prompt is recorded once per run as an ordinary event (`src/types/agent-server/core/events/system-event.ts:5-26`), making the exact bytes sent to the LLM auditable from the same stream that holds the rest of the run.
3. **Filesystem as the user-skill store.** No database or registry for personal/project skills — just conventional directories (`src/utils/skill-scope.ts:15-19`) resolved heuristically from the server-reported `source` path, including backward compatibility for the pre-rename `.openhands/microagents/` layout.
4. **Declarative version pins as the compatibility contract.** `config/defaults.json` is the single source of truth consumed by scripts, Docker, and CI (`config/defaults.json:3-12`), with a hard minimum-server floor enforced at bootstrap (`compatibility.minimumAgentServer`), pinning prompt-producing backend versions operationally even though prompts themselves aren't versioned.
5. **Schema-versioned interchange, not content versioning.** Import/export validates the *envelope* format version (`src/utils/automation-export.ts:190-196`) while treating prompt content as an opaque mutable string.

## Notable Patterns

- **Layered fallback:** skill loading silently degrades to the bundled catalog if the agent-server lacks/unreachable skills endpoint (`src/api/skills-service.ts:58-61`).
- **Deny-list gating:** `disabled_skills` app preferences filter skills out of `agent_context` at launch for both OpenHands and ACP agent kinds, unit-tested (`src/api/agent-server-adapter.test.ts:131-178`).
- **Secret hygiene at display time:** the dynamic-context tail is redacted before rendering (`src/utils/system-message-adapter.ts:26`, tested at `__tests__/utils/system-message-adapter.test.ts:59-76`).
- **Synthetic UI events:** skill activation is re-rendered as a derived "Skill Ready" card built from `activated_skills` + `extended_content` (`src/components/conversation-events/chat/event-content-helpers/create-skill-ready-event.ts:35-59`).
- **Known-gap documentation in code:** the profile-launch enrichment gap (server must restore default tools + public skills) is written directly into the payload builder with a tracking issue reference (`src/api/agent-server-adapter.ts:1086-1098`).

## Tradeoffs

- **Build-time bundling vs freshness:** public skills can't change without an npm release + rebuild (`src/api/skills-service.ts:29-33`), but the design removes GitHub clone latency and makes the shipped catalog reproducible.
- **Full-text replay vs version IDs:** persisting the whole prompt per run gives perfect point-in-time fidelity but bloats event streams and provides no cheap way to compare two runs' prompts or answer "which catalog version was this?" without diffing text manually.
- **Filesystem skills vs governance:** user/project skills update instantly with zero ceremony, but nothing records who changed what; the only durable audit trail is whatever VCS happens to track the workspace.
- **Frontend-owned distribution vs multi-client consistency:** because Canvas injects skills client-side, other clients of the same agent-server would get different public-skill sets unless they replicate `buildBundledSkills()`.

## Failure Modes / Edge Cases

- **Silent degradation:** if the skills endpoint fails, users see only bundled public skills with no error surfaced (`src/api/skills-service.ts:58-61`).
- **Stale query window:** the skills list is cached for 10 minutes with no refetch on focus (`src/hooks/query/use-skills.ts:14-15`), so freshly added skill files may not appear in the picker (though activation at conversation start reads from disk server-side regardless).
- **Profile-launched conversations diverge:** on the `agent_profile_id` path the client-side skill merge does not apply; until software-agent-sdk#3967 lands, such agents may lack public skills entirely (`src/api/agent-server-adapter.ts:1086-1098`).
- **Legacy path misclassification risk:** scope detection depends on string-matching home-directory patterns (`src/utils/skill-scope.ts:35-52`); nonstandard HOME layouts could misfile personal skills as project/public (the code comments acknowledge this mapping is load-bearing).
- **Dead version metadata:** because `openhands_version`/`agent_class` are hardcoded `null` (`src/utils/system-message-adapter.ts:32-33`), any operator relying on the modal header for provenance gets nothing — a trap for debugging "which agent produced this prompt?".
- **Client-tool schema caching:** tool schemas registered at conversation start are cached per-process by the server; edits require a dev-server restart (`src/api/agent-server-adapter.ts:1111-1115`) — adjacent to prompt assembly since tools are part of the `SystemPromptEvent`.

## Future Considerations

- Populate the already-defined `version` fields end-to-end: emit frontmatter `version` from the agent-server `/api/skills`, map it in `catalogEntryToSkillInfo` (`src/api/skills-service.ts:12-24`), and stamp the resolved catalog/npm version onto each `SystemPromptEvent`.
- Wire `openhands_version`/`agent_class` in `adaptSystemMessage` (`src/utils/system-message-adapter.ts:32-33`) to real values from `/server_info` so the existing UI becomes useful.
- Record a content hash (or skill-name+revision list) on automation runs (`src/types/automation.ts:75-98`) to close the "which prompt revision produced this run" gap for automations.
- Include the system prompt in transcript export for offline auditing parity with the in-app modal.
- Resolve software-agent-sdk#3967 so profile-launched conversations receive the same skill enrichment as inline launches.

## Questions / Gaps

- **Unanswerable from this source:** how the backend SDK templates, stores, or versions the core system prompt internally — the SDK repo is outside the isolation boundary. All claims about it here are limited to what crosses the API surface (event shapes, payload fields, version pins).
- No evidence found of any prompt A/B testing, staged rollout, or per-org prompt overrides; searched `src/` for `prompt_version`, variant/experiment identifiers tied to prompts — none exist.
- Whether the cloud `/api/v1/skills/search` returns populated `version` values could not be confirmed from this repo alone (`src/api/cloud/skills-service.api.ts:28-49` passes items through unchanged); the type permits it but nothing here exercises it.
- The automation backend's DB schema (revision history, if any) lives outside this directory; only the REST surface (`src/api/automation-service/automation-service.api.ts`) and its types were observable.

---

Generated by `12.01-prompt-storage-and-versioning` against `openhands`.
