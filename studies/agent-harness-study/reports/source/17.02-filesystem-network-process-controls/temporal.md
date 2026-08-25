# Source Analysis: temporal

## Dimension 17.02: Filesystem, Network, and Process Controls

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal workflow server; gRPC + fx DI + SQL/Cassandra/Elasticsearch persistence) |
| Analyzed | 2026-08-24 |

> Citation convention: all file paths below are workspace-relative and rooted in the selected source directory (`studies/agent-harness-study/sources/temporal/...`). Line numbers refer to the current checkout.

## Summary

Temporal is a workflow orchestration *server*, not an agent harness that executes tenant code, so its "filesystem / network / process control" story is about protecting the server process itself and constraining server-side egress. The model is:

- **Filesystem**: the server touches disk only through a small set of operator-configured surfaces — YAML config loading with env-var templating (`common/config/loader.go:29-53`), TLS cert/key/CA files or inline base64 equivalents (`common/auth/tls.go:4-27`, `common/auth/tls_config_helper.go:140-225`), a polled dynamic-config file (`common/dynamicconfig/file_based_client.go:19-52`), the optional filestore archiver writing JSON histories under an operator-supplied URI (`common/archiver/filestore/history_archiver.go:105-184`), and one subprocess to resolve database passwords (`common/config/persistence.go:301-322`). No API request can steer the server to an arbitrary path.
- **Network**: defense is layered — configurable bind addresses (`common/rpc/rpc.go:200-237`), optional mTLS on frontend/internode listeners (`common/rpc/rpc.go:131-185`), a JWT claim-mapper + authorizer interceptor chain (`service/frontend/fx.go:286-315`, `common/authorization/interceptor.go:57-96`), multi-scope rate limiting (host RPS, per-namespace RPS across three API classes, concurrent long-poll limits: `service/frontend/fx.go:484-516`, `536-647`, `649-663`), payload caps (`common/rpc/grpc.go:30-44`), an HTTP-API Host allowlist (`service/frontend/http_api_server.go:274-285`), and — most notably — a default-deny per-namespace egress allowlist for Nexus callback URLs (`chasm/lib/callback/config.go:71-139`).
- **Shell/process**: exactly one production `exec.Command` site (DB password command, argv-array form, timeout + pipe-wait cap); no shell interpolation anywhere (`common/config/persistence.go:294-322`). There are no cgroup/rlimit/GOMEMLIMIT-style resource ceilings; process protection is quota-based (rate limiters, concurrency tokens, keepalive eviction, deadlock detection with optional abort).
- **Cleanup**: shutdown is choreographed via fx lifecycle hooks and a staged graceful-stop sequence (fail health → drain → `GracefulStop` with deadline → reverse-order service stop: `service/frontend/service.go:544-591`, `temporal/server_impl.go:47-53`, `109-146`); data cleanup is retention-driven via durable `DeleteHistoryEventTask` timers and a five-stage workflow deletion pipeline.

Answering the dimension's headline question — *"Can an agent download arbitrary files from the internet?"* — **No**. Tenant-triggered outbound fetches do not exist; every server-initiated network call has an operator-controlled or allowlisted destination (databases, cluster peers, JWKS URIs at `common/authorization/default_token_key_provider.go:170-187`, archiver providers, registered Nexus endpoints, and a fixed version-check host gated by `frontend.enableServerVersionCheck`, `common/dynamicconfig/constants.go:919-923`, `common/versioninfo/caller.go:14-21`).

## Rating

**Score: 7/10**

Rationale against the rubric:

- **Clear model with explicit interfaces (7–8 band):** every control is a named, typed configuration surface — `auth.TLS` struct (`common/auth/tls.go:4-27`), `callback.AddressMatchRules` (`chasm/lib/callback/config.go:85-114`), dynamic-config settings like `frontend.httpAllowedHosts` (`common/dynamicconfig/constants.go:684-691`) — rather than ad-hoc checks.
- **Operational safeguards:** layered rate limiting, payload caps, keepalive enforcement, deadlock detector with dump/fail-health/abort knobs (`common/deadlock/deadlock.go:21-80`), graceful drain deadlines (`service/frontend/service.go:552-584`).
- **Tested:** unit tests exist for the TLS helpers (`common/auth/tls_config_helper_test.go`), callback validator/rules (`chasm/lib/callback/config_test.go`, `validator_test.go`), rate-limit interceptors (`common/rpc/interceptor/rate_limit_test.go`, `namespace_rate_limit_test.go`, `concurrent_request_limit_test.go`), and HTTP host allowlist behavior (`tests/http_api_test.go:290`).
- **Why not higher:** (a) there are no memory/CPU/fork process limits (no `GOMEMLIMIT`, rlimits, or cgroup integration anywhere — verified by search for `Setrlimit|RLIMIT|debug.SetMemoryLimit`, which only matched test code); (b) filesystem controls rely entirely on operator config correctness — the filestore archiver's path validation only checks existence/type (`common/archiver/filestore/util.go:182-197`), not containment; (c) egress controls cover callbacks but Nexus operation targets lean on admin-gated endpoint registration plus scheme validation (`service/frontend/nexus_endpoint_client.go:360-361`) rather than an address allowlist; (d) pprof serves over unauthenticated plain HTTP when enabled (`common/pprof/pprof.go:41-65`).

## Evidence Collected

Every entry cites a workspace-relative path into `studies/agent-harness-study/sources/temporal`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Config file loading | Server reads hierarchical YAML from `TEMPORAL_CONFIG_DIR` (default `config`), templated with sprig env vars; embedded dev template via `go:embed`; `TEMPORAL_SERVER_CONFIG_FILE_PATH` single-file mode | `common/config/loader.go:20-53`, `common/config/loader.go:101-117` |
| DB password subprocess | Only production `exec.Command`: `ResolvePassword()` runs operator-configured argv array; 30s default timeout; `WaitDelay=5s` prevents hung stdout pipes; mutually exclusive with static password | `common/config/persistence.go:284-297`, `common/config/persistence.go:301-322` |
| TLS artifacts | `certFile/keyFile/caFile` read from disk via `os.ReadFile` OR base64 `certData/keyData/caData`; mutual-exclusion validated; min TLS 1.2 | `common/auth/tls.go:4-27`, `common/auth/tls_config_helper.go:82-138`, `common/auth/tls_config_helper.go:140-169`, `common/auth/tls_config_helper.go:20-27` |
| Listener bind controls | Per-service `grpcPort/membershipPort/httpPort`; `bindOnLocalHost` / `bindOnIP` mutually exclusive, fatal on parse failure | `common/config/config.go:75-89`, `common/rpc/rpc.go:213-237` |
| Frontend interceptor chain | Ordered chain: mask errors → service errors → business ID → namespace validation → metrics → **auth** → handover → redirection → telemetry → health → concurrent limits → namespace RPS → host RPS → SDK/caller/slow-log → retry innermost | `service/frontend/fx.go:286-315` |
| Host-level RPS limit | `ClusterAwareQuotaCalculator` splits global quota across members; health checks exempt; metric records effective quota | `service/frontend/fx.go:484-516` |
| Namespace rate limits | Three limiter categories: execution, visibility, replication-inducing APIs; per-instance + global namespace quotas with burst ratios | `service/frontend/fx.go:536-647` |
| Concurrent request cap | `ConcurrentRequestLimitInterceptor` counts in-flight long-polls per namespace/method, returns `RESOURCE_EXHAUSTED(CONCURRENT_LIMIT)` | `common/rpc/interceptor/concurrent_request_limit.go:19-67` |
| Payload size caps | `MaxHTTPAPIRequestBytes=4MiB`, `MaxNexusAPIRequestBodyBytes=2MiB`, internode recv 128MiB; HTTP body enforced with `http.MaxBytesReader` | `common/rpc/grpc.go:30-44`, `service/frontend/http_api_server.go:229-234`, `service/frontend/nexus_operation_http_handler.go:303-310` |
| gRPC servers w/ TLS | Frontend uses frontend TLS config; internal-frontend/internode use internode TLS creds; keepalive params attachable | `common/rpc/rpc.go:131-185`, `service/frontend/fx.go:275-282` |
| Outbound remote-cluster auth | Remote frontend dials attach token credentials; if `requireRemoteClusterAuth` and no token available → `Unauthenticated`; boot-time validation noted | `common/rpc/rpc.go:260-302` |
| JWT authZ | Interceptor maps claims (JWT or mTLS peer cert) and calls pluggable `Authorizer`; header names configurable | `common/authorization/interceptor.go:57-96`, `common/authorization/default_jwt_claim_mapper.go`, wired at `service/frontend/fx.go:191-216` |
| JWKS key fetch | `openURI` accepts `http(s)://` URIs and `file://` (host must be empty/localhost); keys swapped atomically under lock | `common/authorization/default_token_key_provider.go:138-187`, `common/authorization/default_token_key_provider.go:130-135` |
| Version check callout | POSTs build info to fixed `https://version-info.temporal.io/check`; controlled by `frontend.enableServerVersionCheck` (default true unless `TEMPORAL_VERSION_CHECK_DISABLED`) | `common/versioninfo/caller.go:14-21`, `common/versioninfo/caller.go:90-96`, `common/dynamicconfig/constants.go:919-923` |
| HTTP API host allowlist | `allowedHostsMiddleware` returns 403 `{"code":7,"message":"Host not allowed"}` unless `Host` matches regex list; default allows anything | `service/frontend/http_api_server.go:274-285`, `common/dynamicconfig/constants.go:684-691` |
| Callback egress allowlist | `callback.allowedAddresses` per-namespace rules; **default empty = all external URLs rejected**; system/internal URLs always allowed; https required unless rule sets `AllowInsecure`; invalid pattern entries silently skipped | `chasm/lib/callback/config.go:71-114`, `chasm/lib/callback/config.go:126-139`, `chasm/lib/callback/config.go:141-168` |
| Callback validation point | Callbacks validated at attach time (workflow/activity start): count ≤2000, URL length, endpoint rules, header size | `chasm/lib/callback/validator.go:40-84`, wired at `service/frontend/fx.go:904-913` |
| Callback routing | System callbacks pinned to local/remote frontends by decoded signed token; external targets go through a dedicated `externalClient`; worker-source callbacks routed to cluster frontends | `chasm/lib/callback/request.go:106-162`, `chasm/lib/callback/fx.go:35-62` |
| Nexus endpoint scheme gate | Endpoint registration validates target URL scheme is http/https | `service/frontend/nexus_endpoint_client.go:360-361` |
| Persistence QPS limits | Priority-based persistence rate limiters incl. health-aware adaptive limiter that scales with datastore latency error ratio | `common/persistence/client/quotas.go:201-262`, `common/persistence/client/health_request_rate_limiter.go:29-76` |
| Keepalive controls | Dynamic-config keepalive server params/enforcement policy for frontend; internode client/server keepalive flags | `service/frontend/fx.go:262-272`, `service/frontend/service.go:138-152`, `common/rpc/rpc.go:164-172`, `common/rpc/rpc.go:412-424` |
| Deadlock detection | Periodic ping of registered roots; dynamic knobs: dump goroutines, fail health check, abort process, interval, worker pool size | `common/deadlock/deadlock.go:21-80` |
| Runtime observability | RuntimeMetricsReporter samples goroutines/GOMAXPROCS/memory stats every minute; throttled logger bounded by RPS fn | `common/resource/fx.go:313-322`, `common/resource/fx.go:133-141`, `common/metrics/runtime.go:69` |
| pprof endpoint | Disabled when port unset (0); binds `localhost` by default; served over plain HTTP without auth | `common/pprof/pprof.go:41-65` |
| Filestore archiver | Writes JSON-encoded history to `<uri-path>/<hash(ns,wid,rid)>_<version>.history`; dir/file modes from config; URI scheme must be `file`; path validated for existence/type only | `common/archiver/filestore/history_archiver.go:36-45`, `common/archiver/filestore/history_archiver.go:171-183`, `common/archiver/filestore/history_archiver.go:276-282`, `common/archiver/filestore/util.go:182-197` |
| Dynamic config polling | FileBasedClient re-reads config file on modtime change, poll interval ≥5s | `common/dynamicconfig/file_based_client.go:19-52`, `common/dynamicconfig/file_based_client.go:124-130` |
| Graceful shutdown | Sequence: fail health → membership draining → wait detection window → stop handlers → `GracefulStop` with `AfterFunc` deadline → hard stop; HTTP server drains then closes | `service/frontend/service.go:544-591` |
| Service start/stop ordering | Deterministic init order (matching→history→internal-FE/frontend→worker) reversed on stop | `temporal/server_impl.go:47-53`, `temporal/server_impl.go:109-146` |
| Connection cache hygiene | Internode conn map swept of shut-down conns every 30min off the request path; factory `Close()` closes all cached conns via fx StopHook | `common/rpc/rpc.go:39-40`, `common/rpc/rpc.go:353-371`, `common/rpc/rpc.go:392-410`, `common/resource/fx.go:471` |
| Retention-based data cleanup | On close, `DeleteHistoryEventTask` scheduled at `closeTime + retention + full-jitter`; deletion runs as staged pipeline (visibility→replication→current→mutable state→history) resumable across restarts | `service/history/workflow/task_generator.go:341-367`, `service/history/tasks/workflow_cleanup_timer.go:15-25`, `service/history/shard/context_impl.go:918-1100` |
| Namespace deletion controls | DeleteNamespace activities rate-limited, paginated, concurrency-capped; optional delay before final namespace removal | `service/frontend/service.go:154-171` |

## Answers to Dimension Questions

### 1. What filesystem access does an agent have?

The server process (the "agent" here) has **no general-purpose filesystem access surface**; it reads/writes five narrow, operator-configured locations:

1. **Static config**: YAML hierarchy from `TEMPORAL_CONFIG_DIR` with env-var templating, or a single file via `TEMPORAL_SERVER_CONFIG_FILE_PATH` (`common/config/loader.go:29-45`, `common/config/loader.go:101-117`).
2. **TLS material**: cert/key/CA files named in config (or inline base64, which avoids disk entirely) (`common/auth/tls.go:8-26`, `common/auth/tls_config_helper.go:143-145`, `192-214`).
3. **Dynamic config**: a single polled file whose re-read is modtime-gated (`common/dynamicconfig/file_based_client.go:24-51`).
4. **Archival output**: the filestore archiver creates directories and writes history files strictly under the operator-configured archive URI path (`common/archiver/filestore/history_archiver.go:171-183`); the URI must use the `file` scheme (`history_archiver.go:276-282`). Tenant API requests cannot choose this path; it comes from namespace archival config set by operators.
5. **One subprocess** whose binary/args come from static config (see Q3).

There is **no mount management, chroot/jail, or path-containment layer** — the code trusts operator config. `validateDirPath` only checks emptiness/existence/is-dir (`common/archiver/filestore/util.go:182-197`). Searches for sandbox-like constructs (`mount`, `chroot`, path allowlists) found nothing in production code.

### 2. What network access does an agent have?

**Inbound**: one gRPC listener per service plus an optional HTTP API port and a membership port; bind address restricted by `bindOnLocalHost`/`bindOnIP` (`common/config/config.go:75-89`, `common/rpc/rpc.go:200-237`). The HTTP API additionally enforces a Host-header allowlist (`service/frontend/http_api_server.go:274-285`) and rejects streams (`http_api_server.go:466-473`).

**Outbound**, each with its own control:
- Cluster-internal gRPC to peer services (membership-resolved), internode TLS optional, recv payload capped at 128MiB (`common/rpc/grpc.go:43`, `common/rpc/rpc.go:317-351`).
- Remote-cluster frontends during cross-cluster operations — TLS per hostname plus mandatory bearer token when `remoteClusterAuth.require=true` (`common/rpc/rpc.go:260-302`).
- Persistence stores and visibility stores per operator config (SQL/Cassandra TLS struct shared: `common/auth/tls.go:4-27`).
- Archiver providers (S3/GCS/file) chosen by URI scheme from operator config (`common/archiver/provider`, `common/archiver/filestore/history_archiver.go:38`).
- **Caller-influenced egress** is the sensitive class, and it is constrained:
  - Completion callbacks: default-deny per-namespace allowlist `callback.allowedAddresses`; https-by-default; only exact system/internal URLs bypass rules (`chasm/lib/callback/config.go:71-114`).
  - Nexus endpoints: targets must be admin-registered via OperatorService; registration validates http/https scheme (`service/frontend/nexus_endpoint_client.go:360-361`).
  - JWKS URIs for JWT verification: operator-configured; support http/https and localhost-only `file://` (`common/authorization/default_token_key_provider.go:170-187`).
  - Telemetry/version callout to fixed host `version-info.temporal.io`, disable-able via setting/env (`common/dynamicconfig/constants.go:919-923`).

So: **an agent (tenant) cannot make the server fetch arbitrary internet resources**; the only caller-supplied URLs are callbacks (allowlisted) and pre-registered Nexus endpoints (admin-gated).

### 3. Can the agent spawn arbitrary processes?

**No.** A repo-wide search for `os/exec` found 11 importing files, all tests/tooling except one: `ResolvePassword` in `common/config/persistence.go:301-322`. It executes a statically configured command+args (argv array, never a shell string), validates that `passwordCommand` and `password` are mutually exclusive (`persistence.go:284-292`), bounds execution with a context timeout (default 30s, `persistence.go:294-297`), and caps post-kill pipe blocking with `WaitDelay=5s` (`persistence.go:311-314`). It is flagged `//nolint:gosec` deliberately (`persistence.go:311`). No API handler, task executor, or queue processor spawns processes. Workflow code runs in external workers, entirely outside this process boundary.

### 4. Are resources cleaned up after execution?

Yes, at three layers:

- **Process/network**: fx lifecycle StopHooks close RPC factories, client beans, and SDK client factories deterministically (`common/resource/fx.go:274-290`, `400-409`, `471`); the frontend stops via fail-health → drain-window → `GracefulStop` with a `time.AfterFunc` deadline forcing hard stop (`service/frontend/service.go:552-584`); stale internode connections are swept every 30 minutes (`common/rpc/rpc.go:39-40`, `353-371`). Services stop in reverse init order (`temporal/server_impl.go:112-118`).
- **Data retention**: closing a workflow schedules a durable `DeleteHistoryEventTask` at `closeTime + namespaceRetention + jitter` (`service/history/workflow/task_generator.go:341-367`); deletion proceeds through a five-stage, crash-resumable pipeline covering visibility, replication, current-row, mutable state, then history branches (`service/history/shard/context_impl.go:918-1100`).
- **Namespace teardown**: delete-namespace background activities are themselves rate-limited, paginated, and concurrency-capped, with an optional `DeleteNamespaceNamespaceDeleteDelay` before final metadata removal (`service/frontend/service.go:154-171`).

What is *not* cleaned up automatically: the filestore archiver never deletes archived files (write/read only — `common/archiver/filestore/history_archiver.go` contains no delete path), so disk usage of the archive directory is an operator concern.

## Architectural Decisions

1. **Interceptors as the single enforcement plane.** All inbound controls (authZ, namespace validity, three rate-limit families, error masking, slow-request logging) are composed as an explicitly ordered gRPC chain rather than scattered handler checks (`service/frontend/fx.go:286-315`). The comment block documents why order matters ("Mask error interceptor should be the most outer..."). This makes policy auditable in one place.
2. **Default-deny for caller-supplied egress URLs.** `AddressMatchRules{}` zero value means "no rules" → any external callback URL fails validation (`chasm/lib/callback/config.go:74`, `104-113`); operators must opt namespaces in. This inverts the usual allow-by-default risk for SSRF-style abuse, while `temporal://system` completion callbacks remain always allowed (`config.go:91-93`).
3. **Operator trust boundary instead of OS isolation.** Temporal assumes it runs in a trusted deployment environment; it never sandboxes itself (no seccomp/rlimits/cgroups). Controls are config-validation + quotas. E.g., TLS material can be injected as base64 config values specifically so secrets don't need tmpfs files (`common/auth/tls.go:22-26`).
4. **Quotas instead of hard resource ceilings.** Rather than memory/CPU caps, overload is shed via ResourceExhausted at multiple scopes (host RPS, namespace RPS ×3 categories, concurrent long-poll tokens, persistence QPS with latency-adaptive scaling: `common/persistence/client/health_request_rate_limiter.go:100-138`).
5. **Crash-safe, staged cleanup.** Workflow deletion is modeled as durable tasks with a bit-mask progress stage persisted in mutable state so partial deletions resume correctly after restarts (`service/history/shard/context_impl.go:921`, `service/history/tasks/delete_workflow_execution_stage.go:3-15`).
6. **Subprocess use treated as exceptional.** The lone exec site carries explanatory comments, a timeout, a WaitDelay, and lint suppression — signaling deliberate, reviewed use (`common/config/persistence.go:311-314`).

## Notable Patterns

- **Dynamic config everywhere**: nearly every knob in this dimension (keepalive timings `service/frontend/service.go:138-152`, host allowlist `constants.go:684-691`, callback rules `chasm/lib/callback/config.go:71`, deadlock abort `common/deadlock/deadlock.go:72-76`, RPS quotas `service/frontend/service.go:60-80`) is hot-reloadable through the polled dynamic-config client — ops can tighten policy without redeploy.
- **Cluster-aware quotas**: per-host limits are divided among live members via `MemberCounter` so adding nodes raises fleet capacity transparently (`service/frontend/fx.go:490-497`, `calculator.ClusterAwareNamespaceQuotaCalculator`).
- **Defense-in-depth boot checks**: remote-cluster dialing refuses to send unauthenticated traffic even if boot validation was skipped (e.g., in tests) (`common/rpc/rpc.go:279-299`).
- **Config converters that fail soft but loud**: malformed `callback.allowedAddresses` patterns are skipped during conversion (`chasm/lib/callback/config.go:152-160`) while validation remains deny-by-default — availability is preserved without widening egress.
- **Listener reuse discipline**: the HTTP API derives its TCP listener from the gRPC listener's resolved address and wraps it in TLS only when frontend TLS exists, guaranteeing both ports share bind policy (`service/frontend/http_api_server.go:82-109`).

## Tradeoffs

- **Operator trust vs. tenant safety**: relying on config correctness (rather than OS-level containment) keeps the server portable across Kubernetes/bare-metal/Docker, but a mistyped archive URI or dynamic-config value silently changes the security envelope (e.g., `validateDirPath` accepts any existing directory: `common/archiver/filestore/util.go:182-197`).
- **Allow-by-default inbound, deny-by-default outbound**: `frontend.httpAllowedHosts` defaults to match-anything (`constants.go:687-689`) while callback addresses default to reject-all (`config.go:74`). Sensible per direction, but inconsistent enough to surprise operators auditing one side only.
- **Soft-fail config parsing**: skipping invalid callback patterns avoids outage-on-typo but can leave a namespace unintentionally locked down (deny-all) until someone notices logs.
- **Quota-based protection needs tuning**: with no hard memory ceiling, a pathological payload within the 128MiB internode cap (`common/rpc/grpc.go:43`) still allocates real memory before any limiter engages.
- **pprof usability vs. exposure**: binding localhost by default (`common/pprof/pprof.go:47-52`) is convenient but the endpoint is unauthenticated plain HTTP once enabled; misconfigured `host: 0.0.0.0` would expose goroutine dumps.

## Failure Modes / Edge Cases

- **Hung credential subprocess**: mitigated by ctx timeout + `WaitDelay` so an orphaned child holding stdout cannot block startup forever (`common/config/persistence.go:305-314`).
- **Callback URL validation happens at attach time only** (`chasm/lib/callback/validator.go:42`); if operators later *narrow* `callback.allowedAddresses`, already-persisted callbacks still fire — the runtime router does not re-check rules (`chasm/lib/callback/request.go:106-123`).
- **External callback client has no proxy/egress pinning**: `externalClient` uses a plain instrumented transport (`chasm/lib/callback/fx.go:39-45`); containment rests wholly on the attach-time allowlist.
- **Partial deletion crashes**: covered by stage bit-mask persistence — stages re-run idempotently until all bits set (`service/history/shard/context_impl.go:973-1100`, tested in `context_test.go:192-280` including error-and-continue paths).
- **Drain timeout races**: `GracefulStop` plus `AfterFunc(requestDrainTime)` guarantees termination even if streams refuse to finish (`service/frontend/service.go:571-578`); HTTP server mirrors with `Shutdown(ctx)` then unconditional `Close()` (`http_api_server.go:217-227`).
- **Deadlock runaway**: detector can escalate to failing health checks or aborting the process, converting silent stalls into visible restarts (`common/deadlock/deadlock.go:33-39`).
- **Remote-cluster race on first dial**: concurrent dials may both create connections; loser closes its own to avoid leaks (`common/rpc/rpc.go:252-257`).

## Future Considerations

- Add opt-in hard resource ceilings (Go `debug.SetMemoryLimit`, container-aware CPU hints) to complement quota shedding — searches found none today.
- Re-validate egress rules at dispatch time (not just attach time) so narrowing `callback.allowedAddresses` affects in-flight callbacks.
- Extend the address-allowlist pattern used for callbacks to Nexus operation endpoint targets for uniform egress policy.
- Document/flag the filestore archiver's lack of retention pruning so operators provision disk accordingly.
- Consider authenticating or TLS-optionalizing the pprof listener when bound non-loopback.

## Questions / Gaps

- **No evidence found** for OS-level filesystem mounts, sandboxing, ulimits, cgroups, or fork-bomb protections anywhere in production code (searched: `exec.Command`, `syscall`, `RLIMIT`, `GOMAXPROCS`, `SetMemoryLimit`, `mount`, `chroot`). If such limits exist, they are imposed by the deployment environment (e.g., Docker/Kubernetes), not this repository. Docker configs were outside the analyzed scope of runtime code paths inspected.
- **No evidence found** of IP/CIDR-based ingress filtering (allowlists/denylists by source address); inbound network policy beyond bind address and Host-header matching is delegated to infrastructure.
- Whether Nexus *operation* dispatch (as opposed to callbacks) honors `callback.allowedAddresses` could not be confirmed from the frontend code inspected; the observed gates are admin-only endpoint registration plus scheme validation (`service/frontend/nexus_endpoint_client.go:360-361`). `docs/architecture/nexus.md:86-88` mentions related keys, suggesting intent, but I did not trace the full outbound dispatch path in `components/nexusoperations`.
- Default values of several TLS-related settings (e.g., whether inter-node TLS is required in sample prod templates vs merely supported) were not exhaustively audited across `config/` templates.

---

Generated by `Dimension 17.02: Filesystem, Network, and Process Controls` against `temporal`.
