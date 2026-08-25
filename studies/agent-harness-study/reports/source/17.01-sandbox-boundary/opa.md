# Source Analysis: opa

## Dimension 17.01: Sandbox Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (evaluator, server, CLI), Rego (policy language), WebAssembly + Rust/C toolchain for the Wasm target |
| Analyzed | 2026-08-24 |

*All file paths below are relative to the selected source directory `studies/agent-harness-study/sources/opa`.*

## Summary

OPA executes user-authored Rego policies **inside its own host process**: a Go-implemented tree-walking interpreter ("topdown", `v1/topdown/eval.go`) used by the CLI (`cmd/`), the HTTP server (`v1/server/server.go`), and the embedding SDK (`sdk/opa.go`). There is no process-, container-, or VM-level isolation around native policy evaluation; instead OPA draws the boundary at the **language level** — policies can only act through an explicit allowlist of built-in functions resolved from a versioned `Capabilities` set (`v1/ast/capabilities.go:84-101`, enforced in `v1/ast/compile.go:2038-2050`) — plus targeted runtime checks: network egress allowlisting for `http.send` including redirects (`v1/topdown/http.go:402-423, 475, 647-648`), compile-time elision of `print()` unless explicitly enabled (`v1/ast/compile.go:2685-2692`), and a server-side ban of `http.send` in ad-hoc API queries (`v1/server/server.go:104, 996`). A second execution target compiles policies to WebAssembly, which runs in a wazero VM with no OS imports and builtins re-exported through a host module (`internal/wasm/sdk/internal/wasm/vm.go:57-98`) — this is the only genuinely sandboxed execution mode, and it is opt-in. Defaults are permissive (any-host network egress, full process-environment exposure via `opa.runtime()`), so the strength of the boundary depends almost entirely on operator-supplied capability configuration.

## Rating

**7 / 10** — Clear model with explicit interfaces and tests. The builtin-allowlist boundary is coherent, centrally defined (`Capabilities`), compile-time enforced, and covered by targeted tests (unsafe-builtins rejection: `v1/server/server_test.go:1264`; redirect-aware egress checks: `v1/topdown/http_test.go:793, 3696`; JSON-Schema fetch restriction: `v1/topdown/jsonschema_test.go:519`). It falls short of 8+ because: (a) native evaluation shares the process fate with the host and has no resource limits by default, (b) defaults are open — `AllowNet` unset means any host (`v1/ast/capabilities.go:94-96`) and `opa.runtime().env` dumps all environment variables (`v1/runtime/info/info.go:47-58`), (c) enforcement is inconsistent across ingestion paths (bundle-loaded policies are compiled without the unsafe-builtin ban, `v1/plugins/bundle/plugin.go:627`), and (d) there is documentation drift on what `allow_net` controls (`v1/ast/capabilities.go:97-99` vs. actual use in `v1/topdown/http.go:403`).

## Evidence Collected

Every entry cites a path relative to `studies/agent-harness-study/sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Execution environment (native) | Policies evaluate in-process in the topdown interpreter; `BuiltinContext` hands each builtin request context, caches, seed, clock — no handles to files or processes | `v1/topdown/builtins.go:36-60` |
| Builtins are the only "tool" surface | `RegisterBuiltinFunc` stores Go implementations in a global registry executed by the evaluator | `v1/topdown/builtins.go:88-91` |
| Builtin allowlist from capabilities | Compiler builds `c.builtins` strictly from `capabilities.Builtins` (+ custom); unknown builtins cannot compile | `v1/ast/compile.go:2038-2050` |
| Compile-time unsafe-builtin ban | `checkUnsafeBuiltins` emits type error for banned operators during compilation of every module | `v1/ast/compile.go:1959`, `7558-7570` |
| Server bans `http.send` in ad-hoc queries | Hardcoded `unsafeBuiltinsMap = {http.send}` applied to data/query/compile handlers | `v1/server/server.go:104`, `996`, `1482`, `2668`; `v1/server/compile_handler.go:271` |
| Test: server rejects `http.send` queries | Expected error `"unsafe built-in function calls in expression: http.send"` | `v1/server/server_test.go:1264`, `4901` |
| Network egress allowlist (runtime) | `verifyHost`/`verifyURLHost` reject hosts not in `capabilities.AllowNet`; checked on initial URL and again on every redirect | `v1/topdown/http.go:402-423`, `475`, `647-648` |
| Capabilities flow into evaluation | `BuiltinContext.Capabilities` populated from the query compiler at eval time | `v1/topdown/eval.go:1058-1085` |
| Default-permissive egress | "`allow_net` … If omitted, ANY host can be connected to. If empty, NO host" — with TODO noting only ports unsupported | `v1/ast/capabilities.go:94-101` |
| Print/output gating | `print()` calls erased at compile time unless `WithEnablePrintStatements(true)`; runtime no-op when no print hook installed | `v1/ast/compile.go:147`, `2685-2692`; `v1/topdown/print.go:30-33` |
| Recursion rejected (no runaway self-invocation) | Dedicated compiler stage + DFS cycle detection producing `RecursionErr` | `v1/ast/compile.go:471`, `1290-1316` |
| Host-info disclosure via `opa.runtime()` | Runtime document includes full `os.Environ()` as `env`, plus config; builtin flagged `Nondeterministic` | `v1/runtime/info/info.go:47-62`; `v1/ast/builtins.go:3236-3246` |
| Wasm execution environment | Policy compiled to Wasm module instantiated in wazero runtime; `WithCloseOnContextDone(true)` interrupts tight wasm loops on cancel; ABI version gate (1.x ≥ .1) | `internal/wasm/sdk/internal/wasm/vm.go:57-63`, `84-114` |
| Wasm host boundary | Only imports come from an `env` glue module; real host functions live in an `opa` wazero host module wired through a builtin dispatcher | `internal/wasm/sdk/internal/wasm/vm.go:71-79`; `bindings.go:33`; `glue.go:15-23` |
| Wasm target selection | `targetWasm = "wasm"`; engine looked up via `opa.LookupEngine(targetWasm)` | `v1/rego/rego.go:46`, `1850-1864` |
| Inconsistent enforcement across ingestion paths | Bundle plugin and manager compile stored policies with plain `ast.NewCompiler()` — no unsafe-builtin ban, so bundle policies may call `http.send` | `v1/plugins/bundle/plugin.go:627`; `v1/plugins/plugins.go:1160-1161` |
| Per-run configurability | `--capabilities <version\|file>` flag on eval/build/check/test/fmt/oracle commands; loads capabilities JSON or named release | `cmd/flags.go:138-139`, `199-230`; `cmd/eval.go:368`; `cmd/build.go:291`; `cmd/check.go:211`; `cmd/test.go:623`; `cmd/fmt.go:346` |
| Versioned capability snapshots | 100+ generated `capabilities/vX.Y.Z.json` files plus root `capabilities.json` shipped in-repo | `capabilities/v0.17.0.json` … `capabilities/v1.18.2.json`; `capabilities.json` |
| Deployment packaging (not isolation) | Root Dockerfile packages the binary as a non-root container image; no sandboxing logic inside | `Dockerfile:7-12` |
| Stated design goal for Wasm target | Blog/IR doc describes Wasm as "a safe, _sandboxed_ environment" where host/network/filesystem interaction is prohibited | `docs/blog/2022-10-20-i-have-a-plan-exploring-the-opa-intermediate-representation-ir-format-7319cd94b37d.md:29` |
| Wasm docs: builtin coverage limits | `http.send` not natively supported in Wasm; unsupported builtins must be implemented by the host environment | `docs/docs/wasm.md:22-27` |

## Answers to Dimension Questions

**1. Where does code execute?**
In the same OS process that embeds OPA. Three native entrypoints share one evaluator: the CLI (`cmd/eval.go` etc.), the server's query handlers (`v1/server/server.go:978-1004` builds `rego.Rego` options inline), and the Go SDK (`sdk/opa.go`). Policy "tools" are built-in functions implemented in Go and dispatched inside the interpreter loop via a global registry (`v1/topdown/builtins.go:88-91`, invoked from `v1/topdown/eval.go`'s expression evaluation). Plugins (bundle/discovery/status/logs) run as goroutines within the manager (`v1/plugins/plugins.go:903, 1098`). The alternative execution environment is WebAssembly: `opa build -t wasm` produces a `.wasm` module (`docs/docs/wasm.md:31-39`) evaluated either by an embedded wazero VM (`internal/wasm/sdk/internal/wasm/vm.go:57-63`) or by any external Wasm host (JS SDKs under `wasm/`, built reproducibly in Docker per `wasm/README.md:20-25`).

**2. What boundaries exist between agents and the host?**
- *Language-level capability boundary*: Rego has no arbitrary-code, exec, or filesystem primitives; everything a policy can do must be a declared builtin present in the compiler's `c.builtins` map sourced from `Capabilities.Builtins` (`v1/ast/compile.go:2038-2050`). The `BuiltinContext` passed to builtins exposes only request context, metrics, seed, clock, caches, runtime term, and capabilities — no file descriptors or process handles (`v1/topdown/builtins.go:36-60`).
- *Output boundary*: `print()` is compiled away unless enabled (`v1/ast/compile.go:2685-2692`), and even then requires a caller-installed `PrintHook` (`v1/topdown/print.go:30-33`).
- *Network boundary*: `capabilities.AllowNet` gates `http.send` targets, re-verified across redirects (`v1/topdown/http.go:402-423, 475, 647-648`) and JSON-Schema remote fetches (`v1/topdown/jsonschema.go:69`).
- *Server query boundary*: ad-hoc Data/Query/Compile API calls reject `http.send` entirely (`v1/server/server.go:104, 996`).
- *Structural Wasm boundary*: policy modules import nothing but an `env` memory-glue module; all side effects flow back through the `opa` host-module dispatcher (`internal/wasm/sdk/internal/wasm/glue.go:15-23`).

**3. Are boundaries enforced?**
Yes, at three layers, with test coverage: compile-time (unknown/banned builtins fail compilation, `v1/ast/compile.go:1959, 7558-7570`; recursion rejected, `v1/ast/compile.go:1290-1316`), runtime (host verification on request and redirect, `v1/topdown/http.go:403, 420-422, 647-648`; print hook nil-check, `v1/topdown/print.go:31-32`; wazero closes tight loops when context is cancelled, `vm.go:58-59`), and API-layer (server unsafe-builtin map, tested at `v1/server/server_test.go:1264`). Enforcement is *uneven*, however: the ban applies to ad-hoc server queries only — policies delivered via bundles are compiled with a bare `ast.NewCompiler()` (`v1/plugins/bundle/plugin.go:627`, `v1/plugins/plugins.go:1160-1161`), so they retain full builtin access including `http.send`.

**4. Can sandbox configuration be changed per-run?**
Yes, extensively, via first-class interfaces: the `--capabilities` flag accepts either a released version name or a custom `capabilities.json` path on `eval`/`build`/`check`/`test`/`fmt` (`cmd/flags.go:138-139, 220-230`); library embedders use `Compiler.WithCapabilities` (`v1/ast/compile.go:569-572`) and `RegisterFeatures` (`v1/ast/capabilities.go:73-80`). `AllowNet` is part of the same structure (`v1/ast/capabilities.go:100`). Limits: the server's unsafe-builtin set is hardcoded (`v1/server/server.go:104`) and not configurable; `WithUnsafeBuiltins` is deprecated in favor of capabilities (`v1/ast/compile.go:596-602`); and there is no per-request toggle — configuration is fixed at process start or compile call.

**Can an agent escape its intended execution boundary?**
Not through memory unsafety or undeclared syscalls in the native evaluator — there is no such surface; escapes are *policy-level disclosures enabled by defaults*: (a) any policy can read the entire process environment via `opa.runtime().env` (`v1/runtime/info/info.go:47-58`), a plausible secrets-leak vector; (b) `http.send` reaches any host unless operators supply an `allow_net` list (`v1/ast/capabilities.go:96`), enabling SSRF-style probing from bundle-loaded policies; (c) resource exhaustion is bounded only cooperatively (`Cancel` in `v1/topdown/builtins.go:41`; wazero's close-on-context-done is Wasm-only). The Wasm target structurally prevents these classes except for builtins the host chooses to implement.

## Architectural Decisions

- **Interpreter-in-process over isolated workers.** OPA evaluates policies directly in the serving binary for latency and embedding simplicity; the cost is shared fate with the host process, mitigated only by the language's lack of I/O primitives.
- **Capabilities as the single control plane.** One JSON-described structure (`v1/ast/capabilities.go:84-101`) governs available builtins, language features, Wasm ABI versions, and network allowlist; `WithUnsafeBuiltins` was deprecated in its favor (`v1/ast/compile.go:596-602`), consolidating boundary configuration.
- **Compile-time-first enforcement.** Most restrictions (builtin availability, recursion, print elision) are decided before evaluation, so violations fail fast with precise locations rather than being policed at runtime.
- **Opt-in stronger isolation via Wasm.** Rather than sandboxing the native engine, OPA offers a separate compilation target whose host-interaction story is intentionally minimal (`docs/docs/wasm.md:22-27`), accepting reduced builtin coverage (`http.send` unsupported natively).
- **Narrow, explicit builtin context.** All ambient authority flows through one struct (`BuiltinContext`, `v1/topdown/builtins.go:36-60`) assembled in exactly one place per evaluation (`v1/topdown/eval.go:1058-1085`), making audits of "what can a builtin touch" tractable.

## Notable Patterns

- **Capability snapshotting per release**: generated `capabilities/vX.Y.Z.json` files let operators pin policies to exact engine abilities and validate compatibility at build time (`cmd/build.go:220-256` documents this workflow).
- **Defense-in-depth on redirects**: the egress check is reapplied inside the HTTP client's `CheckRedirect` hook (`v1/topdown/http.go:647-648`), preventing bypass via cross-host redirects — a detail most allowlist implementations miss.
- **Compile-away rather than runtime-deny**: `print()` becomes `erasePrintCalls` output when disabled (`v1/ast/compile.go:2687-2692`) — cheaper than runtime gating and immune to hook misconfiguration.
- **Host-module pattern for Wasm builtins**: guest modules see a single `env` import surface; real implementations stay behind a registered wazero host module and dispatcher (`internal/wasm/sdk/internal/wasm/bindings.go:33`, `glue.go:15-23`), keeping guest/host traffic inspectable at one chokepoint.
- **ABI gating**: Wasm modules are rejected unless they declare supported ABI versions (`vm.go:106-114`), protecting the host/guest contract.

## Tradeoffs

- **Speed vs. containment**: in-process evaluation avoids IPC overhead but means a panicking builtin or OOM takes down the host; no supervisor boundary exists.
- **Open-by-default vs. operability**: unrestricted `http.send` and full env exposure make simple deployments work out of the box but shift the burden of restriction onto operators who often don't know `allow_net` exists (`v1/ast/capabilities.go:94-99`).
- **Consistency vs. flexibility across ingestion paths**: bundles skip the unsafe-builtin ban so policy authors keep full expressiveness; the price is that the server's ad-hoc protection creates a false sense of a uniform egress policy.
- **Wasm safety vs. feature parity**: the sandboxed target cannot run network builtins natively (`docs/docs/wasm.md:24-27`), forcing hosts to reimplement them outside OPA's audited dispatcher.

## Failure Modes / Edge Cases

- **Environment disclosure**: any evaluatable policy (including ones submitted to the Data API, which does not ban `opa.runtime`) reads all env vars, including injected secrets (`v1/runtime/info/info.go:49-56`). Flagged `Nondeterministic` (`v1/ast/builtins.go:3244`) but nondeterminism marking does not restrict availability by itself.
- **SSRF from bundle policies**: `AllowNet == nil` permits any host (`v1/ast/capabilities.go:96`); combined with bundle compilation lacking the unsafe-builtin ban (`v1/plugins/bundle/plugin.go:627`), a malicious bundle author gets arbitrary outbound HTTP from the OPA process's network position.
- **Doc drift invites miscalibration**: the `allow_net` comment still claims it "only controls fetching remote refs for using JSON Schemas" (`v1/ast/capabilities.go:97-99`) although it now also gates `http.send` (`v1/topdown/http.go:403`) — operators reading the struct may under- or over-trust it.
- **Resource exhaustion**: no memory/CPU ceiling for native evaluation found anywhere in `v1/topdown/`; cancellation is cooperative via `Cancel` (`v1/topdown/builtins.go:41`). Only the Wasm target hard-interrupts tight loops (`vm.go:58-59`).
- **Custom builtins widen the boundary silently**: `RegisterBuiltinFunc` mutates a global registry (`v1/topdown/builtins.go:89-91`); any linked Go package that also injects the name into capabilities makes new host access reachable from policies, with no central review point.

## Future Considerations

- Honor port-level granularity in `allow_net` — already acknowledged as missing in-source (`v1/ast/capabilities.go:99` TODO).
- Extend the hardcoded server `unsafeBuiltinsMap` into configuration so operators can ban additional builtins (e.g., `opa.runtime`) per deployment.
- Reconcile the bundle-ingestion compiler with the server's unsafe-builtin posture, or document the asymmetry as intentional.
- Add default-on guardrails: deny `opa.runtime().env` unless opted in (mirroring the `print()` opt-in pattern), and consider a topdown evaluation budget analogous to wazero's context-done interruption.

## Questions / Gaps

- No evidence found of OS-level confinement (seccomp, namespaces, chroot) applied by OPA itself to native evaluation; searched `v1/topdown/`, `v1/runtime/`, `cmd/`, and build scripts. The root `Dockerfile` provides non-root packaging only.
- No evidence found of per-query or per-bundle sandbox profile overrides in the server config schema (`config/`); boundary tuning exists only at CLI/library level via capabilities flags.
- Whether `opa.runtime().env` exposure is considered acceptable risk is undocumented in-repo; no hardening guidance beyond the SECURITY.md reporting process (no threat-model statement on sandbox boundaries was found in `SECURITY.md` or `docs/docs/security.md`).
- The exact upstream status of WASI-based builtin support for Wasm (mentioned prospectively in `docs/blog/2022-10-20-i-have-a-plan-exploring-the-opa-intermediate-representation-ir-format-7319cd94b37d.md:29`) could not be confirmed from current code — no WASI imports appear in `internal/wasm/sdk/`.

---

Generated by dimension `17.01-sandbox-boundary` against source `studies/agent-harness-study/sources/opa`.
