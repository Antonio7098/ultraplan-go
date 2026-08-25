# Source Analysis: openhands

## 11.01 Context Selection Policy

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Vite, Zustand, TanStack Query; agent-server access via `@openhands/typescript-client` |
| Analyzed | 2026-08-25 |

## Summary

This source is the OpenHands **frontend** ("Agent Canvas"). The LLM context assembly proper — building the message list, running the condenser, matching skill triggers — lives in the sibling Python SDK/agent-server (out of scope for this study). Within this repo, the **context selection policy is a boundary policy**: the frontend decides what enters the model's world at conversation start and what is appended to messages on the way in, then delegates history-view management to the backend through explicit configuration.

Concretely, the policy has five planes:

1. **Conversation-start envelope** — one pure builder, `buildStartConversationRequest` (`src/api/agent-server-adapter.ts:1050`), assembles everything the model will ever "know" up front: merged skill catalog filtered by a user deny-list (`buildAgentContext`, `src/api/agent-server-adapter.ts:749-788`), tool availability gates (`getAgentTools`, `src/api/agent-server-adapter.ts:646-677`), workspace/working-dir binding (`src/api/agent-server-adapter.ts:974-990`), an initial user message composed from query + instructions (`buildInitialMessage`, `src/api/agent-server-adapter.ts:679-695`), and a runtime-topology block injected as a system-prompt suffix (`buildRuntimeServicesSystemSuffix`, `src/api/agent-server-adapter.ts:215-300`).
2. **Per-message augmentation** — uploaded files are appended into the prompt text and images embedded as base64 content blocks (`src/utils/send-message-with-attachments.ts:50-79`), gated by hard size limits (`src/utils/file-validation.ts:1-2`). Slash-command interceptors (`/model`, `/goal`, `/btw`) divert input *out* of the model stream entirely (`src/components/features/chat/interactive-chat-box.tsx:47-56`).
3. **UI→model injection channel** — client tools have no result channel, so the UI writes structured JSON results back into the parent conversation as synthetic `role:"user"` messages (`src/services/child-conversation-launch.ts:488-496`), which the chat renderer then hides by string-prefix matching (`shouldRenderEvent`, `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:45-51`).
4. **Policy knobs delegated to the backend** — condenser enable/max-size (`src/services/settings.ts:18-19`, schema-driven screen at `src/routes/condenser-settings.tsx:3-12`), persistent memory (`agent_context.load_memory`, surfaced in `src/mocks/settings-handlers.ts:349-369`), per-skill disable list persisted server-side (`src/routes/skills-settings.tsx:91-103`).
5. **Observability + manual override** — the full system prompt including runtime-injected `dynamic_context` is inspectable via the system-prompt modal (`adaptSystemMessage`, `src/utils/system-message-adapter.ts:13-35`); the context-window meter shows live token usage with warning/danger tones (`src/components/features/chat/components/context-window-meter.tsx:55-63`); and the user can force compaction via `POST /api/conversations/{id}/condense` with verified feedback (`useCompactContextAction`, `src/hooks/use-compact-context-action.ts:80-111`; `condenseConversation`, `src/api/conversation-service/agent-server-conversation-service.api.ts:717-745`).

Secret handling cuts across all planes: secrets never travel inline (LookupSecret indirection, `src/api/agent-server-adapter.ts:1203-1228`), settings round-trip Fernet-encrypted under a tri-mode exposure header (`src/api/settings-service/settings-service.api.ts:115-122,486-512`), and redaction filters guard the *display* of secret-bearing context (`src/utils/redact-custom-secrets.ts:8-31`, `src/utils/redact-mcp-secrets.ts:110-130`).

**Scope caveat:** because this repo owns only the boundary, "why was X included" is answerable here only for skills (declared keyword triggers plus per-message `activated_skills` metadata) and explicit attachments — not for backend-side history-view or condensation decisions.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Explicit interfaces**: every inclusion decision at the boundary flows through typed, named builders (`StartConversationPayload`, `src/api/agent-server-adapter.ts:1002-1023`; `BundledSkill`, `src/api/agent-server-adapter.ts:703-712`) rather than ad-hoc string assembly.
- **Tests**: disabled-skill exclusion for both OpenHands and ACP launch paths (`src/api/agent-server-adapter.test.ts:131-178`), `secrets_encrypted` gating including the ACP exception (`src/api/agent-server-adapter.test.ts:40-129,228-258`), dynamic-context redaction (`__tests__/utils/system-message-adapter.test.ts:59-76`), and end-to-end compaction outcome reporting (`src/hooks/use-compact-context-action.test.tsx:78-104`).
- **Operational safeguards**: tri-mode secret exposure with fail-hard semantics (`getSettingsForConversation` refuses to fall back to redacted settings, `src/api/settings-service/settings-service.api.ts:500-512`), attachment validation before upload, replay ledgers for non-idempotent client-tool calls (`claimToolCall`, `src/services/child-conversation-launch.ts:205-227`).
- **Not higher (8+)** because: several classification mechanisms are brittle text-prefix matches that the code itself flags as such (`should-render-event.ts:20-22`: "Brittle by design"); the profile launch path silently drops client-side context enrichments (documented gap at `src/api/agent-server-adapter.ts:1089-1098`, tracked in software-agent-sdk#3967); and the core selection machinery (history view, condensation execution) is not implemented here, so its policy cannot be audited from this source.
- **Not lower (≤6)** because the safeguards and observability are real, tested, and load-bearing, not decorative.

## Evidence Collected

Every entry cites `path/to/file:NN` relative to `studies/agent-harness-study/sources/openhands`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Context builder (start payload) | `buildStartConversationRequest` assembles agent settings, workspace, confirmation policy, tools, initial message, tags, secrets | src/api/agent-server-adapter.ts:1050-1231 |
| Agent context assembly | `buildAgentContext` merges bundled skills + existing skills, applies deny-list, sets `load_public_skills:false` / `load_user_skills:true` / `load_project_skills:true` | src/api/agent-server-adapter.ts:749-788 |
| Skill packaging | `buildBundledSkills` converts build-time `SKILLS_CATALOG` into SDK `Skill` JSON with `{type:"keyword", keywords}` triggers; trigger-less skills become always-active (`trigger:null`) | src/api/agent-server-adapter.ts:703-747 |
| System-prompt suffix | `buildRuntimeServicesSystemSuffix` renders the `<RUNTIME_SERVICES>` block attached as `AgentContext.system_message_suffix` so the agent trusts listed service URLs over guessing | src/api/agent-server-adapter.ts:215-300 (attach at 784-786) |
| Initial message composition | `buildInitialMessage` joins trimmed query + conversation instructions with `\n\n` into a single `role:"user", run:true` message | src/api/agent-server-adapter.ts:679-695 |
| Tool inclusion policy | `shouldIncludeTool` gates `browser_tool_set` behind env var + server-advertised `usable_tools`, `task_tool_set` behind `enable_sub_agents === true`; defaults terminal/file_editor/task_tracker | src/api/agent-server-adapter.ts:113-119, 631-677 |
| Message augmentation (files/images) | Uploaded file paths appended to prompt text via i18n title; images embedded as base64 image content blocks | src/utils/send-message-with-attachments.ts:50-79 |
| Attachment limits | 3 MB per-file and 3 MB total caps enforced pre-upload | src/utils/file-validation.ts:1-2,58-69 |
| Input diversion (interceptors) | `/model` → `/goal` → `/btw` → submit chain; intercepted commands never reach model context; `/btw` routes to ask-agent side-channel | src/components/features/chat/interactive-chat-box.tsx:47-56; src/hooks/chat/use-btw-interceptor.ts:21-38; src/hooks/chat/use-goal-interceptor.ts:27-61 |
| UI→model result injection | Child-conversation launch results posted back as synthetic `role:"user"` JSON message (the only way a client tool can inform the agent) | src/services/child-conversation-launch.ts:459-497 |
| Display-vs-context split | `shouldRenderEvent` hides goal-loop re-prompts and `[child-conversation]` payloads from chat while they remain in model context; prefix matching self-described as "brittle by design" | src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:15-51,105-112 |
| Secret transport policy | Every saved secret rides as `LookupSecret` pointing back at `/api/settings/secrets/{name}` with auth headers; values never inline | src/api/agent-server-adapter.ts:995-1000,1203-1228 |
| Encrypted settings round-trip | `X-Expose-Secrets: encrypted` fetch for conversation start; plaintext mode documented as backend-only; refusal to start conversations on redacted credentials | src/api/settings-service/settings-service.api.ts:115-122,421-512 (fail-hard at 500-512) |
| Encryption detection | Fernet `gAAAAA` prefix sniffing decides `secrets_encrypted:true`, with ACP exception when no encrypted MCP values present | src/api/agent-server-adapter.ts:540,572-591,1147-1159 |
| Display redaction backstop | `redactCustomSecrets` masks `<CUSTOM_SECRETS>` key/value lines (closing tag optional) before showing dynamic context | src/utils/redact-custom-secrets.ts:8-31; wired at src/utils/system-message-adapter.ts:24-27 |
| MCP credential scrubbing | Known config values + generic token shapes (GitHub/Slack/Linear/JWT/Bearer) stripped from probe error text | src/utils/redact-mcp-secrets.ts:18-31,91-130 |
| Redaction tests | Asserts `MY_API_KEY=<secret-hidden>` substitution in modal content | __tests__/utils/system-message-adapter.test.ts:59-76 |
| Condenser knobs | Defaults `enable_default_condenser:true`, `condenser_max_size:240`; nested `condenser.enabled/max_size` mirrored to flat settings | src/services/settings.ts:18-19,42-45; src/hooks/query/use-settings.ts:98-103; sync at src/api/settings-service/settings-service.api.ts:395-400 |
| Schema-driven condenser screen | Route renders server-provided `condenser` section fields (descriptions: "summarize long conversation histories", max tokens kept) | src/routes/condenser-settings.tsx:3-12; field schema sample at src/mocks/settings-handlers.ts:312-346 |
| Persistent-memory knob | Curated `agent_context.load_memory` section exposed to users; noted as surviving profile launches (server-stamped global preference) | src/mocks/settings-handlers.ts:348-369; src/constants/settings-nav.tsx:40; comment at src/api/agent-server-adapter.ts:1100-1106 |
| Per-skill deny-list UI | Toggle set auto-saved as `disabled_skills` app preference (wholesale-replaced list) | src/routes/skills-settings.tsx:85-103; merge semantics at src/api/settings-service/settings-service.api.ts:67-76 |
| Skill exclusion tests | Disabled custom skills and bundled skills removed from both OpenHands and ACP contexts | src/api/agent-server-adapter.test.ts:131-178 |
| Manual compaction trigger | User-invoked compact action (Usage panel CTA + composer popover) posts `/condense`, disabled while agent runs; baseline event-id snapshot captured *before* request | src/hooks/use-compact-context-action.ts:22-119; consumers at src/components/features/chat/components/context-window-meter.tsx:133-158 and src/components/features/conversation/usage-panel/compact-context-button.tsx:30 |
| Compaction verification | Waits for a real `Condensation` event (type-guarded) plus a drop in `per_turn_token`; outcomes `compacted`/`no_change`/`timeout`, 90 s timeout | src/hooks/use-await-context-compaction.ts:6-24,57-164; type guard at src/types/agent-server/type-guards.ts:284-289 |
| Condensation event contract | `CondensationEvent.forgotten_event_ids` documents events "removed from the View given to the LLM"; optional summary + offset | src/types/agent-server/core/events/condensation-event.ts:14-37 |
| Context usage observability | Context-window ring computes % used of `contextWindow` with neutral/warning/danger tones and token counts | src/components/features/chat/components/context-window-meter.tsx:55-63,94-181 |
| Why-was-this-included signal | `MessageEvent.activated_skills: string[]` + `extended_content` record which skills fired and what they added; rendered as "skill ready" affordance | src/types/agent-server/core/events/message-event.ts:11-24; render gate at src/components/conversation-events/chat/event-message.tsx:58-87 |
| History retrieval (UI-side) | REST-first tail page of 50 newest events (`TIMESTAMP_DESC`), scroll-up pagination via `timestamp__lt`, WebSocket `resend_mode:'since'` anchored at latest preloaded timestamp | src/hooks/query/use-conversation-history.ts:10,21-28,43-72; src/hooks/use-load-older-events.ts:125-134; src/contexts/conversation-websocket-context.tsx:966-973 |
| Event retrieval API | `searchEvents` supports limit/sort_order/page_id/timestamp gte-lt; cloud path clamps limit to 100 | src/api/event-service/event-service.api.ts:102-175 |
| Bounded prompt truncation | Automation debug prompt keeps only the tail 4000 chars of error output ("most useful part is the tail") | src/utils/automation-debug-prompt.ts:14-21 |
| Model-driven sub-context isolation | `launch_child_conversation` requires self-contained briefs: "the child conversation cannot see this one"; worktree/shared isolation modes | src/services/child-conversation-launch.ts:126-135,296 |
| Client tool surface (model→UI) | `canvas_ui_control` lets the model drive which artifact panel the user sees (does not change its own context) | src/api/canvas-ui-client-tool.ts:22-91 |

## Answers to Dimension Questions

**1. What decides what goes into context?**
A deliberate two-plane split. The frontend owns the *envelope*: skills (bundled catalog + user/project skills minus the deny-list), tool specs, workspace binding, initial message, and the `<RUNTIME_SERVICES>` system-prompt suffix — all assembled in `buildStartConversationRequest` (`src/api/agent-server-adapter.ts:1050-1231`). The backend owns the *history view*: the frontend never constructs or trims the LLM message list; it only observes condensation outcomes (`forgotten_event_ids`, `src/types/agent-server/core/events/condensation-event.ts:14-16`) and can request compaction (`POST .../condense`, `src/api/conversation-service/agent-server-conversation-service.api.ts:717-745`). Per-turn, the frontend adds uploaded-file paths and base64 images onto the user message (`src/utils/send-message-with-attachments.ts:63-79`) and injects client-tool results as user-role JSON (`src/services/child-conversation-launch.ts:488-496`).

**2. Is selection policy explicit or implicit?**
Explicit at the frontend boundary and implicit beyond it. Inclusion decisions are named functions with boolean gates (`shouldIncludeTool`, `src/api/agent-server-adapter.ts:631-644`), declared flags (`load_public_skills:false`, `load_user_skills:true`, `load_project_skills:true`, `src/api/agent-server-adapter.ts:781-783`), and persisted user preferences (`disabled_skills`, `enable_default_condenser`, `condenser_max_size`). There is no single declarative policy document; the policy is distributed across builders, settings defaults, and schema descriptions served by the backend (`src/mocks/settings-handlers.ts:312-369` mirrors the shape). Two implicit spots exist: string-prefix classification of synthetic messages (`shouldRenderEvent`) and the enrichment asymmetry between inline-agent and profile launches (`src/api/agent-server-adapter.ts:1089-1098`).

**3. Can the model influence what it sees?**
Only indirectly. It cannot edit its own history view or condenser policy. What it *can* do: (a) spawn child conversations via `launch_child_conversation` whose outcomes return into the parent's context as user messages (`src/services/child-conversation-launch.ts:488-496`), effectively asking for more context by delegation — but children are explicitly told they cannot see the parent (`:126-135`); (b) pull data through server-executed tools (terminal/file_editor/browser) whose observations enter context; (c) steer what the *user* sees via `canvas_ui_control` (`src/api/canvas-ui-client-tool.ts:22-58`), which changes display, not context. Conversely, the **user** has direct influence: force compaction (`src/hooks/use-compact-context-action.ts:80-111`), toggle condenser/memory/skills in settings, and attach repos/workspaces/plugins that reshape the next conversation's envelope.

**4. Are sensitive fields redacted?**
Yes in transport and display — no, deliberately, in the model's own context. Transport: secrets are never placed in prompts or messages; they ride as `LookupSecret` references resolved server-side at spawn time (`src/api/agent-server-adapter.ts:1203-1228`), and settings round-trip as Fernet ciphertext (`secrets_encrypted`, `src/api/agent-server-adapter.ts:1147-1159`) fetched under `X-Expose-Secrets: encrypted` (`src/api/settings-service/settings-service.api.ts:486-512`); display defaults to `"**********"` masking (`:115-122`). Display backstops: `<CUSTOM_SECRETS>` blocks in the system-prompt modal are masked even if backend masking regresses (`src/utils/redact-custom-secrets.ts:1-7`, tested at `__tests__/utils/system-message-adapter.test.ts:59-76`), and MCP credentials/token shapes are scrubbed from error text (`src/utils/redact-mcp-secrets.ts:104-130`). However, the model itself receives resolved secret values — e.g., the SDK injects a `<CUSTOM_SECRETS>` block into `SystemPromptEvent.dynamic_context` (`src/types/agent-server/core/events/system-event.ts:21-25`); the frontend redaction is presentation-only and does not filter what the LLM sees. No evidence of redaction applied to outgoing model-bound content was found (and none would be coherent given the LookupSecret design).

> Can the system explain why a particular document was included in context?
Partially. For skills: yes — triggers are declared data (`{type:"keyword", keywords}`, `src/api/agent-server-adapter.ts:723-728`) and each message records `activated_skills` plus the `extended_content` the activation added (`src/types/agent-server/core/events/message-event.ts:11-19`), surfaced in the chat UI. For attachments: yes — inclusion is traceable to an explicit user action reflected in the prompt text (`CHAT_INTERFACE$AUGMENTED_PROMPT_FILES_TITLE`, `src/utils/send-message-with-attachments.ts:63`). For history items retained vs forgotten: the `CondensationEvent` lists forgotten ids and summary (`src/types/agent-server/core/events/condensation-event.ts:14-26`), giving post-hoc visibility. For backend retention/selection heuristics themselves: no evidence available in this source.

## Architectural Decisions

- **Boundary-builder pattern**: one pure function owns the entire conversation-start context payload (`src/api/agent-server-adapter.ts:1050`), making inclusion auditable and unit-testable without network mocks (`src/api/agent-server-adapter.test.ts:39-259`).
- **Frontend as sole distributor of public skills**: bundled `SKILLS_CATALOG` is shipped in `agent_context.skills` with `load_public_skills:false`, eliminating the server's extensions-repo clone; migration notes document removing the old env-var escape hatch (`src/api/agent-server-adapter.ts:771-781`).
- **Secrets-by-reference**: `LookupSecret` indirection means the browser holds names, not values, at launch time; uniform for ACP and non-ACP (`src/api/agent-server-adapter.ts:1203-1228`), with off-loop resolution guaranteed server-side (comment `:1205-1207`).
- **Fail-hard credential fetching**: conversation start refuses to proceed with redacted/display settings rather than launching an agent doomed to auth-fail (`src/api/settings-service/settings-service.api.ts:500-501`).
- **Client-tool results as user messages**: because the agent-server acks client tools before the browser acts, a follow-up `sendMessage` is the only channel to hand results to the agent — chosen knowingly, with the renderer compensating by hiding machine payloads from the chat (`src/services/child-conversation-launch.ts:454-458`; `should-render-event.ts:45-51`).
- **Delegated compression with verified feedback**: compaction is a backend operation, but the UX closes the loop by correlating the `Condensation` event with a measured `per_turn_token` drop instead of trusting the HTTP ack (`src/hooks/use-await-context-compaction.ts:57-60`).
- **Schema-driven policy surfaces**: condenser/memory/verification knobs are rendered generically from the server's settings schema (`SdkSectionPage`, `src/routes/condenser-settings.tsx:5-10`), keeping policy vocabulary owned by the backend while the frontend supplies defaults and normalization (`src/services/settings.ts:18-19`; `src/hooks/query/use-settings.ts:98-103`).

## Notable Patterns

- **Display/context decoupling**: three distinct filters govern what the *user* sees versus what the *model* sees — REST pagination tail-first (`INITIAL_HISTORY_PAGE_SIZE = 50`, `src/hooks/query/use-conversation-history.ts:10`), render filtering (`shouldRenderEvent`), and grouping/collapsing (`group-events`). None affect model context; the system-prompt modal exists precisely to expose the delta (`src/utils/system-message-adapter.ts:13-35`).
- **Interceptor chain for input routing**: composable wrappers divert commands out of the model stream before any send occurs (`src/components/features/chat/interactive-chat-box.tsx:47-56`), including a home-page variant that prevents `/model NAME` becoming the first message of a new conversation (`src/components/features/home/home-chat-launcher.tsx:214-218`).
- **Defensive redaction layering**: known-value replacement first (longest-first to avoid partial overlaps), then generic token-pattern sweeps, with minimum-length thresholds to avoid mangling ordinary words (`src/utils/redact-mcp-secrets.ts:9,92-101,122-128`).
- **Bounded injection**: free-form error text is truncated to its tail before entering a prompt (`MAX_ERROR_CHARS = 4000`, `src/utils/automation-debug-prompt.ts:14-21`); attachment volume capped at 3 MB total (`src/utils/file-validation.ts:1-2`).
- **Replay protection for context-writing side effects**: a localStorage ledger claims `tool_call_id`s before acting so socket replays cannot double-inject launch-result messages or double-launch billable Cloud conversations (`claimToolCall`, `src/services/child-conversation-launch.ts:196-227`).

## Tradeoffs

- **Envelope completeness vs. launch-path divergence**: inline-agent launches get Canvas enrichments (runtime-services suffix, bundled skills); profile launches resolve server-side and currently do NOT restore them — an accepted, tracked gap (software-agent-sdk#3967) traded for server-side profile authority (`src/api/agent-server-adapter.ts:1089-1098`). A mitigation exists only for the seeded `default` profile, which downgrades to agent_settings specifically to preserve enrichments (`src/hooks/mutation/use-create-conversation.ts:142-160`).
- **User-message result channel vs. clean transcripts**: injecting JSON as user messages guarantees delivery but pollutes persisted history; the cleanup relies on fragile prefix matching that must be kept in sync with SDK prompt texts (`should-render-event.ts:20-22` admits "the durable fix is a persisted goal-loop flag").
- **Bundled skills' freshness vs. latency**: build-time catalog removes clone latency and nondeterminism but freezes public skills until a dependency bump + rebuild (`src/api/skills-service.ts:27-33`).
- **Encrypted round-trip vs. blast radius**: fetching cipher-encrypted settings into the browser enables seamless starts without plaintext exposure, but requires the Fernet-sniffing heuristic (`gAAAAA` prefix, `src/api/agent-server-adapter.ts:540,572-583`) to decide when to force decryption server-side — format-coupled.
- **Manual compaction power vs. misuse**: the compact button is disabled while the agent runs (`src/hooks/use-compact-context-action.ts:35-39`), but a user can still compact mid-task between turns; outcome reporting distinguishes "no_change" from failure so misleading success toasts are avoided (`src/hooks/use-await-context-compaction.ts:17-23`).

## Failure Modes / Edge Cases

- **Redacted-value leakage into new conversations is structurally blocked, but stale-cache risk remains**: encrypted settings are cached 5 minutes (`CACHE_TTL_MS`, `src/api/settings-service/settings-service.api.ts:175`); a secret rotated elsewhere within TTL launches with old ciphertext (decrypts fine, wrong credential).
- **Prefix-match misclassification**: if the SDK ever changes goal-loop re-prompt wording or a user legitimately types a message starting with `[child-conversation] `, it would be hidden from (or shown in) the chat incorrectly — context unchanged, but the user's view diverges silently (`should-render-event.ts:23-26,45-51`).
- **Fast-condensation race**: the HTTP ack may arrive after the `Condensation` event already fired; baseline event ids are therefore snapshotted *before* the POST (`src/hooks/use-compact-context-action.ts:83-88`), and the await hook handles the "landed early" case explicitly (`src/hooks/use-await-context-compaction.ts:141-147` test).
- **Cloud pagination degradation**: older-event backfill degrades to a no-op on unpatched cloud backends lacking the timestamp-comparison fix rather than surfacing errors (`src/hooks/use-load-older-events.ts:35-39`) — the UI shows fewer events than exist, with only a doc-comment trace.
- **Duplicate-initial-message hazard**: when attachments exist, the query is withheld from the create call and sent after navigation; a regression here would double-post the first turn (guarded by comment + code, `src/components/features/home/home-chat-launcher.tsx:97-103`).
- **Scratch-workspace worktree failures**: child launches fall back from worktree to shared isolation, transparently telling the agent (via the injected result) that the child may now conflict with the parent's files (`SHARED_FALLBACK_CONSEQUENCE`, `src/services/child-conversation-launch.ts:269-270,303-323`).
- **Profile-path context loss is silent to users**: a named-profile launch simply lacks the runtime-services suffix; nothing warns the user or the agent (documented tradeoff only, `src/api/agent-server-adapter.ts:1089-1098`).

## Future Considerations

- Replace string-prefix classification of goal-loop re-prompts and child-results with a persisted marker on the event itself (already identified as the durable fix at `should-render-event.ts:21-22`).
- Close the profile-path enrichment gap (toolset + public skills + runtime-services suffix on profile launches) once software-agent-sdk#3967 lands, and consider a UI hint when launching a profile that drops enrichments.
- Extend the "why included" audit trail beyond skills: a per-inclusion provenance record (trigger, source, requesting actor) would let the system fully answer the dimension's guiding question for arbitrary documents, not just skills.
- Surface `CondensationEvent.summary` / `forgotten_event_ids` somewhere in the UI (currently consumed only for compaction verification, `src/hooks/use-await-context-compaction.ts:121-124`); the data contract already supports a "what did the model forget" inspector.
- Consider replacing Fernet-prefix sniffing with an explicit server flag for whether fetched settings contain ciphertext, decoupling the encryption decision from token format.

## Questions / Gaps

- **No evidence found (out of source scope)** for how the backend selects which prior events remain in the LLM View between condensations, or the condenser's internal triggering threshold — these live in `OpenHands/software-agent-sdk`. Searched this repo for condenser logic beyond settings plumbing (`grep -r "condenser"` across `src/`): only defaults, schema mirroring, and the compaction trigger/verification path exist here.
- **No evidence found** for retrieval-augmented generation (vector search, document ranking) anywhere in this repository — context acquisition is exclusively user-attached files/images, skills, and agent tool use. Search boundary: `src/` for `retriev|rag|embedding|vector` patterns returned no implementation hits (only unrelated matches like `retrieveAxiosErrorMessage`).
- Whether the backend masks `<CUSTOM_SECRETS>` values inside `dynamic_context` itself could not be verified from this source; the frontend treats backend masking as expected-but-defended-against ("in case backend masking regresses", `src/utils/redact-custom-secrets.ts:4-6`), implying the model sees unmasked values by design.
- The `title_llm_profile` / autotitle path spawns a separate LLM call whose inputs are not inspectable from the frontend (`autotitle: true`, `src/api/agent-server-adapter.ts:1127`); its prompt-selection policy is server-owned.

---

Generated by `11.01-context-selection-policy` against `openhands`.
