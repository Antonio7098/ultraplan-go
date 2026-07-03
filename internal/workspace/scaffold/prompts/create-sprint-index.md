# Create Sprint Index

> **Inputs:** `project-index.md`, `requirements.md`, `docs/*.md`, injected sprint-index template

---

Use this prompt to create the sprint index for a sprint.

## Required Inputs

Read these files first:

1. Project index: `.ultra/projects/{project}/project-index.md`
2. **Sprint requirements**: `.ultra/projects/{project}/sprints/{sprint-slug}/requirements.md`
3. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
4. Injected sprint-index template section in this prompt

## Output

Write to: `.ultra/projects/{project}/sprints/{sprint-slug}/sprint-index.md`

Use the injected sprint-index template section. Fill every section. The sprint index selects what must be read, distilled, reasoned through, or checked for this sprint. It does NOT make implementation decisions.

## Instructions

1. Read `requirements.md` first — it defines the sprint goal, required outputs, acceptance criteria, non-goals, and constraints.
2. Read the project index to understand the available pool of contracts, studies, evidence reports, reasoning templates, and protocols.
3. Read the PRD and TRD for supporting product and technical context.
4. Use the injected sprint-index template section. Fill every section.
5. At the top of `sprint-index.md`, write `> **Inputs Used:**` listing the exact files used.
6. **Contracts**: list each selected contract by simple name (e.g. "Architecture"). The contract applies as a flat whole — no requirement ID mappings or per-clause breakdowns. If a clause is particularly important, mention it in the Why Selected column.
7. **Selected Evidence Reports**: copy the relevant rows from the project index's "Available Evidence Reports" table. These tell the technical handbook which reports to read.
8. **Reasoning templates**: select which area reasoning templates apply and specify their output filenames.
9. **Excluded context**: record what is explicitly excluded and why. The table must include explicit rows covering these known deferred behaviors when they are out of scope: implementation execution, smoke investigation, review automation, issue tracking, and Git mutation.
10. Do not invent sections or add content outside the template.

## Skip Criteria

Skip creating sprint-index if:

- `sprint-index.md` already exists and is complete
- Required inputs (project-index, requirements.md) are missing
- Selected sections are empty or contain placeholders

## Quality Bar

The sprint index must:

- Name the sprint goal and planned output clearly
- List selected contracts by simple name only (flat, not mapped to IDs)
- Copy the relevant evidence report rows from the project index
- Record carry-forward decisions from prior sprints
- Explicitly exclude non-goals and non-relevant context
- Include explicit Excluded Context rows for implementation execution, smoke investigation, review automation, issue tracking, and Git mutation unless the sprint requirements explicitly bring one of those behaviors into scope
- Reference only items that appear in the project index (no new items invented)
