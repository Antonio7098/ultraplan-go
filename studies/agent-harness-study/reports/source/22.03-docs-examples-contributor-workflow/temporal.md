# Source Analysis: temporal

## Dimension 22.03 — Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | temporal |
| Path | `studies/agent-harness-study/sources/temporal` |
| Language / Stack | Go (Temporal durable-execution server; protobuf/gRPC, Makefile-driven toolchain) |
| Analyzed | 2026-08-24 |

## Summary

The Temporal server repo has a mature, multi-layered contributor documentation model, but it is deliberately split across repos rather than self-contained. The repo root carries a full contributor path (`CONTRIBUTING.md`: prerequisites, build, three test tiers, local run with six persistence backends, IDE debugging, and a documented workflow for working against local `api`/`api-go`/`sdk-go` proto changes at `CONTRIBUTING.md:202-243`). A curated `docs/` tree (`docs/README.md:6-19`) separates architecture deep-dives (16 docs, ~3.4k lines total), developer guides (new RPCs, testing, tracing), and admin ops.

Teaching-by-example is handled in two ways instead of an `examples/` directory (none exists in-repo): (1) user-facing samples are delegated to external per-SDK repos linked from `README.md:46` and `CONTRIBUTING.md:167`, and (2) extension authoring is taught through step-by-step interface contracts inside the tree — most notably `common/archiver/README.md:1-272`, which walks a contributor through implementing and registering a new archiver both built-in and externally injected, including YAML config wiring and a retry/error FAQ. Package-level READMEs exist where onboarding matters (`config/dynamicconfig/README.md:1-40`, `temporaltest/README.md:1-7`, `service/history/workflow/update/README.md:1`, `common/effect/README.md:1`).

Unusually for a server project, the contributor workflow is also formalized *for AI agents*: a root `AGENTS.md` (104 lines) encodes project structure, commands, testing rules, and error-handling policy (`AGENTS.md:25-104`), `.github/copilot-instructions.md` (135 lines) is a detailed review rubric with severity levels (`.github/copilot-instructions.md:97-135`), and CI runs opt-in Claude PR reviews gated by org/team membership checks (`.github/workflows/claude-review-teams.yml:22-217`) driven by a checked-in review skill (`.claude/skills/review/SKILL.md:1-8`, which imports the copilot instructions).

Gaps: no generated API reference is produced from this repo (proto API docs live in the separate `temporalio/api` ecosystem; only linting via buf/api-linter exists at `Makefile:430-437`), no template/starter scaffolding exists, and some framework areas explicitly defer documentation ("docs TBD" at `docs/architecture/nexus.md:243`; CHASM guidance is conceptual rather than tutorial-shaped).

**Rating: 8/10**

Rationale against rubric ("clear model with tests, explicit interfaces, and operational safeguards" band, approaching mature): contribution guides are concrete and command-exact (`CONTRIBUTING.md:53-122`), the testing doc prescribes specific helpers and forbids anti-patterns (`docs/development/testing.md:36-88`) and those prescriptions are enforced by linters, not just prose (`.github/.golangci.yml:48` fails builds using deprecated `FunctionalTestBase`; `tools/parallelize/parallelize.go:160` auto-inserts `t.Parallel()`). The archiver guide is a true under-an-hour path to a working extension. It stops short of 9-10 because API reference generation is absent in-repo, examples for server-side extensions live outside the repo, and newer frameworks (CHASM/Nexus state machines) have architecture docs but no step-by-step "add your first component" tutorial.

## Evidence Collected

Every entry cites file paths with line numbers from within `studies/agent-harness-study/sources/temporal`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contribution guide | Root CONTRIBUTING.md covers CLA, prerequisites, build, test categories | `CONTRIBUTING.md:1-69` |
| Test-tier taxonomy | Unit / integration / functional test definitions with make targets | `CONTRIBUTING.md:73-104` |
| Local run instructions | `make start`, SQLite default, Cassandra+ES / Postgres / MySQL variants | `CONTRIBUTING.md:124-159` |
| Cross-repo proto workflow | Step-by-step submodule + go.mod replace recipe for api/api-go/sdk-go | `CONTRIBUTING.md:202-243` |
| Commit message policy | Chris Beams convention enforced via Overcommit; PR title rules | `CONTRIBUTING.md:245-254` |
| README contributor entrypoints | Links to proposals repo, architecture docs, CONTRIBUTING, testing doc | `README.md:66-76` |
| Feature proposal process | New features routed to external `temporalio/proposals` repo | `README.md:72` |
| Docs index | `docs/README.md` partitions architecture / development / admin audiences | `docs/README.md:6-20` |
| Architecture docs corpus | 15 architecture pages incl. history-service, matching, workflow-lifecycle, chasm, nexus | `docs/architecture/README.md:75-82`, `docs/architecture/chasm.md:1-373` |
| Adding new RPCs guide | 7-step checklist touching protos, metric defs, client gen, quotas | `docs/development/new-rpcs.md:1-16` |
| Testing best practices | require-vs-assert, await helpers, parallelization, testvars, historyrequire DSL | `docs/development/testing.md:36-279` |
| Test-doc enforcement | golangci deprecation rule points violators to the testing doc | `.github/.golangci.yml:48` |
| Tracing tutorial | OTEL setup env vars, collector config, TraceQL debugging | `docs/development/tracing.md:44-103` |
| Extension tutorial (archiver) | Full add-an-archiver walkthrough: layout, interfaces, provider, YAML, FAQ | `common/archiver/README.md:13-84` |
| External-injection extension path | `WithCustomHistoryArchiverFactory` registration + `customStores` config example | `common/archiver/README.md:86-195` |
| Dynamic config format doc | Key/value/constraint YAML schema for `development-*.yaml` overrides | `config/dynamicconfig/README.md:1-40` |
| Test-server library docs | `temporaltest` package doc + compat policy; `TestServer` godoc | `temporaltest/README.md:1-7`, `temporaltest/server.go:11-33` |
| Test harness usage example | `NewServer` constructor and `NewWorker` registration helpers as runnable usage surface | `temporaltest/server.go:132`, `temporaltest/server.go:37-40` |
| Effect-package design doc | README stub routing to architecture page (pattern of colocated module docs) | `common/effect/README.md:1`, `docs/architecture/effect-package.md:1-33` |
| Module design doc | Workflow update subsystem README routes to architecture spec | `service/history/workflow/update/README.md:1` |
| Diagram-as-code pipeline | d2 sources regenerate SVGs via `docs/Makefile` targets | `docs/Makefile:5-21`, `docs/_assets/chasm-engine.d2:2` |
| AI-agent contributor guide | AGENTS.md: structure map, commands, testing rules, error-handling severity policy | `AGENTS.md:25-104` |
| AI review rubric | copilot-instructions.md: naming, testify pitfalls, concurrency safety, severity schema | `.github/copilot-instructions.md:16-40`, `.github/copilot-instructions.md:120-129` |
| AI review skill | Claude review skill reuses the shared review guidelines | `.claude/skills/review/SKILL.md:1-8` |
| AI review CI gate | Opt-in Claude PR review with org-membership/write-permission eligibility checks | `.github/workflows/claude-review-teams.yml:17-19`, `.github/workflows/claude-review-teams.yml:110-161` |
| PR template | Mandatory "What changed / Why / How did you test it" checklist incl. risk section | `.github/PULL_REQUEST_TEMPLATE.md:1-15` |
| Ownership | CODEOWNERS routes `/chasm/*` to oss-foundations, everything else to server/cgs/nexus teams | `.github/CODEOWNERS:5-10` |
| Proto/API linting (not docs) | buf lint + api-linter wired into `make proto` and `make lint-api` | `Makefile:27`, `Makefile:430-437` |
| No generated API docs | No godoc/protoc-gen-doc target anywhere in build; searched Makefile + docs | `Makefile:1-783` (target list), grep over `docs/` returned none |
| No in-repo examples dir | `find -type d -iname "*example*"` returns nothing; samples delegated to SDK repos | `README.md:46-47`, `CONTRIBUTING.md:167` |
| Explicit docs gap marker | Nexus Operation HSM framework documented as "(docs TBD)" | `docs/architecture/nexus.md:243` |

## Answers to Dimension Questions

**1. Are contribution guides clear?**
Yes, unusually so. `CONTRIBUTING.md` is operational rather than aspirational: exact commands for each persistence flavor (`CONTRIBUTING.md:136-159`), a single-test invocation pattern with real arguments (`CONTRIBUTING.md:106-116`), and the hardest contributor task — iterating against unreleased proto changes across three repos — is spelled out step-by-step with copy-pasteable `go.mod` replace blocks (`CONTRIBUTING.md:209-243`). Clarity is reinforced mechanically: the testing guide's rules are mirrored in lint enforcement (`.github/.golangci.yml:48` blocks deprecated patterns with a pointer back to `docs/development/testing.md`), and the PR template forces a test-plan disclosure (`.github/PULL_REQUEST_TEMPLATE.md:7-12`). Weakness: the guide assumes docker and Go fluency, and Windows users are handed off to WSL2 with one sentence (`CONTRIBUTING.md:41-43`).

**2. Are examples comprehensive?**
Coverage is split-brained. For SDK users (the majority audience per `README.md:64`), examples are comprehensive but external: six language sample repos plus helloworld links are listed at `CONTRIBUTING.md:167`. For server-extension contributors, there is no `examples/` directory at all (verified by filesystem search); the closest analogs are the archiver walkthrough's inline code (`common/archiver/README.md:131-160`) and living-reference implementations such as the CHASM scheduler library (`chasm/lib/scheduler/`) plus test scaffolding like `chasm/test_library_test.go:23-57`. Coverage by extension type is uneven: archivers (excellent, `common/archiver/README.md:13-84`), new RPCs (good checklist, `docs/development/new-rpcs.md:6-15`), dynamic config (format-only, `config/dynamicconfig/README.md:13-40`), CHASM components (conceptual only — `docs/architecture/chasm.md:28-54` defines Registry/Library/Component/Tasks but never shows an end-to-end "write your first component" sequence).

**3. Is API documentation available?**
Not generated from this repo. The Makefile exposes `lint-api`/`lint-protos` using buf and the API linter (`Makefile:430-437`) with configs at `proto/api-linter.yaml:1` and `proto/buf.work.yaml:1`, and the copilot review rubric requires proto field comments (`.github/copilot-instructions.md:82-83`), which keeps source annotations healthy — but there is no godoc, protoc-gen-doc, or swagger publication target anywhere in the build (searched all 100+ Makefile targets). Public API reference is implicitly delegated to docs.temporal.io and pkg.go.dev (linked from `README.md:16`). In-repo Go doc comments are good where it counts (e.g., `temporaltest/server.go:22-33`, `chasm/library.go:47-51`), so `go doc` works locally, but that is incidental, not a maintained doc pipeline.

**4. Are there tutorials for key tasks?**
Yes for several key tasks: writing tests against the full helper suite (`docs/development/testing.md:79-279` — parallelsuite, testvars, testcontext, taskpoller, testhooks, historyrequire event DSL), setting up distributed tracing (`docs/development/tracing.md:44-103`), adding RPCs (`docs/development/new-rpcs.md`), and adding archivers end-to-end (`common/archiver/README.md`). Gaps: no tutorial for authoring CHASM libraries/components beyond the architecture narrative (`docs/architecture/chasm.md:165-241` covers Engine/Context/Transition concepts, not recipes), and the Hierarchical State Machine framework behind Nexus operations is marked "docs TBD" (`docs/architecture/nexus.md:243`). There are no template/starter repositories referenced from this repo (grep over `README.md`, `CONTRIBUTING.md`, `docs/` found nothing); scaffold-style starting points are limited to test fixtures like `chasm/lib/tests/gen/testspb/v1/`.

## Architectural Decisions

- **Docs follow code ownership boundaries, not a monolithic manual.** Each subsystem ships its own design doc adjacent to its code (`service/history/workflow/update/README.md:1` → `docs/architecture/workflow-update.md`; `common/effect/README.md:1` → `docs/architecture/effect-package.md`), keeping specs near the engineers who change them.
- **Diagrams are build artifacts.** All architecture SVGs are regenerated from d2 sources via `make` in `docs/` (`docs/Makefile:5-21`; each `.d2` file states this, e.g., `docs/_assets/chasm-engine.d2:2`), preventing stale-image drift.
- **Extension surface is contract-first with two sanctioned modes.** The archiver guide codifies built-in vs. externally-injected implementations and actively steers contributors away from in-tree additions to reduce maintainer burden (`common/archiver/README.md:7-9`) — an explicit scalability decision about contribution intake.
- **Documentation doubles as enforcement.** Testing doctrine is written once (`docs/development/testing.md:36-77`) and then wired into tooling: deprecation lint (`.github/.golangci.yml:48`), an auto-parallelizer (`Makefile` `parallelize-tests` target backed by `tools/parallelize/parallelize.go:160`), and a flake-bisecting tool that even deprioritizes docs-only commits (`tools/flakereport/bisect.go:249-251`).
- **AI agents are first-class contributors.** The repo maintains parallel instruction surfaces for humans (`CONTRIBUTING.md`), generic coding agents (`AGENTS.md:49-54` commands, `AGENTS.md:56-70` practices), and automated reviewers (`.github/copilot-instructions.md` consumed by `.claude/skills/review/SKILL.md:8` and executed by CI at `.github/workflows/claude-review-teams.yml:234-267`), with security gating before any agent sees fork PR content (`.github/workflows/claude-review-teams.yml:2-6`).

## Notable Patterns

- **Audience-partitioned doc index**: `docs/README.md:8-20` explicitly routes server developers vs. operators vs. workflow authors, reducing wrong-audience friction.
- **Checklist-as-tutorial**: `docs/development/new-rpcs.md:6-15` enumerates exactly seven files/systems to touch; the same numbered-step style recurs in `common/archiver/README.md:13-84`, making cross-repo change impact legible.
- **Living examples in test form**: `temporaltest/server_test.go` and `chasm/test_library_test.go:23` act as executable documentation where prose tutorials are absent.
- **FAQ-encoded failure wisdom**: the archiver guide answers retry-progress recording and non-retryable error signaling with code snippets (`common/archiver/README.md:199-250`), transferring operational knowledge without requiring Slack questions.
- **Review-severity taxonomy shared across human and AI reviewers**: `nit/small/med/high` semantics defined once (`.github/copilot-instructions.md:120-125`) keep feedback consistent regardless of reviewer type.

## Tradeoffs

- **External delegation vs. single-source truth**: pushing samples and API references to separate repos (`README.md:46`, `docs/README.md:20`) keeps the server lean but means this snapshot cannot be studied (or versioned) as one unit; a contributor following `CONTRIBUTING.md:204-205` must track three additional repos mid-feature.
- **Prose-plus-lint vs. pure prose**: enforcing testing doctrine through golangci rules (`.github/.golangci.yml:48`) guarantees compliance but makes the docs brittle to tool renames and adds onboarding surprise when valid-looking code fails lint.
- **Contract-first extension guides vs. runnable starters**: the archiver README is thorough yet still requires assembling registration/config by hand (`common/archiver/README.md:149-193`); a starter template would cut time-to-green further but would itself need maintenance.
- **AI-review breadth vs. noise control**: gating Claude reviews to opted-in teams/labels (`REVIEW_TEAMS` at `.github/workflows/claude-review-teams.yml:17`) bounds cost and noise at the price of uneven coverage for outside contributors.

## Failure Modes / Edge Cases

- **Doc rot in fast-moving internals**: `docs/development/new-rpcs.md:11` admits its own staleness ("this is going away soon") regarding metric scope defs — a checklist contributor may follow a deprecated step.
- **Undocumented frameworks invite divergence**: with the Nexus operation HSM marked "docs TBD" (`docs/architecture/nexus.md:243`) and CHASM lacking a component-authoring tutorial, new extension authors will pattern-match against production code (e.g., `chasm/lib/scheduler/`), risking inconsistent designs going unnoticed until review.
- **Single-maintainer-risk in ownership routing**: CODEOWNERS sends every file outside `/chasm/*` to three broad teams (`.github/CODEOWNERS:5`), so review latency can silently concentrate.
- **AI-agent instruction drift**: `AGENTS.md` duplicates the project-structure map and commands that also live in the Makefile and CONTRIBUTING; if targets are renamed (e.g., `make lint-code` at `AGENTS.md:50` vs. definition at `Makefile:406`), agent guidance decays independently of human docs.
- **Windows/macOS edge cases**: non-Linux development is covered by one WSL2 sentence (`CONTRIBUTING.md:41-43`) plus per-database host-run docs (`docs/development/run-dependencies-host.md:5`); contributors off the happy path may burn setup time before reaching code.

## Future Considerations

- Add a "write your first CHASM component/library" walkthrough mirroring `common/archiver/README.md`, anchored to `chasm/lib/tests/gen/` fixtures and `chasm/test_library_test.go:23`.
- Generate publishable API/proto reference in-CI (e.g., protoc-gen-doc) next to the existing `lint-api` stage (`Makefile:430-432`), since field-comment discipline is already mandated (`.github/copilot-instructions.md:82`).
- Resolve the "docs TBD" markers (`docs/architecture/nexus.md:243`) or link them to tracking issues so gaps are visible to contributors at merge time.
- Provide a minimal starter template for external archivers/persistence plugins to complement the injection options documented at `common/archiver/README.md:149-160`.
- Keep `AGENTS.md`, `CONTRIBUTING.md`, and Makefile target names in sync via a lint check, given the triple-maintenance burden observed across `AGENTS.md:49-54` and `Makefile`.

## Questions / Gaps

- No evidence found in-repo for template/starter repositories; searched `README.md`, `CONTRIBUTING.md`, all of `docs/`, and directory names matching `*template*`/`*starter*`/`*example*`. Any templates presumably live in sibling temporalio repos outside this study's isolation boundary.
- No evidence of generated API documentation within this source (no godoc/protoc-gen-doc/swagger build target); assessment of the external docs.temporal.io quality was out of scope per source-isolation rules.
- Whether the `temporalio/proposals` process (linked at `README.md:72`) effectively gates features could not be evaluated from inside this repo.
- Tutorial adequacy for adding new persistence backends could not be assessed: no dedicated guide was found (only the archiver and RPC checklists), suggesting this extension type is either intentionally closed or undocumented; the search boundary was `docs/`, package READMEs, and the Makefile target list.

---

Generated by `22.03-docs-examples-and-contributor-workflow` against `temporal`.
