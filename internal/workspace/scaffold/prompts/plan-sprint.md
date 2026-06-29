# Sprint Planning - Evidence-Grounded Implementation Plan

> **Inputs:** project-index.md, requirements.md, docs/*.md, sprint-index.md, technical-handbook.md, area reasoning (if any), reasoning.md, sprint-plan template

---

Use this prompt to plan one implementation sprint for an Ultra project.

The outputs are a sprint reasoning document and then a sprint plan. Do not implement code. Do not make ungrounded architecture decisions. Every important decision must trace to project requirements, roadmap scope, evolved study evidence, or an explicitly named open question.

## Required Inputs

Load these files first:

1. Project index: `.ultra/projects/{project}/project-index.md`
2. Sprint requirements: `.ultra/projects/{project}/sprints/{sprint-slug}/requirements.md`
3. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
5. Sprint index: `.ultra/projects/{project}/sprints/{sprint-slug}/sprint-index.md`
6. Technical handbook: `.ultra/projects/{project}/sprints/{sprint-slug}/technical-handbook.md`
7. Area reasoning: `.ultra/projects/{project}/sprints/{sprint-slug}/reasoning/*.md` (if present)
8. Sprint reasoning: `.ultra/projects/{project}/sprints/{sprint-slug}/reasoning.md`
9. Sprint plan template: `.ultra/system/templates/sprint-plan.md`

## Evidence Loading Order

Use this order so the plan stays grounded:

1. Read `requirements.md` first.
2. Read the sprint reasoning to understand what was decided and why.
3. Read the sprint index to understand selected context.
4. Read the technical handbook for evidence.
5. Read area reasoning for area-specific decisions.
6. Read the PRD and TRD sections relevant to this sprint.
7. Read the project index for traceability back to selected studies/contracts if needed.
8. Open linked final reports only when a specific decision needs deeper evidence.
9. Resolve code references only for specific implementation questions.
10. At the top of `plan.md`, write an `> **Inputs Used:**` line that lists the exact files used for this document.

If context is too large:

1. Keep sprint reasoning, PRD, TRD, and roadmap sprint section in context.
2. Load evidence packs tied to current decisions.
3. Load final reports only when evidence packs are insufficient.
4. Record omitted evidence and why it was omitted.

## Planning Rules

- Start from fundamentals. Do not pull later-sprint complexity into earlier sprints.
- Respect the roadmap scope. If evidence suggests scope change, record as recommendation.
- Separate requirements from design decisions.
- Use sprint reasoning to justify design choices.
- Prefer small, testable increments over broad abstractions.
- Record tradeoffs, rejected alternatives, and anti-patterns.
- Do not implement code while planning.

## Decision Discipline

For every major decision, capture:

- Decision made
- Requirement it satisfies
- Evidence used
- Tradeoff accepted
- Alternative rejected
- Risk or follow-up

If evidence is insufficient, write an open question instead of guessing.

## Sprint Reasoning Output

Write the sprint reasoning to:
`.ultra/projects/{project}/sprints/{sprint-slug}/reasoning.md`

Use `sprint-reasoning.md` template. Fill every section.

## Sprint Plan Output

Write the sprint plan to:
`.ultra/projects/{project}/sprints/{sprint-slug}/plan.md`

Use `sprint-plan.md` template. Fill every section. The plan must cite `reasoning.md` and carry forward its decisions, expected evidence, risks, assumptions, and open questions.

## Quality Bar

A good sprint plan is specific enough that implementation can proceed without rereading every study report, but evidence-grounded enough that decisions can be audited later.

Avoid generic phrases like "clean architecture" unless named with evidence basis.
