# Ultra System Templates

These templates implement the canonical sprint flow:

```text
project-index.md
  -> sprint-index.md
  -> technical-handbook.md
  -> reasoning/*.md
  -> sprint-reasoning.md
  -> plan.md
  -> implementation
  -> review.md
```

## Templates

| Template | Purpose |
| --- | --- |
| `project-index.md` | Defines the available project governance, reasoning, studies, evidence, decisions, and review pool. |
| `sprint-index.md` | Selects the subset of project context that applies to one sprint. |
| `technical-handbook.md` | Distills selected studies and reports into sprint-specific technical guidance without making final decisions. |
| `sprint-reasoning.md` | Synthesizes selected context and makes final sprint decisions. |
| `sprint-plan.md` | Executes `sprint-reasoning.md`; must not invent architecture or scope. |
| `review.md` | Checks implementation conformance after execution. |
| `meta-report.md` | Existing meta-study report template. |
| `report.md` | Existing study report template. |
| `repo-analysis.md` | Existing repository analysis template. |

## Rules

- Planning artifacts live once.
- `project-index.md` is the available pool.
- `sprint-index.md` is the selected pool.
- Area-specific reasoning is optional for small sprints.
- `sprint-reasoning.md` decides.
- `sprint-plan.md` executes.
- `review.md` checks conformance.

## Canonical Reference

See `sprint_governance_distillation_system.md` at the repository root for the full workflow model.
