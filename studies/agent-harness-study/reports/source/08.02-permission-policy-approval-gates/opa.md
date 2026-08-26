# Source Analysis: opa

## Permission Policy and Approval Gates

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (HTTP server, Rego policy engine, plugin/bundle subsystems) |
| Analyzed | 2026-08-26 |

## Summary

OPA gates sensitive operations through a layered, self-hosted model rather than an interactive "approve this action" workflow. Every incoming API request can be gated by a Rego authorization policy (`system.authz`) evaluated by the server's own engine before the request handler runs (`v1/server/authorizer/authorizer.go:107-165`). Identity is established by pluggable authentication middleware — Bearer token or client TLS certificate — that attaches identity data to the request context (`v1/server/identifier/token.go:22-33`, `v1/server/identifier/tls.go:23-32`). Sensitive *policy supply* operations are additionally gated by bundle JWT signature verification plus key-scope matching (`v1/bundle/verify.go:70-116`, `v1/bundle/verify.go:199-208`), and writes into bundle-owned storage paths are blocked by ownership checks (`v1/server/server.go:2521-2558`).

The distinguishing strength is that OPA applies its own policy engine to its own protection: the authz decision is just another Rego query against the same store, and the shape of that query's input document is enforced by a JSON schema type-check both at startup (`v1/runtime/runtime.go:573-577`) and on every bundle activation (`v1/bundle/store.go:997-1001`). Even OPA's configuration file is validated by an embedded Rego policy that injects the default authorization decision path `/system/authz/allow` (`v1/config/validate.rego:18,29-31`). Authorization is off by default (`v1/server/server.go:77-80`), and there is no built-in audit trail of authorization decisions themselves and no expiry/TTL semantics on granted identities — the main maturity gaps found.

## Rating

**8 / 10.** A clear, explicit permission model with typed input schemas, deny-by-default evaluation when enabled, hot-revocable policies, extensive tests down to failure cases (undefined decisions, eval conflicts, malformed paths), and operational safeguards unusual for this category (startup + activation-time type checking of the authz policy itself; misconfiguration warnings such as token-auth-without-authz at `v1/runtime/runtime.go:680-682`). It falls short of 9–10 because: authorization decisions are not recorded in decision logs by default (no observable audit of who was allowed/denied), identities have no expiration semantics, bundle signature verification checks scope but not time-based JWT claims (`exp`/`nbf`, see `v1/bundle/verify.go:118-208`), and gating is disabled by default.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Authn/authz scheme enums | `AuthenticationOff/Token/TLS` and `AuthorizationOff/Basic`; authorization defaults to off (zero value) | `v1/server/server.go:63-80` |
| Server-level gate wiring | `WithAuthorization` stores scheme; `initHandlerAuthz` wraps router with `authorizer.NewBasic` when `AuthorizationBasic` | `v1/server/server.go:303-306`, `v1/server/server.go:793-814` |
| Middleware ordering | Comment states authorization must be added BEFORE authentication "so that the latter can run first"; chain is authn → authz → router | `v1/server/server.go:866-868`, `v1/server/server.go:232`, `v1/server/server.go:939-941` |
| Approval decision evaluation | `Basic.ServeHTTP` evaluates the configured decision ref per request; boolean or `{allowed, reason}` object semantics; deny returns 401 | `v1/server/authorizer/authorizer.go:107-165` |
| Undefined decision handling | No result from authz policy → HTTP 500 with `authorization policy missing or undefined` (fail-closed) | `v1/server/authorizer/authorizer.go:134-138`, `v1/server/types/types.go:176` |
| Custom deny reasons | Policy may return `{"allowed": false, "reason": "..."}` surfaced to the client | `v1/server/authorizer/authorizer.go:146-158` |
| Permission input schema | Input = path, method, params, headers, body, identity, client_certificates | `v1/server/authorizer/authorizer.go:169-227` |
| Input JSON schema | `schemas/authorizationPolicy.json` declares required properties for the authz input document | `v1/schemas/authorizationPolicy.json:4-42` |
| Startup schema enforcement | `verifyAuthorizationPolicySchema` type-checks rules behind the authz decision ref unless `--skip-known-schema-check` | `v1/runtime/runtime.go:573-577`, `v1/runtime/runtime.go:257-258`, `v1/runtime/runtime.go:1166-1172` |
| Activation-time schema enforcement | Bundle compile path calls `VerifyAuthorizationPolicySchema` on every activation | `v1/bundle/store.go:997-1001` |
| Schema type-check implementation | Compiles authz rules + transitive dependencies against the schema in a fresh compiler | `internal/compiler/utils.go:44-95` |
| Decision ref config | `default_authorization_decision` config field; ref accessor; default `/system/authz/allow` injected by embedded Rego config-validation policy | `v1/config/config.go:97`, `v1/config/config.go:273-278`, `v1/config/validate.rego:18,29-31` |
| Config validated by own engine | `ParseConfig` evaluates embedded Rego validation policy; fatal errors + warnings | `v1/config/config.go:112-154` |
| Live policy pointer | Authorizer queries `b.decision().String()` per request; manager config swapped atomically under lock (discovery-driven reconfig takes effect immediately) | `v1/server/authorizer/authorizer.go:117`, `v1/plugins/plugins.go:1001-1021` |
| Identity extraction (token) | Bearer regex on `Authorization` header sets identity in context | `v1/server/identifier/token.go:20-33` |
| Identity extraction (TLS) | Client cert subject CN set as identity; certs passed into input | `v1/server/identifier/tls.go:23-32`, `v1/server/identifier/certs.go:13-25` |
| Misconfiguration warning | Token authentication without authorization logged as ineffective | `v1/runtime/runtime.go:680-682` |
| Root privilege warning | Warns when running with uid/gid 0 | `v1/runtime/check_user_linux.go:15-21` |
| Runtime info exposure | `authorization_enabled` published via `opa.runtime()` info | `v1/runtime/info/info.go:21`, `v1/runtime/info/info.go:61` |
| Unsafe builtin gate | `http.send` blocked as unsafe builtin in server-side query/policy eval paths | `v1/server/server.go:104`, `v1/server/server.go:996`, `v1/server/server.go:1482`, `v1/server/server.go:2668` |
| Write-protection gates | `checkPathScope` rejects writes to bundle-owned roots; invoked on data PATCH/PUT/DELETE and policy PUT/DELETE | `v1/server/server.go:2521-2558`, `v1/server/server.go:1715`, `v1/server/server.go:1988`, `v1/server/server.go:2061`, `v1/server/server.go:2107`, `v1/server/server.go:2283` |
| Config sanitization | `GET /v1/config` serves `ActiveConfig()` with service credentials and crypto keys removed | `v1/server/server.go:2466-2474`, `v1/config/config.go:299-326` |
| Supply-chain approval gate | `VerifyBundleSignature` verifies exactly one JWT using configured keys; extensible `Verifier` registry | `v1/bundle/verify.go:61-116`, `v1/bundle/verify.go:264-285` |
| Key scope check | Signature payload scope must equal configured bundle/key scope | `v1/bundle/verify.go:199-208` |
| Signing config plumbing | Bundle source `signing` config validated against global `keys`; discovery bundles support signing too | `v1/plugins/bundle/config.go:152`, `v1/plugins/bundle/config.go:175-182`, `v1/plugins/discovery/config.go:31,99-106` |
| Key config schema | Supported algorithms whitelist; key/scope fields | `v1/keys/keys.go:11-56` |
| Log masking/drop gates | Decision log events masked (`remove`/`upsert`) and dropped by Rego policies at `/system/log/mask` and `/system/log/drop` | `v1/plugins/logs/plugin.go:272-273`, `v1/plugins/logs/plugin.go:766-786`, `v1/plugins/logs/mask.go:128-199` |
| Unit tests (authorizer) | Table test covering allow/deny per role/path/method, custom reasons, undefined decision, eval conflict error, non-bool responses | `v1/server/authorizer/authorizer_test.go:43-258` |
| E2E authz test | `TestAuthorization`: allow bob/deny alice, live policy reversal revokes bob and grants alice mid-flight | `v1/server/server_test.go:5070-5189` |
| Authz caching test | Inter-query cache used for authz evaluations; authorizer-parsed body reused by handler | `v1/server/server_test.go:5191-5330` |
| Schema verification tests | Bad refs (`input.identty`) rejected at startup incl. transitive rule deps | `v1/runtime/runtime_test.go:802-902`, `internal/compiler/utils_test.go:15-135` |
| Ineffective-auth warning test | `TestCheckAuthIneffective` asserts the warning text | `v1/runtime/runtime_test.go:904-934` |

## Answers to Dimension Questions

**1. Which actions require approval?**
When started with `--authorization=basic`, *every* request hitting the main and diagnostic routers passes through the authorizer first — including health, config, status, data, and policy endpoints — because `initHandlerAuthz` wraps the whole router (`v1/server/server.go:793-814`, `v1/server/server.go:939-941`). The granularity (which method+path+identity combinations are allowed) is entirely defined by the user-supplied `system.authz` policy; nothing is hard-coded except that the decision is queried at the configured ref (default `/system/authz/allow`, `v1/config/validate.rego:18`). When authorization is off (the default), no request is gated. Separately, two classes of sensitive operation have non-bypassable code-level gates regardless of authz: writes into bundle-owned storage paths (`v1/server/server.go:2521-2558`) and activation of unsigned/incorrectly signed bundles (`v1/bundle/verify.go:94-115`).

**2. Who can approve?**
There is no human approver concept. The runtime "approver" is the `system.authz` policy itself, evaluated per request against an identity established by the authentication middleware: a Bearer token string (`v1/server/identifier/token.go:22-33`) or the TLS client certificate subject (`v1/server/identifier/tls.go:26`). Who can *change* who is approved is also policy-governed: `TestAuthorization` demonstrates bob updating the authz policy over the API because the current policy lets him (`v1/server/server_test.go:5137-5154`). For policy *supply* via bundles, the bundle publisher holding the signing key acts as the approver, verified cryptographically (`v1/bundle/verify.go:105-115`).

**3. Are approvals scoped and expiring?**
Scoped, yes: the input document contains full method/path/query/headers/body plus identity and client certificates (`v1/server/authorizer/authorizer.go:192-226`), so policies can express narrow grants — the test fixture grants read-only access to one specific data path while admin gets `"path": "*"` (`v1/server/authorizer/authorizer_test.go:114-145`). Expiring, no: identities carry no TTL, tokens are static entries in the store, and the bundle signature verifier validates the JWT signature and `scope` claim but never checks `exp`/`nbf` claims (`v1/bundle/verify.go:118-208`). Revocation is achieved only by pushing new policy/data (see below).

**4. Can policy override model intent?**
Reframed for OPA (no LLM/model intent here): can the policy layer override what the client requests? Yes — structurally. The authorizer sits outside all route handlers and short-circuits with 401/500 without ever invoking them (`v1/server/authorizer/authorizer.go:140-164`). Code-level guards additionally override even an allowful authz policy: bundle-root ownership blocks data/policy writes that authz might otherwise permit (`v1/server/server.go:2539-2555`), unsigned bundles are refused during download processing (`v1/plugins/bundle/plugin.go:463-475`), and `http.send` is stripped as unsafe in server eval contexts (`v1/server/server.go:996`). Conversely, the authz policy cannot be bypassed by request content: it sees the raw body before handlers do and caches it on the context to avoid divergence between the authorized body and the processed body (`v1/server/authorizer/authorizer.go:209-214`).

## Architectural Decisions

- **Self-hosted policy gate.** OPA uses its own Rego evaluator as the authorization mechanism instead of embedding an ACL/RBAC library. The gate is a plain `http.Handler` wrapping the mux, parameterized by a decision-ref function (`v1/server/authorizer/authorizer.go:93-126`). This keeps one policy language for both product function and self-protection, at the cost of a bootstrap dependency: the gate needs a compiled policy and store to answer any request.
- **Fail-closed on undefined, but loud.** An undefined authz decision produces HTTP 500 "authorization policy missing or undefined" rather than allowing through (`v1/server/authorizer/authorizer.go:134-138`). Denial-by-default is only guaranteed once a policy exists; with `authorization=off` everything is open.
- **Type-checked trust boundary.** The exact input contract of the security gate is frozen in a JSON schema (`v1/schemas/authorizationPolicy.json:4-42`) and enforced by Rego type checking at startup and on every bundle compile, catching typos like `input.identty` before they become silent always-deny/always-allow bugs (`internal/compiler/utils.go:44-71`, `internal/compiler/utils_test.go:85-96`). This is verifiable hardening, not documentation-only.
- **Cryptographic provenance for policy supply.** Bundles (and discovery configs) can require JWT signatures matched against configured keys with a mandatory scope match, with a pluggable `Verifier` registry for custom implementations (`v1/bundle/verify.go:58-86`, `v1/bundle/verify.go:273-280`).
- **Live-reconfigurable gate.** The decision ref is resolved through `manager.GetConfig()` inside a closure evaluated per request, so discovery-driven config swaps change which decision governs access without restart (`v1/server/server.go:801`, `v1/server/authorizer/authorizer.go:117`, `v1/plugins/plugins.go:1017`).

## Notable Patterns

- **Body-once pattern:** the authorizer parses the request body and stashes it on the request context so downstream handlers reuse it, guaranteeing the authorized bytes are the processed bytes (`v1/server/authorizer/authorizer.go:209-214`, consumed at `v1/server/server.go:2926`, `v1/server/server.go:2973`).
- **Extensible route awareness:** plugins can register extra "expects body" predicates so the authorizer knows which additional paths carry bodies to expose to policy (`v1/server/authorizer/authorizer.go:86-90`, wired from `v1/plugins/plugins.go:777`).
- **Policy-shaped log hygiene:** decision-log events pass through Rego-driven drop and mask decisions (`remove`/`upsert` ops on arbitrary JSON paths) before leaving the process (`v1/plugins/logs/plugin.go:766-786`, `v1/plugins/logs/mask.go:165-199`) — the same "policy gates side effects" idea applied to telemetry.
- **Defense-in-depth warnings:** token-auth-without-authz (`v1/runtime/runtime.go:680-682`) and root-privilege (`v1/runtime/check_user_linux.go:19-21`) warnings encode deployment-security review as startup diagnostics.
- **Secret minimization on introspection APIs:** `ActiveConfig` strips credentials and crypto keys before serving config (`v1/config/config.go:317-324`).

## Tradeoffs

- **Expressiveness vs. auditability:** because the gate is arbitrary Rego, there is no machine-readable permission catalog to diff or export; permissions are whatever the policy computes. Combined with the fact that authorization decisions do not flow into the decision-log pipeline (the logs plugin masks/drops *data* API events; the authorizer performs its own bare `rego.Eval` with no logger hook, `v1/server/authorizer/authorizer.go:116-132`), operators get no built-in record of allowed/denied administrative actions.
- **Off-by-default openness vs. ease of adoption:** `AuthorizationOff` is the zero value (`v1/server/server.go:78`), so an unconfigured OPA accepts everything; safety relies on operator discipline and the docs' localhost-binding advice (`docs/docs/security.md:101-106`).
- **Self-modification power:** the authz policy lives in the same store it protects; whoever can write policies can rewrite their own permissions (demonstrated intentionally in `v1/server/server_test.go:5137-5162`). Mitigations exist (bundle-root write locks, `v1/server/server.go:2521-2558`; signed bundles), but they must be deliberately adopted.
- **Latency vs. strictness:** every request pays a Rego eval; OPA mitigates with prepared-query-style reuse and the inter-query cache (`v1/server/server_test.go:5191`).
- **Permissiveness of signature checks:** bundle JWTs verify signature and scope but ignore temporal claims (`v1/bundle/verify.go:195-208`), simplifying offline verification while giving up automatic key rotation/expiry.

## Failure Modes / Edge Cases

- **Undefined decision → 500:** misnamed decision ref or missing policy makes every request fail closed with an internal-error code, which is safe but can masquerade as an outage rather than a misconfiguration (`v1/server/authorizer/authorizer.go:134-138`; tested at `v1/server/authorizer/authorizer_test.go:190`).
- **Eval conflict → 500:** conflicting complete-rule results in the authz policy produce an evaluation error response, not a deny (`v1/server/authorizer/authorizer_test.go:191`).
- **Malformed URL escapes → 400:** path parsing rejects invalid percent-encodings before policy eval (`v1/server/authorizer/authorizer_test.go:260-288`).
- **Non-bool/non-object decisions → 500 with generic undefined message**, e.g. a reason of wrong type falls back to the standard unauthorized message (`v1/server/authorizer/authorizer_test.go:198-199`).
- **Typo'd input references:** historically would silently change semantics; now caught at startup and bundle activation via schema type-checking (`v1/runtime/runtime_test.go:828-848`).
- **Token auth without authz:** authentication alone is useless (identity extracted but nothing consumes it); OPA warns loudly (`v1/runtime/runtime.go:680-682`, asserted in `v1/runtime/runtime_test.go:904-934`).
- **Escape hatch risk:** `--skip-known-schema-check` disables the authz-policy type checking entirely (`v1/cmd/run.go:248` equivalent flag plumbed at `v1/runtime/runtime.go:573-577`), reintroducing the silent-typo failure mode for users who hit false positives.
- **Unsigned bundle acceptance is opt-in:** if no `signing` config is present, downloaded bundles activate without any signature (`v1/plugins/bundle/config.go:175-182` only injects defaults when keys exist) — supply-chain gating depends on correct configuration, not code enforcement.

## Future Considerations

- Emit decision-log (or at least structured-audit) events for authorization decisions, reusing the existing logs-plugin pipeline with its mask/drop policies (`v1/plugins/logs/plugin.go:766-786`), closing the observability gap for allow/deny history.
- Add optional temporal claim validation (`exp`/`nbf`) in `DefaultVerifier` (`v1/bundle/verify.go:118-208`) to make signing approvals expireable.
- Introduce TTL/lease semantics for token identities stored under `system.tokens` so revocation does not depend solely on pushing new data.
- Make the fail-closed 500-on-undefined case distinguishable from genuine internal errors (dedicated error code) to speed up misconfiguration triage (`v1/server/authorizer/authorizer.go:136`).
- Consider shipping a hardened default authz policy (root-token + health-only) instead of requiring operators to author one, mirroring the minimal policy documented in `docs/docs/security.md:143-270`.

## Questions / Gaps

- **No evidence found** for any interactive/human-in-the-loop approval workflow (pending/approved/denied state machine, approver roles, approval records). Searches across `v1/server`, `v1/runtime`, `v1/plugins`, `cmd`, and `docs/docs/security.md` found only evaluate-per-request authorization; OPA has no persistent "approval" objects.
- **No evidence found** that authorization decisions are persisted anywhere: neither the authorizer (`v1/server/authorizer/authorizer.go`) nor the server's authz wiring (`v1/server/server.go:793-814`) reference the decision-log plugin.
- **Not determined:** whether the diagnostic router's authz coverage differs in policy from the main router (both wrap identically at `v1/server/server.go:868-870`, but discrimination happens inside user policy via `input.path`).
- Rate limiting exists for decision-log uploads (`decision_logs_dropped_rate_limit_exceeded`, `v1/plugins/logs/plugin.go:274`) but **no evidence found** of request-rate-based admission control on the API itself.

---

Generated by `Dimension 08.02: Permission Policy and Approval Gates` against `opa`.
