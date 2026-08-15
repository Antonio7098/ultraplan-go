# Implementation Architecture

UltraPlan is module-driven. Product modules own their state and workflows;
local interfaces adapt shared typed application use cases.

## Local Interface Composition

The process entry point explicitly constructs the independent TUI and web
runners:

```text
cmd/ultraplan -> internal/app
cmd/ultraplan -> internal/tui
cmd/ultraplan -> internal/web

internal/tui -> internal/app
internal/web -> internal/app
internal/app -> product and platform modules
```

This composition avoids the prohibited `internal/app -> internal/web ->
internal/app` cycle. Runners are ordinary injected functions; there is no
package-global mutable registry, `init` callback, service locator, or
context-carried dependency. Web templates, HTTP state, and the listener are
initialized only after the `serve` command has completed workspace/config and
listen-policy preflight. Help, version, other CLI commands, and TUI startup do
not initialize web facilities.

## Web Adapter Boundary

`internal/app/web_usecases.go` owns the query-only browser capability. It
provides typed dashboard, project, sprint, study, validation, artifact, and
health projections; deterministic bounds; canonical identifiers; opaque
reference issuance/resolution; workspace containment; allowlisting; and typed
error identity.

`internal/web` receives only that interface and its plain app result types. It
owns:

- loopback `net/http` lifecycle and graceful shutdown
- HTML and `/api/v1` routing
- transport DTOs and safe error envelopes
- Host/Origin, request-limit, concurrency, and security-header middleware
- `html/template` view models and embedded first-party assets
- escaped source presentation and redacted request/lifecycle diagnostics

It does not import project, sprint, study, workspace, runtime, process, or CLI
handler packages. It cannot execute workflows, persist product state, invoke
providers or the smoke harness, run Git, or read arbitrary files.

## State And Artifact Ownership

Workspace files and product-owned flow, execute, review, smoke, and study run
state remain authoritative. Web requests perform fresh sequential app queries.
The server retains only immutable configuration, parsed embedded templates,
listener/server objects, request contexts and IDs, response models, opaque
reference mappings, and bounded preview buffers.

There is no web cache, watcher, snapshot, browser persistence, database,
background polling, operation hub, confirmation store, or SSE subscriber state
in Sprint 30. Read-only sprint status uses the sprint service's non-persisting
projection mode, so a browser read does not create missing flow state; existing
CLI/TUI behavior remains unchanged.

Opaque artifact references are issued by the app boundary. Resolution repeats
the allowlist check, lexical containment, and symlink-aware canonical
containment before a bounded file read. `internal/web/artifacts.go` validates
the returned media/size contract and renders the source; it has no filesystem
capability.

## Single-Binary Frontend

Go embeds templates, one stylesheet, and one dependency-free progressive script
under `internal/web`. Templates parse once when serving starts and pages render
to a buffer before response headers. Contextual `html/template` escaping,
escaped Markdown/JSON source blocks, CSP, and `nosniff` keep hostile workspace
content inert.

Initial HTML is complete without JavaScript and uses semantic headings,
navigation, breadcrumbs, landmarks, tables/definition lists, status text,
visible focus, narrow single-column reflow, local code/table overflow, zoom
support, and reduced-motion behavior. No Node.js, Vite, framework, hydration,
client router/store, third-party assets, separate frontend process, or asset
build step exists.

## Deferred Phase 4 Capabilities

Sprint 31 may add separately reviewed command capabilities for guarded
validation/workflow operations, normalized confirmations, product-owned
mutation locking, operation handles, explicit cancellation, and bounded SSE.
Those capabilities must not widen Sprint 30's query facade implicitly.

Hosted or LAN/public serving, accounts, authentication, TLS, teams, tenants,
collaboration, remote workers, browser editing, WebSockets, terminal transport,
general-purpose issue tracking, automatic fixes, database state, and Git
mutation remain outside the local web architecture.
