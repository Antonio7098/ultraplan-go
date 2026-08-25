# Source Analysis: opa

## Dimension 11.04 — Context Provenance and Integrity

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go; policy-as-code engine (Rego), HTTP data/decision APIs, bundle distribution, plugin manager |
| Analyzed | 2026-08-25 |

## Summary

OPA's "context" is primarily the set of activated **bundles** (policies + data) plus runtime data and cached `http.send` responses. Provenance is modeled explicitly, but only at the bundle granularity, not per context item:

- Every activated bundle carries a manifest with an opaque `revision`, optional free-form `metadata`, and ownership `roots` (`v1/bundle/bundle.go:147-165`). On activation this manifest (plus the download `etag`) is persisted into the store under `/system/bundles/<name>/manifest` and `/system/bundles/<name>/etag` (`v1/bundle/store.go:44-51`), so provenance survives serialization, restarts, and is queryable by policies as normal data.
- The Data API can attach a provenance block (build version + per-bundle revisions) to any decision response via `?provenance=true`; it is assembled from store-persisted revisions (`v1/server/server.go:2766-2787`, `getRevisions` at `v1/server/server.go:1058-1084`) and documented in `docs/docs/rest-api.md:2408-2458`.
- Decision log events (`EventV1`) carry rich per-decision provenance: labels, decision IDs, per-bundle revisions, requester, timestamp, and explicit transformation records (`erased` / `masked` path lists) (`v1/plugins/logs/plugin.go:49-76`).
- Freshness is tracked via HTTP ETags end-to-end (downloader → plugin → store) and via status timestamps (`last_successful_activation/download/request`, `v1/plugins/bundle/status.go:25-39`). The inter-query cache for `http.send` stores per-item expiry derived from `Cache-Control`/`Expires` (`v1/topdown/http.go:1109-1123`).
- Trust is enforced through optional JWS bundle signing: file hashes are signed, verified against scoped keys, and unsigned-but-signed-bundles fail closed when verification config is present (`v1/bundle/verify.go:70-116,199-207`, `v1/bundle/bundle.go:902-908`). Trust, however, is binary and opt-in; there is no graded trust annotation on individual context items.
- Transformations of logged decisions are recorded in-band (masking appends to `event.Erased`/`event.Masked`, `v1/plugins/logs/mask.go:178,197`), but some transformations are only observable out-of-band (drop counters, ND-cache stripping during encoding).

Overall: a clear, tested, operationally hardened provenance model for bundles and decision logs, with notable gaps at the individual-data-document level (runtime writes carry zero provenance) and in freshness enforcement of signatures.

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Explicit, serialized, test-covered provenance for the dominant context source (bundles): revision/metadata/roots in manifests, etags persisted in-store, provenance surfaced via API and decision logs (`v1/server/server_test.go:3221,3341`, `v1/sdk/opa_test.go:706`).
- Real integrity machinery: signed bundle verification with key IDs, scopes, pluggable verifiers, and per-file hash checks (`v1/bundle/verify.go:212-262`).
- Kept from 8+: no per-item provenance on Data API writes (`v1/server/server.go:1958-2012`); signature `iat` parsed but never validated (`v1/bundle/bundle.go:122` vs. `v1/bundle/verify.go`); drop/ND-cache transformations not recorded in-band (`v1/plugins/logs/plugin.go:772-775`, `v1/plugins/logs/encoder.go:105-131`); `ProvenanceBundleV1` exposes only `revision` (`v1/server/types/types.go:236-238`), discarding richer manifest metadata at the API boundary.

## Evidence Collected

All paths are workspace-relative to `studies/agent-harness-study/sources/opa`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source annotations (bundle identity) | `Bundle` struct with `Signatures`, `Manifest`, `Etag`, `Raw`; `Manifest` carries `revision`, `roots`, `metadata` | `v1/bundle/bundle.go:63-78`, `v1/bundle/bundle.go:147-165` |
| Provenance schema durability | Manifest mirrored as JSON Schema + protobuf ("never reuse a proto field number") | `v1/bundle/manifest.schema.json:1-15`, `v1/bundle/manifest.proto:16-37`, comment at `v1/bundle/bundle.go:145-146` |
| Provenance survives activation | Manifest + etag written to `/system/bundles/<name>/manifest` / `/etag` during activation; erased on deactivate | `v1/bundle/store.go:44-51`, `v1/bundle/store.go:130-137`, `v1/bundle/store.go:548-560`, `v1/bundle/store.go:693-708` |
| Metadata persistence/readback | `ReadBundleMetadataFromStore` reads free-form manifest metadata back from store | `v1/bundle/store.go:306-325` |
| Runtime provenance API | `?provenance=true` param; response field `DataResponseV1.Provenance` | `v1/server/server.go:1533,1652-1653,1911-1912`, `v1/server/types/types.go:266-268`, `v1/server/types/types.go:582-584` |
| Provenance assembly | `getProvenance` builds version/build info + per-bundle revisions from store; legacy single-revision compat | `v1/server/server.go:2766-2792` |
| Revisions read from store | `getRevisions` walks named bundles and reads each stored revision | `v1/server/server.go:1058-1084` |
| SDK parity | SDK `DecisionResult.Provenance` populated for decisions and partial eval | `v1/sdk/opa.go:384-388`, `v1/sdk/opa.go:327`, `v1/sdk/opa.go:460-467` |
| Decision-log provenance fields | `EventV1`: labels, decision_id, bundles map w/ revision, requested_by, timestamp, req_id, custom | `v1/plugins/logs/plugin.go:49-76`, `v1/plugins/logs/plugin.go:716-743` |
| Server→logger handoff | `server.Info.Bundles` + deprecated `Revision`; decisionLogger populates from stored revisions | `v1/server/buffer.go:18-48`, `v1/server/server.go:3102-3149`, `v1/server/server.go:2560-2574` |
| Freshness (ETag round-trip) | Downloader sends `If-None-Match`, parses `ETag` responses; 304 short-circuit in plugin | `v1/download/download.go:290,346,401-408`, `v1/plugins/bundle/plugin.go:578-582` |
| ETag persistence & recovery | Etag written to store on activation; re-read on startup/failure to resume caching | `v1/bundle/store.go:134-137,327-346`, `v1/plugins/bundle/plugin.go:243-247,350-355,517-519` |
| Freshness timestamps | Bundle `Status`: `active_revision`, `last_successful_activation/download/request`, error code/message | `v1/plugins/bundle/status.go:25-39`, setters at `v1/plugins/bundle/status.go:43-52` |
| Cached-response freshness | Inter-query cache items carry `expiresAt`; `http.send` derives expiry from Cache-Control/Expires or forced TTL; stale cleanup routine | `v1/topdown/cache/cache.go:302-306,325-336,268-300`, `v1/topdown/http.go:880,917-942,1109-1123` |
| Trust: signature verification | JWT verified with configured key (`kid` header/payload fallback), then scope equality check; extensible `Verifier` registry | `v1/bundle/verify.go:118-209`, `v1/bundle/verify.go:60-86,264-286` |
| Trust: key scoping config | `keys.Config{Key, Algorithm, Scope}`; scope mismatch fails verification | `v1/keys/keys.go:27-32`, `v1/bundle/verify.go:199-207` |
| Trust: integrity of contents | Per-file hash verification against signed payload (structured files hashed canonically) | `v1/bundle/verify.go:212-262` |
| Trust: fail-closed when configured | Signed bundle without verification key → error "verification key not provided"; plugin wires `Signing` config into readers | `v1/bundle/bundle.go:902-908`, `v1/plugins/bundle/plugin.go:459-471,879` |
| Transformation records (masking) | Mask ops append to `event.Erased` (remove) / `event.Masked` (upsert); mask policy itself is Rego-evaluated | `v1/plugins/logs/mask.go:128-203`, driver at `v1/plugins/logs/plugin.go:1048-1100` |
| Transformations serialize | `erased`/`masked` emitted in console/slog attrs and JSON event payloads | `v1/plugins/logs/plugin.go:1220-1221`, JSON tags at `v1/plugins/logs/plugin.go:64-65` |
| Serialization of events | Events JSON-marshaled, gzipped into size-adaptive chunks; decoder provided | `v1/plugins/logs/encoder.go:103-135`, decoder `v1/plugins/logs/encoder.go:469-487` |
| Delta bundle consistency | Delta patches rejected if roots/wasm resolvers differ from stored manifest | `v1/bundle/store.go:611-631` |
| Disk persistence across restarts | Raw bundle tar.gz persisted atomically (tmp+rename) under persist path | `v1/plugins/bundle/plugin.go:731-757` |
| Tests: provenance API | `TestDataProvenanceSingleBundle`, `TestDataProvenanceSingleFileBundle`, `TestDataProvenanceMultiBundle` | `v1/server/server_test.go:3221,3292,3341` |
| Tests: SDK provenance | `TestDecisionWithProvenance`, `TestPartialWithProvenance` | `v1/sdk/opa_test.go:706,1318` |
| Tests: masking traceability | mask rule-set tests assert `Erased`/`Masked` bookkeeping incl. failure paths | `v1/plugins/logs/mask_test.go` (rule set suite), `v1/plugins/logs/eventBuffer_test.go` |

## Answers to Dimension Questions

### 1. Does each context item know where it came from?

**Partially — strong for bundles, absent for runtime data writes.**
Bundles are self-describing: manifest `revision`/`metadata`/`roots` (`v1/bundle/bundle.go:147-165`) travel inside the signed archive and land in the store (`v1/bundle/store.go:44-75`). Decisions can be attributed to exact bundle revisions via `?provenance=true` (`v1/server/server.go:1652-1653,2766-2787`) and every decision-log event names its bundle revisions (`v1/plugins/logs/plugin.go:56,717-720`). Signature payloads also record issuer/keyid/scope (`v1/bundle/bundle.go:117-124`), though only transiently for verification. However, documents written through the Data API (`PUT/PATCH /v1/data/...`) are stored as bare values with no source, author, or origin annotation whatsoever (`v1/server/server.go:1958-2012`): after a write, that slice of the context tree has no memory of where it came from. Individual files inside a bundle likewise have no retained per-file source beyond the signed hash list used at load time.

### 2. Is freshness tracked?

**Yes, operationally — but not semantically.**
Freshness is tracked mechanically: ETags flow downloader→plugin→store and gate conditional requests (`v1/download/download.go:290`, `v1/plugins/bundle/plugin.go:571,578-582`), wall-clock recency lives in `Status.LastSuccessfulActivation/Download/Request` (`v1/plugins/bundle/status.go:28-33`), and `http.send` cache entries expire per-response TTL (`v1/topdown/http.go:917-942`). But the `revision` string itself has no defined time semantics, and — notably — the bundle-signature `iat` claim is parsed into `DecodedSignature.IssuedAt` (`v1/bundle/bundle.go:122`) yet never checked anywhere (no reference outside its definition; searched `IssuedAt|.Iat|"iat"` repo-wide): an old but validly signed bundle stays fresh forever until replaced by upstream polling.

### 3. Is trust level indicated?

**Binary and bundle-scoped, not graded per item.**
A signed bundle is either fully trusted (JWT verifies, scope matches configured key scope, all file hashes match: `v1/bundle/verify.go:195-207,212-262`) or rejected. Key configs bind a `scope` to each key (`v1/keys/keys.go:27-32`), and the verifier registry allows operators to plug stronger verification (`v1/bundle/verify.go:60-86,273-286`). When a bundle ships `.signatures.json` and OPA lacks verification config it fails closed (`v1/bundle/bundle.go:902-904`). There is no notion of partial trust, no per-document authority annotation, and crucially verification is **opt-in**: an unsigned bundle activates silently unless `signing` is configured (`v1/plugins/bundle/plugin.go:459-471`), so unverified context is indistinguishable from verified context downstream once activated.

### 4. Are transformations traceable?

**In-band for masking, out-of-band for drops and encoding losses.**
Masking is exemplary: each remove/upsert applied by a mask rule is appended to `event.Erased`/`event.Masked` with the masked path (`v1/plugins/logs/mask.go:178,197`), survives JSON/slog serialization (`v1/plugins/logs/plugin.go:64-65,1220-1221`), and the mask rules themselves are version-controlled Rego evaluated per event (`v1/plugins/logs/plugin.go:1048-1100`). Gaps: (a) policy-driven **drops** erase the event entirely with no tombstone — only a debug line (`plugin.go:772-775`); (b) rate-limit/buffer drops are Prometheus counters only (`plugin.go:274-276`); (c) the encoder may strip `nd_builtin_cache` from an oversized event to fit upload limits — the change is visible neither in the emitted event nor its JSON, only in logs/metrics (`v1/plugins/logs/encoder.go:108-131,173-189`).

## Architectural Decisions

1. **Provenance lives in the store, not in memory.** Revisions/etags/metadata are written under reserved `/system/*` paths at activation (`v1/bundle/store.go:36-75`), making provenance transactional with the data it describes, durable across restarts, and readable by the very policies being evaluated.
2. **Opt-in integrity with pluggable crypto.** Signing/verification are registry extension points (`Signer`/`Verifier`, `v1/bundle/sign.go:108-130`, `v1/bundle/verify.go:264-286`) defaulting to local-key JWS; this keeps the core dependency-light while allowing enterprise KMS/HSM integrations.
3. **Compatibility shims over breaking changes.** Deprecated `Revision` fields coexist with `Bundles` maps in server types, decision events, and store paths (`v1/server/types/types.go:231`, `v1/plugins/logs/plugin.go:55`, legacy manifest paths at `v1/bundle/store.go:1295-1328`) — provenance schema evolution is handled gently but leaves dual representations that consumers must interpret.
4. **Transformation policy as Rego.** Both masking and dropping are expressed as ordinary policy queries against the event AST (`v1/plugins/logs/plugin.go:1052-1069,1108-1114`), reusing the same compiler/trust base as decisions rather than a parallel transformation language.
5. **Fail-closed only where context is *declared* signed.** Verification triggers on presence of `.signatures.json` (`v1/bundle/bundle.go:902-908`), not on configuration alone — integrity is a property of the artifact, detected rather than assumed.

## Notable Patterns

- **Round-trip provenance:** the same revision string that gates delta-bundle application (`v1/bundle/store.go:611-631`) is what surfaces in `?provenance` responses and decision logs — one fact, three consumers.
- **Status-as-freshness surface:** `plugins/bundle` exposes a structured status document (activation times, error codes, metrics timers, `v1/plugins/bundle/status.go:25-39`) consumable by `GET /v1/status`-style health tooling instead of log scraping.
- **Canonical hashing before signing:** structured JSON files are normalized (alphabetically ordered) prior to hashing so signatures are whitespace-independent (`v1/bundle/verify.go:230-243`).
- **Adaptive encoding with graceful degradation:** chunk compression scales limits up/down and prefers dropping the non-deterministic builtin cache over losing whole events (`v1/plugins/logs/encoder.go:137-189`).
- **Schema-mirrored evolution:** manifest changes require synchronized updates to `manifest.proto`, the generated JSON Schema, and tests ("never reuse a proto field number", `v1/bundle/bundle.go:145-146`), giving provenance metadata an explicit compatibility contract.

## Tradeoffs

- **Bundle-granular provenance keeps the engine simple** but means mixed-origin data (e.g., a Data API write landing inside a bundle root after activation) flattens attribution: everything under `/system/bundles/<name>` claims that bundle, while sibling writes claim nothing.
- **Opaque revision strings** decouple OPA from any VCS/timestamp scheme (flexible), at the cost that freshness must be inferred from transport signals (ETag/status timestamps) rather than content.
- **Opt-in verification** minimizes deployment friction but produces a silent-trust default; the alternative (require signing config) would break existing users, so OPA chose detection-over-assumption.
- **Out-of-band drop/size telemetry** keeps events small and pipelines cheap, but audit trails reconstructed solely from decision logs will silently miss dropped or trimmed records.
- **Dual legacy/modern fields** preserve upgrade paths but double the surface area of every provenance struct and force consumers to handle both shapes indefinitely.

## Failure Modes / Edge Cases

- **Signature age unchecked:** a leaked, long-valid signing key can keep serving ancient bundles; `iat` is decorative (`v1/bundle/bundle.go:122`, unused in `v1/bundle/verify.go`).
- **Post-activation mutation erases origin:** data patched via REST API overwrites bundle-owned subtrees with anonymous values (`v1/server/server.go:1988-2012` enforces *scope*, not *lineage*), so provenance reported later reflects the bundle revision, not the override.
- **Etag staleness windows:** on download/activation/persistence failure the plugin deliberately re-arms the downloader with the *old* etag (`v1/plugins/bundle/plugin.go:513-520,536-557`), correctly avoiding bad activations but meaning a server-side rollback to identical bytes is invisible.
- **Silent ND-cache loss:** large events upload without their `nd_builtin_cache` (`v1/plugins/logs/encoder.go:120,189`); downstream reproducibility analysis based on the log will misjudge nondeterministic builtin behavior.
- **Corrupt-metadata hard errors:** malformed stored manifests/etags abort operations with explicit "corrupt ..." errors rather than defaults (`v1/bundle/store.go:292-304,334-346`), which is safe but turns a single bad byte into an unavailable bundle.
- **Multiple JWTs unsupported:** exactly one signature per bundle (`v1/bundle/verify.go:101-103`) — multi-signature (cosigning) workflows need a custom Verifier.

## Future Considerations

- Extend `ProvenanceBundleV1` / `BundleInfoV1` beyond `revision` to include etag, metadata, and signature issuer so API/log consumers get the same fidelity available internally (`v1/server/types/types.go:236-238` vs. `v1/bundle/bundle.go:147-165`).
- Enforce or at least expose signature issuance time (`iat`/`exp`) with configurable clock skew, closing the stale-signature window.
- Attach lightweight write-provenance (source = `rest_api`, request id, principal) to Data API mutations, e.g., via `storage.Context` (`v1/server/server.go:1980-1981` already threads a context object) or a parallel `/system/provenance` tree.
- Record in-band markers for drop decisions and ND-cache stripping (e.g., a companion tombstone event or a `"trimmed": [...]` field) so audit completeness is checkable from the stream itself.
- Per-file provenance retention: keep the verified `FileInfo` list (name/hash/algorithm, `v1/bundle/bundle.go:127-131`) queryable post-activation instead of discarding after load.

## Questions / Gaps

- No evidence found of any trust/authority grading below the bundle level; searched for `trust`, `authority`, `confidence` annotations across `v1/storage`, `v1/topdown`, and plugin trees.
- No evidence found that decision-log uploads are payload-signed; `grep 'sign|hmac'` in `v1/plugins/logs/plugin.go` returns nothing — integrity relies entirely on transport auth/TLS.
- Whether the deprecated legacy single-bundle provenance paths (`/system/bundle/manifest`, `v1/bundle/store.go:1295-1328`) still receive active test coverage alongside named bundles was confirmed only indirectly (compat branches at `v1/server/server.go:2777-2784`); a dedicated deprecation-removal plan was not found in-repo.
- The `Metadata` manifest field is persisted and returned via `ReadBundleMetadataFromStore` (`v1/bundle/store.go:306-325`) but is not exposed through the `?provenance` response — intent (user-facing vs. internal bookkeeping) is undocumented.

---

Generated by dimension `11.04-context-provenance-and-integrity` against `opa`.
