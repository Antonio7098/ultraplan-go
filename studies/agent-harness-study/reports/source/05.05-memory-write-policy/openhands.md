# Source Analysis: openhands

## Dimension 05.05: Memory Write Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Agent Canvas frontend; agent-server + memory engine live in the sibling `software-agent-sdk` repo) |
| Analyzed | 2026-08-26 |

## Summary

This source is the OpenHands **Agent Canvas frontend** (React/TypeScript). Per the repo's own architecture note (`AGENTS.md`, "Repository Map"), persistent-memory *mechanics* belong to the sibling `software-agent-sdk` (Python agent-server) repository, which is outside this study's isolation boundary. Within this source, the entire memory-write-policy surface is a **single, explicit, opt-in gate**: `agent_settings.agent_context.load_memory`.

The policy that IS observable here is deliberate and conservative:

1. **Opt-in by default.** The `agent_context.load_memory` schema field defaults to `false` (`src/mocks/settings-handlers.ts:364`), and a test pins this: "renders the persistent-memory toggle from the agent schema, off by default" (`__tests__/routes/agent-context-settings.test.tsx:43-53`). The frontend's own default settings contain no `agent_context` block at all (`src/services/settings.ts:35-52`) — memory only exists once a user enables it.
2. **Explicit user approval for the capability, not per-fact.** Enabling memory requires navigating to Settings → Agent Context (`src/routes/agent-context-settings.tsx:3-12`, rendered via the schema-driven `SdkSectionPage`) and pressing an explicit Save button (`src/components/features/settings/sdk-settings/sdk-section-page.tsx:718-732`). The save is persisted as a nested diff — `agent_settings_diff: { agent_context: { load_memory: true } }` — pinned by test (`__tests__/routes/agent-context-settings.test.tsx:74-94`) and routed through `useSaveSettings` → PATCH (`src/hooks/mutation/use-save-settings.ts:34-47`).
3. **The write itself is delegated to the agent sandbox.** The user-facing description states the contract: "The agent keeps notes under `.openhands/memory/` and loads them at the start of each new conversation, learning your codebase and preferences over time" (`src/mocks/settings-handlers.ts:358`; localized copy at `src/i18n/translation.json:2451`). No extraction, validation, conflict-resolution, or confidence-scoring code exists in this repository.
4. **Both launch paths honor the flag.** Inline launches spread the stored `agent_context` (including `load_memory`) into the conversation payload via `buildAgentContext` (`src/api/agent-server-adapter.ts:749-788`), with tests for both OpenHands and ACP inline paths (`__tests__/api/agent-server-adapter.test.ts:514-559`). Profile launches send no `agent_settings` at all; instead the agent-server "stamps the stored `agent_settings.agent_context.load_memory` onto the profile-resolved agent the same way it already applies the global `mcp_config`" (`src/api/agent-server-adapter.ts:1100-1106`).
5. **Credentials are deliberately kept out of the memory-config channel.** A dedicated test asserts conversation secrets are NOT mirrored onto `agent_context` because `request.secrets` is the sole credential channel (`__tests__/api/agent-server-adapter.test.ts:561+`), and the `load_memory` schema field carries `secret: false` (`src/mocks/settings-handlers.ts:368`).

The dimension's deeper questions — what triggers an individual fact write, fact verification, conflict handling, confidence metadata, per-write user approval, or a UI to correct stored memories — **cannot be answered from this source**: those mechanisms live in `software-agent-sdk`. What this source contributes to memory-write safety is the gate (opt-in, off by default), tested propagation on every launch path, and an architectural rule separating secrets from the agent-context/memory channel.

## Rating

**5 / 10** — Present but incomplete in this source.

Rationale against the rubric:

- **For deliberate design (pushes toward 7-8):** memory cannot be enabled accidentally — it requires explicit user opt-in with a persisted, schema-driven toggle whose default is `false` (`src/mocks/settings-handlers.ts:364`, `__tests__/routes/agent-context-settings.test.tsx:43-53`); propagation across both launch paths is pinned by tests (`__tests__/api/agent-server-adapter.test.ts:514-559`); and secrets are architecturally excluded from the channel that carries the memory configuration (`__tests__/api/agent-server-adapter.test.ts:561+`).
- **Against (caps at 5):** within this source there are **no extractors, no validators, no conflict handlers, no confidence scores, no per-write approval flow, and no memory browsing/correction UI** — all of the actual write-policy machinery is externalized to the sibling SDK repo. The frontend cannot answer whether individual facts are verified before being remembered, nor can users review or delete specific memories from here.

## Evidence Collected

Every entry includes a file path with line numbers (workspace-relative).

| Area | Evidence | File:Line |
|------|----------|-----------|
| Memory write trigger | Global opt-in flag `agent_settings.agent_context.load_memory`; agent-server stamps it onto profile-resolved agents like global `mcp_config` | `studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:1100-1106` |
| Storage location contract | "The agent keeps notes under `.openhands/memory/` and loads them at the start of each new conversation" (schema field description) | `studies/agent-harness-study/sources/openhands/src/mocks/settings-handlers.ts:356-370` |
| Localized user-facing copy (15 languages) | Same contract surfaced through i18n key `SCHEMA$AGENT_CONTEXT$LOAD_MEMORY$DESCRIPTION` | `studies/agent-harness-study/sources/openhands/src/i18n/translation.json:2450-2465` |
| Default-off | Schema default `default: false`, prominence `major`, `secret: false` for `agent_context.load_memory` | `studies/agent-harness-study/sources/openhands/src/mocks/settings-handlers.ts:359-369` |
| Off-by-default test | Test name: "renders the persistent-memory toggle from the agent schema, off by default" | `studies/agent-harness-study/sources/openhands/__tests__/routes/agent-context-settings.test.tsx:43-53` |
| Explicit approval flow (capability level) | Toggle rendered on `/settings/agent-context`; save emits `agent_settings_diff: { agent_context: { load_memory: true } }` on Save-button click | `studies/agent-harness-study/sources/openhands/__tests__/routes/agent-context-settings.test.tsx:74-94` |
| Settings screen wiring | `AgentContextSettingsScreen` renders section `agent_context` via schema-driven `SdkSectionPage` | `studies/agent-harness-study/sources/openhands/src/routes/agent-context-settings.tsx:3-12` |
| Explicit Save button (no auto-persist) | Save button disabled until dirty; calls `handleSave` which builds a dirty-only coerced payload | `studies/agent-harness-study/sources/openhands/src/components/features/settings/sdk-settings/sdk-section-page.tsx:718-732`, `530-584` |
| Persistence mechanism | Diff-based PATCH: `agent_settings_diff` assembled, full `agent_settings` deleted before send | `studies/agent-harness-study/sources/openhands/src/hooks/mutation/use-save-settings.ts:34-47` |
| Inline-launch propagation | `buildAgentContext` spreads stored context (`...existingContext`) so `load_memory` rides into `agent_context.skills`-bearing payloads | `studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:756-787` |
| Inline-launch tests | "passes a stored agent_context.load_memory through on an inline launch" and "...on an inline ACP launch" | `studies/agent-harness-study/sources/openhands/__tests__/api/agent-server-adapter.test.ts:514-559` |
| Profile-launch propagation | Client must NOT re-send `load_memory` (mutually exclusive with `agent_profile_id`); server applies stored preference | `studies/agent-harness-study/sources/openhands/src/api/agent-server-adapter.ts:1085-1109` |
| Secrets excluded from memory-config channel | Test: "does NOT mirror conversation secrets onto agent_context for ACP — request.secrets is the sole channel" | `studies/agent-harness-study/sources/openhands/__tests__/api/agent-server-adapter.test.ts:561-566` |
| Nav entry for the Memory page | Agent Context nav item documented as "today only persistent memory (`agent_context.load_memory`)" | `studies/agent-harness-study/sources/openhands/src/constants/settings-nav.tsx:38-49` |
| Frontend defaults omit agent_context | `DEFAULT_SETTINGS.agent_settings` has no `agent_context` key (memory absent until enabled) | `studies/agent-harness-study/sources/openhands/src/services/settings.ts:35-52` |
| Related in-context "memory" machinery (contrast) | `CondensationEvent` carries `forgotten_event_ids` + optional `summary`; condenser defaults enabled=true, max_size=240 | `studies/agent-harness-study/sources/openhands/src/types/agent-server/core/events/condensation-event.ts:5-52`; `studies/agent-harness-study/sources/openhands/src/services/settings.ts:18-19,42-45` |

## Answers to Dimension Questions

**1. What causes memory to be written?**
Within this source: nothing writes memory directly. Writes happen inside the agent sandbox under `.openhands/memory/` once `agent_context.load_memory` is enabled by the user (`src/mocks/settings-handlers.ts:358`, `src/api/agent-server-adapter.ts:1100-1106`). The trigger chain visible here is: user flips toggle → Save button → `agent_settings_diff` PATCH → flag present on subsequent conversation launches (inline spread at `src/api/agent-server-adapter.ts:769-787`; server-side stamping on profile launches at `src/api/agent-server-adapter.ts:1100-1106`). What triggers an *individual* note write during a session is not observable here — No evidence found (search boundary: greps for `memory`, `save_memory`, `write_memory`, `remember`, `recall` across `src/`, `docs/`, `types/`; only the settings surface matched).

**2. Can the model write arbitrary memory?**
Not answerable from this source. The model runs server-side (software-agent-sdk); this repo exposes no tool allowlist/denylist for memory files. The only adjacent signal is that memory notes live under `.openhands/memory/` while skills live under `.openhands/skills/` (separate namespaces, `src/mocks/settings-handlers.ts:358` vs `src/utils/skill-scope.ts:17`), implying scoped file locations rather than free-form storage. No evidence found of any client-side constraint on what the agent may remember.

**3. Are facts verified?**
No evidence found in this source. There is no verification, critic, or validation step applied to memory contents anywhere in `src/`. (The separate `verification.critic_enabled` setting at `src/services/settings.ts:46-49` governs response critique, not memory validation.)

**4. Can users correct memory?**
Only bluntly: users can turn persistent memory off entirely via the same toggle (`src/routes/agent-context-settings.tsx:3-12`), but there is no UI to list, inspect, edit, or delete individual memories under `.openhands/memory/`. No evidence found of any memory-management surface (search boundary: `memory_icon` usages resolve only to the settings nav item, `src/constants/settings-nav.tsx:3,33`; no memory browser components exist under `src/components/`).

**5. Are sensitive facts excluded?**
Partially evidenced. Two concrete safeguards exist in this source: (a) credentials are architecturally barred from the `agent_context` channel — `request.secrets` (LookupSecrets pointing back at `/api/settings/secrets/{name}`) is the sole credential path, enforced by test (`__tests__/api/agent-server-adapter.test.ts:561-566`); (b) the `load_memory` field itself is marked `secret: false` in the settings schema (`src/mocks/settings-handlers.ts:368`). Whether the agent excludes sensitive *content* when writing notes is not verifiable here — the write path lives outside this source.

## Architectural Decisions

1. **Capability-gate vs content-policy split.** The frontend owns only the enable/disable decision; the SDK owns extraction and storage. This is stated explicitly in the enrichment-boundary comment: profile launches rebuild the agent server-side, and "Persistent memory is NOT on that boundary: `load_memory` is a global user preference" applied like global `mcp_config` (`src/api/agent-server-adapter.ts:1085-1106`).
2. **Single global switch, not per-conversation or per-fact.** The toggle persists in shared `agent_settings` and applies to both OpenHands and ACP agents ("the stored flag rides the shared agent_settings record into ACP conversations too", `src/constants/settings-nav.tsx:38-44`).
3. **Diff-based persistence.** Saves emit minimal nested diffs (`agent_settings_diff`) rather than whole-record replacement (`src/hooks/mutation/use-save-settings.ts:34-47`), reducing clobber risk on concurrent edits.
4. **One credential channel.** Secrets travel exclusively via `request.secrets`; mirroring them onto `agent_context` was deliberately removed ("the agent_context drain is gone entirely in sdk#3528", `__tests__/api/agent-server-adapter.test.ts:561-566`).
5. **Schema-driven UI.** The Memory page renders whatever the backend schema exposes for `agent_context`, curated via `fields_opt_in` so "only load_memory is exposed, never the raw AgentContext model" (`src/mocks/settings-handlers.ts:349-352`). New SDK memory knobs would surface without frontend rewrites but also without frontend-level guardrails.

## Notable Patterns

- **Off-by-default opt-in as safety posture:** memory is absent from frontend defaults entirely (`src/services/settings.ts:35-52`) and schema-defaulted to `false` (`src/mocks/settings-handlers.ts:364`), both pinned by tests.
- **Test-pinned plumbing:** each hop of the flag's journey has a named test — rendering/default (`__tests__/routes/agent-context-settings.test.tsx:43-53`), save shape (`:74-94`), inline OpenHands launch (`__tests__/api/agent-server-adapter.test.ts:514-537`), inline ACP launch (`:539-559`).
- **Cross-repo test references in comments:** the adapter cites the SDK-side coverage contract (`tests/agent_server/test_agent_profile_conv_start.py`, `__tests__/api/agent-server-adapter.test.ts:519-523`), documenting where the unimplemented-in-this-repo behavior is actually verified.
- **Contrast between two "memory" systems:** persistent cross-session memory (`.openhands/memory/`, opt-in) vs in-session condensation (`CondensationEvent.forgotten_event_ids` + `summary`, `src/types/agent-server/core/events/condensation-event.ts:5-27`; condenser on by default, `src/services/settings.ts:18-19`). Forgetting is automatic and default-on; remembering is manual and default-off.

## Tradeoffs

- **Safety vs discoverability:** default-off plus buried settings page means most conversations never benefit from learned preferences, but also never leak state across sessions without consent.
- **Coarse control:** one global boolean gives users no middle ground (no per-project memory, no selective forgetting). Users who want codebase learning in project A but not project B cannot express it from this UI.
- **Delegation opacity:** because the write mechanics are server-side, the frontend can make no promises about fact quality, deduplication, or staleness of memories — it can only promise the switch works.
- **Curated schema exposure:** exposing only `load_memory` keeps the UI simple but means any richer SDK memory controls (scopes, retention limits) would need new curation work on both sides.

## Failure Modes / Edge Cases

- **Silent divergence between launch paths:** if the server-side stamping of `load_memory` onto profile-resolved agents regressed, inline launches would keep memory while profile launches silently lost it; the client deliberately does not re-send the flag on profile launches (`src/api/agent-server-adapter.ts:1104-1106`), so nothing client-side would catch the regression — coverage depends entirely on SDK tests.
- **Schema-absent fallback:** if the backend does not expose the `agent_context` section, the page renders a generic "schema unavailable" state and the toggle disappears (`__tests__/routes/agent-context-settings.test.tsx:114-132`; `src/components/features/settings/sdk-settings/sdk-section-page.tsx:632-643`) — older agent-servers simply have no memory control.
- **No revocation of already-written data:** disabling the toggle stops future loading/writing per the described contract, but this source offers no evidence of any purge of existing `.openhands/memory/` content; the notes presumably persist on disk.
- **Mock-schema drift risk:** the off-by-default behavior is asserted against the mock schema (`src/mocks/settings-handlers.ts:364`); if the real SDK changed its default to `true`, the UI would faithfully render whatever the server says — the frontend test would not fail.

## Future Considerations

- Expose memory inspection/correction UI (list/edit/delete entries under `.openhands/memory/`) — currently the only remedy is toggling the feature off globally.
- Per-workspace or per-profile memory scoping, building on the existing profile-resolution boundary (`src/api/agent-server-adapter.ts:1100-1106`).
- Surface write events (e.g., an event-stream notification when the agent records a new memory) so remembering becomes observable; today only condensation has event types (`src/types/agent-server/core/events/condensation-event.ts`).
- Add retention/sensitive-content policy signals to the settings schema so the frontend can display them alongside the toggle.

## Questions / Gaps

- **Extraction triggers:** what causes an individual fact to be written mid-session? Not in this source — lives in `software-agent-sdk`. No evidence found (searched `src/` for extractor/write-trigger patterns; none exist).
- **Fact verification & conflict resolution:** none implemented here; unverifiable under the isolation boundary.
- **Confidence metadata:** no confidence scores anywhere in the settings/event types (`src/types/` contains no memory-related models at all).
- **Per-write approval:** the only approval flow found is the capability-level settings Save (`sdk-section-page.tsx:718-732`); no per-memory confirmation exists.
- **Memory deletion semantics on disable:** unknown from this source.
- **Cross-check needed:** a companion study of `sources/software-agent-sdk` (if added as a source) would be required to score the write mechanics themselves; this report scores only what this frontend evidences.

---

Generated by `05.05-memory-write-policy` against `openhands`.
