# Local Web Compatibility Baseline

This manifest records the reviewed Sprint 30-31 local-web contract hardened by
Sprint 32. `internal/web/api_compatibility_test.go` is the executable field,
type, nullability, method, status, content-type, and cache fixture. Any change
to that fixture requires a rationale here and coordinated browser and guide
updates; snapshot regeneration alone is not approval.

## Route And Method Inventory

State-bearing HTML routes accept `GET` and `HEAD`: `/`, `/projects`,
`/projects/{project}`, project documentation/sprint/operation/validation
views, `/projects/{project}/sprints/{sprint}` and its run/artifact/operation/
validation views, `/studies`, `/studies/{study}` and its input/progress/
operation/validation/artifact views, `/artifacts/{ref}`, and
`/operations/{id}`. Normal HTML preparation/start/cancel forms use `POST` at
`/operations/prepare`, `/operations/start`, and `/operations/{id}/cancel`.

The JSON/SSE matrix is fixed as follows:

| Route | Methods | Success |
| --- | --- | --- |
| `/api/v1/dashboard` | `GET`, `HEAD` | `200` JSON |
| `/api/v1/projects` | `GET`, `HEAD` | `200` JSON collection |
| `/api/v1/projects/{project}` | `GET`, `HEAD` | `200` JSON |
| `/api/v1/projects/{project}/sprints/{sprint}` | `GET`, `HEAD` | `200` JSON |
| `/api/v1/projects/{project}/sprints/{sprint}/prompts/{stage}` | `GET`, `HEAD` | `200` content-free JSON summary |
| `/api/v1/studies` | `GET`, `HEAD` | `200` JSON collection |
| `/api/v1/studies/{study}` | `GET`, `HEAD` | `200` JSON |
| `/api/v1/validations?scope=...&ref=...` | `GET`, `HEAD` | `200` JSON |
| `/api/v1/artifacts/{opaque-ref}` | `GET`, `HEAD` | `200` JSON |
| `/api/v1/health` | `GET`, `HEAD` | `200` or truthful `503` JSON |
| `/api/v1/operations/prepare` | `POST` | `200` JSON confirmation |
| `/api/v1/operations` | `POST` | `202` JSON and `Location` |
| `/api/v1/operations/{id}` | `GET`, `DELETE` | `200`; cancellation request may be `202` |
| `/api/v1/operations/{id}/events` | `GET` | `200` `text/event-stream` |
| `/api/v1/runs` | `GET`, `HEAD` | `200` JSON collection |
| `/api/v1/timeline?sprint=...` or `?study=...` plus `window=6h\|24h\|7d\|30d&limit=1..50` | `GET`, `HEAD` | `200` JSON |

Success envelopes are `{data, meta}` and errors are `{error, meta}`. The exact
transport DTO tags and Go types are frozen in the compatibility test. Unknown
`/api/` paths and wrong methods remain JSON. State-bearing HTML, JSON, errors,
confirmations, operation projections, and SSE use `Cache-Control: no-store`.
Static assets use `public, max-age=0, must-revalidate`; filenames are not
content-addressed.

The prompt-summary route is an additive observability surface. It is loaded on
demand from the sprint Run page, never invokes an agent, never writes sprint
state, and omits raw prompt and artifact contents. Expected prerequisite gaps
return an available input contract with `available: false`; unknown stages keep
the normal `404` envelope.

## Boundary Inventory

Production files in `internal/web` import only Go standard-library packages
and `internal/app`; `TestWebImportBoundary` enforces this directly. Markdown
conversion is app-owned through `app.RenderSafeMarkdown`. Web owns transport
DTOs, explicit page models, templates, browser security, ephemeral operation
records, subscriber queues, and SSE framing. It owns no filesystem discovery,
product state interpretation, runtime/process construction, mutation lock,
workflow branch, executable invocation, or durable recovery rule.

`cmd/ultraplan` explicitly composes the shared app query/operation capability
into the web runner. There is no subprocess call back to `ultraplan`, dynamic
stage registry, service locator, package-global runner, or web database.

## Lifecycle And Lock Inventory

The server owns one operation root context. The hub lock protects records,
event buffers, subscribers, counters, terminal arbitration, draining, and the
hashed confirmation-to-operation deduplication index. App callbacks, canonical
cancellation, cleanup recording, sends, and waits occur outside that lock.
Product modules retain their own sprint/study mutation leases and durable
terminal/reconciliation writes.

Start ordering is: reap -> reject draining/capacity -> resolve an existing
deduplication record -> atomically validate/consume confirmation -> reserve
capacity -> publish the operation -> launch owned work. Shutdown ordering is:
drain -> snapshot active records -> invoke each canonical cancellation once ->
wait under the shutdown bound -> persist owner-specific uncertainty where
needed -> publish terminal state -> close subscribers -> stop HTTP. Startup
reconciliation delegates to `app.OperationReconciler`; process absence, HTTP
completion, SSE delivery, and artifact presence never imply product success.

## Presentation And Asset Inventory

Templates parse once from embedded assets. Definitions use `primitive/*`,
`component/*`, `layout/*`, and `page/*`; startup rejects missing definitions,
duplicates, cycles, unnamespaced references, and upward/same-layer calls.
Route pages remain complete without JavaScript. CSS is exposed through tokens,
base, primitives, components, layouts, and utilities layers. JavaScript is
dependency-free and split into baseline lifecycle, operation command, and SSE
ownership helpers while the compatibility bundle preserves Sprint 31 browser
behavior.

All assets are embedded. The package test builds the binary, initializes and
serves a workspace from a temporary directory outside the checkout, and
requests every page/API collection and static asset needed at startup.

## Effective Fixed Bounds

The immutable built-in policy is validated before listening: 5s header, 15s
read, 30s write, 60s idle, and 10s shutdown timeouts; 32 in-flight requests;
8 KiB request targets; 64 KiB command bodies; 128-byte identifiers; 8 active
operations; 128 confirmations for 2 minutes; 256 retained events and 256 KiB
per operation; 16 KiB per encoded event; 256 KiB terminal results; 8
subscribers per operation and 32 streams server-wide; 32 queued events per
subscriber; 10-minute terminal retention; 15-second heartbeat; and 30-minute
stream lifetime.

Only `--listen` and `--open-browser` are operator-facing web flags. No
workspace or environment names for weakening safety caps existed in the
Sprint 30-31 public baseline, so Sprint 32 does not invent them. This is an
explicit plan deviation: built-in caps participate in the normal immutable
effective server configuration but are not externally overrideable. Adding
overrides requires named fields, ranges, precedence tests, and a compatibility
decision in a later governed change.

## Reviewed Baseline Differences

| Difference | Classification | Resolution And Rationale |
| --- | --- | --- |
| Same-IP, different-port Origin was accepted while guides said exact same-origin. | Implementation defect | Enforce exact scheme/host/port for browser Origin. Signed session and CSRF remain independent defenses. |
| Markdown was rendered safely while guides still described escaped Markdown source. | Documentation defect | Document safe app-owned GFM rendering; JSON and fallback source remain escaped. |
| Static assets inherited `no-store` despite the planned revalidation policy. | Implementation defect | Preserve non-content-addressed URLs and require revalidation. |
| Retrying a successfully consumed start token returned `confirmation_replayed`. | Implementation defect | Hash the session/token into a bounded deduplication record and return the original operation/Location atomically. Invalid or mismatched replay remains rejected. |
| External names for every web resource limit were absent. | Explicit compatibility decision | Preserve and validate fixed safe defaults; do not invent an override surface during hardening. |
## Durable operation compatibility

The frozen operation routes and JSON field order remain unchanged. Newly
accepted operations use their durable `run_*` identity as `operation.id` and
add a canonical run `Link` header. Active operation list/detail/event/cancel
reads can be projected from the workspace repository when the observing server
does not hold the originating in-memory record; transient `op_*` records remain
recognized for the lifetime of their pre-durable server record.

Compatibility SSE keeps the stable names `snapshot`, `progress`, `warning`,
`finding`, `artifact`, `cancel_requested`, `recovery_required`, and `terminal`.
Durable event sequence is the event ID. Missing compacted history produces an
explicit recovery response rather than fabricated events. Canonical run APIs
are additive and are the recovery authority after server/session restart.

## Run-history timeline

`/api/v1/timeline` is a later additive, read-only observability surface in the
same spirit as the canonical run APIs. It projects durable runs for exactly one
sprint or study scope onto a shared time axis with committed tool-event
timestamps, carries no raw payloads, and never mutates state. It backs the
browser run-history chart on the sprint Run page and the study Progress page;
those pages remain complete without JavaScript.
