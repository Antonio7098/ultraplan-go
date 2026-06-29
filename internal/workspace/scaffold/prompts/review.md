# Sprint Review - Evidence-Grounded Implementation Review

> **Inputs:** plan.md, reasoning.md, sprint-index.md, technical-handbook.md, docs/*.md, study-index.md, evidence packs, final reports

---

Use this prompt to review code implemented in a sprint against the study evidence that informed it.

The goal is verification, not re-planning. Check whether the implementation matches the sprint's decisions, follows the study-backed patterns, avoids the named anti-patterns, and meets the quality gates. Do not redesign or expand scope.

## Required Inputs

Load these files first:

1. Sprint plan: `.ultra/projects/{project}/sprints/{sprint-slug}/plan.md`
2. Sprint reasoning: `.ultra/projects/{project}/sprints/{sprint-slug}/reasoning.md`
3. Sprint index: `.ultra/projects/{project}/sprints/{sprint-slug}/sprint-index.md`
4. Technical handbook: `.ultra/projects/{project}/sprints/{sprint-slug}/technical-handbook.md`
5. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
7. Project study-index: `.ultra/projects/{project}/reports/study-index.md`
8. Evidence packs referenced by sprint plan
9. Final reports cited in sprint reasoning

## Review Workflow

1. Read the sprint plan and reasoning to understand what was supposed to be built and why.
2. Read the relevant project docs in `docs/` and the roadmap sections the sprint addresses.
3. Read the cited evidence packs and final reports.
4. Read the actual implementation code — explore the relevant directories.
5. Check each decision area against the evidence and the implementation.
6. Check tests, quality gates, and success criteria.
7. Write the review findings.

## What To Check

### Decision Fidelity

For every decision in the sprint reasoning:

- Does the implementation match the chosen approach?
- Does it satisfy the requirement the decision was based on?
- If a deviation was necessary, is it documented with the reason?

### Pattern Compliance

Cross-reference the implementation against the patterns cited in the evidence packs:

- Are the study-backed patterns followed in the actual code?
- Are the named anti-patterns avoided?
- If a pattern was not followed, is the reason documented?

### Test Coverage

- Do unit tests cover the logic the sprint scope touches?
- Do fixture tests exercise normal, malformed, and edge-case inputs?
- Are explicit deferrals justified with reason, impact, and follow-up?

### Quality Gates

Check the sprint's quality gates from the roadmap:

- Are all gates satisfied?
- If a gate cannot be checked yet, is that documented?

## Review Output

Write the review to:
`.ultra/projects/{project}/sprints/{sprint-slug}/review.md`

Include:

- Summary (sprint reviewed, files examined, review date)
- Findings by decision area (status, evidence check, code evidence, issues, recommendations)
- Pattern and anti-pattern check
- Test and quality gate assessment
- Decisions needing log update
- Overall assessment (Approve / Approve With Follow-ups / Revisions Needed)

## Quality Bar

The review must:

- Check every decision from sprint-reasoning.md
- Cite specific file paths and line numbers as evidence
- Record deviations with reason and risk
- Not introduce new scope or architecture
- Be actionable — recommendations must be specific
