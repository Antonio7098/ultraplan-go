# Source Analysis: openhands

## Resource Locking and Isolation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, Vite, React Router 7, Zustand, Node.js launcher scripts (`package.json:2-21`) |
| Analyzed | 2026-08-23 |

## Summary

The analyzed source is the OpenHands **agent-canvas frontend** (`@openhands/agent-canvas`, `package.json:2-4`). Per its own repository notes (AGENTS.md, "Repository Map" table), tool execution — terminal, file_editor, browser — happens in a separate agent-server backend (`OpenHands/software-agent-sdk`); this repo only renders UI and calls backend APIs. Consequently there is **no in-process lock manager for tool resources** here: no mutexes, semaphores, or file locks exist anywhere under `src/` (searched for `mutex|semaphore|lock|acquire`).

What this source *does* contain is the operational edge of a real cross-process locking model plus several concurrency-control mechanisms:

1. **Conversation owner leases** — each on-disk conversation directory carries an `owner_lease.json` that "locks it to a single agent-server's `owner_instance_id` for a 45 s TTL refreshed by heartbeat"; a conflicting server raises `ConversationLeaseHeldError`. The lock manager itself lives in the backend, but this repo implements the **stale-lease recovery path**: `releaseStaleConversationLeases()` unlinks stale leases before spawning a fresh agent-server, guarded by a port-busy probe (`scripts/dev-safe.mjs:1135-1182`, used at `scripts/dev-static.mjs:601-618`).
2. **Port allocation discipline** — `findFreePort()`/`findFreePorts()` with a documented check-then-use race acceptance, `assertPortsFree()` pre-flight collision detection naming the busy ports, and sequential multi-port allocation to avoid self-races (`scripts/dev-safe.mjs:200-299`). Tested in `__tests__/scripts/dev-safe.test.ts:50-159`.
3. **Shared-shell command correlation** — `useBashCommandRunner` multiplexes UI-initiated commands over one bash-events WebSocket using FIFO queue pairing and `command_id` matching, buffering commands until the socket opens and fail-fast rejecting all queued work on error/close/unmount (`src/hooks/use-bash-command-runner.ts:49-206`; tests at `__tests__/hooks/use-bash-command-runner.test.ts:3`).
4. **Staleness/conflict visibility** — a monotonic workspace-mutation counter bumps on every agent file-editor mutation and cache-busters file URLs so the UI never shows pre-edit content (`src/stores/use-workspace-mutation-counter.ts:1-53`).
5. **Resource availability gating** — cloud sandboxes are isolated by lifecycle state: WebSocket connections to a paused sandbox's URL are suppressed and auto-resumed via fast polling (`src/contexts/websocket-provider-wrapper.tsx:25-31`, `src/routes/conversation.tsx:145-174`), and API calls are preflight-gated on `sandbox_status` (`src/hooks/query/use-bash-command-logs.ts:110-157`).

Can two tools edit the same file safely? Within this source, the UI is a reader, not a writer (the Files tab is a viewer: `src/components/features/files/file-list.tsx`, `file-item.tsx`; Monaco appears only in diff viewing), so user-vs-agent write conflicts cannot originate here. Agent-vs-agent safety across processes is delegated to the backend lease model, whose failure mode (stale leases hiding conversations) this repo actively repairs.

## Rating

**5 / 10** — Present but partial and coarse-grained. A genuine, well-documented lease model exists at the system boundary (per-conversation-directory `owner_lease.json` with TTL + heartbeat + explicit cleanup protocol), port contention has named-port diagnostics and tests, and resource unavailability (paused sandbox) is visibly handled. However: the lock manager itself is not in this source; the lease-recovery code shipped here has no direct unit test; the check-then-use port race is documented as accepted rather than fixed; and there are no fine-grained locks protecting any shared object inside the app (shared mutable state relies on single-threaded React/Zustand discipline). This matches the rubric band "present but inconsistent, weakly documented [in parts], or fragile" rather than the tested/explicit-interface band of 7-8.

## Evidence Collected

Every entry includes a file path with line numbers relative to the selected source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Frontend-only scope (no tool execution here) | AGENTS.md repository map: tool calls execute in the separate agent-server; this repo owns only UI/frontend services | `AGENTS.md` (Repository Map section) |
| Conversation lock manager (backend-side, observed via contract) | Each conversation dir carries `owner_lease.json` locking it to one agent-server `owner_instance_id`, 45 s TTL heartbeat-refreshed; conflicting server gets `ConversationLeaseHeldError` | `scripts/dev-safe.mjs:1136-1148` |
| Lease release protocol | `releaseStaleConversationLeases()` unlinks `owner_lease.json` per conversation dir, best-effort; caller MUST first verify no live server holds the port | `scripts/dev-safe.mjs:1150-1182` |
| Liveness probe guarding lease cleanup | `isPortBusy()` TCP-connect probe (500 ms timeout) to distinguish live server from stale lease | `scripts/dev-safe.mjs:1113-1133` |
| Lease cleanup invocation | `dev:static` bails out if agent-server port busy, else releases stale leases and logs count | `scripts/dev-static.mjs:592-618` |
| Port allocation race (documented) | `findFreePort` docstring admits check-then-use window; callers must handle EADDRINUSE; Vite `strictPort: true` fails fast | `scripts/dev-safe.mjs:207-236` |
| Collision detection & visibility | `assertPortsFree` throws listing each busy named port, hinting another instance is running | `scripts/dev-safe.mjs:255-285` |
| Multi-port allocation ordering | `findFreePorts` allocates sequentially "to avoid race conditions between checks" | `scripts/dev-safe.mjs:287-299` |
| Shared shell serialization | Bash commands buffered pre-auth, FIFO-paired with `BashCommand` echoes, matched by `command_id`; hung handshake aborted by watchdog so it cannot block the chat socket | `src/hooks/use-bash-command-runner.ts:49-102`, `82-85` |
| Fail-fast teardown of shared channel | `rejectAll()` clears waiting/pending/active queues on BashError, close, error, unmount | `src/hooks/use-bash-command-runner.ts:141-174` |
| Workspace edit visibility | Mutation counter bumped per agent file-editor mutation; used as query key + `?v=<n>` cache buster so previews refetch after edits | `src/stores/use-workspace-mutation-counter.ts:1-53` |
| Sandbox isolation by lifecycle | `WebSocketProviderWrapper` suppresses stale paused-sandbox URL while `sandbox_status === "PAUSED"` | `src/contexts/websocket-provider-wrapper.tsx:25-31` |
| Sandbox resume coordination | Auto-resume of PAUSED cloud sandbox; fast-poll gate in `useActiveConversation` | `src/routes/conversation.tsx:145-174`; `src/hooks/query/use-active-conversation.ts:19-28` |
| Preflight resource gating | Cloud bash-log queries skip doomed round-trips when sandbox paused/starting/errored/missing, surfacing targeted issue states | `src/hooks/query/use-bash-command-logs.ts:110-157` |
| Atomic status pairing note | Stop-conversation updates `execution_status` + `sandbox_status` together so gates fire consistently | `src/hooks/mutation/conversation-mutation-utils.ts:129`; `src/hooks/mutation/use-unified-stop-conversation.ts:58-65` |
| Tests: port allocation | `describe("findFreePort")`, fallback-to-OS-port, `findFreePorts` uniqueness/ordering cases | `__tests__/scripts/dev-safe.test.ts:50-159` |
| Tests: bash runner | Regression suite exists for the FIFO-correlated bash runner | `__tests__/hooks/use-bash-command-runner.test.ts:3` |
| No file-level editor conflicts from UI | Files tab components are viewers only (no save/edit path found) | `src/components/features/files/file-list.tsx:1`; `src/components/features/files/file-item.tsx:1` |

## Answers to Dimension Questions

1. **Which resources are shared?**
   - On-disk conversation directories (workspace + event store), shared between successive/concurrent agent-server instances — protected by `owner_lease.json` (`scripts/dev-safe.mjs:1136-1148`).
   - TCP ports among the launcher-spawned services (agent-server, automation, ingress/static server, Vite, VS Code) (`scripts/dev-safe.mjs:200-299`).
   - The conversation's shell, reached over a single `/sockets/bash-events` WebSocket shared by all UI-initiated commands (`src/hooks/use-bash-command-runner.ts:49-57`).
   - Workspace files: written only by the agent; read/cached by the UI preview layer (`src/stores/use-workspace-mutation-counter.ts:3-16`).
   - Cloud sandboxes as remote compute resources with pause/resume lifecycle (`src/api/cloud/conversation-service.api.ts:229-247`).
   - Secrets/network/browser tools: not managed here at all — they execute in the backend (AGENTS.md Repository Map; no evidence in `src/`).

2. **What protects them?**
   - Backend-enforced per-conversation leases (TTL 45 s + heartbeat + owner id) with a frontend-implemented recovery protocol gated by a liveness probe (`scripts/dev-safe.mjs:1117-1182`).
   - Pre-flight `assertPortsFree` collision detection and OS-assigned port fallback (`scripts/dev-safe.mjs:218-285`).
   - Application-level FIFO ordering + `command_id` correlation over the shared bash socket; watchdog against hung handshakes (`src/hooks/use-bash-command-runner.ts:55-85`).
   - Lifecycle gating that prevents touching unavailable/paused sandboxes (`src/contexts/websocket-provider-wrapper.tsx:25-31`; `src/hooks/query/use-bash-command-logs.ts:139-157`).

3. **Are locks coarse or fine-grained?**
   - Coarse. Isolation granularity is a whole conversation directory (`owner_lease.json` per conversation dir, `scripts/dev-safe.mjs:1171`) and whole service ports; the shell channel is one serialized FIFO stream with no per-session partitioning (`src/hooks/use-bash-command-runner.ts:68-72`). No file-level, row-level, or object-level locking exists anywhere in `src/`.

4. **Can deadlocks occur?**
   - Classic nested-lock deadlock (ABBA) is structurally absent: nothing in this source acquires two locks. Residual risks are liveness, not deadlock:
     - The documented check-then-use port window can double-bind if callers mishandle EADDRINUSE — explicitly accepted, mitigated by `strictPort` fail-fast (`scripts/dev-safe.mjs:207-212`).
     - Lease-vs-liveness ambiguity: "there is no other reliable way to tell a stale lease from an actively renewed one" than the port probe, so wrong-order cleanup could disturb a live server; the protocol mandates probing first (`scripts/dev-safe.mjs:1150-1153`), but enforcement is caller discipline, not a mechanism.
     - Hung WebSocket handshakes previously blocked the chat socket until settled; now bounded by a watchdog abort (`src/hooks/use-bash-command-runner.ts:82-85`).
   - Queued work is never allowed to wait indefinitely: close/error/unmount reject everything immediately (`src/hooks/use-bash-command-runner.ts:141-174`).

5. **Are resource conflicts visible?**
   - Yes, partially. Busy ports are reported by name with remediation hints (`scripts/dev-safe.mjs:277-284`); released stale leases are counted in startup logs (`scripts/dev-static.mjs:611-618`); sandbox issues produce targeted UI states instead of raw errors (`src/hooks/query/use-bash-command-logs.ts:110-116`); agent file edits become visible through mutation-counter-driven refetch/cache-busting (`src/stores/use-workspace-mutation-counter.ts:5-16`). What is *not* visible: lease holders' identity/duration from the UI (the lease format is backend-owned), and there is no UI surface showing which process owns a conversation right now beyond absence/presence in listings.

## Architectural Decisions

- **Defer locking to the execution backend.** The frontend performs zero tool-resource arbitration; ownership conflicts between agent-server instances are resolved by backend leases, and this repo only repairs their fallout (`scripts/dev-safe.mjs:1135-1182`). This keeps the UI stateless w.r.t. resource ownership but couples dev-stack UX to backend lease semantics.
- **Lease recovery as an explicit, guarded startup step.** Stale-lease unlinking runs only after proving the target port is free, converting an otherwise invisible failure ("new server inherits none of my conversations", `scripts/dev-static.mjs:592-600`) into a logged, self-healing action.
- **Fail-fast over blocking.** Every concurrency mechanism here prefers rejection/bail-out (port assertion throws; bash queue rejects all on socket loss; paused-sandbox requests are skipped) over waiting or retry loops, except the deliberate bounded fast-poll for sandbox resume (`src/routes/conversation.tsx:145-174`).
- **Single multiplexed channel instead of pooled channels.** One bash-events WebSocket with FIFO+id correlation rather than per-command connections simplifies auth and ordering at the cost of head-of-line blast radius (any socket loss rejects all in-flight commands, `src/hooks/use-bash-command-runner.ts:151-162`).
- **Optimistic-view staleness control.** Rather than locking files during agent edits, the UI accepts concurrent mutation and defeats staleness via version counters/cache busters (`src/stores/use-workspace-mutation-counter.ts:36-52`).

## Notable Patterns

- **File-based lease with TTL + heartbeat + owner id**, cleaned up best-effort on graceful shutdown and force-released only when provably orphaned (`scripts/dev-safe.mjs:1136-1153`).
- **Probe-then-act guard**: `isPortBusy()` TCP probe as precondition for destructive filesystem cleanup (`scripts/dev-safe.mjs:1113-1133`, `1150-1153`).
- **Sequential resource acquisition** to avoid intra-process races when allocating multiple ports (`scripts/dev-safe.mjs:287-299`).
- **Queue-correlated request/response over WebSocket**: waiting → pending (sent, awaiting echo/id) → active (id known, awaiting exit) pipeline with total teardown semantics (`src/hooks/use-bash-command-runner.ts:68-72`, `104-139`).
- **Monotonic epoch counter as cache invalidation** for externally mutated data (`src/stores/use-workspace-mutation-counter.ts:25-34`).
- **State-machine resource gating**: `sandbox_status` union type drives whether sockets/API calls may touch a sandbox (`src/api/conversation-service/agent-server-conversation-service.types.ts:11`; `src/contexts/websocket-provider-wrapper.tsx:25-31`).

## Tradeoffs

- **Simplicity vs. correctness at the port layer:** accepting the check-then-use race keeps allocation simple and predictable, but shifts correctness onto every caller handling EADDRINUSE (`scripts/dev-safe.mjs:207-212`).
- **Self-healing UX vs. safety margin:** auto-unlinking leases makes restarts seamless, but if the port probe is bypassed or races (e.g., a server binds between probe and spawn), a live server's leases could be destroyed; the design acknowledges this by making the probe a documented caller obligation rather than an enforced invariant (`scripts/dev-safe.mjs:1150-1153`).
- **Single shared bash channel vs. isolation:** easy auth and strict FIFO ordering, but one malformed server stream (`BashError`) tears down every outstanding command, and slow commands queue behind earlier ones (`src/hooks/use-bash-command-runner.ts:136-149`).
- **Read-only UI vs. conflict freedom:** because users cannot edit workspace files from this app, user-agent write conflicts vanish by construction — at the cost of requiring users to ask the agent (or external editors outside this harness) for changes.
- **Backend-owned locking vs. frontend observability:** clean separation means the UI cannot surface lease holder, TTL remaining, or steal/expire actions; operators must inspect the filesystem.

## Failure Modes / Edge Cases

- **Hard-killed agent-server leaves 45 s of phantom ownership:** conversations disappear from `/api/conversations/search` until leases expire or are force-released; this is exactly the scenario the launchers repair (`scripts/dev-safe.mjs:1141-1148`; `scripts/dev-static.mjs:592-600`).
- **Restart faster than the TTL** triggers the same hidden-conversation symptom even after graceful intent, motivating unconditional pre-start cleanup (`scripts/dev-static.mjs:594-597`).
- **Concurrent agent-canvas instances** collide on default ports; detected up front with named-port error output rather than silent port drift (`scripts/dev-safe.mjs:277-284`).
- **Socket loss mid-command batch:** all queued/in-flight bash promises reject ("Bash WebSocket closed"/"error"/"unmounted"); callers see errors rather than hangs, but results are lost, not retried (`src/hooks/use-bash-command-runner.ts:141-174`).
- **Paused cloud sandbox with stale URL:** without the wrapper's suppression the socket would hit the dead host; the gate plus 3 s polling hides the transition (`src/contexts/websocket-provider-wrapper.tsx:25-31`; `src/routes/conversation.tsx:145-174`).
- **Cloud bash-log queries against missing/paused sandboxes** would 404/5xx; preflight classification returns targeted `sandboxIssue`/`unreachable` states instead (`src/hooks/query/use-bash-command-logs.ts:110-157`).
- **No evidence found** of any frontend protection against two *backend* processes writing the same file simultaneously within one conversation — such arbitration is invisible to, and unhandled by, this source.

## Future Considerations

- Add unit tests for `releaseStaleConversationLeases()` (fixture tree with mixed lease/no-lease dirs) and for the probe-then-release ordering; currently only port allocation is directly tested (`__tests__/scripts/dev-safe.test.ts:50-159`).
- Close the check-then-use gap by binding-then-handing-off (hold the probe socket open until the child binds) or by having children report EADDRINUSE distinctly (`scripts/dev-safe.mjs:207-212`).
- Surface lease metadata (owner, expiry) in UI diagnostics so "why is my conversation missing?" becomes observable without filesystem access (`scripts/dev-safe.mjs:1141-1148`).
- Consider per-command timeouts/fairness in the bash FIFO pipeline, or a cancellation signal, so one long command cannot starve later ones indefinitely (`src/hooks/use-bash-command-runner.ts:177-203`).
- Promote the probe-before-cleanup caller obligation into a library function that composes both steps atomically, removing the documented footgun (`scripts/dev-safe.mjs:1150-1157`).

## Questions / Gaps

- **Where is the actual lease manager implemented?** Necessarily outside this source (agent-server SDK). Its exact semantics (heartbeat interval source, clock skew tolerance, steal behavior) cannot be verified here; evidence is limited to the contract described at `scripts/dev-safe.mjs:1136-1153` and consumed at `scripts/dev-static.mjs:592-618`.
- **Is `releaseStaleConversationLeases` exercised anywhere besides `dev:static`?** Searched `scripts/` and `__tests__/`: only `scripts/dev-static.mjs:610` calls it; `dev-with-automation.mjs` references were not found (searched `releaseStaleConversationLeases|owner_lease` across `__tests__/` and `scripts/`), suggesting the full automation stack may still hit the hidden-conversation window on hard restarts. No clear evidence found either way within this boundary.
- **Do any UI paths write workspace files?** None found (Files tab is viewer-only; searched `readOnly|editable|save` under `src/components/features/files/` returned no matches), consistent with the read-only-viewer conclusion, but a repo-wide exhaustive proof was out of scope.
- Browser/database/secrets isolation: no lock or sandbox configuration for these exists in this source (they are backend concerns); searches for `mutex|semaphore|acquire` across `src/` produced no hits — recorded as "No evidence found" rather than assumed safe.

---

Generated by `07.05-resource-locking-and-isolation` against `openhands`.
