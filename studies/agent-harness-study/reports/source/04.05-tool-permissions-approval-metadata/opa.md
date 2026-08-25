# Source Analysis: opa

## Dimension 04.05: Tool Permissions and Approval Metadata

### Source Info

| Field | Value |
|-------|-------|
| Name | opa |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (Rego policy language, CLI + server + embeddable SDK) |
| Analyzed | 2026-08-15 |

## Summary

OPA is not an agent harness, so it has no notion of "tool approval". The closest structural analogue to a tool registry is the **built-in function registry** (`v1/ast/builtins.go:3594-3639` defines the `Builtin` metadata struct; `v1/topdown/builtins.go` holds the runtime `builtinFunctions` map). The analogue to tool permission metadata is the **capabilities** mechanism (`v1/ast/capabilities.go:84-101`), which is an *allowlist* of built-ins, language features, future keywords, Wasm ABI versions, and an `allow_net` host allowlist.

The model of interest for this dimension is: capabilities are an **allowlist enforced primarily at compile time**. `Compiler.WithCapabilities` (`v1/ast/compile.go:571-574`) replaces the compiler's builtin table with only the capabilities-declared builtins (`v1/ast/compile.go:1997-2005`), so any call to an omitted builtin fails the `CheckUndefinedFuncs`/type-check stages (`v1/ast/compile.go:467`, `v1/ast/compile.go:1483-1510`) as `rego_type_error`. A separate, explicitly deprecated denylist path exists — `WithUnsafeBuiltins` (`v1/ast/compile.go:598-604`) with the `CheckUnsafeBuiltins` stage (`v1/ast/compile.go:475`, `v1/ast/compile.go:7208-7220`) producing `unsafe built-in function calls in expression: <name>`, documented at `docs/docs/errors/rego-type-error/unsafe-built-in-function-calls-in-expression-name.md:6-20`.

Risk classification of builtins is **very thin and not security-oriented**. The `Builtin` struct carries only `Categories`, `Relation`, `Deprecated`, `Nondeterministic`, `CanSkipBctx` (`v1/ast/builtins.go:3601-3608`). `Categories` are topical (`aggregates`, `crypto`, `strings` — see `builtin_metadata.json:2-40`), not risk tiers. `Nondeterministic: true` is the only field that incidentally marks the side-effecting/impure builtins: `http.send` (`v1/ast/builtins.go:2965`), `net.lookup_ip_addr` (`v1/ast/builtins.go:3376`), `opa.runtime` (`v1/ast/builtins.go:3206`), `time.now_ns` (`v1/ast/builtins.go:2414`), `rand.intn` (`v1/ast/builtins.go:1475`), `uuid.rfc4122` (`v1/ast/builtins.go:1570`), and the JWT sign/verify family (`v1/ast/builtins.go:2362`, `v1/ast/builtins.go:2381`, `v1/ast/builtins.go:2398`). But that flag exists for **partial-evaluation correctness and result caching**, not risk: it gates PE skipping (`v1/topdown/save.go:494`, `v1/topdown/save.go:374`) and the non-deterministic builtin cache (`v1/topdown/eval.go:2067-2069`). There is no `RequiresApproval`, `Destructive`, `Network`, or `SecretAccess` field anywhere in the struct.

There is exactly one **runtime** (evaluation-time) permission check: `allow_net`. `BuiltinContext.Capabilities` (`v1/topdown/builtins.go:60`) is populated from the compiler (`v1/topdown/eval.go:1070-1097`), and `http.send`/`net.lookup_ip_addr` consult it via `verifyHost`/`verifyURLHost` (`v1/topdown/http.go:374-399`, called at `v1/topdown/http.go:459`, `v1/topdown/http.go:668`, `v1/topdown/net.go:27`), returning `unallowed host: <host>`.

Crucially, the *builtin allowlist itself is not re-checked at eval time*: `eval.builtinFunc` resolves against the global `ast.BuiltinMap` and the global `builtinFunctions` table, not against `capabilities.Builtins` (`v1/topdown/eval.go:211-224`, dispatched at `v1/topdown/eval.go:1048-1051`). And `opa run` — the server/daemon mode — exposes **no `--capabilities` flag** at all; the flag is only wired into `build`, `eval`, `test`, `check`, `bench`, `fmt` (`cmd/flags.go:138-140`; consumers `cmd/build.go:269`, `cmd/eval.go:349`, `cmd/test.go:118`). So a long-running OPA server cannot be started with a restricted builtin set.

The strongest "policy can block a requested action" mechanism in the repo is not about builtins at all: it is the server authorization middleware (`v1/server/authorizer/authorizer.go:107-163`), where a Rego policy decision (`system.authz`) gates every API request and can return `{"allowed": false, "reason": ...}` → HTTP 401.

## Rating

**5 / 10** — Present but inconsistent and partly decorative for risk purposes.

Rationale: there *is* an explicit, versioned, serializable permission surface (`v1/ast/capabilities.go:84-101`, per-release files under `v1/capabilities/`), it *is* enforced with tests (`v1/ast/compile_test.go:12254-12280`, `v1/topdown/http_test.go:820`), and it *is* documented as a security control (`docs/docs/errors/rego-type-error/unsafe-built-in-function-calls-in-expression-name.md:14-20`). That earns above the 1-3 band. It does not reach 7 because: (a) there is no risk taxonomy on tools — `Nondeterministic` is a PE/caching flag reused as a proxy (`v1/topdown/save.go:494`); (b) enforcement is compile-time, and the eval-time dispatcher ignores capabilities (`v1/topdown/eval.go:211-224`); (c) the primary long-running deployment mode (`opa run`) cannot set capabilities at all (no match for `capabilities` in `cmd/run.go`); (d) there is no approval/confirmation concept, no persistence of approvals, and no per-call interception hook for a high-risk builtin.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Permission schema | `Capabilities` struct: `Builtins`, `FutureKeywords`, `WasmABIVersions`, `Features`, `AllowNet` — the entire permission surface | `v1/ast/capabilities.go:84-101` |
| Permission semantics doc | `allow_net`: "If omitted, ANY host can be connected to. If empty, NO host can be connected to." — insecure default is explicit | `v1/ast/capabilities.go:94-100` |
| Tool metadata struct | `Builtin` fields: `Categories`, `Infix`, `Relation`, `Deprecated`, `CanSkipBctx`, `Nondeterministic` — no risk/approval field | `v1/ast/builtins.go:3594-3609` |
| Risk-proxy accessors | `IsDeprecated()`, `IsNondeterministic()` — the only two metadata predicates | `v1/ast/builtins.go:3631-3639` |
| Categories are topical, not risk tiers | `_categories` map: `aggregates`, `array`, `bits`, `comparison`, `conversions`, `crypto` | `builtin_metadata.json:2-40` |
| Network tool flagged only as nondeterministic | `http.send` declaration ends with `Nondeterministic: true` | `v1/ast/builtins.go:2963-2966` |
| DNS tool | `net.lookup_ip_addr`, `Nondeterministic: true` | `v1/ast/builtins.go:3374-3377` |
| Secret/config exposure tool | `opa.runtime` returns config + **environment variables**, marked only `Nondeterministic: true` | `v1/ast/builtins.go:3204-3207` |
| Crypto signing tools | `io.jwt.encode_sign*` marked `Nondeterministic: true`, category `tokenSign` | `v1/ast/builtins.go:2379-2399` |
| Allowlist enforcement (compile) | `WithCapabilities` stores user capabilities; docstring frames it as restricting available builtins | `v1/ast/compile.go:565-574` |
| Allowlist enforcement mechanism | Compiler builtin table built *only* from `c.capabilities.Builtins` + custom builtins | `v1/ast/compile.go:1993-2007` |
| Default is permissive | If no capabilities supplied, compiler falls back to `CapabilitiesForThisVersion()` = every builtin | `v1/ast/compile.go:1993-1995`, `v1/ast/capabilities.go:147-195` |
| Denylist path (deprecated) | `WithUnsafeBuiltins` marked "Deprecated: Use WithCapabilities instead" | `v1/ast/compile.go:598-604` |
| Denylist field comment | `unsafeBuiltinsMap ... user-supplied set of unsafe built-ins functions to block (deprecated: use capabilities)` | `v1/ast/compile.go:147` |
| Denylist enforcement stage | `StageCheckUnsafeBuiltins` registered in the compile pipeline | `v1/ast/compile.go:212`, `v1/ast/compile.go:475` |
| Denial implementation | `checkUnsafeBuiltins` walks exprs, emits `TypeErr` "unsafe built-in function calls in expression: %v" | `v1/ast/compile.go:7208-7220` |
| Denial extends to `with` mocks | `validateWith`/`isBuiltinRefOrVar` reject unsafe builtins used as `with` replacement targets — closes a bypass | `v1/ast/compile.go:7017-7026`, `v1/ast/compile.go:7146-7152` |
| Query-level override of denylist | `QueryCompiler.WithUnsafeBuiltins` lets ad-hoc queries carry their own denylist, falling back to compiler's | `v1/ast/compile.go:371-376`, `v1/ast/compile.go:3607-3609`, `v1/ast/compile.go:3848-3853` |
| Deprecation gate | `checkDeprecatedBuiltins` only errors when `c.strict` or module is `rego.v1` — otherwise decorative | `v1/ast/compile.go:1918-1937` |
| Metadata visible to runtime | `BuiltinContext.Capabilities *ast.Capabilities` | `v1/topdown/builtins.go:60` |
| Metadata plumbed at eval | Capabilities pulled off compiler into `BuiltinContext` | `v1/topdown/eval.go:1070-1097` |
| Only runtime permission check | `verifyHost` returns `unallowed host: %s`; short-circuits to allow when `AllowNet == nil` | `v1/topdown/http.go:374-383` |
| URL variant | `verifyURLHost` parses URL, strips port, delegates to `verifyHost` | `v1/topdown/http.go:386-399` |
| Runtime check call sites | `http.send` raw-URL path and request path; `net.lookup_ip_addr` | `v1/topdown/http.go:459`, `v1/topdown/http.go:668`, `v1/topdown/net.go:27` |
| Runtime check is NOT applied to builtin allowlist | `eval.builtinFunc` resolves via global `ast.BuiltinMap` + global `builtinFunctions`, never `capabilities.Builtins` | `v1/topdown/eval.go:211-224` |
| Dispatch site | Unknown builtin → `unsupportedBuiltinErr`; known builtin executes with no permission gate | `v1/topdown/eval.go:1048-1051` |
| Test: denied tool (compiler + query) | `TestCompilerWithUnsafeBuiltins` asserts both query and module compilation fail with "unsafe built-in function" | `v1/ast/compile_test.go:12254-12280` |
| Test: denied tool (`with` mock) | Table cases expecting `unsafe built-in function calls in expression: count` | `v1/ast/compile_test.go:11413`, `v1/ast/compile_test.go:11422` |
| Test: denied tool (public SDK) | `unsafeCountExpr := "unsafe built-in function calls in expression: count"` across 7 assertions | `v1/rego/rego_test.go:1962-2072` |
| Test: network denial enforced at runtime | Expects `eval_builtin_error` / `http.send: unallowed host: <host>` | `v1/topdown/http_test.go:820`, `v1/topdown/http_test.go:3727`, `v1/topdown/http_test.go:3835` |
| Test: DNS denial | `expError := fmt.Errorf("unallowed host: %s", addr)` | `v1/topdown/net_test.go:192` |
| Test: feature-level capability denial | Table driving `capabilities.Features`/`capabilities.Builtins` and asserting compile errors | `v1/ast/compile_test.go:12115-12138` |
| Test: minimal capabilities compile | `NewCompiler().WithCapabilities(&Capabilities{Builtins: []*Builtin{Split}})` | `v1/ast/compile_test.go:10046-10051` |
| Test: capabilities parse rejection | `TestParserCatchesIllegalCapabilities`, `TestParserCatchesIllegalFutureKeywordsBasedOnCapabilities` | `v1/ast/capabilities_test.go:10`, `v1/ast/capabilities_test.go:61` |
| CLI flag | `--capabilities` "set capabilities version or capabilities.json file path" | `cmd/flags.go:138-140` |
| CLI flag type accepts version or file | `capabilitiesFlag.Set` tries `LoadCapabilitiesFile` then `LoadCapabilitiesVersion` | `cmd/flags.go:199-230` |
| Flag default is nil (permissive) | `newCapabilitiesFlag` returns `C: nil` with a comment about custom builtin registration ordering | `cmd/flags.go:204-210` |
| Where the flag is wired | `build`, `eval`, `test`, `bench`, `check`, `fmt` only | `cmd/build.go:269`, `cmd/eval.go:349`, `cmd/test.go:118`, `cmd/bench.go:62` |
| Server mode gap | `cmd/run.go` contains no `capabilities` reference (grep over `cmd/*.go` lists bench, build, capabilities, check, eval, flags, fmt, test — not run) | `cmd/run.go` (absence), `cmd/flags.go:138` |
| Runtime package gap | No `Capabilities` reference in `v1/runtime/*.go`, `v1/sdk/*.go`, `v1/plugins/*.go` | `v1/runtime/` (absence) |
| Documented allowlist example | `build` help shows a capabilities file permitting only `plus` | `cmd/build.go:203-235` |
| Documented `allow_net` usage | `eval` help: `"allow_net": [ "kubernetesjsonschema.dev" ]`, and "Not providing a capabilities file ... will" allow all | `cmd/eval.go:283-296` |
| Documented as a security control | "commonly used to restrict certain built-ins ... the Rego Playground, where `http.send` is disabled due to security concerns" | `docs/docs/errors/rego-type-error/unsafe-built-in-function-calls-in-expression-name.md:14-20` |
| Documented bypass | "If you're encountering this on the Rego Playground, run the policy on your own machine using e.g. `opa eval` or `opa run` instead." | `docs/docs/errors/rego-type-error/unsafe-built-in-function-calls-in-expression-name.md:24-26` |
| Off-by-default side effect (analogue of gated tool) | `rewritePrintCalls` **erases** all `print()` calls unless `enablePrintStatements` is set | `v1/ast/compile.go:2641-2648`, `v1/ast/compile.go:2826-2840` |
| Off-by-default flag | `enablePrintStatements bool // indicates if print statements should be elided (default)` | `v1/ast/compile.go:149`, `v1/ast/compile.go:492-497` |
| Policy-driven request denial | Authorizer evaluates a Rego decision; `false` or missing `allowed` → HTTP 401 with optional `reason` | `v1/server/authorizer/authorizer.go:107-163` |
| Fail-closed authorizer | Empty result set → HTTP 500 `MsgUnauthorizedUndefinedError`; fallthrough → 401 | `v1/server/authorizer/authorizer.go:134-138`, `v1/server/authorizer/authorizer.go:163` |
| Capability escalation surface | `RegisterFeatures` lets embedders append arbitrary feature strings to the global `Features` slice | `v1/ast/capabilities.go:71-80` |
| Nondeterminism used for caching, not risk | `canUseNDBCache` gates the NDB cache on `bi.Nondeterministic` | `v1/topdown/eval.go:2067-2069` |
| Nondeterminism used for PE, not risk | `cmp.Or(slices.Contains(ast.IgnoreDuringPartialEval, bi), bi.Nondeterministic)` | `v1/topdown/save.go:494`, `v1/topdown/save.go:374` |
| Opt-in impure evaluation | `EvalNondeterministicBuiltins` / `NondeterministicBuiltins` options, exposed over HTTP as `nondeterministicBuiltins` | `v1/rego/rego.go:407-412`, `v1/rego/rego.go:1004-1009`, `v1/server/types/types.go:517` |
| Versioned permission baselines | Per-release capabilities JSON loaded from embedded FS, sorted with semver | `v1/ast/capabilities.go:204-250` |
| Compatibility (not risk) reporting | `MinimumCompatibleVersion` derives min OPA version from builtins/keywords/features | `v1/ast/capabilities.go:252-290` |
| Required-capabilities introspection | `buildRequiredCapabilities` records what a policy actually needs; exposed via `Compiler.Required` | `v1/ast/compile.go:127-128`, `v1/ast/compile.go:1201-1210` |

## Answers to Dimension Questions

**1. Are tools risk-classified?**

Not for risk. The `Builtin` struct has `Categories` (topical: `crypto`, `strings`, `aggregates` — `builtin_metadata.json:2-40`), `Relation`, `Deprecated`, `Nondeterministic`, `CanSkipBctx` (`v1/ast/builtins.go:3601-3608`). There is **no** enum or field distinguishing read-only vs. network vs. secret-access vs. external side effect. Mapping the dimension's risk taxonomy onto OPA's builtins has to be done by hand:

| Risk class | OPA builtins | Metadata that marks them |
|-----------|-------------|--------------------------|
| Read-only / pure | `count`, `split`, `plus`, `regex.match`, etc. | none needed; absence of `Nondeterministic` |
| Network | `http.send` (`v1/ast/builtins.go:2963`), `net.lookup_ip_addr` (`v1/ast/builtins.go:3374`) | `Nondeterministic: true` + `allow_net` enforcement at `v1/topdown/http.go:374` |
| Secret / host-config access | `opa.runtime` (returns `env`, `config` — `v1/ast/builtins.go:3204`) | `Nondeterministic: true` only; **no** `allow_net`-style gate |
| Crypto / signing | `io.jwt.encode_sign*` (`v1/ast/builtins.go:2379-2399`) | `Nondeterministic: true`, category `tokenSign` |
| Ambient/impure | `time.now_ns` (`v1/ast/builtins.go:2412`), `rand.intn` (`v1/ast/builtins.go:1473`), `uuid.rfc4122` (`v1/ast/builtins.go:1568`) | `Nondeterministic: true` |
| Write / delete / money | none — Rego is a pure decision language; no builtin mutates state | n/a |

The absence of the write/delete/execute classes is a legitimate design consequence: OPA evaluates policies and returns decisions, it does not act. `http.send` is the sole outbound side-effect channel, and `opa.runtime` is the sole host-secret channel.

**2. Are permissions enforced?**

Partially, and at two different layers with different strengths:

- **Compile time (strong, tested):** capabilities restrict the compiler's builtin table (`v1/ast/compile.go:1997-2005`), so omitted builtins fail `CheckUndefinedFuncs`/type checking (`v1/ast/compile.go:1483-1510`). The deprecated denylist adds `CheckUnsafeBuiltins` (`v1/ast/compile.go:7208-7220`). Both are covered by tests (`v1/ast/compile_test.go:12254-12280`, `v1/rego/rego_test.go:1962-2072`), including the `with`-mock bypass (`v1/ast/compile.go:7146-7152`, `v1/ast/compile_test.go:11413`).
- **Eval time (only `allow_net`):** `verifyHost` (`v1/topdown/http.go:374-383`) is the single runtime gate, tested at `v1/topdown/http_test.go:820` and `v1/topdown/net_test.go:192`. Everything else is unenforced at eval time — `eval.builtinFunc` consults the *global* registries, not the capabilities set (`v1/topdown/eval.go:211-224`).

Where it is **decorative**: `Deprecated` only produces errors under `--strict` or `rego.v1` modules (`v1/ast/compile.go:1930-1932`). `Nondeterministic` never denies anything; it only affects PE and caching (`v1/topdown/save.go:494`, `v1/topdown/eval.go:2067`). And in `opa run` (server) mode no capabilities file can be supplied at all, so the compile-time gate is unavailable in the deployment where untrusted bundles are most likely to arrive.

**3. Can users approve selectively?**

There is **no approval flow** — no confirmation prompt, no interactive gate, no per-call decision point anywhere in the source. Selectivity exists only as static, pre-execution configuration:

- Per-builtin allowlist: capabilities `builtins` array (`cmd/build.go:206-229` shows a file permitting only `plus`).
- Per-builtin denylist: `WithUnsafeBuiltins` (`v1/ast/compile.go:598-604`), overridable per-query (`v1/ast/compile.go:3848-3853`).
- Per-host allowlist for the network tool: `allow_net` (`v1/ast/capabilities.go:100`).
- Per-feature allowlist: `Features` (`v1/ast/capabilities.go:2118-2119`, `v1/ast/compile.go:2300`).
- Off-by-default side effect: `print()` erased unless enabled (`v1/ast/compile.go:2641-2648`).

The nearest thing to "user approves this action" is the server authorizer, where a human-written Rego policy adjudicates each API request and may return a `reason` (`v1/server/authorizer/authorizer.go:140-160`) — but that gates *requests to OPA*, not *builtins OPA invokes*.

**4. Are approvals persisted?**

No approval state exists, so nothing is persisted. What *is* persisted is **policy**, not approval: capabilities are serialized JSON (`v1/ast/capabilities.go:197-233`), versioned per release under `v1/capabilities/` and loaded from an embedded FS (`v1/ast/capabilities.go:206-223`), and passed by path or version on the command line (`cmd/flags.go:220-230`). This gives durable, reviewable, diffable permission files — a genuine strength — but it is configuration, not a record of granted consent. There is no audit trail of "builtin X was allowed at time T by principal P"; the denial itself surfaces only as a compile error string (`v1/ast/compile.go:7214`) or an `eval_builtin_error` (`v1/topdown/http_test.go:820`).

**5. Can policy block a model-requested tool?**

Reframed for OPA: *can the runtime block a policy-requested builtin?* **At compile time, yes; at eval time, only for network hosts.**

- A restricted capabilities file makes the policy fail to compile — the builtin never reaches the evaluator (`v1/ast/compile.go:1997-2005` + `v1/ast/compile.go:467`). This is the mechanism the Rego Playground uses to disable `http.send` (`docs/docs/errors/rego-type-error/unsafe-built-in-function-calls-in-expression-name.md:14-20`).
- Once a module is compiled with permissive capabilities, the evaluator will happily dispatch any registered builtin: `builtinFunc` looks it up in `ast.BuiltinMap`/`builtinFunctions` (`v1/topdown/eval.go:211-224`) with no capability consultation before invocation (`v1/topdown/eval.go:1048-1051`). Only `http.send`/`net.lookup_ip_addr` re-check policy, via `BuiltinContext.Capabilities.AllowNet` (`v1/topdown/http.go:374-399`, `v1/topdown/net.go:27`).
- And `allow_net` fails **open**: `if bctx.Capabilities == nil || bctx.Capabilities.AllowNet == nil { return nil }` (`v1/topdown/http.go:375-377`), with the semantics spelled out in the struct comment "If omitted, ANY host can be connected to" (`v1/ast/capabilities.go:96`).

So the answer to the dimension's headline question — "Can the runtime stop a high-risk tool even if the model asks for it?" — is: **only if the operator pre-declared restricted capabilities at compile time, and not in `opa run` server mode where the flag does not exist.**

## Architectural Decisions

1. **Allowlist over denylist, with the denylist explicitly deprecated.** `WithUnsafeBuiltins` is annotated "Deprecated: Use WithCapabilities instead" (`v1/ast/compile.go:598-600`) and its backing field carries the same note (`v1/ast/compile.go:147`). Capabilities are the sanctioned mechanism (`v1/ast/compile.go:565-570`). Allowlists fail closed against newly added builtins, which is the safer default as the builtin surface grows.

2. **Enforcement placed in the compiler, not the evaluator.** Permission checking is a pipeline stage (`v1/ast/compile.go:475`, `v1/ast/compile.go:467`) rather than a per-call interceptor. This makes denial static, cheap (zero eval overhead), and diagnosable with source locations (`v1/ast/compile.go:7214` passes `x.Loc()`), at the cost of no defence-in-depth once compilation has happened.

3. **Permission metadata is co-located with the type declaration.** `Capabilities.Builtins` is `[]*Builtin` (`v1/ast/capabilities.go:85`) — the same struct used for type checking (`v1/ast/builtins.go:3603`). One artifact therefore describes both the signature and the grant; there is no separate policy document that can drift from the registry.

4. **Capabilities double as a compatibility contract, not purely a security contract.** `MinimumCompatibleVersion` (`v1/ast/capabilities.go:252-290`) and `buildRequiredCapabilities` (`v1/ast/compile.go:1201-1210`) use the same structure to answer "which OPA versions can run this bundle?". This dual purpose is why risk fields never got added — the schema was designed for version negotiation first.

5. **One narrow runtime gate, threaded through the builtin context.** Rather than a general policy hook, `allow_net` is passed to builtins via `BuiltinContext.Capabilities` (`v1/topdown/builtins.go:60`, populated at `v1/topdown/eval.go:1070-1097`) and each network builtin opts in by calling `verifyHost` (`v1/topdown/net.go:27`, `v1/topdown/http.go:459`). Enforcement is therefore per-builtin voluntary code, not a framework guarantee.

6. **Side effects off by default where the cost is zero.** `print()` is erased at compile time unless explicitly enabled (`v1/ast/compile.go:2643-2648`), and the field comment states elision is the default (`v1/ast/compile.go:149`). This is the cleanest instance of a gated capability in the codebase.

7. **Request-level authorization delegated to Rego itself.** The server gates its own API with a policy decision (`v1/server/authorizer/authorizer.go:116-163`) rather than hard-coded roles — dogfooding, and it fails closed when the decision is undefined (`v1/server/authorizer/authorizer.go:134-138`).

## Notable Patterns

- **Capability object threaded from CLI → compiler → BuiltinContext.** `cmd/flags.go:220-230` → `rego.Capabilities` (`v1/rego/rego.go:1327-1333`) → `Compiler.WithCapabilities` (`v1/ast/compile.go:571`) → `BuiltinContext.Capabilities` (`v1/topdown/eval.go:1097`) → `verifyHost` (`v1/topdown/http.go:374`). A single value carries permission state across all layers.
- **Pipeline stage as policy enforcement point.** Each check is a named, individually skippable stage with a metric name (`v1/ast/compile.go:475-479`); `StageID` constants make the enforcement points enumerable (`v1/ast/compile.go:212`, `v1/ast/compile.go:254`).
- **Bypass closure at the mock boundary.** `with` replacement of builtins is validated against the same unsafe map (`v1/ast/compile.go:7017-7026`, `v1/ast/compile.go:7146-7152`) — recognition that a mocking feature is a permission bypass vector.
- **Embedded, versioned policy baselines.** Capabilities JSON files loaded from `caps.FS` and semver-sorted (`v1/ast/capabilities.go:206-223`, `v1/ast/capabilities.go:236-250`), so a permission baseline ships with the binary.
- **Metadata reuse across concerns.** `Nondeterministic` serves PE (`v1/topdown/save.go:494`), caching (`v1/topdown/eval.go:2067`), and — informally — impurity documentation. Compact, but it means no field is authoritative for risk.
- **Fail-open guard clause.** `if ... AllowNet == nil { return nil }` (`v1/topdown/http.go:375`, repeated at `v1/topdown/http.go:388`) — a recurring shape where absent policy means unrestricted.
- **Fail-closed guard clause (contrast).** The authorizer's terminal `writer.Error(w, http.StatusUnauthorized, ...)` after all allow-branches (`v1/server/authorizer/authorizer.go:163`) — the opposite default, in the same repo.
- **Registry escape hatches.** `RegisterFeatures` (`v1/ast/capabilities.go:73-80`) and `WithBuiltins` (`v1/ast/compile.go:592-596`) mutate/extend the grant surface from embedder code; `maps.Copy(c.builtins, c.customBuiltins)` (`v1/ast/compile.go:2007`) applies custom builtins *after* the capabilities-derived table, so custom builtins are not capability-filtered.

## Tradeoffs

| Decision | Gain | Cost |
|----------|------|------|
| Compile-time enforcement (`v1/ast/compile.go:475`) | Zero eval-time overhead; errors carry source locations (`v1/ast/compile.go:7214`) | No defence-in-depth; an already-compiled module is unconstrained (`v1/topdown/eval.go:211-224`) |
| Allowlist model (`v1/ast/compile.go:1997-2005`) | New builtins are denied by default in an old capabilities file | Operators must maintain a full allowlist; verbose (`cmd/build.go:206-229` needs ~25 lines to permit one function) |
| Permissive default (`v1/ast/compile.go:1993-1995`, `cmd/flags.go:208`) | Zero-config usability; `opa eval` just works | Insecure by default — `http.send` and `opa.runtime` (env vars) available unless restricted |
| `allow_net` fails open (`v1/topdown/http.go:375`) | Backwards compatible; no breakage for existing deployments | An operator who supplies a capabilities file without `allow_net` gets unrestricted egress while believing they configured permissions |
| Reusing `Builtin` for permissions (`v1/ast/capabilities.go:85`) | Single source of truth; signature and grant cannot drift | Schema is shaped by type-checking/versioning needs, leaving no room for risk tiers or approval flags |
| Voluntary per-builtin runtime checks (`v1/topdown/net.go:27`) | Minimal, targeted; no interception cost on pure builtins | A new network-capable builtin that forgets `verifyHost` silently escapes `allow_net` |
| No `--capabilities` on `opa run` | Simpler server config surface | The long-running, bundle-consuming deployment mode cannot restrict builtins at all |
| Deprecating the denylist (`v1/ast/compile.go:598`) | One canonical mechanism | Denylist ergonomics ("allow everything except `http.send`") are lost; the deprecated path remains the only concise way to express that |
| `print()` erased by default (`v1/ast/compile.go:2643`) | No accidental log/PII leakage from policies | Debugging requires a rebuild/reconfigure with `WithEnablePrintStatements` |

## Failure Modes / Edge Cases

1. **Capabilities supplied without `allow_net` ⇒ unrestricted egress.** `verifyHost` returns `nil` when `AllowNet == nil` (`v1/topdown/http.go:375-377`). The distinction between "omitted" (allow all) and "empty array" (allow none) is documented only in a struct comment (`v1/ast/capabilities.go:96-97`) and CLI help (`cmd/eval.go:293-295`). A one-key mistake silently disables the control.

2. **Compiled-then-evaluated bypass.** `eval.builtinFunc` never consults capabilities (`v1/topdown/eval.go:211-224`). Any workflow that compiles with default capabilities and evaluates with restricted ones gets no builtin restriction. The permission check is not idempotent across the compile/eval boundary.

3. **Server mode cannot restrict builtins.** No `capabilities` reference exists in `cmd/run.go` or `v1/runtime/`. A bundle pushed to a running OPA server may call `http.send` or `opa.runtime` regardless of operator intent.

4. **`opa.runtime` leaks environment variables with no gate.** Its own description says it "includes ... an `env` key containing the environment variables that the OPA process was started with" (`v1/ast/builtins.go:3204`). It is marked only `Nondeterministic: true` (`v1/ast/builtins.go:3206`) — there is no `allow_env`-style analogue to `allow_net`. The only defence is omitting it from the capabilities allowlist at compile time.

5. **Custom builtins are not capability-filtered.** `maps.Copy(c.builtins, c.customBuiltins)` runs *after* the capabilities loop (`v1/ast/compile.go:1999-2007`), so embedder-registered builtins bypass the allowlist and can even overwrite a capability-declared entry of the same name.

6. **Global feature list is mutable at process scope.** `RegisterFeatures` appends to the package-level `Features` slice (`v1/ast/capabilities.go:65-80`), which `CapabilitiesForThisVersion` then copies (`v1/ast/capabilities.go:187-188`). Any imported library can widen the default grant for the whole process.

7. **Unix-socket and redirect paths in `http.send`.** `verifyURLHost` is called on the raw URL string (`v1/topdown/http.go:459`) and on the constructed request (`v1/topdown/http.go:668`), but the socket-rewriting path sets `u.Scheme = "http"` and dials a filesystem socket (`v1/topdown/http.go:355-371`) — a code path where the host allowlist has no meaning, since the target is a local file. `allow_net` cannot express "no local sockets".

8. **Deprecation warnings are conditional.** `checkDeprecatedBuiltins` is a no-op unless `c.strict` or the module is `rego.v1` (`v1/ast/compile.go:1930-1932`), and it short-circuits entirely when no required builtin is deprecated (`v1/ast/compile.go:1919-1928`). In default v0 non-strict mode, `Deprecated: true` metadata has no runtime consequence.

9. **Denylist is per-query overridable.** `queryCompiler.unsafeBuiltinsMap()` prefers the query-level map when non-nil (`v1/ast/compile.go:3848-3853`), so a caller with access to `QueryCompiler.WithUnsafeBuiltins` can substitute an empty map and drop the compiler-level restriction.

10. **Nondeterministic builtins can be re-enabled during partial evaluation.** `NondeterministicBuiltins(true)` (`v1/rego/rego.go:1004-1009`) causes side-effecting builtins to actually execute during PE (`v1/topdown/save.go:374`), and this is reachable over HTTP via the `nondeterministicBuiltins` request option (`v1/server/types/types.go:517`, `v1/server/server.go:1468`) — an API-controllable toggle that increases side effects at compile/PE time.

11. **Denial is an error string, not a structured event.** `unsafe built-in function calls in expression: %v` (`v1/ast/compile.go:7214`) and `unallowed host: %s` (`v1/topdown/http.go:383`) carry no machine-readable policy identifier, so denials cannot be reliably aggregated or alerted on.

## Future Considerations

Concrete work items, each anchored to a location:

1. **Add explicit risk metadata to `Builtin`** (`v1/ast/builtins.go:3594-3609`): e.g. `Network bool`, `HostState bool`, `Impure bool`, or a `RiskClass string` enum, so risk stops being inferred from `Nondeterministic` (which exists for PE/caching, `v1/topdown/save.go:494`). Emit it into `builtin_metadata.json` and the per-version capabilities files so tooling can lint policies by risk class.

2. **Re-check the builtin allowlist at dispatch.** In `eval.builtinFunc` (`v1/topdown/eval.go:211-224`) or immediately before invocation (`v1/topdown/eval.go:1048-1051`), consult `e.bctx.Capabilities.ContainsBuiltin(name)` (the helper already exists at `v1/ast/capabilities.go:296-300`) and return a permission error. This closes failure mode #2 and gives defence-in-depth.

3. **Wire `--capabilities` into `opa run`.** Add the flag via `addCapabilitiesFlag` (`cmd/flags.go:138`) in `cmd/run.go` and plumb it through `v1/runtime` into the server's compiler, mirroring `cmd/eval.go:874-880`. Without this, the primary production deployment mode has no builtin restriction at all.

4. **Make `allow_net` fail closed under an explicit opt-in.** Introduce a tri-state or a companion flag so that supplying a capabilities file can mean "deny egress unless listed", changing the guard at `v1/topdown/http.go:375-377`. Also address the TODO already in the source: "support ports to further restrict connection peers" (`v1/ast/capabilities.go:99`), and extend the check to the unix-socket path (`v1/topdown/http.go:355-371`).

5. **Add an `allow_env` / host-state allowlist for `opa.runtime`.** Given its documented env-var exposure (`v1/ast/builtins.go:3204`), add a runtime gate analogous to `verifyHost` (`v1/topdown/http.go:374`) so specific keys can be redacted rather than requiring wholesale removal from the allowlist.

6. **Filter custom builtins through capabilities.** Reorder or validate at `v1/ast/compile.go:1999-2007` so `customBuiltins` cannot silently widen or shadow the declared grant.

7. **Emit structured denial events.** Give `checkUnsafeBuiltins` (`v1/ast/compile.go:7208`) and `verifyHost` (`v1/topdown/http.go:383`) machine-readable codes/fields so denials are observable and countable, not just human-readable strings.

8. **Add a generic pre-invocation builtin hook.** A `BuiltinContext`-level interceptor (extending `v1/topdown/builtins.go:60`) invoked before `evalBuiltin.eval` (`v1/topdown/eval.go:2072+`) would let embedders implement per-call policy — the closest OPA could come to an approval gate — and would make new network builtins safe by default rather than depending on each one remembering to call `verifyHost` (`v1/topdown/net.go:27`).

9. **Provide a supported denylist idiom.** Since `WithUnsafeBuiltins` is deprecated (`v1/ast/compile.go:598`) but the "allow all except X" use case is real (the Playground's `http.send`, per `docs/docs/errors/rego-type-error/unsafe-built-in-function-calls-in-expression-name.md:17-18`), add a `deny_builtins` key to `Capabilities` (`v1/ast/capabilities.go:84-101`) or a helper that derives an allowlist by subtraction from `CapabilitiesForThisVersion()` (`v1/ast/capabilities.go:147`).

10. **Test the negative path for restricted capabilities end to end.** Existing tests cover the deprecated denylist (`v1/ast/compile_test.go:12254-12280`, `v1/rego/rego_test.go:1962`) and features (`v1/ast/compile_test.go:12115-12138`), but a test asserting that a capabilities file omitting `http.send` blocks it through `opa build`/`opa eval` — and that the same policy is blocked at *eval* time — would pin the intended security guarantee.

## Questions / Gaps

- **No approval/confirmation concept exists.** Searched for approval, confirmation, and consent semantics across the source; the only adjudication points found are the compile-time capability/unsafe checks (`v1/ast/compile.go:475`, `v1/ast/compile.go:7208`), the `allow_net` runtime check (`v1/topdown/http.go:374`), and the server authorizer (`v1/server/authorizer/authorizer.go:107`). **No evidence found** of an interactive or deferred approval mechanism, and none is expected — OPA is a non-interactive decision engine.
- **No persisted approval state, no denial audit log.** No evidence found of a store or log recording granted/denied permissions; the storage layer (`storage/`, `v1/storage/`) was not examined for this, as capabilities are file-based configuration (`v1/ast/capabilities.go:225-233`) rather than stored state.
- **`opa run` capability plumbing.** I confirmed by grep over `cmd/*.go` (matches only in `bench.go`, `build.go`, `capabilities.go`, `check.go`, `eval.go`, `flags.go`, `fmt.go`, `test.go`) and over `v1/runtime/*.go`, `v1/sdk/*.go`, `v1/plugins/*.go` (no matches) that server mode has no capabilities wiring. I did not exhaustively read `v1/runtime/runtime.go`, so an indirect path (e.g. via a config key rather than a flag) cannot be fully excluded — but no `Capabilities` symbol appears in that package.
- **Wasm/target-specific builtin restrictions.** `WasmABIVersions` is part of `Capabilities` (`v1/ast/capabilities.go:87`, populated at `v1/ast/capabilities.go:152-154`), and some builtins are presumably unavailable in the Wasm target, which would be a de facto capability difference per compilation target. I did not analyze `internal/wasm/` or `wasm/` for a builtin support matrix; **no evidence gathered** on whether Wasm-unsupported builtins are rejected at build time or fail at runtime.
- **Bundle signing as an authorization analogue.** `keys/` and bundle verification exist in the tree and would be the mechanism for "who is allowed to supply this policy", which is arguably the real approval gate for a server deployment. Not analyzed here because the dimension scopes to per-tool permission metadata; **no evidence collected** on signature enforcement strength.
- **Whether any shipped capabilities file actually restricts anything.** The per-release files under `v1/capabilities/` appear to be full snapshots for version negotiation (consistent with `MinimumCompatibleVersion`, `v1/ast/capabilities.go:252-290`) rather than hardened profiles. I did not diff their contents against `CapabilitiesForThisVersion()`, so I cannot confirm that OPA ships any restricted/hardened baseline profile.
- **Whether `Categories` is complete.** The source comment concedes the category model is partly unrealized: `"minus" for example, is part of two categories: numbers and sets. (NOTE(sr): aspirational)` (`v1/ast/builtins.go:3598-3601`). Any tooling built on categories would be building on an admittedly incomplete taxonomy.

---

Generated by `dimensions/04.05-tool-permissions-approval-metadata.md` against `opa`.
