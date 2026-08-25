# Source Analysis: openhands

## Working Memory and Scratchpad

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + Zustand (`@openhands/agent-canvas` 1.15.0 frontend; agent loop itself lives in the sibling `software-agent-sdk` repo) |
| Analyzed | 2026-08-25 |

## Summary

This source is the OpenHands **Agent Canvas UI** — the React/TypeScript frontend that observes an agent-server over a WebSocket event stream. It does not own the agent loop or LLM context assembly, so "working memory" here appears in two roles: (1) typed **protocol surfaces** through which agent-side working state (todo lists, plan files, logged thoughts, condensation events) is exposed, and (2) **client-side mirrors** (Zustand stores + localStorage) that reconstruct and display that state.

Four distinct working-memory surfaces exist:

1. **Task tracker todo state** — `TaskTrackerAction`/`TaskTrackerObservation` with `view|plan` commands carrying a full-replacement `task_list` of `{title, notes, status}` items (`src/types/agent-server/core/base/action.ts:127-136`, `src/types/agent-server/core/base/observation.ts:186-199`). The UI derives the current list by scanning the event store backwards for the latest `plan` observation (`src/hooks/use-task-list.ts:35-48`) and renders it both in a dedicated tab (`src/routes/task-list-tab.tsx:10-33`) and inline in chat.
2. **Planning scratchpad** — a dedicated planning sub-agent writes `PLAN.md` via a scoped file-editor tool (`PlanningFileEditorAction`, `src/types/agent-server/core/base/action.ts:218-226`). The frontend watches for `PlanningFileEditorObservation` events touching any `*.PLAN.MD` path and re-reads the file from the planning conversation into a `planContent` store field (`src/contexts/conversation-websocket-context.tsx:904-938`).
3. **Logged thoughts / reasoning** — `ThinkAction.thought` ("Your thought has been logged.", `src/types/agent-server/core/base/action.ts:21-25`, `src/types/agent-server/core/base/observation.ts:35-41`) plus streamed `reasoning_content` / `thinking_blocks` on action events (`src/types/agent-server/core/events/action-event.ts:13-25`), rendered as collapsed-by-default collapsible sections (`src/components/conversation-events/chat/event-message-components/collapsible-thinking.tsx:26-58`).
4. **Ephemeral client working state** — per-conversation optimistic message queues, draft persistence, pending attachments, goal status, all in small Zustand stores with explicit boundary-clearing semantics.

The defining design choice: **working memory is not hidden**. Task lists, plans, and thoughts are first-class user-visible artifacts rendered as UI surfaces, while context forgetting (condensation) is the one invisible mechanism — it arrives as protocol events the UI consumes but never renders.

## Rating

**7/10** — Clear model with typed interfaces, tests, and operational safeguards within its scope.

Strengths earning the score:
- Explicit, discriminated-union-typed surfaces for every working-memory artifact (`src/types/agent-server/core/base/action.ts:127-136`, `src/types/agent-server/core/base/common.ts:1-14`).
- Tested derivation logic with latest-wins semantics and edge cases (`__tests__/hooks/use-task-list.test.ts:55-190`).
- Deliberate stream separation so planning-agent output can never be misattributed to the main agent (`src/utils/handle-event-for-ui.ts:33-37`; dual delta batchers at `src/contexts/conversation-websocket-context.tsx:162-180`).
- Atomic conversation-boundary clearing with a documented invariant (`src/stores/use-event-store.ts:75-89, 216-222`).
- Full auditability: everything is a timestamped event; transcript export includes thoughts and task details (`src/utils/transcript-export/index.ts:64-99, 253-260`).

What keeps it below 8–10:
- This repo only *observes* working memory; durability, privacy policy, and scratchpad lifecycle live server-side and cannot be verified here.
- The PLAN.md → `planContent` mirroring path in `conversation-websocket-context.tsx:904-938` has no dedicated test asserting content propagation (the context test file covers session keys and error classification, `__tests__/contexts/conversation-websocket-context.test.tsx:299-354, 600`, but not plan-file mirroring).
- Condensation ("forgotten" events) is invisible to users in the chat UI — auditable at the API level but not surfaced as UI affordance.

## Evidence Collected

Every entry includes a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Todo item schema | `TaskItem {title, notes, status: "todo"\|"in_progress"\|"done"}` wire type | `src/types/agent-server/core/base/common.ts:1-14` |
| Todo write tool | `TaskTrackerAction.command: "view" \| "plan"` with full-list replacement `task_list` ("Always `view` the current list before making changes") | `src/types/agent-server/core/base/action.ts:127-136` |
| Todo read result | `TaskTrackerObservation {content, command, task_list}` — canonical list echoed back | `src/types/agent-server/core/base/observation.ts:186-199` |
| Todo derivation | `useTaskList()` scans events backwards, latest `command === "plan"` observation wins | `src/hooks/use-task-list.ts:35-48` |
| Todo tests | Latest-wins, `view` ignored, empty-list handling, reactivity — 7 cases | `__tests__/hooks/use-task-list.test.ts:55-190` |
| Todo visibility (user) | Dedicated tasklist tab renders derived list; empty-state when none | `src/routes/task-list-tab.tsx:10-33` |
| Todo visibility (chat) | Inline card rendered only for `plan` commands with non-empty lists | `src/components/conversation-events/chat/task-tracking/task-tracking-observation-content.tsx:12-22` |
| Todo grouping exemption | `TaskTrackerObservation` excluded from collapsible event groups so it stays visible | `src/components/conversation-events/chat/group-events.ts:47-49` |
| Plan scratchpad tool | `PlanningFileEditorAction` with `create/str_replace/insert/view/undo_edit`, path "typically workspace/project/PLAN.md" | `src/types/agent-server/core/base/action.ts:218-246` |
| Plan observation type | `PlanningFileEditorObservation` returns old/new content, error flag | `src/types/agent-server/core/base/observation.ts:201-230` |
| Planning sub-agent creation | Plan mode spawns sub-conversation `agentType: "plan"`, entryPoint `"plan_sub_conversation"` | `src/hooks/use-handle-plan-click.ts:57-70` |
| Sub-task id persistence | `subConversationTaskId` written to per-conversation localStorage to survive refresh mid-creation | `src/hooks/use-handle-plan-click.ts:31-38, 73-76` |
| PLAN.md mirroring | `isPlanFilePath()` matches any `PLAN.MD` suffix; live observations trigger `readConversationFile` → `setPlanContent` | `src/contexts/conversation-websocket-context.tsx:207-208, 904-938` |
| History replay rebuild | During planning-history replay only the latest planning-file observation is retained, then read once after load completes | `src/contexts/conversation-websocket-context.tsx:201-205, 477-501` |
| Plan file durability | PLAN.md read back through the file API at `/workspace/project/.agents_tmp/PLAN.md` | `src/api/conversation-service/agent-server-conversation-service.api.ts:656-667` |
| Build handoff prompt | Literal prompt "Execute the plan based on the .agents_tmp/PLAN.md file." sent to code mode | `src/hooks/use-handle-build-plan-click.ts:34-36` |
| Plan preview card | Chat-side truncated preview (300 chars, `plan-preview.tsx:23`) with View (`:45-47`) and Build (`:50-56`) actions | `src/components/features/chat/plan-preview.tsx:23-56` |
| Thought logging tool | `ThinkAction {thought: string}` — "The thought to log"; confirmation "Your thought has been logged." | `src/types/agent-server/core/base/action.ts:21-25`; `src/types/agent-server/core/base/observation.ts:35-41` |
| Extended reasoning fields | `reasoning_content?: string`, `thinking_blocks` on `ActionEvent` | `src/types/agent-server/core/events/action-event.ts:13-25` |
| Thought rendering | Collapsed-by-default collapsible section, expand on demand | `src/components/conversation-events/chat/event-message-components/collapsible-thinking.tsx:26-58` |
| Thought hoisting | Thoughts hoisted out of collapsed groups into their own render items, de-duplicated by action id | `src/components/conversation-events/chat/group-events.ts:114-118` |
| Stream attribution guard | `isFromPlanningAgent` flag keeps planning deltas from concatenating onto main-agent streams (#1656) | `src/utils/handle-event-for-ui.ts:33-37`; batchers at `src/contexts/conversation-websocket-context.tsx:162-180` |
| Event store invariant | Global store keyed by `loadedConversationId`; atomic clear+record boundary action | `src/stores/use-event-store.ts:55-90, 216-222` |
| Conversation-boundary reset | Switching conversations clears terminal, planContent, mode, sub-task id, agent status | `src/routes/conversation.tsx:74-87`; `src/stores/conversation-store.ts:352-362` |
| Optimistic message queue | Pending messages scoped by conversationId, 150 s watchdog, exact-content echo matching | `src/stores/optimistic-user-message-store.ts:14, 16-37, 131-142, 169-198` |
| Draft persistence | Composer draft saved/restored per-conversation across turns and reloads | `src/hooks/chat/use-draft-persistence.ts:99, 160-165` |
| Goal status mirror | Per-conversation goal status fed by goal state-update events | `src/stores/goal-store.ts:19-33` |
| One-shot attachment staging | Pending task attachments consumed exactly once per task id | `src/stores/pending-task-attachments-store.ts:20-40` |
| Context condensation protocol | `CondensationEvent.forgotten_event_ids` + optional summary of forgotten events | `src/types/agent-server/core/events/condensation-event.ts:5-27` |
| Condensation consumed, not shown | UI awaits condensation completion by scanning store events; no component renders Condensation kinds | `src/hooks/use-await-context-compaction.ts:112-138` (grep for `Condensation` in `src/components/` returns nothing) |
| User-triggered compaction | Manual compact action snapshots token baseline before POST | `src/hooks/use-compact-context-action.ts:84-110` |
| Auditability | Transcript export includes reasoning/thought narration and `TaskTrackerAction/Observation` details | `src/utils/transcript-export/index.ts:64-99, 253-260` |
| Agent-driven panel control | CanvasUI client tool lets the agent switch user-visible panels incl. planner/tasklist tabs | `src/api/canvas-ui-client-tool.ts:22-79` |

## Answers to Dimension Questions

1. **Does the agent keep private task state?**
   Yes, three kinds. (a) Structured todo state via the `task_tracker` tool whose authoritative list lives server-side and is echoed in `TaskTrackerObservation.task_list` (`src/types/agent-server/core/base/observation.ts:186-199`). (b) A free-form plan scratchpad: the planning sub-agent edits `.agents_tmp/PLAN.md` in its own workspace (`src/api/conversation-service/agent-server-conversation-service.api.ts:656-667`), isolated in a separate sub-conversation (`src/hooks/use-handle-plan-click.ts:57-70`). (c) Logged thoughts (`ThinkAction`, `src/types/agent-server/core/base/action.ts:21-25`). However, none of this is *hidden* state in the UI layer: the frontend derives all of it exclusively from the observable event stream (`src/hooks/use-task-list.ts:39-44`).

2. **Is it durable?**
   Server side, yes by construction: todo state and thoughts are persisted events in the conversation log, and the plan is a real workspace file — after reload the frontend replays planning history and re-reads PLAN.md once to rebuild `planContent` (`src/contexts/conversation-websocket-context.tsx:477-501`). Client-side mirrors are deliberately ephemeral: the event store is in-memory only (`src/stores/use-event-store.ts:153-158`) and cleared on conversation switch (`src/contexts/conversation-websocket-context.tsx:311`). Two client-side exceptions persist locally: composer drafts (`src/hooks/chat/use-draft-persistence.ts:99`) and the in-flight `subConversationTaskId` (`src/hooks/use-handle-plan-click.ts:73-76`), both keyed per conversation in localStorage.

3. **Is it exposed to users?**
   Yes — this is the design's core stance. Todo state has a dedicated tab plus inline chat cards (`src/routes/task-list-tab.tsx:19-33`, `src/components/conversation-events/chat/task-tracking/task-tracking-observation-content.tsx:17-22`); the plan renders as markdown in the planner tab (`src/routes/planner-tab.tsx:34-46`) with a chat preview card (`src/components/features/chat/plan-preview.tsx:23-56`); thinking is one click away behind a collapsed toggle (`src/components/conversation-events/chat/event-message-components/collapsible-thinking.tsx:26-58`). The single exception is **condensation**: forgotten-event notifications are consumed for token accounting (`src/hooks/use-await-context-compaction.ts:121-138`) but never rendered — the user sees neither which events were dropped nor the summary text.

4. **Does it pollute long-term memory?**
   No evidence of pollution. There is no long-term/user-memory layer in this repo at all; working notes stay inside the conversation event log and the `.agents_tmp/` plan file, while durable user preferences are stored separately under `misc_settings.app_preferences` (documented contract in `AGENTS.md`, implemented via `src/api/settings-service/`). Nothing found writes task/plan/thought content into settings or cross-conversation stores. Boundary hygiene supports this: `resetConversationState()` nulls `planContent` and mode on every conversation switch (`src/stores/conversation-store.ts:352-362`), and optimistic-message consumption is scoped by conversation id so a stale ack can never pop another conversation's entry (`src/stores/optimistic-user-message-store.ts:77-84`).

5. **Can it be audited?**
   Yes. Every artifact is a typed event with id/timestamp (`src/types/agent-server/core/base/event.ts`), including condensation events that name exactly which event ids were forgotten (`src/types/agent-server/core/events/condensation-event.ts:11-18`). The built-in transcript export reproduces thoughts/reasoning and task-tracker details into markdown/HTML (`src/utils/transcript-export/index.ts:64-99, 253-260`), and the dev-only `window.__OH_EVENT_STORE__` hook allows synthetic-event inspection (`src/stores/use-event-store.ts:229-237`). Caveat: auditability is at the data level; there is no UI view that diffs successive todo-list revisions or shows what condensation removed.

## Architectural Decisions

- **Working memory as event-sourced projection, not client-owned state.** The frontend never mutates todos or plans directly; it projects them from the append-only event stream (`src/hooks/use-task-list.ts:38-47`). Consequence: the UI is always a faithful read-only mirror, and correctness reduces to event ordering/dedup, which the store handles (`src/stores/use-event-store.ts:92-129`).
- **Full-replacement todo updates.** `TaskTrackerAction.plan` carries the entire `task_list`, not deltas (`src/types/agent-server/core/base/action.ts:133-135`), making latest-wins reconstruction trivially correct (`__tests__/hooks/use-task-list.test.ts:124-150`) at the cost of larger payloads and lost per-item edit history.
- **Separate process boundary for planning.** Planning runs in its own sub-conversation with its own WebSocket connection and session key (`__tests__/contexts/conversation-websocket-context.test.tsx:299-354`), and its events carry `isFromPlanningAgent` so streaming deltas can never merge across agents (`src/utils/handle-event-for-ui.ts:33-37`).
- **Scratchpad as a real file, not a hidden blob.** PLAN.md is an ordinary workspace file editable with normal editor semantics (`undo_edit` included, `src/types/agent-server/core/base/action.ts:222`), which makes the plan inspectable and hand-editable outside the UI.
- **Visibility-first thinking.** Rather than hiding chain-of-thought, the UI renders it collapsed-by-default (`collapsible-thinking.tsx:21`) and hoists it out of compressed event groups (`group-events.ts:114-118`) — trading screen space risk for transparency.
- **Session-only vs persisted state is an explicit decision matrix.** The right-panel drawer is intentionally session-only with legacy-key sanitization (`src/utils/conversation-local-storage.ts:33-45` note block), while drafts and sub-conversation task ids persist — each documented at its definition site.

## Notable Patterns

- **Backwards-scan reducer hook**: `useTaskList()` iterates the event array from the end and returns the first matching observation — a minimal "latest event of kind X wins" pattern (`src/hooks/use-task-list.ts:39-44`).
- **Deferred side-effect during replay**: history replay records only `{path, conversationId}` in a ref and performs a single file fetch after loading completes, avoiding N reads during resend-all replay (`src/contexts/conversation-websocket-context.tsx:912-917, 477-501`).
- **One-shot consume stores**: pending message echoes and task attachments are removed atomically upon consumption to prevent replay across conversations (`src/stores/optimistic-user-message-store.ts:169-198`; `src/stores/pending-task-attachments-store.ts:28-40`).
- **Watchdog timers for ephemeral state**: pending bubbles flip to error state after 150 s if no server echo lands (`src/stores/optimistic-user-message-store.ts:131-139`).
- **Group-breaker exemptions**: task trackers, think actions, and plan-file observations are excluded from collapsible action groups so high-signal working-memory cards never disappear inside summary rows (`src/components/conversation-events/chat/group-events.ts:29-49`).
- **Agent-controlled salience**: the CanvasUI client tool lets the agent decide when to surface the planner/tasklist panel to the user (`src/api/canvas-ui-client-tool.ts:22-27`) — working memory presentation is partly under agent control.

## Tradeoffs

- **Faithful mirroring vs. offline capability**: because all working state derives from the event stream, the tasklist/planner tabs are empty until history loads; there is no local cache of the last known todo list (`src/stores/use-event-store.ts:209-215` clears unconditionally).
- **Full-list replacement vs. audit granularity**: replacing `task_list` wholesale makes reconstruction simple but discards intermediate transitions; auditing how a task moved `todo → done` requires replaying all observations manually.
- **Collapsed thinking vs. accidental disclosure**: showing reasoning on demand maximizes transparency, but nothing in the UI redacts reasoning content; whatever the model emits is rendered verbatim as markdown (`collapsible-thinking.tsx:56-58`). Privacy depends entirely on upstream model/server policy, which this repo does not implement.
- **Invisible condensation vs. clean UX**: not rendering `forgotten_event_ids` keeps the chat simple, but users cannot see what the model no longer knows — a comprehension gap between what the user remembers discussing and what remains in the LLM view.
- **PLAN.md path matching by suffix**: `isPlanFilePath()` accepts any path ending in `PLAN.MD` (`src/contexts/conversation-websocket-context.tsx:207-208`) — permissive enough to survive path variations, loose enough that an unrelated `notes/PLAN.md` would overwrite the planner tab content.

## Failure Modes / Edge Cases

- **Stale plan across conversations**: `planContent` is reset on conversation switch (`src/routes/conversation.tsx:74-87`), but it is only rebuilt if the planning sub-conversation history replays a `PlanningFileEditorObservation`. If the plan file exists but the observation was condensed away server-side, the planner tab falls back to the empty state even though PLAN.md still exists on disk.
- **Read-back failure is silent-ish**: a failed `readConversationFile` logs a console warning only (`src/contexts/conversation-websocket-context.tsx:492-494, 927-932`); the user gets no error surface for a plan that failed to load.
- **Duplicate-event side-effect asymmetry**: replayed events skip non-idempotent side effects via the `eventIds` set check (`src/contexts/conversation-websocket-context.tsx:556-568, 788-801`); streaming deltas bypass this tracking entirely (by design, `src/stores/use-event-store.ts:94-97`), so correctness relies on the batchers being reset on conversation switch (`conversation-websocket-context.tsx:520-529`).
- **Hardcoded fallback id**: the planning-action cache invalidation uses `"test-conversation-id"` as a fallback with an acknowledged TODO (`src/contexts/conversation-websocket-context.tsx:859-861`) — a latent bug if sub-conversations are absent during a planning event.
- **Optimistic-bubble mismatch**: if the server munges message content beyond whitespace, echo matching falls back to dropping the oldest sending entry, which can consume the wrong bubble when several messages are in flight (`src/stores/optimistic-user-message-store.ts:171-188`).

## Future Considerations

- Add a dedicated test asserting PLAN.md content propagation through `handlePlanningMessage` → `setPlanContent` (currently only session-key and telemetry behavior of the planning socket are tested, `__tests__/contexts/conversation-websocket-context.test.tsx:299-354, 600`).
- Surface condensation to users: a subtle indicator of "N earlier messages summarized" would close the gap between user memory and the model's actual view, leveraging already-available `forgotten_event_ids` (`src/types/agent-server/core/events/condensation-event.ts:11-18`).
- Persist last-known todo list per conversation so the tasklist tab renders instantly before history replay finishes.
- Tighten `isPlanFilePath` to the canonical `.agents_tmp/PLAN.md` constant (AGENTS.md already mandates reusing `DEFAULT_WORKING_DIR` instead of hardcoding `/workspace/project`, `src/api/agent-server-conversation-service.api.ts:656-659` follows this; the suffix matcher does not).
- Replace the `"test-conversation-id"` fallback flagged with TODO at `src/contexts/conversation-websocket-context.tsx:861`.

## Questions / Gaps

- **Server-side scratchpad policy unverifiable here.** Whether the planning sub-agent's PLAN.md is ever garbage-collected, whether `ThinkAction` contents count toward context windows, and whether condenser summaries are themselves persisted are all decided in the sibling `software-agent-sdk` repo, outside this study's isolation boundary. Searched this repo for condenser rendering/policy (`grep -rn "Condensation" src/components src/hooks`); only the await-compaction consumer exists (`src/hooks/use-await-context-compaction.ts`).
- **No redaction layer found.** No code path masks secrets within thoughts, task notes, or plan content before rendering; secret hygiene relies on the separate secrets API never exposing values (`GET /api/settings/secrets/:name` pattern documented in AGENTS.md). No evidence of a leak *mechanism*, but equally no defense-in-depth at the rendering layer — searched `src/components/conversation-events/**` for mask/redact utilities; none found.
- **Cross-turn task persistence semantics** (does the server re-seed the todo list into the LLM view on later turns?) cannot be answered from the frontend; the wire types imply yes (`view` command exists precisely to re-read state, `src/types/agent-server/core/base/action.ts:129-131`), but this is inference from interface shape, not observed behavior.

---

Generated by `Dimension 05.02: Working Memory and Scratchpad` against `openhands`.
