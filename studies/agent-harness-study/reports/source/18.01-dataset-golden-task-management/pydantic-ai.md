# Source Analysis: pydantic-ai

## Dimension 18.01: Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Python (pydantic-core, anyio, yaml; uv workspace with a dedicated `pydantic-evals` sub-package) |
| Analyzed | 2026-08-26 |

## Summary

Pydantic AI treats datasets as a first-class, typed artifact inside a dedicated sub-package, `pydantic-evals` (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:1-8`). A `Dataset` is a named collection of `Case` rows (`name`, `inputs`, `metadata`, `expected_output`, per-case `evaluators`), plus dataset-level `evaluators` and report-level `report_evaluators` (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:177-236`). Golden answers live directly on each case as `expected_output` and are enforced primarily by the `EqualsExpected`/`Equals`/`Contains`/`IsInstance` evaluators, complemented by LLM-as-judge evaluators for fuzzy criteria. Datasets serialize deterministically to YAML or JSON with an auto-generated JSON-schema sidecar for IDE validation and structural drift detection.

There is no first-class dataset version field anywhere in the serialization model; versioning is delegated to version control plus a filename-suffix convention demonstrated in the repo's own examples (`time_range_v1.yaml` vs `time_range_v2.yaml`). Versioning does exist one level down, on evaluators, for online-evaluation attribution (`Evaluator.get_evaluator_version`). Benchmark-style reproducibility is addressed through deterministic file round-trips, a `repeat` parameter for quantifying nondeterminism, experiment metadata attached to OTel spans, retry configuration, and trace/span IDs embedded in every report case — but reproducibility of task behavior itself remains the user's responsibility, which the docs state explicitly.

## Rating

**Score: 7/10** — Clear model with tests, explicit interfaces, and operational safeguards. The `Dataset`/`Case` data model is strongly typed, round-trip serialization is thoroughly tested, golden outputs and metadata schemas are user-definable via generics, and there are real safeguards (duplicate-case-name rejection, `$schema` sidecars, `extra='forbid'`). It stops short of 8–10 because dataset *versioning* is absent from the data model (only example-level filename suffixes), there is no content fingerprint/hash to detect dataset mutation, reports have no first-class save/load API, and reproducibility relies on user discipline (experiment metadata is advisory and can silently disagree with actual task config, as the docs admit).

## Evidence Collected

Every entry includes a file path with line numbers. Paths are relative to the study directory; source root is `studies/agent-harness-study/sources/pydantic-ai/`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Dataset format | `Case` dataclass: `name`, `inputs`, `metadata`, `expected_output`, per-case `evaluators` | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:110-174 |
| Dataset container | `Dataset(BaseModel)` with `name`, `cases`, dataset-level `evaluators`, `report_evaluators` | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:177-236 |
| Serialization schema | Internal `_CaseModel` / `_DatasetModel` with `$schema` alias and `extra='forbid'` | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:89-107 |
| File formats | `from_file`/`from_text`/`from_dict` (YAML+JSON), format inferred from extension | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:556-665, 877-900 |
| Save + schema sidecar | `to_file` writes YAML/JSON plus generated JSON schema; `DEFAULT_SCHEMA_PATH_TEMPLATE = './{stem}_schema.json'` | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:747-794, 77-80 |
| IDE schema hook | YAML first line `# yaml-language-server: $schema=...` emitted on save | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:81, 787-789 |
| Golden answers | `expected_output` documented as expected task output used by comparison evaluators | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:141-142 |
| Golden-output evaluators | `EqualsExpected` returns `{}` when no expected output; exact equality otherwise | sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:46-58 |
| More golden checks | `Equals`, `Contains` (str/list/dict/model-aware), `IsInstance` type check | sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:32-43, 72-152, 155-174 |
| Default evaluator registry | `DEFAULT_EVALUATORS` tuple incl. `LLMJudge`, `GEval`, agentic evaluators | sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:358-372 |
| Serializable evaluator refs | `EvaluatorSpec` supports name-only, single-arg short form, and kwargs dict forms | sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/spec.py:8-18 |
| Task metadata schema | `metadata: MetadataT \| None` generic; user supplies schema (e.g., `TaskMetadata(difficulty='easy', category='general')` in tests) | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:136-140; sources/pydantic-ai/tests/evals/test_dataset.py:133-150 |
| Integrity safeguard | Duplicate case names rejected at construction and in `add_case` | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:254-260, 495-496 |
| Versioned example datasets | Committed `time_range_v1.yaml` and `time_range_v2.yaml`; v2 adds case-level `AgentCalledTool` and dataset-level `ValidateTimeRange`/`UserMessageIsConcise` evaluators | sources/pydantic-ai/examples/pydantic_ai_examples/evals/datasets/time_range_v1.yaml:1; sources/pydantic-ai/examples/pydantic_ai_examples/evals/datasets/time_range_v2.yaml:47-49, 108-112 |
| Dataset generation via LLM | `generate_dataset()` builds cases from a JSON schema with an agent, saves once to file | sources/pydantic-ai/pydantic_evals/pydantic_evals/generation.py:33-87 |
| Generation workflow example | `example_01_generate_dataset.py` writes `datasets/time_range_v1.yaml` as a one-shot step | sources/pydantic-ai/examples/pydantic_ai_examples/evals/example_01_generate_dataset.py:41-44 |
| Round-trip tests | YAML and JSON save/load round trips assert case fidelity and `$schema` presence | sources/pydantic-ai/tests/evals/test_dataset.py:971-1052 |
| Discriminator fidelity | Union-typed inputs survive round trip via inline snapshot equality | sources/pydantic-ai/tests/evals/test_dataset.py:1055-1084 |
| Repeat runs | `repeat` param duplicates cases as `'case [i/n]'` with `source_case_name` key | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:269-279, 292, 317-318; verified in sources/pydantic-ai/tests/evals/test_multi_run.py:79-118 |
| Multi-run aggregation | `EvaluationReport.case_groups()` groups runs per source case; `averages()` aggregates | sources/pydantic-ai/pydantic_evals/pydantic_evals/reporting/__init__.py:343-392 |
| Retries | `retry_task` / `retry_evaluators` (tenacity-backed) stabilize flaky runs | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:288-289, 989-995 |
| Trace linkage | Report/case carry `trace_id`/`span_id`; experiment span gets metadata + pass-rate attributes | sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:377-382, 396-403, 1044-1060; sources/pydantic-ai/pydantic_evals/pydantic_evals/reporting/__init__.py:115-118, 338-341 |
| Report serialization adapters | `EvaluationReportAdapter` / `ReportCaseAdapter` TypeAdapters enable external persistence | sources/pydantic-ai/pydantic_evals/pydantic_evals/reporting/__init__.py:151-152, 726 |
| Evaluator versioning (online) | `get_evaluator_version()` stamps `evaluator_version` onto results/failures; OTel attr `gen_ai.evaluation.evaluator.version` | sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/evaluator.py:185-193, 70-81, 111-114; sources/pydantic-ai/pydantic_evals/pydantic_evals/_otel_emit.py:51, 172-181 |
| Reproducibility guidance | Docs: experiment metadata records model/prompt version; mismatch between claimed and actual config causes "unreproducible results"; shared-config patterns recommended | sources/pydantic-ai/docs/evals/how-to/metrics-attributes.md:443-472, 509-551 |
| Nondeterminism acknowledged | "LLM judges are not deterministic. The same output may receive different scores across runs." | sources/pydantic-ai/docs/evals/evaluators/llm-judge.md:534 |
| Model-comparison benchmark style | `example_04_compare_models.py` evaluates same v2 dataset under two pinned models with named experiments | sources/pydantic-ai/examples/pydantic_ai_examples/evals/example_04_compare_models.py:25-34 |

## Answers to Dimension Questions

### 1. How are datasets managed?

Datasets are managed through a typed, code-first model with declarative file persistence. A `Dataset[InputsT, OutputT, MetadataT]` holds `Case` objects and can be created programmatically, extended via `add_case`/`add_evaluator` (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:475-533`), loaded from YAML/JSON/dict (`from_file`/`from_text`/`from_dict`, sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:556-665), and saved back with `to_file` (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:747-794). Evaluators referenced inside dataset files are resolved through a registry keyed by serialization name, mixing built-ins with user-supplied `custom_evaluator_types`; unknown names raise grouped errors at load time (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:686-745`, registry construction at `sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:1310-1349`). The repo's own eval suite demonstrates the intended lifecycle: generate once with an LLM (`sources/pydantic-ai/examples/pydantic_ai_examples/evals/example_01_generate_dataset.py:41-44`), commit the YAML, then load it in later examples (`sources/pydantic-ai/examples/pydantic_ai_examples/evals/example_04_compare_models.py:26-29`).

### 2. Are datasets versioned?

Not at the framework level. The serialized dataset model contains only `$schema`, `name`, `cases`, `evaluators`, `report_evaluators` — no version field (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:99-107`). Version control is the implied mechanism ("YAML … great for version control", `sources/pydantic-ai/docs/evals/how-to/dataset-serialization.md:9`), and the repo's examples follow a filename-suffix convention: `datasets/time_range_v1.yaml` vs `datasets/time_range_v2.yaml`, where v2 extends v1 with stricter evaluators rather than mutating v1 in place (evidence cited above). What does exist is adjacent versioning machinery: per-evaluator version tags (`get_evaluator_version`, `sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/evaluator.py:185-193`) propagated into results and OTel attributes so online dashboards can filter retired judge versions, and docs recommending `prompt_version`-style keys inside free-form experiment metadata (`sources/pydantic-ai/docs/evals/how-to/metrics-attributes.md:448-450`). The `$schema` sidecar pins dataset *structure* per generic type but carries no content version.

### 3. Are expected outputs defined?

Yes, explicitly and per case. `Case.expected_output` is the canonical golden-answer slot (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:141-142`), carried into every evaluator context (`_run_task`, sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:1000-1010) and echoed into report cases for auditability (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:1170-1174). Enforcement is pluggable: `EqualsExpected` compares output to `expected_output` and self-disables (returns `{}`) when the golden output is absent (`sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:52-55`); `Equals`/`Contains`/`IsInstance` cover fixed values, substring/key containment, and type contracts (`sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:32-43, 72-152, 155-174`); `LLMJudge` can include the expected output in its rubric judgment (`include_expected_output`, `sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:235, 258-263`). The committed example dataset shows realistic golden definitions including structured success payloads, error messages, and LLM-judge rubrics (`sources/pydantic-ai/examples/pydantic_ai_examples/evals/datasets/time_range_v1.yaml:7-10, 51-60, 24-25`).

### 4. Are benchmarks reproducible?

Partially, by design and by convention. On the harness side, reproduction is well supported: datasets round-trip byte-stably between files and objects (tests at `sources/pydantic-ai/tests/evals/test_dataset.py:971-1084`), runs can be repeated N times with per-run indexing and aggregation to expose variance instead of hiding it (`repeat`, sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:269-279; `case_groups`/`averages`, sources/pydantic-ai/pydantic_evals/pydantic_evals/reporting/__init__.py:343-392), transient failures can be retried (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:989-995`), and every report embeds OTel `trace_id`/`span_id` plus experiment metadata for later correlation (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:377-382, 1044-1060). Model comparisons pin the model explicitly per run (`sources/pydantic-ai/examples/pydantic_ai_examples/evals/example_04_compare_models.py:31-34`). On the workload side, reproducibility is deliberately the user's problem: LLM judges are documented as nondeterministic (`sources/pydantic-ai/docs/evals/evaluators/llm-judge.md:534`), the default judge model is an implicit moving dependency (`openai:gpt-5.2` default, `sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:226-230`), and the docs warn that experiment metadata describing configuration that differs from what the task actually used produces "incorrect experiment tracking and unreproducible results," recommending shared-constant/config-object patterns as mitigation (`sources/pydantic-ai/docs/evals/how-to/metrics-attributes.md:509-551`). A result produced today can be re-derived six months later only if the user pinned model, judge, prompts, and dataset in VCS — the library provides the hooks, not the guarantee.

## Architectural Decisions

- **Datasets as typed Pydantic models, cases as dataclasses.** `Dataset` extends `BaseModel` (serialization/validation) while `Case` is a plain dataclass (`sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:110-111, 177`); internal `_CaseModel`/`_DatasetModel` bridge the two for file IO with `extra='forbid'` rejecting unknown fields (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:89-107). This gives schema-validated persistence without burdening in-memory usage.
- **Golden answers stored on the case, enforcement in evaluators.** Separation of data (`expected_output`) from policy (`EqualsExpected`, judges) lets the same dataset serve strict regression gates and fuzzy quality scoring without schema changes.
- **Evaluators referenced by name in files, resolved via registry.** `EvaluatorSpec` short forms (`sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/spec.py:14-18`) keep dataset files human-editable; registries merge defaults with caller-supplied types and fail loudly with grouped errors on unknown names (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:686-745).
- **Schema sidecar as structural contract.** Every `to_file` emits a per-type JSON schema and wires it into the YAML header / JSON `$schema` key (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:769-794), making editor-time validation the primary defense against malformed contributions — a lightweight stand-in for a formal dataset version.
- **Version tags on evaluators, not datasets.** Versioning effort targeted online evaluation attribution (`get_evaluator_version` → `evaluator_version` → `gen_ai.evaluation.evaluator.version`), reflecting the product's Logfire-centric observability posture rather than offline benchmark archiving.
- **Nondeterminism made measurable rather than suppressed.** `repeat` with run-indexed names and `source_case_name` aggregation (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:269-279) treats stochastic tasks as first-class, instead of forcing seeds.

## Notable Patterns

- **Filename-suffix dataset evolution**: `time_range_v1.yaml` → `time_range_v2.yaml` preserves the old golden set while extending it with new evaluators (`AgentCalledTool`, `ValidateTimeRange`, `UserMessageIsConcise` in v2 only) — an exemplar of immutable-versioned eval suites living entirely at the VCS layer.
- **Self-describing datasets in tests**: the test suite's own `TaskMetadata(difficulty=..., category=...)` fixture (`sources/pydantic-ai/tests/evals/test_dataset.py:133-150`) doubles as the reference pattern for task-metadata schemas: fully user-defined, generic, validated by Pydantic.
- **Round-trip snapshot testing**: union-typed inputs are asserted equal after a YAML cycle via `inline_snapshot` (`sources/pydantic-ai/tests/evals/test_dataset.py:1055-1084`), pinning serialization fidelity against silent drift.
- **Metadata-as-span-attributes**: experiment metadata, repeat count, assertion pass rate, analyses, and report-evaluator failures are all mirrored onto the evaluation OTel span (`_set_experiment_span_attributes`, sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:1044-1080), making benchmark runs queryable alongside traces.
- **Guarded removal of dangerous features**: the old `Python` evaluator (arbitrary-code golden checks) raises a descriptive ImportError pointing to the removing PR (`sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:375-380`) — an integrity stance relevant to trusting committed datasets.

## Tradeoffs

- **Convention over mechanism for versioning**: filename suffixes keep the core simple but provide no machine-readable version, no deprecation path, and no way to bind a report to the exact dataset revision beyond file contents in git at that time.
- **Free-form experiment metadata**: maximally flexible (`dict[str, Any]`, sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:291), but nothing enforces that recorded values match actual task configuration — the docs dedicate a whole section to this footgun (`sources/pydantic-ai/docs/evals/how-to/metrics-attributes.md:509-516`).
- **Optional case names**: ergonomic, yet unnamed cases get synthetic `Case {i}` labels (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:276-279), weakening cross-experiment comparability precisely where datasets grow organically.
- **YAML-first ergonomics vs JSON strictness**: dual-format support covers both audiences, but only JSON embeds `$schema` as data while YAML relies on a comment line (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:783-794) — tooling parsing YAML datasets must special-case the header.
- **LLM-generated seed datasets**: `generate_dataset` accelerates authoring (sources/pydantic-ai/pydantic_evals/pydantic_evals/generation.py:33-87) but injects model-dependent quality into golden sets; the framework mitigates only by validating output against the schema before saving.

## Failure Modes / Edge Cases

- **Unknown evaluator in a committed dataset** surfaces only at load time as an `ExceptionGroup` of up to three errors (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:738-739) — CI catches it, editors catch it earlier only if the schema sidecar is present and current.
- **Stale schema sidecar**: `_save_schema` rewrites the `.json` schema only when content differs (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:863-864), but nothing detects a dataset file edited without regenerating its schema; IDE validation then validates against yesterday's shape.
- **Silent golden-answer absence**: `EqualsExpected` returning `{}` for missing `expected_output` (`sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:52-55`) means a typo'd or stripped golden field quietly converts an assertion-bearing case into a metrics-only case rather than failing.
- **Judge-model default drift**: benchmarks using `LLMJudge` without an explicit model inherit whatever default judge ships with the installed version (`openai:gpt-5.2`, sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:226-230), so upgrading the library can move scores without any dataset change — acknowledged indirectly by `set_default_judge_model`.
- **Teardown exceptions crash the run**: lifecycle teardown errors intentionally propagate and can abort an entire evaluation sweep (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:1205-1211), trading partial results for loud failure.
- **Duplicate names rejected early** (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:254-260) — good — but renamed-by-repeat report cases (`'case [2/3]'`) require `source_case_name` bookkeeping everywhere downstream comparisons happen.

## Future Considerations

- Add an optional, machine-readable `version` (or content hash) to `_DatasetModel` and stamp it into `EvaluationReport`/OTel attributes, closing the loop between dataset revision and result provenance.
- Provide `EvaluationReport.to_file`/`from_file` on par with datasets; the TypeAdapter building blocks already exist (`sources/pydantic-ai/pydantic_evals/pydantic_evals/reporting/__init__.py:151-152, 726`) but users hand-roll persistence today.
- Support pinning the judge model/version inside dataset files (e.g., dataset-level judge config) so committed golden sets remain score-stable across library upgrades.
- Emit a warning when `EqualsExpected` is active dataset-wide but individual cases lack `expected_output`, converting the silent skip (`sources/pydantic-ai/pydantic_evals/pydantic_evals/evaluators/common.py:52-55`) into a reviewable signal.
- Optional dataset-content fingerprinting at load time (hash of normalized cases) to detect accidental edits in VCS-reviewed files.

## Questions / Gaps

- **No evidence found** of any dataset-level version identifier, changelog, or migration tooling within `pydantic_evals`; searches across `pydantic_evals/` for `version` returned only Python-version checks (sources/pydantic-ai/pydantic_evals/pydantic_evals/dataset.py:56), package build versioning (`sources/pydantic-ai/pydantic_evals/pyproject.toml:2-15`), and evaluator-version machinery. The conclusion "versioning is VCS + filename convention" rests on the examples directory being representative of intended practice.
- **No evidence found** of a maintained benchmark corpus or leaderboard pipeline inside the repo (no `benchmark` matches in `pydantic_evals/` or `docs/evals/`); the closest artifacts are the four numbered example scripts under `examples/pydantic_ai_examples/evals/`.
- Whether downstream users (e.g., Logfire experiment views) reconstruct dataset identity from `trace_id` alone could not be verified from this source tree; the report carries trace/span IDs (`sources/pydantic-ai/pydantic_evals/pydantic_evals/reporting/__init__.py:338-341`) but no dataset revision pointer.

---

Generated by `dimensions/18.01-dataset-and-golden-task-management.md` against `pydantic-ai`.
