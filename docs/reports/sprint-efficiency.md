# Sprint Flow Efficiency Audit

Date: 2026-08-21
Baseline commit: `a8cb4a8bcdd3518795d46eac25e15c7bf876aa1f`
Implementation branch: `codex/sprint-efficiency`
Worktree: `/home/antonioborgerees/coding/ultraplan/ultraplan-go-sprint-efficiency`

## Outcome

The sprint pipeline now has one auditable prompt assembly contract, a stable provider-cache candidate prefix, bounded derived context reuse, smaller coverage-specific agent inputs, complete bounded direct-input packets, and persisted content-free runtime measurements. Agents receive governed upstream material directly and in deterministic dependency order instead of first spending tool calls rediscovering it. The changes also reduce local shared-context composition cost and make provider token/cache behavior measurable.

No exact-match dependency freshness or automatic stage invalidation was added. The existing completed-review and completed-smoke snapshot freshness switches remain `false`. Review fingerprint behavior was not re-enabled or copied to another stage. Content hashes introduced here are used only for disposable cache lookup, prompt explainability, runtime observability, and existing session diagnostics; cache misses fall back to live composition and never rerun a stage. Interrupted sessions remain compatible across prompt edits when provider, model, and work directory still match.

## Baseline audit

The original flow already placed requirements, code-context, and resolved source evidence before a constant stage boundary. The main inefficiencies and ambiguities were:

- Prompt structure was implicit string concatenation, with no machine-readable order, cache breakpoint, or per-stage input contract.
- Every top-level operation resolved live code-context again. Execute edits could therefore churn source excerpts and reduce prefix reuse even when the governed planning artifacts had not changed.
- Overlapping and duplicate source selections were emitted repeatedly, and each range rescanned its file.
- Multi-area reasoning used one request shaped around the first template while expecting all selected outputs.
- Execute opened an independent agent session for every top-level plan task, repeating shared context and forcing related tasks to rediscover prior work.
- Final reasoning, plan, and smoke told agents where artifacts lived but did not provide a small decision-focused handoff, encouraging repeated file reads.
- Every reviewer received sibling coverage sources that it did not own.
- Code-context source/range/budget errors were discovered downstream rather than before artifact promotion.
- A validation repair resent the original code-context request inside an existing session.
- Provider-reported cache reads/writes, token usage, prompt split, and cost were not persisted, so real cache savings could not be compared.

## Implemented improvements

### Stable, explainable prompt assembly

Shared prompts expose this exact ordered block contract:

1. `shared-instructions`
2. `requirements`
3. `code-context`
4. `source-evidence`
5. `stage-boundary`
6. `stage-instructions`
7. one block per direct input, using its canonical input ID
8. `stage-tail` when content follows the final direct input

`sprint <project> <sprint> prompt <stage> --explain` returns content-free byte counts, SHA-256 digests, stable-prefix breakpoint, cache key, transport status, stage input contract, and `full`/`partial` mode for each directly copied input. Volatile stage/run/task/reviewer data remains after the boundary. Session continuation instructions are inserted after the boundary, preserving the prefix.

Runtime requests carry a cache foundation key and a provider/model/work-directory/policy-scoped cohort key in metadata. This prevents observability from incorrectly grouping incompatible runtime envelopes.

### Derived context reuse without freshness coupling

Successful code-context generation validates actual file resolution, line ranges, UTF-8, reference/line limits, and the complete 256 KiB prompt budget before promotion. It then stores up to eight content-addressed context packs per sprint under `.ultra/cache/sprint-context/`. Existing sprints create the same pack lazily on their first real runtime composition; prompt previews remain read-only, and cache-write failure is ignored.

The pack freezes the exact resolved source evidence selected during planning so later execute edits do not churn the shared planning prefix. Requirements, code-context, and canonical target identity choose the pack. A missing, corrupt, or different pack simply causes live rendering; it does not make an artifact stale and does not schedule work.

### Less duplicated context and agent work

- References are grouped by file in first-file order; adjacent, overlapping, and duplicate ranges are sorted and merged.
- Authored entry labels remain visible, while source bytes appear once and each file is scanned once.
- Reference rationale/symbol metadata is no longer repeated beside source because the exact code-context artifact already contains it.
- Reference count is capped at 64 and unique selected source lines at 4,096, with errors reporting the correct unit.
- Area reasoning launches one independently validated request for each missing/invalid selected template and excludes sibling templates.
- Valid area outputs are skipped rather than regenerated.
- Every agent-backed stage receives its governed file inputs directly when those inputs are bounded files. Full files are preferred. If the 64 KiB stage-suffix budget cannot hold everything, each available dependency receives a fair head/tail excerpt, is marked `partial`, and reports its omitted byte count. Unavailable inputs receive a safe omission reason and remain explicit fallbacks rather than silent gaps.
- Requirements receives project index, roadmap, project docs, and all existing prior-sprint reviews except the current sprint. Sprint index receives project definitions. Handbook receives the selected sprint index and selected reports. Area/final reasoning, plan, and execute receive their required project context and all applicable prior sprint artifacts. Review receives its coverage-specific governed packet. Smoke authoring receives the complete planning, execution, review, and run-state chain.
- The technical handbook is copied as a complete artifact into Plan and Execute, so `Examples Worth Investigating`/`Examples Worth Inspecting` and surrounding rationale arrive together without a separate special-case excerpt.
- Execute uses one agent session for the ordered pending-task queue. The first turn receives shared sprint context, project definitions, every prior planning artifact, the concise queue, current task, and safety policy; later tasks are compact continuation turns in the same session. UltraPlan still checkpoints status and evidence after each task, resumes from the latest compatible session, stops on a failed/cancelled task, and falls back to a full fresh prompt if the runtime returns no reusable session ID.
- Reviewers receive their own contract/handbook plus common governed inputs, execution evidence, and changed files, but no sibling coverage contract/handbook.
- Code-context repair reuses the existing session without duplicating the full original request.

The implementation repository itself is the intentional exception to file copying: code-context and execution must inspect the current approved repository, which is unbounded and mutable working state. Requirements are still copied into code-context, and the validated code-context references plus resolved source evidence are copied into the stable shared foundation for every later stage.

| Stage | Direct governed inputs beyond generated instructions/templates |
|---|---|
| requirements | project index → roadmap → project docs → prior sprint reviews |
| code-context | complete validated requirements; live approved implementation repository |
| sprint-index | shared requirements/code/source → project index → roadmap → project docs |
| technical-handbook | shared foundation → sprint index → selected evidence reports |
| area-reasoning | shared foundation → project docs → sprint index → handbook → selected contracts/evidence/protocols |
| reasoning | shared foundation → project index → roadmap → project docs → sprint index → handbook → selected context → area outputs |
| plan | shared foundation → project index → roadmap → project docs → sprint index → handbook → area outputs → final reasoning |
| execute | shared foundation → project index → roadmap → project docs → sprint index → handbook → area outputs → reasoning → plan → queue/current task |
| review | shared foundation → current coverage source → governed review inputs → changed target files |
| smoke | shared foundation → sprint index → handbook → area outputs → reasoning → plan → execute → review → execute run state → harness contract |

### Measurement and operational visibility

All sprint runtime calls now append bounded records to the sprint's `.runtime-metrics.json`. Records contain no prompts or raw runtime payloads. They capture stage/operation identity, model, status, total/prefix/suffix bytes and digest, cache key, provider-reported input/output/reasoning/cache-read/cache-write/total tokens, turns, cost, timestamps, and error category. Writes are atomic, bounded to 512 records, and serialized across review fan-out; a metrics write failure becomes a warning rather than failing the stage.

Use:

```text
ultraplan sprint <project> <sprint> metrics
ultraplan sprint <project> <sprint> metrics --json
```

## Baseline versus improved

The same deterministic fixture and `TestSprintEfficiencyMetrics` measurement were run at the baseline commit and after the changes. Bytes are exact rendered prompt bytes.

| Stage | Baseline total | Improved total | Baseline prefix | Improved prefix | Direct blocks | Explanation |
|---|---:|---:|---:|---:|---:|---|
| sprint-index | 10,116 | 11,820 | 2,080 | 2,016 | 3 | project index, roadmap, and PRD copied |
| technical-handbook | 7,951 | 9,740 | 2,080 | 2,016 | 2 | sprint index and selected report copied |
| area-reasoning | 8,442 | 11,572 | 2,080 | 2,016 | 4 | project doc, sprint artifacts, and selected evidence copied |
| reasoning | 14,287 | 19,205 | 2,080 | 2,016 | 7 | project definitions and complete reasoning inputs copied |
| plan | 10,417 | 15,523 | 2,080 | 2,016 | 7 | project definitions and every prior planning artifact copied |
| execute | 3,279 | 9,661 | 2,080 | 2,016 | 8 | project definitions, every planning artifact, and queue copied |

Across the one-task preview fixture, total rendered bytes increased from 54,492 to 77,521 (+42.3%). The six prompts contain 31 directly inspectable input blocks totalling 20,101 bytes, all `full` and none `partial` in this fixture. This is an intentional exchange: UltraPlan submits the required material once so the agent does not have to discover paths and reread up to eight files with tool calls. It is not presented as a raw input-token reduction. Provider token/cost telemetry and tool-call telemetry over real sprints are required to determine the net economic result.

For a deterministic two-task execution fixture under the new complete packet contract, two independent full task prompts require 19,718 outbound prompt bytes. One shared execution session uses 10,767 bytes across the initial turn and compact continuation, a 45.4% reduction. The complete packet is sent once, not once per task. This measures UltraPlan's submitted prompt strings, not any conversation history that OpenCode or the provider may internally replay.

Other deterministic checks:

| Check | Baseline | Improved |
|---|---|---|
| execute usage summary populated | no | yes |
| prompt block order/cache split inspectable | no | yes |
| provider cache read/write persisted | no | yes, when reported |
| changed prompt requires exact session match | no | no (intentionally unchanged) |

### Shared-context composition benchmark

Command:

```text
go test ./internal/sprint -run '^$' -bench '^BenchmarkSharedPromptComposition$' -benchmem -count=5
```

The fixture contains overlapping and duplicate ranges from one 300-line source file. Median values from isolated five-run samples:

| Metric | Baseline | Improved | Change |
|---|---:|---:|---:|
| time/op | 794,056 ns | 274,019 ns | 65.5% lower |
| bytes/op | ~199,949 B | 79,673 B | 60.2% lower |
| allocations/op | 344 | 224 | 34.9% lower |

The baseline time samples were 788,225–818,137 ns/op. The improved isolated samples were 252,223–303,239 ns/op. Filesystem and CPU scheduling can vary absolute times; allocation reductions are the more stable result.

## Verification

- `go test ./... -count=1` — pass
- `go test -race ./internal/sprint ./internal/platform/runtime ./internal/web -count=1` — pass
- `go vet ./...` — pass
- Focused app, sprint, runtime, and packaged-web tests — pass
- Real dashboard prompt-summary endpoint against the representative `ultraplan-go/35-durable-run-observability` sprint — schema v2; individual full/partial project, handbook, area-reasoning, and final-reasoning blocks present; pass
- `git diff --check` — pass

Coverage added for prompt block order and input contracts, direct block explanation modes, canonical ordering, fair oversized-input allocation, UTF-8-safe excerpts, hard byte caps, safe omission diagnostics, absolute-path redaction, prior-review selection, complete handbook forwarding, complete smoke forwarding, review packet visibility, cache cohort isolation, frozen and lazy context reuse, preview read-only behavior, cache identity misses, reference/line budgets, concurrent metrics writers, metrics CLI JSON, shared multi-task execute sessions, compact continuations, missing-session fallback, stop-on-failure queue behavior, multi-template area isolation, reviewer packet isolation, tolerant interrupted-session reuse, and cache/token usage persistence.

The packaged-binary test now builds with `-buildvcs=false`; this makes the existing source-packaging assertion runnable from a linked Git worktree without changing production build behavior.

## Provider-cache limitation

The current `agentwrap` OpenCode adapter ultimately invokes `opencode run` with one opaque prompt. Caller metadata is available for policy/observability but is not translated into a provider-native cache-control block or breakpoint. Therefore this change makes the prompt prefix stable, supplies a precise future adapter directive, and measures provider-reported cache usage, but it does not prove or force native cache hits.

OpenCode or the underlying provider may cache identical prefixes automatically. Real savings should be evaluated from `.runtime-metrics.json` over repeated representative sprints, grouped by provider, model, work directory, policy cohort, and shared-prefix digest. No provider token or cost reduction is claimed in this report because baseline telemetry did not exist.
