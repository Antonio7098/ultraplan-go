# Source Analysis: openhands

## Dimension 07.06 — Side-Effect Ledger and Transaction Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, TanStack Query, Vite (the OpenHands "agent-canvas" frontend) |
| Analyzed | 2026-08-24 |

## Summary

This repository is the OpenHands **frontend** (package `@openhands/agent-canvas`, `package.json:2-5`); side effects are *executed* by the out-of-tree agent-server (`software-agent-sdk`), not here. Within its scope, the repo implements the **read/observe half of the side-effect ledger** with unusual rigor: every tool call is captured as a typed `ActionEvent`/`ObservationEvent` pair linked by `action_id`/`tool_call_id`, observations carry before/after file content and exit codes, a confirmation gate (`confirmation_mode`) blocks execution until the user accepts or rejects, and git-based views answer "what did the agent change?" at working-tree, commit, and per-file-diff granularity. Durability of the ledger is delegated to the server (REST event search + trajectory zip export), while the client keeps an in-memory, id-deduplicated store that also drives cache invalidation so the UI converges on real disk state after each mutation. What is absent is the **compensation half**: there is no transaction wrapper, no rollback/undo UX (the schema supports `undo_edit` but nothing in this repo invokes it), and confirmation records are ephemeral client-side. Compensation exists only implicitly through git.

## Rating

**6 / 10.** Clear, tested, well-typed ledger model with explicit interfaces (event schemas, observation snapshots, git change APIs) and an operational safeguard (user confirmation gate with risk levels) — that puts the observability story in the 7–8 band. But transaction boundaries are entirely absent (no begin/commit/rollback semantics anywhere; searches for `transaction|rollback|compensat` return no functional code), compensation is indirect (git worktree diffs, agent-invoked `undo_edit`), and several safeguards are fragile (in-memory-only confirmation dedup, weak pending-action binding). Since this repo owns neither tool execution nor durable persistence, it cannot be credited for those halves; 6 reflects "strong ledger display, weak compensability" for the slice it owns.

## Evidence Collected

Every entry cites files inside `studies/agent-harness-study/sources/openhands`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Side-effecting tools enumerated | Legacy `ActionType` enum (`WRITE`, `RUN`, `RUN_IPYTHON`, `BROWSE`, `MCP`) | src/types/action-type.tsx:14-49 |
| SDK action union | Discriminated union of all tool actions incl. `ExecuteBashAction`, `TerminalAction`, `FileEditorAction`, browser actions, `MCPToolAction` | src/types/agent-server/core/base/action.ts:343-369 |
| File-mutation sub-commands | `command: "view" \| "create" \| "str_replace" \| "insert" \| "undo_edit"` | src/types/agent-server/core/base/action.ts:65-94 |
| External side-effect surface | `MCPToolAction` with dynamic `data` payload | src/types/agent-server/core/base/action.ts:6-11 |
| Ledger record identity | `BaseEvent`: `id` (ULID/UUID), `timestamp`, `source` | src/types/agent-server/core/base/event.ts:10-25 |
| Action provenance | `ActionEvent` carries `tool_name`, `tool_call_id`, `llm_response_id`, `thought`, `security_risk`, LLM-generated `summary` | src/types/agent-server/core/events/action-event.ts:33-71 |
| Observation ↔ action linkage | `ObservationEvent.action_id` and `tool_call_id` tie result to the mutating call | src/types/agent-server/core/events/observation-event.ts:27-39 |
| Before/after snapshot | `FileEditorObservation.old_content` / `new_content` / `prev_exist` — raw compensation data recorded per edit | src/types/agent-server/core/base/observation.ts:111-146 |
| Command outcome record | `TerminalObservation`: command, `exit_code`, `timeout`, `is_error`, `CmdOutputMetadata` (pid, cwd, hostname) | src/types/agent-server/core/base/observation.ts:84-109 |
| Execution metadata external IDs | `CmdOutputMetadata.pid`, `.working_dir`, `.py_interpreter_path` | src/types/agent-server/core/base/common.ts:16-49 |
| Client-side ledger store | Zustand store with id-set dedup, bulk add + timestamp re-sort, per-conversation clear invariant | src/stores/use-event-store.ts:92-129, 159-222 |
| Durable ledger is server-owned | REST event search with `limit/pageId/sortOrder/timestamp__gte/__lt` against cloud App API or runtime | src/api/event-service/event-service.api.ts:102-181 |
| Trajectory export | `downloadConversation` → `FileClient.downloadTrajectory(conversationId)` zip | src/api/conversation-service/agent-server-conversation-service.api.ts:673-681 |
| Confirmation gate state | `ExecutionStatus.WAITING_FOR_CONFIRMATION` → `AgentState.AWAITING_USER_CONFIRMATION` mapping | src/types/agent-server/core/base/common.ts:67-75; src/hooks/use-agent-state.ts:24-25 |
| Confirmation UI + API | Buttons post `{accept}` to `/events/respond_to_confirmation`; reject path = `UserRejectObservation` | src/components/shared/buttons/conversation-confirmation-buttons.tsx:38-58; src/hooks/mutation/use-respond-to-confirmation.ts:12-31; src/api/event-service/event-service.types.ts:1-7 |
| Rejection record type | `UserRejectObservation` with `rejection_reason` and `action_id` | src/types/agent-server/core/events/observation-event.ts:42-52 |
| Risk-tiered safeguard | `SecurityRisk.HIGH` renders `RiskAlert` above confirm buttons | src/components/shared/buttons/conversation-confirmation-buttons.tsx:102-118; src/types/agent-server/core/base/common.ts:59-64 |
| Confirmation dedup guard | `submittedEventIds` store prevents double-submit per event id | src/stores/event-message-store.ts:14-26; conversation-confirmation-buttons.tsx:44-47 |
| Confirmation-mode setting | `confirmation_mode: boolean` persisted server-side; legacy duplicates deduped in verification settings page | src/types/settings.ts:128; src/routes/verification-settings.tsx:9-12 |
| Hook policy/audit events | `HookExecutionEvent`: PreToolUse/PostToolUse hooks with `blocked`, `reason`, `exit_code`, stdout/stderr | src/types/agent-server/core/events/hook-execution-event.ts:8-77 |
| Hooks rendered as audit trail | Hook events render in chat with exit code | src/components/conversation-events/chat/event-content-helpers/should-render-event.ts:119-122; src/components/shared/hook-execution-event-message.tsx:101-106 |
| UI ledger reconciliation | Observation replaces its action in place by `event.id === action_id`; ACP calls merge by `tool_call_id` | src/utils/handle-event-for-ui.ts:404-441 |
| Outcome status derivation | success/error/timeout classified from `is_error`, `exit_code` (-1 = timeout), `timeout` flags | src/components/conversation-events/chat/event-content-helpers/get-observation-result.ts:17-43 |
| Mutation-driven cache refresh | `useAutoRefreshFilesOnEdit` classifies mutation observations (editor kinds minus read-only `view`; bash kinds) and invalidates `file_changes`/`file_diff`/`git_commits`/workspace queries | src/hooks/use-auto-refresh-files-on-edit.ts:8-48, 129-149 |
| Exactly-once reaction tracking | Id `Set` + `WeakSet` for id-less events so each side effect is reacted to once | src/hooks/use-auto-refresh-files-on-edit.ts:77-103 |
| Edit-freshness cache buster | Monotonic workspace mutation counter appended as `?v=<n>` to file URLs | src/stores/use-workspace-mutation-counter.ts:30-53 |
| "What changed" surface | Git changes list (no `ref` → committed work still visible) + diff endpoints | src/api/git-service/agent-server-git-service.api.ts:143-165, 231-279 |
| Commits view | Commit list + uncommitted-changes accordion with file count | src/routes/commits-tab.tsx:21-91; src/components/features/diff-viewer/uncommitted-changes-row.tsx:29-63 |
| Change magnitude summary | Additions/deletions/changeCount aggregated across per-file diffs | src/hooks/use-conversation-overview-git-diff-stats.ts:36-80 |
| Per-action human label | `getActionSummaryTitle` prefers the LLM-generated `summary` field | src/components/conversation-events/chat/event-content-helpers/get-action-event-title.ts:24-32 |
| Complete-history export proof | Transcript loader paginates to exhaustion, detects cursor loops/non-advancing pages, throws on incomplete history | src/utils/transcript-export/load-complete-events.ts:50-124, 153-160 |
| Automation audit log export | CSV columns include run_id, trigger, times, status, conversation_id, error, cost | src/utils/automation-activity-log-export.ts:12-27 |
| Automation run external IDs | `AutomationRun.bash_command_id` links a run to sandbox bash events; `conversation_id` links to the conversation | src/types/automation.ts:75-98 |
| Off-site replication | Git sync status tracks `last_synced_commit`, `dirty_count`, sync-in-progress | src/types/git-sync.ts:1-27 |

## Answers to Dimension Questions

**1. What external changes did the agent make?**
Answerable in three layers: (a) per-call — the chat stream shows every `ActionEvent` replaced by its outcome `ObservationEvent` (`src/utils/handle-event-for-ui.ts:418-441`), with editor observations recording old/new content (`src/types/agent-server/core/base/observation.ts:136-141`); (b) per-workspace — `/api/git/changes` and `/api/git/diff` surfaces modified/added/deleted files vs. the auto-detected base (`src/api/git-service/agent-server-git-service.api.ts:143-165`); (c) durable — commits made by the agent appear in the commits pane (`src/routes/commits-tab.tsx:57-66`). Caveat: outside git-tracked paths there is no change inventory; bash mutations are only visible indirectly via exit-code records and refreshed diff queries.

**2. Are side effects auditable?**
Yes, durably — but server-side. Events are persisted and searchable with pagination/timestamp filters (`src/api/event-service/event-service.types.ts:15-33`); trajectory zip export downloads the whole ledger (`src/api/conversation-service/agent-server-conversation-service.api.ts:673-681`); transcript export refuses partial histories rather than exporting a silent tail (`src/utils/transcript-export/load-complete-events.ts:106-110, 153-160`). Automation runs get a separate activity-log export keyed by run/conversation IDs (`src/utils/automation-activity-log-export.ts:12-27`). Client-side the store is memory-only and cleared per conversation (`src/stores/use-event-store.ts:209-222`). Pre/post-tool hook executions are themselves logged events with blocked/reason/exit-code fields (`src/types/agent-server/core/events/hook-execution-event.ts:12-77`).

**3. Can failed side effects be compensated?**
Only implicitly. No compensation handler, saga, or retry-with-undo logic exists in this repo (searches for `rollback|compensat|transaction` match no functional code). Compensation affordances are: the agent-invocable `undo_edit` file-editor command in the schema (`src/types/agent-server/core/base/action.ts:69`) — never wired to any frontend control; the recorded `old_content` snapshot which *would* enable manual restoration (`observation.ts:137`); and git itself, where users can inspect but (in this repo) not revert — the git actions menu exposes commit/push flows, not reverts (no revert/push-back code found; `UserRejectObservation` at `observation-event.ts:42-52` prevents a not-yet-executed action, it does not undo an executed one).

**4. Are external IDs stored?**
Yes, extensively: event `id` (ULID/UUID), `tool_call_id`, `llm_response_id` grouping parallel calls (`action-event.ts:35-56`), `action_id` back-reference on observations (`observation-event.ts:38`), process-level `pid`/cwd in `CmdOutputMetadata` (`common.ts:22-36`), commit SHAs (`agent-server-git-service.api.ts:20-26`), automation `bash_command_id` linking a scheduled run to its sandbox command (`src/types/automation.ts:79-86`), and child-conversation IDs reported via client-tool results (`should-render-event.ts:45-51`).

**5. Are users shown what changed?**
Yes: collapsible action cards grouped into "actions completed" runs (`src/components/conversation-events/chat/group-events.ts:13-79`), LLM-written per-action summaries used as card titles (`get-action-event-title.ts:24-32`), a Diff tab with uncommitted-change counts and per-file diffs (`uncommitted-changes-row.tsx:44-51`), aggregate +/- line stats in the overview drawer (`use-conversation-overview-git-diff-stats.ts:68-80`), and high-risk actions flagged before confirmation (`conversation-confirmation-buttons.tsx:111-118`).

## Architectural Decisions

1. **Ledger ownership is server-side; the client is a projection.** The frontend treats the event stream as authoritative: REST-first seeding then WebSocket catch-up (`resend_mode='since'` after the latest preloaded timestamp, per `AGENTS.md` and `EventService.searchEvents` at `src/api/event-service/event-service.api.ts:102-181`). The Zustand store is explicitly a session cache, not a database.
2. **Pair-linked event model instead of mutation records.** Rather than a dedicated "side effects" table, every mutation is an `ActionEvent`+`ObservationEvent` pair joined by ids. This makes the ledger complete-by-construction (every tool call has both halves) and lets the UI replace pending cards in place (`handle-event-for-ui.ts:432-441`).
3. **Confirmation as the transaction boundary.** The single pre-commit gate is `WAITING_FOR_CONFIRMATION` execution status plus a user accept/reject POST (`src/hooks/mutation/use-respond-to-confirmation.ts:21-30`); rejection produces a first-class `UserRejectObservation` event, so even refusals are part of the ledger.
4. **Git as the change-summary substrate.** All "what changed" UX is built on runtime git endpoints rather than an internal write-log (`src/api/git-service/agent-server-git-service.api.ts:111-229`), deliberately omitting `ref` so agent-made commits remain visible in the changes list (`:143-149`).
5. **Derived freshness instead of push-based invalidation.** After a mutation observation arrives, the client reclassifies it (`use-auto-refresh-files-on-edit.ts:8-48`) and invalidates React Query caches — a pull-based convergence model that tolerates missed websocket frames at the cost of extra fetches.

## Notable Patterns

- **In-place supersession**: one card per tool call, mutated from "running" to result, for both normal actions and ACP tool calls (`handle-event-for-ui.ts:404-416`) — prevents duplicate-looking ledger entries.
- **Snapshot-rich observations**: `old_content`/`new_content`/`prev_exist` turn each file edit into self-contained compensation data (`observation.ts:130-141`) — an underexploited asset given the absence of undo UX.
- **Completeness-proof exports**: transcript loading detects repeated cursors, non-advancing pagination, and count mismatches, throwing instead of exporting partial history (`load-complete-events.ts:86-123, 153-160`).
- **Exactly-once reaction bookkeeping**: dual `Set`/`WeakSet` tracking handles both id-bearing and id-less events without re-triggering cache bumps on re-render (`use-auto-refresh-files-on-edit.ts:102-124`).
- **Risk-tiered confirmation**: `SecurityRisk` from the LLM risk analyzer drives a warning banner and keyboard-shortcut accept/reject (`Cmd+Enter` / `Shift+Cmd+Backspace`, `conversation-confirmation-buttons.tsx:60-90`).

## Tradeoffs

- **Server-trust for durability**: the client cannot reconstruct history if the backend's event store is lost; conversely the client stays simple and consistent across local/cloud backends.
- **Git-dependence of change visibility**: non-git workspaces have no equivalent change inventory; the codebase acknowledges this by suppressing diff-view defaults when a workspace is not a git repo (per `AGENTS.md`; behavior surfaced via `useUnifiedGetGitChanges` error paths, `use-unified-get-git-changes.ts:45-53`).
- **Memory-only confirmation guards**: `submittedEventIds` lives in a module-level Zustand store (`event-message-store.ts:14-26`); a reload during `AWAITING_USER_CONFIRMATION` loses the submitted marker (server presumably still enforces one response, but the client can re-offer buttons).
- **Cache-invalidation breadth vs precision**: any bash observation invalidates all `file_changes`/`file_diff`/`git_commits` queries because shell commands are opaque (`use-auto-refresh-files-on-edit.ts:57-64`) — correct but potentially chatty.

## Failure Modes / Edge Cases

- **Weak pending-action binding**: `awaitingAction` is found by scanning reversed events for *any* agent-sourced event while the state is `AWAITING_USER_CONFIRMATION`; the predicate ignores the event's content (`conversation-confirmation-buttons.tsx:30-36`), so the confirmation attaches to whatever the last agent event happens to be rather than the specific gated action.
- **Out-of-order arrival**: handled by timestamp re-sorting on insert/bulk-add (`use-event-store.ts:41-53, 131-135`), and observation-before-action arrival still renders (observation appended when its action is missing, `handle-event-for-ui.ts:438-441`).
- **Cloud pagination degradation**: if the cloud backend lacks filter support, interactive history silently degrades to a single page (`event-service.api.ts:149-163`); only transcript export opts into `strictPagination` to fail loudly (`load-complete-events.ts:10, 54`).
- **Deleted-file diffs 400** on older servers (`path.exists()` check), worked around by disabling the query for `D` status and rendering a placeholder (documented in `AGENTS.md`; status filtering example at `use-conversation-overview-git-diff-stats.ts:37`).
- **Timeout ambiguity**: `exit_code === -1` means "soft timeout, still running", mapped to a distinct `timeout` status rather than error (`get-observation-result.ts:30-34`) — avoids mislabeling in-flight work as failed.

## Future Considerations

- Expose a user-facing "revert this edit / restore previous content" control backed by the already-persisted `old_content` snapshots — the highest-leverage upgrade given the data already exists.
- Persist confirmation decisions server-side and echo them back (e.g., a confirmation record event) so the accept/reject audit survives reloads; today only the resulting continuation is implicit in the stream.
- Strengthen the pending-action binding: match the awaited `ActionEvent.id` (the server knows which action is gated) instead of "last agent event".
- Extend the automation activity-log pattern (run IDs + external `bash_command_id`) to interactive conversations as a first-class "changes made" report at finish time, rather than requiring users to browse git panels.

## Questions / Gaps

- **Where is the durable confirmation record?** `respondToConfirmation` returns only `{success}` (`event-service.types.ts:9-11`); whether the server logs the decision as an event could not be verified from this repo. Searched: `src/api/event-service/*`, `src/hooks/mutation/use-respond-to-confirmation.ts`.
- **Does anything ever send `undo_edit`?** No sender found in `src/` (only schema definitions and observation mirrors). The rollback capability may exist purely server-side/agent-side.
- **No evidence found** for transactional multi-step boundaries (e.g., "apply these 5 edits atomically") — searched `rollback|compensat|transaction|revert` across `src/`; the only matches were an xterm `scrollback` option (`src/hooks/use-terminal.ts:104`) and a prompt-string suggestion to revert dependencies (`src/utils/suggestions/repo-suggestions.ts:30`).
- Whether `HookExecutionEvent.blocked=true` actually prevents the paired action's execution is defined in the SDK repo; from this frontend it is observable only as a rendered event.

---

Generated by `07.06-side-effect-ledger-and-transaction-boundaries` against `openhands`.
