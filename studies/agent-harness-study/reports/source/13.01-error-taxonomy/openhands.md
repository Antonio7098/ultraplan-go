# Source Analysis: openhands

## 13.01: Error Taxonomy

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 (React Router v7), Vite, HeroUI v2, Zustand, React Query, PostHog |
| Analyzed | 2026-08-22 |

## Summary

OpenHands is a React/TypeScript frontend (agent-canvas) that consumes the
`@openhands/typescript-client` and an agent-server backend. The frontend
**does not own a single unified error taxonomy**; instead it composes
several layered taxonomies, each tied to a specific subsystem
(backend-availability, MCP, sandbox runtimes, conversation events, ACP
agents, telemetry). Errors arriving over the wire from the agent-server
already carry a structured shape (`code` + `classification`) that is
re-exported from the SDK and used for routing. A small but disciplined
set of frontend-local enums and string-tagged sentinel strings cover
the cases the SDK does not classify. The taxonomy is consistent enough
to drive distinct UI handling (retry, reauth, missing-server screen,
api-key screen, cloud logged-out state, MCP credential banner,
sandbox-gone banner) and to drive telemetry routing (`error_kind` →
`diagnostic` vs `outcome`).

## Rating

**7 / 10** — Clear model with explicit source-side kinds, dedicated
test coverage, and operational safeguards (`error_outcome` telemetry,
reserved metadata keys, structured `error_code`/`classification`,
backend-health disable-after-N-failures, CORS/timeout detection). Not
9–10 because (a) there is no single canonical error type — the model
is distributed across at least seven overlapping taxonomies, and (b)
most of the canonical "kind" values are produced by the backend SDK
and the frontend cannot easily invent new ones without a server
release; the frontend only invents new codes on the ACP side.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Server-provided classification (typed) | `ErrorClassification` imported from `@openhands/typescript-client`; payload includes `kind`, `retryable`, `user_action`, `error_id` | `src/utils/error-handler.ts:2`, `__tests__/utils/error-handler.test.ts:61-79` |
| Conversation event error types | `ConversationErrorEvent` re-export + `ServerErrorEvent` (code+detail) declared locally | `src/types/agent-server/core/events/conversation-state-event.ts:3,5,154-174` |
| Displayability type guard | `isDisplayableErrorEvent` collapses `ConversationErrorEvent \| ServerErrorEvent` for banner routing | `src/types/agent-server/type-guards.ts:242-260` |
| Banner error icon switch on kind | `kind !== "internal" && kind !== "unknown"` → warning icon, else error icon | `src/components/features/chat/error-message-banner.tsx:103-120` |
| Telemetry kind → diagnostic/outcome split | `kind === "internal" \|\| kind === "unknown" ? "diagnostic" : "outcome"` | `src/utils/error-handler.ts:44-46` |
| Reserved telemetry keys prevent spoofing | `RESERVED_ERROR_KEYS` strips caller-supplied `error_source`, `error_kind`, `error_id`, `error_telemetry` | `src/utils/error-handler.ts:10-15,24-26` |
| MCP test failure taxonomy | `ExtendedMCPTestFailureKind = MCPTestFailureKind \| "credentials"`; kinds observed in code: `timeout`, `connection`, `credentials`, `unknown` | `src/types/mcp-server.ts:34,60`; `src/api/mcp-service/mcp-service.api.ts:117,170,263,307` |
| MCP test error message routing | switch over `errorKind` (timeout, connection, credentials, default→unknown) | `src/utils/mcp-test-error-message.ts:15-24` |
| MCP health state machine | `McpServerHealth` discriminated union with `kind: ExtendedMCPTestFailureKind` and verification level `verified \| connectivity-only` | `src/types/mcp-health.ts:10-25` |
| Sandbox-issue taxonomy | `SandboxIssue = "missing" \| "paused" \| "starting" \| "errored" \| "unreachable"` from sandbox status + fetch classification | `src/hooks/query/use-bash-command-logs.ts:16-21,39-57,68-99` |
| Sandbox-issue → UI keys | `SANDBOX_ISSUE_I18N: Record<SandboxIssue, I18nKey>` mapping | `src/components/features/automations/detail/run-logs-modal.tsx:23-` |
| Backend-availability error class hierarchy | `AgentServerUnavailableError` ⊃ `AgentServerUnsupportedVersionError`, `AgentServerUnknownVersionError`, with stable `code` constants | `src/api/agent-server-compatibility.ts:20-23,41-103` |
| Backend-availability typed guards | `isAgentServerUnavailableError`, `isAgentServerUnsupportedVersionError`, `isAgentServerUnknownVersionError`, `isAgentServerAuthError`, `isSdkHttpStatusError` | `src/api/agent-server-compatibility.ts:61-71,105-121,132-133,157-174` |
| Backend-unavailable routing in App | `isAgentServerAuthError` → `ApiKeyEntryScreen`; `isAgentServerUnavailableError` → `MissingAgentServerScreen`; cloud-logged-out → `MissingAgentServerScreen` | `src/root.tsx:354-372` |
| Connection-error message normalization | `isCorsOrNetworkErrorMessage`, `isBackendRequestTimeoutMessage`, `getUserFacingConnectionErrorMessage` walk `cause` chain up to 4 deep | `src/utils/user-facing-error.ts:7-92` |
| Toast translation of error class | `displayErrorToast` rewrites CORS/network → `ERROR$CORS_OR_NETWORK`, timeout → `ERROR$BACKEND_REQUEST_TIMEOUT` | `src/utils/custom-toast-handlers.tsx:96-108` |
| User-facing transient vs sticky | `ErrorMessageType = "connection" \| "conversation"`; connection auto-clears, conversation is sticky | `src/stores/error-message-store.ts:9,33,62-65` |
| Server-connection-error constant | `SERVER_CONNECTION_ERROR_MESSAGE` triggers WS `reconnect()` in chat retry button | `src/constants/server-connection-error.ts:1`; `src/components/features/chat/chat-interface.tsx:589-592` |
| Backend health probe + sentinel labels | `INVALID_BACKEND_API_KEY_ERROR`, `MISSING_BACKEND_API_KEY_ERROR`, `CLOUD_BACKEND_API_KEY_OR_NETWORK_ERROR`, `CLOUD_BACKEND_LOGGED_OUT_ERROR` | `src/hooks/query/use-backends-health.ts:36-40` |
| Health-probe retry / non-retry distinction | `isRetryableProbeError` skips retry on `INVALID/MISSING/CLOUD_LOGGED_OUT`, retries everything else | `src/hooks/query/use-backends-health.ts:164-171` |
| Backend-status label dispatch | ordered string-tag checks: missing key, invalid key, logged out, cloud access, tunnel timeout, URL/network | `src/components/features/backends/backend-status-label.ts:32-76` |
| Backend health entry persisted | `BackendHealthEntry { consecutiveFailures, lastError, lastFailureAt, disabled }` with tamper-resistant validators | `src/api/backend-registry/health-storage.ts:10-39` |
| Backend-health disable cap | `MAX_CONSECUTIVE_FAILURES = 5` flips `disabled: true`, persisted in localStorage | `src/api/backend-registry/health-storage.ts:10`; `src/api/backend-registry/health-store.ts:46-63` |
| No-backend availability error | `NoBackendAvailableError` thrown when no host/apiKey/conversationUrl + no local backend | `src/api/agent-server-client-options.ts:22-36,52-58` |
| ACP code taxonomy | `ACP_ERROR_HEADER_KEYS` maps `ACPAuthRequired`, `ACPSpawnError`, `ACPInitError`, `ACPPromptError`, `UsagePolicyRefusal` to i18n headers | `src/utils/acp-error-codes.ts:8-18` |
| ACP auth-code test | `isAcpAuthErrorCode(code)` returns true only for `ACPAuthRequired`, enabling the reauth button | `src/utils/acp-error-codes.ts:27-29`; `src/components/features/chat/chat-interface.tsx:594-598` |
| Conversation WS classification extraction | `"classification" in errorEvent` narrows `ConversationErrorEvent` vs `ServerErrorEvent` to feed `trackError` and store | `src/contexts/conversation-websocket-context.tsx:572-591` |
| AgentErrorEvent inline rendering (NOT banner) | separate trackError branch; `ErrorEventMessage` renders inline | `src/contexts/conversation-websocket-context.tsx:598-608`; `src/components/conversation-events/chat/event-message-components/error-event-message.tsx:1-16` |
| HTTP body extraction | `getApiErrorBody` accepts Axios + SDK `HttpError` shapes; `getApiErrorMessage` prefers server `message`/`detail` | `src/utils/api-error-message.ts:11-38` |
| Custom typed Error class (validation) | `AutomationFileValidationError extends Error` carries `issues: string[]` | `src/utils/automation-export.ts:13-21` |
| File-validation errors | `errorMessage` returned as `{ errorMessage, ... }` shape | `src/utils/file-validation.ts:6-49` |
| Test coverage of taxonomy | dedicated tests for error-handler, user-facing-error, error-message-store, error-message-banner, MCP service/parse, backend health | `__tests__/utils/error-handler.test.ts`; `src/utils/user-facing-error.test.ts`; `__tests__/stores/error-message-store.test.ts`; `__tests__/components/chat/error-message-banner.test.tsx`; `__tests__/api/mcp-service/mcp-service.api.test.ts`; `__tests__/api/mcp-health/probe-mcp-server-health.test.ts`; `__tests__/hooks/query/use-backends-health.test.tsx` |
| Documentation of error taxonomy | none — `docs/` has `architecture.md`, `ACP_AGENTS.md`, `DEVELOPMENT.md`, `TESTING_MATRIX.md`, but no error-handling guide; `AGENTS.md` only documents 401 auth behaviour | `docs/` (no error doc); `AGENTS.md:549-553` |

## Answers to Dimension Questions

### 1. Are errors classified by source?

**Yes, in multiple overlapping ways.** The closest single "kind" comes
from the backend in `ErrorClassification.kind` (typed union, fields
`internal`, `auth`, `unknown`, plus more from the SDK), which is the
canonical source-side classifier (`src/utils/error-handler.ts:2`,
`__tests__/utils/error-handler.test.ts:65-79`). It is enriched by
frontend-only taxonomies that the SDK does not cover:

- **ACP-specific codes** (`ACPAuthRequired`, `ACPSpawnError`,
  `ACPInitError`, `ACPPromptError`, `UsagePolicyRefusal`)
  (`src/utils/acp-error-codes.ts:8-18`)
- **MCP test failures** (`timeout`, `connection`, `credentials`,
  `unknown`) (`src/types/mcp-server.ts:34`, `src/utils/mcp-test-error-message.ts:15-24`)
- **MCP health state machine** (`status: unchecked/checking/healthy/failed`
  + verification `verified/connectivity-only`)
  (`src/types/mcp-health.ts:10-25`)
- **Backend health sentinels** (`Invalid API key`, `API key required`,
  `Cloud API key or network issue`, `Logged out`)
  (`src/hooks/query/use-backends-health.ts:36-40`)
- **Agent-server availability classes** (`AgentServerUnavailableError`
  ⊃ `AgentServerUnsupportedVersionError`,
  `AgentServerUnknownVersionError`) with stable codes
  (`src/api/agent-server-compatibility.ts:41-103`)
- **Sandbox issues** (`missing`, `paused`, `starting`, `errored`,
  `unreachable`) (`src/hooks/query/use-bash-command-logs.ts:16-21`)
- **Transient vs sticky** (`connection` vs `conversation`)
  (`src/stores/error-message-store.ts:9`)

Mapping to the dimension's reference categories:

- **model**: not surfaced as a kind; surfaces via `auth` classification
  (`error-handler.test.ts:128`) and via the LLM settings UI.
- **provider**: surfaces as `auth`, `credentials` (MCP), or
  `INVALID_BACKEND_API_KEY_ERROR` (cloud/local host).
- **tool**: surfaces as `ACPSpawnError`, `ACPInitError`, `ACPPromptError`.
- **validation**: surfaced via `AutomationFileValidationError`
  (`src/utils/automation-export.ts:13-21`) and `file-validation.ts`
  — a typed error, not a classification kind.
- **policy**: `UsagePolicyRefusal` (`src/utils/acp-error-codes.ts:17`).
- **context**: not explicitly tagged; the only proxy is the
  `conversation` vs `connection` split, which is about error *behavior*
  not cause.
- **user**: not explicitly tagged; only `MISSING_BACKEND_API_KEY_ERROR`
  is a user-actionable label.
- **infrastructure**: `internal` kind, `AgentServerUnavailableError`,
  `MISSING_BACKEND_API_KEY_ERROR`, sandbox `unreachable`/`errored`.
- **timeout**: explicit kind in MCP `ExtendedMCPTestFailureKind`,
  `BACKEND_REQUEST_TIMEOUT_MESSAGE` from `user-facing-error.ts`,
  `SandboxIssue` "timeout" handled via `TimeoutError`/`AbortError`
  in `classifyFetchError` (`use-bash-command-logs.ts:88-96`).

### 2. Is the taxonomy used for handling?

**Yes — heavily.** Examples:

- **Icon choice**: `kind !== "internal" && kind !== "unknown"` switches
  the banner from red error icon to amber warning icon
  (`src/components/features/chat/error-message-banner.tsx:103-120`).
- **Telemetry routing**: `error_kind ∈ {internal, unknown}` → diagnostic
  (full payload), others → outcome (lighter)
  (`src/utils/error-handler.ts:44-46`).
- **Retry button visibility**: only shown when `onRetry` is supplied;
  supplied for `SERVER_CONNECTION_ERROR_MESSAGE` →
  `conversationWebSocket?.reconnect()`
  (`src/components/features/chat/chat-interface.tsx:589-592`).
- **Reauth button visibility**: only shown when `onReauth` is supplied;
  supplied iff `isAcpAuthErrorCode(errorCode)` →
  navigate to `/settings/agents` (`chat-interface.tsx:594-598`).
- **Banner header text**: `getAcpErrorHeaderKey(code)` picks a
  code-specific header from `ACP_ERROR_HEADER_KEYS`
  (`src/utils/acp-error-codes.ts:10-18,21-24`).
- **Auto-clear vs sticky**: `clearConnectionError()` only clears
  `errorType === "connection"` (`src/stores/error-message-store.ts:62-65`).
- **Root routing**: `isAgentServerAuthError` → `ApiKeyEntryScreen`,
  `isAgentServerUnavailableError` (incl. cloud-logged-out /
  cloud-unreachable) → `MissingAgentServerScreen`
  (`src/root.tsx:354-372`).
- **Backend status label dispatch**: ordered cascade of
  `isMissingBackendApiKeyHealthError`, `isInvalidBackendApiKeyHealthError`,
  `isCloudBackendLoggedOutHealthError`, `isCloudBackendApiKeyOrNetworkHealthError`,
  `isBackendRequestTimeoutMessage`, `isCorsOrNetworkErrorMessage`
  (`src/components/features/backends/backend-status-label.ts:32-76`).
- **Toast rewrites**: `isCorsOrNetworkErrorMessage` and
  `isBackendRequestTimeoutMessage` rewrite error text to localized
  guidance before showing the toast
  (`src/utils/custom-toast-handlers.tsx:96-108`).
- **MCP probe interpretation**: `error_kind === "connection" | "unknown"`
  AND `AUTH_FAILURE_TEXT` regex → upgrade to `credentials`
  (`src/api/mcp-health/probe-mcp-server-health.ts:51-59`).
- **MCP test → message**: switch on kind returns the right i18n key
  (`src/utils/mcp-test-error-message.ts:15-24`).
- **Sandbox issue → empty state**: `SANDBOX_ISSUE_I18N` map
  (`src/components/features/automations/detail/run-logs-modal.tsx:23-`).

So yes — the `kind` (and the related sentinels) drives both UI
affordances and telemetry routing.

### 3. Are error categories documented?

**Partially.** Code-level TSDoc explains each classification
(`api/agent-server-compatibility.ts:123-133`,
`utils/user-facing-error.ts:3-92`, `acp-error-codes.ts:3-29`,
`error-message-store.ts:4-8`, `error-handler.ts:5-26`,
`hooks/query/use-backends-health.ts:74-90,136-171`,
`api/mcp-health/probe-mcp-server-health.ts:16-46`). The cross-cutting
"error taxonomy" is **not documented as a single reference** in
`docs/`; the directory contains only `architecture.md`, `ACP_AGENTS.md`,
`DEVELOPMENT.md`, `SELF_HOSTING.md`, `TESTING_MATRIX.md`,
`DefenseClaw.md`. `AGENTS.md` only mentions the auth-failure 401
detection at `AGENTS.md:549-553`. New contributors must read
multiple files to discover the model.

### 4. Can new error types be added without breaking existing handling?

**Mostly yes, with caveats by surface.**

- **ErrorClassification.kind** comes from the backend SDK; adding a new
  value requires extending the union there (a typescript-client
  release). The frontend defaults to `"unknown"`
  (`src/utils/error-handler.ts:27`) and falls back to diagnostic
  telemetry (`kind === "internal" || kind === "unknown"`).
- **ACP errorCode** is `string | null` — adding a new code is harmless:
  unknown codes render the generic banner via the i18n key fallback
  (`src/utils/acp-error-codes.ts:22`).
- **MCP `error_kind`** (`ExtendedMCPTestFailureKind = MCPTestFailureKind | "credentials"`)
  is a closed union; new kinds require a union update + a switch case
  in `makeMcpTestErrorMessage` (`src/utils/mcp-test-error-message.ts:15`).
  Existing `default:` returns the `MCP$TEST_ERROR_UNKNOWN` message,
  so the UI won't crash, but the message will be wrong without a
  matching switch arm.
- **SandboxIssue** is a closed union in
  `src/hooks/query/use-bash-command-logs.ts:16`; adding a value
  requires extending the union and updating
  `SANDBOX_ISSUE_I18N` in `run-logs-modal.tsx`.
- **BackendHealth sentinels** (`INVALID_BACKEND_API_KEY_ERROR`,
  `MISSING_BACKEND_API_KEY_ERROR`, `CLOUD_BACKEND_API_KEY_OR_NETWORK_ERROR`,
  `CLOUD_BACKEND_LOGGED_OUT_ERROR`) are export-string constants; new
  ones only need a new constant + a new `is*HealthError` predicate +
  adding it to the cascade in `backend-status-label.ts`.
- **ErrorMessageType** is a closed union (`"connection" \| "conversation"`)
  with a default (`"conversation"`).

The `RESERVED_ERROR_KEYS` set (`src/utils/error-handler.ts:10-15`)
prevents caller-provided metadata from spoofing `error_source`,
`error_kind`, `error_id`, and `error_telemetry` — a clear extension
safety. Adding new reserved keys requires editing the set.

## Architectural Decisions

1. **Server is the source of truth for `ErrorClassification.kind`**.
   The frontend does not invent kind values; it only consumes what the
   SDK provides. (`src/utils/error-handler.ts:2`,
   `src/contexts/conversation-websocket-context.tsx:576-577`)
2. **ACP error codes are localized on the frontend**, not in the SDK.
   The `ACP_ERROR_HEADER_KEYS` table maps `ACPAuthRequired` etc. to
   `I18nKey` so the agent-server doesn't need to know about UI
   categories. (`src/utils/acp-error-codes.ts:10-18`)
3. **MCP has its own taxonomy, separate from server errors**:
   `ExtendedMCPTestFailureKind = MCPTestFailureKind | "credentials"`,
   produced by the local probe rather than the agent-server's event
   stream. (`src/types/mcp-server.ts:34`,
   `src/api/mcp-health/probe-mcp-server-health.ts:46-72`)
4. **Telemetry distinguishes diagnostic vs outcome** by `kind`:
   `internal`/`unknown` → full payload (diagnostic); others →
   outcome. This makes PII-safe dashboards possible.
   (`src/utils/error-handler.ts:44-46`)
5. **Two-tier transient handling**: `ErrorMessageType` separates
   connection (auto-cleared) from conversation (sticky) so banner
   behaviour matches retry semantics. (`src/stores/error-message-store.ts:9`)
6. **Connection-error text normalisation is centralised**: every error
   that reaches a toast or backend-health label goes through
   `getUserFacingConnectionErrorMessage` / `isCorsOrNetworkErrorMessage`
   / `isBackendRequestTimeoutMessage`, walking the cause chain up to
   4 levels. (`src/utils/user-facing-error.ts:13-92`)
7. **Classified `error_id` is the canonical log-correlation key**;
   a caller-supplied `metadata.eventId` is promoted to `error_id`
   only as a fallback so unknown errors remain correlatable.
   (`src/utils/error-handler.ts:30-43`)
8. **Backend health is a separate, persistent subsystem** with its own
   retry policy (`MAX_CONSECUTIVE_FAILURES = 5`, disable-after-cap)
   and tamper-resistant validators on `localStorage` entries.
   (`src/api/backend-registry/health-storage.ts:10-39`,
   `src/api/backend-registry/health-store.ts:46-63`)
9. **HTTP-error shape is dual**: the frontend recognises both axios
   `AxiosError` and the SDK's `HttpError`, plus `TypeError` from
   `fetch`. (`src/utils/api-error-message.ts:11-17`,
   `src/api/agent-server-compatibility.ts:157-174`,
   `src/hooks/query/use-bash-command-logs.ts:68-99`)
10. **Conversation errors vs agent errors are split**: banner vs
    inline. `isDisplayableErrorEvent` (`ConversationErrorEvent` /
    `ServerErrorEvent`) → banner; `isAgentErrorEvent` →
    inline chat message; both tracked separately.
    (`src/types/agent-server/type-guards.ts:242-260`,
    `src/contexts/conversation-websocket-context.tsx:572-608`,
    `src/components/conversation-events/chat/event-message-components/error-event-message.tsx:1-16`)

## Notable Patterns

- **Two-channel error model**: every error that reaches the user has
  both a *message* (what to show) and a *kind/code* (how to react).
  The pair is stored together in `useErrorMessageStore`
  (`src/stores/error-message-store.ts:11-58`).
- **Reserved telemetry keys**: spoof-resistant `RESERVED_ERROR_KEYS`
  prevents caller-provided metadata from overriding the
  analytics-grade classification (`src/utils/error-handler.ts:10-15`).
- **Status code as a kind**: backend HTTP status drives root-level
  routing (`isAgentServerAuthError` for 401,
  `isSdkHttpStatusError(error, 404)` short-circuits missing secrets
  and missing git diffs) — kind and status are independent axes.
  (`src/api/agent-server-compatibility.ts:132-133`,
  `src/api/secrets-service.ts:144`,
  `src/api/git-service/agent-server-git-service.api.ts:106`)
- **Conservative inference**: `probe-mcp-server-health.ts:22-23` keeps
  a regex for HTTP auth failure text and *upgrades* a
  `connection`/`unknown` kind to `credentials` when the text matches
  — a deliberately conservative fallback for hosted servers that
  classify auth failures as connection errors.
- **Persistence-shaped retries**: backend health writes through to
  `localStorage` on every state change so the disable cap survives a
  page reload (`src/api/backend-registry/health-store.ts:19-23,
  46-63`).
- **Cause-chain walker**: `collectErrorMessages` follows
  `Error.cause` and `record.message` for up to 4 hops before giving
  up; every wrapper layer that needs to recognise CORS or timeout
  benefits automatically (`src/utils/user-facing-error.ts:13-41`).

## Tradeoffs

- **Distributed taxonomy**: there is no single `Error` type — at
  least seven taxonomies exist (ErrorClassification, ACP codes,
  ExtendedMCPTestFailureKind, SandboxIssue, BackendHealth sentinels,
  AgentServerUnavailable hierarchy, ErrorMessageType). A new
  contributor needs to learn the right one for each subsystem.
- **Most "kinds" are server-owned**: the frontend cannot add new
  `kind` values to `ErrorClassification` without a backend release.
- **MCP `error_kind` is closed-union**: forgetting to add a switch
  arm silently produces the wrong-but-non-crashing
  `MCP$TEST_ERROR_UNKNOWN` message.
- **Backend health sentinels are stringly-typed constants**: a typo
  in `INVALID_BACKEND_API_KEY_ERROR` cannot be caught by the type
  system; only `isInvalidBackendApiKeyHealthError` (which uses `===`)
  is the check (`src/hooks/query/use-backends-health.ts:42-46`).
- **Cause-walk depth is hard-capped at 4**: legitimate deep cause
  chains will not be classified
  (`src/utils/user-facing-error.ts:7`).
- **Reserved-keys list is hard-coded**: callers can't add their own
  reserved dimension without editing the file
  (`src/utils/error-handler.ts:10-15`).
- **Tests are per-subsystem, not cross-cutting**: there is no single
  end-to-end test that proves the whole pipeline (server event →
  classification extraction → banner → telemetry) handles new
  kinds correctly.

## Failure Modes / Edge Cases

- **Empty `kind` on a classified event**: `errorEvent.classification`
  may be `null` even when `code` is set (e.g., older SDK). The WS
  handler narrows with `"classification" in errorEvent` and falls
  back to `null`, which `trackError` treats as `kind: "unknown"` and
  `error-message-banner.tsx` renders with the red error icon
  (`src/contexts/conversation-websocket-context.tsx:576-577`,
  `src/utils/error-handler.ts:27`,
  `src/components/features/chat/error-message-banner.tsx:103-120`).
- **`ServerErrorEvent` doesn't carry `classification`**: the WS
  handler explicitly checks `"classification" in errorEvent` because
  the field is only on `ConversationErrorEvent`
  (`src/contexts/conversation-websocket-context.tsx:576-577`).
- **MCP timeout via OAuth**: the OAuth path uses a longer timeout
  (`OAUTH_MCP_TEST_TIMEOUT_SECONDS = 120`,
  `src/api/mcp-service/mcp-service.api.ts:21,44-47`), so a
  timeout error from OAuth is more likely a real hang than a real
  network failure. The frontend still tags it as `error_kind:
  "timeout"` (`mcp-service.api.ts:304-308`).
- **401 swallowed by getSettings probe (non-public)**: the
  `/server_info`-then-`getSettings()` pattern in
  `loadAgentServerInfo` deliberately swallows non-401 errors from
  the second probe so the app loads with an unvalidated key
  (`src/api/agent-server-compatibility.ts:373-394`). Subsequent 401s
  from React Query hooks will *not* trigger the auth screen.
- **localStorage tampering on backend health**:
  `isValidEntry` clamps `consecutiveFailures` to `[0, 5]` and rejects
  malformed entries, but a hand-edited entry that passes validation
  can still disable a backend (`src/api/backend-registry/health-storage.ts:24-39`).
- **Connection probe race**: the probe has its own
  `PROBE_RETRY_ATTEMPTS = 2` with 300ms backoff
  (`use-backends-health.ts:140-186`) — a transient first-probe miss
  is recovered inside one tick, but a definitive auth error is *not*
  retried (`isRetryableProbeError`, line 164-171). Auth errors are
  surfaced immediately to drive the recovery UI.
- **Cloud vs local CORS detection is asymmetric**: cloud paths use
  `isCorsOrNetworkError` (`use-backends-health.ts:109-111`), local
  paths use `isSdkHttpStatusError(error, 401)` for auth only; a
  local CORS failure is not classified and falls through to the
  generic "Disconnected" label (`backend-status-label.ts:68-70`).

## Future Considerations

- **Unify the taxonomies**: a single `ErrorKind` type that subsumes
  ACP codes, MCP `error_kind`, `SandboxIssue`, and backend-health
  sentinels would let one banner / one toast handle them all. Today
  the user-facing surface (`ErrorMessageBanner`) accepts a
  classification object but ignores most kind strings.
- **Document the error model in `docs/`**: the model is documented
  in code but not in a single place. A `docs/errors.md` (or section
  in `architecture.md`) listing the kind values, the cause-chain
  walking behaviour, and the telemetry routing would reduce onboarding
  friction.
- **Make `SandboxIssue` and `ExtendedMCPTestFailureKind` exhaustive
  in switch statements**: add a `never`-style exhaustiveness check
  so missing arms fail tests rather than silently defaulting.
- **Promote backend-health sentinels to a discriminated union**:
  replacing the string constants with `{ kind: "invalid_api_key", ... } | { kind: "missing_api_key", ... } | ...` would catch
  typos at compile time and make the `is*HealthError` predicates
  trivially derivable.
- **Server-side test matrix for `ErrorClassification.kind`**:
  ensure the SDK publishes every kind the frontend expects to see,
  including any new ones.
- **Make cause-walk depth configurable / context-aware**:
  the 4-hop limit is a magic number that the rest of the model
  silently depends on (`src/utils/user-facing-error.ts:7`).

## Questions / Gaps

- **Is the `ErrorClassification.kind` union fully enumerated
  anywhere in this repo?** Only `internal`, `auth`, and `unknown`
  appear in this repo's tests/code; the rest must come from
  `@openhands/typescript-client`, whose sources are not present in
  this checkout. The dimension's full taxonomy (`model`, `tool`,
  `validation`, `policy`, `context`, `user`, `infrastructure`) is not
  obviously matched — confirm via the SDK's own type defs.
- **What happens when an MCP probe returns an `error_kind` not in
  `ExtendedMCPTestFailureKind`?** The TS compiler would reject the
  payload at the cast site, but `interpretMcpTestResponse` would
  fall through with `response.error_kind` as the literal value —
  which is then re-used as `McpServerHealth.kind`. No exhaustiveness
  check exists. (`src/api/mcp-health/probe-mcp-server-health.ts:51-59`)
- **Is there a `MCP$TEST_ERROR_CONNECTION` key that could be reused
  for stdio MCPs that fail with `ENOENT`?** `AGENTS.md:691` notes
  the misleading error message ("Could not reach the server. Check
  the URL and server type.") — the `error_kind` is still
  `"connection"` even though no URL is involved. A `stdio_not_found`
  kind (or a richer `error_kind` enum) would fix this.
- **What about ACP agents that surface non-ACPAuthRequired auth
  failures?** `ACP_ERROR_HEADER_KEYS` covers
  `ACPSpawnError`/`ACPInitError`/`ACPPromptError`/`UsagePolicyRefusal`,
  but only `ACPAuthRequired` triggers a reauth button
  (`src/utils/acp-error-codes.ts:27-29`). A user with a different
  ACP agent's auth code would see a banner without a recovery
  action.
- **The `error_outcome` PostHog event is well-structured, but is
  the dimension property `error_source` actually used as a stable
  filter in dashboards?** Free-form strings (`"conversation"`,
  `"agent"`, `"planning_conversation"`, `"planning_agent"`,
  `"unknown"`) suggest not — consider constraining to a typed union.
- **No evidence of cross-source (model vs validation vs policy)
  test coverage**: tests cover each taxonomy in isolation but not
  the unified handling. (Search boundary: `__tests__/` and
  `tests/`.)

---

Generated by `13.01-error-taxonomy` against `openhands`.
