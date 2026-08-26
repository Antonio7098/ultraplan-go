# Source Analysis: langgraph

## Dimension 18.01: Dataset and Golden Task Management

### Source Info

| Field | Value |
|-------|-------|
| Name | langgraph |
| Path | `studies/agent-harness-study/sources/langgraph` |
| Language / Stack | Python (core libs), TypeScript (sdk-js); pyperf for benchmarks, pytest + syrupy for tests |
| Analyzed | 2026-08-26 |

## Summary

LangGraph is an agent-framework monorepo (`libs/checkpoint`, `libs/langgraph`, `libs/prebuilt`, `libs/cli`, `libs/sdk-py`, `libs/sdk-js`; see `AGENTS.md`), not an evaluation harness. It contains **no first-class eval datasets, golden tasks, or expected-answer corpora** in the LLM-eval sense. What exists instead falls into four buckets:

1. **Performance micro-benchmarks** — a `pyperf`-based harness in `libs/langgraph/bench/` with a registry of synthetic graph scenarios whose inputs are generated programmatically at runtime, wired into CI with baseline comparison via ephemeral GitHub Actions cache (`.github/workflows/bench.yml`, `.github/workflows/baseline.yml`).
2. **Golden outputs as test snapshots** — syrupy `.ambr` snapshot files under `libs/langgraph/tests/__snapshots__/` and `libs/prebuilt/tests/__snapshots__/`, version-controlled in git. These are correctness fixtures, not benchmark tasks.
3. **A capability-based conformance suite** for checkpointer implementations (`libs/checkpoint-conformance/`) — the closest analog to "golden task management": a registry of implementations under test, a fixed behavioral spec corpus per capability, and structured pass/fail reporting.
4. **Externalized eval datasets in example notebooks** — LangSmith-hosted datasets created ad hoc in `examples/` notebooks; dataset management is delegated to the LangSmith platform and is not reproducible from the repository alone.

The repo's own threat model confirms benchmarks are non-production surface: ".github/THREAT_MODEL.md:30" lists "Tests, benchmarks, documentation — Not shipped code".

## Rating

**4 / 10** — Present but inconsistent, weakly versioned, and fragile for long-horizon reproduction.

Rationale: A working benchmark harness with CI baseline comparison exists (`make benchmark`, `.github/workflows/bench.yml:41-58`), and expected outputs are rigorously managed where they exist (syrupy snapshots pinned by git; conformance spec assertions). However: benchmark inputs are seeded by unseeded randomness (`random.choices` at `libs/langgraph/bench/__main__.py:106`, `uuid4()`-generated tool names and args at `libs/langgraph/bench/react_agent.py:37-61`); baseline artifacts live only in a rolling GitHub Actions cache keyed to the latest main SHA (`.github/workflows/baseline.yml:32-37`), not committed or tagged; benchmark definitions carry no schema/version identifiers; and there are no task-metadata fields (difficulty, category) anywhere. Six-month reproduction is possible only in the loose sense of re-running the same code on similar hardware; exact numeric comparison against today's baseline will usually be impossible because the cache entry will have aged out or been superseded.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Benchmark harness entry point | `pyperf._runner.Runner` drives all benchmarks; scenarios registered as `(name, async_graph, sync_graph, input)` tuples | `libs/langgraph/bench/__main__.py:6,99-464,467` |
| Synthetic, unseeded inputs | Fan-out scenario input built with `random.choices(...)` at run time; no seed anywhere in bench package | `libs/langgraph/bench/__main__.py:104-108` |
| Non-deterministic fixture identity | React-agent benchmark tool name and tool-call ids/args are fresh `uuid4()` values per construction | `libs/langgraph/bench/react_agent.py:37-41,43-61` |
| Deterministic fake model | `FakeFunctionChatModel(FakeMessagesListChatModel)` replays scripted `AIMessage`s — no network, no LLM cost | `libs/langgraph/bench/react_agent.py:18-35,43-61` |
| Outputs not asserted | Benchmarks only count streamed events (`len([...])`); no correctness check of results | `libs/langgraph/bench/__main__.py:20-33,57-70` |
| Scenario families | fanout_to_subgraph, react_agent, wide_state/wide_dict/pydantic_state (sized 25x300/15x600/9x1200, ± checkpoint), sequential(10/1000), serde allowlist | `libs/langgraph/bench/__main__.py:100-463,519-520`; builders at `libs/langgraph/bench/react_agent.py:17`, `libs/langgraph/bench/sequential.py:8-27` |
| First-event latency subset | Hand-picked graphs for latency measurement ("limiting just due to the size of the annotation on github") | `libs/langgraph/bench/__main__.py:476-497` |
| Compilation benchmarks | Compile-time measurements for three graph shapes | `libs/langgraph/bench/__main__.py:499-516` |
| Make targets | `benchmark` → `python -m bench -o out/benchmark.json --rigorous`; `benchmark-fast` → `--fast`; plus `profile` via py-spy | `libs/langgraph/Makefile:12,17-25,29-31` |
| Baseline production | On push to main: full rigorous run saved to cache key `{os}-benchmark-baseline-{SHA}` | `.github/workflows/baseline.yml:21,30-37` |
| Baseline consumption | PR job restores rolling-key baseline (`fail-on-cache-miss: true`) then compares with `pyperf compare_to --table --group-by-speed`; results posted as PR annotations | `.github/workflows/bench.yml:32-40,49-58,59-75` |
| Manual benchmark-as-test | DeltaChannel benchmark skipped from CI ("slow benchmark — run manually"), prints results rather than asserting thresholds | `libs/langgraph/tests/test_delta_channel_benchmark.py:1-4,276,304-320` |
| Snapshot golden outputs | syrupy amber files pin JSON schemas / mermaid graphs; `--snapshot-warn-unused` configured | `libs/langgraph/tests/__snapshots__/test_pregel.ambr:1-7`; `libs/langgraph/pyproject.toml:51,136-137` |
| Conformance suite purpose | "validates that a BaseCheckpointSaver subclass correctly implements the checkpoint storage contract" | `libs/checkpoint-conformance/README.md:14-16` |
| Capability taxonomy | `Capability` enum; BASE vs EXTENDED capabilities; method-name map used for auto-detection | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/capabilities.py:15-63` |
| Implementation-under-test registry | Module-level `_REGISTRY`; `@checkpointer_test(name=..., skip_capabilities=..., lifespan=...)` decorator | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/initializer.py:16,23-30,59-100` |
| Golden behavior corpus | Per-capability spec modules with named assertion functions, e.g. round-trip equality expectations | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put.py:22-51,54-95`; runner map at `libs/checkpoint-conformance/langgraph/checkpoint/conformance/validate.py:15-42` |
| Structured reporting | `CapabilityReport`/`CapabilityResult` with detected/passed/failed/skipped counts and progress callbacks | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/report.py:17-24,93-105`; orchestration at `libs/checkpoint-conformance/langgraph/checkpoint/conformance/validate.py:74-118` |
| Fixture generators (non-deterministic) | `generate_checkpoint` uses `datetime.now()` timestamps; configs default to `uuid4()` thread ids; delta-chain fixture builds known-shape chains | `libs/checkpoint-conformance/langgraph/checkpoint/conformance/test_utils.py:19-60`; `libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/_delta_fixtures.py:20-50` |
| External dataset usage (examples) | CRAG notebook creates LangSmith dataset + examples ad hoc (`create_dataset`, `create_examples`) and evaluates against it by name | `examples/rag/langgraph_crag_local.ipynb:596-624,811` |
| External dataset usage (simulation eval) | Chatbot simulation eval consumes LangSmith example dicts; validates required input keys on the fly | `examples/chatbot-simulation-evaluation/simulation_utils.py:118-141` |
| Server-side trace-to-dataset linkage | SDK run metadata supports associating runs with a LangSmith dataset example id | `libs/sdk-py/langgraph_sdk/schema.py:117` |
| No local data corpora | Repo-wide search found no checked-in eval data files (only app-config JSON under `tests/example_app/`, `libs/cli/tests/unit_tests/`); searches for `dataset|golden|benchmark` outside the above returned nothing substantive | searched: `golden|dataset|Dataset`, file globs `*.json/*.csv/*.parquet` |

## Answers to Dimension Questions

1. **How are datasets managed?**
   There is no in-repo dataset management. Performance benchmarks generate inputs programmatically inside the benchmark registry tuple definitions (`libs/langgraph/bench/__main__.py:99-464`); conformance tests build checkpoints via generator helpers (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/test_utils.py:19-60`) and chain fixtures (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/_delta_fixtures.py:20-50`). LLM-quality evaluation datasets appear only in `examples/` notebooks, where they are created ad hoc in the external LangSmith service (`examples/rag/langgraph_crag_local.ipynb:618-624`) and consumed by name (`langgraph_crag_local.ipynb:811`). The SDK merely exposes a metadata field linking traces to LangSmith dataset examples (`libs/sdk-py/langgraph_sdk/schema.py:117`).

2. **Are datasets versioned?**
   No explicit version identifiers exist for any dataset-like artifact. Benchmark scenario definitions are implicitly versioned through git history only; there is no schema version, no checksum of inputs, no changelog. Baseline result files are cached under keys like `${{ runner.os }}-benchmark-baseline-${SHA}` (`.github/workflows/baseline.yml:35`) with rolling restore-keys (`-benchmark-baseline-` prefix match, `.github/workflows/bench.yml:36-37`), meaning the "current baseline" floats with main and old baselines age out of cache. By contrast, snapshot golden files are durably versioned in git (`libs/langgraph/tests/__snapshots__/test_pregel.ambr:1` records `serializer version: 1`), but that versioning belongs to syrupy's format, not to a task corpus.

3. **Are expected outputs defined?**
   For benchmarks: no. The runners discard stream content entirely — they wrap `graph.astream(...)`/`graph.stream(...)` in `len([...])` (`libs/langgraph/bench/__main__.py:20-33,57-70`), so any graph that streams the wrong events would still "pass". For correctness testing: yes, strongly. Syrupy snapshots pin exact JSON schemas, serialized graphs, and mermaid diagrams (`libs/langgraph/tests/test_pregel.py:1726-1754` asserts `get_input_jsonschema`, `get_output_jsonschema`, `to_json`, `draw_mermaid` against `snapshot`), and the conformance suite defines precise round-trip expectations such as `test_put_roundtrip` requiring `tup.checkpoint["channel_values"] == {"msg": "hello"}` (`libs/checkpoint-conformance/langgraph/checkpoint/conformance/spec/test_put.py:39-51`). These are unit-level goldens, not benchmark golden answers.

4. **Are benchmarks reproducible?**
   Partially, and only short-term. Reproducibility mechanisms that do exist: fixed scenario shapes and sizes encoded in code (`libs/langgraph/bench/__main__.py:101-463`), a deterministic fake chat model instead of a real LLM (`libs/langgraph/bench/react_agent.py:18-35`), two rigor levels (`--rigorous` / `--fast`, `libs/langgraph/Makefile:17-25`), and a CI diff against the main-branch baseline using `pyperf compare_to --table --group-by-speed` (`.github/workflows/bench.yml:56`). Factors undermining six-month reproduction: (a) inputs embed unseeded `random.choices` and `uuid4()` values (`__main__.py:106`; `react_agent.py:39,51`), so payloads differ byte-for-byte between runs; (b) baselines are stored only in GitHub Actions cache with a rolling key (`.github/workflows/baseline.yml:33-37`) and are machine-dependent (ubuntu-latest runners, Python 3.11 pinned at `.github/workflows/bench.yml:24-29` but hardware unspecified); (c) result JSON is written to an untracked `out/` directory (`libs/langgraph/Makefile:12,18-19`) and never committed. Re-running `make benchmark` reproduces *the procedure*, not *the numbers*.

## Architectural Decisions

- **Benchmarks as first-class package-local scripts, not a framework.** The bench suite lives inside `libs/langgraph/bench/` with its own `__main__.py` registry (`libs/langgraph/bench/__main__.py:99-520`) and Make targets (`libs/langgraph/Makefile:10-31`) — simple, zero-abstraction, colocated with the library it measures. Tradeoff: no cross-library reuse; `prebuilt` declares a `benchmark` phony target in its Makefile (`.PHONY` line, `libs/prebuilt/Makefile:1`) without any implementation visible.
- **Baseline-as-cache rather than baseline-as-artifact.** Baselines are produced on every push to main and consumed by PRs via Actions cache (`.github/workflows/baseline.yml:30-37`, `bench.yml:32-40`) instead of being committed, attached to releases, or stored in object storage. This keeps the repo clean but makes historical comparison impossible beyond cache lifetime.
- **Correctness goldens delegated to snapshot testing and a dedicated conformance package.** Rather than building a task/dataset system, expected outputs are expressed as syrupy snapshots (`libs/langgraph/pyproject.toml:136-137`) and as a published pip-installable conformance suite third parties can run against their own checkpointers (`libs/checkpoint-conformance/README.md:14-24`, registration API at `initializer.py:59-100`).
- **Eval datasets pushed out of the repo entirely.** Example-notebook evals create/consume LangSmith datasets (`examples/rag/langgraph_crag_local.ipynb:596-624`), keeping LLM-eval data management in a hosted platform with its own versioning, at the cost of in-repo reproducibility.

## Notable Patterns

- **Registry-of-tuples benchmark definition.** Every scenario is a `(name, async_graph, sync_graph, input)` tuple in one module-level constant, with sync variants auto-suffixed `_sync` at registration time (`libs/langgraph/bench/__main__.py:470-473`). Adding a benchmark is one tuple; no decorators or discovery magic.
- **Checkpoint/no-checkpoint pairing.** Each stateful scenario has a twin suffixed `_checkpoint` compiled with `InMemorySaver()` (e.g., `fanout_to_subgraph_10x` vs `fanout_to_subgraph_10x_checkpoint`, `libs/langgraph/bench/__main__.py:100-119`), isolating persistence overhead — a deliberate performance-attribution design.
- **Capability detection over configuration.** The conformance suite infers which specs apply by checking whether `BaseCheckpointSaver` methods are overridden (`capabilities.py:90-96`), so the "task list" adapts to each implementation without per-target config files.
- **Opt-in skip metadata.** Implementations can declare `skip_capabilities` and lifecycle hooks at registration (`initializer.py:62-64`), a lightweight metadata schema for known deviations from the golden spec.

## Tradeoffs

- **Speed of iteration vs durability**: generating inputs in code keeps the repo free of binary blobs, but forfeits input stability (unseeded random, `uuid4` identities), which caps measurement precision and blocks exact reruns (`libs/langgraph/bench/__main__.py:104-137`).
- **CI signal vs noise floor**: comparing PR numbers to a possibly-days-old baseline on shared runners produces annotations that can mislead when infra noise dominates (`.github/workflows/bench.yml:49-75` posts results as non-blocking `core.notice`).
- **Platform-managed datasets vs self-contained examples**: LangSmith-hosted eval data gives real versioning/UI, but the notebooks hard-code dataset names and even a workspace-specific compare URL (`examples/rag/langgraph_crag_local.ipynb:618,764`), so outsiders cannot reproduce the eval exactly.

## Failure Modes / Edge Cases

- **Cache miss breaks PR benchmarking**: `fail-on-cache-miss: true` on the baseline restore (`.github/workflows/bench.yml:38`) means the job fails outright if main's baseline expired or the cache was evicted — recovery requires re-running `baseline.yml` manually (it also supports `workflow_dispatch`, `baseline.yml:4`).
- **Silent semantic drift**: because benchmark runners assert nothing about outputs (`libs/langgraph/bench/__main__.py:20-33`), a refactor that changes graph semantics while preserving event counts would go unnoticed by the benchmark suite; only the separate unit/snapshot tests would catch it.
- **First-event latency subset drift risk**: the latency subset is matched by graph *object identity* against tuples (`if graph not in GRAPHS_FOR_1st_EVENT_LATENCY`, `libs/langgraph/bench/__main__.py:484-486`); renaming or rebuilding graphs would silently drop latency measurements.
- **Snapshot churn**: broad snapshot assertions on generated JSON schemas (`test_pregel.py:1751-1754`) mean upstream dependency changes (e.g., pydantic schema output) invalidate many goldens at once; `--snapshot-warn-unused` (`libs/langgraph/pyproject.toml:137`) warns rather than fails on stale entries, allowing gradual rot.
- **Conformance skips can mask gaps**: `skip_capabilities` (`initializer.py:29`) lets an implementation advertise partial compliance without any recorded justification field in the report.

## Future Considerations

- Commit or externally persist benchmark baselines (e.g., attach to release tags) so `pyperf compare_to` remains possible across months, not just across a cache window (`.github/workflows/baseline.yml:32-37`).
- Seed the RNG and fix UUIDs (or derive them deterministically from the scenario name) in `bench/__main__.py:104-108` and `bench/react_agent.py:37-61` to make payloads stable run-to-run.
- Add lightweight metadata to scenarios (category, size tier, owner) — currently only the name string encodes this (`libs/langgraph/bench/__main__.py:99-464`).
- Record a reason string alongside `skip_capabilities` in the conformance registry (`initializer.py:23-30`) and surface it in `CapabilityReport` (`report.py`).
- If example-based evals are meant to be more than tutorials, extract their dataset seeds into checked-in, versioned files instead of inline notebook cells (`examples/rag/langgraph_crag_local.ipynb:596-624`).

## Questions / Gaps

- **No evidence found** for any LLM-quality eval infrastructure (datasets, graders, golden answers, task difficulty/category metadata) inside `libs/`. Searches for `golden|dataset|Dataset` and `benchmark` across the source surfaced only the performance harness, the conformance suite, SDK trace-linkage fields (`libs/sdk-py/langgraph_sdk/schema.py:117`), and example notebooks.
- **No evidence found** for dataset version identifiers, content hashes, or migration/changelog mechanisms for any dataset-like artifact; git history is the only versioning.
- Whether the LangSmith-hosted example datasets are themselves versioned could not be verified from this source; the boundary of analysis was this repository, and LangSmith platform behavior is external to it.
- The `benchmark` target declared in `libs/prebuilt/Makefile:1` appears to have no corresponding implementation in that library (no bench directory found there); whether it is vestigial could not be confirmed beyond absence of files.

---

Generated by `Dimension 18.01: Dataset and Golden Task Management` against `langgraph`.
