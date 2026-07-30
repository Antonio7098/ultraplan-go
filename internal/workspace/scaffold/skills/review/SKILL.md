---
name: ultraplan-review
description: Review sprint implementation against requirements, reasoning, and plan while synchronizing UltraPlan state.
---

# Review

Use for manual invocation of the review stage. Default to performing the review; provide only a proposal when explicitly requested.

Discover the workspace/project/sprint and implementation repository. Inspect sprint and execution state, git diff, requirements, sprint index, handbook, reasoning, plan, tests, and prior review evidence. Validate execute prerequisites and confirm the implementation being reviewed is current.

If execution is incomplete, evidence is stale, or required artifacts are invalid, explain the exact gaps and ask whether to fill them. On approval, perform only the necessary prerequisite work.

Run the UltraPlan review workflow. Review correctness, requirement coverage, design fidelity, failure semantics, security, maintainability, compatibility, observability, tests, and undocumented plan deviations. Findings must be concrete, prioritized, and tied to code or governing artifacts. Fix findings only when the user asks or the active review workflow explicitly includes repair; otherwise preserve review independence.

Write/update review evidence, validate the review stage, reconcile `flow-state.json`, and re-run status. Report assessment, findings, evidence freshness, blockers, changed artifacts, and smoke eligibility.