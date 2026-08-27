# Release Checklist

This checklist gates the local Phase 3 CLI and TUI release. It does not publish, sign, notarize, tag, upload, or create a GitHub release.

## Scope

- Study workflows and governed sprint delivery through `execute -> review -> smoke -> merge`.
- CLI and TUI support integrated `verify`, resumable/focused review, review-gated smoke, status, validation, cancellation, and recovery.
- Local numeric-loopback browser UI with guarded operations and bounded SSE.
- No issue management, hosted SaaS, remote/multi-user collaboration, or Git automation beyond configured stage publication, sprint worktree creation, and governed sprint merge.
- Runtime integration remains through agentwrap/OpenCode.

## Offline Gates

Run from the repository root:

```bash
go test ./...
go test -race ./...
go build ./cmd/ultraplan
go vet ./...
git diff --check
```

Failures block release unless separately triaged and recorded.

Also require the fake-runtime review suite and fake smoke-harness suite to pass without network, OpenCode, ambient credentials, or the external harness. Required real-runtime prerequisites that are unavailable must be reported as `blocked`, never `pass`.

Local-web gates additionally require:

```bash
go test ./internal/app -run 'TestWeb|TestOperation|TestCapability|TestRenderSafeMarkdown'
go test ./internal/web -run 'TestAPICompatibility|TestSecurity|TestOperation|TestSSE|TestServer|TestTemplate|TestIntegration|TestPackagedBinary'
go test -race ./internal/web ./internal/app
go run ./cmd/ultraplan serve --help
```

Confirm the import boundary, exact-origin/CSRF/session policy, static revalidation,
forbidden-value scans, duplicate-start deduplication, shutdown uncertainty,
namespaced templates, all layered assets, and the outside-tree binary launch.

Record manual keyboard-only navigation, visible focus, live announcement timing,
non-color states, reduced motion, 200% zoom/text enlargement, and narrow reflow
for dashboard, sprint run, confirmation, active/terminal operation, artifact,
and error pages. Automation does not turn these manual checks into a pass.

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
- gated planning runtime smoke pass/fail/skip status.
- redaction statement.
- residual risks.

## Gated OpenCode Smoke

Run [opencode-smoke.md](opencode-smoke.md) only when OpenCode, provider config, network access, and a prepared smoke study are available. Otherwise record an explicit skip reason.

## Gated Planning Smoke

Run [planning-smoke.md](planning-smoke.md) only when OpenCode, provider config, network access, and a prepared planning project/sprint are available. Otherwise record an explicit skip reason. Always run the offline planning checks from that document when a fixture project is available.

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
- CLI reference matches `ultraplan --help`, `ultraplan config --help`, `ultraplan health --help`, `ultraplan project --help`, `ultraplan sprint --help`, `ultraplan study --help`, and `ultraplan code --help`.
- Stage-skill documentation matches `ultraplan skills --help` and `ultraplan skills materialise --help`.
- All nine embedded stage skills materialise idempotently and remain manual-only.
- Stable JSON documentation is limited to documented JSON surfaces.
- Recovery docs describe validation, missing artifacts, failed planning stages, cancellation, stale locks, `--force-unlock`, partial completion, retry/fallback metadata, and atomic write failures.
- Configuration docs document precedence, schema version rejection, runtime/model/retry/fallback settings, agentwrap/OpenCode mapping, and redaction.
- Migration docs explain `.ultra/cli` artifact compatibility, supported stage publication, and deferred issue and general Git workflows.

## Security Review

Confirm docs and evidence contain no:

- provider tokens.
- full sensitive environment dumps.
- full raw prompts.
- full generated report bodies.
- raw unsafe runtime payloads.
- unsupported direct OpenCode supervision claims.
- unsupported Git mutation claims beyond configured stage publication and sprint worktree creation.

## Read-only QA gates

- Two unchanged `qa --dry-run --json` executions have byte-identical normalized map payloads, IDs, ordering, budgets, and fingerprints.
- Every changed path has exactly one primary owner; unknown paths block; boundary overlap and follow-up parent links stay within frozen limits.
- Strict state tests cover unknown versions/fields, trailing JSON, private modes, containment and symlink escape, digest mismatch, atomic failure, pointer-last ordering, stale fencing, retention, quota, recovery, and prior-state preservation.
- Runtime-policy tests prove restricted read-only/default-deny translation, required permission capability, assigned path rules, unsupported-enforcement rejection, no shell/Git/interpreter/write/cache commands, bounded environment names, output/time limits, target and governed-input drift detection, and no child after acceptance failure.
- Scheduler tests cover fixed concurrency, queue bounds, retry exhaustion, cancellation before admission and while active, timeout, terminal promotion ordering, resume without worker adoption, restart interruption, and runtime-free recovery.
- Theory/synthesis tests cover every outcome, falsifiable fields, negative retention, deterministic deduplication, contradictions, interactions, blockers, stale input, and bounded follow-ups with no issues, repairs, patches, generated checks, `qa.md`, or verdict promotion.
- CLI text/JSON, TUI, browser HTML/JSON, app DTOs, and durable run detail agree on fingerprints, coverage, shard progress, outcomes, synthesis, blocker, cancellation, terminal result, and next action. HTML remains complete without JavaScript and escapes hostile retained text.
- `review` and `conformance-review` invoke the same capability. `review`, `review.md`, existing JSON fields, verdict rules, smoke behavior, and planning-stage order remain compatible while human labels say Conformance Review.
- Focused race checks pass with `go test -race ./internal/sprint ./internal/app ./internal/tui ./internal/web`; the full `go test -race ./...` gate also passes.
- `go test ./...`, `go vet ./...`, `go build ./cmd/ultraplan`, and `git diff --check` pass with retained evidence.
- Real-runtime dogfood records two stable maps, read-only shard/synthesis state, durable progress, explicit cancellation/recovery, and clean content plus Git identity. Missing prerequisites are recorded as blocked and do not satisfy the gate.
- Sprint 38 manual repair source may ship only after current Sprint 37 evidence admits one real issue and all manual repair gates below pass. Until then packet admission and automatic mode remain unavailable; missing dogfood evidence is a blocked release gate, not an inferred pass.

## Platform Notes

Linux and macOS binaries are local release artifacts. macOS binaries are cross-compiled, unsigned, and unnotarized. Users may need to handle local OS trust prompts. No installer packages are produced by this checklist.

## Prompt And Version Metadata

Before publishing, confirm:

- `ultraplan version` reports intended build metadata.
- prompt/template changes are intentional.
- runtime metadata redaction remains active in status, health, logs, and smoke evidence.
- run-state and JSON schema versions are documented where stable.
- Durable run-control schema, migration backup/restore, WAL/FULL-sync, private
  permissions, unsupported-schema, and corruption checks pass.
- Every asynchronous CLI, TUI, web, runtime-child, and external-harness entry
  has acceptance/claim-before-start and no-child-on-persistence-failure evidence.
- Canonical run CLI/JSON/HTML/SSE and operation compatibility fixtures agree on
  identity, lifecycle, liveness, cancellation, product status, cursor gaps, and
  terminal outcomes across process/session restart.
- Retention, quota/headroom, owner lease/fencing/process-birth, reconciliation,
  terminal races, sanitization, diagnostics, and private bounded support export
  tests pass under normal and race suites.
- Release evidence records `modernc.org/sqlite` version, binary-size impact,
  Linux/macOS amd64/arm64 build results, local-filesystem topology limits, and
  polling/write measurements.
- Architecture Review and Sprint Review are current before Deep Smoke Sprint.
  Missing runtime/browser/platform prerequisites are recorded as blocked, never
  promoted to passing evidence.

Sprint 35 local release evidence (2026-08-21):

- `modernc.org/sqlite v1.57.0` is pinned; the Linux amd64 CLI is 33,635,397
  bytes, 6,923,097 bytes above the 26,712,300-byte pre-app-wiring checkpoint.
- Linux arm64 and Darwin amd64/arm64 CLI cross-builds pass; Darwin amd64/arm64
  run-control test binaries also cross-compile.
- On the recorded Linux amd64 developer machine, `synchronous=FULL` committed
  append benchmarks measured 0.99–1.40 ms/op and 512-event indexed replay
  measured 2.05–2.18 ms/page over three one-second samples. These figures are
  release evidence, not latency guarantees.

## Evidence-producing QA release gate

- Prove the source identity before and after every disposable check and confirm
  that Git state is unchanged.
- Exercise bounded non-Git and dirty-tree copying, unsafe entry rejection,
  native protected-root denial, cancellation, descendant cleanup, workspace
  removal, and unapproved-change rejection.
- Validate frozen plans, v1 reads, v2 writes, immutable evidence, adjudication
  rejection, issue grouping, assessment, stale fences, rollback, and recovery.
- Compare `smoke` with `qa --suite smoke` for containing selection, argv,
  environment, timeout, external run identity, evidence, verdict, cleanup,
  `smoke.md`, and flow state. Confirm one durable QA run and no nested smoke run.
- Compare the app, CLI text/JSON, TUI, browser HTML/JSON, `qa.md`, and private
  state for one bounded fixture, including hostile display text.
- Run `go test ./...`, `go test -race ./...`, `go vet ./...`,
  `go build ./cmd/ultraplan`, and `git diff --check`.
- Gate real dogfood on a current acceptable review, containing smoke evidence,
  runtime availability, and native isolation. Missing capability or cleanup
  certainty is `blocked`, never a waived pass.

Disposable copies reduce accidental target mutation but are not a hostile
multi-tenant sandbox. Evaluator majority does not prove independent truth.
QA retains patches as evidence only and cannot apply them. The separate repair
protocol may apply one explicitly confirmed, product-derived, journaled patch
for one current repair-eligible issue; it cannot reuse ordinary QA runtime
authority or change Conformance Review, smoke, QA evidence, tests, governed
inputs, configuration, repository controls, or Git state.

## Bounded manual repair release gate

- Prove deterministic packet bytes and rejection of stale, incomplete, cross-attempt, protected-path, target-drift, and ambiguous issue authority without runtime construction or target mutation.
- Prove durable accept, writer claim, single-use confirmation, and dispatch ordering for CLI, TUI, browser, and the shared runner. Acceptance or confirmation persistence failure must start no child.
- Exercise isolation, exact scope, preimage checks, hard-link and symlink rejection, private preimage staging, apply-journal updates, atomic replacement, partial-failure compensation, and crash recovery. No committed apply may repeat.
- Prove the ordered exact, shard, theory, follow-up, QA, and repaired-target smoke ladder. Conformance Review runs once before repair admission.
- Compare bounded CLI text/JSON, TUI, browser HTML/JSON, durable operation, repair state, result, and flow summary. Verify hostile text escaping and absence of patch bodies, production bytes, prompts, private preimages, environment values, and raw output.
- Run `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./cmd/ultraplan`, and `git diff --check`.
- Retain one real manual `verified` or `verified_with_findings` run with current target, complete ladder, proven cleanup, interface captures, and Architecture/Sprint/Deep Smoke reviews. If the prerequisite evidence is missing, record `blocked` and keep automatic mode unavailable.
