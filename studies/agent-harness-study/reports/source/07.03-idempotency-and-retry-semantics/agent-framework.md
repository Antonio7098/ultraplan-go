# Source Analysis: agent-framework

## Dimension 07.03: Idempotency and Retry Semantics

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python (primary implementation), C#/.NET (parallel implementation), Go (pointer stub only) |
| Analyzed | 2026-08-26 |

## Summary

Microsoft Agent Framework treats retry safety as a **layered, side-effect-aware concern** rather than a single generic retry wrapper. There is no framework-wide "retry all failures" policy; instead the codebase distinguishes three regimes:

1. **Model-inference retries** are deliberately not built into core. The official pattern is user-supplied middleware or a client decorator using `tenacity` (`python/samples/02-agents/auto_retry.py:87-95`, `:116-142`), retrying only typed exceptions (`RateLimitError`). On .NET, retries are delegated to the Azure Core pipeline via `ClientPipelineOptions.RetryPolicy` (`dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:624-632`).
2. **MCP tool-call retries** use explicit error classification. Only connection-loss errors (`ClosedResourceError`, "session terminated" `McpError`) are retryable, with exactly one reconnect-and-retry for plain `tools/call` (`python/packages/core/agent_framework/_mcp.py:2108-2160`). For SEP-2663 long-running tasks, a "submit-vs-track" rule forbids re-issuing the augmented `tools/call` before a `task_id` exists — because the server may have accepted it — while poll/fetch requests reconnect-and-retry against the same id (`_mcp.py:2243-2256`, `:2509-2546`).
3. **Approval-gated side effects** (AG-UI package) get a full server-owned idempotency model: an `ApprovalLifecycle` state machine with an explicit `idempotency_key`, a `ClaimRecoveryPolicy.SAFE_TO_RETRY` proof requirement, an `INDETERMINATE` status that blocks automatic retry when execution may have started, and duplicate-decision deduplication that replays retained outcomes instead of re-executing (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:780-847`, `:495-541`).

Below the tool layer, tool exceptions become error function results returned to the model (`python/packages/core/agent_framework/_tools.py:1640-1641`, `:1412-1434`), so *the model* is the primary retry driver for ordinary tools, bounded by a consecutive-error budget (`_tools.py:96`, `:2718-2733`). The function-calling loop spec declares this area high-risk precisely because "small changes can produce duplicate side effects" and mandates exactly-once regression tests (`docs/specs/004-python-function-calling-loop.md:28-45`, `:607`, `:627`).

The headline question — *can a payment/email/delete tool be retried safely?* — is answered structurally: yes, but only through the approval lifecycle's idempotency-key path or after provable non-start (`SAFE_TO_RETRY`); without that proof, an interrupted execution becomes `INDETERMINATE` and "automatic retry is unsafe" (`_approval_lifecycle.py:500-503`, `:809-826`).

## Rating

**Score: 8 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Retryable vs non-retryable failures are classified explicitly in code, not by convention (`_mcp.py:2125-2134`, `:2391-2407`).
- An explicit idempotency-key interface exists for potentially-started side effects, with tests pinning both the safe-retry path and the unsafe default (`packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:818-851`, `test_mcp.py:7148-7177`).
- Operational safeguards include best-effort remote task cancellation on abandonment (`_mcp.py:2303-2308`, `:2573-2582`), retention windows and per-scope capacity (`_approval_lifecycle.py:242-245`, `:357-359`), and a consecutive-error budget for model-driven retries (`_tools.py:96`).
- Not 9–10 because: lifecycle state is process-local/in-memory only (`_approval_lifecycle.py:237`, `_snapshots.py:121-126`), there is no generic idempotency plumbing for arbitrary local tools outside the approval flow, model-call retries are entirely sample-level guidance, and the .NET implementation lacks any counterpart to the approval-lifecycle machinery.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Model-call retry wrapper (sample) | `AsyncRetrying(stop_after_attempt(3), wait_exponential, retry_if_exception_type(RateLimitError))` class decorator and chat-middleware variants | python/samples/02-agents/auto_retry.py:87-95, :116-142, :150-173 |
| .NET retry delegation | `clientOptions.RetryPolicy` copied onto `aiProjectClientOptions`; no framework-level tool retry | dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:624-632 |
| MCP retryable classification | Only `ClosedResourceError` or McpError containing "session terminated" count as connection loss; everything else raises immediately | python/packages/core/agent_framework/_mcp.py:2125-2134 |
| MCP one-reconnect-retry (plain tools/call) | `for attempt in range(2)` with `connect(reset=True)` between attempts; second failure raises "connection lost" | python/packages/core/agent_framework/_mcp.py:2108-2160 |
| Submit-vs-track rule (no retry before task_id) | Comment: retrying could start the operation twice; raises "connection lost; task state unknown" | python/packages/core/agent_framework/_mcp.py:2243-2256 |
| Track-phase bounded retry helper | `_send_with_one_reconnect`: one reconnect-retry against same `task_id`; second loss → `_MCPTaskAbandoned` | python/packages/core/agent_framework/_mcp.py:2509-2546; `_MCPTaskAbandoned` at :335-339 |
| Abandonment → remote cancel | Deadline expiry, abandonment paths spawn best-effort `tasks/cancel` | python/packages/core/agent_framework/_mcp.py:2291-2296, :2303-2308, :2573-2582 |
| Unparseable success → no fallback/retry | Raises ToolExecutionException "cannot safely retry (server may have started the operation)" | python/packages/core/agent_framework/_mcp.py:2372-2381 |
| Transient 408 poll retry | `transient_codes = {REQUEST_TIMEOUT}` retried inside poll loop; hard McpError → abandoned | python/packages/core/agent_framework/_mcp.py:2391-2407 |
| Terminal failures never cancel | failed/cancelled/input_required/completed+isError propagate as plain failure; server already done | python/packages/core/agent_framework/_mcp.py:2309-2311, :2469-2476 |
| Proactive ping-based reconnect | `_ensure_connected` pings and reconnects before calls, avoiding reactive ClosedResourceError | python/packages/core/agent_framework/_mcp.py:1984-2021 |
| Reconnect ownership | `_reconnect_without_loading` marshals reset onto lifecycle-owner task | python/packages/core/agent_framework/_mcp.py:1292-1297 |
| Approval state machine | Statuses PENDING→CLAIMED→EXECUTING→SETTLED/REJECTED/CANCELLED/EXPIRED/INDETERMINATE; terminal predicate | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:93-119 |
| Idempotency key field + validation | `idempotency_key` on occurrence/intent; empty key rejected; alias registration requires key match | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:186, :203, :320-321, :339-349 |
| Unsafe-retry boundary | `begin_execution` docstring: once executing, "arbitrary execution cannot be assumed safe to retry without idempotency proof" | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:780-795 |
| Claim recovery proof requirement | `release_claim` accepts only `ClaimRecoveryPolicy.SAFE_TO_RETRY` and only from CLAIMED (never EXECUTING) | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:131-134, :797-806 |
| Indeterminate = non-retryable | `claim_batch` raises ApprovalIndeterminateError "automatic retry is unsafe"; `mark_indeterminate` records possible start | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:500-503, :808-826 |
| Idempotency-key recovery | `recover_execution` re-grants authority only if intent key matches occurrence key; otherwise indeterminate | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:828-847 |
| Duplicate decision dedupe | Terminal occurrences return retained outcomes (`authorized_executions == ()`), emit "duplicate" audit event | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:495-541 |
| Retention windows & capacity | pending 86,400s; indeterminate 604,800s; terminal 900s; `max_entries=10_000` per scope; injectable monotonic clock | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:239-265, :357-359, :704-740 |
| Executor wrapper enforcing transitions | `LocalPendingToolTransitionOwner.execute`: begin → settle/defer → `recover_execution` on exception/cancel | python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:947-962 |
| Runner releases unstarted claims | Unavailable local tools → `release_claim(SAFE_TO_RETRY)` + `APPROVAL_TOOL_UNAVAILABLE` RunErrorEvent telling client to retry later | python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:2551-2559 |
| Runner replays retained outcomes | `batch.retained_outcomes` projected as results instead of re-execution | python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:1289-1295 |
| Runner recovery on stream failure | `finally` block: forwarded executions recovered (idempotency key) or marked indeterminate | python/packages/ag-ui/agent_framework_ag_ui/_agent_run.py:2762-2769 |
| Session approval dedupe (core) | Duplicate pending request ids raise; responses bind consume-once; unmatched/duplicate responses removed | python/packages/core/agent_framework/_tools.py:2121-2122, :2152-2153, :2182-2213, :2220-2246 |
| Tool error → model-visible result | Exceptions converted to `Content.from_function_result("Error: Function failed.")`; `include_detailed_errors` opt-in detail | python/packages/core/agent_framework/_tools.py:1412-1434, :1640-1641 |
| Consecutive-error budget | `DEFAULT_MAX_CONSECUTIVE_ERRORS_PER_REQUEST = 3`; limit stops further function calls per request | python/packages/core/agent_framework/_tools.py:96, :1407-1408, :2718-2733 |
| Middleware retry durability contract | Documented retry pattern (catch, re-invoke `call_next()`); denied first-attempt output never becomes durable | python/packages/core/tests/core/test_agent_hooks.py:1914-1931, :1936-1965, :1970-1995 |
| Test: no retry before task_id | `send_calls == 1`, reconnect not awaited, expects "task state unknown" | python/packages/core/tests/core/test_mcp.py:7148-7177 |
| Test: reconnect-retry during fetch/poll | tasks/result disconnect then success against same task_id | python/packages/core/tests/core/test_mcp.py:7180-7214 |
| Test: idempotency key permits retry | Local + hosted owners; interruption → CLAIMED → second execute settles; invocation_count == 2 | python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:818-851, :854-886 |
| Test: identical accepted retry replays outcome | `retry.authorized_executions == ()`, `retained_outcomes == (first_outcome,)` | python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:1160-1188 |
| Test: rejection cannot be upgraded by retry | Identical rejection retry returns retained outcome; accepted-after-rejection fails as conflict | python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:1193-1237 |
| Test: cancellation retry idempotent | Retrying an explicit cancellation keeps terminal cancellation | python/packages/ag-ui/tests/ag_ui/test_approval_lifecycle.py:1269+ |
| Spec: duplicate-side-effect risk charter | Loop changes require exactly-once review; issue #6851 cited | docs/specs/004-python-function-calling-loop.md:28-45, :607, :627 |
| Spec: retry-related scenario matrix | Rows for replayed wrappers, duplicate ids, cancellation retry, indeterminate windows, unavailable-tool retryability | docs/specs/004-python-function-calling-loop.md:462-488, :510-514 |
| Consent-gated human retry | Foundry consent errors surface `oauth_consent_request`; client retries after granting | python/packages/foundry_hosting/agent_framework_foundry_hosting/_responses.py:509-537; test at packages/foundry_hosting/tests/test_responses.py:4006-4017 |
| Snapshot store durability boundary | `InMemoryAGUIThreadSnapshotStore` is "process-local and not durable production storage" | python/packages/ag-ui/agent_framework_ag_ui/_snapshots.py:121-126 |

## Answers to Dimension Questions

### 1. Which tool failures are retried?

Only transport-classified, provably-safe failures get automatic retry:

- **Plain MCP `tools/call`**: `ClosedResourceError` or an `McpError` whose message contains "session terminated" triggers exactly one full reconnect and re-issue (`_mcp.py:2125-2146`). A tool-level `isError` result raises `ToolExecutionException` and is never retried (`_mcp.py:2111-2124`); any other `McpError` also propagates immediately (`_mcp.py:2130-2134`).
- **Long-running task tracking**: once a `task_id` exists, `tasks/get` and `tasks/result` each get one reconnect-and-retry against the same id (`_mcp.py:2509-2546`); a slow `tasks/get` surfacing as HTTP 408 is retried within the poll loop (`_mcp.py:2391-2405`).
- **Model inference calls**: nothing automatic in core. The documented approaches are tenacity-based chat middleware/decorators limited to `RateLimitError` (`python/samples/02-agents/auto_retry.py:57`) and .NET Azure Core `RetryPolicy` passthrough (`dotnet/src/Microsoft.Agents.AI.Foundry/FoundryChatClient.cs:630-632`).
- Explicitly non-retryable: pre-`task_id` connection loss (`_mcp.py:2243-2256`), unparseable success-shaped responses (`_mcp.py:2372-2381`), `MiddlewareFailure`/`UserInputRequiredException` which bypass the error-result conversion (`_tools.py:1635-1639`), and `INDETERMINATE` approvals (`_approval_lifecycle.py:500-503`).

### 2. Are repeated attempts safe?

Safety is conditional, never assumed. Three proofs gate a repeated attempt at a side effect: (a) the operation is read-only/idempotent by protocol role (polling `tasks/get`, fetching `tasks/result`); (b) execution provably did not begin — authority is released from `CLAIMED` (pre-execution) status only, via `ClaimRecoveryPolicy.SAFE_TO_RETRY` (`_approval_lifecycle.py:797-806`); or (c) the caller supplied a matching `idempotency_key`, which `recover_execution` checks before re-granting authority (`_approval_lifecycle.py:843-845`). Failing all three, the occurrence is marked `INDETERMINATE` and further automatic attempts raise rather than re-execute. The submit-vs-track rule applies the same logic at the transport layer: the augmented create call is never blindly re-sent (`_mcp.py:2243-2246`).

### 3. Is retry state persisted?

Partially, and this is the weakest dimension. The AG-UI `ApprovalLifecycle` keeps occurrences, aliases, and locks in process-local dicts with time-based purge (pending 24 h, indeterminate 7 days, terminal 15 min) (`_approval_lifecycle.py:239-265`, `:704-740`). The default thread snapshot store is explicitly "process-local and not durable production storage" (`_snapshots.py:121-126`). In core, approval-request/response bindings persist through `AgentSession` state (which can be file-backed via `FileSessionStore`), so duplicate-id detection and consume-once binding survive within a session's storage (`_tools.py:2109-2138`). After a process restart, a replayed decision finds no registered interrupt and fails closed with `APPROVAL_RESUME_NOT_FOUND` rather than executing (`_agent_run.py:1279-1304`). There is no cross-process durable idempotency store (e.g., keyed ledger) anywhere in the repo.

### 4. Are non-idempotent tools protected?

Yes, through several independent mechanisms: harness write tools (`file_access_*`, skill scripts) register with `approval_mode="always_require"` by default so a human gates every dangerous call (core `AGENTS.md`, `packages/core/agent_framework/_harness/_file_access.py`); approval decisions are validated against the registered name/arguments so an edited retry cannot smuggle different arguments past retained authority (`_approval_lifecycle.py:524-530`); rejected calls keep their terminal result and can never be flipped to execution authority by a retry (`_approval_lifecycle.py:1193-1218` in tests; conflict raised at `:517`); and duplicate decisions return the retained outcome instead of executing again (`_approval_lifecycle.py:536-542`). At the loop level, the model cannot silently hammer a failing dangerous tool: three consecutive errors disable further function calls for the request (`_tools.py:96`, `:2727-2732`).

### 5. Can retries create duplicate side effects?

The design is built to prevent exactly this, and the spec names it as the primary hazard ("duplicate side effects ... #6851", `docs/specs/004-python-function-calling-loop.md:30`, `:627`). Residual exposure remains in specific seams:

- A sync tool running in a worker thread during a middleware-failure batch cancel **cannot be interrupted and may complete its side effects**, though its discarded result never reaches transcript/model/history (`docs/specs/004-python-function-calling-loop.md:337`, `:528`) — documented and tested, but a real at-least-once window.
- Local tools executed *outside* the approval flow have no framework-side dedupe; if a user wraps function middleware in a naive retry, duplicates are user-owned. The framework provides no generic idempotency-key argument for ordinary `@tool` functions.
- Process crash between `begin_execution` and settlement loses the in-memory occurrence; recovery relies on fail-closed resume (`APPROVAL_RESUME_NOT_FOUND`) plus the remote MCP cancel sweep, not on durable state.
- Within its own scope, the model is sound: MCP create calls are never re-issued blind (`test_mcp.py:7148-7177` pins `send_calls == 1`), abandoned tasks get best-effort `tasks/cancel` (`_mcp.py:2573-2582`), and approval retries either replay retained outcomes or require key-proofed recovery.

## Architectural Decisions

1. **No universal retry policy; classification at the transport seam.** Rather than wrapping all tool invocations in a retry decorator, the MCP layer enumerates retryable exception shapes inline (`_mcp.py:2125-2134`, `:2528-2530`). This makes the unsafe default (no retry) explicit and forces per-operation reasoning about side effects.
2. **Submit-vs-track split for long-running work** (`_mcp.py:2277-2278`): the request that may start a side effect is treated as unrepeatable until the server acknowledges with a `task_id`; only acknowledged, trackable phases are retryable. This mirrors payment-industry practice (authorize-then-poll) without requiring servers to implement idempotency keys.
3. **Authority state machine with an INDETERMINATE sink** (`_approval_lifecycle.py:93-119`): uncertainty ("may have executed") is modeled as a first-class terminal-ish status with its own longer retention window (7 days vs 15 min), rather than being collapsed into failure. This trades user convenience (manual intervention required) for exactly-once confidence.
4. **Idempotency keys are caller-declared, not generated** (`_approval_lifecycle.py:186`, `:320-321`): the framework verifies key match on recovery but does not synthesize keys for arbitrary tools, keeping the framework out of the business of knowing which operations are actually idempotent server-side.
5. **Errors as data to the model** (`_tools.py:1640-1641`): ordinary tool failures become `function_result` content so the LLM decides whether/how to adapt, with a hard consecutive-error budget (`_tools.py:96`) as the runaway guard. Automatic retry is reserved for layers where safety is provable.
6. **Spec-enforced change control** (`docs/specs/004-python-function-calling-loop.md:35-45`): the duplicate-side-effect hazard area requires scenario-matrix updates, extra review for "exactly-once execution", and full-package validation — institutionalizing retry semantics as a reviewed contract.

## Notable Patterns

- **Bounded attempt loops over backoff libraries**: core uses literal `for attempt in range(_MCP_RECONNECT_ATTEMPTS)` loops (`_mcp.py:2525`) with the constant documenting "initial try + one reconnect-and-retry" (`_mcp.py:329-332`); tenacity appears only at the sample level (`auto_retry.py:23-30`).
- **Marker-exception channeling**: `_MCPTaskAbandoned(ToolExecutionException)` distinguishes "remote may still be running → cancel" from terminal failure "server already done → don't cancel" (`_mcp.py:335-339`, `:2303-2311`).
- **Consume-once binding**: approval responses bind to session-recorded immutable requests and pop them, so a replayed response finds no pending request and is dropped with a warning (`_tools.py:2210-2212`, `:2236-2241`).
- **Retained-outcome replay**: duplicate terminal decisions return `ApprovalOutcome`s projected into the run as tool results (`_agent_run.py:1289-1291`), making client retries observationally identical to the original completion.
- **Audit events for lifecycle transitions**: every transition emits a structured log event including `"duplicate"` (`_approval_lifecycle.py:762-778`, `:541`), giving operators visibility into retry dedupe activity.
- **Fail-closed resume after restart**: unknown interrupts produce `APPROVAL_RESUME_NOT_FOUND` RunErrorEvents, never silent re-registration (`_agent_run.py:1296-1304`).
- **Human-in-the-loop retry for consent**: OAuth-consent failures surface a consent link event and expect the client to retry after granting (`_responses.py:509-537`; pinned by `test_responses.py:4006-4017`).

## Tradeoffs

- **Exactly-once bias over availability**: an interrupted approval execution becomes `INDETERMINATE` and requires operator/client action (or key-proofed recovery) instead of auto-retrying (`_approval_lifecycle.py:846-847`). Safe, but means transient blips can strand approved work.
- **In-memory lifecycle vs durability**: process-local dicts with retention purges are simple and fast, but a restart discards all authority and terminal history; the design compensates by failing closed, at the cost of losing replayable outcomes across restarts (`_approval_lifecycle.py:262-265`; `_snapshots.py:124-125`).
- **Retry knowledge concentrated in one package**: the sophisticated machinery lives in `ag-ui`; agents using plain `Agent.run()` with local functions get only the error-as-result + consecutive-error-budget regime. Cross-language, .NET has no equivalent of the lifecycle (only Azure pipeline retries, `FoundryChatClient.cs:624-632`).
- **Conservative fallback refusal**: refusing to fall back to plain `tools/call` on unparseable augmented-call responses prevents double execution but turns some legacy-server quirks into hard failures (`_mcp.py:2374-2381`).
- **Model-driven retry is nondeterministic**: since the model sees "Error: Function failed." and chooses next steps, retry counts for ordinary tools depend on model behavior; only the consecutive-error cap bounds it (`_tools.py:2718-2733`).

## Failure Modes / Edge Cases

- **Second disconnect during poll/fetch** → `_MCPTaskAbandoned` → best-effort remote cancel + normal tool failure surfaced to the loop (`_mcp.py:2541-2545`, `:2303-2308`).
- **Reconnect failure mid-recovery** → immediate abandonment with cancel (`_mcp.py:2535-2539`).
- **Deadline expiry** (`max_task_wait`) → cancel spawned, `ToolExecutionException` raised; inner stray timeouts deliberately distinguished via `_await_with_deadline` (`_mcp.py:2291-2296`, `:2549-2571`).
- **Local cancellation** → optional best-effort remote cancel controlled by `cancel_remote_task_on_local_cancellation` (`_mcp.py:2299-2302`).
- **Alias/argument drift on retry**: a resumed decision whose arguments differ from the registered occurrence is rejected (`_approval_lifecycle.py:529-530`); batch validation is atomic so one bad decision leaves all siblings claimable (`test_approval_lifecycle.py:889-892`).
- **Capacity exhaustion**: per-scope `max_entries` raises `ApprovalCapacityError` rather than evicting protected occurrences (`_approval_lifecycle.py:357-359`).
- **Corrupt session snapshots** are quarantined and surface an error mentioning retry, rather than silently re-running (`packages/core/tests/core/test_sessions.py:1089-1095`).
- **Sync sibling side effects during batch cancel**: unavoidable completion with discarded result — the documented at-least-once edge (`docs/specs/004-python-function-calling-loop.md:337`).

## Future Considerations

- Provide a durable/pluggable `ApprovalLifecycle` store (the retention-window and clock injection design already anticipates it: `_approval_lifecycle.py:246`) so approval authority survives restarts without widening the fail-open surface.
- Generalize the `idempotency_key` concept beyond approval-gated tools (e.g., an opt-in parameter on `@tool`/function middleware) so ordinary non-idempotent local tools can declare retry safety.
- Port the submit-vs-track and lifecycle patterns to the .NET implementation, which currently relies solely on SDK-level retries.
- Surface retry/dedupe telemetry beyond logs (e.g., OTel events mirroring the `approval_event` log extras) for production observability of duplicate-decision rates.
- Consider surfacing remaining-attempt/error-budget info in tool error results so the model can make better-informed retry decisions than the fixed "Error: Function failed." string (`_tools.py:1426-1428`).

## Questions / Gaps

- **No evidence found** for any cross-process or persistent idempotency-key store: searches for `idempotency` across `python/packages` returned hits only in the ag-ui lifecycle and its tests; no database/ledger-backed implementation exists in-tree.
- **No evidence found** for automatic retry of model-inference calls inside `packages/core` (searches for `tenacity`, `AsyncRetrying`, `retry_if_exception_type` in package sources return no matches; only the sample uses them). Whether this is intentional minimalism or a gap is not stated in-repo beyond the sample's framing as "production agents need retry logic".
- The `.NET` sources contain no counterpart to `ApprovalLifecycle`; whether parity is planned could not be determined from this source tree (no roadmap docs found under `dotnet/`).
- The Go implementation is a pointer stub only (`go/README.md` links to `microsoft/agent-framework-go`), so Go retry semantics are out of scope for this source.
- The interplay between `ToolApprovalMiddleware` standing rules and the AG-UI lifecycle's idempotency keys (whether a standing auto-approval can bypass occurrence registration) was not fully traced; the runner registers occurrences at emit time (`_agent_run.py:2734-2739`), but an exhaustive proof of no bypass path was not completed within this study's boundary.

---

Generated by `dimensions/07.03-idempotency-and-retry-semantics` against `agent-framework`.
