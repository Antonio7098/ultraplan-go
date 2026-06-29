# Create Area Reasoning

> **Inputs:** technical-handbook.md, requirements.md, docs/*.md, reasoning templates

---

Use this prompt to create optional area-specific reasoning documents for a sprint.

## Required Inputs

Read these files first:

1. Technical handbook: `.ultra/projects/{project}/sprints/{sprint-slug}/technical-handbook.md`
2. Sprint requirements: `.ultra/projects/{project}/sprints/{sprint-slug}/requirements.md`
3. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
4. Area-specific reasoning templates: `.ultra/system/reasoning/*.md`

## Output

For each area selected by sprint-index, write to:
`.ultra/projects/{project}/sprints/{sprint-slug}/reasoning/<area>.md`

Only create files for areas explicitly selected in sprint-index. Do NOT create area reasoning documents for ceremony.

## Instructions

1. The flow has already determined which area files to create. Do not infer extra areas.
2. Read the technical handbook for evidence context.
3. Read `requirements.md` and all project docs in `docs/` for sprint-specific scope and constraints.
4. For each selected area:
   - Use the corresponding reasoning template as the output format
   - At the very top of the file, add an `> **Inputs Used:**` line listing the exact files used for that document
   - Ground decisions in technical handbook evidence
   - Record the key conclusion and evidence basis
   - Note any open questions or risks
5. Do not duplicate content from technical-handbook — synthesize into area-specific conclusions.
6. Ensure each area reasoning document is self-contained and can be understood without reading other area documents.

## Skip Criteria

Skip creating area reasoning if:

- No areas are selected in sprint-index
- Area reasoning files already exist and are complete
- Contains placeholders

## Quality Bar

Each area reasoning document must:

- Have a clear area name and scope
- Cite technical handbook evidence
- Make final area-specific decisions (no more "TBD")
- Record rejected alternatives
- Note risks, assumptions, and open questions
- Be referenced in sprint-reasoning.md
