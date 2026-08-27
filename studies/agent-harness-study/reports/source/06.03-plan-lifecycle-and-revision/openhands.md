# Source Analysis: openhands

## Plan Lifecycle and Revision

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | Python (FastAPI + openhands-sdk) / TypeScript React frontend |
| Analyzed | 2026-08-27 |

## Summary

OpenHands implements plans as a **single markdown file (`PLAN.md`)** produced by a dedicated Planning Agent rather than a first-class plan object with state machine. Creation is a separate `AgentType.PLAN` sub-conversation (`openhands/app_server/app_conversation/app_conversation_models.py:68`, `live_status_app_conversation_service.py:1657`) that uses `PlanningFileEditorTool` (`_sdk_inspect/tools/planning_file_editor/definition.py:62`) restricted to editing only `PLAN.md` via `FileEditorExecutor(allowed_edits_files=[plan_path])` (`_sdk_inspect/tools/planning_file_editor/impl.py:28`). The file is initialized from a 5-section template (`_sdk_inspect/tools/preset/planning.py:14`) and rendered/edited through file-editor commands (`create`, `str_replace`, `insert`, `undo_edit`). Updates are entirely user-driven (system_prompt_planning.j2 Phase 4 Refinement) with no automatic replanning, completion validators, drift detection, or explicit version store; history is implicitly the append-only `EventLog` (`_sdk_inspect/sdk/conversation/event_store.py:25`) of `PlanningFileEditorObservation` events, while the frontend collapses that history to a single preview per phase (`frontend/src/components/v1/chat/hooks/use-plan-preview-events.ts:87`). Execution is a manual handoff: the Build button (`frontend/src/hooks/use-handle-build-plan-click.ts:27`) switches to code mode and sends `Execute the plan based on the .agents_tmp/PLAN.md file.`

## Rating

**4/10 — Present but inconsistent, weakly documented, fragile**

Rationale: Plan creation and editing are explicit and enforced (tool restriction, initialized headers, phase-gated system prompt). However lifecycle is incomplete: no replanning triggers, no status transitions, no completion validator, no persisted revision metadata or justification trail, no abandon/invalidate semantics, and no drift detection. History exists only as low-level file-edit events, not as semantic plan revisions. Operational safeguards (concurrency, versioning, observability) are absent.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Plan creation – agent type | `AgentType.PLAN = 'plan'` enum drives branching | `openhands/app_server/app_conversation/app_conversation_models.py:64-68` |
| Plan creation – instruction boundary | `PLANNING_AGENT_INSTRUCTION` forbids execution, directs Build | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:209-220` |
| Plan creation – path computation | `_compute_plan_path` switches `.agents_tmp` vs `agents-tmp-config` by provider | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1140-1160` |
| Plan creation – tool set | `get_planning_tools(plan_path)` returns Glob, Grep, PlanningFileEditorTool | `_sdk_inspect/tools/preset/planning.py:105-133` |
| Plan creation – template | `PLAN_STRUCTURE` 5 sections + `get_plan_headers()` / `format_plan_structure()` | `_sdk_inspect/tools/preset/planning.py:14-53,56-88` |
| Plan creation – file init | `PlanningFileEditorTool.create` validates absolute path, falls back to `.agents_tmp/PLAN.md`, handles legacy root, writes headers | `_sdk_inspect/tools/planning_file_editor/definition.py:68-137` |
| Plan creation – system prompt | Planning workflow Phase 1-4 (Understand → Plan → Synthesize → Refine) with write to `PLAN.md` | `_sdk_inspect/sdk/agent/prompts/system_prompt_planning.j2:26-83` |
| Plan update rules – restriction | `PlanningFileEditorExecutor` delegates to `FileEditorExecutor(allowed_edits_files=[plan_path])` | `_sdk_inspect/tools/planning_file_editor/impl.py:21-31` |
| Plan update rules – edit guard | Non-view ops rejected if path not in allowed set, with explicit error | `_sdk_inspect/tools/file_editor/impl.py:42-55` |
| Plan update rules – workflow guidance | Phase 4 Refinement: "Incorporate user feedback … Summarize changes" | `_sdk_inspect/sdk/agent/prompts/system_prompt_planning.j2:70-83` |
| Plan events – action/observation types | `PlanningFileEditorAction/Observation` inherit FileEditor types | `_sdk_inspect/tools/planning_file_editor/definition.py:33-45` |
| Plan events – frontend guard | `isPlanningFileEditorObservationEvent` type guard | `frontend/src/types/v1/type-guards.ts:135-141` |
| Revision history – event persistence | `EventLog` appends each event as `event-{idx}-{id}.json` with locking | `_sdk_inspect/sdk/conversation/event_store.py:25-158` |
| Revision history – UI collapsing | `usePlanPreviewEvents` groups by user-message phase, keeps only last `PlanningFileEditorObservation` per phase | `frontend/src/components/v1/chat/hooks/use-plan-preview-events.ts:50-105` |
| Revision history – duplicate suppression | `EventMessage` returns `null` for non-designated preview events | `frontend/src/components/v1/chat/event-message.tsx:242-263` |
| Status transitions – planning vs default | `_apply_server_agent_overrides` sets planning system prompt + condenser; `AgentType.PLAN` branch selects tools | `openhands/app_server/app_conversation/live_status_app_conversation_service.py:1653-1661,2043-2047` |
| Completion – Build handoff | `useHandleBuildPlanClick` switches to `code` mode and sends build prompt | `frontend/src/hooks/use-handle-build-plan-click.ts:13-38` |
| Completion – plan preview UI | `PlanPreview` renders `planContent` with streaming shine, Build button disabled while running | `frontend/src/components/features/chat/plan-preview.tsx:34-142` |
| Persistence – store | `planContent: string \| null` in Zustand, `setPlanContent` only holds latest | `frontend/src/stores/conversation-store.ts:35,63,141,300-301` |
| Persistence – history replay | `ConversationWebSocketProvider` tracks `latestPlanningFileEventRef` during replay, then `readConversationFile` once | `frontend/src/contexts/conversation-websocket-context.tsx:144-148,300-325` |
| Persistence – live updates | Planning websocket handler calls `readConversationFile({filePath:path})` on each `PlanningFileEditorObservation` | `frontend/src/contexts/conversation-websocket-context.tsx:690-725` |
| Completion validator – absent | No validator code / test for plan completeness; only free-text TESTING AND VALIDATION section | `_sdk_inspect/tools/preset/planning.py:47-52` |
| Drift detection – absent | No comparison between `PLAN.md` and git diff / execution trace; `get_conversation_git_changes` not linked to plan | `openhands/app_server/app_conversation/app_conversation_router.py:1308-1339` |

## Answers to Dimension Questions

**1. Can plans change?**
Yes, but only via human-in-the-loop file edits. The planning agent can issue `create`/`str_replace`/`insert`/`undo_edit` on `PLAN.md` through the restricted file editor (`_sdk_inspect/tools/planning_file_editor/definition.py:54-56`, `_sdk_inspect/tools/file_editor/impl.py:42-55`). The system prompt explicitly allows Phase 4 Refinement (`_sdk_inspect/sdk/agent/prompts/system_prompt_planning.j2:70-83`). There is no API for atomic plan patch or version increment; changes are raw file mutations.

**2. Are changes justified?**
No systematic justification. The prompt instructs the agent to "Summarize changes after each update" (`_sdk_inspect/sdk/agent/prompts/system_prompt_planning.j2:82`) and to ask clarifying questions, but no structured `reason`/`justification` field is stored alongside the observation. `PlanningFileEditorObservation` (`_sdk_inspect/tools/planning_file_editor/definition.py:41`) inherits `FileEditorObservation` with only `content`, `path`, `is_error`; the `EventLog` (`_sdk_inspect/sdk/conversation/event_store.py:119`) persists the event without semantic diff metadata. Frontend collapses multiple edits to a single preview, losing intermediate justifications.

**3. Is old plan history preserved?**
Partially — as raw event files, not as a revision history. Each edit creates an `EventLog` entry (`_sdk_inspect/sdk/conversation/event_store.py:147-151`) that remains on disk and in the websocket replay. However there is no plan-specific version table, diff view, or revert UI. The frontend intentionally discards history: `usePlanPreviewEvents` (`frontend/src/components/v1/chat/hooks/use-plan-preview-events.ts:87`) and `EventMessage` (`frontend/src/components/v1/chat/event-message.tsx:261`) suppress all but the last observation per phase, and `conversation-store.ts:35` keeps only `planContent: string \| null` (latest). No `plan_revision` or `plan_history` table was found.

**4. Can the agent abandon a plan?**
No explicit abandon/invalidate primitive. The planning agent's instruction says "Your role ends when the plan is finalized" (`openhands/app_server/app_conversation/live_status_app_conversation_service.py:219`), and the Build path is opt-in. A user can ignore the plan, start a new `PLAN` conversation (`AppConversationStartRequest.agent_type`, `openhands/app_server/app_conversation/app_conversation_models.py:249`), or delete the conversation (`DELETE /app-conversations/{id}`, `openhands/app_server/app_conversation/app_conversation_router.py:939`). The execution agent is not constrained to follow the plan; the build prompt is a soft suggestion (`frontend/src/hooks/use-handle-build-plan-click.ts:27`), not an enforced contract.

**5. Can plan drift be detected?**
No. There is no comparison between `PLAN.md` and actual code changes, test results, or execution trajectory. Git state is observable via `GET /{conversation_id}/git/changes` and `/git/diff` (`openhands/app_server/app_conversation/app_conversation_router.py:1308-1366`), but no code links those endpoints to the plan. No failure mode triggers replanning, no confidence/truth check, and no `plan_drift` metric or event. The planning condenser (`_sdk_inspect/tools/preset/planning.py:136-151`, `live_status_app_conversation_service.py:614-648` with `planning_condenser`) summarizes, not validates.

## Architectural Decisions

- **File-as-plan rather than object-as-plan**: Choosing a markdown file in the workspace (`_sdk_inspect/tools/planning_file_editor/definition.py:29-30`) makes the plan visible to the code agent without serialization, but forfeits schema, status, and versioning. Decision explicit in comment "to keep workspace root clean" (`definition.py:27-28`).

- **Sub-conversation isolation**: Plans live in a separate conversation (`subConversations[0]` as planning agent, `frontend/src/contexts/conversation-websocket-context.tsx:215-234`) with its own websocket, event store, and condenser. Decouples planning from execution but introduces eventual consistency (two `isLoadingHistory` flags, merged `connectionState`).

- **Read-all / edit-one tool restriction**: `allowed_edits_files=[plan_path]` (`_sdk_inspect/tools/planning_file_editor/impl.py:30`) is the sole guard; viewing is unrestricted so the planner can explore. Simple and auditable, but coarse — no per-section locking or concurrent edit protection beyond file-level `flock` (`_sdk_inspect/sdk/conversation/event_store.py:129`).

- **Prompt-driven lifecycle over code-driven state machine**: Transitions are described in `system_prompt_planning.j2:26-83` (4 phases) not in a `PlanStatus` enum. No `PLAN → IN_REVIEW → APPROVED → EXECUTING → DONE` transitions exist in `app_conversation_models.py:288` (only `AppConversationStartTaskStatus`). Gives flexibility but no enforceable invariants.

- **Event-sourcing via EventLog for all history**: Reuses generic `EventLog` (`_sdk_inspect/sdk/conversation/event_store.py:25`) for plan edits, gaining durability and locking, but pollutes plan history with unrelated events and requires frontend filtering (`usePlanPreviewEvents`).

## Notable Patterns

- **Planning-only tool preset**: `get_planning_tools()` (`_sdk_inspect/tools/preset/planning.py:105`) intentionally omits `execute_bash`/`browser` to force read-only exploration; execution tools are only in `get_default_tools()` (`live_status_app_conversation_service.py:2049-2053`).

- **Phase-based deduplication**: Frontend treats user messages as phase boundaries to decide which `PlanningFileEditorObservation` gets a `PlanPreview` card (`frontend/src/components/v1/chat/hooks/use-plan-preview-events.ts:16-39,87-105`). Prevents duplicate previews during rapid edits but also hides intermediate steps.

- **Legacy path migration with warning**: `_sdk_inspect/tools/planning_file_editor/definition.py:99-110` detects `PLAN.md` at workspace root and logs a warning to move to `.agents_tmp/PLAN.md`, showing incremental adoption.

- **Provider-aware plan location**: `_compute_plan_path` (`live_status_app_conversation_service.py:1140-1160`) switches config dir for GitLab/Azure DevOps because `.agents_tmp` is invalid there, showing plan path is coupled to git host.

- **Two-stage plan consumption**: Planning websocket writes `planContent` (`conversation-websocket-context.tsx:710-722`), while history replay batches via `latestPlanningFileEventRef` then single `readConversationFile` (`:301-324`). Optimizes replay but introduces race where concurrent edits during replay collapse to one read.

## Tradeoffs

- **Simplicity vs traceability**: A flat file is simplest inter-agent contract but loses structured fields (goals, dependencies, validation criteria) that would enable programmatic completion checks or drift detection. The 5 markdown headers (`_sdk_inspect/tools/preset/planning.py:14`) are unenforced prose.

- **Prompt flexibility vs safety**: Letting the LLM decide when to ask clarifying questions or summarize changes (`system_prompt_planning.j2:10-13,82`) avoids rigid FSMs but means compliance is unverified; no test asserts the summary is emitted.

- **EventLog reuse vs semantic history**: Persisting plan edits as generic events avoids a second store, but makes `plan changes justified?` unanswerable without NLP over free-text observations. The `EventLog` gap/duplicate warnings (`_sdk_inspect/sdk/conversation/event_store.py:234,247`) are about index integrity, not plan semantics.

- **Isolated sub-conversation vs single conversation**: Isolation prevents the planning agent from accidentally executing code (enforced by `PLANNING_AGENT_INSTRUCTION` + tool allowlist), but requires the Build handoff to be manual and non-atomic; a user can interleave messages to the wrong agent (mode switch via `conversation-websocket-context.tsx:908-913`).

- **Frontend-only plan state vs server truth**: `planContent` lives only in `useConversationStore` (`frontend/src/stores/conversation-store.ts:35`) and is refetched from the sandbox filesystem (`app_conversation_router.py:1129-1221` reads any file, defaults to `/workspace/project/PLAN.md`). If the sandbox is paused/archived the file read returns empty string (`router.py:1156,1161`), silently clearing the plan.

## Failure Modes / Edge Cases

- **Lost revisions on history collapse**: Only last observation per phase is shown (`usePlanPreviewEvents.ts:96-100`); if the planner makes 5 str_replace edits, intermediate versions are invisible in UI and not diffable. No `undo_edit` audit beyond the event log.

- **Stale index after external modification**: `EventLog._get_single_item` rebuilds index on stale read (`_sdk_inspect/sdk/conversation/event_store.py:92-101`) but logs a warning for index gaps (`:234-237`) and duplicate IDs (`:247`). Concurrent `PLAN.md` edits from outside the tool bypass `allowed_edits_files` and would not be versioned.

- **Missing file on paused/archived sandbox**: `read_conversation_file` (`openhands/app_server/app_conversation/app_conversation_router.py:1129`) returns `''` for any error or non-RUNNING sandbox (`:1156,1161,1221`), which the frontend treats as "no plan" (`plan-preview.tsx:65-67` returns null). Plan appears to have vanished.

- **Provider-specific path divergence**: The old `app_conversation_router.py:1135` default `'/workspace/project/PLAN.md'` conflicts with the newer `._compute_plan_path` logic (`.agents_tmp` vs `agents-tmp-config`). A plan created under legacy path is still read but triggers a warning (`definition.py:105-108`), not an automatic migration.

- **No completion gate**: Build is enabled solely on `planContent` existence and agent idle (`conversation-tab-title.tsx:36` `isBuildDisabled = isAgentRunning || !planContent`). User can click Build on an incomplete half-filled template (headers only). No validator checks that all 5 sections have content.

- **Silent drift after Build**: Code agent prompt (`frontend/src/hooks/use-handle-build-plan-click.ts:27`) is not enforced; the agent may ignore `PLAN.md`, and no downstream check compares `git diff` (`router.py:1308`) to plan. Drift is undetectable by design.

- **Lock timeout not surfaced to planner**: `EventLog.append` (`event_store.py:129-157`) raises `TimeoutError` after 30s if `flock` fails; the tool executor catches only `ToolError` (`file_editor/impl.py:68`), so timeout would propagate as unhandled and the planning step would error without retry.

## Future Considerations

- Add a typed `Plan` model (`status`, `version`, `justification`, `approval`) rather than a markdown blob; persist revisions in a dedicated table with diff and author, and expose a `GET /{conversation_id}/plan/history` endpoint.
- Enforce section completeness before Build (e.g., require non-empty `IMPLEMENTATION STEPS` and `TESTING AND VALIDATION`), and surface a plan lint/validator as a tool the planner must call.
- Introduce explicit replanning triggers: on test failure, on user "this is wrong" signal, or on measured drift (compare plan's referenced files/functions vs `git changes`). Wire to a `replan` event that increments version with reason.
- Capture `why` for each revision (LLM-generated summary + user instruction) as structured metadata on `PlanningFileEditorObservation`; extend frontend to render revision timeline instead of collapsing to one.
- Decouple plan persistence from sandbox filesystem liveness; store canonical copy in app-server storage so `planContent` survives pause/archive and can be diffed after sandbox reaped.
- Add drift detection: hash referenced files at plan time, compare at execution milestones, emit `PlanDriftDetected` event when implementation diverges beyond threshold.

## Questions / Gaps

- **No server-side plan status machine exists** — search of `openhands/app_server/app_conversation/` found only `AppConversationStartTaskStatus` (`app_conversation_models.py:288`), no `PlanStatus`. Confirmation requires full `app_server` grep for `plan.*status|revis|drift`.
- **No persistence table for plans** — search for `PLAN` in SQL models (`sql_app_conversation_info_service.py`) returned only `completion_tokens`; no `stored_plan*` table. Could a plan be archived via `archive_conversation_workspace` (`remote_sandbox_service.py:1034`) as part of sandbox archive? Not traced.
- **No tests for plan revision semantics** — frontend tests (`use-plan-preview-events.test.ts:79`, `plan-preview.test.tsx:89`) cover rendering, not lifecycle; no test found for `PlanningFileEditorTool` edit restriction or `EventLog` gap handling for plan events.
- **Unclear whether `undo_edit` preserves history** — `FileEditor` (`editor.py` not inspected) likely reverts file content but event history remains; behavior not verified due to isolation to single source.
- **Build prompt path mismatch** — frontend sends `.agents_tmp/PLAN.md` (`use-handle-build-plan-click.ts:27`) while `_compute_plan_path` may produce `agents-tmp-config/PLAN.md` for GitLab/Azure; whether code agent resolves both was not verified.

---

Generated by `Dimension 06.03: Plan Lifecycle and Revision` against `openhands`.
