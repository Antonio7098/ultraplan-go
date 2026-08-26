# Source Analysis: openai-agents-sdk

## 18.01 Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (Pydantic, asyncio; uv-managed) — OpenAI Agents Python SDK |
| Analyzed | 2026-08-26 |

## Summary

The SDK ships **no eval framework, dataset registry, or benchmark harness** as a library feature: there is no `evals` module under `src/agents/`, no eval dependency in `pyproject.toml`, and no eval page in `mkdocs.yml`. Dataset/golden-task management exists only inside the examples tree, in three distinct flavors:

1. **Executable golden-output graders** for two sandbox tutorials. `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:47-210` hardcodes a golden table of 42 expected `(source_file, metric, fiscal_period, segment) → (value, unit)` rows and validates an agent-produced JSONL/CSV artifact against it with exact-match assertions (`evals.py:240-302`). `examples/sandbox/tutorials/repo_code_review/evals.py:7-10` pins the two expected finding file paths and asserts comment content keywords (`evals.py:21-50`) plus patch scope (`evals.py:53-60`).
2. **Schema-defined but unconsumed task metadata** in the healthcare support demo. Six scenario JSON files carry a golden contract — `expected.intent`, `required_entities`, `required_tool_calls`, `required_resolution_elements`, and a `gold.expected_next_step` (e.g., `examples/sandbox/healthcare_support/data/scenarios/messy_ambiguous_knee_case.json`) typed by `ScenarioExpectation` / `ScenarioCase` (`examples/sandbox/healthcare_support/models.py:16-31`). Grep over the example's code shows `.expected` and `.gold` are never read by any grader or assertion; only transcript/prompt fields are consumed (`examples/sandbox/healthcare_support/workflow.py:391-398`). The golden data is documentation-grade, not executable.
3. **A versioned golden API-surface contract** (not a task dataset): `tests/fixtures/released_api_contract.json:2-3` records `"baseline": "v0.22.0"` and `"baseline_commit"`, enforced by `tests/test_released_api_contract.py:37`.

Reproducibility is partially engineered: input corpora are deterministic (generated fixtures, a pinned git SHA), and graders are pure functions of artifacts. But runs depend on live model nondeterminism, no reference results are stored, the Docker image is pinned only by a mutable local tag, and nothing in CI executes these evals.

## Rating

**4 / 10 — Present but inconsistent, weakly documented, and fragile.**

Rationale against the rubric:

- Positive: two tutorials have real, deterministic, runnable golden-answer checks with explicit failure messages (`dataroom_metric_extract/evals.py:265-300`; `repo_code_review/evals.py:21-60`), a typed task-metadata schema (`models.py:16-31`), and a documented difficulty ladder (`examples/sandbox/healthcare_support/README.md`, "built-in scenarios increase in complexity").
- Negative: datasets have **no version identifiers** anywhere (scenario JSON, fixture generator, and golden tables carry no version field); half of the golden metadata (`expected`/`gold`) is dead weight never executed; golden answers are duplicated by hand between fixture text (`data/dataroom/setup.py` writes "$1,284 million") and the golden table (`evals.py:48`) with no cross-check; no stored baseline results, no CI integration (`.github/workflows/` contains only docs/issues/publish/release-tag/tests), and the sandbox image pin is a mutable tag (`misc.py:56`, `sandbox-tutorials:latest`).
- The one mature golden artifact — the released-API contract with baseline commit — covers API shape, not agent task quality, so it cannot lift the score above "fragile."

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| No first-class eval package | Only `_evaluate_policy` / `_evaluate_retry` (retry logic, not evals) exist in `src/agents`; no eval/dataset symbols in the library | `src/agents/retry.py:239`, `src/agents/run_internal/model_retry.py:389` |
| Eval script: dataroom metric extract | Golden source→section metadata map | `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:20-45` |
| Eval script: dataroom metric extract | Golden answer table: 42 keyed `(value, unit)` rows | `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:47-210` |
| Expected-output validation | Duplicate detection, exact row count, section check, missing/unexpected row assertions | `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:242-300` |
| Artifact loaders (JSONL + CSV) | `load_metrics` parses both formats into `FinancialMetricBatch` | `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:218-237` |
| Eval entrypoint | CLI with default artifact path `output/financial_metrics.jsonl` | `examples/sandbox/tutorials/dataroom_metric_extract/evals.py:305-315` |
| Eval script: repo code review | `EXPECTED_FINDING_PATHS` golden set of exactly 2 files | `examples/sandbox/tutorials/repo_code_review/evals.py:7-10` |
| Expected-output validation | Finding-count, path-set, keyword-content, and patch-scope assertions | `examples/sandbox/tutorials/repo_code_review/evals.py:21-60` |
| Task metadata schema | `IntentName` Literal taxonomy (5 intents) | `examples/sandbox/healthcare_support/models.py:7-13` |
| Task metadata schema | `ScenarioExpectation` (intent, required_entities, required_tool_calls, required_resolution_elements, expected_payer) and `ScenarioCase.gold` | `examples/sandbox/healthcare_support/models.py:16-31` |
| Scenario dataset on disk | 6 scenario JSON files with transcripts, followup Q/A, expectations | `examples/sandbox/healthcare_support/data/scenarios/*.json` (e.g., `messy_ambiguous_knee_case.json`) |
| Golden expectation unused | `grep -rn "\.expected\|\.gold"` over the example's Python returns only model definitions — no grader consumes them | `examples/sandbox/healthcare_support/models.py:19-21,30` (sole hits) |
| Dataset loader | Scenarios loaded from disk at runtime via glob, keyed by stem; fixtures from 3 JSON files; policies from markdown | `examples/sandbox/healthcare_support/data.py:14-16,72-101` |
| Difficulty/category metadata | Prose-only ordering: "built-in scenarios increase in complexity" listing each scenario's focus | `examples/sandbox/healthcare_support/README.md` (Scenarios section) |
| Deterministic input corpus | Fixture generator writes synthetic 10-K texts/PDFs whose figures match the golden rows (e.g., "$1,284 million" ↔ `1284.0`) | `examples/sandbox/tutorials/data/dataroom/setup.py:64-140`; `evals.py:48` |
| Pinned external repo input | `REPO_REF = "621e4974ca25ce531773def586ba3ed8e736b3fc"` mounted via `GitRepo(repo=..., ref=...)` | `examples/sandbox/tutorials/repo_code_review/main.py:33-34,105` |
| Sandbox environment pin (weak) | `DEFAULT_SANDBOX_IMAGE = "sandbox-tutorials:latest"` — mutable local tag, not a digest | `examples/sandbox/tutorials/misc.py:56` |
| Reproduction runbook | README documents setup → main → eval command sequence for both CSV and Docker modes | `examples/sandbox/tutorials/dataroom_metric_extract/README.md:17-32` |
| Determinism intent stated | "exits after the scripted review so the generated artifacts and eval contract stay deterministic"; "if the row set is wrong, `evals.py` fails and you iterate" | `examples/sandbox/tutorials/repo_code_review/README.md` (Setup/Why); `dataroom_metric_extract/README.md:11` |
| Example-runner exclusion of eval scripts | Tutorial mains/evals listed in `DEFAULT_AUTO_SKIP`; asserted by test | `examples/run_examples.py:54,89-94`; `tests/test_run_examples_script.py:25-34` |
| Versioned golden contract (API surface) | `"baseline": "v0.22.0"`, `"baseline_commit": "fb8fa1b…"` | `tests/fixtures/released_api_contract.json:2-3` |
| Contract enforcement | Released-contract test imports validators and pins `CONTRACT` fixture path | `tests/test_released_api_contract.py:27-33,37` |
| Deterministic testing substrate | `ScriptedModel` ("deterministic provider-neutral model for testing agent workflows"); scripted sandbox sessions | `src/agents/testing/model.py:249-254`; `src/agents/testing/__init__.py:24` |
| No CI eval execution | Workflows are docs/issues/publish/release-tag/tests only | `.github/workflows/{docs,issues,publish,release-tag,tests}.yml` |

## Answers to Dimension Questions

**1. How are datasets managed?**
Ad hoc, per-example, all inside `examples/`. Three patterns coexist: generated fixtures committed to disk via a script (`examples/sandbox/tutorials/data/dataroom/setup.py:64-140`), hand-written JSON scenario files loaded by glob at runtime (`examples/sandbox/healthcare_support/data.py:74-77`), and a live external repository pinned by SHA and mounted through the sandbox manifest (`examples/sandbox/tutorials/repo_code_review/main.py:105`). There is no central registry, manifest format, or shared loader across examples.

**2. Are datasets versioned?**
No. None of the scenario JSON files, fixture JSONs, or golden tables carries a version field (grep for `version` in scenarios and `setup.py` finds nothing). Versioning is implicit git history only (and this checkout is squashed to a single commit). The sole explicit versioning is the API-contract fixture's `baseline`/`baseline_commit` pair (`tests/fixtures/released_api_contract.json:2-3`), which versions the public API surface, not any task dataset.

**3. Are expected outputs defined?**
Yes, in two styles. Executable style: hardcoded golden tables and structural assertions checked against produced artifacts (`examples/sandbox/tutorials/dataroom_metric_extract/evals.py:47-210,240-302`; `examples/sandbox/tutorials/repo_code_review/evals.py:7-60`). Declarative style: per-scenario `expected` blocks (intent classification, required tool calls, required resolution elements) and `gold.expected_next_step` embedded in scenario JSON and typed by Pydantic (`models.py:16-31`) — but **no code reads them**, so they function as reviewer-facing documentation rather than automated checks. This split is the clearest inconsistency in the dimension.

**4. Are benchmarks reproducible?**
Partially, and only for inputs/graders — not results. Inputs are reproducible: fixtures regenerate deterministically (`setup.py`), and the reviewed repo is pinned to a full commit SHA (`main.py:34`). Graders are pure functions of artifacts, re-runnable via documented commands (`dataroom_metric_extract/README.md:17-32`). But: (a) producing the artifacts requires a live LLM call with no temperature/seed control surfaced in the examples, (b) no baseline result artifacts are stored to compare against six months later, (c) the Docker execution environment is pinned by mutable tag `sandbox-tutorials:latest` (`misc.py:56`), and (d) CI never runs these pipelines (`.github/workflows/tests.yml` covers lint/typecheck/unit tests). A grader verdict is reproducible; a passing run is not guaranteed.

## Architectural Decisions

- **Eval logic lives in examples, not the library.** The core SDK provides only *mechanisms* for determinism — `ScriptedModel` (`src/agents/testing/model.py:249`) and `scripted_sandbox_session` (`src/agents/testing/__init__.py:24`) — while golden-task evaluation is delegated to standalone scripts beside each demo. AGENTS.md codifies this posture ("prefer `ScriptedModel` … over adding a new mock or fake `Model`", AGENTS.md Testing section), keeping the shipped package free of benchmark opinions.
- **Golden answers as code constants, not data files.** Both executable graders embed expectations as Python dicts/sets (`repo_code_review/evals.py:7-10`; `dataroom_metric_extract/evals.py:20-210`), trading dataset/tooling interop for zero-dependency reviewability.
- **Artifact-file contract between agent and grader.** The agent writes structured artifacts (`findings.jsonl`, `fix.patch` via `write_review_artifacts`, `main.py:86-93`; `financial_metrics.{csv,jsonl}`), and the grader is a separate process step reading them (`evals.py --artifact-path`). This decoupling means eval correctness never depends on SDK internals.
- **Contract-fixture pattern reused from release engineering.** The released API contract with baseline commit + policy file (`tests/fixtures/released_api_contract_policy.json`) applies a versioned-golden discipline to the API surface; the same discipline was simply not extended to task datasets.

## Notable Patterns

- **Keyed golden-row comparison with bidirectional set checks**: the dataroom grader builds observed keys, then asserts duplicates absent, count exact, every golden key present with matching value/unit, and no unexpected rows (`dataroom_metric_extract/evals.py:242-300`) — a complete exact-match protocol, not just spot checks.
- **Semantic-but-deterministic assertions**: instead of exact string match on free-text comments, the repo-review grader lowercases/strips words and requires keyword membership (e.g., "nox" must appear; one of {uv, pip, install, project, test}) (`repo_code_review/evals.py:36-42`), plus negative scope rules on the patch (`evals.py:57-58`).
- **Prompt-as-rubric coupling**: the demo question and injected AGENTS.md instruct the agent toward exactly what the grader will check ("Return exactly two findings… mention nox… `-> int` type hints", `main.py:36-56`), making the golden contract visible to the model under test.
- **Difficulty ladder by composition, not scores**: healthcare scenarios escalate capabilities (lookup → referral → memory reuse → prior-auth confusion → document generation → human approval) described in README prose rather than numeric difficulty metadata (`healthcare_support/README.md`, Scenarios).

## Tradeoffs

- **Simplicity vs. scale**: hardcoded golden dicts make each tutorial self-contained and diffable, but adding tasks means editing Python, and nothing shares schema, loaders, or reporting across the three dataset styles.
- **Determinism vs. realism**: synthetic 10-K numbers and a scripted-review exit guarantee stable grading, but measure extraction/review mechanics, not long-horizon robustness; ambiguity is confined to the healthcare transcripts.
- **Documentation-grade gold vs. executable gold**: embedding rich `expected`/`gold` metadata in scenario JSON communicates intent cheaply, yet without a consumer it rots silently — there is no test asserting required_tool_calls actually occur during a run.
- **Git-as-version-control vs. explicit dataset versions**: fine while datasets are tiny and single-commit, but impossible to cite a dataset snapshot (e.g., "scenarios@2024-01") in a reported result.

## Failure Modes / Edge Cases

- **Golden/source drift**: `EXPECTED_ROWS` values are manually transcribed from prose the fixture generator emits (e.g., "$1,284 million" ↔ `(…, "Revenue", "FY2025", None): (1284.0, "USD millions")`, `setup.py` vs `evals.py:48`). Nothing cross-validates generator output against the golden table, so editing one silently breaks the other — the eval would then fail every run (or worse, pass against stale data).
- **Silently dead expectations**: if an agent stops calling `insurance_eligibility_lookup`, no automated check fails; the `required_tool_calls` golden list is inert (`models.py:19`).
- **Mutable tag drift**: rebuilding `sandbox-tutorials:latest` changes the tool environment underneath previously passing runs (`misc.py:56`), undermining six-month reproduction even with identical prompts.
- **Model-version confounding**: results depend on whichever OpenAI model serves the request; no model name/date is recorded alongside artifacts, so historical passes can't be attributed.
- **Grader brittleness to formatting**: exact unit strings ("USD millions", "percent") and stripped-key comparisons mean benign formatting differences fail the run (`evals.py:285-291`); conversely the keyword-based comment check can be gamed by mentioning tokens without substance.

## Future Considerations

- Give the two executable graders a shared minimal runner (discover `evals.py`, uniform exit codes, JSON summary artifact) so tutorial evals become one command and CI-runnable.
- Add a `version` field (or git-sha stamp written by generators) to scenario JSON and generated fixtures; record `{model, model_version, timestamp}` next to each output artifact.
- Either wire `ScenarioCase.expected` into an assertion pass (e.g., post-run check that `required_tool_calls` ⊆ executed tool names, using recorded run items) or remove it — currently it implies more rigor than exists.
- Generate `EXPECTED_ROWS` from the same source-of-truth table used by `setup.py` to eliminate transcription drift.
- Pin the tutorial Docker image by digest and log the resolved image ID into run outputs.

## Questions / Gaps

- **No evidence found** for any dataset version identifier, checksum, or changelog across `examples/sandbox/**/data*`, scenario JSONs, and both `evals.py` files; searched `version|golden|dataset|benchmark` repo-wide (excluding `.git`, `uv.lock`).
- **No evidence found** that any benchmark-style aggregate (pass rate, score) is computed, stored, or trended anywhere in the repository; graders emit only pass/fail lines ("Eval checks passed…").
- Whether upstream GitHub history tracks per-dataset evolution could not be verified from this checkout: `git log` shows a single squashed commit (`2334679`).
- The healthcare `followup_qa` mechanism (simulated second-turn answers) is fed into the workflow prompt (`workflow.py:398`) but no multi-turn scoring harness exercises it; its role as task infrastructure is inferred, not demonstrated.

---

Generated by Dimension 18.01 (Dataset and Golden Task Management) against `openai-agents-sdk`.
