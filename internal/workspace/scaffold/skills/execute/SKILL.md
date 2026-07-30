---
name: ultraplan-execute
description: Execute the validated sprint plan, maintain durable execution state, and synchronize UltraPlan flow state.
---

# Execute

Use for manual invocation of sprint execution. Default to executing; provide only a proposal when explicitly requested.

Discover the workspace/project/sprint and implementation repository. Inspect sprint status, `flow-state.json`, `plan.md`, execution state, git status, and configured runtime. Validate the plan and all execution prerequisites. Never overwrite unrelated local changes.

When prerequisites are missing or the worktree makes execution unsafe, explain the exact gaps and ask whether to fill or resolve them. On approval, complete only the required prerequisite work.

Run the UltraPlan execution workflow, resuming durable state where appropriate. Implement tasks in plan order, preserve traceability, validate each task, update execution evidence/state after every transition, and stop on material divergence rather than silently improvising beyond the plan. Small implementation discoveries may be handled and documented; design-changing discoveries must return to reasoning/plan and synchronize those artifacts first.

After execution, run the relevant tests/builds, validate the execute stage, reconcile `flow-state.json`, and re-run status. Report implemented tasks, checks, deviations, remaining failures, changed files, and review eligibility.