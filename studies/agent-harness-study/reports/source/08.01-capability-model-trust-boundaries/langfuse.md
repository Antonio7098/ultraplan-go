# Source Analysis: langfuse

## Dimension 08.01: Capability Model and Trust Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js web app (UI + tRPC + public REST), BullMQ worker, shared package, AWS Lambda MicroVM / Docker sandbox runtime |
| Analyzed | 2026-08-24 |

## Summary

Langfuse is an LLM engineering platform, and its agent harness is the **in-app agent**: a project-scoped assistant inside the authenticated product UI whose runs execute durably in a worker while the browser observes a persisted event stream (`web/src/features/in-app-agent/README.md:3-5`). The capability model is unusually explicit for this class of system. Authority is split across four trust domains — browser (interaction state, intent), web server (authorization, IDs, sanitization, admission), worker (runtime config, MCP credentials, tools, execution), shared (durable contracts) — documented as ownership at `web/src/features/in-app-agent/README.md:17-21`.

The core design answers the dimension's guiding question — *can the model request power without possessing power?* — with three stacked gates on every Langfuse MCP tool:

1. **RBAC gate**: a tool is only exposed to the model if the driving user's `projectRole`/`isAdmin` covers the tool's required `ProjectScope`, computed by `hasProjectAccessByRole` in `packages/shared/src/in-app-agent/server/mcpPolicy.ts:451-470`. The assistant never sees tools its user could not use manually.
2. **Human approval gate**: every mutating tool carries an `"approval"` classification; non-approved tools are marked Mastra `requireApproval: true` via `withInAppAgentToolApproval` (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:551-563`), producing a durable interrupt that the user must decide in the browser.
3. **API-key gate (defense in depth)**: each run mints a temporary project-scoped API key flagged `isInAppAgentKey` (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:381-399`). At the MCP endpoint, such keys default to read-only and may call a mutating tool only when named in a server-generated allowlist derived from approvals plus stored conversation grants (`web/src/pages/api/public/mcp/index.ts:193-217`, enforced per-call at `web/src/features/mcp/server/registry.ts:170-189`). Even if worker-side policy were bypassed, the endpoint itself refuses unauthorized mutation.

Dangerous code-execution capability (`bash`, `read`, `write`, `edit`) is isolated rather than approved: sandbox tools are auto-approved locally (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:410-421`) but execute only inside a provider-isolated sandbox — a dev-only `dangerous-docker` container with networking disabled, or a production AWS Lambda MicroVM reached through expiring proxy auth tokens.

## Rating

**8 / 10** — Clear model with explicit interfaces and operational safeguards.

Rationale: exhaustive per-tool approval/scope policy map with type-level exhaustiveness enforcement (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:22-360`); defense-in-depth across policy, credential, and registry layers; claim-time revalidation of everything queued ("nothing from enqueue time is trusted", `worker/src/features/in-app-agent/executeInAppAgentRun.ts:196`); approval identity resolved server-side from persisted events so the client cannot tamper with tool name or arguments (`web/src/features/in-app-agent/server/backgroundRunService.ts:396-400`); and a dense authorization test suite (`web/src/__tests__/server/in-app-agent-api-route-auth.servertest.ts:36-330`). It falls short of 9-10 because the sandbox runtime HTTP surface has no application-layer authentication of its own (it trusts network placement), bash auto-approval leans entirely on provider isolation quality, the global-admin bypass is coarse, and production-grade isolation (Lambda MicroVM) is cloud/AWS-specific — self-hosted instances without it simply get no sandbox.

## Evidence Collected

Every entry cites repo-relative paths from the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Trust-domain ownership split (browser/web/shared/worker) | README states web owns authorization, worker owns MCP credentials/tools/execution | `web/src/features/in-app-agent/README.md:17-21` |
| Exhaustive tool policy map (~80 tools, `"auto"` vs `"approval"`, per-scope availability) | `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` with comment that new MCP tools must be classified before the agent can gate them | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360` |
| RBAC scope catalog | `projectScopes` const array of all `Resource:Action` scopes | `packages/shared/src/features/rbac/projectAccessRights.ts:5-86` |
| Role→scope matrix (OWNER/ADMIN/MEMBER/VIEWER/NONE) | `projectRoleAccessRights` record | `packages/shared/src/features/rbac/projectAccessRights.ts:91-272` |
| Pure role-based access check used by agent runtime | `hasProjectAccessByRole`; admin bypass at line 287 | `packages/shared/src/features/rbac/projectAccessRights.ts:282-289` |
| Tool availability filtered by caller role before exposure to model | `isInAppAgentLangfuseMcpToolAvailable` + `createInAppAgentToolPolicy` | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:451-510` |
| Approval marking on non-auto tools | `withInAppAgentToolApproval` sets `requireApproval: true` | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:551-563` |
| Sandbox tools + redirect tool auto-approval set | `IN_APP_AGENT_SANDBOX_TOOL_NAMES` = read/write/edit/bash; docs prefix `langfuseDocs_` auto-approved | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:410-421,565-581` |
| Redirect tool cannot navigate autonomously | Description: "Propose a user-confirmed navigation action … does not navigate automatically" | `worker/src/features/in-app-agent/runtime/tools.ts:448-453` |
| Ephemeral per-run MCP key minted atomically with run link | Transaction: `createAndAddApiKeysToDb({ isInAppAgentKey: true })` + run row update | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:381-399` |
| Key hashing at rest | bcrypt legacy hash + SHA-256 fast hash with SALT | `packages/shared/src/server/auth/apiKeys.ts:14-18,68-71` |
| Agent-key deletion refuses to touch normal project keys | Guard in `deleteApiKeyFromDb` warns and returns false unless `isInAppAgentKey === true` | `packages/shared/src/server/auth/apiKeys.ts:124-134` |
| Single-flight cleanup of run-scoped keys | `cleanupMcpApiKey` dedupes concurrent cleanup paths | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:169-193` |
| Stored grants revalidated against current role each run | Comment "Rebuild each run so grants invalidated by role changes drop out"; test proves MEMBER loses OWNER grant | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-339`; `packages/shared/src/in-app-agent/server/tools.test.ts:6-24` |
| MCP route auth: BasicAuth, project scope only, opt-in agent keys | `verifyAuthHeaderAndReturnScope(..., { allowInAppAgentKey: true })`; ForbiddenError for non-project scopes | `web/src/pages/api/public/mcp/index.ts:87-106` |
| In-app-agent context derivation from override header | `getInAppAgentContext`: default `{ permissions: "read" }`; malformed header degrades to read | `web/src/pages/api/public/mcp/index.ts:193-217` |
| Per-call enforcement at tool registry | `canCallTool`: read-mode requires `readOnlyHint === true`; allowlist mode requires name match; unknown mode denies | `web/src/features/mcp/server/registry.ts:170-189` |
| Override header constant + legacy singular-shape compat | `x-langfuse-in-app-agent-tool-override`; parser accepts both shapes for rolling deploys | `packages/shared/src/in-app-agent/constants.ts:45-48`; `packages/shared/src/in-app-agent/server/mcpPolicy.ts:386-404` |
| Worker sends credentials+override only to Langfuse MCP | Request headers include Authorization and override header on `langfuse` server config | `worker/src/features/in-app-agent/runtime/agent.ts:1022-1048` |
| Approval decision tamper-resistance | Client sends IDs + boolean only; tool name resolved from persisted interrupt event | `web/src/features/in-app-agent/server/backgroundRunService.ts:355-400`; comment at `web/src/features/in-app-agent/server/router.ts:262-266` |
| Standing grants scoped to conversation | `approvalScope: "conversation"` maps to `alwaysAllowToolName` persisted on conversation row | `web/src/features/in-app-agent/server/backgroundRunService.ts:397-400` |
| tRPC project-membership middleware | `enforceUserIsAuthedAndProjectMember` throws UNAUTHORIZED without membership; admin impersonation emits webhook + PostHog | `web/src/server/api/trpc.ts:297-401` |
| Watch SSE route authorization chain | Session → project membership → entitlement → org AI features → conversation ownership (`assertOwnedConversation`) | `web/src/features/in-app-agent/server/watchHandler.ts:20-67` |
| Entitlement gate for agent feature | `assertInAppAgentAvailable` checks instance flag, `in-app-agent` entitlement, org `aiFeaturesEnabled` | `web/src/features/in-app-agent/server/availability.ts:12-63` |
| Claim-time revalidation in worker | Instance flag, model config, conversation owner match, org AI features, non-null triggering user, fresh membership resolution | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:196-257` |
| Fencing via heartbeat | Heartbeat detects `fenced`/cancel requests and aborts the loop | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:401-424` |
| Dev-only docker provider guard | Throws unless `NODE_ENV === "development"` for `dangerous-docker` | `worker/src/features/in-app-agent/runtime/sandbox/config.ts:15-21,27-40` |
| Production MicroVM provider config | Requires image identifier, execution role ARN, region; optional egress network connector ARN | `worker/src/features/in-app-agent/runtime/sandbox/config.ts:42-64` |
| Docker network isolation | Container created with `NetworkDisabled: true` | `worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:77-85` |
| Sandbox path confinement | `resolveSandboxPath` throws "Sandbox path escapes workspace" outside `/workspace` | `packages/in-app-agent-sandbox-runtime/src/server.ts:473-487` |
| Bash execution + containment caveat | `spawn("sh", ["-lc", command])`; process-group SIGKILL with comment that descendants can escape and "a sandbox should have lifetime limits as the final containment boundary" | `packages/in-app-agent-sandbox-runtime/src/server.ts:318-333,391-432` |
| Sandbox request-size cap and serialized operations | 10 MB body limit; exclusive operation queue | `packages/in-app-agent-sandbox-runtime/src/server.ts:29,34-41,489-513` |
| MicroVM proxy auth tokens | `CreateMicrovmAuthTokenCommand`, 30-minute expiry, sent as `X-aws-proxy-auth` header | `worker/src/features/in-app-agent/runtime/sandbox/providers/lambdaMicrovm.ts:23-24,487-494,517-561` |
| MicroVM lifetime/idle policy | suspend after 60 s idle; terminate 4 h after suspension/creation; max duration 4 h | `worker/src/features/in-app-agent/runtime/sandbox/providers/lambdaMicrovm.ts:26-30,101-107` |
| MCP Host/Origin validation | `validateMcpRequestSecurity` allowlist from base URL + `LANGFUSE_MCP_ALLOWED_HOSTS`; wildcard opt-out | `web/src/features/mcp/server/security.ts:70-116` |
| User LLM secrets encrypted, separate from agent | `encrypt()` on secretKey/extraHeaders in LLM API key router; agent uses worker-configured Bedrock instead | `web/src/features/llm-api-key/server/router.ts:158-160,270-272`; `worker/src/features/in-app-agent/runtime/agent.ts:1013-1020` |
| Resource ceilings | Queue timeout 5 min, run max 15 min, approval TTL 24 h; per-user/per-org active-run limits | `packages/shared/src/in-app-agent/server/tunables.ts:8-17`; `web/src/features/in-app-agent/server/runCapacity.ts:79-92` |
| Rate limiting on MCP surface | `RateLimitService.rateLimitRequest(scope, "public-api")` | `web/src/pages/api/public/mcp/index.ts:124-132` |
| Authorization test coverage | Tests: rejects unauthenticated watch, non-member watcher, missing entitlement, disabled AI features, cross-project resolution, other-project-member access | `web/src/__tests__/server/in-app-agent-api-route-auth.servertest.ts:133-330` |
| API-key gating tests | In-app agent keys rejected when `allowInAppAgentKey` omitted/false; allowed when true | `web/src/__tests__/server/in-app-agent-api-route-auth.servertest.ts:77-101` |
| Override-header parsing tests | Non-agent keys get undefined; malformed override degrades to read-only; valid header yields allowlist | `web/src/__tests__/server/unit/mcp-route-in-app-agent-context.servertest.ts:22-63` |

## Answers to Dimension Questions

### 1. What can the agent do?

The agent's total capability surface is:

- **Langfuse MCP tools** (the product's own data plane): ~80 enumerated tools covering annotation queues, comments, datasets, evaluators/rules, experiments, media, metrics, models, observations, prompts, scores/configs, dashboards/widgets, health, feedback (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27-360`). Read-classified tools execute immediately; mutating ones pause for human approval.
- **Sandbox tools** when a sandbox provider is configured: `read`, `write`, `edit`, `bash` against a conversation-scoped workspace (`worker/src/features/in-app-agent/runtime/tools.ts:47-76`). These are auto-approved but confined to the sandbox (`mcpPolicy.ts:410-421`).
- **Documentation tools** from a public Langfuse docs MCP server, auto-approved by the `langfuseDocs_` prefix (`mcpPolicy.ts:569-574`, wired at `worker/src/features/in-app-agent/runtime/agent.ts:1040-1047`).
- **A navigation proposal tool** (`langfuse_proposeRedirect`) that returns typed hrefs for the UI to render as user-confirmed actions; it "does not navigate automatically" (`worker/src/features/in-app-agent/runtime/tools.ts:441-461`; name defined at `packages/shared/src/in-app-agent/constants.ts:4`).

Everything else is structurally absent: there is no arbitrary network tool, no host filesystem access, no shell on the worker itself, and no direct database access from the model.

### 2. What can the model only request but not directly do?

Any tool classified `"approval"` in `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` (e.g., `upsertDataset`, `createScore`, `deleteModel`, all dashboard mutations — spot examples at `packages/shared/src/in-app-agent/server/mcpPolicy.ts:80-83,212-223,284-287`). Mechanically, the model's request produces a Mastra interrupt (`requireApproval: true`, `mcpPolicy.ts:551-563`); execution happens only after a durable continuation is enqueued following the user's decision (`web/src/features/in-app-agent/server/backgroundRunService.ts:402-419`). Crucially, even after approval, the effective authority remains bounded: "approval can allow one execution of a tool the user already has access to, but it does not widen the user's project permissions" (`web/src/features/in-app-agent/README.md:231`). The same is true for standing grants: they are re-filtered against the current role on every run (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:335-339`).

### 3. Where is authority enforced?

Authority is enforced at four independent layers, any one of which is sufficient to stop an out-of-scope action:

1. **Exposure-time RBAC (web/worker policy layer)** — tools are filtered out of the model's toolset by role check before discovery (`mcpPolicy.ts:484-499`, applied at `agent.ts:1101-1104`).
2. **Runtime approval flow (durable, server-adjudicated)** — the approval decision resolves the granted tool from the persisted interrupt event, "never client input" (`backgroundRunService.ts:396-400`).
3. **Credential layer (MCP endpoint)** — the ephemeral `isInAppAgentKey` defaults to read-only; mutating calls require the server-minted allowlist header, parsed only for agent keys (`web/src/pages/api/public/mcp/index.ts:193-217`).
4. **Registry layer (per-call)** — `canCallTool` re-checks annotations/allowlist at invocation time and denies unknown permission modes (`registry.ts:170-189`).

Session-based authority sits upstream: tRPC procedures require an authenticated session and project membership (`web/src/server/api/trpc.ts:297-401`), and the worker revalidates conversation ownership and membership at claim time because "nothing from enqueue time is trusted" (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:196-257`).

### 4. Are dangerous capabilities isolated?

Yes, by construction and by naming:

- **Code execution** runs only inside the sandbox. The dev provider is literally named `dangerous-docker` and hard-fails outside development (`config.ts:15-21`); containers are created with `NetworkDisabled: true` (`docker.ts:83`). The production provider is an AWS Lambda MicroVM with idle/lifetime caps (60 s suspend, 4 h terminate; `lambdaMicrovm.ts:26-30,101-107`), expiring proxy-auth tokens (`lambdaMicrovm.ts:517-561`), and optional egress network connector control (`config.ts:47-48`).
- **Filesystem writes** are path-confined under `/workspace` with traversal rejection (`packages/in-app-agent-sandbox-runtime/src/server.ts:473-487`) — though note this confinement lives inside the sandbox image, not around it; `bash` could write anywhere the sandbox OS permits, which is why provider isolation is the real boundary.
- **Product mutations** are isolated behind the dual RBAC+approval gates described above.
- **Credential blast radius** is minimized: per-run keys are deleted at terminal state, and deletion refuses non-agent keys (`apiKeys.ts:124-134`; cleanup single-flight at `executeInAppAgentRun.ts:169-184`).

## Architectural Decisions

1. **Capability tables as source of truth.** A single exhaustive map pairs every Langfuse MCP tool with an approval classification and a required RBAC scope (`mcpPolicy.ts:27-360`). Exhaustiveness against the MCP registry is enforced by type and runtime assertions in tests, so adding a tool forces an explicit security classification (`mcpPolicy.ts:22-26`).
2. **Defense in depth via a purpose-built credential type.** Rather than trusting the worker's in-process policy, the system mints an ephemeral, DB-flagged API key per run and has the MCP endpoint independently interpret it (`executeInAppAgentRun.ts:381-399`; `mcp/index.ts:87-106,193-217`). Policy bugs alone cannot grant mutation rights.
3. **Durable, server-adjudicated human-in-the-loop.** Approvals are persisted events, not browser promises; the continuation reads the granted tool from history, and the client submits only identifiers plus a boolean (`backgroundRunService.ts:355-421`). This makes approval survive reloads and resist client tampering.
4. **Isolation-by-provider for code execution, approval-free.** Sandbox tools skip human approval (`mcpPolicy.ts:418-421`); safety comes from the provider contract (network-disabled Docker in dev, capped/authenticated MicroVMs in prod). This keeps interactive file/bash work low-friction at the cost of concentrating trust in the sandbox boundary.
5. **Revalidation over trust at every hop.** Enqueued payloads are treated as hints; roles, ownership, org flags, and entitlements are re-checked at claim time (`executeInAppAgentRun.ts:196-257`), and stored grants are rebuilt against current roles per run (`executeInAppAgentRun.ts:335-339`).

## Notable Patterns

- **Fail-closed defaults everywhere.** Missing user access disables a tool (`mcpPolicy.ts:455-457`); missing override header means read-only (`mcp/index.ts:203-205`); malformed override header degrades to read-only rather than erroring open (`mcp/index.ts:207-216`); unknown in-app-agent permission modes deny at the registry (`registry.ts:188`); a missing triggering user aborts the run — "a missing user must never implicitly mean 'trusted system'" (`executeInAppAgentRun.ts:241-245`).
- **Grant lifecycle hygiene.** One-off overrides are dropped after their call while standing grants persist in policy (`agent.ts:634`); deletion helpers refuse to destroy non-agent keys (`apiKeys.ts:124-134`).
- **Rolling-deploy compatibility in the security path itself.** The override schema and writer accept both the legacy singular `toolName` and the plural `toolNames` shape so already-enqueued continuations stay executable during rollout (`mcpPolicy.ts:386-404`).
- **Observability woven into authority.** Admin cross-project access fires a webhook and analytics event (`trpc.ts:338-347`); sandbox lifecycle transitions emit counters (`lambdaMicrovm.ts:137-139`); audit-log tests exist for agent actions (`web/src/__tests__/server/audit-log-in-app-agent.servertest.ts`).
- **Prompt-level reinforcement of enforced boundaries.** The system prompt tells the model that confirmations happen and forbids claiming success without a confirming result (`packages/shared/src/in-app-agent/server/systemPrompt.ts:52-58`) — presentation mirroring enforcement, not replacing it.

## Tradeoffs

- **Approval friction vs. capability.** Auto-approving all sandbox `bash` maximizes usefulness for data-analysis tasks but means the only barrier between the model and arbitrary code execution is the sandbox provider; a weak provider configuration converts directly into arbitrary-code-execution risk within the workspace's privileges.
- **AWS-coupled production isolation.** The hardened path (Lambda MicroVM) requires AWS-specific infrastructure and credentials (`config.ts:42-64`); OSS self-hosters who set no provider get no sandbox tools at all (`executeInAppAgentRun.ts:351-355` leaves the sandbox undefined), trading capability for safety rather than offering a middle tier.
- **Ephemeral keys cost a transaction per run.** Minting and deleting a DB-backed key per turn adds latency and write load, accepted deliberately to make the credential layer enforceable and auditable (`executeInAppAgentRun.ts:381-383` comment: "no crash window can leave a key that is not discoverable from its run").
- **Coarse admin bypass.** `isAdmin: true` short-circuits every scope check (`projectAccessRights.ts:287`), giving global admins the full agent tool surface in any project; simple, but all-or-nothing.
- **Backward-compatible override parsing slightly widens the window.** Accepting the legacy singular shape (`mcpPolicy.ts:389`) keeps old continuations runnable but means two wire formats authorize mutation during rollouts.

## Failure Modes / Edge Cases

- **Sandbox escape acknowledged in code.** Process-group kill is "best-effort: descendants can escape by creating a new session or process group. A sandbox should have lifetime limits as the final containment boundary" (`packages/in-app-agent-sandbox-runtime/src/server.ts:402-404`). The runtime itself imposes no CPU/memory/network quotas — those live entirely in the provider.
- **No application-layer auth on the sandbox HTTP server.** `POST /sandbox` executes whatever it receives; security rests on placement — `docker exec` over loopback in dev (`docker.ts:322-374`) and `X-aws-proxy-auth` at the AWS bridge in prod (`lambdaMicrovm.ts:487-494`). A misrouted port would be an unauthenticated code-execution endpoint.
- **Stale-session semantics.** Terminated MicroVMs cannot be revived; continuation silently resets workspace files while restoring `tool_calls/`, and the model is told via a synthetic run-scoped message the user never sees (`web/src/features/in-app-agent/README.md:208-210`). Deleting a conversation clears the pointer without terminating, so workspace data can outlive the visible object (`README.md:212`).
- **Rate-limit starvation at discovery.** MCP tool discovery consumes a rate-limit point like any call, so a busy org can be throttled "before the run has a chance to call a single tool" (`agent.ts:1052-1054`).
- **Approval outcome ambiguity.** If an approved mutation starts but its result never persists, the run fails with `OUTCOME_UNKNOWN` and asks the user to verify manually rather than retrying (`executeInAppAgentRun.ts:156-167`) — correct for safety, but a real operational edge.
- **Standing grants persist up to 24 h.** Conversation-scoped approvals (`approvalScope: "conversation"`) keep a mutating tool auto-approved across turns until TTL/cleanup (`tunables.ts:17`; `backgroundRunService.ts:397-400`), mitigated only by per-run role re-checks.

## Future Considerations

- Add an application-layer shared-secret or mTLS option to the sandbox runtime HTTP surface so isolation no longer depends solely on network topology (`packages/in-app-agent-sandbox-runtime/src/server.ts:43-84` currently binds and serves without token checks).
- Introduce resource quotas (CPU/memory/output bytes) at the sandbox runtime level, complementing the provider lifetime caps noted as the final containment boundary (`server.ts:402-404`).
- Consider finer-grained admin delegation than the boolean `isAdmin` bypass (`projectAccessRights.ts:287`), e.g., scoping global admins to audited, narrower scope sets for agent surfaces.
- Port the MicroVM-grade isolation story beyond AWS so self-hosted deployments get a production sandbox provider instead of the current "no provider ⇒ no sandbox tools" fallback (`config.ts:8-22`, `executeInAppAgentRun.ts:351-355`).
- Extend the exhaustive-classification pattern (which today covers only Langfuse MCP tools) to third-party MCP servers if the agent ever connects to user-configured external MCP endpoints; the current design assumes both MCP servers are Langfuse-owned (`agent.ts:1022-1048`).

## Questions / Gaps

- **No dedicated egress allowlist implementation found in-repo** for MicroVM networking: the egress control is delegated to an AWS connector ARN string (`config.ts:47-48`); what that connector permits is defined outside this source tree. Searched `sandbox/`, env schemas, and provider files.
- **Sandbox image build provenance** is referenced (image identifier, local build script at `docker.ts:88-90`) but the image recipe's contents (OS packages, users, seccomp/apparmor settings) are outside the inspected `src/` files; I did not find a Dockerfile within `packages/in-app-agent-sandbox-runtime` during analysis — searched the package directory listing, which contains only `contracts.test.ts`, `contracts.ts`, `logging.ts`, `server.ts`.
- **Worker-to-web trust for queue payloads** is revalidated at claim time, but I did not trace whether every field of the persisted `request` JSON survives revalidation equally (e.g., `context` passed through at `executeInAppAgentRun.ts:293-299`); the sanitizer boundary for browser-submitted context is asserted in `web/src/features/in-app-agent/README.md:18` ("the server remains the authoritative sanitizer") but I did not locate the specific sanitization function to cite line numbers.
- **EE-tier org-level RBAC interactions** with the agent policy (e.g., custom org roles) were not examined in depth; the policy consumes only `projectRole` and `isAdmin` (`mcpPolicy.ts:9-13`).

---

Generated by dimension `08.01-capability-model-and-trust-boundaries` against `langfuse`.
