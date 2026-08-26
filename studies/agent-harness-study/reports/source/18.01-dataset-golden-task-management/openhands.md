# Source Analysis: openhands

## Dimension 18.01: Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | openhands |
| Path | `studies/agent-harness-study/sources/openhands` |
| Language / Stack | TypeScript / React (Vite, Playwright, Vitest, MSW); Python used only for test-support servers |
| Analyzed | 2026-08-26 |

## Summary

This source is the **OpenHands frontend ("agent-canvas")**, explicitly scoped in its own repository map as "only the agent-canvas frontend" of a multi-repo system whose backend/SDK work lives in sibling repos (AGENTS.md, "Repository Map — what belongs where"). Consequently, there is **no benchmark or evaluation dataset infrastructure in this repository**: searches for `benchmark`, `golden`, `dataset`, `SWE-bench`, `GAIA`, and `difficulty` return zero implementation hits. The only occurrences of these concepts are (a) a PR-review policy that forbids auto-approving changes which "could plausibly affect benchmark/evaluation performance" (`.agents/skills/custom-codereview-guide.md:18-27`), and (b) a user-facing i18n string "Contribute to public dataset" (`src/i18n/translation.json:8690`).

What *does* exist is a well-engineered **deterministic test-fixture layer** that is the closest analog to golden-task management:

1. A **scripted LLM trajectory server** (`tests/e2e/mock-llm/scripts/mock-llm-server.py`) built on `openhands.sdk.testing.TestLLM`, serving canned tool-call/text turns as OpenAI-compatible completions, with an admin API to register/activate/reset named trajectories per test.
2. **Golden output tokens** — exact sentinel strings (`MOCK_LLM_E2E_BASH_OK`, `MOCK_LLM_E2E_REPLY_OK`, `LIVE_AGENT_CANVAS_E2E_OK`, …) that both the scripted server and the real-LLM smoke test must emit, asserted end-to-end through UI and events APIs.
3. **Test-selection metadata** mapping source paths to E2E test directories (`tests/e2e/mock-llm/test-mapping.json`).
4. **MSW mock API data** and demo fixtures with deterministic timestamps (`src/mocks/`, `src/fixtures/`).

Datasets are **not versioned** (no schema/version identifiers beyond git history), there is **no task metadata schema** (no difficulty/category fields anywhere), and **no benchmark scoring/aggregation exists** — the nearest thing is a single live-E2E check that a `critic_result.score` event arrives as a finite number. Reproducibility engineering for the *test* layer is strong (serial execution, state isolation, pinned toolchains, scripted responses), but for *benchmarks* the question is moot: none exist here.

## Rating

**Score: 3 / 10**

Rationale against the rubric:

- **Eval datasets: absent.** No dataset files, loaders, IDs, or formats exist in this repo (see Questions/Gaps below for what was searched). This alone anchors the score in the 1–3 band.
- **Versioning: absent.** Trajectories are ephemeral in-memory objects registered per test run and cleared on reset (`tests/e2e/mock-llm/scripts/mock-llm-server.py:102-112`); mapping/config metadata carries no version field (`tests/e2e/mock-llm/test-mapping.json:1-4`). Only implicit git-based versioning applies.
- **Expected outputs:** present but narrow — golden tokens for E2E flows, not golden answers for tasks at scale (`tests/e2e/live/utils/agent-server-conversation.ts:12-14`).
- **Reproducibility:** excellent for the deterministic replay harness (would rate 7–8 under a test-infrastructure dimension), irrelevant for benchmarks, which do not exist.

The score reflects the dimension as defined (benchmark dataset management), not the quality of the adjacent test-fixture engineering, which is deliberately noted as a strength in Notable Patterns.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Repo scope (no eval suite here) | Repository map states this repo owns only the React/TS frontend; SDK/server/evaluation belong to sibling repos | AGENTS.md:22 ("Repository Map — what belongs where") |
| Eval/benchmark awareness without a harness | Review policy: never APPROVE PRs affecting "benchmark/evaluation performance"; flag for human "after running lightweight evals" | .agents/skills/custom-codereview-guide.md:18-27 |
| Mock LLM server (trajectory engine) | `TestLLM.from_messages(build_trajectory())` serves scripted turns over `/v1/chat/completions` | tests/e2e/mock-llm/scripts/mock-llm-server.py:47-72, 384-390 |
| Default golden trajectory | Turn 1 = terminal tool call printing `BASH_TOKEN`; turn 2 = text reply `REPLY_TOKEN` | tests/e2e/mock-llm/scripts/mock-llm-server.py:33-34, 53-72 |
| Named-trajectory registry (ephemeral dataset store) | In-memory `_named_trajectories` dict; cleared by `/admin/reset`; register/activate endpoints | tests/e2e/mock-llm/scripts/mock-llm-server.py:79, 102-112, 115-153 |
| Request capture for assertions | `_completion_requests` stores every completion body since last reset; read via `GET /admin/requests` | tests/e2e/mock-llm/scripts/mock-llm-server.py:81-93, 165-168 |
| TS-side golden token duplication | `BASH_TOKEN`/`REPLY_TOKEN` redeclared in helpers "must match mock-llm-server.py" | tests/e2e/mock-llm/utils/mock-llm-helpers.ts:11-17 |
| Live-E2E golden outputs | `EXPECTED_BASH_OUTPUT_TOKEN`, `EXPECTED_BASH_COMMAND`, `EXPECTED_REPLY_TOKEN` constants | tests/e2e/live/utils/agent-server-conversation.ts:12-14 |
| Prompt-as-golden-task | Live spec instructs the model to run the exact printf command and reply with the exact token | tests/e2e/live/real-agent-server-conversation.spec.ts:113-121, 165-173 |
| Golden-output assertion (UI + API) | `waitForSuccessfulBashObservation` polls events for exit_code 0 + token in stdout | tests/e2e/mock-llm/utils/mock-llm-helpers.ts:238-279; tests/e2e/live/utils/agent-server-conversation.ts:567-600, 616-646 |
| Critic score check (nearest eval-score analog) | `hasCriticResult` requires `critic_result.score` to be a finite number | tests/e2e/live/utils/agent-server-conversation.ts:602-614, 648-678 |
| Critic configuration | `critic_enabled`, `critic_mode: "finish_and_message"` verification settings | tests/e2e/live/utils/agent-server-conversation.ts:198-218 |
| Test-selection metadata | `test-mapping.json`: `mappings`, `alwaysRun: ["regressions"]`, `runAllSources`; `$schema` key holds prose, no version | tests/e2e/mock-llm/test-mapping.json:1-4, 6-104, 106, 108-141 |
| Mapping consumer | `resolve-affected-tests.mjs` reads sibling `test-mapping.json` and emits Playwright paths | tests/e2e/mock-llm/scripts/resolve-affected-tests.mjs:24, 33, 70 |
| Deterministic fixture timestamps | Demo conversation pins `BASE_TIME = Date.UTC(2026, 6, 29, …)` | src/fixtures/canvas-demo-conversation.ts:38-40, 15-16 |
| Mock API dataset exports | `MOCK_DEFAULT_USER_SETTINGS` exported and reused via `structuredClone` across handlers | src/mocks/settings-handlers.ts:447, 478, 515 |
| Mock-data maintenance rule | "Keep mocks close to real API contracts — update mocks when backend changes" | \_\_tests\_\_/MSW.md:129-134 |
| Reproduction commands | `test:e2e:mock-llm`, `test:e2e:mock-llm:docker`, `test:e2e:live` scripts | package.json:86-88 |
| CI reproducibility pins | Node pinned to `24.15`, `npm ci`, venv-launched mock server, explicit pre-build for caching | .github/workflows/mock-llm-e2e.yml:79, 83, 132, 154 |
| Dependency reproducibility | Exact-pinned dependencies (no ranges) plus committed `package-lock.json` | package.json:22-175; AGENTS.md ("Direct `dependencies`… are exact-pinned") |
| State isolation between runs | `OH_CANVAS_SAFE_STATE_DIR` set to `.tmp/mock-llm-state`; automation DB outside STATE_DIR; cleaned per run | playwright.mock-llm.config.ts:48, 137; AGENTS.md "Mock-LLM E2E Test Framework" |
| Acknowledged non-determinism (live) | temperature set to 0, yet AGENTS.md notes LLM nondeterminism and permits one CI retry for live E2E | tests/e2e/live/utils/agent-server-conversation.ts:184-196; AGENTS.md "Live End-to-End Test Framework" |
| Stable requirement-spec IDs (metadata discipline, non-task) | "Spec IDs are stable — never renumber"; `@spec BM-001` tags greppable in code/tests | AGENTS.md:677; specs/backend-management.md:3-13 |

## Answers to Dimension Questions

**1. How are datasets managed?**
There are no evaluation datasets in this source. The functional equivalent is managed **as code**: scripted LLM trajectories are either hardcoded in the mock server's default builder (`build_trajectory()`, tests/e2e/mock-llm/scripts/mock-llm-server.py:47-72) or declared inline inside individual Playwright specs via `registerTrajectory(request, name, turns)` calls (tests/e2e/mock-llm/utils/mock-llm-helpers.ts:835-858; usage e.g. tests/e2e/mock-llm/settings/mock-llm-model-switch.spec.ts:92-120). Registered trajectories live only in server memory (`MockLLMHandler._named_trajectories`, tests/e2e/mock-llm/scripts/mock-llm-server.py:79) and are wiped on `/admin/reset` (:102-112) and in each spec's `afterEach`. Mock REST data is likewise code-defined in `src/mocks/*-handlers.ts` with exported seed objects such as `MOCK_DEFAULT_USER_SETTINGS` (src/mocks/settings-handlers.ts:447). There is no external storage, no dataset registry, and no download/pinning mechanism.

**2. Are datasets versioned?**
No explicit versioning. Evidence: `test-mapping.json` uses its `$schema` key for a prose description rather than any identifier (tests/e2e/mock-llm/test-mapping.json:2); trajectory objects carry only a caller-chosen `name` (tests/e2e/mock-llm/utils/mock-llm-helpers.ts:848-857); no `version`/`revision` fields appear in any fixture, handler, or trajectory structure (searched `version` within tests/e2e and src/mocks; all hits are dependency/app versions). The only version control is implicit: because trajectories and mocks are plain source files, any historical snapshot is recoverable from git — but there are no immutable dataset identifiers, checksums, or changelogs that would let someone cite "trajectory dataset vX" in a result.

**3. Are expected outputs defined?**
Yes, in a narrow sense. Golden **sentinel tokens** define the expected observable output of every scripted scenario: `BASH_TOKEN`/`REPLY_TOKEN` shared between the Python server and TypeScript helpers (tests/e2e/mock-llm/scripts/mock-llm-server.py:33-34; tests/e2e/mock-llm/utils/mock-llm-helpers.ts:12-13), `IMAGE_REPLY_TOKEN` plus a canonical minimal-PNG input fixture (:17-26), `ACP_REPLY_TOKEN` for the mock ACP agent (:960), and the live-test trio `EXPECTED_BASH_OUTPUT_TOKEN`/`EXPECTED_BASH_COMMAND`/`EXPECTED_REPLY_TOKEN` (tests/e2e/live/utils/agent-server-conversation.ts:12-14). Assertions verify these tokens through both the rendered UI and the backend events API (tests/e2e/mock-llm/utils/mock-llm-helpers.ts:198-229, 238-279; tests/e2e/live/utils/agent-server-conversation.ts:594-599). What is absent: persisted golden outputs for richer agent behavior (full transcripts, artifacts), and golden-answer files decoupled from test code. The single Vitest snapshot in the repo covers transcript export formatting, not agent behavior (src/utils/transcript-export/\_\_snapshots\_\_/index.test.ts.snap).

**4. Are benchmarks reproducible?**
Not applicable — no benchmarks or score aggregation exist in this repo. For the underlying machinery the answer splits in two:
- **Mock-LLM E2E replays: highly reproducible.** Responses come from a scripted `TestLLM` rather than a model (tests/e2e/mock-llm/scripts/mock-llm-server.py:31, 171); tests run serially with per-suite resets; state dirs are isolated and cleaned (`.tmp/mock-llm-state`, `.tmp/automation/`); session keys are random-per-run so nothing persists; CI pins Node `24.15` and uses `npm ci` with an explicit cached build (.github/workflows/mock-llm-e2e.yml:79, 83, 154). Given a commit, re-running `npm run test:e2e:mock-llm` (package.json:87) reproduces the same pass/fail set modulo timing.
- **Live LLM smoke test: semi-reproducible by design.** It fixes temperature 0 and a pinned default model (`openhands/claude-haiku-4-5-20251001`, tests/e2e/live/utils/agent-server-conversation.ts:39-45, 190), constrains the task to one exact command and one exact reply token (:12-14, 113-121), but AGENTS.md concedes "LLM behavior is not perfectly deterministic even at temperature 0" and allows one CI retry — six months later the same test may flip on provider/model drift, and results carry no stored baseline to compare against.

## Architectural Decisions

- **Trajectories-as-code instead of dataset artifacts.** Golden scenarios are expressed as JSON turn descriptors inline in specs and converted to SDK `Message` objects at runtime (`_parse_trajectory_turns`, tests/e2e/mock-llm/scripts/mock-llm-server.py:341-381). This keeps scenarios reviewable and diffable in PRs but forecloses versioning, reuse across repos, and independent citation.
- **Admin-API lifecycle over static files.** The mock server exposes `register`/`activate`/`reset`/`requests` endpoints (:102-159) so each spec controls its own scenario imperatively; reset-to-default in `afterEach` guarantees cross-spec isolation (helpers at tests/e2e/mock-llm/utils/mock-llm-helpers.ts:883-886).
- **Golden tokens as the universal oracle.** Rather than diffing full outputs, every flow funnels toward "did the exact sentinel string appear," which makes assertions robust to cosmetic drift in both mocked and real LLM modes (tests/e2e/live/real-agent-server-conversation.spec.ts:117-118, 127).
- **Change-driven test selection.** `test-mapping.json` + `resolve-affected-tests.mjs` map changed source globs to test directories, with `runAllSources` for cross-cutting files and `alwaysRun` regressions (tests/e2e/mock-llm/test-mapping.json:106-141) — a categorization scheme for tests, though not for tasks.
- **Governance hook acknowledging upstream benchmarks.** The AI reviewer policy blocks auto-approval for changes plausibly affecting eval performance and demands human sign-off "after running lightweight evals" (.agents/skills/custom-codereview-guide.md:20-27) — an implicit admission that benchmark impact matters even though no local harness can measure it.

## Notable Patterns

- **Dual-layer token contract.** Sentinel strings are declared once in Python and mirrored verbatim in TypeScript with an explicit "must match mock-llm-server.py" comment (tests/e2e/mock-llm/utils/mock-llm-helpers.ts:11-13) — a deliberate, documented duplication that trades DRY for stack independence.
- **Request-capture introspection.** The mock server records every completion body since the last reset and serves them back (`GET /admin/requests`, tests/e2e/mock-llm/scripts/mock-llm-server.py:81-93), enabling white-box assertions like verifying an uploaded image actually reached the LLM payload.
- **Deterministic time in fixtures.** Demo conversations pin `BASE_TIME` to a fixed UTC instant so ordering/rendering is stable across runs (src/fixtures/canvas-demo-conversation.ts:38-40).
- **Padding-turn convention.** Tests prepend a throwaway `{ text: "" }` response to absorb the agent-server's internal condenser/skill-analysis call before the main loop — documented tribal knowledge about consuming exactly one trajectory entry (AGENTS.md "Mock-LLM E2E Test Framework"; applied in tests/e2e/mock-llm/settings/mock-llm-model-switch.spec.ts:117-120).
- **Stable-ID discipline for specs, not tasks.** Requirement specs use permanent, greppable `@spec XXX-NNN` tags linked from implementation and tests (AGENTS.md:677; specs/backend-management.md:3-13) — the repo clearly understands stable-identifier value, it just hasn't applied it to datasets.

## Tradeoffs

- **Ephemeral trajectories vs. durable corpora.** Registering scenarios at runtime avoids managing binary/large fixture files and keeps each test self-contained, but means there is no corpus to grow, measure against, or share with the sibling SDK repos where the actual agent behavior lives.
- **Token-oracle simplicity vs. evaluation depth.** Exact-sentinel matching survives model verbosity drift and works identically for mock and live runs, yet it can only assert presence of a string — it cannot grade quality; the lone `critic_result.score` check verifies a number arrived, not whether it was good (tests/e2e/live/utils/agent-server-conversation.ts:602-614).
- **Code-defined mocks vs. contract fidelity.** Keeping mock payloads next to components maximizes editability (guidance at \_\_tests\_\_/MSW.md:129-134), but the repo itself flags the failure mode in its review rules: TypeScript-client event shapes inferred from Canvas fixtures rather than verified against the SDK schema are treated as blocking errors (.agents/skills/custom-codereview-guide.md:136-141) — precisely because hand-maintained mock data silently diverges from real APIs.

## Failure Modes / Edge Cases

- **Silent token drift between layers.** If `BASH_TOKEN` changes in `mock-llm-server.py` but not in `mock-llm-helpers.ts` (or vice versa), assertions fail confusingly; correctness relies on a comment, not a shared constant or generated artifact.
- **Trajectory exhaustion.** An unexpected extra LLM call raises `TestLLMExhaustedError` surfaced as HTTP 500 with call count (tests/e2e/mock-llm/scripts/mock-llm-server.py:172-178); the documented padding-turn workaround shows how brittle turn-count coupling is when SDK internals change (tests/e2e/mock-llm/settings/mock-llm-model-switch.spec.ts:117-120).
- **Cross-test state leakage.** Serial execution against a real agent-server means earlier specs' settings persist; the maintainers document stale-profile pitfalls (e.g., FK guard preventing profile deletion, worked around by edit-in-place in tests/e2e/mock-llm/utils/mock-llm-helpers.ts:492-501) and mandate `resetMockLLM()` in `afterEach`.
- **Live-mode irreproducibility window.** Provider outages, model deprecation of the pinned `openhands/claude-haiku-4-5-20251001` default (tests/e2e/live/utils/agent-server-conversation.ts:42), or proxy changes can break the smoke test with no recorded historical result to diff against — the "six months later" question fails for live mode by construction.
- **Unversioned mapping config.** Because `test-mapping.json` has no schema/version and is matched by a hand-rolled glob-to-regex (tests/e2e/mock-llm/scripts/resolve-affected-tests.mjs:74-80), malformed entries degrade quietly into "run everything" or "run nothing" semantics depending on which list they land in.

## Future Considerations

- Promote frequently reused trajectories from inline spec literals into versioned, named JSON fixtures under `tests/e2e/mock-llm/trajectories/` loaded by `registerTrajectory`, giving scenarios stable identities without changing the runtime model.
- Add a lightweight manifest (name, purpose, owning team, last-verified commit) alongside trajectory fixtures to answer the dimension's metadata questions without adopting a full benchmark framework.
- Record live-E2E outcomes (pass/fail, duration, model id, timestamp) as CI artifacts so the smoke test's health becomes comparable over time instead of point-in-time.
- Extend the existing eval-risk review gate (.agents/skills/custom-codereview-guide.md:20-27) with a pointer to wherever "lightweight evals" actually run (presumably a sibling repo), so reviewers can act on the policy rather than merely honor it symbolically.
- Share the golden-token contract between Python and TypeScript via generation from a single source (e.g., a small JSON file emitted into both trees) to eliminate the drift-prone duplication.

## Questions / Gaps

- **Where do actual benchmarks live?** Not in this source. Searches across the selected directory for `benchmark`, `golden`, `SWE-bench`/`swebench`, `GAIA`/`gaia`, `dataset` (as eval data), `difficulty`, and `.snap` snapshots found only DOM `dataset` attributes (e.g., \_\_tests\_\_/components/features/sidebar/sidebar.test.tsx:285), the review-policy text, the i18n string, and one transcript-export UI snapshot. Per the repo's own map (AGENTS.md), evaluation of agent capability belongs to the `OpenHands/software-agent-sdk` ecosystem, which is outside this study's isolation boundary; conclusions about upstream dataset practices cannot be drawn from this source ("No evidence found" within boundary).
- **Is there any persisted record of past E2E outcomes?** No evidence found. CI uploads screenshots/videos/reports as transient PR artifacts (.github/workflows/mock-llm-e2e.yml:310-323), but no machine-readable history of scenario results is retained in-repo.
- **Do any task-level categories exist (difficulty, domain)?** No. The only categorization is feature-directory grouping of tests (tests/e2e/mock-llm/test-mapping.json:6-104); nothing annotates individual tasks or scenarios with difficulty/category fields.
- **Does the critic integration produce usable scores today?** Partially evidenced only: settings keys (`critic_enabled`, `critic_mode`) and a finite-number existence check exist (tests/e2e/live/utils/agent-server-conversation.ts:204-205, 612-614); scoring semantics, thresholds, and persistence are owned downstream and leave no trace in this repo.

---

Generated by `Dimension 18.01: Dataset and Golden Task Management` against `openhands`.
