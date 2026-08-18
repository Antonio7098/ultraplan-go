# Local Web Dashboard

UltraPlan can serve a guarded browser view of the current workspace and its
existing operations from the same Go binary:

```bash
ultraplan serve
ultraplan --workspace /path/to/workspace serve
ultraplan serve --listen 127.0.0.1:9090 --open-browser
ultraplan serve --listen '[::1]:8080'
```

The default URL is `http://127.0.0.1:8080/`. UltraPlan prints the canonical
bound URL after the listener starts. `--open-browser` is optional; if the
platform launcher is missing or fails, UltraPlan writes a safe warning to
stderr and continues serving.

## Workspace And Configuration

Workspace discovery and configuration validation are the same as for CLI and
TUI entry points:

1. global `--workspace <path>`
2. `ULTRAPLAN_WORKSPACE`
3. current directory and its ancestors

The selected directory must contain `ultraplan.yml`. UltraPlan validates the
workspace and effective configuration before opening a listener. Server
settings are immutable for the life of the process; restart to change them.

`--listen` accepts only a numeric loopback IP literal and explicit port.
Accepted forms include `127.0.0.1:8080` and `[::1]:8080`. `localhost`,
wildcard addresses, LAN/public addresses, missing ports, port zero, and IPv6
zone identifiers are rejected. UltraPlan does not silently choose a different
port when the configured port is occupied.

## What The Dashboard Shows

The browser pages and bundled `/api/v1` resources inspect:

- workspace, project, sprint, and study summaries
- current planning flow, execute, review, smoke, and study run state
- existing validation findings
- governed Markdown and JSON artifacts through bounded source previews

Project, sprint, and study pages also expose allowlisted operations backed by
the same typed app capability as the TUI: validation, prompt/dry-run and sprint
flow, execute, review, smoke, verify, study run-loop, status, and explicit
cancellation. The browser cannot submit arbitrary commands, paths, prompts, or
executables.

Each request reads current app-owned workspace state. State-bearing HTML, JSON,
errors, operation projections, and SSE use `Cache-Control: no-store`. Embedded
static assets use `public, max-age=0, must-revalidate` because their stable URLs
are not content-addressed. There is no server snapshot, browser database,
watcher, hidden preload, automatic polling, or browser-owned product state.
Use the visible Refresh link or an ordinary page reload after CLI/TUI changes.
A response is a bounded point-in-time projection, not a cross-request
transactional snapshot.

Collections and finding lists return at most 200 entries and report returned,
total, and truncated counts in JSON. The current API does not expose pagination,
search, or caller-configurable bounds.

## Artifact Previews

Detail results issue opaque references for allowlisted governed Markdown and
JSON artifacts. The HTTP routes never accept an absolute or workspace-relative
file path and `internal/web` has no arbitrary filesystem reader.

UltraPlan resolves each reference inside the configured workspace, rejects
stale or forged references, traversal, symlink escapes, unsupported artifact
classes, and non-regular files, then reads at most 256 KiB plus one byte for
truncation detection. Invalid, stale, unsupported, and escaping references all
produce the same safe not-found result.

Markdown is converted by the app boundary through Goldmark's safe GFM renderer;
raw HTML and unsafe link destinations are disabled before the web adapter marks
the reviewed projection as template content. JSON and fallback sources remain
escaped inside labelled code blocks. Workspace HTML, scripts, and JSON strings
are never executed. JSON responses report total bytes, returned bytes,
truncation, and bounded JSON validity.

## Local Security And Trust Boundary

The server is for one local user and is not a hosted or remote service. It
reduces local exposure through:

- numeric loopback-only binding
- an exact canonical Host authority
- exact same-origin validation, a per-process session cookie, and independent
  CSRF proof for mutation requests
- no permissive CORS response
- 8 KiB request-target, 64 KiB declared-body, and 128-byte identifier limits
- rejection of undocumented request bodies and query parameters, with JSON
  command bodies limited to 64 KiB
- 32 in-flight request bound and fixed HTTP timeouts
- restrictive CSP, frame denial, `nosniff`, same-origin referrers, and no-store headers
- server-generated request IDs, safe error projection, and redacted diagnostics

An absent Origin is allowed for top-level navigation and local `GET`/`HEAD`
clients after Host validation. Operation `POST`/`DELETE` requires the exact
server Origin, a valid same-process session cookie, and the session's CSRF
header. Operation confirmation is separate: it binds the normalized request
and current governed-input fingerprint for two minutes and is consumed once.
`Origin: null`, malformed, cross-origin, and non-loopback origins are rejected.
IPv4 and IPv6 server origins are distinct.

Loopback is not an authentication or isolation boundary against another process
running as the same OS user or a compromised local account. The local cookie is
session policy, not an account, tenant, TLS, or remote authentication model.
There is no remote-worker or LAN/public exposure. Do not proxy or port-forward
this server.

## API And Health

The bundled browser uses versioned `/api/v1` JSON resources. Success
responses use `{data, meta}`; errors use `{error, meta}` with safe stable codes.
Unknown `/api/` paths and unsupported versions always return JSON rather than an
HTML page.

This API is compatibility-controlled for the bundled browser. It is not yet a
promised public integration API and has no remote-client or pagination support.
Breaking DTO changes require an explicit version or coordinated migration.

### Guarded operation API

The command lifecycle is:

1. `POST /api/v1/operations/prepare` performs side-effect-free normalization,
   prerequisite inspection, affected-path projection, and fingerprinting.
2. The user reviews the returned scope and short-lived confirmation.
3. `POST /api/v1/operations` repeats the specification, re-normalizes it, and
   starts server-owned work only if the confirmation remains current. Success
   is `202` with a `Location` header. Retrying the same accepted session/token
   returns the original operation and Location without starting duplicate work.
4. `GET /api/v1/operations/{id}` returns retained status and terminal result.
5. `GET /api/v1/operations/{id}/events` observes progress through SSE.
6. `DELETE /api/v1/operations/{id}` requests canonical cancellation and is
   idempotent.

Stable states are `accepted`, `running`, `cancelling`, `succeeded`, `failed`,
`cancelled`, `interrupted`, and `cleanup_uncertain`. Stable SSE event names are
`snapshot`, `progress`, `warning`, `finding`, `artifact`, `cancel_requested`,
`recovery_required`, and `terminal`. Event IDs are decimal and monotonic within
one operation. SSE is progress-only: disconnecting closes only that
subscription, while cancellation requires `DELETE`.

Accepted product failures remain terminal operation results returned from the
status resource with HTTP `200`. Pre-acceptance and transport errors use stable
codes including `invalid_request`, `csrf_failed`, `origin_rejected`,
`session_required`, `operation_not_found`, `confirmation_expired`,
`confirmation_mismatch`, `confirmation_replayed`, `stale_confirmation`,
`operation_conflict`, `validation_failed`, `prerequisite_unavailable`,
`operation_capacity`, `subscriber_capacity`, `server_draining`, and
`internal_failure`.

The hub is deliberately ephemeral. Durable workspace and product run state are
the recovery authority after eviction or restart. A replay gap emits
`recovery_required` plus a current snapshot; it never fabricates missing
progress. Before accepting traffic, server startup reconciles dead-owner sprint
execute/review/smoke attempts to explicit interrupted evidence while leaving
live cross-process mutation leases untouched.

### Operation bounds

| Resource | Bound |
| --- | --- |
| JSON command body | 64 KiB |
| Active operations | 8 |
| Preparations | 128 for 2 minutes |
| Retained events | 256 and 256 KiB per operation |
| Encoded event | 16 KiB |
| Terminal result | 256 KiB |
| Subscribers | 8 per operation, 32 server-wide |
| Subscriber queue | 32 events |
| Terminal retention | 10 minutes |
| Heartbeat / stream lifetime | 15 seconds / 30 minutes |

Slow subscriber queues are disconnected without blocking product work. New
work is never queued: capacity returns `429`, and draining returns `503`.

`GET /api/v1/health` reports only server readiness and lightweight availability
of the configured workspace query. `200`/`ok` means the server can answer that
query; `503`/`unavailable` means it cannot. Health does not validate every
artifact, scan projects or studies, contact a runtime/provider, or run review or
smoke. It is not proof that the whole product state is valid.

## Shutdown And Troubleshooting

Press Ctrl-C or send the process its normal termination signal. UltraPlan first
enters draining, rejects preparation/start, requests `server_shutdown`
cancellation exactly once for every active server-owned operation, waits up to
10 seconds for canonical product/runtime/harness cleanup and durable terminal
reconciliation, publishes terminal SSE, then stops HTTP. Deadline expiry is
reported as interrupted or cleanup-uncertain rather than success. Before the
ephemeral hub closes, the app atomically writes a product-owned
`.cleanup-uncertain.json` marker in the affected sprint. Startup reconciles the
marker under the normal sprint mutation lease; if no canonical running record
can be reconciled, the marker remains available for inspection. Server startup
fails closed until that uncertainty is resolved, rather than accepting
new sprint mutations over ambiguous state. Browser disconnect never triggers
this sequence.

Common failures:

- `workspace not found`: run inside a workspace or pass global
  `--workspace <path>`.
- `config.load`: correct `ultraplan.yml`; the server does not bypass normal
  config validation.
- `serve.listen`: use a numeric loopback literal with a port, not `localhost`.
- `address already in use`: stop the other process or choose another explicit
  loopback port with `--listen`.
- `request rejected`: open the exact URL printed by UltraPlan; do not replace
  its IP literal or port with an alias.
- browser launcher warning: copy the printed URL into a browser; the server
  itself remains healthy.
- artifact not found after a refresh: the opaque reference is stale or the
  governed artifact changed; reload its project/sprint/study detail page.

Templates use validated `primitive/*`, `component/*`, `layout/*`, and `page/*`
definitions. CSS is layered as tokens, base, primitives, components, layouts,
and utilities. Dependency-free JavaScript separates baseline lifecycle,
operation commands, and SSE ownership. There is no Node.js, Vite, frontend
build, separate asset server, database, or web-owned durable job store.
Runtime/provider and smoke-harness prerequisites apply only when their existing
operations are selected.

Snapshot-based invalidation of completed reviews, completed smoke evidence, and
smoke-author changes to the product target or governed project inputs is
temporarily disabled. Small or concurrent filesystem edits were making valid
results unnecessarily stale or failing authoring runs. The implementation is
retained behind explicit policy switches and can be reintroduced after change
attribution and relevance rules are designed reliably. Canonical artifact
existence, format, and recorded digest checks remain active, as does the smoke
harness authoring allowlist; harness changes outside that allowlist are still
hard failures.
