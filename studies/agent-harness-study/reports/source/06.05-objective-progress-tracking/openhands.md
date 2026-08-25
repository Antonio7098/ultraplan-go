# Source Analysis: openhands

## Objective and Progress Tracking

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (Agent Canvas) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, TanStack Query, WebSocket event stream against `@openhands/typescript-client` (agent-server) |
| Analyzed | 2026-08-25 |

**Scope note.** This repository is the OpenHands *frontend* only (`AGENTS.md` "Repository Map": this repo is "only the agent-canvas frontend"). The `/goal` loop engine and its judge run in the sibling `software-agent-sdk` agent-server, which is outside the isolation boundary of this study. Everything below therefore describes how goals, progress, and completion are **represented, transported, verified, and surfaced** in the harness UI layer, plus what the frontend's contracts reveal about the server-side model.

## Summary

OpenHands Canvas tracks objectives at three nested levels, each with an explicit wire type:

1. **The `/goal` loop — a first-class goal object.** A user submits `/goal [--max N] <objective>` in chat; the frontend intercepts it and calls `startGoal` on the agent-server (`src/hooks/chat/use-goal-interceptor.ts:21-65`, `src/hooks/mutation/conversation-mutation-utils.ts:83-92`). Progress streams back as `ConversationStateUpdateEvent`s with `key: "goal"` carrying a `GoalStatus` payload — `active`, `status ∈ {running, complete, capped, interrupted}`, `iteration`/`max_iterations` rounds, the verbatim `objective`, and a judge `verdict {score, complete, missing}` (`src/types/agent-server/core/events/conversation-state-event.ts:66-97`). Completion is measured by a **separate audit/judge**, not by the working agent's own claim.
2. **Task-level decomposition via the `task_tracker` tool.** The agent maintains a task list with per-item `status: "todo" | "in_progress" | "done"` (`src/types/agent-server/core/base/common.ts:1-14`) through `TaskTrackerAction` view/plan commands (`src/types/agent-server/core/base/action.ts:127-136`) whose observations return the full list (`src/types/agent-server/core/base/observation.ts:186-199`). This list is entirely model-authored and self-reported.
3. **Step-level progress via the action→observation event stream.** Every pending `ActionEvent` card is replaced in place by its `ObservationEvent` when it arrives (`src/utils/handle-event-for-ui.ts:432-441`), runs of actions are folded into collapsible groups showing "N/M actions completed" with a live spinner (`src/components/conversation-events/chat/event-message-components/event-group.tsx:100-105`), and a coarse `ExecutionStatus` enum (idle/running/paused/waiting_for_confirmation/finished/error/stuck) drives global status surfaces (`src/types/agent-server/core/base/common.ts:67-75`).

Completion is explicit and honest about failure: terminal goal states distinguish `complete` from `capped` (iteration budget exhausted) and `interrupted`, rendered as green check vs muted cross (`src/components/features/chat/goal-status-content.tsx:146-159`, tests at `__tests__/components/chat/goal-status-content.test.tsx:26-53`). Progress is richly observable in the UI: bottom-pinned live banner, inline terminal status, browser-tab emoji, notification sounds, sidebar status dots, and a cost/context usage panel.

The main weakness is **truthfulness**: task-tracker progress and ordinary `FINISHED` states are self-reported by the model with no runtime independent verification; only the goal loop has an independent (LLM) judge, and even that verdict is trusted as-is by the frontend.

## Rating

**7 / 10.**

- **Why this high:** explicit typed goal objects with judge verdicts (`GoalVerdict.score/complete/missing`, `src/types/agent-server/core/events/conversation-state-event.ts:68-97`); discriminated-union event types plus type guards (`src/types/agent-server/type-guards.ts:235-237`); distinct failure terminus (`capped`) instead of silent give-up; dedicated unit/component tests for goal store, banner, content, interceptor, and task list (`__tests__/stores/goal-store.test.ts`, `__tests__/components/chat/goal-status-content.test.tsx`, `__tests__/hooks/chat/use-goal-interceptor.test.ts`, `__tests__/hooks/use-task-list.test.ts`); multi-surface observability (banner, tab title emoji `src/utils/agent-state-emoji.ts:14-32`, sounds `src/hooks/use-agent-notification.ts:6-10`, sidebar dots `src/components/features/conversation-panel/conversation-status-dot.tsx:24-43`).
- **Why not higher:** task-tracker progress can be faked freely by the model (self-authored statuses, never validated); final success in normal conversations is not independently checked (`FINISHED` = agent stopped); the active-goal banner state is not restored from REST history on reload (store fed only by WS handlers, see Gaps); filtering of injected goal re-prompts relies on brittle text-prefix matching that the code itself labels "Brittle by design" (`src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:15-26`); the conversation-state-update handler carries a literal `// TODO: Tests` comment (`src/contexts/conversation-websocket-context.tsx:638`).

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Goal object | `GoalStatus` interface: `active`, `status`, `iteration`, `max_iterations`, `objective`, `verdict`; streamed as `ConversationStateUpdateEvent` with `key: "goal"` at kickoff, each round, and terminal state | `src/types/agent-server/core/events/conversation-state-event.ts:82-97`, `114`, `141-144` |
| Judge verdict | `GoalVerdict`: `score` (0–1 probability objective is provably done), `complete`, `missing` ("concise description of what remains") | `src/types/agent-server/core/events/conversation-state-event.ts:66-75` |
| Goal entry point | Chat interceptor parses `/goal [--max N] <objective>`, rejects bare command with toast, calls `startGoal` | `src/hooks/chat/use-goal-interceptor.ts:9-13`, `28-61` |
| Goal lifecycle API | `startGoal` / `stopGoal` / `resumeGoal`; stop records `interrupted` so resume can continue | `src/hooks/mutation/conversation-mutation-utils.ts:83-115` |
| Goal store | Zustand store mirroring latest `GoalStatus` per conversation, fed by goal events | `src/stores/goal-store.ts:5-33` |
| WS ingestion | Both main and planning WS handlers mirror goal events into the store; execution_status/stats branches alongside | `src/contexts/conversation-websocket-context.tsx:649-654`, `884-886` |
| Live progress UI | Bottom-pinned `GoalStatusBanner` while `active`; hides on terminal so inline copy settles into timeline | `src/components/features/chat/goal-status-banner.tsx:8-23` |
| Terminal rendering | Only non-active goal events pass `shouldRenderEvent`; rendered inline via `GoalStatusContent` | `src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:53-60`; `src/components/conversation-events/chat/event-message.tsx:159-167` |
| Round/score display | Banner row shows objective, "Round {{iteration}}/{{max}}", status word, "score N%", spinner/check/cross indicator, expandable "Missing:" note | `src/components/features/chat/goal-status-content.tsx:130-166`; i18n copy `GOAL$ROUND`, `GOAL$SCORE`, `GOAL$MISSING` in `src/i18n/translation.json` |
| Loop controls | Stop button (stopGoal + pauseConversation because backend stop leaves the turn running), Resume button for interrupted loops | `src/components/features/chat/goal-status-content.tsx:87-128` |
| Execution status enum | `ExecutionStatus`: idle/running/paused/waiting_for_confirmation/finished/error/**stuck** | `src/types/agent-server/core/base/common.ts:67-75` |
| Status → AgentState mapping | Frontend maps execution status to `AgentState` incl. STUCK→ERROR ("for now") | `src/hooks/use-agent-state.ts:10-58` |
| Step completion marker | Observation replaces its Action in place (`action_id` match); FinishObservation deduped against FinishAction | `src/utils/handle-event-for-ui.ts:418-441` |
| Grouped action counts | Collapsible groups show "{{count}} actions completed" or "{{completed}}/{{total}} actions completed" + spinner while running | `src/components/conversation-events/chat/event-message-components/event-group.tsx:100-105`; keys `EVENT_GROUP$ACTIONS_COMPLETED`/`ACTIONS_PROGRESS` |
| Task tracker types | `TaskItem.status: todo/in_progress/done`; `TaskTrackerAction` view/plan; `TaskTrackerObservation.task_list` | `src/types/agent-server/core/base/common.ts:1-14`; `src/types/agent-server/core/base/action.ts:127-136`; `src/types/agent-server/core/base/observation.ts:186-199` |
| Task list surfacing | `useTaskList` scans newest-first for latest `plan` observation; Task List tab renders items with active highlight | `src/hooks/use-task-list.ts:35-48`; `src/routes/task-list-tab.tsx:21-36` |
| Tool default attachment | `DEFAULT_TOOL_NAMES = ["terminal", "file_editor", "task_tracker"]` sent with new conversations | `src/api/agent-server-adapter.ts:113` |
| User approval gate | `WAITING_FOR_CONFIRMATION` execution status → "User needed"; confirmation-mode banner component exists | `src/utils/status.ts:107-108`; `src/components/features/chat/confirmation-mode-enabled.tsx:12` |
| Completion message | `FinishAction.message` is the authoritative final text; exported as assistant message in transcripts | `src/types/agent-server/core/base/action.ts:13-18`; `src/utils/transcript-export/index.ts:409-419` |
| Cost/budget progress | Stats events accumulate token usage + cost + `max_budget_per_task` into metrics store; UsagePanel shows context-fill meter and cost vs budget | `src/contexts/conversation-websocket-context.tsx:240-273`; `src/stores/metrics-store.ts:3-19`; `src/components/features/conversation/usage-panel/usage-panel.tsx:46-84` |
| Ambient status surfaces | Browser-tab emoji per state (🟢 running, ✅ finished…), sound on FINISHED/AWAITING states, sidebar dots with tooltips | `src/utils/agent-state-emoji.ts:17-31`; `src/hooks/use-agent-notification.ts:6-10`; `src/components/features/conversation-panel/conversation-status-dot.tsx:24-60` |
| Blocker recording | `ServerErrorEvent.code/detail`; `AgentErrorEvent` rendered; STUCK treated as error-class status | `src/types/agent-server/core/events/conversation-state-event.ts:154-174`; `should-render-event.ts:114-117`; `src/utils/status.ts:25-29` |
| Goal-loop fragility guard | Child-conversation launch results withheld while a goal loop is active, because any inbound message cancels the loop | `src/services/child-conversation-launch.ts:481-486` |
| Tests | Unit tests for goal store semantics, banner/content states, interceptor parsing, task list extraction | `__tests__/stores/goal-store.test.ts:26-43`; `__tests__/components/chat/goal-status-content.test.tsx:18-77`; `__tests__/hooks/chat/use-goal-interceptor.test.ts`; `__tests__/hooks/use-task-list.test.ts` |

## Answers to Dimension Questions

1. **What is the goal?** For `/goal` runs: a first-class object — the user's `objective` string plus round budget (`max_iterations`) streamed inside `GoalStatus` (`src/types/agent-server/core/events/conversation-state-event.ts:82-97`). For ordinary chats there is no structured goal object at all; intent lives implicitly in the user's messages. Mid-grain structure comes from the model-authored task list (`TaskItem[]`, `src/types/agent-server/core/base/common.ts:1-14`).

2. **How is progress measured?** Four distinct mechanisms, layered:
   - *Tool success*: an action counts as done only once its observation replaces it in the UI stream (`src/utils/handle-event-for-ui.ts:432-441`); observation kinds carry `error`/`is_error` fields (e.g., `src/types/agent-server/core/base/observation.ts:183`, `209`).
   - *Model judgement*: goal completion is decided each round by an external judge producing `{score, complete, missing}` (`conversation-state-event.ts:68-75`); the worker's claim alone does not terminate the loop.
   - *User approval*: `waiting_for_confirmation` pauses progress until the user acts (`common.ts:71`, surfaced as "User needed" in `src/utils/status.ts:107-108`).
   - *Budget meters*: iteration count vs cap, accumulated cost vs `max_budget_per_task`, and context-window fill are all tracked and shown (`goal-status-content.tsx:138-145`; `usage-panel.tsx:49-59`, `77-80`).
   - Notably **absent**: test-execution-based verification at runtime. No evidence found anywhere in the frontend of success being measured by running tests; searches for test-result consumption in the event stream returned nothing beyond generic bash observations.

3. **Can the model fake progress?** Partly yes. The `task_tracker` list is fully authored by the agent — it can mark items `done` without corresponding work, and nothing cross-checks items against observations. The UI faithfully displays whatever the last `plan` observation said (`src/hooks/use-task-list.ts:39-44`). Mitigations exist only for the goal loop, where a separate judge audits each round (`GoalVerdict`), and the honest `capped` state prevents infinite self-declared victory. In plain conversations, the model can simply stop (`FINISHED`) and declare success; the harness treats the final `FinishAction.message` as authoritative (`handle-event-for-ui.ts:225-230`). The frontend itself performs no independent verification of any success claim.

4. **Are blockers recorded?** Yes, at several granularities: `GoalVerdict.missing` explicitly records what remains for the objective and is displayed expanded on terminal status (`goal-status-content.tsx:66-68`, `163-164`); errors surface as `AgentErrorEvent`s and `ServerErrorEvent{code, detail}` (`should-render-event.ts:114-117`, `conversation-state-event.ts:154-174`); a dedicated `STUCK` execution status exists (`common.ts:74`), though the UI collapses it into ERROR (`use-agent-state.ts:30-31`). Task-item `notes` could hold blockers but are freeform model text with no blocker semantics.

5. **Is final success independently checked?** Only for the goal loop, partially: an audit judge re-evaluates the objective after every run and its verdict — not the agent's assertion — produces `complete`. But the judge is itself an LLM (score described as "probability... provably done", `conversation-state-event.ts:69-70`), and the frontend consumes the verdict without any independent check. For regular conversations: no — `FINISHED` is reported by the server from the agent stopping, and no post-hoc validation runs. Independent checking exists only in CI: the live E2E framework verifies a real `ExecuteBashObservation` succeeded through the events API before accepting the run (`AGENTS.md`, "Live End-to-End Test Framework"), which validates the pipeline, not user tasks.

## Architectural Decisions

- **Server-owned goal loop, client-owned display.** The frontend starts/stops/resumes the loop over typed client methods (`conversation-mutation-utils.ts:83-115`) but never computes progress itself; all truth arrives as events. This keeps one authority (the agent-server) and makes the UI a pure projection — including the deliberate duplication of the goal-mirror branch across both WS handlers, documented as intentional (`conversation-websocket-context.tsx:649-651`).
- **Judge-verdict completion rather than agent self-report.** `GoalVerdict.complete` gates termination, with `score` and `missing` streamed to the human each round — an architectural admission that the working agent's own "done" is insufficient evidence (`conversation-state-event.ts:66-75`).
- **Iteration cap as a designed terminus.** `max_iterations` with the distinct `capped` status makes budget exhaustion an explicit, visible outcome rather than a hang or a false success (`goal-status-content.tsx:22-27`, tests `goal-status-content.test.tsx:41-53`).
- **Events as the universal progress substrate.** One append-only stream carries everything: state updates, actions/observations, stats, goal status. UI state stores (`goal-store`, `metrics-store`, agent store) are thin per-conversation projections rebuilt from it (`src/stores/goal-store.ts`, `src/stores/metrics-store.ts:27-31`).
- **Optimistic/pending markers resolved by supersession.** Streaming deltas are provisional; the final `MessageEvent`/`FinishAction` supersedes them, and observations supersede actions — "the final event is authoritative" is stated in-code (`handle-event-for-ui.ts:225-250`, `432-441`).
- **Client-side interception of a slash-command protocol.** `/goal` parsing lives in the frontend hook (`use-goal-interceptor.ts:28-54`), making the goal feature a UI affordance over a backend capability rather than a hidden internal.

## Notable Patterns

- **Discriminated unions + type guards for event routing.** `ConversationStateUpdateEvent` narrows by `key` into FullState/AgentStatus/Stats/Goal variants (`conversation-state-event.ts:122-151`), consumed via `isGoalConversationStateUpdateEvent` (`type-guards.ts:235-237`) — progress channels are compile-time checked.
- **Dual-placement status pattern.** Live state renders in a transient pinned banner; terminal state re-renders inline in history so it persists in scrollback (`goal-status-banner.tsx:8-13`, `event-message.tsx:159-167`) — the same component serves both (`GoalStatusContent`), with expansion behavior differing by liveness (`initiallyExpanded={!active}`, `goal-status-content.tsx:164`).
- **Progress-aware UX chrome.** Scroll-follow key derived from goal store so advancing rounds keep the banner in view even though in-progress goal events are filtered from renderables (`chat-interface.tsx:171-181`); tab-title emoji; attention sounds on transitions only (`use-agent-notification.ts:34-48`).
- **Honest-failure visual grammar.** Green check exclusively for `complete`; muted cross for `capped`/`interrupted` — the UI refuses to celebrate incomplete outcomes (`goal-status-content.tsx:146-159`).
- **Cross-feature interference awareness.** The child-conversation launcher checks `useGoalStore` before reporting results, since any inbound message cancels an active goal loop (`child-conversation-launch.ts:481-486`) — one subsystem reading another's progress state to avoid breaking it.
- **Self-flagged technical debt in comments.** The re-prompt filter admits "Brittle by design; ... the durable fix is a persisted goal-loop flag on the event" (`should-render-event.ts:15-26`); `// TODO: Tests` sits directly above the state-update dispatch (`conversation-websocket-context.tsx:638`).

## Tradeoffs

- **Rich observability vs frontend-only scope.** All measurement logic (judge, round accounting, stuck detection) is server-side and invisible here; the frontend cannot answer questions like "how was score computed?" — it can only relay. This study's score reflects the contract quality, not the engine.
- **Model-authored plan vs trustworthy plan.** Giving the agent a `task_tracker` tool yields flexible decomposition and nice UI for free, but forfeits ground truth; a system-checked checklist would be stiffer to use.
- **Text-prefix suppression vs clean transcripts.** Filtering injected goal re-prompts by matching their prompt text keeps the judge feedback out of the chat without schema changes, but risks false positives (a genuine user message starting with `"The goal is NOT yet complete (audit iteration"` would vanish) and silently rots if SDK prompts change (`should-render-event.ts:15-43`).
- **WS-only liveness vs REST durability.** Feeding the goal store only from WS keeps the code path simple, but means reload-during-loop loses the banner until the next round streams (see Failure Modes).
- **Coarse `ExecutionStatus` for ambient signals vs precision.** Tab emoji and sidebar dots communicate instantly, but map several states together (e.g., FINISHED/IDLE/WAITING_FOR_CONFIRMATION all get ✅, `agent-state-emoji.ts:20-23`; STUCK→ERROR, `use-agent-state.ts:30-31`) — cheap to read, lossy to interpret.

## Failure Modes / Edge Cases

- **Reload during an active goal loop:** the REST history preload does not feed `useGoalStore` (only the two WS handlers do, `conversation-websocket-context.tsx:652-654`, `884-886`), and in-progress goal events are excluded from renderables (`should-render-event.ts:57-59`). After refresh, the live banner stays hidden until the next `goal` update arrives over WS (with `resend_mode='since'`, previously-streamed updates are not replayed). Progress becomes invisible mid-round despite the loop still running.
- **False-positive transcript filtering:** a real user message beginning with one of the re-prompt prefixes (`"The goal is NOT yet complete (audit iteration"`, `"Resuming a goal that was paused or interrupted."`) is dropped from the chat (`should-render-event.ts:23-43`).
- **Stop semantics surprise:** `stopGoal` deliberately leaves the in-flight agent turn running, so the UI must also call `pauseConversation` to actually halt work (`goal-status-content.tsx:87-93`, documented again in `conversation-mutation-utils.ts:94-99`) — a partial-stop window where progress continues after the user pressed Stop.
- **Stale-goal Resume button:** guarded by re-checking `goalActive` from the store to avoid a 409 on double-resume, acknowledged as a workaround (`goal-status-content.tsx:49-56`).
- **Unverified task lists:** if the model hallucinates `done` statuses, the Task List tab and inline cards present them identically to earned ones; there is no reconciliation pass.
- **STUCK ambiguity:** backend distinguishes `stuck` from `error`, frontend flattens them ("Map STUCK to ERROR for now", `use-agent-state.ts:30-31`), so blocked-vs-broken is indistinguishable in most UI surfaces.
- **Untested dispatch path:** the conversation-state-update ingestion (execution_status, stats, goal mirroring) carries `// TODO: Tests` (`conversation-websocket-context.tsx:638`); surrounding components are tested, this junction is not.

## Future Considerations

- Persist a machine-readable marker on goal-loop injected messages (as the code itself proposes) to replace prefix matching (`should-render-event.ts:20-26`).
- Seed `useGoalStore` from the REST preload (mirroring `seedModelSwitchesFromHistory` at `conversation-websocket-context.tsx:338-341`) so an active loop survives reload.
- Cross-check task-tracker `done` claims against emitted observations (e.g., require ≥1 observation between two status flips) to raise the cost of fake progress.
- Surface `STUCK` as its own visual state instead of aliasing ERROR (`use-agent-state.ts:30-31`), and add coverage for the state-update dispatch flagged `TODO: Tests`.
- Extend the judge contract toward executable criteria (e.g., allow the objective to carry check commands whose exit codes feed `GoalVerdict.score`) so "provably done" can mean tested, not just re-read by another LLM.

## Questions / Gaps

- **Where does the judge live and what grounds it?** Out of scope here (server repo). The frontend contract shows score/complete/missing (`conversation-state-event.ts:68-75`) but nothing about the judge's inputs. No evidence found in this source; boundary respected.
- **Does the agent-server persist goal status for recovery?** The terminal event settles into history (REST-reloadable, since `shouldRenderEvent` passes non-active goal events), implying persistence, but restoration of *active* status is unhandled in this codebase (see Failure Modes). Server-side behavior unknown from this source.
- **What triggers `ExecutionStatus.STUCK`?** Defined (`common.ts:74`) and handled defensively (`status.ts:101` skips it in the switch, falling to the error string), but no producer is visible in this repo. Searched all `STUCK` references; all are consumers.
- **Is there any rate/heartbeat signal for long-running tools?** No evidence found. Progress during a single long tool call is conveyed only by spinners (`event-group.tsx:138-143`, typing indicator excluding TaskTrackerAction in `src/components/features/chat/typing-indicator.tsx:37`); no percentage or heartbeat events were found in the event type inventory under `src/types/agent-server/core/events/`.

---

Generated by `dimensions/06.05-objective-and-progress-tracking` against `openhands`.
