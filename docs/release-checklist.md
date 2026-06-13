# Release Checklist

This checklist gates local study-side release artifacts. It does not publish, sign, notarize, tag, upload, or create a GitHub release.

## Scope

- Study-side CLI only.
- No target scaffolding, sprint planning, sprint execution, hosted SaaS, browser UI, multi-user collaboration, or automatic Git mutation.
- Runtime integration remains through agentwrap/OpenCode.

## Offline Gates

Run from the repository root:

```bash
go test ./...
go test -race ./...
go build ./cmd/ultraplan
```

Failures block release unless separately triaged and recorded.

## Packaging

Build exactly four local binaries:

```bash
mkdir -p dist
GOOS=linux GOARCH=amd64 go build -o dist/ultraplan-linux-amd64 ./cmd/ultraplan
GOOS=linux GOARCH=arm64 go build -o dist/ultraplan-linux-arm64 ./cmd/ultraplan
GOOS=darwin GOARCH=amd64 go build -o dist/ultraplan-darwin-amd64 ./cmd/ultraplan
GOOS=darwin GOARCH=arm64 go build -o dist/ultraplan-darwin-arm64 ./cmd/ultraplan
```

Confirm the four files exist and names match target `GOOS`/`GOARCH`.

## Checksums

Generate exactly four SHA-256 entries:

```bash
sha256sum dist/ultraplan-linux-amd64 dist/ultraplan-linux-arm64 dist/ultraplan-darwin-amd64 dist/ultraplan-darwin-arm64 > dist/checksums.txt
```

Do not include `smoke-evidence.md` or unrelated files in `checksums.txt`.

## Smoke Evidence

Create `dist/smoke-evidence.md` with:

- date/time and working directory.
- offline command results.
- package target commands.
- checksum command and result.
- gated OpenCode smoke pass/fail/skip status.
- redaction statement.
- residual risks.

## Gated OpenCode Smoke

Run [opencode-smoke.md](opencode-smoke.md) only when OpenCode, provider config, network access, and a prepared smoke study are available. Otherwise record an explicit skip reason.

## Dependency Provenance

Audit `go.mod` before publication:

```bash
grep -n '^replace ' go.mod
grep -n 'github.com/Antonio7098/agentwrap' go.mod
```

If `replace github.com/Antonio7098/agentwrap => ../agentwrap` or any other local replace remains, do not publish artifacts until its disposition is explicitly approved and recorded.

## Documentation Review

Check:

- README links every release document.
- CLI reference matches `ultraplan --help`, `ultraplan config --help`, `ultraplan health --help`, `ultraplan study --help`, and `ultraplan code --help`.
- Stable JSON documentation is limited to documented JSON surfaces.
- Recovery docs describe validation, missing artifacts, cancellation, stale locks, `--force-unlock`, partial completion, retry/fallback metadata, and atomic write failures.
- Configuration docs document precedence, schema version rejection, runtime/model/retry/fallback settings, agentwrap/OpenCode mapping, and redaction.

## Security Review

Confirm docs and evidence contain no:

- provider tokens.
- full sensitive environment dumps.
- full raw prompts.
- full generated report bodies.
- raw unsafe runtime payloads.
- unsupported direct OpenCode supervision claims.
- unsupported automatic Git mutation claims.

## Platform Notes

Linux and macOS binaries are local release artifacts. macOS binaries are cross-compiled, unsigned, and unnotarized. Users may need to handle local OS trust prompts. No installer packages are produced by this checklist.

## Prompt And Version Metadata

Before publishing, confirm:

- `ultraplan version` reports intended build metadata.
- prompt/template changes are intentional.
- runtime metadata redaction remains active in status, health, logs, and smoke evidence.
- run-state and JSON schema versions are documented where stable.
