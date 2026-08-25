# Source Analysis: opa

## Dimension 08.03 — Secrets, Identity, and Environment Handling

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go 1.2x; policy engine with embedded Rego evaluator, HTTP server, plugin framework |
| Analyzed | 2026-08-24 |

All citations below are relative to the selected source directory `studies/agent-harness-study/sources/opa`.

## Summary

OPA is a policy engine rather than an agent harness, so this dimension maps onto three "tool" surfaces: (1) OPA's own outbound service clients (bundle/discovery/status/decision-log plugins), (2) the policy-authored `http.send` / `io.aws.sign_req` builtins, and (3) OPA's inbound HTTP API. Secret handling for outbound clients is mature and well tested: a per-service `credentials` block (`v1/plugins/rest/rest.go:61-69`) selects exactly one of bearer token (inline or file), OAuth2 client credentials/JWT-bearer, mTLS, AWS SigV4, GCP metadata, or Azure managed identity (`v1/plugins/rest/rest.go:88-121`). Secrets can be kept out of the config file via `token_path` file reads on every request (`v1/plugins/rest/auth.go:99-124`), cloud metadata services (`v1/plugins/rest/gcp.go:86-107`, `v1/plugins/rest/azure.go:97-110`), and KMS/KeyVault-backed signing so private key material never lands in config (`v1/plugins/rest/auth.go:378-428`). Redaction is layered but mostly opt-in: decision logs get a policy-driven mask/drop framework (`v1/plugins/logs/plugin.go:1048-1141`, default paths `/system/log/mask` and `/system/log/drop` at `plugin.go:272-273`); REST debug logs hard-redact `Authorization` and `X-Amz-Security-Token` headers (`rest.go:36-39`, `rest.go:397-403`); and `opa.runtime().config` strips service credentials and key material before policies see it (`v1/topdown/runtime.go:52-111`). Two deliberate exposures stand out: the full process environment is readable by any policy via `opa.runtime().env` (`v1/runtime/info/info.go:47-58`, documented at `docs/docs/faq.md:374-385`), and the authorization policy receives raw request headers including `Authorization` (`v1/server/authorizer/authorizer.go:192-197`). There is no generic secret-provider abstraction (Vault/Secrets Manager) and no sandboxing or per-run environment isolation; identity is process-wide, not scoped per task.

## Rating

**7 / 10** — A clear, explicit model with extensive tests and real operational safeguards (per-request credential injection, header redaction in debug logs, credential purging from `opa.runtime`, opt-in mask/drop policies with recorded audit trails). It falls short of 8–10 because secret storage still defaults to inline config values (no generic secret-provider extension point beyond custom auth plugins), bundle-signing `private_key` material can only be configured inline (`v1/keys/keys.go:26-32`), `opa.runtime().env` exposes all environment secrets to every policy by design, and masking/drop are opt-in rather than fail-closed defaults.

## Evidence Collected

Every entry cites a path with line numbers relative to `studies/agent-harness-study/sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Credential config schema | `Config.Credentials` supports `bearer`, `oauth2`, `client_tls`, `s3_signing`, `gcp_metadata`, `azure_managed_identity`, custom `plugin` | `v1/plugins/rest/rest.go:61-69` |
| Single-method enforcement | "a maximum one credential method must be specified" error when multiple set | `v1/plugins/rest/rest.go:104-115` |
| Custom auth plugin registry | `Manager.AuthPlugin(name)` resolves registered `rest.HTTPAuthPlugin`s | `v1/plugins/plugins.go:740-749`; selection at `v1/plugins/rest/rest.go:90-102` |
| Bearer token from file | `token_path` read per request (`os.ReadFile`), enabling k8s secret mounts and rotation | `v1/plugins/rest/auth.go:99-124`; struct fields `auth.go:62-71`; mutual-exclusion error `auth.go:82-84` |
| OAuth2 client credentials | `client_secret`, `signing_key`, `aws_kms`, `azure_keyvault`, `client_assertion[_path]` mutually exclusive; token URL forced https | `v1/plugins/rest/auth.go:236-261`, `512-514`, `515-571` |
| OAuth token caching | Access token cached until <10 s before expiry, then re-fetched per request | `v1/plugins/rest/auth.go:696-708` |
| AWS env credentials | Reads `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`/`AWS_SECURITY_TOKEN`, `AWS_REGION` | `v1/plugins/rest/aws.go:49-59`, `77-101` |
| AWS provider chain | Ordered like the AWS SDK default chain: env → assume-role → web-identity → profile → metadata → SSO | `v1/plugins/rest/auth.go:809-839`; ECS container env vars `v1/plugins/rest/aws.go:37-41`; IMDS endpoints `aws.go:31-34` |
| GCP/Azure managed identities | Tokens fetched from metadata endpoints per request; GCP access/identity token paths configurable | `v1/plugins/rest/gcp.go:86-107` (default path `gcp.go:19`); Azure plugin `v1/plugins/rest/azure.go:97-110` |
| KMS / KeyVault signing | JWT client assertions signed by AWS KMS or Azure KeyVault so private keys stay out of OPA config | `v1/plugins/rest/auth.go:377-406` (KMS), `408-428` (KeyVault) |
| mTLS client certs | Cert/key/passphrase config with periodic cert re-read (`cert_reread_interval_seconds`) for rotation | `v1/plugins/rest/auth_tls.go:90-120`; CA cert read from file `auth_tls.go:43-64`; deprecation shim `auth_tls.go:219` |
| Redirect hygiene | No `Authorization` attached on 307/308 redirects; AWS signing skipped when host changes mid-redirect | `v1/plugins/rest/auth.go:117-122`, `866-872` |
| Debug-log redaction | `Authorization` + `X-Amz-Security-Token` replaced with `REDACTED` before debug logging requests | `v1/plugins/rest/rest.go:36-39`, `365`, `397-403`; test `TestDebugLoggingRequestMaskAuthorizationHeader` at `v1/plugins/rest/rest_test.go:1968-2021` |
| `opa.runtime` credential purge | `services[].credentials` and `keys[].key/private_key` deleted before config reaches policies | `v1/topdown/runtime.go:52-111`; test `TestOPARuntimeConfigMasking` at `v1/topdown/runtime_test.go:48-86` |
| Env exposure to policies | `opa.runtime().env` inserts every `NAME=VALUE` from `os.Environ()` into an AST object returned to policy eval | `v1/runtime/info/info.go:47-58`; documented pattern `docs/docs/faq.md:374-385` |
| Decision log mask/drop | `data.system.log.mask` erase/upsert rules and `data.system.log.drop` gate evaluated per event before upload/console | `v1/plugins/logs/plugin.go:1048-1099` (mask), `1102-1141` (drop), defaults `plugin.go:272-273`, pipeline order `plugin.go:766-798` |
| Mask ops & audit trail | `remove`/`upsert` JSON-pointer ops; erased/masked paths recorded on the event | `v1/plugins/logs/mask.go:18-22`, `128-199`; `EventV1.Erased/Masked` fields `v1/plugins/logs/plugin.go:64-65`; docs `docs/docs/management-decision-logs.md:128-249` |
| ND-builtin-cache masking | http.send response cache attachable to events only when enabled; maskable under `/nd_builtin_cache/...` | Config flag `v1/plugins/logs/plugin.go:314`; tests `v1/plugins/logs/plugin_test.go:2602-2625` |
| Request-context allowlist | Only explicitly configured headers enter decision-log events (`request_context.http.headers`) | `v1/plugins/logs/plugin.go:295-301`, `745-759` |
| Inbound authn schemes | `--authentication` = token/tls/off; bearer regex extracts token as identity; TLS CN as identity + client certs surfaced | `cmd/run.go:63`, `286-288`; `v1/server/identifier/token.go:20-33`; `v1/server/identifier/tls.go:23-31`; wiring `v1/server/server.go:782-791`, TLS RequireAndVerifyClientCert `server.go:701-703` |
| Authz input contents | Policy sees method/path/params/**headers**/body/identity/client_certificates | `v1/server/authorizer/authorizer.go:169-227` (headers at 192-197) |
| Authn-without-authz warning | Runtime errors that token auth alone is ineffective | `v1/runtime/runtime.go:680-682` |
| Cache-key isolation | http.send cache keys include all headers unless `cache_ignored_headers` lists them | `v1/topdown/http.go:260-291` |
| SigV4 output exposure | `io.aws.sign_req` returns signed headers incl. `x-amz-security-token` session token to the caller | `internal/providers/aws/signing_v4.go:131-136`; inserted into result `v1/topdown/providers.go:190-196` |
| Bundle signing keys inline-only | `keys.Config{Key, PrivateKey}` come straight from config JSON; public `key` may be a file path, `private_key` may not | `v1/keys/keys.go:26-32`, `58-78` |
| Env-var surface | Only a handful of OPA_* variables (`OPA_LOG_TIMESTAMP_FORMAT`, `OPA_DECISIONS_INTERMEDIATE_RESULTS`, version-check override, internal `HTTP_SEND_TIMEOUT`) | `cmd/run.go:342`; `v1/server/server.go:105`; `internal/versioncheck/versioncheck.go:82`; `v1/topdown/http.go:317-330` |
| Local isolation control | Unix domain socket listener with configurable permissions | `v1/server/server.go:763-776`; flag `cmd/run.go:228` |

## Answers to Dimension Questions

1. **Can the model see secrets?** The "model" analog in OPA is policy code/data. Policies cannot retrieve service credentials through `opa.runtime().config` — those blocks are stripped first (`v1/topdown/runtime.go:54-66`, verified by `v1/topdown/runtime_test.go:48-86`). However, any secret present in process environment variables **is** visible to every policy via `opa.runtime().env` (`v1/runtime/info/info.go:49-56`), which the docs actively recommend as a way to inject certificates (`docs/docs/faq.md:374-385`). Additionally, nothing prevents authors from embedding secrets directly in Rego source, data documents, or `http.send` header maps — there is no scanning or prevention mechanism (no evidence found of any such filter).
2. **Can tools use secrets without exposing them?** Yes for OPA's own outbound clients: tokens live behind `token_path` files (`v1/plugins/rest/auth.go:105-111`), cloud metadata services, or KMS/KeyVault signing operations where the key never leaves the provider (`v1/plugins/rest/auth.go:378-428`), and they are injected only at request time inside `Prepare()` (`v1/plugins/rest/rest.go:48-50`). For policy-driven `http.send`, no: headers including credentials must be authored inline in policy or data — there is no secret-reference indirection (searched `v1/topdown/http.go` for any resolver/secret hook; none exists). Partial exception: `io.aws.sign_req` keeps static keys out of policy but returns ephemeral session tokens in its result headers (`internal/providers/aws/signing_v4.go:133-136`, `v1/topdown/providers.go:190-196`).
3. **Are secrets redacted in traces?** Layered, inconsistent. Automatic: REST client debug logs (`v1/plugins/rest/rest.go:36-39`) and `opa.runtime` config purge. Opt-in: decision-log masking/drop policies (`v1/plugins/logs/plugin.go:1048-1141`) with an audit trail of erased/masked pointers (`plugin.go:64-65`). Not redacted: topdown query trace events (`opa eval --explain full`) carry full expression operands, and no evidence found of any filtering there (searched `v1/topdown/trace.go` for header/secret handling — none); authz-policy input includes the raw `Authorization` header value (`v1/server/authorizer/authorizer.go:196`), though authz decisions bypass the decision logger entirely.
4. **Are identities scoped per user/task?** Inbound, per-request identities exist: bearer token string or TLS certificate CN become `input.identity`, plus peer certificates for the authz policy (`v1/server/identifier/token.go:26-29`, `v1/server/identifier/tls.go:26-27`, consumed at `v1/server/authorizer/authorizer.go:216-225`). Outbound, identity is process-wide: one credential chain per named service shared by all evaluations; the inter-query cache is global across queries (keyed by request content, `v1/topdown/http.go:260-291`). There is no notion of per-task/per-decision identity separation anywhere in the plugin manager or SDK (no evidence found).

## Architectural Decisions

- **Credentials belong to services, not to callers**: each `services[_]` entry carries one credential block chosen reflectively, with an explicit error if two are set (`v1/plugins/rest/rest.go:104-115`). This makes the trust boundary per remote service and keeps auth logic out of plugins that consume clients.
- **Per-request preparation over per-client state**: the `HTTPAuthPlugin` interface mandates per-request work in `Prepare()` (`v1/plugins/rest/rest.go:41-51`), which is what enables file-based token rotation without restart (`auth.go:105-111`) and redirect-safe header attachment (`auth.go:117-122`).
- **Delegate secret custody to infrastructure**: rather than building a secret-provider SPI, OPA relies on file mounts, EC2/ECS/GCP/Azure metadata services, and sign-in-place KMS/KeyVault integrations (`v1/plugins/rest/aws.go:31-59`, `auth.go:377-428`) — the environment is treated as the secret store.
- **Policy-driven, auditable log sanitization**: masking and dropping are themselves Rego decisions (`system.log.mask`, `system.log.drop`), evaluated with prepared queries and invalidated on compiler updates (`v1/plugins/logs/plugin.go:894-900`), with applied pointers recorded on each event for downstream audit (`mask.go:128-199`).
- **Deny-by-default surface, allow-by-default exposure**: request-context headers require explicit configuration to enter events (`plugin.go:745-759`), while `opa.runtime().env` exposes everything unless operators curate the environment themselves (`info.go:47-58`).

## Notable Patterns

- **Credential chain mirroring**: `awsCredentialServiceChain` reproduces the AWS SDK default provider order and collects per-provider errors instead of failing fast (`v1/plugins/rest/auth.go:773-794`, `796-840`).
- **Redaction at the logging boundary only**: `withMaskedHeaders` clones headers and overwrites a fixed denylist at debug time, leaving the live request untouched (`rest.go:397-403`) — simple, but the denylist is hardcoded (custom sensitive headers like `X-Vault-Token` are not covered).
- **Prepared-query caching for hot-path policies**: mask and drop queries are prepared once and reused; failures to prepare do not retry infinitely (`plugin.go:1048-1073`, regression test `plugin_test.go:2939-2945`).
- **Purge-at-read for runtime introspection**: instead of maintaining two configs, `opa.runtime` deletes sensitive keys lazily during evaluation (`topdown/runtime.go:22-46`).
- **Opt-in cache-key widening/shrinking**: `cache_ignored_headers` lets authors deliberately exclude volatile or sensitive headers from cache identity (`http.go:262-291`).

## Tradeoffs

- **File/env-based secret indirection vs. a secret-provider API**: zero dependencies and works everywhere (k8s secret mounts, instance profiles), but enterprises with centralized vaults must write Go plugins (`plugins.go:740-749`) or proxy through their own service.
- **Inline defaults vs. safety**: `bearer.token`, oauth2 `client_secret`, and keys `private_key` remain valid inline config (`auth.go:63`, `240`; `keys.go:29`), maximizing compatibility while leaving leak-prone paths open (config files, discovery bundles, `gitops` repos).
- **Policy-controlled masking vs. automatic scrubbing**: masking is arbitrarily precise (it can even upsert redacted placeholders) but silent if unconfigured — a deployment without mask rules ships full inputs, results, and (if enabled) nd_builtin_cache payloads.
- **Env-as-feature vs. env-as-risk**: `opa.runtime().env` is genuinely useful for cert/config injection (`docs/docs/faq.md:381-385`) yet means any policy author can read every env var, including ones meant for other components sharing the container.

## Failure Modes / Edge Cases

- **Mask-rule evaluation failure fails soft**: if `maskEvent` errors, the event is logged/uploaded *unmasked* (only an error line appears) — `p.logger.Error("Log event masking failed") ... return nil` swallows the failure (`v1/plugins/logs/plugin.go:785-788`); same for drop-eval errors continuing unfiltered (`plugin.go:766-770`). A broken mask policy degrades to full-fidelity leaks rather than dropped logs.
- **Hardcoded debug-redact denylist**: only `Authorization` and `X-Amz-Security-Token` are redacted (`rest.go:36-39`); custom headers carrying credentials (e.g., proxy keys) hit debug logs verbatim, though non-listed headers in the test remain plaintext intentionally (`rest_test.go:2013-2019`).
- **Session-token return in `io.aws.sign_req`**: the resulting header map embeds `x-amz-security-token` (`internal/providers/aws/signing_v4.go:133-136`), which then flows wherever the policy puts it — including decision-log results if returned.
- **Authz input contains raw credentials**: `input.headers.Authorization` is available to the authz policy (`authorizer.go:192-197`); a careless authz policy returning it (e.g., in `reason`) echoes it to unauthorized callers via the 401 body path (`authorizer.go:146-158`).
- **Token-auth-without-authz foot-gun**: mitigated by a startup error message (`runtime.go:680-682`), but the configuration itself is accepted.
- **Global inter-query cache across identities**: responses fetched under one principal's credentials are reusable by other evaluations only if the request object matches; authors who move credentials out of headers into URLs or bodies (or ignore headers via `cache_ignored_headers`, `http.go:265-290`) can accidentally cross identity boundaries.

## Future Considerations

- Introduce a general secret-reference mechanism (e.g., `secret://` URIs or a provider SPI in `Config.Credentials`) so `http.send`, decision-log service config, and `keys.private_key` stop requiring inline material; today only custom `credentials.plugin` implementations cover this gap (`rest.go:90-102`).
- Fail closed on mask/drop evaluation errors (drop the event, count it in metrics — counters like `decision_logs_encoding_failure` already exist at `plugin.go:277`) instead of uploading unmasked content.
- Extend the debug-log denylist to a configurable header-redaction set alongside `maskedHeaderKeys`.
- Consider excluding or hashing credential-bearing headers from `io.aws.sign_req`'s returned header map, or documenting the exposure next to the builtin's schema.
- Provide an operator-level switch to restrict `opa.runtime().env` (allowlist or omit-values mode), preserving the FAQ use case while limiting blast radius.

## Questions / Gaps

- **No generic secret provider integration**: searches for Vault/Secrets Manager-style providers across `v1/plugins` and `internal/providers` found only Azure KeyVault signing and AWS KMS signing (`v1/plugins/rest/auth.go:137-144`, `132-135`). If deeper integrations exist, they would have to be external plugins.
- **Trace-event redaction**: no evidence found of any redaction in topdown trace output; searched `v1/topdown/trace.go` and `http.go` for trace-time filtering of request operands. Whether `opa eval --explain full` output containing `http.send` header objects is considered in-scope for sanitization could not be determined from the repository alone.
- **Discovery-served config secrecy**: credentials delivered via discovery bundles transit the same rest client machinery, but whether upstream users commonly place inline secrets in discovery configs (and how OPA might warn about it) has no in-repo guard found — no validation rejects inline `token`/`private_key` values.
- **Per-decision identity propagation**: the SDK exposes plugin managers per instance (`sdk/` package), but no evidence was found of per-request outbound identity switching (e.g., impersonation headers per evaluated query); scoping remains the embedder's responsibility.

---

Generated by dimension 08.03 (Secrets, Identity, and Environment Handling) against `studies/agent-harness-study/sources/opa`.
