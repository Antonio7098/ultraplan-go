# Source Analysis: openhands

## Dimension 08.03 — Secrets, Identity, and Environment Handling

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite), `@openhands/typescript-client`; frontend ("agent-canvas") of a multi-repo system whose agent-server/SDK lives in a sibling repo |
| Analyzed | 2026-08-24 |

## Summary

This repository is the OpenHands **frontend** only; the server that actually stores, encrypts, and injects secrets lives in the sibling `software-agent-sdk` repo and is out of scope here. Within this boundary, the codebase implements a coherent client-side secrets model built on four pillars:

1. **Server-side storage with value-less listing.** All custom secrets live on the agent-server (`PUT/GET/DELETE /api/settings/secrets` via `SecretsService`, `src/api/secrets-service.ts:26-156`); the list endpoint returns names/descriptions only — the `CustomSecretWithoutValue` type has no `value` field (`src/api/secrets-service.types.ts:11-15`). Git provider tokens are never mirrored to localStorage; the UI reads only the boolean-ish `provider_tokens_set` map (`src/types/settings.ts:132`, `src/hooks/use-user-providers.ts:9-10`).

2. **Three-mode secret exposure.** Settings are fetched under an `X-Expose-Secrets` contract: unset → values redacted to `"**********"` for display, `"encrypted"` → Fernet ciphertext safe for round-trip, `"plaintext"` → explicitly documented as backend-only ("DO NOT USE from frontend") (`src/api/settings-service/settings-service.api.ts:115-122, 421-512`). Separate caches keep redacted display settings away from encrypted conversation-start settings (`settings-service.api.ts:162-181`), and conversation start refuses to fall back to redacted credentials (`settings-service.api.ts:500-511`).

3. **Single injection channel via indirection.** At conversation start every saved secret is attached as a `LookupSecret` pointing back at `/api/settings/secrets/{name}` with session-auth headers; the agent-server resolves them at spawn time (`request.secrets`, `src/api/agent-server-adapter.ts:995-1000, 1203-1228`). Regression tests pin "request.secrets is the sole channel" and forbid mirroring secrets into `agent_context` where prompt-facing context could pick them up (`__tests__/api/agent-server-adapter.test.ts:561-656`). ACP provider credentials ride the same channel; the deprecated inline `acp_env` field is stripped from payloads (`agent-server-adapter.ts:842-843, 932-935`).

4. **Defense-in-depth redaction at display surfaces.** MCP probe/test errors are scrubbed with a value-collection + generic-token-pattern redactor (`src/utils/redact-mcp-secrets.ts:95-130`); dynamic-context `<CUSTOM_SECRETS>` blocks are re-masked in the UI as a "defensive backstop" against backend masking regressions (`src/utils/redact-custom-secrets.ts:1-32`, applied at `src/utils/system-message-adapter.ts:26`).

Environment isolation between runs is handled through per-conversation working directories and git worktrees rather than per-run env sandboxes; one explicit gap (`acp_isolate_data_dir`) is tracked as TODO(#1019) in `src/api/agent-server-adapter.ts:831-836`.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

The exposure-mode header (`settings-service.api.ts:115-122`), the sole-channel `LookupSecret` design pinned by regression tests (`__tests__/api/agent-server-adapter.test.ts:561-656`), and layered redaction with generic token patterns (`redact-mcp-secrets.ts:18-31`) constitute a clear, tested model with real safeguards (Fernet-at-rest key management in `docker/entrypoint.sh:175-191`; generated per-install session keys). It stops short of 8–9 because: all saved secrets attach globally to every conversation start with no per-conversation selection (`agent-server-adapter.ts:1267-1292` enumerates everything); the rename flow pulls a plaintext value back through the browser (`secrets-service.ts:106-116`) because the server exposes only upsert; `acp_isolate_data_dir` is not yet set so concurrent same-provider ACP conversations can race on a shared HOME; and redaction covers curated surfaces but not the general event stream (terminal observations render verbatim). Server-side enforcement is unverifiable from this repo alone.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Secret storage API | `SecretsService` CRUD against agent-server `/api/settings/secrets`; list returns metadata only | `src/api/secrets-service.ts:26-156` |
| Value-less type | `CustomSecretWithoutValue = Omit<CustomSecret, "value">` | `src/api/secrets-service.types.ts:11-15` |
| Cloud secret store | Paginated `/api/v1/secrets/search`; save split across PUT(rename)/POST(value) because cloud never returns stored values | `src/api/cloud/secrets-service.api.ts:27-118` |
| Exposure modes | `X-Expose-Secrets`: undefined→`"**********"`, `"encrypted"`→cipher, `"plaintext"`→backend-only | `src/api/settings-service/settings-service.api.ts:115-122` |
| Dual cache | Separate redacted/encrypted settings caches, 5-min TTL | `src/api/settings-service/settings-service.api.ts:162-181` |
| No redacted fallback | Conversation start throws rather than starting with broken/redacted credentials | `src/api/settings-service/settings-service.api.ts:478-512` |
| Sole secret channel | Every secret becomes a `LookupSecret {url: /api/settings/secrets/{name}}` + auth headers in `payload.secrets` | `src/api/agent-server-adapter.ts:1203-1228` |
| LookupSecret shape | `{kind: "LookupSecret", url, headers?, description?}` | `src/api/agent-server-adapter.ts:995-1000` |
| Encrypted round-trip | `secrets_encrypted: true` flag gates server-side decryption of Fernet tokens; gated by `hasEncryptedMcpSecrets` | `src/api/agent-server-adapter.ts:1147-1159, 572-591` |
| Client enumeration | `buildStartConversationRequestWithEncryptedSettings` fetches encrypted settings + ALL secrets before launch | `src/api/agent-server-adapter.ts:1254-1293` |
| Deprecated env channel removed | `acp_env` skipped/scrubbed from ACP and OpenHands payloads | `src/api/agent-server-adapter.ts:842-843, 932-935` |
| Single-channel regression tests | "does NOT mirror conversation secrets onto agent_context"; uniform LookupSecret delivery for ACP creds + user secrets | `__tests__/api/agent-server-adapter.test.ts:561-656` |
| Serialization test | Custom secrets serialize as host-relative URL-encoded LookupSecret entries | `__tests__/api/agent-server-adapter.test.ts:473-508` |
| MCP redactor | Collects env/header/auth/oauth/URL-userinfo values, longest-first replace; generic patterns `ghp_*`, `github_pat_*`, `xox*`, `lin_api_*`, JWT, Bearer | `src/utils/redact-mcp-secrets.ts:9-130` |
| Redaction consumers | Probe/test error text scrubbed before display | `src/api/mcp-health/probe-mcp-server-health.ts:33`, `src/api/mcp-service/mcp-service.api.ts:68-121` |
| `<CUSTOM_SECRETS>` backstop | UI re-masks any unmasked `KEY: value` inside CUSTOM_SECRETS blocks incl. truncated blocks | `src/utils/redact-custom-secrets.ts:1-32`; consumer `src/utils/system-message-adapter.ts:26` |
| Placeholder constant | `REDACTED_MCP_SECRET_VALUE = "**********"` | `src/utils/mcp-config.ts:12` |
| Mask-substitution on save | `substituteRedactedMcpCredentials` swaps placeholders back to encrypted stored values; regression for rename-overwrites-secret bug | `src/api/mcp-service/mcp-redacted-credentials.ts:35-127`; tests `__tests__/api/mcp-service/mcp-redacted-credentials.test.ts:14-16,63-92` |
| Secret-free health keys | MCP health identity excludes secret VALUES (plaintext/ciphertext/mask would flap) | `src/utils/mcp-server-health-key.ts:18-41` |
| Provider-token storage rule | Tokens stored only server-side; UI reads `provider_tokens_set` presence flags | `src/types/settings.ts:132`; AGENTS.md "Git provider tokens…" note; `src/hooks/use-user-providers.ts:9-10` |
| Per-provider token lookup | Candidate secret names per git provider (GITHUB_TOKEN/GH_TOKEN/github, …); local-backend only | `src/api/git-provider-items-service.ts:17-54` |
| ACP credential registry | Reserved credentials per provider (ANTHROPIC_API_KEY, CODEX_AUTH_JSON, CLAUDE_CODE_OAUTH_TOKEN, GOOGLE_APPLICATION_CREDENTIALS_JSON…); name doubles as subprocess env var | `src/constants/acp-providers.ts:171-258` |
| Credential conflict map | Mirrors SDK `_ENV_CONFLICT_MAP` (OAuth token vs ANTHROPIC_API_KEY/BASE_URL) | `src/constants/acp-providers.ts:262-276` |
| File-blob credential materialisation | Codex/Gemini blobs materialised via SDK `acp_file_secrets`; cloud uses per-user encrypted store routed through `agent_context.secrets` | `src/hooks/use-acp-credential-form.ts:53-59` |
| Session-key lifecycle | `VITE_SESSION_API_KEY` or window-global injected by static server; `X-Session-API-Key` header builder | `src/api/agent-server-config.ts:102-132, 209-212`; `src/api/backend-registry/auth.ts:9-18` |
| Encryption key ops | Docker entrypoint generates/persists 32-byte `OH_SECRET_KEY` (chmod 600) for settings encryption | `docker/entrypoint.sh:175-191` |
| Env-var surface | Deployment env vars: VITE_BACKEND_BASE_URL, VITE_SESSION_API_KEY, VITE_WORKING_DIR, VITE_AUTH_REQUIRED | `src/api/agent-server-config.ts:98-100, 196-201, 214-221`; AGENTS.md "Supported env vars" |
| Per-conversation workspace | Default `workspace/project/<hex>` dir + `worktree: true` default isolates each run's filesystem | `src/api/agent-server-config.ts:203-207`; `src/api/agent-server-adapter.ts:1131`; `src/api/conversation-service/agent-server-conversation-service.api.ts:457-483` |
| Child isolation modes | `"worktree" \| "shared"`; cloud children always own sandbox | `src/constants/child-conversation.ts:26`; `src/services/child-conversation-launch.ts:158-191` |
| Isolation gap (explicit) | `acp_isolate_data_dir` NOT set: concurrent same-provider ACP conversations can race on shared HOME (TODO #1019) | `src/api/agent-server-adapter.ts:831-836` |
| Secrets UI | Dedicated `/secrets` route + screen tests | `src/routes.ts:31`; `__tests__/routes/secrets-settings.test.tsx:21-26` |
| Redaction unit tests | Masking of configured values, URL-embedded secrets, well-known token shapes without config, placeholder/short-value guards | `__tests__/utils/redact-mcp-secrets.test.ts:6-53` |
| Custom-secrets redaction tests | Idempotency, truncated-block, separator variants | `__tests__/utils/redact-custom-secrets.test.ts:4-61` |
| Live-test artifact hygiene | No secrets in videos/screenshots; Playwright trace capture disabled because setup sends LLM credentials and traces record request bodies | AGENTS.md "Live End-to-End Test Framework" section |

## Answers to Dimension Questions

**Q1: Can the model see secrets?**
By design, no — not through the payload path. Secrets travel as `LookupSecret` references resolved server-side at spawn (`src/api/agent-server-adapter.ts:1203-1228`), and regression tests enforce that no `agent_context.secrets` mirror is synthesized that could bleed into prompt context (`__tests__/api/agent-server-adapter.test.ts:587-620`). The runtime-injected `<CUSTOM_SECRETS>` block inside system-prompt dynamic context arrives masked, and the UI re-masks it defensively (`system-message-adapter.ts:22-27`). However, two residual exposures exist: (a) ACP CLIs legitimately receive credentials as environment variables of the very process the agent drives (`acp-providers.ts:172-176` documents name == env var), so a model running `env` or reading pasted blob files like `~/.codex/auth.json` could retrieve them — inherent to local CLI agents and not countered anywhere in this repo; (b) the browser does fetch plaintext values over authenticated GETs for rename flows and provider-token resolution (`secrets-service.ts:108`, `git-provider-items-service.ts:47-53`), which keeps plaintext within browser memory even though it stays off the conversation wire.

**Q2: Can tools use secrets without exposing them?**
Yes for spawn-time injection: neither the wire payload nor the persisted request carries secret values, only lookups; encrypted settings round-trip as Fernet ciphertext flagged with `secrets_encrypted` (`agent-server-adapter.ts:1147-1159`). MCP credentials are substituted server-side from the store when testing connections (`mcp-service.api.ts` via `substituteRedactedMcpCredentials`, `mcp-redacted-credentials.ts:35-127`). The exception is the browser-side flows above, which are tooling UX (renames, PR-link detection), not model-facing.

**Q3: Are secrets redacted in traces?**
On curated surfaces, yes: MCP probe/test failures (`probe-mcp-server-health.ts:33`, `mcp-service.api.ts:75-83`), system-prompt dynamic context (`system-message-adapter.ts:26`), and health-cache identities exclude values entirely (`mcp-server-health-key.ts:12-16`). Generic token regexes catch values "the browser never held" echoed back in server errors (`redact-mcp-secrets.ts:13-31`). CI policy extends this to artifacts: live E2E forbids rendering credentials in videos/screenshots and disables Playwright trace capture because traces record request bodies containing LLM keys (AGENTS.md, Live E2E section). **Gap:** there is no general-purpose redaction filter over the chat event stream itself — terminal observations and bash output render verbatim; if the model prints a secret to stdout, no frontend filter masks it. I searched for redaction call sites outside the MCP/system-message surfaces and found none.

**Q4: Are identities scoped per user/task?**
Per user, yes: cloud secrets live in a "per-user encrypted secret store" (`use-acp-credential-form.ts:55-58`), and local deployments are single-user behind one generated session key (`docker/entrypoint.sh:198-214`). Per task/conversation, no: `buildStartConversationRequestWithEncryptedSettings` attaches **every** saved custom secret to **every** conversation start (`agent-server-adapter.ts:1269-1274, 1208-1228`); there is no per-conversation secret selection, and child conversations inherit the same global set. No evidence found of task-scoped or TTL-scoped credentials.

## Architectural Decisions

1. **Indirection over transport.** Rather than shipping values in the create-conversation body, secrets are `LookupSecret` URLs the server dereferences at spawn (`agent-server-adapter.ts:1212-1227`). This keeps values out of request logs/wireshark-visible payloads and out of any client-side persistence, at the cost of requiring the resolver to share the trust domain with the store (loopback fetch with session headers).
2. **Explicit exposure tri-state.** Encoding redacted/encrypted/plaintext in one header (`settings-service.api.ts:115-122`) makes "display" and "launch" dataflows distinct types at the API boundary instead of convention.
3. **Fail-closed conversation start.** Encrypted-settings fetch failure aborts launch rather than degrading to masked credentials (`settings-service.api.ts:500-511`) — availability is sacrificed for credential integrity.
4. **Deprecation of inline env channel.** `acp_env` was removed from both payload builders in favor of the secrets panel (`agent-server-adapter.ts:842-843, 932-935`), collapsing provider creds and user secrets into one auditable channel (test: "delivers provider credentials and user secrets uniformly as LookupSecrets", `__tests__/api/agent-server-adapter.test.ts:637-656`).
5. **Frontend-as-backstop redaction.** Because the backend owns masking, the frontend still re-masks `<CUSTOM_SECRETS>` and scrubs MCP errors — accepting duplicated logic to survive backend regressions (`redact-custom-secrets.ts:3-7`).

## Notable Patterns

- **Longest-first replacement ordering** in redaction so substituting a shorter secret cannot corrupt a longer one containing it (`redact-mcp-secrets.ts:92-101`).
- **Placeholder-aware diffing**: sparse MCP patches omit unchanged `**********` leaves so a mask never overwrites the stored credential; regression tests document exactly this historical bug class (`mcp-config.ts` patch helpers; `mcp-redacted-credentials.test.ts:14-16`; `__tests__/utils/mcp-config.test.ts:121,376,407`).
- **Secret-free identity keys**: health-map keys hash structure (env *names*, header *names*) not values, simultaneously avoiding secret storage and cache flapping across representations (`mcp-server-health-key.ts:6-17`).
- **Name-is-env-var coupling**: ACP secret field `name` is deliberately identical to the subprocess environment variable, making the mapping self-documenting (`acp-providers.ts:172-180`).
- **Key rotation resilience**: launcher-generated session keys are re-synced into stored backend entries on startup (`AGENTS.md`, "Key rotation resilience"); encryption and session keys are separate files (`session-api-key.txt` vs `secret-key.txt`).

## Tradeoffs

- **Global attachment vs least privilege**: attaching all secrets to every run simplifies the client but violates minimal-exposure; a compromise in one conversation's context yields the whole vault (mitigated only server-side, unverifiable here).
- **Browser-held plaintext for renames**: because the agent-server exposes only upsert (no read-back on cloud), local edits fetch the old value into the browser to re-upsert (`secrets-service.ts:78-116`); cloud avoids this entirely by never returning values (`cloud/secrets-service.api.ts:75-76`) at the cost of a two-request save with ordering constraints (`secrets-service.api.ts:60-79`).
- **Regex redaction vs completeness**: pattern lists (`ghp_`, `xox`, JWT…) cover popular providers but are inherently enumerable-whack-a-mole; unknown token shapes pass through unless they match Bearer or a configured value (`redact-mcp-secrets.ts:18-31`).
- **Worktree isolation vs env isolation**: filesystem state is well isolated per conversation, but process-level environment (HOME, caches) for ACP agents is shared — the deferred `acp_isolate_data_dir` TODO (`agent-server-adapter.ts:831-836`) shows the team knows this and is blocked on client-version compatibility, a pragmatic but real exposure window.

## Failure Modes / Edge Cases

- **Truncated secret blocks leak-by-default safely**: an unterminated `<CUSTOM_SECRETS>` block is still redacted to end-of-text (`redact-custom-secrets.ts:12-14`, test at `redact-custom-secrets.test.ts:54`).
- **Mask overwrite regression**: renaming a stdio server previously left a literal `**********` to clobber the stored encrypted secret — now guarded and tested (`mcp-redacted-credentials.test.ts:14-16`).
- **Half-applied cloud saves**: cloud rename+value split across endpoints; PUT runs first so a rejected rename cannot destroy an unrecoverable value (`cloud/secrets-service.api.ts:74-78`).
- **Stale encrypted cache**: 5-minute dual cache means a just-rotated key may launch one conversation late (`settings-service.api.ts:162-177`); saves invalidate via `clearCache()`.
- **404-as-success deletion**: deleting a nonexistent secret succeeds silently (`secrets-service.ts:139-154`) — idempotent but hides double-delete bugs.
- **Cross-call regex statefulness avoided**: stateful `g` regexes are constructed per invocation to dodge `lastIndex` carryover (`redact-custom-secrets.ts:9-10`).

## Future Considerations

- Set `acp_isolate_data_dir: true` once the pinned `@openhands/typescript-client` surfaces it (tracked as #1019, `agent-server-adapter.ts:831-836`) to stop concurrent same-provider ACP conversations racing on shared HOME.
- Add per-conversation secret selection (or server-side scoping) instead of blanket attachment of all secrets at launch.
- Consider stream-level redaction for terminal/tool observations, extending the current curated-surface approach.
- Derive reserved file-blob names (`CODEX_AUTH_JSON` etc.) from the client registry instead of duplicating SDK specs (`acp-providers.ts:213-216`).
- Expose a server-side rename endpoint to remove the browser plaintext round-trip in `updateSecret`.

## Questions / Gaps

- **Server-side enforcement unverifiable here.** Whether LookupSecret resolution, Fernet decryption, and `<CUSTOM_SECRETS>` masking actually behave as the frontend assumes lives in `software-agent-sdk` (referenced repeatedly, e.g. `agent-server-adapter.ts:1147-1152`, `use-acp-credential-form.ts:53-58`) and was not inspected per source-isolation rules.
- **No evidence found** of redaction applied to arbitrary observation events (terminal output, file content previews); searched `src/` for redact/mask/sanitize outside MCP/markdown-XSS contexts.
- **No per-task identity scoping** mechanisms found; searched for scoped/token/TTL concepts around secrets — only global user-scope exists.
- Sandbox mount strategy for secrets (e.g., tmpfs/env-file mounts) is decided in the sibling SDK/docker layers; this repo only controls the request shape (`StartConversationPayload.secrets`, `agent-server-adapter.ts:1019`).
- The mock layer models the contract faithfully (list-without-values, expose-header behavior at `src/mocks/secrets-handlers.ts:24-54`, `src/mocks/settings-handlers.ts:997-1128`), which confirms intent but not production behavior.

---

Generated by `Dimension 08.03: Secrets, Identity, and Environment Handling` against `openhands`.
