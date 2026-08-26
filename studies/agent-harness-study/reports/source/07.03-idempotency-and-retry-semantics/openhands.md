# Source Analysis: openhands

## Dimension 07.03: Idempotency and Retry Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 + TanStack Query + Zustand (agent-canvas frontend); one Python tool shim (`tools/canvas_ui_tool.py`) |
| Analyzed | 2026-08-26 |

All citations below are relative to the source root `studies/agent-harness-study/sources/openhands/`.

## Summary

This source is the OpenHands **frontend** (agent-canvas), so its "tool retries" story lives in three places rather than in an agent loop: (1) generic API-call retry helpers, (2) React Query per-query/per-mutation retry policy, and (3) client-executed tools whose side effects the browser owns.

The standout finding is that the repo treats idempotency as a **per-tool, explicitly declared property** and protects its one dangerous outward tool with a real duplicate-detection mechanism. Client tool specs carry MCP-style annotations — `canvas_ui_control` declares `idempotentHint: true` (`src/api/canvas-ui-client-tool.ts:85-90`) while `launch_child_conversation` declares `idempotentHint: false`, `readOnlyHint: false`, `openWorldHint: true` (`src/api/launch-child-conversation-client-tool.ts:103-111`). The launch tool is guarded by a persisted claim ledger (`claimToolCall`, `src/services/child-conversation-launch.ts:205-227`) that records handled `tool_call_id`s in localStorage *before any network work*, so a replayed ActionEvent (socket reconnect replay or REST/WS race) cannot start a second — on Cloud, billable — conversation (`src/services/child-conversation-launch.ts:196-204`). This is tested directly (`__tests__/services/child-conversation-launch.test.ts:488-506`).

A second defense layer deduplicates WebSocket reconnect replays at the event level: before running non-idempotent UI side effects (error banners, telemetry, cache invalidation, client-tool dispatch), both message handlers check the event store's `eventIds` set because "the store dedups by id, but the side-effects below aren't idempotent" (`src/contexts/conversation-websocket-context.tsx:556-568`, mirrored for the planning agent at `:788-801`; regression tests at `__tests__/contexts/conversation-websocket-context.test.tsx:394-548`).

By contrast, the generic HTTP retry helper is deliberately dumb: `withRetry` (`src/api/with-retry.ts:4-26`) retries *any* failure up to 3 times with exponential backoff and classifies nothing. It is applied to reads and to mutations that are idempotent by construction (secret PUT-upserts, DELETE-with-404-tolerant semantics, diff-based PATCH saves). React Query mutations default to zero retries and no mutation hook overrides that, so user-triggered writes are never auto-retried. Retries are visible to users via explicit Retry buttons and toast dedup, and to the model via corrective-guidance result messages ("retry only if the cause looks transient", `src/services/child-conversation-launch.ts:525`), but successful background retries are entirely silent — no log, trace, or attempt counter is surfaced.

**Can a payment/email/delete tool be retried safely?** For this harness's own outward tool (conversation launch ≈ a billable "payment-like" action): yes — it is annotated non-idempotent, claimed pre-network against a persisted ledger, and tested against replays. But there is no general mechanism a future non-idempotent tool would inherit; safety relies on each tool author repeating the ledger pattern, and the blanket `withRetry` would happily retry an unclassified failing POST if someone wired it up.

## Rating

**Score: 6 / 10** — Present but inconsistent: strong, tested safeguards around the highest-risk paths, undermined by unclassified generic retries and scattered per-query policy.

Rationale:
- **For 7–8**: an explicit idempotency store with pre-network claiming and replay tests exists (`src/services/child-conversation-launch.ts:205-227`, test at `__tests__/services/child-conversation-launch.test.ts:490-506`); idempotency is a first-class, model-visible tool annotation (`src/api/canvas-ui-client-tool.ts:17-19`, `src/api/launch-child-conversation-client-tool.ts:103-111`); WS-replay side effects are deduped with named-regression coverage (`#1656`); error classification gates retries in the health probe (`src/hooks/query/use-backends-health.ts:164-186`); jittered bounded backoff on reconnects (`src/hooks/use-websocket.ts:125-136`).
- **Not 8+**: the shared `withRetry` has no error classification at all and is copy-pasted a second time into `src/api/settings-service/settings-service.api.ts:134-156` (drift risk); retry policies are ad-hoc per hook across dozens of files with no central policy module; intermediate retries are unobservable (no logging/tracing/attempt counters); the launch ledger is localStorage-only with acknowledged multi-tab and storage-full failure modes (`src/services/child-conversation-launch.ts:222-225`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Generic retry wrapper | `withRetry(fn, maxRetries=3, baseDelayMs=500)` — exponential backoff (`delay = baseDelayMs * 2 ** attempt`), rethrows last error; retries every caught error, no classification, no jitter | `src/api/with-retry.ts:4-26` |
| Duplicated retry helper | Second identical `withRetry` inside the settings service — same signature/backoff, independent evolution risk | `src/api/settings-service/settings-service.api.ts:134-156` |
| Retryable vs non-retryable classification (health probe) | `isRetryableProbeError` excludes decided auth failures (`INVALID_BACKEND_API_KEY_ERROR`, `MISSING_BACKEND_API_KEY_ERROR`, `CLOUD_BACKEND_LOGGED_OUT_ERROR`); `probeBackendWithQuickRetry` does 2 fixed-delay (300 ms) attempts; rationale comment explains why retry lives inside the query fn (one outcome recorded per logical probe) | `src/hooks/query/use-backends-health.ts:164-186`, `:140-141` |
| Backend health circuit breaker | After `MAX_CONSECUTIVE_FAILURES` consecutive failures polling stops; failure count + last error persisted to localStorage so refresh doesn't silently re-arm polling | `src/hooks/query/use-backends-health.ts:220-227` |
| WebSocket reconnect backoff | Exponential base 1 s capped at 30 s with up to 30 % random jitter "so parallel sockets (main + planning) don't retry in lockstep"; `maxAttempts` option defaults to Infinity | `src/hooks/use-websocket.ts:19-20`, `:110-140` |
| React Query read-retry policy (bounded) | Conversation-history query `retry: 1` with rationale: a slow initial load must not hold the WebSocket gate closed; misses arrive via WS `since` replay instead | `src/hooks/query/use-conversation-history.ts:83-96` |
| React Query read-retry predicates | Settings query retries only when status ≠ 404 (`retry: (_, error) => getErrorStatus(error) !== 404`); git-sync and local-workspaces use custom `(failureCount, error)` predicates; vscode URL `retry: 3` | `src/hooks/query/use-settings.ts:145`; `src/hooks/query/use-git-sync.ts:30-33`; `src/hooks/query/use-local-workspaces.ts:20`; `src/hooks/query/use-unified-vscode-url.ts:114` |
| Mutations not auto-retried | No `retry` option in any of ~40 mutation hooks (create/delete conversation, secrets, MCP servers, automations…); TanStack Query mutation default (0 retries) applies | e.g. `src/hooks/mutation/use-create-conversation.ts:80`; `src/hooks/mutation/use-delete-secret.ts:6`; `src/hooks/mutation/use-automations.ts:86-114` |
| Idempotency store (tool-call ledger) | `claimToolCall(parentConversationId, toolCallId)` — localStorage key `openhands-child-conversation-launches:<convId>`, array of handled ids; returns false if already claimed; corrupt JSON → fresh ledger; storage-full → proceed "accepting replay risk over never launching" | `src/services/child-conversation-launch.ts:196-227` |
| Claim-before-side-effect ordering | Handler claims the call on line 1 before validation/network work, "so a replay that arrives mid-flight is dropped too" | `src/services/child-conversation-launch.ts:505-510` |
| Replay-duplicate detection (events) | Both WS handlers check `useEventStore.getState().eventIds.has(event.id)` before non-idempotent side effects (#1656); event store keeps an O(1) `eventIds` Set, skipping transient deltas | `src/contexts/conversation-websocket-context.tsx:556-568`, `:788-801`; `src/stores/use-event-store.ts:92-107`, `:159-178` |
| Replay volume control | Socket subscribes with `resend_mode='since'` + `after_timestamp` anchor from the REST-seeded tail, falling back to `resend_mode='all'` only when history failed | `src/contexts/conversation-websocket-context.tsx:966-973`, `:361-390` |
| Tool idempotency annotations (idempotent) | `canvas_ui_control` client tool spec: `readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false`; description adds "don't repeat it for the same file or tab in the same turn" | `src/api/canvas-ui-client-tool.ts:13-19`, `:85-90`, `:56-58` |
| Tool idempotency annotations (non-idempotent) | `launch_child_conversation`: `readOnlyHint: false … idempotentHint: false … openWorldHint: true` with comment "Launching a conversation writes real state and is not repeatable"; description: "Do NOT call this tool twice for the same task" | `src/api/launch-child-conversation-client-tool.ts:38`, `:103-111` |
| Annotations shipped to the agent-server | Start-conversation payload embeds `client_tools[0].annotations` incl. `idempotentHint`; asserted in adapter test | `__tests__/api/agent-server-adapter.test.ts:689-706`; also `__tests__/tools/launch-child-conversation-tool.test.ts:37-41` |
| Legacy Python tool parity | Server-side `CanvasUITool` registers the same `ToolAnnotations(idempotentHint=True, readOnlyHint=True)` | `tools/canvas_ui_tool.py:126-131` |
| Idempotent-by-design retried mutations | Secret create = PUT upsert retried (`withRetry(() => upsertSecret(...))`); delete tolerates 404 as success (SDK `HttpError` or axios-shaped status check) making repeated deletes safe; update = fetch-value + re-upsert + conditional delete | `src/api/secrets-service.ts:60-76`, `:106-121`, `:130-154` |
| Diff-based settings saves retried | `saveSettings` sends `*_diff` merge patches under `withRetry`; MCP config/server create/delete likewise — replays converge because the server deep-merges diffs | `src/api/settings-service/settings-service.api.ts:537-551`, `:556-601`, `:607-694` |
| Semantic-changing fallback retry | Local launch: if worktree creation fails, one retry as `shared` isolation with an explanatory `isolation_note` returned to the agent; original error preserved if the fallback also fails; no retry when the child never asked for a worktree | `src/services/child-conversation-launch.ts:308-323`; tests `__tests__/services/child-conversation-launch.test.ts:324-356`, `:358-393` |
| Bounded async poll | Cloud sandbox provisioning polled every 3 s with a 180 s deadline; timeout reports still-provisioning task instead of hanging | `src/services/child-conversation-launch.ts:36-38`, `:365-384` |
| Pagination anti-thrash | Cloud events search returns an empty page (not a retry) when timestamp filters are unsupported, "so the UI doesn't retry indefinitely" | `src/api/event-service/event-service.api.ts:149-163` |
| Duplicate request guard (pagination) | `useLoadOlderEvents` uses ref-based `isLoadingRef`/`hasMoreRef` guards because scroll/wheel/effect triggers can fire in the same tick | `src/hooks/use-load-older-events.ts:55-56`, `:89-97`, `:122-123` |
| Retry visibility — user | Error banner exposes a Retry button, but only wired for server-connection errors (→ websocket `reconnect()`); pending user messages expose Retry/Dismiss/Stop per status; banner icon switches on server-provided `classification.kind` | `src/components/features/chat/chat-interface.tsx:589-593`; `src/components/features/chat/pending-user-messages.tsx:114-128`; `src/components/features/chat/error-message-banner.tsx:103-120`, `:172-181` |
| Retry visibility — model | Launch handler "never rejects": failures become corrective guidance posted into the parent conversation ("Report the error to the user; retry only if the cause looks transient", "you can retry, or fall back to target=\"local\""); optimistic user messages cleared by FIFO text match on echo | `src/services/child-conversation-launch.ts:459-497`, `:428`, `:499-536`; echo matching `src/contexts/conversation-websocket-context.tsx:610-624` |
| Retry visibility — errors surfaced once | Global QueryCache/MutationCache onError shows one toast per unique message within a 3 s window (`shownErrors` Set) so retry storms don't spam; 401s trigger auth invalidation instead | `src/query-client-config.ts:29`, `:41-77` |
| Server-side error classification consumed | Error events carry `classification` (`{kind, retryable, user_action}`) forwarded to banner store and telemetry; tests pin forwarding incl. `retryable: false` and `retryable: true` variants | `src/utils/error-handler.ts:2-7`; `__tests__/contexts/conversation-websocket-context.test.tsx:550-598`, `:600-639` |

## Answers to Dimension Questions

1. **Which tool failures are retried?**
   Three regimes. (a) Plain service-layer calls wrapped in `withRetry` (`src/api/with-retry.ts:4-26`) retry *every* thrown error — including deterministic 4xx — up to 3 times: secret list/upsert/delete (`src/api/secrets-service.ts:15-17`, `:69-76`, `:133-138`), settings PATCHes (`src/api/settings-service/settings-service.api.ts:540-550`, `:678-693`). (b) React Query reads have per-hook policies ranging from `retry: false` (~30 hooks) through bounded counts (`retry: 1` for history, `src/hooks/query/use-conversation-history.ts:96`) to custom predicates that skip 404/auth errors (`src/hooks/query/use-settings.ts:145`). (c) Mutating React Query mutations are never auto-retried (no hook sets `retry`; default is 0). Client tools executed by the browser (canvas UI, child launch) are never auto-retried by the harness; the *agent* decides, guided by result messages.

2. **Are repeated attempts safe?**
   Where automatic retries exist, yes — by construction: all `withRetry` targets are reads, name-keyed upserts, 404-tolerant deletes (`src/api/secrets-service.ts:139-154`), or server-side deep-merged diff PATCHes (`src/api/settings-service/settings-service.api.ts:607-694`). Repeated deliveries of the same event are made safe by id-based dedup before side effects (`src/contexts/conversation-websocket-context.tsx:556-568`) and by the launch ledger (`src/services/child-conversation-launch.ts:205-227`). The one semantic-changing retry (worktree→shared fallback) downgrades isolation but reports the change to the agent (`:308-323`).

3. **Is retry state persisted?**
   Partially. The launch ledger persists across reloads in localStorage keyed by parent conversation (`src/services/child-conversation-launch.ts:206`); backend-health failure counts persist too (`src/hooks/query/use-backends-health.ts:220-227`). WS reconnect attempt counters and all `withRetry` loop state are in-memory only. There is no server-side or cross-device retry/idempotency state.

4. **Are non-idempotent tools protected?**
   Yes, for the one non-idempotent tool the frontend ships. Protection is fourfold: schema-level annotation sent to the LLM (`idempotentHint: false`, `src/api/launch-child-conversation-client-tool.ts:103-111`); prompt guidance ("Do NOT call this tool twice for the same task", `:38`); a pre-network claim ledger keyed by `tool_call_id` (`src/services/child-conversation-launch.ts:205-227`, invoked at `:510`); and regression tests proving a replayed call launches nothing twice (`__tests__/services/child-conversation-launch.test.ts:490-506`). However, protection is opt-in per tool — nothing forces a future tool author to add a ledger.

5. **Can retries create duplicate side effects?**
   Designed paths prevent it (see Q2/Q4), with three residual risks stated or observable in code: (a) the ledger itself accepts duplicate risk when localStorage is full (`src/services/child-conversation-launch.ts:222-225`); (b) the ledger's read-modify-write is not atomic, so two tabs processing the same replayed ActionEvent concurrently could both claim; (c) `withRetry` will blindly repeat a failed POST-class call (e.g., `deleteCloudSecret`, `saveCloudSecret` — safe today because they are delete/upsert) if a future non-idempotent endpoint is wrapped without thought (`src/api/secrets-service.ts:133`, `src/api/cloud/secrets-service.api.ts:89-100`).

## Architectural Decisions

- **Idempotency declared per tool, enforced where the browser executes.** The MCP `ToolAnnotations` vocabulary (`readOnlyHint`/`destructiveHint`/`idempotentHint`/`openWorldHint`, `src/api/canvas-ui-client-tool.ts:13-19`) is the contract between harness and model; execution-time enforcement (ledger) lives beside the handler rather than in a generic dispatcher (`src/services/child-conversation-launch.ts:505-510`).
- **Dedup at two layers: transport and effect.** The event store dedups payloads by id (`src/stores/use-event-store.ts:99-102`), and the WS context separately gates *side effects* on the same set, because rendering dedup alone does not protect toasts/telemetry/client tools (`src/contexts/conversation-websocket-context.tsx:556-558`).
- **REST-first history with `since` replay** minimizes what a reconnect can even replay, shrinking the duplicate surface instead of only filtering it (`src/contexts/conversation-websocket-context.tsx:281`, `:966-973`).
- **Mutations default to fire-once; retries are opt-in per read query.** Rather than a global retry policy, each hook declares its own tolerance, tuned to whether failure blocks a gate (`src/hooks/query/use-conversation-history.ts:86-96`).
- **Failures to the agent travel as data, not exceptions.** Client-tool handlers convert every failure into structured guidance posted back into the conversation (`src/services/child-conversation-launch.ts:499-527`), making retry decisions legible to the model.

## Notable Patterns

- **Claim-before-execute ledger** (`claimToolCall`): a durable "already handled" set consulted before any await point, covering both completed and mid-flight replays — the classic exactly-once intent marker, in miniature (`src/services/child-conversation-launch.ts:205-227`).
- **Graceful-degradation retry with disclosure**: worktree→shared fallback changes the retried operation's semantics and must report the downgrade via `isolation_note` (`src/services/child-conversation-launch.ts:308-323`).
- **De-synchronized backoff**: 30 % jitter added specifically because main + planning sockets share fate; documented at the retry site (`src/hooks/use-websocket.ts:125-132`).
- **Classification-gated probe retry** with an explicit rationale for why the retry sits inside the query function (to keep outcome recording once-per-logical-probe) (`src/hooks/query/use-backends-health.ts:143-186`).
- **Idempotent-delete normalization**: mapping 404 to success so callers can retry deletes safely (`src/api/secrets-service.ts:139-154`).
- **Anti-retry fallbacks**: returning empty pages instead of looping when the backend lacks filter support (`src/api/event-service/event-service.api.ts:149-163`).

## Tradeoffs

- **Simplicity of `withRetry` vs correctness cost**: zero classification means 401/404/400 failures burn full backoff cycles before surfacing; contrast the hand-built classification used for probes (`src/api/with-retry.ts:12-21` vs `src/hooks/query/use-backends-health.ts:164-171`).
- **localStorage ledger vs durability**: survives reloads (the actual replay vector) but is per-browser-profile, non-atomic across tabs, and degrades to "accept replay risk" when storage fails (`src/services/child-conversation-launch.ts:209-225`).
- **Per-hook retry freedom vs consistency**: ~40 files each choose their policy; correct outcomes depend on authors remembering to cap retries near gates (e.g. `retry: 1` rationale in `src/hooks/query/use-conversation-history.ts:86-90`) — powerful, fragile.
- **Silent background retries vs observability**: hiding transient blips keeps UX calm, but neither logs nor telemetry record that a save needed 3 attempts; only final failure reaches console/toast (`src/api/secrets-service.ts:37-39`, `src/api/with-retry.ts:12-21`).
- **Model-visible hints vs enforcement**: `idempotentHint` guides the LLM but nothing validates that a tool marked idempotent actually is; the annotation is honest documentation, not a guarantee.

## Failure Modes / Edge Cases

- **Multi-tab double launch**: two tabs receiving the same replayed `launch_child_conversation` ActionEvent can interleave `getItem`/`setItem` in `claimToolCall` and both proceed (`src/services/child-conversation-launch.ts:205-227`).
- **Storage-quota replay window**: with `setItem` failing, the code explicitly proceeds unprotected (`:222-225`).
- **Corrupt-ledger reset**: malformed JSON starts a fresh ledger, forgetting prior claims after a crash mid-write (`:214-216`).
- **Retry storms against dead auth**: any `withRetry`-wrapped call hitting a 401 retries three times before failing, delaying the 401-triggered auth invalidation in `handle401Error` (`src/api/with-retry.ts:12-21`; `src/query-client-config.ts:10-14`).
- **Duplicate-toast suppression window**: the 3-second `shownErrors` window can suppress a genuinely new identical error occurring within 3 s of another (`src/query-client-config.ts:54-61`).
- **Unbounded WS retries**: `maxAttempts` defaults to Infinity (`src/hooks/use-websocket.ts:113-114`); consumers must remember to bound it.
- **Echo-based optimistic-message clearing**: FIFO match by text can mis-clear if two identical pending messages exist (`src/contexts/conversation-websocket-context.tsx:610-624`).

## Future Considerations

- Consolidate the two `withRetry` copies into one module (`src/api/with-retry.ts`, `src/api/settings-service/settings-service.api.ts:134-156`) and add optional error classification (never retry 4xx except 409/429) plus debug logging of attempt counts.
- Extract the launch-ledger into a reusable `claimToolCall(conversationId, toolCallId)` utility (or move it behind the client-tool dispatcher) so every future network-performing client tool inherits replay protection instead of copying the pattern.
- Replace the localStorage read-modify-write with an atomic primitive (e.g., `Web Locks API` or a single-key-per-toolcall write) to close the multi-tab race.
- Surface retry activity: emit a telemetry/console breadcrumb per `withRetry` attempt so flaky-backend incidents are diagnosable post hoc.
- Document a central retry-policy guideline (which statuses retry, caps, jitter) now that per-hook values have diverged across ~40 query hooks.
- Consider honoring the server-provided `classification.retryable` flag (`@openhands/typescript-client` type, consumed at `src/utils/error-handler.ts:2`) in client-side retry decisions rather than treating it as display-only metadata.

## Questions / Gaps

- **No evidence found** for HTTP-level idempotency keys: searched `Idempotency-Key`, `idempotencyKey`, `X-Request-ID` across `src/` — zero hits. Deduplication relies entirely on event/tool-call ids supplied by the agent-server; the frontend cannot itself dedup two distinct calls with identical arguments (e.g., an agent legitimately calling the launch tool twice with the same brief starts two conversations — mitigated only by prompt guidance, `src/api/launch-child-conversation-client-tool.ts:38`).
- **No evidence found** for dedicated unit tests of `withRetry` (searched `__tests__/` for `withRetry|with-retry` — zero hits); its behavior is only exercised transitively through service tests, if at all.
- The exact semantics of `ErrorClassification.retryable` are defined in the external `@openhands/typescript-client` package, which is outside this source directory (source-isolation rule); only its consumption sites are cited here (`src/utils/error-handler.ts:2`, `src/stores/error-message-store.ts:2`).
- Whether the agent-server itself applies retry/idempotency semantics beneath `client_tools` acknowledgement could not be verified from this repository; the frontend comments assert only that acknowledgement precedes browser execution (`src/services/child-conversation-launch.ts:451-458`).

---

Generated by `07.03-idempotency-and-retry-semantics` against `openhands`.
