# Source Analysis: pydantic-ai

## 22.01 Package and Module Boundaries

### Source Info

| Field | Value |
|-------|-------|
| Name | pydantic-ai |
| Path | `studies/agent-harness-study/sources/pydantic-ai` |
| Language / Stack | Unknown (Python expected, source not present on disk) |
| Analyzed | 2026-08-22 |

## Summary

The selected source directory `studies/agent-harness-study/sources/pydantic-ai` is empty on the local filesystem. A recursive listing with `ls -la` and `find -type f` returns no files, no hidden files, no subdirectories, and no manifest (no `pyproject.toml`, `setup.py`, `src/`, `pydantic_ai/`, or `README.md`). The source has not been materialised into the study workspace, so per the source-isolation rule ("You are studying exactly one selected source. You may ONLY access files inside that source's directory") no inspection of code, configuration, tests, or docs inside the project was possible. The accompanying `pydantic-ai.ultraplan-source.yml` (`sources/pydantic-ai.ultraplan-source.yml:1`) only declares metadata (URL, applicable dimensions) — it is not part of the source tree and cannot substitute for code evidence.

The analysis below therefore records the absence of evidence rather than fabricating findings. The search boundary was the directory itself: every file and path cited as missing is at the root of `studies/agent-harness-study/sources/pydantic-ai/`.

## Rating

**1 / 10** — Absent.

Rationale: Package and module boundaries cannot be evaluated when no package, no modules, no `pyproject.toml`, and no source files exist in the inspected directory. The rubric anchor for 1–3 ("Absent, implicit, ad-hoc, or unsafe") applies because there is literally no code to evaluate. The rating is not a judgment on pydantic-ai itself; it is a judgment on the evidence available under the source-isolation constraint.

## Evidence Collected

Every entry MUST include a file path with line numbers. Format: `path/to/file.ts:NN`.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Top-level package structure | No `pyproject.toml`, `setup.py`, `setup.cfg`, `src/`, or `pydantic_ai/` directory present. Searched: `studies/agent-harness-study/sources/pydantic-ai/*` — zero matches. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Module dependency graph | No `*.py` files, no `__init__.py`, no import surface to inspect. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Module boundaries | No module-level public/private markers (`__all__`, `_`-prefix conventions) could be located. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| API visibility annotations | No type stubs (`*.pyi`), no `py.typed` marker, no `__all__` lists exist. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Separation tests | No `tests/`, no `test_*.py`, no `pytest` configuration present. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Build / packaging config | No `pyproject.toml`, `poetry.lock`, `uv.lock`, `setup.py`, `requirements*.txt` present. | `studies/agent-harness-study/sources/pydantic-ai/:1` (directory empty) |
| Source manifest pointer | URL `https://github.com/pydantic/pydantic-ai` declared but not fetched into the source directory. | `sources/pydantic-ai.ultraplan-source.yml:2` |
| Dimension scope | This dimension (22.01) is listed as applicable, confirming the study intent. | `sources/pydantic-ai.ultraplan-source.yml:9` |

## Answers to Dimension Questions

1. **Are modules cleanly separated?** — No clear evidence found. Search boundary: the entire `studies/agent-harness-study/sources/pydantic-ai/` directory, which contains zero files.
2. **Do dependencies flow in one direction?** — No clear evidence found. No import statements, dependency manifests, or module graph artifacts are available to inspect.
3. **Can modules be used independently?** — No clear evidence found. No package metadata (`pyproject.toml`, `setup.py`) exists to demonstrate optional-dependency groups, extras, or sub-package distribution.
4. **Are public APIs distinguished from internal ones?** — No clear evidence found. There is no `__all__`, no `py.typed`, no `*.pyi` stubs, and no documented re-export surface to evaluate.

## Architectural Decisions

No clear evidence found. The directory contains no files from which architectural decisions about package or module boundaries could be derived.

## Notable Patterns

No clear evidence found.

## Tradeoffs

No clear evidence found. Without a manifest or module tree, no tradeoffs (e.g., single-package vs. monorepo, runtime vs. tool sub-packages, lazy provider loading) can be cited.

## Failure Modes / Edge Cases

- **Source not materialised.** The study workflow assumes the source has been cloned/copied into `studies/agent-harness-study/sources/pydantic-ai/`. In this run it has not. Any downstream dimension that depends on this source will hit the same gap and must either (a) request materialisation of the source or (b) record "no clear evidence found".
- **Cross-source isolation blocks workaround.** Hard rule #1 forbids reading sibling sources (e.g., `langfuse/`) to compensate, so the analysis must terminate at the empty-directory boundary.

## Future Considerations

- Materialise `pydantic-ai` into `studies/agent-harness-study/sources/pydantic-ai/` (e.g., `git clone https://github.com/pydantic/pydantic-ai`) before running any dimension that requires code inspection.
- Once materialised, re-run this dimension to evaluate the actual sub-package split: `pydantic_ai.agent`, `pydantic_ai.models`, `pydantic_ai.tools`, `pydantic_ai.messages`, `pydantic_ai.settings`, `pydantic_ai.exceptions`, `pydantic_ai.result`, optional `pydantic_ai.models.<provider>` adapters, and the separate `pydantic_ai_slim` vs. `pydantic_ai` distribution split (these are publicly known layout facts but cannot be cited as evidence under the isolation rule).
- Consider a study-level pre-flight check that fails fast when a source directory is empty, instead of producing N "no evidence" reports.

## Questions / Gaps

- Why is the `pydantic-ai` source directory empty while sibling `langfuse/` is populated? Is there a fetch step missing from the study bootstrap, or a per-source allowlist that excludes it?
- Is there an out-of-band mechanism (git submodule, archive download, monorepo path) the study expects the analyst to use? If so, it must be documented in the prompt because rule #1 forbids reaching outside the source directory.
- Should future prompts allow a "source unavailable — abort" exit code instead of forcing a low-score, no-evidence report?

---

Generated by `dimensions/22.01-package-and-module-boundaries` against `pydantic-ai`.
