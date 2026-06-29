# Sprint Plan: [Sprint Name]

> Project: `[project-slug]`
> Sprint: `[sprint-slug]`
> Source: `sprint-reasoning.md`
> **Inputs Used:** `.ultra/projects/[project-slug]/project-index.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/requirements.md`, `.ultra/projects/[project-slug]/docs/*.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/sprint-index.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/technical-handbook.md`, `reasoning/*.md` (if any), `.ultra/projects/[project-slug]/sprints/[sprint-slug]/reasoning.md

This plan executes `sprint-reasoning.md`. It must not invent architecture, scope, or decisions.

## Reasoning Source

- **Sprint Reasoning:** `sprint-reasoning.md`
- **Sprint Index:** `sprint-index.md`
- **Technical Handbook:** `technical-handbook.md`
- **Area Reasoning:** `reasoning/*.md` or `none`

## Sprint Status

- **Status:** `[not started / in progress / completed / blocked]`
- **Owner:** `[person or agent]`
- **Start Date:** `[date]`
- **Completion Date:** `[date or pending]`

## Decisions To Execute

| Decision | Source Section | Execution Implication |
| --- | --- | --- |
| `[decision]` | `sprint-reasoning.md#[section]` | `[what implementation must do]` |

## Requirements / Contracts To Satisfy

| Contract / Requirement ID | Required Behavior | Evidence Planned |
| --- | --- | --- |
| `[contract or ID]` | `[behavior]` | `[evidence]` |

## Tasks

- [ ] **Task 1: [Task Name]**
  > Executes: `[decision / requirement IDs]`
  - [ ] `[specific implementation step]`
  - [ ] `[specific implementation step]`

- [ ] **Task 2: [Task Name]**
  > Executes: `[decision / requirement IDs]`
  - [ ] `[specific implementation step]`
  - [ ] `[specific implementation step]`

## Evidence Checklist

- [ ] Tests prove the required behavior.
- [ ] Runtime or diagnostic evidence exists where required.
- [ ] Documentation updates are complete where required.
- [ ] Deviations from `sprint-reasoning.md` are recorded before implementation continues.
- [ ] Required review protocols have evidence.

## Verification Commands

| Check | Command | Expected Result |
| --- | --- | --- |
| `[check]` | `[command]` | `[expected result]` |

## Risks And Blockers

| Risk / Blocker | Source | Mitigation | Status |
| --- | --- | --- | --- |
| `[risk]` | `sprint-reasoning.md` | `[mitigation]` | `[open/mitigated/closed]` |

## Review Inputs

Review should use:

- `sprint-index.md`
- `technical-handbook.md`
- `reasoning/*.md` where created
- `sprint-reasoning.md`
- this `plan.md`
- implementation diff
- verification evidence
- required protocols from `sprint-index.md`

## Execution Log

| Date / Step | Action | Evidence / Notes |
| --- | --- | --- |
| `[date]` | `[action]` | `[evidence]` |

## Completion Criteria

- [ ] All tasks are complete or explicitly deferred.
- [ ] Verification commands were run or deferrals are documented.
- [ ] Evidence satisfies the expectations from `sprint-reasoning.md`.
- [ ] `review.md` can evaluate conformance without guessing intent.
