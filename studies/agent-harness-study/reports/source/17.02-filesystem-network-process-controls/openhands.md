# Source Analysis: openhands

## Dimension 17.02 — Filesystem, Network, and Process Controls

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands (agent-canvas) |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React frontend + Node.js launchers, Docker/Helm packaging for a Python agent-server stack |
| Analyzed | 2026-08-24 |

## Summary

This source is the OpenHands **frontend** ("agent-canvas": React/TS UI, Node launcher scripts, Docker image, Helm chart). Per the repository's own map (`AGENTS.md`, "Repository Map" section), the sandbox/runtime enforcement itself (agent tool execution, workspace isolation, network egress of the agent) lives in the sibling `software-agent-sdk` repo, which is outside this study's isolation boundary. Within this source, control over filesystem/network/process surfaces is expressed in three layers:

1. **Packaging layer** — the all-in-one Docker image declares exactly two persistence volumes (`/home/openhands/.openhands` state, `/projects` user code), runs as non-root `openhands` (`docker/Dockerfile:151-164`); the Helm chart pins UID/GID 10001 with `runAsNonRoot: true`, mounts one PVC via `subPath` at two well-known trees, applies CPU/memory limits, and gates cluster access behind an off-by-default RBAC switch (`helm/agent-canvas/values.yaml:115-120,169-192`; `helm/agent-canvas/templates/rbac.yaml:16-51`).
2. **Ingress layer** — a Node reverse proxy (`scripts/static-server.mjs`) routes by an explicit prefix allowlist to backends, refuses to advertise an editor route it does not serve, enforces mutually exclusive auth modes (auto-injected session key vs. required key entry), and strips credential-bearing Referer on the editor path (`scripts/static-server.mjs:181-206,629-635`; `docker/entrypoint.sh:196-284`).
3. **Client API layer** — uploads are sanitized against path traversal and anchored to the agent-server home directory (`src/api/workspace-upload-path.ts:15-56`, tested in `__tests__/api/workspace-upload-path.test.ts:94-110`), file sizes are capped client-side at 3 MB (`src/utils/file-validation.ts:1-2`), and shell execution is a thin passthrough to the runtime's `/api/bash/execute_bash_command` with a default 30 s timeout (`src/api/runtime-service/agent-server-runtime-service.ts:25-56`).

The dominant architectural stance is **delegation**: this repo constrains its own container, ingress, and browser-side calls, but imposes no filesystem jail, no network egress policy, and no command filtering on the agent itself — those are owned upstream. Consequences: there is no Kubernetes `NetworkPolicy` anywhere in the chart, the container `securityContext` hardening block (capability drop, read-only rootfs) is present but commented out (`helm/agent-canvas/values.yaml:195-199`), the network-capable browser tool ships enabled by default (`src/api/agent-server-adapter.ts:114-118,655`), and the entrypoint documents a known editor exposure under `--network host` that is deferred to an upstream SDK issue (`docker/entrypoint.sh:20-25`). Cleanup is solid at process level (signal traps, process-tree SIGTERM→SIGKILL escalation) but absent at workspace-data level (automation run workspaces are created but never reaped in this repo).

## Rating

**Score: 5 / 10**

Rationale against the rubric:

- Controls are *present* (non-root container, pinned UIDs, resource limits, auto-generated session keys, traversal-tested upload paths, explicit ingress allowlist, referrer hygiene) which keeps it out of the 1–3 band.
- They are *inconsistent and partially delegated*: no NetworkPolicy, commented-out container hardening, no pids/cgroup tuning beyond K8s resources, no agent command filtering or egress restriction in-repo, browser tool enabled by default, a documented-but-unfixed editor token exposure (`docker/entrypoint.sh:20-25`, tracked as software-agent-sdk#4317), and no automation-workspace reaping. That matches the "present but inconsistent, weakly documented, or fragile" 4–6 band; the strong ingress guards and tested upload sanitization justify the upper half.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Filesystem volumes (Docker) | Image pre-creates `/home/openhands/.openhands/{conversations,bash_events,automation}` and `/projects`, chowns them, then declares `VOLUME ["/home/openhands/.openhands", "/projects"]`; comment states `/projects` is "user code the agent can read/edit" | docker/Dockerfile:151-164 |
| Non-root container user | `USER openhands` after ownership setup | docker/Dockerfile:157 |
| Helm filesystem mounts | Single PVC shared via `subPath`, mounted at `/home/openhands/.openhands` and `/home/openhands/workspace` (the agent's default working dir) | helm/agent-canvas/values.yaml:115-120 |
| Pod security context | `fsGroup: 10001`, `runAsNonRoot: true`, `runAsUser/runAsGroup: 10001` | helm/agent-canvas/values.yaml:169-178 |
| Container hardening gap | `securityContext: {}` with capability-drop/readOnlyRootFilesystem suggestions left commented out | helm/agent-canvas/values.yaml:180-186 |
| Upload path traversal guard | `getSafeUploadFileName` splits on `[\\/]+`, takes last segment, rejects `.`/`..`; regression test "rejects file names that escape the destination via path traversal" | src/api/workspace-upload-path.ts:15-24; __tests__/api/workspace-upload-path.test.ts:94-110 |
| Upload anchoring (WUP-001) | Relative working dirs anchored to agent-server home via `/api/file/home` instead of resolving to filesystem root | src/api/workspace-upload-path.ts:27-56; src/api/conversation-file-upload.api.ts:117-124 |
| Client-side upload size caps | `MAX_FILE_SIZE = MAX_TOTAL_SIZE = 3 MB`, enforced before upload | src/utils/file-validation.ts:1-2 |
| Upload concurrency bound | Batches of `FILE_UPLOAD_CONCURRENCY = 5` per-runtime uploads | src/api/conversation-file-upload.api.ts:13,159-161 |
| Arbitrary file download passthrough | `downloadFile(path)` → `GET /api/file/download?path=<raw>` (cloud) / `FileClient.downloadFile(path)` (local); no client-side path allowlist | src/api/runtime-service/agent-server-runtime-service.ts:70-96 |
| Ingress route allowlist | Proxy serves only registered prefixes (`/api/automation`, `/api`, `/server_info`, `/sockets`, health/docs, vscode route); unmatched → static SPA; `--reject-prefix` returns 503 | scripts/static-server.mjs:111-124,152-160,257-263 |
| Route-advertisement guard | Server exits if `--vscode-base-path` has no matching `--route` ("advertising an editor it does not serve") | scripts/static-server.mjs:194-206 |
| Auth mode exclusivity | `--session-api-key` (auto-inject) and `--auth-required` (user must paste) are mutually exclusive; startup fails otherwise | scripts/static-server.mjs:181-192 |
| Default-on credentials | Entrypoint generates and persists 32-byte random `OH_SECRET_KEY` and session API key (`chmod 600`) when unset | docker/entrypoint.sh:178-208 |
| Referrer credential hygiene | `Referrer-Policy: no-referrer` on the vscode prefix because the editor URL carries `<origin>/<prefix>/?tkn=<session key>` | scripts/static-server.mjs:629-635; docker/entrypoint.sh:415-447 |
| Known network exposure | Comment: `openvscode-server binds 0.0.0.0`, so `docker run --network host` leaves the editor directly reachable with only its connection token; token scope tracked in OpenHands/software-agent-sdk#4317 | docker/entrypoint.sh:20-25,415-447 |
| No NetworkPolicy | Chart templates contain only ingress/rbac/service/serviceaccount/statefulset — no NetworkPolicy object exists | helm/agent-canvas/templates/ (directory listing) |
| Browser tool default-enabled | `BROWSER_TOOL_SET_NAME = "browser_tool_set"` included unless `VITE_ENABLE_BROWSER_TOOLS === "false"` | src/api/agent-server-adapter.ts:114-118,653-659 |
| Cloud traffic funneling | All cloud/runtime calls must go through `callCloudProxy` server-side hop; CI guard test bans raw axios/fetch to agent-server | AGENTS.md "API Access Rules"; src/api/no-direct-agent-server-calls.test.ts |
| Shell execution passthrough | `executeCommand(command, cwd?, timeout=30)` → `POST /api/bash/execute_bash_command` (cloud proxy timeout = `timeout + 10` s); no command inspection/filtering | src/api/runtime-service/agent-server-runtime-service.ts:25-56 |
| Bash output pagination cap | `MAX_OUTPUT_PAGES = 20` safety cap when reading bash output events | src/api/bash-service/bash-service.api.ts:23 |
| Terminal UI is display-only | Terminal tab renders stored bash commands/events from `useCommandStore`; no local PTY | src/components/features/terminal/terminal.tsx:12-15 |
| Process resource limits | Helm requests cpu 500m / memory 1Gi; limits cpu "2" / memory 4Gi | helm/agent-canvas/values.yaml:186-193 |
| Init/zombie reaping | Docker entrypoint wrapped in `tini --` | docker/Dockerfile:170 |
| Container process supervision | `cleanup()` trap kills all tracked PIDs on EXIT/SIGINT/SIGTERM; supervisor loop exits container if ingress dies | docker/entrypoint.sh:276-284,462-473 |
| Dev-stack process-tree teardown | `shutdown()` escalates SIGTERM→SIGKILL after 3 s using `signalProcessTree` on detached process groups; SIGINT/SIGTERM/SIGHUP wired | scripts/dev-with-automation.mjs:1063-1098 |
| Electron PATH bridging | Bundled Node/uv injected into `PATH` so backend scripts and stdio MCP servers can spawn; `spawnSync("uvx", ["--version"])` availability probe | electron/main.mjs:132-135,137-198 |
| Cluster access gate | RBAC master switch `enabled: false`; optional namespace-scoped `admin` RoleBindings and opt-in only `cluster-admin` ClusterRoleBinding | helm/agent-canvas/values.yaml:47-62; helm/agent-canvas/templates/rbac.yaml:16-51 |
| Automation run limits (client side) | Run timeout default 600 s, validated positive-integer and against server-reported ceiling before request | src/utils/automation-timeout.ts:5-33 |
| Automation run cancel | `POST /api/automation/v1/runs/{run_id}/cancel` exposed to the UI | src/api/automation-service/automation-service.api.ts:447-451 |
| Automation workspace creation, no reaping | `AUTOMATION_WORKSPACE_BASE` created at startup where run tarballs unpack; no cleanup job found in this repo | docker/entrypoint.sh:53,317-319 |
| Conversation stop semantics | Cloud stop pauses the sandbox (`pauseCloudSandbox`); local stop interrupts immediately via `/interrupt` | src/hooks/mutation/conversation-mutation-utils.ts:41-63 |
| Dead-sandbox handling | `sandbox_status` MISSING/ERROR treated as archived/read-only | src/utils/conversation-archive-status.ts:3-7 |

## Answers to Dimension Questions

### 1. What filesystem access does an agent have?

Within this source's boundary, the intended answer is defined by packaging: the agent's writable world is `/projects` ("user code the agent can read/edit", `docker/Dockerfile:161`) plus its own `$HOME/.openhands` state tree (`docker/Dockerfile:151-155`), running as unprivileged `openhands` (UID 10001, `helm/agent-canvas/values.yaml:176-177`). The Helm chart persists exactly `.openhands` and `workspace` subdirectories of `$HOME` on one PVC (`values.yaml:115-119`). The frontend reinforces this on the upload path: filenames are flattened and traversal-checked (`src/api/workspace-upload-path.ts:15-24`), and relative working dirs are anchored to the agent-server home rather than `/` (`src/api/conversation-file-upload.api.ts:117-124`). However, there is no mount-level jail, no `readOnlyRootFilesystem`, and no client-visible allowlist for *reads* — `downloadFile` passes an arbitrary `path` parameter through to the runtime (`src/api/runtime-service/agent-server-runtime-service.ts:70-96`). Read confinement inside the sandbox is enforced (or not) by the agent-server, outside this repo. **Implemented here:** volume shape, non-root identity, upload-write hygiene. **Delegated:** read scope and any chroot/mount namespacing.

### 2. What network access does an agent have?

For the *browser/UI*: tightly channeled — every agent-server call must use typed clients (CI-enforced, `src/api/no-direct-agent-server-calls.test.ts`) and every cloud call goes through the server-side `callCloudProxy` hop; the ingress proxy only forwards a fixed prefix table (`scripts/static-server.mjs:607-671`). For the *agent itself*: **this source contains no egress policy mechanism whatsoever.** There is no Kubernetes `NetworkPolicy` template in the chart, no firewall/proxy env passed to the runtime, and the network-capable `browser_tool_set` is attached to new conversations unless explicitly disabled by env var (`src/api/agent-server-adapter.ts:114-118,653-659`). The dev stack even opens CORS to localhost origins for automation (`scripts/dev-with-automation.mjs:1053-1058`). Two documented exposures corroborate the loose posture: the vscode editor binds `0.0.0.0` and stays reachable under `--network host` with only a query-string token (`docker/entrypoint.sh:20-25`), mitigated only by `Referrer-Policy: no-referrer` on that prefix (`scripts/static-server.mjs:629-635`). **Answer to the dimension's headline question — "Can an agent download arbitrary files from the internet?"**: nothing in this repository prevents it. Enforcement would have to come from the upstream runtime/cloud layer (out of scope here); within this source the default trajectory includes a full browser tool, so the evidence says arbitrary downloads are permitted by default.

### 3. Can the agent spawn arbitrary processes?

Yes, effectively unrestricted from what this repo shows. The shell tool is a raw passthrough: `executeCommand` sends whatever string the caller supplies to `/api/bash/execute_bash_command` with only a wall-clock timeout (`src/api/runtime-service/agent-server-runtime-service.ts:25-56`); the frontend performs zero command parsing, filtering, or allowlisting. Bash history/output are persisted under `~/.openhands/.../bash_events` (`docker/Dockerfile:152`, `docker/entrypoint.sh:224-226`) and paginated with a 20-page safety cap in the UI reader (`src/api/bash-service/bash-service.api.ts:23`). Indirect spawning is also enabled: the Electron build deliberately puts real `node/npm/npx` and `uv` binaries on PATH so the agent-server can spawn stdio MCP servers (`electron/main.mjs:144-198`), and the Docker image pre-installs the `github-mcp-server` binary so MCP entries can exec it (AGENTS.md, MCP catalog section). Resource-level containment is limited to pod CPU/memory limits (`values.yaml:186-193`); no pids limit, ulimit, or cgroup tuning appears anywhere in the packaging. OS-process confinement (user namespaces, seccomp, etc.) is again upstream territory.

### 4. Are resources cleaned up after execution?

**Processes: yes, thoroughly.** Three independent cleanup mechanisms exist: (a) the Docker entrypoint's `cleanup()` trap kills all child PIDs on EXIT/SIGINT/SIGTERM, with a supervisor loop that terminates the container if the ingress dies (`docker/entrypoint.sh:276-284,462-473`); (b) the dev launcher escalates SIGTERM→SIGKILL across detached process groups after 3 s and runs shutdown hooks (`scripts/dev-with-automation.mjs:1063-1098`); (c) `tini` reaps zombies as PID 1 (`docker/Dockerfile:170`). Conversations stop cleanly: cloud stops pause the sandbox and wait for the in-flight LLM call; local stops interrupt immediately (`src/hooks/mutation/conversation-mutation-utils.ts:41-63`), and dead sandboxes degrade to a read-only archived state rather than dangling resources (`src/utils/conversation-archive-status.ts:3-7`).

**Data/workspaces: partially.** Automation runs get a fresh unpack directory under `AUTOMATION_WORKSPACE_BASE` created at boot (`docker/entrypoint.sh:317-319`), runs are cancellable (`src/api/automation-service/automation-service.api.ts:447-451`) and time-bounded client-side (600 s default, ceiling validated against server capability, `src/utils/automation-timeout.ts:5-33`), but **no reaping/GC of run workspaces or conversation artifacts exists in this repo** — retention is bounded only by PVC capacity (20 Gi default, `values.yaml:120`). No evidence of TTL-based cleanup was found; searches covered `docker/`, `helm/`, `src/api/automation-service/`, and launcher scripts.

## Architectural Decisions

1. **Delegate sandbox enforcement upstream, ship guardrails locally.** The repo's own map assigns agent/tool/sandbox behavior to `software-agent-sdk` (AGENTS.md, Repository Map table). This repo therefore invests in what it owns: packaging identity (non-root, pinned UIDs), ingress routing, credentials, and browser-side call discipline.
2. **Explicit allowlist ingress instead of implicit trust.** The proxy serves only registered prefixes, rejects misconfigured editor advertisement at startup, and returns 503 for rejected prefixes (`scripts/static-server.mjs:194-206,257-263,607-671`) — fail-fast configuration validation rather than silent fallback.
3. **Secure-by-default credentials with rotation support.** Both `OH_SECRET_KEY` and the session API key are generated from `/dev/urandom` and persisted `chmod 600` if absent (`docker/entrypoint.sh:178-208`); public mode exists specifically to avoid baking keys into served HTML, and the two modes are structurally exclusive (`scripts/static-server.mjs:181-192`).
4. **Single-writer path resolution for uploads.** All upload destinations funnel through `buildWorkspaceUploadPath` → `resolveAbsoluteWorkspacePath` so relative dirs anchor to the agent-server home once, cached per backend (`src/api/workspace-upload-path.ts:27-56`), fixing the WUP-001 class of writes landing on read-only mounts.
5. **Process-tree-aware lifecycle management.** Detached process groups plus `signalProcessTree` escalation in dev (`scripts/dev-with-automation.mjs:1080-1097`) and PID-array traps plus `tini` in Docker (`docker/entrypoint.sh:276-284`; `docker/Dockerfile:170`) show deliberate treatment of child-process lifecycles.

## Notable Patterns

- **Fail-fast config guards with extracted test seams.** The vscode route normalization/collision logic in the entrypoint is marked for extraction into `__tests__/scripts/docker-vscode-route-sync.test.ts` (`docker/entrypoint.sh:69-141`), and the static server refuses to start on mutually exclusive flags (`static-server.mjs:185-192`) — shell config treated as tested code.
- **Credential-in-URL mitigation at the proxy layer.** Rather than fixing the editor token upstream, the proxy adds `Referrer-Policy: no-referrer` on the editor prefix and deliberately omits the editor route from the public-mode instance (`scripts/static-server.mjs:629-635`; `docker/entrypoint.sh:415-447`).
- **CI-enforced API discipline.** A guard test (`src/api/no-direct-agent-server-calls.test.ts`) makes the "all runtime calls go through typed clients / cloud proxy" rule mechanical, which is also how the network surface of the *UI* stays enumerable.
- **Client-side defense-in-depth on uploads.** Size caps (3 MB, `src/utils/file-validation.ts:1-2`), name flattening/traversal rejection (`workspace-upload-path.ts:15-24`), bounded concurrency (5, `conversation-file-upload.api.ts:13`), and per-file skip-instead-of-fail error aggregation.
- **Read-only degradation over deletion.** Sandboxes that vanish (`MISSING`/`ERROR`) become archived, read-only conversations instead of being garbage-collected (`src/utils/conversation-archive-status.ts:3-7`; `src/mocks/conversation-handlers.ts:69-88`).

## Tradeoffs

- **Delegation vs. completeness:** clean ownership boundaries mean every hard guarantee about the agent's actual filesystem/network/process power lives in another repo; studying or auditing this source alone cannot verify sandbox safety, only its packaging posture.
- **Usability vs. lockdown:** browser tool enabled by default (`agent-server-adapter.ts:114-118`) and full-shell passthrough maximize capability; the cost is that the default deployment has no agent egress restrictions at this layer.
- **Persistence vs. hygiene:** single PVC with `subPath` mounts simplifies state (`values.yaml:110-120`) but guarantees accumulation — no retention policy compensates.
- **Token-in-URL pragmatism:** shipping `Referrer-Policy` mitigation while deferring the token fix to software-agent-sdk#4317 (`docker/entrypoint.sh:443-447`) trades a known leak vector for velocity, consciously documented.
- **Commented-out hardening:** leaving `capabilities.drop: [ALL]` / `allowPrivilegeEscalation` as suggestions (`values.yaml:180-186`) keeps flexibility for local/dev runs but means production charts deploy without them unless operators know to enable.

## Failure Modes / Edge Cases

- **`--network host` deployments expose the editor directly** on `VSCODE_PORT` with only the connection token in front of it (`docker/entrypoint.sh:20-25`); combined with the token being derived from `session_api_keys[0]`, the same secret authenticates `/api`.
- **Public-mode proxy routes bypass the auth flag**: `--auth-required` only controls HTML injection, not proxied paths (`docker/entrypoint.sh:436-441`); safety relies entirely on agent-server enforcing the session key, and the editor route is excluded precisely because its token would land in browser-navigable URLs.
- **Route-table collision hazard**: static-server registers the editor route last, so a colliding prefix silently replaces an earlier route (e.g., `OH_VSCODE_BASE_PATH=/api` would hijack every API call); guarded by normalization + reserved-name checks in the entrypoint (`docker/entrypoint.sh:96-140`) and a startup guard in the server (`static-server.mjs:194-206`).
- **Backend crashes are tolerated, not restarted**: the entrypoint loop keeps the container alive while the proxy 502s downed routes (`docker/entrypoint.sh:456-473`) — availability degrades without signals beyond probe failures.
- **Automation workspaces accumulate**: repeated runs unpack tarballs under `AUTOMATION_WORKSPACE_BASE` with no in-repo GC; disk exhaustion is only bounded by the 20 Gi PVC default.
- **Stale conversation URLs after pause**: the UI must suppress the cached `conversation_url` while `sandbox_status === "PAUSED"` or the WebSocket hammers a dead host (documented resume-gating contract, AGENTS.md "Cloud conversation resume gating"; `src/api/cloud/conversation-service.api.ts:224-255`).
- **Uploads to still-provisioning clouds fail fast** with an explicit "sandbox is still starting" error rather than queueing (`conversation-file-upload.api.ts:64-72`).

## Future Considerations

- Add a `NetworkPolicy` template (default-deny egress with LLM-provider allowlist) and uncomment/enforce the container `securityContext` hardening in the chart — both are pure-packaging changes fully within this repo's ownership.
- Introduce retention/GC for automation run workspaces and bash event stores (TTL or size-based), mirroring the process-level cleanup rigor.
- Surface agent egress posture in `/server_info`/`runtime_services` so the UI can display whether downloads/browser tools are restricted, closing the observability gap between this repo and the upstream sandbox.
- Track the upstream editor-token fix (software-agent-sdk#4317) to retire the `Referrer-Policy` workaround and the public-mode route exclusion (`docker/entrypoint.sh:415-447`).
- Consider a pids limit (`pids_limit` / K8s `pids` resource) given the agent's unrestricted spawn ability via shell and stdio MCP servers.

## Questions / Gaps

- **Agent egress enforcement location:** the actual network policy applied to conversations (if any) is implemented in `software-agent-sdk`/OpenHands Cloud, outside this source. Searched `helm/`, `docker/`, `src/`, and launcher scripts for `NetworkPolicy`, `egress`, `allowlist/denylist` — no agent-facing mechanism found here. **No evidence found in this source.**
- **Sandbox filesystem isolation internals:** whether the runtime chroots/binds-mounts anything beyond the declared volumes cannot be verified from this repo; only the volume declarations and comments (`docker/Dockerfile:159-164`) describe intent.
- **Server-side upload/download path validation:** the runtime's `/api/file/upload`/`/api/file/download` handlers (path traversal checks beyond the client-side ones) live upstream; this repo only demonstrates client-side hygiene with tests (`__tests__/api/workspace-upload-path.test.ts:94-110`).
- **Production usage of the Helm chart:** the chart exists with probes/RBAC/persistence, but no evidence was found in-repo of it being exercised by CI or E2E (searched `.github/` references via AGENTS.md workflow notes; workflows cover Docker/npm/mock-LLM paths, not Helm deploys).

---

Generated by `Dimension 17.02: Filesystem, Network, and Process Controls` against `openhands`.
