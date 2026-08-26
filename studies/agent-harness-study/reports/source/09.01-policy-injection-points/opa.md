# Source Analysis: opa

## 09.01 Policy Injection Points

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (with Rego as the policy language; top-level `server/`, `plugins/`, etc. are thin v0 shims re-exporting `v1/` packages, e.g. `server/server.go:63` delegates to `v1/server`) |
| Analyzed | 2026-08-26 |

## Summary

OPA is a purpose-built policy engine, so "policy injection points" is its core architecture rather than a bolt-on governance layer. Policies (Rego modules + JSON/YAML data packaged as *bundles*) enter the system through five distinct injection points: (1) the REST API (`PUT /v1/policies/{path}`), (2) the bundle plugin polling an HTTP/OCI/file service, (3) the discovery plugin whose downloaded bundle itself generates new OPA configuration via a Rego decision query, (4) startup file/bundle loading from disk with opt-in fsnotify hot-reload, and (5) the embedded SDK configured with bundle/discovery stanzas. All five converge on one pipeline: transactional store write → compile → trigger notification.

Precedence is handled by rejection, not arbitration: bundles must declare disjoint manifest roots, and activation fails atomically on overlap while the previous revision stays live. Within a single store key, writes are last-write-wins with no optimistic concurrency. Versioning is coarse — one free-form `revision` string per bundle — but it is propagated consistently to decision logs, status payloads, Prometheus gauges, and query provenance responses. Integrity of injected policies can be enforced with JWT-based bundle signature verification; who may inject is governed by an optional OPA-policy-based authorization layer wrapping the server's own HTTP handlers. Notably, OPA even validates its own configuration using an embedded Rego policy (`internal/configpolicy`), making config injection policy-governed too.

## Rating

**9 / 10** — Clear, explicit model with tests and operational safeguards: transactional activation that retains the last good revision on failure (`v1/plugins/bundle/plugin_test.go:3994`, "previous revision is retained"), etag rollback after failed download/activation (`v1/plugins/bundle/plugin.go:513-520`), root-overlap conflict detection before any write (`v1/bundle/store.go:1139-1252`), and signature verification hooks at every remote injection point. Runtime updates without code changes are first-class (bundle polling, long-polling, discovery-driven reconfiguration). The point keeping it from 10: no storage-level audit history of policy changes (auditing depends entirely on external decision-log/status sinks), revisions are untyped strings with no schema, and REST same-ID PUTs have no concurrency/version guard.

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| REST policy injection | `PUT /v1/policies/{path}` route registration; handler `v1PoliciesPut` parses Rego, compiles against existing modules, then persists | `v1/server/server.go:915-918`; `v1/server/server.go:2217-2329` (parse :2263, compile :2288-2308, `store.UpsertPolicy` :2312) |
| Scope guards on REST writes | `checkPolicyIDScope`/`checkPolicyPackageScope` reject writes into paths owned by activated bundles ("path %v is owned by bundle %q") | `v1/server/server.go:2242,2283`; `v1/server/server.go:2487-2557` |
| Bundle plugin download→activate | `Plugin.process` receives update and calls `p.activate`; activation opens a write txn and calls `bundle.Activate` | `v1/plugins/bundle/plugin.go:504-588` (activate call :536); `v1/plugins/bundle/plugin.go:607-694` |
| Downloader source types | `newDownloader` dispatches to `file://` loader, OCI registry, or HTTP downloader | `v1/plugins/bundle/plugin.go:437-476` |
| Bundle config keys | `bundles.<name>.{service,resource,signing,persist,size_limit_bytes}` yaml keys | `v1/plugins/bundle/config.go:136-155` (defaults :171-208) |
| Polling config keys | `trigger`, `polling.min_delay_seconds`, `polling.max_delay_seconds`, `polling.long_polling_timeout_seconds`; defaults 60/120s | `v1/download/config.go:27-39` (defaults :16-17, validation :61-71) |
| Etag/long-poll protocol | `If-None-Match` header, `Prefer: modes=snapshot,delta;wait=...`; 304 short-circuits activation | `v1/download/download.go:290-309`; `v1/plugins/bundle/plugin.go:578-585` |
| Etag committed only after success | etag stored post-activation; reset to previous value on failure so next poll refetches | `v1/plugins/bundle/plugin.go:571`; rollback `v1/plugins/bundle/plugin.go:513-520` |
| Discovery = config injection | Discovery bundle evaluated in Rego (`evaluateBundle`) to produce OPA config; `manager.Reconfigure` applies it; bundle/status/log plugins rebuilt dynamically | `v1/plugins/discovery/discovery.go:556-594` (Eval :571-578); `v1/plugins/discovery/discovery.go:466-545` (Reconfigure :533) |
| Discovery pinning | "updates to the discovery service are not allowed" — discovery stanza and boot signing keys cannot be changed remotely | `v1/plugins/discovery/discovery.go:508`; boot-key guard `v1/plugins/discovery/discovery.go:512-526` |
| Discovery config keys | `discovery.{decision,service,resource,signing,persist,polling.*}` | `v1/plugins/discovery/config.go:24-37` |
| Startup file loading | CLI `--bundle` mode flag; positional paths loaded via `LoadPathsForRegoVersion`; `InsertAndCompile` upserts policies into store | `cmd/run.go:252,349`; `internal/runtime/init/init.go:149-212`; `internal/runtime/init/init.go:45-109` (`UpsertPolicy` :98) |
| File-watch hot reload (opt-in) | `--watch` flag; fsnotify watcher re-runs `InsertAndCompile` on change — same injection path as startup | `cmd/run.go:231`; `v1/runtime/runtime.go:974-1022` (watch start :768/:882) |
| SDK embedding | `sdk.New(ctx, Options)` builds manager from YAML config reader, registers discovery plugin for remote updates, live `Configure()` entry | `v1/sdk/opa.go:66-131` (configure :168-288); live reconfig `v1/sdk/opa.go:149-166`; options `v1/sdk/options.go:36-97` |
| Store location of bundle metadata | `/system/bundles/<name>/{manifest,revision,roots,etag,wasm,metadata}`; legacy `/system/bundle/manifest` | `v1/bundle/store.go:37-83` (legacy :1300-1303) |
| Policy storage model | Policies are a plain `map[string][]byte` keyed by ID in the inmem store; `UpsertPolicy` overwrites by ID, no history | `v1/storage/inmem/inmem.go:120`; `v1/storage/inmem/txn.go:398-400` |
| Module ID namespacing | Bundle module IDs prefixed `<bundle-name>/<module-path>` so two bundles can both ship `example.rego` without clobbering storage entries | `v1/bundle/bundle.go:1776-1785`; applied `v1/bundle/store.go:894-911` |
| Precedence: root disjointness | `hasRootsOverlap` rejects cross-bundle root overlap before activation ("root %s is in multiple bundles", "detected overlapping roots…"); within-bundle nesting allowed | `v1/bundle/store.go:1139-1252` (:1200, :1230, :1251); segment-safe overlap test `v1/bundle/bundle.go:1689-1694` |
| Precedence: rule-level merge | Same-package rules from different modules merge; hard conflicts fail compilation ("conflicting rules %v found") or path-conflict check | `v1/ast/compile.go:1431`; `v1/ast/conflicts.go:12-68` |
| Atomic activation & rollback | Activation in one write txn; failed activation retains previous revision; erase-old-then-write sequence | `v1/plugins/bundle/plugin.go:607-694` (txn :613); `v1/bundle/store.go:426-563` (erase :683+, compile :962-1002, write :881-940); retention test `v1/plugins/bundle/plugin_test.go:3994` |
| Signature verification | JWT-based `VerifyBundleSignature`; wired via `signing` config into bundle, file, and discovery downloaders | `v1/bundle/verify.go:70-210`; wiring `v1/plugins/bundle/plugin.go:463,470,879`; `v1/plugins/discovery/discovery.go:138,144` |
| Revision versioning | `Manifest.Revision string` plus `Metadata map[string]any` on every bundle; readable from store | `v1/bundle/bundle.go:147-165`; readers `v1/bundle/store.go:288-290,306-311` |
| Status reporting | Status payload exposes per-bundle `active_revision`, `last_successful_activation`, error codes/messages; Prometheus gauges export `last_successful_activation{bundle,revision}` | `v1/plugins/bundle/status.go:25-97` (success :43-46, errors :65-97); metrics `v1/plugins/status/plugin.go:576-591` |
| Decision-log auditing | Every decision event records `Bundles[name].revision` (plus deprecated flat `Revision`); revisions read from store per request via `getRevisions` | `v1/plugins/logs/plugin.go:49-81` (`Log()` :716); `v1/server/server.go:1058-1084`; attached `v1/server/server.go:2560-2574` |
| Provenance in responses | Query/provenance responses include per-bundle revisions; `?bundles` param returns activation info | `v1/server/server.go:2766-2786`; `v1/server/types/types.go:231-237,593-596` |
| Config validated by Rego | OPA parses its own config through an embedded Rego validation policy injecting defaults and warning on unknown keys | `v1/config/validate.go:29-46`; policy text `v1/config/validate.rego:1-30`; engine `internal/configpolicy/configpolicy.go:55-127` |
| API self-governance | Optional authorizer wraps all HTTP handlers with a Rego decision (default `/system/authz/allow`) | `v1/server/server.go:796-806`; `v1/server/authorizer/authorizer.go:93-148` |
| No storage-level history | `storage.Store` interface is current-state only (no diffs/snapshots); audit relies on external sinks | `v1/storage/interface.go:19-44,143-160` |

## Answers to Dimension Questions

### 1. Where do governance rules live?

Governance rules live exclusively as **Rego modules and data documents**, injected at five points:

- **REST push**: `PUT /v1/policies/{id}` accepts raw Rego text, parses it (`v1/server/server.go:2263`), compiles it together with all existing store modules, and persists via `UpsertPolicy` (`v1/server/server.go:2312`). The URL path is the module identity.
- **Bundle service pull**: the bundle plugin polls an HTTP service, OCI registry, or local `file://` path (`v1/plugins/bundle/plugin.go:437-476`) on a configurable schedule (`polling.min/max_delay_seconds`, long-polling timeout — `v1/download/config.go:27-39`).
- **Discovery-driven configuration**: a special "discovery bundle" contains Rego whose evaluation *produces the OPA config itself* (`evaluateBundle`, `v1/plugins/discovery/discovery.go:556-594`), which then instantiates/reconfigures bundle, decision-log, status, and custom plugins (`getPluginSet`, `v1/plugins/discovery/discovery.go:612-721`). This makes governance topology (which bundles exist, where logs go) itself remotely steerable.
- **Startup files/directories**: `--bundle` mode or positional paths loaded at boot (`cmd/run.go:252,349`; `internal/runtime/init/init.go:149-212`), with optional `--watch` hot-reload re-running the identical insertion+compile path (`v1/runtime/runtime.go:1009-1022`).
- **Embedded SDK**: library consumers pass an OPA YAML config reader; bundles arrive via the registered discovery plugin (`v1/sdk/opa.go:168-288`), and live reconfiguration is exposed via `Configure` (`v1/sdk/opa.go:149-166`).

All points converge on the same destination: the store's policy table keyed by module ID (`v1/storage/inmem/inmem.go:120`) plus bundle metadata under `/system/bundles/<name>/...` (`v1/bundle/store.go:37-83`). There is no external policy engine integration — the engine is the product; extension happens via plugins/hooks (e.g., `hooks.BundlePreActivateHook` invoked pre-compilation, `v1/plugins/bundle/plugin.go:641-647`).

### 2. Can policies be updated at runtime?

Yes — this is the primary operating model. Three mechanisms:

- **Bundle polling/long-polling**: the downloader sends `If-None-Match` etags and `Prefer: modes=snapshot,delta;wait=<n>` headers (`v1/download/download.go:290-309`). On 304 it skips activation entirely (`v1/plugins/bundle/plugin.go:578-585`); on 200 the new bundle activates inside a single write transaction (`v1/plugins/bundle/plugin.go:613`). Delta bundles apply JSON patches incrementally (`activateDeltaBundles`, `v1/bundle/store.go:611-631`).
- **Discovery reconfiguration**: beyond policies, the entire plugin set (including the bundle plugin's own target list) is hot-reloadable — `Plugin.Reconfigure` diffs configs and stops/starts/removes individual downloaders (`v1/plugins/bundle/plugin.go:148-199`). Two things are pinned: the discovery service endpoint itself ("updates to the discovery service are not allowed", `v1/plugins/discovery/discovery.go:508`) and boot-config signing keys.
- **REST PUT and opt-in file watch**: direct pushes via `PUT /v1/policies/{id}`, and `--watch`-mode fsnotify reloads (`v1/runtime/runtime.go:974-1022`). By default, file-based policies load once at boot only.

Failure safety: if activation fails, the previously active revision stays live and the etag is rolled back so the next poll retries the full body (`v1/plugins/bundle/plugin.go:513-520,536-543`; retention verified in `v1/plugins/bundle/plugin_test.go:3994`).

### 3. What happens when policies conflict?

OPA resolves conflicts by **rejection and namespace partitioning**, not runtime arbitration:

- **Cross-bundle data-root conflicts are fatal**: every bundle declares manifest roots; `hasRootsOverlap` canonicalizes and compares roots of all active plus incoming bundles and fails activation with "detected overlapping roots in manifests for these bundles" (`v1/bundle/store.go:1139-1252`). A bundle with nested overlapping roots in its own manifest fails earlier at parse ("manifest has overlapped roots", `v1/bundle/bundle.go:356`). Because activation is transactional, the old state remains serving.
- **Storage-ID collisions are impossible by construction**: bundle modules are stored under `<bundle-name>/<module-path>` IDs (`modulePathWithPrefix`, `v1/bundle/bundle.go:1776-1785`).
- **Same-ID REST PUT is last-write-wins**: uploading a module with an existing ID replaces it after successful compile (`modules[id] = parsedMod`, compile, then `UpsertPolicy` — `v1/server/server.go:2294-2312`); a compile error aborts the transaction and keeps the old policy. There is no optimistic-concurrency (if-match) guard.
- **REST writes into bundle-owned roots are blocked**: `checkPathScope` rejects any write whose path falls under an activated bundle's roots ("path %v is owned by bundle %q", `v1/server/server.go:2528-2557`) — bundles win precedence over ad-hoc API writes by ownership declaration.
- **Rule-level semantics**: rules with the same reference across modules merge into one virtual document; genuinely conflicting complete rules produce compile-time errors ("conflicting rules %v found", `v1/ast/compile.go:1431`) or rule-vs-data path conflicts (`v1/ast/conflicts.go:68`).

### 4. Are policy changes audited?

Partially — **observable but not internally archived**:

- Every bundle carries a `revision` string and optional metadata (`Manifest.Revision`, `Manifest.Metadata`, `v1/bundle/bundle.go:147-165`), persisted under `/system/bundles/<name>/revision` (`v1/bundle/store.go:62-63`).
- The status plugin reports per-bundle `active_revision`, `last_successful_activation`, and structured activation errors (`code: bundle_error`) to any configured status sink, and exports Prometheus gauges including `last_successful_activation{bundle, revision}` (`v1/plugins/bundle/status.go:25-97`; `v1/plugins/status/plugin.go:576-591`).
- Decision logs record exactly which bundle revisions produced each decision: the server reads revisions from the store per request (`getRevisions`, `v1/server/server.go:1058-1084`) and attaches them to events as `Bundles[name].revision` (`EventV1`, `v1/plugins/logs/plugin.go:49-81`). Query responses expose the same via provenance fields (`v1/server/server.go:2766-2786`).
- However, the store keeps **no history**: `storage.Store` is current-state-only (`v1/storage/interface.go:19-44`) and `UpsertPolicy` is overwrite-by-id (`v1/storage/inmem/txn.go:398-400`). Auditing *who changed what when* therefore requires the operator to configure decision-log/status sinks (or rely on the bundle service's own SCM); there is no built-in change log, diff, or actor attribution on the REST path.

## Architectural Decisions

1. **Single convergence point**: all injection mechanisms funnel into transactional store writes followed by compiler rebuild and trigger fan-out (`v1/plugins/bundle/plugin.go:607-694`; server registers triggers so reloads propagate, `v1/server/server.go:218,1086`). This gives uniform atomicity regardless of transport.
2. **Rejection-based precedence**: instead of layering/overriding policies, OPA requires statically disjoint bundle roots and refuses conflicting activations (`v1/bundle/store.go:1139-1252`). Simpler reasoning about "who owns this data", at the cost of operational rigidity (root planning is mandatory).
3. **Config-as-policy (meta-injection)**: the discovery bundle's Rego output *is* the configuration (`v1/plugins/discovery/discovery.go:556-594`), and OPA's static config is itself validated by an embedded Rego policy that injects defaults and flags unknown keys (`v1/config/validate.go:29-46`, `v1/config/validate.rego`). Governance machinery is applied reflexively to governance inputs.
4. **Coarse versioning, strong propagation**: one untyped `revision` string per bundle rather than per-module versions, but propagated everywhere decisions are observable (logs, status, prometheus, provenance).
5. **Integrity at the boundary**: signature verification is attached at download time for every remote source type (`v1/bundle/verify.go:70`; wiring `v1/plugins/bundle/plugin.go:463,470`), not at activation, and API access itself can be gated by a Rego decision (`v1/server/server.go:796-806`).

## Notable Patterns

- **Etag-as-watermark**: the downloader cache commits the etag only after successful activation and rolls back on failure (`v1/plugins/bundle/plugin.go:513-520,571`), turning HTTP caching into a retry-safety mechanism.
- **Ownership enforcement**: `checkPathScope` derives write permissions from currently activated bundle roots, so the REST API and bundles cannot race on the same subtree (`v1/server/server.go:2528-2557`).
- **Legacy compatibility lanes**: deprecated single-bundle config (`bundle:` key) maps onto the named-bundle machinery via `ActivateLegacy` and unprefixed `/system/bundle/manifest` paths (`v1/plugins/bundle/config.go:136-144`; `v1/bundle/store.go:1300-1303`).
- **Pre-activation hooks**: `BundlePreActivateHook` lets plugins inspect manifests and register external sources before compilation (`v1/plugins/bundle/plugin.go:641-647`) — an extension seam adjacent to the injection point.

## Tradeoffs

- **Disjointness vs flexibility**: root-rejection prevents silent clobbering but means teams must coordinate root allocation up front; there is no override/layering escape hatch for legitimate shared-data cases.
- **Last-write-wins REST**: simple and stateless, but two concurrent writers can silently drop each other's module since no version check exists on `PUT /v1/policies/{id}` (`v1/server/server.go:2217-2329`).
- **Externalized audit**: keeping the store history-free keeps the core small and fast, but "who pushed this policy" is unanswerable from OPA alone — it depends on deployment discipline around decision-log/status sinks and SCM.
- **Discovery power vs blast radius**: because discovery can rewrite the plugin set, a compromised or buggy discovery bundle reconfigures everything at once; mitigations are the pinned discovery stanza and mandatory signature verification config (`v1/plugins/discovery/discovery.go:505-526`).

## Failure Modes / Edge Cases

- **Activation failure keeps stale policy serving**: intentional (availability over freshness) — the old revision remains active and status reports the error (`v1/plugins/bundle/plugin_test.go:3994`).
- **Overlapping-root activation fails atomically**: mixed multi-bundle downloads where one bundle overlaps another abort as a set, avoiding partial states (`v1/bundle/store.go:457`).
- **Root containment is segment-aware**: `a/b` does not collide with `a/banana` (`RootPathsOverlap`, `v1/bundle/bundle.go:1689-1694`), avoiding false positives.
- **Empty/unset roots claim everything**: a bundle whose roots are unset or contain `""` causes all other writes to be rejected as bundle-owned (`v1/server/server.go:2538-2550`) — a footgun if a publisher omits roots.
- **Retry storm protection**: bounded activation retries (`maxActivationRetry = 10`, `v1/plugins/bundle/plugin.go:44,408`) and exponential backoff between polls (`v1/download/download.go:232-244`).

## Future Considerations

- Add optimistic concurrency (etag/if-match) to the REST policy PUT to make concurrent administrative edits safe.
- Record an internal, queryable policy-change journal (actor, timestamp, diff) or at minimum emit a dedicated audit event per activation, so auditing doesn't require external sink correlation.
- Type or structure the `revision` field (or promote `Manifest.Metadata` conventions) to enable tooling over policy versions.
- Surface bundle-root ownership more proactively (e.g., warnings when a bundle ships without explicit roots), given the everything-claiming behavior of empty roots.

## Questions / Gaps

- No evidence found for per-module or per-rule versioning; searched `ast.Annotations` (`v1/ast/annotations.go:28-38`) — annotations offer a free-form `custom` map but nothing version-specific is enforced or consumed by the engine.
- No evidence found for actor/authenticated-identity capture on policy mutations: the REST handler does not log the authenticated principal alongside `UpsertPolicy`; searched `v1PoliciesPut` (`v1/server/server.go:2217-2329`) and the inmem policy txn path (`v1/storage/inmem/txn.go:398-400`).
- Whether the `--watch` file-reload path is considered production-grade is undocumented in code; it shares the startup insertion path but has no debounce/coalescing visible in `readWatcher` (`v1/runtime/runtime.go:983-1007`).

---

Generated by `09.01-policy-injection-points` against `opa`.
