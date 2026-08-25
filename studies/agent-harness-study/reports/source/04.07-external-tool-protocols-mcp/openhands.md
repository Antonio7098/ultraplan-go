# Source Analysis: openhands

## Dimension 04.07: External Tool Protocols and MCP Integration

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (React Router, Vite, TanStack Query) — the OpenHands "agent-canvas" frontend; MCP protocol execution lives in the sibling agent-server (out of scope per source isolation) |
| Analyzed | 2026-08-23 |

## Summary

This repository is the OpenHands frontend, and its external-tool story is **MCP-first**. The frontend does not speak the MCP wire protocol itself; instead it owns the full configuration, validation, credential-handling, health-probing, and marketplace UX around a backend-executed MCP integration. Servers are configured as name-keyed entries in `agent_settings.mcp_config`, persisted server-side via a settings PATCH diff (`src/api/settings-service/settings-service.api.ts:539-548`), and forwarded at conversation start to both the OpenHands SDK agent and ACP subprocess agents (`src/api/agent-server-adapter.ts:853-860`). Three transports are supported — stdio (spawned command), SSE, and streamable HTTP (`src/utils/mcp-config.ts:60-70`) — with six auth strategies including OAuth2 (`src/types/mcp-auth.ts:18-25`).

The standout engineering is the secret-safety layer: secrets round-trip encrypted server-side, the browser only ever sees redacted placeholders that are substituted from encrypted storage for connectivity tests (`src/api/mcp-service/mcp-redacted-credentials.ts:84-98`), sparse merge patches never transmit unchanged redacted values (`src/utils/mcp-config.ts:319-378`), and error text is scrubbed with both configured values and generic token patterns before display (`src/utils/redact-mcp-secrets.ts:110-130`). A pre-save connectivity probe runs through a dedicated agent-server endpoint (`POST /api/mcp/test`, mocked in `src/mocks/mcp-handlers.ts:20-21`), augmented by catalog-specific read-only tool calls that catch credentials which "list tools succeeds anyway" providers would miss (`src/utils/mcp-credential-validation.ts:52-84`), plus a full browser-mediated OAuth2 authorization flow (`src/api/mcp-service/mcp-service.api.ts:248-312`). Coverage is unusually strong: unit tests across config patching, service behavior, health probing, and UI components, plus two mock-LLM E2E suites under `tests/e2e/mock-llm/mcp/`.

Because this is the frontend of a multi-repo system, the actual protocol client, sandboxing, and tool execution belong to `software-agent-sdk`; within this source's boundary, external tools are neither sandboxed nor permissioned per-tool here — only enabled/disabled per-server (`src/types/mcp-server.ts:24-29`) with conversation-level confirmation policies applied to all tools (`src/api/agent-server-adapter.ts:593-605`). No OpenAPI-to-tool generation exists in this repo; the only OpenAPI references describe the automation service's docs URL advertised in `<RUNTIME_SERVICES>` (`src/api/agent-server-adapter.ts:147,265-266`).

## Rating

**8 / 10** — Clear model with extensive tests, explicit interfaces, and operational safeguards.

Rationale: The MCP surface is a first-class product feature with a coherent architecture (typed client boundary → service layer → React Query mutations → UI), exhaustive secret-redaction discipline, race-guarded health probing, and layered test coverage (unit + component + mock-LLM E2E). It loses points for: (1) the cloud-backend synthetic success short-circuit that skips validation entirely (`src/api/mcp-service/mcp-service.api.ts:190-195`); (2) unsupported header-removal and rename-with-hidden-secrets operations that surface as hard errors rather than supported flows (`src/utils/mcp-config.ts:271-272,403-404`); (3) no per-MCP-tool permission gating visible in this repo; and (4) health verdicts being memory-only by design, so every reload resets servers to "unchecked" (`src/api/mcp-health/mcp-health-store.ts:8-14`).

## Evidence Collected

Every entry cites a file path with line numbers relative to `studies/agent-harness-study/sources/openhands`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| MCP client access | All agent-server MCP calls go through `MCPClient` from `@openhands/typescript-client/clients` — the sanctioned API boundary (CI-enforced by `src/api/no-direct-agent-server-calls.test.ts`) | `src/api/mcp-service/mcp-service.api.ts:1,143-150,198` |
| Transport protocols | `normalizeTransport` accepts `stdio`, `sse`, `streamable-http`; aliases `http`/`shttp` → `http` | `src/utils/mcp-config.ts:60-70` |
| Server config type | `MCPServerConfig`: stdio (`command`/`args`/`env`) or remote (`url`/`headers`), optional `auth`, `timeout`, `enabled` | `src/types/mcp-server.ts:13-29` |
| Auth strategies | `MCP_AUTH_STRATEGIES = ["none","api_key","bearer","basic","header","oauth2"]`; credential shape re-exported from generated typescript-client models | `src/types/mcp-auth.ts:6-25` |
| Standard-schema import | `getSdkMcpServerMap` unwraps both the standard `{mcpServers:{...}}` wrapper and flat SDK maps, so stock MCP config blobs import cleanly | `src/utils/mcp-config.ts:17-32` |
| Config normalization | `parseMcpConfig` rebuilds typed servers field-by-field, dropping malformed candidates silently | `src/utils/mcp-config.ts:77-138` |
| Persistence | MCP config saved inside `agent_settings_diff.mcp_config` on `PATCH /api/settings` | `src/api/settings-service/settings-service.api.ts:539-548` |
| Cloud schema conversion | `cloudCompatibleMcpConfig` converts SDK `auth` credentials into the cloud's header-only storage shape, tombstoning stale headers on strategy switch | `src/api/settings-service/settings-service.api.ts:232-309` |
| Connectivity test endpoint | Backend test spawns stdio / opens SSE-or-SHTTP from the agent-server process; browser cannot reach cloud sandboxes so cloud gets a synthetic ok | `src/api/mcp-service/mcp-service.api.ts:180-210` |
| Test request build | `buildMcpTestRequest` attaches name, timeout (120 s forced for OAuth), and catalog read-only probe `tool_call` | `src/api/mcp-service/mcp-service.api.ts:44-66` |
| Credential verification probes | Catalog-specific read-only tool calls: GitHub `get_me`, Linear `list_teams`, Slack `slack_list_channels {limit:1}` with an interpreter that distinguishes real auth failures from `missing_scope` | `src/utils/mcp-credential-validation.ts:33-99` |
| Probe-gating guard | Credential interpretation only runs when the probe tool is actually advertised in `tools`, degrading to connectivity-only otherwise | `src/api/mcp-service/mcp-service.api.ts:100-122` |
| Redacted-secret substitution | `substituteRedactedMcpCredentials` swaps `**********` placeholders with stored encrypted leaves fetched via `X-Expose-Secrets: encrypted` settings read, so tests exercise real credentials without exposing plaintext | `src/api/mcp-service/mcp-redacted-credentials.ts:34-98` |
| Sparse merge patches | `buildMcpServerPatch` emits minimal diffs: redacted leaves omitted, strategy replacement nulls stale keys, env/header map diffs with explicit `null` deletes | `src/utils/mcp-config.ts:177-193,314-378` |
| Secret redaction on display | `redactMcpSecrets` scrubs configured values longest-first plus generic patterns (GitHub PATs, Slack xox tokens, Linear keys, JWTs, Bearer headers) from error/tool-result text | `src/utils/redact-mcp-secrets.ts:18-31,91-130` |
| OAuth flow | `authorizeOAuth` opens a popup, polls job status (250 ms until callback-ready, then 1 s up to 120 s), navigates popup to `authorization_url`, submits callback URL | `src/api/mcp-service/mcp-service.api.ts:212-312` |
| Health probing | `probeMcpServerHealth` maps test responses to health states; `verified` requires advertised probe tool invoked without error, else truthfully labeled `connectivity-only` | `src/api/mcp-health/probe-mcp-server-health.ts:38-88` |
| Auth-failure sniffing | Conservative regex upgrades `connection`/`unknown` failures mentioning 401/403/unauthorized/etc. to `credentials` kind | `src/api/mcp-health/probe-mcp-server-health.ts:16-23,51-58` |
| Health store race guards | `resolveMcpHealthCheck` drops results whose checkId no longer matches, so slow stale probes never overwrite newer verdicts; store deliberately not persisted | `src/api/mcp-health/mcp-health-store.ts:7-14,43-60` |
| Per-server enable/disable | Explicit-false `enabled` flag ("Absent means enabled"); toggling builds an enabled-only patch | `src/types/mcp-server.ts:24-29`, `src/utils/mcp-config.ts:167-175` |
| Marketplace catalog | Shared `INTEGRATION_CATALOG` imported from `@openhands/extensions/integrations`; filtered to MCP-installable entries; transport/auth shapes converted to MCP configs | `src/utils/mcp-marketplace-utils.ts:6-12,110-124` |
| OAuth installability filter | Entries requiring provider-side OAuth endpoints (authorization/token/registration URLs) are excluded from local installs — only fastmcp-initiated flows supported | `src/utils/mcp-marketplace-utils.ts:45-70` |
| Client-auth-method conversion | Catalog `clientAuthentication` none/body/basic mapped to `none`/`client_secret_post`/`client_secret_basic` | `src/utils/mcp-marketplace-utils.ts:78-108` |
| Runtime catalog patching | `patchLinearEntry` (SSE→streamable HTTP) and `patchGitHubEntry` (docker→native stdio binary in Docker deployments) mutate catalog entries environment-conditionally | AGENTS.md (repository notes); matching helpers in `src/utils/mcp-marketplace-utils.ts:110-124` |
| Conversation forwarding | Non-empty `mcp_config` forwarded on conversation start for ACP agents; empty/malformed dropped; global `mcp_config` stamped by agent-server onto profile-resolved agents too | `src/api/agent-server-adapter.ts:853-860,1100-1106` |
| Encrypted MCP secrets at start | `secrets_encrypted` flag set when ACP config carries Fernet-encrypted `mcp_config` secrets so the server decrypts them at conversation start | `src/api/agent-server-adapter.ts:1147-1159`, `hasEncryptedMcpSecrets` at `585-591` |
| Tool permissioning (all tools) | `confirmation_policy` NeverConfirm/AlwaysConfirm/ConfirmRisky(HIGH, confirm_unknown) plus LLMSecurityAnalyzer/PatternSecurityAnalyzer/PolicyRailSecurityAnalyzer chosen per conversation settings | `src/api/agent-server-adapter.ts:593-618,1120-1121` |
| Built-in tool gating contrast | Browser/task tool sets gated by env flag, `/server_info` `usable_tools` metadata, and sub-agent setting — a pattern MCP servers do not get individually | `src/api/agent-server-adapter.ts:114-115,631-644,945` |
| Plugin manifests (skills, not tools) | Installed plugins carry `install_path`, bundled `skills`, `files`; managed on local agent-server only (`~/.openhands/plugins/installed/`), auto-load into conversations; cloud returns empty list | `src/api/plugins-management-service.ts:13-33,65-75` |
| Protocol tests (service) | 18 service tests: spec mapping, probe attachment, Slack/GitHub/Linear credential interpretation, redaction, substitution, cloud short-circuit | `src/api/mcp-service/mcp-service.api.test.ts:71-395` |
| Protocol tests (config) | 15+ tests: stable map keys, wrapper normalization, redaction-safe sparse patches, header/OAuth patch semantics, rename rejection with hidden secrets | `__tests__/utils/mcp-config.test.ts:15-464` |
| Health tests | Store race-guard tests and probe interpretation tests | `__tests__/api/mcp-health/mcp-health-store.test.ts`, `__tests__/api/mcp-health/probe-mcp-server-health.test.ts` |
| E2E MCP specs | Mock-LLM E2E: GitHub hosted-MCP install/edit/delete verifying persisted `mcp_config.github` shape; Slack invalid-credential suite proving `invalid_auth` blocks install while `missing_scope` passes and edit tests stored (not placeholder) credentials | `tests/e2e/mock-llm/mcp/mock-llm-mcp-github.spec.ts:1-300`, `tests/e2e/mock-llm/mcp/mock-llm-mcp-slack-credentials.spec.ts:1-30` |

## Answers to Dimension Questions

**1. Can tools live outside the process?**
Yes — that is the primary mode. MCP servers are by definition external processes/endpoints: stdio commands spawned by the agent-server (`command`/`args`/`env`, `src/types/mcp-server.ts:20-22`), or remote SSE/SHTTP endpoints reached by URL (`src/types/mcp-server.ts:16-18`). The frontend's own comment documents that the test endpoint "spawns the configured stdio command / opens an SSE-or-SHTTP connection from that process's environment" (`src/api/mcp-service/mcp-service.api.ts:183-192`) — i.e., out-of-process execution is delegated to the backend. Additionally, built-in client tools can live outside the server: conversations register `client_tools` executed browser-side (`src/api/agent-server-adapter.ts:1111-1119`).

**2. Are external tools trusted by default?**
Largely yes, after install. Any well-formed custom server can be added without review (`CustomServerEditor` add flow, `src/routes/mcp.tsx:129-133`; panel add at `src/components/features/conversation/conversation-overview-mcp-panel.tsx:63-75`), and once installed, its tools are available to the agent subject only to conversation-level confirmation policy (`NeverConfirm` default, `src/api/agent-server-adapter.ts:596-597`). There is no per-server or per-tool trust tier, allowlist, or capability negotiation visible in this repo; the only opt-outs are disabling an entire server (`enabled: false`, `src/types/mcp-server.ts:24-29`) or enabling confirmation mode/security analyzers globally (`src/api/agent-server-adapter.ts:593-618`). The safety investment is concentrated on credential confidentiality, not tool-behavior containment — consistent with enforcement living in the agent-server (outside this source).

**3. How are schemas imported?**
Two paths. (a) Whole configs: `parseMcpConfig` accepts standard `{mcpServers:{name:{...}}}` blobs and the SDK flat map, normalizing transports and coercing only well-typed fields (`src/utils/mcp-config.ts:17-32,77-138`) — so stock MCP JSON imports directly. (b) Marketplace entries: shared catalog data from `@openhands/extensions/integrations` is converted into `MCPServerConfig`s, including enum-mapped OAuth client-auth methods (`src/utils/mcp-marketplace-utils.ts:78-108`) and installability filtering for provider-OAuth entries (`45-70`). Notably, *tool* schemas are not imported client-side at all: the frontend only receives tool *names* from test probes (`tools: string[]`, `src/types/mcp-server.ts:50`); JSON schemas stay server-side.

**4. How are failures isolated?**
Well, for management-plane failures. Probes run through a state machine where each check gets a monotonic `checkId`; a result commits only if its entry is still the matching `checking` state, so slow/stale probes cannot clobber fresh verdicts (`src/api/mcp-health/mcp-health-store.ts:43-60`). Failures are classified (`connection`/`credentials`/`timeout`/...) with heuristic upgrade of handshake auth errors (`src/api/mcp-health/probe-mcp-server-health.ts:51-58`), and credential misinterpretation is prevented by gating interpretation on tool advertisement (`src/api/mcp-service/mcp-service.api.ts:106-120`). Error text is redacted before display (`src/utils/redact-mcp-secrets.ts:110-130`). However, runtime failures of external tools during a conversation are not isolated in this repo — they surface through the generic event stream; there is no MCP-specific circuit breaker or quarantine logic here.

**5. Can the same tool work across clients?**
Yes within the product's own ecosystem. Because `mcp_config` persists server-side in `agent_settings` (not localStorage), any frontend sharing the agent-server sees the same servers, and the adapter explicitly forwards the same config to both OpenHands SDK agents and ACP subprocess agents (`src/routes/mcp.tsx:33-38` design note; `src/api/agent-server-adapter.ts:853-860`). The config format itself follows the de-facto standard MCP map shape, aiding portability to other MCP hosts (`src/utils/mcp-config.ts:22-32`). Cross-*vendor* portability is partial: cloud backends force header-only credential storage via `cloudCompatibleMcpConfig` (`src/api/settings-service/settings-service.api.ts:232-254`), meaning rich auth strategies degrade to headers on that path, and the marketplace catalog is OpenHands-specific packaging even though the underlying servers are standard.

## Architectural Decisions

1. **Thin-client protocol delegation.** The frontend never implements MCP; it composes typed calls to `MCPClient` (`src/api/mcp-service/mcp-service.api.ts:1,143-150`) under a CI-enforced rule banning direct agent-server HTTP (`AGENTS.md` API Access Rules; `src/api/no-direct-agent-server-calls.test.ts`). Protocol evolution is pushed to `typescript-client`/`software-agent-sdk`.
2. **Backend-executed validation probe.** Pre-save testing is an agent-server endpoint (`POST /api/mcp/test`, `src/mocks/mcp-handlers.ts:20-21`) rather than browser-side connection attempts — necessary since browsers cannot spawn processes, and it means validation exercises the exact environment the runtime will use (`src/api/mcp-service/mcp-service.api.ts:183-192`).
3. **Redaction-placeholder secret lifecycle.** Stored secrets render as `**********` (`REDACTED_MCP_SECRET_VALUE`, `src/utils/mcp-config.ts:12`); edits build sparse patches that omit unchanged placeholders (`buildMcpServerPatch`, `319-378`), while tests substitute encrypted stored leaves just-in-time (`mcp-redacted-credentials.ts:77-98`). Plaintext never needs to exist in the browser.
4. **Catalog-aware semantic verification.** Rather than treating "tools/list succeeded" as healthy, catalog entries declare read-only probe tool calls with per-provider interpreters that understand vendor error codes (`src/utils/mcp-credential-validation.ts:28-84`), fixing false positives like Slack advertising tools under any token.
5. **Name-keyed stable identity.** Settings map keys are the persistence identity (`// @spec MCP-003`, `src/utils/mcp-config.ts:76-77`), with renames modeled as delete+insert in one patch and blocked when secrets are hidden (`403-428`) — trading convenience for integrity.
6. **Shared extension package.** Marketplace/skills/plugin catalogs come from the `@openhands/extensions` npm package consumed at build time (`src/utils/mcp-marketplace-utils.ts:6-12`), decoupling catalog content releases from frontend releases.

## Notable Patterns

- **Truthful degradation labels:** health states distinguish `verified` from `connectivity-only` instead of upgrading partial success (`src/api/mcp-health/probe-mcp-server-health.ts:41-45,60-72`).
- **Environment-conditioned catalog patching:** deployment-mode-specific rewrites (Linear SSE→HTTP, GitHub docker→bundled binary) applied immutably before UI consumption (documented in `AGENTS.md` repository notes; filtering helpers in `src/utils/mcp-marketplace-utils.ts`).
- **Defensive parsing everywhere:** `isRecord` guards and field-by-field coercion mean malformed persisted blobs drop silently rather than crashing the settings page (`src/utils/mcp-config.ts:82-137`).
- **Popup-mediated OAuth with polling state machine:** open popup blank → poll until callback ready → navigate popup to authorization URL → bounded 120 s polling loop (`src/api/mcp-service/mcp-service.api.ts:248-312`).
- **Spec-tagged tests:** `// @spec MCP-002/MCP-003` comments tie tests to stable requirement IDs (`src/utils/mcp-config.ts:76,318,406`).
- **MSW mock parity:** dedicated `MCP_HANDLERS` mock the test endpoint for dev/test modes (`src/mocks/mcp-handlers.ts:14-21`).

## Tradeoffs

- **Cloud convenience vs. validation honesty:** cloud users get a synthetic `ok: true` with zero tool discovery at install time (`src/api/mcp-service/mcp-service.api.ts:190-195`) — saving always succeeds, deferring all failure discovery to conversation runtime.
- **Memory-only health:** verdicts reset to "unchecked" on reload by explicit design (`src/api/mcp-health/mcp-health-store.ts:8-14`) — avoids stale "healthy" claims at the cost of repeated probing.
- **Strictness vs. flexibility in edits:** removing one header from header-auth throws (`MCP_HEADER_REMOVAL_ERROR`, `src/utils/mcp-config.ts:271-289`) and renaming with hidden secrets is refused (`403-419`) — safe but user-hostile edges acknowledged in-product.
- **Centralized confirmation vs. per-tool granularity:** one policy knob covers all actions including external MCP tools (`src/api/agent-server-adapter.ts:593-605`); fine-grained trust would require server-side support absent here.
- **Silent normalization vs. surfacing drift:** `parseMcpConfig` quietly drops unrecognized fields/shapes (`src/utils/mcp-config.ts:83,109-111`) — resilient but can mask config corruption.

## Failure Modes / Edge Cases

- **Older backends:** missing `tool_result` in test responses skips credential interpretation entirely, preserving legacy behavior (`src/api/mcp-service/mcp-service.api.ts:213-220` test; guard at `100-122`); missing `enabled` field treated as enabled (`src/utils/mcp-config.ts:55-58`).
- **Hosted variants of catalog servers:** if a server doesn't advertise the expected probe tool, the entry degrades to connectivity-only rather than misreporting bad credentials (`src/api/mcp-service/mcp-service.api.ts:92-98` docstring, `106-111` implementation).
- **Encrypted-settings fetch failure during test:** substitution fails closed to the placeholder, and the test proceeds with the literal placeholder (documented behavior: keeps placeholder when fetch fails, `src/api/mcp-service/mcp-service.api.test.ts:360`).
- **Duplicate installs colliding in health keys:** health keys exclude secret values, so two installs of one catalog entry collide until naming suffixes; seeding is skipped defensively (`src/api/mcp-health/probe-mcp-server-health.ts:118-134`).
- **Stale probe races:** mid-flight server edits/deletes invalidate pending checks via checkId mismatch (`src/api/mcp-health/mcp-health-store.ts:43-60`).
- **OAuth timeout:** bounded 120 s loop ends in an explicit `error_kind: "timeout"` (`src/api/mcp-service/mcp-service.api.ts:288-308`); popup blockers handled via optional chaining on `popup`.
- **Cloud + ACP encryption interplay:** `secrets_encrypted` only forced for ACP when MCP secrets are Fernet-encrypted, avoiding unnecessary cipher requirements (`src/api/agent-server-adapter.ts:1147-1159`).

## Future Considerations

- Per-server/per-tool permission tiers for MCP tools, mirroring the `usable_tools` gating already applied to built-in browser/task tool sets (`src/api/agent-server-adapter.ts:631-644`).
- Closing the cloud validation gap: queue a deferred first-use health probe for cloud-installed servers instead of a blanket synthetic success.
- Supporting individual header removal in header-auth and rename-with-stored-secrets (both currently hard errors, `src/utils/mcp-config.ts:271-272,403-404`).
- Persisting last-known health with explicit staleness windows rather than pure in-memory state, if probing cost becomes a concern.
- Surfacing MCP tool schemas (not just names) client-side to enable richer pre-approval UIs.

## Questions / Gaps

- **Where exactly is the MCP session established at runtime?** Out of this source's boundary (software-agent-sdk). Searched `src/` for MCP client/session implementations beyond the test/probe path: none found — the only MCPClient usage is `MCPClient` construction against the agent-server's management endpoints (`src/api/mcp-service/mcp-service.api.ts:143-150,198`).
- **Is there OpenAPI→tool generation?** No evidence found. Searches for "openapi" across `src/` returned only the automation service's docs/OpenAPI URL strings used in the `<RUNTIME_SERVICES>` prompt block (`src/api/agent-server-adapter.ts:147,265-266`) and a Jira-catalog remark (`src/utils/mcp-marketplace-utils.ts:119`). The dimension's OpenAPI question is answered negatively for this repo.
- **Do installed plugins contribute tools?** Plugins bundle *skills* and files, not MCP/tool definitions (`InstalledPluginInfo.skills/files`, `src/api/plugins-management-service.ts:13-25`); they auto-load into conversations but as skills context. No evidence found of plugin-declared tools.
- **Does anything enforce that disabled (`enabled: false`) servers are excluded at runtime?** The frontend persists the flag and the agent-server stamps global config (`src/api/agent-server-adapter.ts:1100-1106` comment), but exclusion semantics live server-side; not verifiable within this source.

---

Generated by `Dimension 04.07: External Tool Protocols and MCP Integration` against `openhands`.
