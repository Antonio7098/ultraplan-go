# Source Analysis: crewai

## Dimension 18.01: Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | crewai |
| Path | `studies/agent-harness-study/sources/crewai` |
| Language / Stack | Python (uv workspace monorepo: `lib/crewai`, `lib/cli`, `lib/crewai-tools`, `lib/crewai-core`, `lib/devtools`; pytest + VCR cassettes) |
| Analyzed | 2026-08-26 |

## Summary

CrewAI has no first-party evaluation datasets. There are no committed golden-task files (no JSONL/JSON/YAML eval corpora anywhere in the repo — verified by filename search for `*eval*`, `*dataset*`, `*golden*`, `*.jsonl`). What exists instead is an **experimental, in-memory dataset API**: `ExperimentRunner` consumes a user-constructed Python list of dicts shaped `{identifier?, inputs, expected_score}` (`lib/crewai/src/crewai/experimental/evaluation/experiment/runner.py:27`, `runner.py:65-70`) via the public helper `run_experiment()` (`lib/crewai/src/crewai/experimental/evaluation/testing.py:56-64`). Golden answers are not literal expected outputs but **LLM-judged score thresholds** (`>=` comparison, `runner.py:130-165`), judged on a 0–10 scale across six `MetricCategory` dimensions (`lib/crewai/src/crewai/experimental/evaluation/base_evaluator.py:20-29`, `base_evaluator.py:32-49`).

Versioning and task metadata are essentially absent: no version identifiers exist for datasets; the only identity mechanism is an optional user-supplied identifier or an MD5 hash of the test-case dict (`runner.py:67-70`). A `WEIGHTED_BY_COMPLEXITY` aggregation strategy is declared (`base_evaluator.py:82`) but never implemented — aggregation always computes a simple average (`lib/crewai/src/crewai/experimental/evaluation/evaluation_display.py:250-304`). The older production path, `Crew.test()` / `crewai test` CLI (`lib/crewai/src/crewai/crew.py:2227-2272`; `lib/cli/src/crewai_cli/evaluate_crew.py:9-31`), runs n iterations of one input through an LLM judge and prints a table; results are not persisted for later comparison.

Reproducibility is strong where it was engineered deliberately — the framework's own test suite replays recorded LLM traffic from committed VCR cassettes with CI forced to `record_mode=none` (`conftest.py:416`, `conftest.py:424-425`) — but weak at the benchmark layer: baseline regression files are mutable local JSON that the library silently creates and appends to (`lib/crewai/src/crewai/experimental/evaluation/experiment/result.py:48-97`). Six months later, a benchmark result can be re-*compared* against a saved baseline file only if the user kept it; it cannot be reproduced deterministically.

## Rating

**4 / 10 — Present but inconsistent, weakly documented, and fragile.**

Rationale against the rubric:

- The experiment/baseline model exists as explicit, tested code (`tests/experimental/evaluation/test_experiment_runner.py`, `test_experiment_result.py`), which keeps this out of the 1–3 band.
- But the dataset itself is an undocumented dict convention (missing keys raise bare `KeyError` at `runner.py:65-66`), there is no dataset format on disk, no version identifiers, no difficulty/category metadata schema, dead complexity-weighting enum values, and the flagship `crewai test` flow persists nothing comparable. `run_experiment`/`assert_experiment_successfully` have zero call sites in the entire repo (grep over all of `lib/` and `docs/` found only their definitions), marking the whole layer as an unused experimental skeleton.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dataset container | `ExperimentRunner.__init__(self, dataset: list[dict[str, Any]])` — datasets are plain in-memory lists, no loader | lib/crewai/src/crewai/experimental/evaluation/experiment/runner.py:27 |
| Test-case schema (implicit) | Required keys read by direct subscript: `test_case["inputs"]`, `test_case["expected_score"]`; optional `"identifier"` | lib/crewai/src/crewai/experimental/evaluation/experiment/runner.py:65-70 |
| Public entrypoint | `run_experiment(dataset, crew=None, agents=None, verbose=False)` | lib/crewai/src/crewai/experimental/evaluation/testing.py:56-64 |
| Zero adoption of entrypoint | Only match for `run_experiment`/`assert_experiment_successfully` in repo is the definition itself | lib/crewai/src/crewai/experimental/evaluation/testing.py:13,56 |
| Identifier fallback | `md5(str(test_case))` content hash when no identifier given | lib/crewai/src/crewai/experimental/evaluation/experiment/runner.py:67-70 |
| Golden answer = threshold | `_assert_scores`: pass iff `actual >= expected`, per-metric dict matching supported | lib/crewai/src/crewai/experimental/evaluation/experiment/runner.py:130-165 |
| Score scale contract | `EvaluationScore.score`: float 0–10, ge=0 le=10, default 5.0 | lib/crewai/src/crewai/experimental/evaluation/base_evaluator.py:32-38 |
| Metric taxonomy | `MetricCategory` enum: goal_alignment, semantic_quality, reasoning_efficiency, tool_selection, parameter_extraction, tool_invocation | lib/crewai/src/crewai/experimental/evaluation/base_evaluator.py:20-27 |
| Default evaluator set | `create_default_evaluator` wires all six metric evaluators | lib/crewai/src/crewai/experimental/evaluation/agent_evaluator.py:344-365 |
| Expected-output definition per task | `Task.expected_output: str` field, required via validator | lib/crewai/src/crewai/task.py:153, task.py:384-386 |
| LLM-judge prompt uses expected_output | Evaluation query embeds description + expected_output + actual output | lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:86-95 |
| CrewEvaluator score model | `TaskEvaluationPydanticOutput.quality` 1–10 judged against `task_expected_output` | lib/crewai/src/crewai/utilities/evaluators/crew_evaluator_handler.py:23-26, :73-85 |
| Result persistence format | `ExperimentResults.to_json`: `{timestamp, metadata, results[]}`, metadata defaults to `{}` — no version field | lib/crewai/src/crewai/experimental/evaluation/experiment/result.py:19-24, :32-46 |
| Baseline regression compare | `compare_with_baseline`: loads JSON baseline, sorts runs by timestamp, classifies improved/regressed/unchanged/new/missing | lib/crewai/src/crewai/experimental/evaluation/experiment/result.py:48-143 |
| Baseline auto-creation | If baseline missing, current run silently becomes the new baseline | lib/crewai/src/crewai/experimental/evaluation/experiment/result.py:70-78 |
| Regression assertions | `assert_experiment_successfully` raises on failed cases or regressed tests; warns on missing tests | lib/crewai/src/crewai/experimental/evaluation/testing.py:13-53 |
| Dead complexity weighting | `WEIGHTED_BY_COMPLEXITY` enum value exists; no complexity computation anywhere in `lib/` | lib/crewai/src/crewai/experimental/evaluation/base_evaluator.py:82 |
| Aggregation ignores strategy | `_aggregate_agent_results` always simple-averages scores regardless of strategy; strategy only tunes feedback-summary prompt guidance | lib/crewai/src/crewai/experimental/evaluation/evaluation_display.py:250-304, :333-341 |
| Production test path | `Crew.test(n_iterations, eval_llm, inputs)` loops iterations through `CrewEvaluator`, prints table, persists nothing | lib/crewai/src/crewai/crew.py:2227-2272 |
| CLI delegation | `evaluate_crew` shells out to `uv run test <n> <model>` in generated project env | lib/cli/src/crewai_cli/evaluate_crew.py:9-31 |
| Test-command docs | `crewai test -n 5 -m gpt-4o` documented; no dataset concept documented at all | docs/edge/en/concepts/testing.mdx:14-30 |
| Feature origin | Changelog v0.148.0 (Jul 16, 2025): "Added Evaluator experiment and regression testing methods" | docs/edge/en/changelog.mdx:3596 |
| Runner unit tests | Deterministic mocked tests cover identifier hashing, threshold logic, unknown metrics | lib/crewai/tests/experimental/evaluation/test_experiment_runner.py:52-109, :176-209 |
| Baseline-compare unit tests | Improved/regressed/unchanged/new/missing classification asserted against fixture baseline | lib/crewai/tests/experimental/evaluation/test_experiment_result.py:52-111 |
| Recorded goldens for own tests | VCR cassettes store real evaluator LLM exchanges (e.g., asserted score 5.0 and feedback substring) | lib/crewai/tests/experimental/evaluation/test_agent_evaluator.py:57-105; lib/crewai/tests/cassettes/experimental/evaluation/TestAgentEvaluator.test_evaluate_current_iteration.yaml |
| CI replay enforcement | `record_mode: none` when `GITHUB_ACTIONS=true`; default `once` locally | conftest.py:416, conftest.py:424-425 |
| Secret scrubbing on record | 50+ headers filtered into placeholders before cassette write | conftest.py:255-334 |
| Endpoint normalization | Azure host → `fake-azure-endpoint.openai.azure.com`; Bedrock regional hosts → placeholder matcher | conftest.py:324-333, conftest.py:120-134 |
| Dependency pinning | Workspace `uv.lock` committed; CI keyed on `hashFiles('uv.lock')` | uv.lock; .github/workflows/tests.yml:56-62 |
| Training-data artifacts | Pickle files `training_data.pkl` / `trained_agents_data.pkl` via `CrewTrainingHandler(PickleHandler)` | lib/crewai-core/src/crewai_core/constants.py:9-10; lib/crewai/src/crewai/utilities/training_handler.py:7-36 |
| Training data eval | `evaluate_training_data` requires initial_output/human_feedback/improved_output per iteration else ValueError | lib/crewai/src/crewai/utilities/evaluators/task_evaluator.py:115-154 |
| Kickoff output store (replay) | SQLite table `latest_kickoff_task_outputs` stores expected_output/output/inputs/was_replayed per task_id | lib/crewai/src/crewai/memory/storage/kickoff_task_outputs_storage.py:46-58, :92-106 |

## Answers to Dimension Questions

**1. How are datasets managed?**
They aren't, centrally. Datasets are caller-supplied Python lists passed to `run_experiment()` (`testing.py:56-64`); the runner holds them verbatim (`runner.py:28`). There is no dataset file format, loader, registry, or directory convention inside the repository. The production-facing alternative, `crewai test` / `Crew.test()`, uses no dataset at all — it re-runs the crew's single configured input N times (`crew.py:2255-2257`). Search boundary: greps for `dataset`, `golden`, `benchmark` across `lib/` and `docs/edge` returned only third-party tool integrations (Bright Data scraping tool, observability vendor pages) and unrelated doc prose.

**2. Are datasets versioned?**
No. No version field, tag, or checksum-of-dataset exists. Partial mitigations: (a) test-case identifiers may be supplied by the user or fall back to an MD5 of the case dict (`runner.py:67-70`), giving implicit change detection — editing a case's inputs changes its identity, so baseline comparison reports it under `new_tests`/`missing_tests` rather than as a silent modification (`result.py:113-133`); (b) baseline JSON files accumulate timestamped run entries (`result.py:80-95`). Neither constitutes dataset versioning: there is no way to pin, reference, or roll back to "dataset v3".

**3. Are expected outputs defined?**
Yes, but as rubrics, not golden strings. Two layers:
- *Per-task natural-language expectations*: `Task.expected_output` (`task.py:153`) feeds the LLM judge prompts (`task_evaluator.py:89`, `crew_evaluator_handler.py:77`, `metrics/goal_metrics.py:34`).
- *Golden thresholds*: `expected_score` (float or per-metric dict) with `>=` pass semantics (`runner.py:66`, `runner.py:130-165`).
There is no mechanism anywhere for exact-output or structural golden answers (e.g., JSON-schema match, string equality). Scores themselves are produced by an LLM call, so even the thresholds sit on a nondeterministic signal.

**4. Are benchmarks reproducible?**
Only partially.
- *Framework tests*: highly reproducible — VCR-cassette replay of LLM traffic, CI locked to playback-only mode (`conftest.py:424-425`), pinned lockfile, normalized endpoints, scrubbed headers.
- *User benchmarks (`run_experiment` path)*: repeatable comparisons against a local baseline JSON (`testing.py:33-37`, `result.py:48-97`), but the underlying scores come from live LLM judges; baselines are mutable files the library auto-writes/appends; nothing pins model versions in the result payload (`to_json` records only timestamp/metadata/results, `result.py:32-39`).
- *`crewai test` path*: not reproducible in any durable sense — results go to stdout tables and telemetry events only (`crew.py:2259`, `events/types/crew_events.py:61-85`).
Verdict on the dimension's guiding question — *can a benchmark result be reproduced six months later?* No, except by manually preserving baseline JSON and accepting LLM-judge drift; the framework's own cassette-based tests can, but those are unit tests, not benchmark tasks.

## Architectural Decisions

1. **Datasets-as-code, not datasets-as-artifacts.** The experimental layer deliberately takes `list[dict]` instead of defining a file format, pushing all dataset custody onto users (`runner.py:27`, `testing.py:57-61`). Contrast: the framework's own testing infra chose committed YAML cassettes as golden artifacts.
2. **Threshold-based golden answers over exact outputs.** Pass/fail via `expected_score >= actual` (`runner.py:143-163`) tolerates LLM-judge variance but forfeits strict regression semantics — a score of 8.0 vs expected 8 passes while 7.9 fails, with no tolerance/timing controls.
3. **Content-addressed identity as a versioning substitute.** MD5-of-case-dict identifiers (`runner.py:69`) make edits visible as add/remove pairs in baseline diffs rather than supporting true revisions.
4. **Baseline files as append-only run logs.** `compare_with_baseline(save_current=True)` writes every current run back into the baseline file (`result.py:88-95`), conflating "reference" with "history".
5. **Two disconnected evaluation stacks.** The stable stack (`TaskEvaluator`, `CrewEvaluator`, wired via `Crew.test`/CLI) and the experimental stack (`AgentEvaluator`/`ExperimentRunner` under `crewai.experimental`) share neither models (1–10 vs 0–10 scales: `crew_evaluator_handler.py:25` vs `base_evaluator.py:35`) nor storage.

## Notable Patterns

- **Event-driven evaluation hooks**: evaluators subscribe to `TaskCompletedEvent` on the global bus and evaluate asynchronously post-execution (`agent_evaluator.py:68-130`), with thread-safe state guarded by `threading.Lock` (`agent_evaluator.py:61`).
- **Graceful degradation for non-function-calling LLMs**: evaluation prompts inject JSON schemas via i18n templates when function calling is unsupported (`task_evaluator.py:99-104`).
- **Defensive validation in human-feedback paths**: training evaluation hard-fails with actionable messages listing missing fields (`task_evaluator.py:141-154`) — stricter than the experiment path, which bare-`KeyError`s on malformed cases (`runner.py:65-66`).
- **Test-infrastructure hygiene**: cassette organization mirrors test tree (`conftest.py:371-403`), empty/error responses are dropped from recording (`conftest.py:337-354`), and custom matchers handle cross-region Bedrock playback (`conftest.py:130-134`, `pytest_recording_configure` at `conftest.py:406-408`).

## Tradeoffs

- **Flexibility vs. governance**: accepting raw dicts means zero-friction adoption but no schema enforcement, no migration path, and no integrity checks beyond runtime `KeyError`.
- **LLM-judge ubiquity**: every scoring path (`task_evaluator.py:113`, `crew_evaluator_handler.py:199`, all six metric evaluators e.g. `goal_metrics.py:71`) depends on an LLM call, trading determinism for generality. Feedback summarization even makes extra LLM calls during aggregation (`evaluation_display.py:306-359`).
- **Auto-managed baselines**: convenient first-run bootstrap (`result.py:70-78`) but risks silently canonizing a bad run and mutating history on every invocation unless `save_current=False`.
- **Experimental isolation**: keeping the dataset/benchmark layer under `crewai.experimental` avoids API commitment but has left it orphaned — exported (`experimental/evaluation/__init__.py:15-19,30-48`) yet undocumented in `docs/edge/en/concepts/testing.mdx` and unused internally.

## Failure Modes / Edge Cases

- **Malformed test cases fail late and cryptically**: `test_case["inputs"]` / `["expected_score"]` raise bare `KeyError` mid-run (`runner.py:65-66`), aborting the whole experiment loop (no per-case isolation, unlike evaluator errors which are caught per-metric at `agent_evaluator.py:283-292`).
- **Unknown metric names silently weaken checks**: `{"unknown_metric": 7}` matches nothing and fails correctly today (`test_experiment_runner.py:176-209`), but a typo'd real metric key in a dict-vs-dict compare reduces the check to intersection semantics (`runner.py:158-163`).
- **Baseline corruption handling is lenient**: unreadable/empty baselines are warned about and then overwritten as fresh baselines (`result.py:56-78`) — a truncated file destroys regression protection.
- **Exception during kickoff zeroes the score** and marks failure (`runner.py:104-112`), so infrastructure errors are indistinguishable from quality regressions in the baseline diff.
- **Pickle artifacts are unsafe and unversioned**: `training_data.pkl` / `trained_agents_data.pkl` (`constants.py:9-10`) are deserialized trusted data (`training_handler.py` via `PickleHandler.load`) with no schema version — classic pickle-injection and compat-drift risk.
- **Iteration state leakage**: `ExecutionState.iterations_results` keyed by agent *role* string (`agent_evaluator.py:120-130`); duplicate roles would collide within an iteration.

## Future Considerations

Concrete work items implied by the evidence:

1. Define a versioned dataset artifact (e.g., JSONL with `$schema_version`, per-case required-key validation) plus a loader for `ExperimentRunner`, replacing the bare-subscript reads at `runner.py:65-70`.
2. Implement or remove `WEIGHTED_BY_COMPLEXITY` (`base_evaluator.py:82`): either compute per-task weights or delete the variant; today `_aggregate_agent_results` ignores it (`evaluation_display.py:255`).
3. Add run-environment provenance to `ExperimentResults.to_json` — model name/version, judge config, package version — since `metadata` is currently never populated (`result.py:19-24`).
4. Make baseline management explicit: require opt-in for `save_current`, validate baseline JSON shape before overwrite (`result.py:56-78`), and separate reference-baseline from run-history.
5. Unify the two evaluation stacks' score scales and event models (0–10 vs 1–10 discrepancy noted above).
6. Replace pickle training artifacts with a versioned JSON/YAML container, or at minimum gate loading behind a schema-version check.
7. Document the experiment API in `docs/edge/en/concepts/testing.mdx` (currently only `crewai test` is covered, `testing.mdx:14-54`) or mark `run_experiment` clearly as unadopted.

## Questions / Gaps

- **Is `run_experiment` used downstream?** No call sites exist inside this source tree (grep across `lib/`, `docs/`, `scripts/`); whether external users exercise it cannot be determined from this snapshot. No evidence found of internal dogfooding.
- **Where did the cassettes' original prompts come from?** Cassettes fixate responses for evaluator tests (score 5.0 asserted at `test_agent_evaluator.py:99`), but no committed golden-task definitions explain why those specific crews/tasks were chosen.
- **Was complexity weighting planned or abandoned?** Only the enum value and a comment exist (`base_evaluator.py:80-84`); no TODO, issue reference, or partial implementation was found.
- **Enterprise-side dataset management**: `DEFAULT_CREWAI_ENTERPRISE_URL` and deploy scaffolding exist (`lib/crewai-core/src/crewai_core/constants.py:13`), suggesting benchmarking may live in the closed-source platform; no evidence found in this source either way.

---

Generated by `18.01-dataset-and-golden-task-management` against `crewai`.
