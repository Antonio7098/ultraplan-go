# Ultra System Templates

These templates implement the canonical sprint flow:

```text
project-index.md
  -> requirements.md
  -> code-context.md
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
| `code-context.md` | Stores repository-relative source references selected after requirements; downstream prompts resolve source transiently while preserving the artifact bytes exactly. |
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
- `flow --to plan` runs `code-context` exactly once after requirements. Later agent-backed stages share exact requirements/context bytes, transient untrusted source evidence, and permission for additional live repository inspection.
- `$ultraplan-code-context` is manual-only and delegates to the canonical `sprint ... flow --to code-context` operation; it does not duplicate stage mechanics.
- `project-index.md` is the available pool.
- `sprint-index.md` is the selected pool.
- Area-specific reasoning is optional for small sprints and may be owned by the project under `projects/<project>/reasoning/`.
- Project reasoning prompt overrides live under `projects/<project>/prompts/`.
- The project final reasoning template override lives at `projects/<project>/templates/sprint-reasoning.md`.
- `sprint-reasoning.md` decides.
- `sprint-plan.md` executes.
- `review.md` checks conformance.

## Canonical Reference

See `sprint_governance_distillation_system.md` at the repository root for the full workflow model.
