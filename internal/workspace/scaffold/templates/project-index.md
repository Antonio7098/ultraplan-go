# Project Index: [Project Name]

> Project: `[project-slug]`
> Purpose: available governance, evidence, reasoning, and review pool for the project.

This document defines what can be selected for project work. It does not decide what any individual sprint must use.

## Project Scope

- **Project Slug:** `[project-slug]`
- **Repository:** `[repo path or URL]`
- **Target Implementation Directory:** `[local filesystem path where sprint implementation files are written]`
- **Primary Goal:** `[project goal]`
- **Non-Goals:** `[explicit exclusions]`

## Active Contract Pool

| Contract | Path | Applies To | Selection Notes |
| --- | --- | --- | --- |
| `[contract name]` | `.ultra/system/contracts/[area]/[file].md` | `[scope]` | `[when to select]` |

## Available Reasoning Templates

| Template | Path | Use When |
| --- | --- | --- |
| `[project-specific area]` | `projects/[project-slug]/reasoning/[area].md` | `[selection rule]` |
| `[shared area]` | `reasoning/[area].md` | `[selection rule]` |

## Available Studies

| Study | Path | Useful For | Status |
| --- | --- | --- | --- |
| `[study name]` | `.ultra/studies/[study-name]/` | `[area]` | `[draft/final/current]` |

## Available Evidence Packs And Reports

| Evidence | Path | Summary | Use When |
| --- | --- | --- | --- |
| `[evidence name]` | `[path]` | `[short summary]` | `[selection rule]` |

## Prior Decisions

| Decision | Path | Carry Forward When |
| --- | --- | --- |
| `[decision]` | `.ultra/projects/[project-slug]/DECISIONS.md` | `[condition]` |

## Review Protocols

| Protocol | Path | Required When |
| --- | --- | --- |
| `[protocol]` | `.ultra/system/protocols/[protocol].md` | `[condition]` |

## Selection Rules

- Select contracts by changed surface, runtime impact, data sensitivity, and public API exposure.
- Select reasoning templates only when the sprint needs deeper area-specific analysis.
- Keep project-owned area reasoning documents under `projects/[project-slug]/reasoning/`.
- A project may use shared workspace reasoning documents, but cannot select reasoning documents owned by another project.
- Select studies as evidence inputs, not as implementation instructions.
- Select prior decisions when they constrain compatibility, architecture, or operational behavior.
- Select review protocols before implementation starts so evidence can be planned.

## Maintenance Notes

- Update this index when the project adopts new contracts, studies, evidence packs, reasoning templates, or review protocols.
- Do not duplicate sprint-specific selections here. Put sprint-specific selections in `sprint-index.md`.
