---
name: ultraplan-plan
description: Produce an evidence-grounded sprint implementation plan and synchronize UltraPlan flow state.
---

# Plan

Use for manual invocation of the plan stage. Default to producing the plan; provide only a proposal when explicitly requested.

Discover the workspace/project/sprint, inspect status and `flow-state.json`, and validate requirements, sprint index, technical handbook, and required reasoning. If prerequisites are missing, stale, or contradictory, explain the concrete gaps and ask whether to fill them. On approval, complete and validate only the necessary prerequisite work.

Create or update `plan.md` using the effective UltraPlan prompt/template. Translate settled reasoning into an ordered, executable implementation sequence with scoped tasks, concrete files/components where evidence supports them, dependencies, validation for each step, rollback/recovery considerations, and traceability to requirements and decisions. Do not execute implementation work during this stage.

Validate the plan, reconcile flow state, re-run sprint status, and report changed files, validation, remaining blockers, and whether execution is now eligible.