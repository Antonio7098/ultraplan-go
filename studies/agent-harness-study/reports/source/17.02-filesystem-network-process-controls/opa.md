# Source Analysis: opa

## Dimension 17.02 — Filesystem, Network, and Process Controls

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (policy engine, HTTP server, Rego evaluator, CLI; embeddable SDK + WASM target) |
| Analyzed | 2026-08-26 |

## Summary

OPA is not an LLM agent harness; the "agent" here is an OPA server/CLI process evaluating Rego policies. Its containment model is therefore **deny-by-construction for filesystem and process access from policies, opt-in network egress with a capability-based host allowlist**:

- **Filesystem**: Rego has *no* file-reading builtin at all (the default capability set in `capabilities.json` enumerates 206 builtins, none of which touch the filesystem), so policies cannot read or write files. The OPA process itself reads/writes ordinary OS paths (policy load paths, REPL history, persisted bundles, optional disk store, optional file logger) with no sandbox layer.
- **Network**: Policy-initiated egress (`http.send`, `net.lookup_ip_addr`, remote JSON-schema fetch) is checked against `Capabilities.AllowNet` at evaluation time. If `allow_net` is omitted (the default shipped in `capabilities.json`), *any* host may be contacted; if set to `[]`, none may be. Runtime-initiated egress (bundle/discovery/decision-log/status services) is restricted to operator-configured service URLs with rich TLS controls but is not governed by `allow_net`.
- **Shell/process execution**: No code path lets a policy spawn processes, and the runtime itself never invokes `exec.Command` outside build tooling. The only safeguard against privileged operation is a warning when running as uid/gid 0.
- **Process/resource limits**: There are no rlimit/cgroup/memory-limit mechanisms. Resource safety is handled per-subsystem: HTTP request-body decoding limits (256 MB / gzip 512 MB defaults), a 1 GB bundle size limit to blunt gzip bombs, a 5 s default `http.send` timeout with capped retries, response-header timeouts on service clients, and bounded cache entries.
- **Cleanup**: A structured shutdown chain exists: SIGINT/SIGTERM → optional wait period → graceful server shutdown → trace exporter/meter provider shutdown → storage `Closer` → plugin `Stop` under a graceful-shutdown deadline; fsnotify watchers are closed on context cancellation.

Overall: strong, tested gating of *policy-initiated* network access; safe-by-absence filesystem/process surfaces for policies; but weak process-level hardening (root only warns, no memory/CPU governance) and an allow-any-host network default.

## Rating

**6 / 10**

Rationale against the rubric:

- The policy-egress control model is explicit, documented (`docs/docs/operations.md:138-153`), and unit-tested (`v1/topdown/http_test.go:793`, `v1/topdown/http_test.go:3696`, `v1/topdown/jsonschema_test.go:519`) — that sub-area alone merits 7–8.
- Filesystem/process controls are "safe by absence" for policies, but there are *no* enforcement mechanisms around the runtime process itself: no sandbox, no privilege dropping (only a root warning, `v1/runtime/check_user_linux.go:15-22`), no memory/CPU limits anywhere in the tree.
- Defaults favor availability over lockdown: `allow_net` absent ⇒ any host allowed (`v1/ast/capabilities.go:94-100`), authentication off by default (`v1/server/server.go:67-69`), v0-compat mode binds `:8181` on all interfaces (`cmd/run.go:29`, `cmd/run.go:389-391`).
- Cleanup paths are concrete and layered, but several safeguards rely on operators opting in.

The mix of one mature, tested control plane plus several absent/ad-hoc areas lands the aggregate at 6 ("present but inconsistent").

## Evidence Collected

Every entry cites `path/to/file.go:NN` relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Network allowlist field | `Capabilities.AllowNet` documented: omitted = ANY host, empty array = NO host | `v1/ast/capabilities.go:94-100` |
| http.send host check | `verifyHost` / `verifyURLHost` reject hosts not in `caps.AllowNet` | `v1/topdown/http.go:402-423` |
| http.send URL validation at request construction | `case "url": if err := verifyURLHost(bctx.Capabilities, strVal)` | `v1/topdown/http.go:474-477` |
| Redirect re-validation | custom `CheckRedirect` calls `verifyURLHost` when `enable_redirect` | `v1/topdown/http.go:646-650`; default no-follow redirect `v1/topdown/http.go:695-697` |
| DNS lookup gating | `builtinLookupIPAddr` calls `verifyHost(bctx.Capabilities, name)` before resolving | `v1/topdown/net.go:20-30` |
| Remote schema fetch gating | compiler passes `capabilities.AllowNet` into `loadSchema` for input schemas; JSON-schema builtin copies `AllowNet` | `v1/ast/compile.go:990-1003`, `v1/topdown/jsonschema.go:69` |
| Default capabilities ship unrestricted | root `capabilities.json` has no `allow_net` key (206 builtins listed) | `capabilities.json` |
| allow_net tests | `TestHTTPRedirectAllowNet` (nil/match/empty/no-match cases), `TestHTTPGetRequestAllowNet`, `TestBuiltinJSONSchemaAllowNet` | `v1/topdown/http_test.go:793-851`, `v1/topdown/http_test.go:3696-3727`, `v1/topdown/jsonschema_test.go:519-560` |
| Documented behavior | operations guide explains `allow_net` semantics for schemas, `http.send`, `net.lookup_ip_addr` | `docs/docs/operations.md:138-153`, also `docs/docs/policy-language.md:3715-3728` |
| Service client TLS/auth config | `Config` with `tls.ca_cert`, `allow_insecure_tls`, credentials plugins (bearer, oauth2, client_tls, s3_signing, gcp, azure, plugin) | `v1/plugins/rest/rest.go:54-75`, auth plugin contract `v1/plugins/rest/rest.go:42-51` |
| TLS hardening knobs | `DefaultTLSConfig` enforces min TLS version (`config.DefaultMinTLSVersion`) and cipher suites; `InsecureSkipVerify` only via explicit `allow_insecure_tls` | `v1/plugins/rest/auth_tls.go:26-67` |
| Client cert rotation | `clientTLSAuthPlugin.loadCertificate` re-reads cert/key files by hash/interval | `v1/plugins/rest/auth_tls.go:106-194` |
| Service timeouts | default 10 s response-header timeout; configurable per service | `v1/plugins/rest/rest.go:29`, `v1/plugins/rest/rest.go:199-202`, docs table `docs/docs/configuration.md:114` |
| http.send timeout & retries | default 5 s timeout, `HTTP_SEND_TIMEOUT` env override (panics on invalid), per-request `timeout`, retry backoff bounded 100 ms–60 s | `v1/topdown/http.go:36-37`, `v1/topdown/http.go:54-62`, `v1/topdown/http.go:317-330`, `v1/topdown/http.go:708-734` |
| Request body size limits | `DecodingLimitsHandler` rejects Content-Length > max, wraps body in `http.MaxBytesReader`, passes gzip limit via context | `v1/server/handlers/decoding.go:20-52`; wired at `v1/server/server.go:829` |
| Decoding limit defaults | `_default_max_length := 268435456 # 256 MB`, `_default_gzip_max_length := 536870912 # 512 MB`, validated by embedded Rego policy | `v1/plugins/server/decoding/validate.rego:15-17`, loader `v1/plugins/server/decoding/config.go:27-34` |
| Bundle gzip-bomb protection | `DefaultSizeLimitBytes = 1GB // limit bundle reads to 1GB to protect against gzip bombs`; downloader honors it | `v1/bundle/bundle.go:53`, `v1/download/download.go:114-116`, `v1/plugins/bundle/config.go:51,207-208` |
| Inter-query cache bounds | value-cache configs expose `max_num_entries` / `disabled` per named cache | `v1/topdown/cache/cache.go:60-90` |
| No process spawning by policies/runtime | repo-wide grep for `exec.Command` matches only build tooling and WASM compile step | `build/bench-comment/main.go:128,145,161`, `internal/compiler/wasm/optimizations.go:52` |
| Root-user warning | `checkUserPrivileges` logs a warning if uid/gid == 0 (Linux); no refusal | `v1/runtime/check_user_linux.go:15-22`, invoked `v1/runtime/runtime.go:684` |
| Unix domain socket listener | `--addr unix://<path>` supported with configurable socket permission | `v1/runtime/runtime.go:283-284`, `v1/runtime/runtime.go:739-741`; http.send can also target unix sockets `v1/topdown/http.go:368-400` |
| Server binding defaults | v1 CLI defaults to `localhost:8181`; v0-compatible binds `:8181` with a public-interface warning logged | `cmd/run.go:29-31`, `cmd/run.go:226`, `cmd/run.go:389-391`, `v1/runtime/runtime.go:667-669` |
| Authn/authz model | schemes `Off/Token/TLS` and `Off/Basic`; token-auth-without-authz flagged as ineffective; authorizer evaluates `system.authz` decision per request | `v1/server/server.go:67-79`, `v1/runtime/runtime.go:680-682`, `v1/server/authorizer/authorizer.go:93-153` |
| Filesystem reads are plain IO | loader reads files/dirs with `os.ReadFile` / `os.Open`; no VFS abstraction or path allowlist | `v1/loader/loader.go:229,351,484,541` |
| Disk store directory creation | `OptionsFromConfig`: `os.Stat` dir, `MkdirAll(dir, 0700)` when `auto_create`; badger options from superflag | `v1/storage/disk/config.go:44-56,68-82` |
| Bundle persistence to disk | `SaveBundleToDisk`: `MkdirAll(path, os.ModePerm)` then temp file `.bundle.tar.gz.*.tmp`; enabled per-bundle via `persist` | `internal/bundle/utils.go:121-141`, flag `v1/plugins/bundle/config.go:153` |
| REPL history file | default `.opa_history`, written by REPL history mechanism | `cmd/run.go:31`, `cmd/run.go:225`, `v1/repl/repl.go:1389-1392` |
| File logger with rotation | `file_logger` plugin uses lumberjack with `max_size_mb`, `max_age_days`, `max_backups`, `compress` | `v1/plugins/logger/file/plugin.go:16-25` |
| Version-check phone-home is opt-in | `EnableVersionCheck` default false; loop pings every ~1 h (later 6 h ± jitter) | `v1/runtime/runtime.go:78-83,241-243,774-777,909-953` |
| Graceful shutdown chain | wait period → server.Shutdown(timeout) → trace exporter → meter provider → storage.Closer → done | `v1/runtime/runtime.go:1032-1071`; signals SIGINT/SIGTERM buffered `v1/runtime/runtime.go:814-815` |
| Plugin stop with deadline | `Manager.Stop` applies `gracefulShutdownPeriod` timeout, stops all plugins, closes store if it implements `Close(ctx)` | `v1/plugins/plugins.go:920-965`; storage.Closer contract noted at `v1/runtime/runtime.go:101-104` |
| Watcher cleanup | fsnotify watcher closed on ctx.Done during watch loop | `v1/runtime/runtime.go:1002-1004`, watcher creation `internal/pathwatcher/utils.go:23-41` |
| Disk store Close | stops GC ticker and closes badger DB | `v1/storage/disk/disk.go:210-213` |
| No rlimits/memory caps found | greps for `SetMemoryLimit`, `SetMaxThreads`, `RLIMIT`, `ulimit`, `chroot`, `unshare`, `Setuid`, `Seccomp` return nothing in runtime code (only comments about "as if chrooted" bundle roots) | search boundary noted in Questions/Gaps |

## Answers to Dimension Questions

### 1. What filesystem access does an agent have?

Two distinct subjects:

- **Policies (Rego)**: none. There is no filesystem builtin in the evaluated language; the complete builtin inventory ships in `capabilities.json` (206 builtins, verified — all are pure computation, crypto, encoding, time, HTTP/DNS egress). Compilation restricts available builtins to those in the configured capability set (`v1/ast/compile.go:1014-1027` builds the compiler's builtin map from `capabilities.Builtins`). So policy authors cannot read secrets, write files, or traverse the workspace.
- **The OPA process**: whatever its OS user permits. It reads exactly the paths supplied via CLI flags (`params.Paths`, `v1/runtime/runtime.go:176-186`; plain IO at `v1/loader/loader.go:229,351`), writes REPL history to `.opa_history` (`cmd/run.go:31`, `v1/repl/repl.go:1389-1392`), optionally persists downloaded bundles as tarballs (`internal/bundle/utils.go:121-141`), optionally creates a badger-backed disk store directory with `0700` permissions (`v1/storage/disk/config.go:44-51`), and can route debug logs to rotating files (`v1/plugins/logger/file/plugin.go:16-25`). There is no mount namespace, chroot, or per-path ACL layer — containment relies entirely on deployment context (container user, volume mounts).

### 2. What network access does an agent have?

- **Policy-initiated egress**: `http.send` (`v1/topdown/http.go:171-199`) supports arbitrary methods/URLs/TLS options including client certs loaded from files or env vars (`v1/topdown/http.go:556-616`) and even unix-socket targets (`v1/topdown/http.go:368-400`). Host restriction comes solely from `Capabilities.AllowNet`, enforced synchronously at request construction and again on each redirect hop (`v1/topdown/http.go:402-423,474-477,646-650`). Same gate covers `net.lookup_ip_addr` (`v1/topdown/net.go:27`) and remote `$ref` schema fetching during compilation/type-check (`v1/ast/compile.go:995-998`). **By default (no `allow_net` key) any host is reachable** — the shipped `capabilities.json` omits the key. So yes: *an unconfigured OPA instance running a malicious policy can download arbitrary content from the internet* via `http.send`.
- **Runtime-initiated egress**: only to services declared in config (`services[_].url`) used by bundle, discovery, decision-log, and status plugins, plus the opt-in version check (`v1/runtime/runtime.go:241-243`). These connections get hardened-by-default TLS (minimum version floor, explicit CA pinning, opt-in `allow_insecure_tls`, `response_header_timeout_seconds` default 10 s) — see `v1/plugins/rest/auth_tls.go:26-67` and `v1/plugins/rest/rest.go:29`. Notably, `allow_net` does **not** constrain these operator-configured endpoints.

### 3. Can the agent spawn arbitrary processes?

No. A repository-wide search for `exec.Command` finds only CI/build helpers (`build/bench-comment/main.go:128,145,161`) and the optional WASM optimizer invoked while *building* a wasm bundle (`internal/compiler/wasm/optimizations.go:52`). Neither the Rego builtin surface nor the server/REPL exposes any exec, system, or process-control function. The sole privilege-related behavior is a non-fatal warning when OPA runs as uid/gid 0 (`v1/runtime/check_user_linux.go:19-21`).

### 4. Are resources cleaned up after execution?

Yes, via an explicit ordered teardown rather than ad-hoc finalizers:

1. SIGINT/SIGTERM received into a buffered channel (`v1/runtime/runtime.go:814-815`);
2. optional `ShutdownWaitPeriod` delay (`v1/runtime/runtime.go:1033-1036`);
3. HTTP server graceful shutdown bounded by `GracefulShutdownPeriod` (default 1 s, `v1/runtime/runtime.go:348,1039-1046`);
4. OpenTelemetry trace exporter and meter provider shutdown (`v1/runtime/runtime.go:1048-1059`);
5. store closed through the `storage.Closer` interface — implemented by the disk store which stops its GC ticker and closes badger (`v1/runtime/runtime.go:1061-1068`, `v1/storage/disk/disk.go:210-213`), and additionally closed inside `Manager.Stop` (`v1/plugins/plugins.go:948-952`);
6. all plugins stopped under a context with the graceful-shutdown deadline (`v1/plugins/plugins.go:926-965`);
7. file watchers closed when their context is cancelled (`v1/runtime/runtime.go:1002-1004`).

Within a query, cancellation propagates into in-flight `http.send` requests and returns a `Halt` error (`v1/topdown/http.go:305-315`).

## Architectural Decisions

1. **Capability file as the single policy-egress control point.** `AllowNet` lives on `ast.Capabilities` (`v1/ast/capabilities.go:84-101`), so one artifact simultaneously constrains compile-time schema fetching, `http.send`, and DNS lookups, and travels with bundles/`opa build --capabilities`. The maintainers explicitly scope it today to hostname matching with a TODO for ports (`v1/ast/capabilities.go:99`).
2. **Deny-by-absence for dangerous builtins.** Rather than gating a filesystem/process API behind flags, OPA simply never defined such builtins; the compiler rejects unknown functions not present in the capability set (`v1/ast/compile.go:1014-1027`). This makes the fs/process story trivially auditable.
3. **Warnings over refusals for operational posture.** Root execution (`v1/runtime/check_user_linux.go:19-21`), public-interface binding (`v1/runtime/runtime.go:667-669`), and token-auth-without-authorization (`v1/runtime/runtime.go:680-682`) log errors/warnings but do not abort, prioritizing availability.
4. **Rego-validated configuration.** Server request-size limits are injected defaults plus validation rules authored in Rego itself, evaluated by an embedded config-policy engine (`v1/plugins/server/decoding/config.go:27-34`, `validate.rego:15-35`) — dogfooding that guarantees every limit has an enforced default.
5. **Layered, interface-driven shutdown.** Cleanup is split across `storage.Closer`, plugin `Stop(ctx)` with a manager-wide deadline, and runtime-owned exporters/watchers, so embedded consumers registering custom backends inherit the same contract (`v1/runtime/runtime.go:101-108`).

## Notable Patterns

- **Egress caching as a control surface**: `http.send` results go through intra-query and inter-query caches keyed on the full request object, with TTLs derived from `Cache-Control`/`Expires` or forced via `force_cache_duration_seconds` (`v1/topdown/http.go:260-291,850-996`). This bounds repeated egress without a separate rate limiter, and metrics counters distinguish cache hits from real network requests (`v1/topdown/http.go:40-42,1398-1399`).
- **Fail-closed vs fail-open duality encoded in data**: `allow_net` omitted = allow all, `[]` = deny all (`v1/ast/capabilities.go:96-98`) — a deliberate convention that tests pin down (`v1/topdown/jsonschema_test.go:545-550`).
- **Size limits enforced before parsing**: Content-Length rejection prior to body read plus decompressed-size ceilings threaded through request context (`v1/server/handlers/decoding.go:20-52`) — a two-stage defense against zip-bomb-style DoS mirrored on the bundle side by the 1 GB reader cap (`v1/bundle/bundle.go:53`).
- **Credential hygiene**: Authorization headers masked in debug logs (`v1/plugins/rest/rest.go:36-39,397-403`); private-key files hashed so re-reads only occur on content change (`v1/plugins/rest/auth_tls.go:133-141`).
- **Unix sockets both directions**: the server can listen on `unix://` with configurable permissions (`v1/runtime/runtime.go:283-284`), and `http.send` can dial unix sockets — enabling fully local, network-free deployments (`v1/topdown/http.go:368-400`).

## Tradeoffs

- **Simplicity vs precision in egress control**: `AllowNet` compares hostnames/IPs only — no ports, schemes, CIDRs, or per-builtin scoping (`v1/ast/capabilities.go:94-100`). An allowlist entry grants that host to every gated builtin.
- **Safe-by-absence vs extensibility risk**: because custom builtins can be registered by embedders, the "no filesystem/process builtins" property holds for stock OPA but is not structurally guaranteed for forks/embedders.
- **Operator-configured egress bypasses `allow_net`**: bundle/log/status endpoints are trusted wholesale once present in config; a compromised discovery document can redirect future fetches within the configured trust model (discovery is explicitly designed to fetch config remotely).
- **Warnings instead of enforcement** keep OPA runnable everywhere (containers commonly start as root) but mean the shipped defaults are not a hardened posture; security.md's own "Hardened Configuration Example" requires operator assembly (`docs/docs/security.md:607+`).
- **Per-subsystem resource limits** (request bytes, bundle bytes, timeouts, cache entries) avoid global knobs like memory ceilings — predictable in Kubernetes where cgroups exist, but unprotected in bare embed scenarios.

## Failure Modes / Edge Cases

- **Chunked requests evade the Content-Length precheck** by design; they fall back to context-threaded max-length enforcement, acknowledged in a comment at `v1/server/handlers/decoding.go:23-24`.
- **Redirect handling differs by flag**: redirects are not followed unless `enable_redirect` is true; when enabled, each hop is re-checked against `AllowNet`, covered by `TestHTTPRedirectAllowNet` (`v1/topdown/http_test.go:793-851`) — but DNS rebinding between check and dial remains possible since verification happens before `net.Dial` resolution.
- **Invalid `HTTP_SEND_TIMEOUT` panics at init** rather than degrading (`v1/topdown/http.go:317-330`); the comment notes the variable is not public API.
- **Bundle persist writes with `os.ModePerm` (0777)** (`internal/bundle/utils.go:129`), unlike the disk store's restrictive `0700` (`v1/storage/disk/config.go:46`) — inconsistent on multi-tenant hosts; persisted tarballs contain whatever the remote served, mitigated only when bundle signature verification is enabled (`v1/runtime/runtime.go:245-249`).
- **`raise_error=false` turns network failures into data**, letting policies branch on `eval_http_send_network_error` (`v1/topdown/http.go:201-220`) — flexible, but means egress misconfiguration can silently produce undefined decisions instead of loud failures.
- **Root/gid-0 detection failure is swallowed** (only debug-logged, `v1/runtime/check_user_linux.go:17-18`), and Windows checks differ (`v1/runtime/check_user_windows.go`), so the warning is best-effort.

## Future Considerations

- Extend `allow_net` to ports/CIDRs/per-builtin scopes, as flagged in the upstream TODO (`v1/ast/capabilities.go:99`).
- Apply `AllowNet` (or an equivalent) to operator-configured `services[_]` URLs so a single egress policy governs all outbound traffic.
- Promote root-run detection from warning to configurable refusal, mirroring how `ReadyTimeout` gates serving.
- Add optional process-level governance (Go `SetMemoryLimit`, GOMAXPROCS pinning, or documented cgroup guidance) for embedders outside orchestrators.
- Unify persisted-artifact permissions (bundle persist dir `os.ModePerm` → `0700` like the disk store).

## Questions / Gaps

- **No evidence found** of any seccomp/AppArmor/namespace/chroot integration or privilege-dropping code in the runtime; searches for `chroot|unshare|Setuid|Seccomp|rlimit|SetMemoryLimit|GOMAXPROCS` across Go sources matched only build tooling, a WASM thread-pool sizing constant (`internal/wasm/sdk/opa/opa.go:54`), and comments about bundle roots being "as if chrooted" (`v1/bundle/file.go:200`). Containment is delegated to the deployment environment (Helm/Kubernetes docs under `docs/docs/deploy/`, `docs/docs/security.md`).
- Whether decision-log/status plugins impose outbound payload size caps was not traced exhaustively; the download side is bounded (`v1/download/download.go:114-116`) but upload-side limits were out of the searched boundary for this study.
- The exact threat model statement for `http.send` (e.g., SSRF guidance) lives in prose docs tied loosely to implementation; `docs/docs/security.md` sections cover TLS/interface-binding/authz but were not found to discuss `allow_net` directly — the operative documentation is `docs/docs/operations.md:138-153`.

---

Generated by dimension 17.02 (Filesystem, Network, and Process Controls) against `opa`.
