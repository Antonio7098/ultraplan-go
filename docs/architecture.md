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

`internal/app/web_usecases.go` owns the browser query facade, while the closed
operation capability in `internal/app/operations.go` is shared with the TUI. It
provides typed dashboard, project, sprint, study, validation, artifact, and
health projections plus allowlisted operation normalization, affected paths,
mutation class, prerequisites, governed-input inventory, SHA-256 fingerprint,
safe progress/result projections, and canonical context cancellation.

`internal/web` receives only that interface and its plain app result types. It
owns:

- loopback `net/http` lifecycle and graceful shutdown
- HTML and `/api/v1` routing
- transport DTOs and safe error envelopes
- Host/Origin, request-limit, concurrency, and security-header middleware
- per-process session/CSRF and short-lived binding confirmation policy
- the bounded ephemeral operation hub, retained safe event/result projections,
  progress-only SSE, and subscriber lifecycle
- `html/template` view models and embedded first-party assets
- escaped source presentation and redacted request/lifecycle diagnostics

It does not import project, sprint, study, workspace, runtime, process, or CLI
handler packages. It cannot decide workflow semantics, persist product state,
invoke providers or the smoke harness directly, run Git, or read arbitrary
files. It starts only the typed app operation capability supplied by the
composition root.

## State And Artifact Ownership

Workspace files and product-owned flow, execute, review, smoke, and study run
state remain authoritative. Web requests perform fresh sequential app queries.
The server retains only immutable configuration, parsed embedded templates,
listener/server objects, request/session IDs, opaque artifact references,
short-lived confirmations, and bounded ephemeral operation/event/subscriber
state. The hub is transport lifecycle state, not workflow authority: it holds
at most eight active owners, recent already-redacted events, cancellation
handles, and terminal projections for ten minutes. It never persists a queue
or operation history.

Read-only sprint status uses the sprint service's non-persisting projection
mode. Product-owned workspace artifacts, execute/review/smoke state, study run
state, and per-sprint/study mutation locks remain authoritative. Restart and
replay-gap recovery direct users back to that durable state rather than
reconstructing product truth from the hub. Server startup acquires product
leases conservatively and reconciles only dead-owner sprint attempts; live
cross-process work is not rewritten.

Opaque artifact references are issued by the app boundary. Resolution repeats
the allowlist check, lexical containment, and symlink-aware canonical
containment before a bounded file read. `internal/web/artifacts.go` validates
the returned media/size contract and renders the source; it has no filesystem
capability.

## Single-Binary Frontend

Go embeds the namespaced template hierarchy and all layered CSS/JavaScript under
`internal/web`. Templates parse once when serving starts; validation rejects
missing definitions, duplicates, cycles, unnamespaced references, and upward or
same-layer dependencies before a request can be accepted. Pages render to a
buffer before response headers. Contextual `html/template` escaping, app-owned
safe Markdown rendering, escaped JSON/fallback source, CSP, and `nosniff` keep
hostile workspace content inert.

Definitions compose downward through `page/* -> layout/* -> component/* ->
primitive/*`. CSS exposes tokens, base, primitives, components, layouts, and
utilities layers. Dependency-free JavaScript separates baseline page lifetime,
HTTP operation commands, and SSE ownership while preserving the compatibility
bundle used by the Sprint 31 browser.

Initial HTML is complete without JavaScript and uses semantic headings,
navigation, breadcrumbs, landmarks, tables/definition lists, status text,
visible focus, narrow single-column reflow, local code/table overflow, zoom
support, and reduced-motion behavior. No Node.js, Vite, framework, hydration,
client router/store, third-party assets, separate frontend process, or asset
build step exists.

## Operation Ownership And Shutdown

Preparation is side-effect-free and does not reserve capacity or acquire a
mutation lock. Start repeats normalization and fingerprinting, consumes one
session-bound confirmation, and creates a server-owned context immediately;
there is no web queue. Sprint flow, execute, review, smoke, and verify use one
product-owned per-sprint cross-process mutation lease. Study run-loop keeps its
independent product lock.

Each accepted operation has one canonical cancel function and terminal
arbitration point. Slow or disconnected SSE subscribers cannot block or cancel
product work. Graceful shutdown enters draining, rejects new work, requests
`server_shutdown` cancellation once per owner, waits outside hub/product locks
for bounded cleanup and durable reconciliation, publishes a truthful terminal
event, closes subscribers, and only then shuts down HTTP. If the deadline
expires, the app boundary atomically persists a product-owned sprint recovery
marker before transport closure. That marker is separate from canonical run
state so the web layer never races a live lease holder; startup consumes it
only after canonical state is reconciled under the normal product lease. No
detached operation is intentionally allowed to outlive the server.

Smoke authoring uses before/after target identities for diagnosis and retained
runtime tool events for write attribution. Concurrent target drift without an
observed protected-path write is recorded but does not fail a local smoke run;
an author-attributed protected write and any out-of-allowlist harness mutation
remain hard failures. Attribution observes the live runtime event callback and
does not depend on the bounded retained-event tail.

## Deferred Phase 4 Capabilities

Hosted or LAN/public serving, accounts, authentication, TLS, teams, tenants,
collaboration, remote workers, browser editing, WebSockets, terminal transport,
general-purpose issue tracking, automatic fixes, database state, and Git
mutation remain outside the local web architecture.

## Grounded Planning And Shared Prompt Boundary

`internal/sprint` owns `code-context` generation, validation, source-reference resolution, and downstream prompt composition. The complete common prefix is rendered once per top-level operation in this fixed order: stable shared instructions; project/sprint identity; an external frame containing the exact stored `requirements.md` bytes; an external frame containing the exact stored reference-only `code-context.md` bytes; transient resolved source evidence in authored order; and one constant stage-specific boundary as the final prefix bytes. Stage names, output paths/contracts, task and reviewer identities, model/run/session/attempt data, timestamps, and smoke scope occur only after that boundary or in runtime metadata.

Reference resolution is repository-contained, symlink-rejecting, regular-file-only, cancellation-aware, and fail-closed. References retain their authored labels, rationale, and symbol metadata while selected ranges from the same file are sorted and merged so source bytes are injected once and each file is scanned once. The complete prefix is capped at 256 KiB, with 64 KiB reserved for stage suffixes; overflow is an actionable error, never truncation or omission. Evidence is marked untrusted, and agents retain permission to inspect additional live source.

After code-context validation, UltraPlan may persist a bounded, content-addressed, disposable context pack under `.ultra/cache/sprint-context/`; existing sprints create one lazily on their first runtime composition while prompt previews stay read-only. Its identity is derived only from the exact requirements, exact code-context artifact, and canonical target; it freezes the resolved source bytes so execution edits do not churn the planning prefix. It is an acceleration layer, never provenance or freshness authority: write failure is non-fatal, missing or invalid packs fall back to live resolution, and artifact changes select a different identity without invalidating, rerunning, or reopening completed stages. Exact-match dependency freshness remains disabled.

`internal/platform/runtime` receives the final ordinary prompt plus content-free cache metadata: the stable-prefix digest, byte breakpoint, and a provider/model/work-directory cohort key. The current agentwrap/OpenCode boundary transports these values as metadata only; it cannot yet place a native cache-control breakpoint inside the single provider message, so no cache hit is guaranteed. Planning, execute, independent review requests, and agent-backed smoke authoring call the sprint-owned composition boundary explicitly. Review fan-out shares one immutable prefix. Runtime results append bounded, content-free prompt, token, cache-read/cache-write, cost, and timing measurements to the sprint's `.runtime-metrics.json`; `sprint ... metrics` exposes them. Prepared handoff packets and per-stage input contracts minimize downstream artifact reads without becoming dependency fingerprints; when present, the technical handbook's `Examples Worth Investigating` (or legacy `Examples Worth Inspecting`) section is copied directly into plan and execute prompts.

Execute owns one reusable runtime session for the ordered pending-task queue rather than one independent agent per task. Its initial turn carries the shared sprint prefix, queue primer, current task, safety instructions, and optional handbook examples. After each task UltraPlan persists task-specific evidence and status, then submits only the next task delta with `SessionAction=continue`. A missing runtime session ID degrades safely to another complete prompt; failure or cancellation stops queue advancement, while explicit deferral may advance to the next task. Cross-command session reuse is based on model and target compatibility, not exact artifact fingerprints.
