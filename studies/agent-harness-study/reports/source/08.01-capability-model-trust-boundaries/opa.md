# Source Analysis: opa

## Dimension 08.01: Capability Model and Trust Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (policy engine; Rego language, REST server, Go SDK, WASM runtime) |
| Analyzed | 2026-08-24 |

## Summary

OPA is not an LLM agent harness, but it implements one of the most explicit capability models in the studied space: the set of things a policy ("the program") may do is a first-class, versioned, serializable artifact called *capabilities* (`v1/ast/capabilities.go:84-101`). The default capability set — currently 206 built-in functions plus language features and WASM ABI versions — is generated into `capabilities.json` at build time and embedded as per-release snapshots (`v1/capabilities/v1.19.1.json` and predecessors back to `v0.17.0`; loading logic in `v1/ast/capabilities.go:204-243`).

Trust boundaries are enforced at multiple distinct layers, each with its own authority:

1. **Compile time**: the compiler builds its builtin table exclusively from the capabilities set, so a call to any non-whitelisted function fails compilation with "undefined function" (`v1/ast/compile.go:1014-1026`, error at `v1/ast/compile.go:1519`).
2. **Evaluation time**: network-touching builtins (`http.send`, `net.lookup_ip_addr`, remote JSON-schema refs) re-check an `allow_net` host allowlist carried on the capabilities object through the eval context (`v1/topdown/http.go:402-423`, `v1/topdown/net.go:27`, `v1/topdown/jsonschema.go:68-69`).
3. **API boundary**: OPA's own REST API is guarded by an authentication layer that only extracts identities (bearer token or TLS cert CN, no verification — `v1/server/identifier/token.go:24-31`) and an authorization layer that evaluates a user-supplied Rego policy at `data.system.authz.allow` against request path/method/headers/body/identity (`v1/server/authorizer/authorizer.go:107-227`).
4. **Supply chain / control plane**: bundles and discovery configurations can be signature-verified (JWT-based, pluggable verifier — `v1/bundle/verify.go:60-116`, enforcement in `v1/bundle/bundle.go:893-918`), and locally supplied "boot config" overrides remote-discovered keys such as credentials (`v1/plugins/discovery/discovery.go:392-397,542`).

The dimension's guiding question — *"Can the model request power without possessing power?"* — has a direct structural answer in OPA: ad-hoc queries submitted over the API are compiled with `http.send` declared unsafe (`v1/server/server.go:104`, applied at `server.go:996,1482,2668`), while stored policies retain it; `print()` calls are erased unless explicitly enabled (`v1/ast/compile.go:490-496`, no-op without hook at `v1/topdown/print.go:30-33`). Requesters can ask for evaluation of arbitrary queries but cannot smuggle network egress or log output through them.

The model's main weakness: the long-running server (`opa run`) exposes no CLI/config knob to restrict policy capabilities or egress hosts — `allow_net` restriction is reachable from `eval`/`build`/`test`/`bench`/`check` commands (`cmd/eval.go:368`, `cmd/build.go:291`) and the Go SDK, but not in server mode, where stored policies compile against full default capabilities.

## Rating

**8 / 10** — A clear, tested, explicit capability model with layered enforcement (compiler whitelist + eval-time host checks), versioned artifacts, observability tooling (`opa capabilities` command), and operational safeguards (root-user warning, localhost-by-default binding, authn-without-authz misconfiguration detection). Falls short of 9–10 because: (a) the server mode cannot be started with restricted capabilities or an egress allowlist, so the strongest enforcement points are unavailable exactly where policies run long-lived; (b) token "authentication" performs extraction rather than verification, deferring real authn entirely to the authz policy or an external proxy; (c) there is no per-builtin or per-host permission granularity beyond the binary whitelist semantics of `allow_net` (nil = allow all hosts, empty = deny all — `v1/topdown/http.go:402-407`).

## Evidence Collected

Every entry cites file paths relative to `studies/agent-harness-study/sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Capability artifact | `Capabilities` struct: builtins, future keywords, WASM ABI versions, features, `allow_net` | `v1/ast/capabilities.go:84-101` |
| Default capability generation | `CapabilitiesForThisVersion()` copies the builtin registry into the capability set; 206 builtins serialized to root `capabilities.json` | `v1/ast/capabilities.go:147-195` |
| Versioned capability snapshots | Per-release JSON files loaded from embedded FS via `LoadCapabilitiesVersion` | `v1/ast/capabilities.go:205-223`; `v1/capabilities/v0.17.0.json` … `v1.19.1.json` |
| Capability inspection CLI | `opa capabilities --current/--version/--file` prints the exact power set of a build | `cmd/capabilities.go:36-103,105-131` |
| Compatibility targeting | `opa build --capabilities v0.22.0` validates policies against older sets; min compatible version computed from generated index | `cmd/build.go:223-256,291`; `v1/ast/capabilities.go:247-283` |
| Compile-time whitelist enforcement | Compiler builds builtin table only from capabilities; custom builtins merged after; unknown calls → type error | `v1/ast/compile.go:1014-1026,1519,2042-2048` |
| Capability injection API | `Compiler.WithCapabilities` (docs state builtins are what capabilities restrict today); deprecated `WithUnsafeBuiltins`/`WithBuiltins` delegate to it | `v1/ast/compile.go:563-572,588-602` |
| Eval-time network gate | `verifyHost`/`verifyURLHost`: nil `AllowNet` = allow all, empty = deny all; applied to `http.send` URL and redirects | `v1/topdown/http.go:402-423,471-476,645-650` |
| DNS builtin gated | `builtinLookupIPAddr` calls `verifyHost(bctx.Capabilities, name)` before resolving | `v1/topdown/net.go:20-30` |
| Capabilities reach eval | `BuiltinContext.Capabilities` field; populated from compiler during eval | `v1/topdown/builtins.go:59`; `v1/topdown/eval.go:1058-1085` |
| Remote schema fetch gate | `allow_net` also bounds JSON-schema remote reference fetching in type checker and `json.schema` builtins | `v1/ast/compile.go:993-998,1900,2056`; `v1/topdown/jsonschema.go:68-69`; doc `cmd/eval.go:295-315` |
| Ad-hoc query restriction | Server-wide `unsafeBuiltinsMap = {http.send}` used by `/v0/data`, `/v1/query`, `/v1/compile` handlers | `v1/server/server.go:104,996,1482,2668` |
| print() opt-in | Statements erased at compile time unless enabled; runtime no-op without PrintHook; enabled when log level ≥ Info | `v1/ast/compile.go:490-496`; `v1/topdown/print.go:30-33`; `v1/runtime/runtime.go:547` |
| Authn schemes | off / token / TLS enumeration; token middleware only extracts bearer identity, does not validate it | `v1/server/server.go:66-70,782-791`; `v1/server/identifier/token.go:8-32` |
| Authz enforcement | `Basic` authorizer evaluates configured decision (default `data.system.authz`) with input `{path, method, params, headers, body, identity, client_certificates}`; undefined → 500; deny → 401 (+optional reason) | `v1/server/authorizer/authorizer.go:25-37,107-165,192-226` |
| Authz wiring & ordering | Authz handler wraps routers so it runs first; diagnostic router included | `v1/server/server.go:793-814,866-870,939-941` |
| Misconfiguration guard | Token authn + authorization off → startup error "Authentication will be ineffective" | `v1/runtime/runtime.go:680-682`; test `v1/runtime/runtime_test.go:931` |
| Authz policy schema check | Known-input schema verification for authorization policy; skippable via flag | `internal/compiler/utils.go:19-31`; `v1/runtime/runtime.go:1163-1172`; `cmd/run.go:186,248` |
| Interface binding defaults | v1 default binds `localhost:8181`; warning when public interface implied in v0-compat mode | `cmd/run.go:30,226`; `v1/runtime/runtime.go:667-669` |
| Privilege check | Warns when running with uid/gid 0 (root); no-op stub on Windows | `v1/runtime/check_user_unix.go:17-23`; `v1/runtime/check_user_windows.go:14` |
| Bundle signature verification | JWT signatures in `.signatures.json`; pluggable `Verifier` interface; missing key/signature mismatches rejected; `--skip-verify` escape hatch | `v1/bundle/verify.go:60-116`; `v1/bundle/bundle.go:893-918`; `cmd/flags.go:131` |
| Discovery trust flow | Remote discovery bundle can reconfigure all plugins/services; boot-config keys override discovered ones and override list surfaced in status | `v1/plugins/discovery/discovery.go:51-53,105-157,392-397,420-462,542` |
| Service credentials plugins | Bearer, OAuth2 client-credentials, client TLS, AWS SigV4/SigV4a (+ECR/KMS), GCP metadata, Azure managed identity; extensible lookup | `v1/plugins/rest/rest.go:62-67,84-118`; `v1/plugins/rest/aws.go:691-711` |
| Secret injection | Config `${VAR}` environment interpolation on load and on `--set` overrides | `internal/config/config.go:92,103,128-157` |
| Decision-log data protection | Masking policy at `/system/log/mask`, drop policy at `/system/log/drop` evaluated per event | `v1/plugins/logs/plugin.go:272-273,786,896` |
| Telemetry opt-out | Version check against GitHub disabled via `--skip-version-check` (deprecated `--disable-telemetry`) | `cmd/run.go:255-262` |
| One-shot sandboxed-ish mode | `opa exec` evaluates decisions against input files with plugin triggers, no listening server | `cmd/exec.go:35-57` |
| Embedded-host authority | SDK/rego options expose `Capabilities`, `UnsafeBuiltins`, custom builtin registration to embedding applications | `v1/rego/rego.go:1251-1258,1344-1350` |

## Answers to Dimension Questions

### 1. What can the agent do?

OPA's active agent — a compiled Rego policy under evaluation — can: query data documents and rules; invoke any builtin present in the active capability set (206 by default, including crypto, regex, time, encoding, and HTTP builtins); perform outbound HTTP via `http.send` to any host when `AllowNet` is nil (default — `v1/topdown/http.go:403`); resolve DNS names via `net.lookup_ip_addr` (`v1/topdown/net.go:27`); fetch remote JSON schemas when type checking uses schemas (`v1/topdown/jsonschema.go:68-69`); and emit `print()` output when a print hook is configured (`v1/topdown/print.go:30-48`). The surrounding runtime process additionally serves REST APIs, downloads bundles/configuration from configured services using credential plugins (`v1/plugins/rest/rest.go:62-67`), and persists bundles/data to disk.

### 2. What can the model only request but not directly do?

Three concrete cases: (a) an API caller submitting ad-hoc queries can request evaluation of arbitrary logic, but those queries are compiled with `http.send` marked unsafe, so the requested evaluation cannot open network connections — only pre-stored policies can (`v1/server/server.go:104,996,1482,2668`); (b) policy authors can write `print()` statements, which the compiler silently erases unless the operator enables print support (`v1/ast/compile.go:490-496`) — requesting output ≠ getting it; (c) callers of PUT `/v1/policies` can request policy installation, but whether the request succeeds is itself decided by the authz policy evaluated first (`v1/server/authorizer/authorizer.go:140-165`). Similarly, bundle publishers sign manifests, but OPA will refuse activation without a matching verification key (`v1/bundle/bundle.go:898-909`).

### 3. Where is authority enforced?

Five layers, each independently testable: (1) **compiler** — builtin whitelist is closed-world; non-members fail compilation (`v1/ast/compile.go:1018-1026,1519`); (2) **evaluator** — `allow_net` checked inside builtin implementations against `BuiltinContext.Capabilities` (`v1/topdown/http.go:402-423`, `v1/topdown/net.go:27`), meaning even dynamically constructed URLs and redirect targets are re-checked (`v1/topdown/http.go:645-650`); (3) **HTTP middleware** — authz policy gates every route before handlers run (`v1/server/server.go:866-870,939-941`); (4) **bundle reader** — signature verification precedes activation (`v1/bundle/bundle.go:893-918`); (5) **process bootstrap** — boot-config precedence over discovered config for sensitive keys (`v1/plugins/discovery/discovery.go:439-462`). Note the deliberate split between *what exists* (compile-time whitelist) and *what may be contacted* (eval-time host list): both must be tightened to fully contain a policy.

### 4. Are dangerous capabilities isolated?

Partially. Strong isolations: ad-hoc queries lose `http.send` (`v1/server/server.go:104`); print requires dual opt-in (compiler + hook, `v1/ast/compile.go:493-496`, `v1/runtime/runtime.go:547`); running as root produces a warning (`v1/runtime/check_user_unix.go:21-23`); binding defaults to localhost (`cmd/run.go:30`). Weak spots: in server mode there is no supported way to hand the server a restricted capabilities file — `--capabilities` exists on `eval/build/test/bench/check` (`cmd/eval.go:368`, `cmd/build.go:291`) but not on `run` (`cmd/run.go` contains no capabilities flag), so stored policies keep full egress; token authn never validates the bearer token (`v1/server/identifier/token.go:24-31`), making security wholly dependent on the authz policy being correctly deployed; and the authz policy lives in the same store it protects, so any channel that can write policies (API, bundles, local files) can rewrite its own permissions — a self-referential trust anchor mitigated only by operational discipline (external proxies, signed bundles).

## Architectural Decisions

- **Capabilities as data, not code.** The power set is a serializable JSON document with per-release snapshots embedded since v0.17.0 (`v1/ast/capabilities.go:204-243`, `v1/capabilities/*.json`). This turns "what can this policy do" into a diffable, CI-checkable artifact (`opa build --capabilities`, `cmd/build.go:223-256`) and enables forward-compat computation via a generated version index (`v1/ast/capabilities.go:28-50,247-283`).
- **Two-axis restriction: language surface vs. runtime targets.** The whitelist removes builtins from existence at compile time (`v1/ast/compile.go:1018-1024`), while `allow_net` constrains where surviving network builtins may connect at eval time (`v1/topdown/http.go:402-423`). The design acknowledges both are needed: removing `http.send` blocks egress; keeping it with an empty `AllowNet` allows the builtin but denies all peers.
- **Self-hosting authorization.** OPA uses its own policy engine to authorize access to itself (`data.system.authz`, `v1/server/authorizer/authorizer.go:116-126`). This dogfoods the product and gives operators full expressiveness, at the cost of a bootstrap/trust-anchor circularity (undefined decision ⇒ HTTP 500, `authorizer.go:134-138`).
- **Identity extraction separated from identity verification.** The server only establishes *claims* (bearer string, TLS cert) into request context (`v1/server/identifier/token.go`, `v1/server/server.go:782-791`); deciding trustworthiness is delegated to the authz policy. Documented failure signal: token authn without authorization logs an error at startup (`v1/runtime/runtime.go:680-682`).
- **Local boot config outranks control plane for secrets.** Discovered configuration may reconfigure almost everything, but keys/credentials provided at boot override discovered values and the overrides are reported in status (`v1/plugins/discovery/discovery.go:392-397,439-462`).

## Notable Patterns

- **Capability threading through context.** Capabilities travel from compiler to evaluator via `BuiltinContext.Capabilities` (`v1/topdown/builtins.go:59`; populated at `v1/topdown/eval.go:1058-1085`), so each builtin implementation self-enforces — no central choke point needs to know builtin semantics.
- **Fail-closed defaults with explicit escape hatches.** Undefined authz decision → 500 (`v1/server/authorizer/authorizer.go:134-138`); unsigned bundle with expected key ID → error (`v1/bundle/bundle.go:898-900`); empty `AllowNet` → deny all (`v1/topdown/http.go:402-407`); each paired with an explicit opt-out (`--skip-verify` at `cmd/flags.go:131`).
- **Deprecation funnel toward capabilities.** `WithBuiltins` and `WithUnsafeBuiltins` remain only as deprecated shims pointing at `WithCapabilities` (`v1/ast/compile.go:588-602`), consolidating the permission model into one mechanism.
- **Redirect-safe host checking.** `CheckRedirect` re-validates each hop's host against `AllowNet`, closing a classic SSRF bypass (`v1/topdown/http.go:645-650`; tests covering redirects at `v1/topdown/http_test.go:793-850`).
- **Observability of the permission system itself.** `opa capabilities --current|--version|--file` (`cmd/capabilities.go:54-100`) makes the effective power set inspectable at runtime, and authz/authz-handler latency is separately metered (`PromHandlerAPIAuthz`, `v1/server/server.go:94,808-810`).

## Tradeoffs

- **Expressiveness vs. containment.** Stored policies get `http.send` because real-world admission control needs it; the cost is that any policy author (or anyone who can upload policies) holds egress capability unless operators add external network controls. The server-mode gap (no `--capabilities` on `opa run`) is the visible seam of this tradeoff.
- **Self-hosted authz vs. immutable anchor.** Using Rego for API authorization maximizes flexibility but means the guard is mutable through the same door it guards; OPA compensates with schema verification of the authz policy input (`internal/compiler/utils.go:19-31`; `v1/runtime/runtime.go:1163-1172`) rather than immutability.
- **Extraction-only authn vs. simplicity.** Treating the bearer token as an opaque identity (`v1/server/identifier/token.go:24-31`) keeps OPA dependency-free but shifts all verification burden to deployers; the startup misconfiguration error (`v1/runtime/runtime.go:680-682`) is the chosen mitigation.
- **Nil-means-allow-all semantics.** `AllowNet: nil` permits every host to preserve backward compatibility (`v1/topdown/http.go:403`, documented at `v1/ast/capabilities.go:94-100`); secure-by-default would break existing deployments, so safety requires explicit configuration.

## Failure Modes / Edge Cases

- **Server-mode egress is unbounded by default.** Without external controls, any successfully uploaded policy can call `http.send` to any host, including cloud metadata endpoints; nothing in `opa run` restricts this today (no capabilities flag in `cmd/run.go`; default full set at `internal/runtime/init/init.go:158-161`).
- **Authz policy rewrite escalation.** A caller authorized for `PUT /v1/policies` under the current authz policy can install a new `system.authz` module granting broader rights; the next request is judged by the attacker-controlled policy (mechanism: authz reads compiler+store live, `v1/server/authorizer/authorizer.go:27-28,117-119`).
- **Token-authn-no-authz foot-gun.** Deployers enabling `--authentication=token` alone get zero protection (identity extracted but unused); OPA detects and logs this combination loudly (`v1/runtime/runtime.go:680-682`) but still starts.
- **DNS rebinding window.** `http.send`'s `verifyURLHost` parses the URL host string, while connection dialing resolves DNS afterwards; an `allow_net` entry for a hostname does not pin resolved IPs across requests (host-string comparison at `v1/topdown/http.go:409-423`).
- **Skip flags concentrate risk.** `--skip-verify` disables bundle signature checks globally for a command (`cmd/flags.go:131`; branch at `v1/bundle/bundle.go:893-896`); a convenience flag in one deployment script silently downgrades supply-chain integrity everywhere it is reused.
- **Undefined authz decision = outage.** If the authz policy is deleted or returns undefined, every API request gets 500 (`v1/server/authorizer/authorizer.go:134-138`) — availability is coupled to policy correctness, a deliberate fail-closed choice.

## Future Considerations

- Wire a capabilities/egress configuration surface into `opa run` (e.g., `--capabilities` and a config-level `allow_net` applied to the server compiler), closing the largest containment gap between batch commands and server mode.
- Add per-builtin or per-scope permission metadata to capabilities (currently a flat whitelist — `v1/ast/capabilities.go:85`), enabling "allow `http.send` only to these hosts" without removing the builtin entirely; `AllowNet` already carries the right shape but applies only to a subset of builtins.
- Consider pinning resolved IPs for allowlisted hostnames (connect-to-IP with SNI/Host validation) to harden `allow_net` against DNS-based bypasses, complementing the existing redirect re-check (`v1/topdown/http.go:645-650`).
- Extend the known-schema verification pattern (authz input today — `v1/runtime/runtime.go:1163-1172`) to other privileged system policies (`system.log.mask`, `system.log.drop`) as noted in `cmd/run.go:184-186`.
- Make identity verification pluggable at the server layer (e.g., JWKS validation option) so token authn is not permanently limited to extraction (`v1/server/identifier/token.go`).

## Questions / Gaps

- **No evidence found** for a protected/reserved namespace preventing writes to `data.system.*` through the REST API: searches across `v1/server`, `internal/storage`, and plugin code for protection concepts returned only the reserved response fields in `v1/server/types/types_extensible_test.go:209`. Absent contrary evidence, authz-policy mutation via the standard policy APIs appears permitted if authorized — treat as a deployment responsibility, not an enforced boundary.
- Whether the WASM target narrows builtin power relative to Go evaluation could not be confirmed either way: the WASM VM dispatches host callbacks with capabilities and explicitly supports `http.send` (`internal/wasm/sdk/internal/wasm/vm.go:290-294`), suggesting parity rather than isolation; a systematic diff of planned-vs-executed builtins per target was out of scope for this pass.
- The exact threat model intended for `AuthenticationTLS` identity (CN extraction only, `v1/server/server.go:786-788`) — i.e., whether mutual-TLS termination is always expected upstream — is documented behaviorally but not stated as a security guarantee in code comments; `SECURITY.md` was reviewed only via directory listing, not analyzed as normative spec for this dimension.

---

Generated by `Dimension 08.01: Capability Model and Trust Boundaries` against `opa`.
