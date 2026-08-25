# Source Analysis: openhands

## Dimension 17.01 — Sandbox Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (OpenHands Agent Canvas) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React 19, React Router 7, Node 22+, Electron; Python agent-server consumed via `uvx` and Docker (not implemented here) |
| Analyzed | 2026-08-24 |

## Summary

This repository is the **frontend** of the multi-repo OpenHands system (`docs/architecture.md:17` explicitly states the repo does *not* provide "the sandbox or workspace isolation layer" — that belongs to `OpenHands/software-agent-sdk`). Consequently, tool code never executes in this codebase: terminal commands, file edits, and browser actions run inside an **agent-server runtime**, and this repo's job is to address that runtime correctly across three distinct execution topologies:

1. **Local process** — the agent-server is spawned on the host via `uvx` by the launchers (`scripts/dev-safe.mjs`, wired into Electron at `electron/main.mjs:593-616`). There is no OS-level sandbox; the agent runs with full user privileges.
2. **Container** — a Docker all-in-one image built FROM the upstream `ghcr.io/openhands/agent-server` image, running as non-root user `openhands` with tini as PID 1 (`docker/Dockerfile:73,157,170`), plus a Helm chart pinning `runAsNonRoot: true`, `runAsUser: 10001` and default-off RBAC (`helm/agent-canvas/values.yaml:170-180`).
3. **Remote cloud sandbox** — conversations run in sandboxes at `*.prod-runtime.all-hands.dev` whose lifecycle (STARTING/RUNNING/PAUSED/ERROR/MISSING) is managed by the OpenHands Cloud API and observed from this frontend (`src/api/cloud/sandbox-service.types.ts:1-6`, `src/api/runtime-service/agent-server-runtime-service.ts:19-22`).

Boundary enforcement visible in this source is layered but unevenly technical: per-conversation session API keys with auto-generation and secure defaults (`docker/entrypoint.sh:175-209`), a server-side cloud-proxy hop that keeps sandbox traffic out of CORS reach (`src/api/cloud/proxy.ts:18-39`, `src/api/bash-service/bash-service.api.ts:26-37`), action-level confirmation/security policies attached to every conversation start payload (`src/api/agent-server-adapter.ts:593-613`), a sandbox-status state machine that gates network calls against unreachable sandboxes (`src/hooks/query/use-bash-command-logs.ts:39-57`), and an explicit, documented threat model for unsandboxed local deployments (`docs/SELF_HOSTING.md:8-10`). Several known weaknesses are acknowledged in-code rather than fixed (VSCode editor token equals the session API key, path-prefix routing "routes but does not isolate" — `config/defaults.json:36-38`, `docker/entrypoint.sh:407-433`).

## Rating

**7 / 10.** Within its scope as the boundary-*consumer*, the model is clear, explicitly typed, tested, and operationally safeguarded: dual local/cloud execution paths are centralized in dedicated services (`src/api/runtime-service/agent-server-runtime-service.ts:24-94`, `src/api/bash-service/bash-service.api.ts:44-119`), the sandbox lifecycle is a discriminated union with preflight gating and regression tests (`__tests__/api/cloud/sandbox-service.test.ts:39-40`, `__tests__/api/cloud/conversation-pause.test.ts:66-84`), secrets are auto-generated with `chmod 600` persistence, and the self-hosting docs state the threat model honestly. It falls short of 8–9 because: (a) local mode has **no** technical host boundary — mitigation is documentation and firewall advice only (`docs/SELF_HOSTING.md:8-10,54-58`); (b) known credential weaknesses (editor token = session key, upstream SDK issue #4317) are worked around at the route level rather than fixed (`docker/entrypoint.sh:419-433`); and (c) enforcement of frontend access discipline is a lint-style CI scan, not runtime security (`src/api/no-direct-agent-server-calls.test.ts:5-12`).

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Repo scope disclaimer | Architecture doc: this repo does not provide "the sandbox or workspace isolation layer"; recommends Docker sandbox mode + server hardening | `docs/architecture.md:17,82` |
| Tool execution location | `<RUNTIME_SERVICES>` block tells agents "Tool calls (terminal, file_editor, browser, etc.) execute here" at the Agent Server URL | `src/api/agent-server-adapter.ts:231-296`; example block `AGENTS.md` (Runtime Services section) |
| Cloud runtime host | Runtime lives at `*.prod-runtime.all-hands.dev`; browser cannot reach it directly (CORS); all calls tunnel through `callCloudProxy` with per-conversation `X-Session-API-Key` | `src/api/runtime-service/agent-server-runtime-service.ts:17-23,34-52`; `src/api/bash-service/bash-service.api.ts:26-37,84-106` |
| Proxy envelope client | `callCloudProxy` builds a `CloudClient` with mandatory proxy when `hostOverride` targets a runtime; bearer vs session-api-key auth modes | `src/api/cloud/proxy.ts:18-39`; `src/api/cloud/client.ts:33-63` |
| Sandbox lifecycle type | `V1SandboxStatus = STARTING \| RUNNING \| PAUSED \| ERROR \| MISSING`; `V1SandboxInfo` carries `sandbox_spec_id`, `session_api_key`, `exposed_urls` | `src/api/cloud/sandbox-service.types.ts:1-21`; mirrored in `src/api/conversation-service/agent-server-conversation-service.types.ts:7-15` |
| Sandbox read API | Batch-fetch sandboxes `GET /api/v1/sandboxes?id=...`; exposed_urls carry cloud-computed public URLs (VSCODE, AGENT_SERVER, WORKER_*) | `src/api/cloud/sandbox-service.api.ts:14-38` |
| Pause / resume endpoints | `POST /api/v1/sandboxes/{id}/pause` and `/resume`; resume is lightweight unpause, not re-provisioning | `src/api/cloud/conversation-service.api.ts:223-254` |
| Resume orchestration | Route effect resumes PAUSED sandboxes once per conversation id (ref-guarded), then relies on 3 s fast-poll until URL populates | `src/routes/conversation.tsx:145-174` |
| Status-driven polling | Fast-poll 3 s while `conversation_url` missing or `sandbox_status === "PAUSED"` | `src/hooks/query/use-active-conversation.ts:17-32` |
| WebSocket suppression | WS suppressed while `sandbox_status === "PAUSED"` so it does not hit the stale sandbox host | `src/contexts/websocket-provider-wrapper.tsx:25-31` |
| Preflight gating | `SandboxIssue` taxonomy maps status → missing/paused/starting/errored before any request fires; 5xx/404 classified as `unreachable` | `src/hooks/query/use-bash-command-logs.ts:16-21,39-57,68-99,134-157` |
| Container definition | All-in-one Dockerfile FROM `ghcr.io/openhands/agent-server`, `USER openhands`, VOLUME mounts, single EXPOSE 8000, tini entrypoint | `docker/Dockerfile:73,151-168,170` |
| Entrypoint secure defaults | Auto-generates 64-hex-char `OH_SECRET_KEY` and session API key, persists both with `chmod 600`; refuses to start automation without a session key | `docker/entrypoint.sh:175-218` |
| Editor route hardening | Public-mode static server deliberately omits the editor route because the editor connection token derives from the session key (upstream #4317); `--no-referrer-prefix` prevents Referer leaks | `docker/entrypoint.sh:20-25,411-433,403` |
| VSCode port exposure caveat | openvscode-server binds 0.0.0.0 inside the container; `--network host` leaves it reachable with only its connection token | `docker/entrypoint.sh:20-25` |
| Prefix routing ≠ isolation | Config comment: "a path prefix routes but does not isolate, so the editor shares the canvas's browser origin and therefore its localStorage" | `config/defaults.json:34-38` |
| Origin/capability intersection | Frontend must intersect server-reported editor capability with the origin's own route table to avoid rendering someone else's workspace | `src/utils/vscode-origin.ts:1-22` |
| Workspace cookie boundary | Local-only `POST /api/auth/workspace-session` exchanges `X-Session-API-Key` for an `oh_workspace_session_key` cookie scoped to `/api/conversations` so iframes/imgs can embed workspace artifacts | `src/hooks/query/use-workspace-session.ts:18-45,60-87` |
| Kubernetes posture | Helm values pin `runAsNonRoot/runAsUser/runAsGroup: 10001`; RBAC master switch default false; namespace-scoped admin bindings opt-in | `helm/agent-canvas/values.yaml:170-180,47-60` |
| Confirmation policy | Conversation start payload carries `confirmation_policy`: `NeverConfirm` / `AlwaysConfirm` / `ConfirmRisky(threshold=HIGH, confirm_unknown)` | `src/api/agent-server-adapter.ts:593-604,1120-1121` |
| Security analyzers | Payload attaches `LLMSecurityAnalyzer`, `PatternSecurityAnalyzer`, or `PolicyRailSecurityAnalyzer` per settings | `src/api/agent-server-adapter.ts:606-613,1169-1172` |
| Tool-surface gating | `browser_tool_set` omitted when `VITE_ENABLE_BROWSER_TOOLS=false` or server doesn't advertise it in `usable_tools`; `task_tool_set` gated on `enable_sub_agents === true` | `src/api/agent-server-adapter.ts:114-118,627-640` |
| Child-conversation isolation model | Tool spec: `target="cloud"` = isolated cloud sandbox; `isolation="worktree"` (default) vs `"shared"` for local; cross-target parameter misuse returns corrective guidance without launching | `src/api/launch-child-conversation-client-tool.ts:20-58,103-111`; enforcement `src/services/child-conversation-launch.ts:106-192` |
| Worktree fallback downgrade | If parent workspace lacks git commits, child silently downgrades `worktree`→`shared` with an explanatory `isolation_note` | `src/services/child-conversation-launch.ts:300-310` |
| Per-run working dir | `DEFAULT_WORKING_DIR = "workspace/project"`; `VITE_WORKING_DIR` env override; per-conversation dir derived by appending conversation-id hex | `src/api/agent-server-config.ts:1,196-205` |
| Start payload workspace | `workspace: { kind: "LocalWorkspace", working_dir }` plus `worktree: options.worktree ?? true` in every start request | `src/api/agent-server-adapter.ts:973-978,1000-1025,1125` |
| Frontend access discipline guard | CI test scans all of `src/` for raw axios/fetch/HttpClient use; only 3 allow-listed files may bypass typed clients | `src/api/no-direct-agent-server-calls.test.ts:5-12,30-60` |
| Static-server auth flags | `--session-api-key` and `--auth-required` are mutually exclusive (startup error otherwise); `--reject-prefix` returns 503 for unmatched prefixes | `scripts/static-server.mjs:181-190,152-160,552-590` |
| Electron renderer containment | Both windows set `nodeIntegration: false`, `contextIsolation: true`; preload uses contextBridge; IPC handlers verify sender webContents | `electron/main.mjs:306-311,346-349,490-498`; `electron/preload.cjs:1-31` |
| Electron egress control | `window.open()` to non-localhost URLs is denied and redirected to the system browser; OAuth popup navigations intercepted | `electron/main.mjs:362-401` |
| Self-hosting threat model | Warning: the agent "can read and write the filesystem of the machine it runs on, execute shell commands, and reach the network"; defenses listed are network firewall + API key | `docs/SELF_HOSTING.md:8-10,50-58` |
| Tests | Sandbox batch-fetch targets correct endpoint; pause throws when conversation lacks `sandbox_id`; RUNTIME_SERVICES suffix reaches the LLM in E2E | `__tests__/api/cloud/sandbox-service.test.ts:39-40`; `__tests__/api/cloud/conversation-pause.test.ts:66-84`; `tests/e2e/mock-llm/automations/mock-llm-automation.spec.ts` |

## Answers to Dimension Questions

### 1. Where does code execute?

Agent/tool code executes **inside an agent-server runtime process that this repository launches but does not implement**. Three placements exist:

- **Local**: launchers spawn `openhands-agent-server` via `uvx` as a host process (`electron/main.mjs:593-616` passes `staticMode: true` into `dev-with-automation.mjs`; cold-start uvx download is explicitly anticipated at `electron/main.mjs:600-604`). The Docker entrypoint runs the same server as a binary inside the container (`docker/entrypoint.sh:287-299`).
- **Container**: the all-in-one image starts agent-server (:18000), automation (:18001), and static frontend behind one ingress (:8000) (`docker/entrypoint.sh:5-13,286-405`).
- **Cloud**: each cloud conversation provisions its own sandbox; the GUI learns the sandbox URL from `conversation_url` and its lifecycle from `sandbox_status` (`src/api/conversation-service/agent-server-conversation-service.types.ts:188-195`).

The frontend itself executes shell-shaped operations only by **requesting** them from the runtime: file listing runs a bounded `find ... | head -n` through `/api/bash/execute_bash_command` (`src/hooks/query/use-workspace-files.ts:41-47,57-60`), and arbitrary command execution goes through `AgentServerRuntimeService.executeCommand` (`src/api/runtime-service/agent-server-runtime-service.ts:25-68`).

### 2. What boundaries exist between agents and the host?

- **Network/CORS boundary (cloud)**: browsers cannot call `*.prod-runtime.all-hands.dev` directly; everything is tunneled through the local agent-server's proxy envelope with the sandbox's ephemeral `session_api_key` (`src/api/runtime-service/agent-server-runtime-service.ts:17-23`; `src/api/event-service/event-service.api.ts:27-33`).
- **Auth boundary**: per-conversation session keys; auto-generated, persisted `chmod 600`, never defaulted (`docker/entrypoint.sh:193-209`); public mode requires explicit user key entry and never bakes the key into HTML (`scripts/static-server.mjs:149,181-190`; `README` flow in `AGENTS.md`).
- **Action-policy boundary**: `confirmation_policy` + `security_analyzer` travel with every conversation start payload so the server gates risky actions (`src/api/agent-server-adapter.ts:593-613,1119-1172`).
- **Workspace boundary (local)**: child agents get their own git worktree by default; sharing the parent directory requires an explicit `isolation="shared"` choice (`src/api/launch-child-conversation-client-tool.ts:46-52`).
- **Renderer boundary (Electron)**: `nodeIntegration: false` + `contextIsolation: true` on all windows; external URLs forced to the system browser (`electron/main.mjs:306-311,346-349,362-401`).
- **What is *not* a boundary locally**: there is no OS-level confinement of the agent-server in npm/Electron mode. The docs say so plainly — the agent can read/write the machine's filesystem, run shell commands, and reach the network; the compensating controls are firewalls and key hygiene (`docs/SELF_HOSTING.md:8-10,50-58`).

### 3. Are boundaries enforced?

Partially, and mostly at the right layer:

- **Server-side (strongest)**: session keys validated by the agent-server on `/api/*`; the confirmation/security policy objects are interpreted by the SDK, not the UI (`src/api/agent-server-adapter.ts:593-613` is payload construction; enforcement is delegated upstream).
- **Client-state machine (medium)**: sandbox status gates prevent doomed requests and stale-host WebSocket connections (`src/hooks/query/use-bash-command-logs.ts:134-157`; `src/contexts/websocket-provider-wrapper.tsx:25-31`) — these enforce correctness around the boundary, not the boundary itself.
- **Build-time discipline (weak but real)**: `src/api/no-direct-agent-server-calls.test.ts:30-60` fails CI on raw HTTP calls, keeping all cross-boundary traffic on audited typed-client paths.
- **Acknowledged gaps**: the bundled editor's connection token is derived from the session API key (tracked upstream as software-agent-sdk#4317), mitigated by omitting the editor route on the unauthenticated origin and suppressing Referer (`docker/entrypoint.sh:419-433`); path-prefix routing shares origin/localStorage between canvas and editor (`config/defaults.json:34-38`); inside Docker, openvscode-server binds `0.0.0.0` so `--network host` exposes it with only the token in front (`docker/entrypoint.sh:20-25`).

### 4. Can sandbox configuration be changed per-run?

Yes, within what the backend accepts:

- **Per conversation**: `workspace.working_dir` (env-overridable base + conversation-specific subdir, `src/api/agent-server-config.ts:196-205`), `worktree: boolean` on the start request (`src/api/agent-server-adapter.ts:1125`), `confirmation_policy` / `security_analyzer` from per-conversation settings, and tool surface gating (`browser_tool_set`, `task_tool_set`) at `src/api/agent-server-adapter.ts:627-640`.
- **Per delegation**: each child launch picks `target` (local vs cloud sandbox) and `isolation` (worktree/shared) independently (`src/services/child-conversation-launch.ts:186-192`).
- **Lifecycle operations**: pause/resume/delete per sandbox id (`src/api/cloud/conversation-service.api.ts:192-254`).
- **Not configurable from here**: the cloud `sandbox_spec_id` is read-only in `V1SandboxInfo` (`src/api/cloud/sandbox-service.types.ts:16`) — spec selection happens cloud-side; no evidence was found of a frontend API to choose a sandbox image/spec.

## Architectural Decisions

1. **Boundary consumption over boundary implementation.** The repo deliberately owns only the *addressing* of sandboxes; isolation mechanics live in `software-agent-sdk` (`docs/architecture.md:17`; AGENTS.md repo map). This keeps a clean seam but means no sandbox guarantee can be verified from this codebase alone.
2. **One service class per cross-boundary concern, dual-mode internally.** Bash events, runtime commands, git, files, and VSCode each have a service that branches on `backend.kind === "cloud"` and either uses typed SDK clients or `callCloudProxy` with `hostOverride` (`src/api/bash-service/bash-service.api.ts:82-118`; `src/api/runtime-service/agent-server-runtime-service.ts:32-93`). The pattern is uniform and grep-able.
3. **Secure-by-default local credentials.** Keys are generated from `/dev/urandom`, persisted `chmod 600`, and reused across restarts; absence of a key blocks startup rather than degrading to open access (`docker/entrypoint.sh:175-218`).
4. **Treat sandbox status as part of the UI contract.** The five-state `SandboxStatus` union flows through polling cadence, WebSocket suppression, bash-log preflight, and archive detection (`src/utils/conversation-archive-status.ts:4-6`), making transient sandbox states first-class UX rather than raw errors.
5. **Route-table security reasoning in the entrypoint.** The public-mode instance omits the editor route *because* the route would leak a secret-bearing URL into history/Referer on the unauthenticated origin — a documented, test-extracted decision (`docker/entrypoint.sh:411-433`; markers consumed by `__tests__/scripts/docker-vscode-route-sync.test.ts:87-88`).

## Notable Patterns

- **Preflight-then-classify error handling**: check `sandbox_status` before firing; if fired anyway, map 404/5xx/network failures onto the same `SandboxIssue` vocabulary (`src/hooks/query/use-bash-command-logs.ts:110-115,182-186`).
- **Corrective-guidance tool contracts**: invalid child-launch parameters (e.g., `repository` with `target="local"`) return instructive errors instead of throwing, teaching the model the boundary rules (`src/services/child-conversation-launch.ts:143-158`).
- **Origin/capability intersection**: editor availability is computed as server capability ∩ origin route table, preventing "an editor this page has no route to" (`src/utils/vscode-origin.ts:1-22`).
- **Cookie-scoped embedding**: exchanging header-auth for a path-scoped cookie solely to enable iframe/img embedding of sandbox artifacts, with the cloud path explicitly excluded because cookies cannot traverse the proxy hop (`src/hooks/query/use-workspace-session.ts:21-34`).
- **System-prompt topology injection**: the `<RUNTIME_SERVICES>` suffix teaches the *agent* where its own sandbox edges are (which localhost ports are itself vs. other services), reducing boundary-blind probing (`src/api/agent-server-adapter.ts:215-297`).

## Tradeoffs

- **Local speed vs. isolation**: npm/Electron mode gives zero-setup, fast startups but the agent is unconfined; Docker mode confines but trades setup complexity and (per AGENTS.md) volume-mount ergonomics. The repo ships both and documents the risk instead of forcing one.
- **Proxy hop vs. directness**: routing cloud runtime calls through the local agent-server solves CORS and centralizes auth, at the cost of an extra hop and a hard dependency on the local server being up (`src/api/cloud/proxy.ts:18-39`).
- **Prefix routing vs. origin separation**: serving VSCode under `/vscode` on one port simplifies deployment but shares origin/localStorage with the app — accepted and documented (`config/defaults.json:34-38`).
- **Worktree default vs. shared fallback**: defaulting children to worktrees maximizes isolation, but the silent-ish downgrade to `shared` on non-git workspaces trades isolation for availability (surfaced via `isolation_note`, `src/services/child-conversation-launch.ts:300-310`).
- **Status gating vs. freshness**: fast 3-second polling while paused/booting improves responsiveness but triples request rate during those windows (`src/hooks/query/use-active-conversation.ts:17-32`).

## Failure Modes / Edge Cases

- **Stale sandbox host after pause**: `conversation_url` intentionally survives pause; without WS suppression and fast-poll, sockets would connect to a dead host (`src/contexts/websocket-provider-wrapper.tsx:25-31`).
- **Duplicate resume triggers**: guarded per conversation id by ref, but a failed resume only toasts; retry requires navigation remount (`src/routes/conversation.tsx:155-172`).
- **Automation auth fallback to cloud**: if `AUTOMATION_AGENT_SERVER_URL` is unset, the automation backend validates keys against the cloud API and local keys fail 401 (`docker/entrypoint.sh:261-267`) — a misconfiguration that looks like an auth failure.
- **Editor token leakage vectors**: history/Referer on unauthenticated origins mitigated by route omission + `--no-referrer-prefix`; root fix deferred upstream (#4317) (`docker/entrypoint.sh:419-433`).
- **`--network host` exposure**: openvscode-server binds all interfaces inside the container; host networking publishes it beyond the intended ingress path (`docker/entrypoint.sh:20-25`).
- **Unreachable-vs-error conflation**: 5xx from the proxy is mapped to "unreachable," which correctly covers gone-sandboxes but also masks genuine upstream crashes as sandbox issues (`src/hooks/query/use-bash-command-logs.ts:74-78`).
- **Can an agent escape its intended boundary?** From this repo's evidence: in cloud mode, the agent is confined to its provisioned sandbox (boundary owned elsewhere); in local/npm mode there is no boundary to escape — the agent already holds user privileges by design (`docs/SELF_HOSTING.md:8-10`); in Docker mode the container is the boundary, with the noted editor-port caveats.

## Future Considerations

- Give the editor a credential independent of the session API key (upstream #4317) so the public-mode route omission and Referer suppression become unnecessary.
- Expose sandbox spec selection (image/tools) at conversation start so the GUI can request stronger or different isolation per run, instead of reading `sandbox_spec_id` passively (`src/api/cloud/sandbox-service.types.ts:16`).
- Consider an OS-level confinement option for local mode (seccomp/apparmor profile shipped alongside the uvx launcher) to back the SELF_HOSTING guidance with mechanism.
- Add telemetry/alerting for the automation-backend-cloud-auth-fallback misconfiguration detected at startup.
- Extend the preflight `SandboxIssue` classification to distinguish proxy-upstream-crash (true 5xx) from sandbox-gone (404) for cleaner diagnostics.

## Questions / Gaps

- **Actual container isolation internals**: how the cloud sandbox and `ghcr.io/agent-server` isolate processes/network is defined in sibling repos and outside this study's boundary; no evidence available here beyond the FROM reference (`docker/Dockerfile:73`).
- **Confirmation-policy enforcement point**: this repo constructs `confirmation_policy`/`security_analyzer` payloads (`src/api/agent-server-adapter.ts:593-613`) but their runtime interpretation could not be verified from this source.
- **No search performed for VM/microVM options**: nothing in this tree suggests gVisor/Firecracker-style sandboxes; searches were limited to "sandbox", "container", "isolation", "runtime", "prod-runtime", "hostOverride", "cloud-proxy" within the selected source.
- **Windows local-mode boundaries**: `README.windows.md` exists, but Windows-specific confinement (if any) was not examined in depth.

---

Generated by `Dimension 17.01: Sandbox Boundary` against `openhands`.
