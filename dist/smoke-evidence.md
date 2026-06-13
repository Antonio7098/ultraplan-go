# Smoke Evidence

Date: 2026-06-13
Timestamp UTC: 2026-06-13T12:26:43Z
Working directory: `/home/antonioborgerees/coding/ultraplan-go`

## Dependency Fix

The initial Darwin cross-compiles failed because pinned `github.com/Antonio7098/agentwrap/opencode` used Linux-only `syscall.SysProcAttr.Pdeathsig` under a Unix build tag.

Fix committed and pushed to `github.com/Antonio7098/agentwrap`:

- `opencode/process_linux.go` now owns the Linux `Pdeathsig` behavior.
- `opencode/process_unix.go` now applies to `unix && !linux` and sets only `Setpgid`.
- `opencode/process_unix_test.go` is Linux-only because it asserts `Pdeathsig`.

Pushed commit:

```text
bc655a2 fix darwin opencode process build
```

UltraPlan was built with the non-local module version:

```text
github.com/Antonio7098/agentwrap v0.0.0-20260613122459-bc655a256a5f
```

`go.mod` contains no local `replace` directive.

## Offline Gates

| Command | Result | Notes |
| --- | --- | --- |
| `go test ./...` | pass | All packages passed; no OpenCode, provider credentials, network, or real subprocess smoke fixtures required. |
| `go test -race ./...` | pass | All packages passed under the race detector. |
| `go build ./cmd/ultraplan` | pass | Native CLI build succeeded. |

## Agentwrap Verification

| Command | Result | Notes |
| --- | --- | --- |
| `cd /home/antonioborgerees/coding/agentwrap && go test ./...` | pass | Native dependency tests passed before push. |
| `cd /home/antonioborgerees/coding/agentwrap && GOOS=darwin GOARCH=amd64 go test -c -o /tmp/agentwrap-opencode-darwin-amd64.test ./opencode` | pass | Darwin amd64 compile check passed before push; test binary was not executed on Linux host. |
| `cd /home/antonioborgerees/coding/agentwrap && GOOS=darwin GOARCH=arm64 go test -c -o /tmp/agentwrap-opencode-darwin-arm64.test ./opencode` | pass | Darwin arm64 compile check passed before push; test binary was not executed on Linux host. |

## Packaging

| Command | Result | Notes |
| --- | --- | --- |
| `GOOS=linux GOARCH=amd64 go build -o dist/ultraplan-linux-amd64 ./cmd/ultraplan` | pass | Produced `dist/ultraplan-linux-amd64`. |
| `GOOS=linux GOARCH=arm64 go build -o dist/ultraplan-linux-arm64 ./cmd/ultraplan` | pass | Produced `dist/ultraplan-linux-arm64`. |
| `GOOS=darwin GOARCH=amd64 go build -o dist/ultraplan-darwin-amd64 ./cmd/ultraplan` | pass | Produced `dist/ultraplan-darwin-amd64`. |
| `GOOS=darwin GOARCH=arm64 go build -o dist/ultraplan-darwin-arm64 ./cmd/ultraplan` | pass | Produced `dist/ultraplan-darwin-arm64`. |

## Artifact Inspection

Present artifacts:

```text
dist/ultraplan-linux-amd64
dist/ultraplan-linux-arm64
dist/ultraplan-darwin-amd64
dist/ultraplan-darwin-arm64
```

File type inspection:

```text
dist/ultraplan-linux-amd64:  ELF 64-bit LSB executable, x86-64
dist/ultraplan-linux-arm64:  ELF 64-bit LSB executable, ARM aarch64
dist/ultraplan-darwin-amd64: Mach-O 64-bit x86_64 executable
dist/ultraplan-darwin-arm64: Mach-O 64-bit arm64 executable
```

## Checksums

Command:

```bash
sha256sum dist/ultraplan-linux-amd64 dist/ultraplan-linux-arm64 dist/ultraplan-darwin-amd64 dist/ultraplan-darwin-arm64 > dist/checksums.txt
```

Result:

```text
8fefdde7dd9f1b52bca37aba6e497329be781ca4de1627e5522574c7ba9c9ef7  dist/ultraplan-linux-amd64
bb20f626e15a7c9cb8b65d813df7918eace77c284aae75d935662727f6be335d  dist/ultraplan-linux-arm64
fa2b6e5e8abd364affab995e53e87ba261af5e3ec89374387322d26245d0fe40  dist/ultraplan-darwin-amd64
4ae57a93bbc433e8250d1fb0848278bf15995e61e796e29b2b08979e59fee287  dist/ultraplan-darwin-arm64
```

`dist/checksums.txt` contains exactly four entries and no unrelated files.

## Gated OpenCode Smoke

Status: existing smoke evidence reviewed; no new smoke run was executed during this packaging update.

Evidence repository: `/home/antonioborgerees/coding/ultraplan-go-smoke`.

Latest recorded full run:

```text
Run ID: run-UZw1lC12d3
Runtime: minimax-coding-plan/MiniMax-M2.7
Result: 212/218 passed (--level all, 20.3 min)
Command: OPENCODE_MODEL=minimax-coding-plan/MiniMax-M2.7 node --import tsx/esm src/cli.ts smoke --level all --ultraplan ~/coding/ultraplan-go --workspace ~/coding/ultraplan-go-smoke
```

Latest Sprint 14 deep smoke:

```text
Run ID: run-oExhM6FukU
Runtime: minimax-coding-plan/MiniMax-M2.7
Result: 28/28 passed (8.3 s)
Command: OPENCODE_MODEL=minimax-coding-plan/MiniMax-M2.7 node --import tsx/esm src/cli.ts smoke --level sprint-14 --ultraplan ~/coding/ultraplan-go --workspace ~/coding/ultraplan-go-smoke
```

Residual risk: the full smoke baseline has historical open failures documented in `/home/antonioborgerees/coding/ultraplan-go-smoke/README.md`; no new full smoke run was executed after the packaging-only dependency update. Offline fake-first tests, native build, dependency compile checks, and all release package builds passed against the pushed non-local dependency.

## Redaction Review

This evidence records commands, pass/fail summaries, artifact names, checksums, and dependency compiler disposition only. It does not include provider tokens, full environment dumps, full prompts, full generated report bodies, or raw runtime payloads.
