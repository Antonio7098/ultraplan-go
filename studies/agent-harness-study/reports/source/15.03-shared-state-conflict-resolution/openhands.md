# Source Analysis: openhands

## Dimension 15.03 — Shared State and Conflict Resolution

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands "agent-canvas" frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Zustand, TanStack Query, WebSocket, Vite |
| Analyzed | 2026-08-26 |

## Summary

This repository is the OpenHands **frontend** (agent-canvas); the agent loop, agent state, and the file tools themselves live in the sibling `software-agent-sdk` server (see `AGENTS.md`, repository map table). Within its boundary, multi-agent operation takes three forms: (1) a main conversation plus a planning-agent sub-conversation whose two WebSocket streams are merged into a single client-side event store; (2) parent→child conversation delegation via the `launch_child_conversation` client tool, with an explicit workspace-isolation model (`worktree` vs `shared`) as the primary file-conflict avoidance mechanism; and (3) many browser tabs / UI surfaces sharing localStorage-backed registries (backends, launch ledgers, per-conversation metadata).

Conflict handling is layered and mostly **avoidance + dedup** rather than locking: git worktrees isolate child agents' file writes by default; event streams deduplicate by id with timestamp re-sorting; streaming deltas are reconciled sender-scoped so two agents cannot clobber each other's live output; non-idempotent side effects (conversation launches, optimistic-bubble consumption, cache invalidation) are guarded behind explicit duplicate checks; and server-side uniqueness (HTTP 409) is the arbiter of last resort for named resources like LLM profiles and provider connections. There are no mutexes/locks anywhere in the frontend (`grep` for mutex/semaphore/lock returns only unrelated matches such as Chrome's per-host handshake-lock comment in `src/hooks/use-websocket.ts:61-64`); shared mutable resources rely on atomic store transitions, idempotency ledgers, and server arbitration.

The one place two agents can genuinely fight over a resource — a `shared`-isolation child writing to the parent's working directory — is handled by **disclosure, not prevention**: the tool result text explicitly warns that "the two agents may conflict over the same files" (`src/services/child-conversation-launch.ts:269-270`).

## Rating

**6 / 10** — Present but uneven. Client-side shared state has a clear model with tests and explicit interfaces (dedup-by-id event store, sender-scoped delta reconciliation, atomic store transitions, replay-idempotency ledger for launches), which approaches the 7–8 band. What holds it at 6: there is no lock or coordination primitive for the highest-stakes shared resource (the workspace filesystem); `shared` isolation is allowed with only a textual warning; conflicts are detected/resolved implicitly (last-write-wins, server 409s) rather than through explicit resolution strategies; and conflict events are not logged anywhere queryable.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Shared event store (main + planning agents) | Single global Zustand store `useEventStore`; both sockets write via `addEvent`/`addEvents`; planning events flagged `isFromPlanningAgent: true` | `src/stores/use-event-store.ts:153-223`; `src/contexts/conversation-websocket-context.tsx:148-180, 794-797` |
| Dedup by event id | O(1) `eventIds` Set check before append; bulk add re-checks and re-sorts by timestamp | `src/stores/use-event-store.ts:99-107, 159-208` |
| Non-idempotent side-effect guard on replay | `isDuplicateEvent = useEventStore.getState().eventIds.has(event.id)` checked before error banners, pending-message consumption, cache invalidation (#1656) | `src/contexts/conversation-websocket-context.tsx:556-568` (main), `788-801` (planning) |
| Sender-scoped delta reconciliation | `isSameStreamingSender` uses planning flag as sole discriminator so main/planning deltas never merge or strip each other (#1656) | `src/utils/handle-event-for-ui.ts:31-37, 120-144` |
| Observation replaces action | `handleEventForUI` finds action by `event.action_id` and replaces in place; ACP tool-call events merge by `tool_call_id` | `src/utils/handle-event-for-ui.ts:404-441` |
| Final-event supersedes streamed deltas | `finalizeStreamingDeltasInPlace` drops provisional deltas once the authoritative MessageEvent/FinishAction arrives | `src/utils/handle-event-for-ui.ts:225-250` |
| Workspace isolation enum for children | `CHILD_CONVERSATION_ISOLATIONS = ["worktree", "shared"]` | `src/constants/child-conversation.ts:26-28` |
| Worktree-first with disclosed fallback | `parentSupportsWorktree` gates worktree; failure/scratch-dir falls back to `shared` with `SHARED_FALLBACK_CONSEQUENCE` warning text | `src/services/child-conversation-launch.ts:252-323` |
| Launch idempotency ledger | `claimToolCall` claims toolCallId in localStorage (`openhands-child-conversation-launches:<id>`) before any network work; corrupt ledger tolerated; storage-full accepted as replay risk | `src/services/child-conversation-launch.ts:196-227` |
| Optimistic-message echo matching | `consumeMatchingPendingMessage`: exact-content match with FIFO fallback, scoped by `conversationId`, single atomic `set` | `src/stores/optimistic-user-message-store.ts:169-198` |
| Pending-message watchdog | 150 s timeout flips stuck "sending" bubble to "error" | `src/stores/optimistic-user-message-store.ts:14, 131-139` |
| REST/WS overlap coordination | WS gated on first REST history page; subscribes `resend_mode='since'` + `after_timestamp`; overlap deduped by store; falls back to `resend_mode='all'` on initial-load error | `src/contexts/conversation-websocket-context.tsx:278-400, 966-973`; `src/hooks/query/use-conversation-history.ts:73-96` |
| Conversation-switch atomicity | `clearEventsForConversation` clears events and records new id in one `set`; layout effect ordered before seeding | `src/stores/use-event-store.ts:82-89, 216-222`; `src/contexts/conversation-websocket-context.tsx:293-318` |
| Reconnect de-synchronization | Exponential backoff with ≤30% jitter "so parallel sockets (main + planning) don't retry in lockstep" | `src/hooks/use-websocket.ts:18-20, 125-132` |
| Tab-scoped backend selection | Active backend read prefers sessionStorage over localStorage so tab A doesn't adopt tab B's backend | `src/api/backend-registry/storage.ts:205-243` |
| Credential conflict detection | `ACP_CREDENTIAL_CONFLICTS` mirrors SDK `_ENV_CONFLICT_MAP`; `getAcpCredentialConflicts` filters pairs where both values set; rendered as warnings | `src/constants/acp-providers.ts:260-292`; `src/components/features/settings/acp-conflict-warnings.tsx:10-28` |
| Server-arbitrated name conflicts | Profile-name race acknowledged (client check stale, server save fails with conflict); delete-provider-connection 409 surfaced verbatim; test asserts 409 handling | `src/components/features/settings/llm-profiles/llm-settings-local-view.tsx:237-243`; `src/components/features/settings/llm-profiles/delete-provider-connection-modal.tsx:39-43`; `src/components/features/settings/llm-profiles/provider-connections-manager.test.tsx:155-162` |
| Cache write-race prevention | Git-sync save cancels in-flight status GET before seeding query data, else stale snapshot overwrites fresh save | `src/hooks/query/use-git-sync.ts:52-56` |
| Duplicate pagination guard | Ref-based `isLoadingRef`/`hasMoreRef` guards because scroll/wheel/effect can trigger `loadOlder` in the same tick | `src/hooks/use-load-older-events.ts:53-56, 89-97` |
| Workspace list merge precedence | Implicit parents filtered when conflicting with user-added ones; static workspaces win duplicate paths | `src/hooks/query/use-resolved-workspaces.ts:63-75, 121-124` |
| Test coverage (dedup/replay/fallback) | Event-store dedup & sender tests; launch replay test; worktree-fallback tests; optimistic echo out-of-order tests | `__tests__/stores/use-event-store.test.ts:148, 227`; `__tests__/services/child-conversation-launch.test.ts:324, 490`; `__tests__/stores/optimistic-user-message-store.test.ts:97-120` |

## Answers to Dimension Questions

### 1. What state is shared between agents?

Three tiers:

- **Client-side event stream (main + planning agents).** One global, non-conversation-keyed Zustand store receives events from two independent WebSockets (`src/contexts/conversation-websocket-context.tsx:148-180`). Planning-agent events carry `isFromPlanningAgent: true` (`src/contexts/conversation-websocket-context.tsx:794-797`) as their only namespace discriminator. Terminal input/output (`useCommandStore`), execution status, metrics, and goal status stores are also fed by both handlers, with comments noting the duplication is intentional (`src/contexts/conversation-websocket-context.tsx:649-654, 881-887`).
- **Workspace filesystem (parent ↔ local child).** A local child inherits the parent's working directory; the `isolation` parameter decides whether it gets its own git worktree+branch (`worktree`, default) or runs in the parent's exact directory (`shared`) (`src/api/launch-child-conversation-client-tool.ts:46-52`; `src/services/child-conversation-launch.ts:272-298`). Cloud children always get isolated sandboxes (`src/api/launch-child-conversation-client-tool.ts:26-30`). The tool description instructs the LLM to keep sibling scopes independent "so parallel children do not fight over the same files" (`src/api/launch-child-conversation-client-tool.ts:36-38`) — prompt-level coordination.
- **Browser persistence (tabs/surfaces).** Backend registry, active-backend selection, per-conversation metadata, and launch ledgers all live in localStorage/sessionStorage (`src/api/backend-registry/storage.ts:13-14`; ledger key at `src/services/child-conversation-launch.ts:40`).

Notably, agent *memory* is not shared: the launch tool states "it cannot see this conversation's history — everything it needs must be in the task brief," and the result handoff back into the parent is a synthetic user message prefixed `[child-conversation] ` (`src/api/launch-child-conversation-client-tool.ts:16-18`; `src/constants/child-conversation.ts:11-20`).

### 2. How are conflicts detected?

- **Duplicate delivery:** `eventIds` Set membership checks in the store (`src/stores/use-event-store.ts:100, 172-173`) plus pre-side-effect re-checks in both socket handlers (`src/contexts/conversation-websocket-context.tsx:559-561, 790-792`) detect REST/WS overlap and reconnect replays.
- **Cross-agent contamination:** `isSameStreamingSender` compares the planning flag so a planning-agent delta can't concatenate onto a main-agent bubble (`src/utils/handle-event-for-ui.ts:31-37`).
- **Credential conflicts:** static pairwise table checked against typed-and-saved secret values (`src/constants/acp-providers.ts:271-292`).
- **Named-resource collisions:** delegated to the server — profile name uniqueness races end as HTTP conflict errors (`src/components/features/settings/llm-profiles/llm-settings-local-view.tsx:237-243`); deleting an in-use provider connection yields a 409 naming the referencing profiles (`src/components/features/settings/llm-profiles/delete-provider-connection-modal.tsx:39-43`).
- **Stale-cache overwrite:** recognized as a hazard and prevented by cancelling in-flight queries before seeding (`src/hooks/query/use-git-sync.ts:52-56`).
- **File-level edit conflicts between agents: no detection exists.** No evidence found of any diff/check/precondition before a shared-mode child writes alongside its parent; the codebase only predicts the conflict in prose (`src/services/child-conversation-launch.ts:269-270`). Searched `conflict|Conflict` across `src/` — remaining hits are credential conflicts, workspace-list dedup, and a `MERGE_CONFLICTS` suggested-task type imported from git providers (`src/utils/types.ts:4`), none of which is a runtime detector.

### 3. How are conflicts resolved?

- **Avoidance first:** default `worktree` isolation carves each local child its own git worktree and branch, "which is what keeps siblings from colliding" (`src/services/child-conversation-launch.ts:276-298`). When the parent workspace can't host a worktree (scratch dir with unborn HEAD) or creation fails, the launch silently degrades to `shared` but appends an explicit `isolation_note` warning to the agent and user (`src/services/child-conversation-launch.ts:300-323`). Covered by tests ("skips the worktree when the parent workspace cannot host one", "falls back to a shared child when the worktree cannot be created") at `__tests__/services/child-conversation-launch.test.ts:299, 324`.
- **Idempotency/dedup resolution:** replayed launch tool calls are dropped via a claim-before-work localStorage ledger (`src/services/child-conversation-launch.ts:205-227`, test at `__tests__/services/child-conversation-launch.test.ts:490`); replayed socket events update the arrays but skip non-idempotent side effects (`src/contexts/conversation-websocket-context.tsx:566-568`).
- **Last-write-authoritative reconciliation:** observations replace actions by `action_id`; finalized messages supersede streamed deltas; ACP terminal events replace started ones by `tool_call_id` (`src/utils/handle-event-for-ui.ts:225-250, 404-441`).
- **Server arbitration:** unique-name and referential-integrity conflicts resolve to surfaced 409 errors rather than client merges (citations above).
- **No locks:** there is no mutual-exclusion mechanism for any resource. Two writers to the same file under `shared` isolation simply interleave; nothing detects or repairs the resulting state.

### 4. Is shared state consistent?

Within its declared scope, yes — consistency is engineered at store boundaries: atomic single-`set` transitions keep invariants (clear+rebind in `clearEventsForConversation`, `src/stores/use-event-store.ts:82-89`; atomic find+filter in `consumeMatchingPendingMessage`, `src/stores/optimistic-user-message-store.ts:170-178`); bulk inserts re-sort by timestamp so out-of-order pages land correctly (`src/stores/use-event-store.ts:131-135, 202-207`); delta batching keeps ≤1 commit per frame with separate batchers per stream (`src/contexts/conversation-websocket-context.tsx:162-180`); reconnect jitter prevents synchronized hammering (`src/hooks/use-websocket.ts:125-132`); the REST tail refetch is tuned so returning conversations batch missed events instead of replaying them one-by-one (`src/hooks/query/use-conversation-history.ts:73-90`). For the filesystem tier, consistency is *assumed* via worktree isolation; choosing `shared` forfeits it by design. Across browser tabs, the active-backend selection is deliberately made tab-scoped to avoid cross-tab clobbering while keeping localStorage as a fallback for new tabs (`src/api/backend-registry/storage.ts:206-218`) — but other localStorage structures (launch ledger, conversation metadata) have no cross-tab synchronization; last writer wins.

## Architectural Decisions

1. **One global event store instead of per-conversation stores**, with `loadedConversationId` guarding clear-vs-keep semantics (`src/stores/use-event-store.ts:59-66`). Simpler, but forces careful ordering: the conversation switch must clear in a layout effect that runs before history seeding (`src/contexts/conversation-websocket-context.tsx:293-302`).
2. **Isolation-by-default for delegated agents**: `worktree` is the schema default and the code-coerced fallback target; `shared` requires deliberate opt-in and always produces a written consequence note (`src/services/child-conversation-launch.ts:191, 303-306`).
3. **Claim-before-work idempotency ledger** rather than making launches idempotent server-side: the comment explains a replay would start a second, billable Cloud conversation, so claiming happens before any network I/O (`src/services/child-conversation-launch.ts:196-204`).
4. **Deduplication centralized in the store, side-effect gating in the handlers**: the store guarantees array consistency; handlers separately guard toasts/bubbles/cache invalidations because those aren't idempotent (`src/contexts/conversation-websocket-context.tsx:556-568`).
5. **Server as source of truth for named-resource uniqueness**, with the client race explicitly documented as acceptable (`src/components/features/settings/llm-profiles/llm-settings-local-view.tsx:237-243`).

## Notable Patterns

- **Sender-scoped reconciliation (#1656)**: every delta-merge/supersede path filters through `isSameStreamingSender`, an issue-derived invariant that stops one agent's finalization from stripping another agent's live stream (`src/utils/handle-event-for-ui.ts:80-98, 133-144`) — directly tested in `__tests__/stores/use-event-store.test.ts:227`.
- **Disclosed degradation**: whenever a safety property (isolation, parent link) can't be honored, the launch result carries a machine-readable note (`isolation_note`, `parent_link_note`) that is fed back into the parent conversation for the agent to relay (`src/services/child-conversation-launch.ts:53-65, 241-250, 344-348`).
- **Watchdog timers as liveness insurance**: optimistic bubbles time out to an error state after 150 s so a dropped echo can't pin the UI forever (`src/stores/optimistic-user-message-store.ts:14, 131-139`).
- **Cancel-before-seed** for React Query mutations that also seed data (`src/hooks/query/use-git-sync.ts:52-56`), and **ref-based reentrancy guards** where multiple triggers can fire in one tick (`src/hooks/use-load-older-events.ts:53-56`).
- **Version-gated capability disclosure**: parent-link persistence depends on agent-server ≥ 1.37.1; older servers get an honest "not linked" note instead of a silent lie (`src/constants/child-conversation.ts:30-37`; `src/services/child-conversation-launch.ts:236-250`).

## Tradeoffs

- **Avoidance vs enforcement**: worktree isolation avoids most file conflicts cheaply, but `shared` mode offers zero mechanical protection — the design accepts possible corruption in exchange for letting a child see work-in-progress (`src/api/launch-child-conversation-client-tool.ts:50-52`).
- **Global store simplicity vs blast radius**: a single event store makes dedup trivial but means a mis-ordered clear can wipe seeded history; the code mitigates with ordering rules and atomic transitions rather than partitioning state (`src/contexts/conversation-websocket-context.tsx:293-318`).
- **localStorage ledger durability vs availability**: a corrupt ledger restarts empty (accepting replay risk) and a full-storage write proceeds anyway — availability is preferred over exactly-once (`src/services/child-conversation-launch.ts:214-216, 220-225`).
- **Prompt-level coordination**: instructing the LLM to give siblings disjoint scopes shifts real conflict-prevention responsibility onto model behavior (`src/api/launch-child-conversation-client-tool.ts:36-38`); nothing verifies compliance.
- **Tab-scoped backend selection fixes one cross-tab conflict but leaves the backend registry itself last-write-wins across tabs** (`src/api/backend-registry/storage.ts:95-102, 205-243`).

## Failure Modes / Edge Cases

- **Shared-isolation concurrent writes**: two agents editing the same file in the parent directory can corrupt work with no detection, log, or repair path (`src/services/child-conversation-launch.ts:269-270`).
- **Multi-tab same-conversation launches**: the launch ledger is keyed by parent conversation id in shared localStorage but has no compare-and-swap; two tabs handling the same replayed ActionEvent concurrently could both pass the `includes` check and double-launch (read-modify-write window at `src/services/child-conversation-launch.ts:205-227`).
- **FIFO fallback can pop the wrong bubble**: if the server munges echoed content and multiple messages are in flight, the oldest "sending" entry is consumed regardless of which message actually echoed (`src/stores/optimistic-user-message-store.ts:173-188`).
- **Events without ids/timestamps bypass dedup**: streaming deltas are intentionally untracked (perf), and id-less events skip the Set; correctness then leans on ordering guarantees (`src/stores/use-event-store.ts:94-107`).
- **Planning-agent history still uses `resend_mode='all'`** with count-based completion detection — a known asymmetry that replays the full sub-conversation on connect (`src/contexts/conversation-websocket-context.tsx:183-199`).
- **Storage-quota failures silently degrade** several persistence paths (backend registry, launch ledger) with catch-and-continue (`src/api/backend-registry/storage.ts:99-101`).

## Future Considerations

- Add a mechanical guard (e.g., refusing `shared` isolation unless the user confirms, or a server-side lease on the working directory) instead of the current prose-only warning for same-directory children.
- Persist isolation decisions and conflict notes somewhere observable (the `[child-conversation]` message is currently the only audit trail, and the chat hides it — `src/constants/child-conversation.ts:13-19`).
- Migrate the planning sub-conversation to the REST-then-`since` pattern to eliminate its full-replay path (`src/contexts/conversation-websocket-context.tsx:183-186`).
- Make the launch ledger resilient across tabs (Web Locks API or a server-side dedupe on `tool_call_id`).
- Consider keying the event store per conversation to remove clear-ordering hazards entirely.

## Questions / Gaps

- **What happens server-side when a `shared` child actually edits files the parent is editing?** Not answerable from this repo: the runtime, worktree creation (`new_worktree` vs `local_repo` workspace modes at `src/services/child-conversation-launch.ts:296`), and any file locking live in `software-agent-sdk`. No evidence found here beyond the mode names and the warning string.
- **Is there any conflict log?** No dedicated facility found. Searches for conflict/log patterns surfaced only telemetry error tracking (`trackError` with classification, `src/contexts/conversation-websocket-context.tsx:572-608`) and toast deduplication (`src/query-client-config.ts:54-61`). Conflict-adjacent decisions are recorded only inside conversation messages.
- **Do cloud children ever share state?** The tool contract says sandboxes are always isolated (`src/api/launch-child-conversation-client-tool.ts:46-47`); no counter-evidence found in this repo, but cloud-side behavior is out of scope.

---

Generated by dimension `15.03-shared-state-conflict-resolution` against `openhands`.
