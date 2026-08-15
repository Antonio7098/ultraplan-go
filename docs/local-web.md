# Local Web Dashboard

UltraPlan can serve a read-only browser view of the current workspace from the
same Go binary:

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

Each request reads current app-owned workspace state. Responses use
`Cache-Control: no-store`; there is no server snapshot, browser database,
watcher, hidden preload, automatic polling, or browser-owned product state.
Use the visible Refresh link or an ordinary page reload after CLI/TUI changes.
A response is a bounded point-in-time projection, not a cross-request
transactional snapshot.

Collections and finding lists return at most 200 entries and report returned,
total, and truncated counts in JSON. Sprint 30 does not expose pagination,
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

Markdown and JSON are displayed as escaped source inside labelled code blocks.
Workspace HTML, scripts, Markdown links, and JSON strings are never installed as
trusted HTML or executed. JSON responses report total bytes, returned bytes,
truncation, and bounded JSON validity.

## Local Security And Trust Boundary

The server is for one local user and is not a hosted or remote service. It
reduces local exposure through:

- numeric loopback-only binding
- an exact canonical Host authority
- exact same-origin validation when `Origin` is present
- no permissive CORS response
- 8 KiB request-target, 64 KiB declared-body, and 128-byte identifier limits
- rejection of all request bodies and undocumented query parameters
- 32 in-flight request bound and fixed HTTP timeouts
- restrictive CSP, frame denial, `nosniff`, no-referrer, and no-store headers
- server-generated request IDs, safe error projection, and redacted diagnostics

An absent Origin is allowed for top-level navigation and local `GET`/`HEAD`
clients after Host validation. `Origin: null`, malformed, cross-origin, and
non-loopback origins are rejected. IPv4 and IPv6 server origins are distinct.

Loopback is not an authentication or isolation boundary against another process
running as the same OS user or a compromised local account. Sprint 30 uses no
cookies, accounts, authentication, TLS, tenant model, remote worker protocol, or
remote/LAN exposure. Do not proxy or port-forward this server.

## API And Health

The bundled browser uses versioned, read-only `/api/v1` JSON resources. Success
responses use `{data, meta}`; errors use `{error, meta}` with safe stable codes.
Unknown `/api/` paths and unsupported versions always return JSON rather than an
HTML page.

This API is compatibility-controlled for the bundled browser. It is not yet a
promised public integration API and has no remote-client or pagination support.
Breaking DTO changes require an explicit version or coordinated migration.

`GET /api/v1/health` reports only server readiness and lightweight availability
of the configured workspace query. `200`/`ok` means the server can answer that
query; `503`/`unavailable` means it cannot. Health does not validate every
artifact, scan projects or studies, contact a runtime/provider, or run review or
smoke. It is not proof that the whole product state is valid.

## Shutdown And Troubleshooting

Press Ctrl-C or send the process its normal termination signal. UltraPlan stops
accepting requests, propagates cancellation to active reads, waits up to 10
seconds for graceful HTTP cleanup, and reports lifecycle diagnostics on stderr.

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

There is no Node.js, Vite, frontend build, separate asset server, database,
runtime/provider, or smoke-harness prerequisite for normal dashboard use.
