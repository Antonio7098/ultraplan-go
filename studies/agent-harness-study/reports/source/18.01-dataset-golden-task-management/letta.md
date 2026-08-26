# Source Analysis: letta

## 18.01 Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | letta |
| Path | `studies/agent-harness-study/sources/letta` |
| Language / Stack | Python (FastAPI server, pytest, GitHub Actions CI; package version 0.16.8 per `pyproject.toml:3`) |
| Analyzed | 2026-08-26 |

## Summary

Letta has no dedicated `evals/` or `benchmarks/` directory. Its closest thing to a benchmark suite is a **model-sweep framework** living inside CI infrastructure (`.github/scripts/model-sweep/model_sweep.py`), which tests each live LLM's ability to drive the Letta agent loop (greetings, tool calls, streaming, multimodal input, summarization). The "dataset" is implicitly the corpus of ~45 per-model JSON config files under `tests/configs/llm_model_configs/` (`model_sweep.py:33-38`, `model_sweep.py:92-106`), crossed with ~16 test functions and dynamically re-parametrized at collection time against every model the running server exposes (`.github/scripts/model-sweep/conftest.py:273-278`). Results are serialized via `pytest-json-report` (`pyproject.toml:123`), converted to a ranked Markdown report (`generate_model_sweep_markdown.py`), committed to the repo via an automated PR, and archived as workflow artifacts.

Golden outputs exist in three forms: (1) golden tool-schema pairs — input `.py` sources with expected `.json` schemas and structured-output variants in `tests/test_tool_schema_parsing_files/`; (2) behavioral assertion helpers that pin exact message-type sequences rather than free-text answers (`model_sweep.py:109-146`); (3) an exact-payload equality test requiring the preview path to byte-match the step path (`tests/test_preview_accuracy.py:173`). Task metadata is minimal: a single flat mapping of test names to three feature categories in `.github/scripts/model-sweep/feature_mappings.json`. There is **no formal dataset versioning** — no Git LFS, DVC, checksums, or version fields. Version identifiers survive only as conventions: version-encoded legacy directory names now orphaned (`tests/data/memgpt-0.2.11/`, `tests/data/memgpt-0.3.17/`) and dated report snapshots (`supported-models.mdx` front-matter `generated: 2025-06-20T16:40:44`). Reproduction requires a self-hosted runner plus internal secrets injection (`letta_secrets_helper`, `.github/workflows/model-sweep.yaml:60`), so external contributors cannot rerun the sweep out of the box, and results are inherently non-deterministic because they exercise live provider models.

## Rating

**4 / 10** — Present but inconsistent, weakly documented, and fragile.

The benchmark machinery itself is real and operational (CI-dispatched, artifact-backed, auto-published), which lifts it above ad-hoc. But dataset versioning is absent, task taxonomy is a single flat 3-category JSON file with no difficulty ratings or stable task IDs, the primary eval inputs (live provider models) cannot be pinned, and local reproduction depends on internal infrastructure. Against the rubric question "Can a benchmark result be reproduced six months later?" the honest answer is only partially: historical *reports* persist (committed MDX + git history of auto-PR branches), but the *runs* behind them are not reproducible — model availability, provider behavior, and runner infrastructure have all moved on, and no snapshot pins what was actually executed.

## Evidence Collected

Every entry includes a file path with line numbers.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Eval runner | `get_llm_config()` loads per-model JSON configs from `tests/configs/llm_model_configs`; hardcoded model list `all_configs` with commented-out entries and env override `LLM_CONFIG_FILE` | `.github/scripts/model-sweep/model_sweep.py:33-38`, `.github/scripts/model-sweep/model_sweep.py:92-106` |
| Dynamic dataset expansion | `pytest_generate_tests()` parametrizes every test over all models returned by the live server (`client.models.list()`), not just the static file list | `.github/scripts/model-sweep/conftest.py:255-269`, `.github/scripts/model-sweep/conftest.py:273-278` |
| Behavioral goldens | `assert_greeting_with_assistant_message_response()` asserts exact message-type sequence (ReasoningMessage → AssistantMessage) and OTID suffix conventions, not text content beyond one pinned string | `.github/scripts/model-sweep/model_sweep.py:54-56`, `.github/scripts/model-sweep/model_sweep.py:109-144` |
| Task categories | `feature_mappings.json` maps 16 test names to exactly 3 categories: "Basic" (10), "Token Streaming" (4), "Multimodal" (2); no difficulty, tags, or IDs | `.github/scripts/model-sweep/feature_mappings.json:1-21` |
| Report generation & scoring | `load_feature_mappings()`, `categorize_tests()`, position-weighted `calculate_support_score()` (✅=10×weight, ⚠️=5×, ❌=1×) | `.github/scripts/model-sweep/generate_model_sweep_markdown.py:9-24`, `.github/scripts/model-sweep/generate_model_sweep_markdown.py:52-62`, `.github/scripts/model-sweep/generate_model_sweep_markdown.py:65-88` |
| Committed benchmark artifact | Dated report snapshot: front-matter `generated: 2025-06-20T16:40:44`, "Ran 2464 tests against 154 models across 7 providers" | `.github/scripts/model-sweep/supported-models.mdx:1-8` |
| CI reproduction pipeline | Workflow dispatches pytest with `--json-report`, converts to MDX, commits to branch `model-sweep/<date>` and opens automated PR, uploads raw JSON artifact | `.github/workflows/model-sweep.yaml:94-98`, `.github/workflows/model-sweep.yaml:113-135`, `.github/workflows/model-sweep.yaml:137-142` |
| Reproduction barrier | Requires `[self-hosted, medium]` runner and internal secret injector `letta_secrets_helper --env dev --service ci` | `.github/workflows/model-sweep.yaml:11`, `.github/workflows/model-sweep.yaml:51-60` |
| Golden schema pairs | `_run_schema_test()` loads `{name}.py`, derives schema via `derive_openai_json_schema`, compares against golden `{name}.json` and structured-output `{name}_so.json`; `_compare_schemas()` pretty-prints diffs on mismatch | `tests/test_tool_schema_parsing.py:35-55`, `tests/test_tool_schema_parsing.py:58-88` |
| Golden base-tool schemas | Hand-written expected schemas for memory tools (`get_rethink_user_memory_schema`, `get_search_memory_schema`, etc.) | `tests/test_tool_schema_parsing_files/expected_base_tool_schemas.py:1-68` |
| Exact-payload golden | `TestPreviewAccuracy` asserts `preview_payload == step_payload` — preview request must byte-match actual step request | `tests/test_preview_accuracy.py:79`, `tests/test_preview_accuracy.py:173` |
| Model-config dataset | ~45 JSON files defining context window, model name, endpoint type per LLM; same files feed CI matrices via `LLM_CONFIG_FILE` | `tests/configs/llm_model_configs/*.json`; `.github/workflows/reusable-test-workflow.yml:413-432` |
| Send-message eval matrix | Integration matrix runs the same config-file datasets across 5 models (`openai-gpt-4o-mini`, `openai-gpt-4.1`, `openai-gpt-5`, `claude-4-5-sonnet`, `gemini-2.5-pro`) | `.github/workflows/send-message-integration-tests.yml:34-46` |
| Data fixtures | Upload fixtures incl. `toy_chat_fine_tuning.jsonl` (JSONL chat messages), `list_tools.json` (144 KB tool payload), embeddings JSON, PDFs/images for ingestion tests | `tests/data/toy_chat_fine_tuning.jsonl` used at `tests/test_sources.py:178`; `tests/test_sources.py:606` |
| Legacy versioned snapshots | Version-encoded directories `memgpt-0.2.11/` (exported agent states, pickled persistence managers) and `memgpt-0.3.17/sqlite.db`; grep across repo finds zero code references — orphaned convention | `tests/data/memgpt-0.2.11/agents/agent_test/config.json`; `tests/data/memgpt-0.3.17/sqlite.db` |
| Agent export format | `.af` fixtures (12 files); `AgentFileSchema.metadata` is a free-form `Dict[str, str]` ("including revision_id") with no explicit format-version field | `tests/test_agent_files/deep-thought.af`; `letta/schemas/agent_file.py:431-445` |
| Perf/load harnesses | Throughput tests with matplotlib/pandas reporting; Locust user class — measurement harnesses, not golden-task suites | `tests/performance_tests/test_agent_mass_creation.py`; `tests/locust_test.py` |

## Answers to Dimension Questions

### 1. How are datasets managed?

Ad hoc, embedded in the test tree rather than a curated dataset layer. The effective benchmark dataset is the cross-product of (a) static per-model config files (`tests/configs/llm_model_configs/*.json`, consumed by `model_sweep.py:33-38,92-106`), (b) dynamically discovered live models (`conftest.py:260-261,273-278`), and (c) inline prompt constants defined directly in the runner (`USER_MESSAGE_RESPONSE`, `USER_MESSAGE_ROLL_DICE`, image URLs at `model_sweep.py:54-91`). Document-ingestion fixtures sit in `tests/data/` (`tests/test_sources.py:178,606`). No manifest, index, or loader abstraction ties these together; adding a model means editing a Python list literal (`all_configs`, `model_sweep.py:92-103`).

### 2. Are datasets versioned?

No formal mechanism. Searches found no Git LFS pointers, no DVC, no checksum manifests, and no version fields inside any data file. The only versioning signals are incidental:

- Version-encoded directory names from the MemGPT era (`tests/data/memgpt-0.2.11/agents/agent_test/config.json`, `tests/data/memgpt-0.3.17/sqlite.db`) — both now referenced by zero code, evidence of an abandoned convention.
- Report snapshots carry generation timestamps (`supported-models.mdx:3`), and each run's raw JSON is preserved only as a GitHub Actions artifact (`.github/workflows/model-sweep.yaml:137-142`) plus git history of the `model-sweep/<date>` auto-PR branch (`.github/workflows/model-sweep.yaml:117`).
- The package version (`pyproject.toml:3`) versions code, not data; `AgentFileSchema` has a free-form metadata dict but no format-version field (`letta/schemas/agent_file.py:442-444`).

### 3. Are expected outputs defined?

Yes, but heterogeneous in strictness:

- **Structural goldens**: tool-schema derivation is checked against committed expected JSON files for every case, including structured-output variants (`tests/test_tool_schema_parsing.py:58-88`), with diff-printing on failure (`tests/test_tool_schema_parsing.py:46-53`). Base-tool schemas are hand-written expectations (`tests/test_tool_schema_parsing_files/expected_base_tool_schemas.py:1-68`).
- **Behavioral goldens**: model-sweep assertions pin message-type sequences, counts, OTID suffixes, token-stat presence, and one canned string ("Teamwork makes the dream work", `model_sweep.py:55`) — deliberately tolerant of free-text variance.
- **Byte-exact goldens**: preview requests must equal step requests exactly (`tests/test_preview_accuracy.py:173`).
- There is no snapshot-testing library and no stored transcripts of real LLM responses as goldens.

### 4. Are benchmarks reproducible?

Only structurally, not semantically. The pipeline is fully automated end-to-end (`workflow_dispatch` → services → pytest → report → PR, `.github/workflows/model-sweep.yaml:2-7,94-135`), and the send-message matrix makes regular CI consumption of the same config dataset routine (`.github/workflows/send-message-integration-tests.yml:34-46`). However: (1) execution requires self-hosted runners and an internal secrets binary (`model-sweep.yaml:11,60`); (2) the subjects under test are live third-party models whose behavior and availability change without notice — nothing pins them; (3) the dynamic parametrization means the task set depends on whatever models the server happens to expose at run time (`conftest.py:273-278`), so two runs on different days execute different task sets. A result can be *re-read* six months later (committed MDX + artifacts) but not *re-run* faithfully.

## Architectural Decisions

1. **Benchmarks live in CI, not in the package.** The model sweep sits under `.github/scripts/` rather than a top-level evals module, signaling it is an internal quality-gate for provider compatibility, not a public evaluation product.
2. **Config-as-dataset.** Per-model JSON config files double as both unit-test fixtures and the benchmark's model axis (`tests/configs/llm_model_configs/` + `LLM_CONFIG_FILE` override at `model_sweep.py:104-105` and `reusable-test-workflow.yml:415`), giving one source of truth for "which models do we support testing."
3. **Report-as-artifact-as-commit.** Benchmark output flows JSON artifact → deterministic Markdown transform (`generate_model_sweep_markdown.py:213` pipeline) → git-committed MDX via bot PR (`.github/workflows/model-sweep.yaml:113-135`), making the latest results durable and diffable in-repo while history lives in git.
4. **Golden pairs co-located with tests.** Expected tool schemas ship next to their generator inputs (`{name}.py` ↔ `{name}.json` ↔ `{name}_so.json`, `tests/test_tool_schema_parsing.py:63,70,83`), keeping goldens reviewable in the same PR as parser changes.
5. **Live-server parametrization over static matrices.** Delegating the model axis to `client.models.list()` at collection time (`conftest.py:273-278`) trades reproducibility for coverage of whatever the deployment actually serves.

## Notable Patterns

- **Sequence-shape assertions**: instead of comparing LLM prose, goldens assert structural shape — message type ordering, counts, and OTID suffix digits encoding step/index positions (`model_sweep.py:119-143`). This is a pragmatic determinism strategy for non-deterministic generators.
- **Weighted categorical scoring**: support scores weight earlier feature columns higher and score partial support at half credit (`generate_model_sweep_markdown.py:65-88`), turning pass/fail matrices into a ranked leaderboard.
- **Error-tolerant categorization**: tests ending in `_error` are excluded when computing support status so infra failures don't count against a model (`generate_model_sweep_markdown.py:33-41`).
- **Legacy snapshots left in place**: version-named MemGPT export trees remain in `tests/data/` despite having no consumers — archaeology rather than active fixtures.
- **Continue-on-error reporting**: the sweep tolerates failing tests to still publish partial reports (`.github/workflows/model-sweep.yaml:78,101`).

## Tradeoffs

- **Coverage vs. reproducibility**: dynamic discovery maximizes coverage of served models but makes the executed task set vary run-to-run (`conftest.py:273-278`).
- **Simplicity vs. metadata richness**: the 3-category flat JSON keeps maintenance trivial but forecloses difficulty weighting, flakiness tracking, or per-task ownership (`feature_mappings.json:1-21`).
- **Durability vs. provenance of reports**: committed MDX is durable and human-readable, but it aggregates away raw per-test detail (only the JSON artifact retains it, and artifacts expire).
- **Internal automation vs. community reproduction**: depending on `letta_secrets_helper` and self-hosted runners keeps provider keys safe but makes the headline benchmark unrunnable by outsiders (`.github/workflows/model-sweep.yaml:11,51-60`).
- **Co-location of prompts in code vs. data hygiene**: inline prompt constants make tests readable but mean the "dataset" has no existence independent of the runner source (`model_sweep.py:54-91`).

## Failure Modes / Edge Cases

- **Silent dataset drift**: since `all_configs` is hand-edited and one entry is already commented out ("azure-gpt-4o-mini … TODO: Re-enable", `model_sweep.py:94`), the benchmark's model set decays without any test noticing.
- **Non-reproducible comparisons**: comparing this month's sweep to a six-month-old committed report mixes changes in models, prompts, and harness code with no way to isolate variables — no run manifest records the commit SHA or config versions alongside results (the MDX carries only a timestamp, `supported-models.mdx:3`).
- **Orphaned-fixture rot**: `tests/data/memgpt-0.2.11/` contains pickles and SQLite binaries loaded by nothing; they inflate the repo and mislead newcomers about supported migration paths.
- **Fragile exact-equality goldens**: `preview_payload == step_payload` (`tests/test_preview_accuracy.py:173`) fails on any benign serialization change, and conversely its greenness says nothing about semantic correctness of either payload.
- **Partial-report ambiguity**: `continue-on-error` publishing means a committed report may reflect a partially completed sweep, with no marker distinguishing complete from truncated runs (`.github/workflows/model-sweep.yaml:76-108`).

## Future Considerations

- Add content hashes or a manifest (even a simple generated `datasets.lock` listing SHA-256 of each fixture and config) so a report can record which exact dataset version produced it.
- Record provenance in every published report: commit SHA, workflow run URL, and the resolved list of tested models, addressing the "reproduce six months later" gap more cheaply than full pinning.
- Promote `feature_mappings.json` into a richer task registry (IDs, difficulty, owner, flakiness budget) — the plumbing in `generate_model_sweep_markdown.py:52-62` already consumes it generically.
- Either wire the MemGPT legacy snapshots into a migration test suite or delete them; version-named dead data is worse than none.
- Provide an open-source-friendly path to run a reduced sweep (public keys, hosted runners) so external contributors can validate the published matrix.

## Questions / Gaps

- **No difficulty or category depth**: searched all of `tests/`, `.github/`, and top-level dirs for `*eval*`, `*benchmark*`, `*golden*`, `*difficulty*`, `*category*`; only the 3-category `feature_mappings.json` exists. No evidence of difficulty ratings anywhere.
- **No stored reference answers**: no transcript corpora, snapshot libraries, or recorded provider responses were found; behavioral goldens are entirely procedural (assertion helpers), so historical answer distributions cannot be compared.
- **No local benchmark entry point**: no Makefile/Justfile targets or scripts invoke the sweep outside CI (`scripts/` contains only `migrate_tools.py`, `pack_docker.sh`, `wait_for_service.sh`); whether developers ever run it locally could not be confirmed from the repository alone.
- **Historical sweep runs**: raw past-run JSONs live only as expiring GitHub Actions artifacts; the repository itself retains just the latest MDX snapshot, so longitudinal analysis from the checkout alone is impossible.

---

Generated by `Dimension 18.01: Dataset and Golden Task Management` against `letta`.
