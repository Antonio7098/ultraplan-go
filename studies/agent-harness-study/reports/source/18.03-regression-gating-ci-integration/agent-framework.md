# Source Analysis: agent-framework

## Regression Gating and CI Integration (Dimension 18.03)

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Unknown — the selected source directory contains no files (see Evidence Collected) |
| Analyzed | 2026-08-23 |

## Summary

The selected source directory
`studies/agent-harness-study/sources/agent-framework/` contains **zero
files**. There is no code, no CI workflow YAML, no test files, no
configuration, no documentation, no `.git/` history, and no
`package.json` / `pyproject.toml` / `go.mod` / `*.csproj` to inspect
inside the source boundary. Per the study's hard source-isolation
rules, no neighboring source (`langfuse`, `crewai`, `letta`,
`temporal`, `openhands`, `langgraph`, `openai-agents-sdk`,
`pydantic-ai`, `opa`, etc.) and no upstream public repo may be read
to fill the gap, so the dimension's five investigation steps
(`dimensions/18.03-regression-gating-ci-integration.md:9-13`) all
reduce to "no evidence found" against this source.

Because every dimension question
(`dimensions/18.03-regression-gating-ci-integration.md:25-28`) collapses
to the absence branch, the rating is **1/10** ("Absent, implicit,
ad-hoc, or unsafe" per the rubric at
`dimensions/18.03-regression-gating-ci-integration.md:32-34`). The
guiding question at
`dimensions/18.03-regression-gating-ci-integration.md:39` ("Can a code
change that degrades quality be blocked before deployment?") cannot be
answered affirmatively from an empty source tree.

## Rating

**1 / 10 — Absent, implicit, ad-hoc, or unsafe.**

Rationale:

- The rubric band `1-3` is defined as "Absent, implicit, ad-hoc, or
  unsafe"
  (`dimensions/18.03-regression-gating-ci-integration.md:32-33`); an
  empty source cannot score above this band because there is no CI
  pipeline config, regression-gate code, baseline comparison script,
  drift tracking dashboard, or eval report generator
  (`dimensions/18.03-regression-gating-ci-integration.md:17-21`).
- Per the study's hard source-isolation rules, neighboring sources
  and the upstream `https://github.com/microsoft/agent-framework`
  repository may not be inspected. Any substitute evidence would
  violate Rule 1.
- The lowest rubric anchor (1) is therefore the only defensible
  score; 1 is chosen over 2-3 because there is not even an implicit
  or ad-hoc signal in the source — the directory contains literally
  zero files.

## Evidence Collected

Every entry includes a file path with line numbers in the form
`path/to/file.ext:NN`. Because the selected source tree is empty, the
"absent" rows cite the dimension file's own lines that define what
should have been present; the structural rows cite the dimension
file's required-output path and the upstream manifest that names the
dimension as applicable.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Source tree contents (zero files inside selected source path) | Dimension file defines the five evidence categories that should have been found; selected source has none of them | `dimensions/18.03-regression-gating-ci-integration.md:17-21` |
| Required check #1: evals integrated into CI | Dimension step mandates inspection for CI pipeline config; none found | `dimensions/18.03-regression-gating-ci-integration.md:9` |
| Required check #2: regressions block deployments | Dimension step mandates inspection for regression-gate code; none found | `dimensions/18.03-regression-gating-ci-integration.md:10` |
| Required check #3: baseline comparison logic | Dimension step mandates inspection for baseline-comparison scripts; none found | `dimensions/18.03-regression-gating-ci-integration.md:11` |
| Required check #4: pass-rate drift tracking | Dimension step mandates inspection for drift-tracking dashboards; none found | `dimensions/18.03-regression-gating-ci-integration.md:12` |
| Required check #5: eval result reporting | Dimension step mandates inspection for eval report generation; none found | `dimensions/18.03-regression-gating-ci-integration.md:13` |
| Required evidence category #1: CI pipeline config | Evidence category listed in dimension; no file in source matches | `dimensions/18.03-regression-gating-ci-integration.md:17` |
| Required evidence category #2: regression gate code | Evidence category listed in dimension; no file in source matches | `dimensions/18.03-regression-gating-ci-integration.md:18` |
| Required evidence category #3: baseline comparison scripts | Evidence category listed in dimension; no file in source matches | `dimensions/18.03-regression-gating-ci-integration.md:19` |
| Required evidence category #4: drift tracking dashboards | Evidence category listed in dimension; no file in source matches | `dimensions/18.03-regression-gating-ci-integration.md:20` |
| Required evidence category #5: eval report generation | Evidence category listed in dimension; no file in source matches | `dimensions/18.03-regression-gating-ci-integration.md:21` |
| Rubric band that applies to this finding | "Absent, implicit, ad-hoc, or unsafe" → 1-3 | `dimensions/18.03-regression-gating-ci-integration.md:32-33` |
| Required output path for this study | `reports/repo/18.03-regression-gating-and-ci-integration/{repo-name}.md` (rendered to `reports/source/18.03-regression-gating-ci-integration/agent-framework.md`) | `dimensions/18.03-regression-gating-ci-integration.md:41-43` |
| Upstream manifest declares 18.03 applicable to this source | `agent-framework` listed at index line for dimension `18.03` | `sources/agent-framework.ultraplan-source.yml:99` |

## Answers to Dimension Questions

1. **Do evals run in CI?**
   No evidence found. The dimension step that mandates this check is
   at `dimensions/18.03-regression-gating-ci-integration.md:9`, and
   the corresponding evidence category ("CI pipeline config") is at
   `dimensions/18.03-regression-gating-ci-integration.md:17`. Inside
   `studies/agent-harness-study/sources/agent-framework/` (zero
   files) there is no `.github/workflows/*.yml`,
   `.gitlab-ci.yml`, `azure-pipelines.yml`,
   `.circleci/config.yml`, `Makefile`, `tox.ini`, `noxfile.py`,
   `Makefile` target, `.azuredevops/*`, or `package.json`
   `scripts` block that would indicate a CI entrypoint.
2. **Do regressions block deployments?**
   No evidence found. The dimension step that mandates this check is
   at `dimensions/18.03-regression-gating-ci-integration.md:10`, and
   the corresponding evidence category ("regression gate code") is at
   `dimensions/18.03-regression-gating-ci-integration.md:18`. Inside
   `studies/agent-harness-study/sources/agent-framework/` (zero
   files) there is no deployment configuration, no release-gating
   script, no `deploy.yml` / `release.yml` workflow file, no
   branch-protection policy, and no required-status-check
   definition.
3. **Are results compared to baselines?**
   No evidence found. The dimension step that mandates this check is
   at `dimensions/18.03-regression-gating-ci-integration.md:11`, and
   the corresponding evidence category ("baseline comparison
   scripts") is at
   `dimensions/18.03-regression-gating-ci-integration.md:19`. Inside
   `studies/agent-harness-study/sources/agent-framework/` (zero
   files) there is no `*.baseline.*`, `*.expected.*`,
   `__snapshots__/`, `*.snap`, `golden.json`, or comparison script.
4. **Is pass-rate drift tracked?**
   No evidence found. The dimension step that mandates this check is
   at `dimensions/18.03-regression-gating-ci-integration.md:12`, and
   the corresponding evidence category ("drift tracking dashboards")
   is at
   `dimensions/18.03-regression-gating-ci-integration.md:20`. Inside
   `studies/agent-harness-study/sources/agent-framework/` (zero
   files) there is no time-series storage, no `*.jsonl` run log, no
   dashboard definition, no Grafana/Datadog/StatsD integration, and
   no `*drift*` / `*trend*` / `*delta*`-named configuration.

## Architectural Decisions

No evidence found. The selected source directory contains no files
from which architectural decisions could be inferred. The dimension
step that mandates this kind of investigation is at
`dimensions/18.03-regression-gating-ci-integration.md:9-13`, but the
five required artifact categories at
`dimensions/18.03-regression-gating-ci-integration.md:17-21` are all
absent from the source. Per source-isolation rules, neighboring
sources (`langfuse`, `crewai`, `letta`, `temporal`, `openhands`,
`langgraph`, `openai-agents-sdk`, `pydantic-ai`, `opa`) and the
public `https://github.com/microsoft/agent-framework` repository
were not inspected.

## Notable Patterns

No evidence found. The selected source directory contains no files
from which implementation patterns could be extracted. The five
evidence categories that a CI / regression-gating study normally
draws on are listed at
`dimensions/18.03-regression-gating-ci-integration.md:17-21`; none
are present in
`studies/agent-harness-study/sources/agent-framework/`.

## Tradeoffs

No evidence found. Tradeoff analysis requires reading implementation
choices; with zero files in the selected source, no tradeoffs can be
attributed to this source. The dimension's own tradeoff vocabulary
("absent / implicit / ad-hoc / unsafe" vs "mature / durable /
observable / extensible / proven under failure or scale") is defined
at `dimensions/18.03-regression-gating-ci-integration.md:32-37`, and
this source lands squarely in the lower band.

## Failure Modes / Edge Cases

No evidence found. Failure-mode analysis requires reading retry /
recovery / rate-limit logic; the selected source has none. The
dimension's guiding question at
`dimensions/18.03-regression-gating-ci-integration.md:39` ("Can a
code change that degrades quality be blocked before deployment?")
cannot be answered from an empty source.

## Future Considerations

The only finding that follows from the empty source is a process
recommendation, not a code one: before any dimension study can be
meaningfully executed against `agent-framework`, the source must be
materialized under
`studies/agent-harness-study/sources/agent-framework/`. Without
that, every dimension question
(`dimensions/18.03-regression-gating-ci-integration.md:25-28`)
collapses to "no evidence found" and every rating collapses to the
1-3 band of the rubric
(`dimensions/18.03-regression-gating-ci-integration.md:32-33`). The
required output path for this study is fixed at
`dimensions/18.03-regression-gating-ci-integration.md:41-43`.

## Questions / Gaps

- **Is the source missing intentionally?** The directory
  `studies/agent-harness-study/sources/agent-framework/` exists but
  no files have been materialized inside it. The source manifest
  `sources/agent-framework.ultraplan-source.yml` declares the upstream
  URL (`https://github.com/microsoft/agent-framework` at
  `sources/agent-framework.ultraplan-source.yml:2`) and 119
  applicable dimensions including `18.03` (line
  `sources/agent-framework.ultraplan-source.yml:99`), but the study
  prompt itself does not list the manifest as an allowed inspection
  target for evidence, so the ingestion status is not asserted from
  this report.
- **Are sibling-source clones valid substitutes?** Per the study's
  hard source-isolation rules (Rule 1), accessing files from another
  source under `studies/agent-harness-study/sources/*` is **banned**.
  Substituting findings from a populated sibling (e.g., `langfuse`,
  `temporal`) for this dimension study would invalidate the study.
- **Are public-repo fetches allowed?** No. The hard rules say
  "Inspect only the selected source directory. Do not inspect
  unrelated workspace files, sibling sources, provider
  configuration, or generated reports except the explicit template
  and dimension inputs in this prompt." Fetching from
  `https://github.com/microsoft/agent-framework` is outside that
  boundary and would also violate Rule 1.
- **What would unblock this study?** A populated snapshot of the
  Microsoft agent-framework repository at a known commit (CI
  workflows, tests, eval/baseline config, and report generators)
  under
  `studies/agent-harness-study/sources/agent-framework/`. Once
  materialized, the five dimension steps at
  `dimensions/18.03-regression-gating-ci-integration.md:9-13`
  become executable and the rating can move out of the 1-3 band.
- **Does Microsoft agent-framework have any CI / regression-gating
  infrastructure upstream?** Cannot answer from this source. The
  upstream repo may include Azure Pipelines workflows under
  `azure/devops/`, `python/`, and `dotnet/` directories typical for
  the dual-language project, but inspecting any of those would
  violate the source-isolation rules in force for this task.

---

Generated by `18.03-regression-gating-and-ci-integration` against `agent-framework`.
