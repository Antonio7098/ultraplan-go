# Create Sprint Reasoning

> **Inputs:** project-index.md, requirements.md, docs/*.md, sprint-index.md, technical-handbook.md, area reasoning (if any), sprint-reasoning template

---

Use this prompt to create the sprint reasoning document for a sprint.

## Required Inputs

Read these files first:

1. Project index: `.ultra/projects/{project}/project-index.md`
2. Sprint requirements: `.ultra/projects/{project}/sprints/{sprint-slug}/requirements.md`
3. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
4. Sprint index: `.ultra/projects/{project}/sprints/{sprint-slug}/sprint-index.md`
5. Technical handbook: `.ultra/projects/{project}/sprints/{sprint-slug}/technical-handbook.md`
6. Area reasoning files: `.ultra/projects/{project}/sprints/{sprint-slug}/reasoning/*.md` (if any)
7. Sprint reasoning template: `.ultra/system/templates/sprint-reasoning.md`

## Output

Write to: `.ultra/projects/{project}/sprints/{sprint-slug}/reasoning.md`

Use the sprint-reasoning template. Fill every section. This document makes the final sprint decisions — it synthesizes selected context, handbook evidence, area-specific reasoning, and contracts into final sprint decisions. It does NOT execute implementation.

## Instructions

1. Read `requirements.md` first.
2. Read the project index and all project docs in `docs/` for full supporting context.
3. Read the sprint index to understand selected context.
4. Read the technical handbook for study evidence.
5. Read area reasoning documents if present.
6. At the top of `reasoning.md`, write an `> **Inputs Used:**` line that lists the exact files used for this document.
7. If area reasoning files exist, summarize their actual conclusions. Do not say their output paths are empty, unwritten, or pending when files are present.
8. Synthesize all evidence into final sprint decisions.
9. For each decision:
   - State what will be done and why
   - Record the study/source grounding: cite `technical-handbook.md`, selected evidence reports, and concrete studied repos/source references where relevant
   - If no study sources are relevant to a decision, explicitly say so and justify why
   - Record trade-offs accepted, potential technical debt, and future impacts
   - Name rejected alternatives and why
   - Map to applicable contracts/requirement IDs
   - Define expected evidence (tests, logs, review checks)
10. Add a "Repos Studied / Source Evidence Used" section using the technical handbook and selected reports. Include why each repo/report mattered, and which decisions it influenced.
11. Add deeper analysis of accepted trade-offs, possible technical debt, and future considerations before final decisions.
12. Record assumptions and risks.
13. Define implementation constraints.
14. The plan must be able to execute these decisions without reopening architecture.

## Skip Criteria

Skip creating sprint-reasoning if:

- `reasoning.md` already exists and is complete
- Sprint index or technical handbook is missing
- Contains placeholders
- No final decisions are recorded

## Quality Bar

The sprint reasoning must:

- Reference sprint-index selected context
- Reference technical-handbook evidence
- Cite concrete study sources/repositories or explain why no sources were relevant
- Analyze accepted trade-offs, potential technical debt, and future considerations
- Make final, specific decisions (no "TBD" on core architecture)
- Record at least 2 rejected alternatives with rationale
- Map decisions to contracts and requirement IDs
- Define specific, reviewable expected evidence
- Record risks with mitigation or follow-up
- Be actionable by plan.md without reopening architecture
