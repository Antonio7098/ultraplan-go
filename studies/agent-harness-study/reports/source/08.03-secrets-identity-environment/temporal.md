# Source Analysis: temporal

## Dimension 08.03: Secrets, Identity, and Environment Handling

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go 1.26 (Temporal server: frontend/history/matching/worker services, gRPC, Cassandra/SQL/ES persistence) |
| Analyzed | 2026-08-24 |

## Summary

Temporal is not an agent harness; its "tool access to credentials and identities" maps onto (a) how the server itself obtains datastore/TLS/archival credentials at boot, (b) how callers (SDK workers, CLI, HTTP clients) authenticate to the server, and (c) how identity flows between services and clusters.

Secret storage is configuration-driven: passwords, TLS keys, and cloud credentials live in YAML config that can be templated from environment variables via Go templates + sprig (`common/config/loader.go:227-252`, `common/config/config_template_embedded.yaml:23`). There is no pluggable secret-provider abstraction; instead there are targeted mechanisms — an external `passwordCommand` whose stdout becomes the database password, designed for short-lived IAM tokens (`common/config/persistence.go:299-323`), plus a `RefreshingConnector` that re-resolves credentials on every new DB connection (`common/persistence/sql/sqlplugin/connector.go:8-27`). Identity is layered: mTLS with dynamic cert reload and expiration monitoring (`common/rpc/encryption/local_store_tls_provider.go:55-98`, `common/rpc/encryption/local_store_cert_provider.go:52-62`), JWT bearer auth with namespace-scoped permission claims (`common/authorization/default_jwt_claim_mapper.go:76-147`), and outbound remote-cluster bearer tokens that refuse plaintext transport per RFC 9700 (`common/rpc/auth/token_credentials.go:33-37`). Redaction exists but is narrow: config dumps mask `password`/`keyData` fields (`common/masker/masker.go:9-14`, `common/config/config.go:719-726`), Elasticsearch URLs are logged via `URL.Redacted()` (`temporal/fx.go:285`), and internal Nexus callback failures are replaced with opaque reference IDs (`components/callbacks/chasm_invocation.go:40-46`) — but there is no general log/payload scrubbing layer, and Nexus callback tokens are unsigned base64 (`common/nexus/callback_token.go:23-33`). A trace can be shared safely only if it excludes workflow payloads and raw config; the server does not enforce that for you.

## Rating

**7 / 10** — Clear, well-tested identity model (mTLS groups, JWT claims, authorization interceptor) with real operational safeguards: boot-time validation of auth+TLS pairing (`temporal/fx.go:300-308`), certificate expiration metrics/alerts (`common/rpc/encryption/local_store_tls_provider.go:416-445`), spoofing-resistant principal headers (`common/headers/headers.go:125-135`), and extensive tests including failure modes (`common/rpc/test/rpc_token_auth_test.go:63-276`, `common/authorization/interceptor_test.go:591`). Not higher because: secret handling is ad-hoc rather than a unified provider interface; config masking covers only two field names; JWKS key sources may be fetched over plain `http://` (`common/authorization/default_token_key_provider.go:171-187`); Nexus callback tokens lack cryptographic protection (`common/nexus/callback_token.go:28`); dev configs commit plaintext passwords (`config/development-mysql8.yaml:17`); and log-redaction TODOs remain (`service/history/handler.go:2592`).

## Evidence Collected

Every entry includes a file path with line numbers, relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Env config | Server never reads env vars directly except listener IP; documented policy | `temporal/environment/env.go:10-16` |
| Env config | Named env keys: `TEMPORAL_ROOT`, `TEMPORAL_CONFIG_DIR`, `TEMPORAL_ENVIRONMENT`, `TEMPORAL_ALLOW_NO_AUTH`, `TEMPORAL_SERVER_CONFIG_FILE_PATH` | `common/config/loader.go:28-45` |
| Env config | Config files opt into Go-template rendering (`# enable-template`) with sprig funcs exposing `env`/`getenv` | `common/config/loader.go:226-252` |
| Env config | Embedded template pulls DB passwords/TLS keys purely from env: `CASSANDRA_PASSWORD`, `MYSQL_PWD`, `POSTGRES_PWD`, `ES_PWD`, `TEMPORAL_TLS_SERVER_KEY_DATA`, `TEMPORAL_TLS_FRONTEND_KEY_DATA` | `common/config/config_template_embedded.yaml:23,57,110,187,223,257` |
| Env config | Docker template quotes `CASSANDRA_PASSWORD`, `MYSQL_PWD`, `ES_PWD`, visibility password coalescing | `config/docker.yaml:25,58,101,138` |
| Secret storage | `SQL.Password` static field vs `PasswordCommand` (mutually exclusive, validated) | `common/config/config.go:433-439`, `common/config/persistence.go:284-292` |
| Secret storage | `ResolvePassword` execs external command with timeout (30s default) and `WaitDelay` pipe guard | `common/config/persistence.go:294-323` |
| Secret storage | MySQL session uses `PasswordCommand` path with refreshing connector | `common/persistence/sql/sqlplugin/mysql/session/session.go:76-90`; PostgreSQL equivalent `common/persistence/sql/sqlplugin/postgresql/session/session.go:61` |
| Secret storage | RefreshingConnector calls `buildDSN` on every `Connect` so each physical connection gets fresh short-lived credential | `common/persistence/sql/sqlplugin/connector.go:8-27,29-39` |
| Secret storage | TLS artifacts accept both file paths and base64 `Data` variants (`CertFile/CertData/KeyFile/KeyData/CaFile/CaData`) | `common/auth/tls.go:5-27` |
| Secret storage | GCloud archiver: `GOOGLE_APPLICATION_CREDENTIALS` env takes precedence, else `credentialsPath` config; JSON key read from disk | `common/archiver/gcloud/connector/client.go:50-56`, `common/archiver/gcloud/connector/client_delegate.go:88-89` |
| Secret storage | Elasticsearch store config carries `Username`/`Password` and optional AWS request signing | `common/persistence/visibility/store/elasticsearch/client/config.go:25-26`, `temporal/fx.go:270-280` |
| Redaction | `MaskYaml` recursively replaces values under configured field names with `******`; defaults `Password`,`KeyData` (+yaml-cased) | `common/masker/masker.go:9-14,18-37,73-83` |
| Redaction | `Config.String()` serializes config then masks before returning | `common/config/config.go:719-726` |
| Redaction | `render-config` CLI prints only masked config | `cmd/server/main.go:109-124` |
| Redaction | ES connection errors log `cfg.URL.Redacted()` (strips userinfo password) | `common/persistence/visibility/store/elasticsearch/visibility_store.go:154`, `temporal/fx.go:285` |
| Redaction | Internal Nexus/CHASM callback failures log internally and return `internal error, reference-id: <id>` to caller | `components/callbacks/chasm_invocation.go:40-46,73-77` |
| Redaction | Authorization failures return generic `Request unauthorized.` unless `exposeAuthorizerErrors` enabled | `common/authorization/interceptor.go:149-151,317-329` |
| Redaction | Auth token is placed in context but never logged; grep found no logger call carrying `AuthToken` | `common/authorization/interceptor.go:292-299` |
| Redaction gap | History handler still has `TODO: redact certain errors` | `service/history/handler.go:2592,2662` |
| Auth clients (inbound) | gRPC intercept extracts bearer header + TLS subject into `AuthInfo`, maps to `Claims` | `common/authorization/interceptor.go:129-185,251-290` |
| Auth clients (inbound) | Default JWT claim mapper validates RS256/ES256 against rotating JWKS, audience check, regex-parsed permissions | `common/authorization/default_jwt_claim_mapper.go:76-110,153-201` |
| Auth clients (inbound) | Token key provider fetches JWKS from `file://`/HTTP URIs on interval and atomically swaps key sets under RWMutex | `common/authorization/default_token_key_provider.go:43-57,113-136` |
| Auth clients (outbound) | `TokenCredentials` gRPC PerRPCCredentials attach `Bearer` token; `RequireTransportSecurity()==true` per RFC 9700 | `common/rpc/auth/token_credentials.go:10-37` |
| Auth clients (outbound) | Remote-cluster dial wires token provider + fails closed when `remoteClusterAuth.require` and no token | `common/rpc/rpc.go:99-104,260-302` |
| Auth clients (boot safety) | Boot rejects `require=true` without `WithTokenProvider`, and provider-without-TLS combos | `temporal/fx.go:300-308` |
| Per-service identity | mTLS cert groups: internode, frontend, systemWorker, remoteClusters, frontend per-host overrides; refresh ticker reloads certs | `common/rpc/encryption/local_store_tls_provider.go:55-88`, `common/rpc/encryption/local_store_cert_provider.go:52-62` |
| Per-service identity | Dynamic client TLS uses `GetClientCertificate` callback so rotated certs take effect without restart; min TLS 1.2 enforced | `common/auth/tls_config_helper.go:20-28,38-55` |
| Per-service identity | Cert expiration watchdog logs expired/expiring certs and records `tls.certs.expired/expiring` metrics | `common/rpc/encryption/local_store_tls_provider.go:183-199,404-457` |
| Identity spoofing defense | Inbound `PrincipalType/PrincipalName` headers always stripped before authorization; re-set only from server-computed principal when propagation enabled | `common/authorization/interceptor.go:156-158,175-177`, `common/headers/headers.go:125-152` |
| Namespace-scoped identity | `Claims{Subject, System Role, Namespaces map[string]Role, Extensions, AuthType}` | `common/authorization/roles.go:25-36` |
| Namespace-scoped identity | Default authorizer: cluster-scope APIs need System role; namespace APIs accept System OR namespace role; unknown scope denied | `common/authorization/default_authorizer.go:35-65` |
| Cross-namespace commands | RespondWorkflowTaskCompleted cross-ns signals/children/cancels are re-authorized per target namespace with dedup | `common/authorization/interceptor.go:347-417` |
| HTTP→gRPC identity | HTTP API forwards `Authorization-Extras`, client name/version (+configured prefixes) into gRPC metadata; default matcher passes Authorization | `service/frontend/http_api_server.go:52-57,130-143,320-330` |
| Task-scoped tokens | CHASM/Nexus callback token packs component ref + request ID as base64 proto; explicitly notes "encryption support will come later" | `common/nexus/callback_token.go:22-33,39-70`, `chasm/nexus_completion.go:44-63,71-92` |
| Callback validation | Completion handler decodes token, rejects invalid, enforces namespace-in-URL matches token target | `service/frontend/nexus_completion_http_handler.go:127-198` |
| Env isolation (deployments) | Config hierarchy `base.yaml` → `<env>.yaml` → `<env>_<az>.yaml` selected by `--env`/`--zone` flags/env vars | `common/config/loader.go:161-215,281-310`, `cmd/server/main.go:48-67` |
| Env isolation (containers) | `WithEmbedded()` mode loads entire config from env-rendered template only | `common/config/loader.go:113-117,143-148` |
| Safe-default guardrails | Noop authorizer triggers warning unless `--allow-no-auth` / `TEMPORAL_ALLOW_NO_AUTH` set | `cmd/server/main.go:73-77,203-209` |
| Dev-only secrets committed | Development configs ship plaintext password `"temporal"` for mysql/postgres stores | `config/development-mysql8.yaml:17`, `config/development-postgres12.yaml:17` |
| Tests | Masking unit tests incl. nil cases | `common/masker/masker_test.go:11-50` |
| Tests | Token credentials: bearer attached, fetch-per-call, error propagation, empty-token nil metadata, TLS required | `common/rpc/auth/token_credentials_test.go` (see `token_credentials.go` sibling) |
| Tests | Remote-cluster auth integration: token sent on remote conn, absent without provider, wrong token rejected, strict mode rejects empty token, **plaintext dial rejected by credentials** | `common/rpc/test/rpc_token_auth_test.go:63,119,171,227,276` |
| Tests | Interceptor suite: principal-propagation spoofed headers stripped, stream auth, invalid token, audience mapping, cross-namespace authorization | `common/authorization/interceptor_test.go:540-854` |
| Tests | JWT claim mapper & token key provider suites; TLS config helper incl. invalid base64 key and client-cert handshake | `common/authorization/default_jwt_claim_mapper_test.go`, `common/authorization/default_token_key_provider_test.go`, `common/auth/tls_config_helper_test.go:47-310` |

## Answers to Dimension Questions

1. **Can the model see secrets?** N/A literally ("the model" = SDK clients/workers here). Callers never see server-side secrets: datastore passwords, TLS keys, and JWKS sources stay in server config/process memory. Clients present their own identity (client cert or bearer token). However, workflow *participants* can read any payload data they can query unless operators deploy a codec server for encryption — Temporal OSS ships only JSON codec helpers (`common/codec/jsonpb.go`) and no built-in encrypted-payload codec; data-plane encryption is delegated to user infrastructure (documented design goal, not implemented in this source).
2. **Can tools use secrets without exposing them?** Largely yes for the server's own credentials: `passwordCommand` keeps passwords out of config files entirely (`common/config/persistence.go:301-323`), short-lived tokens are refreshed per-connection (`common/persistence/sql/sqlplugin/connector.go:29-39`), and TLS keys can arrive as env-var base64 rather than files (`common/config/config_template_embedded.yaml:223`). Weaknesses: `ResolvePassword` error strings embed command args and stderr (`common/config/persistence.go:318-320`), and JWKS URIs may be plain `http://` (`common/authorization/default_token_key_provider.go:171-187`).
3. **Are secrets redacted in traces?** Partially. Config serialization masks only `password`/`keyData` keys (`common/masker/masker.go:12-13`); ES URLs use `Redacted()` (`temporal/fx.go:285`); auth failures are genericized (`common/authorization/interceptor.go:317-329`); Nexus callback errors become reference IDs (`components/callbacks/chasm_invocation.go:40-46`). But there is no central redaction middleware for arbitrary log fields or workflow payloads, and redaction TODOs are open (`service/history/handler.go:2592`).
4. **Are identities scoped per user/task?** Per-user/per-namespace: yes — JWT claims carry subject + per-namespace roles (`common/authorization/roles.go:25-36`, `common/authorization/default_authorizer.go:51-54`), cross-namespace workflow commands require separate authorization per target (`common/authorization/interceptor.go:347-417`), and principal headers cannot be spoofed (`common/headers/headers.go:125-135`). Per-task: partially — Nexus operation completions carry task-specific callback tokens bound to a component ref and request ID (`chasm/nexus_completion.go:44-63`), but these are unauthenticated base64 capabilities (`common/nexus/callback_token.go:28,32`); anyone who captures one can post a completion to the frontend callback route, mitigated only by network position and namespace-match validation (`service/frontend/nexus_completion_http_handler.go:127-198`).

## Architectural Decisions

- **Config-as-code with template escape hatch, not a secret manager integration.** The loader treats YAML as the single source, optionally rendered by sprig templates reading env vars (`common/config/loader.go:226-252`), and documents a deliberate policy that server code reads no env vars directly (`temporal/environment/env.go:10-16`). Tradeoff: works everywhere (Kubernetes secrets mount naturally), but pushes rotation complexity to operators except where `passwordCommand` applies.
- **Fail-closed outbound auth pairing.** Boot refuses configurations where remote-cluster tokens are required without a provider, or a token provider exists without any remote-cluster TLS — because `TokenCredentials` mandates transport security (`temporal/fx.go:300-308`, `common/rpc/auth/token_credentials.go:33-37`). This encodes RFC 9700 discipline into startup validation rather than runtime hope.
- **Interceptor-centric identity.** All inbound identity (TLS subject + bearer header + extra header + audience) is normalized once into `AuthInfo` → `Claims` and stored in context (`common/authorization/interceptor.go:251-299`); authorizers are pluggable behind a tiny interface taking `(claims, CallTarget)` (`common/authorization/authorizer.go:52-56`). Custom authorizers/claim mappers are injected via server options (`cmd/server/main.go:197-233`).
- **Identity computed server-side, never trusted from the wire.** Principal headers are stripped unconditionally even when authorization is disabled (`common/authorization/interceptor.go:156-158`), then re-established from the authorizer's result (`:175-177`).
- **mTLS as first-class topology identity.** Certs are grouped by relationship (internode, frontend, system worker, remote cluster, per-host frontend overrides) rather than one global pair (`common/rpc/encryption/local_store_tls_provider.go:55-88`), with hot reload and expiry telemetry.

## Notable Patterns

- **Credential-refresh boundary object**: `RefreshingConnector` isolates "resolve DSN per connection" so expiring IAM tokens work without touching driver code (`common/persistence/sql/sqlplugin/connector.go:8-39`).
- **Mask-by-field-name sanitizer** with recursive YAML walk and struct-copy reflection, keeping original structs untouched (`common/masker/masker.go:18-65`).
- **Generic-error pattern**: internal authorization details are logged, callers get `errUnauthorized`, with an explicit dynamic-config opt-in `exposeAuthorizerErrors` (`common/authorization/interceptor.go:314-329`).
- **Reference-ID error laundering**: internal errors are logged with full detail while callers receive `internal error, reference-id: %v` (`components/callbacks/chasm_invocation.go:40-46`) — a correlate-to-support-logs pattern.
- **Capability-token envelope with forward-compatible versioning**: `CallbackToken{Version, Data}` reserves space for future encryption (`common/nexus/callback_token.go:22-33`).
- **Dual-format artifact config**: every TLS artifact accepts File and Data variants, mutually exclusive and validated (`common/auth/tls_config_helper.go:119-138`).

## Tradeoffs

- **Simplicity vs. secret-management depth**: field-name masking and `passwordCommand` cover common cases cheaply, but there is no vault/KMS provider interface, no wildcard masking, and connectAttributes (arbitrary map, `common/config/config.go:448-452`) pass through unmasked.
- **Security posture vs. operator friction**: warning-not-fatal for missing authorizer preserves backward compatibility but lets insecure clusters start indefinitely unless operators heed the deprecation warning (`cmd/server/main.go:203-209`).
- **Hot-reloadable certs vs. staleness risk**: refresh tickers mean a failed reload surfaces only via expiration metrics/logs (`common/rpc/encryption/local_store_cert_provider.go:52-62`, `local_store_tls_provider.go:416-445`).
- **Unsigned callback tokens vs. latency/simplicity**: skipping HMAC avoids key distribution across the fleet now (`common/nexus/callback_token.go:28`), at the cost of relying on network placement for the completion endpoint.
- **Env-var templating vs. leak surface**: convenient, but rendered values transit process argv/stdout in dev tooling; only `render-config` output is masked (`cmd/server/main.go:109-124`).

## Failure Modes / Edge Cases

- **JWKS over plain HTTP**: `openURI` accepts any non-file scheme via bare `http.Get` with no TLS enforcement or size/time limits (`common/authorization/default_token_key_provider.go:171-187`) — a misconfigured `http://` key source silently weakens token verification.
- **Initial key-fetch failure is non-fatal**: `initialize()` logs an error and continues if the first JWKS retrieval fails (`common/authorization/default_token_key_provider.go:46-51`); the server boots unable to validate any token until the next tick succeeds.
- **Password-command subprocess hazards**: stderr content is folded into error messages (`common/config/persistence.go:315-320`), and a subprocess inheriting the stdout pipe is capped only by `WaitDelay` (`:311-314`).
- **Narrow masking**: renaming a config field or adding a new secret-bearing field (e.g., API tokens in `connectAttributes`) silently escapes masking because the mask list is a fixed two names (`common/masker/masker.go:11-14`).
- **Unsigned callback replay**: a captured `Temporal-Callback-Token` remains valid until the operation completes; the handler's defenses are decode validity, structural validation, and namespace match (`service/frontend/nexus_completion_http_handler.go:127-198`, `common/nexus/callback_token.go:72-93`).
- **Streaming endpoints skip authorizer under a flag**: `disableStreamingAuthorizer` bypasses stream authorization entirely (`common/authorization/interceptor.go:195`), trading availability for control-plane consistency during incidents.
- **Committed dev credentials**: `"temporal"` passwords in shipped development configs invite accidental reuse beyond localhost setups (`config/development-mysql8.yaml:17`).

## Future Considerations

- Introduce a `SecretProvider` extension point (mirroring `ClaimMapper`/`Authorizer` plugability) so datastore/cloud credentials can come from Vault/KMS/IRSA without the exec-command shim; wire `RefreshingConnector`-style refresh generically (`common/config/persistence.go:299-323`).
- Enforce HTTPS for JWKS `keySourceURIs` (or warn loudly), and make initial key retrieval fail-fast configurable (`common/authorization/default_token_key_provider.go:170-187`).
- Broaden masking: mask any field whose yaml key contains `password|secret|token|key`, and register new secret-bearing config fields centrally (`common/masker/masker.go:11-14`).
- Sign or encrypt Nexus callback tokens (the struct already reserves for it) and add replay protection via request-ID dedup at the completion handler (`common/nexus/callback_token.go:28-36`).
- Close the redaction TODOs in history error paths and add a shared redaction helper for structured log tags (`service/history/handler.go:2592`).
- Promote the noop-authorizer warning to a hard requirement (flag already exists) in the next major version (`cmd/server/main.go:203-209`).

## Questions / Gaps

- No evidence found of a general-purpose payload/log scrubbing layer: searched `redact|scrub|sanitiz|mask` across `common/log/**` and repo-wide `.go` files; hits were limited to the four sites cited above (ES URL, callback invocation, history TODOs, config masker).
- No evidence found of per-request audit logging that includes caller principal for every API; principal appears only in authorization metrics tags and `Result.Principal` (`common/authorization/interceptor.go:304-345`, `common/authorization/default_authorizer.go:60-63`). Whether Cloud offerings add this is outside this source.
- Data-plane (workflow payload) encryption is not implemented in-repo — only codec plumbing exists (`common/codec/jsonpb.go`); confirming intended reliance on external codec servers would require Temporal docs/samples outside this source, which was out of scope per isolation rules.
- Could not verify whether `Authorization` header survives the grpc-gateway `DefaultHeaderMatcher` path end-to-end in a test; the HTTP API forwards the listed additional headers explicitly (`service/frontend/http_api_server.go:52-57`), and the default matcher behavior was inferred from library semantics, not observed in a test in this repository.

---

Generated by `Dimension 08.03: Secrets, Identity, and Environment Handling` against `temporal`.
