# Project reasoning index

> Project: `[project-slug]`
> Purpose: select and route the evidence required to settle project-wide decision clusters.

## Reasoning Areas

| Area | Template | Output | Required | Depends On | Why |
| --- | --- | --- | --- | --- | --- |
| `[decision cluster]` | `[catalogued template path]` | `projects/[project-slug]/project-reasoning/areas/[slug].md` | yes/no | none or `[area]` | `[why this area is needed]` |

## Evidence Assignments

| Area | Evidence | Relevant Questions | Why Assigned |
| --- | --- | --- | --- |
| `[decision cluster]` | `[catalogued report path]` | `[questions answered]` | `[assignment rationale]` |

## Source Document Assignments

| Area | Source | Authority | Why Assigned |
| --- | --- | --- | --- |
| `[decision cluster]` | `[catalogued source path]` | `[authority level]` | `[assignment rationale]` |

## Excluded Evidence

| Source | Reason Excluded | Revisit Trigger |
| --- | --- | --- |
| `[catalogued path]` | `[why it is excluded]` | `[condition for reconsideration]` |

Select only catalogued project-reasoning templates, evidence, and source documents. Keep outputs contained under `project-reasoning/areas/`, model many-to-many evidence assignments, and declare dependencies only when one area consumes another area's output.
