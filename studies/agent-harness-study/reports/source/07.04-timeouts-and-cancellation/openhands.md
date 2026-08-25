# Source Analysis: openhands

## 07.04 — Timeouts and Cancellation

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands Agent Canvas frontend) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + Vite, TanStack Query, `@openhands/typescript-client`, WebSocket |
| Analyzed | 2026-08-23 |

## Summary

This source is the OpenHands agent-canvas **frontend**, so timeout/cancellation behavior is split between client-side deadlines it enforces itself and per-call timeouts it forwards to the agent-server. The codebase has a clear, deliberate model:

1. **WebSocket handshakes are force-closed** by a shared watchdog (`src/utils/websocket-handshake.ts:17-26`) because browsers never time out a socket stuck in `CONNECTING`; both the events socket (`src/hooks/use-websocket.ts:64`) and the bash-events socket (`src/hooks/use-bash-command-runner.ts:85`) install it.
2. **Every HTTP surface carries an explicit numeric timeout** sized per concern: backend health probe 4 s (`src/hooks/query/use-backends-health.ts:35`), `/server_info` bootstrap 5 s (`src/api/agent-server-compatibility.ts:15`), LLM balance 10 s (`src/constants/llm-balance.ts:14`), cloud proxy default 30 s (`src/api/cloud/client.ts:45`), typed-client default 60 s (`src/api/agent-server-client-options.ts:78`), conversation creation 5 min (`src/api/conversation-service/agent-server-conversation-service.api.ts:79`), cloud start-task polling bounded at 180 s (`src/services/child-conversation-launch.ts:37-38`).
3. **Tool execution is server-timed**: bash commands carry a caller-supplied `timeout` (seconds) that is sent to the agent-server rather than enforced client-side (`src/api/runtime-service/agent-server-runtime-service.ts:30,47`; `src/hooks/use-bash-command-runner.ts:99`). The cloud proxy adds a +10 s HTTP grace over the command timeout (`src/api/runtime-service/agent-server-runtime-service.ts:51`) so the transport outlives the tool.
4. **User-initiated stop is first-class**: chat and panel UIs call `pauseConversation`, which interrupts immediately on local backends (`/interrupt` cancels in-flight LLM requests) and pauses the sandbox on Cloud (`src/hooks/mutation/conversation-mutation-utils.ts:36-61`); success optimistically patches structured statuses into caches (`src/hooks/mutation/use-unified-stop-conversation.ts:63-66`).
5. **Timeouts produce structured outcomes**, not silent hangs: `"timeout"` compaction outcome (`src/hooks/use-await-context-compaction.ts:11,150-154`), `error_kind: "timeout"` for MCP OAuth probes (`src/api/mcp-service/mcp-service.api.ts:304-308`), `CANCELLED` automation-run status with an explicit cancel API (`src/api/automation-service/automation-service.api.ts:446-451`).
6. **Cancellation is cooperative** via `AbortController` where long-running browser work exists — device-flow polling (`src/hooks/use-device-flow.ts:81-192`) and update downloads (`src/api/agent-canvas-updates.ts:22-25`) — plus exhaustive cleanup handlers that reject queued commands on socket close/error/unmount (`src/hooks/use-bash-command-runner.ts:141-174`).

The model is well tested at the seams where hangs actually occur (handshake watchdogs, device-flow timeout, compaction timeout, MCP timeout classification). Its main weakness is that timeout values are scattered as per-module constants with no central registry, and the bash runner has no client-side deadline of its own — it trusts the server to honor the forwarded timeout.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale against the rubric:

- **Why not lower:** every network path has a named, documented timeout constant (e.g., the rationale comment at `src/constants/llm-balance.ts:8-13` explains why a stuck request must be bounded when `retry: false` suppresses retries); handshake hang protection exists with dedicated tests (`__tests__/hooks/use-websocket.test.ts:533`, `__tests__/hooks/use-bash-command-runner.test.ts:135`); cancellation produces typed outcomes and cache-visible statuses; unmount cleanup rejects all pending promises so nothing leaks.
- **Why not higher:** timeouts are ad-hoc per-call constants rather than one configurable policy (no user-facing global "tool timeout" setting in this repo); most service calls cannot be cancelled mid-flight (numeric `timeoutSeconds` only — `AbortSignal` is threaded in just two places: `src/hooks/use-device-flow.ts:134`, `src/api/agent-canvas-updates.ts:22`); the bash command runner forwards its timeout but never enforces it locally, so a misbehaving server leaves the promise pending until the socket closes.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| WebSocket handshake watchdog | 10 s `HANDSHAKE_TIMEOUT_MS`; closes sockets stuck in `CONNECTING` because browsers never do; abort flows through normal close handling via cancel fn | `src/utils/websocket-handshake.ts:1-26` |
| Watchdog installed on events socket | `startHandshakeWatchdog(ws)` before `onopen`; cancelled in open/close | `src/hooks/use-websocket.ts:64-84` |
| Watchdog installed on bash-events socket | watchdog started immediately after socket creation, cancelled in open/close/unmount cleanup | `src/hooks/use-bash-command-runner.ts:85,88,152,164-166` |
| Reconnect backoff bounds | 1 s base doubling to 30 s cap with ≤30 % jitter so parallel sockets don't retry in lockstep | `src/hooks/use-websocket.ts:18-20,125-136` |
| Typed-client default timeout | `timeout: timeout ?? 60000` assembled centrally for all SDK clients | `src/api/agent-server-client-options.ts:74-78` |
| Cloud client default timeout | `timeout: 30_000` default, overridable per request via `timeoutSeconds` | `src/api/cloud/client.ts:42-45` |
| `/server_info` bootstrap timeout | `AGENT_SERVER_INFO_TIMEOUT_MS = 5000` gates availability detection | `src/api/agent-server-compatibility.ts:15,345` |
| Runtime-services fetch fail-fast | `getAgentServerClientOptions({ timeout: 3000 })` for optional metadata | `src/api/agent-server-adapter.ts:178` |
| Backend health probe timeout | `PROBE_TIMEOUT_MS = 4000` passed to each probe | `src/hooks/query/use-backends-health.ts:35,121` |
| Probe failure circuit breaker | polling stops after repeated failures ("Once a backend has failed this many probes in a row, polling stops") | `src/api/backend-registry/health-store.ts:43`; `src/api/backend-registry/health-storage.ts:6` |
| LLM balance hard ceiling | `LLM_BALANCE_TIMEOUT_MS = 10_000` because `retry: false` does not bound a stuck request; timeout stays an error instead of collapsing to cached `null` | `src/constants/llm-balance.ts:8-14`; `src/api/llm-balance-service.ts:57-69` |
| Timeout vs manual abort classification | `isAbortLike` distinguishes `TimeoutError` (`AbortSignal.timeout`) from `AbortError` (manual abort) | `src/api/llm-balance-service.ts:34-40` |
| Per-command tool timeout forwarding | `executeCommand(..., timeout = 30)` sends `timeout: Math.floor(timeout)` to `/api/bash/execute_bash_command` or `RemoteWorkspace.executeCommand` | `src/api/runtime-service/agent-server-runtime-service.ts:25-62` |
| Transport outlives tool | cloud proxy `timeoutSeconds: timeout + 10` grace so HTTP doesn't cut off a still-running command | `src/api/runtime-service/agent-server-runtime-service.ts:51` |
| Bash WS wire format includes `timeout` | `{ command, cwd, timeout }` JSON frames; FIFO queue correlates echoes by `command_id` | `src/hooks/use-bash-command-runner.ts:91-101,177-203` |
| Cleanup on socket close/error/unmount | `rejectAll()` rejects waiting/pending/active commands; unmount nulls handlers then closes socket to avoid double-reject | `src/hooks/use-bash-command-runner.ts:141-174` |
| Conversation create timeout override | `CREATE_CONVERSATION_TIMEOUT_MS = 5 * 60 * 1000` because provisioning "can exceed the client's 60 s default" | `src/api/conversation-service/agent-server-conversation-service.api.ts:78-79,490` |
| Bounded cloud start-task poll | `CLOUD_START_POLL_INTERVAL_MS = 3_000`, `CLOUD_START_POLL_TIMEOUT_MS = 180_000`; deadline loop returns still-provisioning task instead of hanging | `src/services/child-conversation-launch.ts:37-38,365-384` |
| User stop: local interrupt semantics | `pauseConversation`: Cloud pauses sandbox (waits for current LLM call); local uses `/interrupt` so "in-flight LLM requests are cancelled immediately" | `src/hooks/mutation/conversation-mutation-utils.ts:36-61` |
| User stop UI wiring | `ChatStopButton` (`data-testid="stop-button"`), pause/resume mutations, pending-state flag | `src/components/features/chat/chat-stop-button.tsx:7-18`; `src/components/features/chat/components/chat-input-actions.tsx:162-173` |
| Stop confirmation modal | `ConfirmStopModal` wired through conversation-name context menu | `src/components/features/conversation-panel/confirm-stop-modal.tsx`; `src/components/features/conversation/conversation-name.tsx:249-252` |
| Optimistic cancelled status patch | success sets `execution_status: PAUSED` + `sandbox_status: "PAUSED"` so the WS gate fires before the next poll | `src/hooks/mutation/use-unified-stop-conversation.ts:58-71` |
| Pending-message withdrawal | queued user messages can be dropped ("e.g., after success/cancellation") with per-message stop handler | `src/stores/optimistic-user-message-store.ts:69`; `src/components/features/chat/pending-user-messages.tsx:87-126` |
| Automation run cancellation API | `cancelAutomationRun` → `POST /api/automation/v1/runs/{run_id}/cancel`; UI exposes cancel only for in-flight runs with pending-state tracking | `src/api/automation-service/automation-service.api.ts:446-451`; `src/hooks/use-home-automation-actions.ts:50-60,114-133` |
| Automation run timeout config | per-automation `timeout` field (default 600 s) validated positive-int and capped by deployment-reported max | `src/utils/automation-timeout.ts:7-37`; `src/manifests/types.ts:109,193`; `src/manifests/automation-setup.ts:313` |
| Structured timeout result (compaction) | `ContextCompactionOutcome = "compacted" \| "no_change" \| "timeout"`; 90 s default timer treats missing Condensation event as failure; consumer branches on `outcome === "timeout"` | `src/hooks/use-await-context-compaction.ts:7-24,150-154`; `src/hooks/use-compact-context-action.ts:52` |
| Structured timeout result (MCP OAuth) | poll loop bounded by `OAUTH_MCP_TEST_TIMEOUT_SECONDS = 120`; expiry returns `{ ok: false, error_kind: "timeout" }`; probe client gets +5 s headroom | `src/api/mcp-service/mcp-service.api.ts:21,143-149,288-308` |
| Per-server MCP test timeout | `getMcpTestTimeout` prefers server-declared `timeout`, overrides to OAuth ceiling for oauth2 servers | `src/api/mcp-service/mcp-service.api.ts:44-47` |
| Timeout error classification for logs UI | `AbortError`/`TimeoutError` (incl. wrapped `cause`) mapped to `unreachable` sandbox issue instead of raw error | `src/hooks/query/use-bash-command-logs.ts:80-98` |
| AbortController lifecycle (device flow) | ref-held controller; `start` aborts prior flow; token polling receives `signal`; `cancel`/`reset`/unmount all abort; aborted guards prevent stale state writes | `src/hooks/use-device-flow.ts:81-192` |
| AbortSignal parameter passthrough | update-download fetch accepts optional `signal` | `src/api/agent-canvas-updates.ts:22-25` |
| Handshake watchdog tests | "closes a handshake stuck in CONNECTING at the timeout and retries" (events socket); same for bash socket | `__tests__/hooks/use-websocket.test.ts:533-584`; `__tests__/hooks/use-bash-command-runner.test.ts:135-153` |
| Device-flow timeout test | 50 ms timeout configured; expects rejection matching `/timeout/i` | `__tests__/api/device-flow-client.test.ts:372-378` |
| Compaction timeout test | "reports timeout when no condensation event arrives in time" asserting `outcome: "timeout"` | `src/hooks/use-await-context-compaction.test.ts:119-137` |
| MCP timeout classification test | mutation returns `ok=false` with `error_kind=timeout` on timeout failure | `__tests__/hooks/mutation/use-test-mcp-server.test.ts:66-85` |
| Automation timeout validation tests | positive-int/max-ceiling validation covered directly | `__tests__/utils/automation-timeout.test.ts:3+` |
| Profiles service non-blocking timeout | 30 s timeout on provider-profile fetch; transient errors/timeouts treated as non-blocking `null` | `src/api/profiles-service/profiles-service.api.ts:153-165` |

## Answers to Dimension Questions

1. **Can a tool hang forever?**
   Mostly no, with one caveat. HTTP calls always have an explicit bound (smallest 3 s at `src/api/agent-server-adapter.ts:178`; largest 5 min at `src/api/conversation-service/agent-server-conversation-service.api.ts:79`), and WebSocket *handshakes* are force-closed at 10 s (`src/utils/websocket-handshake.ts:5,20-24`). However, once a bash command is accepted over the bash-events socket, the client-side promise resolves only on a terminal `BashOutput(exit_code)` frame, a `BashError`, or socket close/error/unmount (`src/hooks/use-bash-command-runner.ts:104-139,151-162`) — there is no client-side deadline ticking down the forwarded `timeout`. A server that accepts the frame but never emits output keeps that promise pending until the socket dies. The actual kill switch is the server-side timeout carried in the frame (`src/api/runtime-service/agent-server-runtime-service.ts:47`).

2. **Are timeouts configurable?**
   Per call site, yes; globally, no. Each service fixes its own named constant (`PROBE_TIMEOUT_MS`, `LLM_BALANCE_TIMEOUT_MS`, `AGENT_SERVER_INFO_TIMEOUT_MS`, `CREATE_CONVERSATION_TIMEOUT_MS`, etc.) and several accept per-request overrides (`timeoutSeconds` on cloud-proxy requests, `src/api/cloud/proxy.ts:11,30`; client-option overrides, `src/api/agent-server-client-options.ts:12,67`). Tool-level timeouts are genuinely user-configurable in two places: bash commands take a caller-supplied seconds value (`src/hooks/use-local-git-info.ts:146-147` threads it through), and automations expose a validated per-run `timeout` capped by a server-advertised ceiling (`src/utils/automation-timeout.ts:19-37`; ceiling documented at `src/manifests/types.ts:193`, test coverage of the cap seam at `__tests__/manifests/automation-interface.test.ts:63,73`). There is no single settings key that scales all timeouts.

3. **Can users cancel?**
   Yes, at three levels: (a) the running agent turn — a visible stop button issues `pauseConversation` (`src/components/features/chat/components/chat-input-actions.tsx:162-165`, button at `src/components/features/chat/chat-stop-button.tsx:7-18`, with confirmation dialog `src/components/features/conversation-panel/confirm-stop-modal.tsx` and sidebar equivalents `src/components/features/conversation/conversation-name-with-status.tsx:59-63`); (b) queued-but-not-sent chat messages can be withdrawn (`src/components/features/chat/pending-user-messages.tsx:87-126`); (c) automation runs have an explicit cancel endpoint surfaced only while a run is in flight (`src/api/automation-service/automation-service.api.ts:446-451`; `src/hooks/use-home-automation-actions.ts:114-133`).

4. **Is cancellation cooperative or forced?**
   Cooperative and backend-dependent. Locally, `/interrupt` is the closest to forced: the docstring states in-flight LLM requests "are cancelled immediately rather than waiting for the current call to finish" (`src/hooks/mutation/conversation-mutation-utils.ts:55-60`). On Cloud, stopping means pausing the sandbox, which explicitly "waits for current LLM call to finish" (`src/hooks/mutation/conversation-mutation-utils.ts:37-52`) — cooperative drain. Browser-side cancellation is cooperative throughout: `AbortController.abort()` plus `signal.aborted` checks between async steps (`src/hooks/use-device-flow.ts:100,117,138,149`), and the goal-loop's own doc notes the backend "deliberately leaves the in-flight agent turn running" so callers must also interrupt (`src/hooks/mutation/conversation-mutation-utils.ts:94-99`).

5. **Does cancellation leave resources dirty?**
   No evidence of leaks in the paths studied. Cancellation converges to explicit structured state: stop patches `execution_status: PAUSED` + `sandbox_status: "PAUSED"` into both list and detail query caches so downstream gates react immediately (`src/hooks/mutation/use-unified-stop-conversation.ts:58-66`); automation cancels return the updated `AutomationRun` (status `CANCELLED`, excluded from duration analytics — `src/fixtures/home-automations-demo.ts:325`; `src/manifests/automation-insights.ts:12`); aborted device flows reset to initial state and never write stale results (`src/hooks/use-device-flow.ts:148-159,175-192`). Unmount teardown is thorough: bash runner rejects all queued commands and closes the socket (`src/hooks/use-bash-command-runner.ts:164-174`), the WS hook disables reconnection before closing and clears pending reconnect timers (`src/hooks/use-websocket.ts:164-188`). One residual risk: if the bash-events socket stays healthy but the server drops a command silently, its entry lingers in `activeCommandsRef` until some later close/error sweeps it (`src/hooks/use-bash-command-runner.ts:72,141-149`) — bounded memory-wise but unresolved promise-wise.

## Architectural Decisions

- **Server owns tool deadlines; client owns transport deadlines.** Command `timeout` values are payload fields handed to the agent-server (`src/hooks/use-bash-command-runner.ts:99`; `src/api/runtime-service/agent-server-runtime-service.ts:44-48`), while the frontend guarantees its own layers never hang (handshake watchdog, per-request HTTP timeouts). This respects the repo's boundary rule that tool execution lives in the agent-server, not the frontend (see repository map in `AGENTS.md`).
- **Grace-over-tool pattern.** The cloud proxy transport timeout is derived as `timeout + 10` s so the HTTP hop can never expire before the command it carries (`src/api/runtime-service/agent-server-runtime-service.ts:51`) — a deliberate ordering invariant rather than two independent numbers.
- **Shared, reusable hang-protection primitive.** `startHandshakeWatchdog` is a tiny pure utility consumed identically by both socket hooks (`src/utils/websocket-handshake.ts:17-26`; consumers at `src/hooks/use-websocket.ts:64`, `src/hooks/use-bash-command-runner.ts:85`), returning a cancel function so the watchdog never outlives its socket.
- **Timeout-as-error, not timeout-as-absence.** The balance service deliberately rethrows timeouts instead of mapping them to `null`, because `null` is cached with `staleTime: Infinity` and would hide the balance card permanently (`src/api/llm-balance-service.ts:62-69`). Error identity is preserved by distinguishing `TimeoutError` from `AbortError` (`src/api/llm-balance-service.ts:34-40`).
- **Optimistic status convergence after stop.** Rather than waiting for the next poll, stop writes `PAUSED` statuses into TanStack Query caches synchronously so the WebSocket gate and UI react instantly (`src/hooks/mutation/use-unified-stop-conversation.ts:58-71`; gate described in the same block).
- **Capability-negotiated ceilings for user-facing timeouts.** Automation timeout validation takes an optional `maxSeconds` from capability discovery, rejecting oversized values before the request leaves the browser (`src/utils/automation-timeout.ts:33-35`; seam test `__tests__/manifests/automation-interface.test.ts:63`).

## Notable Patterns

- **Watchdog + normal-path reuse**: closing a stuck CONNECTING socket fires its regular `error`/`close` (code 1006), so the abort reuses existing reconnect/reject handling instead of a parallel failure path (`src/utils/websocket-handshake.ts:7-11`; relied upon by `src/hooks/use-websocket.ts:61-63`).
- **Jittered exponential backoff** for reconnects (1 s→30 s cap, ≤30 % jitter) specifically to de-synchronize the main and planning sockets hammering a struggling server (`src/hooks/use-websocket.ts:18-20,125-132`).
- **Reject-all sweep**: a single `rejectAll(reason)` helper fans rejection across the three command queues (waiting/pending/active) on any fatal socket event, guaranteeing no orphaned promises (`src/hooks/use-bash-command-runner.ts:141-156`).
- **Deadline-loop polling**: bounded `while` loops with wall-clock deadlines that return partial results (the still-provisioning task) instead of throwing or hanging (`src/services/child-conversation-launch.ts:369-384`; OAuth status loop `src/api/mcp-service/mcp-service.api.ts:288-302`).
- **Preflight gating before firing doomed requests**: cloud log fetches check `sandbox_status` first and skip the request for paused/starting/errored/missing sandboxes (`src/hooks/query/use-bash-command-logs.ts:110-157`).
- **Structured outcome unions**: `"timeout"` appears as a first-class member of result types (`ContextCompactionOutcome`, `src/hooks/use-await-context-compaction.ts:11`; MCP `error_kind`, `src/api/mcp-service/mcp-service.api.ts:307`) rather than being conflated with generic errors.

## Tradeoffs

- **Server-trusted tool timeouts vs client simplicity**: forwarding `timeout` avoids duplicating process management in the browser but means correctness depends on server compliance; there is no belt-and-braces client timer (`src/hooks/use-bash-command-runner.ts:177-203`).
- **Cooperative Cloud pause vs instant local interrupt**: pausing preserves the sandbox and current LLM spend but users on Cloud wait for the in-flight call; local users get immediate cancellation — same button, different latency contract (`src/hooks/mutation/conversation-mutation-utils.ts:36-61`).
- **Many precise constants vs configurability**: hand-tuned per-endpoint timeouts (3 s–5 min) give tight failure detection but scatter policy across ~10 modules; changing global responsiveness requires touching each file (`src/api/agent-server-adapter.ts:178` … `src/api/conversation-service/agent-server-conversation-service.api.ts:79`).
- **Numeric timeouts vs signal propagation**: the TypeScript-client option style (`timeoutSeconds`) is simple but makes mid-flight cancellation impossible for those calls; only device flow and update download support true abort (`src/api/agent-server-client-options.ts:78` vs `src/api/agent-canvas-updates.ts:22`).

## Failure Modes / Edge Cases

- **Silent server drop of a bash command**: promise remains pending in `activeCommandsRef` until socket close/error/unmount triggers `rejectAll` (`src/hooks/use-bash-command-runner.ts:122-135,151-162`).
- **Replaced-socket races**: late close/error events from deliberately replaced or watchdog-killed sockets are filtered via a `WeakSet` allowlist so they neither clobber the new socket's OPEN state nor trigger spurious errors (`src/hooks/use-websocket.ts:30-31,105-108,142-151`).
- **Unmount double-rejection**: cleanup nulls `onclose`/`onerror` before `ws.close()` so the sweep runs exactly once (`src/hooks/use-bash-command-runner.ts:164-174`).
- **Timeout masked as "endpoint absent"**: guarded explicitly — 404 maps to `null` (hide UI) but abort-like errors rethrow (`src/api/llm-balance-service.ts:84-97`), and log-fetch classification distinguishes auth bugs (surfaced) from unreachable sandboxes (rendered as empty state) (`src/hooks/query/use-bash-command-logs.ts:74-78`).
- **Stale async writes after cancellation**: every continuation in the device flow re-checks `signal.aborted` before `setState`, including error paths (`src/hooks/use-device-flow.ts:100,117,138,149`).
- **Compaction ack ambiguity**: the HTTP `/condense` response only means work *started*; a missing Condensation event within 90 s is reported as `"timeout"` (failure), never as a successful no-op (`src/hooks/use-await-context-compaction.ts:57-61,150-154`).

## Future Considerations

- Add a client-side deadline to `useBashCommandRunner` that auto-rejects active commands whose forwarded `timeout` elapses without a terminal `BashOutput`, mirroring the server contract (`src/hooks/use-bash-command-runner.ts:177-203`).
- Centralize the timeout constants into one module (or read them from `/server_info` capabilities like the automation ceiling) so operational tuning is a one-file change.
- Thread `AbortSignal` through the typed-client option helpers (`src/api/agent-server-client-options.ts:12-19`) so long calls (conversation create at 5 min, `src/api/conversation-service/agent-server-conversation-service.api.ts:79`) can be cancelled when the user navigates away.

## Questions / Gaps

- **No evidence found** in this repository for server-side enforcement details of the forwarded bash `timeout` (kill semantics, SIGKILL escalation): the executor lives in the separate `software-agent-sdk` repo, which is outside this study's isolation boundary. What was searched: all files under `src/api/runtime-service/`, `src/hooks/use-bash-command-runner*`, and `AGENTS.md`.
- No evidence found for per-agent-step or LLM-inference timeout configuration surfaces in this frontend beyond the generic settings schema mention ("Maximum number of agent steps allowed before the conversation stops", `src/mocks/settings-handlers.ts:390`) — that is step-count, not time-based, and the mock handler may not reflect production schemas.
- Whether the planning-agent sub-conversation's legacy `resend_all` socket also installs the handshake watchdog could not be confirmed from a single authoritative file; the shared hook (`useWebSocket`) applies wherever it is used, but the planning socket's exact wiring was outside the sampled set.

---

Generated by dimension `07.04-timeouts-and-cancellation` against `openhands`.
