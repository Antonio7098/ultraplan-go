# Source Analysis: agent-framework

## Dimension 18.01 — Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python + .NET (C#) monorepo, plus a nascent `go/` tree; GitHub Actions CI |
| Analyzed | 2026-08-26 |

## Summary

agent-framework manages eval datasets and golden tasks in three distinct layers rather than one registry. First, the experimental `agent-framework-lab` package (`python/packages/lab/`) ships benchmark harnesses for GAIA and τ²-bench plus RL training data: GAIA downloads its dataset from Hugging Face pinned to an explicit snapshot revision (`python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:426`), while τ² delegates dataset ownership to the upstream sierra-research repo with a pinned package commit but an unpinned manual data clone (`python/packages/lab/tau2/README.md:35-50`). Second, the core evaluation APIs in both languages define first-class golden-answer fields — `EvalItem.expected_output` / `expected_tool_calls` in Python (`python/packages/core/agent_framework/_evaluation.py:206-214`) and `ExpectedOutput` / `ExpectedToolCalls` in .NET (`dotnet/src/Microsoft.Agents.AI/Evaluation/EvalItem.cs:113-124`) — but datasets themselves are inline arrays inside samples, not versioned files. Third, a deterministic replay mechanism exists only for sample validation, where agent-authored "playbooks" are content-hashed and replayed without LLM involvement (`python/scripts/sample_validation/playbook.py:96-162`).

Reproducibility is partial. The strongest signal is GAIA's pinned HF revision and its tested official scorer (`gaia_scorer`, `python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:196-235`; tests at `python/packages/lab/gaia/tests/test_gaia.py:11-35`). The weakest signals are an unseeded `random.shuffle` before `max_n` truncation in the GAIA task loader (`gaia.py:357-360`), a default data cache under the system temp directory (`gaia.py:387`), and τ²'s unpinned data clone instruction. CI runs only lab unit tests, never the benchmarks (`/.github/workflows/python-lab-tests.yml:80-84`), so no automated process can reproduce a published benchmark number six months later; the τ² results table in `python/packages/lab/tau2/README.md:109-121` is a manually produced artifact.

## Rating

**5 / 10** — Present but inconsistent and fragile.

Golden-answer plumbing is genuinely well designed and tested in both languages (that sub-area alone would score 7). But dataset management is fragmented across three mechanisms, version pinning is applied to code artifacts (HF revision, tau2 git commit) while the tau2 *data* itself is cloned unpinned, the GAIA runner's unseeded shuffle makes sampled runs non-reproducible, benchmark execution is entirely outside CI, and all of it lives in a self-declared experimental package that "may experience breaking changes or be deprecated" (`python/packages/lab/README.md:5`, `88-92`). A benchmark result today cannot be reliably reproduced six months later without undocumented luck.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Eval dataset: GAIA | External HF dataset `gaia-benchmark/GAIA`, downloaded via `snapshot_download` | python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:420-430 |
| Dataset version identifier (GAIA) | Pinned revision `682dd723ee1e1697e00360edccf2366dc8418dd9` | python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:426 |
| Dataset format (GAIA) | Parquet (`metadata*.parquet`) preferred, JSONL fallback; validation split prioritized for public answers | python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:271-275, 325-327 |
| Golden answer definition (GAIA) | `Task.answer` field + official `gaia_scorer` with numeric/list/string normalization rules | python/packages/lab/gaia/agent_framework_lab_gaia/_types.py:24; gaia.py:196-235 |
| Scorer tests | Numeric, string-normalization, list-matching, None-handling cases | python/packages/lab/gaia/tests/test_gaia.py:11-35 |
| Task metadata schema (GAIA) | `Task(task_id, question, answer, level, file_name, metadata)`; full source record preserved | python/packages/lab/gaia/agent_framework_lab_gaia/_types.py:18-27 |
| Difficulty metadata (GAIA) | `level` filter parameter on run; emitted as span attributes | python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:512-524, 443-448 |
| Eval dataset: τ²-bench | Data owned upstream; requires manual clone into `TAU2_DATA_DIR` | python/packages/lab/tau2/README.md:38-50 |
| Version identifier (τ²) | Package pinned to commit `5ba9e3e56db57c5e4114bf7f901291f09b2c5619`; data clone NOT pinned | python/packages/lab/tau2/README.md:35 vs 40-44 |
| Reproduction script (τ²) | `samples/run_benchmark.py` with model/task/max-steps CLI args; records model config per result | python/packages/lab/tau2/samples/run_benchmark.py:249-259, 134-140 |
| Eval dataset: RL math training | Vendored JSONL, 64 train / 20 test records with ground-truth `result` field | python/packages/lab/lightning/samples/data/math/train.jsonl:1; test.jsonl:1 |
| Task schema (RL) | `MathProblem` TypedDict: `id`, `question`, `chain`, `result` (ground truth), `source` | python/packages/lab/lightning/samples/train_math_agent.py:30-46 |
| Seeded split (RL) | `random.Random(42)` "Deterministic train/val split ... for reproducible experiments" | python/packages/lab/lightning/samples/train_tau2_agent.py:59-62 |
| Golden output field (Python) | `EvalItem(expected_output=..., expected_tool_calls=...)` | python/packages/core/agent_framework/_evaluation.py:201-215 |
| Golden output orchestration (Python) | `evaluate_agent(..., expected_output=[...])` with length validation against queries | python/packages/core/agent_framework/_evaluation.py:1630-1636, 1757-1759 |
| Consistency repetitions (Python) | `num_repetitions` param validated ≥1 and looped per query | python/packages/core/agent_framework/_evaluation.py:1641, 1753-1755, 1791 |
| Result record schema (Python) | `EvalItemResult`: item_id/status/scores/error_code/response_id/token_usage/metadata | python/packages/core/agent_framework/_evaluation.py:326-355 |
| Golden output fields (.NET) | `EvalItem.ExpectedOutput`, `EvalItem.ExpectedToolCalls` | dotnet/src/Microsoft.Agents.AI/Evaluation/EvalItem.cs:113-124 |
| Golden comparison checks (.NET) | `ContainsExpected`, `ToolCallArgsMatch` (subset arg matching), `KeywordCheck`, `NonEmpty` | dotnet/src/Microsoft.Agents.AI/Evaluation/EvalChecks.cs:224-244, 138-198, 33-63, 205-217 |
| Expected-output sample (.NET) | Inline query/expected arrays fed to `EvaluateAsync` | dotnet/samples/02-agents/Evaluation/Evaluation_ExpectedOutputs/Program.cs:27-35 |
| Evaluation unit tests (.NET) | ~40 tests incl. mismatched expected-output throws and ContainsExpected failure paths | dotnet/tests/Microsoft.Agents.AI.UnitTests/EvaluationTests.cs:1431, 1511-1538 |
| Eval architecture decision | Accepted ADR: evaluator protocol, Foundry integration, `num_repetitions` design | docs/decisions/0023-foundry-evals-integration.md:1-53 |
| Golden event-stream tests | Scenario suites assert ordered AG-UI event sequences in memory (no stored golden files) | python/packages/ag-ui/tests/ag_ui/golden/test_scenario_hitl.py:60-80; python/packages/ag-ui/tests/ag_ui/event_stream.py:84-175 |
| Deterministic sample validation | Playbook = hashed, replayable validation recipe; staleness via SHA-256 of sample files | python/scripts/sample_validation/playbook.py:44-76, 96-115, 155-162 |
| CI boundary for benchmarks | Lab workflow runs unit tests + lint only; integration/benchmarks excluded | .github/workflows/python-lab-tests.yml:80-84 |
| Results persistence (GAIA) | JSONL export incl. timestamp, task_metadata, messages, prediction_metadata | python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:613-649 |
| Results viewer (GAIA) | `gaia_viewer` console tool filters by level/correctness | python/packages/lab/gaia/agent_framework_lab_gaia/gaia.py:652-713 |
| Experimental status | Lab modules explicitly not production-stable; breaking changes expected | python/packages/lab/README.md:3-5, 86-92 |

## Answers to Dimension Questions

**1. How are datasets managed?**
Three separate mechanisms. Benchmark datasets are external and fetched at runtime: GAIA from Hugging Face into a configurable `data_dir` that defaults to `{tempdir}/data_gaia_hub` (`gaia.py:387`, `406-433`); τ² by manually cloning the upstream repo's `data/` folder and pointing `TAU2_DATA_DIR` at it (`python/packages/lab/tau2/README.md:40-50`). Training data for the Lightning RL sample is vendored JSONL committed to the repo (`python/packages/lab/lightning/samples/data/math/train.jsonl`). For application-level evaluation there is no dataset object at all — callers pass inline `queries`/`expected_output` sequences to `evaluate_agent()` (`_evaluation.py:1630-1636`) or `EvaluateAsync()` (`dotnet/samples/02-agents/Evaluation/Evaluation_ExpectedOutputs/Program.cs:32-35`). There is no central dataset registry, catalog, or manifest anywhere in the repo (searched for `dataset|golden|fixture` directories; only test-fixture folders for unrelated provider tests were found).

**2. Are datasets versioned?**
Partially, and inconsistently. GAIA is pinned to an exact HF snapshot revision (`gaia.py:426`) — the best versioning practice found. The τ² *package* is pinned to a git commit (`python/packages/lab/tau2/README.md:35`), but the instructions for cloning the τ² *data* use a bare `git clone` of the default branch with no commit or tag (`README.md:40-44`), so the task set can drift independently of the pinned code. The vendored math JSONL carries no version identifier; provenance is only inferable from record IDs like `ape210k__00384263` and `svamp__chal-551` (`train.jsonl:1`, `test.jsonl:1`). The lab package itself is versioned as `1.0.0b260730` (`python/packages/lab/pyproject.toml:7`).

**3. Are expected outputs defined?**
Yes — this is the strongest area. Golden answers are first-class API fields in both languages: `Task.answer` consumed by the official GAIA scorer (`gaia.py:196-235`), `EvalItem.expected_output`/`expected_tool_calls` in Python (`_evaluation.py:206-214`), and `ExpectedOutput`/`ExpectedToolCalls` in .NET (`EvalItem.cs:113-124`). Built-in comparators implement the matching semantics: normalized exact match for GAIA (numbers to 1e-6 tolerance, list element-wise, punctuation/space-insensitive strings, `gaia.py:170-235`); substring containment (`ContainsExpected`, `EvalChecks.cs:224-244`) and subset argument matching for expected tool calls (`ToolCallArgsMatch`, `EvalChecks.cs:138-198`) on the .NET side; keyword, tool-called, and tool-args checks in Python (`_evaluation.py:1061-1284`). Mismatch handling is defensive: passing zero `expected_output` values for N queries raises (`_evaluation.py:1757-1759`; mirrored by `BuildItemsFromResponses_MismatchedExpectedOutput_Throws` at `dotnet/tests/Microsoft.Agents.AI.UnitTests/EvaluationTests.cs:1431`), and `ContainsExpected` fails explicitly when `ExpectedOutput` is unset (`EvalChecks.cs:228-231`). However, expected outputs exist only inline at call sites in samples; none are persisted as versioned dataset files.

**4. Are benchmarks reproducible?**
Only partially — this is where the model breaks down. In favor: GAIA pins the dataset revision; τ²'s `run_benchmark.py` records assistant/user model IDs in every result row and writes incrementally to timestamped JSONL (`python/packages/lab/tau2/samples/run_benchmark.py:71-74, 134-140`); the RL sample uses a seeded `Random(42)` split explicitly labeled "for reproducible experiments" (`train_tau2_agent.py:59-62`); the evaluation API offers `num_repetitions` to measure nondeterminism (`_evaluation.py:1641,1791`; ADR `docs/decisions/0023-foundry-evals-integration.md:117-130`). Against: (a) GAIA's loader calls `random.shuffle(tasks)` before applying `max_n` with no seed (`gaia.py:357-360`), so two runs with identical arguments evaluate different task subsets; (b) the τ² data clone is unpinned (`tau2/README.md:40-44`); (c) GAIA caches data in the system temp dir keyed only by existence of any `metadata.jsonl` (`gaia.py:408-409`), so a stale or foreign cache silently changes the task set; (d) CI never runs the benchmarks (`python-lab-tests.yml:80-84` excludes integration markers), so the published success-rate table (`tau2/README.md:109-121`) has no automated reproduction path; (e) user-simulator sampling temperature is configurable but nothing seeds the simulation. A full-dataset run (no `max_n`) with a pinned cache would reproduce; the common sampled invocation would not.

## Architectural Decisions

1. **Benchmarks live in an experimental "lab" package, deliberately outside the stable core** (`python/packages/lab/README.md:3-14`). This isolates volatile third-party benchmark dependencies (HF hub, tau2-bench from git, agent-lightning) from the framework contract, at the cost of signaling that dataset management has no stability guarantees.
2. **Provider-agnostic evaluation protocol instead of a bundled eval framework** (`docs/decisions/0023-foundry-evals-integration.md:47-53`). The team explicitly rejected building "full eval infrastructure including custom evaluator definitions, scoring profiles, and reporting" — which explains why dataset management is thin: the design centers on an `Evaluator` protocol (`evaluate(items) → results`, ADR line 31; Python `_evaluation.py:683`; .NET `IAgentEvaluator.cs`) rather than dataset abstractions.
3. **Golden answers as optional item fields, not external fixtures** (`EvalItem.cs:113-124`, `_evaluation.py:206-214`). Keeps evaluation self-contained and easy in samples, but pushes dataset curation responsibility onto downstream users.
4. **Upstream dataset ownership for τ²** (`tau2/README.md:32-50`): the harness wraps sierra-research's environment/evaluators rather than vendoring tasks, avoiding duplication but inheriting unpinned-data risk.
5. **Deterministic replay caching for sample validation** (`playbook.py:1-11,155-162`): the one place in the repo where reproducibility is engineered deliberately — capture once, hash inputs (SHA-256 over sample files), replay without LLM, invalidate on change.

## Notable Patterns

- **Robust multi-schema ingestion**: GAIA's loader accepts parquet and JSONL and resolves field names across dataset variants (`Question|question|query|prompt`, `Final answer|answer|final_answer`, `task_id|question_id|id|uuid`) with synthetic ID fallbacks (`gaia.py:290-302, 330-341`) — pragmatic against upstream format drift.
- **Answer-only filtering**: only tasks whose answers are publicly available (validation split, non-placeholder) are evaluated (`gaia.py:307-310, 347-350`), keeping grading local instead of requiring leaderboard submission.
- **Rich result envelopes for auditability**: GAIA exports question, answer, prediction, correctness, timing, error, timestamp, full message transcript, and both metadata dicts to JSONL (`gaia.py:631-648`); Python `EvalItemResult` carries error codes, response IDs, and token usage separately from quality status (`_evaluation.py:345-355`).
- **Observability-integrated runs**: every GAIA phase (data ensure, task load, execute, evaluate, save) is a traced span with task-level attributes including difficulty level (`gaia.py:443-448, 543-558`).
- **Golden-scenario conformance tests**: the ag-ui package maintains nine scenario suites asserting exact event-sequence bookends and type ordering in memory (`python/packages/ag-ui/tests/ag_ui/event_stream.py:84-175`), a "golden trace" pattern implemented without stored fixture files.

## Tradeoffs

- **Flexibility vs. reproducibility**: untyped inline `queries`/`expected_output` lists make evaluation trivially accessible, but nothing records which dataset version produced a result — except τ², which embeds the whole task dump per row (`run_benchmark.py:42`).
- **Freshness vs. stability**: GAIA's temp-dir cache avoids re-downloads yet accepts any pre-existing `metadata.jsonl` layout (`gaia.py:408`), trading correctness for convenience.
- **Sampling vs. cost**: shuffling before `max_n` gives varied coverage across repeated cheap runs (the stated rationale, "rate-limits and fairness", `gaia.py:357`) but destroys run-to-run comparability.
- **Experimental isolation vs. investment signal**: keeping datasets/benchmarks in `lab` protects the core API, yet means the reproducibility gaps documented above carry no fix-forward obligation.

## Failure Modes / Edge Cases

- **Non-reproducible sampled runs**: unseeded shuffle + `max_n` (`gaia.py:357-360`) yields different accuracy numbers for identical commands; no warning is emitted.
- **Silent stale-cache evaluation**: if `{tempdir}/data_gaia_hub` contains an old-format or partially deleted download, `_ensure_data` short-circuits and evaluates against whatever is present (`gaia.py:408-409`).
- **Dataset drift for τ²**: re-cloning tau2-bench main after the pinned-code commit can add/rename airline tasks; `run_benchmark.py` would happily average over the changed set (`run_benchmark.py:80-81`).
- **Parquet loader degradation**: if pyarrow is missing or a file fails to parse, the loader prints a warning and continues, potentially evaluating an empty/partial set — caught only by the later `RuntimeError` when *zero* tasks load (`gaia.py:318-323, 565-569`); a partial set proceeds silently.
- **Placeholder-answer leakage guard**: rows whose answer is `"?"` or empty are skipped (`gaia.py:309-310`) — correct behavior, but it means dataset composition depends on upstream annotation state at the pinned revision.
- **Playbook staleness scope**: sample-validation hashing covers sample files only; a change to validation tooling itself does not invalidate stored playbooks (`playbook.py:96-115`).

## Future Considerations

- Seed (or remove) the GAIA shuffle and persist the selected task IDs alongside results, making sampled runs reconstructible (`gaia.py:357-360`).
- Pin the τ² data clone to a specific commit/tag in `python/packages/lab/tau2/README.md:40-44`, mirroring the package pin already present.
- Add a lightweight dataset manifest (name, version/revision, checksum, task count, split) written into every results JSONL header, so archived results self-describe their input.
- Promote the inline `queries`/`expected_output` pattern to an optional file-backed dataset loader in the core evaluation API (JSONL of `{query, expected_output, expected_tool_calls}` rows), closing the gap between the lab harnesses' file formats and the core API's inline-only interface.
- Add a scheduled CI job that reproduces a small, seeded slice of GAIA and τ² to detect scorer/dataset regressions, extending `python-lab-tests.yml` beyond unit tests.
- Give the vendored Lightning math data an explicit version/provenance note (upstream dataset names are already embedded in IDs: `ape210k__*`, `svamp__*`).

## Questions / Gaps

- No evidence found of any central benchmark-results store, dashboard, or historical tracking inside the repo; results are per-run JSONL files only (`gaia.py:613-649`, `run_benchmark.py:71-74`).
- No evidence found that the τ² README results table (`python/packages/lab/tau2/README.md:109-121`) can be regenerated exactly: temperature seeding for the user simulator is not documented, and the data snapshot used is unspecified.
- No evidence found of difficulty/category taxonomies beyond GAIA's integer `level` (1–3) — core `evaluate_agent` queries carry no metadata fields (`_evaluation.py:1630-1649`), so task categorization for application evals is left entirely to consumers.
- The `.NET Harness` evaluator tests (e.g., `dotnet/tests/Microsoft.Agents.AI.UnitTests/Harness/Loop/TodoCompletionLoopEvaluatorTests.cs`) cover loop-completion judging logic, but no harness-level golden-task corpus was found in either language; searched `golden|fixture|dataset` across both trees.
- The `go/` tree contained no evaluation or dataset code (directory listing showed no Go module content relevant to this dimension).

---

Generated by dimension 18.01 (Dataset and Golden Task Management) against `agent-framework`.
