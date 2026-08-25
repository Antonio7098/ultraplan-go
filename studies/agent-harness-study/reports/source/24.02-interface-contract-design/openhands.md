# Source Analysis: openhands

## Interface Contract Design

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19 / Vite / Node 22 / React Router 7 / TanStack Query / HeroUI / Monaco / PostHog |
| Analyzed | 2026-08-23 |

## Summary

The OpenHands source is `@openhands/agent-canvas`, a React/TypeScript single-page app that talks to a sibling Python **agent-server** and an optional **Cloud** backend. There is no in-repo domain logic to model — it is purely a consumer. Interface contracts are therefore not in the "abstract base class" sense; they live as discriminated-union type families (`Action`, `Observation`, `OpenHandsEvent`, `Backend`, `BackendKind`) plus a centralized options/transport helper (`getAgentServerClientOptions`), a **CI-enforced API-access rule** that bans raw `axios`/`fetch` for agent-server calls, and an explicit admission pipeline for externally-authored catalog/interface manifests.

The contracts are narrow and consumer-owned, but the model is single-implementation in practice: every adapter ultimately wraps the SDK class generated from the Python server's OpenAPI. The strongest guarantees are (a) compile-time discriminated unions for events/actions/observations, (b) a version-compatibility floor enforced via `assertAgentServerVersionIsSupported`, (c) trust-boundary validation of extension manifests, and (d) explicit error classes for every recoverable vs. blocking failure (`AgentServerUnavailableError`, `AgentServerUnsupportedVersionError`, `AgentServerUnknownVersionError`, `NoBackendAvailableError`). The weakest areas are substitution: there is effectively one implementation per contract (the SDK client), so the "can two independent implementations satisfy it?" question is partially moot — the contract is enforced by being re-generated from the same OpenAPI. Cloud-vs-local dualism is the one place where two implementations genuinely co-exist (`callCloudProxy` vs typed SDK), and it is handled through a uniform envelope plus per-call backend-kind branching.

## Rating

**7/10** — Clear model with tests, explicit interfaces, and operational safeguards. The consumer-side type families and the CI guard are mature, the trust-boundary validators for extension manifests are unusually thorough, and the version-compatibility gate is real (throws, not warns). What keeps this from 8+ is that almost every "interface" has exactly one realization (the SDK client), so substitutability is largely hypothetical, and a few contracts encode shape but not semantics (notably `SettingsUpdateRequest`, `WorkspaceMode`, `misc_settings`).

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Central event/action/observation unions | `Action`, `Observation`, `OpenHandsEvent` discriminated unions, exported from `src/types/agent-server/core/index.ts:1-10`; re-aggregated as a single union at `src/types/agent-server/core/openhands-event.ts:25-46` | `src/types/agent-server/core/index.ts:1-10`, `src/types/agent-server/core/openhands-event.ts:25-46` |
| Generic event base | `BaseEvent { id, timestamp, source }` with literal-typed `SourceType = "agent" \| "user" \| "environment" \| "hook"` | `src/types/agent-server/core/base/event.ts:10-25`, `src/types/agent-server/core/base/common.ts:56` |
| `ActionBase<T>` / `ObservationBase<T>` discriminated by `kind` literal | Generic generic-keyed interface base | `src/types/agent-server/core/base/base.ts:55-63` |
| Closed enum-like literal union `EventType`/`ActionOnlyType`/`ObservationOnlyType` | `src/types/agent-server/core/base/base.ts:4-53` |
| `SecurityRisk` and `ExecutionStatus` closed-string enums | `src/types/agent-server/core/base/common.ts:59-75` |
| Substitutability annotation (deprecated vs new) | `StrReplaceEditorObservation` kept for backward compat in addition to `FileEditorObservation` | `src/types/agent-server/core/base/observation.ts:148-184` |
| Discriminated by literal `kind` tag, narrowed via `ConversationStateUpdateEvent` with `key: "full_state" \| "execution_status" \| "stats" \| "goal"` | `src/types/agent-server/core/events/conversation-state-event.ts:99-151` |
| Narrowed agent-error, user-reject, message-event subtypes | `src/types/agent-server/core/events/observation-event.ts:42-52`, `src/types/agent-server/core/events/action-event.ts:11-72`, `src/types/agent-server/core/events/message-event.ts:5-25` |
| Type guards with documented shape checks | `isBaseEvent`, `isObservationEvent`, `isActionEvent`, `isMessageEvent`, `isAgentServerEvent` | `src/types/agent-server/type-guards.ts:45-75`, `src/types/agent-server/type-guards.ts:67-122`, `src/types/agent-server/type-guards.ts:299-301` |
| SDK-narrowed guards carrying tool-name checks instead of action-kind | `isCanvasUIActionEvent`, `isLaunchChildConversationActionEvent` | `src/types/agent-server/type-guards.ts:183-200` |
| `Backend` interface, single source of truth | `Backend { id, name, host, apiKey, kind, authMode?, connectionRevision? }`, `BackendSelection`, `ResolvedActiveBackend` | `src/api/backend-registry/types.ts:4-23` |
| `BackendKind = "local" \| "cloud"` literal union | `src/api/backend-registry/types.ts:1` |
| Sealed sentinel for the empty-registry state | `NO_BACKEND` constant + `isNoBackend()` predicate | `src/api/backend-registry/active-store.ts:22-39` |
| Central client-options helper with a typed error for missing backend | `AgentServerClientOptions`, `AgentServerClientOverrides`, `NoBackendAvailableError` | `src/api/agent-server-client-options.ts:6-80` |
| CI-enforced rule against raw `axios`/`fetch` to `/api/*` | `no-direct-agent-server-calls.test.ts` scans `src/` for `openHands.`, `createHttpClient`, `@openhands/typescript-client/client/http-client`, `new HttpClient`, `axios`, and `fetch('/api/...`); allow-list is just `automation-service.api.ts`, `cloud/proxy.ts`, `main-app-auth.ts` | `src/api/no-direct-agent-server-calls.test.ts:1-79` |
| Typed clients used in services | `ConversationClient`, `SettingsClient`, `FileClient`, `VSCodeClient`, `BashClient`, `MCPClient`, `ProfilesClient`, `PluginsClient`, `WorkspacesClient`, `SkillsClient`, `LLMMetadataClient`, `ServerClient` from `@openhands/typescript-client/clients` | e.g. `src/api/event-service/event-service.api.ts:1`, `src/api/skills-service.ts:1`, `src/api/bash-service/bash-service.api.ts:1`, `src/api/workspaces-service/workspaces-service.api.ts:11-14` |
| Uniform `callCloudProxy` envelope for cross-origin cloud calls | `CloudProxyRequest` interface; `callCloudProxy<T>(req)` builds a `CloudClient` and forwards; supports `bearer` / `session-api-key` / `none` auth modes | `src/api/cloud/proxy.ts:5-39` |
| Cloud-client factory with runtime-host override | `createCloudClient`, `createCloudClientForRuntime`, `activeOrgForBackend` | `src/api/cloud/client.ts:10-63` |
| Per-call backend branching pattern | `if (active.kind === "cloud") { return callCloudProxy(...); } return new ConversationClient(getAgentServerClientOptions({ conversationUrl, sessionApiKey })).method(...)` | `src/api/event-service/event-service.api.ts:46-69`, `src/api/bash-service/bash-service.api.ts:82-118` |
| Error hierarchy for compatibility failures | `AgentServerUnavailableError` base; `AgentServerUnsupportedVersionError`, `AgentServerUnknownVersionError` extending it; carries `code`, `actualVersion`, `requiredVersion` | `src/api/agent-server-compatibility.ts:41-103` |
| `MINIMUM_COMPATIBLE_AGENT_SERVER_VERSION` floor + semver parser | `compareAgentServerVersions`, `parseAgentServerVersion` (handles `major.minor.patch[-prerelease][+build]`) | `src/api/agent-server-compatibility.ts:18-19`, `src/api/agent-server-compatibility.ts:243-291` |
| Auth-required detection honoring build-time and runtime flags | `isAgentServerAuthError` requires `isAuthRequired()` AND status 401 | `src/api/agent-server-compatibility.ts:132-133` |
| Runtime `usable_tools` capability advertisement from server | `isAgentServerToolAvailable` reads `usable_tools`, defaults allow when missing | `src/api/agent-server-compatibility.ts:149-155` |
| `Settings` schema with discriminated `agent_kind`, both local and cloud flags (`_is_set` vs `_set`) | `Settings.llm_api_key_set`, `Settings.llm_api_key_is_set` both surfaced | `src/types/settings.ts:112-153` |
| `SettingsSchema`/`SettingsSectionSchema`/`SettingsFieldSchema` types mirror `/api/settings/{agent,conversation}-schema` | `src/types/settings.ts:46-71` |
| Settings diff contract (agent_settings_diff, conversation_settings_diff, misc_settings_diff) with `[key: string]: unknown` for SDK forward compat | `SettingsUpdateRequest` | `src/api/settings-service/settings-service.api.ts:77-85` |
| Container of frontend-owned settings, deep-merge semantics spelled out | `MiscSettings.app_preferences`, deep-merge for primitives, list-replacement for `disabled_skills` | `src/api/settings-service/settings-service.api.ts:25-49` |
| `AppPreferenceField` literal-set with `isAppPreferenceField` type guard | `src/api/settings-service/settings-service.api.ts:25-92` |
| `ClientToolSpec` contract with `annotations` (MCP-flavored) | name/description/parameters/annotations with `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint` | `src/api/canvas-ui-client-tool.ts:9-20` |
| Hard-coded JSON-Schema for `canvas_ui_control` client tool | `parameters: { type: "object", additionalProperties: false, properties: {...}, required: ["command"] }`, `enum` for `command` and `tab` | `src/api/canvas-ui-client-tool.ts:62-91` |
| Hard-coded JSON-Schema for `launch_child_conversation` client tool | `parameters` with `enum` over `CHILD_CONVERSATION_TARGETS` and `CHILD_CONVERSATION_ISOLATIONS` | `src/api/launch-child-conversation-client-tool.ts:67-102` |
| Discriminator constants kept in one module to avoid leaking SDK-generated names | `CANVAS_UI_CLIENT_TOOL_NAME`, `CANVAS_UI_CLIENT_ACTION_KIND`, `LEGACY_CANVAS_UI_TOOL_NAME`, `LAUNCH_CHILD_CONVERSATION_*` | `src/constants/canvas-ui.ts:1-10`, `src/constants/child-conversation.ts` |
| `ActionEvent` interface is the canonical shape carrying thought, action, tool_call, llm_response_id, security_risk, critic_result | `src/types/agent-server/core/events/action-event.ts:11-72` |
| `ObservationEvent<T extends Observation>` generic with action_id correlation | `src/types/agent-server/core/events/observation-event.ts:9-39` |
| Streaming delta event contract (delta content + reasoning) | `StreamingDeltaEvent` literal-kind discriminator | `src/types/agent-server/core/events/streaming-delta-event.ts:3-8` |
| ACP-sub-agent tool call event | `ACPToolCallEvent` (imported by the core index re-export) | `src/types/agent-server/core/events/acp-tool-call-event.ts` |
| Critic metadata contract with closed `CriticFeature.name` semantics | `CriticResult.score`, `CriticMetadata` with `[key: string]: unknown` escape hatch | `src/types/agent-server/core/base/critic.ts:4-50` |
| Conversation metadata local extension (not round-tripped) | `ConversationMetadata` interface with `selected_workspace`, `active_profile`, `plugins` | `src/api/conversation-metadata-store.ts:8-40` |
| MCP server config discriminated by `MCPServerType` literal | `MCPServerType = "sse" \| "stdio" \| "shttp"`, `MCPServerConfig.enabled?` with explicit "absent means enabled" | `src/types/mcp-server.ts:11-29` |
| MCP test response contract with `ExtendedMCPTestFailureKind` extension | `MCPTestFailureKind` (SDK) + `"credentials"` extension | `src/types/mcp-server.ts:34-65` |
| `ModelsResponse`/`WebClientConfig` typed config payloads | `src/api/option-service/option.types.ts:9-33` |
| `AppConversation`/`AppConversationPage`/`AppConversationStartRequest`/`AppConversationStartTask`/`AppConversationStartTaskStatus` | `src/api/conversation-service/agent-server-conversation-service.types.ts:65-122` |
| `SandboxStatus` cloud-only lifecycle enum | `"PAUSED" \| "RUNNING" \| "STARTING" \| "MISSING" \| "ERROR"` with doc note "null for local" | `src/api/conversation-service/agent-server-conversation-service.types.ts:7-15` |
| `RuntimeServicesInfo` typed shape for `/server_info.runtime_services`, parsed at runtime | `parseRuntimeServicesInfo` narrows by `typeof` checks, tolerates stringified JSON | `src/api/agent-server-adapter.ts:128-173` |
| `DirectConversationInfo` typed mapping input, mapped to `AppConversation` | `toAppConversation`, `toConversationPage` adapter functions | `src/api/agent-server-adapter.ts:48-111`, `src/api/agent-server-adapter.ts:317-412` |
| `LookupSecret` discriminated record for conversation-start secret material | `{ kind: "LookupSecret", url, headers?, description? }` | `src/api/agent-server-adapter.ts:995-1000` |
| `StartConversationPayload` with mutually-exclusive `agent_settings` vs `agent_profile_id` (enrichment boundary) | `src/api/agent-server-adapter.ts:1002-1023`, `src/api/agent-server-adapter.ts:1085-1112` |
| `withRetry` exponential-backoff helper | 3 retries default, `baseDelayMs * 2 ** attempt` | `src/api/with-retry.ts:4-26` |
| `BackendHealthEntry` interface with explicit invariants and a `isValidEntry` type guard against localStorage tampering | `consecutiveFailures ∈ [0, MAX_CONSECUTIVE_FAILURES=5]`, `disabled: boolean` | `src/api/backend-registry/health-storage.ts:10-39` |
| `isValidBackend` rejects malformed localStorage with shape checks (`typeof id === "string" && id.length > 0`, etc.) | `src/api/backend-registry/storage.ts:24-40` |
| Cloud locked-host normalization rules for `app.all-hands.dev` → `app.openhands.dev` rewrite | `canonicalizeCloudHostname` | `src/api/agent-server-config.ts:55-70` |
| Header contract for client identification (coarse, non-PII) | `X-OpenHands-Client`, `X-OpenHands-Client-Version`, `X-OpenHands-Telemetry-Distinct-Id` | `src/api/client-source.ts:12-20` |
| Trust-boundary manifest validators | `validateInterfaceManifest`, `validateSetupManifest` | `src/manifests/interface-validation.ts`, `src/manifests/manifest-validation.ts` |
| Manifest validator enforces: version, markup-free copy, restricted docs URL, regex-pinned endpoints, `{id}` substitution shape, attribute names matching `HOST_ATTRIBUTES`, allowed enum values | `src/manifests/interface-validation.ts:28-100` |
| Manifest attribute types/requiredness must match what the host's edit dialog implements | `HOST_ATTRIBUTES: Record<AutomationAttributeName, { type, required }>` | `src/manifests/interface-validation.ts:49-58` |
| `LOOKUP_TABLES` (e.g., `OVERVIEW_METRICS`, `DASHBOARD_FILTER_IDS`, `INTERFACE_ICON_SLUGS`) for host-known identifiers | `src/manifests/types.ts` |
| All-or-nothing manifest rejection policy with structured error list | `SetupValidationResult.valid`, `errors[]` | `src/manifests/manifest-validation.ts:67-70`, `src/manifests/interface-validation.ts:83-86` |
| Template-version regex enforces full semver incl. pre-release and build | `TEMPLATE_VERSION_PATTERN` | `src/manifests/manifest-validation.ts:37-38` |
| Cross-source enforcement of typed-client usage in test | `src/api/no-direct-agent-server-calls.test.ts:1-79` |
| Compatibility version test (parse/compare/display) | `src/api/agent-server-compatibility.test.ts:14-57` |
| Compatibility probe test (registers backends, mocks `ServerClient.getServerInfo`, asserts throw type per scenario) | `src/__tests__/api/agent-server-compatibility-bundled-pin.test.ts:69-141` |
| Backend registry storage round-trip and seed policies | `src/__tests__/api/backend-registry/storage.test.ts:34-80` |
| Manifest validation suite (well-formed admission + per-invariance rejection) | `src/__tests__/manifests/interface-validation.test.ts:38-100` |

## Answers to Dimension Questions

1. **Are interfaces small, coherent, and owned by the consumer side?**

   Mostly yes. The frontend never owns Python; it owns the wire-types it consumes. Each contract is small and single-purpose: `Backend` (5 required fields + an optional revision token), `ActionBase<T>` / `ObservationBase<T>` (one required `kind` field), `BaseEvent` (id, timestamp, source), `MCPServerConfig` (transport-tag + discriminated fields), `ClientToolSpec` (name + description + JSON-Schema + MCP annotations). Where shape would otherwise bloat — e.g., the agent-server SDK dumps a single `agent_settings` blob — the frontend narrows the contract by computing `buildConfiguredOpenHandsAgentSettings` / `buildConfiguredAcpAgentSettings` (`src/api/agent-server-adapter.ts:817-947`) and explicitly removing reserved/legacy keys (e.g., `delete agentSettings.acp_env` at `src/api/agent-server-adapter.ts:935`). Discriminated ownership is also visible in the "tool name vs action kind" split: legacy `canvas_ui` and SDK-generated `canvas_ui_control` are two tool names that share one action-kind via `ActionBase<"CanvasUIAction" | typeof CANVAS_UI_CLIENT_ACTION_KIND>` (`src/types/agent-server/core/base/action.ts:317-323`), so the consumer can switch without leaking the SDK-generated discriminator.

2. **Do contracts specify behavior, not just method signatures?**

   Partially. The contracts are mostly **structural** — `AgentServerClientOptions { host, apiKey?, workingDir, timeout? }` carries no behavior. Where behavior is encoded, it lives as **runtime guards and validators**, not in the type system:
   - `assertAgentServerVersionIsSupported` enforces a minimum compatible version and throws dedicated error types with `code`/`actualVersion`/`requiredVersion` (`src/api/agent-server-compatibility.ts:293-318`).
   - `isValidBackend` rejects malformed localStorage (`src/api/backend-registry/storage.ts:24-40`).
   - `isValidEntry` rejects negative or out-of-range failure counts that would defeat the disable mechanism (`src/api/backend-registry/health-storage.ts:24-39`).
   - `parseRuntimeServicesInfo` narrows untyped server JSON before downstream uses it (`src/api/agent-server-adapter.ts:153-173`).
   - `validateInterfaceManifest` rejects externally-authored manifests that don't match the host's own dialog semantics (`src/manifests/interface-validation.ts`).
   - `isAgentServerAuthError` requires both `isAuthRequired()` and HTTP 401 to flag a stale key, so the same status code is harmless in non-public mode (`src/api/agent-server-compatibility.ts:132-133`).

   Behavioral encoding that is **missing**: `SettingsUpdateRequest` is purely structural (`{ agent_settings_diff?, conversation_settings_diff?, misc_settings_diff?, [key: string]: unknown }` — `src/api/settings-service/settings-service.api.ts:77-85`); the consumer-side narrative that "lists are replaced wholesale" lives in a doc comment. `WorkspaceMode = "local_repo" | "new_worktree"` (`src/api/conversation-metadata-store.ts:6`) is a literal union with no helper that maps a user click into a payload. The `ConversationMetadata.plugins` field claims "Store plugin coordinates only; parameters may contain secrets" (`src/api/conversation-metadata-store.ts:38`) and the consumer is expected to call `toPluginCoordinates(plugin)` (`src/api/conversation-metadata-store.ts:42-46`) before persisting, but nothing prevents a caller from forgetting.

3. **Can providers, tools, stores, and runtimes be replaced safely?**

   Tools, stores, and runtimes: yes, with caveats. Tools: the SDK ships a `WorkspacesClient`, `ProfilesClient`, `SkillsClient`, `MCPClient`, `PluginsClient`, `BashClient`, etc., and the consumer wraps each through a single `client()` factory in `src/api/workspaces-service/workspaces-service.api.ts:25-27`. The cloud-or-local branch is uniform: `if (isCloudBackend()) return fetchCloudProfiles(); return new ProfilesClient(getAgentServerClientOptions()).listProfiles();` (`src/api/profiles-service/profiles-service.api.ts:91-94`). Stores: `useConversationStore` / `useFilesTabStore` are swappable via Zustand getters (`src/services/canvas-ui.ts:19-26`); the canvas-ui service treats the stores as opaque state holders.

   **Providers and runtimes**: only partially. The `Backend` interface is structurally clean, but `getEffectiveLocalBackend()` only returns the active backend and short-circuits if it's cloud (`src/api/backend-registry/active-store.ts:140-144`), so a caller cannot borrow an alternate local backend. The `no-direct-agent-server-calls.test.ts` CI guard forces every caller through the SDK client classes — there is effectively **one implementation per contract** (the SDK class generated from the agent-server OpenAPI), and the only second implementation is the cloud proxy. Two independent implementations of the same Python endpoint contract therefore do not exist in this repo; the contract is enforced by re-generation, not by cross-implementation agreement. The runtime substitution path is more about **transport** (local SDK vs cloud-proxy hop) than about two independently written agents.

4. **Are compatibility failures caught early by tests or validation?**

   Yes — this is a strong point. The compatibility floor is real: a too-old agent-server throws `AgentServerUnsupportedVersionError`, an unknown version throws `AgentServerUnknownVersionError`, and an unreachable server throws `AgentServerUnavailableError` with `noBackendConfigured` flag distinguished (`src/api/agent-server-compatibility.ts:293-318`, `src/api/agent-server-compatibility.ts:41-59`). The probe is wired through `root.tsx` and the entry path; tests cover every branch — minimum-version success, too-old rejection, missing-version rejection, cache-by-host, cloud-active skip, empty-registry rejection (`src/__tests__/api/agent-server-compatibility-bundled-pin.test.ts:69-141`). `agent-server-compatibility.test.ts` covers version parsing edge cases including "unknown", empty, malformed, prerelease, and `sdk_version` fallback (`src/api/agent-server-compatibility.test.ts:14-57`).

   The CI guard test (`src/api/no-direct-agent-server-calls.test.ts`) catches future drift toward raw HTTP at lint time. The manifest validators reject a malformed manifest entirely (`src/manifests/manifest-validation.ts:1-17`), and per-invariance rejection cases are exercised by `src/__tests__/manifests/interface-validation.test.ts:53-100+`. Where the contract is loose — e.g., cloud vs local `AppConversation` shape — the adapter (`toAppConversation` in `src/api/agent-server-adapter.ts:317-402`) normalizes and overlays stored repo metadata to paper over partial cloud hydration (`src/api/cloud/conversation-service.api.ts:30-47`).

## Architectural Decisions

- **Single sanctioned HTTP entry point: `@openhands/typescript-client`.** The CI guard test (`src/api/no-direct-agent-server-calls.test.ts:32-79`) makes any `openHands.`, `axios(`, `fetch('/api/...`, `new HttpClient(`, `createHttpClient(`, or `@openhands/typescript-client/client/http-client` import a test failure. Allow-list is exactly three files. This is the strongest contract enforcement in the repo.

- **Discriminated-union event family, sealed by literal `kind` tags.** Every event extends `BaseEvent` and adds a literal `kind` field (`ActionEvent`, `MessageEvent`, `ObservationEvent`, `UserRejectObservation`, `AgentErrorEvent`, `SystemPromptEvent`, `CondensationEvent`, `ConversationStateUpdateEvent`, `ConversationErrorEvent`, `HookExecutionEvent`, `PauseEvent`, `ServerErrorEvent`, `StreamingDeltaEvent`, `ACPToolCallEvent` — see `src/types/agent-server/core/openhands-event.ts:25-46`).

- **Centralized options assembly.** Every agent-server call goes through `getAgentServerClientOptions(overrides?)` (`src/api/agent-server-client-options.ts:52-69`), which throws `NoBackendAvailableError` when no local backend is registered and the call isn't otherwise overridden. Cloud-only callers go through `getAgentServerHttpClientOptions` or the typed `callCloudProxy`.

- **Errors are typed and discriminated.** `AgentServerUnavailableError` is the base; `AgentServerUnsupportedVersionError` and `AgentServerUnknownVersionError` extend it and carry `code`/`actualVersion`/`requiredVersion` (`src/api/agent-server-compatibility.ts:41-103`). Type-guard predicates (`isAgentServerUnavailableError`, `isAgentServerUnsupportedVersionError`, `isAgentServerUnknownVersionError`) identify them by both `instanceof` and a structural `name`/`code` check, so cross-realm errors are still narrowed (`src/api/agent-server-compatibility.ts:61-121`).

- **Manifests are a trust boundary.** Externally-authored catalog/interface manifests are *admitted* against host-owned allow-lists (`OVERVIEW_METRICS`, `DASHBOARD_FILTER_IDS`, `INTERFACE_ICON_SLUGS`, `INTERFACE_SUB_PAGE_IDS`, `INTERFACE_VERSION` in `src/manifests/types.ts`), regex-pinned endpoint shapes, and `HOST_ATTRIBUTES` for the edit dialog (`src/manifests/interface-validation.ts:49-58`). Admission is all-or-nothing; a single bad field rejects the manifest.

- **Tool-name vs action-kind are decoupled.** Legacy `canvas_ui` and SDK-generated `canvas_ui_control` share one action-kind discriminator (`CanvasUIAction` extends `ActionBase<"CanvasUIAction" | typeof CANVAS_UI_CLIENT_ACTION_KIND>` — `src/types/agent-server/core/base/action.ts:317-323`), and the type guards key on `tool_name` (`isCanvasUIActionEvent`, `isLaunchChildConversationActionEvent` — `src/types/agent-server/type-guards.ts:183-200`) so the SDK-generated name stays inside `constants/canvas-ui.ts` and `constants/child-conversation.ts`.

- **`misc_settings.app_preferences` container with deep-merge semantics.** Adding a new sibling category (e.g., `ui_preferences`) is non-breaking (`src/api/settings-service/settings-service.api.ts:40-49`); lists like `disabled_skills` are replaced wholesale on PATCH (`src/api/settings-service/settings-service.api.ts:69-76`).

- **`agent_profile_id` and `agent_settings` are mutually exclusive on conversation start.** The payload builder documents the enrichment boundary and the SDK-server behavior on each path (`src/api/agent-server-adapter.ts:1085-1112`).

- **`usable_tools` (renamed from `available_tools` in SDK PR #3028) is the runtime capability advertisement.** Browser-tool and `task_tool_set` gating reads this field; missing field defaults to allow (`src/api/agent-server-compatibility.ts:34-39`, `src/api/agent-server-adapter.ts:631-644`).

## Notable Patterns

- **Discriminator-by-literal-kind** is the dominant pattern: every event, action, observation, and tool type carries a `kind` string that participates in `switch` and discriminated narrowing. The frontend never depends on Python `type` names — only on the literal `kind` and on consumer-side guard predicates (`src/types/agent-server/type-guards.ts`).

- **Adapter functions normalize wire shapes to consumer shapes.** `toAppConversation(DirectConversationInfo) → AppConversation` (`src/api/agent-server-adapter.ts:317-402`) is the canonical example: it overlays local metadata, normalizes ACP/OpenHands discrimination, and synthesizes default titles. Cloud/local routes (`src/api/cloud/conversation-service.api.ts:30-47`, `src/api/git-service/agent-server-git-service.api.ts:30-94`) layer on top to handle hydration gaps.

- **Branch-on-`active.kind === "cloud"`, else fall through to the typed SDK.** This pattern repeats in `EventService.respondToConfirmation` (`src/api/event-service/event-service.api.ts:46-69`), `BashService.searchEvents` (`src/api/bash-service/bash-service.api.ts:82-118`), `AgentServerRuntimeService.executeCommand` (`src/api/runtime-service/agent-server-runtime-service.ts:32-58`), `ConfigService.searchModels/searchProviders` (`src/api/config-service/config-service.api.ts:65-185`), `SettingsService`/`SecretsService`/`SkillsService`/`ProfilesService`. Each branch uses the same envelope (`callCloudProxy` with `authMode: "bearer" | "session-api-key" | "none"`, `hostOverride` for runtime sandboxes — `src/api/cloud/proxy.ts:5-39`).

- **Trust-boundary validation with structural pattern matches.** `manifest-validation.ts` and `interface-validation.ts` are textbook examples: regex-pinned slugs, closed `LOOKUP_TABLES`, type guards (`isRecord`, `isOneOf`, `isInteger`), `SetupChecker.fail(path, reason)` collecting structured errors instead of throwing early (`src/manifests/manifest-validation.ts:93-99`).

- **Localstorage validation at the boundary.** `readStoredBackends` re-validates every entry before accepting it (`src/api/backend-registry/storage.ts:24-40`, `src/api/backend-registry/storage.ts:130-149`); `readStoredHealth` does the same with `isValidEntry` (`src/api/backend-registry/health-storage.ts:24-39`); the comment on `consecutiveFailures` is explicit about why a tampered `-1` would defeat the disable mechanism.

- **Version pin via `parseAgentServerVersion`.** Full semver with optional `prerelease` and `+build` (`src/api/agent-server-compatibility.ts:273-291`); comparison is lexicographic per-component with prerelease handling.

- **Behavioral encoding via discriminated error subclasses** rather than status codes. Callers check `instanceof AgentServerUnsupportedVersionError` or `error.code === AGENT_SERVER_UNSUPPORTED_VERSION_ERROR_CODE` (`src/api/agent-server-compatibility.ts:105-121`).

- **Constants modules for SDK-generated discriminator names.** `CANVAS_UI_CLIENT_ACTION_KIND`, `CANVAS_UI_CLIENT_TOOL_NAME`, `LEGACY_CANVAS_UI_TOOL_NAME` live in `src/constants/canvas-ui.ts:1-10`; the generated `ClientAction_<tool-name>` discriminator is named once and exported for re-use, never duplicated.

## Tradeoffs

- **One-impl-per-contract by design.** Because every "interface" is a wrapper around a single SDK class generated from the agent-server's OpenAPI, substitutability is theoretical, not exercised. Adding a new provider (e.g., a non-OpenHands runtime) would require writing a parallel SDK or standing up an adapter that itself becomes the contract — a deliberate cost paid in exchange for guaranteed shape parity with the server.

- **Structural-only contracts in the settings path.** `SettingsUpdateRequest` is `Record<string, unknown>`-shaped (`src/api/settings-service/settings-service.api.ts:77-85`); the deep-merge semantics, list replacement, and category-non-breaking invariant live only in doc comments. A future consumer that ignores those rules could ship a regression silently.

- **Single-active-backend policy.** `getEffectiveLocalBackend()` returns the active backend or `null` (`src/api/backend-registry/active-store.ts:140-144`); a cloud-active session cannot borrow a registered-but-inactive local backend. This makes the contract simple and race-free, but bars mixed-backend workflows.

- **Cloud/runtime host detection lives in services, not in the contract.** Each service (`BashService`, `AgentServerRuntimeService`, `AgentServerGitService`) re-implements the `if (active.kind === "cloud" && conversationUrl)` branch. A new transport (e.g., a third backend kind) would require touching every service; the discriminator is structurally supported but the cloud/local pattern isn't centralized into a single dispatch function.

- **Health-store and conversation-metadata are localStorage-only.** Schema-validation guards exist (`isValidEntry`, etc.), but there is no migration path in either module (`src/api/backend-registry/health-storage.ts:41-59`, `src/api/conversation-metadata-store.ts:50-61`). Renaming a field requires coordinated release.

- **Cap detection (`usable_tools`) trusts an optional server payload.** When `usable_tools` is absent, `isAgentServerToolAvailable` returns `true` by default (`src/api/agent-server-compatibility.ts:149-155`). A server that forgets to advertise a tool will get it through, while a server that advertises an unsupported tool will be filtered — the asymmetry is documented but is a real tradeoff between safety and capability.

- **SDK hand-coupling.** A breaking change in `@openhands/typescript-client` (the wire-mirror) is a breaking change here. The CI guard test does not pin against SDK ABI, only against import shape; there is no contract test that asserts the SDK's TypeScript types stay in sync with the Python OpenAPI.

## Failure Modes / Edge Cases

- **Malformed `/server_info` is fatal.** `parseRuntimeServicesInfo` accepts JSON strings, tolerates `null`, rejects non-objects and arrays, and requires `services` to be an object (`src/api/agent-server-adapter.ts:153-173`). An older agent-server that omits the field gracefully returns `null`; a corrupted JSON yields `null` without throwing.

- **`SettingsUpdateRequest`'s `[key: string]: unknown` escape hatch.** A consumer that ships extra keys won't fail at compile time; the SDK forwards them, and the server validates. There's no consumer-side guard rail for that path.

- **Manual `getServerInfo` probes in public mode swallow non-401 errors.** The probe only rethrows on 401; 5xx, 403, network, and timeout errors fall through with a `console.warn` (`src/api/agent-server-compatibility.ts:372-394`). A server that returns 403 in public mode would load the app with an unvalidated key — documented but real.

- **`syncLauncherDefaultLocalBackend` overwrites the API key on every module init.** For any backend whose id is `default-local` and whose host matches the launcher's, the apiKey is silently rewritten from the current session key (`src/api/backend-registry/storage.ts:70-93`). Acceptable for the documented flow; surprising for a user who edited the field in storage.

- **`useStoredActiveBackend` falls back to localStorage.** When sessionStorage is empty, the code reads localStorage and on the next write mirrors the selection back to both (`src/api/backend-registry/storage.ts:205-243`). A cleared localStorage leads to silent re-seed; a user with two tabs editing different backends can race.

- **No substitution path for `RemoteWorkspace.gitChanges` on older agent servers.** The service detects 404 from `getGitCommits` and treats it as "server too old" (`src/api/git-service/agent-server-git-service.api.ts:104-109`); older `gitChanges` endpoints aren't covered by the same fallback.

- **`open-hands.types.ts` carries legacy cloud/v0 shapes alongside v1.** This is a deliberate, documented coexistence, but a casual reader will find `Conversation` (the v0 envelope) and `AppConversation` (the v1 envelope) side-by-side (`src/api/open-hands.types.ts:66-140` vs `src/api/conversation-service/agent-server-conversation-service.types.ts:134-215`) and may mistakenly treat them as the same contract.

- **ACP vs OpenHands discrimination depends on tags, server fields, and saved settings.** `toAppConversation` reads `info.agent?.kind`, falls back to `info.tags?.[ACP_SERVER_TAG_KEY]`, then to `info.agent?.acp_server` (`src/api/agent-server-adapter.ts:323-336`). If a profile launch doesn't stamp the tag and the SDK doesn't repopulate `acp_server`, the chip degrades to a generic "ACP" — documented but observable.

- **Sandbox pause/resume race.** `WebSocketProviderWrapper` must suppress the URL when `sandbox_status === "PAUSED"` to avoid hitting the stale sandbox URL (`AGENTS.md`, conversation-context gating). Symmetric `useActiveConversation` fast-polls on both `!conversation_url` and `sandbox_status === "PAUSED"`; checking only the missing URL would leave the hook on a 30s interval during resume (`AGENTS.md`, Cloud conversation resume gating section).

## Future Considerations

- **Add a runtime conformance test that talks to a fixture agent-server.** The contract is currently enforced by type-checking and a CI guard for HTTP imports; an integration test that asserts the SDK and the agent-server agree on the wire would catch schema drift earlier. A `tests/contract/agent-server-shape.test.ts` fixture, executed against a pinned server image in CI, would be a high-leverage addition.

- **Centralize the cloud/local branch into a transport dispatch.** A single `callAgentServer({ kind: "conversations.list", params })` style dispatcher could replace the `if (active.kind === "cloud") { ... } else { new SomeClient(...).method(...) }` pattern in `EventService`, `BashService`, `AgentServerRuntimeService`, `AgentServerGitService`. The contract would still be per-method but the dual transport would live in one place.

- **Tighten `SettingsUpdateRequest` with a generic literal allow-list.** Today `[key: string]: unknown` accepts any key; tightening to `[key in AppPreferenceField | "agent_settings_diff" | ...]?: ...` would move the "lists are replaced wholesale" invariant into the type system.

- **Schema-versioned `conversation-metadata` and `backend-health` stores.** Today both are validated structurally but have no `version` field (`src/api/conversation-metadata-store.ts:8-40`, `src/api/backend-registry/health-storage.ts:15-22`). Adding `schemaVersion: 1` plus a migrator would let the consumer evolve the shape without coordinated releases.

- **Promote `RESERVED_CONVERSATION_TAG_KEYS` / `PRIORITY_CONVERSATION_TAG_KEYS` to the SDK contract.** Today these live in the frontend (`src/api/agent-server-adapter.ts:478-499`). If the agent-server stamps reserved keys, it should declare them in one place that both sides consume.

- **Move the `BACKEND_HEALTH_STORAGE_KEY` invariants into a typed codec.** `isValidEntry` is good, but the absence of a `schemaVersion` field means a future field addition is a breaking change. A codec with versioning (e.g., `@sinclair/typebox` or `zod`) would unify backend-health, backend-storage, conversation-metadata, and active-selection parsing.

- **Replace `Lookbehind`-style `event.kind === ...` switches with a small finite-state helper.** Many services manually pattern-match on `event.kind`; a `matchEvent(event, { ... })` helper would centralize the contract for the chat rendering pipeline.

- **Carry a `manifest_version` on each admitted catalog.** Today `SETUP_VERSION` and `INTERFACE_VERSION` are constants in `src/manifests/types.ts`; promoting them to fields on the manifests (and emitting them in `client_source` telemetry) would give downstream consumers a per-manifest minimum-version signal.

## Questions / Gaps

- **No conformance test fixture for `@openhands/typescript-client` × agent-server.** Searched `__tests__/`, `tests/`, `src/api/*` for fixture-style contract tests; only per-method mocks (e.g., `agent-server-compatibility-bundled-pin.test.ts`) exist. No evidence found for an end-to-end contract test that records the SDK's wire shape and asserts it against the agent-server's OpenAPI.

- **No documented "two implementations of the same contract" anywhere in the repo.** Every contract has a single implementation in the SDK plus a parallel cloud branch; a literal reading of the rubric's last question — *"can two independent implementations satisfy the same contract?"* — has no evidence either way, because the question's premise doesn't apply yet. The closest the repo gets is the cloud/local dual transport, but both paths route to the same OpenHands backend, just via different hosts.

- **No behavior contract for `Settings` persistence in tests.** Searched `__tests__/` for "deep-merge" or "diff" test fixtures; none found. The semantic claim that `misc_settings_diff` deep-merges while `disabled_skills` replaces wholesale is documented in `src/api/settings-service/settings-service.api.ts:67-76` but not asserted by a test that runs against the agent-server.

- **`AgentKind = "openhands" | "acp"` is locally defined and not enforced by the server.** Searched `src/types/` for `"openhands"`/`"acp"` literal unions; `AgentKind` lives only at `src/types/settings.ts:110`. The agent-server's Pydantic discriminator lives in the SDK; if the SDK adds a third value, the consumer would have to widen the union.

- **Cloud runtime sandbox URLs are not part of an explicit type contract.** `buildHttpBaseUrl(conversationUrl)` is used everywhere a runtime host is needed (`src/api/git-service/agent-server-git-service.api.ts:55-57`, `src/api/bash-service/bash-service.api.ts:101`, `src/api/runtime-service/agent-server-runtime-service.ts:42-46`), but the contract for what makes a valid runtime sandbox URL is not typed anywhere — only inferred from URL parsing.

- **`ConfiguredAgentSettings` shape is hand-coded, not derived from a schema.** `src/api/agent-server-adapter.ts:817-947` writes the agent-settings payload by hand; `SettingsSchema` (`src/types/settings.ts:67-70`) is consumed only at the read side. A mismatch between the two — e.g., a renamed field — would not be caught at compile time.

- **Behavioral specs for `client_tools` cache conflict are documented but not tested.** `client_tools` schema conflicts trigger `ClientToolSchemaConflictError` on the agent-server (per `AGENTS.md`); no test in `__tests__/` covers this path. No evidence found of an integration test that exercises the cache-conflict recovery flow.

---

Generated by `24.02-interface-contract-design` against `openhands`.