# Source Analysis: opa

## 22.03 Docs, Examples, and Contributor Workflow

### Source Info

| Field | Value |
|-------|-------|
| Name | opa (Open Policy Agent) |
| Path | `studies/agent-harness-study/sources/opa` |
| Language / Stack | Go (with Docusaurus/React docs site, Rego policy language, WASM tooling) |
| Analyzed | 2026-08-24 |

## Summary

OPA's contributor workflow is documentation-first: the root `CONTRIBUTING.md` is a thin pointer into a dedicated, audience-segmented contribution section of the Docusaurus site (`docs/docs/contributing.md`, `contrib-code.md`, `contrib-development.md`, `contrib-docs.md`), supplemented by a purpose-built `AGENTS.md` governing AI-assisted PRs. Extension teaching material is a first-class docs area: `docs/docs/extensions.md` walks through all three Go extension surfaces (custom built-in functions, runtime plugins, custom storage backends) with complete compilable examples, and `docs/docs/contrib-adding-builtin-functions.md` is an end-to-end tutorial for adding a built-in function — declare, implement, test in YAML, auto-document, regenerate capabilities — which is OPA's analog of "add a tool." Documentation quality is operationally enforced rather than aspirational: doc examples live as data (`_examples/*/policy.rego` + `config.json`) and are executed against a real `opa eval` binary by `docs/bin/eval-examples.sh`, with drift gating via `make gen-check`; CLI docs and the built-in-function reference are generated from the binary and from Go source metadata (`go:generate` directives in `main.go`). API reference coverage is strong for Go (godoc + rich interface doc comments) and generated surfaces, but the REST API "authoritative specification" is hand-written prose with no OpenAPI/Swagger artifact in the repo — the most significant gap found. There is no in-repo starter/template scaffolding; that role is delegated to the external `open-policy-agent/contrib` repo.

## Rating

**8 / 10** — A new contributor can add a built-in function ("tool") in well under an hour by following `docs/docs/contrib-adding-builtin-functions.md`: every step names the exact file to touch (`ast/builtins.go`, `topdown/*.go`, `test/cases/testdata/v1`, `capabilities.json` via `make generate`), provides copy-pasteable code and YAML test cases, and documents the automated payoff (auto-generated reference entry). The workflow earns its upper-band score through operational safeguards: CI-verified runnable examples (`docs/bin/eval-examples.sh` + `make gen-check`), generated-with-drift-testing wire schemas, lint gates (`golangci-lint` pinned in `Makefile:23`), PR template requirements tying public API changes to docs, and explicit AI-contribution governance. It stops short of 9–10 because the REST API spec has no machine-checkable schema, plugin example code lives out-of-repo where it can drift from the documented snippets, and no template/starter repo or scaffolding command exists for bootstrapping custom OPA distributions.

## Evidence Collected

Every entry cites `path:NN` relative to the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Contribution hub routing users by intent | Hub page splits "help users" / "contribute code" / "improve docs" / "share project", links each sub-guide | `docs/docs/contributing.md:15-53` |
| Root CONTRIBUTING stub pointing to docs site | Single-purpose pointer file; real guides live under `docs/docs/` | `CONTRIBUTING.md:5` |
| Code contribution guide: tests required | "Almost all code changes should be accompanied by tests… no warnings from `make check`" | `docs/docs/contrib-code.md:11-13` |
| Public API surface discipline for contributors | Prefer unexported; share logic via `internal` package | `docs/docs/contrib-code.md:17-24` |
| Wire-format schema commitment documented | Contributors told to update `.proto` alongside Go types; drift tests named (`e2e/proto/plan_test.go`, `e2e/proto/manifest_test.go`) | `docs/docs/contrib-code.md:30-41` |
| DCO sign-off process | `git commit -s` instructions, human-author requirement for AI patches | `docs/docs/contrib-code.md:101-137` |
| AI-assisted contribution guidelines | LF rules, maintainer-time protection, ban on LLM-authored review replies | `docs/docs/contrib-code.md:193-229` |
| AI agent steering file at repo root | Commands agents can run (`go test`, `golangci-lint run --fix`), PR title format `area: $TITLE`, refusal rules | `AGENTS.md:22-46` |
| Claude Code hook injecting contributor rules on session start | SessionStart hooks cat `AGENTS.md` and `docs/docs/contrib-code.md` into context | `.claude/settings.json:1-19` |
| Developer environment guide | Requirements list, WSL/MSYS2 Windows paths, one-command `make` bootstrap, `make test-short` fast loop | `docs/docs/contrib-development.md:16-60` |
| Contributor git workflow documented | Fork/clone/branch/rebase/submission steps with commands | `docs/docs/contrib-development.md:62-123` |
| Docs contribution guide | Local Docusaurus dev server, build checks broken links, Netlify preview per PR | `docs/docs/contrib-docs.md:10-49,120-127` |
| PR template enforces test+docs pairing | Checklist: tests for code changes; "All changes to public APIs **must** be accompanied with docs" | `.github/PULL_REQUEST_TEMPLATE.md:7-15` |
| Built-in function contribution tutorial (5 steps) | Declare/register → implement → test → document → capability, with full Go/YAML listings | `docs/docs/contrib-adding-builtin-functions.md:16-23,28-101` |
| Tutorial requires YAML test cases in-repo | Positive+negative case files under `test/cases/testdata/v1`; targeted run command given | `docs/docs/contrib-adding-builtin-functions.md:103-147` |
| Docs auto-generation promised for new built-ins | "All built-in functions will automatically be documented … under an appropriate subsection" | `docs/docs/contrib-adding-builtin-functions.md:149-153` |
| Capability regeneration step | `make generate` regenerates `capabilities.json` entry for the new built-in | `docs/docs/contrib-adding-builtin-functions.md:155-187` |
| Custom built-in functions extension docs | Full worked examples incl. `rego.Function#Memoize` / `Nondeterministic` safety flags and effect-free warning | `docs/docs/extensions.md:29-96,130-140,184-188` |
| Custom plugin extension docs | Complete decision-logger plugin: `Factory.New`/`Validate`, status reporting, config file, run commands | `docs/docs/extensions.md:190-368` |
| Plugin example maintained out-of-repo | Source link to `open-policy-agent/contrib` `decision_logger_plugin_example` | `docs/docs/extensions.md:360-361` |
| Custom storage backend extension docs | `storage.Store` + `runtime.RegisterStorageBackend` + `storage.Closer` shutdown hook | `docs/docs/extensions.md:369-404` |
| Runtime-registration appendix (complete program) | Full `main()` registering built-in into the OPA runtime binary | `docs/docs/extensions.md:509-575` |
| Plugin interfaces carry usage-bearing godoc | `Factory.Validate/New` contract with numbered steps; `Plugin.Start/Stop/Reconfigure` lifecycle notes | `v1/plugins/plugins.go:42-110` |
| Runnable interactive docs examples as structured data | Per-example dirs (`admin`, `ai`, `app`, `envoy`, `k8s`) each holding `policy.rego`, `input.json`, `data.json`, `config.json`, `output.json` | `docs/src/pages/_examples/*` (5 categories) |
| Example evaluation harness executes every doc example | Script runs `opa eval -d policy.rego -i input.json <command>` per `_examples/config.json`, fails on error, writes `output.json` | `docs/bin/eval-examples.sh:12-57` |
| Drift gate for regenerated example output | `gen-check` reruns generation and fails on dirty working tree | `docs/Makefile:68-74` |
| CLI reference generated from the binary | `generate-cli-docs` target pipes `build/gen-cli-docs.sh` into `src/data/cli.json` | `docs/Makefile:28-30` |
| Built-in metadata generated from Go source | `go:generate internal/cmd/genbuiltinmetadata/main.go builtin_metadata.json` | `main.go:33` |
| Docusaurus ingests generated builtin metadata | `builtinData` plugin reads `../builtin_metadata.json` at site-build time | `docs/docusaurus.config.js:428-446` |
| Capabilities JSON generated from source | `go:generate internal/cmd/genopacapabilities/main.go capabilities.json` | `main.go:32` |
| Wire schemas generated from Go types | `go:generate genplanschema/genmanifestschema` producing `plan.schema.json`, `manifest.schema.json` | `main.go:35-36` |
| REST API reference is hand-written prose | Self-described "authoritative specification"; ~2,464 lines of Markdown | `docs/docs/rest-api.md:6` |
| No OpenAPI/Swagger artifact in repo | Search for `*swagger*`/`*openapi*` across source returned no files | (search boundary noted in Gaps) |
| Policy testing framework tutorial | Getting-started policy + `_test.rego` walkthrough; 705-line evals guide | `docs/docs/policy-testing.md:9-40` |
| Kubernetes admission-control tutorial | Deploy OPA as admission controller from scratch (minikube walkthrough) | `docs/docs/kubernetes/tutorial.md:5-66` |
| Envoy tutorials (3 variants) | Istio AuthorizationPolicy, Gloo Edge, standalone ExternalAuthorization | `docs/docs/envoy/tutorial-istio.md:10`, `docs/docs/envoy/tutorial-gloo-edge.md:10`, `docs/docs/envoy/tutorial-standalone-envoy.md:6-15` |
| Integration tutorials set | Docker, HTTP API, GraphQL, Kafka, SSH/PAM, Terraform, AWS CloudFormation Hooks, SQL filtering | `docs/docs/docker-authorization.md:14`, `docs/docs/http-api-authorization.md:9`, `docs/docs/graphql-api-authorization.md:11`, `docs/docs/kafka-authorization.md:12`, `docs/docs/ssh-and-sudo-authorization.md:11`, `docs/docs/terraform.md:34`, `docs/docs/aws-cloudformation-hooks.md:20`, `docs/docs/filtering/tutorial-sql-filtering.md:6` |
| Curated tutorial index for newcomers | "Popular tutorials": Kubernetes, Envoy, Terraform | `docs/docs/index.md:1263-1267` |
| Rego Playground as zero-install start | Playground + access-control example group linked from README | `README.md:11` |
| Policy authoring style guide (imported, linter-backed) | Style guide sourced from external canonical repo; points to Regal linter enforcement | `docs/docs/style-guide.md:5-17` |
| Ecosystem listing as contribution path | Add project via markdown entry + icon files | `docs/docs/contributing.md:55-64` |
| External contrib repo doubles as starter examples | PAM module, echo server, custom bundle signing examples referenced from tutorials | `docs/docs/ssh-and-sudo-authorization.md:17`, `docs/docs/http-api-authorization.md:132`, `docs/docs/management-bundles/index.md:621` |
| No template/starter scaffolding in repo | Searches for `starter\|template repo\|cookiecutter\|scaffold` in `docs/docs/**.md` returned no matches | (search boundary noted in Gaps) |
| Lint configuration pinned for contributors | `GOLANGCI_LINT_VERSION := v2.13.0`; `.golangci.yaml` at root | `Makefile:23`, `.golangci.yaml:1` |
| e2e module isolated from core deps | Separate Go module so heavy deps never bloat the library | `e2e/README.md:1-6` |

## Answers to Dimension Questions

**1. Are contribution guides clear?**
Yes — unusually so. The hub page segments audiences before dumping process (`docs/docs/contributing.md:15-77`); `contrib-code.md` covers testing expectations, export discipline, wire-format schema duties, DCO, review etiquette, vulnerability scanning, and AI tooling rules with concrete commands (`docs/docs/contrib-code.md:11-44,51-99,169-191,193-229`); `contrib-development.md` gets a working build in one command (`make`) and offers a sub-minute feedback loop (`make test-short`, `docs/docs/contrib-development.md:36-55`). The PR template hard-codes the two non-negotiables (tests; docs for public APIs) at submission time (`.github/PULL_REQUEST_TEMPLATE.md:7-15`). A distinct strength is AI-workflow governance: `AGENTS.md` plus `.claude/settings.json` hooks inject these rules into agent sessions automatically.

**2. Are examples comprehensive?**
Comprehensive across OPA's actual extension surface. All three Go extension mechanisms — built-in functions (`docs/docs/extensions.md:29-188`), plugins (`190-368`), storage backends (`369-404`) — have complete, compilable examples including safety flags (`Memoize`, `Nondeterministic`) and the "no side effects" danger callout (`184-188`). Doc examples are not static: interactive examples are stored as executable fixtures (`policy.rego`/`input.json`/`config.json`/`output.json` per category dir under `docs/src/pages/_examples/`) and re-evaluated by `docs/bin/eval-examples.sh:12-57`, with `make gen-check` failing CI on stale output (`docs/Makefile:68-74`). Coverage of *agent-harness-style* extensions specifically: tools ≈ built-ins (tutorialized), policies ≈ Rego (style guide + playground), tracing/plugins ≈ plugin docs, evals ≈ `policy-testing.md`. Not comprehensive for: there is no dedicated worked example of authoring a custom data-filter compile integration beyond the SQL tutorial's user-level view, and WASM-side custom development is documented mainly for consumers of compiled bundles rather than authors of new WASM targets.

**3. Is API documentation available?**
Yes, in three tiers. (a) Go APIs rely on pkg.go.dev godoc, with genuinely instructive doc comments on extension interfaces — `Factory`/`Plugin` contracts include config examples and numbered implementation recipes (`v1/plugins/plugins.go:42-110`) — and godoc badges/links from README (`README.md:33-35`) and integration docs. (b) Generated references: the CLI reference is extracted from the built binary into `src/data/cli.json` (`docs/Makefile:28-30`); the entire built-in function reference is generated from Go-source-derived `builtin_metadata.json` (`main.go:33` → consumed by the `builtinData` Docusaurus plugin at `docs/docusaurus.config.js:428-446`); JSON Schemas for IR plan and bundle manifest are generated and drift-tested (`main.go:35-36`, commitments described at `docs/docs/contrib-code.md:30-41`). (c) The REST API is the weak spot: `rest-api.md` declares itself the authoritative specification (`docs/docs/rest-api.md:6`) but is prose-only — no OpenAPI/Swagger document exists in the tree (verified by filename search), so request/response shape drift is guarded only by human review and e2e tests, not schema validation.

**4. Are there tutorials for key tasks?**
Yes, task-oriented and environment-complete. The flagship contributor tutorial (`contrib-adding-builtin-functions.md`) is exactly the "add a tool in under an hour" scenario: five steps, exact files, copy-paste code, YAML test cases, and automatic documentation/capability payoffs (`docs/docs/contrib-adding-builtin-functions.md:16-187`). User-facing deployment/integration tutorials cover Kubernetes admission control, three Envoy topologies, Docker, HTTP/GraphQL/Kafka/SSH authorization, Terraform plan validation, AWS CloudFormation Hooks, and SQL data filtering, each with bootstrap commands (Docker Compose/minikube) and cleanup guidance. A curated shortlist funnels newcomers (`docs/docs/index.md:1263-1267`).

## Architectural Decisions

- **Docs-as-part-of-the-monorepo, deployed independently**: website sources live in `docs/` with their own Makefile/toolchain (`docs/Makefile:1-75`) and deploy via Netlify (`netlify.toml:1-8`), while the root Makefile shims `docs-%` targets (`Makefile:195-199`). Contributors change docs in the same PR as code.
- **Generated-over-hand-written for reference material**: anything derivable from the binary or Go source (CLI flags, built-in signatures/descriptions, capabilities, wire schemas) is produced by `go:generate` (`main.go:31-36`) rather than maintained by hand, converting doc staleness into a CI failure mode.
- **Executable documentation**: interactive examples are fixtures evaluated by a script against the real engine (`docs/bin/eval-examples.sh:26-52`), making the docs test suite behavioral evidence rather than prose.
- **Delegated extensibility showcase**: complete plugin implementations are kept in the separate `open-policy-agent/contrib` repository and cross-linked (`docs/docs/extensions.md:360-361`), keeping the main repo dependency-minimal (consistent with the vendoring aversion policy at `docs/docs/contrib-code.md:25-29`).
- **Governance-aware contributor automation**: AI assistance is explicitly regulated rather than ignored — root `AGENTS.md`, AI Guidelines section in `contrib-code.md`, DCO human-signoff requirement, and editor-level enforcement via `.claude/settings.json` session hooks.

## Notable Patterns

- **Audience-routed docs hub** (`docs/docs/contributing.md:15-77`): readers self-select ("I'd like to help users / contribute code / improve docs / share a project"), preventing the common wall-of-text contributing page.
- **Tutorial-with-teeth pattern**: `contrib-adding-builtin-functions.md` doesn't just show code; it binds each step to a repo artifact (`ast/builtins.go`, `DefaultBuiltins`, YAML case dirs, `capabilities.json`) and shows the verification command for each (`go test ./topdown -v -run 'TestRego/v1/repeat'`).
- **Safety-flag education inside examples**: the GitHub-fetcher built-in example demonstrates `Memoize: true` and `Nondeterministic: true` with rationale about partial evaluation hazards (`docs/docs/extensions.md:130-140,184-188`) — extension docs double as correctness training.
- **Interface doc comments as mini-tutorials**: `Factory.Validate`/`New` godoc lists the exact 3–4 steps each method should perform (`v1/plugins/plugins.go:68-88`).
- **Fast-loop developer ergonomics**: `make test-short` documented with expected duration tradeoff (`docs/docs/contrib-development.md:52-55`).
- **Imported single-source-of-truth guides**: the Rego style guide and cheat sheet are imported from external canonical repos via scripts (`docs/bin/import-regal-docs.sh`, `import-rego-cheat-sheet.sh`), avoiding forks of upstream content.

## Tradeoffs

- **Generated docs vs. discoverability**: because references are generated at site-build time (`docusaurus.config.js:428-446`), contributors cannot grep the rendered reference in-tree without running generation; the tradeoff buys guaranteed accuracy at the cost of raw-file visibility (mitigated partially by committed artifacts like `builtin_metadata.json`, `capabilities.json` at repo root).
- **Out-of-repo examples vs. lean main module**: hosting full plugin programs in `open-policy-agent/contrib` honors minimal-dependency principles but introduces version skew risk between the in-doc snippet (`docs/docs/extensions.md:225-306`) and the maintained example it points to.
- **Prose REST spec vs. schema-first API**: a hand-written 2,464-line specification is more readable than generated OpenAPI but sacrifices client generation, diffable breaking-change detection, and machine validation.
- **Strict AI governance vs. contributor velocity**: banning LLM-generated discussion replies and requiring disclosure adds friction, traded deliberately against reviewer trust and maintainer burden (`docs/docs/contrib-code.md:209-219`).
- **Docs in monorepo vs. docs repo**: same-PR docs changes keep behavior and description atomic, at the cost of a heavyweight Node toolchain living beside a Go codebase (`docs/package.json`, `dprint.json`, `eslint.config.mjs`).

## Failure Modes / Edge Cases

- **REST API drift** is only caught by humans/e2e tests; nothing mechanically validates that documented endpoints match `server` handlers (no OpenAPI artifact exists — see search boundary in Gaps).
- **Stale interactive-example output**: mitigated structurally by `gen-check` (`docs/Makefile:68-74`), but examples can opt out entirely via `skip_output_reason` in `config.json` (`docs/bin/eval-examples.sh:18-23`), so a silently skipped example can rot without CI signal beyond the skip line.
- **Windows contributor friction** is acknowledged rather than fully solved: some tests may still fail on path separators even under MSYS2 (`docs/docs/contrib-development.md:33`).
- **v0/v1 import-path ambiguity in docs snippets**: several extension examples import v0 paths (`github.com/open-policy-agent/opa/ast`, `docs/docs/extensions.md:37-39,445-447`) while the module's primary surface is `v1/...`; compatibility shim packages exist (e.g., `topdown/doc.go:13`, `rego` package deprecation notes like `sdk/doc.go:5-7`), but a newcomer copying the appendix verbatim gets v0 semantics unless they know the compatibility story.
- **Contributor onboarding depends on external services**: Slack channels, GitHub Discussions, Netlify previews, and the playground are integral to the documented flow; outages or link rot degrade the experience (a `link-checker.yaml` workflow exists at `.github/workflows/link-checker.yaml` to counter this).
- **Capability omission hazard**: a built-in added without running `make generate` compiles and passes unit tests but is missing from `capabilities.json`; the tutorial warns, but the guard is procedural (`docs/docs/contrib-adding-builtin-functions.md:161-165`), not a test assertion cited in-repo.

## Future Considerations

- Publish an OpenAPI (or equivalent schema) artifact for the REST API, generated from server handler types and drift-tested like `plan.schema.json`, closing the largest documentation-integrity gap.
- Add a minimal in-repo scaffold (or `opa dev new-plugin` generator) producing a compiling custom-distribution skeleton, reducing reliance on the external contrib repo for first plugin projects.
- Pin interactive-example execution into the standard PR check matrix (not only local `gen-check`) if not already enforced in `pull-request.yaml`.
- Show v1 import paths by default in `extensions.md` snippets, reserving v0 forms for the compatibility page.
- Consider a CI assertion that newly registered `Builtin` symbols appear in committed `capabilities.json` to convert the procedural capability step into an automated invariant.

## Questions / Gaps

- **No OpenAPI/Swagger file found.** Searched filenames matching `*swagger*` / `*openapi*` across the source tree (excluding `.git`) — zero results. The REST API contract is therefore unverifiable by machine within this repo.
- **No template/starter repo or scaffolding found in-tree.** Searched `docs/docs/**/*.md` for `starter|template repo|cookiecutter|scaffold` — zero matches. The starter role is played exclusively by external resources (Rego Playground, `open-policy-agent/contrib`), whose current state lies outside this source's boundary and was not inspected per isolation rules.
- **CI enforcement scope of doc checks unconfirmed**: `docs/Makefile` defines `gen-check`, `lint-check`, `markdownlint-check`, `spell-check`, and `smoke-test`, but this analysis did not trace which of these run in `.github/workflows/pull-request.yaml` versus being locally optional.
- **Playground content governance** (`play.openpolicyagent.org` example groups, `README.md:11`) is external to this source and could not be assessed.

---

Generated by `dimensions/22.03-docs-examples-and-contributor-workflow` against `opa`.
