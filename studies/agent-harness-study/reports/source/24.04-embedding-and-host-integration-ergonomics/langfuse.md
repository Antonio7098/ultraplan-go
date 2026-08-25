# Source Analysis: langfuse

## Dimension 24.04: Embedding and Host Integration Ergonomics

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript / Next.js (web), Express + BullMQ (worker), pnpm monorepo, Postgres + ClickHouse + Redis + S3 |
| Analyzed | 2026-08-24 |

## Summary

Langfuse (v4.16.0, `package.json:5`) is not a library-shaped agent harness; it is a deployable LLM engineering platform whose embedding story is "run our two containers and integrate over HTTP." The repo ships a web app (UI + tRPC + public REST API, `web/`), a queue worker (`worker/`), a shared contracts package (`packages/shared/`), and an EE package (`ee/`). Hosts integrate in three ways:

1. **Self-hosted/hosted service deployment** — `docker-compose.yml:6-195` defines the full topology (worker, web, ClickHouse, MinIO, Redis, Postgres); all configuration is injected via validated environment variables (`packages/shared/src/env.ts:32-95`, 587-line zod env schema), with provisioning hooks like `LANGFUSE_INIT_ORG_ID` at boot (`web/src/initialize.ts:1-60`).
2. **Public HTTP APIs** — Fern-specified REST contracts with Basic Auth (`fern/apis/server/definition/api.yml:1-16`) covering ingestion, OTel, traces, observations, scores, prompts, datasets, MCP, and more (`fern/apis/server/definition/*.yml`, e.g. `observations.yml`, `opentelemetry.yml`, `llm-connections.yml`); route implementations live under `web/src/pages/api/public/*` (e.g. `ingestion.ts`, `otel/`, `spans.ts`). OpenAPI specs are generated, never hand-edited (`package.json:36-40`, AGENTS.md "Generated Files" section).
3. **An embeddable sandbox sidecar** — `packages/in-app-agent-sandbox-runtime` is a standalone unprivileged HTTP control server for agent tool execution (read/write/edit/bash) with Docker-image and AWS Lambda MicroVM packaging (`packages/in-app-agent-sandbox-runtime/README.md:7-26,58-96`).

Internally, Langfuse embeds its own agent harness ("in-app agent") across browser/web/worker boundaries using durable AG-UI event contracts, durable approval interrupts, SSE watch streams, and Postgres-owned run lifecycle (`web/src/features/in-app-agent/README.md:12-59`). These integration seams are well-engineered but are internal product plumbing authenticated by Langfuse sessions — they are **not** exported as an SDK that another host can adopt while keeping its own identity/policy/UX.

The critical caveat for embedders: the web and worker processes perform module-import-time side effects (starting cleaners, runners, background migrations, binding ports), rely on process-wide singletons (Prisma client, `WorkerManager` registry, `ClickhouseReadSkipCache.getInstance()`), and expose no programmatic initialization/DI API. Embedding the harness *inside another application's process* is effectively unsupported; embedding it *beside* an application as services is mature.

## Rating

**5 / 10** — Service-style embedding is real and reasonably documented (deployable compose topology, Fern-generated OpenAPI, health/readiness endpoints, validated env config), and the sandbox runtime is a genuinely clean embeddable sidecar contract with tests. But there is no library/SDK embedding mode in this repo, no programmatic dependency-injection surface for storage/tools/policy/telemetry/secrets, host-facing policy (RBAC, MCP tool classifications) is code-owned rather than injectable, lifecycle constants are deliberately frozen in shared source (`packages/shared/src/in-app-agent/server/tunables.ts:2-17`), and both server processes carry import-time side effects and singletons that make in-process hosting unsafe. The dimension's core question — *"Can a host product embed the harness while retaining ownership of policy, state, telemetry, and UX?"* — is answerable only as "adopt the whole platform," which forfeits ownership of UX and policy.

## Evidence Collected

Every entry cites workspace-relative file paths from `studies/agent-harness-study/sources/langfuse`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Deployment-as-embedding | Compose topology of worker+web+ClickHouse+MinIO+Redis+Postgres images and ports | `docker-compose.yml:6-195` |
| Public API auth model | Basic Auth with public/secret key pair; OpenAPI export URL | `fern/apis/server/definition/api.yml:3-16` |
| Public API surface breadth | Ingestion, OTel, observations, scores, datasets, prompts, LLM connections, MCP definitions | `fern/apis/server/definition/{api.yml,scores-v3.yml,observations.yml,opentelemetry.yml,llm-connections.yml}` |
| Public REST implementation routes | `src/pages/api/public/*` incl. `ingestion.ts`, `otel/`, `spans.ts`, `health.ts`, `ready.ts` | `web/src/pages/api/public/` (dir listing; `ingestion.ts`, `health.ts`, `ready.ts`) |
| Contract generation discipline | `openapi:export` regenerates specs from Fern sources; generated outputs must not be hand-edited | `package.json:37-41`; root `.agents/AGENTS.md` (Generated Files section) |
| Env-based configuration injection | Zod-validated env schema (Redis TLS/cluster/sentinel keys shown) | `packages/shared/src/env.ts:32-95` |
| Boot provisioning hooks | `LANGFUSE_INIT_*` org/project/user/API-key upserts run during web startup | `web/src/initialize.ts:14-60` |
| Web instrumentation entrypoint | Next.js `register()` runs observability config + init scripts when `NEXT_RUNTIME === "nodejs"` | `web/src/instrumentation.ts:2-15` |
| Worker initialization | `initializeWorker()`: ClickHouse compatibility + default model prices/dashboards upserts | `worker/src/initialize.ts:5-9` |
| Env-gated consumer registration | Every BullMQ consumer registered only if its `QUEUE_CONSUMER_*_IS_ENABLED` flag is `"true"` | `worker/src/app.ts:137-149,151-164,432-448` |
| Graceful shutdown | `onShutdown` stops server/cleaners/runners, aborts agent loops, closes workers, flushes ClickHouse, disconnects redis/prisma | `worker/src/utils/shutdown.ts:29-116` |
| Signal handling | SIGINT/SIGTERM wired to `onShutdown` | `worker/src/app.ts:812-813` |
| Agent-run cancellation on shutdown | `activeRunAborts` set + `abortActiveInAppAgentRuns()` aborts loops at next step boundary; runs finish FAILED(worker_shutdown) | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:69-77`; `worker/src/utils/shutdown.ts:80-86` |
| Durable run claim/heartbeat/reconcile | Postgres owns correctness (claim CAS, heartbeat, reconcile); BullMQ stalled recovery disabled, lock outlives max duration | `worker/src/app.ts:432-444`; imports at `worker/src/features/in-app-agent/executeInAppAgentRun.ts:37-45` |
| Frozen lifecycle constants | Heartbeat 5s, stale 60s, queue timeout 300s, max run 900s, approval TTL 86400s as shared constants | `packages/shared/src/in-app-agent/server/tunables.ts:2-17`; rationale in `web/src/features/in-app-agent/README.md:170-176` |
| Host control surface (internal) | tRPC router: startRun / cancelRun / decideToolApproval / deleteConversation / renameConversation / submitFeedback behind session auth | `web/src/features/in-app-agent/server/router.ts:100-451` (startRun `207-240`, cancelRun `242-258`, decideToolApproval `260-305`) |
| Streaming to observers | SSE endpoint with cursor resume via query param or `last-event-id`, keepalive frames, typed error frame `watch_failed`, 90s max connection | `web/src/features/in-app-agent/server/watchHandler.ts:13-18,69-87,120-156,158-161`; `web/src/app/api/in-app-agent/watch/route.ts:5-9` |
| Durable transcript vocabulary | Runtime-neutral AG-UI message/event zod schemas shared by browser/web/worker | `packages/shared/src/in-app-agent/schema.ts:1-8,158-213` |
| Approval callback contract | `InAppAgentToolApprovalRequestSchema` (`tool_approval_request` with toolCallId/toolName/args/runId); tamper-proof decision path reads tool args from persisted event | `packages/shared/src/in-app-agent/schema.ts:221-231`; `web/src/features/in-app-agent/server/router.ts:260-267` |
| Error surfacing | Typed `InAppAgentRunErrorCode` incl. `OUTCOME_UNKNOWN` durability window ("approved action may have completed") | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:15,160-167` |
| Rate limiting surfaced to clients | `code: rate_limited` + `retryAfterSeconds` error schema | `packages/shared/src/in-app-agent/schema.ts:32-37`; applied at `web/src/features/in-app-agent/server/router.ts:223,283` |
| Policy is code-owned, not injectable | `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` classifies every MCP tool auto/approval; RBAC checked before tools are exposed | `packages/shared/src/in-app-agent/server/mcpPolicy.ts:27,477-512`; described in `web/src/features/in-app-agent/README.md:216-235` |
| Secrets handling | Per-run temporary MCP API key created then deleted with single-flight cleanup and pointer clearing | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:65,82-100,169-180` |
| Sandbox provider DI seam | `SandboxProvider` interface: `probeSession?/ensureSession/suspendSession?/terminateSession?` returning session-bound handles | `worker/src/features/in-app-agent/runtime/sandbox/types.ts:55-84` |
| Sandbox provider selection via env | `dangerous-docker` refused outside NODE_ENV=development; lambda-microvm requires image/role/region envs | `worker/src/features/in-app-agent/runtime/sandbox/config.ts:8-22,42-64` |
| Sandbox runtime wire contract | Discriminated union of read/write/edit/bash operations incl. per-call `toolCallFiles` sync | `packages/in-app-agent-sandbox-runtime/src/contracts.ts:8-41` |
| Sandbox HTTP control server | Fixed port 5000, `/health`, `/sandbox`, MicroVM ready/run/resume/suspend/terminate hooks; binds at module load | `packages/in-app-agent-sandbox-runtime/src/server.ts:25-31,79-84,109-180` |
| Sandbox containment safeguards | Workspace path-escape rejection, 10 MiB body cap, bash timeout with process-group SIGKILL + grace period, exclusive operation serialization | `packages/in-app-agent-sandbox-runtime/src/server.ts:34-41,391-432,473-487,489-527` |
| Sandbox tests | Contract unit test + container e2e hitting `/health`, `/sandbox`, MicroVM hooks | `packages/in-app-agent-sandbox-runtime/src/contracts.test.ts`; `packages/in-app-agent-sandbox-runtime/tests/sandbox.test.ts`; README `packages/in-app-agent-sandbox-runtime/README.md:45-56` |
| Agent runtime stream/cancellation | Mastra adapter receives `AbortSignal`; abortSignal plumbed into provider calls | `worker/src/features/in-app-agent/runtime/agent.ts:261,999-1001,1170,1216` |
| Import-time process side effects | Background migrations, ClickhouseReadSkipCache init, cleaners/runners started at module scope of app.ts | `worker/src/app.ts:123-135,699-810` |
| Module singletons | `ClickhouseReadSkipCache.getInstance(prisma)`; Prisma client singleton; module-level `WorkerManager` registry | `worker/src/app.ts:131-135`; `worker/src/utils/shutdown.ts:12,86`; root `.agents/AGENTS.md` (shared `src/db.ts` singleton) |

## Answers to Dimension Questions

**1. Can the harness run inside another application without owning the whole process?**
No, not as a library. The worker starts background migrations, caches, cleaners, and runner loops as import-time side effects of `worker/src/app.ts:123-135,699-810`, registers signal handlers at module scope (`worker/src/app.ts:812-813`), and depends on process-wide singletons (Prisma, Redis, `WorkerManager`, `ClickHouseClientManager`). The sandbox runtime server binds port 5000 at module load (`packages/in-app-agent-sandbox-runtime/src/server.ts:79-84`) with no programmatic start/stop API. The supported mode is process-beside-process: separate web/worker containers (`docker-compose.yml:6-99`) plus, optionally, the sandbox sidecar. The one legitimately embeddable component is the sandbox runtime package, consumed either as a local Docker container (`worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:28-33,66-109`) or Lambda MicroVM image (`packages/in-app-agent-sandbox-runtime/README.md:58-96`).

**2. Can the host supply policy, tools, identity, storage, telemetry, and secrets?**
Only as infrastructure endpoints, not as injected components:
- *Storage*: host provides Postgres/ClickHouse/Redis/S3 via connection env vars (`packages/shared/src/env.ts:32-95`; `.env.prod.example`), but repositories are hard-wired to those clients — no storage interface to implement.
- *Identity*: Langfuse enforces its own auth everywhere — Basic Auth public/secret keys for the REST API (`fern/apis/server/definition/api.yml:3-16`), session auth + project membership for tRPC/watch (`web/src/features/in-app-agent/server/router.ts:27-29`; `watchHandler.ts:22-53`). No external identity hook.
- *Policy*: RBAC gates and MCP tool approval classifications are code-owned constants (`packages/shared/src/in-app-agent/server/mcpPolicy.ts:27,477-512`); hosts cannot register their own policies.
- *Tools*: the local sandbox toolset is fixed to read/write/edit/bash (`packages/shared/src/in-app-agent/schema.ts:46-51`); custom tools exist only inside the worker runtime (`worker/src/features/in-app-agent/runtime/tools.ts`), not exposed for host registration.
- *Telemetry*: OpenTelemetry bootstrap is internal (`web/src/instrumentation.ts:2-15`; `@langfuse/shared/instrumentation/bootstrap` export per `packages/shared/AGENTS.md`); self-instrumentation writes into a configured Langfuse project (`router.ts:406-447`). Hosts consume Langfuse telemetry via OTel ingestion, they do not provide the sink.
- *Secrets*: env-var based; the system manages its own ephemeral per-run MCP credentials (`executeInAppAgentRun.ts:82-100,172-180`) rather than accepting a host secret resolver.

**3. Are lifecycle, cancellation, shutdown, and error propagation explicit?**
Yes — this is the strongest area. Worker shutdown is a staged sequence (stop intake → stop runners → abort agent loops at step boundary → drain BullMQ → flush ClickHouse → disconnect clients) with named reasons (`worker_shutdown`) and durability-aware failure codes (`OUTCOME_UNKNOWN` for possibly-executed approved mutations) (`worker/src/utils/shutdown.ts:29-116`; `executeInAppAgentRun.ts:67-77,160-167`). Cancellation is a first-class durable operation (`cancelRun` at `router.ts:242-258`; AbortController registry at `executeInAppAgentRun.ts:71-77`; AbortSignal plumbed into the model stream at `runtime/agent.ts:1170,1216`). Run correctness is explicitly moved out of the queue layer into Postgres claim CAS/heartbeat/reconcile with stalled recovery disabled (`worker/src/app.ts:438-443`). Errors are typed (`BaseError` hierarchy in `packages/shared/src/errors/`, `InAppAgentRunErrorCode`, rate-limit schema with retry-after). However, all of this explicitness is *internal*; external hosts see only HTTP status codes and JSON errors (`fern/apis/server/definition/api.yml:17-19` status-code discrimination).

**4. Does the integration model work for both local-first and service deployments?**
Yes for deployment-based integration. The same containers serve dev (`pnpm run dx` with compose infra, `package.json:16-30`), self-host (`docker-compose.yml`), and cloud (region-scoped flags like `NEXT_PUBLIC_LANGFUSE_CLOUD_REGION` at `packages/shared/env.ts:33`; Stripe/cloud-only queues at `worker/src/app.ts:450-491`). The in-app agent documents capacity knobs per deployment (`LANGFUSE_IN_APP_AGENT_MAX_ACTIVE_RUNS_PER_ORG`, `README.md:157-176`). The sandbox runtime explicitly supports two execution substrates with one contract (local Docker vs Lambda MicroVM, `web/src/features/in-app-agent/README.md:178-202`). What does not transfer is *in-process* embedding: there is no local-first "import the harness into my CLI/app" path anywhere in this repo. Note also that the language SDKs referenced by the public API docs live in separate repositories; no client SDK packages exist inside this source (searched `packages/*`: only `shared`, `config-eslint`, `config-typescript`, `eslint-plugin`, `in-app-agent-sandbox-runtime`).

## Architectural Decisions

- **Platform, not library.** The unit of adoption is a container pair plus four backing stores (`docker-compose.yml:6-195`). All cross-cutting configuration flows through one zod-validated environment schema (`packages/shared/src/env.ts:32+`) instead of programmatic options objects, making deployments uniform but ruling out in-process hosting.
- **Contract-first public API.** REST surfaces are defined in Fern YAML (`fern/apis/server/definition/**`) and regenerated into OpenAPI (`package.json:37-41`); hand-editing generated output is forbidden (root `.agents/AGENTS.md`). This gives embedders stable, versioned HTTP contracts (e.g. v2/v3 score routes coexisting: `scores.yml`, `scores-v3.yml`) independent of internal refactors.
- **Durable event log as the integration spine.** The in-app agent persists AG-UI events as the render source of truth; browsers hydrate snapshots and tail SSE streams with resumable cursors instead of holding live connections to the executor (`web/src/features/in-app-agent/README.md:12-25,103-119`; `conversationSnapshot.ts` rebuilds state from one read of the event log).
- **Postgres-owned run correctness.** Claim CAS + heartbeat + reconcile supersede queue redelivery; BullMQ locks merely outlive the run backstop and stalled recovery is zeroed (`worker/src/app.ts:432-444`). This makes the executor safe under worker restarts — a property embedders usually have to build themselves.
- **Fixed lifecycle policy in shared source.** Queue timeout, max run duration, approval TTL, heartbeat intervals are compile-time constants shared by web and worker so the two processes cannot diverge (`packages/shared/src/in-app-agent/server/tunables.ts:2-17`; `README.md:170-176` states these are intentionally not env vars).
- **Sandbox as isolated sidecar with a narrow wire contract.** Tool execution is pushed into an unprivileged single-purpose HTTP service (`sandbox-server` user, network-disabled container in dev) with one discriminated-union endpoint (`packages/in-app-agent-sandbox-runtime/README.md:7-26`; `contracts.ts:36-41`), keeping destructive capability out of the trust boundary of web/worker.

## Notable Patterns

- **Env-flag-gated composition root.** Each queue consumer, cleaner, and runner is registered only under an explicit `*_IS_ENABLED === "true"` check (`worker/src/app.ts:137-149,432-448,699-810`), letting operators shrink the process to the features they run.
- **Session-bound handle pattern for providers.** `SandboxProvider.ensureSession()` returns a `SandboxSession` closing over the backing runtime so callers pass only per-operation inputs (`worker/src/features/in-app-agent/runtime/sandbox/types.ts:16-48`); optional `probeSession` distinguishes "lost workspace" before the agent starts and feeds a run-scoped system message (`README.md:204-214`).
- **Tamper-proof approval decisions.** The client sends only IDs plus a boolean; tool name/args are re-read server-side from the persisted interrupt event (`web/src/features/in-app-agent/server/router.ts:260-267`), eliminating fingerprint-sync bugs.
- **Resumable SSE with keepalive and bounded connections.** Cursor via query param or `Last-Event-ID`, comment-frame keepalives, forced 90s reconnect inside the 120s platform cap (`web/src/features/in-app-agent/server/watchHandler.ts:33-47,128`; `watch/route.ts:5-9`).
- **Single-flight cleanup.** Concurrent finish/failure paths share one MCP-key cleanup promise (`executeInAppAgentRun.ts:172-180`), avoiding double-delete races.
- **Defense-in-depth in the sandbox**: path-escape rejection, request-size caps, serialized operations, process-group SIGKILL with documented best-effort limits and a final containment backstop (`server.ts:34-41,400-405,473-487`).

## Tradeoffs

- **Operational simplicity vs embedding freedom.** Env-var-only configuration and container deployment make self-hosting predictable, but a host cannot compose Langfuse internals (its own storage layer, its own scheduler) — integration is limited to the HTTP membrane.
- **Internal protocol quality vs external availability.** The AG-UI event vocabulary, durable approvals, and resumable watch streams are excellent harness-integration primitives, yet they are reachable only through Langfuse session-authenticated tRPC/SSE routes (`router.ts:27-29`, `watchHandler.ts:22-53`), not through the public API; an external host cannot reuse them without forking the auth model.
- **Frozen constants vs operator flexibility.** Hard-coded lifecycle timings prevent web/worker drift (a real reliability win) but remove tuning levers an embedding host would normally expect (`tunables.ts:2-17`; `README.md:174-176`).
- **Import-time wiring vs fast boot.** Side-effectful module scope removes boilerplate for the primary "run the container" flow but makes tree-shaken or embedded reuse impossible without refactor (`app.ts:123-135,699-810`).
- **Two sandbox substrates vs provider extensibility.** Exactly two providers exist; the `SandboxProvider` interface is a clean seam but is worker-internal (`types.ts:55-84`) — third parties cannot ship providers as plugins today.

## Failure Modes / Edge Cases

- **Approved-but-unverified mutations.** If a worker dies between executing an approved mutating tool and persisting its result, the run fails with `OUTCOME_UNKNOWN` and instructs the user to verify manually — an honest acknowledgment of the durability window (`executeInAppAgentRun.ts:160-167`).
- **Lost sandbox sessions.** Terminated MicroVMs cannot revive; continuation silently starts a fresh VM, restores only persisted conversation history and reconstructed `tool_calls/`, and warns the model (not the user) that files are gone (`web/src/features/in-app-agent/README.md:208-214`).
- **Escaped child processes.** Process-group kill after bash timeout is explicitly best-effort; descendants that create new sessions escape it, relying on outer lifetime limits (`server.ts:402-405`).
- **Queue misconfiguration kills runs.** With `QUEUE_CONSUMER_IN_APP_AGENT_RUN_QUEUE_IS_ENABLED` off, committed runs sit `QUEUED` until `queue_timeout` failure — a documented operational footgun (`README.md:157-161`).
- **Watch-stream degradation.** Failures mid-stream convert into a typed `watch_failed` frame and connection close; recovery relies on client reconnect semantics (`watchHandler.ts:132-153`).
- **Dev-only guardrails.** `dangerous-docker` throws outside development, preventing accidental production use of the unhardened provider (`config.ts:15-19,27-32`).

## Future Considerations

- Expose the in-app agent's durable run/approval/watch contracts (AG-UI events + interrupt parser + resumable SSE) through the public API so external hosts could operate the harness under their own identity and UX; the pieces already exist and are tested (`schema.ts`, `interrupts.ts`, `watchHandler.ts`).
- Add a programmatic initialization API (options object, injectable logger/store) wrapping `initializeWorker`/`onShutdown` to enable library-style embedding and better test harnessing.
- Allow policy extension points: today `IN_APP_AGENT_LANGFUSE_MCP_TOOL_POLICIES` requires a code change per tool (`mcpPolicy.ts:27`); a data-driven or host-supplied classifier would decouple product releases from policy updates.
- Publish the `SandboxProvider` interface as a versioned package seam so third-party isolation backends (gVisor, Firecracker directly, WASM) can be integrated without touching worker internals (`types.ts:55-84`).

## Questions / Gaps

- **No client SDKs in-repo.** The public API implies multi-language SDKs, but none live here (only `packages/shared`, lint configs, and the sandbox runtime under `packages/`), so SDK ergonomics (retry, streaming helpers, typed clients) could not be assessed from this source. Search boundary: `packages/*` listing and repo-wide glob for sdk packages.
- **EE embedding surfaces not fully audited.** `ee/src` was not exhaustively inspected; enterprise-only integrations (SCIM exists at `web/src/pages/api/public/scim`) may add host-facing surfaces not covered above.
- **Web-side graceful shutdown** equivalent to the worker's staged `onShutdown` was not located (Next.js manages its own lifecycle); evidence gap for long-running web requests beyond the documented 90s watch cap (`watch/route.ts:5-9`).
- Whether the Fern-generated clients include webhook/OAuth machine-to-machine flows beyond Basic Auth could not be confirmed from `fern/apis/server/definition/api.yml` headers alone.

---

Generated by `Dimension 24.04: Embedding and Host Integration Ergonomics` against `langfuse`.
