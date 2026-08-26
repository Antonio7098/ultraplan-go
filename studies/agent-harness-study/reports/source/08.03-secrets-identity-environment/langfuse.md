# Source Analysis: langfuse

## Dimension 08.03: Secrets, Identity, and Environment Handling

### Source Info

| Field | Value |
|-------|-------|
| Name | langfuse |
| Path | `studies/agent-harness-study/sources/langfuse` |
| Language / Stack | TypeScript monorepo: Next.js (`web`), Express+BullMQ worker (`worker`), shared package (`packages/shared`), enterprise package (`ee`), microVM sandbox runtime (`packages/in-app-agent-sandbox-runtime`) |
| Analyzed | 2026-08-26 |

## Summary

Langfuse is a multi-tenant LLM observability platform, so its secret-handling story centers on three credential classes rather than on agent tool credentials: (1) third-party provider credentials users store in the platform (LLM connections, Slack tokens, blob-storage keys, SSO client secrets, webhook signing secrets), (2) Langfuse's own project/org API keys, and (3) ephemeral identities minted for its in-app agent.

The core model is consistent across all three classes:

- **Encrypt at rest, decrypt at execution time.** All user-stored provider credentials are sealed with AES-256-GCM under an `ENCRYPTION_KEY` validated at boot (`packages/shared/src/env.ts:97-103`), and decrypted only inside the job handler or LLM call that needs them (`packages/shared/src/server/llm/llmText.ts:301-304`, `worker/src/queues/webhooks.ts:538`). Clients never receive the ciphertext-bearing fields; read paths return allowlisted "safe" projections with masked display values (`web/src/features/llm-api-key/server/router.ts:412-422`, `packages/shared/src/domain/automations.ts:218-229`).
- **Hash, never store, platform API keys.** Project/org API keys are stored as bcrypt hashes plus a salted SHA-256 "fast hash" for O(1) Redis-cached verification; only a display prefix is retained (`packages/shared/src/server/auth/apiKeys.ts:14-40`, `web/src/features/public-api/server/apiAuth.ts:96-151`).
- **Per-task ephemeral identity for the agent.** Every in-app agent run mints a dedicated project-scoped MCP API key inside the same transaction that links it to the run row, flags it `isInAppAgentKey`, defaults it to read-only permissions, and deletes it when the run finishes (`worker/src/features/in-app-agent/executeInAppAgentRun.ts:381-399`, `web/src/pages/api/public/mcp/index.ts:193-217`).
- **Environment isolation via sandboxing and schema validation.** In-app agent code execution runs in per-conversation AWS Lambda microVMs or network-disabled Docker containers; no environment variables are injected into sandboxes — only explicit tool-call files (`worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:77-85`). Process env is zod-validated per app (`packages/shared/src/env.ts:32+`, `web/src/env.mjs`).

Redaction of *trace content* (what users ingest) is deliberately separate from redaction of *platform secrets*: content masking is an opt-in EE callback feature with fail-open/fail-closed semantics (`packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:145-216`), while platform-secret exposure is controlled by data-layer omission, allowlist sanitizers, and negative tests codified in the repo's own security-review doctrine (`.agents/skills/security-review/references/secret-read-paths.md:44-72`).

## Rating

**8 / 10** — A clear, well-tested model with explicit interfaces: a single encryption module, paired sanitized/raw repository accessors, client-wide Prisma `omit` of secret columns, per-run ephemeral agent keys with deletion guards, SSRF/outbound-URL guards that block credentialed URLs, background migrations that encrypt legacy plaintext rows, and VIEWER-role negative tests asserting secrets never reach clients. It falls short of 9–10 because log-pipeline redaction does not exist as a mechanism (the winston logger has no `redact` filter; discipline is procedural), trace-content masking is EE-gated and fail-open by default, and shipped compose files default to an all-zeros `ENCRYPTION_KEY`.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Secret encryption primitive | AES-256-GCM with random IV + auth tag; throws without key; `keyGen()` helper emits 64-hex keys | `packages/shared/src/encryption/encryption.ts:18-34` |
| Env validation of master key | Zod length-64 hex check with generation instructions | `packages/shared/src/env.ts:97-103`; `web/src/env.mjs:346-350` |
| Platform API key storage | bcrypt legacy hash, salted SHA-256 fast hash, display prefix `pk…sk` only; `SALT` required | `packages/shared/src/server/auth/apiKeys.ts:10-40,59-89` |
| Key verification path | Basic-auth verify via fast-hash Redis lookup, bcrypt fallback, lazy upgrade to fast hash; public-key mismatch warning | `web/src/features/public-api/server/apiAuth.ts:92-206` |
| Encrypted credential write | LLM connection `secretKey`/`extraHeaders` encrypted before insert; masked `displaySecretKey` stored alongside | `web/src/features/llm-api-key/server/router.ts:267-285,50-60` |
| Safe read projection | Read strips `secretKey`/`extraHeaders`, parses through `SafeLlmApiKeySchema`, derives Bedrock auth method server-side only | `web/src/features/llm-api-key/server/router.ts:390-433`; `web/src/features/llm-api-key/types.ts:55-61` |
| Client-wide column omission | Prisma client omits dataset remote-experiment secret key/headers from every result; execution opts back in via `select` | `packages/shared/src/db.ts:32-39` |
| Paired accessors (sanitize vs execute) | `getActionByIdWithSecrets` reserved for worker delivery; `getActionById` returns allowlisted domain object | `packages/shared/src/server/repositories/automation-repository.ts:33-111` |
| Allowlist sanitizers | Per-type converters include only safe fields (display headers, display token) | `packages/shared/src/domain/automations.ts:218-229,268-278` |
| Decrypt-at-execution | Worker decrypts Mixpanel token / webhook signing secret / GitHub token inside job handlers | `worker/src/features/mixpanel/handleMixpanelIntegrationProjectJob.ts:253`; `worker/src/queues/webhooks.ts:538,679-688` |
| LLM credential injection | `decrypt(options.connection.secretKey)` immediately before model call; decrypted extra headers treated as sensitive on redirect | `packages/shared/src/server/llm/llmText.ts:301-318` |
| Redirect exfiltration guard | `createSecureLlmFetch` strips provider auth + gateway headers when origin changes | `packages/shared/src/server/llm/llmText.ts:306-318` |
| Outbound URL hardening | Validation rejects URLs embedding credentials, blocked IPs/hosts, disallowed ports/protocols (anti-SSRF around stored endpoints) | `packages/shared/src/server/outbound-url/validation.ts:10-19` |
| Legacy-plaintext remediation | Background migration detects unencrypted blob-storage secrets and encrypts them in place | `worker/src/backgroundMigrations/encryptBlobStorageSecrets.ts:74-107` |
| SSO secret handling | Admin API refuses to run without `ENCRYPTION_KEY`; SSO `clientSecret` encrypted on write, decrypted per-provider on use | `web/src/ee/features/multi-tenant-sso/createNewSsoConfigHandler.ts:28-31`; `web/src/ee/features/multi-tenant-sso/utils.ts:204-343` |
| Ephemeral agent identity | Per-run MCP key minted in transaction linking `inAppAgentRun.mcpApiKeyId`; flagged `isInAppAgentKey`, attributed to `createdByUserId` | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:381-399` |
| Identity teardown | Single-flight cleanup deletes key + nulls pointer on finish/error/cancel; reconcile sweeps terminal runs | `worker/src/features/in-app-agent/executeInAppAgentRun.ts:82-100,130-137,172-193` |
| Key-type deletion guard | Refuses to delete a normal project key through the agent-key path (and vice versa) | `packages/shared/src/server/auth/apiKeys.ts:124-134,147-162` |
| Agent permission scoping | MCP route requires project-scope BasicAuth; in-app-agent keys are read-only unless a per-run tool-allowlist override header parses | `web/src/pages/api/public/mcp/index.ts:86-106,134-148,193-217` |
| Audit attribution | Audit logs resolve agent-key actors back to their human creator via `createdByUserId` | `web/src/features/audit-logs/auditLog.ts:96-122` |
| Trace-content masking | EE ingestion masking callback over OTEL events pre-ClickHouse; retries, fail-open default, fail-closed opt-in; wired into queue consumer | `packages/shared/src/server/ee/ingestionMasking/applyIngestionMasking.ts:145-216`; `worker/src/queues/otelIngestionQueue.ts:518-542`; env knobs `packages/shared/src/env.ts:562-576` |
| Logging pipeline | Winston logger adds trace correlation but has **no** redact filter; JSON/text formats only | `packages/shared/src/server/logger.ts:27-55` |
| Log-volume discipline | Sandbox providers log summarized payloads (byte counts, truncated previews) instead of full content | `worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:537-599` |
| Sandbox env isolation (prod) | Per-conversation Lambda microVMs: idle policy, 4h max duration, suspend/resume/terminate, short-lived bridge auth tokens (30 min, refresh buffer) | `worker/src/features/in-app-agent/runtime/sandbox/providers/lambdaMicrovm.ts:23-31,85-142,517-561` |
| Sandbox env isolation (dev) | Docker containers created with `NetworkDisabled: true`, no `Env` passed, sanitized container names per conversation | `worker/src/features/in-app-agent/runtime/sandbox/providers/docker.ts:66-109,676-685` |
| Sandbox filesystem jail | Workspace-root path containment rejects escapes; 10 MB body cap; command timeouts with process-group SIGKILL; serialized ops | `packages/in-app-agent-sandbox-runtime/src/server.ts:26-41,29,391-432,473-487` |
| Credentials stay out of VMs | Agent loop builds model calls and MCP BasicAuth header in the worker process; sandbox operations carry only tool-call file contents | `worker/src/features/in-app-agent/runtime/agent.ts:240-246,267-269`; `packages/in-app-agent-sandbox-runtime/src/server.ts:86-107` |
| Negative read-path tests | `describe("automations read path secret redaction")` asserts VIEWER-role reads lack `secretKey`/headers; many `not.toHaveProperty` assertions across suites | `web/src/__tests__/server/automations-trpc.servertest.ts:2896,1243-1244`; `web/src/__tests__/server/datasets-remote-experiment.servertest.ts:403-411`; `web/src/__tests__/server/blob-storage-integration-api.servertest.ts:223,297` |
| Encryption unit/regression tests | keyGen length, known-vector decrypt, roundtrip; worker webhook test covers decryption-failure skip path | `worker/src/__tests__/encryption.test.ts:21-68`; `worker/src/__tests__/webhooks.test.ts:1259,1337` |
| Deployment defaults | Compose ships all-zeros `ENCRYPTION_KEY` and trivial `SALT` with CHANGEME comments; prod example repeats placeholder values | `docker-compose.yml:24-25`; `.env.prod.example:22,26` |
| Codified internal doctrine | Security-review skill documents the secret-read-path threat model, canonical helpers, required defenses, anti-patterns | `.agents/skills/security-review/references/secret-read-paths.md:1-102` |

## Answers to Dimension Questions

**1. Can the model see secrets?**
The models invoked by Langfuse's own agent runtime cannot see platform secrets by construction. Model-provider credentials are resolved and decrypted inside the worker process when constructing provider calls (`packages/shared/src/server/llm/llmText.ts:301-304`, `worker/src/features/in-app-agent/runtime/agent.ts:238-246`); the sandbox receives only tool-call file contents and no env vars (`packages/in-app-agent-sandbox-runtime/src/server.ts:86-107`, `docker.ts:77-85`). The in-app agent reaches Langfuse data exclusively through an ephemeral, per-run MCP key whose mutating tools require a run-scoped allowlist header (`web/src/pages/api/public/mcp/index.ts:193-217`). Caveat: trace *content* the user ingested may itself contain secrets — content masking exists but is EE-gated and off unless configured (`applyIngestionMasking.ts:43-57`), so a model reading traces via MCP sees whatever the customer stored.

**2. Can tools use secrets without exposing them?**
Yes — this is the strongest axis. The pattern is encrypt-on-write (`router.ts:270`), mask-for-display (`router.ts:50-60`), strip-on-read (`router.ts:412-422`; `db.ts:34-39`), decrypt-in-handler (`webhooks.ts:538`), and treat decrypted headers as sensitive during fetch redirects (`llmText.ts:306-318`). Stored endpoint URLs are validated to reject embedded credentials and blocked hosts, closing an SSRF-to-cloud-metadata exfiltration path (`outbound-url/validation.ts:10-19`). Webhook delivery even avoids writing decrypted execution state back into the stored JSON config (`webhooks.ts:1012-1026`).

**3. Are secrets redacted in traces?**
Platform secrets are structurally excluded from trace/API responses (allowlist sanitizers, safe schemas, negative tests). Trace *payload* redaction is available only as the EE ingestion-masking callback with configurable fail-open/fail-closed behavior (`applyIngestionMasking.ts:200-215`); OSS deployments have no built-in payload masking. Application logs have no redaction mechanism — the shared winston logger defines formatting only (`logger.ts:27-55`) — so redaction depends on call-site discipline (e.g., summarized sandbox logging, `docker.ts:537-591`). One residual leak vector: the LLM-key delete mutation passes the raw DB row (including encrypted `secretKey`/`extraHeaders` ciphertext) into audit-log `before` state (`router.ts:312-317,353-359`; serialized verbatim at `auditLog.ts:92-93`) — ciphertext, not plaintext, but it widens the blast radius of an `ENCRYPTION_KEY` compromise exactly as the repo's own doctrine warns (`secret-read-paths.md:14-17`).

**4. Are identities scoped per user/task?**
Yes, at three granularities. API keys are PROJECT- or ORGANIZATION-scoped (`apiKeys.ts:73-74`) with RBAC scope checks per feature (`router.ts:207-211`). The in-app agent gets a fresh project-scoped key per run, linked to the run row, attributed to the initiating user, and deleted at terminal states with a reconciliation sweep for crashed workers (`executeInAppAgentRun.ts:381-399,172-193,130-137`). Sandbox compute is scoped per conversation with time-boxed existence (4h cap) and short-lived bridge auth tokens (`lambdaMicrovm.ts:29-30,278-287`). Audit logs distinguish user-actor vs API-key-actor records and map agent keys back to their creator (`auditLog.ts:60-84,96-122`).

## Architectural Decisions

1. **One symmetric master key per deployment, not per tenant.** `ENCRYPTION_KEY` seals every integration secret across projects (`encryption.ts:4`). Simpler operationally (self-hosted friendly) but makes the key a single high-value target; the repo acknowledges this blast-radius concern in its own doctrine (`secret-read-paths.md:14-17`).
2. **Dual hashing for API keys: correctness then speed.** bcrypt remains the source of truth while a salted SHA-256 fast hash enables Redis-cached verification; legacy keys lazily upgrade on first use (`apiAuth.ts:99-151`, `apiKeys.ts:14-18`). Tradeoff: two hash schemes must stay in sync forever.
3. **Sanitization belongs in the data layer, not routes.** Canonical converters plus a client-wide Prisma `omit` mean every read path inherits stripping by default, and delivery paths must explicitly opt back in (`db.ts:32-39`, `automation-repository.ts:33-111`, doctrine at `secret-read-paths.md:44-67`).
4. **Ephemeral capability keys instead of reusing long-lived keys for the agent.** Mint-in-transaction linking guarantees no orphaned undiscoverable keys; deletion guards prevent cleanup from eating user keys (`executeInAppAgentRun.ts:381-399`, `apiKeys.ts:124-134`). This mirrors classic capability-token design applied to database-backed API keys.
5. **Compute isolation pushed to the cloud primitive.** Production sandboxes are managed Lambda microVMs with suspend/resume rather than in-process isolation, accepting cold-start/session-replacement complexity (`lambdaMicrovm.ts:144-271`) in exchange for strong boundaries.
6. **Trace-content masking externalized to a callback**, not implemented in-process (`applyIngestionMasking.ts:62-135`) — keeps Langfuse agnostic of customer PII policy but shifts availability risk onto ingestion (hence fail-open/fail-closed knob).

## Notable Patterns

- **Masked-display twins**: every secret column has a sibling display field computed at write time (`displaySecretKey` at `router.ts:278`, `displayHeaders` in `automation-repository.ts:75`), so UIs never need the real value.
- **Allowlist-by-construction safe schemas**: `Safe*Schema = FullSchema.omit({secrets})` keeps type and allowlist derived from one declaration (`automations.ts:87,123`; doctrine `secret-read-paths.md:30-32`).
- **Exhaustive-switch sanitization**: unhandled action types become compile errors via `never` assignment rather than falling through with secrets (`secret-read-paths.md:49-60`).
- **Negative testing as security invariant**: least-privileged-role read assertions (`expect(config).not.toHaveProperty("secretKey")`) across list *and* detail routes (`automations-trpc.servertest.ts:2896+`, `blob-storage-integration-api.servertest.ts:223,297`).
- **Credential-aware fetch wrappers**: decrypted extra headers are appended to the sensitive-header set stripped on cross-origin redirects (`llmText.ts:306-318`).
- **Default-credential sentinels**: magic values like `BEDROCK_USE_DEFAULT_CREDENTIALS` let self-hosted deployments use IAM/ADC roles instead of storing static keys, with cloud deployments explicitly forbidden from them (`router.ts:217-250`; sentinel handling `router.ts:50-55`).
- **Token lifecycle management**: `RefreshingTokenManager` refreshes managed tokens ahead of expiry (80% of remaining TTL) and pushes rotations to live connections (`RefreshingTokenManager.ts:10-92`).

## Tradeoffs

- **Operational simplicity vs blast radius**: one global AES key and one global `SALT` keep self-hosting simple but concentrate compromise value; there is no per-project key envelope or rotation story visible in code (key rotation would require re-encrypting every row; only a legacy-plaintext migration exists, `encryptBlobStorageSecrets.ts`).
- **Fail-open masking vs ingestion durability**: default fail-open preserves trace delivery when the masking service is down (`applyIngestionMasking.ts:210-215`) but silently stores unmasked PII; fail-closed drops events and tells operators to replay from S3 (`otelIngestionQueue.ts:527-540`).
- **Compile-time sanitization vs velocity**: the allowlist/exhaustive-switch regime adds ceremony to every new automation type, which the repo justifies explicitly as the cost of not shipping the next credential (`secret-read-paths.md:19-22`).
- **Ephemeral agent keys vs key churn**: per-run mint/delete creates a row per run and leans on correct cleanup paths (including crash reconciliation) rather than TTL expiry — durable but dependent on the reconcile sweep actually running (`executeInAppAgentRun.ts:125-137`).
- **No log-redaction mechanism vs flexibility**: avoiding a central redaction filter avoids false-positive performance costs on hot paths, but means any future `logger.info(config)` regression leaks by default.

## Failure Modes / Edge Cases

- **Decryption failure handling**: corrupt cipher format raises a typed error (`encryption.ts:41-43`); webhook delivery skips invalid secret headers gracefully and continues (`webhooks.test.ts:1259,1337`).
- **Stale identity caches**: deleting an API key invalidates Redis cache entries so revocation takes effect immediately (`apiKeys.ts:136`, `invalidateCachedApiKeys.ts`); non-existent keys are negatively cached (`apiAuth.ts:117-125`).
- **Partial credential rotation mismatch**: if the submitted public key doesn't match the key resolved via secret-hash, the mismatch is logged as a warning rather than failing open (`apiAuth.ts:181-188`).
- **Sandbox session loss mid-run**: lost microVM sessions are detected (probe/terminal-state checks) and replaced transparently, with the run informed its workspace was reset (`lambdaMicrovm.ts:372-398`; `agent.ts:255-256`).
- **Legacy plaintext rows**: background migration tolerates already-encrypted rows by attempting decrypt first and logging unexpected errors separately (`encryptBlobStorageSecrets.ts:74-107`).
- **Residual gaps observed**: audit-log delete records embed ciphertext secrets (`router.ts:353-359`); compose defaults invite running with known-zero crypto material (`docker-compose.yml:24-25`); no mechanism prevents a privileged user's browser receiving `extraHeaderKeys` correlated with display values (low sensitivity, keys-only by design, `router.ts:274-276`).

## Future Considerations

- Add a winston `redact` filter (or structured-field denylist) in `packages/shared/src/server/logger.ts` so credential-shaped fields cannot reach logs regardless of call-site discipline.
- Strip encrypted `secretKey`/`extraHeaders` from the `before` payload of llmApiKey delete audit logs (`web/src/features/llm-api-key/server/router.ts:353-359`).
- Provide an envelope/key-rotation path for `ENCRYPTION_KEY` (per-row key IDs) to shrink the single-key blast radius called out in `secret-read-paths.md:15`.
- Consider making ingestion masking fail-closed-by-default per project, or offering an in-process OSS masking hook, since the current capability is license-gated (`applyIngestionMasking.ts:43-57`).
- Add TTL-based expiry for in-app-agent MCP keys as a backstop behind the reconcile sweep (`executeInAppAgentRun.ts:130-137`).

## Questions / Gaps

- No evidence found of per-tenant or per-project encryption keys; searches across `packages/shared/src/encryption`, `web/src/features/*/server`, and migrations surfaced only the global `ENCRYPTION_KEY`.
- No evidence found of secret scanning in CI (searched `.github/workflows` only superficially via grep for ENCRYPTION_KEY; a dedicated gitleaks/trufflehog workflow was not located within the inspected boundary — searched `.github/workflows/*` for "secret scan" patterns).
- Whether ClickHouse/S3 backups inherit any redaction beyond the optional ingestion masking could not be verified from code alone; no post-storage scrub job was found under `worker/src/features` or `packages/shared/src/server/data-deletion` beyond full deletion processors.
- The in-app agent's own tracing (`langfuseTracing`, `agent.ts:583-594`) captures tool-call payloads into Langfuse traces; whether MCP tool outputs could embed the ephemeral key material was not proven either way from code (the key is transmitted only in the Authorization header to the MCP endpoint, `agent.ts:267-269,1029`).

---

Generated by `Dimension 08.03: Secrets, Identity, and Environment Handling` against `langfuse`.
