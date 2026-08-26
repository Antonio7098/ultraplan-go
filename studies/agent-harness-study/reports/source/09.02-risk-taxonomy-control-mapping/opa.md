# Source Analysis: opa

## 09.02 Risk Taxonomy and Control Mapping

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (policy engine; Rego language, HTTP server, Go SDK, Wasm target) |
| Analyzed | 2026-08-26 |

> All file citations below are relative to the source root `studies/agent-harness-study/sources/opa`.

## Summary

OPA does **not** implement an explicit, named risk taxonomy. There is no risk enum, risk score, severity class, or risk-registry type anywhere in the codebase — a repo-wide case-insensitive search for "risk"/"taxonomy" in Go code returns only incidental comments (`v1/plugins/discovery/discovery.go:492` "These changes are risky…", `v1/ast/term.go:1947`, `v1/plugins/server/decoding/config.go:14`) and no type or constant named after risk.

Instead, OPA's model is the inverse of a taxonomy-first design: **controls are named and machine-readable; risks are left unnamed and delegated to policy authors**. The closest things to a taxonomy are:

1. A **machine-readable capability inventory** (`capabilities.json`, generated from `v1/ast/builtins.go`) that enumerates every builtin with per-builtin property flags — `Deprecated` and `Nondeterministic` — which function as de-facto risk attributes on each "tool".
2. An ordered, named **compiler enforcement pipeline** whose stages include safety, unsafe-builtin, deprecated-builtin, and required-capability checks.
3. A set of domain-scoped runtime controls (network egress allowlist, per-request authorization decision, decision-log masking, credential purging, bundle signature verification), each mapping one implicit risk class to one enforcement point.

Risk assessment granularity: OPA assesses **per-tool** (per builtin, via capability flags and `allow_net`), **per-action** (every API request is evaluated against an authorization policy over `{path, method, params, headers, body, identity}`), and **per-policy/per-runtime** (capabilities restrict what a given compilation/runtime may use). It never scores risk per agent — there is no agent concept.

Runtime exposure of risk metadata is asymmetric: decision outcomes, masked paths, and nondeterministic-builtin caches are observable via decision logs and status; but the capability inventory itself is exposed only through the CLI (`opa capabilities`), not through any HTTP endpoint.

## Rating

**5 / 10** — Present but inconsistent. The control side is genuinely strong: enforcement points are explicit interfaces with tests and operational safeguards (e.g., tested `allow_net` rejection, fail-closed masking). But the dimension's core subject — risks being *named, categorized, and mapped to controls* — is largely absent: there is no risk vocabulary, no central risk→control registry, and the mapping exists only implicitly through package layout and configuration flags. An operator can explain *what control* applies to an action only by reading code/docs across five subsystems; nothing in the product answers "which controls apply here?" for them.

## Evidence Collected

Every entry includes a file path with line numbers, relative to `studies/agent-harness-study/sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| No formal risk taxonomy | Repo-wide grep for "risk"/"taxonomy" in Go sources yields only comments; no risk enum/type exists | `v1/plugins/discovery/discovery.go:492`, `v1/ast/term.go:1947` |
| Builtin struct carries risk-like attributes | `Deprecated bool` and `Nondeterministic bool` fields on every builtin declaration | `v1/ast/builtins.go:3632-3650` |
| Accessors for those attributes | `IsDeprecated()` at :3670, `IsNondeterministic()` at :3675 | `v1/ast/builtins.go:3670-3676` |
| Machine-readable inventory | `capabilities.json`: 206 builtins, 9 flagged `nondeterministic` (e.g. `http.send`, `time.now_ns`, `rand.intn`, `opa.runtime`), 11 flagged `deprecated` | `capabilities.json:1` (generated artifact) |
| Deprecated builtins section | Named block of legacy builtins each marked `Deprecated: true` | `v1/ast/builtins.go:3480-3493` |
| Capabilities as runtime control plane | `Capabilities{Builtins, FutureKeywords, WasmABIVersions, Features, AllowNet}`; `AllowNet` documented as egress restriction | `v1/ast/capabilities.go:82-101` |
| Compiler wires capabilities into enforcement | `WithCapabilities` sets available builtins; nil → full current-version capabilities | `v1/ast/compile.go:563-571`, `v1/ast/compile.go:1014-1020` |
| Unsafe/deprecated builtin blocking (legacy API) | `WithUnsafeBuiltins` (deprecated in favor of capabilities) plus dedicated compiler stages `compile_state_check_unsafe_builtins` and `compile_state_check_deprecated_builtins` | `v1/ast/compile.go:599-602`, `v1/ast/compile.go:474-475` |
| Per-policy risk profile | `buildRequiredCapabilities` computes keywords/features/builtin dependencies each module requires | `v1/ast/compile.go:1198-1240` |
| Ordered enforcement pipeline | Named compile stages incl. safety heads/bodies, recursion, types, unsafe & deprecated builtins | `v1/ast/compile.go:455-477` |
| Network egress control | `verifyHost`/`verifyURLHost` gate `http.send`, `net.lookup_ip_addr`, remote schema fetch against `AllowNet` | `v1/topdown/http.go:402-421` |
| Egress control tested | Test asserts `http.send` fails with `"disallowed host"` under restricted capabilities | `v1/topdown/http_test.go:816`, `v1/topdown/http_test.go:3727` |
| Nondeterminism isolated in partial eval | Non-deterministic builtins saved/not-evaluated during PE unless explicitly enabled | `v1/topdown/save.go:368`, `v1/topdown/save.go:535`, `v1/topdown/save.go:541` |
| Per-request (per-action) authorization | Authorizer evaluates configured decision ref per request; allow must be boolean `true` else 401 | `v1/server/authorizer/authorizer.go:107-165` |
| Action input schema for authz | Input = `{path, method, params, headers, body, identity, client_certificates}` | `v1/server/authorizer/authorizer.go:192-224` |
| Default-deny authz pattern | Docs require `default allow := false` under reserved `system.authz`; authz defaults to `off` | `docs/docs/security.md:103-160` |
| Hardened deployment guidance | Minimal-surface config example (localhost TLS, deny-by-default authz limited to `POST /`) | `docs/docs/security.md:607-654` |
| Decision-log masking control | Mask ops limited to `remove`/`upsert`; paths restricted to `input`, `result`, `nd_builtin_cache` prefixes | `v1/plugins/logs/mask.go:18-76` |
| Mask rule source of truth | Mask decision queried from policy path, default `/system/log/mask` | `v1/plugins/logs/plugin.go:272`, `v1/plugins/logs/plugin.go:1048-1090` |
| Fail-closed masking | On mask error the event is logged-and-dropped, never pushed unmasked | `v1/plugins/logs/plugin.go:785-788` |
| Observability of applied controls | Event records `Masked []string` and opt-in `nd_builtin_cache` field | `v1/plugins/logs/plugin.go:63-65`, `v1/plugins/logs/plugin.go:1219-1221` |
| Credential-leak control on `opa.runtime()` | `activeConfig` strips service credentials and crypto keys before exposing config to policy | `v1/topdown/runtime.go:52-92` |
| print() data-leak control | `print()` calls erased at compile time unless `EnablePrintStatements`; no-op without hook at eval | `v1/ast/compile.go:493-498`, `v1/topdown/print.go:33-36` |
| Fail-closed builtin errors toggle | `WithStrictBuiltinErrors` promoted through rego options and HTTP query params | `v1/topdown/query.go:269-271`, `v1/rego/rego.go:1304-1306`, `docs/docs/policy-language.md:2603-2611` |
| Supply-chain control | Bundle JWT signature verification + per-file hash verification | `v1/bundle/verify.go:60-85`, `v1/bundle/verify.go:211-233` |
| Config self-modification guard | Discovery plugin refuses updates to discovery service config ("risky because errors would be unrecoverable") and to signing keys | `v1/plugins/discovery/discovery.go:489-512` |
| Runtime exposure of capabilities: CLI only | `opa capabilities` prints inventory; server route table has no `/v1/capabilities` endpoint | `cmd/capabilities.go:36-43`, `v1/server/server.go:904-927` |
| Policy metadata (annotations) not risk-aware | Annotation scopes are `package/rule/document/subpackages`; no severity/risk field | `v1/ast/annotations.go:20-34` |
| Strict mode as extra safety tier | `--strict` enables additional compiler checks (unused imports/vars, shadowing) | `v1/ast/compile.go:604-607`, `docs/docs/policy-language.md:3776-3780` |

## Answers to Dimension Questions

**1. Are risks named and categorized?**
No — not as risks. There is no risk enum, class, score, or registry (search boundary: case-insensitive `grep -ril "risk"|"taxonomy"` across all `.go`, `.md`, `.yaml`, `.json` files in the source tree returned only prose/comments). The nearest implemented analog is per-builtin categorization: `Deprecated` and `Nondeterministic` flags on the `Builtin` struct (`v1/ast/builtins.go:3632-3650`) serialized into the versioned capability files (`capabilities.json`). These name two *properties* that correlate with risk (side-effectful/nondeterministic behavior; removal hazard) but the codebase nowhere frames them as risk categories, and other risk-relevant properties (network access, filesystem access, crypto) are uncategorized.

**2. Is every risk mapped to a control?**
There is no registry that would let anyone answer "every", but a consistent implicit mapping exists per domain:

| Implicit risk | Control | Enforcement point |
|---|---|---|
| Policy uses unavailable/unsafe builtins | Capabilities allowlist | `v1/ast/compile.go:569-571`, `1014-1020` |
| Egress to arbitrary hosts | `allow_net` host check | `v1/topdown/http.go:402-421` |
| Unauthorized API action | Per-request Rego authorization | `v1/server/authorizer/authorizer.go:107-165` |
| Sensitive data in logs | Mask rules (+ fail-closed drop) | `v1/plugins/logs/mask.go:52-76`, `plugin.go:785-788` |
| Credential exposure to policies | Purge in `opa.runtime()` | `v1/topdown/runtime.go:52-92` |
| Debug output leaking values | Compile-time erase of `print()` | `v1/ast/compile.go:493-498` |
| Tampered policy supply chain | Signed bundles + file hashes | `v1/bundle/verify.go:70-85`, `211+` |
| Builtin errors swallowed silently | strict-builtin-errors toggle | `v1/topdown/query.go:269-271` |

Risks outside these domains (e.g., memory exhaustion, infinite recursion — though recursion *is* checked at `v1/ast/compile.go:471`) have no enumerated entry anywhere.

**3. Can risks be assessed at runtime?**
Partially, and only as pass/fail checks rather than assessments. At evaluation time: `allow_net` is enforced per call (`v1/topdown/http.go:402-406`), the authorizer re-evaluates every request (`v1/server/authorizer/authorizer.go:128`), and nondeterministic builtins get special treatment in partial evaluation (`v1/topdown/save.go:535`). Risk *metadata* is surfaced post-hoc through decision-log events (`masked` list, `nd_builtin_cache`, decision IDs — `v1/plugins/logs/plugin.go:63-65, 394-402`) and status reporting, but there is no runtime query like "which controls apply to this action". The capability inventory itself is inspectable only offline via `opa capabilities` (`cmd/capabilities.go:36-43`); the server exposes `/v1/config`, `/v1/status`, `/v1/data`, etc., but no capabilities endpoint (`v1/server/server.go:924-925`).

**4. Can controls be bypassed?**
Yes, through several concrete seams:
- **Opt-in wiring**: capabilities, unsafe-builtin blocking, and `allow_net` apply only if the embedder passes them into the compiler (`v1/ast/compile.go:563-571`); with `nil` capabilities the compiler installs the full current builtin set (`v1/ast/compile.go:1014-1020`). The Go SDK will happily evaluate unrestricted policies.
- **String-match egress check**: `verifyHost` compares the URL host string against `AllowNet` entries as-is (`v1/topdown/http.go:402-406`); docs acknowledge hosts are checked literally (`docs/docs/policy-language.md:3728`), so hostname-vs-IP-literal mismatches evade the list.
- **Authorization off by default**: both authentication and authorization default to `off` (`docs/docs/security.md:117-119`); an unprotected server exposes policy read/write APIs (`PUT /v1/policies/{path...}`, `v1/server/server.go:918`).
- **Deprecated escape hatches remain**: `WithBuiltins`/`WithUnsafeBuiltins` still function (`v1/ast/compile.go:590-602`), letting callers register custom builtins outside capability gating.
- **Mask failure trades observability**: a mask error silently drops the event instead of pushing it (`v1/plugins/logs/plugin.go:785-788`) — good for leak-prevention, but decisions then vanish from the audit trail with only a local log line.
- **Legacy custom-builtins path**: noted above; also `Categories` on builtins is aspirational only (`v1/ast/builtins.go:3638-3640` comment "(NOTE(sr): aspirational)"), so category-based controls can't exist yet.

## Architectural Decisions

- **Controls-not-taxonomy**: OPA externalizes risk judgment to policy authors; the engine provides generic, composable enforcement points (compiler gates, authorizer middleware, log masking) rather than a built-in risk model. This is consistent with its stated philosophy that Rego is domain-agnostic (`docs/blog/2017-12-14-opas-full-stack-policy-language-caeaadb1e077.md:165`).
- **Machine-readable capability contract**: the entire feature surface (206 builtins, keywords, Wasm ABI versions) is serializable (`v1/ast/capabilities.go:82-108`) and versioned per release (`capabilities/v0.17.0.json` …), enabling forward-compatibility checks when policies move between OPA versions — effectively a per-policy "required capabilities" bill of materials computed by the compiler (`v1/ast/compile.go:1198-1240`).
- **Fail-closed defaults at sensitive boundaries**: undefined authorization decision → 500/misconfiguration signal (`v1/server/authorizer/authorizer.go:134-138`); masking failure → event dropped; `print()` erased unless explicitly enabled.
- **Named compiler pipeline**: every safety mechanism is a discrete, named, ordered stage (`v1/ast/compile.go:455-477`), which makes the control chain auditable in code even though it isn't exposed to operators.

## Notable Patterns

- **Capability flags as proto-risk-metadata**: `Nondeterministic` drives real behavior (partial-eval save semantics, `v1/topdown/save.go:535`), not just documentation — a property flag wired into an actual control decision.
- **Reserved namespace for platform controls**: `system.authz` and `system.log.mask` are engine-owned policy paths (`docs/docs/security.md:148-160`, `v1/plugins/logs/plugin.go:272`), cleanly separating platform-control policies from user policies.
- **Defense spread across phases**: the same risk (unwanted network access) is mitigated at compile time (builtin presence), config time (`allow_net`), and eval time (`verifyHost` per call).
- **Self-modification guards**: the discovery plugin freezes its own bootstrap config and signing keys, refusing risky runtime changes (`v1/plugins/discovery/discovery.go:492-512`).

## Tradeoffs

- **Generality vs. operator legibility**: because risk naming is delegated to policy authors, OPA ships no answer to "what controls protect this action?" — the mapping table above had to be reconstructed from six packages.
- **Safety opt-in vs. safe-by-default**: capabilities/`allow_net`/strict errors are powerful but dormant unless configured; the only always-on controls are `print()` erasure, recursion checks, and credential purging.
- **Fail-closed vs. auditability**: dropping masked-failure events protects data but creates silent observability gaps (`v1/plugins/logs/plugin.go:785-788`).
- **Literal host matching vs. DNS agility**: string-based `allow_net` is simple and testable but brittle around IPs/redirects, acknowledged in docs (`docs/docs/policy-language.md:3728`).

## Failure Modes / Edge Cases

- Embedder forgets `rego.SetDefaultCapabilities`/`WithCapabilities` → all 206 builtins including `http.send` available (`v1/ast/compile.go:1014-1020`).
- `allow_net` bypass via IP literal where hostname listed (and vice versa) — `v1/topdown/http.go:402-406`.
- Mask rule targeting an undefined path without `fail_undefined_path` leaves data intact (`v1/plugins/logs/mask.go:53-54, 121-126`).
- Authorization decision returns non-boolean/non-map → request rejected 401 even if policy intended allow (`v1/server/authorizer/authorizer.go:140-164`); undefined decision → 500 misconfiguration signal (:134-138).
- Custom builtins registered via the deprecated `WithBuiltins` path are invisible to capability-based audits (`v1/ast/compile.go:144-145`).

## Future Considerations

- Promote the aspirational builtin `Categories` field (`v1/ast/builtins.go:3638-3640`) to a populated, enforced classification (network/fs/crypto/dangerous), giving capabilities a real risk axis.
- Expose effective capabilities and active controls via a server endpoint (today CLI-only, `cmd/capabilities.go:36-43`) so operators can diff a running instance against its intended posture.
- Emit a per-decision "controls applied" record (capabilities used, masks applied, authz decision ref) into decision-log events to make the implicit mapping observable at runtime.
- Normalize host matching in `verifyHost` (resolve or canonicalize) to close the string-match gap (`v1/topdown/http.go:402-406`).

## Questions / Gaps

- No evidence found of any risk-scoring, risk-level, or severity concept in policy metadata: annotations support free-form `custom` keys (`v1/ast/annotations.go:29-34`) but nothing consumes them as risk signals.
- No evidence found that `allow_net` covers redirect targets or DNS rebinding beyond the initial URL parse; tests cover direct disallowed hosts only (`v1/topdown/http_test.go:816`).
- Whether Wasm-target policies honor the same capability gating was not traced end-to-end in this study (Wasm ABI versions appear in capabilities, `v1/ast/capabilities.go:87`, but the wasm compile-path enforcement points were not inspected).
- The SECURITY_AUDIT.pdf and `SECURITY.md` were not treated as implementation evidence per study rules; they may state additional design goals not verified here.

---

Generated by `dimensions/09.02-risk-taxonomy-and-control-mapping` against `opa`.
