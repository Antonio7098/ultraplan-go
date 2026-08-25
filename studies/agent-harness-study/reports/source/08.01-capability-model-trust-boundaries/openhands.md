# Source Analysis: openhands

## Dimension 08.01 — Capability Model and Trust Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (`@openhands/agent-canvas`) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Vite, Zustand, Electron (desktop shell), Node launch scripts; consumes `@openhands/typescript-client` and a Python `openhands-agent-server` (separate repo) |
| Analyzed | 2026-08-24 |

**Scope note on evidence:** this repository is the *frontend* of the OpenHands multi-repo system (`AGENTS.md` "Repository Map"). Tool execution itself lives in the agent-server (`OpenHands/software-agent-sdk`, not present in this source tree). The capability model studied here is therefore the frontend's half of the contract: which tools it grants at conversation creation, what it executes on the model's behalf in the browser, how it authenticates each actor, and where it sanitizes agent-produced content. All citations below are relative to the source root `studies/agent-harness-study/sources/openhands/`.

## Summary

Agent Canvas implements a layered capability model in which **the browser never executes agent power directly** — all stateful tools (`terminal`, `file_editor`, `task_tracker`, optional `browser_tool_set`/`task_tool_set`) are requested by name at conversation start and executed by the agent-server inside its runtime sandbox. The frontend shapes that grant through three explicit gates: server-advertised tool availability (`usable_tools` from `/server_info`), build-time env opt-outs, and user settings (sub-agents toggle). A distinct **client-tool** mechanism (`canvas_ui_control`, `launch_child_conversation`) lets the model *request* actions that only the browser can perform — the agent-server acknowledges the call before any client work happens, and the browser validates, deduplicates, and reports results back as chat messages. Authority for risky server-side actions is delegated to a confirmation policy + security analyzer pair sent with every conversation start (defaulting to `NeverConfirm`). Secrets follow an indirection discipline: values live only on the backend, the UI sees redacted or Fernet-encrypted forms, and conversations receive `LookupSecret` URL pointers resolved server-side at spawn. Content trust boundaries are concrete: agent-authored markdown/HTML is rehype-sanitized against an explicit schema with dedicated XSS tests, and rich HTML previews run in a script-less sandboxed iframe. Auth is split per actor: `X-Session-API-Key` for local backends and automation, bearer tokens or cookies for Cloud (forced through a server-side proxy envelope), and one-shot workspace-session cookies for static file embedding. The main soft spots are fail-open defaults: unadvertised tools are allowed, confirmation is off by default, network topology guidance to the agent is prompt-only, and the session key is stored in browser localStorage.

## Rating

**7 / 10** — Clear, well-documented capability model with explicit interfaces, unit/E2E coverage of gating and auth flows, and real operational safeguards (sanitizer schema tests, iframe sandboxing, CI-enforced API access rules, auto-generated keys). Held out of 8+ by fail-open edges: tool availability defaults to "allowed" when the server advertises nothing (`src/api/agent-server-compatibility.ts:151-153`), confirmation defaults off (`src/services/settings.ts:13`), client tools that create billable Cloud resources have no human-confirmation gate, and agent egress policy exists only as prompt text.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Default server-side tool grant | `DEFAULT_TOOL_NAMES = ["terminal", "file_editor", "task_tracker"]` plus optional `browser_tool_set` / `task_tool_set` names | `src/api/agent-server-adapter.ts:113-115` |
| Browser-tool env kill switch | `browserToolsEnabled()` reads `VITE_ENABLE_BROWSER_TOOLS !== "false"` | `src/api/agent-server-adapter.ts:117-119` |
| Tool inclusion gate | `shouldIncludeTool()` requires server advertisement for browser/task tools and `enable_sub_agents === true` for sub-agents | `src/api/agent-server-adapter.ts:631-644` |
| Capability discovery | `AgentServerInfo.usable_tools` from `/server_info`; `isAgentServerToolAvailable()` returns **true when no list advertised** (fail-open) | `src/api/agent-server-compatibility.ts:25-29`, `34-39`, `149-155` |
| Client tool spec type | `ClientToolSpec` with MCP-style safety annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) | `src/api/canvas-ui-client-tool.ts:9-20` |
| UI-control client tool | `canvas_ui_control`: tab switching/navigation only; annotated `readOnlyHint: true, idempotentHint: true, openWorldHint: false` | `src/api/canvas-ui-client-tool.ts:60-91` |
| Child-conversation client tool | `launch_child_conversation` schema (target/task/isolation); annotated `readOnlyHint: false … openWorldHint: true` ("cloud target reaches OpenHands Cloud") | `src/api/launch-child-conversation-client-tool.ts:61-112` |
| Client tools attached only to OpenHands-kind launches | `client_tools` registered for `launchAgentKind === "openhands"`, empty for ACP subprocesses | `src/api/agent-server-adapter.ts:1116-1119` |
| Client-tool execution authority | Agent-server acknowledges call before browser work; result reported back via injected user message so the model relays it | `src/services/child-conversation-launch.ts:451-458`, `459-497` |
| Client-side validation beyond JSON Schema | `validateLaunchParams()` enforces enum + cross-target rules the SDK drops; failures return corrective guidance, launching nothing | `src/services/child-conversation-launch.ts:99-194` |
| Replay protection for non-idempotent launches | `claimToolCall()` ledger in localStorage keyed by parent conversation + tool-call id | `src/services/child-conversation-launch.ts:196-227` |
| Cloud-child guardrail | Launch fails fast with guidance when no Cloud backend connected; cloud children always isolated sandboxes | `src/services/child-conversation-launch.ts:386-396`; `src/api/launch-child-conversation-client-tool.ts:26-30` |
| Confirmation policy derivation | `NeverConfirm` default → `AlwaysConfirm` when enabled → `ConfirmRisky(threshold HIGH, confirm_unknown)` with LLM analyzer | `src/api/agent-server-adapter.ts:593-605` |
| Security analyzers | `LLMSecurityAnalyzer` / `PatternSecurityAnalyzer` / `PolicyRailSecurityAnalyzer` mapped from settings into start payload | `src/api/agent-server-adapter.ts:607-618`, `1169-1173` |
| Confirmation default off | `DEFAULT_SETTINGS.confirmation_mode: false` | `src/services/settings.ts:13` |
| Verification settings UI source-of-truth note | SDK deprecated agent-side verification fields; canonical copies live in ConversationSettings | `src/routes/verification-settings.tsx:4-11` |
| Per-backend auth header selection | `buildAuthHeaders()`: cloud cookie → `{}`, cloud api-key → `Authorization: Bearer`, local → `X-Session-API-Key` | `src/api/backend-registry/auth.ts:9-18` |
| Backend kinds/auth modes | `BackendKind = "local" \| "cloud"`, `BackendAuthMode = "api-key" \| "cookie"` | `src/api/backend-registry/types.ts:1-10` |
| Session key delivery paths | Build-time `VITE_SESSION_API_KEY` or serve-time `window.__AGENT_CANVAS_SESSION_API_KEY__` injection | `src/api/agent-server-config.ts:102-132` |
| Static-server key injection | Injects `window.__AGENT_CANVAS_SESSION_API_KEY__=<literal>` before `</head>` | `scripts/static-server.mjs:339`, `424-427` |
| Public mode deliberately keyless | Public mode does NOT bake the key; 401 → `ApiKeyEntryScreen` | `scripts/dev-with-automation.mjs:507-509`; `src/api/agent-server-compatibility.ts:132-133` |
| Key validation against protected endpoint | `/server_info` is unprotected, so bootstrap re-probes `getSettings()` under auth to catch rotated keys; race window documented | `src/api/agent-server-compatibility.ts:368-395` |
| Version floor as supply-chain gate | `assertAgentServerVersionIsSupported()` blocks unsupported/unknown agent-server versions before caching info | `src/api/agent-server-compatibility.ts:293-318` |
| Cloud CORS boundary | All Cloud/sandbox calls must go through `callCloudProxy` envelope POSTed to local `/api/cloud-proxy` (server-side hop) | `src/api/cloud/proxy.ts:18-39` |
| Proxy wiring & org scoping | `CloudClient` gets proxy host+headers; `X-Org-Id` scoped to active backend only | `src/api/cloud/client.ts:19-31`, `33-55` |
| Sandbox auth mode split | Runtime-sandbox proxy calls use `authMode: "session-api-key"`, cloud uses bearer | `src/api/cloud/proxy.ts:13-14`; AGENTS.md Rule 2 examples |
| Cookie-auth gate for locked deployments | `POST /api/authenticate` withCredentials; only when locked-cloud mode and non-localhost origin | `src/api/main-app-auth.ts:15-36` |
| Secrets API surface (names only) | `SecretsService.getSecrets()` returns `CustomSecretWithoutValue[]` — name/description only | `src/api/secrets-service.ts:26-49` |
| Redaction vs encrypted exposure | `X-Expose-Secrets` modes: undefined → `"**********"`, `"encrypted"` → Fernet round-trip safe, `"plaintext"` → "backend use only!" | `src/api/settings-service/settings-service.api.ts:115-122`, `421-438` |
| Encrypted-settings conversation flow | `getSettingsForConversation()` fetches encrypted settings; refuses redacted fallback; sets `secrets_encrypted` flag | `src/api/settings-service/settings-service.api.ts:478-512`; `src/api/agent-server-adapter.ts:1147-1159` |
| Fernet detection helper | `FERNET_TOKEN_PREFIX = "gAAAAA"` + recursive `hasEncryptedMcpSecrets()` scan | `src/api/agent-server-adapter.ts:540`, `572-591` |
| LookupSecret indirection | Saved secrets ride as `{kind: "LookupSecret", url: "/api/settings/secrets/{name}", headers: <auth>}`; agent-server resolves at spawn | `src/api/agent-server-adapter.ts:995-1000`, `1203-1228` |
| MCP credential substitution | Redacted placeholders replaced with stored encrypted leaves for connectivity probes; plaintext never rendered in browser | `src/api/mcp-service/mcp-redacted-credentials.ts:77-137` |
| Git tokens server-side only | Only `provider_tokens_set` booleans reach the GUI | `src/types/settings.ts:132`; AGENTS.md "Git provider tokens…" note |
| Deliberate agent-facing secret grant | Session key seeded into agent-server secrets as `OPENHANDS_AUTOMATION_API_KEY` so agents can authenticate to automation backend | `scripts/dev-with-automation.mjs:1202-1234` |
| Encryption key lifecycle | Docker entrypoint auto-generates & persists `OH_SECRET_KEY` (32 bytes from `/dev/urandom`) and session key so image doesn't "run wide-open" | `docker/entrypoint.sh:175-214` |
| Frontend-initiated shell access | `useBashCommandRunner` executes commands over `/sockets/bash-events` WS after sending `{type:"auth", session_api_key}`; used by git-info probing | `src/hooks/use-bash-command-runner.ts:87-101`, `177-203`; `src/hooks/query/use-local-git-info.ts:84` |
| Terminal is read-only telemetry | xterm renders the command store from the event stream; no user input channel | `src/components/features/terminal/terminal.tsx:10-37`; `src/hooks/use-terminal.ts:151-190` |
| Browser tab is read-only screenshots | Renders base64 PNG screenshot + URL string from agent browsing events | `src/components/features/browser/browser.tsx:7-20` |
| Workspace file embed credential | `oh_workspace_session_key` cookie minted by exchanging session key (`POST /api/auth/workspace-session`) because iframes can't send headers; local-only | `src/hooks/query/use-workspace-session.ts:18-60` |
| HTML/SVG preview sandbox | `sandbox="allow-same-origin"` without `allow-scripts` → agent-written scripts inert in preview | `src/components/features/files-tab/file-content-viewer.tsx:165-181` |
| PDF exception rationale | Unsandboxed PDF iframe justified by content-type sniffing + plugin behavior, documented inline | `src/components/features/files-tab/file-content-viewer.tsx:140-158` |
| Markdown sanitizer schema | Explicit schema bans `style`, `data:` protocol; protocol allow-lists for src/href; sanitize runs *after* rehype-raw | `src/components/features/markdown/markdown-renderer.tsx:18-86`, `170-176` |
| Sanitizer regression tests | Tests strip `<script>`, `onclick`/`onerror`, `javascript:` URLs, `style`, `data:text/html`, `<iframe>` | `__tests__/components/features/markdown/markdown-renderer.test.tsx:64-176` |
| API-access architectural guard | CI test forbids raw axios/fetch to agent-server outside 3 allow-listed infra files | `src/api/no-direct-agent-server-calls.test.ts:7-11`, `32-79` |
| Electron renderer hardening | `nodeIntegration:false, contextIsolation:true` on both windows; external URLs forced to system browser | `electron/main.mjs:306-311`, `346-350`, `362-383` |
| Ingress network segmentation | Path-prefix router: `/api/automation/*`, `/api/*`→agent-server, `/*`→frontend; unknown route → 503; `Referrer-Policy: no-referrer` prefixes | `scripts/ingress.mjs:166-210`; `scripts/static-server.mjs:162-169` |
| Editor-token referer mitigation | VSCode connection token derived from session key travels in query string; mitigated with `--no-referrer-prefix` | `scripts/dev-with-automation.mjs:785-797` |
| Prompt-level network policy to agent | `<RUNTIME_SERVICES>` block lists agent-visible URLs, instructs "Trust this block over guessing" | `src/api/agent-server-adapter.ts:215-300`, `286-296` |
| Client-source attribution headers | Coarse `X-OpenHands-Client(-Version)` headers; explicitly excludes keys/codes/content | `src/api/client-source.ts:6-20` |
| Tool-gating unit tests | Server-not-advertising ⇒ only default trio; sub-agents disabled ⇒ no `task_tool_set` even when advertised | `__tests__/api/agent-server-adapter.test.ts:277-347` |
| Confirmation-policy unit test | `confirmation_mode:true` + LLM analyzer ⇒ `ConfirmRisky(HIGH, confirm_unknown)` payload asserted | `__tests__/api/agent-server-adapter.test.ts:349-376` |
| Secret-channel unit tests | Host-relative LookupSecret serialization; ACP credentials delivered uniformly via `request.secrets` only; no mirrored `agent_context.secrets` | `__tests__/api/agent-server-adapter.test.ts:473`, `561-682` |
| Worktree isolation default | New conversations request `worktree: true` (per-conversation git worktree) | `__tests__/api/agent-server-adapter.test.ts:400-412` |
| Auth-mode E2E coverage | Fresh-install injection, stale-key rotation recovery, public-gate reject/accept/stale-reprompt specs | `tests/e2e/mock-llm/backends/mock-llm-auth-modes.spec.ts:46-363` |

## Answers to Dimension Questions

### 1. What can the agent do?

The agent's effective capability set is fixed at conversation creation by the frontend's payload builder. Every OpenHands-kind conversation receives `terminal`, `file_editor`, and `task_tracker` (`src/api/agent-server-adapter.ts:113`, assembled at `646-677`); `browser_tool_set` is added unless the deployer set `VITE_ENABLE_BROWSER_TOOLS=false` (`src/api/agent-server-adapter.ts:117-119`) and the server advertises it; `task_tool_set` (sub-agents) additionally requires the user's `enable_sub_agents === true` setting (`src/api/agent-server-adapter.ts:636-641`). Within the runtime sandbox the agent also gets whatever the server/SDK attaches (the profile-launch path makes that the server's responsibility — see the enrichment-boundary comment, `src/api/agent-server-adapter.ts:1089-1098`). Two client tools extend reach into the browser tier: `canvas_ui_control` (switch UI tabs, navigate preview — read-only by construction) and `launch_child_conversation` (create local child conversations in the same workspace, optionally isolated via git worktree, or billable Cloud children in fresh sandboxes — `src/api/launch-child-conversation-client-tool.ts:22-52`). Finally, skills: bundled public skills are merged into `agent_context.skills` with keyword triggers unless the user disabled them (`src/api/agent-server-adapter.ts:722-787`).

### 2. What can the model only request but not directly do?

Exactly two things, both via registered client tools (`src/api/agent-server-adapter.ts:1116-1119`). The agent-server acknowledges a client-tool call *before* the browser acts, and results flow back as a message the model must relay (`src/services/child-conversation-launch.ts:451-458`, `488-496`). Concretely:

- `canvas_ui_control` can only change what the user sees (tabs/file navigation); it holds no data path beyond existing UI state (`src/api/canvas-ui-client-tool.ts:22-58`).
- `launch_child_conversation` requests conversation creation; the browser validates parameters the server cannot (`enum` is dropped by the SDK's pydantic conversion — documented at `src/services/child-conversation-launch.ts:99-109`), checks that a Cloud backend actually exists before honoring `target:"cloud"` (`386-396`), de-duplicates replays via a localStorage ledger (`205-227`), and downgrades impossible worktree isolation to `shared` with an explicit warning instead of failing (`303-323`).

The inverse also holds: the model cannot add server tools mid-conversation — the frontend composes the tool list once, and the agent-server caches client-tool schemas per process (`ClientToolSchemaConflictError` note, `src/api/agent-server-adapter.ts:1111-1115`).

### 3. Where is authority enforced?

Four tiers, each with a named owner:

- **Runtime authority (dominant):** the agent-server executes all file/terminal/browser/task tools inside its workspace; the frontend only composes requests through typed clients. This is architecturally enforced — a CI test rejects ad-hoc HTTP to the agent-server from anywhere except three infrastructure files (`src/api/no-direct-agent-server-calls.test.ts:7-11`, `32-79`).
- **Action-confirmation authority:** the frontend derives a `confirmation_policy` (and optional security analyzer) into the start payload, and the *server* enforces it against tool actions (`src/api/agent-server-adapter.ts:593-605`, `1120-1121`, `1169-1173`). The default is `NeverConfirm` (`src/services/settings.ts:13`); opting in yields either `AlwaysConfirm` or `ConfirmRisky` at HIGH threshold with `confirm_unknown` when the LLM analyzer is selected.
- **User/session authority:** every local request carries `X-Session-API-Key` (`src/api/backend-registry/auth.ts:17`); public-mode deployments withhold the key and force interactive entry on 401 (`src/api/agent-server-compatibility.ts:132-133`). WebSocket channels authenticate with an in-band auth frame (`src/utils/websocket-auth.ts:4-17`).
- **Browser-render authority:** agent-produced content passes the sanitize schema (`src/components/features/markdown/markdown-renderer.tsx:47-86`) and script-less preview iframes (`file-content-viewer.tsx:177`).

### 4. Are dangerous capabilities isolated?

Partially, with clear intent:

- **Isolated well:** agent HTML previews run in `sandbox="allow-same-origin"` iframes with `allow-scripts` deliberately omitted (`src/components/features/files-tab/file-content-viewer.tsx:166-181`); markdown XSS is schema-blocked with direct tests (`markdown-renderer.test.tsx:64-176`); secrets travel only as server-resolved LookupSecrets (`agent-server-adapter.ts:1203-1228`); Electron renderers run with `nodeIntegration:false`/`contextIsolation:true` and external navigations divert to the system browser (`electron/main.mjs:306-350`, `362-383`); Cloud traffic is funneled through a server-side proxy that keeps Cloud origins unreachable from the browser directly (`src/api/cloud/client.ts:46-53`).
- **Isolated by process boundaries, not by grant:** child conversations default to git-worktree isolation (`__tests__/api/agent-server-adapter.test.ts:400-412`), and Cloud children always get their own sandbox (`launch-child-conversation-client-tool.ts:26-30, 46-47`).
- **Not isolated:** the terminal tool shares the conversation workspace with the user's own machine-level bash runner (`src/hooks/use-bash-command-runner.ts`) — there is no per-actor privilege separation inside the sandbox visible from this repo; the automation session key handed to agents (`dev-with-automation.mjs:1202-1234`) is the *same* key the browser uses, so the agent's blast radius against the automation backend equals the user's.

## Architectural Decisions

1. **Frontend as capability composer, not executor.** The start-request builder is the single choke point where tool grants, confirmation policy, security analyzer, skills, MCP config, and secret bindings are decided (`src/api/agent-server-adapter.ts:1050-1231`). This keeps enforcement server-side while making grants auditable in one pure function with extensive unit tests.
2. **Client tools as a first-class request/exec split.** Rather than hiding UI control and delegation behind prompt conventions, they are declared JSON-schema tools with MCP-style safety annotations and executed by the browser (`canvas-ui-client-tool.ts:9-20`). The acknowledgment-before-execution semantics are stated explicitly (`child-conversation-launch.ts:451-458`).
3. **Capability discovery over hardcoding.** Tool inclusion ANDs local policy with `usable_tools` advertised by `/server_info` (`agent-server-compatibility.ts:149-155`), so an older/newer server cannot be granted tools it lacks — though the check fails open when metadata is absent.
4. **Secrets as references, not values.** The LookupSecret pattern plus `X-Expose-Secrets` redaction tiers mean plaintext credentials cross the browser boundary zero times in steady state; the encrypted round-trip exists solely so conversation-start payloads can carry cipher text the server decrypts (`settings-service.api.ts:115-122`, `478-512`).
5. **Proxy-enforced network topology.** Because Cloud origins disallow localhost CORS, all Cloud calls are enveloped through the local agent-server (`/api/cloud-proxy`), making the local backend both router and credential vault for cloud access (`AGENTS.md` API Access Rule 2; `cloud/proxy.ts:18-39`).
6. **Secure-by-default packaging.** Launchers generate and persist a 64-character session key and a separate Fernet `OH_SECRET_KEY` on first run rather than shipping defaults (`docker/entrypoint.sh:175-214`; AGENTS.md "Security" section).

## Notable Patterns

- **Fail-safe validation with corrective feedback:** malformed client-tool calls never throw at the user; every failure becomes structured guidance returned to the model (`child-conversation-launch.ts:505-528`), turning policy violations into self-correction loops.
- **Idempotency ledgers for non-idempotent powers:** replay-prone event delivery (socket resend modes) is neutralized by claiming tool-call IDs before any network work (`child-conversation-launch.ts:196-227`).
- **Documented deviation permits:** the unsandboxed PDF viewer and the auth-bootstrap 401 race each carry inline justifications and residual-risk statements (`file-content-viewer.tsx:141-149`; `agent-server-compatibility.ts:380-385`) — trust-boundary decisions are written down where they are made.
- **Prompt-as-network-map:** the `<RUNTIME_SERVICES>` suffix gives the agent authoritative service URLs and explicitly warns against port-scanning guesses (`agent-server-adapter.ts:286-296`), reducing exploratory side effects inside the sandbox.
- **Defense-in-depth rendering:** sanitize-after-raw ordering, exported schema for direct tests, and rel-token handling to avoid reverse-tabnabbing regressions (`markdown-renderer.tsx:170-176`, `47-58`).

## Tradeoffs

- **Fail-open availability vs compatibility:** defaulting `isAgentServerToolAvailable` to true for servers that omit `usable_tools` maximizes compatibility but means a silent server bug re-enables browser tools and sub-agents (`agent-server-compatibility.ts:151-153`).
- **UX convenience vs key exposure (local mode):** baking/injecting the session key into the page (env var or `window.__AGENT_CANVAS_SESSION_API_KEY__`, then localStorage persistence via `syncLauncherDefaultLocalBackend`) removes all friction but places a full-authority credential in XSS-reachable storage; the sanitizer stack is the compensating control.
- **Client-tool autonomy vs human oversight:** `launch_child_conversation` can create billable Cloud conversations with only advisory annotations and a dedup ledger — no confirmation-policy hook covers client-tier actions, since `confirmation_policy` governs server-side tool execution.
- **Advisory vs enforced network policy:** telling the agent which hosts exist is cheaper than egress filtering and works across deployment modes, but a compromised/misbehaving model can still ignore the block; actual egress control would have to live in the runtime (outside this repo).
- **Shared session key across tiers:** browser, agent, and automation backend share one `X-Session-API-Key` value (`dev-with-automation.mjs:451-452`, `1040-1041`), simplifying ops at the cost of undifferentiated authority between actors.

## Failure Modes / Edge Cases

- **Auth bootstrap race:** if connectivity drops between the `/server_info` probe and the protected-endpoint probe, the app loads with an unvalidated key and later 401s arrive through React Query, bypassing the auth screen until refresh — accepted and documented (`agent-server-compatibility.ts:380-385`).
- **Ledger unavailable:** if localStorage writes fail, launch replay protection is skipped in favor of launching at all (`child-conversation-launch.ts:220-225`) — duplicate billable Cloud children become possible after socket replays.
- **Scratch-workspace worktree fallback:** children silently degrade from `worktree` to `shared` isolation on unborn-HEAD workspaces, exposing in-progress files to a sibling agent with only a textual warning (`child-conversation-launch.ts:269-270`, `303-323`).
- **Redaction-substitution dependency:** MCP connectivity probes fetch encrypted settings to substitute redacted placeholders; on fetch failure the probe proceeds with placeholders and fails opaquely (`mcp-redacted-credentials.ts:100-137`).
- **Referer leak surface:** the VSCode editor token rides in the URL query string; protection is a header policy on specific prefixes, so any future prefix misconfiguration reopens leakage (`dev-with-automation.mjs:785-797`; `ingress.mjs:178-186`).
- **Sanitizer scope:** the schema governs chat/preview markdown; other render paths must opt in (transcript export re-implements inert-text sanitization separately, `src/utils/transcript-export/index.ts:136-138`).

## Future Considerations

- Add a human-confirmation (or at least budget-guard) step for client tools with real-world cost, mirroring what `confirmation_policy` provides server-side; the annotations already distinguish risk, so a UI consent prompt could key off them (`canvas-ui-client-tool.ts:13-19`).
- Flip tool-availability to fail-closed once the minimum agent-server version reliably ships `usable_tools` (version floor machinery already exists at `agent-server-compatibility.ts:293-318`).
- Differentiate credentials per actor (browser vs agent vs automation) now that the registry abstraction centralizes key handling (`backend-registry/auth.ts`), enabling revocation scoped to one tier.
- Promote the `<RUNTIME_SERVICES>` advisory into an enforced egress allowlist in the runtime, keeping the block as documentation of the enforced policy.
- Move the session key out of localStorage to a memory-only + window-global strategy for embedded/library consumers, given `getBakedSessionApiKey()` already supports injection without storage.

## Questions / Gaps

- **Sandbox internals are out of view.** Container/runtime isolation, filesystem scoping, and network egress rules for tool execution live in `software-agent-sdk`/deployment (not in this tree). No evidence found here for OS-level sandboxing of `terminal`; searched `src/`, `docker/`, `scripts/` for sandbox/egress configuration and found only process routing and key management.
- **Confirmation UX at runtime.** This repo renders a `ConfirmationModeEnabled` indicator (`src/components/features/chat/confirmation-mode-enabled.tsx:7-12`) and sends the policy, but the approve/reject interaction itself is server/event-stream driven; its robustness could not be assessed from frontend code alone.
- **ACP agent capability parity.** ACP launches register *no* client tools (`agent-server-adapter.ts:1116-1119`), but whether ACP CLIs enforce equivalent confirmation policies depends entirely on the external CLI process; no evidence found in this repo.
- **Cloud-side policy enforcement.** Whether `confirmation_policy`/security analyzers are honored identically for Cloud conversations (payload sent via Cloud APIs) could not be verified here; the Cloud conversation service mirrors settings fields (`src/api/cloud/settings-service.api.ts:110-111`) but enforcement evidence would be in the Cloud backend.

---

Generated by `Dimension 08.01: Capability Model and Trust Boundaries` against `openhands`.
