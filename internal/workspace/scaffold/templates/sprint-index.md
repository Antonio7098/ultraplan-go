# Sprint Index: [Sprint Name]

> Project: `[project-slug]`
> Sprint: `[sprint-slug]`
> Purpose: selected context for this sprint. Must be a subset of `.ultra/projects/[project-slug]/project-index.md`.
> **Inputs Used:** `.ultra/projects/[project-slug]/project-index.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/requirements.md`, `.ultra/projects/[project-slug]/docs/*.md`

This document selects what must be read, distilled, reasoned through, or checked for this sprint. It does not make implementation decisions. All selections must come from the project index — no items may be included that are not listed in the project index.

## Sprint Scope

- **Sprint Goal:** `[goal]`
- **Planned Output:** `[deliverable]`
- **Depends On:** `[prior sprint/output or none]`
- **Non-Goals:** `[explicit exclusions]`

## Source Project Index

- `.ultra/projects/[project-slug]/project-index.md` — authoritative source. Any file or item referenced below must appear there.

## Selected Contracts

Each contract applies as a flat whole to this sprint. All paths must appear in the project index's "Active Contract Pool" table.

| Contract     | Why Selected                                 |
| ------------ | -------------------------------------------- |
| `[contract]` | `[brief reason — no ID mapping needed]`      |

## Selected Evidence Reports

Copied from the project index's "Available Evidence Reports" table. These tell the technical handbook which reports to read – the project index is the authoritative source.

| Report      | Path                                                   | Covers                          |
| ----------- | ------------------------------------------------------ | ------------------------------- |
| `[report]`  | `.ultra/studies/[study]/reports/final/[report].md`     | `[topic summary]`                |

## Selected Reasoning Templates

All paths must appear in the project index's "Available Reasoning Templates" table.

| Template     | Output Path                                                     | Why Selected |
| ------------ | -------------------------------------------------------------- | ------------ |
| `[template]` | `.ultra/projects/[project-slug]/sprints/[sprint-slug]/reasoning/[name].md` | `[reason]`   |

## Prior Decisions To Carry Forward

All decision paths must appear in the project index's "Prior Decisions" table.

| Decision     | Path     | Constraint For This Sprint |
| ------------ | -------- | -------------------------- |
| `[decision]` | `[path]` | `[constraint]`             |

## Required Review Protocols

All paths must appear in the project index's "Review Protocols" table.

| Protocol     | Path                                    | Required Evidence |
| ------------ | --------------------------------------- | ----------------- |
| `[protocol]` | `.ultra/system/protocols/[protocol].md` | `[evidence]`     |

## Excluded Context

| Context     | Reason Excluded | Revisit If    |
| ----------- | --------------- | ------------- |
| `[context]` | `[reason]`      | `[condition]` |

## Next Artifacts

- `technical-handbook.md` reads from the evidence reports listed above.
- `reasoning/*.md` captures area-specific reasoning.
- `reasoning.md` makes final sprint decisions.
- `plan.md` executes `reasoning.md`.
- `review.md` runs the selected review protocols against implementation.
